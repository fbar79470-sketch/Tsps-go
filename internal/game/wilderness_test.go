package game

import "testing"

// mapZoneSource is a ZoneBoundarySource backed by a tag map.
type mapZoneSource map[string][]Boundary

func (s mapZoneSource) ZoneBoundaries(tag string) []Boundary { return s[tag] }

// fakeRoof is a RoofChecker answering a fixed value.
type fakeRoof bool

func (f fakeRoof) IsUnderRoof(x, y, plane int) bool { return bool(f) }

func pvpWilderness() *Wilderness {
	return NewWilderness(mapZoneSource{
		"pvp": {NewBoundary(3000, 3300, 3500, 3700, 0)},
	}, nil)
}

func TestWildernessIsPvpArea(t *testing.T) {
	w := pvpWilderness()
	if !w.IsPvpArea(Location{X: 3000, Y: 3500, Z: 0}) {
		t.Error("IsPvpArea(min corner) = false, want true")
	}
	if !w.IsPvpArea(Location{X: 3300, Y: 3700, Z: 0}) {
		t.Error("IsPvpArea(max corner) = false, want true")
	}
	if w.IsPvpArea(Location{X: 2999, Y: 3600, Z: 0}) {
		t.Error("IsPvpArea(outside) = true, want false")
	}
	if w.IsPvpArea(Location{X: 3200, Y: 3600, Z: 1}) {
		t.Error("IsPvpArea(wrong plane) = true, want false")
	}
	if NewWilderness(nil, nil).IsPvpArea(Location{X: 3200, Y: 3600, Z: 0}) {
		t.Error("IsPvpArea(nil source) = true, want false")
	}
}

// isInLocation = PvP AND NOT safe AND NOT safe-building
// (Wilderness.ts:35-41). Each carve-out is tested
// independently.
func TestWildernessIsInLocation(t *testing.T) {
	inside := Location{X: 3200, Y: 3600, Z: 0}
	source := mapZoneSource{
		"pvp": {NewBoundary(3000, 3300, 3500, 3700, 0)},
	}
	// PvP only: in.
	if !NewWilderness(source, nil).IsInLocation(inside) {
		t.Error("PvP-only tile not in location")
	}
	// Safe zone carve-out: out.
	safe := mapZoneSource{
		"pvp":  {NewBoundary(3000, 3300, 3500, 3700, 0)},
		"safe": {NewBoundary(3100, 3250, 3550, 3650, 0)},
	}
	if NewWilderness(safe, nil).IsInLocation(inside) {
		t.Error("safe-zone tile in location")
	}
	// Safe-building tag + roof: out.
	building := mapZoneSource{
		"pvp":                {NewBoundary(3000, 3300, 3500, 3700, 0)},
		"all-buildings-safe": {NewBoundary(3100, 3250, 3550, 3650, 0)},
	}
	if NewWilderness(building, fakeRoof(true)).IsInLocation(inside) {
		t.Error("safe-building tile under roof in location")
	}
	// Safe-building tag but NO roof: still in (both halves
	// required, Wilderness.ts:21-25).
	if !NewWilderness(building, fakeRoof(false)).IsInLocation(inside) {
		t.Error("safe-building tile without roof not in location")
	}
	// Safe-building tag but no roof checker: the roof half
	// is false, so still in.
	if !NewWilderness(building, nil).IsInLocation(inside) {
		t.Error("safe-building tile with no roof checker not in location")
	}
	// Not PvP at all: out, even without carve-outs.
	if NewWilderness(source, nil).IsInLocation(Location{X: 100, Y: 100, Z: 0}) {
		t.Error("non-PvP tile in location")
	}
}

func TestWildernessIsInSafeBuilding(t *testing.T) {
	loc := Location{X: 3200, Y: 3600, Z: 0}
	source := mapZoneSource{
		"all-buildings-safe": {NewBoundary(3000, 3300, 3500, 3700, 0)},
	}
	if !NewWilderness(source, fakeRoof(true)).IsInSafeBuilding(loc) {
		t.Error("tag + roof: not a safe building")
	}
	if NewWilderness(source, fakeRoof(false)).IsInSafeBuilding(loc) {
		t.Error("tag without roof: safe building")
	}
	if NewWilderness(source, nil).IsInSafeBuilding(loc) {
		t.Error("tag without roof checker: safe building")
	}
	if NewWilderness(mapZoneSource{}, fakeRoof(true)).IsInSafeBuilding(loc) {
		t.Error("roof without tag: safe building")
	}
}

type locatedFixture struct{ loc Location }

func (f locatedFixture) Location() Location { return f.loc }

func TestWildernessIsIn(t *testing.T) {
	w := pvpWilderness()
	if !w.IsIn(locatedFixture{Location{X: 3200, Y: 3600, Z: 0}}) {
		t.Error("IsIn(PvP) = false, want true")
	}
	if w.IsIn(locatedFixture{Location{X: 100, Y: 100, Z: 0}}) {
		t.Error("IsIn(non-PvP) = true, want false")
	}
	if w.IsIn(nil) {
		t.Error("IsIn(nil) = true, want false")
	}
}

// LevelAt: the two levelled strips, floor((y-minY)/8)+1
// (Wilderness.ts:53-66). Strip 1: y 3520..4351; strip 2
// (caves): y 9920..10879.
func TestWildernessLevelAtStrips(t *testing.T) {
	w := NewWilderness(nil, nil)
	cases := []struct {
		x, y, want int
	}{
		{2944, 3520, 1},    // strip 1 min corner: level 1
		{3391, 3520, 1},    // strip 1 max x, level 1
		{3200, 3527, 1},    // last tile of level 1
		{3200, 3528, 2},    // first tile of level 2
		{3200, 4351, 104},  // (4351-3520)/8+1
		{2943, 3600, 0},    // x below strip
		{3392, 3600, 0},    // x above strip 1 (below strip 2's max, wrong y)
		{3200, 3519, 0},    // y below strip
		{3200, 4352, 0},    // y above strip 1
		{2944, 9920, 1},    // strip 2 min corner
		{3455, 9920, 1},    // strip 2 max x (wider than strip 1)
		{3400, 10000, 11},  // (10000-9920)/8+1; x only in strip 2
		{3200, 10879, 120}, // (10879-9920)/8+1
		{3200, 10880, 0},   // y above strip 2
		{100, 100, 0},      // anywhere else: unlevelled
	}
	for _, c := range cases {
		if got := w.LevelAt(c.x, c.y); got != c.want {
			t.Errorf("LevelAt(%d, %d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

// Level sources are consulted first, in registration order;
// the first answer wins (including 0); re-registering a name
// replaces its source without moving it.
func TestWildernessLevelAtSources(t *testing.T) {
	w := NewWilderness(nil, nil)
	// A source that passes everywhere: the strip still wins.
	w.SetLevelSource("pass", func(x, y int) (int, bool) { return 0, false })
	if got := w.LevelAt(3200, 3520); got != 1 {
		t.Errorf("LevelAt with passing source = %d, want 1", got)
	}
	// A source answering 0 wins over the strip (TS: level
	// != null, Wilderness.ts:56).
	w.SetLevelSource("zero", func(x, y int) (int, bool) { return 0, true })
	if got := w.LevelAt(3200, 3520); got != 0 {
		t.Errorf("LevelAt with zero source = %d, want 0", got)
	}
	// Replace "zero" by name: the strip shows through again,
	// and the replacement keeps its position (after "pass",
	// which still passes).
	w.SetLevelSource("zero", func(x, y int) (int, bool) { return 0, false })
	if got := w.LevelAt(3200, 3520); got != 1 {
		t.Errorf("LevelAt after replace = %d, want 1", got)
	}
	// Order: the first answering source wins.
	w2 := NewWilderness(nil, nil)
	w2.SetLevelSource("first", func(x, y int) (int, bool) { return 5, true })
	w2.SetLevelSource("second", func(x, y int) (int, bool) { return 9, true })
	if got := w2.LevelAt(100, 100); got != 5 {
		t.Errorf("LevelAt = %d, want 5 (first source)", got)
	}
	// Replacing "first" keeps it first.
	w2.SetLevelSource("first", func(x, y int) (int, bool) { return 7, true })
	if got := w2.LevelAt(100, 100); got != 7 {
		t.Errorf("LevelAt after replace = %d, want 7", got)
	}
}

func TestWildernessIsMulti(t *testing.T) {
	w := NewWilderness(mapZoneSource{
		"multi-combat": {NewBoundary(3000, 3300, 3500, 3700, 1)},
	}, nil)
	if w.IsMulti(3200, 3600, 0) {
		t.Error("IsMulti(plane 0) = true, want false (zone is plane 1)")
	}
	if !w.IsMulti(3200, 3600, 1) {
		t.Error("IsMulti(plane 1) = false, want true")
	}
	if w.IsMulti(100, 100, 1) {
		t.Error("IsMulti(outside) = true, want false")
	}
	if NewWilderness(nil, nil).IsMulti(3200, 3600, 0) {
		t.Error("IsMulti(nil source) = true, want false")
	}
}

// IntersectsArea: rectangle overlap with any PvP boundary on
// the same plane; a height-less boundary counts as plane 0
// (Wilderness.ts:74-95).
func TestWildernessIntersectsArea(t *testing.T) {
	w := NewWilderness(mapZoneSource{
		"pvp": {
			NewBoundary(3000, 3300, 3500, 3700, 0),
			NewBoundary(4000, 4100, 4000, 4100), // height-less: plane 0
		},
	}, nil)
	if !w.IntersectsArea(3200, 3400, 3600, 3800, 0) {
		t.Error("overlapping rectangle does not intersect")
	}
	if !w.IntersectsArea(3300, 3400, 3700, 3800, 0) {
		t.Error("edge-touching rectangle does not intersect")
	}
	if w.IntersectsArea(3301, 3400, 3600, 3800, 0) {
		t.Error("disjoint rectangle intersects")
	}
	if w.IntersectsArea(3200, 3400, 3600, 3800, 1) {
		t.Error("rectangle on another plane intersects")
	}
	// The height-less boundary counts as plane 0.
	if !w.IntersectsArea(4050, 4060, 4050, 4060, 0) {
		t.Error("height-less boundary does not intersect on plane 0")
	}
	if w.IntersectsArea(4050, 4060, 4050, 4060, 1) {
		t.Error("height-less boundary intersects on plane 1")
	}
	if NewWilderness(nil, nil).IntersectsArea(0, 100, 0, 100, 0) {
		t.Error("nil source intersects")
	}
}

func TestRegionRoofChecker(t *testing.T) {
	if RegionRoofChecker(nil).IsUnderRoof(3200, 3600, 0) {
		t.Error("nil RegionManager under roof")
	}
	// An unloaded region (no source, nothing indexed) is
	// not under a roof, and the lookup must not panic:
	// ensureRegion returns nil for an unindexed tile.
	rm := NewRegionManager(nil, nil, false)
	if RegionRoofChecker(rm).IsUnderRoof(3200, 3600, 0) {
		t.Error("unloaded region under roof")
	}
	if RegionRoofChecker(rm).IsUnderRoof(3200, 3600, 9) {
		t.Error("out-of-range plane under roof")
	}
}
