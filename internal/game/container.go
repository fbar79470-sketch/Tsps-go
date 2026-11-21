package game

// ItemContainer core: the port of
// game/model/container/ItemContainer.ts (886 lines) plus
// StackType.ts. The TS abstract class becomes the unexported
// itemContainer struct, embedded by the distinct Inventory,
// Equipment, and Bank types; TS instanceof checks become switches
// on the kind field, and TS virtual dispatch (Bank's overrides of
// getEmptySlot/getFreeSlots/canTake/isEmpty/sortItems) becomes
// kind dispatch inside the shared methods, so a call through the
// Container interface always runs the right variant.
//
// RETURN VALUES. TS methods return `this` for chaining. Go
// mutating methods return nothing; callers sequence calls.
// Methods whose TS return value carries information (canTake,
// getAmount, getSlot, getAddableAmount, ...) keep their returns.
//
// VALUE SEMANTICS. TS containers hold Item references and
// getItems() returns the live array. Go containers hold Item
// values; Items() returns deep copies (the TS getCopiedItems
// behavior), ItemAt/Get return deep copies, and SetItem/SetItems
// store copies. There is no live-array accessor: code that in TS
// mutates through getItems()[slot] must read-modify-write through
// ItemAt/SetItem in Go.
//
// CHAT MESSAGES. Several TS paths call player.sendMessage when a
// container cannot take items (add overflow, switchItem bank-full,
// full()). Those messages ARE sent in Go, through
// Player.SendMessage (see message.go): the exact TS texts at the
// exact TS call sites. The one remaining gap is Bank.full's
// bank:full plugin event, which lets a plugin rewrite the
// message: with no plugin system the default message is sent
// (see Full). The container mutations themselves are exact: the
// message never affects state.
//
// NOT PORTED (gaps, with reasons):
//   - forceAdd (ItemContainer.ts:840-862): needs the ground-item
//     manager seam (ItemOnGroundManager.registers) to drop the
//     overflow at the player's feet. The manager exists, but no
//     seam from containers to it does; inventing one is the
//     listener worker's job, not this port's.
//   - getTotalValue (ItemContainer.ts:864-876): needs the item
//     value lookup, which no other ported container code reads;
//     Value is deliberately absent from ItemDefinitionSource.
//   - The Bank search-tab mirror in switchItem
//     (ItemContainer.ts:348-353): needs the searching-bank state,
//     which does not exist in Go (no bank interface state).

// StackType is StackType.ts: the stacking behavior of a container.
type StackType int

const (
	// StackTypeDefault: items do not stack, except noted/stackable
	// ones (inventory, equipment). StackType.ts: DEFAULT.
	StackTypeDefault StackType = iota
	// StackTypeStacks: every item stacks (banks, shops).
	// StackType.ts: STACKS.
	StackTypeStacks
)

// MaxStackAmount (0x7fffffff) is declared in grounditems.go, where
// the ground-item manager already ports MAX_STACK_AMOUNT
// (StackType.ts:6). It is reused here, not redeclared.

// ContainerKind identifies which container a base struct is, for
// the TS instanceof dispatch and for refresh events.
type ContainerKind int

const (
	// ContainerInventory is an Inventory (capacity 28, DEFAULT).
	ContainerInventory ContainerKind = iota
	// ContainerEquipment is an Equipment (capacity 14, DEFAULT).
	ContainerEquipment
	// ContainerBank is one Bank tab (growing, STACKS).
	ContainerBank
	// ContainerWornInventory appears only in refresh events from
	// Equipment.SendWornInventory (the worn inventory 94); no
	// container has this kind. See equipment.go.
	ContainerWornInventory
)

// RefreshEvent is one container refresh delivered to the
// coordinator-installed hook: which container refreshed, whose it
// is, and defensive copies of its slots at refresh time. The
// adapter distinguishes containers by Kind (and, for banks, reads
// the whole bank layout through Player — a bank refresh always
// sends the full snapshot, as in TS).
type RefreshEvent struct {
	Kind   ContainerKind
	Player *Player
	Items  []Item
}

// RefreshHook receives container refreshes. The coordinator
// installs it (see SetRefreshHook); with no hook installed,
// refreshes are silent no-ops. Hooks must not mutate the event
// items — they are copies, but mutating them has no effect on the
// container — and must not call back into the container.
type RefreshHook func(ev RefreshEvent)

// Container is the interface cross-container operations use: the
// Go form of the TS ItemContainer type as switchItem/switchItems/
// deleteItemContainer/moveItems see it. *Inventory, *Equipment,
// and *Bank all satisfy it through the embedded base.
type Container interface {
	Kind() ContainerKind
	Player() *Player
	Capacity() int
	StackType() StackType
	Items() []Item
	ItemAt(slot int) Item
	GetAmount(id int) int
	GetFreeSlots() int
	IsEmpty() bool
	IsFull() bool
	ContainsNumber(id int) bool
	CanTake(item Item) bool
	Full()
	GetAddableAmount(item Item) int
	Add(item Item, refresh bool)
	DeleteItemContainer(item *Item, slot int, refresh bool, to Container)
	RefreshItems()
	SortItems()
}

// itemContainer is the shared core. kind selects the TS subclass
// behavior; player is the owning player (nil only for containers
// built without one, which never happens in production);
// refreshHook is the coordinator-installed hook, nil until
// installed.
type itemContainer struct {
	kind        ContainerKind
	player      *Player
	items       []Item
	refreshHook RefreshHook
}

// newItemContainer builds a core of the given kind with capacity
// empty slots (ItemContainer constructor, ItemContainer.ts:45-58:
// every slot starts as new Item(-1, 0)). The refresh hook is
// inherited from a sibling container that already has one, so a
// bank tab created lazily after the coordinator installed hooks
// (Player.Bank, or a NewBank the save loader hands to SetBank)
// refreshes like its siblings without a second install call.
func newItemContainer(kind ContainerKind, p *Player, capacity int) *itemContainer {
	c := &itemContainer{kind: kind, player: p, items: make([]Item, capacity)}
	for i := range c.items {
		c.items[i] = NewItem(-1, 0)
	}
	c.refreshHook = inheritRefreshHook(p)
	return c
}

// inheritRefreshHook returns the refresh hook already installed on
// one of p's containers, or nil. It checks the inventory, the
// equipment, then the existing bank tabs, in that order; the
// adapter installs the same hook on all of them, so the order is
// unobservable in production.
func inheritRefreshHook(p *Player) RefreshHook {
	if p == nil {
		return nil
	}
	if p.inventory != nil && p.inventory.itemContainer != nil && p.inventory.refreshHook != nil {
		return p.inventory.refreshHook
	}
	if p.equipment != nil && p.equipment.itemContainer != nil && p.equipment.refreshHook != nil {
		return p.equipment.refreshHook
	}
	for _, b := range p.banks {
		if b != nil && b.itemContainer != nil && b.refreshHook != nil {
			return b.refreshHook
		}
	}
	return nil
}

// Kind returns the container kind (the TS instanceof dispatch).
func (c *itemContainer) Kind() ContainerKind { return c.kind }

// Player returns the owning player (getPlayer,
// ItemContainer.ts:68-70).
func (c *itemContainer) Player() *Player { return c.player }

// SetPlayer replaces the owning player (setPlayer,
// ItemContainer.ts:72-75).
func (c *itemContainer) SetPlayer(p *Player) { c.player = p }

// SetRefreshHook installs the refresh hook RefreshItems invokes.
// A nil hook uninstalls. The cmd/server adapter installs one hook
// shared by all of a player's containers; see RefreshHook.
func (c *itemContainer) SetRefreshHook(hook RefreshHook) { c.refreshHook = hook }

// Capacity returns the slot count: 28 for inventory, 14 for
// equipment, and the current array length for a bank tab (which
// grows as items are added — Bank.capacity, Bank.ts:1346-1348).
func (c *itemContainer) Capacity() int {
	if c.kind == ContainerBank {
		return len(c.items)
	}
	switch c.kind {
	case ContainerInventory:
		return InventoryCapacity
	case ContainerEquipment:
		return EquipmentCapacity
	default:
		return len(c.items)
	}
}

// StackType returns the stacking behavior: STACKS for banks,
// DEFAULT otherwise (stackType, Inventory.ts:16-18,
// Equipment.ts:66-68, Bank.ts:1350-1352).
func (c *itemContainer) StackType() StackType {
	if c.kind == ContainerBank {
		return StackTypeStacks
	}
	return StackTypeDefault
}

// defs returns the item-definition source the container reads
// through its player, or nil when none is installed.
func (c *itemContainer) defs() ItemDefinitionSource {
	if c.player == nil {
		return nil
	}
	return c.player.ItemDefinitions()
}

// stackable answers IsStackable through the source, defaulting to
// false with no source (see ItemDefinitionSource).
func (c *itemContainer) stackable(id int) bool {
	if src := c.defs(); src != nil {
		return src.IsStackable(id)
	}
	return false
}

// unNote answers UnNote through the source, defaulting to the id
// itself with no source (see ItemDefinitionSource).
func (c *itemContainer) unNote(id int) int {
	if src := c.defs(); src != nil {
		return src.UnNote(id)
	}
	return id
}

// placeholderID answers PlaceholderID through the source,
// defaulting to -1 with no source (see ItemDefinitionSource).
func (c *itemContainer) placeholderID(id int) int {
	if src := c.defs(); src != nil {
		return src.PlaceholderID(id)
	}
	return -1
}

// isBank reports whether this container is a bank tab (the
// `instanceof Bank` checks in ItemContainer.ts).
func (c *itemContainer) isBank() bool { return c.kind == ContainerBank }

// Items returns defensive copies of all slots (getCopiedItems,
// ItemContainer.ts:94-100). This is the ONLY whole-container
// accessor: TS getItems returns the live array, which Go value
// semantics cannot offer without aliasing. The coordinator's save
// snapshot reads Items().
func (c *itemContainer) Items() []Item {
	out := make([]Item, len(c.items))
	for i := range c.items {
		out[i] = c.items[i].Clone()
	}
	return out
}

// GetItemIDs returns the id of every slot (getItemIdsArray,
// ItemContainer.ts:81-87).
func (c *itemContainer) GetItemIDs() []int {
	ids := make([]int, len(c.items))
	for i := range c.items {
		ids[i] = c.items[i].ID()
	}
	return ids
}

// SetItems replaces the container contents (setItems,
// ItemContainer.ts:89-92). TS assigns the array wholesale with no
// length check — callers (save hydration) always pass an
// exactly-sized array, padded with Item(-1, 0) — and aliases it.
// Go copies every item (deep meta clone) so the caller's slice
// can never mutate the container, and normalizes the length:
// inventory/equipment pad with empty items or truncate to their
// fixed capacity, while a bank tab takes the array's length as
// its new capacity (Bank.capacity is the array length). A nil or
// short inventory/equipment array thus yields a full container of
// empties rather than the out-of-range reads TS would suffer.
func (c *itemContainer) SetItems(items []Item) {
	length := len(items)
	if !c.isBank() {
		length = c.Capacity()
	}
	next := make([]Item, length)
	for i := range next {
		if i < len(items) {
			next[i] = items[i].Clone()
		} else {
			next[i] = NewItem(-1, 0)
		}
	}
	c.items = next
}

// GetValidItems returns copies of the valid stacks: id >= 0 and
// amount > 0 — except in a bank, where an amount-0 item is a
// placeholder and is valid too (getValidItems,
// ItemContainer.ts:102-116). TS returns the live Item references;
// Go returns copies (see Items).
func (c *itemContainer) GetValidItems() []Item {
	var valid []Item
	for i := range c.items {
		item := &c.items[i]
		if item.ID() >= 0 && (item.Amount() > 0 || (c.isBank() && item.Amount() == 0)) {
			valid = append(valid, item.Clone())
		}
	}
	return valid
}

// GetValidItemsArray returns the valid stacks as an array
// (getValidItemsArray, ItemContainer.ts:118-125). In Go it is the
// same as GetValidItems: TS copies the list into a fresh array,
// and GetValidItems already returns a fresh slice of copies.
func (c *itemContainer) GetValidItemsArray() []Item { return c.GetValidItems() }

// CopyValidItemsArray returns cloned valid stacks
// (copyValidItemsArray, ItemContainer.ts:127-134). In Go it is the
// same as GetValidItems, which already clones.
func (c *itemContainer) CopyValidItemsArray() []Item { return c.GetValidItems() }

// SetItem stores a copy of item in slot (setItem,
// ItemContainer.ts:136-139). An out-of-range slot is ignored:
// TS would extend the array or create a hole, which would corrupt
// the fixed capacities Go enforces.
func (c *itemContainer) SetItem(slot int, item Item) {
	if slot < 0 || slot >= len(c.items) {
		return
	}
	c.items[slot] = item.Clone()
}

// Set stores a copy of item in slot (set, ItemContainer.ts:785-787).
// It is identical to SetItem, as in TS.
func (c *itemContainer) Set(slot int, item Item) { c.SetItem(slot, item) }

// Get returns a copy of the item in slot (get,
// ItemContainer.ts:789-791). An out-of-range slot returns the
// empty item; TS returns undefined, which Go cannot express for a
// value type.
func (c *itemContainer) Get(slot int) Item { return c.ItemAt(slot) }

// ForSlot returns a copy of the item in slot (forSlot,
// ItemContainer.ts:294-296). Same out-of-range behavior as Get.
func (c *itemContainer) ForSlot(slot int) Item { return c.ItemAt(slot) }

// ItemAt returns a copy of the item in slot, or the empty item
// for an out-of-range slot. It is the dictated slot accessor; Get
// and ForSlot are the TS-named aliases.
func (c *itemContainer) ItemAt(slot int) Item {
	if slot < 0 || slot >= len(c.items) {
		return NewItem(-1, 0)
	}
	return c.items[slot].Clone()
}

// IsSlotOccupied reports whether slot holds a valid stack
// (isSlotOccupied, ItemContainer.ts:141-147). An out-of-range slot
// is unoccupied, as in TS (the undefined slot fails the null
// check).
func (c *itemContainer) IsSlotOccupied(slot int) bool {
	if slot < 0 || slot >= len(c.items) {
		return false
	}
	return c.items[slot].IsValid()
}

// IsSlotFree reports whether slot holds no item (isSlotFree,
// ItemContainer.ts:793-795: no item, or id -1). An out-of-range
// slot reads free, matching TS (the undefined slot is falsy).
func (c *itemContainer) IsSlotFree(slot int) bool {
	if slot < 0 || slot >= len(c.items) {
		return true
	}
	return c.items[slot].ID() == -1
}

// Swap exchanges the items in fromSlot and toSlot (swap,
// ItemContainer.ts:149-157). When the source slot is empty or
// out of range, nothing happens — including when the source holds
// an amount-0 placeholder, whose id is >= 0: TS swaps it (the
// guard tests only id < 0), and so does Go. An out-of-range
// destination is ignored; TS would store a hole there.
func (c *itemContainer) Swap(fromSlot, toSlot int) {
	if fromSlot < 0 || fromSlot >= len(c.items) || toSlot < 0 || toSlot >= len(c.items) {
		return
	}
	if c.items[fromSlot].ID() < 0 {
		return
	}
	c.items[fromSlot], c.items[toSlot] = c.items[toSlot], c.items[fromSlot]
}

// ShiftSwap is the TS method of the same name
// (ItemContainer.ts:159-165), ported exactly: it validates the
// source slot and then returns without swapping anything. The
// upstream method is a stub — the shift behavior was never
// implemented there — and Go does not invent it.
func (c *itemContainer) ShiftSwap(fromSlot, toSlot int) {
	if fromSlot < 0 || fromSlot >= len(c.items) {
		return
	}
	if c.items[fromSlot].ID() < 0 {
		return
	}
}

// CanTake reports whether item fits: a free slot exists, or a
// stack of it can be joined (canTake, ItemContainer.ts:168-171).
// A bank tab uses the Bank override (Bank.ts:262-269): a full bank
// still takes an item it holds as a placeholder or a live stack
// with matching meta, or a member of a placeholder family.
func (c *itemContainer) CanTake(item Item) bool {
	if c.isBank() {
		return c.bankCanTake(item)
	}
	return c.GetFreeSlots() > 0 || (c.ContainsNumber(item.ID()) && c.stackable(item.ID()))
}

// bankCanTake is Bank.canTake (Bank.ts:262-269). The id is
// un-noted first (noted items never sit in a bank); a held stack
// accepts the item when it is the same id with amount 0 (a
// placeholder) or matching meta, or when it is an amount-0
// placeholder sharing a placeholder family with the id.
func (c *itemContainer) bankCanTake(item Item) bool {
	if c.GetFreeSlots() > 0 {
		return true
	}
	id := c.unNote(item.ID())
	meta := item.metaFingerprint()
	for i := range c.items {
		held := &c.items[i]
		if held.ID() == id && (held.Amount() == 0 || held.metaFingerprint() == meta) {
			return true
		}
		if held.Amount() == 0 && sharesPlaceholder(held.ID(), id) {
			return true
		}
	}
	return false
}

// GetFreeSlots counts the empty slots: slots with id -1
// (getFreeSlots, ItemContainer.ts:173-181). A bank tab uses the
// Bank override (Bank.ts:257-259): the array's free slots plus
// the growth headroom up to MAX_SLOTS, capped by the room the
// player's bank space still allows.
func (c *itemContainer) GetFreeSlots() int {
	free := 0
	for i := range c.items {
		if c.items[i].ID() == -1 {
			free++
		}
	}
	if !c.isBank() {
		return free
	}
	room := free + MaxBankSlots - len(c.items)
	if left := c.bankRoomLeft(); left < room {
		room = left
	}
	return room
}

// IsFull reports whether no empty slot exists (isFull,
// ItemContainer.ts:183-185). For a bank tab this consults the
// Bank getEmptySlot override, including growth and room.
func (c *itemContainer) IsFull() bool { return c.GetEmptySlot() == -1 }

// IsEmpty reports whether every slot is free (isEmpty,
// ItemContainer.ts:187-189). A bank tab uses the Bank override
// (Bank.ts:271-273): empty means no slot holds an id at all —
// placeholders included — because the Bank free-slot count
// includes growth headroom and would make the base formula wrong.
func (c *itemContainer) IsEmpty() bool {
	if c.isBank() {
		for i := range c.items {
			if c.items[i].ID() >= 0 {
				return false
			}
		}
		return true
	}
	return c.GetFreeSlots() == c.Capacity()
}

// ContainsNumber reports whether any slot holds id, regardless of
// amount (containsNumber, ItemContainer.ts:191-198). Note the
// amount-blindness: an amount-0 placeholder contains its id.
func (c *itemContainer) ContainsNumber(id int) bool {
	for i := range c.items {
		if c.items[i].ID() == id {
			return true
		}
	}
	return false
}

// Contains reports whether any slot holds id (contains,
// ItemContainer.ts:763-770). It is identical to ContainsNumber,
// as in TS.
func (c *itemContainer) Contains(id int) bool { return c.ContainsNumber(id) }

// ContainsItem reports whether the container holds at least
// item's amount of its id (containsItem, ItemContainer.ts:200-202).
func (c *itemContainer) ContainsItem(item Item) bool {
	return c.GetAmount(item.ID()) >= item.Amount()
}

// ContainsArray reports whether every id in items is present
// (containsArray, ItemContainer.ts:204-219). An empty array
// reports false; presence is by id only (ContainsNumber), not by
// amount.
func (c *itemContainer) ContainsArray(items []Item) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !c.ContainsNumber(item.ID()) {
			return false
		}
	}
	return true
}

// ContainsAnyIDs reports whether any of the ids is present
// (containsAnyIds, ItemContainer.ts:221-236). An empty id list,
// or an empty container, reports false; -1 ids are skipped.
func (c *itemContainer) ContainsAnyIDs(ids []int) bool {
	if len(ids) == 0 || c.IsEmpty() {
		return false
	}
	for _, id := range ids {
		if id == -1 {
			continue
		}
		if c.ContainsNumber(id) {
			return true
		}
	}
	return false
}

// ContainsAllAny reports whether every id is present
// (containsAllAny, ItemContainer.ts:771-773).
func (c *itemContainer) ContainsAllAny(ids []int) bool {
	for _, id := range ids {
		if !c.ContainsNumber(id) {
			return false
		}
	}
	return true
}

// ContainsAllItem reports whether every item's id is present
// (containsAllItem, ItemContainer.ts:775-779). Presence is by id
// only, as in TS.
func (c *itemContainer) ContainsAllItem(items []Item) bool {
	for _, item := range items {
		if !c.ContainsNumber(item.ID()) {
			return false
		}
	}
	return true
}

// ContainsAny reports whether any id is present (containsAny,
// ItemContainer.ts:781-783). Unlike ContainsAnyIDs it has no
// empty-container fast path and does not skip -1.
func (c *itemContainer) ContainsAny(ids []int) bool {
	for _, id := range ids {
		if c.ContainsNumber(id) {
			return true
		}
	}
	return false
}

// GetEmptySlot returns the first empty slot, or -1
// (getEmptySlot, ItemContainer.ts:238-249): a slot with id < 0,
// or with amount <= 0 in a non-bank container (a bank placeholder
// is not an empty slot). A bank tab uses the Bank override
// (Bank.ts:253-255): when the player's bank space allows room, an
// empty slot is found or the tab grows by one.
func (c *itemContainer) GetEmptySlot() int {
	if c.isBank() {
		if c.bankRoomLeft() <= 0 {
			return -1
		}
		return c.bankFreeSlot()
	}
	return c.baseEmptySlot()
}

// baseEmptySlot is the ItemContainer.getEmptySlot scan
// (ItemContainer.ts:238-249), shared by the base path and the
// bank growth path (Bank.freeSlot calls super.getEmptySlot,
// Bank.ts:246-251).
func (c *itemContainer) baseEmptySlot() int {
	for i := 0; i < c.Capacity() && i < len(c.items); i++ {
		if c.items[i].ID() < 0 || (c.items[i].Amount() <= 0 && !c.isBank()) {
			return i
		}
	}
	return -1
}

// bankFreeSlot is Bank.freeSlot (Bank.ts:246-251): the base empty
// slot when one exists, otherwise one new slot appended to the
// tab — up to MAX_SLOTS, past which it returns -1.
func (c *itemContainer) bankFreeSlot() int {
	if slot := c.baseEmptySlot(); slot >= 0 || len(c.items) >= MaxBankSlots {
		return slot
	}
	c.items = append(c.items, NewItem(-1, 0))
	return len(c.items) - 1
}

// bankRoomLeft is Bank.roomLeft (Bank.ts:237-243): the slots the
// player's bank space still allows. With no player, or for the
// search tab (tab 10), the capacity does not apply and every slot
// is open (TS Infinity; Go returns MaxBankSlots, which the
// min() in GetFreeSlots cannot distinguish from infinity — the
// array can never hold more than MaxBankSlots slots). Otherwise
// it is the player's free bank space: capacity minus slots used
// across tabs 0-9. The capacity plugin event (bank:capacity) has
// no plugins in Go, so the capacity is always MAX_SLOTS
// (Bank.getCapacity, Bank.ts:200-205, with the event unhandled).
//
// GAP: Bank.withoutLimit (Bank.ts:226-234), which lifts the
// capacity while items move between tabs or a save loads, is not
// ported — it needs per-player state no Go field holds. The load
// path does not need it: SetItems replaces the array directly,
// bypassing capacity exactly as a withoutLimit load does.
func (c *itemContainer) bankRoomLeft() int {
	p := c.player
	if p == nil {
		return MaxBankSlots
	}
	if search := p.Bank(BankSearchTabIndex); search != nil && search.itemContainer == c {
		return MaxBankSlots
	}
	used := 0
	for tab := 0; tab < TotalBankTabs; tab++ {
		if tab == BankSearchTabIndex {
			continue
		}
		if bank := p.Bank(tab); bank != nil {
			for i := range bank.items {
				if bank.items[i].ID() >= 0 {
					used++
				}
			}
		}
	}
	if free := MaxBankSlots - used; free > 0 {
		return free
	}
	return 0
}

// GetSlot returns the id in slotId, or -1 when the slot holds no
// valid stack (getSlot, ItemContainer.ts:251-257).
//
// DELIBERATE HARDENING: TS guards with `items.length < slotId`,
// which lets slotId == length through and then throws reading the
// undefined slot. Go returns -1 for every out-of-range slot,
// including negative ones.
func (c *itemContainer) GetSlot(slotID int) int {
	if slotID < 0 || slotID >= len(c.items) || !c.items[slotID].IsValid() {
		return -1
	}
	return c.items[slotID].ID()
}

// GetSlotForItemID returns the first slot holding id with amount
// > 0 — or, in a bank, amount == 0 (a placeholder)
// (getSlotForItemId, ItemContainer.ts:259-271). -1 when absent.
func (c *itemContainer) GetSlotForItemID(id int) int {
	for i := 0; i < c.Capacity() && i < len(c.items); i++ {
		if c.items[i].ID() == id && (c.items[i].Amount() > 0 || (c.isBank() && c.items[i].Amount() == 0)) {
			return i
		}
	}
	return -1
}

// GetAmount totals the amount of id across all slots (getAmount,
// ItemContainer.ts:273-281).
func (c *itemContainer) GetAmount(id int) int {
	total := 0
	for i := range c.items {
		if c.items[i].ID() == id {
			total += c.items[i].Amount()
		}
	}
	return total
}

// GetAmountForSlot returns the amount in slot (getAmountForSlot,
// ItemContainer.ts:283-285). An out-of-range slot returns 0; TS
// throws reading the undefined slot.
func (c *itemContainer) GetAmountForSlot(slot int) int {
	if slot < 0 || slot >= len(c.items) {
		return 0
	}
	return c.items[slot].Amount()
}

// GetByID returns a copy of the first item with id, and whether
// one exists (getById, ItemContainer.ts:751-761, which returns the
// live item or null).
func (c *itemContainer) GetByID(id int) (Item, bool) {
	for i := range c.items {
		if c.items[i].ID() == id {
			return c.items[i].Clone(), true
		}
	}
	return NewItem(-1, 0), false
}

// ResetItems empties every slot (resetItems,
// ItemContainer.ts:287-292).
func (c *itemContainer) ResetItems() {
	for i := range c.items {
		c.items[i] = NewItem(-1, 0)
	}
}

// ToSafeArray returns copies of all slots (toSafeArray,
// ItemContainer.ts:797-799, which filters out nulls — Go slots
// are never null, so this is Items()).
func (c *itemContainer) ToSafeArray() []Item { return c.Items() }

// HasAtSlot reports whether slot holds itemID (hasAts,
// ItemContainer.ts:878-881). An out-of-range slot reports false.
func (c *itemContainer) HasAtSlot(slot, itemID int) bool {
	if slot < 0 || slot >= len(c.items) {
		return false
	}
	return c.items[slot].ID() == itemID
}

// HasAt reports whether slot exists (hasAt,
// ItemContainer.ts:883-885: in range and non-null; Go slots are
// never null).
func (c *itemContainer) HasAt(slot int) bool { return slot >= 0 && slot < len(c.items) }

// SwitchItem moves item from slot in this container to container
// to (switchItem, ItemContainer.ts:298-356). The item must sit in
// slot; the destination must take it (otherwise to.Full() runs);
// a deposit into a bank un-notes the item and clamps the amount
// to what the bank can accept; the source slot is deleted, the
// item added to the destination, the source optionally sorted
// when the id is gone, and both containers optionally refreshed.
//
// item is a pointer because TS mutates the passed Item (the
// bank-deposit amount clamp, the un-noting, and the delete clamp
// in deleteItemContainer); the mutations are visible to the
// caller, as in TS. A nil item or destination, or an out-of-range
// slot, is a no-op.
//
// GAPS: the bank-search mirror update (ItemContainer.ts:348-353)
// is skipped — no searching-bank state exists. The "cannot
// deposit that entire amount" message (ItemContainer.ts:322-324)
// IS sent, through Player.SendMessage.
func (c *itemContainer) SwitchItem(to Container, item *Item, sort bool, slot int, refresh bool) {
	if to == nil || item == nil || slot < 0 || slot >= len(c.items) {
		return
	}
	if c.items[slot].ID() != item.ID() {
		return
	}
	if !to.CanTake(*item) {
		to.Full()
		return
	}
	if (c.kind == ContainerInventory || c.kind == ContainerEquipment) && to.Kind() == ContainerBank {
		bankItem := item.Clone()
		bankItem.SetID(c.unNote(item.ID()))
		item.SetAmount(to.GetAddableAmount(bankItem))
		if item.Amount() <= 0 {
			// ItemContainer.ts:322-324: the source
			// container's player is told the whole amount
			// cannot be deposited.
			c.player.SendMessage("You cannot deposit that entire amount into your bank.")
			return
		}
	}
	c.DeleteItemContainer(item, slot, refresh, to)
	// Noted items should not be in bank. Un-note if it's noted.
	// The unnoted id comes from the definition: it is not always
	// the noted id minus one (ItemContainer.ts:331-335).
	if to.Kind() == ContainerBank {
		item.SetID(c.unNote(item.ID()))
	}
	to.Add(*item, refresh)
	if sort && c.GetAmount(item.ID()) <= 0 {
		c.SortItems()
	}
	if refresh {
		c.RefreshItems()
		to.RefreshItems()
	}
	// Bank-search mirror update skipped: no searching-bank state.
}

// SwitchItems moves item — from wherever it sits in this
// container — to container to (switchItems,
// ItemContainer.ts:358-407). It differs from SwitchItem in having
// no slot: the amount is clamped to what this container holds and
// the deletion spans slots (deleteBoolean). item is a pointer for
// the same mutation reason as SwitchItem. The bank-deposit clamp
// sends the same "cannot deposit that entire amount" message as
// SwitchItem (ItemContainer.ts:383-385).
func (c *itemContainer) SwitchItems(to Container, item *Item, sort bool, refresh bool) {
	if to == nil || item == nil {
		return
	}
	if !to.CanTake(*item) {
		to.Full()
		return
	}
	properAmount := c.GetAmount(item.ID())
	if item.Amount() > properAmount {
		item.SetAmount(properAmount)
	}
	if item.Amount() <= 0 {
		return
	}
	if to.Kind() == ContainerBank {
		bankItem := item.Clone()
		bankItem.SetID(c.unNote(item.ID()))
		item.SetAmount(to.GetAddableAmount(bankItem))
		if item.Amount() <= 0 {
			// ItemContainer.ts:383-385: as in SwitchItem.
			c.player.SendMessage("You cannot deposit that entire amount into your bank.")
			return
		}
	}
	c.DeleteBoolean(*item, refresh)
	// Noted items should not be in bank (ItemContainer.ts:392-395).
	if to.Kind() == ContainerBank {
		item.SetID(c.unNote(item.ID()))
	}
	to.Add(*item, refresh)
	if sort && c.GetAmount(item.ID()) <= 0 {
		c.SortItems()
	}
	if refresh {
		c.RefreshItems()
		to.RefreshItems()
	}
}

// FullBoolean reports whether the container is full for itemID:
// no free slot, and no stack of it to join (fullBoolean,
// ItemContainer.ts:412-420).
func (c *itemContainer) FullBoolean(itemID int) bool {
	return c.GetFreeSlots() <= 0 && !(c.ContainsNumber(itemID) && c.stackable(itemID))
}

// AddItems adds every valid item in items (addItems,
// ItemContainer.ts:422-436): id >= 0 and amount > 0, except in a
// bank, where amount-0 placeholders are added too. A nil slice
// adds nothing.
func (c *itemContainer) AddItems(items []Item, refresh bool) {
	for _, item := range items {
		if item.ID() >= 0 && (item.Amount() > 0 || (item.Amount() == 0 && c.isBank())) {
			c.Add(item, refresh)
		}
	}
}

// SortItems compacts the container toward the front (sortItems,
// ItemContainer.ts:438-454): a bubble pass that swaps each empty
// slot with the next slot, repeated capacity times, so items
// keep their order and empties sink to the end. A bank tab uses
// the Bank override (Bank.ts:276-283): a stable partition, filled
// slots (placeholders included) first, empties last, in place.
func (c *itemContainer) SortItems() {
	if c.isBank() {
		c.bankSortItems()
		return
	}
	for k := 0; k < c.Capacity(); k++ {
		for i := 0; i < c.Capacity()-1; i++ {
			if i >= len(c.items) || i+1 >= len(c.items) {
				continue
			}
			if c.items[i].ID() < 0 || (c.items[i].Amount() <= 0 && !c.isBank()) {
				c.Swap(i+1, i)
			}
		}
	}
}

// bankSortItems is Bank.sortItems (Bank.ts:276-283): filled slots
// (id >= 0, placeholders included) keep their order at the front,
// empty slots move to the end, written back in place.
func (c *itemContainer) bankSortItems() {
	var filled, empty []Item
	for i := range c.items {
		if c.items[i].ID() >= 0 {
			filled = append(filled, c.items[i])
		} else {
			empty = append(empty, c.items[i])
		}
	}
	sorted := append(filled, empty...)
	for slot := range c.items {
		c.items[slot] = sorted[slot]
	}
}

// AddItem adds item with a refresh (addItem,
// ItemContainer.ts:462-464).
func (c *itemContainer) AddItem(item Item) { c.Add(item, true) }

// Adds adds amount of id with a refresh (adds,
// ItemContainer.ts:474-476).
func (c *itemContainer) Adds(id, amount int) { c.AddItem(NewItem(id, amount)) }

// GetAddableAmount reports how much of item Add can accept,
// without changing the item or the container (getAddableAmount,
// ItemContainer.ts:479-495). Stackable items (or any item in a
// STACKS container) join an existing stack with matching meta up
// to MAX_STACK_AMOUNT, fill a matching bank placeholder or
// placeholder-family slot, or take a free slot; non-stackable
// items take one free slot each.
func (c *itemContainer) GetAddableAmount(item Item) int {
	requested := item.Amount()
	if requested < 0 {
		requested = 0
	}
	if item.ID() < 0 || requested <= 0 {
		return 0
	}
	if c.stackable(item.ID()) || c.StackType() == StackTypeStacks {
		meta := item.metaFingerprint()
		for i := range c.items {
			held := &c.items[i]
			if held.ID() == item.ID() && (!c.isBank() || held.Amount() > 0) && held.metaFingerprint() == meta {
				room := MaxStackAmount - held.Amount()
				if room < 0 {
					room = 0
				}
				if requested < room {
					return requested
				}
				return room
			}
		}
		if c.isBank() {
			for i := range c.items {
				if c.items[i].Amount() == 0 && sharesPlaceholder(c.items[i].ID(), item.ID()) {
					if requested < MaxStackAmount {
						return requested
					}
					return MaxStackAmount
				}
			}
		}
		if c.GetFreeSlots() > 0 {
			if requested < MaxStackAmount {
				return requested
			}
			return MaxStackAmount
		}
		return 0
	}
	if requested < c.GetFreeSlots() {
		return requested
	}
	return c.GetFreeSlots()
}

// Add adds item to the container (add, ItemContainer.ts:497-583).
//
// Stacking: when the definition says the item stacks, or the
// container is STACKS, the item joins an existing stack of the
// same id with identical meta (JSON-compared), fills a matching
// empty bank placeholder slot (or a placeholder-family slot),
// or takes an empty slot; the total caps at MAX_STACK_AMOUNT and
// the excess is silently lost. Refilling an empty slot stores a
// fresh copy of the item (TS replaces the slot's Item identity so
// delayed actions holding the removed stack's reference cannot
// observe the new contents); merging into a live stack keeps the
// slot's identity and overwrites its id and meta from the added
// item. Otherwise the item fans out one per empty slot.
//
// Overflow: when no slot can take (part of) the item, TS sends
// "You couldn't hold all those items." — sent here, through
// Player.SendMessage — refreshes when asked, and returns,
// keeping whatever fit. A nil player in the stackable branch
// skips the message in TS (the null check at
// ItemContainer.ts:539); SendMessage is nil-safe, so the same
// skip holds here. The non-stackable branch has no such check
// and would throw in TS, which Go does not reproduce: a
// nil-player container drops the message instead of panicking.
//
// The refresh flag threads through exactly as in TS: a refresh
// runs after a successful add, and also after a failed add that
// found no slot.
func (c *itemContainer) Add(item Item, refresh bool) {
	if item.ID() < 0 || (item.Amount() <= 0 && !c.isBank()) {
		return
	}
	if c.stackable(item.ID()) || c.StackType() == StackTypeStacks {
		slot := -1
		emptyBankSlot := -1
		familyBankSlot := -1
		for i := 0; i < c.Capacity() && i < len(c.items); i++ {
			if c.isBank() && c.items[i].Amount() == 0 {
				if c.items[i].ID() == item.ID() {
					if emptyBankSlot == -1 {
						emptyBankSlot = i
					}
				} else if familyBankSlot == -1 && sharesPlaceholder(c.items[i].ID(), item.ID()) {
					familyBankSlot = i
				}
				continue
			}
			if c.items[i].ID() != item.ID() {
				continue
			}
			if c.items[i].metaFingerprint() == item.metaFingerprint() {
				slot = i
				break
			}
		}
		if slot == -1 {
			switch {
			case emptyBankSlot >= 0:
				slot = emptyBankSlot
			case familyBankSlot >= 0:
				slot = familyBankSlot
			default:
				slot = c.GetEmptySlot()
			}
		}
		if slot == -1 {
			// ItemContainer.ts:539-540: the player is told
			// the items would not all fit.
			c.player.SendMessage("You couldn't hold all those items.")
			if refresh {
				c.RefreshItems()
			}
			return
		}
		// Refilling an empty slot creates new contents. Do not
		// revive its old Item identity: delayed actions may still
		// retain that removed stack reference. Ordinary merges
		// into a nonempty stack keep their existing identity
		// (ItemContainer.ts:547-549).
		if c.items[slot].ID() < 0 || c.items[slot].Amount() <= 0 {
			fresh := item.Clone()
			fresh.SetAmount(0)
			c.items[slot] = fresh
		}
		totalAmount := c.items[slot].Amount() + item.Amount()
		c.items[slot].SetID(item.ID())
		c.items[slot].SetMeta(item.Meta())
		if totalAmount > MaxStackAmount {
			c.items[slot].SetAmount(MaxStackAmount)
		} else {
			c.items[slot].SetAmount(c.items[slot].Amount() + item.Amount())
		}
	} else {
		amount := item.Amount()
		for amount > 0 {
			slot := c.GetEmptySlot()
			if slot == -1 {
				// ItemContainer.ts:567-568: as in the
				// stackable branch.
				c.player.SendMessage("You couldn't hold all those items.")
				if refresh {
					c.RefreshItems()
				}
				return
			}
			one := item.Clone()
			one.SetAmount(1)
			c.items[slot] = one
			amount--
		}
	}
	if refresh {
		c.RefreshItems()
	}
}

// Deletes deletes item's amount of its id, with a refresh
// (deletes, ItemContainer.ts:585-587).
func (c *itemContainer) Deletes(item Item) { c.DeleteNumber(item.ID(), item.Amount()) }

// DeleteItem deletes item from the exact slot, with a refresh
// (deleteItem, ItemContainer.ts:598-600). item is a pointer
// because deleteItemContainer may clamp its amount (see
// DeleteItemContainer).
func (c *itemContainer) DeleteItem(item *Item, slot int) {
	c.DeletedItem(item, slot, true)
}

// DeleteAtSlot deletes amount from the exact slot, with a refresh
// by default (deleteAtSlot, ItemContainer.ts:606-622). It is for
// use after packet decoding has already validated the clicked
// slot: the amount is clamped to what the slot holds, and an
// empty or out-of-range slot deletes nothing.
func (c *itemContainer) DeleteAtSlot(slot, amount int, refresh bool) {
	if slot < 0 || slot >= c.Capacity() || slot >= len(c.items) {
		return
	}
	held := c.items[slot]
	if held.ID() < 0 || held.Amount() <= 0 {
		return
	}
	deleteAmount := amount
	if deleteAmount < 0 {
		deleteAmount = 0
	}
	if deleteAmount > held.Amount() {
		deleteAmount = held.Amount()
	}
	if deleteAmount <= 0 {
		return
	}
	target := NewItem(held.ID(), deleteAmount)
	c.DeletedItem(&target, slot, refresh)
}

// Delete deletes amount of id, with a refresh (delete,
// ItemContainer.ts:631-633).
func (c *itemContainer) Delete(id, amount int) { c.Deleted(id, amount, true) }

// DeleteNumber deletes amount of id, with a refresh
// (deleteNumber, ItemContainer.ts:634-636). Identical to Delete,
// as in TS.
func (c *itemContainer) DeleteNumber(id, amount int) { c.Deleted(id, amount, true) }

// DeleteBoolean deletes item's amount of its id, refreshing when
// asked (deleteBoolean, ItemContainer.ts:638-640).
func (c *itemContainer) DeleteBoolean(item Item, refresh bool) {
	c.Deleted(item.ID(), item.Amount(), refresh)
}

// Deleted deletes amount of id, refreshing when asked (deleted,
// ItemContainer.ts:649-669). One id can sit in several stacks
// with different metadata; when the item stacks and two or more
// such stacks exist, the amount is taken across them in slot
// order (otherwise deleteItemContainer would stop at the end of
// the one slot it is given). Otherwise the deletion goes to the
// first slot holding the id.
func (c *itemContainer) Deleted(id, amount int, refresh bool) {
	var stacks []int
	if id >= 0 && amount > 0 {
		for slot := range c.items {
			if c.items[slot].ID() == id && c.items[slot].Amount() > 0 {
				stacks = append(stacks, slot)
			}
		}
	}
	if len(stacks) < 2 || !(c.stackable(id) || c.StackType() == StackTypeStacks) {
		target := NewItem(id, amount)
		c.DeletedItem(&target, c.GetSlotForItemID(id), refresh)
		return
	}
	left := amount
	for _, slot := range stacks {
		if left <= 0 {
			break
		}
		taken := left
		if taken > c.items[slot].Amount() {
			taken = c.items[slot].Amount()
		}
		target := NewItem(id, taken)
		c.DeletedItem(&target, slot, false)
		left -= taken
	}
	if refresh {
		c.RefreshItems()
	}
}

// DeletedItem deletes item from slot, refreshing when asked
// (deletedItem, ItemContainer.ts:671-673). item is a pointer
// because deleteItemContainer may clamp its amount.
func (c *itemContainer) DeletedItem(item *Item, slot int, refresh bool) {
	c.DeleteItemContainer(item, slot, refresh, nil)
}

// DeleteItemContainer deletes item from slot, refreshing when
// asked, optionally on its way to container to
// (deleteItemContainer, ItemContainer.ts:675-738).
//
// The amount is clamped to what the container holds in total.
// Stackable items subtract from the slot, clearing it (id -1,
// meta cleared) when the remainder drops below 1. Non-stackable
// items clear one slot per unit, re-finding the next slot
// holding the id after each, and stop early when the destination
// container can no longer take the item.
//
// item is a pointer because TS clamps the passed Item's amount
// in place (ItemContainer.ts:695-697) and callers observe it. A
// nil item, a negative slot, or an out-of-range slot deletes
// nothing; TS would throw on an out-of-range slot.
//
// The placeholder-leaving branch (ItemContainer.ts:691-694,
// 705-713, 722-728) follows the player's placeholders preference
// through placeholdersEnabled, with placeholderBankID supplying
// the lookup (bank.go). It leaves a placeholder only when the
// destination is the inventory and the item has a cache
// placeholder id, as in TS.
func (c *itemContainer) DeleteItemContainer(item *Item, slot int, refresh bool, to Container) {
	if item == nil || item.ID() < 0 || (item.Amount() <= 0 && !c.isBank()) || slot < 0 || slot >= len(c.items) {
		return
	}
	// Only items with a cache placeholder, or a family shared
	// through one, leave one (ItemContainer.ts:688-694).
	placeholderBankID := -1
	leavePlaceHolder := false
	if to != nil && to.Kind() == ContainerInventory && c.isBank() && c.placeholdersEnabled() {
		placeholderBankID = c.placeholderBankID(item.ID())
		leavePlaceHolder = placeholderBankID >= 0
	}
	if item.Amount() > c.GetAmount(item.ID()) {
		item.SetAmount(c.GetAmount(item.ID()))
	}
	if c.stackable(item.ID()) || c.StackType() == StackTypeStacks {
		c.items[slot].SetAmount(c.items[slot].Amount() - item.Amount())
		if c.items[slot].Amount() < 1 {
			c.items[slot].SetAmount(0)
			if !leavePlaceHolder {
				c.items[slot].SetID(-1)
				c.items[slot].SetMeta(nil)
			} else if c.items[slot].ID() != placeholderBankID {
				c.items[slot].SetID(placeholderBankID)
				c.items[slot].SetMeta(nil)
			}
		}
	} else {
		amount := item.Amount()
		for amount > 0 {
			// Use the same acceptance as the caller: a full
			// bank still takes an item it holds
			// (ItemContainer.ts:718-721).
			if slot == -1 || (to != nil && !to.CanTake(*item)) {
				break
			}
			if !leavePlaceHolder {
				c.items[slot].SetID(-1)
				c.items[slot].SetMeta(nil)
			} else if c.items[slot].ID() != placeholderBankID {
				c.items[slot].SetID(placeholderBankID)
				c.items[slot].SetMeta(nil)
			}
			c.items[slot].SetAmount(0)
			slot = c.GetSlotForItemID(item.ID())
			amount--
		}
	}
	if refresh {
		c.RefreshItems()
	}
}

// placeholdersEnabled reports whether the player leaves bank
// placeholders (Player.isPlaceholders, Player.ts:1614-1616): the
// bank:placeholders attribute strictly true. The container
// reaches the player through its own player field, as TS Bank
// reaches it through getPlayer() (Bank.ts:1514). A nil player
// (a bare container in a test) leaves no placeholders, as a
// missing attribute does in TS.
func (c *itemContainer) placeholdersEnabled() bool {
	return c.player != nil && c.player.GetAttribute(BankPlaceholdersAttribute) == true
}

// DeleteItemAny deletes each item in items (deleteItemAny,
// ItemContainer.ts:740-749). A nil slice deletes nothing.
func (c *itemContainer) DeleteItemAny(items []Item) {
	for _, item := range items {
		c.Deletes(item)
	}
}

// MoveItems moves every valid stack in this container to to
// (moveItems, ItemContainer.ts:801-819), stopping when the
// destination can take no more, then refreshing each side as
// asked.
func (c *itemContainer) MoveItems(to Container, refreshOrig, refreshTo bool) {
	if to == nil {
		return
	}
	for _, it := range c.GetValidItems() {
		if to.GetFreeSlots() <= 0 && !(to.ContainsNumber(it.ID()) && c.stackable(it.ID())) {
			break
		}
		to.Add(it, false)
		c.Deleted(it.ID(), it.Amount(), false)
	}
	if refreshOrig {
		c.RefreshItems()
	}
	if refreshTo {
		to.RefreshItems()
	}
}

// AddItemSet adds every item, each with a refresh (addItemSet,
// ItemContainer.ts:821-828).
func (c *itemContainer) AddItemSet(items []Item) {
	for _, item := range items {
		c.AddItem(item)
	}
}

// DeleteItemSet deletes every item, each with a refresh
// (deleteItemSet, ItemContainer.ts:830-838).
func (c *itemContainer) DeleteItemSet(items []Item) {
	for _, item := range items {
		c.Deletes(item)
	}
}

// RefreshItems sends the container's current contents to the
// client through the coordinator-installed hook (refreshItems;
// the per-type TS methods are Inventory.ts:20-33,
// Equipment.ts:70-81, Bank.ts:1417-1433). With no hook installed
// it is a silent no-op, except for the equipment side effect
// below. The game layer never imports protocol: the hook carries
// a RefreshEvent and the cmd/server adapter encodes and sends.
//
//   - Inventory: the own-inventory guard (Inventory.ts:21-25)
//     applies — a copy built on the player to plan a change must
//     never reach the client, so a refresh from a container that
//     is not the player's own inventory returns silently. The
//     bank-side-ops call (Inventory.ts:26-28) is a gap: the
//     interface-id state it reads does not exist in Go.
//   - Equipment: the hook runs first, then
//     Player.onEquipmentChanged flags APPEARANCE (and invokes the
//     coordinator's OnEquipmentChanged hook). The worn-inventory
//     send for transformed players (Equipment.ts:72-74) never
//     fires: no NPC-transformation state exists in Go. The
//     equipment:changed plugin event (Equipment.ts:76-79) is a
//     gap: no plugin system exists.
//   - Bank: the tab-reconfiguration early return
//     (Bank.ts:1418-1420) degenerates — no tab UI state exists,
//     so refresh always proceeds — and the hook carries the tab's
//     contents; the adapter sends the tab-count varbits, the
//     full-bank snapshot, and the inventory snapshot, as
//     Bank.refreshItems does. The bank:refreshed plugin event is
//     a gap.
func (c *itemContainer) RefreshItems() {
	switch c.kind {
	case ContainerInventory:
		if p := c.player; p != nil {
			if own := p.Inventory(); own != nil && own.itemContainer != nil && own.itemContainer != c {
				return
			}
		}
		// Bank side-ops call skipped: no interface-id state.
		c.invokeRefresh()
	case ContainerEquipment:
		c.invokeRefresh()
		// Transformed-player worn-inventory send skipped: no
		// NPC-transformation state exists in Go.
		if p := c.player; p != nil {
			p.onEquipmentChanged()
		}
		// equipment:changed plugin event skipped: no plugins.
	case ContainerBank:
		// Tab reconfiguration skipped: no tab UI state exists.
		c.invokeRefresh()
	default:
		c.invokeRefresh()
	}
}

// invokeRefresh delivers a RefreshEvent to the installed hook, or
// does nothing when no hook is installed.
func (c *itemContainer) invokeRefresh() {
	if c.refreshHook == nil {
		return
	}
	c.refreshHook(RefreshEvent{Kind: c.kind, Player: c.player, Items: c.Items()})
}

// Full runs the container-full behavior (full; the per-type TS
// methods are Inventory.ts:35-38, Equipment.ts:11-13,
// Bank.ts:43-48). Inventory.full sends "Not enough space in
// your inventory."; Bank.full emits the bank:full plugin event
// (a plugin may rewrite the message) and sends the resulting
// message — with no plugin system the default "Not enough
// space in bank." is sent, and the event is a gap. Both
// messages go through Player.SendMessage, which drops them
// for a nil player. Equipment.full is a no-op in TS and here.
// The method exists so switchItem/switchItems callers keep
// their TS shape.
func (c *itemContainer) Full() {
	switch c.kind {
	case ContainerInventory:
		c.player.SendMessage("Not enough space in your inventory.")
	case ContainerBank:
		// Bank.full's bank:full custom event (Bank.ts:43-48)
		// is a GAP: no plugin system exists to rewrite the
		// message, so the default is sent.
		c.player.SendMessage("Not enough space in bank.")
	}
}
