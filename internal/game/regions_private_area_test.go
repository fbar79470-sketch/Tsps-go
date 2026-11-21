package game

import "testing"

// testPrivateArea covers the collision test region: a private
// area over the same tiles the region manager loads, so area
// and region clips can be compared tile for tile.
func testPrivateArea() *PrivateArea {
	return NewPrivateArea([]Boundary{NewBoundary(3200, 3263, 3200, 3263, 0)})
}

func TestGetClippingInArea(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	x, y := 3200, 3200

	// No opinion: the region clip answers (open ground: 0).
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0 {
		t.Fatalf("GetClippingInArea = %#x, want 0", got)
	}
	// A region clip shows through until the area has an opinion.
	m.AddClipping(x, y, 0, 0x100)
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0x100 {
		t.Fatalf("GetClippingInArea = %#x, want region clip 0x100", got)
	}
	// An area opinion wins over the region grid.
	pa.SetClip(Location{X: x, Y: y, Z: 0}, 0x200)
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0x200 {
		t.Fatalf("GetClippingInArea = %#x, want area clip 0x200", got)
	}
	// Dropping the opinion falls back to the region clip.
	pa.RemoveClip(Location{X: x, Y: y, Z: 0})
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0x100 {
		t.Fatalf("GetClippingInArea after RemoveClip = %#x, want 0x100", got)
	}
	// A nil area behaves exactly like GetClipping.
	if got := m.GetClippingInArea(x, y, 0, nil); got != m.GetClipping(x, y, 0) {
		t.Fatalf("nil-area GetClippingInArea = %#x, want %#x", got, m.GetClipping(x, y, 0))
	}
}

func TestAddRemoveClippingInArea(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	x, y := 3205, 3205

	// The first area write seeds from the region clip.
	m.AddClipping(x, y, 0, 0x100)
	m.AddClippingInArea(x, y, 0, 0x200, pa)
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0x300 {
		t.Fatalf("seeded area clip = %#x, want 0x300", got)
	}
	// The region grid is untouched by area writes.
	if got := m.GetClipping(x, y, 0); got != 0x100 {
		t.Fatalf("region clip = %#x, want 0x100 (untouched)", got)
	}
	// Further writes build on the stored opinion.
	m.AddClippingInArea(x, y, 0, 0x400, pa)
	m.RemoveClippingInArea(x, y, 0, 0x100, pa)
	if got := m.GetClippingInArea(x, y, 0, pa); got != 0x600 {
		t.Fatalf("area clip = %#x, want 0x600", got)
	}
	if got := m.GetClipping(x, y, 0); got != 0x100 {
		t.Fatalf("region clip = %#x, want 0x100 (untouched)", got)
	}
	// A nil area writes the region grid, exactly like the
	// nil-area forms.
	m.AddClippingInArea(x, y, 0, 0x800, nil)
	if got := m.GetClipping(x, y, 0); got != 0x900 {
		t.Fatalf("nil-area write: region clip = %#x, want 0x900", got)
	}
	m.RemoveClippingInArea(x, y, 0, 0x800, nil)
	if got := m.GetClipping(x, y, 0); got != 0x100 {
		t.Fatalf("nil-area remove: region clip = %#x, want 0x100", got)
	}

	// With no region indexed, the seed is 0 and no region is
	// created by the area write.
	m2 := NewRegionManager(nil, nil, true)
	pa2 := testPrivateArea()
	m2.AddClippingInArea(x, y, 0, 0x200, pa2)
	if got := m2.GetClippingInArea(x, y, 0, pa2); got != 0x200 {
		t.Fatalf("unloaded seed = %#x, want 0x200", got)
	}
	if m2.RegionCount() != 0 {
		t.Fatal("area write created a region")
	}
	if got := m2.GetClipping(x, y, 0); got != 0 {
		t.Fatalf("unloaded region clip = %#x, want 0", got)
	}
}

func TestBlockedInArea(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	pos := Location{X: 3200, Y: 3200, Z: 0}

	// Blocked: the tile's own clip against BLOCKED_MASK.
	pa.SetClip(pos, 0x100)
	if !m.BlockedInArea(pos, pa) {
		t.Fatal("BlockedInArea false with a blocking opinion")
	}
	if m.Blocked(pos) {
		t.Fatal("Blocked true: area opinion leaked into the region view")
	}
	// Directionals read the ADJACENT tile through the area.
	pa.RemoveClip(pos)
	north := Location{X: 3200, Y: 3201, Z: 0}
	pa.SetClip(north, 0x120)
	if !m.BlockedNorthInArea(pos, pa) {
		t.Fatal("BlockedNorthInArea false with a blocking north opinion")
	}
	if m.BlockedNorth(pos) {
		t.Fatal("BlockedNorth true: area opinion leaked")
	}
	if m.BlockedSouthInArea(pos, pa) || m.BlockedEastInArea(pos, pa) || m.BlockedWestInArea(pos, pa) {
		t.Fatal("other directionals true from a north opinion")
	}
	if m.BlockedNorthEastInArea(pos, pa) || m.BlockedNorthWestInArea(pos, pa) ||
		m.BlockedSouthEastInArea(pos, pa) || m.BlockedSouthWestInArea(pos, pa) {
		t.Fatal("diagonal directionals true from a north opinion")
	}
	// CanMoveIsBlocked: north is impossible, south is possible.
	if m.CanMoveIsBlockedInArea(pos, 1, pa) {
		t.Fatal("CanMoveIsBlockedInArea north true with north blocked")
	}
	if !m.CanMoveIsBlockedInArea(pos, 6, pa) {
		t.Fatal("CanMoveIsBlockedInArea south false with south open")
	}
	// Nil areas behave exactly like the nil-area forms.
	if m.BlockedInArea(pos, nil) != m.Blocked(pos) ||
		m.BlockedNorthInArea(pos, nil) != m.BlockedNorth(pos) ||
		m.CanMoveIsBlockedInArea(pos, 1, nil) != m.CanMoveIsBlocked(pos, 1) {
		t.Fatal("nil-area directional/IsBlocked forms diverge from the nil-area forms")
	}
}

func TestCanMoveInArea(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()

	// Single step east: the destination's east mask blocks.
	if !m.CanMoveInArea(3200, 3200, 3201, 3200, 0, 1, 1, pa) {
		t.Fatal("CanMoveInArea false on open ground")
	}
	pa.SetClip(Location{X: 3201, Y: 3200, Z: 0}, 0x12c0180)
	if m.CanMoveInArea(3200, 3200, 3201, 3200, 0, 1, 1, pa) {
		t.Fatal("CanMoveInArea true with the destination blocked in the area")
	}
	if !m.CanMove(3200, 3200, 3201, 3200, 0, 1, 1) {
		t.Fatal("CanMove false: area opinion leaked into the region view")
	}
	pa.RemoveClip(Location{X: 3201, Y: 3200, Z: 0})

	// Diagonal step: the diagonal tile blocks.
	pa.SetClip(Location{X: 3201, Y: 3201, Z: 0}, 0x12c01e0)
	if m.CanMoveInArea(3200, 3200, 3201, 3201, 0, 1, 1, pa) {
		t.Fatal("CanMoveInArea true with the diagonal blocked in the area")
	}
	pa.RemoveClip(Location{X: 3201, Y: 3201, Z: 0})

	// Size 2: a blocked tile at the footprint's far edge blocks.
	pa.SetClip(Location{X: 3202, Y: 3201, Z: 0}, 0x12c0180)
	if m.CanMoveInArea(3200, 3200, 3201, 3200, 0, 2, 2, pa) {
		t.Fatal("CanMoveInArea true with a size-2 footprint edge blocked")
	}
	pa.RemoveClip(Location{X: 3202, Y: 3201, Z: 0})

	// The Start/Position wrappers thread the area.
	pa.SetClip(Location{X: 3201, Y: 3200, Z: 0}, 0x12c0180)
	start := Location{X: 3200, Y: 3200, Z: 0}
	if m.CanMoveStartInArea(start, Location{X: 3201, Y: 3200, Z: 0}, 1, pa) {
		t.Fatal("CanMoveStartInArea true with the destination blocked")
	}
	if m.CanMovePositionInArea(start, DirEast, 1, pa) {
		t.Fatal("CanMovePositionInArea true with the east step blocked")
	}
	if !m.CanMovePositionInArea(start, DirNorth, 1, pa) {
		t.Fatal("CanMovePositionInArea false with the north step open")
	}
	// Nil areas behave exactly like the nil-area forms.
	if m.CanMoveInArea(3200, 3200, 3201, 3200, 0, 1, 1, nil) != m.CanMove(3200, 3200, 3201, 3200, 0, 1, 1) {
		t.Fatal("nil-area CanMoveInArea diverges from CanMove")
	}
}

func TestLookupForArea(t *testing.T) {
	m := openTestManager(t, nil)
	pa := testPrivateArea()
	m.AddClipping(3200, 3200, 0, 0x100)
	lookup := m.LookupForArea(pa)
	if got := lookup(3200, 3200, 0); got != 0x100 {
		t.Fatalf("LookupForArea = %#x, want region clip 0x100", got)
	}
	pa.SetClip(Location{X: 3200, Y: 3200, Z: 0}, 0x200)
	if got := lookup(3200, 3200, 0); got != 0x200 {
		t.Fatalf("LookupForArea = %#x, want area clip 0x200", got)
	}
	if got := m.Lookup()(3200, 3200, 0); got != 0x100 {
		t.Fatalf("Lookup = %#x, want region clip 0x100", got)
	}
	nilLookup := m.LookupForArea(nil)
	if got := nilLookup(3200, 3200, 0); got != 0x100 {
		t.Fatalf("nil-area LookupForArea = %#x, want 0x100", got)
	}
}

func TestObjectClippingInArea(t *testing.T) {
	defs := map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true},
		101: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: false},
		102: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: false},
	}
	m := openTestManager(t, defs)
	pa := testPrivateArea()

	// Solid tall loc: the clip lands on the area store only.
	m.AddObjectClippingInArea(ObjectPlacement{ID: 100, X: 3200, Y: 3200, Level: 0, Type: 10, Direction: 0}, pa)
	if got := m.GetClippingInArea(3200, 3200, 0, pa); got&(256|0x20000) != (256 | 0x20000) {
		t.Fatalf("solid area clip = %#x, want solid+projectile bits", got)
	}
	if got := m.GetClipping(3200, 3200, 0); got != 0 {
		t.Fatalf("region clip = %#x, want 0 (untouched)", got)
	}
	m.RemoveObjectClippingInArea(ObjectPlacement{ID: 100, X: 3200, Y: 3200, Level: 0, Type: 10, Direction: 0}, pa)
	if got := m.GetClippingInArea(3200, 3200, 0, pa); got != 0 {
		t.Fatalf("area clip after remove = %#x, want 0", got)
	}

	// Type 22 clipped decoration: 0x40000, area only.
	m.AddObjectClippingInArea(ObjectPlacement{ID: 102, X: 3201, Y: 3200, Level: 0, Type: 22, Direction: 0}, pa)
	if got := m.GetClippingInArea(3201, 3200, 0, pa); got != 0x40000 {
		t.Fatalf("type-22 area clip = %#x, want 0x40000", got)
	}

	// Variable wall: walk flags on the tile and the neighbor.
	m.AddObjectClippingInArea(ObjectPlacement{ID: 101, X: 3202, Y: 3200, Level: 0, Type: 0, Direction: 0}, pa)
	if got := m.GetClippingInArea(3202, 3200, 0, pa); got&128 == 0 {
		t.Fatalf("wall tile clip = %#x, want walk bit 128", got)
	}
	if got := m.GetClippingInArea(3201, 3200, 0, pa); got&8 == 0 {
		t.Fatalf("wall neighbor clip = %#x, want walk bit 8", got)
	}

	// Unknown definition: no-op, no opinion created.
	m.AddObjectClippingInArea(ObjectPlacement{ID: 999, X: 3210, Y: 3210, Level: 0, Type: 10, Direction: 0}, pa)
	if pa.HasClip(Location{X: 3210, Y: 3210, Z: 0}) {
		t.Fatal("unknown definition created an area opinion")
	}

	// Id -1: TS removes a zero shift through the area branch,
	// which seeds (and stores) the region value without
	// changing it.
	m.AddClipping(3211, 3211, 0, 0x100)
	m.AddObjectClippingInArea(ObjectPlacement{ID: -1, X: 3211, Y: 3211, Level: 0, Type: 10, Direction: 0}, pa)
	if !pa.HasClip(Location{X: 3211, Y: 3211, Z: 0}) {
		t.Fatal("id -1 did not seed an area opinion")
	}
	if got := m.GetClippingInArea(3211, 3211, 0, pa); got != 0x100 {
		t.Fatalf("id -1 area clip = %#x, want seeded 0x100", got)
	}
}

// TestObjectClippingInAreaParity pins the remove-quirk parity
// between the nil-area path (region grid) and the InArea path
// (area store): the same placement sequence must leave the
// same bits in both. Removing a NON-tall type-1 wall strips
// projectile flags too — the upstream quirk — on both paths.
func TestObjectClippingInAreaParity(t *testing.T) {
	defs := map[int]ObjectDef{
		101: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: false},
	}
	// Region path: lay the tall-table projectile flags (512 at
	// the wall tile, 8192 at the neighbor) plus a control bit
	// (0x10000) no strip mask covers, then remove a non-tall
	// type-1 wall over them.
	regionMgr := openTestManager(t, defs)
	regionMgr.AddClipping(3200, 3200, 0, 512|0x10000)
	regionMgr.AddClipping(3199, 3201, 0, 8192)
	regionMgr.RemoveObjectClipping(ObjectPlacement{ID: 101, X: 3200, Y: 3200, Level: 0, Type: 1, Direction: 0})
	// Area path: the same, through the area store.
	areaMgr := openTestManager(t, defs)
	pa := testPrivateArea()
	areaMgr.AddClippingInArea(3200, 3200, 0, 512|0x10000, pa)
	areaMgr.AddClippingInArea(3199, 3201, 0, 8192, pa)
	areaMgr.RemoveObjectClippingInArea(ObjectPlacement{ID: 101, X: 3200, Y: 3200, Level: 0, Type: 1, Direction: 0}, pa)
	// Both paths must agree tile for tile — and both must
	// have stripped the tall-table flags (the quirk) while
	// keeping the control bit.
	for _, tile := range [][2]int{{3200, 3200}, {3199, 3201}, {3201, 3200}, {3200, 3201}} {
		regionClip := regionMgr.GetClipping(tile[0], tile[1], 0)
		areaClip := areaMgr.GetClippingInArea(tile[0], tile[1], 0, pa)
		if regionClip != areaClip {
			t.Errorf("tile %v: region clip %#x != area clip %#x", tile, regionClip, areaClip)
		}
	}
	if got := areaMgr.GetClippingInArea(3200, 3200, 0, pa); got != 0x10000 {
		t.Errorf("wall tile clip = %#x, want 0x10000 (tall flag stripped, control kept)", got)
	}
	if got := areaMgr.GetClippingInArea(3199, 3201, 0, pa); got != 0 {
		t.Errorf("neighbor clip = %#x, want 0 (tall flag stripped)", got)
	}
}

func TestClippingInTemplatedArea(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{0, 0, 0}: append(tileTypeUShort(50), 0, 0), // settings 1 (blocked)
	})
	installTemplateFixtures(t, terrain, nil, nil)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})

	// The RegionManager consults the templated grid through
	// the area: blocked terrain reads blocked, the ring reads
	// unmapped, and the region grid is never involved (no
	// region is indexed out here).
	m := NewRegionManager(nil, nil, true)
	pa := tia.PrivateArea
	base := tia.Tile(0, 0, 0)
	if got := m.GetClippingInArea(base.X, base.Y, 0, pa); got != 0x200000 {
		t.Fatalf("templated GetClippingInArea = %#x, want 0x200000", got)
	}
	ring := tia.Tile(-1, 0, 0)
	if got := m.GetClippingInArea(ring.X, ring.Y, 0, pa); got != 0x200100 {
		t.Fatalf("ring GetClippingInArea = %#x, want 0x200100", got)
	}
	if got := m.LookupForArea(pa)(base.X, base.Y, 0); got != 0x200000 {
		t.Fatalf("templated LookupForArea = %#x, want 0x200000", got)
	}
	if m.BlockedInArea(base, pa) != true {
		t.Fatal("templated BlockedInArea false on the blocked tile")
	}
	// Area writes route to the grid: clearing the blocked tile
	// through the manager opens it.
	m.RemoveClippingInArea(base.X, base.Y, 0, 0x200000, pa)
	if got := m.GetClippingInArea(base.X, base.Y, 0, pa); got != 0 {
		t.Fatalf("grid after RemoveClippingInArea = %#x, want 0", got)
	}
}
