package game

import (
	"encoding/json"
	"testing"
)

func TestEquipmentSlotConstants(t *testing.T) {
	want := map[string]int{
		"HEAD": 0, "CAPE": 1, "AMULET": 2, "WEAPON": 3, "BODY": 4,
		"SHIELD": 5, "LEG": 7, "HANDS": 9, "FEET": 10, "RING": 12,
		"AMMUNITION": 13,
	}
	got := map[string]int{
		"HEAD": HeadSlot, "CAPE": CapeSlot, "AMULET": AmuletSlot,
		"WEAPON": WeaponSlot, "BODY": BodySlot, "SHIELD": ShieldSlot,
		"LEG": LegSlot, "HANDS": HandsSlot, "FEET": FeetSlot,
		"RING": RingSlot, "AMMUNITION": AmmunitionSlot,
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("slot %s = %d, want %d", name, got[name], w)
		}
	}
	if EquipmentInventoryInterfaceID != 1688 || EquipmentScreenInterfaceID != 84 || WornInventoryID != 94 {
		t.Fatalf("interface ids: %d %d %d", EquipmentInventoryInterfaceID, EquipmentScreenInterfaceID, WornInventoryID)
	}
}

func TestEquipmentGetWeaponAndSet(t *testing.T) {
	p := testPlayerWithDefs(nil)
	eq := p.Equipment()
	eq.Set(WeaponSlot, NewItem(4151, 1))
	if eq.GetWeapon().ID() != 4151 {
		t.Fatalf("GetWeapon = %d", eq.GetWeapon().ID())
	}
	// GetWeapon returns a copy.
	w := eq.GetWeapon()
	w.SetID(1)
	if eq.GetWeapon().ID() != 4151 {
		t.Fatal("mutating GetWeapon result mutated the container")
	}
}

func TestEquipmentRefreshFlagsAppearance(t *testing.T) {
	p := testPlayerWithDefs(nil)
	eq := p.Equipment()
	hookCalls := 0
	eq.SetRefreshHook(func(ev RefreshEvent) {
		hookCalls++
		if ev.Kind != ContainerEquipment {
			t.Errorf("refresh kind = %v", ev.Kind)
		}
	})
	changed := 0
	p.OnEquipmentChanged = func(pl *Player) { changed++ }
	eq.RefreshItems()
	if hookCalls != 1 {
		t.Fatalf("refresh hook called %d times", hookCalls)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Fatal("equipment refresh did not flag APPEARANCE")
	}
	if changed != 1 {
		t.Fatalf("OnEquipmentChanged called %d times", changed)
	}
	// Without a hook or coordinator, the flag still sets.
	p2 := testPlayerWithDefs(nil)
	p2.Equipment().RefreshItems()
	if !p2.Flags.Has(FlagAppearance) {
		t.Fatal("hook-less equipment refresh did not flag APPEARANCE")
	}
}

func TestEquipmentWornInventoryShape(t *testing.T) {
	p := testPlayerWithDefs(nil)
	eq := p.Equipment()
	eq.Set(WeaponSlot, NewItem(4151, 1))
	eq.Set(HeadSlot, NewItem(0, 1)) // id 0 maps to null (id > 0 test)
	eq.Set(BodySlot, NewItem(1127, 0))
	slots := eq.WornInventorySlots()
	if len(slots) != 14 {
		t.Fatalf("worn slots len = %d, want 14", len(slots))
	}
	if !slots[WeaponSlot].Present || slots[WeaponSlot].ID != 4151 || slots[WeaponSlot].Amount != 1 {
		t.Fatalf("weapon worn slot = %+v", slots[WeaponSlot])
	}
	if slots[HeadSlot].Present {
		t.Fatal("id-0 slot maps to a present worn slot (TS requires id > 0)")
	}
	if slots[BodySlot].Present {
		t.Fatal("zero-amount slot maps to a present worn slot")
	}
	// SendWornInventory delivers the shape through the hook with
	// the worn kind; absent slots arrive as empty items.
	var ev RefreshEvent
	eq.SetRefreshHook(func(e RefreshEvent) { ev = e })
	eq.SendWornInventory()
	if ev.Kind != ContainerWornInventory {
		t.Fatalf("worn event kind = %v", ev.Kind)
	}
	if ev.Items[WeaponSlot].ID() != 4151 || ev.Items[HeadSlot].ID() != -1 {
		t.Fatalf("worn event items: weapon=%d head=%d", ev.Items[WeaponSlot].ID(), ev.Items[HeadSlot].ID())
	}
}

func TestWearingNexAmours(t *testing.T) {
	p := testPlayerWithDefs(nil)
	eq := p.Equipment()
	if eq.WearingNexAmours() {
		t.Fatal("naked player wears Nex armour")
	}
	eq.Set(HeadSlot, NewItem(14008, 1))
	eq.Set(BodySlot, NewItem(14009, 1))
	eq.Set(LegSlot, NewItem(14010, 1))
	if !eq.WearingNexAmours() {
		t.Fatal("Torva set not recognized")
	}
	eq.Set(HeadSlot, NewItem(14011, 1))
	eq.Set(BodySlot, NewItem(14012, 1))
	eq.Set(LegSlot, NewItem(14013, 1))
	if !eq.WearingNexAmours() {
		t.Fatal("Pernix set not recognized")
	}
	eq.Set(LegSlot, NewItem(14010, 1)) // mixed set
	if eq.WearingNexAmours() {
		t.Fatal("mixed set recognized as Nex armour")
	}
}

func TestProperEquipmentForWilderness(t *testing.T) {
	src := &testItemDefs{tradeable: map[int]bool{1: true, 2: true, 3: true, 4: false}}
	p := testPlayerWithDefs(src)
	eq := p.Equipment()
	eq.Set(HeadSlot, NewItem(1, 1))
	eq.Set(BodySlot, NewItem(2, 1))
	if eq.ProperEquipmentForWilderness() {
		t.Fatal("2 tradeable items count as proper wilderness equipment")
	}
	eq.Set(LegSlot, NewItem(3, 1))
	if !eq.ProperEquipmentForWilderness() {
		t.Fatal("3 tradeable items do not count")
	}
	// An untradeable meta flag removes an item from the count.
	eq.Set(LegSlot, NewItemWithMeta(3, 1, map[string]json.RawMessage{"untradeable": rawJSON(t, "true")}))
	if eq.ProperEquipmentForWilderness() {
		t.Fatal("untradeable-flagged item counted toward wilderness equipment")
	}
	// No definition source: nothing is tradeable.
	p2 := testPlayerWithDefs(nil)
	p2.Equipment().Set(HeadSlot, NewItem(1, 1))
	p2.Equipment().Set(BodySlot, NewItem(2, 1))
	p2.Equipment().Set(LegSlot, NewItem(3, 1))
	if p2.Equipment().ProperEquipmentForWilderness() {
		t.Fatal("nil-source equipment counts as proper wilderness equipment")
	}
}

// testLevels is a RequirementLevels over fixed max levels and a
// combat level.
type testLevels struct {
	max    [24]int
	combat int
}

func (l testLevels) MaxLevel(skill int) int { return l.max[skill] }
func (l testLevels) CombatLevel() int       { return l.combat }

func TestEquipmentRequirements(t *testing.T) {
	// Item 500: Attack 40, Defence 30. Item 501: combat 100.
	// Item 502: Agility 10 (vowel label). Item 503: no requirements.
	reqs := make([]int, 24)
	reqs[0] = 40 // Attack
	reqs[1] = 30 // Defence
	agility := make([]int, 24)
	agility[16] = 10 // Agility
	src := &testItemDefs{
		requirements: map[int][]int{500: reqs, 502: agility},
		combatReq:    map[int]int{501: 100},
	}
	levels := testLevels{combat: 50}
	levels.max[0] = 40
	levels.max[1] = 29
	unmet := GetUnmetRequirement(levels, src, 500)
	if unmet == nil || unmet.Label != "Defence" || unmet.Level != 30 {
		t.Fatalf("getUnmet(500) = %+v, want Defence 30", unmet)
	}
	if MeetsRequirements(levels, src, 500) {
		t.Fatal("meets(500) with Defence 29")
	}
	levels.max[1] = 30
	if !MeetsRequirements(levels, src, 500) {
		t.Fatal("does not meet 500 with exact levels")
	}
	// First unmet in declaration order wins: Attack checked
	// before Defence.
	levels.max[0] = 39
	unmet = GetUnmetRequirement(levels, src, 500)
	if unmet == nil || unmet.Label != "Attack" {
		t.Fatalf("getUnmet order = %+v, want Attack", unmet)
	}
	levels.max[0] = 40
	// Combat gate.
	unmet = GetUnmetRequirement(levels, src, 501)
	if unmet == nil || unmet.Label != "combat" || unmet.Level != 100 {
		t.Fatalf("getUnmet(501) = %+v, want combat 100", unmet)
	}
	// No requirements: nil, including for a nil source.
	if GetUnmetRequirement(levels, src, 503) != nil {
		t.Fatal("requirement-free item has an unmet requirement")
	}
	if GetUnmetRequirement(levels, nil, 500) != nil {
		t.Fatal("nil-source item has an unmet requirement")
	}
	// Short requirement arrays: missing entries count as 0.
	short := &testItemDefs{requirements: map[int][]int{504: {40}}}
	if GetUnmetRequirement(testLevels{}, short, 504) == nil {
		t.Fatal("short array: Attack 40 not enforced")
	}
	shortReq := GetUnmetRequirement(testLevels{max: [24]int{40}}, short, 504)
	if shortReq != nil {
		t.Fatalf("short array with Attack met: %+v", shortReq)
	}
	// Messages: vowel article, consonant article, combat label,
	// default subject.
	if got := RequirementMessage(UnmetRequirement{"Agility", 10}, "this"); got != "You need an Agility level of at least 10 to wear this." {
		t.Fatalf("message = %q", got)
	}
	if got := RequirementMessage(UnmetRequirement{"Defence", 30}, "this"); got != "You need a Defence level of at least 30 to wear this." {
		t.Fatalf("message = %q", got)
	}
	if got := RequirementMessage(UnmetRequirement{"combat", 100}, "this"); got != "You need a combat level of at least 100 to wear this." {
		t.Fatalf("message = %q", got)
	}
	if got := RequirementMessage(UnmetRequirement{"Attack", 40}, "the abyssal whip"); got != "You need an Attack level of at least 40 to wear the abyssal whip." {
		t.Fatalf("message = %q", got)
	}
	// Agility label comes from index 16 (Skill declaration order).
	low := testLevels{}
	unmet = GetUnmetRequirement(low, src, 502)
	if unmet == nil || unmet.Label != "Agility" {
		t.Fatalf("getUnmet(502) = %+v, want Agility", unmet)
	}
}
