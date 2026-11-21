package game

import (
	"fmt"
	"log"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// WorldHooks fills the World.Process phases that belong to systems not
// yet ported, so later milestones can implement a phase without editing
// Process. A nil hook means "phase does nothing" for ground items, boats,
// and combat hits — the honest foundation state. The visibility hooks
// (VisibilityBuckets, UpdatePlayersNPCs) and WalkToSweep differ: a nil
// hook there runs the real implementation, and a set hook overrides it.
//
// Hook failure policy: a hook that panics is recovered by the phase
// runner and reported via OnPhaseError; the tick continues with the next
// phase (see World.Process).
type WorldHooks struct {
	// GroundItems runs the ground-item phase (despawn/respawn).
	// ItemOnGroundManager.process — NOT STARTED.
	GroundItems func(cycle int64)

	// Shops runs the shop restock pass (ShopManager.Tick, the
	// ShopRestockTask pass: restock/destock toward the
	// definition amounts while the task flag is set). TS runs
	// the restock task through the TaskManager, so this phase
	// sits right after task_manager. A nil hook runs no
	// restock: shops hold their stock until the coordinator
	// wires the manager (phase 17).
	Shops func(cycle int64)

	// ShopProcessPlayer runs ShopManager.processPlayer for one
	// player: close the shop of a player whose interface no
	// longer shows it (World.ts process_players, after
	// player.process() and before emitPlayerProcess). It runs
	// for every processed player, bots included — the manager
	// itself skips unwatched bots. A nil hook closes nothing:
	// shops stay open until the player departs or closes one
	// explicitly. With no interface-state seam the manager
	// cannot observe a close, so the hook is a faithful no-op
	// until interface state lands (milestone 8).
	ShopProcessPlayer func(p *Player)

	// NPCActivation runs the NPC-spawn activation sweep for every player
	// plus the real-player observer bucket rebuild (the two untimed
	// statements between queue_remove_npcs and process_npcs in
	// World.ts process()). Region/spawn systems — NOT STARTED.
	NPCActivation func(cycle int64)

	// Boats runs BoatManager.tick + region refresh under boats (after
	// passengers move, before sync). Installed by InstallSailing
	// (see sailingwiring.go); nil when sailing is not installed.
	Boats func(cycle int64)

	// CombatHits runs the HitQueue sweep for entities that did not take
	// a turn. Combat — NOT STARTED.
	CombatHits func(cycle int64)

	// VisibilityBuckets overrides the update-bucket rebuild phase.
	// When nil, the phase runs World.RebuildUpdateBuckets (the real
	// implementation); when set, the hook wins and the buckets are
	// left to it. Visibility is ported (see visibility.go); the hook
	// remains as an override point.
	VisibilityBuckets func(cycle int64)

	// UpdatePlayersNPCs overrides the per-player local player/NPC
	// sync-state phase (updateLocalPlayers / updateLocalNpcs). When
	// nil, the phase runs the real implementation; when set, the hook
	// wins. Actual byte encoding happens in the session layer's
	// FlushTick.
	UpdatePlayersNPCs func(cycle int64)

	// WalkToSweep overrides the walk-to sweep phase. When nil, the
	// phase runs Tasks().ProcessRemainingWalkTo(cycle), as in TS.
	WalkToSweep func(cycle int64)

	// OnPhaseError reports a recovered panic (or a queue failure such as
	// a failed NPC add) from a named phase. When nil, the error is
	// written with log.Printf. It must not panic.
	OnPhaseError func(phase string, err error)

	// OnPlayerError reports a per-player failure in the process, flush,
	// or reset phases (the player is also queued for removal, matching
	// the TS catch → requestLogout behavior). When nil, the error is
	// reported via OnPhaseError. It must not panic.
	OnPlayerError func(p *Player, phase string, err error)

	// OnNPCError reports a per-NPC failure in the process or reset
	// phases (TS logs and continues). When nil, the error is reported
	// via OnPhaseError. It must not panic.
	OnNPCError func(n *NPC, phase string, err error)

	// OnPlayerDisconnect runs when a queued player leaves the world,
	// with the disconnect source ("logout", "removed", ...). In TS this
	// is PluginManager.emitPlayerDisconnect, fired only for non-bot
	// registered players; the same gate applies here. Persistence and
	// plugin notification are the coordinator's job behind this hook.
	OnPlayerDisconnect func(p *Player, source string)

	// LogoutRefusal answers a queued logout request: a non-empty return
	// refuses the logout with that message (TS: PluginManager
	// .emitCanLogout ?? player.logoutRefusal()). When nil, all logout
	// requests are accepted. Note the combat-based refusal in TS
	// (logoutRefusal/inLogoutCombat) needs combat state that is NOT
	// STARTED; the coordinator supplies it through this hook.
	LogoutRefusal func(p *Player) string

	// OnLogoutRefused runs when a logout request is refused. The
	// session layer implements the client-facing refusal behind this
	// hook (TS: session.refuseLogout); SessionPort has no such method
	// on purpose — refusal delivery is a session concern.
	OnLogoutRefused func(p *Player, reason string)

	// OnLogoutAccepted runs when a logout request is accepted and the
	// player is queued for removal. The session layer sends the logout
	// response behind this hook (TS: session.logout()).
	OnLogoutAccepted func(p *Player)

	// PlayerProcess runs once per non-bot registered player per tick,
	// after Player.Process (TS: PluginManager.emitPlayerProcess,
	// World.ts:1372, after player.process(); ShopManager.processPlayer
	// does not exist in Go). The plugin hub is wired behind this hook
	// by the coordinator. A panic in the hook is recovered by
	// processPlayerGuarded like a process panic; the hub isolates
	// per-hook panics itself, so a throwing content hook never
	// reaches the recover.
	PlayerProcess func(p *Player, cycle int64)
}

// World is the Go form of the static World class in game/World.ts: the
// player/NPC lists, add/remove queues, logout requests, the process
// cycle counter, and the per-tick phase skeleton.
//
// Unlike TS, World is a struct with an explicit constructor so tests can
// own isolated instances. Process must be called from exactly one
// goroutine (the tick goroutine), as in TS. The Queue* methods,
// IsFull, and PlayerByName are safe to call from any goroutine: the
// queues are guarded by queueMu (drains swap the queue contents out
// under the lock and process them without holding it) and the player
// and NPC lists are internally locked (see MobileList). Player
// .QueueTurnPacket is likewise mutex-guarded because the network
// goroutine enqueues directly. Everything else on World (Tasks,
// Hooks, ProcessCycle reads during a tick) is tick-goroutine-only.
//
// NOT STARTED in World: ground items (behind Hooks.GroundItems),
// active-region index, tile/npc occupant tracking, boats, shops,
// and persistence (savePlayers). Game objects ARE ported: the
// ObjectRegistry (see objects.go) holds the MapObjects tile/owner
// maps and the ObjectManager world indexes, reachable through
// GameObjects(). Update buckets and visibility/local lists ARE
// ported (see visibility.go); region gating of the NPC set they
// cover is NOT STARTED,
// so the bucketed NPC set is every registered NPC. Save-all-on-tick-error is
// the coordinator's job behind the Engine's OnTickError hook.
type World struct {
	spawn   Location
	players *MobileList[*Player]
	npcs    *MobileList[*NPC]
	tasks   *TaskManager

	// regions is the collision/region manager: the Go form of the
	// static RegionManager (game/collision/RegionManager.ts). It
	// defaults to an empty open-ground manager (nil sources) and the
	// coordinator replaces it with a cache-backed one at boot when a
	// cache is present. Set it only before the engine starts (see
	// SetRegionManager); after that it is read-only, and AddPlayer
	// wiring reads it from network goroutines.
	regions *RegionManager

	// objects is the game-object registry: the Go form of the
	// static MapObjects tile/owner maps plus the ObjectManager
	// world list/indexes (see objects.go). It is constructed by
	// NewWorld and follows the region manager through
	// SetRegionManager. Tick-goroutine-only for load-triggering
	// lookups (see ObjectRegistry); the registry guards its own
	// maps for concurrent Register/Deregister calls.
	objects *ObjectRegistry

	// areas is the world's area registry: the Go form of the
	// static AreaManager.areas list plus its per-actor hints and
	// search index (see areamanager.go). It is constructed by
	// NewWorld; the coordinator installs the world's Wilderness
	// on it (SetWilderness) before the engine starts, and the
	// hub's registerArea pushes content areas onto it. Players
	// and NPCs read it through the areaManager field World
	// installs on add. Tick-goroutine-only (registration at
	// boot/plugin load happens before ticking starts).
	areas *AreaManager

	// boats is the world's boat manager: the Go form of the
	// static BoatManager (BoatManager.ts). It is installed by
	// the coordinator (SetBoatManager) before the engine
	// starts and is nil in worlds without sailing; visibility
	// and movement degenerate to the no-boats behavior then.
	// Tick-goroutine-only after boot. It deliberately lives
	// here, not in a process-wide global: the engine tolerates
	// a tick in flight past Stop (see Engine.Stop), and a
	// re-published global would let that tick read another
	// world's boats.
	boats *BoatManager

	// equipmentEffects is the world's run-energy modifier
	// registry: the Go form of the runEnergyRestoreModifiers /
	// runEnergyDrainModifiers module arrays
	// (EquipmentEffects.ts:61-62). It is constructed by
	// NewWorld; the hub's registerRunEnergy* methods push
	// content modifiers onto it (through the coordinator's
	// adapter, see cmd/server/equipmentwiring.go). Players
	// read it through the equipmentEffects field World
	// installs on add. It deliberately lives here, not in a
	// process-wide global, for the same reason boats does.
	// The registry guards its own slices (see
	// equipment_effects.go); registration at boot happens
	// before ticking starts, and late registration is safe.
	equipmentEffects *EquipmentEffects

	// worldEntities is the world's per-viewer boat sync state
	// (see world_entity_sync.go). It is constructed by
	// NewWorld and follows the boat manager through
	// SetBoatManager. Tick-goroutine-only, like the
	// visibility buckets.
	worldEntities *WorldEntitySync

	// cycle is the process cycle counter. Only Process writes it,
	// but ProcessCycle reads it from other goroutines (operators,
	// tests), so it is atomic.
	cycle atomic.Int64

	// queueMu guards the five queues below. Queue* methods lock it
	// briefly to append; phase drains swap the contents out under the
	// lock and process them without holding it, so a drain never
	// blocks concurrent queueing for longer than the swap, and queue
	// operations issued mid-tick land in the fresh queue for the next
	// drain. Nothing else may read or write these fields.
	queueMu           sync.Mutex
	addPlayerQueue    []*Player
	removePlayerQueue []*Player
	logoutRequests    map[*Player]struct{}
	addNPCQueue       []*NPC
	removeNPCQueue    []*NPC

	processOrder     []*Player
	nextShuffleCycle int64
	rng              *rand.Rand

	npcsAddedThisCycle map[int]bool

	// playerUpdateBuckets and npcUpdateBuckets are the per-chunk
	// visibility buckets, keyed `z:(x>>3):(y>>3)` (World.ts
	// getUpdateBucketKey). Rebuilt every tick by
	// RebuildUpdateBuckets and read by the Nearby/Update methods in
	// the same tick. Tick-goroutine-only: not synchronized, and no
	// other goroutine may read or write them. See visibility.go.
	playerUpdateBuckets map[string][]*Player
	npcUpdateBuckets    map[string][]*NPC

	// Hooks fills unported phases and observes world events. See
	// WorldHooks. It may be assigned before the first Process call;
	// mutating it mid-tick is not safe.
	Hooks WorldHooks

	// Updating mirrors Server.isUpdating(): when true, queued player
	// removals proceed regardless of the combat hold
	// (World.ts queue_remove_players phase).
	Updating bool
}

// NewWorld returns a World whose players/NPCs lists have the TS
// capacities (2048 / 65534) and whose spawn is the given location (the
// world spawn from world.json; (3089, 3524, 0) in the analyzed data — the
// coordinator supplies the configured value, this constructor does not
// invent one).
func NewWorld(spawn Location) *World {
	w := &World{
		spawn:   spawn,
		players: NewMobileList[*Player](MaxPlayers),
		npcs:    NewMobileList[*NPC](MaxNPCs),
		tasks:   NewTaskManager(),
		// Empty manager, open ground: nil sources tolerate every
		// tile as unclipped, matching TS getClipping's answer for
		// a missing region. The new-terrain-format flag is true
		// (the rev-242 format); with no sources it loads nothing.
		regions:             NewRegionManager(nil, nil, true),
		logoutRequests:      make(map[*Player]struct{}),
		rng:                 rand.New(rand.NewSource(time.Now().UnixNano())),
		npcsAddedThisCycle:  make(map[int]bool),
		playerUpdateBuckets: make(map[string][]*Player),
		npcUpdateBuckets:    make(map[string][]*NPC),
		worldEntities:       NewWorldEntitySync(nil),
	}
	w.objects = NewObjectRegistry()
	w.objects.SetRegionManager(w.regions)
	w.areas = NewAreaManager()
	w.equipmentEffects = NewEquipmentEffects()
	return w
}

// Areas returns the world's area registry. It is never nil for a
// NewWorld world; a zero World lazily creates one (with no
// Wilderness installed, so the multi-combat zone fallback
// answers false until the coordinator installs one).
func (w *World) Areas() *AreaManager {
	if w.areas == nil {
		w.areas = NewAreaManager()
	}
	return w.areas
}

// SetAreaManager replaces the world's area registry. Call it only
// before the engine starts (areas register at boot/plugin load,
// and players/NPCs added later read the installed manager); a
// nil manager is ignored. The coordinator uses it to install a
// manager pre-wired with the world's Wilderness.
func (w *World) SetAreaManager(m *AreaManager) {
	if m != nil {
		w.areas = m
	}
}

// EquipmentEffects returns the world's run-energy modifier
// registry. It is never nil for a NewWorld world; a zero World
// lazily creates one. There is no setter: nothing configures
// the registry (unlike the AreaManager's Wilderness), so the
// one NewWorld builds is the one the hub flushes into and
// players read — replacing it after adds would strand
// already-added players on the old instance.
func (w *World) EquipmentEffects() *EquipmentEffects {
	if w.equipmentEffects == nil {
		w.equipmentEffects = NewEquipmentEffects()
	}
	return w.equipmentEffects
}

// GameObjects returns the world's game-object registry. It is
// never nil for a NewWorld world; a zero World lazily creates one
// (bound to the world's region manager, which may itself be nil,
// in which case the registry loads nothing and clips nothing).
func (w *World) GameObjects() *ObjectRegistry {
	if w.objects == nil {
		w.objects = NewObjectRegistry()
		w.objects.SetRegionManager(w.regions)
	}
	return w.objects
}

// RegionManager returns the world's collision/region manager. It is
// never nil: NewWorld installs the empty open-ground manager, and
// SetRegionManager ignores nil.
func (w *World) RegionManager() *RegionManager { return w.regions }

// SetRegionManager replaces the world's collision/region manager
// with the cache-backed one. Call it only before the engine starts:
// after boot, AddPlayer wiring reads RegionManager from network
// goroutines while the tick goroutine steps against it, and the
// swap is not synchronized. A nil manager is ignored (the current
// manager is kept), so RegionManager never returns nil.
func (w *World) SetRegionManager(rm *RegionManager) {
	if rm != nil {
		w.regions = rm
		// The object registry follows the manager: its lookups
		// load regions and its performs clip through it. A nil
		// registry (zero World) picks the manager up lazily
		// from GameObjects.
		if w.objects != nil {
			w.objects.SetRegionManager(rm)
		}
	}
}

// BoatManager returns the world's boat manager, or nil when
// sailing is not installed (visibility and movement then
// degenerate to the no-boats behavior). Safe on a nil World.
func (w *World) BoatManager() *BoatManager {
	if w == nil {
		return nil
	}
	return w.boats
}

// SetBoatManager installs the world's boat manager. Call it only
// before the engine starts: after boot, the tick goroutine reads
// it on every tick (visibility, world-entity sync), and the swap
// is not synchronized. The world-entity sync follows the
// manager. A nil manager restores the no-boats degeneration.
// Safe on a nil World (a no-op).
func (w *World) SetBoatManager(m *BoatManager) {
	if w == nil {
		return
	}
	w.boats = m
	if w.worldEntities != nil {
		w.worldEntities.SetBoats(m)
	}
}

// WorldEntities returns the world's per-viewer boat sync state.
// It is never nil for a NewWorld world; a zero World lazily
// creates one (with no boats until SetBoatManager installs a
// manager). Safe on a nil World (returns nil).
func (w *World) WorldEntities() *WorldEntitySync {
	if w == nil {
		return nil
	}
	if w.worldEntities == nil {
		w.worldEntities = NewWorldEntitySync(w.boats)
	}
	return w.worldEntities
}

// ForgetWorldEntityViewer drops a player's boat sync state (on
// logout/disconnect; the TS WeakMap drops it when the player is
// collected), so a stale viewer never survives a relogin. Safe
// on a nil World (a no-op).
func (w *World) ForgetWorldEntityViewer(p *Player) {
	if s := w.WorldEntities(); s != nil {
		s.Forget(p)
	}
}

// SeedRand reseeds the player-order shuffle RNG. Tests use it for
// deterministic shuffles; production code should leave the default
// time-based seed alone.
func (w *World) SeedRand(seed int64) {
	w.rng = rand.New(rand.NewSource(seed))
}

// Spawn returns the world's spawn location.
func (w *World) Spawn() Location { return w.spawn }

// Players returns the world player list.
func (w *World) Players() *MobileList[*Player] { return w.players }

// NPCs returns the world NPC list.
func (w *World) NPCs() *MobileList[*NPC] { return w.npcs }

// Tasks returns the world's task manager.
func (w *World) Tasks() *TaskManager { return w.tasks }

// ProcessCycle returns the current process cycle counter. It increments
// at the start of every Process call (masked to 31 bits).
// World.ts:1182: processCycle = (processCycle + 1) & 0x7fffffff.
func (w *World) ProcessCycle() int64 { return w.cycle.Load() }

// IsFull reports whether the player list is full (2048 players).
func (w *World) IsFull() bool { return w.players.IsFull() }

// QueueAddPlayer enqueues a player for addition on the next cycle.
// There is no queue-size cap; the drain cap (150) applies in Process.
// Safe to call from any goroutine.
func (w *World) QueueAddPlayer(p *Player) {
	if p != nil {
		w.queueMu.Lock()
		w.addPlayerQueue = append(w.addPlayerQueue, p)
		w.queueMu.Unlock()
	}
}

// QueueRemovePlayer enqueues the registered player at index for removal
// on the next cycle. Unknown indices and duplicates are ignored (TS
// Player.requestLogout checks remove-queue membership before pushing).
func (w *World) QueueRemovePlayer(index int) {
	p, ok := w.players.Get(index)
	if !ok {
		return
	}
	w.queueRemovePlayer(p)
}

func (w *World) queueRemovePlayer(p *Player) {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	for _, q := range w.removePlayerQueue {
		if q == p {
			return
		}
	}
	w.removePlayerQueue = append(w.removePlayerQueue, p)
}

// isQueuedForRemoval reports whether p is currently in the remove
// queue. Used by answerLogoutRequest, which must not re-queue a player
// already awaiting removal.
func (w *World) isQueuedForRemoval(p *Player) bool {
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	for _, q := range w.removePlayerQueue {
		if q == p {
			return true
		}
	}
	return false
}

// QueueLogout enqueues a logout-button press for the registered player at
// index, to be answered on the next cycle (World.ts queueLogoutRequest).
// Unknown indices are ignored; requests form a set, so repeats collapse.
// Safe to call from any goroutine.
func (w *World) QueueLogout(index int) {
	p, ok := w.players.Get(index)
	if !ok {
		return
	}
	w.queueMu.Lock()
	w.logoutRequests[p] = struct{}{}
	w.queueMu.Unlock()
}

// QueueAddNPC enqueues an NPC for addition on the next cycle.
// Safe to call from any goroutine.
func (w *World) QueueAddNPC(n *NPC) {
	if n != nil {
		w.queueMu.Lock()
		w.addNPCQueue = append(w.addNPCQueue, n)
		w.queueMu.Unlock()
	}
}

// QueueRemoveNPC enqueues the registered NPC at index for removal on the
// next cycle. Unknown indices and duplicates are ignored.
// Safe to call from any goroutine.
func (w *World) QueueRemoveNPC(index int) {
	n, ok := w.npcs.Get(index)
	if !ok {
		return
	}
	w.queueMu.Lock()
	defer w.queueMu.Unlock()
	for _, q := range w.removeNPCQueue {
		if q == n {
			return
		}
	}
	w.removeNPCQueue = append(w.removeNPCQueue, n)
}

// PlayerByName returns the registered player with the given username, or
// nil. TS World.getPlayerByName normalizes both sides with
// Misc.formatNameForProtocol(formatDisplayName()) — the Misc name
// functions are NOT STARTED, so the foundation compares usernames
// exactly. The coordinator must normalize before calling if it needs
// TS-equivalent matching.
func (w *World) PlayerByName(username string) *Player {
	p, ok := w.players.Search(func(p *Player) bool { return p.Username == username })
	if !ok {
		return nil
	}
	return p
}

// Process runs one world cycle. The process cycle counter increments
// first, masked to 31 bits (World.ts:1182).
//
// Phase order, ported from World.process() (World.ts:1181-1453; ANALYSIS
// §6.2). Each phase is failure-safe: a panic is recovered, reported via
// Hooks.OnPhaseError (or log.Printf), and the tick continues with the
// next phase. Per-player/per-NPC failures inside the process/flush/reset
// phases are isolated the same way TS isolates them (try/catch per
// entity), with players additionally queued for removal on error:
//
//  1. task_manager — TaskManager.Process.
//  2. ground_items — hook (empty by default).
//  3. queue_add_players — drain addPlayerQueue, cap 150; kick same-name
//     copies first (they are queued for removal).
//  4. logout_requests — answer queued logout requests.
//  5. queue_remove_players — drain removePlayerQueue BACKWARDS, cap 150
//     examined entries; remove only if Updating or the player is
//     readyToLeaveWorld (combat hold).
//  6. queue_add_npcs / queue_remove_npcs — same pattern, cap 150 each,
//     forward (shift) order.
//  7. npc_activation — hook: NPC-spawn activation sweep + observer
//     bucket rebuild (untimed statements in TS).
//  8. process_npcs — run Process on each NPC except those registered
//     this tick (TS: newly registered NPCs wait until next tick).
//  9. process_players — shuffled order (see orderedPlayersForCycle);
//     per-player failure queues removal.
//  10. move_boats — hook (empty by default).
//  11. walk_to_sweep — TaskManager.ProcessRemainingWalkTo (or hook).
//  12. combat_hits — hook (empty by default).
//  13. rebuild_update_buckets + update_players_npcs — rebuild the
//     visibility buckets, then per visibility player collect nearby
//     NPCs and update the local player/NPC lists (per-player failure
//     isolation: a failure reports via OnPlayerError and requests
//     logout for that player only). Either phase runs its hook
//     instead when one is installed.
//  14. flush_network_players — SessionPort.FlushTick for players with an
//     open session; failure queues removal.
//  15. reset_player_updates / reset_npcs — clear per-tick update flags
//     (players first, after all observers built their frames).
func (w *World) Process() {
	w.cycle.Store((w.cycle.Load() + 1) & 0x7fffffff)
	cycle := w.cycle.Load()
	w.npcsAddedThisCycle = make(map[int]bool)

	w.runPhase("task_manager", func() { w.tasks.Process(cycle) })
	w.runPhase("shops", func() {
		if h := w.Hooks.Shops; h != nil {
			h(cycle)
		}
	})
	w.runPhase("ground_items", func() {
		if h := w.Hooks.GroundItems; h != nil {
			h(cycle)
		}
	})
	w.runPhase("queue_add_players", func() { w.drainAddPlayers() })
	w.runPhase("logout_requests", func() { w.answerLogoutRequests() })
	w.runPhase("queue_remove_players", func() { w.drainRemovePlayers(cycle) })
	w.runPhase("queue_add_npcs", func() { w.drainAddNPCs() })
	w.runPhase("queue_remove_npcs", func() { w.drainRemoveNPCs() })
	w.runPhase("npc_activation", func() {
		if h := w.Hooks.NPCActivation; h != nil {
			h(cycle)
		}
	})
	w.runPhase("process_npcs", func() { w.processNPCs(cycle) })
	w.runPhase("process_players", func() { w.processPlayers(cycle) })
	w.runPhase("move_boats", func() {
		if h := w.Hooks.Boats; h != nil {
			h(cycle)
		}
	})
	w.runPhase("walk_to_sweep", func() {
		if h := w.Hooks.WalkToSweep; h != nil {
			h(cycle)
		} else {
			w.tasks.ProcessRemainingWalkTo(cycle)
		}
	})
	w.runPhase("combat_hits", func() {
		if h := w.Hooks.CombatHits; h != nil {
			h(cycle)
		}
	})
	w.runPhase("rebuild_update_buckets", func() {
		if h := w.Hooks.VisibilityBuckets; h != nil {
			h(cycle)
		} else {
			w.RebuildUpdateBuckets()
		}
	})
	w.runPhase("update_players_npcs", func() {
		if h := w.Hooks.UpdatePlayersNPCs; h != nil {
			h(cycle)
		} else {
			w.updateVisibilityPlayers()
		}
	})
	w.runPhase("flush_network_players", func() { w.flushPlayers(cycle) })
	w.runPhase("reset_player_updates", func() { w.resetPlayers() })
	w.runPhase("reset_npcs", func() { w.resetNPCs() })
}

// runPhase executes a phase, recovering panics into a phase error report
// so the tick continues. (TS wraps the task/ground-item phases in
// try/catch explicitly and isolates per-entity failures; the foundation
// applies the same failure-safety uniformly, per the port spec.)
func (w *World) runPhase(name string, fn func()) {
	defer func() {
		if r := recover(); r != nil {
			w.phaseError(name, fmt.Errorf("panic: %v", r))
		}
	}()
	fn()
}

func (w *World) phaseError(phase string, err error) {
	if h := w.Hooks.OnPhaseError; h != nil {
		h(phase, err)
		return
	}
	log.Printf("[world] phase %s failure: %v", phase, err)
}

func (w *World) playerError(p *Player, phase string, err error) {
	if h := w.Hooks.OnPlayerError; h != nil {
		h(p, phase, err)
		return
	}
	w.phaseError(phase, err)
}

func (w *World) npcError(n *NPC, phase string, err error) {
	if h := w.Hooks.OnNPCError; h != nil {
		h(n, phase, err)
		return
	}
	w.phaseError(phase, err)
}

func (w *World) drainAddPlayers() {
	// Swap out at most QueueDrainCap entries; the rest stay queued in
	// order, and entries queued mid-drain land behind them.
	w.queueMu.Lock()
	take := QueueDrainCap
	if len(w.addPlayerQueue) < take {
		take = len(w.addPlayerQueue)
	}
	batch := w.addPlayerQueue[:take]
	w.addPlayerQueue = w.addPlayerQueue[take:]
	w.queueMu.Unlock()
	for _, p := range batch {
		if p == nil {
			continue
		}
		// Kick any copies before adding the new player
		// (World.ts queue_add_players phase).
		if existing := w.PlayerByName(p.Username); existing != nil && existing != p {
			w.queueRemovePlayer(existing)
		}
		// Wire RequestLogout to this world if the coordinator has not
		// installed its own hook. The closure reads p.Index at call
		// time, after Add assigns it.
		if p.OnRequestLogout == nil {
			p.OnRequestLogout = func(pl *Player) { w.QueueRemovePlayer(pl.Index) }
		}
		// Install the world's AreaManager so Player.Process runs
		// area processing in the TS position (Player.ts:553).
		// Areas() lazily creates a bare manager for a zero
		// World; a bare manager still sends the initial
		// multi-icon varbit, as TS does with no areas
		// registered.
		p.areaManager = w.Areas()
		// Install the world's run-energy modifier registry so
		// the drain/restore formulas apply content modifiers
		// (EquipmentEffects.ts). EquipmentEffects() lazily
		// creates a bare registry for a zero World; a bare
		// registry applies identity.
		p.equipmentEffects = w.EquipmentEffects()
		// TS ignores the add result for players (a full list silently
		// drops the login; the login path checks IsFull first). NPC
		// spawn activation, tile occupancy deferral, and active-region
		// refresh are NOT STARTED and intentionally absent here.
		w.players.Add(p)
	}
}

func (w *World) answerLogoutRequests() {
	// Swap the set out; requests queued mid-phase wait for the next
	// cycle. TS clears the whole set after answering, including
	// requests that were skipped (disconnected) or refused.
	w.queueMu.Lock()
	requests := w.logoutRequests
	w.logoutRequests = make(map[*Player]struct{})
	w.queueMu.Unlock()
	for p := range requests {
		w.answerLogoutRequest(p)
	}
}

func (w *World) answerLogoutRequest(p *Player) {
	if p == nil {
		return
	}
	if w.isQueuedForRemoval(p) {
		return
	}
	// TS: skip when the session is not connected
	// (World.ts answerLogoutRequest).
	if p.Session == nil || !p.Session.IsOpen() {
		return
	}
	if h := w.Hooks.LogoutRefusal; h != nil {
		if reason := h(p); reason != "" {
			if rh := w.Hooks.OnLogoutRefused; rh != nil {
				rh(p, reason)
			}
			return
		}
	}
	p.SetDisconnectSource("logout")
	w.queueRemovePlayer(p)
	if h := w.Hooks.OnLogoutAccepted; h != nil {
		h(p)
	}
}

func (w *World) drainRemovePlayers(cycle int64) {
	// Deregister queued players. A player hit in combat stays in the
	// world after their connection is gone until combat ends or the
	// hold runs out. Iteration is BACKWARDS and amount counts every
	// examined entry (nil entries excepted — TS continues before
	// incrementing for them), so a full queue of held players still
	// consumes the 150 budget. (World.ts queue_remove_players phase.)
	//
	// The queue is swapped out under the lock and survivors
	// (unexamined entries plus held players, in original order) are
	// re-queued ahead of anything queued mid-drain.
	w.queueMu.Lock()
	q := w.removePlayerQueue
	w.removePlayerQueue = nil
	w.queueMu.Unlock()

	amount := 0
	var held []*Player // examined but not ready, in encounter (reverse) order
	cutoff := len(q)   // entries at indices < cutoff were never examined
	for index := len(q) - 1; index >= 0; index-- {
		p := q[index]
		if p == nil {
			// Examined nils are dropped (TS splices them out).
			cutoff = index
			continue
		}
		if amount >= QueueDrainCap {
			break
		}
		cutoff = index
		amount++
		if w.Updating || p.ReadyToLeaveWorld(cycle) {
			// TS emits the disconnect event only for non-bot
			// registered players (a socket closing after its player
			// already left queues them again).
			if !p.IsBot && p.IsRegistered() {
				if h := w.Hooks.OnPlayerDisconnect; h != nil {
					source := p.DisconnectSource()
					if source == "" {
						source = "removed"
					}
					h(p, source)
				}
			}
			w.removePlayerGuarded(p)
			// Leave the player's area, as onLogout does
			// (Player.ts:741-748, via onRemove, inside
			// MobileList.remove): the area hears leave with
			// logout=true, and a throwing leave must not stop
			// the rest of the removal — TS wraps it in
			// try/catch and logs. Go runs it here, after
			// removal, next to the other post-removal
			// cleanups; the player object is still fully
			// readable, so postLeave hooks observe the same
			// state. The leave goes through the AreaMethodCaller
			// seam (callArea in TS) when one is installed.
			w.leaveAreaOnRemoval(p)
			// Drop the departed player's area hint: Go has no
			// WeakMap, so the hint map needs this call where
			// TS garbage collection drops the entry (see
			// AreaManager.Forget).
			if w.areas != nil {
				w.areas.Forget(p)
			}
			// Drop the departed player's owner-object entries:
			// Go has no WeakMap, so the registry's explicit
			// owner map needs this call where TS garbage
			// collection drops the entry (see
			// ObjectRegistry.DiscardPlayer). The disconnect
			// hook above has already run; removal is complete.
			if w.objects != nil {
				w.objects.DiscardPlayer(p)
			}
		} else {
			held = append(held, p)
		}
	}
	if cutoff > 0 || len(held) > 0 {
		survivors := make([]*Player, 0, cutoff+len(held))
		survivors = append(survivors, q[:cutoff]...)
		for i := len(held) - 1; i >= 0; i-- {
			survivors = append(survivors, held[i])
		}
		w.queueMu.Lock()
		w.removePlayerQueue = append(survivors, w.removePlayerQueue...)
		w.queueMu.Unlock()
	}
}

// leaveAreaOnRemoval delivers the logout leave to the player's
// current area, if any (Player.ts:741-748: onLogout calls
// getArea().leave(player, true) inside try/catch, logging
// `[World] area leave failed on logout (username)` on failure).
// A throwing leave is reported through the phase-error path and
// never aborts the removal. NPCs get no equivalent: TS
// NPC.onRemove is empty (NPC.ts:259-261).
func (w *World) leaveAreaOnRemoval(p *Player) {
	area := p.Area()
	if area == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			w.phaseError("queue_remove_players", fmt.Errorf("[World] area leave failed on logout (%s): %v", p.Username, r))
		}
	}()
	callAreaLeave(area, p, true)
}

// removePlayerGuarded removes p from the list, recovering a panic from a
// removal hook so the tick continues. (TS wraps World.players.remove in
// try/catch for the same reason: the player is already deregistered, so
// the failure must stay observable without aborting the tick.)
func (w *World) removePlayerGuarded(p *Player) {
	defer func() {
		if r := recover(); r != nil {
			w.phaseError("queue_remove_players", fmt.Errorf("panic removing player %s: %v", p.Username, r))
		}
	}()
	w.players.Remove(p.Index)
}

func (w *World) drainAddNPCs() {
	w.queueMu.Lock()
	take := QueueDrainCap
	if len(w.addNPCQueue) < take {
		take = len(w.addNPCQueue)
	}
	batch := w.addNPCQueue[:take]
	w.addNPCQueue = w.addNPCQueue[take:]
	w.queueMu.Unlock()
	for _, n := range batch {
		if n == nil {
			continue
		}
		index, ok := w.npcs.Add(n)
		if !ok {
			// TS logs add_queue_failed with npc id/index/counts.
			w.phaseError("queue_add_npcs", fmt.Errorf("add_queue_failed npcId=%d npcIndex=%d worldNpcCount=%d", n.ID, n.Index, w.npcs.Len()))
			continue
		}
		// Install the world's AreaManager so NPC.Process runs
		// area processing in the TS position (NPC.ts:378).
		n.areaManager = w.Areas()
		// Install collision/clipping on the movement queue and
		// the movement coordinator so wandering, walking home,
		// and combat approaches route on real ground instead
		// of open tiles (the player queues get the same install
		// in cmd/server). A nil RegionManager (tests, worlds
		// without regions) leaves the open-ground defaults.
		if w.regions != nil {
			// Actor-aware surfaces: step validation and
			// route-finding read the NPC's current private
			// area fresh on every call (MovementQueue
			// passes character.getPrivateArea() at every
			// call site in TS), so an NPC that enters an
			// instance routes on the area's clips without
			// reinstalling. The player queues get the same
			// install in cmd/server.
			area := func() *PrivateArea { return n.PrivateArea() }
			if q := n.MovementQueue(); q != nil {
				q.SetCollision(w.regions.CollisionForActor(area))
				q.SetClipping(w.regions.LookupForActor(area))
				// The world's boat manager, so deck
				// walk-tos project onto the main world
				// (nil in worlds without sailing).
				q.SetBoatManager(w.boats)
			}
			if coordinator := n.MovementCoordinator(); coordinator != nil {
				coordinator.SetClipping(w.regions.LookupForActor(area))
			}
		}
		// Bind the NPC combat identity (and install the
		// coordinator's combat seams) before the first tick,
		// so combat checks never see an unbound fighter.
		n.GetCombat()
		w.npcsAddedThisCycle[index] = true
	}
}

func (w *World) drainRemoveNPCs() {
	w.queueMu.Lock()
	take := QueueDrainCap
	if len(w.removeNPCQueue) < take {
		take = len(w.removeNPCQueue)
	}
	batch := w.removeNPCQueue[:take]
	w.removeNPCQueue = w.removeNPCQueue[take:]
	w.queueMu.Unlock()
	for _, n := range batch {
		if n == nil {
			continue
		}
		w.removeNPCGuarded(n)
		// Drop the departed NPC's area hint (see Forget on
		// the player path). No area leave: TS NPC.onRemove
		// is empty (NPC.ts:259-261).
		if w.areas != nil {
			w.areas.Forget(n)
		}
	}
}

func (w *World) removeNPCGuarded(n *NPC) {
	defer func() {
		if r := recover(); r != nil {
			w.phaseError("queue_remove_npcs", fmt.Errorf("panic removing npc %d: %v", n.ID, r))
		}
	}()
	w.npcs.Remove(n.Index)
}

func (w *World) processNPCs(cycle int64) {
	// TS collects region-active NPCs and runs processNpcMechanics, which
	// isolates per-NPC failures (World.ts:594-601). Region gating is NOT
	// STARTED, so every registered NPC is processed — except NPCs
	// registered this tick, whose first mechanics turn waits until next
	// tick (World.ts process_npcs phase comment).
	for _, n := range w.npcs.All() {
		if w.npcsAddedThisCycle[n.Index] {
			continue
		}
		w.processNPCGuarded(n, cycle)
	}
}

func (w *World) processNPCGuarded(n *NPC, cycle int64) {
	defer func() {
		if r := recover(); r != nil {
			w.npcError(n, "process_npcs", fmt.Errorf("panic: %v", r))
		}
	}()
	n.Process(cycle)
}

func (w *World) processPlayers(cycle int64) {
	// TS: per-player try/catch; on error, requestLogout
	// (World.ts process_players phase). Shop processing runs
	// behind Hooks.ShopProcessPlayer in processPlayerGuarded
	// (phase 17). Bot process strides (shouldProcessBotPlayerThisTick)
	// are NOT STARTED: every registered player takes a turn every
	// cycle. The per-player plugin emit (emitPlayerProcess) runs
	// behind Hooks.PlayerProcess in processPlayerGuarded.
	for _, p := range w.orderedPlayersForCycle(cycle) {
		w.processPlayerGuarded(p, cycle)
	}
}

func (w *World) processPlayerGuarded(p *Player, cycle int64) {
	defer func() {
		if r := recover(); r != nil {
			w.playerError(p, "process_players", fmt.Errorf("panic: %v", r))
			p.RequestLogout()
		}
	}()
	p.Process(cycle)
	// TS order (World.ts process_players): ShopManager
	// .processPlayer runs after player.process() and before
	// emitPlayerProcess, for every processed player — the
	// manager itself skips unwatched bots. The hook call sits
	// under the same recover as Player.Process.
	if h := w.Hooks.ShopProcessPlayer; h != nil {
		h(p)
	}
	// TS emits the player-process event only for non-bot players
	// (World.ts:1372); the hub's EmitPlayerProcess drives the
	// zone-enter/exit update first, before the process hooks,
	// as TS updatePlayerZones does (PluginManager.ts:881-882).
	// The hook call sits under the same recover as
	// Player.Process, so a panic in the hub machinery itself is
	// treated like a process panic; per-hook panics never reach
	// here because the hub isolates them (executeHook).
	if !p.IsBot && p.IsRegistered() {
		if h := w.Hooks.PlayerProcess; h != nil {
			h(p, cycle)
		}
	}
}

// orderedPlayersForCycle ports World.ts:958-976 (orderedPlayersForCycle).
// The process order persists across cycles: departed players are
// filtered out, new players are appended, and the order is reshuffled
// every 100-150 cycles (cycle + 100 + rand(0..50)).
//
// The duelling-order exception in TS shufflePlayerProcessOrder —
// duelling players keep their slots so both fighters see stable
// relative priority — is implemented in
// shufflePlayerProcessOrder (Dueling state ported in phase 17;
// no duel gameplay exists yet, so no player can be in a duel
// in production, but the exception is live for any future
// state that sets one).
func (w *World) orderedPlayersForCycle(cycle int64) []*Player {
	current := w.players.All()
	registered := make(map[*Player]bool, len(current))
	for _, p := range current {
		registered[p] = true
	}
	kept := w.processOrder[:0]
	for _, p := range w.processOrder {
		if registered[p] {
			kept = append(kept, p)
		}
	}
	w.processOrder = kept
	ordered := make(map[*Player]bool, len(w.processOrder))
	for _, p := range w.processOrder {
		ordered[p] = true
	}
	for _, p := range current {
		if !ordered[p] {
			w.processOrder = append(w.processOrder, p)
		}
	}
	if w.nextShuffleCycle == 0 || cycle >= w.nextShuffleCycle {
		w.shufflePlayerProcessOrder()
		w.nextShuffleCycle = cycle + 100 + int64(w.rng.Intn(51))
	}
	out := make([]*Player, len(w.processOrder))
	copy(out, w.processOrder)
	return out
}

// shufflePlayerProcessOrder is the Fisher-Yates shuffle from
// World.ts:985-998, including the duelling exception: indices
// whose player is in a duel are not movable, so a reshuffle
// never flips which fighter lands first mid-fight.
func (w *World) shufflePlayerProcessOrder() {
	order := w.processOrder
	var movable []int
	for index, p := range order {
		if p == nil || !p.Dueling().InDuel() {
			movable = append(movable, index)
		}
	}
	for i := len(movable) - 1; i > 0; i-- {
		j := w.rng.Intn(i + 1)
		a, b := movable[i], movable[j]
		order[a], order[b] = order[b], order[a]
	}
}

func (w *World) flushPlayers(cycle int64) {
	// TS: for players that should run network updates,
	// session.flush(processCycle); on error, requestLogout
	// (World.ts flush_network_players phase). shouldRunNetworkUpdates
	// excludes bots without a network client and disconnected
	// sessions; the foundation gate is Session != nil && IsOpen(),
	// which covers both (a bot with an administrator-driven client has
	// an open session).
	w.players.ForEach(func(p *Player) {
		if p.Session == nil || !p.Session.IsOpen() {
			return
		}
		w.flushPlayerGuarded(p, cycle)
	})
}

func (w *World) flushPlayerGuarded(p *Player, cycle int64) {
	defer func() {
		if r := recover(); r != nil {
			w.playerError(p, "flush_network_players", fmt.Errorf("panic: %v", r))
			p.RequestLogout()
		}
	}()
	if err := p.Session.FlushTick(cycle); err != nil {
		w.playerError(p, "flush_network_players", err)
		p.RequestLogout()
	}
}

func (w *World) resetPlayers() {
	// A bot can appear before its human observer in the player list.
	// Keep its flags until every observer has built this tick's sync
	// frame — hence players reset after the flush phase, players first.
	// (World.ts reset_player_updates phase.) On error TS requests
	// logout; the foundation does the same.
	w.players.ForEach(func(p *Player) {
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.playerError(p, "reset_player_updates", fmt.Errorf("panic: %v", r))
					p.RequestLogout()
				}
			}()
			p.ResetUpdating()
		}()
	})
}

func (w *World) resetNPCs() {
	// TS resets only activeNpcsForUpdate (the region-collected set).
	// Region gating is NOT STARTED, so the foundation resets every
	// registered NPC. On error TS logs and continues (no logout).
	for _, n := range w.npcs.All() {
		func() {
			defer func() {
				if r := recover(); r != nil {
					w.npcError(n, "reset_npcs", fmt.Errorf("panic: %v", r))
				}
			}()
			n.ResetUpdating()
		}()
	}
}

func spliceOut[T any](s []T, index int) []T {
	var zero T
	s[index] = zero
	return append(s[:index], s[index+1:]...)
}
