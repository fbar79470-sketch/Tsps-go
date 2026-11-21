package game

import (
	"math"
	"regexp"
	"strings"
)

// ShopManager: the port of game/model/container/shop/ShopManager.ts
// (1339 lines). TS holds all state in static fields; Go holds it
// on a ShopManager instance (the GroundItemManager / AreaManager
// precedent), constructed with NewShopManager and driven by the
// coordinator: definitions in at boot (Initialize), one Tick per
// game tick for restocking, ProcessPlayer per player per tick,
// and Close when a player's interfaces close or the player
// leaves.
//
// PERSISTENCE. Shop stock is runtime-only: TS rebuilds every
// shop from its definition on every boot (GameBuilder calls
// ShopManager.initialize, which reloads from ShopDefinition),
// and no save/load path touches shop stock anywhere in TS core
// (the only shop-related save state is the player's own
// attributes: "shop:points" is a legacy PlayerSave field
// attribute, and "shop:quantity-mode" is registered persistent
// by initialize). Reloading — including a plugin's registerShop
// / removeShop churn — never writes anything durable, and this
// port adds no save fields.
//
// CONCURRENCY. Like the rest of the game layer, a ShopManager is
// tick-goroutine-only after boot: Initialize/Reload,
// RegisterCurrency/RegisterItemCurrency and ShareStock run at
// boot (or from plugin registration, before ticking), and every
// other method runs on the tick. Nothing here is synchronized.
//
// INTERFACE STATE GAP. TS decides whether a shop is open from
// the player's status (SHOPPING) and interface id, which it sets
// in openInterface and clears when interfaces close. No
// status/interface-id state exists in Go, so this port tracks
// "open" itself: a player with an active-shop entry counts as
// shopping. The IsShopInterfaceOpen seam lets the coordinator
// supply the real check when interface state lands; until then
// it is nil and ProcessPlayer cannot detect a shop closed by an
// interface change, so the coordinator must call Close when a
// player's interfaces close (PacketSender.closeInterface in TS)
// and when the player leaves the world (TS keeps viewers in a
// WeakMap, which drops departed players on its own; the Go maps
// need the explicit Close).
//
// SOUNDS GAP. TS plays CONTAINER_OPEN on opening, PICK_UP_ITEM
// on buying and DROP_ITEM on selling. No sound system exists in
// Go; the open view carries Opening so a future sender can play
// the open sound, and the buy/sell sounds have no seam at all.

const (
	// ShopInterfaceID is SHOP_INTERFACE_ID (ShopManager.ts):
	// the shop modal's interface id, one of the two ids
	// isOpen accepts in TS (the other is ShopMainInterfaceID).
	ShopInterfaceID = 3824
	// ShopInventoryInterfaceID is INVENTORY_INTERFACE_ID
	// (ShopManager.ts): the sell-from inventory container id.
	ShopInventoryInterfaceID = 3823
	// ShopItemsInterfaceID is ITEMS_INTERFACE_ID
	// (ShopManager.ts): the stock container id.
	ShopItemsInterfaceID = 3900
	// ShopMainInterfaceID is MAIN_INTERFACE_ID
	// (ShopManager.ts): the shop widget group id.
	ShopMainInterfaceID = 300
	// ShopSideInterfaceID is SIDE_INTERFACE_ID
	// (ShopManager.ts): the sell inventory widget group id.
	ShopSideInterfaceID = 301

	// DefaultShopTargetUID is the default targetUid for opening
	// a shop (ShopManager.ts open: (161 << 16) | 16).
	DefaultShopTargetUID = (161 << 16) | 16

	// MaxShopItems is MAX_SHOP_ITEMS (ShopManager.ts): the most
	// distinct items a shop's stock can hold.
	MaxShopItems = 1000
	// MaxShopActionAmount is MAX_ACTION_AMOUNT
	// (ShopManager.ts): the most units one buy/sell takes.
	MaxShopActionAmount = 5000
	// ShopQuantityVarbit is QUANTITY_VARBIT (ShopManager.ts
	// 6348): the quantity row sets it so the cache scripts make
	// an item's left-click "Value" or "Buy N".
	ShopQuantityVarbit = 6348
)

// ShopQuantityModeAttribute is QUANTITY_MODE_ATTRIBUTE
// (ShopManager.ts): the persistent player attribute holding the
// quantity-row mode. ShopManager.initialize() registers it
// persistent in TS (PlayerSave.persistAttribute with no
// default); the Go coordinator must register it at boot through
// persist.PersistAttribute — the game package cannot, because
// persistence lives outside it. See ShopPointsAttribute.
const ShopQuantityModeAttribute = "shop:quantity-mode"

// ShopPointsAttribute is POINTS_ATTRIBUTE (ShopManager.ts): the
// player attribute holding a POINTS-currency balance. It is
// already a persistent key in Go (a legacy PlayerSave field
// attribute, persist/attributes.go), so no boot registration
// is needed for it.
const ShopPointsAttribute = "shop:points"

// maxSafeInteger is Number.MAX_SAFE_INTEGER: the cap addStock
// applies to a stock pile, and the bound a hook-set price must
// satisfy (Number.isSafeInteger in buyPrice/sellPrice).
const maxSafeInteger = 9007199254740991

// shopQuantityByMode is QUANTITY_BY_MODE (ShopManager.ts): the
// buy/sell amount each quantity-row mode selects. Mode 0 is
// "Value" (no amount).
var shopQuantityByMode = [5]int{0, 1, 5, 10, 50}

// shopQuantityModeByButton is QUANTITY_MODE_BY_BUTTON
// (ShopManager.ts): the widget child id of each quantity-row
// button, mapped to its mode.
var shopQuantityModeByButton = map[int]int{5: 0, 8: 1, 10: 2, 12: 3, 14: 4}

// ShopStockEntry is one original-stock row (ShopStockDefinition,
// ShopDefinition.ts:20-25). RestockTicks and Price are nil when
// the row configures none (TS null).
type ShopStockEntry struct {
	ID           int
	Amount       int
	RestockTicks *int
	Price        *int
}

// ShopSalesPolicy is the Wiki StoreTableHead pricing policy
// (ShopSalesPolicy, ShopDefinition.ts:8-18). Each field is nil
// when the shop configures no value (TS undefined). Rates are
// per mille of the item's value.
type ShopSalesPolicy struct {
	GeneralStore *bool
	BuysNothing  *bool
	SellsAt      *int
	BuysAt       *int
	ChangePer    *int
}

// ShopDefinition is one shop's definition (ShopDefinition,
// ShopDefinition.ts:27-101), in the game package's own shape:
// package game never imports internal/definition (see doc.go),
// so the coordinator converts each loaded definition.Store shop
// into this struct, field for field (ID/Name/Currency/
// OriginalStock/DefaultRestockTicks/DefaultDestockTicks/
// SoldItemDestockTicks/Source/SalesPolicy, with the same
// nil-for-unset pointer semantics), and hands the slice to
// Initialize/Reload in load order.
type ShopDefinition struct {
	ID                   int
	Name                 string
	Currency             string
	OriginalStock        []ShopStockEntry
	DefaultRestockTicks  int
	DefaultDestockTicks  int
	SoldItemDestockTicks int
	Source               string
	SalesPolicy          ShopSalesPolicy
}

// ShopItemDefinitionSource is the item-definition surface shops,
// pricing and the examine price line read. It embeds
// ItemDefinitionSource (stackable/tradeable/sellable/un-note,
// which containers and Item.IsSellable already read) and adds
// the value/name accessors ShopManager and ExaminePrice read
// from ItemDefinition. Deliberately absent: everything else on
// ItemDefinition (equipment, bonuses, animations, sounds) —
// no shop path reads it. The coordinator implements this over
// the definition Store (Store.Item); a nil source answers every
// method with the TS DEFAULT shape (empty name, zero values,
// false flags, un-note to self), matching forId for an unknown
// id (ItemDefinition.ts:127-138).
type ShopItemDefinitionSource interface {
	ItemDefinitionSource

	// ItemName returns the item name (getName,
	// ItemDefinition.ts:177-179). Read by priceCheck messages
	// and the examine price line. Nil-source default: "".
	ItemName(id int) string
	// ItemExamine returns the examine text (getExamine,
	// ItemDefinition.ts:181-183). Read by examineItem.
	// Nil-source default: "".
	ItemExamine(id int) string
	// ItemValue returns the store value (getValue,
	// ItemDefinition.ts:185-187). Read by itemPrice and the
	// rated pricing formulas. Nil-source default: 0.
	ItemValue(id int) int
	// ItemBloodMoneyValue returns the blood money value
	// (getBloodMoneyValue, ItemDefinition.ts:204-206). Read
	// by itemPrice for BLOOD_MONEY shops. Nil-source
	// default: 0.
	ItemBloodMoneyValue(id int) int
	// ItemHasGrandExchangeValue reports whether a Grand
	// Exchange quote exists (hasGrandExchangeValue,
	// ItemDefinition.ts:195-197). Read by examinePrices.
	// Nil-source default: false.
	ItemHasGrandExchangeValue(id int) bool
	// ItemGrandExchangeValue returns the Grand Exchange quote,
	// falling back to the store value (getGrandExchangeValue,
	// ItemDefinition.ts:190-192). Read by examinePrices.
	// Nil-source default: 0.
	ItemGrandExchangeValue(id int) int
	// ItemHighAlchValue returns the high alchemy value
	// (getHighAlchValue, ItemDefinition.ts:208-210). Read by
	// examinePrices when ItemHasHighAlchValue is true; the
	// unset case falls back to 60% of value (ExaminePrice.ts
	// alch), which is why the Has method exists — TS
	// distinguishes an unset (undefined) value from a set 0
	// (a bond). Nil-source default: 0.
	ItemHighAlchValue(id int) int
	// ItemHasHighAlchValue reports whether a high alchemy
	// value was ever set for the item (getHighAlchValue() !==
	// undefined). Nil-source default: false.
	ItemHasHighAlchValue(id int) bool
	// ItemLowAlchValue returns the low alchemy value
	// (getLowAlchValue, ItemDefinition.ts:212-214). Read by
	// examinePrices when ItemHasLowAlchValue is true; the
	// unset case falls back to 40% of value.
	// Nil-source default: 0.
	ItemLowAlchValue(id int) int
	// ItemHasLowAlchValue reports whether a low alchemy value
	// was ever set for the item. Nil-source default: false.
	ItemHasLowAlchValue(id int) bool
}

// shopItemDef is one resolved item definition: the fields the
// shop paths read, resolved once per use through the manager's
// source (or the DEFAULT shape with a nil source).
type shopItemDef struct {
	id          int
	name        string
	examine     string
	value       int
	bloodMoney  int
	geValue     int
	hasGEValue  bool
	highAlch    int
	hasHighAlch bool
	lowAlch     int
	hasLowAlch  bool
	tradeable   bool
	sellable    bool
	stackable   bool
	unNote      int
}

// itemDef resolves id through the manager's source, or the
// DEFAULT shape when no source is installed.
func (m *ShopManager) itemDef(id int) shopItemDef {
	def := shopItemDef{id: id, unNote: id}
	if m == nil || m.items == nil {
		return def
	}
	src := m.items
	def.name = src.ItemName(id)
	def.examine = src.ItemExamine(id)
	def.value = src.ItemValue(id)
	def.bloodMoney = src.ItemBloodMoneyValue(id)
	def.geValue = src.ItemGrandExchangeValue(id)
	def.hasGEValue = src.ItemHasGrandExchangeValue(id)
	def.highAlch = src.ItemHighAlchValue(id)
	def.hasHighAlch = src.ItemHasHighAlchValue(id)
	def.lowAlch = src.ItemLowAlchValue(id)
	def.hasLowAlch = src.ItemHasLowAlchValue(id)
	def.tradeable = src.IsTradeable(id)
	def.sellable = src.IsSellable(id)
	def.stackable = src.IsStackable(id)
	def.unNote = src.UnNote(id)
	return def
}

// ShopSlotView is one stock row in a shop-open view: the
// ShopSlotView payload (ClientProtocol.ts:2086) in game-package
// shape (the game layer never imports protocol). The sender
// converts it to protocol.ShopSlotView field for field.
type ShopSlotView struct {
	Slot            int
	ItemID          int
	Quantity        int
	DefaultQuantity int
	PriceEach       int
	SellPrice       int
}

// ShopOpenView is everything openInterface sends for one shop
// opening (ShopManager.ts openInterface): the stock in display
// order with per-row prices, the title, the currency item, the
// general-store flag, the player's quantity mode, and the
// target widget uid. The sender (coordinator/session) owns the
// frame sequence — sendVarbit(QUANTITY_VARBIT, QuantityMode),
// the two sub-interfaces, interface scripts 1074 and 149, the
// two flags ranges, the inventory container (3823), and
// encodeShopOpen(String(ShopID), Title, CurrencyItemID,
// GeneralStore, 1, 1, Stock) — plus the CONTAINER_OPEN sound
// when Opening is true (no sound system exists in Go yet; see
// the file header). Opening reports whether this open is the
// player's first for this shop visit (TS computes it from the
// pre-open interface state, which Go does not have; the
// manager derives it from its active-shop tracking instead).
type ShopOpenView struct {
	ShopID         int
	Title          string
	CurrencyItemID int
	GeneralStore   bool
	QuantityMode   int
	TargetUID      int
	Opening        bool
	Stock          []ShopSlotView
}

// ShopSender is the outbound seam for shop interface traffic:
// the session/coordinator side of openInterface, close, and
// the buy/sell-X amount prompts. The manager calls it
// nil-safely; with no sender installed, opens and closes still
// update tracking but send nothing, and amount prompts are
// dropped (no entered-amount listener exists in Go yet, so the
// prompt has nowhere to return to even when sent — see
// HandleItemContainerAction).
type ShopSender interface {
	// SendShopOpen delivers one shop-open view (see
	// ShopOpenView for the frame sequence).
	SendShopOpen(p *Player, view ShopOpenView)
	// SendShopClose delivers encodeShopClose
	// (ClientProtocol.ts:2109-2111).
	SendShopClose(p *Player)
	// SendEnterAmountPrompt shows an enter-amount prompt and
	// runs execute with the entered amount when the player
	// answers (setEnteredAmountAction +
	// sendEnterAmountPrompt, ShopManager.ts
	// handleItemContainerAction). A sender without
	// amount-entry support drops the prompt.
	SendEnterAmountPrompt(p *Player, prompt string, execute func(amount int))
}

// shopStockState is the shareable half of a runtime shop: the
// maps and order shareStock links between versions of one store
// (ShopManager.ts linkStock). It lives behind a pointer so
// linked versions hold the SAME state object — TS assigns the
// same Map/Array references, and removeStock splices the order
// in place ("versions sharing a stock hold this same array").
// Prices (itemPrices) are deliberately NOT shared: each version
// keeps its own (ShopManager.ts shareStock comment).
type shopStockState struct {
	originalAmounts   map[int]int
	itemRestockTicks  map[int]int
	stock             map[int]int
	order             []int
	originalSlotCount int
	changeTimers      map[int]int
}

// runtimeShop is RuntimeShop (ShopManager.ts): a definition plus
// its live stock state. sharesStock reports whether state is
// another shop's (stockOwner in TS); stockOwnerID names that
// shop when it does.
type runtimeShop struct {
	definition   ShopDefinition
	itemPrices   map[int]int
	state        *shopStockState
	sharesStock  bool
	stockOwnerID int
}

// ShopManager is the ShopManager port. Construct with
// NewShopManager; the coordinator sets Hooks and Sender before
// ticking starts, loads definitions with Initialize, and drives
// Tick/ProcessPlayer from the game tick.
type ShopManager struct {
	items ShopItemDefinitionSource

	shops     map[int]*runtimeShop
	shopOrder []int

	activeShopByPlayer   map[*Player]int
	activeTargetByPlayer map[*Player]int
	viewersByShopID      map[int]map[*Player]bool

	restockTaskRunning bool
	currencyHandlers   map[string]ShopCurrencyHandler
	stockSources       []ShopStockSource

	// Hooks is the plugin-event seam (see shop_hooks.go). A
	// zero Hooks is inert.
	Hooks ShopHooks
	// Sender delivers shop interface traffic (see
	// ShopSender). Nil sends nothing.
	Sender ShopSender
	// IsShopInterfaceOpen, when set, reports whether the
	// player's client still shows the shop interface (the
	// PlayerStatus.SHOPPING + interface-id check in TS
	// isOpen). Nil means "assume open" — see the file-header
	// interface-state gap.
	IsShopInterfaceOpen func(p *Player) bool
}

// ShopStockSource names the shop version whose stock another
// version shares, or "" for none (ShopStockSource,
// ShopManager.ts: the TS source returns string | null |
// undefined; "" is the Go form of null/undefined).
type ShopStockSource func(definition ShopDefinition) string

// NewShopManager returns an empty ShopManager reading item
// definitions through items (nil answers the DEFAULT shape;
// see ShopItemDefinitionSource). Definitions arrive through
// Initialize; currency handlers start empty (TS registers
// none in core — coins and blood money work through the
// currency-item fallback, POINTS through its attribute).
func NewShopManager(items ShopItemDefinitionSource) *ShopManager {
	return &ShopManager{
		items:                items,
		shops:                make(map[int]*runtimeShop),
		activeShopByPlayer:   make(map[*Player]int),
		activeTargetByPlayer: make(map[*Player]int),
		viewersByShopID:      make(map[int]map[*Player]bool),
		currencyHandlers:     make(map[string]ShopCurrencyHandler),
	}
}

// Initialize ports ShopManager.initialize (ShopManager.ts):
// reload from definitions. The TS initialize also registers
// the quantity-mode attribute persistent
// (PlayerSave.persistAttribute); the Go coordinator must do
// that at boot (persist.PersistAttribute with no default) —
// the game package cannot reach persistence. It returns the
// number of shops loaded. Definitions must already be loaded
// (GameBuilder loads ShopDefinitionLoader before initialize).
func (m *ShopManager) Initialize(defs []ShopDefinition) int {
	return m.Reload(defs)
}

// Reload ports ShopManager.reload (ShopManager.ts): rebuild
// every runtime shop from definitions, re-link shared stock,
// close viewers of shops that no longer exist, and refresh the
// rest. It returns the number of shops loaded. Viewer and
// active-shop tracking survive the reload (TS clears only
// shopsById); a viewer whose shop vanished is closed by the
// loop below, and currentShop drops any other stale entry.
func (m *ShopManager) Reload(defs []ShopDefinition) int {
	m.shops = make(map[int]*runtimeShop, len(defs))
	m.shopOrder = nil
	for _, def := range defs {
		m.shops[def.ID] = m.createRuntimeShop(def)
		m.shopOrder = append(m.shopOrder, def.ID)
	}
	m.linkStock()

	viewed := make([]int, 0, len(m.viewersByShopID))
	for shopID := range m.viewersByShopID {
		viewed = append(viewed, shopID)
	}
	for _, shopID := range viewed {
		if _, ok := m.shops[shopID]; !ok {
			for _, player := range m.viewerList(shopID) {
				m.Close(player)
			}
			continue
		}
		m.refresh(shopID)
	}
	return len(m.shops)
}

// RegisterShop ports ShopManager.registerShop (ShopManager.ts):
// add a plugin-owned shop without touching the stock of the
// others (reload would reset them all). Remove it with
// RemoveShop. An id that already exists replaces its runtime
// shop in place, keeping its load position (JS Map.set
// semantics, as in the definition loader).
func (m *ShopManager) RegisterShop(def ShopDefinition) {
	if _, exists := m.shops[def.ID]; !exists {
		m.shopOrder = append(m.shopOrder, def.ID)
	}
	m.shops[def.ID] = m.createRuntimeShop(def)
	m.linkStock()
}

// HasShop reports whether a shop is registered under id
// (ShopManager.ts hasShop).
func (m *ShopManager) HasShop(shopID int) bool {
	_, ok := m.shops[shopID]
	return ok
}

// AddShopStock ports ShopManager.addShopStock (ShopManager.ts):
// put more of an item into a shop's stock; anything above its
// original amount falls back with the shop's restock timer. It
// reports false for an unknown shop or an amount that adds
// nothing, and refreshes viewers plus ensures the restock task
// on success.
func (m *ShopManager) AddShopStock(shopID, itemID, amount int) bool {
	shop, ok := m.shops[shopID]
	if !ok || !m.addStock(shop, itemID, amount) {
		return false
	}
	m.refresh(shopID)
	m.ensureRestockTask()
	return true
}

// ShareStock ports ShopManager.shareStock (ShopManager.ts): let
// versions of one store share a single stock. source names, for
// a shop definition, the version whose stock it shows and
// trades ("" for none). Each version keeps its own name,
// currency and prices, so a trade at either version moves the
// stock both show. It applies now and after every reload, and
// refreshes all current viewers.
func (m *ShopManager) ShareStock(source ShopStockSource) {
	m.stockSources = append(m.stockSources, source)
	m.linkStock()
	for _, shopID := range m.viewerShopIDs() {
		m.refresh(shopID)
	}
}

// linkStock ports ShopManager.linkStock (ShopManager.ts): apply
// the registered stock sources. Shops are matched by definition
// name (a later shop with the same name wins, as a JS Map.set
// does); an already-linked shop is skipped, an owner must not
// itself share stock, and a shop never links to itself. The
// first source that names a usable owner wins.
func (m *ShopManager) linkStock() {
	if len(m.stockSources) == 0 {
		return
	}
	byName := make(map[string]*runtimeShop, len(m.shops))
	for _, id := range m.shopOrder {
		byName[m.shops[id].definition.Name] = m.shops[id]
	}
	for _, id := range m.shopOrder {
		shop := m.shops[id]
		if shop.sharesStock {
			continue
		}
		for _, source := range m.stockSources {
			if source == nil {
				continue
			}
			owner := byName[source(shop.definition)]
			if owner == nil || owner == shop || owner.sharesStock {
				continue
			}
			shop.state = owner.state
			shop.sharesStock = true
			shop.stockOwnerID = owner.definition.ID
			break
		}
	}
}

// RemoveShop ports ShopManager.removeShop (ShopManager.ts):
// close every viewer, then drop the shop and its viewers.
func (m *ShopManager) RemoveShop(shopID int) {
	for _, player := range m.viewerList(shopID) {
		m.Close(player)
	}
	delete(m.shops, shopID)
	delete(m.viewersByShopID, shopID)
	for i, id := range m.shopOrder {
		if id == shopID {
			m.shopOrder = append(m.shopOrder[:i], m.shopOrder[i+1:]...)
			break
		}
	}
}

// Open ports ShopManager.open (ShopManager.ts): open shopID for
// player. It reports false for a nil player or an unknown shop.
// A CanShop denial reports TRUE without opening (TS returns
// true on denial — the open was handled, not failed). On
// success the shop:opened event fires after the interface
// opens. resetScroll is accepted for signature parity and
// ignored, as TS ignores its _resetScroll parameter.
func (m *ShopManager) Open(player *Player, shopID int, resetScroll bool, targetUID int) bool {
	_ = resetScroll // Ignored, as in TS (openInterface's _resetScroll).
	shop, ok := m.shops[shopID]
	if player == nil || !ok {
		return false
	}
	if m.Hooks.CanShop != nil {
		if allow := m.Hooks.CanShop(player, shopID); allow != nil && !*allow {
			return true
		}
	}

	opening := !m.hasActiveShop(player)
	m.setActiveShop(player, shopID, targetUID)
	opened := m.openInterface(player, shop, targetUID, opening)
	if opened && m.Hooks.OnShopOpened != nil {
		m.Hooks.OnShopOpened(&ShopOpenedEvent{
			Player:   player,
			ShopID:   shopID,
			ShopName: shop.definition.Name,
		})
	}
	return opened
}

// Close ports ShopManager.close (ShopManager.ts): drop the
// player's active shop, remove them from the viewers, and send
// the shop-close packet. A player with no active shop, and a
// nil player, are no-ops.
func (m *ShopManager) Close(player *Player) {
	if player == nil {
		return
	}
	shopID, ok := m.activeShopByPlayer[player]
	if !ok {
		return
	}
	if viewers := m.viewersByShopID[shopID]; viewers != nil {
		delete(viewers, player)
		if len(viewers) == 0 {
			delete(m.viewersByShopID, shopID)
		}
	}
	delete(m.activeShopByPlayer, player)
	delete(m.activeTargetByPlayer, player)
	if m.Sender != nil {
		m.Sender.SendShopClose(player)
	}
}

// ProcessPlayer ports ShopManager.processPlayer (World.ts
// process_players, after player.process()): close the shop of
// a player whose interface no longer shows it. Unwatched bots
// (IsBot with no attached session — the message.go bot guard:
// session presence is the watched distinction) and players
// with no active shop are skipped. With no IsShopInterfaceOpen
// seam installed this never closes (see the file header).
func (m *ShopManager) ProcessPlayer(player *Player) {
	if player == nil {
		return
	}
	if (player.IsBot && player.Session == nil) || !m.hasActiveShop(player) {
		return
	}
	if !m.isOpen(player) {
		m.Close(player)
	}
}

// ActiveShopID reports the shop id a player currently has open,
// or false when none. It is the Go form of the
// activeShopByPlayer lookup currentShop/getOpenShop build on,
// exposed for the coordinator and the dialogue seams.
func (m *ShopManager) ActiveShopID(player *Player) (int, bool) {
	if player == nil {
		return 0, false
	}
	id, ok := m.activeShopByPlayer[player]
	return id, ok
}

// ShopIDs returns the loaded shop ids in load order (the
// definition order Reload/Initialize received, with
// registerShop appends and removeShop removals).
func (m *ShopManager) ShopIDs() []int {
	return append([]int(nil), m.shopOrder...)
}

// ShopDefinition returns the definition a loaded shop was built
// from, or false for an unknown id.
func (m *ShopManager) ShopDefinition(shopID int) (ShopDefinition, bool) {
	shop, ok := m.shops[shopID]
	if !ok {
		return ShopDefinition{}, false
	}
	return shop.definition, true
}

// createRuntimeShop ports ShopManager.createRuntimeShop
// (ShopManager.ts): fold the definition's original stock into
// the live maps. Duplicate rows for one id sum their amounts;
// a row's restock ticks / price overwrite earlier rows for the
// same id (TS Map.set); the display order holds each id once,
// first-seen. originalSlotCount is the raw row count
// (duplicates included), as in TS.
func (m *ShopManager) createRuntimeShop(def ShopDefinition) *runtimeShop {
	stock := make([]ShopStockEntry, len(def.OriginalStock))
	copy(stock, def.OriginalStock)
	def.OriginalStock = stock
	state := &shopStockState{
		originalAmounts:  make(map[int]int),
		itemRestockTicks: make(map[int]int),
		stock:            make(map[int]int),
		changeTimers:     make(map[int]int),
	}
	seen := make(map[int]bool)
	itemPrices := make(map[int]int)
	for _, entry := range def.OriginalStock {
		state.originalAmounts[entry.ID] += entry.Amount
		state.stock[entry.ID] += entry.Amount
		if entry.RestockTicks != nil {
			state.itemRestockTicks[entry.ID] = *entry.RestockTicks
		}
		if entry.Price != nil {
			itemPrices[entry.ID] = *entry.Price
		}
		if !seen[entry.ID] {
			seen[entry.ID] = true
			state.order = append(state.order, entry.ID)
		}
	}
	state.originalSlotCount = len(def.OriginalStock)
	return &runtimeShop{definition: def, itemPrices: itemPrices, state: state}
}

// currentShop ports ShopManager.currentShop (ShopManager.ts):
// the player's open shop, closing the player first when the
// shop is no longer open. Nil when the player has no open shop
// or its shop vanished in a reload.
func (m *ShopManager) currentShop(player *Player) *runtimeShop {
	if !m.isOpen(player) {
		m.Close(player)
		return nil
	}
	shopID, ok := m.activeShopByPlayer[player]
	if !ok {
		return nil
	}
	return m.shops[shopID]
}

// isOpen ports ShopManager.isOpen (ShopManager.ts), reduced to
// what Go can observe: the player has an active-shop entry and
// the installed interface seam (when any) still shows the shop.
// TS additionally requires PlayerStatus.SHOPPING and interface
// id 3824/300; neither exists in Go (see the file header).
func (m *ShopManager) isOpen(player *Player) bool {
	if player == nil || !m.hasActiveShop(player) {
		return false
	}
	if m.IsShopInterfaceOpen != nil {
		return m.IsShopInterfaceOpen(player)
	}
	return true
}

// hasActiveShop reports whether the player has an active-shop
// entry (the activeShopByPlayer.has check in processPlayer).
func (m *ShopManager) hasActiveShop(player *Player) bool {
	_, ok := m.activeShopByPlayer[player]
	return ok
}

// setActiveShop ports ShopManager.setActiveShop
// (ShopManager.ts): close any previous shop, then record the
// new one and add the player to its viewers.
func (m *ShopManager) setActiveShop(player *Player, shopID, targetUID int) {
	m.Close(player)
	m.activeShopByPlayer[player] = shopID
	m.activeTargetByPlayer[player] = targetUID
	viewers := m.viewersByShopID[shopID]
	if viewers == nil {
		viewers = make(map[*Player]bool)
		m.viewersByShopID[shopID] = viewers
	}
	viewers[player] = true
}

// openInterface ports ShopManager.openInterface
// (ShopManager.ts): build the stock view in display order —
// each row priced for this player — and hand it to the sender.
// The interface-id/status sets and the frame sequence live with
// the sender (see ShopOpenView); the _resetScroll parameter
// does not exist here because TS ignores it. It always reports
// true, as in TS.
func (m *ShopManager) openInterface(player *Player, shop *runtimeShop, targetUID int, opening bool) bool {
	entries := m.displayEntries(shop, player)
	stock := make([]ShopSlotView, 0, len(entries))
	for slot, entry := range entries {
		def := m.itemDef(entry.ItemID)
		stock = append(stock, ShopSlotView{
			Slot:            slot,
			ItemID:          entry.ItemID,
			Quantity:        entry.Amount,
			DefaultQuantity: shop.state.originalAmounts[entry.ItemID],
			PriceEach:       m.buyPrice(player, shop, def, 0),
			SellPrice:       m.sellPrice(shop, def, player, 0),
		})
	}
	// TS sets the player's interface id before mounting the
	// shop interface (ShopManager.ts:550); a refresh re-open
	// is a same-id set and emits nothing.
	player.SetInterfaceId(ShopMainInterfaceID)
	if m.Sender != nil {
		m.Sender.SendShopOpen(player, ShopOpenView{
			ShopID:         shop.definition.ID,
			Title:          shopTitle(shop.definition.Name),
			CurrencyItemID: m.currencyItemID(shop.definition.Currency),
			GeneralStore:   m.isGeneralStore(shop),
			QuantityMode:   m.quantityMode(player),
			TargetUID:      targetUID,
			Opening:        opening,
			Stock:          stock,
		})
	}
	return true
}

// shopTitle ports ShopManager.title (ShopManager.ts): the name
// without the Wiki's " (shop)" disambiguation.
func shopTitle(name string) string {
	return strings.TrimSuffix(name, " (shop)")
}

// refresh ports ShopManager.refresh (ShopManager.ts): re-send
// the shop to its viewers. With stock sources registered, every
// shop sharing the changed stock object refreshes too (TS
// compares stock Map identity; Go compares the shared state
// pointer).
func (m *ShopManager) refresh(shopID int) {
	changed, ok := m.shops[shopID]
	if !ok {
		return
	}
	if len(m.stockSources) == 0 {
		m.refreshViewers(shopID)
		return
	}
	for _, id := range m.shopOrder {
		if id == shopID || m.shops[id].state == changed.state {
			m.refreshViewers(id)
		}
	}
}

// refreshViewers ports ShopManager.refreshViewers
// (ShopManager.ts): re-open the shop for every viewer whose
// interface still shows it, closing the rest. The viewer list
// is copied first: closing mutates the set (TS iterates
// Array.from(viewers)).
func (m *ShopManager) refreshViewers(shopID int) {
	shop, ok := m.shops[shopID]
	if !ok {
		return
	}
	for _, player := range m.viewerList(shopID) {
		if !m.isOpen(player) || m.activeShopByPlayer[player] != shopID {
			m.Close(player)
			continue
		}
		m.openInterface(player, shop, m.activeTargetByPlayer[player], false)
	}
}

// viewerList returns a snapshot of a shop's viewers.
func (m *ShopManager) viewerList(shopID int) []*Player {
	viewers := m.viewersByShopID[shopID]
	out := make([]*Player, 0, len(viewers))
	for player := range viewers {
		out = append(out, player)
	}
	return out
}

// viewerShopIDs returns a snapshot of the viewed shop ids.
func (m *ShopManager) viewerShopIDs() []int {
	out := make([]int, 0, len(m.viewersByShopID))
	for shopID := range m.viewersByShopID {
		out = append(out, shopID)
	}
	return out
}

// DisplayEntry is one visible stock row (DisplayEntry,
// ShopManager.ts): the item and its amount, in display order —
// an entry's index is its display slot.
type DisplayEntry struct {
	ItemID int
	Amount int
}

// displayEntries ports ShopManager.displayEntries
// (ShopManager.ts): the stock this player sees, in slot order.
// A stocked item keeps its slot at 0 when sold out; items
// players sold in leave when gone. Hidden stock (see
// stockVisible) takes no slot.
func (m *ShopManager) displayEntries(shop *runtimeShop, player *Player) []DisplayEntry {
	var entries []DisplayEntry
	for _, itemID := range shop.state.order {
		amount := shop.state.stock[itemID]
		_, original := shop.state.originalAmounts[itemID]
		if (amount > 0 || original) && m.stockVisible(player, shop, itemID) {
			entries = append(entries, DisplayEntry{ItemID: itemID, Amount: amount})
		}
	}
	return entries
}

// DisplayEntries returns the stock player sees in shopID, in
// display order, or false for an unknown shop. It exposes
// displayEntries for the coordinator and tests; production
// callers normally go through the open view.
func (m *ShopManager) DisplayEntries(shopID int, player *Player) ([]DisplayEntry, bool) {
	shop, ok := m.shops[shopID]
	if !ok {
		return nil, false
	}
	return m.displayEntries(shop, player), true
}

// stockVisible ports ShopManager.stockVisible (ShopManager.ts):
// the shop:stock-visible event. Content may hide stock from one
// player; hidden stock takes no slot. With no hook installed
// every entry is visible.
func (m *ShopManager) stockVisible(player *Player, shop *runtimeShop, itemID int) bool {
	if m.Hooks.OnStockVisible == nil {
		return true
	}
	request := &ShopStockVisibleEvent{
		Player:   player,
		ShopID:   shop.definition.ID,
		ShopName: shop.definition.Name,
		ItemID:   itemID,
		Visible:  true,
	}
	m.Hooks.OnStockVisible(request)
	return request.Visible
}

// itemAtDisplaySlot ports ShopManager.itemAtDisplaySlot
// (ShopManager.ts): the display entry at slot, or nil for an
// invalid slot. The display is recomputed per call, as in TS.
func (m *ShopManager) itemAtDisplaySlot(player *Player, shop *runtimeShop, slot int) *DisplayEntry {
	if slot < 0 {
		return nil
	}
	entries := m.displayEntries(shop, player)
	if slot >= len(entries) {
		return nil
	}
	entry := entries[slot]
	return &entry
}

// OpenShopEntry is one row of GetOpenShop's stock: the display
// entry plus the price this player pays for one.
type OpenShopEntry struct {
	ItemID int
	Amount int
	Price  int
}

// OpenShop is the player's open shop (ShopManager.ts
// getOpenShop): its name, its currency's display name, and its
// stock in display order, each row priced for the player.
type OpenShop struct {
	Name     string
	Currency string
	Stock    []OpenShopEntry
}

// GetOpenShop ports ShopManager.getOpenShop (ShopManager.ts).
// It reports nil when the player has no open shop.
func (m *ShopManager) GetOpenShop(player *Player) *OpenShop {
	shop := m.currentShop(player)
	if shop == nil {
		return nil
	}
	entries := m.displayEntries(shop, player)
	stock := make([]OpenShopEntry, 0, len(entries))
	for _, entry := range entries {
		stock = append(stock, OpenShopEntry{
			ItemID: entry.ItemID,
			Amount: entry.Amount,
			Price:  m.buyPrice(player, shop, m.itemDef(entry.ItemID), 0),
		})
	}
	return &OpenShop{
		Name:     shop.definition.Name,
		Currency: m.currencyName(shop.definition.Currency),
		Stock:    stock,
	}
}

// quantityMode ports ShopManager.quantityMode (ShopManager.ts):
// the player's quantity-row mode from its persistent attribute,
// sanitized to a valid mode (0 when unset, non-integral, or out
// of range — TS requires Number.isInteger).
func (m *ShopManager) quantityMode(player *Player) int {
	mode, ok := attributeInteger(player.GetAttribute(ShopQuantityModeAttribute))
	if ok && mode >= 0 && mode < len(shopQuantityByMode) {
		return mode
	}
	return 0
}

// QuantityMode reports the player's quantity-row mode (see
// quantityMode). It exposes the mode for the coordinator and
// tests; the widget handler sets it through SetQuantityMode.
func (m *ShopManager) QuantityMode(player *Player) int {
	if player == nil {
		return 0
	}
	return m.quantityMode(player)
}

// setQuantityMode ports ShopManager.setQuantityMode
// (ShopManager.ts): store the mode, then reopen the current
// shop so each item's left-click op rebuilds. With no open shop
// only the attribute is set.
func (m *ShopManager) setQuantityMode(player *Player, mode int) {
	player.SetAttribute(ShopQuantityModeAttribute, mode)
	if shop := m.currentShop(player); shop != nil {
		targetUID := m.activeTargetByPlayer[player]
		if _, ok := m.activeTargetByPlayer[player]; !ok {
			targetUID = DefaultShopTargetUID
		}
		m.openInterface(player, shop, targetUID, false)
	}
}

// attributeInteger coerces a stored attribute value to an
// integer when it is one: JSON numbers arrive as int or float64,
// and a float counts only when integral (TS Number.isInteger).
// Anything else reports false.
func attributeInteger(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		f := float64(v)
		if math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
			return 0, false
		}
		return int(f), true
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

// attributeNumber coerces a stored attribute value to an integer
// the way TS Number() does for the values shops store: JSON
// numbers arrive as int or float64 (and json.Number from a raw
// decode), and anything else counts as 0 (TS Number of a
// non-numeric value is NaN, which fails the integer check and
// yields the default just the same).
func attributeNumber(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case float32:
		return int(math.Trunc(float64(v)))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return 0
		}
		return int(math.Trunc(v))
	default:
		return 0
	}
}

// ActionAmount ports ShopManager.actionAmount (ShopManager.ts):
// the buy/sell amount a widget option names ("Buy 5", "Sell-10"
// with a space or hyphen, case-insensitive), else the amount
// for buttons 2-5 (1/5/10/50). It reports false when neither
// names an amount.
func ActionAmount(button int, option string) (int, bool) {
	if m := shopActionAmountPattern.FindStringSubmatch(strings.TrimSpace(option)); m != nil {
		amount := 0
		for _, c := range m[1] {
			amount = amount*10 + int(c-'0')
		}
		return amount, true
	}
	switch button {
	case 2:
		return 1, true
	case 3:
		return 5, true
	case 4:
		return 10, true
	case 5:
		return 50, true
	default:
		return 0, false
	}
}

// shopActionAmountPattern is /(?:buy|sell)[ -](1|5|10|50)$/i
// (ShopManager.ts actionAmount).
var shopActionAmountPattern = regexp.MustCompile(`(?i)(?:buy|sell)[ -](1|5|10|50)$`)

// ShopItemContainerActionKind is the kind of a shop container
// action (ShopItemContainerAction.kind, ShopManager.ts):
// "value" (price check), "buy_sell" (transact the amount), or
// "x" (prompt for an amount first).
type ShopItemContainerActionKind string

const (
	// ShopActionValue is kind "value": a price check.
	ShopActionValue ShopItemContainerActionKind = "value"
	// ShopActionBuySell is kind "buy_sell": buy/sell the
	// action's amount.
	ShopActionBuySell ShopItemContainerActionKind = "buy_sell"
	// ShopActionX is kind "x": prompt for the amount, then
	// buy/sell it.
	ShopActionX ShopItemContainerActionKind = "x"
)

// ShopItemContainerAction is one shop container click
// (ShopItemContainerAction, ShopManager.ts): which container
// (stock 3900 or inventory 3823), which slot, the item id the
// client claims is there, and the amount for buy_sell actions.
type ShopItemContainerAction struct {
	Kind        ShopItemContainerActionKind
	ContainerID int
	Slot        int
	ItemID      int
	Amount      int
}

// HandleItemContainerAction ports ShopManager.
// handleItemContainerAction (ShopManager.ts): route one shop
// container click to a price check, a buy, or a sell. It
// reports false when the player has no open shop or the
// container is neither the stock nor the shop inventory; every
// other outcome — including a stale slot or an empty slot —
// reports true (handled). The "x" kind arms an entered-amount
// prompt through the sender; with no sender the prompt is
// dropped (see ShopSender).
func (m *ShopManager) HandleItemContainerAction(player *Player, action ShopItemContainerAction) bool {
	shop := m.currentShop(player)
	if shop == nil {
		return false
	}
	fromShop := action.ContainerID == ShopItemsInterfaceID
	fromInventory := action.ContainerID == ShopInventoryInterfaceID
	if !fromShop && !fromInventory {
		return false
	}

	if fromShop {
		entry := m.itemAtDisplaySlot(player, shop, action.Slot)
		if entry == nil {
			return true
		}
		packetItemID := toUnsignedShort(action.ItemID)
		itemID := entry.ItemID
		if packetItemID > 0 && packetItemID == entry.ItemID {
			itemID = packetItemID
		}
		switch action.Kind {
		case ShopActionValue:
			m.priceCheck(player, shop, itemID, true, Item{})
		case ShopActionX:
			if m.Sender != nil {
				captured := itemID
				m.Sender.SendEnterAmountPrompt(player, "How many would you like to buy?",
					func(amount int) { m.buyItem(player, shop, captured, amount) })
			}
		default:
			m.buyItem(player, shop, itemID, action.Amount)
		}
		return true
	}

	inventory := player.Inventory()
	if inventory == nil || action.Slot < 0 || action.Slot >= inventory.Capacity() {
		return true
	}
	inventoryItem := inventory.ItemAt(action.Slot)
	if inventoryItem.ID() <= 0 {
		return true
	}
	itemID := inventoryItem.ID()
	switch action.Kind {
	case ShopActionValue:
		m.priceCheck(player, shop, itemID, false, inventoryItem)
	case ShopActionX:
		if m.Sender != nil {
			captured := inventoryItem
			slot := action.Slot
			m.Sender.SendEnterAmountPrompt(player, "How many would you like to sell?",
				func(amount int) { m.sellItem(player, shop, captured, slot, amount) })
		}
	default:
		m.sellItem(player, shop, inventoryItem, action.Slot, action.Amount)
	}
	return true
}

// ShopWidgetAction is one shop widget click
// (ShopManager.ts handleWidgetAction's packet): the widget
// group/child, the button number, the option text when the
// client sent one, and the slot/item id when the click carried
// them. HasSlot/HasItemID distinguish "absent" from 0, as TS
// distinguishes null from a number (packet.slot == null
// returns false; a packet itemId is only trusted when present
// and > 0).
type ShopWidgetAction struct {
	GroupID   int
	ChildID   int
	ButtonNum int
	Option    string
	Slot      int
	HasSlot   bool
	ItemID    int
	HasItemID bool
}

// HandleWidgetAction ports ShopManager.handleWidgetAction
// (ShopManager.ts): route one widget click while a shop is
// open — the quantity-row buttons, the stock grid (child 16 of
// group 300, slots 1-based), and the sell inventory (child 0
// of group 301, slots 0-based). It reports false when no shop
// is open, when the click carries no slot, or when the click
// belongs to neither grid; a stale or mismatched slot reports
// true (handled, ignored). Examine clicks (button 10 or the
// "examine" option) examine the item instead of trading it.
func (m *ShopManager) HandleWidgetAction(player *Player, packet ShopWidgetAction) bool {
	if !m.isOpen(player) {
		return false
	}
	if mode, ok := shopQuantityModeByButton[packet.ChildID]; ok && packet.GroupID == ShopMainInterfaceID {
		m.setQuantityMode(player, mode)
		return true
	}
	if !packet.HasSlot {
		return false
	}
	option := strings.ToLower(strings.TrimSpace(packet.Option))
	selected := shopQuantityByMode[m.quantityMode(player)]
	amount, hasAmount := ActionAmount(packet.ButtonNum, packet.Option)
	if !hasAmount && packet.GroupID == ShopMainInterfaceID && packet.ButtonNum == 1 && option == "" && selected > 0 {
		amount, hasAmount = selected, true
	}
	examine := packet.ButtonNum == 10 || option == "examine"

	if packet.GroupID == ShopMainInterfaceID && packet.ChildID == 16 {
		slot := packet.Slot - 1
		shop := m.currentShop(player)
		var item *DisplayEntry
		if shop != nil {
			item = m.itemAtDisplaySlot(player, shop, slot)
		}
		// The stock can shift (sold-in items leave), so the
		// clicked slot must still hold the item the client
		// showed there.
		if item == nil || (packet.HasItemID && packet.ItemID > 0 && packet.ItemID != item.ItemID) {
			return true
		}
		if examine {
			ExamineItem(player, item.ItemID, item.Amount, m.items)
		} else {
			kind := ShopActionValue
			amt := 0
			if hasAmount {
				kind, amt = ShopActionBuySell, amount
			}
			m.HandleItemContainerAction(player, ShopItemContainerAction{
				Kind: kind, ContainerID: ShopItemsInterfaceID,
				Slot: slot, ItemID: item.ItemID, Amount: amt,
			})
		}
		return true
	}

	if packet.GroupID == ShopSideInterfaceID && packet.ChildID == 0 {
		inventory := player.Inventory()
		if inventory == nil {
			return true
		}
		item := inventory.ItemAt(packet.Slot)
		if item.ID() < 0 {
			return true
		}
		if examine {
			ExamineItem(player, item.ID(), item.Amount(), m.items)
		} else {
			kind := ShopActionValue
			amt := 0
			if hasAmount {
				kind, amt = ShopActionBuySell, amount
			}
			m.HandleItemContainerAction(player, ShopItemContainerAction{
				Kind: kind, ContainerID: ShopInventoryInterfaceID,
				Slot: packet.Slot, ItemID: item.ID(), Amount: amt,
			})
		}
		return true
	}
	return false
}

// normalizeAmount ports ShopManager.normalizeAmount
// (ShopManager.ts): a finite value floored and clamped to >= 0.
// Go amounts are already integers, so this is the clamp.
func normalizeAmount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}

// toUnsignedShort ports ShopManager.toUnsignedShort
// (ShopManager.ts): the value masked to 16 bits.
func toUnsignedShort(value int) int {
	return value & 0xffff
}

// isSafeInteger reports Number.isSafeInteger for an int: within
// ±(2^53 - 1). Hook-set prices must pass it (buyPrice /
// sellPrice).
func isSafeInteger(value int) bool {
	return value <= maxSafeInteger && value >= -maxSafeInteger
}

// satMul multiplies saturating at maxSafeInteger, for payment
// totals a hook-inflated price could otherwise overflow: TS
// computes them in float64, where the same inputs stay finite.
// Amounts above the safe-integer bound cannot be paid anyway
// (currency amounts are int32-truncated on read), so the
// saturation is unobservable in any payable transaction.
func satMul(a, b int) int {
	if a == 0 || b == 0 {
		return 0
	}
	if a > 0 && b > 0 && a > maxSafeInteger/b {
		return maxSafeInteger
	}
	if a < 0 || b < 0 {
		return 0
	}
	return a * b
}

// insertCommas ports Misc.insertCommasToNumber (Misc.ts:335-341)
// for non-negative integers: groups of three digits from the
// right, comma-separated ("400" stays "400", "1234567" becomes
// "1,234,567"). It is also the Go form of Number.toLocaleString
// ("en-US") for the integer values the examine price line
// formats. Negative inputs keep a leading "-" (TS formats only
// non-negative prices and counts; the sign handling just keeps
// the helper total).
func insertCommas(value int) string {
	if value < 0 {
		return "-" + insertCommas(-value)
	}
	digits := ""
	if value == 0 {
		digits = "0"
	}
	for n := value; n > 0; n /= 10 {
		digits = string(rune('0'+n%10)) + digits
	}
	if len(digits) < 4 {
		return digits
	}
	var out strings.Builder
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	out.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		out.WriteByte(',')
		out.WriteString(digits[i : i+3])
	}
	return out.String()
}
