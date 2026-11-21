package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// hasPersistentKey reports whether key is in the live persistent
// registry, via the sorted key list.
func hasPersistentKey(t *testing.T, key string) bool {
	t.Helper()
	for _, k := range persist.PersistentAttributeKeys() {
		if k == key {
			return true
		}
	}
	return false
}

func TestEntryLoadsSaveAttributes(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Attributes["shop:points"] = json.RawMessage(`5`)
	save.Attributes["killstreaks:total-kills"] = json.RawMessage(`12`)
	// A session-only key and an unknown key in the save must NOT load:
	// entry loads only persistent registry keys.
	save.Attributes["session:thing"] = json.RawMessage(`"live-only"`)
	save.Attributes["unknown:key"] = json.RawMessage(`1`)
	// A camelCase key migrates first (MigrateForEntry), then loads.
	save.Attributes["ringOfRecoil:damage"] = json.RawMessage(`3`)
	// No bank:quantity-mode in the save: the default (0) must fill it.

	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})

	if got := p.GetAttribute("shop:points"); got != float64(5) {
		t.Fatalf("shop:points = %v (%T), want float64 5", got, got)
	}
	if got := p.GetAttribute("killstreaks:total-kills"); got != float64(12) {
		t.Fatalf("killstreaks:total-kills = %v, want 12", got)
	}
	if got := p.GetAttribute("ring-of-recoil:damage"); got != float64(3) {
		t.Fatalf("migrated ring-of-recoil:damage = %v, want 3", got)
	}
	if got := p.GetAttribute("bank:quantity-mode"); got != float64(0) {
		t.Fatalf("bank:quantity-mode default = %v, want float64 0", got)
	}
	if p.HasAttribute("session:thing") {
		t.Fatal("session-only key from the save must not load live")
	}
	if p.HasAttribute("unknown:key") {
		t.Fatal("unknown key from the save must not load live")
	}
}

func TestEntryNewAccountAppliesDefaults(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Bob", Profile: "main", PasswordHash: "h"})

	// The new-account path applies defaults only (NetworkBuilder.ts:1669).
	if got := p.GetAttribute("ring-of-recoil:damage"); got != float64(0) {
		t.Fatalf("ring-of-recoil:damage = %v, want default 0", got)
	}
	tokens, ok := p.GetAttribute("login:tokens").([]any)
	if !ok || len(tokens) != 0 {
		t.Fatalf("login:tokens = %v (%T), want empty array default", p.GetAttribute("login:tokens"), p.GetAttribute("login:tokens"))
	}
	if got := p.GetAttribute("bank:placeholders"); got != false {
		t.Fatalf("bank:placeholders = %v, want default false", got)
	}
	if got := p.GetAttribute("combat:style-by-category"); !isEmptyObject(got) {
		t.Fatalf("combat:style-by-category = %v, want default {}", got)
	}
	if p.HasAttribute("shop:points") {
		t.Fatal("shop:points has no default and must be absent for a new account")
	}
}

func isEmptyObject(v any) bool {
	m, ok := v.(map[string]any)
	return ok && len(m) == 0
}

func TestEntryTransientClientAttributes(t *testing.T) {
	t.Run("desktop sets neither", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
		if p.HasAttribute("mobile-client") || p.HasAttribute("native-client") {
			t.Fatal("desktop login must set no transient client attribute")
		}
	})
	t.Run("mobile sets mobile-client", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h", ClientType: 1})
		if got := p.GetAttribute("mobile-client"); got != true {
			t.Fatalf("mobile-client = %v, want true", got)
		}
		if p.HasAttribute("native-client") {
			t.Fatal("mobile login without the native bit must not set native-client")
		}
	})
	t.Run("native capability sets native-client", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h", Capabilities: protocol.HandshakeCapabilityNativeClient})
		if got := p.GetAttribute("native-client"); got != true {
			t.Fatalf("native-client = %v, want true", got)
		}
		if p.HasAttribute("mobile-client") {
			t.Fatal("native login without clientType 1 must not set mobile-client")
		}
	})
	t.Run("both facts set both", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h", ClientType: 1, Capabilities: protocol.HandshakeCapabilityNativeClient})
		if p.GetAttribute("mobile-client") != true || p.GetAttribute("native-client") != true {
			t.Fatal("both transient attributes must be set")
		}
	})
	t.Run("transient keys are not persistent", func(t *testing.T) {
		if hasPersistentKey(t, "mobile-client") || hasPersistentKey(t, "native-client") {
			t.Fatal("transient client attributes must never be registered persistent")
		}
	})
}

func TestEntryCorruptAttributeSkipped(t *testing.T) {
	// prepareCarriedSave exercised directly: a hand-built save can
	// hold attribute bytes no store would yield, and a number too
	// large for float64 (1e999) parses as RawMessage yet fails
	// decoding to any. Neither may fail the login; the readable
	// keys still load and the corrupt key falls back to its
	// default (ring-of-recoil:damage defaults to 0).
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Attributes["shop:points"] = json.RawMessage(`1e999`)
	save.Attributes["ring-of-recoil:damage"] = json.RawMessage(`{invalid`)
	save.Attributes["killstreaks:total-kills"] = json.RawMessage(`9`)
	p := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h"})

	if p.HasAttribute("shop:points") {
		t.Fatal("undecodable shop:points must be skipped")
	}
	if got := p.GetAttribute("ring-of-recoil:damage"); got != float64(0) {
		t.Fatalf("corrupt ring-of-recoil:damage = %v, want the 0 default", got)
	}
	if got := p.GetAttribute("killstreaks:total-kills"); got != float64(9) {
		t.Fatalf("readable key after corrupt keys = %v, want 9", got)
	}
	logged := h.logBuf.String()
	if !strings.Contains(logged, "shop:points") || !strings.Contains(logged, "ring-of-recoil:damage") {
		t.Fatalf("log %q must name both dropped keys", logged)
	}
}

func TestEntryLoginTokensLiveAttribute(t *testing.T) {
	now := time.Now()
	t.Run("primary overwrites stale save tokens live", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		save := saveAtPosition("Alice", 3100, 3530, 0, true)
		// Stale save entries: the pending (rotated) entries must win
		// live, per the NetworkBuilder.ts:1672-1674 order.
		stale := persist.TokenEntry{Hash: strings.Repeat("b", 64), LastUsed: now}
		if err := save.SetTokens([]persist.TokenEntry{stale}); err != nil {
			t.Fatalf("SetTokens: %v", err)
		}
		pending := testTokenEntry(now) // hash is 'a' * 64
		p, _ := h.enter(t, &session.EnteringPlayer{
			Username: "Alice", Profile: "main", Save: save,
			CredentialName: "Alice",
			LoginTokens:    []persist.TokenEntry{pending},
		})
		entries, ok := p.GetAttribute("login:tokens").([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("live login:tokens = %v, want one entry", p.GetAttribute("login:tokens"))
		}
		entry, ok := entries[0].(map[string]any)
		if !ok || entry["hash"] != strings.Repeat("a", 64) {
			t.Fatalf("live login:tokens entry = %v, want the pending hash", entries[0])
		}
	})
	t.Run("alt keeps save tokens live", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		save := saveAtPosition("Alicia", 3100, 3530, 0, true)
		stale := persist.TokenEntry{Hash: strings.Repeat("b", 64), LastUsed: now}
		if err := save.SetTokens([]persist.TokenEntry{stale}); err != nil {
			t.Fatalf("SetTokens: %v", err)
		}
		p, _ := h.enter(t, &session.EnteringPlayer{
			Username: "Alicia", Profile: "main", Save: save,
			CredentialName: "Alice",
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		})
		entries, ok := p.GetAttribute("login:tokens").([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("live login:tokens = %v, want the save entry", p.GetAttribute("login:tokens"))
		}
		entry, ok := entries[0].(map[string]any)
		if !ok || entry["hash"] != strings.Repeat("b", 64) {
			t.Fatalf("live login:tokens entry = %v, want the save hash", entries[0])
		}
	})
	t.Run("new character primary holds folded tokens live", func(t *testing.T) {
		h := newManualHarness(t, 1000, 500)
		p, _ := h.enter(t, &session.EnteringPlayer{
			Username: "Bob", Profile: "main", PasswordHash: "h",
			CredentialName: "Bob",
			LoginTokens:    []persist.TokenEntry{testTokenEntry(now)},
		})
		entries, ok := p.GetAttribute("login:tokens").([]any)
		if !ok || len(entries) != 1 {
			t.Fatalf("live login:tokens = %v, want the folded entry", p.GetAttribute("login:tokens"))
		}
	})
}

func TestSnapshotRebuildsAttributes(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3089, 3524, 0, true)
	save.Attributes["shop:points"] = json.RawMessage(`5`)
	p, index := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})

	// In-session changes: a persistent value moves, a session-only
	// attribute appears, and a default-equal value is set live.
	p.SetAttribute("shop:points", 42)
	p.SetAttribute("session:thing", "gone-after-relogin")
	p.SetAttribute("ring-of-recoil:damage", 0)
	p.SetLocation(game.Location{X: 3110, Y: 3540, Z: 0})
	h.remove(t, index)

	stored, err := h.store.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if string(stored.Attributes["shop:points"]) != `42` {
		t.Fatalf("persisted shop:points = %s, want 42", stored.Attributes["shop:points"])
	}
	if _, ok := stored.Attributes["ring-of-recoil:damage"]; ok {
		t.Fatal("default-equal ring-of-recoil:damage must be stripped from the snapshot")
	}
	if _, ok := stored.Attributes["session:thing"]; ok {
		t.Fatal("session-only attribute must never reach a save")
	}
	if stored.Position == nil || *stored.Position != (persist.Location{X: 3110, Y: 3540, Z: 0}) {
		t.Fatalf("position = %+v, want the moved tile (other fields untouched)", stored.Position)
	}
	// Relogin restores the persisted value and drops the session-only
	// attribute: entry loads only what the save holds.
	p2, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: stored, PasswordHash: "h"})
	if got := p2.GetAttribute("shop:points"); got != float64(42) {
		t.Fatalf("relogin shop:points = %v, want 42", got)
	}
	if p2.HasAttribute("session:thing") {
		t.Fatal("session-only attribute survived a relogin")
	}
}

func TestSnapshotAttributesEmptyMapWhenOnlyDefaults(t *testing.T) {
	// A new account holds only default-valued attributes live — except
	// the aggression-tolerance state, which the first Process tick
	// seeds (AggressionTolerance.update, called from Player.Process
	// per Player.ts:540): the snapshot writes an attributes object,
	// non-nil, holding exactly that seeded state at the spawn tile.
	// The tick count varies with harness scheduling (later ticks
	// increment it while the player stays inside the regions), so
	// the test pins the centres and bounds the ticks, not an exact
	// count.
	h := newManualHarness(t, 1000, 500)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	carried, _ := h.coord.carriedSave(p)
	snap, err := h.coord.buildSnapshot(p, carried)
	if err != nil {
		t.Fatalf("buildSnapshot: %v", err)
	}
	if snap.Attributes == nil {
		t.Fatal("snapshot attributes must be non-nil even when empty")
	}
	if len(snap.Attributes) != 1 {
		t.Fatalf("snapshot attributes = %v, want exactly the tolerance seed", snap.Attributes)
	}
	raw, ok := snap.Attributes["npc-aggression:tolerance"]
	if !ok {
		t.Fatalf("snapshot attributes = %v, want the npc-aggression:tolerance seed", snap.Attributes)
	}
	var state struct {
		Older struct{ X, Y, Z int } `json:"older"`
		Newer struct{ X, Y, Z int } `json:"newer"`
		Ticks int                   `json:"ticks"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatalf("tolerance state unmarshal: %v", err)
	}
	spawn := struct{ X, Y, Z int }{3089, 3524, 0}
	if state.Older != spawn || state.Newer != spawn {
		t.Fatalf("tolerance centres = %+v/%+v, want both %v", state.Older, state.Newer, spawn)
	}
	if state.Ticks < 0 || state.Ticks > 1000 {
		t.Fatalf("tolerance ticks = %d, want 0..1000", state.Ticks)
	}
}

func TestSnapshotMarshalErrorSkipsWrite(t *testing.T) {
	// A live attribute value that fails the cloneAttribute rule
	// (here: a func) fails the whole snapshot, as TS cloneAttribute
	// throwing fails fromPlayer: the coordinator logs per player and
	// writes nothing, and the player is unaffected.
	h := newManualHarness(t, 1000, 500)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", PasswordHash: "h"})
	p.SetAttribute("shop:points", func() {})
	if err := h.coord.snapshot(p, "autosave"); err == nil {
		t.Fatal("snapshot with a non-JSON-compatible live value must fail")
	}
	if got := h.store.snapshot(); len(got) != 0 {
		t.Fatalf("store writes = %v, want none", got)
	}
	// The value can be repaired in-session; the next snapshot succeeds.
	p.SetAttribute("shop:points", 7)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot after repair: %v", err)
	}
	if got := h.store.snapshot(); len(got) != 1 {
		t.Fatalf("store writes = %v, want one", got)
	}
	stored, err := h.store.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if string(stored.Attributes["shop:points"]) != `7` {
		t.Fatalf("stored shop:points = %s, want 7", stored.Attributes["shop:points"])
	}
}

func TestEntryAppearanceReadsUnaffectedByAttributeLoad(t *testing.T) {
	// The appearance entry init reads pronouns/skin unlocks from
	// save.Attributes directly (applyEntryLiveState). Attribute
	// loading must not mutate the save map, so those reads still see
	// the migrated values. (The appearance lines themselves are
	// untouched by phase 11; this pins the interaction.)
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	save.Attributes["appearance:pronouns"] = json.RawMessage(`2`)
	for _, entry := range game.SkinUnlocks {
		save.Attributes[entry.Attribute] = json.RawMessage(`true`)
	}
	p := game.NewPlayer("Alice", h.spawn)
	h.adapter.prepareCarriedSave(p, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h"})
	if got := p.Appearance().Pronouns(); got != 2 {
		t.Fatalf("pronouns = %d, want 2", got)
	}
	for _, entry := range game.SkinUnlocks {
		for _, colour := range entry.Colours {
			if !p.Appearance().IsSkinColourUnlocked(colour) {
				t.Fatalf("skin colour %d (unlock %q) not applied", colour, entry.Attribute)
			}
		}
	}
	// The save map itself is unmutated by entry application.
	if string(save.Attributes["appearance:pronouns"]) != `2` {
		t.Fatal("entry application mutated save.Attributes")
	}
}

// TestSpecPercentageSaveRoundTrip pins the phase-16 save
// split for special energy: a carried save's specPercentage
// restores into live combat on entry (PlayerSave.ts:655),
// and a snapshot writes the live value back
// (PlayerSave.ts:763). FightType and autocast stay preserved
// verbatim — nothing in Go sets them yet — and are covered
// by the persist-level split tests.
func TestSpecPercentageSaveRoundTrip(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3089, 3524, 0, true)
	save.SpecPercentage = 40
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Profile: "main", Save: save, PasswordHash: "h"})
	if got := p.GetCombat().SpecialPercentage(); got != 40 {
		t.Fatalf("live spec after entry = %d, want 40", got)
	}
	// Draining live energy must reach the snapshot.
	p.GetCombat().SetSpecialPercentage(25)
	carried, _ := h.coord.carriedSave(p)
	snap, err := h.coord.buildSnapshot(p, carried)
	if err != nil {
		t.Fatalf("buildSnapshot: %v", err)
	}
	if snap.SpecPercentage != 25 {
		t.Fatalf("snapshot specPercentage = %d, want 25", snap.SpecPercentage)
	}
}
