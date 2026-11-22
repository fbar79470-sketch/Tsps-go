package doors

import (
	"os"
	"path/filepath"
	"testing"

	"tsps-go/internal/cache"
)

// fixtureDefs is a small fake CatalogDefs for unit tests.
type fixtureDefs struct {
	defs   map[int]*cache.LocType
	names  map[int]string
	byName map[string]int
	count  int
}

func newFixtureDefs() *fixtureDefs {
	return &fixtureDefs{
		defs:   map[int]*cache.LocType{},
		names:  map[int]string{},
		byName: map[string]int{},
	}
}

func (f *fixtureDefs) GetObject(id int) *cache.LocType { return f.defs[id] }
func (f *fixtureDefs) ObjectCount() int                { return f.count }
func (f *fixtureDefs) LocName(id int) string           { return f.names[id] }
func (f *fixtureDefs) LocID(name string) (int, bool) {
	id, ok := f.byName[name]
	return id, ok
}

// loc adds a loc definition to the fixture. gameval may be "".
func (f *fixtureDefs) loc(id int, name string, actions []string, models [][]int, gameval string) {
	f.defs[id] = &cache.LocType{Name: name, Actions: actions, Models: models}
	if gameval != "" {
		f.names[id] = gameval
		if _, ok := f.byName[gameval]; !ok {
			f.byName[gameval] = id
		}
	}
	if id+1 > f.count {
		f.count = id + 1
	}
}

func openOnly() []string  { return []string{"Open", "", "", "", ""} }
func closeOnly() []string { return []string{"Close", "", "", "", ""} }
func noActions() []string { return []string{"", "", "", "", ""} }

func mustCatalog(t *testing.T, defs CatalogDefs) *Catalog {
	t.Helper()
	c, err := BuildCatalog(defs)
	if err != nil {
		t.Fatalf("BuildCatalog: %v", err)
	}
	return c
}

func TestClassicPair(t *testing.T) {
	f := newFixtureDefs()
	f.loc(100, "Door", openOnly(), [][]int{{1}}, "")
	f.loc(101, "Door", closeOnly(), [][]int{{1}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(100); !ok || open != 101 {
		t.Errorf("OpenID(100) = %d, %v; want 101, true", open, ok)
	}
	if closed, ok := c.ClosedID(101); !ok || closed != 100 {
		t.Errorf("ClosedID(101) = %d, %v; want 100, true", closed, ok)
	}
}

func TestNamedOpenLeaf(t *testing.T) {
	f := newFixtureDefs()
	// The id+1 neighbour is an unrelated door, so only the gameval
	// name can pair this one.
	f.loc(200, "Door", openOnly(), [][]int{{5}}, "mydoor_closed")
	f.loc(201, "Large door", openOnly(), [][]int{{6}}, "")
	f.loc(202, "Door", closeOnly(), [][]int{{5}}, "mydoor_open")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(200); !ok || open != 202 {
		t.Errorf("OpenID(200) = %d, %v; want 202, true", open, ok)
	}
	if _, ok := c.OpenID(201); ok {
		t.Errorf("OpenID(201) paired unexpectedly")
	}
}

func TestNamedOpenLeafRequiresCloseOnlyAndSameName(t *testing.T) {
	f := newFixtureDefs()
	// Same models and a matching gameval name, but the candidate
	// also offers Open: not a valid open leaf.
	f.loc(210, "Door", openOnly(), [][]int{{5}}, "otherdoor_closed")
	f.loc(211, "Gate", []string{"Open", "Close", "", "", ""}, [][]int{{5}}, "otherdoor_open")
	c := mustCatalog(t, f)
	if _, ok := c.OpenID(210); ok {
		t.Errorf("OpenID(210) paired to a leaf that also offers Open")
	}
}

func TestSkippedActionlessPair(t *testing.T) {
	f := newFixtureDefs()
	f.loc(300, "Door", openOnly(), [][]int{{7}}, "")
	f.loc(301, "Door", noActions(), [][]int{{7}}, "")
	f.loc(302, "Door", closeOnly(), [][]int{{7}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(300); !ok || open != 302 {
		t.Errorf("OpenID(300) = %d, %v; want 302, true", open, ok)
	}
}

func TestInterleavedPair(t *testing.T) {
	f := newFixtureDefs()
	f.loc(400, "Door", openOnly(), [][]int{{1}}, "")
	f.loc(401, "Door", openOnly(), [][]int{{2}}, "")
	f.loc(402, "Door", closeOnly(), [][]int{{2}}, "")
	f.loc(403, "Door", closeOnly(), [][]int{{1}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(400); !ok || open != 403 {
		t.Errorf("OpenID(400) = %d, %v; want 403, true", open, ok)
	}
	// The inner door pairs classically with its own open leaf.
	if open, ok := c.OpenID(401); !ok || open != 402 {
		t.Errorf("OpenID(401) = %d, %v; want 402, true", open, ok)
	}
}

func TestSameModelDoubleDoor(t *testing.T) {
	f := newFixtureDefs()
	// Two consecutive same-model Open-only leaves with mirrored
	// gameval names and gameval-named open locs. The open leaves
	// carry a different display name, as in the cache, so the
	// second leaf is not paired classically to its open loc.
	f.loc(500, "Large door", openOnly(), [][]int{{9}}, "fai_test_gatel")
	f.loc(501, "Large door", openOnly(), [][]int{{9}}, "fai_test_gater")
	f.loc(502, "Gate", closeOnly(), [][]int{{9}}, "fai_test_gatel_open")
	f.loc(503, "Gate", closeOnly(), [][]int{{9}}, "fai_test_gater_open")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(500); !ok || open != 500 {
		t.Errorf("OpenID(500) = %d, %v; want 500, true (self-opening leaf)", open, ok)
	}
	if open, ok := c.OpenID(501); !ok || open != 501 {
		t.Errorf("OpenID(501) = %d, %v; want 501, true (self-opening leaf)", open, ok)
	}
	family, ok := c.DoubleDoorFamily(500)
	if !ok {
		t.Fatalf("DoubleDoorFamily(500) missing")
	}
	want := map[int]bool{500: true, 501: true, 502: true, 503: true}
	if len(family) != 4 {
		t.Errorf("DoubleDoorFamily(500) = %v; want the 4 leaves", family)
	}
	for _, id := range family {
		if !want[id] {
			t.Errorf("DoubleDoorFamily(500) contains unexpected id %d", id)
		}
	}
	if open, ok := c.SpecialDoubleDoorOpen(500); !ok || open != 502 {
		t.Errorf("SpecialDoubleDoorOpen(500) = %d, %v; want 502, true", open, ok)
	}
	if open, ok := c.SpecialDoubleDoorOpen(501); !ok || open != 503 {
		t.Errorf("SpecialDoubleDoorOpen(501) = %d, %v; want 503, true", open, ok)
	}
}

func TestSelfOpening(t *testing.T) {
	f := newFixtureDefs()
	// O.DOOR_67 (2184): Open-only, no paired open loc; its id+1 is
	// an unrelated same-model Close loc belonging to another door.
	f.loc(2184, "Door", openOnly(), [][]int{{11}}, "")
	f.loc(2185, "Door", closeOnly(), [][]int{{11}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(2184); !ok || open != 2184 {
		t.Errorf("OpenID(2184) = %d, %v; want 2184, true", open, ok)
	}
	if !c.IsSelfOpening(2184) {
		t.Errorf("IsSelfOpening(2184) = false; want true")
	}
	if _, ok := c.ClosedID(2185); ok {
		t.Errorf("ClosedID(2185) resolved unexpectedly: 2185 belongs to another door")
	}
}

func TestSingleDoorOverride(t *testing.T) {
	f := newFixtureDefs()
	// Lumbridge kitchen door: O.DOOR_302 (12348) -> O.DOOR_31
	// (1544). Its id+1 is a Large door, so the scan never pairs it.
	f.loc(12348, "Door", openOnly(), [][]int{{13}}, "")
	f.loc(12349, "Large door", openOnly(), [][]int{{14}}, "")
	f.loc(1543, "Door", openOnly(), [][]int{{15}}, "")
	f.loc(1544, "Door", closeOnly(), [][]int{{15}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(12348); !ok || open != 1544 {
		t.Errorf("OpenID(12348) = %d, %v; want 1544, true", open, ok)
	}
	// Shared leaf 1544 keeps the door the scan already paired it
	// with (first wins on the reverse map).
	if closed, ok := c.ClosedID(1544); !ok || closed != 1543 {
		t.Errorf("ClosedID(1544) = %d, %v; want 1543, true (first wins)", closed, ok)
	}
}

func TestSingleDoorOverrideSharedLeaf(t *testing.T) {
	f := newFixtureDefs()
	// Games Room / Runeversi doors share O.DOOR_132 (4639).
	for _, id := range []int{4636, 4637, 11196, 11197} {
		f.loc(id, "Door", openOnly(), [][]int{{17}}, "")
	}
	c := mustCatalog(t, f)
	for _, id := range []int{4636, 4637, 11196, 11197} {
		if open, ok := c.OpenID(id); !ok || open != 4639 {
			t.Errorf("OpenID(%d) = %d, %v; want 4639, true", id, open, ok)
		}
	}
}

func TestWoodenGateSkip(t *testing.T) {
	f := newFixtureDefs()
	// Lumbridge wooden gate: O.GATE_116/O.GATE_117 (12986/12987).
	f.loc(12986, "Gate", openOnly(), [][]int{{19}}, "")
	f.loc(12987, "Gate", openOnly(), [][]int{{20}}, "")
	c := mustCatalog(t, f)
	if _, ok := c.OpenID(12986); ok {
		t.Errorf("OpenID(12986) paired unexpectedly: wooden gates skip generic pairing")
	}
	if _, ok := c.OpenID(12987); ok {
		t.Errorf("OpenID(12987) paired unexpectedly: wooden gates skip generic pairing")
	}
	gate, ok := c.WoodenGate(12986)
	if !ok {
		t.Fatalf("WoodenGate(12986) missing")
	}
	if gate.ClosedHinge != 12986 || gate.ClosedExtension != 12987 ||
		gate.OpenHinge != 12988 || gate.OpenExtension != 12989 {
		t.Errorf("WoodenGate(12986) = %+v; want hinge/extension 12986/12987 -> 12988/12989", gate)
	}
	// The opened panels resolve to the same gate.
	if _, ok := c.WoodenGate(12988); !ok {
		t.Errorf("WoodenGate(12988) missing: opened panels belong to the gate too")
	}
}

func TestReverseOwnTile(t *testing.T) {
	f := newFixtureDefs()
	f.loc(600, "Door", openOnly(), [][]int{{21}}, "test_door_reverse")
	f.loc(601, "Door", closeOnly(), [][]int{{21}}, "")
	c := mustCatalog(t, f)
	if open, ok := c.OpenID(600); !ok || open != 601 {
		t.Errorf("OpenID(600) = %d, %v; want 601, true", open, ok)
	}
	if !c.IsOwnTile(600) {
		t.Errorf("IsOwnTile(600) = false; want true for a *_reverse door")
	}
}

func TestMirroredLeafDoubleDoor(t *testing.T) {
	f := newFixtureDefs()
	// w_ardougnedoubledoorl 2546 -> 2547, w_ardougnedoubledoorr
	// 2548 -> 2549: paired as two single doors, then detected as
	// one double door by the mirrored leaf names.
	f.loc(2546, "Door", openOnly(), [][]int{{23}}, "w_ardougnedoubledoorl")
	f.loc(2547, "Door", closeOnly(), [][]int{{23}}, "")
	f.loc(2548, "Door", openOnly(), [][]int{{24}}, "w_ardougnedoubledoorr")
	f.loc(2549, "Door", closeOnly(), [][]int{{24}}, "")
	c := mustCatalog(t, f)
	family, ok := c.DoubleDoorFamily(2546)
	if !ok {
		t.Fatalf("DoubleDoorFamily(2546) missing")
	}
	want := map[int]bool{2546: true, 2548: true, 2547: true, 2549: true}
	if len(family) != 4 {
		t.Errorf("DoubleDoorFamily(2546) = %v; want the 4 leaves", family)
	}
	for _, id := range family {
		if !want[id] {
			t.Errorf("DoubleDoorFamily(2546) contains unexpected id %d", id)
		}
	}
	if open, ok := c.SpecialDoubleDoorOpen(2548); !ok || open != 2549 {
		t.Errorf("SpecialDoubleDoorOpen(2548) = %d, %v; want 2549, true", open, ok)
	}
}

func TestMirroredLeafRequiresMirroredNames(t *testing.T) {
	f := newFixtureDefs()
	// Same layout, but the names differ by more than a leaf side.
	f.loc(2546, "Door", openOnly(), [][]int{{23}}, "some_door_alpha")
	f.loc(2547, "Door", closeOnly(), [][]int{{23}}, "")
	f.loc(2548, "Door", openOnly(), [][]int{{24}}, "some_door_beta")
	f.loc(2549, "Door", closeOnly(), [][]int{{24}}, "")
	c := mustCatalog(t, f)
	if _, ok := c.DoubleDoorFamily(2546); ok {
		t.Errorf("DoubleDoorFamily(2546) present for non-mirrored names")
	}
	if open, ok := c.OpenID(2546); !ok || open != 2547 {
		t.Errorf("OpenID(2546) = %d, %v; want 2547, true (still a single door)", open, ok)
	}
}

func TestTableCounts(t *testing.T) {
	c := mustCatalog(t, newFixtureDefs())
	if got := len(DoorNames); got != 36 {
		t.Errorf("DOOR_NAMES = %d; want 36", got)
	}
	if got := len(selfOpeningDoorIDs); got != 36 {
		t.Errorf("SELF_OPENING_DOOR_IDS = %d unique; want 36", got)
	}
	if got := len(singleDoorOpenIDs); got != 21 {
		t.Errorf("SINGLE_DOOR_OPEN_IDS = %d; want 21", got)
	}
	if got := len(inPlaceDoorIDs); got != 3 {
		t.Errorf("IN_PLACE_DOOR_IDS = %d; want 3", got)
	}
	if got := len(ownTileDoorIDs); got != 7 {
		t.Errorf("OWN_TILE_DOOR_IDS = %d; want 7", got)
	}
	if got := len(compositeDoorOpenings); got != 3 {
		t.Errorf("COMPOSITE_DOOR_OPENINGS = %d; want 3", got)
	}
	if got := len(dorgeshCityDoorIDs); got != 2 {
		t.Errorf("DORGESH_CITY_DOOR_IDS = %d; want 2", got)
	}
	if got := len(woodenGates); got != 14 {
		t.Errorf("WOODEN_GATES = %d; want 14", got)
	}
	if got := len(doubleDoorIDFamilies); got != 26 {
		t.Errorf("DOUBLE_DOOR_ID_FAMILIES = %d; want 26", got)
	}
	uniquePairs := map[[2]int]bool{}
	for _, pair := range specialDoubleDoorPairs {
		uniquePairs[pair] = true
	}
	if got := len(specialDoubleDoorPairs); got != 36 {
		t.Errorf("SPECIAL_DOUBLE_DOOR_PAIRS = %d entries; want 36", got)
	}
	if got := len(uniquePairs); got != 35 {
		t.Errorf("SPECIAL_DOUBLE_DOOR_PAIRS = %d unique; want 35", got)
	}
	if got := len(specialDoubleDoorOpenIDsByClosedID); got != 38 {
		t.Errorf("SPECIAL_DOUBLE_DOOR_OPEN_IDS_BY_CLOSED_ID = %d; want 38", got)
	}
	if got := len(passThroughGates); got != 31 {
		t.Errorf("PASS_THROUGH_GATES = %d; want 31 (29 TS array elements, Mining spread expands to 3)", got)
	}
	if got := len(passThroughFamilies); got != 15 {
		t.Errorf("PASS_THROUGH_FAMILIES = %d; want 15", got)
	}
	// Spot-check the table accessors on the empty catalog.
	if !c.IsInPlace(1533) {
		t.Errorf("IsInPlace(1533) = false; want true (O.CURTAIN_3)")
	}
	if !c.IsOwnTile(21403) {
		t.Errorf("IsOwnTile(21403) = false; want true (O.GATE_155)")
	}
	if leaves, ok := c.CompositeOpening(190); !ok || len(leaves) != 2 {
		t.Errorf("CompositeOpening(190) = %v, %v; want 2 leaves", leaves, ok)
	}
	if !c.IsDorgeshDoor(6919) || !c.IsDorgeshDoor(6920) {
		t.Errorf("IsDorgeshDoor(6919/6920) = false; want true")
	}
	if _, ok := c.PassThroughFamily(2002); !ok {
		t.Errorf("PassThroughFamily(2002) missing (O.DOOR_48)")
	}
	if fam, ok := c.PassThroughFamily(2596); !ok || fam.HasOpenLeaf {
		t.Errorf("PassThroughFamily(2596) = %+v, %v; want a family with no explicit open leaf (O.RED_DOOR)", fam, ok)
	}
	if fam, ok := c.PassThroughFamily(37321); !ok || !fam.HasOpenLeaf || fam.OpenLeaf != 37322 || fam.OpenSoundID != 81 {
		t.Errorf("PassThroughFamily(37321) = %+v, %v; want open 37322, sound 81", fam, ok)
	}
	// Harold's door: the pass-through gate at its exact tile.
	gate, ok := c.PassThroughGate(3747, 2906, 3543, 1)
	if !ok {
		t.Fatalf("PassThroughGate(3747, 2906, 3543, 1) missing")
	}
	if len(gate.Closed) != 1 || len(gate.Opened) != 2 || !gate.WalkOver ||
		!gate.HasCrossY || gate.CrossY != 3543 || gate.CloseTicks != 2 || gate.OpenSoundID != 81 {
		t.Errorf("Harold pass-through gate = %+v; want 1 closed / 2 opened leaves, crossY 3543, walkOver, closeTicks 2, sound 81", gate)
	}
	// West Ardougne wall gate: thick, carried, 5-tick close.
	wall, ok := c.PassThroughGate(8738, 2558, 3299, 0)
	if !ok {
		t.Fatalf("PassThroughGate(8738, 2558, 3299, 0) missing")
	}
	if !wall.HasCrossBetween || wall.CrossBetween != [2]int{2556, 2559} ||
		len(wall.CrossRows) != 2 || wall.CloseTicks != 5 || wall.WalkOver || wall.Carry {
		t.Errorf("West Ardougne wall gate = %+v; want crossBetween [2556,2559], 2 cross rows, closeTicks 5, no walkOver/carry", wall)
	}
	// Ranging Guild entrance: diagonal crossing.
	ranging, ok := c.PassThroughGate(11665, 2658, 3438, 0)
	if !ok {
		t.Fatalf("PassThroughGate(11665, 2658, 3438, 0) missing")
	}
	if !ranging.HasCrossDiagonal || ranging.CrossDiagonalOutside != [2]int{2657, 3439} ||
		ranging.CrossDiagonalInside != [2]int{2659, 3437} || !ranging.CrossDiagonalEastward ||
		ranging.OpenSoundID != 67 {
		t.Errorf("Ranging Guild gate = %+v; want diagonal outside [2657,3439], inside [2659,3437], eastward, sound 67", ranging)
	}
	// Lumbridge double door family: 1521/1524 close into 1522/1525.
	if fam, ok := c.DoubleDoorFamily(1521); !ok || len(fam) != 4 {
		t.Errorf("DoubleDoorFamily(1521) = %v, %v; want the 4-leaf family", fam, ok)
	}
	// 1524 -> 1525 is a classic id+1 pair, not a special-open entry;
	// the special map covers leaves whose open id is not closedId+1.
	if open, ok := c.SpecialDoubleDoorOpen(30387); !ok || open != 1522 {
		t.Errorf("SpecialDoubleDoorOpen(30387) = %d, %v; want 1522, true", open, ok)
	}
}

func TestBuildCatalogNilDefs(t *testing.T) {
	if _, err := BuildCatalog(nil); err == nil {
		t.Errorf("BuildCatalog(nil) = nil error; want an error")
	}
}

// cacheAdapter adapts the real cache definitions to CatalogDefs.
type cacheAdapter struct {
	defs   *cache.CacheDefinitions
	names  map[int]string
	byName map[string]int
}

func (a *cacheAdapter) GetObject(id int) *cache.LocType { return a.defs.GetObject(id) }
func (a *cacheAdapter) ObjectCount() int                { return a.defs.GetBaseCounts().Objects }
func (a *cacheAdapter) LocName(id int) string           { return a.names[id] }
func (a *cacheAdapter) LocID(name string) (int, bool) {
	id, ok := a.byName[name]
	return id, ok
}

// TestRealCacheCatalog runs the scan against the real rev-242 cache
// and pins the final counts. The plan §1.4 baseline (873 door-named
// Open, 575 paired by the simplified probe rules, 408 unpaired)
// changes once the family tables land; these are the real numbers.
func TestRealCacheCatalog(t *testing.T) {
	seed := "/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03"
	if _, err := os.Stat(seed); err != nil {
		t.Skipf("rev-242 seed cache not present: %v", err)
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "target.txt"), []byte("osrs-242_2026-11-03\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cachesDir := filepath.Join(root, "caches")
	if err := os.MkdirAll(cachesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(seed, filepath.Join(cachesDir, "osrs-242_2026-11-03")); err != nil {
		t.Fatal(err)
	}
	active, err := cache.EnsureCache(root)
	if err != nil {
		t.Fatalf("EnsureCache: %v", err)
	}
	store, err := cache.OpenStore(active.Directory)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	defs, err := cache.NewCacheDefinitionsFromStore(cache.CacheInfoForActive(active), store)
	if err != nil {
		t.Fatalf("NewCacheDefinitionsFromStore: %v", err)
	}
	names, err := defs.GetLocNames()
	if err != nil {
		t.Fatalf("GetLocNames: %v", err)
	}
	adapter := &cacheAdapter{defs: defs, names: names, byName: map[string]int{}}
	for id, name := range names {
		if _, ok := adapter.byName[name]; !ok {
			adapter.byName[name] = id
		}
	}
	c := mustCatalog(t, adapter)

	total := adapter.ObjectCount()
	doorNamedOpen := 0
	for id := 0; id < total; id++ {
		def := adapter.GetObject(id)
		if def == nil || !DoorNames[def.Name] || !hasAction(def.Actions, "open") {
			continue
		}
		doorNamedOpen++
	}
	t.Logf("total object defs: %d", total)
	t.Logf("door-named with Open: %d", doorNamedOpen)
	t.Logf("closed->open pairs: %d", len(c.ClosedToOpen))
	t.Logf("open->closed reverse: %d", len(c.OpenToClosed))
	selfOpening := 0
	for id, open := range c.ClosedToOpen {
		if id == open {
			selfOpening++
		}
	}
	t.Logf("self-opening pairs (open id == closed id): %d", selfOpening)
	t.Logf("unpaired door-named Open: %d", doorNamedOpen-len(c.ClosedToOpen))
	if total != 63334 {
		t.Errorf("total object defs = %d; want 63334", total)
	}
	if doorNamedOpen != 873 {
		t.Errorf("door-named with Open = %d; want 873", doorNamedOpen)
	}
	if got := len(c.ClosedToOpen); got != 556 {
		t.Errorf("closed->open pairs = %d; want 556", got)
	}
	if got := len(c.OpenToClosed); got != 259 {
		t.Errorf("open->closed reverse = %d; want 259", got)
	}
	if selfOpening != 280 {
		t.Errorf("self-opening pairs = %d; want 280 (36 table + 244 same-model double leaves)", selfOpening)
	}
	if doorNamedOpen-len(c.ClosedToOpen) != 317 {
		t.Errorf("unpaired door-named Open = %d; want 317", doorNamedOpen-len(c.ClosedToOpen))
	}
	// Spot-check live Lumbridge / Varrock / Falador doors from the
	// plan §1.4 probe.
	checks := []struct {
		closed, open int
	}{
		{1535, 1536}, // Lumbridge single door
		{1543, 1544}, // Lumbridge Castle door
		{1540, 1541},
		{12348, 1544},  // kitchen door (single-door override)
		{24057, 24058}, // Falador
		{24060, 24061},
		{24062, 24063},
		{24064, 24065},
		{11775, 11774}, // Varrock inverted pair
		{11780, 11778}, // Varrock (Katrine's hideout area)
		{46863, 46864}, // Ardougne poshdoor_reverse
		{56376, 56377}, // Lumbridge
		{1521, 1522},   // Lumbridge double door, left leaf
		{1524, 1525},   // Lumbridge double door, right leaf
	}
	for _, check := range checks {
		if open, ok := c.OpenID(check.closed); !ok || open != check.open {
			t.Errorf("OpenID(%d) = %d, %v; want %d, true", check.closed, open, ok, check.open)
		}
	}
	// Lumbridge double door 1521/1524 opens into 1522/1525.
	if fam, ok := c.DoubleDoorFamily(1521); !ok {
		t.Errorf("DoubleDoorFamily(1521) missing on the real cache")
	} else {
		want := map[int]bool{1521: true, 1524: true, 1522: true, 1525: true}
		for _, id := range fam {
			if !want[id] {
				t.Errorf("DoubleDoorFamily(1521) contains unexpected id %d: %v", id, fam)
			}
		}
		if len(fam) != 4 {
			t.Errorf("DoubleDoorFamily(1521) = %v; want 4 ids", fam)
		}
	}
	// Self-opening: O.DOOR_67 (2184) and the Ranging Guild door.
	if open, ok := c.OpenID(2184); !ok || open != 2184 {
		t.Errorf("OpenID(2184) = %d, %v; want 2184, true", open, ok)
	}
	if open, ok := c.OpenID(11665); !ok || open != 11665 {
		t.Errorf("OpenID(11665) = %d, %v; want 11665, true (Guild door)", open, ok)
	}
	// Castle door 24059 has no ClosedToOpen entry: it is a family
	// leaf whose open leaf comes from the special map.
	if _, ok := c.OpenID(24059); ok {
		t.Errorf("OpenID(24059) paired unexpectedly; its open leaf is the special-map 24065")
	}
	if open, ok := c.SpecialDoubleDoorOpen(24059); !ok || open != 24065 {
		t.Errorf("SpecialDoubleDoorOpen(24059) = %d, %v; want 24065, true", open, ok)
	}
	// Wooden gate: Lumbridge 12986/12987.
	if _, ok := c.OpenID(12986); ok {
		t.Errorf("OpenID(12986) paired on the real cache; wooden gates skip generic pairing")
	}
	if gate, ok := c.WoodenGate(12986); !ok || gate.OpenHinge != 12988 || gate.OpenExtension != 12989 {
		t.Errorf("WoodenGate(12986) = %+v, %v; want open panels 12988/12989", gate, ok)
	}
}
