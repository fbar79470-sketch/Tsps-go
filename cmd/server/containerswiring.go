package main

import (
	"sort"

	"tsps-go/internal/cache"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Container wiring: the cmd/server adapter between the game
// layer's containers and the session layer's packet sends. It has
// two halves:
//
//   - The ItemDefinitionSource implementation over the loaded
//     definition Store, reusing the same Store.Item lookups
//     behind the ground-item Stackable/TradeableDropable seams
//     (grounditemswiring.go) — extended, never duplicated.
//     buildServer builds it once; entry loading
//     (prepareCarriedSave) installs it on each entering player
//     before loading the containers, because bank loading stacks
//     through the source.
//   - The refresh hooks on the player's containers, installed by
//     installContainerRefresh, which encode snapshots through the
//     existing protocol encoders and send them through the
//     existing SessionPort.SendPacket. No new SessionPort method
//     is added (phase 9 seam contract).
//
// The game layer never imports protocol or definition; this
// package imports both, as the flush and ground-item adapters do.

// storeItemDefinitions implements game.ItemDefinitionSource over
// a *definition.Store. Store.Item never returns nil: unknown ids,
// and every id with no cache, answer the DEFAULT shape
// (definition/item.go), so every method is total over all ids,
// including negative and out-of-range ones. A nil Store is never
// wrapped — newItemDefinitionSource returns a nil source instead,
// and the game layer's documented nil-source defaults apply.
//
// It also implements game.ItemAnimationSource (over the Store)
// and game.ItemActionSource (over the cache CacheDefinitions:
// the action lists live on the cache ObjType, not on the
// JSON-layered ItemDefinition). A nil CacheDefinitions degrades
// the action methods to the ObjType defaults, exactly as a cache
// item with no action opcodes decodes.
type storeItemDefinitions struct {
	store *definition.Store
	defs  *cache.CacheDefinitions
}

// newItemDefinitionSource returns the ItemDefinitionSource for
// store, or nil when store is nil (a load that produced none:
// the game layer degrades to its documented no-definition
// defaults — nothing stacks, nothing weighs anything, nothing is
// wearable/tradeable/dropable, requirements empty). defs is the
// cache facade the action-list methods read (nil in NoCache mode:
// action lists read the ObjType defaults).
func newItemDefinitionSource(store *definition.Store, defs *cache.CacheDefinitions) game.ItemDefinitionSource {
	if store == nil {
		return nil
	}
	return storeItemDefinitions{store: store, defs: defs}
}

func (s storeItemDefinitions) IsStackable(id int) bool { return s.store.Item(id).IsStackable() }
func (s storeItemDefinitions) IsNoted(id int) bool     { return s.store.Item(id).IsNoted() }
func (s storeItemDefinitions) NoteID(id int) int       { return s.store.Item(id).NoteID() }
func (s storeItemDefinitions) WeightGrams(id int) int  { return s.store.Item(id).Weight() }
func (s storeItemDefinitions) EquipmentSlot(id int) int {
	return s.store.Item(id).EquipmentSlot()
}
func (s storeItemDefinitions) IsDoubleHanded(id int) bool {
	return s.store.Item(id).IsDoubleHanded()
}
func (s storeItemDefinitions) Requirements(id int) []int { return s.store.Item(id).Requirements() }
func (s storeItemDefinitions) CombatRequirement(id int) int {
	return s.store.Item(id).CombatRequirement()
}
func (s storeItemDefinitions) IsTradeable(id int) bool { return s.store.Item(id).IsTradeable() }
func (s storeItemDefinitions) IsSellable(id int) bool  { return s.store.Item(id).IsSellable() }
func (s storeItemDefinitions) IsDropable(id int) bool  { return s.store.Item(id).IsDropable() }
func (s storeItemDefinitions) UnNote(id int) int       { return s.store.Item(id).UnNote() }
func (s storeItemDefinitions) PlaceholderID(id int) int {
	return s.store.Item(id).PlaceholderID()
}
func (s storeItemDefinitions) StandAnim(id int) int { return s.store.Item(id).StandAnim() }
func (s storeItemDefinitions) WalkAnim(id int) int  { return s.store.Item(id).WalkAnim() }
func (s storeItemDefinitions) RunAnim(id int) int   { return s.store.Item(id).RunAnim() }

// The shop/pricing/examine value and name reads
// (game.ShopItemDefinitionSource, over the Store): the fields
// ShopManager pricing, the examine price line, and the trade
// value line read from ItemDefinition. Store.Item never returns
// nil, so every method is total over all ids, matching the TS
// DEFAULT shape for unknown ids.

// ItemName returns the item name (getName, ItemDefinition.ts:177-179).
func (s storeItemDefinitions) ItemName(id int) string { return s.store.Item(id).Name() }

// ItemExamine returns the examine text (getExamine,
// ItemDefinition.ts:181-183).
func (s storeItemDefinitions) ItemExamine(id int) string { return s.store.Item(id).Examine() }

// ItemValue returns the store value (getValue,
// ItemDefinition.ts:185-187).
func (s storeItemDefinitions) ItemValue(id int) int { return s.store.Item(id).Value() }

// ItemBloodMoneyValue returns the blood money value
// (getBloodMoneyValue, ItemDefinition.ts:204-206).
func (s storeItemDefinitions) ItemBloodMoneyValue(id int) int {
	return s.store.Item(id).BloodMoneyValue()
}

// ItemHasGrandExchangeValue reports whether a Grand Exchange
// quote exists (hasGrandExchangeValue, ItemDefinition.ts:195-197).
func (s storeItemDefinitions) ItemHasGrandExchangeValue(id int) bool {
	return s.store.Item(id).HasGrandExchangeValue()
}

// ItemGrandExchangeValue returns the Grand Exchange quote,
// falling back to the store value (getGrandExchangeValue,
// ItemDefinition.ts:190-192).
func (s storeItemDefinitions) ItemGrandExchangeValue(id int) int {
	return s.store.Item(id).GrandExchangeValue()
}

// ItemHighAlchValue returns the high alchemy value
// (getHighAlchValue, ItemDefinition.ts:208-210).
func (s storeItemDefinitions) ItemHighAlchValue(id int) int { return s.store.Item(id).HighAlchValue() }

// ItemHasHighAlchValue reports whether a high alchemy value was
// ever set for the item (getHighAlchValue() !== undefined).
func (s storeItemDefinitions) ItemHasHighAlchValue(id int) bool {
	return s.store.Item(id).HasHighAlchValue()
}

// ItemLowAlchValue returns the low alchemy value
// (getLowAlchValue, ItemDefinition.ts:212-214).
func (s storeItemDefinitions) ItemLowAlchValue(id int) int { return s.store.Item(id).LowAlchValue() }

// ItemHasLowAlchValue reports whether a low alchemy value was
// ever set for the item.
func (s storeItemDefinitions) ItemHasLowAlchValue(id int) bool {
	return s.store.Item(id).HasLowAlchValue()
}

// GrandExchangeValue returns the Grand Exchange quote, falling
// back to the store value (game.ItemGEValueSource, read by the
// trade value line). It is the same read as
// ItemGrandExchangeValue under the trade source's method name.
func (s storeItemDefinitions) GrandExchangeValue(id int) int {
	return s.store.Item(id).GrandExchangeValue()
}

// ItemBonuses returns the 14-entry equipment bonus array for id
// (game.ItemBonusSource, over the Store): 0..4 attack, 5..9
// defence, 10 melee strength, 11 ranged strength, 12 magic
// damage percent, 13 prayer. ItemDefinition.Bonuses derives
// them from cache params, defaulting to zeros; without this
// adapter every fighter reads as naked (see combatant.go).
func (s storeItemDefinitions) ItemBonuses(id int) []float64 {
	return s.store.Item(id).Bonuses()
}

// WeaponAttackSpeed returns the weapon attack speed in ticks
// for id (game.WeaponInfoSource, over the Store).
func (s storeItemDefinitions) WeaponAttackSpeed(id int) int {
	return s.store.Item(id).AttackSpeed()
}

// DeathComponents returns the tradeable items id splits into
// when lost to another player in the Wilderness
// (game.ItemDeathComponentsSource, over the Store):
// ItemDefinition.getDeathComponents (ItemDefinition.ts:318-320),
// over the deathComponents field from item-gameplay.json. The
// returned slice is a fresh copy the caller owns.
func (s storeItemDefinitions) DeathComponents(id int) []int {
	return s.store.Item(id).DeathComponents()
}

// WeaponAttackRange returns the weapon attack range in tiles
// for id (game.WeaponInfoSource, over the Store).
func (s storeItemDefinitions) WeaponAttackRange(id int) int {
	return s.store.Item(id).AttackRange()
}

// WeaponAttackAnim returns this weapon's own animation for an
// attack of attackType (game.WeaponInfoSource, over the
// Store): ItemDefinition.getAttackAnim
// (ItemDefinition.ts:308-311), or -1 for its type's.
func (s storeItemDefinitions) WeaponAttackAnim(id int, attackType string) int {
	return s.store.Item(id).AttackAnim(attackType)
}

// WeaponBlockAnim returns the block animation for id
// (game.WeaponInfoSource, over the Store):
// ItemDefinition.getBlockAnim (ItemDefinition.ts:253-255),
// default 424.
func (s storeItemDefinitions) WeaponBlockAnim(id int) int {
	return s.store.Item(id).BlockAnim()
}

// WeaponInterfaceName returns the weapon interface name for id
// (game.WeaponInfoSource, over the Store).
func (s storeItemDefinitions) WeaponInterfaceName(id int) string {
	return s.store.Item(id).WeaponInterfaceName()
}

// InventoryActions returns the five inventory action texts for id
// from the cache ObjType (game.ItemActionSource). With no cache
// facade, or a nil/empty action list, the ObjType defaults apply
// (CacheDefinitions.ts:676). The returned slice is fresh on every
// call and always length 5: shorter lists pad with "", longer
// ones truncate, matching the listeners' normalization.
func (s storeItemDefinitions) InventoryActions(id int) []string {
	if s.defs == nil {
		return game.DefaultInventoryActions()
	}
	obj := s.defs.GetItem(id)
	if obj == nil {
		return game.DefaultInventoryActions()
	}
	return copyActions(obj.InventoryActions, game.DefaultInventoryActions())
}

// GroundActions returns the five ground action texts for id from
// the cache ObjType (game.ItemActionSource). With no cache
// facade, or a nil/empty action list, the ObjType defaults apply
// (ObjType.ts:118). The returned slice is fresh on every call
// and always length 5.
func (s storeItemDefinitions) GroundActions(id int) []string {
	if s.defs == nil {
		return game.DefaultGroundActions()
	}
	obj := s.defs.GetItem(id)
	if obj == nil {
		return game.DefaultGroundActions()
	}
	return copyActions(obj.GroundActions, game.DefaultGroundActions())
}

// ShiftClickIndex returns the shift-click action index for id
// from the cache ObjType (game.ItemActionSource,
// ObjType.GetShiftClickIndex). With no cache facade, or a nil
// definition, the default rule applies over the default actions:
// the fifth option reads "Drop", so the index is 4.
func (s storeItemDefinitions) ShiftClickIndex(id int) int {
	if s.defs == nil {
		return 4
	}
	obj := s.defs.GetItem(id)
	if obj == nil {
		return 4
	}
	return obj.GetShiftClickIndex()
}

// copyActions copies actions into a fresh length-5 slice, padding
// with "" or truncating. A nil or empty list yields the defaults
// (already a fresh slice from the caller).
func copyActions(actions []string, defaults []string) []string {
	if len(actions) == 0 {
		return defaults
	}
	out := make([]string, 5)
	copy(out, actions)
	return out
}

var _ game.ItemDefinitionSource = storeItemDefinitions{}
var _ game.ItemBonusSource = storeItemDefinitions{}
var _ game.WeaponInfoSource = storeItemDefinitions{}
var _ game.ShopItemDefinitionSource = storeItemDefinitions{}
var _ game.ItemGEValueSource = storeItemDefinitions{}
var _ game.ItemNameSource = storeItemDefinitions{}

var _ game.ItemAnimationSource = storeItemDefinitions{}

var _ game.ItemActionSource = storeItemDefinitions{}

// installContainerRefresh installs the container refresh hooks on
// p's live containers: the inventory, the equipment, and all 11
// bank tabs (created eagerly here, so every tab refreshes even
// before the player opens the bank). nativeClient reports whether
// the entering client is a native client (ep.Capabilities &
// protocol.HandshakeCapabilityNativeClient != 0): only native
// clients receive the worn-inventory run-script packet on
// equipment refresh (PacketSender.sendItemContainer, Equipment
// branch, PacketSender.ts:1056-1069); browser clients build worn
// equipment from the appearance block.
//
// The item-definition source is NOT installed here: entry loading
// (prepareCarriedSave) installs it before loading the containers,
// because bank loading stacks through the source. This function
// reads the installed source back for the placeholder ids the bank
// snapshot needs; with no source installed, no placeholder
// substitution happens.
//
// CALL SITE (coordinator): AddPlayer, after prepareCarriedSave
// (so save-loaded banks, installed via SetBank, receive hooks)
// and before the player is queued (so no tick can refresh a
// container before its hook exists). Calling it again re-installs
// the same hook everywhere, which is harmless; bank tabs created
// lazily after this call inherit the hook from a sibling container
// (see newItemContainer).
//
// A nil player installs nothing.
func installContainerRefresh(p *game.Player, nativeClient bool) {
	if p == nil {
		return
	}
	hook := containerRefreshHook(nativeClient, p.ItemDefinitions())
	if inv := p.Inventory(); inv != nil {
		inv.SetRefreshHook(hook)
	}
	if eq := p.Equipment(); eq != nil {
		eq.SetRefreshHook(hook)
	}
	for tab := 0; tab < p.BankCount(); tab++ {
		if bank := p.Bank(tab); bank != nil {
			bank.SetRefreshHook(hook)
		}
	}
}

// containerRefreshHook returns the refresh hook the adapter
// installs: one closure dispatching on the event kind, sending
// through the player's SessionPort. A nil session (tests, bots)
// drops every packet. src supplies the placeholder ids the bank
// snapshot needs; a nil src means no placeholder substitution.
func containerRefreshHook(nativeClient bool, src game.ItemDefinitionSource) game.RefreshHook {
	return func(ev game.RefreshEvent) {
		p := ev.Player
		if p == nil || p.Session == nil {
			return
		}
		switch ev.Kind {
		case game.ContainerInventory:
			// sendItemContainer, Inventory branch
			// (PacketSender.ts:1042-1050): the full 28 slots
			// as an inventory snapshot.
			p.Session.SendPacket(protocol.EncodeInventorySnapshot(itemSlots(ev.Items)))
		case game.ContainerEquipment:
			// sendItemContainer, Equipment branch
			// (PacketSender.ts:1052-1070): flag APPEARANCE
			// (already done in the game layer by
			// onEquipmentChanged — the adapter must not
			// repeat it) and, for native clients only, the
			// worn inventory (94) as a run-script packet
			// with quantities, carrying only the occupied
			// slots (the TS filter: itemId >= 0). Browser
			// clients build worn equipment from the
			// appearance block and receive nothing here.
			if !nativeClient {
				return
			}
			var slots []protocol.ItemSlot
			for slot, item := range ev.Items {
				if item.ID() >= 0 {
					slots = append(slots, protocol.ItemSlot{Slot: slot, ItemID: item.ID(), Quantity: item.Amount()})
				}
			}
			p.Session.SendPacket(protocol.EncodeWidgetRunScript(-1, nil, nil, nil, map[int]protocol.ScriptInventorySnapshot{
				game.WornInventoryID: {Capacity: game.EquipmentCapacity, Slots: slots},
			}))
		case game.ContainerWornInventory:
			// sendWornInventory (Equipment.ts:87-91) via
			// sendInventory (PacketSender.ts:1725-1739):
			// all 14 slots, absent slots as id -1 /
			// quantity 0 (no filter — unlike the
			// equipment-refresh branch above). Sent
			// unconditionally, as TS sends it.
			p.Session.SendPacket(protocol.EncodeWidgetRunScript(-1, nil, nil, nil, map[int]protocol.ScriptInventorySnapshot{
				game.WornInventoryID: {Capacity: game.EquipmentCapacity, Slots: itemSlots(ev.Items)},
			}))
		case game.ContainerBank:
			sendBankRefresh(p, src)
		}
	}
}

// itemSlots converts event items to protocol slots, indexed by
// position (sendItemContainer builds one slot per capacity slot,
// PacketSender.ts:1043-1048).
func itemSlots(items []game.Item) []protocol.ItemSlot {
	slots := make([]protocol.ItemSlot, len(items))
	for i, item := range items {
		slots[i] = protocol.ItemSlot{Slot: i, ItemID: item.ID(), Quantity: item.Amount()}
	}
	return slots
}

// sendBankRefresh sends a bank refresh (Bank.refreshItems,
// Bank.ts:1417-1433): the tab-count varbits (4170+tab, tabs 1-9),
// the full-bank snapshot, and the inventory snapshot — in that
// order. The tab-reconfiguration early return and the
// bank:refreshed plugin event are gaps (no tab UI state, no
// plugin system); the varbit sends the adapter CAN do, it does.
func sendBankRefresh(p *game.Player, src game.ItemDefinitionSource) {
	for tab := 1; tab <= 9; tab++ {
		bank := p.Bank(tab)
		if bank == nil {
			continue
		}
		p.Session.SendPacket(protocol.EncodeVarbit(4170+tab, len(bank.GetValidItems())))
	}
	p.Session.SendPacket(protocol.EncodeBankSnapshot(game.MaxBankSlots, bankLayoutSlots(p, src)))
	if inv := p.Inventory(); inv != nil {
		p.Session.SendPacket(protocol.EncodeInventorySnapshot(itemSlots(inv.Items())))
	}
}

// bankLayoutSlots builds the bank snapshot slots (sendBankSnapshot,
// PacketSender.ts:858-867, over Bank.layout, Bank.ts:523-533):
// tabs in order 1..9 then 0 (the search tab is not part of the
// layout), every slot with id >= 0, numbered sequentially across
// the whole layout, carrying the display id (a placeholder shows
// its faded cache placeholder), the amount, and the tab. The
// placeholder flag bit is NOT set: TS sendBankSnapshot maps no
// placeholder field (BankSlotView.placeholder stays undefined),
// so the encoded flag byte carries only the tab.
func bankLayoutSlots(p *game.Player, src game.ItemDefinitionSource) []protocol.BankSlot {
	var slots []protocol.BankSlot
	slot := 0
	for _, tab := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0} {
		bank := p.Bank(tab)
		if bank == nil {
			continue
		}
		for _, item := range bank.Items() {
			if item.ID() < 0 || item.Amount() < 0 {
				continue
			}
			slots = append(slots, protocol.BankSlot{
				Slot:     slot,
				ItemID:   bankDisplayItemID(item, src),
				Quantity: item.Amount(),
				Tab:      tab,
			})
			slot++
		}
	}
	return slots
}

// bankDisplayItemID is Bank.displayItemId (Bank.ts:536-543): a
// live stack shows its own id; a placeholder (amount 0) shows its
// faded cache placeholder id when it has one, else its own id.
// The TS fallback through the plugin placeholder family
// (placeholderBankId) reduces to the same cache term without
// plugins: placeholderBankId returns the id itself exactly when
// the cache placeholder exists, and the family's placeholder is
// then the same id. A nil source means no placeholder
// substitution.
func bankDisplayItemID(item game.Item, src game.ItemDefinitionSource) int {
	if item.Amount() > 0 || src == nil {
		return item.ID()
	}
	if placeholder := src.PlaceholderID(item.ID()); placeholder >= 0 {
		return placeholder
	}
	return item.ID()
}

// ---- bank open-sequence sender ---------------------------------

// bankSender implements game.BankSender by reaching the player's
// session through the SessionPort interface (the shopSender
// precedent): a session-less player sends nothing. Stateless
// except for the world name carried for the title string; one
// shared instance serves the bank.
type bankSender struct {
	worldName string
}

// SendBankOpen delivers one bank-open view as the frame sequence
// of Bank.open (Bank.ts:1366-1414): script 917 [-1,-2]; the
// mount varps/varbits as separate varp/varbit packets, sent in
// ascending id order so the sequence is deterministic (the
// session MountSubInterface has no options form, so the {varps,
// varbits} the mounts carry in TS are sent before them instead);
// the two sub-interface mounts (12 type 0 at (161<<16)|16, 15
// type 3 at (161<<16)|74); the five flags ranges — the item grid
// V2 range (0-1409, ITEM_SLOT_FLAGS/OPS), the tab-drop-target
// range (1428-1437, 1<<20), the tab bar (10-19,
// TAB_BUTTON_FLAGS), the side inventory (0-27, 3278846), the
// side worn (0-27, SIDE_WORN_FLAGS); varbit 12393=1; the "Bank
// of <world>" string at (12<<16)|3; the view-container reset
// when the view asks for it; then sendBankRefresh (the refresh
// half of Bank.open, Bank.ts:1412). The CONTAINER_OPEN sound
// and the bank:opened event are the game layer's job (bankOpen),
// not the sender's.
func (s bankSender) SendBankOpen(p *game.Player, view game.BankOpenView) {
	if p == nil || p.Session == nil {
		return
	}
	send := p.Session.SendPacket
	send(protocol.EncodeWidgetRunScript(917, []protocol.ScriptArg{
		protocol.ScriptArgInt(-1),
		protocol.ScriptArgInt(-2),
	}, nil, nil, nil))
	for _, id := range sortedKeys(view.Varps) {
		send(protocol.EncodeVarp(id, view.Varps[id]))
	}
	for _, id := range sortedKeys(view.Varbits) {
		send(protocol.EncodeVarbit(id, view.Varbits[id]))
	}
	send(protocol.EncodeWidgetOpenSub((161<<16)|16, game.BankMainInterfaceID, 0, protocol.WidgetOpenSubOptions{}))
	send(protocol.EncodeWidgetOpenSub((161<<16)|74, game.BankSideInterfaceID, 3, protocol.WidgetOpenSubOptions{}))
	send(protocol.EncodeWidgetSetFlagsRangeV2((game.BankMainInterfaceID<<16)|game.BankItemsChild, 0, 1409, game.BankItemSlotFlags, game.BankItemSlotOps))
	send(protocol.EncodeWidgetSetFlagsRange((game.BankMainInterfaceID<<16)|game.BankItemsChild, game.BankTabDropSlotOffset, game.BankTabDropSlotOffset+9, 1<<20))
	send(protocol.EncodeWidgetSetFlagsRange((game.BankMainInterfaceID<<16)|game.BankTabsChild, game.BankTabButtonSlotOffset, game.BankTabButtonSlotOffset+9, game.BankTabButtonFlags))
	send(protocol.EncodeWidgetSetFlagsRange((game.BankSideInterfaceID<<16)|3, 0, 27, 3278846))
	send(protocol.EncodeWidgetSetFlagsRange((game.BankSideInterfaceID<<16)|game.BankSideWornChild, 0, 27, game.BankSideWornFlags))
	send(protocol.EncodeVarbit(12393, 1))
	send(protocol.EncodeWidgetSetText((game.BankMainInterfaceID<<16)|3, "Bank of "+s.worldName, nil))
	if view.ResetViewContainer {
		send(protocol.EncodeVarbit(game.BankViewContainerVarbit, 0))
		send(protocol.EncodeVarp(game.BankViewContainerObjVarp, -1))
	}
	sendBankRefresh(p, p.ItemDefinitions())
}

// SendAmountPrompt sends the "Enter amount:" prompt frame
// (sendEnterAmountPrompt, PacketSender.ts:446-452:
// EncodeRunClientScript(108, [title])). The game layer arms
// the EnteredAmountAction first; this sends only the frame.
func (bankSender) SendAmountPrompt(p *game.Player, prompt string) {
	if p == nil || p.Session == nil {
		return
	}
	p.Session.SendPacket(protocol.EncodeRunClientScript(108, []protocol.ScriptArg{protocol.ScriptArgString(prompt)}))
}

// SendInputPrompt sends the free-text prompt frame
// (sendEnterInputPrompt, PacketSender.ts:446-452:
// EncodeRunClientScript(110, [title, 0])). The game layer arms
// the EnteredSyntaxAction first; this sends only the frame.
func (bankSender) SendInputPrompt(p *game.Player, prompt string) {
	if p == nil || p.Session == nil {
		return
	}
	p.Session.SendPacket(protocol.EncodeRunClientScript(110, []protocol.ScriptArg{
		protocol.ScriptArgString(prompt),
		protocol.ScriptArgInt(0),
	}))
}

// installBankSender installs the bank open-sequence sender
// (game.SetBankSender). The coordinator calls it from
// buildServer after loadGameDefinitions (the
// installShopHubSurfaces precedent, server.go:301), alongside
// installBankFlows in bankwiring.go. The world name is the
// configured world definition name (Server.WorldName).
func installBankSender(worldName string) {
	game.SetBankSender(bankSender{worldName: worldName})
}

// sortedKeys returns the map's keys in ascending order, so a
// varp/varbit bundle is sent deterministically.
func sortedKeys(m map[int]int) []int {
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}
