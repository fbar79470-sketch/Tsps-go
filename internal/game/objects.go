package game

import (
	"fmt"
	"math"
	"sync"
)

// Game objects: GameObject, the MapObjects tile registry, and the
// ObjectManager runtime indexes, ported from
// game/entity/impl/object/GameObject.ts, MapObjects.ts, and
// ObjectManager.ts (upstream commit bfd81e3). The three TS classes
// collapse into two Go types: GameObject (the placed loc) and
// ObjectRegistry (one instance per World, holding both the
// MapObjects tile/owner maps and the ObjectManager world
// list/indexes, which TS keeps as parallel statics).
//
// THE TWO REGISTERS. TS keeps every placed loc in two places, and
// the Go registry keeps both, because they answer different
// questions:
//
//   - The tile map (MapObjects.mapObjects, MapObjects.ts:12) holds
//     shared, ownerless, non-instanced objects: the cache-map
//     originals a region load places plus every runtime object
//     registered with playerUpdate. Lookups (Find/Get/GetType),
//     cheat checks, and clipping all read this map.
//   - The world list and its by-tile/by-region indexes
//     (ObjectManager.objectsByTile/objectsByRegion,
//     ObjectManager.ts:19-20, over World.getObjects()) hold the
//     RUNTIME objects only: register() skips map originals
//     (ObjectManager.ts:281-283), so a cache-map loc never
//     appears here. Scene snapshots (InScene) and shutdown
//     cleanup (RemovePluginObjects) read these indexes.
//
// A runtime object therefore appears in both registers; a map
// original appears only in the tile map. Owner objects appear in
// the owner map and the world indexes, never in the tile map
// (MapObjects.add returns early for owned objects,
// MapObjects.ts:204-206).
//
// CLIPPING. MapObjects.add/remove are the only paths that touch
// clipping (RegionManager.addObjectClipping/removeObjectClipping,
// MapObjects.ts:229/251): registering with playerUpdate adds the
// object's clipping, deregistering with playerUpdate lifts it,
// and playerUpdate=false touches neither the tile map nor the
// clip grid. The region loader lays cache-map clipping during
// decode (RegionManager.loadMapFiles), so population imports
// placements WITHOUT re-applying clipping.
//
// DELTAS. TS register/deregister with playerUpdate send
// immediately: perform() walks every network player and sends the
// spawn/removal to those whose scene holds the tile
// (ObjectManager.ts:356-404), synchronously, wherever register
// was called. Go cannot send from the game layer (it never
// imports session/protocol), so Register/Deregister append an
// ObjectDelta instead, and the session layer (Worker 2's flush
// path) sends the collected deltas once per tick, like the
// ground-item deltas. The ordering consequence is real and is
// stated plainly: a spawn and a despawn of the same loc within
// one tick reach the client as two ordered frames at flush time,
// where TS would have sent two frames at the two call sites —
// the client-visible end state is the same, and intermediate
// frames are never coalesced away. Deltas recorded with no
// consumer (boot-time spawns, before any player exists) are
// discarded by the spawn appliers, matching TS sends that reach
// zero network players. The per-player shown/missed-removal/
// hidden tracking and all frame emission belong to the session
// layer, not here: this registry exposes queries (InScene,
// RemovedObjects), deltas, and the hide/show lookups, and Worker
// 2 ports the onRegionChange ordering against them.
//
// SAVE/STATE. No object state persists across restarts, and no
// owned save field is added for objects. The world object list
// and the removed-objects list are static runtime collections
// (World.ts:94, 1095-1100); World.getRemovedObjects() is read
// only by ObjectManager, RegionManager's replay, and PrivateArea
// (all runtime systems); savePlayers (World.ts:207-230) saves
// players only; PlayerSave carries no object fields; and object
// spawns re-derive from object-spawns.json at every boot
// (ObjectSpawnDefinitionLoader.ts:20-47). Shutdown's
// removePluginObjects (Server.ts:223) runs AFTER persistence and
// only cleans runtime state before exit. A removed map object
// stays removed only until restart, exactly as in TS.
//
// PRIVATE AREAS. Private areas (PrivateArea /
// TemplatedInstanceArea, milestone 11) are live: an object
// constructed with a private area attaches to it (AttachObject;
// Deregister detaches), template-object lookup and resolveObject
// live on TemplatedInstanceArea, and actors inside an area route
// and step on the area's clips through LookupForActor /
// CollisionForActor. The object-sync viewedArea seam is NOT
// wired (see internal/session/objects.go): area objects are
// treated as main-world objects for sync purposes, and template
// objects are not synced. Deck-object interaction lookup
// (object clicks on boat-deck tiles) is a gap — see the Phase 21
// report.
//
// FORPLAYER / MULTI-LOC. Find resolves the variant a player sees
// through ObjectVariantSource, the Go form of
// ObjectDefinition.forPlayer (ObjectDefinition.ts:81-93) reduced
// to ids. It reuses the rule Store.ObjectForPlayer already ports
// (internal/definition/object.go, the Phase 12 examine seam):
// the coordinator installs an adapter over the Store, and the
// per-player vars arrive through the vars source. With no
// variant source installed, resolution degenerates to the
// identity (every id resolves to itself, the non-multi-loc
// answer); with a source but no vars source, the vars read as
// unset and the default (last) transform is used, the documented
// Store.ObjectForPlayer nil-reader degradation (see
// cmd/server/chatwiring.go). No varp/varbit store is built here.
//
// CONCURRENCY. The registry guards its own maps with a mutex, so
// Register/Deregister/queries are safe from any goroutine. The
// region-load path (Find/Get/GetType/StillStands/hide/show, and
// Deregister via standsForMapObject) triggers RegionManager
// loads, and RegionManager is tick-goroutine-only (see
// regions.go): load-triggering calls must run on the tick
// goroutine in practice, as listener code does. Register never
// triggers a load. Variant/vars/rights sources run under the
// registry mutex and must be pure lookups that never call back
// into the registry.

// PrivateArea lives in private_area.go: the instanced-area type
// (game/model/areas/impl/PrivateArea.ts), landed in milestone 11.
// GameObject carries the nil-able area TS gives it
// (GameObject.ts:16,28-37), and the identity comparisons in the
// registry (sameObjectIdentity, viewedArea) keep their TS shape
// against it.

// GameObject is one placed loc: an id, a shape type, a facing, and
// a tile, plus the ownership and provenance flags the registry
// needs. It ports GameObject.ts (which extends Entity); the Go
// form is a plain struct, NOT a Mobile — it has no world-list
// index, no movement, and no update flags.
//
// What Entity gives it in TS (Entity.ts) and what is ported: the
// location storage (getLocation, Entity.ts:47) is the loc field,
// read through Location(); the area reference behind
// getPrivateArea (Entity.ts:68-70, which returns the area only
// when it carries objects) is the privateArea field, set only by
// the constructor; the string flag set (Entity.ts) is NOT
// STARTED anywhere in Go (see entity.go) and is not ported;
// setLocation is not ported (no object path moves a placed loc
// after registration — the registry indexes key on the
// registration-time tile, as in TS).
//
// NOT ported, each for a named reason: getDefinition
// (GameObject.ts:96) needs the ObjectDefinition wrapper, which
// lives outside this package's seams — callers pass the
// definition footprint (ObjectDef) to Size/FootprintDistance
// instead; performAnimation/performGraphic
// (GameObject.ts:100-122) need World.forEachNetworkPlayer plus
// the sendObjectAnimation/sendGraphic encoders, which are NOT
// STARTED in the session layer.
type GameObject struct {
	id          int
	typ         int
	face        int
	loc         Location
	privateArea *PrivateArea
	owner       *Player
	// baseMap: a loc of the cache's map, loaded with the region
	// or put back in place of one. Only these are remembered once
	// removed, so a reloaded scene keeps them hidden
	// (GameObject.ts:18-22).
	baseMap bool
	// mapOriginal: the very instance the region loader created
	// from the cache's map data (GameObject.ts:23-24).
	mapOriginal bool
	// spawnedBy: the plugin whose code registered this object,
	// or "" for core's own (TS null, GameObject.ts:25-26).
	spawnedBy string
}

// NewGameObject builds a placed loc: id, position, type, face,
// private area (GameObject.ts:28-37). A non-nil area attaches
// the object to itself (privateArea.add(this), GameObject.ts:
// 33-35): the object joins the area's spawned-object list,
// which GetObjects reports and Destroy deregisters. Template
// locs detach again immediately after construction — they
// belong to the scene, not to the spawned entities (see
// TemplatedInstanceArea.objectFor).
func NewGameObject(id int, loc Location, typ, face int, privateArea *PrivateArea) *GameObject {
	obj := &GameObject{id: id, loc: loc, typ: typ, face: face, privateArea: privateArea}
	if privateArea != nil {
		privateArea.AttachObject(obj)
	}
	return obj
}

// ID returns the object id (GameObject.ts:39-41).
func (o *GameObject) ID() int { return o.id }

// Location returns the object's tile (Entity.getLocation,
// Entity.ts:47).
func (o *GameObject) Location() Location { return o.loc }

// Type returns the shape type (GameObject.ts:71-73).
func (o *GameObject) Type() int { return o.typ }

// SetType sets the shape type (GameObject.ts:75-77).
func (o *GameObject) SetType(typ int) { o.typ = typ }

// Face returns the facing direction 0-3 (GameObject.ts:79-81).
func (o *GameObject) Face() int { return o.face }

// SetFace sets the facing direction (GameObject.ts:92-94).
func (o *GameObject) SetFace(face int) { o.face = face }

// PrivateArea returns the instanced area this object belongs to,
// or nil for a shared-world object (GameObject.ts:16).
func (o *GameObject) PrivateArea() *PrivateArea { return o.privateArea }

// Owner returns the owning player, or nil for a shared object
// (GameObject.ts:83-85). Owned objects are visible only to their
// owner.
func (o *GameObject) Owner() *Player { return o.owner }

// SetOwner sets the owning player (nil clears it) and returns the
// object for chaining (GameObject.ts:87-90).
func (o *GameObject) SetOwner(owner *Player) *GameObject {
	o.owner = owner
	return o
}

// IsBaseMap reports whether this object stands for a cache-map
// loc (GameObject.ts:43-45).
func (o *GameObject) IsBaseMap() bool { return o.baseMap }

// MarkBaseMap marks the object as a cache-map loc and returns it
// for chaining (GameObject.ts:47-50).
func (o *GameObject) MarkBaseMap() *GameObject {
	o.baseMap = true
	return o
}

// IsMapOriginal reports whether the region loader created this
// very instance from cache-map data (GameObject.ts:52-54).
func (o *GameObject) IsMapOriginal() bool { return o.mapOriginal }

// MarkMapOriginal marks the object as the region loader's own
// instance; it also marks it base-map, as TS does
// (GameObject.ts:57-60: markMapOriginal returns markBaseMap()).
// Set by the region-load population path only.
func (o *GameObject) MarkMapOriginal() *GameObject {
	o.mapOriginal = true
	return o.MarkBaseMap()
}

// SpawnedBy returns the name of the plugin whose code registered
// this object, or "" for core's own (TS null,
// GameObject.ts:62-64).
func (o *GameObject) SpawnedBy() string { return o.spawnedBy }

// SetSpawnedBy sets the registering plugin name ("" clears it to
// core's own) and returns the object for chaining
// (GameObject.ts:66-69).
func (o *GameObject) SetSpawnedBy(pluginName string) *GameObject {
	o.spawnedBy = pluginName
	return o
}

// Size returns the object's size: (sizeX + sizeY) - 1 from its
// definition, or 1 when there is no definition
// (GameObject.ts:124-130). The definition footprint arrives as
// an ObjectDef because getDefinition is not ported (see the
// GameObject doc); a def with a non-positive size in either axis
// reads as "no definition" and yields 1, matching the TS
// null-definition branch (cache sizes are never below 1).
func (o *GameObject) Size(def ObjectDef) int {
	if def.SizeX < 1 || def.SizeY < 1 {
		return 1
	}
	return def.SizeX + def.SizeY - 1
}

// interactionDistance is GameObject.INTERACTION_DISTANCE
// (GameObject.ts:155): a skilling action may continue within 2
// tiles of the loc's footprint.
const interactionDistance = 2

// FootprintDistance returns the tiles (Chebyshev) from a location
// to the nearest tile of this loc's footprint, its sizes swapped
// by a quarter turn (GameObject.ts:137-149). A different plane
// returns math.MaxInt, the Go form of TS
// Number.POSITIVE_INFINITY. The definition footprint arrives as
// an ObjectDef (see Size); either size below 1 reads as 1, the
// TS `?? 1` fallback (GameObject.ts:141-142).
func (o *GameObject) FootprintDistance(from Location, def ObjectDef) int {
	at := o.loc
	if from.Z != at.Z {
		return math.MaxInt
	}
	sizeX, sizeY := def.SizeX, def.SizeY
	if sizeX < 1 {
		sizeX = 1
	}
	if sizeY < 1 {
		sizeY = 1
	}
	turned := (o.face & 1) == 1
	width, length := sizeX, sizeY
	if turned {
		width, length = sizeY, sizeX
	}
	if width < 1 {
		width = 1
	}
	if length < 1 {
		length = 1
	}
	gap := func(point, start, size int) int {
		g := start - point
		if alt := point - (start + size - 1); alt > g {
			g = alt
		}
		if g < 0 {
			g = 0
		}
		return g
	}
	if gx, gy := gap(from.X, at.X, width), gap(from.Y, at.Y, length); gx > gy {
		return gx
	} else {
		return gy
	}
}

// IsWithinInteractionDistance reports whether a skilling action
// on this loc may continue from that tile: within 2 tiles of its
// footprint (GameObject.ts:151-153).
func (o *GameObject) IsWithinInteractionDistance(from Location, def ObjectDef) bool {
	return o.FootprintDistance(from, def) <= interactionDistance
}

// Equals reports TS GameObject.equals (GameObject.ts:157-163):
// same tile, id, face, type, and private area. Owner, spawnedBy,
// and the base-map flags are deliberately NOT compared — that is
// sameObjectIdentity's job in the registry, and the two
// predicates differ exactly there.
func (o *GameObject) Equals(other *GameObject) bool {
	if o == nil || other == nil {
		return false
	}
	return o.loc.Equals(other.loc) && o.id == other.id && o.face == other.face &&
		o.typ == other.typ && o.privateArea == other.privateArea
}

// Clone returns a fresh object with the same id, tile, type,
// face, private area, and owner (GameObject.ts:165-174). The
// base-map/map-original flags and spawnedBy are NOT carried: TS
// clone constructs a new GameObject, whose flags default off and
// whose spawnedBy defaults null.
func (o *GameObject) Clone() *GameObject {
	return NewGameObject(o.id, o.loc, o.typ, o.face, o.privateArea).SetOwner(o.owner)
}

// ObjectVarReader is the per-player varp/varbit view forPlayer
// reads (ObjectDefinition.forPlayer, ObjectDefinition.ts:81-93,
// reads them through the PacketSender). It mirrors cache.VarReader
// method-for-method without importing internal/cache, following
// the seam convention that keeps this package cache-free (see
// collision.go); the coordinator adapts a real var reader to it.
// No production reader is wired to variant resolution yet: the
// varp store is per-session (phase 18) while the registry
// resolves variants world-wide, so wiring a per-player reader
// into the registry is a separate decision — see the file
// header and cmd/server/objectregistrywiring.go.
type ObjectVarReader interface {
	GetVarp(id int) int
	GetVarbit(id int) int
}

// ObjectVariantSource resolves the loc variant a player sees: the
// Go form of ObjectDefinition.forPlayer (ObjectDefinition.ts:81-93)
// reduced to ids. It returns the variant id and true, or false
// when forPlayer returns null (the resolved transform is -1, or
// the id has no definition). A nil vars reader means "no vars
// set": the resolution uses the default (last) transform, the
// Store.ObjectForPlayer nil-reader degradation. The coordinator
// implements it over the definition Store (Store.ObjectForPlayer,
// the Phase 12 examine seam); with no source installed the
// registry resolves every id to itself.
type ObjectVariantSource interface {
	ObjectVariantForPlayer(id int, vars ObjectVarReader) (variantID int, ok bool)
}

// ObjectDelta is one runtime spawn or removal the session layer
// has not yet sent: the Go form of perform()'s immediate sends
// (ObjectManager.ts:356-404), collected per tick. Removed=false
// is a spawn (send the object), Removed=true is a removal (send
// the removal). The Object pointer is the registered instance;
// consumers must read it before the next tick's mutations. See
// the file header for the delta contract.
type ObjectDelta struct {
	Object  *GameObject
	Removed bool
}

// regionKey is the ObjectManager region index key: the 64x64
// region coordinates (regionKey, ObjectManager.ts:101-103,
// `${x >> 6},${y >> 6}`), as a comparable pair.
type regionKey struct{ x, y int }

func regionKeyFor(loc Location) regionKey {
	return regionKey{loc.X >> 6, loc.Y >> 6}
}

// ObjectRegistry holds the MapObjects tile/owner maps and the
// ObjectManager world list/indexes for one world. Use
// NewObjectRegistry; the zero value is not usable. See the file
// header for the two-register model, the delta contract, and the
// concurrency discipline.
//
// DEVIATION — tile keys: TS keys the tile map by the numeric hash
// (z*16384+x)*16384+y (MapObjects.getHash, MapObjects.ts:275-277,
// unique only for coordinates below 16384). Go keys by the
// Location struct itself: identical observable behavior for every
// in-world tile, with no coordinate bound and no aliasing.
//
// DEVIATION — owner map: TS uses a WeakMap keyed by Player
// (MapObjects.ts:13), so a departed player's owner objects vanish
// with garbage collection. Go has no WeakMap; this is an explicit
// player-keyed map, and DiscardPlayer must run when a player
// leaves the world (wired in World.drainRemovePlayers). Forgetting
// the call leaks the departed player's owner entries until the
// world is discarded.
type ObjectRegistry struct {
	mu sync.Mutex

	// regions supplies map loading, cache-map placements, and
	// clipping. Nil means no map loading and no clipping (the
	// NoCache degradation); lookups then see only registered
	// objects.
	regions *RegionManager

	// tileObjects is MapObjects.mapObjects: shared, ownerless,
	// non-instanced objects by tile, map originals included.
	tileObjects map[Location][]*GameObject
	// ownerObjects is MapObjects.ownerObjects: per-player tile
	// maps for owned objects. See the WeakMap deviation above.
	ownerObjects map[*Player]map[Location][]*GameObject
	// worldObjects is World.getObjects(): runtime objects in
	// registration order, map originals excluded.
	worldObjects []*GameObject
	// byTile/byRegion are ObjectManager.objectsByTile/
	// objectsByRegion: the world-list indexes. byRegion holds
	// insertion-ordered slices (TS uses Sets, which iterate in
	// insertion order); duplicates by identity are refused on add,
	// matching Set.add.
	byTile   map[Location][]*GameObject
	byRegion map[regionKey][]*GameObject
	// removedObjects is World.getRemovedObjects(): base-map
	// objects removed at runtime, remembered so reloaded scenes
	// keep them hidden (ObjectManager.ts:319-347). Runtime-only;
	// nothing persists it (see the file header).
	removedObjects []*GameObject
	// deltas collects perform() sends for the session layer.
	deltas []ObjectDelta
	// populated records, per region id, the RegionManager load
	// sequence whose placements were imported into the tile map.
	// A mismatch (region reloaded) drops that region's
	// map-original entries and re-imports.
	populated map[int]int

	// variantSource resolves forPlayer variants (nil = identity).
	// varsSource supplies the per-player var reader (nil = no
	// vars). rightsSource answers the player's rights id for the
	// developer notice (nil = RightsNone, never a developer).
	variantSource ObjectVariantSource
	varsSource    func(*Player) ObjectVarReader
	rightsSource  func(*Player) int
}

// NewObjectRegistry returns an empty registry with no region
// manager, no variant/vars/rights sources, and no objects. The
// World installs its RegionManager through SetRegionManager; the
// coordinator installs the definition/vars/rights seams.
func NewObjectRegistry() *ObjectRegistry {
	return &ObjectRegistry{
		tileObjects:  make(map[Location][]*GameObject),
		ownerObjects: make(map[*Player]map[Location][]*GameObject),
		byTile:       make(map[Location][]*GameObject),
		byRegion:     make(map[regionKey][]*GameObject),
		populated:    make(map[int]int),
	}
}

// SetRegionManager installs the region manager used for map
// loading, placement import, and clipping. A swap invalidates
// population bookkeeping: every map-original entry is dropped
// from the tile map and the populated record is cleared, so the
// next lookup re-imports from the new manager; runtime objects
// are untouched. Call only before the engine starts (World
// calls it from SetRegionManager); a nil manager means no map
// loading and no clipping.
func (r *ObjectRegistry) SetRegionManager(rm *RegionManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.regions = rm
	r.populated = make(map[int]int)
	for loc, list := range r.tileObjects {
		kept := list[:0]
		for _, obj := range list {
			if !obj.IsMapOriginal() {
				kept = append(kept, obj)
			}
		}
		if len(kept) == 0 {
			delete(r.tileObjects, loc)
		} else {
			r.tileObjects[loc] = kept
		}
	}
}

// SetVariantSource installs the forPlayer variant resolver (see
// ObjectVariantSource). Nil restores identity resolution.
func (r *ObjectRegistry) SetVariantSource(src ObjectVariantSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.variantSource = src
}

// SetVarsSource installs the per-player var reader supplier (nil
// restores "no vars"). The supplier must be a pure lookup that
// never calls back into the registry: it runs under the registry
// mutex.
func (r *ObjectRegistry) SetVarsSource(src func(*Player) ObjectVarReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.varsSource = src
}

// SetRightsSource installs the rights-id seam for the developer
// notice in Find (see commands.go SetRightsSource for the same
// seam shape). Nil restores RightsNone for every player.
func (r *ObjectRegistry) SetRightsSource(src func(*Player) int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rightsSource = src
}

// placementOf renders a GameObject as the ObjectPlacement the
// RegionManager clipping calls read (id, tile, type, face — the
// fields RegionManager.addObjectClipping reads,
// RegionManager.ts:346-354).
func placementOf(obj *GameObject) ObjectPlacement {
	loc := obj.Location()
	return ObjectPlacement{
		ID:        obj.ID(),
		X:         loc.X,
		Y:         loc.Y,
		Level:     loc.Z,
		Type:      obj.Type(),
		Direction: obj.Face(),
	}
}

// sameObjectIdentity is ObjectManager.sameObjectIdentity
// (ObjectManager.ts:163-170): id, type, face, private area,
// owner, and tile. It differs from GameObject.Equals by including
// the owner.
func sameObjectIdentity(a, b *GameObject) bool {
	return a.ID() == b.ID() && a.Type() == b.Type() && a.Face() == b.Face() &&
		a.PrivateArea() == b.PrivateArea() && a.Owner() == b.Owner() &&
		a.Location().Equals(b.Location())
}

// The helpers below assume r.mu is held. Public methods lock;
// helpers never lock, and never call public methods.

// addTileObject appends obj to the tile map unless the identical
// instance is already there (MapObjects.add's identity dedup,
// MapObjects.ts:212-225).
func (r *ObjectRegistry) addTileObject(obj *GameObject) {
	loc := obj.Location()
	for _, existing := range r.tileObjects[loc] {
		if existing == obj {
			return
		}
	}
	r.tileObjects[loc] = append(r.tileObjects[loc], obj)
}

// mapAdd is MapObjects.add (MapObjects.ts:203-230). An owned
// object goes to the owner map ONLY and returns before any
// clipping — owned objects never clip through this path. A
// private-area object skips the tile map but still clips in TS,
// on its private grid; Go has no private grid, so its clipping
// is skipped (documented collapse — no production object carries
// an area). A shared object enters the tile map and its clipping
// is laid on the region grid.
func (r *ObjectRegistry) mapAdd(obj *GameObject) {
	if obj.Owner() != nil {
		owner := obj.Owner()
		byTile := r.ownerObjects[owner]
		if byTile == nil {
			byTile = make(map[Location][]*GameObject)
			r.ownerObjects[owner] = byTile
		}
		loc := obj.Location()
		for _, existing := range byTile[loc] {
			if existing == obj {
				return
			}
		}
		byTile[loc] = append(byTile[loc], obj)
		return
	}
	if obj.PrivateArea() == nil {
		r.addTileObject(obj)
	}
	if r.regions != nil && obj.PrivateArea() == nil {
		r.regions.AddObjectClipping(placementOf(obj))
	}
}

// mapRemove is MapObjects.remove (MapObjects.ts:232-252). An
// owned object leaves the owner map ONLY, with no clipping
// change. Otherwise every tile-map entry matching by FIELDS
// (id, type, face, tile — not identity) is removed, so one
// removal clears every stacked copy, and the clipping is lifted.
// Private-area objects skip both the tile map and (in Go) the
// clipping, mirroring mapAdd's collapse.
func (r *ObjectRegistry) mapRemove(obj *GameObject) {
	if obj.Owner() != nil {
		byTile := r.ownerObjects[obj.Owner()]
		if byTile != nil {
			loc := obj.Location()
			list := byTile[loc]
			for i, existing := range list {
				if existing == obj {
					byTile[loc] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(byTile[loc]) == 0 {
				delete(byTile, loc)
			}
		}
		return
	}
	if obj.PrivateArea() == nil {
		loc := obj.Location()
		list := r.tileObjects[loc]
		for i := len(list) - 1; i >= 0; i-- {
			existing := list[i]
			if existing.ID() == obj.ID() && existing.Type() == obj.Type() &&
				existing.Face() == obj.Face() && existing.Location().Equals(loc) {
				list = append(list[:i], list[i+1:]...)
			}
		}
		if len(list) == 0 {
			delete(r.tileObjects, loc)
		} else {
			r.tileObjects[loc] = list
		}
	}
	if r.regions != nil && obj.PrivateArea() == nil {
		r.regions.RemoveObjectClipping(placementOf(obj))
	}
}

// addToWorld is ObjectManager.addToWorld
// (ObjectManager.ts:105-116): append to the world list and both
// indexes. The region index refuses identity duplicates,
// matching TS Set.add.
func (r *ObjectRegistry) addToWorld(obj *GameObject) {
	r.worldObjects = append(r.worldObjects, obj)
	loc := obj.Location()
	r.byTile[loc] = append(r.byTile[loc], obj)
	key := regionKeyFor(loc)
	for _, existing := range r.byRegion[key] {
		if existing == obj {
			return
		}
	}
	r.byRegion[key] = append(r.byRegion[key], obj)
}

// removeFromWorld is ObjectManager.removeFromWorld
// (ObjectManager.ts:118-136): splice the FIRST identity match
// from the world list and the tile index, delete from the region
// index, and drop empty index entries.
func (r *ObjectRegistry) removeFromWorld(obj *GameObject) {
	for i, existing := range r.worldObjects {
		if existing == obj {
			r.worldObjects = append(r.worldObjects[:i], r.worldObjects[i+1:]...)
			break
		}
	}
	loc := obj.Location()
	if list := r.byTile[loc]; list != nil {
		for i, existing := range list {
			if existing == obj {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(r.byTile, loc)
		} else {
			r.byTile[loc] = list
		}
	}
	key := regionKeyFor(loc)
	if list := r.byRegion[key]; list != nil {
		for i, existing := range list {
			if existing == obj {
				list = append(list[:i], list[i+1:]...)
				break
			}
		}
		if len(list) == 0 {
			delete(r.byRegion, key)
		} else {
			r.byRegion[key] = list
		}
	}
}

// atLocked returns a copy of the runtime objects on a tile (the
// by-tile index). It never triggers a region load, matching
// ObjectManager.objectsAt (ObjectManager.ts:159-161).
func (r *ObjectRegistry) atLocked(loc Location) []*GameObject {
	list := r.byTile[loc]
	if len(list) == 0 {
		return nil
	}
	return append([]*GameObject(nil), list...)
}

// syncRegion imports the cache-map placements of the region
// containing loc into the tile map, once per region load. It is
// the Go form of the load half of MapObjects.get/getType (which
// call RegionManager.loadMapFiles, MapObjects.ts:101/145) plus
// the population RegionManager.loadMapFiles performs through
// addObject -> MapObjects.add (RegionManager.ts:999, 337-345).
// The RegionManager decodes and clips during its own load; this
// side imports the decoded placements (Placements accessor) as
// map-original GameObjects. A region whose load sequence changed
// (reload) has its old map-original entries dropped and its
// placements re-imported; runtime entries are never dropped.
// With no region manager, or a region with no data, this is a
// no-op (lookups see registered objects only).
func (r *ObjectRegistry) syncRegion(loc Location) {
	if r.regions == nil {
		return
	}
	regionID := RegionIDForTile(loc.X, loc.Y)
	if !r.regions.IsRegionLoaded(regionID) {
		r.regions.LoadRegion(loc.X, loc.Y)
		if !r.regions.IsRegionLoaded(regionID) {
			// The load failed (no data for the region): drop
			// stale map-original entries from a previous load,
			// as reloadRegion clears before reloading
			// (RegionManager.ts:873).
			if _, ok := r.populated[regionID]; ok {
				r.dropMapOriginals(regionID)
				delete(r.populated, regionID)
			}
			return
		}
	}
	seq := r.regions.RegionLoadSeq(regionID)
	if current, ok := r.populated[regionID]; ok && current == seq {
		return
	}
	r.dropMapOriginals(regionID)
	r.importRegion(regionID)
	r.populated[regionID] = seq
}

// dropMapOriginals removes every map-original entry in one region
// from the tile map: the Go form of clearRegionMapObjects
// (RegionManager.ts:814-824) reduced to the entries a load
// placed. Runtime entries on the same tiles are kept — TS
// deletes and then re-adds them in replayWorldObjects
// (RegionManager.ts:833-855); keeping them reaches the same tile
// state without the churn. Map originals never appear in the
// world indexes, so those are untouched.
func (r *ObjectRegistry) dropMapOriginals(regionID int) {
	for loc, list := range r.tileObjects {
		if RegionIDForTile(loc.X, loc.Y) != regionID {
			continue
		}
		kept := list[:0]
		for _, obj := range list {
			if !obj.IsMapOriginal() {
				kept = append(kept, obj)
			}
		}
		if len(kept) == 0 {
			delete(r.tileObjects, loc)
		} else {
			r.tileObjects[loc] = kept
		}
	}
}

// importRegion creates one map-original GameObject per decoded
// placement of a loaded region and adds it to the tile map
// WITHOUT touching clipping (the decode already laid it) or the
// world indexes (register() skips map originals) or the deltas
// (a load sends nothing; clients rebuild the map from the cache
// data itself). A placement matching a runtime-removed object
// (same id, type, face, tile) is skipped and its freshly laid
// clipping is lifted: the replayWorldObjects step
// (RegionManager.ts:833-855) that keeps a removed map object
// hidden after its region reloads. The wasRemovedOnLoad
// bookkeeping (loadRemovals) is NOT STARTED, as regions.go notes.
// Placements can never carry id -1 (decodeObjects builds ids
// from readSmart deltas of at least 1 from -1), so the
// addObject(-1) -> MapObjects.clear path (RegionManager.ts:340-341)
// has no Go form here.
func (r *ObjectRegistry) importRegion(regionID int) {
	placements, ok := r.regions.Placements(regionID)
	if !ok {
		return
	}
	for _, p := range placements {
		loc := Location{X: p.X, Y: p.Y, Z: p.Level}
		removed := false
		for _, gone := range r.removedObjects {
			if gone.ID() == p.ID && gone.Type() == p.Type && gone.Face() == p.Direction &&
				gone.Location().Equals(loc) {
				removed = true
				break
			}
		}
		if removed {
			r.regions.RemoveObjectClipping(p)
			continue
		}
		obj := NewGameObject(p.ID, loc, p.Type, p.Direction, nil)
		obj.MarkMapOriginal()
		r.addTileObject(obj)
	}
}

// preferReachable is MapObjects.preferReachable
// (MapObjects.ts:130-133): when one loc id is placed twice on a
// tile, a straight wall decoration (shapes 4-5) loses to any
// other copy, because the decoration is only reached by standing
// on its tile under the other copy.
func preferReachable(matches []*GameObject) *GameObject {
	if len(matches) <= 1 {
		if len(matches) == 0 {
			return nil
		}
		return matches[0]
	}
	for _, obj := range matches {
		if obj.Type() != 4 && obj.Type() != 5 {
			return obj
		}
	}
	return matches[0]
}

// getByID is MapObjects.get with a nil private area
// (MapObjects.ts:94-121): load the region, clamp a plane >= 4 to
// plane 0 (TS clones and setZ(0) before hashing AND before the
// location-equality filter), and return the preferReachable
// match on id and tile. The instanced-object branch needs a
// private area's object list, which does not exist in Go; with
// no areas it is unreachable.
func (r *ObjectRegistry) getByID(id int, loc Location) *GameObject {
	r.syncRegion(loc)
	if loc.Z >= 4 {
		loc = Location{X: loc.X, Y: loc.Y, Z: 0}
	}
	var matches []*GameObject
	for _, obj := range r.tileObjects[loc] {
		if obj.ID() == id && obj.Location().Equals(loc) {
			matches = append(matches, obj)
		}
	}
	return preferReachable(matches)
}

// getByType is MapObjects.getType with a nil private area
// (MapObjects.ts:135-167): load the region, clamp a plane >= 4
// to plane 0, and return the FIRST tile-map entry with the shape
// type on the tile. Unlike getByID there is no preferReachable
// step: TS returns the first match in list order.
func (r *ObjectRegistry) getByType(loc Location, typ int) *GameObject {
	r.syncRegion(loc)
	if loc.Z >= 4 {
		loc = Location{X: loc.X, Y: loc.Y, Z: 0}
	}
	for _, obj := range r.tileObjects[loc] {
		if obj.Type() == typ && obj.Location().Equals(loc) {
			return obj
		}
	}
	return nil
}

// forPlayer resolves the variant id a player sees for a loc id:
// ObjectDefinition.forPlayer (ObjectDefinition.ts:81-93) through
// the installed variant source, or the identity when no source
// is installed. ok=false is the TS null (resolved -1 or unknown
// id). A nil player reads vars as unset.
func (r *ObjectRegistry) forPlayer(player *Player, id int) (int, bool) {
	if r.variantSource == nil {
		return id, true
	}
	var vars ObjectVarReader
	if player != nil && r.varsSource != nil {
		vars = r.varsSource(player)
	}
	return r.variantSource.ObjectVariantForPlayer(id, vars)
}

// findObject is MapObjects.find (MapObjects.ts:58-80), the quiet
// lookup behind Find: owned objects first (variant-resolved),
// then get(), then a shared-tile scan (variant-resolved) over
// the unclamped tile, then the bridge-tile z-1 fallback, then the
// final forPlayer null guard. The private-area branches — the
// area's own objects, template objects, resolveObject — are
// skipped: no areas exist in Go, so they can contribute nothing.
// A nil player skips the owner branch and reads vars as unset
// (Go-only tolerance; TS always passes a player).
func (r *ObjectRegistry) findObject(player *Player, id int, loc Location) *GameObject {
	if player != nil {
		if byTile := r.ownerObjects[player]; byTile != nil {
			for _, entry := range byTile[loc] {
				if entry.Location().Equals(loc) {
					if variant, ok := r.forPlayer(player, entry.ID()); ok && variant == id {
						return entry
					}
				}
			}
		}
	}
	obj := r.getByID(id, loc)
	if obj == nil {
		// TS scans [...areaCandidates, ...sharedTile] with the
		// UNCLAMPED tile hash; the area candidates are empty
		// here. getByID already loaded the region.
		for _, entry := range r.tileObjects[loc] {
			if entry.Location().Equals(loc) {
				if variant, ok := r.forPlayer(player, entry.ID()); ok && variant == id {
					obj = entry
					break
				}
			}
		}
	}
	// Locs on bridge tiles are loaded one plane down
	// (RegionManager's height adjustment), but the player beside
	// them — on a castle wall, say — is still on the plane above
	// (MapObjects.ts:74-78).
	if obj == nil && loc.Z > 0 {
		obj = r.getByID(id, Location{X: loc.X, Y: loc.Y, Z: loc.Z - 1})
	}
	if obj != nil {
		if _, ok := r.forPlayer(player, obj.ID()); !ok {
			return nil
		}
	}
	return obj
}

// perform is the registry half of ObjectManager.perform
// (ObjectManager.ts:356-404): the MapObjects add/remove plus the
// delta. The send half — walking every network player, the
// scene check, sendShown/recordMissedRemoval — belongs to the
// session layer, which reads Deltas() (see the file header). An
// id of -1 forces a despawn, as TS retypes it
// (ObjectManager.ts:358-360).
func (r *ObjectRegistry) perform(obj *GameObject, spawn bool) {
	removed := !spawn || obj.ID() == -1
	if removed {
		r.mapRemove(obj)
	} else {
		r.mapAdd(obj)
	}
	r.deltas = append(r.deltas, ObjectDelta{Object: obj, Removed: removed})
}

// standsForMapObject is ObjectManager.standsForMapObject
// (ObjectManager.ts:311-317): true for a base-map object, or a
// copy of one — a placed map original with the same id and type
// stands at the tile. The getByID call loads the region, as the
// TS MapObjects.get does.
func (r *ObjectRegistry) standsForMapObject(obj *GameObject) bool {
	if obj.IsBaseMap() {
		return true
	}
	placed := r.getByID(obj.ID(), obj.Location())
	return placed != nil && placed.IsBaseMap() && placed.Type() == obj.Type()
}

// Register registers a runtime object: ObjectManager.register
// (ObjectManager.ts:247-284). The playerUpdate parameter means
// exactly what TS means by it: when true, the registration
// performs (tile-map add + clipping + delta, and same-shape
// predecessors lose their tile-map entries and clipping); when
// false, only the world indexes and the removed-list bookkeeping
// change, and nothing is sent. A nil object is ignored.
//
// In order: same-shape predecessors on the tile (same type, same
// private area, same owner) leave the world indexes, losing
// their tile-map entries and clipping when playerUpdate is true;
// removed-list entries for the same shape slot are spliced out
// (their clipping is lifted directly, NOT through the tile map —
// TS calls RegionManager.removeObjectClipping on the removed
// entry) and the new object is marked base-map, so removing it
// later hides the spot on scene reloads; map originals skip the
// world list (unless instanced); then perform runs when
// playerUpdate is true.
//
// DEVIATION — spawnedBy: TS stamps a null spawnedBy from
// PluginContext.current() (ObjectManager.ts:248), the ambient
// plugin registering the object. Go has no plugin context; the
// spawnedBy a caller set (or "" for core) stands as-is. Content
// spawn surfaces must stamp the plugin name themselves, or
// RemovePluginObjects will not recognize their objects at
// shutdown. The private-area detach calls in the replacement
// loop are no-ops: no area system exists to detach from.
func (r *ObjectRegistry) Register(obj *GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registerLocked(obj, playerUpdate)
}

// registerLocked is Register with the mutex already held.
func (r *ObjectRegistry) registerLocked(obj *GameObject, playerUpdate bool) {
	loc := obj.Location()
	for _, o := range r.atLocked(loc) {
		if o.Type() == obj.Type() && o.PrivateArea() == obj.PrivateArea() && o.Owner() == obj.Owner() {
			// Replacing a solid loc with an empty form must
			// clear its old clipping too.
			if playerUpdate {
				r.mapRemove(o)
			}
			r.removeFromWorld(o)
		}
	}
	// Put back exactly as the map had it (a door closed again,
	// from a snapshot copy).
	for i := len(r.removedObjects) - 1; i >= 0; i-- {
		removed := r.removedObjects[i]
		if removed.Type() != obj.Type() {
			continue
		}
		if !removed.Location().Equals(loc) {
			continue
		}
		if removed.PrivateArea() != obj.PrivateArea() {
			continue
		}
		if r.regions != nil {
			r.regions.RemoveObjectClipping(placementOf(removed))
		}
		r.removedObjects = append(r.removedObjects[:i], r.removedObjects[i+1:]...)
		// It takes a removed map object's place: removing it
		// later must hide that spot on scene reloads, as for
		// the original.
		obj.MarkBaseMap()
	}
	// Original map objects need no replay when they respawn.
	// Snapshot copies must replay their restored state.
	if !obj.IsMapOriginal() || obj.PrivateArea() != nil {
		r.addToWorld(obj)
	}
	if playerUpdate {
		r.perform(obj, true)
	}
}

// Deregister removes a runtime object: ObjectManager.deregister
// (ObjectManager.ts:319-347). playerUpdate means what TS means
// (see Register): when true the removal performs (tile-map
// removal + clipping lift + removal delta); when false only the
// world indexes change — but the removed-list entry is still
// recorded for map objects either way, which is what
// ReplaceInPlace relies on. A nil object is ignored.
//
// In order: whether the object stands for a map object is
// decided BEFORE the despawn takes it out of the tile map (the
// check reads the tile map); world-index entries matching by
// full identity leave; perform runs when playerUpdate is true;
// then, only for map objects, the removed list gains the object
// (deduplicated by identity) so scene reloads keep the spot
// hidden. Runtime objects (fires, traps, temporary locs) are
// never remembered: TS used to pile them up here for good.
func (r *ObjectRegistry) Deregister(obj *GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deregisterLocked(obj, playerUpdate)
}

// deregisterLocked is Deregister with the mutex already held.
func (r *ObjectRegistry) deregisterLocked(obj *GameObject, playerUpdate bool) {
	// Before the despawn takes it out of the tile map.
	mapObject := obj.PrivateArea() != nil || r.standsForMapObject(obj)
	for _, existing := range r.atLocked(obj.Location()) {
		if sameObjectIdentity(existing, obj) {
			r.removeFromWorld(existing)
		}
	}
	// Leaving the area's object list (ObjectManager.ts:324,328):
	// the register-time AttachObject is undone here, so a
	// deregistered instance object no longer answers area
	// lookups. DetachObject is nil-safe and idempotent.
	if pa := obj.PrivateArea(); pa != nil {
		pa.DetachObject(obj)
	}
	if playerUpdate {
		r.perform(obj, false)
	}
	if !mapObject {
		return
	}
	for _, removed := range r.removedObjects {
		if sameObjectIdentity(removed, obj) {
			return
		}
	}
	r.removedObjects = append(r.removedObjects, obj)
}

// ReplaceInPlace swaps one loc for another in the same tile and
// shape slot (a lever pulled down and back up), as live sends
// it: one spawn delta for the incoming loc, no removal delta for
// the outgoing one (ObjectManager.replaceInPlace,
// ObjectManager.ts:294-308). The outgoing loc leaves the world
// indexes (deregister without perform) and the tile map (its
// clipping and later lookups go, as a despawn would); the
// incoming loc registers with playerUpdate.
//
// GAP — missed removals: TS records a missed removal for every
// away player who was shown the outgoing loc
// (ObjectManager.ts:296-306), so they are put right when they
// come back. That needs the per-player shown state, which
// belongs to the session layer (Worker 2); this method emits no
// removal delta for the outgoing loc, so the session layer can
// only correct away players from the incoming spawn delta and
// the region-change snapshot. Nil arguments are ignored.
func (r *ObjectRegistry) ReplaceInPlace(outgoing, incoming *GameObject) {
	if outgoing == nil || incoming == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deregisterLocked(outgoing, false)
	r.mapRemove(outgoing)
	r.registerLocked(incoming, true)
}

// RemovePluginObjects takes out every object a plugin
// registered, as the server shuts down: plugins never reload, so
// nothing else would (ObjectManager.removePluginObjects,
// ObjectManager.ts:148-157). A stand-in for a map object (a door
// closed again) stays. It returns the number removed. Each
// removal deregisters with playerUpdate, so deltas are recorded
// (the shutdown path discards them with the world). The only TS
// call site is Server shutdown, after persistence
// (Server.ts:223); the Go call site is a coordinator wiring need
// (see the report) — nothing calls this yet.
func (r *ObjectRegistry) RemovePluginObjects() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	removed := 0
	for _, obj := range append([]*GameObject(nil), r.worldObjects...) {
		if obj.SpawnedBy() == "" || obj.IsBaseMap() {
			continue
		}
		r.deregisterLocked(obj, true)
		removed++
	}
	return removed
}

// At returns the runtime objects registered on a tile (any
// private area), in registration order: ObjectManager.objectsAt
// (ObjectManager.ts:159-161). Map originals are NOT included —
// they never enter the world indexes. The slice is a copy; it
// never triggers a region load.
func (r *ObjectRegistry) At(loc Location) []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.atLocked(loc)
}

// InRegion returns the runtime objects registered in one 64x64
// region (all planes, any private area), in registration order:
// ObjectManager.objectsInRegion (ObjectManager.ts:139-142).
// regionX/regionY are region coordinates (tile >> 6), not tile
// coordinates. The slice is a copy; it never triggers a load.
func (r *ObjectRegistry) InRegion(regionX, regionY int) []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	list := r.byRegion[regionKey{regionX, regionY}]
	if len(list) == 0 {
		return nil
	}
	return append([]*GameObject(nil), list...)
}

// InScene returns the shared runtime objects in a scene: the
// region-loop half of ObjectManager.onRegionChange
// (ObjectManager.ts:221-239), without a player. Regions are
// walked in ascending order and objects in registration order;
// the bounds test covers x/y only — every plane, because the
// scene holds all four and the packets carry their own level.
// Owned objects are EXCLUDED (they belong to one player; see
// OwnerObjectsInScene), as are instanced objects (viewedArea is
// nil with no areas). The matchesBaseMapObject skip
// (ObjectManager.ts:230) cannot fire with no areas and is not
// evaluated. Removed objects, hidden objects, and missed
// removals are NOT included: the session layer sends those
// first, from RemovedObjects and its own per-player state, in
// the onRegionChange order. The slice is a copy; it never
// triggers a region load.
//
// NOTE — the TS double send: onRegionChange sends an owner's own
// objects twice, once in the region loop (which only skips OTHER
// players' objects) and once in the ownerObjectsInScene loop
// (ObjectManager.ts:224-242). Go separates the two queries so the
// session layer sends each object once; that is a deliberate
// deviation from the duplicated packet, with no client-visible
// difference.
func (r *ObjectRegistry) InScene(baseX, baseY, size int) []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	if size <= 0 {
		return nil
	}
	var out []*GameObject
	for regionX := baseX >> 6; regionX <= (baseX+size-1)>>6; regionX++ {
		for regionY := baseY >> 6; regionY <= (baseY+size-1)>>6; regionY++ {
			for _, obj := range r.byRegion[regionKey{regionX, regionY}] {
				if obj.Owner() != nil {
					continue
				}
				if obj.PrivateArea() != nil {
					continue
				}
				loc := obj.Location()
				if loc.X < baseX || loc.X >= baseX+size || loc.Y < baseY || loc.Y >= baseY+size {
					continue
				}
				out = append(out, obj)
			}
		}
	}
	return out
}

// OwnerObjectsInScene returns one player's owned objects in a
// scene: MapObjects.ownerObjectsInScene (MapObjects.ts:35-47).
// The bounds test covers x/y only (all planes). Order is
// unspecified (Go map iteration over the owner's tile map); TS
// iterates in hash-insertion order, which no consumer depends
// on — each object is sent independently. It never triggers a
// region load: owner objects are runtime-only.
func (r *ObjectRegistry) OwnerObjectsInScene(player *Player, baseX, baseY, size int) []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	byTile := r.ownerObjects[player]
	if byTile == nil || size <= 0 {
		return nil
	}
	var out []*GameObject
	for _, list := range byTile {
		for _, obj := range list {
			loc := obj.Location()
			if loc.X >= baseX && loc.X < baseX+size && loc.Y >= baseY && loc.Y < baseY+size {
				out = append(out, obj)
			}
		}
	}
	return out
}

// Get returns the loc with an id on a tile, loading the region
// first: MapObjects.get with a nil private area
// (MapObjects.ts:94-121). See getByID for the clamp and
// preferReachable rules. A nil result means no such loc stands
// there.
func (r *ObjectRegistry) Get(id int, loc Location) *GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getByID(id, loc)
}

// GetType returns the loc with a shape type on a tile, loading
// the region first: MapObjects.getType with a nil private area
// (MapObjects.ts:135-167). See getByType: the first match in
// tile-map order, with no preferReachable step.
func (r *ObjectRegistry) GetType(loc Location, typ int) *GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getByType(loc, typ)
}

// ObjectsAtTile returns all scenery at a tile in the shared
// world's view: MapObjects.getObjectsAt (MapObjects.ts:175-196)
// with no private area — the tile-map entries on the tile (a
// plane >= 4 clamps to plane 0, as TS clamps the map location).
// The private-area and template branches contribute nothing with
// no areas. The slice is a copy.
func (r *ObjectRegistry) ObjectsAtTile(loc Location) []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.syncRegion(loc)
	if loc.Z >= 4 {
		loc = Location{X: loc.X, Y: loc.Y, Z: 0}
	}
	var out []*GameObject
	for _, obj := range r.tileObjects[loc] {
		if obj.PrivateArea() == nil && obj.Location().Equals(loc) {
			out = append(out, obj)
		}
	}
	return out
}

// findWithNoticeLocked is findObject plus the getPrivateArea
// developer notice (MapObjects.ts:49-54): when no loc stands
// there and the player is a DEVELOPER, the player is told. The
// mutex must be held.
func (r *ObjectRegistry) findWithNoticeLocked(player *Player, id int, loc Location) *GameObject {
	obj := r.findObject(player, id, loc)
	if obj == nil && player != nil && r.rightsSource != nil && r.rightsSource(player) == RightsDeveloper {
		player.SendMessage(fmt.Sprintf("@red@Object with id %d does not exist.", id))
	}
	return obj
}

// Find returns the loc a player sees with an id on a tile:
// MapObjects.getPrivateArea (MapObjects.ts:49-54), the public
// lookup the object-click path uses
// (ObjectActionPacketListener.ts:48). When no such loc stands
// there and the player is a DEVELOPER (rights id 4,
// PlayerRights.ts:6, read through the rights source), the player
// is told "@red@Object with id <id> does not exist." via
// SendMessage, exactly as TS sends it. With no rights source
// installed the notice never fires (every player reads as
// RightsNone). A nil player skips the owner branch and the
// notice. See findObject for the lookup order, including the
// bridge-tile z-1 fallback.
func (r *ObjectRegistry) Find(player *Player, id int, loc Location) *GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.findWithNoticeLocked(player, id, loc)
}

// StillStands reports whether the loc a player clicked, or one
// just like it, still stands where they clicked: nobody removed
// or changed it while they walked there (MapObjects.stillStands,
// MapObjects.ts:87-92). quiet selects the notice-free lookup for
// checks that run every tick of a walk; loud (quiet=false) is
// the Find lookup with its developer notice. "Just like it"
// means same id, type, and face on the same tile — a different
// instance passes. A nil object matches only a missing loc
// (Go-only nil tolerance; TS would throw).
func (r *ObjectRegistry) StillStands(player *Player, id int, loc Location, object *GameObject, quiet bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	var current *GameObject
	if quiet {
		current = r.findObject(player, id, loc)
	} else {
		current = r.findWithNoticeLocked(player, id, loc)
	}
	if object == nil {
		return current == nil
	}
	return current == object || (current != nil && current.ID() == object.ID() &&
		current.Type() == object.Type() && current.Face() == object.Face())
}

// Exists reports whether any runtime object stands on a tile:
// ObjectManager.exists (ObjectManager.ts:422-425). The id
// argument is IGNORED, exactly as in TS ("any runtime object on
// the tile counts, whatever its id") — callers that need an
// id-specific check want Get or Find.
func (r *ObjectRegistry) Exists(id int, loc Location) bool {
	_ = id // Ignored, as in TS. See the doc comment.
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byTile[loc]) > 0
}

// ExistsLocation is Exists without the ignored id argument:
// ObjectManager.existsLocation (ObjectManager.ts:427-429).
func (r *ObjectRegistry) ExistsLocation(loc Location) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byTile[loc]) > 0
}

// ExistsObject reports whether this exact instance is the loc
// get() returns for its id and tile: MapObjects.exists
// (MapObjects.ts:199-201). A nil object never exists.
func (r *ObjectRegistry) ExistsObject(obj *GameObject) bool {
	if obj == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.getByID(obj.ID(), obj.Location()) == obj
}

// HideMapObjectFor resolves the map object a hide request names:
// the lookup half of ObjectManager.hideMapObjectFor
// (ObjectManager.ts:59-67), which finds the base loc via
// MapObjects.get(id, location, null). It returns the base object
// and true, or (nil, false) when no such loc stands there.
//
// SPLIT WITH THE SESSION LAYER: TS also records the object in
// the per-player hiddenFor map and sends the removal
// immediately. Per-player hidden tracking and all frame emission
// belong to Worker 2's session layer, so this method records
// nothing and sends nothing — the session layer calls it to
// resolve the object, records the hide in its own state, and
// emits the removal. The player argument is unused here (TS
// uses it only for the send) and is kept so call sites keep
// their TS shape and future area resolution has the player.
func (r *ObjectRegistry) HideMapObjectFor(player *Player, id int, loc Location) (*GameObject, bool) {
	_ = player // Unused: see the doc comment.
	r.mu.Lock()
	defer r.mu.Unlock()
	base := r.getByID(id, loc)
	if base == nil {
		return nil, false
	}
	return base, true
}

// ShowMapObjectFor resolves the map object an unhide request
// names: the lookup half of ObjectManager.showMapObjectFor
// (ObjectManager.ts:70-78). It returns the base object and true,
// or (nil, false) when no such loc stands there. The hidden-map
// check ("only re-send if it was hidden") and the send belong to
// the session layer, as in HideMapObjectFor; the player argument
// is likewise unused here.
func (r *ObjectRegistry) ShowMapObjectFor(player *Player, id int, loc Location) (*GameObject, bool) {
	_ = player // Unused: see the doc comment.
	r.mu.Lock()
	defer r.mu.Unlock()
	base := r.getByID(id, loc)
	if base == nil {
		return nil, false
	}
	return base, true
}

// RemovedObjects returns the removed-objects list in push order:
// World.getRemovedObjects() (World.ts:1099-1101). The session
// layer sends these removals before additions on region change
// (ObjectManager.onRegionChange, ObjectManager.ts:196-209). The
// slice is a copy.
func (r *ObjectRegistry) RemovedObjects() []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.removedObjects) == 0 {
		return nil
	}
	return append([]*GameObject(nil), r.removedObjects...)
}

// WorldObjects returns the runtime object list in registration
// order: World.getObjects() (World.ts:1095-1097). The slice is a
// copy.
func (r *ObjectRegistry) WorldObjects() []*GameObject {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.worldObjects) == 0 {
		return nil
	}
	return append([]*GameObject(nil), r.worldObjects...)
}

// Deltas returns the spawn/removal deltas collected since the
// last ClearDeltas, in recording order. The session layer reads
// them once per tick and clears them (see the file header). The
// slice is a copy: clearing while a consumer holds the copy is
// safe.
func (r *ObjectRegistry) Deltas() []ObjectDelta {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.deltas) == 0 {
		return nil
	}
	return append([]ObjectDelta(nil), r.deltas...)
}

// ClearDeltas discards all collected deltas. The session layer
// calls it after sending each tick's deltas; the boot-time spawn
// appliers call it because their deltas reach no player.
func (r *ObjectRegistry) ClearDeltas() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.deltas = nil
}

// DiscardPlayer drops a departed player's owner-object entries:
// the explicit cleanup Go needs because it has no WeakMap (see
// the ObjectRegistry doc). The world calls it when a player is
// removed (World.drainRemovePlayers). It drops ONLY the owner
// map: world-index entries for the player's owned objects stay,
// exactly as TS leaves them when the WeakMap entry is collected
// — they leave only through Deregister.
func (r *ObjectRegistry) DiscardPlayer(p *Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.ownerObjects, p)
}
