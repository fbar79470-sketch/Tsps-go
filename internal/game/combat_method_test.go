package game

import "testing"

func TestMeleeMethodHits(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	hits := MeleeMethod.Hits(&attacker.Mobile, &target.Mobile)
	if len(hits) != 1 {
		t.Fatalf("melee hits = %d, want 1", len(hits))
	}
	if hits[0].Delay() != 0 {
		t.Errorf("melee hit delay = %d, want 0", hits[0].Delay())
	}
	if got := MeleeMethod.AttackDistance(&attacker.Mobile); got != 1 {
		t.Errorf("melee attack distance = %d, want 1", got)
	}
	if !MeleeMethod.CanAttack(&attacker.Mobile, &target.Mobile) {
		t.Errorf("melee should be allowed by default")
	}
}

// TestRangedMethodDarkBowSecondHitDelay is the dark bow's
// second hit delay (WeaponProfile.ts:295): at edge distance
// 9 the two shots land on 1 + (3+9)/6 = 3 and
// 1 + (2+9)/3 = 4 — not both on the STANDARD delay.
func TestRangedMethodDarkBowSecondHitDelay(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	attacker.Equipment().Set(WeaponSlot, NewItem(11235, 1))
	attacker.Equipment().Set(AmmunitionSlot, NewItem(892, 10))
	attacker.GetCombat().SetRangedWeapon(rangedWeaponByItem[11235])
	attacker.GetCombat().SetAmmunition(rangedAmmunitionByItem[892])
	far, _ := combatTestPlayer(t, "far", 3009, 3000)
	hits := RangedMethod.Hits(&attacker.Mobile, &far.Mobile)
	if got := delaysOf(hits); len(got) != 2 || got[0] != 3 || got[1] != 4 {
		t.Errorf("dark bow hit delays at distance 9 = %v, want [3 4]", got)
	}
	// One arrow left: a single-arrow shot keeps only the
	// first delay (RangedCombatMethod.shotLimit).
	attacker.Equipment().Set(AmmunitionSlot, NewItem(892, 1))
	hits = RangedMethod.Hits(&attacker.Mobile, &far.Mobile)
	if got := delaysOf(hits); len(got) != 1 || got[0] != 3 {
		t.Errorf("dark bow one-arrow hit delays = %v, want [3]", got)
	}
}

func TestRangedMethodDelays(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	// Player formula: 1 + floor((3+d)/6).
	near, _ := combatTestPlayer(t, "near", 3001, 3000)
	hits := RangedMethod.Hits(&attacker.Mobile, &near.Mobile)
	if len(hits) != 1 || hits[0].Delay() != 1 {
		t.Errorf("ranged delay at distance 1 = %v, want 1", delaysOf(hits))
	}
	far, _ := combatTestPlayer(t, "far", 3009, 3000)
	hits = RangedMethod.Hits(&attacker.Mobile, &far.Mobile)
	if len(hits) != 1 || hits[0].Delay() != 3 {
		t.Errorf("ranged delay at distance 9 = %v, want 3", delaysOf(hits))
	}
	// NPC: arrivalTicks(46, 5/tile): 0 tiles -> 2, 10 tiles -> 4.
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	hits = RangedMethod.Hits(&npc.Mobile, &near.Mobile)
	if len(hits) != 1 || hits[0].Delay() != 2 {
		t.Errorf("NPC ranged delay adjacent = %v, want 2", delaysOf(hits))
	}
	hits = RangedMethod.Hits(&npc.Mobile, &far.Mobile)
	if len(hits) != 1 || hits[0].Delay() != 4 {
		t.Errorf("NPC ranged delay at 9 = %v, want 4", delaysOf(hits))
	}
	if got := RangedMethod.AttackDistance(&attacker.Mobile); got != 6 {
		t.Errorf("ranged attack distance = %d, want 6", got)
	}
}

func delaysOf(hits []*PendingHit) []int {
	out := make([]int, len(hits))
	for i, h := range hits {
		out[i] = h.Delay()
	}
	return out
}

func TestMagicMethodDelaysAndTeardown(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	caster, _ := combatTestPlayer(t, "caster", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	spell := newFakeSpell()
	spell.attackSpeed = 5
	caster.GetCombat().SetCastSpell(spell)
	// Player formula: 1 + floor((1+d)/3); d=1 -> 1.
	hits := MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if len(hits) != 1 || hits[0].Delay() != 1 {
		t.Fatalf("magic delay = %v, want [1]", delaysOf(hits))
	}
	if hits[0].Spell() != CombatSpell(spell) {
		t.Errorf("magic hit should carry the spell")
	}
	// Spell fixed delay wins.
	spell.hitDelay = 4
	hits = MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if hits[0].Delay() != 4 {
		t.Errorf("spell fixed delay = %d, want 4", hits[0].Delay())
	}
	spell.hitDelay = -1
	// NPC: arrivalTicks(46, 10/tile): adjacent -> 2.
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	hits = MagicMethod.Hits(&npc.Mobile, &target.Mobile)
	if hits[0].Delay() != 2 {
		t.Errorf("NPC magic delay = %d, want 2", hits[0].Delay())
	}
	// Attack speed comes from the previous cast.
	if got := MagicMethod.AttackSpeed(&caster.Mobile); got != 4 {
		t.Errorf("attack speed without previous cast = %d, want base 4", got)
	}
	caster.GetCombat().SetPreviousCast(spell)
	if got := MagicMethod.AttackSpeed(&caster.Mobile); got != 5 {
		t.Errorf("attack speed with previous cast = %d, want 5", got)
	}
	// Finished: not autocasting -> cast becomes previous, combat resets.
	caster.GetCombat().SetCastSpell(spell)
	caster.GetCombat().Attack(&target.Mobile)
	MagicMethod.Finished(&caster.Mobile, &target.Mobile)
	if caster.GetCombat().CastSpell() != nil {
		t.Errorf("cast spell should clear on finish")
	}
	if caster.GetCombat().PreviousCast() != CombatSpell(spell) {
		t.Errorf("previous cast should be the finished spell")
	}
	if caster.GetCombat().Target() != nil {
		t.Errorf("non-autocast finish should reset combat")
	}
	// CanAttack with no spell: false.
	if MagicMethod.CanAttack(&caster.Mobile, &target.Mobile) {
		t.Errorf("magic without a spell should refuse")
	}
}

func TestMagicBaseExperienceOnce(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	caster, _ := combatTestPlayer(t, "caster", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	setPlayerLevels(target, map[int]int{SkillHitpoints: 99})
	spell := newFakeSpell()
	spell.baseXP = 30
	spell.splashXP = 5
	caster.GetCombat().SetCastSpell(spell)
	MagicMethod.Start(&caster.Mobile, &target.Mobile)
	MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if got := caster.Skills().Experience(SkillMagic); got != 30 {
		t.Errorf("base XP after first cast = %d, want 30", got)
	}
	// A second Hits without Start pays nothing more.
	MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if got := caster.Skills().Experience(SkillMagic); got != 30 {
		t.Errorf("base XP after second Hits = %d, want 30", got)
	}
}

func TestProviderRegistryOrder(t *testing.T) {
	resetCombatHooks(t)
	npcCombatMethodProviders = nil
	t.Cleanup(func() { npcCombatMethodProviders = nil })
	first := &stubCombatMethod{methodType: CombatTypeRanged}
	second := &stubCombatMethod{methodType: CombatTypeMagic}
	RegisterNPCCombatMethodProvider("first-plugin", []int{7}, NPCCombatMethodProviderFunc(func(npc *NPC) CombatMethod { return first }))
	RegisterNPCCombatMethodProvider("second-plugin", []int{7}, NPCCombatMethodProviderFunc(func(npc *NPC) CombatMethod { return second }))
	npc := combatTestNPC(t, 7, 3000, 3000, nil)
	// Registration order decides: the first-registered wins.
	if got := npcCombatMethodFromProviders(npc); got != CombatMethod(first) {
		t.Errorf("provider = %v, want the first-registered", got)
	}
	// A provider returning nil passes to the next.
	npcCombatMethodProviders = nil
	RegisterNPCCombatMethodProvider("nil-plugin", []int{7}, NPCCombatMethodProviderFunc(func(npc *NPC) CombatMethod { return nil }))
	RegisterNPCCombatMethodProvider("real-plugin", []int{7}, NPCCombatMethodProviderFunc(func(npc *NPC) CombatMethod { return second }))
	if got := npcCombatMethodFromProviders(npc); got != CombatMethod(second) {
		t.Errorf("provider after nil = %v, want the second", got)
	}
	// Non-matching ids never consult the provider.
	other := combatTestNPC(t, 8, 3000, 3000, nil)
	if got := npcCombatMethodFromProviders(other); got != nil {
		t.Errorf("provider for id 8 = %v, want nil", got)
	}
	// GetMethod consults providers before the definition fallback.
	if got := GetMethod(&npc.Mobile); got != CombatMethod(second) {
		t.Errorf("GetMethod = %v, want the provider method", got)
	}
}

// stubCombatMethod is a minimal CombatMethod for registry tests.
type stubCombatMethod struct {
	BaseCombatMethod
	methodType CombatType
}

func (m *stubCombatMethod) Type() CombatType { return m.methodType }
func (m *stubCombatMethod) Hits(attacker, target *Mobile) []*PendingHit {
	return []*PendingHit{NewPendingHit(attacker, target, m, 0)}
}
