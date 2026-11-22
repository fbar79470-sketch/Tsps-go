package crafting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func furnaceObject(p *game.Player) *game.GameObject {
	return testGameObject(fxFurnaceObj, p.Location())
}

func TestFurnaceSmeltOpensGoldMenu(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.objectNameResolver = func(id int) string {
		if id == fxFurnaceObj {
			return "Furnace"
		}
		return ""
	}
	p, session := testPlayer("alice")
	setCrafting(p, 5)
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	handled := c.smeltAtFurnace(&plugins.ObjectInteractionEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj,
		Definition: testObjectDef{id: fxFurnaceObj, name: "Furnace", interactions: []string{"Smelt"}},
		ClickType:  1,
	})
	if !handled {
		t.Fatal("smeltAtFurnace = false with gold bar + mould, want true")
	}
	if p.GetInterfaceId() != 446 {
		t.Errorf("interface id = %d, want 446 (gold)", p.GetInterfaceId())
	}
	if len(session.mounts) != 1 || session.mounts[0].targetUID != (161<<16)|16 || session.mounts[0].groupID != 446 {
		t.Errorf("mounts = %v, want one mount of 446 at (161<<16)|16", session.mounts)
	}
	if got := session.varps[2224]; got != 1 {
		t.Errorf("varp 2224 = %d, want 1", got)
	}
	if menuOf(p) == nil || menuOf(p).amount != 1 {
		t.Error("jewellery menu attribute missing or amount != 1")
	}
}

func TestFurnaceSmeltFallsThroughWithoutMould(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 5)
	give(p, fxGoldBar, 1)
	// No mould: Crafting must leave the interaction unhandled
	// so Smithing's bars menu can open.
	handled := c.smeltAtFurnace(&plugins.ObjectInteractionEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj,
		Definition: testObjectDef{id: fxFurnaceObj, name: "Furnace", interactions: []string{"Smelt"}},
		ClickType:  1,
	})
	if handled {
		t.Error("smeltAtFurnace = true with no mould, want false (fall through to Smithing)")
	}
	if p.GetInterfaceId() != -1 {
		t.Errorf("interface id = %d, want -1 (no menu opened)", p.GetInterfaceId())
	}
}

func TestFurnaceFallThroughAtHub(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.objectNameResolver = func(id int) string {
		if id == fxFurnaceObj {
			return "Furnace"
		}
		return ""
	}
	// A stub "Smithing" plugin whose Smelt handler records
	// whether it ran: it must run when Crafting falls through,
	// and must not run when Crafting claims the furnace.
	smithingRan := false
	stub := plugins.Plugin{
		Name: "StubSmithing",
		Register: func(api *plugins.Api) {
			api.OnObjectInteractionNamed("Furnace", map[string]plugins.ObjectInteractionHandler{
				"Smelt": func(*plugins.ObjectInteractionEvent) bool {
					smithingRan = true
					return true
				},
			})
		},
	}
	hub := plugins.NewHub([]plugins.Plugin{c.plugin(), stub}, plugins.HubOptions{MembersWorld: true})
	hub.Load()

	p, _ := testPlayer("alice")
	setCrafting(p, 5)
	give(p, fxGoldBar, 1)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj,
		Definition: testObjectDef{id: fxFurnaceObj, name: "Furnace", interactions: []string{"Smelt"}},
		ClickType:  1,
	}
	if !hub.EmitObjectInteraction(event) {
		t.Fatal("EmitObjectInteraction = false with no mould, want the stub to claim it")
	}
	if !smithingRan {
		t.Error("stub Smithing Smelt did not run after Crafting fell through")
	}

	// With a mould, Crafting claims the furnace and the stub
	// must not run.
	smithingRan = false
	p2, _ := testPlayer("bob")
	setCrafting(p2, 5)
	give(p2, fxGoldBar, 1)
	give(p2, fxRingMould, 1)
	event2 := &plugins.ObjectInteractionEvent{
		Player: p2, Object: furnaceObject(p2), ObjectID: fxFurnaceObj,
		Definition: testObjectDef{id: fxFurnaceObj, name: "Furnace", interactions: []string{"Smelt"}},
		ClickType:  1,
	}
	if !hub.EmitObjectInteraction(event2) {
		t.Fatal("EmitObjectInteraction = false with gold bar + mould, want Crafting to claim it")
	}
	if smithingRan {
		t.Error("stub Smithing Smelt ran after Crafting claimed the furnace")
	}
	if p2.GetInterfaceId() != 446 {
		t.Errorf("interface id = %d, want 446", p2.GetInterfaceId())
	}
}

func TestBarOnFurnaceOpensMenu(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.objectNameResolver = func(id int) string {
		if id == fxFurnaceObj {
			return "Furnace"
		}
		return ""
	}
	p, session := testPlayer("alice")
	setCrafting(p, 5)
	// No mould carried: a bar on the furnace still opens the
	// menu for it (TS opens the cache menu directly).
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj, ItemID: fxSilverBar,
	}
	c.handleBarOnFurnace(event)
	if !event.Handled {
		t.Error("silver bar on furnace was not handled")
	}
	if p.GetInterfaceId() != 6 {
		t.Errorf("interface id = %d, want 6 (silver)", p.GetInterfaceId())
	}
	if len(session.mounts) != 1 || session.mounts[0].groupID != 6 {
		t.Errorf("mounts = %v, want one mount of 6", session.mounts)
	}
}

func TestJewelleryGoldRingLoop(t *testing.T) {
	c, emitted, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 5)
	give(p, fxGoldBar, 3)
	give(p, fxRingMould, 1)
	c.openFurnaceMenu(p, goldCraftingInterface)
	// Quantity 1 (component 60), then the gold ring (component 8).
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 60, Action: 1,
	})
	if got := session.varps[2224]; got != 1 {
		t.Errorf("varp 2224 after quantity 1 = %d, want 1", got)
	}
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 8, Action: 1,
	})
	if p.GetInterfaceId() != -1 {
		t.Errorf("interface id = %d, want -1 (menu closed on choice)", p.GetInterfaceId())
	}
	if len(session.closed) != 1 || session.closed[0] != (161<<16)|16 {
		t.Errorf("closed = %v, want the main modal closed", session.closed)
	}
	// Last-item varbit: enum 4318 maps (446,8) -> 1.
	if got := session.varbits[13892]; got != 1 {
		t.Errorf("gold last-item varbit = %d, want 1", got)
	}
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxGoldRing); got != 1 {
		t.Fatalf("gold rings after tick 2 = %d, want 1", got)
	}
	if got := countOf(p, fxGoldBar); got != 2 {
		t.Errorf("gold bars left = %d, want 2", got)
	}
	if !hasEvent(emitted, CraftingMadeEventName) {
		t.Error("crafting:made was not emitted")
	}
}

func TestJewelleryGemRingNeedsGem(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 20)
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	// No sapphire carried: the sapphire-ring component (9)
	// must not start a loop.
	c.openFurnaceMenu(p, goldCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 9, Action: 1,
	})
	if c.sessionFor(p) != nil {
		t.Error("sapphire-ring loop started without a sapphire, want none")
	}
	if got := countOf(p, fxGoldBar); got != 1 {
		t.Errorf("gold bars = %d, want 1 (nothing consumed)", got)
	}
}

func TestJewellerySingleRingFitsFullInventory(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, _ := testPlayer("alice")
	setCrafting(p, 5)
	// 26 junk + bar + mould = 28 slots. One ring frees the
	// bar's slot and stacks into it, so it fits.
	inv := p.Inventory()
	for i := 0; i < 26; i++ {
		inv.AddItem(game.NewItem(9000+i, 1))
	}
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	product := furnaceProduct{itemID: fxGoldRing, level: 5, xp: 15, bar: fxGoldBar, mouldID: fxRingMould, amount: 1}
	if !c.craftFurnaceProduct(p, product) {
		t.Error("craftFurnaceProduct refused a 1-ring batch with a freeing slot, want it made")
	}
	if got := countOf(p, fxGoldRing); got != 1 {
		t.Errorf("gold rings = %d, want 1", got)
	}
}

func TestJewelleryPreviewRefusesOversizedBatch(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 5)
	// 26 junk + silver bar + mould = 28 slots. Silver bolts
	// come 10 at a time and do not stack: after the bar frees
	// one slot the preview can only add 1, so the batch
	// refuses and nothing is consumed.
	inv := p.Inventory()
	for i := 0; i < 26; i++ {
		inv.AddItem(game.NewItem(9000+i, 1))
	}
	give(p, fxSilverBar, 1)
	give(p, fxRingMould, 1)
	c.openFurnaceMenu(p, silverCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (6 << 16) | 27, Action: 1,
	})
	if c.sessionFor(p) == nil {
		t.Fatal("silver-bolts loop did not start")
	}
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxSilverBolts); got != 0 {
		t.Errorf("silver bolts = %d, want 0 (batch refused)", got)
	}
	if got := countOf(p, fxSilverBar); got != 1 {
		t.Errorf("silver bars = %d, want 1 (nothing consumed)", got)
	}
	if !session.hasMessage("Not enough space in your inventory.") {
		t.Errorf("messages = %v, want the inventory-full line", session.messages)
	}
	if c.sessionFor(p) != nil {
		t.Error("loop still running after the preview refusal, want it stopped")
	}
}

func TestJewelleryQuantityXAsksAmount(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 5)
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	c.openFurnaceMenu(p, goldCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 63, Action: 1,
	})
	if p.GetEnteredAmountAction() == nil {
		t.Fatal("no entered-amount action armed for the x button")
	}
	if len(session.packets) == 0 {
		t.Fatal("no enter-amount prompt packet sent for the x button")
	}
	// Enter 5: the remembered quantity becomes 5.
	p.GetEnteredAmountAction().Execute(5)
	if got := session.varps[2224]; got != 5 {
		t.Errorf("varp 2224 after entering 5 = %d, want 5", got)
	}
	if got := menuOf(p).amount; got != 5 {
		t.Errorf("menu amount = %d, want 5", got)
	}
}

func TestJewelleryTooLowLevel(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 4)
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	c.openFurnaceMenu(p, goldCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 8, Action: 1,
	})
	if !session.hasMessage("You need a Crafting level of at least 5 to craft this.") {
		t.Errorf("messages = %v, want the level-5 line", session.messages)
	}
	if c.sessionFor(p) != nil {
		t.Error("loop started below level, want none")
	}
}

func TestSlayerRingLockedWithoutVarbit(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 75)
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	give(p, fxEnchantedGem, 1)
	c.openFurnaceMenu(p, goldCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 16, Action: 1,
	})
	if !session.hasMessage(slayerRingLocked) {
		t.Errorf("messages = %v, want the slayer-ring lock line", session.messages)
	}
	if c.sessionFor(p) != nil {
		t.Error("slayer-ring loop started without the unlock, want none")
	}
}

func TestSlayerRingEternalPreferred(t *testing.T) {
	c, _, _ := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 75)
	session.varbits[3207] = 1 // slayer_ring_unlocked
	give(p, fxGoldBar, 1)
	give(p, fxRingMould, 1)
	give(p, fxEternalGem, 1)
	give(p, fxEnchantedGem, 1)
	c.openFurnaceMenu(p, goldCraftingInterface)
	c.handleJewelleryButton(&plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (446 << 16) | 16, Action: 1,
	})
	driveCraftingTicks(c, 2)
	if got := countOf(p, fxSlayerRingEternal); got != 1 {
		t.Fatalf("eternal slayer rings = %d, want 1 (eternal preferred)", got)
	}
	if got := countOf(p, fxSlayerRing8); got != 0 {
		t.Errorf("enchanted slayer rings = %d, want 0", got)
	}
}

func TestPerfectJewellerySingleMould(t *testing.T) {
	c, _, _ := newTestCrafting()
	c.objectNameResolver = func(id int) string {
		if id == fxFurnaceObj {
			return "Furnace"
		}
		return ""
	}
	p, session := testPlayer("alice")
	setCrafting(p, 40)
	give(p, fxPerfectGoldBar, 1)
	give(p, fxRuby, 1)
	give(p, fxRingMould, 1)
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj, ItemID: fxPerfectGoldBar,
	}
	c.handlePerfectJewellery(event)
	if !event.Handled {
		t.Error("perfect bar on furnace was not handled")
	}
	if got := countOf(p, fxPerfectRing); got != 1 {
		t.Fatalf("perfect rings = %d, want 1", got)
	}
	if !session.hasMessage("You craft a 'perfect' ring.") {
		t.Errorf("messages = %v, want the perfect-ring line", session.messages)
	}
}

func TestPerfectJewelleryTwoMouldsPrompts(t *testing.T) {
	c, _, _ := newTestCrafting()
	hub := hubWith(t, c)
	prompts := game.NewMultiChatboxPrompt(nil)
	hub.SetMultiChatboxPrompt(prompts)
	c.objectNameResolver = func(id int) string {
		if id == fxFurnaceObj {
			return "Furnace"
		}
		return ""
	}
	p, _ := testPlayer("alice")
	setCrafting(p, 40)
	give(p, fxPerfectGoldBar, 1)
	give(p, fxRuby, 1)
	give(p, fxRingMould, 1)
	give(p, fxNecklaceMould, 1)
	event := &plugins.ItemOnObjectEvent{
		Player: p, Object: furnaceObject(p), ObjectID: fxFurnaceObj, ItemID: fxPerfectGoldBar,
	}
	c.handlePerfectJewellery(event)
	if !event.Handled {
		t.Error("perfect bar on furnace was not handled")
	}
	title, options, ok := prompts.GetPending(p)
	if !ok {
		t.Fatal("no multi-chatbox prompt pending for two moulds")
	}
	if title != "What would you like to make?" {
		t.Errorf("prompt title = %q", title)
	}
	if len(options) != 2 || options[0] != "'perfect' ring" || options[1] != "'perfect' necklace" {
		t.Errorf("prompt options = %v, want the ring and necklace", options)
	}
	// Choose the ring (option 0): the prompt's click path
	// runs the callback through the registry.
	if !prompts.HandleInterfaceActionClick(p, game.OptionsWidgetID, 1, 219) {
		t.Fatal("prompt click did not consume the option")
	}
	if got := countOf(p, fxPerfectRing); got != 1 {
		t.Fatalf("perfect rings after choosing = %d, want 1", got)
	}
}

func TestStringAmuletNextTick(t *testing.T) {
	c, _, tm := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxBallOfWool, 1)
	give(p, fxGoldAmuletU, 1)
	event := itemOnItemEvent(p, fxBallOfWool, fxGoldAmuletU)
	c.stringAmulet(event)
	if !event.Handled {
		t.Error("ball of wool on unstrung amulet was not handled")
	}
	if got := countOf(p, fxGoldAmulet); got != 0 {
		t.Fatalf("gold amulet before the next tick = %d, want 0", got)
	}
	driveTaskTicks(tm, 1)
	if got := countOf(p, fxGoldAmulet); got != 1 {
		t.Fatalf("gold amulet after the next tick = %d, want 1", got)
	}
	if !session.hasSpam("You put some string on your amulet.") {
		t.Error("missing stringing spam")
	}
}

func TestStringAmuletSymbolNoun(t *testing.T) {
	c, _, tm := newTestCrafting()
	p, session := testPlayer("alice")
	setCrafting(p, 1)
	give(p, fxBallOfWool, 1)
	give(p, fxUnstrungSymbol, 1)
	c.stringAmulet(itemOnItemEvent(p, fxUnstrungSymbol, fxBallOfWool))
	driveTaskTicks(tm, 1)
	if got := countOf(p, fxUnblessedSymbol); got != 1 {
		t.Fatalf("unblessed symbols = %d, want 1", got)
	}
	if !session.hasSpam("You put some string on your symbol.") {
		t.Error("missing symbol stringing spam")
	}
}
