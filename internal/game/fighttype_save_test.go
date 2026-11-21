package game

import (
	"encoding/json"
	"testing"
)

// Fight-type save tests (phase 30, item 9): the save's
// fightType field is live in TS (PlayerSave.applyToPlayer,
// PlayerSave.ts:642; PlayerSave.fromPlayer,
// PlayerSave.ts:754), resolved against the offered styles
// (PlayerSave.resolveSavedFightType, PlayerSave.ts:255-260).

func TestResolveSavedFightType(t *testing.T) {
	installFixtureStyles(t)
	reg := CombatStyles()
	whip := reg.WeaponInterface("WHIP")
	if whip == nil {
		t.Fatal("fixture has no WHIP interface")
	}

	// The full saved object resolves to the offered style.
	full := json.RawMessage(`{"animation":1658,"parentId":43,"childId":1,"bonusType":1,"style":{},"attackSound":{"id":2720,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`)
	if got := ResolveSavedFightType(full, whip.FightTypes); got == nil || got.Name != "WHIP_LASH" {
		t.Fatalf("full saved fight type = %+v, want WHIP_LASH", got)
	}
	// A partial object ignores the missing fields (TS
	// FightType.resolve, FightType.ts:91-104): childId alone
	// still finds LASH.
	partial := json.RawMessage(`{"childId":1}`)
	if got := ResolveSavedFightType(partial, whip.FightTypes); got == nil || got.Name != "WHIP_LASH" {
		t.Fatalf("partial saved fight type = %+v, want WHIP_LASH", got)
	}
	// No offered style matches childId 9.
	if got := ResolveSavedFightType(json.RawMessage(`{"childId":9}`), whip.FightTypes); got != nil {
		t.Fatalf("unmatched saved fight type = %+v, want nil", got)
	}
	// No identifying field at all resolves to nothing.
	if got := ResolveSavedFightType(json.RawMessage(`{}`), whip.FightTypes); got != nil {
		t.Fatalf("empty saved fight type = %+v, want nil", got)
	}
	// Malformed and empty saves resolve to nothing.
	if got := ResolveSavedFightType(json.RawMessage(`{bad`), whip.FightTypes); got != nil {
		t.Fatalf("malformed saved fight type = %+v, want nil", got)
	}
	if got := ResolveSavedFightType(nil, whip.FightTypes); got != nil {
		t.Fatalf("nil saved fight type = %+v, want nil", got)
	}
}

func TestRestoreSavedFightType(t *testing.T) {
	installFixtureStyles(t)
	reg := CombatStyles()
	p, _ := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	p.GetCombat().SetFightType(reg.FightType("WHIP_FLICK"))

	// A saved LASH restores onto the live player.
	RestoreSavedFightType(p, json.RawMessage(`{"animation":1658,"parentId":43,"childId":1,"bonusType":1,"style":{},"attackSound":{"id":2720,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`))
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("restored fight type = %+v, want WHIP_LASH", ft)
	}
	// A saved style the whip does not offer leaves the
	// current fight type alone (TS setFightType ignores an
	// unresolvable value, Player.ts:1678-1683).
	RestoreSavedFightType(p, json.RawMessage(`{"animation":401,"parentId":43,"childId":0,"bonusType":2}`))
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("after unresolvable restore, fight type = %+v, want WHIP_LASH", ft)
	}
	// Empty and nil inputs are no-ops, not panics.
	RestoreSavedFightType(p, nil)
	RestoreSavedFightType(nil, json.RawMessage(`{"childId":1}`))
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("after no-op restores, fight type = %+v, want WHIP_LASH", ft)
	}
}

func TestFightTypeSaveJSON(t *testing.T) {
	// UNARMED_PUNCH encodes exactly as the new-character
	// save's default fightType JSON (persist/apply.go,
	// defaultFightTypeJSON).
	wantPunch := `{"animation":422,"parentId":43,"childId":0,"bonusType":2,"style":{},"attackSound":{"id":2566,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`
	if got := string(FightTypeSaveJSON(UnarmedPunch)); got != wantPunch {
		t.Fatalf("UnarmedPunch JSON = %s, want %s", got, wantPunch)
	}
	// WHIP_LASH encodes in the same TS field order.
	wantLash := `{"animation":1658,"parentId":43,"childId":1,"bonusType":1,"style":{},"attackSound":{"id":2720,"volume":1,"delay":0,"loopType":0},"rapid":false,"npcAnimation":-1}`
	installFixtureStyles(t)
	lash := CombatStyles().FightType("WHIP_LASH")
	if got := string(FightTypeSaveJSON(lash)); got != wantLash {
		t.Fatalf("WHIP_LASH JSON = %s, want %s", got, wantLash)
	}
	// Round trip: the encoded value resolves back to the
	// same offered style.
	if got := ResolveSavedFightType(FightTypeSaveJSON(lash), CombatStyles().WeaponInterface("WHIP").FightTypes); got != lash {
		t.Fatalf("round trip resolved to %+v, want WHIP_LASH", got)
	}
	// A nil fight type encodes as the UNARMED_PUNCH default.
	if got := string(FightTypeSaveJSON(nil)); got != wantPunch {
		t.Fatalf("nil fight type JSON = %s, want %s", got, wantPunch)
	}
}
