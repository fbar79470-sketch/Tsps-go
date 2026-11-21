package game

import (
	"math"

	"tsps-go/internal/definition"
)

// NPC is the core of game/entity/impl/npc/NPC.ts (1,146 lines in TS;
// only the fields listed here are ported).
//
// Ported core fields:
//   - Mobile core (index, location, flags, facing, timers).
//   - ID: the NPC definition id (NPC.ts: constructor, id field).
//   - Spawn: the spawn position, cloned from the construction location
//     (NPC.ts:136, 193, spawnPosition). SetSpawnPosition moves it
//     (NPC.ts:866-870).
//   - Hitpoints: current hitpoints. ZERO MEANS UNSET in this package: TS
//     initializes hitpoints from the NPC definition, or to 10 when no
//     definition exists (NPC.ts:195-199). NewNPC deliberately does not
//     default to 10 — the spawn path assigns real hitpoints (see
//     spawns.go), and a later milestone that ports definitions must
//     assign real hitpoints. Do not treat 0 as "dead" in this package;
//     death is the Dying flag (see the aggression hitpoints check in
//     npc_aggression.go).
//   - Dying: whether the death sequence has started (NPC.ts: isDying,
//     appendDeath). Death handling itself belongs to the combat core;
//     the respawn half lives in npc_respawn.go.
//   - Visible: whether the NPC is visible to players (NPC.ts:
//     setVisible). Defaults to false (zero value) in this package —
//     spawn code must set it explicitly; do not read "invisible" as
//     a TS default, since TS visibility defaults to true (NPC.ts:167).
//   - Movement queue: every NPC carries a MovementQueue (Mobile.ts:147,
//     constructed in the Mobile constructor), stepped from Process.
//     The queue starts on open ground (nil collision/clipping); the
//     world wiring must point it at the world's RegionManager (see
//     MovementQueue), or NPC steps ignore clipping.
//   - Movement coordinator: wandering, walking home, and the radius-0
//     stationary rule (NPCMovementCoordinator.ts; see
//     npc_movement_coordinator.go).
//   - Face: the spawn-facing idle direction (NPC.ts:168, face,
//     FacingDirection.SOUTH default), distinct from Mobile.Facing,
//     the last direction faced or stepped in. The stationary
//     interaction-clear reset turns the NPC back to Face.
//   - Following / interaction / face-target state: the Mobile
//     following/interaction/positionToFace surface NPC.process and
//     the follow driver rely on (Mobile.ts:340-352, 587-668,
//     911-934; see npc_interaction.go). It lives on NPC, not Mobile:
//     entity.go is owned by another milestone, and only NPCs read it
//     so far.
//   - scriptedMovement: a plugin driving the NPC's walk owns its
//     movement (NPC.ts:171, 1096-1104).
//   - Owner / ownerOnly / multiCombat / pet: spawn scope and combat
//     scope (NPC.ts:160-161, 166, 169).
//   - Spawn lineage / unique key: what spawned the NPC, kept by
//     respawn clones (NPC.ts:162-165; see npc_respawn.go).
//   - Definition: the NPC definition surface the drivers read
//     (size, aggression, respawn, hitpoints). Nil means the TS
//     DEFAULT shape (NpcDefinition.forId never returns null); see
//     NPCDefinition.
//
// NOT ported (absent on purpose): headbars,
// exact-move/telejump/transparency/model state, faceTileState,
// animations/graphics, prayers/poison, combat-override views,
// stat drains, and all NPC subclass overrides (Systems is NOT
// STARTED). SetCombatFollowing is not ported either: it needs
// Combat.Attack/Reset, which the combat-core work owns (see
// npc_interaction.go). Transformations ARE ported (Phase 33):
// TransformationID + ContentID + SetNpcTransformationID
// (Mobile.ts:715-722, NPC.ts:694-696, 834) and the
// hitpoints lock (NPC.ts:175, 1107-1113).
type NPC struct {
	Mobile

	ID    int
	Spawn Location

	// Hitpoints is unset (0) until the spawn path or a definition
	// assigns it. See above.
	Hitpoints int

	Dying   bool
	Visible bool

	// ProcessHook, when set, runs after the timer phase in Process,
	// before the movement/combat phases. It is the extension point
	// for NPC mechanics the ported phases do not cover. Existing
	// tests observe the process_npcs phase through it; Process must
	// keep firing it exactly once per turn.
	ProcessHook func(n *NPC, cycle int64)

	// areaManager is the world's AreaManager, installed by World
	// when the NPC is added (see drainAddNPCs). Nil for a
	// standalone NPC: Process skips area processing.
	// Tick-goroutine-only.
	areaManager *AreaManager

	// movementQueue is the NPC's walk queue (Mobile.ts:147). NewNPC
	// always installs one; a zero-value NPC has none, and Process
	// guards for that, as Player.Process does. Tick-goroutine-only.
	movementQueue *MovementQueue

	// coordinator is the NPC's movement coordinator
	// (NPC.ts:126, constructed in the field initializer). NewNPC
	// always installs one; a zero-value NPC has none, and Process
	// guards for that. Tick-goroutine-only.
	coordinator *NPCMovementCoordinator

	// definition is the installed NPC definition, or nil for the
	// DEFAULT shape (see NPCDefinition). Installed once by the
	// spawn path; read on the tick goroutine thereafter.
	definition NPCDefinition

	// face is the spawn-facing idle direction (NPC.ts:168). NewNPC
	// defaults it to south, the TS field initializer.
	face Direction

	// movementSteps is how many steps the NPC takes per tick when
	// running (NPC.ts:170, 668-674, clamped to 1..2). NewNPC sets
	// 1. The Go MovementQueue does not honor it yet (IsRunToggled
	// reads non-players as walking), so a value of 2 currently
	// has no effect on stepping; the field is ported so the queue
	// follow-up has the value to read.
	movementSteps int

	// scriptedMovement reports whether a plugin owns this NPC's
	// walk (NPC.ts:171). While set, the coordinator never runs
	// (no wandering, no walking home) and combat leaves the queue
	// alone; queued steps still process. See SetScriptedMovement.
	scriptedMovement bool

	// heldByDialogue is the player whose dialogue holds this NPC
	// still, or nil (NPC.ts:173, heldByDialogue). Maintained by
	// updateDialogueHold; see npc_interaction.go.
	heldByDialogue *Player

	// interacting is the Mobile this NPC faces through entity
	// interaction (Mobile.ts:113, interactingMobile), plus the
	// typed identity when the target is a player or an NPC. See
	// npc_interaction.go.
	interacting mobileTarget

	// interactionGeneration is bumped by every SetMobileInteraction
	// (Mobile.ts:114-115), so a delayed clear can tell it was
	// overtaken. No delayed clear exists in Go yet; the counter
	// is maintained for the same reason TS maintains it.
	interactionGeneration int

	// positionToFace is the tile this NPC turns toward (Mobile.ts:88,
	// positionToFace), or nil for none. See npc_interaction.go.
	positionToFace *Location

	// following is the Mobile this NPC follows (Mobile.ts:93,
	// following), plus the typed identity when known. See
	// npc_interaction.go.
	following mobileTarget

	// owner is the player who owns this NPC (a pet, a thrall, a
	// quest instance), or nil (NPC.ts:160).
	owner *Player

	// ownerOnly reports whether only the owner may see or interact
	// with this NPC (NPC.ts:161, 747-754).
	ownerOnly bool

	// multiCombat reports whether fights with this NPC follow
	// multi-combat rules wherever it stands (NPC.ts:166, 762-766).
	multiCombat bool

	// pet reports whether this NPC is a pet (NPC.ts:169).
	pet bool

	// keepInteraction is the Go form of the "interaction:keep"
	// entity flag (NPC.ts:344): a scripted NPC keeps facing its
	// interaction target at any range. Go has no string-flag set
	// on Mobile, so the opt-in is an explicit field, following
	// the MovementQueue ignoreClipping precedent. Set by boss
	// content in TS (AbyssalSire, Nex, Zulrah, Hunllef); no core
	// code sets it.
	keepInteraction bool

	// spawnLineage is what spawned this NPC; respawn clones keep
	// it (NPC.ts:162-163). See npc_respawn.go.
	spawnLineage *NpcSpawnLineage

	// uniqueKey is the uniqueness key held while this NPC is live,
	// or "" when none is held (NPC.ts:165). hasUniqueKey
	// distinguishes "no key" from a held empty key.
	uniqueKey    string
	hasUniqueKey bool

	// skipDefaultRespawn is the Go form of the __skipDefaultRespawn
	// marker NPCDeathTask.remove reads and clears (NPCDeathTask.ts:
	// 94-95): a spawn stood down in favor of a replacement queues
	// no respawn. See BeginRespawn.
	skipDefaultRespawn bool

	// transformationID is the NPC transformation id
	// (Mobile.ts:715-722, npcTransformationId, -1 for none):
	// the id the client renders and content keys on, while ID
	// stays the spawned definition id. hasTransformation
	// distinguishes "set" from the zero value, so a zero-value
	// NPC (never through NewNPC) reads untransformed. Set
	// through SetNpcTransformationID, which flags APPEARANCE
	// like TS; the sync encoder re-adds the NPC under the new
	// type id when it changes (protocol/sync.go EncodeNpcSync).
	transformationID  int
	hasTransformation bool

	// hitpointsLocked is the NPC hitpoints lock (NPC.ts:175,
	// 1107-1113, hitpointsLocked): a locked NPC shows hits but
	// keeps its hitpoints (Mobile.ts:732-735, decrementHealth).
	// DecrementHealth enforces it; content sets it through
	// SetHitpointsLocked.
	hitpointsLocked bool
}

// NPCDefinition is the NPC-definition surface the drivers read.
// *definition.NpcDefinition satisfies it directly (see the assertion
// in spawns.go), so the spawn path passes Store.NPC results with no
// adapter. A nil definition on an NPC reads as the DEFAULT shape
// (see defaultNPCDefinition): TS NpcDefinition.forId never returns
// null, so driver code never branches on a missing definition.
//
// The method set is exactly what the drivers read: Size for
// movement/footprints, the aggression trio plus AggressionRange
// and CombatLevel for NpcAggression, Respawn for BeginRespawn,
// Hitpoints for respawn clones, and AttackType for the combat
// method's reach (see attackDistance). Combat stats, animations,
// and the rest of NpcDefinition belong to the combat core, not
// to this interface.
type NPCDefinition interface {
	Size() int
	IsAggressive() bool
	BuildsAggressionTolerance() bool
	CombatFollowDistance() int
	AggressionRange(attackDistance float64) int
	HuntRange() int
	CombatLevel() int
	Respawn() int
	Hitpoints() int
	AttackType() definition.CombatType
}

// defaultNPCDefinition is the DEFAULT NpcDefinition shape
// (NpcDefinition.ts field initializers, mirrored by
// newDefaultNpcDefinition in internal/definition): size 1,
// non-aggressive, builds tolerance, combat follow distance 7,
// hunt range 3, combat level 0, respawn 25 ticks, 10 hitpoints,
// melee attack type. It is what an NPC with no installed
// definition reads, exactly as TS forId yields DEFAULT for an
// out-of-range id.
type defaultNPCDefinition struct{}

func (defaultNPCDefinition) Size() int { return 1 }
func (defaultNPCDefinition) IsAggressive() bool {
	return false
}
func (defaultNPCDefinition) BuildsAggressionTolerance() bool { return true }
func (defaultNPCDefinition) CombatFollowDistance() int       { return 7 }
func (defaultNPCDefinition) AggressionRange(attackDistance float64) int {
	// NpcDefinition.ts:332-335 (getAggressionRange): a non-finite
	// attack distance counts as 1, the reach floors at 1, and a
	// negative follow distance counts as 0.
	reach := 1
	if !math.IsNaN(attackDistance) && !math.IsInf(attackDistance, 0) {
		if truncated := int(attackDistance); truncated > 1 {
			reach = truncated
		}
	}
	return 7 + reach
}
func (defaultNPCDefinition) HuntRange() int   { return 3 }
func (defaultNPCDefinition) CombatLevel() int { return 0 }
func (defaultNPCDefinition) Respawn() int     { return 25 }
func (defaultNPCDefinition) Hitpoints() int   { return 10 }
func (defaultNPCDefinition) AttackType() definition.CombatType {
	return definition.CombatTypeMelee
}

// NewNPC returns an NPC with the given definition id at loc, with Spawn
// set to loc, a movement queue and coordinator installed, face south,
// and movementSteps 1. NPC.ts:190-199 (constructor; hitpoints
// assignment omitted — see Hitpoints above) plus the coordinator
// field initializer (NPC.ts:126) and the Mobile movement-queue
// construction (Mobile.ts:147).
func NewNPC(id int, loc Location) *NPC {
	n := &NPC{
		Mobile:        NewMobile(loc),
		ID:            id,
		Spawn:         loc,
		face:          DirSouth,
		movementSteps: 1,
	}
	n.movementQueue = NewMovementQueue(n, nil, nil)
	n.coordinator = NewNPCMovementCoordinator(n)
	return n
}

// MovementQueue returns the NPC's walk queue. It is never nil for an
// NPC built with NewNPC. Tick-goroutine-only, like the queue itself
// (see movement_queue.go). The world wiring uses it to point the
// queue at the world's RegionManager (SetCollision/SetClipping);
// until then the queue steps on open ground.
func (n *NPC) MovementQueue() *MovementQueue { return n.movementQueue }

// MovementCoordinator returns the NPC's movement coordinator. It is
// never nil for an NPC built with NewNPC (NPC.ts:824,
// getMovementCoordinator). The spawn path sets its radius through
// it; content reads its coordinate state through it.
func (n *NPC) MovementCoordinator() *NPCMovementCoordinator { return n.coordinator }

// Definition returns the installed NPC definition, or the DEFAULT
// shape when none is installed (see NPCDefinition). It never
// returns nil, matching NpcDefinition.forId.
func (n *NPC) Definition() NPCDefinition {
	if n.definition != nil {
		return n.definition
	}
	return defaultNPCDefinition{}
}

// SetDefinition installs the NPC definition the drivers read. A nil
// definition clears back to the DEFAULT shape. The spawn path
// installs the Store-backed definition; call it once, before the
// NPC takes a turn.
func (n *NPC) SetDefinition(d NPCDefinition) { n.definition = d }

// ContentID is the id content keys on (NPC.ts:694-696, getId;
// NPC.ts:834, getCurrentDefinition): the transformation id when
// one is set, else the spawned definition id. It is the Go form
// of TS npc.getContentId(player) — the id the interaction
// events, the death event, and the sync TypeID carry — while
// ID stays the spawned id for definition lookups and identity
// checks.
func (n *NPC) ContentID() int {
	if n == nil {
		return 0
	}
	if n.hasTransformation {
		return n.transformationID
	}
	return n.ID
}

// SetNpcTransformationID sets the transformation id
// (Mobile.ts:719-722, setNpcTransformationId) and flags
// APPEARANCE, as TS does. Pass -1 to clear it
// (NPCDeathTask.ts:105, remove). A nil NPC is a no-op.
func (n *NPC) SetNpcTransformationID(id int) {
	if n == nil {
		return
	}
	n.transformationID = id
	n.hasTransformation = id >= 0
	n.Flags = n.Flags.Set(FlagAppearance)
}

// IsHitpointsLocked reports the hitpoints lock (NPC.ts:1111,
// isHitpointsLocked). A nil NPC reads unlocked.
func (n *NPC) IsHitpointsLocked() bool {
	return n != nil && n.hitpointsLocked
}

// SetHitpointsLocked sets the hitpoints lock (NPC.ts:1107,
// setHitpointsLocked). A nil NPC is a no-op.
func (n *NPC) SetHitpointsLocked(locked bool) {
	if n == nil {
		return
	}
	n.hitpointsLocked = locked
}

// attackDistance is the combat method's reach for this NPC
// (CombatMethod.attackDistance), resolved from the definition's
// attack type: melee NPCs reach 1 (MeleeCombatMethod.ts:30-32),
// ranged NPCs 6 (RangedCombatMethod.ts:228-229), magic NPCs 10
// (MagicCombatMethod.ts:317-319). A provider-supplied combat
// method (registerNpcCombatMethodProvider) may reach further;
// that override belongs to the combat factory, and this mapping
// does not consult it — a documented gap until the factory seam
// exists.
func (n *NPC) attackDistance() int {
	switch n.Definition().AttackType() {
	case definition.CombatTypeRanged:
		return 6
	case definition.CombatTypeMagic:
		return 10
	default:
		return 1
	}
}

// Size returns the NPC's footprint in tiles: the definition's size
// (NPC.ts:513, size(): the current definition's size, or 1 when
// there is none — the DEFAULT shape supplies 1 here). It satisfies
// the MovementOwner interface. A non-positive size reads as 1,
// matching the queue's max(size, 1) normalization on its route
// paths; cache sizes are always positive, so the clamp only ever
// guards a hand-built fixture.
func (n *NPC) Size() int {
	if size := n.Definition().Size(); size > 0 {
		return size
	}
	return 1
}

// GetFace returns the spawn-facing idle direction (NPC.ts:1045,
// getFace). It is the direction the stationary interaction-clear
// reset turns the NPC back to, not the direction the NPC currently
// faces (Mobile.Facing).
func (n *NPC) GetFace() Direction { return n.face }

// SetFace sets the spawn-facing idle direction (NPC.ts:1078-1080,
// setFace). An out-of-range direction reads as south, matching the
// TS toFacingDirection default for an unrecognized Direction id
// (NPC.ts:1048-1068). The spawn path sets it from the spawn
// record; content sets it when it re-faces a stationary NPC.
func (n *NPC) SetFace(face Direction) {
	if face < 0 || int(face) > 7 {
		n.face = DirSouth
		return
	}
	n.face = face
}

// SetSpawnPosition moves the NPC's home: where its movement
// coordinator keeps it and walks it back to (NPC.ts:866-870,
// setSpawnPosition). Content that walks an NPC around sets it to
// where it sends the NPC, or the coordinator walks it straight
// back.
func (n *NPC) SetSpawnPosition(position Location) { n.Spawn = position }

// GetMovementSteps returns the steps-per-tick when running
// (NPC.ts:668-670).
func (n *NPC) GetMovementSteps() int { return n.movementSteps }

// SetMovementSteps sets the steps-per-tick when running, clamped
// to 1..2 (NPC.ts:672-674). See the movementSteps field comment:
// the queue does not honor a value of 2 yet.
func (n *NPC) SetMovementSteps(steps int) {
	n.movementSteps = max(1, min(2, steps))
}

// SetScriptedMovement marks the NPC as plugin-driven: while true,
// combat leaves its movement queue alone and does not attack, and
// it neither wanders nor walks home (NPC.ts:1096-1100,
// setScriptedMovement). Plugins driving a scripted walk set this,
// path the NPC themselves, then clear it on arrival to resume
// combat (GemstoneCrab, Leviathan, Sarachnis, and other boss
// content set it; core never does). This setter is the seam the
// hub and listeners use; the coordinator and Process read it.
func (n *NPC) SetScriptedMovement(scripted bool) { n.scriptedMovement = scripted }

// IsScriptedMovement reports whether a plugin owns this NPC's walk
// (NPC.ts:1102-1104).
func (n *NPC) IsScriptedMovement() bool { return n.scriptedMovement }

// Owner returns the player who owns this NPC, or nil (NPC.ts:721,
// getOwner).
func (n *NPC) Owner() *Player { return n.owner }

// SetOwner records the owning player (NPC.ts:724-727, setOwner).
func (n *NPC) SetOwner(owner *Player) { n.owner = owner }

// IsOwnerOnly reports whether only the owner may see or interact
// with this NPC (NPC.ts:747-750, isOwnerOnly).
func (n *NPC) IsOwnerOnly() bool { return n.ownerOnly }

// SetOwnerOnly marks the NPC owner-scoped (NPC.ts:752-755).
func (n *NPC) SetOwnerOnly(ownerOnly bool) { n.ownerOnly = ownerOnly }

// IsMultiCombat reports whether fights with this NPC follow
// multi-combat rules wherever it stands (NPC.ts:762-764).
func (n *NPC) IsMultiCombat() bool { return n.multiCombat }

// SetMultiCombat marks the NPC always-multi (NPC.ts:766-769).
func (n *NPC) SetMultiCombat(multiCombat bool) { n.multiCombat = multiCombat }

// IsPet reports whether this NPC is a pet (NPC.ts:1073-1075).
func (n *NPC) IsPet() bool { return n.pet }

// SetPet marks the NPC a pet (NPC.ts:1077-1079).
func (n *NPC) SetPet(pet bool) { n.pet = pet }

// KeepInteraction reports the "interaction:keep" opt-in (see the
// keepInteraction field comment).
func (n *NPC) KeepInteraction() bool { return n.keepInteraction }

// SetKeepInteraction sets the "interaction:keep" opt-in: the NPC
// keeps facing its interaction target at any range. It is the Go
// form of setFlag("interaction:keep") (NPC.ts:344).
func (n *NPC) SetKeepInteraction(keep bool) { n.keepInteraction = keep }

// canMove is the coordinator/follow pre-check: the Go form of
// getMobility().canMove() (MovementQueue.ts:337-342) reduced to
// the gates Go can produce — the blockMovement hold and the
// FREEZE/STUN timers (see MovementQueue.canMove for the per-gate
// audit). The queue re-checks authoritatively inside Process;
// this pre-check only decides whether the coordinator acts at
// all. It reads the timer repository without allocating one.
func (n *NPC) canMove() bool {
	if q := n.movementQueue; q != nil && q.IsMovementBlocked() {
		return false
	}
	if repo := n.timerRepository(false); repo != nil {
		if repo.Has(TimerKeyFreeze) || repo.Has(TimerKeyStun) {
			return false
		}
	}
	return true
}

// npcInCombat reports whether the NPC is in combat, as
// CombatFactory.inCombat reads it (CombatFactory.ts:1237-1239:
// attacking or being attacked). The Go form is the combat
// state's HasPendingWork (Combat.ts:567-568: a target, an
// attacker, or queued hits) — the only combat signal the stub
// exposes. It over-reads by exactly the queued-hits-without-a-
// target case: hits still in flight after a fight ends count as
// combat here. The coordinator's inCombat seam (see
// npc_movement_coordinator.go) lets the combat wiring install
// the exact predicate once Combat exposes target/attacker.
func npcInCombat(n *NPC) bool {
	return n.GetCombat().HasPendingWork()
}

// Process runs this NPC's turn for the given cycle, in the TS
// phase order (NPC.process(), NPC.ts:305-398):
//
//	hit queue → timers → ProcessHook → beginCycle → dialogue hold
//	→ movement coordinator (unless scripted movement or dialogue
//	hold) → combat pre-movement → follow + movement queue (unless
//	dialogue hold) → combat post-movement → interaction/facing
//	maintenance → area processing.
//
// The outer `getDefinition() != null` guard (NPC.ts:306) is
// always true in TS (forId never returns null) and has no Go
// form: Definition never returns nil either. The hitpoints
// regeneration block after area processing (NPC.ts:382-392) is
// NOT ported: it reads the combat last-attack stopwatch, which
// the combat stub does not expose; it belongs with the combat
// core. The combat phases call the Combat stub methods (see
// combat.go) and are unobservable until the combat core lands.
//
// A zero-value NPC (no queue, no coordinator) runs only the
// timer, hook, interaction, and area phases; NewNPC always
// installs both, so production NPCs run the full order.
func (n *NPC) Process(cycle int64) {
	combat := n.GetCombat()
	// Queued impacts land before this NPC acts (NPC.ts:308).
	combat.ProcessHitQueue(cycle)
	n.ProcessTimers(cycle)
	if n.ProcessHook != nil {
		n.ProcessHook(n, cycle)
	}
	queue := n.movementQueue
	if queue != nil {
		queue.BeginCycle()
	}
	heldByDialogue := n.updateDialogueHold()
	// A plugin driving this NPC's walk owns its movement: no
	// wandering or walking home (NPC.ts:314-315).
	if !n.scriptedMovement && !heldByDialogue && n.coordinator != nil {
		n.coordinator.Process(cycle)
	}
	processCombat := combat.HasPendingWork()
	if processCombat {
		combat.PreMovementProcess()
	}
	// A held NPC keeps its walk queued and finishes it once the
	// conversation ends (NPC.ts:320-323). The pending-work gate
	// adds the follow state TS MovementQueue.hasPendingWork
	// includes (MovementQueue.ts:548) but the Go queue cannot
	// see: following lives on the NPC (see npc_interaction.go).
	// The follow driver runs inside this gate, in the position
	// MovementQueue.process runs processFollowing: before
	// stepping, in the same movement phase (MovementQueue.ts:
	// 580-586).
	if queue != nil && (queue.HasPendingWork() || n.following.present()) && !heldByDialogue {
		n.processFollowing()
		queue.Process(cycle)
	}
	if processCombat {
		combat.PostMovementProcess()
	}
	n.clearInteractionIfNeeded(heldByDialogue)
	if n.areaManager != nil {
		n.areaManager.Process(n)
	}
}

// npcInteractRadius is MovementQueue.NPC_INTERACT_RADIUS
// (MovementQueue.ts:64): an interaction target further than
// this (Chebyshev, same plane) is out of range, unless the
// tracking guard holds it.
const npcInteractRadius = 2

// clearInteractionIfNeeded ports the interaction/facing
// maintenance block (NPC.ts:328-376): an interaction target
// that unregistered, or that moved out of range while the NPC
// is not intentionally tracking it, is cleared; a stationary
// NPC (radius 0) then turns back to its spawn-facing idle
// direction, and a wandering NPC clears its face target.
//
// The tracking guard (NPC.ts:341-346) holds the interaction
// when the NPC follows the target, combat-follows it, has it
// as a combat target, carries "interaction:keep", or is held
// by its dialogue. getCombatFollowing and getTarget are the
// same value (Mobile.ts:932-934, getCombatFollowing delegates
// to getTarget); both disjuncts are kept, as in TS.
//
// The missing-target-location branch (NPC.ts:331-333, 347-350)
// has no Go form: an interaction target is always a Mobile
// with a location here, so that disjunct is unreachable and
// only the unregistered and out-of-range branches remain.
// A cross-plane target reads as out of range: Location.Distance
// returns -1 across planes, where TS getDistance returns
// MAX_SAFE_INTEGER — both exceed the radius.
func (n *NPC) clearInteractionIfNeeded(heldByDialogue bool) {
	target := n.interacting
	if !target.present() {
		return
	}
	targetLoc, _ := target.location()
	distance := n.Location().Distance(targetLoc)
	outOfRange := distance < 0 || distance > npcInteractRadius
	tracking := n.following.equals(target) ||
		n.GetCombatFollowing() == target.mobile ||
		n.GetCombat().Target() == target.mobile ||
		n.keepInteraction ||
		heldByDialogue
	targetUnregistered := !target.registered()
	if !targetUnregistered && !(outOfRange && !tracking) {
		return
	}
	n.SetMobileInteraction(nil)
	if n.coordinatorRadius() == 0 {
		// OSRS-like behavior for stationary NPCs: after an
		// interaction ends, they return to their spawn-facing
		// idle direction (NPC.ts:362-371).
		face := n.face
		adjacent := n.Location().Transform(face.Dx(), face.Dy())
		n.SetPositionToFace(&adjacent)
	} else {
		n.SetPositionToFace(nil)
	}
}

// coordinatorRadius returns the movement coordinator's wander
// radius, or 0 when there is no coordinator (a zero-value NPC):
// the TS coordinator always exists with radius 0 by default
// (NPCMovementCoordinator.ts:15), so 0 is the faithful answer
// for a missing one.
func (n *NPC) coordinatorRadius() int {
	if n.coordinator == nil {
		return 0
	}
	return n.coordinator.GetRadius()
}

// IsPlayer reports false: an NPC is never a player. It completes the
// AreaActor predicate pair (Area.ts:28, 41, 53-56 tests both) and
// satisfies the MovementOwner interface.
func (n *NPC) IsPlayer() bool { return false }

// IsNPC reports true. See IsPlayer.
func (n *NPC) IsNPC() bool { return true }

// AsPlayer returns nil: an NPC has no player identity (getAsPlayer
// on an NPC is never a member of the player map).
func (n *NPC) AsPlayer() *Player { return nil }

// AsNPC returns n: the typed identity Area stores in its NPC member
// map (getAsNpc).
func (n *NPC) AsNPC() *NPC { return n }

// MovementOwner conformance: the queue's owner surface, reduced to
// what an NPC can answer. The queue's player-only branches (run
// toggling, run-energy drain, region-change handling) never fire
// for a non-player owner — IsRunToggled answers false for
// non-players and Process gates the drain on IsPlayer — so the
// run-energy and messaging methods below are never called with
// live effect; they exist only to satisfy the interface, with the
// values TS would imply (an NPC never runs, never tires, and has
// no session to send packets to). Location, SetLocation,
// SetWalkingDirection, SetRunningDirection, GetTimers, and the
// LastKnownRegion accessors are promoted from Mobile.
var _ MovementOwner = (*NPC)(nil)

// IsPlayerBot reports false: an NPC is never a player bot.
func (n *NPC) IsPlayerBot() bool { return false }

// IsRunning reports false: NPCs have no run setting (TS
// isRunningReturn is a Player method; NPC extra steps come from
// movementSteps, which the queue does not honor yet).
func (n *NPC) IsRunning() bool { return false }

// SetRunning is a no-op: the queue calls it only from the
// player-gated run-energy drain, which never runs for an NPC.
func (n *NPC) SetRunning(running bool) {}

// GetRunEnergyUnits reports full energy: NPCs have no run-energy
// state and never tire. The queue's drain never reads it for a
// non-player owner.
func (n *NPC) GetRunEnergyUnits() int { return RunEnergyMaxUnits }

// SetRunEnergyUnits is a no-op: see GetRunEnergyUnits.
func (n *NPC) SetRunEnergyUnits(units int) {}

// GetCurrentAgilityLevel reports 0: NPCs have no Agility level,
// and the queue's drain never reads it for a non-player owner.
func (n *NPC) GetCurrentAgilityLevel() int { return 0 }

// GetWeightKg reports 0: NPCs carry nothing, and the queue's
// drain never reads it for a non-player owner.
func (n *NPC) GetWeightKg() int { return 0 }

// SendRunEnergy is a no-op: an NPC has no session, and the queue
// never sends run energy for a non-player owner.
func (n *NPC) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {}

// SendDestination is a no-op: the destination flag is a
// player-client packet, and an NPC has no session. The queue
// syncs it on route changes and resets; for an NPC the sync
// records state and sends nothing.
func (n *NPC) SendDestination(x, y int) {}

// SendMessage is a no-op: an NPC has no session. The queue calls
// it only from the walk-to unreachable path, which NPCs never
// take (no walk-to interactions are ever submitted for an NPC).
func (n *NPC) SendMessage(text string) {}

// SendTypedMessage is a no-op: an NPC has no session. The queue
// calls it only from the STUNNED mobility refusal, which the
// NPC paths never surface to a client.
func (n *NPC) SendTypedMessage(chatType int, message, from string) {}

// ResetUpdating clears per-tick update state via Mobile.ResetUpdating.
// TS NPC.resetUpdating (NPC.ts:1009-1016) additionally clears exact-move
// state, headbars, face-tile state, and transparency state — none of
// which exist in this package yet. Interaction, following, and
// face-target state deliberately survive: TS Mobile.resetUpdating
// does not clear them either.
func (n *NPC) ResetUpdating() { n.Mobile.ResetUpdating() }
