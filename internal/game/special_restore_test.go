package game

import "testing"

// TestEnsureSpecialRestoreStartsTask pins the ensure contract
// (CombatSpecial.ensureRestoreTask): below 100 with no task
// running, a task starts; at 100, or while one runs, nothing
// starts; a nil Mobile is a no-op.
func TestEnsureSpecialRestoreStartsTask(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.GetCombat().SetSpecialPercentage(100)
	if EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure at 100 must not start a task")
	}
	if EnsureSpecialRestore(nil) {
		t.Fatal("ensure with nil Mobile must not start a task")
	}
	p.GetCombat().SetSpecialPercentage(50)
	if !EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure below 100 must start a task")
	}
	if !IsRecoveringSpecialAttack(&p.Mobile) {
		t.Fatal("fighter must read as recovering after ensure")
	}
	if EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("second ensure while restoring must not start another task")
	}
	if taskCount(tasks) != 1 {
		t.Fatalf("task count = %d, want 1", taskCount(tasks))
	}
}

// taskCount is the white-box task total (pending + active).
func taskCount(m *TaskManager) int { return len(m.pending) + len(m.active) }

// TestSpecialRestoreTaskRestoresTenPerCycle pins the restore
// task (RestoreSpecialAttackTask): +10 every 50 ticks until
// 100, then the task stops and the recovering state clears.
// The fighter must read as registered or the task stops
// immediately (the isRegistered execute guard).
func TestSpecialRestoreTaskRestoresTenPerCycle(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.GetCombat().SetSpecialPercentage(80)
	p.SetRegistered(true)
	if !EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure must start the task")
	}
	// 49 ticks: nothing yet (first execution after the full
	// delay — login restarts the timer). Process ticks once per
	// call, so drive it one call per tick.
	for cycle := int64(1); cycle < SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if got := p.GetCombat().SpecialPercentage(); got != 80 {
		t.Fatalf("spec after 49 ticks = %d, want 80", got)
	}
	tasks.Process(SpecialRestoreDelayTicks)
	if got := p.GetCombat().SpecialPercentage(); got != 90 {
		t.Fatalf("spec after 50 ticks = %d, want 90", got)
	}
	if !IsRecoveringSpecialAttack(&p.Mobile) {
		t.Fatal("still recovering below 100")
	}
	for cycle := int64(51); cycle <= 2*SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if got := p.GetCombat().SpecialPercentage(); got != 100 {
		t.Fatalf("spec after 100 ticks = %d, want 100", got)
	}
	if IsRecoveringSpecialAttack(&p.Mobile) {
		t.Fatal("recovering state must clear at 100")
	}
	if taskCount(tasks) != 0 {
		t.Fatalf("task count = %d, want 0 (task stopped at 100)", taskCount(tasks))
	}
}

// TestSpecialRestoreTaskStopsWhenUnregistered pins the
// isRegistered execute guard: a fighter that leaves the world
// stops restoring and clears the recovering state.
func TestSpecialRestoreTaskStopsWhenUnregistered(t *testing.T) {
	tasks := &TaskManager{}
	SetSpecialRestoreTasks(tasks)
	t.Cleanup(func() { SetSpecialRestoreTasks(nil) })

	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.GetCombat().SetSpecialPercentage(50)
	// Never marked registered: the first execution stops.
	if !EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure must start the task")
	}
	for cycle := int64(1); cycle <= SpecialRestoreDelayTicks; cycle++ {
		tasks.Process(cycle)
	}
	if got := p.GetCombat().SpecialPercentage(); got != 50 {
		t.Fatalf("spec = %d, want 50 (unregistered: no restore)", got)
	}
	if IsRecoveringSpecialAttack(&p.Mobile) {
		t.Fatal("recovering state must clear when the task stops")
	}
}

// TestEnsureSpecialRestoreWithoutTasks pins the no-scheduler
// degeneration: with no TaskManager installed, ensure records
// nothing and returns false, so a later install still starts
// the restore (the Player.Process call site retries).
func TestEnsureSpecialRestoreWithoutTasks(t *testing.T) {
	SetSpecialRestoreTasks(nil)
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.GetCombat().SetSpecialPercentage(50)
	if EnsureSpecialRestore(&p.Mobile) {
		t.Fatal("ensure with no TaskManager must return false")
	}
	if IsRecoveringSpecialAttack(&p.Mobile) {
		t.Fatal("no recovering state without a TaskManager")
	}
}
