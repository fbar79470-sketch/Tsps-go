// LightOrbs.Crafting.js (37 lines, commit bfd81e3): complete
// an empty light orb with cave goblin wire.
//
// Level 87 Crafting, 104 XP, at once, no menu, no animation,
// no message. Light orbs have NO section in crafting.json;
// the level, XP and items are hardcoded here, as in TS. The
// skill action also exposes a focused event for Sherlock's
// Dorgesh-Kaan task ("crafting:light-orb-made").
package crafting

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (LightOrbs.Crafting.js) ----

const (
	// lightOrbLevel / lightOrbXP are the hardcoded level and
	// XP (87 / 104).
	lightOrbLevel = 87
	lightOrbXP    = 104
)

// makeLightOrb is an empty light orb + cave goblin wire.
func (c *crafting) makeLightOrb(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	emptyOrb, wire := c.itemID("EMPTY_LIGHT_ORB"), c.itemID("CAVE_GOBLIN_WIRE")
	pair := map[int]bool{event.UsedItemID: true, event.UsedWithItemID: true}
	if !pair[emptyOrb] || !pair[wire] || !c.isMembersWorld() {
		return
	}
	event.Handled = true
	player := event.Player
	c.stopCrafting(player)
	if c.craftingLevel(player) < lightOrbLevel {
		player.SendMessage("You need a Crafting level of 87 to do that.")
		return
	}
	inv := player.Inventory()
	if inv == nil || !inv.Contains(emptyOrb) || !inv.Contains(wire) {
		return
	}
	inv.DeleteNumber(emptyOrb, 1)
	inv.DeleteNumber(wire, 1)
	inv.AddItem(game.NewItem(c.itemID("LIGHT_ORB"), 1))
	c.addCraftingXp(player, lightOrbXP)
	c.produced(player, c.itemID("LIGHT_ORB"), 1, lightOrbXP, "light-orb", [][2]int{{emptyOrb, 1}, {wire, 1}})
	c.emitEvent(LightOrbMadeEventName, &LightOrbMadeEvent{Player: player, ItemID: c.itemID("LIGHT_ORB")})
}

// AttachLightOrbs registers the LightOrbs.Crafting.js hook:
// empty light orb on cave goblin wire, onItemOnItem with
// noted:false.
func (c *crafting) AttachLightOrbs(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.makeLightOrb, &plugins.ItemUseFilter{Noted: &noted})
}
