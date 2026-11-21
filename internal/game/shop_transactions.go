package game

// Shop transactions: the port of buyItem and sellItem
// (ShopManager.ts), with their exact refusal texts.
//
// SLOT IDENTITY. TS sellItem locates the sold stack with
// indexOf on the live inventory array — reference identity. Go
// containers hold values and expose copies, so there is no
// reference to find: HandleItemContainerAction passes the
// clicked slot through, sellItem verifies the slot still holds
// the clicked id, and the entered-amount closure falls back to
// the first slot holding the same id with the same metadata
// fingerprint (the value-semantics form of "the same stack").
// A stack that moved and changed metadata in between is not
// found, and the sale silently drops — the closest Go behavior
// to TS indexOf returning -1.
//
// SOUNDS GAP. TS plays PICK_UP_ITEM after a purchase and
// DROP_ITEM after a sale. No sound system exists in Go; the
// transactions complete without them (see the file header of
// shop_manager.go).

// buyItem ports ShopManager.buyItem (ShopManager.ts): buy up to
// amount of itemID from shop for player. An unpriced or hidden
// item is silently refused; an empty stock, a buy-limit clamp
// to zero, an unaffordable purchase, and a full inventory all
// refuse with their exact texts. The delivery event may change
// what is delivered and in what bundle size; the purchase event
// fires after the delivery lands. Stock, currency and viewers
// update only on success, and the restock task is ensured.
func (m *ShopManager) buyItem(player *Player, shop *runtimeShop, itemID, amount int) {
	def := m.itemDef(itemID)
	// An item with no price of its own is never sold; content
	// may still make a priced one free ("shop:buy-price").
	if m.itemPrice(shop, def) <= 0 || !m.stockVisible(player, shop, itemID) {
		return
	}
	price := m.buyPrice(player, shop, def, 0)

	quantity := normalizeAmount(amount)
	if quantity > MaxShopActionAmount {
		quantity = MaxShopActionAmount
	}
	if quantity <= 0 {
		return
	}
	available := shop.state.stock[itemID]
	if available <= 0 {
		player.SendMessage("This item is currently out of stock. Come back later.")
		return
	}
	// "shop:buy-limit": a plugin may lower how many of the
	// stock this player can buy.
	if m.Hooks.OnBuyLimit != nil {
		limit := &ShopBuyLimitEvent{
			Player: player, ShopID: shop.definition.ID, ItemID: itemID,
			Available: available, Original: shop.state.originalAmounts[itemID],
			Limit: available,
		}
		m.Hooks.OnBuyLimit(limit)
		if limit.Limit < available {
			available = limit.Limit
			if available < 0 {
				available = 0
			}
			if available <= 0 {
				if limit.Message != "" {
					player.SendMessage(limit.Message)
				}
				return
			}
		}
	}
	if quantity > available {
		quantity = available
	}
	unitPrices := m.affordablePrices(player, shop, def, price, quantity)
	quantity = len(unitPrices)
	if quantity <= 0 {
		if unit := m.currencyUnit(shop.definition.Currency); unit != "" {
			player.SendMessage("You don't have enough " + unit + ".")
		} else {
			player.SendMessage("You can't afford that.")
		}
		return
	}

	// Content may change the delivered item or bundle size;
	// capacity upgrades opt into a delivery receipt.
	deliverID := itemID
	perPurchase := 1
	captureDelivery := false
	if m.Hooks.OnDelivery != nil {
		delivery := &ShopDeliveryEvent{
			Player: player, ShopID: shop.definition.ID,
			Currency: shop.definition.Currency, ItemID: itemID,
			DeliverID: itemID, Amount: 1,
		}
		m.Hooks.OnDelivery(delivery)
		if delivery.DeliverID > 0 {
			deliverID = delivery.DeliverID
		}
		if delivery.Amount > 0 {
			perPurchase = delivery.Amount
		}
		captureDelivery = delivery.CaptureDelivery
	}
	delivered := m.itemDef(deliverID)
	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	if delivered.stackable {
		addable := inventory.GetAddableAmount(NewItem(deliverID, satMul(quantity, perPurchase)))
		if fitting := addable / perPurchase; fitting < quantity {
			quantity = fitting
		}
		if quantity <= 0 {
			inventory.Full()
			return
		}
	} else {
		if fitting := inventory.GetFreeSlots() / perPurchase; fitting < quantity {
			quantity = fitting
		}
		if quantity <= 0 {
			inventory.Full()
			return
		}
	}

	removed := m.takeStock(player, shop, itemID, quantity)
	if removed <= 0 {
		return
	}
	payment := 0
	for i := 0; i < removed && i < len(unitPrices); i++ {
		payment = satAdd(payment, unitPrices[i])
	}
	m.removeCurrency(player, shop.definition.Currency, payment)
	// Opt-in receipt for one nonstackable item: retain the
	// actual newly filled slot and object.
	deliveredSlot := -1
	if captureDelivery && removed == 1 && perPurchase == 1 && !delivered.stackable {
		deliveredSlot = inventory.GetEmptySlot()
	}
	inventory.Adds(deliverID, satMul(removed, perPurchase))
	var deliveredItem *Item
	if deliveredSlot >= 0 {
		item := inventory.ItemAt(deliveredSlot)
		deliveredItem = &item
	}
	if m.Hooks.OnPurchase != nil {
		event := &ShopPurchaseEvent{
			Player: player, ShopID: shop.definition.ID, ShopName: shop.definition.Name,
			ItemID: itemID, Amount: removed,
			OriginalStock: shop.state.originalAmounts[itemID] > 0,
			Currency:      shop.definition.Currency,
		}
		if captureDelivery {
			event.CaptureDelivery = true
			event.DeliverID = deliverID
			event.DeliveredSlot = deliveredSlot
			event.DeliveredItem = deliveredItem
		}
		m.Hooks.OnPurchase(event)
	}
	// Sounds.sendSound(player, Sound.PICK_UP_ITEM): no sound
	// system exists in Go.
	m.refresh(shop.definition.ID)
	m.ensureRestockTask()
}

// sellItem ports ShopManager.sellItem (ShopManager.ts): sell up
// to amount of the clicked stack to shop for player. Notes sell
// as the item they stand for; blood-money items, unbuyable
// items, unsellable stacks, valueless items, a full shop, and
// a payment that would not fit all refuse with their exact
// texts. A stack sells from its own slot; unstackable items
// start at the clicked slot and continue through the others.
// A stock-priced sale reprices after each unit. The keep-stock
// event (with selling: true) may keep the stock as it was —
// the item is paid for and not added.
func (m *ShopManager) sellItem(player *Player, shop *runtimeShop, item Item, slot, amount int) {
	itemID := item.ID()
	// A note sells as the item it stands for.
	stockID := m.itemDef(itemID).unNote
	if shop.definition.Currency == "COINS" && m.isBloodMoneyShopItem(stockID) {
		player.SendMessage("PK shop items cannot be sold for coins.")
		return
	}
	if !m.buysItem(shop, stockID) {
		player.SendMessage(m.sellRefusal(shop))
		return
	}
	def := m.itemDef(stockID)
	if !m.canSell(shop, item, player) {
		player.SendMessage("This item cannot be sold.")
		return
	}

	inventory := player.Inventory()
	if inventory == nil {
		return
	}
	slot = m.resolveSellSlot(inventory, item, slot)
	if slot < 0 {
		return
	}
	// A stack sells from its own slot; unstackable items (one
	// per slot) start at the clicked slot and continue through
	// the others.
	var slots []int
	if m.itemDef(itemID).stackable {
		slots = []int{slot}
	} else {
		slots = append(slots, slot)
		for index := 0; index < inventory.Capacity(); index++ {
			if index != slot {
				slots = append(slots, index)
			}
		}
	}
	var sellable []int
	for _, index := range slots {
		held := inventory.ItemAt(index)
		if held.ID() == itemID && m.canSell(shop, held, player) {
			sellable = append(sellable, index)
		}
	}
	held := 0
	for _, index := range sellable {
		held += inventory.ItemAt(index).Amount()
	}
	quantity := normalizeAmount(amount)
	if quantity > MaxShopActionAmount {
		quantity = MaxShopActionAmount
	}
	if quantity > held {
		quantity = held
	}
	if quantity <= 0 {
		return
	}
	price := m.sellPrice(shop, def, player, 0)
	// Wiki (Lumbridge General Store, Bandit Duty Free): a
	// stocked pot, jug or chisel is "bought at" 0 coins. A
	// stock-priced shop refuses only an item of no value, not
	// one its formula rounds down to 0.
	if m.isOwnCurrency(shop, stockID) || (m.stockPriced(shop) && m.itemPrice(shop, def) <= 0) || (!m.stockPriced(shop) && price <= 0) {
		player.SendMessage("This item has no value.")
		return
	}
	if shop.state.stock[stockID] <= 0 && len(shop.state.order) >= MaxShopItems {
		player.SendMessage("The shop is currently full.")
		return
	}

	// A multi-item sale changes the price after each unit, even
	// when content restores stock after the transaction (the
	// league keep-stock hook).
	payment := satMul(quantity, price)
	if m.stockPriced(shop) {
		payment = 0
		for offset := 0; offset < quantity; offset++ {
			payment = satAdd(payment, m.sellPrice(shop, def, player, offset))
		}
	}
	if !m.canReceivePayment(player, shop.definition.Currency, sellable, quantity, payment) {
		player.SendMessage("You don't have enough inventory space.")
		return
	}

	remaining := quantity
	for _, index := range sellable {
		if remaining <= 0 {
			break
		}
		taken := remaining
		if held := inventory.ItemAt(index).Amount(); taken > held {
			taken = held
		}
		inventory.DeleteAtSlot(index, taken, false)
		remaining -= taken
	}
	inventory.RefreshItems()
	m.addCurrency(player, shop.definition.Currency, payment)
	// Content may keep the stock as it was on a sale too
	// ("shop:keep-stock" with `selling`, a league's shops whose
	// stock never changes): the item is paid for and not added.
	keep := false
	if m.Hooks.OnKeepStock != nil {
		request := &ShopKeepStockEvent{
			Player: player, ShopID: shop.definition.ID,
			ItemID: stockID, Amount: quantity, Keep: false, Selling: true,
		}
		m.Hooks.OnKeepStock(request)
		keep = request.Keep
	}
	if !keep {
		m.addStock(shop, stockID, quantity)
	}
	// Sounds.sendSound(player, Sound.DROP_ITEM): no sound
	// system exists in Go.
	m.refresh(shop.definition.ID)
	m.ensureRestockTask()
}

// resolveSellSlot locates the stack being sold: the clicked
// slot when it still holds the clicked id, else the first slot
// holding the same id with the same metadata (the
// value-semantics form of TS indexOf's reference identity — see
// the file header). -1 when the stack is nowhere to be found.
func (m *ShopManager) resolveSellSlot(inventory *Inventory, item Item, slot int) int {
	if slot >= 0 && slot < inventory.Capacity() && inventory.ItemAt(slot).ID() == item.ID() {
		return slot
	}
	for index := 0; index < inventory.Capacity(); index++ {
		held := inventory.ItemAt(index)
		if held.ID() == item.ID() && held.metaFingerprint() == item.metaFingerprint() {
			return index
		}
	}
	return -1
}

// satAdd adds saturating at maxSafeInteger, for payment totals
// a hook-inflated price could otherwise overflow (see satMul).
func satAdd(a, b int) int {
	if a < 0 || b < 0 {
		return 0
	}
	if a > maxSafeInteger-b {
		return maxSafeInteger
	}
	return a + b
}
