package game

import "testing"

func TestHooksNilSafetyFullAttack(t *testing.T) {
	// Zero Hooks: a complete attack cycle must not panic and
	// must still land damage.
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	// PvP needs a hook opinion, so fight an NPC (PvE default allow).
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	npc.Hitpoints = 99
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker.GetCombat().Attack(&npc.Mobile)
	advance(100)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	advance(101)
	npc.GetCombat().ProcessHitQueue(101)
	if got := npc.Hitpoints; got >= 99 {
		t.Errorf("NPC HP = %d, want damage with zero hooks", got)
	}
}

func TestHooksHitDamageMutation(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	seen := 0
	Hooks.OnCombatHitDamage = func(e *CombatHitDamageEvent) {
		seen++
		e.Damage.SetDamage(42)
		e.Accurate = true
	}
	// The hook fires during the roll and rewrites the splat.
	d := 5
	hit := NewPendingHitConfig(&attacker.Mobile, &victim.Mobile, MeleeMethod,
		PendingHitConfig{Damage: &d, RollAccuracy: func() *bool { b := false; return &b }()})
	if seen == 0 {
		t.Errorf("OnCombatHitDamage never fired")
	}
	if got := hit.TotalDamage(); got != 42 {
		t.Errorf("mutated total = %d, want 42", got)
	}
}

func TestHooksAttackTimingIgnoreDelay(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	allowPvP(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	setPlayerLevels(target, map[int]int{SkillHitpoints: 99})
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker.GetCombat().Attack(&target.Mobile)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	target.GetCombat().ProcessHitQueue(100)
	// IgnoreDelay: the next cycle attacks despite the cooldown.
	Hooks.OnAttackTiming = func(attacker, target *Mobile, method CombatMethod, newTarget bool) *AttackTimingResult {
		return &AttackTimingResult{IgnoreDelay: true}
	}
	advance(101)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	if got := target.GetCombat().HitQueue().AccumulatedDamage(); got <= 0 {
		t.Errorf("ignore-delay attack queued no damage")
	}
}

func TestHooksCanAttackDeny(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	deny := false
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &deny }
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Errorf("denied policy = %v, want CANT_ATTACK_IN_AREA", got)
	}
}

func TestHooksAttackStartedFinished(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	allowPvP(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	started, finished := 0, 0
	Hooks.OnCombatAttackStarted = func(a, b *Mobile, m CombatMethod) { started++ }
	Hooks.OnCombatAttackFinished = func(a, b *Mobile, m CombatMethod) { finished++ }
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker.GetCombat().Attack(&target.Mobile)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	if started != 1 || finished != 1 {
		t.Errorf("started=%d finished=%d, want 1/1", started, finished)
	}
}

func TestHooksDeathEvents(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	before, after := 0, 0
	Hooks.OnNpcBeforeDeath = func(e *CombatBeforeDeathEvent) { before++ }
	Hooks.OnNpcDeath = func(e *CombatDeathEvent) { after++ }
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	npc.Hitpoints = 10
	killer, _ := combatTestPlayer(t, "killer", 3000, 3000)
	npc.GetCombat().AddDamage(&killer.Mobile, 10)
	DecrementHealth(&npc.Mobile, NewHitDamage(10, HitMaskRed))
	if before != 1 {
		t.Errorf("before-death fired %d times, want 1", before)
	}
	advance(102)
	npc.GetCombat().ProcessHitQueue(102)
	if after != 1 {
		t.Errorf("death fired %d times, want 1", after)
	}
}

func TestHooksManipulateHitCancel(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	// A nil return from ManipulateHit cancels the hit.
	Hooks.ManipulateHit = func(target *Mobile, hit *PendingHit) *PendingHit { return nil }
	hit := fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 5)
	AddPendingHit(hit)
	advance(100)
	victim.GetCombat().ProcessHitQueue(100)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("cancelled hit landed: HP = %d", got)
	}
}
