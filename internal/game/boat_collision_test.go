package game

import "testing"

func TestHullTilesCoverTheHull(t *testing.T) {
	boat := testBoat()
	tiles := HullTiles(boat, boat.FineX, boat.FineY, 0)
	// The hull trails south of the position: x tile 2999,
	// y tiles 2996..2999.
	for y := 2996; y <= 2999; y++ {
		if _, ok := tiles[packTile(2999, y)]; !ok {
			t.Errorf("hull must cover (2999, %d)", y)
		}
	}
	if _, ok := tiles[packTile(3000, 3000)]; ok {
		t.Error("hull must not cover the boat's own tile (3000, 3000)")
	}
	if len(tiles) != 4 {
		t.Errorf("hull covers %d tiles, want 4", len(tiles))
	}
}

func TestCanOccupyOnlyChecksNewTiles(t *testing.T) {
	boat := testBoat()
	// The hull's current tiles are land, but the boat may
	// still sail off them: only newly covered tiles are
	// checked. The destination's new tile (2999, 2995) is
	// water.
	isSailable := func(x, y, level int) bool {
		return x == 2999 && y == 2995
	}
	if !CanOccupy(boat, boat.FineX, boat.FineY-64, 0, isSailable) {
		t.Error("moving onto water over land must be allowed (only new tiles checked)")
	}
	// The same move with the new tile land is refused.
	land := func(x, y, level int) bool { return false }
	if CanOccupy(boat, boat.FineX, boat.FineY-64, 0, land) {
		t.Error("moving onto land must be refused")
	}
	// Standing still is always occupiable: no new tiles.
	if !CanOccupy(boat, boat.FineX, boat.FineY, boat.Angle, land) {
		t.Error("standing still must always be occupiable")
	}
}

func TestPackTileRoundTrip(t *testing.T) {
	// Non-negative y round-trips (all tiles a hull can sample
	// in the real map, and all deck tiles). A negative y does
	// not: the TS packing x*0x8000+y is ambiguous there, and
	// Go keeps the same packing.
	for _, tile := range [][2]int{{0, 0}, {3000, 3000}, {9600, 9600}, {-1, 5}, {0, 32767}} {
		packed := packTile(tile[0], tile[1])
		if gotX, gotY := unpackTileX(packed), unpackTileY(packed); gotX != tile[0] || gotY != tile[1] {
			t.Errorf("pack/unpack (%d, %d) = (%d, %d)", tile[0], tile[1], gotX, gotY)
		}
	}
}
