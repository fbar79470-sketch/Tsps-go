package game

import (
	"testing"

	"tsps-go/internal/definition"
)

// fakeNPCDefinition is a hand-built NPCDefinition for driver
// tests. Zero values are the DEFAULT shape except where the
// test sets a field; AggressionRange mirrors
// NpcDefinition.ts:332-335.
type fakeNPCDefinition struct {
	size        int
	aggressive  bool
	tolerance   bool
	followDist  int
	hunt        int
	combatLevel int
	respawn     int
	hitpoints   int
	attackType  definition.CombatType
}

func (d *fakeNPCDefinition) Size() int                       { return d.size }
func (d *fakeNPCDefinition) IsAggressive() bool              { return d.aggressive }
func (d *fakeNPCDefinition) BuildsAggressionTolerance() bool { return d.tolerance }
func (d *fakeNPCDefinition) CombatFollowDistance() int       { return d.followDist }
func (d *fakeNPCDefinition) AggressionRange(attackDistance float64) int {
	reach := 1
	if attackDistance == attackDistance && attackDistance < 1e15 && attackDistance > -1e15 {
		if truncated := int(attackDistance); truncated > 1 {
			reach = truncated
		}
	}
	follow := d.followDist
	if follow < 0 {
		follow = 0
	}
	return follow + reach
}
func (d *fakeNPCDefinition) HuntRange() int                    { return d.hunt }
func (d *fakeNPCDefinition) CombatLevel() int                  { return d.combatLevel }
func (d *fakeNPCDefinition) Respawn() int                      { return d.respawn }
func (d *fakeNPCDefinition) Hitpoints() int                    { return d.hitpoints }
func (d *fakeNPCDefinition) AttackType() definition.CombatType { return d.attackType }

// defaultFakeDef returns the DEFAULT-shape fake definition:
// size 1, non-aggressive, builds tolerance, follow 7, hunt 3,
// combat level 0, respawn 25, 10 hitpoints, melee.
func defaultFakeDef() *fakeNPCDefinition {
	return &fakeNPCDefinition{
		size: 1, tolerance: true, followDist: 7, hunt: 3,
		respawn: 25, hitpoints: 10, attackType: definition.CombatTypeMelee,
	}
}

// aggressiveFakeDef returns a definition for an aggressive
// hunter: combat level 50 (so the level check passes for a
// default player), hunt 3, follow 7.
func aggressiveFakeDef() *fakeNPCDefinition {
	d := defaultFakeDef()
	d.aggressive = true
	d.combatLevel = 50
	return d
}

func TestDefaultNPCDefinitionShape(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10})
	def := n.Definition()
	if def.Size() != 1 || def.IsAggressive() || !def.BuildsAggressionTolerance() ||
		def.CombatFollowDistance() != 7 || def.HuntRange() != 3 || def.CombatLevel() != 0 ||
		def.Respawn() != 25 || def.Hitpoints() != 10 || def.AttackType() != definition.CombatTypeMelee {
		t.Fatalf("DEFAULT shape mismatch: %+v", def)
	}
	if got := def.AggressionRange(1); got != 8 {
		t.Errorf("AggressionRange(1) = %d, want 8", got)
	}
	if got := def.AggressionRange(6); got != 13 {
		t.Errorf("AggressionRange(6) = %d, want 13", got)
	}
	if n.Size() != 1 {
		t.Errorf("Size() = %d, want 1", n.Size())
	}
}

func TestNPCDefinitionAttackDistance(t *testing.T) {
	cases := []struct {
		attackType definition.CombatType
		want       int
	}{
		{definition.CombatTypeMelee, 1},
		{definition.CombatTypeRanged, 6},
		{definition.CombatTypeMagic, 10},
	}
	for _, tc := range cases {
		n := NewNPC(1, Location{})
		d := defaultFakeDef()
		d.attackType = tc.attackType
		n.SetDefinition(d)
		if got := n.attackDistance(); got != tc.want {
			t.Errorf("attackType %v: attackDistance = %d, want %d", tc.attackType, got, tc.want)
		}
	}
}

// cycleRandom returns a random func cycling through values, so
// wander tests control the roll and both coordinate rolls
// exactly (three random calls per wander roll).
func cycleRandom(values ...float64) func() float64 {
	i := 0
	return func() float64 {
		v := values[i%len(values)]
		i++
		return v
	}
}

func TestCoordinatorStationaryRadiusZero(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	coord := n.MovementCoordinator()
	coord.random = cycleRandom(0.0) // would always wander if it rolled
	for cycle := int64(1); cycle <= 20; cycle++ {
		coord.Process(cycle)
		n.MovementQueue().Process(cycle)
	}
	if !n.Location().Equals(spawn) {
		t.Errorf("radius-0 NPC moved to %+v, want spawn %+v", n.Location(), spawn)
	}
	if got := coord.GetCoordinateState(); got != CoordinateStateHome {
		t.Errorf("state = %v, want HOME", got)
	}
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("queued points = %d, want 0", got)
	}
}

func TestCoordinatorNoWanderWhenRollFails(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	coord.random = cycleRandom(0.5) // 0.5 >= 0.26: never wanders
	for cycle := int64(1); cycle <= 20; cycle++ {
		n.Process(cycle)
	}
	if !n.Location().Equals(spawn) {
		t.Errorf("NPC moved to %+v without a wander roll", n.Location())
	}
}

func TestCoordinatorWanderStaysWithinRadius(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	// Roll 0.0 (wander), dx roll 0.0 (dx -5), dy roll 0.99 (dy +5):
	// every wander targets the north-west corner of the square.
	coord.random = cycleRandom(0.0, 0.0, 0.99)
	moved := false
	for cycle := int64(1); cycle <= 60; cycle++ {
		n.Process(cycle)
		loc := n.Location()
		if dx := loc.X - spawn.X; dx < -5 || dx > 5 {
			t.Fatalf("cycle %d: x offset %d outside radius 5 (loc %+v)", cycle, dx, loc)
		}
		if dy := loc.Y - spawn.Y; dy < -5 || dy > 5 {
			t.Fatalf("cycle %d: y offset %d outside radius 5 (loc %+v)", cycle, dy, loc)
		}
		if !loc.Equals(spawn) {
			moved = true
		}
	}
	if !moved {
		t.Error("NPC never left its spawn tile despite forced wander rolls")
	}
	if got := n.Location(); !got.Equals(Location{X: 95, Y: 105, Z: 0}) {
		t.Errorf("final loc = %+v, want the wander corner (95, 105)", got)
	}
}

func TestCoordinatorWanderToCurrentTileQueuesNothing(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	// Roll 0.0 (wander), dx/dy rolls 0.5: floor(0.5*10-5+0.5) = 0.
	coord.random = cycleRandom(0.0, 0.5, 0.5)
	coord.Process(1)
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("queued points = %d, want 0 (wander target is the current tile)", got)
	}
}

func TestCoordinatorWalkHome(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	n.SetLocation(Location{X: 120, Y: 108, Z: 0})
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	coord.random = cycleRandom(0.0)
	for cycle := int64(1); cycle <= 40; cycle++ {
		n.Process(cycle)
		if n.Location().Equals(spawn) {
			break
		}
	}
	if !n.Location().Equals(spawn) {
		t.Fatalf("NPC did not walk home: at %+v, want %+v", n.Location(), spawn)
	}
	// One more tick lets updateCoordinator observe the arrival.
	n.Process(41)
	if got := coord.GetCoordinateState(); got != CoordinateStateHome {
		t.Errorf("state = %v, want HOME after arriving", got)
	}
}

func TestCoordinatorRadiusZeroDisplacedStaysPut(t *testing.T) {
	// The stationary rule is absolute for a HOME radius-0 NPC:
	// the coordinator returns before updateCoordinator, so a
	// displaced NPC never walks home on its own. Only combat
	// marks it RETREATING (Combat.ts:834, 861), which re-arms
	// the coordinator — and combat is not this test's subject.
	spawn := Location{X: 100, Y: 100, Z: 0}
	displaced := Location{X: 120, Y: 108, Z: 0}
	n := NewNPC(1, spawn)
	n.SetLocation(displaced)
	coord := n.MovementCoordinator()
	coord.random = cycleRandom(0.0)
	for cycle := int64(1); cycle <= 10; cycle++ {
		n.Process(cycle)
	}
	if !n.Location().Equals(displaced) {
		t.Errorf("displaced radius-0 NPC moved to %+v, want %+v", n.Location(), displaced)
	}
	if got := coord.GetCoordinateState(); got != CoordinateStateHome {
		t.Errorf("state = %v, want HOME (never re-evaluated)", got)
	}
}

func TestCoordinatorRadiusZeroAwayWalksHome(t *testing.T) {
	// A radius-0 NPC already AWAY (combat marked it, or a test
	// did) does walk home: the early return only fires for HOME.
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	n.SetLocation(Location{X: 105, Y: 100, Z: 0})
	coord := n.MovementCoordinator()
	coord.SetCoordinateState(CoordinateStateAway)
	coord.random = cycleRandom(0.0)
	for cycle := int64(1); cycle <= 10; cycle++ {
		n.Process(cycle)
		if n.Location().Equals(spawn) {
			break
		}
	}
	if !n.Location().Equals(spawn) {
		t.Fatalf("AWAY radius-0 NPC did not walk home: at %+v", n.Location())
	}
}

func TestCoordinatorUpdateStates(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	cases := []struct {
		name string
		loc  Location
		want CoordinateState
	}{
		{"at spawn", spawn, CoordinateStateHome},
		{"radius edge", Location{X: 105, Y: 100, Z: 0}, CoordinateStateHome},
		{"one past radius", Location{X: 106, Y: 100, Z: 0}, CoordinateStateAway},
		{"diagonal past", Location{X: 103, Y: 106, Z: 0}, CoordinateStateAway},
		{"other plane same tile", Location{X: 100, Y: 100, Z: 1}, CoordinateStateHome},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := NewNPC(1, spawn)
			n.SetLocation(tc.loc)
			coord := n.MovementCoordinator()
			coord.SetRadius(5)
			coord.updateCoordinator()
			if got := coord.GetCoordinateState(); got != tc.want {
				t.Errorf("state = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCoordinatorImmobile(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	t.Run("away stays away with no route", func(t *testing.T) {
		n := NewNPC(1, spawn)
		n.SetLocation(Location{X: 120, Y: 100, Z: 0})
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.SetCoordinateState(CoordinateStateAway)
		coord.random = cycleRandom(0.0)
		n.MovementQueue().SetBlockMovement(true)
		coord.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("blocked NPC queued %d points", got)
		}
		if got := coord.GetCoordinateState(); got != CoordinateStateAway {
			t.Errorf("state = %v, want AWAY (not at spawn)", got)
		}
	})
	t.Run("at spawn marks home", func(t *testing.T) {
		n := NewNPC(1, spawn)
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.SetCoordinateState(CoordinateStateAway)
		n.MovementQueue().SetBlockMovement(true)
		coord.Process(1)
		if got := coord.GetCoordinateState(); got != CoordinateStateHome {
			t.Errorf("state = %v, want HOME", got)
		}
	})
	t.Run("frozen never wanders", func(t *testing.T) {
		n := NewNPC(1, spawn)
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.random = cycleRandom(0.0)
		n.GetTimers().Registers(TimerKeyFreeze, 5)
		coord.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("frozen NPC queued %d points", got)
		}
	})
}

func TestCoordinatorInCombat(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	t.Run("home in combat does not wander", func(t *testing.T) {
		n := NewNPC(1, spawn)
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.random = cycleRandom(0.0)
		coord.inCombatCheck = func(*NPC) bool { return true }
		coord.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("in-combat NPC queued %d wander points", got)
		}
	})
	t.Run("away becomes retreating and resets", func(t *testing.T) {
		n := NewNPC(1, spawn)
		n.SetLocation(Location{X: 120, Y: 100, Z: 0})
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.SetCoordinateState(CoordinateStateAway)
		coord.inCombatCheck = func(*NPC) bool { return true }
		reset := 0
		coord.retreatReset = func(*NPC) { reset++ }
		coord.Process(1)
		if got := coord.GetCoordinateState(); got != CoordinateStateRetreating {
			t.Errorf("state = %v, want RETREATING", got)
		}
		if reset != 1 {
			t.Errorf("retreat resets = %d, want 1", reset)
		}
	})
	t.Run("retreating at spawn returns home", func(t *testing.T) {
		n := NewNPC(1, spawn)
		coord := n.MovementCoordinator()
		coord.SetRadius(5)
		coord.SetCoordinateState(CoordinateStateRetreating)
		coord.inCombatCheck = func(*NPC) bool { return true }
		coord.updateCoordinator()
		if got := coord.GetCoordinateState(); got != CoordinateStateHome {
			t.Errorf("state = %v, want HOME", got)
		}
	})
}

func TestCoordinatorInteractingDoesNotWander(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	p := NewPlayer("alice", Location{X: 101, Y: 100, Z: 0})
	n.SetMobileInteraction(p)
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	coord.random = cycleRandom(0.0)
	coord.Process(1)
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("interacting NPC queued %d wander points", got)
	}
}

func TestCoordinatorBlockedStepReset(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	// A wall everywhere: every step is refused, so the step-blocked
	// flag sets and the coordinator drops the checkpoint.
	n.MovementQueue().SetCollision(blockAllCollision{})
	n.MovementQueue().AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	n.MovementQueue().Process(1)
	if !n.MovementQueue().IsStepBlocked() {
		t.Fatal("step should be blocked against the wall")
	}
	coord.random = cycleRandom(0.9) // no new wander after the reset
	coord.Process(2)
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("queued points after blocked reset = %d, want 0", got)
	}
}

type blockAllCollision struct{}

func (blockAllCollision) CanMoveStart(start, end Location, size int) bool { return false }

func TestCoordinatorRetreatRouteActiveAndCooldown(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	n := NewNPC(1, spawn)
	n.SetLocation(Location{X: 110, Y: 100, Z: 0})
	coord := n.MovementCoordinator()
	coord.SetRadius(5)
	coord.Process(10)
	points := n.MovementQueue().Points()
	if len(points) == 0 {
		t.Fatal("walk-home route queued no points")
	}
	// While the retreat route is active, re-processing with a
	// later cycle must not rebuild it.
	coord.Process(11)
	again := n.MovementQueue().Points()
	if len(again) != len(points) {
		t.Fatalf("route rebuilt while active: %d points, want %d", len(again), len(points))
	}
	for i := range points {
		if points[i] != again[i] {
			t.Fatalf("point %d changed while route active: %+v -> %+v", i, points[i], again[i])
		}
	}
	// Drop the route: the cooldown (cycle 10 + 1) blocks an
	// immediate re-route on the same cycle, then allows it.
	n.MovementQueue().Reset(false)
	coord.Process(10)
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("re-routed during cooldown: %d points", got)
	}
	coord.Process(11)
	if got := n.MovementQueue().Size(); got == 0 {
		t.Error("no re-route after the cooldown elapsed")
	}
}

func TestCoordinatorAccessors(t *testing.T) {
	n := NewNPC(1, Location{X: 1, Y: 1})
	coord := n.MovementCoordinator()
	if coord.GetRadius() != 0 || coord.GetCoordinateState() != CoordinateStateHome {
		t.Errorf("fresh coordinator: radius=%d state=%v", coord.GetRadius(), coord.GetCoordinateState())
	}
	coord.SetRadius(7)
	coord.SetCoordinateState(CoordinateStateAway)
	if coord.GetRadius() != 7 || coord.GetCoordinateState() != CoordinateStateAway {
		t.Errorf("after set: radius=%d state=%v", coord.GetRadius(), coord.GetCoordinateState())
	}
	if got := CoordinateStateRetreating.String(); got != "RETREATING" {
		t.Errorf("String() = %q", got)
	}
}

func TestNPCMovementStepsClamp(t *testing.T) {
	n := NewNPC(1, Location{})
	if got := n.GetMovementSteps(); got != 1 {
		t.Fatalf("movementSteps = %d, want 1", got)
	}
	n.SetMovementSteps(5)
	if got := n.GetMovementSteps(); got != 2 {
		t.Errorf("movementSteps = %d, want clamped 2", got)
	}
	n.SetMovementSteps(0)
	if got := n.GetMovementSteps(); got != 1 {
		t.Errorf("movementSteps = %d, want clamped 1", got)
	}
}

func TestNPCFaceAccessors(t *testing.T) {
	n := NewNPC(1, Location{})
	if got := n.GetFace(); got != DirSouth {
		t.Fatalf("face = %v, want SOUTH default", got)
	}
	n.SetFace(DirNorth)
	if got := n.GetFace(); got != DirNorth {
		t.Errorf("face = %v, want NORTH", got)
	}
	n.SetFace(DirNone)
	if got := n.GetFace(); got != DirSouth {
		t.Errorf("face = %v, want SOUTH for an unknown direction", got)
	}
	n.SetSpawnPosition(Location{X: 5, Y: 6, Z: 1})
	if n.Spawn != (Location{X: 5, Y: 6, Z: 1}) {
		t.Errorf("Spawn = %+v", n.Spawn)
	}
}

func TestNPCMovementOwnerConformance(t *testing.T) {
	// The queue must step an NPC owner without touching player
	// state: one step per tick, facing updated, no run energy
	// or destination traffic (the sends are no-ops).
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	if n.IsPlayer() || n.IsPlayerBot() || n.IsRunning() {
		t.Error("NPC owner predicates must read non-player")
	}
	if got := n.GetRunEnergyUnits(); got != RunEnergyMaxUnits {
		t.Errorf("run energy = %d, want max", got)
	}
	q := n.MovementQueue()
	q.AddCheckpoint(Location{X: 12, Y: 10, Z: 0})
	q.Process(1)
	if got := n.Location(); !got.Equals(Location{X: 11, Y: 10, Z: 0}) {
		t.Errorf("after one tick at %+v, want (11, 10)", got)
	}
	if n.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST", n.Facing)
	}
	if q.IsRunToggled() {
		t.Error("IsRunToggled must read false for a non-player owner")
	}
}
