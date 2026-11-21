// Crystal pickaxe charges (CrystalPickaxe.Mining.js): created
// with 10,000 charges, one consumed for each ore mined,
// recharged with crystal shards (100 each, up to 20,000). An
// empty pickaxe turns inactive and mines like a dragon pickaxe.
// Wearing an elven signet gives a 10% chance to save the charge.
package mining

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Crystal pickaxe constants (CrystalPickaxe.Mining.js:7-10).
const (
	crystalPickaxeStartCharges = 10000
	crystalPickaxeMaxCharges   = 20000
	crystalShardCharges        = 100
	crystalPickaxeMetaKey      = "crystal-pickaxe"
	crystalShardID             = 23962 // ItemIds.CRYSTAL_SHARD
	crystalPickaxeID           = 23680 // ItemIds.CRYSTAL_PICKAXE
)

// unchargedByChargedCrystal maps a charged crystal pickaxe to
// its inactive form (CrystalPickaxe.Mining.js:104-107).
var unchargedByChargedCrystalPickaxe = map[int]int{
	23680: 23682, // CRYSTAL_PICKAXE -> CRYSTAL_PICKAXE_INACTIVE_
	23863: 23682, // CRYSTAL_PICKAXE_3 (gauntlet) -> CRYSTAL_PICKAXE_INACTIVE_
}

// chargedByUnchargedCrystalPickaxe maps an inactive crystal
// pickaxe to its charged form (CrystalPickaxe.Mining.js:108-111).
var chargedByUnchargedCrystalPickaxe = map[int]int{
	23682: 23680, // CRYSTAL_PICKAXE_INACTIVE_ -> CRYSTAL_PICKAXE
	23683: 23680, // CRYSTAL_PICKAXE_INACTIVE__2 (placeholder) -> CRYSTAL_PICKAXE
}

// chargedCrystalPickaxeIDs is the chargedIds set: the keys of
// unchargedByCharged.
var chargedCrystalPickaxeIDs = map[int]bool{
	23680: true,
	23863: true,
}

// elvenSignetIDs are the ids wearingSignet checks
// (CrystalPickaxe.Mining.js:62-66): ELVEN_SIGNET (23943),
// ELVEN_SIGNET_2 (23944), ELVEN_SIGNET_3 (23945),
// CELESTIAL_SIGNET (25545), CELESTIAL_SIGNET_UNCHARGED_
// (25543).
var elvenSignetIDs = []int{23943, 23944, 23945, 25545, 25543}

// pickaxeSlot is one pickaxe in a container: the item as read,
// its slot, and whether it is wielded (the weapon slot) rather
// than carried (the woodcutting axeSlot precedent).
type pickaxeSlot struct {
	item    game.Item
	slot    int
	wielded bool
}

// writePickaxeItem writes item back to loc's slot: the
// equipment weapon slot when wielded, else the inventory slot.
func writePickaxeItem(player *game.Player, loc pickaxeSlot, item game.Item) {
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

// refreshPickaxeContainers refreshes the inventory and the
// equipment after a pickaxe changes form.
func refreshPickaxeContainers(player *game.Player) {
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

// chargesFromMeta reads {"charges": N} under key, reporting
// whether a finite number was stored (the woodcutting
// chargesFromMeta precedent).
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

// formatThousands renders an int with en-US grouping
// (toLocaleString("en-US")).
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

// crystalPickaxeCharges reads a pickaxe's crystal charges
// (CrystalPickaxe.Mining.js:17-23): the saved value clamped to
// [0, MAX_CHARGES], else START_CHARGES for a charged pickaxe
// with no saved value, else 0.
func crystalPickaxeCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, crystalPickaxeMetaKey); ok {
		return min(max(saved, 0), crystalPickaxeMaxCharges)
	}
	if chargedCrystalPickaxeIDs[item.ID()] {
		return crystalPickaxeStartCharges
	}
	return 0
}

// findChargedCrystalPickaxe finds the charged crystal pickaxe
// the player is wielding or carrying, wielded first
// (CrystalPickaxe.Mining.js:27-34).
func findChargedCrystalPickaxe(player *game.Player) (pickaxeSlot, bool) {
	if player == nil {
		return pickaxeSlot{}, false
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if chargedCrystalPickaxeIDs[weapon.ID()] {
			return pickaxeSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if chargedCrystalPickaxeIDs[item.ID()] {
				return pickaxeSlot{item: item, slot: i}, true
			}
		}
	}
	return pickaxeSlot{}, false
}

// useCrystalPickaxeCharge spends one charge of the pickaxe at
// loc (CrystalPickaxe.Mining.js:43-52).
func useCrystalPickaxeCharge(player *game.Player, loc pickaxeSlot) {
	left := crystalPickaxeCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(crystalPickaxeMetaKey, chargesMetaValue(left))
		writePickaxeItem(player, loc, item)
		return
	}
	uncharged, ok := unchargedByChargedCrystalPickaxe[loc.item.ID()]
	if !ok {
		return
	}
	item := loc.item
	item.SetID(uncharged)
	item.SetMetaValue(crystalPickaxeMetaKey, nil)
	writePickaxeItem(player, loc, item)
	refreshPickaxeContainers(player)
	player.SendMessage("Your crystal pickaxe has run out of charges.")
}

// tryUseCrystalCharge spends one charge for a mined ore;
// returns false when the signet saved it
// (CrystalPickaxe.Mining.js:68-77). Only the charged crystal
// pickaxe doing the mining is drained (the core loop calls
// this only when the mining pickaxe is CRYSTAL_PICKAXE).
func (m *mining) tryUseCrystalCharge(player *game.Player) bool {
	loc, ok := findChargedCrystalPickaxe(player)
	if !ok {
		return false
	}
	if wearingSignet(player) && m.randFloat() < 0.10 {
		return false
	}
	useCrystalPickaxeCharge(player, loc)
	return true
}

// wearingSignet reports whether the player wears an elven (or
// celestial) signet (CrystalPickaxe.Mining.js:58-66).
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

// checkCrystalPickaxeCharges answers the Check action
// (CrystalPickaxe.Mining.js:80-84): "Your crystal pickaxe has
// X charge(s) left."
func (m *mining) checkCrystalPickaxeCharges(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	left := crystalPickaxeCharges(actionItem(event.Player, event))
	suffix := "s"
	if left == 1 {
		suffix = ""
	}
	event.Player.SendMessage(fmt.Sprintf("Your crystal pickaxe has %s charge%s left.", formatThousands(left), suffix))
	return true
}

// actionItem reads the item an item-action event clicked: the
// inventory item at the slot or the equipped item at the slot
// (the woodcutting actionItem precedent).
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

// isCrystalPickaxe reports whether id is a crystal pickaxe in
// either form.
func isCrystalPickaxe(id int) bool {
	if chargedCrystalPickaxeIDs[id] {
		return true
	}
	_, ok := chargedByUnchargedCrystalPickaxe[id]
	return ok
}

// addCrystalShardCharges recharges a crystal pickaxe with
// crystal shards (CrystalPickaxe.Mining.js:86-116): the shard
// on the pickaxe in either direction; full → message +
// handled; uncharged → charged form; capped at MAX_CHARGES.
func (m *mining) addCrystalShardCharges(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	var pickaxe game.Item
	var slot int
	switch {
	case isCrystalPickaxe(event.UsedItemID):
		pickaxe, slot = event.UsedItem, event.UsedItemSlot
	case isCrystalPickaxe(event.UsedWithItemID):
		pickaxe, slot = event.UsedWithItem, event.UsedWithItemSlot
	default:
		return false
	}
	inv := player.Inventory()
	if inv == nil {
		return false
	}
	// Re-read the pickaxe from its slot: the event carries a
	// value copy, and the slot is the live item.
	if current := inv.ItemAt(slot); current.ID() == pickaxe.ID() {
		pickaxe = current
	}
	shardCount := inv.GetAmount(crystalShardID)
	if shardCount <= 0 {
		return false
	}
	left := crystalPickaxeCharges(pickaxe)
	if left >= crystalPickaxeMaxCharges {
		player.SendMessage("Your crystal pickaxe cannot hold any more charges.")
		return true
	}
	shards := min(shardCount, (crystalPickaxeMaxCharges-left+crystalShardCharges-1)/crystalShardCharges)
	inv.DeleteNumber(crystalShardID, shards)
	if charged, ok := chargedByUnchargedCrystalPickaxe[pickaxe.ID()]; ok {
		pickaxe.SetID(charged)
	}
	pickaxe.SetMetaValue(crystalPickaxeMetaKey,
		chargesMetaValue(min(crystalPickaxeMaxCharges, left+shards*crystalShardCharges)))
	inv.SetItem(slot, pickaxe)
	refreshPickaxeContainers(player)
	suffix := "s"
	if shards == 1 {
		suffix = ""
	}
	player.SendMessage(fmt.Sprintf("You add %d crystal shard%s to the pickaxe.", shards, suffix))
	return true
}

// registerCrystalPickaxe wires the CrystalPickaxe registrations
// (CrystalPickaxe.Mining.js:119-123).
func (m *mining) registerCrystalPickaxe(api *plugins.Api) {
	if api == nil {
		return
	}
	check := map[string]plugins.ItemActionHandler{"Check": m.checkCrystalPickaxeCharges}
	unnoted := &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)}
	api.OnItemActionNamed("Crystal pickaxe", check, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal pickaxe", m.addCrystalShardCharges, unnoted)
	api.OnItemOnItemNamed("Crystal shard", "Crystal pickaxe (inactive)", m.addCrystalShardCharges, unnoted)
}
