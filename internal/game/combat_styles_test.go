package game

import (
	"encoding/json"
	"testing"
)

// Fixture combat-styles JSON: the UNARMED, STAFF and WHIP rows
// transcribed verbatim from data/definitions/item-combat-styles.json
// (the shipped file's rows, cited by key). Unit tests never read
// the shipped file; the full file's counts and SHA are verified
// by the live smoke test and the loader's SHA check.
const fixtureCombatStylesJSON = `{
  "fightTypes": {
    "UNARMED_PUNCH": {"name": "Punch", "animation": 422, "childId": 0, "attackType": "CRUSH", "style": "ACCURATE", "sound": "WEAPON_UNARMED_PUNCH"},
    "UNARMED_KICK": {"name": "Kick", "animation": 423, "childId": 1, "attackType": "CRUSH", "style": "AGGRESSIVE", "sound": "WEAPON_UNARMED_KICK"},
    "UNARMED_BLOCK": {"name": "Block", "animation": 422, "childId": 3, "attackType": "CRUSH", "style": "DEFENSIVE", "sound": "WEAPON_UNARMED_PUNCH"},
    "STAFF_BASH": {"name": "Bash", "animation": 401, "childId": 0, "attackType": "CRUSH", "style": "ACCURATE", "sound": "WEAPON_STAFF"},
    "STAFF_POUND": {"name": "Pound", "animation": 406, "childId": 1, "attackType": "CRUSH", "style": "AGGRESSIVE", "sound": "WEAPON_STAFF"},
    "STAFF_FOCUS": {"name": "Focus", "animation": 406, "childId": 3, "attackType": "CRUSH", "style": "DEFENSIVE", "sound": "WEAPON_STAFF"},
    "WHIP_FLICK": {"name": "Flick", "animation": 1658, "childId": 0, "attackType": "SLASH", "style": "ACCURATE", "sound": "WEAPON_WHIP"},
    "WHIP_LASH": {"name": "Lash", "animation": 1658, "childId": 1, "attackType": "SLASH", "style": "CONTROLLED", "sound": "WEAPON_WHIP"},
    "WHIP_DEFLECT": {"name": "Deflect", "animation": 1658, "childId": 3, "attackType": "SLASH", "style": "DEFENSIVE", "sound": "WEAPON_WHIP"}
  },
  "weaponInterfaces": {
    "UNARMED": {"interfaceId": 5855, "nameLineId": 5857, "speed": 4, "category": 0, "specialBar": -1, "specialMeter": -1, "fightTypes": ["UNARMED_PUNCH", "UNARMED_KICK", "UNARMED_BLOCK"]},
    "STAFF": {"interfaceId": 328, "nameLineId": 355, "speed": 5, "category": 18, "specialBar": -1, "specialMeter": -1, "fightTypes": ["STAFF_BASH", "STAFF_POUND", "STAFF_FOCUS"]},
    "WHIP": {"interfaceId": 12290, "nameLineId": 12293, "speed": 4, "category": 20, "specialBar": 12323, "specialMeter": 12335, "fightTypes": ["WHIP_FLICK", "WHIP_LASH", "WHIP_DEFLECT"]}
  }
}`

func fixtureRegistry(t *testing.T) *CombatStyleRegistry {
	t.Helper()
	reg, err := parseCombatStyles([]byte(fixtureCombatStylesJSON))
	if err != nil {
		t.Fatalf("parsing fixture combat styles: %v", err)
	}
	return reg
}

// styleTestDefs is a testItemDefs extended with the optional
// combat capabilities: weapon-interface names, item names and
// bonus arrays, all per-id from the test's maps.
type styleTestDefs struct {
	*testItemDefs
	interfaces map[int]string
	names      map[int]string
	bonuses    map[int][]float64
}

func (s *styleTestDefs) WeaponInterfaceName(id int) string { return s.interfaces[id] }
func (s *styleTestDefs) WeaponAttackSpeed(id int) int      { return 4 }
func (s *styleTestDefs) WeaponAttackRange(id int) int      { return 6 }
func (s *styleTestDefs) WeaponAttackAnim(id int, attackType string) int {
	return -1
}
func (s *styleTestDefs) WeaponBlockAnim(id int) int { return 424 }
func (s *styleTestDefs) ItemName(id int) string     { return s.names[id] }
func (s *styleTestDefs) ItemBonuses(id int) []float64 {
	if b, ok := s.bonuses[id]; ok {
		return append([]float64(nil), b...)
	}
	return nil
}

func TestParseCombatStylesFixtureCounts(t *testing.T) {
	reg := fixtureRegistry(t)
	if reg.FightTypeCount() != 9 {
		t.Fatalf("fight types = %d, want 9", reg.FightTypeCount())
	}
	if reg.WeaponInterfaceCount() != 3 {
		t.Fatalf("weapon interfaces = %d, want 3", reg.WeaponInterfaceCount())
	}
}

func TestParseCombatStylesSpotChecks(t *testing.T) {
	reg := fixtureRegistry(t)
	punch := reg.FightType("UNARMED_PUNCH")
	if punch == nil || punch.Animation != 422 || punch.ChildID != 0 || punch.ParentID != 43 ||
		punch.BonusType != BonusAttackCrush || punch.Style != FightStyleAccurate || punch.NPCAnimation != -1 || punch.Rapid {
		t.Fatalf("UNARMED_PUNCH = %+v", punch)
	}
	if punch.Sound != SoundWeaponUnarmedPunch {
		t.Fatalf("UNARMED_PUNCH sound = %+v, want WEAPON_UNARMED_PUNCH", punch.Sound)
	}
	// The package default is the same row (combat_types.go).
	if UnarmedPunch.Animation != punch.Animation || UnarmedPunch.ChildID != punch.ChildID ||
		UnarmedPunch.BonusType != punch.BonusType || UnarmedPunch.Style != punch.Style {
		t.Fatalf("UnarmedPunch %+v does not match registry UNARMED_PUNCH %+v", UnarmedPunch, punch)
	}
	whip := reg.WeaponInterface("WHIP")
	if whip == nil || whip.InterfaceID != 12290 || whip.NameLineID != 12293 || whip.Speed != 4 ||
		whip.Category != 20 || whip.SpecialBar != 12323 || whip.SpecialMeter != 12335 || len(whip.FightTypes) != 3 {
		t.Fatalf("WHIP = %+v", whip)
	}
	if lash := whip.FightTypeForChild(1); lash == nil || lash.Name != "WHIP_LASH" || lash.Style != FightStyleControlled || lash.BonusType != BonusAttackSlash {
		t.Fatalf("WHIP child 1 = %+v", lash)
	}
	if lowest := whip.LowestFightType(); lowest == nil || lowest.ChildID != 0 {
		t.Fatalf("WHIP lowest = %+v", lowest)
	}
	staff := reg.WeaponInterface("STAFF")
	if staff == nil || staff.Category != 18 || staff.SpecialBar != -1 || staff.FightTypeForChild(2) != nil {
		t.Fatalf("STAFF = %+v", staff)
	}
	// Unknown names resolve to nil, never a panic.
	if reg.FightType("NOPE") != nil || reg.WeaponInterface("NOPE") != nil {
		t.Fatal("unknown names must resolve to nil")
	}
}

func TestParseCombatStylesUnknownFightTypeErrors(t *testing.T) {
	bad := `{"fightTypes": {}, "weaponInterfaces": {"X": {"interfaceId": 1, "nameLineId": 2, "speed": 4, "category": 0, "fightTypes": ["MISSING"]}}}`
	if _, err := parseCombatStyles([]byte(bad)); err == nil {
		t.Fatal("unknown fight-type reference must error (TS throws)")
	}
}

func TestParseCombatStylesUnknownNamesFallBack(t *testing.T) {
	// TS fallbacks (FightType.ts static block): unknown attack
	// type reads CRUSH, unknown style reads ACCURATE, unknown
	// sound reads WEAPON.
	row := `{"fightTypes": {"X": {"animation": 1, "childId": 0, "attackType": "BOGUS", "style": "BOGUS", "sound": "BOGUS"}}, "weaponInterfaces": {}}`
	reg, err := parseCombatStyles([]byte(row))
	if err != nil {
		t.Fatalf("parsing fallback row: %v", err)
	}
	ft := reg.FightType("X")
	if ft == nil || ft.BonusType != BonusAttackCrush || ft.Style != FightStyleAccurate || ft.Sound != SoundWeapon {
		t.Fatalf("fallback row = %+v", ft)
	}
}

func TestLoadCombatStylesRejectsWrongSHA(t *testing.T) {
	if _, err := LoadCombatStyles([]byte(fixtureCombatStylesJSON)); err == nil {
		t.Fatal("fixture bytes must fail the SHA check (they are not the shipped file)")
	}
	if _, err := LoadCombatStyles([]byte("{}")); err == nil {
		t.Fatal("empty JSON must fail the SHA check")
	}
}

func TestUnarmedFallbackRegistry(t *testing.T) {
	reg := UnarmedFallbackRegistry()
	if reg.FightTypeCount() != 3 || reg.WeaponInterfaceCount() != 1 {
		t.Fatalf("fallback counts = %d/%d, want 3/1", reg.FightTypeCount(), reg.WeaponInterfaceCount())
	}
	unarmed := reg.WeaponInterface("UNARMED")
	if unarmed == nil || unarmed.InterfaceID != 5855 || unarmed.NameLineID != 5857 || unarmed.Category != 0 {
		t.Fatalf("fallback UNARMED = %+v", unarmed)
	}
	if reg.InterfaceForItem(1333, &styleTestDefs{testItemDefs: &testItemDefs{}, interfaces: map[int]string{1333: "SCIMITAR"}}) != nil {
		t.Fatal("fallback registry must not resolve SCIMITAR")
	}
}

func TestCombatStylesInstallAndFallback(t *testing.T) {
	previous := InstallCombatStyles(fixtureRegistry(t))
	defer InstallCombatStyles(previous)
	if CombatStyles().WeaponInterfaceCount() != 3 {
		t.Fatal("installed registry not visible through CombatStyles()")
	}
	previous = InstallCombatStyles(nil)
	defer InstallCombatStyles(previous)
	if CombatStyles().WeaponInterfaceCount() != 1 {
		t.Fatal("nil install must read as the UNARMED fallback")
	}
}

func TestInterfaceForItemResolution(t *testing.T) {
	reg := fixtureRegistry(t)
	defs := &styleTestDefs{testItemDefs: &testItemDefs{}, interfaces: map[int]string{1333: "WHIP", 1381: "STAFF"}}
	if got := reg.InterfaceForItem(1333, defs); got == nil || got.Name != "WHIP" {
		t.Fatalf("InterfaceForItem(1333) = %+v", got)
	}
	// No name, unknown name, nil source, invalid id: all nil.
	if reg.InterfaceForItem(9999, defs) != nil {
		t.Fatal("item with no interface name must resolve nil")
	}
	if reg.InterfaceForItem(1333, &styleTestDefs{testItemDefs: &testItemDefs{}, interfaces: map[int]string{1333: "NOPE"}}) != nil {
		t.Fatal("unknown interface name must resolve nil")
	}
	if reg.InterfaceForItem(1333, nil) != nil || reg.InterfaceForItem(-1, defs) != nil {
		t.Fatal("nil source / invalid id must resolve nil")
	}
}

func TestResolveFightType(t *testing.T) {
	reg := fixtureRegistry(t)
	whip := reg.WeaponInterface("WHIP")
	available := whip.FightTypes
	// Identity resolves.
	if got := reg.ResolveFightType(available[1], available); got != available[1] {
		t.Fatalf("identity resolve = %+v", got)
	}
	// Field-equal copy resolves to the registry row.
	copy := &FightType{Name: "COPY", Animation: 1658, NPCAnimation: -1, ChildID: 1, ParentID: 43, BonusType: BonusAttackSlash, Style: FightStyleControlled}
	if got := reg.ResolveFightType(copy, available); got != available[1] {
		t.Fatalf("field resolve = %+v", got)
	}
	// Nothing matches.
	other := &FightType{Animation: 1, ParentID: 43, ChildID: 2, BonusType: BonusAttackCrush}
	if reg.ResolveFightType(other, available) != nil {
		t.Fatal("non-matching fight type must resolve nil")
	}
	if reg.ResolveFightType(nil, available) != nil {
		t.Fatal("nil fight type must resolve nil")
	}
}

func TestCorrespondingBonus(t *testing.T) {
	for _, tc := range []struct {
		bonus int
		want  int
	}{
		{BonusAttackStab, BonusAttackStab},
		{BonusAttackSlash, BonusAttackSlash},
		{BonusAttackCrush, BonusAttackCrush},
		{BonusAttackMagic, BonusAttackMagic},
		{BonusAttackRange, BonusAttackRange},
		{99, BonusAttackCrush},
	} {
		if got := (&FightType{BonusType: tc.bonus}).CorrespondingBonus(); got != tc.want {
			t.Fatalf("CorrespondingBonus(%d) = %d, want %d", tc.bonus, got, tc.want)
		}
	}
	if got := (*FightType)(nil).CorrespondingBonus(); got != BonusAttackCrush {
		t.Fatalf("nil CorrespondingBonus = %d, want crush", got)
	}
}

func TestRememberCombatStyleMemory(t *testing.T) {
	p := NewPlayer("styler", Location{X: 3200, Y: 3200, Z: 0})
	if _, ok := RememberedCombatStyleSlot(p, 20); ok {
		t.Fatal("fresh player must have no remembered slot")
	}
	RememberCombatStyle(p, 20, 1)
	RememberCombatStyle(p, 9, 3)
	if slot, ok := RememberedCombatStyleSlot(p, 20); !ok || slot != 1 {
		t.Fatalf("remembered slot for 20 = %d/%v, want 1/true", slot, ok)
	}
	if slot, ok := RememberedCombatStyleSlot(p, 9); !ok || slot != 3 {
		t.Fatalf("remembered slot for 9 = %d/%v, want 3/true", slot, ok)
	}
	// Overwriting one category leaves the other alone.
	RememberCombatStyle(p, 20, 3)
	if slot, _ := RememberedCombatStyleSlot(p, 20); slot != 3 {
		t.Fatalf("overwritten slot for 20 = %d, want 3", slot)
	}
	if slot, _ := RememberedCombatStyleSlot(p, 9); slot != 3 {
		t.Fatalf("slot for 9 changed to %d", slot)
	}
	// The stored value is JSON-compatible (string keys): the
	// save's cloneAttribute rule rejects map[int]... keys.
	raw := p.GetAttribute(CombatStyleByCategoryAttribute)
	byJSON, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("style memory not JSON-marshalable: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(byJSON, &decoded); err != nil {
		t.Fatalf("style memory JSON round-trip: %v", err)
	}
	// A save-decoded map[string]any (float64 values) reads back.
	p.SetAttribute(CombatStyleByCategoryAttribute, map[string]any{"20": float64(1), "9": float64(0)})
	if slot, ok := RememberedCombatStyleSlot(p, 20); !ok || slot != 1 {
		t.Fatalf("save-decoded slot for 20 = %d/%v, want 1/true", slot, ok)
	}
	RememberCombatStyle(nil, 0, 0)
	if _, ok := RememberedCombatStyleSlot(nil, 0); ok {
		t.Fatal("nil player must have no remembered slot")
	}
}
