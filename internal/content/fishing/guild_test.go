package fishing

import (
	"testing"

	"tsps-go/internal/game"
)

// Guild tests: the guild bounds and the invisible +7
// (Guild.Fishing.js). Fixtures only.

func TestIsInGuild(t *testing.T) {
	f, _ := newTestFishing(t)
	cases := []struct {
		name    string
		x, y, z int
		want    bool
	}{
		{"middle of the guild", 2600, 3400, 0, true},
		{"south-west corner", 2560, 3394, 0, true},
		{"north-east corner", 2623, 3436, 0, true},
		{"door tile", 2611, 3394, 0, true},
		{"one west of the guild", 2559, 3400, 0, false},
		{"one east of the guild", 2624, 3400, 0, false},
		{"one south of the guild", 2600, 3393, 0, false},
		{"Kylie platform, y 3437", 2610, 3437, 0, false},
		{"first floor", 2600, 3400, 1, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := game.NewPlayer("alice", game.Location{X: c.x, Y: c.y, Z: c.z})
			if got := f.isInGuild(p); got != c.want {
				t.Fatalf("isInGuild(%d,%d,%d) = %v, want %v", c.x, c.y, c.z, got, c.want)
			}
		})
	}
	if f.isInGuild(nil) {
		t.Error("nil player must not be in the guild")
	}
}

func TestGuildInvisibleBoost(t *testing.T) {
	f, _ := newTestFishing(t)
	inside := newTestPlayer("alice", 2600, 3400)
	if got := f.guildInvisibleBoost(inside); got != 7 {
		t.Fatalf("inside boost = %d, want 7", got)
	}
	outside := newTestPlayer("bob", 3000, 3000)
	if got := f.guildInvisibleBoost(outside); got != 0 {
		t.Fatalf("outside boost = %d, want 0", got)
	}
	platform := newTestPlayer("carol", 2610, 3437)
	if got := f.guildInvisibleBoost(platform); got != 0 {
		t.Fatalf("platform boost = %d, want 0 (no boost above y 3436)", got)
	}
	if got := f.guildInvisibleBoost(nil); got != 0 {
		t.Fatalf("nil player boost = %d, want 0", got)
	}
}
