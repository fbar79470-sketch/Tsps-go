package game

import (
	"sync"
	"sync/atomic"
)

// NPC respawn: the port of game/task/impl/NPCRespawnTask.ts,
// the respawn-queueing half of NPCDeathTask.remove
// (NPCDeathTask.ts:92-110), NPC.clone (NPC.ts:1032-1042), and
// NpcSpawnLineage.ts.
//
// LIFECYCLE. When an NPC dies, the combat core's death handling
// calls BeginRespawn at the point NPCDeathTask.remove runs in
// TS (after the death animation ticks, not at appendDeath):
// the respawn delay comes from the NPC definition
// (NpcDefinition.getRespawn, default 25 ticks; 0 disables),
// and a respawn task is submitted to the world's TaskManager.
// When the delay elapses, the task checks the spawn lineage
// (a retired lineage queues no more respawns), clones the NPC,
// claims its unique key (a unique NPC another spawn already
// brought back stays single), and queues the clone onto the
// world's add-NPC queue.
//
// SCHEDULING SEAM. An NPC holds no World reference, so the
// TaskManager and the add queue cross through the
// NpcRespawnScheduler seam (see SetNpcRespawnScheduler):
// production installs the world-backed scheduler with
// World.InstallNpcRespawnWiring at boot; with none installed,
// BeginRespawn schedules nothing. Tests install a recording
// scheduler. The task itself is built by newNPCRespawnTask, so
// any scheduler runs the same fire-time semantics.
//
// CLONE STATE. Clone keeps the id, spawn position, spawn face,
// wander radius, owner/ownerOnly scope, spawn lineage, and
// unique key (NPC.ts:1032-1042). Hitpoints come from the
// definition, as the TS constructor assigns them. Nothing else
// carries over: no timers, attributes, interaction, following,
// scripted movement, or combat state — a respawn is a new life.
//
// LINEAGE RETIRE. NpcSpawnLineage.Retire marks the lineage
// retired, which stops its queued respawns at fire time. The
// TS retire also removes the lineage's live NPCs from the
// world (NpcSpawnLineage.ts:18-28); that half needs World
// iteration and is NOT ported — see Retire. The spawn-file
// lineage (owner "npc-spawns") is shared by every NPC the
// spawn loader places, as in TS (NpcSpawnDefinitionLoader
// .ts:53, 306-316).

// NpcSpawnsLineageOwner is LINEAGE_OWNER
// (NpcSpawnDefinitionLoader.ts:53): what the spawn file's
// lineage is called.
const NpcSpawnsLineageOwner = "npc-spawns"

// npcSpawnsLineage is the shared lineage for every NPC the
// spawn loader places (NpcSpawnDefinitionLoader.ts:52, the
// static lineage). Spawned NPCs share one lineage object so a
// reload retires them together.
var npcSpawnsLineage = NewNpcSpawnLineage(NpcSpawnsLineageOwner)

// NpcSpawnsLineage returns the shared spawn-file lineage.
func NpcSpawnsLineage() *NpcSpawnLineage { return npcSpawnsLineage }

// NpcSpawnLineage is what put an NPC in the world: the spawn
// file, or a plugin by name (NpcSpawnLineage.ts:5-9). A respawn
// clone inherits it, so everything one source placed can be
// found and taken out together, and a retired lineage queues
// no more respawns. The retired flag is atomic: retire may be
// called from content goroutines while respawn tasks check it
// on the tick goroutine.
type NpcSpawnLineage struct {
	owner   string
	retired atomic.Bool
}

// NewNpcSpawnLineage returns a lineage owned by owner.
func NewNpcSpawnLineage(owner string) *NpcSpawnLineage {
	return &NpcSpawnLineage{owner: owner}
}

// Owner returns what put the lineage's NPCs in the world
// (NpcSpawnLineage.ts:11, the readonly owner).
func (l *NpcSpawnLineage) Owner() string {
	if l == nil {
		return ""
	}
	return l.owner
}

// IsRetired reports whether the lineage was retired
// (NpcSpawnLineage.ts:13-15). A nil lineage reads as live.
func (l *NpcSpawnLineage) IsRetired() bool {
	return l != nil && l.retired.Load()
}

// Retire stops the lineage's queued respawns
// (NpcSpawnLineage.ts:18-28, retire). The live-NPC removal
// half of TS retire — taking every live NPC of the lineage
// out of the world, optionally keeping player-owned ones — is
// NOT ported: it iterates the world's NPC list and queues,
// which no lineage method can reach. The coordinator owns that
// half (a World method over the NPC lists); until it lands, a
// retired lineage's live NPCs stay standing while its queued
// respawns stop. Retiring twice is harmless.
func (l *NpcSpawnLineage) Retire() {
	if l == nil {
		return
	}
	l.retired.Store(true)
}

// SpawnLineage returns the NPC's spawn lineage, or nil
// (NPC.ts:729-731, getSpawnLineage).
func (n *NPC) SpawnLineage() *NpcSpawnLineage { return n.spawnLineage }

// SetSpawnLineage records what spawned the NPC
// (NPC.ts:733-736, setSpawnLineage). The spawn path sets the
// shared spawn-file lineage; respawn clones inherit it.
func (n *NPC) SetSpawnLineage(lineage *NpcSpawnLineage) { n.spawnLineage = lineage }

// UniqueKey returns the uniqueness key held while this NPC is
// live, and whether one is held (NPC.ts:739-741, getUniqueKey:
// null when none). A key means a boss that must never be in
// the world twice.
func (n *NPC) UniqueKey() (string, bool) { return n.uniqueKey, n.hasUniqueKey }

// SetUniqueKey records the uniqueness key (NPC.ts:743-746,
// setUniqueKey). ClearUniqueKey clears it.
func (n *NPC) SetUniqueKey(key string) {
	n.uniqueKey = key
	n.hasUniqueKey = true
}

// ClearUniqueKey clears the uniqueness key: the Go form of
// setUniqueKey(null).
func (n *NPC) ClearUniqueKey() {
	n.uniqueKey = ""
	n.hasUniqueKey = false
}

// uniqueHolders maps a uniqueness key to the NPC holding it
// (NpcSpawnLineage.ts:30-31, the static uniqueHolders map).
// The mutex guards it: claims run on the tick goroutine, but
// content may claim from the login goroutine.
var (
	uniqueHoldersMu sync.Mutex
	uniqueHolders   = make(map[string]*NPC)
)

// uniqueHolderLive answers NpcSpawnLineage.isLive for a holder:
// in the world or queued to join it, and not queued to leave
// (NpcSpawnLineage.ts:34-38). The default answers registration
// alone, which misses the add-queue window (a clone queued but
// not yet registered reads as gone); World
// .InstallNpcRespawnWiring installs the exact check over the
// world's queues. It is set once at boot and never changed
// afterwards, like the other coordinator-installed wiring.
var uniqueHolderLive = func(n *NPC) bool { return n.IsRegistered() }

// SetUniqueHolderLiveness installs the isLive predicate
// UniqueHolder prunes with. A nil predicate restores the
// registration-only default. Call once at boot, before
// ticking starts.
func SetUniqueHolderLiveness(live func(*NPC) bool) {
	if live == nil {
		uniqueHolderLive = func(n *NPC) bool { return n.IsRegistered() }
		return
	}
	uniqueHolderLive = live
}

// UniqueHolder returns the live NPC holding key, or nil once it
// has left the world (NpcSpawnLineage.ts:41-48, uniqueHolder).
// A stale holder is pruned from the map when observed.
func UniqueHolder(key string) *NPC {
	uniqueHoldersMu.Lock()
	defer uniqueHoldersMu.Unlock()
	holder := uniqueHolders[key]
	if holder == nil {
		return nil
	}
	if uniqueHolderLive(holder) {
		return holder
	}
	delete(uniqueHolders, key)
	return nil
}

// ClaimUnique gives npc the key unless a live NPC already holds
// it (NpcSpawnLineage.ts:51-56, claimUnique). Claiming sets the
// key on the NPC. Re-claiming a key the same NPC already holds
// succeeds.
func ClaimUnique(key string, npc *NPC) bool {
	if npc == nil {
		return false
	}
	uniqueHoldersMu.Lock()
	defer uniqueHoldersMu.Unlock()
	holder := uniqueHolders[key]
	if holder != nil && holder != npc && uniqueHolderLive(holder) {
		return false
	}
	if holder != nil && holder != npc {
		delete(uniqueHolders, key)
	}
	npc.SetUniqueKey(key)
	uniqueHolders[key] = npc
	return true
}

// Clone returns the respawn clone of this NPC (NPC.ts:1032-
// 1042, clone): a fresh NPC of the same id at the spawn
// position, with the spawn face, the wander radius, the
// owner/ownerOnly scope, the spawn lineage, and the unique key.
// Visible is set (the TS default, true — the Go zero value is
// false, so the clone sets it explicitly, as the spawn path
// does), and hitpoints come from the definition, as the TS
// constructor assigns them. Mobile.Facing starts at the spawn
// face, matching the spawn path's initial facing; the TS
// facing direction starts unknown, but Go sync reads Facing
// for the NPC rotation, and a clone that faces nowhere until
// it moves would render wrongly. Everything else is fresh:
// timers, attributes, interaction, following, scripted
// movement, multi-combat, and pet state do not carry over, as
// TS clone does not copy them.
func (n *NPC) Clone() *NPC {
	clone := NewNPC(n.ID, n.Spawn)
	clone.SetFace(n.face)
	clone.Facing = n.face
	clone.MovementCoordinator().SetRadius(n.coordinatorRadius())
	if owner := n.owner; owner != nil {
		clone.SetOwner(owner)
	}
	if n.ownerOnly {
		clone.SetOwnerOnly(true)
	}
	clone.SetSpawnLineage(n.spawnLineage)
	if key, ok := n.UniqueKey(); ok {
		clone.SetUniqueKey(key)
	}
	clone.Visible = true
	clone.definition = n.definition
	// Hitpoints from the definition, clamped to the default when
	// non-positive — the same rule the spawn path applies
	// (SpawnNPCsWithHitpointsAndDefinitions): a degenerate
	// definition (a 0 hitpoints override) must not respawn an
	// NPC with 0 hitpoints, which the attack gate reads as
	// dead/dying and would make unattackable.
	clone.Hitpoints = clone.Definition().Hitpoints()
	if clone.Hitpoints <= 0 {
		clone.Hitpoints = DefaultNPCHitpoints
	}
	return clone
}

// SetSkipDefaultRespawn sets the skip marker BeginRespawn reads
// (see the skipDefaultRespawn field comment): the next
// BeginRespawn queues no respawn and clears the marker. It is
// the Go form of setting __skipDefaultRespawn (NPCDeathTask
// .ts:94, NpcSpawnDefinitionLoader.ts:296).
func (n *NPC) SetSkipDefaultRespawn(skip bool) { n.skipDefaultRespawn = skip }

// BeginRespawn schedules this NPC's respawn after its
// definition's respawn delay (NPCDeathTask.remove,
// NPCDeathTask.ts:106-109: respawnTicks ?? getDefinition()
// .getRespawn(), queued only when > 0 and the default respawn
// was not skipped). The combat core's death handling calls it
// at the remove point — after the death animation, when the
// NPC leaves the world — not at appendDeath. The area leave,
// dying-flag clear, and transformation reset of remove() are
// death-task work, not respawn scheduling, and live with the
// death handling. With no scheduler installed (see
// SetNpcRespawnScheduler), no respawn is scheduled.
func (n *NPC) BeginRespawn() {
	n.BeginRespawnWithTicks(n.Definition().Respawn())
}

// BeginRespawnWithTicks schedules this NPC's respawn after
// ticks, the explicit form of BeginRespawn for callers that
// override the definition delay (the remains path,
// NPCDeathTask.ts:116-124, passes its respawnTicks through
// remove). A non-positive delay queues nothing, as in TS.
func (n *NPC) BeginRespawnWithTicks(ticks int) {
	skip := n.skipDefaultRespawn
	n.skipDefaultRespawn = false
	if skip || ticks <= 0 {
		return
	}
	if s := npcRespawnScheduler; s != nil {
		s.ScheduleRespawn(n, ticks)
	}
}

// npcRemover removes a dead NPC from the world at the remove
// point (NPCDeathTask.remove pushes
// World.getRemoveNPCQueue()). An NPC holds no World reference,
// so removal crosses this seam like respawn scheduling.
// Installed by World.InstallNpcRespawnWiring; nil means no
// removal (tests without a world, where nothing observes the
// corpse). The removal half was missing until Phase 26: the
// respawn was scheduled but the corpse stayed registered
// forever.
var npcRemover func(*NPC)

// SetNpcRemover installs the dead-NPC removal function. A nil
// function restores the no-removal default. Call once at boot,
// before ticking starts.
func SetNpcRemover(f func(*NPC)) { npcRemover = f }

// NpcRespawnScheduler schedules a delayed respawn: the seam
// between the NPC (which holds no World reference) and the
// world's TaskManager plus add-NPC queue. See the file header.
type NpcRespawnScheduler interface {
	// ScheduleRespawn queues original's respawn clone to join
	// the world after ticks ticks.
	ScheduleRespawn(original *NPC, ticks int)
}

// npcRespawnScheduler is the installed scheduler, or nil. It is
// set once at boot via SetNpcRespawnScheduler (or
// World.InstallNpcRespawnWiring), before the world starts
// ticking, and never changed afterwards; like the other
// coordinator-installed wiring in this package it is
// deliberately unsynchronized.
var npcRespawnScheduler NpcRespawnScheduler

// SetNpcRespawnScheduler installs the respawn scheduler. A nil
// scheduler means BeginRespawn schedules nothing. Call once at
// boot, before ticking starts.
func SetNpcRespawnScheduler(scheduler NpcRespawnScheduler) {
	npcRespawnScheduler = scheduler
}

// taskManagerRespawnScheduler is the world-backed scheduler:
// respawn tasks run on the world's TaskManager, and clones join
// through the world's add-NPC queue.
type taskManagerRespawnScheduler struct {
	tasks *TaskManager
	add   func(*NPC)
}

// NewTaskManagerRespawnScheduler returns a scheduler backed by
// tasks and add (usually World.Tasks() and World.QueueAddNPC).
// A nil tasks or add makes ScheduleRespawn a no-op.
func NewTaskManagerRespawnScheduler(tasks *TaskManager, add func(*NPC)) NpcRespawnScheduler {
	return &taskManagerRespawnScheduler{tasks: tasks, add: add}
}

// ScheduleRespawn submits the respawn task for original with
// the given delay (NPCRespawnTask, NPCRespawnTask.ts:6-9:
// super(ticks)).
func (s *taskManagerRespawnScheduler) ScheduleRespawn(original *NPC, ticks int) {
	if s == nil || s.tasks == nil || s.add == nil || original == nil {
		return
	}
	s.tasks.Schedule(newNPCRespawnTask(original, ticks, s.add))
}

// InstallNpcRespawnWiring installs the world-backed respawn
// scheduler and the exact unique-holder liveness check over
// this world's queues (NpcSpawnLineage.isLive: in the world or
// queued to join, and not queued to leave). It also installs
// the dead-NPC removal (NPCDeathTask.remove pushes
// World.getRemoveNPCQueue()). The coordinator calls it once at
// boot, before the engine starts. It lives in this file — not
// world.go — because respawn is this milestone's work; it
// touches only the respawn seams.
func (w *World) InstallNpcRespawnWiring() {
	SetNpcRespawnScheduler(NewTaskManagerRespawnScheduler(w.Tasks(), w.QueueAddNPC))
	SetNpcRemover(func(n *NPC) {
		if n != nil {
			w.QueueRemoveNPC(n.Index)
		}
	})
	SetUniqueHolderLiveness(func(n *NPC) bool {
		w.queueMu.Lock()
		defer w.queueMu.Unlock()
		for _, q := range w.removeNPCQueue {
			if q == n {
				return false
			}
		}
		if n.IsRegistered() {
			return true
		}
		for _, q := range w.addNPCQueue {
			if q == n {
				return true
			}
		}
		return false
	})
}

// newNPCRespawnTask returns the respawn task for original
// (NPCRespawnTask.ts:6-21, execute): stop, drop the respawn
// when the lineage retired, clone, claim the unique key (a
// live holder elsewhere keeps the NPC single), and queue the
// clone. The task delay is the respawn delay in ticks (Task
// .ts countdown semantics: the task executes after that many
// Process calls). The task name is the TS class name, which is
// what HasActiveTask matches on.
func newNPCRespawnTask(original *NPC, ticks int, add func(*NPC)) *Task {
	task := NewTask(ticks, func(cycle int64) bool {
		_ = cycle
		// Its source was reloaded or removed: the replacement
		// is already in the world.
		if lineage := original.SpawnLineage(); lineage != nil && lineage.IsRetired() {
			return true
		}
		clone := original.Clone()
		if key, ok := original.UniqueKey(); ok {
			// A unique NPC another spawn already brought
			// back stays single.
			if !ClaimUnique(key, clone) {
				return true
			}
		}
		add(clone)
		return true
	})
	task.Name = "NPCRespawnTask"
	return task
}
