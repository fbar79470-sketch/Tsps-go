package game

import (
	"testing"
)

// installFixtureStyles installs the fixture registry for one
// test and restores the previous registry afterwards.
func installFixtureStyles(t *testing.T) {
	t.Helper()
	previous := InstallCombatStyles(fixtureRegistry(t))
	t.Cleanup(func() { InstallCombatStyles(previous) })
}

// stylePlayer builds a player with the fixture defs and a
// recording fake session attached.
func stylePlayer(defs *styleTestDefs) (*Player, *fakeSession) {
	p := NewPlayer("styler", Location{X: 3200, Y: 3200, Z: 0})
	if defs != nil {
		p.SetItemDefinitions(defs)
	}
	session := newFakeSession()
	p.Session = session
	return p, session
}

func whipDefs() *styleTestDefs {
	return &styleTestDefs{
		testItemDefs: &testItemDefs{slots: map[int]int{4151: WeaponSlot, 1381: WeaponSlot}},
		interfaces:   map[int]string{4151: "WHIP", 1381: "STAFF"},
		names:        map[int]string{4151: "Abyssal whip", 1381: "Staff of air"},
	}
}

func equipWeapon(p *Player, id int) {
	p.Equipment().SetItem(WeaponSlot, NewItem(id, 1))
}

func findVarp(session *fakeSession, id int) (int, bool) {
	for i := len(session.varps) - 1; i >= 0; i-- {
		if session.varps[i][0] == id {
			return session.varps[i][1], true
		}
	}
	return 0, false
}

func findVarbit(session *fakeSession, id int) (int, bool) {
	for i := len(session.varbits) - 1; i >= 0; i-- {
		if session.varbits[i][0] == id {
			return session.varbits[i][1], true
		}
	}
	return 0, false
}

func TestCurrentWeaponInterface(t *testing.T) {
	installFixtureStyles(t)
	if got := CurrentWeaponInterface(nil); got == nil || got.Name != "UNARMED" {
		t.Fatalf("nil player interface = %+v", got)
	}
	p, _ := stylePlayer(nil)
	if got := CurrentWeaponInterface(p); got == nil || got.Name != "UNARMED" {
		t.Fatalf("bare player interface = %+v", got)
	}
	p, _ = stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	if got := CurrentWeaponInterface(p); got == nil || got.Name != "WHIP" {
		t.Fatalf("whip interface = %+v", got)
	}
	// Unknown interface name falls back to UNARMED.
	p.SetItemDefinitions(&styleTestDefs{
		testItemDefs: &testItemDefs{slots: map[int]int{4151: WeaponSlot}},
		interfaces:   map[int]string{4151: "NOPE"},
	})
	if got := CurrentWeaponInterface(p); got == nil || got.Name != "UNARMED" {
		t.Fatalf("unknown interface name = %+v", got)
	}
}

func TestAssignUnarmed(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(nil)
	AssignWeaponInterface(p)
	if v, ok := findVarbit(session, WeaponCategoryVarbit); !ok || v != 0 {
		t.Fatalf("varbit 357 = %d/%v, want 0/true", v, ok)
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{0, 5855} {
		t.Fatalf("tab interfaces = %v, want [[0 5855]]", session.tabInterfaces)
	}
	if v, ok := findVarp(session, AutoRetaliateVarp); !ok || v != 0 {
		t.Fatalf("varp 172 = %d/%v, want 0/true (auto-retaliate on)", v, ok)
	}
	if v, ok := findVarp(session, CombatStyleVarp); !ok || v != 0 {
		t.Fatalf("varp 43 = %d/%v, want 0/true", v, ok)
	}
	if len(session.strings) != 1 || session.strings[0] != [2]any{"Unarmed", 5857} {
		t.Fatalf("strings = %v, want [[Unarmed 5857]]", session.strings)
	}
	if len(session.displayStates) != 0 {
		t.Fatalf("display states = %v, want none (no special bar)", session.displayStates)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_PUNCH" {
		t.Fatalf("fight type = %+v, want UNARMED_PUNCH", ft)
	}
}

func TestAssignWhip(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	AssignWeaponInterface(p)
	if v, ok := findVarbit(session, WeaponCategoryVarbit); !ok || v != 20 {
		t.Fatalf("varbit 357 = %d/%v, want 20/true", v, ok)
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{0, 12290} {
		t.Fatalf("tab interfaces = %v, want [[0 12290]]", session.tabInterfaces)
	}
	// WHIP writes "Weapon: " on the line before the name line.
	if len(session.strings) != 2 || session.strings[0] != [2]any{"Weapon: ", 12292} || session.strings[1] != [2]any{"Abyssal whip", 12293} {
		t.Fatalf("strings = %v", session.strings)
	}
	if v, ok := findVarp(session, CombatStyleVarp); !ok || v != 0 {
		t.Fatalf("varp 43 = %d/%v, want 0/true (WHIP_FLICK)", v, ok)
	}
	// No special registered for the whip in tests: the bar hides.
	if len(session.displayStates) != 1 || session.displayStates[0] != [2]any{12323, true} {
		t.Fatalf("display states = %v, want [[12323 true]]", session.displayStates)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_FLICK" {
		t.Fatalf("fight type = %+v, want WHIP_FLICK", ft)
	}
}

func TestAssignStaff(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(whipDefs())
	equipWeapon(p, 1381)
	AssignWeaponInterface(p)
	if v, ok := findVarbit(session, WeaponCategoryVarbit); !ok || v != 18 {
		t.Fatalf("varbit 357 = %d/%v, want 18/true", v, ok)
	}
	// STAFF writes only the name line (no "Weapon: " prefix).
	if len(session.strings) != 1 || session.strings[0] != [2]any{"Staff of air", 355} {
		t.Fatalf("strings = %v, want [[Staff of air 355]]", session.strings)
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "STAFF_BASH" {
		t.Fatalf("fight type = %+v, want STAFF_BASH", ft)
	}
}

func TestChangeCombatStyle(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	AssignWeaponInterface(p)
	if !ChangeCombatStyle(p, 1) {
		t.Fatal("ChangeCombatStyle(1) must succeed (WHIP_LASH)")
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("fight type = %+v, want WHIP_LASH", ft)
	}
	if v, ok := findVarp(session, CombatStyleVarp); !ok || v != 1 {
		t.Fatalf("varp 43 = %d/%v, want 1/true", v, ok)
	}
	// No child 2 on the whip; slots outside 0..3 refuse.
	for _, slot := range []int{-1, 2, 4, 99} {
		if ChangeCombatStyle(p, slot) {
			t.Fatalf("ChangeCombatStyle(%d) must fail", slot)
		}
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("failed change moved fight type to %+v", ft)
	}
	if ChangeCombatStyle(nil, 0) {
		t.Fatal("nil player must fail")
	}
}

func TestStyleMemoryAcrossEquip(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	AssignWeaponInterface(p)
	// Pick LASH (slot 1), then unequip: unarmed remembers its
	// own category-0 slot; re-equipping the whip restores LASH.
	if !ChangeCombatStyle(p, 1) {
		t.Fatal("ChangeCombatStyle(1) must succeed")
	}
	p.Equipment().SetItem(WeaponSlot, NewItem(-1, 0))
	AssignWeaponInterface(p)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "UNARMED_PUNCH" {
		t.Fatalf("after unequip fight type = %+v", ft)
	}
	equipWeapon(p, 4151)
	AssignWeaponInterface(p)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("after re-equip fight type = %+v, want WHIP_LASH (remembered)", ft)
	}
	if v, ok := findVarp(session, CombatStyleVarp); !ok || v != 1 {
		t.Fatalf("varp 43 after re-equip = %d/%v, want 1/true", v, ok)
	}
}

func TestAssignKeepsCurrentStyleWhenUnremembered(t *testing.T) {
	installFixtureStyles(t)
	p, _ := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	// No memory for category 20, but the current fight type
	// is an offered style: assign keeps it (TS second rule).
	reg := CombatStyles()
	p.GetCombat().SetFightType(reg.FightType("WHIP_DEFLECT"))
	AssignWeaponInterface(p)
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_DEFLECT" {
		t.Fatalf("fight type = %+v, want WHIP_DEFLECT (kept)", ft)
	}
}

func TestAutoRetaliate(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(nil)
	if !p.GetCombat().AutoRetaliate() {
		t.Fatal("auto-retaliate must default on")
	}
	ApplyAutoRetaliate(p, 1)
	if p.GetCombat().AutoRetaliate() {
		t.Fatal("varp 172 = 1 must disable auto-retaliate")
	}
	if v, ok := findVarp(session, AutoRetaliateVarp); !ok || v != 1 {
		t.Fatalf("varp 172 echo = %d/%v, want 1/true", v, ok)
	}
	ApplyAutoRetaliate(p, 0)
	if !p.GetCombat().AutoRetaliate() {
		t.Fatal("varp 172 = 0 must enable auto-retaliate")
	}
	if v, ok := findVarp(session, AutoRetaliateVarp); !ok || v != 0 {
		t.Fatalf("varp 172 echo = %d/%v, want 0/true", v, ok)
	}
	// The 593:32 button toggles.
	if !HandleAutoRetaliateButton(p, 593, 32) {
		t.Fatal("auto-retaliate button must claim the click")
	}
	if p.GetCombat().AutoRetaliate() {
		t.Fatal("button must toggle auto-retaliate off")
	}
	if HandleAutoRetaliateButton(p, 593, 31) || HandleAutoRetaliateButton(p, 592, 32) {
		t.Fatal("wrong group/child must not claim the click")
	}
	ApplyAutoRetaliate(nil, 0)
}

func TestHandleStyleButton(t *testing.T) {
	installFixtureStyles(t)
	p, _ := stylePlayer(whipDefs())
	equipWeapon(p, 4151)
	AssignWeaponInterface(p)
	// WHIP has no child 2: the click claims nothing and the
	// style stays. (The button is claimed only when the slot
	// resolves; a 593 button for a missing slot falls through
	// in TS too — handleStyleButton returns changeCombatStyle's
	// false.)
	if HandleStyleButton(p, 593, 14) {
		t.Fatal("style button 593:14 must not claim the click (no slot 2 on the whip)")
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_FLICK" {
		t.Fatalf("fight type = %+v, want WHIP_FLICK (unchanged)", ft)
	}
	if !HandleStyleButton(p, 593, 10) {
		t.Fatal("style button 593:10 must claim the click (slot 1)")
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "WHIP_LASH" {
		t.Fatalf("fight type = %+v, want WHIP_LASH", ft)
	}
	if HandleStyleButton(p, 593, 7) || HandleStyleButton(p, 592, 10) {
		t.Fatal("wrong group/child must not claim the click")
	}
}

func TestRefreshAutocastIndicators(t *testing.T) {
	installFixtureStyles(t)
	p, session := stylePlayer(whipDefs())
	// No spell: all three varbits 0, no varp 43 = 4.
	RefreshAutocastIndicators(p)
	for _, id := range []int{275, 276, 2668} {
		if v, ok := findVarbit(session, id); !ok || v != 0 {
			t.Fatalf("varbit %d = %d/%v, want 0/true", id, v, ok)
		}
	}
	if v, ok := findVarp(session, CombatStyleVarp); ok && v == 4 {
		t.Fatal("varp 43 = 4 must not send without an autocast spell")
	}
	// A spell with a staff: set + its table index; defensive
	// adds 2668. The fake spell reports id 1152 (Wind Strike,
	// Worker A gave fakeSpell SpellID()), so its index is 1 —
	// the Phase 26 expectation of 0 predates the SpellID and
	// the Phase 27 index lookup (RefreshAutocastIndicators
	// used to hardcode 0).
	equipWeapon(p, 1381)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	session.varbits = nil
	session.varps = nil
	RefreshAutocastIndicators(p)
	if v, ok := findVarbit(session, 275); !ok || v != 1 {
		t.Fatalf("varbit 275 = %d/%v, want 1/true", v, ok)
	}
	if v, ok := findVarbit(session, 276); !ok || v != 1 {
		t.Fatalf("varbit 276 = %d/%v, want 1/true (Wind Strike)", v, ok)
	}
	if v, ok := findVarbit(session, 2668); !ok || v != 0 {
		t.Fatalf("varbit 2668 = %d/%v, want 0/true (accurate staff)", v, ok)
	}
	if v, ok := findVarp(session, CombatStyleVarp); !ok || v != 4 {
		t.Fatalf("varp 43 = %d/%v, want 4/true (autocast)", v, ok)
	}
	// Defensive staff style: 2668 turns on.
	p.GetCombat().SetFightType(CombatStyles().FightType("STAFF_FOCUS"))
	session.varbits = nil
	RefreshAutocastIndicators(p)
	if v, ok := findVarbit(session, 2668); !ok || v != 1 {
		t.Fatalf("varbit 2668 = %d/%v, want 1/true (defensive autocast)", v, ok)
	}
	// A spell without a staff does not count.
	equipWeapon(p, 4151)
	session.varbits = nil
	session.varps = nil
	RefreshAutocastIndicators(p)
	if v, ok := findVarbit(session, 275); !ok || v != 0 {
		t.Fatalf("varbit 275 with whip = %d/%v, want 0/true", v, ok)
	}
	// Choosing a style clears the spell (clearForStyle).
	equipWeapon(p, 1381)
	AssignWeaponInterface(p)
	if p.GetCombat().AutoCastSpell() == nil {
		t.Fatal("assign must not clear the autocast spell")
	}
	if !ChangeCombatStyle(p, 0) {
		t.Fatal("ChangeCombatStyle(0) must succeed")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("ChangeCombatStyle must clear the autocast spell")
	}
	RefreshAutocastIndicators(nil)
	ClearAutocastForStyle(nil)
}
