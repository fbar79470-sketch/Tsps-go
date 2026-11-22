// Leather.Crafting.js (354 lines, commit bfd81e3): sewing
// leather and hides with a needle, the Hunters' Guild meat
// and fur pouches from fur, and the armour made by adding
// something to a finished piece (studs, kebbit claws, hides
// nailed onto a shield).
//
// Thread: a reel lasts 5 items (persistAttribute
// "crafting:thread-uses"); the 5th consumes it. A costume
// needle needs no thread. Combine counts only plain copies
// (PoweredStaff plainAmount/takePlain): Go has no presets, so
// plain == total — recorded as a simplification, not a gap
// (gap 27h). A preset-only hold therefore never arises, and
// heldOnlyAsPreset always answers false.
package crafting

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Leather.Crafting.js) ----

const (
	// craftsPerThread is CRAFTS_PER_THREAD: a reel of thread
	// lasts five items.
	craftsPerThread = 5
	// leatherMenuTitle is the skillmulti title for the
	// leather and combine menus.
	leatherMenuTitle = "How many do you wish to make?"
	// leatherMenuMode is skillmulti_setup's first argument
	// for leather menus, as captured (mode 0).
	leatherMenuMode = 0
	// leatherCraftingSeqName is human_leather_crafting, the
	// sewing animation (1249 in rev-242).
	leatherCraftingSeqName = "human_leather_crafting"
	// presetText is PoweredStaff PRESET_TEXT: the line for a
	// combine held only as preset copies. Go has no presets,
	// so it is never sent; the constant is kept so the
	// fail-closed branch reads as TS does.
	presetText = "Preset items cannot be used for this."
)

// ---- thread (Leather.Crafting.js useThread) ----

// useThread charges one use of the carried reel; the fifth
// uses it up. A costume needle needs no thread.
func (c *crafting) useThread(player *game.Player, needleID int) {
	if player == nil || player.Inventory() == nil {
		return
	}
	if needleID != 0 && needleID == c.itemID("COSTUME_NEEDLE") {
		return
	}
	uses := threadUses(player) + 1
	if uses < craftsPerThread {
		player.SetAttribute(ThreadUsesAttribute, uses)
		return
	}
	player.SetAttribute(ThreadUsesAttribute, 0)
	player.Inventory().DeleteNumber(c.itemID("THREAD"), 1)
	c.spam(player, "You use up one of your reels of thread.")
}

// ---- leather sewing ----

// needleIDs returns the needle ids (needle + costume needle).
func (c *crafting) needleIDs() map[int]bool {
	needles := make(map[int]bool)
	for _, name := range []string{"NEEDLE", "COSTUME_NEEDLE"} {
		if id := c.itemID(name); id != 0 {
			needles[id] = true
		}
	}
	return needles
}

// hasLeatherMaterials reports whether the player can sew one
// more of recipe from materialId with needleId.
func (c *crafting) hasLeatherMaterials(player *game.Player, materialID int, recipe LeatherRecipe, needleID int) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	return inv.Contains(needleID) &&
		(needleID == c.itemID("COSTUME_NEEDLE") || inv.Contains(c.itemID("THREAD"))) &&
		inv.GetAmount(materialID) >= recipe.Leather
}

// craftLeatherItem sews one leather item (Leather.Crafting.js
// craftLeatherItem).
func (c *crafting) craftLeatherItem(player *game.Player, materialID int, recipe LeatherRecipe, needleID int) bool {
	if !c.hasLeatherMaterials(player, materialID, recipe, needleID) {
		return false
	}
	inv := player.Inventory()
	inv.DeleteNumber(materialID, recipe.Leather)
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	c.spam(player, fmt.Sprintf("You make %s.", recipe.Made))
	c.useThread(player, needleID)
	c.emitEvent(CraftingMadeEventName, &CraftingMadeEvent{Player: player, ItemID: recipe.ItemID})
	c.produced(player, recipe.ItemID, 1, recipe.XP, "leather", [][2]int{{materialID, recipe.Leather}})
	return true
}

// tooLowStatement is the captured message box for an item
// above the player's level.
func tooLowStatement(player *game.Player, level int, made string) {
	statement(player, fmt.Sprintf("You need a Crafting level of %d to make %s.", level, made))
}

func (c *crafting) startLeatherCrafting(player *game.Player, materialID int, recipe LeatherRecipe, amount, needleID int) {
	if c.craftingLevel(player) < recipe.Level {
		tooLowStatement(player, recipe.Level, recipe.Made)
		return
	}
	// Items on ticks 2, 5, 8 ... after the choice; the
	// animation restarts with each one.
	zero := 0
	c.startMaking(player, MakeOptions{
		Amount: amount, StartTick: &zero, FirstTick: 2, Interval: 3,
		Animation: c.seqID(leatherCraftingSeqName), ResetAnimation: true, Kind: "leather",
		Has:  func() bool { return c.hasLeatherMaterials(player, materialID, recipe, needleID) },
		Make: func() bool { return c.craftLeatherItem(player, materialID, recipe, needleID) },
	})
}

// handleNeedleOnLeather is a needle (or costume needle) used
// on leather, in either order: it opens the skillmulti menu.
func (c *crafting) handleNeedleOnLeather(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	needles := c.needleIDs()
	needleID := 0
	materialID := 0
	switch {
	case needles[event.UsedItemID]:
		needleID, materialID = event.UsedItemID, event.UsedWithItemID
	case needles[event.UsedWithItemID]:
		needleID, materialID = event.UsedWithItemID, event.UsedItemID
	default:
		return
	}
	recipes := c.tables.LeatherByMaterial(materialID)
	if len(recipes) == 0 {
		return
	}
	event.Handled = true
	player := event.Player
	inv := player.Inventory()
	if needleID != c.itemID("COSTUME_NEEDLE") && (inv == nil || !inv.Contains(c.itemID("THREAD"))) {
		// ponytail: not in the captures or on the Wiki;
		// replace with the live text when captured.
		player.SendMessage("You need some thread to sew leather.")
		return
	}
	c.stopCrafting(player)
	items := make([]MakeMenuItem, 0, len(recipes))
	for _, recipe := range availableFor(c, recipes, func(r LeatherRecipe) bool { return r.Members }) {
		items = append(items, MakeMenuItem{ItemID: recipe.ItemID, Label: recipe.Label})
	}
	// The menu carries the recipes by item id; Choose looks
	// the recipe back up from the table.
	byItem := make(map[int]LeatherRecipe, len(recipes))
	for _, recipe := range recipes {
		byItem[recipe.ItemID] = recipe
	}
	maxAmount := 0
	if inv != nil {
		maxAmount = inv.GetAmount(materialID)
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: leatherMenuTitle, Mode: leatherMenuMode, Items: items, MaxAmount: maxAmount,
		Choose: func(item MakeMenuItem, amount int) {
			if recipe, ok := byItem[item.ItemID]; ok {
				c.startLeatherCrafting(player, materialID, recipe, amount, needleID)
			}
		},
	})
}

// ---- hunters' pouches ----

// pouchName is the pouch's lowercase display name.
func pouchName(player *game.Player, id int) string {
	return strings.ToLower(player.ItemName(id))
}

// hasPouchParts reports whether the player can sew one more
// pouch of recipe with needleId.
func (c *crafting) hasPouchParts(player *game.Player, recipe HunterPouchRecipe, needleID int) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	return inv.Contains(needleID) && (needleID == c.itemID("COSTUME_NEEDLE") || inv.Contains(c.itemID("THREAD"))) &&
		inv.GetAmount(recipe.FurID) >= recipe.Amount && (!recipe.HasFrom || inv.Contains(recipe.FromID))
}

// sewPouch sews one hunters' pouch (Leather.Crafting.js
// sewPouch): the inputs go, then the pouch takes the first
// free slot.
func (c *crafting) sewPouch(player *game.Player, recipe HunterPouchRecipe, needleID int) bool {
	if !c.hasPouchParts(player, recipe, needleID) {
		return false
	}
	inv := player.Inventory()
	inv.DeleteNumber(recipe.FurID, recipe.Amount)
	if recipe.HasFrom {
		inv.DeleteNumber(recipe.FromID, 1)
	}
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	c.spam(player, fmt.Sprintf("You make %s.", withArticle(pouchName(player, recipe.ItemID))))
	c.useThread(player, needleID)
	c.emitEvent(CraftingMadeEventName, &CraftingMadeEvent{Player: player, ItemID: recipe.ItemID})
	used := [][2]int{{recipe.FurID, recipe.Amount}}
	if recipe.HasFrom {
		used = append(used, [2]int{recipe.FromID, 1})
	}
	c.produced(player, recipe.ItemID, 1, recipe.XP, "leather", used)
	return true
}

// handleNeedleOnFur is a needle on fur, or fur on a needle:
// one pouch, no menu (rsprox captures: the animation at once,
// the pouch and the XP 2 ticks later).
func (c *crafting) handleNeedleOnFur(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	needles := c.needleIDs()
	needleID := 0
	furID := 0
	switch {
	case needles[event.UsedItemID]:
		needleID, furID = event.UsedItemID, event.UsedWithItemID
	case needles[event.UsedWithItemID]:
		needleID, furID = event.UsedWithItemID, event.UsedItemID
	default:
		return
	}
	var recipe *HunterPouchRecipe
	for _, entry := range availableFor(c, c.tables.HuntersPouches, func(r HunterPouchRecipe) bool { return r.Members }) {
		if entry.FurID == furID {
			found := entry
			recipe = &found
			break
		}
	}
	if recipe == nil {
		return
	}
	event.Handled = true
	player := event.Player
	inv := player.Inventory()
	made := pouchName(player, recipe.ItemID)
	if needleID != c.itemID("COSTUME_NEEDLE") && (inv == nil || !inv.Contains(c.itemID("THREAD"))) {
		statement(player, fmt.Sprintf("You'll need a needle and some thread to craft %s.", withArticle(made)))
		return
	}
	if recipe.HasFrom && (inv == nil || !inv.Contains(recipe.FromID)) {
		size := strings.Split(made, " ")[0]
		itemStatement(player, recipe.FromID, fmt.Sprintf("You need %s to upgrade into %s pouch.", withArticle(pouchName(player, recipe.FromID)), withArticle(size)))
		return
	}
	if c.craftingLevel(player) < recipe.Level {
		tooLowStatement(player, recipe.Level, withArticle(made))
		return
	}
	if inv == nil || inv.GetAmount(furID) < recipe.Amount {
		// Ours: not captured.
		player.SendMessage(fmt.Sprintf("You need %d %s to make %s.", recipe.Amount, pouchName(player, furID), withArticle(made)))
		return
	}
	c.stopCrafting(player)
	// The animation plays once, at the click; the pouch comes
	// with its reset (captured).
	zero := 0
	c.startMaking(player, MakeOptions{
		Amount: 1, StartTick: &zero, FirstTick: 2, Interval: 3,
		Animation: 0, ResetAnimation: true, Kind: "leather",
		Has:  func() bool { return c.hasPouchParts(player, *recipe, needleID) },
		Make: func() bool { return c.sewPouch(player, *recipe, needleID) },
	})
	player.PerformAnimation(c.seqID(leatherCraftingSeqName), 0)
}

// ---- combine ----

// combineAmounts is the pair amounts a combine takes
// (pairAmounts, default [1, 1]).
func combineAmounts(recipe CombineRecipe) [2]int {
	return recipe.PairAmounts
}

// hasCombineTool reports whether the player holds a combine
// tool: a repaired Imcando hammer substitutes for a carried
// hammer, including when wielded.
func (c *crafting) hasCombineTool(player *game.Player, toolID int) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	if inv.Contains(toolID) {
		return true
	}
	if toolID != c.itemID("HAMMER") {
		return false
	}
	alternatives := []int{c.itemID("IMCANDO_HAMMER"), c.itemID("IMCANDO_HAMMER_OFF_HAND_")}
	equipment := player.Equipment()
	for _, id := range alternatives {
		if id != 0 && inv.Contains(id) {
			return true
		}
		if equipment == nil || id == 0 {
			continue
		}
		if equipment.ItemAt(game.WeaponSlot).ID() == id || equipment.ItemAt(game.ShieldSlot).ID() == id {
			return true
		}
	}
	return false
}

// hasCombineMaterials reports whether the player holds
// everything one combine takes.
func (c *crafting) hasCombineMaterials(player *game.Player, recipe CombineRecipe) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	amounts := combineAmounts(recipe)
	if inv.GetAmount(recipe.Pair[0]) < amounts[0] || inv.GetAmount(recipe.Pair[1]) < amounts[1] {
		return false
	}
	for _, material := range recipe.Materials {
		if inv.GetAmount(material.ItemID) < material.Amount {
			return false
		}
	}
	for _, tool := range recipe.ToolIDs {
		if !c.hasCombineTool(player, tool) {
			return false
		}
	}
	return true
}

// combineItem combines one finished piece (Leather.Crafting.js
// combine): no spam line and no crafting:made emit in TS.
func (c *crafting) combineItem(player *game.Player, recipe CombineRecipe) bool {
	if !c.hasCombineMaterials(player, recipe) {
		return false
	}
	inv := player.Inventory()
	amounts := combineAmounts(recipe)
	inv.DeleteNumber(recipe.Pair[0], amounts[0])
	inv.DeleteNumber(recipe.Pair[1], amounts[1])
	for _, material := range recipe.Materials {
		inv.DeleteNumber(material.ItemID, material.Amount)
	}
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	materials := [][2]int{{recipe.Pair[0], amounts[0]}, {recipe.Pair[1], amounts[1]}}
	for _, material := range recipe.Materials {
		materials = append(materials, [2]int{material.ItemID, material.Amount})
	}
	c.produced(player, recipe.ItemID, 1, recipe.XP, "leather", materials)
	return true
}

// missingCombineItem is the first tool or material short for
// one item, as [id, amount needed].
func (c *crafting) missingCombineItem(player *game.Player, recipe CombineRecipe) (int, int) {
	inv := player.Inventory()
	for _, tool := range recipe.ToolIDs {
		if !c.hasCombineTool(player, tool) {
			return tool, 1
		}
	}
	amounts := combineAmounts(recipe)
	needs := [][2]int{{recipe.Pair[0], amounts[0]}, {recipe.Pair[1], amounts[1]}}
	for _, material := range recipe.Materials {
		needs = append(needs, [2]int{material.ItemID, material.Amount})
	}
	for _, need := range needs {
		if inv == nil || inv.GetAmount(need[0]) < need[1] {
			return need[0], need[1]
		}
	}
	return 0, 0
}

// heldOnlyAsPreset reports whether every tool and piece is
// held, but enough of one piece only counting its preset
// copies. Go has no presets, so plain == total and this is
// always false (gap 27h: a simplification, not a divergence
// in this codebase).
func (c *crafting) heldOnlyAsPreset(player *game.Player, recipe CombineRecipe) bool {
	return false
}

// maxCombines is how many combines the carried pieces cover.
func (c *crafting) maxCombines(player *game.Player, recipe CombineRecipe) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	inv := player.Inventory()
	amounts := combineAmounts(recipe)
	most := inv.GetAmount(recipe.Pair[0]) / amounts[0]
	if second := inv.GetAmount(recipe.Pair[1]) / amounts[1]; second < most {
		most = second
	}
	for _, material := range recipe.Materials {
		if count := inv.GetAmount(material.ItemID) / material.Amount; count < most {
			most = count
		}
	}
	return most
}

func (c *crafting) startCombining(player *game.Player, recipe CombineRecipe, amount int) {
	name := strings.ToLower(player.ItemName(recipe.ItemID))
	if c.craftingLevel(player) < recipe.Level {
		tooLowStatement(player, recipe.Level, "a "+name)
		return
	}
	zero := 0
	c.startMaking(player, MakeOptions{
		Amount: amount, StartTick: &zero, FirstTick: recipe.Ticks - 1, Interval: recipe.Ticks,
		Animation: recipe.SeqID, ResetAnimation: true, Kind: "leather",
		Has:  func() bool { return c.hasCombineMaterials(player, recipe) },
		Make: func() bool { return c.combineItem(player, recipe) },
	})
}

// handleCombine is studs, claws or a hide used on the piece
// it goes onto, in either order.
func (c *crafting) handleCombine(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	var recipe *CombineRecipe
	for _, entry := range availableFor(c, c.tables.Combine, func(r CombineRecipe) bool { return r.Members }) {
		if (entry.Pair[0] == event.UsedItemID && entry.Pair[1] == event.UsedWithItemID) ||
			(entry.Pair[1] == event.UsedItemID && entry.Pair[0] == event.UsedWithItemID) {
			found := entry
			recipe = &found
			break
		}
	}
	if recipe == nil {
		return
	}
	event.Handled = true
	player := event.Player
	if !c.hasCombineMaterials(player, *recipe) && c.heldOnlyAsPreset(player, *recipe) {
		player.SendMessage(presetText)
		return
	}
	if !c.hasCombineMaterials(player, *recipe) {
		// ponytail: not captured; names what is missing, and
		// how many when more than one is needed.
		id, amount := c.missingCombineItem(player, *recipe)
		name := strings.ToLower(player.ItemName(id))
		if amount > 1 {
			player.SendMessage(fmt.Sprintf("You need %d %s to make this.", amount, name))
		} else {
			player.SendMessage(fmt.Sprintf("You need %s to make this.", withArticle(name)))
		}
		return
	}
	c.stopCrafting(player)
	c.openMakeMenu(player, MakeMenuOptions{
		Title: leatherMenuTitle, Mode: leatherMenuMode,
		Items: []MakeMenuItem{{ItemID: recipe.ItemID}}, MaxAmount: c.maxCombines(player, *recipe),
		Choose: func(chosen MakeMenuItem, amount int) {
			c.startCombining(player, *recipe, amount)
		},
	})
}

// AttachLeather registers the Leather.Crafting.js hooks:
// needle on leather, needle on fur, and combine, all
// onItemOnItem with noted:false.
func (c *crafting) AttachLeather(api *plugins.Api) {
	if api == nil {
		return
	}
	api.PersistAttribute(ThreadUsesAttribute, 0)
	noted := false
	filter := &plugins.ItemUseFilter{Noted: &noted}
	api.OnItemOnItem(c.handleNeedleOnLeather, filter)
	api.OnItemOnItem(c.handleNeedleOnFur, filter)
	api.OnItemOnItem(c.handleCombine, filter)
}
