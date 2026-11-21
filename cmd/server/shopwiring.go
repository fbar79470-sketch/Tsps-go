package main

import (
	"bytes"
	"encoding/json"
	"log"
	"strconv"
	"strings"

	"tsps-go/internal/cache"
	"tsps-go/internal/content/shopkeepers"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
)

// Shop + trade wiring: the cmd/server adapters between the game
// layer's ShopManager/Trading, the session layer's packet sends,
// the definition Store, and the plugin hub. It ports the
// PacketSender side of ShopManager.openInterface/close and the
// hub-seam population for both systems as glue, following the
// ground-item and dialogue wiring precedents: the game layer
// never imports protocol/definition/plugins, and this package
// imports all of them.
//
// Boot order (grounded in TS Server.ts:282/290 + GameBuilder):
// the hub loads first (newPluginHub, inside buildServer), then
// installCanStockItemHook binds the stock veto into the
// definition loader, then loadGameDefinitions loads the shops
// (the veto fires per stock entry), then newShopManager builds
// the runtime shops from the loaded definitions. The dialogue
// playback is built after the manager exists, so its shop seams
// bind the live opener.

// newShopItemDefinitionSource returns the ShopItemDefinitionSource
// for store, or nil when store is nil (the game layer degrades
// to its documented no-definition defaults — see
// newItemDefinitionSource, which this mirrors for the wider
// shop surface).
func newShopItemDefinitionSource(store *definition.Store, defs *cache.CacheDefinitions) game.ShopItemDefinitionSource {
	if store == nil {
		return nil
	}
	return storeItemDefinitions{store: store, defs: defs}
}

// convertShopDefinitions converts every loaded shop definition
// into the game package's shape, field for field, in load order
// (see game.ShopDefinition). The pointer fields are shared with
// the loaded definitions, which are immutable after Load; the
// manager only reads them. A nil store yields no definitions.
func convertShopDefinitions(store *definition.Store) []game.ShopDefinition {
	if store == nil {
		return nil
	}
	loaded := store.Shops()
	out := make([]game.ShopDefinition, 0, len(loaded))
	for _, def := range loaded {
		stock := def.OriginalStock()
		entries := make([]game.ShopStockEntry, len(stock))
		for i, row := range stock {
			entries[i] = game.ShopStockEntry{ID: row.ID, Amount: row.Amount, RestockTicks: row.RestockTicks, Price: row.Price}
		}
		policy := def.SalesPolicy()
		out = append(out, game.ShopDefinition{
			ID:                   def.ID(),
			Name:                 def.Name(),
			Currency:             def.Currency(),
			OriginalStock:        entries,
			DefaultRestockTicks:  def.DefaultRestockTicks(),
			DefaultDestockTicks:  def.DefaultDestockTicks(),
			SoldItemDestockTicks: def.SoldItemDestockTicks(),
			Source:               def.Source(),
			SalesPolicy: game.ShopSalesPolicy{
				GeneralStore: policy.GeneralStore,
				BuysNothing:  policy.BuysNothing,
				SellsAt:      policy.SellsAt,
				BuysAt:       policy.BuysAt,
				ChangePer:    policy.ChangePer,
			},
		})
	}
	return out
}

// newShopManager builds the ShopManager over the loaded Store,
// installs the session sender, and initializes the runtime shops
// from the loaded definitions (ShopManager.initialize,
// GameBuilder.ts:51). A nil store yields an empty manager that
// answers the DEFAULT item shape and holds no shops.
func newShopManager(store *definition.Store, defs *cache.CacheDefinitions) *game.ShopManager {
	manager := game.NewShopManager(newShopItemDefinitionSource(store, defs))
	manager.Sender = shopSender{}
	if store != nil {
		manager.Initialize(convertShopDefinitions(store))
	}
	return manager
}

// shopSender implements game.ShopSender by reaching the player's
// session through the SessionPort interface (the groundItemSender
// precedent): a session-less player sends nothing. Stateless: one
// shared instance serves the manager.
type shopSender struct{}

// SendShopOpen delivers one shop-open view as the frame sequence
// of ShopManager.openInterface (ShopManager.ts:531-569): the
// quantity varbit, the two sub-interfaces, script 1074 (title +
// currency), the main flags range, script 149 (sell options),
// the side flags range, the inventory container, and
// encodeShopOpen. The interface-id/status sets have no Go
// equivalent (no PlayerStatus/interface-id state); the
// CONTAINER_OPEN sound is skipped (no sound system —
// ShopOpenView.Opening is carried for a future sender).
func (shopSender) SendShopOpen(p *game.Player, view game.ShopOpenView) {
	if p == nil || p.Session == nil {
		return
	}
	send := p.Session.SendPacket
	send(protocol.EncodeVarbit(shopQuantityVarbit, view.QuantityMode))
	send(protocol.EncodeWidgetOpenSub(view.TargetUID, shopMainInterfaceID, 0, protocol.WidgetOpenSubOptions{}))
	send(protocol.EncodeWidgetOpenSub(shopSideTargetUID, shopSideInterfaceID, 1, protocol.WidgetOpenSubOptions{}))
	send(protocol.EncodeWidgetRunScript(1074, []protocol.ScriptArg{
		protocol.ScriptArgInt(516),
		protocol.ScriptArgString(view.Title),
		protocol.ScriptArgInt(view.CurrencyItemID),
		protocol.ScriptArgInt(0),
		protocol.ScriptArgInt(1),
	}, nil, nil, nil))
	send(protocol.EncodeWidgetSetFlagsRange((shopMainInterfaceID<<16)|16, 0, 299, 1662))
	send(protocol.EncodeWidgetRunScript(149, []protocol.ScriptArg{
		protocol.ScriptArgInt(shopSideInterfaceID << 16),
		protocol.ScriptArgInt(93),
		protocol.ScriptArgInt(4),
		protocol.ScriptArgInt(7),
		protocol.ScriptArgInt(0),
		protocol.ScriptArgInt(-1),
		protocol.ScriptArgString("Value"),
		protocol.ScriptArgString("Sell 1"),
		protocol.ScriptArgString("Sell 5"),
		protocol.ScriptArgString("Sell 10"),
		protocol.ScriptArgString("Sell 50"),
	}, nil, nil, nil))
	send(protocol.EncodeWidgetSetFlagsRange(shopSideInterfaceID<<16, 0, 27, 1086))
	// sendItemContainer for an Inventory ignores the interface
	// id and sends the inventory snapshot (PacketSender.ts:
	// 1042-1050) — exactly what the container refresh hook
	// sends, so RefreshItems is the equivalent call.
	if inv := p.Inventory(); inv != nil {
		inv.RefreshItems()
	}
	stock := make([]protocol.ShopSlotView, len(view.Stock))
	for i, row := range view.Stock {
		stock[i] = protocol.ShopSlotView{
			Slot:            row.Slot,
			ItemID:          row.ItemID,
			Quantity:        row.Quantity,
			DefaultQuantity: row.DefaultQuantity,
			PriceEach:       row.PriceEach,
			SellPrice:       row.SellPrice,
		}
	}
	send(protocol.EncodeShopOpen(strconv.Itoa(view.ShopID), view.Title, view.CurrencyItemID, view.GeneralStore, 1, 1, stock))
}

// SendShopClose delivers encodeShopClose (ShopManager.close,
// ShopManager.ts:257-273, via the session).
func (shopSender) SendShopClose(p *game.Player) {
	if p == nil || p.Session == nil {
		return
	}
	p.Session.SendPacket(protocol.EncodeShopClose())
}

// SendEnterAmountPrompt arms the entered-amount action and sends
// the amount prompt (sendEnterAmountPrompt, PacketSender.ts:446,
// via sendClientScript/encodeRunClientScript, script 108). The
// dialogue_amount dispatch case (opcode 192) runs the armed
// execute closure with the entered amount — the shop's buy/sell
// closures (ShopManager.ts:319,347). A session-less player
// cannot answer, so nothing is armed.
func (shopSender) SendEnterAmountPrompt(p *game.Player, prompt string, execute func(amount int)) {
	if p == nil || p.Session == nil {
		return
	}
	p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: execute})
	p.Session.SendPacket(protocol.EncodeRunClientScript(108, []protocol.ScriptArg{protocol.ScriptArgString(prompt)}))
}

// Shop interface ids (ShopManager.ts:75-79, 92) and the
// side-interface target uid (openInterface's (161 << 16) | 79).
const (
	shopQuantityVarbit  = 6348
	shopMainInterfaceID = 300
	shopSideInterfaceID = 301
	shopSideTargetUID   = (161 << 16) | 79
)

// installShopHooks populates the manager's plugin-event seam
// from the hub: the CanShop gate and the ten shop:* custom
// events, each fired with the game-side payload struct passed
// opaquely (the npc-dialogue:* precedent — handlers read and
// mutate the request fields, and the manager reads them back).
// A nil hub or manager leaves the seam inert (the no-hooks
// defaults).
func installShopHooks(manager *game.ShopManager, hub *plugins.Hub) {
	if manager == nil || hub == nil {
		return
	}
	manager.Hooks.CanShop = func(player *game.Player, shopID int) *bool {
		return hub.EmitCanShop(player, shopID, true)
	}
	manager.Hooks.OnShopOpened = func(e *game.ShopOpenedEvent) {
		hub.EmitCustomEvent(plugins.ShopOpenedEventName, e)
	}
	manager.Hooks.OnStockVisible = func(e *game.ShopStockVisibleEvent) {
		hub.EmitCustomEvent(plugins.ShopStockVisibleEventName, e)
	}
	manager.Hooks.OnBuyLimit = func(e *game.ShopBuyLimitEvent) {
		hub.EmitCustomEvent(plugins.ShopBuyLimitEventName, e)
	}
	manager.Hooks.OnDelivery = func(e *game.ShopDeliveryEvent) {
		hub.EmitCustomEvent(plugins.ShopDeliveryEventName, e)
	}
	manager.Hooks.OnPurchase = func(e *game.ShopPurchaseEvent) {
		hub.EmitCustomEvent(plugins.ShopPurchaseEventName, e)
	}
	manager.Hooks.OnKeepStock = func(e *game.ShopKeepStockEvent) {
		hub.EmitCustomEvent(plugins.ShopKeepStockEventName, e)
	}
	manager.Hooks.OnSellPrice = func(e *game.ShopSellPriceEvent) {
		hub.EmitCustomEvent(plugins.ShopSellPriceEventName, e)
	}
	manager.Hooks.OnBuyPrice = func(e *game.ShopBuyPriceEvent) {
		hub.EmitCustomEvent(plugins.ShopBuyPriceEventName, e)
	}
	manager.Hooks.OnBuysBack = func(e *game.ShopBuysBackEvent) {
		hub.EmitCustomEvent(plugins.ShopBuysBackEventName, e)
	}
	manager.Hooks.OnCanSell = func(e *game.ShopCanSellEvent) {
		hub.EmitCustomEvent(plugins.ShopCanSellEventName, e)
	}
}

// installCanStockItemHook binds the plugin stock veto into the
// definition loader (emitCanStockItem, ShopDefinitionLoader
// .ts:183, fired per stock entry at load). It must run BEFORE
// loadGameDefinitions: the hook is read only during Load. A nil
// hub leaves the loader's default (keep every entry). "No
// opinion" (nil) keeps the entry, matching TS's !== false check.
func installCanStockItemHook(hub *plugins.Hub) {
	if hub == nil {
		return
	}
	definition.CanStockItemHook = func(shopID, itemID int) bool {
		answer := hub.EmitCanStockItem(shopID, itemID)
		return answer == nil || *answer
	}
}

// installTradeHooks populates the game package's trade hook seam
// (game.TradeHookSeam) from the hub and the engine systems: the
// three trade hook families, the trade:item-eligibility custom
// event, the ground-item fallback for aborted offers, and the
// post-trade save. It runs once at boot, before ticking starts;
// a zero seam is inert, so a nil hub/saves/manager degrades to
// standalone core trading (no hook delivery, no ground fallback,
// no post-trade save — the coordinator MUST wire saves and
// ground items in production, or aborted offers vanish and
// completed trades persist only at the next autosave).
//
// GetVarbit reads the player's live varbit state through the
// session's varp store (PacketSender.getVarbit,
// PacketSender.ts:344-348): the session implements the game's
// VarbitReader extension, and the closure asserts it, returning 0
// for a nil player, a session-less player, or a session without
// the extension — the fresh-store default, so the accept-delay
// rule (Trading.ts:367, varbit 13130) only triggers once the
// player's own session has recorded the setting. It is wired
// unconditionally, independent of the hub: the store is
// per-session, not per-plugin.
func installTradeHooks(hub *plugins.Hub, saves *saveCoordinator, groundItems *game.GroundItemManager, logger *log.Logger) {
	if logger == nil {
		logger = log.Default()
	}
	seam := game.TradeHooks{}
	seam.GetVarbit = func(player *game.Player, id int) int {
		if player == nil || player.Session == nil {
			return 0
		}
		reader, ok := player.Session.(game.VarbitReader)
		if !ok {
			return 0
		}
		return reader.GetVarbit(id)
	}
	if hub != nil {
		seam.EmitCanTrade = func(player, target *game.Player) *bool {
			return hub.EmitCanTrade(player, target)
		}
		seam.EmitTradeRequest = func(e *game.TradeRequestEvent) bool {
			pe := &plugins.TradeRequestEvent{Player: e.Player, Target: e.Target, Handled: e.Handled}
			handled := hub.EmitTradeRequest(pe)
			e.Handled = pe.Handled
			return handled
		}
		seam.EmitTradeCompleted = func(e *game.TradeCompletedEvent) {
			hub.EmitTradeCompleted(&plugins.TradeCompletedEvent{
				Player: e.Player, Partner: e.Partner, Received: e.Received, Given: e.Given,
			})
		}
		// trade:item-eligibility (Trading.ts:461-469, canOffer):
		// the request starts with allowed=false and hooks may
		// set it true; the adapter reads it back after the emit.
		seam.ItemEligibility = func(item game.Item) bool {
			req := &game.TradeItemEligibilityRequest{Item: item}
			hub.EmitCustomEvent("trade:item-eligibility", req)
			return req.Allowed
		}
	}
	if groundItems != nil {
		// ItemOnGroundManager.registers(player, item)
		// (Trading.abort): the stack registers at the player's
		// tile, owner-only until the reveal delay, then
		// global — the same construction the drop listener
		// uses for its registers() branch.
		seam.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
			if owner == nil {
				return
			}
			stack := game.NewItemOnGround(
				game.GroundItemSeenByPlayer,
				owner.Username,
				loc,
				item.ID(),
				item.Amount(),
				tradeItemMetaToGround(item.Meta()),
				true, -1, false, nil,
			)
			groundItems.Register(stack)
		}
	}
	if saves != nil {
		// Trading.save (Trading.ts:493-500): save after a
		// completed trade, logging failures with the TS
		// message. Bots and players without a carried save
		// are refused by snapshot itself.
		seam.SavePlayer = func(player *game.Player, reason string) {
			if player == nil {
				return
			}
			if err := saves.snapshot(player, reason); err != nil {
				logger.Printf("[trade] Failed to save %s after a trade: %v", player.Username, err)
			}
		}
	}
	game.TradeHookSeam = seam
}

// tradeItemMetaToGround converts an item's metadata to the
// ground-item map form: each RawMessage value JSON-decodes (with
// json.Number, preserving integer precision), falling back to
// the raw string on a decode failure. It mirrors
// listeners.itemMetaToGround, which the drop listener uses for
// the same conversion; the two must stay in step.
func tradeItemMetaToGround(meta map[string]json.RawMessage) map[string]any {
	if meta == nil {
		return nil
	}
	out := make(map[string]any, len(meta))
	for key, raw := range meta {
		var value any
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			out[key] = string(raw)
			continue
		}
		out[key] = value
	}
	return out
}

// shopCurrencyRegistryAdapter implements
// plugins.ShopCurrencyRegistry over the ShopManager: Api
// .RegisterShopCurrency / RegisterItemShopCurrency push onto it
// (queued until installed, in registration order — see
// plugins/shops.go). Validation happens eagerly in the Api
// methods (which panic, mirroring the TS throws), so the
// adapter receives only valid calls and ignores the manager's
// errors.
type shopCurrencyRegistryAdapter struct {
	manager *game.ShopManager
}

var _ plugins.ShopCurrencyRegistry = shopCurrencyRegistryAdapter{}

// RegisterCurrency stores the handler under name verbatim (TS
// registerCurrency does no normalization).
func (a shopCurrencyRegistryAdapter) RegisterCurrency(name string, handler plugins.ShopCurrencyHandler) {
	if a.manager == nil {
		return
	}
	itemID := 0
	if handler.HasItemID {
		itemID = handler.ItemID
	}
	unit := ""
	if handler.HasUnit {
		unit = handler.Unit
	}
	_ = a.manager.RegisterCurrency(name, game.ShopCurrencyHandler{
		Amount: handler.Amount,
		Add:    handler.Add,
		Remove: handler.Remove,
		Name:   handler.Name,
		ItemID: itemID,
		Unit:   unit,
	})
}

// RegisterItemCurrency registers an inventory item as a shop
// currency, following ShopManager.registerItemCurrency
// (ShopManager.ts:112-138): the display name is the option name,
// else the item's definition name, else "Item <id>"; the handler
// reads the inventory amount and adds/removes through it; the
// handler registers under the name plus each alias, each trimmed
// and uppercased, skipping blank keys. The common case delegates
// to the manager's RegisterItemCurrency, which ports exactly
// that; the explicit-empty-name case cannot delegate (the
// manager's "" means "fall back to the definition name", while
// TS's ?? keeps an explicit "" — which then registers under no
// key, only under aliases), so it registers the alias keys
// directly with a handler named "".
func (a shopCurrencyRegistryAdapter) RegisterItemCurrency(itemID int, opts plugins.ShopItemCurrencyOptions) {
	if a.manager == nil {
		return
	}
	unit := ""
	if opts.HasUnit {
		unit = opts.Unit
	}
	if opts.HasName && opts.Name == "" {
		handler := game.ShopCurrencyHandler{
			Name:   "",
			ItemID: itemID,
			Unit:   unit,
			Amount: func(player *game.Player) int {
				if player == nil || player.Inventory() == nil {
					return 0
				}
				return player.Inventory().GetAmount(itemID)
			},
			Add: func(player *game.Player, amount int) {
				if player == nil || player.Inventory() == nil {
					return
				}
				player.Inventory().Adds(itemID, amount)
			},
			Remove: func(player *game.Player, amount int) {
				if player == nil || player.Inventory() == nil {
					return
				}
				player.Inventory().DeleteNumber(itemID, amount)
			},
		}
		for _, key := range opts.Aliases {
			if strings.TrimSpace(key) != "" {
				_ = a.manager.RegisterCurrency(strings.ToUpper(strings.TrimSpace(key)), handler)
			}
		}
		return
	}
	name := ""
	if opts.HasName {
		name = opts.Name
	}
	_ = a.manager.RegisterItemCurrency(itemID, game.ItemCurrencyOptions{
		Name:    name,
		Aliases: opts.Aliases,
		Unit:    unit,
	})
}

// shopManagerAPIAdapter implements plugins.ShopManagerAPI over
// the ShopManager (see plugins/shops.go for the surface's
// scoping). A nil manager degrades every method to the
// no-shop answer.
type shopManagerAPIAdapter struct {
	manager *game.ShopManager
	store   *definition.Store
}

var _ plugins.ShopManagerAPI = shopManagerAPIAdapter{}

func (a shopManagerAPIAdapter) Open(player *game.Player, shopID int, resetScroll bool) bool {
	if a.manager == nil {
		return false
	}
	return a.manager.Open(player, shopID, resetScroll, game.DefaultShopTargetUID)
}

func (a shopManagerAPIAdapter) Close(player *game.Player) {
	if a.manager == nil {
		return
	}
	a.manager.Close(player)
}

func (a shopManagerAPIAdapter) HasShop(shopID int) bool {
	if a.manager == nil {
		return false
	}
	return a.manager.HasShop(shopID)
}

func (a shopManagerAPIAdapter) GetOpenShop(player *game.Player) *plugins.OpenShopView {
	if a.manager == nil {
		return nil
	}
	open := a.manager.GetOpenShop(player)
	if open == nil {
		return nil
	}
	stock := make([]plugins.OpenShopStockEntry, len(open.Stock))
	for i, row := range open.Stock {
		stock[i] = plugins.OpenShopStockEntry{ItemID: row.ItemID, Amount: row.Amount, Price: row.Price}
	}
	return &plugins.OpenShopView{Name: open.Name, Currency: open.Currency, Stock: stock}
}

// Reload rebuilds every runtime shop from the loaded definitions
// (ShopManager.reload). Runtime-registered shops (RegisterShop)
// do not survive: the Store is the only definition registry Go
// has, and no RegisterShop callers exist yet — the first
// content shop registered at runtime will need the reload to
// merge them, as TS reload re-reads the shared static registry.
func (a shopManagerAPIAdapter) Reload() int {
	if a.manager == nil {
		return 0
	}
	return a.manager.Reload(convertShopDefinitions(a.store))
}

func (a shopManagerAPIAdapter) AddShopStock(shopID, itemID, amount int) bool {
	if a.manager == nil {
		return false
	}
	return a.manager.AddShopStock(shopID, itemID, amount)
}

func (a shopManagerAPIAdapter) RemoveShop(shopID int) {
	if a.manager == nil {
		return
	}
	a.manager.RemoveShop(shopID)
}

func (a shopManagerAPIAdapter) ShareStock(source plugins.ShopStockSource) {
	if a.manager == nil {
		return
	}
	a.manager.ShareStock(func(def game.ShopDefinition) string {
		if source == nil {
			return ""
		}
		return source(shopDefinitionAdapter{def: def})
	})
}

// shopDefinitionAdapter implements plugins.ShopDefinition over a
// game.ShopDefinition value (the ShareStock callback shape).
type shopDefinitionAdapter struct {
	def game.ShopDefinition
}

var _ plugins.ShopDefinition = shopDefinitionAdapter{}

func (a shopDefinitionAdapter) ID() int          { return a.def.ID }
func (a shopDefinitionAdapter) Name() string     { return a.def.Name }
func (a shopDefinitionAdapter) Currency() string { return a.def.Currency }

// NpcInteractions is empty: game.ShopDefinition carries no
// trader rows (they live on the loaded definition, exposed by
// definitionShopAdapter below).
func (a shopDefinitionAdapter) NpcInteractions() []plugins.ShopNpcInteraction { return nil }
func (a shopDefinitionAdapter) OriginalStock() []plugins.ShopStockEntry {
	stock := make([]plugins.ShopStockEntry, len(a.def.OriginalStock))
	for i, row := range a.def.OriginalStock {
		stock[i] = plugins.ShopStockEntry{ID: row.ID, Amount: row.Amount, RestockTicks: row.RestockTicks, Price: row.Price}
	}
	return stock
}
func (a shopDefinitionAdapter) DefaultRestockTicks() int  { return a.def.DefaultRestockTicks }
func (a shopDefinitionAdapter) DefaultDestockTicks() int  { return a.def.DefaultDestockTicks }
func (a shopDefinitionAdapter) SoldItemDestockTicks() int { return a.def.SoldItemDestockTicks }
func (a shopDefinitionAdapter) Source() string            { return a.def.Source }
func (a shopDefinitionAdapter) SalesPolicy() plugins.ShopSalesPolicy {
	p := a.def.SalesPolicy
	return plugins.ShopSalesPolicy{
		GeneralStore: p.GeneralStore, BuysNothing: p.BuysNothing,
		SellsAt: p.SellsAt, BuysAt: p.BuysAt, ChangePer: p.ChangePer,
	}
}

// definitionShopAdapter implements plugins.ShopDefinition over a
// loaded definition.Store shop (the registry shape).
type definitionShopAdapter struct {
	def *definition.ShopDefinition
}

var _ plugins.ShopDefinition = definitionShopAdapter{}

func (a definitionShopAdapter) ID() int          { return a.def.ID() }
func (a definitionShopAdapter) Name() string     { return a.def.Name() }
func (a definitionShopAdapter) Currency() string { return a.def.Currency() }
func (a definitionShopAdapter) OriginalStock() []plugins.ShopStockEntry {
	stock := a.def.OriginalStock()
	out := make([]plugins.ShopStockEntry, len(stock))
	for i, row := range stock {
		out[i] = plugins.ShopStockEntry{ID: row.ID, Amount: row.Amount, RestockTicks: row.RestockTicks, Price: row.Price}
	}
	return out
}
func (a definitionShopAdapter) DefaultRestockTicks() int  { return a.def.DefaultRestockTicks() }
func (a definitionShopAdapter) DefaultDestockTicks() int  { return a.def.DefaultDestockTicks() }
func (a definitionShopAdapter) SoldItemDestockTicks() int { return a.def.SoldItemDestockTicks() }
func (a definitionShopAdapter) Source() string            { return a.def.Source() }
func (a definitionShopAdapter) NpcInteractions() []plugins.ShopNpcInteraction {
	traders := a.def.NpcInteractions()
	out := make([]plugins.ShopNpcInteraction, len(traders))
	for i, trader := range traders {
		out[i] = plugins.ShopNpcInteraction{NPCIDs: append([]int(nil), trader.NPCIDs...), OptionSlot: trader.OptionSlot}
	}
	return out
}
func (a definitionShopAdapter) SalesPolicy() plugins.ShopSalesPolicy {
	p := a.def.SalesPolicy()
	return plugins.ShopSalesPolicy{
		GeneralStore: p.GeneralStore, BuysNothing: p.BuysNothing,
		SellsAt: p.SellsAt, BuysAt: p.BuysAt, ChangePer: p.ChangePer,
	}
}

// shopDefinitionRegistryAdapter implements
// plugins.ShopDefinitionRegistry over the definition Store.
type shopDefinitionRegistryAdapter struct {
	store *definition.Store
}

var _ plugins.ShopDefinitionRegistry = shopDefinitionRegistryAdapter{}

func (a shopDefinitionRegistryAdapter) ForID(id int) (plugins.ShopDefinition, bool) {
	if a.store == nil {
		return nil, false
	}
	def, ok := a.store.Shop(id)
	if !ok {
		return nil, false
	}
	return definitionShopAdapter{def: def}, true
}

func (a shopDefinitionRegistryAdapter) All() []plugins.ShopDefinition {
	if a.store == nil {
		return nil
	}
	loaded := a.store.Shops()
	out := make([]plugins.ShopDefinition, len(loaded))
	for i, def := range loaded {
		out[i] = definitionShopAdapter{def: def}
	}
	return out
}

// tradeStateReader is the Core.TradeState seam over the trading
// system (see plugins/trades.go): the ordinals of game.TradeState
// and plugins.TradeState are aligned by construction (both
// NONE..ACCEPTED_CONFIRM_SCREEN in TS order), so the conversion
// is a cast — pinned by TestTradeStateOrdinalsAligned. A nil
// player has no trading state to read.
func tradeStateReader(player *game.Player) (plugins.TradeState, bool) {
	if player == nil {
		return plugins.TradeStateNone, false
	}
	return plugins.TradeState(player.Trading().State()), true
}

// installShopHubSurfaces installs the shop/trade core surfaces
// content reads through Core: the currency registry, the shop
// manager API, the shop definition registry, and the trade-state
// reader. A nil hub installs nothing; a nil manager leaves the
// manager/currency surfaces uninstalled (content must check for
// nil, as the plugins docs say); a nil store leaves the registry
// uninstalled. The trade-state reader needs no system: trading
// state lives on the player.
func installShopHubSurfaces(hub *plugins.Hub, manager *game.ShopManager, store *definition.Store) {
	if hub == nil {
		return
	}
	if manager != nil {
		hub.SetShopCurrencyRegistry(shopCurrencyRegistryAdapter{manager: manager})
		hub.SetShopManager(shopManagerAPIAdapter{manager: manager, store: store})
	}
	if store != nil {
		hub.SetShopDefinitionRegistry(shopDefinitionRegistryAdapter{store: store})
	}
	hub.SetTradeStateReader(tradeStateReader)
}

// installShopkeepers installs the Shopkeepers plugin's NPC
// bindings (see internal/content/shopkeepers): the shops.json
// trader rows split into defaults and customs (Store
// .IsCustomShop), with the cache's NPC action labels for the
// any-hook. It runs after installShopHubSurfaces — the handlers
// resolve Core.ShopManager lazily, but the surface must exist
// by the first click — and after loadGameDefinitions, when the
// shop data and the cache facades both exist. A nil hub, store,
// or defs installs nothing (a NoCache boot has no NPC actions
// to label with). The bound count is logged for the boot
// record: 669 distinct trader keys in the shipped data (see
// PHASE41_PLAN.md).
func installShopkeepers(hub *plugins.Hub, store *definition.Store, defs *cache.CacheDefinitions, logger *log.Logger) {
	if hub == nil || store == nil || defs == nil {
		return
	}
	if logger == nil {
		logger = log.Default()
	}
	var defaults, customs []plugins.ShopDefinition
	for _, def := range store.Shops() {
		adapted := definitionShopAdapter{def: def}
		if store.IsCustomShop(def.ID()) {
			customs = append(customs, adapted)
		} else {
			defaults = append(defaults, adapted)
		}
	}
	npcActions := func(npcID int) []string {
		npc := defs.GetNpc(npcID)
		if npc == nil {
			return nil
		}
		return npc.Actions
	}
	bound := shopkeepers.InstallBindings(hub, defaults, customs, npcActions)
	logger.Printf("[shopkeepers] installed %d shopkeeper bindings (%d default shops, %d custom shops)", bound, len(defaults), len(customs))
}

// shopOpener implements game.ShopOpener over the ShopManager
// plus the definition Store: the dialogue lookup reads
// definitions (ShopDefinition.all), the open reads the runtime
// manager. See internal/game/shop_dialogue.go.
type shopOpener struct {
	manager *game.ShopManager
	store   *definition.Store
}

var _ game.ShopOpener = shopOpener{}

// FindShops returns every loaded shop definition whose name
// equals name exactly (case-sensitive), in load order. The
// runner applies the >1 core-source filter itself, so this
// method does not filter. An empty name matches nothing.
func (o shopOpener) FindShops(name string) []game.TranscriptShop {
	if o.store == nil || name == "" {
		return nil
	}
	var out []game.TranscriptShop
	for _, def := range o.store.Shops() {
		if def.Name() == name {
			out = append(out, game.TranscriptShop{ID: def.ID(), Source: def.Source()})
		}
	}
	return out
}

// OpenShop opens shopID with resetScroll true (the dialogue
// call site always passes true) and reports the open's result,
// following the ShopManager.open return contract.
func (o shopOpener) OpenShop(player *game.Player, shopID int) bool {
	if o.manager == nil || player == nil {
		return false
	}
	return o.manager.Open(player, shopID, true, game.DefaultShopTargetUID)
}

// npcInteractionSource implements listeners.NpcInteractionSource
// over the definition Store: the static-interaction action for
// an NPC id and click type, with Method/Args passed verbatim.
type npcInteractionSource struct {
	store *definition.Store
}

var _ listeners.NpcInteractionSource = npcInteractionSource{}

func (s npcInteractionSource) InteractionAction(npcID, clickType int) (listeners.NpcInteractionAction, bool) {
	if s.store == nil {
		return listeners.NpcInteractionAction{}, false
	}
	def, ok := s.store.Interaction(npcID)
	if !ok || def == nil {
		return listeners.NpcInteractionAction{}, false
	}
	action := def.Action(clickType)
	if action == nil {
		return listeners.NpcInteractionAction{}, false
	}
	return listeners.NpcInteractionAction{Method: action.Method, Args: action.Args}, true
}

// shopWidgetHandlerAdapter implements listeners.ShopWidgetHandler
// over the ShopManager, translating the listener-facing packet
// and action shapes into the manager's (field-for-field; the
// packet's HasOption has no manager counterpart — the manager
// trims the option itself — and the container-action kind enum
// maps onto the manager's kind strings, with any unknown kind
// reading as buy/sell, matching the manager's default branch).
type shopWidgetHandlerAdapter struct {
	manager *game.ShopManager
}

var _ listeners.ShopWidgetHandler = shopWidgetHandlerAdapter{}

func (a shopWidgetHandlerAdapter) HandleWidgetAction(player *game.Player, packet listeners.ShopWidgetPacket) bool {
	if a.manager == nil {
		return false
	}
	return a.manager.HandleWidgetAction(player, game.ShopWidgetAction{
		GroupID:   packet.GroupID,
		ChildID:   packet.ChildID,
		ButtonNum: packet.ButtonNum,
		Option:    packet.Option,
		Slot:      packet.Slot,
		HasSlot:   packet.HasSlot,
		ItemID:    packet.ItemID,
		HasItemID: packet.HasItemID,
	})
}

func (a shopWidgetHandlerAdapter) HandleItemContainerAction(player *game.Player, action listeners.ShopItemContainerAction) bool {
	if a.manager == nil {
		return false
	}
	kind := game.ShopActionBuySell
	switch action.Kind {
	case listeners.ShopContainerActionValue:
		kind = game.ShopActionValue
	case listeners.ShopContainerActionX:
		kind = game.ShopActionX
	}
	return a.manager.HandleItemContainerAction(player, game.ShopItemContainerAction{
		Kind:        kind,
		ContainerID: action.ContainerID,
		Slot:        action.Slot,
		ItemID:      action.ItemID,
		Amount:      action.Amount,
	})
}

// InstallShopListeners fills the shop seams on deps: the NPC
// static-interaction source + OpenShop on the NpcHooks struct,
// and the shop widget handler on Deps.Shop. A nil deps is a
// no-op; a nil store leaves Interactions unset (no static
// bindings); a nil manager leaves OpenShop and Shop unset (the
// listeners degrade to their documented fall-throughs). It must
// run AFTER InstallDialogueListeners (which replaces
// deps.NpcHooks wholesale) — the InstallCombatListeners
// ordering precedent. The coordinator calls it once per
// entering player, in AddPlayer's Deps block, after
// InstallCombatListeners.
func InstallShopListeners(deps *listeners.Deps, store *definition.Store, manager *game.ShopManager) {
	if deps == nil {
		return
	}
	if store != nil {
		deps.NpcHooks.Interactions = npcInteractionSource{store: store}
	}
	if manager != nil {
		deps.NpcHooks.OpenShop = func(player *game.Player, shopID int) bool {
			return manager.Open(player, shopID, true, game.DefaultShopTargetUID)
		}
		deps.Shop = shopWidgetHandlerAdapter{manager: manager}
	}
}
