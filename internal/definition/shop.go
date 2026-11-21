package definition

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// DefaultStockChangeTicks is DEFAULT_STOCK_CHANGE_TICKS
// (ShopDefinitionLoader.ts:49): the restock/destock interval when
// a shop configures none.
const DefaultStockChangeTicks = 4

// GeneralStoreSoldItemDestockTicks is
// GENERAL_STORE_SOLD_ITEM_DESTOCK_TICKS
// (ShopDefinitionLoader.ts:50).
const GeneralStoreSoldItemDestockTicks = 100

// CanStockItemHook is the plugin stock veto (emitCanStockItem,
// PluginManager.ts:1682, fired per shop stock entry at load by
// ShopDefinitionLoader.ts:183): when non-nil it is consulted for
// every stock entry of every shop definition as it loads, and an
// entry the hook refuses (false) is dropped from the shop — e.g.
// members items on a free-to-play world. A nil hook keeps every
// entry (TS with no onCanStockItem hooks: emitCanStockItem
// returns null, which is !== false).
//
// The hook is a package variable because the veto fires inside
// Store loading, before any ShopManager exists, and the hub's
// first-answer wiring can only reach the loader this way. Set it
// once, before Load, from the coordinator's hub wiring (the
// hub's answer, with "no opinion" mapped to true); it is read
// only during Load, never after. Tests that set it must restore
// the previous value.
var CanStockItemHook func(shopID, itemID int) bool

// ShopSalesPolicy is the Wiki StoreTableHead pricing policy
// (ShopSalesPolicy, ShopDefinition.ts:8-18). Each field is nil
// when the shop configures no value (TS undefined). Rates are per
// mille of the item's value.
type ShopSalesPolicy struct {
	GeneralStore *bool
	BuysNothing  *bool
	SellsAt      *int
	BuysAt       *int
	ChangePer    *int
}

// ShopStockDefinition is one original-stock row
// (ShopStockDefinition, ShopDefinition.ts:20-25). RestockTicks
// and Price are nil when the row configures none (TS null).
type ShopStockDefinition struct {
	ID           int
	Amount       int
	RestockTicks *int
	Price        *int
}

// ShopDefinition is one shop (ShopDefinition.ts:27-101).
type ShopDefinition struct {
	id                   int
	name                 string
	currency             string
	originalStock        []ShopStockDefinition
	defaultRestockTicks  int
	defaultDestockTicks  int
	soldItemDestockTicks int
	source               string
	salesPolicy          ShopSalesPolicy
}

// ID returns the shop id.
func (d *ShopDefinition) ID() int { return d.id }

// Name returns the shop name ("Shop" when unconfigured).
func (d *ShopDefinition) Name() string { return d.name }

// Currency returns the shop currency key ("COINS" by default).
func (d *ShopDefinition) Currency() string { return d.currency }

// OriginalStock returns the shop's original stock rows.
func (d *ShopDefinition) OriginalStock() []ShopStockDefinition {
	return append([]ShopStockDefinition(nil), d.originalStock...)
}

// DefaultRestockTicks returns the shop-wide restock interval.
func (d *ShopDefinition) DefaultRestockTicks() int { return d.defaultRestockTicks }

// DefaultDestockTicks returns the shop-wide destock interval.
func (d *ShopDefinition) DefaultDestockTicks() int { return d.defaultDestockTicks }

// SoldItemDestockTicks returns the destock interval for items
// players sold to the shop.
func (d *ShopDefinition) SoldItemDestockTicks() int { return d.soldItemDestockTicks }

// Source returns the contributing source's name ("core" for the
// core files).
func (d *ShopDefinition) Source() string { return d.source }

// SalesPolicy returns the shop's sales policy.
func (d *ShopDefinition) SalesPolicy() ShopSalesPolicy { return d.salesPolicy }

// Shop returns the shop definition for an id, or false
// (ShopDefinition.forId, ShopDefinition.ts:58-60).
func (s *Store) Shop(id int) (*ShopDefinition, bool) {
	def, ok := s.shops[id]
	return def, ok
}

// Shops returns all loaded shops in first-insertion order: a
// later source's shop with an already-seen id replaces the value
// but keeps the original position, as a JS Map.set does.
func (s *Store) Shops() []*ShopDefinition {
	return append([]*ShopDefinition(nil), s.shopList...)
}

// loadShops ports ShopDefinitionLoader.load()
// (ShopDefinitionLoader.ts:54-97): the core source ("core",
// priority 0) concatenates shops.json and custom-shops.json —
// BOTH files are REQUIRED — then contributed sources are merged
// in (priority, name) order, later definitions replacing earlier
// ones by id. Invalid entries are skipped. The plugin stock veto
// (emitCanStockItem, ShopDefinitionLoader.ts:183) runs through
// CanStockItemHook when one is installed; with none installed no
// stock is dropped.
func (s *Store) loadShops() error {
	contributed, failures := s.registry.loadSources(shopsDefinitionType, s.logf)
	core, err := s.readCoreShops()
	if err != nil {
		return err
	}
	sources := append([]LoadedSource{{
		Name:        "core",
		Owner:       "core",
		Priority:    0,
		Definitions: core,
	}}, contributed...)
	sortLoadedSources(sources)

	unresolved := make(map[string]bool)
	candidates, invalid := 0, 0
	for _, source := range sources {
		candidates += len(source.Definitions)
		for _, raw := range source.Definitions {
			def, ok := s.toShop(raw, source.Name, unresolved)
			if !ok {
				invalid++
				continue
			}
			if _, exists := s.shops[def.id]; !exists {
				s.shopList = append(s.shopList, def)
			} else {
				// Replace in place, keeping first-insertion
				// position (JS Map.set semantics).
				for i, existing := range s.shopList {
					if existing.id == def.id {
						s.shopList[i] = def
						break
					}
				}
			}
			s.shops[def.id] = def
		}
	}
	s.logf("[shops] Loaded %d definitions from %s (candidates=%d, invalid=%d, unresolvedStock=%d)",
		len(s.shopList), joinSourceNames(sources), candidates, invalid, len(unresolved))
	_ = failures // TS returns contributed.failures == 0; GameBuilder ignores it.
	return nil
}

// readCoreShops is readCoreDefinitions
// (ShopDefinitionLoader.ts:103-114): shops.json followed by
// custom-shops.json, each required to exist and hold an array.
// (custom-shops.json is shop data the TS loader reads as a core
// file; it is not a cache overlay, so reading it here does not
// cross into the overlay work's custom-definitions territory.)
func (s *Store) readCoreShops() ([]any, error) {
	files := []string{
		filepath.Join(s.dir, "shops.json"),
		filepath.Join(s.dir, "custom-shops.json"),
	}
	var definitions []any
	for _, file := range files {
		parsed, err := readJSONValue(file)
		if err != nil {
			return nil, err
		}
		rows, ok := parsed.([]any)
		if !ok {
			return nil, fmt.Errorf("%s must contain an array", file)
		}
		definitions = append(definitions, rows...)
	}
	return definitions, nil
}

// toShop is toDefinition (ShopDefinitionLoader.ts:116-213).
func (s *Store) toShop(raw any, source string, unresolved map[string]bool) (*ShopDefinition, bool) {
	idNum := numField(raw, "id")
	if !isJSInteger(idNum) || jsTrunc(idNum) < 0 {
		return nil, false
	}
	id := jsTrunc(idNum)

	defaultRestockTicks := firstTicks(DefaultStockChangeTicks,
		parseShopTicks(mustNum(raw, "defaultRestockTicks")),
		parseShopTicks(mustNum(raw, "restockTicks")))
	defaultDestockTicks := firstTicks(defaultRestockTicks,
		parseShopTicks(mustNum(raw, "defaultDestockTicks")))
	soldItemDestockTicks := firstTicks(GeneralStoreSoldItemDestockTicks,
		parseShopTicks(mustNum(raw, "soldItemDestockTicks")))

	requiresPresets := boolField(raw, "requiresPresets")
	usePresetPrices := requiresPresets && s.presetShopPrices
	var prices map[int]int
	if requiresPresets && !usePresetPrices {
		prices = make(map[int]int)
	} else {
		prices = s.shopItemPrices(mustNum(raw, "prices"))
	}
	// The host editor's default stack size fills originalStock
	// rows that omit an amount; imported shops leave stockAmount
	// out, so their rows default to one (line 136-138).
	stockAmount := normalizeShopAmount(mustNumOr(raw, "stockAmount", 1))
	var stock []ShopStockDefinition
	if rows, ok := mustNum(raw, "originalStock").([]any); ok {
		for _, entry := range rows {
			itemIDNum := numField(entry, "id")
			rawAmount := mustNum(entry, "amount")
			if _, present := field(entry, "amount"); !present || rawAmount == nil {
				rawAmount = float64(stockAmount)
			}
			amount := normalizeShopAmount(rawAmount)
			// A listed stock of 0 is real: specialist shops
			// show and buy items they start with none of.
			// The resolved amount counts, so a shop-wide
			// stockAmount of 0 keeps rows that omit their
			// own amount (lines 145-148).
			listedEmpty := jsNumber(rawAmount, true) == 0
			if !isJSInteger(itemIDNum) || jsTrunc(itemIDNum) <= 0 || (amount <= 0 && !listedEmpty) {
				continue
			}
			itemID := jsTrunc(itemIDNum)
			row := ShopStockDefinition{ID: itemID, Amount: amount}
			if ticks, ok := firstTicksOk(
				parseShopTicks(mustNum(entry, "restockTicks")),
				parseShopTicks(mustNum(entry, "restockTime")),
				parseShopTicks(mustNum(entry, "restock"))); ok {
				row.RestockTicks = &ticks
			}
			if priceValue, ok := shopRowPrice(prices, itemID, entry); ok {
				row.Price = &priceValue
			}
			stock = append(stock, row)
		}
	}
	for _, item := range shopItems(mustNum(raw, "items")) {
		var itemID int
		var name string
		isName := false
		switch v := item.(type) {
		case int:
			itemID = v
		case string:
			isName = true
			name = v
			resolved, ok := s.shopItemIDForName(v)
			if !ok {
				itemID = -1
			} else {
				itemID = resolved
			}
		}
		if itemID < 0 || stockAmount <= 0 {
			if isName {
				unresolved[name] = true
			}
			continue
		}
		row := ShopStockDefinition{ID: itemID, Amount: stockAmount}
		if price, ok := prices[itemID]; ok {
			row.Price = &price
		}
		stock = append(stock, row)
	}

	// Plugins can drop stock entries (e.g. members items on a
	// free-to-play world): ShopDefinitionLoader.ts:183 filters
	// the combined stock (originalStock rows plus items rows)
	// through emitCanStockItem. CanStockItemHook is that emit;
	// a nil hook keeps everything.
	if CanStockItemHook != nil {
		kept := stock[:0]
		for _, entry := range stock {
			if CanStockItemHook(id, entry.ID) {
				kept = append(kept, entry)
			}
		}
		stock = kept
	}

	name := "Shop"
	if text, ok := mustNum(raw, "name").(string); ok && strings.TrimSpace(text) != "" {
		name = strings.TrimSpace(text)
	}
	configuredCurrency := "COINS"
	if text, ok := mustNum(raw, "currency").(string); ok && strings.TrimSpace(text) != "" {
		configuredCurrency = strings.Join(strings.Fields(strings.ToUpper(strings.TrimSpace(text))), " ")
	}
	// Blood money is the one currency key with an underscore;
	// accept the Wiki's spaced name too (line 191-192).
	currencyKey := configuredCurrency
	if currencyKey == "BLOOD MONEY" {
		currencyKey = "BLOOD_MONEY"
	}
	currency := currencyKey
	if requiresPresets && !usePresetPrices {
		currency = "COINS"
	}
	var generalStore *bool
	if v, present := field(raw, "generalStore"); present {
		if b, ok := v.(bool); ok {
			generalStore = &b
		}
	}
	var buysNothing *bool
	if boolField(raw, "buysNothing") {
		b := true
		buysNothing = &b
	}
	return &ShopDefinition{
		id:                   id,
		name:                 name,
		currency:             currency,
		originalStock:        stock,
		defaultRestockTicks:  defaultRestockTicks,
		defaultDestockTicks:  defaultDestockTicks,
		soldItemDestockTicks: soldItemDestockTicks,
		source:               source,
		salesPolicy: ShopSalesPolicy{
			GeneralStore: generalStore,
			BuysNothing:  buysNothing,
			SellsAt:      perMille(mustNum(raw, "sellsAt")),
			BuysAt:       perMille(mustNum(raw, "buysAt")),
			ChangePer:    perMille(mustNum(raw, "changePer")),
		},
	}, true
}

// shopRowPrice computes one stock row's price: the prices map
// wins, else the row's own price; only a finite price > 0 counts,
// floored (ShopDefinitionLoader.ts:152,160-162).
func shopRowPrice(prices map[int]int, itemID int, entry any) (int, bool) {
	if price, ok := prices[itemID]; ok {
		return price, true
	}
	p := numField(entry, "price")
	if isFiniteNumber(p) && p > 0 {
		return jsTrunc(math.Floor(p)), true
	}
	return 0, false
}

// perMille is perMille (ShopDefinitionLoader.ts:216-218): a
// StoreTableHead rate, or nil when the shop has none.
func perMille(value any) *int {
	n, ok := numberValue(value)
	if !ok || !isJSSafeInteger(n) || n < 0 {
		return nil
	}
	v := jsTrunc(n)
	return &v
}

// normalizeShopAmount is normalizeAmount
// (ShopDefinitionLoader.ts:220-223): Number(value), finite floored
// and clamped to >= 0, else 0.
func normalizeShopAmount(value any) int {
	n := jsNumber(value, true)
	if !isFiniteNumber(n) {
		return 0
	}
	amount := jsTrunc(math.Floor(n))
	if amount < 0 {
		return 0
	}
	return amount
}

// mustNumOr returns a field's value, or the fallback when absent.
// A present JSON null is NOT replaced (TS ?? replaces null too —
// stockAmount uses ??, so null also falls back; this helper is
// used only for that case).
func mustNumOr(obj any, key string, fallback any) any {
	v, present := field(obj, key)
	if !present || v == nil {
		return fallback
	}
	return v
}

// shopItems is items (ShopDefinitionLoader.ts:225-235): numbers
// (integer, > 0) pass through, strings split on commas; anything
// else is dropped. A non-array value is treated as a single item.
func shopItems(value any) []any {
	var values []any
	if arr, ok := value.([]any); ok {
		values = arr
	} else {
		values = []any{value}
	}
	var items []any
	for _, item := range values {
		switch v := item.(type) {
		case float64:
			if isJSInteger(v) && v > 0 {
				items = append(items, jsTrunc(v))
			}
		case string:
			for _, name := range strings.Split(v, ",") {
				if trimmed := strings.TrimSpace(name); trimmed != "" {
					items = append(items, trimmed)
				}
			}
		}
	}
	return items
}

// shopItemPrices is itemPrices (ShopDefinitionLoader.ts:237-252):
// a map from item id (numeric keys, or names resolved through the
// cache) to floored positive prices.
func (s *Store) shopItemPrices(value any) map[int]int {
	prices := make(map[int]int)
	obj, ok := value.(map[string]any)
	if !ok {
		return prices
	}
	for key, rawPrice := range obj {
		var itemID int
		if n, err := parseShopPriceKey(key); err == nil {
			itemID = n
		} else if resolved, ok := s.shopItemIDForName(key); ok {
			itemID = resolved
		} else {
			continue
		}
		price := jsNumber(rawPrice, true)
		if itemID > 0 && isFiniteNumber(price) && price > 0 {
			prices[itemID] = jsTrunc(math.Floor(price))
		}
	}
	return prices
}

// parseShopPriceKey mirrors Number.isInteger(Number(item)) for a
// prices-map key: the whole key must parse as an integer number.
func parseShopPriceKey(key string) (int, error) {
	n := jsParseNumber(key)
	if !isJSInteger(n) {
		return 0, fmt.Errorf("not an integer key")
	}
	return jsTrunc(n), nil
}

// shopItemIDForName is itemIdForName
// (ShopDefinitionLoader.ts:254-272): the first cache item id whose
// normalized name matches, built lazily. Custom items (the
// getCustomItems scan, lines 264-269) are not consulted: custom
// content is the overlay work's, and no custom items exist here.
// With no cache the map is empty and every name is unresolved.
func (s *Store) shopItemIDForName(name string) (int, bool) {
	if !s.itemIDsByNameDone {
		s.itemIDsByName = make(map[string]int)
		if s.cache != nil {
			count := s.cache.GetCounts().Items
			for id := 0; id < count; id++ {
				itemName := s.cache.GetItem(id).Name
				if itemName == "" {
					continue
				}
				normalized := normalizeShopItemName(itemName)
				if _, exists := s.itemIDsByName[normalized]; !exists {
					s.itemIDsByName[normalized] = id
				}
			}
		}
		s.itemIDsByNameDone = true
	}
	id, ok := s.itemIDsByName[normalizeShopItemName(name)]
	return id, ok
}

var shopItemNamePlural = regexp.MustCompile(`\b([a-z]+)s\b`)
var shopItemNameStrip = regexp.MustCompile(`[^a-z0-9+]+`)

// normalizeShopItemName is normalizeItemName
// (ShopDefinitionLoader.ts:274-280).
func normalizeShopItemName(name string) string {
	lower := strings.ToLower(name)
	lower = strings.ReplaceAll(lower, "’", "")
	lower = strings.ReplaceAll(lower, "'", "")
	lower = shopItemNamePlural.ReplaceAllString(lower, "$1")
	return shopItemNameStrip.ReplaceAllString(lower, "")
}

// firstTicks returns the first non-nil tick value, or def when
// all are nil (the ?? chains in toDefinition).
func firstTicks(def int, values ...*int) int {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return def
}

// firstTicksOk returns the first non-nil tick value.
func firstTicksOk(values ...*int) (int, bool) {
	for _, v := range values {
		if v != nil {
			return *v, true
		}
	}
	return 0, false
}

var (
	// The TS patterns carry the /i flag; (?i) matches it.
	shopTickPattern   = regexp.MustCompile(`(?i)(\d+)\s*t\b`)
	shopMinutePattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*m(?:in(?:ute)?s?)?\b`)
	shopSecondPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*s(?:ec(?:ond)?s?)?\b`)
)

// parseShopTicks is parseTicks (ShopDefinitionLoader.ts:282-311):
// a finite number floors to ticks (> 0, else nil); a string is
// searched for a tick count, then minutes, then seconds (a tick
// is 0.6 s). Anything else yields nil. The TS regexes are
// unanchored .match calls, so the Go patterns search too.
func parseShopTicks(value any) *int {
	if n, ok := numberValue(value); ok {
		ticks := jsTrunc(math.Floor(n))
		if ticks > 0 {
			return &ticks
		}
		return nil
	}
	text, ok := value.(string)
	if !ok || strings.TrimSpace(text) == "" {
		return nil
	}
	trimmed := strings.TrimSpace(text)
	if m := shopTickPattern.FindStringSubmatch(trimmed); m != nil {
		ticks, err := parseShopInt(m[1])
		if err == nil && ticks > 0 {
			return &ticks
		}
		return nil
	}
	if m := shopMinutePattern.FindStringSubmatch(trimmed); m != nil {
		minutes, err := strconv.ParseFloat(m[1], 64)
		if err == nil && isFiniteNumber(minutes) && minutes > 0 {
			ticks := int(math.Max(1, math.Round(minutes*60/0.6)))
			return &ticks
		}
		return nil
	}
	if m := shopSecondPattern.FindStringSubmatch(trimmed); m != nil {
		seconds, err := strconv.ParseFloat(m[1], 64)
		if err == nil && isFiniteNumber(seconds) && seconds > 0 {
			ticks := int(math.Max(1, math.Round(seconds/0.6)))
			return &ticks
		}
		return nil
	}
	return nil
}

// parseShopInt parses the tick-count capture. TS uses
// Number.parseInt, which stops at the first non-digit; the regex
// capture is digits-only, so a plain parse matches.
func parseShopInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not digits")
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
