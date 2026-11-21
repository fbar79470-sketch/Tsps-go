package game

import (
	"math"
	"time"
)

// PoisonClock is the port of game/content/combat/PoisonClock.ts
// (159 lines): the shared player status pulse. Advancing it is
// owned by CombatPoisonEffect (poison.go): the clock itself never
// ticks on its own — TickPulse runs once per effect tick, and
// the brief timers below run on the wall clock, exactly as the
// TS SecondsTimer they mirror does (SecondsTimer.ts:
// performance.now-based; a dose counts in 18-second steps of
// 30 ticks).
//
// Constants (PoisonClock.ts:11-13): PERIOD_TICKS 30,
// PERIOD_SECONDS 18, VENOM_CUTOFF 38.
//
// A dose changes the counter, never the phase of an existing
// pulse (Start). protectBriefly sets separate short flags
// (equipment/food); neither replaces a dose nor cures.
// reduceProtection removes protection time without moving the
// pulse and records the pendingReduction transition the
// effect reports as resistance/reduction lines.
// tickPulse counts down ticksUntilPulse; a due pulse
// decrements the counter (the protection-ticks path holds a
// due transition while modal). isVenomImmune is counter > 38
// or a brief-venom timer running. snapshot/restore carry the
// save shape (persist PoisonClockState) with the TS strict
// validation; legacy seconds rebuild the counter + phase when
// no state survives.
//
// The brief timers are wall-clock timers with float seconds,
// mirroring SecondsTimer.start/secondsRemaining/finished:
// secondsRemaining is the started seconds minus whole elapsed
// seconds, floored at 0; Finished stops the timer as a side
// effect when nothing remains. The Go SecondsTimer in
// secondstimer.go takes int seconds and is shared with trading
// and duelling, so the clock keeps its own small float timer
// instead of widening that type. now is injectable for tests;
// production uses time.Now.

// PoisonClockPeriodTicks is PERIOD_TICKS (30): ticks per
// 18-second dose step.
const PoisonClockPeriodTicks = 30

// PoisonClockPeriodSeconds is PERIOD_SECONDS (18): seconds
// per dose step.
const PoisonClockPeriodSeconds = 18

// PoisonClockVenomCutoff is VENOM_CUTOFF (38): counters above
// it are venom resistance; 38 and below are poison resistance.
const PoisonClockVenomCutoff = 38

// PoisonClockSave is the snapshot shape
// (PoisonClock.ts:3-9, PoisonClockState; persist
// PoisonClockState): the counter, the phase, the optional
// reduced-dose deadline, and the optional brief seconds.
// It deliberately mirrors persist.PoisonClockState
// field-for-field so the save wiring converts between them
// without interpretation; this package cannot import persist
// (see doc.go layering), so the shape is restated here.
type PoisonClockSave struct {
	Counter                  int
	TicksUntilPulse          int
	ProtectionTicksRemaining *int
	BriefPoisonSeconds       *float64
	BriefVenomSeconds        *float64
}

// poisonReduction is one pending dose reduction
// (PoisonClock.ts: pendingReduction {before, after}).
type poisonReduction struct {
	before int
	after  int
}

// poisonBriefTimer is one SecondsTimer (model/SecondsTimer.ts)
// as PoisonClock uses it: float seconds on the wall clock.
type poisonBriefTimer struct {
	seconds   float64
	running   bool
	startedAt time.Time
	now       func() time.Time
}

// clock returns the current time through the injected clock.
func (t *poisonBriefTimer) clock() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

// start begins a countdown of seconds (SecondsTimer.ts:20-31).
func (t *poisonBriefTimer) start(seconds float64) {
	t.seconds = seconds
	t.startedAt = t.clock()
	t.running = true
}

// stop ends the countdown (SecondsTimer.ts:33-37).
func (t *poisonBriefTimer) stop() {
	t.seconds = 0
	t.running = false
}

// secondsElapsed reports whole elapsed seconds
// (SecondsTimer.ts:86-91): 0 when not running.
func (t *poisonBriefTimer) secondsElapsed() float64 {
	if !t.running {
		return 0
	}
	return math.Floor(t.clock().Sub(t.startedAt).Seconds())
}

// secondsRemaining reports the seconds left
// (SecondsTimer.ts:57-65): started seconds minus whole
// elapsed seconds, floored at 0; 0 when never started.
func (t *poisonBriefTimer) secondsRemaining() float64 {
	if t.seconds == 0 {
		return 0
	}
	remaining := t.seconds - t.secondsElapsed()
	if remaining < 0 {
		remaining = 0
	}
	return remaining
}

// finished reports whether the countdown has run out
// (SecondsTimer.ts:78-84): true when nothing remains,
// stopping the timer as a side effect; false while time
// remains. A timer that was never started is finished.
func (t *poisonBriefTimer) finished() bool {
	if t.secondsRemaining() == 0 {
		t.stop()
		return true
	}
	return false
}

// PoisonClock is the shared player status pulse
// (PoisonClock.ts). The zero value is usable but starts with
// ticksUntilPulse 0; NewPoisonClock is the constructor.
type PoisonClock struct {
	counter                  int
	protectionTicksRemaining *int
	ticksUntilPulse          int
	briefPoison              poisonBriefTimer
	briefVenom               poisonBriefTimer
	startingSeconds          float64
	pendingReduction         *poisonReduction
	ensureTask               func()
	now                      func() time.Time
}

// NewPoisonClock returns a PoisonClock with a full phase
// (ticksUntilPulse 30). ensureTask runs when a dose starts,
// a reduction drops the counter, or a restore leaves a
// counter — in TS it resumes the poison effect
// (Combat.ts:808-812, CombatPoisonEffect.resume); in Go the
// Combat.PoisonClock accessor passes ResumePoisonEffect for
// the owner. A nil ensureTask is inert.
func NewPoisonClock(ensureTask func()) *PoisonClock {
	c := &PoisonClock{
		ticksUntilPulse: PoisonClockPeriodTicks,
		ensureTask:      ensureTask,
		now:             time.Now,
	}
	// The brief timers read the clock's time, so a test that
	// replaces c.now moves them too.
	c.briefPoison.now = func() time.Time { return c.clock() }
	c.briefVenom.now = func() time.Time { return c.clock() }
	return c
}

// clock returns the current time through the injected clock.
func (c *PoisonClock) clock() time.Time {
	if c.now != nil {
		return c.now()
	}
	return time.Now()
}

// ensure runs the ensure callback, if one is installed.
func (c *PoisonClock) ensure() {
	if c.ensureTask != nil {
		c.ensureTask()
	}
}

// Start starts a dose of seconds (PoisonClock.ts start): the
// counter is ceil(seconds / 18) and never moves the phase of
// an existing pulse. A non-finite or non-positive dose stops
// the clock instead.
func (c *PoisonClock) Start(seconds float64) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		c.Stop()
		return
	}
	c.counter = int(math.Ceil(seconds / PoisonClockPeriodSeconds))
	c.pendingReduction = nil
	c.protectionTicksRemaining = nil
	c.startingSeconds = seconds
	c.ensure()
}

// ProtectBriefly sets a short protection flag
// (PoisonClock.ts protectBriefly): equipment/food flags that
// neither replace a dose nor cure. The flag only ever grows:
// a shorter new flag leaves the longer running one alone.
// venom also flags brief venom protection.
func (c *PoisonClock) ProtectBriefly(seconds float64, venom bool) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return
	}
	if c.briefPoison.secondsRemaining() < seconds {
		c.briefPoison.start(seconds)
	}
	if venom && c.briefVenom.secondsRemaining() < seconds {
		c.briefVenom.start(seconds)
	}
}

// protectionTicks is the protection time left in ticks
// (PoisonClock.ts protectionTicks): the reduced-dose deadline
// when one exists, else the counter's remaining ticks.
func (c *PoisonClock) protectionTicks() int {
	if c.protectionTicksRemaining != nil {
		return *c.protectionTicksRemaining
	}
	if c.counter > 0 {
		remaining := (c.counter-1)*PoisonClockPeriodTicks + c.ticksUntilPulse
		if remaining < 0 {
			return 0
		}
		return remaining
	}
	return 0
}

// ReduceProtection removes protection time without moving
// the status pulse or changing an affliction
// (PoisonClock.ts reduceProtection): the counter drops to
// ceil(remaining / 30) and the transition is recorded for
// the effect to report; the brief timers lose the same
// seconds.
func (c *PoisonClock) ReduceProtection(seconds float64) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return
	}
	before := c.counter
	remaining := c.protectionTicks() - int(math.Ceil(seconds/0.6))
	if remaining < 0 {
		remaining = 0
	}
	c.protectionTicksRemaining = &remaining
	c.counter = int(math.Ceil(float64(remaining) / PoisonClockPeriodTicks))
	if remaining == 0 {
		c.protectionTicksRemaining = nil
	}
	if before > c.counter {
		previous := before
		if c.pendingReduction != nil {
			previous = c.pendingReduction.before
		}
		c.pendingReduction = &poisonReduction{before: previous, after: c.counter}
		c.ensure()
	}
	poison := c.briefPoison.secondsRemaining() - seconds
	if poison < 0 {
		poison = 0
	}
	venom := c.briefVenom.secondsRemaining() - seconds
	if venom < 0 {
		venom = 0
	}
	if poison > 0 {
		c.briefPoison.start(poison)
	} else {
		c.briefPoison.stop()
	}
	if venom > 0 {
		c.briefVenom.start(venom)
	} else {
		c.briefVenom.stop()
	}
}

// Counter returns the dose counter (getCounter).
func (c *PoisonClock) Counter() int { return c.counter }

// HasPendingReduction reports whether a reduction
// transition is waiting to be reported.
func (c *PoisonClock) HasPendingReduction() bool { return c.pendingReduction != nil }

// TakeReduction takes the pending reduction transition, if
// any (takeReduction): the effect owns the feedback,
// including reductions before its next tick. The second
// return is false when nothing was pending.
func (c *PoisonClock) TakeReduction() (before, after int, ok bool) {
	if c.pendingReduction == nil {
		return 0, 0, false
	}
	transition := c.pendingReduction
	c.pendingReduction = nil
	return transition.before, transition.after, true
}

// HasCounter reports whether a dose counter is running.
func (c *PoisonClock) HasCounter() bool { return c.counter > 0 }

// ClearCounter clears the dose counter and any reduction
// state (clearCounter).
func (c *PoisonClock) ClearCounter() {
	c.counter = 0
	c.protectionTicksRemaining = nil
	c.pendingReduction = nil
}

// IsVenomImmune reports venom immunity (isVenomImmune):
// the counter is above the venom cutoff, or a brief-venom
// timer is running.
func (c *PoisonClock) IsVenomImmune() bool {
	return c.counter > PoisonClockVenomCutoff || !c.briefVenom.finished()
}

// TickPulse counts toward one deadline (tickPulse): the
// phase counts down; a due pulse decrements the counter
// (the protection-ticks path holds a due transition while
// modal) and reports true. While modal, or while the phase
// has ticks left, it reports false and never moves the
// pulse. At most one overdue pulse is retained.
func (c *PoisonClock) TickPulse(modal bool) bool {
	c.ticksUntilPulse--
	if c.ticksUntilPulse < 0 {
		c.ticksUntilPulse = 0
	}
	if c.protectionTicksRemaining != nil {
		remaining := *c.protectionTicksRemaining - 1
		if remaining < 0 {
			remaining = 0
		}
		counter := int(math.Ceil(float64(remaining) / PoisonClockPeriodTicks))
		// A reduced dose has its own deadline. Hold a due
		// transition while modal, just as an ordinary dose
		// holds its due shared pulse; never move that pulse.
		if !modal || counter == c.counter {
			c.protectionTicksRemaining = &remaining
			c.counter = counter
			if remaining == 0 {
				c.protectionTicksRemaining = nil
			}
		}
	}
	if modal || c.ticksUntilPulse > 0 {
		return false
	}
	c.ticksUntilPulse = PoisonClockPeriodTicks
	if c.protectionTicksRemaining == nil && c.counter > 0 {
		c.counter--
	}
	return true
}

// ResetPhase restores the full phase (resetPhase).
func (c *PoisonClock) ResetPhase() { c.ticksUntilPulse = PoisonClockPeriodTicks }

// Stop clears the counter and both brief timers (stop).
func (c *PoisonClock) Stop() {
	c.pendingReduction = nil
	c.ClearCounter()
	c.briefPoison.stop()
	c.briefVenom.stop()
}

// Finished reports whether no dose and no brief poison
// protection remain (finished).
func (c *PoisonClock) Finished() bool {
	return !c.HasCounter() && c.briefPoison.finished()
}

// IsRunning reports whether any protection remains.
func (c *PoisonClock) IsRunning() bool { return !c.Finished() }

// SecondsRemaining reports the protection seconds left
// (secondsRemaining): the dose ticks at 0.6s each, or the
// brief-poison seconds, whichever is longer.
func (c *PoisonClock) SecondsRemaining() float64 {
	dose := math.Ceil(float64(c.protectionTicks()) * 0.6)
	brief := c.briefPoison.secondsRemaining()
	if dose > brief {
		return dose
	}
	return brief
}

// SecondsElapsed reports the seconds consumed from the
// starting dose (secondsElapsed).
func (c *PoisonClock) SecondsElapsed() float64 {
	elapsed := c.startingSeconds - c.SecondsRemaining()
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

// Snapshot returns the save state (snapshot): the counter
// and phase always, the reduced-dose deadline and the brief
// seconds only while positive.
func (c *PoisonClock) Snapshot() PoisonClockSave {
	state := PoisonClockSave{Counter: c.counter, TicksUntilPulse: c.ticksUntilPulse}
	if c.protectionTicksRemaining != nil {
		remaining := *c.protectionTicksRemaining
		state.ProtectionTicksRemaining = &remaining
	}
	if brief := c.briefPoison.secondsRemaining(); brief > 0 {
		state.BriefPoisonSeconds = &brief
	}
	if brief := c.briefVenom.secondsRemaining(); brief > 0 {
		state.BriefVenomSeconds = &brief
	}
	return state
}

// maxSafeCounter is the restore bound (PoisonClock.ts
// restore): counter <= MAX_SAFE_INTEGER / PERIOD_TICKS.
const maxSafeCounter = int(^uint(0)>>1) / PoisonClockPeriodTicks

// Restore rebuilds the clock from a snapshot
// (PoisonClock.ts restore): a nil state restores nothing
// (the legacy-seconds branch below still applies). The
// state is accepted only when every field validates —
// counter bounds, phase 0..30, protection-ticks
// consistency (ceil(remaining / 30) == counter), brief
// seconds finite and >= 0, brief venom <= brief poison —
// otherwise the legacy seconds rebuild it: <= 2s protects
// briefly, else the counter + phase are rebuilt from the
// seconds. Either way startingSeconds becomes the seconds
// remaining, and a surviving counter resumes the effect.
func (c *PoisonClock) Restore(state *PoisonClockSave, legacySeconds float64) {
	c.Stop()
	c.ResetPhase()
	if state != nil &&
		state.Counter >= 0 && state.Counter <= maxSafeCounter &&
		state.TicksUntilPulse >= 0 && state.TicksUntilPulse <= PoisonClockPeriodTicks &&
		(state.ProtectionTicksRemaining == nil || (*state.ProtectionTicksRemaining > 0 &&
			int(math.Ceil(float64(*state.ProtectionTicksRemaining)/PoisonClockPeriodTicks)) == state.Counter)) &&
		(state.BriefPoisonSeconds == nil || (!math.IsNaN(*state.BriefPoisonSeconds) && !math.IsInf(*state.BriefPoisonSeconds, 0) && *state.BriefPoisonSeconds >= 0)) &&
		(state.BriefVenomSeconds == nil || (!math.IsNaN(*state.BriefVenomSeconds) && !math.IsInf(*state.BriefVenomSeconds, 0) && *state.BriefVenomSeconds >= 0 &&
			*state.BriefVenomSeconds <= float64Value(state.BriefPoisonSeconds))) {
		c.counter = state.Counter
		c.ticksUntilPulse = state.TicksUntilPulse
		// Copy the deadline: the caller's state (a save
		// struct) must never alias live clock state.
		if state.ProtectionTicksRemaining != nil {
			remaining := *state.ProtectionTicksRemaining
			c.protectionTicksRemaining = &remaining
		}
		if state.BriefPoisonSeconds != nil && *state.BriefPoisonSeconds != 0 {
			c.briefPoison.start(*state.BriefPoisonSeconds)
		}
		if state.BriefVenomSeconds != nil && *state.BriefVenomSeconds != 0 {
			c.briefVenom.start(*state.BriefVenomSeconds)
		}
	} else if !math.IsNaN(legacySeconds) && !math.IsInf(legacySeconds, 0) && legacySeconds > 0 {
		// Old saves knew only whole seconds; exact
		// historical phase is unavailable.
		if legacySeconds <= 2 {
			c.ProtectBriefly(legacySeconds, false)
		} else {
			c.counter = int(math.Ceil(legacySeconds / PoisonClockPeriodSeconds))
			phase := int(math.Ceil((legacySeconds - float64(c.counter-1)*PoisonClockPeriodSeconds) / 0.6))
			if phase > PoisonClockPeriodTicks {
				phase = PoisonClockPeriodTicks
			}
			c.ticksUntilPulse = phase
		}
	}
	c.startingSeconds = c.SecondsRemaining()
	if c.counter > 0 {
		c.ensure()
	}
}

// float64Value dereferences a *float64, or 0 when nil.
func float64Value(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}
