package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf16"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// Save wiring: the coordinator between live players and the character-save
// store. It ports the persistence wiring around PlayerSave.fromPlayer /
// applyToPlayer (the save-shape operations themselves live in
// internal/persist/apply.go, Worker A) and the Autosave plugin
// (plugins/persistence/Autosave.plugin.js), grounded in the TS sources
// cited at each site.
//
// SYNCHRONOUS WRITES (deliberate divergence): every save here writes
// synchronously on the tick goroutine (or, for shutdown writes, after the
// engine stops, from immutable snapshots). TS queues saves through an
// async persistence chain (the JSON backend's write queue, the SQLite
// backend's save worker) and requestSave coalescing; this port has no
// async writer, consistent with the existing JSONFileStore
// synchronous-write divergence (internal/persist/store.go). The write
// volume is bounded — autosave saves at most 10 players per tick,
// disconnect saves are one player per removal, shutdown/tick-error
// save-alls are rare — so a synchronous write cannot stall the tick loop
// the way an unbounded queue could. Do NOT build an async writer here;
// requestSave coalescing is explicitly out of scope (PHASE5_PLAN.md).
//
// TICK-GOROUTINE DISCIPLINE: snapshot, saveAll, disconnectSave, and
// runAutosave read live player state (Location, IsRunning) and must run
// on the tick goroutine (or, for the shutdown snapshot, after the engine
// has fully stopped with no tick in flight — which the engine cannot
// guarantee, hence the task-mediated snapshot in
// requestShutdownSnapshot). They never race the tick loop: the carried
// registry and the autosave due-map are mutex-guarded for the
// network-goroutine entry path (register/activate/schedule), but live
// player state is only ever read from the tick goroutine. Every save
// path recovers panics per player and logs, so a save failure can never
// crash the tick loop (TS World.savePlayers catches per player too,
// World.ts:217-224; the disconnect path additionally must not panic
// because drainRemovePlayers does not recover its disconnect hook — a
// panic there would abort the rest of the removal drain).

// maxSavesPerTick is MAX_SAVES_PER_TICK (Autosave.plugin.js:7): no more
// than 10 autosaves on any tick; over-budget players wait a tick.
const maxSavesPerTick = 10

// defaultAutosaveIntervalTicks / defaultAutosaveMinIntervalTicks are the
// Autosave.plugin.js defaults when AUTOSAVE_INTERVAL_TICKS is unset:
// INTERVAL_TICKS 1000, MIN_INTERVAL_TICKS 500 (Autosave.plugin.js:5-6).
const (
	defaultAutosaveIntervalTicks    = 1000
	defaultAutosaveMinIntervalTicks = 500
)

// shutdownSnapshotMargin is the slack added to the 3-tick shutdown
// snapshot wait (see shutdownWaitTimeout): scheduling jitter must not
// turn a healthy shutdown into a timeout.
const shutdownSnapshotMargin = 500 * time.Millisecond

// errNoCarriedSave reports that a player has no activated carried save:
// it never entered through the adapter (a bare world registration in
// tests), or its entry was already taken by a disconnect save. Callers
// treat it as "nothing to preserve", not a failure.
var errNoCarriedSave = errors.New("savewiring: no activated carried save for player")

// errBotPlayer reports that a player is a bot. Bots are never saved:
// see the skip-persistence note on saveCoordinator.
var errBotPlayer = errors.New("savewiring: bot players are never saved")

// carriedEntry is one player's carried save: the save object loaded at
// login (or built for a new character) that snapshots merge live state
// onto. It is created in AddPlayer but only ACTIVATED when the add-wait
// succeeds (the tick loop assigned an index). A login that never entered
// the world must never write a save — especially a new-character save at
// spawn, and especially must never overwrite a real save — so
// disconnect/shutdown/autosave paths save only activated entries, and
// the timeout path removes the entry without saving. The entry is
// removed when its disconnect save runs. See worldAdapter.AddPlayer and
// saveCoordinator.disconnectSave.
type carriedEntry struct {
	save      *persist.PlayerSave
	activated bool
}

// saveCoordinator owns the save store, the carried-save registry, the
// autosave due-map and tick counter, and the shutdown snapshot
// handshake. One coordinator exists per server, built in buildServer.
//
// SKIP-PERSISTENCE DEGENERATION: TS gates saves on the
// "bot-skip-persistence" player attribute (World.ts:27, 214;
// Player.ts:69, 761; Autosave.plugin.js:9), which only bots carry.
// Nothing in the Go port sets that attribute — bots are created with
// IsBot and no attribute — so IsBot is the proxy: bots are the only
// skip-persistence holders in the ported runtime, and every save
// path skips them. The disconnect hook itself is already gated on
// non-bot registered players by World.drainRemovePlayers, so the IsBot
// checks here are defense in depth for the save-all/autosave paths.
type saveCoordinator struct {
	store  persist.SaveStore
	world  *game.World
	logger *log.Logger

	tickRate         time.Duration
	intervalTicks    int
	minIntervalTicks int

	// mu guards carried, dueOrder, dueAt, and tick. The entry path
	// (register/activate/schedule) runs on network goroutines while
	// the save paths run on the tick goroutine; all registry/due-map
	// access goes through this mutex, in short critical sections that
	// never perform I/O. Live player state is NOT guarded here — it is
	// tick-goroutine-only by discipline (see the file header).
	mu      sync.Mutex
	carried map[*game.Player]*carriedEntry
	// dueOrder preserves insertion order for the autosave due-map: JS
	// Map iterates in insertion order (Autosave.plugin.js iterates
	// dueAt directly), and Go maps do not, so the order lives in this
	// slice while dueAt holds the due ticks. Re-scheduling an existing
	// player keeps its position (JS Map.set on an existing key does
	// not move it); forgetting splices it out.
	dueOrder []*game.Player
	dueAt    map[*game.Player]int64
	// tick is the autosave plugin's own counter (Autosave.plugin.js:
	// `let tick = 0`, incremented per task execution), NOT the world
	// process cycle. Scheduling and due checks use this counter only.
	tick int64

	// Shutdown snapshot handshake: Stop sets shutdownRequested and
	// waits on shutdownResult; the autosave task (tick goroutine)
	// delivers exactly one snapshot slice when it next runs. See
	// requestShutdownSnapshot.
	shutdownRequested atomic.Bool
	shutdownDelivered atomic.Bool
	shutdownResult    chan []*persist.PlayerSave
	// shutdownMu serializes requestShutdownSnapshot and caches its
	// result, so a repeated Stop reuses the first snapshot instead of
	// waiting out a second timeout against an already-stopped engine.
	shutdownMu        sync.Mutex
	shutdownComplete  bool
	shutdownSnapshots []*persist.PlayerSave
}

// newSaveCoordinator returns a coordinator over store/world. The
// autosave intervals come from the caller (buildServer reads the
// environment once via autosaveIntervalsFromEnv; tests inject values
// directly — the task itself never reads the environment). A nil logger
// means log.Default(); a non-positive tick rate means 600 ms.
func newSaveCoordinator(store persist.SaveStore, world *game.World, logger *log.Logger, tickRateMs, intervalTicks, minIntervalTicks int) *saveCoordinator {
	if logger == nil {
		logger = log.Default()
	}
	tickRate := time.Duration(tickRateMs) * time.Millisecond
	if tickRate <= 0 {
		tickRate = 600 * time.Millisecond
	}
	return &saveCoordinator{
		store:            store,
		world:            world,
		logger:           logger,
		tickRate:         tickRate,
		intervalTicks:    intervalTicks,
		minIntervalTicks: minIntervalTicks,
		carried:          map[*game.Player]*carriedEntry{},
		dueAt:            map[*game.Player]int64{},
		shutdownResult:   make(chan []*persist.PlayerSave, 1),
	}
}

// autosaveIntervalsFromEnv resolves INTERVAL_TICKS / MIN_INTERVAL_TICKS
// from AUTOSAVE_INTERVAL_TICKS exactly as Autosave.plugin.js:5-6 does.
// Read once, at coordinator construction time (see buildServer).
func autosaveIntervalsFromEnv() (interval, min int) {
	return autosaveIntervalsFromRaw(os.Getenv("AUTOSAVE_INTERVAL_TICKS"))
}

// autosaveIntervalsFromRaw is the pure form of autosaveIntervalsFromEnv,
// over the raw variable value ("" when unset — which behaves identically
// to an empty value: Number("") is 0 and "" is falsy, and
// Number(undefined) is NaN, also falsy for the > 0 test and the
// truthiness test alike).
//
//	INTERVAL_TICKS = Number(env) > 0 ? floor(Number(env)) : 1000
//	MIN_INTERVAL_TICKS = env ? INTERVAL_TICKS : 500
//
// Note the truthy-string quirk in the second line: ANY non-empty value —
// including "0" or "abc", which resolve INTERVAL_TICKS to the 1000
// default — makes MIN equal INTERVAL, collapsing the spread to a fixed
// interval. Only an unset/empty variable yields the 500–1000 spread.
// This is ported faithfully, quirk included.
func autosaveIntervalsFromRaw(raw string) (interval, min int) {
	interval = defaultAutosaveIntervalTicks
	if n := jsNumber(raw); n > 0 {
		if math.IsInf(n, 1) {
			// Math.floor(Infinity) is Infinity; no int holds it.
			// Clamp to the largest int32, far beyond any real
			// uptime in ticks. (A finite value above the int
			// range converts the same way via the floor below
			// only when it fits; guard the conversion too.)
			interval = math.MaxInt32
		} else {
			floored := math.Floor(n)
			if floored > float64(math.MaxInt32) {
				interval = math.MaxInt32
			} else {
				interval = int(floored)
			}
		}
	}
	if raw != "" {
		min = interval
	} else {
		min = defaultAutosaveMinIntervalTicks
	}
	return interval, min
}

// jsNumber parses s as JavaScript Number(s) (ECMAScript ToNumber on a
// string): surrounding whitespace is ignored, "" is 0, "Infinity" (with
// an optional sign) is infinite, 0x/0b/0o integer literals are accepted,
// and anything else must be a decimal numeric literal — trailing
// garbage, underscores, and bare "inf"/"nan" spellings are NaN. Only the
// cases an operator could plausibly set are exercised by tests; the
// whitespace set is Go's unicode space set plus U+FEFF (JS also trims
// U+FEFF), which covers every ASCII spelling exactly.
func jsNumber(s string) float64 {
	// Trim JS whitespace (WhiteSpace + LineTerminator, approximated
	// by Go's unicode space set) and U+FEFF, repeatedly: a BOM
	// adjacent to spaces must not shield them from trimming.
	trimmed := s
	for {
		next := strings.Trim(strings.TrimSpace(trimmed), "\ufeff")
		if next == trimmed {
			break
		}
		trimmed = next
	}
	if trimmed == "" {
		return 0
	}
	switch trimmed {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	rest := trimmed
	neg := false
	if strings.HasPrefix(rest, "+") || strings.HasPrefix(rest, "-") {
		neg = rest[0] == '-'
		rest = rest[1:]
	}
	if len(rest) >= 2 && rest[0] == '0' {
		var base int
		switch rest[1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 {
			digits := rest[2:]
			if digits == "" {
				return math.NaN()
			}
			n, err := strconv.ParseUint(digits, base, 64)
			if err != nil {
				if errors.Is(err, strconv.ErrRange) {
					if neg {
						return math.Inf(-1)
					}
					return math.Inf(1)
				}
				return math.NaN()
			}
			if neg {
				return -float64(n)
			}
			return float64(n)
		}
	}
	// Decimal: reject anything ParseFloat would accept that JS
	// Number would not (underscores, inf/infinity/nan spellings).
	if strings.ContainsAny(trimmed, "_") {
		return math.NaN()
	}
	for _, r := range trimmed {
		if (r >= '0' && r <= '9') || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-' {
			continue
		}
		return math.NaN()
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return math.NaN()
	}
	return f
}

// autosaveOffsetOf ports offsetOf (Autosave.plugin.js:18-22): the
// name hash, 0 to interval-1, the same for a name every time.
//
//	hash = (hash * 31 + charCodeAt) >>> 0, per character
//
// JS iterates the string with for...of (code points) and takes
// charCodeAt(0) of each: for an astral character that is the HIGH
// surrogate only — the low surrogate never enters the hash. This port
// matches that exactly: each rune contributes its first UTF-16 code
// unit (the unit itself for BMP runes, the high surrogate for astral
// ones). Real usernames are 1–12 protocol characters and never contain
// astral characters, so the distinction is unreachable in production;
// it is preserved so the function is exactly the TS function. An
// interval <= 0 yields 0 (TS would produce NaN; the span guard in
// intervalOf keeps this unreachable).
func autosaveOffsetOf(username string, interval int) int {
	if interval <= 0 {
		return 0
	}
	var hash uint32
	for _, r := range username {
		var code uint32
		if r > 0xFFFF {
			hi, _ := utf16.EncodeRune(r)
			code = uint32(hi)
		} else {
			code = uint32(r)
		}
		hash = hash*31 + code // uint32 arithmetic wraps, as >>> 0 does
	}
	return int(hash % uint32(interval))
}

// intervalOf ports intervalOf (Autosave.plugin.js:28-30): MIN plus the
// name offset over the span INTERVAL-MIN+1, so due ticks spread across
// the interval window. The span is clamped to >= 1 (the env parser can
// produce INTERVAL < MIN only through the degenerate floor-to-0 case,
// where span is already 1; the clamp keeps a hand-built coordinator
// from dividing by zero).
func (c *saveCoordinator) intervalOf(p *game.Player) int {
	span := c.intervalTicks - c.minIntervalTicks + 1
	if span < 1 {
		span = 1
	}
	return c.minIntervalTicks + autosaveOffsetOf(p.Username, span)
}

// registerPending records p's carried save before the add-wait. The
// entry is NOT activated: only a successful add activates it (see
// carriedEntry).
func (c *saveCoordinator) registerPending(p *game.Player, save *persist.PlayerSave) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.carried[p] = &carriedEntry{save: save}
}

// activate marks p's entry activated: the add-wait succeeded and the
// player is in the world, so its saves may now be written.
func (c *saveCoordinator) activate(p *game.Player) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.carried[p]; ok {
		e.activated = true
	}
}

// removeEntry drops p's carried entry without saving: the timeout path
// for a login that never entered.
func (c *saveCoordinator) removeEntry(p *game.Player) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.carried, p)
}

// takeEntry removes and returns p's carried entry for the disconnect
// save. Taking (rather than reading) guarantees the disconnect save runs
// at most once per entry even if the hook fired twice.
func (c *saveCoordinator) takeEntry(p *game.Player) (*persist.PlayerSave, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.carried[p]
	delete(c.carried, p)
	if !ok {
		return nil, false
	}
	return e.save, e.activated
}

// carriedSave returns p's carried save and activation state, if any.
func (c *saveCoordinator) carriedSave(p *game.Player) (*persist.PlayerSave, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.carried[p]
	if !ok {
		return nil, false
	}
	return e.save, e.activated
}

// carriedCount reports the number of registry entries (pending +
// activated). Tests use it to assert no registry leak.
func (c *saveCoordinator) carriedCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.carried)
}

// dueCount reports the number of autosave-scheduled players. Tests use
// it to assert forget-on-disconnect.
func (c *saveCoordinator) dueCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.dueAt)
}

// schedule ports the Autosave schedule handler (Autosave.plugin.js:24-26):
// dueAt.set(player, tick + intervalOf(player)), called on login (here:
// on successful entry, from AddPlayer). Re-scheduling keeps insertion
// position, as a JS Map.set on an existing key does.
func (c *saveCoordinator) schedule(p *game.Player) {
	if p == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.dueAt[p]; !ok {
		c.dueOrder = append(c.dueOrder, p)
	}
	c.dueAt[p] = c.tick + int64(c.intervalOf(p))
}

// forget ports the Autosave forget handler (Autosave.plugin.js:32-34):
// dueAt.delete(player), called on logout (here: from disconnectSave).
func (c *saveCoordinator) forget(p *game.Player) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.dueAt[p]; !ok {
		return
	}
	delete(c.dueAt, p)
	for i, q := range c.dueOrder {
		if q == p {
			c.dueOrder = append(c.dueOrder[:i], c.dueOrder[i+1:]...)
			break
		}
	}
}

// reschedule moves p's due tick without changing its insertion
// position. A player forgotten since the caller last looked is not
// resurrected: forgetting wins over a stale reschedule.
func (c *saveCoordinator) reschedule(p *game.Player, due int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.dueAt[p]; !ok {
		return
	}
	c.dueAt[p] = due
}

// snapshot writes one player's current live state over its carried save
// and stores it under reason. It MUST be called on the tick goroutine
// (or after the engine is fully stopped with no tick in flight): it
// reads p.Location(), p.IsRunning(), and p.GetRunEnergyUnits(), which
// are tick-goroutine-only.
// It never panics: panics from snapshot construction or the store are
// recovered into an error. Bots and players without an activated
// carried save yield errBotPlayer / errNoCarriedSave; callers treat
// those as "nothing to preserve", not failures.
func (c *saveCoordinator) snapshot(p *game.Player, reason string) error {
	if p.IsBot {
		return errBotPlayer
	}
	save, activated := c.carriedSave(p)
	if save == nil || !activated {
		return errNoCarriedSave
	}
	return c.snapshotSave(p, save, reason)
}

// snapshotSave is the snapshot core over an already-resolved carried
// save: build the WithOwned clone from live state and store it. The
// carried save itself is never mutated and never handed to the store:
// WithOwned deep-clones, so a store that retained its argument could
// never alias the registry entry. Panics are recovered into errors.
func (c *saveCoordinator) snapshotSave(p *game.Player, base *persist.PlayerSave, reason string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("savewiring: panic saving %s: %v", p.Username, r)
		}
	}()
	snap, err := c.buildSnapshot(p, base)
	if err != nil {
		return err
	}
	if err := c.store.Save(snap, reason); err != nil {
		return fmt.Errorf("savewiring: saving %s (%s): %w", p.Username, reason, err)
	}
	return nil
}

// buildSnapshot merges live owned state onto a carried save without
// writing anything: the WithOwned half of snapshotSave, shared with the
// shutdown path (which must snapshot on the tick goroutine but write
// after the engine stops). Tick-goroutine-only, like snapshotSave.
// Panics are recovered into errors.
//
// The owned writes are the Go form of fromPlayer
// (PlayerSave.ts:737-862) restricted to the owned subset: position,
// running, run energy, inventory/equipment copied with the
// trade-escrow fold (PlayerSave.ts:782-791: offered items fold
// back into the saved inventory), appearance sanitized,
// skills cloned, bank tabs 0-9 read via getValidItems (the
// search tab is skipped), and attributes rebuilt from the live
// persistent keys (liveAttributes, PlayerSave.ts:811-816). WithOwned
// copies everything again, so the reads here need no further
// defensive copying; Items, Look, Snapshot, and GetValidItems all
// return copies.
//
// A live attribute value that is not JSON-compatible (a func, a
// struct, a non-finite number — see persist.MarshalAttributeValue)
// fails the whole snapshot with an error, as TS cloneAttribute
// throwing fails fromPlayer and the save: the caller logs per player
// and skips the write (saveAll, disconnectSave, runAutosave, and
// snapshotAllForShutdown all treat a buildSnapshot error as a skipped
// save, never a crash), and no partial save is written.
func (c *saveCoordinator) buildSnapshot(p *game.Player, base *persist.PlayerSave) (snap *persist.PlayerSave, err error) {
	defer func() {
		if r := recover(); r != nil {
			snap = nil
			err = fmt.Errorf("savewiring: panic snapshotting %s: %v", p.Username, r)
		}
	}()
	loc := p.Location()
	level, maxLevel, xp := p.Skills().Snapshot()
	// Attributes rebuild from live state (fromPlayer,
	// PlayerSave.ts:811-816): a non-JSON-compatible live value fails
	// the snapshot — see the buildSnapshot doc comment.
	attributes, err := liveAttributes(p)
	if err != nil {
		return nil, err
	}
	// Tabs 0-9 only: the search tab (index 10) is never saved
	// (fromPlayer, PlayerSave.ts:844-849). Bank() materializes
	// missing tabs, so all 10 tabs are always written, matching
	// TS's getBank materialization.
	banks := make(persist.Banks, game.BankSearchTabIndex)
	for tab := 0; tab < game.BankSearchTabIndex; tab++ {
		bank := p.Bank(tab)
		if bank == nil {
			banks[tab] = nil
			continue
		}
		banks[tab] = gameItemsToPersist(bank.GetValidItems())
	}
	// Trade escrow (fromPlayer, PlayerSave.ts:782-791): items
	// the player currently offers in a trade fold back into
	// the saved inventory, so a save taken while a trade is
	// open (autosave, shutdown) captures them. The disconnect
	// path aborts the trade first, so the fold is a no-op
	// there. A fold failure (offered items overflow the
	// inventory) fails the snapshot and the save is skipped,
	// as TS's add() returning false skips the write.
	inventory := gameItemsToPersist(p.Inventory().Items())
	if offered := p.Trading().OfferedItems(); len(offered) > 0 {
		stackable := func(id int) bool {
			if src := p.ItemDefinitions(); src != nil {
				return src.IsStackable(id)
			}
			return false
		}
		folded, err := persist.FoldTradeEscrow(inventory, gameItemsToPersist(offered), stackable)
		if err != nil {
			return nil, err
		}
		inventory = folded
	}
	// Relations snapshot (fromPlayer, PlayerSave.ts:819-830):
	// the live lists as decimal strings, the listed-only friend
	// ranks, and the channel fields. The scalar pointers are
	// always set: WithOwned dereferences them immediately, and
	// a live value of "" or 0 overwrites the carried value (see
	// the OwnedState pointer note in persist/apply.go).
	rel := p.Relations().SnapshotSaveState()
	// Sailing snapshot (fromPlayer, PlayerSave.ts:810): the
	// live sailing state, normalized, as save JSON. A marshal
	// failure fails the snapshot, as a non-JSON-compatible
	// live attribute does (see snapshotSailingJSON in
	// sailingwiring.go).
	sailing, err := snapshotSailingJSON(p)
	if err != nil {
		return nil, err
	}
	snap = base.WithOwned(persist.OwnedState{
		Position:        persist.Location{X: loc.X, Y: loc.Y, Z: loc.Z},
		Running:         p.IsRunning(),
		RunEnergyUnits:  p.GetRunEnergyUnits(),
		SpecPercentage:  p.GetCombat().SpecialPercentage(),
		AutocastSpellID: liveAutocastSpellID(p),
		Inventory:       inventory,
		Equipment:       gameItemsToPersist(p.Equipment().Items()),
		Appearance:      p.Appearance().Look(),
		Skills: persist.Skills{
			Level:      level,
			MaxLevel:   maxLevel,
			Experience: xp,
		},
		Banks:                  banks,
		Attributes:             attributes,
		Friends:                formatRelationHashes(rel.Friends),
		Ignores:                formatRelationHashes(rel.Ignores),
		FriendRanks:            rel.FriendRanks,
		FriendsChatChannelName: &rel.FriendsChatChannelName,
		FriendsChatLastOwner:   &rel.FriendsChatLastOwner,
		FriendsChatEntryRank:   &rel.FriendsChatEntryRank,
		FriendsChatTalkRank:    &rel.FriendsChatTalkRank,
		FriendsChatKickRank:    &rel.FriendsChatKickRank,
		Sailing:                sailing,
		// FightType snapshot (fromPlayer, PlayerSave.ts:754):
		// the live fight type, encoded as save JSON. A nil
		// return from the encoder cannot happen (a marshal
		// of fixed fields), but a nil value would preserve
		// the carried value per the OwnedState contract.
		FightType: game.FightTypeSaveJSON(p.GetCombat().FightType()),
	})
	// Skull snapshot (fromPlayer, PlayerSave.ts:777-778): the
	// live skull timer (ticks) and type, written directly onto
	// the clone — skull is NOT an OwnedState field (persist
	// keeps no skull in OwnedState; the pointer-optional
	// pattern would add a second ownership story for two
	// scalar fields, and WithOwned's always-write fields
	// would break persist tests that build OwnedState
	// literals without a skull). A skulled player therefore
	// keeps timer + type across the next snapshot and relogin;
	// an unskulled player writes 0/white over a carried skull.
	snap.SkullTimer = p.SkullTimer()
	snap.SkullType = persist.SkullType{IconID: p.SkullType().IconID()}
	// Poison snapshot (fromPlayer, PlayerSave.ts:764-771): the
	// live affliction fields, written directly onto the clone
	// like the skull scalars above — poison is NOT an
	// OwnedState field, so WithOwned preserves the carried
	// values and a live affliction would never reach the save
	// without this. LastVenomDamage is written only while
	// venomed (PoisonSaveState sets it only then); the clock
	// snapshot only while poisoned or protected, and nil
	// otherwise so a cured player clears a carried clock.
	// PoisonImmunityTimer is the clock's whole seconds
	// remaining, rounded (the persist field is an int; TS
	// writes the float into the JSON number).
	poison := p.GetCombat().PoisonSaveState()
	snap.PoisonDamage = poison.Damage
	snap.Venomed = poison.Venomed
	snap.LastVenomDamage = poison.LastVenomDamage
	snap.PoisonImmunityTimer = int(math.Round(poison.PoisonImmunitySeconds))
	if poison.Clock != nil {
		snap.PoisonClock = poisonClockSaveToPersist(poison.Clock)
	} else {
		snap.PoisonClock = nil
	}
	return snap, nil
}

// poisonClockSaveToPersist converts a game clock snapshot to
// the persist shape field-for-field (the structs are
// identical; game never imports persist). Pointers are
// deep-copied so the save never aliases live state.
func poisonClockSaveToPersist(s *game.PoisonClockSave) *persist.PoisonClockState {
	if s == nil {
		return nil
	}
	out := &persist.PoisonClockState{
		Counter:         s.Counter,
		TicksUntilPulse: s.TicksUntilPulse,
	}
	if s.ProtectionTicksRemaining != nil {
		v := *s.ProtectionTicksRemaining
		out.ProtectionTicksRemaining = &v
	}
	if s.BriefPoisonSeconds != nil {
		v := *s.BriefPoisonSeconds
		out.BriefPoisonSeconds = &v
	}
	if s.BriefVenomSeconds != nil {
		v := *s.BriefVenomSeconds
		out.BriefVenomSeconds = &v
	}
	return out
}

// poisonClockSaveFromPersist converts a persist clock state
// to the game shape field-for-field. Pointers are deep-copied
// so live state never aliases the carried save.
func poisonClockSaveFromPersist(s *persist.PoisonClockState) *game.PoisonClockSave {
	if s == nil {
		return nil
	}
	out := &game.PoisonClockSave{
		Counter:         s.Counter,
		TicksUntilPulse: s.TicksUntilPulse,
	}
	if s.ProtectionTicksRemaining != nil {
		v := *s.ProtectionTicksRemaining
		out.ProtectionTicksRemaining = &v
	}
	if s.BriefPoisonSeconds != nil {
		v := *s.BriefPoisonSeconds
		out.BriefPoisonSeconds = &v
	}
	if s.BriefVenomSeconds != nil {
		v := *s.BriefVenomSeconds
		out.BriefVenomSeconds = &v
	}
	return out
}

// liveAutocastSpellID reads the live autocast spell id for the
// snapshot (fromPlayer, PlayerSave.ts:756): the combat's autocast
// spell id, or -1 when no spell is set. A spell without a SpellID
// reads -1 too, the TS optional-chaining case
// (getAutocastSpell()?.spellId?.() ?? -1). Phase 27: the pick
// lives in combat state (SetAutocast), so the snapshot must read
// it here — preserving the carried value would lose every pick
// on the next snapshot.
func liveAutocastSpellID(p *game.Player) int {
	if p == nil || p.GetCombat() == nil {
		return -1
	}
	spell := p.GetCombat().AutoCastSpell()
	if spell == nil {
		return -1
	}
	if withID, ok := spell.(interface{ SpellID() int }); ok {
		return withID.SpellID()
	}
	return -1
}

// liveAttributes rebuilds the save attribute map from live player
// state, the Go form of the attributes rebuild in fromPlayer
// (PlayerSave.ts:811-816): for each registered persistent key the
// player holds, the live value is cloned through canonical JSON
// (persist.MarshalAttributeValue, the cloneAttribute rule), and
// default-equal values are stripped (persist.StripDefaultAttributes,
// the JSON.stringify comparison). Session-only attributes — keys
// outside the persistent registry — never reach the map, and keys
// the carried save held that live state no longer holds are dropped
// by the WithOwned overwrite, exactly as the TS rebuild drops them.
// The map is ALWAYS non-nil, even when empty: TS fromPlayer always
// writes an attributes object, possibly {}.
//
// The first non-JSON-compatible live value aborts the rebuild with
// an error: TS cloneAttribute throws, fromPlayer fails, and the save
// fails. No partial map is returned.
func liveAttributes(p *game.Player) (map[string]json.RawMessage, error) {
	keys := persist.PersistentAttributeKeys()
	attrs := make(map[string]json.RawMessage, len(keys))
	for _, key := range keys {
		if !p.HasAttribute(key) {
			continue
		}
		raw, err := persist.MarshalAttributeValue(key, p.GetAttribute(key))
		if err != nil {
			return nil, err
		}
		attrs[key] = raw
	}
	return persist.StripDefaultAttributes(attrs), nil
}

// saveAll ports World.savePlayers (World.ts:207-230): iterate the
// world's CURRENT registered players in slot order, skip
// skip-persistence players (IsBot here; see the struct note), save each
// with per-player recovery, count saved/failed, and log the summary
// line when either count is nonzero. The cancelRequestedSave call in TS
// has no Go equivalent: requestSave coalescing is out of scope, so
// there is nothing pending to cancel. Tick-goroutine-only (or the
// engine-goroutine-serial tick-error callback; see buildServer).
// Players without an activated carried save (bare world registrations
// that bypassed the adapter) are skipped silently: there is no base
// save to preserve, and counting them as failures would blame the
// save path for an adapter bypass.
func (c *saveCoordinator) saveAll(reason string) (saved, failed int) {
	for _, p := range c.world.Players().All() {
		if p == nil {
			continue
		}
		err := c.snapshot(p, reason)
		switch {
		case err == nil:
			saved++
		case errors.Is(err, errNoCarriedSave) || errors.Is(err, errBotPlayer):
			// Skipped, not failed: see the doc comment.
		default:
			failed++
			c.logger.Printf("[world] Failed to save player %s: %v", p.Username, err)
		}
	}
	if saved > 0 || failed > 0 {
		c.logger.Printf("[world] savePlayers saved=%d failed=%d reason=%q", saved, failed, reason)
	}
	return saved, failed
}

// disconnectSave saves a leaving player with reason "logout" and drops
// its registry entry and autosave schedule. It runs on the tick
// goroutine from onPlayerDisconnect (drainRemovePlayers), so snapshot
// discipline is satisfied. A failed save is logged and the player
// still leaves; the entry is dropped regardless, so a failed
// disconnect save is never retried by a later path.
//
// REASON GROUNDING: TS saves on removal with reason "logout" from
// Player.onLogout (Player.ts:761-764: cancelRequestedSave + save(this,
// "logout") unless skip-persistence), reached via Player.onRemove
// (Player.ts:304-306) <- MobileList.removes (MobileList.ts:109-111) <-
// World.queue_remove_players (World.ts:1266-1267, World.players.remove).
// Every removal — logout-button press, socket close, same-name kick —
// funnels through onRemove, so the reason is "logout" for all of them;
// the disconnect `source` ("logout"/"removed") is the
// emitPlayerDisconnect event field (World.ts:1260-1264), not the save
// reason, and is ignored here. The Go seam is OnPlayerDisconnect,
// which fires from drainRemovePlayers just before removal, for the
// same non-bot registered players; saving there instead of in a
// removal hook reads identical state (the player has not moved since
// its last turn).
func (c *saveCoordinator) disconnectSave(p *game.Player) {
	c.forget(p)
	save, activated := c.takeEntry(p)
	if save == nil || !activated || p.IsBot {
		return
	}
	if err := c.snapshotSave(p, save, "logout"); err != nil {
		c.logger.Printf("[save] Failed to save player %s on disconnect: %v", p.Username, err)
	}
}

// autosaveTask returns the TaskManager task that ports the Autosave
// plugin's task (Autosave.plugin.js:60-68): delay 1 (every tick),
// named "autosave", executing saveDuePlayers. It must be scheduled
// before the engine starts (buildServer); TaskManager is otherwise
// tick-goroutine-only.
func (c *saveCoordinator) autosaveTask() *game.Task {
	task := game.NewTask(1, func(cycle int64) bool {
		_ = cycle // the plugin keeps its own tick counter; see runAutosave.
		c.runAutosave()
		return false // never done: run every tick until the world stops.
	})
	task.Name = "autosave"
	return task
}

// runAutosave ports saveDuePlayers (Autosave.plugin.js:37-55) exactly:
//
//	tick++; saved = 0
//	for [player, due] of dueAt (insertion order):
//	    if due > tick: continue
//	    if saved >= 10: dueAt.set(player, tick+1); continue
//	    dueAt.set(player, tick + intervalOf(player))
//	    if skip-persistence: continue   // rescheduled ABOVE, so skipped
//	                                    // players still advance their due tick
//	    saved++; try save(player, "autosave") catch log-and-continue
//
// Deviations forced by Go concurrency, all documented: the iteration
// runs over a snapshot of the insertion order (the due-map is shared
// with the network-goroutine entry path, so the live order cannot be
// iterated without holding the mutex across store writes); a player
// forgotten between the snapshot and its save yields errNoCarriedSave
// and is skipped silently instead of logging (TS, single-threaded,
// never observes a logout mid-iteration); and the shutdown snapshot
// check runs first (see deliverShutdownSnapshot) — when it delivers,
// the autosave work for that tick is skipped because shutdown is
// imminent and the snapshot already covers every activated player.
func (c *saveCoordinator) runAutosave() {
	c.mu.Lock()
	c.tick++
	tick := c.tick
	order := append([]*game.Player(nil), c.dueOrder...)
	c.mu.Unlock()

	if c.deliverShutdownSnapshot() {
		return
	}

	saved := 0
	for _, p := range order {
		c.mu.Lock()
		due, ok := c.dueAt[p]
		c.mu.Unlock()
		if !ok {
			continue // forgotten since the order snapshot: left the world.
		}
		if due > tick {
			continue
		}
		if saved >= maxSavesPerTick {
			c.reschedule(p, tick+1)
			continue
		}
		// Reschedule BEFORE the skip check, as TS does: skipped
		// players still advance their due tick.
		c.reschedule(p, tick+int64(c.intervalOf(p)))
		if p.IsBot {
			continue
		}
		saved++ // TS counts the attempt before the try: failures consume budget too.
		if err := c.snapshot(p, "autosave"); err != nil {
			if errors.Is(err, errNoCarriedSave) || errors.Is(err, errBotPlayer) {
				continue // left between the order snapshot and the save.
			}
			c.logger.Printf("[autosave] Failed to save %s: %v", p.Username, err)
		}
	}
}

// snapshotAllForShutdown builds immutable snapshots of every activated
// non-bot registered player, without writing anything. Tick-goroutine-
// only: it reads live player state. Per-player failures are logged and
// skipped; a failed snapshot never blocks the others.
func (c *saveCoordinator) snapshotAllForShutdown() []*persist.PlayerSave {
	var out []*persist.PlayerSave
	for _, p := range c.world.Players().All() {
		if p == nil || p.IsBot {
			continue
		}
		save, activated := c.carriedSave(p)
		if save == nil || !activated {
			continue
		}
		snap, err := c.buildSnapshot(p, save)
		if err != nil {
			c.logger.Printf("[shutdown] Failed to snapshot player %s: %v", p.Username, err)
			continue
		}
		out = append(out, snap)
	}
	return out
}

// deliverShutdownSnapshot snapshots all activated players into the
// shutdown channel when Stop has requested a snapshot. It runs on the
// tick goroutine from runAutosave (task_manager phase, the first phase
// of the tick). It delivers at most once; later ticks with the flag
// still set do nothing. It reports whether it delivered (in which case
// runAutosave skips the autosave work for that tick).
//
// LIMIT: the snapshot reflects positions at the START of the tick that
// delivers it (the task phase precedes the process_players phase, so
// movement stepping later in that same tick is not captured). At most
// one tick of movement can be lost this way; there is no later
// tick-goroutine seam a shutdown snapshot could use, and waiting for a
// further tick would only move the same one-tick window.
func (c *saveCoordinator) deliverShutdownSnapshot() bool {
	if !c.shutdownRequested.Load() {
		return false
	}
	if !c.shutdownDelivered.CompareAndSwap(false, true) {
		return false
	}
	c.shutdownResult <- c.snapshotAllForShutdown()
	return true
}

// shutdownWaitTimeout is how long Stop waits for the tick-goroutine
// snapshot: 3 ticks plus margin, from the configured tick rate.
func (c *saveCoordinator) shutdownWaitTimeout() time.Duration {
	return 3*c.tickRate + shutdownSnapshotMargin
}

// requestShutdownSnapshot obtains the shutdown snapshots, honoring the
// tick-goroutine discipline. When the engine was never started, no tick
// goroutine exists and no tick can be in flight, so the snapshot is
// taken directly by the caller (the caller must not be driving
// World.Process concurrently from another goroutine — the production
// shutdown path never does). Otherwise the autosave task is asked, via
// the shutdown-request flag, to snapshot on its next run, and this
// waits up to timeout for the delivery; on timeout it logs and returns
// nil — Stop proceeds WITHOUT a snapshot rather than ever reading live
// player state off the tick goroutine. The result is cached: a repeated
// Stop reuses it instead of waiting out another timeout. Serialized by
// shutdownMu: concurrent callers queue behind the first.
func (c *saveCoordinator) requestShutdownSnapshot(engineStarted bool, timeout time.Duration) []*persist.PlayerSave {
	c.shutdownMu.Lock()
	defer c.shutdownMu.Unlock()
	if c.shutdownComplete {
		return c.shutdownSnapshots
	}
	var snaps []*persist.PlayerSave
	if !engineStarted {
		snaps = c.snapshotAllForShutdown()
	} else {
		c.shutdownRequested.Store(true)
		select {
		case snaps = <-c.shutdownResult:
		case <-time.After(timeout):
			c.logger.Printf("[shutdown] timed out waiting for the tick-goroutine save snapshot; proceeding without saving online players")
		}
	}
	c.shutdownComplete = true
	c.shutdownSnapshots = snaps
	return snaps
}

// writeSnapshots stores immutable snapshots (built by
// snapshotAllForShutdown) under reason, after the engine has stopped.
// Writes after stop are safe: the snapshots share no mutable state with
// live players. Per-snapshot failures (including panics) are logged and
// counted, never fatal: a SIGINT shutdown cannot be cancelled the way
// TS cancels its own on save failure (Server.ts:204-211), so shutdown
// proceeds — see Server.Stop.
func (c *saveCoordinator) writeSnapshots(snaps []*persist.PlayerSave, reason string) (saved, failed int) {
	for _, snap := range snaps {
		if snap == nil {
			continue
		}
		err := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic: %v", r)
				}
			}()
			return c.store.Save(snap, reason)
		}()
		if err != nil {
			failed++
			c.logger.Printf("[shutdown] Failed to save player %s: %v", snap.DisplayName, err)
			continue
		}
		saved++
	}
	return saved, failed
}

// isConfiguredDeveloperUsername mirrors isConfiguredDeveloperUsername
// (NetworkBuilder.ts:335-340) and the bootstrap's copy of it
// (bootstrapAdmin, internal/session/bootstrap.go): DEV_USERNAME entries,
// trimmed, empty entries dropped, compared lowercased against the
// lowercased username. The config layer already trims and drops empties
// (config.Env.DevUsernames), but the defensive re-trim matches both TS
// and the bootstrap exactly, so a hand-built dev list behaves the same.
func isConfiguredDeveloperUsername(username string, devUsernames []string) bool {
	name := strings.ToLower(username)
	for _, dev := range devUsernames {
		if trimmed := strings.TrimSpace(dev); trimmed != "" && strings.ToLower(trimmed) == name {
			return true
		}
	}
	return false
}

// saveLogger returns the logger save-wiring messages use: the
// coordinator's when one is attached, else the default logger (an
// adapter built standalone in tests has no coordinator).
func (a *worldAdapter) saveLogger() *log.Logger {
	if a.saves != nil && a.saves.logger != nil {
		return a.saves.logger
	}
	return log.Default()
}

// Transient client-type attributes (ClientProtocol.ts:2658-2661):
// set from the handshake, never persisted. They are deliberately NOT
// registered with the persist registry: a mobile login must never
// overwrite the layout saved from a desktop session, and the native
// flag describes this connection only (NetworkBuilder.ts:1664-1667
// comment). resolveGameframeRoot reads the mobile fact from the
// handshake's clientType byte in the Go bootstrap, not from the
// attribute store, but the attributes are set here so live-player
// readers (content, later phases) see the same facts TS sets.
const (
	mobileClientAttribute = "mobile-client"
	nativeClientAttribute = "native-client"
)

// applyTransientClientAttributes sets the transient client-type
// attributes from the handshake facts, BEFORE the applyToPlayer
// equivalent runs (NetworkBuilder.ts:1664-1667): clientType 1 means a
// mobile client, and the HANDSHAKE_CAPABILITY_NATIVE_CLIENT bit means
// an official OSRS client behind the gateway. Only the matching facts
// set an attribute; the absent case sets nothing (TS sets the
// attribute only when the condition holds — there is no explicit
// false). Session-only: neither key is persistent.
func applyTransientClientAttributes(p *game.Player, ep *session.EnteringPlayer) {
	if ep.ClientType == 1 {
		p.SetAttribute(mobileClientAttribute, true)
	}
	if ep.Capabilities&protocol.HandshakeCapabilityNativeClient != 0 {
		p.SetAttribute(nativeClientAttribute, true)
	}
}

// applySaveAttributes loads the carried save's persistent attributes
// onto the live player, then fills missing defaults: the attribute
// half of applyToPlayer (PlayerSave.ts:693-697). The save MUST have
// gone through MigrateForEntry first (prepareCarriedSave does it),
// so keys are in their migrated form and legacy fields are folded.
//
// TS iterates the persistent Set in insertion order; Go iterates the
// sorted key list instead. The order is unobservable here: each
// iteration decodes one value and sets one independent map entry —
// no key's value reads another key — and defaults fill only keys
// still absent afterwards. The sorted order is deterministic, which
// the TS insertion order (registration-order-dependent across
// plugins) is not.
//
// A value that fails to decode is logged and SKIPPED; it must not
// fail the login. This is a Go-necessity deviation: TS saves are
// already parsed JSON, so a corrupt attribute value cannot occur
// there. The Go stores only yield saves whose JSON parsed as a whole
// (JSONFileStore.Load / SQLite Load unmarshal the file), but a value
// can still be undecodable to `any` — a number too large for float64
// (e.g. 1e999) parses fine as json.RawMessage yet fails decoding —
// and a hand-built save can hold arbitrary bytes. Skipping the key
// (which then falls back to its default, if any) is the safe shape.
// A JSON null decodes to nil, which SetAttribute treats as a delete,
// so a null-valued key likewise falls back to its default: the
// nil-deletes design of the attribute store, not a special case here.
func (a *worldAdapter) applySaveAttributes(p *game.Player, save *persist.PlayerSave) {
	for _, key := range persist.PersistentAttributeKeys() {
		raw, ok := save.Attributes[key]
		if !ok {
			continue
		}
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			a.saveLogger().Printf("[login] dropping unreadable attribute %q for %s: %v", key, p.Username, err)
			continue
		}
		p.SetAttribute(key, value)
	}
	persist.ApplyAttributeDefaults(p.HasAttribute, p.SetAttribute)
}

// prepareCarriedSave applies the entering save to the new player and
// returns the carried save the registry will hold. It runs on the
// connection goroutine in AddPlayer, before the player is queued, so
// mutating the player and the save here races no tick. It ports the
// enterWorld order (NetworkBuilder.ts:1653-1676): construct at spawn,
// transient client attributes, applyToPlayer (or attribute defaults),
// dev-rights override, password hash, token fold for the primary,
// setLastKnownRegion.
//
//   - Transient attributes first: the mobile/native facts from the
//     handshake are set on the live player before either branch
//     (NetworkBuilder.ts:1664-1667), so they are present whether or
//     not a save exists. They are session-only and never touch the
//     save.
//   - With a save: MigrateForEntry (the applyToPlayer save-side
//     mutation: sanitize + attribute-key migration + legacy fold)
//     runs in place BEFORE the bootstrap reads this same object —
//     enterWorldLocked calls AddPlayer, then sendBootstrapLocked — so
//     the bootstrap sees sanitized appearance and migrated keys, as
//     TS does. The item-definition source is installed on the
//     player first (bank loading stacks through it), then
//     position/running are applied to the live player, the live
//     systems (run energy, containers, skills, appearance) are
//     loaded from the save by applyEntryLiveState, and the save's
//     persistent attributes are loaded onto the live player by
//     applySaveAttributes (missing defaults filled), and
//     LastKnownRegion is set to the applied location. The
//     audio-settings snapshot is applied after both branches (see
//     below). A nil Position yields the zero Location
//     (OwnedState, pinned by persist); stores normalize loaded saves
//     so a position is always present, and no spawn fallback is
//     added. NOTE on dev rights: TS also overwrites the LIVE player's
//     rights with DEVELOPER for dev usernames after apply
//     (NetworkBuilder.ts:1670). The Go Player holds no rights field
//     and rights are PRESERVED in the carried save (PHASE5_PLAN.md
//     §1), so a loaded save keeps its rights; the bootstrap's admin
//     flag already reflects the dev list. Only new-character saves
//     take the DEVELOPER grant, at construction below.
//   - Without a save: a new-character save is built (NewCharacterSave)
//     with the world spawn, the pending password hash, the entering
//     profile ("" means main, per persist.Profile/EffectiveProfile),
//     and DEVELOPER rights iff the username is a configured developer.
//     The player starts at spawn with defaults (NewPlayer already
//     does this); LastKnownRegion is set to the spawn explicitly, as
//     TS sets it after apply. The live systems are loaded from the
//     built save by applyEntryLiveState, as in the loaded-save
//     branch (max units, Agility 1, empty containers, default
//     skills/appearance) — explicit application in both branches,
//     rather than relying on NewPlayer defaults. Attribute defaults
//     are applied directly (NetworkBuilder.ts:1669 else-branch:
//     PlayerSave.applyAttributeDefaults), since there is no save to
//     load attributes from.
//   - Token fold, both branches: remembered logins live on the
//     primary (NetworkBuilder.ts:1672-1674), so the pending tokens
//     fold into the carried save iff Accounts.key(CredentialName) ==
//     Accounts.key(Username) (persist.Key). A new account's issued
//     token is carried on the pending login precisely because no save
//     existed to hold it, so folding it into the new-character save is
//     what keeps the client logged in across the first snapshot. A
//     SetTokens error is logged and ignored: TokenEntry marshals
//     infallibly, so it cannot realistically fail, and a login must
//     not fail over a remembered-login write. The fold ALSO sets the
//     live login:tokens attribute, AFTER the save-attribute load
//     (NetworkBuilder.ts:1672-1674 order), so the pending (rotated or
//     newly issued) entries — not the stale save entries — are what
//     the next snapshot rebuilds from live state. The live value is
//     the decoded form of the JSON SetTokens just wrote, not the
//     []TokenEntry itself: a slice of structs would fail the
//     cloneAttribute rule at snapshot time, while the decoded generic
//     form round-trips through snapshot marshaling identically.
func (a *worldAdapter) prepareCarriedSave(p *game.Player, ep *session.EnteringPlayer) *persist.PlayerSave {
	// The definition source goes on first, in both branches: bank
	// loading stacks through it (see applyEntryLiveState), and
	// every live system must see the same source. A standalone
	// adapter (tests) has a nil source, which the game layer
	// tolerates with documented defaults.
	p.SetItemDefinitions(a.itemDefinitions)
	// Transient client attributes precede the applyToPlayer
	// equivalent, in both branches (NetworkBuilder.ts:1664-1667).
	applyTransientClientAttributes(p, ep)
	var carried *persist.PlayerSave
	if ep.Save != nil {
		ep.Save.MigrateForEntry()
		owned := ep.Save.OwnedState()
		loc := game.Location{X: owned.Position.X, Y: owned.Position.Y, Z: owned.Position.Z}
		p.SetLocation(loc)
		p.SetRunning(owned.Running)
		p.SetLastKnownRegion(loc)
		a.applyEntryLiveState(p, ep.Save)
		// Save attributes onto the live player, defaults filled
		// (applyToPlayer, PlayerSave.ts:693-697). This reads
		// save.Attributes only; the appearance reads in
		// applyEntryLiveState above saw the same migrated map.
		a.applySaveAttributes(p, ep.Save)
		if ep.PasswordHash != "" {
			ep.Save.PasswordHashWithSalt = ep.PasswordHash
		}
		carried = ep.Save
	} else {
		carried = persist.NewCharacterSave(persist.NewCharacterParams{
			Username:     ep.Username,
			PasswordHash: ep.PasswordHash,
			Profile:      persist.Profile(ep.Profile),
			Spawn:        persist.Location{X: a.spawn.X, Y: a.spawn.Y, Z: a.spawn.Z},
			Developer:    isConfiguredDeveloperUsername(ep.Username, a.devUsernames),
			Now:          time.Now(),
		})
		p.SetLastKnownRegion(a.spawn)
		a.applyEntryLiveState(p, carried)
		// No save: defaults only (NetworkBuilder.ts:1669).
		persist.ApplyAttributeDefaults(p.HasAttribute, p.SetAttribute)
	}
	// Relations entry load, in both branches (see
	// applyEntryRelations in socialwiring.go): the long
	// username + the save's friend/ignore lists, friend ranks,
	// and friends-chat channel fields. The new-character save
	// carries empty lists and the default ranks (-1/-1/2), so
	// the same call loads fresh defaults.
	applyEntryRelations(p, carried)
	// Sailing entry load, in both branches (see
	// applyEntrySailing in sailingwiring.go): the save's
	// sailing state, normalized. The new-character save
	// carries the empty state, so the same call loads fresh
	// defaults.
	applyEntrySailing(p, carried)
	// Spellbook entry load, in both branches (see
	// applyEntrySpellbook in spellwiring.go): the save's
	// spellbook interface id becomes the live session-only
	// book. The new-character save carries NORMAL (1151), so
	// the same call loads fresh defaults.
	applyEntrySpellbook(p, carried)
	// Fight-type entry load, in both branches (see
	// applyEntryFightType in combatstylewiring.go): the save's
	// fightType becomes the live fight type when it resolves
	// against the equipped weapon's offered styles. The
	// new-character save carries UNARMED_PUNCH, so the same
	// call loads fresh defaults.
	applyEntryFightType(p, carried)
	if len(ep.LoginTokens) > 0 && persist.Key(ep.CredentialName) == persist.Key(ep.Username) {
		if err := carried.SetTokens(ep.LoginTokens); err != nil {
			a.saveLogger().Printf("[login] failed to fold login tokens for %s: %v", ep.Username, err)
		} else {
			// Live attribute, AFTER the save-attribute load
			// (NetworkBuilder.ts:1672-1674): the pending
			// entries overwrite the stale save entries live,
			// so snapshots rebuild from the pending list.
			// The value is the decoded form of the JSON just
			// written (see the prepareCarriedSave doc comment);
			// decoding it cannot fail, and a failure here
			// leaves the entry-loaded value live rather than
			// failing the login.
			if raw, ok := carried.Attributes[persist.LoginTokensAttribute]; ok {
				var value any
				if err := json.Unmarshal(raw, &value); err != nil {
					a.saveLogger().Printf("[login] failed to decode folded login tokens for %s: %v", ep.Username, err)
				} else {
					p.SetAttribute(persist.LoginTokensAttribute, value)
				}
			}
		}
	}
	// Audio settings, both branches: the entry-time snapshot from
	// the carried save (setAudioSettings, PlayerSave.ts:650),
	// read by the music block. A new-character save carries the
	// defaults; a loaded save carries its stored settings, or
	// the defaults when none were stored (the setter resets to
	// defaults first). The settings stay PRESERVED in the save
	// merge: nothing in Go mutates them after entry.
	p.SetAudioSettings(carried.AudioSettings)
	return carried
}

// applyEntryLiveState loads the live player systems from the
// carried save, at entry, before the player is queued (so it races
// no tick). It ports the applyToPlayer live-state application
// (PlayerSave.ts:672-680, 726-735) for the owned fields:
//
//   - Energy units from the save percent, through OwnedState (the
//     setRunEnergy rule: floor(percent*100 + 1e-6), clamped —
//     PlayerSave.ts:654 calls player.setRunEnergy(this.runEnergy)).
//   - Inventory/equipment via setItems (PlayerSave.ts:672-676),
//     meta carried byte-for-byte through NewItemWithMeta.
//   - Skills via setSkills (PlayerSave.ts:677-680), with the TS
//     normalization (see SkillManager.SetSkills).
//   - Appearance via the sanitized look (PlayerSave.ts:680), plus
//     the pronouns and skin-unlock attributes, which TS reads
//     through the attribute system at Appearance construction /
//     entry (Appearance.ts): pronouns only when the attribute is
//     an integer 0..2 (otherwise the gender fallback stands), skin
//     unlocks with the strict `=== true` read.
//   - Banks via add (PlayerSave.ts:726-735), for the tabs present
//     in the save map, tabs 0-9 only (the search tab is never
//     saved). TS loads inside Bank.withoutLimit; Go has no
//     withoutLimit, so a save holding more than the player's
//     space loads only what fits — a documented gap (see
//     bank.go). The definition source must already be installed
//     (prepareCarriedSave does it first): bank adds stack
//     through it.
//   - Skull timer/type via SetSkullTimer/SetSkullType
//     (PlayerSave.ts:667-668); the icon override stays null at
//     login (session-only, never persisted).
//
// Container refreshes are suppressed during load (SetItems does
// not refresh; bank adds pass refresh=false): no hook is
// installed yet — installContainerRefresh runs after this, from
// AddPlayer — and the bootstrap sends the initial snapshots.
func (a *worldAdapter) applyEntryLiveState(p *game.Player, save *persist.PlayerSave) {
	owned := save.OwnedState()
	p.SetRunEnergyUnits(owned.RunEnergyUnits)
	// Special-attack energy restores from the save
	// (applyToPlayer calls player.setSpecialPercentage,
	// PlayerSave.ts:655); the restore task restarts from the
	// first Process tick (see EnsureSpecialRestore), since
	// the restore timer is not saved.
	p.GetCombat().SetSpecialPercentage(owned.SpecPercentage)
	// Poison state restores from the save (applyToPlayer,
	// PlayerSave.ts:656-660: setPoisonDamage / setVenomed /
	// setLastVenomDamage, then the clock restore with the
	// legacy immunity seconds). The effect resumes on the
	// next tick when the restore leaves an affliction (the
	// per-tick resume re-check in processPoisonEffect);
	// entry-time resume would no-op before registration.
	p.GetCombat().RestorePoisonFromSave(save.PoisonDamage, save.Venomed, save.LastVenomDamage, poisonClockSaveFromPersist(save.PoisonClock), float64(save.PoisonImmunityTimer))
	// Skull state restores from the save (applyToPlayer,
	// PlayerSave.ts:667-668: setSkullTimer / setSkullType).
	// Icon 1 is RED, anything else WHITE (the save hydrator
	// defaults unknown icons to 0); the icon override is
	// session-only and stays null at login, exactly as TS
	// constructs a fresh Player.
	p.SetSkullTimer(save.SkullTimer)
	p.SetSkullType(game.SkullTypeFromIconID(save.SkullType.IconID))
	p.Inventory().SetItems(persistItemsToGame(save.Inventory))
	p.Equipment().SetItems(persistItemsToGame(save.Equipment))
	p.Skills().SetSkills(save.Skills.Level, save.Skills.MaxLevel, save.Skills.Experience)
	// SanitizeAppearance always returns 13 values, so SetLookArray
	// cannot fail here; the error is unreachable.
	_ = p.Appearance().SetLookArray(persist.SanitizeAppearance(save.Appearance))
	if v, ok := attributeInt(save.Attributes, "appearance:pronouns"); ok && v >= 0 && v <= 2 {
		p.Appearance().SetPronouns(v)
	}
	for _, entry := range game.SkinUnlocks {
		p.Appearance().SetSkinUnlock(entry.Attribute, attributeBool(save.Attributes, entry.Attribute))
	}
	for tab, items := range save.Banks {
		if tab < 0 || tab >= game.BankSearchTabIndex {
			continue
		}
		bank := game.NewBank(p)
		p.SetBank(tab, bank)
		bank.AddItems(persistItemsToGame(items), false)
	}
}

// persistItemsToGame converts saved items to live items, carrying
// meta byte-for-byte (the map[string]json.RawMessage shape is
// shared between persist.Item and game.Item; NewItemWithMeta
// deep-copies it). Empty slots (id < 0) convert to empty items;
// SetItems pads/truncates to the container capacity.
func persistItemsToGame(items []persist.Item) []game.Item {
	out := make([]game.Item, len(items))
	for i, item := range items {
		out[i] = game.NewItemWithMeta(item.ID, item.Amount, item.Meta)
	}
	return out
}

// gameItemsToPersist converts live items to saved items, carrying
// meta byte-for-byte. Item.Meta returns a copy, so the snapshot
// shares no mutable state with the live container.
func gameItemsToPersist(items []game.Item) []persist.Item {
	out := make([]persist.Item, len(items))
	for i, item := range items {
		out[i] = persist.Item{ID: item.ID(), Amount: item.Amount(), Meta: item.Meta()}
	}
	return out
}

// attributeInt reads an integer-valued attribute from a save's
// attribute map, mirroring the session package's attributeInt
// (bootstrap.go): the raw JSON must unmarshal to a float64 that is
// integral, and the result must fit in an int. Anything else —
// missing, non-numeric, fractional, or out of range — reports
// false. The duplication is deliberate: the session helper is
// unexported and the entry path cannot import it.
func attributeInt(attrs map[string]json.RawMessage, key string) (int, bool) {
	raw, ok := attrs[key]
	if !ok {
		return 0, false
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err != nil || f != math.Trunc(f) || f > math.MaxInt || f < math.MinInt {
		return 0, false
	}
	return int(f), true
}

// attributeBool reads a boolean-valued attribute with the strict
// `=== true` semantics TS applies to the skin-unlock attributes
// (Appearance.ts): only the JSON literal `true` counts. Missing,
// non-boolean, or false values report false.
func attributeBool(attrs map[string]json.RawMessage, key string) bool {
	raw, ok := attrs[key]
	if !ok {
		return false
	}
	var b bool
	if err := json.Unmarshal(raw, &b); err != nil {
		return false
	}
	return b
}
