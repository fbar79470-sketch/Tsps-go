package game

// This file is the Phase 4 seam: the small shared types the collision,
// route-finding, and movement-queue pieces agree on. It exists so the
// three pieces can be built against identical interfaces. The
// implementations live in regions.go (RegionManager), routefinding.go
// (RouteFinder), and movement_queue.go (MovementQueue).

// CollisionLookup answers the clipping flags for one tile: the Go form
// of RsmodRouteFinding's CollisionLookup
// (RsmodRouteFinding.ts: CollisionLookup type) and of
// RegionManager.getClipping (RegionManager.ts:472-488). A missing or
// unloaded region answers 0 (open), exactly as TS getClipping does
// when no Region exists for the tile.
type CollisionLookup func(x, y, level int) int32

// RegionDataSource is the region-bytes surface RegionManager needs:
// the region ids the cache holds, and one region's decoded terrain +
// object bytes. It is the Go form of the CacheMaps surface
// RegionManager.loadMapFiles reads (CacheMaps.getRegionIds /
// getRegion, via CacheMaps.ts). The cmd/server adapter implements it
// over *cache.CacheMaps; tests substitute fakes. Keeping this an
// interface is what lets internal/game stay free of an
// internal/cache import (the established seam convention: see
// SessionPort, FlushSource, ItemWeightSource).
type RegionDataSource interface {
	// RegionIDs lists every region id the index holds.
	RegionIDs() []int
	// RegionData returns one region's terrain bytes and object bytes
	// (object bytes nil when the region has no object file), or
	// ok=false when the region is absent or unreadable.
	RegionData(regionID int) (terrain, objects []byte, ok bool)
}

// ObjectDef is the Loc-definition surface RegionManager needs to lay
// object clipping: footprint, clip type, and projectile blocking. It
// is the Go form of the ObjectDefinition accessors
// RegionManager.addObjectClipping reads (ObjectDefinition.ts:
// getSizeX/getSizeY, isSolid = clipType != 0, isClippedDecoration =
// clipType == 1, isImpenetrable = projectileBlocking).
type ObjectDef struct {
	SizeX int
	SizeY int
	// ClipType is the cache clipType: 0 = not solid, 1 = clipped
	// decoration, other non-zero = solid.
	ClipType int
	// BlocksProjectile is the cache blocksProjectile flag
	// (ObjectDefinition.isImpenetrable).
	BlocksProjectile bool
}

// ObjectDefSource resolves an object id to its ObjectDef, or
// ok=false when no definition exists (TS addObjectClipping returns
// early for a missing def). The cmd/server adapter implements it
// over *cache.CacheDefinitions.
type ObjectDefSource interface {
	ObjectDef(id int) (ObjectDef, bool)
}

// ObjectPlacement is one placed map object for clipping purposes:
// the Go form of the GameObject fields RegionManager.addObjectClipping
// reads (id, location, type, face). Region loading builds placements
// directly from the object stream and records them per region (see
// RegionManager.Placements); the object registry (objects.go) imports
// them as map-original GameObjects.
type ObjectPlacement struct {
	ID        int
	X, Y      int
	Level     int
	Type      int
	Direction int
}

// MovementCollision is the collision surface MovementQueue steps
// against: can an actor of the given size step from start to end?
// It is the Go form of RegionManager.canMovestart
// (RegionManager.ts:804-806) as MovementQueue.canWalkTo calls it
// (always with size x size). *RegionManager satisfies it; tests
// substitute fakes, and a nil MovementCollision means open ground
// (TS missing-region semantics: zero flags everywhere).
type MovementCollision interface {
	CanMoveStart(start, end Location, size int) bool
}
