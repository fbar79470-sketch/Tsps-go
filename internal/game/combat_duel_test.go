package game

import "testing"

// Duel gates in combat (CombatFactory.ts:718-731, 814-823,
// 894-897; PrayerHandler.ts:391-397).

// duelPair builds two registered, bound players and puts them
// in a mutual duel at the given states.
func duelPair(t *testing.T, attackerState, targetState DuelState) (attacker, target *Player) {
	t.Helper()
	attacker, _ = combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ = combatTestPlayer(t, "target", 3001, 3000)
	attacker.Dueling().SetInteract(target)
	target.Dueling().SetInteract(attacker)
	attacker.Dueling().SetState(attackerState)
	target.Dueling().SetState(targetState)
	return attacker, target
}

func TestCanAttackPermissionDuelWrongOpponent(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	// Attacker in a duel, target not the agreed opponent.
	attacker, target := duelPair(t, DuelStateInDuel, DuelStateNone)
	target.Dueling().SetInteract(nil)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackDuelWrongOpponent {
		t.Fatalf("one-sided duel = %v, want DUEL_WRONG_OPPONENT", got)
	}
	// Attacker in a duel, target an NPC: no duel state at all.
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackDuelWrongOpponent {
		t.Fatalf("duel vs NPC = %v, want DUEL_WRONG_OPPONENT", got)
	}
	// Target in a duel, attacker not: the duel is still
	// active, and the attacker is not the opponent.
	attacker2, target2 := duelPair(t, DuelStateNone, DuelStateInDuel)
	attacker2.Dueling().SetInteract(nil)
	if got := CanAttackPermission(&attacker2.Mobile, &target2.Mobile, true, MeleeMethod); got != CanAttackDuelWrongOpponent {
		t.Fatalf("attacker outside target's duel = %v, want DUEL_WRONG_OPPONENT", got)
	}
}

func TestCanAttackPermissionDuelNotStartedYet(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	// Mutual opponents, but the target is still starting.
	attacker, target := duelPair(t, DuelStateInDuel, DuelStateStartingDuel)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackDuelNotStartedYet {
		t.Fatalf("starting duel = %v, want DUEL_NOT_STARTED_YET", got)
	}
	// Both starting: same refusal.
	attacker.Dueling().SetState(DuelStateStartingDuel)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackDuelNotStartedYet {
		t.Fatalf("both starting = %v, want DUEL_NOT_STARTED_YET", got)
	}
}

func TestCanAttackPermissionDuelAgreedOpponent(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, target := duelPair(t, DuelStateInDuel, DuelStateInDuel)
	allow := true
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &allow }
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Fatalf("agreed opponents in duel = %v, want OK", got)
	}
}

func TestCanAttackPermissionDuelRules(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	allow := true
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &allow }
	attacker, target := duelPair(t, DuelStateInDuel, DuelStateInDuel)

	melee := &stubCombatMethod{methodType: CombatTypeMelee}
	ranged := &stubCombatMethod{methodType: CombatTypeRanged}
	magic := &stubCombatMethod{methodType: CombatTypeMagic}
	for _, tc := range []struct {
		name   string
		rule   DuelRule
		method CombatMethod
		want   CanAttackResponse
	}{
		{"melee disabled", DuelRuleNoMelee, melee, CanAttackDuelMeleeDisabled},
		{"ranged disabled", DuelRuleNoRanged, ranged, CanAttackDuelRangedDisabled},
		{"magic disabled", DuelRuleNoMagic, magic, CanAttackDuelMagicDisabled},
		{"melee allowed when ranged disabled", DuelRuleNoRanged, melee, CanAttackOK},
		{"ranged allowed when magic disabled", DuelRuleNoMagic, ranged, CanAttackOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for i := range attacker.Dueling().Rules() {
				attacker.Dueling().Rules()[i] = false
			}
			attacker.Dueling().Rules()[tc.rule.ButtonID()] = true
			if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, tc.method); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}

	// The rules do not apply outside a duel.
	attacker.Dueling().SetState(DuelStateNone)
	attacker.Dueling().Rules()[DuelRuleNoMelee.ButtonID()] = true
	target.Dueling().SetState(DuelStateNone)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, melee); got != CanAttackOK {
		t.Fatalf("rules outside a duel = %v, want OK", got)
	}
	// A nil method skips the rule check.
	attacker.Dueling().SetState(DuelStateInDuel)
	target.Dueling().SetState(DuelStateInDuel)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, nil); got != CanAttackOK {
		t.Fatalf("nil method in duel = %v, want OK", got)
	}
}

func TestCanAttackSecondaryTargetDuel(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	primary, _ := combatTestPlayer(t, "primary", 3001, 3000)
	candidate, _ := combatTestPlayer(t, "candidate", 3001, 3001)

	// Attacker in a duel: no splash, whoever the candidate is.
	attacker.Dueling().SetState(DuelStateInDuel)
	if CanAttackSecondaryTarget(&attacker.Mobile, &primary.Mobile, &candidate.Mobile, -1) {
		t.Fatal("attacker in a duel must exclude secondary targets")
	}
	attacker.Dueling().SetState(DuelStateNone)

	// Candidate in a duel: excluded even when the attacker is not.
	candidate.Dueling().SetState(DuelStateInDuel)
	if CanAttackSecondaryTarget(&attacker.Mobile, &primary.Mobile, &candidate.Mobile, -1) {
		t.Fatal("candidate in a duel must be excluded")
	}
	candidate.Dueling().SetState(DuelStateNone)

	// Control: neither in a duel, multi + policy allow.
	allow := true
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &allow }
	SetCombatMultiSource(func(c Combatant) bool { return true })
	t.Cleanup(func() { SetCombatMultiSource(nil) })
	if !CanAttackSecondaryTarget(&attacker.Mobile, &primary.Mobile, &candidate.Mobile, -1) {
		t.Fatal("no duel + multi + policy must allow the secondary target")
	}
}

func TestActivatePrayerDuelNoPrayer(t *testing.T) {
	resetCombatHooks(t)
	p, sess := combatTestPlayer(t, "Alice", 3000, 3000)
	p.Dueling().SetState(DuelStateInDuel)
	p.Dueling().Rules()[DuelRuleNoPrayer.ButtonID()] = true
	if ActivatePrayer(p, 0) {
		t.Fatal("NO_PRAYER in a duel must refuse activation")
	}
	if PrayerActive(&p.Mobile, 0) {
		t.Fatal("prayer must not be active after refusal")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("refusal is silent, got %v", sess.messages)
	}
	// The rule off: activation works.
	p.Dueling().Rules()[DuelRuleNoPrayer.ButtonID()] = false
	if !ActivatePrayer(p, 0) {
		t.Fatal("rule off must allow activation")
	}
	DeactivatePrayer(p, 0)
	// Outside a duel the rule does not apply.
	p.Dueling().Rules()[DuelRuleNoPrayer.ButtonID()] = true
	p.Dueling().SetState(DuelStateNone)
	if !ActivatePrayer(p, 0) {
		t.Fatal("rule outside a duel must allow activation")
	}
}

func TestHandleAttackDeniedDuelMessages(t *testing.T) {
	attacker, sess := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	combat := attacker.GetCombat()
	combat.target = &target.Mobile
	for _, tc := range []struct {
		response CanAttackResponse
		want     string
	}{
		{CanAttackDuelNotStartedYet, "The duel has not started yet!"},
		{CanAttackDuelWrongOpponent, "This is not your opponent!"},
		{CanAttackDuelMeleeDisabled, "Melee has been disabled in this duel!"},
		{CanAttackDuelRangedDisabled, "Ranged has been disabled in this duel!"},
		{CanAttackDuelMagicDisabled, "Magic has been disabled in this duel!"},
	} {
		combat.target = &target.Mobile
		combat.generation = 0
		combat.handleAttackDenied(tc.response, &target.Mobile, 0)
		if len(sess.messages) == 0 || sess.messages[len(sess.messages)-1] != tc.want {
			t.Fatalf("%v messages = %v, want %q", tc.response, sess.messages, tc.want)
		}
	}
}
