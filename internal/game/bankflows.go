package game

import (
	"strings"
)

// Bank core flows: the port of the player-facing half of
// game/model/container/impl/Bank.ts (1,567 lines) — open,
// refresh, withdraw, deposit, tabs, search, buttons, widget
// actions, and drags. The container mechanics (tab storage,
// growth, placeholders, sort, rearrange) already live in
// bank.go / container.go; this file builds on them and never
// rewrites them.
//
// LAYERING. This file never imports protocol or session. All
// outbound frames go through the small sender seams in
// bankstate.go (BankSender for the open sequence and prompt
// frames, DialogueSender for strings/flags/scripts/removal,
// Player.SendVarp / bankSendVarbit for varps/varbits, and
// SendSound for the four bank sounds), or through the
// container refresh hook the cmd/server adapter installs
// (sendBankRefresh: tab-count varbits + bank snapshot +
// inventory snapshot).
//
// GATES THAT DEGRADE TO "PROCEED". TS gates several flows on
// plugin events with no subscribers in Go; each degenerates
// to the TS no-plugin behaviour, documented at the site:
//   - emitCanBank (open) / emitCanBankItem (deposit): allow.
//   - bank:capacity: capacity is always MAX_SLOTS.
//   - bank:full / bank:opened / bank:refreshed /
//     bank:placeholder-identity / bank:inventory-op /
//     bank:item-op / bank:withdrawn-items / bank:incinerate:
//     emitted through EmitCustomEvent for future content;
//     with no subscribers the default behaviour runs.
//   - Bank PIN (emitCanBank "open" hook): out of scope —
//     the bank opens without a PIN by default.
//   - BankSpace capacity system: out of scope — the
//     no-plugin default 1410 stands.
//
// SAVE HINTS. TS calls PLAYER_PERSISTENCE.requestSave after
// tab collapse, placeholder release, tab reorder, and drags.
// Go has no per-flow save-request seam for the bank (the
// CombatHooks.RequestSave seam belongs to combat); banks
// persist through the normal save path, so the hints are
// skipped and noted as a gap at each site.

// Bank message texts (Bank.ts:17-20, 1438-1558, 376-425).
const (
	// bankInventoryFullMessage is INVENTORY_FULL_MESSAGE
	// (Bank.ts:18): sent as a spam-filtered line when a
	// withdraw finds no inventory room at all.
	bankInventoryFullMessage = "You don't have enough inventory space."
	// bankWithdrawPartialMessage is WITHDRAW_PARTIAL_MESSAGE
	// (Bank.ts:19): spam, when a withdraw is shortened to
	// the free inventory slots.
	bankWithdrawPartialMessage = "You don't have enough inventory space to withdraw that many."
	// bankNewTabMessage is NEW_TAB_MESSAGE (Bank.ts:20): an
	// empty non-zero tab was picked.
	bankNewTabMessage = "To create a new tab, drag items from your bank onto this tab."
	// bankFullMessage is the Bank.full default (Bank.ts:43-48).
	bankFullMessage = "Not enough space in bank."
	// bankCollapseNoRoomMessage is the collapse refusal
	// (Bank.ts:578-580).
	bankCollapseNoRoomMessage = "You don't have enough free slots in your main tab to do that."
)

// Bank widget / varp / varbit constants (Bank.ts:66-190).
const (
	// BankSideInterfaceID is SIDE_INTERFACE_ID (Bank.ts:74).
	BankSideInterfaceID = 15
	// BankTabsChild is TABS_CHILD (Bank.ts:78): the tab bar.
	BankTabsChild = 10
	// BankTabButtonSlotOffset is TAB_BUTTON_SLOT_OFFSET
	// (Bank.ts:79): slot 10 is "all items".
	BankTabButtonSlotOffset = 10
	// BankSearchChild is SEARCH_CHILD (Bank.ts:81).
	BankSearchChild = 42
	// BankTabHeadingFirstSlot / BankTabHeadingLastSlot are
	// TAB_HEADING_FIRST_SLOT / LAST_SLOT (Bank.ts:83-84):
	// bankmain:items slots 1419-1427, the tab headings in
	// the all-items view.
	BankTabHeadingFirstSlot = 1419
	BankTabHeadingLastSlot  = 1427
	// BankSearchInputMode is SEARCH_INPUT_MODE (Bank.ts:85):
	// varc 5's mode while the bank search is typed.
	BankSearchInputMode = 11
	// BankMeslayerCloseScript is MESLAYER_CLOSE_SCRIPT
	// (Bank.ts:87): closes the chatbox input in a mode.
	BankMeslayerCloseScript = 101
	// BankViewContainerVarbit / BankViewContainerObjVarp are
	// VIEW_CONTAINER_VARBIT / VIEW_CONTAINER_OBJ_VARP
	// (Bank.ts:90-91).
	BankViewContainerVarbit  = 9727
	BankViewContainerObjVarp = 4385
	// BankItemsChild is ITEMS_CHILD (Bank.ts:139).
	BankItemsChild = 12
	// BankSideItemsChild is SIDE_ITEMS_CHILD (Bank.ts:140).
	BankSideItemsChild = 3
	// BankSideWornChild is SIDE_WORN_CHILD (Bank.ts:146).
	BankSideWornChild = 4
	// BankSideWornFlags is SIDE_WORN_FLAGS (Bank.ts:149).
	BankSideWornFlags = (1 << 1) | (1 << 9) | (1 << 10) | (1 << 17) | (1 << 20)
	// BankFrameChild is FRAME_CHILD (Bank.ts:150).
	BankFrameChild = 2
	// BankSwapInsertChild is SWAP_INSERT_CHILD (Bank.ts:151).
	BankSwapInsertChild = 23
	// BankNoteChild is NOTE_CHILD (Bank.ts:152).
	BankNoteChild = 25
	// BankQuantityXMode is QUANTITY_X_MODE (Bank.ts:155):
	// the quantity-mode index of quantityx.
	BankQuantityXMode = 3
	// BankRequestedQuantityVarbit is REQUESTED_QUANTITY_VARBIT
	// (Bank.ts:157): the remembered X amount.
	BankRequestedQuantityVarbit = 3960
	// BankPlaceholderChild is PLACEHOLDER_CHILD (Bank.ts:158).
	BankPlaceholderChild = 40
	// BankDepositContainersChild is DEPOSIT_CONTAINERS_CHILD
	// (Bank.ts:159).
	BankDepositContainersChild = 45
	// BankDepositInventoryChild is DEPOSIT_INVENTORY_CHILD
	// (Bank.ts:160).
	BankDepositInventoryChild = 47
	// BankDepositWornChild is DEPOSIT_WORN_CHILD (Bank.ts:161).
	BankDepositWornChild = 49
	// BankWornSlotFirstChild / BankWornSlotLastChild are
	// WORN_SLOT_FIRST_CHILD / LAST_CHILD (Bank.ts:167-168).
	BankWornSlotFirstChild = 68
	BankWornSlotLastChild  = 78
	// BankWornSlotBankOp / BankWornSlotExamineOp are
	// WORN_SLOT_BANK_OP / EXAMINE_OP (Bank.ts:169-170).
	BankWornSlotBankOp    = 2
	BankWornSlotExamineOp = 10
	// BankIncineratorChild is INCINERATOR_CHILD (Bank.ts:172).
	BankIncineratorChild = 54
	// BankTabDropSlotOffset is TAB_DROP_SLOT_OFFSET
	// (Bank.ts:174): drops on the empty space after tab N's
	// items report item-grid slot 1428 + N.
	BankTabDropSlotOffset = 1410 + 9*2
	// BankItemSlotFlags / BankItemSlotOps are ITEM_SLOT_FLAGS /
	// ITEM_SLOT_OPS (Bank.ts:178-181).
	BankItemSlotFlags = 0x7fe | (2 << 17) | (1 << 20) | (1 << 21)
	BankItemSlotOps   = 0x7ff
	// BankTabButtonFlags is TAB_BUTTON_FLAGS (Bank.ts:183).
	BankTabButtonFlags = 0x7fe | (1 << 17) | (1 << 20)
	// BankExamineOp is EXAMINE_OP (Bank.ts:184).
	BankExamineOp = 11
	// BankSideEquipOp is SIDE_EQUIP_OP (Bank.ts:186): op 9 on
	// a bank side-panel item equips it.
	BankSideEquipOp = 9
	// BankSideWearOpsVarp is SIDE_WEAR_OPS_VARP (Bank.ts:190).
	BankSideWearOpsVarp = 262
	// BankLockedSlotsVarp is LOCKED_SLOTS_VARP (Bank.ts:52).
	BankLockedSlotsVarp = 4611
	// BankSettingsInterfaceID is the settings interface
	// (32500, BankBooths / handleButton, Bank.ts:1099).
	BankSettingsInterfaceID = 32500
)

// bankQuantityChildren are QUANTITY_CHILDREN (Bank.ts:154):
// bankmain:quantity1, quantity5, quantity10, quantityx,
// quantityall.
var bankQuantityChildren = [5]int{29, 31, 33, 35, 37}

// bankWornSlotOrder is the equipment slot for each worn-panel
// child 68-78, in enum 2777's slot order (Bank.ts:164-166):
// the same order as EquipPacketListener.EQUIPMENT_SLOTS
// (internal/listeners/equip.go), restated here because the
// game layer cannot import the listeners package.
var bankWornSlotOrder = [11]int{
	HeadSlot, CapeSlot, AmuletSlot, WeaponSlot, BodySlot,
	ShieldSlot, LegSlot, HandsSlot, FeetSlot, RingSlot,
	AmmunitionSlot,
}

// bankResolveWornSlot maps a worn-panel child id to its
// equipment slot, or -1 (resolveEquipmentSlot,
// EquipPacketListener.ts:66-72, group 12 first child 68).
func bankResolveWornSlot(childID int) int {
	index := childID - BankWornSlotFirstChild
	if index < 0 || index >= len(bankWornSlotOrder) {
		return -1
	}
	return bankWornSlotOrder[index]
}

// ---- layout ------------------------------------------------

// BankLayoutEntry is one entry of Bank.layout
// (Bank.ts:523-533): the tab, the slot within it, and the
// item. Items are copies; mutate through SetItem, never
// through the entry.
type BankLayoutEntry struct {
	Tab  int
	Slot int
	Item Item
}

// BankLayout returns the bank in the order the client lays it
// out (Bank.ts:523-533): tabs 1-9, then the main tab. The
// cache scripts find each tab's range from the tab-size
// varbits (4171-4179), so this order must match them.
// Placeholders are included. The search tab is not part of
// the layout. (The cmd/server adapter's bankLayoutSlots
// builds the same order for the snapshot encoding.)
func BankLayout(p *Player) []BankLayoutEntry {
	if p == nil {
		return nil
	}
	var entries []BankLayoutEntry
	for _, tab := range []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 0} {
		bank := p.Bank(tab)
		if bank == nil {
			continue
		}
		for slot, item := range bank.Items() {
			if item.ID() >= 0 && item.Amount() >= 0 {
				entries = append(entries, BankLayoutEntry{Tab: tab, Slot: slot, Item: item})
			}
		}
	}
	return entries
}

// BankDisplayItemID is Bank.displayItemId (Bank.ts:536-543):
// a live stack shows its own id; a placeholder (amount 0)
// shows its faded cache placeholder id when it has one, else
// its own id. The TS fallback through the plugin placeholder
// family reduces to the same cache term without plugins (see
// bankDisplayItemID in cmd/server/containerswiring.go).
func BankDisplayItemID(p *Player, item Item) int {
	if item.Amount() > 0 {
		return item.ID()
	}
	if src := p.ItemDefinitions(); src != nil {
		if placeholder := src.PlaceholderID(item.ID()); placeholder >= 0 {
			return placeholder
		}
	}
	return item.ID()
}

// BankResolveDisplaySlot resolves a client bank-grid slot to
// its layout entry (Bank.ts:789-792, resolveDisplaySlot): the
// layout entry at that index, or false for a negative or
// out-of-range slot.
func BankResolveDisplaySlot(p *Player, clientSlot int) (BankLayoutEntry, bool) {
	if p == nil || clientSlot < 0 {
		return BankLayoutEntry{}, false
	}
	layout := BankLayout(p)
	if clientSlot >= len(layout) {
		return BankLayoutEntry{}, false
	}
	return layout[clientSlot], true
}

// ---- open / refresh / close ---------------------------------

// BankOpen opens the bank on p's current tab (Bank.open,
// Bank.ts:1354-1414). The debounce is the content layer's
// job (BankBooths, 1 tick); here the state set and the send
// sequence run. Re-opening on a tab change re-sends the
// sequence without the opening-only steps.
func BankOpen(p *Player) {
	bankOpen(p, BankCurrentTab(p))
}

// bankOpen opens the bank on tab (Bank.open is an instance
// method on the tab being opened; callers pass the current
// tab, the search tab, or the re-opened tab from
// reconfigureTabs).
func bankOpen(p *Player, tab int) {
	if p == nil {
		return
	}
	// emitCanBank gate (PIN hook): no plugin answers it in
	// Go, so the open proceeds (see the file header).
	opening := !BankIsOpen(p)
	if opening {
		BankSetNoteWithdrawal(p, false)
	}
	// setStatus(BANKING): the real status field (status.go,
	// Worker A). BankIsOpen reads it back together with the
	// interface id.
	p.SetStatus(PlayerStatusBanking)
	p.SetEnteredSyntaxAction(nil)
	if bank := p.Bank(tab); bank != nil {
		bank.SortItems()
	}
	p.SetInterfaceId(BankMainInterfaceID)
	if sender := bankSender; sender != nil {
		sender.SendBankOpen(p, bankOpenView(p, opening))
	}
	BankRefreshTab(p, tab)
	if opening {
		SendSound(p, SoundContainerOpen)
		bankEmit(p, BankOpenedEventName, BankOpenedEvent{Player: p})
	}
}

// bankOpenView builds the open-sequence view (Bank.ts:1366-1387):
// the varps {548:1, 262:sideWearOpsMask, 4611:MAX-capacity}
// and varbits {3755, 3958, 3959, 3960, 4150, 4170:0, 5450:1,
// 6590, 4171-4179 tab sizes}.
func bankOpenView(p *Player, opening bool) BankOpenView {
	varps := map[int]int{
		548:                 1,
		BankSideWearOpsVarp: bankSideWearOpsMask(p),
		BankLockedSlotsVarp: MaxBankSlots - bankCapacity(p),
	}
	varbits := map[int]int{
		3755:                        boolToInt(BankPlaceholders(p)),
		3958:                        boolToInt(BankWithdrawAsNote(p)),
		3959:                        boolToInt(BankInsertMode(p)),
		BankRequestedQuantityVarbit: BankCustomQuantity(p),
		4150:                        BankCurrentTab(p),
		4170:                        0,
		5450:                        1,
		6590:                        BankQuantityMode(p),
	}
	for tab := 1; tab <= 9; tab++ {
		if bank := p.Bank(tab); bank != nil {
			varbits[4170+tab] = len(bank.GetValidItems())
		} else {
			varbits[4170+tab] = 0
		}
	}
	reset := false
	if opening {
		if reader, ok := p.Session.(VarbitReader); ok && reader.GetVarbit(BankViewContainerVarbit) != 0 {
			reset = true
		}
	}
	return BankOpenView{Opening: opening, Varps: varps, Varbits: varbits, ResetViewContainer: reset}
}

// bankSideWearOpsMask is Bank.sideWearOpsMask (Bank.ts:861-868):
// one bit per inventory slot holding something that can be
// worn (equipment slot >= 0).
func bankSideWearOpsMask(p *Player) int {
	if p == nil {
		return 0
	}
	mask := 0
	src := p.ItemDefinitions()
	if src == nil {
		return 0
	}
	if inv := p.Inventory(); inv != nil {
		for slot, item := range inv.Items() {
			if item.ID() > 0 && src.EquipmentSlot(item.ID()) >= 0 {
				mask |= 1 << slot
			}
		}
	}
	return mask
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// BankRefresh refreshes the bank on p's current tab
// (Bank.refreshItems, Bank.ts:1417-1433).
func BankRefresh(p *Player) {
	BankRefreshTab(p, BankCurrentTab(p))
}

// BankRefreshTab refreshes the bank on tab: reconfigureTabs
// first (which may re-open a tab and return early), then the
// tab-count varbits, the bank snapshot, and the inventory
// snapshot — in Go, the last three ride the container refresh
// hook the adapter installed (sendBankRefresh), so refreshing
// the tab is enough — and the bank:refreshed event. Varbit
// 4150 is deliberately NOT re-sent on refresh (Bank.ts:1421-1424:
// the client picks its tab locally).
func BankRefreshTab(p *Player, tab int) {
	if p == nil {
		return
	}
	if BankReconfigureTabs(p) {
		return
	}
	if bank := p.Bank(tab); bank != nil {
		bank.RefreshItems()
	}
	bankEmit(p, BankRefreshedEventName, BankRefreshedEvent{Player: p})
}

// BankClose closes the bank for p: the game half of closing
// (sendInterfaceRemoval, PacketSender.ts:1011-1025). The
// session's removal resets varp 548 -> 0 and varbit 12393
// -> 0 when the open interface is 12, aborts the prompt,
// and resets the dialogue; the SetStatusNone / EndBankSearch
// deps stay nil until the coordinator wires them to
// BankClearStatus / BankEndSearch (bankstate.go), so this
// clears the bank state directly as well. It sends no sound;
// callers play CONTAINER_CLOSE where TS does.
func BankClose(p *Player) {
	if p == nil {
		return
	}
	BankClearStatus(p)
	BankSetSearching(p, false)
	BankSetSearchSyntax(p, "")
	if sender := bankDialogueSender(p); sender != nil {
		sender.SendInterfaceRemoval()
	}
}

// BankFull runs the bank-full behaviour (Bank.full,
// Bank.ts:43-48): the bank:full event (a handler may rewrite
// the message) and the resulting message. Used by the
// move-to-tab / move-item paths that call target.full()
// directly; the container Full() path (switchItem) keeps its
// own default message (see container.go).
func BankFull(p *Player) {
	if p == nil {
		return
	}
	ev := &BankFullEvent{Player: p, Message: bankFullMessage}
	bankEmit(p, BankFullEventName, ev)
	p.SendMessage(ev.Message)
}

// ---- amounts -------------------------------------------------

// BankAmountKind is the actionAmount / asksAmount kind
// ("withdraw" | "deposit", Bank.ts:792-821).
type BankAmountKind string

const (
	// BankAmountWithdraw is the withdraw kind.
	BankAmountWithdraw BankAmountKind = "withdraw"
	// BankAmountDeposit is the deposit kind.
	BankAmountDeposit BankAmountKind = "deposit"
)

// BankActionAmount is Bank.actionAmount (Bank.ts:792-821):
// the amount an op moves. Option suffixes override the
// button: -1/-5/-10/-x/-all (withdraw also -all-but-1).
// Withdraw: op 1 = the quantity mode default, 2 = 1, 3 = 5,
// 4 = 10, 5 = custom, 6 = custom (X asks first), 7 = All,
// 8 = All-but-1, 9/10 = 0. Deposit: op 1 = 1, 2 = the
// quantity mode default, 3 = 1, 4 = 5, 5 = 10, 6 = custom,
// 7 = custom (X asks first), 8 = All. available is the
// stack amount the op draws from; custom is the remembered
// X; defaultMode is the quantity mode (0-4).
func BankActionAmount(kind BankAmountKind, button int, option string, available, custom, defaultMode int) int {
	total := available
	if total < 0 {
		total = 0
	}
	named := strings.TrimSpace(strings.ToLower(option))
	if named != "" {
		if strings.HasSuffix(named, "-all-but-1") {
			if total-1 < 0 {
				return 0
			}
			return total - 1
		}
		if strings.HasSuffix(named, "-1") {
			return min(total, 1)
		}
		if strings.HasSuffix(named, "-5") {
			return min(total, 5)
		}
		if strings.HasSuffix(named, "-10") {
			return min(total, 10)
		}
		if strings.HasSuffix(named, "-all") {
			return total
		}
		if strings.HasSuffix(named, "-x") {
			return min(total, custom)
		}
	}
	defaultAmounts := [5]int{1, 5, 10, custom, total}
	defaultAmount := 1
	if defaultMode >= 0 && defaultMode < len(defaultAmounts) {
		defaultAmount = defaultAmounts[defaultMode]
	}
	if kind == BankAmountWithdraw {
		if button == 1 {
			return min(total, defaultAmount)
		}
		amounts := map[int]int{2: 1, 3: 5, 4: 10, 5: custom, 6: custom, 7: total, 8: total - 1}
		amount, ok := amounts[button]
		if !ok {
			return 0
		}
		if amount < 0 {
			amount = 0
		}
		return min(total, amount)
	}
	if button == 2 {
		return min(total, defaultAmount)
	}
	amounts := map[int]int{1: 1, 3: 1, 4: 5, 5: 10, 6: custom, 7: custom, 8: total}
	amount, ok := amounts[button]
	if !ok {
		return 0
	}
	return min(total, amount)
}

// BankAsksAmount is Bank.asksAmount (Bank.ts:824-837):
// whether an item op asks for the amount first — Withdraw-X
// (op 6) and Deposit-X (op 7), as captured, or the default
// click in X mode before any amount was entered.
func BankAsksAmount(kind BankAmountKind, button int, option string, custom, defaultMode int) bool {
	named := strings.TrimSpace(strings.ToLower(option))
	xOp := false
	if named != "" {
		xOp = strings.HasSuffix(named, "-x")
	} else if kind == BankAmountWithdraw {
		xOp = button == 6
	} else {
		xOp = button == 7
	}
	defaultOp := false
	if kind == BankAmountWithdraw {
		defaultOp = button == 1
	} else {
		defaultOp = button == 2
	}
	return xOp || (defaultOp && defaultMode == BankQuantityXMode && custom <= 0)
}

// BankPromptAmount is Bank.promptAmount (Bank.ts:844-854):
// the "Enter amount:" chatbox prompt (meslayer_mode7). The
// answer becomes the remembered X (varbit 3960), then `then`
// runs with it in the same tick. The action is armed first
// and the frame sent through the BankSender; with no sender
// the action still arms, so tests (and the dispatch) can
// answer it without a client.
func BankPromptAmount(p *Player, then func(amount int)) {
	if p == nil {
		return
	}
	p.SetEnteredAmountAction(&EnteredAmountAction{Execute: func(amount int) {
		if !BankIsOpen(p) {
			return
		}
		BankSetCustomQuantity(p, amount)
		bankSendVarbit(p, BankRequestedQuantityVarbit, BankCustomQuantity(p))
		if then != nil {
			then(BankCustomQuantity(p))
		}
	}})
	if sender := bankSender; sender != nil {
		sender.SendAmountPrompt(p, "Enter amount:")
	}
}

// ---- withdraw / deposit -------------------------------------

// BankWithdraw is Bank.withdraw (Bank.ts:290-375): withdraw
// amount of item in slot of fromBankTab. From the search tab
// the item resolves to its real tab (BankGetTabForItem) and
// the slot is re-resolved there.
func BankWithdraw(p *Player, itemID, slot, amount, fromBankTab int) {
	if p == nil || !BankIsOpen(p) {
		return
	}
	itemTab := fromBankTab
	if BankSearching(p) {
		itemTab = BankGetTabForItem(p, itemID)
	}
	bank := p.Bank(itemTab)
	if bank == nil {
		return
	}
	maxAmount := bank.GetAmount(itemID)
	if amount == -1 || amount > maxAmount {
		amount = maxAmount
	}
	if BankSearching(p) {
		if !bank.ContainsNumber(itemID) || !p.Bank(BankSearchTabIndex).ContainsNumber(itemID) || amount <= 0 {
			return
		}
		if fromBankTab != BankSearchTabIndex {
			return
		}
		slot = bank.GetSlotForItemID(itemID)
		// The aggregate count can include other
		// charge/metadata states; only clone the amount held
		// by this exact source stack.
		amount = min(amount, bank.ItemAt(slot).Amount())
		BankSwitchItem(p, itemTab, p.Inventory(), bankCloneWithAmount(bank, slot, amount), slot, false, false)
		if slot == 0 {
			BankReconfigureTabs(p)
		}
		BankRefreshTab(p, BankSearchTabIndex)
	} else {
		stack := bank.ItemAt(slot)
		if stack.ID() != itemID {
			return
		}
		// Another stack of the same item (different metadata)
		// may share the tab, so take no more than this one
		// holds.
		amount = min(amount, stack.Amount())
		if amount <= 0 {
			// Delete placeholder.
			bank.SetItem(slot, NewItem(-1, 0))
			if current := p.Bank(BankCurrentTab(p)); current != nil {
				current.SortItems()
			}
			BankRefreshTab(p, BankCurrentTab(p))
		} else {
			BankSwitchItem(p, itemTab, p.Inventory(), bankCloneWithAmount(bank, slot, amount), slot, false, false)
			if slot == 0 {
				BankReconfigureTabs(p)
			}
			BankRefreshTab(p, BankCurrentTab(p))
		}
	}
	SendSound(p, SoundPickUpItem)
	if inv := p.Inventory(); inv != nil {
		inv.RefreshItems()
	}
}

// bankCloneWithAmount clones the bank stack in slot with
// amount: the Go form of
// `getItems()[slot].clone().setAmount(amount)` (Bank.ts:335,
// 360), which mutates the clone in place.
func bankCloneWithAmount(bank *Bank, slot, amount int) Item {
	item := bank.ItemAt(slot).Clone()
	item.SetAmount(amount)
	return item
}

// BankSwitchItem is Bank.switchsItem (Bank.ts:1438-1558):
// move item (a clone of the bank stack in slot of tab) to
// container `to` (the inventory in practice). The note,
// free-slot, shorten, max-stack, room, and placeholder
// checks run in TS order; the exact messages are sent at
// the exact sites.
func BankSwitchItem(p *Player, tab int, to Container, item Item, slot int, sort, refresh bool) {
	if p == nil || to == nil || !BankIsOpen(p) {
		return
	}
	bank := p.Bank(tab)
	if bank == nil {
		return
	}
	// Make sure we have the item.
	if bank.ItemAt(slot).ID() != item.ID() || !bank.ContainsNumber(item.ID()) {
		return
	}
	noteID := bankNoteID(p, item.ID())
	canWithdrawAsNote := false
	if noteID >= 0 {
		if src := p.ItemDefinitions(); src != nil && src.IsNoted(noteID) {
			canWithdrawAsNote = strings.EqualFold(bankItemName(p, noteID), bankItemName(p, item.ID()))
		}
	}
	// Make sure we have enough space in the other container.
	_, toIsInventory := to.(*Inventory)
	if to.GetFreeSlots() <= 0 &&
		!(to.ContainsNumber(item.ID()) && itemDefinitionStackable(p, item.ID())) &&
		!(BankWithdrawAsNote(p) && canWithdrawAsNote && to.ContainsNumber(noteID)) {
		if toIsInventory {
			p.SendTypedMessage(chatTypeSpam, bankInventoryFullMessage, "")
		} else {
			to.Full()
		}
		return
	}
	// If bank > inventory and item.amount > inventory free
	// slots, change the item amount to the free slots.
	shortened := false
	if item.Amount() > to.GetFreeSlots() && !itemDefinitionStackable(p, item.ID()) {
		if toIsInventory {
			if BankWithdrawAsNote(p) {
				if !canWithdrawAsNote {
					item.SetAmount(to.GetFreeSlots())
					shortened = true
				}
			} else {
				item.SetAmount(to.GetFreeSlots())
				shortened = true
			}
		}
	}
	if shortened {
		p.SendTypedMessage(chatTypeSpam, bankWithdrawPartialMessage, "")
	}
	// Make sure we aren't taking more than we have.
	if item.Amount() > bank.GetAmount(item.ID()) {
		item.SetAmount(bank.GetAmount(item.ID()))
	}
	if toIsInventory {
		withdrawAsNote := BankWithdrawAsNote(p) && canWithdrawAsNote
		checkID := item.ID()
		if withdrawAsNote {
			checkID = noteID
		}
		if to.GetAmount(checkID)+item.Amount() > MaxStackAmount || to.GetAmount(checkID)+item.Amount() <= 0 {
			item.SetAmount(MaxStackAmount - to.GetAmount(checkID))
			if item.Amount() <= 0 {
				p.SendMessage("You cannot withdraw that entire amount into your inventory.")
				return
			}
		}
	}
	// Make sure the item is still valid.
	if item.Amount() <= 0 {
		return
	}
	// The room checks above go by id: a stack of the same id
	// with other metadata is not one this item joins, so with
	// no free slot the add below would fail after the bank's
	// stack was already taken.
	arriving := item.Clone()
	if BankWithdrawAsNote(p) && canWithdrawAsNote {
		arriving.SetID(noteID)
	}
	room := to.GetAddableAmount(arriving)
	if room <= 0 {
		if toIsInventory {
			p.SendTypedMessage(chatTypeSpam, bankInventoryFullMessage, "")
		} else {
			to.Full()
		}
		return
	}
	if room < item.Amount() {
		item.SetAmount(room)
	}
	bankStackAmount := bank.ItemAt(slot).Amount()
	if toIsInventory && BankPlaceholders(p) &&
		item.Amount() >= bankStackAmount &&
		bank.PlaceholderBankID(item.ID()) < 0 {
		// rsprox_transcripts/227/00549.txt: a spam-filtered
		// game message.
		p.SendTypedMessage(chatTypeSpam, "This item cannot leave a placeholder.", "")
	}
	bank.DeleteItemContainer(&item, slot, !toIsInventory && refresh, to)
	// Check if we can actually withdraw the item as a note.
	if BankWithdrawAsNote(p) {
		if canWithdrawAsNote {
			item.SetID(noteID)
		} else {
			p.SendMessage("This item cannot be withdrawn as a note.")
		}
	}
	// Add the item to the other container.
	var withdrawn []Item
	if toIsInventory {
		// The withdrawn-items event carries the new item
		// instances: a stack the destination did not already
		// hold one of. TS tracks live Item identity; Go
		// approximates by id presence — a same-id stack with
		// different metadata reads as joined, where TS would
		// report a new instance.
		checkID := item.ID()
		hadStack := to.GetAmount(checkID) > 0
		to.Add(item, false)
		if !hadStack && item.Amount() > 0 {
			withdrawn = []Item{item.Clone()}
		}
	} else {
		to.Add(item, refresh)
	}
	if len(withdrawn) > 0 {
		bankEmit(p, BankWithdrawnItemsEventName, BankWithdrawnItemsEvent{Player: p, Items: withdrawn})
	}
	// Sort this container.
	if sort && bank.GetAmount(item.ID()) <= 0 {
		bank.SortItems()
	}
	// Refresh containers.
	if refresh {
		BankRefreshTab(p, tab)
		to.RefreshItems()
	}
	if BankSearching(p) {
		BankRemoveFromBankSearch(p, item.Clone(), true)
	}
}

// itemDefinitionStackable answers IsStackable through the
// player's source, defaulting to false with no source (see
// ItemDefinitionSource).
func itemDefinitionStackable(p *Player, id int) bool {
	if p == nil {
		return false
	}
	if src := p.ItemDefinitions(); src != nil {
		return src.IsStackable(id)
	}
	return false
}

// BankDeposits deposits one inventory stack
// (Bank.deposits, Bank.ts:376-378).
func BankDeposits(p *Player, itemID, slot, amount int) {
	BankDeposit(p, itemID, slot, amount, false)
}

// BankDeposit is Bank.deposit (Bank.ts:380-425): deposit
// amount of item in inventory slot. ignore bypasses the
// isOpen gate (the deposit-box path; unused by the bank
// flows, which always pass false).
func BankDeposit(p *Player, itemID, slot, amount int, ignore bool) {
	if p == nil {
		return
	}
	if !ignore && !BankIsOpen(p) {
		return
	}
	inv := p.Inventory()
	if inv == nil {
		return
	}
	inventoryItem := inv.ItemAt(slot)
	if inventoryItem.ID() != itemID {
		return
	}
	// emitCanBankItem gate: no plugin answers it in Go, so
	// the deposit proceeds (see the file header).
	if amount == -1 || amount > inv.GetAmount(itemID) {
		amount = inv.GetAmount(itemID)
	}
	if amount <= 0 {
		return
	}
	tab := BankGetTabForItem(p, itemID)
	bankDepositFromSlots(p, p.Bank(tab), slot, amount, false)
	if !BankSearching(p) {
		BankRefreshTab(p, tab)
	}
	if BankSearching(p) {
		BankRefreshTab(p, BankSearchTabIndex)
	}
	SendSound(p, SoundDropItem)
	inv.RefreshItems()
}

// bankDepositFromSlots is Bank.depositFromSlots
// (Bank.ts:433-451): move up to amount of the inventory item
// in slot into bank, each item as itself so it keeps its
// metadata. A stack moves from the clicked slot only.
// Unstackable items go from the first (top-left) bankable
// slot holding the same item onward, wherever the click
// landed, as in OSRS; a dragged item (fromClickedSlot)
// starts at the slot it was dragged from.
func bankDepositFromSlots(p *Player, bank *Bank, slot, amount int, fromClickedSlot bool) {
	if p == nil || bank == nil {
		return
	}
	inv := p.Inventory()
	if inv == nil {
		return
	}
	clicked := inv.ItemAt(slot)
	if clicked.ID() < 0 {
		return
	}
	if itemDefinitionStackable(p, clicked.ID()) {
		item := clicked.Clone()
		if amount < item.Amount() {
			item.SetAmount(amount)
		}
		inv.SwitchItem(bank, &item, false, slot, false)
		return
	}
	id := clicked.ID()
	indexes := make([]int, InventoryCapacity)
	for i := range indexes {
		indexes[i] = i
	}
	slots := indexes
	if fromClickedSlot {
		slots = []int{slot}
		for _, index := range indexes {
			if index != slot {
				slots = append(slots, index)
			}
		}
	}
	moved := 0
	for _, index := range slots {
		if moved >= amount {
			break
		}
		item := inv.ItemAt(index)
		if item.ID() != id {
			continue
		}
		// emitCanBankItem gate: no plugin answers it.
		clone := item.Clone()
		inv.SwitchItem(bank, &clone, false, index, false)
		moved++
	}
}

// BankDepositItems is Bank.depositItems (Bank.ts:1200-1233):
// bank [from]'s items, or only the one in onlySlot (a worn
// panel slot's Bank op). from is the inventory or the
// equipment; ignoreReqs bypasses the isOpen gate.
func BankDepositItems(p *Player, from Container, ignoreReqs bool, onlySlot ...int) {
	if p == nil || from == nil {
		return
	}
	if !ignoreReqs && !BankIsOpen(p) {
		return
	}
	var src *itemContainer
	_, fromIsEquipment := from.(*Equipment)
	switch c := from.(type) {
	case *Inventory:
		src = c.itemContainer
	case *Equipment:
		src = c.itemContainer
	default:
		return
	}
	movedAny := false
	for slot := 0; slot < from.Capacity(); slot++ {
		if len(onlySlot) > 0 && slot != onlySlot[0] {
			continue
		}
		item := from.ItemAt(slot)
		if item.ID() < 0 || item.Amount() <= 0 {
			continue
		}
		// emitCanBankItem gate: no plugin answers it.
		// By slot: a delete by id would take a preset stack
		// in a lower slot instead of this one.
		clone := item.Clone()
		src.SwitchItem(p.Bank(BankGetTabForItem(p, item.ID())), &clone, false, slot, false)
		movedAny = true
	}
	from.RefreshItems()
	if BankSearching(p) {
		BankRefreshTab(p, BankSearchTabIndex)
	} else {
		BankRefreshTab(p, BankCurrentTab(p))
	}
	if movedAny {
		if fromIsEquipment {
			SendSound(p, SoundEquipmentOff)
		} else {
			SendSound(p, SoundDropItem)
		}
	}
	if fromIsEquipment {
		// WeaponInterfaceManager.assign + BonusManager.update
		// + APPEARANCE flag (Bank.ts:1228-1231). The assign
		// runs (AssignWeaponInterface); BonusManager has no
		// cached array in Go — bonuses compute live — so the
		// update is a documented no-op (see equip.go); the
		// APPEARANCE flag already ran through
		// onEquipmentChanged on the equipment refresh above.
		AssignWeaponInterface(p)
	}
}

// ---- tabs ---------------------------------------------------

// BankIsEmpty is Bank.isEmpty (Bank.ts:1236-1238): the tab
// sorts, then reads empty when it holds no valid stacks
// (placeholders count as held).
func BankIsEmpty(bank *Bank) bool {
	if bank == nil {
		return true
	}
	bank.SortItems()
	return len(bank.GetValidItems()) <= 0
}

// BankGetTabCount is Bank.getTabCount (Bank.ts:1286-1299):
// the consecutive non-empty custom tabs from 1.
func BankGetTabCount(p *Player) int {
	if p == nil {
		return 0
	}
	tabs := 0
	for i := 1; i < TotalBankTabs; i++ {
		if i == BankSearchTabIndex {
			continue
		}
		if !BankIsEmpty(p.Bank(i)) {
			tabs++
		} else {
			break
		}
	}
	return tabs
}

// BankReconfigureTabs is Bank.reconfigureTabs
// (Bank.ts:1255-1284): compact the tabs, shifting each empty
// tab's successor down, then re-open the highest non-empty
// tab when the viewed tab went empty. It reports whether it
// re-opened a tab (Bank.refreshItems returns early then).
func BankReconfigureTabs(p *Player) bool {
	if p == nil {
		return false
	}
	updateRequired := false
	for k := 1; k < BankSearchTabIndex-1; k++ {
		if BankIsEmpty(p.Bank(k)) || updateRequired {
			p.SetBank(k, p.Bank(k+1))
			p.SetBank(k+1, NewBank(p))
			updateRequired = true
		}
	}
	totalTabs := BankGetTabCount(p)
	if !BankSearching(p) {
		if BankCurrentTab(p) > totalTabs {
			BankSetCurrentTab(p, totalTabs)
			bankOpen(p, totalTabs)
			return true
		}
	}
	return false
}

// BankGetTabForItem is Bank.getTabForItem (Bank.ts:1303-1345):
// the tab an item deposits into — the tab already holding
// it (or its placeholder family), else the current tab when
// it has room, else the first tab with room, else 0. Notes
// un-note first. The placeholder-family loop degenerates:
// families are identity-only without the plugin event (see
// PlaceholderFamily), so the family minus the id is empty.
func BankGetTabForItem(p *Player, itemID int) int {
	if p == nil {
		return 0
	}
	if src := p.ItemDefinitions(); src != nil {
		itemID = src.UnNote(itemID)
	}
	for k := 0; k < TotalBankTabs; k++ {
		if k == BankSearchTabIndex {
			continue
		}
		if bank := p.Bank(k); bank != nil && bank.ContainsNumber(itemID) {
			return k
		}
	}
	family := PlaceholderFamily(itemID)
	for _, id := range family {
		if id == itemID {
			continue
		}
		for k := 0; k < TotalBankTabs; k++ {
			if k == BankSearchTabIndex {
				continue
			}
			if bank := p.Bank(k); bank != nil {
				for _, held := range bank.Items() {
					if held.ID() != itemID && held.Amount() == 0 && held.ID() == id {
						return k
					}
				}
			}
		}
	}
	if bank := p.Bank(BankCurrentTab(p)); bank != nil && bank.GetFreeSlots() > 0 {
		return BankCurrentTab(p)
	}
	for k := 0; k < TotalBankTabs; k++ {
		if k == BankSearchTabIndex {
			continue
		}
		if bank := p.Bank(k); bank != nil && bank.GetFreeSlots() > 0 {
			return k
		}
	}
	return 0
}

// BankContains reports whether the bank holds at least
// item's amount of its id (a note's item), in every tab
// together (Bank.contains, Bank.ts:1347-1357). The search tab
// only shows copies; placeholders add 0.
func BankContains(p *Player, item Item) bool {
	if p == nil {
		return false
	}
	id := item.ID()
	if src := p.ItemDefinitions(); src != nil && src.IsNoted(id) {
		id = src.UnNote(id)
	}
	amount := 0
	for tab := 0; tab < TotalBankTabs; tab++ {
		if tab == BankSearchTabIndex {
			continue
		}
		if bank := p.Bank(tab); bank != nil {
			amount += bank.GetAmount(id)
		}
	}
	return amount >= item.Amount()
}

// BankViewTab is Bank.viewTab (Bank.ts:554-564): view tab,
// or the new-tab message when it is past the tab count. The
// client tracks the tab it is viewing itself; varbit 4150 is
// sent only when the server changes the tab for it.
func BankViewTab(p *Player, tab int) {
	if p == nil {
		return
	}
	if tab > BankGetTabCount(p) {
		p.SendMessage(bankNewTabMessage)
		return
	}
	BankSetCurrentTab(p, tab)
	bankSendVarbit(p, 4150, tab)
	BankRefreshTab(p, tab)
}

// BankCollapseTab is Bank.collapseTab (Bank.ts:566-592):
// move every item in tab (placeholders included) into the
// main tab. The main tab must have room; the move runs
// without the capacity limit (Bank.withoutLimit), which Go
// approximates with bankAddIgnoringCapacity (container.go
// documents the withoutLimit gap).
func BankCollapseTab(p *Player, tab int) {
	if p == nil {
		return
	}
	if tab <= 0 || tab > BankGetTabCount(p) {
		return
	}
	bank := p.Bank(tab)
	if bank == nil {
		return
	}
	items := bank.GetValidItems()
	main := p.Bank(0)
	if main == nil {
		return
	}
	if bankTabFreeSlotsIgnoringCapacity(main) < len(items) {
		p.SendMessage(bankCollapseNoRoomMessage)
		return
	}
	for _, item := range items {
		bankAddIgnoringCapacity(main, item.Clone())
	}
	bank.ResetItems()
	if BankCurrentTab(p) == tab {
		BankSetCurrentTab(p, 0)
		bankSendVarbit(p, 4150, 0)
	}
	BankRefreshTab(p, 0)
	// requestSave("bank-tab-collapse"): no bank save-request
	// seam exists in Go; banks persist through the normal
	// save path (see the file header).
}

// bankTabFreeSlotsIgnoringCapacity counts the slots tab 0
// can still take ignoring the bank-space capacity: its empty
// slots plus the growth headroom to MAX_SLOTS. It is the Go
// form of Bank.withoutLimit(getBank(0).getFreeSlots()) in
// collapseTab (Bank.ts:578-580).
func bankTabFreeSlotsIgnoringCapacity(bank *Bank) int {
	if bank == nil {
		return 0
	}
	free := 0
	length := 0
	for _, item := range bank.Items() {
		length++
		if item.ID() < 0 {
			free++
		}
	}
	if headroom := MaxBankSlots - length; headroom > 0 {
		free += headroom
	}
	return free
}

// bankAddIgnoringCapacity adds item to bank ignoring the
// bank-space capacity (the withoutLimit half of collapseTab,
// Bank.ts:582-586): it merges into a live stack of the same
// id with identical metadata first (STACKS semantics, capped
// at MAX_STACK_AMOUNT, excess silently lost as in Add),
// else takes the tab's free slot, growing it. It reports
// false only when the tab is MAX_SLOTS long with no empty
// slot — unreachable after the caller's room check.
func bankAddIgnoringCapacity(bank *Bank, item Item) bool {
	if bank == nil || item.ID() < 0 || item.Amount() < 0 {
		return false
	}
	if item.Amount() > 0 {
		for slot, held := range bank.Items() {
			if held.ID() == item.ID() && held.Amount() > 0 && held.metaFingerprint() == item.metaFingerprint() {
				total := held.Amount() + item.Amount()
				if total > MaxStackAmount {
					total = MaxStackAmount
				}
				merged := item.Clone()
				merged.SetAmount(total)
				bank.SetItem(slot, merged)
				return true
			}
		}
	}
	slot := bank.FreeSlot()
	if slot < 0 {
		return false
	}
	bank.SetItem(slot, item.Clone())
	return true
}

// BankReleasePlaceholder is Bank.releasePlaceholder
// (Bank.ts:545-551): remove the placeholder in slot of tab.
func BankReleasePlaceholder(p *Player, tab, slot int) {
	if p == nil {
		return
	}
	bank := p.Bank(tab)
	if bank == nil {
		return
	}
	item := bank.ItemAt(slot)
	if item.ID() < 0 || item.Amount() != 0 {
		return
	}
	bank.SetItem(slot, NewItem(-1, 0))
	BankRefreshTab(p, tab)
}

// BankReleasePlaceholders is Bank.releasePlaceholders
// (Bank.ts:594-604): remove the placeholders in tab, or in
// every tab for "all items" (0).
func BankReleasePlaceholders(p *Player, tab int) {
	if p == nil {
		return
	}
	var tabs []int
	if tab == 0 {
		tabs = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9}
	} else {
		tabs = []int{tab}
	}
	for _, index := range tabs {
		bank := p.Bank(index)
		if bank == nil {
			continue
		}
		for slot, item := range bank.Items() {
			if item.ID() >= 0 && item.Amount() == 0 {
				bank.SetItem(slot, NewItem(-1, 0))
			}
		}
	}
	BankRefreshTab(p, 0)
	// requestSave("bank-placeholder-clear"): no bank
	// save-request seam exists in Go (see the file header).
}

// ---- search -------------------------------------------------

// BankSearch is Bank.search (Bank.ts:453-481): fill the search
// tab with clones of the name-matching items and open it.
func BankSearch(p *Player, syntax string) {
	if p == nil || !BankIsOpen(p) {
		return
	}
	BankSetSearchSyntax(p, syntax)
	BankSetSearching(p, true)
	if search := p.Bank(BankSearchTabIndex); search != nil {
		search.ResetItems()
	}
	for i := 0; i < TotalBankTabs; i++ {
		if i == BankSearchTabIndex {
			continue
		}
		bank := p.Bank(i)
		if bank == nil {
			continue
		}
		bank.SortItems()
		for _, item := range bank.GetValidItems() {
			if item.Amount() > 0 {
				BankAddToSearch(p, item.Clone(), false)
			}
		}
	}
	BankSetCurrentTab(p, 0)
	bankOpen(p, BankSearchTabIndex)
}

// BankAddToSearch is Bank.addToBankSearch (Bank.ts:505-516):
// add item to the search tab when its name contains the
// search syntax. The name is lowercased; the syntax is
// compared as typed (TS-literal).
func BankAddToSearch(p *Player, item Item, refresh bool) {
	if p == nil {
		return
	}
	search := p.Bank(BankSearchTabIndex)
	if search == nil || search.GetFreeSlots() == 0 {
		return
	}
	if strings.Contains(strings.ToLower(bankItemName(p, item.ID())), BankSearchSyntax(p)) {
		search.Add(item, refresh)
	}
}

// BankRemoveFromBankSearch is Bank.removeFromBankSearch
// (Bank.ts:1044-1054): remove item from the search tab, after
// un-noting it.
func BankRemoveFromBankSearch(p *Player, item Item, refresh bool) {
	if p == nil {
		return
	}
	if src := p.ItemDefinitions(); src != nil && src.IsNoted(item.ID()) {
		item.SetID(src.UnNote(item.ID()))
	}
	if search := p.Bank(BankSearchTabIndex); search != nil {
		search.DeleteBoolean(item, refresh)
	}
}

// BankExitSearch is Bank.exitSearch (Bank.ts:483-503): clear
// the search and, when openBank, re-open the last tab.
func BankExitSearch(p *Player, openBank bool) {
	if p == nil || !BankIsOpen(p) {
		return
	}
	BankSetSearchSyntax(p, "")
	BankSetSearching(p, false)
	if search := p.Bank(BankSearchTabIndex); search != nil {
		search.ResetItems()
	}
	if BankCurrentTab(p) == BankSearchTabIndex {
		BankSetCurrentTab(p, 0)
	}
	if openBank {
		BankOpen(p)
	}
}

// BankCloseSearch is Bank.closeSearch (Bank.ts:137-139): end
// the client's bank search, as OSRS does on closing the bank
// or picking a tab (meslayer_close, script 101 mode 11).
func BankCloseSearch(p *Player) {
	if p == nil {
		return
	}
	if sender := bankDialogueSender(p); sender != nil {
		sender.SendClientScript(BankMeslayerCloseScript, ClientScriptInt(BankSearchInputMode))
	}
}

// ---- drag ---------------------------------------------------

// BankDragPacket is the IF_BUTTOND drag packet
// (BankDragPacket, Bank.ts:22-25): the source and target
// widget uids (group << 16 | child), slots, and the source
// item id.
type BankDragPacket struct {
	SourceWidgetID int
	SourceSlot     int
	SourceItemID   int
	TargetWidgetID int
	TargetSlot     int
}

// BankHandleDrag is Bank.handleDrag (Bank.ts:607-611): handle
// a drag within the bank or its inventory panel. It reports
// false for drags the bank doesn't own.
func BankHandleDrag(p *Player, drag BankDragPacket) bool {
	return bankAbortingPrompt(p, func() bool { return bankDragWidget(p, drag) })
}

// bankDragWidget is Bank.dragBankWidget (Bank.ts:613-695).
func bankDragWidget(p *Player, drag BankDragPacket) bool {
	if p == nil {
		return false
	}
	sourceGroup := drag.SourceWidgetID >> 16
	sourceChild := drag.SourceWidgetID & 0xffff
	targetGroup := drag.TargetWidgetID >> 16
	targetChild := drag.TargetWidgetID & 0xffff
	fromBank := sourceGroup == BankMainInterfaceID && sourceChild == BankItemsChild
	fromInventory := sourceGroup == BankSideInterfaceID && sourceChild == BankSideItemsChild
	fromTab := sourceGroup == BankMainInterfaceID && sourceChild == BankTabsChild
	if !(fromBank || fromInventory || fromTab) || !BankIsOpen(p) {
		return false
	}
	if fromTab {
		from := drag.SourceSlot - BankTabButtonSlotOffset
		to := drag.TargetSlot - BankTabButtonSlotOffset
		count := BankGetTabCount(p)
		// Cache script 512 accepts populated custom tabs
		// only, not the main or new-tab slots.
		if targetGroup != BankMainInterfaceID || targetChild != BankTabsChild ||
			from < 1 || to < 1 || from > count || to > count || from == to {
			return true
		}
		source := p.Bank(from)
		p.SetBank(from, p.Bank(to))
		p.SetBank(to, source)
		viewed := BankCurrentTab(p)
		if viewed == from || viewed == to {
			if viewed == from {
				BankViewTab(p, to)
			} else {
				BankViewTab(p, from)
			}
		} else if bank := p.Bank(0); bank != nil {
			bank.RefreshItems()
		}
		// requestSave("bank-tab-reorder"): no bank
		// save-request seam exists in Go (see the file
		// header).
		return true
	}
	var tab int
	hasTab := false
	if targetGroup == BankMainInterfaceID && targetChild == BankTabsChild {
		tab = drag.TargetSlot - BankTabButtonSlotOffset
		hasTab = true
	} else if targetGroup == BankMainInterfaceID && targetChild == BankItemsChild &&
		drag.TargetSlot >= BankTabDropSlotOffset {
		tab = drag.TargetSlot - BankTabDropSlotOffset
		hasTab = true
	}
	if hasTab && (tab < 0 || tab > 9) {
		return true
	}
	if targetGroup == BankMainInterfaceID && targetChild == BankIncineratorChild {
		// The incinerator destroys bank stacks only. A
		// dragged inventory item must not fall through to
		// the deposit branch and disappear into the bank
		// instead.
		if fromBank {
			bankIncinerateDragged(p, drag.SourceSlot, drag.SourceItemID)
		}
		return true
	}
	if fromInventory && targetGroup == BankSideInterfaceID && targetChild == BankSideItemsChild {
		inv := p.Inventory()
		// Both slots inside the backpack, as
		// SwitchItemSlotPacketListener checks: the client's
		// target is a 16-bit slot, and a swap past capacity
		// left the item outside the inventory and an empty
		// slot behind.
		if inv == nil ||
			drag.SourceSlot < 0 || drag.TargetSlot < 0 ||
			drag.SourceSlot >= InventoryCapacity || drag.TargetSlot >= InventoryCapacity {
			return true
		}
		// Legacy slot-only requests omit the ID or use a
		// negative/16-bit sentinel.
		if drag.SourceItemID >= 0 && drag.SourceItemID != 0xffff &&
			inv.ItemAt(drag.SourceSlot).ID() != drag.SourceItemID {
			return true
		}
		inv.Swap(drag.SourceSlot, drag.TargetSlot)
		inv.RefreshItems()
	} else if fromInventory && targetGroup == BankMainInterfaceID {
		bankDepositDragged(p, drag.SourceSlot, drag.SourceItemID, tab, hasTab)
	} else if fromBank && hasTab {
		BankMoveToTab(p, drag.SourceSlot, drag.SourceItemID, tab)
	} else if fromBank && targetGroup == BankMainInterfaceID && targetChild == BankItemsChild {
		BankMoveItem(p, drag.SourceSlot, drag.SourceItemID, drag.TargetSlot)
	}
	// requestSave("container-reorder"): no bank save-request
	// seam exists in Go (see the file header).
	return true
}

// bankIncinerateDragged is Bank.incinerateDragged
// (Bank.ts:697-715): publish the bank stack a drag released
// on the incinerator for the destroy confirmation. The
// destroy itself has no core consumer (out of scope): only
// the bank:incinerate event is emitted.
func bankIncinerateDragged(p *Player, clientSlot, itemID int) {
	var entry BankLayoutEntry
	found := false
	if BankSearching(p) {
		searched := p.Bank(BankSearchTabIndex).ItemAt(clientSlot)
		if searched.ID() != itemID || searched.Amount() <= 0 {
			return
		}
		for _, e := range BankLayout(p) {
			if e.Item.Equals(searched) {
				entry, found = e, true
				break
			}
		}
		if !found {
			return
		}
	} else {
		e, ok := BankResolveDisplaySlot(p, clientSlot)
		if !ok || BankDisplayItemID(p, e.Item) != itemID || e.Item.Amount() <= 0 {
			return
		}
		entry = e
	}
	bankEmit(p, BankIncinerateEventName, &BankIncinerateEvent{
		Player: p, Item: entry.Item, Tab: entry.Tab, Slot: entry.Slot, Handled: false,
	})
}

// bankEndSlot is Bank.endSlot (Bank.ts:717-725): the slot
// after bank's last item, where a moved item joins the tab;
// -1 if full.
func bankEndSlot(bank *Bank) int {
	if bank == nil {
		return -1
	}
	last := -1
	for slot, item := range bank.Items() {
		if item.ID() >= 0 {
			last = slot
		}
	}
	if last+1 < bank.Capacity() {
		return last + 1
	}
	return bank.FreeSlot()
}

// bankDropTab is Bank.dropTab (Bank.ts:727-730): the tab a
// drop onto tab goes to — an existing tab, or one new tab
// after the last.
func bankDropTab(p *Player, tab int) int {
	count := BankGetTabCount(p)
	if tab < count+1 {
		return tab
	}
	return count + 1
}

// bankDepositDragged is Bank.depositDragged (Bank.ts:732-745):
// deposit an inventory stack dropped on the bank. An item
// already in the bank joins its stack; otherwise it goes to
// the tab it was dropped on, or where a click would deposit
// it.
func bankDepositDragged(p *Player, slot, itemID int, tab int, hasTab bool) {
	if p == nil {
		return
	}
	inv := p.Inventory()
	if inv == nil {
		return
	}
	item := inv.ItemAt(slot)
	if item.ID() < 0 || item.ID() != itemID {
		return
	}
	// emitCanBankItem gate: no plugin answers it.
	unnoted := itemID
	if src := p.ItemDefinitions(); src != nil {
		unnoted = src.UnNote(itemID)
	}
	target := -1
	for index := 0; index <= 9; index++ {
		if bank := p.Bank(index); bank != nil && bank.ContainsNumber(unnoted) {
			target = index
			break
		}
	}
	if target < 0 {
		if hasTab {
			target = bankDropTab(p, tab)
		} else {
			target = BankGetTabForItem(p, itemID)
		}
	}
	amount := 1
	if itemDefinitionStackable(p, item.ID()) {
		amount = item.Amount()
	}
	bankDepositFromSlots(p, p.Bank(target), slot, amount, true)
	BankRefreshTab(p, target)
	inv.RefreshItems()
}

// BankMoveToTab is Bank.moveToTab (Bank.ts:747-760): move the
// bank item at clientSlot to the end of tab, creating the tab
// if it's new.
func BankMoveToTab(p *Player, clientSlot, itemID, tab int) {
	if p == nil {
		return
	}
	entry, ok := BankResolveDisplaySlot(p, clientSlot)
	if !ok || BankDisplayItemID(p, entry.Item) != itemID {
		return
	}
	tab = bankDropTab(p, tab)
	if tab == entry.Tab {
		return
	}
	target := p.Bank(tab)
	if target == nil {
		return
	}
	slot := bankEndSlot(target)
	if slot < 0 {
		BankFull(p)
		return
	}
	target.SetItem(slot, entry.Item.Clone())
	if source := p.Bank(entry.Tab); source != nil {
		source.SetItem(entry.Slot, NewItem(-1, 0))
	}
	BankRefreshTab(p, entry.Tab)
}

// BankMoveItem is Bank.moveItem (Bank.ts:762-788): move the
// bank item at fromClientSlot onto the item at toClientSlot,
// swapping or inserting by the player's setting. Across tabs,
// a swap trades the two items' places and an insert puts the
// dragged item into the other tab there.
func BankMoveItem(p *Player, fromClientSlot, itemID, toClientSlot int) {
	if p == nil {
		return
	}
	from, ok := BankResolveDisplaySlot(p, fromClientSlot)
	if !ok {
		return
	}
	to, ok := BankResolveDisplaySlot(p, toClientSlot)
	if !ok || BankDisplayItemID(p, from.Item) != itemID || fromClientSlot == toClientSlot {
		return
	}
	bank := p.Bank(to.Tab)
	if bank == nil {
		return
	}
	if from.Tab != to.Tab && !BankInsertMode(p) {
		if source := p.Bank(from.Tab); source != nil {
			source.SetItem(from.Slot, to.Item.Clone())
		}
		bank.SetItem(to.Slot, from.Item.Clone())
		BankRefreshTab(p, to.Tab)
		return
	}
	fromSlot := from.Slot
	if from.Tab != to.Tab {
		fromSlot = bankEndSlot(bank)
		if fromSlot < 0 {
			BankFull(p)
			return
		}
		bank.SetItem(fromSlot, from.Item.Clone())
		if source := p.Bank(from.Tab); source != nil {
			source.SetItem(from.Slot, NewItem(-1, 0))
		}
	}
	BankRearrange(p, bank, fromSlot, to.Slot)
	if from.Tab != to.Tab {
		BankRefreshTab(p, from.Tab)
	}
}

// BankRearrange is Bank.rearrange (Bank.ts:1067-1095): move
// the item in fromSlot to toSlot of bank, inserting or
// swapping by the player's setting, then refresh (the Bank
// method refreshes the tab; the packet is the full-bank
// snapshot either way, see bank.go) and reconfigure the tabs
// when a move touched slot 0.
func BankRearrange(p *Player, bank *Bank, fromSlot, toSlot int) {
	if p == nil || bank == nil {
		return
	}
	bank.Rearrange(fromSlot, toSlot, BankInsertMode(p))
	if fromSlot == 0 || toSlot == 0 {
		BankReconfigureTabs(p)
	}
}

// BankMoveSlots is the opcode-245 bank-move path
// (NetworkBuilder.ts:860-878): resolve both display slots;
// when both resolve to the same tab, set the insert mode
// from the packet mode and rearrange. It reports whether the
// move ran. The 245 decoder and dispatch live in the
// listeners (Worker A); this is the bank half they call.
func BankMoveSlots(p *Player, fromClientSlot, toClientSlot int, insert bool) bool {
	if p == nil || !BankIsOpen(p) {
		return false
	}
	from, ok := BankResolveDisplaySlot(p, fromClientSlot)
	if !ok {
		return false
	}
	to, ok := BankResolveDisplaySlot(p, toClientSlot)
	if !ok || from.Tab != to.Tab {
		return false
	}
	BankSetInsertMode(p, insert)
	BankRearrange(p, p.Bank(from.Tab), from.Slot, to.Slot)
	return true
}

// ---- buttons ------------------------------------------------

// BankHandleButton is Bank.handleButton (Bank.ts:1098-1196),
// the legacy button route: settings interface 32500, the
// tab-select buttons (50070 + 4*tab), and the main bank
// buttons. It reports whether the bank owned the button.
func BankHandleButton(p *Player, button, action int) bool {
	if p == nil {
		return false
	}
	if p.GetInterfaceId() == BankSettingsInterfaceID {
		switch button {
		case 32503:
			SendSound(p, SoundContainerClose)
			BankClose(p)
		case 32512:
			BankOpen(p)
		case 32513:
			BankSetPlaceholders(p, !BankPlaceholders(p))
			p.SendVarp(118, boolToInt(BankPlaceholders(p)))
			state := "disabled"
			if BankPlaceholders(p) {
				state = "enabled"
			}
			p.SendMessage("Placeholders are now " + state + ".")
		}
		return true
	}
	if !BankIsOpen(p) {
		return false
	}
	if p.Status() == PlayerStatusBanking {
		tabSelectStart := 50070
		for bankID := 0; bankID < TotalBankTabs; bankID++ {
			if button == tabSelectStart+(bankID*4) {
				searching := BankSearching(p)
				if searching {
					BankExitSearch(p, false)
				}
				empty := false
				if bankID > 0 {
					empty = BankIsEmpty(p.Bank(bankID))
				}
				if action == 1 {
					// Collapse tab.
					if bankID == 0 {
						return true
					}
					if empty {
						return true
					}
					BankCollapseTab(p, bankID)
				} else {
					if !empty || bankID == 0 {
						BankSetCurrentTab(p, bankID)
						BankOpen(p)
					} else {
						p.SendMessage(bankNewTabMessage)
						if searching {
							BankOpen(p)
						}
					}
				}
				return true
			}
		}
		switch button {
		case 50013:
			// Show menu.
			BankClose(p)
			p.SendInterface(BankSettingsInterfaceID)
		case 5386:
			BankSetNoteWithdrawal(p, true)
		case 5387:
			BankSetNoteWithdrawal(p, false)
		case 8130:
			BankSetInsertMode(p, false)
		case 8131:
			BankSetInsertMode(p, true)
		case 50004:
			BankDepositItems(p, p.Inventory(), false)
		case 50007:
			BankDepositItems(p, p.Equipment(), false)
		case 5384, 50001:
			SendSound(p, SoundContainerClose)
			BankClose(p)
		case 50010:
			if BankSearching(p) {
				BankExitSearch(p, true)
				return true
			}
			p.SetEnteredSyntaxAction(&EnteredSyntaxAction{Execute: func(syntax string) {
				BankSearch(p, syntax)
			}})
			if sender := bankSender; sender != nil {
				sender.SendInputPrompt(p, "What do you wish to search for?")
			}
		}
	}
	return true
}

// ---- widget actions -----------------------------------------

// BankWidgetPacket is the cache-native widget action packet
// (BankWidgetPacket, Bank.ts:27-34): the group/child, the
// button number, the option text, and the slot/item id.
// Slot and ItemID are -1 when the packet carries none.
type BankWidgetPacket struct {
	GroupID   int
	ChildID   int
	ButtonNum int
	Option    string
	Slot      int
	ItemID    int
}

// BankHandleWidgetAction is Bank.handleWidgetAction
// (Bank.ts:877-881): the cache-native widget route, wrapped
// in the aborting-prompt rule (Bank.ts:887-897): another bank
// op with the "Enter amount:" prompt still open runs, then
// the server aborts the prompt — unless the op asked for an
// amount again, replacing it. It reports whether the bank
// owned the packet.
func BankHandleWidgetAction(p *Player, packet BankWidgetPacket) bool {
	return bankAbortingPrompt(p, func() bool { return bankHandleWidget(p, packet) })
}

// bankAbortingPrompt is Bank.abortingPrompt (Bank.ts:887-897).
func bankAbortingPrompt(p *Player, op func() bool) bool {
	if p == nil {
		return false
	}
	pending := p.GetEnteredAmountAction()
	handled := op()
	if handled && pending != nil && p.GetEnteredAmountAction() == pending {
		bankAbortInputPrompt(p)
	}
	return handled
}

// bankAbortInputPrompt aborts an unanswered prompt: the
// session's AbortInputPrompt frame when the session
// implements it (PacketSender.abortInputPrompt,
// PacketSender.ts:465-475), else the player half only
// (AbortEnteredActions: clear + Cancel, no frame).
func bankAbortInputPrompt(p *Player) {
	if p == nil {
		return
	}
	type abortSender interface{ AbortInputPrompt() }
	if sender, ok := p.Session.(abortSender); ok {
		sender.AbortInputPrompt()
		return
	}
	p.AbortEnteredActions()
}

// bankHandleWidget is Bank.handleBankWidget (Bank.ts:899-1032).
func bankHandleWidget(p *Player, packet BankWidgetPacket) bool {
	if p == nil || !BankIsOpen(p) {
		return false
	}
	// Capture (bank search): picking a tab heading switches
	// to that tab and ends the search.
	if packet.GroupID == BankMainInterfaceID && packet.ChildID == BankItemsChild &&
		packet.Slot >= BankTabHeadingFirstSlot && packet.Slot <= BankTabHeadingLastSlot {
		BankViewTab(p, packet.Slot-BankTabHeadingFirstSlot+1)
		BankCloseSearch(p)
		return true
	}
	if packet.GroupID == BankMainInterfaceID && packet.ChildID == BankItemsChild && packet.Slot >= 0 {
		entry, ok := BankResolveDisplaySlot(p, packet.Slot)
		if !ok || (packet.ItemID >= 0 && packet.ItemID != BankDisplayItemID(p, entry.Item)) {
			return true
		}
		if packet.ButtonNum == BankExamineOp {
			bankExamine(p, entry.Item.ID(), entry.Item.Amount())
			return true
		}
		if entry.Item.Amount() == 0 {
			if packet.ButtonNum == 8 || strings.TrimSpace(strings.ToLower(packet.Option)) == "release" {
				BankReleasePlaceholder(p, entry.Tab, entry.Slot)
			}
			return true
		}
		ev := &BankItemOpEvent{
			Player: p, Item: entry.Item, Tab: entry.Tab, Slot: entry.Slot,
			Op: packet.ButtonNum, Option: packet.Option, Handled: false,
		}
		bankEmit(p, BankItemOpEventName, ev)
		if ev.Handled {
			return true
		}
		if BankAsksAmount(BankAmountWithdraw, packet.ButtonNum, packet.Option, BankCustomQuantity(p), BankQuantityMode(p)) {
			itemID := entry.Item.ID()
			tab, slot := entry.Tab, entry.Slot
			BankPromptAmount(p, func(amount int) {
				// The bank may have changed while the
				// prompt was open.
				if item := p.Bank(tab).ItemAt(slot); item.ID() == itemID && item.Amount() > 0 {
					BankWithdraw(p, itemID, slot, amount, tab)
				}
			})
			return true
		}
		amount := BankActionAmount(BankAmountWithdraw, packet.ButtonNum, packet.Option,
			entry.Item.Amount(), BankCustomQuantity(p), BankQuantityMode(p))
		if amount > 0 {
			BankWithdraw(p, entry.Item.ID(), entry.Slot, amount, entry.Tab)
		}
		return true
	}
	// The worn items view lists the backpack: its slot is the
	// inventory slot.
	if packet.GroupID == BankSideInterfaceID && packet.ChildID == BankSideWornChild && packet.Slot >= 0 {
		inv := p.Inventory()
		if inv == nil {
			return true
		}
		item := inv.ItemAt(packet.Slot)
		if item.ID() < 0 || (packet.ItemID >= 0 && packet.ItemID != item.ID()) {
			return true
		}
		if packet.ButtonNum == 1 {
			bankEquipFromSide(p, item.ID(), packet.Slot)
		} else if packet.ButtonNum == 9 {
			BankDeposits(p, item.ID(), packet.Slot, item.Amount())
		} else if packet.ButtonNum == 10 {
			bankExamine(p, item.ID(), item.Amount())
		}
		return true
	}
	if packet.GroupID == BankSideInterfaceID &&
		(packet.ChildID == BankSideItemsChild || packet.ChildID >= 0x8000) && packet.Slot >= 0 {
		inv := p.Inventory()
		if inv == nil {
			return true
		}
		item := inv.ItemAt(packet.Slot)
		if item.ID() < 0 || (packet.ItemID >= 0 && packet.ItemID != item.ID()) {
			return true
		}
		ev := &BankInventoryOpEvent{
			Player: p, Item: item, Slot: packet.Slot,
			Op: packet.ButtonNum, Option: packet.Option, Handled: false,
		}
		bankEmit(p, BankInventoryOpEventName, ev)
		if ev.Handled {
			return true
		}
		if packet.ButtonNum == BankSideEquipOp {
			bankEquipFromSide(p, item.ID(), packet.Slot)
			return true
		}
		if BankAsksAmount(BankAmountDeposit, packet.ButtonNum, packet.Option, BankCustomQuantity(p), BankQuantityMode(p)) {
			itemID := item.ID()
			slot := packet.Slot
			BankPromptAmount(p, func(amount int) {
				if inv := p.Inventory(); inv != nil && inv.ItemAt(slot).ID() == itemID {
					BankDeposits(p, itemID, slot, amount)
				}
			})
			return true
		}
		amount := BankActionAmount(BankAmountDeposit, packet.ButtonNum, packet.Option,
			inv.GetAmount(item.ID()), BankCustomQuantity(p), BankQuantityMode(p))
		if amount > 0 {
			BankDeposits(p, item.ID(), packet.Slot, amount)
		}
		return true
	}
	if packet.GroupID == BankMainInterfaceID && packet.ChildID == BankTabsChild && packet.Slot >= 0 {
		tab := packet.Slot - BankTabButtonSlotOffset
		if tab < 0 || tab > 9 {
			return true
		}
		option := strings.TrimSpace(strings.ToLower(packet.Option))
		if packet.ButtonNum == 6 || strings.Contains(option, "collapse") {
			BankCollapseTab(p, tab)
		} else if packet.ButtonNum == 7 || strings.Contains(option, "placeholder") {
			BankReleasePlaceholders(p, tab)
		} else {
			BankViewTab(p, tab)
			BankCloseSearch(p)
		}
		return true
	}
	// Capture: Search switches to the main tab (the client
	// runs the search itself).
	if packet.GroupID == BankMainInterfaceID && packet.ChildID == BankSearchChild {
		if BankCurrentTab(p) != 0 {
			BankViewTab(p, 0)
		}
		// The search box the client just opened replaced the
		// count prompt; an abort would close it (script 138
		// closes meslayer mode 11). No recording shows Search
		// under the prompt, so its answer is only dropped
		// here.
		p.SetEnteredAmountAction(nil)
		return true
	}
	if packet.GroupID != BankMainInterfaceID {
		return false
	}
	if packet.ChildID >= BankWornSlotFirstChild && packet.ChildID <= BankWornSlotLastChild {
		bankWornSlotAction(p, packet.ChildID, packet.ButtonNum)
		return true
	}
	switch {
	case packet.ChildID == BankFrameChild:
		BankClose(p)
	case packet.ChildID == BankSwapInsertChild:
		BankSetInsertMode(p, !BankInsertMode(p))
		bankSendVarbit(p, 3959, boolToInt(BankInsertMode(p)))
	case packet.ChildID == BankNoteChild:
		BankSetNoteWithdrawal(p, !BankWithdrawAsNote(p))
		bankSendVarbit(p, 3958, boolToInt(BankWithdrawAsNote(p)))
	case packet.ChildID == bankQuantityChildren[BankQuantityXMode] && packet.ButtonNum == 2:
		// Capture: Set custom quantity only asks for the
		// amount; nothing moves.
		BankPromptAmount(p, nil)
	case bankIsQuantityChild(packet.ChildID):
		BankSetQuantityMode(p, bankQuantityChildIndex(packet.ChildID))
		bankSendVarbit(p, 6590, BankQuantityMode(p))
	case packet.ChildID == BankPlaceholderChild:
		BankSetPlaceholders(p, !BankPlaceholders(p))
		bankSendVarbit(p, 3755, boolToInt(BankPlaceholders(p)))
	case packet.ChildID == BankDepositContainersChild:
		bankEmit(p, BankEmptyContainersEventName, BankEmptyContainersEvent{Player: p})
	case packet.ChildID == BankDepositInventoryChild:
		BankDepositItems(p, p.Inventory(), false)
	case packet.ChildID == BankDepositWornChild:
		BankDepositItems(p, p.Equipment(), false)
	default:
		return false
	}
	return true
}

// bankIsQuantityChild reports whether child is one of the
// quantity children (Bank.ts:154).
func bankIsQuantityChild(childID int) bool {
	for _, child := range bankQuantityChildren {
		if child == childID {
			return true
		}
	}
	return false
}

// bankQuantityChildIndex returns the quantity-mode index of a
// quantity child (Bank.ts:1025-1027).
func bankQuantityChildIndex(childID int) int {
	for i, child := range bankQuantityChildren {
		if child == childID {
			return i
		}
	}
	return 0
}

// bankWornSlotAction is Bank.wornSlotAction (Bank.ts:1034-1041):
// op 2 banks just that worn item (as Deposit worn items does
// for all); op 10 examines it; op 1 removes it (the listener's
// unequip route, via BankUnequipHook).
func bankWornSlotAction(p *Player, childID, op int) {
	slot := bankResolveWornSlot(childID)
	if slot < 0 {
		return
	}
	eq := p.Equipment()
	if eq == nil {
		return
	}
	item := eq.ItemAt(slot)
	if item.ID() < 0 || item.Amount() <= 0 {
		return
	}
	if op == BankWornSlotExamineOp {
		bankExamine(p, item.ID(), item.Amount())
	} else if op == BankWornSlotBankOp {
		BankDepositItems(p, p.Equipment(), false, slot)
	} else if op == 1 && BankUnequipHook != nil {
		BankUnequipHook(p, slot)
	}
}

// bankEquipFromSide is Bank.equipFromSide (Bank.ts:874-881):
// the item moves to worn (synth_sound 2238) and the bank
// stays open. The equip itself is the listener's
// EquipPacketListener.equip; the coordinator installs it as
// BankEquipHook (bankstate.go). With no hook the op is
// consumed and nothing is worn. A non-wearable item is
// refused, as TS checks the equipment slot first.
func bankEquipFromSide(p *Player, itemID, slot int) {
	if p == nil {
		return
	}
	if src := p.ItemDefinitions(); src != nil && src.EquipmentSlot(itemID) < 0 {
		return
	}
	if BankEquipHook != nil {
		BankEquipHook(p, itemID, slot)
	}
}

// bankExamine examines an item the player banks, wears, or
// carries (examineItem, ExaminePrice.ts:71-75, via
// game.ExamineItem): its examine text, then the price line.
// The definition source is the player's installed source
// when it carries the shop/examine capability; without it
// the text is empty and no price line sends.
func bankExamine(p *Player, itemID, amount int) {
	if p == nil {
		return
	}
	src, _ := p.ItemDefinitions().(ShopItemDefinitionSource)
	ExamineItem(p, itemID, amount, src)
}
