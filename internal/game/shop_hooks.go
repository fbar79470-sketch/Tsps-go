package game

// ShopHooks is the game-side shop event seam: the func fields
// ShopManager calls for the events TS emits through
// PluginManager, and the policy gates TS consults before acting.
// The coordinator's wiring populates the ShopManager.Hooks field
// from the plugin hub; every call site checks for nil, so an
// unpopulated seam is inert and core shops run standalone. The
// game package must never import plugins — the seam is func
// fields only, following the combat-hooks pattern
// (combat_hooks.go).
//
// Event shapes mirror the TS request objects at each emit site
// in ShopManager.ts: hooks mutate the pointed-to struct in
// place, and the manager reads the fields back after the call,
// exactly as TS reads the mutated request object. Where TS uses
// null (the buy-limit message, the purchase delivery receipt),
// the Go shape uses "" / a presence flag and documents it.
//
// TS emits TEN shop events, not the seven the phase plan lists:
// shop:opened, shop:stock-visible, shop:buy-limit,
// shop:delivery, shop:purchase, shop:keep-stock,
// shop:sell-price, plus shop:buy-price (buyPrice),
// shop:buys-back (buysItem) and shop:can-sell (canSell). All ten
// have seam fields here. The onCanShop / onCanStockItem hook
// families are gates, not custom events: CanShop lives here
// (emitCanShop, PluginManager.ts:1534, consulted by open), while
// CanStockItem fires at definition-load time in
// ShopDefinitionLoader.ts:183, before any ShopManager exists —
// its seam lives on the definition loader (definition
// package), not here.

// ShopOpenedEvent is the shop:opened payload
// (ShopManager.ts open): a shop actually opened for a player.
type ShopOpenedEvent struct {
	Player   *Player
	ShopID   int
	ShopName string
}

// ShopStockVisibleEvent is the shop:stock-visible payload
// (ShopManager.ts stockVisible): content may hide stock from
// one player. Visible starts true; a hook sets it false to
// hide the entry, which then takes no display slot and cannot
// be bought (TS: request.visible !== false).
type ShopStockVisibleEvent struct {
	Player   *Player
	ShopID   int
	ShopName string
	ItemID   int
	Visible  bool
}

// ShopBuyLimitEvent is the shop:buy-limit payload
// (ShopManager.ts buyItem): a hook may lower how many of the
// stock this player can buy. Limit starts at Available; when a
// hook lowers it below Available the purchase clamps to it, and
// a clamp to zero refuses the purchase, sending Message when
// non-empty (TS: message: string | null).
type ShopBuyLimitEvent struct {
	Player    *Player
	ShopID    int
	ItemID    int
	Available int
	Original  int
	Limit     int
	Message   string
}

// ShopDeliveryEvent is the shop:delivery payload
// (ShopManager.ts buyItem): content may change the delivered
// item or bundle size. DeliverID starts at ItemID and Amount
// at 1; the manager validates both (integer > 0) before use,
// falling back to ItemID / 1, as TS does. CaptureDelivery opts
// into the purchase receipt: the delivered slot and item are
// then reported on ShopPurchaseEvent.
type ShopDeliveryEvent struct {
	Player          *Player
	ShopID          int
	Currency        string
	ItemID          int
	DeliverID       int
	Amount          int
	CaptureDelivery bool
}

// ShopPurchaseEvent is the shop:purchase payload
// (ShopManager.ts buyItem): a purchase completed. OriginalStock
// reports whether the shop lists the item in its own stock.
// The delivery fields are meaningful only when the delivery
// hook set CaptureDelivery (TS spreads them conditionally):
// DeliverID is what was delivered, DeliveredSlot the newly
// filled inventory slot (-1 when no slot applies), and
// DeliveredItem the item now in it (nil when the slot holds
// nothing — TS reads the slot after the add, which may be
// undefined for a stackable merge into an existing stack; the
// receipt path only arms for a single non-stackable item, so
// in practice it is set).
type ShopPurchaseEvent struct {
	Player        *Player
	ShopID        int
	ShopName      string
	ItemID        int
	Amount        int
	OriginalStock bool
	Currency      string
	// Delivery receipt: see the type comment.
	CaptureDelivery bool
	DeliverID       int
	DeliveredSlot   int
	DeliveredItem   *Item
}

// ShopKeepStockEvent is the shop:keep-stock payload
// (ShopManager.ts takeStock / sellItem): content may keep the
// shop's stock as it was. On a purchase (Selling false) only
// stock above the shop's own amount is then taken; on a sale
// (Selling true) the sold item is paid for but not added.
// Keep starts false.
type ShopKeepStockEvent struct {
	Player  *Player
	ShopID  int
	ItemID  int
	Amount  int
	Keep    bool
	Selling bool
}

// ShopSellPriceEvent is the shop:sell-price payload
// (ShopManager.ts sellPrice): content may change what a player
// is paid for one. BuyPrice is the item's price before the
// sell formula; Price starts at the computed sell price, and
// the manager uses the hook's value only when it is a safe
// integer >= 0, as TS does. Player is nil when the price is
// computed without a player (display paths always pass one).
type ShopSellPriceEvent struct {
	Player   *Player
	ShopID   int
	ShopName string
	Currency string
	ItemID   int
	BuyPrice int
	Price    int
}

// ShopBuyPriceEvent is the shop:buy-price payload
// (ShopManager.ts buyPrice): content may change what a player
// pays for one ("a league relic's free coin shops"). Price
// starts at the computed buy price, and the manager uses the
// hook's value only when it is a safe integer >= 0, as TS
// does.
type ShopBuyPriceEvent struct {
	Player   *Player
	ShopID   int
	ShopName string
	Currency string
	ItemID   int
	Price    int
}

// ShopBuysBackEvent is the shop:buys-back payload
// (ShopManager.ts buysItem): for a shop whose currency is not
// one of the buyback currencies, content may say the shop buys
// an item back. Buys starts at whether the shop has a Wiki
// specialist-buyback rate; the shop buys the item only when a
// hook leaves Buys true (TS: request.buys !== true refuses).
type ShopBuysBackEvent struct {
	ShopID   int
	ShopName string
	Currency string
	ItemID   int
	Buys     bool
}

// ShopCanSellEvent is the shop:can-sell payload
// (ShopManager.ts canSell): content may refuse a sale for one
// player. Sellable starts at whether the item is the shop's
// own buyback stock or definition-sellable; the sale proceeds
// only when a hook leaves Sellable true (TS:
// request.sellable === true). Player is nil when the check
// runs without a player (canSell's default).
type ShopCanSellEvent struct {
	Player   *Player
	ShopID   int
	ShopName string
	Currency string
	ItemID   int
	Sellable bool
}

// ShopHooks holds every shop hook field. All fields are
// optional; ShopManager calls them nil-safely. The wiring
// populates the ShopManager.Hooks field once at boot before
// ticking starts.
type ShopHooks struct {
	// OnShopOpened fires shop:opened after a shop opens
	// (ShopManager.ts open).
	OnShopOpened func(e *ShopOpenedEvent)
	// OnStockVisible fires shop:stock-visible per stock entry
	// per display computation (ShopManager.ts stockVisible).
	OnStockVisible func(e *ShopStockVisibleEvent)
	// OnBuyLimit fires shop:buy-limit per purchase
	// (ShopManager.ts buyItem).
	OnBuyLimit func(e *ShopBuyLimitEvent)
	// OnDelivery fires shop:delivery per purchase
	// (ShopManager.ts buyItem).
	OnDelivery func(e *ShopDeliveryEvent)
	// OnPurchase fires shop:purchase after a purchase
	// completes (ShopManager.ts buyItem).
	OnPurchase func(e *ShopPurchaseEvent)
	// OnKeepStock fires shop:keep-stock per purchase and per
	// sale (ShopManager.ts takeStock / sellItem).
	OnKeepStock func(e *ShopKeepStockEvent)
	// OnSellPrice fires shop:sell-price per sell-price
	// computation (ShopManager.ts sellPrice), including once
	// per unit of a stock-priced multi-item sale.
	OnSellPrice func(e *ShopSellPriceEvent)
	// OnBuyPrice fires shop:buy-price per buy-price
	// computation (ShopManager.ts buyPrice), including once
	// per unit of a rated multi-item purchase.
	OnBuyPrice func(e *ShopBuyPriceEvent)
	// OnBuysBack fires shop:buys-back for non-buyback
	// currencies (ShopManager.ts buysItem).
	OnBuysBack func(e *ShopBuysBackEvent)
	// OnCanSell fires shop:can-sell per sellability check
	// (ShopManager.ts canSell).
	OnCanSell func(e *ShopCanSellEvent)

	// CanShop is the emitCanShop policy gate
	// (PluginManager.ts:1534): the onCanShop hooks' first
	// answer. A nil return means "no opinion" (the open
	// proceeds); a false answer blocks the open, and open
	// reports true (handled) without opening, exactly as TS
	// does (ShopManager.ts open returns true on denial).
	CanShop func(player *Player, shopID int) *bool
}
