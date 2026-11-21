package game

import "testing"

// The actor-aware collision surfaces (LookupForActor /
// CollisionForActor) are the coordinator's integration seam for
// private areas: the movement queue and the route finder read
// the actor's CURRENT area on every call, so boarding a boat or
// entering an instance re-routes on the area's clips without
// reinstalling, and leaving restores main-world clips. These
// tests pin the dynamic read; the clip-consultation semantics
// themselves are pinned in regions_private_area_test.go.

func TestLookupForActorDynamic(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	x, y := 3200, 3200
	m.AddClipping(x, y, 0, 0x100)
	pa.SetClip(Location{X: x, Y: y, Z: 0}, 0x200)

	var current *PrivateArea
	lookup := m.LookupForActor(func() *PrivateArea { return current })
	if got := lookup(x, y, 0); got != 0x100 {
		t.Fatalf("outside area: lookup = %#x, want region clip 0x100", got)
	}
	current = pa
	if got := lookup(x, y, 0); got != 0x200 {
		t.Fatalf("inside area: lookup = %#x, want area clip 0x200", got)
	}
	current = nil
	if got := lookup(x, y, 0); got != 0x100 {
		t.Fatalf("after leaving: lookup = %#x, want region clip 0x100", got)
	}
}

func TestCollisionForActorDynamic(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	start := Location{X: 3200, Y: 3200, Z: 0}
	end := Location{X: 3201, Y: 3200, Z: 0}
	pa.SetClip(end, ClipBlockedTile)

	var current *PrivateArea
	collision := m.CollisionForActor(func() *PrivateArea { return current })
	if !collision.CanMoveStart(start, end, 1) {
		t.Fatalf("outside area: CanMoveStart = false, want true (open ground)")
	}
	current = pa
	if collision.CanMoveStart(start, end, 1) {
		t.Fatalf("inside area: CanMoveStart = true, want false (area clip blocks the step)")
	}
	current = nil
	if !collision.CanMoveStart(start, end, 1) {
		t.Fatalf("after leaving: CanMoveStart = false, want true (main-world clips restored)")
	}
}

func TestRouteFinderActorAware(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	// A wall column at x=3202, y=3198..3202, in the AREA only;
	// the region grid stays open.
	for y := 3198; y <= 3202; y++ {
		pa.SetClip(Location{X: 3202, Y: y, Z: 0}, ClipBlockedTile)
	}

	var current *PrivateArea
	f := NewRouteFinder(m.LookupForActor(func() *PrivateArea { return current }))

	// Outside the area the route goes straight through the
	// column's tiles: the area's wall does not exist there.
	got := f.FindRoute(routeOpts(3200, 3200, 3204, 3200))
	if !got.Success || got.EndX != 3204 || got.EndY != 3200 {
		t.Fatalf("outside area: result = %+v, want success to (3204,3200)", got)
	}
	steps := expandSteps(t, Location{X: 3200, Y: 3200}, got.Waypoints)
	crossed := false
	for _, s := range steps {
		if s.X == 3202 && s.Y == 3200 {
			crossed = true
		}
	}
	if !crossed {
		t.Fatalf("outside area: steps %v never cross (3202,3200), want the straight route", steps)
	}

	// Inside the area the same route detours around the wall.
	current = pa
	got = f.FindRoute(routeOpts(3200, 3200, 3204, 3200))
	if !got.Success || got.Alternative || got.EndX != 3204 || got.EndY != 3200 {
		t.Fatalf("inside area: result = %+v, want success non-alternative to (3204,3200)", got)
	}
	steps = expandSteps(t, Location{X: 3200, Y: 3200}, got.Waypoints)
	for _, s := range steps {
		if s.X == 3202 && s.Y >= 3198 && s.Y <= 3202 {
			t.Fatalf("inside area: step %v crosses the area wall column", s)
		}
	}
}
