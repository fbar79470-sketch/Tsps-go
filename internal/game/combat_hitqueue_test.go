package game

import "testing"

func TestHitDamageMaskRules(t *testing.T) {
	// Damage > 0 with a BLUE start mask becomes RED.
	d := NewHitDamage(5, HitMaskBlue)
	if d.Mask() != HitMaskRed {
		t.Errorf("positive damage mask = %v, want RED", d.Mask())
	}
	// Zero damage stays BLUE with damage 0.
	d = NewHitDamage(0, HitMaskRed)
	if d.Mask() != HitMaskBlue || d.Damage() != 0 {
		t.Errorf("zero damage = {%d,%v}, want {0,BLUE}", d.Damage(), d.Mask())
	}
	// A green (poison) mask survives positive damage.
	d = NewHitDamage(3, HitMaskGreen)
	if d.Mask() != HitMaskGreen {
		t.Errorf("green mask = %v, want GREEN", d.Mask())
	}
	// multiplyDamage floors: 17 * 0.6 = 10.2 -> 10.
	d = NewHitDamage(17, HitMaskRed)
	d.MultiplyDamage(0.6)
	if d.Damage() != 10 {
		t.Errorf("17*0.6 = %d, want 10", d.Damage())
	}
	// Max-hit tracking.
	d = NewHitDamage(10, HitMaskRed)
	d.SetRolledMaximum(10)
	if !d.IsMaxHit() {
		t.Errorf("damage == rolledMaximum should be a max hit")
	}
	d.SetDamage(9)
	if d.IsMaxHit() {
		t.Errorf("damage below rolledMaximum should not be a max hit")
	}
	// Observers fire once and are consumed.
	called := 0
	d.OnApplied(func(lost int) {
		called++
		if lost != 9 {
			t.Errorf("observer lost = %d, want 9", lost)
		}
	})
	d.NotifyApplied(9)
	d.NotifyApplied(9)
	if called != 1 {
		t.Errorf("observer called %d times, want 1", called)
	}
}

func TestPendingHitFixedAndStatusDamage(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	fixed := fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 7)
	if got := fixed.TotalDamage(); got != 7 {
		t.Errorf("fixed total = %d, want 7", got)
	}
	if fixed.Hits()[0].RolledMaximum() != 0 {
		t.Errorf("fixed hit rolledMaximum = %d, want 0", fixed.Hits()[0].RolledMaximum())
	}
	// Status damage: capped to target HP, RED, no XP, bypassed.
	status := NewPendingHitConfig(&attacker.Mobile, &victim.Mobile, MeleeMethod,
		PendingHitConfig{StatusDamage: true, Damage: func() *int { d := 500; return &d }()})
	if got := status.TotalDamage(); got != 99 {
		t.Errorf("status damage = %d, want 99 (HP cap)", got)
	}
	if status.RewardsExperience() {
		t.Errorf("status damage should not reward XP")
	}
	if !status.BypassesProtectionPrayer() {
		t.Errorf("status damage should bypass protection prayers")
	}
	if hits := status.Hits(); len(hits) != 1 || hits[0].Mask() != HitMaskRed {
		t.Errorf("status hits = %v, want one RED splat", hits)
	}
}

func TestHitQueueTiming(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	hit := fixedDamageHitDelayed(t, &attacker.Mobile, &victim.Mobile, 5, 2)
	// Player targets get +0 processing delay; reveal at cycle+delay.
	AddPendingHit(hit)
	advance(101)
	victim.GetCombat().ProcessHitQueue(101)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("hit landed early: HP = %d", got)
	}
	advance(102)
	victim.GetCombat().ProcessHitQueue(102)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 94 {
		t.Errorf("HP after reveal = %d, want 94", got)
	}
	if n := len(victim.GetCombat().TickHits()); n != 1 {
		t.Errorf("tick hits = %d, want 1", n)
	}
	// NPC targets get +1 processing delay.
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	hit2 := fixedDamageHit(t, &attacker.Mobile, &npc.Mobile, 4)
	AddPendingHit(hit2)
	advance(102)
	npc.GetCombat().ProcessHitQueue(102)
	if hp, _ := HitpointsOf(&npc.Mobile); hp != 10 {
		t.Errorf("NPC hit landed early: HP = %d", hp)
	}
	advance(103)
	npc.GetCombat().ProcessHitQueue(103)
	if hp, _ := HitpointsOf(&npc.Mobile); hp != 6 {
		t.Errorf("NPC HP after reveal = %d, want 6", hp)
	}
}

func TestHitQueueClearIncoming(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	hit := fixedDamageHitDelayed(t, &attacker.Mobile, &victim.Mobile, 5, 5)
	AddPendingHit(hit)
	q := victim.GetCombat().HitQueue()
	gen := q.IncomingGeneration()
	q.ClearIncoming()
	if q.IncomingGeneration() != gen+1 {
		t.Errorf("generation = %d, want %d", q.IncomingGeneration(), gen+1)
	}
	advance(110)
	victim.GetCombat().ProcessHitQueue(110)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("cleared hit still landed: HP = %d", got)
	}
}

func TestHitQueueDropHitsFrom(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	other, _ := combatTestPlayer(t, "other", 3002, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	AddPendingHit(fixedDamageHitDelayed(t, &attacker.Mobile, &victim.Mobile, 5, 1))
	AddPendingHit(fixedDamageHitDelayed(t, &other.Mobile, &victim.Mobile, 3, 1))
	DropHitsFrom(&attacker.Mobile)
	q := victim.GetCombat().HitQueue()
	if q.HasPendingHitFrom(&attacker.Mobile) {
		t.Errorf("attacker's hits should have been dropped")
	}
	if !q.HasPendingHitFrom(&other.Mobile) {
		t.Errorf("other's hits should survive the drop")
	}
	if got := q.AccumulatedDamage(); got != 3 {
		t.Errorf("accumulated damage after drop = %d, want 3", got)
	}
}

func TestHitQueueStallHoldsIncoming(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	// A full stall holding hits delays incoming damage.
	victim.GetCombat().SetAnimationStall(5)
	hit := fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 5)
	AddPendingHit(hit)
	advance(101)
	victim.GetCombat().ProcessHitQueue(101)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Errorf("stalled hit landed: HP = %d", got)
	}
	// After the stall ends the held damage lands.
	advance(110)
	victim.GetCombat().ProcessHitQueue(110)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 94 {
		t.Errorf("HP after stall = %d, want 94", got)
	}
}

func TestHitDamageCacheExpiry(t *testing.T) {
	c := NewHitDamageCache(5, 100)
	c.Increment(3, 105)
	if got := c.Damage(); got != 8 {
		t.Errorf("cache damage = %d, want 8", got)
	}
	if got := c.ElapsedTicks(205); got != 100 {
		t.Errorf("elapsed = %d, want 100", got)
	}
}
