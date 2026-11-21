package game

import "time"

// SecondsTimer is the port of the SecondsTimer methods Trading and
// Duelling use (game/model/SecondsTimer.ts): Start, Finished,
// SecondsRemaining, and Stop. It is a WALL-CLOCK timer
// (performance.now in TS), not a tick countdown: the button delay
// (1 s) and the trade-request delay (2 s) expire in real seconds,
// independent of tick phase.
//
// No existing Go timer models this. The TimerRepository
// (timers.go) counts ticks, and its key set lives outside this
// phase's ownership; the cycle-stamp pattern (the pickup
// throttle in player.go) measures whole ticks, which would make
// a 1 s delay last 1-2 ticks depending on phase. So the four
// methods the two consumers call are ported directly here, with
// an injectable clock for tests. The remaining SecondsTimer
// surface — pause/resume, millisRemaining, isRunning, toString —
// has no caller in Trading or Duelling and is NOT ported.
type SecondsTimer struct {
	seconds int
	running bool
	start   time.Time
	now     func() time.Time
}

// newSecondsTimer returns a stopped timer reading the wall clock.
func newSecondsTimer() *SecondsTimer {
	return &SecondsTimer{now: time.Now}
}

// clock returns the current time through the injected clock,
// defaulting to the wall clock when none was injected (a
// zero-value SecondsTimer).
func (t *SecondsTimer) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// Start begins a countdown of seconds (SecondsTimer.ts:20-31:
// reset + start the stopwatch, record the start time, running).
// A paused timer is not modeled: pause/resume are not ported.
func (t *SecondsTimer) Start(seconds int) {
	t.seconds = seconds
	t.start = t.clock()
	t.running = true
}

// Stop ends the countdown (SecondsTimer.ts:33-37): zero seconds,
// not running. Finished calls it when the countdown reaches zero.
func (t *SecondsTimer) Stop() {
	t.seconds = 0
	t.running = false
}

// SecondsRemaining reports the whole seconds left
// (SecondsTimer.ts:57-65): seconds minus whole elapsed seconds,
// floored at 0; 0 when no countdown was ever started (seconds
// == 0, including after Stop).
func (t *SecondsTimer) SecondsRemaining() int {
	if t.seconds == 0 {
		return 0
	}
	remaining := t.seconds - t.secondsElapsed()
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// Finished reports whether the countdown has run out
// (SecondsTimer.ts:78-84): true when nothing remains, stopping
// the timer as a side effect; false while time remains. A timer
// that was never started is finished.
func (t *SecondsTimer) Finished() bool {
	if t.SecondsRemaining() == 0 {
		t.Stop()
		return true
	}
	return false
}

// secondsElapsed reports whole elapsed seconds
// (SecondsTimer.ts:86-91): 0 when not running, else the floor of
// the elapsed wall time in seconds.
func (t *SecondsTimer) secondsElapsed() int {
	if !t.running {
		return 0
	}
	return int(t.clock().Sub(t.start) / time.Second)
}
