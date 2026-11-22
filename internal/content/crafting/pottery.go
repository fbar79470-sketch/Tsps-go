// Pottery.Crafting.js (229 lines, commit bfd81e3): soft clay,
// the potter's wheel and the pottery oven.
//
// Water container on clay softens it and returns the empty
// container. The wheel shapes (menu mode 0); the oven fires
// (menu mode 9). Shaping: result at ticks 2, 5, 8, ...; firing:
// put at ticks 0, 7, 14, ... and remove at 6, 13, 20, ...
// (interval 7, delay 6); firing has no level requirement.
// Crack chance is the firing curve (guaranteed from level 13);
// a cracked piece is destroyed with no product, no XP, no
// message (except cups, cracks: false). Pot lids are locked
// behind One Small Favour (quest:get-stage has no Go answerer,
// so fail-closed: locked — gap 27d). A successful firing emits
// "pottery:fired".
//
// Pottery uses its own task shape (not startMaking): the
// PotteryTask below, following the TS class, including the
// putMessage / shapeMessage / removeMessage spam lines from
// the JSON. Only BOWL and CUP carry those messages.
package crafting

import (
	"fmt"
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- constants (Pottery.Crafting.js) ----

const (
	// softClayMessage is the water-on-clay spam line (the
	// <br> is in the TS string; sent as-is).
	softClayMessage = "You mix the clay and water.<br>You now have some soft workable clay."
	// shapeMenuMode / fireMenuMode are the wheel and oven
	// menu modes (0 / 9).
	shapeMenuMode = 0
	fireMenuMode  = 9
)

// potterWheelNames / potteryOvenNames are the station names
// (the Barbarian Village wheel/oven use the capitalized
// forms; Civitas illa Fortis labels its oven "Pottery oven").
var potterWheelNames = map[string]bool{"Potter's Wheel": true, "Potter's wheel": true}
var potteryOvenNames = map[string]bool{"Pottery Oven": true, "Pottery oven": true}

// makeSoftClay is a container of water on clay: soft clay and
// the empty container.
func (c *crafting) makeSoftClay(event *plugins.PluginItemOnItemEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	waterByFull := make(map[int]int, len(c.tables.Pottery.WaterContainers))
	for _, pair := range c.tables.Pottery.WaterContainers {
		waterByFull[pair.ItemID] = pair.Amount
	}
	waterID := 0
	otherID := 0
	switch {
	case waterByFull[event.UsedItemID] != 0:
		waterID, otherID = event.UsedItemID, event.UsedWithItemID
	case waterByFull[event.UsedWithItemID] != 0:
		waterID, otherID = event.UsedWithItemID, event.UsedItemID
	default:
		return
	}
	if otherID != c.itemID("CLAY") {
		return
	}
	player := event.Player
	inv := player.Inventory()
	if inv == nil || !inv.Contains(c.itemID("CLAY")) || !inv.Contains(waterID) {
		event.Handled = true
		return
	}
	inv.DeleteNumber(c.itemID("CLAY"), 1)
	inv.DeleteNumber(waterID, 1)
	inv.AddItem(game.NewItem(c.itemID("SOFT_CLAY"), 1))
	inv.AddItem(game.NewItem(waterByFull[waterID], 1))
	c.spam(player, softClayMessage)
	event.Handled = true
}

// recipeUnlocked reports whether a pottery recipe's quest
// unlock is met (Pottery.Crafting.js recipeUnlocked): the
// quest runtime owns the saved stage. No Go answerer exists,
// so a locked recipe stays locked (gap 27d).
func (c *crafting) recipeUnlocked(player *game.Player, recipe PotteryRecipe) bool {
	if !recipe.HasUnlock {
		return true
	}
	request := &QuestStageRequest{Player: player, Quest: recipe.UnlockQuest, Stage: 0}
	c.emitEvent(QuestStageEventName, request)
	return request.Stage >= recipe.UnlockStage
}

// canShape reports whether the player may shape a recipe.
func (c *crafting) canShape(player *game.Player, recipe PotteryRecipe) bool {
	return c.craftingLevel(player) >= recipe.Level && c.recipeUnlocked(player, recipe)
}

// firingChance is the firing curve (Pottery.Crafting.js
// firingChance): Wiki's estimated statrandomlow/high
// (180/800); guaranteed from level 13. Cups are unconfirmed
// in TS, but the JSON sets cracks: false for them, so the
// curve never runs for cups.
func firingChance(level int) float64 {
	clamped := level
	if clamped < 1 {
		clamped = 1
	}
	if clamped > 99 {
		clamped = 99
	}
	chance := (1 + math.Floor(180*float64(99-clamped)/98+800*float64(clamped-1)/98+0.5)) / 256
	if chance > 1 {
		return 1
	}
	return chance
}

// makePottery makes one pottery piece (Pottery.Crafting.js
// makePottery): shaping consumes soft clay, firing consumes
// the unfired piece. stationID is the oven's object id (0
// when there is no station).
func (c *crafting) makePottery(player *game.Player, recipe PotteryRecipe, firing bool, stationID int) bool {
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	input := recipe.UnfiredID
	if !firing {
		input = c.itemID("SOFT_CLAY")
	}
	if !inv.Contains(input) || (!firing && !c.canShape(player, recipe)) {
		return false
	}
	// A shaping that makes several (a set of four cups) needs
	// room for all of them: the clay frees one slot, and the
	// rest of the set was dropped by the add ("You couldn't
	// hold all those items."), XP paid and the loop going on.
	made := 1
	if !firing {
		made = recipe.Shaped
	}
	if made > 1 && inv.GetFreeSlots()+1 < made {
		player.SendMessage("You don't have enough inventory space.")
		return false
	}
	inv.DeleteNumber(input, 1)
	if firing && recipe.Cracks && c.roll() >= firingChance(c.craftingLevel(player)) {
		return true // Cracked pottery is destroyed, with no product or XP.
	}
	outputID, amount, xp := recipe.UnfiredID, made, recipe.ShapingXP
	if firing {
		outputID, amount, xp = recipe.FiredID, 1, recipe.FiringXP
	}
	inv.AddItem(game.NewItem(outputID, amount))
	c.addCraftingXp(player, xp)
	c.produced(player, outputID, amount, xp, "pottery", nil)
	// "pottery:fired": an oven really fired a piece (a cracked
	// one returns above, and a refused menu never starts).
	if firing {
		c.emitEvent(PotteryFiredEventName, &PotteryFiredEvent{Player: player, ObjectID: stationID, ItemID: recipe.FiredID})
	}
	message := recipe.ShapeMessage
	if firing {
		message = recipe.RemoveMessage
	}
	if message != "" {
		c.spam(player, message)
	}
	return true
}

// ---- the bespoke pottery task ----

// potteryTask is one player's active shaping/firing run
// (Pottery.Crafting.js PotteryTask): a 1-tick task with its
// own tick counting (interval 3 shaping / interval 7, delay 6
// firing), station-anchored within 2 tiles. It is not a
// craftSession, so it is tracked in c.pottery and stopped by
// stopCrafting.
type potteryTask struct {
	c         *crafting
	player    *game.Player
	recipe    PotteryRecipe
	firing    bool
	station   plugins.GameObject
	stationID int
	origin    game.Location
	interval  int
	delay     int
	remaining int
	ticks     int
	stopped   bool
}

// animate plays the wheel/oven animation and sound, and (for
// firing) the put message.
func (t *potteryTask) animate() {
	if t == nil || t.c == nil {
		return
	}
	if t.firing {
		animate(t.player, t.c.tables.Pottery.OvenSeqID, game.NewSound(t.c.tables.Pottery.OvenSoundID, 1, 0, 0))
		if t.recipe.PutMessage != "" {
			t.c.spam(t.player, t.recipe.PutMessage)
		}
		return
	}
	animate(t.player, t.c.tables.Pottery.SeqID, game.NewSound(t.c.tables.Pottery.SoundID, 1, 0, 0))
}

// has reports whether another piece can be made.
func (t *potteryTask) has() bool {
	if t == nil || t.c == nil || t.player == nil || t.player.Inventory() == nil {
		return false
	}
	input := t.recipe.UnfiredID
	if !t.firing {
		input = t.c.itemID("SOFT_CLAY")
	}
	return t.player.Inventory().Contains(input) && (t.firing || t.c.canShape(t.player, t.recipe))
}

// execute runs one tick; it returns true when the task ends
// (Pottery.Crafting.js PotteryTask.execute).
func (t *potteryTask) execute() bool {
	if t == nil || t.c == nil || t.player == nil {
		return true
	}
	player := t.player
	movement := player.MovementQueue()
	skills := player.Skills()
	stationOK := t.station == nil || player.Location().Distance(t.station.Location()) <= 2
	if !player.IsRegistered() || skills == nil || skills.CurrentLevel(game.SkillHitpoints) <= 0 ||
		(movement != nil && movement.Size() > 0) || player.ForceMovement() != nil ||
		player.GetInterfaceId() != -1 || !player.Location().Equals(t.origin) ||
		!stationOK || !t.has() {
		return true
	}
	tick := t.ticks + 1
	t.ticks = tick
	if t.firing && tick%t.interval == 0 {
		t.animate()
	}
	if tick%t.interval != t.delay {
		return false
	}
	// Content may make several pieces in the one cycle
	// ("skills:actions-per-cycle").
	actions := t.c.actionsPerCycle(player, "pottery")
	for done := 0; done < actions; done++ {
		if !t.c.makePottery(player, t.recipe, t.firing, t.stationID) {
			return true
		}
		t.remaining--
		if t.remaining <= 0 || !t.has() {
			return true
		}
	}
	if !t.firing {
		t.animate()
	}
	return false
}

// stop ends the task: it clears the "crafting:task" attribute
// when it holds this task (Pottery.Crafting.js
// PotteryTask.stop).
func (t *potteryTask) stop() {
	if t == nil {
		return
	}
	t.stopped = true
	if t.c != nil && t.player != nil {
		delete(t.c.pottery, t.player)
		if current, ok := t.player.GetAttribute(CraftingTaskAttribute).(*potteryTask); ok && current == t {
			t.player.SetAttribute(CraftingTaskAttribute, nil)
		}
	}
}

// startPottery starts a shaping or firing run (Pottery
// .Crafting.js startPottery).
func (c *crafting) startPottery(player *game.Player, station plugins.GameObject, recipe PotteryRecipe, amount int, firing bool) {
	if !firing && !c.canShape(player, recipe) {
		return
	}
	c.stopCrafting(player)
	origin := player.Location()
	interval := 3
	if firing {
		interval = 7
	}
	interval = c.actionInterval(player, "pottery", interval)
	task := &potteryTask{
		c: c, player: player, recipe: recipe, firing: firing,
		station: station, origin: origin,
		interval: interval, delay: interval - 1, remaining: amount,
	}
	if station != nil {
		task.stationID = station.ID()
	}
	if !task.has() {
		if firing {
			statement(player, fmt.Sprintf("You don't have any %s which need firing.", recipe.Plural))
		}
		return
	}
	task.animate()
	player.SetAttribute(CraftingTaskAttribute, task)
	c.pottery[player] = task
	tm := c.taskManager()
	if tm == nil {
		return
	}
	runner := game.NewTask(1, func(cycle int64) bool {
		if task.execute() {
			task.stop()
			return true
		}
		return false
	})
	runner.Name = "PotteryTask"
	runner.Key = player.Index
	tm.Schedule(runner)
}

// openPotteryMenu opens the shaping or firing menu for a
// station.
func (c *crafting) openPotteryMenu(player *game.Player, object plugins.GameObject, firing bool) {
	c.stopCrafting(player)
	inv := player.Inventory()
	recipes := availableFor(c, c.tables.Pottery.Recipes, func(r PotteryRecipe) bool { return r.Members })
	unlocked := make([]PotteryRecipe, 0, len(recipes))
	for _, recipe := range recipes {
		// Both menus leave pot lids out until One Small Favour
		// teaches them.
		if c.recipeUnlocked(player, recipe) {
			unlocked = append(unlocked, recipe)
		}
	}
	maxAmount := 0
	if inv != nil {
		if firing {
			for _, recipe := range unlocked {
				if amount := inv.GetAmount(recipe.UnfiredID); amount > maxAmount {
					maxAmount = amount
				}
			}
		} else {
			maxAmount = inv.GetAmount(c.itemID("SOFT_CLAY"))
		}
	}
	if maxAmount == 0 {
		if firing {
			player.SendMessage("You have no unfired pottery to fire.")
		} else {
			statement(player, "You don't have anything suitable to craft with.")
		}
		return
	}
	byUnfired := make(map[int]PotteryRecipe, len(unlocked))
	items := make([]MakeMenuItem, 0, len(unlocked))
	for _, recipe := range unlocked {
		byUnfired[recipe.UnfiredID] = recipe
		label := capitalizeFirst(recipe.Name)
		if !firing && recipe.ShapeLabel != "" {
			label = recipe.ShapeLabel
		}
		items = append(items, MakeMenuItem{ItemID: recipe.UnfiredID, Label: label})
	}
	title := "How many do you wish to make?"
	mode := shapeMenuMode
	if firing {
		title, mode = "What would you like to fire in the oven?", fireMenuMode
	}
	c.openMakeMenu(player, MakeMenuOptions{
		Title: title, Mode: mode, Items: items, MaxAmount: maxAmount,
		Choose: func(item MakeMenuItem, amount int) {
			if recipe, ok := byUnfired[item.ItemID]; ok {
				c.startPottery(player, object, recipe, amount, firing)
			}
		},
	})
}

// usePotterWheel is Use on a potter's wheel: the shaping menu.
func (c *crafting) usePotterWheel(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || c.tables == nil {
		return false
	}
	c.openPotteryMenu(event.Player, event.Object, false)
	return true
}

// usePotteryOven is Fire on a pottery oven: the firing menu.
func (c *crafting) usePotteryOven(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || c.tables == nil {
		return false
	}
	c.openPotteryMenu(event.Player, event.Object, true)
	return true
}

// handlePotteryItemOnObject is soft clay on a wheel, or
// unfired pottery on a pottery oven.
func (c *crafting) handlePotteryItemOnObject(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	name := c.objectNameOf(event.ObjectID)
	if potterWheelNames[name] && event.ItemID == c.itemID("SOFT_CLAY") {
		event.Handled = true
		c.openPotteryMenu(event.Player, event.Object, false)
		return
	}
	if potteryOvenNames[name] {
		for _, recipe := range c.tables.Pottery.Recipes {
			if recipe.UnfiredID == event.ItemID {
				event.Handled = true
				c.openPotteryMenu(event.Player, event.Object, true)
				return
			}
		}
	}
}

// AttachPottery registers the Pottery.Crafting.js hooks: water
// on clay, soft clay / unfired pottery on the stations, and
// Use / Fire on the wheel and oven (both capitalizations).
func (c *crafting) AttachPottery(api *plugins.Api) {
	if api == nil {
		return
	}
	noted := false
	filter := &plugins.ItemUseFilter{Noted: &noted}
	api.OnItemOnItem(c.makeSoftClay, filter)
	api.OnItemOnObject(c.handlePotteryItemOnObject, filter)
	api.OnObjectInteractionNamed("Potter's Wheel", map[string]plugins.ObjectInteractionHandler{"Use": c.usePotterWheel})
	api.OnObjectInteractionNamed("Potter's wheel", map[string]plugins.ObjectInteractionHandler{"Use": c.usePotterWheel})
	api.OnObjectInteractionNamed("Pottery Oven", map[string]plugins.ObjectInteractionHandler{"Fire": c.usePotteryOven})
	api.OnObjectInteractionNamed("Pottery oven", map[string]plugins.ObjectInteractionHandler{"Fire": c.usePotteryOven})
}
