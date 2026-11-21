package game

// Region map-file decoding: the terrain and object stream readers
// of RegionManager.loadMapFiles (RegionManager.ts:885-1031).
//
// The reader is local to this file on purpose: internal/game must
// not import internal/cache (the seam convention), so the small
// Buffer.ts surface loadMapFiles actually uses is mirrored here
// rather than reused from the cache ByteBuffer. Only the four
// reads loadMapFiles needs are ported: readUnsignedByte,
// readUShort, getUSmart, readSmart.

// mapReader mirrors the Buffer class (Buffer.ts) over a byte slice.
// Reads past the end return 0, as TS reads do: indexing past a
// Uint8Array yields undefined, and `undefined & 0xff` is 0, so a
// truncated stream decodes as terminator bytes (tileType 0,
// readSmart 0) rather than throwing. The offset clamps at the end
// of the buffer instead of advancing without bound. This makes the
// decoders below total over arbitrary input: the terrain loop
// always terminates (a 0 tileType breaks it) and the object loops
// always terminate (a 0 smart breaks them), matching what TS does
// with the same bytes.
type mapReader struct {
	buf []byte
	off int
}

// readUnsignedByte mirrors Buffer.readUnsignedByte (Buffer.ts:26-28).
func (r *mapReader) readUnsignedByte() int {
	if r.off >= len(r.buf) {
		return 0
	}
	b := r.buf[r.off]
	r.off++
	return int(b)
}

// readUShort mirrors Buffer.readUShort (Buffer.ts:38-40): big-endian.
func (r *mapReader) readUShort() int {
	return (r.readUnsignedByte() << 8) + r.readUnsignedByte()
}

// getUSmart mirrors Buffer.getUSmart (Buffer.ts:55-62): a leading
// byte below 128 is the value itself, otherwise the value is the
// big-endian ushort minus 32768.
func (r *mapReader) getUSmart() int {
	if r.off >= len(r.buf) {
		return 0
	}
	if r.buf[r.off] < 128 {
		return r.readUnsignedByte()
	}
	return r.readUShort() - 32768
}

// readSmart mirrors Buffer.readSmart (Buffer.ts:64-75), including
// the 32767-accumulation loop: successive 32767 values accumulate
// until a smaller value terminates the sum. The TS try/catch
// returning -1 is not reproduced: getUSmart cannot throw (see the
// mapReader note), so the catch is unreachable for these reads.
func (r *mapReader) readSmart() int {
	value := 0
	for {
		ptr := r.getUSmart()
		if ptr != 32767 {
			value += ptr
			return value
		}
		value += 32767
	}
}

// isWaterOverlayID mirrors isWaterOverlayId (TerrainWater.ts): raw
// terrain overlay values are cache overlay ids plus one, and ice
// (overlay 195, raw 196) is land.
func isWaterOverlayID(rawOverlayID int) bool {
	return rawOverlayID == 6 || rawOverlayID == 7 || rawOverlayID == 442 ||
		rawOverlayID == 444 || (rawOverlayID >= 445 && rawOverlayID <= 625)
}

// heightMap is the per-plane 64x64 tile-height grid loadMapFiles
// builds from the terrain stream (RegionManager.ts:915-918):
// heightMap[plane][tileX][tileY].
type heightMap [4][64][64]int

// loadRegion decodes one region's map files into its clip grid,
// roof tiles and water tiles: the body of loadMapFiles
// (RegionManager.ts:900-1012) with the out-of-scope steps removed:
//
//   - No MapRegionReplacementManager lookup: replacement data is
//     never consulted; the cache bytes are always decoded.
//   - No replayWorldObjects: the removed-object replay lives in
//     the object registry's import step (objects.go), which
//     skips placements matching runtime-removed objects and
//     lifts their clipping; the loadRemovals/wasRemovedOnLoad
//     bookkeeping is NOT STARTED.
//   - No PluginManager.emitRegionLoaded: no plugin system.
//
// The MapObjects population: TS routes every decoded object
// through addObject -> MapObjects.add (RegionManager.ts:1000-1002,
// MapObjects.ts:203-229), whose only collision effect is
// addObjectClipping (MapObjects.ts:229). The placement still goes
// straight to AddObjectClipping here, and is ALSO recorded on
// the region (placements + loadSeq) so the object registry can
// import it into the tile map through the Placements accessor
// without re-decoding; the registry import adds no clipping of
// its own.
//
// A region with no source data stays unloaded (TS: `if
// (!gFileData)` returns early, lines 910-913) and a later lookup
// retries the load, because loadMapFiles runs again whenever no
// load is in progress.
func (m *RegionManager) loadRegion(r *region) {
	if m.source == nil {
		return
	}
	terrain, objects, ok := m.source.RegionData(r.id)
	if !ok || terrain == nil {
		return
	}
	absX := (r.id >> 8) * 64
	absY := (r.id & 0xff) * 64
	heights, roofTiles, waterTiles := decodeTerrain(terrain, m.newTerrainFormat)
	r.waterTiles = waterTiles
	// Blocked-tile clipping from the height map
	// (RegionManager.ts:960-971): any tile whose height value has
	// bit 1 set is blocked, on a plane lowered by one when the
	// PLANE-1 tile at the same coordinates has bit 2 set — the
	// adjustment reads heightMap[1] for every plane, including
	// plane 1 itself. The clip is written to this region directly,
	// as TS passes the region explicitly to addClipping (line 967).
	for i := 0; i < 4; i++ {
		for i2 := 0; i2 < 64; i2++ {
			for i3 := 0; i3 < 64; i3++ {
				if (heights[i][i2][i3] & 1) == 1 {
					height := i
					if (heights[1][i2][i3] & 2) == 2 {
						height--
					}
					if height >= 0 && height <= 3 {
						r.addClip(absX+i2, absY+i3, height, 0x200000)
					}
				}
			}
		}
	}
	if objects != nil {
		var placements []ObjectPlacement
		decodeObjects(objects, heights, absX, absY, func(p ObjectPlacement) {
			placements = append(placements, p)
			m.AddObjectClipping(p)
		})
		r.placements = placements
	} else {
		r.placements = nil
	}
	r.roofTiles = roofTiles
	r.loaded = true
	r.loadSeq++
}

// decodeTerrain mirrors the terrain loop of loadMapFiles
// (RegionManager.ts:924-957). Tiles iterate planes, then x, then y.
// Each tile reads tile types until a terminator: 0 ends the tile, 1
// consumes one more byte (a plain unsigned byte in BOTH formats,
// line 933) and ends the tile, 2-49 read an overlay id (ushort in
// the new format, byte in the old), 50-81 set the height value
// (tileType - 49) and record a roof bit when the value has
// TILE_FLAG_UNDER_ROOF set. A tileType above 81 matches no branch
// in TS, so the loop simply reads the next tileType; that is
// preserved. Water is recorded only on plane 0, from the LAST
// overlay id read for the tile (overlayId is overwritten by each
// 2-49 entry, and stays 0 when none was read).
func decodeTerrain(data []byte, newFormat bool) (heightMap, []byte, []byte) {
	var heights heightMap
	roofTiles := make([]byte, 2048)
	waterTiles := make([]byte, 512)
	stream := &mapReader{buf: data}
	for z := 0; z < 4; z++ {
		for tileX := 0; tileX < 64; tileX++ {
			for tileY := 0; tileY < 64; tileY++ {
				overlayID := 0
				for {
					var tileType int
					if newFormat {
						tileType = stream.readUShort()
					} else {
						tileType = stream.readUnsignedByte()
					}
					if tileType == 0 {
						break
					} else if tileType == 1 {
						stream.readUnsignedByte()
						break
					} else if tileType <= 49 {
						if newFormat {
							overlayID = stream.readUShort()
						} else {
							overlayID = stream.readUnsignedByte()
						}
					} else if tileType <= 81 {
						heights[z][tileX][tileY] = tileType - 49
						if ((tileType - 49) & tileFlagUnderRoof) != 0 {
							index := (z << 12) | (tileX << 6) | tileY
							roofTiles[index>>3] |= 1 << (index & 7)
						}
					}
				}
				if z == 0 && isWaterOverlayID(overlayID) {
					index := (tileX << 6) | tileY
					waterTiles[index>>3] |= 1 << (index & 7)
				}
			}
		}
	}
	return heights, roofTiles, waterTiles
}

// decodeObjects mirrors the object stream loop of loadMapFiles
// (RegionManager.ts:975-1003). Object ids are readSmart deltas from
// -1; each object's placements are getUSmart location deltas from
// 0, where location packs (height << 12) | (localX << 6) | localY.
// The hash byte is read BEFORE the bounds check (TS lines 986-990),
// so a skipped placement still consumes its byte and the stream
// stays aligned. The bounds check itself can never fire for a
// non-negative location — localX/localY are masked with 0x3f — but
// it is preserved verbatim. Placements whose plane-1 tile has
// height bit 2 set drop one plane (the same adjustment as the
// blocked-tile pass), and placements outside planes 0-3 are
// dropped.
func decodeObjects(data []byte, heights heightMap, absX, absY int, place func(ObjectPlacement)) {
	stream := &mapReader{buf: data}
	objectID := -1
	for {
		incr := stream.readSmart()
		if incr == 0 {
			break
		}
		objectID += incr
		location := 0
		for {
			incr2 := stream.getUSmart()
			if incr2 == 0 {
				break
			}
			location += incr2 - 1
			localX := (location >> 6) & 0x3f
			localY := location & 0x3f
			height := location >> 12
			hash := stream.readUnsignedByte()
			typ := hash >> 2
			direction := hash & 0x3
			if localX < 0 || localX >= 64 || localY < 0 || localY >= 64 {
				continue
			}
			if (heights[1][localX][localY] & 2) == 2 {
				height--
			}
			if height >= 0 && height <= 3 {
				place(ObjectPlacement{
					ID:        objectID,
					X:         absX + localX,
					Y:         absY + localY,
					Level:     height,
					Type:      typ,
					Direction: direction,
				})
			}
		}
	}
}
