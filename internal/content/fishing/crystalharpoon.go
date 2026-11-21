// CrystalHarpoon is the Go port of plugins/skills/fishing/
// CrystalHarpoon.Fishing.js (commit bfd81e3): the crystal
// harpoon's charges — created with 10,000, one spent per
// fish caught, recharged with crystal shards (100 each, up
// to 20,000), an empty harpoon turning inactive and fishing
// like a dragon harpoon, and an elven or celestial signet
// giving a 10% chance to save the charge. Charge state is
// {"charges": N} under the "crystal-harpoon" meta key,
// which survives save/load (internal/persist/apply.go:507,
// the same key the crystal axe uses for its own meta).
//
// All harpoon-form ids are ItemIdentifiers.ts values,
// verified against the real rev-242 seed cache by display
// name (probe, 2026-11-20): charged 23762 ("Crystal
// harpoon") and 23864 ("Crystal harpoon", the gauntlet
// harpoon); inactive 23764/23765 ("Crystal harpoon
// (inactive)"). The maps are verbatim from the unit file,
// including the asymmetry: the inactive forms both
// recharge to 23762, and 23864's uncharged form is 23764.
// The shard and signet ids come from the resolved
// FishingIDs, not from constants here.
//
// The charge drain is wired into the core loop
// (landCatch): only the charged crystal harpoon doing the
// fishing spends a charge, after the catch, exactly as
// the loop's crystal-charge call site has it. The shard
// recharge rides the onItemOnItem family.
//
// Go differences, all forced by value semantics: TS
// mutates the live Item reference; Go Items are values, so
// every mutation reads the item, mutates the copy, and
// writes it back through the container at the carried slot
// (itemSlot / writeSlotItem). The event's UsedItem is
// therefore re-read from the slot in addShardCharges, and
// findChargedCrystalHarpoon returns the slot with the item
// (the woodcutting crystalaxe.go precedent). The shared
// charge helpers in this file (chargesFromMeta,
// chargesMetaValue, formatThousands, actionItem,
// itemSlot, writeSlotItem, refreshContainers) are shared
// with InfernalHarpoon.
package fishing

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- CrystalHarpoon constants (CrystalHarpoon.Fishing.js:8-11) ----

const (
	// crystalHarpoonStartCharges is START_CHARGES: a fresh
	// charged crystal harpoon holds 10,000 charges.
	crystalHarpoonStartCharges = 10000
	// crystalHarpoonMaxCharges is MAX_CHARGES: a harpoon
	// holds at most 20,000 charges.
	crystalHarpoonMaxCharges = 20000
	// crystalHarpoonShardCharges is SHARD_CHARGES: one
	// crystal shard adds 100 charges.
	crystalHarpoonShardCharges = 100
	// crystalHarpoonChargesMetaKey is CHARGES_META_KEY: the
	// item-meta key holding {"charges": N}.
	crystalHarpoonChargesMetaKey = "crystal-harpoon"
)

// unchargedByChargedCrystalHarpoon is the unit file's
// unchargedByCharged map, verbatim: CRYSTAL_HARPOON
// (23762) and CRYSTAL_HARPOON_3 (23864) →
// CRYSTAL_HARPOON_INACTIVE_ (23764).
// (ItemIdentifiers.ts:21919, 21921, 22013.)
var unchargedByChargedCrystalHarpoon = map[int]int{
	23762: 23764,
	23864: 23764,
}

// chargedByUnchargedCrystalHarpoon is the unit file's
// chargedByUncharged map, verbatim:
// CRYSTAL_HARPOON_INACTIVE_ (23764) and
// CRYSTAL_HARPOON_INACTIVE__2 (23765) → CRYSTAL_HARPOON
// (23762). (ItemIdentifiers.ts:21921-21922.)
var chargedByUnchargedCrystalHarpoon = map[int]int{
	23764: 23762,
	23765: 23762,
}

// chargedCrystalHarpoonIDs is the unit file's chargedIds
// set: the keys of unchargedByCharged.
var chargedCrystalHarpoonIDs = map[int]bool{
	23762: true,
	23864: true,
}

// isCrystalHarpoon reports whether id is a crystal harpoon
// in either form: charged (unchargedByCharged keys) or
// uncharged (chargedByUncharged keys).
func isCrystalHarpoon(id int) bool {
	if chargedCrystalHarpoonIDs[id] {
		return true
	}
	_, ok := chargedByUnchargedCrystalHarpoon[id]
	return ok
}

// ---- shared charge helpers (CrystalHarpoon + InfernalHarpoon) ----

// itemSlot is one charged item in a container: the item as
// read, its slot, and whether it is wielded (the weapon
// slot) rather than carried. TS finders return the live
// Item; Go returns the slot so the caller can write the
// mutated copy back.
type itemSlot struct {
	item    game.Item
	slot    int
	wielded bool
}

// writeSlotItem writes item back to loc's slot: the
// equipment weapon slot when wielded, else the inventory
// slot.
func writeSlotItem(player *game.Player, loc itemSlot, item game.Item) {
	if player == nil {
		return
	}
	if loc.wielded {
		if eq := player.Equipment(); eq != nil {
			eq.SetItem(loc.slot, item)
		}
		return
	}
	if inv := player.Inventory(); inv != nil {
		inv.SetItem(loc.slot, item)
	}
}

// refreshContainers refreshes the inventory and the
// equipment: the Go form of the unit files' refresh()
// (CrystalHarpoon.Fishing.js:34-37,
// InfernalHarpoon.Fishing.js:47-50), which refreshes both
// after a harpoon changes form.
func refreshContainers(player *game.Player) {
	if player == nil {
		return
	}
	if inv := player.Inventory(); inv != nil {
		inv.RefreshItems()
	}
	if eq := player.Equipment(); eq != nil {
		eq.RefreshItems()
	}
}

// actionItem reads the item an item-action event clicked:
// the inventory item at the slot (interface 3214) or the
// equipped item at the slot (interface 1688). The listener
// verified the slot holds the clicked id before emitting
// (handleAction, itemaction.go), so the slot item IS the
// clicked item; no fallback is needed. An out-of-range slot
// reads the empty item. (The woodcutting crystalaxe.go
// precedent.)
func actionItem(player *game.Player, event *plugins.PluginItemActionEvent) game.Item {
	if player == nil || event == nil {
		return game.NewItem(-1, 0)
	}
	if event.InterfaceID == game.EquipmentInventoryInterfaceID {
		if eq := player.Equipment(); eq != nil {
			return eq.ItemAt(event.Slot)
		}
		return game.NewItem(-1, 0)
	}
	if inv := player.Inventory(); inv != nil {
		return inv.ItemAt(event.Slot)
	}
	return game.NewItem(-1, 0)
}

// chargesFromMeta reads {"charges": N} under key, reporting
// whether a finite number was stored. No meta, unparsable
// meta, or a missing/non-numeric charges field all report
// false, and the caller falls back to its id default — the
// Go form of TS's Number(...) / Number.isFinite check
// (CrystalHarpoon.Fishing.js:16-17,
// InfernalHarpoon.Fishing.js:14-16).
func chargesFromMeta(item game.Item, key string) (int, bool) {
	raw, ok := item.GetMetaValue(key)
	if !ok {
		return 0, false
	}
	var saved struct {
		Charges *float64 `json:"charges"`
	}
	if err := json.Unmarshal(raw, &saved); err != nil || saved.Charges == nil ||
		math.IsNaN(*saved.Charges) || math.IsInf(*saved.Charges, 0) {
		return 0, false
	}
	return int(*saved.Charges), true
}

// chargesMetaValue encodes {"charges": N} for SetMetaValue.
func chargesMetaValue(charges int) json.RawMessage {
	raw, _ := json.Marshal(struct {
		Charges int `json:"charges"`
	}{Charges: charges})
	return raw
}

// formatThousands renders n with en-US grouping ("10,000"):
// the Go form of Number.toLocaleString("en-US") in
// checkCharges (CrystalHarpoon.Fishing.js:81).
func formatThousands(n int) string {
	s := strconv.Itoa(n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	var out []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, byte(c))
	}
	if neg {
		return "-" + string(out)
	}
	return string(out)
}

// ---- CrystalHarpoon ----

// crystalHarpoonCharges reads an item's crystal-harpoon
// charges (CrystalHarpoon.Fishing.js:15-22): the saved value
// clamped to [0, MAX_CHARGES], else START_CHARGES for a
// charged harpoon with no saved value, else 0.
func crystalHarpoonCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, crystalHarpoonChargesMetaKey); ok {
		return min(max(saved, 0), crystalHarpoonMaxCharges)
	}
	if chargedCrystalHarpoonIDs[item.ID()] {
		return crystalHarpoonStartCharges
	}
	return 0
}

// findChargedCrystalHarpoon finds the charged crystal
// harpoon the player is wielding or carrying, wielded first
// (CrystalHarpoon.Fishing.js:25-32).
func findChargedCrystalHarpoon(player *game.Player) (itemSlot, bool) {
	if player == nil {
		return itemSlot{}, false
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if chargedCrystalHarpoonIDs[weapon.ID()] {
			return itemSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if chargedCrystalHarpoonIDs[item.ID()] {
				return itemSlot{item: item, slot: i}, true
			}
		}
	}
	return itemSlot{}, false
}

// wearingSignet reports whether the player wears an elven
// (or celestial) signet (CrystalHarpoon.Fishing.js:58-63):
// any of the five signet ids in the equipment. The ids
// come from the resolved FishingIDs.
func (f *fishing) wearingSignet(player *game.Player) bool {
	if player == nil || player.Equipment() == nil || f == nil || f.ids == nil {
		return false
	}
	for _, id := range []int{
		f.ids.ElvenSignet, f.ids.ElvenSignet2, f.ids.ElvenSignet3,
		f.ids.CelestialSignet, f.ids.CelestialSignetUncharged,
	} {
		if player.Equipment().Contains(id) {
			return true
		}
	}
	return false
}

// useCrystalHarpoonCharge spends one charge of the harpoon
// at loc (CrystalHarpoon.Fishing.js:40-50): decrement the
// saved charges, or at 0 set the id to the uncharged form,
// clear the meta, refresh, and message.
func useCrystalHarpoonCharge(player *game.Player, loc itemSlot) {
	left := crystalHarpoonCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(crystalHarpoonChargesMetaKey, chargesMetaValue(left))
		writeSlotItem(player, loc, item)
		return
	}
	uncharged, ok := unchargedByChargedCrystalHarpoon[loc.item.ID()]
	if !ok {
		return
	}
	item := loc.item
	item.SetID(uncharged)
	item.SetMetaValue(crystalHarpoonChargesMetaKey, nil)
	writeSlotItem(player, loc, item)
	refreshContainers(player)
	player.SendMessage("Your crystal harpoon has run out of charges.")
}

// tryUseCrystalCharge consumes one charge per caught fish;
// it returns false when the signet saved it
// (CrystalHarpoon.Fishing.js:66-77). It returns false too
// when no charged harpoon is held. The caller is landCatch:
// only the charged crystal harpoon doing the fishing is
// drained.
func (f *fishing) tryUseCrystalCharge(player *game.Player) bool {
	loc, ok := findChargedCrystalHarpoon(player)
	if !ok {
		return false
	}
	if f.wearingSignet(player) && rand.Float64() < 0.10 {
		return false
	}
	useCrystalHarpoonCharge(player, loc)
	return true
}

// checkCrystalHarpoonCharges answers the Check action
// (CrystalHarpoon.Fishing.js:79-83): "Your crystal harpoon
// has X charge(s) left." with en-US grouping. It always
// claims the click.
func (f *fishing) checkCrystalHarpoonCharges(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	left := crystalHarpoonCharges(actionItem(event.Player, event))
	suffix := "s"
	if left == 1 {
		suffix = ""
	}
	event.Player.SendMessage(fmt.Sprintf("Your crystal harpoon has %s charge%s left.", formatThousands(left), suffix))
	return true
}

// addShardCharges recharges a crystal harpoon with crystal
// shards (CrystalHarpoon.Fishing.js:86-116): the shard on
// the harpoon in either direction; full → message +
// handled; uncharged → charged form; capped at
// MAX_CHARGES. It returns false when the pair is not a
// shard + crystal harpoon or no shards are carried (the use
// falls through), and true when it handled the use.
func (f *fishing) addShardCharges(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil || f == nil || f.ids == nil {
		return false
	}
	player := event.Player
	var harpoon game.Item
	var slot int
	switch {
	case isCrystalHarpoon(event.UsedItemID):
		harpoon, slot = event.UsedItem, event.UsedItemSlot
	case isCrystalHarpoon(event.UsedWithItemID):
		harpoon, slot = event.UsedWithItem, event.UsedWithItemSlot
	default:
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	// Re-read the harpoon from its slot: the event carries a
	// value copy, and the slot is the live item.
	if current := inv.ItemAt(slot); current.ID() == harpoon.ID() {
		harpoon = current
	}
	shardCount := inv.GetAmount(f.ids.CrystalShard)
	if shardCount <= 0 {
		return false
	}
	left := crystalHarpoonCharges(harpoon)
	if left >= crystalHarpoonMaxCharges {
		player.SendMessage("Your crystal harpoon cannot hold any more charges.")
		return true
	}
	shards := min(shardCount, (crystalHarpoonMaxCharges-left+crystalHarpoonShardCharges-1)/crystalHarpoonShardCharges)
	inv.DeleteNumber(f.ids.CrystalShard, shards)
	if charged, ok := chargedByUnchargedCrystalHarpoon[harpoon.ID()]; ok {
		harpoon.SetID(charged)
	}
	harpoon.SetMetaValue(crystalHarpoonChargesMetaKey,
		chargesMetaValue(min(crystalHarpoonMaxCharges, left+shards*crystalHarpoonShardCharges)))
	inv.SetItem(slot, harpoon)
	refreshContainers(player)
	suffix := "s"
	if shards == 1 {
		suffix = ""
	}
	player.SendMessage(fmt.Sprintf("You add %d crystal shard%s to the harpoon.", shards, suffix))
	return true
}

// registerCrystalHarpoon wires the CrystalHarpoon
// registrations (CrystalHarpoon.Fishing.js:118-130), in TS
// order: Check on the charged harpoon name, then the shard
// pairs per harpoon form.
func (f *fishing) registerCrystalHarpoon(api *plugins.Api) {
	if api == nil {
		return
	}
	check := map[string]plugins.ItemActionHandler{"Check": f.checkCrystalHarpoonCharges}
	unnoted := &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)}
	api.OnItemActionNamed("Crystal harpoon", check, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal harpoon", f.addShardCharges, unnoted)
	api.OnItemOnItemNamed("Crystal shard", "Crystal harpoon (inactive)", f.addShardCharges, unnoted)
}
