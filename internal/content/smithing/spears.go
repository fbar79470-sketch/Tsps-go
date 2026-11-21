// Spears.Smithing.js: barbarian smithing — a bar and a
// matching log make a spear or a hasta, only at a barbarian
// anvil (Otto's Grotto and Barbarian Village), which works
// nothing else.
//
// Captured (rsprox recs 1530, 1533): Smith says "You should
// select a suitable metal bar from your inventory and use it
// on the anvil."; without a hammer, "You need a hammer in
// order to smith here."
package smithing

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- barbarian anvil constants (Spears.Smithing.js) ----

const (
	barbarianAnvilName = "Barbarian anvil"
	// barbarianTrainedAttribute is the per-player training flag.
	barbarianTrainedAttribute = "barbarian-training:smithing"
	// spearIntervalTicks is the spear loop interval (4 ticks).
	spearIntervalTicks = 4
	selectBarMessage   = "You should select a suitable metal bar from your inventory and use it on the anvil."
	noHammerBarbarian  = "You need a hammer in order to smith here."
	unfamiliarMessage  = "This anvil is unfamiliar and barbaric, you can't seem to use it."
)

// spearTrained reports whether the player may use a barbarian
// anvil (Spears.Smithing.js trained): it returns true when no
// "Barbarian Training" quest is registered, so it is OPEN
// until that quest exists (fail-open, unlike most quest gates
// here). Go has no quest registry at all, so the lock is
// always open; when Barbarian Training ports, trained() must
// consult it and the player's attribute (gap 26c).
func spearTrained(player *game.Player) bool {
	return true
}

// makeSpear makes one spear or hasta
// (Spears.Smithing.js makeOne).
func (s *smithing) makeSpear(player *game.Player, recipe *SpearRecipe, itemID int) bool {
	if player == nil || recipe == nil || player.Inventory() == nil {
		return false
	}
	if !hasHammer(player) {
		player.SendMessage(noHammerBarbarian)
		return false
	}
	if s.smithingLevel(player) < recipe.Level {
		messageBox(player, "You need a Smithing level of "+itoa(recipe.Level)+" to make that.")
		return false
	}
	inv := player.Inventory()
	if !inv.Contains(recipe.BarID) || !inv.Contains(recipe.LogID) {
		bar := strings.ToLower(player.ItemName(recipe.BarID))
		log := strings.ToLower(player.ItemName(recipe.LogID))
		if bar == "" {
			bar = "bar"
		}
		if log == "" {
			log = "logs"
		}
		player.SendMessage("You need a " + bar + " and some " + log + " to make that.")
		return false
	}
	inv.DeleteNumber(recipe.BarID, 1)
	inv.DeleteNumber(recipe.LogID, 1)
	inv.Add(game.NewItem(itemID, 1), true)
	player.PerformAnimation(humanSmithingSeq, 0)
	addXP(player, game.SkillSmithing, recipe.XP)
	s.emitEvent("smithing:success", &smithingSuccessEvent{
		Player: player, Skill: game.SkillSmithing, ItemID: itemID, Amount: 1,
		BarID: recipe.BarID, Bars: 1,
	})
	return true
}

// startSpears starts a spear loop (Spears.Smithing.js start).
func (s *smithing) startSpears(player *game.Player, recipe *SpearRecipe, itemID, amount int) {
	if player == nil || recipe == nil || amount <= 0 || tables == nil || player.Inventory() == nil {
		return
	}
	inv := player.Inventory()
	most := inv.GetAmount(recipe.BarID)
	if logs := inv.GetAmount(recipe.LogID); logs < most {
		most = logs
	}
	if most <= 0 {
		return
	}
	s.startSession(player, sessionOptions{
		kind:      "spears",
		skill:     game.SkillSmithing,
		remaining: min(amount, most),
		interval:  constInterval(spearIntervalTicks),
		step:      func(smith *game.Player) bool { return s.makeSpear(smith, recipe, itemID) },
	})
}

// spearSmith is "Smith" on a barbarian anvil: it only says
// which bar to use (Spears.Smithing.js smith).
func (s *smithing) spearSmith(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	if spearTrained(event.Player) {
		messageBox(event.Player, selectBarMessage)
	} else {
		messageBox(event.Player, unfamiliarMessage)
	}
	return true
}

// spearUseBar is a bar used on a barbarian anvil
// (Spears.Smithing.js useBar).
func (s *smithing) spearUseBar(event *plugins.ItemOnObjectEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	if s.objectNameOf(event.ObjectID) != barbarianAnvilName {
		return false
	}
	var recipe *SpearRecipe
	for i := range tables.Spears {
		if tables.Spears[i].BarID == event.ItemID {
			recipe = &tables.Spears[i]
			break
		}
	}
	if recipe == nil {
		return false
	}
	event.Handled = true
	player := event.Player
	if !spearTrained(player) {
		messageBox(player, unfamiliarMessage)
		return true
	}
	if !hasHammer(player) {
		player.SendMessage(noHammerBarbarian)
		return true
	}
	most := 1
	if inv := player.Inventory(); inv != nil {
		most = inv.GetAmount(recipe.BarID)
		if logs := inv.GetAmount(recipe.LogID); logs < most {
			most = logs
		}
		if most < 1 {
			most = 1
		}
	}
	menu := game.NewCreationMenu(
		"What would you like to make?",
		[]int{recipe.SpearID, recipe.HastaID},
		func(itemID, amount int) { s.startSpears(player, recipe, itemID, amount) },
		game.CreationMenuOptions{MaxAmount: &most},
	)
	if player.Session == nil {
		return true
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return true
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
	return true
}

// registerSpears wires the barbarian anvil hooks
// (Spears.Smithing.js attach).
func (s *smithing) registerSpears(api *plugins.Api) {
	api.PersistAttribute(barbarianTrainedAttribute, false)
	api.OnObjectInteractionNamed(barbarianAnvilName, map[string]plugins.ObjectInteractionHandler{
		"Smith": s.spearSmith,
	})
	api.OnItemOnObject(func(event *plugins.ItemOnObjectEvent) { s.spearUseBar(event) }, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)})
}
