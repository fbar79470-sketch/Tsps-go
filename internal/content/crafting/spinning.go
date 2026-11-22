// Spinning.Crafting.js (200 lines, commit bfd81e3): spinning
// wheels and looms.
//
// The wheel's Spin option opens skillmulti mode 16, "What
// would you like to spin?" ("How many would you like to spin?"
// with one product); the loom's Weave option opens mode 35,
// "How many would you like to weave?". Spinning: first item
// tick 2, then every 3 ticks, anim 13139
// (human_spinningwheel_60) + wheel anim 466 + sound 2590,
// replayed per item. Weaving: first item tick 3, then every 3,
// anim 2270 (farming_useloom) + sound 2587 (delay 2). Neither
// sends a message. Using a material on the wheel or loom opens
// the same menu.
//
// The bonus: makeOne emits "crafting:spun" {player, itemId,
// amount: 1} AFTER adding the item and the XP, for spinning
// only (not weaving), so a carried bow string spool
// (Fletching) can take spun bow strings. The Go emit is
// &fletching.CraftingSpunEvent{Player, ItemID, Amount: 1} —
// the same order (item in inventory first, then emit), so the
// spool's inventory check sees the bow string.
//
// The Seers' Village 2-tick flax rule needs diary:is-complete,
// which has no Go answerer: spinTicks asks anyway (fail-closed)
// and always returns 3 (gap 27f).
//
// The wheel's own animation is sent through
// protocol.EncodeLocAnim to the spinning player only: Go has
// no GameObject.performAnimation fan-out (protocol/loc.go), so
// nearby viewers do not see the wheel turn — recorded here,
// not silently.
package crafting

import (
	"fmt"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"

	"tsps-go/internal/content/fletching"
)

// ---- constants (Spinning.Crafting.js) ----

const (
	// spinMenuTitle / spinOneMenuTitle / spinMenuMode are the
	// wheel menu titles and mode (16).
	spinMenuTitle    = "What would you like to spin?"
	spinOneMenuTitle = "How many would you like to spin?"
	spinMenuMode     = 16
	// weaveMenuTitle / weaveMenuMode are the loom menu title
	// and mode (35).
	weaveMenuTitle = "How many would you like to weave?"
	weaveMenuMode  = 35
	// spinTicksValue is the ticks between spins (3); the
	// Seers' diary rule would make it 2 for flax there.
	spinTicksValue      = 3
	seersDiarySpinTicks = 2
)

// spinningWheelNames are the wheel names (most wheels are
// "Spinning wheel", a few "Spinning Wheel").
var spinningWheelNames = map[string]bool{"Spinning wheel": true, "Spinning Wheel": true}

// seersWheel is the Seers' Village spinning wheel tile
// (2710, 3471, 1).
var seersWheel = game.Location{X: 2710, Y: 3471, Z: 1}

// spinStation is the station context a spin/weave run holds
// (Spinning.Crafting.js stationContext): the station object
// and its tile, and the player's tile when the menu opened. Go
// has no getTeleportSerial / getArea, so the snapshot holds
// locations only (the smithing stationSnapshot gap 26g shape):
// a teleport that lands back on the same tile does not stop a
// run.
type spinStation struct {
	stationObjectID int
	location        game.Location
	startLocation   game.Location
}

// stationContext snapshots a station, or nil when there is no
// object (remainsAtStation is then always true, as in TS).
func stationContext(player *game.Player, station plugins.GameObject) *spinStation {
	if player == nil || station == nil {
		return nil
	}
	return &spinStation{
		stationObjectID: station.ID(),
		location:        station.Location(),
		startLocation:   player.Location(),
	}
}

// remainsAtStation reports whether the player is still at the
// station the run started at.
func remainsAtStation(player *game.Player, context *spinStation) bool {
	if context == nil || player == nil {
		return true
	}
	return player.Location().Equals(context.startLocation)
}

// carriedMaterial is the first of a recipe's materials carried
// in the amount one item needs, or 0.
func carriedMaterial(player *game.Player, recipe SpinningRecipe) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	inv := player.Inventory()
	for _, id := range recipe.MaterialIDs {
		if inv.GetAmount(id) >= recipe.Amount {
			return id
		}
	}
	return 0
}

// carriedWeavingMaterial is carriedMaterial over a weaving
// recipe (same shape, different table type).
func carriedWeavingMaterial(player *game.Player, recipe WeavingRecipe) int {
	if player == nil || player.Inventory() == nil {
		return 0
	}
	inv := player.Inventory()
	for _, id := range recipe.MaterialIDs {
		if inv.GetAmount(id) >= recipe.Amount {
			return id
		}
	}
	return 0
}

// canGrantAfterConsumption reports whether the product fits
// once its material is consumed (the smithing preview shape).
func (c *crafting) canGrantAfterConsumption(player *game.Player, itemID, materialID, amount int) bool {
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	preview := game.NewInventory(player)
	preview.SetItems(inv.Items())
	preview.Deleted(materialID, amount, false)
	return preview.GetAddableAmount(game.NewItem(itemID, 1)) >= 1
}

// makeOne makes one spun or woven item (Spinning.Crafting.js
// makeOne). A spun one is announced as "crafting:spun" after
// the item and the XP land, for spinning only.
func (c *crafting) makeOne(player *game.Player, itemID, materialID, amount int, xp float64, spinning bool, station *spinStation) bool {
	inv := player.Inventory()
	if inv == nil || materialID == 0 || !remainsAtStation(player, station) ||
		!c.canGrantAfterConsumption(player, itemID, materialID, amount) {
		return false
	}
	inv.DeleteNumber(materialID, amount)
	inv.AddItem(game.NewItem(itemID, 1))
	c.addCraftingXp(player, xp)
	if spinning {
		c.emitEvent(CraftingSpunEventName, &fletching.CraftingSpunEvent{Player: player, ItemID: itemID, Amount: 1})
	}
	kind := "weaving"
	if spinning {
		kind = "spinning"
	}
	c.produced(player, itemID, 1, xp, kind, nil)
	return true
}

// spinTicks is the ticks between spins (Spinning.Crafting.js
// spinTicks): 3, or 2 at the Seers' Village wheel for flax
// with the Kandarin medium diary complete. The diary query
// has no Go answerer, so it always answers incomplete and
// the rule never applies (gap 27f).
func (c *crafting) spinTicks(player *game.Player, station plugins.GameObject, materialIDs []int) int {
	flax := c.itemID("FLAX")
	isFlax := false
	for _, id := range materialIDs {
		if id == flax {
			isFlax = true
			break
		}
	}
	if station == nil || !isFlax {
		return spinTicksValue
	}
	at := station.Location()
	if at.X != seersWheel.X || at.Y != seersWheel.Y || at.Z != seersWheel.Z {
		return spinTicksValue
	}
	request := &DiaryCompleteRequest{Player: player, Diary: "kandarin", Tier: "medium", Complete: false}
	c.emitEvent(DiaryCompleteEventName, request)
	if request.Complete {
		return seersDiarySpinTicks
	}
	return spinTicksValue
}

// sendWheelAnimation plays the wheel's own animation
// (Spinning.Crafting.js onAnimate: sendObjectAnimation with
// the wheel seq). Go has no object-animation fan-out, so the
// frame goes to the spinning player only (see the file
// header).
func sendWheelAnimation(player *game.Player, station plugins.GameObject, animationID int) {
	if player == nil || station == nil || player.Session == nil || animationID == 0 {
		return
	}
	loc := station.Location()
	player.Session.SendPacket(protocol.EncodeLocAnim(
		station.ID(), loc.X, loc.Y, loc.Z, station.Type(), station.Face(), animationID))
}

// startSpinning starts a spinning or weaving run
// (Spinning.Crafting.js start).
// startSpinning starts a spinning or weaving run
// (Spinning.Crafting.js start). materialIDs is the recipe's
// materials in preference order; perItem is how much of one
// material one item takes; count is how many to make.
func (c *crafting) startSpinning(player *game.Player, station plugins.GameObject, itemID int, materialIDs []int, perItem, level int, xp float64, count int, spinning bool, provenance *spinStation) {
	if !remainsAtStation(player, provenance) {
		return
	}
	if c.craftingLevel(player) < level {
		// ponytail: not captured; the live text may differ.
		player.SendMessage(fmt.Sprintf("You need a Crafting level of %d to make %s.", level, withArticle(strings.ToLower(player.ItemName(itemID)))))
		return
	}
	carried := func() int {
		if player == nil || player.Inventory() == nil {
			return 0
		}
		inv := player.Inventory()
		for _, id := range materialIDs {
			if inv.GetAmount(id) >= perItem {
				return id
			}
		}
		return 0
	}
	zero := 0
	if spinning {
		c.startMaking(player, MakeOptions{
			Amount: count, StartTick: &zero, FirstTick: 2, Interval: c.spinTicks(player, station, materialIDs),
			Animation: c.tables.Spinning.SeqID, Sound: game.NewSound(c.tables.Spinning.SoundID, 1, 0, 0),
			Kind:      "spinning",
			OnAnimate: func() { sendWheelAnimation(player, station, c.tables.Spinning.WheelSeqID) },
			Has: func() bool {
				return remainsAtStation(player, provenance) && carried() != 0
			},
			Make: func() bool {
				return c.makeOne(player, itemID, carried(), perItem, xp, true, provenance)
			},
		})
		return
	}
	c.startMaking(player, MakeOptions{
		Amount: count, StartTick: &zero, FirstTick: 3, Interval: 3,
		Animation: c.tables.Weaving.SeqID, Sound: game.NewSound(c.tables.Weaving.SoundID, 1, c.tables.Weaving.SoundDelay, 0),
		Kind: "weaving",
		Has: func() bool {
			return remainsAtStation(player, provenance) && carried() != 0
		},
		Make: func() bool {
			return c.makeOne(player, itemID, carried(), perItem, xp, false, provenance)
		},
	})
}

// openSpinMenu opens the spinning or weaving menu for a
// station.
func (c *crafting) openSpinMenu(player *game.Player, station plugins.GameObject, spinning bool) {
	provenance := stationContext(player, station)
	inv := player.Inventory()
	if spinning {
		recipes := availableFor(c, c.tables.Spinning.Recipes, func(r SpinningRecipe) bool { return r.Members })
		var offered []SpinningRecipe
		for _, recipe := range recipes {
			if carriedMaterial(player, recipe) != 0 {
				offered = append(offered, recipe)
			}
		}
		if len(offered) == 0 {
			// ponytail: not captured; the live text may differ.
			player.SendMessage("You have nothing to spin.")
			return
		}
		maxAmount := 0
		for _, recipe := range offered {
			most := 0
			for _, id := range recipe.MaterialIDs {
				if amount := inv.GetAmount(id); amount > most {
					most = amount
				}
			}
			if count := most / recipe.Amount; count > maxAmount {
				maxAmount = count
			}
		}
		items := make([]MakeMenuItem, 0, len(offered))
		for _, recipe := range offered {
			items = append(items, MakeMenuItem{ItemID: recipe.ItemID})
		}
		byItem := make(map[int]SpinningRecipe, len(offered))
		for _, recipe := range offered {
			byItem[recipe.ItemID] = recipe
		}
		title := spinMenuTitle
		if len(offered) == 1 {
			title = spinOneMenuTitle
		}
		c.stopCrafting(player)
		c.openMakeMenu(player, MakeMenuOptions{
			Title: title, Mode: spinMenuMode, Items: items, MaxAmount: maxAmount,
			Choose: func(item MakeMenuItem, amount int) {
				if recipe, ok := byItem[item.ItemID]; ok {
					c.startSpinning(player, station, recipe.ItemID, recipe.MaterialIDs, recipe.Amount, recipe.Level, recipe.XP, amount, true, provenance)
				}
			},
		})
		return
	}
	recipes := availableFor(c, c.tables.Weaving.Recipes, func(r WeavingRecipe) bool { return r.Members })
	var offered []WeavingRecipe
	for _, recipe := range recipes {
		if carriedWeavingMaterial(player, recipe) != 0 {
			offered = append(offered, recipe)
		}
	}
	if len(offered) == 0 {
		// ponytail: not captured; the live text may differ.
		player.SendMessage("You have nothing to weave.")
		return
	}
	maxAmount := 0
	for _, recipe := range offered {
		most := 0
		for _, id := range recipe.MaterialIDs {
			if amount := inv.GetAmount(id); amount > most {
				most = amount
			}
		}
		if count := most / recipe.Amount; count > maxAmount {
			maxAmount = count
		}
	}
	items := make([]MakeMenuItem, 0, len(offered))
	for _, recipe := range offered {
		items = append(items, MakeMenuItem{ItemID: recipe.ItemID})
	}
	byItem := make(map[int]WeavingRecipe, len(offered))
	for _, recipe := range offered {
		byItem[recipe.ItemID] = recipe
	}
	c.stopCrafting(player)
	c.openMakeMenu(player, MakeMenuOptions{
		Title: weaveMenuTitle, Mode: weaveMenuMode, Items: items, MaxAmount: maxAmount,
		Choose: func(item MakeMenuItem, amount int) {
			if recipe, ok := byItem[item.ItemID]; ok {
				c.startSpinning(player, station, recipe.ItemID, recipe.MaterialIDs, recipe.Amount, recipe.Level, recipe.XP, amount, false, provenance)
			}
		},
	})
}

// spin is Spin on a spinning wheel: the spinning menu.
func (c *crafting) spin(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || c.tables == nil {
		return false
	}
	c.openSpinMenu(event.Player, event.Object, true)
	return true
}

// weave is Weave on a loom: the weaving menu.
func (c *crafting) weave(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || c.tables == nil {
		return false
	}
	c.openSpinMenu(event.Player, event.Object, false)
	return true
}

// handleMaterialOnStation is a material used on a wheel or
// loom: the same menu, when the station takes it.
func (c *crafting) handleMaterialOnStation(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || c.tables == nil {
		return
	}
	name := c.objectNameOf(event.ObjectID)
	spinning := spinningWheelNames[name]
	if !spinning && name != "Loom" {
		return
	}
	if spinning {
		for _, recipe := range availableFor(c, c.tables.Spinning.Recipes, func(r SpinningRecipe) bool { return r.Members }) {
			for _, id := range recipe.MaterialIDs {
				if id == event.ItemID {
					event.Handled = true
					c.openSpinMenu(event.Player, event.Object, true)
					return
				}
			}
		}
		return
	}
	for _, recipe := range availableFor(c, c.tables.Weaving.Recipes, func(r WeavingRecipe) bool { return r.Members }) {
		for _, id := range recipe.MaterialIDs {
			if id == event.ItemID {
				event.Handled = true
				c.openSpinMenu(event.Player, event.Object, false)
				return
			}
		}
	}
}

// AttachSpinning registers the Spinning.Crafting.js hooks:
// Spin on wheels, Weave on looms, and materials on either.
func (c *crafting) AttachSpinning(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnObjectInteractionNamed("Spinning wheel", map[string]plugins.ObjectInteractionHandler{"Spin": c.spin})
	api.OnObjectInteractionNamed("Spinning Wheel", map[string]plugins.ObjectInteractionHandler{"Spin": c.spin})
	api.OnObjectInteractionNamed("Loom", map[string]plugins.ObjectInteractionHandler{"Weave": c.weave})
	noted := false
	api.OnItemOnObject(c.handleMaterialOnStation, &plugins.ItemUseFilter{Noted: &noted})
}
