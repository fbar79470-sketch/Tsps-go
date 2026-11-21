package game

import "testing"

// TestInventoryPlanningCopyGuard ports the own-inventory guard
// (Inventory.ts:20-25, issue 4684): a copy built on the player to
// plan a change must never reach the client. Only the player's
// OWN inventory refreshes through the hook.
func TestInventoryPlanningCopyGuard(t *testing.T) {
	p := testPlayerWithDefs(nil)
	own := p.Inventory()
	var refreshed []*Inventory
	hook := func(ev RefreshEvent) { refreshed = append(refreshed, p.Inventory()) }
	own.SetRefreshHook(hook)
	// A planning copy: same player, different container, same hook
	// (inherited at construction).
	planning := NewInventory(p)
	if planning == own {
		t.Fatal("planning copy is the own inventory")
	}
	planning.Add(NewItem(4151, 1), true)
	if len(refreshed) != 0 {
		t.Fatal("planning-copy refresh reached the hook (issue 4684)")
	}
	// The own inventory refreshes normally.
	own.RefreshItems()
	if len(refreshed) != 1 {
		t.Fatalf("own refresh reached the hook %d times, want 1", len(refreshed))
	}
	// A player-less inventory has no own inventory to compare
	// against: it refreshes (the TS guard's own == null case
	// falls through to the send).
	orphan := NewInventory(nil)
	orphanCalls := 0
	orphan.SetRefreshHook(func(ev RefreshEvent) { orphanCalls++ })
	orphan.RefreshItems()
	if orphanCalls != 1 {
		t.Fatalf("orphan inventory refreshed %d times, want 1", orphanCalls)
	}
}

func TestInventoryInterfaceID(t *testing.T) {
	if InventoryInterfaceID != 3214 {
		t.Fatalf("InventoryInterfaceID = %d, want 3214", InventoryInterfaceID)
	}
}
