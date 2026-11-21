package game

import (
	"fmt"
	"sync"
)

// TemplatedInstanceArea ports
// game/model/areas/impl/TemplatedInstanceArea.ts (409 lines): a
// private area built from cache template chunks, as OSRS builds
// raids, the Gauntlet, and other dynamic maps. Chunks are copied
// (and turned) into a grid at an allocated place outside the real
// map. The client draws them from the REBUILD_REGION palette,
// which the session sends from BuildScenePalette around each
// player inside; the server builds the same collision and locs
// from the templates, so walking and interacting match what is
// drawn.
//
// Chunks can be copied or cleared at any time (a maze revealing
// rooms): the scene version goes up and everyone inside is sent
// the new palette (the session's rebuild path watches the
// version; see internal/session/flush.go).
//
// CONSTRUCTION AND SLOTS. Instances live where the cache has no
// map (tile 8192+), in slots spaced MAX_INSTANCE_CHUNKS+4
// chunks apart, 6 columns by 70 rows (420 slots). The slot
// allocation is package-global, mirroring the TS module-level
// usedSlots array; Destroy frees the slot. The constructor
// panics on an out-of-range size or when no slot is free,
// mirroring the TS throws. Tests must Destroy every instance
// they create, or the 420 slots exhaust across cases.
//
// CLIPS. The templated area overrides the PrivateArea clip
// methods with a grid (TS method overrides; Go: the area
// installs itself as its PrivateArea's ClipStore at
// construction, and its HasClip/GetClip/SetClip/RemoveClip
// methods shadow the promoted ones). HasClip answers true for
// the instance plus a one-tile ring — nothing walks out of an
// instance — GetClip answers UNMAPPED_TILE outside the
// instance, and SetClip writes the grid (ignoring tiles outside
// the instance). The grid is built lazily from the templates
// and invalidated by every chunk change.
//
// DATA SOURCES. Template decoding reads through the
// TemplateRegionData seam (GetTemplateRegion over the installed
// RegionDataSource; see template_region.go). Object footprints
// and clipping read through the ObjectDefSource seam installed
// with SetTemplateObjectDefSource (the Go form of
// ObjectDefinition.forId, which TS calls as a global). With no
// def source installed, every loc reads as 1x1 with no clipping
// definition — placeChunkLocs uses size 1, and the object
// clipping pass lays nothing — matching the TS behavior for an
// unknown definition (forId returns undefined) extended to the
// no-source case.
//
// NO ENCODE METHOD. TS encodeScene returns a framed
// REBUILD_REGION Buffer via the ClientProtocol encoder. Go
// cannot frame packets from this package (game must never
// import protocol), so the area exposes BuildScenePalette
// (palette + source region ids, or palette + resolved XTEA keys
// via BuildScenePaletteWithKeys) and the session frames it with
// protocol.EncodeRebuildRegion. The palette layout and the
// first-seen XTEA order are exactly the TS ones.
//
// CONCURRENCY. Tick-goroutine-only, like PrivateArea, except
// slot allocation, which is mutex-guarded.

// TemplateChunk is an 8x8 chunk of the cache map that an
// instance copies, turned clockwise Rotation times
// (TemplatedInstanceArea.ts:20-25).
type TemplateChunk struct {
	SourceChunkX int
	SourceChunkY int
	SourcePlane  int
	Rotation     int
}

// Instance geometry constants (TemplatedInstanceArea.ts:28-38).
const (
	// MaxInstanceChunks is MAX_INSTANCE_CHUNKS: the largest
	// instance, in chunks a side, and the spacing between
	// allocations (plus the 4-chunk gap).
	MaxInstanceChunks = 24
)

const (
	templatePlanes      = 4
	templateSceneChunks = 13
	slotStride          = MaxInstanceChunks + 4
	firstSlotChunkX     = 1024
	firstSlotChunkY     = 200
	slotColumns         = 6
	slotRows            = 70
)

// Settings bits (cache terrain) and the collision flags they
// set (TemplatedInstanceArea.ts:40-45).
const (
	templateSettingBlocked = 1
	templateSettingBridge  = 2
	templateBlockedTile    = 0x200000
	// templateUnmappedTile is UNMAPPED_TILE: a tile no chunk
	// has been copied to — nothing is drawn there and nothing
	// walks there.
	templateUnmappedTile = templateBlockedTile | 0x100
)

// instanceSlots is the TS module-level usedSlots array:
// one boolean per allocation slot, false when free.
var (
	instanceSlotMu sync.Mutex
	instanceSlots  [slotColumns * slotRows]bool
)

func allocInstanceSlot() (int, bool) {
	instanceSlotMu.Lock()
	defer instanceSlotMu.Unlock()
	for i, used := range instanceSlots {
		if !used {
			instanceSlots[i] = true
			return i, true
		}
	}
	return 0, false
}

func freeInstanceSlot(slot int) {
	instanceSlotMu.Lock()
	defer instanceSlotMu.Unlock()
	if slot >= 0 && slot < len(instanceSlots) {
		instanceSlots[slot] = false
	}
}

// templateObjectDefs is the ObjectDefinition seam for
// templated areas (see the file comment): the Go form of the
// global ObjectDefinition.forId. Installed once at boot by the
// coordinator; nil means no definitions.
var (
	templateObjectDefsMu sync.Mutex
	templateObjectDefs   ObjectDefSource
)

// SetTemplateObjectDefSource installs the object-definition
// source templated areas read footprints and clipping through.
// A nil source means no definition ever resolves (1x1 locs, no
// object clipping). Call once at boot, before ticking starts;
// tests install fakes and restore with a cleanup.
func SetTemplateObjectDefSource(src ObjectDefSource) {
	templateObjectDefsMu.Lock()
	defer templateObjectDefsMu.Unlock()
	templateObjectDefs = src
}

func templateObjectDef(id int) (ObjectDef, bool) {
	templateObjectDefsMu.Lock()
	defer templateObjectDefsMu.Unlock()
	if templateObjectDefs == nil {
		return ObjectDef{}, false
	}
	return templateObjectDefs.ObjectDef(id)
}

// PackTemplateChunk packs a chunk into the 26-bit palette value
// (TemplatedInstanceArea.ts:58-63): plane in bits 24-25,
// sourceChunkX in bits 14-23, sourceChunkY in bits 3-13,
// rotation in bits 1-2. The value always fits in 26 bits, so
// the int32 form equals the TS unsigned form numerically.
func PackTemplateChunk(chunk TemplateChunk) int32 {
	return int32(((chunk.SourcePlane & 3) << 24) |
		((chunk.SourceChunkX & 0x3ff) << 14) |
		((chunk.SourceChunkY & 0x7ff) << 3) |
		((chunk.Rotation & 3) << 1))
}

// RotateChunkTile answers where a tile of a chunk lands once
// the chunk is turned (the client's rotation)
// (TemplatedInstanceArea.ts:66-74).
func RotateChunkTile(x, y, rotation int) (int, int) {
	switch rotation & 3 {
	case 0:
		return x, y
	case 1:
		return y, 7 - x
	case 2:
		return 7 - x, 7 - y
	default:
		return 7 - y, x
	}
}

// RotateChunkLoc answers where a loc's south-west tile lands
// once its chunk is turned; sizeX/sizeY are the loc's
// footprint (TemplatedInstanceArea.ts:77-86). The footprint
// swaps when the loc faces 1 or 3, as in TS.
func RotateChunkLoc(x, y, rotation, sizeX, sizeY, face int) (int, int) {
	if (face & 1) == 1 {
		sizeX, sizeY = sizeY, sizeX
	}
	switch rotation & 3 {
	case 0:
		return x, y
	case 1:
		return y, 7 - x - (sizeX - 1)
	case 2:
		return 7 - x - (sizeX - 1), 7 - y - (sizeY - 1)
	default:
		return 7 - y - (sizeY - 1), x
	}
}

// rotateSquare answers a chunk's place in a turned square
// block, last being the block's size less one
// (TemplatedInstanceArea.ts:405-412).
func rotateSquare(x, y, last, rotation int) (int, int) {
	switch rotation & 3 {
	case 0:
		return x, y
	case 1:
		return y, last - x
	case 2:
		return last - x, last - y
	default:
		return last - y, x
	}
}

func regionOfChunk(chunk TemplateChunk) int {
	return ((chunk.SourceChunkX >> 3) << 8) | (chunk.SourceChunkY >> 3)
}

// placedLoc is one template loc placed in the instance grid
// (TemplatedInstanceArea.ts:47-54, PlacedLoc): the id, the
// placed tile, the shape type and facing, plus the lazily
// built GameObject.
type placedLoc struct {
	id   int
	x, y int
	z    int
	typ  int
	face int
	obj  *GameObject
}

// TemplatedInstanceArea is a private area built from cache
// template chunks. See the file comment. It embeds
// *PrivateArea, whose embedded *Area carries the instance
// boundaries (one per plane); the templated back-pointer on
// the PrivateArea lets the *PrivateArea view reach this value.
type TemplatedInstanceArea struct {
	*PrivateArea

	baseChunkX, baseChunkY    int
	widthChunks, heightChunks int
	slot                      int
	chunks                    []*TemplateChunk
	clipGrid                  []int32
	locIndex                  map[int][]*placedLoc
	sceneVersion              int
}

// NewTemplatedInstanceArea allocates an instance of
// widthChunks by heightChunks chunks
// (TemplatedInstanceArea.ts:100-123). It panics when the size
// is outside 1-MaxInstanceChunks or no allocation slot is
// free, mirroring the TS throws with the same messages. The
// caller must call Destroy to free the slot.
func NewTemplatedInstanceArea(widthChunks, heightChunks int) *TemplatedInstanceArea {
	if widthChunks < 1 || heightChunks < 1 || widthChunks > MaxInstanceChunks || heightChunks > MaxInstanceChunks {
		panic(fmt.Sprintf("Templated instance size %dx%d is outside 1-%d chunks.", widthChunks, heightChunks, MaxInstanceChunks))
	}
	slot, ok := allocInstanceSlot()
	if !ok {
		panic("No templated instance allocation is available.")
	}
	baseChunkX := firstSlotChunkX + (slot%slotColumns)*slotStride
	baseChunkY := firstSlotChunkY + (slot/slotColumns)*slotStride
	minX := baseChunkX * 8
	minY := baseChunkY * 8
	boundaries := make([]Boundary, 0, templatePlanes)
	for z := 0; z < templatePlanes; z++ {
		boundaries = append(boundaries, NewBoundary(minX, minX+widthChunks*8-1, minY, minY+heightChunks*8-1, z))
	}
	pa := newPrivateArea("TemplatedInstanceArea", boundaries)
	tia := &TemplatedInstanceArea{
		PrivateArea:  pa,
		baseChunkX:   baseChunkX,
		baseChunkY:   baseChunkY,
		widthChunks:  widthChunks,
		heightChunks: heightChunks,
		slot:         slot,
		chunks:       make([]*TemplateChunk, templatePlanes*widthChunks*heightChunks),
		locIndex:     make(map[int][]*placedLoc),
	}
	pa.templated = tia
	// The grid clip overrides (TS method overrides): the area
	// is its own clip store.
	pa.SetClipStore(tia)
	return tia
}

// GetBaseX returns the instance's south-west tile X
// (TemplatedInstanceArea.ts:125-127).
func (tia *TemplatedInstanceArea) GetBaseX() int {
	if tia == nil {
		return 0
	}
	return tia.baseChunkX * 8
}

// GetBaseY returns the instance's south-west tile Y
// (TemplatedInstanceArea.ts:129-131).
func (tia *TemplatedInstanceArea) GetBaseY() int {
	if tia == nil {
		return 0
	}
	return tia.baseChunkY * 8
}

// BaseChunkX returns the instance's south-west chunk X.
func (tia *TemplatedInstanceArea) BaseChunkX() int {
	if tia == nil {
		return 0
	}
	return tia.baseChunkX
}

// BaseChunkY returns the instance's south-west chunk Y.
func (tia *TemplatedInstanceArea) BaseChunkY() int {
	if tia == nil {
		return 0
	}
	return tia.baseChunkY
}

// WidthChunks returns the instance width in chunks.
func (tia *TemplatedInstanceArea) WidthChunks() int {
	if tia == nil {
		return 0
	}
	return tia.widthChunks
}

// HeightChunks returns the instance height in chunks.
func (tia *TemplatedInstanceArea) HeightChunks() int {
	if tia == nil {
		return 0
	}
	return tia.heightChunks
}

// Tile returns the world tile at an offset from the instance's
// south-west corner (TemplatedInstanceArea.ts:134-136).
func (tia *TemplatedInstanceArea) Tile(localX, localY, z int) Location {
	return Location{X: tia.GetBaseX() + localX, Y: tia.GetBaseY() + localY, Z: z}
}

// Contains reports whether location lies within the instance
// grid (TemplatedInstanceArea.ts:138-143).
func (tia *TemplatedInstanceArea) Contains(loc Location) bool {
	if tia == nil {
		return false
	}
	x := loc.X - tia.GetBaseX()
	y := loc.Y - tia.GetBaseY()
	z := loc.Z
	return x >= 0 && y >= 0 && z >= 0 && z < templatePlanes && x < tia.widthChunks*8 && y < tia.heightChunks*8
}

// GetSceneVersion returns the scene version: incremented by
// every chunk change (TemplatedInstanceArea.ts:145-147). The
// session's rebuild path watches it for palette invalidation.
func (tia *TemplatedInstanceArea) GetSceneVersion() int {
	if tia == nil {
		return 0
	}
	return tia.sceneVersion
}

// UsesTemplateMap reports true: the scene is copied from cache
// template chunks (TemplatedInstanceArea.ts:149-151). It
// shadows the promoted PrivateArea method, which answers the
// same through the back-pointer.
func (tia *TemplatedInstanceArea) UsesTemplateMap() bool { return true }

// GetChunk returns the chunk copied at a chunk offset within
// the instance, or false when the offset is outside the grid
// or no chunk was copied there
// (TemplatedInstanceArea.ts:153-155). Chunk offsets are
// relative to the instance's south-west corner, in chunks.
func (tia *TemplatedInstanceArea) GetChunk(plane, chunkX, chunkY int) (TemplateChunk, bool) {
	if tia == nil || !tia.inGrid(plane, chunkX, chunkY) {
		return TemplateChunk{}, false
	}
	chunk := tia.chunks[tia.chunkIndex(plane, chunkX, chunkY)]
	if chunk == nil {
		return TemplateChunk{}, false
	}
	return *chunk, true
}

// SetChunk copies one chunk (or clears it with nil) at a chunk
// offset within the instance (TemplatedInstanceArea.ts:158-164).
// It panics when the offset is outside the grid, mirroring the
// TS throw, and invalidates the scene (the version goes up).
// The chunk is copied with its rotation masked to 0-3.
func (tia *TemplatedInstanceArea) SetChunk(plane, chunkX, chunkY int, chunk *TemplateChunk) {
	if !tia.inGrid(plane, chunkX, chunkY) {
		panic(fmt.Sprintf("Chunk %d,%d,%d is outside the %dx%d instance.", chunkX, chunkY, plane, tia.widthChunks, tia.heightChunks))
	}
	if chunk == nil {
		tia.chunks[tia.chunkIndex(plane, chunkX, chunkY)] = nil
	} else {
		copied := *chunk
		copied.Rotation &= 3
		tia.chunks[tia.chunkIndex(plane, chunkX, chunkY)] = &copied
	}
	tia.invalidate()
}

// CopySquare copies a size x size block of chunks, the whole
// block turned as one: what is at the source's north-west ends
// up north-east after one clockwise turn
// (TemplatedInstanceArea.ts:170-184). It calls SetChunk per
// cell, so the scene version goes up once per cell and an
// out-of-grid target panics midway through a partial copy —
// exactly the TS behavior. The parameter order is the TS one:
// size, source chunk, destination chunk offset, plane,
// rotation.
func (tia *TemplatedInstanceArea) CopySquare(size, sourceChunkX, sourceChunkY, sourcePlane, chunkX, chunkY, plane, rotation int) {
	last := size - 1
	for x := 0; x < size; x++ {
		for y := 0; y < size; y++ {
			toX, toY := rotateSquare(x, y, last, rotation)
			tia.SetChunk(plane, chunkX+toX, chunkY+toY, &TemplateChunk{
				SourceChunkX: sourceChunkX + x,
				SourceChunkY: sourceChunkY + y,
				SourcePlane:  sourcePlane,
				Rotation:     rotation,
			})
		}
	}
}

// BuildScenePalette returns the REBUILD_REGION palette for the
// 13x13-chunk scene centred on a chunk, plus the distinct
// source region ids the palette draws from, in first-seen
// order (TemplatedInstanceArea.ts:180-200, buildScenePalette).
// Palette entries are packed chunks, or -1 where no chunk was
// copied. The caller resolves the region ids to XTEA keys —
// TS calls CachePipeline.getXtea inline, which this package
// cannot reach (see BuildScenePaletteWithKeys for the
// resolver-taking form). The palette index order is
// [plane][x][y], matching protocol.EncodeRebuildRegion.
func (tia *TemplatedInstanceArea) BuildScenePalette(centerChunkX, centerChunkY int) ([4][13][13]int32, []int) {
	var palette [4][13][13]int32
	for plane := range palette {
		for x := range palette[plane] {
			for y := range palette[plane][x] {
				palette[plane][x][y] = -1
			}
		}
	}
	var regionIDs []int
	seen := make(map[int]struct{})
	for plane := 0; plane < templatePlanes; plane++ {
		for x := 0; x < templateSceneChunks; x++ {
			for y := 0; y < templateSceneChunks; y++ {
				chunk, ok := tia.GetChunk(plane, centerChunkX-6+x-tia.baseChunkX, centerChunkY-6+y-tia.baseChunkY)
				if !ok {
					continue
				}
				palette[plane][x][y] = PackTemplateChunk(chunk)
				regionID := regionOfChunk(chunk)
				if _, dup := seen[regionID]; !dup {
					seen[regionID] = struct{}{}
					regionIDs = append(regionIDs, regionID)
				}
			}
		}
	}
	return palette, regionIDs
}

// BuildScenePaletteWithKeys is BuildScenePalette with the XTEA
// keys resolved through xtea, in the same first-seen order TS
// collects them (TemplatedInstanceArea.ts:193-197). A nil
// resolver yields all-zero keys, matching CachePipeline.getXtea
// for an unknown region. The result feeds
// protocol.EncodeRebuildRegion directly (from the session
// layer, which owns the XTEA provider).
func (tia *TemplatedInstanceArea) BuildScenePaletteWithKeys(centerChunkX, centerChunkY int, xtea func(regionID int) [4]int32) ([4][13][13]int32, [][4]int32) {
	palette, regionIDs := tia.BuildScenePalette(centerChunkX, centerChunkY)
	keys := make([][4]int32, 0, len(regionIDs))
	for _, regionID := range regionIDs {
		var key [4]int32
		if xtea != nil {
			key = xtea(regionID)
		}
		keys = append(keys, key)
	}
	return palette, keys
}

// HasClip reports whether the area has a clip opinion at loc:
// the instance plus a ring of tiles around it, on planes 0-3
// (TemplatedInstanceArea.ts:210-216). Nothing walks out of an
// instance: the ring reads as unmapped through GetClip.
func (tia *TemplatedInstanceArea) HasClip(loc Location) bool {
	if tia == nil {
		return false
	}
	x := loc.X - tia.GetBaseX()
	y := loc.Y - tia.GetBaseY()
	z := loc.Z
	return x >= -1 && y >= -1 && z >= 0 && z < templatePlanes && x <= tia.widthChunks*8 && y <= tia.heightChunks*8
}

// GetClip returns the collision mask at loc: UNMAPPED_TILE
// outside the instance, the grid value inside
// (TemplatedInstanceArea.ts:218-221).
func (tia *TemplatedInstanceArea) GetClip(loc Location) int32 {
	if tia == nil {
		return 0
	}
	if !tia.Contains(loc) {
		return templateUnmappedTile
	}
	return tia.collision()[tia.tileIndex(loc.X, loc.Y, loc.Z)]
}

// SetClip writes the collision mask at loc, ignoring tiles
// outside the instance (TemplatedInstanceArea.ts:223-226).
func (tia *TemplatedInstanceArea) SetClip(loc Location, mask int32) {
	if tia == nil || !tia.Contains(loc) {
		return
	}
	tia.collision()[tia.tileIndex(loc.X, loc.Y, loc.Z)] = mask
}

// RemoveClip resets the collision mask at loc to 0
// (TemplatedInstanceArea.ts:228-230). Unlike the map store,
// removal does not unmap the tile: the grid cell keeps
// existing, with a zero mask.
func (tia *TemplatedInstanceArea) RemoveClip(loc Location) {
	tia.SetClip(loc, 0)
}

// GetTemplateObject returns the template loc of a type on a
// tile, as the client draws it there
// (TemplatedInstanceArea.ts:237-240), or nil when none matches.
func (tia *TemplatedInstanceArea) GetTemplateObject(loc Location, typ int) *GameObject {
	for _, entry := range tia.locsAt(loc) {
		if entry.typ == typ {
			return tia.objectFor(entry)
		}
	}
	return nil
}

// GetTemplateObjects returns every template loc on a tile
// (TemplatedInstanceArea.ts:243-245).
func (tia *TemplatedInstanceArea) GetTemplateObjects(loc Location) []*GameObject {
	locs := tia.locsAt(loc)
	objects := make([]*GameObject, 0, len(locs))
	for _, entry := range locs {
		objects = append(objects, tia.objectFor(entry))
	}
	return objects
}

// ResolveObject validates a client-only dynamic loc on demand
// (TemplatedInstanceArea.ts:247-251): the template loc whose id
// matches, or whose player-visible variant id matches through
// variantFor (the Go form of ObjectDefinition.forPlayer; nil
// means identity, no multi-loc resolution). It returns nil
// when no template loc on the tile matches.
func (tia *TemplatedInstanceArea) ResolveObject(id int, loc Location, variantFor func(entryID int) int) *GameObject {
	for _, entry := range tia.locsAt(loc) {
		if entry.id == id || (variantFor != nil && variantFor(entry.id) == id) {
			return tia.objectFor(entry)
		}
	}
	return nil
}

// GetChunkObjects returns all template locs (with an id) on a
// plane of a chunk, e.g. to find a room's spawn spots
// (TemplatedInstanceArea.ts:254-266).
func (tia *TemplatedInstanceArea) GetChunkObjects(plane, chunkX, chunkY int) []*GameObject {
	tia.collision()
	var objects []*GameObject
	minX := tia.GetBaseX() + chunkX*8
	minY := tia.GetBaseY() + chunkY*8
	for x := minX; x < minX+8; x++ {
		for y := minY; y < minY+8; y++ {
			objects = append(objects, tia.GetTemplateObjects(Location{X: x, Y: y, Z: plane})...)
		}
	}
	return objects
}

// InstanceSourceTile answers the cache-map tile an instance
// tile copies, through its chunk's rotation — the Go form of
// Music.instanceSourceTile (Music.ts:195-205). It returns
// false outside the instance's chunks. The music block maps
// the player's tile through it so an instance plays the music
// of the cache-map tiles it copies (PlayerSession.ts:379-381);
// see the Phase 21 report for the music.go seam.
func (tia *TemplatedInstanceArea) InstanceSourceTile(x, y, plane int) (sourceX, sourceY, sourcePlane int, ok bool) {
	if tia == nil {
		return 0, 0, 0, false
	}
	localX := x - tia.GetBaseX()
	localY := y - tia.GetBaseY()
	chunk, found := tia.GetChunk(plane, localX>>3, localY>>3)
	if !found {
		return 0, 0, 0, false
	}
	// The placed tile turned back: a chunk turned clockwise r
	// times is undone by 4 - r turns.
	tx := localX & 7
	ty := localY & 7
	var sx, sy int
	switch chunk.Rotation & 3 {
	case 0:
		sx, sy = tx, ty
	case 1:
		sx, sy = 7-ty, tx
	case 2:
		sx, sy = 7-tx, 7-ty
	default:
		sx, sy = ty, 7-tx
	}
	return (chunk.SourceChunkX << 3) + sx, (chunk.SourceChunkY << 3) + sy, chunk.SourcePlane, true
}

// Destroy frees the allocation slot, drops the collision grid
// and loc index, and runs the PrivateArea destruction
// (TemplatedInstanceArea.ts:268-274). It is idempotent through
// the PrivateArea guard.
func (tia *TemplatedInstanceArea) Destroy() {
	if tia == nil || tia.Destroyed() {
		return
	}
	freeInstanceSlot(tia.slot)
	tia.clipGrid = nil
	tia.locIndex = make(map[int][]*placedLoc)
	tia.PrivateArea.Destroy()
}

// invalidate drops the collision grid and bumps the scene
// version (TemplatedInstanceArea.ts:280-283). The loc index
// rebuilds with the grid on the next collision() call.
func (tia *TemplatedInstanceArea) invalidate() {
	tia.clipGrid = nil
	tia.sceneVersion++
}

func (tia *TemplatedInstanceArea) locsAt(loc Location) []*placedLoc {
	if !tia.Contains(loc) {
		return nil
	}
	tia.collision()
	return tia.locIndex[tia.tileIndex(loc.X, loc.Y, loc.Z)]
}

func (tia *TemplatedInstanceArea) objectFor(loc *placedLoc) *GameObject {
	if loc.obj == nil {
		loc.obj = NewGameObject(loc.id, Location{X: loc.x, Y: loc.y, Z: loc.z}, loc.typ, loc.face, tia.PrivateArea)
		// Template locs belong to the scene, not to the
		// area's spawned entities (the constructor attached
		// it; detach it again).
		tia.DetachObject(loc.obj)
	}
	return loc.obj
}

func (tia *TemplatedInstanceArea) inGrid(plane, chunkX, chunkY int) bool {
	return tia != nil && plane >= 0 && plane < templatePlanes && chunkX >= 0 && chunkY >= 0 &&
		chunkX < tia.widthChunks && chunkY < tia.heightChunks
}

func (tia *TemplatedInstanceArea) chunkIndex(plane, chunkX, chunkY int) int {
	return (plane*tia.widthChunks+chunkX)*tia.heightChunks + chunkY
}

func (tia *TemplatedInstanceArea) tileIndex(x, y, z int) int {
	width := tia.widthChunks * 8
	height := tia.heightChunks * 8
	return (z*width+(x-tia.GetBaseX()))*height + (y - tia.GetBaseY())
}

// collision returns the collision grid, rebuilt from the
// templates after any chunk changed
// (TemplatedInstanceArea.ts:302-350): terrain settings copied
// per chunk (blocked tiles, lowered a plane over bridges),
// unmapped tiles flagged, template locs placed, then object
// clipping laid for template locs and for spawned locs
// (resources, doors swapped at runtime).
func (tia *TemplatedInstanceArea) collision() []int32 {
	if tia.clipGrid != nil {
		return tia.clipGrid
	}
	width := tia.widthChunks * 8
	height := tia.heightChunks * 8
	grid := make([]int32, templatePlanes*width*height)
	tia.clipGrid = grid
	tia.locIndex = make(map[int][]*placedLoc)

	settings := make([]uint8, len(grid))
	for plane := 0; plane < templatePlanes; plane++ {
		for chunkX := 0; chunkX < tia.widthChunks; chunkX++ {
			for chunkY := 0; chunkY < tia.heightChunks; chunkY++ {
				tia.copyChunkSettings(settings, plane, chunkX, chunkY)
			}
		}
	}
	bridge := func(x, y int) bool {
		return settings[tia.tileIndex(x, y, 1)]&templateSettingBridge == templateSettingBridge
	}

	for plane := 0; plane < templatePlanes; plane++ {
		for x := tia.GetBaseX(); x < tia.GetBaseX()+width; x++ {
			for y := tia.GetBaseY(); y < tia.GetBaseY()+height; y++ {
				index := tia.tileIndex(x, y, plane)
				if _, ok := tia.GetChunk(plane, (x-tia.GetBaseX())>>3, (y-tia.GetBaseY())>>3); !ok {
					grid[index] |= templateUnmappedTile
					continue
				}
				if settings[index]&templateSettingBlocked == 0 {
					continue
				}
				level := plane
				if bridge(x, y) {
					level = plane - 1
				}
				if level >= 0 {
					grid[tia.tileIndex(x, y, level)] |= templateBlockedTile
				}
			}
		}
	}

	for plane := 0; plane < templatePlanes; plane++ {
		for chunkX := 0; chunkX < tia.widthChunks; chunkX++ {
			for chunkY := 0; chunkY < tia.heightChunks; chunkY++ {
				tia.placeChunkLocs(plane, chunkX, chunkY, bridge)
			}
		}
	}
	for _, locs := range tia.locIndex {
		for _, loc := range locs {
			tia.addObjectClipping(tia.objectFor(loc))
		}
	}
	// Spawned locs (resources, doors swapped at runtime) keep
	// their collision.
	for _, object := range tia.GetObjects() {
		tia.addObjectClipping(object)
	}
	return grid
}

func (tia *TemplatedInstanceArea) copyChunkSettings(settings []uint8, plane, chunkX, chunkY int) {
	chunk, ok := tia.GetChunk(plane, chunkX, chunkY)
	if !ok {
		return
	}
	region := GetTemplateRegion(regionOfChunk(chunk))
	if region == nil {
		return
	}
	sourceX := (chunk.SourceChunkX & 7) * 8
	sourceY := (chunk.SourceChunkY & 7) * 8
	for x := 0; x < 8; x++ {
		for y := 0; y < 8; y++ {
			toX, toY := RotateChunkTile(x, y, chunk.Rotation)
			value := region.Settings[(chunk.SourcePlane<<12)|((sourceX+x)<<6)|(sourceY+y)]
			settings[tia.tileIndex(tia.GetBaseX()+chunkX*8+toX, tia.GetBaseY()+chunkY*8+toY, plane)] = value
		}
	}
}

func (tia *TemplatedInstanceArea) placeChunkLocs(plane, chunkX, chunkY int, bridge func(x, y int) bool) {
	chunk, ok := tia.GetChunk(plane, chunkX, chunkY)
	if !ok {
		return
	}
	region := GetTemplateRegion(regionOfChunk(chunk))
	if region == nil {
		return
	}
	locs := region.Chunks[TemplateChunkKey(chunk.SourcePlane, chunk.SourceChunkX, chunk.SourceChunkY)]
	for _, loc := range locs {
		def, hasDef := templateObjectDef(loc.ID)
		sizeX, sizeY := 1, 1
		if hasDef {
			sizeX, sizeY = def.SizeX, def.SizeY
		}
		toX, toY := RotateChunkLoc(loc.LocalX&7, loc.LocalY&7, chunk.Rotation, sizeX, sizeY, loc.Face)
		x := tia.GetBaseX() + chunkX*8 + toX
		y := tia.GetBaseY() + chunkY*8 + toY
		// As in the main world, a loc over a bridge sits (and
		// collides) a plane lower.
		z := plane
		if tia.Contains(Location{X: x, Y: y, Z: 1}) && bridge(x, y) {
			z = plane - 1
		}
		if z < 0 || !tia.Contains(Location{X: x, Y: y, Z: z}) {
			continue
		}
		placed := &placedLoc{id: loc.ID, x: x, y: y, z: z, typ: loc.Type, face: (loc.Face + chunk.Rotation) & 3}
		index := tia.tileIndex(x, y, z)
		tia.locIndex[index] = append(tia.locIndex[index], placed)
	}
}

// addObjectClipping lays one object's clipping onto the grid:
// the RegionManager.addObjectClipping call TS makes with the
// object (whose private area routes the writes to this grid),
// expressed through the shared applyObjectClips core with a
// grid writer. Tiles outside the instance are skipped, as the
// templated SetClip ignores them. An id of -1 lays nothing
// (TS removes a zero shift: a no-op), and a missing definition
// lays nothing (TS returns early).
func (tia *TemplatedInstanceArea) addObjectClipping(obj *GameObject) {
	if obj == nil || obj.ID() == -1 {
		return
	}
	def, ok := templateObjectDef(obj.ID())
	if !ok {
		return
	}
	loc := obj.Location()
	placement := ObjectPlacement{ID: obj.ID(), X: loc.X, Y: loc.Y, Level: loc.Z, Type: obj.Type(), Direction: obj.Face()}
	grid := tia.clipGrid
	applyObjectClips(placement, def, true, func(x, y, level, shift int) {
		at := Location{X: x, Y: y, Z: level}
		if !tia.Contains(at) {
			return
		}
		grid[tia.tileIndex(x, y, level)] |= int32(shift)
	})
}

// Compile-time proof that a templated instance is a ClipStore:
// it installs itself as its PrivateArea's store.
var _ ClipStore = (*TemplatedInstanceArea)(nil)
