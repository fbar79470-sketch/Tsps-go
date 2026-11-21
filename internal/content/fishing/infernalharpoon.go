// InfernalHarpoon is the Go port of plugins/skills/fishing/
// InfernalHarpoon.Fishing.js (commit bfd81e3): the infernal
// harpoon's 1/3 chance that each fish it catches is cooked
// and destroyed, giving half that fish's Cooking
// experience and using one of 5,000 charges; Fishing
// experience is unaffected. With no charges left it turns
// into its uncharged form, which fishes like a dragon
// harpoon. A smouldering stone on a dragon harpoon makes
// one (75 Fishing and 85 Cooking, max levels, not
// boostable; 200 Fishing and 350 Cooking experience), and
// a smouldering stone or another dragon harpoon recharges
// an uncharged one. Harpoons with no charge data yet count
// as fully charged.
//
// All harpoon-form ids are ItemIdentifiers.ts values,
// verified against the real rev-242 seed cache by display
// name (probe, 2026-11-20): charged 21031 ("Infernal
// harpoon"), 25059 and 30342 ("Infernal harpoon (or)");
// uncharged 21033, 25367, 30343 (all "Infernal harpoon
// (uncharged)"). The maps are verbatim from the unit file.
// The smouldering stone and dragon harpoon ids come from
// the resolved FishingIDs.
//
// HONEST DEGRADATION: tryCookFish emits "cooking:raw-xp"
// and the cook fires only when a handler answers with xp >
// 0 — but Cooking is NOT STARTED, so no Go answerer exists
// yet, the request stays unanswered, and the cook NEVER
// fires. The cook is wired into the core loop anyway
// (landCatch, per fish, exactly as the loop's infernal
// call site has it) so it starts working the moment a
// Cooking port answers the query; no answerer is invented
// here. A test pins the never-fires behaviour (the
// infernal-axe precedent, Phase 33). tryCookKept needs no
// answerer — the 1/3 roll and the charge are its whole
// behaviour — so it fires today.
//
// Go differences: TS mutates the live Item reference; Go
// Items are values, so mutations write back through the
// container at the carried slot (itemSlot / writeSlotItem
// in crystalharpoon.go). Cooking XP is int in Go (TS
// stores tenths as floats), so the half-XP grant
// truncates toward zero, like the Fishing XP truncation
// in landCatch.
package fishing

import (
	"fmt"
	"math/rand/v2"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ---- InfernalHarpoon constants (InfernalHarpoon.Fishing.js:8-12) ----

const (
	// infernalHarpoonCookChance is COOK_CHANCE: the cook
	// roll is 1 in 3 (Math.floor(Math.random() * 3) === 0).
	infernalHarpoonCookChance = 3
	// infernalHarpoonMaxCharges is MAX_CHARGES: a harpoon
	// holds at most 5,000 charges.
	infernalHarpoonMaxCharges = 5000
	// infernalHarpoonChargesMetaKey is CHARGES_META_KEY:
	// the item-meta key holding {"charges": N}.
	infernalHarpoonChargesMetaKey = "infernal-harpoon"
	// infernalHarpoonCreateFishingLevel is
	// CREATE_LEVELS.fishing: 75 Fishing (max level) to make
	// an infernal harpoon.
	infernalHarpoonCreateFishingLevel = 75
	// infernalHarpoonCreateCookingLevel is
	// CREATE_LEVELS.cooking: 85 Cooking (max level) to make
	// an infernal harpoon.
	infernalHarpoonCreateCookingLevel = 85
	// infernalHarpoonCreateFishingXP is CREATE_XP.fishing:
	// making one grants 200 Fishing XP.
	infernalHarpoonCreateFishingXP = 200
	// infernalHarpoonCreateCookingXP is CREATE_XP.cooking:
	// making one grants 350 Cooking XP.
	infernalHarpoonCreateCookingXP = 350
	// cookingRawXPEventName is the "cooking:raw-xp" custom
	// event: the querier asks Cooking for the Cooking XP of
	// a raw fish; the answerer sets XP (and HasXP) on the
	// request. No answerer exists yet — see the file header.
	cookingRawXPEventName = "cooking:raw-xp"
)

// unchargedByChargedInfernalHarpoon is the unit file's
// unchargedByCharged map, verbatim: INFERNAL_HARPOON
// (21031) → INFERNAL_HARPOON_UNCHARGED_ (21033);
// INFERNAL_HARPOON_OR_ (25059) →
// INFERNAL_HARPOON_UNCHARGED__2 (25367);
// INFERNAL_HARPOON_OR__3 (30342) →
// INFERNAL_HARPOON_UNCHARGED__4 (30343).
// (ItemIdentifiers.ts:19378, 19380, 23120, 23392,
// 28156-28157.)
var unchargedByChargedInfernalHarpoon = map[int]int{
	21031: 21033,
	25059: 25367,
	30342: 30343,
}

// chargedByUnchargedInfernalHarpoon is the inverse map:
// each uncharged form recharges to its charged form.
var chargedByUnchargedInfernalHarpoon = map[int]int{
	21033: 21031,
	25367: 25059,
	30343: 30342,
}

// infernalHarpoonID is the made harpoon's id:
// INFERNAL_HARPOON (21031).
const infernalHarpoonID = 21031

// CookingRawXPRequest is the payload of the
// "cooking:raw-xp" custom event
// (InfernalHarpoon.Fishing.js:81-84): the raw fish id and
// the answer — the fish's Cooking XP, with HasXP reporting
// whether any handler answered. The querier reads XP after
// the emit returns; with no handlers it stays unanswered.
type CookingRawXPRequest struct {
	Player *game.Player
	RawID  int
	XP     int
	HasXP  bool
}

// infernalHarpoonCharges reads an item's infernal-harpoon
// charges (InfernalHarpoon.Fishing.js:14-17): the saved
// value clamped to [0, MAX_CHARGES], else MAX_CHARGES — a
// harpoon with no charge data yet is fully charged.
func infernalHarpoonCharges(item game.Item) int {
	if saved, ok := chargesFromMeta(item, infernalHarpoonChargesMetaKey); ok {
		return min(max(saved, 0), infernalHarpoonMaxCharges)
	}
	return infernalHarpoonMaxCharges
}

// findChargedInfernalHarpoon finds the charged infernal
// harpoon the player is wielding or carrying, wielded first
// (InfernalHarpoon.Fishing.js:20-25): the weapon slot when
// its id is a charged infernal harpoon, else the first
// inventory item with a charged infernal harpoon id.
func findChargedInfernalHarpoon(player *game.Player) (itemSlot, bool) {
	if player == nil {
		return itemSlot{}, false
	}
	if eq := player.Equipment(); eq != nil {
		weapon := eq.ItemAt(game.WeaponSlot)
		if _, ok := unchargedByChargedInfernalHarpoon[weapon.ID()]; ok {
			return itemSlot{item: weapon, slot: game.WeaponSlot, wielded: true}, true
		}
	}
	if inv := player.Inventory(); inv != nil {
		for i, item := range inv.Items() {
			if _, ok := unchargedByChargedInfernalHarpoon[item.ID()]; ok {
				return itemSlot{item: item, slot: i}, true
			}
		}
	}
	return itemSlot{}, false
}

// useInfernalHarpoonCharge spends one charge of the harpoon
// at loc (InfernalHarpoon.Fishing.js:27-37): decrement the
// saved charges, or at 0 set the id to the uncharged form,
// clear the meta, refresh, and message.
func useInfernalHarpoonCharge(player *game.Player, loc itemSlot) {
	left := infernalHarpoonCharges(loc.item) - 1
	if left > 0 {
		item := loc.item
		item.SetMetaValue(infernalHarpoonChargesMetaKey, chargesMetaValue(left))
		writeSlotItem(player, loc, item)
		return
	}
	uncharged, ok := unchargedByChargedInfernalHarpoon[loc.item.ID()]
	if !ok {
		return
	}
	item := loc.item
	item.SetID(uncharged)
	item.SetMetaValue(infernalHarpoonChargesMetaKey, nil)
	writeSlotItem(player, loc, item)
	refreshContainers(player)
	player.SendMessage("Your infernal harpoon has run out of charges.")
}

// tryCookFish cooks the just-caught fish instead of putting
// it in the inventory (InfernalHarpoon.Fishing.js:79-91),
// in TS order: no charged infernal harpoon, or the 1/3 roll
// misses → false; ask Cooking for the fish's raw XP — no
// positive XP → false (with no Cooking answerer yet, the
// cook never fires: see the file header); grant half the
// Cooking XP; spend one charge; return true. The caller is
// landCatch, per fish.
func (f *fishing) tryCookFish(player *game.Player, rawID int) bool {
	loc, ok := findChargedInfernalHarpoon(player)
	if !ok {
		return false
	}
	if rand.Float64()*infernalHarpoonCookChance >= 1 {
		return false
	}
	request := &CookingRawXPRequest{Player: player, RawID: rawID}
	f.emitCustomEvent(cookingRawXPEventName, request)
	if !request.HasXP || request.XP <= 0 {
		return false
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillCooking, request.XP/2)
	}
	useInfernalHarpoonCharge(player, loc)
	return true
}

// tryCookKept cooks a catch that is kept anyway, as
// Tempoross's harpoonfish are (InfernalHarpoon.Fishing.js:
// 93-104): the 1/3 roll and the charge, with the caller
// giving the cooked fish and its experience. It needs no
// Cooking answerer, so it fires today.
func (f *fishing) tryCookKept(player *game.Player) bool {
	loc, ok := findChargedInfernalHarpoon(player)
	if !ok {
		return false
	}
	if rand.Float64()*infernalHarpoonCookChance >= 1 {
		return false
	}
	useInfernalHarpoonCharge(player, loc)
	return true
}

// checkInfernalHarpoon answers the Check action
// (InfernalHarpoon.Fishing.js:110-114): "Infernal harpoon:
// X.X% remaining." It returns false — the click falls
// through — when the item is not a charged infernal
// harpoon.
func (f *fishing) checkInfernalHarpoon(event *plugins.PluginItemActionEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	item := actionItem(event.Player, event)
	if _, ok := unchargedByChargedInfernalHarpoon[item.ID()]; !ok {
		return false
	}
	pct := float64(infernalHarpoonCharges(item)) / float64(infernalHarpoonMaxCharges) * 100
	event.Player.SendMessage(fmt.Sprintf("Infernal harpoon: %.1f%% remaining.", pct))
	return true
}

// harpoonSlotOf returns the inventory slot of whichever of
// the two used items has one of ids (slotOf,
// InfernalHarpoon.Fishing.js:117-121), or -1.
func harpoonSlotOf(event *plugins.PluginItemOnItemEvent, ids map[int]bool) int {
	if ids[event.UsedItemID] {
		return event.UsedItemSlot
	}
	if ids[event.UsedWithItemID] {
		return event.UsedWithItemSlot
	}
	return -1
}

// harpoonOtherID returns the id of the used item that is not
// in slot (otherId, InfernalHarpoon.Fishing.js:123-125).
func harpoonOtherID(event *plugins.PluginItemOnItemEvent, slot int) int {
	if slot == event.UsedItemSlot {
		return event.UsedWithItemID
	}
	return event.UsedItemID
}

// makeInfernalHarpoon infuses a dragon harpoon with a
// smouldering stone (InfernalHarpoon.Fishing.js:127-148):
// 75 Fishing and 85 Cooking (max levels) or the refusal
// line; the stone is consumed, the harpoon becomes an
// infernal harpoon with no charge data (fully charged),
// and 200 Fishing + 350 Cooking XP are granted. It always
// claims the use — TS returns undefined from the handler,
// which claims it (PluginManager.ts:4139), including on
// the level refusal.
func (f *fishing) makeInfernalHarpoon(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil || f == nil || f.ids == nil {
		return false
	}
	player := event.Player
	skills := player.Skills()
	if skills == nil ||
		skills.MaxLevel(game.SkillFishing) < infernalHarpoonCreateFishingLevel ||
		skills.MaxLevel(game.SkillCooking) < infernalHarpoonCreateCookingLevel {
		player.SendMessage(fmt.Sprintf("You need a Fishing level of %d and a Cooking level of %d to do that.",
			infernalHarpoonCreateFishingLevel, infernalHarpoonCreateCookingLevel))
		return true
	}
	slot := harpoonSlotOf(event, map[int]bool{f.ids.DragonHarpoon: true})
	inv := player.Inventory()
	if inv == nil || slot < 0 {
		return true
	}
	harpoon := inv.ItemAt(slot)
	if harpoon.ID() != f.ids.DragonHarpoon {
		return true
	}
	inv.DeleteNumber(harpoonOtherID(event, slot), 1)
	harpoon.SetID(infernalHarpoonID)
	harpoon.SetMetaValue(infernalHarpoonChargesMetaKey, nil)
	inv.SetItem(slot, harpoon)
	refreshContainers(player)
	skills.AddExperiences(game.SkillFishing, infernalHarpoonCreateFishingXP)
	skills.AddExperiences(game.SkillCooking, infernalHarpoonCreateCookingXP)
	player.SendMessage("You infuse the dragon harpoon with the smouldering stone.")
	return true
}

// rechargeInfernalHarpoon recharges an uncharged infernal
// harpoon with a smouldering stone or a dragon harpoon
// (InfernalHarpoon.Fishing.js:150-162): the other item is
// consumed, the harpoon becomes its charged form with no
// charge data (fully charged), and the 5,000-charge line is
// sent. It always claims the use.
func (f *fishing) rechargeInfernalHarpoon(event *plugins.PluginItemOnItemEvent) bool {
	if event == nil || event.Player == nil {
		return false
	}
	player := event.Player
	slot := harpoonSlotOf(event, chargedByUnchargedInfernalHarpoonSet())
	inv := player.Inventory()
	if inv == nil || slot < 0 {
		return true
	}
	harpoon := inv.ItemAt(slot)
	charged, ok := chargedByUnchargedInfernalHarpoon[harpoon.ID()]
	if !ok {
		return true
	}
	inv.DeleteNumber(harpoonOtherID(event, slot), 1)
	harpoon.SetID(charged)
	harpoon.SetMetaValue(infernalHarpoonChargesMetaKey, nil)
	inv.SetItem(slot, harpoon)
	refreshContainers(player)
	player.SendMessage(fmt.Sprintf("Your infernal harpoon has been recharged with %s charges.",
		formatThousands(infernalHarpoonMaxCharges)))
	return true
}

// chargedByUnchargedInfernalHarpoonSet is the
// chargedByUncharged key set, for slotOf lookups.
func chargedByUnchargedInfernalHarpoonSet() map[int]bool {
	set := make(map[int]bool, len(chargedByUnchargedInfernalHarpoon))
	for id := range chargedByUnchargedInfernalHarpoon {
		set[id] = true
	}
	return set
}

// registerInfernalHarpoon wires the InfernalHarpoon
// registrations (InfernalHarpoon.Fishing.js:164-175), in TS
// order: Check per charged harpoon name, then the
// make/recharge pairs.
func (f *fishing) registerInfernalHarpoon(api *plugins.Api) {
	if api == nil {
		return
	}
	check := map[string]plugins.ItemActionHandler{"Check": f.checkInfernalHarpoon}
	unnoted := &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)}
	api.OnItemActionNamed("Infernal harpoon", check, nil)
	api.OnItemActionNamed("Infernal harpoon (or)", check, nil)
	api.OnItemOnItemNamed("Smouldering stone", "Dragon harpoon", f.makeInfernalHarpoon, unnoted)
	api.OnItemOnItemNamed("Smouldering stone", "Infernal harpoon (uncharged)", f.rechargeInfernalHarpoon, unnoted)
	api.OnItemOnItemNamed("Dragon harpoon", "Infernal harpoon (uncharged)", f.rechargeInfernalHarpoon, unnoted)
}
