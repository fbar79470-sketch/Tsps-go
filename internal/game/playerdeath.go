package game

import (
	"math"
	"sort"
)

// Player death: the port of the respawn-tick item loss in
// task/impl/PlayerDeath.ts (441 lines) at commit bfd81e3, as far
// as core goes. The respawn tick itself lives in combat_death.go
// (respawnPlayer); this file holds the kept-items / drop
// functions and the resetAttributes subset.
//
// TS ORDER (PlayerDeathTask case 0): death message + jingle 90,
// onShouldDropItemsOnDeath first-answer gate (default true),
// player:death-bones event + a BONES ground item (owner = killer
// else player), dropItems if losing, resetAttributes, kept items
// returned, emitPlayerDefeated for ALL deaths, save "death",
// onPlayerDeath handled chain (first handler setting handled
// stops it), player:died custom event, if not handled:
// player:respawn custom event, else moveTo(DEFAULT_LOCATION),
// save "respawn". On error: resetAttributes + move home.
//
// DEGRADATIONS, each documented where it bites:
//
//   - deathComponents splitting IS done (splitIntoComponents):
//     an unkept item with tradeable components, lost to
//     another player in the Wilderness, drops as its
//     components (PlayerDeath.ts splitIntoComponents +
//     item-gameplay.json deathComponents, 10 entries).
//   - keepsEverything is FALSE in Go: TS keeps everything for
//     OWNER/DEVELOPER rights unless ::mortal, and Go has no
//     rights field (Phase 20 documented partial).
//   - Barrows items are ordinary untradeables here: TS drops them
//     to the floor (Barrows.isBarrowsItem, via baseItemId), and
//     Go has no Barrows identification seam, so they land in
//     lostUntradeables and come back with the kept items.
//   - Ironman loot-owner rule: Go has no Ironman mode, so the
//     player-death:loot-owner query is the only override.
//   - Graves are plugin content: core emits onPlayerDeathDrops
//     and, with no taker, drops to the floor. No grave is built.
//   - Animation 836 is recorded as DeathAnimationID
//     (combat_death.go) and never sent: no performAnimation
//     exists yet (Phase 29).

// DeathJingleID is the respawn-tick jingle (PlayerDeath.ts
// case 0: sendJingle(90); PacketSender.sendJingle(id, delay=0)).
const DeathJingleID = 90

// DeathBonesItemID is ItemIdentifiers.BONES (526, "Bones"): the
// ground item a death leaves when the player:death-bones event
// allows it.
const DeathBonesItemID = 526

// DeathWaiveCosmeticsVarbit is the varbit behind
// cosmeticProtectionOn (PlayerDeath.ts): varbit 10987
// (death_waivecosmeticsprotection), 1 when the player waived
// cosmetic death protection.
const DeathWaiveCosmeticsVarbit = 10987

// ProtectItemPrayerRequirement is the Prayer level Protect Item
// needs (PrayerData.PROTECT_ITEM = new PrayerData(25, 2, 5617,
// 4112): requirement first, 25).
const ProtectItemPrayerRequirement = 25

// DefaultRespawnLocation is GameConstants.DEFAULT_LOCATION =
// WORLD_SPAWN (WorldDefinition.ts): the world.json spawn,
// (3089, 3524, 0). The coordinator overrides it from the
// world's live spawn at boot (SetDefaultRespawnLocation); the
// constant value is the world.json ground truth, so an
// unwired run still teleports home.
var DefaultRespawnLocation = Location{X: 3089, Y: 3524, Z: 0}

// SetDefaultRespawnLocation overrides the respawn teleport
// target, following the SetNpcRemover precedent: the wiring
// installs the world's live spawn, and the default above is
// what an unwired run uses.
func SetDefaultRespawnLocation(loc Location) {
	DefaultRespawnLocation = loc
}

// JingleSender is the optional SessionPort extension for sending
// a jingle (the MultiIconSender precedent, areamanager.go):
// Session.SendJingle. A session-less player, or a SessionPort
// without the extension, hears nothing.
type JingleSender interface {
	SendJingle(id, delay int)
}

// sendDeathJingle sends jingle 90 through the player's session
// when it carries the extension, and drops it otherwise.
func sendDeathJingle(player *Player) {
	if player == nil {
		return
	}
	if sender, ok := player.Session.(JingleSender); ok {
		sender.SendJingle(DeathJingleID, 0)
	}
}

// --- custom-event payloads (C4; names verbatim) ---

// DeathBonesEventName is the "player:death-bones" custom event
// (PlayerDeath.ts case 0): {player, killer, location, allow}.
const DeathBonesEventName = "player:death-bones"

// DeathBonesEvent is the player:death-bones payload. Allow starts
// true; a handler that sets it false suppresses the bones.
type DeathBonesEvent struct {
	Player   *Player
	Killer   *Player
	Location Location
	Allow    bool
}

// PlayerDiedEventName is the "player:died" custom event
// (PlayerDeath.ts case 0): every observer hears the death,
// whichever death hook took it.
const PlayerDiedEventName = "player:died"

// PlayerDiedEvent is the player:died payload.
type PlayerDiedEvent struct {
	Player    *Player
	Killer    *Player
	ItemsLost bool
}

// PlayerRespawnEventName is the "player:respawn" custom event
// (PlayerDeath.ts case 0): respawn policies run after every
// dangerous-death observer.
const PlayerRespawnEventName = "player:respawn"

// PlayerRespawnEvent is the player:respawn payload. Handled
// starts false; a handler that sets it takes the respawn over
// and the core teleport home is skipped.
type PlayerRespawnEvent struct {
	Player  *Player
	Killer  *Player
	Handled bool
}

// DeathLootOwnerEventName is the "player-death:loot-owner"
// custom event (PlayerDeath.ts lootOwner): {player, killer,
// owner}.
const DeathLootOwnerEventName = "player-death:loot-owner"

// DeathLootOwnerQuery is the player-death:loot-owner payload.
// Owner starts as the killer; a handler may reassign it.
type DeathLootOwnerQuery struct {
	Player *Player
	Killer *Player
	Owner  *Player
}

// DeathCosmeticEventName is the "player-death:cosmetic" custom
// event (PlayerDeath.ts isCosmetic): {player, item, cosmetic}.
const DeathCosmeticEventName = "player-death:cosmetic"

// DeathCosmeticQuery is the player-death:cosmetic payload.
// Cosmetic starts false; a handler (OrnamentKits, content)
// sets it true for an ornamented item. With no answerer in
// core, no item is cosmetic in core.
type DeathCosmeticQuery struct {
	Player   *Player
	Item     Item
	Cosmetic bool
}

// --- game-side hook event shapes (CombatHooks seam) ---

// PlayerDeathLayout is the per-slot item-id layout before the
// death emptied the containers (-1 for an empty slot): the
// `layout` field of PluginPlayerDeathDropsEvent
// (PluginTypes.ts:521).
type PlayerDeathLayout struct {
	Inventory []int
	Equipment []int
}

// DeathItemDropEvent is the game-side form of
// PluginPlayerDeathItemDropEvent (PluginTypes.ts:489-505): one
// item the death is about to lose, run through the
// onPlayerDeathItemDrop hooks. Handled skips the item;
// SuppressDefaultDrop drops nothing for it (but does not stop
// later death-drop hooks); ExtraDrops are lost with it.
type DeathItemDropEvent struct {
	Player              *Player
	Killer              *Player
	Item                Item
	Kept                bool
	Location            Location
	ShouldDropItems     bool
	DropEligible        bool
	SuppressDefaultDrop bool
	Handled             bool
	ExtraDrops          []Item
}

// DeathDropsEvent is the game-side form of
// PluginPlayerDeathDropsEvent (PluginTypes.ts:512-522):
// everything a death is about to lose, once, after the
// per-item hooks. A hook takes items by reassigning Items or
// Untradeables; whatever is left falls to the floor or is
// kept, as without the hook.
type DeathDropsEvent struct {
	Player       *Player
	Killer       *Player
	Location     Location
	Items        []Item
	Untradeables []Item
	Layout       PlayerDeathLayout
}

// deathValueSource is the value-reading subset of the item
// definition source that keepValue needs (the ShopItemDefinitionSource
// value methods, without taking a dependency on the shop
// package surface): GE quote presence/value, high alchemy
// value, store value. The production source
// (cmd/server storeItemDefinitions) implements all of them;
// a player whose source lacks them ranks every item 0.
type deathValueSource interface {
	ItemHasGrandExchangeValue(id int) bool
	ItemGrandExchangeValue(id int) int
	ItemHighAlchValue(id int) int
	ItemValue(id int) int
}

// KeepValue is PlayerDeathTask.keepValue: an item's worth when a
// death picks the kept ones — its Grand Exchange guide price,
// or its high alchemy value when it has none, or floor(store
// value x 0.6) when it cannot be alchemised
// (ItemDefinition.ts:190-210: getGrandExchangeValue falls back
// to the store value, getHighAlchValue returns the raw value,
// 0 when unset — the 60% fallback lives here, in keepValue,
// not in the definition). A nil source reads every value as 0,
// so every item ranks 0 and the sort keeps container order.
func KeepValue(item Item, src deathValueSource) int {
	if src == nil {
		return 0
	}
	if src.ItemHasGrandExchangeValue(item.ID()) {
		return src.ItemGrandExchangeValue(item.ID())
	}
	if alch := src.ItemHighAlchValue(item.ID()); alch > 0 {
		return alch
	}
	value := src.ItemValue(item.ID())
	if value < 0 {
		value = 0
	}
	return int(math.Floor(float64(value) * 0.6))
}

// AmountToKeep is PlayerDeathTask.getAmountToKeep: a red skull
// keeps 0; a skull keeps 0; otherwise 3, plus 1 with Protect
// Item (max Prayer level >= its requirement AND current Prayer
// > 0 AND PROTECT_ITEM active — PrayerHandler.isActivated).
// The skull state is read live (Player.SkullTimer/SkullType,
// Worker B): a red skull and a white skull both keep 0, so
// the type only matters for the red-skull branch, which is
// kept explicit to match TS.
func AmountToKeep(player *Player) int {
	if player == nil {
		return 3
	}
	if player.SkullTimer() > 0 && player.SkullType() == SkullTypeRed {
		return 0
	}
	skills := player.Skills()
	protectItem := skills != nil &&
		skills.MaxLevel(SkillPrayer) >= ProtectItemPrayerRequirement &&
		skills.CurrentLevel(SkillPrayer) > 0 &&
		PrayerActive(&player.Mobile, PrayerProtectItem)
	kept := 3
	if player.SkullTimer() > 0 {
		kept = 0
	}
	if protectItem {
		kept++
	}
	return kept
}

// ProtectsCosmetics is PlayerDeathTask.protectsCosmetics: false
// for a PvP kill or in the Wilderness; else the varbit 10987
// setting (cosmeticProtectionOn: the varbit is 1 when the
// player waived it). The varbit is read through the session's
// VarbitReader extension (examine_price.go); a session-less
// player, or a session without the extension, reads 0 —
// default on when unset, as the task specifies.
func ProtectsCosmetics(player, killer *Player) bool {
	if player == nil {
		return false
	}
	if killer != nil && killer != player {
		return false
	}
	if combatInWilderness(player.Location()) {
		return false
	}
	return cosmeticProtectionOn(player)
}

// cosmeticProtectionOn is PlayerDeathTask.cosmeticProtectionOn:
// varbit 10987 is 1 when the player waived cosmetic death
// protection.
func cosmeticProtectionOn(player *Player) bool {
	if player == nil || player.Session == nil {
		return true
	}
	reader, ok := player.Session.(VarbitReader)
	if !ok {
		return true
	}
	return reader.GetVarbit(DeathWaiveCosmeticsVarbit) != 1
}

// GetKeptItems is PlayerDeathTask.getKeptItems: the kept items
// by why they are kept — ranked the most valuable up to the
// count, always those a plugin keeps beyond it
// (onShouldKeepItemOnDeath true), and cosmetic the ornamented
// items `cosmetics` protects that were not valuable enough for
// a ranked slot. Untradeable items compete for the kept slots
// too, by value; lost-on-death items (the untradeable meta
// flag) never take one; onShouldKeepItemOnDeath false never
// keeps. The sort is stable, so equal-valued items keep
// container order (inventory before equipment, slot order).
func GetKeptItems(player *Player, amountToKeep int, cosmetics bool) (ranked, always, cosmetic []Item) {
	if player == nil {
		return nil, nil, nil
	}
	src, _ := player.ItemDefinitions().(deathValueSource)
	var items []Item
	var alwaysKept []Item
	containers := []*itemContainer{}
	if inv := player.Inventory(); inv != nil && inv.itemContainer != nil {
		containers = append(containers, inv.itemContainer)
	}
	if eq := player.Equipment(); eq != nil && eq.itemContainer != nil {
		containers = append(containers, eq.itemContainer)
	}
	for _, c := range containers {
		for _, item := range c.Items() {
			if item.ID() <= 0 || item.Amount() <= 0 || item.IsLostOnDeath() {
				continue
			}
			keep := shouldKeepItemOnDeath(player, item)
			if keep != nil && !*keep {
				continue
			}
			if keep != nil && *keep {
				alwaysKept = append(alwaysKept, item)
				continue
			}
			items = append(items, item)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return KeepValue(items[i], src) > KeepValue(items[j], src)
	})
	if amountToKeep < 0 {
		amountToKeep = 0
	}
	if amountToKeep > len(items) {
		amountToKeep = len(items)
	}
	ranked = items[:amountToKeep]
	rest := items[amountToKeep:]
	if cosmetics {
		for _, item := range rest {
			if isCosmetic(player, item) {
				cosmetic = append(cosmetic, item)
			}
		}
	}
	return ranked, alwaysKept, cosmetic
}

// GetItemsToKeep is PlayerDeathTask.getItemsToKeep: ranked +
// always + cosmetic, in that order.
func GetItemsToKeep(player *Player, amountToKeep int, cosmetics bool) []Item {
	ranked, always, cosmetic := GetKeptItems(player, amountToKeep, cosmetics)
	kept := make([]Item, 0, len(ranked)+len(always)+len(cosmetic))
	kept = append(kept, ranked...)
	kept = append(kept, always...)
	kept = append(kept, cosmetic...)
	return kept
}

// shouldKeepItemOnDeath consults the onShouldKeepItemOnDeath
// first-answer gate through the seam (nil = no opinion).
func shouldKeepItemOnDeath(player *Player, item Item) *bool {
	if Hooks.ShouldKeepItemOnDeath == nil {
		return nil
	}
	return Hooks.ShouldKeepItemOnDeath(player, item)
}

// isCosmetic is PlayerDeathTask.isCosmetic: the
// player-death:cosmetic query, answered by content
// (OrnamentKits). The payload is a pointer so handler writes
// are visible here (the EquipmentWeightsRequest precedent).
func isCosmetic(player *Player, item Item) bool {
	if player == nil || player.EmitCustomEvent == nil {
		return false
	}
	query := &DeathCosmeticQuery{Player: player, Item: item, Cosmetic: false}
	player.EmitCustomEvent(DeathCosmeticEventName, query)
	return query.Cosmetic
}

// DeathSlotLayout is PlayerDeathTask.slotLayout: the item id in
// each inventory and equipment slot, -1 where empty.
func DeathSlotLayout(player *Player) PlayerDeathLayout {
	layout := PlayerDeathLayout{}
	if player == nil {
		return layout
	}
	ids := func(items []Item) []int {
		out := make([]int, len(items))
		for i, item := range items {
			if item.ID() > 0 && item.Amount() > 0 {
				out[i] = item.ID()
			} else {
				out[i] = -1
			}
		}
		return out
	}
	if inv := player.Inventory(); inv != nil {
		layout.Inventory = ids(inv.Items())
	}
	if eq := player.Equipment(); eq != nil {
		layout.Equipment = ids(eq.Items())
	}
	return layout
}

// DeathLootOwner is PlayerDeathTask.lootOwner: the killer,
// unless the player-death:loot-owner query says otherwise.
// With no killer (or the killer is the victim) the owner is
// the victim. The query payload is a pointer so handler
// writes are visible here. An Ironman killer takes no other
// player's loot in TS — Go has no Ironman mode, so the query
// is the only override.
func DeathLootOwner(player, killer *Player) *Player {
	if killer == nil || killer == player {
		return player
	}
	if player == nil || player.EmitCustomEvent == nil {
		return killer
	}
	query := &DeathLootOwnerQuery{Player: player, Killer: killer, Owner: killer}
	player.EmitCustomEvent(DeathLootOwnerEventName, query)
	if query.Owner == nil {
		return player
	}
	return query.Owner
}

// includesItem reports whether kept holds an item equal to
// item (PlayerDeath.ts includes: TS Array.includes is
// reference equality over the same Item objects; Go items are
// values, so equality is id + amount + meta).
func includesItem(kept []Item, item Item) bool {
	for _, keptItem := range kept {
		if keptItem.Equals(item) {
			return true
		}
	}
	return false
}

// deathComponentsFor reads one item's death components from
// the player's definition source (ItemDefinition.
// getDeathComponents, ItemDefinition.ts:318-320): the
// deathComponents field of item-gameplay.json. A source
// without the capability — or a nil player — yields none,
// exactly as the TS DEFAULT definition does (deathComponents
// defaults to []). The returned slice is owned by the caller.
func deathComponentsFor(player *Player, id int) []int {
	if player == nil {
		return nil
	}
	src, ok := player.ItemDefinitions().(ItemDeathComponentsSource)
	if !ok || src == nil {
		return nil
	}
	return src.DeathComponents(id)
}

// splitIntoComponents is PlayerDeathTask.splitIntoComponents
// (PlayerDeath.ts:353-360): killed by another player in the
// Wilderness, an unkept item with tradeable components (an
// upgraded Nightmare staff: the staff and its orb,
// `deathComponents` in item-gameplay.json) drops as those
// components for the killer, and the item itself is gone.
// Each component carries the whole item's amount (TS builds
// `new Item(id, item.getAmount())` per component). Anything
// else drops whole: no killer, a self-kill, outside the
// Wilderness, a kept item, a lost-on-death item, or an item
// with no components. The keepsEverything guard TS carries
// here is omitted: Go has no rights field, so keepsEverything
// is always false (see DropDeathItems). The split runs before
// the per-item drop loop, so the components — not the whole
// item — reach the death-drops hooks and the floor.
func splitIntoComponents(player, killer *Player, kept, items []Item) []Item {
	if killer == nil || killer == player || player == nil || !combatInWilderness(player.Location()) {
		return items
	}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		components := deathComponentsFor(player, item.ID())
		if len(components) == 0 || includesItem(kept, item) || item.IsLostOnDeath() {
			out = append(out, item)
			continue
		}
		for _, componentID := range components {
			out = append(out, NewItem(componentID, item.Amount()))
		}
	}
	return out
}

// DropDeathItems is PlayerDeathTask.dropItems: a death's item
// loss at `position`. The kept items are set aside and
// returned, the rest go to the death-drops hooks (a grave) or
// the floor, and the inventory and equipment are emptied;
// returnKeptItems gives the kept ones back. The drops event
// fires even with nothing lost: a death still changes an
// existing grave. keepsEverything is always false in Go (no
// rights field), so the keeps-everything branches are dead
// and documented, not branched on.
func DropDeathItems(player, killer *Player, position Location) []Item {
	if player == nil {
		return nil
	}
	kept := GetItemsToKeep(player, AmountToKeep(player), ProtectsCosmetics(player, killer))
	src := player.ItemDefinitions()
	var playerItems []Item
	if inv := player.Inventory(); inv != nil {
		playerItems = append(playerItems, inv.GetValidItems()...)
	}
	if eq := player.Equipment(); eq != nil {
		playerItems = append(playerItems, eq.GetValidItems()...)
	}
	// A Wilderness PvP death splits component items before
	// the per-item loop (PlayerDeath.ts dropItems calls
	// splitIntoComponents here).
	playerItems = splitIntoComponents(player, killer, kept, playerItems)
	layout := DeathSlotLayout(player)
	var droppedItems []Item
	var lostUntradeables []Item
	dropped := false
	pluginHandledDrop := false

	for _, item := range playerItems {
		// Barrows items drop to the floor in TS
		// (!isLostOnDeath && isBarrowsItem); Go has no
		// Barrows seam, so isBarrowsItem is false here and
		// they take the ordinary untradeable path.
		dropEligible := item.IsTradeable(src) && !includesItem(kept, item)
		event := &DeathItemDropEvent{
			Player:          player,
			Killer:          killer,
			Item:            item,
			Kept:            includesItem(kept, item),
			Location:        position,
			ShouldDropItems: true,
			DropEligible:    dropEligible,
			Handled:         false,
		}
		handled := false
		if Hooks.OnPlayerDeathItemDrop != nil {
			handled = Hooks.OnPlayerDeathItemDrop(event)
		}
		// Items a hook adds to the death are lost with it:
		// to the grave or the floor, as the rest are.
		droppedItems = append(droppedItems, event.ExtraDrops...)
		if handled {
			pluginHandledDrop = true
			continue
		}
		if event.SuppressDefaultDrop && event.DropEligible {
			dropped = true
			continue
		}
		if item.IsLostOnDeath() || includesItem(kept, item) {
			continue
		}
		// Untradeables are kept unless a death-drops hook
		// takes them (a grave).
		if !item.IsTradeable(src) {
			lostUntradeables = append(lostUntradeables, item)
			continue
		}
		droppedItems = append(droppedItems, item)
	}

	// Emitted with nothing lost too.
	before := len(droppedItems) + len(lostUntradeables)
	if Hooks.OnPlayerDeathDrops != nil {
		event := &DeathDropsEvent{
			Player:       player,
			Killer:       killer,
			Location:     position,
			Items:        droppedItems,
			Untradeables: lostUntradeables,
			Layout:       layout,
		}
		Hooks.OnPlayerDeathDrops(event)
		droppedItems = event.Items
		lostUntradeables = event.Untradeables
		if len(droppedItems)+len(lostUntradeables) < before {
			pluginHandledDrop = true
		}
	}
	// Asked after the item hooks: one can make an item
	// tradeable, so it reaches here past the hooks' own
	// eligibility check.
	owner := DeathLootOwner(player, killer)
	for _, item := range droppedItems {
		if Hooks.DropGroundItem != nil {
			Hooks.DropGroundItem(item, position, owner)
		}
		dropped = true
	}
	kept = append(kept, lostUntradeables...)

	if killer != nil && !dropped && !pluginHandledDrop {
		killer.SendMessage(player.GetDisplayName() + " had no valuable items to be dropped.")
	}

	if inv := player.Inventory(); inv != nil {
		inv.ResetItems()
		inv.RefreshItems()
	}
	if eq := player.Equipment(); eq != nil {
		eq.ResetItems()
		eq.RefreshItems()
	}
	return kept
}

// ReturnKeptItems is PlayerDeathTask.returnKeptItems: the items
// a death let the player keep, back in the inventory as clones.
// Overflow never eats an item silently: an item that does not
// fit the emptied inventory is grounded for the player through
// the ground-item seam instead (Go's container Add would drop
// the remainder with only a chat message). With no seam the
// item is still added through Add — the Add path itself warns
// the player — but the grounded path is the production one.
func ReturnKeptItems(player *Player, items []Item) {
	if player == nil {
		return
	}
	inv := player.Inventory()
	if inv == nil {
		return
	}
	for _, item := range items {
		clone := item.Clone()
		if !deathItemFits(inv, clone, player.ItemDefinitions()) {
			if Hooks.DropGroundItem != nil {
				Hooks.DropGroundItem(clone, player.Location(), player)
				continue
			}
		}
		inv.AddItem(clone)
	}
}

// deathItemFits reports whether item fits in the inventory as
// Add would place it: a stackable item fits when a stack of the
// same id is already present, otherwise it needs a free slot
// per non-stackable unit (one slot for a stackable item with no
// stack yet). The meta check Add applies to stack matching is
// approximated by id: two stacks of one id with different meta
// would take two slots, so this may over-approve by one slot in
// that rare case — Add then warns the player instead of
// grounding, and nothing is eaten silently.
func deathItemFits(inv *Inventory, item Item, src ItemDefinitionSource) bool {
	if inv == nil {
		return false
	}
	stackable := src != nil && src.IsStackable(item.ID())
	slots := inv.Items()
	free := 0
	for _, slot := range slots {
		if slot.ID() <= 0 || slot.Amount() <= 0 {
			free++
			continue
		}
		if stackable && slot.ID() == item.ID() {
			return true
		}
	}
	if stackable {
		return free >= 1
	}
	return free >= item.Amount()
}

// LoseItemsAsDeath is PlayerDeathTask.loseItemsAsDeath: loses
// items as a death at `position` would, without the death
// itself (a raid that holds its dead until the whole party
// falls takes their items then). The death save request runs
// through the same seam as the death task's.
func LoseItemsAsDeath(player *Player, position Location) {
	if player == nil {
		return
	}
	ReturnKeptItems(player, DropDeathItems(player, nil, position))
	requestDeathSave(player, "death")
}

// requestDeathSave runs the "death"/"respawn" save requests
// (GameConstants.PLAYER_PERSISTENCE.requestSave) through the
// CombatHooks.RequestSave seam, wired by the coordinator to the
// save coordinator (the TradeHookSeam.SavePlayer precedent).
// With no seam installed the save is a documented gap, not an
// invented call.
func requestDeathSave(player *Player, reason string) {
	if Hooks.RequestSave != nil {
		Hooks.RequestSave(player, reason)
	}
}

// resetDeathAttributes is Player.resetAttributes (Player.ts
// :256-296), the Go-applicable subset in TS order. Each skipped
// line carries its one-line reason. It runs on the respawn tick
// (PlayerDeath.ts case 0) and on the error-path fallback.
func resetDeathAttributes(player *Player) {
	if player == nil {
		return
	}
	combat := player.GetCombat()
	// Animation 65535 (reset pose): skipped — no
	// performAnimation (Phase 29).
	// Special deactivated + bar update: the combat flag is
	// cleared and the orb varp re-sent through
	// updateSpecialAttackOrb, the Go form of
	// CombatSpecial.updateBar's state send.
	if combat != nil {
		combat.SetSpecialActivated(false)
		updateSpecialAttackOrb(&player.Mobile)
		// Vengeance: cleared (setHasVengeance(false)).
		combat.SetVengeance(false)
	}
	// Antifire is left running, timer and protection both
	// (Player.ts:261-265): deliberately untouched.
	// Poison immunity timer stop + poison clock resetPhase
	// (Player.ts:264-265): for a player the immunity timer
	// IS the clock (Combat.ts:805-806), so stop() clears the
	// counter and brief timers; resetPhase() restores the
	// 30-tick phase. Poison 0 + venomed false
	// (Player.ts:274-275) via CurePoison, which also stops
	// the effect and re-sends the orb.
	if combat != nil {
		clock := combat.PoisonClock()
		clock.Stop()
		clock.ResetPhase()
	}
	CurePoison(&player.Mobile)
	// Teleblock timer stop: SKIPPED — no teleblock timer in
	// Go (no TimerKey, no field).
	// Freeze + freeze-immunity timers cancelled.
	timers := player.GetTimers()
	if timers != nil {
		timers.Cancel(TimerKeyFreeze)
		timers.Cancel(TimerKeyFreezeImmunity)
	}
	// Cast spell / previous cast cleared.
	if combat != nil {
		combat.SetCastSpell(nil)
		combat.SetPreviousCast(nil)
	}
	// Prayer-block timer stop: SKIPPED — no prayer-block
	// timer in Go.
	// Wilderness level 0: SKIPPED — Go computes the
	// wilderness level live from the location; no stored
	// level exists to clear.
	// Recoil attribute 0.
	player.SetAttribute(RecoilDamageAttribute, 0)
	// Skull timer 0 + type WHITE (Worker B: ClearSkull).
	player.ClearSkull()
	// Weapon-interface re-assign.
	AssignWeaponInterface(player)
	// BonusManager.update: no-op in Go — bonuses are
	// computed on demand from equipment (ComputeBonuses),
	// so there is no cached bonus state to refresh.
	// Prayers deactivated.
	DeactivatePrayers(&player.Mobile)
	// Equipment + inventory refreshed.
	if eq := player.Equipment(); eq != nil {
		eq.RefreshItems()
	}
	if inv := player.Inventory(); inv != nil {
		inv.RefreshItems()
	}
	// Every skill's current level back to its resting level
	// (the base, raised by a plugin's permanent boost — a
	// death keeps such a boost). This also restores
	// hitpoints: Hitpoints is a skill, and its resting level
	// is its max.
	if skills := player.Skills(); skills != nil {
		for skill := 0; skill < SkillCount; skill++ {
			skills.SetCurrentLevels(skill, skills.RestingLevel(skill))
		}
	}
	// Run energy 100 + send.
	player.SetRunEnergy(100)
	player.SendRunEnergy(player.GetRunEnergyUnits(), player.IsRunning(), player.GetWeightKg(), 0)
	// Movement queue unblocked + reset (Player.ts:291).
	if queue := player.MovementQueue(); queue != nil {
		queue.SetBlockMovement(false)
		queue.Reset(true)
	}
	// Effect timers (antifire/freeze/vengeance/teleblock)
	// zeroed: SKIPPED — no effect-timer sender in Go.
	// Special attack state false: sent above with the orb
	// (updateSpecialAttackOrb sends varp 301, 0).
	// Untargetable false + dying false (Player.ts:295-296).
	if combat != nil {
		combat.SetUntargetable(false)
	}
	combatantFor(&player.Mobile).SetDying(false)
}
