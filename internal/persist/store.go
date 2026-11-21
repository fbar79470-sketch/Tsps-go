package persist

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Save backends: the SaveStore interface, an in-memory store, and the
// JSON file store (port of plugins/persistence/JsonPlayerPersistence
// .plugin.js). The SQLite backend is NOT implemented (see doc.go).

// ErrSaveNotFound is returned by Load when no save exists for the
// username/profile. TS load() returns null in that case; Go callers
// should check errors.Is(err, ErrSaveNotFound) or use Exists first.
var ErrSaveNotFound = errors.New("persist: no save for this username and profile")

// SaveStore persists PlayerSave snapshots, keyed by username and
// profile. Load returns ErrSaveNotFound (with a nil save) when no save
// exists. Save's reason ("logout", "autosave", "shutdown", ...) is
// recorded only by backends with a save history (SQLite); the JSON and
// memory backends accept and ignore it, as the TS JSON backend ignores
// its _reason argument.
type SaveStore interface {
	Load(username string, profile Profile) (*PlayerSave, error)
	Save(save *PlayerSave, reason string) error
	Exists(username string, profile Profile) bool
}

// MemoryStore is an in-memory SaveStore, for tests and headless use.
// Saves are deep-copied through JSON on both Save and Load, so mutating
// a returned save never affects the stored copy. Unlike JSONFileStore,
// it does not validate container/skill lengths: partial saves are
// allowed, as befits a test double. It is safe for concurrent use.
type MemoryStore struct {
	mu    sync.Mutex
	saves map[string][]byte // storage key -> serialized save
}

// NewMemoryStore returns an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{saves: map[string][]byte{}}
}

var _ SaveStore = (*MemoryStore)(nil)

// Load returns a copy of the stored save, or ErrSaveNotFound.
func (m *MemoryStore) Load(username string, profile Profile) (*PlayerSave, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, ok := m.saves[StorageKey(username, profile)]
	if !ok {
		return nil, ErrSaveNotFound
	}
	var save PlayerSave
	if err := json.Unmarshal(data, &save); err != nil {
		return nil, fmt.Errorf("persist: decoding stored save for %q: %w", username, err)
	}
	save.Profile = profile
	return &save, nil
}

// Save stores a copy of save. The username comes from
// save.DisplayName and the profile from save.Profile ("" means main).
// A nil save, an empty display name, or an unknown profile is an error.
func (m *MemoryStore) Save(save *PlayerSave, reason string) error {
	if save == nil {
		return errors.New("persist: cannot save a nil PlayerSave")
	}
	if save.DisplayName == "" {
		return errors.New("persist: cannot save a PlayerSave with an empty displayName")
	}
	profile := save.EffectiveProfile()
	if !profile.Valid() {
		return fmt.Errorf("persist: unknown profile %q", string(profile))
	}
	data, err := json.Marshal(save)
	if err != nil {
		return fmt.Errorf("persist: encoding save for %q: %w", save.DisplayName, err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves[StorageKey(save.DisplayName, profile)] = data
	return nil
}

// Exists reports whether a save is stored for the username/profile.
func (m *MemoryStore) Exists(username string, profile Profile) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.saves[StorageKey(username, profile)]
	return ok
}

// Delete removes a stored save, if any.
func (m *MemoryStore) Delete(username string, profile Profile) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.saves, StorageKey(username, profile))
}

// JSONFileStore is a SaveStore keeping one JSON file per character under
// a directory: the main profile's file sits in the directory itself
// (<normalized>.json); another profile's sits in a subdirectory named
// after it (league/<normalized>.json). Port of
// JsonPlayerPersistence.plugin.js (SAVE_DIRECTORY =
// data/saves/characters; resolveFilePath).
//
// Writes are atomic with fsync, exactly as in the plugin's
// writeFileAtomicallyAsync: write a temp file in the same directory,
// fsync it, rename over the target, fsync the directory (best-effort).
//
// DIVERGENCE (synchronous writes): the TS backend serializes and
// enqueues writes on an async chain, keeping pending writes in memory
// so a load during a queued write sees the newest data, and surfacing
// failures via flush(). This port writes synchronously in Save and
// returns failures immediately; there is no pending-write cache and no
// Flush method. Callers that need TS's "load sees the newest write"
// guarantee get it trivially, because Save does not return until the
// file is durable.
//
// DIVERGENCE (temp cleanup): on a failed write this port removes the
// temp file; TS leaves it behind (its finally block only closes the
// descriptor). Successful writes never leave temp files in either.
type JSONFileStore struct {
	mu  sync.Mutex
	dir string
}

// NewJSONFileStore returns a store rooted at dir (e.g.
// data/saves/characters). The directory is created on first write.
func NewJSONFileStore(dir string) *JSONFileStore {
	return &JSONFileStore{dir: dir}
}

var _ SaveStore = (*JSONFileStore)(nil)

// Dir returns the store's root directory.
func (s *JSONFileStore) Dir() string { return s.dir }

// FilePath returns the file a username/profile maps to, or an error for
// an unknown profile. Exported so tooling and the SQLite import path
// can locate legacy JSON saves.
func (s *JSONFileStore) FilePath(username string, profile Profile) (string, error) {
	if !profile.Valid() {
		return "", fmt.Errorf("persist: unknown profile %q", string(profile))
	}
	normalized := NormalizeUsername(username)
	if profile == ProfileMain {
		return filepath.Join(s.dir, normalized+".json"), nil
	}
	return filepath.Join(s.dir, string(profile), normalized+".json"), nil
}

// Load reads and parses a save file, applying the load-time
// normalization in normalizeLoadedSave. A missing file yields
// ErrSaveNotFound; an unreadable or malformed file yields an error
// naming the file. (TS load() would throw on a malformed file too:
// JSON.parse throws.)
func (s *JSONFileStore) Load(username string, profile Profile) (*PlayerSave, error) {
	path, err := s.FilePath(username, profile)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrSaveNotFound
		}
		return nil, fmt.Errorf("persist: reading save %s: %w", path, err)
	}
	var save PlayerSave
	// The reviver equivalent lives in the field types: Banks accepts
	// the {"__type": "Map", "entries": [...]} shape the replacer wrote,
	// and Item/Rights/DonatorRights accept their legacy shapes.
	if err := json.Unmarshal(data, &save); err != nil {
		return nil, fmt.Errorf("persist: parsing save %s: %w", path, err)
	}
	normalizeLoadedSave(&save, profile)
	return &save, nil
}

// Save validates and writes save atomically. The username comes from
// save.DisplayName and the profile from save.Profile ("" means main).
// reason is accepted and ignored (see SaveStore). Validation mirrors
// validateSerializedSave in the JSON and SQLite plugins: a missing
// position, a wrongly-sized inventory/equipment, or wrongly-sized
// skill arrays are refused with an explicit error and nothing is
// written.
func (s *JSONFileStore) Save(save *PlayerSave, reason string) error {
	if save == nil {
		return errors.New("persist: cannot save a nil PlayerSave")
	}
	profile := save.EffectiveProfile()
	path, err := s.FilePath(save.DisplayName, profile)
	if err != nil {
		return err
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("persist: creating save directory for %s: %w", path, err)
	}
	if err := writeFileAtomically(path, data); err != nil {
		return fmt.Errorf("persist: writing save %s: %w", path, err)
	}
	return nil
}

// Exists reports whether the save file exists on disk. An unknown
// profile reports false.
func (s *JSONFileStore) Exists(username string, profile Profile) bool {
	path, err := s.FilePath(username, profile)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Delete removes a save file, if it exists (cf. the plugin's delete()).
// Removing a missing file is not an error.
func (s *JSONFileStore) Delete(username string, profile Profile) error {
	path, err := s.FilePath(username, profile)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("persist: deleting save %s: %w", path, err)
	}
	return nil
}

// marshalSaveJSON renders a save as TS does: JSON.stringify(save,
// replacer, 2) — two-space indent, no HTML escaping (TS does not escape
// <, >, &), no trailing newline.
func marshalSaveJSON(save *PlayerSave) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(save); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}

// validateSaveForWrite mirrors validateSerializedSave
// (JsonPlayerPersistence.plugin.js validateSerializedSave and
// ValidateSave.SqlitePlayerPersistence.js): the root must be an object
// (guaranteed by the struct), position must be present, inventory must
// have 28 items, equipment 14, and each skill array must have
// SkillCount entries. Go ints are always finite, so no finiteness check
// is needed. Error messages mirror the TS wording.
func validateSaveForWrite(save *PlayerSave) error {
	name := save.DisplayName
	if save.Position == nil {
		return fmt.Errorf("Refusing to save %s: position must be an object", name)
	}
	if len(save.Inventory) != InventorySize {
		return fmt.Errorf("Refusing to save %s: inventory must be an array of length %d", name, InventorySize)
	}
	if len(save.Equipment) != EquipmentSize {
		return fmt.Errorf("Refusing to save %s: equipment must be an array of length %d", name, EquipmentSize)
	}
	if len(save.Skills.Level) != SkillCount {
		return fmt.Errorf("Refusing to save %s: skills.level must be an array of length %d", name, SkillCount)
	}
	if len(save.Skills.MaxLevel) != SkillCount {
		return fmt.Errorf("Refusing to save %s: skills.maxLevel must be an array of length %d", name, SkillCount)
	}
	if len(save.Skills.Experience) != SkillCount {
		return fmt.Errorf("Refusing to save %s: skills.experience must be an array of length %d", name, SkillCount)
	}
	return nil
}

// normalizeLoadedSave applies the load-time hydration subset that is
// grounded in JsonPlayerPersistence.hydratePlayerSave and needs no game
// data:
//   - Attributes/Banks default to empty (never nil) when absent.
//   - A missing position defaults to the Lumbridge spawn
//     (hydrateLocation defaults 3089/3524/0). NOTE: TS defaults each
//     axis independently when that axis is missing/non-numeric; this
//     port defaults only a wholly missing position. A save with a
//     partial position keeps its zero axes. Real saves always carry all
//     three axes.
//   - Inventory/equipment are padded with empty items or truncated to
//     28/14 (hydrateItems).
//   - Skill arrays are merged over the defaults, per index
//     (mergeSkillArray, non-legacy path): short arrays are padded with
//     default values, long ones truncated. Legacy button-indexed skill
//     arrays (isLegacyButtonIndexedSkillArray) ARE unmapped via the
//     static Skill button-id table (see skillButtonIDs): values are
//     read from the button index and written to the skill index, as in
//     JsonPlayerPersistence.mergeSkillArray. LIMIT: Go decodes JSON
//     null into []int as 0, so a zero at a button index is treated as
//     absent and the fallback is kept (see
//     isLegacyButtonIndexedSkillArray); an explicit 0 at a button
//     index therefore loads as the fallback, observable only for a
//     hitpoints experience of exactly 0.
//   - Friends/ignores are normalized to positive unique stringified
//     bigints, capped at 200/100 (hydrateRelationArray).
//   - Flags are trimmed, de-duplicated, empties dropped (hydrateFlags).
//
// NOT applied (needs game data; the player package owns them):
//   - sanitizeAppearance (cache ident kits), rights/spellbook/skull
//     resolution beyond the numeric-form handling in the field types,
//     fightType resolution (FightType.resolve), quick-prayer resolution
//     by configId, legacy-field/attribute migration.
func normalizeLoadedSave(save *PlayerSave, profile Profile) {
	save.Profile = profile
	if save.Attributes == nil {
		save.Attributes = map[string]json.RawMessage{}
	}
	if save.Banks == nil {
		save.Banks = Banks{}
	}
	if save.Position == nil {
		save.Position = &Location{X: DefaultSpawnX, Y: DefaultSpawnY, Z: DefaultSpawnZ}
	}
	save.Inventory = fitItems(save.Inventory, InventorySize)
	save.Equipment = fitItems(save.Equipment, EquipmentSize)
	def := DefaultSkills()
	save.Skills.Level = mergeSkillArray(save.Skills.Level, def.Level)
	save.Skills.MaxLevel = mergeSkillArray(save.Skills.MaxLevel, def.MaxLevel)
	save.Skills.Experience = mergeSkillArray(save.Skills.Experience, def.Experience)
	save.Friends = normalizeRelationList(save.Friends, MaxFriends)
	save.Ignores = normalizeRelationList(save.Ignores, MaxIgnores)
	save.Flags = normalizeFlags(save.Flags)
}

func fitItems(items []Item, size int) []Item {
	if len(items) > size {
		return items[:size]
	}
	for len(items) < size {
		items = append(items, EmptyItem())
	}
	return items
}

func mergeSkillArray(raw, fallback []int) []int {
	out := append([]int(nil), fallback...)
	if raw == nil {
		return out
	}
	if isLegacyButtonIndexedSkillArray(raw) {
		for index, button := range skillButtonIDs {
			// A zero at a button index reads as absent (see
			// isLegacyButtonIndexedSkillArray): the fallback is kept.
			if button >= 0 && button < len(raw) && raw[button] != 0 {
				out[index] = raw[button]
			}
		}
		return out
	}
	for i := 0; i < len(raw) && i < len(out); i++ {
		out[i] = raw[i]
	}
	return out
}

// skillButtonIDs is Skill.getButton() per skill-array index, in Skill
// VALUES order (model/Skill.ts:8-31, 33-57). The table is static data,
// so the legacy unmapping lives here rather than in the game model.
// Sailing has no legacy button (-1): TS reads raw[-1] (undefined) and
// keeps the fallback, which the button >= 0 guard reproduces.
var skillButtonIDs = [SkillCount]int{
	8654, 8660, 8657, 8655, 8663, 8666, 8669, 8665,
	8671, 8670, 8662, 8668, 8667, 8659, 8656, 8661,
	8658, 8664, 12162, 13928, 8672, 18801, 18829, -1,
}

// isLegacyButtonIndexedSkillArray is isLegacyButtonIndexedSkillArray
// (JsonPlayerPersistence.plugin.js): an array longer than the skill
// count with a value at some skill's button index. TS tests
// raw[button] != null and its toNumber keeps the fallback for a null
// hole; Go's []int decode turns JSON null into 0, indistinguishable
// from an explicit 0, so this port treats a zero at a button index as
// absent, both here and in the unmapping. That reproduces TS for null
// holes (the overwhelmingly common case in a sparse array's JSON,
// which is mostly nulls) and differs only for an explicit 0 at a
// button index, which loads as the fallback instead of 0 — observable
// only for a hitpoints experience of exactly 0, which loads as the
// 1184 default. A legacy array whose button slots are all zero/null
// is not detected and loads via the truncation path; that shape
// carries no skill information either way (level/maxLevel values are
// never 0 in a real save).
func isLegacyButtonIndexedSkillArray(raw []int) bool {
	if len(raw) <= SkillCount {
		return false
	}
	for _, button := range skillButtonIDs {
		if button >= 0 && button < len(raw) && raw[button] != 0 {
			return true
		}
	}
	return false
}

// normalizeRelationList is hydrateRelationArray: values that parse as
// positive bigints, de-duplicated by canonical form, capped at max,
// order preserved.
func normalizeRelationList(values []string, max int) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, v := range values {
		n, ok := new(big.Int).SetString(strings.TrimSpace(v), 10)
		if !ok || n.Sign() <= 0 {
			continue
		}
		canonical := n.String()
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
		if len(out) >= max {
			break
		}
	}
	return out
}

func normalizeFlags(values []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	for _, v := range values {
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			continue
		}
		if _, dup := seen[trimmed]; dup {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

// writeFileAtomically is writeFileAtomicallyAsync
// (JsonPlayerPersistence.plugin.js): write a uniquely-named temp file
// in the target directory, fsync it (ignoring "not implemented"
// errors), close, rename over the target, fsync the directory
// (best-effort, all errors ignored).
func writeFileAtomically(filePath string, content []byte) error {
	directory := filepath.Dir(filePath)
	tempPath := filepath.Join(directory, fmt.Sprintf(".%s.%d.%d.%d.tmp",
		filepath.Base(filePath), os.Getpid(), time.Now().UnixMilli(), tempRandom()))
	f, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	writeErr := func() error {
		if _, err := f.Write(content); err != nil {
			return err
		}
		if err := fsyncFile(f); err != nil {
			return err
		}
		return nil
	}()
	closeErr := f.Close()
	if writeErr != nil {
		_ = os.Remove(tempPath)
		return writeErr
	}
	if closeErr != nil {
		_ = os.Remove(tempPath)
		return closeErr
	}
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	fsyncDirBestEffort(directory)
	return nil
}

func tempRandom() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return uint32(time.Now().UnixNano())
	}
	return binary.LittleEndian.Uint32(b[:]) % 1_000_000
}

// fsyncFile syncs a file, ignoring ENOSYS/ENOTSUP "not implemented"
// errors as isIgnorableFsyncError does in the plugin.
func fsyncFile(f *os.File) error {
	if err := f.Sync(); err != nil {
		if errors.Is(err, syscall.ENOSYS) || errors.Is(err, syscall.ENOTSUP) {
			return nil
		}
		return err
	}
	return nil
}

func fsyncDirBestEffort(directory string) {
	d, err := os.Open(directory)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
