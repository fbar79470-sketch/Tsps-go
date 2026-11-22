// Gems.Crafting.js (194 lines, commit bfd81e3): cutting gems
// and amethyst with a chisel.
//
// A chisel on an uncut gem opens skillmulti mode 12; the first
// gem is cut on the tick after the choice and one more every 2
// ticks, each with the gem's cutting animation, sound 2586 and
// "You cut the diamond." (spam). A semi-precious gem can be
// smashed instead (a crushed gem, sound 2589). A Jeweller's
// chisel adds +0.2 success and has a 10% chance to cut a second
// gem at once. Amethyst opens menu mode 0. The
// "crafting:cut-gem" answerer lives in crafting.go (registered
// by register), as in TS where Gems registers it.
package crafting

import (
	"fmt"
	"math"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Gems.Crafting.js) ----

const (
	// gemMenuTitle / gemMenuMode are the gem-cutting menu
	// ("How many gems would you like to cut?", mode 12).
	gemMenuTitle = "How many gems would you like to cut?"
	gemMenuMode  = 12
	// amethystMenuTitle / amethystMenuMode are the amethyst
	// menu ("What would you like to make?", mode 0).
	amethystMenuTitle = "What would you like to make?"
	amethystMenuMode  = 0
	// gemCuttingSoundID is the cut sound (2586); gemCrushedSoundID
	// is the crush sound (2589).
	gemCuttingSoundID = 2586
	gemCrushedSoundID = 2589
	// jewellersChiselSuccessBonus is the +0.2 success a
	// Jeweller's chisel adds to the crush roll.
	jewellersChiselSuccessBonus = 0.2
	// jewellersChiselDoubleChance is the 10% chance a
	// Jeweller's chisel cuts a second gem at once.
	jewellersChiselDoubleChance = 0.1
)

// cutChance is the OSRS skilling success curve (Gems.Crafting
// .js cutChance, exported for tests in TS): low at level 1,
// high at 99, out of 256.
func cutChance(level, low, high int) float64 {
	clamped := level
	if clamped < 1 {
		clamped = 1
	}
	if clamped > 99 {
		clamped = 99
	}
	return (1 + math.Floor(float64(low)*float64(99-clamped)/98+float64(high)*float64(clamped-1)/98+0.5)) / 256
}

// chiselPair is the other item of a chisel use, or 0 when the
// use involves no chisel.
func (c *crafting) chiselPair(event *plugins.PluginItemOnItemEvent) int {
	chisel := c.itemID("CHISEL")
	jewellers := c.itemID("JEWELLERS_CHISEL")
	switch {
	case event.UsedItemID == chisel || event.UsedItemID == jewellers:
		return event.UsedWithItemID
	case event.UsedWithItemID == chisel || event.UsedWithItemID == jewellers:
		return event.UsedItemID
	default:
		return 0
	}
}

// gemName is the cut gem's lowercase display name.
func gemName(player *game.Player, gem GemRecipe) string {
	return strings.ToLower(player.ItemName(gem.CutID))
}

// cutOne cuts one gem: crushed on a failed roll for
// semi-precious gems, sometimes two with a Jeweller's chisel.
func (c *crafting) cutOne(player *game.Player, gem GemRecipe) bool {
	inv := player.Inventory()
	if inv == nil || !inv.Contains(gem.UncutID) || c.chiselOf(player) == 0 {
		return false
	}
	jewellers := c.chiselOf(player) == c.itemID("JEWELLERS_CHISEL")
	inv.DeleteNumber(gem.UncutID, 1)
	if gem.HasCrush {
		chance := cutChance(c.craftingLevel(player), gem.Crush.Low, gem.Crush.High)
		if jewellers {
			chance += jewellersChiselSuccessBonus
		}
		if c.roll() >= chance {
			inv.AddItem(game.NewItem(c.itemID("CRUSHED_GEM"), 1))
			c.addCraftingXp(player, gem.Crush.XP)
			animate(player, 0, game.NewSound(gemCrushedSoundID, 1, 0, 0))
			c.spam(player, fmt.Sprintf("You mis-hit the chisel and smash the %s to pieces!", gemName(player, gem)))
			return true
		}
	}
	inv.AddItem(game.NewItem(gem.CutID, 1))
	c.addCraftingXp(player, gem.XP)
	c.produced(player, gem.CutID, 1, gem.XP, "gems", nil)
	c.spam(player, fmt.Sprintf("You cut the %s.", gemName(player, gem)))
	if jewellers && inv.Contains(gem.UncutID) && c.roll() < jewellersChiselDoubleChance {
		inv.DeleteNumber(gem.UncutID, 1)
		inv.AddItem(game.NewItem(gem.CutID, 1))
		c.addCraftingXp(player, gem.XP)
	}
	return true
}

// tooLowToCut is the next-tick item box for a gem above the
// player's level.
func tooLowToCut(player *game.Player, gem GemRecipe) {
	itemStatement(player, gem.UncutID, fmt.Sprintf("You need a Crafting level of %d to cut %s.", gem.Level, gem.Plural))
}

// cutGem is a chisel on an uncut gem: the cutting menu.
func (c *crafting) cutGem(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	gem := c.tables.GemByUncut(c.chiselPair(event))
	if gem == nil || (gem.Members && !c.isMembersWorld()) {
		return
	}
	event.Handled = true
	player := event.Player
	if c.craftingLevel(player) < gem.Level {
		c.stopCrafting(player)
		recipe := *gem
		c.nextTick(player, func() { tooLowToCut(player, recipe) })
		return
	}
	c.stopCrafting(player)
	c.openMakeMenu(player, MakeMenuOptions{
		Title: gemMenuTitle, Mode: gemMenuMode,
		Items: []MakeMenuItem{{ItemID: gem.UncutID}}, MaxAmount: player.Inventory().GetAmount(gem.UncutID),
		Choose: func(_ MakeMenuItem, amount int) {
			recipe := *gem
			c.startMaking(player, MakeOptions{
				Amount: amount, StartTick: nil, FirstTick: 1, Interval: 2,
				Animation: recipe.SeqID, Kind: "gems", Sound: game.NewSound(gemCuttingSoundID, 1, 0, 0),
				Has:  func() bool { return player.Inventory().Contains(recipe.UncutID) && c.chiselOf(player) != 0 },
				Make: func() bool { return c.cutOne(player, recipe) },
			})
		},
	})
}

// ---- amethyst ----

// cutAmethyst cuts one amethyst into recipe.Amount products.
func (c *crafting) cutAmethyst(player *game.Player, recipe AmethystRecipe) bool {
	inv := player.Inventory()
	if inv == nil || !inv.Contains(c.itemID("AMETHYST")) || c.chiselOf(player) == 0 {
		return false
	}
	product := game.NewItem(recipe.ItemID, recipe.Amount)
	// With no stack to join, the amethyst's own slot takes the
	// product.
	joins := false
	for _, item := range inv.Items() {
		if item.ID() == recipe.ItemID && item.Amount() > 0 && item.Meta() == nil {
			joins = true
			break
		}
	}
	if joins && inv.GetAddableAmount(product) < recipe.Amount {
		player.SendMessage("You don't have enough inventory space.")
		return false
	}
	inv.DeleteNumber(c.itemID("AMETHYST"), 1)
	inv.AddItem(product)
	c.addCraftingXp(player, recipe.XP)
	c.produced(player, recipe.ItemID, recipe.Amount, recipe.XP, "amethyst", nil)
	return true
}

func (c *crafting) startAmethyst(player *game.Player, recipe AmethystRecipe, amount int) {
	if c.craftingLevel(player) < recipe.Level {
		// ponytail: not captured; follows the glassblowing
		// message.
		player.SendMessage(fmt.Sprintf("You need level %d Crafting to make %s.", recipe.Level, strings.ToLower(player.ItemName(recipe.ItemID))))
		return
	}
	c.startMaking(player, MakeOptions{
		Amount: amount, StartTick: nil, FirstTick: 1, Interval: 2,
		Animation: c.tables.Amethyst.SeqID, Kind: "amethyst",
		Has:  func() bool { return player.Inventory().Contains(c.itemID("AMETHYST")) && c.chiselOf(player) != 0 },
		Make: func() bool { return c.cutAmethyst(player, recipe) },
	})
}

// handleAmethyst is a chisel on amethyst: the amethyst menu.
func (c *crafting) handleAmethyst(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if c.chiselPair(event) != c.itemID("AMETHYST") || !c.isMembersWorld() {
		return
	}
	event.Handled = true
	player := event.Player
	c.stopCrafting(player)
	items := make([]MakeMenuItem, 0, len(c.tables.Amethyst.Recipes))
	for _, recipe := range availableFor(c, c.tables.Amethyst.Recipes, func(r AmethystRecipe) bool { return r.Members }) {
		items = append(items, MakeMenuItem{ItemID: recipe.ItemID})
	}
	byItem := make(map[int]AmethystRecipe, len(c.tables.Amethyst.Recipes))
	for _, recipe := range c.tables.Amethyst.Recipes {
		byItem[recipe.ItemID] = recipe
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: amethystMenuTitle, Mode: amethystMenuMode, Items: items,
		MaxAmount: player.Inventory().GetAmount(c.itemID("AMETHYST")),
		Choose: func(item MakeMenuItem, amount int) {
			if recipe, ok := byItem[item.ItemID]; ok {
				c.startAmethyst(player, recipe, amount)
			}
		},
	})
}

// AttachGems registers the Gems.Crafting.js hooks: chisel on a
// gem and chisel on amethyst, both onItemOnItem with
// noted:false. The "crafting:cut-gem" answerer is registered
// by register in crafting.go.
func (c *crafting) AttachGems(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	filter := &plugins.ItemUseFilter{Noted: &noted}
	api.OnItemOnItem(c.cutGem, filter)
	api.OnItemOnItem(c.handleAmethyst, filter)
}
