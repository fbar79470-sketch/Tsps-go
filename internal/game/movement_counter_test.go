package game

import "testing"

// Counter tests: canInteractWithUnreachableNpc
// (MovementQueue.ts:1923-1928) — the bank-counter case. A
// banker at (3090,3243) has the booth row blocking every
// adjacent tile, so a player at (3092,3243) can never reach
// range 1, but stands within NPC_INTERACT_RADIUS (2) with a
// clear projectile ray: TS interacts from the route end
// instead of refusing with "I can't reach that!".

// counterSetup builds a player east of a banker with the
// counter between them: the 8 tiles around the banker answer
// blockWalk to the route finder, so no adjacent tile is
// reachable. The projectile ray uses the combat clipping
// source, which stays unset (open ground) unless a test
// installs one.
func counterSetup(t *testing.T, playerLoc, npcLoc Location) (*Player, *NPC, *fakeSession) {
	t.Helper()
	p := NewPlayer("counter", playerLoc)
	p.SetIndex(1)
	fs := newFakeSession()
	p.Session = fs
	p.MovementQueue().SetClipping(func(x, y, level int) int32 {
		if level != npcLoc.Z {
			return 0
		}
		dx := x - npcLoc.X
		if dx < 0 {
			dx = -dx
		}
		dy := y - npcLoc.Y
		if dy < 0 {
			dy = -dy
		}
		if dx <= 1 && dy <= 1 && (dx != 0 || dy != 0) {
			return blockWalk
		}
		return 0
	})
	npc := NewNPC(1618, npcLoc)
	npc.SetRegistered(true)
	return p, npc, fs
}

func TestCounterBankerInteractsAtOnce(t *testing.T) {
	// The player already stands on the nearest tile: the
	// route is dry, so WalkToEntity fires the unreachable-NPC
	// branch at once (MovementQueue.ts:1553).
	p, npc, fs := counterSetup(t, Location{X: 3092, Y: 3243, Z: 0}, Location{X: 3090, Y: 3243, Z: 0})
	ran := false
	p.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !ran {
		t.Fatalf("the action never ran; player at %+v, pending=%v", p.Location(), p.MovementQueue().WalkToPending())
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("no walk-to must pend after an immediate counter interact")
	}
	if len(fs.messages) != 0 {
		t.Errorf("messages = %v, want none", fs.messages)
	}
}

func TestCounterBankerInteractsOnArrivalTick(t *testing.T) {
	// The player starts 5 tiles east: WalkToEntity routes to
	// the nearest tile (3092,3243), walks there, and the
	// action runs on the arrival tick via the closure branch
	// (MovementQueue.ts:1579) instead of the refusal.
	p, npc, fs := counterSetup(t, Location{X: 3095, Y: 3243, Z: 0}, Location{X: 3090, Y: 3243, Z: 0})
	ran := false
	q := p.MovementQueue()
	q.WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !q.WalkToPending() {
		t.Fatal("a walk-to must pend from 5 tiles out")
	}
	for cycle := int64(1); cycle <= 15 && !ran; cycle++ {
		q.Process(cycle)
	}
	if !ran {
		t.Fatalf("the action never ran; player at %+v, pending=%v", p.Location(), q.WalkToPending())
	}
	if p.Location() != (Location{X: 3092, Y: 3243, Z: 0}) {
		t.Errorf("player at %+v, want the nearest tile (3092,3243,0)", p.Location())
	}
	if len(fs.messages) != 0 {
		t.Errorf("messages = %v, want none", fs.messages)
	}
}

func TestCounterBankerBlockedRayRefuses(t *testing.T) {
	// The booth tile blocks the westward projectile ray, so
	// the branch cannot fire: the click fails with the
	// refusal, exactly as before the port.
	p, npc, fs := counterSetup(t, Location{X: 3092, Y: 3243, Z: 0}, Location{X: 3090, Y: 3243, Z: 0})
	SetCombatClippingSource(func(x, y, z int) int32 {
		if x == 3091 && y == 3243 && z == 0 {
			return ProjectileEastBlocked
		}
		return 0
	})
	t.Cleanup(func() { SetCombatClippingSource(nil) })
	ran := false
	p.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if ran {
		t.Error("the action must not run across a blocked projectile ray")
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("a walk-to must pend while the ray blocks the immediate branch")
	}
	q := p.MovementQueue()
	for cycle := int64(1); cycle <= 5 && p.MovementQueue().WalkToPending(); cycle++ {
		q.Process(cycle)
	}
	if ran {
		t.Error("the action must not run across a blocked projectile ray")
	}
	if len(fs.messages) != 1 || fs.messages[0] != "I can't reach that!" {
		t.Errorf("messages = %v, want [I can't reach that!]", fs.messages)
	}
}

func TestCanInteractWithUnreachableNpcGuards(t *testing.T) {
	// Direct predicate checks (MovementQueue.ts:1923-1928).
	npcLoc := Location{X: 3090, Y: 3243, Z: 0}
	npc := NewNPC(1618, npcLoc)
	npc.SetRegistered(true)
	target := NPCTarget(npc)

	// Player target: not an NPC.
	player := NewPlayer("other", Location{X: 3090, Y: 3243, Z: 0})
	q := NewMovementQueue(newFakeOwner(Location{X: 3092, Y: 3243, Z: 0}), nil, nil)
	if q.canInteractWithUnreachableNpc(PlayerTarget(player)) {
		t.Error("a player target must never take the unreachable-NPC branch")
	}
	// Nil mobile.
	if q.canInteractWithUnreachableNpc(EntityTarget{}) {
		t.Error("a nil target must never take the unreachable-NPC branch")
	}
	// Distance 3: outside NPC_INTERACT_RADIUS.
	far := NewMovementQueue(newFakeOwner(Location{X: 3093, Y: 3243, Z: 0}), nil, nil)
	if far.canInteractWithUnreachableNpc(target) {
		t.Error("distance 3 must be outside the interact radius")
	}
	// Different plane: Location.Distance is -1.
	above := NewMovementQueue(newFakeOwner(Location{X: 3092, Y: 3243, Z: 1}), nil, nil)
	if above.canInteractWithUnreachableNpc(target) {
		t.Error("a different plane must never take the branch")
	}
	// Route still has points: not yet at the route end.
	busy := NewMovementQueue(newFakeOwner(Location{X: 3092, Y: 3243, Z: 0}), nil, nil)
	busy.AddCheckpoint(Location{X: 3092, Y: 3243, Z: 0})
	if busy.canInteractWithUnreachableNpc(target) {
		t.Error("a non-dry route must never take the branch")
	}
	// Within radius with a dry route and a clear ray: true.
	if !q.canInteractWithUnreachableNpc(target) {
		t.Error("distance 2 with a dry route and a clear ray must take the branch")
	}
}
