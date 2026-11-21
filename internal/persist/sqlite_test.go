package persist

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// openSQLiteTestStore opens a store on a fresh nested path under a temp
// dir (which also exercises parent-directory creation) and registers its
// Close with the test.
func openSQLiteTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(filepath.Join(t.TempDir(), "nested", "saves", "players.sqlite"))
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// sqliteHistoryEntry is one player_save_history row, read back directly
// for assertions.
type sqliteHistoryEntry struct {
	id      int64
	savedAt string
	reason  string
	hash    string
	gz      []byte
}

func sqliteHistory(t *testing.T, s *SQLiteStore, key string) []sqliteHistoryEntry {
	t.Helper()
	rows, err := s.db.Query(
		"SELECT id, saved_at, reason, save_hash, save_gz FROM player_save_history WHERE username = ? ORDER BY id", key)
	if err != nil {
		t.Fatalf("query history: %v", err)
	}
	defer rows.Close()
	var out []sqliteHistoryEntry
	for rows.Next() {
		var e sqliteHistoryEntry
		if err := rows.Scan(&e.id, &e.savedAt, &e.reason, &e.hash, &e.gz); err != nil {
			t.Fatalf("scan history: %v", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("history rows: %v", err)
	}
	return out
}

func sqliteRow(t *testing.T, s *SQLiteStore, key string) (saveJSON, updatedAt string, found bool) {
	t.Helper()
	err := s.db.QueryRow("SELECT save_json, updated_at FROM player_saves WHERE username = ?", key).Scan(&saveJSON, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false
	}
	if err != nil {
		t.Fatalf("query row: %v", err)
	}
	return saveJSON, updatedAt, true
}

func TestSQLiteDriverRegistered(t *testing.T) {
	// The blank driver import in sqlite.go must register "sqlite"; if it
	// is ever dropped, every test below fails at Open instead. The
	// no-CGO requirement itself is verified by building with
	// CGO_ENABLED=0 (see the phase report), not by this test.
	for _, name := range sql.Drivers() {
		if name == "sqlite" {
			return
		}
	}
	t.Fatalf("sqlite driver not registered; drivers = %v", sql.Drivers())
}

func TestSQLiteStoreSchemaAndWAL(t *testing.T) {
	store := openSQLiteTestStore(t)
	var mode string
	if err := store.db.QueryRow("PRAGMA journal_mode").Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	for _, want := range []struct{ kind, name string }{
		{"table", "player_saves"},
		{"table", "player_save_history"},
		{"index", "player_save_history_by_player"},
	} {
		var name string
		err := store.db.QueryRow(
			"SELECT name FROM sqlite_master WHERE type = ? AND name = ?", want.kind, want.name).Scan(&name)
		if err != nil {
			t.Fatalf("schema object %s %q missing: %v", want.kind, want.name, err)
		}
	}
	// Parent directories were created and the file exists.
	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("database file missing: %v", err)
	}
	// An empty path is refused, never a silent temp database.
	if _, err := NewSQLiteStore(""); err == nil {
		t.Fatal("empty path must be refused")
	}
}

func TestSQLiteStoreRoundTrip(t *testing.T) {
	store := openSQLiteTestStore(t)
	save := validTestSave("Round Trip")
	save.Extra = map[string]json.RawMessage{
		"barrowsCrypt": json.RawMessage(`3`),
		"customThing":  json.RawMessage(`{"kept":true}`),
	}
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !store.Exists("Round Trip", ProfileMain) || store.Exists("Round Trip", ProfileLeague) {
		t.Fatal("Exists wrong")
	}
	loaded, err := store.Load("round_trip", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Profile != ProfileMain || loaded.DisplayName != "Round Trip" {
		t.Fatalf("loaded identity wrong: %+v", loaded)
	}
	// JSON comparison: the loaded save re-marshals byte-identically to
	// the saved one, including Extra/unknown keys.
	want := mustMarshalSave(t, save)
	got := mustMarshalSave(t, loaded)
	if string(want) != string(got) {
		t.Fatalf("round-trip JSON differs:\nwant %s\n got %s", want, got)
	}
	// Overwriting replaces the row.
	save.RunEnergy = 42.5
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	loaded, _ = store.Load("Round Trip", ProfileMain)
	if loaded.RunEnergy != 42.5 {
		t.Fatal("overwrite did not take effect")
	}
	// Missing saves report ErrSaveNotFound.
	if _, err := store.Load("Ghost", ProfileMain); !errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("missing load: err = %v, want ErrSaveNotFound", err)
	}
	if store.Exists("Ghost", ProfileMain) {
		t.Fatal("Exists must be false for a missing save")
	}
}

func TestSQLiteStoreKeyParity(t *testing.T) {
	store := openSQLiteTestStore(t)
	for _, profile := range Profiles {
		save := validTestSave("Key Player")
		save.Profile = profile
		if err := store.Save(save, "logout"); err != nil {
			t.Fatalf("Save %s: %v", profile, err)
		}
	}
	// Row keys are exactly the StorageKey values.
	for _, want := range []string{"key_player", "league:key_player", "hardcore:key_player"} {
		if _, _, found := sqliteRow(t, store, want); !found {
			t.Fatalf("no row under key %q", want)
		}
	}
	// Loads are profile-distinct and carry the requested profile.
	for _, profile := range Profiles {
		loaded, err := store.Load("KEY PLAYER", profile)
		if err != nil || loaded.Profile != profile {
			t.Fatalf("Load %s: %+v, %v", profile, loaded, err)
		}
	}
	// Unknown profiles: Load errors, Exists is false, Save/Delete error.
	if _, err := store.Load("Key Player", "bogus"); err == nil {
		t.Fatal("Load with unknown profile must error")
	}
	if store.Exists("Key Player", "bogus") {
		t.Fatal("Exists with unknown profile must be false")
	}
	bad := validTestSave("Key Player")
	bad.Profile = "bogus"
	if err := store.Save(bad, "logout"); err == nil {
		t.Fatal("Save with unknown profile must error")
	}
	if err := store.Delete("Key Player", "bogus"); err == nil {
		t.Fatal("Delete with unknown profile must error")
	}
}

func TestSQLiteStoreJSONShapeParity(t *testing.T) {
	dir := t.TempDir()
	jsonStore := NewJSONFileStore(dir)
	save := validTestSave("Shape Player")
	save.Extra = map[string]json.RawMessage{"barrowsCrypt": json.RawMessage(`3`)}
	if err := jsonStore.Save(save, "logout"); err != nil {
		t.Fatalf("JSONFileStore.Save: %v", err)
	}
	fileBytes, err := os.ReadFile(filepath.Join(dir, "shape_player.json"))
	if err != nil {
		t.Fatalf("read JSON save file: %v", err)
	}

	store := openSQLiteTestStore(t)
	// The bytes a JSONFileStore wrote, placed in a SQLite row under the
	// storage key, must load identically through the SQLite store.
	key := StorageKey("Shape Player", ProfileMain)
	if _, err := store.db.Exec(
		"INSERT INTO player_saves (username, save_json, updated_at) VALUES (?, ?, ?)",
		key, string(fileBytes), "2026-11-14T00:00:00.000Z"); err != nil {
		t.Fatalf("insert JSON-shaped row: %v", err)
	}
	fromSQLite, err := store.Load("Shape Player", ProfileMain)
	if err != nil {
		t.Fatalf("SQLite Load: %v", err)
	}
	fromJSON, err := jsonStore.Load("Shape Player", ProfileMain)
	if err != nil {
		t.Fatalf("JSONFileStore Load: %v", err)
	}
	if string(mustMarshalSave(t, fromSQLite)) != string(mustMarshalSave(t, fromJSON)) {
		t.Fatal("same JSON bytes must load identically from both backends")
	}
	// And a save written by the SQLite store stores byte-identical JSON
	// to the JSON file store's, because both use marshalSaveJSON.
	save2 := validTestSave("Shape Player 2")
	if err := store.Save(save2, "logout"); err != nil {
		t.Fatalf("SQLite Save: %v", err)
	}
	rowJSON, _, found := sqliteRow(t, store, StorageKey("Shape Player 2", ProfileMain))
	if !found {
		t.Fatal("SQLite row missing")
	}
	if err := jsonStore.Save(save2, "logout"); err != nil {
		t.Fatalf("JSONFileStore.Save 2: %v", err)
	}
	fileBytes2, err := os.ReadFile(filepath.Join(dir, "shape_player_2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if rowJSON != string(fileBytes2) {
		t.Fatal("SQLite row JSON must equal the JSON file store's bytes")
	}
}

func TestSQLiteStoreValidationParity(t *testing.T) {
	store := openSQLiteTestStore(t)
	jsonStore := NewJSONFileStore(t.TempDir())
	cases := map[string]func() *PlayerSave{
		"nil position": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Position = nil
			return s
		},
		"short inventory": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Inventory = s.Inventory[:10]
			return s
		},
		"short equipment": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Equipment = s.Equipment[:3]
			return s
		},
		"short skills.level": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Skills.Level = s.Skills.Level[:5]
			return s
		},
		"short skills.maxLevel": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Skills.MaxLevel = s.Skills.MaxLevel[:5]
			return s
		},
		"short skills.experience": func() *PlayerSave {
			s := validTestSave("Bad Save")
			s.Skills.Experience = s.Skills.Experience[:5]
			return s
		},
		"empty display name": func() *PlayerSave { return validTestSave("") },
	}
	for name, build := range cases {
		jsonErr := jsonStore.Save(build(), "logout")
		sqliteErr := store.Save(build(), "logout")
		if jsonErr == nil || sqliteErr == nil {
			t.Fatalf("%s: json err = %v, sqlite err = %v, both must refuse", name, jsonErr, sqliteErr)
		}
		if jsonErr.Error() != sqliteErr.Error() {
			t.Fatalf("%s: messages differ:\njson:   %v\nsqlite: %v", name, jsonErr, sqliteErr)
		}
	}
	if err := store.Save(nil, "logout"); err == nil || err.Error() != jsonStore.Save(nil, "logout").Error() {
		t.Fatalf("nil save: sqlite err = %v", err)
	}
	// Nothing was written: no rows, no history.
	var rows, copies int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM player_saves").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow("SELECT COUNT(*) FROM player_save_history").Scan(&copies); err != nil {
		t.Fatal(err)
	}
	if rows != 0 || copies != 0 {
		t.Fatalf("refused saves wrote data: %d rows, %d history copies", rows, copies)
	}
}

func TestSQLiteStoreHistory(t *testing.T) {
	store := openSQLiteTestStore(t)
	key := StorageKey("History Player", ProfileMain)
	save := validTestSave("History Player")
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	entries := sqliteHistory(t, store, key)
	if len(entries) != 1 {
		t.Fatalf("history copies = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.reason != "logout" {
		t.Fatalf("history reason = %q, want logout", e.reason)
	}
	// Hash is the SHA-1 hex of the serialized bytes; the blob gunzips to
	// exactly the stored row JSON.
	rowJSON, updatedAt, _ := sqliteRow(t, store, key)
	if e.hash != sha1Hex([]byte(rowJSON)) {
		t.Fatalf("history hash = %q, want SHA-1 of row JSON", e.hash)
	}
	plain, err := gunzip(e.gz)
	if err != nil {
		t.Fatalf("gunzip history copy: %v", err)
	}
	if string(plain) != rowJSON {
		t.Fatal("history copy does not decompress to the stored save JSON")
	}
	// updated_at and the copy's saved_at share one timestamp, in
	// toISOString (millis + Z) form.
	if e.savedAt != updatedAt {
		t.Fatalf("saved_at %q != updated_at %q", e.savedAt, updatedAt)
	}
	if matched, _ := regexp.MatchString(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`, updatedAt); !matched {
		t.Fatalf("updated_at %q is not ISO millis form", updatedAt)
	}
	if _, err := time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		t.Fatalf("updated_at %q does not parse: %v", updatedAt, err)
	}

	// An identical re-save adds no copy (hash dedupe vs the latest).
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("identical re-save: %v", err)
	}
	if got := len(sqliteHistory(t, store, key)); got != 1 {
		t.Fatalf("identical re-save: %d copies, want 1", got)
	}
	// A different save adds one, with its own reason.
	save.RunEnergy = 55
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("changed save: %v", err)
	}
	entries = sqliteHistory(t, store, key)
	if len(entries) != 2 {
		t.Fatalf("changed save: %d copies, want 2", len(entries))
	}
	if entries[1].reason != "autosave" {
		t.Fatalf("second copy reason = %q, want autosave", entries[1].reason)
	}
	// Saving the first content again adds a third copy: dedupe compares
	// only against the latest, not all history.
	save.RunEnergy = 100
	if err := store.Save(save, "shutdown"); err != nil {
		t.Fatalf("revert save: %v", err)
	}
	if got := len(sqliteHistory(t, store, key)); got != 3 {
		t.Fatalf("revert save: %d copies, want 3", got)
	}
}

func TestSQLiteStoreHistoryPruning(t *testing.T) {
	store := openSQLiteTestStore(t)
	key := StorageKey("Prune Player", ProfileMain)
	save := validTestSave("Prune Player")
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	gz, err := gzipBytes([]byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	// Seed old copies directly, with saved_at values bucketed safely
	// inside the retention windows relative to now (the Save below
	// prunes against its own timestamp, i.e. ~now).
	now := time.Now().UTC()
	hourStart := now.Truncate(time.Hour)
	dayStart := now.Truncate(24 * time.Hour)
	seed := []struct {
		at   time.Time
		hash string
	}{
		{now.Add(-23 * time.Hour), "keep-recent"},                        // < 24h: kept
		{hourStart.Add(-25 * time.Hour), "prune-hour-old"},               // same hour bucket as next, older: pruned
		{hourStart.Add(-25*time.Hour + 30*time.Minute), "keep-hour-new"}, // same bucket, newer: kept
		{dayStart.Add(-8 * 24 * time.Hour), "prune-day-old"},             // same day bucket as next, older: pruned
		{dayStart.Add(-8*24*time.Hour + 12*time.Hour), "keep-day-new"},   // same bucket, newer: kept
		{now.Add(-31 * 24 * time.Hour), "prune-ancient"},                 // > 30d: pruned
	}
	for _, s := range seed {
		if _, err := store.db.Exec(
			"INSERT INTO player_save_history (username, saved_at, reason, save_hash, save_gz) VALUES (?, ?, ?, ?, ?)",
			key, s.at.Format(isoMillis), "seed", s.hash, gz); err != nil {
			t.Fatalf("seed history: %v", err)
		}
	}
	// A changed save triggers pruning.
	save.RunEnergy = 77
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	var hashes []string
	for _, e := range sqliteHistory(t, store, key) {
		hashes = append(hashes, e.hash)
	}
	// Survivors: the two real copies (hashes are SHA-1 hex, not seed
	// labels) plus keep-recent, keep-hour-new, keep-day-new.
	if len(hashes) != 5 {
		t.Fatalf("surviving copies = %d (%v), want 5", len(hashes), hashes)
	}
	for _, want := range []string{"keep-recent", "keep-hour-new", "keep-day-new"} {
		found := false
		for _, h := range hashes {
			if h == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("seed copy %q was pruned, want kept; survivors %v", want, hashes)
		}
	}
	for _, gone := range []string{"prune-hour-old", "prune-day-old", "prune-ancient"} {
		for _, h := range hashes {
			if h == gone {
				t.Fatalf("seed copy %q survived, want pruned; survivors %v", gone, hashes)
			}
		}
	}
}

func TestIdsToPrune(t *testing.T) {
	// Fixed clock: 2026-11-14T12:00:00.000Z.
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC).UnixMilli()
	at := func(s string) string { return s }
	rows := []historyRow{
		{id: 1, savedAt: at("2026-11-14T11:00:00.000Z")},  // 1h ago: keep-all
		{id: 2, savedAt: at("2026-11-13T12:00:00.000Z")},  // exactly 24h: keep-all (age <= 24h)
		{id: 3, savedAt: at("2026-11-13T11:00:00.000Z")},  // 25h, hour bucket A, older
		{id: 4, savedAt: at("2026-11-13T11:30:00.000Z")},  // 24.5h, hour bucket A, newer: keep
		{id: 5, savedAt: at("2026-11-13T10:00:00.000Z")},  // 26h, hour bucket B: keep (only one)
		{id: 6, savedAt: at("2026-11-06T12:00:00.000Z")},  // 8d, day bucket C, older
		{id: 7, savedAt: at("2026-11-06T18:00:00.000Z")},  // 7d18h, day bucket C, newer: keep
		{id: 8, savedAt: at("2026-10-15T12:00:00.000Z")},  // 30d: daily window, keep (only one)
		{id: 9, savedAt: at("2026-10-14T11:00:00.000Z")},  // > 30d: prune
		{id: 10, savedAt: at("2026-11-14T13:00:00.000Z")}, // future: keep (negative age)
		{id: 11, savedAt: at("2026-11-13T11:30:00.000Z")}, // same instant as id 4, higher id: kept first, so 4 pruned
	}
	// Note id 11 shares id 4's instant and bucket; newest-first order
	// puts 11 before 4, so 11 keeps the bucket and 4 is pruned.
	got := idsToPrune(rows, now)
	want := map[int64]bool{3: true, 4: true, 6: true, 9: true}
	if len(got) != len(want) {
		t.Fatalf("idsToPrune = %v, want %v", got, want)
	}
	for _, id := range got {
		if !want[id] {
			t.Fatalf("idsToPrune = %v, want %v", got, want)
		}
	}
	// Order-independence: shuffled input prunes the same ids.
	shuffled := []historyRow{rows[8], rows[0], rows[5], rows[10], rows[2], rows[4], rows[9], rows[1], rows[7], rows[3], rows[6]}
	got2 := idsToPrune(shuffled, now)
	if len(got2) != len(want) {
		t.Fatalf("shuffled idsToPrune = %v, want %v", got2, want)
	}
	for _, id := range got2 {
		if !want[id] {
			t.Fatalf("shuffled idsToPrune = %v, want %v", got2, want)
		}
	}
	if got := idsToPrune(nil, now); len(got) != 0 {
		t.Fatalf("empty input pruned %v", got)
	}
}

func TestSQLiteStoreDelete(t *testing.T) {
	store := openSQLiteTestStore(t)
	key := StorageKey("Delete Player", ProfileMain)
	// Deleting a missing save is not an error and writes nothing.
	if err := store.Delete("Delete Player", ProfileMain); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
	if got := len(sqliteHistory(t, store, key)); got != 0 {
		t.Fatalf("delete of missing save wrote %d history copies", got)
	}

	save := validTestSave("Delete Player")
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	save.RunEnergy = 33
	if err := store.Save(save, "autosave"); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	rowJSON, _, _ := sqliteRow(t, store, key)
	if err := store.Delete("Delete Player", ProfileMain); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if store.Exists("Delete Player", ProfileMain) {
		t.Fatal("deleted save still exists")
	}
	if _, err := store.Load("Delete Player", ProfileMain); !errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("deleted load: %v, want ErrSaveNotFound", err)
	}
	// No history remains under the original key: it was re-keyed under
	// `deleted:<millis>:<key>`. The character-delete copy dedupes
	// against the latest copy (identical bytes), so exactly the two
	// save copies moved.
	if got := len(sqliteHistory(t, store, key)); got != 0 {
		t.Fatalf("history under original key after delete: %d", got)
	}
	var moved []sqliteHistoryEntry
	rows, err := store.db.Query(
		"SELECT id, saved_at, reason, save_hash, save_gz FROM player_save_history WHERE username LIKE 'deleted:%' ORDER BY id")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var movedKey string
	for rows.Next() {
		var e sqliteHistoryEntry
		if err := rows.Scan(&e.id, &e.savedAt, &e.reason, &e.hash, &e.gz); err != nil {
			t.Fatal(err)
		}
		moved = append(moved, e)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(
		"SELECT DISTINCT username FROM player_save_history WHERE username LIKE 'deleted:%'").Scan(&movedKey); err != nil {
		t.Fatalf("no re-keyed history: %v", err)
	}
	if !strings.HasPrefix(movedKey, "deleted:") || !strings.HasSuffix(movedKey, ":"+key) {
		t.Fatalf("re-keyed history key = %q, want deleted:<millis>:%s", movedKey, key)
	}
	if len(moved) != 2 {
		t.Fatalf("re-keyed copies = %d, want 2", len(moved))
	}
	// The newest moved copy decompresses to the deleted row's JSON.
	plain, err := gunzip(moved[1].gz)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != rowJSON {
		t.Fatal("newest moved copy is not the deleted save")
	}

	// When the row differs from the latest history copy, Delete adds a
	// "character-delete" copy before re-keying.
	save2 := validTestSave("Delete Player 2")
	key2 := StorageKey("Delete Player 2", ProfileMain)
	if err := store.Save(save2, "logout"); err != nil {
		t.Fatalf("Save 2: %v", err)
	}
	// Rewrite the row directly with different JSON, bypassing history.
	other := validTestSave("Delete Player 2")
	other.RunEnergy = 11
	otherJSON, err := marshalSaveJSON(other)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec("UPDATE player_saves SET save_json = ? WHERE username = ?", string(otherJSON), key2); err != nil {
		t.Fatal(err)
	}
	if err := store.Delete("Delete Player 2", ProfileMain); err != nil {
		t.Fatalf("Delete 2: %v", err)
	}
	var reasons []string
	var gzLast []byte
	rrows, err := store.db.Query(
		"SELECT reason, save_gz FROM player_save_history WHERE username LIKE 'deleted:%:' || ? ORDER BY id", key2)
	if err != nil {
		t.Fatal(err)
	}
	defer rrows.Close()
	for rrows.Next() {
		var reason string
		var gz []byte
		if err := rrows.Scan(&reason, &gz); err != nil {
			t.Fatal(err)
		}
		reasons = append(reasons, reason)
		gzLast = gz
	}
	if err := rrows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(reasons) != 2 || reasons[1] != "character-delete" {
		t.Fatalf("reasons after delete = %v, want [logout character-delete]", reasons)
	}
	plain, err = gunzip(gzLast)
	if err != nil {
		t.Fatal(err)
	}
	if string(plain) != string(otherJSON) {
		t.Fatal("character-delete copy is not the deleted row's JSON")
	}

	// The name starts clean: a fresh save works and histories under the
	// plain key again.
	fresh := validTestSave("Delete Player")
	if err := store.Save(fresh, "logout"); err != nil {
		t.Fatalf("re-save after delete: %v", err)
	}
	if !store.Exists("Delete Player", ProfileMain) {
		t.Fatal("re-saved character must exist")
	}
	if got := len(sqliteHistory(t, store, key)); got != 1 {
		t.Fatalf("fresh history copies = %d, want 1", got)
	}
}

func TestSQLiteStoreCorruptRow(t *testing.T) {
	store := openSQLiteTestStore(t)
	key := StorageKey("Broken", ProfileMain)
	if _, err := store.db.Exec(
		"INSERT INTO player_saves (username, save_json, updated_at) VALUES (?, ?, ?)",
		key, "{oops", "2026-11-14T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	_, err := store.Load("Broken", ProfileMain)
	if err == nil || errors.Is(err, ErrSaveNotFound) {
		t.Fatalf("corrupt row must error (not ErrSaveNotFound): %v", err)
	}
	if !strings.Contains(err.Error(), key) {
		t.Fatalf("corrupt-row error must name the key %q: %v", key, err)
	}
	// The row still exists; only parsing fails.
	if !store.Exists("Broken", ProfileMain) {
		t.Fatal("corrupt row must still exist")
	}
}

func TestSQLiteStoreReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "players.sqlite")
	store, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	save := validTestSave("Reopen Player")
	save.RunEnergy = 61
	if err := store.Save(save, "logout"); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("second Close must be idempotent: %v", err)
	}
	// Operations after Close fail.
	if _, err := store.Load("Reopen Player", ProfileMain); err == nil {
		t.Fatal("Load after Close must fail")
	}
	if err := store.Save(save, "logout"); err == nil {
		t.Fatal("Save after Close must fail")
	}

	reopened, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	loaded, err := reopened.Load("Reopen Player", ProfileMain)
	if err != nil {
		t.Fatalf("Load after reopen: %v", err)
	}
	if loaded.RunEnergy != 61 {
		t.Fatalf("RunEnergy after reopen = %v, want 61", loaded.RunEnergy)
	}
	if got := len(sqliteHistory(t, reopened, StorageKey("Reopen Player", ProfileMain))); got != 1 {
		t.Fatalf("history after reopen = %d copies, want 1", got)
	}
}

func TestSQLiteStoreConcurrent(t *testing.T) {
	store := openSQLiteTestStore(t)
	names := []string{"Alpha", "Beta", "Gamma", "Delta"}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 20; i++ {
				name := names[(g+i)%len(names)]
				save := validTestSave(name)
				save.RunEnergy = float64(g*20 + i)
				if err := store.Save(save, "autosave"); err != nil {
					t.Errorf("concurrent Save: %v", err)
					return
				}
				if _, err := store.Load(name, ProfileMain); err != nil {
					t.Errorf("concurrent Load: %v", err)
					return
				}
				_ = store.Exists(name, ProfileMain)
			}
		}(g)
	}
	wg.Wait()
	for _, name := range names {
		if !store.Exists(name, ProfileMain) {
			t.Fatalf("%s missing after concurrent use", name)
		}
	}
}

func TestSQLiteStoreLoadNormalization(t *testing.T) {
	// The TS fixture, stored as a row, loads with the same normalization
	// JSONFileStore applies (padded containers, merged skills, deduped
	// friends/flags, Extra preserved).
	store := openSQLiteTestStore(t)
	key := StorageKey("Fixture Player", ProfileMain)
	if _, err := store.db.Exec(
		"INSERT INTO player_saves (username, save_json, updated_at) VALUES (?, ?, ?)",
		key, tsSaveFixture, "2026-11-14T00:00:00.000Z"); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load("Fixture Player", ProfileMain)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(loaded.Inventory) != InventorySize || len(loaded.Equipment) != EquipmentSize {
		t.Fatalf("containers not padded: %d/%d", len(loaded.Inventory), len(loaded.Equipment))
	}
	if len(loaded.Skills.Level) != SkillCount || loaded.Skills.Level[4] != 1 {
		t.Fatalf("skills not merged: %+v", loaded.Skills)
	}
	if len(loaded.Friends) != 2 || loaded.Friends[1] != "67890" {
		t.Fatalf("friends wrong: %v", loaded.Friends)
	}
	if len(loaded.Flags) != 2 || loaded.Flags[0] != "alpha" {
		t.Fatalf("flags wrong: %v", loaded.Flags)
	}
	if _, ok := loaded.Extra["barrowsCrypt"]; !ok {
		t.Fatal("Extra lost on load")
	}
}
