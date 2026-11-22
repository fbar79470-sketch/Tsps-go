// Glassblowing.Crafting.js (80 lines, commit bfd81e3):
// blowing molten glass with a glassblowing pipe.
//
// The pipe on molten glass, in either order, opens skillmulti
// mode 0, "How many do you wish to make?", with all eight items
// whatever the player's level. Choosing one above it answers
// "You need level 87 Crafting to make a light orb." and makes
// nothing. Otherwise human_glassblowing and sound 2724 start at
// once, and every 3 ticks from the second the player makes one
// ("You make a beer glass."), the animation and sound replayed.
// Every item is members.
package crafting

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Glassblowing.Crafting.js) ----

const (
	// glassMenuTitle / glassMenuMode are the glassblowing
	// menu ("How many do you wish to make?", mode 0).
	glassMenuTitle = "How many do you wish to make?"
	glassMenuMode  = 0
)

// hasGlass reports whether the player holds a pipe and molten
// glass.
func (c *crafting) hasGlass(player *game.Player) bool {
	if player == nil || player.Inventory() == nil {
		return false
	}
	inv := player.Inventory()
	return inv.Contains(c.itemID("GLASSBLOWING_PIPE")) && inv.Contains(c.itemID("MOLTEN_GLASS"))
}

// blowOne blows one glass item (Glassblowing.Crafting.js
// blowOne).
func (c *crafting) blowOne(player *game.Player, recipe GlassblowingRecipe) bool {
	if !c.hasGlass(player) {
		return false
	}
	inv := player.Inventory()
	inv.DeleteNumber(c.itemID("MOLTEN_GLASS"), 1)
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, recipe.XP)
	c.produced(player, recipe.ItemID, 1, recipe.XP, "glassblowing", nil)
	made := recipe.Made
	if made == "" {
		made = withArticle(recipe.Name)
	}
	// rsprox captures: spam, and the orb's own wording ("You
	// make an orb.").
	c.spam(player, fmt.Sprintf("You make %s.", made))
	return true
}

func (c *crafting) startBlowing(player *game.Player, recipe GlassblowingRecipe, amount int) {
	if c.craftingLevel(player) < recipe.Level {
		player.SendMessage(fmt.Sprintf("You need level %d Crafting to make %s.", recipe.Level, withArticle(recipe.Name)))
		return
	}
	zero := 0
	c.startMaking(player, MakeOptions{
		Amount: amount, StartTick: &zero, FirstTick: 2, Interval: 3,
		Animation: c.tables.Glassblowing.SeqID,
		Sound:     game.NewSound(c.tables.Glassblowing.SoundID, 1, c.tables.Glassblowing.SoundDelay, 0),
		Kind:      "glassblowing",
		Has:       func() bool { return c.hasGlass(player) },
		Make:      func() bool { return c.blowOne(player, recipe) },
	})
}

// blowGlass is a glassblowing pipe on molten glass, in either
// order: the glassblowing menu.
func (c *crafting) blowGlass(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	pipe, molten := c.itemID("GLASSBLOWING_PIPE"), c.itemID("MOLTEN_GLASS")
	ids := map[int]bool{event.UsedItemID: true, event.UsedWithItemID: true}
	if !ids[pipe] || !ids[molten] || !c.isMembersWorld() {
		return
	}
	event.Handled = true
	player := event.Player
	c.stopCrafting(player)
	items := make([]MakeMenuItem, 0, len(c.tables.Glassblowing.Recipes))
	for _, recipe := range availableFor(c, c.tables.Glassblowing.Recipes, func(r GlassblowingRecipe) bool { return r.Members }) {
		items = append(items, MakeMenuItem{ItemID: recipe.ItemID, Label: recipe.Label})
	}
	byItem := make(map[int]GlassblowingRecipe, len(c.tables.Glassblowing.Recipes))
	for _, recipe := range c.tables.Glassblowing.Recipes {
		byItem[recipe.ItemID] = recipe
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: glassMenuTitle, Mode: glassMenuMode, Items: items,
		MaxAmount: player.Inventory().GetAmount(molten),
		Choose: func(item MakeMenuItem, amount int) {
			if recipe, ok := byItem[item.ItemID]; ok {
				c.startBlowing(player, recipe, amount)
			}
		},
	})
}

// AttachGlassblowing registers the Glassblowing.Crafting.js
// hook: pipe on molten glass, onItemOnItem with noted:false.
func (c *crafting) AttachGlassblowing(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.blowGlass, &plugins.ItemUseFilter{Noted: &noted})
}
