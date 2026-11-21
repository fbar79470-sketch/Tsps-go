// Package wornweights is the Go port of the WornWeights plugin
// (server/plugins/items/WornWeights.plugin.js): boots of lightness,
// penance gloves, and spotted/spottier capes weigh less once worn.
// The cache keeps the worn weight on a separate item id, and nothing
// swaps the id on equip, so answering "equipment:weights" gives the
// equipment panel, the run-energy packet, and the drain the worn
// weight (as GracefulOutfit does for graceful pieces).
//
// The port is faithful: same plugin name and description, no members
// flag (it loads on both world types), same attach-only register,
// same id map, same iterate-all-slots loop, and the same
// first-writer-wins rule (a slot whose override is already finite
// keeps it). Line numbers in comments cite WornWeights.plugin.js.
//
// Event inventory:
//
//   - Listens to "equipment:weights" (game.EquipmentWeightsEventName):
//     produced in Go by Player.CarriedWeightGrams (engine). This
//     package and GracefulOutfit both answer it; handler order is
//     unobservable — no item matches both pieces' rules (graceful
//     pieces match by name prefix, worn-weight items by id, and the
//     two sets are disjoint), so the first-writer-wins skip never
//     sees a graceful override on a worn-weight slot.
//   - Emits nothing.
//
// One deliberate deviation, forced by the language change: TS reads
// the worn weight via core.ItemDefinition.forId(wornId).getWeight().
// Go reads it via the player's installed definition source
// (Player.ItemDefinitions().WeightGrams, the same Store). A player
// with no definition source installed gets no overrides: with no
// source every slot weighs 0 through the only consumer
// (CarriedWeightGrams), so skipping is observably identical to
// writing the TS default-definition weight of 0.
package wornweights

import (
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// wornWeightID is WORN_WEIGHT_ID (WornWeights.plugin.js:12-17): the
// worn id carrying the lighter weight for each inventory id.
var wornWeightID = map[int]int{
	88:    89,    // Boots of lightness
	10553: 10554, // Penance gloves
	10069: 10073, // Spotted cape
	10071: 10074, // Spottier cape
}

// addWornWeights answers an "equipment:weights" request
// (addWornWeights, WornWeights.plugin.js:21-29): it iterates ALL
// equipment slots, and when the worn id is in the map AND the
// slot's override is not already finite (first writer wins), sets
// the override to the worn id's definition weight in kg
// (grams / 1000). A missing override reads as not finite, as does
// NaN or an infinity, so all three are overwritten. It is a no-op
// when there is no player or no slotWeights map.
func addWornWeights(request *game.EquipmentWeightsRequest) {
	if request == nil || request.Player == nil || request.SlotWeights == nil {
		return
	}
	equipment := request.Player.Equipment()
	if equipment == nil {
		return
	}
	src := request.Player.ItemDefinitions()
	if src == nil {
		return
	}
	for slot, item := range equipment.Items() {
		wornID, ok := wornWeightID[item.ID()]
		if !ok {
			continue
		}
		if existing, present := request.SlotWeights[slot]; present && !math.IsNaN(existing) && !math.IsInf(existing, 0) {
			continue
		}
		request.SlotWeights[slot] = float64(src.WeightGrams(wornID)) / 1000
	}
}

// onEquipmentWeights is the "equipment:weights" custom-event
// handler: it type-asserts the opaque payload and delegates to
// addWornWeights. A payload of any other type is ignored.
func onEquipmentWeights(payload any) {
	request, ok := payload.(*game.EquipmentWeightsRequest)
	if !ok || request == nil {
		return
	}
	addWornWeights(request)
}

// Plugin is the registered WornWeights plugin value.
var Plugin = plugins.Plugin{
	Name:        "WornWeights",
	Description: "Makes boots of lightness, penance gloves and spotted and spottier capes weigh their lighter worn weight in the equipment panel and for run-energy drain.",
	Register: func(api *plugins.Api) {
		api.OnCustomEvent(game.EquipmentWeightsEventName, onEquipmentWeights)
	},
}

func init() {
	plugins.Register(Plugin)
}
