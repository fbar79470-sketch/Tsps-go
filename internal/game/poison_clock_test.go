package game

import (
	"testing"
	"time"
)

// PoisonClock unit tests (PoisonClock.ts). The clock's brief
// timers run on a fake wall clock; the dose pulse counts
// ticks through TickPulse.

func poisonClockWithTime(t *testing.T) (*PoisonClock, *time.Time) {
	t.Helper()
	now := time.Date(2026, 11, 17, 0, 0, 0, 0, time.UTC)
	clock := NewPoisonClock(nil)
	clock.now = func() time.Time { return now }
	clock.briefPoison.now = func() time.Time { return clock.clock() }
	clock.briefVenom.now = func() time.Time { return clock.clock() }
	return clock, &now
}

func TestPoisonClockStartCounterAndPhase(t *testing.T) {
	clock, _ := poisonClockWithTime(t)
	clock.Start(90)
	if got := clock.Counter(); got != 5 {
		t.Errorf("Start(90) counter = %d, want 5 (ceil(90/18))", got)
	}
	if got := clock.SecondsRemaining(); got != 90 {
		t.Errorf("Start(90) secondsRemaining = %v, want 90", got)
	}
	// A dose never moves the phase of an existing pulse.
	for i := 0; i < 7; i++ {
		clock.TickPulse(false)
	}
	clock.Start(36)
	if got := clock.Counter(); got != 2 {
		t.Errorf("re-Start counter = %d, want 2", got)
	}
	if got := clock.Snapshot().TicksUntilPulse; got != 23 {
		t.Errorf("re-Start ticksUntilPulse = %d, want 23 (phase untouched)", got)
	}
	// A non-positive dose stops the clock.
	clock.Start(0)
	if !clock.Finished() || clock.Counter() != 0 {
		t.Errorf("Start(0) should stop the clock, counter = %d finished = %v", clock.Counter(), clock.Finished())
	}
}

func TestPoisonClockTickPulse(t *testing.T) {
	clock, _ := poisonClockWithTime(t)
	clock.Start(36) // counter 2
	for i := 0; i < 29; i++ {
		if clock.TickPulse(false) {
			t.Fatalf("tick %d pulsed early", i+1)
		}
	}
	if !clock.TickPulse(false) {
		t.Fatal("tick 30 should pulse")
	}
	if got := clock.Counter(); got != 1 {
		t.Errorf("after one pulse counter = %d, want 1", got)
	}
	// A modal tick counts down but never pulses.
	for i := 0; i < 29; i++ {
		if clock.TickPulse(true) {
			t.Fatalf("modal tick %d pulsed", i+1)
		}
	}
	if clock.TickPulse(true) {
		t.Fatal("modal tick at phase 0 must not pulse")
	}
	if got := clock.Counter(); got != 1 {
		t.Errorf("modal hold counter = %d, want 1", got)
	}
	if !clock.TickPulse(false) {
		t.Fatal("first non-modal tick after the hold should pulse")
	}
	if got := clock.Counter(); got != 0 {
		t.Errorf("after second pulse counter = %d, want 0", got)
	}
}

func TestPoisonClockProtectBrieflyAndVenomImmunity(t *testing.T) {
	clock, now := poisonClockWithTime(t)
	clock.ProtectBriefly(60, true)
	if got := clock.SecondsRemaining(); got != 60 {
		t.Errorf("brief secondsRemaining = %v, want 60", got)
	}
	if !clock.IsVenomImmune() {
		t.Error("brief venom should make the clock venom-immune")
	}
	if clock.Finished() {
		t.Error("brief protection should keep the clock running")
	}
	// A shorter flag never replaces a longer one.
	clock.ProtectBriefly(10, true)
	if got := clock.SecondsRemaining(); got != 60 {
		t.Errorf("shorter protectBriefly replaced the flag: %v, want 60", got)
	}
	*now = now.Add(61 * time.Second)
	if clock.IsVenomImmune() {
		t.Error("brief venom should expire after its seconds")
	}
	if !clock.Finished() {
		t.Error("expired brief protection should finish the clock")
	}
	// Counter 39 is venom-immune, 38 is not (without brief venom).
	clock.Start(39 * 18)
	if !clock.IsVenomImmune() {
		t.Error("counter 39 should be venom-immune")
	}
	clock.Start(38 * 18)
	if clock.IsVenomImmune() {
		t.Error("counter 38 should not be venom-immune")
	}
}

func TestPoisonClockReduceProtection(t *testing.T) {
	clock, _ := poisonClockWithTime(t)
	clock.Start(90) // counter 5, 150 ticks of protection
	clock.ReduceProtection(36)
	if got := clock.Counter(); got != 3 {
		t.Errorf("after -36s counter = %d, want 3", got)
	}
	before, after, ok := clock.TakeReduction()
	if !ok || before != 5 || after != 3 {
		t.Errorf("TakeReduction = (%d, %d, %v), want (5, 3, true)", before, after, ok)
	}
	if _, _, ok := clock.TakeReduction(); ok {
		t.Error("second TakeReduction should report nothing pending")
	}
	// Reducing past zero clears the counter and the deadline.
	clock.ReduceProtection(1000)
	if clock.Counter() != 0 || clock.HasCounter() {
		t.Errorf("over-reduction counter = %d, want 0", clock.Counter())
	}
	if got := clock.SecondsRemaining(); got != 0 {
		t.Errorf("over-reduction secondsRemaining = %v, want 0", got)
	}
}

func TestPoisonClockReduceProtectionModalHold(t *testing.T) {
	clock, _ := poisonClockWithTime(t)
	clock.Start(36)           // counter 2, 60 ticks of protection
	clock.ReduceProtection(9) // 45 ticks left, counter still 2
	if got := clock.Counter(); got != 2 {
		t.Fatalf("reduced counter = %d, want 2", got)
	}
	// Tick the deadline down while modal: the counter must
	// not drop to 1 while modal, even past the 30-tick
	// boundary (31 ticks held).
	for i := 0; i < 20; i++ {
		if clock.TickPulse(true) {
			t.Fatalf("modal tick %d pulsed", i+1)
		}
	}
	if got := clock.Counter(); got != 2 {
		t.Fatalf("modal ticks counter = %d, want 2", got)
	}
	if got := clock.Snapshot().ProtectionTicksRemaining; got == nil || *got != 31 {
		t.Fatalf("held deadline = %v, want 31", got)
	}
	// The next non-modal tick releases the held transition:
	// the counter drops without a pulse (ticks remain).
	if clock.TickPulse(false) {
		t.Fatal("release tick must not pulse while phase ticks remain")
	}
	if got := clock.Counter(); got != 1 {
		t.Errorf("released counter = %d, want 1", got)
	}
}

func TestPoisonClockSecondsElapsed(t *testing.T) {
	clock, _ := poisonClockWithTime(t)
	clock.Start(90)
	// Wall-clock time alone does not consume a dose: the
	// seconds elapse only as the pulse ticks (TS
	// secondsElapsed is startingSeconds minus the
	// dose-based secondsRemaining).
	if got := clock.SecondsElapsed(); got != 0 {
		t.Errorf("secondsElapsed before any pulse = %v, want 0", got)
	}
	for i := 0; i < 30; i++ {
		clock.TickPulse(false)
	}
	if got := clock.SecondsRemaining(); got != 72 {
		t.Errorf("secondsRemaining after one pulse = %v, want 72", got)
	}
	if got := clock.SecondsElapsed(); got != 18 {
		t.Errorf("secondsElapsed after one pulse = %v, want 18", got)
	}
}

func TestPoisonClockSnapshotRestoreRoundTrip(t *testing.T) {
	clock, now := poisonClockWithTime(t)
	clock.Start(90)
	clock.ReduceProtection(18) // counter 4, deadline 120 ticks
	clock.ProtectBriefly(30, true)
	*now = now.Add(5 * time.Second) // brief poison 25s, brief venom 25s
	state := clock.Snapshot()
	if state.Counter != 4 {
		t.Fatalf("snapshot counter = %d, want 4", state.Counter)
	}
	if state.ProtectionTicksRemaining == nil || *state.ProtectionTicksRemaining != 120 {
		t.Fatalf("snapshot protectionTicks = %v, want 120", state.ProtectionTicksRemaining)
	}
	if state.BriefPoisonSeconds == nil || *state.BriefPoisonSeconds != 25 {
		t.Fatalf("snapshot brief poison = %v, want 25", state.BriefPoisonSeconds)
	}
	restored, _ := poisonClockWithTime(t)
	restored.Restore(&state, 0)
	if got := restored.Snapshot(); got.Counter != state.Counter ||
		got.TicksUntilPulse != state.TicksUntilPulse ||
		got.ProtectionTicksRemaining == nil || *got.ProtectionTicksRemaining != 120 ||
		got.BriefPoisonSeconds == nil || *got.BriefPoisonSeconds != 25 {
		t.Errorf("restored snapshot = %+v, want %+v", got, state)
	}
	if got := restored.SecondsRemaining(); got != clock.SecondsRemaining() {
		t.Errorf("restored secondsRemaining = %v, want %v", got, clock.SecondsRemaining())
	}
	// Mutating the snapshot afterwards must not alias live state.
	*state.ProtectionTicksRemaining = 1
	if got := restored.Snapshot().ProtectionTicksRemaining; got == nil || *got != 120 {
		t.Errorf("snapshot aliasing: restored deadline = %v, want 120", got)
	}
}

func TestPoisonClockRestoreValidation(t *testing.T) {
	// An invalid phase falls back to the legacy seconds.
	clock, _ := poisonClockWithTime(t)
	state := &PoisonClockSave{Counter: 2, TicksUntilPulse: 31}
	clock.Restore(state, 90)
	if got := clock.Counter(); got != 5 {
		t.Errorf("invalid state + legacy 90 counter = %d, want 5", got)
	}
	// Legacy <= 2s protects briefly instead of building a counter.
	clock.Restore(nil, 2)
	if clock.HasCounter() {
		t.Errorf("legacy 2s should not build a counter, got %d", clock.Counter())
	}
	if got := clock.SecondsRemaining(); got != 2 {
		t.Errorf("legacy 2s secondsRemaining = %v, want 2", got)
	}
	// A protection-ticks / counter mismatch is invalid too.
	clock.Restore(nil, 0)
	protection := 61
	state = &PoisonClockSave{Counter: 2, TicksUntilPulse: 30, ProtectionTicksRemaining: &protection}
	clock.Restore(state, 0)
	if clock.HasCounter() || clock.IsRunning() {
		t.Errorf("mismatched deadline should restore nothing, counter = %d", clock.Counter())
	}
	// Brief venom above brief poison is invalid.
	clock.Restore(nil, 0)
	briefPoison := 10.0
	briefVenom := 20.0
	state = &PoisonClockSave{Counter: 0, TicksUntilPulse: 30, BriefPoisonSeconds: &briefPoison, BriefVenomSeconds: &briefVenom}
	clock.Restore(state, 0)
	if clock.IsRunning() {
		t.Error("brief venom > brief poison should restore nothing")
	}
	// A valid zero-counter state with only brief seconds restores.
	clock.Restore(&PoisonClockSave{Counter: 0, TicksUntilPulse: 30, BriefPoisonSeconds: &briefPoison}, 0)
	if got := clock.SecondsRemaining(); got != 10 {
		t.Errorf("brief-only restore secondsRemaining = %v, want 10", got)
	}
}
