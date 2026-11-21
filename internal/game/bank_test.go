package game

import "testing"

func TestBankConstants(t *testing.T) {
	if MaxBankSlots != 1410 || TotalBankTabs != 11 || BankContainerStart != 50300 ||
		BankSearchTabIndex != 10 || BankInventoryInterfaceID != 5064 || BankMainInterfaceID != 12 {
		t.Fatalf("bank constants: max=%d tabs=%d start=%d search=%d inv=%d main=%d",
			MaxBankSlots, TotalBankTabs, BankContainerStart, BankSearchTabIndex,
			BankInventoryInterfaceID, BankMainInterfaceID)
	}
	if initialBankTabSlots != 352 {
		t.Fatalf("initialBankTabSlots = %d, want 352", initialBankTabSlots)
	}
}

func TestBankPlaceholdersValid(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	bank.SetItem(0, NewItem(995, 0)) // placeholder: amount 0, id >= 0
	bank.SetItem(1, NewItem(4151, 1))
	valid := bank.GetValidItems()
	if len(valid) != 2 {
		t.Fatalf("bank valid items = %d, want 2 (placeholder included)", len(valid))
	}
	// The placeholder is not an empty slot and not free.
	if bank.GetEmptySlot() == 0 {
		t.Fatal("placeholder slot reads empty in a bank")
	}
	if bank.IsEmpty() {
		t.Fatal("bank with a placeholder reads empty")
	}
	// In an inventory the same item is not valid.
	inv := p.Inventory()
	inv.SetItem(0, NewItem(995, 0))
	if len(inv.GetValidItems()) != 0 {
		t.Fatal("inventory counts an amount-0 item as valid")
	}
	if inv.GetEmptySlot() != 0 {
		t.Fatal("amount-0 slot does not read empty in an inventory")
	}
	// getSlotForItemId finds placeholders in banks only.
	if bank.GetSlotForItemID(995) != 0 {
		t.Fatal("bank GetSlotForItemID missed the placeholder")
	}
	if inv.GetSlotForItemID(995) != -1 {
		t.Fatal("inventory GetSlotForItemID found an amount-0 item")
	}
}

func TestBankGrowth(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	// Fill the initial 352 slots; the next empty slot grows the tab.
	for i := 0; i < 352; i++ {
		bank.SetItem(i, NewItem(1000+i, 1))
	}
	if got := bank.GetEmptySlot(); got != 352 {
		t.Fatalf("growth slot = %d, want 352", got)
	}
	if bank.Capacity() != 353 {
		t.Fatalf("grown capacity = %d, want 353", bank.Capacity())
	}
	// FreeSlot grows directly too, once the grown slot is taken.
	bank.SetItem(352, NewItem(2000, 1))
	if got := bank.FreeSlot(); got != 353 || bank.Capacity() != 354 {
		t.Fatalf("FreeSlot = %d cap %d", got, bank.Capacity())
	}
}

func TestBankRoomLeftCapsGrowth(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	// Fill tab 0 to MAX_SLOTS via SetItems: used slots across tabs
	// hit the capacity, so no room is left anywhere.
	full := make([]Item, MaxBankSlots)
	for i := range full {
		full[i] = NewItem(1000+i, 1)
	}
	p.Bank(0).SetItems(full)
	other := p.Bank(1)
	if other.GetEmptySlot() != -1 || other.GetFreeSlots() != 0 {
		t.Fatalf("full bank: emptySlot=%d free=%d", other.GetEmptySlot(), other.GetFreeSlots())
	}
	// The search tab is exempt from the capacity: it still grows.
	search := p.Bank(BankSearchTabIndex)
	if search.GetEmptySlot() != 0 {
		t.Fatalf("search tab empty slot = %d", search.GetEmptySlot())
	}
}

func TestBankFreeSlotsIncludesHeadroom(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	// Empty 352-slot tab: free = 352 + (1410 - 352) = 1410.
	if bank.GetFreeSlots() != MaxBankSlots {
		t.Fatalf("empty bank free slots = %d, want %d", bank.GetFreeSlots(), MaxBankSlots)
	}
	bank.SetItem(0, NewItem(995, 1))
	if bank.GetFreeSlots() != MaxBankSlots-1 {
		t.Fatalf("one-item bank free slots = %d", bank.GetFreeSlots())
	}
}

func TestBankCanTakePlaceholder(t *testing.T) {
	src := &testItemDefs{unNote: map[int]int{996: 995}}
	p := testPlayerWithDefs(src)
	bank := p.Bank(0)
	// Fill the tab to capacity with distinct items, one of them a
	// placeholder for 995.
	items := make([]Item, MaxBankSlots)
	for i := range items {
		items[i] = NewItem(2000+i, 1)
	}
	items[0] = NewItem(995, 0)
	bank.SetItems(items)
	if bank.GetFreeSlots() != 0 {
		t.Fatalf("setup: free slots = %d", bank.GetFreeSlots())
	}
	// The placeholder accepts its item, even noted (un-noted first).
	if !bank.CanTake(NewItem(996, 5)) {
		t.Fatal("full bank cannot take an item it holds as a placeholder")
	}
	if bank.CanTake(NewItem(9999, 5)) {
		t.Fatal("full bank takes an unheld item")
	}
	// Adding refills the placeholder slot in place.
	bank.Add(NewItem(995, 5), false)
	if bank.ItemAt(0).ID() != 995 || bank.ItemAt(0).Amount() != 5 {
		t.Fatalf("placeholder refill: %+v", bank.ItemAt(0))
	}
}

func TestBankSortKeepsPlaceholders(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	bank.SetItem(2, NewItem(995, 0)) // placeholder
	bank.SetItem(4, NewItem(4151, 1))
	bank.SetItem(5, NewItem(995, 10))
	bank.SortItems()
	ids := []int{bank.ItemAt(0).ID(), bank.ItemAt(1).ID(), bank.ItemAt(2).ID(), bank.ItemAt(3).ID()}
	amounts := []int{bank.ItemAt(0).Amount(), bank.ItemAt(1).Amount(), bank.ItemAt(2).Amount()}
	if ids[0] != 995 || amounts[0] != 0 || ids[1] != 4151 || ids[2] != 995 || amounts[2] != 10 || ids[3] != -1 {
		t.Fatalf("bank sort: ids=%v amounts=%v", ids, amounts)
	}
}

func TestBankRearrange(t *testing.T) {
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	refreshes := 0
	bank.SetRefreshHook(func(ev RefreshEvent) { refreshes++ })
	for i := 0; i < 4; i++ {
		bank.SetItem(i, NewItem(10+i, 1))
	}
	// Swap mode: exchange the two slots.
	bank.Rearrange(0, 3, false)
	if ids := slotIDs(bank)[:4]; ids[0] != 13 || ids[3] != 10 {
		t.Fatalf("swap rearrange: %v", ids)
	}
	// Insert mode: bubble slot 3 down to slot 0, shifting others.
	bank.Rearrange(3, 0, true)
	if ids := slotIDs(bank)[:4]; ids[0] != 10 || ids[1] != 13 || ids[2] != 11 || ids[3] != 12 {
		t.Fatalf("insert rearrange: %v", ids)
	}
	// Insert mode upward: bubble slot 0 up to slot 2.
	bank.Rearrange(0, 2, true)
	if ids := slotIDs(bank)[:4]; ids[0] != 13 || ids[1] != 11 || ids[2] != 10 || ids[3] != 12 {
		t.Fatalf("upward insert rearrange: %v", ids)
	}
	if refreshes != 3 {
		t.Fatalf("rearrange refreshed %d times, want 3", refreshes)
	}
}

func TestBankRefreshHookKind(t *testing.T) {
	p := testPlayerWithDefs(nil)
	bank := p.Bank(0)
	var kinds []ContainerKind
	bank.SetRefreshHook(func(ev RefreshEvent) { kinds = append(kinds, ev.Kind) })
	bank.RefreshItems()
	if len(kinds) != 1 || kinds[0] != ContainerBank {
		t.Fatalf("bank refresh kinds = %v", kinds)
	}
}

func TestPlaceholderHelpers(t *testing.T) {
	// Families are identity-only without the plugin event.
	if fam := PlaceholderFamily(995); len(fam) != 1 || fam[0] != 995 {
		t.Fatalf("PlaceholderFamily(995) = %v", fam)
	}
	if !SharesPlaceholder(995, 995) || SharesPlaceholder(995, 996) {
		t.Fatal("SharesPlaceholder identity rule wrong")
	}
	if !SharesPlaceholder(-1, -1) {
		t.Fatal("SharesPlaceholder(-1, -1) must be true (TS checks equality first)")
	}
	if SharesPlaceholder(-1, 5) || SharesPlaceholder(5, -1) {
		t.Fatal("SharesPlaceholder with a negative id must be false")
	}
	// PlaceholderBankID follows the cache placeholder term.
	src := &testItemDefs{placeholder: map[int]int{995: 20000}}
	p := testPlayerWithDefs(src)
	bank := p.Bank(0)
	if bank.PlaceholderBankID(995) != 995 {
		t.Fatalf("PlaceholderBankID(995) = %d", bank.PlaceholderBankID(995))
	}
	if bank.PlaceholderBankID(4151) != -1 || bank.PlaceholderBankID(-1) != -1 {
		t.Fatal("PlaceholderBankID without a cache placeholder must be -1")
	}
	// Nil source: no placeholder ids at all.
	p2 := testPlayerWithDefs(nil)
	if p2.Bank(0).PlaceholderBankID(995) != -1 {
		t.Fatal("nil-source PlaceholderBankID != -1")
	}
}

func TestBankStacksEverything(t *testing.T) {
	// STACKS containers stack even non-stackable definitions.
	p := testPlayerWithDefs(&testItemDefs{})
	bank := p.Bank(0)
	bank.Add(NewItem(4151, 5), false)
	if bank.GetAmount(4151) != 5 || bank.GetFreeSlots() != MaxBankSlots-1 {
		t.Fatalf("bank stack: amount=%d free=%d", bank.GetAmount(4151), bank.GetFreeSlots())
	}
	if bank.ItemAt(0).Amount() != 5 {
		t.Fatalf("bank stacked into multiple slots: %+v", bank.ItemAt(0))
	}
}

func TestBankAttributeKeys(t *testing.T) {
	// Player.ts:87-91.
	if BankNoteWithdrawalAttribute != "bank:note-withdrawal" {
		t.Errorf("BankNoteWithdrawalAttribute = %q", BankNoteWithdrawalAttribute)
	}
	if BankInsertModeAttribute != "bank:insert-mode" {
		t.Errorf("BankInsertModeAttribute = %q", BankInsertModeAttribute)
	}
	if BankQuantityModeAttribute != "bank:quantity-mode" {
		t.Errorf("BankQuantityModeAttribute = %q", BankQuantityModeAttribute)
	}
	if BankCustomQuantityAttribute != "bank:custom-quantity" {
		t.Errorf("BankCustomQuantityAttribute = %q", BankCustomQuantityAttribute)
	}
	if BankPlaceholdersAttribute != "bank:placeholders" {
		t.Errorf("BankPlaceholdersAttribute = %q", BankPlaceholdersAttribute)
	}
}

func TestBankDeleteLeavesPlaceholderWhenAttributeSet(t *testing.T) {
	// The placeholder-leaving branch (ItemContainer.ts:691-694)
	// follows the bank:placeholders attribute through
	// placeholdersEnabled (Player.isPlaceholders,
	// Player.ts:1614-1616). Deleting the whole stack to the
	// inventory leaves an amount-0 placeholder when the
	// attribute is strictly true, and clears the slot otherwise.
	newSetup := func() (*Player, *Bank) {
		p := testPlayerWithDefs(&testItemDefs{
			stackable:   map[int]bool{995: true},
			placeholder: map[int]int{995: 995},
		})
		bank := p.Bank(0)
		bank.SetItem(0, NewItem(995, 5))
		return p, bank
	}
	deleteAll := func(p *Player, bank *Bank) {
		item := NewItem(995, 5)
		bank.DeleteItemContainer(&item, 0, false, p.Inventory())
	}
	// Attribute true: the slot keeps id 995 at amount 0.
	p, bank := newSetup()
	p.SetAttribute(BankPlaceholdersAttribute, true)
	deleteAll(p, bank)
	if got := bank.ItemAt(0); got.ID() != 995 || got.Amount() != 0 {
		t.Errorf("placeholders on: slot = %+v, want id 995 amount 0", got)
	}
	// Attribute absent: the slot clears.
	p3, bank3 := newSetup()
	deleteAll(p3, bank3)
	if got := bank3.ItemAt(0); got.ID() != -1 {
		t.Errorf("placeholders unset: slot = %+v, want cleared", got)
	}
	// Attribute 1 (truthy but not === true): the slot clears.
	p4, bank4 := newSetup()
	p4.SetAttribute(BankPlaceholdersAttribute, 1)
	deleteAll(p4, bank4)
	if got := bank4.ItemAt(0); got.ID() != -1 {
		t.Errorf("placeholders=1: slot = %+v, want cleared", got)
	}
	// Attribute true but no inventory destination: the slot
	// clears (the branch needs the to-inventory condition).
	p5, bank5 := newSetup()
	p5.SetAttribute(BankPlaceholdersAttribute, true)
	item := NewItem(995, 5)
	bank5.DeleteItemContainer(&item, 0, false, nil)
	if got := bank5.ItemAt(0); got.ID() != -1 {
		t.Errorf("placeholders on, no destination: slot = %+v, want cleared", got)
	}
}
