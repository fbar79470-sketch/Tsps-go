// Cannonballs.Smithing.js: bars poured into an ammo mould
// at any furnace, 4 balls a bar; the double ammo mould takes
// two bars a time for 8 balls at the same speed. Every
// cannonball needs Dwarf Cannon. The furnace's Smelt menu
// offers a cannonball for each bar the player carries (with a
// mould), and a bar used on a furnace opens it too.
//
// Not captured: the Wiki gives 8 ticks for the first batch and
// 10 between, the bone-burying animation (human_pickupfloor),
// and the experience; the messages are the game's
// long-standing ones, not checked against a capture
// (docs/smithing.md).
package smithing

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- cannonball constants (Cannonballs.Smithing.js) ----

const (
	// cannonballFirstTicks is the first batch's delay (8 ticks).
	cannonballFirstTicks = 8
	// cannonballIntervalTicks is the delay between batches
	// (10 ticks).
	cannonballIntervalTicks = 10
	// ballsPerBar is the balls one bar pours (4).
	ballsPerBar = 4
	// dwarfCannonQuest is the quest key for Dwarf Cannon.
	dwarfCannonQuest = "dwarf_cannon"
)

// mouldBars is the bars a mould takes at once: 2 for the
// double ammo mould, 1 for the ammo mould, 0 for none
// (Cannonballs.Smithing.js mouldOf).
func mouldBars(player *game.Player) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	inv := player.Inventory()
	if inv.Contains(doubleAmmoMouldItemID) {
		return 2
	}
	if inv.Contains(ammoMouldItemID) {
		return 1
	}
	return 0
}

// cannonballFor is the cannonball a bar pours into, or nil.
func cannonballFor(barID int) *CannonballRecipe {
	if tables == nil {
		return nil
	}
	for i := range tables.Cannonballs {
		if tables.Cannonballs[i].BarID == barID {
			return &tables.Cannonballs[i]
		}
	}
	return nil
}

// pour pours one batch of cannonballs
// (Cannonballs.Smithing.js pour).
func (s *smithing) pour(player *game.Player, cannonball *CannonballRecipe) bool {
	if player == nil || cannonball == nil || player.Inventory() == nil {
		return false
	}
	mould := mouldBars(player)
	inv := player.Inventory()
	bars := mould
	if have := inv.GetAmount(cannonball.BarID); have < bars {
		bars = have
	}
	if mould == 0 {
		player.SendMessage("You need an ammo mould to make cannonballs.")
		return false
	}
	if bars <= 0 {
		name := strings.ToLower(player.ItemName(cannonball.BarID))
		if name == "" {
			name = "bar"
		}
		player.SendMessage("You have run out of " + name + "s.")
		return false
	}
	inv.DeleteNumber(cannonball.BarID, bars)
	balls := bars * ballsPerBar
	inv.Add(game.NewItem(cannonball.BallID, balls), true)
	addXP(player, game.SkillSmithing, cannonball.XPPerBar*float64(bars))
	player.SendMessage("The molten metal cools slowly to form " + itoa(balls) + " cannonballs.")
	s.emitEvent("smithing:success", &smithingSuccessEvent{
		Player: player, Skill: game.SkillSmithing, ItemID: cannonball.BallID,
		Amount: balls, BarID: cannonball.BarID, Bars: bars,
	})
	s.produced(player, game.SkillSmithing, cannonball.BallID, balls, cannonball.XPPerBar*float64(bars), "cannonballs")
	return true
}

// beginCannonballs opens a pour cycle
// (Cannonballs.Smithing.js begin).
func (s *smithing) beginCannonballs(player *game.Player, cannonball *CannonballRecipe) bool {
	if player == nil || cannonball == nil || player.Inventory() == nil {
		return false
	}
	if player.Inventory().GetAmount(cannonball.BarID) <= 0 || mouldBars(player) == 0 {
		return s.pour(player, cannonball)
	}
	metal := metalName(player, cannonball.BarID)
	if metal == "" {
		metal = "metal"
	}
	player.SendMessage("You heat the " + metal + " bar into a liquid state.")
	player.SendMessage("You pour the molten metal into your cannonball mould.")
	player.PerformAnimation(humanPickupFloorSeq, 0)
	return true
}

// startCannonballs starts a cannonball loop
// (Cannonballs.Smithing.js start).
func (s *smithing) startCannonballs(player *game.Player, cannonball *CannonballRecipe, amount int) bool {
	if player == nil || cannonball == nil || amount <= 0 || tables == nil {
		return false
	}
	if !s.membersWorld {
		return false
	}
	if !s.questComplete(player, dwarfCannonQuest) {
		player.SendMessage("You need to complete " + questName(dwarfCannonQuest) + " to make cannonballs.")
		return false
	}
	if s.smithingLevel(player) < cannonball.Level {
		metal := metalName(player, cannonball.BarID)
		if metal == "" {
			metal = "metal"
		}
		messageBox(player, "You need a Smithing level of "+itoa(cannonball.Level)+" to make "+metal+" cannonballs.")
		return false
	}
	mould := mouldBars(player)
	most := 0
	if mould > 0 && player.Inventory() != nil {
		most = (player.Inventory().GetAmount(cannonball.BarID) + mould - 1) / mould
	}
	if most <= 0 {
		return false
	}
	s.startSession(player, sessionOptions{
		kind:        "cannonballs",
		skill:       game.SkillSmithing,
		remaining:   min(amount, most),
		interval:    constInterval(cannonballIntervalTicks),
		finishDelay: cannonballFirstTicks,
		begin:       func(smith *game.Player) bool { return s.beginCannonballs(smith, cannonball) },
		step:        func(smith *game.Player) bool { return s.pour(smith, cannonball) },
	})
	return true
}

// cannonballMenuEntries is the Smelt menu's cannonball
// entries: one per bar carried, with a mould
// (Cannonballs.Smithing.js menuEntries).
func (s *smithing) cannonballMenuEntries(player *game.Player) []smeltMenuEntry {
	if player == nil || tables == nil {
		return nil
	}
	mould := mouldBars(player)
	if mould == 0 || !s.membersWorld {
		return nil
	}
	inv := player.Inventory()
	if inv == nil {
		return nil
	}
	var entries []smeltMenuEntry
	for i := range tables.Cannonballs {
		cannonball := &tables.Cannonballs[i]
		if !inv.Contains(cannonball.BarID) {
			continue
		}
		entries = append(entries, smeltMenuEntry{
			itemID: cannonball.BallID,
			most:   (inv.GetAmount(cannonball.BarID) + mould - 1) / mould,
			start:  func(smith *game.Player, amount int) { s.startCannonballs(smith, cannonball, amount) },
		})
	}
	return entries
}

// cannonballsUseOnFurnace is a bar used on a furnace, with a
// mould: it opens a menu of its cannonball
// (Cannonballs.Smithing.js useOnFurnace). True when handled.
func (s *smithing) cannonballsUseOnFurnace(event *plugins.ItemOnObjectEvent) bool {
	if event == nil || event.Player == nil || tables == nil {
		return false
	}
	cannonball := cannonballFor(event.ItemID)
	if cannonball == nil || mouldBars(event.Player) == 0 {
		return false
	}
	event.Handled = true
	var entries []smeltMenuEntry
	for _, entry := range s.cannonballMenuEntries(event.Player) {
		if entry.itemID == cannonball.BallID {
			entries = append(entries, entry)
		}
	}
	s.openMenu(event.Player, entries)
	return true
}

// registerCannonballs wires the cannonball menu extras
// (Cannonballs.Smithing.js attach: Smelting.addMenuEntries).
func (s *smithing) registerCannonballs(api *plugins.Api) {
	s.addMenuExtra(s.cannonballMenuEntries)
}
