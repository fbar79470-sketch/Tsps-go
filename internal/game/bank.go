package game

// Bank is the port of the CONTAINER MECHANICS of
// game/model/container/impl/Bank.ts (1,567 lines): one bank tab as
// a growing STACKS container with placeholder support. All shared
// behavior lives on the embedded itemContainer core (the Bank
// overrides of getEmptySlot/getFreeSlots/canTake/isEmpty/sortItems
// dispatch on kind there); this file holds the tab constants, the
// growth entry point, rearrange, and the placeholder-identity
// helpers the container mechanics call.
//
// CAPACITY SEMANTICS. A tab starts with 352 slots
// (INITIAL_TAB_SLOTS, Bank.ts:52) and grows by one slot at a time
// up to MAX_SLOTS (Bank.ts:49-50) as items are added
// (Bank.freeSlot, Bank.ts:246-251); capacity() is the current
// array length (Bank.ts:1346-1348). The player's bank SPACE is a
// separate number — Bank.getCapacity (Bank.ts:200-205), MAX_SLOTS
// unless the bank:capacity plugin event lowers it — enforced
// through roomLeft in the core, counting used slots across tabs
// 0-9 (placeholders included, search tab excluded).
//
// PLACEHOLDERS. An amount-0 item with id >= 0 is a placeholder:
// valid in a bank (getValidItems, ItemContainer.ts:102-116), not
// an empty slot, and kept by the bank sort. Placeholder identity
// has two sources in TS: the cache placeholder id
// (getPlaceholderId) and the bank:placeholder-identity plugin
// event (Bank.ts:105-113). With no plugin system the event term
// is absent, so placeholder families degenerate to identity (see
// PlaceholderFamily/SharesPlaceholder) while the cache term in
// PlaceholderBankID still works through the definition source.
//
// OUT OF SCOPE (explicit gaps, all needing unported systems):
// open/deposit/withdraw/note/quantity/swap-insert/search
// interface flows (Bank.open, withdraw, deposits, switchsItem,
// handleButton, handleDrag, viewTab, and the drag/move/collapse
// helpers), the bank:capacity/bank:full/bank:opened/
// bank:refreshed/bank:placeholder-identity/bank:inventory-op/
// bank:item-op/bank:withdrawn-items/bank:incinerate plugin
// events, MESLAYER scripts, side-ops (sendSideOps), tab layout
// display (layout/displayItemId live in the cmd/server adapter,
// which owns the snapshot encoding), and Bank.withoutLimit (see
// bankRoomLeft in container.go).

const (
	// MaxBankSlots is the most slots any bank tab array can hold
	// (MAX_SLOTS, Bank.ts:49-50): the client's bank holds 1410
	// items.
	MaxBankSlots = 1410
	// TotalBankTabs is the number of bank tabs, search tab
	// included (TOTAL_BANK_TABS, Bank.ts:66).
	TotalBankTabs = 11
	// BankContainerStart is the first container id of the bank
	// tab containers (CONTAINER_START, Bank.ts:67).
	BankContainerStart = 50300
	// BankSearchTabIndex is the search tab: the last tab
	// (BANK_SEARCH_TAB_INDEX, Bank.ts:68).
	BankSearchTabIndex = 10
	// BankInventoryInterfaceID is the bank side-panel inventory
	// interface id (INVENTORY_INTERFACE_ID, Bank.ts:69).
	BankInventoryInterfaceID = 5064
	// BankMainInterfaceID is the bank main interface id
	// (MAIN_INTERFACE_ID, Bank.ts:70).
	BankMainInterfaceID = 12
)

// Bank preference attribute keys (Player.ts:87-91): the player
// attributes backing the bank modes. TS reads them through Player
// methods — withdrawAsNote / insertModeReturn / getBankQuantityMode
// / getBankCustomQuantity (Player.ts:1441-1472) and isPlaceholders
// (Player.ts:1614-1616) — with strict `=== true` checks for the
// booleans and `Number(... ?? 0)` clamping for the numbers. Go
// reads the two keys that have live consumers directly off the
// player attribute store: placeholders in container.go
// (placeholdersEnabled) and insert mode in the switch-slot
// listener. The note-withdrawal and quantity-mode keys have no
// live consumer yet (the bank interface flows are gaps, see the
// file header); the constants exist so every reader spells the
// keys identically when those flows land, and so the entry-time
// defaults stay visible in one place (false, false, 0, 0, false;
// PlayerSave.ts:793-797).
const (
	// BankNoteWithdrawalAttribute is
	// Player.BANK_NOTE_WITHDRAWAL_ATTRIBUTE: "bank:note-withdrawal".
	BankNoteWithdrawalAttribute = "bank:note-withdrawal"
	// BankInsertModeAttribute is Player.BANK_INSERT_MODE_ATTRIBUTE:
	// "bank:insert-mode".
	BankInsertModeAttribute = "bank:insert-mode"
	// BankQuantityModeAttribute is
	// Player.BANK_QUANTITY_MODE_ATTRIBUTE: "bank:quantity-mode".
	BankQuantityModeAttribute = "bank:quantity-mode"
	// BankCustomQuantityAttribute is
	// Player.BANK_CUSTOM_QUANTITY_ATTRIBUTE: "bank:custom-quantity".
	BankCustomQuantityAttribute = "bank:custom-quantity"
	// BankPlaceholdersAttribute is Player.BANK_PLACEHOLDERS_ATTRIBUTE:
	// "bank:placeholders".
	BankPlaceholdersAttribute = "bank:placeholders"
)

// initialBankTabSlots is the slot count a tab starts with; it
// grows, up to MaxBankSlots, as items are added
// (INITIAL_TAB_SLOTS, Bank.ts:51-52).
const initialBankTabSlots = 352

// Bank is one bank tab. Construct with NewBank; NewPlayer leaves
// the 11 tabs lazy and Player.Bank creates them on first use
// (Player.getBank, Player.ts:1480-1488).
type Bank struct {
	*itemContainer
}

// NewBank returns an empty bank tab owned by p (Bank constructor,
// Bank.ts:195-197: super(player, INITIAL_TAB_SLOTS)).
func NewBank(p *Player) *Bank {
	return &Bank{itemContainer: newItemContainer(ContainerBank, p, initialBankTabSlots)}
}

var _ Container = (*Bank)(nil)

// FreeSlot returns an empty slot in this tab, growing the tab by
// one slot when every slot is taken; -1 once the tab is
// MAX_SLOTS long (freeSlot, Bank.ts:246-251). GetEmptySlot calls
// it after the room check; it is exposed because TS exposes it
// and tab-growth callers (end-of-tab moves) need it.
func (b *Bank) FreeSlot() int { return b.bankFreeSlot() }

// Rearrange moves the item in fromSlot to toSlot (Bank.rearrange,
// Bank.ts:1067-1095). In insert mode the item bubbles toward the
// target one swap at a time, shifting the items between; otherwise
// the two slots swap. The container then refreshes.
//
// insertMode is a parameter because the TS condition reads the
// player's insert-mode preference (player.insertModeReturn,
// Player.ts:1452-1454, backed by the bank:insert-mode attribute),
// which the switch-slot listener supplies after reading it from
// the player's attribute store. The TS refresh targets tab 0 when the viewed tab is 0 and no search is
// active, else this tab (Bank.ts:1086-1090); with no viewed-tab
// or search state in Go, this tab always refreshes, which is the
// same packet the adapter sends either way (the full-bank
// snapshot). The reconfigureTabs call for slot-0 moves
// (Bank.ts:1092-1094) is a gap: no tab UI state exists.
func (b *Bank) Rearrange(fromSlot, toSlot int, insertMode bool) {
	if insertMode {
		tempFrom := fromSlot
		for tempFrom != toSlot {
			if tempFrom > toSlot {
				b.Swap(tempFrom, tempFrom-1)
				tempFrom--
			} else {
				b.Swap(tempFrom, tempFrom+1)
				tempFrom++
			}
		}
	} else {
		b.Swap(fromSlot, toSlot)
	}
	b.RefreshItems()
	// reconfigureTabs skipped: no tab UI state exists.
}

// PlaceholderBankID returns the id a withdrawn item leaves as its
// placeholder, or -1 when the item leaves none
// (placeholderBankId, Bank.ts:125-131): the item's own id when
// the definition has a cache placeholder id, else the plugin
// identity's placeholder id when THAT has a cache placeholder id.
// The plugin term is absent in Go (no
// bank:placeholder-identity event), so only the cache term
// applies: id when PlaceholderID(id) >= 0, else -1. An invalid id
// returns -1.
func (b *Bank) PlaceholderBankID(id int) int {
	if id < 0 {
		return -1
	}
	if b.placeholderID(id) >= 0 {
		return id
	}
	return -1
}

// placeholderBankID is the core's access to PlaceholderBankID for
// the deleteItemContainer placeholder branch.
func (c *itemContainer) placeholderBankID(id int) int {
	if id < 0 {
		return -1
	}
	if c.placeholderID(id) >= 0 {
		return id
	}
	return -1
}

// PlaceholderFamily returns the ids sharing a placeholder with
// id (placeholderFamily, Bank.ts:115-117): the plugin identity's
// id set, or [id] when there is none. The plugin identity event
// (placeholderIdentity, Bank.ts:105-113) has no plugins in Go and
// always yields null, so the family is always exactly [id].
// placeholderIdentity itself is not ported: with no event to
// answer it, it could only return null.
func PlaceholderFamily(id int) []int { return []int{id} }

// SharesPlaceholder reports whether a and b share a placeholder
// (sharesPlaceholder, Bank.ts:119-123): true when the ids are
// equal (including -1 == -1, which TS returns before the
// negativity check), false for negative ids, else whether b is in
// a's placeholder family. With identity-only families (see
// PlaceholderFamily) the last case reduces to a == b, which the
// first check already covered — but the structure is kept literal
// so the plugin term has one place to land.
func SharesPlaceholder(a, b int) bool {
	if a == b {
		return true
	}
	if a < 0 || b < 0 {
		return false
	}
	for _, id := range PlaceholderFamily(a) {
		if id == b {
			return true
		}
	}
	return false
}

// sharesPlaceholder is the core's unexported alias, matching the
// lowercase call sites in ItemContainer.ts (getAddableAmount,
// add, Bank.canTake).
func sharesPlaceholder(a, b int) bool { return SharesPlaceholder(a, b) }
