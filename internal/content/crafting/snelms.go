// Snelms.Crafting.js (99 lines, commit bfd81e3): a chisel on
// a blamish snail shell.
//
// Level 15 Crafting, 32.5 XP, 2 ticks, one shell per snelm,
// the chisel kept. Round shells make round snelms; pointed
// shells make pointed snelms, with no pointed bark variant in
// the cache. The cache has no dedicated snelm sequence, so the
// chisel-on-gem sequence (890, human_opalcutting) is reused.
// One shell needs no menu: straight into the loop.
package crafting

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Snelms.Crafting.js) ----

const (
	// snelmMenuTitle / snelmMenuMode are the snelm menu
	// ("How many would you like to make?", mode 0).
	snelmMenuTitle = "How many would you like to make?"
	snelmMenuMode  = 0
)

// craftSnelm makes one snelm (Snelms.Crafting.js craftOne).
func (c *crafting) craftSnelm(player *game.Player, recipe SnelmRecipe) bool {
	inv := player.Inventory()
	if inv == nil || !inv.Contains(recipe.ShellID) || c.chiselOf(player) == 0 {
		return false
	}
	inv.DeleteNumber(recipe.ShellID, 1)
	inv.AddItem(game.NewItem(recipe.ItemID, 1))
	c.addCraftingXp(player, c.tables.Snelms.XP)
	c.produced(player, recipe.ItemID, 1, c.tables.Snelms.XP, "snelms", [][2]int{{recipe.ShellID, 1}})
	c.emitEvent(CraftingMadeEventName, &CraftingMadeEvent{Player: player, ItemID: recipe.ItemID})
	c.spam(player, fmt.Sprintf("You make a %s.", strings.ToLower(player.ItemName(recipe.ItemID))))
	return true
}

// tooLowSnelm is the next-tick item box for a shell above the
// player's level.
func tooLowSnelm(player *game.Player, recipe SnelmRecipe, level int) {
	itemStatement(player, recipe.ShellID, fmt.Sprintf("You need a Crafting level of %d to craft this.", level))
}

// chiselOnShell is a chisel on a blamish snail shell.
func (c *crafting) chiselOnShell(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if !c.isMembersWorld() {
		return
	}
	shellID := c.chiselPair(event)
	recipe := c.tables.SnelmByShell(shellID)
	if recipe == nil {
		return
	}
	event.Handled = true
	player := event.Player
	if c.craftingLevel(player) < c.tables.Snelms.Level {
		c.stopCrafting(player)
		found := *recipe
		level := c.tables.Snelms.Level
		c.nextTick(player, func() { tooLowSnelm(player, found, level) })
		return
	}
	c.stopCrafting(player)
	amount := player.Inventory().GetAmount(recipe.ShellID)
	if amount == 0 {
		return
	}
	start := func(chosen int) {
		found := *recipe
		c.startMaking(player, MakeOptions{
			Amount: chosen, StartTick: nil, FirstTick: 1, Interval: c.tables.Snelms.Ticks,
			Animation: c.tables.Snelms.SeqID, Kind: "snelms",
			Has:  func() bool { return player.Inventory().Contains(found.ShellID) && c.chiselOf(player) != 0 },
			Make: func() bool { return c.craftSnelm(player, found) },
		})
	}
	// One shell needs no menu: the Wiki's Make-X prompt only
	// matters for several.
	if amount == 1 {
		start(1)
		return
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: snelmMenuTitle, Mode: snelmMenuMode,
		Items: []MakeMenuItem{{ItemID: recipe.ShellID}}, MaxAmount: amount,
		Choose: func(_ MakeMenuItem, chosen int) { start(chosen) },
	})
}

// AttachSnelms registers the Snelms.Crafting.js hook: chisel
// on a shell, onItemOnItem with noted:false.
func (c *crafting) AttachSnelms(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.chiselOnShell, &plugins.ItemUseFilter{Noted: &noted})
}
