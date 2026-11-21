package game

import "time"

// TaskType distinguishes ordinary tick tasks from walk-to interaction
// tasks. Quoted from game/task/TaskType.ts: enum TaskType { DEFAULT,
// WALK_TO }.
type TaskType int

const (
	// TaskDefault is an ordinary recurring/delayed task.
	TaskDefault TaskType = iota
	// TaskWalkTo is a walk-to interaction task: it owns an interaction's
	// reach check and is ticked from its owner's turn (after movement),
	// not from TaskManager.Process. See TaskManager.ProcessWalkTo.
	TaskWalkTo
)

// Task is one tick-based task, porting game/task/Task.ts.
//
// Delay/repeat semantics (Task.ts constructor + tick()):
//   - Delay is clamped to >= 0; the TS default when no delay is given is 1.
//   - The task executes when its countdown reaches 0, then the countdown
//     resets to Delay. Delay 1 executes every tick, delay 2 every other
//     tick, delay 0 every tick (countdown == 0 executes immediately).
//   - There is NO Repeat field in TS: a task repeats until stopped. The
//     Go Action reports completion by returning done=true, which stops
//     the task; returning false keeps it running. Stopping from outside
//     uses Stop() or TaskManager.CancelTasks.
//   - Immediate tasks execute once at Schedule time, before joining the
//     pending queue (TaskManager.ts submit()).
//   - Type TaskWalkTo tasks are never ticked by Process; see
//     TaskManager.ProcessWalkTo.
//   - Key identifies the task's owner for walk-to processing and
//     cancellation. In TS the key is compared by identity (===); in Go
//     keys are compared with ==, so keys MUST be comparable values
//     (pointers, ints, strings). TS uses the player index (a number) for
//     walk-to tasks and the entity itself for cancelTasks.
//
// The plugin-name context (Task.ts pluginName / PluginContext) is NOT
// STARTED: there is no plugin system in Go yet.
type Task struct {
	// Name identifies the task in slow-task reports. It defaults to ""
	// when unset; set it for any task whose slowness should be
	// attributable.
	Name string

	// Delay in ticks between executions. See semantics above.
	Delay int

	// Immediate executes Action once at Schedule time.
	Immediate bool

	// Type is TaskDefault or TaskWalkTo.
	Type TaskType

	// Key is the owner key for walk-to processing and cancellation.
	Key any

	// Action runs when the countdown expires. Returning done=true stops
	// the task after this execution. Action must be non-nil; a nil
	// Action is treated as an immediately-done no-op.
	Action func(cycle int64) (done bool)

	// OnTick runs on every tick while the task is active, whether or not
	// Action executed (Task.ts onTick() hook, empty by default in TS).
	OnTick func(cycle int64)

	countdown   int
	running     bool
	initialized bool
}

// NewTask returns a task with the given delay and action. Delay is clamped
// to >= 0, matching the Task.ts constructor (Math.max(0, delay)).
func NewTask(delay int, action func(cycle int64) (done bool)) *Task {
	if delay < 0 {
		delay = 0
	}
	return &Task{Delay: delay, Action: action, countdown: delay, initialized: true}
}

// IsRunning reports whether the task is scheduled and not stopped.
// Task.ts: isRunning().
func (t *Task) IsRunning() bool { return t.running }

// IsImmediate reports the immediate flag. Task.ts: isImmediate().
func (t *Task) IsImmediate() bool { return t.Immediate }

// RemainingTicks returns the countdown to the next execution.
// Task.ts: getRemainingTicks().
func (t *Task) RemainingTicks() int { return t.countdown }

// SetDelay changes the delay, but only for positive values, matching
// Task.ts setDelay (if (delay > 0) this.delay = delay). The current
// countdown is left untouched.
func (t *Task) SetDelay(delay int) {
	if delay > 0 {
		t.Delay = delay
	}
}

// Stop marks the task stopped; the manager drops it on the next pass.
// Task.ts: stop().
func (t *Task) Stop() { t.running = false }

// Tick advances the task one tick and reports whether it is still running
// afterwards, porting Task.ts tick():
//
//	if running && (countdown == 0 || --countdown == 0) { execute(); countdown = delay }
//	onTick(); return running
func (t *Task) Tick(cycle int64) bool {
	if t.running {
		if t.countdown == 0 {
			t.execute(cycle)
		} else {
			t.countdown--
			if t.countdown == 0 {
				t.execute(cycle)
			}
		}
	}
	if t.OnTick != nil {
		t.OnTick(cycle)
	}
	return t.running
}

func (t *Task) execute(cycle int64) {
	done := true
	if t.Action != nil {
		done = t.Action(cycle)
	}
	t.countdown = t.Delay
	if done {
		t.running = false
	}
}

// TaskManager ports game/task/TaskManager.ts (a static class in TS; a
// struct here so tests and the coordinator can own an instance).
//
// Scheduling model (TaskManager.ts process/submit):
//   - Schedule marks the task running, executes it immediately if
//     Immediate, and appends it to the pending queue.
//   - Process drains pending tasks into the active list (running ones
//     only), then ticks each active non-WALK_TO task once, dropping tasks
//     that stopped. WALK_TO tasks are kept but not ticked here.
//   - ProcessWalkTo ticks the WALK_TO tasks of one owner key; each task
//     is ticked at most once per cycle (walkToTickedThisCycle).
//   - ProcessRemainingWalkTo sweeps WALK_TO tasks whose owner took no
//     turn this cycle.
//
// The manager is NOT safe for concurrent use; Schedule/Process must run
// on the tick goroutine (in TS everything runs on one event loop). If a
// later milestone needs cross-goroutine scheduling, it must add explicit
// synchronization here — do not call Schedule from network goroutines.
//
// Divergence from TS, deliberate: TaskManager.ts wraps the whole Process
// loop in one try/catch, so a throwing task aborts the remaining tasks
// that cycle. The Go manager recovers per task (stopping the throwing
// task and reporting it via OnError) so one bad task cannot starve the
// others; this matches the per-task recovery TS already uses in
// processWalkTo. World phases remain failure-safe either way.
type TaskManager struct {
	pending            []*Task
	active             []*Task
	processedThisCycle []*Task
	walkToTicked       map[*Task]bool
	lastSlowLog        map[string]time.Time
	lastCycle          int64

	// OnSlowTask reports a task tick lasting at least SlowTaskWarnMs,
	// throttled to one report per task name per SlowTaskLogCooldownMs.
	// It replaces the FreezeDiagnostics/console warning in
	// TaskManager.ts logSlowTaskTick. Nil disables reporting.
	// (TS gates it on env TASK_SLOW_WARN_ENABLED; the coordinator owns
	// that switch by setting or clearing this hook.)
	OnSlowTask func(name string, durationMs int64)

	// OnError reports a panic recovered from a task tick. The throwing
	// task is stopped and dropped. Nil swallows the report (the task is
	// still stopped).
	OnError func(task *Task, recovered any)
}

// NewTaskManager returns an empty TaskManager.
func NewTaskManager() *TaskManager {
	return &TaskManager{
		walkToTicked: make(map[*Task]bool),
		lastSlowLog:  make(map[string]time.Time),
	}
}

// Schedule submits a task, porting TaskManager.ts submit(): a task that is
// already running is ignored; otherwise it is marked running, executed
// immediately if Immediate (with the manager's last processed cycle),
// and appended to the pending queue.
func (m *TaskManager) Schedule(task *Task) {
	if task == nil || task.running {
		return
	}
	if !task.initialized {
		if task.Delay < 0 {
			task.Delay = 0
		}
		task.countdown = task.Delay
		task.initialized = true
	}
	task.running = true
	if task.Immediate {
		task.execute(m.lastCycle)
	}
	m.pending = append(m.pending, task)
}

// Process ticks all active non-WALK_TO tasks once for the given cycle.
// See the TaskManager doc comment for the drain/tick/drop model.
func (m *TaskManager) Process(cycle int64) {
	m.lastCycle = cycle
	m.processedThisCycle = m.processedThisCycle[:0]
	for _, t := range m.pending {
		if t.running {
			m.active = append(m.active, t)
		}
	}
	m.pending = m.pending[:0]

	clear(m.walkToTicked)
	write := 0
	for i := 0; i < len(m.active); i++ {
		t := m.active[i]
		if t.running {
			m.processedThisCycle = append(m.processedThisCycle, t)
		}
		// WALK_TO tasks own an interaction's reach check. They are
		// ticked from the owner's own turn, after movement, so an
		// interaction resolves on the cycle the owner arrives instead
		// of the cycle after. (TaskManager.ts process().)
		if t.Type == TaskWalkTo {
			m.active[write] = t
			write++
			continue
		}
		keep := m.tickGuarded(t, cycle)
		if keep {
			m.active[write] = t
			write++
		}
	}
	m.active = m.active[:write]
}

// tickGuarded ticks one task, timing it for the slow-task hook and
// recovering panics (stopping the task). It reports whether the task
// should be kept.
func (m *TaskManager) tickGuarded(t *Task, cycle int64) (keep bool) {
	started := time.Now()
	defer func() {
		if r := recover(); r != nil {
			t.Stop()
			if m.OnError != nil {
				m.OnError(t, r)
			}
			keep = false
		}
	}()
	keep = t.Tick(cycle)
	m.logSlowTask(t.Name, time.Since(started))
	return keep
}

// ProcessWalkTo ticks the WALK_TO tasks belonging to one key, porting
// TaskManager.ts processWalkTo. It first activates pending WALK_TO tasks
// for the key (a walk-to submitted from client input during this cycle's
// turns must still get its reach check after this turn's movement), then
// ticks each matching task at most once per cycle. A throwing task is
// dropped rather than retried (TS processWalkTo catches, stops, and drops
// it, with a comment that a throwing interaction would otherwise re-throw
// every cycle).
func (m *TaskManager) ProcessWalkTo(key any, cycle int64) {
	m.activatePendingWalkTo(key)
	write := 0
	for i := 0; i < len(m.active); i++ {
		t := m.active[i]
		keep := true
		if t.Type == TaskWalkTo && t.Key == key && !m.walkToTicked[t] {
			m.walkToTicked[t] = true
			keep = m.tickWalkToGuarded(t, cycle)
		}
		if keep {
			m.active[write] = t
			write++
		}
	}
	m.active = m.active[:write]
}

func (m *TaskManager) tickWalkToGuarded(t *Task, cycle int64) (keep bool) {
	started := time.Now()
	defer func() {
		if r := recover(); r != nil {
			t.Stop()
			if m.OnError != nil {
				m.OnError(t, r)
			}
			keep = false
		}
	}()
	keep = t.Tick(cycle)
	m.logSlowTask(t.Name, time.Since(started))
	return keep
}

// ProcessRemainingWalkTo catches WALK_TO tasks whose owner did not take a
// turn this cycle. TaskManager.ts: processRemainingWalkTo().
func (m *TaskManager) ProcessRemainingWalkTo(cycle int64) {
	var keys []any
	seen := make(map[any]bool)
	for _, t := range m.active {
		if t.Type == TaskWalkTo && !m.walkToTicked[t] && !seen[t.Key] {
			seen[t.Key] = true
			keys = append(keys, t.Key)
		}
	}
	for _, key := range keys {
		m.ProcessWalkTo(key, cycle)
	}
}

// activatePendingWalkTo moves pending WALK_TO tasks for key into the
// active list, preserving the order of the remaining pending tasks.
// TaskManager.ts: activatePendingWalkTo().
func (m *TaskManager) activatePendingWalkTo(key any) {
	if len(m.pending) == 0 {
		return
	}
	found := false
	for _, t := range m.pending {
		if t.Type == TaskWalkTo && t.Key == key {
			found = true
			break
		}
	}
	if !found {
		return
	}
	kept := m.pending[:0]
	for _, t := range m.pending {
		if t.Type != TaskWalkTo || t.Key != key {
			kept = append(kept, t)
		} else if t.running {
			m.active = append(m.active, t)
		}
	}
	// Clear the tail so dropped entries do not retain references.
	for i := len(kept); i < len(m.pending); i++ {
		m.pending[i] = nil
	}
	m.pending = kept
}

// CancelTasks stops every pending and active task with the given key.
// TaskManager.ts: cancelTasks().
func (m *TaskManager) CancelTasks(key any) {
	for _, t := range m.pending {
		if t.Key == key {
			t.Stop()
		}
	}
	for _, t := range m.active {
		if t.Key == key {
			t.Stop()
		}
	}
}

// CancelWalkToTasks stops every WALK_TO task for the key without touching
// anything else. TaskManager.ts: cancelWalkToTasks().
func (m *TaskManager) CancelWalkToTasks(key any) {
	for _, t := range m.pending {
		if t.Key == key && t.Type == TaskWalkTo {
			t.Stop()
		}
	}
	for _, t := range m.active {
		if t.Key == key && t.Type == TaskWalkTo {
			t.Stop()
		}
	}
}

// HasWalkToTask reports whether the key still has a running WALK_TO task.
// TaskManager.ts: hasWalkToTask().
func (m *TaskManager) HasWalkToTask(key any) bool {
	matches := func(t *Task) bool {
		return t.running && t.Key == key && t.Type == TaskWalkTo
	}
	for _, t := range m.pending {
		if matches(t) {
			return true
		}
	}
	for _, t := range m.active {
		if matches(t) {
			return true
		}
	}
	return false
}

// HasActiveTask reports whether the key has a running task, optionally
// restricted to a task name. TaskManager.ts: hasActiveTask() (which
// matches on the constructor name; here Name plays that role).
func (m *TaskManager) HasActiveTask(key any, name string) bool {
	matches := func(t *Task) bool {
		return t.running && t.Key == key && (name == "" || t.Name == name)
	}
	for _, t := range m.pending {
		if matches(t) {
			return true
		}
	}
	for _, t := range m.active {
		if matches(t) {
			return true
		}
	}
	return false
}

// WasActiveThisCycle reports whether a task for the key was processed by
// the most recent Process call. TaskManager.ts: wasTaskActiveThisCycle().
func (m *TaskManager) WasActiveThisCycle(key any, name string) bool {
	for _, t := range m.processedThisCycle {
		if t.Key == key && (name == "" || t.Name == name) {
			return true
		}
	}
	return false
}

// Count returns the number of pending + active tasks.
// TaskManager.ts: getTaskAmount().
func (m *TaskManager) Count() int { return len(m.pending) + len(m.active) }

func (m *TaskManager) logSlowTask(name string, duration time.Duration) {
	if m.OnSlowTask == nil {
		return
	}
	durationMs := duration.Milliseconds()
	if durationMs < SlowTaskWarnMs {
		return
	}
	now := time.Now()
	if last, ok := m.lastSlowLog[name]; ok && now.Sub(last) < SlowTaskLogCooldownMs*time.Millisecond {
		return
	}
	m.lastSlowLog[name] = now
	m.OnSlowTask(name, durationMs)
}
