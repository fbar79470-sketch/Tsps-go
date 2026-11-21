// Package shopkeepers is the Go port of the Shopkeepers plugin
// (server/plugins/npcs/Shopkeepers.plugin.js, 231 lines): the
// NPC-click bindings that open shops from the shop data, plus the
// item/point shop-currency registrations.
//
// The plugin has two halves with different boot timing. The
// currencies register at plugin load (Register): item ids and
// point-currency names are constants, so nothing data-dependent is
// needed. The NPC bindings attach later (InstallBindings, called by
// the coordinator after the definition Store loads — see below),
// because they are computed from shops.json trader rows and cache
// NPC action labels, neither of which exists at hub Load time
// (cmd/server/server.go: loadDefinitions → hub.Load →
// loadGameDefinitions → installShopHubSurfaces). The handlers
// close over the Api stashed at Register (mirroring TS's
// module-level `let pluginApi`) and resolve Core.ShopManager
// lazily at emit time — the surface is installed after Load, so
// it must never be captured in Register (PORT_STATUS.md
// hub-contract lesson).
//
// Bindings (Shopkeepers.plugin.js attachShopkeepers):
//   - Custom shops (custom-shops.json) bind as HARD definitions:
//     in TS, registerNpcInteraction writes them into the
//     NpcInteractionDefinition table, so NpcInteractionManager
//     .handle opens the shop before the emit. Go has no
//     registerNpcInteraction surface, so customs install as
//     onNpcClick hooks, which run FIRST inside Hub.EmitNpcInteraction
//     — the same claim order relative to every other hook. The one
//     ordering difference (a slot-1 custom binding would reach Go's
//     Talk-to-only StartDialogue before the emit; TS handle() runs
//     first) is unobservable today: custom-shops.json is empty, and
//     no default binding's label is "Talk-to".
//   - Generated shops (shops.json) bind as the any-NPC fallback:
//     onAnyNpcInteraction handlers keyed by the cache's exact action
//     label, running last in the emit — behind named owners such as
//     Castle Wars' Lanthus, exactly as in TS.
package shopkeepers

import (
	"tsps-go/internal/plugins"
)

// itemCurrency is one ITEM_CURRENCIES entry
// (Shopkeepers.plugin.js:21-40): the item id, its alias keys, and
// its unit word when one exists. Ids are TS ItemIdentifiers values;
// the line cites each.
var itemCurrencies = []struct {
	id      int
	aliases []string
	unit    string
	hasUnit bool
}{
	{id: 26792}, // ABYSSAL_PEARLS, ItemIdentifiers.ts:24745
	{id: 2996, aliases: []string{"Agility arena ticket (discontinued)"}}, // AGILITY_ARENA_TICKET, :2869
	{id: 12746, aliases: []string{"Archaic emblem"}},                     // ARCHAIC_EMBLEM_TIER_1_, :11210
	{id: 1464},  // ARCHERY_TICKET, :1398
	{id: 29482}, // BRIMHAVEN_VOUCHER, :27335
	{id: 4067},  // CASTLE_WARS_TICKET, :3666
	{id: 4278, aliases: []string{"ecto-tokens"}}, // ECTO_TOKEN, :3869
	{id: 12012}, // GOLDEN_NUGGET, :10477
	{id: 24711}, // HALLOWED_MARK, :22789
	{id: 11849}, // MARK_OF_GRACE, :10314
	{id: 21656}, // MERMAIDS_TEAR, :19960
	{id: 22820}, // MOLCH_PEARL, :21066
	{id: 8951},  // PIECES_OF_EIGHT, :7957
	{id: 25588}, // SPIRIT_FLAKES, :23608
	{id: 25527}, // STARDUST, :23551
	{id: 30038}, // TERMITES, :27859
	{id: 6529},  // TOKKUL, :5773
	{id: 6306, unit: "trade sticks", hasUnit: true}, // TRADING_STICKS, :5601
	{id: 21341}, // UNIDENTIFIED_MINERALS, :19688
}

// pointCurrencies is POINT_CURRENCIES
// (Shopkeepers.plugin.js:42-56), verbatim, in file order.
var pointCurrencies = []string{
	"Bounty Hunter Points",
	"Deadman Points",
	"event points",
	"Honour points",
	"Last Man Standing Points",
	"League Points",
	"Mahogany Homes Points",
	"NMZ",
	"PvP Arena Points",
	"Speedrun Points",
	"Tithe",
	"Volcanic Mine points",
	"Zeal Tokens",
}

// pluginAPI is the Api this plugin registered with, stashed for
// InstallBindings (mirroring TS's module-level `let pluginApi` in
// Shopkeepers.plugin.js). The bindings install after hub Load,
// when Register's api is the only handle on the hub's emit and
// core surfaces; a nil api (plugin never registered, e.g.
// disabled by world.json switches) makes InstallBindings a no-op.
var pluginAPI *plugins.Api

// bindings holds the installed trader bindings (see
// InstallBindings). Handlers read it at emit time; it is set
// before any hook is installed, so a registered handler always
// sees the installed tables.
var bindings Bindings

// Plugin is the registered Shopkeepers plugin value: name and
// description from the plugin file (Shopkeepers.plugin.js).
var Plugin = plugins.Plugin{
	Name:        "Shopkeepers",
	Description: "Opens shops from NPC Trade options using the shop data, with default and custom NPC-to-shop bindings.",
	Register: func(api *plugins.Api) {
		pluginAPI = api
		for _, currency := range itemCurrencies {
			api.RegisterItemShopCurrency(currency.id, plugins.ShopItemCurrencyOptions{
				Aliases: currency.aliases,
				Unit:    currency.unit,
				HasUnit: currency.hasUnit,
			})
		}
		for _, name := range pointCurrencies {
			RegisterPointCurrency(api, name)
		}
		// The NPC bindings attach later, via InstallBindings:
		// they are computed from the loaded shop definitions
		// and cache NPC actions, which do not exist at hub
		// Load time (see the package doc).
	},
}

func init() {
	plugins.Register(Plugin)
}
