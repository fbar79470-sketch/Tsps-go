package game

import "testing"

// TestForceMovementTaskLandsAfterThreeTicks pins the ditch's
// ForceMovementTask (ForceMovementTask.ts): the player does not move
// on ticks 1-2 and lands at start + delta on the 3rd tick, and the
// force movement clears once the task has executed.
func TestForceMovementTaskLandsAfterThreeTicks(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	fm := &ForceMovement{
		Start:        p.Location(),
		Delta:        Location{X: 0, Y: 3},
		Speed:        0,
		ReverseSpeed: 70,
		Direction:    0,
		Animation:    6132,
	}
	tasks := NewTaskManager()
	tasks.Schedule(NewForceMovementTask(p, 3, fm).Task())
	if p.ForceMovement() != fm {
		t.Fatal("after schedule: want the force movement installed")
	}
	if !p.Flags.Has(FlagForcedMovement) {
		t.Error("after schedule: want FlagForcedMovement set")
	}
	for cycle := int64(1); cycle <= 2; cycle++ {
		tasks.Process(cycle)
		if p.Location() != (Location{X: 3089, Y: 3520, Z: 0}) {
			t.Fatalf("after tick %d: player moved early to %+v", cycle, p.Location())
		}
		if p.ForceMovement() != fm {
			t.Fatalf("after tick %d: force movement cleared early", cycle)
		}
	}
	tasks.Process(3)
	if p.Location() != (Location{X: 3089, Y: 3523, Z: 0}) {
		t.Fatalf("after tick 3: location = %+v, want (3089, 3523, 0)", p.Location())
	}
	if p.ForceMovement() != nil {
		t.Error("after tick 3: want the force movement cleared")
	}
}

// TestForceMovementTaskStopClears pins ForceMovementTask.stop(): stopping
// the task before it executes clears the force movement it owns, and
// the player never moves.
func TestForceMovementTaskStopClears(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	fm := &ForceMovement{Start: p.Location(), Delta: Location{X: 0, Y: 3}, Direction: 0}
	tasks := NewTaskManager()
	task := NewForceMovementTask(p, 3, fm)
	tasks.Schedule(task.Task())
	task.Task().Stop()
	if p.ForceMovement() != nil {
		t.Error("after stop: want the force movement cleared")
	}
	// The flag is a per-tick dirty marker: TS setForceMovement(null)
	// does not clear it (Player.ts:1207-1214); ResetUpdating clears
	// it at the end of the tick.
	if !p.Flags.Has(FlagForcedMovement) {
		t.Error("after stop: want FlagForcedMovement still set until ResetUpdating")
	}
	p.ResetUpdating()
	if p.Flags.Has(FlagForcedMovement) {
		t.Error("after ResetUpdating: want FlagForcedMovement cleared")
	}
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (Location{X: 3089, Y: 3520, Z: 0}) {
		t.Fatalf("after 3 ticks: location = %+v, want the player unmoved", p.Location())
	}
}

// TestForceMovementTaskIdentityMismatch pins the execute() identity
// check (ForceMovementTask.ts: execute): a task whose force movement
// is no longer the player's active one does not move the player and
// does not clear the movement that replaced it.
func TestForceMovementTaskIdentityMismatch(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	first := &ForceMovement{Start: p.Location(), Delta: Location{X: 0, Y: 3}, Direction: 0}
	second := &ForceMovement{Start: p.Location(), Delta: Location{X: 3, Y: 0}, Direction: 1}
	tasks := NewTaskManager()
	tasks.Schedule(NewForceMovementTask(p, 3, first).Task())
	p.SetForceMovement(second)
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if p.Location() != (Location{X: 3089, Y: 3520, Z: 0}) {
		t.Fatalf("after 3 ticks: location = %+v, want the player unmoved", p.Location())
	}
	if p.ForceMovement() != second {
		t.Error("after the stale task executed: want the second force movement still active")
	}
}

// TestForceMovementTaskStopClearsOnlyOwned pins the stop() identity
// check: stopping a task whose force movement was replaced does not
// clear the replacement.
func TestForceMovementTaskStopClearsOnlyOwned(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	first := &ForceMovement{Start: p.Location(), Delta: Location{X: 0, Y: 3}, Direction: 0}
	second := &ForceMovement{Start: p.Location(), Delta: Location{X: 3, Y: 0}, Direction: 1}
	tasks := NewTaskManager()
	task := NewForceMovementTask(p, 3, first)
	tasks.Schedule(task.Task())
	p.SetForceMovement(second)
	task.Task().Stop()
	if p.ForceMovement() != second {
		t.Error("after stopping the stale task: want the second force movement still active")
	}
}

// TestClickDelayElapsed pins the click-delay TickStopwatch
// (TickStopwatch.ts, Player.ts:123): never reset passes (the
// never-started stopwatch reads Infinity), a reset on the current
// cycle blocks for 250 ms, and a later cycle passes.
func TestClickDelayElapsed(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	if !p.ClickDelayElapsed(250) {
		t.Error("never reset: want the click delay elapsed")
	}
	// Install the cycle source BEFORE the reset: the stamp must be
	// taken from this test's cycle, not whatever a prior test in
	// the package left behind in the cycle source.
	cycle := int64(0)
	SetCombatCycleSource(func() int64 { return cycle })
	defer SetCombatCycleSource(nil)
	p.ResetClickDelay()
	if p.ClickDelayElapsed(250) {
		t.Error("reset on the current cycle: want the click delay NOT elapsed")
	}
	cycle = 1
	if !p.ClickDelayElapsed(250) {
		t.Error("one cycle (600 ms) after reset: want the click delay elapsed")
	}
}

// TestSetForceMovementFlags pins SetForceMovement's flag bookkeeping:
// installing sets FlagForcedMovement, clearing clears it, and the
// flag survives a task install (the task constructor installs via
// SetForceMovement).
func TestSetForceMovementFlags(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3520, Z: 0})
	fm := &ForceMovement{Start: p.Location(), Delta: Location{X: 0, Y: 3}, Direction: 0}
	p.SetForceMovement(fm)
	if !p.Flags.Has(FlagForcedMovement) {
		t.Error("after install: want FlagForcedMovement set")
	}
	p.SetForceMovement(nil)
	// Clearing the instance does not clear the flag (Player.ts:
	// 1207-1214): the flag is a per-tick dirty marker, cleared by
	// ResetUpdating.
	if !p.Flags.Has(FlagForcedMovement) {
		t.Error("after clear: want FlagForcedMovement still set until ResetUpdating")
	}
	if p.ForceMovement() != nil {
		t.Error("after clear: want no force movement")
	}
	p.ResetUpdating()
	if p.Flags.Has(FlagForcedMovement) {
		t.Error("after ResetUpdating: want FlagForcedMovement cleared")
	}
}

// TestRequestWalkBlockedByForceMovement pins the MovementQueue.ts:350
// mobility gate: a player with an active force movement cannot walk —
// the request is refused silently (INVALID sends nothing), so the
// queue stays empty. The control leg (no force movement) queues.
func TestRequestWalkBlockedByForceMovement(t *testing.T) {
	control := NewPlayer("alice", Location{X: 100, Y: 100, Z: 0})
	control.MovementQueue().RequestWalk(Location{X: 105, Y: 103, Z: 0}, nil)
	if control.MovementQueue().Size() == 0 {
		t.Fatal("control: requestWalk on open ground queued no checkpoints")
	}

	held := NewPlayer("bob", Location{X: 100, Y: 100, Z: 0})
	held.SetForceMovement(&ForceMovement{Start: held.Location(), Delta: Location{X: 0, Y: 3}, Direction: 0})
	held.MovementQueue().RequestWalk(Location{X: 105, Y: 103, Z: 0}, nil)
	if held.MovementQueue().Size() != 0 {
		t.Errorf("with an active force movement: queue size = %d, want 0", held.MovementQueue().Size())
	}
}

// TestTaskOnStopRunsOnce pins the Task.OnStop contract: Stop fires
// the hook on the first call only, and a task that finishes by
// returning done=true never fires it.
func TestTaskOnStopRunsOnce(t *testing.T) {
	tasks := NewTaskManager()
	calls := 0
	task := NewTask(1, func(cycle int64) (done bool) { return false })
	task.OnStop = func() { calls++ }
	tasks.Schedule(task)
	task.Stop()
	task.Stop()
	if calls != 1 {
		t.Errorf("OnStop calls = %d, want 1", calls)
	}

	doneCalls := 0
	done := NewTask(1, func(cycle int64) (done bool) { return true })
	done.OnStop = func() { doneCalls++ }
	tasks.Schedule(done)
	tasks.Process(1)
	if doneCalls != 0 {
		t.Errorf("OnStop calls after done = %d, want 0", doneCalls)
	}
}
