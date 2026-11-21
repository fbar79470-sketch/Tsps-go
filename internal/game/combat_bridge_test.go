package game

import "testing"

// TestAggressionCombatBridgeDelegates pins the coordinator
// bridge between Worker 1's AggressionCombat interface and
// Worker 2's Combat: InCombat reads the NPC's combat state,
// Attack retargets through the typed combat binding, and
// CanAttack consults the combat permission checks.
func TestAggressionCombatBridgeDelegates(t *testing.T) {
	InstallAggressionCombatBridge()
	t.Cleanup(func() { SetAggressionCombatBridge(nil) })

	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	p := NewPlayer("alice", Location{X: 11, Y: 10, Z: 0})
	n.SetRegistered(true)
	p.SetRegistered(true)
	n.Hitpoints = 10
	p.Skills().SetCurrentLevel(SkillHitpoints, 10, true)
	// Bind the typed identities the way drainAddNPCs does:
	// without the typed GetCombat calls, ValidTarget fails
	// closed and CanAttack never succeeds.
	n.GetCombat()
	p.GetCombat()

	bridge := aggressionCombat()
	if _, ok := bridge.(combatAggressionBridge); !ok {
		t.Fatalf("installed bridge = %T, want combatAggressionBridge", bridge)
	}
	if bridge.InCombat(&n.Mobile) {
		t.Fatal("idle NPC must not read as in combat")
	}
	if !bridge.CanAttack(n, p) {
		t.Fatal("fresh adjacent pair must pass CanAttack")
	}
	bridge.Attack(n, p)
	if !bridge.InCombat(&n.Mobile) {
		t.Fatal("NPC must read as in combat after Attack")
	}
	if got := n.GetCombat().Target(); got != &p.Mobile {
		t.Fatalf("combat target = %v, want the player", got)
	}
}

// TestAggressionCombatBridgeNilSafety pins the bridge's
// nil-tolerance: nil mobiles answer false / no-op instead of
// panicking (the aggression scan calls the bridge for NPCs
// whose combat state may be unbound).
func TestAggressionCombatBridgeNilSafety(t *testing.T) {
	InstallAggressionCombatBridge()
	t.Cleanup(func() { SetAggressionCombatBridge(nil) })
	bridge := aggressionCombat()
	if bridge.InCombat(nil) {
		t.Fatal("nil NPC must not read as in combat")
	}
	if bridge.CanAttack(nil, nil) {
		t.Fatal("nil pair must fail CanAttack")
	}
	bridge.Attack(nil, nil) // must not panic
}
