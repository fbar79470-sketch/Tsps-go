// Limestone.Crafting.js (53 lines, commit bfd81e3): a chisel
// on limestone carves a limestone brick.
//
// Level 12 Crafting, 6 XP, 0 ticks: each use carves one brick
// on the tick of the use, with no menu; the brick takes the
// first limestone's slot, human_limestonecutting plays, the
// line is spam, and there is no sound. Limestone has NO
// section in crafting.json; the level, XP and animation are
// hardcoded here, as in TS.
//
// Not covered: the Wiki says carving can fail below level 40,
// making a rock for 1.5 XP, but no source gives the failure
// rate or its line, so every carve succeeds (gap 27g).
package crafting

import (
	"tsps-go/internal/plugins"
)

// ---- constants (Limestone.Crafting.js) ----

const (
	// limestoneLevel / limestoneXP are the hardcoded level and
	// XP (12 / 6).
	limestoneLevel = 12
	limestoneXP    = 6
	// limestoneCarveMessage is the spam line.
	limestoneCarveMessage = "You use the chisel on the limestone and carve it into a building block."
	// limestoneTooLowMessage is the below-level line (ours: no
	// capture or Wiki line for a player below level 12).
	limestoneTooLowMessage = "You need a Crafting level of 12 to carve limestone."
	// limestoneSeqName is human_limestonecutting (1309 in
	// rev-242).
	limestoneSeqName = "human_limestonecutting"
)

// limestoneSlot is the inventory slot of the limestone in a
// chisel use, or -1 when the use is not chisel-on-limestone.
func (c *crafting) limestoneSlot(event *plugins.PluginItemOnItemEvent) int {
	limestone := c.itemID("LIMESTONE")
	chisel, jewellers := c.itemID("CHISEL"), c.itemID("JEWELLERS_CHISEL")
	if event.UsedItemID == limestone && (event.UsedWithItemID == chisel || event.UsedWithItemID == jewellers) {
		return event.UsedItemSlot
	}
	if event.UsedWithItemID == limestone && (event.UsedItemID == chisel || event.UsedItemID == jewellers) {
		return event.UsedWithItemSlot
	}
	return -1
}

// carveLimestone is a chisel on limestone: one brick, on the
// tick of the use.
func (c *crafting) carveLimestone(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if event.Handled || !c.isMembersWorld() {
		return
	}
	slot := c.limestoneSlot(event)
	if slot < 0 {
		return
	}
	player := event.Player
	inv := player.Inventory()
	if inv == nil || slot >= len(inv.Items()) || inv.Items()[slot].ID() != c.itemID("LIMESTONE") {
		return
	}
	event.Handled = true
	c.stopCrafting(player)
	if c.craftingLevel(player) < limestoneLevel {
		player.SendMessage(limestoneTooLowMessage)
		return
	}
	// The brick replaces the FIRST limestone's slot, whichever
	// limestone was used (captured).
	items := inv.Items()
	brick := c.itemID("LIMESTONE_BRICK")
	for i, item := range items {
		if item.ID() == c.itemID("LIMESTONE") {
			item.SetID(brick)
			inv.SetItem(i, item)
			break
		}
	}
	inv.RefreshItems()
	player.PerformAnimation(c.seqID(limestoneSeqName), 0)
	c.spam(player, limestoneCarveMessage)
	c.addCraftingXp(player, limestoneXP)
	c.produced(player, brick, 1, limestoneXP, "limestone-brick", [][2]int{{c.itemID("LIMESTONE"), 1}})
	c.emitEvent(CraftingMadeEventName, &CraftingMadeEvent{Player: player, ItemID: brick})
}

// AttachLimestone registers the Limestone.Crafting.js hook:
// chisel on limestone, onItemOnItem with noted:false.
func (c *crafting) AttachLimestone(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.carveLimestone, &plugins.ItemUseFilter{Noted: &noted})
}
