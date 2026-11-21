package shopkeepers

import (
	"math"
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// RegisterPointCurrency ports registerPointCurrency
// (Shopkeepers.plugin.js:60-80) and is exported: other content
// (BountyHunter, Last Man Standing, and the rest) replaces the
// generic attribute-backed handler with its own, so the export is
// the seam those ports will call. The handler stores the balance
// under the persisted attribute "shop-currency:<name>" and is
// registered under each of [name, ...aliases], trimmed and
// uppercased (TS: registerShopCurrency(name, handler) per key;
// Api.RegisterShopCurrency applies the same key normalisation).
// Amount mirrors TS exactly: Number(value), finite and > 0 ?
// floor : 0. Add clamps the added value at 0 (a negative add is
// a no-op, not a subtraction); Remove clamps the balance at 0
// and does NOT clamp the removed amount, so removing more than
// the balance zeroes it — including when the removed amount is
// itself negative, which ADDS to the balance (TS faithfulness:
// `amount() - value` with value negative).
func RegisterPointCurrency(api *plugins.Api, name string, aliases ...string) {
	if api == nil {
		return
	}
	attribute := "shop-currency:" + name
	api.PersistAttribute(attribute)
	amount := func(player *game.Player) int {
		if player == nil {
			return 0
		}
		return currencyAmount(player.GetAttribute(attribute))
	}
	handler := plugins.ShopCurrencyHandler{
		Name: name,
		Amount: func(player *game.Player) int {
			return amount(player)
		},
		Add: func(player *game.Player, value int) {
			if player == nil {
				return
			}
			add := value
			if add < 0 {
				add = 0
			}
			player.SetAttribute(attribute, amount(player)+add)
		},
		Remove: func(player *game.Player, value int) {
			if player == nil {
				return
			}
			remaining := amount(player) - value
			if remaining < 0 {
				remaining = 0
			}
			player.SetAttribute(attribute, remaining)
		},
	}
	for _, key := range append([]string{name}, aliases...) {
		if trimmed := strings.TrimSpace(key); trimmed != "" {
			api.RegisterShopCurrency(strings.ToUpper(trimmed), handler)
		}
	}
}

// currencyAmount is TS Number(value) with the finite-and-positive
// floor rule (Shopkeepers.plugin.js:62-64): a stored attribute
// may round-trip through JSON as a float, so every numeric kind
// is accepted, as are decimal numeric strings (Number("5.7") is
// 5.7); anything else (nil, a non-numeric string, a bool) is 0.
// Hex strings and the like are not Number() — the port does not
// reproduce JS's full numeric-string grammar, only the decimal
// form a persisted balance could take.
func currencyAmount(value any) int {
	var n float64
	switch v := value.(type) {
	case int:
		n = float64(v)
	case int8:
		n = float64(v)
	case int16:
		n = float64(v)
	case int32:
		n = float64(v)
	case int64:
		n = float64(v)
	case uint:
		n = float64(v)
	case uint8:
		n = float64(v)
	case uint16:
		n = float64(v)
	case uint32:
		n = float64(v)
	case uint64:
		n = float64(v)
	case float32:
		n = float64(v)
	case float64:
		n = v
	case string:
		parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil {
			return 0
		}
		n = parsed
	default:
		return 0
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || n <= 0 {
		return 0
	}
	return int(math.Floor(n))
}

// openShop ports openShop (Shopkeepers.plugin.js:144-146):
// core.ShopManager.open(player, shopId, true). The ShopManager is
// resolved lazily at emit time through the Api's Core surface —
// the surface is installed after hub Load, so it must never be
// captured in Register (PORT_STATUS.md hub-contract lesson).
func openShop(event *plugins.NpcInteractionEvent, shopID int) bool {
	if pluginAPI == nil || event == nil || event.Player == nil {
		return false
	}
	core := pluginAPI.Core()
	if core == nil {
		return false
	}
	manager := core.ShopManager()
	if manager == nil {
		return false
	}
	return manager.Open(event.Player, shopID, true)
}

// openDefaultShop ports openDefaultShop
// (Shopkeepers.plugin.js:173-181): the any-hook fallback. A
// custom binding claims the click (false — the hard definition
// handles it); a shared trader asks a subscriber to pick a
// version (tradeVersion); a unique binding opens its shop;
// anything else is unhandled. Click types outside 1-5 never
// reach this handler (the any-hook wrapper filters them), but
// the guard stays so a direct call is safe.
func openDefaultShop(event *plugins.NpcInteractionEvent) bool {
	if event == nil || event.ClickType < 1 || event.ClickType > 5 {
		return false
	}
	key := TraderKey{NPCID: event.NpcID, Slot: event.ClickType}
	if _, custom := bindings.Custom[key]; custom {
		return false
	}
	if shared, ok := bindings.Shared[key]; ok {
		return tradeVersion(event, shared)
	}
	if shopID, ok := bindings.Unique[key]; ok {
		return openShop(event, shopID)
	}
	return false
}

// tradeVersion ports tradeVersion
// (Shopkeepers.plugin.js:155-171): emit "npc-dialogue:shop"
// with the shop names and let a subscriber pick one by setting
// request.Shop to a name (QuestRuntime for "(After Contact!)"
// -style names, KaramjaGloves for its gloves versions), else
// open the first version whose name ends " (Default)". With
// neither a pick nor a Default, the click is unhandled —
// exactly the shared-trader case the live leg proves on NPC
// 1943. The emit's handled flag is NOT consulted here (TS
// reads request.shop ?? the Default name, unconditionally).
func tradeVersion(event *plugins.NpcInteractionEvent, shops []ShopRef) bool {
	if pluginAPI == nil || event == nil {
		return false
	}
	names := make([]string, len(shops))
	for i, shop := range shops {
		names[i] = shop.Name
	}
	request := &game.NpcDialogueShopEvent{
		Player: event.Player,
		NPCID:  event.NpcID,
		// Target stays "" (TS target: null — the Go shape
		// carries it as a string; the dialogue runtime's
		// open-shop step treats "" as no target).
		Shops: names,
		Shop:  nil,
	}
	pluginAPI.EmitCustomEvent(plugins.NpcDialogueShopEventName, request)
	chosen := ""
	if request.Shop != nil {
		chosen = *request.Shop
	} else {
		for _, name := range names {
			if strings.HasSuffix(name, " (Default)") {
				chosen = name
				break
			}
		}
	}
	if chosen == "" {
		return false
	}
	for _, shop := range shops {
		if shop.Name == chosen {
			return openShop(event, shop.ID)
		}
	}
	return false
}

// InstallBindings computes the trader bindings from the loaded
// shop definitions and installs the NPC hooks on the hub: custom
// bindings as onNpcClick hooks (hard definitions in TS — see the
// package doc) and the default bindings as onAnyNpcInteraction
// handlers keyed by the cache's exact action labels. defaults
// are the shops.json shops, customs the custom-shops.json shops;
// npcActions answers the NPC definition's action list for an id
// (nil for unknown ids). The coordinator calls this once, after
// loadGameDefinitions + installShopHubSurfaces (the ShopManager
// surface the handlers resolve lazily must exist by then, and
// the shop data and NPC labels must be loaded). It returns the
// number of trader keys bound (custom keys + default keys); 0
// when the plugin never registered (pluginAPI is nil) or the hub
// is nil.
func InstallBindings(hub *plugins.Hub, defaults, customs []plugins.ShopDefinition, npcActions func(npcID int) []string) int {
	if hub == nil || pluginAPI == nil {
		return 0
	}
	bindings = ComputeBindings(defaults, customs)
	for key, shopID := range bindings.Custom {
		id, slot := key.NPCID, key.Slot
		hub.AddNpcClickHook("Shopkeepers", []int{id}, slot, func(event *plugins.NpcInteractionEvent) bool {
			return openShop(event, shopID)
		})
	}
	labels := LabelsFor(bindings, npcActions)
	if len(labels) > 0 {
		actions := make(map[string]plugins.NpcInteractionHandler, len(labels))
		for label := range labels {
			actions[label] = openDefaultShop
		}
		hub.AddAnyNpcInteractionHook("Shopkeepers", actions)
	}
	// Distinct trader keys with any binding: custom keys plus
	// the default keys (Unique ∪ Shared — an ambiguous key is
	// absent from Unique but present in Shared).
	keys := make(map[TraderKey]bool)
	for key := range bindings.Custom {
		keys[key] = true
	}
	for key := range bindings.Unique {
		keys[key] = true
	}
	for key := range bindings.Shared {
		keys[key] = true
	}
	return len(keys)
}
