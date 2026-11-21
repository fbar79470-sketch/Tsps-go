package shopkeepers

import (
	"tsps-go/internal/plugins"
)

// TraderKey is one "npcId:slot" binding key
// (Shopkeepers.plugin.js traderKeys): the NPC id and the option
// slot (1-5) whose click opens a shop.
type TraderKey struct {
	NPCID int
	Slot  int
}

// ShopRef is one shop in a shared-trader list: the id and name
// (Shopkeepers.plugin.js sharedTraders keeps the shop objects;
// the port keeps only what tradeVersion reads).
type ShopRef struct {
	ID   int
	Name string
}

// Bindings is the computed trader-binding state
// (Shopkeepers.plugin.js defaultShopkeepers +
// customShopkeepers + shopkeepers + sharedTraders, with the
// uniqueBindings filter applied):
//   - Custom: the custom-shops.json bindings (hard definitions in
//     TS). A key present here wins over any default binding.
//   - Unique: the shops.json bindings with an unambiguous shop
//     (ambiguous keys — the same key claimed by two different
//     shops — are absent, as TS keeps them as null and the
//     uniqueBindings filter drops nulls).
//   - Shared: the shops.json keys bound to more than one shop, in
//     first-seen order with duplicates kept per occurrence, exactly
//     as sharedTraders builds the list (a key listed twice for the
//     same shop still counts as shared).
type Bindings struct {
	Custom map[TraderKey]int
	Unique map[TraderKey]int
	Shared map[TraderKey][]ShopRef
}

// ComputeBindings ports defaultShopkeepers, customShopkeepers,
// shopkeepers, and sharedTraders (Shopkeepers.plugin.js:99-143)
// over the loaded shop definitions. defaults are the shops.json
// shops, customs the custom-shops.json shops (split by the
// coordinator on Store.IsCustomShop). Shops are read in load
// order; a later custom shop overwrites an earlier custom binding
// for the same key (TS Map.set), and a custom binding overrides
// any default binding at emit time (openDefaultShop checks the
// custom map first).
func ComputeBindings(defaults, customs []plugins.ShopDefinition) Bindings {
	out := Bindings{
		Custom: make(map[TraderKey]int),
		Unique: make(map[TraderKey]int),
		Shared: make(map[TraderKey][]ShopRef),
	}
	// defaultShopkeepers: first claim wins per key; a second,
	// different shop makes the key ambiguous (null). The same
	// shop claiming a key twice keeps it.
	seen := make(map[TraderKey]int)
	ambiguous := make(map[TraderKey]bool)
	sharedLists := make(map[TraderKey][]ShopRef)
	for _, shop := range defaults {
		if shop == nil {
			continue
		}
		for _, trader := range shop.NpcInteractions() {
			for _, npcID := range trader.NPCIDs {
				key := TraderKey{NPCID: npcID, Slot: trader.OptionSlot}
				sharedLists[key] = append(sharedLists[key], ShopRef{ID: shop.ID(), Name: shop.Name()})
				if prev, ok := seen[key]; ok {
					if prev != shop.ID() {
						ambiguous[key] = true
					}
					continue
				}
				seen[key] = shop.ID()
			}
		}
	}
	for key, shopID := range seen {
		if !ambiguous[key] {
			out.Unique[key] = shopID
		}
	}
	// sharedTraders: every key whose list holds more than one
	// entry (duplicates per occurrence, as TS appends per key
	// appearance).
	for key, list := range sharedLists {
		if len(list) > 1 {
			out.Shared[key] = list
		}
	}
	// customShopkeepers: customs overwrite, in load order.
	for _, shop := range customs {
		if shop == nil {
			continue
		}
		for _, trader := range shop.NpcInteractions() {
			for _, npcID := range trader.NPCIDs {
				out.Custom[TraderKey{NPCID: npcID, Slot: trader.OptionSlot}] = shop.ID()
			}
		}
	}
	return out
}

// LabelsFor ports the actions-map build in attachShopkeepers
// (Shopkeepers.plugin.js:198-205): the distinct cache action
// labels of the default keys that have NO custom binding (a key
// with a custom binding contributes no label — its click never
// reaches the any-hook). npcActions answers the NPC definition's
// action list for an id (nil or short lists yield no label for
// the slot, as actionLabel returns null). The returned set is the
// any-hook's action map keys. The default keys are the union of
// Unique and Shared: a key claimed once lives in Unique, a key
// claimed by two different shops is ambiguous (absent from
// Unique) but present in Shared.
func LabelsFor(b Bindings, npcActions func(npcID int) []string) map[string]bool {
	labels := make(map[string]bool)
	if npcActions == nil {
		return labels
	}
	keys := make(map[TraderKey]bool)
	for key := range b.Unique {
		keys[key] = true
	}
	for key := range b.Shared {
		keys[key] = true
	}
	for key := range keys {
		if _, custom := b.Custom[key]; custom {
			continue
		}
		actions := npcActions(key.NPCID)
		if key.Slot < 1 || key.Slot > len(actions) {
			continue
		}
		if label := actions[key.Slot-1]; label != "" {
			labels[label] = true
		}
	}
	return labels
}
