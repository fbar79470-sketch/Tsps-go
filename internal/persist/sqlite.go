package persist

import (
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLite backend: port of plugins/persistence/SqlitePlayerPersistence
// .plugin.js, SaveWorker.SqlitePlayerPersistence.js, and
// SaveHistory.SqlitePlayerPersistence.js (commit bfd81e3). One row per
// storage key in player_saves, plus a gzipped save history in
// player_save_history with hash dedupe and retention pruning.
//
// DRIVER DECISION: modernc.org/sqlite v1.57.0, a pure-Go SQLite
// implementation (no CGO). It was chosen over the CGO-based
// mattn/go-sqlite3 because the rest of this port builds with
// CGO_ENABLED=0, and over github.com/ncruces/go-sqlite3 because
// modernc built and tested cleanly here first (CGO_ENABLED=0 build
// with the pinned Go 1.25.1 toolchain verified, WAL mode confirmed at
// runtime). v1.57.0 is the newest modernc release whose go.mod
// requires only go 1.25: v1.60.0 and v1.62.0 require go 1.26 and
// would force a toolchain bump for the whole project. The driver is
// registered under the name "sqlite" via the blank import above; this
// file is the only place that imports it. Cold-build cost is
// significant (the SQLite amalgamation transpiled to Go); incremental
// builds reuse the build cache.
//
// DIVERGENCE (synchronous writes): TS writes asynchronously through a
// worker thread (SaveWorker.SqlitePlayerPersistence.js), keeping
// pending writes in an unwritten map so loads see the newest data, and
// surfacing failures via flush(). This port writes synchronously in
// Save, inside one transaction covering the row and its history copy
// (as the worker's commit does), and returns failures immediately —
// the same documented divergence as JSONFileStore. Callers get TS's
// "load sees the newest write" guarantee trivially, because Save does
// not return until the transaction commits. There is no Flush method,
// no unwritten/deleted/in-flight maps, and no writer restart logic.
//
// DIVERGENCE (single connection): TS keeps one read connection on the
// game thread and one write connection in the worker. This port uses a
// single database/sql connection (MaxOpenConns(1)) plus a mutex, so
// all operations serialize. SQLite's WAL mode still lets other
// processes read while this one writes.
//
// GAPS (NOT STARTED, absent — nothing here pretends otherwise):
//   - listSnapshots / restoreSnapshot (PlayerPersistence.ts
//     listSnapshots/restoreSnapshot; SqlitePlayerPersistence
//     .plugin.js:256-279): no Go API surface exists for them. The
//     history rows this store writes are in the TS shape, so a future
//     port reads them directly.
//   - Pending restores (pendingRestores, SqlitePlayerPersistence
//     .plugin.js:67, 218-223, 262-269): a rollback of an online player
//     has no representation here; Save always writes the given save.
//   - Legacy JSON import (importLegacySaves,
//     SqlitePlayerPersistence.plugin.js:316-372, env-gated by
//     PLAYER_SAVE_IMPORT_LEGACY_JSON and disabled by default): never
//     runs. Existing JSON saves are not copied into the database.
//   - Pid-file single-writer guard (markInUse,
//     SqlitePlayerPersistence.plugin.js:240-250): no `<database>.pid`
//     file is written. Concurrent writers rely on busy_timeout plus
//     the single-process assumption; the rollback script's guard
//     against running under a live server does not exist here.
//   - Bots: TS skips the history copy for bots (save(), line 225) but
//     still writes their row. The store layer cannot know whether a
//     save belongs to a bot; callers skip bot saves entirely, so every
//     Save here records a history copy. Documented, not handled.

// isoMillis is the layout of JavaScript Date.toISOString(): UTC, exactly
// three fractional digits, "Z" suffix (e.g. 2026-11-14T16:28:03.123Z).
// TS uses one such timestamp per save message for both the row's
// updated_at and the history copy's saved_at (enqueue,
// SqlitePlayerPersistence.plugin.js:126).
const isoMillis = "2006-01-02T15:04:05.000Z"

// sqliteSchema is the schema the TS constructor and SqliteSaveHistory
// create: the player_saves table (SqlitePlayerPersistence.plugin.js
// :54-58) and the player_save_history table + index
// (SaveHistory.SqlitePlayerPersistence.js:54-64).
const sqliteSchema = `
CREATE TABLE IF NOT EXISTS player_saves (
  username TEXT PRIMARY KEY,
  save_json TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS player_save_history (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  username TEXT NOT NULL,
  saved_at TEXT NOT NULL,
  reason TEXT NOT NULL,
  save_hash TEXT NOT NULL,
  save_gz BLOB NOT NULL
);
CREATE INDEX IF NOT EXISTS player_save_history_by_player
  ON player_save_history (username, saved_at);
`

// sqlitePragmas are the connection pragmas TS applies on both its
// connections (SqlitePlayerPersistence.plugin.js:49-52 and
// SaveWorker.SqlitePlayerPersistence.js:13-18).
var sqlitePragmas = []string{
	"PRAGMA journal_mode = WAL",
	"PRAGMA foreign_keys = ON",
	"PRAGMA busy_timeout = 5000",
	"PRAGMA synchronous = NORMAL",
}

// SQLiteStore is a SaveStore backed by a SQLite database file: the TS
// default backend. The key is the profile storage key (see StorageKey);
// the row holds the same JSON shape marshalSaveJSON produces for
// JSONFileStore. It is safe for concurrent use.
type SQLiteStore struct {
	mu     sync.Mutex
	db     *sql.DB
	path   string
	closed bool
}

var _ SaveStore = (*SQLiteStore)(nil)

// NewSQLiteStore opens (creating parent directories and the schema as
// needed) the SQLite save database at path, e.g.
// data/saves/players.sqlite. The path is passed verbatim to SQLite as
// the database filename. Open or schema failures are returned; the
// store never silently falls back to another backend. Call Close when
// the store is no longer needed.
//
// The default path and its PLAYER_SAVE_DATABASE_PATH env override live
// in config.Env.PlayerSaveDatabasePath (config/env.go), following the
// TS static (SqlitePlayerPersistence.plugin.js:34-36); this
// constructor takes the resolved path explicitly and reads no env.
func NewSQLiteStore(path string) (*SQLiteStore, error) {
	if path == "" {
		return nil, errors.New("persist: SQLite database path must not be empty")
	}
	// TS: fs.mkdirSync(path.dirname(DATABASE_PATH), { recursive: true })
	// (SqlitePlayerPersistence.plugin.js:46).
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("persist: creating SQLite directory for %s: %w", path, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("persist: opening SQLite database %s: %w", path, err)
	}
	// Single connection: see the DIVERGENCE note above.
	db.SetMaxOpenConns(1)
	s := &SQLiteStore{db: db, path: path}
	if err := s.init(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

// init applies the TS pragmas and creates the schema. The caller closes
// the database on error.
func (s *SQLiteStore) init() error {
	if err := s.db.Ping(); err != nil {
		return fmt.Errorf("persist: opening SQLite database %s: %w", s.path, err)
	}
	for _, pragma := range sqlitePragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("persist: applying %s on %s: %w", pragma, s.path, err)
		}
	}
	if _, err := s.db.Exec(sqliteSchema); err != nil {
		return fmt.Errorf("persist: creating SQLite schema in %s: %w", s.path, err)
	}
	return nil
}

// Path returns the database file path the store was opened with.
func (s *SQLiteStore) Path() string { return s.path }

// Close closes the database connection. It is idempotent; operations
// after Close fail with an error. The SaveStore interface has no Close;
// callers that construct a SQLiteStore must arrange to call this on
// shutdown (the coordinator wires it in cmd/server).
func (s *SQLiteStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.db.Close()
}

// Load reads a save row, applying the same load-time normalization as
// JSONFileStore (normalizeLoadedSave). A missing row yields
// ErrSaveNotFound; a malformed row yields an error naming the storage
// key. (TS load() would throw on a malformed row too: JSON.parse
// throws.) An unknown profile is an error, as in JSONFileStore.
func (s *SQLiteStore) Load(username string, profile Profile) (*PlayerSave, error) {
	if !profile.Valid() {
		return nil, fmt.Errorf("persist: unknown profile %q", string(profile))
	}
	// The row key: saveKey (SqlitePlayerPersistence.plugin.js:189-191)
	// is profileStorageKey(normalizeUsername(username), profile) —
	// exactly StorageKey. The main profile keeps the plain normalised
	// name; any other is prefixed ("league:bob"), which is also how
	// JSONFileStore's subdirectory mapping distinguishes the same
	// character (see FilePath).
	key := StorageKey(username, profile)
	s.mu.Lock()
	defer s.mu.Unlock()
	var saveJSON string
	err := s.db.QueryRow("SELECT save_json FROM player_saves WHERE username = ?", key).Scan(&saveJSON)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrSaveNotFound
		}
		return nil, fmt.Errorf("persist: reading save for %q: %w", key, err)
	}
	var save PlayerSave
	if err := json.Unmarshal([]byte(saveJSON), &save); err != nil {
		return nil, fmt.Errorf("persist: parsing stored save for %q: %w", key, err)
	}
	normalizeLoadedSave(&save, profile)
	return &save, nil
}

// Save validates and upserts save, recording a history copy with the
// given reason. The username comes from save.DisplayName and the
// profile from save.Profile ("" means main). Validation mirrors
// validateSerializedSave exactly as JSONFileStore does (see
// validateSaveForWrite); the row holds the same JSON marshalSaveJSON
// produces. The upsert is the worker's statement
// (SaveWorker.SqlitePlayerPersistence.js:28-33), and the row write and
// history copy commit in one transaction, as the worker's commit does
// (SaveWorker.SqlitePlayerPersistence.js:40-54). updated_at and the
// history copy's saved_at share one timestamp, as in TS's enqueue
// (SqlitePlayerPersistence.plugin.js:126).
func (s *SQLiteStore) Save(save *PlayerSave, reason string) error {
	if save == nil {
		return errors.New("persist: cannot save a nil PlayerSave")
	}
	profile := save.EffectiveProfile()
	if !profile.Valid() {
		return fmt.Errorf("persist: unknown profile %q", string(profile))
	}
	if save.DisplayName == "" {
		return errors.New("persist: cannot save a PlayerSave with an empty displayName")
	}
	if err := validateSaveForWrite(save); err != nil {
		return err
	}
	data, err := marshalSaveJSON(save)
	if err != nil {
		return fmt.Errorf("persist: encoding save for %q: %w", save.DisplayName, err)
	}
	key := StorageKey(save.DisplayName, profile)
	savedAt := time.Now().UTC().Format(isoMillis)

	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("persist: saving %q: %w", key, err)
	}
	if _, err := tx.Exec(`
		INSERT INTO player_saves (username, save_json, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET
		  save_json = excluded.save_json,
		  updated_at = excluded.updated_at
	`, key, string(data), savedAt); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist: saving %q: %w", key, err)
	}
	if err := recordHistory(tx, key, data, reason, savedAt); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist: recording history for %q: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("persist: saving %q: %w", key, err)
	}
	return nil
}

// Exists reports whether a save row exists. An unknown profile reports
// false, matching JSONFileStore. Any database error also reports false,
// as JSONFileStore reports false for any stat error.
func (s *SQLiteStore) Exists(username string, profile Profile) bool {
	if !profile.Valid() {
		return false
	}
	key := StorageKey(username, profile)
	s.mu.Lock()
	defer s.mu.Unlock()
	var one int
	return s.db.QueryRow("SELECT 1 FROM player_saves WHERE username = ?", key).Scan(&one) == nil
}

// Delete removes a save, mirroring the SQLite plugin's delete()
// (SqlitePlayerPersistence.plugin.js:290-299), which overrides the base
// class's "does not support deleting saves" throw
// (PlayerPersistence.ts delete): the current save is first copied into
// the history with reason "character-delete", the row is removed, and
// the character's whole history is re-keyed under
// `deleted:<epoch-millis>:<key>` so the name starts clean. Removing a
// missing save is not an error and leaves any history under the key
// untouched (TS returns early when there is no current save).
func (s *SQLiteStore) Delete(username string, profile Profile) error {
	if !profile.Valid() {
		return fmt.Errorf("persist: unknown profile %q", string(profile))
	}
	key := StorageKey(username, profile)
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	var current string
	err = tx.QueryRow("SELECT save_json FROM player_saves WHERE username = ?", key).Scan(&current)
	if err != nil {
		_ = tx.Rollback()
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	savedAt := time.Now().UTC().Format(isoMillis)
	// The history copy goes in first so the re-key below moves it with
	// the rest: the worker applies writes in message order (history,
	// then delete + rekey; SaveWorker.SqlitePlayerPersistence.js:42-47).
	// The copy is subject to the usual hash dedupe (history.record).
	if err := recordHistory(tx, key, []byte(current), "character-delete", savedAt); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	if _, err := tx.Exec("DELETE FROM player_saves WHERE username = ?", key); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	historyKey := fmt.Sprintf("deleted:%d:%s", time.Now().UnixMilli(), key)
	if _, err := tx.Exec("UPDATE player_save_history SET username = ? WHERE username = ?", historyKey, key); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("persist: deleting save for %q: %w", key, err)
	}
	return nil
}

// recordHistory is SqliteSaveHistory.record
// (SaveHistory.SqlitePlayerPersistence.js:86-91): insert a gzipped copy
// of serialized with its SHA-1 hash and reason, unless it matches the
// player's newest copy, then prune that player's copies per the
// retention rules. It runs inside the caller's transaction.
func recordHistory(tx *sql.Tx, key string, serialized []byte, reason, savedAt string) error {
	hash := sha1Hex(serialized)
	var latest string
	err := tx.QueryRow(
		"SELECT save_hash FROM player_save_history WHERE username = ? ORDER BY saved_at DESC, id DESC LIMIT 1",
		key).Scan(&latest)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil && latest == hash {
		return nil
	}
	gz, err := gzipBytes(serialized)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(
		"INSERT INTO player_save_history (username, saved_at, reason, save_hash, save_gz) VALUES (?, ?, ?, ?, ?)",
		key, savedAt, reason, hash, gz); err != nil {
		return err
	}
	rows, err := historyTimes(tx, key)
	if err != nil {
		return err
	}
	// TS prunes against Date.parse(savedAt) — the timestamp of the copy
	// just inserted — not the wall clock (SaveHistory:90).
	now, err := time.Parse(time.RFC3339Nano, savedAt)
	if err != nil {
		return fmt.Errorf("parsing saved_at %q: %w", savedAt, err)
	}
	for _, id := range idsToPrune(rows, now.UnixMilli()) {
		if _, err := tx.Exec("DELETE FROM player_save_history WHERE id = ?", id); err != nil {
			return err
		}
	}
	return nil
}

// sha1Hex is hashOf (SaveHistory.SqlitePlayerPersistence.js:46-48): the
// SHA-1 hex of the serialized save bytes.
func sha1Hex(serialized []byte) string {
	sum := sha1.Sum(serialized)
	return hex.EncodeToString(sum[:])
}

// gzipBytes is zlib.gzipSync(serialized) (SaveHistory:89). The gzip
// framing bytes need not match Node's; only the decompressed content is
// specified (SqliteSaveHistory.get gunzips).
func gzipBytes(serialized []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(serialized); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// historyRow is one { id, savedAt } row for retention pruning
// (SaveHistory.SqlitePlayerPersistence.js:28-29).
type historyRow struct {
	id      int64
	savedAt string
}

func historyTimes(tx *sql.Tx, key string) ([]historyRow, error) {
	rows, err := tx.Query("SELECT id, saved_at FROM player_save_history WHERE username = ?", key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []historyRow
	for rows.Next() {
		var r historyRow
		if err := rows.Scan(&r.id, &r.savedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Retention windows (SaveHistory.SqlitePlayerPersistence.js:14-18): keep
// every copy from the last 24 hours; from 24 hours to 7 days, the
// newest of each hour; from 7 to 30 days, the newest of each day;
// nothing older.
const (
	historyHourMillis = 60 * 60 * 1000
	historyDayMillis  = 24 * historyHourMillis
	historyKeepAll    = historyDayMillis
	historyKeepHourly = 7 * historyDayMillis
	historyKeepDaily  = 30 * historyDayMillis
)

// idsToPrune is the faithful port of idsToPrune
// (SaveHistory.SqlitePlayerPersistence.js:28-44). Rows may be in any
// order; nowMs is milliseconds since the epoch. Rows whose saved_at
// does not parse sort as the epoch (hardening: TS Date.parse yields
// NaN, whose sort order is unspecified; every row this store writes
// carries a valid ISO timestamp).
func idsToPrune(rows []historyRow, nowMs int64) []int64 {
	type parsed struct {
		id int64
		at int64
	}
	parsedRows := make([]parsed, 0, len(rows))
	for _, r := range rows {
		at := int64(0)
		if t, err := time.Parse(time.RFC3339Nano, r.savedAt); err == nil {
			at = t.UnixMilli()
		}
		parsedRows = append(parsedRows, parsed{id: r.id, at: at})
	}
	sort.Slice(parsedRows, func(i, j int) bool {
		if parsedRows[i].at != parsedRows[j].at {
			return parsedRows[i].at > parsedRows[j].at
		}
		return parsedRows[i].id > parsedRows[j].id
	})
	buckets := map[string]struct{}{}
	var prune []int64
	for _, row := range parsedRows {
		age := nowMs - row.at
		if age <= historyKeepAll {
			continue
		}
		var bucket string
		var keep bool
		switch {
		case age <= historyKeepHourly:
			bucket, keep = fmt.Sprintf("h%d", floorDiv(row.at, historyHourMillis)), true
		case age <= historyKeepDaily:
			bucket, keep = fmt.Sprintf("d%d", floorDiv(row.at, historyDayMillis)), true
		}
		if keep {
			if _, seen := buckets[bucket]; !seen {
				buckets[bucket] = struct{}{}
				continue
			}
		}
		prune = append(prune, row.id)
	}
	return prune
}

// floorDiv is Math.floor(a / b) for the bucket computation in
// idsToPrune; Go's integer division truncates toward zero, which
// differs for negative timestamps.
func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// gunzip is the test/read counterpart of gzipBytes (cf.
// SqliteSaveHistory.get, which gunzips the stored blob). It is kept
// unexported: list/restore are NOT STARTED, and no production caller
// reads history rows back yet.
func gunzip(gz []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
