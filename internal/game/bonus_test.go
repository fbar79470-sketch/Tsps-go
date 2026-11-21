package game

import (
	"testing"
)

// The bonus arrays below are the real rev-242 cache values for
// each item, dumped from the live cache + definition store with
// the throwaway cmd/dumpbonuses probe (deleted after use; the
// arrays are pasted here as fixtures, not loaded from disk).
// Layout (BonusManager.ts:5-20): 0..4 attack stab/slash/crush/
// magic/range, 5..9 defence in the same order, 10 melee
// strength, 11 ranged strength, 12 magic damage, 13 prayer.
var bonusFixtureArrays = map[int][]float64{
	1333:  {7, 45, -2, 0, 0, 0, 1, 0, 0, 0, 44, 0, 0, 0},     // Rune scimitar
	1163:  {0, 0, 0, -6, -3, 30, 32, 27, -1, 30, 0, 0, 0, 0}, // Rune full helm
	1127:  {0, 0, 0, -30, -15, 82, 80, 72, -6, 80, 0, 0, 0, 0},
	1079:  {0, 0, 0, -21, -11, 51, 49, 47, -4, 49, 0, 0, 0, 0},
	1201:  {0, 0, 0, -8, -3, 44, 48, 46, -1, 46, 0, 0, 0, 0},
	4131:  {0, 0, 0, -3, -1, 12, 13, 14, 0, 0, 2, 0, 0, 0},
	1725:  {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 10, 0, 0, 0},  // Amulet of strength
	861:   {0, 0, 0, 0, 69, 0, 0, 0, 0, 0, 0, 0, 0, 0},  // Magic shortbow
	886:   {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16, 0, 0},  // Steel arrow
	22804: {0, 0, 0, 0, 28, 0, 0, 0, 0, 0, 0, 30, 0, 0}, // Dragon knife
	11212: {0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 60, 0, 0},  // Dragon arrow
}

func bonusTestPlayer() (*Player, *styleTestDefs) {
	bonuses := make(map[int][]float64, len(bonusFixtureArrays))
	for id, b := range bonusFixtureArrays {
		bonuses[id] = append([]float64(nil), b...)
	}
	defs := &styleTestDefs{testItemDefs: &testItemDefs{}, bonuses: bonuses}
	p := NewPlayer("bonuses", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	return p, defs
}

func TestComputeBonusesRuneMeleeSet(t *testing.T) {
	p, _ := bonusTestPlayer()
	eq := p.Equipment()
	eq.SetItem(WeaponSlot, NewItem(1333, 1))
	eq.SetItem(HeadSlot, NewItem(1163, 1))
	eq.SetItem(BodySlot, NewItem(1127, 1))
	eq.SetItem(LegSlot, NewItem(1079, 1))
	eq.SetItem(ShieldSlot, NewItem(1201, 1))
	eq.SetItem(FeetSlot, NewItem(4131, 1))
	eq.SetItem(AmuletSlot, NewItem(1725, 1))
	// Hand-computed from the arrays above, index by index:
	// stab atk 7; slash atk 45; crush atk -2;
	// magic atk -6-30-21-8-3 = -68; range atk -3-15-11-3-1 = -33;
	// stab def 30+82+51+44+12 = 219;
	// slash def 1+32+80+49+48+13 = 223;
	// crush def 27+72+47+46+14 = 206;
	// magic def -1-6-4-1 = -12;
	// range def 30+80+49+46 = 205;
	// melee str 44+2+10 = 56; ranged str 0; magic dmg 0; prayer 0.
	want := [14]int{7, 45, -2, -68, -33, 219, 223, 206, -12, 205, 56, 0, 0, 0}
	if got := ComputeBonuses(p, nil).Bonus(); got != want {
		t.Fatalf("rune set bonuses = %v, want %v", got, want)
	}
	// The split getters read the same totals.
	m := ComputeBonuses(p, nil)
	if m.AttackBonus(BonusAttackSlash) != 45 || m.DefenceBonus(0) != 219 ||
		m.OtherBonus(OtherBonusStrength) != 56 || m.OtherBonus(OtherBonusPrayer) != 0 {
		t.Fatalf("split getters = slash %d, stab def %d, str %d, prayer %d",
			m.AttackBonus(BonusAttackSlash), m.DefenceBonus(0),
			m.OtherBonus(OtherBonusStrength), m.OtherBonus(OtherBonusPrayer))
	}
}

func TestComputeBonusesAmmoRule(t *testing.T) {
	p, _ := bonusTestPlayer()
	eq := p.Equipment()
	// A bow fires from the ammo slot: the arrows' ranged
	// strength counts (BonusManager.ts:59-67). Index 11 is
	// 10 + OtherBonusRangedStrength.
	eq.SetItem(WeaponSlot, NewItem(861, 1))
	eq.SetItem(AmmunitionSlot, NewItem(886, 100))
	got := ComputeBonuses(p, &BonusOptions{FiresFromAmmoSlot: func(*Player) bool { return true }}).Bonus()
	if got[BonusAttackRange] != 69 || got[10+OtherBonusRangedStrength] != 16 {
		t.Fatalf("bow+arrows = range atk %d, ranged str %d; want 69, 16", got[BonusAttackRange], got[10+OtherBonusRangedStrength])
	}
	// A thrown weapon does not fire its ammo slot: the arrows'
	// ranged strength does not count — only the knife's +30
	// (BonusManager.ts:57-58, RangedData.ts:317-322).
	eq.SetItem(WeaponSlot, NewItem(22804, 1))
	eq.SetItem(AmmunitionSlot, NewItem(11212, 100))
	got = ComputeBonuses(p, &BonusOptions{FiresFromAmmoSlot: func(*Player) bool { return false }}).Bonus()
	if got[BonusAttackRange] != 28 || got[10+OtherBonusRangedStrength] != 30 {
		t.Fatalf("knife+arrows = range atk %d, ranged str %d; want 28, 30", got[BonusAttackRange], got[10+OtherBonusRangedStrength])
	}
	// The arrows' other bonuses still count even when the ammo
	// slot does not fire: only index 11 is subtracted.
	if got[BonusAttackStab] != 0 || got[5+BonusAttackRange] != 0 {
		t.Fatalf("knife+arrows other indices = %v", got)
	}
}

func TestComputeBonusesVetoAndEdges(t *testing.T) {
	p, defs := bonusTestPlayer()
	eq := p.Equipment()
	eq.SetItem(WeaponSlot, NewItem(1333, 1))
	eq.SetItem(BodySlot, NewItem(1127, 1))
	// emitCanUseItem veto ("bonus"): the platebody's bonuses
	// drop out entirely (BonusManager.ts:49-51).
	vetoed := ComputeBonuses(p, &BonusOptions{CanUseItem: func(int) bool { return false }}).Bonus()
	if vetoed[5+BonusAttackStab] != 0 {
		t.Fatalf("vetoed platebody still counts: stab def %d", vetoed[5+BonusAttackStab])
	}
	// A per-item veto: only the platebody is refused.
	perItem := ComputeBonuses(p, &BonusOptions{CanUseItem: func(id int) bool { return id != 1127 }}).Bonus()
	if perItem[5+BonusAttackStab] != 0 || perItem[BonusAttackSlash] != 45 {
		t.Fatalf("per-item veto = %v, want stab def 0, slash atk 45", perItem)
	}
	// Empty equipment: all zeros. No definitions: all zeros.
	if got := ComputeBonuses(NewPlayer("naked", Location{X: 3200, Y: 3200, Z: 0}), nil).Bonus(); got != [14]int{} {
		t.Fatalf("naked bonuses = %v, want zeros", got)
	}
	naked := NewPlayer("naked2", Location{X: 3200, Y: 3200, Z: 0})
	naked.SetItemDefinitions(defs)
	if got := ComputeBonuses(naked, nil).Bonus(); got != [14]int{} {
		t.Fatalf("empty-equipment bonuses = %v, want zeros", got)
	}
	// A short bonus array adds only its entries, without a panic:
	// stab atk 7 (scimitar) + 5, slash atk 45 + 6, stab def still
	// 82 (platebody only — the short array has no index 5).
	defs.bonuses[9999] = []float64{5, 6}
	eq.SetItem(HeadSlot, NewItem(9999, 1))
	got := ComputeBonuses(p, nil).Bonus()
	if got[BonusAttackStab] != 12 || got[BonusAttackSlash] != 51 || got[5+BonusAttackStab] != 82 {
		t.Fatalf("short-array bonuses = %v", got)
	}
	// Nil player and nil defs are safe.
	if got := ComputeBonuses(nil, nil).Bonus(); got != [14]int{} {
		t.Fatalf("nil player bonuses = %v", got)
	}
}
