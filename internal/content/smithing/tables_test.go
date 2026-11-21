package smithing

import (
	"strings"
	"testing"

	"tsps-go/internal/cache"
)

// ---- load-proven tables (real rev-242 cache) ----

const seedCachePath = "/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03"

// loadRealTables loads the tables against the real rev-242
// cache, skipping when the seed cache is absent.
func loadRealTables(t *testing.T) *Tables {
	t.Helper()
	store, err := cache.OpenStore(seedCachePath)
	if err != nil {
		t.Skipf("seed cache absent (%v)", err)
	}
	defer store.Close()
	defs, err := cache.NewCacheDefinitionsFromStore(cache.CacheInfo{
		Name: "osrs-242_2026-11-03", Game: cache.GameOldschool,
		Environment: "live", Revision: 242, Timestamp: "2026-11-03",
	}, store)
	if err != nil {
		t.Fatalf("NewCacheDefinitionsFromStore: %v", err)
	}
	// Tests run from the package directory, so the
	// repo-relative default path does not resolve here;
	// LoadFile takes the same files by their package-relative
	// path (the layout file sits beside it, as LoadFile
	// expects). The real-cache assertions double as the
	// display-name verification for the itemIDFallbacks ids.
	tables, err := LoadFile("../../../data/definitions/smithing.json", cacheResolver{defs: defs})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	// Fallback display names, verified against the real cache.
	wantNames := map[int]string{
		2:    "Steel cannonball",
		9376: "Blurite bolts (unf)",
		9422: "Blurite limbs",
		1783: "Bucket of sand",
		1925: "Bucket",
	}
	for id, want := range wantNames {
		item := defs.GetItem(id)
		if item == nil {
			t.Errorf("fallback item %d: missing from cache", id)
			continue
		}
		if item.Name != want {
			t.Errorf("fallback item %d: name %q, want %q", id, item.Name, want)
		}
	}
	return tables
}

func TestLoadRealFile(t *testing.T) {
	tables := loadRealTables(t)
	if got := len(tables.Smelting); got != 14 {
		t.Errorf("Smelting = %d, want 14", got)
	}
	if got := len(tables.Cannonballs); got != 6 {
		t.Errorf("Cannonballs = %d, want 6", got)
	}
	if got := len(tables.Blurite); got != 2 {
		t.Errorf("Blurite = %d, want 2", got)
	}
	if got := len(tables.Spears); got != 6 {
		t.Errorf("Spears = %d, want 6", got)
	}
	if got := len(tables.Anvil); got != 188 {
		t.Errorf("Anvil = %d, want 188", got)
	}

	// Smelting spot checks (float XP stays float in the tables).
	bronze := tables.SmeltByBar(2349)
	if bronze == nil {
		t.Fatal("SmeltByBar(2349 bronze) = nil")
	}
	if bronze.XP != 6.2 || bronze.Level != 1 || bronze.Name != "Bronze bar" {
		t.Errorf("bronze = %+v, want XP 6.2 level 1", bronze)
	}
	if len(bronze.Ingredients) != 2 || bronze.Ingredients[0].ItemID != 436 || bronze.Ingredients[1].ItemID != 438 {
		t.Errorf("bronze ingredients = %v, want copper 436 + tin 438", bronze.Ingredients)
	}
	iron := tables.SmeltByBar(2351)
	if iron == nil || !iron.HasSuccessChance || iron.SuccessChance != 0.5 {
		t.Errorf("iron success chance = %+v, want 0.5", iron)
	}
	if iron == nil || iron.FailMessage != "The ore is too impure and you fail to refine it." {
		t.Errorf("iron fail message = %q", iron.FailMessage)
	}
	gold := tables.SmeltByBar(2357)
	if gold == nil || gold.XP != 22.5 {
		t.Errorf("gold XP = %v, want 22.5", gold)
	}
	// Molten glass: Crafting recipe with timing and a bucket
	// byproduct (BUCKET is a fallback id, 1925).
	glass := tables.SmeltByBar(1775)
	if glass == nil {
		t.Fatal("SmeltByBar(1775 molten glass) = nil")
	}
	if glass.Skill != "CRAFTING" || !glass.HasTiming || glass.Interval != 2 || glass.FirstDelay != 2 || glass.FinishDelay != 0 {
		t.Errorf("molten glass = %+v, want CRAFTING timing [2,2,0]", glass)
	}
	if len(glass.Byproducts) != 1 || glass.Byproducts[0].ItemID != 1925 || glass.Byproducts[0].Amount != 1 {
		t.Errorf("molten glass byproducts = %v, want bucket 1925 x1", glass.Byproducts)
	}
	if glass.SmeltMessage != "" {
		t.Errorf("molten glass smelt message = %q, want empty (JSON null)", glass.SmeltMessage)
	}
	// XP per bar.
	if tables.XPPerBar[2363] != 75 || tables.XPPerBar[2349] != 12.5 || tables.XPPerBar[13354] != 60 {
		t.Errorf("XPPerBar = %v, want runite 75, bronze 12.5, lovakite 60", tables.XPPerBar)
	}
	// Blurite smelt recipe carries the quest gate.
	bluriteBar := tables.SmeltByBar(9467)
	if bluriteBar == nil || bluriteBar.Quest != "the_knights_sword" {
		t.Errorf("blurite bar quest = %q, want the_knights_sword", bluriteBar.Quest)
	}

	// Steel cannonball: TS has the ItemIdentifiers constant
	// (STEEL_CANNONBALL = 2), so the entry is KEPT with
	// BallID 2 — the plan's drop branch does not apply.
	steel := tables.Cannonballs[2]
	if steel.BallID != 2 || steel.BarID != 2353 || steel.Level != 35 || steel.XPPerBar != 25.6 {
		t.Errorf("steel cannonball = %+v, want ball 2 bar 2353 level 35 xp 25.6", steel)
	}
	if tables.Cannonballs[0].BallID != 31906 || tables.Cannonballs[5].BallID != 31914 {
		t.Errorf("cannonball balls = %d..%d, want 31906..31914", tables.Cannonballs[0].BallID, tables.Cannonballs[5].BallID)
	}

	// Blurite products resolve through the fallback table.
	if tables.Blurite[0].ItemID != 9376 || tables.Blurite[0].Amount != 10 || tables.Blurite[0].XP != 17.5 {
		t.Errorf("blurite bolts = %+v, want item 9376 amount 10 xp 17.5", tables.Blurite[0])
	}
	if tables.Blurite[1].ItemID != 9422 || tables.Blurite[1].Amount != 1 {
		t.Errorf("blurite limbs = %+v, want item 9422 amount 1", tables.Blurite[1])
	}

	// Spears: bar + log + spear/hasta ids.
	bronzeSpear := tables.Spears[0]
	if bronzeSpear.BarID != 2349 || bronzeSpear.LogID != 1511 || bronzeSpear.SpearID != 1237 || bronzeSpear.HastaID != 11367 {
		t.Errorf("bronze spear = %+v, want bar 2349 log 1511 spear 1237 hasta 11367", bronzeSpear)
	}
	runeSpear := tables.Spears[5]
	if runeSpear.Level != 90 || runeSpear.XP != 150 || runeSpear.SpearID != 1247 {
		t.Errorf("rune spear = %+v, want level 90 xp 150 spear 1247", runeSpear)
	}

	// Anvil: enum 846 stores 0 for exactly 1205 (Bronze
	// dagger) and 1351 (Bronze axe) — 0 means level 1.
	dagger := tables.AnvilByComponent(1, 9)
	if dagger == nil || dagger.ItemID != 1205 || dagger.Level != 1 || dagger.Bars != 1 || dagger.Amount != 1 || dagger.XP != 12.5 {
		t.Errorf("bronze dagger = %+v, want item 1205 level 1 bars 1 amount 1 xp 12.5", dagger)
	}
	axe := tables.AnvilByComponent(1, 14)
	if axe == nil || axe.ItemID != 1351 || axe.Level != 1 {
		t.Errorf("bronze axe = %+v, want item 1351 level 1", axe)
	}
	// Enum 1101 IS present in rev-242 (30 entries, component
	// 9..38 -> slot index 1..30): every product's slot index
	// is its component minus 8, for every bar.
	for _, p := range tables.Anvil {
		if p.SlotIndex != p.Component-8 {
			t.Errorf("anvil item %d: slotIndex %d, want component-8 = %d", p.ItemID, p.SlotIndex, p.Component-8)
		}
	}
	// Enum 844/845 spot checks: nails 15, platebody 5 bars.
	nails := tables.AnvilByComponent(1, 23)
	if nails == nil || nails.Amount != 15 {
		t.Errorf("bronze nails = %+v, want amount 15", nails)
	}
	platebody := tables.AnvilByComponent(1, 22)
	if platebody == nil || platebody.Bars != 5 || platebody.XP != 62.5 || platebody.Level != 18 {
		t.Errorf("bronze platebody = %+v, want bars 5 xp 62.5 level 18", platebody)
	}
	// Phrases: label phrase, and the objPhrases win is
	// covered by the fixture test; here the metal is the
	// bar's gameval without "_bar".
	if dagger.Metal != "bronze" || dagger.Phrase != "a dagger" {
		t.Errorf("bronze dagger metal/phrase = %q/%q, want bronze/a dagger", dagger.Metal, dagger.Phrase)
	}
	// Members + quest flags ride the layout slots.
	bolts := tables.AnvilByComponent(1, 34)
	if bolts == nil || !bolts.Members || bolts.ItemID != 9375 {
		t.Errorf("bronze bolts = %+v, want item 9375 members", bolts)
	}
	claws := tables.AnvilByComponent(1, 18)
	if claws == nil || claws.Quest != "death_plateau" {
		t.Errorf("bronze claws = %+v, want quest death_plateau", claws)
	}
	// Per-bar counts match the layout (27/28/29/27/26/26/25).
	wantBars := map[int]int{2349: 27, 2351: 28, 2353: 29, 2359: 27, 2361: 26, 2363: 26, 13354: 25}
	for bar, want := range wantBars {
		if got := len(tables.AnvilByBar(bar)); got != want {
			t.Errorf("AnvilByBar(%d) = %d, want %d", bar, got, want)
		}
	}
}

// ---- fixture tests (no live cache) ----

// fixtureResolver is a hand-built IDResolver: every gameval
// name the fixture JSON uses maps to a deterministic fake id.
// EnumValues answers the enums the fixture sets; an enum the
// fixture leaves unset answers empty (as GetEnumValues does
// for a missing enum).
type fixtureResolver struct {
	obj      map[string]int
	enums    map[int]map[int]int
	objNames map[int]string
	ifaceID  int
	ifaceOK  bool
}

func (f fixtureResolver) ObjID(name string) (int, bool, error) {
	id, ok := f.obj[strings.ToLower(name)]
	return id, ok, nil
}

func (f fixtureResolver) EnumValues(id int) (map[int]int, error) {
	if values, ok := f.enums[id]; ok {
		return values, nil
	}
	return map[int]int{}, nil
}

func (f fixtureResolver) ObjNames() (map[int]string, error) {
	return f.objNames, nil
}

func (f fixtureResolver) InterfaceID(name string) (int, bool, error) {
	return f.ifaceID, f.ifaceOK, nil
}

const fixtureSmithingJSON = `{
  "smelting": [
    {"name": "Bronze bar", "bar": "BRONZE_BAR", "level": 1, "xp": 6.2,
     "ingredients": [["COPPER_ORE", 1], ["TIN_ORE", 1]],
     "barPhrase": "a bronze bar", "smeltMessage": "You smelt the copper and tin together in the furnace.",
     "retrieveMessage": "You retrieve a bar of bronze.",
     "missingMessage": "You need at least one copper ore and one tin ore to smelt a bronze bar."},
    {"name": "Iron bar", "bar": "IRON_BAR", "level": 15, "xp": 12.5,
     "ingredients": [["IRON_ORE", 1]],
     "barPhrase": "an iron bar", "smeltMessage": "You smelt the iron in the furnace.",
     "retrieveMessage": "You retrieve a bar of iron.",
     "missingMessage": "You need at least one iron ore to smelt an iron bar.",
     "successChance": 0.5, "failMessage": "The ore is too impure and you fail to refine it."},
    {"name": "Molten glass", "bar": "MOLTEN_GLASS", "members": true, "level": 1, "xp": 20,
     "skill": "CRAFTING", "firstDelay": 2, "finishDelay": 0, "interval": 2, "notInMenu": true,
     "ingredients": [["BUCKET_OF_SAND", 1], ["SODA_ASH", 1]],
     "missingMessage": "You need sand and soda ash to make glass.",
     "barPhrase": "molten glass", "smeltMessage": null,
     "retrieveMessage": "You heat the sand and soda ash in the furnace to make glass.",
     "byproducts": [["BUCKET", 1]]}
  ],
  "cannonballs": [
    {"ball": "BRONZE_CANNONBALL", "bar": "BRONZE_BAR", "level": 5, "xpPerBar": 8.8},
    {"ball": "STEEL_CANNONBALL", "bar": "STEEL_BAR", "level": 35, "xpPerBar": 25.6}
  ],
  "blurite": [
    {"item": "BLURITE_BOLTS_UNF_", "level": 8, "xp": 17.5, "amount": 10, "phrase": "ten blurite crossbow bolts"}
  ],
  "spears": [
    {"bar": "BRONZE_BAR", "log": "LOGS", "level": 5, "xp": 25, "spear": "bronze_spear", "hasta": "brut_bronze_spear"}
  ],
  "phrases": {"Dagger": "a dagger", "Axe": "an axe"},
  "objPhrases": {"oil_lantern_frame": "an oil lantern frame"},
  "questNames": {"the_knights_sword": "The Knight's Sword"},
  "xpPerBar": {"BRONZE_BAR": 12.5, "IRON_BAR": 25},
  "productXp": {"bronze_dagger": 99}
}`

const fixtureLayoutJSON = `{
  "bars": [
    {"barType": 1, "bar": 2349, "obj": "bronze_bar", "slots": [
      {"component": 9, "item": 1205, "obj": "bronze_dagger", "label": "Dagger"},
      {"component": 14, "item": 1351, "obj": "bronze_axe", "label": "Axe"},
      {"component": 20, "item": 9991, "obj": "oil_lantern_frame", "label": "Lantern frame"},
      {"component": 21, "item": 9992, "obj": "bronze_thing", "label": "Thing"}
    ]}
  ]
}`

func newFixtureResolver(withSlotIndex bool) fixtureResolver {
	obj := map[string]int{
		"bronze_bar": 2349, "iron_bar": 2351, "steel_bar": 2353,
		"molten_glass": 1775, "copper_ore": 436, "tin_ore": 438,
		"iron_ore": 440, "soda_ash": 1781, "logs": 1511,
		"bronze_spear": 1237, "brut_bronze_spear": 11367,
		"bronze_cannonball": 31906,
		"bronze_dagger":     1205, "bronze_axe": 1351,
		"oil_lantern_frame": 9991, "bronze_thing": 9992,
	}
	enums := map[int]map[int]int{
		846: {1205: 0, 1351: 0, 9991: 12, 9992: 3}, // 0 means level 1
		845: {9991: 2},                             // dagger/axe/thing default to 1 bar
		844: {9992: 15},                            // thing makes 15; the rest default to 1
	}
	if withSlotIndex {
		enums[1101] = map[int]int{
			(312 << 16) | 9:  1,
			(312 << 16) | 14: 6,
			(312 << 16) | 20: 12,
			(312 << 16) | 21: 13,
		}
	}
	return fixtureResolver{
		obj:      obj,
		enums:    enums,
		objNames: map[int]string{2349: "bronze_bar"},
		ifaceID:  312,
		ifaceOK:  true,
	}
}

func loadFixture(t *testing.T, withSlotIndex bool) *Tables {
	t.Helper()
	data, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte(fixtureLayoutJSON))
	if err != nil {
		t.Fatalf("parseSmithingData(fixture): %v", err)
	}
	tables, err := Load(data, newFixtureResolver(withSlotIndex))
	if err != nil {
		t.Fatalf("Load(fixture): %v", err)
	}
	return tables
}

func TestLoadFixtureCounts(t *testing.T) {
	tables := loadFixture(t, true)
	if got := len(tables.Smelting); got != 3 {
		t.Errorf("Smelting = %d, want 3", got)
	}
	if got := len(tables.Cannonballs); got != 2 {
		t.Errorf("Cannonballs = %d, want 2", got)
	}
	if got := len(tables.Blurite); got != 1 {
		t.Errorf("Blurite = %d, want 1", got)
	}
	if got := len(tables.Spears); got != 1 {
		t.Errorf("Spears = %d, want 1", got)
	}
	if got := len(tables.Anvil); got != 4 {
		t.Errorf("Anvil = %d, want 4", got)
	}
	if tables.QuestNames["the_knights_sword"] != "The Knight's Sword" {
		t.Errorf("QuestNames = %v", tables.QuestNames)
	}
}

func TestLoadFixtureSmelting(t *testing.T) {
	tables := loadFixture(t, true)
	iron := tables.SmeltByBar(2351)
	if iron == nil {
		t.Fatal("SmeltByBar(iron) = nil")
	}
	if !iron.HasSuccessChance || iron.SuccessChance != 0.5 || iron.FailMessage == "" {
		t.Errorf("iron = %+v, want successChance 0.5 + fail message", iron)
	}
	glass := tables.SmeltByBar(1775)
	if glass == nil {
		t.Fatal("SmeltByBar(molten glass) = nil")
	}
	if glass.Skill != "CRAFTING" || !glass.HasTiming || glass.Interval != 2 || glass.FirstDelay != 2 || glass.FinishDelay != 0 {
		t.Errorf("molten glass = %+v, want CRAFTING timing [2,2,0]", glass)
	}
	if !glass.NotInMenu || !glass.Members || glass.SmeltMessage != "" {
		t.Errorf("molten glass = %+v, want notInMenu + members + empty smelt message", glass)
	}
	if len(glass.Byproducts) != 1 || glass.Byproducts[0].ItemID != 1925 {
		t.Errorf("molten glass byproducts = %v, want bucket 1925 (fallback)", glass.Byproducts)
	}
	// Fallbacks resolve without reaching the resolver: the
	// fixture resolver has no blurite entries at all.
	if tables.Blurite[0].ItemID != 9376 {
		t.Errorf("blurite bolts = %d, want fallback 9376", tables.Blurite[0].ItemID)
	}
	if tables.Cannonballs[1].BallID != 2 {
		t.Errorf("steel cannonball = %d, want fallback 2", tables.Cannonballs[1].BallID)
	}
	// Bar and BarID both hold the bar item id.
	if tables.Smelting[0].BarID != 2349 || tables.Smelting[0].Bar != 2349 {
		t.Errorf("bronze BarID/Bar = %d/%d, want 2349/2349", tables.Smelting[0].BarID, tables.Smelting[0].Bar)
	}
	if tables.SmeltByBar(999999) != nil {
		t.Errorf("SmeltByBar(unknown) = non-nil")
	}
}

func TestLoadFixtureAnvil(t *testing.T) {
	tables := loadFixture(t, true)
	// Enum 846 zero means level 1; productXp overrides the
	// xpPerBar x bars rate (99 for the bronze dagger).
	dagger := tables.AnvilByComponent(1, 9)
	if dagger == nil || dagger.Level != 1 || dagger.XP != 99 || dagger.SlotIndex != 1 || dagger.Phrase != "a dagger" {
		t.Errorf("dagger = %+v, want level 1 xp 99 slotIndex 1 phrase 'a dagger'", dagger)
	}
	axe := tables.AnvilByComponent(1, 14)
	if axe == nil || axe.Level != 1 || axe.XP != 12.5 || axe.SlotIndex != 6 || axe.Metal != "bronze" {
		t.Errorf("axe = %+v, want level 1 xp 12.5 slotIndex 6 metal bronze", axe)
	}
	// objPhrases wins over the label phrase.
	lantern := tables.AnvilByComponent(1, 20)
	if lantern == nil || lantern.Phrase != "an oil lantern frame" || lantern.Bars != 2 || lantern.Level != 12 {
		t.Errorf("lantern frame = %+v, want phrase 'an oil lantern frame' bars 2 level 12", lantern)
	}
	// No phrase anywhere: "a <label>"; enum 844 amount 15.
	thing := tables.AnvilByComponent(1, 21)
	if thing == nil || thing.Phrase != "a thing" || thing.Amount != 15 || thing.Bars != 1 {
		t.Errorf("thing = %+v, want phrase 'a thing' amount 15 bars 1", thing)
	}
	if got := len(tables.AnvilByBar(2349)); got != 4 {
		t.Errorf("AnvilByBar(2349) = %d, want 4", got)
	}
	if tables.AnvilByBar(999999) != nil {
		t.Errorf("AnvilByBar(unknown) = non-nil")
	}
	if tables.AnvilByComponent(1, 99) != nil || tables.AnvilByComponent(2, 9) != nil {
		t.Errorf("AnvilByComponent(unknown) = non-nil")
	}
	// Nil-table lookups are nil-safe.
	var nilTables *Tables
	if nilTables.SmeltByBar(2349) != nil || nilTables.AnvilByBar(2349) != nil || nilTables.AnvilByComponent(1, 9) != nil {
		t.Errorf("nil Tables lookups = non-nil")
	}
}

func TestLoadFixtureSlotIndexDerived(t *testing.T) {
	// Enum 1101 absent: slotIndex derives from layout order,
	// 1-based within the bar's slots.
	tables := loadFixture(t, false)
	want := map[int]int{9: 1, 14: 2, 20: 3, 21: 4}
	for _, p := range tables.Anvil {
		if want[p.Component] != p.SlotIndex {
			t.Errorf("component %d: slotIndex %d, want derived %d", p.Component, p.SlotIndex, want[p.Component])
		}
	}
}

func TestLoadFixtureUnresolvedFailsClosed(t *testing.T) {
	data, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte(fixtureLayoutJSON))
	if err != nil {
		t.Fatalf("parseSmithingData: %v", err)
	}
	resolver := newFixtureResolver(true)
	delete(resolver.obj, "tin_ore")
	delete(resolver.obj, "bronze_spear")
	if _, err := Load(data, resolver); err == nil {
		t.Fatal("Load with missing tin_ore + bronze_spear: want error")
	} else {
		// Both missing names are listed, not just the first.
		if !strings.Contains(err.Error(), "TIN_ORE") || !strings.Contains(err.Error(), "bronze_spear") {
			t.Errorf("error %v, want both TIN_ORE and bronze_spear listed", err)
		}
	}
}

func TestLoadFixtureLayoutMismatch(t *testing.T) {
	// A layout slot whose obj resolves to a different id than
	// the slot's item is a data error.
	layout := `{"bars": [{"barType": 1, "bar": 2349, "obj": "bronze_bar", "slots": [
		{"component": 9, "item": 1351, "obj": "bronze_dagger", "label": "Dagger"}
	]}]}`
	data, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte(layout))
	if err != nil {
		t.Fatalf("parseSmithingData: %v", err)
	}
	if _, err := Load(data, newFixtureResolver(true)); err == nil {
		t.Fatal("Load with layout mismatch: want error")
	}
}

func TestLoadFixtureMissingLevel(t *testing.T) {
	// A slot whose item has no enum-846 level fails the load:
	// TS would read NaN, Go fails closed instead.
	resolver := newFixtureResolver(true)
	delete(resolver.enums[846], 9992)
	data, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte(fixtureLayoutJSON))
	if err != nil {
		t.Fatalf("parseSmithingData: %v", err)
	}
	if _, err := Load(data, resolver); err == nil {
		t.Fatal("Load with missing enum-846 level: want error")
	}
}

func TestLoadBadInputs(t *testing.T) {
	data, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte(fixtureLayoutJSON))
	if err != nil {
		t.Fatalf("parseSmithingData: %v", err)
	}
	if _, err := Load(nil, newFixtureResolver(true)); err == nil {
		t.Errorf("Load(nil data): want error")
	}
	if _, err := Load(data, nil); err == nil {
		t.Errorf("Load(nil resolver): want error")
	}
	if _, err := LoadFile("../../../data/definitions/does-not-exist.json", newFixtureResolver(true)); err == nil {
		t.Errorf("LoadFile(missing file): want error")
	}
	if _, err := LoadFromCache(nil); err == nil {
		t.Errorf("LoadFromCache(nil): want error")
	}
	// Malformed JSON and empty sections fail.
	if _, err := parseSmithingData([]byte("{nope"), []byte(fixtureLayoutJSON)); err == nil {
		t.Errorf("parseSmithingData(malformed smithing): want error")
	}
	if _, err := parseSmithingData([]byte(fixtureSmithingJSON), []byte("{nope")); err == nil {
		t.Errorf("parseSmithingData(malformed layout): want error")
	}
	empty := `{"smelting": [], "cannonballs": [], "blurite": [], "spears": [], "bars": []}`
	emptyData, err := parseSmithingData([]byte(empty), []byte(`{"bars": []}`))
	if err != nil {
		t.Fatalf("parseSmithingData(empty): %v", err)
	}
	if _, err := Load(emptyData, newFixtureResolver(true)); err == nil {
		t.Errorf("Load(empty sections): want error")
	}
}

func TestSetTables(t *testing.T) {
	defer SetTables(nil)
	if TablesLoaded() {
		t.Errorf("TablesLoaded() before install = true")
	}
	tables := loadFixture(t, true)
	SetTables(tables)
	if !TablesLoaded() {
		t.Errorf("TablesLoaded() after install = false")
	}
	SetTables(nil)
	if TablesLoaded() {
		t.Errorf("TablesLoaded() after uninstall = true")
	}
}
