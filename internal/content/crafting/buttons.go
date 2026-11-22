// Buttons.Crafting.js (23 lines, commit bfd81e3): Polish on
// buttons.
//
// Level 3, 5 XP, one use polishes the pair in place (setId +
// refreshItems). Members-only.
package crafting

import (
	"tsps-go/internal/plugins"
)

// ---- constants (Buttons.Crafting.js) ----

const (
	// buttonsLevel / buttonsXP are the hardcoded level and XP
	// (3 / 5).
	buttonsLevel = 3
	buttonsXP    = 5
)

// polish is Polish on Buttons (Buttons.Crafting.js polish).
func (c *crafting) polish(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil || c.tables == nil {
		return false
	}
	if !c.isMembersWorld() {
		return false
	}
	player := event.Player
	inv := player.Inventory()
	if inv == nil || event.Slot < 0 || event.Slot >= len(inv.Items()) {
		return false
	}
	buttons := c.itemID("BUTTONS")
	item := inv.Items()[event.Slot]
	if event.ItemID != buttons || item.ID() != buttons {
		return false
	}
	if c.craftingLevel(player) < buttonsLevel {
		c.spam(player, "You rub the buttons on your clothes but they aren't improved by the process.")
		return true
	}
	item.SetID(c.itemID("POLISHED_BUTTONS"))
	inv.SetItem(event.Slot, item)
	inv.RefreshItems()
	c.addCraftingXp(player, buttonsXP)
	c.spam(player, "You rub the buttons on your clothes and they become more shiny.")
	return true
}

// AttachButtons registers the Buttons.Crafting.js hook:
// onItemAction "Buttons" Polish.
func (c *crafting) AttachButtons(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnItemActionNamed("Buttons", map[string]plugins.ItemActionHandler{"Polish": c.polish}, nil)
}
