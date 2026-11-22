package doors

import (
	"testing"

	"tsps-go/internal/game"
)

// TestCrossingFor pins crossingFor (WallDoor.Doors.js, the whole file):
// a type-0 wall door separates its own tile from the neighbour across
// its face; the approach is the tile on the player's side, the
// destination the tile on the other side; any other type has no
// crossing.
func TestCrossingFor(t *testing.T) {
	at := game.Location{X: 3225, Y: 3240, Z: 0}
	cases := []struct {
		name                  string
		objectType, face      int
		from                  game.Location
		wantApproach, wantDst game.Location
		wantOK                bool
	}{
		{"face 2 east, player west approaches own tile, dest east", 0, 2,
			game.Location{X: 3224, Y: 3240, Z: 0}, at, game.Location{X: 3226, Y: 3240, Z: 0}, true},
		{"face 2 east, player east approaches neighbour, dest own tile", 0, 2,
			game.Location{X: 3226, Y: 3240, Z: 0}, game.Location{X: 3226, Y: 3240, Z: 0}, at, true},
		{"face 0 west, player east approaches own tile, dest west", 0, 0,
			game.Location{X: 3226, Y: 3240, Z: 0}, at, game.Location{X: 3224, Y: 3240, Z: 0}, true},
		{"face 0 west, player west approaches neighbour, dest own tile", 0, 0,
			game.Location{X: 3224, Y: 3240, Z: 0}, game.Location{X: 3224, Y: 3240, Z: 0}, at, true},
		{"face 1 north, player south approaches own tile, dest north", 0, 1,
			game.Location{X: 3225, Y: 3239, Z: 0}, at, game.Location{X: 3225, Y: 3241, Z: 0}, true},
		{"face 3 south, player north approaches own tile, dest south", 0, 3,
			game.Location{X: 3225, Y: 3241, Z: 0}, at, game.Location{X: 3225, Y: 3239, Z: 0}, true},
		{"player beside the wall (not across the face) approaches own tile", 0, 2,
			game.Location{X: 3225, Y: 3241, Z: 0}, at, game.Location{X: 3226, Y: 3240, Z: 0}, true},
		{"face wraps with & 3", 0, 6,
			game.Location{X: 3224, Y: 3240, Z: 0}, at, game.Location{X: 3226, Y: 3240, Z: 0}, true},
		{"type 10 solid object has no crossing", 10, 2,
			game.Location{X: 3224, Y: 3240, Z: 0}, game.Location{}, game.Location{}, false},
		{"type 9 diagonal door has no crossing", 9, 2,
			game.Location{X: 3224, Y: 3240, Z: 0}, game.Location{}, game.Location{}, false},
		{"level is preserved", 0, 1,
			game.Location{X: 3225, Y: 3239, Z: 1}, game.Location{X: 3225, Y: 3240, Z: 1}, game.Location{X: 3225, Y: 3241, Z: 1}, true},
	}
	for _, c := range cases {
		// The level case stands on level 1, so move the door there too.
		doorAt := at
		if c.from.Z == 1 {
			doorAt = game.Location{X: 3225, Y: 3240, Z: 1}
		}
		approach, dest, ok := CrossingFor(c.objectType, c.face, doorAt, c.from)
		if ok != c.wantOK || approach != c.wantApproach || dest != c.wantDst {
			t.Errorf("%s: CrossingFor = (%+v, %+v, %v), want (%+v, %+v, %v)",
				c.name, approach, dest, ok, c.wantApproach, c.wantDst, c.wantOK)
		}
	}
}
