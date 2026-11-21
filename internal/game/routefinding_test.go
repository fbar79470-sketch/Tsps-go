package game

import (
	"reflect"
	"testing"
)

// Hand-built fixtures only: fake CollisionLookup closures over small
// flag grids. No network, no cache binaries.
//
// Expected waypoints below were derived BY HAND from the TS algorithm
// (expansion order W/E/S/N/diagonals, reverse-direction storage,
// direction-change compression), not by running TS output. The three
// hand-traced cases are called out in their test comments:
//   - TestRouteFindLShape: BFS tie-break (east run, then one diagonal).
//   - TestRouteFindBlockedDestinationMoveNear: approach-point scan
//     cost + distance tie-break.
//   - TestRouteFindDiagonalCornerCut: diagonal side-tile rules.

// flagGrid is a sparse clipping grid: absent tiles answer 0 (open).
type flagGrid map[[2]int]int32

func (g flagGrid) lookup(x, y, level int) int32 { return g[[2]int{x, y}] }

func openLookup() CollisionLookup { return flagGrid{}.lookup }

// blockedColumn returns a lookup with blockWalk on every tile of the
// vertical segment x, y0..y1 inclusive.
func blockedColumn(x, y0, y1 int) CollisionLookup {
	g := flagGrid{}
	for y := y0; y <= y1; y++ {
		g[[2]int{x, y}] = blockWalk
	}
	return g.lookup
}

func routeOpts(srcX, srcY, destX, destY int) RouteOptions {
	return RouteOptions{
		Level: 0, SrcX: srcX, SrcY: srcY, SrcSize: 1,
		DestX: destX, DestY: destY, DestWidth: 1, DestLength: 1,
		LocAngle: 0, LocShape: -1, BlockAccessFlags: 0,
		MaxWaypoints: 25, MoveNear: true,
	}
}

func checkRoute(t *testing.T, got RouteResult, wantWaypoints []RouteWaypoint, wantAlt, wantSuccess bool, wantEndX, wantEndY int) {
	t.Helper()
	if got.Success != wantSuccess {
		t.Errorf("Success = %v, want %v (result %+v)", got.Success, wantSuccess, got)
	}
	if got.Alternative != wantAlt {
		t.Errorf("Alternative = %v, want %v (result %+v)", got.Alternative, wantAlt, got)
	}
	if got.EndX != wantEndX || got.EndY != wantEndY {
		t.Errorf("end = (%d,%d), want (%d,%d)", got.EndX, got.EndY, wantEndX, wantEndY)
	}
	if !reflect.DeepEqual(got.Waypoints, wantWaypoints) {
		t.Errorf("waypoints = %v, want %v", got.Waypoints, wantWaypoints)
	}
}

// expandSteps walks the compressed waypoint list back into single
// steps: consecutive waypoints must lie on one axis/diagonal run from
// the previous point (that is what direction-change compression
// guarantees), so each leg expands by repeated unit steps in the leg's
// direction. It fails the test if a leg is not a straight run.
func expandSteps(t *testing.T, src Location, waypoints []RouteWaypoint) []Location {
	t.Helper()
	var steps []Location
	cur := src
	for _, wp := range waypoints {
		dx := wp.X - cur.X
		dy := wp.Y - cur.Y
		if dx != 0 && dy != 0 && (dx < 0) != (dy < 0) {
			// Still a straight diagonal run; sign handling below covers it.
		}
		if dx != 0 && dy != 0 && abs(dx) != abs(dy) {
			t.Fatalf("waypoint %v is not on a straight/diagonal run from %v", wp, cur)
		}
		sx, sy := stepSign(dx), stepSign(dy)
		for cur.X != wp.X || cur.Y != wp.Y {
			cur = Location{X: cur.X + sx, Y: cur.Y + sy, Z: cur.Z}
			steps = append(steps, cur)
		}
		if cur.Z != wp.Z {
			t.Fatalf("waypoint %v has Z %d, want %d", wp, wp.Z, cur.Z)
		}
	}
	return steps
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func stepSign(n int) int {
	switch {
	case n < 0:
		return -1
	case n > 0:
		return 1
	default:
		return 0
	}
}

func TestRouteFindOpenGroundStraight(t *testing.T) {
	f := NewRouteFinder(openLookup())
	got := f.FindRoute(routeOpts(100, 100, 105, 100))
	// One run east: a single waypoint at the destination.
	checkRoute(t, got, []RouteWaypoint{{X: 105, Y: 100, Z: 0}}, false, true, 105, 100)
}

func TestRouteFindOpenGroundDiagonal(t *testing.T) {
	f := NewRouteFinder(openLookup())
	got := f.FindRoute(routeOpts(100, 100, 103, 103))
	// Three NE diagonal steps share one stored direction, so they
	// compress to a single waypoint at the destination.
	checkRoute(t, got, []RouteWaypoint{{X: 103, Y: 103, Z: 0}}, false, true, 103, 103)
}

func TestRouteFindLShape(t *testing.T) {
	// HAND TRACE (BFS tie-break): dest (103,101) is Chebyshev 3 from
	// src, so the shortest path is 3 steps: two east + one NE diagonal
	// in some order. Expansion order is W,E,S,N,SW,SE,NW,NE. The dist-1
	// queue is (99,100),(101,100),(100,99),(100,101),(99,99),(101,99),
	// (99,101),(101,101); expanding (101,100) second enqueues (102,100)
	// (dirWest) before any other cell can claim it, and expanding
	// (102,100) — the first dist-2 cell adjacent to the dest — enqueues
	// (103,101) via its NE step (dirSouthWest). Extraction therefore
	// yields the east run first, then the diagonal: waypoints
	// (102,100),(103,101).
	f := NewRouteFinder(openLookup())
	got := f.FindRoute(routeOpts(100, 100, 103, 101))
	checkRoute(t, got, []RouteWaypoint{{X: 102, Y: 100, Z: 0}, {X: 103, Y: 101, Z: 0}}, false, true, 103, 101)
}

func TestRouteFindSrcEqualsDest(t *testing.T) {
	f := NewRouteFinder(openLookup())
	got := f.FindRoute(routeOpts(100, 100, 100, 100))
	// reached() fires on the dequeued source cell; extraction walks
	// zero steps, so the route succeeds with no waypoints.
	checkRoute(t, got, []RouteWaypoint{}, false, true, 100, 100)
}

func TestRouteFindWallDetour(t *testing.T) {
	// A wall column at x=102, y=98..102; src west, dest east. The route
	// must detour around the north or south end (which end is a BFS
	// tie-break and is deliberately not pinned).
	f := NewRouteFinder(blockedColumn(102, 98, 102))
	got := f.FindRoute(routeOpts(100, 100, 104, 100))
	if !got.Success || got.Alternative {
		t.Fatalf("result = %+v, want success non-alternative", got)
	}
	if got.EndX != 104 || got.EndY != 100 {
		t.Fatalf("end = (%d,%d), want (104,100)", got.EndX, got.EndY)
	}
	if len(got.Waypoints) < 2 {
		t.Fatalf("waypoints = %v, want a multi-leg detour", got.Waypoints)
	}
	steps := expandSteps(t, Location{X: 100, Y: 100}, got.Waypoints)
	if len(steps) == 0 || steps[len(steps)-1] != (Location{X: 104, Y: 100}) {
		t.Fatalf("expanded steps end at %v, want (104,100)", steps[len(steps)-1])
	}
	for _, s := range steps {
		if s.X == 102 && s.Y >= 98 && s.Y <= 102 {
			t.Fatalf("step %v crosses the blocked wall column", s)
		}
	}
}

func TestRouteFindDiagonalCornerCut(t *testing.T) {
	// HAND TRACE (diagonal corner rule): dest (101,101) with the east
	// side tile (101,100) blocked. The NE diagonal from src requires
	// the side tiles (101,100) [BLOCK_EAST] and (100,101) [BLOCK_NORTH]
	// to be clear; (101,100) carries BLOCK_WALK, so the diagonal is
	// never enqueued and the route goes north then east: waypoints
	// (100,101),(101,101).
	g := flagGrid{[2]int{101, 100}: blockWalk}
	f := NewRouteFinder(g.lookup)
	got := f.FindRoute(routeOpts(100, 100, 101, 101))
	checkRoute(t, got, []RouteWaypoint{{X: 100, Y: 101, Z: 0}, {X: 101, Y: 101, Z: 0}}, false, true, 101, 101)
}

func TestRouteFindDiagonalBlockedBothSides(t *testing.T) {
	// Both side tiles blocked: the diagonal is impossible and the
	// direct neighbours are walls, so the route must loop around.
	// Assert it still succeeds without ever stepping on a blocked tile
	// and without taking the diagonal first step.
	g := flagGrid{[2]int{101, 100}: blockWalk, [2]int{100, 101}: blockWalk}
	f := NewRouteFinder(g.lookup)
	got := f.FindRoute(routeOpts(100, 100, 101, 101))
	if !got.Success || got.Alternative {
		t.Fatalf("result = %+v, want success non-alternative", got)
	}
	steps := expandSteps(t, Location{X: 100, Y: 100}, got.Waypoints)
	if len(steps) < 3 {
		t.Fatalf("steps = %v, want a loop-around of 3+ steps", steps)
	}
	if steps[0] == (Location{X: 101, Y: 101}) {
		t.Fatalf("first step cuts the blocked corner diagonally")
	}
	for _, s := range steps {
		if g[[2]int{s.X, s.Y}] != 0 {
			t.Fatalf("step %v lands on a blocked tile", s)
		}
	}
}

func TestRouteFindDiagonalWallBitOnDest(t *testing.T) {
	// A WALL_WEST bit on the destination tile blocks the NE diagonal
	// (BLOCK_NORTH_EAST includes WALL_WEST) and the east step into the
	// dest from (100,101) (BLOCK_EAST includes WALL_WEST), but not the
	// north step from (101,100) (BLOCK_NORTH checks WALL_SOUTH). Route:
	// east to (101,100), then north into the dest.
	g := flagGrid{[2]int{101, 101}: wallWest}
	f := NewRouteFinder(g.lookup)
	got := f.FindRoute(routeOpts(100, 100, 101, 101))
	checkRoute(t, got, []RouteWaypoint{{X: 101, Y: 100, Z: 0}, {X: 101, Y: 101, Z: 0}}, false, true, 101, 101)
}

func TestRouteFindEnclosedMoveNear(t *testing.T) {
	// All 8 neighbours blocked: the BFS cannot move. moveNear falls
	// back to the approach scan, whose only visited cell is the source
	// itself (cost 25, the only candidate) — an alternative route with
	// no waypoints ending at the source.
	g := flagGrid{}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx != 0 || dy != 0 {
				g[[2]int{100 + dx, 100 + dy}] = blockWalk
			}
		}
	}
	f := NewRouteFinder(g.lookup)
	got := f.FindRoute(routeOpts(100, 100, 105, 100))
	checkRoute(t, got, []RouteWaypoint{}, true, true, 100, 100)
}

func TestRouteFindEnclosedNoMoveNear(t *testing.T) {
	g := flagGrid{}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			if dx != 0 || dy != 0 {
				g[[2]int{100 + dx, 100 + dy}] = blockWalk
			}
		}
	}
	f := NewRouteFinder(g.lookup)
	opts := routeOpts(100, 100, 105, 100)
	opts.MoveNear = false
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{}, false, false, 100, 100)
}

func TestRouteFindBlockedDestinationMoveNear(t *testing.T) {
	// HAND TRACE (approach-point search): dest (105,100) is blocked, so
	// no route reaches it. The approach scan (±10 around dest) scores
	// cost = squared distance to the 1x1 dest rect over visited cells
	// with BFS distance < 100. Cost-1 cells are (104,100) d=4,
	// (105,99) d=5, (105,101) d=5, (106,100) d=6; the cost tie breaks
	// toward the smallest BFS distance, so the approach point is
	// (104,100) — an alternative route ending one tile short.
	g := flagGrid{[2]int{105, 100}: blockWalk}
	f := NewRouteFinder(g.lookup)
	got := f.FindRoute(routeOpts(100, 100, 105, 100))
	checkRoute(t, got, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, true, true, 104, 100)
}

func TestRouteFindBlockedDestinationNoMoveNear(t *testing.T) {
	g := flagGrid{[2]int{105, 100}: blockWalk}
	f := NewRouteFinder(g.lookup)
	opts := routeOpts(100, 100, 105, 100)
	opts.MoveNear = false
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{}, false, false, 100, 100)
}

func TestRouteFindOutOfSearchArea(t *testing.T) {
	// Dest 100 tiles east: localDestX is 164, outside the 128-wide
	// search map, and the ±10 approach scan never enters the map, so
	// even moveNear fails.
	f := NewRouteFinder(openLookup())
	got := f.FindRoute(routeOpts(100, 100, 200, 100))
	checkRoute(t, got, []RouteWaypoint{}, false, false, 100, 100)
}

func TestRouteFindNilLookupIsOpenGround(t *testing.T) {
	f := NewRouteFinder(nil)
	got := f.FindRoute(routeOpts(100, 100, 105, 100))
	checkRoute(t, got, []RouteWaypoint{{X: 105, Y: 100, Z: 0}}, false, true, 105, 100)
}

func TestRouteFindSize2OpenGround(t *testing.T) {
	f := NewRouteFinder(openLookup())
	opts := routeOpts(100, 100, 104, 100)
	opts.SrcSize = 2
	got := f.FindRoute(opts)
	// routeFindBig: four east steps of the footprint corner compress
	// to one waypoint.
	checkRoute(t, got, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, false, true, 104, 100)
}

func TestRouteFindSize2ThroughWideGap(t *testing.T) {
	// Wall column x=102, y=98..104 with a 2-tall gap at y=100..101:
	// the 2x2 footprint fits through, so the route is straight east.
	g := flagGrid{}
	for y := 98; y <= 104; y++ {
		if y == 100 || y == 101 {
			continue
		}
		g[[2]int{102, y}] = blockWalk
	}
	f := NewRouteFinder(g.lookup)
	opts := routeOpts(100, 100, 104, 100)
	opts.SrcSize = 2
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, false, true, 104, 100)
}

func TestRouteFindSize2NarrowGapDetour(t *testing.T) {
	// Same wall but a 1-tall gap (y=101 only): the 2x2 footprint can
	// never occupy the gap column, so it must detour around an end.
	// Assert the footprint never overlaps a blocked tile.
	g := flagGrid{}
	for y := 98; y <= 104; y++ {
		if y == 101 {
			continue
		}
		g[[2]int{102, y}] = blockWalk
	}
	f := NewRouteFinder(g.lookup)
	opts := routeOpts(100, 100, 104, 100)
	opts.SrcSize = 2
	got := f.FindRoute(opts)
	if !got.Success || got.Alternative {
		t.Fatalf("result = %+v, want success non-alternative", got)
	}
	if got.EndX != 104 || got.EndY != 100 {
		t.Fatalf("end = (%d,%d), want (104,100)", got.EndX, got.EndY)
	}
	steps := expandSteps(t, Location{X: 100, Y: 100}, got.Waypoints)
	for _, s := range steps {
		for dx := 0; dx < 2; dx++ {
			for dy := 0; dy < 2; dy++ {
				if g[[2]int{s.X + dx, s.Y + dy}] != 0 {
					t.Fatalf("footprint at corner %v overlaps blocked tile (%d,%d)", s, s.X+dx, s.Y+dy)
				}
			}
		}
	}
}

func TestRouteFindSize2DiagonalFootprintBlocked(t *testing.T) {
	// A blocked tile at (102,101) kills the size-2 NE diagonal (the
	// swept edge check reads it) and the east step, forcing a northern
	// loop. Assert success with no footprint overlap.
	g := flagGrid{[2]int{102, 101}: blockWalk}
	f := NewRouteFinder(g.lookup)
	opts := routeOpts(100, 100, 102, 102)
	opts.SrcSize = 2
	got := f.FindRoute(opts)
	if !got.Success || got.Alternative {
		t.Fatalf("result = %+v, want success non-alternative", got)
	}
	steps := expandSteps(t, Location{X: 100, Y: 100}, got.Waypoints)
	if len(steps) < 3 {
		t.Fatalf("steps = %v, want a loop of 3+ steps, not a 2-step diagonal", steps)
	}
	for _, s := range steps {
		for dx := 0; dx < 2; dx++ {
			for dy := 0; dy < 2; dy++ {
				if g[[2]int{s.X + dx, s.Y + dy}] != 0 {
					t.Fatalf("footprint at corner %v overlaps blocked tile (%d,%d)", s, s.X+dx, s.Y+dy)
				}
			}
		}
	}
}

func TestRouteFindMaxWaypointsCap(t *testing.T) {
	// The L-shaped route has 2 direction runs. With MaxWaypoints=1,
	// extraction pops the destination-side waypoint before unshifting
	// the second run's start, so the kept waypoint is the run boundary
	// (102,100) — while EndX/EndY still report the true end (103,101).
	// This pins the TS pop-then-unshift cap semantics exactly.
	f := NewRouteFinder(openLookup())
	opts := routeOpts(100, 100, 103, 101)
	opts.MaxWaypoints = 1
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{{X: 102, Y: 100, Z: 0}}, false, true, 103, 101)
}

func TestRouteFindDefaultCap25(t *testing.T) {
	// A walled corridor forces a zigzag with far more than 25 direction
	// runs (verified below by running the same search uncapped). The
	// corridor walls span the whole search map (baseX=36..163 for a
	// source at x=100), so no around-the-end bypass exists; alternating
	// baffles inside force the path to dip south, rise north, and
	// repeat. With the default cap, exactly 25 waypoints survive, they
	// are the first 25 checkpoints of the uncapped route (source-side
	// prefix: the pop drops destination-side waypoints), and EndX/EndY
	// still report the true end.
	g := flagGrid{}
	for x := 36; x <= 163; x++ {
		g[[2]int{x, 100}] = blockWalk
		g[[2]int{x, 110}] = blockWalk
	}
	for k := 0; k < 16; k++ {
		x := 103 + 3*k
		if k%2 == 0 {
			// North baffle: gap at the south (y=101..104).
			for y := 105; y <= 109; y++ {
				g[[2]int{x, y}] = blockWalk
			}
		} else {
			// South baffle: gap at the north (y=106..109).
			for y := 101; y <= 105; y++ {
				g[[2]int{x, y}] = blockWalk
			}
		}
	}
	f := NewRouteFinder(g.lookup)
	uncapped := routeOpts(100, 105, 152, 105)
	uncapped.MaxWaypoints = 1000
	full := f.FindRoute(uncapped)
	if !full.Success {
		t.Fatalf("uncapped route failed: %+v", full)
	}
	if len(full.Waypoints) <= 25 {
		t.Fatalf("fixture produces only %d runs; need >25 to exercise the cap", len(full.Waypoints))
	}
	capped := f.FindRoute(routeOpts(100, 105, 152, 105))
	if !capped.Success || capped.Alternative {
		t.Fatalf("capped route = %+v, want success non-alternative", capped)
	}
	if len(capped.Waypoints) != 25 {
		t.Fatalf("capped waypoints = %d, want 25", len(capped.Waypoints))
	}
	if !reflect.DeepEqual(capped.Waypoints, full.Waypoints[:25]) {
		t.Errorf("capped waypoints = %v, want prefix %v", capped.Waypoints, full.Waypoints[:25])
	}
	if capped.EndX != full.EndX || capped.EndY != full.EndY {
		t.Errorf("capped end = (%d,%d), want (%d,%d)", capped.EndX, capped.EndY, full.EndX, full.EndY)
	}
}

func TestRouteFindDefaults(t *testing.T) {
	// DestWidth/DestLength <= 0 mean 1; MaxWaypoints <= 0 means 25.
	f := NewRouteFinder(openLookup())
	opts := routeOpts(100, 100, 103, 100)
	opts.DestWidth, opts.DestLength, opts.MaxWaypoints = 0, 0, 0
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{{X: 103, Y: 100, Z: 0}}, false, true, 103, 100)
}

func TestRouteFinderReuse(t *testing.T) {
	// A finder reused across calls resets its BFS arrays: a route
	// computed after a wall detour must not retain stale directions.
	g := flagGrid{}
	lookup := func(x, y, level int) int32 { return g[[2]int{x, y}] }
	f := NewRouteFinder(lookup)
	for y := 98; y <= 102; y++ {
		g[[2]int{102, y}] = blockWalk
	}
	first := f.FindRoute(routeOpts(100, 100, 104, 100))
	if !first.Success || len(first.Waypoints) < 2 {
		t.Fatalf("first route = %+v, want a detour", first)
	}
	delete(g, [2]int{102, 100})
	for y := 98; y <= 102; y++ {
		delete(g, [2]int{102, y})
	}
	second := f.FindRoute(routeOpts(100, 100, 104, 100))
	checkRoute(t, second, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, false, true, 104, 100)
}

func TestRouteFindLevelPropagates(t *testing.T) {
	var sawLevel = -1
	lookup := func(x, y, level int) int32 {
		sawLevel = level
		return 0
	}
	f := NewRouteFinder(lookup)
	opts := routeOpts(100, 100, 102, 100)
	opts.Level = 2
	got := f.FindRoute(opts)
	checkRoute(t, got, []RouteWaypoint{{X: 102, Y: 100, Z: 2}}, false, true, 102, 100)
	if sawLevel != 2 {
		t.Errorf("lookup saw level %d, want 2", sawLevel)
	}
}

func reachOpts(srcX, srcY, srcSize, destX, destY int) ReachOptions {
	return ReachOptions{
		Level: 0, SrcX: srcX, SrcY: srcY, SrcSize: srcSize,
		DestX: destX, DestY: destY, DestWidth: 1, DestLength: 1,
		LocAngle: 0, LocShape: -1, BlockAccessFlags: 0,
	}
}

func TestReachedAbsoluteNoStrategy(t *testing.T) {
	f := NewRouteFinder(openLookup())
	// locShape -1: only the exact destination tile reaches.
	if !f.ReachedAbsolute(reachOpts(105, 100, 1, 105, 100)) {
		t.Error("standing on dest with locShape -1 should reach")
	}
	if f.ReachedAbsolute(reachOpts(104, 100, 1, 105, 100)) {
		t.Error("adjacent tile with locShape -1 should not reach")
	}
}

func TestReachedAbsoluteWall(t *testing.T) {
	f := NewRouteFinder(openLookup())
	// locShape 0 (straight wall), angle 0: west neighbour reaches
	// unconditionally; north/south neighbours reach when their facing
	// BLOCK mask is clear; the east neighbour does not reach.
	wall := func(sx, sy int) ReachOptions {
		o := reachOpts(sx, sy, 1, 105, 100)
		o.LocShape = 0
		o.LocAngle = 0
		return o
	}
	if !f.ReachedAbsolute(wall(104, 100)) {
		t.Error("west neighbour of shape-0 angle-0 wall should reach")
	}
	if !f.ReachedAbsolute(wall(105, 101)) {
		t.Error("north neighbour of shape-0 angle-0 wall should reach on open ground")
	}
	if !f.ReachedAbsolute(wall(105, 99)) {
		t.Error("south neighbour of shape-0 angle-0 wall should reach on open ground")
	}
	if f.ReachedAbsolute(wall(106, 100)) {
		t.Error("east neighbour of shape-0 angle-0 wall should not reach")
	}
	if f.ReachedAbsolute(wall(104, 101)) {
		t.Error("diagonal neighbour of wall should not reach")
	}

	// A blocked north-neighbour tile fails the BLOCK_NORTH check.
	g := flagGrid{[2]int{105, 101}: blockWalk}
	fb := NewRouteFinder(g.lookup)
	if fb.ReachedAbsolute(wall(105, 101)) {
		t.Error("blocked north neighbour should not reach the wall")
	}
	// The west neighbour reaches even when blocked: its table entry
	// has no flag check (the BFS can never stand there, but
	// reachedAbsolute answers the table literally).
	if !fb.ReachedAbsolute(wall(104, 100)) {
		t.Error("west neighbour table entry should reach regardless of flags")
	}

	// Block-access: approaching from the west with BLOCK_ACCESS_WEST
	// set is rejected before the wall table is consulted.
	blocked := wall(104, 100)
	blocked.BlockAccessFlags = blockAccessWest
	if f.ReachedAbsolute(blocked) {
		t.Error("west approach with BLOCK_ACCESS_WEST should not reach")
	}
}

func TestReachedAbsoluteWallDeco(t *testing.T) {
	f := NewRouteFinder(openLookup())
	deco := func(sx, sy, shape, angle int) ReachOptions {
		o := reachOpts(sx, sy, 1, 105, 100)
		o.LocShape = shape
		o.LocAngle = angle
		return o
	}
	// Shape 6, angle 0: east and south neighbours reach.
	if !f.ReachedAbsolute(deco(106, 100, 6, 0)) {
		t.Error("east neighbour of shape-6 angle-0 wall deco should reach")
	}
	if !f.ReachedAbsolute(deco(105, 99, 6, 0)) {
		t.Error("south neighbour of shape-6 angle-0 wall deco should reach")
	}
	if f.ReachedAbsolute(deco(104, 100, 6, 0)) {
		t.Error("west neighbour of shape-6 angle-0 wall deco should not reach")
	}
	// Shape 7 flips the angle by 2: shape 7 angle 0 behaves like
	// shape 6 angle 2 (west and north neighbours).
	if !f.ReachedAbsolute(deco(104, 100, 7, 0)) {
		t.Error("west neighbour of shape-7 angle-0 wall deco should reach (angle flip)")
	}
	if !f.ReachedAbsolute(deco(105, 101, 7, 0)) {
		t.Error("north neighbour of shape-7 angle-0 wall deco should reach (angle flip)")
	}
	if f.ReachedAbsolute(deco(106, 100, 7, 0)) {
		t.Error("east neighbour of shape-7 angle-0 wall deco should not reach")
	}
	// Shape 8 reaches from any orthogonal neighbour.
	if !f.ReachedAbsolute(deco(104, 100, 8, 0)) || !f.ReachedAbsolute(deco(105, 101, 8, 0)) {
		t.Error("orthogonal neighbours of shape-8 wall deco should reach")
	}
	// Shapes 4/5 reach only from their own tile (exact match).
	if f.ReachedAbsolute(deco(104, 100, 4, 0)) {
		t.Error("neighbour of shape-4 wall deco should not reach")
	}
	if !f.ReachedAbsolute(deco(105, 100, 4, 0)) {
		t.Error("own tile of shape-4 wall deco should reach (collides shortcut)")
	}
}

func TestReachedAbsoluteRectangle(t *testing.T) {
	f := NewRouteFinder(openLookup())
	rect := func(sx, sy int) ReachOptions {
		o := reachOpts(sx, sy, 1, 105, 100)
		o.DestWidth, o.DestLength = 2, 2
		o.LocShape = 10
		return o
	}
	// Inside the 2x2 rect reaches (overlap).
	if !f.ReachedAbsolute(rect(106, 101)) {
		t.Error("tile inside rectangle should reach")
	}
	// Each side's adjacent tile reaches; diagonals do not.
	if !f.ReachedAbsolute(rect(104, 101)) {
		t.Error("west side tile should reach rectangle")
	}
	if !f.ReachedAbsolute(rect(107, 100)) {
		t.Error("east side tile should reach rectangle")
	}
	if !f.ReachedAbsolute(rect(105, 99)) {
		t.Error("south side tile should reach rectangle")
	}
	if !f.ReachedAbsolute(rect(106, 102)) {
		t.Error("north side tile should reach rectangle")
	}
	if f.ReachedAbsolute(rect(104, 102)) {
		t.Error("diagonal tile should not reach rectangle")
	}
	if f.ReachedAbsolute(rect(104, 99)) {
		t.Error("off-side tile should not reach rectangle")
	}
	// A wall bit facing the rectangle on the actor's tile blocks that side.
	g := flagGrid{[2]int{104, 101}: wallEast}
	fw := NewRouteFinder(g.lookup)
	if fw.ReachedAbsolute(rect(104, 101)) {
		t.Error("west side tile with WALL_EAST should not reach rectangle")
	}
	// Block-access on the approached side blocks it.
	blocked := rect(104, 101)
	blocked.BlockAccessFlags = blockAccessWest
	if f.ReachedAbsolute(blocked) {
		t.Error("west side tile with BLOCK_ACCESS_WEST should not reach rectangle")
	}
}

func TestReachedAbsoluteRectangleExclusive(t *testing.T) {
	f := NewRouteFinder(openLookup())
	excl := func(sx, sy int) ReachOptions {
		o := reachOpts(sx, sy, 1, 105, 100)
		o.LocShape = -2
		return o
	}
	// Exclusive: standing ON the dest does NOT reach; adjacent does.
	if f.ReachedAbsolute(excl(105, 100)) {
		t.Error("own tile should not reach rectangle-exclusive")
	}
	if !f.ReachedAbsolute(excl(104, 100)) {
		t.Error("adjacent tile should reach rectangle-exclusive")
	}
	if f.ReachedAbsolute(excl(104, 101)) {
		t.Error("diagonal tile should not reach rectangle-exclusive")
	}
}

func TestReachedAbsoluteRectangleRotation(t *testing.T) {
	f := NewRouteFinder(openLookup())
	// Dest 2 wide x 1 long at (105,100). With angle 1 the footprint
	// rotates to 1 wide x 2 long, so (105,102) becomes the north-side
	// tile and reaches; with angle 0 it is two tiles north and does not.
	rot := func(sx, sy, angle int) ReachOptions {
		o := reachOpts(sx, sy, 1, 105, 100)
		o.DestWidth, o.DestLength = 2, 1
		o.LocShape = 10
		o.LocAngle = angle
		return o
	}
	if !f.ReachedAbsolute(rot(105, 102, 1)) {
		t.Error("north-side tile of rotated rectangle should reach")
	}
	if f.ReachedAbsolute(rot(105, 102, 0)) {
		t.Error("two-north tile of unrotated rectangle should not reach")
	}
}

func TestReachedAbsoluteRectangleN(t *testing.T) {
	f := NewRouteFinder(openLookup())
	// Size-2 actor flush against the west side of a 2x2 rect: the
	// footprint edge (103..104, 100..101) abuts dest x=105.
	o := reachOpts(103, 100, 2, 105, 100)
	o.DestWidth, o.DestLength = 2, 2
	o.LocShape = 10
	if !f.ReachedAbsolute(o) {
		t.Error("size-2 footprint flush against rectangle side should reach")
	}
	// One tile further west: no shared edge.
	o.SrcX = 102
	if f.ReachedAbsolute(o) {
		t.Error("size-2 footprint one tile off the rectangle should not reach")
	}
}

func TestReachedAbsoluteWallN(t *testing.T) {
	f := NewRouteFinder(openLookup())
	// Size-2 actor west of a shape-0 angle-0 wall at (105,100): the
	// footprint (103..104, 100..101) abuts the wall from the west.
	o := reachOpts(103, 100, 2, 105, 100)
	o.LocShape = 0
	o.LocAngle = 0
	if !f.ReachedAbsolute(o) {
		t.Error("size-2 footprint abutting wall from the west should reach")
	}
}

func TestRotateHelpers(t *testing.T) {
	if got := rotateDimensions(0, 2, 3); got != 2 {
		t.Errorf("rotateDimensions(0,2,3) = %d, want 2", got)
	}
	if got := rotateDimensions(1, 2, 3); got != 3 {
		t.Errorf("rotateDimensions(1,2,3) = %d, want 3", got)
	}
	if got := rotateDimensions(2, 2, 3); got != 2 {
		t.Errorf("rotateDimensions(2,2,3) = %d, want 2", got)
	}
	// Rotating WEST (0x8) by one quarter-turn yields NORTH (0x1):
	// ((8<<1)&0xf)|(8>>3) = 0|1.
	if got := rotateBlockAccess(1, blockAccessWest); got != blockAccessNorth {
		t.Errorf("rotateBlockAccess(1, WEST) = %x, want %x", got, blockAccessNorth)
	}
	if got := rotateBlockAccess(0, blockAccessWest); got != blockAccessWest {
		t.Errorf("rotateBlockAccess(0, WEST) = %x, want %x", got, blockAccessWest)
	}
	if !collides(100, 100, 101, 101, 2, 2, 1, 1) {
		t.Error("overlapping rectangles should collide")
	}
	if collides(100, 100, 102, 100, 2, 2, 1, 1) {
		t.Error("edge-abutting rectangles should not collide")
	}
}
