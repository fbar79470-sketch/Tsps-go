package game

// Collision: Region clip grids and RegionManager, ported from
// game/collision/Region.ts and game/collision/RegionManager.ts
// (upstream commit bfd81e3). Region data arrives through the
// RegionDataSource / ObjectDefSource seams in collision.go; this
// package never imports internal/cache.
//
// Thread-safety: TS RegionManager is static state on a single event
// loop. The Go manager is likewise tick-goroutine-only: the World
// owns it, movement and route-finding consult it from the tick, and
// no other goroutine may call its methods. The maps and clip grids
// are not synchronized. (This matches the visibility.go convention:
// tick-goroutine-only state is documented, not locked.)
//
// Deliberate collapses and omissions versus TS, each documented
// again at the site:
//
//   - Private-area clipping is threaded explicitly, as in TS:
//     every clip read/write, blocked check, canMove, and
//     object-clipping call has a nil-area form (GetClipping,
//     AddClipping, CanMove, ...) and a private-area form
//     (...InArea, taking a *PrivateArea), mirroring the
//     privateArea parameter TS threads through RegionManager
//     (RegionManager.ts:440-488, 521-580, 714-812) from
//     MovementQueue (character.getPrivateArea()) and the route
//     finder (FindRouteOptions.privateArea). The InArea forms
//     consult the area's clip store first (HasClip wins over
//     the region grid) and seed area writes from the region
//     clip, exactly as applyPrivateClipping does
//     (RegionManager.ts:429-470). The call sites that must pass
//     the actor's area — MovementQueue.canWalkTo and the route
//     finder — are NOT YET WIRED: movement_queue.go and
//     routefinding.go still call the nil-area forms, so
//     private-area collision is live only where a caller passes
//     the area explicitly (templated collision building,
//     LookupForArea). See the Phase 21 report for the wiring
//     snippets.
//   - No MapObjects / ObjectManager send paths: region loading
//     applies object clipping directly from the object stream
//     (see loadRegion in regions_decode.go) and stores the
//     decoded placements per region; the object registry imports
//     them through the Placements accessor (see objects.go).
//     MapObjects.add's only collision effect is
//     RegionManager.addObjectClipping (MapObjects.ts:229).
//     reloadRegion's clearRegionMapObjects step is expressed
//     through the load sequence (see RegionLoadSeq): the registry
//     drops the region's map-original entries when the sequence
//     changes. replayWorldObjects is otherwise omitted, except
//     the removed-object skip the registry performs on import.
//   - No MapRegionReplacementManager lookup inside loadRegion:
//     replacement-aware loading arrives through
//     ReplacementRegionSource, which wraps the RegionDataSource
//     the manager reads (see map_region_replacement.go); the
//     decoder itself always decodes the bytes its source
//     returns.
//   - No plugin event: PluginManager.emitRegionLoaded has no
//     counterpart (no plugin system).
//   - NOT STARTED: projectile methods (canProjectile*,
//     getProjectileClipping, canProjectileRay), wallExists /
//     wallsExist (need MapObjects), wasRemovedOnLoad (needs the
//     replay bookkeeping), sailing. Template regions and
//     private areas are ported (private_area.go,
//     templated_instance_area.go, template_region.go); the
//     movement/route-finder threading of the actor's area is
//     the remaining gap (see above).

// Clipping flag constants, quoted from RegionManager.ts:21-39.
// UNLOADED_TILE is never written into any clip grid: a repo-wide
// grep at commit bfd81e3 finds it only in masks (BLOCKED_MASK,
// the projectile ray flags, and CombatRange's invalid-tile check).
// A missing or unloaded region answers 0 from getClipping
// (RegionManager.ts:472-488), i.e. open ground, and this port
// preserves that: GetClipping returns 0, it never synthesizes
// UNLOADED_TILE blocking.
const (
	ProjectileNorthWestBlocked = 0x200
	ProjectileNorthBlocked     = 0x400
	ProjectileNorthEastBlocked = 0x800
	ProjectileEastBlocked      = 0x1000
	ProjectileSouthEastBlocked = 0x2000
	ProjectileSouthBlocked     = 0x4000
	ProjectileSouthWestBlocked = 0x8000
	ProjectileWestBlocked      = 0x10000
	ProjectileTileBlocked      = 0x20000
	// ClipUnknown is UNKNOWN (0x80000): an unclassified blocking flag
	// that fills a tile for walking.
	ClipUnknown = 0x80000
	// ClipBlockedTile is BLOCKED_TILE (0x200000): the flag the
	// terrain height map lays on blocked tiles.
	ClipBlockedTile = 0x200000
	// ClipUnloadedTile is UNLOADED_TILE (0x1000000). It appears only
	// in masks; see the note above.
	ClipUnloadedTile = 0x1000000
	// ClipOceanTile is OCEAN_TILE (2097152 == 0x200000, the same
	// value as BLOCKED_TILE, spelled as a decimal in TS).
	ClipOceanTile = 2097152
	// BlockedMask is BLOCKED_MASK (0x12c0100): the flags that fill
	// a tile for walking, as blocked() reads them — a solid loc
	// (0x100), a blocking floor decoration (0x40000), UNKNOWN,
	// BLOCKED_TILE and UNLOADED_TILE.
	BlockedMask = 0x12c0100
)

// tileFlagUnderRoof is Region.TILE_FLAG_UNDER_ROOF (Region.ts:10).
const tileFlagUnderRoof = 0x4

// clipGridSize is the Region clip grid: 4 planes of 64x64
// (Region.ts:49: new Uint32Array(4 * 64 * 64)).
const clipGridSize = 4 * 64 * 64

// region is one 64x64 Region (Region.ts). The terrain/object file
// ids TS keeps on the Region are not stored: region bytes arrive
// through RegionDataSource, keyed by region id alone.
type region struct {
	id int
	// clips is nil until the first addClip, as in TS (clips is an
	// optional Uint32Array allocated lazily by addClip, Region.ts:47-52).
	clips []uint32
	// roofTiles/waterTiles are nil until the region loads; TS assigns
	// fresh bitsets during loadMapFiles (RegionManager.ts:958,1004).
	roofTiles  []byte // 2048 bytes: one bit per tile per plane
	waterTiles []byte // 512 bytes: one bit per tile, plane 0 only
	loaded     bool
	// placements are the decoded object placements of the latest
	// load, in stream order (the addObject calls of loadMapFiles,
	// RegionManager.ts:975-1003). They exist so the object
	// registry can import cache-map objects without re-decoding
	// (see Placements); they carry no behavior of their own, and
	// clipping still comes from the decode-time AddObjectClipping
	// calls. A reload overwrites them.
	placements []ObjectPlacement
	// loadSeq counts successful loads of this region. It lets the
	// object registry tell a fresh load from the one it already
	// imported (see RegionLoadSeq); a reload that fails to load
	// leaves the sequence — and loaded=false — so the registry
	// drops the stale entries instead.
	loadSeq int
}

// clipIndex mirrors Region.clipIndex (Region.ts:31-39): -1 outside
// this region (masking coordinates would alias adjacent tiles), and
// an out-of-range height is clamped to 0 rather than rejected.
func (r *region) clipIndex(x, y, height int) int {
	localX := x - (r.id>>8)*64
	localY := y - (r.id&0xff)*64
	if localX < 0 || localX >= 64 || localY < 0 || localY >= 64 {
		return -1
	}
	if height < 0 || height >= 4 {
		height = 0
	}
	return (height << 12) | (localX << 6) | localY
}

// getClip mirrors Region.getClip (Region.ts:41-45), preserving the
// signed-32-bit masks via the int32 conversion (TS: (clips[index] ??
// 0) | 0).
func (r *region) getClip(x, y, height int) int32 {
	index := r.clipIndex(x, y, height)
	if index < 0 || r.clips == nil {
		return 0
	}
	return int32(r.clips[index])
}

// addClip mirrors Region.addClip (Region.ts:47-52). The shift passes
// through int32, as TS bitwise OR converts its operand to a signed
// 32-bit integer before storing it in the Uint32Array.
func (r *region) addClip(x, y, height, shift int) {
	index := r.clipIndex(x, y, height)
	if index < 0 {
		return
	}
	if r.clips == nil {
		r.clips = make([]uint32, clipGridSize)
	}
	r.clips[index] |= uint32(int32(shift))
}

// removeClip mirrors Region.removeClip (Region.ts:54-58).
func (r *region) removeClip(x, y, height, shift int) {
	index := r.clipIndex(x, y, height)
	if index < 0 || r.clips == nil {
		return
	}
	r.clips[index] &^= uint32(int32(shift))
}

// isUnderRoof mirrors Region.isUnderRoof (Region.ts:60-64). Unlike
// clipIndex it masks coordinates with & 63 (aliasing across region
// boundaries, as in TS) and rejects out-of-range planes instead of
// clamping them.
func (r *region) isUnderRoof(x, y, plane int) bool {
	if plane < 0 || plane > 3 {
		return false
	}
	index := (plane << 12) | ((x & 63) << 6) | (y & 63)
	if r.roofTiles == nil {
		return false
	}
	return (r.roofTiles[index>>3] & (1 << (index & 7))) != 0
}

// isWater mirrors Region.isWater (Region.ts:66-70): plane 0 only,
// coordinates masked with & 63 as in TS.
func (r *region) isWater(x, y, plane int) bool {
	if plane != 0 {
		return false
	}
	index := ((x & 63) << 6) | (y & 63)
	if r.waterTiles == nil {
		return false
	}
	return (r.waterTiles[index>>3] & (1 << (index & 7))) != 0
}

// RegionManager is the Go form of the static RegionManager class
// (RegionManager.ts). TS init() indexes every cache region id into
// the regions map (RegionManager.ts:46-52, plus the replacement-dir
// scan which is omitted — see the header); NewRegionManager does
// the indexing, and ObjectDefinition.init() has no counterpart
// because definitions are loaded at boot by cmd/server.
type RegionManager struct {
	regions map[int]*region
	// loading is the loadingRegions guard (RegionManager.ts:42):
	// region ids whose load is in progress. It also suppresses
	// nested loads from getRegion while any load runs.
	loading map[int]bool
	source  RegionDataSource
	objects ObjectDefSource
	// newTerrainFormat is CachePipeline.getActive().revision >= 209
	// (RegionManager.ts:924), supplied by the caller: true for the
	// rev-242 target.
	newTerrainFormat bool
}

// NewRegionManager indexes every region id the source holds, as
// RegionManager.init does. A nil RegionDataSource means no region
// ever loads: GetClipping answers 0 everywhere (open ground,
// matching TS missing-region semantics). A nil ObjectDefSource
// means object decoding lays no clipping. Both are tolerated so a
// NoCache server still boots with an empty, open manager.
func NewRegionManager(regions RegionDataSource, objects ObjectDefSource, newTerrainFormat bool) *RegionManager {
	m := &RegionManager{
		regions:          make(map[int]*region),
		loading:          make(map[int]bool),
		source:           regions,
		objects:          objects,
		newTerrainFormat: newTerrainFormat,
	}
	if regions != nil {
		for _, id := range regions.RegionIDs() {
			m.regions[id] = &region{id: id}
		}
	}
	return m
}

// RegionIDForTile mirrors RegionManager.regionIdForTile
// (RegionManager.ts:96-98).
func RegionIDForTile(x, y int) int {
	return ((x >> 6) << 8) + (y >> 6)
}

// regionFor is the no-load lookup: RegionManager.getRegionid
// (RegionManager.ts:92-94).
func (m *RegionManager) regionFor(x, y int) *region {
	return m.regions[RegionIDForTile(x, y)]
}

// ensureRegion mirrors RegionManager.getRegion
// (RegionManager.ts:100-106): it triggers a load only when no load
// is already in progress, then returns the indexed region (which
// may still be unloaded when the source has no data for it).
func (m *RegionManager) ensureRegion(x, y int) *region {
	if len(m.loading) == 0 {
		m.loadMapFiles(x, y, true)
	}
	return m.regionFor(x, y)
}

// GetClipping mirrors RegionManager.getClipping
// (RegionManager.ts:472-488) with a null private area: the region
// clip, or 0 when no region is indexed for the tile. It never
// synthesizes UNLOADED_TILE blocking; see the constant note above.
func (m *RegionManager) GetClipping(x, y, level int) int32 {
	return m.getClipping(x, y, level, nil)
}

// GetClippingInArea mirrors RegionManager.getClipping with a
// private area (RegionManager.ts:472-488): when the area has a
// clip opinion at the tile (HasClip), the area's clip wins over
// the region grid; otherwise the region clip answers (0 when no
// region is indexed). A nil area behaves exactly like
// GetClipping. This is the consultation point every other InArea
// method funnels through, as TS funnels through getClipping.
func (m *RegionManager) GetClippingInArea(x, y, level int, area *PrivateArea) int32 {
	return m.getClipping(x, y, level, area)
}

// getClipping is the shared clip read. The TS duck-type guard
// (applyPrivateClipping checks getClip is a function,
// RegionManager.ts:433) has no Go form: a non-nil *PrivateArea
// always carries the clip methods.
func (m *RegionManager) getClipping(x, y, level int, area *PrivateArea) int32 {
	if area != nil {
		loc := Location{X: x, Y: y, Z: level}
		if area.HasClip(loc) {
			return area.GetClip(loc)
		}
	}
	r := m.ensureRegion(x, y)
	if r == nil {
		return 0
	}
	return r.getClip(x, y, level)
}

// AddClipping mirrors RegionManager.addClipping
// (RegionManager.ts:440-454) with a null private area: the clip
// lands on the region grid.
func (m *RegionManager) AddClipping(x, y, level, shift int) {
	m.addClipping(x, y, level, shift, nil)
}

// AddClippingInArea mirrors RegionManager.addClipping with a
// private area (RegionManager.ts:440-454, via
// applyPrivateClipping): the write lands ONLY on the area's
// clip store, never on the region grid. When the area has no
// opinion at the tile yet, the write seeds from the region clip
// (the current grid value, or 0 when no region is indexed) and
// ORs the shift in; when it already has one, the shift ORs onto
// the stored value. A nil area behaves exactly like AddClipping.
func (m *RegionManager) AddClippingInArea(x, y, level, shift int, area *PrivateArea) {
	m.addClipping(x, y, level, shift, area)
}

// addClipping is the shared clip write. The TS optional region
// parameter (used by the region decoder to write to a region
// under load) has no Go form: the Go decoder writes to the
// region directly (see regions_decode.go), and this method
// always resolves the region itself.
func (m *RegionManager) addClipping(x, y, level, shift int, area *PrivateArea) {
	if area != nil {
		loc := Location{X: x, Y: y, Z: level}
		current := m.areaClipSeed(x, y, level, area, loc)
		area.SetClip(loc, current|int32(shift))
		return
	}
	if r := m.ensureRegion(x, y); r != nil {
		r.addClip(x, y, level, shift)
	}
}

// RemoveClipping mirrors RegionManager.removeClipping
// (RegionManager.ts:456-470) with a null private area.
func (m *RegionManager) RemoveClipping(x, y, level, shift int) {
	m.removeClipping(x, y, level, shift, nil)
}

// RemoveClippingInArea mirrors RegionManager.removeClipping
// with a private area (RegionManager.ts:456-470): the strip
// lands ONLY on the area's clip store, seeded from the region
// clip when the area has no opinion at the tile yet (see
// AddClippingInArea). A nil area behaves exactly like
// RemoveClipping.
func (m *RegionManager) RemoveClippingInArea(x, y, level, shift int, area *PrivateArea) {
	m.removeClipping(x, y, level, shift, area)
}

func (m *RegionManager) removeClipping(x, y, level, shift int, area *PrivateArea) {
	if area != nil {
		loc := Location{X: x, Y: y, Z: level}
		current := m.areaClipSeed(x, y, level, area, loc)
		area.SetClip(loc, current&^int32(shift))
		return
	}
	if r := m.ensureRegion(x, y); r != nil {
		r.removeClip(x, y, level, shift)
	}
}

// areaClipSeed answers the base value an area clip write builds
// on (RegionManager.ts:443-445, 459-461): the area's stored
// clip when it has one, else the region grid's clip (0 when no
// region is indexed). The region lookup may trigger a load, as
// TS getRegion does.
func (m *RegionManager) areaClipSeed(x, y, level int, area *PrivateArea, loc Location) int32 {
	if area.HasClip(loc) {
		return area.GetClip(loc)
	}
	if r := m.ensureRegion(x, y); r != nil {
		return r.getClip(x, y, level)
	}
	return 0
}

// IsWater mirrors RegionManager.isWater (RegionManager.ts:108-111).
func (m *RegionManager) IsWater(pos Location) bool {
	r := m.ensureRegion(pos.X, pos.Y)
	return r != nil && r.isWater(pos.X, pos.Y, pos.Z)
}

// LoadRegion loads the region containing tile (x, y) plus its 8
// surroundings, as loadMapFiles does with loadSurroundings=true
// (RegionManager.ts:885). Loading an already-loaded region, or a
// tile with no indexed region, is a no-op.
func (m *RegionManager) LoadRegion(x, y int) {
	m.loadMapFiles(x, y, true)
}

// IsRegionLoaded reports whether regionID is indexed and its map
// files have loaded (Region.isLoaded, Region.ts:82-84).
func (m *RegionManager) IsRegionLoaded(regionID int) bool {
	r, ok := m.regions[regionID]
	return ok && r.loaded
}

// Placements returns a copy of the decoded object placements of
// the latest load of regionID, in stream order: the Go form of
// the addObject calls loadMapFiles makes while decoding the
// object stream (RegionManager.ts:975-1003), surfaced so the
// object registry can populate its tile map from cache-map data
// (see ObjectRegistry in objects.go). It reports ok=false when
// no region is indexed for the id or the region has not loaded.
// The placements carry the decode-time plane adjustment (bridge
// tiles drop one plane), exactly as the clipping calls saw them.
// This accessor changes no collision behavior: clipping is laid
// during the load itself, and placements are inert data.
func (m *RegionManager) Placements(regionID int) ([]ObjectPlacement, bool) {
	r, ok := m.regions[regionID]
	if !ok || !r.loaded {
		return nil, false
	}
	if len(r.placements) == 0 {
		return nil, true
	}
	return append([]ObjectPlacement(nil), r.placements...), true
}

// RegionLoadSeq returns the number of successful loads of
// regionID (0 when no region is indexed for the id). The object
// registry compares it against the sequence it imported to
// detect reloads: a changed sequence means the region's
// map-original entries must be dropped and re-imported. It is
// bookkeeping for the Placements accessor and carries no
// collision meaning.
func (m *RegionManager) RegionLoadSeq(regionID int) int {
	r, ok := m.regions[regionID]
	if !ok {
		return 0
	}
	return r.loadSeq
}

// RegionCount is the number of indexed regions (the regions map
// size after init).
func (m *RegionManager) RegionCount() int {
	return len(m.regions)
}

// Lookup returns the CollisionLookup view of this manager: the
// function form of getClipping that the route finder reads through
// (see collision.go).
func (m *RegionManager) Lookup() CollisionLookup {
	return func(x, y, level int) int32 {
		return m.GetClipping(x, y, level)
	}
}

// LookupForArea returns the CollisionLookup view of this manager
// for one private area: the function form of getClipping with
// that area, for route-finding inside an instance (the
// CollisionLookup type in RsmodRouteFinding.ts takes the
// private area as its fourth argument; Go bakes it into the
// closure). A nil area behaves exactly like Lookup. The area is
// fixed at construction: callers whose actor changes areas must
// build a fresh lookup (or a closure over the actor's current
// PrivateArea — see the Phase 21 report).
func (m *RegionManager) LookupForArea(area *PrivateArea) CollisionLookup {
	return func(x, y, level int) int32 {
		return m.GetClippingInArea(x, y, level, area)
	}
}

// LookupForActor returns the CollisionLookup view of this manager
// for one actor: the function form of getClipping with the
// actor's CURRENT private area, read fresh on every call.
// MovementQueue passes character.getPrivateArea() at every call
// site (RegionManager.ts:440-488; MovementQueue.ts:673-681), and
// route computation is synchronous on one goroutine, so the
// dynamic read is equivalent to threading the area through the
// call. A nil area function behaves exactly like Lookup. The
// wired installs (cmd/server players, World NPCs) use this form
// so an actor that boards a boat or enters an instance routes on
// the area's clips without reinstalling the lookup.
func (m *RegionManager) LookupForActor(area func() *PrivateArea) CollisionLookup {
	return func(x, y, level int) int32 {
		var pa *PrivateArea
		if area != nil {
			pa = area()
		}
		return m.GetClippingInArea(x, y, level, pa)
	}
}

// ReloadRegion mirrors RegionManager.reloadRegion
// (RegionManager.ts:867-883): drop the region's clips, roof/water
// tiles and loaded flag, then load it again (with surroundings).
// The load sequence advances on the fresh load, so the object
// registry drops the region's old map-original entries and
// re-imports on its next lookup (the clearRegionMapObjects step,
// RegionManager.ts:814-824, expressed through the sequence);
// runtime-added clipping on the region is dropped along with the
// loaded clips, as in TS (clips = undefined). It reports whether
// the region loaded.
func (m *RegionManager) ReloadRegion(regionID int) bool {
	r, ok := m.regions[regionID]
	if !ok {
		return false
	}
	delete(m.loading, regionID)
	r.clips = nil
	r.roofTiles = nil
	r.waterTiles = nil
	r.loaded = false
	absX := ((regionID >> 8) & 0xff) * 64
	absY := (regionID & 0xff) * 64
	m.loadMapFiles(absX, absY, true)
	return r.loaded
}

// loadMapFiles mirrors RegionManager.loadMapFiles
// (RegionManager.ts:885-1031) reduced to the cache path: the
// replacement lookup, replayWorldObjects, and the plugin event are
// omitted (see the header); the decode itself lives in
// regions_decode.go. The surroundings fan-out loads the 8
// neighbors with loadSurroundings=false (lines 1014-1028).
func (m *RegionManager) loadMapFiles(x, y int, surroundings bool) {
	regionID := RegionIDForTile(x, y)
	r := m.regions[regionID]
	if r == nil || m.loading[regionID] || r.loaded {
		return
	}
	m.loading[regionID] = true
	m.loadRegion(r)
	delete(m.loading, regionID)
	if surroundings {
		mapX := regionID >> 8
		mapY := regionID & 0xff
		for deltaX := -1; deltaX <= 1; deltaX++ {
			for deltaY := -1; deltaY <= 1; deltaY++ {
				if deltaX != 0 || deltaY != 0 {
					m.loadMapFiles((mapX+deltaX)*64, (mapY+deltaY)*64, false)
				}
			}
		}
	}
}

// Blocked mirrors RegionManager.blocked (RegionManager.ts:547-549):
// the tile's own clip against BLOCKED_MASK, with a null private
// area.
func (m *RegionManager) Blocked(pos Location) bool {
	return m.blocked(pos, nil)
}

// BlockedInArea mirrors RegionManager.blocked with a private
// area: the area-aware clip against BLOCKED_MASK. A nil area
// behaves exactly like Blocked.
func (m *RegionManager) BlockedInArea(pos Location, area *PrivateArea) bool {
	return m.blocked(pos, area)
}

func (m *RegionManager) blocked(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X, pos.Y, pos.Z, area) & BlockedMask) != 0
}

// The eight directional helpers mirror RegionManager.blockedNorth
// et al. (RegionManager.ts:551-580). Each reads the ADJACENT tile in
// the named direction and tests it against the exact TS mask; the
// masks are quoted verbatim, including the 0x12c0000 high bits
// (UNLOADED_TILE | BLOCKED_TILE | UNKNOWN | floor decoration) that
// make any filled tile block from every side. The nil-area forms
// keep the original names; the InArea forms take the private area
// TS threads through every one of these calls.
func (m *RegionManager) BlockedNorth(pos Location) bool {
	return m.blockedNorth(pos, nil)
}

// BlockedNorthInArea mirrors RegionManager.blockedNorth with a
// private area. A nil area behaves exactly like BlockedNorth.
func (m *RegionManager) BlockedNorthInArea(pos Location, area *PrivateArea) bool {
	return m.blockedNorth(pos, area)
}

func (m *RegionManager) blockedNorth(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X, pos.Y+1, pos.Z, area) & 0x12c0120) != 0
}

func (m *RegionManager) BlockedEast(pos Location) bool {
	return m.blockedEast(pos, nil)
}

// BlockedEastInArea mirrors RegionManager.blockedEast with a
// private area. A nil area behaves exactly like BlockedEast.
func (m *RegionManager) BlockedEastInArea(pos Location, area *PrivateArea) bool {
	return m.blockedEast(pos, area)
}

func (m *RegionManager) blockedEast(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X+1, pos.Y, pos.Z, area) & 0x12c0180) != 0
}

func (m *RegionManager) BlockedSouth(pos Location) bool {
	return m.blockedSouth(pos, nil)
}

// BlockedSouthInArea mirrors RegionManager.blockedSouth with a
// private area. A nil area behaves exactly like BlockedSouth.
func (m *RegionManager) BlockedSouthInArea(pos Location, area *PrivateArea) bool {
	return m.blockedSouth(pos, area)
}

func (m *RegionManager) blockedSouth(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X, pos.Y-1, pos.Z, area) & 0x12c0102) != 0
}

func (m *RegionManager) BlockedWest(pos Location) bool {
	return m.blockedWest(pos, nil)
}

// BlockedWestInArea mirrors RegionManager.blockedWest with a
// private area. A nil area behaves exactly like BlockedWest.
func (m *RegionManager) BlockedWestInArea(pos Location, area *PrivateArea) bool {
	return m.blockedWest(pos, area)
}

func (m *RegionManager) blockedWest(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X-1, pos.Y, pos.Z, area) & 0x12c0108) != 0
}

func (m *RegionManager) BlockedNorthEast(pos Location) bool {
	return m.blockedNorthEast(pos, nil)
}

// BlockedNorthEastInArea mirrors RegionManager.blockedNorthEast
// with a private area. A nil area behaves exactly like
// BlockedNorthEast.
func (m *RegionManager) BlockedNorthEastInArea(pos Location, area *PrivateArea) bool {
	return m.blockedNorthEast(pos, area)
}

func (m *RegionManager) blockedNorthEast(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X+1, pos.Y+1, pos.Z, area) & 0x12c01e0) != 0
}

func (m *RegionManager) BlockedNorthWest(pos Location) bool {
	return m.blockedNorthWest(pos, nil)
}

// BlockedNorthWestInArea mirrors RegionManager.blockedNorthWest
// with a private area. A nil area behaves exactly like
// BlockedNorthWest.
func (m *RegionManager) BlockedNorthWestInArea(pos Location, area *PrivateArea) bool {
	return m.blockedNorthWest(pos, area)
}

func (m *RegionManager) blockedNorthWest(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X-1, pos.Y+1, pos.Z, area) & 0x12c0138) != 0
}

func (m *RegionManager) BlockedSouthEast(pos Location) bool {
	return m.blockedSouthEast(pos, nil)
}

// BlockedSouthEastInArea mirrors RegionManager.blockedSouthEast
// with a private area. A nil area behaves exactly like
// BlockedSouthEast.
func (m *RegionManager) BlockedSouthEastInArea(pos Location, area *PrivateArea) bool {
	return m.blockedSouthEast(pos, area)
}

func (m *RegionManager) blockedSouthEast(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X+1, pos.Y-1, pos.Z, area) & 0x12c0183) != 0
}

func (m *RegionManager) BlockedSouthWest(pos Location) bool {
	return m.blockedSouthWest(pos, nil)
}

// BlockedSouthWestInArea mirrors RegionManager.blockedSouthWest
// with a private area. A nil area behaves exactly like
// BlockedSouthWest.
func (m *RegionManager) BlockedSouthWestInArea(pos Location, area *PrivateArea) bool {
	return m.blockedSouthWest(pos, area)
}

func (m *RegionManager) blockedSouthWest(pos Location, area *PrivateArea) bool {
	return (m.getClipping(pos.X-1, pos.Y-1, pos.Z, area) & 0x12c010e) != 0
}

// CanMoveIsBlocked mirrors RegionManager.canMoveIsBlocked
// (RegionManager.ts:521-540). Despite the name it reports whether
// movement in the given direction is POSSIBLE: it returns true when
// the relevant blocked* helpers are all false. Direction ids are
// the TS order: 0 NW, 1 N, 2 NE, 3 W, 4 E, 5 SW, 6 S, 7 SE; any
// other direction returns false. Diagonal directions require the
// diagonal and both axis steps to be clear. The private area is
// null.
func (m *RegionManager) CanMoveIsBlocked(pos Location, direction int) bool {
	return m.canMoveIsBlocked(pos, direction, nil)
}

// CanMoveIsBlockedInArea mirrors RegionManager.canMoveIsBlocked
// with a private area. A nil area behaves exactly like
// CanMoveIsBlocked.
func (m *RegionManager) CanMoveIsBlockedInArea(pos Location, direction int, area *PrivateArea) bool {
	return m.canMoveIsBlocked(pos, direction, area)
}

func (m *RegionManager) canMoveIsBlocked(pos Location, direction int, area *PrivateArea) bool {
	switch direction {
	case 0:
		return !m.blockedNorthWest(pos, area) && !m.blockedNorth(pos, area) && !m.blockedWest(pos, area)
	case 1:
		return !m.blockedNorth(pos, area)
	case 2:
		return !m.blockedNorthEast(pos, area) && !m.blockedNorth(pos, area) && !m.blockedEast(pos, area)
	case 3:
		return !m.blockedWest(pos, area)
	case 4:
		return !m.blockedEast(pos, area)
	case 5:
		return !m.blockedSouthWest(pos, area) && !m.blockedSouth(pos, area) && !m.blockedWest(pos, area)
	case 6:
		return !m.blockedSouth(pos, area)
	case 7:
		return !m.blockedSouthEast(pos, area) && !m.blockedSouth(pos, area) && !m.blockedEast(pos, area)
	}
	return false
}

// CanMove mirrors RegionManager.canMove (RegionManager.ts:714-783)
// with a null private area: it walks from the start toward the end
// one tile at a time, testing the tiles the actor's footprint would
// newly occupy at each step. The masks below are quoted verbatim
// from lines 728-757.
func (m *RegionManager) CanMove(startX, startY, endX, endY, level, xLength, yLength int) bool {
	return m.canMove(startX, startY, endX, endY, level, xLength, yLength, nil)
}

// CanMoveInArea mirrors RegionManager.canMove with a private
// area. A nil area behaves exactly like CanMove.
func (m *RegionManager) CanMoveInArea(startX, startY, endX, endY, level, xLength, yLength int, area *PrivateArea) bool {
	return m.canMove(startX, startY, endX, endY, level, xLength, yLength, area)
}

func (m *RegionManager) canMove(startX, startY, endX, endY, level, xLength, yLength int, area *PrivateArea) bool {
	diffX := endX - startX
	diffY := endY - startY
	max := absInt(diffX)
	if absInt(diffY) > max {
		max = absInt(diffY)
	}
	for ii := 0; ii < max; ii++ {
		currentX := endX - diffX
		currentY := endY - diffY
		if (xLength > 1 || yLength > 1) && diffX != 0 && diffY != 0 {
			if !m.canStepDiagonally(currentX, currentY, signInt(diffX), signInt(diffY), level, xLength, yLength, area) {
				return false
			}
		} else {
			for i := 0; i < xLength; i++ {
				for i2 := 0; i2 < yLength; i2++ {
					if diffX < 0 && diffY < 0 {
						if (m.getClipping((currentX+i)-1, (currentY+i2)-1, level, area)&0x12c010e) != 0 ||
							(m.getClipping((currentX+i)-1, currentY+i2, level, area)&0x12c0108) != 0 ||
							(m.getClipping(currentX+i, (currentY+i2)-1, level, area)&0x12c0102) != 0 {
							return false
						}
					} else if diffX > 0 && diffY > 0 {
						if (m.getClipping(currentX+i+1, currentY+i2+1, level, area)&0x12c01e0) != 0 ||
							(m.getClipping(currentX+i+1, currentY+i2, level, area)&0x12c0180) != 0 ||
							(m.getClipping(currentX+i, currentY+i2+1, level, area)&0x12c0120) != 0 {
							return false
						}
					} else if diffX < 0 && diffY > 0 {
						if (m.getClipping((currentX+i)-1, currentY+i2+1, level, area)&0x12c0138) != 0 ||
							(m.getClipping((currentX+i)-1, currentY+i2, level, area)&0x12c0108) != 0 ||
							(m.getClipping(currentX+i, currentY+i2+1, level, area)&0x12c0120) != 0 {
							return false
						}
					} else if diffX > 0 && diffY < 0 {
						if (m.getClipping(currentX+i+1, (currentY+i2)-1, level, area)&0x12c0183) != 0 ||
							(m.getClipping(currentX+i+1, currentY+i2, level, area)&0x12c0180) != 0 ||
							(m.getClipping(currentX+i, (currentY+i2)-1, level, area)&0x12c0102) != 0 {
							return false
						}
					} else if diffX > 0 && diffY == 0 {
						if (m.getClipping(currentX+i+1, currentY+i2, level, area) & 0x12c0180) != 0 {
							return false
						}
					} else if diffX < 0 && diffY == 0 {
						if (m.getClipping((currentX+i)-1, currentY+i2, level, area) & 0x12c0108) != 0 {
							return false
						}
					} else if diffX == 0 && diffY > 0 {
						if (m.getClipping(currentX+i, currentY+i2+1, level, area) & 0x12c0120) != 0 {
							return false
						}
					} else if diffX == 0 && diffY < 0 &&
						(m.getClipping(currentX+i, (currentY+i2)-1, level, area)&0x12c0102) != 0 {
						return false
					}
				}
			}
		}
		if diffX < 0 {
			diffX++
		} else if diffX > 0 {
			diffX--
		}
		if diffY < 0 {
			diffY++
		} else if diffY > 0 {
			diffY--
		}
	}
	return true
}

// canStepDiagonally mirrors RegionManager.canStepDiagonally
// (RegionManager.ts:785-803): one diagonal step of an actor larger
// than 1x1 from its south-west tile (x, y). Only the tiles it steps
// into are checked — the new corner, the new column and the new
// row — as OSRS's route finder does; checking beside every tile of
// the old footprint as well refused diagonal steps the route finder
// had allowed (see the TS comment). Masks quoted verbatim from
// lines 790-792.
func (m *RegionManager) canStepDiagonally(x, y, dx, dy, level, xLength, yLength int, area *PrivateArea) bool {
	blocked := func(tileX, tileY int, mask int32) bool {
		return (m.getClipping(tileX, tileY, level, area) & mask) != 0
	}
	var corner int32
	if dx < 0 {
		if dy > 0 {
			corner = 0x12c0138
		} else {
			corner = 0x12c010e
		}
	} else {
		if dy > 0 {
			corner = 0x12c01e0
		} else {
			corner = 0x12c0183
		}
	}
	var column int32
	if dx < 0 {
		column = 0x12c013e
	} else {
		column = 0x12c01e3
	}
	var row int32
	if dy > 0 {
		row = 0x12c01f8
	} else {
		row = 0x12c018f
	}
	columnX := x + xLength
	if dx < 0 {
		columnX = x - 1
	}
	rowY := y - 1
	if dy > 0 {
		rowY = y + yLength
	}
	if blocked(columnX, rowY, corner) {
		return false
	}
	for i := 1; i < yLength; i++ {
		tileY := y + i - 1
		if dy > 0 {
			tileY = y + i
		}
		if blocked(columnX, tileY, column) {
			return false
		}
	}
	for i := 1; i < xLength; i++ {
		tileX := x + i
		if dx < 0 {
			tileX = x + i - 1
		}
		if blocked(tileX, rowY, row) {
			return false
		}
	}
	return true
}

// CanMoveStart mirrors RegionManager.canMovestart
// (RegionManager.ts:805-807) as MovementQueue calls it: always
// with size x size, on the start tile's level, with a null
// private area. This is the MovementCollision method (see
// collision.go).
func (m *RegionManager) CanMoveStart(start, end Location, size int) bool {
	return m.CanMove(start.X, start.Y, end.X, end.Y, start.Z, size, size)
}

// CanMoveStartInArea mirrors RegionManager.canMovestart with a
// private area — the form MovementQueue.canWalkTo calls, with
// character.getPrivateArea() (MovementQueue.ts:673-681). A nil
// area behaves exactly like CanMoveStart.
func (m *RegionManager) CanMoveStartInArea(start, end Location, size int, area *PrivateArea) bool {
	return m.CanMoveInArea(start.X, start.Y, end.X, end.Y, start.Z, size, size, area)
}

// areaCollision is a MovementCollision surface bound to one
// actor's current private area: every step validation reads the
// actor's area fresh (see LookupForActor). It exists because
// MovementCollision.CanMoveStart takes no area argument, while
// TS threads character.getPrivateArea() through every
// canMovestart call (MovementQueue.ts:673-681).
type areaCollision struct {
	manager *RegionManager
	area    func() *PrivateArea
}

func (c *areaCollision) CanMoveStart(start, end Location, size int) bool {
	var pa *PrivateArea
	if c.area != nil {
		pa = c.area()
	}
	return c.manager.CanMoveStartInArea(start, end, size, pa)
}

// CollisionForActor returns the MovementCollision surface for one
// actor: step validation against the actor's current private
// area, read fresh on every call. A nil area function behaves
// exactly like the manager itself. The wired installs (cmd/server
// players, World NPCs) use this form alongside LookupForActor.
func (m *RegionManager) CollisionForActor(area func() *PrivateArea) MovementCollision {
	return &areaCollision{manager: m, area: area}
}

// CanMovePosition mirrors RegionManager.canMoveposition
// (RegionManager.ts:809-812): one step from position in direction,
// for an actor of the given size, with a null private area.
// Location.Add preserves z, as TS transform does (the z-preserving
// form movement uses; see PORT_STATUS.md Game notes).
func (m *RegionManager) CanMovePosition(pos Location, dir Direction, size int) bool {
	end := pos.Add(dir.Dx(), dir.Dy())
	return m.CanMove(pos.X, pos.Y, end.X, end.Y, pos.Z, size, size)
}

// CanMovePositionInArea mirrors RegionManager.canMoveposition
// with a private area. A nil area behaves exactly like
// CanMovePosition.
func (m *RegionManager) CanMovePositionInArea(pos Location, dir Direction, size int, area *PrivateArea) bool {
	end := pos.Add(dir.Dx(), dir.Dy())
	return m.CanMoveInArea(pos.X, pos.Y, end.X, end.Y, pos.Z, size, size, area)
}

// absInt and signInt are the integer forms of Math.abs / Math.sign
// as canMove uses them.
func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func signInt(v int) int {
	if v < 0 {
		return -1
	}
	if v > 0 {
		return 1
	}
	return 0
}

// clipOffset is one cell of a variable object's clip footprint: the
// offset from the object's tile and the mask laid there.
type clipOffset struct {
	dx, dy, mask int
}

// variableWalkClips is the walk-flag table of
// addClippingForVariableObject / removeClippingForVariableObject
// (RegionManager.ts:112-210 and 211-282). The first index is the
// type group: 0 for wall type 0, 1 for types 1 and 3, 2 for wall
// type 2; the second is the direction 0-3. Entries are in TS order.
var variableWalkClips = [3][4][]clipOffset{
	{
		{{0, 0, 128}, {-1, 0, 8}},
		{{0, 0, 2}, {0, 1, 32}},
		{{0, 0, 8}, {1, 0, 128}},
		{{0, 0, 32}, {0, -1, 2}},
	},
	{
		{{0, 0, 1}, {-1, 1, 16}},
		{{0, 0, 4}, {1, 1, 64}},
		{{0, 0, 16}, {1, -1, 1}},
		{{0, 0, 64}, {-1, -1, 4}},
	},
	{
		{{0, 0, 130}, {-1, 0, 8}, {0, 1, 32}},
		{{0, 0, 10}, {0, 1, 32}, {1, 0, 128}},
		{{0, 0, 40}, {1, 0, 128}, {0, -1, 2}},
		{{0, 0, 160}, {0, -1, 2}, {-1, 0, 8}},
	},
}

// variableTallClips is the projectile-flag table for tall variable
// objects (RegionManager.ts:168-210, the `if (tall)` block of
// addClippingForVariableObject). Same indexing as variableWalkClips.
var variableTallClips = [3][4][]clipOffset{
	{
		{{0, 0, 65536}, {-1, 0, 4096}},
		{{0, 0, 1024}, {0, 1, 16384}},
		{{0, 0, 4096}, {1, 0, 65536}},
		{{0, 0, 16384}, {0, -1, 1024}},
	},
	{
		{{0, 0, 512}, {-1, 1, 8192}},
		{{0, 0, 2048}, {1, 1, 32768}},
		{{0, 0, 8192}, {1, -1, 512}},
		{{0, 0, 32768}, {-1, -1, 2048}},
	},
	{
		{{0, 0, 66560}, {-1, 0, 4096}, {0, 1, 16384}},
		{{0, 0, 5120}, {0, 1, 16384}, {1, 0, 65536}},
		{{0, 0, 20480}, {1, 0, 65536}, {0, -1, 1024}},
		{{0, 0, 81920}, {0, -1, 1024}, {-1, 0, 4096}},
	},
}

// variableGroup maps a wall type to its clip-table group, or -1
// when the type lays no variable clipping. Types 1 and 3 share a
// group in TS (the `type == 1 || type == 3` branches).
func variableGroup(typ int) int {
	switch typ {
	case 0:
		return 0
	case 1, 3:
		return 1
	case 2:
		return 2
	}
	return -1
}

// applyVariableClipsTo writes one clip table's cells through
// write. Directions outside 0-3 match no TS branch and write
// nothing.
func applyVariableClipsTo(table [3][4][]clipOffset, x, y, height, typ, direction int, write func(x, y, level, shift int)) {
	group := variableGroup(typ)
	if group < 0 || direction < 0 || direction > 3 {
		return
	}
	for _, c := range table[group][direction] {
		write(x+c.dx, y+c.dy, height, c.mask)
	}
}

// applyVariableObjectClips writes a variable object's clipping
// through write: walk flags always, projectile flags per the
// add/remove asymmetry of addClippingForVariableObject /
// removeClippingForVariableObject (RegionManager.ts:112-311).
// On add, the projectile flags apply only when the object is
// tall (impenetrable). On remove, the type-0 projectile removal
// is guarded by tall, but the type-1/3 and type-2 projectile
// removals run UNCONDITIONALLY — removing a non-tall corner or
// straight wall strips projectile flags too. That is preserved
// verbatim; it is an upstream quirk, not a transcription error.
func applyVariableObjectClips(x, y, height, typ, direction int, tall, add bool, write func(x, y, level, shift int)) {
	applyVariableClipsTo(variableWalkClips, x, y, height, typ, direction, write)
	if add {
		if tall {
			applyVariableClipsTo(variableTallClips, x, y, height, typ, direction, write)
		}
		return
	}
	if (tall && typ == 0) || typ == 1 || typ == 2 || typ == 3 {
		applyVariableClipsTo(variableTallClips, x, y, height, typ, direction, write)
	}
}

// applyObjectClips dispatches one object's clipping through
// write: the shared core of addObjectClipping and
// removeObjectClipping (RegionManager.ts:346-427), factored so
// the templated-instance grid builder can lay the same clipping
// onto its grid without a RegionManager. Type 22 lays 0x40000
// when the definition is a clipped decoration (clipType == 1,
// ObjectDefinition.ts:76-78); types 9-11 lay solid clipping
// when the definition is solid (clipType != 0,
// ObjectDefinition.ts:124); types 0-3 lay variable clipping
// when solid; all other types lay nothing. add selects the
// variable-object tall-flag behavior (see
// applyVariableObjectClips); write receives the cells to lay
// (add) or strip (remove). The id -1 case is the caller's: TS
// removes a zero shift for it, a no-op on every grid, handled
// at the call sites.
func applyObjectClips(obj ObjectPlacement, def ObjectDef, add bool, write func(x, y, level, shift int)) {
	xLength, yLength := footprint(def, obj.Direction)
	if obj.Type == 22 {
		if def.ClipType == 1 {
			write(obj.X, obj.Y, obj.Level, 0x40000)
		}
	} else if obj.Type >= 9 && obj.Type <= 11 {
		if def.ClipType != 0 {
			clipping := 256
			if def.BlocksProjectile {
				clipping += 0x20000
			}
			for i := obj.X; i < obj.X+xLength; i++ {
				for i2 := obj.Y; i2 < obj.Y+yLength; i2++ {
					write(i, i2, obj.Level, clipping)
				}
			}
		}
	} else if obj.Type >= 0 && obj.Type <= 3 {
		if def.ClipType != 0 {
			applyVariableObjectClips(obj.X, obj.Y, obj.Level, obj.Type, obj.Direction, def.BlocksProjectile, add, write)
		}
	}
}

// objectDef resolves an object id, or false when there is no
// definition source or no definition — the early return of
// addObjectClipping (RegionManager.ts:360-363).
func (m *RegionManager) objectDef(id int) (ObjectDef, bool) {
	if m.objects == nil {
		return ObjectDef{}, false
	}
	return m.objects.ObjectDef(id)
}

// footprint returns the x/y footprint lengths for an object facing
// direction: swapped when the object faces 1 or 3
// (RegionManager.ts:365-372).
func footprint(def ObjectDef, direction int) (xLength, yLength int) {
	if direction != 1 && direction != 3 {
		return def.SizeX, def.SizeY
	}
	return def.SizeY, def.SizeX
}

// AddObjectClipping mirrors RegionManager.addObjectClipping
// (RegionManager.ts:346-386) over an ObjectPlacement, with a null
// private area: the clip lands on the region grid. See
// applyObjectClips for the type dispatch. The id -1 case removes
// a zero shift, exactly as TS does (line 356-359) — a no-op on
// the grid that still routes through the region lookup.
func (m *RegionManager) AddObjectClipping(obj ObjectPlacement) {
	m.addObjectClipping(obj, nil)
}

// AddObjectClippingInArea mirrors RegionManager.addObjectClipping
// with a private area (TS reads object.getPrivateArea(),
// RegionManager.ts:355-384): the clip lands ONLY on the area's
// clip store, seeded from the region grid per tile (see
// AddClippingInArea). A nil area behaves exactly like
// AddObjectClipping.
func (m *RegionManager) AddObjectClippingInArea(obj ObjectPlacement, area *PrivateArea) {
	m.addObjectClipping(obj, area)
}

func (m *RegionManager) addObjectClipping(obj ObjectPlacement, area *PrivateArea) {
	if obj.ID == -1 {
		m.removeClipping(obj.X, obj.Y, obj.Level, 0, area)
		return
	}
	def, ok := m.objectDef(obj.ID)
	if !ok {
		return
	}
	applyObjectClips(obj, def, true, func(x, y, level, shift int) {
		m.addClipping(x, y, level, shift, area)
	})
}

// RemoveObjectClipping mirrors RegionManager.removeObjectClipping
// (RegionManager.ts:388-427), with a null private area.
func (m *RegionManager) RemoveObjectClipping(obj ObjectPlacement) {
	m.removeObjectClipping(obj, nil)
}

// RemoveObjectClippingInArea mirrors
// RegionManager.removeObjectClipping with a private area. A nil
// area behaves exactly like RemoveObjectClipping.
func (m *RegionManager) RemoveObjectClippingInArea(obj ObjectPlacement, area *PrivateArea) {
	m.removeObjectClipping(obj, area)
}

func (m *RegionManager) removeObjectClipping(obj ObjectPlacement, area *PrivateArea) {
	if obj.ID == -1 {
		m.removeClipping(obj.X, obj.Y, obj.Level, 0, area)
		return
	}
	def, ok := m.objectDef(obj.ID)
	if !ok {
		return
	}
	applyObjectClips(obj, def, false, func(x, y, level, shift int) {
		m.removeClipping(x, y, level, shift, area)
	})
}

// RegionManager satisfies MovementCollision (CanMoveStart).
var _ MovementCollision = (*RegionManager)(nil)
