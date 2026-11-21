package game

import (
	"testing"
	"time"
)

// fakeOwner is a MovementOwner test double: position, footprint,
// player/run state, and step directions, with Mobile's facing rule
// (a step direction with id >= 0 becomes the facing). Energy state
// defaults to a fresh player: max units, Agility 1, weight 0, not a
// bot; tests set the fields directly to stage drain/auto-stop/bot
// cases.
type fakeOwner struct {
	loc     Location
	size    int
	player  bool
	running bool
	region  Location

	// bot, when true, makes IsPlayerBot report a player bot.
	bot bool
	// energyUnits/agility/weightKg are the live run-energy state
	// the queue's drain reads and writes (see the accessors below).
	energyUnits int
	agility     int
	weightKg    int

	walkingDir Direction
	runningDir Direction
	facing     Direction

	// runEnergy records SendRunEnergy calls; destinations records
	// SendDestination calls; messages records SendMessage calls;
	// typedMessages records SendTypedMessage calls; timers is
	// the owner's timer repository (nil by default, so the
	// mobility gates never fire unless a test installs one).
	runEnergy     [][4]any
	destinations  [][2]int
	messages      []string
	typedMessages [][3]any
	timers        *TimerRepository
}

func newFakeOwner(loc Location) *fakeOwner {
	return &fakeOwner{
		loc: loc, size: 1, player: true, running: true, region: loc,
		energyUnits: RunEnergyMaxUnits, agility: 1,
		walkingDir: DirNone, runningDir: DirNone, facing: DirNone,
	}
}

func (o *fakeOwner) Location() Location       { return o.loc }
func (o *fakeOwner) SetLocation(loc Location) { o.loc = loc }
func (o *fakeOwner) Size() int                { return o.size }
func (o *fakeOwner) IsPlayer() bool           { return o.player }
func (o *fakeOwner) IsPlayerBot() bool        { return o.bot }
func (o *fakeOwner) IsRunning() bool          { return o.running }
func (o *fakeOwner) SetRunning(running bool)  { o.running = running }
func (o *fakeOwner) GetRunEnergyUnits() int   { return o.energyUnits }
func (o *fakeOwner) SetRunEnergyUnits(units int) {
	if units < 0 {
		units = 0
	} else if units > RunEnergyMaxUnits {
		units = RunEnergyMaxUnits
	}
	o.energyUnits = units
}
func (o *fakeOwner) GetCurrentAgilityLevel() int {
	if o.agility < 0 {
		o.agility = 0
		return 0
	}
	return o.agility
}
func (o *fakeOwner) GetWeightKg() int { return o.weightKg }
func (o *fakeOwner) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
	o.runEnergy = append(o.runEnergy, [4]any{units, running, weightKg, staminaTicks})
}
func (o *fakeOwner) SendDestination(x, y int) {
	o.destinations = append(o.destinations, [2]int{x, y})
}
func (o *fakeOwner) SendMessage(text string) {
	o.messages = append(o.messages, text)
}
func (o *fakeOwner) SendTypedMessage(chatType int, message, from string) {
	o.typedMessages = append(o.typedMessages, [3]any{chatType, message, from})
}
func (o *fakeOwner) GetTimers() *TimerRepository   { return o.timers }
func (o *fakeOwner) GetLastKnownRegion() Location  { return o.region }
func (o *fakeOwner) SetLastKnownRegion(r Location) { o.region = r }
func (o *fakeOwner) SetWalkingDirection(d Direction) {
	o.walkingDir = d
	if int(d) >= 0 {
		o.facing = d
	}
}
func (o *fakeOwner) SetRunningDirection(d Direction) {
	o.runningDir = d
	if int(d) >= 0 {
		o.facing = d
	}
}

// fakeCollision blocks steps ending on a configured tile (or every
// step, when blockAll), standing in for RegionManager collision.
type fakeCollision struct {
	blocked  map[Location]bool
	blockAll bool
}

func (c *fakeCollision) CanMoveStart(start, end Location, size int) bool {
	if c.blockAll {
		return false
	}
	return !c.blocked[end]
}

func boolPtr(b bool) *bool { return &b }

// tick mirrors Player.Process's movement step: begin the cycle, then
// process when the queue has pending work.
func tickQueue(q *MovementQueue, cycle int64) {
	q.BeginCycle()
	if q.HasPendingWork() {
		q.Process(cycle)
	}
}

func TestLocationTransformPreservesZ(t *testing.T) {
	// Location.ts:134-135: transform = clone().addX().addY(); clone
	// copies z (138-144).
	l := Location{X: 100, Y: 200, Z: 2}
	got := l.Transform(3, -5)
	if got != (Location{X: 103, Y: 195, Z: 2}) {
		t.Fatalf("Transform = %+v, want [103, 195, 2]", got)
	}
	if l != (Location{X: 100, Y: 200, Z: 2}) {
		t.Fatalf("Transform mutated the receiver: %+v", l)
	}
	if got := (Location{X: 0, Y: 0, Z: 1}).Transform(0, 0); got.Z != 1 {
		t.Fatalf("zero Transform dropped z: %+v", got)
	}
}

func TestDirectionFromDeltas(t *testing.T) {
	// Direction.ts:75-96 (fromDeltas): sign-only branches, (0,0) NONE.
	cases := []struct {
		dx, dy int
		want   Direction
	}{
		{0, 0, DirNone},
		{0, 1, DirNorth}, {0, 5, DirNorth}, {0, -1, DirSouth}, {0, -9, DirSouth},
		{1, 0, DirEast}, {7, 0, DirEast}, {-1, 0, DirWest}, {-3, 0, DirWest},
		{1, 1, DirNorthEast}, {5, 3, DirNorthEast},
		{-1, 1, DirNorthWest}, {-4, 2, DirNorthWest},
		{1, -1, DirSouthEast}, {2, -8, DirSouthEast},
		{-1, -1, DirSouthWest}, {-6, -1, DirSouthWest},
	}
	for _, c := range cases {
		if got := DirectionFromDeltas(c.dx, c.dy); got != c.want {
			t.Errorf("DirectionFromDeltas(%d, %d) = %v, want %v", c.dx, c.dy, got, c.want)
		}
	}
}

func TestMobileStepDirectionsSetFacing(t *testing.T) {
	// Mobile.ts:806-822: the step direction is stored, and facing
	// follows it when the direction id is >= 0.
	m := NewMobile(Location{X: 10, Y: 10, Z: 0})
	m.SetWalkingDirection(DirEast)
	if m.WalkingDirection != DirEast || m.Facing != DirEast {
		t.Fatalf("walking east: dir=%v facing=%v", m.WalkingDirection, m.Facing)
	}
	m.SetWalkingDirection(DirNone)
	if m.WalkingDirection != DirNone || m.Facing != DirEast {
		t.Fatalf("walking NONE must keep facing: dir=%v facing=%v", m.WalkingDirection, m.Facing)
	}
	m.SetRunningDirection(DirSouth)
	if m.RunningDirection != DirSouth || m.Facing != DirSouth {
		t.Fatalf("running south: dir=%v facing=%v", m.RunningDirection, m.Facing)
	}
	m.SetRunningDirection(DirNone)
	if m.Facing != DirSouth {
		t.Fatalf("running NONE must keep facing: %v", m.Facing)
	}
}

func TestCheckDestination(t *testing.T) {
	// MovementQueue.ts:384-398.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 1})
	q := NewMovementQueue(owner, nil, nil)
	cases := []struct {
		dest Location
		want bool
	}{
		{Location{X: 100, Y: 100, Z: 1}, true},
		{Location{X: 0, Y: 0, Z: 1}, true},
		{Location{X: 32767, Y: 32767, Z: 1}, true},
		{Location{X: 32768, Y: 100, Z: 1}, false},
		{Location{X: 100, Y: 32768, Z: 1}, false},
		{Location{X: -1, Y: 100, Z: 1}, false},
		{Location{X: 100, Y: -1, Z: 1}, false},
		{Location{X: 100, Y: 100, Z: 0}, false},  // z mismatch
		{Location{X: 100, Y: 100, Z: 2}, false},  // z mismatch
		{Location{X: 100, Y: 100, Z: -1}, false}, // z < 0
	}
	for _, c := range cases {
		if got := q.CheckDestination(c.dest); got != c.want {
			t.Errorf("CheckDestination(%+v) = %v, want %v", c.dest, got, c.want)
		}
	}
}

func TestRequestWalkEnqueuesRouteOnOpenGround(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil) // nil collision/clipping: open ground.
	q.RequestWalk(Location{X: 105, Y: 103, Z: 0}, nil)
	if q.Size() == 0 {
		t.Fatal("requestWalk on open ground queued no checkpoints")
	}
	if !q.HasRoute() || !q.WasRouteEvaluated() {
		t.Fatal("route flags not set after a successful route")
	}
	if q.LastDestX != 105 || q.LastDestY != 103 {
		t.Fatalf("lastDest = (%d, %d), want (105, 103)", q.LastDestX, q.LastDestY)
	}
	dest, ok := q.GetDestination()
	if !ok || dest != (Location{X: 105, Y: 103, Z: 0}) {
		t.Fatalf("destination = (%+v, %v), want (105, 103, 0)", dest, ok)
	}
	// PathX/PathY round-trip the region-base math to the route end.
	if q.PathX != 105 || q.PathY != 103 {
		t.Fatalf("path = (%d, %d), want (105, 103)", q.PathX, q.PathY)
	}
	// Stepping drains the queue and lands on the destination.
	for i := int64(1); i <= 20 && q.Size() > 0; i++ {
		tickQueue(q, i)
	}
	if owner.loc != (Location{X: 105, Y: 103, Z: 0}) {
		t.Fatalf("owner ended at %+v, want (105, 103, 0)", owner.loc)
	}
	if q.Size() != 0 {
		t.Fatalf("queue not drained: %d points", q.Size())
	}
}

func TestRequestWalkUnreachableLeavesQueueEmpty(t *testing.T) {
	// Every tile answers blockWalk (routefinding.go floorBlocked),
	// so the BFS cannot leave the source and no approach point
	// exists 30 tiles away: the route fails.
	blocked := func(x, y, level int) int32 { return blockWalk }
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, blocked)
	q.RequestWalk(Location{X: 130, Y: 100, Z: 0}, boolPtr(true))
	if q.Size() != 0 {
		t.Fatalf("unreachable route queued %d checkpoints", q.Size())
	}
	if q.HasRoute() {
		t.Fatal("HasRoute must be false for a failed route")
	}
	if !q.WasRouteEvaluated() {
		t.Fatal("WasRouteEvaluated must be true after a failed search")
	}
	if _, ok := q.GetDestination(); ok {
		t.Fatal("GetDestination must report none for an empty queue")
	}
	// Run override is set only when points exist (MovementQueue.ts:431):
	// with an empty queue the setting (walking) wins.
	if q.IsRunToggled() {
		t.Fatal("runOverride must be nil when routing produced no points")
	}
}

func TestRequestWalkToOwnTileQueuesNothing(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 100, Y: 100, Z: 0}, boolPtr(true))
	if q.Size() != 0 {
		t.Fatalf("walk to own tile queued %d checkpoints", q.Size())
	}
	tickQueue(q, 1)
	if owner.loc != (Location{X: 100, Y: 100, Z: 0}) {
		t.Fatalf("owner moved to %+v", owner.loc)
	}
}

func TestRequestWalkRejectsBadDestinationAndNonPlayer(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 105, Y: 100, Z: 1}, nil) // z mismatch
	if q.Size() != 0 {
		t.Fatal("z-mismatched destination must be rejected")
	}
	npc := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	npc.player = false
	nq := NewMovementQueue(npc, nil, nil)
	nq.RequestWalk(Location{X: 105, Y: 100, Z: 0}, nil)
	if nq.Size() != 0 {
		t.Fatal("requestWalk must refuse a non-player owner (no player gate)")
	}
}

func TestRequestWalkRunOverrideOrdering(t *testing.T) {
	// MovementQueue.ts:431: runOverride is set AFTER routing, only
	// when points exist; Process clears it when the queue empties.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false // walking setting
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 102, Y: 100, Z: 0}, boolPtr(true))
	if !q.IsRunToggled() {
		t.Fatal("runOverride=true must win over the walking setting")
	}
	// One running tick covers both tiles; the queue empties and the
	// override clears, so the setting wins again.
	tickQueue(q, 1)
	if owner.loc != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("owner at %+v after one running tick", owner.loc)
	}
	if q.Size() != 0 {
		t.Fatal("queue must be empty after arrival")
	}
	if q.IsRunToggled() {
		t.Fatal("runOverride must clear when the queue empties")
	}
}

func TestResetClearsAndPromotesPending(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 105, Y: 100, Z: 0}, nil)
	if q.Size() == 0 {
		t.Fatal("setup: no checkpoints queued")
	}
	// A pending click override is promoted by Reset (MovementQueue.ts:870-873).
	q.SetClickRunOverride(boolPtr(true))
	q.Reset(true)
	if q.Size() != 0 || q.FollowX != -1 || q.FollowY != -1 {
		t.Fatal("Reset must clear points and follow state")
	}
	if q.HasRoute() || q.WasRouteEvaluated() {
		t.Fatal("Reset must clear route flags")
	}
	if !q.IsRunToggled() {
		t.Fatal("pending click override must be promoted by Reset")
	}
	// clickRunOverride OUTLIVES a reset with nothing pending
	// (MovementQueue.ts:868-869).
	q.Reset(true)
	if !q.IsRunToggled() {
		t.Fatal("clickRunOverride must outlive a reset")
	}
	// Reset(false) also promotes; it only skips the destination sync.
	q.EndClickRunOverride()
	q.SetClickRunOverride(boolPtr(true))
	q.Reset(false)
	if !q.IsRunToggled() {
		t.Fatal("Reset(false) must promote a pending click override")
	}
}

func TestClickRunOverrideLifecycle(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)

	// Pending alone changes nothing until a reset promotes it.
	q.SetClickRunOverride(boolPtr(true))
	if q.IsRunToggled() {
		t.Fatal("pending override must not apply before a reset")
	}
	// Dropping the pending value leaves the setting in force.
	q.DropPendingClickRunOverride()
	q.Reset(true)
	if q.IsRunToggled() {
		t.Fatal("dropped pending override must not be promoted")
	}
	// endClickRunOverrideWhenIdle: kept while a route is queued,
	// cleared once idle (MovementQueue.ts:1136-1141).
	q.SetClickRunOverride(boolPtr(true))
	q.Reset(true)
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	// requestWalk clears clickRunOverride (walk click replaces the
	// interaction choice) — re-stage it as an interaction would.
	q.SetClickRunOverride(boolPtr(true))
	q.Reset(false)
	if q.Size() != 0 {
		t.Fatal("setup: Reset(false) must clear the route")
	}
	q.AddCheckpoint(Location{X: 103, Y: 100, Z: 0})
	q.EndClickRunOverrideWhenIdle()
	if !q.IsRunToggled() {
		t.Fatal("click override must survive while a route is queued")
	}
	q.Reset(true) // clears points, keeps the click override
	q.EndClickRunOverrideWhenIdle()
	if q.IsRunToggled() {
		t.Fatal("click override must end once idle with no route")
	}
	// EndClickRunOverride clears both the active and pending values.
	q.SetClickRunOverride(boolPtr(true))
	q.Reset(true)
	q.SetClickRunOverride(boolPtr(true))
	q.EndClickRunOverride()
	q.Reset(true)
	if q.IsRunToggled() {
		t.Fatal("EndClickRunOverride must clear active and pending values")
	}
}

func TestIsRunToggledPrecedence(t *testing.T) {
	// MovementQueue.ts:1143-1147: runOverride ?? clickRunOverride ??
	// run setting.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)

	owner.running = false
	if q.IsRunToggled() {
		t.Fatal("no overrides: setting (walking) must win")
	}
	owner.running = true
	if !q.IsRunToggled() {
		t.Fatal("no overrides: setting (running) must win")
	}
	// Click override beats the setting.
	owner.running = true
	q.SetClickRunOverride(boolPtr(false))
	q.Reset(true)
	if q.IsRunToggled() {
		t.Fatal("click override=false must beat running=true")
	}
	// Path override beats the click override. (RequestWalk clears
	// the click override first, so stage the click override after
	// routing via a reset that keeps the route: use Reset(false)
	// promotion, then re-queue — instead exercise the precedence
	// directly: route with runOverride=true while a click override
	// of false is pending promotion is impossible through the
	// public path, so verify via a fresh queue with click=false
	// promoted, then a walk with runOverride=true.)
	q2 := NewMovementQueue(owner, nil, nil)
	q2.SetClickRunOverride(boolPtr(false))
	q2.Reset(true)
	// Simulate an interaction walk: the click override is active;
	// a new walk click's runOverride replaces it for this path.
	// RequestWalk clears clickRunOverride, so the path override is
	// what remains — assert the path override wins over the setting.
	owner.running = false
	q2.RequestWalk(Location{X: 103, Y: 100, Z: 0}, boolPtr(true))
	if !q2.IsRunToggled() {
		t.Fatal("path runOverride=true must win")
	}
	// Non-player owners read as walking (NPC branch NOT STARTED).
	npc := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	npc.player = false
	nq := NewMovementQueue(npc, nil, nil)
	if nq.IsRunToggled() {
		t.Fatal("non-player owner must read as walking")
	}
}

func TestClickRunOverride(t *testing.T) {
	// NetworkBuilder.ts:238-244: Ctrl (bit 1) or flags==2 inverts;
	// otherwise nil. Energy/run-invert degenerate to "allowed" (see
	// ClickRunOverride's doc comment).
	cases := []struct {
		running bool
		flags   int
		want    *bool
	}{
		{false, 0, nil},
		{true, 0, nil},
		{false, 1, boolPtr(true)},
		{true, 1, boolPtr(false)},
		{false, 2, boolPtr(true)}, // Ctrl+Shift walks like Ctrl.
		{true, 2, boolPtr(false)},
		{false, 3, boolPtr(true)},
		{true, 3, boolPtr(false)},
		{false, 4, nil}, // no Ctrl bit, not 2.
		{true, 4, nil},
	}
	for _, c := range cases {
		got := ClickRunOverride(c.running, c.flags)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("ClickRunOverride(%v, %d) = %v, want %v", c.running, c.flags, derefBool(got), derefBool(c.want))
		}
	}
}

func derefBool(b *bool) string {
	if b == nil {
		return "<nil>"
	}
	if *b {
		return "true"
	}
	return "false"
}

func TestProcessWalksOneStepPerTick(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	tickQueue(q, 1)
	if owner.loc != (Location{X: 101, Y: 100, Z: 0}) {
		t.Fatalf("walking tick moved to %+v, want one step east", owner.loc)
	}
	if owner.walkingDir != DirEast || owner.runningDir != DirNone {
		t.Fatalf("directions: walking=%v running=%v, want EAST/NONE", owner.walkingDir, owner.runningDir)
	}
	if owner.facing != DirEast {
		t.Fatalf("facing = %v, want EAST", owner.facing)
	}
	if !q.IsMoving() || !q.DidMoveThisCycle() {
		t.Fatal("moving flags must be set after a walking step")
	}
	if q.DidRunThisCycle() {
		t.Fatal("a walking tick must not count as running")
	}
	if !q.SteppedThisWorldCycle(1) || q.SteppedThisWorldCycle(2) {
		t.Fatal("SteppedThisWorldCycle must match the processed cycle")
	}
	if !q.DidMovePreviousCycle(2) {
		t.Fatal("DidMovePreviousCycle(2) must be true after stepping on cycle 1")
	}
	// FollowX/FollowY record the tile stepped FROM.
	if q.FollowX != 100 || q.FollowY != 100 {
		t.Fatalf("follow = (%d, %d), want (100, 100)", q.FollowX, q.FollowY)
	}
	tickQueue(q, 2)
	if owner.loc.X != 102 {
		t.Fatalf("second walking tick: %+v", owner.loc)
	}
}

func TestProcessRunsTwoStepsPerTick(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	// running defaults true.
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	tickQueue(q, 1)
	if owner.loc != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("running tick moved to %+v, want two steps east", owner.loc)
	}
	if owner.walkingDir != DirEast || owner.runningDir != DirEast {
		t.Fatalf("directions: walking=%v running=%v, want EAST/EAST", owner.walkingDir, owner.runningDir)
	}
	if !q.DidRunThisCycle() {
		t.Fatal("a two-step running tick must set ranThisCycle")
	}
	// A lone final step is a walk, not a run (MovementQueue.ts:646
	// comment): one tile left, one step taken, no run marker.
	tickQueue(q, 2)
	if owner.loc != (Location{X: 103, Y: 100, Z: 0}) {
		t.Fatalf("final tick moved to %+v", owner.loc)
	}
	// BeginCycle cleared ranThisCycle; the one-step tick must not
	// set it again.
	if q.DidRunThisCycle() {
		t.Fatal("a one-step tick must not count as running")
	}
}

func TestProcessDiagonalStep(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 102, Y: 102, Z: 0}, nil)
	tickQueue(q, 1)
	if owner.loc != (Location{X: 101, Y: 101, Z: 0}) {
		t.Fatalf("diagonal tick moved to %+v, want (101, 101, 0)", owner.loc)
	}
	if owner.walkingDir != DirNorthEast {
		t.Fatalf("walking direction = %v, want NORTH_EAST", owner.walkingDir)
	}
}

func TestProcessBlockedStepRetainsCheckpoint(t *testing.T) {
	// Collision blocks the tile east of the owner; the route (open
	// ground) still points east.
	collision := &fakeCollision{blocked: map[Location]bool{{X: 101, Y: 100, Z: 0}: true}}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, collision, nil)
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	if q.Size() == 0 {
		t.Fatal("setup: no checkpoints queued")
	}
	before := q.Size()
	tickQueue(q, 1)
	if owner.loc != (Location{X: 100, Y: 100, Z: 0}) {
		t.Fatalf("blocked step moved to %+v", owner.loc)
	}
	if q.Size() != before {
		t.Fatalf("blocked checkpoint must be retained: %d -> %d", before, q.Size())
	}
	if !q.IsStepBlocked() {
		t.Fatal("IsStepBlocked must be set after a rejected step")
	}
	if !q.WasRouteInvalidated() {
		t.Fatal("a clipping block must invalidate the route (not entity-caused)")
	}
	if q.WasBlockedByDynamicOccupancy() {
		t.Fatal("static clipping must not report dynamic occupancy")
	}
	if q.IsMoving() || q.DidMoveThisCycle() {
		t.Fatal("a blocked tick must not report movement")
	}
	// Clearing the block lets the same checkpoint drain.
	delete(collision.blocked, Location{X: 101, Y: 100, Z: 0})
	tickQueue(q, 2)
	if owner.loc.X != 101 {
		t.Fatalf("after unblock, owner at %+v", owner.loc)
	}
	if q.IsStepBlocked() {
		t.Fatal("a successful step must clear stepBlocked")
	}
}

func TestValidatedStepSquaresTheCorner(t *testing.T) {
	// Diagonal tile blocked, axis tiles free: the step squares the
	// corner east instead of stepping diagonally
	// (MovementQueue.ts:738-769: diagonal first, then axes).
	collision := &fakeCollision{blocked: map[Location]bool{{X: 101, Y: 101, Z: 0}: true}}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, collision, nil)
	q.AddCheckpoint(Location{X: 102, Y: 101, Z: 0})
	tickQueue(q, 1)
	if owner.loc != (Location{X: 101, Y: 100, Z: 0}) {
		t.Fatalf("corner step moved to %+v, want (101, 100, 0)", owner.loc)
	}
	if owner.walkingDir != DirEast {
		t.Fatalf("corner step direction = %v, want EAST", owner.walkingDir)
	}
}

func TestProcessSkipsCheckpointOnCurrentTile(t *testing.T) {
	// A head checkpoint equal to the current tile is skipped without
	// consuming a step (MovementQueue.ts:601-603).
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 100, Y: 100, Z: 0})
	q.AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	tickQueue(q, 1)
	if owner.loc != (Location{X: 101, Y: 100, Z: 0}) {
		t.Fatalf("owner at %+v, want (101, 100, 0)", owner.loc)
	}
	if q.Size() != 0 {
		t.Fatalf("both checkpoints must be consumed, %d left", q.Size())
	}
}

func TestCanWalkTo(t *testing.T) {
	collision := &fakeCollision{blocked: map[Location]bool{{X: 101, Y: 100, Z: 0}: true}}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, collision, nil)
	src := Location{X: 100, Y: 100, Z: 0}
	if q.CanWalkTo(Location{X: 101, Y: 100, Z: 0}, src) {
		t.Fatal("blocked tile must not be walkable")
	}
	if !q.CanWalkTo(Location{X: 100, Y: 101, Z: 0}, src) {
		t.Fatal("open tile must be walkable")
	}
	if q.CanWalkTo(Location{X: 100, Y: 101, Z: 1}, src) {
		t.Fatal("cross-level step must not be walkable")
	}
	// Nil collision is open ground.
	open := NewMovementQueue(owner, nil, nil)
	if !open.CanWalkTo(Location{X: 101, Y: 100, Z: 0}, src) {
		t.Fatal("nil collision must allow every step")
	}
}

func TestIgnoreClippingStepsStraightOn(t *testing.T) {
	collision := &fakeCollision{blockAll: true}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, collision, nil)
	q.SetIgnoreClipping(true)
	q.AddCheckpoint(Location{X: 102, Y: 102, Z: 0})
	tickQueue(q, 1)
	if owner.loc != (Location{X: 101, Y: 101, Z: 0}) {
		t.Fatalf("ignore-clipping step moved to %+v, want diagonal", owner.loc)
	}
}

func TestMobilityGate(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	if q.BlockMovementGeneration() != 0 || q.IsMovementBlocked() {
		t.Fatal("movement must start unblocked at generation 0")
	}
	q.SetBlockMovement(true)
	if !q.IsMovementBlocked() || q.BlockMovementGeneration() != 1 {
		t.Fatal("SetBlockMovement must hold and bump the generation")
	}
	// requestWalk and addCheckpoint refuse while blocked.
	q.RequestWalk(Location{X: 105, Y: 100, Z: 0}, nil)
	if q.Size() != 0 {
		t.Fatal("requestWalk must refuse while movement is blocked")
	}
	q.AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	if q.Size() != 0 {
		t.Fatal("addCheckpoint must refuse while movement is blocked")
	}
	// A queued route does not step while blocked.
	q.SetBlockMovement(false)
	q.RequestWalk(Location{X: 102, Y: 100, Z: 0}, nil)
	q.SetBlockMovement(true)
	tickQueue(q, 1)
	if owner.loc.X != 100 || q.IsMoving() {
		t.Fatalf("blocked process moved to %+v (moving=%v)", owner.loc, q.IsMoving())
	}
	if q.Size() == 0 {
		t.Fatal("blocked process must keep the checkpoints")
	}
	// Releasing the hold resumes stepping.
	q.SetBlockMovement(false)
	if q.BlockMovementGeneration() != 4 {
		t.Fatalf("generation = %d, want 4", q.BlockMovementGeneration())
	}
	tickQueue(q, 2)
	if owner.loc.X == 100 {
		t.Fatal("releasing the hold must resume stepping")
	}
}

func TestBeginCycleClearsPerCycleStateButNotStepBlocked(t *testing.T) {
	// MovementQueue.ts:473-479 clears moved/ran/blocked-by-occupancy
	// but NOT stepBlocked/stepBlockedByEntity.
	collision := &fakeCollision{blockAll: true}
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, collision, nil)
	q.AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	tickQueue(q, 1)
	if !q.IsStepBlocked() {
		t.Fatal("setup: step must be blocked")
	}
	q.BeginCycle()
	if !q.IsStepBlocked() {
		t.Fatal("BeginCycle must NOT clear stepBlocked")
	}
	if q.DidMoveThisCycle() || q.DidRunThisCycle() || q.WasBlockedByDynamicOccupancy() {
		t.Fatal("BeginCycle must clear moved/ran/occupancy flags")
	}
}

func TestHasPendingWork(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	if q.HasPendingWork() {
		t.Fatal("empty idle queue must have no pending work")
	}
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	if !q.HasPendingWork() {
		t.Fatal("queued route must be pending work")
	}
	for i := int64(1); i <= 10 && q.Size() > 0; i++ {
		tickQueue(q, i)
	}
	// After the arrival tick isMoving is true until the next
	// Process runs with an empty queue; one more tick settles it.
	tickQueue(q, 11)
	if q.HasPendingWork() {
		t.Fatal("drained idle queue must have no pending work")
	}
}

func TestSetExternalMovement(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.SetExternalMovement(true, false)
	if !q.HasPendingWork() {
		t.Fatal("external movement must count as pending work")
	}
	q.BeginCycle()
	q.Process(7)
	if !q.IsMoving() || !q.DidMoveThisCycle() || !q.SteppedThisWorldCycle(7) {
		t.Fatal("external movement must report moving on the cycle")
	}
	if owner.loc.X != 100 {
		t.Fatalf("external movement must not step the owner: %+v", owner.loc)
	}
	q.SetExternalMovement(false, false)
	// isMoving stays true until the next Process runs with an empty
	// queue (hasPendingWork reads isMoving, as in TS); one more
	// tick settles it.
	tickQueue(q, 8)
	if q.HasPendingWork() {
		t.Fatal("ending external movement must clear pending work")
	}
	// tileChanged refreshes the region baseline.
	owner.loc = Location{X: 200, Y: 200, Z: 0}
	q.SetExternalMovement(true, true)
	if got := owner.GetLastKnownRegion(); got != owner.loc {
		t.Fatalf("tileChanged must refresh LastKnownRegion: %+v", got)
	}
	// The 250 ms window expires on its own.
	q2 := NewMovementQueue(owner, nil, nil)
	q2.SetExternalMovement(true, false)
	time.Sleep(300 * time.Millisecond)
	if q2.HasPendingWork() {
		t.Fatal("external movement window must expire after 250ms")
	}
}

func TestHandleRegionChangeThresholds(t *testing.T) {
	// MovementQueue.ts:771-801: diff from the baseline region origin
	// (RegionX()*8); changed when either diff is < 16 or >= 88, or
	// the plane differs.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	// Baseline region origin: RegionX(100) = 6 -> 48; diffs 52/52.
	owner.loc = Location{X: 110, Y: 105, Z: 0} // diffs 62/57: inside.
	q.HandleRegionChange()
	if got := owner.GetLastKnownRegion(); got != (Location{X: 100, Y: 100, Z: 0}) {
		t.Fatalf("inside-band move must not refresh region: %+v", got)
	}
	// diffX = 136-48 = 88: at the threshold, refresh.
	owner.loc = Location{X: 136, Y: 105, Z: 0}
	q.HandleRegionChange()
	if got := owner.GetLastKnownRegion(); got != owner.loc {
		t.Fatalf("diffX>=88 must refresh region: %+v", got)
	}
	// New baseline origin: RegionX(136) = 11 -> 88; move to diffX 15.
	owner.loc = Location{X: 103, Y: 105, Z: 0} // diffX = 15 < 16.
	q.HandleRegionChange()
	if got := owner.GetLastKnownRegion(); got != owner.loc {
		t.Fatalf("diffX<16 must refresh region: %+v", got)
	}
	// Plane change alone refreshes, even at the same x/y.
	owner.loc = Location{X: 103, Y: 105, Z: 1}
	q.HandleRegionChange()
	if got := owner.GetLastKnownRegion(); got != owner.loc {
		t.Fatalf("plane change must refresh region: %+v", got)
	}
}

func TestAddCheckpointCap(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	for i := 0; i < 150; i++ {
		q.AddCheckpoint(Location{X: 100 + i, Y: 100, Z: 0})
	}
	if q.Size() != maxQueuePoints {
		t.Fatalf("queue size = %d, want cap %d", q.Size(), maxQueuePoints)
	}
}

func TestQueuePointsAndDestination(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	q.AddCheckpoint(Location{X: 102, Y: 100, Z: 0})
	pts := q.Points()
	if len(pts) != 2 || pts[1] != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("Points = %v", pts)
	}
	pts[0] = Location{}
	if q.Points()[0] != (Location{X: 101, Y: 100, Z: 0}) {
		t.Fatal("Points must return a copy")
	}
}

func TestSetPathXAndY(t *testing.T) {
	// MovementQueue.ts:1173-1181: pathX = RegionX()*8 + x.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0}) // RegionX=6, RegionY=6.
	q := NewMovementQueue(owner, nil, nil)
	q.SetPathX(10)
	q.SetPathY(20)
	if q.PathX != 58 || q.PathY != 68 {
		t.Fatalf("path = (%d, %d), want (58, 68)", q.PathX, q.PathY)
	}
}

func TestSetRouteFlags(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.SetRoute(true, true)
	if !q.HasRoute() || !q.WasRouteEvaluated() || !q.UsedAlternativeRoute() {
		t.Fatal("SetRoute(true, true) flags wrong")
	}
	q.SetRoute(false, false)
	if q.HasRoute() || q.UsedAlternativeRoute() {
		t.Fatal("SetRoute(false, false) flags wrong")
	}
}

func TestPlayerMovementDefaults(t *testing.T) {
	// Player.ts:160 (isRunning=true), :93 (getSize=1).
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	if !p.IsRunning() {
		t.Fatal("new players must default to running")
	}
	if p.Size() != 1 || !p.IsPlayer() {
		t.Fatal("player Size/IsPlayer wrong")
	}
	if p.MovementQueue() == nil {
		t.Fatal("NewPlayer must install a movement queue")
	}
	if p.MovementQueue().HasPendingWork() {
		t.Fatal("fresh queue must have no pending work")
	}
	p.SetRunning(false)
	if p.IsRunning() {
		t.Fatal("SetRunning(false) must stick")
	}
	// The queue's owner is the player: its Location tracks Loc.
	if p.MovementQueue().owner.Location() != p.Loc {
		t.Fatal("queue owner must be the player")
	}
}

func TestWorldDefaultRegions(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	rm := w.RegionManager()
	if rm == nil {
		t.Fatal("NewWorld must install a default RegionManager")
	}
	// Empty manager: open ground.
	if !rm.CanMoveStart(Location{X: 100, Y: 100, Z: 0}, Location{X: 101, Y: 100, Z: 0}, 1) {
		t.Fatal("default RegionManager must allow movement (open ground)")
	}
	if rm.Lookup() == nil {
		t.Fatal("default RegionManager lookup must be non-nil")
	}
	if got := rm.Lookup()(100, 100, 0); got != 0 {
		t.Fatalf("default lookup = %d, want 0 (open)", got)
	}
	replacement := NewRegionManager(nil, nil, false)
	w.SetRegionManager(replacement)
	if w.RegionManager() != replacement {
		t.Fatal("SetRegionManager must replace the manager")
	}
	w.SetRegionManager(nil)
	if w.RegionManager() != replacement {
		t.Fatal("SetRegionManager(nil) must keep the current manager")
	}
}

func TestSetCollisionAndClippingRewire(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := NewMovementQueue(owner, nil, nil)
	// Rewire collision to block everything: stepping stops.
	q.SetCollision(&fakeCollision{blockAll: true})
	q.AddCheckpoint(Location{X: 101, Y: 100, Z: 0})
	tickQueue(q, 1)
	if owner.loc.X != 100 {
		t.Fatalf("rewired collision must block: %+v", owner.loc)
	}
	// Rewire clipping to block everything: routing fails.
	q.Reset(true)
	q.SetCollision(nil)
	q.SetClipping(func(x, y, level int) int32 { return blockWalk })
	q.RequestWalk(Location{X: 130, Y: 100, Z: 0}, nil)
	if q.Size() != 0 {
		t.Fatal("rewired clipping must fail routing")
	}
}
