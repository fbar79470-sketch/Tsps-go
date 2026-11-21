package fishing

import (
	"testing"

	"tsps-go/internal/game"
)

// ClueBottles tests: the denominator math, the tier
// table, the roll (deterministic denominators), the
// can-receive fallback, the vessel-chance block, the F2P
// tier skip, and the Wilderness ring
// (ClueBottles.Fishing.js). Fixtures only; drops are
// captured through game.Hooks.DropGroundItem.

// dropCapture records ground drops for a test and restores
// the previous hook on cleanup.
type dropCapture struct {
	drops []game.Item
	prev  func(item game.Item, loc game.Location, owner *game.Player)
}

func captureDrops(t *testing.T) *dropCapture {
	t.Helper()
	c := &dropCapture{prev: game.Hooks.DropGroundItem}
	game.Hooks.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
		c.drops = append(c.drops, item)
	}
	t.Cleanup(func() { game.Hooks.DropGroundItem = c.prev })
	return c
}

func (c *dropCapture) count(id int) int {
	n := 0
	for _, item := range c.drops {
		if item.ID() == id {
			n++
		}
	}
	return n
}

func TestBottleDenominator(t *testing.T) {
	cases := []struct {
		name                string
		base, level, tenths int
		ca                  bool
		want                int
	}{
		{"park base at 99, beginner", 152120, 99, 2, false, 152},
		{"100k at 99, beginner", 100000, 99, 2, false, 100},
		{"100k at 99, beginner, CA met", 100000, 99, 2, true, 95},
		{"600 at 1, beginner", 600, 1, 2, false, 1},
		{"100 at 1, beginner", 100, 1, 2, false, 0},
		{"100 at 1, easy", 100, 1, 17, false, 1},
		{"100 at 1, medium", 100, 1, 20, false, 1},
		{"100 at 1, elite", 100, 1, 100, false, 9},
		{"zero base", 0, 1, 2, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := bottleDenominator(c.base, c.level, c.tenths, c.ca); got != c.want {
				t.Fatalf("bottleDenominator(%d, %d, %d, %v) = %d, want %d",
					c.base, c.level, c.tenths, c.ca, got, c.want)
			}
		})
	}
}

func TestClueBottleTiers(t *testing.T) {
	f, _ := newTestFishing(t)
	tiers := f.clueBottleTiers()
	if len(tiers) != 5 {
		t.Fatalf("clueBottleTiers = %d tiers, want 5", len(tiers))
	}
	want := []clueBottleTier{
		{"beginner", 2, f.ids.ClueBottleBeginner},
		{"easy", 17, f.ids.ClueBottleEasy},
		{"medium", 20, f.ids.ClueBottleMedium},
		{"hard", 33, f.ids.ClueBottleHard},
		{"elite", 100, f.ids.ClueBottleElite},
	}
	for i, w := range want {
		if tiers[i] != w {
			t.Errorf("tier %d = %+v, want %+v", i, tiers[i], w)
		}
	}
	if (&fishing{}).clueBottleTiers() != nil {
		t.Error("no-ids tiers must be nil")
	}
}

func TestRollClueBottlesBaseZero(t *testing.T) {
	f, _ := newTestFishing(t)
	c := captureDrops(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillFishing, 99)
	f.rollClueBottles(p, 0)
	if len(c.drops) != 0 {
		t.Fatalf("base 0 dropped %v, want nothing", c.drops)
	}
	f.rollClueBottles(nil, 100000)
	if len(c.drops) != 0 {
		t.Fatalf("nil player dropped %v, want nothing", c.drops)
	}
	(&fishing{}).rollClueBottles(p, 100000)
	if len(c.drops) != 0 {
		t.Fatalf("no-ids roll dropped %v, want nothing", c.drops)
	}
}

func TestRollClueBottlesBeginnerHit(t *testing.T) {
	// Level 1, base 600: the beginner denominator is
	// exactly 1, so the beginner roll always lands; the
	// bottle falls to the ground (no forceAdd in Go).
	f, _ := newTestFishing(t)
	c := captureDrops(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillFishing, 1)
	f.rollClueBottles(p, 600)
	if got := c.count(f.ids.ClueBottleBeginner); got != 1 {
		t.Fatalf("beginner bottles dropped = %d, want 1 (drops %v)", got, c.drops)
	}
}

func TestRollClueBottlesMembersControl(t *testing.T) {
	// Level 1, base 100: easy and medium denominators are
	// exactly 1, beginner is 0 — on a members world both
	// land; on a free world neither tier rolls at all.
	f, _ := newTestFishing(t)
	c := captureDrops(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillFishing, 1)
	f.rollClueBottles(p, 100)
	if got := c.count(f.ids.ClueBottleEasy); got != 1 {
		t.Fatalf("members easy bottles = %d, want 1 (drops %v)", got, c.drops)
	}
	if got := c.count(f.ids.ClueBottleMedium); got != 1 {
		t.Fatalf("members medium bottles = %d, want 1 (drops %v)", got, c.drops)
	}
	f.membersWorld = false
	c2 := captureDrops(t)
	p2 := newTestPlayer("bob", 3000, 3000)
	setLevel(p2, game.SkillFishing, 1)
	f.rollClueBottles(p2, 100)
	if len(c2.drops) != 0 {
		t.Fatalf("free-world drops = %v, want nothing", c2.drops)
	}
}

func TestRollClueBottlesOwnedSkips(t *testing.T) {
	// The player already owns a beginner bottle (carried):
	// the roll hits, the missed-clue emit fires, and no
	// bottle drops.
	f, emitted := newTestFishing(t)
	c := captureDrops(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillFishing, 1)
	p.SetItemDefinitions(namedDefs{names: map[int]string{
		f.ids.ClueBottleBeginner: "Clue bottle (beginner)",
	}})
	p.Inventory().Adds(f.ids.ClueBottleBeginner, 1)
	f.rollClueBottles(p, 600)
	if got := c.count(f.ids.ClueBottleBeginner); got != 0 {
		t.Fatalf("owned beginner bottles dropped = %d, want 0", got)
	}
	missed := 0
	for _, e := range *emitted {
		if e.name == treasureTrailsMissedClueEventName {
			missed++
			ev, ok := e.payload.(*MissedClueEvent)
			if !ok || ev.Tier != "beginner" || ev.Player != p {
				t.Fatalf("missed-clue payload = %+v, want beginner for the player", e.payload)
			}
		}
	}
	if missed != 1 {
		t.Fatalf("missed-clue emitted %d times, want 1", missed)
	}
}

func TestRollClueBottlesVesselChanceBlocks(t *testing.T) {
	// A vessel-chance answerer blocking easy: easy never
	// drops (its denominator is 1 here), medium still does.
	f, _ := newTestFishing(t)
	f.emit = func(name string, payload any) {
		if name == treasureTrailsVesselChanceEventName {
			if req, ok := payload.(*VesselChanceRequest); ok && req.Tier == "easy" {
				blocked := false
				req.Allowed = &blocked
			}
		}
	}
	c := captureDrops(t)
	p := newTestPlayer("alice", 3000, 3000)
	setLevel(p, game.SkillFishing, 1)
	f.rollClueBottles(p, 100)
	if got := c.count(f.ids.ClueBottleEasy); got != 0 {
		t.Fatalf("blocked easy bottles dropped = %d, want 0", got)
	}
	if got := c.count(f.ids.ClueBottleMedium); got != 1 {
		t.Fatalf("medium bottles dropped = %d, want 1 (drops %v)", got, c.drops)
	}
}

// fakeZoneSource is a ZoneBoundarySource test double: one
// boundary under the "pvp" tag, nothing under any other.
type fakeZoneSource struct{ b game.Boundary }

func (s fakeZoneSource) ZoneBoundaries(tag string) []game.Boundary {
	if tag == "pvp" {
		return []game.Boundary{s.b}
	}
	return nil
}

func TestHasWildernessRing(t *testing.T) {
	f, _ := newTestFishing(t)
	// The boundary covers (3000..3100, 3500..3600) on z 0;
	// (3050, 3550) is inside the levelled strip too, so the
	// wilderness level is 4.
	wild := game.NewWilderness(fakeZoneSource{b: game.NewBoundary(3000, 3100, 3500, 3600, 0)}, nil)
	SetWilderness(wild)
	defer SetWilderness(nil)
	ring := f.ids.ImbuedRings[0]
	inside := newTestPlayer("alice", 3050, 3550)
	inside.Equipment().SetItem(game.RingSlot, game.NewItem(ring, 1))
	if !f.hasWildernessRing(inside) {
		t.Error("ring worn in the Wilderness must double")
	}
	bare := newTestPlayer("bob", 3050, 3550)
	if f.hasWildernessRing(bare) {
		t.Error("no ring worn must not double")
	}
	carried := newTestPlayer("carol", 3050, 3550)
	carried.Inventory().Adds(ring, 1)
	if f.hasWildernessRing(carried) {
		t.Error("a carried ring must not double")
	}
	outside := newTestPlayer("dave", 3000, 3000)
	outside.Equipment().SetItem(game.RingSlot, game.NewItem(ring, 1))
	if f.hasWildernessRing(outside) {
		t.Error("a ring worn outside the Wilderness must not double")
	}
	// No Wilderness installed: the ring never doubles.
	SetWilderness(nil)
	if f.hasWildernessRing(inside) {
		t.Error("a ring with no Wilderness installed must not double")
	}
	if f.hasWildernessRing(nil) {
		t.Error("nil player must not double")
	}
}
