// Jewellery.Crafting.js (344 lines, commit bfd81e3): gold and
// silver jewellery at a furnace, Family Crest's 'perfect'
// jewellery, and stringing amulets.
//
// The furnace's Smelt option opens crafting_gold (446) or
// silver_crafting (6) instead of the bars menu when the player
// carries the bar and a mould for it; using the bar on the
// furnace does the same. The menus are client-drawn (cache
// scripts); the server only opens the sub-interface, sets varp
// 2224 (makexcrafting, the remembered quantity) and, on a
// product click, the last-item varbit (enums 4318/4319). The
// "x" quantity asks "How many would you like to make?" (cap
// 28). Choosing a product closes the menu, plays anim 899 +
// sound 2725, first item 2 ticks later, then one every 3
// ticks, no message.
//
// The two-mould 'perfect' choice and the gem-sickle choice
// both use Api.SendMultiChatboxPrompt: the seam EXISTS in Go
// (plugins/dialogue.go, installed by cmd/server at boot — the
// plan's gap 27e premise was outdated; Smithing's gap 26h
// predates it), so both prompts are ported, not deferred.
//
// One seam does NOT exist: Api has no EmitCanUseItem
// passthrough (Hub.EmitCanUseItem only), so
// craftFurnaceProduct skips TS's emitCanUseItem "craft" check.
// No Go content registers an onCanUseItem answerer, so the
// check could never refuse; the skip is behavior-preserving.
package crafting

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
	"tsps-go/internal/session"
)

// ---- constants (Jewellery.Crafting.js) ----

const (
	// goldCraftingInterface is crafting_gold (446); silver
	// is silver_crafting (6).
	goldCraftingInterface   = 446
	silverCraftingInterface = 6
	// mainModalTargetUID is the main modal target (161<<16)|16.
	mainModalTargetUID = (161 << 16) | 16
	// makeQuantityVarp is makexcrafting (2224), the remembered
	// quantity.
	makeQuantityVarp = 2224
	// jewelleryMenuAttribute is the player attribute holding
	// the open jewellery menu (its amount and station).
	jewelleryMenuAttribute = "crafting:jewellery-menu"
	// furnaceAnimationSeqName is human_furnace (899).
	furnaceAnimationSeqName = "human_furnace"
	// furnaceSoundID is the smelting sound (2725).
	furnaceSoundID = 2725
	// slayerRingLocked is the line for a slayer ring without
	// Ring bling (ours: the Wiki gives no message).
	slayerRingLocked = "You need to unlock the ability to craft slayer rings from a Slayer master."
)

// furnaceNames are the furnace object names (Furnace, Small
// furnace).
var furnaceNames = map[string]bool{"Furnace": true, "Small furnace": true}

// jewelleryPacketSender is the session-side sender surface
// Jewellery needs (the smithing anvilPacketSender precedent):
// the varbit send and the sub-interface mount/close.
type jewelleryPacketSender interface {
	SendVarbit(id, value int)
	MountSubInterface(targetUID, groupID, typ int) *session.SubInterfaceMount
	CloseSubInterface(targetUID int)
}

// jewellerySender returns the player's jewelleryPacketSender,
// or nil when the player has no session (or a session without
// the surface).
func jewellerySender(player *game.Player) jewelleryPacketSender {
	if player == nil || player.Session == nil {
		return nil
	}
	sender, ok := player.Session.(jewelleryPacketSender)
	if !ok {
		return nil
	}
	return sender
}

// furnaceProduct is one furnace product: `bar` plus `extra`
// used up, `mouldID` kept, `amount` made.
type furnaceProduct struct {
	itemID  int
	level   int
	xp      float64
	bar     int
	mouldID int
	extra   []int
	amount  int
}

// jewelleryMenu is the open jewellery menu's state: the
// chosen quantity and the furnace tile it opened at.
type jewelleryMenu struct {
	amount   int
	location game.Location
}

// menuOf returns the player's open jewellery menu, or nil.
func menuOf(player *game.Player) *jewelleryMenu {
	if player == nil {
		return nil
	}
	menu, _ := player.GetAttribute(jewelleryMenuAttribute).(*jewelleryMenu)
	return menu
}

// isFurnace reports whether an object id is a furnace, by
// its resolved name.
func (c *crafting) isFurnace(objectID int) bool {
	return furnaceNames[c.objectNameOf(objectID)]
}

// hasFurnaceMaterials reports whether the player holds a
// furnace product's bar, mould and extras.
func hasFurnaceMaterials(player *game.Player, product furnaceProduct) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	if !inv.Contains(product.bar) || !inv.Contains(product.mouldID) {
		return false
	}
	for _, id := range product.extra {
		if !inv.Contains(id) {
			return false
		}
	}
	return true
}

// craftFurnaceProduct makes one furnace product
// (Jewellery.Crafting.js craftFurnaceProduct).
func (c *crafting) craftFurnaceProduct(player *game.Player, product furnaceProduct) bool {
	if c.craftingLevel(player) < product.level {
		player.SendMessage(fmt.Sprintf("You need a Crafting level of at least %d to craft this.", product.level))
		return false
	}
	if !hasFurnaceMaterials(player, product) {
		return false
	}
	// No Api EmitCanUseItem passthrough exists (see the file
	// header); no Go answerer could refuse, so the TS
	// emitCanUseItem "craft" check is skipped.
	inv := player.Inventory()
	output := game.NewItem(product.itemID, product.amount)
	// Check the whole batch after the inputs have freed their
	// slots (the smithing canGrantAfterConsumption preview).
	preview := game.NewInventory(player)
	preview.SetItems(inv.Items())
	preview.Deleted(product.bar, 1, false)
	for _, id := range product.extra {
		preview.Deleted(id, 1, false)
	}
	if preview.GetAddableAmount(output) < product.amount {
		inv.Full()
		return false
	}
	inv.DeleteNumber(product.bar, 1)
	for _, id := range product.extra {
		inv.DeleteNumber(id, 1)
	}
	inv.AddItem(output)
	c.addCraftingXp(player, product.xp)
	c.emitEvent(CraftingMadeEventName, &CraftingMadeEvent{Player: player, ItemID: product.itemID, Furnace: true})
	materials := [][2]int{{product.bar, 1}}
	for _, id := range product.extra {
		materials = append(materials, [2]int{id, 1})
	}
	c.produced(player, product.itemID, product.amount, product.xp, "jewellery", materials)
	return true
}

// storeLastItem stores the chosen button as the menu's last
// item, so it opens highlighted next time (enums 4318/4319,
// varbits crafting_gold_item_lasttype /
// crafting_silver_item_lasttype).
func (c *crafting) storeLastItem(player *game.Player, buttonID int) {
	if c.tables == nil {
		return
	}
	group := buttonID >> 16
	var enum map[int]int
	var varbitName string
	switch group {
	case goldCraftingInterface:
		enum, varbitName = c.tables.GoldLastType, "crafting_gold_item_lasttype"
	case silverCraftingInterface:
		enum, varbitName = c.tables.SilverLastType, "crafting_silver_item_lasttype"
	default:
		return
	}
	value, ok := enum[buttonID]
	if !ok {
		return
	}
	varbitID, ok := c.tables.VarbitIDs[varbitName]
	if !ok {
		return
	}
	sendVarbit(player, varbitID, value)
}

// startFurnaceProduct starts the furnace loop for a chosen
// product (Jewellery.Crafting.js startFurnaceProduct). The
// cache's menu only gives a product its Craft option when the
// player has the level and materials, so the guards here are
// a refusal before the menu closes or anything animates.
func (c *crafting) startFurnaceProduct(player *game.Player, menu *jewelleryMenu, product furnaceProduct, buttonID int) {
	if c.craftingLevel(player) < product.level {
		player.SendMessage(fmt.Sprintf("You need a Crafting level of at least %d to craft this.", product.level))
		return
	}
	if !hasFurnaceMaterials(player, product) {
		return
	}
	c.storeLastItem(player, buttonID)
	player.SetAttribute(jewelleryMenuAttribute, nil)
	player.SetInterfaceId(-1)
	if sender := jewellerySender(player); sender != nil {
		sender.CloseSubInterface(mainModalTargetUID)
	}
	location := menu.location
	c.startMaking(player, MakeOptions{
		Amount: menu.amount, StartTick: intPtr(0), FirstTick: 2, Interval: 3,
		Animation: c.seqID(furnaceAnimationSeqName), Sound: game.NewSound(furnaceSoundID, 1, 0, 0),
		Kind: "jewellery", Anchor: &location, ResetAnimation: true,
		Has:  func() bool { return hasFurnaceMaterials(player, product) },
		Make: func() bool { return c.craftFurnaceProduct(player, product) },
	})
}

func intPtr(n int) *int { return &n }

// goldProduct is the gold product for a menu component: the
// slayer ring on component 16, else the type's product at the
// component's gem slot. The gem-set product is made when the
// matching cut gem is carried (it is the product's extra, so
// hasFurnaceMaterials refuses without it); the plain piece is
// the plain slot's own component.
func (c *crafting) goldProduct(player *game.Player, component int) *furnaceProduct {
	if c.tables == nil {
		return nil
	}
	if component == c.tables.Gold.SlayerRing.Component {
		return c.slayerRingProduct(player)
	}
	for i := range c.tables.Gold.Types {
		typ := &c.tables.Gold.Types[i]
		for gemIndex, comp := range typ.Components {
			if comp != component || gemIndex >= len(typ.Products) {
				continue
			}
			product := typ.Products[gemIndex]
			extra := []int{}
			if gemIndex < len(c.tables.Gold.GemIDs) && c.tables.Gold.GemIDs[gemIndex] >= 0 {
				extra = []int{c.tables.Gold.GemIDs[gemIndex]}
			}
			return &furnaceProduct{
				itemID: product.ItemID, level: product.Level, xp: product.XP,
				bar: c.itemID("GOLD_BAR"), mouldID: typ.MouldID, extra: extra, amount: 1,
			}
		}
	}
	return nil
}

// slayerRingProduct is component 16 on 446: the eternal ring
// when an eternal gem is carried, else the enchanted-gem ring;
// it needs Ring bling (the slayer_ring_unlocked varbit).
func (c *crafting) slayerRingProduct(player *game.Player) *furnaceProduct {
	slayer := c.tables.Gold.SlayerRing
	varbitID, ok := c.tables.VarbitIDs[slayer.UnlockVarbit]
	if !ok || getVarbit(player, varbitID) != 1 {
		player.SendMessage(slayerRingLocked)
		return nil
	}
	inv := player.Inventory()
	gemID, ringID := 0, 0
	for _, product := range slayer.Products {
		if inv != nil && inv.Contains(product.ItemID) {
			gemID, ringID = product.ItemID, product.Amount
			break
		}
	}
	if ringID == 0 && len(slayer.Products) > 0 {
		last := slayer.Products[len(slayer.Products)-1]
		gemID, ringID = last.ItemID, last.Amount
	}
	if ringID == 0 {
		return nil
	}
	return &furnaceProduct{
		itemID: ringID, level: slayer.Level, xp: slayer.XP,
		bar: c.itemID("GOLD_BAR"), mouldID: slayer.MouldID, extra: []int{gemID}, amount: 1,
	}
}

// silverProduct is the silver product for a menu component.
func (c *crafting) silverProduct(component int) *furnaceProduct {
	if c.tables == nil {
		return nil
	}
	for i := range c.tables.Silver.Products {
		product := &c.tables.Silver.Products[i]
		if product.Component != component {
			continue
		}
		if product.Members && !c.isMembersWorld() {
			return nil
		}
		return &furnaceProduct{
			itemID: product.ItemID, level: product.Level, xp: product.XP,
			bar: c.itemID("SILVER_BAR"), mouldID: product.MouldID,
			extra: append([]int(nil), product.ExtraIDs...), amount: product.Amount,
		}
	}
	return nil
}

// handleJewelleryButton is a quantity or product click on the
// open gold or silver menu.
func (c *crafting) handleJewelleryButton(event *plugins.InterfaceActionClickEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	player := event.Player
	group := event.ButtonID >> 16
	if (group != goldCraftingInterface && group != silverCraftingInterface) || player.GetInterfaceId() != group {
		return
	}
	menu := menuOf(player)
	if menu == nil || player.Location().Distance(menu.location) > 2 {
		return
	}
	component := event.ButtonID & 0xffff
	var quantities map[int]int
	var xComponent int
	if group == goldCraftingInterface {
		quantities, xComponent = c.tables.Gold.Quantities, c.tables.Gold.XComponent
	} else {
		quantities, xComponent = c.tables.Silver.Quantities, c.tables.Silver.XComponent
	}
	event.Handled = true
	if component == xComponent {
		menuCopy := menu
		player.SetEnteredAmountAction(&game.EnteredAmountAction{
			Execute: func(value int) {
				if value <= 0 || menuOf(player) != menuCopy {
					return
				}
				menuCopy.amount = min(value, 28)
				player.SendVarp(makeQuantityVarp, menuCopy.amount)
			},
		})
		if player.Session != nil {
			player.Session.SendPacket(protocol.EncodeRunClientScript(108, []protocol.ScriptArg{
				protocol.ScriptArgString("How many would you like to make?"),
			}))
		}
		return
	}
	if amount, ok := quantities[component]; ok {
		menu.amount = amount
		remembered := amount
		if remembered == 28 {
			remembered = 0
		}
		player.SendVarp(makeQuantityVarp, remembered)
		return
	}
	var product *furnaceProduct
	if group == goldCraftingInterface {
		product = c.goldProduct(player, component)
	} else {
		product = c.silverProduct(component)
	}
	if product != nil {
		c.startFurnaceProduct(player, menu, *product, event.ButtonID)
	}
}

// openFurnaceMenu opens the gold or silver jewellery menu.
func (c *crafting) openFurnaceMenu(player *game.Player, group int) {
	c.stopCrafting(player)
	player.SetAttribute(jewelleryMenuAttribute, &jewelleryMenu{amount: 1, location: player.Location()})
	player.SetInterfaceId(group)
	player.SendVarp(makeQuantityVarp, 1)
	if sender := jewellerySender(player); sender != nil {
		sender.MountSubInterface(mainModalTargetUID, group, 0)
	}
}

// furnaceMenuFor is the jewellery menu a furnace opens for
// what the player carries: gold first, else silver, else none
// (and then Smithing's bars menu opens — the caller falls
// through).
func (c *crafting) furnaceMenuFor(player *game.Player) int {
	if player == nil || player.Inventory() == nil || c.tables == nil {
		return 0
	}
	inv := player.Inventory()
	for i := range c.tables.Gold.Types {
		if inv.Contains(c.itemID("GOLD_BAR")) && inv.Contains(c.tables.Gold.Types[i].MouldID) {
			return goldCraftingInterface
		}
	}
	for i := range c.tables.Silver.Products {
		if inv.Contains(c.itemID("SILVER_BAR")) && inv.Contains(c.tables.Silver.Products[i].MouldID) {
			return silverCraftingInterface
		}
	}
	return 0
}

// smeltAtFurnace is Smelt on a furnace: the jewellery menu
// when a bar and its mould are carried, else false so
// Smithing's bars menu opens (Crafting's blank import sorts
// before Smithing's, so this handler runs first).
func (c *crafting) smeltAtFurnace(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	group := c.furnaceMenuFor(event.Player)
	if group == 0 {
		return false
	}
	c.openFurnaceMenu(event.Player, group)
	return true
}

// handleBarOnFurnace is a gold or silver bar on a furnace: it
// opens the cache's jewellery menu for it.
func (c *crafting) handleBarOnFurnace(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if !c.isFurnace(event.ObjectID) {
		return
	}
	if event.ItemID == c.itemID("GOLD_BAR") {
		event.Handled = true
		c.openFurnaceMenu(event.Player, goldCraftingInterface)
	} else if event.ItemID == c.itemID("SILVER_BAR") {
		event.Handled = true
		c.openFurnaceMenu(event.Player, silverCraftingInterface)
	}
}

// ---- perfect jewellery ----

// craftPerfectJewellery makes one 'perfect' piece (Jewellery
// .Crafting.js craftPerfectJewellery).
func (c *crafting) craftPerfectJewellery(player *game.Player, recipe PerfectRecipe) {
	inv := player.Inventory()
	if c.craftingLevel(player) < recipe.Level {
		player.SendMessage(fmt.Sprintf("You need a Crafting level of at least %d to craft this.", recipe.Level))
		return
	}
	if inv == nil || !inv.Contains(c.itemID("PERFECT_GOLD_BAR")) || !inv.Contains(c.itemID("RUBY")) || !inv.Contains(recipe.MouldID) {
		return
	}
	inv.DeleteNumber(c.itemID("PERFECT_GOLD_BAR"), 1)
	inv.DeleteNumber(c.itemID("RUBY"), 1)
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	player.SendMessage(fmt.Sprintf("You craft a %s.", recipe.Name))
}

// sendPrompt shows a multi-option chatbox prompt through the
// Api seam (plugins/dialogue.go), or false when no registry is
// installed (unit tests without a hub).
func (c *crafting) sendPrompt(player *game.Player, title string, pairs ...any) bool {
	if c == nil || c.api == nil {
		return false
	}
	return c.api.SendMultiChatboxPrompt(player, title, pairs...)
}

// handlePerfectJewellery is a 'perfect' gold bar on a furnace,
// with a ruby and a mould: one mould carried makes it at once;
// both moulds ask what to make.
func (c *crafting) handlePerfectJewellery(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if event.ItemID != c.itemID("PERFECT_GOLD_BAR") || !c.isFurnace(event.ObjectID) {
		return
	}
	player := event.Player
	inv := player.Inventory()
	if inv == nil {
		return
	}
	var recipes []PerfectRecipe
	for _, recipe := range c.tables.Perfect {
		if inv.Contains(recipe.MouldID) && inv.Contains(c.itemID("RUBY")) {
			recipes = append(recipes, recipe)
		}
	}
	if len(recipes) == 0 {
		return
	}
	event.Handled = true
	if len(recipes) == 1 {
		c.craftPerfectJewellery(player, recipes[0])
		return
	}
	// Both moulds carried: let the player choose, as the OSRS
	// jewellery interface does.
	first, second := recipes[0], recipes[1]
	c.sendPrompt(player, "What would you like to make?",
		first.Name, plugins.PromptCallback(func(*game.Player, int, string) { c.craftPerfectJewellery(player, first) }),
		second.Name, plugins.PromptCallback(func(*game.Player, int, string) { c.craftPerfectJewellery(player, second) }),
	)
}

// ---- stringing amulets ----

// stringAmulet is a ball of wool (or the recipe's material) on
// an unstrung amulet/symbol/emblem: it strings on the tick
// after the click, 4 XP, no animation.
func (c *crafting) stringAmulet(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	var pair *StrungAmuletPair
	for i := range c.tables.StrungAmulets.Pairs {
		entry := &c.tables.StrungAmulets.Pairs[i]
		if (entry.MaterialID == event.UsedItemID && entry.UnstrungID == event.UsedWithItemID) ||
			(entry.MaterialID == event.UsedWithItemID && entry.UnstrungID == event.UsedItemID) {
			pair = entry
			break
		}
	}
	if pair == nil {
		return
	}
	event.Handled = true
	player := event.Player
	strung := *pair
	materialID, unstrungID := strung.MaterialID, strung.UnstrungID
	noun := strung.Noun
	if noun == "" {
		noun = "amulet"
	}
	c.nextTick(player, func() {
		inv := player.Inventory()
		if inv == nil || !inv.Contains(materialID) || !inv.Contains(unstrungID) {
			return
		}
		inv.DeleteNumber(materialID, 1)
		inv.DeleteNumber(unstrungID, 1)
		inv.AddItem(game.NewItem(strung.StrungID, 1))
		c.addCraftingXp(player, c.tables.StrungAmulets.XP)
		c.spam(player, fmt.Sprintf("You put some string on your %s.", noun))
		c.produced(player, strung.StrungID, 1, c.tables.StrungAmulets.XP, "string", [][2]int{{unstrungID, 1}, {materialID, 1}})
	})
}

// AttachJewellery registers the Jewellery.Crafting.js hooks:
// Smelt on furnaces, 'perfect' bars and bars on furnaces,
// the interface clicks, and stringing. Jewellery does NOT use
// CreationMenu.
func (c *crafting) AttachJewellery(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnObjectInteractionNamed("Furnace", map[string]plugins.ObjectInteractionHandler{"Smelt": c.smeltAtFurnace})
	api.OnObjectInteractionNamed("Small furnace", map[string]plugins.ObjectInteractionHandler{"Smelt": c.smeltAtFurnace})
	noted := false
	filter := &plugins.ItemUseFilter{Noted: &noted}
	api.OnItemOnObject(c.handlePerfectJewellery, filter)
	api.OnItemOnObject(c.handleBarOnFurnace, filter)
	api.OnInterfaceActionClick(c.handleJewelleryButton)
	api.OnItemOnItem(c.stringAmulet, filter)
}
