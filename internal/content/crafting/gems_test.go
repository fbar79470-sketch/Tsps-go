package crafting

import (
	"math"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func itemOnItemEvent(p *game.Player, used, with int) *plugins.PluginItemOnItemEvent {
	return &plugins.PluginItemOnItemEvent{
		Player: p, UsedItemID: used, UsedWithItemID: with,
		UsedItem: game.NewItem(used, 1), UsedWithItem: game.NewItem(with, 1),
	}
}

func TestCutChanceBoundaries(t *testing.T) {
	// Opal crush curve (low 128, high 250): level 1 and 99.
	if got := cutChance(1, 128, 250); math.Abs(got-129.0/256) > 1e-9 {
		t.Errorf("cutChance(1, 128, 250) = %v, want 129/256", got)
	}
	if got := cutChance(99, 128, 250); math.Abs(got-251.0/256) > 1e-9 {
		t.Errorf("cutChance(99, 128, 250) = %v, want 251/256", got)
	}
	// Clamped outside 1..99.
	if got := cutChance(0, 128, 250); got != cutChance(1, 128, 250) {
		t.Errorf("cutChance(0) = %v, want the level-1 value", got)
	}
	if got := cutChance(150, 128, 250); got != cutChance(99, 128, 250) {
		t.Errorf("cutChance(150) = %v, want the level-99 value", got)
	}
}

func TestGemCutMenuMode12(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 20)
	give(p, fxChisel, 1)
	give(p, fxUncutSapphire, 3)
	c.cutGem(itemOnItemEvent(p, fxChisel, fxUncutSapphire))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != "How many gems would you like to cut?" {
		t.Errorf("menu title = %q", menu.Title())
	}
	if got := menu.Options().Mode; got != 12 {
		t.Errorf("menu mode = %d, want 12", got)
	}
}

func TestGemCutFirstTick1ThenEvery2(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.randFloat = func() float64 { return 1.0 } // sapphire has no crush; roll unused
	p, session := testPlayer("alice")
	setCrafting(p, 20)
	give(p, fxChisel, 1)
	give(p, fxUncutSapphire, 3)
	c.cutGem(itemOnItemEvent(p, fxChisel, fxUncutSapphire))
	session.menus[0].Execute(fxUncutSapphire, 3)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxSapphire); got != 1 {
		t.Fatalf("sapphires after tick 1 = %d, want 1", got)
	}
	if !session.hasSpam("You cut the sapphire.") {
		t.Error("missing 'You cut the sapphire.' spam")
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxSapphire); got != 1 {
		t.Fatalf("sapphires after tick 2 = %d, want 1 (every 2 ticks)", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxSapphire); got != 2 {
		t.Fatalf("sapphires after tick 3 = %d, want 2", got)
	}
}

func TestGemTooLowNextTickItemBox(t *testing.T) {
	c, _, tm := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 19)
	give(p, fxChisel, 1)
	give(p, fxUncutSapphire, 1)
	c.cutGem(itemOnItemEvent(p, fxChisel, fxUncutSapphire))
	if _, _, ok := itemStatementText(p); ok {
		t.Fatal("item box shown before the next tick")
	}
	driveTaskTicks(tm, 1)
	id, text, ok := itemStatementText(p)
	if !ok || id != fxUncutSapphire || text != "You need a Crafting level of 20 to cut sapphires." {
		t.Errorf("item statement = (%d, %q), %v, want the sapphire level line", id, text, ok)
	}
}

func TestGemCrushFailsOnBadRoll(t *testing.T) {
	c, _, _ := newTestCrafting()
	// Roll 1.0: above the level-1 crush chance (129/256), so
	// the opal smashes (cutOne crushes when roll >= chance).
	c.randFloat = func() float64 { return 1.0 }
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxChisel, 1)
	give(p, fxUncutOpal, 2)
	c.cutGem(itemOnItemEvent(p, fxChisel, fxUncutOpal))
	session.menus[0].Execute(fxUncutOpal, 1)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxCrushedGem); got != 1 {
		t.Fatalf("crushed gems = %d, want 1", got)
	}
	if got := countOf(p, fxOpal); got != 0 {
		t.Errorf("opals = %d, want 0 (smashed, not cut)", got)
	}
	if !session.hasSpam("You mis-hit the chisel and smash the opal to pieces!") {
		t.Error("missing crush spam")
	}
	// Crush XP 3.8 truncates to 3.
	if got := p.Skills().Experience(game.SkillCrafting); got != 3 {
		t.Errorf("crafting XP = %d, want 3 (3.8 truncated)", got)
	}
}

func TestGemCrushSucceedsOnGoodRoll(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.randFloat = func() float64 { return 0.0 } // below the crush chance: cuts cleanly
	p, _ := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxChisel, 1)
	give(p, fxUncutOpal, 1)
	c.cutGem(itemOnItemEvent(p, fxChisel, fxUncutOpal))
	p.Session.(*fakeSession).menus[0].Execute(fxUncutOpal, 1)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxOpal); got != 1 {
		t.Fatalf("opals = %d, want 1", got)
	}
}

func TestJewellersChiselDoubleCut(t *testing.T) {
	c, _, _ := newTestCrafting()
	// Sapphire has no crush roll, so the only roll is the
	// double-cut roll: 0.05 (< 0.1) triggers the double cut.
	c.randFloat = func() float64 { return 0.05 }
	p, _ := testPlayer("alice")
	setCrafting(p, 20)
	give(p, fxJewellersChisel, 1)
	give(p, fxUncutSapphire, 2)
	c.cutGem(itemOnItemEvent(p, fxJewellersChisel, fxUncutSapphire))
	p.Session.(*fakeSession).menus[0].Execute(fxUncutSapphire, 1)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxSapphire); got != 2 {
		t.Fatalf("sapphires = %d, want 2 (jeweller's double cut)", got)
	}
	if got := countOf(p, fxUncutSapphire); got != 0 {
		t.Errorf("uncut sapphires left = %d, want 0", got)
	}
}

func TestAmethystMenuAndCut(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 83)
	give(p, fxChisel, 1)
	give(p, fxAmethyst, 2)
	c.handleAmethyst(itemOnItemEvent(p, fxChisel, fxAmethyst))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	if got := session.menus[0].Title(); got != "What would you like to make?" {
		t.Errorf("menu title = %q", got)
	}
	if got := session.menus[0].Options().Mode; got != 0 {
		t.Errorf("menu mode = %d, want 0", got)
	}
	session.menus[0].Execute(fxAmethystBoltTips, 1)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxAmethystBoltTips); got != 15 {
		t.Fatalf("amethyst bolt tips = %d, want 15", got)
	}
	if got := countOf(p, fxAmethyst); got != 1 {
		t.Errorf("amethyst left = %d, want 1", got)
	}
}

func TestAmethystTooLowMessage(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 82)
	give(p, fxChisel, 1)
	give(p, fxAmethyst, 1)
	c.handleAmethyst(itemOnItemEvent(p, fxChisel, fxAmethyst))
	session.menus[0].Execute(fxAmethystBoltTips, 1)
	if !session.hasMessage("You need level 83 Crafting to make amethyst bolt tips.") {
		t.Errorf("messages = %v, want the level-83 line", session.messages)
	}
	if c.sessionFor(p) != nil {
		t.Error("loop started for a too-low amethyst recipe, want none")
	}
}

func TestCutGemAnswerer(t *testing.T) {
	c, _, _ := newTestCrafting()
	request := &CutGemRequest{UncutID: fxUncutSapphire}
	c.answerCutGem(request)
	if request.CutID != fxSapphire || request.XP != 50 {
		t.Errorf("cut-gem answer = (%d, %v), want (%d, 50)", request.CutID, request.XP, fxSapphire)
	}
	miss := &CutGemRequest{UncutID: fxLeather}
	c.answerCutGem(miss)
	if miss.CutID != 0 || miss.XP != 0 {
		t.Errorf("cut-gem miss answer = (%d, %v), want (0, 0)", miss.CutID, miss.XP)
	}
}
