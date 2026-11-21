package game

// Boat specs, ported from game/content/sailing/BoatSpec.ts.
// Everything needed to build a boat of one type. Offsets are
// relative to the template zone.

// BoatDeckLoc is a loc placed on the deck, relative to the boat
// template zone's south-west tile (BoatSpec.ts: BoatDeckLoc).
type BoatDeckLoc struct {
	ID       int
	X, Y     int
	Level    int
	Shape    int
	Rotation int
	// Blocks marks a loc that blocks its tile on the deck
	// (facilities); other locs are scenery or sound.
	Blocks bool
	// OpFlags is which ops the client shows (bit 0 is op1), as
	// live's loc_add_change_v2; nil shows all. The pointer keeps
	// the TS optional: an absent opFlags encodes the 8-byte loc
	// form, a present one the 9-byte form (ClientProtocol.ts
	// encodeLocAddChange; WorldEntitySync.ts:183).
	OpFlags *int
	// Removed marks a change that takes the loc of this shape off
	// its tile (sent as LOC_DEL).
	Removed bool
}

// DeckTile is one tile relative to the template zone's south-west
// tile: a walkable-deck entry or the boarding tile.
type DeckTile struct {
	X, Y int
}

// BoatSpecStats is the stats block of a BoatSpec: full sail speed
// in fine units a tick (the hull's base speed; 192 when absent),
// the hull's speed cap (the trimmed sails' speed), how long a trim
// lasts in ticks (the sails'), and the speed gained or lost a
// tick (64 and the sails' extra; 64 when absent). Pointer fields
// keep the TS optional: a present 0 is a real 0, distinct from
// absent (Boat.ts resolves absent through ??).
type BoatSpecStats struct {
	BaseSpeed          *int
	SpeedCap           *int
	SpeedBoostDuration *int
	Acceleration       *int
}

// BoatSpec is everything needed to build a boat of one type
// (BoatSpec.ts: BoatSpec).
type BoatSpec struct {
	Type string
	// ConfigID is the worldentity type the client draws.
	ConfigID int
	// TemplateChunkX/Y is the source chunk of the boat template
	// in the cache: its south-west zone when it spans several.
	TemplateChunkX int
	TemplateChunkY int
	// SizeX/SizeZ is the deck footprint in tiles (8 per template
	// zone), sent as the world entity size.
	SizeX int
	SizeZ int
	Hull  BoatHull
	// DeckCentreX/Y is the fine offset of the boat's centre from
	// the template zone's south-west corner: size * 64 plus the
	// worldentity type's base offset. The client uses the same
	// formula.
	DeckCentreX int
	DeckCentreY int
	// DeckLevel is the template plane that holds the deck; the
	// deck scene's level 0 reads from it.
	DeckLevel int
	// WalkableDeck is the walkable deck tiles; every other tile
	// of the deck scene is blocked.
	WalkableDeck []DeckTile
	// BoardingTile is where a player lands when boarding.
	BoardingTile DeckTile
	Locs         []BoatDeckLoc
	// SyncLocs are the loc adds a fixed ship must send on top of
	// its cache template (dynamic overrides). Nil means absent:
	// a static ship with none sends no deck locs.
	SyncLocs []BoatDeckLoc
	Stats    *BoatSpecStats
}

// BoatPlacement is a boat's position in the main world: fine units
// (1/128 tile) and an angle out of 2048 (BoatSpec.ts:
// BoatPlacement).
type BoatPlacement struct {
	FineX int
	FineY int
	Level int
	Angle int
}

// IntPtr returns a pointer to v, for building optional spec/stat
// fields in fixtures and content registrations.
func IntPtr(v int) *int { return &v }
