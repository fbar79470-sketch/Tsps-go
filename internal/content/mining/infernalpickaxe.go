// Infernal pickaxe (InfernalPickaxe.Mining.js): a 1/3 chance
// that each ore is destroyed as it is mined, granting about
// half the Smithing experience a bar would give and using one
// of 5,000 charges. Mining experience is unaffected. An empty
// pickaxe turns into its uncharged form, which mines like a
// dragon pickaxe. Created and recharged by using a smouldering
// stone (or dragon pickaxe) on it at 85 Smithing.
package mining

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Infernal pickaxe constants (InfernalPickaxe.Mining.js:7-10).
const (
	infernalCombustChance = 3
	infernalMaxCharges    = 5000
	infernalMetaKey       = "infernal-pickaxe"
	infernalSmithingLevel = 85
	infernalPickaxeID     = 13243 // ItemIds.INFERNAL_PICKAXE
	smoulderingStoneID    = 13233 // ItemIds.SMOULDERING_STONE
	dragonPickaxeID       = 11920 // ItemIds.DRAGON_PICKAXE
)

// unchargedByChargedInfernal maps a charged infernal pickaxe to
// its uncharged form (InfernalPickaxe.Mining.js:102-106).
var unchargedByChargedInfernal = map[int]int{
	13243: 13244, // INFERNAL_PICKAXE -> INFERNAL_PICKAXE_UNCHARGED_
	25063: 25369, // INFERNAL_PICKAXE_OR_ -> INFERNAL_PICKAXE_UNCHARGED__2
	30345: 30346, // INFERNAL_PICKAXE_OR__3 -> INFERNAL_PICKAXE_UNCHARGED__4
}

// chargedInfernalPickaxeIDs is the chargedIds set: the keys of
// unchargedByCharged.
var chargedInfernalPickaxeIDs = map[int]bool{
	13243: true,
	25063: true,
	30345: true,
}

// infernalRechargeSourceIDs are the items that create/recharge
// an infernal pickaxe (InfernalPickaxe.Mining.js:107-110).
var infernalRechargeSourceIDs = map[int]bool{
	smoulderingStoneID: true,
	dragonPickaxeID:    true,
}

// infernalOreXP is the Smithing experience per combusted ore
// (InfernalPickaxe.Mining.js:111-134), already the halved bar
// experience. Fractional XP truncates at the grant site.
var infernalOreXP = map[int]float64{
	668:   4,     // BLURITE_ORE
	436:   3.6,   // COPPER_ORE
	438:   3.6,   // TIN_ORE
	440:   5.5,   // IRON_ORE
	442:   5.5,   // SILVER_ORE
	453:   6,     // COAL
	444:   9,     // GOLD_ORE
	447:   12,    // MITHRIL_ORE
	449:   18.75, // ADAMANTITE_ORE
	451:   25,    // RUNITE_ORE
	6971:  10.2,  // SANDSTONE_1KG_
	6975:  19.2,  // SANDSTONE_5KG_
	6977:  28.2,  // SANDSTONE_10KG_
	6979:  7.2,   // GRANITE_500G_
	6981:  13.2,  // GRANITE_2KG_
	6983:  22.2,  // GRANITE_5KG_
	34020: 2,     // SUNSTONE
	31722: 2,     // RUBIUM_SPLINTERS
}

// infernalCharges reads an infernal pickaxe's charges
// (InfernalPickaxe.Mining.js:20-23): the saved value clamped to
// [0, MAX_CHARGES], else MAX_CHARGES (an uncharged pickaxe's
// default — it is never spent, since tryCombustOre only runs
// for charged ids).
func infernalCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, infernalMetaKey); ok {
		return min(max(saved, 0), infernalMaxCharges)
	}
	return infernalMaxCharges
}

// findChargedInfernalPickaxe finds the charged infernal
// pickaxe (of pickaxeId) the player is wielding or carrying,
// wielded first (InfernalPickaxe.Mining.js:25-31).
func findChargedInfernalPickaxe(player *game.Player, pickaxeID int) (pickaxeSlot, bool) {
	if player == nil {
		return pickaxeSlot{}, false
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if weapon.ID() == pickaxeID {
			return pickaxeSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if item.ID() == pickaxeID {
				return pickaxeSlot{item: item, slot: i}, true
			}
		}
	}
	return pickaxeSlot{}, false
}

// useInfernalCharge spends one charge of the pickaxe at loc
// (InfernalPickaxe.Mining.js:33-45).
func useInfernalCharge(player *game.Player, loc pickaxeSlot) {
	left := infernalCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(infernalMetaKey, chargesMetaValue(left))
		writePickaxeItem(player, loc, item)
		return
	}
	item := loc.item
	if uncharged, ok := unchargedByChargedInfernal[item.ID()]; ok {
		item.SetID(uncharged)
	}
	item.SetMetaValue(infernalMetaKey, nil)
	writePickaxeItem(player, loc, item)
	refreshPickaxeContainers(player)
	player.SendMessage("Your infernal pickaxe has run out of charges.")
}

// tryCombustOre incinerates the ore instead of adding it
// (InfernalPickaxe.Mining.js:48-61). Returns true when the ore
// was combusted.
func (m *mining) tryCombustOre(player *game.Player, pickaxeID, oreID int) bool {
	xp, ok := infernalOreXP[oreID]
	if !chargedInfernalPickaxeIDs[pickaxeID] || !ok || !(xp > 0) || int(m.randFloat()*infernalCombustChance) != 0 {
		return false
	}
	loc, found := findChargedInfernalPickaxe(player, pickaxeID)
	if !found {
		return false
	}
	player.Skills().AddExperiences(game.SkillSmithing, int(xp))
	useInfernalCharge(player, loc)
	return true
}

// handleInfernalCreation creates or recharges an infernal
// pickaxe (InfernalPickaxe.Mining.js:64-88). Creation needs 85
// Smithing; recharging an uncharged pickaxe only restores
// charges. Returns true (handled) in both cases and when the
// level is too low.
func (m *mining) handleInfernalCreation(player *game.Player, sourceItem, pickaxeItem game.Item, pickaxeSlot int) bool {
	if player == nil {
		return true
	}
	if player.Skills().CurrentLevel(game.SkillSmithing) < infernalSmithingLevel {
		player.SendMessage("You need a Smithing level of 85 to do this.")
		return true
	}
	creating := pickaxeItem.ID() == dragonPickaxeID
	if creating {
		player.Skills().AddExperiences(game.SkillMining, 200)
		player.Skills().AddExperiences(game.SkillSmithing, 350)
	}
	inv := player.Inventory()
	if inv == nil {
		return true
	}
	inv.DeleteNumber(sourceItem.ID(), 1)
	pickaxeItem.SetID(infernalPickaxeID)
	pickaxeItem.SetMetaValue(infernalMetaKey, chargesMetaValue(infernalMaxCharges))
	inv.SetItem(pickaxeSlot, pickaxeItem)
	refreshPickaxeContainers(player)
	if creating {
		player.SendMessage("You infuse the pickaxe with the smouldering stone; it bursts into flame.")
	} else {
		player.SendMessage("You recharge your infernal pickaxe.")
	}
	return true
}

// handleInfernalRecharge handles a smouldering stone (or dragon
// pickaxe) used on an uncharged infernal pickaxe
// (InfernalPickaxe.Mining.js:137-147).
func (m *mining) handleInfernalRecharge(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	var pickaxeItem game.Item
	var pickaxeSlot int
	var sourceItem game.Item
	switch {
	case event.UsedItemID == 13244: // INFERNAL_PICKAXE_UNCHARGED_
		pickaxeItem, pickaxeSlot = event.UsedItem, event.UsedItemSlot
		sourceItem = event.UsedWithItem
	case event.UsedWithItemID == 13244:
		pickaxeItem, pickaxeSlot = event.UsedWithItem, event.UsedWithItemSlot
		sourceItem = event.UsedItem
	default:
		return false
	}
	if !infernalRechargeSourceIDs[sourceItem.ID()] {
		return false
	}
	inv := event.Player.Inventory()
	if inv == nil {
		return false
	}
	if current := inv.ItemAt(pickaxeSlot); current.ID() == pickaxeItem.ID() {
		pickaxeItem = current
	}
	return m.handleInfernalCreation(event.Player, sourceItem, pickaxeItem, pickaxeSlot)
}

// handleInfernalCreationUse handles a smouldering stone used on
// a tradeable dragon pickaxe (InfernalPickaxe.Mining.js:150-159).
func (m *mining) handleInfernalCreationUse(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	var stoneItem game.Item
	var pickaxeItem game.Item
	var pickaxeSlot int
	switch {
	case event.UsedItemID == smoulderingStoneID:
		stoneItem = event.UsedItem
		pickaxeItem, pickaxeSlot = event.UsedWithItem, event.UsedWithItemSlot
	case event.UsedWithItemID == smoulderingStoneID:
		stoneItem = event.UsedWithItem
		pickaxeItem, pickaxeSlot = event.UsedItem, event.UsedItemSlot
	default:
		return false
	}
	if pickaxeItem.ID() != dragonPickaxeID {
		return false
	}
	inv := event.Player.Inventory()
	if inv == nil {
		return false
	}
	if current := inv.ItemAt(pickaxeSlot); current.ID() == pickaxeItem.ID() {
		pickaxeItem = current
	}
	return m.handleInfernalCreation(event.Player, stoneItem, pickaxeItem, pickaxeSlot)
}

// registerInfernalPickaxe wires the InfernalPickaxe
// registrations (InfernalPickaxe.Mining.js:137-159): the two
// unnamed item-on-item handlers, in TS order.
func (m *mining) registerInfernalPickaxe(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnItemOnItem(func(event *plugins.PluginItemOnItemEvent) {
		if m.handleInfernalRecharge(event) {
			event.Handled = true
		}
	}, nil)
	api.OnItemOnItem(func(event *plugins.PluginItemOnItemEvent) {
		if m.handleInfernalCreationUse(event) {
			event.Handled = true
		}
	}, nil)
}
