// GemSickles.Crafting.js (101 lines, commit bfd81e3): a gem on
// a silver sickle (b) — the emerald, ruby and diamond sickles
// of the Myreque quests.
//
// The use opens a Yes/No chat menu, "Craft a ruby sickle?".
// On Yes, the gem and the sickle go and the new sickle takes
// the first empty slot, with the gem's cutting animation, sound
// 2586 and the Crafting XP, and an item box: "You embed the
// ruby into the sickle." XP: 20/26/32, a chisel needed. The
// quest that asks for the sickle decides when it can be made:
// this unit asks "crafting:gem-sickle-allowed" {player,
// product, allowed} and leaves the use alone unless one says
// yes. No Go answerer exists, so the gate fails closed (gap
// 27d) and the use does nothing — but the handler and the gate
// are ported, so a future answerer switches it on.
//
// The Yes/No prompt uses Api.SendMultiChatboxPrompt (the seam
// exists in Go — see jewellery.go); with no registry installed
// the prompt cannot show and the use does nothing after the
// gate passes.
package crafting

import (
	"fmt"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (GemSickles.Crafting.js) ----

const (
	// silverSickleBName is the sickle's display name, as the
	// TS named hook matches it.
	silverSickleBName = "Silver sickle (b)"
	// gemCuttingSoundID is the craft sound (2586).
	gemSickleSoundID = 2586
)

// gemSickleAllowed reports whether the quest that wants this
// sickle lets the player make it now (GemSickles.Crafting.js
// allowed).
func (c *crafting) gemSickleAllowed(player *game.Player, product int) bool {
	request := &GemSickleAllowedRequest{Player: player, Product: product, Allowed: false}
	c.emitEvent(GemSickleAllowedEventName, request)
	return request.Allowed
}

// hasSickleChisel reports whether the player carries a chisel.
func (c *crafting) hasSickleChisel(player *game.Player) bool {
	return c.chiselOf(player) != 0
}

// gemSickleByGem returns the gem-sickle recipe for a cut gem
// item id, or nil. The JSON names the gem by display name
// ("Ruby"); the gem item is the cut gem of that name.
func (c *crafting) gemSickleByGem(gemID int) *GemSickleRecipe {
	if c.tables == nil {
		return nil
	}
	for i := range c.tables.GemSickles {
		recipe := &c.tables.GemSickles[i]
		name := ""
		switch recipe.GemName {
		case "Emerald":
			name = "EMERALD"
		case "Ruby":
			name = "RUBY"
		case "Diamond":
			name = "DIAMOND"
		default:
			continue
		}
		if c.itemID(name) == gemID {
			return recipe
		}
	}
	return nil
}

// craftGemSickle is the Yes branch (GemSickles.Crafting.js
// craft): checked again, as the gem, the sickle or the chisel
// may have gone while the menu was open.
func (c *crafting) craftGemSickle(player *game.Player, recipe GemSickleRecipe, gemID, sickleID int) {
	inv := player.Inventory()
	if inv == nil || !inv.Contains(gemID) || !inv.Contains(sickleID) || !c.hasSickleChisel(player) {
		return
	}
	if !c.gemSickleAllowed(player, recipe.ProductID) {
		return
	}
	inv.DeleteNumber(gemID, 1)
	inv.DeleteNumber(sickleID, 1)
	inv.Adds(recipe.ProductID, 1)
	animate(player, recipe.SeqID, game.NewSound(gemSickleSoundID, 1, 0, 0))
	c.addCraftingXp(player, recipe.XP)
	// Not "skills:produced": a quest item, so no league relic
	// doubles it.
	c.emitEvent(GemSickleMadeEventName, &GemSickleMadeEvent{Player: player, Product: recipe.ProductID})
	itemStatement(player, recipe.ProductID, fmt.Sprintf("You embed the %s into the sickle.", recipe.Name))
}

// useGemOnSickle is a gem on a silver sickle (b).
func (c *crafting) useGemOnSickle(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	if !c.isMembersWorld() {
		return
	}
	player := event.Player
	sickleID := c.itemID("SILVER_SICKLE_B_")
	var gemID int
	switch {
	case event.UsedItemID == sickleID:
		gemID = event.UsedWithItemID
	case event.UsedWithItemID == sickleID:
		gemID = event.UsedItemID
	default:
		return
	}
	recipe := c.gemSickleByGem(gemID)
	if recipe == nil || !c.gemSickleAllowed(player, recipe.ProductID) {
		return
	}
	event.Handled = true
	if !c.hasSickleChisel(player) {
		// Ours: no capture has the use without a chisel.
		player.SendMessage(fmt.Sprintf("You need a chisel to embed the %s into the sickle.", recipe.Name))
		return
	}
	recipeCopy := *recipe
	c.sendPrompt(player, fmt.Sprintf("Craft %s %s sickle?", recipeCopy.Article, recipeCopy.Name),
		"Yes.", plugins.PromptCallback(func(*game.Player, int, string) { c.craftGemSickle(player, recipeCopy, gemID, sickleID) }),
		"No.", plugins.PromptCallback(func(*game.Player, int, string) {}),
	)
}

// AttachGemSickles registers the GemSickles.Crafting.js hook:
// gem on a silver sickle (b), onItemOnItem with noted:false.
// (TS registers one named hook per gem; Go matches by id,
// which is equivalent — the gem item ids are the cut gems of
// the JSON's gem names.)
func (c *crafting) AttachGemSickles(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	api.OnItemOnItem(c.useGemOnSickle, &plugins.ItemUseFilter{Noted: &noted})
}
