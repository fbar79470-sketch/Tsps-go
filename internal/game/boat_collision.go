package game

import "math"

// Deck collision, ported from game/content/sailing/BoatCollision.ts:
// whether a boat hull may cover a main-world tile. Based on rsmod's
// BoatCollision.kt (ISC license), using the world entity type's
// hull bounds instead of the whole template zone.

// SailableTileCheck answers whether a boat hull may cover a
// main-world tile (BoatCollision.ts: SailableTileCheck). It is
// BoatManager.IsSailable in production.
type SailableTileCheck func(x, y, level int) bool

// Hull sample spacing in fine units (half a tile), so thin hulls
// still cover every tile.
const hullSampleStep = 64

// HullTiles returns the main-world tiles covered by the boat's
// hull at a candidate position and facing. The hull rectangle is
// sampled every half tile and rotated about the boat's centre.
// Tiles are packed as x*0x8000+y, exactly as TS packs them.
func HullTiles(boat *Boat, fineX, fineY, angle int) map[int]struct{} {
	radians := float64(angle) * math.Pi / 1024
	cos, sin := math.Cos(radians), math.Sin(radians)
	hull := boat.Hull
	tiles := make(map[int]struct{})
	for lx := -float64(hull.Width)/2 + hullSampleStep/2; lx < float64(hull.Width)/2; lx += hullSampleStep {
		for ly := -float64(hull.Length)/2 + hullSampleStep/2; ly < float64(hull.Length)/2; ly += hullSampleStep {
			x := lx + float64(hull.OffsetX)
			y := ly + float64(hull.OffsetY)
			// Same rotation as angleToFineDelta: local
			// "forward" (0, -1) maps to the facing.
			wx := float64(fineX) + x*cos + y*sin
			wy := float64(fineY) + y*cos - x*sin
			tiles[packTile(
				int(math.Floor(wx/FineUnitsPerTile)),
				int(math.Floor(wy/FineUnitsPerTile)),
			)] = struct{}{}
		}
	}
	return tiles
}

// CanOccupy reports whether the boat may move to a candidate
// position and facing. Only tiles the hull newly covers are
// checked, so a boat that ends up overlapping land can still
// sail off it.
func CanOccupy(boat *Boat, fineX, fineY, angle int, isSailable SailableTileCheck) bool {
	before := HullTiles(boat, boat.FineX, boat.FineY, boat.Angle)
	for tile := range HullTiles(boat, fineX, fineY, angle) {
		if _, ok := before[tile]; ok {
			continue
		}
		if !isSailable(unpackTileX(tile), unpackTileY(tile), boat.Level) {
			return false
		}
	}
	return true
}

func packTile(x, y int) int { return x*0x8000 + y }

func unpackTileX(tile int) int { return floorDiv(tile, 0x8000) }

// unpackTileY is the floor-modulo form of TS `tile % 0x8000`: the
// two agree for every non-negative packed tile (all tiles a hull
// can sample in the real map); for a negative tile TS % keeps the
// dividend's sign and mis-unpacks, while this round-trips.
func unpackTileY(tile int) int { return tile - floorDiv(tile, 0x8000)*0x8000 }
