package game

import (
	"strings"
	"testing"
)

// installTemplateFixtures installs a fake template source holding
// one region (257: source chunk (8,8)) plus fake object
// definitions, restoring the seams on cleanup. Region 257's
// source tiles are region-local (0..7, 0..7) on the chunk's
// source plane.
func installTemplateFixtures(t *testing.T, terrain, objects []byte, defs map[int]ObjectDef) {
	t.Helper()
	src := newFakeRegionSource([]int{257})
	src.data[257] = fakeRegionData{terrain: terrain, objects: objects, ok: true}
	SetTemplateRegionSource(src, true)
	SetTemplateObjectDefSource(&fakeObjectDefs{defs: defs})
	t.Cleanup(func() {
		SetTemplateRegionSource(nil, false)
		SetTemplateObjectDefSource(nil)
	})
}

// objectGroup is one id group of a template object stream: the
// id and its (packed, attributes) locs in packed order.
type objectGroup struct {
	id   int
	locs [][2]int // {packed, attributes}
}

// buildTemplateObjectStream encodes id groups the way the cache
// map file stores them: readSmart id deltas, getUSmart packed
// deltas, one attributes byte per loc.
func buildTemplateObjectStream(groups []objectGroup) []byte {
	var out []byte
	usmart := func(v int) {
		if v < 128 {
			out = append(out, byte(v))
		} else {
			w := v + 32768
			out = append(out, byte(w>>8), byte(w))
		}
	}
	prevID := -1
	for _, g := range groups {
		usmart(g.id - prevID)
		prevID = g.id
		packed := 0
		for _, loc := range g.locs {
			usmart(loc[0] - packed + 1)
			packed = loc[0]
			out = append(out, byte(loc[1]))
		}
		out = append(out, 0)
	}
	out = append(out, 0)
	return out
}

// packTemplateLoc packs a region-local tile for the object stream.
func packTemplateLoc(plane, x, y int) int { return (plane << 12) | (x << 6) | y }

func expectTemplatedPanic(t *testing.T, name, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("%s: no panic", name)
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, want) {
			t.Fatalf("%s: panic = %v, want containing %q", name, r, want)
		}
	}()
	f()
}

func TestRotateChunkTile(t *testing.T) {
	cases := []struct {
		x, y, rot    int
		wantX, wantY int
	}{
		{0, 0, 0, 0, 0},
		{0, 0, 1, 0, 7},
		{0, 0, 2, 7, 7},
		{0, 0, 3, 7, 0},
		{3, 5, 0, 3, 5},
		{3, 5, 1, 5, 4},
		{3, 5, 2, 4, 2},
		{3, 5, 3, 2, 3},
		// Rotation masks to 0-3.
		{3, 5, 4, 3, 5},
		{3, 5, 5, 5, 4},
	}
	for _, c := range cases {
		if x, y := RotateChunkTile(c.x, c.y, c.rot); x != c.wantX || y != c.wantY {
			t.Errorf("RotateChunkTile(%d,%d,%d) = (%d,%d), want (%d,%d)", c.x, c.y, c.rot, x, y, c.wantX, c.wantY)
		}
	}
}

func TestRotateChunkLoc(t *testing.T) {
	cases := []struct {
		x, y, rot, sx, sy, face int
		wantX, wantY            int
	}{
		{1, 2, 0, 2, 1, 0, 1, 2},
		{1, 2, 1, 2, 1, 0, 2, 5},
		{1, 2, 2, 2, 1, 0, 5, 5},
		{1, 2, 3, 2, 1, 0, 5, 1},
		// The footprint swaps when the loc faces 1 or 3.
		{1, 0, 1, 2, 1, 1, 0, 6},
		{1, 0, 1, 2, 1, 3, 0, 6},
		{1, 0, 1, 2, 1, 2, 0, 5},
		{1, 0, 1, 2, 1, 0, 0, 5},
	}
	for _, c := range cases {
		if x, y := RotateChunkLoc(c.x, c.y, c.rot, c.sx, c.sy, c.face); x != c.wantX || y != c.wantY {
			t.Errorf("RotateChunkLoc(%d,%d,%d,%d,%d,%d) = (%d,%d), want (%d,%d)",
				c.x, c.y, c.rot, c.sx, c.sy, c.face, x, y, c.wantX, c.wantY)
		}
	}
}

func TestPackTemplateChunk(t *testing.T) {
	got := PackTemplateChunk(TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 1, Rotation: 2})
	want := int32((1 << 24) | (8 << 14) | (8 << 3) | (2 << 1))
	if got != want {
		t.Fatalf("PackTemplateChunk = %d, want %d", got, want)
	}
	// Fields mask to their bit widths.
	masked := PackTemplateChunk(TemplateChunk{SourceChunkX: 0x401, SourceChunkY: 0x801, SourcePlane: 5, Rotation: 5})
	plain := PackTemplateChunk(TemplateChunk{SourceChunkX: 1, SourceChunkY: 1, SourcePlane: 1, Rotation: 1})
	if masked != plain {
		t.Fatalf("masked pack = %d, want %d", masked, plain)
	}
}

func TestTemplatedInstanceConstructor(t *testing.T) {
	tia := NewTemplatedInstanceArea(2, 3)
	defer tia.Destroy()
	if tia.WidthChunks() != 2 || tia.HeightChunks() != 3 {
		t.Fatalf("size = %dx%d, want 2x3", tia.WidthChunks(), tia.HeightChunks())
	}
	if tia.GetBaseX() != tia.BaseChunkX()*8 || tia.GetBaseY() != tia.BaseChunkY()*8 {
		t.Fatal("base tiles are not base chunks * 8")
	}
	if tia.GetSceneVersion() != 0 {
		t.Fatal("scene version does not start at 0")
	}
	if tia.PrivateArea == nil || tia.PrivateArea.GetName() != "TemplatedInstanceArea" {
		t.Fatal("embedded PrivateArea missing or misnamed")
	}
	if !tia.UsesTemplateMap() || !tia.PrivateArea.UsesTemplateMap() {
		t.Fatal("UsesTemplateMap false through either view")
	}
	if tia.Templated() != tia || tia.PrivateArea.Templated() != tia {
		t.Fatal("Templated back-pointer does not recover the instance")
	}
	// Geometry: Tile offsets, Contains corners and planes.
	if got := tia.Tile(0, 0, 0); got.X != tia.GetBaseX() || got.Y != tia.GetBaseY() || got.Z != 0 {
		t.Fatalf("Tile(0,0,0) = %v", got)
	}
	if !tia.Contains(tia.Tile(15, 23, 3)) {
		t.Fatal("far corner not contained")
	}
	if tia.Contains(tia.Tile(16, 0, 0)) || tia.Contains(tia.Tile(0, 24, 0)) ||
		tia.Contains(tia.Tile(-1, 0, 0)) || tia.Contains(tia.Tile(0, 0, 4)) {
		t.Fatal("Contains true outside the grid")
	}

	expectTemplatedPanic(t, "zero width", "outside 1-24", func() { NewTemplatedInstanceArea(0, 1) })
	expectTemplatedPanic(t, "zero height", "outside 1-24", func() { NewTemplatedInstanceArea(1, 0) })
	expectTemplatedPanic(t, "too wide", "outside 1-24", func() { NewTemplatedInstanceArea(25, 1) })
	expectTemplatedPanic(t, "too tall", "outside 1-24", func() { NewTemplatedInstanceArea(1, 25) })
}

func TestTemplatedInstanceSlotReuse(t *testing.T) {
	a := NewTemplatedInstanceArea(1, 1)
	baseA := a.GetBaseX()
	a.Destroy()
	b := NewTemplatedInstanceArea(1, 1)
	defer b.Destroy()
	if b.GetBaseX() != baseA {
		t.Fatalf("reused slot base = %d, want %d", b.GetBaseX(), baseA)
	}
	// Destroy is idempotent and frees the slot exactly once.
	b.Destroy()
	b.Destroy()
	c := NewTemplatedInstanceArea(1, 1)
	defer c.Destroy()
	if c.GetBaseX() != baseA {
		t.Fatalf("slot after double destroy = %d, want %d", c.GetBaseX(), baseA)
	}
	d := NewTemplatedInstanceArea(1, 1)
	defer d.Destroy()
	if d.GetBaseX() == c.GetBaseX() {
		t.Fatal("two live instances share a slot")
	}
	if !b.Destroyed() || !b.PrivateArea.Destroyed() {
		t.Fatal("Destroyed false after Destroy through either view")
	}
}

func TestTemplatedInstanceSetGetChunk(t *testing.T) {
	tia := NewTemplatedInstanceArea(2, 2)
	defer tia.Destroy()
	if _, ok := tia.GetChunk(0, 0, 0); ok {
		t.Fatal("empty grid cell reports a chunk")
	}
	if _, ok := tia.GetChunk(0, 2, 0); ok {
		t.Fatal("out-of-grid GetChunk reports a chunk")
	}
	if _, ok := tia.GetChunk(4, 0, 0); ok {
		t.Fatal("out-of-plane GetChunk reports a chunk")
	}
	chunk := TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 5}
	tia.SetChunk(0, 1, 0, &chunk)
	if tia.GetSceneVersion() != 1 {
		t.Fatalf("scene version = %d, want 1 after one SetChunk", tia.GetSceneVersion())
	}
	got, ok := tia.GetChunk(0, 1, 0)
	if !ok {
		t.Fatal("SetChunk cell reports no chunk")
	}
	if got.Rotation != 1 {
		t.Fatalf("stored rotation = %d, want 1 (masked from 5)", got.Rotation)
	}
	// The stored chunk is a copy: mutating the argument does not
	// move the grid.
	chunk.SourceChunkX = 99
	if got, _ := tia.GetChunk(0, 1, 0); got.SourceChunkX != 8 {
		t.Fatal("SetChunk aliased the argument")
	}
	// Clearing with nil invalidates too.
	tia.SetChunk(0, 1, 0, nil)
	if _, ok := tia.GetChunk(0, 1, 0); ok {
		t.Fatal("nil SetChunk did not clear the cell")
	}
	if tia.GetSceneVersion() != 2 {
		t.Fatalf("scene version = %d, want 2 after clear", tia.GetSceneVersion())
	}
	expectTemplatedPanic(t, "SetChunk out of grid", "outside", func() {
		tia.SetChunk(0, 2, 0, &TemplateChunk{})
	})
}

func TestTemplatedInstanceCopySquare(t *testing.T) {
	tia := NewTemplatedInstanceArea(2, 2)
	defer tia.Destroy()
	tia.CopySquare(2, 8, 8, 0, 0, 0, 0, 1)
	if tia.GetSceneVersion() != 4 {
		t.Fatalf("scene version = %d, want 4 (one per cell)", tia.GetSceneVersion())
	}
	// One clockwise turn: source (x,y) lands at rotateSquare.
	want := map[[2]int]TemplateChunk{
		{0, 1}: {SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 1},
		{1, 1}: {SourceChunkX: 8, SourceChunkY: 9, SourcePlane: 0, Rotation: 1},
		{0, 0}: {SourceChunkX: 9, SourceChunkY: 8, SourcePlane: 0, Rotation: 1},
		{1, 0}: {SourceChunkX: 9, SourceChunkY: 9, SourcePlane: 0, Rotation: 1},
	}
	for cell, w := range want {
		got, ok := tia.GetChunk(0, cell[0], cell[1])
		if !ok || got != w {
			t.Errorf("chunk %v = %+v (ok=%v), want %+v", cell, got, ok, w)
		}
	}

	// Rotation 0 copies identity.
	tia2 := NewTemplatedInstanceArea(2, 2)
	defer tia2.Destroy()
	tia2.CopySquare(2, 8, 8, 1, 0, 0, 2, 0)
	got, ok := tia2.GetChunk(2, 1, 1)
	if !ok || got != (TemplateChunk{SourceChunkX: 9, SourceChunkY: 9, SourcePlane: 1, Rotation: 0}) {
		t.Fatalf("rotation-0 copy = %+v (ok=%v)", got, ok)
	}
}

func TestTemplatedInstanceBuildScenePalette(t *testing.T) {
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	chunk := TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0}
	tia.SetChunk(0, 0, 0, &chunk)
	palette, regionIDs := tia.BuildScenePalette(tia.BaseChunkX(), tia.BaseChunkY())
	if len(regionIDs) != 1 || regionIDs[0] != 257 {
		t.Fatalf("regionIDs = %v, want [257]", regionIDs)
	}
	if palette[0][6][6] != PackTemplateChunk(chunk) {
		t.Fatalf("palette center = %d, want packed chunk %d", palette[0][6][6], PackTemplateChunk(chunk))
	}
	for plane := range palette {
		for x := range palette[plane] {
			for y := range palette[plane][x] {
				if plane == 0 && x == 6 && y == 6 {
					continue
				}
				if palette[plane][x][y] != -1 {
					t.Fatalf("palette[%d][%d][%d] = %d, want -1", plane, x, y, palette[plane][x][y])
				}
			}
		}
	}
	// Off-center: the chunk moves in the palette.
	palette2, _ := tia.BuildScenePalette(tia.BaseChunkX()+1, tia.BaseChunkY())
	if palette2[0][5][6] != PackTemplateChunk(chunk) || palette2[0][6][6] != -1 {
		t.Fatal("off-center palette misplaced the chunk")
	}

	// First-seen region order is plane-major: plane 0's region
	// precedes plane 1's, even when plane 1 was set first.
	tia.SetChunk(1, 0, 0, &TemplateChunk{SourceChunkX: 16, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})
	_, regionIDs = tia.BuildScenePalette(tia.BaseChunkX(), tia.BaseChunkY())
	if len(regionIDs) != 2 || regionIDs[0] != 257 || regionIDs[1] != 513 {
		t.Fatalf("regionIDs = %v, want [257 513]", regionIDs)
	}
	// Keys resolve in the same order; a nil resolver yields zeros.
	_, keys := tia.BuildScenePaletteWithKeys(tia.BaseChunkX(), tia.BaseChunkY(), func(regionID int) [4]int32 {
		return [4]int32{int32(regionID), 0, 0, 0}
	})
	if len(keys) != 2 || keys[0][0] != 257 || keys[1][0] != 513 {
		t.Fatalf("keys = %v, want [257 513] first elements", keys)
	}
	_, zeroKeys := tia.BuildScenePaletteWithKeys(tia.BaseChunkX(), tia.BaseChunkY(), nil)
	for i, k := range zeroKeys {
		if k != ([4]int32{}) {
			t.Fatalf("keys[%d] = %v, want zeros with nil resolver", i, k)
		}
	}
}

func TestTemplatedInstanceGridClips(t *testing.T) {
	installTemplateFixtures(t, []byte{}, nil, nil)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})

	inside := tia.Tile(0, 0, 0)
	ring := tia.Tile(-1, 0, 0)
	outside := tia.Tile(-2, 0, 0)
	if !tia.HasClip(inside) || !tia.HasClip(ring) {
		t.Fatal("HasClip false inside the instance or its ring")
	}
	if tia.HasClip(outside) || tia.HasClip(tia.Tile(0, 0, 4)) {
		t.Fatal("HasClip true outside the instance+ring")
	}
	// The ring reads as unmapped; an unmapped chunk reads as
	// unmapped too (plane 1 has no chunk).
	if got := tia.GetClip(ring); got != 0x200100 {
		t.Fatalf("ring GetClip = %#x, want 0x200100", got)
	}
	if got := tia.GetClip(tia.Tile(0, 0, 1)); got != 0x200100 {
		t.Fatalf("unmapped chunk GetClip = %#x, want 0x200100", got)
	}
	// Empty terrain: the mapped tile reads 0.
	if got := tia.GetClip(inside); got != 0 {
		t.Fatalf("empty GetClip = %#x, want 0", got)
	}

	// The PrivateArea view routes to the grid, not the map store.
	pa := tia.PrivateArea
	pa.SetClip(inside, 0x123)
	if got := pa.GetClip(inside); got != 0x123 {
		t.Fatalf("PrivateArea-view GetClip = %#x, want 0x123", got)
	}
	if pa.BaseClipStore().HasClip(inside) {
		t.Fatal("grid write leaked into the base map store")
	}
	pa.RemoveClip(inside)
	if got := pa.GetClip(inside); got != 0 {
		t.Fatalf("RemoveClip GetClip = %#x, want 0", got)
	}
	if !pa.HasClip(inside) {
		t.Fatal("RemoveClip unmapped the grid cell (grid removal keeps the cell)")
	}
	// Writes outside the instance are ignored.
	pa.SetClip(outside, 0x123)
	if got := pa.GetClip(outside); got != 0x200100 {
		t.Fatalf("outside GetClip after write = %#x, want 0x200100", got)
	}
}

func TestTemplatedInstanceBlockedTerrain(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{0, 0, 0}: append(tileTypeUShort(50), 0, 0), // settings 1 (blocked)
	})
	installTemplateFixtures(t, terrain, nil, nil)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})
	if got := tia.GetClip(tia.Tile(0, 0, 0)); got != 0x200000 {
		t.Fatalf("blocked tile GetClip = %#x, want 0x200000", got)
	}
	if got := tia.GetClip(tia.Tile(1, 0, 0)); got != 0 {
		t.Fatalf("open tile GetClip = %#x, want 0", got)
	}
}

func TestTemplatedInstanceBridgeLowering(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		// Plane 1 tile: blocked + bridge (settings 3).
		{1, 0, 0}: append(tileTypeUShort(52), 0, 0),
	})
	installTemplateFixtures(t, terrain, nil, nil)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})
	tia.SetChunk(1, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 1, Rotation: 0})
	// The plane-1 blocked flag lowers to plane 0 over the bridge.
	if got := tia.GetClip(tia.Tile(0, 0, 0)); got != 0x200000 {
		t.Fatalf("bridge-lowered GetClip = %#x, want 0x200000", got)
	}
	if got := tia.GetClip(tia.Tile(0, 0, 1)); got != 0 {
		t.Fatalf("bridge plane GetClip = %#x, want 0", got)
	}
}

func TestTemplatedInstanceRotatedSettings(t *testing.T) {
	terrain := buildTestTerrain(true, map[[3]int][]byte{
		{0, 0, 0}: append(tileTypeUShort(50), 0, 0),
	})
	installTemplateFixtures(t, terrain, nil, nil)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 1})
	// Source (0,0) lands at local (0,7) after one clockwise turn.
	if got := tia.GetClip(tia.Tile(0, 7, 0)); got != 0x200000 {
		t.Fatalf("rotated blocked GetClip = %#x, want 0x200000", got)
	}
	if got := tia.GetClip(tia.Tile(0, 0, 0)); got != 0 {
		t.Fatalf("unrotated tile GetClip = %#x, want 0", got)
	}
}

func TestTemplatedInstanceLocPlacementAndClipping(t *testing.T) {
	objects := buildTemplateObjectStream([]objectGroup{
		{100, [][2]int{{packTemplateLoc(0, 0, 0), (10 << 2) | 0}}},
		{101, [][2]int{{packTemplateLoc(0, 1, 0), (0 << 2) | 1}}},
	})
	defs := map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true},
		101: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: false},
	}
	installTemplateFixtures(t, []byte{}, objects, defs)
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})

	// Placement + lookup by type.
	got := tia.GetTemplateObjects(tia.Tile(0, 0, 0))
	if len(got) != 1 || got[0].ID() != 100 || got[0].Type() != 10 || got[0].Face() != 0 {
		t.Fatalf("GetTemplateObjects(0,0) = %v, want [id 100 type 10 face 0]", got)
	}
	if obj := tia.GetTemplateObject(tia.Tile(1, 0, 0), 0); obj == nil || obj.ID() != 101 {
		t.Fatalf("GetTemplateObject(1,0,type 0) = %v, want id 101", obj)
	}
	if obj := tia.GetTemplateObject(tia.Tile(1, 0, 0), 10); obj != nil {
		t.Fatalf("GetTemplateObject wrong type = %v, want nil", obj)
	}
	if len(tia.GetTemplateObjects(tia.Tile(5, 5, 0))) != 0 {
		t.Fatal("empty tile reports template objects")
	}
	// Template locs are detached: they are scene, not spawned
	// entities.
	if len(tia.GetObjects()) != 0 {
		t.Fatalf("GetObjects = %d, want 0 (template locs detached)", len(tia.GetObjects()))
	}
	// Chunk objects + resolution, including the variant form.
	chunkObjs := tia.GetChunkObjects(0, 0, 0)
	if len(chunkObjs) != 2 {
		t.Fatalf("GetChunkObjects = %d, want 2", len(chunkObjs))
	}
	if obj := tia.ResolveObject(100, tia.Tile(0, 0, 0), nil); obj == nil || obj.ID() != 100 {
		t.Fatalf("ResolveObject(100) = %v", obj)
	}
	variantFor := func(entryID int) int {
		if entryID == 100 {
			return 200
		}
		return entryID
	}
	if obj := tia.ResolveObject(200, tia.Tile(0, 0, 0), variantFor); obj == nil || obj.ID() != 100 {
		t.Fatalf("ResolveObject(200 via variant) = %v, want id 100", obj)
	}
	if obj := tia.ResolveObject(999, tia.Tile(0, 0, 0), variantFor); obj != nil {
		t.Fatalf("ResolveObject(999) = %v, want nil", obj)
	}

	// Object clipping: the solid tall loc lays 256|0x20000 on
	// its tile; the variable wall lays walk flags on its tile
	// and the neighbor (type 0, face 1: (0,0,2),(0,1,32)).
	if got := tia.GetClip(tia.Tile(0, 0, 0)); got&(256|0x20000) != (256 | 0x20000) {
		t.Fatalf("solid loc GetClip = %#x, want solid+projectile bits", got)
	}
	if got := tia.GetClip(tia.Tile(1, 0, 0)); got&2 == 0 {
		t.Fatalf("wall loc GetClip = %#x, want walk bit 2", got)
	}
	if got := tia.GetClip(tia.Tile(1, 1, 0)); got&32 == 0 {
		t.Fatalf("wall neighbor GetClip = %#x, want walk bit 32", got)
	}
}

func TestTemplatedInstanceRotatedLoc(t *testing.T) {
	objects := buildTemplateObjectStream([]objectGroup{
		{100, [][2]int{{packTemplateLoc(0, 0, 0), (10 << 2) | 0}}},
	})
	installTemplateFixtures(t, []byte{}, objects, map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: false},
	})
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 1})
	got := tia.GetTemplateObjects(tia.Tile(0, 7, 0))
	if len(got) != 1 || got[0].ID() != 100 {
		t.Fatalf("rotated loc at (0,7) = %v, want [id 100]", got)
	}
	if got[0].Face() != 1 {
		t.Fatalf("rotated loc face = %d, want 1 (0 + rotation)", got[0].Face())
	}
	if len(tia.GetTemplateObjects(tia.Tile(0, 0, 0))) != 0 {
		t.Fatal("unrotated tile reports the loc")
	}
}

func TestTemplatedInstanceSpawnedObjectClipping(t *testing.T) {
	installTemplateFixtures(t, []byte{}, nil, map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true},
	})
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: 0})
	// Spawned before the first collision build: the grid lays
	// its clipping with the template locs'.
	NewGameObject(100, tia.Tile(3, 3, 0), 10, 0, tia.PrivateArea)
	if got := tia.GetClip(tia.Tile(3, 3, 0)); got&(256|0x20000) != (256 | 0x20000) {
		t.Fatalf("spawned loc GetClip = %#x, want solid+projectile bits", got)
	}
}

func TestTemplatedInstanceSourceTile(t *testing.T) {
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	// Rotation 0: identity.
	tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 2, Rotation: 0})
	x, y, plane, ok := tia.InstanceSourceTile(tia.GetBaseX()+3, tia.GetBaseY()+5, 0)
	if !ok || x != 64+3 || y != 64+5 || plane != 2 {
		t.Fatalf("source tile = (%d,%d,%d,%v), want (67,69,2,true)", x, y, plane, ok)
	}
	// Each rotation inverts the placement turn: the placed tile
	// maps back to source (0,0).
	cases := []struct {
		rot    int
		localX int
		localY int
	}{
		{0, 0, 0},
		{1, 0, 7},
		{2, 7, 7},
		{3, 7, 0},
	}
	for _, c := range cases {
		tia.SetChunk(0, 0, 0, &TemplateChunk{SourceChunkX: 8, SourceChunkY: 8, SourcePlane: 0, Rotation: c.rot})
		x, y, plane, ok := tia.InstanceSourceTile(tia.GetBaseX()+c.localX, tia.GetBaseY()+c.localY, 0)
		if !ok || x != 64 || y != 64 || plane != 0 {
			t.Errorf("rotation %d: source tile = (%d,%d,%d,%v), want (64,64,0,true)", c.rot, x, y, plane, ok)
		}
	}
	// Outside the instance, and on a plane with no chunk: false.
	if _, _, _, ok := tia.InstanceSourceTile(tia.GetBaseX()-1, tia.GetBaseY(), 0); ok {
		t.Fatal("InstanceSourceTile true outside the instance")
	}
	if _, _, _, ok := tia.InstanceSourceTile(tia.GetBaseX(), tia.GetBaseY(), 1); ok {
		t.Fatal("InstanceSourceTile true on a plane with no chunk")
	}
	var nilTIA *TemplatedInstanceArea
	if _, _, _, ok := nilTIA.InstanceSourceTile(0, 0, 0); ok {
		t.Fatal("nil InstanceSourceTile true")
	}
}

func TestTemplatedInstanceMemberViews(t *testing.T) {
	tia := NewTemplatedInstanceArea(1, 1)
	defer tia.Destroy()
	p := NewPlayer("alice", tia.Tile(0, 0, 0))
	p.SetIndex(1)
	tia.Add(p)
	if p.PrivateArea() != tia.PrivateArea {
		t.Fatal("member PrivateArea is not the instance's area")
	}
	if p.PrivateArea().Templated() != tia {
		t.Fatal("member cannot recover the instance through Templated()")
	}
	if !p.PrivateArea().UsesTemplateMap() {
		t.Fatal("UsesTemplateMap false through the member's area view")
	}
}
