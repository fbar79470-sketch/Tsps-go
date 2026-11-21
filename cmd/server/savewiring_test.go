package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// saveRecord is one observed store write: who, why, and the owned state
// the snapshot carried.
type saveRecord struct {
	username string
	reason   string
	position persist.Location
	running  bool
}

// recordingSaveStore wraps a MemoryStore, recording every Save attempt
// (including failed ones) with its reason — the memory backend itself
// ignores the reason, so tests that assert on it need the wrapper. It
// can fail writes globally, per username, or panic per username, to
// exercise the coordinator's failure isolation.
type recordingSaveStore struct {
	mu      sync.Mutex
	inner   *persist.MemoryStore
	records []saveRecord
	failAll error
	failFor map[string]error
	panicOn map[string]bool
}

func newRecordingSaveStore() *recordingSaveStore {
	return &recordingSaveStore{inner: persist.NewMemoryStore(), failFor: map[string]error{}, panicOn: map[string]bool{}}
}

func (s *recordingSaveStore) Load(username string, profile persist.Profile) (*persist.PlayerSave, error) {
	return s.inner.Load(username, profile)
}

func (s *recordingSaveStore) Save(save *persist.PlayerSave, reason string) error {
	s.mu.Lock()
	if s.panicOn[save.DisplayName] {
		s.mu.Unlock()
		panic("recordingSaveStore: injected panic for " + save.DisplayName)
	}
	rec := saveRecord{username: save.DisplayName, reason: reason, running: save.Running}
	if save.Position != nil {
		rec.position = *save.Position
	}
	s.records = append(s.records, rec)
	failAll, failFor := s.failAll, s.failFor[save.DisplayName]
	s.mu.Unlock()
	if failAll != nil {
		return failAll
	}
	if failFor != nil {
		return failFor
	}
	return s.inner.Save(save, reason)
}

func (s *recordingSaveStore) Exists(username string, profile persist.Profile) bool {
	return s.inner.Exists(username, profile)
}

func (s *recordingSaveStore) snapshot() []saveRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]saveRecord(nil), s.records...)
}

func (s *recordingSaveStore) reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records = nil
}

func (s *recordingSaveStore) usernames() []string {
	out := []string{}
	for _, r := range s.snapshot() {
		out = append(out, r.username)
	}
	return out
}

// manualHarness drives a world without the tick engine: the test
// goroutine IS the tick goroutine (it calls world.Process directly),
// so reading and writing player state from the test is race-free and
// every phase runs deterministically. AddPlayer still runs on a helper
// goroutine (as a network goroutine would) while the test drives ticks.
type manualHarness struct {
	world   *game.World
	adapter *worldAdapter
	coord   *saveCoordinator
	store   *recordingSaveStore
	logBuf  *bytes.Buffer
	spawn   game.Location
}

func newManualHarness(t *testing.T, interval, min int) *manualHarness {
	t.Helper()
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	logBuf := &bytes.Buffer{}
	coord := newSaveCoordinator(store, world, log.New(logBuf, "", 0), 10, interval, min)
	adapter := newWorldAdapter(world, spawn, time.Second)
	adapter.saves = coord
	world.Players().OnAdded = adapter.onPlayerAdded
	world.Hooks.OnPlayerDisconnect = adapter.onPlayerDisconnect
	return &manualHarness{world: world, adapter: adapter, coord: coord, store: store, logBuf: logBuf, spawn: spawn}
}

// enter runs AddPlayer on a helper goroutine while driving ticks until
// it returns, and returns the registered player.
func (m *manualHarness) enter(t *testing.T, ep *session.EnteringPlayer) (*game.Player, int) {
	t.Helper()
	type result struct {
		index int
		err   error
	}
	done := make(chan result, 1)
	go func() {
		index, _, err := m.adapter.AddPlayer(ep)
		done <- result{index, err}
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		select {
		case r := <-done:
			if r.err != nil {
				t.Fatalf("AddPlayer(%s): %v", ep.Username, r.err)
			}
			p, ok := m.world.Players().Get(r.index)
			if !ok {
				t.Fatalf("player %s not registered at index %d", ep.Username, r.index)
			}
			return p, r.index
		default:
			m.world.Process()
			if time.Now().After(deadline) {
				t.Fatal("AddPlayer never returned")
			}
		}
	}
}

// remove queues a removal and drives ticks until the world is empty of
// that player (or the deadline passes).
func (m *manualHarness) remove(t *testing.T, index int) {
	t.Helper()
	m.world.QueueRemovePlayer(index)
	deadline := time.Now().Add(2 * time.Second)
	for {
		m.world.Process()
		if _, ok := m.world.Players().Get(index); !ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("player at index %d never left the world", index)
		}
	}
}

// saveAtPosition builds a valid save (NewCharacterSave shape) at a tile
// with the given running flag and password hash.
func saveAtPosition(username string, x, y, z int, running bool) *persist.PlayerSave {
	s := persist.NewCharacterSave(persist.NewCharacterParams{
		Username:     username,
		PasswordHash: "old-hash",
		Profile:      persist.ProfileMain,
		Spawn:        persist.Location{X: x, Y: y, Z: z},
		Now:          time.Now(),
	})
	s.Running = running
	return s
}

func testTokenEntry(now time.Time) persist.TokenEntry {
	return persist.TokenEntry{Hash: strings.Repeat("a", 64), LastUsed: now}
}

func TestAutosaveOffsetOf(t *testing.T) {
	cases := []struct {
		name     string
		interval int
		want     int
	}{
		{"", 501, 0},
		{"a", 501, 97},
		{"ab", 501, 99},
		{"Alice", 501, 421},
		{"alice", 501, 105},
		{"Bob", 501, 332},
		{"Alicia", 501, 246},
		{"PlayerOne", 501, 227},
		{"Z", 1, 0},
		{"Alice", 1, 0},
		// Non-ASCII BMP: the rune is its UTF-16 code unit.
		{"é", 501, 233},
		// Astral: for...of yields the surrogate pair, charCodeAt(0)
		// is the high surrogate (0xD83D = 55357) only.
		{"\U0001F600", 501, 247},
		// Degenerate intervals never divide by zero.
		{"Alice", 0, 0},
		{"Alice", -3, 0},
	}
	for _, tc := range cases {
		if got := autosaveOffsetOf(tc.name, tc.interval); got != tc.want {
			t.Errorf("autosaveOffsetOf(%q, %d) = %d, want %d", tc.name, tc.interval, got, tc.want)
		}
	}
	// The hash wraps mod 2^32 (>>> 0): a long name stays in range.
	if got := autosaveOffsetOf("AliciaAliciaAlicia", 501); got < 0 || got >= 501 {
		t.Errorf("autosaveOffsetOf(long name) = %d, out of range", got)
	}
}

func TestAutosaveIntervalsFromRaw(t *testing.T) {
	cases := []struct {
		raw          string
		wantInterval int
		wantMin      int
	}{
		{"", 1000, 500},                      // unset/empty: the 500-1000 spread
		{"2000", 2000, 2000},                 // set: fixed interval
		{"0", 1000, 1000},                    // truthy-string quirk: "0" collapses the spread
		{"abc", 1000, 1000},                  // NaN -> default interval, truthy -> min = interval
		{"0.5", 0, 0},                        // > 0, floor -> 0 (degenerate, faithful)
		{"1e3", 1000, 1000},                  // exponent form
		{" 750 ", 750, 750},                  // surrounding whitespace ignored
		{"-5", 1000, 1000},                   // not > 0 -> default, truthy -> min = interval
		{"0x10", 16, 16},                     // hex literal
		{"0b101", 5, 5},                      // binary literal
		{"0o17", 15, 15},                     // octal literal
		{"1000.9", 1000, 1000},               // floor, not round
		{"NaN", 1000, 1000},                  // NaN spelling
		{"1_000", 1000, 1000},                // underscores: NaN in JS Number
		{"12abc", 1000, 1000},                // trailing garbage: NaN
		{"Infinity", 2147483647, 2147483647}, // clamped, not an int-conversion trap
	}
	for _, tc := range cases {
		interval, min := autosaveIntervalsFromRaw(tc.raw)
		if interval != tc.wantInterval || min != tc.wantMin {
			t.Errorf("autosaveIntervalsFromRaw(%q) = (%d, %d), want (%d, %d)", tc.raw, interval, min, tc.wantInterval, tc.wantMin)
		}
	}
}

func TestAutosaveIntervalsFromEnv(t *testing.T) {
	t.Setenv("AUTOSAVE_INTERVAL_TICKS", "250")
	interval, min := autosaveIntervalsFromEnv()
	if interval != 250 || min != 250 {
		t.Fatalf("from env 250 = (%d, %d), want (250, 250)", interval, min)
	}
}

func TestIsConfiguredDeveloperUsername(t *testing.T) {
	devs := []string{" Alice ", "", "BOB", "  "}
	cases := []struct {
		username string
		want     bool
	}{
		{"Alice", true},
		{"alice", true},
		{"ALICE", true},
		{"bob", true},
		{"Bob", true},
		{"Carol", false},
		{"", false},
		{" Alice ", false}, // the username itself is not trimmed in TS
	}
	for _, tc := range cases {
		if got := isConfiguredDeveloperUsername(tc.username, devs); got != tc.want {
			t.Errorf("isConfiguredDeveloperUsername(%q) = %v, want %v", tc.username, got, tc.want)
		}
	}
	if isConfiguredDeveloperUsername("Alice", nil) {
		t.Error("nil dev list matched")
	}
}

func TestEntryAppliesSave(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, false)
	save.Appearance = []int{0, 999, 18, 26, 34, 38, 42, 14, 2, 14, 5, 4, 0}
	save.Extra = map[string]json.RawMessage{"title": json.RawMessage(`"Sir"`)}
	ep := &session.EnteringPlayer{
		Username:       "Alice",
		DisplayName:    "Alice",
		Profile:        "main",
		Save:           save,
		PasswordHash:   "new-hash",
		CredentialName: "Alice",
	}
	p, _ := h.enter(t, ep)

	if got := p.Location(); got != (game.Location{X: 3100, Y: 3530, Z: 0}) {
		t.Errorf("location = %v, want (3100, 3530, 0)", got)
	}
	if p.IsRunning() {
		t.Error("running = true, want false from the save")
	}
	if got := p.GetLastKnownRegion(); got != (game.Location{X: 3100, Y: 3530, Z: 0}) {
		t.Errorf("LastKnownRegion = %v, want the applied location", got)
	}

	carried, activated := h.coord.carriedSave(p)
	if !activated {
		t.Error("carried entry not activated after successful entry")
	}
	if carried != save {
		t.Error("carried save is not the pending save object (bootstrap must read the same, migrated object)")
	}
	// MigrateForEntry effects are visible on the carried save: the
	// out-of-range head kit is clamped to the male range, and the
	// legacy title folds into attributes.
	if len(carried.Appearance) != 13 || carried.Appearance[1] != 8 {
		t.Errorf("appearance = %v, want head kit clamped to 8", carried.Appearance)
	}
	if raw, ok := carried.Attributes["loyalty:title"]; !ok || string(raw) != `"Sir"` {
		t.Errorf("loyalty:title attribute = %q, want Sir", string(raw))
	}
	if _, ok := carried.Extra["title"]; ok {
		t.Error("legacy title key still in Extra after migration")
	}
	if carried.PasswordHashWithSalt != "new-hash" {
		t.Errorf("password hash = %q, want the pending hash", carried.PasswordHashWithSalt)
	}
}

func TestEntryAppliesSaveEmptyPasswordHashKept(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	ep := &session.EnteringPlayer{
		Username:     "Alice",
		Profile:      "main",
		Save:         save,
		PasswordHash: "",
	}
	p, _ := h.enter(t, ep)
	carried, _ := h.coord.carriedSave(p)
	if carried.PasswordHashWithSalt != "old-hash" {
		t.Errorf("password hash = %q, want old-hash kept when the pending hash is empty", carried.PasswordHashWithSalt)
	}
}

func TestEntryNewCharacter(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	ep := &session.EnteringPlayer{
		Username:       "Bob",
		DisplayName:    "Bob",
		Profile:        "main",
		PasswordHash:   "hash-bob",
		CredentialName: "Bob",
	}
	p, _ := h.enter(t, ep)

	if got := p.Location(); got != h.spawn {
		t.Errorf("location = %v, want spawn %v", got, h.spawn)
	}
	if !p.IsRunning() {
		t.Error("running = false, want true (new-character default)")
	}
	if got := p.GetLastKnownRegion(); got != h.spawn {
		t.Errorf("LastKnownRegion = %v, want spawn", got)
	}
	carried, activated := h.coord.carriedSave(p)
	if !activated {
		t.Error("carried entry not activated")
	}
	if carried.DisplayName != "Bob" || carried.PasswordHashWithSalt != "hash-bob" {
		t.Errorf("carried identity = %q/%q", carried.DisplayName, carried.PasswordHashWithSalt)
	}
	if carried.Position == nil || *carried.Position != (persist.Location{X: 3089, Y: 3524, Z: 0}) {
		t.Errorf("carried position = %+v, want spawn", carried.Position)
	}
	if !carried.Running || carried.Rights.ID != 0 {
		t.Errorf("carried running/rights = %v/%d, want true/0 (NONE, non-dev)", carried.Running, carried.Rights.ID)
	}
	if carried.EffectiveProfile() != persist.ProfileMain {
		t.Errorf("carried profile = %q, want main", carried.EffectiveProfile())
	}
	if carried.CreationDate <= 0 {
		t.Error("carried creationDate not set")
	}
	if len(carried.Inventory) != persist.InventorySize || len(carried.Equipment) != persist.EquipmentSize {
		t.Errorf("carried containers = %d/%d, want 28/14", len(carried.Inventory), len(carried.Equipment))
	}
	if len(carried.Skills.Level) != persist.SkillCount {
		t.Errorf("carried skills = %d, want 24", len(carried.Skills.Level))
	}
}

func TestEntryNewCharacterEmptyProfileMeansMain(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	ep := &session.EnteringPlayer{Username: "Carol", Profile: "", PasswordHash: "h"}
	p, _ := h.enter(t, ep)
	carried, _ := h.coord.carriedSave(p)
	if carried.EffectiveProfile() != persist.ProfileMain {
		t.Errorf("empty profile -> %q, want main", carried.EffectiveProfile())
	}
	// And the save round-trips through the store under the main key.
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := h.store.Load("Carol", persist.ProfileMain); err != nil {
		t.Errorf("load under main: %v", err)
	}
}

func TestEntryDevUsernameGrantsDeveloperOnNewCharacter(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	h.adapter.devUsernames = []string{" alice ", "", "BOB"}

	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	carried, _ := h.coord.carriedSave(p)
	if carried.Rights.ID != 2 {
		t.Errorf("dev Alice rights = %d, want 2 (DEVELOPER)", carried.Rights.ID)
	}

	p2, _ := h.enter(t, &session.EnteringPlayer{Username: "bob", Profile: "main", PasswordHash: "h"})
	carried2, _ := h.coord.carriedSave(p2)
	if carried2.Rights.ID != 2 {
		t.Errorf("dev bob rights = %d, want 2 (DEVELOPER)", carried2.Rights.ID)
	}

	p3, _ := h.enter(t, &session.EnteringPlayer{Username: "Carol", Profile: "main", PasswordHash: "h"})
	carried3, _ := h.coord.carriedSave(p3)
	if carried3.Rights.ID != 0 {
		t.Errorf("non-dev Carol rights = %d, want 0 (NONE)", carried3.Rights.ID)
	}
}

func TestEntryLoadedSaveKeepsRightsForDevUsername(t *testing.T) {
	// TS overwrites the LIVE player's rights for dev usernames; the Go
	// Player holds no rights field and rights are PRESERVED in the
	// carried save, so a loaded save keeps its rights (the bootstrap's
	// admin flag already reflects the dev list). Only new-character
	// saves take the grant. See prepareCarriedSave.
	h := newManualHarness(t, 1000, 500)
	h.adapter.devUsernames = []string{"Alice"}
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})
	carried, _ := h.coord.carriedSave(p)
	if carried.Rights.ID != 0 {
		t.Errorf("loaded-save rights = %d, want 0 (preserved)", carried.Rights.ID)
	}
}

func TestEntryTokenFoldPrimaryOnly(t *testing.T) {
	now := time.Now()
	t.Run("primary folds", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		save := saveAtPosition("Alice", 3100, 3530, 0, true)
		ep := &session.EnteringPlayer{
			Username: "Alice", Profile: "main", Save: save,
			CredentialName: "alice", // Key-equal, case differs
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		}
		p, _ := h.enter(t, ep)
		carried, _ := h.coord.carriedSave(p)
		if tokens := carried.Tokens(now); len(tokens) != 1 {
			t.Fatalf("tokens = %d, want 1 folded", len(tokens))
		}
	})
	t.Run("alt does not fold", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		save := saveAtPosition("Alicia", 3100, 3530, 0, true)
		ep := &session.EnteringPlayer{
			Username: "Alicia", Profile: "main", Save: save,
			CredentialName: "Alice",
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		}
		p, _ := h.enter(t, ep)
		carried, _ := h.coord.carriedSave(p)
		if _, ok := carried.Attributes[persist.LoginTokensAttribute]; ok {
			t.Error("alt character folded tokens into its save")
		}
	})
	t.Run("new character primary folds", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		ep := &session.EnteringPlayer{
			Username: "Bob", Profile: "main", PasswordHash: "h",
			CredentialName: "Bob",
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		}
		p, _ := h.enter(t, ep)
		carried, _ := h.coord.carriedSave(p)
		if tokens := carried.Tokens(now); len(tokens) != 1 {
			t.Fatalf("new-character tokens = %d, want 1 folded", len(tokens))
		}
	})
	t.Run("empty credential name never folds", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		ep := &session.EnteringPlayer{
			Username: "Carol", Profile: "main", PasswordHash: "h",
			CredentialName: "",
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		}
		p, _ := h.enter(t, ep)
		carried, _ := h.coord.carriedSave(p)
		if _, ok := carried.Attributes[persist.LoginTokensAttribute]; ok {
			t.Error("empty credential name folded tokens")
		}
	})
}

func TestDisconnectSavesMovedPosition(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3089, 3524, 0, true)
	save.RunEnergy = 42
	save.TeleblockTimer = 99 // snapshots always write 0 (fromPlayer constant)
	save.Attributes["shop:points"] = json.RawMessage(`5`)
	save.Attributes["custom:marker"] = json.RawMessage(`"keep-me"`)
	save.Extra = map[string]json.RawMessage{"custom:extra": json.RawMessage(`7`)}
	p, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})

	// Move on the tick goroutine (this test goroutine drives ticks).
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})
	p.SetRunning(false)
	h.remove(t, index)

	records := h.store.snapshot()
	if len(records) != 1 {
		t.Fatalf("records = %v, want exactly one disconnect save", records)
	}
	rec := records[0]
	if rec.reason != "logout" {
		t.Errorf("reason = %q, want logout", rec.reason)
	}
	if rec.position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
		t.Errorf("position = %+v, want the moved tile", rec.position)
	}
	if rec.running {
		t.Error("running = true, want false")
	}
	stored, err := h.store.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	// Preserved fields ride verbatim; owned fields come from live state.
	// runEnergy is OWNED (phase 7): the entered 42% is live state,
	// and the idle ticks between entry and removal restore 15
	// units (0.15%) each (Agility 1), so the stored value exceeds
	// 42. The exact live value is pinned by
	// TestSnapshotWritesLiveRunEnergy; here only the direction and
	// a generous bound are asserted, since the tick count between
	// entry and removal is timing-dependent.
	if stored.RunEnergy <= 42 || stored.RunEnergy > 43 {
		t.Errorf("runEnergy = %v, want live owned state in (42, 43]", stored.RunEnergy)
	}
	if stored.TeleblockTimer != 0 {
		t.Errorf("teleblockTimer = %d, want 0 (logout ends Tele Block)", stored.TeleblockTimer)
	}
	// A persistent attribute survives the round-trip (loaded at
	// entry, rebuilt at snapshot); an UNREGISTERED attribute key is
	// DROPPED by the snapshot rebuild (fromPlayer rebuilds
	// attributes from the live persistent keys, PlayerSave.ts:811-816
	// — the phase 11 end of preserved-verbatim attributes). Extra
	// keys are still preserved verbatim.
	if string(stored.Attributes["shop:points"]) != `5` {
		t.Errorf("persistent attribute = %s, want 5", string(stored.Attributes["shop:points"]))
	}
	if _, ok := stored.Attributes["custom:marker"]; ok {
		t.Errorf("unregistered attribute survived the snapshot: %v", stored.Attributes)
	}
	if string(stored.Extra["custom:extra"]) != "7" {
		t.Errorf("custom extra = %s, want 7", string(stored.Extra["custom:extra"]))
	}
	if h.coord.carriedCount() != 0 {
		t.Errorf("carried entries = %d after disconnect, want 0", h.coord.carriedCount())
	}
	if h.coord.dueCount() != 0 {
		t.Errorf("due entries = %d after disconnect, want 0 (forgotten)", h.coord.dueCount())
	}
}

func TestDisconnectSaveWritesJSONFile(t *testing.T) {
	// The same disconnect path through the real JSON file store: a
	// file lands on disk and reloads with the moved position.
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	dir := t.TempDir()
	store := persist.NewJSONFileStore(dir)
	world := game.NewWorld(spawn)
	coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 1000, 500)
	adapter := newWorldAdapter(world, spawn, time.Second)
	adapter.saves = coord
	world.Players().OnAdded = adapter.onPlayerAdded
	world.Hooks.OnPlayerDisconnect = adapter.onPlayerDisconnect
	h := &manualHarness{world: world, adapter: adapter, coord: coord, logBuf: &bytes.Buffer{}, spawn: spawn}

	p, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})
	h.remove(t, index)

	path, err := store.FilePath("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("FilePath: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading save file %s: %v", path, err)
	}
	if !strings.Contains(string(data), `"x": 3110`) {
		t.Errorf("save file lacks the moved x:\n%s", data)
	}
	// A fresh store over the same directory loads the save back.
	reloaded, err := persist.NewJSONFileStore(dir).Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Position == nil || *reloaded.Position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
		t.Errorf("reloaded position = %+v, want the moved tile", reloaded.Position)
	}
}

func TestDisconnectSkipsBot(t *testing.T) {
	t.Run("coordinator drops the entry without saving", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{Username: "Botty", Profile: "main", PasswordHash: "h"})
		p.IsBot = true
		h.coord.disconnectSave(p)
		if len(h.store.snapshot()) != 0 {
			t.Errorf("records = %v, want none for a bot", h.store.snapshot())
		}
		if h.coord.carriedCount() != 0 || h.coord.dueCount() != 0 {
			t.Errorf("registry/due leak after bot disconnect: %d/%d", h.coord.carriedCount(), h.coord.dueCount())
		}
	})
	t.Run("world removal of a bot writes nothing", func(t *testing.T) {
		// World.drainRemovePlayers gates OnPlayerDisconnect on
		// non-bot registered players, so the hook never fires for a
		// bot. (Bots never enter via the adapter in production, so
		// no registry entry can exist for one there.)
		h := newManualHarness(t, 1000, 500)
		p, index := h.enter(t, &session.EnteringPlayer{Username: "Botty", Profile: "main", PasswordHash: "h"})
		p.IsBot = true
		h.remove(t, index)
		if len(h.store.snapshot()) != 0 {
			t.Errorf("records = %v, want none for a bot removal", h.store.snapshot())
		}
	})
}

func TestTimedOutAddWritesNoSave(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 1000, 500)
	adapter := newWorldAdapter(world, spawn, 20*time.Millisecond)
	adapter.saves = coord
	world.Players().OnAdded = adapter.onPlayerAdded
	world.Hooks.OnPlayerDisconnect = adapter.onPlayerDisconnect

	// No ticks are driven: the add-wait must time out.
	_, _, err := adapter.AddPlayer(&session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	if err == nil {
		t.Fatal("AddPlayer without ticks = nil error, want timeout")
	}
	if coord.carriedCount() != 0 {
		t.Errorf("carried entries = %d after timeout, want 0", coord.carriedCount())
	}
	if len(store.snapshot()) != 0 {
		t.Errorf("records = %v after timeout, want none", store.snapshot())
	}

	// The late add is cancelled and removed by later ticks; still
	// nothing is ever written, and no entry leaks.
	world.Process()
	world.Process()
	if len(store.snapshot()) != 0 {
		t.Errorf("records = %v after late add/removal, want none", store.snapshot())
	}
	if coord.carriedCount() != 0 {
		t.Errorf("carried entries = %d, want 0", coord.carriedCount())
	}
	if world.Players().Len() != 0 {
		t.Errorf("world players = %d, want 0 (late add removed)", world.Players().Len())
	}
	if adapter.isOnline(pendingKey("Alice")) {
		t.Error("Alice still online after late-add removal")
	}
}

func TestDisconnectFailingStore(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	h.store.failAll = errors.New("disk on fire")
	p, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})
	h.remove(t, index)

	// The player still leaves, the entry is dropped, and the failure
	// is logged — and the tick loop continues afterwards.
	if worldLen := h.world.Players().Len(); worldLen != 0 {
		t.Errorf("world players = %d, want 0 after failed disconnect save", worldLen)
	}
	if h.coord.carriedCount() != 0 {
		t.Errorf("carried entries = %d, want 0", h.coord.carriedCount())
	}
	if !strings.Contains(h.logBuf.String(), "Failed to save player Alice on disconnect") {
		t.Errorf("log = %q, want a disconnect-save failure line", h.logBuf.String())
	}
	before := h.world.ProcessCycle()
	h.world.Process()
	if h.world.ProcessCycle() != before+1 {
		t.Error("tick did not advance after a failed disconnect save")
	}
}

func TestDisconnectPanickingStore(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	h.store.panicOn["Alice"] = true
	_, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	// Must not panic out of the removal drain.
	h.remove(t, index)
	if h.world.Players().Len() != 0 {
		t.Error("player did not leave after a panicking store")
	}
	if !strings.Contains(h.logBuf.String(), "panic saving Alice") {
		t.Errorf("log = %q, want the recovered panic", h.logBuf.String())
	}
}

func TestSaveAllCountsAndSkips(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	h.enter(t, &session.EnteringPlayer{Username: "Bob", Profile: "main", PasswordHash: "h"})
	// A bare registration bypassing the adapter: no carried save.
	bare := game.NewPlayer("bare", h.spawn)
	h.world.QueueAddPlayer(bare)
	h.world.Process()
	// A bot with a carried entry: skipped, not failed.
	bot, _ := h.enter(t, &session.EnteringPlayer{Username: "Botty", Profile: "main", PasswordHash: "h"})
	bot.IsBot = true

	saved, failed := h.coord.saveAll("shutdown")
	if saved != 2 || failed != 0 {
		t.Errorf("saveAll = (%d, %d), want (2, 0)", saved, failed)
	}
	if got := h.store.usernames(); len(got) != 2 {
		t.Errorf("records = %v, want Alice + Bob only", got)
	}
	for _, r := range h.store.snapshot() {
		if r.reason != "shutdown" {
			t.Errorf("reason = %q, want shutdown", r.reason)
		}
	}
	if !strings.Contains(h.logBuf.String(), "savePlayers saved=2 failed=0") {
		t.Errorf("log = %q, want the savePlayers summary", h.logBuf.String())
	}

	// Failures count and log per player; the summary reflects them.
	h.store.failAll = errors.New("boom")
	h.logBuf.Reset()
	saved, failed = h.coord.saveAll("tick-error")
	if saved != 0 || failed != 2 {
		t.Errorf("saveAll failing = (%d, %d), want (0, 2)", saved, failed)
	}
	if !strings.Contains(h.logBuf.String(), "savePlayers saved=0 failed=2") {
		t.Errorf("log = %q, want the failing summary", h.logBuf.String())
	}
}

func TestAutosaveScheduleSpread(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 1000, 500)

	names := []string{"alice", "bob", "carol", "dave", "erin", "frank"}
	wantDue := map[string]int{"alice": 605, "bob": 522, "carol": 924, "dave": 937, "erin": 579, "frank": 556}
	players := map[string]*game.Player{}
	for _, name := range names {
		p := game.NewPlayer(name, spawn)
		players[name] = p
		coord.registerPending(p, saveAtPosition(name, 3089, 3524, 0, true))
		coord.activate(p)
		coord.schedule(p)
		if got := coord.dueAt[p]; int(got) != wantDue[name] {
			t.Errorf("due(%s) = %d, want %d", name, got, wantDue[name])
		}
	}
	// Run ticks until every player has saved once; each must save
	// exactly on its due tick (spread by name, not all at once).
	firstSave := map[string]int{}
	for tick := 1; tick <= 1000 && len(firstSave) < len(names); tick++ {
		before := len(store.snapshot())
		coord.runAutosave()
		for _, r := range store.snapshot()[before:] {
			if _, ok := firstSave[r.username]; !ok {
				firstSave[r.username] = tick
			}
			if r.reason != "autosave" {
				t.Errorf("reason = %q, want autosave", r.reason)
			}
		}
	}
	for name, want := range wantDue {
		if got, ok := firstSave[name]; !ok || got != want {
			t.Errorf("first save(%s) at tick %d, want %d", name, got, want)
		}
	}
}

func TestAutosaveBudgetAndReschedule(t *testing.T) {
	runScenario := func(t *testing.T) []string {
		t.Helper()
		spawn := game.Location{X: 3089, Y: 3524, Z: 0}
		world := game.NewWorld(spawn)
		store := newRecordingSaveStore()
		coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 5, 5)
		names := []string{"p01", "p02", "p03", "p04", "p05", "p06", "p07", "p08", "p09", "p10", "p11", "p12"}
		players := map[string]*game.Player{}
		for _, name := range names {
			p := game.NewPlayer(name, spawn)
			players[name] = p
			coord.registerPending(p, saveAtPosition(name, 3089, 3524, 0, true))
			coord.activate(p)
			coord.schedule(p)
		}
		for i := 0; i < 4; i++ {
			coord.runAutosave()
		}
		if len(store.snapshot()) != 0 {
			t.Fatalf("saves before due tick: %v", store.snapshot())
		}
		// Tick 5: the budget saves 10, in insertion order.
		coord.runAutosave()
		got := store.usernames()
		if len(got) != 10 {
			t.Fatalf("tick-5 saves = %v, want 10", got)
		}
		for i, name := range names[:10] {
			if got[i] != name {
				t.Fatalf("tick-5 save order = %v, want insertion order", got)
			}
		}
		// The over-budget players wait exactly one tick.
		if due := coord.dueAt[players["p11"]]; due != 6 {
			t.Errorf("due(p11) = %d, want 6 (tick+1)", due)
		}
		if due := coord.dueAt[players["p12"]]; due != 6 {
			t.Errorf("due(p12) = %d, want 6 (tick+1)", due)
		}
		coord.runAutosave()
		got = store.usernames()
		if len(got) != 12 || got[10] != "p11" || got[11] != "p12" {
			t.Fatalf("all saves = %v, want p01..p12 in order", got)
		}
		return got
	}
	first := runScenario(t)
	second := runScenario(t)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("save order not deterministic: %v vs %v", first, second)
		}
	}
}

func TestAutosaveBotSkipAdvancesDue(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 5, 5)

	bot := game.NewPlayer("botty", spawn)
	bot.IsBot = true
	coord.registerPending(bot, saveAtPosition("botty", 3089, 3524, 0, true))
	coord.activate(bot)
	coord.schedule(bot) // scheduled first: insertion order puts it first
	for i := 0; i < 10; i++ {
		name := "human" + string(rune('a'+i))
		p := game.NewPlayer(name, spawn)
		coord.registerPending(p, saveAtPosition(name, 3089, 3524, 0, true))
		coord.activate(p)
		coord.schedule(p)
	}
	for i := 0; i < 5; i++ {
		coord.runAutosave()
	}
	// The bot consumed no budget (all 10 humans saved on the due tick)
	// and was never written — but its due tick advanced.
	if got := len(store.snapshot()); got != 10 {
		t.Fatalf("saves = %d, want 10 (bot consumes no budget)", got)
	}
	for _, r := range store.snapshot() {
		if r.username == "botty" {
			t.Error("bot was saved")
		}
	}
	if due := coord.dueAt[bot]; due != 10 {
		t.Errorf("bot due = %d, want 10 (rescheduled before the skip check)", due)
	}
	for i := 0; i < 5; i++ {
		coord.runAutosave()
	}
	for _, r := range store.snapshot() {
		if r.username == "botty" {
			t.Error("bot was saved on a later tick")
		}
	}
	if due := coord.dueAt[bot]; due != 15 {
		t.Errorf("bot due = %d, want 15", due)
	}
}

func TestAutosaveFailureIsolation(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	store.failFor["failer"] = errors.New("boom")
	logBuf := &bytes.Buffer{}
	coord := newSaveCoordinator(store, world, log.New(logBuf, "", 0), 10, 5, 5)
	for _, name := range []string{"failer", "ok1", "ok2"} {
		p := game.NewPlayer(name, spawn)
		coord.registerPending(p, saveAtPosition(name, 3089, 3524, 0, true))
		coord.activate(p)
		coord.schedule(p)
	}
	for i := 0; i < 5; i++ {
		coord.runAutosave()
	}
	// All three were attempted (the recording store logs attempts);
	// only the two healthy players persisted.
	if got := len(store.snapshot()); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}
	if _, err := store.Load("failer", persist.ProfileMain); !errors.Is(err, persist.ErrSaveNotFound) {
		t.Errorf("failer persisted despite the failure: err = %v", err)
	}
	for _, name := range []string{"ok1", "ok2"} {
		if _, err := store.Load(name, persist.ProfileMain); err != nil {
			t.Errorf("load %s: %v", name, err)
		}
	}
	if !strings.Contains(logBuf.String(), "[autosave] Failed to save failer") {
		t.Errorf("log = %q, want the per-player failure", logBuf.String())
	}
	// The failed player is retried on its next due tick (it was
	// rescheduled before the save attempt, as TS does).
	logBuf.Reset()
	for i := 0; i < 5; i++ {
		coord.runAutosave()
	}
	if !strings.Contains(logBuf.String(), "[autosave] Failed to save failer") {
		t.Errorf("log = %q, want the retry failure on the next due tick", logBuf.String())
	}
}

func TestAutosaveForget(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	store := newRecordingSaveStore()
	coord := newSaveCoordinator(store, world, log.New(io.Discard, "", 0), 10, 2, 2)
	p := game.NewPlayer("alice", spawn)
	coord.registerPending(p, saveAtPosition("alice", 3089, 3524, 0, true))
	coord.activate(p)
	coord.schedule(p)
	coord.forget(p)
	if coord.dueCount() != 0 {
		t.Fatal("forget did not remove the schedule")
	}
	for i := 0; i < 10; i++ {
		coord.runAutosave()
	}
	if len(store.snapshot()) != 0 {
		t.Errorf("records = %v, want none after forget", store.snapshot())
	}
	// Re-scheduling after forgetting appends at the end (insertion
	// order is per current membership, as a JS Map re-insert does).
	coord.schedule(p)
	if coord.dueCount() != 1 {
		t.Fatal("re-schedule after forget did not re-add")
	}
}

func TestAutosaveTaskRunsViaTaskManager(t *testing.T) {
	// Delay-1 task semantics: the task executes on the first Process
	// and every tick after (Task.Tick with delay 1).
	h := newManualHarness(t, 2, 2)
	h.world.Tasks().Schedule(h.coord.autosaveTask())
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	// The entry drove ticks already; the schedule was made at the
	// coordinator tick then current. Drive until the autosave fires.
	deadline := time.Now().Add(2 * time.Second)
	for len(h.store.snapshot()) == 0 {
		h.world.Process()
		if time.Now().After(deadline) {
			t.Fatal("autosave task never fired through TaskManager")
		}
	}
	if h.store.snapshot()[0].reason != "autosave" {
		t.Errorf("reason = %q, want autosave", h.store.snapshot()[0].reason)
	}
	_ = p
}

func TestShutdownSnapshotDirect(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})

	// Engine never started: the snapshot is taken directly.
	snaps := h.coord.requestShutdownSnapshot(false, time.Second)
	if len(snaps) != 1 {
		t.Fatalf("snapshots = %d, want 1", len(snaps))
	}
	if snaps[0].Position == nil || *snaps[0].Position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
		t.Errorf("snapshot position = %+v, want the moved tile", snaps[0].Position)
	}
	// Snapshots are immutable: moving after the snapshot does not
	// change what gets written.
	p.SetLocation(game.Location{X: 3200, Y: 3600, Z: 0})
	saved, failed := h.coord.writeSnapshots(snaps, "shutdown")
	if saved != 1 || failed != 0 {
		t.Fatalf("writeSnapshots = (%d, %d), want (1, 0)", saved, failed)
	}
	if recs := h.store.snapshot(); len(recs) != 1 || recs[0].reason != "shutdown" || recs[0].position.X != 3110 {
		t.Errorf("records = %v, want one shutdown write at the snapshot tile", recs)
	}
	// A repeated request reuses the cached snapshot.
	again := h.coord.requestShutdownSnapshot(false, time.Second)
	if len(again) != 1 || again[0] != snaps[0] {
		t.Error("repeated shutdown request did not return the cached snapshot")
	}
}

func TestShutdownSnapshotTimeout(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	world := game.NewWorld(spawn)
	logBuf := &bytes.Buffer{}
	coord := newSaveCoordinator(newRecordingSaveStore(), world, log.New(logBuf, "", 0), 10, 1000, 500)
	// Engine "started" but no tick ever runs: the wait times out and
	// yields no snapshot, rather than reading player state off-tick.
	snaps := coord.requestShutdownSnapshot(true, 20*time.Millisecond)
	if snaps != nil {
		t.Errorf("snapshots = %v, want nil on timeout", snaps)
	}
	if !strings.Contains(logBuf.String(), "timed out waiting for the tick-goroutine save snapshot") {
		t.Errorf("log = %q, want the timeout line", logBuf.String())
	}
	// The timeout result is cached: a repeat returns immediately.
	start := time.Now()
	if again := coord.requestShutdownSnapshot(true, 20*time.Millisecond); again != nil {
		t.Errorf("cached snapshots = %v, want nil", again)
	}
	if elapsed := time.Since(start); elapsed >= 20*time.Millisecond {
		t.Errorf("cached request took %v, want an immediate return", elapsed)
	}
}

func TestShutdownSnapshotViaTaskDelivery(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	h.world.Tasks().Schedule(h.coord.autosaveTask())
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})

	// The request runs on a helper goroutine (as Stop would) while
	// this goroutine drives ticks; the autosave task delivers the
	// snapshot on its next run.
	type result struct{ snaps []*persist.PlayerSave }
	done := make(chan result, 1)
	go func() {
		done <- result{h.coord.requestShutdownSnapshot(true, 2*time.Second)}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		select {
		case r := <-done:
			if len(r.snaps) != 1 {
				t.Fatalf("snapshots = %d, want 1", len(r.snaps))
			}
			if r.snaps[0].Position == nil || *r.snaps[0].Position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
				t.Fatalf("snapshot position = %+v, want the moved tile", r.snaps[0].Position)
			}
			return
		default:
			h.world.Process()
			if time.Now().After(deadline) {
				t.Fatal("shutdown snapshot never delivered")
			}
		}
	}
}

func TestShutdownEngineNeverStartedServer(t *testing.T) {
	srv := testServer(t, 10) // no Start: the engine never runs
	ch := &recordingChannel{}
	conn := session.NewConnection(ch, srv.loginDeps, srv.adapter, 10)
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientLogin, loginPayload("alice", "secret", 242, 0))); err != nil {
		t.Fatalf("login: %v", err)
	}
	// The handshake blocks in AddPlayer until a tick assigns the
	// index; drive ticks on this goroutine until the handshake
	// goroutine returns. (Do NOT poll conn.Entered() in the loop
	// condition: enterWorldLocked holds the connection mutex for the
	// whole blocking AddPlayer wait, so Entered would deadlock the
	// tick driver. Check it only after the handshake returns.)
	handshakeDone := make(chan error, 1)
	go func() {
		handshakeDone <- conn.HandleFrame(clientFrame(t, protocol.ClientHandshake, handshakePayload("alice")))
	}()
	deadline := time.Now().Add(5 * time.Second)
handshakeLoop:
	for {
		select {
		case err := <-handshakeDone:
			if err != nil {
				t.Fatalf("handshake: %v", err)
			}
			break handshakeLoop
		default:
			srv.world.Process()
			if time.Now().After(deadline) {
				t.Fatal("player never entered while driving ticks manually")
			}
		}
	}
	if !conn.Entered() {
		t.Fatalf("connection did not enter: %q", conn.LoginFailure())
	}
	players := srv.world.Players().All()
	if len(players) != 1 {
		t.Fatalf("players = %d, want 1", len(players))
	}
	players[0].SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})

	srv.Stop() // direct snapshot path (engine never started)

	stored, err := srv.saves.Load("alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load after shutdown: %v", err)
	}
	if stored.Position == nil || *stored.Position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
		t.Errorf("stored position = %+v, want the moved tile", stored.Position)
	}
}

func TestShutdownEngineDrivenSavesWalkedPosition(t *testing.T) {
	srv := testServer(t, 10)
	// Capture reasons: swap the coordinator's store for a recording
	// one before anyone enters. (Login token writes use the separate
	// saveAdapter store and are unaffected.)
	rec := newRecordingSaveStore()
	srv.adapter.saves.store = rec
	srv.Start()

	_, conn := enterTestPlayer(t, srv, "alice", "secret")
	defer conn.Cleanup()

	walkPayload := []byte{0x0c, 0x1c, 0x0d, 0xca, 0x03} // (3100, 3530), flags 3
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientWalk, walkPayload)); err != nil {
		t.Fatalf("HandleFrame(walk): %v", err)
	}
	var player *game.Player
	waitFor(t, "player registration", 2*time.Second, func() bool {
		players := srv.world.Players().All()
		if len(players) == 1 {
			player = players[0]
			return true
		}
		return false
	})
	waitFor(t, "turn queue drain", 2*time.Second, func() bool {
		return player.TurnPacketCount() == 0
	})
	drainedCycle := srv.world.ProcessCycle()
	waitFor(t, "walk arrival", 2*time.Second, func() bool {
		return srv.world.ProcessCycle() >= drainedCycle+20
	})

	srv.Stop()

	records := rec.snapshot()
	if len(records) != 1 {
		t.Fatalf("records = %v, want exactly one shutdown save", records)
	}
	if records[0].reason != "shutdown" {
		t.Errorf("reason = %q, want shutdown", records[0].reason)
	}
	if records[0].position != (persist.Location{X: 3100, Y: 3530, Z: 0}) {
		t.Errorf("position = %+v, want the walked-to tile (3100, 3530, 0)", records[0].position)
	}
}

// findRebuildRegion returns the region (chunk) coordinates of the first
// REBUILD_NORMAL packet in frames, if any.
func findRebuildRegion(t *testing.T, frames [][]byte) (int, int, bool) {
	t.Helper()
	for _, frame := range frames {
		for _, p := range decodeServerFrame(t, frame) {
			if p.Opcode == protocol.ServerRebuildNormal {
				if len(p.Payload) < 4 {
					t.Fatalf("rebuild payload = %d bytes, want >= 4", len(p.Payload))
				}
				return int(binary.BigEndian.Uint16(p.Payload[0:2])), int(binary.BigEndian.Uint16(p.Payload[2:4])), true
			}
		}
	}
	return 0, 0, false
}

func TestReloginResumesAtWalkedTile(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()

	// First login: walk away from spawn, then disconnect.
	ch, conn := enterTestPlayer(t, srv, "alice", "secret")
	walkPayload := []byte{0x0c, 0x1c, 0x0d, 0xca, 0x03} // (3100, 3530), flags 3
	if err := conn.HandleFrame(clientFrame(t, protocol.ClientWalk, walkPayload)); err != nil {
		t.Fatalf("HandleFrame(walk): %v", err)
	}
	var player *game.Player
	waitFor(t, "player registration", 2*time.Second, func() bool {
		players := srv.world.Players().All()
		if len(players) == 1 {
			player = players[0]
			return true
		}
		return false
	})
	waitFor(t, "turn queue drain", 2*time.Second, func() bool {
		return player.TurnPacketCount() == 0
	})
	drainedCycle := srv.world.ProcessCycle()
	waitFor(t, "walk arrival", 2*time.Second, func() bool {
		return srv.world.ProcessCycle() >= drainedCycle+20
	})
	// The first login's rebuild centers on the spawn chunk.
	waitFor(t, "first rebuild", 2*time.Second, func() bool {
		_, _, ok := findRebuildRegion(t, ch.sent())
		return ok
	})
	rx, ry, _ := findRebuildRegion(t, ch.sent())
	if rx != 386 || ry != 440 { // spawn (3089, 3524) >> 3
		t.Errorf("first rebuild region = (%d, %d), want (386, 440)", rx, ry)
	}

	conn.Cleanup()
	waitFor(t, "player removal", 2*time.Second, func() bool {
		return srv.world.Players().Len() == 0
	})
	// The disconnect save holds the walked-to tile.
	waitFor(t, "disconnect save", 2*time.Second, func() bool {
		stored, err := srv.saves.Load("alice", persist.ProfileMain)
		return err == nil && stored.Position != nil && *stored.Position == (persist.Location{X: 3100, Y: 3530, Z: 0})
	})

	// Second login: the player resumes at the walked-to tile, not
	// the spawn — visible in the rebuild region the client receives.
	ch2, conn2 := enterTestPlayer(t, srv, "alice", "secret")
	defer conn2.Cleanup()
	waitFor(t, "relogin rebuild", 2*time.Second, func() bool {
		_, _, ok := findRebuildRegion(t, ch2.sent())
		return ok
	})
	rx, ry, _ = findRebuildRegion(t, ch2.sent())
	if rx != 387 || ry != 441 { // (3100, 3530) >> 3
		t.Errorf("relogin rebuild region = (%d, %d), want (387, 441) (resumed, not spawn)", rx, ry)
	}
}

func TestReloginRoundTripOverWebSocket(t *testing.T) {
	srv := testServer(t, 10)
	srv.Start()
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http")
	dialer := websocket.Dialer{HandshakeTimeout: 5 * time.Second}

	loginAndRebuild := func(t *testing.T, username string) (*websocket.Conn, int, int) {
		t.Helper()
		ws, _, err := dialer.Dial(wsURL, nil)
		if err != nil {
			t.Fatalf("dial: %v", err)
		}
		if err := ws.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
			t.Fatalf("SetReadDeadline: %v", err)
		}
		// WELCOME, then LOGIN + response, then HANDSHAKE, then read
		// until the first REBUILD_NORMAL arrives.
		if _, _, err := ws.ReadMessage(); err != nil {
			t.Fatalf("reading welcome: %v", err)
		}
		if err := ws.WriteMessage(websocket.BinaryMessage, clientFrame(t, protocol.ClientLogin, loginPayload(username, "secret", 242, 0))); err != nil {
			t.Fatalf("writing login: %v", err)
		}
		if _, resp, err := ws.ReadMessage(); err != nil {
			t.Fatalf("reading login response: %v", err)
		} else {
			packets := decodeServerFrame(t, resp)
			if len(packets) != 1 || packets[0].Opcode != protocol.ServerLoginResponse {
				t.Fatalf("login response packets = %+v", packets)
			}
			success, code, _, _, _ := parseLoginResponse(t, packets[0].Payload)
			if success != 1 || code != -1 {
				t.Fatalf("login response = success %d code %d, want 1/-1", success, code)
			}
		}
		if err := ws.WriteMessage(websocket.BinaryMessage, clientFrame(t, protocol.ClientHandshake, handshakePayload(username))); err != nil {
			t.Fatalf("writing handshake: %v", err)
		}
		for {
			_, msg, err := ws.ReadMessage()
			if err != nil {
				t.Fatalf("reading for rebuild: %v", err)
			}
			for _, p := range decodeServerFrame(t, msg) {
				if p.Opcode == protocol.ServerRebuildNormal {
					if len(p.Payload) < 4 {
						t.Fatalf("rebuild payload = %d bytes", len(p.Payload))
					}
					return ws, int(binary.BigEndian.Uint16(p.Payload[0:2])), int(binary.BigEndian.Uint16(p.Payload[2:4]))
				}
			}
		}
	}

	// First session: spawn rebuild, walk away, disconnect.
	ws, rx, ry := loginAndRebuild(t, "wally")
	if rx != 386 || ry != 440 {
		t.Errorf("first rebuild region = (%d, %d), want (386, 440)", rx, ry)
	}
	walkPayload := []byte{0x0c, 0x1c, 0x0d, 0xca, 0x03} // (3100, 3530), flags 3
	if err := ws.WriteMessage(websocket.BinaryMessage, clientFrame(t, protocol.ClientWalk, walkPayload)); err != nil {
		t.Fatalf("writing walk: %v", err)
	}
	time.Sleep(600 * time.Millisecond) // ~60 ticks: ample for arrival
	if err := ws.Close(); err != nil {
		t.Fatalf("closing socket: %v", err)
	}
	waitFor(t, "player removal after socket close", 2*time.Second, func() bool {
		return srv.world.Players().Len() == 0
	})
	waitFor(t, "disconnect save", 2*time.Second, func() bool {
		stored, err := srv.saves.Load("wally", persist.ProfileMain)
		return err == nil && stored.Position != nil && *stored.Position == (persist.Location{X: 3100, Y: 3530, Z: 0})
	})

	// Second session: resume at the walked-to tile.
	ws2, rx, ry := loginAndRebuild(t, "wally")
	defer ws2.Close()
	if rx != 387 || ry != 441 {
		t.Errorf("relogin rebuild region = (%d, %d), want (387, 441) (resumed, not spawn)", rx, ry)
	}
}

// --- Phase 7: run-energy entry application and snapshot tests ---

// testItemDefinitions is an ItemDefinitionSource fake overriding
// only the three methods the live weight computation reads; the
// embedded nil interface is never called.
type testItemDefinitions struct {
	game.ItemDefinitionSource
	grams map[int]int
}

func (d testItemDefinitions) IsStackable(int) bool   { return false }
func (d testItemDefinitions) IsNoted(int) bool       { return false }
func (d testItemDefinitions) WeightGrams(id int) int { return d.grams[id] }

func TestEntryAppliesRunEnergyState(t *testing.T) {
	// prepareCarriedSave is exercised directly (no ticks run), so
	// the restore branch cannot add units between application and
	// assertion: the applied state is exact.
	h := newManualHarness(t, 1000, 500)
	h.adapter.itemDefinitions = testItemDefinitions{grams: map[int]int{100: 2500, 200: 1500}}
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.RunEnergy = 42.5
	save.Skills.Level[persist.AgilityIndex] = 73
	save.Equipment[0] = persist.Item{ID: 100, Amount: 1} // 2500g
	save.Inventory[0] = persist.Item{ID: 200, Amount: 5} // 1500g, amount ignored
	save.Inventory[1] = persist.Item{ID: 300, Amount: 1} // absent from weights: 0g
	p := game.NewPlayer("Alice", h.spawn)
	ep := &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "new-hash", CredentialName: "Alice"}
	h.adapter.prepareCarriedSave(p, ep)

	if got := p.GetRunEnergyUnits(); got != 4250 {
		t.Errorf("units = %d, want 4250 (42.5%%)", got)
	}
	if got := p.GetRunEnergy(); got != 42.5 {
		t.Errorf("percent = %v, want 42.5", got)
	}
	if got := p.GetCurrentAgilityLevel(); got != 73 {
		t.Errorf("agility = %d, want 73", got)
	}
	// (2500 + 1500) / 1000 = 4 kg.
	if got := p.GetWeightKg(); got != 4 {
		t.Errorf("weight = %d kg, want 4", got)
	}
}

func TestEntryAppliesRunEnergyFloatEdge(t *testing.T) {
	// The 0.29 relog regression: percent -> units must not lose a
	// unit (setRunEnergy's +1e-6 hair, Player.ts:1263).
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.RunEnergy = 0.29
	p := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p, &session.EnteringPlayer{Username: "Alice", Save: save})
	if got := p.GetRunEnergyUnits(); got != 29 {
		t.Errorf("units = %d, want 29", got)
	}
}

func TestEntryEnergyWithoutWeightSourceIsZero(t *testing.T) {
	// No definition source on the adapter (NoCache mode): weight 0
	// even with a full inventory, exactly as the bootstrap sends 0.
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Inventory[0] = persist.Item{ID: 100, Amount: 1}
	p := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p, &session.EnteringPlayer{Username: "Alice", Save: save})
	if got := p.GetWeightKg(); got != 0 {
		t.Errorf("weight = %d, want 0 with no weight source", got)
	}
}

func TestEntryNewCharacterEnergyDefaults(t *testing.T) {
	// No save: max units, Agility 1, weight 0 — applied from the
	// built new-character save, not merely NewPlayer defaults.
	h := newManualHarness(t, 1000, 500)
	p := game.NewPlayer("Bob", h.spawn)
	carried := h.adapter.prepareCarriedSave(p, &session.EnteringPlayer{
		Username: "Bob", PasswordHash: "hash", Profile: "main",
	})
	if got := p.GetRunEnergyUnits(); got != game.RunEnergyMaxUnits {
		t.Errorf("units = %d, want %d", got, game.RunEnergyMaxUnits)
	}
	if got := p.GetCurrentAgilityLevel(); got != 1 {
		t.Errorf("agility = %d, want 1", got)
	}
	if got := p.GetWeightKg(); got != 0 {
		t.Errorf("weight = %d, want 0 (empty containers)", got)
	}
	if carried.RunEnergy != 100 {
		t.Errorf("new-character save runEnergy = %v, want 100", carried.RunEnergy)
	}
}

// dropWiringDefs is an ItemDefinitionSource fake for the
// turn-handler wiring test: every id is dropable, nothing stacks,
// nothing weighs anything. The embedded nil interface is never
// called: the drop/pickup paths read only the overridden methods
// (plus the optional action source, which this fake does not
// implement, so routing uses the ObjType defaults).
type dropWiringDefs struct {
	game.ItemDefinitionSource
}

func (dropWiringDefs) IsStackable(int) bool { return false }
func (dropWiringDefs) IsNoted(int) bool     { return false }
func (dropWiringDefs) WeightGrams(int) int  { return 0 }
func (dropWiringDefs) IsDropable(int) bool  { return true }

func wiringInventoryUsePayload(slot, itemID int, option string) []byte {
	payload := make([]byte, 0, 8+len(option)+1)
	var tmp [4]byte
	binary.BigEndian.PutUint16(tmp[:2], uint16(slot))
	payload = append(payload, tmp[:2]...)
	binary.BigEndian.PutUint16(tmp[:2], uint16(itemID))
	payload = append(payload, tmp[:2]...)
	payload = append(payload, 0, 0, 0, 0) // discarded int
	payload = append(payload, option...)
	return append(payload, 0)
}

func wiringGroundItemActionPayload(stackID int, x, y, itemID int, option string, optionIndex byte) []byte {
	payload := make([]byte, 0, 16+len(option)+1)
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(int32(stackID)))
	payload = append(payload, tmp[:]...)
	binary.BigEndian.PutUint16(tmp[:2], uint16(x))
	payload = append(payload, tmp[:2]...)
	binary.BigEndian.PutUint16(tmp[:2], uint16(y))
	payload = append(payload, tmp[:2]...)
	payload = append(payload, 0) // skipped byte
	binary.BigEndian.PutUint16(tmp[:2], uint16(itemID))
	payload = append(payload, tmp[:2]...)
	payload = append(payload, 0, 0, 0, 0) // skipped int
	payload = append(payload, option...)
	payload = append(payload, 0)
	return append(payload, optionIndex)
}

func TestTurnHandlerWiresGroundItems(t *testing.T) {
	// AddPlayer installs a TurnHandler closure carrying the
	// world's ground-item manager (Deps.GroundItems): a drop
	// packet through the handler registers a stack, and a
	// ground-item take through the same handler picks it back
	// up — standing on the tile, so the arrival action runs
	// immediately. No ticks are driven after entry: the handler
	// is invoked directly, as Player.Process would invoke it.
	h := newManualHarness(t, 1000, 500)
	mgr := game.NewGroundItemManager(game.GroundItemManagerConfig{})
	h.adapter.groundItems = mgr
	h.adapter.itemDefinitions = dropWiringDefs{}
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Inventory[0] = persist.Item{ID: 100, Amount: 3}
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	if p.TurnHandler == nil {
		t.Fatal("no TurnHandler installed")
	}
	loc := p.Location()

	// Drop slot 0 ("Drop" option) through the handler.
	p.TurnHandler(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: wiringInventoryUsePayload(0, 100, "Drop")}, 7)
	if got := p.Inventory().ItemAt(0); got.ID() != -1 {
		t.Fatalf("slot 0 after drop = id %d, want empty", got.ID())
	}
	stacks := mgr.Items()
	if len(stacks) != 1 || stacks[0].ItemID != 100 || stacks[0].Amount != 3 {
		t.Fatalf("manager stacks = %+v, want one stack of 3x100", stacks)
	}
	if !stacks[0].Position.Equals(loc) {
		t.Fatalf("stack at %+v, want player tile %+v", stacks[0].Position, loc)
	}

	// Take it back through the handler (stack-id path, option
	// "Take"), on the next cycle.
	p.TurnHandler(p, game.TurnPacket{Opcode: protocol.ClientGroundItemAction, Payload: wiringGroundItemActionPayload(stacks[0].ID(), loc.X, loc.Y, 100, "Take", 3)}, 8)
	if got := p.Inventory().GetAmount(100); got != 3 {
		t.Fatalf("inventory amount of 100 after pickup = %d, want 3", got)
	}
	if !stacks[0].PendingRemoval {
		t.Fatal("stack not deregistered after full pickup")
	}
}

func TestSnapshotWritesLiveRunEnergy(t *testing.T) {
	// The snapshot path reads LIVE units (no ticks driven between
	// setting them and snapshotting, so no restore interferes).
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.RunEnergy = 42.5
	ep := &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"}
	p, _ := h.enter(t, ep)
	p.SetRunEnergyUnits(3123)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if stored.RunEnergy != 31.23 {
		t.Errorf("stored runEnergy = %v, want 31.23 (live units)", stored.RunEnergy)
	}
	if stored.Position == nil || *stored.Position != (persist.Location{X: 3100, Y: 3530, Z: 0}) {
		t.Errorf("stored position = %+v, want the entered tile", stored.Position)
	}
	// The carried save itself is never mutated by a snapshot.
	carried, _ := h.coord.carriedSave(p)
	if carried.RunEnergy != 42.5 {
		t.Errorf("carried runEnergy = %v, want 42.5 (snapshot must not mutate the base)", carried.RunEnergy)
	}
}

func TestSnapshotEnergyRoundTripsThroughRelogin(t *testing.T) {
	// Enter at 42.5%, snapshot at live 3123 units, re-enter from
	// the stored save: the player resumes at 3123 units.
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.RunEnergy = 42.5
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	p.SetRunEnergyUnits(3123)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	p2 := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p2, &session.EnteringPlayer{Username: "Alice", Save: stored})
	if got := p2.GetRunEnergyUnits(); got != 3123 {
		t.Errorf("relogin units = %d, want 3123", got)
	}
}

func TestSnapshotWritesLivePoison(t *testing.T) {
	// Phase 28 (coordinator integration): the snapshot writes
	// the LIVE poison state (fromPlayer, PlayerSave.ts:764-771),
	// and entry loads it back (applyToPlayer, :656-660).
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	p.SetPoisonDamage(20)
	p.GetCombat().PoisonClock().Start(180) // counter 10, phase 30
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if stored.PoisonDamage != 20 || stored.Venomed || stored.LastVenomDamage != nil {
		t.Fatalf("stored affliction = damage %d venomed %v lastVenom %v, want 20/false/nil",
			stored.PoisonDamage, stored.Venomed, stored.LastVenomDamage)
	}
	if stored.PoisonClock == nil || stored.PoisonClock.Counter != 10 {
		t.Fatalf("stored clock = %+v, want counter 10", stored.PoisonClock)
	}
	if stored.PoisonImmunityTimer != 180 {
		t.Fatalf("stored immunity timer = %d, want 180", stored.PoisonImmunityTimer)
	}
	// The carried save itself is never mutated by a snapshot.
	carried, _ := h.coord.carriedSave(p)
	if carried.PoisonDamage != 0 || carried.PoisonClock != nil {
		t.Fatalf("carried poison = damage %d clock %+v, want 0/nil (snapshot must not mutate the base)",
			carried.PoisonDamage, carried.PoisonClock)
	}
}

func TestSnapshotPoisonRoundTripsThroughRelogin(t *testing.T) {
	// Enter, get poisoned, snapshot, re-enter from the stored
	// save: the player resumes poisoned with the same damage
	// and clock counter. Curing clears the affliction but the
	// immunity dose survives (TS writes the clock while
	// !finished()); only a spent dose clears the clock.
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	p.SetPoisonDamage(20)
	p.GetCombat().PoisonClock().Start(180) // counter 10
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	p2 := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p2, &session.EnteringPlayer{Username: "Alice", Save: stored})
	if !p2.IsPoisoned() || p2.PoisonDamage() != 20 {
		t.Fatalf("relogin poison = damage %d poisoned %v, want 20/true",
			p2.PoisonDamage(), p2.IsPoisoned())
	}
	if got := p2.GetCombat().PoisonClock().Counter(); got != 10 {
		t.Fatalf("relogin clock counter = %d, want 10", got)
	}
	// Cure, snapshot, re-enter: the affliction is gone but the
	// immunity dose survives the cure (fromPlayer writes the
	// clock while !finished(), PlayerSave.ts:770-771), so the
	// stored clock still holds counter 10. Only once the dose
	// itself is spent does the snapshot clear the clock.
	game.CurePoison(&p2.Mobile)
	if err := h.coord.snapshotSave(p2, stored, "autosave"); err != nil {
		t.Fatalf("snapshot after cure: %v", err)
	}
	stored2, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save after cure: %v", err)
	}
	if stored2.PoisonDamage != 0 || stored2.Venomed {
		t.Fatalf("stored poison after cure = damage %d venomed %v, want 0/false",
			stored2.PoisonDamage, stored2.Venomed)
	}
	if stored2.PoisonClock == nil || stored2.PoisonClock.Counter != 10 {
		t.Fatalf("stored clock after cure = %+v, want counter 10 (immunity survives the cure)",
			stored2.PoisonClock)
	}
	p2.GetCombat().PoisonClock().ReduceProtection(180) // spends the dose
	if err := h.coord.snapshotSave(p2, stored2, "autosave"); err != nil {
		t.Fatalf("snapshot after dose spent: %v", err)
	}
	stored3, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save after dose spent: %v", err)
	}
	if stored3.PoisonDamage != 0 || stored3.Venomed || stored3.PoisonClock != nil {
		t.Fatalf("stored poison after dose spent = damage %d venomed %v clock %+v, want 0/false/nil",
			stored3.PoisonDamage, stored3.Venomed, stored3.PoisonClock)
	}
}

func TestSnapshotWritesLiveSkull(t *testing.T) {
	// Phase 28 (Worker B): the snapshot writes the LIVE skull
	// timer (ticks) and type (fromPlayer, PlayerSave.ts:777-778),
	// and entry loads them back (applyToPlayer, :667-668).
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	if p.IsSkulled() {
		t.Fatal("a fresh save must enter unskulled")
	}
	p.ApplySkull(game.SkullTypeRed, game.PVPSkullSeconds)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if stored.SkullTimer != 3000 || stored.SkullType.IconID != 1 {
		t.Fatalf("stored skull = timer %d icon %d, want 3000/1 (red)",
			stored.SkullTimer, stored.SkullType.IconID)
	}
	// The carried save itself is never mutated by a snapshot.
	carried, _ := h.coord.carriedSave(p)
	if carried.SkullTimer != 0 || carried.SkullType.IconID != 0 {
		t.Fatalf("carried skull = timer %d icon %d, want 0/0 (snapshot must not mutate the base)",
			carried.SkullTimer, carried.SkullType.IconID)
	}
}

func TestSnapshotSkullRoundTripsThroughRelogin(t *testing.T) {
	// Enter unskulled, gain a red skull, snapshot, re-enter from
	// the stored save: the player resumes skulled, red, with the
	// same timer — and the head icon shows red.
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	p.ApplySkull(game.SkullTypeRed, game.PVPSkullSeconds)
	p.SetSkullTimer(2999)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	p2 := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p2, &session.EnteringPlayer{Username: "Alice", Save: stored})
	if !p2.IsSkulled() || p2.SkullTimer() != 2999 || p2.SkullType() != game.SkullTypeRed {
		t.Fatalf("relogin skull = timer %d type %v skulled %v, want 2999/red/true",
			p2.SkullTimer(), p2.SkullType(), p2.IsSkulled())
	}
	if got := p2.HeadSkullIcon(); got != 1 {
		t.Fatalf("relogin head skull icon = %d, want 1 (red)", got)
	}
}
