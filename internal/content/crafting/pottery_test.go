package crafting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func TestSoftClayFromBucket(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	give(p, fxClay, 2)
	give(p, fxBucketOfWater, 1)
	event := itemOnItemEvent(p, fxClay, fxBucketOfWater)
	c.makeSoftClay(event)
	if !event.Handled {
		t.Error("clay on bucket of water was not handled")
	}
	if got := countOf(p, fxSoftClay); got != 1 {
		t.Fatalf("soft clay = %d, want 1", got)
	}
	if got := countOf(p, fxBucket); got != 1 {
		t.Errorf("buckets = %d, want 1 (empty container returned)", got)
	}
	if !session.hasSpam("You mix the clay and water.<br>You now have some soft workable clay.") {
		t.Errorf("spam = %v, want the soft-clay line with <br>", session.spam)
	}
}

func TestSoftClayBowlOfWaterReturnsBowl(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	give(p, fxClay, 1)
	give(p, fxBowlOfWater, 1)
	c.makeSoftClay(itemOnItemEvent(p, fxClay, fxBowlOfWater))
	if got := countOf(p, fxSoftClay); got != 1 {
		t.Fatalf("soft clay = %d, want 1", got)
	}
	if got := countOf(p, fxBowl); got != 1 {
		t.Errorf("bowls = %d, want 1", got)
	}
}

func TestPotteryWheelShapeMenu(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 8)
	give(p, fxSoftClay, 3)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxWheelObj, p.Location()), ObjectID: fxWheelObj,
		Definition: testObjectDef{id: fxWheelObj, name: "Potter's wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
	if !c.usePotterWheel(event) {
		t.Error("potter's wheel was not handled")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if got := menu.Options().Mode; got != 0 {
		t.Errorf("menu mode = %d, want 0", got)
	}
	// The pot lid is quest-locked (fail-closed) and the cup
	// needs members; on a members world both are offered but
	// the lid's loop must refuse at choose time.
	for _, id := range menu.Items() {
		if id == fxUnfiredPotLid {
			t.Error("quest-locked pot lid offered on the wheel menu")
		}
	}
}

func TestPotteryShapingBowlTiming(t *testing.T) {
	c, _, tm := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 8)
	give(p, fxSoftClay, 3)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxWheelObj, p.Location()), ObjectID: fxWheelObj,
		Definition: testObjectDef{id: fxWheelObj, name: "Potter's wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
	if !c.usePotterWheel(event) {
		t.Fatal("potter's wheel was not handled")
	}
	session.menus[0].Execute(fxUnfiredBowl, 1)
	// Shaping is a potteryTask (interval 3, delay 2): the
	// first make lands on task tick 2.
	driveTaskTicks(tm, 1)
	if got := countOf(p, fxUnfiredBowl); got != 0 {
		t.Fatalf("unfired bowls after task tick 1 = %d, want 0", got)
	}
	driveTaskTicks(tm, 1)
	if got := countOf(p, fxUnfiredBowl); got != 1 {
		t.Fatalf("unfired bowls after task tick 2 = %d, want 1", got)
	}
	if !session.hasSpam("You make the clay into a bowl.") {
		t.Error("missing bowl shape message")
	}
	// Shaping XP 18 truncates to 18.
	if got := p.Skills().Experience(game.SkillCrafting); got != 18 {
		t.Errorf("crafting XP = %d, want 18", got)
	}
}

func TestPotteryFiringTiming(t *testing.T) {
	c, emitted, tm := newTestCrafting()
	c.randFloat = func() float64 { return 0.0 } // never cracks
	p, session := testPlayer("alice")
	setCrafting(p, 8)
	give(p, fxUnfiredBowl, 2)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxOvenObj, p.Location()), ObjectID: fxOvenObj,
		Definition: testObjectDef{id: fxOvenObj, name: "Pottery oven", interactions: []string{"Fire"}},
		ClickType:  1,
	}
	if !c.usePotteryOven(event) {
		t.Error("pottery oven was not handled")
	}
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	if got := session.menus[0].Options().Mode; got != 9 {
		t.Errorf("menu mode = %d, want 9", got)
	}
	session.menus[0].Execute(fxUnfiredBowl, 2)
	// The opening animate() is only the put message — no
	// item is consumed until the first make at task tick 6.
	driveTaskTicks(tm, 1)
	if got := countOf(p, fxUnfiredBowl); got != 2 {
		t.Fatalf("unfired bowls after task tick 1 = %d, want 2", got)
	}
	if !session.hasSpam("You put a bowl in the oven.") {
		t.Error("missing bowl put message")
	}
	// First removal at task tick 6.
	driveTaskTicks(tm, 5)
	if got := countOf(p, fxBowl); got != 1 {
		t.Fatalf("bowls after task tick 6 = %d, want 1", got)
	}
	if !session.hasSpam("You remove the bowl from the oven.") {
		t.Error("missing bowl remove message")
	}
	// Second removal at task tick 13.
	driveTaskTicks(tm, 7)
	if got := countOf(p, fxBowl); got != 2 {
		t.Fatalf("bowls after task tick 13 = %d, want 2", got)
	}
	if !hasEvent(emitted, PotteryFiredEventName) {
		t.Error("pottery:fired was not emitted")
	}
}

func TestPotteryFiringCurve(t *testing.T) {
	if got := firingChance(1); got != 181.0/256 {
		t.Errorf("firingChance(1) = %v, want 181/256", got)
	}
	if got := firingChance(12); got != 251.0/256 {
		t.Errorf("firingChance(12) = %v, want 251/256", got)
	}
	if got := firingChance(13); got != 1.0 {
		t.Errorf("firingChance(13) = %v, want 1.0 (guaranteed from 13)", got)
	}
	if got := firingChance(99); got != 1.0 {
		t.Errorf("firingChance(99) = %v, want 1.0", got)
	}
}

func TestPotteryCracksBelow13(t *testing.T) {
	c, emitted, tm := newTestCrafting()
	c.randFloat = func() float64 { return 0.99 } // above the level-1 chance 181/256: cracks
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxUnfiredPot, 1)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxOvenObj, p.Location()), ObjectID: fxOvenObj,
		Definition: testObjectDef{id: fxOvenObj, name: "Pottery oven", interactions: []string{"Fire"}},
		ClickType:  1,
	}
	if !c.usePotteryOven(event) {
		t.Fatal("pottery oven was not handled")
	}
	session.menus[0].Execute(fxUnfiredPot, 1)
	driveTaskTicks(tm, 6)
	// A cracked pot is destroyed silently (no product, no XP,
	// no pottery:fired): the Go port sends no crack line.
	if got := countOf(p, fxUnfiredPot); got != 0 {
		t.Errorf("unfired pots = %d, want 0 (consumed by the crack)", got)
	}
	if got := countOf(p, fxPot); got != 0 {
		t.Errorf("pots = %d, want 0 (cracked)", got)
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 0 {
		t.Errorf("crafting XP = %d, want 0 (cracked pieces pay nothing)", got)
	}
	if hasEvent(emitted, PotteryFiredEventName) {
		t.Error("pottery:fired emitted for a cracked pot, want none")
	}
}

func TestPotteryCupsNeverCrack(t *testing.T) {
	c, _, tm := newTestCrafting()
	c.randFloat = func() float64 { return 0.99 } // would crack a pot, but cups never crack
	p, session := testPlayer("alice")
	setCrafting(p, 3)
	// Shaping: one soft clay shapes a set of four cups.
	give(p, fxSoftClay, 1)
	wheel := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxWheelObj, p.Location()), ObjectID: fxWheelObj,
		Definition: testObjectDef{id: fxWheelObj, name: "Potter's wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
	if !c.usePotterWheel(wheel) {
		t.Fatal("potter's wheel was not handled")
	}
	session.menus[0].Execute(fxUnfiredCup, 1)
	driveTaskTicks(tm, 2)
	if got := countOf(p, fxUnfiredCup); got != 4 {
		t.Fatalf("unfired cups after shaping = %d, want 4 (a set of four)", got)
	}
	// Firing: one unfired cup fires into one empty cup, and
	// the roll above the crack chance still does not crack it.
	oven := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxOvenObj, p.Location()), ObjectID: fxOvenObj,
		Definition: testObjectDef{id: fxOvenObj, name: "Pottery oven", interactions: []string{"Fire"}},
		ClickType:  1,
	}
	if !c.usePotteryOven(oven) {
		t.Fatal("pottery oven was not handled")
	}
	ovenMenu := session.menus[len(session.menus)-1]
	ovenMenu.Execute(fxUnfiredCup, 1)
	driveTaskTicks(tm, 6)
	if got := countOf(p, fxEmptyCup); got != 1 {
		t.Fatalf("empty cups = %d, want 1 (cups never crack)", got)
	}
}

func TestPotteryWheelNoSoftClay(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxWheelObj, p.Location()), ObjectID: fxWheelObj,
		Definition: testObjectDef{id: fxWheelObj, name: "Potter's wheel", interactions: []string{"Spin"}},
		ClickType:  1,
	}
	if !c.usePotterWheel(event) {
		t.Error("wheel not handled with no soft clay, want handled (with a statement)")
	}
	text, ok := statementText(p)
	if !ok || text != "You don't have anything suitable to craft with." {
		t.Errorf("statement = %q, %v, want the nothing-to-craft line", text, ok)
	}
	if len(session.menus) != 0 {
		t.Errorf("menus = %d, want 0", len(session.menus))
	}
}

func TestPotteryOvenNoUnfired(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 1)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: testGameObject(fxOvenObj, p.Location()), ObjectID: fxOvenObj,
		Definition: testObjectDef{id: fxOvenObj, name: "Pottery oven", interactions: []string{"Fire"}},
		ClickType:  1,
	}
	if !c.usePotteryOven(event) {
		t.Error("oven not handled with no unfired pottery, want handled (with a message)")
	}
	if !p.Session.(*fakeSession).hasMessage("You have no unfired pottery to fire.") {
		t.Errorf("messages = %v, want the no-unfired line", p.Session.(*fakeSession).messages)
	}
}

func TestPotteryPotLidQuestLocked(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 25)
	give(p, fxSoftClay, 1)
	// No quest:get-stage answerer: the pot lid is locked.
	recipe := c.tables.Pottery.Recipes[3]
	if c.recipeUnlocked(p, recipe) {
		t.Error("pot-lid recipe unlocked with no quest answerer, want locked")
	}
}
