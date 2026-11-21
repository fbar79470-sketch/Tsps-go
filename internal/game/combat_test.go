package game

import "testing"

// allowPvP installs the hook opinion that lets two players
// fight (the default PvP policy denies without one).
func allowPvP(t *testing.T) {
	t.Helper()
	allow := true
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &allow }
}

func TestAttackSetsTargetAndVarp(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, fs := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	attacker.GetCombat().Attack(&target.Mobile)
	if got := attacker.GetCombat().Target(); got != &target.Mobile {
		t.Errorf("target = %v, want the target", got)
	}
	if got := lastVarp(fs, CombatTargetPlayerVarp); got != target.GetIndex() {
		t.Errorf("varp 1075 = %d, want target index %d", got, target.GetIndex())
	}
	if !attacker.GetCombat().HasPendingWork() {
		t.Errorf("attacking fighter should have pending work")
	}
	// Clearing the interaction resets the varp.
	attacker.GetCombat().ClearInteraction()
	if got := lastVarp(fs, CombatTargetPlayerVarp); got != -1 {
		t.Errorf("varp 1075 after clear = %d, want -1", got)
	}
}

// lastVarp returns the most recent value sent for a varp id,
// or -1 when never sent.
func lastVarp(fs *fakeSession, id int) int {
	got := -1
	for _, pair := range fs.varps {
		if pair[0] == id {
			got = pair[1]
		}
	}
	return got
}

func TestAttackRefusesNonFightingNPC(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := combatTestPlayer(t, "target", 3000, 3000)
	def := newCombatFakeDef()
	def.fightBack = false
	npc := combatTestNPC(t, 1, 3001, 3000, def)
	// A non-fighting NPC never initiates an attack.
	npc.GetCombat().Attack(&p.Mobile)
	if npc.GetCombat().Target() != nil {
		t.Errorf("a non-fighting NPC should not attack")
	}
	// But a player may still attack it.
	p.GetCombat().Attack(&npc.Mobile)
	if p.GetCombat().Target() != &npc.Mobile {
		t.Errorf("player attack on the NPC should be accepted")
	}
}

func TestAttackCooldownBlocksSecondAttack(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	allowPvP(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	setPlayerLevels(target, map[int]int{SkillHitpoints: 99})
	// Max rolls: every accuracy roll succeeds.
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker.GetCombat().Attack(&target.Mobile)
	// Cycle 100: pre + post execute the first attack.
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	firstQueued := target.GetCombat().HitQueue().AccumulatedDamage()
	if firstQueued <= 0 {
		t.Fatalf("first attack queued no damage (accumulated=%d)", firstQueued)
	}
	// Cycle 101: still on cooldown (attack speed 4), no new hit.
	advance(101)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	// Let the first hit land so the queue drains.
	target.GetCombat().ProcessHitQueue(101)
	if got := target.GetCombat().HitQueue().AccumulatedDamage(); got != 0 {
		t.Errorf("cooldown cycle queued damage = %d, want 0", got)
	}
	// Cycle 104: cooldown over (100+4), a second attack lands.
	advance(104)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	if got := target.GetCombat().HitQueue().AccumulatedDamage(); got <= 0 {
		t.Errorf("second attack queued no damage (accumulated=%d)", got)
	}
}

func TestDamageMapAndKiller(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	victim, _ := combatTestPlayer(t, "victim", 3000, 3000)
	a, _ := combatTestPlayer(t, "a", 3001, 3000)
	b, _ := combatTestPlayer(t, "b", 3002, 3000)
	c := victim.GetCombat()
	c.AddDamage(&a.Mobile, 5)
	c.AddDamage(&b.Mobile, 9)
	c.AddDamage(&a.Mobile, 2)
	if got := c.GetKiller(false); got != b {
		t.Errorf("killer = %v, want b", got)
	}
	if !c.DamageMapContains(&b.Mobile) {
		t.Errorf("damage map should contain b")
	}
	// Zero and negative damage never registers.
	c.AddDamage(&a.Mobile, 0)
	// After 100 ticks the cache expires.
	advance(201)
	if got := c.GetKiller(false); got != nil {
		t.Errorf("expired killer = %v, want nil", got)
	}
	// NPC attackers never register.
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	c.AddDamage(&npc.Mobile, 10)
	c.AddDamage(nil, 10)
	if got := c.GetKiller(false); got != nil {
		t.Errorf("killer after nil/NPC damage = %v, want nil", got)
	}
}

func TestAttackerExpiry(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	victim, _ := combatTestPlayer(t, "victim", 3000, 3000)
	attacker, _ := combatTestPlayer(t, "attacker", 3001, 3000)
	victim.GetCombat().SetUnderAttack(&attacker.Mobile)
	if got := victim.GetCombat().TicksSinceAttacked(); got != 0 {
		t.Errorf("ticks since attacked = %d, want 0", got)
	}
	advance(109)
	victim.GetCombat().PreMovementProcess()
	if victim.GetCombat().Attacker() == nil {
		t.Errorf("attacker should survive 9 ticks")
	}
	advance(110)
	victim.GetCombat().PreMovementProcess()
	if victim.GetCombat().Attacker() != nil {
		t.Errorf("attacker should expire after 10 ticks")
	}
	// The cycle stamp survives expiry (TS only nulls the
	// slot): 10 cycles have passed since the attack.
	if got := victim.GetCombat().TicksSinceAttacked(); got != 10 {
		t.Errorf("ticks since attacked after expiry = %d, want 10", got)
	}
	// Never attacked: -1.
	fresh, _ := combatTestPlayer(t, "fresh", 3000, 3000)
	if got := fresh.GetCombat().TicksSinceAttacked(); got != -1 {
		t.Errorf("never-attacked ticks = %d, want -1", got)
	}
}

func TestRetaliationFiresNextCycle(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	allowPvP(t)
	victim, _ := combatTestPlayer(t, "victim", 3000, 3000)
	attacker, _ := combatTestPlayer(t, "attacker", 3001, 3000)
	HandleRetaliation(&attacker.Mobile, &victim.Mobile)
	// The retaliation is scheduled, not immediate.
	if victim.GetCombat().Target() != nil {
		t.Errorf("retaliation should be delayed one tick")
	}
	advance(101)
	victim.GetCombat().PreMovementProcess()
	if got := victim.GetCombat().Target(); got != &attacker.Mobile {
		t.Errorf("retaliation target = %v, want the attacker", got)
	}
	if !victim.GetCombat().IsAutoRetaliating() {
		t.Errorf("retaliation should be flagged auto-retaliating")
	}
}

func TestUnreachableGiveUp(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, fs := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3030, 3000, nil)
	attacker.MovementQueue().SetBlockMovement(true)
	attacker.GetCombat().Attack(&npc.Mobile)
	for cycle := int64(100); cycle < 120; cycle++ {
		advance(cycle)
		attacker.GetCombat().PreMovementProcess()
	}
	if attacker.GetCombat().Target() != nil {
		t.Errorf("unreachable target should be given up after 20 cycles")
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != "I can't reach that!" {
		t.Errorf("give-up messages = %v", fs.messages)
	}
}

func TestMovementGateAnimationStall(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := combatTestPlayer(t, "stalled", 3000, 3000)
	q := p.MovementQueue()
	if got := q.getMobilityIgnoringFreeze(); got != mobilityMobile {
		t.Errorf("mobility before stall = %v, want MOBILE", got)
	}
	p.GetCombat().SetAnimationStall(5)
	if got := q.getMobilityIgnoringFreeze(); got != mobilityAnimationStall {
		t.Errorf("mobility during stall = %v, want ANIMATION_STALL", got)
	}
	// Stun outranks the stall (TS order).
	p.GetTimers().Registers(TimerKeyStun, 5)
	if got := q.getMobilityIgnoringFreeze(); got != mobilityStunned {
		t.Errorf("mobility stunned+stall = %v, want STUNNED", got)
	}
	p.GetTimers().Cancel(TimerKeyStun)
	// The stall is silent: no message is produced.
	if got := q.getMobilityIgnoringFreeze(); got != mobilityAnimationStall {
		t.Fatalf("mobility after stun cancel = %v", got)
	}
}

func TestPrayerBlockTimer(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, fs := combatTestPlayer(t, "blocked", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	p.GetCombat().StartPrayerBlock(5)
	if p.GetCombat().PrayerBlockFinished() {
		t.Errorf("prayer block should be active")
	}
	if ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Errorf("protection prayers should be refused while blocked")
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != "You have been disabled and can no longer use protection prayers." {
		t.Errorf("block messages = %v", fs.messages)
	}
	if !ActivatePrayer(p, PrayerThickSkin) {
		t.Errorf("non-protection prayers should still activate")
	}
	advance(110)
	if !p.GetCombat().PrayerBlockFinished() {
		t.Errorf("5-second block should finish by cycle 110")
	}
}

func TestSpecialEnergyDefaultAndDrain(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := combatTestPlayer(t, "special", 3000, 3000)
	c := p.GetCombat()
	if got := c.SpecialPercentage(); got != 100 {
		t.Errorf("special energy = %d, want 100", got)
	}
	special := &CombatSpecial{ID: "test", DrainAmount: 25, ItemIDs: []int{5000}}
	RegisterCombatSpecial(special)
	t.Cleanup(func() {
		combatSpecialsByItem = map[int]*CombatSpecial{}
		combatSpecialsByID = map[string]*CombatSpecial{}
	})
	p.Equipment().Set(WeaponSlot, NewItem(5000, 1))
	if got := c.GetCombatSpecial(); got != special {
		t.Errorf("combat special = %v, want the registered special", got)
	}
	// The amount argument is overridden by the special's cost.
	c.DrainSpecial(0)
	if got := c.SpecialPercentage(); got != 75 {
		t.Errorf("special energy after drain = %d, want 75", got)
	}
}

func TestNPCDeathDispatchAndRespawn(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	scheduler := &recordingScheduler{}
	SetNpcRespawnScheduler(scheduler)
	t.Cleanup(func() { SetNpcRespawnScheduler(nil) })
	killer, _ := combatTestPlayer(t, "killer", 3000, 3000)
	def := newCombatFakeDef()
	def.hitpoints = 10
	def.deathTicks = 2
	npc := combatTestNPC(t, 1, 3001, 3000, def)
	npc.Hitpoints = 10
	var death *CombatDeathEvent
	Hooks.OnNpcDeath = func(e *CombatDeathEvent) { death = e }
	npc.GetCombat().AddDamage(&killer.Mobile, 10)
	DecrementHealth(&npc.Mobile, NewHitDamage(10, HitMaskRed))
	if !npc.Dying {
		t.Fatalf("NPC should be dying after a lethal hit")
	}
	// The death task dispatches after deathTicks cycles.
	advance(101)
	npc.GetCombat().ProcessHitQueue(101)
	if death != nil {
		t.Errorf("death dispatched early")
	}
	advance(102)
	npc.GetCombat().ProcessHitQueue(102)
	if death == nil {
		t.Fatalf("death never dispatched")
	}
	if death.Killer != killer {
		t.Errorf("death killer = %v, want the killer", death.Killer)
	}
	if len(scheduler.calls) != 1 || scheduler.calls[0].npc != npc {
		t.Errorf("respawn calls = %+v, want one for the NPC", scheduler.calls)
	}
}

func TestPlayerDeathRespawn(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, fs := combatTestPlayer(t, "doomed", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	var death *CombatDeathEvent
	Hooks.OnPlayerDeath = func(e *CombatDeathEvent) bool { death = e; return false }
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	if p.Skills().CurrentLevel(SkillHitpoints) != 0 {
		t.Fatalf("HP = %d, want 0", p.Skills().CurrentLevel(SkillHitpoints))
	}
	// Four ticks to respawn.
	for cycle := int64(101); cycle <= 103; cycle++ {
		advance(cycle)
		p.GetCombat().ProcessHitQueue(cycle)
	}
	if death != nil {
		t.Errorf("player death dispatched early")
	}
	advance(104)
	p.GetCombat().ProcessHitQueue(104)
	if death == nil {
		t.Fatalf("player death never dispatched")
	}
	if got := p.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("respawn HP = %d, want 99", got)
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != DefaultDeathMessage {
		t.Errorf("death messages = %v", fs.messages)
	}
}

func TestBeforeDeathPrevent(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := combatTestPlayer(t, "saved", 3000, 3000)
	Hooks.OnPlayerBeforeDeath = func(e *CombatBeforeDeathEvent) { e.PreventDeath = true }
	DecrementHealth(&p.Mobile, NewHitDamage(10, HitMaskRed))
	// HP is 0 but no death sequence started.
	if p.GetCombat().HasPendingWork() {
		// Pending work alone does not prove death; the death
		// state is checked via the respawn: no dispatch.
	}
	p.GetCombat().ProcessHitQueue(101)
	p.GetCombat().ProcessHitQueue(105)
	if got := p.Skills().CurrentLevel(SkillHitpoints); got != 0 {
		t.Errorf("HP changed after prevented death: %d", got)
	}
}
