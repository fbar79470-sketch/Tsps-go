package crafting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func leatherEvent(p *game.Player, used, with int) *plugins.PluginItemOnItemEvent {
	return &plugins.PluginItemOnItemEvent{
		Player: p, UsedItemID: used, UsedWithItemID: with,
		UsedItem: game.NewItem(used, 1), UsedWithItem: game.NewItem(with, 1),
	}
}

func TestLeatherMenuOpens(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if menu.Title() != "How many do you wish to make?" {
		t.Errorf("menu title = %q, want the leather title", menu.Title())
	}
}

func TestLeatherNoThreadRefuses(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxNeedle, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	if !session.hasMessage("You need some thread to sew leather.") {
		t.Error("no-thread leather use did not send the thread message")
	}
	if len(session.menus) != 0 {
		t.Errorf("menus = %d, want 0", len(session.menus))
	}
}

func TestLeatherCostumeNeedleNeedsNoThread(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxCostumeNeedle, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxCostumeNeedle, fxLeather))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (costume needle needs no thread)", len(session.menus))
	}
}

func TestLeatherCraftsGlovesOnTick2(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	menu := session.menus[0]
	// Choose gloves, amount 2: the menu action runs the choice.
	menu.Execute(fxLeatherGloves, 2)
	if got := countOf(p, fxLeatherGloves); got != 0 {
		t.Fatalf("gloves before tick 2 = %d, want 0", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxLeatherGloves); got != 0 {
		t.Fatalf("gloves after tick 1 = %d, want 0", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxLeatherGloves); got != 1 {
		t.Fatalf("gloves after tick 2 = %d, want 1", got)
	}
	if !session.hasSpam("You make a pair of leather gloves.") {
		t.Error("missing 'You make a pair of leather gloves.' spam")
	}
	if !hasEvent(emitted, CraftingMadeEventName) {
		t.Error("crafting:made was not emitted")
	}
	// XP truncates: 13.8 -> 13.
	if got := p.Skills().Experience(game.SkillCrafting); got != 13 {
		t.Errorf("crafting XP = %d, want 13 (13.8 truncated)", got)
	}
}

func TestLeatherThreadReelLastsFive(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxLeather, 10)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	session.menus[0].Execute(fxLeatherGloves, 5)
	// 5 items land on ticks 2, 5, 8, 11, 14.
	driveCraftingTicks(c, 14)
	if got := countOf(p, fxLeatherGloves); got != 5 {
		t.Fatalf("gloves = %d, want 5", got)
	}
	if got := countOf(p, fxThread); got != 0 {
		t.Errorf("thread left = %d, want 0 (5th craft consumes the reel)", got)
	}
	if got := threadUses(p); got != 0 {
		t.Errorf("thread uses = %d, want 0 after the reel is used up", got)
	}
	if !session.hasSpam("You use up one of your reels of thread.") {
		t.Error("missing thread-used-up spam")
	}
}

func TestLeatherCostumeNeedleExemptFromThread(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxCostumeNeedle, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxCostumeNeedle, fxLeather))
	// No menu session needed: drive the craft directly.
	recipe := c.tables.LeatherByMaterial(fxLeather)[0]
	c.startLeatherCrafting(p, fxLeather, recipe, 3, fxCostumeNeedle)
	driveCraftingTicks(c, 8)
	if got := countOf(p, fxLeatherGloves); got != 3 {
		t.Fatalf("gloves = %d, want 3", got)
	}
	if got := threadUses(p); got != 0 {
		t.Errorf("thread uses = %d, want 0 (costume needle needs no thread)", got)
	}
}

func TestLeatherTooLowStatement(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	// Choose the coif (level 38) at level 1.
	menu := session.menus[0]
	menu.Execute(fxCoif, 1)
	text, ok := statementText(p)
	if !ok || text != "You need a Crafting level of 38 to make a coif." {
		t.Errorf("statement = %q, %v, want the level-38 coif line", text, ok)
	}
	if c.sessionFor(p) != nil {
		t.Error("loop started for a too-low recipe, want none")
	}
}

func TestLeatherMembersFilteredOnFreeWorld(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.membersWorld = false
	p, session := testPlayer("alice")
	setCrafting(p, 99)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxLeather, 5)
	c.handleNeedleOnLeather(leatherEvent(p, fxNeedle, fxLeather))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	// The menu must not offer the members coif; choosing it
	// directly must not start a loop either (recipes are
	// looked up from the unfiltered table, but the menu never
	// offers it — assert the offered items instead).
	for _, id := range session.menus[0].Items() {
		if id == fxCoif {
			t.Error("members coif offered on a free world")
		}
	}
}

func TestPouchCraftsOne(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 35)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxFoxFur, 4)
	c.handleNeedleOnFur(leatherEvent(p, fxNeedle, fxFoxFur))
	if len(session.menus) != 0 {
		t.Fatalf("menus = %d, want 0 (pouches have no menu)", len(session.menus))
	}
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxSmallMeatPouch); got != 1 {
		t.Fatalf("small meat pouch = %d, want 1", got)
	}
	if got := countOf(p, fxFoxFur); got != 0 {
		t.Errorf("fox fur left = %d, want 0", got)
	}
	if !session.hasSpam("You make a small meat pouch.") {
		t.Error("missing pouch spam")
	}
	if !hasEvent(emitted, CraftingMadeEventName) {
		t.Error("crafting:made was not emitted for the pouch")
	}
}

func TestPouchNoThreadStatement(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 35)
	give(p, fxNeedle, 1)
	give(p, fxFoxFur, 4)
	c.handleNeedleOnFur(leatherEvent(p, fxNeedle, fxFoxFur))
	text, ok := statementText(p)
	if !ok || text != "You'll need a needle and some thread to craft a small meat pouch." {
		t.Errorf("statement = %q, %v, want the pouch thread line", text, ok)
	}
}

func TestPouchUpgradeNeedsBase(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 65)
	give(p, fxNeedle, 1)
	give(p, fxThread, 1)
	give(p, fxAntelopeFur, 3)
	c.handleNeedleOnFur(leatherEvent(p, fxNeedle, fxAntelopeFur))
	id, text, ok := itemStatementText(p)
	if !ok || id != fxSmallMeatPouch || text != "You need a small meat pouch to upgrade into a large pouch." {
		t.Errorf("item statement = (%d, %q), %v, want the small-pouch upgrade line", id, text, ok)
	}
}

func TestCombineStudsBody(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 41)
	give(p, fxSteelStuds, 2)
	give(p, fxLeatherBody, 2)
	c.handleCombine(leatherEvent(p, fxSteelStuds, fxLeatherBody))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	session.menus[0].Execute(fxStuddedBody, 2)
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxStuddedBody); got != 1 {
		t.Fatalf("studded bodies after tick 2 = %d, want 1", got)
	}
	driveCraftingTicks(c, 3)
	if got := countOf(p, fxStuddedBody); got != 2 {
		t.Fatalf("studded bodies after tick 5 = %d, want 2", got)
	}
}

func TestCombineMissingMaterialNamesIt(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 41)
	give(p, fxHardLeather, 2)
	give(p, fxOakShield, 1)
	// No hammer, no bronze nails: the hammer is named first.
	c.handleCombine(leatherEvent(p, fxHardLeather, fxOakShield))
	if !session.hasMessage("You need a hammer to make this.") {
		t.Errorf("messages = %v, want the hammer line", session.messages)
	}
}

func TestCombineImcandoHammerSubstitutes(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 41)
	give(p, fxHardLeather, 2)
	give(p, fxOakShield, 1)
	give(p, fxBronzeNails, 15)
	give(p, fxImcandoHammer, 1)
	c.handleCombine(leatherEvent(p, fxHardLeather, fxOakShield))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (Imcando hammer substitutes)", len(session.menus))
	}
}

func TestCombineShieldNeedsFifteenNails(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 41)
	give(p, fxHardLeather, 2)
	give(p, fxOakShield, 1)
	give(p, fxHammer, 1)
	give(p, fxBronzeNails, 14)
	c.handleCombine(leatherEvent(p, fxHardLeather, fxOakShield))
	if !session.hasMessage("You need 15 bronze nails to make this.") {
		t.Errorf("messages = %v, want the 15-nails line", session.messages)
	}
}
