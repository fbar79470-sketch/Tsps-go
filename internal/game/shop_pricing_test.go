package game

import (
	"testing"
)

// shopFixtureDefs is a ShopItemDefinitionSource test double: the
// shared testItemDefs base (stackable/tradeable/sellable/un-note)
// plus the shop-only accessors (name/examine/value/GE/alch).
type shopFixtureDefs struct {
	*testItemDefs
	names           map[int]string
	examines        map[int]string
	values          map[int]int
	bloodMoney      map[int]int
	geValues        map[int]int
	hasGE           map[int]bool
	highAlch        map[int]int
	hasHigh         map[int]bool
	lowAlch         map[int]int
	hasLow          map[int]bool
	deathComponents map[int][]int
}

func (s *shopFixtureDefs) ItemName(id int) string         { return s.names[id] }
func (s *shopFixtureDefs) ItemExamine(id int) string      { return s.examines[id] }
func (s *shopFixtureDefs) ItemValue(id int) int           { return s.values[id] }
func (s *shopFixtureDefs) ItemBloodMoneyValue(id int) int { return s.bloodMoney[id] }
func (s *shopFixtureDefs) ItemHasGrandExchangeValue(id int) bool {
	return s.hasGE[id]
}
func (s *shopFixtureDefs) ItemGrandExchangeValue(id int) int { return s.geValues[id] }
func (s *shopFixtureDefs) ItemHighAlchValue(id int) int      { return s.highAlch[id] }
func (s *shopFixtureDefs) ItemHasHighAlchValue(id int) bool  { return s.hasHigh[id] }
func (s *shopFixtureDefs) ItemLowAlchValue(id int) int       { return s.lowAlch[id] }
func (s *shopFixtureDefs) ItemHasLowAlchValue(id int) bool   { return s.hasLow[id] }
func (s *shopFixtureDefs) DeathComponents(id int) []int      { return s.deathComponents[id] }

// newShopFixtureDefs returns the shared shop test definitions:
//
//	100 Iron sword: value 100, GE 150, HA 60 / LA 40 (set)
//	101 Feather: value 3, stackable, no GE quote, alch unset
//	102 Bond: value 1000, HA/LA 0 (set — alchemy refuses it)
//	103 Untradeable helm: value 50, not tradeable/sellable
//	104 Noted sword: note of 100
//	200 Raw chicken: value 10
//	201 Pot: value 5
//	300 Tokkul: value 1, stackable
//	400 PK sword: value 500, blood money value 7
//	995 Coins: value 1, stackable
//	13307 Blood money: value 1, stackable
func newShopFixtureDefs() *shopFixtureDefs {
	return &shopFixtureDefs{
		testItemDefs: &testItemDefs{
			stackable: map[int]bool{101: true, 300: true, 995: true, 13307: true},
			tradeable: map[int]bool{100: true, 101: true, 102: true, 104: true, 200: true, 201: true, 300: true, 400: true, 995: true, 13307: true},
			sellable:  map[int]bool{100: true, 101: true, 102: true, 104: true, 200: true, 201: true, 300: true, 400: true, 995: true, 13307: true},
			dropable:  map[int]bool{100: true, 101: true, 102: true, 104: true, 200: true, 201: true, 300: true, 400: true, 995: true, 13307: true},
			unNote:    map[int]int{104: 100},
		},
		names: map[int]string{
			100: "Iron sword", 101: "Feather", 102: "Bond", 103: "Untradeable helm",
			104: "Iron sword", 200: "Raw chicken", 201: "Pot", 300: "Tokkul",
			400: "PK sword", 995: "Coins", 13307: "Blood money",
		},
		examines: map[int]string{
			100: "A sword.", 101: "A feather.", 200: "A raw chicken.",
		},
		values: map[int]int{
			100: 100, 101: 3, 102: 1000, 103: 50, 104: 100, 200: 10,
			201: 5, 300: 1, 400: 500, 995: 1, 13307: 1,
		},
		bloodMoney: map[int]int{400: 7},
		geValues:   map[int]int{100: 150},
		hasGE:      map[int]bool{100: true},
		highAlch:   map[int]int{100: 60, 102: 0},
		hasHigh:    map[int]bool{100: true, 102: true},
		lowAlch:    map[int]int{100: 40, 102: 0},
		hasLow:     map[int]bool{100: true, 102: true},
	}
}

func shopIntPtr(v int) *int    { return &v }
func shopBoolPtr(v bool) *bool { return &v }

// shopTestSender records shop interface traffic.
type shopTestSender struct {
	opens   []ShopOpenView
	closes  int
	prompts []string
	// promptExecute, when set, runs the prompt's execute
	// immediately with promptAmount (simulating the player
	// answering the amount prompt).
	promptExecute func(execute func(amount int))
	promptAmount  int
}

func (s *shopTestSender) SendShopOpen(p *Player, view ShopOpenView) { s.opens = append(s.opens, view) }
func (s *shopTestSender) SendShopClose(p *Player)                   { s.closes++ }
func (s *shopTestSender) SendEnterAmountPrompt(p *Player, prompt string, execute func(amount int)) {
	s.prompts = append(s.prompts, prompt)
	if s.promptExecute != nil {
		s.promptExecute(execute)
	}
}

// newShopTestPlayer returns a player with the fixture
// definitions installed and a fake session recording messages.
func newShopTestPlayer(defs ShopItemDefinitionSource) (*Player, *fakeSession) {
	p := NewPlayer("shopper", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	sess := newFakeSession()
	p.Session = sess
	return p, sess
}

func legacyCoinShop() ShopDefinition {
	return ShopDefinition{
		ID: 1, Name: "Sword Shop", Currency: "COINS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
		Source:               "core",
	}
}

func ratedShop() ShopDefinition {
	return ShopDefinition{
		ID: 2, Name: "General Store", Currency: "COINS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10}, {ID: 200, Amount: 5}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
		Source:               "core",
		SalesPolicy: ShopSalesPolicy{
			SellsAt:   shopIntPtr(1000),
			BuysAt:    shopIntPtr(600),
			ChangePer: shopIntPtr(20),
		},
	}
}

func TestLegacyPricing(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)

	buy, ok := mgr.BuyPrice(p, 1, 100)
	if !ok || buy != 100 {
		t.Fatalf("BuyPrice = %d, %v, want 100, true", buy, ok)
	}
	sell, ok := mgr.SellPrice(p, 1, 100)
	if !ok || sell != 85 {
		t.Fatalf("SellPrice = %d, %v, want 85 (floor(100*0.85)), true", sell, ok)
	}
	// A price of 1 pays in full (the buyPrice > 1 rule).
	sell, _ = mgr.SellPrice(p, 1, 995)
	if sell != 1 {
		t.Fatalf("SellPrice(coins) = %d, want 1", sell)
	}
}

func TestRatedPricingAtNormalStock(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	p, _ := newShopTestPlayer(defs)

	buy, _ := mgr.BuyPrice(p, 2, 100)
	if buy != 100 {
		t.Fatalf("BuyPrice at normal stock = %d, want 100", buy)
	}
	sell, _ := mgr.SellPrice(p, 2, 100)
	if sell != 60 {
		t.Fatalf("SellPrice at normal stock = %d, want 60", sell)
	}
}

func TestRatedPricingFollowsStock(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	p, _ := newShopTestPlayer(defs)

	// Overstocked: stock 12 of 10 (offset +2) — buy cheaper,
	// sell cheaper.
	mgr.AddShopStock(2, 100, 2)
	if buy, _ := mgr.BuyPrice(p, 2, 100); buy != 96 {
		t.Fatalf("BuyPrice overstocked = %d, want 96", buy)
	}
	if sell, _ := mgr.SellPrice(p, 2, 100); sell != 56 {
		t.Fatalf("SellPrice overstocked = %d, want 56", sell)
	}

	// Understocked: drain to 8 via the shop's own take path
	// (buy 4 with ample coins).
	p.Inventory().Adds(995, 100000)
	mgr.Open(p, 2, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 4})
	if buy, _ := mgr.BuyPrice(p, 2, 100); buy != 104 {
		t.Fatalf("BuyPrice understocked = %d, want 104", buy)
	}
	if sell, _ := mgr.SellPrice(p, 2, 100); sell != 64 {
		t.Fatalf("SellPrice understocked = %d, want 64", sell)
	}
}

func TestRatedPricingBoughtSoldOffsets(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	shop := mgr.shops[2]
	def := mgr.itemDef(100)

	// Buying: each unit already taken lowers the offset, raising
	// the next unit's price.
	if got := mgr.buyPrice(nil, shop, def, 1); got != 102 {
		t.Fatalf("buyPrice(bought=1) = %d, want 102", got)
	}
	// Selling: each unit already handed over raises the offset,
	// lowering the next unit's price.
	if got := mgr.sellPrice(shop, def, nil, 1); got != 58 {
		t.Fatalf("sellPrice(sold=1) = %d, want 58", got)
	}
}

func TestRatedPricingClamps(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 3, Name: "Volatile Shop", Currency: "COINS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
		SalesPolicy: ShopSalesPolicy{
			SellsAt: shopIntPtr(1000), BuysAt: shopIntPtr(600), ChangePer: shopIntPtr(500),
		},
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, _ := newShopTestPlayer(defs)

	// Deeply overstocked: the buy formula goes negative, the
	// floor is max(1, 10% of value, sellsAt-100% rate) = 10.
	mgr.AddShopStock(3, 100, 20)
	if buy, _ := mgr.BuyPrice(p, 3, 100); buy != 10 {
		t.Fatalf("BuyPrice floored = %d, want 10", buy)
	}
	if sell, _ := mgr.SellPrice(p, 3, 100); sell != 10 {
		t.Fatalf("SellPrice floored = %d, want 10", sell)
	}

	// Sold out: the buy formula exceeds the cap (value x
	// (sellsAt + 500%) = 600); the sell formula exceeds value
	// plus the buy rate (160).
	mgr2 := NewShopManager(defs)
	empty := shop
	empty.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 10}}
	mgr2.Initialize([]ShopDefinition{empty})
	p2, _ := newShopTestPlayer(defs)
	p2.Inventory().Adds(995, 1000000)
	mgr2.Open(p2, 3, true, DefaultShopTargetUID)
	mgr2.HandleItemContainerAction(p2, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 10})
	if buy, _ := mgr2.BuyPrice(p2, 3, 100); buy != 600 {
		t.Fatalf("BuyPrice capped = %d, want 600", buy)
	}
	if sell, _ := mgr2.SellPrice(p2, 3, 100); sell != 160 {
		t.Fatalf("SellPrice capped = %d, want 160", sell)
	}
}

func TestPerItemPriceOverride(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ratedShop()
	shop.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(250)}}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, _ := newShopTestPlayer(defs)

	// The override is the buy price as it stands (rated buy is
	// null); the sell price still uses the rated formula from
	// the item's value — ratedSellPrice has no override check.
	if buy, _ := mgr.BuyPrice(p, 2, 100); buy != 250 {
		t.Fatalf("BuyPrice override = %d, want 250", buy)
	}
	if sell, _ := mgr.SellPrice(p, 2, 100); sell != 60 {
		t.Fatalf("SellPrice override shop = %d, want 60 (rated)", sell)
	}

	// Without rates, the override prices both sides (sell via
	// the 85% rule on the override).
	legacy := legacyCoinShop()
	legacy.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(250)}}
	mgr2 := NewShopManager(defs)
	mgr2.Initialize([]ShopDefinition{legacy})
	if buy, _ := mgr2.BuyPrice(p, 1, 100); buy != 250 {
		t.Fatalf("BuyPrice legacy override = %d, want 250", buy)
	}
	if sell, _ := mgr2.SellPrice(p, 1, 100); sell != 212 {
		t.Fatalf("SellPrice legacy override = %d, want 212", sell)
	}
}

func TestBloodMoneyPricing(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 13, Name: "PK Shop", Currency: "BLOOD_MONEY",
		OriginalStock:        []ShopStockEntry{{ID: 400, Amount: 5}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, _ := newShopTestPlayer(defs)

	if buy, _ := mgr.BuyPrice(p, 13, 400); buy != 7 {
		t.Fatalf("BuyPrice blood money = %d, want 7", buy)
	}
	if sell, _ := mgr.SellPrice(p, 13, 400); sell != 5 {
		t.Fatalf("SellPrice blood money = %d, want 5 (floor(7*0.85))", sell)
	}
}

func TestSpecialistBuybackPricing(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 5, Name: "Grace's Graceful Clothing", Currency: "MARKS OF GRACE",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(35)}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
		SalesPolicy:          ShopSalesPolicy{BuysAt: shopIntPtr(800)},
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, _ := newShopTestPlayer(defs)

	// A graceful hood bought for 35 marks sells for 28 (80%).
	if buy, _ := mgr.BuyPrice(p, 5, 100); buy != 35 {
		t.Fatalf("BuyPrice specialist = %d, want 35", buy)
	}
	if sell, _ := mgr.SellPrice(p, 5, 100); sell != 28 {
		t.Fatalf("SellPrice specialist = %d, want 28", sell)
	}
}

func TestZeroValueItemRatesZero(t *testing.T) {
	defs := newShopFixtureDefs()
	defs.values[200] = 0
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	shop := mgr.shops[2]
	def := mgr.itemDef(200)
	// ratedBuyPrice returns (0, true) for a valueless item — the
	// buy price is 0, and buyItem refuses it as unpriced via
	// itemPrice, not via the rated path.
	if price, ok := mgr.ratedBuyPrice(shop, def, 0); !ok || price != 0 {
		t.Fatalf("ratedBuyPrice valueless = %d, %v, want 0, true", price, ok)
	}
	if price, ok := mgr.ratedSellPrice(shop, def, 0); !ok || price != 0 {
		t.Fatalf("ratedSellPrice valueless = %d, %v, want 0, true", price, ok)
	}
}

func TestBuySellPriceHooks(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)

	mgr.Hooks.OnBuyPrice = func(e *ShopBuyPriceEvent) { e.Price = 0 }
	if buy, _ := mgr.BuyPrice(p, 1, 100); buy != 0 {
		t.Fatalf("BuyPrice with free hook = %d, want 0", buy)
	}
	mgr.Hooks.OnBuyPrice = func(e *ShopBuyPriceEvent) { e.Price = -5 }
	if buy, _ := mgr.BuyPrice(p, 1, 100); buy != 100 {
		t.Fatalf("BuyPrice with invalid hook = %d, want 100 (hook ignored)", buy)
	}
	mgr.Hooks.OnBuyPrice = nil

	mgr.Hooks.OnSellPrice = func(e *ShopSellPriceEvent) {
		if e.BuyPrice != 100 {
			t.Fatalf("sell-price hook BuyPrice = %d, want 100", e.BuyPrice)
		}
		e.Price = 99
	}
	if sell, _ := mgr.SellPrice(p, 1, 100); sell != 99 {
		t.Fatalf("SellPrice with hook = %d, want 99", sell)
	}
}

func TestPriceCheckMessages(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, sess := newShopTestPlayer(defs)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	// From the shop: unit-word form for coins.
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100})
	if len(sess.messages) != 1 || sess.messages[0] != "Iron sword: currently costs 100 coins." {
		t.Fatalf("buy price check = %q, want Iron sword: currently costs 100 coins.", sess.messages)
	}

	// From the inventory: the sell form, 85% of value.
	p.Inventory().Adds(100, 1)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100})
	if len(sess.messages) != 2 || sess.messages[1] != "Iron sword: shop will buy for 85 coins." {
		t.Fatalf("sell price check = %q", sess.messages)
	}

	// One coin uses the singular.
	defs.values[200] = 1
	mgr2 := NewShopManager(defs)
	oneCoin := legacyCoinShop()
	oneCoin.OriginalStock = []ShopStockEntry{{ID: 200, Amount: 10}}
	mgr2.Initialize([]ShopDefinition{oneCoin})
	p2, sess2 := newShopTestPlayer(defs)
	mgr2.Open(p2, 1, true, DefaultShopTargetUID)
	mgr2.HandleItemContainerAction(p2, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 200})
	if len(sess2.messages) != 1 || sess2.messages[0] != "Raw chicken: currently costs 1 coin." {
		t.Fatalf("singular coin check = %q", sess2.messages)
	}
}

func TestPriceCheckNoValueAndRefusals(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop(), generalStoreShop()})
	p, sess := newShopTestPlayer(defs)

	// Selling coins to a general store (which buys anything):
	// the shop's own currency has no value. A specialist shop
	// refuses coins earlier, at buysItem.
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	p.Inventory().Adds(995, 10)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 995})
	if len(sess.messages) != 1 || sess.messages[0] != "This item has no value." {
		t.Fatalf("own-currency check = %q", sess.messages)
	}

	// Selling an unlisted item to a specialist shop.
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	p.Inventory().Adds(200, 1)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopInventoryInterfaceID, Slot: 1, ItemID: 200})
	if len(sess.messages) != 2 || sess.messages[1] != "You can't sell this item to this shop." {
		t.Fatalf("unlisted check = %q", sess.messages)
	}
}

func TestPriceCheckNonUnitCurrency(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 6, Name: "Reward Shop", Currency: "POINTS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(1200)}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, sess := newShopTestPlayer(defs)
	mgr.Open(p, 6, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100})
	// POINTS has no unit word: the "x Currency" form, with
	// comma grouping.
	if len(sess.messages) != 1 || sess.messages[0] != "Iron sword currently costs 1,200 x Points." {
		t.Fatalf("points check = %q", sess.messages)
	}
}

func TestInsertCommas(t *testing.T) {
	cases := map[int]string{0: "0", 5: "5", 400: "400", 1000: "1,000", 1234567: "1,234,567", 1000000: "1,000,000"}
	for value, want := range cases {
		if got := insertCommas(value); got != want {
			t.Fatalf("insertCommas(%d) = %q, want %q", value, got, want)
		}
	}
}

func TestActionAmount(t *testing.T) {
	cases := []struct {
		button int
		option string
		want   int
		ok     bool
	}{
		{2, "", 1, true}, {3, "", 5, true}, {4, "", 10, true}, {5, "", 50, true},
		{1, "", 0, false}, {10, "", 0, false},
		{1, "Buy 1", 1, true}, {1, "Buy 5", 5, true}, {1, "Sell 10", 10, true},
		{1, "Sell-50", 50, true}, {1, "buy 5", 5, true}, {1, "  Buy 10  ", 10, true},
		{1, "Buy 7", 0, false}, {1, "Value", 0, false}, {3, "Buy 1", 1, true},
	}
	for _, tc := range cases {
		got, ok := ActionAmount(tc.button, tc.option)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ActionAmount(%d, %q) = %d, %v, want %d, %v", tc.button, tc.option, got, ok, tc.want, tc.ok)
		}
	}
}
