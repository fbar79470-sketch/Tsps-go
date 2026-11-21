package game

import (
	"testing"
	"time"
)

func countExecutions(t *testing.T, delay, cycles int) int {
	t.Helper()
	m := NewTaskManager()
	count := 0
	m.Schedule(NewTask(delay, func(cycle int64) bool { count++; return false }))
	for c := 1; c <= cycles; c++ {
		m.Process(int64(c))
	}
	return count
}

func TestTaskDelaySemantics(t *testing.T) {
	// Task.ts tick(): delay 1 executes every tick.
	if got := countExecutions(t, 1, 5); got != 5 {
		t.Fatalf("delay 1 over 5 cycles = %d executions, want 5", got)
	}
	// Delay 2: countdown 2 -> 1 -> 0 (exec) -> 2 -> 1 -> 0 (exec): 2 in 5.
	if got := countExecutions(t, 2, 5); got != 2 {
		t.Fatalf("delay 2 over 5 cycles = %d executions, want 2", got)
	}
	// Delay 0: countdown == 0 executes every tick.
	if got := countExecutions(t, 0, 5); got != 5 {
		t.Fatalf("delay 0 over 5 cycles = %d executions, want 5", got)
	}
	// Delay 3 over 6 cycles: exec on cycles 3 and 6.
	if got := countExecutions(t, 3, 6); got != 2 {
		t.Fatalf("delay 3 over 6 cycles = %d executions, want 2", got)
	}
}

func TestTaskDoneStops(t *testing.T) {
	m := NewTaskManager()
	count := 0
	m.Schedule(NewTask(1, func(cycle int64) bool {
		count++
		return count >= 2 // done on second execution
	}))
	for c := 1; c <= 5; c++ {
		m.Process(int64(c))
	}
	if count != 2 {
		t.Fatalf("done task executed %d times, want 2", count)
	}
	if m.Count() != 0 {
		t.Fatal("stopped task must be dropped")
	}
}

func TestTaskImmediateExecutesAtSchedule(t *testing.T) {
	m := NewTaskManager()
	var cycles []int64
	task := NewTask(10, func(cycle int64) bool { cycles = append(cycles, cycle); return false })
	task.Immediate = true
	m.Schedule(task)
	if len(cycles) != 1 {
		t.Fatal("immediate task must execute once at Schedule")
	}
	// Then it joins the pending queue and runs on its delay.
	m.Process(1)
	if len(cycles) != 1 {
		t.Fatal("immediate task must not re-execute before its delay")
	}
}

func TestTaskRescheduleWhileRunningIgnored(t *testing.T) {
	m := NewTaskManager()
	task := NewTask(1, func(cycle int64) bool { return false })
	m.Schedule(task)
	m.Schedule(task)
	if m.Count() != 1 {
		t.Fatal("scheduling a running task must be ignored")
	}
}

func TestTaskOnTickRunsEveryTick(t *testing.T) {
	m := NewTaskManager()
	ticks, execs := 0, 0
	task := NewTask(5, func(cycle int64) bool { execs++; return false })
	task.OnTick = func(cycle int64) { ticks++ }
	m.Schedule(task)
	for c := 1; c <= 5; c++ {
		m.Process(int64(c))
	}
	if ticks != 5 {
		t.Fatalf("OnTick ran %d times, want 5", ticks)
	}
	if execs != 1 {
		t.Fatalf("Action ran %d times, want 1", execs)
	}
}

func TestTaskWalkToNotTickedByProcess(t *testing.T) {
	m := NewTaskManager()
	execs := 0
	task := NewTask(1, func(cycle int64) bool { execs++; return false })
	task.Type = TaskWalkTo
	task.Key = 7
	m.Schedule(task)
	m.Process(1)
	m.Process(2)
	if execs != 0 {
		t.Fatal("WALK_TO tasks must not be ticked by Process")
	}
	// Ticked from the owner's turn, at most once per cycle.
	m.ProcessWalkTo(7, 3)
	m.ProcessWalkTo(7, 3)
	if execs != 1 {
		t.Fatalf("ProcessWalkTo twice in one cycle executed %d times, want 1", execs)
	}
	m.Process(4) // clears the per-cycle ticked set
	m.ProcessWalkTo(7, 4)
	if execs != 2 {
		t.Fatalf("next cycle walk-to executed %d times total, want 2", execs)
	}
	// Other keys do not tick it.
	m.Process(5)
	m.ProcessWalkTo(8, 5)
	if execs != 2 {
		t.Fatal("wrong key must not tick the walk-to task")
	}
}

func TestTaskWalkToPendingActivation(t *testing.T) {
	m := NewTaskManager()
	execs := 0
	task := NewTask(1, func(cycle int64) bool { execs++; return false })
	task.Type = TaskWalkTo
	task.Key = 9
	// Scheduled after Process ran (e.g. from client input during a turn):
	// ProcessWalkTo must still activate and tick it this cycle.
	m.Process(1)
	m.Schedule(task)
	m.ProcessWalkTo(9, 1)
	if execs != 1 {
		t.Fatal("pending walk-to for the key must activate via ProcessWalkTo")
	}
}

func TestTaskProcessRemainingWalkTo(t *testing.T) {
	m := NewTaskManager()
	var ran []any
	for _, key := range []any{1, 2} {
		k := key
		task := NewTask(1, func(cycle int64) bool { ran = append(ran, k); return false })
		task.Type = TaskWalkTo
		task.Key = k
		m.Schedule(task)
	}
	m.Process(1)
	// Owner 1 took a turn; owner 2 did not.
	m.ProcessWalkTo(1, 1)
	m.ProcessRemainingWalkTo(1)
	if len(ran) != 2 || ran[0] != 1 || ran[1] != 2 {
		t.Fatalf("remaining walk-to sweep ran %v, want [1 2]", ran)
	}
	// Second sweep in the same cycle must not re-tick.
	m.ProcessRemainingWalkTo(1)
	if len(ran) != 2 {
		t.Fatal("sweep must not re-tick already-ticked walk-to tasks")
	}
}

func TestTaskCancelAndQuery(t *testing.T) {
	m := NewTaskManager()
	plain := NewTask(1, func(cycle int64) bool { return false })
	plain.Key = "owner"
	walk := NewTask(1, func(cycle int64) bool { return false })
	walk.Type = TaskWalkTo
	walk.Key = "owner"
	m.Schedule(plain)
	m.Schedule(walk)
	if !m.HasWalkToTask("owner") || !m.HasActiveTask("owner", "") {
		t.Fatal("query helpers must find scheduled tasks")
	}
	m.CancelWalkToTasks("owner")
	if m.HasWalkToTask("owner") {
		t.Fatal("CancelWalkToTasks must stop walk-to tasks")
	}
	if !m.HasActiveTask("owner", "") {
		t.Fatal("CancelWalkToTasks must not touch ordinary tasks")
	}
	m.CancelTasks("owner")
	if m.HasActiveTask("owner", "") {
		t.Fatal("CancelTasks must stop all tasks for the key")
	}
	// Dropped on next Process.
	m.Process(1)
	if m.Count() != 0 {
		t.Fatal("cancelled tasks must be dropped by Process")
	}
}

func TestTaskPanicIsolatedAndReported(t *testing.T) {
	m := NewTaskManager()
	var reported []any
	m.OnError = func(task *Task, r any) { reported = append(reported, r) }
	good := 0
	m.Schedule(NewTask(1, func(cycle int64) bool { panic("boom") }))
	m.Schedule(NewTask(1, func(cycle int64) bool { good++; return false }))
	m.Process(1)
	if len(reported) != 1 {
		t.Fatalf("OnError reports = %d, want 1", len(reported))
	}
	if good != 1 {
		t.Fatal("a panicking task must not starve later tasks")
	}
	m.Process(2)
	if good != 2 || len(reported) != 1 {
		t.Fatal("panicking task must be stopped and dropped, not retried")
	}
}

func TestTaskSlowWarnHook(t *testing.T) {
	m := NewTaskManager()
	var slow []string
	m.OnSlowTask = func(name string, ms int64) { slow = append(slow, name) }
	task := NewTask(1, func(cycle int64) bool {
		time.Sleep(60 * time.Millisecond)
		return false
	})
	task.Name = "slow-one"
	m.Schedule(task)
	m.Process(1)
	if len(slow) != 1 || slow[0] != "slow-one" {
		t.Fatalf("slow reports = %v, want [slow-one]", slow)
	}
	// Cooldown: immediate re-tick must not re-report.
	m.Process(2)
	if len(slow) != 1 {
		t.Fatal("slow-task reports must be throttled per name")
	}
}
