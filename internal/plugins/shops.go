// Shop hooks, shop-currency members, and the scoped shop core surface:
// the Go form of the shop families of PluginApi (PluginTypes.ts) that the
// engine can fire since phase 17: onCanShop and onCanStockItem, plus the
// registerShopCurrency / registerItemShopCurrency members and the
// ShopManager / ShopDefinition core surface (PluginCoreApi).
//
// Every emit below ports its PluginManager.emit* body, including the
// entry guards, the registration wrappers, the hook ordering, and the
// short-circuit/run-all shape — the families differ from each other,
// and those differences are behavior content relies on:
//
//   - emitCanShop (PluginManager.ts:1534-1546): first-answer gate
//     (runFirstAnswerGate, gates.go); the zero-hooks early return is
//     unobservable (the gate returns nil with no hooks). The
//     registration wrapper skips events without a player
//     (PluginManager.ts:3767-3779); a null shopId is NOT skipped —
//     the emit's shopId parameter defaults to null, and handlers
//     must read HasShopID before ShopID.
//   - emitCanStockItem (PluginManager.ts:1682-1685, via firstAnswer
//     1639-1652): first-answer gate; the zero-hooks early return is
//     unobservable. NO registration wrapper (PluginManager.ts:
//     3838-3840): the handler runs unconditionally.
//
// EMIT SITES (Worker A / coordinator; this file owns only the hub side):
//
//   - emitCanShop fires from ShopManager.open (ShopManager.ts:241):
//     a false answer makes open return TRUE — a gated shop counts as
//     handled (the click is claimed, no interface opens, no fallback
//     message). A nil answer (no hooks, or silence) opens normally.
//     Handlers may open a different shop re-entrantly (the skillcape
//     copies in Sellers.SkillCapes.js open the copy and veto the
//     original); the hub holds no lock across hook execution, so a
//     nested emit is safe.
//   - emitCanStockItem fires per stock entry at shop-definition load
//     (ShopDefinitionLoader.ts:183): `!== false` keeps the entry, so
//     a nil answer keeps the stock. It fires DURING GameBuilder load,
//     AFTER plugins load (Server.ts:282 loads plugins, :290 builds
//     the game) — hooks must already be registered when the shop
//     definitions load. In Go, buildServer already loads the hub
//     before the definition Store; the coordinator must preserve
//     that order and wire Hub.EmitCanStockItem into the shop loader
//     path (Worker A owns the definition extension). A hook
//     registered after the load never sees the entries.
//
// SHOP CURRENCY (registerShopCurrency / registerItemShopCurrency,
// PluginTypes.ts:1338-1346; createApi passthroughs,
// PluginManager.ts:4610-4614, delegating to ShopManager
// .registerCurrency / .registerItemCurrency, ShopManager.ts:105-138):
// the currency map lives in Worker A's ShopManager, which does not
// exist at plugin-load time, so registrations QUEUE until the
// coordinator installs the ShopCurrencyRegistry (the RegisterArea /
// provider-registry precedent). Validation is EAGER, at the Api
// call: an invalid registration panics, mirroring the TS throw, so
// a bad call during Register fails that plugin's load through the
// loader's recovery — exactly as a TS throw does — instead of
// failing the boot at flush time.
//
// CORE SURFACE (following the getObjectManager / ObjectManagerAPI
// precedent in objects.go, but WITHOUT an Api member: TS has no
// getShopManager member — ShopManager and ShopDefinition reach
// content only through core, PluginTypes.ts:1741-1742, built in
// getCoreApi, PluginManager.ts:2790-2791): Core.ShopManager
// returns the installed ShopManagerAPI, Core.ShopDefinition
// returns the installed ShopDefinitionRegistry. Both are nil until
// the coordinator installs them; content must check for nil. The
// surface is scoped to what content demonstrably calls and the
// engine can serve — see ShopManagerAPI / ShopDefinitionRegistry
// for the included and excluded members, each grounded.
//
// SHOP CUSTOM EVENTS: the shop:* events fire through the hub's
// custom-event mechanism (Api.OnCustomEvent / Hub.EmitCustomEvent)
// at the ShopManager sites; there is no dedicated registration
// helper in TS. The name constants below are the exact TS strings
// (grep-verified in ShopManager.ts); payload shapes are owned by
// Worker A's ShopManager and pass through the hub opaquely as
// `any`, following the npc-dialogue:* precedent (dialogue.go).
// The seven events the phase 17 plan names are joined by three
// more found at the emit sites (shop:buy-price, shop:buys-back,
// shop:can-sell); all ten are listed so content subscribes to
// real names only.
//
// NOT PORTED: onCanBank / onCanBankItem stay table-only (the bank
// open flow is milestone 8); registerShop has no Go surface (a
// ShopDefinition cannot be constructed in Go — the definition
// package builds them from JSON only — so there is nothing to
// register); ShopManager.handleWidgetAction /
// handleItemContainerAction have no core-surface form (their only
// TS callers outside the widget chain are bots, NOT STARTED);
// the per-player shop:points attribute and the QUANTITY_MODE
// attribute are ShopManager internals, not hub members.

package plugins

import (
	"fmt"

	"tsps-go/internal/game"
)

// CanShopEvent is the payload of the onCanShop hooks. It mirrors
// PluginCanShopEvent (PluginTypes.ts:471-475): the player, the
// shop id (null when the query names no shop), and the allow
// answer. Allow is a *bool: nil is TS null (undecided), non-nil
// is the hook's answer. A hook sets Allow through BoolPtr (gates.go).
// HasShopID distinguishes "no shop named" (TS null) from shop id 0.
type CanShopEvent struct {
	Player    *game.Player
	ShopID    int
	HasShopID bool
	Allow     *bool
}

// CanStockItemEvent is the payload of the onCanStockItem hooks. It
// mirrors PluginCanStockItemEvent (PluginTypes.ts:364-368): the
// shop id, the item id, and the allow answer. Allow is a *bool:
// nil is TS null (undecided — the entry is kept), non-nil is the
// hook's answer. A hook sets Allow through BoolPtr (gates.go).
// There is no player on this event: it fires at load time.
type CanStockItemEvent struct {
	ShopID int
	ItemID int
	Allow  *bool
}

// --- hookRegistry storage for the shop families (see hooks.go) ---

func (r *hookRegistry) addCanShop(e hookEntry[*CanShopEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canShop = append(r.canShop, e)
}

func (r *hookRegistry) snapshotCanShop() []hookEntry[*CanShopEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanShopEvent](nil), r.canShop...)
}

func (r *hookRegistry) addCanStockItem(e hookEntry[*CanStockItemEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canStockItem = append(r.canStockItem, e)
}

func (r *hookRegistry) snapshotCanStockItem() []hookEntry[*CanStockItemEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanStockItemEvent](nil), r.canStockItem...)
}

// --- registration methods ---

// OnCanShop registers a can-shop hook, mirroring onCanShop
// (PluginTypes.ts:1124). Hooks run in registration order until
// one answers (EmitCanShop). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3767-3779). The
// stored handler wraps the plugin's with the TS guard: an event
// without a player never reaches the plugin's handler. A missing
// shop id (HasShopID false) is NOT guarded: TS passes shopId null
// through, and the wrapper checks the player only.
func (a *Api) OnCanShop(handler func(*CanShopEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanShop") {
		return
	}
	a.hub.hooks.addCanShop(hookEntry[*CanShopEvent]{pluginName: a.name, handler: func(event *CanShopEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnCanStockItem registers a can-stock-item hook, mirroring
// onCanStockItem (PluginTypes.ts:1139). Hooks run in registration
// order until one answers (EmitCanStockItem). A nil handler is
// ignored silently, mirroring the createApi guard
// (PluginManager.ts:3838-3840). There is no registration wrapper:
// the TS registration pushes the bare handler.
func (a *Api) OnCanStockItem(handler func(*CanStockItemEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanStockItem") {
		return
	}
	a.hub.hooks.addCanStockItem(hookEntry[*CanStockItemEvent]{pluginName: a.name, handler: handler})
}

// --- Hub emits ---

// EmitCanShop asks the registered hooks whether player may open a
// shop, mirroring PluginManager.emitCanShop
// (PluginManager.ts:1534-1546) with label "can_shop": the first
// non-nil Allow wins, later hooks never run, and no answer yields
// nil. shopID names the shop when hasShopID is true; hasShopID
// false is the TS default (shopId null). A nil Hub or a Hub
// without a registry yields nil. A nil player yields nil through
// the registration wrappers (which skip player-less events), so
// no emit-level player guard is added — the outcome is identical
// to TS, where the wrappers do the same work.
//
// The production call site is ShopManager.open (Worker A), through
// the game-side shop seam: only a false answer gates the open
// (ShopManager.ts:241, `=== false` returns true); nil opens
// normally. See the file header for the re-entrancy note.
func (h *Hub) EmitCanShop(player *game.Player, shopID int, hasShopID bool) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &CanShopEvent{Player: player, ShopID: shopID, HasShopID: hasShopID}
	return runFirstAnswerGate(h.Logger(), "can_shop", h.hooks.snapshotCanShop(), event,
		func(e *CanShopEvent) *bool { return e.Allow })
}

// EmitCanStockItem asks the registered hooks whether one stock
// entry may load, mirroring PluginManager.emitCanStockItem
// (PluginManager.ts:1682-1685, via firstAnswer 1639-1652) with
// label "can_stock_item": the first non-nil Allow wins, later
// hooks never run, and no answer yields nil. A nil Hub or a Hub
// without a registry yields nil.
//
// The production call site is the shop-definition load (Worker A):
// only a false answer drops the entry (ShopDefinitionLoader.ts:183,
// `!== false` keeps it). It must run after Hub.Load — see the file
// header for the boot-order requirement.
func (h *Hub) EmitCanStockItem(shopID, itemID int) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &CanStockItemEvent{ShopID: shopID, ItemID: itemID}
	return runFirstAnswerGate(h.Logger(), "can_stock_item", h.hooks.snapshotCanStockItem(), event,
		func(e *CanStockItemEvent) *bool { return e.Allow })
}

// --- Shop currencies ---

// ShopCurrencyHandler is the Go form of the handler object of
// registerShopCurrency (PluginTypes.ts:1338-1341) and
// ShopCurrencyHandler (ShopManager.ts:22-36): the balance reader
// and the add/remove operations over a player, plus the handler's
// display name, the inventory item the currency is paid in (when
// it is one), and the unit word live shop messages use ("trade
// sticks"). Content passes a struct literal, mirroring the TS
// object literal. Amount/Add/Remove must be non-nil: TS
// registerCurrency throws when any of them is not a function
// (ShopManager.ts:105-110), and Api.RegisterShopCurrency panics
// on the same condition. Name is carried verbatim (TS never
// validates handler.name); ItemID/Unit are absent unless their
// Has* flag is set, mirroring the optional TS fields.
type ShopCurrencyHandler struct {
	// Amount answers the player's balance of this currency.
	Amount func(player *game.Player) int
	// Add credits amount of this currency to the player.
	Add func(player *game.Player, amount int)
	// Remove debits amount of this currency from the player.
	Remove func(player *game.Player, amount int)
	// Name is the handler's display name (handler.name).
	Name string
	// ItemID is the inventory item the currency is paid in,
	// when HasItemID (handler.itemId). The shop manager reads
	// it to price the currency item itself and to name the
	// currency in the shop interface (ShopManager.ts:1125,
	// 1285, 1299-1310).
	ItemID int
	// HasItemID reports ItemID was supplied.
	HasItemID bool
	// Unit is how live shop messages name an amount of this
	// currency ("trade sticks"), when HasUnit (handler.unit).
	Unit string
	// HasUnit reports Unit was supplied.
	HasUnit bool
}

// ShopItemCurrencyOptions is the options object of
// registerItemShopCurrency (PluginTypes.ts:1343-1346): the
// currency name (default: the item's definition name, else
// "Item <id>"), extra lookup keys, and the unit word. Name is
// used only when HasName: TS resolves
// `options?.name ?? ItemDefinition name ?? "Item ${itemId}"`
// (ShopManager.ts:119-122), and ?? triggers only on
// null/undefined, so an explicitly empty name is used as-is
// (and then registers under no key, since blank keys are
// skipped). Unit follows the same rule (ShopManager.ts:126).
// Aliases registers the same handler under each additional
// key; nil and empty mean no aliases.
type ShopItemCurrencyOptions struct {
	Name    string
	HasName bool
	Aliases []string
	Unit    string
	HasUnit bool
}

// ShopCurrencyRegistry is the narrow currency-registry surface
// Api.RegisterShopCurrency / Api.RegisterItemShopCurrency push
// onto: the Go form of the currencyHandlers map writes in
// ShopManager.registerCurrency / registerItemCurrency
// (ShopManager.ts:105-138). It follows the AreaRegistry
// precedent (areas.go): the interface is declared here, in the
// plugins package, so game never imports plugins; the
// coordinator's cmd/server adapter implements it over Worker
// A's ShopManager. The adapter owns the TS key rules:
// RegisterCurrency stores the handler under name VERBATIM (TS
// registerCurrency does no normalization — call sites that
// want uppercase keys uppercase them, e.g. Shopkeepers
// .plugin.js); RegisterItemCurrency resolves the display name
// (options name, else the item definition name, else
// "Item <id>"), builds the inventory-backed handler (amount /
// adds / deleteNumber over the player's inventory), and
// registers it under the name plus each alias, each trimmed
// and uppercased, skipping blank keys
// (ShopManager.ts:112-138). Validation of name/handler/itemId
// happens eagerly in the Api methods (which panic, mirroring
// the TS throws); the adapter receives only valid calls.
type ShopCurrencyRegistry interface {
	// RegisterCurrency stores handler under name, verbatim.
	RegisterCurrency(name string, handler ShopCurrencyHandler)
	// RegisterItemCurrency registers itemID as a shop currency
	// under its resolved name plus aliases (see above).
	RegisterItemCurrency(itemID int, opts ShopItemCurrencyOptions)
}

// shopCurrencyRegistration is one queued currency registration:
// either a named handler or an item currency, applied in
// registration order when the registry is installed.
type shopCurrencyRegistration struct {
	isItem  bool
	name    string
	handler ShopCurrencyHandler
	itemID  int
	opts    ShopItemCurrencyOptions
}

// shopCurrencyRegistrationTarget returns the installed currency
// registry for entry to be pushed onto, or queues entry (in
// registration order) and returns nil when no registry is
// installed yet. The caller pushes onto a returned registry
// outside the lock. It mirrors areaRegistrationTarget (hooks.go).
func (r *hookRegistry) shopCurrencyRegistrationTarget(entry shopCurrencyRegistration) ShopCurrencyRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.shopCurrencyRegistry != nil {
		return r.shopCurrencyRegistry
	}
	r.queuedShopCurrencies = append(r.queuedShopCurrencies, entry)
	return nil
}

// installShopCurrencyRegistry installs reg and returns the
// registrations queued before installation, in registration
// order, for the caller to push outside the lock. Installing
// nil clears the registry but keeps the queue. It mirrors
// installAreaRegistry (hooks.go).
func (r *hookRegistry) installShopCurrencyRegistry(reg ShopCurrencyRegistry) []shopCurrencyRegistration {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shopCurrencyRegistry = reg
	if reg == nil {
		return nil
	}
	queued := r.queuedShopCurrencies
	r.queuedShopCurrencies = nil
	return queued
}

func (r *hookRegistry) installedShopCurrencyRegistry() ShopCurrencyRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shopCurrencyRegistry
}

// applyShopCurrencyRegistration pushes one queued registration
// onto reg. It is the single application path for both the
// flush in SetShopCurrencyRegistry and direct registration.
func applyShopCurrencyRegistration(reg ShopCurrencyRegistry, entry shopCurrencyRegistration) {
	if entry.isItem {
		reg.RegisterItemCurrency(entry.itemID, entry.opts)
		return
	}
	reg.RegisterCurrency(entry.name, entry.handler)
}

// SetShopCurrencyRegistry installs the registry
// Api.RegisterShopCurrency / Api.RegisterItemShopCurrency push
// onto, flushing every registration queued before installation,
// in registration order. The coordinator calls it once at boot,
// after Worker A's ShopManager exists; calling it with nil
// clears the installed registry but keeps the queue (a later
// non-nil install flushes it), mirroring SetAreaRegistry. It is
// a no-op on a nil Hub or a Hub without a registry. The flush
// runs outside the registry lock, as SetAreaRegistry's does.
// Install order relative to shop-definition load does not
// affect stock: currency handlers are consulted at price time,
// not load time — but the registry must be installed before
// the first shop opens, or custom-currency shops price against
// the default coin handler.
func (h *Hub) SetShopCurrencyRegistry(reg ShopCurrencyRegistry) {
	if h == nil || h.hooks == nil {
		return
	}
	queued := h.hooks.installShopCurrencyRegistry(reg)
	for _, entry := range queued {
		applyShopCurrencyRegistration(reg, entry)
	}
}

// ShopCurrencyRegistry returns the installed currency registry,
// or nil when none is installed (or the Hub is nil). It mirrors
// Hub.AreaRegistry: the read half of the install seam, for the
// coordinator and tests.
func (h *Hub) ShopCurrencyRegistry() ShopCurrencyRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedShopCurrencyRegistry()
}

// RegisterShopCurrency registers a custom shop currency,
// mirroring the PluginApi registerShopCurrency member
// (PluginTypes.ts:1338-1341) and its createApi passthrough
// (PluginManager.ts:4610-4612), which calls
// ShopManager.registerCurrency directly.
//
// Validation mirrors the TS throw exactly
// (ShopManager.ts:105-110): an empty name, or a handler whose
// Amount/Add/Remove is nil, panics with the TS message
// ("Invalid shop currency registration: <name>") — never an
// error return, never a silent drop. A panic during Register
// is recovered by the loader, which logs failed-to-initialize
// and drops the plugin, exactly as a TS throw fails that
// plugin's load. The name is stored verbatim (no trimming, no
// uppercasing): TS registerCurrency stores the key as given.
//
// RegisterShopCurrency has NO load-phase gate, like
// RegisterArea: the TS closure has no phase check, and both
// load-time registration (queued until the registry is
// installed) and later registration (direct) are legitimate.
// A nil Api, or an Api with a nil hub, is a no-op.
func (a *Api) RegisterShopCurrency(name string, handler ShopCurrencyHandler) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if name == "" || handler.Amount == nil || handler.Add == nil || handler.Remove == nil {
		panic(fmt.Errorf("Invalid shop currency registration: %s", name))
	}
	entry := shopCurrencyRegistration{name: name, handler: handler}
	if reg := a.hub.hooks.shopCurrencyRegistrationTarget(entry); reg != nil {
		reg.RegisterCurrency(entry.name, entry.handler)
	}
}

// RegisterItemShopCurrency registers an inventory item as a
// shop currency, mirroring the PluginApi
// registerItemShopCurrency member (PluginTypes.ts:1343-1346)
// and its createApi passthrough (PluginManager.ts:4613-4615),
// which calls ShopManager.registerItemCurrency directly.
//
// Validation mirrors the TS throw exactly
// (ShopManager.ts:112-115): an item id that is not a positive
// integer panics with the TS message ("Invalid item shop
// currency: <id>"). Go ints are always integers, so only the
// positivity check has a Go form. Name/alias/unit resolution
// lives in the adapter (see ShopCurrencyRegistry): this method
// validates and queues only, so a bad id fails the plugin's
// load at registration time, not the boot at flush time.
//
// RegisterItemShopCurrency has NO load-phase gate, like
// RegisterShopCurrency. A nil Api, or an Api with a nil hub,
// is a no-op.
func (a *Api) RegisterItemShopCurrency(itemID int, opts ShopItemCurrencyOptions) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if itemID <= 0 {
		panic(fmt.Errorf("Invalid item shop currency: %d", itemID))
	}
	entry := shopCurrencyRegistration{isItem: true, itemID: itemID, opts: opts}
	if reg := a.hub.hooks.shopCurrencyRegistrationTarget(entry); reg != nil {
		reg.RegisterItemCurrency(entry.itemID, entry.opts)
	}
}

// --- Scoped ShopManager / ShopDefinition core surface ---

// ShopStockEntry is one original-stock row of a ShopDefinition:
// the Go form of ShopStockDefinition (ShopDefinition.ts:20-25),
// with plugins-local types so this package never imports the
// definition package (the ObjectDefinition precedent,
// objects.go). RestockTicks and Price are nil when the row
// configures none (TS null).
type ShopStockEntry struct {
	ID           int
	Amount       int
	RestockTicks *int
	Price        *int
}

// ShopSalesPolicy is a shop's Wiki pricing policy: the Go form
// of ShopSalesPolicy (ShopDefinition.ts:8-18), with
// plugins-local types. Each field is nil when the shop
// configures no value (TS undefined). Rates are per mille of
// the item's value.
type ShopSalesPolicy struct {
	GeneralStore *bool
	BuysNothing  *bool
	SellsAt      *int
	BuysAt       *int
	ChangePer    *int
}

// ShopDefinition is the minimal shop-definition surface content
// reads: the ShopDefinition accessors (ShopDefinition.ts:64-101)
// that plugin code demonstrably calls — getId/getName (shop
// lookup by name, e.g. NpcDialogues.plugin.js, TaiBwoWannai
// Cleanup), getCurrency/getSource (the dialogue shop-step
// disambiguation reads source), getOriginalStock (Recipe for
// Disaster reads one row's price), and the restock-tick and
// sales-policy getters. The definition package's
// *ShopDefinition does NOT satisfy this interface: its
// OriginalStock/SalesPolicy return definition-local types. The
// coordinator's adapter wraps it (translating the two value
// types); the hub never constructs a definition.
type ShopDefinition interface {
	// ID is the shop id (getId).
	ID() int
	// Name is the shop name (getName).
	Name() string
	// Currency is the shop currency key (getCurrency).
	Currency() string
	// OriginalStock is the shop's original stock rows
	// (getOriginalStock).
	OriginalStock() []ShopStockEntry
	// DefaultRestockTicks is the shop-wide restock interval
	// (getDefaultRestockTicks).
	DefaultRestockTicks() int
	// DefaultDestockTicks is the shop-wide destock interval
	// (getDefaultDestockTicks).
	DefaultDestockTicks() int
	// SoldItemDestockTicks is the destock interval for items
	// players sold to the shop (getSoldItemDestockTicks).
	SoldItemDestockTicks() int
	// Source is the contributing source's name (getSource):
	// "core" for the shops.json shops.
	Source() string
	// SalesPolicy is the shop's pricing policy
	// (getSalesPolicy).
	SalesPolicy() ShopSalesPolicy
}

// ShopDefinitionRegistry is the Go-idiomatic equivalent of the
// ShopDefinition class's static lookup surface
// (ShopDefinition.forId / .all, ShopDefinition.ts:58-64;
// core.ShopDefinition, PluginManager.ts:2791): content looks
// shops up by id or enumerates them to find one by name. The
// coordinator installs it from the definition Store behind a
// thin adapter (see ShopDefinition). Deliberately absent:
// register/unregister (a ShopDefinition cannot be constructed
// in Go — the definition package builds them from JSON only —
// so there is nothing to register; plugin shops arrive through
// definition sources, not this surface) and replace (a load
// operation, not a content operation). A nil registry means
// uninstalled: content must check for nil before calling.
type ShopDefinitionRegistry interface {
	// ForID returns the shop definition for id, or false
	// (ShopDefinition.forId).
	ForID(id int) (ShopDefinition, bool)
	// All returns every loaded shop definition, in load order
	// (ShopDefinition.all).
	All() []ShopDefinition
}

// OpenShopStockEntry is one row of an open shop's stock view:
// the Go form of the stock objects getOpenShop returns
// (ShopManager.ts:418-431).
type OpenShopStockEntry struct {
	ItemID int
	Amount int
	Price  int
}

// OpenShopView is the open-shop view getOpenShop returns
// (ShopManager.ts:418-431): the shop's name, its currency's
// display name, and the stock this player sees, in display
// order, each row priced for this player.
type OpenShopView struct {
	Name     string
	Currency string
	Stock    []OpenShopStockEntry
}

// ShopStockSource names the shop version whose stock another
// version shares, or "" for none: the Go form of ShopStockSource
// (ShopManager.ts:51). TS returns string | null | undefined;
// "" covers both null and undefined. See ShopManagerAPI.ShareStock.
type ShopStockSource func(def ShopDefinition) string

// ShopManagerAPI is the Go-idiomatic equivalent of core.ShopManager
// (PluginTypes.ts:1741; PluginManager.ts:2790): the shop
// operations content needs — open/close, lookup, stock, and
// restock — reduced to the narrow surface the Go engine can
// honestly supply. TS exposes the ShopManager class itself (a
// static singleton); Go returns this interface, installed by
// the coordinator at boot from Worker A's ShopManager behind a
// thin adapter.
//
// Included, each grounded in ShopManager.ts and in demonstrated
// plugin use: Open / Close (the NPC/dialogue/admin open paths
// and the close side of every flow), HasShop (the registered
// check), GetOpenShop (the MCP shop view), Reload (the admin
// reload command), AddShopStock (content top-ups),
// RemoveShop (the per-player-shop teardown), and ShareStock
// (the Karamja gloves shared-stock versions).
//
// Deliberately absent: registerShop (no constructible
// ShopDefinition — see ShopDefinitionRegistry), initialize /
// processPlayer / restockAll (engine-internal boot/tick
// plumbing, driven by the coordinator, not by content),
// handleWidgetAction / handleItemContainerAction (their only
// TS callers outside the widget chain are bots, NOT STARTED;
// the widget chain reaches them through the listeners seam,
// not through Core), actionAmount (a pure helper of
// handleWidgetAction, not a content operation), and the
// interface-id constants (they live with Worker A's port and
// the listeners seam, not on this surface). Open takes no
// targetUid: TS open defaults it to (161<<16)|16, and the only
// caller passing another uid is the custom-interface runtime
// (milestone 8); the adapter always uses the default. A nil
// ShopManagerAPI means the shop system is not installed:
// content must check for nil before calling.
type ShopManagerAPI interface {
	// Open opens shopID for player, returning whether the shop
	// opened (ShopManager.open, ShopManager.ts:231-256).
	// resetScroll rebuilds the stock scroll position (both
	// the NPC and dialogue call sites pass true). A gated
	// shop (emitCanShop false) returns true with no interface
	// opened: the click is claimed, as in TS.
	Open(player *game.Player, shopID int, resetScroll bool) bool
	// Close closes player's open shop, if any
	// (ShopManager.close, ShopManager.ts:257-273).
	Close(player *game.Player)
	// HasShop reports whether a shop is registered under
	// shopID (ShopManager.hasShop, ShopManager.ts:175-177).
	HasShop(shopID int) bool
	// GetOpenShop returns player's open-shop view, or nil
	// when no shop is open (ShopManager.getOpenShop,
	// ShopManager.ts:418-431).
	GetOpenShop(player *game.Player) *OpenShopView
	// Reload rebuilds every runtime shop from the loaded
	// definitions and returns how many shops exist
	// (ShopManager.reload, ShopManager.ts:145-164).
	Reload() int
	// AddShopStock puts more of an item into a shop's stock
	// and refreshes its viewers, returning false when the
	// shop does not exist or the stock cannot grow
	// (ShopManager.addShopStock, ShopManager.ts:180-189).
	AddShopStock(shopID, itemID, amount int) bool
	// RemoveShop closes the shop for its viewers and removes
	// it (ShopManager.removeShop, ShopManager.ts:222-229).
	RemoveShop(shopID int)
	// ShareStock lets versions of one store share a single
	// stock: source names, for a shop definition, the version
	// whose stock it shows and trades (ShopManager.shareStock,
	// ShopManager.ts:195-201). It applies now and after every
	// reload.
	ShareStock(source ShopStockSource)
}

// SetShopManager installs the ShopManagerAPI content reads
// through Core.ShopManager. The coordinator calls it once at
// boot, after Worker A's ShopManager exists; calling it with
// nil clears the surface. It is a no-op on a nil Hub or a Hub
// without a registry.
func (h *Hub) SetShopManager(api ShopManagerAPI) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setShopManager(api)
}

// ShopManager returns the installed ShopManagerAPI, or nil when
// none is installed (or the Hub is nil).
func (h *Hub) ShopManager() ShopManagerAPI {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.shopManager()
}

// SetShopDefinitionRegistry installs the ShopDefinitionRegistry
// content reads through Core.ShopDefinition. The coordinator
// calls it once at boot, from the definition Store; calling it
// with nil clears the surface. It is a no-op on a nil Hub or a
// Hub without a registry.
func (h *Hub) SetShopDefinitionRegistry(reg ShopDefinitionRegistry) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setShopDefinitionRegistry(reg)
}

// ShopDefinitionRegistry returns the installed registry, or nil
// when none is installed (or the Hub is nil).
func (h *Hub) ShopDefinitionRegistry() ShopDefinitionRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedShopDefinitionRegistry()
}

func (r *hookRegistry) setShopManager(api ShopManagerAPI) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shopManagerAPI = api
}

func (r *hookRegistry) shopManager() ShopManagerAPI {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shopManagerAPI
}

func (r *hookRegistry) setShopDefinitionRegistry(reg ShopDefinitionRegistry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shopDefinitionRegistry = reg
}

func (r *hookRegistry) installedShopDefinitionRegistry() ShopDefinitionRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.shopDefinitionRegistry
}

// --- shop:* custom-event names ---
//
// The payloads are built by Worker A's ShopManager and pass
// through the hub opaquely as `any` (handlers receive pointers
// to the game-side payload structs, as the npc-dialogue:*
// precedent). Field lists below are the TS request objects at
// each emit site, for content authors; the game-side structs
// are Worker A's to define.

// ShopOpenedEventName is emitted when a shop opens
// (ShopManager.ts:248): {player, shopId, shopName}.
const ShopOpenedEventName = "shop:opened"

// ShopStockVisibleEventName lets content hide stock from one
// player (ShopManager.ts:633): {player, shopId, shopName,
// itemId, visible} — visible read back, false hides the entry
// (and its slot).
const ShopStockVisibleEventName = "shop:stock-visible"

// ShopBuyLimitEventName lets content lower how many of the
// stock one player can buy (ShopManager.ts:725): {player,
// shopId, itemId, available, original, limit, message} — limit
// and message read back.
const ShopBuyLimitEventName = "shop:buy-limit"

// ShopDeliveryEventName lets content change the delivered item
// or bundle size (ShopManager.ts:744): {player, shopId,
// currency, itemId, deliverId, amount, captureDelivery} —
// deliverId, amount, and captureDelivery read back.
const ShopDeliveryEventName = "shop:delivery"

// ShopPurchaseEventName fires after a purchase lands in the
// inventory (ShopManager.ts:777): {player, shopId, shopName,
// itemId, amount, originalStock, currency} plus, when the
// delivery asked for a receipt, {deliverId, deliveredSlot,
// deliveredItem}.
const ShopPurchaseEventName = "shop:purchase"

// ShopKeepStockEventName lets content keep the stock as it was
// on a purchase, or on a sale with selling set
// (ShopManager.ts:898, 993): {player, shopId, itemId, amount,
// keep} — keep read back; the sale form adds selling: true.
const ShopKeepStockEventName = "shop:keep-stock"

// ShopSellPriceEventName lets content change what one sale pays
// (ShopManager.ts:1198): {player, shopId, shopName, currency,
// itemId, buyPrice, price} — price read back (safe integer >= 0
// wins, else the computed price).
const ShopSellPriceEventName = "shop:sell-price"

// ShopBuyPriceEventName lets content change what one purchase
// costs (ShopManager.ts:1119): {player, shopId, shopName,
// currency, itemId, price} — price read back (safe integer >= 0
// wins, else the computed price).
const ShopBuyPriceEventName = "shop:buy-price"

// ShopBuysBackEventName lets content say whether a
// non-buyback-currency shop buys an item back
// (ShopManager.ts:1044): {shopId, shopName, currency, itemId,
// buys} — buys read back (only exactly true buys).
const ShopBuysBackEventName = "shop:buys-back"

// ShopCanSellEventName lets content refuse a sale for one player
// (ShopManager.ts:1104): {player, shopId, shopName, currency,
// itemId, sellable} — sellable read back (only exactly true
// sells).
const ShopCanSellEventName = "shop:can-sell"
