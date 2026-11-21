package cooking

import (
	"testing"

	"tsps-go/internal/game"
)

// Leaping Fish unit tests (LeapingFish.Cooking.js): hand-built
// fixtures only. Fixture ids: trout 510, salmon 511, sturgeon
// 512, roe 513, caviar 514, offcuts 515, knife 946 (fxKnife).

func wcFishResolver() wcMapResolver {
	return wcMapResolver{objs: map[string]int{
		"brut_spawning_trout":  510,
		"brut_roe":             513,
		"brut_spawning_salmon": 511,
		"brut_sturgeon":        512,
		"brut_caviar":          514,
		"brut_fish_cuts":       515,
	}}
}

func TestLeapingFishRoeChance(t *testing.T) {
	trout := &leapingFishRow{FishName: "brut_spawning_trout", Per: 150}
	salmon := &leapingFishRow{FishName: "brut_spawning_salmon", Per: 80}
	if got := roeChance(trout, 1); got != 1.0/150 {
		t.Fatalf("roeChance(trout, 1) = %v, want %v", got, 1.0/150)
	}
	if got := roeChance(trout, 99); got != 99.0/150 {
		t.Fatalf("roeChance(trout, 99) = %v, want %v", got, 99.0/150)
	}
	if got := roeChance(salmon, 1); got != 1.0/80 {
		t.Fatalf("roeChance(salmon, 1) = %v, want %v", got, 1.0/80)
	}
	if got := roeChance(salmon, 80); got != 1 {
		t.Fatalf("roeChance(salmon, 80) = %v, want 1", got)
	}
	if got := roeChance(salmon, 99); got != 1 {
		t.Fatalf("roeChance(salmon, 99) = %v, want 1 (capped)", got)
	}
}

func TestLeapingFishCutOneSuccess(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0 } // roe + offcuts both hit
	u.cycle = func() int64 { return 7 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, 510, 2)
	give(p, fxKnife, 1)
	fish := u.fishFor(wcItemOnItem(p, fxKnife, 510))
	if fish == nil || fish.FishID != 510 {
		t.Fatalf("fishFor(knife, trout) = %v, want the trout row", fish)
	}
	if !u.cutOne(p, fish) {
		t.Fatal("cutOne = false, want true")
	}
	if got := countOf(p, 510); got != 1 {
		t.Fatalf("trout = %d, want 1", got)
	}
	if got := countOf(p, 513); got != 1 {
		t.Fatalf("roe = %d, want 1", got)
	}
	if got := countOf(p, 515); got != 1 {
		t.Fatalf("offcuts = %d, want 1", got)
	}
	if got := wcXP(p); got != 10 {
		t.Fatalf("xp = %d, want 10", got)
	}
	if got := lastCutTick(p); got != 7 {
		t.Fatalf("last cut tick = %d, want 7", got)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].Kind != "leaping-fish" || produced[0].ItemID != 513 || produced[0].XP != 10 {
		t.Fatalf("produced = %+v, want one leaping-fish event for roe / 10 xp", produced)
	}
}

func TestLeapingFishCutOneMissConsumesFish(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0.999 } // roe roll misses
	u.cycle = func() int64 { return 7 }
	p := newTestPlayer("alice")
	setCooking(p, 1)
	give(p, 510, 1)
	give(p, fxKnife, 1)
	fish := u.fishFor(wcItemOnItem(p, fxKnife, 510))
	if !u.cutOne(p, fish) {
		t.Fatal("cutOne = false, want true (the fish is still consumed)")
	}
	if got := countOf(p, 510); got != 0 {
		t.Fatalf("trout = %d, want 0", got)
	}
	if got := countOf(p, 513); got != 0 {
		t.Fatalf("roe = %d, want 0 (missed)", got)
	}
	if got := wcXP(p); got != 0 {
		t.Fatalf("xp = %d, want 0", got)
	}
	if len(wcProduced(emitted)) != 0 {
		t.Fatalf("produced = %d events, want none", len(wcProduced(emitted)))
	}
	// The failed cut still stamps the tick.
	if got := lastCutTick(p); got != 7 {
		t.Fatalf("last cut tick = %d, want 7", got)
	}
}

func TestLeapingFishSameTickGuard(t *testing.T) {
	c, _, tm := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0 }
	u.cycle = func() int64 { return 5 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, 510, 2)
	give(p, fxKnife, 1)
	u.cut(wcItemOnItem(p, fxKnife, 510))
	if got := countOf(p, 510); got != 1 {
		t.Fatalf("trout after first cut = %d, want 1", got)
	}
	// Same cycle: the second use cuts nothing (and does not
	// schedule another auto-cut).
	u.cut(wcItemOnItem(p, fxKnife, 510))
	if got := countOf(p, 510); got != 1 {
		t.Fatalf("trout after same-tick cut = %d, want 1", got)
	}
	driveTicks(tm, 3)
	if got := countOf(p, 510); got != 0 {
		t.Fatalf("trout after auto-cut = %d, want 0", got)
	}
}

func TestLeapingFishAutoCutEveryThreeTicks(t *testing.T) {
	c, _, tm := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0 }
	u.cycle = func() int64 { return 5 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	give(p, 510, 3)
	give(p, fxKnife, 1)
	u.cut(wcItemOnItem(p, fxKnife, 510))
	if got := countOf(p, 510); got != 2 {
		t.Fatalf("trout after immediate cut = %d, want 2", got)
	}
	driveTicks(tm, 2)
	if got := countOf(p, 510); got != 2 {
		t.Fatalf("trout after 2 ticks = %d, want 2", got)
	}
	driveTicks(tm, 1)
	if got := countOf(p, 510); got != 1 {
		t.Fatalf("trout after 3 ticks = %d, want 1", got)
	}
	driveTicks(tm, 3)
	if got := countOf(p, 510); got != 0 {
		t.Fatalf("trout after 6 ticks = %d, want 0", got)
	}
	// No fish left: the auto-cut stops on its own.
	driveTicks(tm, 3)
	if got := countOf(p, 513); got != 3 {
		t.Fatalf("roe = %d, want 3", got)
	}
	if got := wcXP(p); got != 30 {
		t.Fatalf("xp = %d, want 30", got)
	}
}

func TestLeapingFishOffcutsGroundDropWhenFull(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0 }
	u.cycle = func() int64 { return 5 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	for i := 0; i < 26; i++ {
		give(p, 1000+i, 1)
	}
	give(p, 510, 1)
	give(p, fxKnife, 1)
	var dropped []game.Item
	old := game.Hooks.DropGroundItem
	game.Hooks.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
		dropped = append(dropped, item)
	}
	defer func() { game.Hooks.DropGroundItem = old }()
	u.cut(wcItemOnItem(p, fxKnife, 510))
	if got := countOf(p, 513); got != 1 {
		t.Fatalf("roe = %d, want 1 (took the freed fish slot)", got)
	}
	if len(dropped) != 1 || dropped[0].ID() != 515 {
		t.Fatalf("dropped = %v, want [offcuts]", dropped)
	}
}

func TestLeapingFishFreeWorldSilent(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newLeapingFishUnit(c, wcFishResolver())
	u.randFloat = func() float64 { return 0 }
	p := newTestPlayer("alice")
	setCooking(p, 99)
	c.membersWorld = false
	give(p, 510, 1)
	give(p, fxKnife, 1)
	u.cut(wcItemOnItem(p, fxKnife, 510))
	if got := countOf(p, 510); got != 1 {
		t.Fatalf("trout = %d, want 1 (kept)", got)
	}
}
