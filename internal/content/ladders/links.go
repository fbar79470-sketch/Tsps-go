package ladders

import (
	"fmt"

	"tsps-go/internal/game"
)

// The 7 declared climb-link pairs, TRANSCRIBED into a Go table
// with per-pair JSON line citations (the Phase 33 transcription
// precedent — transcribing upstream table values into Go source
// with citations is normal porting methodology; the file itself
// is NOT copied as an asset).
//
// Source: server/plugins/objects/data/climb-links.json (upstream
// HEAD bfd81e3). `landing` is where climbing up lands,
// `downLanding` where climbing down does (ClimbLinks.js
// loadDeclaredLinks). Each pair's up object offers Climb-up; down
// offers Climb-down — except the Grand Tree cave ladder's down
// end (id 2446, a Trapdoor offering only "Open"), which the
// resolver's climbs() gate therefore refuses to climb down from,
// exactly as upstream's destination() does.

type declaredLink struct {
	// direction is the direction climbing the key endpoint goes:
	// Up for the pair's up endpoint, Down for its down endpoint.
	direction int
	otherID   int
	otherX    int
	otherY    int
	otherZ    int
	// landing overrides the computed landing tile for a climb in
	// direction from the key endpoint; nil means compute it.
	landing *game.Location
}

type declaredEndpoint struct {
	id int
	x  int
	y  int
	z  int
}

func loc(x, y, z int) *game.Location {
	return &game.Location{X: x, Y: y, Z: z}
}

// declaredPairs is the transcribed table. The comment on each
// entry cites the JSON lines (name / up / down / landing /
// downLanding) it was transcribed from.
var declaredPairs = []struct {
	// name is the JSON pair's "name" (a comment, not a lookup key).
	name    string
	up      declaredEndpoint
	down    declaredEndpoint
	landing *game.Location
	// downLanding is where climbing down lands.
	downLanding *game.Location
}{
	{
		// climb-links.json:5-8 — Lady Lumbridge's hold ladder,
		// Port Sarim (down from 3049,3208,1 lands on 3048,9640,1
		// in the hold, 6432 tiles north).
		name:        "Lady Lumbridge's hold ladder",
		up:          declaredEndpoint{id: 2592, x: 3049, y: 9640, z: 1},
		down:        declaredEndpoint{id: 2590, x: 3049, y: 3208, z: 1},
		downLanding: loc(3048, 9640, 1),
	},
	{
		// climb-links.json:11-15 — Port Sarim Rat Pits manhole
		// (landings match the RuneLite shortest-path transports
		// 3018,3231,0 -> 2962,9650,0 and 2962,9650,0 ->
		// 3018,3233,0).
		name:        "Port Sarim Rat Pits manhole",
		up:          declaredEndpoint{id: 10309, x: 2962, y: 9651, z: 0},
		down:        declaredEndpoint{id: 10321, x: 3018, y: 3232, z: 0},
		landing:     loc(3018, 3233, 0),
		downLanding: loc(2962, 9650, 0),
	},
	{
		// climb-links.json:18-20 — Wizards' Tower basement ladder.
		name: "Wizards' Tower basement ladder",
		up:   declaredEndpoint{id: 2148, x: 3103, y: 9576, z: 0},
		down: declaredEndpoint{id: 2147, x: 3104, y: 3162, z: 0},
	},
	{
		// climb-links.json:24-27 — Forthos Ruin spiral staircase
		// (landings match the shortest-path transports).
		name:        "Forthos Ruin spiral staircase",
		up:          declaredEndpoint{id: 34864, x: 1798, y: 9967, z: 0},
		down:        declaredEndpoint{id: 34865, x: 1669, y: 3567, z: 0},
		landing:     loc(1670, 3569, 0),
		downLanding: loc(1800, 9968, 0),
	},
	{
		// climb-links.json:30-33 — Grand Tree base staircase
		// (Anita's floor).
		name:    "Grand Tree base staircase",
		up:      declaredEndpoint{id: 16675, x: 2389, y: 3512, z: 0},
		down:    declaredEndpoint{id: 16677, x: 2389, y: 3513, z: 1},
		landing: loc(2388, 3513, 1),
	},
	{
		// climb-links.json:37-39 — Grand Tree cave ladder (base
		// trapdoor).
		name:    "Grand Tree cave ladder",
		up:      declaredEndpoint{id: 17385, x: 2463, y: 9897, z: 0},
		down:    declaredEndpoint{id: 2446, x: 2463, y: 3497, z: 0},
		landing: loc(2463, 3498, 0),
	},
	{
		// climb-links.json:42-46 — Ardougne Castle main staircase
		// (landings match the RuneLite shortest-path transports
		// 2571,3298,0 <-> 2571,3294,1).
		name:        "Ardougne Castle main staircase",
		up:          declaredEndpoint{id: 15645, x: 2571, y: 3295, z: 0},
		down:        declaredEndpoint{id: 15648, x: 2571, y: 3295, z: 1},
		landing:     loc(2571, 3294, 1),
		downLanding: loc(2571, 3298, 0),
	},
}

// declaredLinks maps "id:x,y,z" to the link for that endpoint,
// built once from declaredPairs. ClimbLinks.js: loadDeclaredLinks()
// (DECLARED_LINKS).
var declaredLinks = func() map[string]declaredLink {
	links := make(map[string]declaredLink, len(declaredPairs)*2)
	for _, pair := range declaredPairs {
		links[fmt.Sprintf("%d:%d,%d,%d", pair.up.id, pair.up.x, pair.up.y, pair.up.z)] = declaredLink{
			direction: Up, otherID: pair.down.id, otherX: pair.down.x, otherY: pair.down.y, otherZ: pair.down.z, landing: pair.landing,
		}
		links[fmt.Sprintf("%d:%d,%d,%d", pair.down.id, pair.down.x, pair.down.y, pair.down.z)] = declaredLink{
			direction: Down, otherID: pair.up.id, otherX: pair.up.x, otherY: pair.up.y, otherZ: pair.up.z, landing: pair.downLanding,
		}
	}
	return links
}()

// declaredLinkFor returns the declared link for the object with
// id at loc, if one is declared. ClimbLinks.js: DECLARED_LINKS
// lookup in otherEnd() / destination().
func declaredLinkFor(id int, loc game.Location) (declaredLink, bool) {
	link, ok := declaredLinks[fmt.Sprintf("%d:%d,%d,%d", id, loc.X, loc.Y, loc.Z)]
	return link, ok
}

// DeclaredPairCount reports how many declared pairs the table
// holds (7, matching climb-links.json).
func DeclaredPairCount() int { return len(declaredPairs) }
