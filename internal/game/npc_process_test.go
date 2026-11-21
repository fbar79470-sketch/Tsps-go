package game

import "testing"

func TestNPCProcessPhaseOrder(t *testing.T) {
	// The phases are observable through their effects: a timer
	// ticks down, the hook fires once with the cycle, the
	// coordinator's wander reaches the movement phase in the
	// same tick, and the area phase assigns the NPC's area.
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	n.GetTimers().Registers(TimerKeyFood, 1)
	var hookCycles []int64
	n.ProcessHook = func(_ *NPC, cycle int64) { hookCycles = append(hookCycles, cycle) }
	n.MovementCoordinator().SetRadius(5)
	n.MovementCoordinator().random = cycleRandom(0.0) // wander, dest (95,95)
	manager := NewAreaManager()
	area := NewArea("home", []Boundary{NewBoundary(90, 110, 90, 110, 0)})
	manager.Register(area)
	n.areaManager = manager

	n.Process(7)

	if got := n.GetTimers().Left(TimerKeyFood); got != 0 {
		t.Errorf("timer left = %d, want 0 (ticked in the timer phase)", got)
	}
	if len(hookCycles) != 1 || hookCycles[0] != 7 {
		t.Errorf("hook cycles = %v, want [7]", hookCycles)
	}
	if n.Location().Equals(spawn) {
		t.Error("NPC did not step toward its wander destination in the same tick")
	}
	if n.Area() != area {
		t.Error("area phase did not assign the NPC's area")
	}
}

func TestNPCProcessBeginCycleResetsMovedFlag(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.MovementQueue().AddCheckpoint(Location{X: 11, Y: 10, Z: 0})
	n.Process(1)
	if !n.MovementQueue().DidMoveThisCycle() {
		t.Error("DidMoveThisCycle must be true after a stepped tick")
	}
	n.Process(2) // no points left: BeginCycle must clear the flag
	if n.MovementQueue().DidMoveThisCycle() {
		t.Error("DidMoveThisCycle must be false after a tick with no movement")
	}
}

func TestNPCProcessZeroValueDoesNotPanic(t *testing.T) {
	var n NPC
	n.Process(1) // no queue, no coordinator, no combat: must be safe
}

func TestInteractionClearingAfterWalkingAway(t *testing.T) {
	// The NPC walks east away from its target; once the gap
	// exceeds the interact radius (2), the clearing phase drops
	// the interaction — and a radius-0 NPC turns back to its
	// spawn-facing idle direction.
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	n.SetFace(DirNorth)
	p := NewPlayer("alice", spawn)
	p.SetRegistered(true)
	n.SetMobileInteraction(p)
	n.MovementQueue().AddCheckpoint(Location{X: 104, Y: 100, Z: 0})
	n.Process(1) // gap 1: still in range
	if n.GetInteractingMobile() == nil {
		t.Fatal("interaction cleared while the target was in range")
	}
	n.Process(2) // gap 2: still in range
	if n.GetInteractingMobile() == nil {
		t.Fatal("interaction cleared at the radius edge")
	}
	n.Process(3) // gap 3: out of range, cleared
	if n.GetInteractingMobile() != nil {
		t.Fatal("interaction not cleared after walking out of range")
	}
	wantFace := Location{X: 103, Y: 101, Z: 0} // one tile north of (103,100)
	if got, ok := n.PositionToFace(); !ok || !got.Equals(wantFace) {
		t.Errorf("PositionToFace = %+v, %v, want %+v (spawn-facing reset)", got, ok, wantFace)
	}
	if n.Facing != DirNorth {
		t.Errorf("Facing = %v, want NORTH (spawn face)", n.Facing)
	}
}

func TestInteractionClearingWanderingNPCClearsFace(t *testing.T) {
	n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
	n.MovementCoordinator().SetRadius(5)
	p := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	p.SetRegistered(true)
	n.SetMobileInteraction(p)
	face := Location{X: 101, Y: 100, Z: 0}
	n.SetPositionToFace(&face)
	p.SetLocation(Location{X: 110, Y: 100, Z: 0}) // out of range
	n.MovementCoordinator().random = cycleRandom(0.9)
	n.Process(1)
	if n.GetInteractingMobile() != nil {
		t.Error("interaction not cleared for the out-of-range target")
	}
	if _, ok := n.PositionToFace(); ok {
		t.Error("wandering NPC must clear its face target, not reset to spawn-facing")
	}
}

func TestInteractionClearingTrackingGuard(t *testing.T) {
	t.Run("following holds an out-of-range interaction", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		p := NewPlayer("alice", Location{X: 110, Y: 100, Z: 0})
		p.SetRegistered(true)
		n.SetFollowing(p)
		n.SetMobileInteraction(p)
		n.MovementCoordinator().random = cycleRandom(0.9)
		n.Process(1)
		if n.GetInteractingMobile() == nil {
			t.Error("following must hold the interaction past the radius")
		}
	})
	t.Run("keepInteraction holds an out-of-range interaction", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		p := NewPlayer("alice", Location{X: 110, Y: 100, Z: 0})
		p.SetRegistered(true)
		n.SetKeepInteraction(true)
		n.SetMobileInteraction(p)
		n.MovementCoordinator().random = cycleRandom(0.9)
		n.Process(1)
		if n.GetInteractingMobile() == nil {
			t.Error("interaction:keep must hold the interaction past the radius")
		}
	})
	t.Run("unregistered target clears despite keepInteraction", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		p := NewPlayer("alice", Location{X: 101, Y: 100, Z: 0})
		// Deliberately unregistered.
		n.SetKeepInteraction(true)
		n.SetMobileInteraction(p)
		n.MovementCoordinator().random = cycleRandom(0.9)
		n.Process(1)
		if n.GetInteractingMobile() != nil {
			t.Error("unregistered target must clear even with interaction:keep")
		}
	})
	t.Run("cross-plane target clears", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		p := NewPlayer("alice", Location{X: 101, Y: 100, Z: 1})
		p.SetRegistered(true)
		n.SetMobileInteraction(p)
		n.MovementCoordinator().random = cycleRandom(0.9)
		n.Process(1)
		if n.GetInteractingMobile() != nil {
			t.Error("cross-plane target must read as out of range and clear")
		}
	})
	t.Run("no interaction is a no-op", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		n.SetFace(DirNorth)
		n.Process(1)
		if _, ok := n.PositionToFace(); ok {
			t.Error("no interaction must not set a face target")
		}
	})
}

func TestScriptedMovementGates(t *testing.T) {
	t.Run("no wandering", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		n.MovementCoordinator().SetRadius(5)
		n.MovementCoordinator().random = cycleRandom(0.0)
		n.SetScriptedMovement(true)
		n.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("scripted NPC queued %d wander points", got)
		}
	})
	t.Run("no walking home", func(t *testing.T) {
		spawn := Location{X: 100, Y: 100, Z: 0}
		n := NewNPC(1, spawn)
		n.SetLocation(Location{X: 120, Y: 100, Z: 0})
		n.MovementCoordinator().SetRadius(5)
		n.MovementCoordinator().SetCoordinateState(CoordinateStateAway)
		n.SetScriptedMovement(true)
		n.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("scripted NPC queued %d walk-home points", got)
		}
		if got := n.MovementCoordinator().GetCoordinateState(); got != CoordinateStateAway {
			t.Errorf("coordinator state = %v, want AWAY (never re-evaluated)", got)
		}
	})
	t.Run("queued steps still process", func(t *testing.T) {
		// scriptedMovement owns the queue: a plugin's walk
		// steps normally; only the coordinator is gated.
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		n.SetScriptedMovement(true)
		n.MovementQueue().AddCheckpoint(Location{X: 103, Y: 100, Z: 0})
		n.Process(1)
		if got := n.Location(); got.X != 101 {
			t.Errorf("scripted NPC at %+v, want one step east", got)
		}
	})
	t.Run("releasing scripted movement resumes wandering", func(t *testing.T) {
		n := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
		n.MovementCoordinator().SetRadius(5)
		n.MovementCoordinator().random = cycleRandom(0.0)
		n.SetScriptedMovement(true)
		n.Process(1)
		n.SetScriptedMovement(false)
		n.Process(2)
		if n.Location().Equals(Location{X: 100, Y: 100, Z: 0}) {
			t.Error("NPC did not wander after scripted movement released")
		}
	})
}
