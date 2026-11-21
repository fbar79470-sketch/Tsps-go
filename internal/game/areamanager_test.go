package game

import "testing"

// logArea returns an area whose post-enter/post-leave/process
// hooks append to log.
func logArea(name string, bounds []Boundary, log *[]string) *Area {
	a := NewArea(name, bounds)
	a.PostEnterHook = func(actor AreaActor) { *log = append(*log, "enter:"+name) }
	a.PostLeaveHook = func(actor AreaActor, logout bool) {
		*log = append(*log, "leave:"+name)
	}
	a.ProcessHook = func(actor AreaActor) { *log = append(*log, "process:"+name) }
	return a
}

func equalStrings(t *testing.T, got, want []string, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %q, want %q", what, i, got[i], want[i])
		}
	}
}

// Get returns the FIRST registered area containing the
// position (AreaManager.ts:127-138); registration order is
// priority order, in both directions.
func TestAreaManagerGetPriority(t *testing.T) {
	broad := []Boundary{NewBoundary(0, 100, 0, 100, 0)}
	narrow := []Boundary{NewBoundary(10, 20, 10, 20, 0)}
	overlap := Location{X: 15, Y: 15, Z: 0}

	mgr := NewAreaManager()
	broadArea := NewArea("broad", broad)
	narrowArea := NewArea("narrow", narrow)
	mgr.Register(broadArea)
	mgr.Register(narrowArea)
	if got := mgr.Get(overlap); got != broadArea {
		t.Errorf("Get(overlap) = %v, want broad (registered first)", areaName(got))
	}
	if got := mgr.Get(Location{X: 50, Y: 50, Z: 0}); got != broadArea {
		t.Errorf("Get(broad-only) = %v, want broad", areaName(got))
	}
	if got := mgr.Get(Location{X: 500, Y: 500, Z: 0}); got != nil {
		t.Errorf("Get(outside) = %v, want nil", areaName(got))
	}

	reversed := NewAreaManager()
	reversed.Register(narrowArea)
	reversed.Register(broadArea)
	if got := reversed.Get(overlap); got != narrowArea {
		t.Errorf("Get(overlap) reversed = %v, want narrow", areaName(got))
	}
}

func areaName(a *Area) string {
	if a == nil {
		return "<nil>"
	}
	return a.GetName()
}

func TestAreaManagerGetHeightAndEmpty(t *testing.T) {
	mgr := NewAreaManager()
	upper := NewArea("upper", []Boundary{NewBoundary(0, 10, 0, 10, 1)})
	heightless := NewArea("heightless", []Boundary{NewBoundary(0, 10, 0, 10)})
	empty := NewArea("empty", nil)
	mgr.Register(upper)
	mgr.Register(heightless)
	mgr.Register(empty)

	if got := mgr.Get(Location{X: 5, Y: 5, Z: 0}); got != nil {
		t.Errorf("Get(z=0) = %v, want nil (plane 1 only)", areaName(got))
	}
	if got := mgr.Get(Location{X: 5, Y: 5, Z: 1}); got != upper {
		t.Errorf("Get(z=1) = %v, want upper", areaName(got))
	}
	// The height-less boundary never matches, on any plane,
	// and the empty area never matches either.
	if got := mgr.Get(Location{X: 5, Y: 5, Z: 2}); got != nil {
		t.Errorf("Get(z=2) = %v, want nil", areaName(got))
	}
}

func TestAreaManagerInside(t *testing.T) {
	mgr := NewAreaManager()
	a := NewArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)})
	if !mgr.Inside(Location{X: 10, Y: 10, Z: 0}, a) {
		t.Error("Inside(max corner) = false, want true")
	}
	if mgr.Inside(Location{X: 11, Y: 5, Z: 0}, a) {
		t.Error("Inside(outside) = true, want false")
	}
	if mgr.Inside(Location{X: 5, Y: 5, Z: 1}, a) {
		t.Error("Inside(wrong plane) = true, want false")
	}
	if mgr.Inside(Location{X: 5, Y: 5, Z: 0}, NewArea("empty", nil)) {
		t.Error("Inside(empty area) = true, want false")
	}
}

func TestAreaManagerInMulti(t *testing.T) {
	mgr := NewAreaManager()
	multi := NewArea("multi", nil)
	multi.IsMultiHook = func() bool { return true }
	plain := NewArea("plain", nil)

	player := newFakePlayer(1, Location{X: 3200, Y: 3600, Z: 0})
	player.SetArea(multi)
	if !mgr.InMulti(player) {
		t.Error("InMulti(multi area) = false, want true")
	}
	player.SetArea(plain)
	if mgr.InMulti(player) {
		t.Error("InMulti(plain area, no wilderness) = true, want false")
	}
	player.SetArea(nil)
	if mgr.InMulti(player) {
		t.Error("InMulti(no area, no wilderness) = true, want false")
	}

	// Wilderness fallback: a multi-combat zone under the
	// actor reports multi even with no (multi) area.
	wild := NewWilderness(mapZoneSource{
		"multi-combat": {NewBoundary(3000, 3300, 3500, 3700, 0)},
	}, nil)
	mgr.SetWilderness(wild)
	if !mgr.InMulti(player) {
		t.Error("InMulti(wilderness zone) = false, want true")
	}
	player.moveTo(Location{X: 100, Y: 100, Z: 0})
	if mgr.InMulti(player) {
		t.Error("InMulti(outside zone) = true, want false")
	}
}

func TestAreaManagerProcessEnterLeaveStay(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	a := logArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)}, &log)
	mgr.Register(a)
	player := newFakePlayer(1, Location{X: 50, Y: 50, Z: 0})

	mgr.Process(player)
	if player.Area() != nil {
		t.Error("Area set while outside")
	}
	equalStrings(t, log, nil, "log outside")

	player.moveTo(Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	equalStrings(t, log, []string{"enter:A", "process:A"}, "log on enter")
	if player.Area() != a || !a.HasMember(player) {
		t.Error("actor not recorded as member after enter")
	}

	log = nil
	mgr.Process(player)
	equalStrings(t, log, []string{"process:A"}, "log stationary (no re-enter)")

	// Leave reports logout=false from Process
	// (AreaManager.ts:41).
	var logoutSeen []bool
	a.PostLeaveHook = func(actor AreaActor, logout bool) { logoutSeen = append(logoutSeen, logout) }
	player.moveTo(Location{X: 50, Y: 50, Z: 0})
	mgr.Process(player)
	if player.Area() != nil || a.HasMember(player) {
		t.Error("actor still in area after leaving")
	}
	if len(logoutSeen) != 1 || logoutSeen[0] {
		t.Errorf("leave logout = %v, want [false]", logoutSeen)
	}
}

// Moving inside a broad, LATER-registered area into a
// HIGHER-priority (earlier-registered) overlapping area
// re-resolves: leave the broad area, enter the narrow one
// (AreaManager.ts:32-39). The reverse priority keeps the
// actor in the broad area: first match wins.
func TestAreaManagerProcessReResolution(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	narrow := logArea("narrow", []Boundary{NewBoundary(10, 20, 10, 20, 0)}, &log)
	broad := logArea("broad", []Boundary{NewBoundary(0, 100, 0, 100, 0)}, &log)
	mgr.Register(narrow) // higher priority
	mgr.Register(broad)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})

	mgr.Process(player)
	equalStrings(t, log, []string{"enter:broad", "process:broad"}, "log in broad-only tile")
	if player.Area() != broad {
		t.Fatalf("Area = %v, want broad", areaName(player.Area()))
	}

	log = nil
	player.moveTo(Location{X: 15, Y: 15, Z: 0})
	mgr.Process(player)
	equalStrings(t, log, []string{"leave:broad", "enter:narrow", "process:narrow"}, "log moving into overlap")
	if player.Area() != narrow {
		t.Fatalf("Area = %v, want narrow", areaName(player.Area()))
	}

	// Stationary in the overlap: the hint fast path skips
	// re-resolution, so no churn.
	log = nil
	mgr.Process(player)
	equalStrings(t, log, []string{"process:narrow"}, "log stationary in overlap")

	// Moving back out of the narrow area re-enters broad.
	log = nil
	player.moveTo(Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	equalStrings(t, log, []string{"leave:narrow", "enter:broad", "process:broad"}, "log moving out of overlap")
}

func TestAreaManagerProcessStaysInBroadWhenBroadIsFirst(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	broad := logArea("broad", []Boundary{NewBoundary(0, 100, 0, 100, 0)}, &log)
	narrow := logArea("narrow", []Boundary{NewBoundary(10, 20, 10, 20, 0)}, &log)
	mgr.Register(broad) // higher priority
	mgr.Register(narrow)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	if player.Area() != broad {
		t.Fatalf("Area = %v, want broad", areaName(player.Area()))
	}
	log = nil
	player.moveTo(Location{X: 15, Y: 15, Z: 0})
	mgr.Process(player)
	equalStrings(t, log, []string{"process:broad"}, "log moving into overlap with broad first")
	if player.Area() != broad {
		t.Errorf("Area = %v, want broad", areaName(player.Area()))
	}
}

// An area NOT in the registry is never re-resolved into or
// out of by priority lookup (the `areas.includes(area)`
// guard, AreaManager.ts:35): an actor recorded in one stays
// while inside its bounds, and leaves only by walking out.
func TestAreaManagerProcessUnregisteredArea(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	registered := logArea("registered", []Boundary{NewBoundary(0, 100, 0, 100, 0)}, &log)
	mgr.Register(registered)
	unregistered := logArea("unregistered", []Boundary{NewBoundary(0, 100, 0, 100, 0)}, &log)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})

	// Place the actor in the unregistered area directly,
	// bypassing the manager (an instance claiming a player).
	unregistered.Enter(player)
	player.SetArea(unregistered)
	log = nil

	player.moveTo(Location{X: 6, Y: 6, Z: 0})
	mgr.Process(player)
	equalStrings(t, log, []string{"process:unregistered"}, "log moving inside unregistered area")
	if player.Area() != unregistered {
		t.Errorf("Area = %v, want unregistered", areaName(player.Area()))
	}

	player.moveTo(Location{X: 500, Y: 500, Z: 0})
	mgr.Process(player)
	if player.Area() != nil {
		t.Errorf("Area = %v, want nil after walking out", areaName(player.Area()))
	}
	if len(log) == 0 || log[len(log)-1] != "leave:unregistered" {
		t.Errorf("log = %v, want leave:unregistered last", log)
	}
}

// process() may move the actor (AreaManager.ts:69-70): the
// area is recorded BEFORE process runs, so a teleport is
// only detected on the NEXT tick.
func TestAreaManagerProcessTeleportDetectedNextTick(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	a := logArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)}, &log)
	b := logArea("B", []Boundary{NewBoundary(100, 110, 100, 110, 0)}, &log)
	mgr.Register(a)
	mgr.Register(b)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	if player.Area() != a {
		t.Fatalf("Area = %v, want A", areaName(player.Area()))
	}

	teleported := false
	a.ProcessHook = func(actor AreaActor) {
		if !teleported {
			teleported = true
			actor.(*fakePlayerActor).moveTo(Location{X: 105, Y: 105, Z: 0})
		}
	}
	log = nil
	mgr.Process(player)
	// The teleport happened during process, but the area
	// recorded this tick is still A.
	if player.Area() != a {
		t.Errorf("Area after teleport tick = %v, want A (detected next tick)", areaName(player.Area()))
	}

	log = nil
	mgr.Process(player)
	equalStrings(t, log, []string{"leave:A", "enter:B", "process:B"}, "log on detection tick")
	if player.Area() != b {
		t.Errorf("Area = %v, want B", areaName(player.Area()))
	}
}

// When process() changes the actor's recorded area itself
// (a nested AreaManager.process, as Pest Control does), the
// outer Process adopts the new area and records the hint at
// the POST-process position (AreaManager.ts:74-83).
func TestAreaManagerProcessNestedAreaChange(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	a := NewArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)})
	b := logArea("B", []Boundary{NewBoundary(100, 110, 100, 110, 0)}, &log)
	mgr.Register(a)
	mgr.Register(b)
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)

	changed := false
	a.ProcessHook = func(actor AreaActor) {
		if !changed {
			changed = true
			actor.(*fakePlayerActor).moveTo(Location{X: 105, Y: 105, Z: 0})
			actor.SetArea(b)
		}
	}
	mgr.Process(player)
	if player.Area() != b {
		t.Fatalf("Area = %v, want B after nested change", areaName(player.Area()))
	}
	// Stationary next tick: the hint records the post-process
	// position, so no leave/enter churn.
	log = nil
	mgr.Process(player)
	equalStrings(t, log, []string{"process:B"}, "log stationary after nested change")
}

// The multi-icon update (AreaManager.ts:86-94): sent on
// change, including the initial send from the unset
// sentinel; skipped when unchanged; never for NPCs or for
// players without the MultiIconActor extension.
func TestAreaManagerProcessMultiIcon(t *testing.T) {
	mgr := NewAreaManager()
	multi := NewArea("multi", []Boundary{NewBoundary(0, 10, 0, 10, 0)})
	multi.IsMultiHook = func() bool { return true }
	mgr.Register(multi)

	player := newFakePlayer(1, Location{X: 50, Y: 50, Z: 0})
	mgr.Process(player)
	// Initial send: sentinel -1 != 0, so 0 is sent.
	equalInts(t, player.multiIconSets, []int{0}, "initial icon sends")
	mgr.Process(player)
	equalInts(t, player.multiIconSets, []int{0}, "no resend when unchanged")

	player.moveTo(Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	equalInts(t, player.multiIconSets, []int{0, 1}, "icon sends on entering multi")
	mgr.Process(player)
	equalInts(t, player.multiIconSets, []int{0, 1}, "no resend while multi")

	player.moveTo(Location{X: 50, Y: 50, Z: 0})
	mgr.Process(player)
	// The leave tick does NOT clear the icon: InMulti reads
	// the actor's area, which is still the multi area until
	// setArea runs after the multi-icon block (the stale-area
	// subtlety in Process). The icon clears on the next tick.
	equalInts(t, player.multiIconSets, []int{0, 1}, "icon kept on the leave tick (stale area)")
	mgr.Process(player)
	equalInts(t, player.multiIconSets, []int{0, 1, 0}, "icon clears on the tick after leaving")
}

func TestAreaManagerProcessMultiIconSkippedForNPCAndBarePlayer(t *testing.T) {
	mgr := NewAreaManager()
	multi := NewArea("multi", []Boundary{NewBoundary(0, 10, 0, 10, 0)})
	multi.IsMultiHook = func() bool { return true }
	mgr.Register(multi)

	// Must not panic, and records nothing: NPCs have no icon.
	npc := newFakeNPC(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(npc)
	if npc.Area() != multi {
		t.Error("NPC not entered into area")
	}

	// A player without MultiIconActor: the assertion fails
	// silently, no panic.
	p := NewPlayer("bare", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(2)
	bare := &fakePlayerNoIcon{player: p}
	mgr.Process(bare)
	if bare.Area() != multi {
		t.Error("bare player not entered into area")
	}
}

// A stationary actor with a nil-area hint skips lookups
// (AreaManager.ts:51-56) — including after a new area is
// registered over its tile — until it moves or the hint is
// forgotten.
func TestAreaManagerNilHintAndForget(t *testing.T) {
	var log []string
	mgr := NewAreaManager()
	player := newFakePlayer(1, Location{X: 5, Y: 5, Z: 0})
	mgr.Process(player)
	if player.Area() != nil {
		t.Fatal("Area set with no areas registered")
	}

	a := logArea("A", []Boundary{NewBoundary(0, 10, 0, 10, 0)}, &log)
	mgr.Register(a)
	mgr.Process(player)
	if player.Area() != nil {
		t.Error("stationary actor entered a newly registered area (nil-hint fast path skipped?)")
	}

	mgr.Forget(player)
	mgr.Process(player)
	if player.Area() != a {
		t.Errorf("Area after Forget = %v, want A", areaName(player.Area()))
	}
	equalStrings(t, log, []string{"enter:A", "process:A"}, "log after Forget")
}

// The bucketed search index: a map-wide boundary goes on the
// fallback path, and the merge preserves registration order
// in both directions (AreaManager.ts:201, 293-318).
func TestAreaManagerSearchIndexFallbackMerge(t *testing.T) {
	huge := []Boundary{NewBoundary(0, 4095, 0, 4095, 0)} // 64x64 buckets > 1024
	small := []Boundary{NewBoundary(10, 20, 10, 20, 0)}
	overlap := Location{X: 15, Y: 15, Z: 0}

	mgr := NewAreaManager()
	hugeArea := NewArea("huge", huge)
	smallArea := NewArea("small", small)
	mgr.Register(hugeArea)
	mgr.Register(smallArea)
	if got := mgr.Get(overlap); got != hugeArea {
		t.Errorf("Get(overlap) = %v, want huge (registered first)", areaName(got))
	}
	if got := mgr.Get(Location{X: 3000, Y: 3000, Z: 0}); got != hugeArea {
		t.Errorf("Get(huge-only) = %v, want huge", areaName(got))
	}

	reversed := NewAreaManager()
	reversed.Register(smallArea)
	reversed.Register(hugeArea)
	if got := reversed.Get(overlap); got != smallArea {
		t.Errorf("Get(overlap) reversed = %v, want small", areaName(got))
	}
}

// One area holding both a huge (fallback) and a small
// (bucketed) boundary resolves through either path, and a
// height-less boundary alongside a normal one never matches
// but does not break the index.
func TestAreaManagerSearchIndexMixedBoundaries(t *testing.T) {
	mgr := NewAreaManager()
	mixed := NewArea("mixed", []Boundary{
		NewBoundary(0, 4095, 0, 4095, 0),       // fallback
		NewBoundary(5000, 5010, 5000, 5010, 0), // bucketed
		NewBoundary(6000, 6010, 6000, 6010),    // height-less: inert
	})
	mgr.Register(mixed)
	if got := mgr.Get(Location{X: 100, Y: 100, Z: 0}); got != mixed {
		t.Errorf("Get(huge) = %v, want mixed", areaName(got))
	}
	if got := mgr.Get(Location{X: 5005, Y: 5005, Z: 0}); got != mixed {
		t.Errorf("Get(small) = %v, want mixed", areaName(got))
	}
	if got := mgr.Get(Location{X: 6005, Y: 6005, Z: 0}); got != nil {
		t.Errorf("Get(height-less) = %v, want nil", areaName(got))
	}
	// Bucket edges: 64-tile buckets, coordinates 63/64.
	edge := NewAreaManager()
	edgeArea := NewArea("edge", []Boundary{NewBoundary(64, 127, 64, 127, 0)})
	edge.Register(edgeArea)
	if got := edge.Get(Location{X: 63, Y: 63, Z: 0}); got != nil {
		t.Errorf("Get(63,63) = %v, want nil", areaName(got))
	}
	if got := edge.Get(Location{X: 64, Y: 64, Z: 0}); got != edgeArea {
		t.Errorf("Get(64,64) = %v, want edge", areaName(got))
	}
	if got := edge.Get(Location{X: 127, Y: 127, Z: 0}); got != edgeArea {
		t.Errorf("Get(127,127) = %v, want edge", areaName(got))
	}
	if got := edge.Get(Location{X: 128, Y: 128, Z: 0}); got != nil {
		t.Errorf("Get(128,128) = %v, want nil", areaName(got))
	}
}

func TestAreaManagerRegisterNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Register(nil) did not panic")
		}
	}()
	NewAreaManager().Register(nil)
}

func TestAreaManagerAreasCopy(t *testing.T) {
	mgr := NewAreaManager()
	a := NewArea("A", nil)
	mgr.Register(a)
	got := mgr.Areas()
	if len(got) != 1 || got[0] != a {
		t.Fatalf("Areas() = %v, want [A]", got)
	}
	got[0] = nil
	if mgr.Areas()[0] != a {
		t.Error("Areas() mutation affected the registry")
	}
}

func equalInts(t *testing.T, got, want []int, what string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d] = %d, want %d", what, i, got[i], want[i])
		}
	}
}
