// Battlestaves.Crafting.js (79 lines, commit bfd81e3): a
// charged orb used on a battlestaff, in either order.
//
// Menu mode 0, "How many do you wish to make?", single recipe
// per orb; the first staff takes 3 ticks after the choice and
// each one after it 2, anim 7531 (human_battlestaff_crafting),
// no message. Members-only.
package crafting

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Battlestaves.Crafting.js) ----

const (
	// battlestaffMenuTitle / battlestaffMenuMode are the
	// battlestaff menu ("How many do you wish to make?",
	// mode 0).
	battlestaffMenuTitle = "How many do you wish to make?"
	battlestaffMenuMode  = 0
)

// hasBattlestaffMaterials reports whether the player holds a
// charged orb and a battlestaff.
func (c *crafting) hasBattlestaffMaterials(player *game.Player, recipe BattlestavesRecipe) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	return inv.Contains(recipe.OrbID) && inv.Contains(c.itemID("BATTLESTAFF"))
}

// makeStaff makes one battlestaff (Battlestaves.Crafting.js
// makeStaff).
func (c *crafting) makeStaff(player *game.Player, recipe BattlestavesRecipe) bool {
	if !c.hasBattlestaffMaterials(player, recipe) {
		return false
	}
	inv := player.Inventory()
	inv.DeleteNumber(recipe.OrbID, 1)
	inv.DeleteNumber(c.itemID("BATTLESTAFF"), 1)
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	c.produced(player, recipe.ItemID, 1, recipe.XP, "battlestaves", nil)
	return true
}

func (c *crafting) startStaves(player *game.Player, recipe BattlestavesRecipe, amount int) {
	if c.craftingLevel(player) < recipe.Level {
		// ponytail: not captured; follows the glassblowing
		// message.
		player.SendMessage(fmt.Sprintf("You need level %d Crafting to make %s.", recipe.Level, withArticle(strings.ToLower(player.ItemName(recipe.ItemID)))))
		return
	}
	zero := 0
	c.startMaking(player, MakeOptions{
		Amount: amount, StartTick: &zero, FirstTick: 3, Interval: 2,
		Animation: c.tables.Battlestaves.SeqID, Kind: "battlestaves",
		Has:  func() bool { return c.hasBattlestaffMaterials(player, recipe) },
		Make: func() bool { return c.makeStaff(player, recipe) },
	})
}

// attachOrb is a charged orb on a battlestaff, in either
// order: the battlestaff menu.
func (c *crafting) attachOrb(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	staff := c.itemID("BATTLESTAFF")
	var orbID int
	switch {
	case event.UsedItemID == staff:
		orbID = event.UsedWithItemID
	case event.UsedWithItemID == staff:
		orbID = event.UsedItemID
	default:
		return
	}
	var recipe *BattlestavesRecipe
	for i := range c.tables.Battlestaves.Recipes {
		if c.tables.Battlestaves.Recipes[i].OrbID == orbID {
			found := c.tables.Battlestaves.Recipes[i]
			recipe = &found
			break
		}
	}
	if recipe == nil || !c.isMembersWorld() {
		return
	}
	event.Handled = true
	player := event.Player
	inv := player.Inventory()
	c.stopCrafting(player)
	maxAmount := 0
	if inv != nil {
		maxAmount = min(inv.GetAmount(recipe.OrbID), inv.GetAmount(staff))
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: battlestaffMenuTitle, Mode: battlestaffMenuMode,
		Items: []MakeMenuItem{{ItemID: recipe.ItemID}}, MaxAmount: maxAmount,
		Choose: func(_ MakeMenuItem, amount int) {
			c.startStaves(player, *recipe, amount)
		},
	})
}

// AttachBattlestaves registers the Battlestaves.Crafting.js
// hook: orb on battlestaff, onItemOnItem with noted:false.
func (c *crafting) AttachBattlestaves(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.attachOrb, &plugins.ItemUseFilter{Noted: &noted})
}
