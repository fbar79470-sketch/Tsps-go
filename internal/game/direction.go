package game

// Direction is a compass step. IDs, deltas, and opposites are quoted from
// game/model/Direction.ts:
//
//	NORTH(1, 0, 1, 6)  NORTH_EAST(2, 1, 1, 5)  EAST(4, 1, 0, 3)
//	SOUTH_EAST(7, 1, -1, 0)  SOUTH(6, 0, -1, 1)  SOUTH_WEST(5, -1, -1, 2)
//	WEST(3, -1, 0, 4)  NORTH_WEST(0, -1, 1, 7)  NONE(-1, 0, 0, -1)
//
// The id order is the TS wire order (Direction.valueOf); do not renumber.
type Direction int

const (
	DirNone      Direction = -1
	DirNorthWest Direction = 0
	DirNorth     Direction = 1
	DirNorthEast Direction = 2
	DirWest      Direction = 3
	DirEast      Direction = 4
	DirSouthWest Direction = 5
	DirSouth     Direction = 6
	DirSouthEast Direction = 7
)

type directionInfo struct {
	dx, dy   int
	opposite Direction
	name     string
	diagonal bool
}

var directionTable = [8]directionInfo{
	{dx: -1, dy: 1, opposite: DirSouthEast, name: "NORTH_WEST", diagonal: true},
	{dx: 0, dy: 1, opposite: DirSouth, name: "NORTH"},
	{dx: 1, dy: 1, opposite: DirSouthWest, name: "NORTH_EAST", diagonal: true},
	{dx: -1, dy: 0, opposite: DirEast, name: "WEST"},
	{dx: 1, dy: 0, opposite: DirWest, name: "EAST"},
	{dx: -1, dy: -1, opposite: DirNorthEast, name: "SOUTH_WEST", diagonal: true},
	{dx: 0, dy: -1, opposite: DirNorth, name: "SOUTH"},
	{dx: 1, dy: -1, opposite: DirNorthWest, name: "SOUTH_EAST", diagonal: true},
}

// Dx returns the x step. Direction.ts constructor (x field).
func (d Direction) Dx() int {
	if d < 0 || int(d) >= len(directionTable) {
		return 0
	}
	return directionTable[d].dx
}

// Dy returns the y step. Direction.ts constructor (y field).
func (d Direction) Dy() int {
	if d < 0 || int(d) >= len(directionTable) {
		return 0
	}
	return directionTable[d].dy
}

// Opposite returns the facing direction. Direction.ts constructor
// (opposite field). DirNone's opposite is itself.
func (d Direction) Opposite() Direction {
	if d < 0 || int(d) >= len(directionTable) {
		return DirNone
	}
	return directionTable[d].opposite
}

// IsDiagonal reports whether the direction is a diagonal step.
// Direction.ts constructor: diagonal iff the name contains '_'.
func (d Direction) IsDiagonal() bool {
	if d < 0 || int(d) >= len(directionTable) {
		return false
	}
	return directionTable[d].diagonal
}

// String returns the TS constant name, or "NONE".
func (d Direction) String() string {
	if d < 0 || int(d) >= len(directionTable) {
		return "NONE"
	}
	return directionTable[d].name
}

// DirectionFromID maps a wire id to a Direction, following
// Direction.valueOf. Unknown ids yield DirNone.
func DirectionFromID(id int) Direction {
	if id < 0 || id >= len(directionTable) {
		return DirNone
	}
	return Direction(id)
}

// DirectionFromDeltas maps a step delta to a Direction, following
// Direction.fromDeltas (Direction.ts:75-96). The mapping is sign-only:
// any nonzero magnitude in a direction yields the same Direction as a
// unit step (there is no clamping step upstream — (5, -3) is
// SOUTH_EAST exactly as (1, -1) is), and (0, 0) yields DirNone.
func DirectionFromDeltas(dx, dy int) Direction {
	if dx < 0 {
		if dy < 0 {
			return DirSouthWest
		} else if dy > 0 {
			return DirNorthWest
		}
		return DirWest
	} else if dx > 0 {
		if dy < 0 {
			return DirSouthEast
		} else if dy > 0 {
			return DirNorthEast
		}
		return DirEast
	}
	if dy < 0 {
		return DirSouth
	} else if dy > 0 {
		return DirNorth
	}
	return DirNone
}
