package cache

import "fmt"

// CacheMaps reads region terrain and object data from the maps
// index (index 5). It ports CacheMaps (CacheMaps.ts).
//
// Adaptation: the TypeScript CacheMaps is a static class with lazy
// state over the CachePipeline store and XTEA map. Here it is a
// struct built over a maps index plus the Keys map, with no global
// state. The named/unnamed distinction, the mX_Y/lX_Y archive
// names, and the XTEA handling (object data only, keyed by the
// object archive id) are preserved exactly.

// MapIndex is the maps-index surface CacheMaps needs: the
// ArchiveSource plus name lookup, existence checks, and file
// reads. *CacheIndex satisfies it; tests substitute a fake.
type MapIndex interface {
	ArchiveSource
	// Named reports whether the reference table carries names.
	Named() bool
	// ArchiveID returns the archive id for a name, or -1.
	ArchiveID(name string) int
	// ArchiveExists reports whether the index lists the archive.
	ArchiveExists(id int) bool
	// ArchiveIDs lists every archive id in table order.
	ArchiveIDs() []int
	// GetFile returns one file's bytes, or found=false.
	GetFile(archiveID, fileID int, key *[4]int32) ([]byte, bool, error)
}

// MapData is one region's decoded data: terrain is always present,
// ObjectData is nil when the region has no object file. It is the
// MapData type (CacheMaps.ts:5).
type MapData struct {
	TerrainData []byte
	ObjectData  []byte
}

// CacheMaps serves region data from the maps index.
type CacheMaps struct {
	maps  MapIndex
	named bool
	keys  Keys
}

// NewCacheMaps builds a CacheMaps over the maps index with the
// region XTEA keys.
func NewCacheMaps(index MapIndex, keys Keys) *CacheMaps {
	return &CacheMaps{maps: index, named: index.Named(), keys: keys}
}

// NewCacheMapsFromStore builds a CacheMaps over the real Store,
// loading the maps index reference table.
func NewCacheMapsFromStore(store *Store, keys Keys) (*CacheMaps, error) {
	index, err := NewCacheIndexFromStore(IndexMaps, store)
	if err != nil {
		return nil, err
	}
	return NewCacheMaps(index, keys), nil
}

// GetArchiveIDs returns the terrain and object archive ids for a
// region, or false when either is missing. It mirrors
// CacheMaps.getArchiveIds (CacheMaps.ts:19-27): in a named index
// the archives are `mX_Y` and `lX_Y` (X = region >> 8, Y = region
// & 0xff); in an unnamed index both are the region id itself, and
// both must exist.
func (m *CacheMaps) GetArchiveIDs(regionID int) (terrainFile, objectFile int, ok bool) {
	mapX := regionID >> 8
	mapY := regionID & 0xff
	if m.named {
		terrainFile = m.maps.ArchiveID(fmt.Sprintf("m%d_%d", mapX, mapY))
		objectFile = m.maps.ArchiveID(fmt.Sprintf("l%d_%d", mapX, mapY))
	} else {
		terrainFile = regionID
		objectFile = regionID
	}
	if terrainFile != -1 && objectFile != -1 &&
		m.maps.ArchiveExists(terrainFile) && m.maps.ArchiveExists(objectFile) {
		return terrainFile, objectFile, true
	}
	return 0, 0, false
}

// GetRegionIDs returns every region id the index holds. It mirrors
// CacheMaps.getRegionIds (CacheMaps.ts:29-40): in an unnamed index
// the archive ids themselves (filtered to 0..0xffff); in a named
// index every 256x256 coordinate whose archives exist.
func (m *CacheMaps) GetRegionIDs() []int {
	if !m.named {
		var ids []int
		for _, id := range m.maps.ArchiveIDs() {
			if id >= 0 && id <= 0xffff {
				ids = append(ids, id)
			}
		}
		return ids
	}
	var ids []int
	for mapX := 0; mapX < 256; mapX++ {
		for mapY := 0; mapY < 256; mapY++ {
			id := (mapX << 8) | mapY
			if _, _, ok := m.GetArchiveIDs(id); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// GetRegion returns a region's terrain and object data, or false
// when the region is absent or unreadable. It mirrors
// CacheMaps.getRegion (CacheMaps.ts:42-65): terrain is file 0,
// read without a key; object data is file 0 in a named index and
// file 1 in an unnamed index, decrypted with the XTEA key for the
// object archive id (all-zero when the region has no key). Any
// read or decode failure yields false, as the TypeScript try/catch
// returns null.
func (m *CacheMaps) GetRegion(regionID int) (MapData, bool) {
	terrainFile, objectFile, ok := m.GetArchiveIDs(regionID)
	if !ok {
		return MapData{}, false
	}
	objectFileID := 0
	if !m.named {
		objectFileID = 1
	}
	terrainData, ok, err := m.maps.GetFile(terrainFile, 0, nil)
	if err != nil || !ok || terrainData == nil {
		return MapData{}, false
	}
	key := m.keys.Get(objectFile)
	objectData, _, err := m.maps.GetFile(objectFile, objectFileID, &key)
	if err != nil {
		return MapData{}, false
	}
	return MapData{TerrainData: terrainData, ObjectData: objectData}, true
}
