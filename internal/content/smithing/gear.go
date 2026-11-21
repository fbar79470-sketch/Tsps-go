// Gear.Smithing.js: gear that changes a furnace's result
// (answers "smithing:smelt-outcome").
//
// Ring of forging: iron never fails while it's worn. 140
// charges kept on the player, one per iron ore; at none the
// ring melts, and Break resets them for the next ring.
//
// Varrock armour: at the Edgeville furnace, a 10% chance to
// smelt two bars at once, up to the tier's metal. Each extra
// iron bar takes a ring charge too, and a smithing catalyst
// while one is left (otherwise the full coal).
//
// Neither has captures; the messages are the game's, not
// checked against one (docs/smithing.md).
package smithing

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- gear constants (Gear.Smithing.js) ----

const (
	// ringChargesTotal is the ring of forging's charges (140).
	ringChargesTotal = 140
	// ringChargesAttribute is the persisted charges key.
	ringChargesAttribute = "smithing:ring-of-forging-charges"
	// varrockArmourChance is the double-bar chance (10%).
	varrockArmourChance = 0.1
)

// smeltOutcome is the "smithing:smelt-outcome" payload
// (Smelting.Smithing.js finishSmelt): the resolved recipe,
// the recipe chosen before the catalyst (Base), the furnace,
// whether the smelt succeeds, and how many bars it makes.
// Gear may force success (ring), double the bars (Varrock
// armour, with Second the recipe the extra bar's ingredients
// were taken by), or melt the ring (stopping the session).
type smeltOutcome struct {
	Player    *game.Player
	Recipe    *SmeltRecipe
	Base      *SmeltRecipe
	FurnaceID int
	Success   bool
	Bars      int
	Second    *smeltChoice
}

// varrockArmourTier is one Varrock armour tier: the armour id
// and the bars it smelts two of. Higher tiers include the
// lower (Gear.Smithing.js VARROCK_ARMOUR_TIERS).
type varrockArmourTier struct {
	armour int
	bars   []string // smelt recipe names
}

var varrockArmourTiers = []varrockArmourTier{
	{varrockArmour1ItemID, []string{"Bronze bar", "Blurite bar", "Iron bar", "Silver bar", "Steel bar"}},
	{varrockArmour2ItemID, []string{"Mithril bar", "Gold bar", "'perfect' gold bar"}},
	{varrockArmour3ItemID, []string{"Adamantite bar"}},
	{varrockArmour4ItemID, []string{"Runite bar"}},
}

// wearsRingOfForging reports whether the player wears a ring
// of forging (Gear.Smithing.js wearsRingOfForging).
func wearsRingOfForging(player *game.Player) bool {
	if player == nil || player.Equipment() == nil {
		return false
	}
	ring := player.Equipment().ItemAt(game.RingSlot)
	if !ring.IsValid() {
		return false
	}
	return ring.ID() == ringOfForgingItemID || ring.ID() == ringOfForging3ItemID
}

// ringCharges is the ring's remaining charges, 140 when the
// attribute is unset (Gear.Smithing.js ringCharges).
func ringCharges(player *game.Player) int {
	if player == nil {
		return ringChargesTotal
	}
	switch v := player.GetAttribute(ringChargesAttribute).(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return ringChargesTotal
	}
}

// useRingCharges spends ring charges (Gear.Smithing.js
// useRingCharges): at none the ring melts, the charges reset
// for the next ring, and the smelting stops.
func (s *smithing) useRingCharges(player *game.Player, count int) {
	if player == nil {
		return
	}
	charges := ringCharges(player) - count
	if charges > 0 {
		player.SetAttribute(ringChargesAttribute, charges)
		return
	}
	player.SetAttribute(ringChargesAttribute, ringChargesTotal)
	if eq := player.Equipment(); eq != nil {
		eq.Set(game.RingSlot, game.NewItem(-1, 0))
		eq.RefreshItems()
	}
	player.SendMessage("<col=7f007f>Your Ring of Forging has melted.</col>")
	// Wiki (Ring of forging): "the smelting will stop once
	// the ring crumbles".
	s.stopSession(player, false)
}

// varrockArmourBars is the bars the worn Varrock armour smelts
// two of, or none (Gear.Smithing.js varrockArmourBars).
func varrockArmourBars(player *game.Player) map[int]bool {
	bars := map[int]bool{}
	if player == nil || player.Equipment() == nil {
		return bars
	}
	body := player.Equipment().ItemAt(game.BodySlot)
	if !body.IsValid() {
		return bars
	}
	for _, tier := range varrockArmourTiers {
		for _, name := range tier.bars {
			if id := barIDByName(name); id != 0 {
				bars[id] = true
			}
		}
		if tier.armour == body.ID() {
			return bars
		}
	}
	return map[int]bool{}
}

// smeltOutcome answers "smithing:smelt-outcome"
// (Gear.Smithing.js smeltOutcome).
func (s *smithing) smeltOutcome(payload any) {
	outcome, ok := payload.(*smeltOutcome)
	if !ok || outcome == nil || outcome.Player == nil || outcome.Recipe == nil {
		return
	}
	player := outcome.Player
	base := outcome.Base
	if base == nil {
		base = outcome.Recipe
	}
	second := s.resolveCatalyst(player, base)
	iron := outcome.Recipe.BarID == barIDByName("Iron bar")
	ring := iron && wearsRingOfForging(player)
	if ring {
		outcome.Success = true
	}
	if outcome.Success &&
		outcome.FurnaceID == edgevilleFurnaceLocID &&
		varrockArmourBars(player)[outcome.Recipe.BarID] &&
		s.randFloat() < varrockArmourChance &&
		s.hasIngredients(player, second) {
		s.consumeIngredients(player, second)
		outcome.Bars = 2
		secondCopy := second
		outcome.Second = &secondCopy
		player.SendMessage("The Varrock platebody enabled you to smelt your next ore simultaneously.")
	}
	if ring {
		s.useRingCharges(player, outcome.Bars)
	}
}

// checkRing is "Check" on a ring of forging
// (Gear.Smithing.js checkRing).
func (s *smithing) checkRing(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	charges := ringCharges(event.Player)
	pieces := "pieces"
	if charges == 1 {
		pieces = "piece"
	}
	event.Player.SendMessage("You can smelt " + itoa(charges) + " more " + pieces + " of iron ore before a ring of forging melts.")
	return true
}

// breakRing is "Break" on a ring of forging
// (Gear.Smithing.js breakRing).
func (s *smithing) breakRing(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	if event.InterfaceID == game.EquipmentInventoryInterfaceID {
		if eq := player.Equipment(); eq != nil {
			eq.Set(event.Slot, game.NewItem(-1, 0))
			eq.RefreshItems()
		}
	} else if inv := player.Inventory(); inv != nil {
		inv.Set(event.Slot, game.NewItem(-1, 0))
		inv.RefreshItems()
	}
	player.SetAttribute(ringChargesAttribute, ringChargesTotal)
	player.SendMessage("The ring shatters. Your next ring of forging will start afresh from 140 pieces of iron.")
	return true
}

// registerGear wires the gear hooks (Gear.Smithing.js attach).
func (s *smithing) registerGear(api *plugins.Api) {
	api.PersistAttribute(ringChargesAttribute, ringChargesTotal)
	api.OnItemActionNamed("Ring of forging", map[string]plugins.ItemActionHandler{
		"Check": s.checkRing, "Break": s.breakRing,
	}, nil)
}
