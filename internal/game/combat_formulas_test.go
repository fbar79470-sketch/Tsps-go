package game

import (
	"math"
	"testing"
)

// Formula vectors derived from TS (DamageFormulas.ts,
// AccuracyFormulasDpsCalc.ts). Every expected value is
// hand-computed from the TS formula in the test comment.

func TestMeleeMaxHitVectors(t *testing.T) {
	cases := []struct {
		name     string
		strength int
		strBonus int
		style    FightStyle
		prayer   int // -1 = none
		want     int
		comment  string
	}{
		{
			name: "str 10 accurate naked", strength: 10, strBonus: 0, style: FightStyleAccurate, prayer: -1, want: 2,
			comment: "eff 10+0+8=18; (18*64+320)/640=2.3 -> 2",
		},
		{
			name: "str 99 aggressive +80", strength: 99, strBonus: 80, style: FightStyleAggressive, prayer: -1, want: 25,
			comment: "eff 99+3+8=110; (110*144+320)/640=25.25 -> 25",
		},
		{
			name: "str 20 piety aggressive", strength: 20, strBonus: 0, style: FightStyleAggressive, prayer: PrayerPiety, want: 4,
			comment: "eff floor(20*123/100)=24+3+8=35; (35*64+320)/640=4.0 -> 4",
		},
		{
			name: "str 1 controlled", strength: 1, strBonus: 0, style: FightStyleControlled, prayer: -1, want: 1,
			comment: "eff 1+1+8=10; (10*64+320)/640=1.5 -> 1",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, _ := combatTestPlayer(t, "formula", 3000, 3000)
			setPlayerLevels(p, map[int]int{SkillStrength: tc.strength, SkillPrayer: 99})
			defs := newCombatTestItemDefs()
			weaponID := 100
			bonuses := make([]float64, 14)
			bonuses[10] = float64(tc.strBonus) // cache index 10: melee strength
			defs.bonuses[weaponID] = bonuses
			defs.slots[weaponID] = WeaponSlot
			p.SetItemDefinitions(defs)
			p.Equipment().Set(WeaponSlot, NewItem(weaponID, 1))
			ft := *UnarmedPunch
			ft.Style = tc.style
			p.GetCombat().SetFightType(&ft)
			if tc.prayer >= 0 {
				if !ActivatePrayer(p, tc.prayer) {
					t.Fatalf("ActivatePrayer(%d) failed", tc.prayer)
				}
			}
			if got := CalculateMaxMeleeHit(combatantFor(&p.Mobile), false); got != tc.want {
				t.Errorf("CalculateMaxMeleeHit = %d, want %d (%s)", got, tc.want, tc.comment)
			}
		})
	}
}

func TestRangedMaxHitVectors(t *testing.T) {
	p, _ := combatTestPlayer(t, "ranged", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillRanged: 10})
	// eff 10+3+8=21; (21*64+320)/640=2.6 -> 2.
	if got := CalculateMaxRangedHit(combatantFor(&p.Mobile), false); got != 2 {
		t.Errorf("CalculateMaxRangedHit = %d, want 2", got)
	}
}

func TestSpecialRollsVectors(t *testing.T) {
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	defender, _ := combatTestPlayer(t, "defender", 3001, 3000)
	setPlayerLevels(attacker, map[int]int{SkillAttack: 10})
	setPlayerLevels(defender, map[int]int{SkillDefence: 10})
	resetCombatHooks(t)
	rolls := SpecialRolls(combatantFor(&attacker.Mobile), combatantFor(&defender.Mobile), CombatTypeMelee, nil, "", nil)
	// Attack: eff 10+3+8=21 x (0+64) = 1344.
	if rolls.Attack != 1344 {
		t.Errorf("attack roll = %d, want 1344", rolls.Attack)
	}
	// Defence: eff 10+0+8=18 x (0+64) = 1152.
	if rolls.Defence != 1152 {
		t.Errorf("defence roll = %d, want 1152", rolls.Defence)
	}
}

func TestSpecialRollsNPCLevel1(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	defender, _ := combatTestPlayer(t, "defender", 3001, 3000)
	setPlayerLevels(defender, map[int]int{SkillDefence: 1})
	resetCombatHooks(t)
	rolls := SpecialRolls(combatantFor(&npc.Mobile), combatantFor(&defender.Mobile), CombatTypeMelee, nil, "", nil)
	// NPC attack: eff 1+9=10 x (stats[5]=0+64) = 640.
	if rolls.Attack != 640 {
		t.Errorf("NPC attack roll = %d, want 640", rolls.Attack)
	}
}

func TestMagicDefenceRoll(t *testing.T) {
	attacker, _ := combatTestPlayer(t, "mage", 3000, 3000)
	defender, _ := combatTestPlayer(t, "defender", 3001, 3000)
	setPlayerLevels(attacker, map[int]int{SkillMagic: 10})
	setPlayerLevels(defender, map[int]int{SkillMagic: 10, SkillDefence: 10})
	resetCombatHooks(t)
	rolls := SpecialRolls(combatantFor(&attacker.Mobile), combatantFor(&defender.Mobile), CombatTypeMagic, nil, "", nil)
	// Magic defence: floor(10*7/10)+floor(10*3/10)+0+8 = 18 x 64 = 1152.
	if rolls.Defence != 1152 {
		t.Errorf("magic defence roll = %d, want 1152", rolls.Defence)
	}
	// Magic attack: eff 10+9=19 x 64 = 1216.
	if rolls.Attack != 1216 {
		t.Errorf("magic attack roll = %d, want 1216", rolls.Attack)
	}
}

func TestHitChance(t *testing.T) {
	if got := HitChance(100, 0); got < 0.99 || got > 1.0 {
		t.Errorf("HitChance(100, 0) = %v, want ~1", got)
	}
	if got := HitChance(0, 100); got != 0 {
		t.Errorf("HitChance(0, 100) = %v, want 0", got)
	}
	// att == def: att/(2*(def+1)) = 50/102.
	if got := HitChance(50, 50); math.Abs(got-50.0/102.0) > 1e-9 {
		t.Errorf("HitChance(50, 50) = %v, want %v", got, 50.0/102.0)
	}
}

func TestRollAccuracyDeterministic(t *testing.T) {
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	defender, _ := combatTestPlayer(t, "defender", 3001, 3000)
	setPlayerLevels(attacker, map[int]int{SkillAttack: 99})
	setPlayerLevels(defender, map[int]int{SkillDefence: 1})
	resetCombatHooks(t)
	// Max rolls: attack roll >> defence roll, so the max
	// attack roll beats the max defence roll.
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	if !RollAccuracy(combatantFor(&attacker.Mobile), combatantFor(&defender.Mobile), CombatTypeMelee, nil, "", nil) {
		t.Errorf("max rolls: expected accurate")
	}
	// Min rolls: 0 > 0 is false.
	setFixedRandom(t, func(max int) int { return 0 }, 0.5)
	if RollAccuracy(combatantFor(&attacker.Mobile), combatantFor(&defender.Mobile), CombatTypeMelee, nil, "", nil) {
		t.Errorf("min rolls: expected miss")
	}
}

func TestSpecialDamageRange(t *testing.T) {
	p, _ := combatTestPlayer(t, "ranger", 3000, 3000)
	attacker := combatantFor(&p.Mobile)
	// No traits, no override: {max(0, floor), maxHit}.
	min, max := SpecialDamageRange(attacker, 10, nil, false, 0)
	if min != 0 || max != 10 {
		t.Errorf("plain range = {%d,%d}, want {0,10}", min, max)
	}
	// Decimal multipliers: 0.5..1.5 of 10.
	half, oneHalf := 0.5, 1.5
	min, max = SpecialDamageRange(attacker, 10, &SpecialDamageBounds{
		MinimumMultiplier: &half, MaximumMultiplier: &oneHalf,
	}, false, 0)
	if min != 5 || max != 15 {
		t.Errorf("multiplier range = {%d,%d}, want {5,15}", min, max)
	}
	// Cap truncates the maximum and clamps the minimum.
	capVal := 8
	min, max = SpecialDamageRange(attacker, 10, &SpecialDamageBounds{
		MinimumMultiplier: &half, MaximumMultiplier: &oneHalf, Cap: &capVal,
	}, false, 0)
	if min != 5 || max != 8 {
		t.Errorf("capped range = {%d,%d}, want {5,8}", min, max)
	}
	// Maximum split: index 1 of 2 over maximum 10 ->
	// floor(10*2/2)-floor(10*1/2) = 10-5 = 5.
	min, max = SpecialDamageRange(attacker, 10, &SpecialDamageBounds{
		MaximumSplit: &MaximumSplit{Count: 2, Index: 1},
	}, false, 0)
	if min != 0 || max != 5 {
		t.Errorf("split range = {%d,%d}, want {0,5}", min, max)
	}
}

func TestScaleSpecial(t *testing.T) {
	if got := scaleSpecial(10, 1.3); got != 13 {
		t.Errorf("scaleSpecial(10, 1.3) = %d, want 13", got)
	}
	if got := scaleSpecial(7, 0.5); got != 3 {
		t.Errorf("scaleSpecial(7, 0.5) = %d, want 3", got)
	}
}

func TestNPCMaxHitDrained(t *testing.T) {
	def := newCombatFakeDef()
	def.stats = [18]int{50, 50, 50, 50, 50, 0, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
	def.maxHit = 20
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	// Full strength: the definition maximum.
	if got := CalculateMaxMeleeHit(combatantFor(&npc.Mobile), false); got != 20 {
		t.Errorf("full max hit = %d, want 20", got)
	}
}

func TestMagicHitBoundsHarmlessSpell(t *testing.T) {
	p, _ := combatTestPlayer(t, "mage", 3000, 3000)
	spell := newFakeSpell()
	spell.maximumHit = 0
	// Declared maximum 0 stays 0 through every bonus.
	min, max := GetMagicHitBounds(combatantFor(&p.Mobile), spell, nil)
	if min != 0 || max != 0 {
		t.Errorf("harmless bounds = {%d,%d}, want {0,0}", min, max)
	}
}

func TestApplyMagicDamageBonus(t *testing.T) {
	p, _ := combatTestPlayer(t, "mage", 3000, 3000)
	defs := newCombatTestItemDefs()
	weaponID := 200
	bonuses := make([]float64, 14)
	bonuses[12] = 10 // +10% magic damage
	defs.bonuses[weaponID] = bonuses
	p.SetItemDefinitions(defs)
	p.Equipment().Set(WeaponSlot, NewItem(weaponID, 1))
	// floor(10 * 1100/1000) = 11.
	if got := ApplyMagicDamageBonus(combatantFor(&p.Mobile), 10); got != 11 {
		t.Errorf("magic damage bonus = %d, want 11", got)
	}
}

func TestPrayerPercentChains(t *testing.T) {
	p, _ := combatTestPlayer(t, "priest", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99, SkillAttack: 20, SkillStrength: 20, SkillDefence: 20})
	// Piety: attack 120, strength 123, defence 125.
	if !ActivatePrayer(p, PrayerPiety) {
		t.Fatal("ActivatePrayer(Piety) failed")
	}
	// Attack roll with Piety: eff floor(20*120/100)=24+3+8=35 x 64 = 2240.
	defender, _ := combatTestPlayer(t, "defender", 3001, 3000)
	resetCombatHooks(t)
	rolls := SpecialRolls(combatantFor(&p.Mobile), combatantFor(&defender.Mobile), CombatTypeMelee, nil, "", nil)
	if rolls.Attack != 2240 {
		t.Errorf("Piety attack roll = %d, want 2240", rolls.Attack)
	}
}
