package game

import (
	"math"
	"testing"
)

// Fixture-only collision tests: hand-built terrain/object streams,
// no network, no cache binaries. The region under test is the one
// containing tile (3200, 3200): region id 12850, absolute origin
// (3200, 3200).

const (
	regionTestAbsX = 3200
	regionTestAbsY = 3200
)

func regionTestID() int { return RegionIDForTile(regionTestAbsX, regionTestAbsY) }

type fakeRegionData struct {
	terrain []byte
	objects []byte
	ok      bool
}

type fakeRegionSource struct {
	ids   []int
	data  map[int]fakeRegionData
	calls map[int]int
}

func newFakeRegionSource(ids []int) *fakeRegionSource {
	return &fakeRegionSource{ids: ids, data: make(map[int]fakeRegionData), calls: make(map[int]int)}
}

func (f *fakeRegionSource) RegionIDs() []int { return f.ids }

func (f *fakeRegionSource) RegionData(regionID int) ([]byte, []byte, bool) {
	f.calls[regionID]++
	d, ok := f.data[regionID]
	if !ok {
		return nil, nil, false
	}
	return d.terrain, d.objects, d.ok
}

type fakeObjectDefs struct {
	defs map[int]ObjectDef
}

func (f *fakeObjectDefs) ObjectDef(id int) (ObjectDef, bool) {
	d, ok := f.defs[id]
	return d, ok
}

// buildTestTerrain assembles a terrain stream: every tile gets the
// terminator for the format unless overridden. Overrides map
// [z, x, y] to the tile's raw bytes (including its terminator).
func buildTestTerrain(newFormat bool, overrides map[[3]int][]byte) []byte {
	var out []byte
	term := []byte{0}
	if newFormat {
		term = []byte{0, 0}
	}
	for z := 0; z < 4; z++ {
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				if b, ok := overrides[[3]int{z, x, y}]; ok {
					out = append(out, b...)
				} else {
					out = append(out, term...)
				}
			}
		}
	}
	return out
}

// tileTypeUShort encodes one new-format tileType value.
func tileTypeUShort(v int) []byte { return []byte{byte(v >> 8), byte(v)} }

// heightTile encodes a new-format tile holding one height value
// (tileType = value + 49) and its terminator.
func heightTile(value int) []byte {
	return append(tileTypeUShort(value+49), 0, 0)
}

// overlayTile encodes a new-format tile holding one overlay id
// (via tileType 2) and its terminator.
func overlayTile(overlay int) []byte {
	b := tileTypeUShort(2)
	b = append(b, tileTypeUShort(overlay)...)
	return append(b, 0, 0)
}

type placementSpec struct {
	loc int // packed (height << 12) | (localX << 6) | localY
	typ int
	dir int
}

type objectSpec struct {
	id         int
	placements []placementSpec
}

func packTestLoc(height, x, y int) int { return (height << 12) | (x << 6) | y }

func appendTestUSmart(out []byte, v int) []byte {
	if v < 128 {
		return append(out, byte(v))
	}
	v += 32768
	return append(out, byte(v>>8), byte(v))
}

// appendTestSmart encodes a readSmart value, emitting 32767 chunks
// the way readSmart's accumulation loop consumes them.
func appendTestSmart(out []byte, v int) []byte {
	for v >= 32767 {
		out = appendTestUSmart(out, 32767)
		v -= 32767
	}
	return appendTestUSmart(out, v)
}

func buildTestObjectStream(specs []objectSpec) []byte {
	var out []byte
	prevID := -1
	for _, spec := range specs {
		out = appendTestSmart(out, spec.id-prevID)
		prevID = spec.id
		prevLoc := 0
		for _, p := range spec.placements {
			out = appendTestUSmart(out, p.loc-prevLoc+1)
			prevLoc = p.loc
			out = append(out, byte((p.typ<<2)|p.dir))
		}
		out = appendTestUSmart(out, 0)
	}
	return appendTestSmart(out, 0)
}

// openTestManager builds a manager whose test region loads from an
// all-terminator terrain stream (open ground) and no objects.
func openTestManager(t *testing.T, defs map[int]ObjectDef) *RegionManager {
	t.Helper()
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{
		terrain: buildTestTerrain(true, nil),
		objects: []byte{0},
		ok:      true,
	}
	var objSrc ObjectDefSource
	if defs != nil {
		objSrc = &fakeObjectDefs{defs: defs}
	}
	m := NewRegionManager(src, objSrc, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Fatalf("test region did not load")
	}
	return m
}

func TestRegionIDForTile(t *testing.T) {
	cases := []struct{ x, y, want int }{
		{0, 0, 0},
		{63, 63, 0},
		{64, 0, 256},
		{0, 64, 1},
		{3200, 3200, 12850},
		{3263, 3263, 12850},
		{3264, 3200, (51 << 8) | 50},
		{-1, -1, -257}, // arithmetic shifts, as in JS >>
	}
	for _, c := range cases {
		if got := RegionIDForTile(c.x, c.y); got != c.want {
			t.Errorf("RegionIDForTile(%d, %d) = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

func TestFlagConstants(t *testing.T) {
	if ProjectileNorthWestBlocked != 0x200 || ProjectileNorthBlocked != 0x400 ||
		ProjectileNorthEastBlocked != 0x800 || ProjectileEastBlocked != 0x1000 ||
		ProjectileSouthEastBlocked != 0x2000 || ProjectileSouthBlocked != 0x4000 ||
		ProjectileSouthWestBlocked != 0x8000 || ProjectileWestBlocked != 0x10000 ||
		ProjectileTileBlocked != 0x20000 {
		t.Error("projectile flag constants differ from RegionManager.ts:21-29")
	}
	if ClipUnknown != 0x80000 || ClipBlockedTile != 0x200000 || ClipUnloadedTile != 0x1000000 {
		t.Error("UNKNOWN/BLOCKED_TILE/UNLOADED_TILE differ from RegionManager.ts:30-32")
	}
	// OCEAN_TILE is spelled 2097152 in TS: the same value as BLOCKED_TILE.
	if ClipOceanTile != 2097152 {
		t.Errorf("ClipOceanTile = %d, want 2097152", ClipOceanTile)
	}
	if ClipOceanTile != ClipBlockedTile {
		t.Errorf("ClipOceanTile %#x != ClipBlockedTile %#x", ClipOceanTile, ClipBlockedTile)
	}
	if BlockedMask != 0x12c0100 {
		t.Errorf("BlockedMask = %#x, want 0x12c0100", BlockedMask)
	}
	// The mask is exactly UNLOADED | BLOCKED | UNKNOWN | floor-decor | solid loc.
	if BlockedMask != ClipUnloadedTile|ClipBlockedTile|ClipUnknown|0x40000|0x100 {
		t.Errorf("BlockedMask %#x does not decompose as documented", BlockedMask)
	}
}

func TestRegionClipGridIndexing(t *testing.T) {
	r := &region{id: regionTestID()}
	// Fresh grid reads 0; removing from a fresh grid is a no-op.
	if got := r.getClip(regionTestAbsX, regionTestAbsY, 0); got != 0 {
		t.Errorf("fresh getClip = %#x, want 0", got)
	}
	r.removeClip(regionTestAbsX, regionTestAbsY, 0, 0x100)

	r.addClip(regionTestAbsX+10, regionTestAbsY+10, 0, 0x100)
	if got := r.getClip(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x100 {
		t.Errorf("getClip = %#x, want 0x100", got)
	}
	// Other planes are independent.
	if got := r.getClip(regionTestAbsX+10, regionTestAbsY+10, 1); got != 0 {
		t.Errorf("plane 1 getClip = %#x, want 0", got)
	}
	// OR accumulation and removal.
	r.addClip(regionTestAbsX+10, regionTestAbsY+10, 0, 0x20)
	r.removeClip(regionTestAbsX+10, regionTestAbsY+10, 0, 0x100)
	if got := r.getClip(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x20 {
		t.Errorf("after remove getClip = %#x, want 0x20", got)
	}
	// Outside the region: -1 index, writes dropped, reads 0.
	r.addClip(regionTestAbsX-1, regionTestAbsY, 0, 0x100)
	r.addClip(regionTestAbsX+64, regionTestAbsY, 0, 0x100)
	r.addClip(regionTestAbsX, regionTestAbsY+64, 0, 0x100)
	r.addClip(regionTestAbsX, regionTestAbsY-1, 0, 0x100)
	for _, at := range [][2]int{
		{regionTestAbsX - 1, regionTestAbsY},
		{regionTestAbsX + 64, regionTestAbsY},
		{regionTestAbsX, regionTestAbsY + 64},
		{regionTestAbsX, regionTestAbsY - 1},
	} {
		if got := r.getClip(at[0], at[1], 0); got != 0 {
			t.Errorf("outside-region getClip(%d, %d) = %#x, want 0", at[0], at[1], got)
		}
	}
	// Out-of-range heights clamp to plane 0 (Region.ts clipIndex).
	r.addClip(regionTestAbsX+20, regionTestAbsY+20, 9, 0x40)
	if got := r.getClip(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0x40 {
		t.Errorf("height-clamped getClip = %#x, want 0x40", got)
	}
	if got := r.getClip(regionTestAbsX+20, regionTestAbsY+20, -1); got != 0x40 {
		t.Errorf("negative-height getClip = %#x, want 0x40 (clamped to plane 0)", got)
	}
}

func TestRegionClipSigned32(t *testing.T) {
	r := &region{id: regionTestID()}
	r.addClip(regionTestAbsX, regionTestAbsY, 0, -1)
	if got := r.getClip(regionTestAbsX, regionTestAbsY, 0); got != -1 {
		t.Errorf("getClip after shift -1 = %d, want -1 (signed 32-bit)", got)
	}
	r2 := &region{id: regionTestID()}
	r2.addClip(regionTestAbsX, regionTestAbsY, 0, 0x80000000)
	if got := r2.getClip(regionTestAbsX, regionTestAbsY, 0); got != math.MinInt32 {
		t.Errorf("getClip after shift 0x80000000 = %d, want MinInt32", got)
	}
}

func TestRegionRoofAndWaterBits(t *testing.T) {
	r := &region{id: regionTestID()}
	// Nil bitsets read false.
	if r.isUnderRoof(regionTestAbsX, regionTestAbsY, 0) || r.isWater(regionTestAbsX, regionTestAbsY, 0) {
		t.Error("nil bitsets must read false")
	}
	r.roofTiles = make([]byte, 2048)
	r.waterTiles = make([]byte, 512)
	roofIndex := (2 << 12) | (10 << 6) | 20
	r.roofTiles[roofIndex>>3] |= 1 << (roofIndex & 7)
	if !r.isUnderRoof(regionTestAbsX+10, regionTestAbsY+20, 2) {
		t.Error("roof bit not reported on plane 2")
	}
	for _, plane := range []int{0, 1, 3} {
		if r.isUnderRoof(regionTestAbsX+10, regionTestAbsY+20, plane) {
			t.Errorf("roof bit reported on plane %d", plane)
		}
	}
	// Out-of-range planes are rejected, not clamped.
	if r.isUnderRoof(regionTestAbsX+10, regionTestAbsY+20, 4) || r.isUnderRoof(regionTestAbsX+10, regionTestAbsY+20, -1) {
		t.Error("isUnderRoof must reject planes outside 0..3")
	}
	waterIndex := (5 << 6) | 5
	r.waterTiles[waterIndex>>3] |= 1 << (waterIndex & 7)
	if !r.isWater(regionTestAbsX+5, regionTestAbsY+5, 0) {
		t.Error("water bit not reported on plane 0")
	}
	if r.isWater(regionTestAbsX+5, regionTestAbsY+5, 1) {
		t.Error("water must be plane 0 only")
	}
	// Coordinates are masked with & 63, aliasing across the region
	// edge exactly as Region.isWater does.
	if !r.isWater(regionTestAbsX+69, regionTestAbsY+5, 0) {
		t.Error("isWater must mask coordinates with & 63 (TS aliasing)")
	}
}

func TestNilSourcesOpenGround(t *testing.T) {
	m := NewRegionManager(nil, nil, true)
	if m.RegionCount() != 0 {
		t.Errorf("RegionCount = %d, want 0", m.RegionCount())
	}
	if got := m.GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("GetClipping = %#x, want 0", got)
	}
	if m.Blocked(Location{X: 3200, Y: 3200, Z: 0}) {
		t.Error("Blocked must be false on open ground")
	}
	if m.IsWater(Location{X: 3200, Y: 3200, Z: 0}) {
		t.Error("IsWater must be false with no regions")
	}
	if !m.CanMove(3200, 3200, 3210, 3210, 0, 1, 1) {
		t.Error("CanMove must be true on open ground")
	}
	if !m.CanMoveStart(Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3201, Y: 3200, Z: 0}, 1) {
		t.Error("CanMoveStart must be true on open ground")
	}
	if got := m.Lookup()(3200, 3200, 0); got != 0 {
		t.Errorf("Lookup = %#x, want 0", got)
	}
	// No-ops must not panic.
	m.LoadRegion(3200, 3200)
	m.AddClipping(3200, 3200, 0, 0x100)
	m.RemoveClipping(3200, 3200, 0, 0x100)
	m.AddObjectClipping(ObjectPlacement{ID: 1, X: 3200, Y: 3200, Type: 10})
	if m.IsRegionLoaded(regionTestID()) {
		t.Error("no region may load without a source")
	}
	if m.ReloadRegion(regionTestID()) {
		t.Error("ReloadRegion of an unindexed region must return false")
	}
}

func TestTerrainDecodeBlockedTile(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{0, 10, 10}: heightTile(1), // bit 1 set -> blocked
		{0, 11, 10}: heightTile(2), // bit 1 clear -> open
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Fatal("region did not load")
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x200000 {
		t.Errorf("blocked tile clip = %#x, want 0x200000", got)
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 1); got != 0 {
		t.Errorf("plane 1 clip = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+11, regionTestAbsY+10, 0); got != 0 {
		t.Errorf("unblocked tile clip = %#x, want 0", got)
	}
}

func TestTerrainDecodePlane1Adjustment(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		// Plane-1 bit-2 tile lowers every plane's clip at (10,10) by one.
		{1, 10, 10}: heightTile(2),
		{0, 10, 10}: heightTile(1), // would land at height -1: skipped
		{2, 10, 10}: heightTile(1), // lands at height 1
		// No adjustment at (20,20): the plane-3 clip stays on plane 3.
		{3, 20, 20}: heightTile(1),
		// Plane 1's own tile with both bits: blocked, lowered to plane 0.
		{1, 30, 30}: heightTile(3),
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	cases := []struct {
		x, y, level int
		want        int32
	}{
		{regionTestAbsX + 10, regionTestAbsY + 10, 0, 0},
		{regionTestAbsX + 10, regionTestAbsY + 10, 1, 0x200000},
		{regionTestAbsX + 10, regionTestAbsY + 10, 2, 0},
		{regionTestAbsX + 20, regionTestAbsY + 20, 3, 0x200000},
		{regionTestAbsX + 20, regionTestAbsY + 20, 2, 0},
		{regionTestAbsX + 30, regionTestAbsY + 30, 0, 0x200000},
		{regionTestAbsX + 30, regionTestAbsY + 30, 1, 0},
	}
	for _, c := range cases {
		if got := m.GetClipping(c.x, c.y, c.level); got != c.want {
			t.Errorf("GetClipping(%d, %d, %d) = %#x, want %#x", c.x, c.y, c.level, got, c.want)
		}
	}
}

func TestTerrainDecodeRoofAndWater(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{2, 20, 20}: heightTile(4), // roof bit, no blocked bit
		{0, 5, 5}:   overlayTile(6),
		{0, 6, 6}:   overlayTile(196), // ice: land
		{0, 7, 7}:   overlayTile(442),
		{0, 8, 8}:   overlayTile(625),
		{0, 9, 9}:   overlayTile(626),
		{0, 11, 11}: overlayTile(5),
		{1, 12, 12}: overlayTile(6), // plane 1 overlay: no water recorded
		// tileType 1 consumes one byte and ends the tile; the next
		// tile must still decode (stream alignment).
		{0, 15, 15}: {0, 1, 0xFF},
		{0, 15, 16}: heightTile(1),
		// tileType > 81 matches no branch; the next tileType is read.
		{0, 16, 16}: append(tileTypeUShort(90), heightTile(1)...),
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	r := m.regions[regionTestID()]
	if !r.isUnderRoof(regionTestAbsX+20, regionTestAbsY+20, 2) {
		t.Error("roof tile not recorded on plane 2")
	}
	if r.isUnderRoof(regionTestAbsX+20, regionTestAbsY+20, 0) {
		t.Error("roof tile leaked to plane 0")
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 2); got != 0 {
		t.Errorf("roof-only tile clip = %#x, want 0", got)
	}
	waterCases := []struct {
		x, y int
		want bool
	}{
		{regionTestAbsX + 5, regionTestAbsY + 5, true},
		{regionTestAbsX + 6, regionTestAbsY + 6, false},
		{regionTestAbsX + 7, regionTestAbsY + 7, true},
		{regionTestAbsX + 8, regionTestAbsY + 8, true},
		{regionTestAbsX + 9, regionTestAbsY + 9, false},
		{regionTestAbsX + 11, regionTestAbsY + 11, false},
		{regionTestAbsX + 12, regionTestAbsY + 12, false},
	}
	for _, c := range waterCases {
		if got := m.IsWater(Location{X: c.x, Y: c.y, Z: 0}); got != c.want {
			t.Errorf("IsWater(%d, %d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	if got := m.GetClipping(regionTestAbsX+15, regionTestAbsY+16, 0); got != 0x200000 {
		t.Errorf("tile after tileType-1 tile = %#x, want 0x200000", got)
	}
	if got := m.GetClipping(regionTestAbsX+16, regionTestAbsY+16, 0); got != 0x200000 {
		t.Errorf("tile after tileType-90 entry = %#x, want 0x200000", got)
	}
}

func TestTerrainDecodeOldFormat(t *testing.T) {
	// Old format: single-byte tileTypes.
	terrain := buildTestTerrain(false, map[[3]int][]byte{
		{0, 10, 10}: {50, 0},
		{0, 5, 5}:   {2, 6, 0},
		{0, 6, 6}:   {1, 0xFF},
		{0, 6, 7}:   {50, 0},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, false)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x200000 {
		t.Errorf("old-format blocked tile = %#x, want 0x200000", got)
	}
	if !m.IsWater(Location{X: regionTestAbsX + 5, Y: regionTestAbsY + 5, Z: 0}) {
		t.Error("old-format water tile not recorded")
	}
	if got := m.GetClipping(regionTestAbsX+6, regionTestAbsY+7, 0); got != 0x200000 {
		t.Errorf("tile after old-format tileType-1 = %#x, want 0x200000", got)
	}
}

func TestTerrainDecodeDegenerateData(t *testing.T) {
	// Empty (but non-nil) terrain decodes as terminators, as TS
	// reads past a Uint8Array yield 0: the region loads, open.
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: []byte{}, objects: []byte{}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Error("empty terrain must still load the region")
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0 {
		t.Errorf("empty terrain clip = %#x, want 0", got)
	}

	// Nil terrain, or ok=false, leaves the region unloaded.
	for name, data := range map[string]fakeRegionData{
		"nil terrain": {terrain: nil, objects: nil, ok: true},
		"not ok":      {terrain: buildTestTerrain(true, nil), objects: nil, ok: false},
	} {
		src2 := newFakeRegionSource([]int{regionTestID()})
		src2.data[regionTestID()] = data
		m2 := NewRegionManager(src2, nil, true)
		m2.LoadRegion(regionTestAbsX, regionTestAbsY)
		if m2.IsRegionLoaded(regionTestID()) {
			t.Errorf("%s: region must stay unloaded", name)
		}
		if got := m2.GetClipping(regionTestAbsX, regionTestAbsY, 0); got != 0 {
			t.Errorf("%s: GetClipping = %#x, want 0", name, got)
		}
	}
}

func TestObjectDecodeSolidAndRotation(t *testing.T) {
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 2, SizeY: 3, ClipType: 2, BlocksProjectile: true},
		101: {SizeX: 2, SizeY: 3, ClipType: 2, BlocksProjectile: false},
	}}
	objects := buildTestObjectStream([]objectSpec{
		{id: 100, placements: []placementSpec{
			{loc: packTestLoc(0, 10, 10), typ: 10, dir: 0},
			{loc: packTestLoc(0, 20, 20), typ: 10, dir: 1},
			{loc: packTestLoc(1, 40, 40), typ: 10, dir: 0},
		}},
		{id: 101, placements: []placementSpec{
			{loc: packTestLoc(0, 30, 30), typ: 9, dir: 0},
		}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: objects, ok: true}
	m := NewRegionManager(src, defs, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)

	solid := int32(0x100 | 0x20000)
	// Direction 0: 2 wide (x) by 3 deep (y).
	for x := 10; x < 12; x++ {
		for y := 10; y < 13; y++ {
			if got := m.GetClipping(regionTestAbsX+x, regionTestAbsY+y, 0); got != solid {
				t.Errorf("dir-0 footprint (%d,%d) = %#x, want %#x", x, y, got, solid)
			}
		}
	}
	if got := m.GetClipping(regionTestAbsX+12, regionTestAbsY+10, 0); got != 0 {
		t.Errorf("past dir-0 footprint = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+13, 0); got != 0 {
		t.Errorf("past dir-0 footprint y = %#x, want 0", got)
	}
	// Direction 1: footprint swapped, 3 wide by 2 deep.
	for x := 20; x < 23; x++ {
		for y := 20; y < 22; y++ {
			if got := m.GetClipping(regionTestAbsX+x, regionTestAbsY+y, 0); got != solid {
				t.Errorf("dir-1 footprint (%d,%d) = %#x, want %#x", x, y, got, solid)
			}
		}
	}
	if got := m.GetClipping(regionTestAbsX+23, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("past dir-1 footprint = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+22, 0); got != 0 {
		t.Errorf("past dir-1 footprint y = %#x, want 0", got)
	}
	// Plane-1 placement clips plane 1 only.
	if got := m.GetClipping(regionTestAbsX+40, regionTestAbsY+40, 1); got != solid {
		t.Errorf("plane-1 footprint = %#x, want %#x", got, solid)
	}
	if got := m.GetClipping(regionTestAbsX+40, regionTestAbsY+40, 0); got != 0 {
		t.Errorf("plane-0 under plane-1 object = %#x, want 0", got)
	}
	// Non-projectile solid: 0x100 only.
	if got := m.GetClipping(regionTestAbsX+30, regionTestAbsY+30, 0); got != 0x100 {
		t.Errorf("non-projectile solid = %#x, want 0x100", got)
	}
}

func TestObjectDecodeWallVariable(t *testing.T) {
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		200: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
		201: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: false},
	}}
	objects := buildTestObjectStream([]objectSpec{
		{id: 200, placements: []placementSpec{
			{loc: packTestLoc(0, 10, 10), typ: 0, dir: 0},
			{loc: packTestLoc(0, 20, 20), typ: 1, dir: 1},
			{loc: packTestLoc(0, 30, 30), typ: 2, dir: 3},
			{loc: packTestLoc(0, 40, 40), typ: 3, dir: 2},
		}},
		{id: 201, placements: []placementSpec{
			{loc: packTestLoc(0, 50, 50), typ: 0, dir: 0},
		}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: objects, ok: true}
	m := NewRegionManager(src, defs, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	cases := []struct {
		x, y int
		want int32
	}{
		{10, 10, 128 | 65536}, {9, 10, 8 | 4096},
		{20, 20, 4 | 2048}, {21, 21, 64 | 32768},
		{30, 30, 160 | 81920}, {30, 29, 2 | 1024}, {29, 30, 8 | 4096},
		{40, 40, 16 | 8192}, {41, 39, 1 | 512},
		{50, 50, 128}, {49, 50, 8}, // non-tall: walk flags only
	}
	for _, c := range cases {
		if got := m.GetClipping(regionTestAbsX+c.x, regionTestAbsY+c.y, 0); got != c.want {
			t.Errorf("wall clip (%d,%d) = %#x, want %#x", c.x, c.y, got, c.want)
		}
	}
}

func TestObjectDecodeDecoration22(t *testing.T) {
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		300: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true},
		301: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
		302: {SizeX: 1, SizeY: 1, ClipType: 0, BlocksProjectile: false},
	}}
	objects := buildTestObjectStream([]objectSpec{
		{id: 300, placements: []placementSpec{{loc: packTestLoc(0, 10, 10), typ: 22, dir: 0}}},
		{id: 301, placements: []placementSpec{{loc: packTestLoc(0, 20, 20), typ: 22, dir: 1}}},
		{id: 302, placements: []placementSpec{{loc: packTestLoc(0, 30, 30), typ: 22, dir: 0}}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: objects, ok: true}
	m := NewRegionManager(src, defs, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x40000 {
		t.Errorf("clipped decoration = %#x, want 0x40000", got)
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("clipType-2 decoration = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+30, regionTestAbsY+30, 0); got != 0 {
		t.Errorf("clipType-0 decoration = %#x, want 0", got)
	}
}

func TestObjectDecodeSkippedTypesMissingDefAndSmart(t *testing.T) {
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		400: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
		401: {SizeX: 1, SizeY: 1, ClipType: 0, BlocksProjectile: false},
		// 40000 is reached only if readSmart accumulates 32767 chunks.
		40000: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	objects := buildTestObjectStream([]objectSpec{
		{id: 400, placements: []placementSpec{
			{loc: packTestLoc(0, 10, 10), typ: 5, dir: 0},  // wall decoration: nothing
			{loc: packTestLoc(0, 11, 10), typ: 12, dir: 0}, // roof: nothing
			{loc: packTestLoc(0, 60, 60), typ: 10, dir: 0}, // 2-byte location delta
		}},
		{id: 401, placements: []placementSpec{
			{loc: packTestLoc(0, 12, 10), typ: 10, dir: 0}, // not solid: nothing
			{loc: packTestLoc(0, 13, 10), typ: 0, dir: 0},  // not solid: nothing
		}},
		// Missing definition: no clipping, stream stays aligned.
		{id: 999, placements: []placementSpec{
			{loc: packTestLoc(0, 14, 10), typ: 10, dir: 0},
		}},
		// Large id delta exercises readSmart's 32767 accumulation.
		{id: 39999, placements: []placementSpec{
			{loc: packTestLoc(0, 15, 10), typ: 10, dir: 0}, // missing def
		}},
		{id: 40000, placements: []placementSpec{
			{loc: packTestLoc(0, 20, 20), typ: 10, dir: 0},
		}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: objects, ok: true}
	m := NewRegionManager(src, defs, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	for _, at := range [][2]int{{10, 10}, {11, 10}, {12, 10}, {13, 10}, {14, 10}, {15, 10}} {
		if got := m.GetClipping(regionTestAbsX+at[0], regionTestAbsY+at[1], 0); got != 0 {
			t.Errorf("skipped object at (%d,%d) = %#x, want 0", at[0], at[1], got)
		}
	}
	if got := m.GetClipping(regionTestAbsX+60, regionTestAbsY+60, 0); got != 0x20100 {
		t.Errorf("far placement = %#x, want 0x20100", got)
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0x20100 {
		t.Errorf("post-accumulation object = %#x, want 0x20100", got)
	}
}

func TestObjectDecodeHeightAdjustmentAndRange(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{1, 10, 10}: heightTile(2), // lowers objects at (10,10) one plane
		{1, 30, 30}: heightTile(2),
	})
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	objects := buildTestObjectStream([]objectSpec{
		{id: 100, placements: []placementSpec{
			// Placements must be in ascending packed-location
			// order: stream deltas are unsigned.
			{loc: packTestLoc(0, 30, 30), typ: 10, dir: 0}, // would land at -1: dropped
			{loc: packTestLoc(1, 10, 10), typ: 10, dir: 0}, // lands on plane 0
			{loc: packTestLoc(3, 40, 40), typ: 10, dir: 0}, // plane 3: kept
			{loc: packTestLoc(5, 20, 20), typ: 10, dir: 0}, // plane 5: dropped
		}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: objects, ok: true}
	m := NewRegionManager(src, defs, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x20100 {
		t.Errorf("lowered object = %#x, want 0x20100", got)
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 1); got != 0 {
		t.Errorf("lowered object on plane 1 = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+30, regionTestAbsY+30, 0); got != 0 {
		t.Errorf("object lowered below plane 0 = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("plane-5 object leaked to plane 0 = %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+40, regionTestAbsY+40, 3); got != 0x20100 {
		t.Errorf("plane-3 object = %#x, want 0x20100", got)
	}
}

func TestNilObjectDefSourceLoadsTerrainOnly(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{{0, 10, 10}: heightTile(1)})
	objects := buildTestObjectStream([]objectSpec{
		{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 20, 20), typ: 10, dir: 0}}},
	})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: objects, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Fatal("region did not load")
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x200000 {
		t.Errorf("terrain clip = %#x, want 0x200000", got)
	}
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("object clip without def source = %#x, want 0", got)
	}
}

// expectedWallCell is one cell of a tall wall's clip footprint.
type expectedWallCell struct {
	dx, dy int
	want   int32
}

// expectedTallWalls transcribes the TS walk+projectile tables
// (RegionManager.ts:112-210) independently of the Go tables:
// map[type][direction] -> cells.
var expectedTallWalls = [4][4][]expectedWallCell{
	{
		{{0, 0, 65664}, {-1, 0, 4104}},
		{{0, 0, 1026}, {0, 1, 16416}},
		{{0, 0, 4104}, {1, 0, 65664}},
		{{0, 0, 16416}, {0, -1, 1026}},
	},
	{
		{{0, 0, 513}, {-1, 1, 8208}},
		{{0, 0, 2052}, {1, 1, 32832}},
		{{0, 0, 8208}, {1, -1, 513}},
		{{0, 0, 32832}, {-1, -1, 2052}},
	},
	{
		{{0, 0, 66690}, {-1, 0, 4104}, {0, 1, 16416}},
		{{0, 0, 5130}, {0, 1, 16416}, {1, 0, 65664}},
		{{0, 0, 20520}, {1, 0, 65664}, {0, -1, 1026}},
		{{0, 0, 82080}, {0, -1, 1026}, {-1, 0, 4104}},
	},
	{
		{{0, 0, 513}, {-1, 1, 8208}},
		{{0, 0, 2052}, {1, 1, 32832}},
		{{0, 0, 8208}, {1, -1, 513}},
		{{0, 0, 32832}, {-1, -1, 2052}},
	},
}

func TestVariableClipsAllTypesAndDirections(t *testing.T) {
	defs := map[int]ObjectDef{
		1: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}
	m := openTestManager(t, defs)
	for typ := 0; typ <= 3; typ++ {
		for dir := 0; dir <= 3; dir++ {
			// Spread cases 5 tiles apart so footprints never overlap.
			baseX, baseY := 5+typ*15, 5+dir*10
			obj := ObjectPlacement{ID: 1, X: regionTestAbsX + baseX, Y: regionTestAbsY + baseY, Level: 0, Type: typ, Direction: dir}
			m.AddObjectClipping(obj)
			want := map[[2]int]int32{}
			for _, c := range expectedTallWalls[typ][dir] {
				want[[2]int{c.dx, c.dy}] = c.want
			}
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					got := m.GetClipping(regionTestAbsX+baseX+dx, regionTestAbsY+baseY+dy, 0)
					if got != want[[2]int{dx, dy}] {
						t.Errorf("type %d dir %d offset (%d,%d) = %#x, want %#x",
							typ, dir, dx, dy, got, want[[2]int{dx, dy}])
					}
				}
			}
			// Removal restores open ground.
			m.RemoveObjectClipping(obj)
			for dx := -1; dx <= 1; dx++ {
				for dy := -1; dy <= 1; dy++ {
					if got := m.GetClipping(regionTestAbsX+baseX+dx, regionTestAbsY+baseY+dy, 0); got != 0 {
						t.Errorf("type %d dir %d offset (%d,%d) after remove = %#x, want 0",
							typ, dir, dx, dy, got)
					}
				}
			}
		}
	}
	// Directions outside 0-3 match no TS branch.
	for _, dir := range []int{-1, 4, 7} {
		obj := ObjectPlacement{ID: 1, X: regionTestAbsX + 60, Y: regionTestAbsY + 60, Level: 0, Type: 0, Direction: dir}
		m.AddObjectClipping(obj)
		if got := m.GetClipping(regionTestAbsX+60, regionTestAbsY+60, 0); got != 0 {
			t.Errorf("direction %d laid clip %#x, want 0", dir, got)
		}
	}
}

func TestObjectClippingRemoveAsymmetry(t *testing.T) {
	// TS removeClippingForVariableObject strips projectile flags for
	// types 1/3 and 2 even when the object is NOT tall, but guards
	// the type-0 removal with `tall`. Both behaviors are preserved.
	defs := map[int]ObjectDef{
		1: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: false},
	}
	m := openTestManager(t, defs)

	// Non-tall type 1: removal strips the projectile flags.
	m.AddClipping(regionTestAbsX+10, regionTestAbsY+10, 0, 512)
	m.AddClipping(regionTestAbsX+9, regionTestAbsY+11, 0, 8192)
	m.RemoveObjectClipping(ObjectPlacement{ID: 1, X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Type: 1, Direction: 0})
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0 {
		t.Errorf("non-tall type-1 removal left %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+9, regionTestAbsY+11, 0); got != 0 {
		t.Errorf("non-tall type-1 removal left %#x at neighbor, want 0", got)
	}

	// Non-tall type 2: same unconditional strip.
	m.AddClipping(regionTestAbsX+20, regionTestAbsY+20, 0, 66560)
	m.RemoveObjectClipping(ObjectPlacement{ID: 1, X: regionTestAbsX + 20, Y: regionTestAbsY + 20, Type: 2, Direction: 0})
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("non-tall type-2 removal left %#x, want 0", got)
	}

	// Non-tall type 0: projectile flags SURVIVE removal.
	m.AddClipping(regionTestAbsX+30, regionTestAbsY+30, 0, 65536)
	m.AddClipping(regionTestAbsX+30, regionTestAbsY+30, 0, 128)
	m.RemoveObjectClipping(ObjectPlacement{ID: 1, X: regionTestAbsX + 30, Y: regionTestAbsY + 30, Type: 0, Direction: 0})
	if got := m.GetClipping(regionTestAbsX+30, regionTestAbsY+30, 0); got != 65536 {
		t.Errorf("non-tall type-0 removal left %#x, want 0x10000 (projectile kept)", got)
	}

	// Non-tall adds lay walk flags only, for every wall type. The
	// placements are spaced 3 tiles apart in a fixed order: wall
	// clips also touch neighboring tiles, so adjacent placements (or
	// map-iteration order) would contaminate each other's reads.
	// Spacing 3 keeps every center inside the 64-wide test region
	// while keeping neighbor clips (reach 1) off other centers.
	for _, tc := range []struct {
		typ  int
		want int32
	}{{0, 128}, {1, 1}, {2, 130}, {3, 1}} {
		x := regionTestAbsX + 40 + tc.typ*3
		m.AddObjectClipping(ObjectPlacement{ID: 1, X: x, Y: regionTestAbsY + 40, Type: tc.typ, Direction: 0})
		if got := m.GetClipping(x, regionTestAbsY+40, 0); got != tc.want {
			t.Errorf("non-tall type %d add = %#x, want %#x", tc.typ, got, tc.want)
		}
	}
}

func TestObjectClippingMinusOneAndMissingDef(t *testing.T) {
	m := openTestManager(t, map[int]ObjectDef{
		1: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	})
	m.AddClipping(regionTestAbsX+10, regionTestAbsY+10, 0, 0x100)
	// id -1 removes a zero shift: a no-op on the grid.
	m.AddObjectClipping(ObjectPlacement{ID: -1, X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Type: 10})
	m.RemoveObjectClipping(ObjectPlacement{ID: -1, X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Type: 10})
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x100 {
		t.Errorf("id -1 changed clip to %#x, want 0x100", got)
	}
	// Missing definition: no clipping, no panic.
	m.AddObjectClipping(ObjectPlacement{ID: 9999, X: regionTestAbsX + 20, Y: regionTestAbsY + 20, Type: 10})
	m.RemoveObjectClipping(ObjectPlacement{ID: 9999, X: regionTestAbsX + 20, Y: regionTestAbsY + 20, Type: 10})
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("missing-def object laid %#x, want 0", got)
	}
}

func TestSolidObjectClippingRoundTrip(t *testing.T) {
	m := openTestManager(t, map[int]ObjectDef{
		1: {SizeX: 2, SizeY: 2, ClipType: 2, BlocksProjectile: true},
	})
	obj := ObjectPlacement{ID: 1, X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Type: 11, Direction: 3}
	m.AddObjectClipping(obj)
	for x := 10; x < 12; x++ {
		for y := 10; y < 12; y++ {
			if got := m.GetClipping(regionTestAbsX+x, regionTestAbsY+y, 0); got != 0x20100 {
				t.Errorf("solid footprint (%d,%d) = %#x, want 0x20100", x, y, got)
			}
		}
	}
	m.RemoveObjectClipping(obj)
	for x := 10; x < 12; x++ {
		for y := 10; y < 12; y++ {
			if got := m.GetClipping(regionTestAbsX+x, regionTestAbsY+y, 0); got != 0 {
				t.Errorf("solid footprint after remove (%d,%d) = %#x, want 0", x, y, got)
			}
		}
	}
}

func TestBlockedHelpers(t *testing.T) {
	m := openTestManager(t, nil)
	pos := Location{X: regionTestAbsX + 30, Y: regionTestAbsY + 30, Z: 0}
	// Each helper reads the adjacent tile against its TS mask: one
	// bit inside the mask blocks, one outside does not, and a bit on
	// the actor's own tile never affects a directional helper.
	cases := []struct {
		name  string
		check func(Location) bool
		dx    int
		dy    int
		in    int // bit inside the mask
		out   int // bit outside the mask
	}{
		{"North", m.BlockedNorth, 0, 1, 0x20, 0x2},
		{"East", m.BlockedEast, 1, 0, 0x80, 0x20},
		{"South", m.BlockedSouth, 0, -1, 0x2, 0x8},
		{"West", m.BlockedWest, -1, 0, 0x8, 0x2},
		{"NorthEast", m.BlockedNorthEast, 1, 1, 0x40, 0x8},
		{"NorthWest", m.BlockedNorthWest, -1, 1, 0x10, 0x40},
		{"SouthEast", m.BlockedSouthEast, 1, -1, 0x1, 0x10},
		{"SouthWest", m.BlockedSouthWest, -1, -1, 0x4, 0x20},
	}
	for _, c := range cases {
		m.RemoveClipping(pos.X+c.dx, pos.Y+c.dy, 0, 0x12c01ff)
		m.RemoveClipping(pos.X, pos.Y, 0, 0x12c01ff)
		if c.check(pos) {
			t.Errorf("%s: blocked on open ground", c.name)
		}
		m.AddClipping(pos.X+c.dx, pos.Y+c.dy, 0, c.in)
		if !c.check(pos) {
			t.Errorf("%s: bit %#x must block", c.name, c.in)
		}
		m.RemoveClipping(pos.X+c.dx, pos.Y+c.dy, 0, c.in)
		m.AddClipping(pos.X+c.dx, pos.Y+c.dy, 0, c.out)
		if c.check(pos) {
			t.Errorf("%s: bit %#x must not block", c.name, c.out)
		}
		m.RemoveClipping(pos.X+c.dx, pos.Y+c.dy, 0, c.out)
		// A bit on the actor's own tile does not affect the helper.
		m.AddClipping(pos.X, pos.Y, 0, c.in)
		if c.check(pos) {
			t.Errorf("%s: own-tile bit %#x must not block", c.name, c.in)
		}
		m.RemoveClipping(pos.X, pos.Y, 0, c.in)
	}
	// blocked() reads the actor's own tile against BLOCKED_MASK.
	for _, bit := range []int{0x100, 0x40000, 0x80000, 0x200000, 0x1000000} {
		m.AddClipping(pos.X, pos.Y, 0, bit)
		if !m.Blocked(pos) {
			t.Errorf("Blocked: bit %#x must block", bit)
		}
		m.RemoveClipping(pos.X, pos.Y, 0, bit)
	}
	m.AddClipping(pos.X, pos.Y, 0, 0x20)
	if m.Blocked(pos) {
		t.Error("Blocked: wall bit 0x20 must not fill the tile")
	}
	m.RemoveClipping(pos.X, pos.Y, 0, 0x20)
	if m.Blocked(pos) {
		t.Error("Blocked: open tile must not be blocked")
	}
}

func TestCanMoveIsBlocked(t *testing.T) {
	m := openTestManager(t, nil)
	pos := Location{X: regionTestAbsX + 30, Y: regionTestAbsY + 30, Z: 0}
	// Open ground: every direction is passable (the method returns
	// true when movement is possible, despite its name).
	for dir := 0; dir <= 7; dir++ {
		if !m.CanMoveIsBlocked(pos, dir) {
			t.Errorf("open ground: direction %d must be passable", dir)
		}
	}
	if m.CanMoveIsBlocked(pos, 8) || m.CanMoveIsBlocked(pos, -1) {
		t.Error("unknown directions must return false")
	}
	// A wall to the north blocks N, NW and NE only.
	m.AddClipping(pos.X, pos.Y+1, 0, 0x20)
	for dir, want := range map[int]bool{0: false, 1: false, 2: false, 3: true, 4: true, 5: true, 6: true, 7: true} {
		if got := m.CanMoveIsBlocked(pos, dir); got != want {
			t.Errorf("north wall: direction %d = %v, want %v", dir, got, want)
		}
	}
	m.RemoveClipping(pos.X, pos.Y+1, 0, 0x20)
	// A wall to the north-east blocks only the NE diagonal.
	m.AddClipping(pos.X+1, pos.Y+1, 0, 0x40)
	if m.CanMoveIsBlocked(pos, 2) {
		t.Error("north-east wall must block direction 2")
	}
	if !m.CanMoveIsBlocked(pos, 1) || !m.CanMoveIsBlocked(pos, 4) {
		t.Error("north-east wall must not block N or E")
	}
}

func TestCanMoveStraightAndDiagonal(t *testing.T) {
	m := openTestManager(t, nil)
	x, y := regionTestAbsX+10, regionTestAbsY+10
	// Straight steps: the destination tile's mask decides.
	m.AddClipping(x+1, y, 0, 0x80)
	if m.CanMove(x, y, x+1, y, 0, 1, 1) {
		t.Error("east step into 0x80 must fail")
	}
	m.RemoveClipping(x+1, y, 0, 0x80)
	m.AddClipping(x+1, y, 0, 0x2)
	if !m.CanMove(x, y, x+1, y, 0, 1, 1) {
		t.Error("east step into 0x2 must succeed")
	}
	m.RemoveClipping(x+1, y, 0, 0x2)

	m.AddClipping(x, y+1, 0, 0x20)
	if m.CanMove(x, y, x, y+1, 0, 1, 1) {
		t.Error("north step into 0x20 must fail")
	}
	m.RemoveClipping(x, y+1, 0, 0x20)
	m.AddClipping(x, y-1, 0, 0x2)
	if m.CanMove(x, y, x, y-1, 0, 1, 1) {
		t.Error("south step into 0x2 must fail")
	}
	m.RemoveClipping(x, y-1, 0, 0x2)
	m.AddClipping(x-1, y, 0, 0x8)
	if m.CanMove(x, y, x-1, y, 0, 1, 1) {
		t.Error("west step into 0x8 must fail")
	}
	m.RemoveClipping(x-1, y, 0, 0x8)

	// Diagonal steps check the corner and both sides.
	if !m.CanMove(x, y, x+1, y+1, 0, 1, 1) {
		t.Error("open diagonal must succeed")
	}
	for _, at := range [][2]int{{x + 1, y + 1}, {x + 1, y}, {x, y + 1}} {
		m.AddClipping(at[0], at[1], 0, 0x100)
		if m.CanMove(x, y, x+1, y+1, 0, 1, 1) {
			t.Errorf("diagonal past blocked (%d,%d) must fail", at[0], at[1])
		}
		m.RemoveClipping(at[0], at[1], 0, 0x100)
	}
	// A bit outside the NE corner mask does not block the diagonal.
	m.AddClipping(x+1, y+1, 0, 0x1)
	if !m.CanMove(x, y, x+1, y+1, 0, 1, 1) {
		t.Error("diagonal into corner bit 0x1 must succeed")
	}
	m.RemoveClipping(x+1, y+1, 0, 0x1)

	// Multi-step paths fail on a wall anywhere along the way.
	m.AddClipping(x+2, y, 0, 0x80)
	if m.CanMove(x, y, x+3, y, 0, 1, 1) {
		t.Error("path through a wall must fail")
	}
	m.RemoveClipping(x+2, y, 0, 0x80)
	if !m.CanMove(x, y, x+3, y, 0, 1, 1) {
		t.Error("open path must succeed")
	}
	// Standing still always succeeds.
	if !m.CanMove(x, y, x, y, 0, 1, 1) {
		t.Error("zero-length move must succeed")
	}
	// Clips on other planes do not interfere.
	m.AddClipping(x+1, y, 1, 0x80)
	if !m.CanMove(x, y, x+1, y, 0, 1, 1) {
		t.Error("plane-1 clip must not block a plane-0 move")
	}
	m.RemoveClipping(x+1, y, 1, 0x80)

	// Size-2 straight steps check the whole leading edge — and, as
	// in TS, the tiles the footprint already covers.
	m.AddClipping(x+2, y+1, 0, 0x80)
	if m.CanMove(x, y, x+1, y, 0, 2, 2) {
		t.Error("size-2 east step past blocked leading edge must fail")
	}
	m.RemoveClipping(x+2, y+1, 0, 0x80)
	m.AddClipping(x+1, y, 0, 0x80)
	if m.CanMove(x, y, x+1, y, 0, 2, 2) {
		t.Error("size-2 east step with blocked footprint tile must fail (TS checks it)")
	}
	m.RemoveClipping(x+1, y, 0, 0x80)
}

func TestCanMoveStartUsesStartLevel(t *testing.T) {
	m := openTestManager(t, nil)
	start := Location{X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Z: 1}
	end := Location{X: regionTestAbsX + 11, Y: regionTestAbsY + 10, Z: 0}
	m.AddClipping(regionTestAbsX+11, regionTestAbsY+10, 1, 0x80)
	if m.CanMoveStart(start, end, 1) {
		t.Error("clip on the start level must block")
	}
	m.RemoveClipping(regionTestAbsX+11, regionTestAbsY+10, 1, 0x80)
	m.AddClipping(regionTestAbsX+11, regionTestAbsY+10, 0, 0x80)
	if !m.CanMoveStart(start, end, 1) {
		t.Error("clip on another level must not block (end Z is ignored)")
	}
}

func TestCanMoveDiagonalLargeActor(t *testing.T) {
	m := openTestManager(t, nil)
	// 2x2 stepping north-east from (10,10): corner (12,12), column
	// (12,11), row (11,12) are checked; old-footprint sides are not.
	x, y := regionTestAbsX+10, regionTestAbsY+10
	if !m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("open size-2 diagonal must succeed")
	}
	m.AddClipping(x+2, y+2, 0, 0x100)
	if m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal into blocked corner must fail")
	}
	m.RemoveClipping(x+2, y+2, 0, 0x100)
	// Column mask 0x12c01e3 contains 0x2 but not 0x8.
	m.AddClipping(x+2, y+1, 0, 0x2)
	if m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal past column bit 0x2 must fail")
	}
	m.RemoveClipping(x+2, y+1, 0, 0x2)
	m.AddClipping(x+2, y+1, 0, 0x8)
	if !m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal past column bit 0x8 must succeed")
	}
	m.RemoveClipping(x+2, y+1, 0, 0x8)
	// Row mask 0x12c01f8 contains 0x10 but not 0x2.
	m.AddClipping(x+1, y+2, 0, 0x10)
	if m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal past row bit 0x10 must fail")
	}
	m.RemoveClipping(x+1, y+2, 0, 0x10)
	m.AddClipping(x+1, y+2, 0, 0x2)
	if !m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal past row bit 0x2 must succeed")
	}
	m.RemoveClipping(x+1, y+2, 0, 0x2)
	// Tiles beside the old footprint are NOT checked (the rsmod
	// corner-cutting case in the TS comment).
	m.AddClipping(x, y+2, 0, 0x100)
	m.AddClipping(x-1, y+1, 0, 0x100)
	if !m.CanMove(x, y, x+1, y+1, 0, 2, 2) {
		t.Error("size-2 diagonal must ignore old-footprint sides")
	}
	m.RemoveClipping(x, y+2, 0, 0x100)
	m.RemoveClipping(x-1, y+1, 0, 0x100)

	// 3x3 stepping north-east: two column tiles, two row tiles.
	x3, y3 := regionTestAbsX+20, regionTestAbsY+20
	m.AddClipping(x3+3, y3+2, 0, 0x2)
	if m.CanMove(x3, y3, x3+1, y3+1, 0, 3, 3) {
		t.Error("size-3 diagonal past blocked column must fail")
	}
	m.RemoveClipping(x3+3, y3+2, 0, 0x2)
	m.AddClipping(x3+2, y3+3, 0, 0x10)
	if m.CanMove(x3, y3, x3+1, y3+1, 0, 3, 3) {
		t.Error("size-3 diagonal past blocked row must fail")
	}
	m.RemoveClipping(x3+2, y3+3, 0, 0x10)
	if !m.CanMove(x3, y3, x3+1, y3+1, 0, 3, 3) {
		t.Error("open size-3 diagonal must succeed")
	}

	// 2x2 stepping south-west from (30,30): corner (29,29), column
	// (29,30), row (30,29).
	xs, ys := regionTestAbsX+30, regionTestAbsY+30
	m.AddClipping(xs-1, ys-1, 0, 0x100)
	if m.CanMove(xs, ys, xs-1, ys-1, 0, 2, 2) {
		t.Error("size-2 SW diagonal into blocked corner must fail")
	}
	m.RemoveClipping(xs-1, ys-1, 0, 0x100)
	// SW row mask 0x12c018f contains 0x4.
	m.AddClipping(xs, ys-1, 0, 0x4)
	if m.CanMove(xs, ys, xs-1, ys-1, 0, 2, 2) {
		t.Error("size-2 SW diagonal past row bit 0x4 must fail")
	}
	m.RemoveClipping(xs, ys-1, 0, 0x4)
}

func TestCanMovePosition(t *testing.T) {
	m := openTestManager(t, nil)
	pos := Location{X: regionTestAbsX + 10, Y: regionTestAbsY + 10, Z: 0}
	if !m.CanMovePosition(pos, DirEast, 1) {
		t.Error("open east step must succeed")
	}
	m.AddClipping(pos.X+1, pos.Y, 0, 0x80)
	if m.CanMovePosition(pos, DirEast, 1) {
		t.Error("east step into a wall must fail")
	}
	m.RemoveClipping(pos.X+1, pos.Y, 0, 0x80)
	m.AddClipping(pos.X+1, pos.Y+1, 0, 0x100)
	if m.CanMovePosition(pos, DirNorthEast, 1) {
		t.Error("diagonal step into a blocked corner must fail")
	}
}

func TestLazyLoadAndSurroundings(t *testing.T) {
	center := regionTestID()
	ids := []int{}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			ids = append(ids, RegionIDForTile(regionTestAbsX+dx*64, regionTestAbsY+dy*64))
		}
	}
	src := newFakeRegionSource(ids)
	for _, id := range ids {
		src.data[id] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: []byte{0}, ok: true}
	}
	m := NewRegionManager(src, nil, true)
	if m.RegionCount() != 9 {
		t.Fatalf("RegionCount = %d, want 9", m.RegionCount())
	}
	for _, id := range ids {
		if m.IsRegionLoaded(id) {
			t.Fatalf("region %d loaded before any lookup", id)
		}
	}
	// A single clip read loads the region and its 8 surroundings.
	if got := m.GetClipping(regionTestAbsX, regionTestAbsY, 0); got != 0 {
		t.Errorf("GetClipping = %#x, want 0", got)
	}
	for _, id := range ids {
		if !m.IsRegionLoaded(id) {
			t.Errorf("region %d not loaded after lookup", id)
		}
		if src.calls[id] != 1 {
			t.Errorf("region %d fetched %d times, want 1", id, src.calls[id])
		}
	}
	// Loaded regions are not re-fetched.
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	_ = m.GetClipping(regionTestAbsX, regionTestAbsY, 0)
	for _, id := range ids {
		if src.calls[id] != 1 {
			t.Errorf("region %d fetched %d times after reload attempt, want 1", id, src.calls[id])
		}
	}
	// A tile with no indexed region answers 0 without a fetch.
	if got := m.GetClipping(0, 0, 0); got != 0 {
		t.Errorf("unindexed GetClipping = %#x, want 0", got)
	}
	if n := src.calls[RegionIDForTile(0, 0)]; n != 0 {
		t.Errorf("unindexed region fetched %d times, want 0", n)
	}
	_ = center
}

func TestFailedLoadRetries(t *testing.T) {
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{ok: false}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if m.IsRegionLoaded(regionTestID()) {
		t.Fatal("failed region must stay unloaded")
	}
	if src.calls[regionTestID()] != 1 {
		t.Fatalf("calls = %d, want 1", src.calls[regionTestID()])
	}
	// The next lookup retries the load, as TS does (loadMapFiles
	// runs whenever no load is in progress and the region is not
	// loaded).
	_ = m.GetClipping(regionTestAbsX, regionTestAbsY, 0)
	if src.calls[regionTestID()] != 2 {
		t.Errorf("calls = %d, want 2 (load retried)", src.calls[regionTestID()])
	}
	// Once the source serves data, the retry succeeds.
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: []byte{0}, ok: true}
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Error("region must load once data is available")
	}
}

func TestAddClippingTriggersLoad(t *testing.T) {
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.AddClipping(regionTestAbsX+5, regionTestAbsY+5, 0, 0x100)
	if !m.IsRegionLoaded(regionTestID()) {
		t.Error("AddClipping must trigger the lazy load")
	}
	if got := m.GetClipping(regionTestAbsX+5, regionTestAbsY+5, 0); got != 0x100 {
		t.Errorf("clip after load-triggering add = %#x, want 0x100", got)
	}
}

func TestReloadRegion(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{{0, 10, 10}: heightTile(1)})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: []byte{0}, ok: true}
	m := NewRegionManager(src, nil, true)
	m.LoadRegion(regionTestAbsX, regionTestAbsY)
	m.AddClipping(regionTestAbsX+20, regionTestAbsY+20, 0, 0x100)
	if !m.ReloadRegion(regionTestID()) {
		t.Fatal("ReloadRegion must return true")
	}
	// Runtime-added clipping is dropped; terrain clipping is rebuilt.
	if got := m.GetClipping(regionTestAbsX+20, regionTestAbsY+20, 0); got != 0 {
		t.Errorf("runtime clip survived reload: %#x, want 0", got)
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0x200000 {
		t.Errorf("terrain clip after reload = %#x, want 0x200000", got)
	}
	if !m.IsRegionLoaded(regionTestID()) {
		t.Error("region must be loaded after reload")
	}
	// Reloading when the source now fails clears state and reports false.
	src.data[regionTestID()] = fakeRegionData{ok: false}
	if m.ReloadRegion(regionTestID()) {
		t.Error("ReloadRegion with no data must return false")
	}
	if m.IsRegionLoaded(regionTestID()) {
		t.Error("region must be unloaded after failed reload")
	}
	if got := m.GetClipping(regionTestAbsX+10, regionTestAbsY+10, 0); got != 0 {
		t.Errorf("clip after failed reload = %#x, want 0", got)
	}
}

func TestLookupMatchesGetClipping(t *testing.T) {
	m := openTestManager(t, nil)
	m.AddClipping(regionTestAbsX+7, regionTestAbsY+7, 0, 0x100)
	lookup := m.Lookup()
	if got := lookup(regionTestAbsX+7, regionTestAbsY+7, 0); got != 0x100 {
		t.Errorf("Lookup = %#x, want 0x100", got)
	}
	if got := lookup(0, 0, 0); got != 0 {
		t.Errorf("Lookup of unindexed tile = %#x, want 0", got)
	}
}

func TestMapReader(t *testing.T) {
	// getUSmart: one byte below 128, ushort minus 32768 otherwise.
	r := &mapReader{buf: []byte{0x40}}
	if got := r.getUSmart(); got != 64 {
		t.Errorf("getUSmart 1-byte = %d, want 64", got)
	}
	r = &mapReader{buf: []byte{0x80, 0x00}}
	if got := r.getUSmart(); got != 0 {
		t.Errorf("getUSmart 0x8000 = %d, want 0", got)
	}
	r = &mapReader{buf: []byte{0xFF, 0xFF}}
	if got := r.getUSmart(); got != 32767 {
		t.Errorf("getUSmart 0xFFFF = %d, want 32767", got)
	}
	// readUShort is big-endian.
	r = &mapReader{buf: []byte{0x12, 0x34}}
	if got := r.readUShort(); got != 0x1234 {
		t.Errorf("readUShort = %#x, want 0x1234", got)
	}
	// readSmart accumulates 32767 chunks.
	r = &mapReader{buf: []byte{0xFF, 0xFF, 0x01}}
	if got := r.readSmart(); got != 32768 {
		t.Errorf("readSmart = %d, want 32768", got)
	}
	r = &mapReader{buf: []byte{0xFF, 0xFF, 0xFF, 0xFF, 0x05}}
	if got := r.readSmart(); got != 32767*2+5 {
		t.Errorf("readSmart = %d, want %d", got, 32767*2+5)
	}
	// Reads past the end return 0, as TS reads past a Uint8Array do.
	r = &mapReader{buf: []byte{}}
	if got := r.readUnsignedByte(); got != 0 {
		t.Errorf("past-end byte = %d, want 0", got)
	}
	if got := r.getUSmart(); got != 0 {
		t.Errorf("past-end getUSmart = %d, want 0", got)
	}
	if got := r.readSmart(); got != 0 {
		t.Errorf("past-end readSmart = %d, want 0", got)
	}
	r = &mapReader{buf: []byte{0x80}}
	if got := r.getUSmart(); got != 0x8000-32768 {
		t.Errorf("truncated ushort getUSmart = %d, want 0", got)
	}
}

func TestIsWaterOverlayID(t *testing.T) {
	cases := []struct {
		id   int
		want bool
	}{
		{0, false}, {5, false}, {6, true}, {7, true},
		{196, false}, // ice: land
		{441, false}, {442, true}, {443, false}, {444, true},
		{445, true}, {625, true}, {626, false},
	}
	for _, c := range cases {
		if got := isWaterOverlayID(c.id); got != c.want {
			t.Errorf("isWaterOverlayID(%d) = %v, want %v", c.id, got, c.want)
		}
	}
}
