package game

import (
	"math"
	"regexp"
	"strings"
)

// Shop pricing: the port of the pricing half of ShopManager.ts
// (buyPrice, sellPrice, ratedBuyPrice, ratedSellPrice,
// stockOffset, valueRates, itemPrice, specialistBuyback,
// buysItem, sellRefusal, isBloodMoneyShopItem, deletesItems,
// isGeneralStore, isOwnCurrency, canSell, priceCheck,
// affordablePrices).
//
// ARITHMETIC. TS computes every rated price in float64
// (Math.floor(value * rate / 1000)) and the sales-tax price as
// Math.floor(buyPrice * 0.85). Go mirrors that with float64
// throughout, so the results match TS bit for bit on every
// input, including any binary rounding edge. Do not "fix" these
// to integer arithmetic: the float behavior IS the TS behavior.

// salesTax is SALES_TAX (ShopManager.ts): what a shop without
// the Wiki's rates pays, of the item's price (the legacy core
// shops).
const salesTax = 0.85

// bloodMoneyShopIDs is BLOOD_MONEY_SHOP_IDS (ShopManager.ts):
// the shops whose stock counts as PK-shop items, unsellable for
// coins elsewhere.
var bloodMoneyShopIDs = []int{13, 14, 27}

// buybackCurrencies is BUYBACK_CURRENCIES (ShopManager.ts): the
// currencies whose shops buy stock back without asking content.
var buybackCurrencies = map[string]bool{
	"COINS": true, "TOKKUL": true, "TRADING STICKS": true, "BLOOD_MONEY": true,
}

// valuePricedCurrencies is VALUE_PRICED_CURRENCIES
// (ShopManager.ts): the currencies priced from the item's value.
var valuePricedCurrencies = map[string]bool{
	"COINS": true, "TOKKUL": true, "TRADING STICKS": true,
}

// generalStoreName matches /general store/i (ShopManager.ts
// isGeneralStore): the shop data only marks general stores by
// name.
var generalStoreName = regexp.MustCompile(`(?i)general store`)

// itemPrice ports ShopManager.itemPrice (ShopManager.ts): the
// per-item price override when one is configured and positive,
// else the blood-money value for BLOOD_MONEY shops, else the
// store value.
func (m *ShopManager) itemPrice(shop *runtimeShop, def shopItemDef) int {
	if override := shop.itemPrices[def.id]; override > 0 {
		return override
	}
	if shop.definition.Currency == "BLOOD_MONEY" {
		return def.bloodMoney
	}
	return def.value
}

// ItemPrice reports a shop's price for an item (see itemPrice).
// It exposes the lookup for the coordinator and tests.
func (m *ShopManager) ItemPrice(shopID, itemID int) (int, bool) {
	shop, ok := m.shops[shopID]
	if !ok {
		return 0, false
	}
	return m.itemPrice(shop, m.itemDef(itemID)), true
}

// valueRates ports ShopManager.valueRates (ShopManager.ts): the
// Wiki rates that price this shop, or false where it has none
// (the legacy core shops, reward currencies). The currency key
// is uppercased before the set lookup, as TS uppercases it.
func (m *ShopManager) valueRates(shop *runtimeShop) (ShopSalesPolicy, bool) {
	if !valuePricedCurrencies[strings.ToUpper(shop.definition.Currency)] {
		return ShopSalesPolicy{}, false
	}
	return shop.definition.SalesPolicy, true
}

// stockOffset ports ShopManager.stockOffset (ShopManager.ts):
// how far the shop's stock of an item is above (positive) or
// below (negative) its normal stock, 0 for items it doesn't
// list. change shifts the stock first (a purchase in progress
// counts stock already taken; a sale counts stock already
// handed over).
func (m *ShopManager) stockOffset(shop *runtimeShop, itemID, change int) int {
	return shop.state.stock[itemID] + change - shop.state.originalAmounts[itemID]
}

// ratedBuyPrice ports ShopManager.ratedBuyPrice
// (ShopManager.ts): the Wiki rated buy price, or false where
// the shop's rates don't price the item (no sellsAt rate, or a
// per-item price override). Wiki (Shop § Pricing): buying one
// costs value × (sells at − change per × items above the normal
// stock), rising the same way for each item below it, rounded
// down. It never costs more than value × (sells at + 500%),
// nor less than 1 coin, 10% of value, or value × (sells at −
// 100%). bought is how many of it this purchase has already
// taken. An item of no value rates 0, not "unpriced" — the
// (0, true) return.
func (m *ShopManager) ratedBuyPrice(shop *runtimeShop, def shopItemDef, bought int) (int, bool) {
	rates, ok := m.valueRates(shop)
	if !ok || rates.SellsAt == nil || shop.itemPrices[def.id] > 0 {
		return 0, false
	}
	sellsAt := *rates.SellsAt
	value := def.value
	if !(value > 0) {
		return 0, true
	}
	changePer := 0
	if rates.ChangePer != nil {
		changePer = *rates.ChangePer
	}
	rate := sellsAt - changePer*m.stockOffset(shop, def.id, -bought)
	highest := int(math.Floor(float64(value) * float64(sellsAt+5000) / 1000))
	lowest := int(math.Floor(float64(value) / 10))
	if floor := int(math.Floor(float64(value) * float64(sellsAt-1000) / 1000)); floor > lowest {
		lowest = floor
	}
	if lowest < 1 {
		lowest = 1
	}
	price := int(math.Floor(float64(value) * float64(rate) / 1000))
	if price < lowest {
		price = lowest
	}
	if price > highest {
		price = highest
	}
	return price, true
}

// ratedSellPrice ports ShopManager.ratedSellPrice
// (ShopManager.ts): the Wiki rated sell price, or false where
// the shop's rates don't price the item (no buysAt rate). Wiki:
// selling one pays value × (buys at − change per × items above
// the normal stock), rising the same way for each item below
// it, rounded down. Never more than value plus the buy rate,
// nor less than 10% of value. sold is how many of it this sale
// has already handed over. An item of no value rates 0.
func (m *ShopManager) ratedSellPrice(shop *runtimeShop, def shopItemDef, sold int) (int, bool) {
	rates, ok := m.valueRates(shop)
	if !ok || rates.BuysAt == nil {
		return 0, false
	}
	buysAt := *rates.BuysAt
	value := def.value
	if !(value > 0) {
		return 0, true
	}
	changePer := 0
	if rates.ChangePer != nil {
		changePer = *rates.ChangePer
	}
	rate := buysAt - changePer*m.stockOffset(shop, def.id, sold)
	highest := int(math.Floor(float64(value) * float64(1000+buysAt) / 1000))
	price := int(math.Floor(float64(value) * float64(rate) / 1000))
	if price > highest {
		price = highest
	}
	if floor := int(math.Floor(float64(value) / 10)); price < floor {
		price = floor
	}
	return price, true
}

// stockPriced ports ShopManager.stockPriced (ShopManager.ts): a
// shop whose sale price follows its stock — one with a buysAt
// rate under a value-priced currency. Such a shop refuses only
// an item of no value, not one its formula rounds down to 0.
func (m *ShopManager) stockPriced(shop *runtimeShop) bool {
	rates, ok := m.valueRates(shop)
	return ok && rates.BuysAt != nil
}

// sellPrice ports ShopManager.sellPrice (ShopManager.ts): what
// this player is paid for one. The rated price wins; else a
// specialist-buyback shop pays its rate of the item price; else
// the legacy rule pays 85% (float sales tax), except a price of
// 0 or 1, which pays in full. additionalStock is how many of
// it this sale has already handed over. The shop:sell-price
// event may rewrite the price; a hook value that is not a safe
// integer >= 0 is ignored, as in TS.
func (m *ShopManager) sellPrice(shop *runtimeShop, def shopItemDef, player *Player, additionalStock int) int {
	buyPrice := m.itemPrice(shop, def)
	buyback := m.specialistBuyback(shop)
	var price int
	if rated, ok := m.ratedSellPrice(shop, def, additionalStock); ok {
		price = rated
	} else if buyback > 0 {
		price = int(math.Floor(float64(buyPrice) * float64(buyback) / 1000))
	} else if buyPrice > 1 {
		price = int(math.Floor(float64(buyPrice) * salesTax))
	} else {
		price = buyPrice
	}
	if m.Hooks.OnSellPrice != nil {
		request := &ShopSellPriceEvent{
			Player: player, ShopID: shop.definition.ID, ShopName: shop.definition.Name,
			Currency: shop.definition.Currency, ItemID: def.id, BuyPrice: buyPrice, Price: price,
		}
		m.Hooks.OnSellPrice(request)
		if isSafeInteger(request.Price) && request.Price >= 0 {
			return request.Price
		}
	}
	return price
}

// buyPrice ports ShopManager.buyPrice (ShopManager.ts): what
// this player pays for one — the rated price, else the item
// price — as the shop:buy-price event may change it (a league
// relic's free coin shops). Never below 0: a hook value that
// is not a safe integer >= 0 is ignored, as in TS. bought is
// how many of it this purchase has already taken, for a shop
// whose price follows its stock.
func (m *ShopManager) buyPrice(player *Player, shop *runtimeShop, def shopItemDef, bought int) int {
	price := m.itemPrice(shop, def)
	if rated, ok := m.ratedBuyPrice(shop, def, bought); ok {
		price = rated
	}
	if m.Hooks.OnBuyPrice != nil {
		request := &ShopBuyPriceEvent{
			Player: player, ShopID: shop.definition.ID, ShopName: shop.definition.Name,
			Currency: shop.definition.Currency, ItemID: def.id, Price: price,
		}
		m.Hooks.OnBuyPrice(request)
		if isSafeInteger(request.Price) && request.Price >= 0 {
			return request.Price
		}
	}
	return price
}

// BuyPrice reports what player pays for one of itemID in shopID
// (see buyPrice), or false for an unknown shop. It exposes the
// pricing for the coordinator and tests.
func (m *ShopManager) BuyPrice(player *Player, shopID, itemID int) (int, bool) {
	shop, ok := m.shops[shopID]
	if !ok {
		return 0, false
	}
	return m.buyPrice(player, shop, m.itemDef(itemID), 0), true
}

// SellPrice reports what player is paid for one of itemID in
// shopID (see sellPrice), or false for an unknown shop.
func (m *ShopManager) SellPrice(player *Player, shopID, itemID int) (int, bool) {
	shop, ok := m.shops[shopID]
	if !ok {
		return 0, false
	}
	return m.sellPrice(shop, m.itemDef(itemID), player, 0), true
}

// specialistBuyback ports ShopManager.specialistBuyback
// (ShopManager.ts): the Wiki's "Buys at" (per mille) for a shop
// paying in its own reward currency, 0 where it has none.
// Buyback-currency shops return 0 — they are priced by value.
func (m *ShopManager) specialistBuyback(shop *runtimeShop) int {
	if buybackCurrencies[strings.ToUpper(shop.definition.Currency)] {
		return 0
	}
	buysAt := shop.definition.SalesPolicy.BuysAt
	if buysAt == nil || !isSafeInteger(*buysAt) || *buysAt <= 0 {
		return 0
	}
	return *buysAt
}

// buysItem ports ShopManager.buysItem (ShopManager.ts): whether
// the shop buys an item. A buys-nothing shop buys none; a
// reward-currency shop buys only what content says it buys
// (shop:buys-back, defaulting to its specialist-buyback rate);
// otherwise the shop buys what it lists (including items listed
// at 0), and a general store buys anything.
func (m *ShopManager) buysItem(shop *runtimeShop, itemID int) bool {
	if policy := shop.definition.SalesPolicy; policy.BuysNothing != nil && *policy.BuysNothing {
		return false
	}
	if !buybackCurrencies[strings.ToUpper(shop.definition.Currency)] {
		buys := m.specialistBuyback(shop) > 0
		if m.Hooks.OnBuysBack != nil {
			request := &ShopBuysBackEvent{
				ShopID: shop.definition.ID, ShopName: shop.definition.Name,
				Currency: shop.definition.Currency, ItemID: itemID, Buys: buys,
			}
			m.Hooks.OnBuysBack(request)
			buys = request.Buys
		}
		if !buys {
			return false
		}
	}
	if m.isGeneralStore(shop) {
		return true
	}
	_, listed := shop.state.originalAmounts[itemID]
	return listed
}

// sellRefusal ports ShopManager.sellRefusal (ShopManager.ts):
// why a sale is refused. A shop that buys nothing: "You cannot
// sell to this shop." One that buys only some items: "You can't
// sell this item to this shop." A shop that buys nothing at all
// (no buyback currency, no stocked item it would buy) uses the
// first wording too.
func (m *ShopManager) sellRefusal(shop *runtimeShop) string {
	if policy := shop.definition.SalesPolicy; policy.BuysNothing != nil && *policy.BuysNothing {
		return "You cannot sell to this shop."
	}
	buysAnything := buybackCurrencies[strings.ToUpper(shop.definition.Currency)]
	if !buysAnything {
		for stocked := range shop.state.originalAmounts {
			if m.buysItem(shop, stocked) {
				buysAnything = true
				break
			}
		}
	}
	if buysAnything {
		return "You can't sell this item to this shop."
	}
	return "You cannot sell to this shop."
}

// isBloodMoneyShopItem ports ShopManager.isBloodMoneyShopItem
// (ShopManager.ts): whether any blood-money shop (ids 13, 14,
// 27) lists the item in its original stock. Such items cannot
// be sold for coins.
func (m *ShopManager) isBloodMoneyShopItem(itemID int) bool {
	for _, shopID := range bloodMoneyShopIDs {
		shop, ok := m.shops[shopID]
		if !ok || shop.definition.Currency != "BLOOD_MONEY" {
			continue
		}
		for _, entry := range shop.definition.OriginalStock {
			if entry.ID == itemID {
				return true
			}
		}
	}
	return false
}

// deletesItems ports ShopManager.deletesItems (ShopManager.ts):
// general stores delete sold-in items on the sold-item destock
// timer; specialist shops restock them instead.
func (m *ShopManager) deletesItems(shop *runtimeShop) bool {
	return m.isGeneralStore(shop)
}

// isGeneralStore ports ShopManager.isGeneralStore
// (ShopManager.ts): the configured policy wins; else shop id 0
// (ShopIdentifiers.GENERAL_STORE) or a name matching
// /general store/i.
func (m *ShopManager) isGeneralStore(shop *runtimeShop) bool {
	if configured := shop.definition.SalesPolicy.GeneralStore; configured != nil {
		return *configured
	}
	return shop.definition.ID == 0 || generalStoreName.MatchString(shop.definition.Name)
}

// isOwnCurrency ports ShopManager.isOwnCurrency
// (ShopManager.ts): whether the item IS the shop's own currency
// (coins to a coin shop): never bought, even where the price
// formula would allow 0. A note counts as the item it stands
// for. The currency item is the handler's item when the handler
// has one, else the currency-item fallback.
func (m *ShopManager) isOwnCurrency(shop *runtimeShop, itemID int) bool {
	currencyID := m.currencyItemID(shop.definition.Currency)
	if handler, ok := m.currencyHandlers[shop.definition.Currency]; ok && handler.ItemID > 0 {
		currencyID = handler.ItemID
	}
	return currencyID > 0 && m.itemDef(itemID).unNote == currencyID
}

// canSell ports ShopManager.canSell (ShopManager.ts): whether
// this item stack may be sold to this shop. An untradeable
// stack (the meta flag — Item.isUntradeable) never sells. A
// shop that buys its stock back for its own currency takes the
// items it lists, though no other shop would. Otherwise the
// definition's sellable flag decides, and the shop:can-sell
// event may refuse the sale for one player. The event's
// sellable must be left exactly true for the sale to proceed.
func (m *ShopManager) canSell(shop *runtimeShop, item Item, player *Player) bool {
	if item.IsUntradeable() {
		return false
	}
	_, ownStock := shop.state.originalAmounts[m.itemDef(item.ID()).unNote]
	ownStock = ownStock && m.specialistBuyback(shop) > 0
	sellable := ownStock || item.IsSellable(m.items)
	if m.Hooks.OnCanSell != nil {
		request := &ShopCanSellEvent{
			Player: player, ShopID: shop.definition.ID, ShopName: shop.definition.Name,
			Currency: shop.definition.Currency, ItemID: item.ID(), Sellable: sellable,
		}
		m.Hooks.OnCanSell(request)
		sellable = request.Sellable
	}
	return sellable
}

// priceCheck ports ShopManager.priceCheck (ShopManager.ts): the
// Value action's answer. A note is priced as the item it stands
// for; blood-money items, unbuyable items and unsellable stacks
// refuse with their exact texts; an item of no value says so;
// otherwise the price line names the item, the price with
// commas, and the currency — in the unit-word form when the
// currency has a unit ("currently costs 240 trade sticks",
// "1 coin" for a single coin), else the "x Currency" form.
func (m *ShopManager) priceCheck(player *Player, shop *runtimeShop, itemID int, fromShop bool, item Item) {
	if !fromShop {
		itemID = m.itemDef(itemID).unNote
	}
	if !fromShop && shop.definition.Currency == "COINS" && m.isBloodMoneyShopItem(itemID) {
		player.SendMessage("PK shop items cannot be sold for coins.")
		return
	}
	if !fromShop && !m.buysItem(shop, itemID) {
		player.SendMessage(m.sellRefusal(shop))
		return
	}
	def := m.itemDef(itemID)
	if !fromShop && !m.canSell(shop, item, player) {
		player.SendMessage("This item cannot be sold to a shop.")
		return
	}
	var price int
	if fromShop {
		price = m.buyPrice(player, shop, def, 0)
	} else {
		price = m.sellPrice(shop, def, player, 0)
	}
	if (!fromShop && m.isOwnCurrency(shop, itemID)) ||
		(price <= 0 && !((fromShop || m.stockPriced(shop)) && m.itemPrice(shop, def) > 0)) {
		player.SendMessage("This item has no value.")
		return
	}
	if unit := m.currencyUnit(shop.definition.Currency); unit != "" {
		named := unit
		if unit == "coins" && price == 1 {
			named = "coin"
		}
		verb := "shop will buy for"
		if fromShop {
			verb = "currently costs"
		}
		player.SendMessage(def.name + ": " + verb + " " + insertCommas(price) + " " + named + ".")
		return
	}
	if fromShop {
		player.SendMessage(def.name + " currently costs " + insertCommas(price) + " x " + m.currencyName(shop.definition.Currency) + ".")
	} else {
		player.SendMessage(def.name + ": shop will buy for " + insertCommas(price) + " x " + m.currencyName(shop.definition.Currency) + ".")
	}
}

// affordablePrices ports ShopManager.affordablePrices
// (ShopManager.ts): the price of each of up to quantity the
// player can pay for, in order. A shop whose price follows its
// stock charges each one at the stock it is taken from, so a
// large purchase costs more per item: each unit after the first
// reprices with bought units already taken (re-firing
// shop:buy-price per unit, as TS does). A flat-priced shop fits
// as many whole units as the funds cover; a free (price <= 0)
// flat item fits them all.
func (m *ShopManager) affordablePrices(player *Player, shop *runtimeShop, def shopItemDef, first, quantity int) []int {
	funds := m.currencyAmount(player, shop.definition.Currency)
	_, rated := m.ratedBuyPrice(shop, def, 0)
	changePer := 0
	if rates, ok := m.valueRates(shop); ok && rates.ChangePer != nil {
		changePer = *rates.ChangePer
	}
	varies := rated && changePer > 0
	if !varies {
		count := quantity
		if first > 0 {
			if affordable := funds / first; affordable < count {
				count = affordable
			}
		}
		if count < 0 {
			count = 0
		}
		prices := make([]int, count)
		for i := range prices {
			prices[i] = first
		}
		return prices
	}
	var prices []int
	total := 0
	for bought := 0; bought < quantity; bought++ {
		each := first
		if bought > 0 {
			each = m.buyPrice(player, shop, def, bought)
		}
		if total+each > funds {
			break
		}
		total += each
		prices = append(prices, each)
	}
	return prices
}
