package definition

import (
	"testing"
)

// HasHighAlchValue / HasLowAlchValue distinguish an unset alchemy
// value (the examine price line falls back to a share of the
// store value) from a set 0 (an item the alchemy spells refuse).
// See ItemDefinition.HasHighAlchValue.
func TestItemHasAlchValues(t *testing.T) {
	// No JSON stages: nothing set.
	store := loadItems(t, minimalFiles(), itemCacheSpec())
	sword := store.Item(2)
	if sword.HasHighAlchValue() || sword.HasLowAlchValue() {
		t.Fatal("unset alch values report set")
	}
	if sword.HighAlchValue() != 0 || sword.LowAlchValue() != 0 {
		t.Fatal("unset alch values are not 0")
	}

	// Gameplay sets both.
	store = loadItems(t, withFiles(map[string]string{
		"item-gameplay.json": `[{"id": 2, "name": "Bronze sword", "highAlch": 6, "lowAlch": 4}]`,
	}), itemCacheSpec())
	sword = store.Item(2)
	if !sword.HasHighAlchValue() || !sword.HasLowAlchValue() {
		t.Fatal("gameplay alch values report unset")
	}

	// Alch entries with invalid/missing numbers store an
	// explicit 0 — set, not unset.
	store = loadItems(t, withFiles(map[string]string{
		"item-alch.json": `{"4": {"highalch": "x", "lowalch": 1.5}, "5": {}}`,
	}), itemCacheSpec())
	thing := store.Item(4)
	if thing.HighAlchValue() != 0 || !thing.HasHighAlchValue() || !thing.HasLowAlchValue() {
		t.Fatalf("invalid alch entry = %d/%d set=%v/%v, want 0/0 set",
			thing.HighAlchValue(), thing.LowAlchValue(), thing.HasHighAlchValue(), thing.HasLowAlchValue())
	}
	if !store.Item(5).HasHighAlchValue() {
		t.Fatal("empty alch entry reports unset")
	}

	// Note propagation carries the flags.
	store = loadItems(t, withFiles(map[string]string{
		"item-alch.json": `{"2": {"highalch": 6, "lowalch": 4}}`,
	}), itemCacheSpec())
	note := store.Item(3)
	if note.HighAlchValue() != 6 || !note.HasHighAlchValue() || !note.HasLowAlchValue() {
		t.Fatalf("note alch = %d set=%v/%v", note.HighAlchValue(), note.HasHighAlchValue(), note.HasLowAlchValue())
	}

	// Overrides set the flags too.
	store = loadItems(t, withFiles(map[string]string{
		"item-overrides.json": `{"2": {"highAlch": 7}}`,
	}), itemCacheSpec())
	sword = store.Item(2)
	if sword.HighAlchValue() != 7 || !sword.HasHighAlchValue() || sword.HasLowAlchValue() {
		t.Fatalf("override alch = %d set=%v/%v", sword.HighAlchValue(), sword.HasHighAlchValue(), sword.HasLowAlchValue())
	}
}

// CanStockItemHook drops refused stock entries at load
// (ShopDefinitionLoader.ts:183). A nil hook keeps everything.
func TestCanStockItemHook(t *testing.T) {
	shops := `[{"id": 1, "name": "Shop", "originalStock": [
		{"id": 1, "amount": 10}, {"id": 2, "amount": 5}, {"id": 4, "amount": 1}]}]`

	// No hook: all stock kept.
	store := loadShops(t, shopFiles(shops, `[]`), LoadOptions{})
	if got := len(store.Shops()[0].OriginalStock()); got != 3 {
		t.Fatalf("stock without hook = %d entries, want 3", got)
	}

	// A hook refusing item 2 drops only that row.
	prev := CanStockItemHook
	CanStockItemHook = func(shopID, itemID int) bool {
		if shopID != 1 {
			t.Fatalf("hook shopID = %d, want 1", shopID)
		}
		return itemID != 2
	}
	defer func() { CanStockItemHook = prev }()
	store = loadShops(t, shopFiles(shops, `[]`), LoadOptions{})
	stock := store.Shops()[0].OriginalStock()
	if len(stock) != 2 || stock[0].ID != 1 || stock[1].ID != 4 {
		t.Fatalf("stock with hook = %+v, want ids 1 and 4", stock)
	}

	// The veto applies to `items` rows too (the combined stock).
	shops2 := `[{"id": 1, "name": "Shop", "stockAmount": 10, "items": [1, 2]}]`
	store = loadShops(t, shopFiles(shops2, `[]`), LoadOptions{})
	stock = store.Shops()[0].OriginalStock()
	if len(stock) != 1 || stock[0].ID != 1 {
		t.Fatalf("items stock with hook = %+v, want id 1 only", stock)
	}
}
