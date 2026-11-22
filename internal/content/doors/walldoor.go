// Package doors is the Go port of the door mechanism in
// server/plugins/objects/Doors.plugin.js (upstream HEAD bfd81e3).
// Worker B owns the toggle engine in this package (doors.go,
// passthrough.go, walldoor.go); Worker A owns the catalog
// (catalog.go) and the transcribed exception tables (tables.go).
package doors

import "tsps-go/internal/game"

// wallDoorOffsets is the per-face step across a straight wall door
// (WallDoor.Doors.js:7): face 0 west, 1 north, 2 east, 3 south.
// The same table is Doors.plugin.js COORD_OFFSETS (:906-909).
var wallDoorOffsets = [4][2]int{{-1, 0}, {0, 1}, {1, 0}, {0, -1}}

// CrossingFor ports crossingFor (WallDoor.Doors.js — the whole file,
// 14 lines, transcribed with the citation below):
//
//	// A straight wall door separates its own tile from the neighbour across its face.
//	function crossingFor(object, from) {
//	  if (object?.getType?.() !== 0) return null;
//	  const at = object.getLocation();
//	  const [dx, dy] = [[-1, 0], [0, 1], [1, 0], [0, -1]][object.getFace() & 3];
//	  const tile = { x: at.x, y: at.y, z: at.z };
//	  const neighbour = { x: at.x + dx, y: at.y + dy, z: at.z };
//	  const outside = (from.x - at.x) * dx + (from.y - at.y) * dy > 0;
//	  return { approach: outside ? neighbour : tile, destination: outside ? tile : neighbour };
//	}
//
// A type-0 (straight wall) door at tile `at` with the given face
// separates its own tile from the neighbour across its face. From a
// player standing on `from`, the approach is the tile on the player's
// side and the destination the tile on the other side. A non-wall
// object (type != 0) has no crossing: ok is false.
func CrossingFor(objectType, face int, at, from game.Location) (approach, destination game.Location, ok bool) {
	if objectType != 0 {
		return game.Location{}, game.Location{}, false
	}
	d := wallDoorOffsets[face&3]
	neighbour := game.Location{X: at.X + d[0], Y: at.Y + d[1], Z: at.Z}
	outside := (from.X-at.X)*d[0]+(from.Y-at.Y)*d[1] > 0
	if outside {
		return neighbour, at, true
	}
	return at, neighbour, true
}
