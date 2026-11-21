package game

import (
	"strings"
	"sync"
	"testing"
)

func TestTimerRepositoryAbsentKey(t *testing.T) {
	r := NewTimerRepository()
	if r.Has(TimerKeyFreeze) {
		t.Fatal("absent key must answer Has false")
	}
	if got := r.Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("absent key Left = %d, want 0", got)
	}
	if got := r.GetTicks(TimerKeyFreeze); got != 0 {
		t.Fatalf("absent key GetTicks = %d, want 0", got)
	}
	if !r.WillEndIn(TimerKeyFreeze, 0) {
		t.Fatal("absent key must answer WillEndIn true, even for 0 ticks")
	}
	if !r.WillEndIn(TimerKeyFreeze, 100) {
		t.Fatal("absent key must answer WillEndIn true")
	}
	if r.HasActive() {
		t.Fatal("empty repository must answer HasActive false")
	}
	if got := len(r.Timers()); got != 0 {
		t.Fatalf("empty repository Timers() = %d entries, want 0", got)
	}
	// Absent-key writes are no-ops and must not panic or allocate
	// anything observable.
	r.Cancel(TimerKeyFreeze)
	r.Process()
	if got := len(r.Timers()); got != 0 {
		t.Fatalf("cancel/process on empty repository left %d entries, want 0", got)
	}
}

func TestTimerRegistersAndTicks(t *testing.T) {
	r := NewTimerRepository()
	r.Registers(TimerKeyFreeze, 3)
	if !r.Has(TimerKeyFreeze) {
		t.Fatal("registered timer must be active")
	}
	if got := r.Left(TimerKeyFreeze); got != 3 {
		t.Fatalf("Left = %d, want 3", got)
	}
	if got := r.GetTicks(TimerKeyFreeze); got != 3 {
		t.Fatalf("GetTicks = %d, want 3", got)
	}
	if r.WillEndIn(TimerKeyFreeze, 2) {
		t.Fatal("WillEndIn(2) must be false with 3 ticks left")
	}
	if !r.WillEndIn(TimerKeyFreeze, 3) {
		t.Fatal("WillEndIn(3) must be true with 3 ticks left (<=)")
	}
	if !r.HasActive() {
		t.Fatal("HasActive must be true with a live timer")
	}

	r.Process()
	if got := r.Left(TimerKeyFreeze); got != 2 {
		t.Fatalf("after 1 process Left = %d, want 2", got)
	}
	r.Process()
	r.Process()
	if r.Has(TimerKeyFreeze) {
		t.Fatal("timer at 0 must be inactive")
	}
	if got := r.Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("Left at expiry = %d, want 0", got)
	}
	// Entries stay in the map at 0: ticking never removes them.
	snap := r.Timers()
	if _, ok := snap[TimerKeyFreeze]; !ok {
		t.Fatal("expired timer must stay present in Timers()")
	}
	if r.HasActive() {
		t.Fatal("HasActive must be false once every timer is at 0")
	}
	// Further ticks floor at 0.
	r.Process()
	if got := r.Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("Left after extra process = %d, want 0", got)
	}
	if _, ok := r.Timers()[TimerKeyFreeze]; !ok {
		t.Fatal("expired timer must still be present after extra ticks")
	}
}

func TestTimerProcessTicksEveryEntry(t *testing.T) {
	r := NewTimerRepository()
	r.Registers(TimerKeyFreeze, 2)
	r.Registers(TimerKeyStun, 5)
	r.Registers(TimerKeyPotion, 1)
	r.Process()
	if got := r.Left(TimerKeyFreeze); got != 1 {
		t.Fatalf("FREEZE Left = %d, want 1", got)
	}
	if got := r.Left(TimerKeyStun); got != 4 {
		t.Fatalf("STUN Left = %d, want 4", got)
	}
	if got := r.Left(TimerKeyPotion); got != 0 {
		t.Fatalf("POTION Left = %d, want 0", got)
	}
}

func TestTimerRegisterStoresByOwnKey(t *testing.T) {
	r := NewTimerRepository()
	held := NewTimer(TimerKeyFreeze, 2)
	r.Register(held)
	if !r.Has(TimerKeyFreeze) || r.Left(TimerKeyFreeze) != 2 {
		t.Fatal("Register must store the timer under its own key")
	}
	// Process ticks the same object the caller holds.
	r.Process()
	if got := held.Ticks(); got != 1 {
		t.Fatalf("held timer Ticks = %d, want 1 (repository ticks the stored object)", got)
	}
	// Registering another timer for the same key replaces it; the
	// old object is then untouched by process.
	replacement := NewTimer(TimerKeyFreeze, 9)
	r.Register(replacement)
	r.Process()
	if got := held.Ticks(); got != 1 {
		t.Fatalf("replaced timer Ticks = %d, want 1 (untouched after replacement)", got)
	}
	if got := r.Left(TimerKeyFreeze); got != 8 {
		t.Fatalf("Left = %d, want 8", got)
	}
	// A nil timer is ignored, not stored and not a panic.
	r.Register(nil)
	if got := len(r.Timers()); got != 1 {
		t.Fatalf("Register(nil) left %d entries, want 1", got)
	}
}

func TestTimerRegisterTimerKey(t *testing.T) {
	r := NewTimerRepository()

	// The one key with a default: Misc.getTicks(180) = 300.
	r.RegisterTimerKey(TimerKeyBotWaitForPlayers)
	if !r.Has(TimerKeyBotWaitForPlayers) {
		t.Fatal("BOT_WAIT_FOR_PLAYERS must be active after RegisterTimerKey")
	}
	if got := r.Left(TimerKeyBotWaitForPlayers); got != 300 {
		t.Fatalf("BOT_WAIT_FOR_PLAYERS Left = %d, want 300", got)
	}

	// A no-default key stores an undefined-ticks timer: present
	// but never active, never ticking, and WillEndIn false
	// (the TS undefined comparisons — see timers.go).
	r.RegisterTimerKey(TimerKeyFood)
	if r.Has(TimerKeyFood) {
		t.Fatal("undefined-ticks timer must never count as active")
	}
	if got := r.Left(TimerKeyFood); got != 0 {
		t.Fatalf("undefined-ticks Left = %d, want 0", got)
	}
	if got := r.GetTicks(TimerKeyFood); got != 0 {
		t.Fatalf("undefined-ticks GetTicks = %d, want 0", got)
	}
	if r.WillEndIn(TimerKeyFood, 100) {
		t.Fatal("undefined-ticks timer must answer WillEndIn false (unlike an absent key)")
	}
	if _, ok := r.Timers()[TimerKeyFood]; !ok {
		t.Fatal("undefined-ticks timer must be present in Timers()")
	}
	r.Process()
	r.Process()
	if _, ok := r.Timers()[TimerKeyFood]; !ok {
		t.Fatal("undefined-ticks timer must survive process()")
	}
	if r.WillEndIn(TimerKeyFood, 100) {
		t.Fatal("undefined-ticks timer must still answer WillEndIn false after process()")
	}

	// A constructed key with a default registers that default;
	// one without behaves like FOOD.
	withDefault := NewTimerKey(7)
	r.RegisterTimerKey(withDefault)
	if got := r.Left(withDefault); got != 7 {
		t.Fatalf("constructed default Left = %d, want 7", got)
	}
	withoutDefault := NewTimerKey()
	r.RegisterTimerKey(withoutDefault)
	if r.Has(withoutDefault) || r.WillEndIn(withoutDefault, 100) {
		t.Fatal("constructed no-default key must behave like a no-default singleton")
	}

	// RegisterTimerKey replaces whatever the key held.
	r.Registers(TimerKeyPotion, 50)
	r.RegisterTimerKey(TimerKeyPotion)
	if r.Has(TimerKeyPotion) {
		t.Fatal("RegisterTimerKey must replace the held timer with the key's undefined ticks")
	}
}

func TestTimerExtendOrRegister(t *testing.T) {
	r := NewTimerRepository()

	// Absent: registers.
	r.ExtendOrRegister(TimerKeyFreeze, 5)
	if got := r.Left(TimerKeyFreeze); got != 5 {
		t.Fatalf("absent ExtendOrRegister Left = %d, want 5", got)
	}

	// Existing longer timer is kept (same object: a held
	// reference keeps ticking in place).
	held := NewTimer(TimerKeyStun, 10)
	r.Register(held)
	r.ExtendOrRegister(TimerKeyStun, 3)
	r.Process()
	if got := held.Ticks(); got != 9 {
		t.Fatalf("kept timer Ticks = %d, want 9 (existing longer timer survives)", got)
	}

	// Equal ticks keep the existing timer (>= keeps).
	r.ExtendOrRegister(TimerKeyStun, 9)
	r.Process()
	if got := held.Ticks(); got != 8 {
		t.Fatalf("equal-ticks timer Ticks = %d, want 8 (equal keeps existing)", got)
	}

	// Shorter existing timer is replaced (the held object is
	// then untouched by process).
	r.ExtendOrRegister(TimerKeyStun, 20)
	r.Process()
	if got := held.Ticks(); got != 8 {
		t.Fatalf("replaced timer Ticks = %d, want 8 (untouched after replacement)", got)
	}
	if got := r.Left(TimerKeyStun); got != 19 {
		t.Fatalf("Left = %d, want 19", got)
	}

	// An undefined-ticks timer is kept: undefined < ticks is
	// false in TS, so extendOrRegister never replaces it.
	r.RegisterTimerKey(TimerKeyFood)
	r.ExtendOrRegister(TimerKeyFood, 100)
	if r.Has(TimerKeyFood) {
		t.Fatal("ExtendOrRegister must keep an undefined-ticks timer")
	}
	if r.WillEndIn(TimerKeyFood, 100) {
		t.Fatal("undefined-ticks timer must still answer WillEndIn false after ExtendOrRegister")
	}
}

func TestTimerAddOrSet(t *testing.T) {
	r := NewTimerRepository()

	// Absent: sets.
	r.AddOrSet(TimerKeyFreeze, 4)
	if got := r.Left(TimerKeyFreeze); got != 4 {
		t.Fatalf("absent AddOrSet Left = %d, want 4", got)
	}

	// Present: accumulates, storing a fresh object.
	held := NewTimer(TimerKeyStun, 3)
	r.Register(held)
	r.AddOrSet(TimerKeyStun, 5)
	if got := r.Left(TimerKeyStun); got != 8 {
		t.Fatalf("AddOrSet Left = %d, want 8", got)
	}
	r.Process()
	if got := held.Ticks(); got != 3 {
		t.Fatalf("old timer Ticks = %d, want 3 (AddOrSet stores a fresh object)", got)
	}
	if got := r.Left(TimerKeyStun); got != 7 {
		t.Fatalf("Left after process = %d, want 7", got)
	}

	// Accumulation applies no clamping.
	r.AddOrSet(TimerKeyStun, -10)
	if got := r.Left(TimerKeyStun); got != -3 {
		t.Fatalf("AddOrSet Left = %d, want -3", got)
	}
	if r.Has(TimerKeyStun) {
		t.Fatal("negative timer must be inactive")
	}

	// Adding onto an undefined-ticks timer keeps it undefined:
	// TS stores NaN, which compares false everywhere, exactly
	// like undefined.
	r.RegisterTimerKey(TimerKeyFood)
	r.AddOrSet(TimerKeyFood, 50)
	if r.Has(TimerKeyFood) || r.WillEndIn(TimerKeyFood, 100) {
		t.Fatal("AddOrSet onto an undefined-ticks timer must keep it undefined")
	}
}

func TestTimerCancel(t *testing.T) {
	r := NewTimerRepository()
	r.Registers(TimerKeyFreeze, 5)
	r.Registers(TimerKeyStun, 5)
	r.Cancel(TimerKeyFreeze)
	if r.Has(TimerKeyFreeze) {
		t.Fatal("cancelled timer must be inactive")
	}
	if !r.WillEndIn(TimerKeyFreeze, 0) {
		t.Fatal("cancelled key must answer WillEndIn true (absent again)")
	}
	if _, ok := r.Timers()[TimerKeyFreeze]; ok {
		t.Fatal("cancelled timer must be gone from Timers()")
	}
	if !r.Has(TimerKeyStun) {
		t.Fatal("cancel must not touch other keys")
	}
	// Cancelling an absent key is a no-op.
	r.Cancel(TimerKeyFreeze)
	r.Cancel(TimerKeyPotion)
	if got := len(r.Timers()); got != 1 {
		t.Fatalf("Timers() = %d entries, want 1", got)
	}
}

func TestTimerZeroAndNegativeRegisters(t *testing.T) {
	r := NewTimerRepository()
	// No clamping on register: a 0 or negative count stores a
	// present-but-inactive timer, and ticking never moves it.
	r.Registers(TimerKeyPotion, 0)
	r.Registers(TimerKeyStun, -3)
	if r.Has(TimerKeyPotion) || r.Has(TimerKeyStun) {
		t.Fatal("0/negative timers must be inactive")
	}
	if got := r.Left(TimerKeyStun); got != -3 {
		t.Fatalf("negative Left = %d, want -3", got)
	}
	if _, ok := r.Timers()[TimerKeyPotion]; !ok {
		t.Fatal("0-tick timer must be present in Timers()")
	}
	if !r.WillEndIn(TimerKeyPotion, 0) {
		t.Fatal("0-tick timer must answer WillEndIn(0) true (0 <= 0)")
	}
	r.Process()
	if got := r.Left(TimerKeyStun); got != -3 {
		t.Fatalf("negative Left after process = %d, want -3", got)
	}
	if r.HasActive() {
		t.Fatal("HasActive must be false with only 0/negative timers")
	}
}

func TestTimerTickFloorsAtZero(t *testing.T) {
	timer := NewTimer(TimerKeyFreeze, 1)
	timer.Tick()
	if got := timer.Ticks(); got != 0 {
		t.Fatalf("Ticks = %d, want 0", got)
	}
	timer.Tick()
	if got := timer.Ticks(); got != 0 {
		t.Fatalf("Ticks after extra tick = %d, want 0 (floors, never negative)", got)
	}
	zero := NewTimer(TimerKeyFreeze, 0)
	zero.Tick()
	if got := zero.Ticks(); got != 0 {
		t.Fatalf("0-tick timer Ticks = %d, want 0", got)
	}
	negative := NewTimer(TimerKeyFreeze, -2)
	negative.Tick()
	if got := negative.Ticks(); got != -2 {
		t.Fatalf("negative timer Ticks = %d, want -2 (tick only decrements while > 0)", got)
	}
	if got := timer.Key(); got != TimerKeyFreeze {
		t.Fatalf("Key = %v, want TimerKeyFreeze", got)
	}
	// Nil-timer methods are safe.
	var nilTimer *Timer
	nilTimer.Tick()
	if got := nilTimer.Ticks(); got != 0 {
		t.Fatalf("nil timer Ticks = %d, want 0", got)
	}
	if got := nilTimer.Key(); got != (TimerKey{}) {
		t.Fatalf("nil timer Key = %v, want zero key", got)
	}
}

func TestTimerKeySingletons(t *testing.T) {
	keys := []TimerKey{
		TimerKeyFood, TimerKeyKarambwan, TimerKeyPotion,
		TimerKeyFreeze, TimerKeyFreezeImmunity,
		TimerKeyStun, TimerKeyStunImmunity,
		TimerKeyAttackImmunity, TimerKeyCastlewarsTakeItem,
		TimerKeySteppingOut, TimerKeyBotWaitForPlayers,
	}
	if len(keys) != 11 {
		t.Fatalf("got %d singletons, want 11", len(keys))
	}
	seen := make(map[TimerKey]bool)
	for _, k := range keys {
		if seen[k] {
			t.Fatalf("duplicate singleton key: %v", k)
		}
		seen[k] = true
	}
	// Singleton identity: the same key equals itself and keys
	// never collide in a map.
	r := NewTimerRepository()
	for i, k := range keys {
		r.Registers(k, i+1)
	}
	if got := len(r.Timers()); got != 11 {
		t.Fatalf("Timers() = %d entries, want 11 (no two singletons collide)", got)
	}
	// Only BOT_WAIT_FOR_PLAYERS carries a default.
	for _, k := range keys {
		if k == TimerKeyBotWaitForPlayers {
			if !k.HasDefaultTicks() {
				t.Fatal("BOT_WAIT_FOR_PLAYERS must carry a default")
			}
			if got := k.GetTicks(); got != 300 {
				t.Fatalf("BOT_WAIT_FOR_PLAYERS default = %d, want 300", got)
			}
			continue
		}
		if k.HasDefaultTicks() {
			t.Fatalf("%v must not carry a default", k)
		}
		if got := k.GetTicks(); got != 0 {
			t.Fatalf("%v GetTicks = %d, want 0 (no default)", k, got)
		}
	}
	if got := TimerKeyFreeze.String(); got != "TimerKey.FREEZE" {
		t.Fatalf("String = %q, want %q", got, "TimerKey.FREEZE")
	}
	if got := TimerKeyBotWaitForPlayers.String(); got != "TimerKey.BOT_WAIT_FOR_PLAYERS" {
		t.Fatalf("String = %q, want %q", got, "TimerKey.BOT_WAIT_FOR_PLAYERS")
	}
}

func TestNewTimerKeyDistinct(t *testing.T) {
	a := NewTimerKey()
	b := NewTimerKey()
	if a == b {
		t.Fatal("two constructed keys must be distinct")
	}
	singletons := []TimerKey{
		TimerKeyFood, TimerKeyKarambwan, TimerKeyPotion,
		TimerKeyFreeze, TimerKeyFreezeImmunity,
		TimerKeyStun, TimerKeyStunImmunity,
		TimerKeyAttackImmunity, TimerKeyCastlewarsTakeItem,
		TimerKeySteppingOut, TimerKeyBotWaitForPlayers,
	}
	for _, s := range singletons {
		if a == s || b == s {
			t.Fatalf("constructed key %v collides with singleton %v", a, s)
		}
	}
	if a.HasDefaultTicks() || b.HasDefaultTicks() {
		t.Fatal("keys constructed with no argument must have no default")
	}
	withDefault := NewTimerKey(42)
	if !withDefault.HasDefaultTicks() || withDefault.GetTicks() != 42 {
		t.Fatalf("NewTimerKey(42) default = %d/%v, want 42/true", withDefault.GetTicks(), withDefault.HasDefaultTicks())
	}
	if withDefault == a || withDefault == b {
		t.Fatal("constructed key with a default must still be distinct")
	}
	// The zero key is distinct from every singleton and usable
	// as a map key without colliding with one.
	var zero TimerKey
	for _, s := range singletons {
		if zero == s {
			t.Fatalf("zero key collides with singleton %v", s)
		}
	}
	if got := zero.String(); got != "TimerKey#0" {
		t.Fatalf("zero key String = %q, want %q", got, "TimerKey#0")
	}
	if !strings.HasPrefix(a.String(), "TimerKey#") {
		t.Fatalf("constructed key String = %q, want TimerKey#<id>", a.String())
	}
	// Constructed keys work as repository keys.
	r := NewTimerRepository()
	r.Registers(a, 2)
	r.Registers(b, 3)
	if got := r.Left(a); got != 2 {
		t.Fatalf("Left(a) = %d, want 2", got)
	}
	if got := r.Left(b); got != 3 {
		t.Fatalf("Left(b) = %d, want 3", got)
	}
}

func TestTimerRepositoryNilSafe(t *testing.T) {
	var r *TimerRepository
	if r.Has(TimerKeyFreeze) || r.HasActive() {
		t.Fatal("nil repository must answer Has/HasActive false")
	}
	if r.Left(TimerKeyFreeze) != 0 || r.GetTicks(TimerKeyFreeze) != 0 {
		t.Fatal("nil repository must answer Left/GetTicks 0")
	}
	if !r.WillEndIn(TimerKeyFreeze, 0) {
		t.Fatal("nil repository must answer WillEndIn true")
	}
	if got := len(r.Timers()); got != 0 {
		t.Fatalf("nil repository Timers() = %d entries, want 0", got)
	}
	// Writes to a nil repository are safe no-ops.
	r.Register(NewTimer(TimerKeyFreeze, 5))
	r.RegisterTimerKey(TimerKeyFreeze)
	r.Registers(TimerKeyFreeze, 5)
	r.ExtendOrRegister(TimerKeyFreeze, 5)
	r.AddOrSet(TimerKeyFreeze, 5)
	r.Cancel(TimerKeyFreeze)
	r.Process()
}

func TestTimerRepositoryZeroValueUsable(t *testing.T) {
	var r TimerRepository
	r.Registers(TimerKeyFreeze, 2)
	if !r.Has(TimerKeyFreeze) {
		t.Fatal("zero-value repository must accept registrations")
	}
	r.Process()
	if got := r.Left(TimerKeyFreeze); got != 1 {
		t.Fatalf("Left = %d, want 1", got)
	}
}

func TestTimersSnapshotIsCopy(t *testing.T) {
	r := NewTimerRepository()
	r.Registers(TimerKeyFreeze, 5)
	snap := r.Timers()
	snap[TimerKeyFreeze] = *NewTimer(TimerKeyFreeze, 99)
	delete(snap, TimerKeyFreeze)
	snap[TimerKeyStun] = *NewTimer(TimerKeyStun, 1)
	if got := r.Left(TimerKeyFreeze); got != 5 {
		t.Fatalf("Left = %d, want 5 (mutating the snapshot must not affect the repository)", got)
	}
	if r.Has(TimerKeyStun) {
		t.Fatal("adding to the snapshot must not affect the repository")
	}
	if got := len(r.Timers()); got != 1 {
		t.Fatalf("Timers() = %d entries, want 1", got)
	}
}

func TestTimerRepositoryConcurrentUse(t *testing.T) {
	r := NewTimerRepository()
	keys := []TimerKey{TimerKeyFreeze, TimerKeyStun, TimerKeyPotion, TimerKeyFood}
	var wg sync.WaitGroup
	for _, k := range keys {
		wg.Add(1)
		go func(k TimerKey) {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				r.Registers(k, 50)
				r.Has(k)
				r.Left(k)
				r.Process()
			}
		}(k)
	}
	wg.Wait()
	if got := len(r.Timers()); got != len(keys) {
		t.Fatalf("Timers() = %d entries, want %d", got, len(keys))
	}
}

func TestMobileGetTimersLazy(t *testing.T) {
	m := NewMobile(Location{X: 3222, Y: 3222, Z: 0})
	if m.timers != nil {
		t.Fatal("fresh Mobile must hold no repository yet")
	}
	// ProcessTimers on a mobile with no repository allocates
	// nothing: there is nothing to tick.
	m.ProcessTimers(1)
	if m.timers != nil {
		t.Fatal("ProcessTimers must not allocate a repository for a mobile with no timers")
	}
	repo := m.GetTimers()
	if repo == nil {
		t.Fatal("GetTimers must allocate a repository")
	}
	if m.GetTimers() != repo {
		t.Fatal("GetTimers must always return the same repository")
	}
}

func TestMobileProcessTimersDrivesRepository(t *testing.T) {
	m := NewMobile(Location{X: 3222, Y: 3222, Z: 0})
	m.GetTimers().Registers(TimerKeyFreeze, 3)
	// The cycle argument is accepted but unused: TS process()
	// takes no cycle, so any value ticks exactly once.
	m.ProcessTimers(99)
	if got := m.GetTimers().Left(TimerKeyFreeze); got != 2 {
		t.Fatalf("Left = %d, want 2", got)
	}
	m.ProcessTimers(-7)
	m.ProcessTimers(0)
	if got := m.GetTimers().Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("Left = %d, want 0", got)
	}
	m.ProcessTimers(1)
	if got := m.GetTimers().Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("Left = %d, want 0 (floors at 0)", got)
	}
}

func TestPlayerAndNPCProcessTimers(t *testing.T) {
	p := NewPlayer("TimerTest", Location{X: 3222, Y: 3222, Z: 0})
	p.GetTimers().Registers(TimerKeyStun, 2)
	p.ProcessTimers(1)
	if got := p.GetTimers().Left(TimerKeyStun); got != 1 {
		t.Fatalf("player Left = %d, want 1", got)
	}

	n := NewNPC(1, Location{X: 3222, Y: 3222, Z: 0})
	n.GetTimers().Registers(TimerKeyFreeze, 2)
	n.ProcessTimers(1)
	if got := n.GetTimers().Left(TimerKeyFreeze); got != 1 {
		t.Fatalf("npc Left = %d, want 1", got)
	}
}

func TestPlayerProcessTicksTimers(t *testing.T) {
	p := NewPlayer("TimerTest", Location{X: 3222, Y: 3222, Z: 0})
	p.GetTimers().Registers(TimerKeyFreeze, 2)
	p.Process(1)
	if got := p.GetTimers().Left(TimerKeyFreeze); got != 1 {
		t.Fatalf("Left after Process = %d, want 1", got)
	}
	p.Process(2)
	if got := p.GetTimers().Left(TimerKeyFreeze); got != 0 {
		t.Fatalf("Left after 2nd Process = %d, want 0", got)
	}
}

func TestNPCProcessTicksTimers(t *testing.T) {
	n := NewNPC(1, Location{X: 3222, Y: 3222, Z: 0})
	n.GetTimers().Registers(TimerKeyStun, 1)
	n.Process(1)
	if got := n.GetTimers().Left(TimerKeyStun); got != 0 {
		t.Fatalf("Left after Process = %d, want 0", got)
	}
}

// timerTestPlayer builds a player with a recording session and
// walking (not running) so step counts are deterministic.
func timerTestPlayer() (*Player, *fakeSession) {
	p := NewPlayer("TimerTest", Location{X: 3222, Y: 3222, Z: 0})
	p.SetRunning(false)
	sess := newFakeSession()
	p.Session = sess
	return p, sess
}

func TestFreezeBlocksRequestWalkSilently(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	q.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if got := q.Size(); got != 0 {
		t.Fatalf("frozen RequestWalk queued %d checkpoints, want 0", got)
	}
	if len(sess.messages) != 0 || len(sess.typedMessages) != 0 {
		t.Fatalf("frozen refusal must send nothing, got messages=%v typed=%v", sess.messages, sess.typedMessages)
	}
	// Once the timer expires, the same click walks.
	for i := 0; i < 5; i++ {
		p.ProcessTimers(int64(i))
	}
	if p.GetTimers().Has(TimerKeyFreeze) {
		t.Fatal("freeze must have expired after 5 ticks")
	}
	q.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if got := q.Size(); got == 0 {
		t.Fatal("RequestWalk after freeze expiry must queue checkpoints")
	}
}

func TestStunBlocksRequestWalkWithSpamLine(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyStun, 5)
	q.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if got := q.Size(); got != 0 {
		t.Fatalf("stunned RequestWalk queued %d checkpoints, want 0", got)
	}
	if len(sess.messages) != 0 {
		t.Fatalf("stun refusal must send no plain message, got %v", sess.messages)
	}
	if len(sess.typedMessages) != 1 {
		t.Fatalf("stun refusal sent %d typed messages, want 1: %v", len(sess.typedMessages), sess.typedMessages)
	}
	got := sess.typedMessages[0]
	if got[0] != chatTypeSpam || got[1] != "You're stunned!" || got[2] != "" {
		t.Fatalf("typed message = %v, want [105 \"You're stunned!\" \"\"]", got)
	}
}

func TestFrozenAndStunnedReportsFrozen(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	// getMobility checks FREEZE before STUN: a frozen-and-
	// stunned click is silent, not a stun line.
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	p.GetTimers().Registers(TimerKeyStun, 5)
	q.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if got := q.Size(); got != 0 {
		t.Fatalf("frozen+stunned RequestWalk queued %d checkpoints, want 0", got)
	}
	if len(sess.typedMessages) != 0 || len(sess.messages) != 0 {
		t.Fatalf("frozen+stunned refusal must send nothing, got typed=%v messages=%v", sess.typedMessages, sess.messages)
	}
}

func TestStunBeatsBlockMovement(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	// TS order in getMobilityIgnoringFreeze: STUN is checked
	// before the INVALID sources, so a blocked-and-stunned
	// click still sends the stun line.
	q.SetBlockMovement(true)
	p.GetTimers().Registers(TimerKeyStun, 5)
	q.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if len(sess.typedMessages) != 1 || sess.typedMessages[0][1] != "You're stunned!" {
		t.Fatalf("blocked+stunned refusal must send the stun line, got %v", sess.typedMessages)
	}

	// Blocked alone stays silent (INVALID sends nothing).
	p2, sess2 := timerTestPlayer()
	q2 := p2.MovementQueue()
	q2.SetBlockMovement(true)
	q2.RequestWalk(Location{X: 3225, Y: 3222, Z: 0}, nil)
	if len(sess2.typedMessages) != 0 || len(sess2.messages) != 0 {
		t.Fatalf("blocked refusal must send nothing, got typed=%v messages=%v", sess2.typedMessages, sess2.messages)
	}
}

func TestFreezeStopsQueuedSteps(t *testing.T) {
	p, _ := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	q.RequestWalk(Location{X: 3226, Y: 3222, Z: 0}, nil)
	queued := q.Size()
	if queued == 0 {
		t.Fatal("RequestWalk must queue checkpoints before the freeze")
	}
	p.GetTimers().Registers(TimerKeyFreeze, 3)
	q.BeginCycle()
	q.Process(1)
	if got := p.Location(); got != start {
		t.Fatalf("frozen player moved to %v, want to stay at %v", got, start)
	}
	if got := q.Size(); got != queued {
		t.Fatalf("frozen Process kept %d checkpoints, want %d (queue is kept, not cleared)", got, queued)
	}
	if q.IsMoving() {
		t.Fatal("frozen Process must report not moving")
	}
	// Expire the freeze: the kept queue resumes stepping.
	for i := 0; i < 3; i++ {
		p.ProcessTimers(int64(i))
	}
	q.BeginCycle()
	q.Process(2)
	if got := p.Location(); got == start {
		t.Fatal("player must step once the freeze expires")
	}
}

func TestStunStopsQueuedStepsWithoutMessage(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	q.RequestWalk(Location{X: 3226, Y: 3222, Z: 0}, nil)
	p.GetTimers().Registers(TimerKeyStun, 2)
	q.BeginCycle()
	q.Process(1)
	if got := p.Location(); got != start {
		t.Fatalf("stunned player moved to %v, want to stay at %v", got, start)
	}
	// Process never sends a refusal line: sendMessage runs
	// only at the request/walk-to entry points.
	if len(sess.typedMessages) != 0 || len(sess.messages) != 0 {
		t.Fatalf("Process must send no refusal, got typed=%v messages=%v", sess.typedMessages, sess.messages)
	}
}

func TestAddCheckpointRefusedWhileFrozen(t *testing.T) {
	p, _ := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyFreeze, 2)
	q.AddCheckpoint(Location{X: 3223, Y: 3222, Z: 0})
	if got := q.Size(); got != 0 {
		t.Fatalf("frozen AddCheckpoint queued %d checkpoints, want 0", got)
	}
	p.GetTimers().Cancel(TimerKeyFreeze)
	q.AddCheckpoint(Location{X: 3223, Y: 3222, Z: 0})
	if got := q.Size(); got != 1 {
		t.Fatalf("AddCheckpoint after cancel queued %d checkpoints, want 1", got)
	}
}

func TestPlayerProcessFrozenDoesNotStep(t *testing.T) {
	p, _ := timerTestPlayer()
	start := p.Location()
	p.MovementQueue().RequestWalk(Location{X: 3226, Y: 3222, Z: 0}, nil)
	p.GetTimers().Registers(TimerKeyFreeze, 1)
	// The freeze ticks down in the timer phase of this same
	// turn (ProcessTimers runs before movement), so register
	// enough ticks to survive the turn's own tick.
	p.GetTimers().Registers(TimerKeyFreeze, 2)
	p.Process(1)
	if got := p.Location(); got != start {
		t.Fatalf("frozen player moved to %v, want to stay at %v", got, start)
	}
	// The next turn ticks the last freeze tick away before
	// movement, so the kept queue steps.
	p.Process(2)
	if got := p.Location(); got == start {
		t.Fatal("player must step on the turn the freeze expires")
	}
}

func TestWalkToTileStunRefusal(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyStun, 5)
	called := false
	q.WalkToTile(Location{X: 3230, Y: 3222, Z: 0}, func() { called = true }, nil, false, 1)
	if called {
		t.Fatal("stunned WalkToTile must not run the action")
	}
	if q.WalkToPending() {
		t.Fatal("stunned WalkToTile must not install a walk-to")
	}
	if got := q.Size(); got != 0 {
		t.Fatalf("stunned WalkToTile left %d checkpoints, want 0 (reset)", got)
	}
	if len(sess.typedMessages) != 1 || sess.typedMessages[0][1] != "You're stunned!" {
		t.Fatalf("stunned WalkToTile must send the stun line, got %v", sess.typedMessages)
	}
}

func TestWalkToTileExactMatchRunsWhileStunned(t *testing.T) {
	p, _ := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyStun, 5)
	// An exact tile match runs the action before any mobility
	// check (TS call-time short-circuit).
	called := false
	q.WalkToTile(p.Location(), func() { called = true }, nil, false, 1)
	if !called {
		t.Fatal("exact-tile WalkToTile must run the action even while stunned")
	}
}

func TestWalkToTileFrozenSilent(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	called := false
	q.WalkToTile(Location{X: 3230, Y: 3222, Z: 0}, func() { called = true }, nil, false, 1)
	if called || q.WalkToPending() {
		t.Fatal("frozen WalkToTile must neither run the action nor install a walk-to")
	}
	if len(sess.typedMessages) != 0 || len(sess.messages) != 0 {
		t.Fatalf("frozen WalkToTile must send nothing, got typed=%v messages=%v", sess.typedMessages, sess.messages)
	}
}

func timerTestObject(loc Location) ObjectTarget {
	return ObjectTarget{
		ID:        1,
		Location:  loc,
		Type:      10,
		Face:      0,
		SizeX:     1,
		SizeY:     1,
		BaseSizeX: 1,
		BaseSizeY: 1,
	}
}

func TestWalkToObjectFrozenReachesFromTile(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	// A 1x1 object on the orthogonally adjacent tile: reached
	// from where the frozen player stands.
	target := timerTestObject(Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	called := false
	q.WalkToObject(target, func() { called = true }, 1, 1, WalkToObjectHooks{})
	if !q.WalkToPending() {
		t.Fatal("frozen WalkToObject in reach must install the walk-to")
	}
	if got := q.Size(); got != 0 {
		t.Fatalf("frozen object route queued %d checkpoints, want 0 (AddCheckpoint refuses while frozen)", got)
	}
	q.BeginCycle()
	q.Process(1)
	if !called {
		t.Fatal("frozen WalkToObject in reach must fire the action without stepping")
	}
	if got := p.Location(); got != start {
		t.Fatalf("frozen player moved to %v, want to stay at %v", got, start)
	}
	if len(sess.messages) != 0 || len(sess.typedMessages) != 0 {
		t.Fatalf("frozen in-reach WalkToObject must send nothing, got messages=%v typed=%v", sess.messages, sess.typedMessages)
	}
}

func TestWalkToObjectFrozenOutOfReach(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	target := timerTestObject(Location{X: 3230, Y: 3222, Z: 0})
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	called := false
	q.WalkToObject(target, func() { called = true }, 1, 1, WalkToObjectHooks{})
	if called || q.WalkToPending() {
		t.Fatal("frozen out-of-reach WalkToObject must neither run the action nor install a walk-to")
	}
	if len(sess.messages) != 1 || sess.messages[0] != "I can't reach that!" {
		t.Fatalf("frozen out-of-reach WalkToObject must say so, got %v", sess.messages)
	}
	if len(sess.typedMessages) != 0 {
		t.Fatalf("frozen out-of-reach WalkToObject must send no typed message, got %v", sess.typedMessages)
	}
}

func TestWalkToObjectStunned(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	target := timerTestObject(Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	p.GetTimers().Registers(TimerKeyStun, 5)
	called := false
	q.WalkToObject(target, func() { called = true }, 1, 1, WalkToObjectHooks{})
	if called || q.WalkToPending() {
		t.Fatal("stunned WalkToObject must neither run the action nor install a walk-to")
	}
	if len(sess.typedMessages) != 1 || sess.typedMessages[0][1] != "You're stunned!" {
		t.Fatalf("stunned WalkToObject must send the stun line, got %v", sess.typedMessages)
	}
}

func TestWalkToObjectFrozenAndStunned(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	target := timerTestObject(Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	// Frozen AND stunned: the blocking check looks past the
	// freeze, finds the stun, and sends the stun line — the
	// frozen in-reach exception does not apply.
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	p.GetTimers().Registers(TimerKeyStun, 5)
	called := false
	q.WalkToObject(target, func() { called = true }, 1, 1, WalkToObjectHooks{})
	if called || q.WalkToPending() {
		t.Fatal("frozen+stunned WalkToObject must neither run the action nor install a walk-to")
	}
	if len(sess.typedMessages) != 1 || sess.typedMessages[0][1] != "You're stunned!" {
		t.Fatalf("frozen+stunned WalkToObject must send the stun line, got %v", sess.typedMessages)
	}
	if len(sess.messages) != 0 {
		t.Fatalf("frozen+stunned WalkToObject must not say unreachable, got %v", sess.messages)
	}
}

func TestWalkToObjectBlockedSilent(t *testing.T) {
	p, sess := timerTestPlayer()
	q := p.MovementQueue()
	start := p.Location()
	target := timerTestObject(Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	q.SetBlockMovement(true)
	called := false
	q.WalkToObject(target, func() { called = true }, 1, 1, WalkToObjectHooks{})
	if called || q.WalkToPending() {
		t.Fatal("blocked WalkToObject must neither run the action nor install a walk-to")
	}
	if len(sess.typedMessages) != 0 || len(sess.messages) != 0 {
		t.Fatalf("blocked WalkToObject must send nothing, got typed=%v messages=%v", sess.typedMessages, sess.messages)
	}
}
