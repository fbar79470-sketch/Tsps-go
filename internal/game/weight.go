package game

// Live carried weight: the port of Player.getCarriedWeightGrams /
// getWeightKg (Player.ts:1287-1316), computed from the live
// inventory and equipment containers on every call — replacing
// the phase 7 entry-time static weight (see runenergy.go).
//
// The rule (verified against Player.ts, not trusted from the
// phase 3 bootstrap comment): the "equipment:weights" custom
// event fires FIRST with an empty per-slot override map
// (Player.ts:1288-1289); equipment slots then use `override *
// 1000` grams when the override is finite AND the slot holds an
// item with id >= 0 and amount > 0, else the definition weight
// (Player.ts:1299-1304); inventory slots use definition weight
// only. Each occupied slot contributes its definition weight
// once regardless of the stack amount; stackable or noted items
// contribute 0; unoccupied slots (id < 0 or amount <= 0)
// contribute 0. Kilograms truncate toward zero (Math.trunc;
// Go integer division truncates the same way, including for
// negative totals — graceful-style negative weights can make the
// sum negative, Player.ts:1280-1286).
//
// FLOAT ROUND-TRIP: overrides are kg floats (graceful whole kg;
// worn weights WeightGrams/1000), and each slot contributes
// int(override * 1000) grams — a per-slot truncation toward
// zero, where TS accumulates the float grams and truncates only
// at the kg level (getWeightKg). Whole-kg overrides are exact.
// Fractional overrides round-trip exactly for every realistic
// item weight (measured: all gram values -1000..1000 survive
// float64(grams)/1000*1000 truncated back to the same grams);
// the first value that does not is 1001g (1.001kg reads back as
// 1000g). A sub-gram per-slot loss can only move the truncated
// kg total when the true total sits within a few grams above a
// kg boundary; the kg totals TS exposes are otherwise
// identical. The exact values are pinned in
// equipment_effects_test.go.
//
// With no EmitCustomEvent sink installed, no event fires and
// every slot uses its definition weight. With no definition
// source installed every slot weighs 0, matching the
// bootstrap's no-cache behavior.
//
// The slot iteration reads the containers' slot arrays directly
// (same package) instead of Items() copies: the drain reads the
// weight every running tick, and cloning 42 items with deep meta
// copies per read would be pure waste. This is safe under the
// tick-goroutine-only discipline the rest of Player follows.

// CarriedWeightGrams returns the live carried weight in grams
// (getCarriedWeightGrams, Player.ts:1287-1311). A nil player, or
// a player without live containers (a zero-value Player), weighs
// 0.
func (p *Player) CarriedWeightGrams() int {
	if p == nil {
		return 0
	}
	// The weights query fires BEFORE computing (Player.ts:
	// 1288-1289), through the installed custom-event sink; a
	// nil sink means no subscribers and no overrides. The
	// request is a pointer so handler writes are visible here.
	var request *EquipmentWeightsRequest
	if p.EmitCustomEvent != nil {
		request = &EquipmentWeightsRequest{Player: p, SlotWeights: make(map[int]float64)}
		p.EmitCustomEvent(EquipmentWeightsEventName, request)
	}
	var src ItemDefinitionSource
	if p.itemDefs != nil {
		src = p.itemDefs
	}
	grams := 0
	if p.equipment != nil && p.equipment.itemContainer != nil {
		for i := range p.equipment.items {
			override, hasOverride := slotWeightOverride(request, i)
			grams += equippedSlotWeightGrams(src, &p.equipment.items[i], override, hasOverride)
		}
	}
	if p.inventory != nil && p.inventory.itemContainer != nil {
		for i := range p.inventory.items {
			grams += slotWeightGrams(src, &p.inventory.items[i])
		}
	}
	return grams
}

// slotWeightOverride reads the kg override for equipment slot
// from a weights request, reporting whether one is present. A
// nil request (no emit) has no overrides.
func slotWeightOverride(request *EquipmentWeightsRequest, slot int) (float64, bool) {
	if request == nil {
		return 0, false
	}
	override, ok := request.SlotWeights[slot]
	return override, ok
}

// equippedSlotWeightGrams is the equipment-slot branch of
// getCarriedWeightGrams (Player.ts:1299-1304): a finite override
// on an occupied slot (id >= 0, amount > 0) contributes
// int(override * 1000) grams — the override wins even for a
// stackable or noted item, as TS never consults itemGrams on
// that branch — and every other slot falls back to the
// definition weight via slotWeightGrams. hasOverride false
// (no emit, or the slot unwritten) always falls back.
func equippedSlotWeightGrams(src ItemDefinitionSource, item *Item, override float64, hasOverride bool) int {
	if hasOverride && isFinite(override) && item != nil && item.id >= 0 && item.amount > 0 {
		return int(override * 1000)
	}
	return slotWeightGrams(src, item)
}

// CarriedWeightKg returns the live carried weight in whole
// kilograms (getWeightKg, Player.ts:1314-1316).
func (p *Player) CarriedWeightKg() int {
	return p.CarriedWeightGrams() / 1000
}

// slotWeightGrams is the itemGrams closure of
// getCarriedWeightGrams (Player.ts:1290-1296): 0 for an empty
// slot or a stackable/noted item, else the definition weight. A
// nil source weighs every slot 0.
func slotWeightGrams(src ItemDefinitionSource, item *Item) int {
	if item == nil || item.id < 0 || item.amount <= 0 {
		return 0
	}
	if src == nil {
		return 0
	}
	if src.IsStackable(item.id) || src.IsNoted(item.id) {
		return 0
	}
	return src.WeightGrams(item.id)
}
