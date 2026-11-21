package game

import (
	"math"
	"math/rand"
)

// NPCMovementCoordinator is the port of
// game/entity/impl/npc/NPCMovementCoordinator.ts (157 lines):
// wandering within a radius of home, walking home when driven
// away, and the radius-0 stationary rule. One coordinator is
// constructed per NPC (NPC.ts:126) and runs from NPC.Process
// unless the NPC is scripted or held by dialogue.
//
// The coordinator never steps the NPC itself: it queues walks
// on the NPC's MovementQueue (a wander via tile-by-tile
// checkpoints, a walk home via the route finder), and the
// queue's Process steps them later in the same turn.
//
// ROUTING. The walk-home route replicates PathFinder
// .calculateWalkRoute plus the applyRsmodRoute queue half
// (PathFinder.ts:128-208) through the coordinator's own
// RouteFinder, because the queue's route application is
// player-gated (RequestWalk returns for non-players) and its
// finder is private. The finder's clipping starts nil (open
// ground); the world wiring must point it at the world's
// RegionManager through SetClipping, alongside the queue's own
// SetClipping, or walk-home routes ignore walls. The wander
// path needs no route finder: it expands a straight line of
// checkpoints (MovementQueue.addSteps), and the queue's step
// validation still refuses blocked steps.
//
// RANDOMNESS. Wandering rolls math/rand through the random
// field, so tests can install a deterministic sequence. The
// default is the shared rand.Float64.

// CoordinateState is the coordinator's home/away state
// (NPCMovementCoordinator.ts:151-155, CoordinateState).
type CoordinateState int

const (
	// CoordinateStateHome: the NPC is within its wander radius
	// of its spawn.
	CoordinateStateHome CoordinateState = iota
	// CoordinateStateAway: the NPC is outside its wander
	// radius, out of combat.
	CoordinateStateAway
	// CoordinateStateRetreating: the NPC is outside its wander
	// radius while in combat, or walking home from combat.
	CoordinateStateRetreating
)

// String names the state for debugging. GO-ONLY: TS enums have
// no names at runtime beyond their keys, which these match.
func (s CoordinateState) String() string {
	switch s {
	case CoordinateStateHome:
		return "HOME"
	case CoordinateStateAway:
		return "AWAY"
	case CoordinateStateRetreating:
		return "RETREATING"
	default:
		return "UNKNOWN"
	}
}

// retreatRepathCooldownTicks is RETREAT_REPATH_COOLDOWN_TICKS
// (NPCMovementCoordinator.ts:6): a walk-home route is
// recalculated at most once per tick.
const retreatRepathCooldownTicks = 1

// wanderChance is the per-tick wander roll
// (NPCMovementCoordinator.ts:44-48): 0.26, the ~26% per server
// tick the HD-client 10/1000-per-client-tick roll implies.
const wanderChance = 0.26

// NPCMovementCoordinator keeps one NPC near its spawn. See the
// file header. It is tick-goroutine-only, like the NPC it
// drives; the seams (inCombatCheck, retreatReset, random) are
// installed before ticking starts or by same-package tests.
type NPCMovementCoordinator struct {
	npc                    *NPC
	state                  CoordinateState
	radius                 int
	nextRetreatRepathCycle int64

	// routeFinder routes walk-home (and follow) walks. It is
	// lazily allocated (see finder): a RouteFinder holds
	// ~164KB of search scratch, and most NPCs never route
	// home. Its clipping starts nil; see SetClipping.
	routeFinder *RouteFinder
	clipping    CollisionLookup

	// random supplies wander rolls in [0, 1). The default is
	// rand.Float64; tests install deterministic sequences.
	random func() float64

	// inCombatCheck, when set, answers CombatFactory.inCombat
	// for the NPC. When nil, npcInCombat (the HasPendingWork
	// proxy) answers; see npcInCombat in npc.go. The combat
	// wiring installs the exact predicate once Combat exposes
	// target/attacker.
	inCombatCheck func(n *NPC) bool

	// retreatReset, when set, runs the combat reset the
	// RETREATING branch of updateCoordinator performs
	// (NPCMovementCoordinator.ts:103: getCombat().reset()).
	// When nil, no reset runs: the Combat stub exposes no
	// Reset, and the combat-core work owns it. The seam lets
	// that work install `func(n *NPC) { n.GetCombat().Reset() }`
	// without touching the coordinator.
	retreatReset func(n *NPC)
}

// NewNPCMovementCoordinator returns a coordinator for npc, in
// the HOME state with radius 0 (NPCMovementCoordinator.ts:10-16,
// the constructor). Radius 0 with HOME means stationary: the
// spawn path sets the real radius.
func NewNPCMovementCoordinator(npc *NPC) *NPCMovementCoordinator {
	return &NPCMovementCoordinator{
		npc:    npc,
		state:  CoordinateStateHome,
		random: rand.Float64,
	}
}

// finder returns the coordinator's RouteFinder, allocating it
// on first use with the clipping lookup SetClipping last
// installed. Tick-goroutine-only, like the coordinator.
func (c *NPCMovementCoordinator) finder() *RouteFinder {
	if c.routeFinder == nil {
		c.routeFinder = NewRouteFinder(c.clipping)
	}
	return c.routeFinder
}

// SetClipping replaces the clipping lookup the walk-home route
// finder reads, dropping the finder so the next route rebuilds
// it against the new lookup. A nil lookup means open ground. The
// world wiring calls it with the world's RegionManager lookup
// when the NPC is added, alongside the queue's SetClipping;
// until then walk-home routes ignore clipping. It mirrors
// MovementQueue.SetClipping.
func (c *NPCMovementCoordinator) SetClipping(clipping CollisionLookup) {
	c.clipping = clipping
	c.routeFinder = nil
}

// GetCoordinateState returns the coordinator state
// (NPCMovementCoordinator.ts:135-137).
func (c *NPCMovementCoordinator) GetCoordinateState() CoordinateState { return c.state }

// SetCoordinateState sets the coordinator state
// (NPCMovementCoordinator.ts:139-141).
func (c *NPCMovementCoordinator) SetCoordinateState(state CoordinateState) { c.state = state }

// GetRadius returns the wander radius
// (NPCMovementCoordinator.ts:143-145).
func (c *NPCMovementCoordinator) GetRadius() int { return c.radius }

// SetRadius sets the wander radius
// (NPCMovementCoordinator.ts:147-149). It stores the value as
// given; the spawn loader clamps loaded radii to >= 0 before
// calling it (see internal/definition).
func (c *NPCMovementCoordinator) SetRadius(radius int) { c.radius = radius }

// inCombat answers whether the NPC is in combat, through the
// installed check or the HasPendingWork proxy (see the
// inCombatCheck field comment).
func (c *NPCMovementCoordinator) inCombat() bool {
	if c.inCombatCheck != nil {
		return c.inCombatCheck(c.npc)
	}
	return npcInCombat(c.npc)
}

// Process runs one coordinator turn for cycle, in TS order
// (NPCMovementCoordinator.ts:18-55):
//
//	radius-0 + HOME → return; immobile → mark HOME when at spawn
//	and return; updateCoordinator; HOME → maybe wander;
//	AWAY/RETREATING → walk home.
//
// The active-region gate on wandering (NPCMovementCoordinator
// .ts:47, World.isLocationActive) is OMITTED: Go's World
// processes every registered NPC every tick with no region
// gating (see world.go processNPCs), so every processed NPC is
// "active" by the world's own definition, and the gate would
// always pass. A region-gated world would need the gate back.
func (c *NPCMovementCoordinator) Process(cycle int64) {
	if c.radius == 0 && c.state == CoordinateStateHome {
		return
	}
	queue := c.npc.movementQueue
	if !c.npc.canMove() {
		// An immobile NPC already home has finished
		// retreating without taking a step.
		if c.npc.Location().Equals(c.npc.Spawn) {
			c.state = CoordinateStateHome
		}
		return
	}
	c.updateCoordinator()
	switch c.state {
	case CoordinateStateHome:
		if c.inCombat() {
			return
		}
		if c.npc.interacting.present() {
			return
		}
		// A blocked step keeps its checkpoint forever; drop
		// it so the NPC can roll a new wander.
		if queue != nil && queue.IsStepBlocked() {
			queue.Reset(true)
		}
		if c.random() < wanderChance {
			c.wander()
		}
	case CoordinateStateAway, CoordinateStateRetreating:
		c.processRetreatingMovement(cycle)
	}
}

// processRetreatingMovement walks the NPC home
// (NPCMovementCoordinator.ts:57-82): at the spawn tile the
// state returns to HOME; while a retreat route is still active
// (its destination is the spawn and checkpoints remain or the
// NPC is moving) nothing is re-routed; otherwise a walk route
// to the spawn is calculated at most once per tick.
func (c *NPCMovementCoordinator) processRetreatingMovement(cycle int64) {
	spawn := c.npc.Spawn
	if c.npc.Location().Equals(spawn) {
		c.state = CoordinateStateHome
		c.nextRetreatRepathCycle = 0
		return
	}
	queue := c.npc.movementQueue
	if queue == nil {
		return
	}
	retreatRouteActive :=
		queue.LastDestX == spawn.X &&
			queue.LastDestY == spawn.Y &&
			(queue.Size() > 0 || queue.IsMoving())
	if retreatRouteActive {
		return
	}
	if cycle < c.nextRetreatRepathCycle {
		return
	}
	c.nextRetreatRepathCycle = cycle + retreatRepathCooldownTicks
	c.routeTo(spawn)
}

// updateCoordinator refreshes the coordinate state
// (NPCMovementCoordinator.ts:84-125). In combat, AWAY becomes
// RETREATING, and a RETREATING NPC at its spawn returns HOME
// while its combat is reset (through the retreatReset seam;
// see the field comment). Out of combat, the state follows the
// Chebyshev distance from the spawn against the radius: beyond
// it on either axis is AWAY, within it is HOME (and the repath
// cooldown clears). The plane plays no part in TS — the deltas
// are x/y only — and none here either.
func (c *NPCMovementCoordinator) updateCoordinator() {
	if c.inCombat() {
		if c.state == CoordinateStateAway {
			c.state = CoordinateStateRetreating
		}
		if c.state == CoordinateStateRetreating {
			if c.npc.Location().Equals(c.npc.Spawn) {
				c.state = CoordinateStateHome
			}
			if c.retreatReset != nil {
				c.retreatReset(c.npc)
			}
		}
		return
	}
	spawn := c.npc.Spawn
	current := c.npc.Location()
	deltaX := spawn.X - current.X
	if deltaX < 0 {
		deltaX = -deltaX
	}
	deltaY := spawn.Y - current.Y
	if deltaY < 0 {
		deltaY = -deltaY
	}
	if deltaX > c.radius || deltaY > c.radius {
		c.state = CoordinateStateAway
	} else {
		c.state = CoordinateStateHome
		c.nextRetreatRepathCycle = 0
	}
}

// wander rolls a destination in the radius square around the
// spawn and walks to it (NPCMovementCoordinator.ts:127-134):
// each axis is Math.round(random * radius * 2 - radius), the
// destination is the spawn translated by the roll (same plane),
// and a destination other than the current tile replaces the
// queued walk with tile-by-tile steps to it. A roll that lands
// on the current tile queues nothing.
func (c *NPCMovementCoordinator) wander() {
	spawn := c.npc.Spawn
	// math.Floor(v + 0.5) is JavaScript Math.round: halves round
	// toward +Inf (2.5 -> 3, -2.5 -> -2), where Go's math.Round
	// rounds halves away from zero (-2.5 -> -3).
	dx := int(math.Floor(c.random()*float64(c.radius)*2 - float64(c.radius) + 0.5))
	dy := int(math.Floor(c.random()*float64(c.radius)*2 - float64(c.radius) + 0.5))
	dest := spawn.Transform(dx, dy)
	if dest.Equals(c.npc.Location()) {
		return
	}
	queue := c.npc.movementQueue
	if queue == nil {
		return
	}
	queue.Reset(true)
	c.addSteps(dest)
}

// addSteps expands a straight line of tile checkpoints from the
// NPC's current tile to dest, replicating MovementQueue.addSteps
// (MovementQueue.ts:286-309) through the queue's exported
// AddCheckpoint: the queue was just reset, so the expansion
// starts from the current tile (TS getLast after a reset), each
// iteration steps each axis one tile toward the destination,
// and each intermediate tile becomes a checkpoint. The stored
// checkpoint directions differ (AddCheckpoint stores DirNone;
// TS addStep stores the step direction), but the stepping loop
// recomputes directions from deltas, so the stored values are
// informational only (see movementPoint). AddCheckpoint applies
// the same mobility and 100-point cap gates TS addStep applies.
func (c *NPCMovementCoordinator) addSteps(dest Location) {
	queue := c.npc.movementQueue
	if queue == nil {
		return
	}
	last := c.npc.Location()
	deltaX := dest.X - last.X
	deltaY := dest.Y - last.Y
	steps := max(deltaX, -deltaX, deltaY, -deltaY)
	for i := 0; i < steps; i++ {
		if deltaX < 0 {
			deltaX++
		} else if deltaX > 0 {
			deltaX--
		}
		if deltaY < 0 {
			deltaY++
		} else if deltaY > 0 {
			deltaY--
		}
		queue.AddCheckpoint(Location{X: dest.X - deltaX, Y: dest.Y - deltaY, Z: dest.Z})
	}
}

// routeTo routes the NPC to dest through the route finder and
// queues the resulting checkpoints, replicating the
// calculateWalkRoute entry (PathFinder.ts:412-422: locShape -1,
// moveNear true) plus the applyRsmodRoute queue-mutation half
// (PathFinder.ts:128-208): reset without a destination sync,
// lastDestX/lastDestY, setRoute, setPathX/setPathY with the
// region-base math, one checkpoint per waypoint, and a
// destination-flag sync. It returns the number of checkpoints
// queued. The plugin emitNoPath event on failure is omitted:
// no plugin system exists. The shape mirrors applyWalkRoute in
// movement_queue.go, which the coordinator cannot call (it is
// private, and RequestWalk is player-gated).
func (c *NPCMovementCoordinator) routeTo(dest Location) int {
	queue := c.npc.movementQueue
	if queue == nil {
		return 0
	}
	queue.Reset(false)
	queue.LastDestX = dest.X
	queue.LastDestY = dest.Y
	src := c.npc.Location()
	regionBaseX := src.RegionX() << 3
	regionBaseY := src.RegionY() << 3
	route := WalkRoute(c.finder(), src, max(c.npc.Size(), 1), dest.X, dest.Y)
	if !route.Success {
		queue.SetRoute(false, route.Alternative)
		queue.SyncDestinationFlagToRoute()
		return 0
	}
	queue.SetRoute(true, route.Alternative)
	queue.SetPathX(route.EndX - regionBaseX)
	queue.SetPathY(route.EndY - regionBaseY)
	steps := 0
	for _, waypoint := range route.Waypoints {
		queue.AddCheckpoint(Location{X: waypoint.X, Y: waypoint.Y, Z: waypoint.Z})
		steps++
	}
	queue.SyncDestinationFlagToRoute()
	return steps
}
