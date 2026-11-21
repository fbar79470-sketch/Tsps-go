package game

import (
	"fmt"
	"sync"
	"sync/atomic"
)

// Timers: the port of game/util/timers/ (TimerRepository.ts,
// Timer.ts, TimerKey.ts) plus the Mobile field and accessor
// (Mobile.ts:73, 963-965). Every Mobile carries one repository,
// ticked once per turn from Player.process (Player.ts:501,
// before movement) and NPC.process (NPC.ts:309) through
// Mobile.ProcessTimers (see entity.go).
//
// SEMANTICS. A repository maps keys to countdown timers. A
// timer is active while its ticks are > 0; process() ticks
// every entry once per turn, decrementing only while > 0, and
// entries stay in the map at 0 forever — ticking never removes
// them, only cancel() does (TimerRepository.ts:65-71,
// Timer.ts:21-25). All repository methods below mirror the TS
// method of the same name, including the absent-key answers
// (has false, left/getTicks 0, willEndIn true).
//
// TICKS ARE INTS. TS ticks are JS numbers (floats), but every
// value the system ever holds is integral: the only default is
// Misc.getTicks(180) = 300, and every registers/extendOrRegister/
// addOrSet call site in core and plugins passes whole ticks
// (combat freeze/stun ticks, Misc.getTicks(3) = 5). No caller
// depends on fractional ticks, so Go counts in ints and the
// float-to-int collapse is unobservable.
//
// UNDEFINED TICKS. Ten of the eleven TimerKey singletons are
// constructed with no ticks argument (TimerKey.ts:4-14), so
// key.getTicks() returns undefined at runtime, and
// registerTimerKey stores new Timer(key, undefined)
// (TimerRepository.ts:21-23). Such a timer never counts as
// active (undefined > 0 is false) and tick() never changes it
// (undefined > 0 is false); willEndIn answers false for it
// (undefined <= ticks is false), where an absent key answers
// true; extendOrRegister keeps it (undefined < ticks is
// false); addOrSet onto it produces NaN, which compares false
// everywhere and so behaves exactly like undefined. Go has no
// undefined int, so Timer carries a defined flag: false is the
// Go form of undefined ticks, and every method above
// reproduces the comparisons listed here. Ticks()/Left()/
// GetTicks() report 0 for an undefined timer — the int
// collapse of a value TS callers could only ever compare,
// never print. The path is unreachable in practice:
// registerTimerKey has zero callers outside its own
// definition at commit bfd81e3, and the one key with a
// default (BOT_WAIT_FOR_PLAYERS) is itself never registered
// in core; the faithful undefined handling exists so a
// future caller gets TS behavior, not an invented default.
//
// THREAD-SAFETY. The repository guards its map with an RWMutex
// and is always held behind a pointer (see Mobile.timers), so
// a Mobile value copy never copies the mutex — the same shape
// as the attribute store (attributes.go). The mutex is
// required for the same reason: login-goroutine content can
// register timers while the tick goroutine processes them.
// Methods on a nil *TimerRepository are safe no-ops/zero
// reads, following the attributes/message nil-receiver
// precedent: a nil repository behaves like an empty one that
// can never gain entries.
//
// NO SERIALIZATION. The repository is session-only: neither
// applyToPlayer nor fromPlayer in PlayerSave.ts touches
// getTimers/TimerRepository/TimerKey (zero references in the
// file), and the save's scalar *Timer fields
// (lastVengeanceTimer, poisonImmunityTimer, fireImmunityTimer,
// teleblockTimer, skullTimer) belong to separate SecondsTimer/
// clock systems, not to this repository. Nothing here is
// read or written by internal/persist.

// TimerKey identifies one countdown in a TimerRepository.
// TimerKey.ts:3-27.
//
// TS keys are object identities: each singleton is a distinct
// `new TimerKey()` instance, and Map lookup is by reference.
// Go keys are struct values carrying a unique id, so distinct
// keys can never collide: the eleven singletons hold ids 1-11
// and every constructed key takes a fresh id from a package
// counter. Copying a key copies its id, so a copied key is the
// same key, as a copied TS reference is. The zero TimerKey
// (id 0, no name, no default) is distinct from every singleton
// and every constructed key; it is a usable map key but means
// nothing — content uses the singletons.
//
// CONSTRUCTION IS EXPOSED. The TS constructor is public
// (`constructor(ticks?: number)`, TimerKey.ts:20-22) and
// PluginManager hands the TimerKey class itself to plugin
// content (PluginManager.ts:2690, PluginTypes.ts:1636), so
// content may construct keys; NewTimerKey is the Go form. No
// core or plugin code constructs a key at commit bfd81e3 (the
// only `new TimerKey` sites are the singletons), but the
// capability exists in TS and is preserved rather than
// closed off.
type TimerKey struct {
	id           int64
	name         string
	defaultTicks int
	hasDefault   bool
}

// timerKeySingletons is the id range reserved for the eleven
// singletons below; constructed keys take ids above it.
const timerKeySingletonCount = 11

// timerKeyNextID hands out ids for constructed keys. It starts
// above the singleton range so a constructed key can never
// collide with a singleton.
var timerKeyNextID atomic.Int64

func init() {
	timerKeyNextID.Store(timerKeySingletonCount + 1)
}

// The eleven TimerKey singletons (TimerKey.ts:4-15). Treat them
// as constants: TS declares them `static readonly`, and Go
// cannot express that for struct values, so nothing reassigns
// them. Only BOT_WAIT_FOR_PLAYERS carries default ticks:
// Misc.getTicks(180) = 180 / 0.6 = 300 (Misc.ts:14-16); the
// rest are constructed with no ticks argument, so GetTicks
// has no default to return (see the file header).
var (
	TimerKeyFood               = TimerKey{id: 1, name: "FOOD"}
	TimerKeyKarambwan          = TimerKey{id: 2, name: "KARAMBWAN"}
	TimerKeyPotion             = TimerKey{id: 3, name: "POTION"}
	TimerKeyFreeze             = TimerKey{id: 4, name: "FREEZE"}
	TimerKeyFreezeImmunity     = TimerKey{id: 5, name: "FREEZE_IMMUNITY"}
	TimerKeyStun               = TimerKey{id: 6, name: "STUN"}
	TimerKeyStunImmunity       = TimerKey{id: 7, name: "STUN_IMMUNITY"}
	TimerKeyAttackImmunity     = TimerKey{id: 8, name: "ATTACK_IMMUNITY"}
	TimerKeyCastlewarsTakeItem = TimerKey{id: 9, name: "CASTLEWARS_TAKE_ITEM"}
	TimerKeySteppingOut        = TimerKey{id: 10, name: "STEPPING_OUT"}
	TimerKeyBotWaitForPlayers  = TimerKey{id: 11, name: "BOT_WAIT_FOR_PLAYERS", defaultTicks: 300, hasDefault: true}
)

// NewTimerKey constructs a fresh key, distinct from every
// singleton and every other constructed key: the Go form of
// `new TimerKey()` (TimerKey.ts:20-22). At most one default
// tick count may be given, mirroring the constructor's single
// optional argument; further arguments are ignored, as JS
// ignores extra constructor arguments. A key constructed with
// no default behaves like the ten no-default singletons when
// passed to RegisterTimerKey (see the file header).
func NewTimerKey(defaultTicks ...int) TimerKey {
	key := TimerKey{id: timerKeyNextID.Add(1) - 1}
	if len(defaultTicks) > 0 {
		key.defaultTicks = defaultTicks[0]
		key.hasDefault = true
	}
	return key
}

// GetTicks returns the key's default tick count, or 0 when the
// key was constructed without one. TimerKey.ts:24-26
// (getTicks): TS returns undefined for a no-default key; 0 is
// the int collapse, and HasDefaultTicks tells the two apart.
// RegisterTimerKey — the only caller that must distinguish —
// reads HasDefaultTicks, never this value alone.
func (k TimerKey) GetTicks() int {
	return k.defaultTicks
}

// HasDefaultTicks reports whether the key was constructed with
// a default tick count. It is a GO-ONLY HELPER: TS reads the
// undefined directly, but Go needs the flag to tell "default
// 0" from "no default" (see the file header).
func (k TimerKey) HasDefaultTicks() bool {
	return k.hasDefault
}

// String names the key for debugging: "TimerKey.FREEZE" for a
// singleton, "TimerKey#<id>" for a constructed or zero key.
// GO-ONLY: TS keys have no names.
func (k TimerKey) String() string {
	if k.name != "" {
		return "TimerKey." + k.name
	}
	return fmt.Sprintf("TimerKey#%d", k.id)
}

// Timer is one countdown: a key plus its remaining ticks.
// Timer.ts:4-26. The zero Timer has the zero key and undefined
// ticks (defined false); build timers with NewTimer. A Timer
// is always held behind a pointer inside a repository, so
// Process ticks the same object a Register caller holds, as
// TS ticks the same object it stored.
type Timer struct {
	key     TimerKey
	ticks   int
	defined bool
}

// NewTimer returns a timer for key with ticks remaining: the Go
// form of `new Timer(key, ticks)` (Timer.ts:8-11). The ticks
// are always defined; only RegisterTimerKey creates an
// undefined-ticks timer, from a no-default key.
func NewTimer(key TimerKey, ticks int) *Timer {
	return &Timer{key: key, ticks: ticks, defined: true}
}

// Key returns the timer's key. Timer.ts:17-19 (key()). A nil
// timer reports the zero key.
func (t *Timer) Key() TimerKey {
	if t == nil {
		return TimerKey{}
	}
	return t.key
}

// Ticks returns the ticks remaining, or 0 for an undefined
// timer. Timer.ts:13-15 (ticks()): TS returns undefined for
// an undefined timer; 0 is the int collapse (see the file
// header). A nil timer reports 0.
func (t *Timer) Ticks() int {
	if t == nil || !t.defined {
		return 0
	}
	return t.ticks
}

// Tick decrements the remaining ticks by one, only while they
// are above 0: the count floors at 0 and never goes negative
// from ticking. Timer.ts:21-25 (tick()). An undefined timer
// never changes. A nil timer is a no-op.
func (t *Timer) Tick() {
	if t == nil || !t.defined || t.ticks <= 0 {
		return
	}
	t.ticks--
}

// TimerRepository is one Mobile's keyed countdown store.
// TimerRepository.ts:6-82. The zero value is usable: the map
// stays nil until the first registration, mirroring the TS
// `Map | null` field (TimerRepository.ts:7), and methods that
// only read never allocate it. Use NewTimerRepository for a
// fresh repository; Mobiles allocate one lazily through
// GetTimers (see entity.go).
type TimerRepository struct {
	mu     sync.RWMutex
	timers map[TimerKey]*Timer
}

// NewTimerRepository returns an empty repository: the Go form
// of `new TimerRepository()` (Mobile.ts:73).
func NewTimerRepository() *TimerRepository {
	return &TimerRepository{}
}

// ensureMap allocates the map when it does not exist yet. The
// caller must hold the write lock.
func (r *TimerRepository) ensureMap() {
	if r.timers == nil {
		r.timers = make(map[TimerKey]*Timer)
	}
}

// Has reports whether key holds an active timer: present with
// ticks > 0. TimerRepository.ts:9-15 (has). An absent key, a
// 0-tick timer, and an undefined-ticks timer all answer false.
func (r *TimerRepository) Has(key TimerKey) bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.timers[key]
	return ok && t != nil && t.defined && t.ticks > 0
}

// Register stores timer under its own key, replacing any timer
// already held for that key. TimerRepository.ts:17-19
// (register). A nil timer is ignored: TS would throw
// dereferencing it, and the Go form stays silent so a bare
// fixture cannot panic the tick goroutine (see the file
// header).
func (r *TimerRepository) Register(timer *Timer) {
	if r == nil || timer == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMap()
	r.timers[timer.key] = timer
}

// RegisterTimerKey stores a fresh timer for key carrying the
// key's own default ticks. TimerRepository.ts:21-23
// (registerTimerKey): `new Timer(key, key.getTicks())`. For a
// key with no default the stored timer has undefined ticks:
// present but never active, never ticking (see the file
// header). It replaces any timer already held for the key.
func (r *TimerRepository) RegisterTimerKey(key TimerKey) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMap()
	if key.hasDefault {
		r.timers[key] = NewTimer(key, key.defaultTicks)
	} else {
		r.timers[key] = &Timer{key: key}
	}
}

// Left returns the ticks remaining for key, or 0 when no timer
// is held for it. TimerRepository.ts:26-29 (left). An
// undefined-ticks timer reports 0 (see the file header).
func (r *TimerRepository) Left(key TimerKey) int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if t, ok := r.timers[key]; ok && t != nil {
		return t.Ticks()
	}
	return 0
}

// WillEndIn reports whether key's timer will have ended within
// ticks: an absent key answers true (there is nothing to wait
// for), a held timer answers ticks-remaining <= ticks.
// TimerRepository.ts:31-37 (willEndIn). An undefined-ticks
// timer answers false, reproducing the TS comparison
// (undefined <= ticks is false).
func (r *TimerRepository) WillEndIn(key TimerKey, ticks int) bool {
	if r == nil {
		return true
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.timers[key]
	if !ok || t == nil {
		return true
	}
	if !t.defined {
		return false
	}
	return t.ticks <= ticks
}

// GetTicks returns the ticks remaining for key, or 0 when no
// timer is held for it. TimerRepository.ts:40-46 (getTicks).
// An undefined-ticks timer reports 0 (see the file header).
func (r *TimerRepository) GetTicks(key TimerKey) int {
	return r.Left(key)
}

// Registers stores a fresh timer for key with ticks remaining,
// replacing any timer already held for the key.
// TimerRepository.ts:48-50 (registers). Ticks are stored as
// given: TS applies no clamping, so a 0 or negative count
// stores a present-but-inactive timer.
func (r *TimerRepository) Registers(key TimerKey, ticks int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMap()
	r.timers[key] = NewTimer(key, ticks)
}

// ExtendOrRegister stores a fresh timer for key with ticks
// remaining, unless a timer already held for the key has at
// least that many ticks left, in which case the existing timer
// is kept. TimerRepository.ts:52-55 (extendOrRegister): the
// existing timer survives exactly when `existing.ticks() <
// ticks` is false — including an undefined-ticks timer, whose
// comparison is false, so it is kept.
func (r *TimerRepository) ExtendOrRegister(key TimerKey, ticks int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMap()
	if existing, ok := r.timers[key]; ok && existing != nil {
		if !existing.defined || existing.ticks >= ticks {
			return
		}
	}
	r.timers[key] = NewTimer(key, ticks)
}

// AddOrSet adds ticks to the timer held for key, or stores a
// fresh timer with ticks when none is held.
// TimerRepository.ts:56-59 (addOrSet): the stored timer is a
// fresh object carrying existing + ticks. Adding onto an
// undefined-ticks timer keeps the undefined entry: TS stores
// existing(undefined) + ticks = NaN, which compares false
// everywhere and is observably identical to undefined (see the
// file header).
func (r *TimerRepository) AddOrSet(key TimerKey, ticks int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ensureMap()
	if existing, ok := r.timers[key]; ok && existing != nil && existing.defined {
		r.timers[key] = NewTimer(key, existing.ticks+ticks)
		return
	}
	if _, ok := r.timers[key]; !ok {
		r.timers[key] = NewTimer(key, ticks)
	}
}

// Cancel removes key's timer, if any. TimerRepository.ts:61-63
// (cancel). It never allocates the map: cancelling from an
// untouched repository is a no-op, as `this.timer?.delete`
// is in TS.
func (r *TimerRepository) Cancel(key TimerKey) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.timers, key)
}

// Process ticks every held timer once. TimerRepository.ts:65-71
// (process). Entries stay in the map at 0 — ticking never
// removes them — so a timer that ran out still answers Has
// false while remaining present for Left/GetTicks/WillEndIn
// and for Timers.
func (r *TimerRepository) Process() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.timers {
		t.Tick()
	}
}

// HasActive reports whether any held timer is active (ticks >
// 0). TimerRepository.ts:73-77 (hasActive). An untouched
// repository answers false.
func (r *TimerRepository) HasActive() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, t := range r.timers {
		if t != nil && t.defined && t.ticks > 0 {
			return true
		}
	}
	return false
}

// Timers returns a snapshot of the held timers, keyed by key,
// as Timer values. TimerRepository.ts:79-81 (timers()).
//
// DEVIATION, DOCUMENTED: TS returns the live Map itself
// (allocating it on first call). Go returns a copy: the
// repository is mutex-guarded (see the file header), and
// handing out the live map would let callers mutate it
// without the lock. No TS code outside TimerRepository itself
// calls timers() at commit bfd81e3 (verified by grep), so no
// behavior depends on the returned map being live; the
// snapshot carries the same entries. Mutating a returned
// Timer value never affects the repository.
func (r *TimerRepository) Timers() map[TimerKey]Timer {
	out := make(map[TimerKey]Timer)
	if r == nil {
		return out
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	for key, t := range r.timers {
		if t != nil {
			out[key] = *t
		}
	}
	return out
}
