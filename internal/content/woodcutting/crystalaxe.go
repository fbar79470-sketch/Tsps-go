// CrystalAxe is the Go port of plugins/skills/woodcutting/
// CrystalAxe.Woodcutting.js (132 lines, commit bfd81e3): the
// crystal axe's charges — created with 10,000, one spent per
// log chopped, recharged with crystal shards (100 each, up to
// 20,000), an empty axe turning inactive and chopping like a
// dragon axe, and an elven signet giving a 10% chance to save
// the charge. Charge state is {"charges": N} under the
// "crystal-axe" meta key, which survives save/load
// (internal/persist/apply.go:507).
//
// All ids are ItemIdentifiers.ts values, verified against the
// real rev-242 seed cache (probe, 2026-11-20): charged 23673
// ("Crystal axe", actions include "Check"), 23862 ("Crystal
// axe", no Check action), felling 28220 ("Crystal felling
// axe", "Check"); inactive 23675/23676, felling inactive
// 28223/28225; shard 23962 ("Crystal shard", unnoted);
// signets 23943/23944/23945 (elven) and 25543/25545
// (celestial, uncharged/charged). The maps are verbatim from
// the unit file, including the asymmetries: CRYSTAL_AXE_2
// (23674) and CRYSTAL_FELLING_AXE_2 (28222) are in neither
// map, and CRYSTAL_FELLING_AXE_INACTIVE__2 (28225) has no
// charged form in chargedByUncharged.
//
// The charge drain is wired into the core loop (chopSuccess):
// only the charged crystal axe doing the chopping spends a
// charge, after the log line and before XP, exactly as
// Woodcutting.plugin.js:1416-1420 has it. The shard recharge
// rides the onItemOnItem family (phase 33).
//
// Go differences, all forced by value semantics: TS mutates
// the live Item reference; Go Items are values, so every
// mutation reads the item, mutates the copy, and writes it
// back through the container at the carried slot (writeAxeItem).
// The event's UsedItem is therefore re-read from the slot in
// addShardCharges, and findCharged returns the slot with the
// item (axeSlot, shared with InfernalAxe).

package woodcutting

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- CrystalAxe constants (CrystalAxe.Woodcutting.js:12-14) ----

const (
	// crystalStartCharges is START_CHARGES: a fresh charged
	// crystal axe holds 10,000 charges.
	crystalStartCharges = 10000
	// crystalMaxCharges is MAX_CHARGES: an axe holds at most
	// 20,000 charges.
	crystalMaxCharges = 20000
	// crystalShardCharges is SHARD_CHARGES: one crystal shard
	// adds 100 charges.
	crystalShardCharges = 100
	// crystalChargesMetaKey is CHARGES_META_KEY: the item-meta
	// key holding {"charges": N}.
	crystalChargesMetaKey = "crystal-axe"
	// crystalShardID is ItemIds.CRYSTAL_SHARD
	// (ItemIdentifiers.ts:22111).
	crystalShardID = 23962
)

// unchargedByChargedCrystal is the unit file's
// unchargedByCharged map, verbatim: CRYSTAL_AXE (23673) and
// CRYSTAL_AXE_3 (23862) → CRYSTAL_AXE_INACTIVE_ (23675);
// CRYSTAL_FELLING_AXE (28220) → CRYSTAL_FELLING_AXE_INACTIVE_
// (28223). (ItemIdentifiers.ts:21832, 22011, 26147-26150.)
var unchargedByChargedCrystal = map[int]int{
	23673: 23675,
	23862: 23675,
	28220: 28223,
}

// chargedByUnchargedCrystal is the unit file's
// chargedByUncharged map, verbatim: CRYSTAL_AXE_INACTIVE_
// (23675) and CRYSTAL_AXE_INACTIVE__2 (23676) → CRYSTAL_AXE
// (23673); CRYSTAL_FELLING_AXE_INACTIVE_ (28223) →
// CRYSTAL_FELLING_AXE (28220). Note 28225 has no entry.
// (ItemIdentifiers.ts:21834-21835, 26150.)
var chargedByUnchargedCrystal = map[int]int{
	23675: 23673,
	23676: 23673,
	28223: 28220,
}

// chargedCrystalAxeIDs is the unit file's chargedIds set: the
// keys of unchargedByCharged.
var chargedCrystalAxeIDs = map[int]bool{
	23673: true,
	23862: true,
	28220: true,
}

// isCrystalAxe reports whether id is a crystal axe in
// either form: charged (unchargedByCharged keys) or
// uncharged (chargedByUncharged keys).
func isCrystalAxe(id int) bool {
	if chargedCrystalAxeIDs[id] {
		return true
	}
	_, ok := chargedByUnchargedCrystal[id]
	return ok
}

// elvenSignetIDs are the ids wearingSignet checks
// (CrystalAxe.Woodcutting.js:66-70): ELVEN_SIGNET (23943),
// ELVEN_SIGNET_2 (23944), ELVEN_SIGNET_3 (23945),
// CELESTIAL_SIGNET (25545), CELESTIAL_SIGNET_UNCHARGED_
// (25543). (ItemIdentifiers.ts:22092-22094, 23563, 23565.)
var elvenSignetIDs = []int{23943, 23944, 23945, 25545, 25543}

// ---- shared axe-slot helpers (CrystalAxe + InfernalAxe) ----

// axeSlot is one axe in a container: the item as read, its
// slot, and whether it is wielded (the weapon slot) rather
// than carried. TS finders return the live Item; Go returns
// the slot so the caller can write the mutated copy back.
type axeSlot struct {
	item    game.Item
	slot    int
	wielded bool
}

// writeAxeItem writes item back to loc's slot: the equipment
// weapon slot when wielded, else the inventory slot.
func writeAxeItem(player *game.Player, loc axeSlot, item game.Item) {
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
// (CrystalAxe.Woodcutting.js:38-41,
// InfernalAxe.Woodcutting.js:55-58), which refreshes both
// after an axe changes form.
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
// reads the empty item.
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
// (CrystalAxe.Woodcutting.js:18-19,
// InfernalAxe.Woodcutting.js:16-18).
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
// checkCharges (CrystalAxe.Woodcutting.js:81).
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

// ---- CrystalAxe ----

// crystalCharges reads an item's crystal-axe charges
// (CrystalAxe.Woodcutting.js:17-23): the saved value clamped
// to [0, MAX_CHARGES], else START_CHARGES for a charged axe
// with no saved value, else 0.
func crystalCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, crystalChargesMetaKey); ok {
		return min(max(saved, 0), crystalMaxCharges)
	}
	if chargedCrystalAxeIDs[item.ID()] {
		return crystalStartCharges
	}
	return 0
}

// findChargedCrystalAxe finds the charged crystal axe (of
// axeID, when given; 0 means any) the player is wielding or
// carrying, wielded first (CrystalAxe.Woodcutting.js:26-34).
func findChargedCrystalAxe(player *game.Player, axeID int) (axeSlot, bool) {
	if player == nil {
		return axeSlot{}, false
	}
	matches := func(id int) bool {
		return chargedCrystalAxeIDs[id] && (axeID == 0 || id == axeID)
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if matches(weapon.ID()) {
			return axeSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if matches(item.ID()) {
				return axeSlot{item: item, slot: i}, true
			}
		}
	}
	return axeSlot{}, false
}

// wearingSignet reports whether the player wears an elven
// (or celestial) signet (CrystalAxe.Woodcutting.js:64-71):
// any of the five signet ids in the equipment.
func wearingSignet(player *game.Player) bool {
	if player == nil || player.Equipment() == nil {
		return false
	}
	for _, id := range elvenSignetIDs {
		if player.Equipment().Contains(id) {
			return true
		}
	}
	return false
}

// useCrystalCharge spends one charge of the axe at loc
// (CrystalAxe.Woodcutting.js:43-52): decrement the saved
// charges, or at 0 set the id to the uncharged form, clear
// the meta, refresh, and message.
func useCrystalCharge(player *game.Player, loc axeSlot) {
	left := crystalCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(crystalChargesMetaKey, chargesMetaValue(left))
		writeAxeItem(player, loc, item)
		return
	}
	uncharged, ok := unchargedByChargedCrystal[loc.item.ID()]
	if !ok {
		return
	}
	item := loc.item
	item.SetID(uncharged)
	item.SetMetaValue(crystalChargesMetaKey, nil)
	writeAxeItem(player, loc, item)
	refreshContainers(player)
	player.SendMessage("Your crystal axe has run out of charges.")
}

// tryUseCrystalCharge spends one charge of the chopping axe
// (axeID) for a chopped log; it returns false when the
// signet saved it (CrystalAxe.Woodcutting.js:74-83). It
// returns false too when no charged axe of that id is held.
// The caller is chopSuccess: only the charged crystal axe
// doing the chopping is drained.
func (w *woodcutting) tryUseCrystalCharge(player *game.Player, axeID int) bool {
	loc, ok := findChargedCrystalAxe(player, axeID)
	if !ok {
		return false
	}
	if wearingSignet(player) && w.randFloat() < 0.10 {
		return false
	}
	useCrystalCharge(player, loc)
	return true
}

// checkCrystalCharges answers the Check action
// (CrystalAxe.Woodcutting.js:78-82): "Your crystal axe has
// X charge(s) left." with en-US grouping. It always claims
// the click.
func (w *woodcutting) checkCrystalCharges(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	left := crystalCharges(actionItem(event.Player, event))
	suffix := "s"
	if left == 1 {
		suffix = ""
	}
	event.Player.SendMessage(fmt.Sprintf("Your crystal axe has %s charge%s left.", formatThousands(left), suffix))
	return true
}

// addShardCharges recharges a crystal axe with crystal
// shards (CrystalAxe.Woodcutting.js:85-116): the shard on
// the axe in either direction; full → message + handled;
// uncharged → charged form; capped at MAX_CHARGES. It
// returns false when the pair is not a shard + crystal axe
// or no shards are carried (the use falls through), and
// true when it handled the use.
func (w *woodcutting) addShardCharges(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	var axe game.Item
	var slot int
	switch {
	case isCrystalAxe(event.UsedItemID):
		axe, slot = event.UsedItem, event.UsedItemSlot
	case isCrystalAxe(event.UsedWithItemID):
		axe, slot = event.UsedWithItem, event.UsedWithItemSlot
	default:
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	// Re-read the axe from its slot: the event carries a
	// value copy, and the slot is the live item.
	if current := inv.ItemAt(slot); current.ID() == axe.ID() {
		axe = current
	}
	shardCount := inv.GetAmount(crystalShardID)
	if shardCount <= 0 {
		return false
	}
	left := crystalCharges(axe)
	if left >= crystalMaxCharges {
		player.SendMessage("Your crystal axe cannot hold any more charges.")
		return true
	}
	shards := min(shardCount, (crystalMaxCharges-left+crystalShardCharges-1)/crystalShardCharges)
	inv.DeleteNumber(crystalShardID, shards)
	if charged, ok := chargedByUnchargedCrystal[axe.ID()]; ok {
		axe.SetID(charged)
	}
	axe.SetMetaValue(crystalChargesMetaKey,
		chargesMetaValue(min(crystalMaxCharges, left+shards*crystalShardCharges)))
	inv.SetItem(slot, axe)
	refreshContainers(player)
	suffix := "s"
	if shards == 1 {
		suffix = ""
	}
	player.SendMessage(fmt.Sprintf("You add %d crystal shard%s to the axe.", shards, suffix))
	return true
}

// registerCrystalAxe wires the CrystalAxe registrations
// (CrystalAxe.Woodcutting.js:118-130), in TS order: Check
// per charged axe name, then the shard pairs per axe.
func (w *woodcutting) registerCrystalAxe(api *plugins.Api) {
	if api == nil {
		return
	}
	check := map[string]plugins.ItemActionHandler{"Check": w.checkCrystalCharges}
	unnoted := &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)}
	api.OnItemActionNamed("Crystal axe", check, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe", w.addShardCharges, unnoted)
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe (inactive)", w.addShardCharges, unnoted)
	api.OnItemActionNamed("Crystal felling axe", check, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal felling axe", w.addShardCharges, unnoted)
	api.OnItemOnItemNamed("Crystal shard", "Crystal felling axe (inactive)", w.addShardCharges, unnoted)
}
