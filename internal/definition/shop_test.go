package definition

import (
	"testing"
)

func shopCacheSpec() fakeCacheSpec {
	return fakeCacheSpec{
		objs: map[int][]byte{
			0: {0},
			1: objEntry("Coins", objStackable(), objPrice(1), objTradable()),
			2: objEntry("Bronze sword", objPrice(10), objTradable()),
			3: objEntry("Bronze swords", objPrice(11)),
			4: objEntry("Untradable thing", objPrice(100)),
		},
	}
}

func loadShops(t *testing.T, files map[string]string, opts LoadOptions) *Store {
	t.Helper()
	if opts.Cache == nil {
		defs, gamevals := newFakeCache(t, shopCacheSpec())
		opts.Cache = defs
		opts.Gamevals = gamevals
	}
	dir := fixtureDir(t, files)
	store, err := Load(dir, opts)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func shopFiles(shops, custom string) map[string]string {
	return withFiles(map[string]string{"shops.json": shops, "custom-shops.json": custom})
}

func TestShopHappyPath(t *testing.T) {
	shops := `[{"id": 1, "name": " General Store ", "currency": "coins", "stockAmount": 10,
		"defaultRestockTicks": "5t", "defaultDestockTicks": 6, "soldItemDestockTicks": 7,
		"generalStore": true, "sellsAt": 1000, "buysAt": 500, "changePer": 10,
		"prices": {"2": 50},
		"originalStock": [{"id": 2, "amount": 5, "restockTicks": 3},
		                  {"id": 4, "amount": 0},
		                  {"id": 3}],
		"items": [2, "Bronze sword", "Unknown item"]}]`
	store := loadShops(t, shopFiles(shops, `[]`), LoadOptions{})
	shop, ok := store.Shop(1)
	if !ok || shop.Name() != "General Store" || shop.Currency() != "COINS" || shop.Source() != "core" {
		t.Fatalf("shop = %+v ok=%v", shop, ok)
	}
	if shop.DefaultRestockTicks() != 5 || shop.DefaultDestockTicks() != 6 || shop.SoldItemDestockTicks() != 7 {
		t.Fatalf("ticks = %d/%d/%d", shop.DefaultRestockTicks(), shop.DefaultDestockTicks(), shop.SoldItemDestockTicks())
	}
	policy := shop.SalesPolicy()
	if policy.GeneralStore == nil || !*policy.GeneralStore || policy.BuysNothing != nil ||
		policy.SellsAt == nil || *policy.SellsAt != 1000 || *policy.BuysAt != 500 || *policy.ChangePer != 10 {
		t.Fatalf("policy = %+v", policy)
	}
	stock := shop.OriginalStock()
	if len(stock) != 5 {
		t.Fatalf("stock = %+v", stock)
	}
	// Row 0: explicit amount, restock, price from the map.
	if stock[0].ID != 2 || stock[0].Amount != 5 || stock[0].RestockTicks == nil || *stock[0].RestockTicks != 3 ||
		stock[0].Price == nil || *stock[0].Price != 50 {
		t.Fatalf("row 0 = %+v", stock[0])
	}
	// Row 1: listed 0 is real and kept.
	if stock[1].ID != 4 || stock[1].Amount != 0 {
		t.Fatalf("row 1 = %+v", stock[1])
	}
	// Row 2: omitted amount falls back to stockAmount.
	if stock[2].ID != 3 || stock[2].Amount != 10 {
		t.Fatalf("row 2 = %+v", stock[2])
	}
	// Items: numeric id and resolved name both append; the unknown
	// name is skipped.
	if stock[3].ID != 2 || stock[3].Amount != 10 || stock[3].Price == nil || *stock[3].Price != 50 {
		t.Fatalf("row 3 = %+v", stock[3])
	}
	if stock[4].ID != 2 || stock[4].Amount != 10 {
		t.Fatalf("row 4 = %+v", stock[4])
	}
	if _, ok := store.Shop(2); ok {
		t.Fatal("shop 2 must not exist")
	}
}

func TestShopDefaults(t *testing.T) {
	store := loadShops(t, shopFiles(`[{"id": 9}]`, `[]`), LoadOptions{})
	shop, ok := store.Shop(9)
	if !ok || shop.Name() != "Shop" || shop.Currency() != "COINS" {
		t.Fatalf("shop = %+v", shop)
	}
	if shop.DefaultRestockTicks() != 4 || shop.DefaultDestockTicks() != 4 || shop.SoldItemDestockTicks() != 100 {
		t.Fatalf("ticks = %d/%d/%d", shop.DefaultRestockTicks(), shop.DefaultDestockTicks(), shop.SoldItemDestockTicks())
	}
	if len(shop.OriginalStock()) != 0 {
		t.Fatalf("stock = %v", shop.OriginalStock())
	}
}

func TestParseShopTicks(t *testing.T) {
	cases := []struct {
		input any
		want  int
		ok    bool
	}{
		{5.0, 5, true},
		{5.9, 5, true},
		{0.0, 0, false},
		{-1.0, 0, false},
		{"5t", 5, true},
		{"5T", 5, true},
		{"restock every 5t", 5, true},
		{"2 minutes", 200, true},
		{"1 minute", 100, true},
		{"1.5m", 150, true},
		{"90 seconds", 150, true},
		{"1 second", 2, true},
		{"0.5s", 1, true},
		{"abc", 0, false},
		{"5", 0, false},
		{"", 0, false},
		{nil, 0, false},
		{true, 0, false},
	}
	for _, tc := range cases {
		got := parseShopTicks(tc.input)
		if tc.ok {
			if got == nil || *got != tc.want {
				t.Fatalf("parseShopTicks(%v) = %v, want %d", tc.input, got, tc.want)
			}
		} else if got != nil {
			t.Fatalf("parseShopTicks(%v) = %d, want nil", tc.input, *got)
		}
	}
}

func TestPerMille(t *testing.T) {
	if got := perMille(1000.0); got == nil || *got != 1000 {
		t.Fatalf("perMille(1000) = %v", got)
	}
	if got := perMille(0.0); got == nil || *got != 0 {
		t.Fatalf("perMille(0) = %v", got)
	}
	for _, bad := range []any{-1.0, 1.5, "1000", nil, true} {
		if got := perMille(bad); got != nil {
			t.Fatalf("perMille(%v) = %d, want nil", bad, *got)
		}
	}
}

func TestShopCurrencyNormalization(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"coins", "COINS"},
		{"blood money", "BLOOD_MONEY"},
		{" Blood  Money ", "BLOOD_MONEY"},
		{"blood_money", "BLOOD_MONEY"},
		{"tokkul", "TOKKUL"},
		{"", "COINS"},
		{"   ", "COINS"},
	}
	for _, tc := range cases {
		store := loadShops(t, shopFiles(`[{"id": 1, "currency": "`+tc.in+`"}]`, `[]`), LoadOptions{})
		shop, _ := store.Shop(1)
		if shop.Currency() != tc.want {
			t.Fatalf("currency %q = %q, want %q", tc.in, shop.Currency(), tc.want)
		}
	}
}

func TestShopRequiresPresets(t *testing.T) {
	shops := `[{"id": 1, "currency": "tokkul", "requiresPresets": true,
		"prices": {"2": 50}, "originalStock": [{"id": 2, "amount": 1}]}]`
	// Flag off: prices are dropped and the currency resets.
	store := loadShops(t, shopFiles(shops, `[]`), LoadOptions{})
	shop, _ := store.Shop(1)
	if shop.Currency() != "COINS" || shop.OriginalStock()[0].Price != nil {
		t.Fatalf("preset-off: currency=%s price=%v", shop.Currency(), shop.OriginalStock()[0].Price)
	}
	// Flag on: presets apply.
	store = loadShops(t, shopFiles(shops, `[]`), LoadOptions{PresetShopPricesEnabled: true})
	shop, _ = store.Shop(1)
	if shop.Currency() != "TOKKUL" || shop.OriginalStock()[0].Price == nil || *shop.OriginalStock()[0].Price != 50 {
		t.Fatalf("preset-on: currency=%s price=%v", shop.Currency(), shop.OriginalStock()[0].Price)
	}
}

func TestShopNameResolution(t *testing.T) {
	// First-wins: both id 2 and id 3 normalize to "bronze sword";
	// the lower id wins.
	store := loadShops(t, shopFiles(`[{"id": 1, "stockAmount": 1, "items": ["bronze swords", "Bronze Sword's", "bronze  sword"]}]`, `[]`), LoadOptions{})
	shop, _ := store.Shop(1)
	stock := shop.OriginalStock()
	if len(stock) != 3 {
		t.Fatalf("stock = %v", stock)
	}
	for i, row := range stock {
		if row.ID != 2 {
			t.Fatalf("row %d = %d, want 2", i, row.ID)
		}
	}
	// Name-keyed prices resolve the same way.
	store = loadShops(t, shopFiles(`[{"id": 1, "stockAmount": 1, "prices": {"Bronze sword": 60}, "items": [2]}]`, `[]`), LoadOptions{})
	shop, _ = store.Shop(1)
	if price := shop.OriginalStock()[0].Price; price == nil || *price != 60 {
		t.Fatalf("name-keyed price = %v", price)
	}
	// With no cache, names never resolve.
	dir := fixtureDir(t, shopFiles(`[{"id": 1, "stockAmount": 1, "items": ["Bronze sword", 2]}]`, `[]`))
	nilStore, err := Load(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	nilShop, _ := nilStore.Shop(1)
	if len(nilShop.OriginalStock()) != 1 || nilShop.OriginalStock()[0].ID != 2 {
		t.Fatalf("nil-cache stock = %v", nilShop.OriginalStock())
	}
}

func TestShopItemsCommaString(t *testing.T) {
	// A comma-split string is a name list: numeric strings resolve
	// by name only, so "2, 4" yields nothing.
	store := loadShops(t, shopFiles(`[{"id": 1, "stockAmount": 2, "items": "2, 4"}]`, `[]`), LoadOptions{})
	shop, _ := store.Shop(1)
	if len(shop.OriginalStock()) != 0 {
		t.Fatalf("stock = %v", shop.OriginalStock())
	}
	store = loadShops(t, shopFiles(`[{"id": 1, "stockAmount": 1, "items": "Bronze sword, Untradable thing"}]`, `[]`), LoadOptions{})
	shop, _ = store.Shop(1)
	stock := shop.OriginalStock()
	if len(stock) != 2 || stock[0].ID != 2 || stock[1].ID != 4 {
		t.Fatalf("stock = %v", stock)
	}
}

func TestShopInvalidEntriesSkipped(t *testing.T) {
	var logs logCapture
	store := loadShops(t, shopFiles(`[
		{"id": "x", "name": "Bad"},
		{"id": -1, "name": "Negative"},
		"not-an-object",
		{"id": 1, "name": "Good"}
	]`, `[]`), LoadOptions{Logf: logs.logf})
	if len(store.Shops()) != 1 {
		t.Fatalf("shops = %d", len(store.Shops()))
	}
	if !logs.contains("invalid=3") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestShopFilesRequired(t *testing.T) {
	for _, tc := range []struct {
		name  string
		files map[string]string
	}{
		{"missing shops", withoutFiles("shops.json")},
		{"missing custom", withoutFiles("custom-shops.json")},
		{"malformed shops", withFiles(map[string]string{"shops.json": `[oops`})},
		{"malformed custom", withFiles(map[string]string{"custom-shops.json": `{bad`})},
		{"non-array shops", withFiles(map[string]string{"shops.json": `{}`})},
		{"non-array custom", withFiles(map[string]string{"custom-shops.json": `{"a":1}`})},
	} {
		defs, _ := newFakeCache(t, shopCacheSpec())
		dir := fixtureDir(t, tc.files)
		if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
			t.Fatalf("%s: Load must fail", tc.name)
		}
	}
}

func TestShopSourceOrderingAndReplacement(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(shopsDefinitionType, "editor", Source{
		Name:     "custom",
		Priority: IntPriority(10),
		Load: func() (any, error) {
			return []any{
				map[string]any{"id": 1.0, "name": "Replaced"},
				map[string]any{"id": 2.0, "name": "Added"},
			}, nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := loadShops(t, shopFiles(`[{"id": 1, "name": "Core"}, {"id": 3, "name": "Third"}]`, `[]`),
		LoadOptions{Sources: registry})
	shops := store.Shops()
	if len(shops) != 3 {
		t.Fatalf("shops = %d", len(shops))
	}
	// Replacement keeps the first-insertion position. The source
	// recorded on the shop is the source name, not the owner.
	if shops[0].ID() != 1 || shops[0].Name() != "Replaced" || shops[0].Source() != "custom" {
		t.Fatalf("shops[0] = id=%d name=%q source=%q", shops[0].ID(), shops[0].Name(), shops[0].Source())
	}
	if shops[1].ID() != 3 || shops[2].ID() != 2 {
		t.Fatalf("order = %d/%d/%d", shops[0].ID(), shops[1].ID(), shops[2].ID())
	}
}
