package game

import "testing"

// TestMeleeApproachReachesOrthogonalAdjacency drives the
// full pre -> movement -> post cycle for a melee attacker
// starting diagonally offset from its target. TS
// calculateEntityRoute is shape-aware; the Go route goes
// through WalkRoute (moveNear). This test pins the observed
// behavior: the attacker must end on a tile from which
// CanReach is true and land a hit, not loop re-routing from
// a diagonal stop.
func TestMeleeApproachReachesOrthogonalAdjacency(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3002, 3001, nil)
	npc.Hitpoints = 99
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker.GetCombat().Attack(&npc.Mobile)
	reached := false
	for cycle := int64(100); cycle < 112; cycle++ {
		advance(cycle)
		attacker.GetCombat().PreMovementProcess()
		attacker.MovementQueue().BeginCycle()
		attacker.MovementQueue().Process(cycle)
		attacker.GetCombat().PostMovementProcess()
		npc.GetCombat().ProcessHitQueue(cycle)
		if CanReach(combatantFor(&attacker.Mobile), MeleeMethod, combatantFor(&npc.Mobile)) {
			reached = true
		}
		if npc.Hitpoints < 99 {
			break
		}
	}
	if !reached {
		t.Errorf("attacker never reached a melee-reachable tile; final loc %v, npc %v",
			attacker.Location(), npc.Location())
	}
	if npc.Hitpoints >= 99 {
		t.Errorf("no damage landed after the approach; final loc %v", attacker.Location())
	}
}
