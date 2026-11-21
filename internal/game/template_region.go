package game

import "sync"

// TemplateRegionData ports game/collision/TemplateRegionData.ts
// (93 lines): what a templated instance copies from a cache
// region — tile settings and locs grouped by 8x8 chunk — plus
// the decode-once cache over the cache's map files.
//
// DATA SOURCE. TS reads through CacheMaps.getRegion, a static
// over the global cache pipeline. Go reads through the existing
// RegionDataSource seam (collision.go), the same interface
// RegionManager loads through: the coordinator installs the
// cache-backed adapter once at boot with
// SetTemplateRegionSource, and template decoding never touches
// the cache package directly (internal/game must not import
// internal/cache). No new cache copies are made and no new
// cache reads exist outside this seam: GetTemplateRegion /
// ReadTemplateRegion call RegionData, exactly as RegionManager
// does. With no source installed, every lookup answers nil —
// template chunks decode to nothing, and instances built from
// them are unmapped (all tiles blocked), matching the TS
// behavior when CacheMaps has no region.
//
// CACHING. TS caches decoded regions in a module Map, including
// nulls (TemplateRegionData.ts:27-36). The Go cache below does
// the same: a region that fails to decode is remembered as a
// miss, so a missing region is not re-read on every chunk
// lookup. Installing a new source clears the cache. The cache
// is mutex-guarded: template lookups run on the tick goroutine
// in production, but GetTemplateRegion is also a hub Core
// surface (core.getTemplateRegion), and a content call must
// never race the tick's collision rebuild.
//
// TERRAIN FORMAT. The settings decode switches format at cache
// revision 209 (TemplateRegionData.ts:49, mirroring
// RegionManager.ts:924); the flag arrives with the source, as
// it does for RegionManager, and is true for the rev-242
// target. The decoder itself is total over arbitrary input:
// mapReader reads past the end return 0 (see regions_decode.go),
// so a truncated stream decodes as terminators, exactly as TS
// reads undefined bytes as 0.

// TemplateLoc is one loc as the cache map file stores it:
// a region-local tile, its own plane, before bridges
// (TemplateRegionData.ts:9-16).
type TemplateLoc struct {
	ID     int
	LocalX int
	LocalY int
	Plane  int
	Type   int
	Face   int
}

// TemplateRegion is what a templated instance copies from a
// cache region (TemplateRegionData.ts:19-25): tile settings
// (`1` blocked, `2` bridge, `4` under roof) at
// `(plane << 12) | (x << 6) | y`, and locs grouped by 8x8 chunk
// at TemplateChunkKey(plane, chunkX, chunkY).
//
// Settings always has length 4*64*64; Chunks always has length
// 4*64 (256), with nil slices for empty chunks. Both are
// immutable after decoding: the templated collision builder
// reads them, never writes them.
type TemplateRegion struct {
	Settings []uint8
	Chunks   [][]TemplateLoc
}

// TemplateChunkKey answers the chunk index within a region
// (TemplateRegionData.ts:29-31): `(plane << 6) | (chunkX << 3)
// | chunkY`, with the chunk coordinates masked to the 8 chunks
// of a region. Callers pass region-local chunk coordinates
// (chunkX>>3 within the region is NOT applied here — the mask
// does the region-local reduction, as in TS, where callers
// pass absolute chunk coordinates and the & 7 folds them).
func TemplateChunkKey(plane, chunkX, chunkY int) int {
	return (plane << 6) | ((chunkX & 7) << 3) | (chunkY & 7)
}

// templateRegionState is the decode-once cache and its source.
// One global instance mirrors the TS module-level Map; see the
// file comment.
var (
	templateRegionMu        sync.Mutex
	templateRegionSource    RegionDataSource
	templateRegionNewFormat bool
	templateRegionCache     = make(map[int]*TemplateRegion)
)

// SetTemplateRegionSource installs the region-bytes source
// template decoding reads through, and the terrain-format flag
// for the active cache revision (true at revision >= 209).
// Installing a source clears the decode-once cache, so a
// source swap never serves stale decodes. A nil source means
// no template region ever decodes (all lookups answer nil).
// Call once at boot, before ticking starts; tests install fakes
// and restore with a cleanup.
func SetTemplateRegionSource(source RegionDataSource, newTerrainFormat bool) {
	templateRegionMu.Lock()
	defer templateRegionMu.Unlock()
	templateRegionSource = source
	templateRegionNewFormat = newTerrainFormat
	templateRegionCache = make(map[int]*TemplateRegion)
}

// GetTemplateRegion decodes (once) the cache map files of a
// region that instances use as templates
// (TemplateRegionData.ts:34-40). A cached decode — including a
// cached miss (nil) — is returned without re-reading. It
// returns nil when the region has no cache map data or no
// source is installed.
func GetTemplateRegion(regionID int) *TemplateRegion {
	templateRegionMu.Lock()
	defer templateRegionMu.Unlock()
	if region, ok := templateRegionCache[regionID]; ok {
		return region
	}
	region := readTemplateRegionLocked(regionID)
	templateRegionCache[regionID] = region
	return region
}

// ReadTemplateRegion reads a cache region without retaining its
// decoded locs or loading live collision/objects
// (TemplateRegionData.ts:43-46): every call decodes fresh, and
// the decode-once cache is neither read nor written. It returns
// nil when the region has no cache map data or no source is
// installed. This is the core.readTemplateRegion surface.
func ReadTemplateRegion(regionID int) *TemplateRegion {
	templateRegionMu.Lock()
	defer templateRegionMu.Unlock()
	return readTemplateRegionLocked(regionID)
}

// readTemplateRegionLocked decodes one region from the
// installed source. The caller holds templateRegionMu. TS:
// `data?.terrainData ? decode(...) : null` — a missing region
// (or missing terrain) yields null, while an empty-but-present
// terrain array still decodes (an empty Uint8Array is truthy
// in JS); Go mirrors that with the nil check below.
func readTemplateRegionLocked(regionID int) *TemplateRegion {
	if templateRegionSource == nil {
		return nil
	}
	terrain, objects, ok := templateRegionSource.RegionData(regionID)
	if !ok || terrain == nil {
		return nil
	}
	return decodeTemplateRegion(terrain, objects, templateRegionNewFormat)
}

// decodeTemplateRegion decodes one region's terrain and object
// bytes into a TemplateRegion (TemplateRegionData.ts:48-91).
// It is a pure function of its inputs, so tests can drive it
// with hand-built streams without installing a source.
func decodeTemplateRegion(terrain, objects []byte, newTerrainFormat bool) *TemplateRegion {
	settings := make([]uint8, 4*64*64)
	ground := &mapReader{buf: terrain}
	for z := 0; z < 4; z++ {
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				for {
					var opcode int
					if newTerrainFormat {
						opcode = ground.readUShort()
					} else {
						opcode = ground.readUnsignedByte()
					}
					if opcode == 0 {
						break
					}
					if opcode == 1 {
						// The height byte: always one byte,
						// in both formats (TS reads
						// readUnsignedByte unconditionally).
						ground.readUnsignedByte()
						break
					}
					if opcode <= 49 {
						if newTerrainFormat {
							ground.readUShort()
						} else {
							ground.readUnsignedByte()
						}
					} else if opcode <= 81 {
						settings[(z<<12)|(x<<6)|y] = uint8(opcode - 49)
					}
					// Opcodes above 81 match no branch in
					// TS either: the loop reads the next
					// opcode without consuming anything
					// further. Do not "fix" this.
				}
			}
		}
	}

	chunks := make([][]TemplateLoc, 4*64)
	if objects != nil {
		stream := &mapReader{buf: objects}
		id := -1
		for {
			idDelta := stream.readSmart()
			if idDelta == 0 {
				break
			}
			id += idDelta
			packed := 0
			for {
				packedDelta := stream.getUSmart()
				if packedDelta == 0 {
					break
				}
				packed += packedDelta - 1
				attributes := stream.readUnsignedByte()
				loc := TemplateLoc{
					ID:     id,
					LocalX: (packed >> 6) & 0x3f,
					LocalY: packed & 0x3f,
					Plane:  (packed >> 12) & 3,
					Type:   attributes >> 2,
					Face:   attributes & 3,
				}
				key := TemplateChunkKey(loc.Plane, loc.LocalX>>3, loc.LocalY>>3)
				chunks[key] = append(chunks[key], loc)
			}
		}
	}
	return &TemplateRegion{Settings: settings, Chunks: chunks}
}
