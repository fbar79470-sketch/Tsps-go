package game

import "testing"

// Walk-to-object tests: buildObjectRouteSpec
// (MovementQueue.ts:1348-1384), ReachedObject
// (PathFinder.ts:229-248), and the WalkToObject task
// (MovementQueue.ts:1742-1912). Fixtures only; open ground
// unless a fake clipping/collision source says otherwise.

func TestBuildObjectRouteSpec(t *testing.T) {
	base := ObjectTarget{
		ID: 100, Location: Location{X: 50, Y: 60, Z: 0}, Type: 10, Face: 1,
		SizeX: 1, SizeY: 1, BaseSizeX: 4, BaseSizeY: 2, BlockingMask: 0x100,
	}
	// Type 10: the BASE footprint (4x2), not the resolved
	// child's (1x1); mask OR'd with blockAccessFlags; reach
	// shape 10 with the footprint's width/length.
	spec := buildObjectRouteSpec(base, 0x8)
	if spec.routeSize != 0 || spec.routeXLength != 4 || spec.routeYLength != 2 {
		t.Fatalf("type 10 route shape = size %d %dx%d, want 0 4x2", spec.routeSize, spec.routeXLength, spec.routeYLength)
	}
	if spec.routeDirection != 1 || spec.routeBlockingMask != 0x108 {
		t.Fatalf("type 10 route dir/mask = %d/%#x, want 1/0x108", spec.routeDirection, spec.routeBlockingMask)
	}
	if spec.reachWidth != 4 || spec.reachLength != 2 || spec.reachShape != 10 || spec.reachAngle != 1 || spec.reachBlockAccessFlags != 0x108 {
		t.Fatalf("type 10 reach = %+v, want 4x2 shape 10 angle 1 mask 0x108", spec)
	}
	if spec.objectX != 50 || spec.objectY != 60 {
		t.Fatalf("type 10 object tile = %d,%d, want 50,60", spec.objectX, spec.objectY)
	}

	// Type 11: same footprint rule as type 10.
	eleven := base
	eleven.Type = 11
	spec = buildObjectRouteSpec(eleven, 0)
	if spec.routeXLength != 4 || spec.routeYLength != 2 || spec.reachShape != 10 {
		t.Fatalf("type 11 spec = %+v, want 4x2 footprint, reach shape 10", spec)
	}

	// Type 22: the RESOLVED child's footprint.
	ground := base
	ground.Type = 22
	spec = buildObjectRouteSpec(ground, 0)
	if spec.routeXLength != 1 || spec.routeYLength != 1 || spec.reachWidth != 1 || spec.reachLength != 1 {
		t.Fatalf("type 22 spec = %+v, want 1x1 resolved footprint", spec)
	}
	if spec.reachShape != 10 || spec.routeBlockingMask != 0x100 {
		t.Fatalf("type 22 reach/mask = %+v, want shape 10 mask 0x100", spec)
	}

	// Other types: routeSize = type+1, zero route lengths,
	// mask = blockAccessFlags alone, 1x1 reach, reach shape
	// = type, reach angle = face.
	wall := base
	wall.Type = 0
	wall.Face = 2
	spec = buildObjectRouteSpec(wall, 0x8)
	if spec.routeSize != 1 || spec.routeXLength != 0 || spec.routeYLength != 0 {
		t.Fatalf("type 0 route shape = %+v, want size 1, zero lengths", spec)
	}
	if spec.routeBlockingMask != 0x8 || spec.reachBlockAccessFlags != 0x8 {
		t.Fatalf("type 0 mask = %#x/%#x, want 0x8/0x8", spec.routeBlockingMask, spec.reachBlockAccessFlags)
	}
	if spec.reachWidth != 1 || spec.reachLength != 1 || spec.reachShape != 0 || spec.reachAngle != 2 {
		t.Fatalf("type 0 reach = %+v, want 1x1 shape 0 angle 2", spec)
	}
	door := base
	door.Type = 4
	spec = buildObjectRouteSpec(door, 0)
	if spec.routeSize != 5 || spec.reachShape != 4 {
		t.Fatalf("type 4 spec = %+v, want routeSize 5, reach shape 4", spec)
	}
}

func TestReachedObjectVectors(t *testing.T) {
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	// 1x1 type-10 object at (100,100): orthogonally adjacent
	// is reached; two tiles away is not.
	if !q.ReachedObject(100, 100, 1, 1, 0, 10, 0) {
		t.Fatal("adjacent tile should reach a 1x1 object")
	}
	owner.loc = Location{X: 102, Y: 100, Z: 0}
	if q.ReachedObject(100, 100, 1, 1, 0, 10, 0) {
		t.Fatal("two tiles away should not reach a 1x1 object")
	}
	// Diagonally adjacent does NOT reach: the rectangle rule
	// (reachRectangle1) counts only tiles directly beside a
	// side of the footprint.
	owner.loc = Location{X: 101, Y: 101, Z: 0}
	if q.ReachedObject(100, 100, 1, 1, 0, 10, 0) {
		t.Fatal("diagonally adjacent tile should not reach a 1x1 object")
	}
	// Multi-tile 3x2 object at (100,100): adjacent to the east
	// edge is reached; one tile further is not.
	owner.loc = Location{X: 103, Y: 101, Z: 0}
	if !q.ReachedObject(100, 100, 3, 2, 0, 10, 0) {
		t.Fatal("tile adjacent to a 3x2 footprint's edge should reach it")
	}
	owner.loc = Location{X: 104, Y: 101, Z: 0}
	if q.ReachedObject(100, 100, 3, 2, 0, 10, 0) {
		t.Fatal("two tiles from a 3x2 footprint's edge should not reach it")
	}
	// Adjacent to the north edge of the same footprint.
	owner.loc = Location{X: 101, Y: 102, Z: 0}
	if !q.ReachedObject(100, 100, 3, 2, 0, 10, 0) {
		t.Fatal("tile adjacent to a 3x2 footprint's north edge should reach it")
	}
}

func objectTargetAt(id, x, y, typ, face int) ObjectTarget {
	return ObjectTarget{
		ID: id, Location: Location{X: x, Y: y, Z: 0}, Type: typ, Face: face,
		SizeX: 1, SizeY: 1, BaseSizeX: 1, BaseSizeY: 1,
		Name: "Test object", Interactions: []string{"Open"},
	}
}

func TestWalkToObjectAlreadyAdjacent(t *testing.T) {
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	ran := false
	q.WalkToObject(objectTargetAt(100, 100, 100, 10, 0), func() { ran = true }, 1, 1, WalkToObjectHooks{
		StillStands: func() bool { return true },
	})
	if !q.WalkToPending() {
		t.Fatal("WalkToObject should install the walk-to state")
	}
	tickQueue(q, 2)
	if !ran {
		t.Fatal("already-adjacent walk-to-object should run the action on the first tick")
	}
	if q.WalkToPending() {
		t.Fatal("walk-to state should clear after arrival")
	}
	if len(owner.messages) != 0 {
		t.Fatalf("unexpected messages: %v", owner.messages)
	}
	// Arrival records the arrival tile in pathX/pathY.
	if q.PathX != 101 || q.PathY != 100 {
		t.Fatalf("pathX/pathY = %d,%d, want 101,100", q.PathX, q.PathY)
	}
}

func TestWalkToObjectWalksAndArrives(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	ran := false
	ranAt := -1
	q.WalkToObject(objectTargetAt(100, 105, 100, 10, 0), func() { ran = true }, 1, 1, WalkToObjectHooks{
		StillStands: func() bool { return true },
	})
	for cycle := int64(2); cycle < 12 && !ran; cycle++ {
		tickQueue(q, cycle)
		if ran {
			ranAt = int(cycle)
		}
	}
	if !ran {
		t.Fatalf("walk-to-object never arrived; owner at %+v, messages %v", owner.loc, owner.messages)
	}
	if ranAt < 3 {
		t.Fatalf("action ran at cycle %d: the didMove gate must defer arrival past the stepping tick", ranAt)
	}
	if !q.ReachedObject(105, 100, 1, 1, 0, 10, 0) {
		t.Fatalf("owner ended at %+v, which does not reach the object", owner.loc)
	}
	if q.WalkToPending() {
		t.Fatal("walk-to state should clear after arrival")
	}
}

func TestWalkToObjectDestinationOverride(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	var seen *ObjectRouteEvent
	dest := Location{X: 102, Y: 100, Z: 0}
	ran := false
	q.WalkToObject(objectTargetAt(100, 105, 100, 10, 0), func() { ran = true }, 3, 1, WalkToObjectHooks{
		Route: func(ev *ObjectRouteEvent) {
			seen = ev
			ev.Destination = &dest
		},
	})
	if seen == nil {
		t.Fatal("route hook should fire")
	}
	if seen.Target.ID != 100 || seen.ClickType != 3 || seen.Source != (Location{X: 100, Y: 100, Z: 0}) {
		t.Fatalf("route event = %+v, want target 100, clickType 3, source 100,100", seen)
	}
	if seen.Target.Name != "Test object" || len(seen.Target.Interactions) != 1 {
		t.Fatalf("route event target should carry the definition name/menu: %+v", seen.Target)
	}
	// The override redirects to a tile walk: the action runs
	// when the tile is reached (the tile walk's own
	// reachedDestination delay applies).
	for cycle := int64(2); cycle < 12 && !ran; cycle++ {
		tickQueue(q, cycle)
	}
	if !ran {
		t.Fatalf("destination-override walk never arrived; owner at %+v", owner.loc)
	}
	if owner.loc != dest {
		t.Fatalf("owner ended at %+v, want the override destination %+v", owner.loc, dest)
	}
}

func TestWalkToObjectRouteHookFiresBeforeChecks(t *testing.T) {
	// The route event fires before the mobility/destination
	// checks (MovementQueue.ts:1743-1760): even a walk that
	// then refuses still fires it.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	fired := false
	target := objectTargetAt(100, 105, 100, 10, 0)
	target.Location.Z = 1 // Different plane: CheckDestination fails.
	q.WalkToObject(target, func() {}, 1, 1, WalkToObjectHooks{
		Route: func(ev *ObjectRouteEvent) { fired = true },
	})
	if !fired {
		t.Fatal("route hook should fire before the destination check")
	}
	if q.WalkToPending() {
		t.Fatal("a plane-mismatched target should refuse silently, installing no walk-to")
	}
	if len(owner.messages) != 0 {
		t.Fatalf("destination refusal sends nothing, got %v", owner.messages)
	}
}

func TestWalkToObjectBlockedMovement(t *testing.T) {
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.SetBlockMovement(true)
	q.WalkToObject(objectTargetAt(100, 100, 100, 10, 0), func() {}, 1, 1, WalkToObjectHooks{})
	if q.WalkToPending() {
		t.Fatal("blockMovement should refuse the walk silently")
	}
	if len(owner.messages) != 0 {
		t.Fatalf("mobility refusal sends nothing, got %v", owner.messages)
	}
}

func TestWalkToObjectUnreachableSendsMessage(t *testing.T) {
	// A target outside the route finder's 128x128 window
	// routes nowhere: the task fails with the TS message.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	ran := false
	q.WalkToObject(objectTargetAt(100, 300, 300, 10, 0), func() { ran = true }, 1, 1, WalkToObjectHooks{
		StillStands: func() bool { return true },
	})
	for cycle := int64(2); cycle < 6 && q.WalkToPending(); cycle++ {
		tickQueue(q, cycle)
	}
	if ran {
		t.Fatal("unreachable walk-to-object must not run the action")
	}
	if q.WalkToPending() {
		t.Fatal("unreachable walk-to-object should stop")
	}
	if len(owner.messages) != 1 || owner.messages[0] != "I can't reach that!" {
		t.Fatalf("messages = %v, want exactly [\"I can't reach that!\"]", owner.messages)
	}
}

func TestWalkToObjectValidityStop(t *testing.T) {
	// stoodAtClick is true (adjacent, object standing at click
	// time); the object is then removed, so the next tick's
	// validity check stops the walk silently.
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	standing := true
	ran := false
	q.WalkToObject(objectTargetAt(100, 100, 100, 10, 0), func() { ran = true }, 1, 1, WalkToObjectHooks{
		StillStands: func() bool { return standing },
	})
	standing = false
	tickQueue(q, 2)
	if ran {
		t.Fatal("removed object must not run the action")
	}
	if q.WalkToPending() {
		t.Fatal("validity failure should stop the walk")
	}
	if len(owner.messages) != 0 {
		t.Fatalf("validity failure is silent, got %v", owner.messages)
	}
}

func TestWalkToObjectNonPlayerSkipsStillStands(t *testing.T) {
	// stoodAtClick requires isPlayer: for a non-player owner
	// the still-stands re-check never runs, so a false hook
	// does not stop the walk.
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	owner.player = false
	q := NewMovementQueue(owner, nil, nil)
	ran := false
	q.WalkToObject(objectTargetAt(100, 100, 100, 10, 0), func() { ran = true }, 1, 1, WalkToObjectHooks{
		StillStands: func() bool { return false },
	})
	tickQueue(q, 2)
	if !ran {
		t.Fatal("non-player walk-to-object should arrive despite a false still-stands hook")
	}
}

func TestWalkToObjectNilActionAndNilHooks(t *testing.T) {
	owner := newFakeOwner(Location{X: 101, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	// A nil action becomes a no-op; nil hooks mean no route
	// redirection and an unknowable still-stands check. The
	// walk still arrives and clears without panicking.
	q.WalkToObject(objectTargetAt(100, 100, 100, 10, 0), nil, 1, 1, WalkToObjectHooks{})
	tickQueue(q, 2)
	if q.WalkToPending() {
		t.Fatal("nil-action walk-to-object should clear after arrival")
	}
}
