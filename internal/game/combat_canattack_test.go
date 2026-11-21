package game

import "testing"

// onCanAttack gate tests (phase 26): the CombatHooks.OnCanAttack
// seam and its emit sites in CanAttackPermission (the duel branch)
// and CanAttackByPolicy (the global hook), grounded in
// PluginManager.emitCanAttack (PluginManager.ts:1360-1377) and
// CombatFactory.canAttackPermission / canAttackByPolicy
// (CombatFactory.ts:779-789, 837-872). The hub-side allow-veto
// contract (reset per hook, deny short-circuits, allow sticks,
// silence undecided) is tested in internal/plugins (canattack
// tests); here the seam routing, the method passthrough, and
// the duel-branch ordering are pinned. There is no legacy
// fallback: the Phase 16 method-less CanAttack field was
// removed in phase 30 — TS has only the method-carrying emit —
// and its test assignments migrated to OnCanAttack.

func TestOnCanAttackGateAllowVetoNoHook(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)

	// No hook: PvE passes on the policy default.
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("no hook PvE = %v, want CAN_ATTACK", got)
	}

	// A veto denies through the policy.
	deny := false
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &deny }
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Fatalf("vetoed = %v, want CANT_ATTACK_IN_AREA", got)
	}

	// An allow passes.
	allow := true
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &allow }
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("allowed = %v, want CAN_ATTACK", got)
	}

	// No opinion passes PvE on the default, like no hook.
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return nil }
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("undecided PvE = %v, want CAN_ATTACK", got)
	}
}

func TestOnCanAttackGateMethodPassthrough(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)

	var seen CombatMethod
	seenCalls := 0
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool {
		seen, seenCalls = m, seenCalls+1
		return nil
	}
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, RangedMethod); got != CanAttackOK {
		t.Fatalf("permission = %v, want CAN_ATTACK", got)
	}
	if seenCalls != 1 || seen != RangedMethod {
		t.Fatalf("gate saw method %v in %d calls, want RangedMethod in 1 call", seen, seenCalls)
	}

	// A nil method reaches the gate as nil (TS undefined, e.g.
	// canAttackByPolicy called without a method).
	seenCalls = 0
	if got := CanAttackByPolicy(&attacker.Mobile, &npc.Mobile, nil); got != CanAttackOK {
		t.Fatalf("nil-method policy = %v, want CAN_ATTACK", got)
	}
	if seenCalls != 1 || seen != nil {
		t.Fatalf("gate saw method %v in %d calls, want nil in 1 call", seen, seenCalls)
	}
}

func TestOnCanAttackGateDuelBranch(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, target := duelPair(t, DuelStateInDuel, DuelStateInDuel)

	// No hook at all: agreed opponents in a duel can fight.
	// TS (CombatFactory.ts:782-789) runs emitCanAttack alone
	// here — null is not a veto — so the area/PvP policy never
	// denies them.
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("no-hook duel = %v, want CAN_ATTACK", got)
	}

	// A veto denies agreed opponents.
	deny := false
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &deny }
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Fatalf("vetoed duel = %v, want CANT_ATTACK_IN_AREA", got)
	}

	// An allow passes the gate, and the later duel-rule check
	// still applies: melee disabled refuses with the duel rule,
	// proving the gate runs before the duel rules.
	allow := true
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &allow }
	attacker.Dueling().Rules()[DuelRuleNoMelee.ButtonID()] = true
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackDuelMeleeDisabled {
		t.Fatalf("allowed + no-melee duel = %v, want DUEL_MELEE_DISABLED", got)
	}
	attacker.Dueling().Rules()[DuelRuleNoMelee.ButtonID()] = false
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("allowed duel = %v, want CAN_ATTACK", got)
	}
}

func TestOnCanAttackGateSingleField(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)

	// The gate is single-field since phase 30: OnCanAttack
	// alone decides, and there is no method-less fallback to
	// consult when it is unset (a veto denies, an allow
	// passes, no opinion falls through to the policy default).
	deny := false
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &deny }
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Fatalf("veto = %v, want CANT_ATTACK_IN_AREA", got)
	}

	allow := true
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &allow }
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("allow = %v, want CAN_ATTACK", got)
	}
}

func TestOnCanAttackGatePvPDefault(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)

	// No opinion: PvP is denied by the policy default.
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Fatalf("no-hook PvP = %v, want CANT_ATTACK_IN_AREA", got)
	}

	// An allow lifts the PvP default.
	allow := true
	Hooks.OnCanAttack = func(a, b *Mobile, m CombatMethod) *bool { return &allow }
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("allowed PvP = %v, want CAN_ATTACK", got)
	}
}
