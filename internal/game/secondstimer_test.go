package game

import (
	"testing"
	"time"
)

func TestSecondsTimerNeverStartedIsFinished(t *testing.T) {
	timer := newSecondsTimer()
	if !timer.Finished() {
		t.Fatal("never-started timer: expected finished")
	}
	if got := timer.SecondsRemaining(); got != 0 {
		t.Fatalf("never-started timer: remaining = %d, want 0", got)
	}
}

func TestSecondsTimerCountsWholeSeconds(t *testing.T) {
	now := time.Unix(1000, 0)
	timer := newSecondsTimer()
	timer.now = func() time.Time { return now }
	timer.Start(2)
	if timer.Finished() {
		t.Fatal("just started: expected not finished")
	}
	if got := timer.SecondsRemaining(); got != 2 {
		t.Fatalf("remaining = %d, want 2", got)
	}
	now = now.Add(1500 * time.Millisecond)
	if got := timer.SecondsRemaining(); got != 1 {
		t.Fatalf("after 1.5s: remaining = %d, want 1 (whole seconds)", got)
	}
	if timer.Finished() {
		t.Fatal("after 1.5s: expected not finished")
	}
	now = now.Add(500 * time.Millisecond)
	if !timer.Finished() {
		t.Fatal("after 2s: expected finished")
	}
	// Finished stops the timer: a second call stays finished with
	// zero remaining.
	if got := timer.SecondsRemaining(); got != 0 {
		t.Fatalf("after finish: remaining = %d, want 0", got)
	}
}

func TestSecondsTimerStopAndRestart(t *testing.T) {
	now := time.Unix(1000, 0)
	timer := newSecondsTimer()
	timer.now = func() time.Time { return now }
	timer.Start(10)
	timer.Stop()
	if !timer.Finished() {
		t.Fatal("stopped timer: expected finished")
	}
	timer.Start(1)
	if timer.Finished() {
		t.Fatal("restarted timer: expected not finished")
	}
	now = now.Add(2 * time.Second)
	if !timer.Finished() {
		t.Fatal("restarted timer after 2s: expected finished")
	}
}

func TestSecondsTimerZeroValue(t *testing.T) {
	// A zero-value timer reads the wall clock and reports
	// finished (never started), without panicking.
	var timer SecondsTimer
	if !timer.Finished() {
		t.Fatal("zero timer: expected finished")
	}
}
