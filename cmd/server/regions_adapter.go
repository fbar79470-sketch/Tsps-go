package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// cacheRegionSources adapts the cache facades to the collision
// seams in internal/game (game.RegionDataSource and
// game.ObjectDefSource, see internal/game/collision.go), so the
// RegionManager can read region bytes and object definitions
// without internal/game importing internal/cache. Either facade
// may be nil (NoCache mode): a nil maps facade answers no regions
// and a nil definitions facade answers no definitions, and the
// RegionManager degrades to open ground.
type cacheRegionSources struct {
	maps *cache.CacheMaps
	defs *cache.CacheDefinitions
}

// newCacheRegionSources builds the adapter over the boot-loaded
// facades (either may be nil).
func newCacheRegionSources(maps *cache.CacheMaps, defs *cache.CacheDefinitions) *cacheRegionSources {
	return &cacheRegionSources{maps: maps, defs: defs}
}

// RegionIDs delegates to CacheMaps.GetRegionIDs
// (CacheMaps.getRegionIds, CacheMaps.ts:29-40).
func (s *cacheRegionSources) RegionIDs() []int {
	if s.maps == nil {
		return nil
	}
	return s.maps.GetRegionIDs()
}

// RegionData delegates to CacheMaps.GetRegion (CacheMaps.getRegion,
// CacheMaps.ts:42-65): terrain bytes plus object bytes (nil when
// the region has no object file), or ok=false when the region is
// absent or unreadable.
func (s *cacheRegionSources) RegionData(regionID int) (terrain, objects []byte, ok bool) {
	if s.maps == nil {
		return nil, nil, false
	}
	data, ok := s.maps.GetRegion(regionID)
	if !ok {
		return nil, nil, false
	}
	return data.TerrainData, data.ObjectData, true
}

// ObjectDef maps a Loc definition to the ObjectDef surface
// RegionManager.addObjectClipping reads: SizeX/SizeY, ClipType
// (isSolid = clipType != 0, isClippedDecoration = clipType == 1),
// and BlocksProjectile (isImpenetrable). See
// ObjectDefinition.ts:76-78,121-125 and the ObjectDef comment in
// internal/game/collision.go.
//
// A missing definition answers ok=false, mirroring
// ObjectDefinition.forId (ObjectDefinition.ts:61-62), which
// returns undefined for ids outside [0, totalObjects), where
// totalObjects is CacheDefinitions.getCounts().objects. The Go
// loaders answer unknown ids with a default-initialized LocType
// instead of nil, so the bound check — not a nil check — is what
// detects a missing definition; the nil guard covers only a loader
// that breaks that contract.
func (s *cacheRegionSources) ObjectDef(id int) (game.ObjectDef, bool) {
	if s.defs == nil {
		return game.ObjectDef{}, false
	}
	if id < 0 || id >= s.defs.GetCounts().Objects {
		return game.ObjectDef{}, false
	}
	def := s.defs.GetObject(id)
	if def == nil {
		return game.ObjectDef{}, false
	}
	return game.ObjectDef{
		SizeX:            def.SizeX,
		SizeY:            def.SizeY,
		ClipType:         def.ClipType,
		BlocksProjectile: def.BlocksProjectile,
	}, true
}

var _ game.RegionDataSource = (*cacheRegionSources)(nil)
var _ game.ObjectDefSource = (*cacheRegionSources)(nil)
