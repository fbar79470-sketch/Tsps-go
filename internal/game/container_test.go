package game

import (
	"encoding/json"
	"testing"
)

// testPlayer builds a player with the given definition source
// installed. Containers are reached through the player so the
// own-inventory guard and bank tab creation behave as in
// production.
func testPlayerWithDefs(src ItemDefinitionSource) *Player {
	p := NewPlayer("test", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(src)
	return p
}

func slotIDs(c Container) []int {
	items := c.Items()
	ids := make([]int, len(items))
	for i, it := range items {
		ids[i] = it.ID()
	}
	return ids
}

func TestContainerCapacitiesAndStackTypes(t *testing.T) {
	p := testPlayerWithDefs(nil)
	if p.Inventory().Capacity() != 28 || p.Inventory().StackType() != StackTypeDefault {
		t.Fatalf("inventory capacity/stack = %d/%d", p.Inventory().Capacity(), p.Inventory().StackType())
	}
	if p.Equipment().Capacity() != 14 || p.Equipment().StackType() != StackTypeDefault {
		t.Fatalf("equipment capacity/stack = %d/%d", p.Equipment().Capacity(), p.Equipment().StackType())
	}
	bank := p.Bank(0)
	if bank.Capacity() != 352 || bank.StackType() != StackTypeStacks {
		t.Fatalf("bank capacity/stack = %d/%d", bank.Capacity(), bank.StackType())
	}
	if p.BankCount() != 11 || p.Bank(10) == nil || p.Bank(11) != nil || p.Bank(-1) != nil {
		t.Fatal("bank tab range wrong")
	}
	// Every slot starts empty (ItemContainer.ts:55-56). The bank's
	// free-slot count includes growth headroom (see
	// TestBankFreeSlotsIncludesHeadroom), so only IsEmpty is
	// asserted for it.
	for _, c := range []Container{p.Inventory(), p.Equipment()} {
		if !c.IsEmpty() || c.GetFreeSlots() != c.Capacity() {
			t.Fatalf("fresh container not empty: free=%d cap=%d", c.GetFreeSlots(), c.Capacity())
		}
	}
	if !bank.IsEmpty() {
		t.Fatal("fresh bank tab not empty")
	}
}

func TestContainerSetItemsLengthHandling(t *testing.T) {
	p := testPlayerWithDefs(nil)
	inv := p.Inventory()
	// Short array pads with empties to capacity.
	inv.SetItems([]Item{NewItem(995, 10)})
	if len(inv.Items()) != 28 || inv.ItemAt(0).ID() != 995 || inv.ItemAt(1).ID() != -1 {
		t.Fatalf("short SetItems: len=%d slot0=%d slot1=%d", len(inv.Items()), inv.ItemAt(0).ID(), inv.ItemAt(1).ID())
	}
	// Long array truncates.
	long := make([]Item, 40)
	for i := range long {
		long[i] = NewItem(100+i, 1)
	}
	inv.SetItems(long)
	if len(inv.Items()) != 28 || inv.ItemAt(27).ID() != 127 {
		t.Fatalf("long SetItems: len=%d slot27=%d", len(inv.Items()), inv.ItemAt(27).ID())
	}
	// Bank takes the array length as its capacity.
	bank := p.Bank(0)
	bank.SetItems([]Item{NewItem(995, 5), NewItem(4151, 1)})
	if bank.Capacity() != 2 || bank.ItemAt(1).ID() != 4151 {
		t.Fatalf("bank SetItems: capacity=%d slot1=%d", bank.Capacity(), bank.ItemAt(1).ID())
	}
	// SetItems copies: mutating the source slice afterwards must
	// not touch the container.
	src := []Item{NewItem(1, 1)}
	inv.SetItems(src)
	src[0].SetID(2)
	if inv.ItemAt(0).ID() != 1 {
		t.Fatal("SetItems aliased the caller's slice")
	}
}

func TestContainerItemsDefensiveCopies(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{stackable: map[int]bool{995: true}})
	inv := p.Inventory()
	inv.Adds(995, 5)
	got := inv.Items()
	got[0].SetID(1)
	got[0].SetAmount(99)
	if inv.ItemAt(0).ID() != 995 || inv.ItemAt(0).Amount() != 5 {
		t.Fatal("mutating Items() result mutated the container")
	}
	at := inv.ItemAt(0)
	at.SetID(1)
	if inv.ItemAt(0).ID() != 995 {
		t.Fatal("mutating ItemAt() result mutated the container")
	}
}

func TestAddStackableMergeAndCap(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	inv.Add(NewItem(995, 100), false)
	inv.Add(NewItem(995, 50), false)
	if inv.GetAmount(995) != 150 || inv.GetFreeSlots() != 27 {
		t.Fatalf("stackable merge: amount=%d free=%d", inv.GetAmount(995), inv.GetFreeSlots())
	}
	// Different meta does not merge: a second stack forms.
	meta := NewItemWithMeta(995, 10, map[string]json.RawMessage{"k": rawJSON(t, "1")})
	inv.Add(meta, false)
	if inv.GetAmount(995) != 160 || inv.GetFreeSlots() != 26 {
		t.Fatalf("meta-split stacks: amount=%d free=%d", inv.GetAmount(995), inv.GetFreeSlots())
	}
	// Cap at MAX_STACK_AMOUNT; the excess is lost, not spilled.
	inv.Add(NewItem(995, MaxStackAmount), false)
	if inv.ItemAt(0).Amount() != MaxStackAmount {
		t.Fatalf("capped stack = %d, want %d", inv.ItemAt(0).Amount(), MaxStackAmount)
	}
	if inv.GetFreeSlots() != 26 {
		t.Fatalf("cap overflow spilled: free=%d", inv.GetFreeSlots())
	}
}

func TestAddNonStackableFanOut(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	inv.Add(NewItem(4151, 3), false)
	if inv.GetFreeSlots() != 25 || inv.GetAmount(4151) != 3 {
		t.Fatalf("fan-out: free=%d amount=%d", inv.GetFreeSlots(), inv.GetAmount(4151))
	}
	for i := 0; i < 3; i++ {
		if inv.ItemAt(i).ID() != 4151 || inv.ItemAt(i).Amount() != 1 {
			t.Fatalf("slot %d = %+v, want one whip", i, inv.ItemAt(i))
		}
	}
	// Overflow: fills what fits, keeps the rest nowhere, no panic.
	inv.Add(NewItem(4151, 30), false)
	if inv.GetFreeSlots() != 0 || inv.GetAmount(4151) != 28 {
		t.Fatalf("overflow fill: free=%d amount=%d", inv.GetFreeSlots(), inv.GetAmount(4151))
	}
	// Invalid adds are ignored.
	inv2 := NewInventory(nil)
	inv2.Add(NewItem(-1, 5), false)
	inv2.Add(NewItem(4151, 0), false)
	inv2.Add(NewItem(4151, -3), false)
	if !inv2.IsEmpty() {
		t.Fatal("invalid adds changed the container")
	}
}

func TestAddRefreshFlag(t *testing.T) {
	p := testPlayerWithDefs(nil)
	inv := p.Inventory()
	calls := 0
	inv.SetRefreshHook(func(ev RefreshEvent) { calls++ })
	inv.Add(NewItem(4151, 1), false)
	if calls != 0 {
		t.Fatal("Add(refresh=false) refreshed")
	}
	inv.Add(NewItem(4151, 1), true)
	if calls != 1 {
		t.Fatalf("Add(refresh=true) refreshed %d times", calls)
	}
}

func TestGetAddableAmount(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	inv.Add(NewItem(995, MaxStackAmount-10), false)
	if got := inv.GetAddableAmount(NewItem(995, 50)); got != 10 {
		t.Fatalf("stackable addable = %d, want 10", got)
	}
	// Meta-mismatched stack: a new stack, full room up to MAX.
	meta := NewItemWithMeta(995, 50, map[string]json.RawMessage{"k": rawJSON(t, "1")})
	if got := inv.GetAddableAmount(meta); got != 50 {
		t.Fatalf("new-stack addable = %d, want 50", got)
	}
	// Non-stackable: free slots bound it.
	inv2 := p.Equipment()
	if got := inv2.GetAddableAmount(NewItem(4151, 20)); got != 14 {
		t.Fatalf("non-stack addable = %d, want 14", got)
	}
	// Invalid / zero requests: 0, and the container is unchanged.
	before := inv.Items()
	if got := inv.GetAddableAmount(NewItem(-1, 5)); got != 0 {
		t.Fatalf("invalid id addable = %d", got)
	}
	if got := inv.GetAddableAmount(NewItem(995, 0)); got != 0 {
		t.Fatalf("zero amount addable = %d", got)
	}
	after := inv.Items()
	for i := range before {
		if !before[i].Equals(after[i]) {
			t.Fatal("GetAddableAmount changed the container")
		}
	}
}

func TestDeleteAcrossMetaStacks(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	inv.Add(NewItem(995, 10), false)
	inv.Add(NewItemWithMeta(995, 10, map[string]json.RawMessage{"k": rawJSON(t, "1")}), false)
	// deleted() spans stacks when >= 2 stacks exist (ItemContainer.ts:649-669).
	inv.Deleted(995, 15, false)
	if inv.GetAmount(995) != 5 {
		t.Fatalf("cross-stack delete left %d, want 5", inv.GetAmount(995))
	}
	// The remainder sits in the second stack.
	if inv.ItemAt(0).ID() != -1 || inv.ItemAt(1).Amount() != 5 {
		t.Fatalf("slots after cross-stack delete: %v %v", inv.ItemAt(0), inv.ItemAt(1))
	}
}

func TestDeleteNonStackableAndClamp(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	inv.Add(NewItem(4151, 3), false)
	inv.Deleted(4151, 2, false)
	if inv.GetAmount(4151) != 1 || inv.GetFreeSlots() != 27 {
		t.Fatalf("non-stack delete: amount=%d free=%d", inv.GetAmount(4151), inv.GetFreeSlots())
	}
	// Deleting more than held clamps to what exists.
	inv.Deleted(4151, 99, false)
	if !inv.IsEmpty() {
		t.Fatal("over-delete left items")
	}
	// Deleting an absent id is a no-op (slot -1 guard).
	inv.Deleted(9999, 5, false)
	if !inv.IsEmpty() {
		t.Fatal("absent-id delete changed the container")
	}
}

func TestDeleteAtSlot(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	inv.Add(NewItem(995, 10), false)
	inv.Add(NewItem(995, 10), false) // merges into slot 0
	inv.SetItem(1, NewItem(4151, 1))
	inv.DeleteAtSlot(1, 1, false)
	if inv.ItemAt(1).ID() != -1 {
		t.Fatal("DeleteAtSlot did not clear the slot")
	}
	inv.DeleteAtSlot(0, 99, false) // clamps to the stack
	if inv.ItemAt(0).ID() != -1 {
		t.Fatal("DeleteAtSlot over-amount left a stack")
	}
	inv.DeleteAtSlot(-1, 1, false)
	inv.DeleteAtSlot(28, 1, false)
	inv.DeleteAtSlot(5, 1, false) // empty slot
	if !inv.IsEmpty() {
		t.Fatal("invalid DeleteAtSlot changed the container")
	}
}

func TestSwapShiftSwapSort(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	inv.SetItem(0, NewItem(1, 1))
	inv.SetItem(2, NewItem(2, 1))
	inv.Swap(0, 2)
	if inv.ItemAt(0).ID() != 2 || inv.ItemAt(2).ID() != 1 {
		t.Fatalf("swap: %d %d", inv.ItemAt(0).ID(), inv.ItemAt(2).ID())
	}
	// Swap from an empty slot is a no-op.
	inv.Swap(5, 0)
	if inv.ItemAt(0).ID() != 2 {
		t.Fatal("swap from empty slot moved an item")
	}
	// ShiftSwap is the upstream stub: it never swaps.
	inv.ShiftSwap(0, 2)
	if inv.ItemAt(0).ID() != 2 || inv.ItemAt(2).ID() != 1 {
		t.Fatal("ShiftSwap swapped (upstream never swaps)")
	}
	// SortItems compacts toward the front, order preserved.
	inv.ResetItems()
	inv.SetItem(3, NewItem(3, 1))
	inv.SetItem(5, NewItem(5, 1))
	inv.SetItem(6, NewItem(6, 1))
	inv.SortItems()
	ids := slotIDs(inv)
	if ids[0] != 3 || ids[1] != 5 || ids[2] != 6 || ids[3] != -1 {
		t.Fatalf("sort: first slots %v", ids[:4])
	}
}

func TestContainsAndSlots(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	inv.Add(NewItem(4151, 2), false)
	inv.Add(NewItem(995, 1), false)
	if !inv.ContainsNumber(4151) || inv.ContainsNumber(9999) {
		t.Fatal("ContainsNumber wrong")
	}
	if !inv.ContainsItem(NewItem(4151, 2)) || inv.ContainsItem(NewItem(4151, 3)) {
		t.Fatal("ContainsItem wrong")
	}
	if !inv.ContainsArray([]Item{NewItem(4151, 1), NewItem(995, 1)}) ||
		inv.ContainsArray([]Item{}) || inv.ContainsArray([]Item{NewItem(9999, 1)}) {
		t.Fatal("ContainsArray wrong")
	}
	if !inv.ContainsAnyIDs([]int{9999, 995}) || inv.ContainsAnyIDs([]int{-1}) ||
		inv.ContainsAnyIDs([]int{}) {
		t.Fatal("ContainsAnyIDs wrong")
	}
	if inv.GetSlotForItemID(4151) != 0 || inv.GetSlotForItemID(9999) != -1 {
		t.Fatal("GetSlotForItemID wrong")
	}
	if inv.GetSlot(0) != 4151 || inv.GetSlot(5) != -1 || inv.GetSlot(28) != -1 || inv.GetSlot(-1) != -1 {
		t.Fatalf("GetSlot: %d %d %d %d", inv.GetSlot(0), inv.GetSlot(5), inv.GetSlot(28), inv.GetSlot(-1))
	}
	if inv.GetAmount(4151) != 2 || inv.GetAmountForSlot(0) != 1 || inv.GetAmountForSlot(28) != 0 {
		t.Fatal("GetAmount/GetAmountForSlot wrong")
	}
	if !inv.IsSlotOccupied(0) || inv.IsSlotOccupied(5) || inv.IsSlotOccupied(28) {
		t.Fatal("IsSlotOccupied wrong")
	}
	if inv.IsSlotFree(0) || !inv.IsSlotFree(5) || !inv.IsSlotFree(28) {
		t.Fatal("IsSlotFree wrong")
	}
	if got, ok := inv.GetByID(995); !ok || got.ID() != 995 {
		t.Fatal("GetByID wrong")
	}
	if _, ok := inv.GetByID(9999); ok {
		t.Fatal("GetByID found an absent id")
	}
}

func TestCanTakeAndFullBoolean(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	if !inv.CanTake(NewItem(4151, 1)) || inv.FullBoolean(4151) {
		t.Fatal("empty container cannot take / reads full")
	}
	// Fill every slot with non-stackables.
	for i := 0; i < 28; i++ {
		inv.SetItem(i, NewItem(1000+i, 1))
	}
	if inv.CanTake(NewItem(4151, 1)) || !inv.FullBoolean(4151) || !inv.IsFull() {
		t.Fatal("full container takes a new non-stackable")
	}
	// A stackable already held still fits.
	inv.SetItem(0, NewItem(995, 5))
	if !inv.CanTake(NewItem(995, 1)) || inv.FullBoolean(995) {
		t.Fatal("full container cannot join a held stack")
	}
}

func TestSwitchItemToBankUnnotes(t *testing.T) {
	src := &testItemDefs{
		stackable: map[int]bool{995: true, 996: true},
		unNote:    map[int]int{996: 995},
	}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	bank := p.Bank(0)
	// Noted coins (996) in the inventory deposit as unnoted (995).
	inv.SetItem(0, NewItem(996, 10))
	item := NewItem(996, 10)
	inv.SwitchItem(bank, &item, false, 0, false)
	if item.ID() != 995 {
		t.Fatalf("switchItem left item id %d, want unnoted 995", item.ID())
	}
	if inv.GetAmount(996) != 0 || bank.GetAmount(995) != 10 {
		t.Fatalf("deposit: inv 996=%d bank 995=%d", inv.GetAmount(996), bank.GetAmount(995))
	}
	// Wrong slot: no-op.
	inv.SetItem(0, NewItem(4151, 1))
	wrong := NewItem(995, 1)
	inv.SwitchItem(bank, &wrong, false, 0, false)
	if bank.GetAmount(995) != 10 || inv.GetAmount(4151) != 1 {
		t.Fatal("switchItem with mismatched slot moved an item")
	}
}

func TestSwitchItemBankCapClamp(t *testing.T) {
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p := testPlayerWithDefs(src)
	inv := p.Inventory()
	bank := p.Bank(0)
	bank.SetItem(0, NewItem(995, MaxStackAmount-5))
	inv.SetItem(0, NewItem(995, 10))
	item := NewItem(995, 10)
	inv.SwitchItem(bank, &item, false, 0, false)
	// Only 5 fit: the item amount clamps, the bank caps, and 5
	// stay in the inventory.
	if item.Amount() != 5 || bank.GetAmount(995) != MaxStackAmount || inv.GetAmount(995) != 5 {
		t.Fatalf("clamp: item=%d bank=%d inv=%d", item.Amount(), bank.GetAmount(995), inv.GetAmount(995))
	}
}

func TestSwitchItemsAcrossSlots(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	eq := p.Equipment()
	inv.Add(NewItem(4151, 3), false)
	item := NewItem(4151, 99) // clamped to the 3 held
	inv.SwitchItems(eq, &item, false, false)
	if item.Amount() != 3 || inv.GetAmount(4151) != 0 || eq.GetAmount(4151) != 3 {
		t.Fatalf("switchItems: item=%d inv=%d eq=%d", item.Amount(), inv.GetAmount(4151), eq.GetAmount(4151))
	}
}

func TestMoveItemsStopsWhenFull(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	eq := p.Equipment()
	inv.Add(NewItem(4151, 3), false)
	inv.MoveItems(eq, false, false)
	if eq.GetAmount(4151) != 3 || !inv.IsEmpty() {
		t.Fatal("MoveItems did not move everything")
	}
	// Fill the destination; MoveItems stops at the boundary.
	for i := 0; i < 14; i++ {
		eq.SetItem(i, NewItem(2000+i, 1))
	}
	inv.Add(NewItem(4151, 2), false)
	inv.MoveItems(eq, false, false)
	if inv.GetAmount(4151) != 2 {
		t.Fatal("MoveItems moved into a full container")
	}
}

func TestAddItemsAndSets(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	inv := p.Inventory()
	inv.AddItems([]Item{NewItem(1, 1), NewItem(-1, 5), NewItem(2, 0), NewItem(3, 2)}, false)
	if inv.GetAmount(1) != 1 || inv.GetAmount(3) != 2 || inv.GetFreeSlots() != 25 {
		t.Fatalf("AddItems: amounts %d/%d free %d", inv.GetAmount(1), inv.GetAmount(3), inv.GetFreeSlots())
	}
	inv.AddItems(nil, false)
	inv.DeleteItemSet([]Item{NewItem(1, 1), NewItem(3, 1)})
	if inv.GetAmount(1) != 0 || inv.GetAmount(3) != 1 {
		t.Fatal("DeleteItemSet wrong")
	}
	inv.DeleteItemAny([]Item{NewItem(3, 1)})
	if !inv.IsEmpty() {
		t.Fatal("DeleteItemAny left items")
	}
	inv.DeleteItemAny(nil)
	inv.AddItemSet([]Item{NewItem(7, 1)})
	if inv.GetAmount(7) != 1 {
		t.Fatal("AddItemSet wrong")
	}
}

func TestRefreshHookReceivesCopies(t *testing.T) {
	p := testPlayerWithDefs(nil)
	inv := p.Inventory()
	inv.Adds(995, 5)
	var seen []Item
	inv.SetRefreshHook(func(ev RefreshEvent) {
		if ev.Kind != ContainerInventory || ev.Player != p {
			t.Errorf("refresh event kind/player wrong: %v", ev.Kind)
		}
		seen = ev.Items
	})
	inv.RefreshItems()
	if len(seen) != 28 || seen[0].ID() != 995 {
		t.Fatalf("refresh event items: len=%d first=%d", len(seen), seen[0].ID())
	}
	seen[0].SetID(1)
	if inv.ItemAt(0).ID() != 995 {
		t.Fatal("mutating refresh event items mutated the container")
	}
	// No hook: silent no-op, no panic.
	inv.SetRefreshHook(nil)
	inv.RefreshItems()
}

func TestFullIsNoOp(t *testing.T) {
	p := testPlayerWithDefs(nil)
	p.Inventory().Full()
	p.Equipment().Full()
	p.Bank(0).Full()
}
