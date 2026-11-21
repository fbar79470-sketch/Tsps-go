package game

import "time"

// MovementQueue is the core subset of game/model/movement/MovementQueue.ts
// (2,058 lines in TS): the checkpoint deque, requestWalk routing, the
// per-tick stepping loop, run/click-run overrides, and region-change
// bookkeeping. The queue is owner-generic over MovementOwner; this phase
// wires it to Player only (see Player.MovementQueue). NPCs get no queue:
// no NPC drivers exist yet (see npc.go).
//
// Everything in this file is tick-goroutine-only, like visibility.go:
// RequestWalk runs from the turn-packet drain inside Player.Process,
// Process runs from Player.Process, and no other goroutine may call
// these methods or read the queue's mutable state. (The MovementRequest
// itself crosses goroutines through the turn queue, as before; the
// queue only ever acts on the tick.)
//
// Ported from MovementQueue.ts (line numbers cite commit bfd81e3):
// points deque (cap 100, MAXIMUM_SIZE, line 74), addCheckpoint (312),
// reset (864), checkDestination (384), requestWalk (400),
// getDestination (435), syncDestinationFlagToRoute (446), beginCycle
// (473), process (554), canWalkTo (670), validatedStep (738),
// handleRegionChange (771), drainRunEnergy (817), setClickRunOverride /
// dropPendingClickRunOverride / endClickRunOverride /
// endClickRunOverrideWhenIdle (1117-1141), isRunToggled (1143),
// setBlockMovement + generation (1149-1164), setPathX/setPathY (1173,
// 1178), setRoute/hasRoute/wasRouteEvaluated/usedAlternativeRoute
// (1185-1201), setExternalMovement (535), hasPendingWork (543), and
// the applyRsmodRoute queue-mutation half of PathFinder.ts:128-208
// (reset, lastDestX/lastDestY, setRoute, setPathX/setPathY,
// addCheckpoint per waypoint, destination-flag sync) via WalkRoute.
// The walk-to subset (phase 9, listeners): walkToTile (1262),
// walkToGroundItem (1234) + reachedGroundItem (1245), the MovementTask
// closure (1290-1329) ticked from Process, and the walk-to and
// stopSkillable halves of walkToReset (1334) — see WalkToTile
// for the exact subset and its degenerations. The walk-to-object subset (phase 13): walkToObject
// (1742) + buildObjectRouteSpec (1348) + the MovementTask closure
// (1825-1884) ticked from Process, the reachedObject wrapper
// (PathFinder.ts:229) via ReachedAbsolute, and the calculateObjectRoute /
// calculateRoute findRoute shape (PathFinder.ts:463-490) via
// applyObjectRoute — see WalkToObject for the exact subset and its
// degenerations. The walk-to-entity subset (phase 17): walkToEntity
// (1478) at the default range + the MovementTask closure (1564) with
// the entityPursuit re-path (1589) folded into the post-step tick,
// the reachedEntity wrapper (PathFinder.ts:214) via ReachedAbsolute,
// and the calculateEntityRoute findRoute shape (PathFinder.ts:265)
// via applyEntityRoute — see WalkToEntity for the exact subset and
// its degenerations. The projected-NPC subset (phase 21):
// walkToProjectedNpc (1619) + isProjectedMainWorldNpc (1715) +
// reachedProjectedNpc (1724), the boat-deck branch of walkToEntity,
// ticked from Process through the same walk-to dispatch — see
// WalkToEntity for the exact subset and its degenerations.
//
// Omitted, each documented again at the site that would call it:
//   - Following / pursuit: processFollowing (896), resetFollow (889),
//     and the mounted-decoration reach helper
//     (isMountedDecorationReachable, 1435). No Following field, no
//     combat, no interaction targets exist in Go. walkToTile,
//     walkToGroundItem, walkToObject, and walkToEntity are ported
//     (see above); processEntityPursuit (1659) is folded into
//     tickWalkToEntity rather than ported as a pre-step hook.
//   - Mobility gates beyond blockMovement, the FREEZE/STUN
//     timers, the trading/duel button delays, and the duel
//     NO_MOVEMENT rule (Phase 7 audit, verified gate by gate
//     against getMobility/getMobilityIgnoringFreeze, 337-376,
//     and the Mobility class, 1969-2024; the timer gates
//     landed with the TimerRepository port, and the
//     trading/duel gates with the Trading/Dueling ports in
//     Phase 17, see canMove): every remaining gate is a
//     documented gap — needs-placement (no setter or teleport
//     path ever sets the field) and force movement (no
//     ForceMovement). The STUNNED refusal line IS ported (a
//     typed spam message through
//     MovementOwner.SendTypedMessage), as are the BUSY and
//     DUEL_MOVEMENT_DISABLED lines (plain messages through
//     MovementOwner.SendMessage); FROZEN_SPELL sends nothing
//     live, so a frozen refusal returns silently, exactly as
//     in TS. No full Mobility type is ported: the mobility
//     enum below carries only the states Go can produce. See
//     canMove for the per-gate audit.
//   - Run-energy drain (drainRunEnergy, 817) and the drain/restore
//     formulas (803-815) ARE ported (see drainRunEnergy and
//     runenergy.go), with identity equipment modifiers, entry-time
//     static weight, and stamina always 0. Restore lives in
//     Player.Process (Player.ts:559-566).
//   - syncWildernessStateForMovedPlayer (839): no Wilderness or
//     PacketSender systems.
//   - The destination-flag packet IS ported (phase 7):
//     syncDestinationFlagToRoute sends through the
//     MovementOwner.SendDestination forward (see destination.go);
//     all bots are excluded, where TS excludes only unwatched
//     bots (no hasNetworkClient distinction exists in Go).
//   - Dynamic occupancy (isDynamicallyOccupied, 715): degenerates to
//     false — no NPC/player occupancy systems exist and the world has
//     zero NPCs. See isDynamicallyOccupied.
//   - Plugin events: emitWalkRoute in requestWalk, emitPathBlocked in
//     applyRsmodRoute, movement:follow-blocked. No plugin system.
//   - addFirstStep (214), walkStep (226), walkStepIgnoringFreeze (237),
//     addStep/addSteps (262, 286), setPursuitCheckpoint (321): see the
//     note above applyWalkRoute for the caller-grounded reasons.
//   - TS debug logging (LOG_ENABLED is false upstream) and ServerPerf
//     phase timing: omitted; no logging/perf infra exists in this
//     package.
//   - The freeze-bypass step (forceOneStepThroughFreeze): its only
//     setter is walkStepIgnoringFreeze, which is omitted, so the
//     branch in process() is absent too.
//   - World.markActiveRegionsDirty in handleRegionChange: no Go
//     equivalent; rebuild detection lives in FlushTick scene-change
//     detection (see HandleRegionChange).
//   - World.onPlayerMoved/onNpcMoved in setLocation: NOT STARTED; see
//     Mobile.SetLocation.

// MovementOwner is the minimal surface MovementQueue calls on its
// owner: position, footprint, step directions, the timer
// repository the mobility gates read, the player/run state
// isRunToggled and requestWalk need, and the LastKnownRegion
// bookkeeping handleRegionChange maintains. *Player satisfies it
// (see the conformance assertion below); no other owner exists this
// phase. The methods are the Go forms of the Mobile/Player accessors
// MovementQueue.ts touches (getLocation, setLocation, getSize,
// setWalkingDirection, setRunningDirection, getTimers, isPlayer,
// isRunningReturn, getLastKnownRegion, setLastKnownRegion).
type MovementOwner interface {
	// Location returns the owner's current tile.
	Location() Location
	// SetLocation moves the owner to loc.
	SetLocation(loc Location)
	// Size is the owner's footprint in tiles (1 for a player).
	Size() int
	// SetWalkingDirection records this tick's walking step.
	SetWalkingDirection(d Direction)
	// SetRunningDirection records this tick's running step.
	SetRunningDirection(d Direction)
	// GetTimers returns the owner's timer repository. The
	// mobility gates read it (getMobility checks FREEZE, the
	// freeze-ignoring check reads STUN). Mobile.ts:963-965
	// (getTimers). A nil repository answers every Has check
	// false (see timers.go), so an owner without timers
	// never reports frozen or stunned.
	GetTimers() *TimerRepository
	// IsPlayer reports whether the owner is a player (the queue's
	// requestWalk gate and isRunToggled player branch).
	IsPlayer() bool
	// IsPlayerBot reports whether the owner is a player bot. The
	// queue's bot branches (energy pinning, destination-flag
	// suppression) key off it. TS: isPlayerBot/isUnwatchedBot.
	IsPlayerBot() bool
	// IsRunning reports the owner's run setting (the isRunToggled
	// fallback when no override applies).
	IsRunning() bool
	// SetRunning sets the owner's run setting. The queue calls it
	// when run energy hits zero (drainRunEnergy auto-stop,
	// MovementQueue.ts:832). Player.ts:1160-1163 (setRunning).
	SetRunning(running bool)
	// GetRunEnergyUnits returns the owner's live run energy in OSRS
	// units (0..10,000). The queue's drain reads it.
	// Player.ts:1267-1269 (getRunEnergyUnits).
	GetRunEnergyUnits() int
	// SetRunEnergyUnits sets the owner's live run energy in OSRS
	// units, clamped to 0..10,000. The queue's drain subtracts
	// through it and pins bots to max through it.
	// Player.ts:1271-1273 (setRunEnergyUnits).
	SetRunEnergyUnits(units int)
	// GetCurrentAgilityLevel returns the owner's Agility current
	// level, sanitized (negative -> 0). The queue's drain/restore
	// formulas read it. SkillManager.ts:425-433 (getCurrentLevel).
	GetCurrentAgilityLevel() int
	// GetWeightKg returns the owner's carried weight in whole
	// kilograms. The queue's drain reads it. Player.ts:1314-1316
	// (getWeightKg); in Go the value is computed live from the
	// owner's containers on every read (see Player.GetWeightKg).
	GetWeightKg() int
	// SendRunEnergy forwards the run-energy orb packet to the
	// owner's session, if any. The queue calls it when energy
	// drains, restores, or auto-stops running (TS
	// PacketSender.sendRunEnergy/sendRunStatus). Implementations
	// must tolerate a missing session: session-less owners drop
	// the packet silently.
	SendRunEnergy(units int, running bool, weightKg int, staminaTicks int)
	// SendDestination forwards the destination-flag packet to the
	// owner's session, if any. The queue calls it from
	// syncDestinationFlagToRoute (TS PacketSender.sendDestination).
	// Like SendRunEnergy, it must tolerate a missing session.
	SendDestination(x, y int)
	// SendMessage forwards a game chat message to the owner's
	// session, if any. The queue calls it from the walk-to
	// unreachable path (TS this.player.sendMessage,
	// MovementQueue.ts:1316). Like SendRunEnergy, it must
	// tolerate a missing session. Player.SendMessage applies
	// the Mobile.sendMessage guards; see message.go.
	SendMessage(text string)
	// SendTypedMessage forwards a game chat message on the
	// given chatbox channel to the owner's session, if any.
	// The queue calls it only from the STUNNED mobility
	// refusal (TS Mobility.sendMessage,
	// MovementQueue.ts:2007-2010, ChatType.SPAM). chatType is
	// a protocol ChatType value restated in this package (see
	// chatTypeSpam); from is "" for the stun line. Like
	// SendMessage, it must tolerate a missing session.
	// Player.SendTypedMessage applies no bot guard, matching
	// the TS direct PacketSender call; see message.go.
	SendTypedMessage(chatType int, message, from string)
	// GetLastKnownRegion returns the sync baseline region.
	GetLastKnownRegion() Location
	// SetLastKnownRegion records the sync baseline region.
	SetLastKnownRegion(region Location)
}

var _ MovementOwner = (*Player)(nil)

// movementPoint is one queued checkpoint: the Go form of the Point
// class at the end of MovementQueue.ts (position + direction).
// Checkpoints enqueued by addCheckpoint carry Direction NONE; the
// stepping loop recomputes the step direction from deltas each tick
// (process, MovementQueue.ts:618-621), so the stored direction is
// informational only.
type movementPoint struct {
	position  Location
	direction Direction
}

// maxQueuePoints is the checkpoint deque cap.
// MovementQueue.ts:74: MAXIMUM_SIZE = 100.
const maxQueuePoints = 100

// destUnsent is the lastSentDestX/Y sentinel for "no destination flag
// sent yet": the Go form of Number.MIN_SAFE_INTEGER in
// MovementQueue.ts:124-125. No real tile (0..32767, or the (0, 0)
// empty-route value) can equal it.
const destUnsent = -1 << 60

// externalMovementWindow is how long setExternalMovement reports
// movement after being told the owner is moving externally.
// MovementQueue.ts:536: Date.now() + 250.
const externalMovementWindow = 250 * time.Millisecond

// MovementQueue holds one owner's walk state. See the file comment
// for the ported subset and the omissions. The zero value is not
// usable: build queues with NewMovementQueue (which installs the
// owned RouteFinder and the -1 follow/region sentinels).
type MovementQueue struct {
	owner     MovementOwner
	collision MovementCollision
	clipping  CollisionLookup
	boats     *BoatManager // The world's boat manager, or nil
	// in worlds without sailing: the deck walk-to branch
	// projects onto the main world through it. Installed by
	// the World (NPC queues) and the coordinator (player
	// queues) alongside collision/clipping, before ticking.
	routeFinder *RouteFinder // Lazily allocated: a RouteFinder
	// holds ~164KB of search scratch, and most queues never
	// route (NPC wander uses checkpoints; only walk clicks,
	// walk-home, and combat close-in search). Eagerly
	// allocating one per queue cost ~5GB for the 30k NPC
	// spawns and OOM-killed the server at boot (Phase 16
	// finding); finder() builds it on first use.

	points []movementPoint

	blockMovement           bool
	blockMovementGeneration int

	isMoving      bool
	movedCycle    bool // movedThisCycle in TS
	ranCycle      bool // ranThisCycle in TS
	lastMoveCycle int64

	externalMovementUntil time.Time

	blockedByDynamicOccupancy bool

	foundRoute       bool
	routeEvaluated   bool
	alternativeRoute bool

	stepBlocked         bool
	stepBlockedByEntity bool

	lastSentDestX int
	lastSentDestY int

	// runOverride is the Ctrl-click run choice for the current path
	// only; nil follows the run setting / clickRunOverride.
	// MovementQueue.ts:127.
	runOverride *bool
	// clickRunOverride is an interaction click's run choice, holding
	// through re-paths until idle or the next click; nil follows the
	// run setting. MovementQueue.ts:133.
	clickRunOverride *bool
	// pendingClickRunOverride is this turn's interaction click's run
	// choice until its walk starts. It is a three-state value in TS
	// (boolean | null | undefined, MovementQueue.ts:135): undefined
	// means "no click this turn", null means "follow the run
	// setting". Go carries it as a *bool plus a set flag: set=false
	// is undefined, set=true with a nil pointer is null.
	pendingClickRunOverride    *bool
	pendingClickRunOverrideSet bool

	// ignoreClipping is the Go form of the IGNORE_CLIPPING_FLAG
	// entity flag (MovementQueue.ts:76, "movement:ignore-clipping"):
	// steps go straight on, whatever the map's clipping. Go has no
	// string-flag set on Mobile, so the queue carries the opt-in as
	// an explicit field, set with SetIgnoreClipping. It is consulted
	// only by validatedStep.
	ignoreClipping bool

	// walkTo is the pending walk-to interaction, if any: the Go form
	// of a submitted MovementTask (MovementQueue.ts:2042-2058,
	// TaskType.WALK_TO). TS holds walk-to tasks in the global
	// TaskManager keyed by player index and ticks them from the
	// owner's turn after movement (TaskManager.processWalkTo,
	// TaskManager.ts:71-108); Go holds at most one per queue and
	// ticks it at the end of Process (see tickWalkTo). At most one
	// can be pending because every walk-to starts with walkToReset,
	// which cancels the previous one (MovementQueue.ts:1334-1344).
	// Nil means no interaction is walking. Tick-goroutine-only.
	walkTo *walkToState

	// FollowX/FollowY are the tile the owner stepped FROM on its
	// most recent step (-1 when no step has been taken or the queue
	// was reset). MovementQueue.ts:463-464 (followX/followY, public
	// fields read by following leaders). No follower reads them in
	// Go yet; they are maintained for the same reason TS maintains
	// them and are exported so a future following port can read
	// them without a queue change.
	FollowX int
	FollowY int

	// LastDestX/LastDestY are the last routed destination, set by
	// the route-application step before routing (PathFinder.ts:149-
	// 150). PathX/PathY are the route end after setPathX/setPathY
	// apply the region-base math (PathFinder.ts:198-199,
	// MovementQueue.ts:1173-1180). All four are public fields in TS
	// (MovementQueue.ts:1167-1170); they are exported here for the
	// same reason.
	LastDestX int
	LastDestY int
	PathX     int
	PathY     int
}

// NewMovementQueue returns a queue for owner, stepping against
// collision and routing through a lazily allocated RouteFinder
// reading clipping (see the routeFinder field comment). A nil
// collision means open ground (TS missing-region semantics: zero
// flags everywhere — see MovementCollision in collision.go); a
// nil clipping lookup means open ground for the RouteFinder
// (NewRouteFinder documents the nil case). The wired server
// replaces both with the world's RegionManager via
// SetCollision/SetClipping; until then the queue routes and steps
// on open ground. owner must be non-nil.
func NewMovementQueue(owner MovementOwner, collision MovementCollision, clipping CollisionLookup) *MovementQueue {
	return &MovementQueue{
		owner:         owner,
		collision:     collision,
		clipping:      clipping,
		lastMoveCycle: -2, // MovementQueue.ts:109.
		lastSentDestX: destUnsent,
		lastSentDestY: destUnsent,
		FollowX:       -1,
		FollowY:       -1,
	}
}

// finder returns the queue's RouteFinder, allocating it on first
// use with the clipping lookup SetClipping last installed (or the
// constructor's). Tick-goroutine-only, like the rest of the queue.
func (q *MovementQueue) finder() *RouteFinder {
	if q.routeFinder == nil {
		q.routeFinder = NewRouteFinder(q.clipping)
	}
	return q.routeFinder
}

// SetCollision replaces the collision surface steps are validated
// against. A nil collision means open ground. The wired server calls
// it with the world's RegionManager when the player is added.
func (q *MovementQueue) SetCollision(collision MovementCollision) {
	q.collision = collision
}

// SetClipping replaces the clipping lookup the RouteFinder
// reads, dropping the finder so the next route rebuilds it
// against the new lookup. A nil lookup means open ground. The
// wired server calls it with the world's RegionManager lookup
// when the player is added.
func (q *MovementQueue) SetClipping(clipping CollisionLookup) {
	q.clipping = clipping
	q.routeFinder = nil
}

// SetBoatManager installs the world's boat manager for the deck
// walk-to branch. A nil manager restores the no-boats behavior
// (no projected walk-tos). Install it alongside
// collision/clipping, before ticking starts.
func (q *MovementQueue) SetBoatManager(m *BoatManager) {
	q.boats = m
}

// boatAboard is BoatManager.getBoatAboard through the queue's
// manager, or nil with none installed.
func (q *MovementQueue) boatAboard(mobile *Mobile) *Boat {
	if q.boats == nil {
		return nil
	}
	return q.boats.GetBoatAboard(mobile)
}

// rootLocationOf is BoatManager.rootLocation through the queue's
// manager, or the actor's own tile with none installed.
func (q *MovementQueue) rootLocationOf(mobile *Mobile) Location {
	if q.boats != nil {
		return q.boats.RootLocation(mobile)
	}
	return mobile.Location()
}

// SetIgnoreClipping opts the owner into (or out of) clipping-free
// steps: the Go form of setting the IGNORE_CLIPPING_FLAG entity flag
// (MovementQueue.ts:76). See the ignoreClipping field comment.
func (q *MovementQueue) SetIgnoreClipping(ignore bool) {
	q.ignoreClipping = ignore
}

// chatTypeSpam is ChatType.SPAM (105, ClientProtocol.ts:1112;
// protocol.ChatTypeSpam). The game layer cannot import the
// protocol package, so the value is restated here with its
// cite, following the chatTypeMesbox precedent
// (dialogue_entries.go): SendTypedMessage's chatType argument
// is a protocol ChatType value (message.go).
const chatTypeSpam = 105

// mobility is the Go form of the Mobility class
// (MovementQueue.ts:1969-2024), reduced to the states Go can
// produce: MOBILE, INVALID (from blockMovement only — the
// other INVALID sources are gaps, see canMove), FROZEN_SPELL,
// STUNNED, ANIMATION_STALL (a full combat stall, players
// only), BUSY (a trading/duel button delay unfinished), and
// DUEL_MOVEMENT_DISABLED (in a duel with the NO_MOVEMENT
// rule). The trading/duel states landed with the Trading and
// Dueling ports (phase 17).
type mobility int

const (
	mobilityMobile mobility = iota
	mobilityInvalid
	mobilityFrozenSpell
	mobilityStunned
	mobilityAnimationStall
	mobilityBusy
	mobilityDuelMovementDisabled
)

// canMove answers true only for MOBILE. MovementQueue.ts:
// 1989-1991 (Mobility.canMove).
func (m mobility) canMove() bool { return m == mobilityMobile }

// sendMessage sends the refusal line for this mobility, if TS
// associates one with it. MovementQueue.ts:1998-2023
// (Mobility.sendMessage): STUNNED sends a typed spam line
// ("You're stunned!", ChatType.SPAM); BUSY sends the plain
// line "You cannot do that right now."; DUEL_MOVEMENT_DISABLED
// sends the plain line "Movement has been disabled in this
// duel!"; FROZEN_SPELL, INVALID, and ANIMATION_STALL send
// nothing live — they fall in the switch's default case — so
// a frozen, blocked, or stalled refusal returns silently. The
// stun line goes through SendTypedMessage, the Go form of the
// TS direct PacketSender.sendTypedMessage call, which carries
// no Mobile.sendMessage bot guard (see message.go); the BUSY
// and DUEL lines go through SendMessage, the Go form of
// player.sendMessage.
func (m mobility) sendMessage(owner MovementOwner) {
	switch m {
	case mobilityStunned:
		owner.SendTypedMessage(chatTypeSpam, "You're stunned!", "")
	case mobilityBusy:
		owner.SendMessage("You cannot do that right now.")
	case mobilityDuelMovementDisabled:
		owner.SendMessage("Movement has been disabled in this duel!")
	}
}

// getMobility ports MovementQueue.ts:337-342 (getMobility): a
// live FREEZE timer reports FROZEN_SPELL, checked before any
// other lock; anything else defers to
// getMobilityIgnoringFreeze. A frozen-and-stunned owner
// therefore reports FROZEN_SPELL here, and callers that must
// see past the freeze (WalkToObject) re-check through
// getMobilityIgnoringFreeze.
func (q *MovementQueue) getMobility() mobility {
	if q.owner.GetTimers().Has(TimerKeyFreeze) {
		return mobilityFrozenSpell
	}
	return q.getMobilityIgnoringFreeze()
}

// getMobilityIgnoringFreeze ports the FREEZE/STUN/blockMovement/
// animation-stall/trading-duel subset of MovementQueue.ts:345-376
// (getMobilityIgnoringFreeze), in TS order: a live STUN timer
// reports STUNNED, checked before the INVALID sources; the
// blockMovement hold reports INVALID; an active force movement
// reports INVALID; a player's full combat animation stall reports
// ANIMATION_STALL; an unfinished trading or duelling button delay
// reports BUSY; a duel with the NO_MOVEMENT rule reports
// DUEL_MOVEMENT_DISABLED; anything else is MOBILE. The remaining TS
// check in this method — needsPlacement — reads state the gate
// deliberately does not consult yet; see canMove.
func (q *MovementQueue) getMobilityIgnoringFreeze() mobility {
	if q.owner.GetTimers().Has(TimerKeyStun) {
		return mobilityStunned
	}
	if q.blockMovement {
		return mobilityInvalid
	}
	// MovementQueue.ts:350-351: an active force movement holds the
	// player still (INVALID, silent — see canMove). The interface
	// assertion keeps owners without forced movement (test fakes,
	// NPCs) on the old path; NPCs never glide (TS checks isPlayer
	// first via the player?.getForceMovement() read).
	if q.owner.IsPlayer() {
		if fmOwner, ok := q.owner.(interface{ ForceMovement() *ForceMovement }); ok && fmOwner.ForceMovement() != nil {
			return mobilityInvalid
		}
	}
	// MovementQueue.ts:361-362: a player's full animation
	// stall (Combat.isAnimationStalled) holds movement. The
	// interface assertion keeps owners without combat
	// (test fakes, NPCs) on the old path; NPCs never stall
	// (TS checks isPlayer first).
	if q.owner.IsPlayer() {
		if combatOwner, ok := q.owner.(interface{ GetCombat() *Combat }); ok {
			if combat := combatOwner.GetCombat(); combat != nil && combat.IsAnimationStalled() {
				return mobilityAnimationStall
			}
		}
		// MovementQueue.ts:366-371: the trading/duelling
		// button delays and the duel NO_MOVEMENT rule. The
		// interface assertion keeps owners without trading
		// state (test fakes) on the old path; *Player
		// satisfies it through Trading/Dueling.
		if tradeOwner, ok := q.owner.(interface {
			Trading() *Trading
			Dueling() *Dueling
		}); ok {
			trading := tradeOwner.Trading()
			dueling := tradeOwner.Dueling()
			if trading != nil && dueling != nil {
				if !trading.ButtonDelay().Finished() || !dueling.ButtonDelay().Finished() {
					return mobilityBusy
				}
				if dueling.InDuel() && dueling.RuleSet(DuelRuleNoMovement) {
					return mobilityDuelMovementDisabled
				}
			}
		}
	}
	return mobilityMobile
}

// canMove is the mobility gate: the Go form of
// getMobility().canMove() (MovementQueue.ts:337-342,
// getMobilityIgnoringFreeze at 345-376; Mobility.canMove at
// 1989-1991 answers true only for MOBILE). The FREEZE and
// STUN gates are ported: a live FREEZE or STUN timer stops
// movement, and an expired (0-tick) timer does not, because
// TimerRepository.Has answers ticks > 0. Phase 7 audit
// (verified gate by gate against commit bfd81e3), updated
// for the timer port: every remaining TS gate reads state
// that does not exist in Go, so each is a documented gap —
// never an invented system:
//   - INVALID via needsPlacement (350-351):
//     character.isNeedsPlacement() (Mobile.ts:839-841). The Go field
//     exists (Mobile.NeedsPlacement) and IS now set by production
//     code — MovePlayerTo, the Go form of Mobile.moveTo — but the
//     gate is deliberately not consulted yet: it needs its own
//     verification (a freshly placed player must still refuse
//     same-tick walks exactly as TS does, and the login/bind
//     setters are still unported), so it stays a gap. The field's
//     live use is sync placement (visibility/flush), cleared by
//     ResetUpdating.
//   - INVALID via force movement (350-351):
//     player.getForceMovement() (Player.ts:1203-1205). Ported in
//     Phase 40 (forcemovement.go): an active force movement
//     reports INVALID here, silently, as in TS.
//   - ANIMATION_STALL (361-362):
//     player.getCombat().isAnimationStalled() (Combat.ts:238-240, a
//     full stall only). Ported in Phase 16 (combat.go); the
//     refusal is silent, as in TS (Mobility.sendMessage's
//     default case), and the click is queued, not rejected.
//   - BUSY (366-367): the trading/duelling button delays unfinished
//     (Trading.getButtonDelay, Trading.ts:551;
//     Dueling.getButtonDelay, Duelling.ts:18). Ported in
//     Phase 17 with the Trading/Dueling ports; the refusal
//     line is the plain "You cannot do that right now."
//     (Mobility.sendMessage), sent through
//     MovementOwner.SendMessage.
//   - DUEL_MOVEMENT_DISABLED (370-371): inDuel() with the NO_MOVEMENT
//     rule (Duelling.ts:19, 32). Ported in Phase 17; the
//     refusal line is the plain "Movement has been disabled
//     in this duel!" through MovementOwner.SendMessage.
//   - Mobility.sendMessage (1998-2023), which requestWalk and the
//     walk-to entry points call on refusal: STUNNED sends the
//     typed spam line through MovementOwner.SendTypedMessage
//     (see mobility.sendMessage); BUSY and
//     DUEL_MOVEMENT_DISABLED send their plain lines through
//     MovementOwner.SendMessage; FROZEN_SPELL, INVALID, and
//     ANIMATION_STALL send nothing, so frozen/blocked/stalled
//     refusals return silently, exactly as in TS.
//
// The forced-walk area fallback on refusal (requestWalk, 406-413)
// needs force movement, placement, and Area.handleForcedWalk: also
// absent; see RequestWalk. The other getMobility call sites in TS
// (canWalk, walkStep/addStep, the following/walk-to paths) belong
// to methods omitted from this port, except the walk-to entry
// points ported here (WalkToTile, WalkToObject), which call
// getMobility directly so they can send the refusal line; the Go
// call sites of canMove are RequestWalk (via getMobility),
// AddCheckpoint, and Process. blockMovement behavior is
// unchanged by this audit: TestMobilityGate pins
// requestWalk/addCheckpoint refusal, no-step-while-blocked with
// checkpoints kept, release/resume, and generation bumps.
func (q *MovementQueue) canMove() bool {
	return q.getMobility().canMove()
}

// CanMove is the exported form of canMove for the coordinator's
// move-path interface-close gate: NetworkBuilder.ts:697 closes
// interruptible interfaces on a "move" packet only when
// getMobility().canMove() answers true, so an immobile player's
// click leaves the interface open. See canMove for the ported
// gates and documented gaps.
func (q *MovementQueue) CanMove() bool {
	return q.canMove()
}

// SetBlockMovement holds or releases movement, bumping the
// generation counter every call. MovementQueue.ts:1149-1152.
func (q *MovementQueue) SetBlockMovement(block bool) {
	q.blockMovement = block
	q.blockMovementGeneration++
}

// BlockMovementGeneration returns the generation counter bumped by
// every SetBlockMovement call, so a delayed owner can tell whether
// its hold is still current. MovementQueue.ts:1159-1161
// (getBlockMovementGeneration).
func (q *MovementQueue) BlockMovementGeneration() int {
	return q.blockMovementGeneration
}

// IsMovementBlocked reports the blockMovement hold.
// MovementQueue.ts:1163-1165 (isMovementBlocked).
func (q *MovementQueue) IsMovementBlocked() bool {
	return q.blockMovement
}

// CheckDestination validates a destination: the z level must be
// non-negative and equal to the owner's, and x/y must lie in
// 0..32767. MovementQueue.ts:384-398 (checkDestination).
func (q *MovementQueue) CheckDestination(destination Location) bool {
	if destination.Z < 0 {
		return false
	}
	if q.owner.Location().Z != destination.Z {
		return false
	}
	if destination.X > 32767 || destination.X < 0 || destination.Y > 32767 || destination.Y < 0 {
		return false
	}
	return true
}

// RequestWalk routes the owner to destination, replacing any queued
// walk. runOverride is the click's run choice (nil follows the run
// setting / click override); it is applied AFTER routing, and only
// when the route produced checkpoints, exactly as in TS.
// MovementQueue.ts:400-433 (requestWalk).
//
// Degenerations, all at this site:
//   - The hitpoints gate (TS: `!player || hitpoints <= 0`) degenerates
//     to the player check only: Go has no hitpoints on Player (no
//     combat/death yet), so all players are treated as alive.
//   - The mobility refusal's forced-walk area fallback (TS
//     407-413) is omitted: no force movement, areas, or
//     handleForcedWalk exists. The refusal's sendMessage (TS
//     414) IS ported: a STUNNED refusal sends the typed spam
//     line, while FROZEN_SPELL/INVALID refusals send nothing
//     (see mobility.sendMessage).
//   - walkToReset (TS 421, MovementQueue.ts:1334-1344) is ported
//     for its walk-to and stopSkillable halves: a pending walk-to
//     interaction is cancelled and the player's current skill
//     action is stopped (see walkToReset). Its combat reset and
//     resetFollow steps are omitted: no combat reset or following
//     exists at this site.
//   - The plugin emitWalkRoute event (TS 424-429) is omitted: no
//     plugin system. A plugin-supplied route can never pre-empt a
//     walk click in Go.
func (q *MovementQueue) RequestWalk(destination Location, runOverride *bool) {
	if !q.owner.IsPlayer() {
		return
	}
	if mobility := q.getMobility(); !mobility.canMove() {
		// The forced-walk area fallback (TS 407-413) is
		// omitted: see the method comment. The refusal
		// line (TS 414) is sent; a frozen/blocked refusal
		// sends nothing. No reset: TS returns directly.
		mobility.sendMessage(q.owner)
		return
	}
	if !q.CheckDestination(destination) {
		return
	}
	q.Reset(true)
	// walkToReset() (TS 421): cancel any pending walk-to
	// interaction and stop the player's current skill action.
	// See the method comment for the omitted halves.
	q.walkToReset()
	// A walk click replaces an interaction's run choice. This runs
	// AFTER Reset, so a pending click override promoted by the
	// reset is cleared again here — the TS order (reset at 420,
	// clear at 423).
	q.clickRunOverride = nil
	// Plugin emitWalkRoute omitted: see the method comment.
	q.applyWalkRoute(destination.X, destination.Y)
	// Set after routing: the route calculation resets the queue.
	// MovementQueue.ts:431.
	if len(q.points) > 0 {
		q.runOverride = runOverride
	} else {
		q.runOverride = nil
	}
}

// applyWalkRoute is the queue-mutation half of
// PathFinder.applyRsmodRoute (PathFinder.ts:128-208) for the
// calculateWalkRoute entry (PathFinder.ts:412-422: locShape -1,
// moveNear true): reset(false) first, lastDestX/lastDestY,
// setRoute, setPathX/setPathY with the region-base math,
// addCheckpoint per waypoint, destination-flag sync. The route
// computation itself is WalkRoute (pathfinder.go); the plugin
// emitNoPath event on failure (PathFinder.ts:178-190) is omitted
// (no plugin system). It returns the number of checkpoints queued,
// as applyRsmodRoute returns the step count.
//
// NOT PORTED here (caller-grounded, verified against commit
// bfd81e3): addFirstStep (MovementQueue.ts:214) has no TS callers
// outside quest plugins that drive NPCs (no NPC drivers in Go);
// walkStep (226) is called only by the clippedStep/randomClippedStep
// NPC helpers inside MovementQueue.ts; walkStepIgnoringFreeze (237)
// has no callers outside its definition; addStep/addSteps (262, 286)
// are called externally only by NPCMovementCoordinator (NPC
// movement, NOT STARTED); setPursuitCheckpoint (321) is called only
// by combat (Combat.ts, ArceuusThralls.ts — no combat in Go). The
// walk path uses addCheckpoint only.
func (q *MovementQueue) applyWalkRoute(destX, destY int) int {
	// A recalculation replaces the final stretch from the actor's
	// live tile. PathFinder.ts:147-150.
	q.Reset(false)
	q.LastDestX = destX
	q.LastDestY = destY

	src := q.owner.Location()
	regionBaseX := src.RegionX() << 3
	regionBaseY := src.RegionY() << 3
	route := WalkRoute(q.finder(), src, max(q.owner.Size(), 1), destX, destY)

	if !route.Success {
		q.SetRoute(false, route.Alternative)
		q.SyncDestinationFlagToRoute()
		// emitNoPath omitted: no plugin system.
		return 0
	}

	q.SetRoute(true, route.Alternative)
	q.SetPathX(route.EndX - regionBaseX)
	q.SetPathY(route.EndY - regionBaseY)

	steps := 0
	for _, waypoint := range route.Waypoints {
		q.AddCheckpoint(Location{X: waypoint.X, Y: waypoint.Y, Z: waypoint.Z})
		steps++
	}
	q.SyncDestinationFlagToRoute()
	return steps
}

// AddCheckpoint stores a routefinder turning point without expanding
// the segment into tiles. MovementQueue.ts:312-318 (addCheckpoint).
// It is refused when the owner cannot move or the deque is full.
// The TS NPC branch (markNpcCombatActive) is omitted: no NPC/combat
// systems exist, and the queue is player-only this phase.
func (q *MovementQueue) AddCheckpoint(position Location) {
	if !q.canMove() || len(q.points) >= maxQueuePoints {
		return
	}
	q.points = append(q.points, movementPoint{position: position, direction: DirNone})
}

// GetDestination returns where the queued walk ends, or false when
// nothing is queued. MovementQueue.ts:435-437 (getDestination
// returns the last point's position or null).
func (q *MovementQueue) GetDestination() (Location, bool) {
	if len(q.points) == 0 {
		return Location{}, false
	}
	return q.points[len(q.points)-1].position, true
}

// Size returns the number of queued checkpoints.
// MovementQueue.ts:1108-1110 (size()).
func (q *MovementQueue) Size() int { return len(q.points) }

// Points returns the queued checkpoint positions in order, oldest
// first: the Go form of pointsReturn (MovementQueue.ts:1203-1205),
// reduced to positions (the stored directions are informational —
// see movementPoint). The returned slice is a copy.
func (q *MovementQueue) Points() []Location {
	out := make([]Location, len(q.points))
	for i, p := range q.points {
		out[i] = p.position
	}
	return out
}

// SyncDestinationFlagToRoute records the current route destination
// for the client's destination flag, sending only when it moves.
// MovementQueue.ts:446-461 (syncDestinationFlagToRoute). The
// implementation lives in destination.go (phase 7); this exported
// method is the call-site entry point the queue's route, process,
// and reset paths already call.
func (q *MovementQueue) SyncDestinationFlagToRoute() {
	q.syncDestinationFlagToRoute()
}

// InvalidateDestinationFlagCache clears the "already sent"
// destination, forcing the next sync to re-send.
// MovementQueue.ts:530-533. TS calls it from
// Mobile.setNeedsPlacement; Go has no such setter yet.
func (q *MovementQueue) InvalidateDestinationFlagCache() {
	q.lastSentDestX = destUnsent
	q.lastSentDestY = destUnsent
}

// IsMoving reports whether the owner moved on the last processed
// cycle. MovementQueue.ts:469-471 (isMovings; the Go name drops
// the TS typo).
func (q *MovementQueue) IsMoving() bool { return q.isMoving }

// BeginCycle clears the per-cycle movement bookkeeping at the start
// of the owner's turn, before movement runs. Player.ts calls it
// unconditionally every turn (Player.process, line 506); Player
// .Process does the same. MovementQueue.ts:473-479 (beginCycle).
// The TS ignoredOccupant clear is absent: dynamic occupancy
// degenerates (see isDynamicallyOccupied), so there is no ignored
// occupant to clear.
func (q *MovementQueue) BeginCycle() {
	q.movedCycle = false
	q.ranCycle = false
	q.blockedByDynamicOccupancy = false
}

// DidRunThisCycle reports whether the cycle just processed was
// spent running (a running tick spends energy, so it must not
// restore). MovementQueue.ts:491-493 (didRunThisCycle).
func (q *MovementQueue) DidRunThisCycle() bool { return q.ranCycle }

// DidMoveThisCycle reports whether the owner moved on the cycle
// just processed. MovementQueue.ts:495-497 (didMoveThisCycle).
func (q *MovementQueue) DidMoveThisCycle() bool { return q.movedCycle }

// SteppedThisWorldCycle reports whether the owner stepped during
// the given world cycle (unlike DidMoveThisCycle, never stale
// before this actor's turn). MovementQueue.ts:499-502
// (steppedThisWorldCycle reads World.getProcessCycle(); Go passes
// the cycle in, since the queue holds no World reference).
func (q *MovementQueue) SteppedThisWorldCycle(cycle int64) bool {
	return q.lastMoveCycle == cycle
}

// DidMovePreviousCycle reports whether the owner stepped on the
// cycle before the given one. MovementQueue.ts:504-506
// (didMovePreviousCycle).
func (q *MovementQueue) DidMovePreviousCycle(cycle int64) bool {
	return q.lastMoveCycle == cycle-1
}

// WasBlockedByDynamicOccupancy reports whether a step this cycle
// was refused because an entity occupied the tile (as opposed to
// static clipping). MovementQueue.ts:508-510. Always false in Go:
// dynamic occupancy degenerates (see isDynamicallyOccupied).
func (q *MovementQueue) WasBlockedByDynamicOccupancy() bool {
	return q.blockedByDynamicOccupancy
}

// IsStepBlocked reports whether the last processed cycle could not
// advance toward the head checkpoint. MovementQueue.ts:522-524
// (isStepBlocked). It survives BeginCycle so the next cycle's
// pursuit can decide whether rebuilding the route would help; no
// pursuit exists in Go yet, but the flag is maintained faithfully.
func (q *MovementQueue) IsStepBlocked() bool { return q.stepBlocked }

// WasRouteInvalidated reports whether the last processed cycle was
// blocked for a reason the route finder can see (a shut door, a new
// wall) rather than an entity in the way.
// MovementQueue.ts:526-528 (wasRouteInvalidated).
func (q *MovementQueue) WasRouteInvalidated() bool {
	return q.stepBlocked && !q.stepBlockedByEntity
}

// SetExternalMovement reports externally-driven movement (a boat, a
// cutscene slide): the queue reports pending work and counts the
// owner as moving for 250 ms. MovementQueue.ts:535-541
// (setExternalMovement). When tileChanged, the region baseline is
// refreshed; the TS wilderness sync on the same branch is omitted
// (no Wilderness system — see Process). No caller drives this in Go
// yet (no boats/cutscenes); the field defaults to zero, so the
// branch in Process is never taken until one does.
func (q *MovementQueue) SetExternalMovement(moving, tileChanged bool) {
	if moving {
		q.externalMovementUntil = time.Now().Add(externalMovementWindow)
	} else {
		q.externalMovementUntil = time.Time{}
	}
	if tileChanged && q.owner.IsPlayer() {
		q.HandleRegionChange()
		// syncWildernessStateForMovedPlayer omitted: no Wilderness.
	}
}

// HasPendingWork reports whether the queue needs a Process call
// this cycle: external movement is active, checkpoints remain, the
// owner is mid-move, or a walk-to interaction is pending (its reach
// check must tick even with no checkpoints left — TS ticks WALK_TO
// tasks from the owner's turn after movement regardless of whether
// movement ran, TaskManager.processWalkTo, TaskManager.ts:71-108).
// MovementQueue.ts:543-551 (hasPendingWork). The TS following branch
// (character.getFollowing() != null) always reads nil in Go: no
// Following field exists, and following is NOT STARTED.
func (q *MovementQueue) HasPendingWork() bool {
	if time.Now().Before(q.externalMovementUntil) {
		return true
	}
	if len(q.points) > 0 || q.isMoving {
		return true
	}
	if q.walkTo != nil {
		return true
	}
	// Following: always nil in Go (no Following field). See the
	// file comment.
	return false
}

// Process steps the owner toward the head checkpoint: 1 step walking,
// 2 running, per tick. MovementQueue.ts:554-662 (process). The loop
// is ported exactly: skip-while-head-equals-current, validatedStep,
// direction from deltas, followX/followY, setLocation, walking
// direction on the first step and running direction on the second,
// checkpoint shift on arrival, stepBlocked flags, and the
// isMoving/movedThisCycle/lastMoveCycle bookkeeping plus the
// runOverride clear and destination-flag sync when the queue
// empties. cycle is the world's process cycle (TS reads
// World.getProcessCycle()).
//
// After the movement work — on EVERY exit path, including the
// early returns — a pending walk-to interaction ticks (tickWalkTo):
// TS ticks WALK_TO tasks from the owner's turn after movement
// (TaskManager.processWalkTo) whether or not movement ran, so an
// arrival observed on an idle cycle still resolves, and a blocked
// walk still fails. With no walk-to pending the tick is a no-op and
// Process behaves exactly as before.
//
// Omissions inside the loop, each at its site below: the following
// branch (no Following field), the freeze-bypass step (no
// walkStepIgnoringFreeze), and the wilderness sync. The run-energy
// drain runs through drainRunEnergy (see below). The TS `previous`
// local is assigned but never read in process(); Go omits it.
func (q *MovementQueue) Process(cycle int64) {
	q.movedCycle = false
	if time.Now().Before(q.externalMovementUntil) {
		q.isMoving = q.canMove()
		q.movedCycle = q.isMoving
		if q.isMoving {
			q.lastMoveCycle = cycle
		}
		q.tickWalkTo(cycle)
		return
	}
	// The TS early return also requires getFollowing() == null;
	// following is always nil in Go. MovementQueue.ts:563-568.
	if len(q.points) == 0 && !q.isMoving {
		q.tickWalkTo(cycle)
		return
	}
	// The TS freeze-bypass branch (forceOneStepThroughFreeze, set
	// only by walkStepIgnoringFreeze) is absent: see the file
	// comment. MovementQueue.ts:570-579.
	if !q.canMove() {
		q.isMoving = false
		q.tickWalkTo(cycle)
		return
	}
	// Following branch (processFollowing) omitted: no Following
	// field. MovementQueue.ts:581-586.

	current := q.owner.Location()
	steps := 1
	if q.IsRunToggled() {
		steps = 2
	}
	moved := false
	stepsMoved := 0
	q.stepBlocked = false
	q.stepBlockedByEntity = false

	for steps > 0 {
		steps--
		for len(q.points) > 0 && q.points[0].position.Equals(current) {
			q.points = q.points[1:]
		}
		if len(q.points) == 0 {
			break
		}
		checkpoint := q.points[0].position
		next, ok := q.validatedStep(current, checkpoint)
		if !ok {
			q.stepBlocked = true
			q.stepBlockedByEntity = q.blockedByDynamicOccupancy
			break
		}

		direction := DirectionFromDeltas(next.X-current.X, next.Y-current.Y)
		q.FollowX = current.X
		q.FollowY = current.Y
		current = next
		// World.onPlayerMoved/onNpcMoved hooks are NOT STARTED;
		// visibility rebuild reads Loc per tick. See
		// Mobile.SetLocation.
		q.owner.SetLocation(next)
		if !moved {
			q.owner.SetWalkingDirection(direction)
		} else {
			q.owner.SetRunningDirection(direction)
		}
		moved = true
		stepsMoved++

		if checkpoint.Equals(current) {
			q.points = q.points[1:]
		}
	}

	if q.owner.IsPlayer() {
		if moved {
			q.HandleRegionChange()
			// syncWildernessStateForMovedPlayer omitted: no
			// Wilderness/PacketSender systems.
			// Live OSRS: only a tick that covered two tiles is a
			// run tick; a lone step is a walk.
			// MovementQueue.ts:646.
			q.drainRunEnergy(stepsMoved)
		}
	}

	q.isMoving = moved
	q.movedCycle = moved
	if moved {
		q.lastMoveCycle = cycle
	}

	if len(q.points) == 0 {
		q.runOverride = nil
		q.SyncDestinationFlagToRoute()
	}

	// The walk-to reach check runs after movement on every cycle,
	// including cycles that stepped: see the method comment.
	q.tickWalkTo(cycle)
}

// CanWalkTo reports whether the owner can step from source to next:
// same level, static clipping allows the step, no dynamic occupancy,
// and (for a player's diagonal step) both cut-corner tiles free of
// player-blocking NPC occupancy. MovementQueue.ts:670-700
// (canWalkTo). source defaults to the owner's tile in TS; Go callers
// pass it explicitly.
func (q *MovementQueue) CanWalkTo(next, source Location) bool {
	if next.Z != source.Z || !q.canMoveStart(source, next) {
		return false
	}
	if q.isDynamicallyOccupied(next) {
		return false
	}
	// The player-diagonal cut-corner check (TS 693-699) degenerates:
	// World.isPlayerBlockingNpcOnTile has no Go equivalent (no
	// occupancy systems; see isDynamicallyOccupied), so both
	// corner tiles always read free. The branch is documented
	// here rather than coded as a call that cannot exist.
	return true
}

// canMoveStart is the static-clipping step check: the Go form of
// RegionManager.canMovestart as canWalkTo calls it (always size x
// size). A nil collision means open ground.
func (q *MovementQueue) canMoveStart(source, next Location) bool {
	if q.collision == nil {
		return true
	}
	size := q.owner.Size()
	return q.collision.CanMoveStart(source, next, size)
}

// isDynamicallyOccupied reports occupancy the static clipping map
// cannot know about. MovementQueue.ts:715-736 (isDynamicallyOccupied).
// DEGENERATED to false: the TS checks consult World.isNpcOccupyingTile
// / isPlayerOccupyingTile / isPlayerBlockingNpcOnTile, and none of
// those occupancy systems exists in Go — the world has zero NPCs and
// no tile-occupant tracking. Players therefore never block each
// other (or NPCs) by standing on a tile, and NPC-flag/"full"-flag
// blocking (e.g. the Agility Pyramid blocks) does not apply. The
// method is kept so validatedStep keeps its TS shape and the
// occupancy port has one place to land.
func (q *MovementQueue) isDynamicallyOccupied(next Location) bool {
	_ = next
	return false
}

// validatedStep picks the owner's next tile toward destination:
// diagonal first, then the x-axis step, then the y-axis step, for
// any footprint size. MovementQueue.ts:738-769 (validatedStep). It
// returns false when no candidate step is walkable; the caller keeps
// the checkpoint and marks the step blocked. When a candidate passes
// static clipping but is dynamically occupied, blockedByDynamicOccupancy
// is set (never in Go — see isDynamicallyOccupied — but the branch
// keeps the TS shape).
func (q *MovementQueue) validatedStep(source, destination Location) (Location, bool) {
	signX := sign(destination.X - source.X)
	signY := sign(destination.Y - source.Y)
	// An entity that moves over what the map blocks steps straight
	// on. MovementQueue.ts:743-745 (IGNORE_CLIPPING_FLAG branch).
	if q.ignoreClipping {
		return source.Transform(signX, signY), true
	}
	var candidates [][2]int
	if signX != 0 && signY != 0 {
		candidates = append(candidates, [2]int{signX, signY})
	}
	if signX != 0 {
		candidates = append(candidates, [2]int{signX, 0})
	}
	if signY != 0 {
		candidates = append(candidates, [2]int{0, signY})
	}
	for _, c := range candidates {
		next := source.Transform(c[0], c[1])
		if q.CanWalkTo(next, source) {
			return next, true
		}
		if next.Z == source.Z && q.canMoveStart(source, next) && q.isDynamicallyOccupied(next) {
			q.blockedByDynamicOccupancy = true
		}
	}
	return Location{}, false
}

func sign(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

// HandleRegionChange refreshes the owner's LastKnownRegion baseline
// when it has moved far enough from the baseline region (or changed
// plane). MovementQueue.ts:771-801 (handleRegionChange). The
// thresholds are the TS ones: diffX/diffY are measured from the
// baseline region's origin (RegionX()*8), and the region counts as
// changed when either diff is < 16 or >= 88, or the plane differs.
// The TS null-population branch (772-775) has no Go form:
// LastKnownRegion is a value, always populated by NewMobile.
// World.markActiveRegionsDirty (798-800) has NO Go equivalent: no
// active-region index exists, and rebuild detection lives in
// FlushTick scene-change detection (internal/session/flush.go),
// which reads the player's live Loc every tick. The TS unwatched-bot
// skip around the dirty-mark is moot for the same reason.
func (q *MovementQueue) HandleRegionChange() {
	loc := q.owner.Location()
	lastKnown := q.owner.GetLastKnownRegion()
	diffX := loc.X - lastKnown.RegionX()*8
	diffY := loc.Y - lastKnown.RegionY()*8
	// A plane change keeps the same x/y region, but it swaps the
	// whole set of NPCs/objects visible to the player, so it must
	// refresh the active regions too. MovementQueue.ts:779-781.
	regionChanged := loc.Z != lastKnown.Z
	if diffX < 16 {
		regionChanged = true
	} else if diffX >= 88 {
		regionChanged = true
	}
	if diffY < 16 {
		regionChanged = true
	} else if diffY >= 88 {
		regionChanged = true
	}
	if regionChanged {
		q.owner.SetLastKnownRegion(loc)
		// World.markActiveRegionsDirty: no Go equivalent; see
		// the method comment.
	}
}

// movementOwnerPlayer resolves a movement queue's owner to the
// *Player the run-energy formulas need, or nil when the owner is
// not a player. It asserts the optional AsPlayer accessor both
// concrete owners implement (Player returns itself, NPC returns
// nil — getAsPlayer, Mobile.ts): an owner without the accessor
// (a test fake) resolves to nil and the formulas read the
// unmodified base. The resolution is sound because the drain is
// already gated on IsPlayer (Process, and MovementQueue.ts:
// 641-646): NPCs own queues in TS too, but process only drains
// when character.isPlayer(), and drainRunEnergy casts the
// character to Player (MovementQueue.ts:817-818) — an NPC queue
// never drains, so a nil resolution inside drainRunEnergy means
// "no player, no modifiers", never a skipped NPC drain.
func movementOwnerPlayer(owner MovementOwner) *Player {
	if resolver, ok := owner.(interface{ AsPlayer() *Player }); ok {
		return resolver.AsPlayer()
	}
	return nil
}

// drainRunEnergy spends run energy for a tick that moved, porting
// MovementQueue.ts:817-833 (drainRunEnergy) exactly. The caller
// (Process) gates on IsPlayer; TS casts the character to Player.
//
//   - Bots are pinned to max instead of draining: when a bot's
//     energy is not max it is set to max, and the method returns
//     without setting ranThisCycle or sending anything.
//   - Otherwise, only when run-toggled AND stepsMoved >= 2 (a lone
//     step is a walk): set ranThisCycle, subtract the drain units,
//     and send run energy. At <= 0, clamp to 0, clear BOTH run
//     overrides (runOverride and clickRunOverride), setRunning
//     (false), and send run status AND run energy: TS calls
//     sendRunStatus() then sendRunEnergy(), and both send the same
//     encodeRunEnergy packet with live state
//     (PacketSender.ts:374-410), so Go sends the packet twice in
//     the same order, with the same arguments (0 units, running
//     false, live weight, stamina 0). The double send is
//     deliberate, not a bug: the client receives two identical
//     run-energy frames, exactly as in TS.
func (q *MovementQueue) drainRunEnergy(stepsMoved int) {
	if q.owner.IsPlayerBot() {
		if q.owner.GetRunEnergyUnits() != RunEnergyMaxUnits {
			q.owner.SetRunEnergyUnits(RunEnergyMaxUnits)
		}
		return
	}
	if q.IsRunToggled() && stepsMoved >= 2 {
		q.ranCycle = true
		q.owner.SetRunEnergyUnits(q.owner.GetRunEnergyUnits() - runEnergyDrainUnits(movementOwnerPlayer(q.owner), q.owner.GetWeightKg(), q.owner.GetCurrentAgilityLevel()))
		if q.owner.GetRunEnergyUnits() <= 0 {
			q.owner.SetRunEnergyUnits(0)
			q.runOverride = nil
			q.clickRunOverride = nil
			q.owner.SetRunning(false)
			// sendRunStatus (PacketSender.ts:404-406).
			q.sendRunEnergyPacket()
		}
		// sendRunEnergy (PacketSender.ts:374-376).
		q.sendRunEnergyPacket()
	}
}

// sendRunEnergyPacket forwards the run-energy orb packet with the
// owner's LIVE state, porting the shared
// PacketSender.sendRunEnergyPacket (PacketSender.ts:379-388):
// encodeRunEnergy(units, running, weightKg, staminaTicks). Stamina
// ticks are always 0: no stamina system exists, and the status
// modifiers start from 0 ticks (EquipmentEffects.ts:311-316).
func (q *MovementQueue) sendRunEnergyPacket() {
	q.owner.SendRunEnergy(q.owner.GetRunEnergyUnits(), q.owner.IsRunning(), q.owner.GetWeightKg(), 0)
}

// Reset clears the queue and its per-route state, returning the
// queue. MovementQueue.ts:864-887 (reset). clickRunOverride
// OUTLIVES a reset: interactions reset the queue on every re-path,
// and a click's walk starts with a reset, which is where its run
// choice takes over — so a pending click override is promoted into
// clickRunOverride here, and an existing clickRunOverride is left
// alone when nothing is pending. runOverride is always cleared.
// When clearDestination is true, the destination flag syncs to the
// emptied route (which records (0, 0)); applyWalkRoute passes false
// because it syncs after enqueuing. The TS resetFollow step is NOT
// part of reset (it lives in walkToReset/resetFollow, and
// resetFollow is still omitted); followX/followY ARE reset here,
// as in TS. The TS forceOneStepThroughFreeze clear is absent with
// that field. TS
// teleport paths (Mobile.moveTo) call reset() before placing the
// owner; no teleport caller exists in Go yet, so Reset is reached
// only from requestWalk, route application, and the tests.
func (q *MovementQueue) Reset(clearDestination bool) {
	q.externalMovementUntil = time.Time{}
	q.runOverride = nil
	// clickRunOverride outlives a reset: see the method comment.
	if q.pendingClickRunOverrideSet {
		q.clickRunOverride = q.pendingClickRunOverride
		q.pendingClickRunOverride = nil
		q.pendingClickRunOverrideSet = false
	}
	q.points = q.points[:0]
	q.FollowX = -1
	q.FollowY = -1
	q.isMoving = false
	q.foundRoute = false
	q.routeEvaluated = false
	q.alternativeRoute = false
	q.stepBlocked = false
	q.stepBlockedByEntity = false
	if clearDestination {
		q.SyncDestinationFlagToRoute()
	}
}

// SetClickRunOverride stages an interaction click's run choice (nil
// follows the run setting). It takes effect when the click starts
// its walk: every walk to a target resets the queue first, and
// Reset promotes the pending value — so a click that walks nowhere
// leaves a walk in progress as it is. MovementQueue.ts:1113-1120
// (setClickRunOverride). No interaction-click listeners exist in Go
// yet (the TS call site is NetworkBuilder.ts:717); the lifecycle is
// ported so they can use it unchanged.
func (q *MovementQueue) SetClickRunOverride(run *bool) {
	q.pendingClickRunOverride = run
	q.pendingClickRunOverrideSet = true
}

// DropPendingClickRunOverride drops a click's run choice that no
// walk took up this turn. MovementQueue.ts:1122-1124.
// Player.Process calls it at the end of every turn, as TS does
// (Player.ts, after the walk-to reach checks).
func (q *MovementQueue) DropPendingClickRunOverride() {
	q.pendingClickRunOverride = nil
	q.pendingClickRunOverrideSet = false
}

// EndClickRunOverride ends an interaction click's run choice, for a
// walk the click didn't start (a retaliation chase).
// MovementQueue.ts:1127-1130 (endClickRunOverride).
func (q *MovementQueue) EndClickRunOverride() {
	q.clickRunOverride = nil
	q.pendingClickRunOverride = nil
	q.pendingClickRunOverrideSet = false
}

// EndClickRunOverrideWhenIdle ends an interaction click's run
// choice once the player is idle: no route, follow, fight, or
// walk-to interaction left. MovementQueue.ts:1133-1141. The TS
// follow/combat checks degenerate: following is always nil and no
// combat target exists, so they never hold the override. The
// walk-to check (TaskManager.hasWalkToTask) IS ported: a pending
// walk-to interaction holds the override. Player.Process calls it
// at the end of every turn, as TS does.
func (q *MovementQueue) EndClickRunOverrideWhenIdle() {
	if q.clickRunOverride == nil || len(q.points) > 0 {
		return
	}
	// Following/combat checks: all degenerate to idle; see the
	// method comment. A pending walk-to holds the override.
	if q.walkTo != nil {
		return
	}
	q.clickRunOverride = nil
}

// IsRunToggled reports whether the owner runs this tick: the
// current-path runOverride, else the interaction clickRunOverride,
// else the owner's run setting. MovementQueue.ts:1143-1147
// (isRunToggled). The TS NPC branch (movement steps > 1) is NOT
// STARTED: the queue is player-only this phase, so a non-player
// owner reads as walking.
func (q *MovementQueue) IsRunToggled() bool {
	if !q.owner.IsPlayer() {
		return false
	}
	if q.runOverride != nil {
		return *q.runOverride
	}
	if q.clickRunOverride != nil {
		return *q.clickRunOverride
	}
	return q.owner.IsRunning()
}

// SetRoute records whether the last route search found a route, and
// whether it was a closest-approach (alternative) route.
// MovementQueue.ts:1185-1189 (setRoute).
func (q *MovementQueue) SetRoute(route, alternative bool) {
	q.routeEvaluated = true
	q.foundRoute = route
	q.alternativeRoute = alternative
}

// HasRoute reports whether the last route search found a route.
// MovementQueue.ts:1191-1193.
func (q *MovementQueue) HasRoute() bool { return q.foundRoute }

// WasRouteEvaluated reports whether a route search has run since
// the last reset. MovementQueue.ts:1195-1197.
func (q *MovementQueue) WasRouteEvaluated() bool { return q.routeEvaluated }

// UsedAlternativeRoute reports whether the last route was a
// closest-approach (alternative) route. MovementQueue.ts:1199-1201.
func (q *MovementQueue) UsedAlternativeRoute() bool { return q.alternativeRoute }

// SetPathX records the route end's x, relative to the owner's
// region base: pathX = RegionX()*8 + x. MovementQueue.ts:1173-1176
// (setPathX). applyWalkRoute passes EndX - regionBaseX, so the
// round trip lands on the route end; both halves are ported
// literally rather than shortcut.
func (q *MovementQueue) SetPathX(x int) {
	q.PathX = q.owner.Location().RegionX()*8 + x
}

// SetPathY records the route end's y, relative to the owner's
// region base. MovementQueue.ts:1178-1181 (setPathY). See SetPathX.
func (q *MovementQueue) SetPathY(y int) {
	q.PathY = q.owner.Location().RegionY()*8 + y
}

// Walk-to interactions: the minimal faithful subset of
// walkToTile/walkToGroundItem (MovementQueue.ts:1234-1331) plus the
// MovementTask closure (1290-1329) that ticks them, and the
// walk-to-object subset (walkToObject, MovementQueue.ts:1742-1912;
// see WalkToObject). Only the ground-item and object forms are
// ported: walkToEntity and the projected-NPC / mounted-decoration
// reach helpers are NOT STARTED (no NPC interaction targets exist
// in Go, and the mounted-decoration check needs the object
// registry).
//
// TS shape, and where each piece lives in Go:
//   - walkToTile routes with PathFinder.calculateGroundItemRoute
//     (PathFinder.ts:424-430: applyRsmodRoute, locShape -1,
//     moveNear true) — exactly WalkRoute, which applyWalkRoute
//     already wraps. The route call is shared, not duplicated.
//   - The MovementTask (MovementQueue.ts:2042-2058) is a WALK_TO
//     task submitted with immediate=false for ground items: its
//     first reach check runs after movement, never on the click
//     cycle itself. Go holds the task state on the queue
//     (walkToState) and ticks it at the end of Process on every
//     exit path, which is the same order (TaskManager
//     .processWalkTo runs from the owner's turn after movement,
//     TaskManager.ts:71-108; the post-turn sweep for skipped
//     entities, processRemainingWalkTo, has no Go form — every Go
//     player takes a turn every tick, so no sweep is needed).
//   - The task closure's re-route throttle, fail-when-unreachable
//     check, and "resolves on the cycle after arrival" delay are
//     ported in tickWalkTo, in TS order.
//   - walkToReset (MovementQueue.ts:1334-1344) is ported for
//     its task-cancellation and stopSkillable halves (see
//     walkToReset): TaskManager.cancelTasks cancels every task
//     keyed by the player index, but the Go TaskManager's tasks
//     carry no owner key (respawn tasks are the only other
//     tasks, and none belongs to a player), so cancelling the
//     pending walk-to is the whole observable task effect, and
//     StopSkillable cancels the player's current skill action.
//     The combat reset and resetFollow steps are gaps: no
//     combat reset or following exists at this site.
//
// Degenerations, all at their sites: the mobility refusal
// line is sent for STUNNED (a typed spam message) and sends
// nothing for FROZEN_SPELL/INVALID (see mobility.sendMessage),
// and the "I can't reach that!" message IS sent through
// MovementOwner.SendMessage. The getMobility gate is
// getMobility/canMove (blockMovement plus the FREEZE/STUN
// timer gates; see canMove for the per-gate audit).

// walkToState is one pending walk-to interaction: the Go form of a
// submitted MovementTask. reached decides arrival; failWhenUnreachable
// selects the TS failWhenUnreachable option; lastRouteCycle is the
// cycle the current route was calculated on (the re-route throttle);
// reachedDestination records that arrival was observed on a previous
// tick, so the action runs on the cycle after arrival. object, when
// non-nil, selects the walk-to-object task shape (see
// tickWalkToObject): the tile-mode fields above are unused except
// destination (the object's tile, kept for symmetry) and action.
// entity, when non-nil, selects the walk-to-entity task shape (see
// tickWalkToEntity): likewise, only destination (the target's tile
// at click time) and action are used.
type walkToState struct {
	destination         Location
	action              func()
	reached             func(dest Location) bool
	failWhenUnreachable bool
	lastRouteCycle      int64
	reachedDestination  bool
	object              *walkToObjectState
	entity              *walkToEntityState
	projected           *walkToProjectedState
}

// ReachedTile reports whether the owner stands exactly on dest:
// PathFinder.reachedTile (PathFinder.ts:433-435: location equality,
// including the plane).
func (q *MovementQueue) ReachedTile(dest Location) bool {
	return q.owner.Location().Equals(dest)
}

// ReachedObj reports whether the owner is at operable distance from
// a 1x1 ground destination: on its tile, or orthogonally adjacent
// with no wall between — PathFinder.reachedObj (PathFinder.ts:
// 443-458), which is reachedTile plus the Rsmod rectangle-exclusive
// (locShape -2) test over the owner's footprint. Diagonal neighbours
// do NOT reach (reachRectangle1). The plane is part of the test:
// ReachedAbsolute reads clipping at the owner's level, and a
// destination on another plane never shares the owner's tile, so a
// cross-plane destination can only reach through the rectangle test
// at the owner's level — exactly as in TS, where reachedAbsolute
// takes the entity's level and the destination's x/y.
func (q *MovementQueue) ReachedObj(dest Location) bool {
	if q.ReachedTile(dest) {
		return true
	}
	return q.finder().ReachedAbsolute(ReachOptions{
		Level:      q.owner.Location().Z,
		SrcX:       q.owner.Location().X,
		SrcY:       q.owner.Location().Y,
		SrcSize:    max(q.owner.Size(), 1),
		DestX:      dest.X,
		DestY:      dest.Y,
		DestWidth:  1,
		DestLength: 1,
		LocShape:   -2,
	})
}

// ReachedGroundItem reports whether the owner has reached a ground
// item's tile: standing on the tile always counts; an adjacent tile
// counts ONLY when this cycle produced no step — the didMoveThisCycle
// gate (MovementQueue.reachedGroundItem, MovementQueue.ts:1245-1251).
// A click from one tile away still routes onto the item and picks it
// up on arrival, but a player who cannot close the last square
// (frozen, blocked) still gets to take it.
func (q *MovementQueue) ReachedGroundItem(dest Location) bool {
	if q.ReachedTile(dest) {
		return true
	}
	return !q.DidMoveThisCycle() && q.ReachedObj(dest)
}

// WalkToGroundItem walks the owner to a ground item's tile and runs
// action on arrival: walkToGroundItem (MovementQueue.ts:1234-1235),
// which is walkToTile with the reachedGroundItem predicate and
// failWhenUnreachable. cycle is the current world process cycle: the
// re-route throttle measures from the cycle the route was calculated
// on. See WalkToTile for the full subset and degenerations.
func (q *MovementQueue) WalkToGroundItem(pos Location, action func(), cycle int64) {
	q.WalkToTile(pos, action, q.ReachedGroundItem, true, cycle)
}

// WalkToTile walks the owner to pos and runs action on arrival, as
// decided by reached (MovementQueue.ts:1262-1331, walkToTile).
// reached defaults in TS to PathFinder.reachedTile; callers pass the
// predicate for their interaction (ground items pass
// ReachedGroundItem). failWhenUnreachable selects the TS option of
// the same name. cycle is the current world process cycle.
//
// The call-time behavior is TS-exact: an exact tile match runs the
// action immediately, before any mobility/destination check — only
// an exact match may skip the walk, so an adjacent click never
// resolves without stepping. A mobility refusal sends the refusal
// line (STUNNED's typed spam; FROZEN_SPELL/INVALID send nothing),
// resets the queue, and returns. An invalid destination resets and returns. Otherwise the queue
// resets, any pending walk-to is cancelled and the player's
// current skill action is stopped (the walkToReset halves),
// a ground-item route is calculated, and the interaction ticks from
// Process until it resolves or fails. A nil action is never called:
// TS guards the call with `action?.()`, and a nil action with a
// pending walk-to would tick forever — so a nil action is treated
// as an immediate no-op at call time when already arrived, and as a
// walk-to with a no-op action otherwise. (No TS caller passes a nil
// action; the guard only keeps a Go caller from hanging the queue.)
func (q *MovementQueue) WalkToTile(pos Location, action func(), reached func(dest Location) bool, failWhenUnreachable bool, cycle int64) {
	// Call time, not tick time: only an exact tile match may skip
	// the walk. MovementQueue.ts:1270-1273.
	if q.ReachedTile(pos) {
		if action != nil {
			action()
		}
		return
	}
	if reached == nil {
		reached = q.ReachedTile
	}
	if action == nil {
		action = func() {}
	}

	if mobility := q.getMobility(); !mobility.canMove() {
		// mobility.sendMessage (MovementQueue.ts:1275-1279):
		// STUNNED sends the typed spam line;
		// FROZEN_SPELL/INVALID send nothing. The reset IS
		// ported.
		mobility.sendMessage(q.owner)
		q.Reset(true)
		return
	}

	if !q.CheckDestination(pos) {
		q.Reset(true)
		return
	}

	q.Reset(true)

	// walkToReset: cancel any pending walk-to and stop the
	// player's current skill action. The combat reset and
	// resetFollow halves are gaps; see the section comment
	// above walkToState.
	q.walkToReset()

	q.applyWalkRoute(pos.X, pos.Y)
	q.walkTo = &walkToState{
		destination:         pos,
		action:              action,
		reached:             reached,
		failWhenUnreachable: failWhenUnreachable,
		lastRouteCycle:      cycle,
	}
}

// walkToReset is the Go form of MovementQueue.walkToReset
// (MovementQueue.ts:1334-1344): cancel the pending walk-to
// interaction (the TaskManager.cancelTasks half, via
// CancelWalkTo) and stop the player's current skill action
// (SkillManager.stopSkillable). TS also resets combat
// (setCastSpell(null), reset) and the follow state; both stay
// gaps — no combat reset or following exists at this site. A
// non-player owner (an NPC queue, a test fake) has no skill
// slot: only the walk-to cancellation runs. This is NOT
// CancelWalkTo: listeners call CancelWalkTo to end an
// interaction without stopping a skill (the clearPendingAction
// shape), while every TS walkToReset call site — requestWalk,
// walkToTile, walkToEntity, walkToProjectedNpc, and both
// walkToObject branches —
// runs this method.
func (q *MovementQueue) walkToReset() {
	q.CancelWalkTo()
	if ownerPlayer, ok := q.owner.(*Player); ok && ownerPlayer != nil {
		if skills := ownerPlayer.Skills(); skills != nil {
			skills.StopSkillable()
		}
	}
}

// CancelWalkTo drops the pending walk-to interaction, if any,
// without touching the walk queue: the Go form of
// TaskManager.cancelWalkToTasks (TaskManager.ts:169-180), which
// clearPendingAction calls (Player.ts:860-868). The walk queue
// keeps its checkpoints — an inventory item action ends the
// interaction but leaves the player walking. Reset does NOT call
// this: TS reset() and walkToReset() are different methods, and
// the walk-to task itself resets the queue on every re-route.
func (q *MovementQueue) CancelWalkTo() {
	q.walkTo = nil
}

// WalkToPending reports whether a walk-to interaction is pending.
func (q *MovementQueue) WalkToPending() bool {
	return q.walkTo != nil
}

// tickWalkTo runs one tick of the pending walk-to interaction: the
// MovementTask closure (MovementQueue.ts:1290-1329), in TS order.
// It runs at the end of Process on every exit path, after movement,
// so DidMoveThisCycle and the route-evaluation flags describe the
// cycle just processed — the same values TS reads in processWalkTo.
//
// Order: an arrival observed on a previous tick runs the action
// (the interaction resolves on the cycle after arrival), then
// resets the queue and stops; a fresh arrival only records
// reachedDestination; an unreachable walk (no checkpoints, no step
// this cycle, route evaluated, alternative route or no route) ends
// with "I can't reach that!", the queue reset, and the task
// stopped; otherwise the walk re-routes, throttled to at most one
// recalculation per two cycles while a path exists, and never on
// the route cycle itself when no path exists.
func (q *MovementQueue) tickWalkTo(cycle int64) {
	wt := q.walkTo
	if wt == nil {
		return
	}
	if wt.object != nil {
		// The walk-to-object task shape (MovementQueue.ts:1825-1884);
		// see tickWalkToObject. It has no re-route throttle and no
		// reachedDestination delay of its own: the didMoveThisCycle
		// gate is what defers arrival past the stepping tick.
		q.tickWalkToObject(wt)
		return
	}
	if wt.entity != nil {
		// The walk-to-entity task shape (MovementQueue.ts:1564-1607):
		// the MovementTask closure with the entityPursuit re-path
		// folded in; see tickWalkToEntity.
		q.tickWalkToEntity(wt)
		return
	}
	if wt.projected != nil {
		// The walk-to-projected-NPC task shape
		// (MovementQueue.ts:1619-1657): the MovementTask closure
		// with no re-path; see tickWalkToProjectedNpc.
		q.tickWalkToProjectedNpc(wt)
		return
	}
	if wt.reachedDestination {
		// TS order is action, reset, stop. Go clears the state
		// first so an action that starts a new walk-to (no TS
		// pickup callback does) is not dropped by the clear;
		// the reset that follows still clears the queue, as in
		// TS, and a new walk-to's task re-routes on its next
		// tick.
		action := wt.action
		q.walkTo = nil
		action()
		q.Reset(true)
		return
	}

	if wt.reached(wt.destination) {
		// The interaction resolves on the cycle after arrival.
		wt.reachedDestination = true
		return
	}

	// Out of steps, not stepping this cycle and still not there:
	// live says so and stops. A blocked step keeps its
	// checkpoint, and a route cut at the checkpoint cap is not a
	// closest-tile route, so both still re-route below.
	// MovementQueue.ts:1303-1310.
	if wt.failWhenUnreachable && len(q.points) == 0 && !q.DidMoveThisCycle() &&
		q.WasRouteEvaluated() && (q.UsedAlternativeRoute() || !q.HasRoute()) {
		// MovementQueue.ts:1316: live says so, then the
		// queue resets and the task stops.
		q.owner.SendMessage("I can't reach that!")
		q.walkTo = nil
		q.Reset(true)
		return
	}

	hasPath := len(q.points) > 0
	if (hasPath && cycle-wt.lastRouteCycle < 2) || (!hasPath && cycle == wt.lastRouteCycle) {
		return
	}

	q.Reset(true)
	wt.lastRouteCycle = cycle
	q.applyWalkRoute(wt.destination.X, wt.destination.Y)
}

// Walk-to-object: walkToObject + buildObjectRouteSpec + reachedObject
// (MovementQueue.ts:1348-1384, 1742-1912; PathFinder.ts:229-248,
// 463-490). WalkToObject routes the owner to a loc and runs action on
// arrival; the route hook (onObjectRoute) fires before any mobility
// or destination check, and may redirect the walk to a tile
// (destination) or add blockAccessFlags. The route spec is the TS
// buildObjectRouteSpec shape: types 10/11/22 route to the object's
// footprint (base-definition sizes for 10/11, resolved-child sizes
// for 22) with the definition's blocking mask OR'd with
// blockAccessFlags; other types route with routeSize = type+1, a
// 1x1 reach, and blockAccessFlags alone. Routing uses the same
// findRoute shape as calculateRoute with moveNear=true
// (PathFinder.ts:463-490), via applyObjectRoute — NOT the walk-only
// WalkRoute used by applyWalkRoute.
//
// Degenerations and gaps (all documented again at the call sites):
//   - The frozen branch (MovementQueue.ts:1765-1780) IS ported:
//     a frozen player operates what they reach from their tile
//     (the object route queues no checkpoints while frozen, so
//     the arrival check in tickWalkToObject fires the action
//     without stepping), a frozen player out of reach hears
//     "I can't reach that!", and another lock (a stun) still
//     stops a frozen player with its own refusal line. The
//     faceObjectCentre call inside the branch is still a gap
//     (no facing machinery); see WalkToObject.
//   - Facing: TS calls setPositionToFace at queue time and in the
//     arrival closure. Go has no position-to-face/flag machinery, so
//     no facing is set. The arrival closure's facing is the
//     listener's gap, not this function's.
//   - The mounted-decoration reach term (MovementQueue.ts:1435-1476,
//     reachesObject 1729): the OR term over wall-mounted objects is
//     NOT ported — it needs the object registry and wall clipping
//     this queue cannot see. ReachedObject is the ReachedAbsolute
//     term only.
//   - The entityPursuit pre-step validity check (MovementQueue.ts:
//     processEntityPursuit runs before the queue steps): validity is
//     checked after movement in tickWalkToObject, so one step may
//     occur after the object is removed. The next tick's validity
//     check stops the walk silently.
//   - The failure console.warn for non-bots (MovementQueue.ts:1874)
//     has no equivalent in this package (no logging) and is omitted;
//     the "I can't reach that!" message is ported.
//   - Private-area terms of isInteractionObjectValid (1674-1687)
//     degenerate: no private areas exist, so the z check and the
//     stood-at-click still-stands re-check are the ported terms.
//     The id/type terms are snapshot-immutable in the target.

// ObjectTarget is a loc being walked to: the identity and definition
// shape WalkToObject needs, resolved by the caller. ID, Location,
// Type, and Face identify the object; SizeX/SizeY are the resolved
// (forPlayer) definition footprint; BaseSizeX/BaseSizeY are the base
// (forId) definition footprint, falling back to the resolved sizes
// when the base definition is unavailable (the TS fallback is
// ObjectDefinition.forId(...) ?? definition —
// MovementQueue.ts:1357-1358); BlockingMask is the resolved
// definition's blocking mask (clipMask); Name and Interactions are
// the resolved definition's name and menu, carried for the route
// event (PluginTypes.ts:149-157).
type ObjectTarget struct {
	ID           int
	Location     Location
	Type         int
	Face         int
	SizeX        int
	SizeY        int
	BaseSizeX    int
	BaseSizeY    int
	BlockingMask int
	Name         string
	Interactions []string
}

// ObjectRouteEvent is the game-package form of the
// PluginObjectRouteEvent (PluginTypes.ts:149-157): the walk-to
// path's view of the object being approached. Route hooks may set
// Destination (redirecting the walk to a tile) and/or
// BlockAccessFlags (added to the route and reach masks). Source is
// the owner's tile at click time. The hub translates this into the
// plugin-facing event; see internal/plugins.ObjectRouteEvent.
type ObjectRouteEvent struct {
	Target           ObjectTarget
	ClickType        int
	Source           Location
	Destination      *Location
	BlockAccessFlags int
}

// WalkToObjectHooks carries the walk-to-object extension points.
// Route fires the onObjectRoute hooks; nil means no hooks and no
// redirection. StillStands answers whether the object still stands
// (identity, or same id/type/face at the tile — MapObjects.ts:
// stillStands); nil means unknowable, which degenerates
// stoodAtClick to false and skips the re-check.
type WalkToObjectHooks struct {
	Route       func(*ObjectRouteEvent)
	StillStands func() bool
}

// walkToObjectState is the object-mode half of walkToState: the
// snapshot target, the route/reach spec built at click time, the
// stoodAtClick flag (isPlayer && stillStands at click time —
// MovementQueue.ts:1837), the still-stands re-check, and the
// repath attempt count (max 2 — MovementQueue.ts:1864).
type walkToObjectState struct {
	target         ObjectTarget
	spec           objectRouteSpec
	stoodAtClick   bool
	stillStands    func() bool
	repathAttempts int
}

// objectRouteSpec is the Go form of ObjectRouteSpec
// (MovementQueue.ts:1341-1347): the findRoute shape (routeSize /
// routeXLength / routeYLength / routeDirection / routeBlockingMask)
// plus the reach predicate shape (reachWidth / reachLength /
// reachAngle / reachShape / reachBlockAccessFlags) and the
// object's tile.
type objectRouteSpec struct {
	objectX, objectY      int
	routeSize             int
	routeXLength          int
	routeYLength          int
	routeDirection        int
	routeBlockingMask     int
	reachWidth            int
	reachLength           int
	reachAngle            int
	reachShape            int
	reachBlockAccessFlags int
}

// buildObjectRouteSpec mirrors buildObjectRouteSpec
// (MovementQueue.ts:1348-1384). Types 10/11/22 route to the
// footprint: base-definition sizes for 10/11 (the transform
// child's size is NOT used — e.g. the Ancient Grave is 4x2 at
// base but 1x1 as a child), resolved-child sizes for type 22,
// with blockingMask = definition mask OR blockAccessFlags, reach
// shape 10, and reach width/length equal to the footprint. Other
// types route with routeSize = type+1, zero route lengths, mask
// = blockAccessFlags alone, a 1x1 reach, reach shape = type, and
// reach angle = face.
func buildObjectRouteSpec(target ObjectTarget, blockAccessFlags int) objectRouteSpec {
	spec := objectRouteSpec{objectX: target.Location.X, objectY: target.Location.Y}
	if target.Type == 10 || target.Type == 11 || target.Type == 22 {
		xLength, yLength := target.BaseSizeX, target.BaseSizeY
		if target.Type == 22 {
			xLength, yLength = target.SizeX, target.SizeY
		}
		mask := target.BlockingMask | blockAccessFlags
		spec.routeXLength = xLength
		spec.routeYLength = yLength
		spec.routeDirection = target.Face
		spec.routeBlockingMask = mask
		spec.reachWidth = xLength
		spec.reachLength = yLength
		spec.reachAngle = target.Face
		spec.reachShape = 10
		spec.reachBlockAccessFlags = mask
		return spec
	}
	spec.routeSize = target.Type + 1
	spec.routeDirection = target.Face
	spec.routeBlockingMask = blockAccessFlags
	spec.reachWidth = 1
	spec.reachLength = 1
	spec.reachAngle = target.Face
	spec.reachShape = target.Type
	spec.reachBlockAccessFlags = blockAccessFlags
	return spec
}

// applyObjectRoute mirrors queueObjectRoute + applyRsmodRoute
// (MovementQueue.ts:1873-1912, 1071-1091) with the calculateRoute
// findRoute shape (PathFinder.ts:469-490): when both route lengths
// are positive the destination is the footprint with locShape 10;
// else when routeSize is non-zero the locShape is routeSize-1;
// else a 1x1 destination with locShape -1. moveNear is always
// true (calculateObjectRoute, PathFinder.ts:463-465). It returns
// the number of checkpoints queued. Like applyWalkRoute, it
// resets the queue without touching the walk-to state and syncs
// the destination flag.
func (q *MovementQueue) applyObjectRoute(spec objectRouteSpec) int {
	q.Reset(false)
	q.LastDestX = spec.objectX
	q.LastDestY = spec.objectY

	src := q.owner.Location()
	regionBaseX := src.RegionX() << 3
	regionBaseY := src.RegionY() << 3

	destWidth, destLength, locShape, locAngle, blockAccessFlags := 1, 1, -1, 0, 0
	if spec.routeXLength > 0 && spec.routeYLength > 0 {
		destWidth, destLength = spec.routeXLength, spec.routeYLength
		locShape, locAngle, blockAccessFlags = 10, spec.routeDirection, spec.routeBlockingMask
	} else if spec.routeSize != 0 {
		locShape, locAngle, blockAccessFlags = spec.routeSize-1, spec.routeDirection, spec.routeBlockingMask
	}

	route := q.finder().FindRoute(RouteOptions{
		Level:            src.Z,
		SrcX:             src.X,
		SrcY:             src.Y,
		SrcSize:          max(q.owner.Size(), 1),
		DestX:            spec.objectX,
		DestY:            spec.objectY,
		DestWidth:        destWidth,
		DestLength:       destLength,
		LocAngle:         locAngle,
		LocShape:         locShape,
		BlockAccessFlags: blockAccessFlags,
		MaxWaypoints:     25,
		MoveNear:         true,
	})
	if !route.Success {
		q.SetRoute(false, route.Alternative)
		q.SyncDestinationFlagToRoute()
		return 0
	}
	q.SetRoute(true, route.Alternative)
	q.SetPathX(route.EndX - regionBaseX)
	q.SetPathY(route.EndY - regionBaseY)
	steps := 0
	for _, waypoint := range route.Waypoints {
		q.AddCheckpoint(Location{X: waypoint.X, Y: waypoint.Y, Z: waypoint.Z})
		steps++
	}
	q.SyncDestinationFlagToRoute()
	return steps
}

// ReachedObject mirrors PathFinder.reachedObject
// (PathFinder.ts:229-248): ReachedAbsolute with the destination
// footprint, locAngle, locShape, and blockAccessFlags. The
// mounted-decoration OR term of MovementQueue.reachesObject
// (MovementQueue.ts:1729) is NOT ported — see the WalkToObject
// section header.
func (q *MovementQueue) ReachedObject(destX, destY, destWidth, destLength, locAngle, locShape, blockAccessFlags int) bool {
	loc := q.owner.Location()
	return q.finder().ReachedAbsolute(ReachOptions{
		Level:            loc.Z,
		SrcX:             loc.X,
		SrcY:             loc.Y,
		SrcSize:          max(q.owner.Size(), 1),
		DestX:            destX,
		DestY:            destY,
		DestWidth:        destWidth,
		DestLength:       destLength,
		LocAngle:         locAngle,
		LocShape:         locShape,
		BlockAccessFlags: blockAccessFlags,
	})
}

// reachesObject is the object-mode arrival predicate: ReachedObject
// with the click-time spec's reach shape.
func (q *MovementQueue) reachesObject(spec objectRouteSpec) bool {
	return q.ReachedObject(spec.objectX, spec.objectY, spec.reachWidth, spec.reachLength, spec.reachAngle, spec.reachShape, spec.reachBlockAccessFlags)
}

// WalkToObject mirrors walkToObject (MovementQueue.ts:1742-1912):
// the route event fires first; a set destination redirects to a
// tile walk (default reachedTile, no failWhenUnreachable); else
// the route spec is built, mobility and destination are checked,
// the queue is reset, the object route is queued, and the
// object-mode task is installed. Unlike WalkToTile there is no
// call-time reach short-circuit: an already-adjacent player
// arrives through tickWalkToObject on the click cycle, after the
// didMoveThisCycle gate. The walk-to state is installed even
// when no route was found (TS installs the task unconditionally);
// tickWalkToObject then repaths up to twice before failing with
// the message. A nil action is replaced with a no-op, as in
// WalkToTile.
func (q *MovementQueue) WalkToObject(target ObjectTarget, action func(), clickType int, cycle int64, hooks WalkToObjectHooks) {
	if action == nil {
		action = func() {}
	}

	// Route event (MovementQueue.ts:1743-1760): fired before any
	// mobility or destination check, with the click-time source
	// tile and the resolved definition's name/menu carried on the
	// target. A set destination redirects to a tile walk and
	// returns; the object task is never installed.
	routeEvent := &ObjectRouteEvent{
		Target:    target,
		ClickType: clickType,
		Source:    q.owner.Location(),
	}
	if hooks.Route != nil {
		hooks.Route(routeEvent)
	}
	if routeEvent.Destination != nil {
		dest := *routeEvent.Destination
		q.WalkToTile(dest, action, nil, false, cycle)
		return
	}

	spec := buildObjectRouteSpec(target, routeEvent.BlockAccessFlags)

	// Mobility (MovementQueue.ts:1765-1780): frozen players may
	// operate what they reach from their tile, other locks stop
	// them. blocking is the mobility ignoring freeze when
	// frozen, else the mobility itself: a stun still stops a
	// frozen player. When blocked — or frozen and out of
	// reach — a movable blocking state means frozen-out-of-
	// reach (cancel the walk-to, face the object, say so,
	// reset); any other blocking state sends its refusal line
	// (STUNNED's typed spam; INVALID sends nothing) and
	// resets. A frozen player who reaches the object falls
	// through: applyObjectRoute queues no checkpoints while
	// frozen (AddCheckpoint refuses), so tickWalkToObject
	// fires the action without stepping.
	mobility := q.getMobility()
	frozen := mobility == mobilityFrozenSpell
	blocking := mobility
	if frozen {
		blocking = q.getMobilityIgnoringFreeze()
	}
	if !blocking.canMove() || (frozen && !q.reachesObject(spec)) {
		if blocking.canMove() {
			// Frozen, otherwise movable, out of reach:
			// walkToReset + face + message, then reset
			// (MovementQueue.ts:1773-1778).
			q.walkToReset()
			// faceObjectCentre (TS 1774): GAP — no
			// position-to-face/flag machinery in Go.
			q.owner.SendMessage("I can't reach that!")
		} else {
			blocking.sendMessage(q.owner)
		}
		q.Reset(true)
		return
	}

	// Destination validity (MovementQueue.ts:1800-1802):
	// checkDestination already ran for the tile walk; the
	// object tile is checked the same way. A failed check
	// resets silently, as in TS.
	if !q.CheckDestination(target.Location) {
		q.Reset(true)
		return
	}

	q.Reset(true)
	// walkToReset (MovementQueue.ts:1790): cancel any pending
	// walk-to and stop the player's current skill action.
	q.walkToReset()
	q.applyObjectRoute(spec)
	// setPositionToFace (MovementQueue.ts:1809): GAP — the
	// player-facing seam exists since Phase 33
	// (Player.SetPositionToFace / ForcePositionToFace), but
	// this walk-to path does not call it yet; no facing is set.

	// stoodAtClick (MovementQueue.ts:1837): isPlayer &&
	// stillStands at click time. A nil StillStands hook
	// degenerates to false.
	stoodAtClick := false
	if q.owner.IsPlayer() && hooks.StillStands != nil {
		stoodAtClick = hooks.StillStands()
	}

	q.walkTo = &walkToState{
		destination: target.Location,
		action:      action,
		object: &walkToObjectState{
			target:       target,
			spec:         spec,
			stoodAtClick: stoodAtClick,
			stillStands:  hooks.StillStands,
		},
	}
}

// interactionObjectValid mirrors isInteractionObjectValid
// (MovementQueue.ts:1674-1687) minus the terms that degenerate:
// the private-area identity check (no areas), and the id/type
// identity terms (the target is a snapshot, so they cannot
// change). What remains: the plane must match, and when the
// player stood at the object at click time the object must
// still stand (quiet re-check).
func (q *MovementQueue) interactionObjectValid(obj *walkToObjectState) bool {
	if obj.target.Location.Z != q.owner.Location().Z {
		return false
	}
	if obj.stoodAtClick && obj.stillStands != nil && !obj.stillStands() {
		return false
	}
	return true
}

// tickWalkToObject mirrors the walkToObject MovementTask closure
// (MovementQueue.ts:1825-1884). In order: the validity check
// (area change or invalid object) resets and stops silently;
// arrival (no checkpoints and reachesObject) waits while the
// player moved this cycle, else records the arrival tile,
// stops the task, and runs the action immediately; pending
// checkpoints or movement return without re-evaluating; a lost
// route repaths up to twice; else the walk fails with the TS
// message and stops. The task ticks on the click cycle (the
// MovementTask is immediate=false with delay 0, activated for
// processWalkTo after the same turn's movement — TaskManager.ts:
// 71-142), so no arm-delay is needed: tickWalkTo runs at the
// end of Process, after the queue stepped.
func (q *MovementQueue) tickWalkToObject(wt *walkToState) {
	obj := wt.object

	if !q.interactionObjectValid(obj) {
		q.Reset(true)
		q.walkTo = nil
		return
	}

	if len(q.points) == 0 && q.reachesObject(obj.spec) {
		if q.DidMoveThisCycle() {
			// The player stepped this cycle: wait for the next
			// tick so arrival is observed standing still
			// (MovementQueue.ts:1841-1843).
			return
		}
		// Arrival: pathX/pathY record the arrival tile literally
		// (TS assigns the coordinates, not via setPathX/setPathY
		// — MovementQueue.ts:1844-1845). The setPositionToFace
		// call at 1847 is a gap (no facing system). The task
		// stops before the action runs, so a walk-to started by
		// the action survives.
		loc := q.owner.Location()
		q.PathX = loc.X
		q.PathY = loc.Y
		action := wt.action
		q.walkTo = nil
		action()
		return
	}

	if len(q.points) > 0 || q.IsMoving() {
		return
	}

	// Route lost (MovementQueue.ts:1862-1870): repath up to
	// twice, then fail. A re-queued route (or a HasRoute
	// marker) keeps the task alive for the next tick.
	routeLost := q.WasRouteInvalidated() || !q.WasRouteEvaluated() || (q.HasRoute() && !q.UsedAlternativeRoute())
	if routeLost && obj.repathAttempts < 2 {
		obj.repathAttempts++
		q.applyObjectRoute(obj.spec)
		if len(q.points) > 0 || q.HasRoute() {
			return
		}
	}

	// Failure (MovementQueue.ts:1872-1879): the message, then
	// stop. No reset (the queue is already empty) and no warn
	// log (no logging in this package).
	q.owner.SendMessage("I can't reach that!")
	q.walkTo = nil
}

// Walk-to-entity: walkToEntity + reachedEntity + calculateEntityRoute
// (MovementQueue.ts:1478-1615; PathFinder.ts:214-229, 265-280, 309-318).
// WalkToEntity routes the owner to a live entity and runs action on
// arrival, re-pathing to the target's current tile while it moves
// (the entityPursuit half, folded into the post-step tick — see
// tickWalkToEntity). Routing uses the calculateEntityRoute findRoute
// shape at the default range (destination = the target's tile, size
// = the target's footprint, locShape -2, moveNear) via
// applyEntityRoute; arrival is ReachedAbsolute with the same shape
// (PathFinder.reachedEntity).
//
// Degenerations and gaps (all documented again at the call sites):
//   - Range, projectile clip, and overlap: only the default range
//     (range = 1, routeRange = range) is ported. The range > 1
//     reach predicate (distance + canProjectileAttack), the
//     ignoreProjectileClip option, and allowOverlappingTiles have
//     no callers in Go and need projectile-clip machinery that
//     does not exist; WalkToEntity takes no range.
//   - Facing and interaction state: setMobileInteraction and
//     faceMobileBriefly are gaps (no interaction/facing
//     machinery); the action still runs.
//   - The unreachable-NPC branch (canInteractWithUnreachableNpc,
//     MovementQueue.ts:1923-1928, plus the frozen
//     isOnEntityRouteEnd dry run at 1542): NPC-only, and it
//     needs projectile clip. It is always false in Go, so a
//     frozen player out of reach hears "I can't reach that!"
//     and an NPC across a counter is never operated from the
//     route end. Player targets are unaffected: the branch
//     requires entity.isNpc().
//   - The projected-NPC branch (walkToProjectedNpc): ported, at
//     the default range (range = 1, as for walkToEntity). The
//     canProjectileMove term of reachedProjectedNpc degenerates
//     to true: no projectile-clip machinery exists in Go, so a
//     main-world NPC behind a wall still counts as reached
//     from the deck when the boat's root tile is in range.
//     Deck routing itself depends on the route finder reading
//     deck clips: deck tiles have no cache map region, so
//     until area clips feed the finder the deck route is
//     unclipped (see BoatDeckArea). setMobileInteraction is a
//     gap, as for walkToEntity.
//   - The entityPursuit pre-step timing (MovementQueue.ts:1589,
//     processEntityPursuit runs before the queue steps):
//     validity and re-pathing are checked after movement in
//     tickWalkToEntity, so one step may occur after the
//     target is invalidated, and re-pathing lags a tick behind
//     a moving target — the same degeneration as
//     walk-to-object's.
//   - Private-area terms of isInteractionTargetValid degenerate:
//     no private areas exist, so registration, hitpoints, the
//     plane, and the NPC-owner rule are the ported terms. The
//     NPC type-change term reads the live NPC id; NPC
//     transformations are NOT ported, so the id cannot change
//     and the term cannot fire until they land.

// EntityTarget is a live entity being walked to: the Mobile whose
// location, size, and registration are read every tick, plus the
// reads that live on the typed entity rather than on Mobile.
// Alive reports hitpoints > 0; nil means always alive. NPCType
// reports the NPC type the target shows now (its transform, else
// its id); nil means -1, the player value. Owner is the NPC's
// owner, or nil for players and unowned NPCs. Size is the
// target's footprint in tiles (1 for a player); it is read at
// construction — NPC transformations, the only size-changer,
// are NOT ported. PlayerTarget and NPCTarget build targets
// from typed entities; a zero target (nil Mobile) is invalid
// and WalkToEntity refuses it with a silent reset.
type EntityTarget struct {
	Mobile  *Mobile
	Alive   func() bool
	NPCType func() int
	Owner   *Player
	Size    int
}

// PlayerTarget builds an entity target from a player: hitpoints
// are the Hitpoints skill's current level (the playerCombatant
// read), the NPC type is -1, and there is no owner.
func PlayerTarget(p *Player) EntityTarget {
	if p == nil {
		return EntityTarget{}
	}
	return EntityTarget{
		Mobile: &p.Mobile,
		Alive:  func() bool { return p.Skills().CurrentLevel(SkillHitpoints) > 0 },
		Size:   1,
	}
}

// NPCTarget builds an entity target from an NPC: hitpoints are
// the NPC's live hitpoints (the npcCombatant read: the Hitpoints
// field, or the definition value while unset), the type is the
// NPC's id (no transformations exist), and the owner is the
// NPC's owner.
func NPCTarget(n *NPC) EntityTarget {
	if n == nil {
		return EntityTarget{}
	}
	return EntityTarget{
		Mobile:  &n.Mobile,
		Alive:   func() bool { return npcTargetHitpoints(n) > 0 },
		NPCType: func() int { return n.ID },
		Owner:   n.Owner(),
		Size:    n.Size(),
	}
}

// npcTargetHitpoints mirrors npcCombatant.Hitpoints
// (combatant.go): the Hitpoints field while set, else the
// definition value (zero means unset, npc.go).
func npcTargetHitpoints(n *NPC) int {
	if n.Hitpoints != 0 {
		return n.Hitpoints
	}
	return n.Definition().Hitpoints()
}

// walkToEntityState is the entity-mode half of walkToState: the
// live target, the NPC type captured at click time, and the
// queue owner as a player (for the NPC-owner rule; nil when
// the owner is not a player).
type walkToEntityState struct {
	target      EntityTarget
	clickedType int
	ownerPlayer *Player
}

// ReachedEntity mirrors PathFinder.reachedEntity
// (PathFinder.ts:214-229): ReachedAbsolute with the target's
// footprint, locShape -2, at the owner's level. Unlike
// ReachedObj there is no standing-on-the-tile short-circuit:
// locShape -2 is rectangle-exclusive, so overlapping the
// target does NOT reach — exactly as in TS, where
// reachedEntity is the bare ReachedAbsolute call (a player
// standing on the target routes one tile off, then reaches).
// A nil-Mobile target never reaches.
func (q *MovementQueue) ReachedEntity(target EntityTarget) bool {
	if target.Mobile == nil {
		return false
	}
	loc := q.owner.Location()
	targetLoc := target.Mobile.Location()
	return q.finder().ReachedAbsolute(ReachOptions{
		Level:      loc.Z,
		SrcX:       loc.X,
		SrcY:       loc.Y,
		SrcSize:    max(q.owner.Size(), 1),
		DestX:      targetLoc.X,
		DestY:      targetLoc.Y,
		DestWidth:  max(target.Size, 1),
		DestLength: max(target.Size, 1),
		LocShape:   -2,
	})
}

// applyEntityRoute mirrors calculateEntityRoute at the default
// range (PathFinder.ts:265-280, entityRouteArea at 309-318)
// through the applyRsmodRoute queue-mutation shape: the
// destination is the target's live tile with the target's
// footprint, locShape -2, moveNear. It returns the number of
// checkpoints queued. Like applyObjectRoute, it resets the
// queue without touching the walk-to state and syncs the
// destination flag.
func (q *MovementQueue) applyEntityRoute(target EntityTarget) int {
	q.Reset(false)
	targetLoc := target.Mobile.Location()
	q.LastDestX = targetLoc.X
	q.LastDestY = targetLoc.Y

	src := q.owner.Location()
	regionBaseX := src.RegionX() << 3
	regionBaseY := src.RegionY() << 3

	size := max(target.Size, 1)
	route := q.finder().FindRoute(RouteOptions{
		Level:        src.Z,
		SrcX:         src.X,
		SrcY:         src.Y,
		SrcSize:      max(q.owner.Size(), 1),
		DestX:        targetLoc.X,
		DestY:        targetLoc.Y,
		DestWidth:    size,
		DestLength:   size,
		LocShape:     -2,
		MaxWaypoints: 25,
		MoveNear:     true,
	})
	if !route.Success {
		q.SetRoute(false, route.Alternative)
		q.SyncDestinationFlagToRoute()
		return 0
	}
	q.SetRoute(true, route.Alternative)
	q.SetPathX(route.EndX - regionBaseX)
	q.SetPathY(route.EndY - regionBaseY)
	steps := 0
	for _, waypoint := range route.Waypoints {
		q.AddCheckpoint(Location{X: waypoint.X, Y: waypoint.Y, Z: waypoint.Z})
		steps++
	}
	q.SyncDestinationFlagToRoute()
	return steps
}

// interactionEntityValid mirrors isInteractionTargetValid
// (MovementQueue.ts:1690-1702) plus the NPC type-change term
// (MovementQueue.ts:1526-1527): the target must be registered
// and alive, on the owner's plane, and — for an owned NPC —
// owned by the queue owner; an NPC whose type changed since
// the click is no longer what was clicked. The private-area
// term degenerates (no private areas exist).
func (q *MovementQueue) interactionEntityValid(st *walkToEntityState) bool {
	target := st.target
	if target.Mobile == nil || !target.Mobile.IsRegistered() {
		return false
	}
	if target.Alive != nil && !target.Alive() {
		return false
	}
	if target.Mobile.Location().Z != q.owner.Location().Z {
		return false
	}
	if target.Owner != nil && target.Owner != st.ownerPlayer {
		return false
	}
	if target.NPCType != nil && target.NPCType() != st.clickedType {
		return false
	}
	return true
}

// WalkToEntity mirrors walkToEntity at the default range
// (MovementQueue.ts:1478-1615): the mobility check with the
// frozen special-case, the destination check, the validity
// check, the immediate reached short-circuit, the frozen
// branch, the entity route, and the installed task. Unlike
// WalkToTile there is no cycle parameter: entity pursuit
// re-paths unthrottled, so no re-route throttle exists to
// measure. A nil action is replaced with a no-op, as in
// WalkToTile. See the section header for the subset and its
// degenerations.
func (q *MovementQueue) WalkToEntity(target EntityTarget, action func()) {
	if action == nil {
		action = func() {}
	}

	// Mobility (MovementQueue.ts:1493-1500): frozen players
	// may interact with what they reach from their tile,
	// other locks stop them. blocking is the mobility
	// ignoring freeze when frozen, else the mobility
	// itself: a stun still stops a frozen player.
	mobility := q.getMobility()
	frozen := mobility == mobilityFrozenSpell
	blocking := mobility
	if frozen {
		blocking = q.getMobilityIgnoringFreeze()
	}
	if !blocking.canMove() {
		blocking.sendMessage(q.owner)
		q.Reset(true)
		return
	}

	// Destination validity (MovementQueue.ts:1502-1505). A
	// nil-Mobile target has no destination: refuse with the
	// same silent reset (TS would throw on a null entity;
	// Go degrades to the listener no-disconnect shape).
	if target.Mobile == nil || !q.CheckDestination(target.Mobile.Location()) {
		q.Reset(true)
		return
	}

	// A main-world NPC seen from a boat deck lives in the world's
	// coordinate space while the player stands in the deck
	// scene. Resolve that interaction from the boat's root
	// tile instead of trying to path the deck scene across
	// the map and timing out (MovementQueue.ts:1504-1509).
	if q.isProjectedMainWorldNpc(target) {
		q.walkToProjectedNpc(target, action)
		return
	}

	ownerPlayer, _ := q.owner.(*Player)
	st := &walkToEntityState{target: target, ownerPlayer: ownerPlayer, clickedType: -1}
	if target.NPCType != nil {
		st.clickedType = target.NPCType()
	}
	if !q.interactionEntityValid(st) {
		q.Reset(true)
		return
	}

	q.Reset(true)
	// walkToReset (MovementQueue.ts:1519): cancel any pending
	// walk-to and stop the player's current skill action.
	q.walkToReset()

	// An already-reached target resolves at once
	// (MovementQueue.ts:1535-1539). setMobileInteraction
	// (TS 1536): GAP — no interaction state in Go.
	if q.ReachedEntity(target) {
		action()
		return
	}
	if frozen {
		// Frozen and out of reach (MovementQueue.ts:1540-1549):
		// the route-end + unreachable-NPC branch cannot fire
		// (see the section header), so the player faces the
		// target and hears the message. faceMobileBriefly
		// (TS 1547): GAP — no facing machinery.
		q.owner.SendMessage("I can't reach that!")
		return
	}

	q.applyEntityRoute(target)
	// canInteractWithUnreachableNpc (MovementQueue.ts:1553):
	// always false in Go (see the section header), so the
	// reset + immediate action branch cannot fire.

	q.walkTo = &walkToState{
		destination: target.Mobile.Location(),
		action:      action,
		entity:      st,
	}
}

// tickWalkToEntity mirrors the walkToEntity MovementTask closure
// (MovementQueue.ts:1564-1588) with the entityPursuit half
// (1589-1607) folded in. In order: an invalid target resets
// and stops silently; arrival resets, stops, and runs the
// action; pending checkpoints or movement return without
// re-evaluating; otherwise the walk re-paths to the target's
// live tile (the pursuit re-path, which TS runs pre-step when
// points <= 1 or the route was invalidated — here it runs
// whenever the queue has run dry, which subsumes both); a
// re-path that queues nothing fails with the message and
// stops. The task ticks on the click cycle, as the object
// task does. The unreachable-NPC branch of the closure
// (1579) is always false in Go (see WalkToEntity).
func (q *MovementQueue) tickWalkToEntity(wt *walkToState) {
	st := wt.entity

	if !q.interactionEntityValid(st) {
		q.Reset(true)
		q.walkTo = nil
		return
	}

	// setMobileInteraction (MovementQueue.ts:1570): GAP — no
	// interaction state in Go.

	if q.ReachedEntity(st.target) {
		// Arrival: reset, stop, run the action. The task
		// stops before the action runs, so a walk-to
		// started by the action survives.
		q.Reset(true)
		action := wt.action
		q.walkTo = nil
		action()
		return
	}

	if len(q.points) > 0 || q.IsMoving() {
		return
	}

	// Route dry (MovementQueue.ts:1599-1602): re-path to
	// where the target stands now. A route keeps the task
	// alive for the next tick; no route fails with the
	// message, as the closure's else branch does (1584-1587).
	// No reset on failure: the queue is already empty.
	q.applyEntityRoute(st.target)
	if len(q.points) > 0 {
		return
	}
	q.owner.SendMessage("I can't reach that!")
	q.walkTo = nil
}

// walkToProjectedState is the pending walk-to-projected-NPC
// interaction: the main-world NPC being walked to from a deck.
type walkToProjectedState struct {
	target EntityTarget
}

// ownerMobile returns the queue owner's Mobile, or nil for an
// owner that is neither a player nor an NPC (a test fake).
func (q *MovementQueue) ownerMobile() *Mobile {
	switch owner := q.owner.(type) {
	case *Player:
		if owner != nil {
			return &owner.Mobile
		}
	case *NPC:
		if owner != nil {
			return &owner.Mobile
		}
	}
	return nil
}

// isProjectedMainWorldNpc reports whether target is an NPC in
// the main world that a player aboard a boat can see and talk
// to from its root (MovementQueue.ts:1715-1723). A target is
// an NPC when it carries an NPCType read (PlayerTarget leaves
// it nil); same-deck and other-boat NPCs keep local deck
// routing, and only main-world actors project. The owner term
// reads the target's owner, as TS reads getOwner.
func (q *MovementQueue) isProjectedMainWorldNpc(target EntityTarget) bool {
	if target.Mobile == nil || target.NPCType == nil {
		return false
	}
	ownerPlayer, ok := q.owner.(*Player)
	if !ok || ownerPlayer == nil {
		return false
	}
	ownerMobile := &ownerPlayer.Mobile
	if q.boatAboard(ownerMobile) == nil {
		return false
	}
	// Same-deck and other-boat NPCs keep local deck routing;
	// only main-world actors project.
	if q.boatAboard(target.Mobile) != nil {
		return false
	}
	if target.Mobile.PrivateArea() != nil {
		return false
	}
	return target.Owner == nil || target.Owner == ownerPlayer
}

// reachedProjectedNpc reports whether the owner, aboard a
// boat, is in reach of a main-world NPC from the boat's root
// tile (MovementQueue.ts:1724-1741): the root within
// max(1, range) + max(0, size-1) of the target's tile, with
// main-world projectile clipping. Go walks at the default
// range (range = 1, as for walkToEntity), and the
// canProjectileMove term degenerates to true: no
// projectile-clip machinery exists in Go (see the section
// header).
func (q *MovementQueue) reachedProjectedNpc(target EntityTarget) bool {
	ownerMobile := q.ownerMobile()
	if ownerMobile == nil || target.Mobile == nil {
		return false
	}
	root := q.rootLocationOf(ownerMobile)
	reach := max(1, 1) + max(0, target.Size-1)
	return root.IsWithinDistance(target.Mobile.Location(), reach)
}

// walkToProjectedNpc is a main-world NPC from a boat's deck:
// done at once when in reach of the boat's root tile;
// otherwise the player walks along the deck towards it
// (BoatManager.deckTileToward) and acts as soon as they're in
// reach. "I can't reach that!" once the deck takes them no
// nearer (MovementQueue.ts:1619-1657). setMobileInteraction
// (TS 1626, 1642): GAP — no interaction state in Go.
func (q *MovementQueue) walkToProjectedNpc(target EntityTarget, action func()) {
	if q.reachedProjectedNpc(target) {
		action()
		return
	}
	ownerMobile := q.ownerMobile()
	var tile Location
	routed := false
	if ownerMobile != nil && q.boats != nil {
		tile, routed = q.boats.DeckTileToward(ownerMobile, target.Mobile.Location(), target.Size)
	}
	q.Reset(true)
	if !routed {
		q.owner.SendMessage("I can't reach that!")
		return
	}
	// walkToReset (MovementQueue.ts:1634): cancel any pending
	// walk-to and stop the player's current skill action —
	// the 6th walkToReset site, closed by this branch.
	q.walkToReset()
	q.applyWalkRoute(tile.X, tile.Y)
	q.walkTo = &walkToState{
		destination: tile,
		action:      action,
		projected:   &walkToProjectedState{target: target},
	}
}

// tickWalkToProjectedNpc mirrors the walkToProjectedNpc
// MovementTask closure (MovementQueue.ts:1636-1656). In order:
// an unregistered or no-longer-projected target resets and
// stops silently; arrival resets, stops, and runs the action;
// pending checkpoints or movement return without
// re-evaluating; otherwise the queue has run dry without
// reaching, so it resets, stops, and sends the message. There
// is no re-path: TS routes once at call time.
func (q *MovementQueue) tickWalkToProjectedNpc(wt *walkToState) {
	st := wt.projected

	if st.target.Mobile == nil || !st.target.Mobile.IsRegistered() || !q.isProjectedMainWorldNpc(st.target) {
		q.Reset(true)
		q.walkTo = nil
		return
	}

	// setMobileInteraction (MovementQueue.ts:1642): GAP — no
	// interaction state in Go.

	if q.reachedProjectedNpc(st.target) {
		// Arrival: reset, stop, run the action. The task
		// stops before the action runs, so a walk-to
		// started by the action survives.
		q.Reset(true)
		action := wt.action
		q.walkTo = nil
		action()
		return
	}

	if len(q.points) > 0 || q.IsMoving() {
		return
	}

	q.Reset(true)
	q.walkTo = nil
	q.owner.SendMessage("I can't reach that!")
}
