package mining

import (
	"testing"

	"tsps-go/internal/game"
)

// ---- calcified water ----

// placeCalcifiedRocks stands four calcified rocks in the mine
// chamber as map originals: two west, two east.
func placeCalcifiedRocks(objects *fakeObjects) {
	sites := []struct {
		id, x, y int
	}{
		{51485, 1497, 9540}, {51487, 1497, 9543}, // west
		{51489, 1514, 9546}, {51491, 1518, 9537}, // east
	}
	for _, s := range sites {
		obj := game.NewGameObject(s.id, game.Location{X: s.x, Y: s.y, Z: 1}, 10, 0, nil)
		objects.placeMapOriginal(obj)
	}
}

func TestCalcifiedSites(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	placeCalcifiedRocks(objects)
	sites := m.calcifiedSites()
	if len(sites) != 4 {
		t.Fatalf("sites = %d, want 4", len(sites))
	}
	// The scan is cached: a second call returns the same list
	// without re-scanning.
	if got := m.calcifiedSites(); len(got) != 4 {
		t.Fatalf("second scan sites = %d, want 4", len(got))
	}
}

func TestWaterCycle(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	placeCalcifiedRocks(objects)
	// randInt returns min: the ooze rolls 117 ticks out, the
	// wet window rolls 38 ticks.
	for i := 0; i < 117; i++ {
		m.tickWater()
	}
	if len(m.water.wet) != 4 {
		t.Fatalf("after 117 ticks: wet = %d, want 4 (2 per half)", len(m.water.wet))
	}
	if m.water.stopAt != 117+38 {
		t.Errorf("stopAt = %d, want 155", m.water.stopAt)
	}
	// A wet rock answers isWetRock.
	for _, wet := range m.water.wet {
		if !m.isWetRock(wet.water) {
			t.Error("wet rock does not answer isWetRock")
		}
		// The water boost turns a miss into a hit with
		// probability (0.575-0.5)/(1-0.5) = 0.15; rand 0.0
		// always hits it.
		attempt := &attemptEvent{
			Rock: "Calcified rocks", Object: wet.water,
			Chance: 0.5, Success: false,
		}
		m.randFloat = func() float64 { return 0.0 }
		m.waterBoost(attempt)
		if !attempt.Success {
			t.Error("water boost with rand 0.0: want success")
		}
		attempt2 := &attemptEvent{
			Rock: "Calcified rocks", Object: wet.water,
			Chance: 0.5, Success: false,
		}
		m.randFloat = func() float64 { return 0.5 }
		m.waterBoost(attempt2)
		if attempt2.Success {
			t.Error("water boost with rand 0.5: want still a miss")
		}
		break
	}
	// Tick to the stop: the water drains, and one tick later
	// the wet rocks mine out.
	for m.water.tick < 155 {
		m.tickWater()
	}
	if m.water.stopAt != -1 {
		t.Errorf("after stop: stopAt = %d, want -1", m.water.stopAt)
	}
	if m.water.depleteAt != 156 {
		t.Errorf("depleteAt = %d, want 156", m.water.depleteAt)
	}
	m.tickWater() // tick 156
	if len(m.water.wet) != 0 {
		t.Errorf("after deplete: wet = %d, want 0", len(m.water.wet))
	}
	// The four rocks are mined out (their _empty locs stand).
	empties := 0
	for _, id := range []int{51486, 51488, 51490, 51492} {
		for _, loc := range []game.Location{
			{X: 1497, Y: 9540, Z: 1}, {X: 1497, Y: 9543, Z: 1},
			{X: 1514, Y: 9546, Z: 1}, {X: 1518, Y: 9537, Z: 1},
		} {
			if objects.Get(id, loc) != nil {
				empties++
			}
		}
	}
	if empties != 4 {
		t.Errorf("depleted locs = %d, want 4", empties)
	}
}

func TestWaterBoostGuards(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	placeCalcifiedRocks(objects)
	// Drive the cycle to the ooze so a wet rock exists.
	for i := 0; i < 117; i++ {
		m.tickWater()
	}
	// A boost on a non-calcified rock does nothing.
	attempt := &attemptEvent{
		Rock: "Copper rocks", Chance: 0.5, Success: false,
	}
	m.randFloat = func() float64 { return 0.0 }
	m.waterBoost(attempt)
	if attempt.Success {
		t.Error("non-calcified rock: want no boost")
	}
	// A boost on an already-successful roll does nothing.
	for _, wet := range m.water.wet {
		attempt = &attemptEvent{
			Rock: "Calcified rocks", Object: wet.water,
			Chance: 0.5, Success: true,
		}
		m.waterBoost(attempt)
		break
	}
}

// ---- ancient essence ----

func TestAncientEssenceAmount(t *testing.T) {
	m, _ := newTestMining(t)
	// randInt returns min: 1-2 at 75-77, 2-3 at 78-89, 3-4
	// from 90.
	cases := []struct {
		level, want int
	}{
		{74, 1}, {75, 1}, {77, 1}, {78, 2}, {89, 2}, {90, 3}, {99, 3},
	}
	for _, c := range cases {
		if got := m.ancientEssenceAmount(c.level); got != c.want {
			t.Errorf("amount(%d) = %d, want %d", c.level, got, c.want)
		}
	}
}

func placeAncientWalls(objects *fakeObjects) {
	for _, spot := range ancientSpots {
		obj := game.NewGameObject(ancientWallID, game.Location{X: spot.x, Y: spot.y, Z: 0}, 10, spot.face, nil)
		objects.placeMapOriginal(obj)
	}
}

func TestAncientMove(t *testing.T) {
	m, _ := newTestMining(t)
	objects := newFakeObjects()
	m.objects = objects
	placeAncientWalls(objects)
	// First tick: the crystals appear at a random spot —
	// randInt returns min, so the first of the three.
	m.tickAncient()
	if m.ancient.active == nil {
		t.Fatal("no active spot after first tick")
	}
	first := m.ancient.active
	if got := objects.Get(ancientCrystalsID, game.Location{X: first.x, Y: first.y, Z: 0}); got == nil {
		t.Errorf("no crystals at the active spot (%d, %d)", first.x, first.y)
	}
	if got := objects.Get(ancientWallID, game.Location{X: first.x, Y: first.y, Z: 0}); got != nil {
		t.Error("wall still stands at the active spot")
	}
	if m.ancient.wait != 100 {
		t.Errorf("wait = %d, want 100 (randInt min)", m.ancient.wait)
	}
	// 99 ticks pass without moving; the 100th moves it to one
	// of the other two spots (randInt min → the first other).
	for i := 0; i < 99; i++ {
		m.tickAncient()
	}
	if m.ancient.active != first {
		t.Error("crystals moved before the wait expired")
	}
	m.tickAncient()
	if m.ancient.active == first {
		t.Error("crystals did not move after the wait expired")
	}
	second := m.ancient.active
	if got := objects.Get(ancientCrystalsID, game.Location{X: second.x, Y: second.y, Z: 0}); got == nil {
		t.Errorf("no crystals at the new spot (%d, %d)", second.x, second.y)
	}
	// The old spot is a wall again.
	if got := objects.Get(ancientWallID, game.Location{X: first.x, Y: first.y, Z: 0}); got == nil {
		t.Error("old spot is not a wall again")
	}
}
