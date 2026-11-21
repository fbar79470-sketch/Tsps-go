package game

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestEngineTicksAtRate(t *testing.T) {
	var count atomic.Int64
	e := NewEngine(5, func(cycle int64) { count.Add(1) }, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	time.Sleep(60 * time.Millisecond)
	e.Stop()
	n := e.TickCount()
	if n < 5 || n > 30 {
		t.Fatalf("TickCount = %d over ~60ms at 5ms rate, want 5..30 (timing tolerance)", n)
	}
	// Stop waits for a tick already in flight (engine.go), so the
	// process count must already equal TickCount on return — this
	// is the regression check for the 2026-11-21 race fix.
	if count.Load() != n {
		t.Fatalf("process calls = %d, TickCount = %d (Stop must wait for the in-flight tick)", count.Load(), n)
	}
}

func TestEngineNoOverlap(t *testing.T) {
	var current, maxConcurrent atomic.Int64
	e := NewEngine(5, func(cycle int64) {
		c := current.Add(1)
		for {
			m := maxConcurrent.Load()
			if c <= m || maxConcurrent.CompareAndSwap(m, c) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		current.Add(-1)
	}, nil)
	overlaps := atomic.Int64{}
	e.OnOverlap = func(tick int64) { overlaps.Add(1) }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	time.Sleep(150 * time.Millisecond)
	e.Stop()
	if maxConcurrent.Load() != 1 {
		t.Fatalf("max concurrent ticks = %d, want 1 (no overlap)", maxConcurrent.Load())
	}
	if e.SkippedCount() == 0 || overlaps.Load() == 0 {
		t.Fatal("slow ticks must produce skipped firings + overlap reports")
	}
	if e.TickCount() < 2 {
		t.Fatalf("TickCount = %d, want >= 2", e.TickCount())
	}
}

func TestEngineTickErrorHook(t *testing.T) {
	var errs atomic.Int64
	e := NewEngine(5, func(cycle int64) { panic("tick boom") }, func(cycle int64, err error) {
		errs.Add(1)
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	time.Sleep(40 * time.Millisecond)
	e.Stop()
	if errs.Load() == 0 {
		t.Fatal("onTickError must be called for panicking ticks")
	}
	if e.TickCount() == 0 {
		t.Fatal("engine must keep ticking after tick errors")
	}
	// A recovered tick must release the in-progress guard: the next
	// firing must execute, not skip forever.
	if e.TickCount() < 2 && e.SkippedCount() > e.TickCount() {
		t.Fatal("in-progress guard must release after a tick panic")
	}
}

func TestEngineStopHaltsTicks(t *testing.T) {
	e := NewEngine(5, func(cycle int64) {}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	time.Sleep(30 * time.Millisecond)
	e.Stop()
	afterStop := e.TickCount()
	time.Sleep(30 * time.Millisecond)
	if e.TickCount() != afterStop {
		t.Fatalf("ticks continued after Stop: %d -> %d", afterStop, e.TickCount())
	}
	// Context cancellation also halts the schedule.
	e2 := NewEngine(5, func(cycle int64) {}, nil)
	ctx2, cancel2 := context.WithCancel(context.Background())
	e2.Start(ctx2)
	time.Sleep(30 * time.Millisecond)
	cancel2()
	time.Sleep(30 * time.Millisecond)
	stopped := e2.TickCount()
	time.Sleep(20 * time.Millisecond)
	if e2.TickCount() != stopped {
		t.Fatal("ticks continued after context cancellation")
	}
	e2.Stop() // must not hang or panic after ctx cancel
}

func TestEngineFirstTickAfterRate(t *testing.T) {
	// The rate is long relative to the checks below: an earlier
	// version slept 20 ms against a 50 ms rate, and under parallel
	// load the sleep overshot the rate, so the tick had legitimately
	// fired before the assertion ran. The "must wait" check is now
	// immediate (no sleep can overshoot it), and the "must fire"
	// check polls with a generous deadline.
	e := NewEngine(200, func(cycle int64) {}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	if e.TickCount() != 0 {
		t.Fatal("first tick must wait a full tick rate")
	}
	deadline := time.Now().Add(600 * time.Millisecond)
	for e.TickCount() < 1 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	e.Stop()
	if e.TickCount() < 1 {
		t.Fatal("first tick must fire after the rate elapses")
	}
}

func TestEngineDoubleStartIsNoop(t *testing.T) {
	e := NewEngine(5, func(cycle int64) {}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	e.Start(ctx)
	e.Start(ctx) // must not start a second scheduler
	time.Sleep(30 * time.Millisecond)
	e.Stop()
	// Two schedulers would double the tick count; with tolerance, a
	// single scheduler at 5ms over 30ms stays well under 30.
	if e.TickCount() > 30 {
		t.Fatalf("TickCount = %d, suggests a second scheduler ran", e.TickCount())
	}
}
