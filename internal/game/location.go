package game

import "fmt"

// Location is a world tile: x/y tile coordinates plus z height level.
// Ported from game/model/Location.ts (constructor, Location.ts:26-30).
// It is a value type in Go; TS Location is mutable, so callers that need
// TS-style mutation assign a new Location instead of mutating in place.
type Location struct {
	X, Y, Z int
}

// RegionX returns the region x used by map-rebuild math.
// Location.ts:110-112: getRegionX() = (x >> 3) - 6.
//
// NOTE: this is NOT x>>6. The x>>6 form is World's map-square key
// (World.ts getRegionKey: `${z}:${x >> 6}:${y >> 6}`), a different quantity.
// Do not "fix" this formula to x>>6.
func (l Location) RegionX() int { return (l.X >> 3) - 6 }

// RegionY returns the region y used by map-rebuild math.
// Location.ts:114-116: getRegionY() = (y >> 3) - 6.
func (l Location) RegionY() int { return (l.Y >> 3) - 6 }

// LocalX returns the x offset within the 8x8 region chunk containing
// position. Location.ts:100-102: getLocalX = x - 8*position.getRegionX().
func (l Location) LocalX(position Location) int { return l.X - 8*position.RegionX() }

// LocalY returns the y offset within the 8x8 region chunk containing
// position. Location.ts:104-106: getLocalY = y - 8*position.getRegionY().
func (l Location) LocalY(position Location) int { return l.Y - 8*position.RegionY() }

// IsWithinDistance reports whether other is within distance tiles on the
// same height level (Chebyshev box test).
// Location.ts:147-153.
func (l Location) IsWithinDistance(other Location, distance int) bool {
	if l.Z != other.Z {
		return false
	}
	dx := l.X - other.X
	if dx < 0 {
		dx = -dx
	}
	dy := l.Y - other.Y
	if dy < 0 {
		dy = -dy
	}
	return dx <= distance && dy <= distance
}

// Distance returns Chebyshev tile distance, or -1 when the height levels
// differ. TS returns Number.MAX_SAFE_INTEGER for differing levels
// (Location.ts:163-172); -1 is the Go sentinel for "unreachable by this
// measure" and must be checked by callers.
func (l Location) Distance(other Location) int {
	if l.Z != other.Z {
		return -1
	}
	dx := l.X - other.X
	if dx < 0 {
		dx = -dx
	}
	dy := l.Y - other.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// IsViewableFrom reports whether l is within the standard 15-tile view
// distance of other on the same level.
// Location.ts:224-226 (isViewableFrom delegates to isViewableFromWithin
// with distance 15).
func (l Location) IsViewableFrom(other Location) bool {
	return l.IsViewableFromWithin(other, 15)
}

// IsViewableFromWithin is the hot-path view check from Location.ts:234-245.
func (l Location) IsViewableFromWithin(other Location, distance int) bool {
	if l.Z != other.Z {
		return false
	}
	dx := other.X - l.X
	if dx > distance || dx < -distance {
		return false
	}
	dy := other.Y - l.Y
	return dy <= distance && dy >= -distance
}

// Add returns l translated by (x, y) on the same level.
// Location.ts:118-122 (add/addX/addY; TS mutates, Go returns a copy).
func (l Location) Add(x, y int) Location { return Location{X: l.X + x, Y: l.Y + y, Z: l.Z} }

// Transform returns l translated by (dx, dy), preserving the height
// level. Location.ts:134-135: transform(x, y) = clone().addX(x).addY(y),
// and clone (Location.ts:138-144) copies z. Movement/collision use
// Transform (MovementQueue.validatedStep/canWalkTo, Mobile tile
// iteration); it is the z-preserving translation, unlike the
// z-dropping move/translate below.
func (l Location) Transform(dx, dy int) Location {
	return Location{X: l.X + dx, Y: l.Y + dy, Z: l.Z}
}

// Equals reports tile equality. Location.ts:214-220.
func (l Location) Equals(other Location) bool {
	return l.X == other.X && l.Y == other.Y && l.Z == other.Z
}

// String renders "[x, y, z]". Location.ts:208-210.
func (l Location) String() string {
	return fmt.Sprintf("[%d, %d, %d]", l.X, l.Y, l.Z)
}

// Deliberately NOT ported from Location.ts: move/getMove/translate/
// getTranslate drop the z coordinate in TS — move (Location.ts:178-183)
// computes z then discards it by constructing new Location(x, y),
// translate (Location.ts:261-263) ignores its z parameter entirely,
// and getTranslate (Location.ts:257-259) passes 0. Checked against
// upstream at commit bfd81e3, this is an upstream bug, not intended
// behavior (see PORT_STATUS.md, Game notes): no caller in
// MovementQueue, RegionManager, PathFinder, or Mobile uses
// translate/getTranslate or relies on a level reset; movement and
// collision instead use the z-preserving transform() and reject
// destinations whose z differs. DECISION (recorded): the Go port
// preserves z and does not reproduce the drop. move/translate remain
// unported because no Go caller needs them — movement uses
// Transform above. Caveat: a repo-wide caller search was not possible
// (GitHub code search needs sign-in), so an unchecked caller under
// server/plugins/ cannot be ruled out. rotate() is unused by the
// foundation and is also omitted.
