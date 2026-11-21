package main

import (
	"encoding/json"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
	"tsps-go/internal/session"
)

// Fight-type entry-load / snapshot tests (phase 30, item 9):
// applyEntryFightType in combatstylewiring.go and the
// FightType snapshot write in savewiring.go. Fixtures only: a
// hand-built player and a hand-built carried save. No
// combat-styles registry is installed, so the UNARMED
// fallback registry answers (UNARMED_PUNCH/KICK/BLOCK); the
// whip-interface branch is covered in the game package
// (TestRestoreSavedFightType).

func TestApplyEntryFightType(t *testing.T) {
	p := game.NewPlayer("styler", game.Location{X: 3200, Y: 3200, Z: 0})
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_PUNCH" {
		t.Fatalf("fresh player fight type = %+v, want UNARMED_PUNCH", ft)
	}
	// A saved KICK resolves against the unarmed offered styles.
	carried := &persist.PlayerSave{
		FightType: json.RawMessage(`{"animation":423,"parentId":43,"childId":1,"bonusType":2,"style":{},"attackSound":{"id":2566,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`),
	}
	applyEntryFightType(p, carried)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type = %+v, want UNARMED_KICK", ft)
	}
	// A saved style the wielded (unarmed) weapon does not
	// offer leaves the current fight type alone.
	carried.FightType = json.RawMessage(`{"animation":1658,"parentId":43,"childId":1,"bonusType":1}`)
	applyEntryFightType(p, carried)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type after unresolvable restore = %+v, want UNARMED_KICK", ft)
	}
	// Nil guards: no panic, no change.
	applyEntryFightType(nil, carried)
	applyEntryFightType(p, nil)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("fight type after nil guards = %+v, want UNARMED_KICK", ft)
	}
}

// TestSnapshotWritesLiveFightType covers the phase 30
// save-split fix: fightType is owned state
// (PlayerSave.ts:754), so a snapshot writes the live combat
// fight type instead of preserving the carried value.
func TestSnapshotWritesLiveFightType(t *testing.T) {
	h := newManualHarness(t, 1000, 500)
	save := saveAtPosition("Alice", 3100, 3530, 0, true)
	p, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: save, PasswordHash: "h", CredentialName: "Alice"})
	kick := game.CombatStyles().FightType("UNARMED_KICK")
	if kick == nil {
		t.Fatal("fallback registry has no UNARMED_KICK")
	}
	p.GetCombat().SetFightType(kick)
	// Remember the pick per category, as a combat-tab style
	// click would (WeaponInterfaceManager.changeCombatStyle):
	// the login assign reads the remembered slot first, so a
	// snapshot that only wrote the fight type without the
	// remembered slot would relogin as the old style.
	game.RememberCombatStyle(p, 0, kick.ChildID)
	if err := h.coord.snapshot(p, "autosave"); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	stored, err := h.store.inner.Load("Alice", persist.ProfileMain)
	if err != nil {
		t.Fatalf("load stored save: %v", err)
	}
	if string(stored.FightType) != string(game.FightTypeSaveJSON(kick)) {
		t.Fatalf("stored fightType = %s, want %s (live kick)", stored.FightType, game.FightTypeSaveJSON(kick))
	}
	// Relogin restores the snapshotted style: the carried
	// KICK resolves against the unarmed offered styles.
	p2, _ := h.enter(t, &session.EnteringPlayer{Username: "Alice", Save: stored, PasswordHash: "h", CredentialName: "Alice"})
	if ft := p2.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_KICK" {
		t.Fatalf("relogin fight type = %+v, want UNARMED_KICK", ft)
	}
}
