package game

import "testing"

// testBoat returns a sloop-shaped boat at a fine position, facing
// south (angle 0), with deck region (1200, 1200): the first
// allocated deck scene.
func testBoat() *Boat {
	return NewBoat(BoatInit{
		EntityIndex: 3000,
		ConfigID:    1,
		DeckRegionX: 1200,
		DeckRegionY: 1200,
		SizeX:       8,
		SizeZ:       16,
		Hull:        BoatHull{OffsetX: -64, OffsetY: -256, Width: 128, Length: 512},
		DeckCentreX: 512,
		DeckCentreY: 1024,
		FineX:       3000 * FineUnitsPerTile,
		FineY:       3000 * FineUnitsPerTile,
		Angle:       0,
	})
}

func TestNewBoatDefaults(t *testing.T) {
	boat := testBoat()
	if boat.BaseSpeed != WoodenHullSpeed {
		t.Errorf("BaseSpeed = %d, want wooden default %d", boat.BaseSpeed, WoodenHullSpeed)
	}
	if boat.SpeedCap != WoodenHullSpeed {
		t.Errorf("SpeedCap = %d, want at least base speed %d", boat.SpeedCap, WoodenHullSpeed)
	}
	if boat.Acceleration != BaseAcceleration {
		t.Errorf("Acceleration = %d, want %d", boat.Acceleration, BaseAcceleration)
	}
	if boat.Heading != boat.Angle {
		t.Errorf("Heading = %d, want angle %d", boat.Heading, boat.Angle)
	}
	if boat.PrevFineX != boat.FineX || boat.PrevFineY != boat.FineY {
		t.Errorf("prev fine = (%d, %d), want (%d, %d)", boat.PrevFineX, boat.PrevFineY, boat.FineX, boat.FineY)
	}
	if boat.SpeedBonuses == nil || boat.BurstBonuses == nil || boat.SpeedLimits == nil {
		t.Error("speed maps must be non-nil")
	}
}

func TestNewBoatExplicitSpeeds(t *testing.T) {
	init := BoatInit{BaseSpeed: IntPtr(256), SpeedCap: IntPtr(128), Acceleration: IntPtr(96)}
	boat := NewBoat(init)
	if boat.BaseSpeed != 256 {
		t.Errorf("BaseSpeed = %d, want 256", boat.BaseSpeed)
	}
	// The cap is at least the base speed.
	if boat.SpeedCap != 256 {
		t.Errorf("SpeedCap = %d, want 256 (max of cap and base)", boat.SpeedCap)
	}
	if boat.Acceleration != 96 {
		t.Errorf("Acceleration = %d, want 96", boat.Acceleration)
	}
}

func TestBoatTilesAndBases(t *testing.T) {
	boat := testBoat()
	if boat.TileX() != 3000 || boat.TileY() != 3000 {
		t.Errorf("tile = (%d, %d), want (3000, 3000)", boat.TileX(), boat.TileY())
	}
	boat.FineX = 3000*FineUnitsPerTile + 127
	if boat.TileX() != 3000 {
		t.Errorf("TileX with +127 fine = %d, want 3000", boat.TileX())
	}
	boat.FineX = -1
	if boat.TileX() != -1 {
		t.Errorf("TileX with fine -1 = %d, want -1 (floor)", boat.TileX())
	}
	if boat.SceneBaseX() != (1200-6)*8 || boat.SceneBaseY() != (1200-6)*8 {
		t.Errorf("scene base = (%d, %d), want (9552, 9552)", boat.SceneBaseX(), boat.SceneBaseY())
	}
	if boat.DeckBaseX() != 9600 || boat.DeckBaseY() != 9600 {
		t.Errorf("deck base = (%d, %d), want (9600, 9600)", boat.DeckBaseX(), boat.DeckBaseY())
	}
}

func TestBoatDeckTileToWorld(t *testing.T) {
	boat := testBoat()
	// The deck centre tile maps onto the boat's own tile: the
	// centre offset (512, 1024) is 4 tiles east, 8 north of the
	// zone corner, so deck tile (9604, 9608) centres on the
	// boat.
	x, y := boat.DeckTileToWorld(9604, 9608)
	if x != 3000 || y != 3000 {
		t.Errorf("centre deck tile -> (%d, %d), want (3000, 3000)", x, y)
	}
	// One deck tile east is one world tile east at angle 0.
	x, y = boat.DeckTileToWorld(9605, 9608)
	if x != 3001 || y != 3000 {
		t.Errorf("east deck tile -> (%d, %d), want (3001, 3000)", x, y)
	}
	// Turned north (1024), the deck rotates with the boat: the
	// centre tile lands a tile southwest of the boat (the
	// 64-unit tile centre offset flips across the boundary),
	// and one deck tile west lands one world tile east.
	boat.Angle = 1024
	x, y = boat.DeckTileToWorld(9604, 9608)
	if x != 2999 || y != 2999 {
		t.Errorf("turned centre deck tile -> (%d, %d), want (2999, 2999)", x, y)
	}
	x, y = boat.DeckTileToWorld(9603, 9608)
	if x != 3000 || y != 2999 {
		t.Errorf("turned west deck tile -> (%d, %d), want (3000, 2999)", x, y)
	}
}

func TestBoatHelmHeadingToward(t *testing.T) {
	boat := testBoat()
	// Due south of the boat: heading 0.
	heading, ok := boat.HelmHeadingToward(3000, 2990)
	if !ok || heading != 0 {
		t.Errorf("south heading = (%d, %v), want (0, true)", heading, ok)
	}
	// Due north: heading 1024.
	heading, ok = boat.HelmHeadingToward(3000, 3010)
	if !ok || heading != 1024 {
		t.Errorf("north heading = (%d, %v), want (1024, true)", heading, ok)
	}
	// The tile under the centre: no heading. The boat sits at
	// its tile centre for this check: at a tile corner the
	// centre is exactly half a tile away, the boundary of
	// the under-centre test.
	boat.FineX = 3000*FineUnitsPerTile + FineUnitsPerTile/2
	boat.FineY = 3000*FineUnitsPerTile + FineUnitsPerTile/2
	if _, ok := boat.HelmHeadingToward(3000, 3000); ok {
		t.Error("centre tile must report no heading")
	}
	// Headings snap to multiples of 128.
	heading, ok = boat.HelmHeadingToward(3010, 2990)
	if !ok || heading%128 != 0 {
		t.Errorf("diagonal heading = (%d, %v), want a multiple of 128", heading, ok)
	}
}

func TestBoatContainsDeckTile(t *testing.T) {
	boat := testBoat()
	if !boat.ContainsDeckTile(9552, 9552) || !boat.ContainsDeckTile(9655, 9655) {
		t.Error("scene corners must be deck tiles")
	}
	if boat.ContainsDeckTile(9551, 9600) || boat.ContainsDeckTile(9656, 9600) {
		t.Error("tiles outside the scene must not be deck tiles")
	}
	if boat.ContainsDeckTile(3000, 3000) {
		t.Error("a main-world tile must not be a deck tile")
	}
}
