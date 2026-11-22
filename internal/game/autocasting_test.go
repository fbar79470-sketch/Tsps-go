package game

import (
	"testing"
)

// autocastFakeSession is a fakeSession extended with the
// sub-interface and flags-range sends the autocast selector
// uses (autocastSender): subInterfaces records SendSubInterface
// calls as [targetUID, groupID, typ]; flagsRanges records
// SendInterfaceFlagsRange calls as [uid, fromSlot, toSlot, flags].
type autocastFakeSession struct {
	*fakeSession
	subInterfaces [][3]int
	flagsRanges   [][4]int
}

func newAutocastFakeSession() *autocastFakeSession {
	return &autocastFakeSession{fakeSession: newFakeSession()}
}

func (f *autocastFakeSession) SendSubInterface(targetUID, groupID, typ int) {
	f.subInterfaces = append(f.subInterfaces, [3]int{targetUID, groupID, typ})
}

func (f *autocastFakeSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	f.flagsRanges = append(f.flagsRanges, [4]int{uid, fromSlot, toSlot, flags})
}

// autocastStaffDefs maps ids to the STAFF weapon interface with
// names, for staff-equipped autocast tests. The fixture registry
// knows STAFF (category 18, styles BASH/POUND/FOCUS); the id
// itself is what the refusal/god/selector tables read, so an
// ancient staff (4675) can be equipped on the STAFF interface
// to exercise the refusal paths.
func autocastStaffDefs(ids map[int]string) *styleTestDefs {
	interfaces := make(map[int]string, len(ids))
	names := make(map[int]string, len(ids))
	for id, name := range ids {
		interfaces[id] = "STAFF"
		names[id] = name
	}
	return &styleTestDefs{
		testItemDefs: &testItemDefs{slots: func() map[int]int {
			slots := make(map[int]int, len(ids))
			for id := range ids {
				slots[id] = WeaponSlot
			}
			return slots
		}()},
		interfaces: interfaces,
		names:      names,
	}
}

// autocastFakeSpell is a fakeSpell with a configurable spell id:
// newFakeSpell always reports 1152 (Worker A gave it
// SpellID() = Wind Strike), so tests that need other slots —
// or a spell outside the table — wrap it and override SpellID.
type autocastFakeSpell struct {
	*fakeSpell
	id int
}

func (s autocastFakeSpell) SpellID() int { return s.id }

// registerAutocastTestSpells registers a fake spell under every
// STANDARD-book autocast id (the registry Worker A owns), so
// AutocastSpell(slot) resolves in tests; t.Cleanup unregisters
// them, so no test leaks a spell into another package test
// (notably TestCombatSpellRegistry, which expects unknown names
// to resolve to nil — installing the real standard spells here
// would break it). The names are test-only and never collide
// with real spell names. Ancient (31-46) ids stay unregistered
// here because the real Ancient table installs them
// (installAncientSpells, Phase 45); Arceuus (53-58) ids stay
// unregistered because that book has not landed, and
// AutocastSpell must read nil for them.
func registerAutocastTestSpells(t *testing.T) {
	t.Helper()
	for slot, id := range autocastSpellIDs {
		if slot >= 31 && slot <= 46 || slot >= 53 {
			continue
		}
		// Per-iteration copy for the cleanup closure.
		id := id
		RegisterCombatSpellNamed(id, "Autocast Test Spell", autocastFakeSpell{fakeSpell: newFakeSpell(), id: id})
		t.Cleanup(func() { UnregisterCombatSpell(id) })
	}
}

// "Staff of air") with the fixture styles and a recording
// autocast session attached.
func autocastPlayer(t *testing.T) (*Player, *autocastFakeSession) {
	t.Helper()
	installFixtureStyles(t)
	p := NewPlayer("caster", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(autocastStaffDefs(map[int]string{1381: "Staff of air"}))
	session := newAutocastFakeSession()
	p.Session = session
	equipWeapon(p, 1381)
	return p, session
}

func TestAutocastSpellResolution(t *testing.T) {
	// The standard spells register through
	// registerAutocastTestSpells (test fakes under the real
	// autocast ids; see the helper above). The Ancient book
	// registers for real (Phase 45), so its slots resolve;
	// the Arceuus slots stay nil until that book lands.
	registerAutocastTestSpells(t)
	installAncientSpells(t)
	// Standard slots resolve through the registry (Worker A);
	// ancient slots resolve through the Ancient table;
	// arceuus slots read nil until their book lands;
	// unlisted slots read nil.
	if spell := AutocastSpell(1); spell == nil {
		t.Fatal("AutocastSpell(1) = nil, want Wind Strike")
	}
	if spell := AutocastSpell(48); spell == nil {
		t.Fatal("AutocastSpell(48) = nil, want Wind Surge")
	}
	if spell := AutocastSpell(19); spell == nil {
		t.Fatal("AutocastSpell(19) = nil, want Claws of Guthix")
	}
	if spell := AutocastSpell(47); spell == nil {
		t.Fatal("AutocastSpell(47) = nil, want Iban Blast")
	}
	if spell := AutocastSpell(31); spell == nil || spell.SpellID() != 12939 {
		t.Fatalf("AutocastSpell(31) = %v, want Smoke Rush (12939)", spell)
	}
	if spell := AutocastSpell(46); spell == nil || spell.SpellID() != 12891 {
		t.Fatalf("AutocastSpell(46) = %v, want Ice Barrage (12891)", spell)
	}
	for _, slot := range []int{53, 58} {
		if spell := AutocastSpell(slot); spell != nil {
			t.Fatalf("AutocastSpell(%d) = %v, want nil (book not landed)", slot, spell)
		}
	}
	for _, slot := range []int{0, 21, 59, 999} {
		if spell := AutocastSpell(slot); spell != nil {
			t.Fatalf("AutocastSpell(%d) = %v, want nil", slot, spell)
		}
	}
}

func TestAutocastSpellIndex(t *testing.T) {
	registerAutocastTestSpells(t)
	windStrike := AutocastSpell(1)
	if windStrike == nil {
		t.Fatal("AutocastSpell(1) = nil")
	}
	if got := autocastSpellIndex(windStrike); got != 1 {
		t.Fatalf("index of Wind Strike = %d, want 1", got)
	}
	if got := autocastSpellIndex(AutocastSpell(51)); got != 51 {
		t.Fatalf("index of Fire Surge = %d, want 51", got)
	}
	// A spell outside the table reads the TS fallback 0:
	// newFakeSpell reports 1152 (Wind Strike), so the
	// non-table case needs a spell with an unlisted id.
	if got := autocastSpellIndex(autocastFakeSpell{fakeSpell: newFakeSpell(), id: 424242}); got != 0 {
		t.Fatalf("index of a non-table spell = %d, want 0", got)
	}
	if got := autocastSpellIndex(nil); got != 0 {
		t.Fatalf("index of nil = %d, want 0", got)
	}
}

func TestSelectorList(t *testing.T) {
	cases := []struct {
		book     Spellbook
		weaponID int
		want     int
	}{
		{SpellbookNormal, 1381, -1},
		{SpellbookAncient, 1381, 4675},
		{SpellbookArceuus, 1381, 9013},
		{SpellbookNormal, 4170, 4170},
		{SpellbookNormal, 21255, 4170},
		{SpellbookNormal, 11791, 11791},
		{SpellbookNormal, 12904, 11791},
		{SpellbookNormal, 22296, 22296},
		{SpellbookNormal, 24144, 24144},
		{SpellbookNormal, 8841, 8841},
		{SpellbookNormal, 1409, 1409},
		{SpellbookNormal, 27788, 27788},
		{SpellbookNormal, 27679, 27679},
	}
	for _, c := range cases {
		if got := selectorList(c.book, c.weaponID); got != c.want {
			t.Fatalf("selectorList(%v, %d) = %d, want %d", c.book, c.weaponID, got, c.want)
		}
	}
}

func TestSelectorSlots(t *testing.T) {
	standard := selectorSlotsForList(-1)
	for _, slot := range []int{1, 16, 48, 51} {
		if !standard[slot] {
			t.Fatalf("standard list missing slot %d", slot)
		}
	}
	for _, slot := range []int{0, 17, 19, 47, 52} {
		if standard[slot] {
			t.Fatalf("standard list places slot %d", slot)
		}
	}
	slayers := selectorSlotsForList(4170)
	for _, slot := range []int{17, 18, 13, 16, 48, 51} {
		if !slayers[slot] {
			t.Fatalf("slayer's list missing slot %d", slot)
		}
	}
	if slayers[1] || slayers[19] {
		t.Fatal("slayer's list places a strike or god slot")
	}
	ancient := selectorSlotsForList(4675)
	for slot := 31; slot <= 46; slot++ {
		if !ancient[slot] {
			t.Fatalf("ancient list missing slot %d", slot)
		}
	}
	if ancient[1] || ancient[53] {
		t.Fatal("ancient list places a standard or arceuus slot")
	}
	arceuus := selectorSlotsForList(9013)
	for slot := 53; slot <= 58; slot++ {
		if !arceuus[slot] {
			t.Fatalf("arceuus list missing slot %d", slot)
		}
	}
	mace := selectorSlotsForList(8841)
	if !mace[19] || mace[20] || !mace[17] {
		t.Fatal("void mace list wants 17 + 19, not 20")
	}
	sotd := selectorSlotsForList(11791)
	for _, slot := range []int{17, 18, 19, 20, 52} {
		if !sotd[slot] {
			t.Fatalf("staff of the dead list missing slot %d", slot)
		}
	}
	thammaron := selectorSlotsForList(27788)
	if !thammaron[20] || thammaron[19] {
		t.Fatal("thammaron list wants 20, not 19")
	}
	// An unknown list falls back to the standard set.
	unknown := selectorSlotsForList(12345)
	if !unknown[1] || unknown[17] {
		t.Fatal("unknown list does not fall back to standard")
	}
}

func TestWeaponRefusal(t *testing.T) {
	p, _ := autocastPlayer(t)
	// Normal book, plain staff: no refusal.
	if got := weaponRefusal(p, SpellbookNormal); got != "" {
		t.Fatalf("refusal for a plain staff on Normal = %q, want none", got)
	}
	// Ancient book, plain staff: refused.
	if got := weaponRefusal(p, SpellbookAncient); got != "You cannot autocast Ancient Magicks with that." {
		t.Fatalf("refusal for a plain staff on Ancient = %q", got)
	}
	// Ancient book, ancient staff: allowed.
	p.SetItemDefinitions(autocastStaffDefs(map[int]string{4675: "Ancient staff"}))
	equipWeapon(p, 4675)
	if got := weaponRefusal(p, SpellbookAncient); got != "" {
		t.Fatalf("refusal for an ancient staff on Ancient = %q, want none", got)
	}
	// Normal book, ancient staff: refused the other way.
	if got := weaponRefusal(p, SpellbookNormal); got != "You can only autocast ancient magicks with that." {
		t.Fatalf("refusal for an ancient staff on Normal = %q", got)
	}
	// Arceuus book, plain staff: refused; Kodai wand: allowed.
	p.SetItemDefinitions(autocastStaffDefs(map[int]string{
		1381:  "Staff of air",
		21006: "Kodai wand",
		21007: "Kodai wand (broken)",
	}))
	equipWeapon(p, 1381)
	if got := weaponRefusal(p, SpellbookArceuus); got != "You can only autocast regular offensive spells with this staff." {
		t.Fatalf("refusal for a plain staff on Arceuus = %q", got)
	}
	equipWeapon(p, 21006)
	if got := weaponRefusal(p, SpellbookArceuus); got != "" {
		t.Fatalf("refusal for a kodai wand on Arceuus = %q, want none", got)
	}
	equipWeapon(p, 21007)
	if got := weaponRefusal(p, SpellbookArceuus); got != "" {
		t.Fatalf("refusal for a charged kodai wand on Arceuus = %q, want none", got)
	}
	if !canAutocastArceuus("Kodai wand") || !canAutocastArceuus("  Skull Sceptre (i) ") || canAutocastArceuus("Staff of air") {
		t.Fatal("canAutocastArceuus name matching is wrong")
	}
}

func TestGodSpellID(t *testing.T) {
	cases := []struct {
		weaponID int
		want     int
	}{
		{2415, 1190}, {22296, 1190}, // SARADOMIN_STAFF, STAFF_OF_LIGHT
		{2416, 1191}, {24144, 1191}, {8841, 1191}, {24181, 1191},
		{2417, 1192}, {11791, 1192}, {12904, 1192}, {27788, 1192}, {27679, 1192},
		{1381, 0}, {4675, 0}, {-1, 0},
	}
	for _, c := range cases {
		if got := godSpellID(c.weaponID); got != c.want {
			t.Fatalf("godSpellID(%d) = %d, want %d", c.weaponID, got, c.want)
		}
	}
}

func TestSetAutocastWithoutStaff(t *testing.T) {
	installFixtureStyles(t)
	p := NewPlayer("caster", Location{X: 3200, Y: 3200, Z: 0})
	session := newAutocastFakeSession()
	p.Session = session
	spell := newFakeSpell()
	SetAutocast(p, spell)
	if p.GetCombat().AutoCastSpell() != spell {
		t.Fatal("spell set without a staff was not remembered")
	}
	if len(session.messages) != 1 || session.messages[0] != "Default spell set. Please equip a staff to use autocast." {
		t.Fatalf("messages = %v, want the default-spell line", session.messages)
	}
	// No staff: the indicators read off.
	if v, ok := findVarbit(session.fakeSession, 275); !ok || v != 0 {
		t.Fatalf("varbit 275 = %d/%v, want 0/true", v, ok)
	}
	if IsActive(p) {
		t.Fatal("IsActive with no staff equipped")
	}
}

func TestSetAutocastWithStaff(t *testing.T) {
	registerAutocastTestSpells(t)
	p, session := autocastPlayer(t)
	// A spell outside the autocast table (id 424242, not a
	// real spell): newFakeSpell would report 1152, so the
	// non-table case needs the wrapper.
	spell := autocastFakeSpell{fakeSpell: newFakeSpell(), id: 424242}
	SetAutocast(p, spell)
	if p.GetCombat().AutoCastSpell() != spell {
		t.Fatal("spell was not set with a staff equipped")
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none", session.messages)
	}
	if v, ok := findVarbit(session.fakeSession, 275); !ok || v != 1 {
		t.Fatalf("varbit 275 = %d/%v, want 1/true", v, ok)
	}
	// A spell outside the autocast table reads index 0.
	if v, ok := findVarbit(session.fakeSession, 276); !ok || v != 0 {
		t.Fatalf("varbit 276 = %d/%v, want 0/true", v, ok)
	}
	if v, ok := findVarp(session.fakeSession, CombatStyleVarp); !ok || v != 4 {
		t.Fatalf("varp 43 = %d/%v, want 4/true", v, ok)
	}
	if !IsActive(p) {
		t.Fatal("IsActive with a spell and a staff equipped")
	}
	// A real table spell reports its slot.
	SetAutocast(p, AutocastSpell(1))
	if v, ok := findVarbit(session.fakeSession, 276); !ok || v != 1 {
		t.Fatalf("varbit 276 = %d/%v, want 1/true", v, ok)
	}
}

func TestSetAutocastClearsUncastableWeapon(t *testing.T) {
	p, session := autocastPlayer(t)
	// An ancient staff on the Normal book cannot autocast it.
	p.SetItemDefinitions(autocastStaffDefs(map[int]string{4675: "Ancient staff"}))
	equipWeapon(p, 4675)
	SetAutocast(p, newFakeSpell())
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("a spell the weapon cannot cast was kept")
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none (cleared silently)", session.messages)
	}
}

func TestHandleWidgetActionClear(t *testing.T) {
	p, _ := autocastPlayer(t)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	if !HandleAutocastWidgetAction(p, CombatInterfaceGroup, 26, -1) {
		t.Fatal("593:26 returned false")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("593:26 did not clear autocast")
	}
	if HandleAutocastWidgetAction(p, CombatInterfaceGroup, 6, -1) {
		t.Fatal("593:6 (a style button) was claimed by autocast")
	}
	if HandleAutocastWidgetAction(p, 999, 1, 1) {
		t.Fatal("an unrelated group was claimed by autocast")
	}
	if HandleAutocastWidgetAction(nil, CombatInterfaceGroup, 26, -1) {
		t.Fatal("nil player was claimed")
	}
}

func TestHandleWidgetActionOpenSelector(t *testing.T) {
	p, session := autocastPlayer(t)
	if !HandleAutocastWidgetAction(p, CombatInterfaceGroup, 23, -1) {
		t.Fatal("593:23 returned false")
	}
	// Defensive (child 23 passes defensive=true,
	// Autocasting.ts:160): STAFF_FOCUS, child 3; varp 664
	// standard.
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "STAFF_FOCUS" {
		t.Fatalf("fight type = %+v, want STAFF_FOCUS", ft)
	}
	if v, ok := findVarp(session.fakeSession, CombatStyleVarp); !ok || v != 3 {
		t.Fatalf("varp 43 = %d/%v, want 3/true", v, ok)
	}
	if v, ok := findVarp(session.fakeSession, autocastSelectorVarp); !ok || v != -1 {
		t.Fatalf("varp 664 = %d/%v, want -1/true", v, ok)
	}
	if len(session.subInterfaces) != 1 || session.subInterfaces[0] != [3]int{(161 << 16) | 76, 201, 1} {
		t.Fatalf("sub-interfaces = %v, want [[(161<<16)|76 201 1]]", session.subInterfaces)
	}
	if len(session.flagsRanges) != 1 || session.flagsRanges[0] != [4]int{(201 << 16) | 1, 0, 64, 1 << 1} {
		t.Fatalf("flags ranges = %v, want [[(201<<16)|1 0 64 2]]", session.flagsRanges)
	}
	// Aggressive (child 28): STAFF_POUND, child 1.
	session.subInterfaces = nil
	session.flagsRanges = nil
	if !HandleAutocastWidgetAction(p, CombatInterfaceGroup, 28, -1) {
		t.Fatal("593:28 returned false")
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "STAFF_POUND" {
		t.Fatalf("fight type = %+v, want STAFF_POUND", ft)
	}
}

func TestHandleWidgetActionLunarRefusal(t *testing.T) {
	p, session := autocastPlayer(t)
	p.SetSpellbook(SpellbookLunar)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	if !HandleAutocastWidgetAction(p, CombatInterfaceGroup, 23, -1) {
		t.Fatal("593:23 on Lunar returned false")
	}
	if len(session.messages) != 1 || session.messages[0] != "You cannot autocast Lunar Magicks." {
		t.Fatalf("messages = %v, want the Lunar refusal", session.messages)
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("Lunar refusal did not clear autocast")
	}
	if len(session.subInterfaces) != 0 {
		t.Fatalf("sub-interfaces = %v, want none", session.subInterfaces)
	}
}

func TestHandleWidgetActionNoStaff(t *testing.T) {
	installFixtureStyles(t)
	p := NewPlayer("caster", Location{X: 3200, Y: 3200, Z: 0})
	session := newAutocastFakeSession()
	p.Session = session
	if !HandleAutocastWidgetAction(p, CombatInterfaceGroup, 23, -1) {
		t.Fatal("593:23 with no staff returned false")
	}
	if len(session.messages) != 1 || session.messages[0] != "You need to equip a staff to autocast spells." {
		t.Fatalf("messages = %v, want the staff line", session.messages)
	}
}

func TestHandleWidgetActionSlotPick(t *testing.T) {
	registerAutocastTestSpells(t)
	p, session := autocastPlayer(t)
	if !HandleAutocastWidgetAction(p, autocastInterfaceGroup, 1, 1) {
		t.Fatal("201:1 slot 1 returned false")
	}
	if got := autocastSpellIndex(p.GetCombat().AutoCastSpell()); got != 1 {
		t.Fatalf("autocast spell index = %d, want 1 (Wind Strike)", got)
	}
	// Picking returns to the combat interface.
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{0, 593} {
		t.Fatalf("tab interfaces = %v, want [[0 593]]", session.tabInterfaces)
	}
	// Slot 0 goes back without changing the spell.
	session.tabInterfaces = nil
	if !HandleAutocastWidgetAction(p, autocastInterfaceGroup, 1, 0) {
		t.Fatal("201:1 slot 0 returned false")
	}
	if got := autocastSpellIndex(p.GetCombat().AutoCastSpell()); got != 1 {
		t.Fatalf("autocast spell index after slot 0 = %d, want 1", got)
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{0, 593} {
		t.Fatalf("tab interfaces = %v, want [[0 593]]", session.tabInterfaces)
	}
	// A god slot the plain staff cannot choose is refused
	// (the spell is cleared, not set).
	p.GetCombat().SetAutoCastSpell(nil)
	if !HandleAutocastWidgetAction(p, autocastInterfaceGroup, 1, 19) {
		t.Fatal("201:1 slot 19 returned false")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("a god slot was chosen with a plain staff")
	}
	// An ancient slot resolves to nil until the book lands.
	if !HandleAutocastWidgetAction(p, autocastInterfaceGroup, 1, 31) {
		t.Fatal("201:1 slot 31 returned false")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("an ancient slot was chosen with no ancient spell")
	}
	// No slot: not a pick.
	if HandleAutocastWidgetAction(p, autocastInterfaceGroup, 1, -1) {
		t.Fatal("201:1 with no slot was claimed")
	}
}

func TestClearAutocastForRefusalRestoresStyle(t *testing.T) {
	p, session := autocastPlayer(t)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	// The remembered pre-autocast slot for the STAFF category
	// (18) is BASH, child 0.
	RememberCombatStyle(p, 18, 0)
	clearAutocastForRefusal(p)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("refusal clear did not clear autocast")
	}
	if ft := p.GetCombat().FightType(); ft == nil || ft.Name != "STAFF_BASH" {
		t.Fatalf("fight type = %+v, want STAFF_BASH", ft)
	}
	if v, ok := findVarp(session.fakeSession, CombatStyleVarp); !ok || v != 0 {
		t.Fatalf("varp 43 = %d/%v, want 0/true", v, ok)
	}
	// Nothing set: a refusal clear sends nothing.
	session.varps = nil
	clearAutocastForRefusal(p)
	if len(session.varps) != 0 {
		t.Fatalf("varps = %v, want none", session.varps)
	}
}

func TestClearAutocastForStyleAlreadyWired(t *testing.T) {
	p, _ := autocastPlayer(t)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	// ChangeCombatStyle (the Phase 26 style-change path)
	// clears autocast for the style: the default spell is
	// not kept.
	if !ChangeCombatStyle(p, 1) {
		t.Fatal("ChangeCombatStyle(1) returned false")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("style change did not clear autocast")
	}
}

func TestRestoreAutocastOnLogin(t *testing.T) {
	registerAutocastTestSpells(t)
	p, session := autocastPlayer(t)
	// Staff equipped: the saved spell restores.
	RestoreAutocastOnLogin(p, 1152)
	if got := autocastSpellIndex(p.GetCombat().AutoCastSpell()); got != 1 {
		t.Fatalf("restored spell index = %d, want 1 (Wind Strike)", got)
	}
	if v, ok := findVarbit(session.fakeSession, 276); !ok || v != 1 {
		t.Fatalf("varbit 276 = %d/%v, want 1/true", v, ok)
	}
	// No staff: the spell is remembered, silently.
	p.GetCombat().SetAutoCastSpell(nil)
	p.Equipment().SetItem(WeaponSlot, NewItem(-1, 0))
	session.messages = nil
	RestoreAutocastOnLogin(p, 1152)
	if got := autocastSpellIndex(p.GetCombat().AutoCastSpell()); got != 1 {
		t.Fatalf("remembered spell index = %d, want 1", got)
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none on login restore", session.messages)
	}
	// Book mismatch: the spell clears.
	p.SetSpellbook(SpellbookAncient)
	RestoreAutocastOnLogin(p, 1152)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("book-mismatched spell was not cleared")
	}
	// Unknown id and -1: nothing happens.
	p.SetSpellbook(SpellbookNormal)
	RestoreAutocastOnLogin(p, 99999)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("unknown spell id set a spell")
	}
	RestoreAutocastOnLogin(p, -1)
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("-1 set a spell")
	}
	RestoreAutocastOnLogin(nil, 1152)
}
