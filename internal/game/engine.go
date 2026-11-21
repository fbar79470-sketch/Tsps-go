package game

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

// Engine ports game/GameEngine.ts: a monotonic 600ms tick schedule with
// drift correction, no-overlap skip, and tick-error handling.
//
// Schedule (GameEngine.ts init/run/logTickLag):
//   - The first tick fires after one tick rate.
//   - The clock is monotonic (TS performance.now(); Go time.Now/time.Timer
//     are monotonic), so wall-clock jumps cannot disturb the schedule.
//   - Drift correction: each firing advances the expected schedule by one
//     tick rate; if the firing is more than a tick behind the advanced
//     schedule, the schedule resets to firing time + tick rate (no
//     catch-up burst). The first firing sets the schedule instead of
//     being measured against it (plugin startup work is not game lag).
//   - No-overlap: if a tick is still in progress when the next firing
//     arrives, the firing is skipped (counted, reported via OnOverlap)
//     and no tick is queued. TS logs an overlap warning and returns
//     without running.
//   - Tick errors: a panic in the process function is recovered and
//     reported via onTickError. In TS the catch logs and saves all
//     players with reason "tick-error" (World.savePlayers); persistence
//     is NOT STARTED in this package, so the coordinator performs the
//     save-all behind onTickError.
//
// Adaptation, documented: TS schedules the next run after the current
// tick completes (scheduleNextRun in run()'s finally), so the overlap
// guard is defensive. The Go engine advances the schedule per firing and
// runs each tick in its own goroutine guarded by an in-progress flag, so
// a slow tick produces skips on subsequent firings rather than delaying
// the schedule. The observable contract is the same: ticks fire at most
// once per rate, never overlap, and never catch up.
//
// The event-loop probe, ServerPerf timings, lag/overrun/freeze
// diagnostics (GameEngine.ts probeEventLoopDelay/logTickLag/
// logTickOverrun/logFreezeDiagnostic) are observability, NOT STARTED
// here (ANALYSIS §6.1: "port last or not at all"). OnOverlap is the only
// diagnostic hook in this foundation.
type Engine struct {
	tickRate    time.Duration
	process     func(cycle int64)
	onTickError func(cycle int64, err error)

	// OnOverlap reports a skipped firing: tick is the TS-style tick
	// number (counting attempts, including skipped ones). It runs on the
	// scheduler goroutine and must return quickly and must not panic.
	// Nil means skips are only counted (see SkippedCount).
	OnOverlap func(tick int64)

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}

	inProgress atomic.Bool
	attempts   atomic.Int64 // TS tickNumber: increments per firing, skips included.
	executed   atomic.Int64 // Ticks actually started.
	skipped    atomic.Int64
}

// NewEngine returns an Engine with the given tick rate in milliseconds.
// A non-positive rate defaults to TickRateMs (600). process runs once per
// tick with the TS-style tick number; onTickError reports a recovered
// panic from process (nil onTickError logs via log.Printf).
func NewEngine(tickRateMs int, process func(cycle int64), onTickError func(cycle int64, err error)) *Engine {
	if tickRateMs <= 0 {
		tickRateMs = TickRateMs
	}
	if onTickError == nil {
		onTickError = func(cycle int64, err error) {
			log.Printf("[engine] tick error tick=%d: %v", cycle, err)
		}
	}
	return &Engine{
		tickRate:    time.Duration(tickRateMs) * time.Millisecond,
		process:     process,
		onTickError: onTickError,
	}
}

// Start begins the tick schedule: the first tick fires after one tick
// rate. Start is idempotent while running (a second call is a no-op). A
// nil ctx is treated as context.Background. Stop ends the schedule; a
// stopped Engine may be started again.
func (e *Engine) Start(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	runCtx, cancel := context.WithCancel(ctx)
	e.cancel = cancel
	e.done = make(chan struct{})
	e.running = true
	e.mu.Unlock()
	go e.loop(runCtx)
}

// Stop ends the tick schedule and waits for the scheduler goroutine to
// exit. A tick already in flight runs to completion (Stop does not wait
// for it); the process function must therefore tolerate running briefly
// past Stop, exactly as a TS tick in flight past shutdown would. Stop is
// safe to call when the Engine is not running.
func (e *Engine) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	cancel := e.cancel
	done := e.done
	e.mu.Unlock()
	cancel()
	<-done
	e.mu.Lock()
	e.running = false
	e.cancel = nil
	e.done = nil
	e.mu.Unlock()
}

// TickCount returns the number of ticks actually started (skipped firings
// excluded). TS tickNumber counts attempts including overlaps; use
// TickCount + SkippedCount for the TS-equivalent attempt count.
func (e *Engine) TickCount() int64 { return e.executed.Load() }

// SkippedCount returns the number of firings skipped because a tick was
// already in progress.
func (e *Engine) SkippedCount() int64 { return e.skipped.Load() }

func (e *Engine) loop(ctx context.Context) {
	defer close(e.done)
	nextExpected := time.Now().Add(e.tickRate)
	first := true
	for {
		delay := time.Until(nextExpected)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case startedAt := <-timer.C:
			// Advance the drift-corrected schedule, mirroring
			// logTickLag: the first firing sets the schedule; later
			// firings advance it by one rate, resetting when more
			// than a tick behind.
			if first {
				nextExpected = startedAt.Add(e.tickRate)
				first = false
			} else {
				nextExpected = nextExpected.Add(e.tickRate)
				if startedAt.After(nextExpected.Add(e.tickRate)) {
					nextExpected = startedAt.Add(e.tickRate)
				}
			}
			e.fire()
		}
	}
}

func (e *Engine) fire() {
	tick := e.attempts.Add(1)
	if !e.inProgress.CompareAndSwap(false, true) {
		e.skipped.Add(1)
		if e.OnOverlap != nil {
			e.OnOverlap(tick)
		}
		return
	}
	e.executed.Add(1)
	go e.runTick(tick)
}

func (e *Engine) runTick(tick int64) {
	defer e.inProgress.Store(false)
	defer func() {
		if r := recover(); r != nil {
			e.onTickError(tick, fmt.Errorf("panic: %v", r))
		}
	}()
	if e.process != nil {
		e.process(tick)
	}
}
