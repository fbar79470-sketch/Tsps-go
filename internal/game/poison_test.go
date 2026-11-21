package game

import (
	"testing"
)

// Poison effect, poisonEntity, cure, orb, and save
// round-trip tests (CombatPoisonEffect.ts,
// CombatFactory.poisonEntity). Hand-built fixtures only.

// poisonTick drives count ProcessHitQueue cycles for a
// combat, advancing the shared cycle source in step.
func poisonTick(t *testing.T, c *Combat, advance func(int64), from, count int64) {
	t.Helper()
	for i := int64(0); i < count; i++ {
		cycle := from + i
		advance(cycle)
		c.ProcessHitQueue(cycle)
	}
}

func TestDamageFromSeverity(t *testing.T) {
	cases := []struct {
		severity, want int
	}{
		{30, 6}, {25, 5}, {20, 4}, {16, 4}, {11, 3}, {6, 2}, {1, 1}, {0, 0},
	}
	for _, tc := range cases {
		if got := DamageFromSeverity(tc.severity); got != tc.want {
			t.Errorf("DamageFromSeverity(%d) = %d, want %d", tc.severity, got, tc.want)
		}
	}
}

func TestPoisonSeveritiesAndRollRange(t *testing.T) {
	if got := GetMeleeSeverity(PoisonTypeExtra); got != 25 {
		t.Errorf("melee EXTRA = %d, want 25", got)
	}
	if got := GetMeleeSeverity(PoisonTypeSuper); got != 30 {
		t.Errorf("melee SUPER = %d, want 30", got)
	}
	if got := GetMeleeSeverity(PoisonTypeVenom); got != 6 {
		t.Errorf("melee VENOM = %d, want 6", got)
	}
	if got := GetMeleeSeverity(PoisonTypeMild); got != 20 {
		t.Errorf("melee default = %d, want 20", got)
	}
	if got := GetRangedSeverity(PoisonTypeWeak); got != 11 {
		t.Errorf("ranged WEAK = %d, want 11", got)
	}
	if got := GetRangedSeverity(PoisonTypeMild); got != 16 {
		t.Errorf("ranged MILD = %d, want 16", got)
	}
	if got := GetRangedSeverity(PoisonTypeVenom); got != 6 {
		t.Errorf("ranged VENOM = %d, want 6", got)
	}
	if got := GetRangedSeverity(PoisonTypeSuper); got != 6 {
		t.Errorf("ranged default = %d, want 6", got)
	}
	if got := GetMeleeRollRange(NoxiousHalberdID); got != 2 {
		t.Errorf("noxious roll range = %d, want 2", got)
	}
	if got := GetMeleeRollRange(NoxiousHalberd4ID); got != 2 {
		t.Errorf("noxious (4) roll range = %d, want 2", got)
	}
	if got := GetMeleeRollRange(5696); got != 3 {
		t.Errorf("ordinary roll range = %d, want 3", got)
	}
}

func TestPoisonEffectDecaysAndStops(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 100
	PoisonEntity(&npc.Mobile, 20, 1)
	if !npc.IsPoisoned() || npc.PoisonDamage() != 20 {
		t.Fatalf("poisoned NPC damage = %d poisoned = %v, want 20 true", npc.PoisonDamage(), npc.IsPoisoned())
	}
	advance := setCombatCycle(t, 0)
	// No damage before the 30-tick pulse.
	poisonTick(t, npc.GetCombat(), advance, 1, 29)
	if npc.Hitpoints != 100 {
		t.Errorf("HP after 29 ticks = %d, want 100", npc.Hitpoints)
	}
	// Tick 30: floor((20+4)/5) = 4 damage, severity 19.
	poisonTick(t, npc.GetCombat(), advance, 30, 1)
	if npc.Hitpoints != 96 {
		t.Errorf("HP after first pulse = %d, want 96", npc.Hitpoints)
	}
	if got := npc.PoisonDamage(); got != 19 {
		t.Errorf("severity after first pulse = %d, want 19", got)
	}
	// Second pulse (tick 60): floor((19+4)/5) = 4, severity 18.
	poisonTick(t, npc.GetCombat(), advance, 31, 30)
	if npc.Hitpoints != 92 {
		t.Errorf("HP after second pulse = %d, want 92", npc.Hitpoints)
	}
	if got := npc.PoisonDamage(); got != 18 {
		t.Errorf("severity after second pulse = %d, want 18", got)
	}
	// The hits are green splats in the hit queue.
	if got := npc.GetCombat().HitQueue().PoisonSplatCount(); got != 2 {
		t.Errorf("green splats = %d, want 2", got)
	}
	// Run the severity out: 18 more pulses take it to 0 and
	// the effect stops (severity 1's hit is the last).
	poisonTick(t, npc.GetCombat(), advance, 61, 18*30)
	if npc.IsPoisoned() {
		t.Errorf("severity should have decayed to 0, got %d", npc.PoisonDamage())
	}
	// Total poison damage: sum floor((s+4)/5) for s = 20..1.
	wantHP := 100
	for s := 20; s >= 1; s-- {
		wantHP -= DamageFromSeverity(s)
	}
	if npc.Hitpoints != wantHP {
		t.Errorf("HP after full decay = %d, want %d", npc.Hitpoints, wantHP)
	}
	// A stopped effect ticks no further damage.
	poisonTick(t, npc.GetCombat(), advance, 61+18*30, 60)
	if npc.Hitpoints != wantHP {
		t.Errorf("HP after effect stopped = %d, want %d", npc.Hitpoints, wantHP)
	}
}

func TestVenomEscalatesToCap(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 500
	PoisonEntity(&npc.Mobile, 6, 2)
	if !npc.IsVenomed() || npc.PoisonDamage() != 6 {
		t.Fatalf("venomed NPC damage = %d venomed = %v, want 6 true", npc.PoisonDamage(), npc.IsVenomed())
	}
	advance := setCombatCycle(t, 0)
	poisonTick(t, npc.GetCombat(), advance, 1, 30)
	if npc.Hitpoints != 494 {
		t.Errorf("HP after first venom hit = %d, want 494 (6 damage)", npc.Hitpoints)
	}
	if got := npc.PoisonDamage(); got != 8 {
		t.Errorf("stored venom after first hit = %d, want 8", got)
	}
	if last := npc.LastVenomDamage(); last == nil || *last != 6 {
		t.Errorf("last venom damage = %v, want 6", last)
	}
	poisonTick(t, npc.GetCombat(), advance, 31, 30)
	if npc.Hitpoints != 486 {
		t.Errorf("HP after second venom hit = %d, want 486 (8 damage)", npc.Hitpoints)
	}
	// Escalate to the cap: hits 6,8,10,12,14,16,18,20,20,20.
	poisonTick(t, npc.GetCombat(), advance, 61, 8*30)
	if got := npc.PoisonDamage(); got != 20 {
		t.Errorf("stored venom at cap = %d, want 20", got)
	}
	wantHP := 500 - (6 + 8 + 10 + 12 + 14 + 16 + 18 + 20 + 20 + 20)
	if npc.Hitpoints != wantHP {
		t.Errorf("HP after 10 venom hits = %d, want %d", npc.Hitpoints, wantHP)
	}
	// Venom never decays or expires on its own.
	poisonTick(t, npc.GetCombat(), advance, 301, 30)
	if npc.Hitpoints != wantHP-20 {
		t.Errorf("venom should keep hitting 20, HP = %d, want %d", npc.Hitpoints, wantHP-20)
	}
}

func TestVenomCannotRestackOrDowngrade(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 100
	PoisonEntity(&npc.Mobile, 6, 2)
	PoisonEntity(&npc.Mobile, 30, 1) // poison must not replace venom
	if !npc.IsVenomed() || npc.PoisonDamage() != 6 {
		t.Errorf("re-poison changed venom: damage = %d venomed = %v", npc.PoisonDamage(), npc.IsVenomed())
	}
	PoisonEntity(&npc.Mobile, 6, 2) // re-venom must not restack
	if npc.PoisonDamage() != 6 {
		t.Errorf("re-venom restacked: damage = %d, want 6", npc.PoisonDamage())
	}
	// Poison takes the max: a stronger re-poison wins, a
	// weaker one leaves the value alone.
	other := combatTestNPC(t, 2, 3001, 3000, nil)
	other.Hitpoints = 100
	PoisonEntity(&other.Mobile, 20, 1)
	PoisonEntity(&other.Mobile, 25, 1)
	if got := other.PoisonDamage(); got != 25 {
		t.Errorf("stronger re-poison = %d, want 25", got)
	}
	PoisonEntity(&other.Mobile, 10, 1)
	if got := other.PoisonDamage(); got != 25 {
		t.Errorf("weaker re-poison = %d, want 25", got)
	}
	// Zero severity never applies.
	PoisonEntity(&other.Mobile, 0, 1)
	if got := other.PoisonDamage(); got != 25 {
		t.Errorf("zero severity changed damage to %d", got)
	}
}

func TestPoisonEntityNpcImmunity(t *testing.T) {
	// Venom-immune converts venom to poison, severity x 5.
	def := newCombatFakeDef()
	def.venomImmune = true
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	npc.Hitpoints = 100
	PoisonEntity(&npc.Mobile, 6, 2)
	if npc.IsVenomed() {
		t.Error("venom-immune NPC should not be venomed")
	}
	if got := npc.PoisonDamage(); got != 30 {
		t.Errorf("converted poison severity = %d, want 30 (6 x 5)", got)
	}
	// Poison-immune blocks poison outright (and converted venom).
	def2 := newCombatFakeDef()
	def2.poisonImmune = true
	immune := combatTestNPC(t, 2, 3001, 3000, def2)
	immune.Hitpoints = 100
	PoisonEntity(&immune.Mobile, 30, 1)
	if immune.IsPoisoned() {
		t.Error("poison-immune NPC should not be poisoned")
	}
	def3 := newCombatFakeDef()
	def3.poisonImmune = true
	def3.venomImmune = true
	both := combatTestNPC(t, 3, 3002, 3000, def3)
	both.Hitpoints = 100
	PoisonEntity(&both.Mobile, 6, 2)
	if both.IsPoisoned() {
		t.Error("venom converted to poison is still blocked by poison immunity")
	}
}

func TestPoisonEntityPlayerImmunityAndStatusVeto(t *testing.T) {
	player, _ := combatTestPlayer(t, "victim", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 99})
	// A running clock blocks poison.
	player.GetCombat().PoisonClock().Start(90)
	PoisonEntity(&player.Mobile, 20, 1)
	if player.IsPoisoned() {
		t.Error("running clock should block poison")
	}
	// ... and venom immunity (counter > 38) blocks venom.
	player.GetCombat().PoisonClock().Start(39 * 18)
	PoisonEntity(&player.Mobile, 6, 2)
	if player.IsVenomed() {
		t.Error("venom immunity should block venom")
	}
	// The combat:status-apply custom event may veto.
	player.GetCombat().PoisonClock().Stop()
	vetoed := false
	player.EmitCustomEvent = func(name string, payload any) {
		if name == "combat:status-apply" {
			if status, ok := payload.(*StatusApplyPayload); ok {
				vetoed = true
				if status.Kind != "poison" {
					t.Errorf("status kind = %q, want poison", status.Kind)
				}
				status.Allow = false
			}
		}
	}
	PoisonEntity(&player.Mobile, 20, 1)
	if !vetoed {
		t.Error("status-apply event was not emitted")
	}
	if player.IsPoisoned() {
		t.Error("vetoed poison should not apply")
	}
	// Without a veto, the message fires only from nothing.
	victim, vfs := combatTestPlayer(t, "victim3", 3000, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	PoisonEntity(&victim.Mobile, 20, 1)
	if !containsMessage(vfs.messages, "You have been poisoned!") {
		t.Errorf("messages = %v, want the poisoned line", vfs.messages)
	}
	// Re-poisoning an already-poisoned player says nothing.
	before := len(vfs.messages)
	PoisonEntity(&victim.Mobile, 25, 1)
	if len(vfs.messages) != before {
		t.Errorf("re-poison sent a message: %v", vfs.messages[before:])
	}
	// Venom from poison clears the clock counter and names venom.
	victim.GetCombat().PoisonClock().Start(90)
	PoisonEntity(&victim.Mobile, 6, 2)
	if !victim.IsVenomed() {
		t.Error("venom should override poison")
	}
	if victim.GetCombat().PoisonClock().HasCounter() {
		t.Error("accepted venom should clear the poison counter")
	}
}

func containsMessage(messages []string, want string) bool {
	for _, m := range messages {
		if m == want {
			return true
		}
	}
	return false
}

func TestCurePoison(t *testing.T) {
	player, fs := combatTestPlayer(t, "cured", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 99})
	// Poison first, then start the clock: immunity does not
	// cure an existing affliction, so both run together.
	PoisonEntity(&player.Mobile, 20, 1)
	player.GetCombat().PoisonClock().Start(90)
	advance := setCombatCycle(t, 0)
	poisonTick(t, player.GetCombat(), advance, 1, 30)
	if player.Skills().CurrentLevel(SkillHitpoints) != 95 {
		t.Fatalf("HP after poison hit = %d, want 95", player.Skills().CurrentLevel(SkillHitpoints))
	}
	CurePoison(&player.Mobile)
	if player.IsPoisoned() || player.IsVenomed() || player.PoisonDamage() != 0 {
		t.Errorf("cure left affliction: damage = %d venomed = %v", player.PoisonDamage(), player.IsVenomed())
	}
	// The clock keeps running: a cure is not a dose end.
	if !player.GetCombat().PoisonClock().HasCounter() {
		t.Error("cure should not stop the clock")
	}
	// The orb clears to the immunity value at the cure:
	// one pulse has run, so the counter is 4.
	lastVarp := fs.varps[len(fs.varps)-1]
	if lastVarp != [2]int{PoisonOrbVarp, -4} {
		t.Errorf("orb varp at cure = %v, want [102 -4] (counter 4 immunity)", lastVarp)
	}
	// No further damage ticks (the clock keeps pulsing
	// underneath, so the orb value moves on — only the
	// hitpoints are asserted here).
	poisonTick(t, player.GetCombat(), advance, 31, 60)
	if player.Skills().CurrentLevel(SkillHitpoints) != 95 {
		t.Errorf("HP after cure = %d, want 95", player.Skills().CurrentLevel(SkillHitpoints))
	}
}

func TestPoisonDeathClearsAffliction(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 3
	PoisonEntity(&npc.Mobile, 20, 1)
	advance := setCombatCycle(t, 0)
	// The first pulse deals 4, clamped to the 3 HP left.
	poisonTick(t, npc.GetCombat(), advance, 1, 30)
	if npc.Hitpoints != 0 {
		t.Fatalf("HP after lethal pulse = %d, want 0", npc.Hitpoints)
	}
	// The next tick clears the affliction (HP <= 0 branch).
	poisonTick(t, npc.GetCombat(), advance, 31, 1)
	if npc.IsPoisoned() || npc.PoisonDamage() != 0 {
		t.Errorf("death left affliction: damage = %d", npc.PoisonDamage())
	}
	// A player death stops the clock too.
	player, _ := combatTestPlayer(t, "dead", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 10})
	PoisonEntity(&player.Mobile, 20, 1)
	player.GetCombat().PoisonClock().Start(90)
	player.Skills().SetCurrentLevel(SkillHitpoints, 0, false)
	poisonTick(t, player.GetCombat(), advance, 32, 1)
	if player.IsPoisoned() || player.GetCombat().PoisonClock().IsRunning() {
		t.Errorf("player death left poison/clock: damage = %d running = %v",
			player.PoisonDamage(), player.GetCombat().PoisonClock().IsRunning())
	}
}

func TestPoisonSuspendResume(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 100
	PoisonEntity(&npc.Mobile, 20, 1)
	advance := setCombatCycle(t, 0)
	SuspendPoisonEffect(&npc.Mobile)
	// Suspended: no ticks, affliction retained.
	poisonTick(t, npc.GetCombat(), advance, 1, 60)
	if npc.Hitpoints != 100 || npc.PoisonDamage() != 20 {
		t.Errorf("suspended tick changed state: HP = %d damage = %d", npc.Hitpoints, npc.PoisonDamage())
	}
	// Resume restarts the 30-tick countdown from scratch.
	ResumePoisonEffect(&npc.Mobile)
	poisonTick(t, npc.GetCombat(), advance, 61, 29)
	if npc.Hitpoints != 100 {
		t.Errorf("HP after 29 resumed ticks = %d, want 100", npc.Hitpoints)
	}
	poisonTick(t, npc.GetCombat(), advance, 90, 1)
	if npc.Hitpoints != 96 {
		t.Errorf("HP after resumed pulse = %d, want 96", npc.Hitpoints)
	}
}

func TestPoisonOrbValue(t *testing.T) {
	player, _ := combatTestPlayer(t, "orb", 3000, 3000)
	if got := OrbValue(&player.Mobile); got != 0 {
		t.Errorf("clean orb = %d, want 0", got)
	}
	player.SetPoisonDamage(20)
	if got := OrbValue(&player.Mobile); got != 20 {
		t.Errorf("poison orb = %d, want 20 (severity)", got)
	}
	player.SetVenomed(true)
	player.SetPoisonDamage(6)
	if got := OrbValue(&player.Mobile); got != 1000000 {
		t.Errorf("venom orb at 6 = %d, want 1000000", got)
	}
	player.SetPoisonDamage(10)
	if got := OrbValue(&player.Mobile); got != 1000002 {
		t.Errorf("venom orb at 10 = %d, want 1000002 (round((10-6)/2))", got)
	}
	player.SetPoisonDamage(0)
	player.SetVenomed(false)
	player.GetCombat().PoisonClock().Start(90) // counter 5
	if got := OrbValue(&player.Mobile); got != -5 {
		t.Errorf("immunity orb = %d, want -5", got)
	}
	// NPC immunity: 90s -> -5; 2s or less -> 0.
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.GetCombat().StartPoisonImmunity(90)
	if got := OrbValue(&npc.Mobile); got != -5 {
		t.Errorf("NPC immunity orb = %d, want -5", got)
	}
	npc.GetCombat().StartPoisonImmunity(2)
	if got := OrbValue(&npc.Mobile); got != 0 {
		t.Errorf("NPC short immunity orb = %d, want 0", got)
	}
}

func TestPoisonResistanceLines(t *testing.T) {
	player, fs := combatTestPlayer(t, "resist", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 99})
	// Counter-only protection (no affliction): the effect
	// pulses the clock and reports the tier ends.
	player.GetCombat().PoisonClock().Start(36) // counter 2
	advance := setCombatCycle(t, 0)
	poisonTick(t, player.GetCombat(), advance, 1, 30)
	if !containsMessage(fs.messages, "<col=7f007f>Your poison resistance is about to wear off.</col>") {
		t.Errorf("messages after first pulse = %v, want the about-to-wear-off line", fs.messages)
	}
	poisonTick(t, player.GetCombat(), advance, 31, 30)
	if !containsMessage(fs.messages, "<col=7f007f>Your poison resistance has worn off.</col>") {
		t.Errorf("messages after second pulse = %v, want the worn-off line", fs.messages)
	}
	if player.GetCombat().PoisonClock().HasCounter() {
		t.Error("clock should be exhausted after two pulses")
	}
	// Venom tier: counter 39 -> 38 reports the venom end line.
	player2, fs2 := combatTestPlayer(t, "resist2", 3000, 3000)
	setPlayerLevels(player2, map[int]int{SkillHitpoints: 99})
	player2.GetCombat().PoisonClock().Start(39 * 18)
	poisonTick(t, player2.GetCombat(), advance, 61, 30)
	if !containsMessage(fs2.messages, "<col=7f007f>Your venom resistance has worn off, but you are still resistant to poison.</col>") {
		t.Errorf("venom messages = %v, want the venom worn-off line", fs2.messages)
	}
}

func TestPoisonReductionLines(t *testing.T) {
	player, fs := combatTestPlayer(t, "reduce", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 99})
	player.GetCombat().PoisonClock().Start(90) // counter 5
	// A reduction crossing into the venom tier's end (39->38
	// is the venom boundary; here 5->0 exhausts poison).
	player.GetCombat().PoisonClock().ReduceProtection(90)
	advance := setCombatCycle(t, 0)
	poisonTick(t, player.GetCombat(), advance, 1, 1)
	if !containsMessage(fs.messages, "<col=7f007f>Your poison resistance has worn off.</col>") {
		t.Errorf("reduction messages = %v, want the worn-off line", fs.messages)
	}
}

func TestPoisonSaveRoundTrip(t *testing.T) {
	player, _ := combatTestPlayer(t, "saved", 3000, 3000)
	setPlayerLevels(player, map[int]int{SkillHitpoints: 99})
	player.GetCombat().PoisonClock().Start(90) // counter 5
	PoisonEntity(&player.Mobile, 6, 2)
	save := player.GetCombat().PoisonSaveState()
	if save.Damage != 6 || !save.Venomed {
		t.Fatalf("save = %+v, want damage 6 venomed", save)
	}
	if save.Clock == nil || save.Clock.Counter != 0 {
		// Venom cleared the counter at application.
		t.Fatalf("save clock = %+v, want counter 0 after venom cleared it", save.Clock)
	}
	// Restore into a fresh player: fields, clock, and the
	// running effect all come back.
	fresh, _ := combatTestPlayer(t, "fresh", 3000, 3000)
	setPlayerLevels(fresh, map[int]int{SkillHitpoints: 99})
	fresh.GetCombat().RestorePoisonFromSave(save.Damage, save.Venomed, save.LastVenomDamage, save.Clock, 0)
	if !fresh.IsVenomed() || fresh.PoisonDamage() != 6 {
		t.Errorf("restored affliction: damage = %d venomed = %v", fresh.PoisonDamage(), fresh.IsVenomed())
	}
	advance := setCombatCycle(t, 0)
	poisonTick(t, fresh.GetCombat(), advance, 1, 30)
	if fresh.Skills().CurrentLevel(SkillHitpoints) != 93 {
		t.Errorf("restored venom did not hit: HP = %d, want 93", fresh.Skills().CurrentLevel(SkillHitpoints))
	}
	// Snapshot again: the round-trip is stable.
	save2 := fresh.GetCombat().PoisonSaveState()
	if save2.Damage != 8 || !save2.Venomed {
		t.Errorf("second save = %+v, want damage 8 venomed", save2)
	}
	// Legacy seconds rebuild the clock when no state survives.
	legacy, _ := combatTestPlayer(t, "legacy", 3000, 3000)
	legacy.GetCombat().RestorePoisonFromSave(0, false, nil, nil, 90)
	if got := legacy.GetCombat().PoisonClock().Counter(); got != 5 {
		t.Errorf("legacy restore counter = %d, want 5", got)
	}
	// A clean player snapshots no clock.
	clean, _ := combatTestPlayer(t, "clean", 3000, 3000)
	if save := clean.GetCombat().PoisonSaveState(); save.Clock != nil {
		t.Errorf("clean save clock = %+v, want nil", save.Clock)
	}
}

func TestPoisonEffectDoesNotDoubleTick(t *testing.T) {
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	npc.Hitpoints = 100
	PoisonEntity(&npc.Mobile, 20, 1)
	advance := setCombatCycle(t, 0)
	advance(1)
	// The same cycle twice must tick the effect once.
	npc.GetCombat().ProcessHitQueue(1)
	npc.GetCombat().ProcessHitQueue(1)
	poisonTick(t, npc.GetCombat(), advance, 2, 29)
	if npc.Hitpoints != 96 {
		t.Errorf("HP after 30 cycles with a double call = %d, want 96 (one pulse)", npc.Hitpoints)
	}
}
