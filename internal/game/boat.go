package game

import "math"

// A boat: a world entity whose deck is its own scene (the deck
// frame), drawn at a moving position in the main world. Players
// aboard stand in deck coordinates. Ported from
// game/content/sailing/Boat.ts.

const (
	// WoodenHullSpeed is WOODEN_HULL_SPEED (Boat.ts): a wooden
	// hull's base speed (the raft's), 1.5 tiles a tick: the
	// default full sail speed.
	WoodenHullSpeed = 192
	// BaseAcceleration is BASE_ACCELERATION (Boat.ts): speed
	// gained or lost a tick without camphor-or-better sails:
	// half a tile (rsprox).
	BaseAcceleration = 64
	// FineUnitsPerTile is FINE_UNITS_PER_TILE: world entity
	// positions use 1/128-tile precision.
	FineUnitsPerTile = 128
)

// BoatMoveMode is the sails' setting (Boat.ts: BoatMoveMode).
type BoatMoveMode int

const (
	BoatMoveModeStopped BoatMoveMode = 0
	BoatMoveModeHalf    BoatMoveMode = 1
	BoatMoveModeFull    BoatMoveMode = 2
	BoatMoveModeReverse BoatMoveMode = 3
)

// BoatHull is the hull rectangle in fine units, centred on the
// boat's position (worldentity type bounds).
type BoatHull struct {
	OffsetX int
	OffsetY int
	Width   int
	Length  int
}

// BoatInit is the Boat constructor argument (Boat.ts: BoatInit).
// BaseSpeed, SpeedCap, and Acceleration are pointers to keep the
// TS optional: absent resolves through the ?? defaults in NewBoat.
type BoatInit struct {
	EntityIndex   int
	ConfigID      int
	OwnerPlayerID int
	// Static marks a fixed quest/world ship rather than a
	// player's moving boat.
	Static bool
	// DeckRegionX/Y are the chunk coords of the deck scene's
	// centre chunk, where the boat template is copied to.
	DeckRegionX int
	DeckRegionY int
	// SizeX/SizeZ is the deck footprint in tiles (the template
	// zone), sent as the world entity size.
	SizeX int
	SizeZ int
	Hull  BoatHull
	// DeckCentreX/Y is the fine offset from the template zone's
	// south-west corner to the point that sits at the boat's
	// position: size * 64 plus the worldentity type's base
	// offset (the client uses the same formula).
	DeckCentreX int
	DeckCentreY int
	// BaseSpeed is the full sail speed in fine units a tick.
	BaseSpeed *int
	// SpeedCap is the hull's speed cap: what rapids can push
	// the sails to (bursts go a step past it).
	SpeedCap *int
	// Acceleration is the speed gained or lost a tick, in fine
	// units.
	Acceleration *int
	// FineX/FineY is the world position of the boat's centre
	// in fine units.
	FineX int
	FineY int
	Level int
	Angle int
}

// Boat is a player-owned boat (Boat.ts: Boat). Fields TS mutates
// are exported; the constructor-resolved speeds are read-only
// after NewBoat (TS readonly) and are only ever set there.
type Boat struct {
	EntityIndex   int
	ConfigID      int
	OwnerPlayerID int
	// Static marks a fixed quest/world ship rather than a
	// player's moving boat.
	Static bool
	// OwnerOnly shows the boat only to its owner (a boat in
	// the shipyard), with anyone aboard it.
	OwnerOnly   bool
	DeckRegionX int
	DeckRegionY int
	SizeX       int
	SizeZ       int
	Hull        BoatHull
	DeckCentreX int
	DeckCentreY int
	// BaseSpeed is the full sail speed in fine units a tick
	// (the hull's base speed); half sail is half.
	BaseSpeed int
	// Acceleration is the speed gained or lost a tick in fine
	// units: half a tile, more with camphor sails or better.
	Acceleration int

	FineX int
	FineY int
	Level int
	// Angle is the facing, 0-2047 (0 = south, 512 = west,
	// 1024 = north, 1536 = east).
	Angle int
	// Heading is the heading the helm is steering toward.
	Heading  int
	MoveMode BoatMoveMode
	// Velocity is the speed along the bow in fine units a
	// tick, negative backing up. It eases toward the sails'
	// speed (boat_movement.go), so the boat gathers way when
	// the sails go up and drifts when stopped.
	Velocity int
	// HelmPlayerID is the player steering from the helm;
	// their world clicks set the heading. Nil when no one
	// holds the helm.
	HelmPlayerID *int
	// HelmCrew is a crewmate holding the helm for the owner,
	// whose world clicks then set the heading.
	HelmCrew *Mobile
	// SpeedMultiplier multiplies the distance sailed each
	// tick (BoatMovement.ts: Math.max(1, speedMultiplier)).
	SpeedMultiplier float64
	// SpeedCap is the hull's speed cap in fine units a tick:
	// rapids push the sails up to it (see boat_movement.go).
	SpeedCap int
	// BoostSpeed is what a burst of speed adds (BURST_STEP)
	// while it is up (a trim, a released wind mote). Nil
	// when no burst is up.
	BoostSpeed *int
	// SpeedBonuses is the speed added under the hull's cap
	// by source (a Barracuda Trial's rapids), or taken off
	// (a lightning strike), in fine units a tick.
	SpeedBonuses map[string]int
	// BurstBonuses is the speed added past the cap by source
	// (a portal's burst), at most a step over it in all.
	BurstBonuses map[string]int
	// SpeedLimits is the most the boat may go by source (a
	// fetid pool), in fine units a tick.
	SpeedLimits map[string]int
	// TeleportCount is bumped by BoatManager.Teleport:
	// viewers jump the boat there rather than sail it there.
	TeleportCount int
	// PrevFineX/PrevFineY is where the boat was before this
	// tick's move, and LastStep is what the move did
	// (BoatManager.Tick).
	PrevFineX int
	PrevFineY int
	LastStep  *BoatStep
}

// NewBoat builds a Boat from init (Boat.ts constructor): the
// speeds resolve through the ?? defaults, the speed cap is at
// least the base speed, and the heading starts at the angle.
func NewBoat(init BoatInit) *Boat {
	baseSpeed := WoodenHullSpeed
	if init.BaseSpeed != nil {
		baseSpeed = *init.BaseSpeed
	}
	acceleration := BaseAcceleration
	if init.Acceleration != nil {
		acceleration = *init.Acceleration
	}
	speedCap := baseSpeed
	if init.SpeedCap != nil {
		speedCap = *init.SpeedCap
	}
	if speedCap < baseSpeed {
		speedCap = baseSpeed
	}
	return &Boat{
		EntityIndex:     init.EntityIndex,
		ConfigID:        init.ConfigID,
		OwnerPlayerID:   init.OwnerPlayerID,
		Static:          init.Static,
		DeckRegionX:     init.DeckRegionX,
		DeckRegionY:     init.DeckRegionY,
		SizeX:           init.SizeX,
		SizeZ:           init.SizeZ,
		Hull:            init.Hull,
		DeckCentreX:     init.DeckCentreX,
		DeckCentreY:     init.DeckCentreY,
		BaseSpeed:       baseSpeed,
		Acceleration:    acceleration,
		SpeedCap:        speedCap,
		FineX:           init.FineX,
		FineY:           init.FineY,
		PrevFineX:       init.FineX,
		PrevFineY:       init.FineY,
		Level:           init.Level,
		Angle:           init.Angle,
		Heading:         init.Angle,
		SpeedMultiplier: 1,
		SpeedBonuses:    make(map[string]int),
		BurstBonuses:    make(map[string]int),
		SpeedLimits:     make(map[string]int),
	}
}

// TileX is the tile the boat's fine X position lies in.
func (b *Boat) TileX() int { return floorDiv(b.FineX, FineUnitsPerTile) }

// TileY is the tile the boat's fine Y position lies in.
func (b *Boat) TileY() int { return floorDiv(b.FineY, FineUnitsPerTile) }

// SceneBaseX is the south-west tile of the deck scene (13x13
// chunks centred on the deck region).
func (b *Boat) SceneBaseX() int { return (b.DeckRegionX - 6) * 8 }

// SceneBaseY is the south-west tile of the deck scene.
func (b *Boat) SceneBaseY() int { return (b.DeckRegionY - 6) * 8 }

// DeckBaseX is the south-west tile of the boat template zone
// inside the deck scene.
func (b *Boat) DeckBaseX() int { return b.DeckRegionX * 8 }

// DeckBaseY is the south-west tile of the boat template zone
// inside the deck scene.
func (b *Boat) DeckBaseY() int { return b.DeckRegionY * 8 }

// WorldTile is the boat's own main-world tile: where its world
// entity is, the tile its fine position lies in.
func (b *Boat) WorldTile() Location {
	return Location{X: b.TileX(), Y: b.TileY(), Z: b.Level}
}

// DeckTileToWorld is the main-world tile under the centre of a
// deck tile, given the boat's position and angle.
func (b *Boat) DeckTileToWorld(x, y int) (tileX, tileY int) {
	localX := float64((x-b.DeckBaseX())*FineUnitsPerTile + 64 - b.DeckCentreX)
	localY := float64((y-b.DeckBaseY())*FineUnitsPerTile + 64 - b.DeckCentreY)
	radians := float64(b.Angle) * math.Pi / 1024
	cos, sin := math.Cos(radians), math.Sin(radians)
	wx := float64(b.FineX) + localX*cos + localY*sin
	wy := float64(b.FineY) + localY*cos - localX*sin
	return int(math.Floor(wx / FineUnitsPerTile)), int(math.Floor(wy / FineUnitsPerTile))
}

// HelmHeadingToward is the helm heading from the boat's exact
// position to the centre of a main-world tile, snapped to the 16
// helm directions. It reports false when the tile is under the
// boat's centre (TS undefined).
func (b *Boat) HelmHeadingToward(tileX, tileY int) (int, bool) {
	dx := tileX*FineUnitsPerTile + FineUnitsPerTile/2 - b.FineX
	dy := tileY*FineUnitsPerTile + FineUnitsPerTile/2 - b.FineY
	if absInt(dx) < FineUnitsPerTile/2 && absInt(dy) < FineUnitsPerTile/2 {
		return 0, false
	}
	angle := AngleFromCoordDelta(dx, dy)
	return NormalizeAngle(int(math.Round(float64(angle)/PackedHeadingScale)) * PackedHeadingScale), true
}

// ContainsDeckTile reports whether x, y is inside the boat's
// 104x104 deck scene.
func (b *Boat) ContainsDeckTile(x, y int) bool {
	lx := x - b.SceneBaseX()
	ly := y - b.SceneBaseY()
	return lx >= 0 && lx < 104 && ly >= 0 && ly < 104
}

// floorDiv divides rounding toward negative infinity, matching
// TS Math.floor(a / b) for the tile conversions above (Go's /
// truncates toward zero).
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}
