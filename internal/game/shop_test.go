package game

import (
	"testing"
)

func generalStoreShop() ShopDefinition {
	return ShopDefinition{
		ID: 0, Name: "Lumbridge General Store", Currency: "COINS",
		OriginalStock:        []ShopStockEntry{{ID: 201, Amount: 10}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 2,
		Source:               "core",
	}
}

func TestShopInitializeAndReload(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	if n := mgr.Initialize([]ShopDefinition{legacyCoinShop(), ratedShop()}); n != 2 {
		t.Fatalf("Initialize = %d, want 2", n)
	}
	if !mgr.HasShop(1) || !mgr.HasShop(2) || mgr.HasShop(99) {
		t.Fatal("HasShop mismatch after Initialize")
	}
	if got := mgr.ShopIDs(); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("ShopIDs = %v, want [1 2]", got)
	}
	def, ok := mgr.ShopDefinition(1)
	if !ok || def.Name != "Sword Shop" || def.Currency != "COINS" {
		t.Fatalf("ShopDefinition(1) = %+v, %v", def, ok)
	}
	// Reload replaces the set; stock resets to the definition.
	mgr.AddShopStock(1, 100, 5)
	if n := mgr.Reload([]ShopDefinition{legacyCoinShop()}); n != 1 {
		t.Fatalf("Reload = %d, want 1", n)
	}
	entries, _ := mgr.DisplayEntries(1, nil)
	if len(entries) != 1 || entries[0].Amount != 10 {
		t.Fatalf("reloaded stock = %+v, want one entry of 10", entries)
	}
}

func TestShopRegisterAndRemove(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender

	extra := ShopDefinition{ID: 77, Name: "Rogue Trader", Currency: "COINS",
		OriginalStock:       []ShopStockEntry{{ID: 200, Amount: 3}},
		DefaultRestockTicks: 4, DefaultDestockTicks: 4, SoldItemDestockTicks: 100}
	mgr.RegisterShop(extra)
	if !mgr.HasShop(77) {
		t.Fatal("HasShop(77) false after RegisterShop")
	}
	p, _ := newShopTestPlayer(defs)
	if !mgr.Open(p, 77, true, DefaultShopTargetUID) {
		t.Fatal("Open(77) failed")
	}
	mgr.RemoveShop(77)
	if mgr.HasShop(77) {
		t.Fatal("HasShop(77) true after RemoveShop")
	}
	if _, ok := mgr.ActiveShopID(p); ok {
		t.Fatal("viewer still has an active shop after RemoveShop")
	}
	if sender.closes != 1 {
		t.Fatalf("closes = %d, want 1", sender.closes)
	}
}

func TestShopOpenClose(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)

	if mgr.Open(p, 99, true, DefaultShopTargetUID) {
		t.Fatal("Open(unknown) reported true")
	}
	if mgr.Open(nil, 1, true, DefaultShopTargetUID) {
		t.Fatal("Open(nil player) reported true")
	}
	if !mgr.Open(p, 1, true, DefaultShopTargetUID) {
		t.Fatal("Open failed")
	}
	if id, ok := mgr.ActiveShopID(p); !ok || id != 1 {
		t.Fatalf("ActiveShopID = %d, %v, want 1, true", id, ok)
	}
	if len(sender.opens) != 1 {
		t.Fatalf("opens = %d, want 1", len(sender.opens))
	}
	view := sender.opens[0]
	if view.ShopID != 1 || view.Title != "Sword Shop" || view.CurrencyItemID != 995 || view.GeneralStore {
		t.Fatalf("open view = %+v", view)
	}
	if !view.Opening {
		t.Fatal("first open should report Opening")
	}
	if len(view.Stock) != 1 || view.Stock[0].ItemID != 100 || view.Stock[0].Quantity != 10 ||
		view.Stock[0].DefaultQuantity != 10 || view.Stock[0].PriceEach != 100 || view.Stock[0].SellPrice != 85 {
		t.Fatalf("open stock = %+v", view.Stock)
	}

	// A second open for another shop closes the first (a close
	// frame is sent) and opens the new one.
	mgr.RegisterShop(ratedShop())
	mgr.Open(p, 2, true, DefaultShopTargetUID)
	if id, _ := mgr.ActiveShopID(p); id != 2 {
		t.Fatalf("ActiveShopID after reopen = %d, want 2", id)
	}
	if sender.closes != 1 {
		t.Fatalf("closes = %d, want 1", sender.closes)
	}

	mgr.Close(p)
	if _, ok := mgr.ActiveShopID(p); ok {
		t.Fatal("active shop survives Close")
	}
	if sender.closes != 2 {
		t.Fatalf("closes = %d, want 2", sender.closes)
	}
	mgr.Close(p) // No-op: no active shop, no second close frame.
	if sender.closes != 2 {
		t.Fatalf("closes after double Close = %d, want 2", sender.closes)
	}
}

func TestShopOpenTitleStripsDisambiguation(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	shop := legacyCoinShop()
	shop.Name = "Oziach (shop)"
	mgr.Initialize([]ShopDefinition{shop})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	if sender.opens[0].Title != "Oziach" {
		t.Fatalf("title = %q, want Oziach", sender.opens[0].Title)
	}
}

func TestShopCanShopGate(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	deny := false
	mgr.Hooks.CanShop = func(player *Player, shopID int) *bool { return &deny }
	p, _ := newShopTestPlayer(defs)
	// A denial reports true (handled) without opening.
	if !mgr.Open(p, 1, true, DefaultShopTargetUID) {
		t.Fatal("denied Open reported false, want true")
	}
	if _, ok := mgr.ActiveShopID(p); ok {
		t.Fatal("denied open left an active shop")
	}
	if len(sender.opens) != 0 {
		t.Fatal("denied open sent frames")
	}
	mgr.Hooks.CanShop = func(player *Player, shopID int) *bool { return nil }
	if !mgr.Open(p, 1, true, DefaultShopTargetUID) {
		t.Fatal("no-opinion Open failed")
	}
}

func TestShopOpenedHook(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	var opened *ShopOpenedEvent
	mgr.Hooks.OnShopOpened = func(e *ShopOpenedEvent) { opened = e }
	p, _ := newShopTestPlayer(defs)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	if opened == nil || opened.ShopID != 1 || opened.ShopName != "Sword Shop" || opened.Player != p {
		t.Fatalf("opened event = %+v", opened)
	}
}

func TestDisplayEntriesSoldOutKeepsSlot(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	shop := legacyCoinShop()
	shop.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 0}, {ID: 200, Amount: 5}}
	mgr.Initialize([]ShopDefinition{shop})
	entries, _ := mgr.DisplayEntries(1, nil)
	if len(entries) != 2 || entries[0].ItemID != 100 || entries[0].Amount != 0 {
		t.Fatalf("entries = %+v, want sold-out 100 at slot 0", entries)
	}
	// A sold-in item leaves when its pile empties.
	mgr.AddShopStock(1, 201, 3)
	entries, _ = mgr.DisplayEntries(1, nil)
	if len(entries) != 3 {
		t.Fatalf("entries after add = %+v", entries)
	}
	mgr.removeStock(mgr.shops[1], 201, 3)
	entries, _ = mgr.DisplayEntries(1, nil)
	if len(entries) != 2 {
		t.Fatalf("entries after drain = %+v, want sold-in item gone", entries)
	}
}

func TestStockVisibleHookHidesSlot(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	mgr.Hooks.OnStockVisible = func(e *ShopStockVisibleEvent) {
		if e.ItemID == 100 {
			e.Visible = false
		}
	}
	entries, _ := mgr.DisplayEntries(2, nil)
	if len(entries) != 1 || entries[0].ItemID != 200 {
		t.Fatalf("entries = %+v, want only 200", entries)
	}
	// Hidden stock cannot be bought: display slot 0 is now 200,
	// and buying it buys 200, not the hidden 100.
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 1000)
	mgr.Open(p, 2, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if got := p.Inventory().GetAmount(200); got != 1 {
		t.Fatalf("bought 200 = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(100); got != 0 {
		t.Fatalf("bought hidden 100 = %d, want 0", got)
	}
}

func TestBuyItemSuccessAndPartial(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 250)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	sender.opens = nil

	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 5})
	// 250 coins afford 2 swords at 100 each.
	if got := p.Inventory().GetAmount(100); got != 2 {
		t.Fatalf("swords = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(995); got != 50 {
		t.Fatalf("coins = %d, want 50", got)
	}
	entries, _ := mgr.DisplayEntries(1, p)
	if entries[0].Amount != 8 {
		t.Fatalf("stock = %d, want 8", entries[0].Amount)
	}
	if len(sender.opens) != 1 {
		t.Fatalf("refresh opens = %d, want 1", len(sender.opens))
	}
	if !mgr.RestockTaskRunning() {
		t.Fatal("restock task not ensured after a purchase")
	}
}

func TestBuyItemRefusals(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	shop := legacyCoinShop()
	shop.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 0}, {ID: 101, Amount: 5}}
	mgr.Initialize([]ShopDefinition{shop})
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 1000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	// Out of stock (listed at 0).
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "This item is currently out of stock. Come back later." {
		t.Fatalf("messages = %q", sess.messages)
	}

	// Unaffordable: feathers cost 3 each; drain coins first.
	p.Inventory().DeleteNumber(995, 999)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 1, ItemID: 101, Amount: 1})
	if len(sess.messages) != 2 || sess.messages[1] != "You don't have enough coins." {
		t.Fatalf("messages = %q", sess.messages)
	}
	if got := p.Inventory().GetAmount(101); got != 0 {
		t.Fatalf("feathers = %d, want 0", got)
	}
}

func TestBuyItemUnpricedIsSilent(t *testing.T) {
	defs := newShopFixtureDefs()
	defs.values[200] = 0
	mgr := NewShopManager(defs)
	shop := legacyCoinShop()
	shop.OriginalStock = []ShopStockEntry{{ID: 200, Amount: 5}}
	mgr.Initialize([]ShopDefinition{shop})
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 1000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 200, Amount: 1})
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %q, want none", sess.messages)
	}
	if got := p.Inventory().GetAmount(200); got != 0 {
		t.Fatalf("bought = %d, want 0", got)
	}
}

func TestBuyItemFullInventory(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, sess := newShopTestPlayer(defs)
	// Fill the inventory with unstackable filler, plus coins
	// stacked in one slot — no free slot for a sword.
	for i := 0; i < 27; i++ {
		p.Inventory().Adds(200, 1)
	}
	p.Inventory().Adds(995, 1000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	sess.messages = nil
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "Not enough space in your inventory." {
		t.Fatalf("messages = %q", sess.messages)
	}
	if got := p.Inventory().GetAmount(995); got != 1000 {
		t.Fatalf("coins = %d, want 1000 (no charge)", got)
	}
}

func TestBuyLimitHook(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	mgr.Hooks.OnBuyLimit = func(e *ShopBuyLimitEvent) {
		if e.Available != 10 || e.Original != 10 || e.Limit != 10 {
			t.Fatalf("buy-limit event = %+v", e)
		}
		e.Limit = 2
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 5})
	if got := p.Inventory().GetAmount(100); got != 2 {
		t.Fatalf("swords = %d, want 2 (limited)", got)
	}

	mgr.Hooks.OnBuyLimit = func(e *ShopBuyLimitEvent) {
		e.Limit = 0
		e.Message = "You cannot buy that here."
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "You cannot buy that here." {
		t.Fatalf("messages = %q", sess.messages)
	}
	if got := p.Inventory().GetAmount(100); got != 2 {
		t.Fatalf("swords = %d, want 2 (no further purchase)", got)
	}
}

func TestDeliveryHook(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	var purchase *ShopPurchaseEvent
	mgr.Hooks.OnDelivery = func(e *ShopDeliveryEvent) {
		e.DeliverID = 200
		e.Amount = 1
		e.CaptureDelivery = true
	}
	mgr.Hooks.OnPurchase = func(e *ShopPurchaseEvent) { purchase = e }
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	// One purchase delivers one chicken; the sword stock drops
	// by 1 and the sword price is charged. The receipt names
	// the newly filled slot and the delivered item.
	if got := p.Inventory().GetAmount(200); got != 1 {
		t.Fatalf("chickens = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(995); got != 9900 {
		t.Fatalf("coins = %d, want 9900", got)
	}
	if purchase == nil || purchase.ItemID != 100 || purchase.Amount != 1 || !purchase.OriginalStock ||
		!purchase.CaptureDelivery || purchase.DeliverID != 200 || purchase.DeliveredSlot != 1 ||
		purchase.DeliveredItem == nil || purchase.DeliveredItem.ID() != 200 {
		t.Fatalf("purchase event = %+v", purchase)
	}
}

func TestDeliveryBundleSize(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	var purchase *ShopPurchaseEvent
	mgr.Hooks.OnDelivery = func(e *ShopDeliveryEvent) {
		e.DeliverID = 200
		e.Amount = 2
		e.CaptureDelivery = true
	}
	mgr.Hooks.OnPurchase = func(e *ShopPurchaseEvent) { purchase = e }
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	// A bundle of 2 delivers 2 chickens for one purchase. The
	// receipt does not arm: it requires a single non-stackable
	// item at bundle size 1.
	if got := p.Inventory().GetAmount(200); got != 2 {
		t.Fatalf("chickens = %d, want 2", got)
	}
	if purchase == nil || purchase.DeliveredSlot != -1 || purchase.DeliveredItem != nil {
		t.Fatalf("purchase event = %+v, want no receipt slot/item", purchase)
	}
}

func TestKeepStockHookOnBuy(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	mgr.AddShopStock(1, 100, 2) // Stock 12, original 10.
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	mgr.Hooks.OnKeepStock = func(e *ShopKeepStockEvent) {
		if e.Selling {
			t.Fatal("buy keep-stock event has Selling set")
		}
		e.Keep = true
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 5})
	// Only the 2 surplus units leave; the player still gets and
	// pays for all 5.
	entries, _ := mgr.DisplayEntries(1, p)
	if entries[0].Amount != 10 {
		t.Fatalf("stock = %d, want 10 (own stock kept)", entries[0].Amount)
	}
	if got := p.Inventory().GetAmount(100); got != 5 {
		t.Fatalf("swords = %d, want 5", got)
	}
	if got := p.Inventory().GetAmount(995); got != 9500 {
		t.Fatalf("coins = %d, want 9500", got)
	}
}

func TestSellItemSuccess(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 3)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 2})
	if got := p.Inventory().GetAmount(100); got != 1 {
		t.Fatalf("swords left = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(995); got != 170 {
		t.Fatalf("coins = %d, want 170 (2 x 85)", got)
	}
	entries, _ := mgr.DisplayEntries(0, p)
	if len(entries) != 2 || entries[1].ItemID != 100 || entries[1].Amount != 2 {
		t.Fatalf("entries = %+v, want sold-in swords appended", entries)
	}
}

func TestSellItemRefusals(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	buysNothing := legacyCoinShop()
	buysNothing.ID = 9
	buysNothing.Name = "Farming Supplies"
	buysNothing.SalesPolicy.BuysNothing = shopBoolPtr(true)
	mgr.Initialize([]ShopDefinition{legacyCoinShop(), buysNothing, generalStoreShop()})

	// Specialist shop, unlisted item.
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(200, 1)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 200, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "You can't sell this item to this shop." {
		t.Fatalf("messages = %q", sess.messages)
	}

	// Buys-nothing shop.
	p2, sess2 := newShopTestPlayer(defs)
	p2.Inventory().Adds(100, 1)
	mgr.Open(p2, 9, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p2, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess2.messages) != 1 || sess2.messages[0] != "You cannot sell to this shop." {
		t.Fatalf("messages = %q", sess2.messages)
	}

	// Own currency: coins to a general store (which buys
	// anything — a specialist shop refuses coins earlier, at
	// buysItem).
	p3, sess3 := newShopTestPlayer(defs)
	p3.Inventory().Adds(995, 10)
	mgr.Open(p3, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p3, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 995, Amount: 5})
	if len(sess3.messages) != 1 || sess3.messages[0] != "This item has no value." {
		t.Fatalf("messages = %q", sess3.messages)
	}
	if got := p3.Inventory().GetAmount(995); got != 10 {
		t.Fatalf("coins = %d, want 10", got)
	}
}

func TestSellItemCannotSell(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, sess := newShopTestPlayer(defs)
	// The untradeable helm is not sellable by definition.
	p.Inventory().Adds(103, 1)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 103, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "This item cannot be sold." {
		t.Fatalf("messages = %q", sess.messages)
	}
}

func TestSellItemNoteSellsAsUnnoted(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(104, 2) // Noted swords.
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 104, Amount: 1})
	if got := p.Inventory().GetAmount(104); got != 1 {
		t.Fatalf("notes left = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(995); got != 85 {
		t.Fatalf("coins = %d, want 85", got)
	}
	entries, _ := mgr.DisplayEntries(0, p)
	found := false
	for _, e := range entries {
		if e.ItemID == 100 && e.Amount == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("entries = %+v, want unnoted sword stock", entries)
	}
}

func TestSellItemNoRoomForPayment(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, sess := newShopTestPlayer(defs)
	// 27 unstackable fillers + a stack of 5 feathers: selling
	// one feather leaves the stack's slot occupied, so the coin
	// payment has nowhere to go and no coin stack to join.
	for i := 0; i < 27; i++ {
		p.Inventory().Adds(200, 1)
	}
	p.Inventory().Adds(101, 5)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 27, ItemID: 101, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "You don't have enough inventory space." {
		t.Fatalf("messages = %q", sess.messages)
	}
	if got := p.Inventory().GetAmount(101); got != 5 {
		t.Fatalf("feathers = %d, want 5 (no sale)", got)
	}
}

func TestSellItemKeepStock(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 1)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.Hooks.OnKeepStock = func(e *ShopKeepStockEvent) {
		if !e.Selling {
			t.Fatal("sell keep-stock event lacks Selling")
		}
		e.Keep = true
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if got := p.Inventory().GetAmount(995); got != 85 {
		t.Fatalf("coins = %d, want 85 (paid)", got)
	}
	entries, _ := mgr.DisplayEntries(0, p)
	if len(entries) != 1 {
		t.Fatalf("entries = %+v, want no added stock", entries)
	}
}

func TestSellItemStockPricedReprices(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{ratedShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 2)
	mgr.Open(p, 2, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 2})
	// Unit 1 at offset 0: 60. Unit 2 at offset +1: 58. Total 118.
	if got := p.Inventory().GetAmount(995); got != 118 {
		t.Fatalf("coins = %d, want 118", got)
	}
}

func TestRestockRefillsAndStops(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	mgr.removeStock(mgr.shops[1], 100, 1) // Stock 9 of 10.
	mgr.ensureRestockTask()
	if !mgr.RestockTaskRunning() {
		t.Fatal("restock task not running after ensure")
	}
	for i := 0; i < 3; i++ {
		mgr.Tick()
		entries, _ := mgr.DisplayEntries(1, nil)
		if entries[0].Amount != 9 {
			t.Fatalf("tick %d: stock = %d, want 9 (interval 4)", i+1, entries[0].Amount)
		}
	}
	mgr.Tick()
	entries, _ := mgr.DisplayEntries(1, nil)
	if entries[0].Amount != 10 {
		t.Fatalf("tick 4: stock = %d, want 10", entries[0].Amount)
	}
	// The pass that balances the shop still reports pending
	// (the item was out of balance when the pass found it), so
	// the task stops on the next pass, as in TS.
	if !mgr.RestockTaskRunning() {
		t.Fatal("restock task stopped on the balancing pass")
	}
	mgr.Tick()
	if mgr.RestockTaskRunning() {
		t.Fatal("restock task still running once balanced")
	}
	// A balanced shop reports no pending work.
	if mgr.RestockAll() {
		t.Fatal("RestockAll on a balanced shop reported pending")
	}
}

func TestDestockSoldItemInGeneralStore(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	mgr.AddShopStock(0, 100, 2) // Sold-in swords, destock interval 2.
	mgr.Tick()
	entries, _ := mgr.DisplayEntries(0, nil)
	if len(entries) != 2 || entries[1].Amount != 2 {
		t.Fatalf("tick 1: entries = %+v", entries)
	}
	mgr.Tick()
	entries, _ = mgr.DisplayEntries(0, nil)
	if len(entries) != 2 || entries[1].Amount != 1 {
		t.Fatalf("tick 2: entries = %+v, want one destocked", entries)
	}
	mgr.Tick()
	mgr.Tick()
	entries, _ = mgr.DisplayEntries(0, nil)
	if len(entries) != 1 {
		t.Fatalf("after drain: entries = %+v, want sold-in item gone", entries)
	}
}

func TestRestockPerItemInterval(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := legacyCoinShop()
	shop.OriginalStock = []ShopStockEntry{{ID: 100, Amount: 10, RestockTicks: shopIntPtr(2)}}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	mgr.removeStock(mgr.shops[1], 100, 1)
	mgr.ensureRestockTask()
	mgr.Tick()
	entries, _ := mgr.DisplayEntries(1, nil)
	if entries[0].Amount != 9 {
		t.Fatalf("tick 1: stock = %d, want 9", entries[0].Amount)
	}
	mgr.Tick()
	entries, _ = mgr.DisplayEntries(1, nil)
	if entries[0].Amount != 10 {
		t.Fatalf("tick 2: stock = %d, want 10 (per-item interval 2)", entries[0].Amount)
	}
}

func TestSharedStock(t *testing.T) {
	defs := newShopFixtureDefs()
	a := legacyCoinShop()
	a.ID = 10
	a.Name = "Shared Shop"
	b := legacyCoinShop()
	b.ID = 11
	b.Name = "Shared Shop Copy"
	b.Currency = "COINS"
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{a, b})
	mgr.ShareStock(func(def ShopDefinition) string {
		if def.ID == 11 {
			return "Shared Shop"
		}
		return ""
	})
	// A purchase at the copy moves the stock both show.
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 11, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 3})
	entries, _ := mgr.DisplayEntries(10, p)
	if entries[0].Amount != 7 {
		t.Fatalf("owner stock = %d, want 7", entries[0].Amount)
	}
	// Restocking runs once, with the owner: one tick with
	// interval 4 must not double-step.
	mgr.ensureRestockTask()
	for i := 0; i < 4; i++ {
		mgr.Tick()
	}
	entries, _ = mgr.DisplayEntries(10, p)
	if entries[0].Amount != 8 {
		t.Fatalf("owner stock after restock = %d, want 8 (single step)", entries[0].Amount)
	}
	entries, _ = mgr.DisplayEntries(11, p)
	if entries[0].Amount != 8 {
		t.Fatalf("copy stock after restock = %d, want 8", entries[0].Amount)
	}
}

func TestQuantityModeAndWidgetButtons(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	sender.opens = nil

	if mgr.QuantityMode(p) != 0 {
		t.Fatalf("initial mode = %d, want 0", mgr.QuantityMode(p))
	}
	// Child 8 is the "Buy 1" button: mode 1, reopening the shop.
	if !mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 8, ButtonNum: 1, HasSlot: true, Slot: 0}) {
		t.Fatal("quantity button not handled")
	}
	if mgr.QuantityMode(p) != 1 {
		t.Fatalf("mode = %d, want 1", mgr.QuantityMode(p))
	}
	if len(sender.opens) != 1 || sender.opens[0].QuantityMode != 1 {
		t.Fatalf("reopen views = %+v", sender.opens)
	}
	if got := p.GetAttribute(ShopQuantityModeAttribute); got != 1 {
		t.Fatalf("attribute = %v, want 1", got)
	}
	// An out-of-range stored mode sanitizes to 0, as does a
	// fractional one (TS requires an integer).
	p.SetAttribute(ShopQuantityModeAttribute, 99)
	if mgr.QuantityMode(p) != 0 {
		t.Fatalf("sanitized mode = %d, want 0", mgr.QuantityMode(p))
	}
	p.SetAttribute(ShopQuantityModeAttribute, 1.5)
	if mgr.QuantityMode(p) != 0 {
		t.Fatalf("fractional mode = %d, want 0", mgr.QuantityMode(p))
	}
	p.SetAttribute(ShopQuantityModeAttribute, float64(2))
	if mgr.QuantityMode(p) != 2 {
		t.Fatalf("integral float mode = %d, want 2", mgr.QuantityMode(p))
	}
}

func TestWidgetStockClick(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 1000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	// Slot is 1-based in the main grid; button 2 buys 1.
	if !mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 16, ButtonNum: 2, HasSlot: true, Slot: 1, HasItemID: true, ItemID: 100}) {
		t.Fatal("stock click not handled")
	}
	if got := p.Inventory().GetAmount(100); got != 1 {
		t.Fatalf("swords = %d, want 1", got)
	}
	// A mismatched item id is ignored (handled, no trade).
	mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 16, ButtonNum: 2, HasSlot: true, Slot: 1, HasItemID: true, ItemID: 200})
	if got := p.Inventory().GetAmount(100); got != 1 {
		t.Fatalf("swords after mismatch = %d, want 1", got)
	}
	// Button 10 examines.
	mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 16, ButtonNum: 10, HasSlot: true, Slot: 1, HasItemID: true, ItemID: 100})
	if len(sess.messages) != 1 || sess.messages[0] != "A sword." {
		t.Fatalf("examine messages = %q", sess.messages)
	}
	// No slot: not handled.
	if mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 16, ButtonNum: 2}) {
		t.Fatal("slotless click reported handled")
	}
	// Another group: not handled.
	if mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: 999, ChildID: 16, ButtonNum: 2, HasSlot: true, Slot: 1}) {
		t.Fatal("foreign group click reported handled")
	}
}

func TestWidgetSideInventoryClick(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 1)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	// Side grid slots are 0-based; button 2 sells 1.
	if !mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopSideInterfaceID, ChildID: 0, ButtonNum: 2, HasSlot: true, Slot: 0}) {
		t.Fatal("side click not handled")
	}
	if got := p.Inventory().GetAmount(100); got != 0 {
		t.Fatalf("swords = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(995); got != 85 {
		t.Fatalf("coins = %d, want 85", got)
	}
}

func TestWidgetLeftClickUsesQuantityMode(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	p.SetAttribute(ShopQuantityModeAttribute, 2) // Buy 5.
	mgr.HandleWidgetAction(p, ShopWidgetAction{GroupID: ShopMainInterfaceID, ChildID: 16, ButtonNum: 1, HasSlot: true, Slot: 1, HasItemID: true, ItemID: 100})
	if got := p.Inventory().GetAmount(100); got != 5 {
		t.Fatalf("swords = %d, want 5 (mode amount)", got)
	}
}

func TestContainerActionXPrompt(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{promptAmount: 3}
	sender.promptExecute = func(execute func(amount int)) { execute(sender.promptAmount) }
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(995, 10000)
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionX, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100})
	if len(sender.prompts) != 1 || sender.prompts[0] != "How many would you like to buy?" {
		t.Fatalf("prompts = %q", sender.prompts)
	}
	if got := p.Inventory().GetAmount(100); got != 3 {
		t.Fatalf("swords = %d, want 3", got)
	}
}

func TestProcessPlayerClosesWhenInterfaceGone(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	mgr.Open(p, 1, true, DefaultShopTargetUID)

	// With no seam installed, ProcessPlayer never closes.
	mgr.ProcessPlayer(p)
	if _, ok := mgr.ActiveShopID(p); !ok {
		t.Fatal("ProcessPlayer closed the shop with no seam")
	}
	// With a seam reporting the interface gone, it closes.
	mgr.IsShopInterfaceOpen = func(player *Player) bool { return false }
	mgr.ProcessPlayer(p)
	if _, ok := mgr.ActiveShopID(p); ok {
		t.Fatal("ProcessPlayer left the shop open after the interface closed")
	}
	if sender.closes != 1 {
		t.Fatalf("closes = %d, want 1", sender.closes)
	}
}

func TestProcessPlayerSkipsUnwatchedBots(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	mgr.IsShopInterfaceOpen = func(player *Player) bool { return false }
	p := NewPlayer("bot", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	p.IsBot = true
	// No session: unwatched. Open still tracks (TS open has no
	// bot check); ProcessPlayer must skip the close check.
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	mgr.ProcessPlayer(p)
	if _, ok := mgr.ActiveShopID(p); !ok {
		t.Fatal("ProcessPlayer closed an unwatched bot's shop")
	}
}

func TestGetOpenShop(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p, _ := newShopTestPlayer(defs)
	if mgr.GetOpenShop(p) != nil {
		t.Fatal("GetOpenShop without an open shop is non-nil")
	}
	mgr.Open(p, 1, true, DefaultShopTargetUID)
	view := mgr.GetOpenShop(p)
	if view == nil || view.Name != "Sword Shop" || view.Currency != "Coins" {
		t.Fatalf("view = %+v", view)
	}
	if len(view.Stock) != 1 || view.Stock[0].Price != 100 {
		t.Fatalf("stock = %+v", view.Stock)
	}
}

func TestCurrencyRegistrationValidation(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	if err := mgr.RegisterCurrency("", ShopCurrencyHandler{
		Amount: func(p *Player) int { return 0 },
		Add:    func(p *Player, n int) {},
		Remove: func(p *Player, n int) {},
	}); err == nil {
		t.Fatal("empty currency name accepted")
	}
	if err := mgr.RegisterCurrency("X", ShopCurrencyHandler{}); err == nil {
		t.Fatal("handler without functions accepted")
	}
	if err := mgr.RegisterItemCurrency(0, ItemCurrencyOptions{}); err == nil {
		t.Fatal("item currency 0 accepted")
	}
	if err := mgr.RegisterItemCurrency(-3, ItemCurrencyOptions{}); err == nil {
		t.Fatal("negative item currency accepted")
	}
}

func TestItemCurrency(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 20, Name: "TzHaar Shop", Currency: "TOKKUL",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
		SalesPolicy: ShopSalesPolicy{
			SellsAt: shopIntPtr(1000), BuysAt: shopIntPtr(600), ChangePer: shopIntPtr(0),
		},
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	if err := mgr.RegisterItemCurrency(300, ItemCurrencyOptions{Unit: "tokkul"}); err != nil {
		t.Fatalf("RegisterItemCurrency: %v", err)
	}
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(300, 250)
	mgr.Open(p, 20, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 2})
	if got := p.Inventory().GetAmount(300); got != 50 {
		t.Fatalf("tokkul = %d, want 50", got)
	}
	if got := p.Inventory().GetAmount(100); got != 2 {
		t.Fatalf("swords = %d, want 2", got)
	}
	// The unit word appears in the price check and the refusal.
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionValue, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100})
	if len(sess.messages) != 1 || sess.messages[0] != "Iron sword: currently costs 100 tokkul." {
		t.Fatalf("messages = %q", sess.messages)
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess.messages) != 2 || sess.messages[1] != "You don't have enough tokkul." {
		t.Fatalf("messages = %q", sess.messages)
	}
}

func TestPointsCurrency(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 21, Name: "Points Shop", Currency: "POINTS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(50)}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	p, _ := newShopTestPlayer(defs)
	p.SetAttribute(ShopPointsAttribute, 120)
	mgr.Open(p, 21, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 2})
	if got := p.Inventory().GetAmount(100); got != 2 {
		t.Fatalf("swords = %d, want 2", got)
	}
	if got := attributeNumber(p.GetAttribute(ShopPointsAttribute)); got != 20 {
		t.Fatalf("points = %d, want 20", got)
	}
	// A sale pays points back (the shop lists the item, and
	// POINTS is not a buyback currency — content must allow the
	// buyback; here the specialist rate is absent, so the sale
	// refuses: this pins the reward-shop default).
	p2, sess2 := newShopTestPlayer(defs)
	p2.Inventory().Adds(100, 1)
	mgr.Open(p2, 21, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p2, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess2.messages) != 1 || sess2.messages[0] != "You cannot sell to this shop." {
		t.Fatalf("messages = %q", sess2.messages)
	}
}

func TestBuysBackHook(t *testing.T) {
	defs := newShopFixtureDefs()
	shop := ShopDefinition{
		ID: 22, Name: "Reward Shop", Currency: "POINTS",
		OriginalStock:        []ShopStockEntry{{ID: 100, Amount: 10, Price: shopIntPtr(50)}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{shop})
	mgr.Hooks.OnBuysBack = func(e *ShopBuysBackEvent) { e.Buys = true }
	p, _ := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 1)
	mgr.Open(p, 22, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	// Sold: 85% of 50 = 42 points (no rates, no buyback rate).
	if got := attributeNumber(p.GetAttribute(ShopPointsAttribute)); got != 42 {
		t.Fatalf("points = %d, want 42", got)
	}
}

func TestCanSellHook(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop()})
	mgr.Hooks.OnCanSell = func(e *ShopCanSellEvent) { e.Sellable = false }
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(100, 1)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "This item cannot be sold." {
		t.Fatalf("messages = %q", sess.messages)
	}
}

func TestBloodMoneyItemsCannotSellForCoins(t *testing.T) {
	defs := newShopFixtureDefs()
	pkShop := ShopDefinition{
		ID: 13, Name: "PK Shop", Currency: "BLOOD_MONEY",
		OriginalStock:        []ShopStockEntry{{ID: 400, Amount: 5}},
		DefaultRestockTicks:  4,
		DefaultDestockTicks:  4,
		SoldItemDestockTicks: 100,
	}
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{generalStoreShop(), pkShop})
	p, sess := newShopTestPlayer(defs)
	p.Inventory().Adds(400, 1)
	mgr.Open(p, 0, true, DefaultShopTargetUID)
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopInventoryInterfaceID, Slot: 0, ItemID: 400, Amount: 1})
	if len(sess.messages) != 1 || sess.messages[0] != "PK shop items cannot be sold for coins." {
		t.Fatalf("messages = %q", sess.messages)
	}
}

func TestNilSourceDegrades(t *testing.T) {
	mgr := NewShopManager(nil)
	mgr.Initialize([]ShopDefinition{legacyCoinShop()})
	p := NewPlayer("bare", Location{X: 3200, Y: 3200, Z: 0})
	// No definitions: every price is 0, so buying silently
	// refuses and opening still tracks.
	if !mgr.Open(p, 1, true, DefaultShopTargetUID) {
		t.Fatal("Open with nil source failed")
	}
	mgr.HandleItemContainerAction(p, ShopItemContainerAction{Kind: ShopActionBuySell, ContainerID: ShopItemsInterfaceID, Slot: 0, ItemID: 100, Amount: 1})
	if got := p.Inventory().GetAmount(100); got != 0 {
		t.Fatalf("bought = %d, want 0", got)
	}
}

func TestReloadClosesViewersOfRemovedShops(t *testing.T) {
	defs := newShopFixtureDefs()
	mgr := NewShopManager(defs)
	mgr.Initialize([]ShopDefinition{legacyCoinShop(), ratedShop()})
	sender := &shopTestSender{}
	mgr.Sender = sender
	p, _ := newShopTestPlayer(defs)
	mgr.Open(p, 2, true, DefaultShopTargetUID)
	sender.opens = nil
	sender.closes = 0
	mgr.Reload([]ShopDefinition{legacyCoinShop()})
	if _, ok := mgr.ActiveShopID(p); ok {
		t.Fatal("viewer of a removed shop still active after Reload")
	}
	if sender.closes != 1 {
		t.Fatalf("closes = %d, want 1", sender.closes)
	}
}
