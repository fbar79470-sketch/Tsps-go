package game

import "testing"

// TestNPCContentID pins the transform seam (Phase 33):
// ContentID is the transformation id while one is set, else
// the spawned id (NPC.ts:694-696, getId; Mobile.ts:715-722).
// A zero-value NPC reads untransformed.
func TestNPCContentID(t *testing.T) {
	npc := NewNPC(7234, Location{X: 3200, Y: 3200, Z: 0})
	if got := npc.ContentID(); got != 7234 {
		t.Errorf("ContentID = %d, want 7234", got)
	}
	npc.SetNpcTransformationID(9474)
	if got := npc.ContentID(); got != 9474 {
		t.Errorf("transformed ContentID = %d, want 9474", got)
	}
	if npc.ID != 7234 {
		t.Errorf("ID after transform = %d, want 7234 (spawned id kept)", npc.ID)
	}
	npc.SetNpcTransformationID(-1)
	if got := npc.ContentID(); got != 7234 {
		t.Errorf("cleared ContentID = %d, want 7234", got)
	}
	zero := &NPC{ID: 5}
	if got := zero.ContentID(); got != 5 {
		t.Errorf("zero-value ContentID = %d, want 5", got)
	}
}

// TestNPCHitpointsLocked pins the hitpoints lock (Phase 33):
// a locked NPC shows the hit but keeps its HP
// (Mobile.ts:732-735, decrementHealth); an unlocked NPC at
// the same HP dies from it.
func TestNPCHitpointsLocked(t *testing.T) {
	advance := setCombatCycle(t, 100)
	_ = advance
	resetCombatHooks(t)
	locked := combatTestNPC(t, 7234, 3001, 3000, nil)
	locked.Hitpoints = 1
	locked.SetHitpointsLocked(true)
	hit := NewHitDamage(5, HitMaskRed)
	DecrementHealth(&locked.Mobile, hit)
	if locked.Hitpoints != 1 {
		t.Errorf("locked hitpoints = %d, want 1", locked.Hitpoints)
	}
	if hit.HealthLost() != 0 {
		t.Errorf("locked health lost = %d, want 0", hit.HealthLost())
	}
	if locked.IsHitpointsLocked() != true {
		t.Error("want IsHitpointsLocked true")
	}
	locked.SetHitpointsLocked(false)
	if locked.IsHitpointsLocked() {
		t.Error("want IsHitpointsLocked false after clear")
	}
	unlocked := combatTestNPC(t, 7234, 3002, 3000, nil)
	unlocked.Hitpoints = 1
	DecrementHealth(&unlocked.Mobile, NewHitDamage(5, HitMaskRed))
	if unlocked.Hitpoints != 0 {
		t.Errorf("unlocked hitpoints = %d, want 0", unlocked.Hitpoints)
	}
}

// TestNPCDeathRemainsDelay pins the remains delay (Phase 33):
// a death whose hook sets Remains stays registered for
// RemainsTicks before the removal runs (NPCDeathTask.ts:82-88,
// NPCRemainsTask), while a death without remains removes at
// dispatch.
func TestNPCDeathRemainsDelay(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	removed := 0
	var removedNPC *NPC
	SetNpcRemover(func(n *NPC) {
		removed++
		removedNPC = n
	})
	t.Cleanup(func() { SetNpcRemover(nil) })
	Hooks.OnNpcDeath = func(e *CombatDeathEvent) {
		e.RemainsTicks = 103
		e.RespawnTicks = 50
	}
	killer, _ := combatTestPlayer(t, "killer", 3000, 3000)
	npc := combatTestNPC(t, 7234, 3001, 3000, nil)
	npc.Hitpoints = 10
	npc.GetCombat().AddDamage(&killer.Mobile, 10)
	DecrementHealth(&npc.Mobile, NewHitDamage(10, HitMaskRed))
	// Death ticks are 2 (fake def): dispatch at cycle 102.
	advance(102)
	npc.GetCombat().ProcessHitQueue(102)
	if removed != 0 {
		t.Fatalf("at dispatch: removed = %d, want 0 (remains pending)", removed)
	}
	// 102 ticks of remains countdown: still standing at 204.
	for cycle := int64(103); cycle <= 204; cycle++ {
		advance(cycle)
		npc.GetCombat().ProcessHitQueue(cycle)
	}
	if removed != 0 {
		t.Fatalf("at cycle 204: removed = %d, want 0", removed)
	}
	advance(205)
	npc.GetCombat().ProcessHitQueue(205)
	if removed != 1 || removedNPC != npc {
		t.Fatalf("at cycle 205: removed = %d (npc %v), want 1 (the Ent)", removed, removedNPC)
	}
	// Without remains, removal is immediate at dispatch.
	Hooks.OnNpcDeath = nil
	other := combatTestNPC(t, 3030, 3001, 3000, nil)
	other.Hitpoints = 10
	other.GetCombat().AddDamage(&killer.Mobile, 10)
	DecrementHealth(&other.Mobile, NewHitDamage(10, HitMaskRed))
	advance(207)
	other.GetCombat().ProcessHitQueue(207)
	if removed != 2 || removedNPC != other {
		t.Fatalf("no-remains death: removed = %d (npc %v), want 2 (the goblin)", removed, removedNPC)
	}
}
