package game

import "testing"

// walkHook mirrors the wired server's OnMovementRequest hook (the Go
// form of walk(), NetworkBuilder.ts:1739-1752): destination z is the
// player's current level, the run choice is the WALK force-run bit
// or ClickRunOverride for gameclicks, and TargetLocID is discarded.
// The cmd/server tests prove the real installed hook; these
// game-package tests prove the MovementRequest -> position path with
// fake collision and no server involved.
func walkHook(p *Player, req MovementRequest, cycle int64) {
	_ = cycle
	destination := Location{X: req.X, Y: req.Y, Z: p.Loc.Z}
	var runOverride *bool
	if req.Run {
		run := true
		runOverride = &run
	} else {
		runOverride = ClickRunOverride(p.IsRunning(), req.ModifierFlags)
	}
	p.MovementQueue().RequestWalk(destination, runOverride)
}

// TestMovementRequestMovesPlayerOverTicks proves the full path at the
// game-package level: a MovementRequest delivered via
// OnMovementRequest moves the player over successive Process calls,
// one tile per walking tick, flagging APPEARANCE while moving.
func TestMovementRequestMovesPlayerOverTicks(t *testing.T) {
	spawn := Location{X: 3089, Y: 3524, Z: 0}
	p := NewPlayer("alice", spawn)
	p.SetRunning(false) // one step per tick, deterministic.
	p.MovementQueue().SetCollision(&fakeCollision{})
	p.OnMovementRequest = walkHook

	// A gameclick (no force-run bit, no modifiers): follows the
	// walking setting.
	p.OnMovementRequest(p, MovementRequest{X: 3092, Y: 3524}, 1)
	if p.MovementQueue().Size() == 0 {
		t.Fatal("movement request queued no checkpoints")
	}

	want := []Location{
		{X: 3090, Y: 3524, Z: 0},
		{X: 3091, Y: 3524, Z: 0},
		{X: 3092, Y: 3524, Z: 0},
	}
	for i, w := range want {
		p.Process(int64(i + 2))
		if p.Loc != w {
			t.Fatalf("tick %d: Loc = %+v, want %+v", i+2, p.Loc, w)
		}
		if !p.Flags.Has(FlagAppearance) {
			t.Fatalf("tick %d: moving player must flag APPEARANCE", i+2)
		}
		// Simulate the World's reset phase between ticks.
		p.ResetUpdating()
	}
	if p.MovementQueue().Size() != 0 {
		t.Fatal("queue must drain on arrival")
	}
	// An idle tick flags nothing.
	p.Process(99)
	if p.Flags.Has(FlagAppearance) {
		t.Fatal("idle tick must not flag APPEARANCE")
	}
}

// TestMovementRequestForceRunStepsTwice proves the WALK force-run bit
// path: runOverride=true steps twice per tick even with the walking
// setting on.
func TestMovementRequestForceRunStepsTwice(t *testing.T) {
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	p.SetRunning(false)
	p.MovementQueue().SetCollision(&fakeCollision{})
	p.OnMovementRequest = walkHook
	p.OnMovementRequest(p, MovementRequest{X: 104, Y: 100, Run: true}, 1)
	p.Process(2)
	if p.Loc != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("force-run tick moved to %+v, want two steps", p.Loc)
	}
}

// TestMovementRequestCtrlClickInverts proves a Ctrl gameclick inverts
// the run setting for the walk only: walking + Ctrl runs, and the
// setting itself is untouched.
func TestMovementRequestCtrlClickInverts(t *testing.T) {
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	p.SetRunning(false)
	p.MovementQueue().SetCollision(&fakeCollision{})
	p.OnMovementRequest = walkHook
	// ModifierFlags 1 = Ctrl.
	p.OnMovementRequest(p, MovementRequest{X: 104, Y: 100, ModifierFlags: 1}, 1)
	p.Process(2)
	if p.Loc != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("Ctrl-click tick moved to %+v, want two steps (inverted to run)", p.Loc)
	}
	if p.IsRunning() {
		t.Fatal("a Ctrl-click must not change the run setting")
	}
}

// TestMovementRequestDestinationKeepsPlayerLevel proves the walk()
// destination z: the request carries no level, so the player's
// current level is used (NetworkBuilder.ts:1748-1750).
func TestMovementRequestDestinationKeepsPlayerLevel(t *testing.T) {
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 1})
	p.MovementQueue().SetCollision(&fakeCollision{})
	p.OnMovementRequest = walkHook
	p.OnMovementRequest(p, MovementRequest{X: 102, Y: 100}, 1)
	dest, ok := p.MovementQueue().GetDestination()
	if !ok || dest.Z != 1 {
		t.Fatalf("destination = (%+v, %v), want z=1", dest, ok)
	}
}

// TestMovementBlockedByFakeWall proves collision stops the player:
// a wall tile east of the spawn blocks the step, the checkpoint is
// retained, and the player never enters the blocked tile.
func TestMovementBlockedByFakeWall(t *testing.T) {
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	p.SetRunning(false)
	wall := Location{X: 102, Y: 100, Z: 0}
	p.MovementQueue().SetCollision(&fakeCollision{blocked: map[Location]bool{wall: true}})
	p.OnMovementRequest = walkHook
	p.OnMovementRequest(p, MovementRequest{X: 105, Y: 100}, 1)
	for i := int64(2); i <= 6; i++ {
		p.Process(i)
		p.ResetUpdating()
	}
	if p.Loc != (Location{X: 101, Y: 100, Z: 0}) {
		t.Fatalf("player stopped at %+v, want (101, 100, 0) before the wall", p.Loc)
	}
	if !p.MovementQueue().IsStepBlocked() {
		t.Fatal("queue must report the step blocked")
	}
}

// TestMovementRoutesAroundClippedWall proves routing + stepping
// against real clipping: a wall segment blocks the direct line, the
// route finder goes around it, and the player arrives without ever
// standing on a wall tile.
func TestMovementRoutesAroundClippedWall(t *testing.T) {
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	p.SetRunning(false)
	// Wall: x=105, y=98..102. Both the clipping lookup (routing)
	// and the collision (stepping) honor it.
	wall := map[Location]bool{}
	for y := 98; y <= 102; y++ {
		wall[Location{X: 105, Y: y, Z: 0}] = true
	}
	p.MovementQueue().SetCollision(&fakeCollision{blocked: wall})
	p.MovementQueue().SetClipping(func(x, y, level int) int32 {
		if wall[Location{X: x, Y: y, Z: level}] {
			return blockWalk
		}
		return 0
	})
	p.OnMovementRequest = walkHook
	p.OnMovementRequest(p, MovementRequest{X: 110, Y: 100}, 1)
	for i := int64(2); i <= 40; i++ {
		p.Process(i)
		if wall[p.Loc] {
			t.Fatalf("player stepped onto wall tile %+v", p.Loc)
		}
		if p.Loc == (Location{X: 110, Y: 100, Z: 0}) {
			break
		}
		p.ResetUpdating()
	}
	if p.Loc != (Location{X: 110, Y: 100, Z: 0}) {
		t.Fatalf("player ended at %+v, want (110, 100, 0)", p.Loc)
	}
}

// TestWorldTicksMovePlayer proves the queue runs inside World.Process:
// the player is registered through the world queues, a turn packet
// drains through a decoding TurnHandler into OnMovementRequest, and
// successive world ticks step the player to the destination.
func TestWorldTicksMovePlayer(t *testing.T) {
	spawn := Location{X: 3089, Y: 3524, Z: 0}
	w := NewWorld(spawn)
	p := NewPlayer("alice", spawn)
	p.SetRunning(false)
	p.TurnHandler = func(pl *Player, pkt TurnPacket, cycle int64) {
		// Test stand-in for the cmd/server decoder: any packet is
		// a walk to (3091, 3524).
		_ = pkt
		if pl.OnMovementRequest != nil {
			pl.OnMovementRequest(pl, MovementRequest{X: 3091, Y: 3524}, cycle)
		}
	}
	p.OnMovementRequest = walkHook
	w.QueueAddPlayer(p)
	w.Process() // registers the player (cycle 1)
	if !p.IsRegistered() {
		t.Fatal("player must be registered after the first world tick")
	}
	if !p.QueueTurnPacket(TurnPacket{Opcode: 210, Payload: []byte{}}) {
		t.Fatal("turn packet must enqueue")
	}
	w.Process() // drains the packet, routes, steps once (cycle 2)
	if p.Loc != (Location{X: 3090, Y: 3524, Z: 0}) {
		t.Fatalf("after drain tick: %+v, want one step east", p.Loc)
	}
	w.Process() // steps again (cycle 3)
	if p.Loc != (Location{X: 3091, Y: 3524, Z: 0}) {
		t.Fatalf("after second tick: %+v, want (3091, 3524, 0)", p.Loc)
	}
}
