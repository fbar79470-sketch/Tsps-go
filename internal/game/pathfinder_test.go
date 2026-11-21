package game

import (
	"reflect"
	"testing"
)

// WalkRoute pins the calculateWalkRoute call shape (PathFinder.ts:
// 412-422 -> applyRsmodRoute): locShape -1, moveNear true, maxWaypoints
// 25, 1x1 destination, level from the source tile. The queue mutation
// in applyRsmodRoute belongs to the movement-queue worker and is not
// tested here.

func TestWalkRouteOpenGround(t *testing.T) {
	f := NewRouteFinder(openLookup())
	got := WalkRoute(f, Location{X: 100, Y: 100, Z: 0}, 1, 105, 100)
	checkRoute(t, got, []RouteWaypoint{{X: 105, Y: 100, Z: 0}}, false, true, 105, 100)
}

func TestWalkRouteMatchesExplicitFindRoute(t *testing.T) {
	// WalkRoute must equal FindRoute with the exact applyRsmodRoute
	// options: level from src, 1x1 dest, angle 0, shape -1, moveNear,
	// no block-access flags, 25 waypoints.
	f := NewRouteFinder(openLookup())
	src := Location{X: 100, Y: 100, Z: 1}
	want := f.FindRoute(RouteOptions{
		Level: 1, SrcX: 100, SrcY: 100, SrcSize: 1,
		DestX: 103, DestY: 101, DestWidth: 1, DestLength: 1,
		LocAngle: 0, LocShape: -1, BlockAccessFlags: 0,
		MaxWaypoints: 25, MoveNear: true,
	})
	got := WalkRoute(f, src, 1, 103, 101)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WalkRoute = %+v, want FindRoute-equivalent %+v", got, want)
	}
	if got.Waypoints[0].Z != 1 {
		t.Errorf("waypoint Z = %d, want 1 (source level)", got.Waypoints[0].Z)
	}
}

func TestWalkRouteNilFinderIsOpenGround(t *testing.T) {
	got := WalkRoute(nil, Location{X: 100, Y: 100, Z: 0}, 1, 102, 100)
	checkRoute(t, got, []RouteWaypoint{{X: 102, Y: 100, Z: 0}}, false, true, 102, 100)
}

func TestWalkRouteBlockedDestinationSettlesNear(t *testing.T) {
	// moveNear=true (the walk-route shape): a blocked destination
	// settles for the closest approach tile, flagged alternative.
	g := flagGrid{[2]int{105, 100}: blockWalk}
	f := NewRouteFinder(g.lookup)
	got := WalkRoute(f, Location{X: 100, Y: 100, Z: 0}, 1, 105, 100)
	checkRoute(t, got, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, true, true, 104, 100)
}

func TestWalkRouteSize2(t *testing.T) {
	f := NewRouteFinder(openLookup())
	got := WalkRoute(f, Location{X: 100, Y: 100, Z: 0}, 2, 104, 100)
	checkRoute(t, got, []RouteWaypoint{{X: 104, Y: 100, Z: 0}}, false, true, 104, 100)
}
