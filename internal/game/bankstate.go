package game

// Bank UI state and preferences: the §4 contract block for
// Phase 46 (PHASE46_PLAN.md §4, "Bank-open state").
//
// SEAM NOTE (Worker A / coordinator). The plan assigns the
// BANKING status field and the bank UI state (current tab,
// searching flag + syntax) to Worker A, in internal/game and
// internal/session. Worker A has now landed (phase 46): the
// real status field lives on Player (status.go: Status /
// SetStatus / SetStatusNone), BankStatus is deleted in favour
// of Player.Status(), and BankClearStatus delegates to
// SetStatusNone. The current-tab / searching / syntax state
// stays in the attribute-store block below — it is the landed
// home for that state, and no second copy exists on Player.
// The remaining wiring is the coordinator's: SetStatusNone
// takes p.SetStatusNone (or BankClearStatus) and
// EndBankSearch takes BankEndSearch, in installInterfaceRuntime
// (cmd/server/interfacewiring.go), where both deps are
// deliberately unset today.
//
// STORAGE. The block is held in the player's attribute store
// under a NUL-prefixed key, lazily allocated on first access
// under the store's own mutex (the Trading precedent,
// trading.go: the instance's lifetime is the player's
// lifetime, the key is never registered persistent, so no save
// snapshot can see it). The pointer form mirrors the
// attribute store itself: the mutex inside is never copied.
//
// PREFERENCES. The mode preferences live in the attribute
// store under the bank.go keys (Player.ts:87-91, 1441-1472,
// 1614-1616), read live with the TS strictness: booleans are
// `=== true`, numbers are Number(... ?? 0) clamped. They are
// persistent attributes (internal/persist/attributes.go), so
// a saved value loads and a fresh player reads the defaults
// (false, false, 0, 0, false; PlayerSave.ts:793-797).

// Bank event names (Bank.ts:55-100): the custom events the
// bank flows emit. The plugin system has no subscribers in
// Go, so the emits are no-ops until content registers
// handlers through Api.OnCustomEvent; the payloads exist so
// a handler can claim an op (Handled) or rewrite a message.
const (
	// BankOpenedEventName is "bank:opened" (Bank.ts:63): once
	// the bank interface has opened (not on a tab change).
	BankOpenedEventName = "bank:opened"
	// BankRefreshedEventName is "bank:refreshed"
	// (Bank.ts:100): after the open bank redraws its items
	// and the side panel's inventory.
	BankRefreshedEventName = "bank:refreshed"
	// BankFullEventName is "bank:full" (Bank.ts:57): when an
	// item doesn't fit; a plugin can change the message sent.
	BankFullEventName = "bank:full"
	// BankCapacityEventName is "bank:capacity" (Bank.ts:55):
	// asked whenever the bank checks its room. No plugin
	// answers it in Go, so the capacity is always MAX_SLOTS
	// (Bank.getCapacity, Bank.ts:200-205).
	BankCapacityEventName = "bank:capacity"
	// BankInventoryOpEventName is "bank:inventory-op"
	// (Bank.ts:90-95): an op on an inventory item in the bank
	// side panel, before it deposits.
	BankInventoryOpEventName = "bank:inventory-op"
	// BankItemOpEventName is "bank:item-op" (Bank.ts:96-100):
	// an op on a banked item, before it withdraws.
	BankItemOpEventName = "bank:item-op"
	// BankWithdrawnItemsEventName is "bank:withdrawn-items"
	// (Bank.ts:99): new inventory item instances after a
	// successful withdrawal, before refresh.
	BankWithdrawnItemsEventName = "bank:withdrawn-items"
	// BankIncinerateEventName is "bank:incinerate"
	// (Bank.ts:176): a bank stack dropped on the incinerator.
	BankIncinerateEventName = "bank:incinerate"
	// BankEmptyContainersEventName is "bank:empty-containers"
	// (Bank.ts:1032): the deposit-containers control; no core
	// behaviour, plugin-owned.
	BankEmptyContainersEventName = "bank:empty-containers"
	// BankPlaceholderIdentityEventName is
	// "bank:placeholder-identity" (Bank.ts:101): the plugin
	// identity term of placeholder families. No plugin
	// answers it in Go, so families are identity-only (see
	// PlaceholderFamily in bank.go).
	BankPlaceholderIdentityEventName = "bank:placeholder-identity"
)

// BankOpenedEvent is the "bank:opened" payload ({ player },
// Bank.ts:63).
type BankOpenedEvent struct {
	Player *Player
}

// BankRefreshedEvent is the "bank:refreshed" payload
// ({ player }, Bank.ts:100).
type BankRefreshedEvent struct {
	Player *Player
}

// BankFullEvent is the "bank:full" payload ({ player,
// message }, Bank.ts:43-48). A handler may rewrite Message;
// the default is "Not enough space in bank."
type BankFullEvent struct {
	Player  *Player
	Message string
}

// BankInventoryOpEvent is the "bank:inventory-op" payload
// ({ player, item, slot, op, option, handled },
// Bank.ts:90-95). Handled is set by a handler whose item
// owns the op (a rune pouch's View or Empty); the deposit
// then does not run. Emit the pointer form so a handler's
// Handled write is visible to the caller.
type BankInventoryOpEvent struct {
	Player  *Player
	Item    Item
	Slot    int
	Op      int
	Option  string
	Handled bool
}

// BankItemOpEvent is the "bank:item-op" payload ({ player,
// item, tab, slot, op, option, handled }, Bank.ts:96-100).
// Handled is set by a handler whose op is not a withdrawal
// (op 9, Configure-Charges, on items with cache param 2257);
// the withdrawal then does not run. Emit the pointer form.
type BankItemOpEvent struct {
	Player  *Player
	Item    Item
	Tab     int
	Slot    int
	Op      int
	Option  string
	Handled bool
}

// BankWithdrawnItemsEvent is the "bank:withdrawn-items"
// payload ({ player, items }, Bank.ts:99): the new inventory
// item instances a withdrawal created, before refresh.
type BankWithdrawnItemsEvent struct {
	Player *Player
	Items  []Item
}

// BankIncinerateEvent is the "bank:incinerate" payload
// ({ player, item, tab, slot, handled }, Bank.ts:176): a
// bank stack dropped on the incinerator. No core consumer
// destroys the stack (out of scope); a handler sets Handled
// when it takes the destroy on.
type BankIncinerateEvent struct {
	Player  *Player
	Item    Item
	Tab     int
	Slot    int
	Handled bool
}

// BankEmptyContainersEvent is the "bank:empty-containers"
// payload ({ player }, Bank.ts:1032): the deposit-containers
// control. No core behaviour; plugin-owned.
type BankEmptyContainersEvent struct {
	Player *Player
}

// bankUIStateKey is the attribute-store key for the per-player
// bank UI block. NUL-prefixed, like Trading's key: content
// can never address it, and it is never persistent.
const bankUIStateKey = "\x00bank-ui-state"

// bankUIState is the per-player bank UI block (§4): the
// current tab and the search flag + syntax. Tab defaults to 0
// (the main tab); the search tab (10) is never a current tab
// outside a search, and setCurrentBankTab clamps to 0..10
// (Player.ts:1424-1438). The open flag used to live here too;
// it is gone: the open state is the Player status field
// (status.go), read through BankIsOpen below.
type bankUIState struct {
	currentTab int
	searching  bool
	syntax     string
}

// bankState returns p's bank UI block, allocating it on first
// use (the Trading accessor precedent). A nil player yields a
// fresh throwaway block: reads degrade to the defaults and
// writes go nowhere, so a bare fixture cannot panic.
func bankState(p *Player) *bankUIState {
	if p == nil {
		return &bankUIState{}
	}
	if v := p.GetAttribute(bankUIStateKey); v != nil {
		if st, ok := v.(*bankUIState); ok && st != nil {
			return st
		}
	}
	st := &bankUIState{}
	p.SetAttribute(bankUIStateKey, st)
	return st
}

// BankIsOpen reports whether the bank interface is open for p
// (Bank.isOpen, Bank.ts:285-288): status BANKING and interface
// id 12. The status term reads the real Player status field
// (status.go): BANKING is set by the bank open and cleared by
// the interface reset, never derived from a second flag.
func BankIsOpen(p *Player) bool {
	if p == nil {
		return false
	}
	return p.Status() == PlayerStatusBanking && p.GetInterfaceId() == BankMainInterfaceID
}

// BankClearStatus clears the bank-open status: the game half
// of the session's SetStatusNone dep (resetInterfaceState,
// PacketSender.ts:876-900), which the coordinator wires in
// installInterfaceRuntime. It delegates to
// Player.SetStatusNone (status.go); the interface id is
// cleared by the session's own reset, and the search state by
// BankEndSearch.
func BankClearStatus(p *Player) {
	if p == nil {
		return
	}
	p.SetStatusNone()
}

// BankCurrentTab returns the tab p is viewing (getCurrentBankTab,
// Player.ts:1424-1429): clamped to 0..10, defaulting to 0.
func BankCurrentTab(p *Player) int {
	if p == nil {
		return 0
	}
	tab := bankState(p).currentTab
	if tab < 0 || tab >= TotalBankTabs {
		tab = 0
		bankState(p).currentTab = 0
	}
	return tab
}

// BankSetCurrentTab sets the tab p is viewing
// (setCurrentBankTab, Player.ts:1431-1438): an out-of-range
// tab reads as 0.
func BankSetCurrentTab(p *Player, tab int) {
	if p == nil {
		return
	}
	if tab < 0 || tab >= TotalBankTabs {
		tab = 0
	}
	bankState(p).currentTab = tab
}

// BankSearching reports whether p is searching the bank
// (isSearchingBank, Player.ts:1499-1500).
func BankSearching(p *Player) bool {
	if p == nil {
		return false
	}
	return bankState(p).searching
}

// BankSetSearching sets the bank-search flag
// (setSearchingBank, Player.ts:1502-1504).
func BankSetSearching(p *Player, searching bool) {
	if p == nil {
		return
	}
	bankState(p).searching = searching
}

// BankSearchSyntax returns p's bank search text
// (getSearchSyntax, Player.ts:1506-1508).
func BankSearchSyntax(p *Player) string {
	if p == nil {
		return ""
	}
	return bankState(p).syntax
}

// BankSetSearchSyntax sets p's bank search text
// (setSearchSyntax, Player.ts:1510-1512). Stored as typed;
// the match lowercases the item name only (see
// BankAddToSearch).
func BankSetSearchSyntax(p *Player, syntax string) {
	if p == nil {
		return
	}
	bankState(p).syntax = syntax
}

// BankEndSearch ends p's bank search: the game half of the
// session's EndBankSearch dep (removeInterfaces,
// PacketSender.ts:1011-1025), which the coordinator wires in
// installInterfaceRuntime. It clears the flag and syntax and
// empties the search tab; it does not re-open a tab — the
// caller (BankExitSearch) does that when the close came from
// the bank itself. The closedInterfaceID argument is the
// outgoing main interface id, unused here as in TS (the
// session passes it through; closeSearch runs regardless).
func BankEndSearch(p *Player, closedInterfaceID int) {
	if p == nil {
		return
	}
	BankSetSearchSyntax(p, "")
	BankSetSearching(p, false)
	if search := p.Bank(BankSearchTabIndex); search != nil {
		search.ResetItems()
	}
}

// BankWithdrawAsNote reports whether withdrawals come out as
// notes (withdrawAsNote, Player.ts:1444-1446): the
// bank:note-withdrawal attribute strictly true.
func BankWithdrawAsNote(p *Player) bool {
	return p != nil && p.GetAttribute(BankNoteWithdrawalAttribute) == true
}

// BankSetNoteWithdrawal sets note withdrawal
// (setNoteWithdrawal, Player.ts:1440-1442).
func BankSetNoteWithdrawal(p *Player, note bool) {
	if p == nil {
		return
	}
	p.SetAttribute(BankNoteWithdrawalAttribute, note)
}

// BankInsertMode reports whether bank moves insert instead of
// swapping (insertModeReturn, Player.ts:1452-1454): the
// bank:insert-mode attribute strictly true.
func BankInsertMode(p *Player) bool {
	return p != nil && p.GetAttribute(BankInsertModeAttribute) == true
}

// BankSetInsertMode sets insert mode (setInsertMode,
// Player.ts:1448-1450).
func BankSetInsertMode(p *Player, insert bool) {
	if p == nil {
		return
	}
	p.SetAttribute(BankInsertModeAttribute, insert)
}

// BankQuantityMode returns the quantity mode, 0..4
// (getBankQuantityMode, Player.ts:1461-1464): Number(attr ??
// 0), truncated and clamped. Mode 0 = 1, 1 = 5, 2 = 10,
// 3 = X, 4 = All.
func BankQuantityMode(p *Player) int {
	if p == nil {
		return 0
	}
	return bankNumberAttribute(p, BankQuantityModeAttribute, 4)
}

// BankSetQuantityMode sets the quantity mode, 0..4
// (setBankQuantityMode, Player.ts:1456-1459): truncated and
// clamped to 0..4.
func BankSetQuantityMode(p *Player, mode int) {
	if p == nil {
		return
	}
	if mode < 0 {
		mode = 0
	}
	if mode > 4 {
		mode = 4
	}
	p.SetAttribute(BankQuantityModeAttribute, mode)
}

// BankCustomQuantity returns the remembered X amount
// (getBankCustomQuantity, Player.ts:1471-1475): Number(attr
// ?? 0), truncated, floored at 0.
func BankCustomQuantity(p *Player) int {
	if p == nil {
		return 0
	}
	return bankNumberAttribute(p, BankCustomQuantityAttribute, -1)
}

// BankSetCustomQuantity sets the remembered X amount
// (setBankCustomQuantity, Player.ts:1466-1469): truncated,
// floored at 0.
func BankSetCustomQuantity(p *Player, amount int) {
	if p == nil {
		return
	}
	if amount < 0 {
		amount = 0
	}
	p.SetAttribute(BankCustomQuantityAttribute, amount)
}

// BankPlaceholders reports whether full withdrawals leave
// placeholders (isPlaceholders, Player.ts:1614-1616): the
// bank:placeholders attribute strictly true.
func BankPlaceholders(p *Player) bool {
	return p != nil && p.GetAttribute(BankPlaceholdersAttribute) == true
}

// BankSetPlaceholders sets the placeholder preference
// (setPlaceholders, Player.ts:1618-1620).
func BankSetPlaceholders(p *Player, placeholders bool) {
	if p == nil {
		return
	}
	p.SetAttribute(BankPlaceholdersAttribute, placeholders)
}

// bankNumberAttribute reads a numeric bank preference:
// Number(attr ?? 0), truncated, floored at 0, and capped at
// max when max >= 0 (the getBankQuantityMode /
// getBankCustomQuantity shape, Player.ts:1461-1475). The
// attribute store holds ints in Go (persisted as JSON
// numbers); a non-int value reads as 0, matching Number()
// on a non-numeric value only in the finite case — a stored
// float is truncated the same way TS truncates it.
func bankNumberAttribute(p *Player, key string, max int) int {
	v := p.GetAttribute(key)
	n := 0
	switch t := v.(type) {
	case int:
		n = t
	case int64:
		n = int(t)
	case float64:
		n = int(t)
	}
	if n < 0 {
		n = 0
	}
	if max >= 0 && n > max {
		n = max
	}
	return n
}

// bankCapacity is Bank.getCapacity (Bank.ts:200-205): the
// player's bank space in slots. The bank:capacity plugin
// event has no subscribers in Go, so the capacity is always
// MAX_SLOTS (1410). The BankSpace capacity system (400 F2P /
// 900 members + unlocks) is out of scope for Phase 46; when
// it lands it answers the event and this function reads it.
func bankCapacity(p *Player) int { return MaxBankSlots }

// bankVarbitSender is the optional SessionPort extension for
// varbit sends (PacketSender.sendVarbit), following the
// CombatStyleSender precedent (weapon_interface.go): the
// session implements SendVarbit, a session-less player drops
// the send.
type bankVarbitSender interface {
	SendVarbit(id, value int)
}

// bankSendVarbit sends one varbit through the player's
// session, or drops it when the player is nil or the session
// does not implement the extension.
func bankSendVarbit(p *Player, id, value int) {
	if p == nil {
		return
	}
	if sender, ok := p.Session.(bankVarbitSender); ok {
		sender.SendVarbit(id, value)
	}
}

// bankDialogueSender returns the player's DialogueSender, or
// nil (the dialogueSender assertion pattern,
// dialogue_entries.go): the frame sends the bank flows need
// (strings, flags ranges, client scripts, interface removal).
func bankDialogueSender(p *Player) DialogueSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(DialogueSender)
	return sender
}

// bankNoteIDSource is the optional note-id capability of an
// item-definition source (getNoteId,
// ItemDefinition.ts:245-247): only Bank.switchsItem reads it,
// so it stays optional like ItemNameSource (itemdef.go) — a
// source without it yields -1, and no item is withdrawable
// as a note. The cmd/server adapter's storeItemDefinitions
// implements it over the Store.
type bankNoteIDSource interface {
	// NoteID returns the note id for id, or -1 when id has
	// no note form.
	NoteID(id int) int
}

// bankNoteID returns the note id for id, or -1 when the
// player's definition source lacks the capability or the
// item has no note form.
func bankNoteID(p *Player, id int) int {
	if p == nil {
		return -1
	}
	src := p.ItemDefinitions()
	if src == nil {
		return -1
	}
	if notes, ok := src.(bankNoteIDSource); ok {
		return notes.NoteID(id)
	}
	return -1
}

// bankItemName returns the display name of id through the
// installed definition source's ItemNameSource capability
// (Player.ItemName): "" with no source or no capability.
func bankItemName(p *Player, id int) string {
	if p == nil {
		return ""
	}
	return p.ItemName(id)
}

// bankEmit emits a bank custom event through the player's
// EmitCustomEvent sink (installed by the coordinator wiring
// in installInterfaceRuntime). A nil sink drops the event.
func bankEmit(p *Player, name string, payload any) {
	if p == nil || p.EmitCustomEvent == nil {
		return
	}
	p.EmitCustomEvent(name, payload)
}

// BankSender is the outbound seam for the bank open sequence
// and the prompt frames (the ShopSender / TradeSender
// precedent): the game layer never imports protocol or
// session, so the cmd/server adapter implements these sends
// over the protocol encoders and the coordinator installs
// the sender with SetBankSender (see installBankSender in
// cmd/server/containerswiring.go). A nil sender drops the
// sends; the bank state changes still land, so a session-less
// player (tests, bots) can open and use the bank without a
// client.
type BankSender interface {
	// SendBankOpen delivers one bank-open frame sequence
	// (Bank.open, Bank.ts:1354-1414): script 917, the main
	// 12 / side 15 mounts with the varp/varbit bundle, the
	// flags ranges, varbit 12393, the "Bank of <world>"
	// string, and the view-container reset when opening.
	SendBankOpen(p *Player, view BankOpenView)
	// SendAmountPrompt sends the "Enter amount:" prompt
	// frame (sendEnterAmountPrompt, PacketSender.ts:446-452:
	// EncodeRunClientScript(108, [title])). The game layer
	// arms the EnteredAmountAction first; this sends only
	// the frame.
	SendAmountPrompt(p *Player, prompt string)
	// SendInputPrompt sends the free-text prompt frame
	// (sendEnterInputPrompt, PacketSender.ts:446-452:
	// EncodeRunClientScript(110, [title, 0])). The game
	// layer arms the EnteredSyntaxAction first; this sends
	// only the frame.
	SendInputPrompt(p *Player, prompt string)
}

// BankOpenView is one bank-open frame sequence, built by
// BankOpen from live state (Bank.ts:1354-1414): the varps and
// varbits the two sub-interface mounts carry, whether the
// bank is opening (as opposed to re-opening on a tab change),
// and whether the view-container reset applies.
type BankOpenView struct {
	// Opening reports whether the bank was closed before
	// this open (Bank.isOpen was false): note withdrawal is
	// forced off, the view-container reset runs, and the
	// open sound + bank:opened event fire only then.
	Opening bool
	// Varps are the mount varps: 548 = 1, 262 = the side
	// wear-ops mask, 4611 = MAX_SLOTS - capacity.
	Varps map[int]int
	// Varbits are the mount varbits: 3755 placeholders,
	// 3958 note, 3959 insert, 3960 custom quantity, 4150
	// current tab, 4170 = 0, 5450 = 1, 6590 quantity mode,
	// 4171-4179 tab sizes.
	Varbits map[int]int
	// ResetViewContainer reports whether the view-container
	// reset applies (opening and varbit 9727 != 0): varbit
	// 9727 -> 0 and varp 4385 -> -1.
	ResetViewContainer bool
}

// bankSender is the installed BankSender, or nil. Set once
// at boot by the coordinator (installBankSender); never
// reassigned afterwards, so no lock guards it — the same
// install-once shape as the sounds network-player source.
var bankSender BankSender

// SetBankSender installs the BankSender the bank flows send
// through. Called once at boot by the coordinator; tests
// install a recording fake and restore nil afterwards.
func SetBankSender(sender BankSender) { bankSender = sender }

// BankEquipHook is the equip seam for the bank side panel
// (Bank.equipFromSide, Bank.ts:874-881, via
// EquipPacketListener.equip): the listener layer owns
// equipping (requirements, two-handed, displace, interface
// preservation), and the game layer cannot call it. The
// coordinator installs the hook over the listener's equip
// when the bank widget routes an equip op; with no hook the
// op is consumed and nothing is worn. It reports whether the
// item was worn.
var BankEquipHook func(p *Player, itemID, slot int) bool

// BankUnequipHook is the unequip seam for the bank worn panel
// (wornSlotAction op 1, Bank.ts:1034-1041, via
// EquipPacketListener.unequip): the listener layer owns
// unequipping. The coordinator installs the hook over the
// listener's unequip; with no hook the op is consumed and
// nothing is removed. It reports whether the item was
// removed to the inventory.
var BankUnequipHook func(p *Player, equipmentSlot int) bool
