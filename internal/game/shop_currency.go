package game

import (
	"fmt"
	"strings"
)

// Shop currency handlers: the port of the currency half of
// ShopManager.ts (registerCurrency, registerItemCurrency,
// currencyAmount, addCurrency, removeCurrency,
// canReceivePayment, currencyItemId, currencyName,
// currencyUnit).
//
// No currency handler is registered by core: coins and blood
// money work through the currency-item fallback (inventory item
// 995 / 13307), POINTS works through its player attribute, and
// every other currency (tokkul, trading sticks, minigame
// points) needs a plugin-registered handler. The hub's
// registerShopCurrency / registerItemShopCurrency members
// delegate to RegisterCurrency / RegisterItemCurrency here.

// CoinsItemID is ItemIdentifiers.COINS (995): the inventory item
// COINS shops price in (ShopManager.ts currencyItemId).
const CoinsItemID = 995

// BloodMoneyItemID is ItemIdentifiers.BLOOD_MONEY (13307): the
// inventory item BLOOD_MONEY shops price in (ShopManager.ts
// currencyItemId).
const BloodMoneyItemID = 13307

// ShopCurrencyHandler is one currency's read/write surface
// (ShopCurrencyHandler, ShopManager.ts): how much of it a player
// has, how to give some, how to take some, its display name,
// the inventory item it is paid in when it is one (ItemID <= 0
// means "not an inventory item"), and the unit word live shop
// messages use for it ("trade sticks"; "" means none — COINS
// defaults to "coins" without a handler).
type ShopCurrencyHandler struct {
	// Amount reports the player's balance. It must tolerate a
	// nil player (TS: player?.getInventory?.()... ?? 0).
	Amount func(player *Player) int
	// Add gives amount of the currency. It must tolerate a nil
	// player (TS: player?.getInventory... no-op).
	Add func(player *Player, amount int)
	// Remove takes amount of the currency. It must tolerate a
	// nil player.
	Remove func(player *Player, amount int)
	// Name is the currency's display name ("Coins",
	// "Tokkul", ...).
	Name string
	// ItemID is the inventory item the currency is paid in,
	// when it is one. <= 0 means it is not an inventory item
	// (TS: itemId?: number — undefined falls back to
	// currencyItemId at the call sites that need an item).
	ItemID int
	// Unit is how live shop messages name an amount of it
	// ("trade sticks"). "" means no unit word.
	Unit string
}

// RegisterCurrency ports ShopManager.registerCurrency
// (ShopManager.ts): store handler under name, verbatim (TS
// stores the key as given; registerItemCurrency uppercases its
// keys before calling). It returns an error, where TS throws,
// for an empty name, a handler with a missing Amount/Add/Remove
// function — with the TS error text. Re-registering a name
// replaces its handler (JS Map.set).
func (m *ShopManager) RegisterCurrency(name string, handler ShopCurrencyHandler) error {
	if name == "" || handler.Amount == nil || handler.Add == nil || handler.Remove == nil {
		return fmt.Errorf("Invalid shop currency registration: %s", name)
	}
	m.currencyHandlers[name] = handler
	return nil
}

// ItemCurrencyOptions are registerItemCurrency's options
// (ShopManager.ts): an override display name, extra registry
// keys, and the unit word. An empty Name derives the name from
// the item definition (see RegisterItemCurrency).
type ItemCurrencyOptions struct {
	Name    string
	Aliases []string
	Unit    string
}

// RegisterItemCurrency ports ShopManager.registerItemCurrency
// (ShopManager.ts): register an inventory item as a currency.
// The handler reads the player's inventory amount, adds with
// Adds, and removes with DeleteNumber. The display name is the
// option name, else the item's definition name, else "Item N"
// (TS: options?.name ?? ItemDefinition.forId(itemId)?.getName?.()
// ?? `Item ${itemId}` — with a definition source installed, a
// definition with an empty name yields "" exactly as TS yields
// "", and the empty key is then skipped by the trim filter
// below; only a nil source falls through to "Item N"). The
// handler is registered under the uppercased trimmed name and
// every uppercased trimmed alias. It returns an error, where TS
// throws, for a non-positive item id.
func (m *ShopManager) RegisterItemCurrency(itemID int, options ItemCurrencyOptions) error {
	if itemID <= 0 {
		return fmt.Errorf("Invalid item shop currency: %d", itemID)
	}
	name := options.Name
	if name == "" {
		if m.items != nil {
			name = m.items.ItemName(itemID)
		} else {
			name = fmt.Sprintf("Item %d", itemID)
		}
	}
	handler := ShopCurrencyHandler{
		Name:   name,
		ItemID: itemID,
		Unit:   options.Unit,
		Amount: func(player *Player) int {
			if player == nil || player.Inventory() == nil {
				return 0
			}
			return player.Inventory().GetAmount(itemID)
		},
		Add: func(player *Player, amount int) {
			if player == nil || player.Inventory() == nil {
				return
			}
			player.Inventory().Adds(itemID, amount)
		},
		Remove: func(player *Player, amount int) {
			if player == nil || player.Inventory() == nil {
				return
			}
			player.Inventory().DeleteNumber(itemID, amount)
		},
	}
	keys := append([]string{name}, options.Aliases...)
	for _, key := range keys {
		if strings.TrimSpace(key) != "" {
			// registerCurrency cannot fail here: the key is
			// non-empty and the handler's functions are set.
			_ = m.RegisterCurrency(strings.ToUpper(strings.TrimSpace(key)), handler)
		}
	}
	return nil
}

// currencyAmount ports ShopManager.currencyAmount
// (ShopManager.ts): the player's balance in a currency — the
// registered handler's amount (clamped to >= 0; TS truncates
// with | 0, which Go ints do not need), the POINTS attribute,
// or the inventory amount of the currency item. Anything else
// is 0.
func (m *ShopManager) currencyAmount(player *Player, currency string) int {
	if handler, ok := m.currencyHandlers[currency]; ok {
		if amount := handler.Amount(player); amount > 0 {
			return amount
		}
		return 0
	}
	if currency == "POINTS" {
		if player == nil {
			return 0
		}
		if amount := attributeNumber(player.GetAttribute(ShopPointsAttribute)); amount > 0 {
			return amount
		}
		return 0
	}
	if itemID := m.currencyItemID(currency); itemID > 0 && player != nil && player.Inventory() != nil {
		return player.Inventory().GetAmount(itemID)
	}
	return 0
}

// CurrencyAmount reports the player's balance in a currency
// (see currencyAmount). It exposes the balance for the
// coordinator and tests.
func (m *ShopManager) CurrencyAmount(player *Player, currency string) int {
	return m.currencyAmount(player, currency)
}

// addCurrency ports ShopManager.addCurrency (ShopManager.ts):
// give quantity of a currency — through its handler, the POINTS
// attribute, or the inventory. A non-positive quantity gives
// nothing.
func (m *ShopManager) addCurrency(player *Player, currency string, amount int) {
	quantity := normalizeAmount(amount)
	if quantity <= 0 || player == nil {
		return
	}
	if handler, ok := m.currencyHandlers[currency]; ok {
		handler.Add(player, quantity)
		return
	}
	if currency == "POINTS" {
		player.SetAttribute(ShopPointsAttribute, m.currencyAmount(player, currency)+quantity)
		return
	}
	if itemID := m.currencyItemID(currency); itemID > 0 && player.Inventory() != nil {
		player.Inventory().Adds(itemID, quantity)
	}
}

// removeCurrency ports ShopManager.removeCurrency
// (ShopManager.ts): take quantity of a currency — through its
// handler, the POINTS attribute (floored at 0), or the
// inventory. A non-positive quantity takes nothing.
func (m *ShopManager) removeCurrency(player *Player, currency string, amount int) {
	quantity := normalizeAmount(amount)
	if quantity <= 0 || player == nil {
		return
	}
	if handler, ok := m.currencyHandlers[currency]; ok {
		handler.Remove(player, quantity)
		return
	}
	if currency == "POINTS" {
		balance := m.currencyAmount(player, currency) - quantity
		if balance < 0 {
			balance = 0
		}
		player.SetAttribute(ShopPointsAttribute, balance)
		return
	}
	if itemID := m.currencyItemID(currency); itemID > 0 && player.Inventory() != nil {
		player.Inventory().DeleteNumber(itemID, quantity)
	}
}

// canReceivePayment ports ShopManager.canReceivePayment
// (ShopManager.ts): preview the sale's slot deletions on a copy
// of the inventory, then ask whether the whole payment fits —
// including stack limits — exactly as the container would.
// A currency with no inventory item always fits (a handler or
// POINTS balance has no capacity). The preview container's
// refreshes are silent: it is not the player's own inventory
// (see RefreshItems' own-inventory guard).
func (m *ShopManager) canReceivePayment(player *Player, currency string, slots []int, quantity, payment int) bool {
	itemID := m.currencyItemID(currency)
	if handler, ok := m.currencyHandlers[currency]; ok && handler.ItemID > 0 {
		itemID = handler.ItemID
	}
	// A handler without an item id (ItemID <= 0) falls back to
	// the currency item, as TS's ?? does (handler?.itemId ??
	// currencyItemId).
	if itemID <= 0 {
		return true
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	preview := NewInventory(player)
	preview.SetItems(inventory.Items())
	remaining := quantity
	for _, slot := range slots {
		if remaining <= 0 {
			break
		}
		taken := remaining
		if held := preview.ItemAt(slot).Amount(); taken > held {
			taken = held
		}
		preview.DeleteAtSlot(slot, taken, false)
		remaining -= taken
	}
	return remaining == 0 && preview.GetAddableAmount(NewItem(itemID, payment)) == payment
}

// currencyItemID ports ShopManager.currencyItemId
// (ShopManager.ts): COINS is item 995, BLOOD_MONEY is item
// 13307, anything else has no currency item (-1).
func (m *ShopManager) currencyItemID(currency string) int {
	switch currency {
	case "COINS":
		return CoinsItemID
	case "BLOOD_MONEY":
		return BloodMoneyItemID
	default:
		return -1
	}
}

// CurrencyItemID reports the inventory item id a currency is
// paid in, or -1 (see currencyItemID). It exposes the lookup
// for the coordinator (the open view's currency item) and
// tests.
func (m *ShopManager) CurrencyItemID(currency string) int {
	return m.currencyItemID(currency)
}

// currencyName ports ShopManager.currencyName (ShopManager.ts):
// the registered handler's name, else Coins / Blood money /
// Points / the "Currency" fallback.
func (m *ShopManager) currencyName(currency string) string {
	if handler, ok := m.currencyHandlers[currency]; ok {
		return handler.Name
	}
	switch currency {
	case "COINS":
		return "Coins"
	case "BLOOD_MONEY":
		return "Blood money"
	case "POINTS":
		return "Points"
	default:
		return "Currency"
	}
}

// currencyUnit ports ShopManager.currencyUnit (ShopManager.ts):
// the registered handler's unit word, else "coins" for COINS,
// else "" (TS null — call sites treat both as "no unit").
func (m *ShopManager) currencyUnit(currency string) string {
	if handler, ok := m.currencyHandlers[currency]; ok && handler.Unit != "" {
		return handler.Unit
	}
	if currency == "COINS" {
		return "coins"
	}
	return ""
}
