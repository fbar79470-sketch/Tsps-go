package crafting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func TestGlassblowingMenuMode0(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 33)
	give(p, fxGlassblowingPipe, 1)
	give(p, fxMoltenGlass, 3)
	c.blowGlass(itemOnItemEvent(p, fxGlassblowingPipe, fxMoltenGlass))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	menu := session.menus[0]
	if got := menu.Title(); got != "How many do you wish to make?" {
		t.Errorf("menu title = %q", got)
	}
	if got := menu.Options().Mode; got != 0 {
		t.Errorf("menu mode = %d, want 0", got)
	}
	if len(menu.Items()) != 2 {
		t.Errorf("menu items = %v, want both fixture recipes", menu.Items())
	}
}

func TestGlassblowingBeerGlassLoop(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxGlassblowingPipe, 1)
	give(p, fxMoltenGlass, 3)
	c.blowGlass(itemOnItemEvent(p, fxGlassblowingPipe, fxMoltenGlass))
	session.menus[0].Execute(fxBeerGlass, 2)
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxBeerGlass); got != 0 {
		t.Fatalf("beer glasses after tick 1 = %d, want 0", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxBeerGlass); got != 1 {
		t.Fatalf("beer glasses after tick 2 = %d, want 1", got)
	}
	if !session.hasSpam("You make a beer glass.") {
		t.Error("missing 'You make a beer glass.' spam")
	}
	// 17.5 XP truncates to 17.
	if got := p.Skills().Experience(game.SkillCrafting); got != 17 {
		t.Errorf("crafting XP = %d, want 17 (17.5 truncated)", got)
	}
}

func TestGlassblowingTooLowMessage(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxGlassblowingPipe, 1)
	give(p, fxMoltenGlass, 1)
	c.blowGlass(itemOnItemEvent(p, fxGlassblowingPipe, fxMoltenGlass))
	session.menus[0].Execute(fxVial, 1)
	if !session.hasMessage("You need level 33 Crafting to make a vial.") {
		t.Errorf("messages = %v, want the level-33 vial line", session.messages)
	}
	if c.sessionFor(p) != nil {
		t.Error("loop started for a too-low glass recipe, want none")
	}
}

func TestGlassblowingFreeWorldRefuses(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.membersWorld = false
	p, session := testPlayer("alice")
	setCrafting(p, 99)
	give(p, fxGlassblowingPipe, 1)
	give(p, fxMoltenGlass, 1)
	c.blowGlass(itemOnItemEvent(p, fxGlassblowingPipe, fxMoltenGlass))
	if len(session.menus) != 0 {
		t.Errorf("menus = %d, want 0 on a free world (glassblowing is members)", len(session.menus))
	}
}

func TestBattlestaffFirstTick3Interval2(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 54)
	give(p, fxBattlestaff, 3)
	give(p, fxWaterOrb, 3)
	c.attachOrb(itemOnItemEvent(p, fxWaterOrb, fxBattlestaff))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1", len(session.menus))
	}
	session.menus[0].Execute(fxWaterBattlestaff, 2)
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxWaterBattlestaff); got != 0 {
		t.Fatalf("water battlestaves after tick 2 = %d, want 0", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxWaterBattlestaff); got != 1 {
		t.Fatalf("water battlestaves after tick 3 = %d, want 1", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxWaterBattlestaff); got != 1 {
		t.Fatalf("water battlestaves after tick 4 = %d, want 1 (every 2 ticks)", got)
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxWaterBattlestaff); got != 2 {
		t.Fatalf("water battlestaves after tick 5 = %d, want 2", got)
	}
}

func TestBattlestaffTooLowMessage(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 53)
	give(p, fxBattlestaff, 1)
	give(p, fxWaterOrb, 1)
	c.attachOrb(itemOnItemEvent(p, fxWaterOrb, fxBattlestaff))
	session.menus[0].Execute(fxWaterBattlestaff, 1)
	if !session.hasMessage("You need level 54 Crafting to make a water battlestaff.") {
		t.Errorf("messages = %v, want the level-54 line", session.messages)
	}
}

func TestSnelmOneShellNoMenu(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 15)
	give(p, fxChisel, 1)
	give(p, fxMyreShell, 1)
	c.chiselOnShell(itemOnItemEvent(p, fxChisel, fxMyreShell))
	if len(session.menus) != 0 {
		t.Fatalf("menus = %d, want 0 (one shell needs no menu)", len(session.menus))
	}
	driveCraftingTicks(c, 1)
	if got := countOf(p, fxMyreSnelm); got != 1 {
		t.Fatalf("myre snelms after tick 1 = %d, want 1", got)
	}
	if !session.hasSpam("You make a myre snelm.") {
		t.Error("missing snelm spam")
	}
	// 32.5 XP truncates to 32.
	if got := p.Skills().Experience(game.SkillCrafting); got != 32 {
		t.Errorf("crafting XP = %d, want 32 (32.5 truncated)", got)
	}
	if !hasEvent(emitted, CraftingMadeEventName) {
		t.Error("crafting:made was not emitted")
	}
}

func TestSnelmTooLowNextTickItemBox(t *testing.T) {
	c, _, tm := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 14)
	give(p, fxChisel, 1)
	give(p, fxMyreShell, 1)
	c.chiselOnShell(itemOnItemEvent(p, fxChisel, fxMyreShell))
	if _, _, ok := itemStatementText(p); ok {
		t.Fatal("item box shown before the next tick")
	}
	driveTaskTicks(tm, 1)
	id, text, ok := itemStatementText(p)
	if !ok || id != fxMyreShell || text != "You need a Crafting level of 15 to craft this." {
		t.Errorf("item statement = (%d, %q), %v, want the level-15 snelm line", id, text, ok)
	}
}

func TestSnelmSeveralShellsMenu(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 15)
	give(p, fxChisel, 1)
	give(p, fxMyreShell, 3)
	c.chiselOnShell(itemOnItemEvent(p, fxChisel, fxMyreShell))
	if len(session.menus) != 1 {
		t.Fatalf("menus = %d, want 1 (several shells get a menu)", len(session.menus))
	}
	if got := session.menus[0].Options().Mode; got != 0 {
		t.Errorf("menu mode = %d, want 0", got)
	}
	session.menus[0].Execute(fxMyreShell, 2)
	driveCraftingTicks(c, 3)
	if got := countOf(p, fxMyreSnelm); got != 2 {
		t.Fatalf("myre snelms after tick 3 = %d, want 2", got)
	}
}

func TestLimestoneCarvesInPlace(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 12)
	give(p, fxLimestone, 1)
	give(p, fxChisel, 1)
	event := itemOnItemEvent(p, fxChisel, fxLimestone)
	event.UsedItemSlot, event.UsedWithItemSlot = 1, 0
	c.carveLimestone(event)
	if !event.Handled {
		t.Error("chisel on limestone was not handled")
	}
	if got := countOf(p, fxLimestoneBrick); got != 1 {
		t.Fatalf("limestone bricks = %d, want 1", got)
	}
	if got := countOf(p, fxLimestone); got != 0 {
		t.Errorf("limestone left = %d, want 0", got)
	}
	// The brick replaces the limestone's slot, in place.
	if got := p.Inventory().Items()[0].ID(); got != fxLimestoneBrick {
		t.Errorf("slot 0 = %d, want the brick (in-place transform)", got)
	}
	if !session.hasSpam("You use the chisel on the limestone and carve it into a building block.") {
		t.Error("missing limestone carve spam")
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 6 {
		t.Errorf("crafting XP = %d, want 6", got)
	}
}

func TestLimestoneTooLow(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 11)
	give(p, fxLimestone, 1)
	give(p, fxChisel, 1)
	event := itemOnItemEvent(p, fxChisel, fxLimestone)
	event.UsedItemSlot, event.UsedWithItemSlot = 1, 0
	c.carveLimestone(event)
	if !session.hasMessage("You need a Crafting level of 12 to carve limestone.") {
		t.Errorf("messages = %v, want the level-12 line", session.messages)
	}
	if got := countOf(p, fxLimestoneBrick); got != 0 {
		t.Errorf("limestone bricks = %d, want 0", got)
	}
}

func TestLightOrbImmediate(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 87)
	give(p, fxEmptyLightOrb, 1)
	give(p, fxCaveGoblinWire, 1)
	event := itemOnItemEvent(p, fxEmptyLightOrb, fxCaveGoblinWire)
	c.makeLightOrb(event)
	if !event.Handled {
		t.Error("empty light orb on wire was not handled")
	}
	if got := countOf(p, fxLightOrb); got != 1 {
		t.Fatalf("light orbs = %d, want 1 (immediate, no loop)", got)
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 104 {
		t.Errorf("crafting XP = %d, want 104", got)
	}
	if !hasEvent(emitted, LightOrbMadeEventName) {
		t.Error("crafting:light-orb-made was not emitted")
	}
}

func TestLightOrbTooLow(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 86)
	give(p, fxEmptyLightOrb, 1)
	give(p, fxCaveGoblinWire, 1)
	c.makeLightOrb(itemOnItemEvent(p, fxEmptyLightOrb, fxCaveGoblinWire))
	if !session.hasMessage("You need a Crafting level of 87 to do that.") {
		t.Errorf("messages = %v, want the level-87 line", session.messages)
	}
	if got := countOf(p, fxLightOrb); got != 0 {
		t.Errorf("light orbs = %d, want 0", got)
	}
}

func TestButtonsPolishInPlace(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 3)
	give(p, fxButtons, 1)
	if !c.polish(&plugins.PluginItemActionEvent{Player: p, ItemID: fxButtons, Slot: 0, Option: "Polish"}) {
		t.Fatal("Polish on buttons returned false")
	}
	if got := p.Inventory().Items()[0].ID(); got != fxPolishedButtons {
		t.Errorf("slot 0 = %d, want polished buttons (in-place transform)", got)
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 5 {
		t.Errorf("crafting XP = %d, want 5", got)
	}
	if !session.hasSpam("You rub the buttons on your clothes and they become more shiny.") {
		t.Error("missing polish spam")
	}
}

func TestButtonsTooLowSpam(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 2)
	give(p, fxButtons, 1)
	if !c.polish(&plugins.PluginItemActionEvent{Player: p, ItemID: fxButtons, Slot: 0, Option: "Polish"}) {
		t.Fatal("Polish on buttons returned false below level")
	}
	if got := countOf(p, fxPolishedButtons); got != 0 {
		t.Errorf("polished buttons = %d, want 0", got)
	}
	if !session.hasSpam("You rub the buttons on your clothes but they aren't improved by the process.") {
		t.Error("missing too-low polish spam")
	}
}

func TestGemSickleFailClosed(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 99)
	give(p, fxEmerald, 1)
	give(p, fxSilverSickleB, 1)
	give(p, fxChisel, 1)
	event := itemOnItemEvent(p, fxEmerald, fxSilverSickleB)
	c.useGemOnSickle(event)
	// No crafting:gem-sickle-allowed answerer: the use does
	// nothing and stays unhandled.
	if event.Handled {
		t.Error("gem on sickle handled with no allowed answerer, want fail-closed")
	}
	if got := countOf(p, fxEmeraldSickleB); got != 0 {
		t.Errorf("emerald sickles = %d, want 0", got)
	}
}

func TestGemSickleAllowedPromptsAndMakes(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	// A stub "quest" plugin answers the allowed gate: the
	// shape a future real answerer takes.
	answerer := plugins.Plugin{
		Name: "StubQuests",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent(GemSickleAllowedEventName, func(payload any) {
				if request, ok := payload.(*GemSickleAllowedRequest); ok {
					request.Allowed = true
				}
			})
		},
	}
	hub := plugins.NewHub([]plugins.Plugin{c.plugin(), answerer}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	prompts := game.NewMultiChatboxPrompt(nil)
	hub.SetMultiChatboxPrompt(prompts)
	// Hub load replaced the capturing emit with the hub's;
	// re-wrap it so the test still sees crafting:gem-sickle-
	// made while the gate request still reaches the answerer.
	forward := c.emit
	c.emit = func(name string, payload any) {
		*emitted = append(*emitted, emittedEvent{name: name, payload: payload})
		forward(name, payload)
	}

	p, _ := testPlayer("alice")
	setCrafting(p, 99)
	give(p, fxEmerald, 1)
	give(p, fxSilverSickleB, 1)
	give(p, fxChisel, 1)
	event := itemOnItemEvent(p, fxEmerald, fxSilverSickleB)
	c.useGemOnSickle(event)
	if !event.Handled {
		t.Fatal("gem on sickle not handled with the gate allowed")
	}
	title, options, ok := prompts.GetPending(p)
	if !ok {
		t.Fatal("no Yes/No prompt pending for the gem sickle")
	}
	if title != "Craft an emerald sickle?" {
		t.Errorf("prompt title = %q", title)
	}
	if len(options) != 2 || options[0] != "Yes." || options[1] != "No." {
		t.Errorf("prompt options = %v, want Yes. / No.", options)
	}
	// Choose Yes (option 1).
	if !prompts.HandleInterfaceActionClick(p, game.OptionsWidgetID, 1, 219) {
		t.Fatal("prompt click did not consume the Yes option")
	}
	if got := countOf(p, fxEmeraldSickleB); got != 1 {
		t.Fatalf("emerald sickles = %d, want 1", got)
	}
	if got := countOf(p, fxEmerald); got != 0 {
		t.Errorf("emeralds left = %d, want 0", got)
	}
	if got := countOf(p, fxSilverSickleB); got != 0 {
		t.Errorf("silver sickles left = %d, want 0", got)
	}
	if got := p.Skills().Experience(game.SkillCrafting); got != 20 {
		t.Errorf("crafting XP = %d, want 20", got)
	}
	if !hasEvent(emitted, GemSickleMadeEventName) {
		t.Error("crafting:gem-sickle-made was not emitted")
	}
	id, text, ok := itemStatementText(p)
	if !ok || id != fxEmeraldSickleB || text != "You embed the emerald into the sickle." {
		t.Errorf("item statement = (%d, %q), %v, want the embed line", id, text, ok)
	}
}

func TestGemSickleNoChiselMessage(t *testing.T) {
	c, _, _ := newTestCrafting()
	answerer := plugins.Plugin{
		Name: "StubQuests",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent(GemSickleAllowedEventName, func(payload any) {
				if request, ok := payload.(*GemSickleAllowedRequest); ok {
					request.Allowed = true
				}
			})
		},
	}
	hub := plugins.NewHub([]plugins.Plugin{c.plugin(), answerer}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	hub.SetMultiChatboxPrompt(game.NewMultiChatboxPrompt(nil))

	p, session := testPlayer("alice")
	setCrafting(p, 99)
	give(p, fxEmerald, 1)
	give(p, fxSilverSickleB, 1)
	// No chisel carried.
	event := itemOnItemEvent(p, fxEmerald, fxSilverSickleB)
	c.useGemOnSickle(event)
	if !event.Handled {
		t.Error("gem on sickle not handled (gate allowed, no chisel)")
	}
	if !session.hasMessage("You need a chisel to embed the emerald into the sickle.") {
		t.Errorf("messages = %v, want the chisel line", session.messages)
	}
}
