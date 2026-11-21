package definition

import (
	"strings"
	"testing"

	"tsps-go/internal/cache"
)

// itemCacheSpec builds a fake cache with a small item set for the
// wrapper tests. Id 0 is the default "null" entry.
func itemCacheSpec() fakeCacheSpec {
	return fakeCacheSpec{
		objs: map[int][]byte{
			0: {0},
			1: objEntry("Coins", objStackable(), objPrice(1), objTradable()),
			2: objEntry("Bronze sword", objPrice(10), objWearPos(3), objWeight(5), objTradable(),
				objNote(3), objParams([2]int{0, 5}, [2]int{14, 4}, [2]int{13, 1})),
			3: objEntry("Bronze sword note", objNote(2), objNoteTemplate(2)),
			4: objEntry("Untradable thing", objPrice(100), objInventoryAction(4, "Destroy")),
			5: objEntry("Two-handed sword", objPrice(50), objWearPos(3), objWearPos2(5), objTradable()),
			6: objEntry("Helmet", objPrice(20), objWearPos(0), objTradable(),
				objParams([2]int{299, 20}, [2]int{11, 1}, [2]int{12, 3}, [2]int{189, 2})),
			7: objEntry("Placeholder item", objPrice(1), objPlaceholder(8)),
			8: objEntry("Faded placeholder", objPrice(0), objPlaceholderTemplate(7)),
			9: objEntry("Slow weapon", objPrice(1), objParams([2]int{14, 0})),
		},
	}
}

func loadItems(t *testing.T, files map[string]string, spec fakeCacheSpec) *Store {
	t.Helper()
	defs, gamevals := newFakeCache(t, spec)
	dir := fixtureDir(t, files)
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestItemHydrateFromCache(t *testing.T) {
	store := loadItems(t, minimalFiles(), itemCacheSpec())

	coins := store.Item(1)
	if coins.Name() != "Coins" || !coins.IsStackable() || !coins.IsTradeable() || coins.Value() != 1 {
		t.Fatalf("coins = %+v", coins)
	}
	if !coins.IsDropable() {
		t.Fatal("coins must be dropable (default inventory action 4 is Drop)")
	}

	sword := store.Item(2)
	if sword.Name() != "Bronze sword" || sword.Value() != 10 || sword.Weight() != 5 {
		t.Fatalf("sword = %+v", sword)
	}
	if sword.EquipmentSlot() != 3 || sword.IsDoubleHanded() {
		t.Fatalf("sword slot=%d double=%v", sword.EquipmentSlot(), sword.IsDoubleHanded())
	}
	if sword.AttackSpeed() != 4 || sword.AttackRange() != 1 {
		t.Fatalf("sword speed=%d range=%d", sword.AttackSpeed(), sword.AttackRange())
	}
	bonuses := sword.Bonuses()
	if len(bonuses) != 14 || bonuses[0] != 5 || !sword.HasCacheBonuses() {
		t.Fatalf("sword bonuses = %v", bonuses)
	}
	if sword.NoteID() != 3 || sword.IsNoted() {
		t.Fatalf("sword note=%d noted=%v", sword.NoteID(), sword.IsNoted())
	}

	note := store.Item(3)
	if !note.IsNoted() || note.NoteID() != 2 || note.UnNote() != 2 {
		t.Fatalf("note = noted=%v noteID=%d unnote=%d", note.IsNoted(), note.NoteID(), note.UnNote())
	}
	if sword.UnNote() != 2 {
		t.Fatalf("sword unnote = %d", sword.UnNote())
	}

	untradable := store.Item(4)
	if untradable.IsTradeable() || untradable.IsDropable() {
		t.Fatalf("untradable = tradeable=%v dropable=%v", untradable.IsTradeable(), untradable.IsDropable())
	}

	twoHanded := store.Item(5)
	if !twoHanded.IsDoubleHanded() || twoHanded.EquipmentSlot() != 3 {
		t.Fatalf("two-handed = double=%v slot=%d", twoHanded.IsDoubleHanded(), twoHanded.EquipmentSlot())
	}

	helmet := store.Item(6)
	if helmet.EquipmentSlot() != 0 {
		t.Fatalf("helmet slot = %d", helmet.EquipmentSlot())
	}
	hb := helmet.Bonuses()
	if len(hb) != 14 || hb[11] != 5 || hb[12] != 2.0 || hb[13] != 1 {
		t.Fatalf("helmet bonuses = %v (ranged str 12+189, magic dmg 299/10, prayer 11)", hb)
	}

	if got := store.Item(7).PlaceholderID(); got != 8 {
		t.Fatalf("placeholder id = %d", got)
	}
	if got := store.Item(8).PlaceholderID(); got != -1 {
		t.Fatalf("placeholder-template item placeholder id = %d", got)
	}
	if got := store.Item(9).AttackSpeed(); got != -1 {
		t.Fatalf("attack speed param 0 must yield -1, got %d", got)
	}
}

func TestItemCoinsTradeableSpecialCase(t *testing.T) {
	// Item 995 is tradeable even when the cache says otherwise.
	objs := map[int][]byte{0: {0}}
	for i := 1; i < 995; i++ {
		objs[i] = []byte{0}
	}
	objs[995] = objEntry("Coins", objStackable(), objPrice(1))
	defs, _ := newFakeCache(t, fakeCacheSpec{objs: objs})
	dir := fixtureDir(t, minimalFiles())
	store, err := Load(dir, LoadOptions{Cache: defs})
	if err != nil {
		t.Fatal(err)
	}
	if !store.Item(995).IsTradeable() {
		t.Fatal("coins (995) must be tradeable without the cache flag")
	}
}

func TestItemDefaults(t *testing.T) {
	store := loadItems(t, minimalFiles(), itemCacheSpec())
	def := store.Item(9999)
	if def.ID() != -1 || def.Name() != "" || def.EquipmentSlot() != -1 {
		t.Fatalf("default = %+v", def)
	}
	if def.BlockAnim() != 424 || def.StandAnim() != 808 || def.WalkAnim() != 819 || def.RunAnim() != 824 {
		t.Fatal("default anims wrong")
	}
	if def.AttackAnim("stab") != -1 || def.EquipSound() != -1 || def.Bonuses() != nil {
		t.Fatalf("default attack/equip/bonuses wrong")
	}
	if def.GrandExchangeValue() != 0 || def.HasGrandExchangeValue() {
		t.Fatal("default GE value must fall back to 0 value")
	}
}

func TestItemGameplayApplication(t *testing.T) {
	gameplay := `[
		{"id": 2, "name": "  bronze sword ", "sellable": true, "bloodMoneyValue": 50,
		 "highAlch": 6, "lowAlch": 4, "dropValue": 3, "blockAnim": 100, "standAnim": 101,
		 "walkAnim": 102, "runAnim": 103, "standTurnAnim": 104, "turn180Anim": 105,
		 "turn90CWAnim": 106, "turn90CCWAnim": 107, "attackAnim": 108, "equipSound": 109,
		 "bonuses": [9,9,9,9,9,9,9,9,9,9,9,9,9,9], "requirements": [1,0,0,0,0],
		 "deathComponents": [1], "combatRequirement": 10, "weaponInterface": "Sword",
		 "equipmentType": "shield", "value": 999, "weight": 99, "examine": "Shiny."},
		{"id": 4, "name": "Untradable thing", "bonuses": [1,2,3,4,5,6,7,8,9,10,11,12,13,14],
		 "attackAnim": {"stab": 200, "slash": 0}},
		{"id": 6, "name": "Wrong name", "sellable": true},
		{"id": 999, "name": "Nope", "sellable": true},
		{"id": "bad", "name": "Bad", "sellable": true},
		"not-an-object"
	]`
	store := loadItems(t, withFiles(map[string]string{"item-gameplay.json": gameplay}), itemCacheSpec())

	sword := store.Item(2)
	if !sword.IsSellable() || sword.BloodMoneyValue() != 50 || sword.HighAlchValue() != 6 ||
		sword.LowAlchValue() != 4 || sword.DropValue() != 3 {
		t.Fatalf("sword gameplay values wrong: %+v", sword)
	}
	if sword.BlockAnim() != 100 || sword.StandAnim() != 101 || sword.AttackAnim("stab") != 108 || sword.EquipSound() != 109 {
		t.Fatal("sword anims/sound wrong")
	}
	// Cache wins over the export for bonuses, slot, value, weight, examine.
	if sword.Bonuses()[0] != 5 || sword.EquipmentSlot() != 3 || sword.Value() != 10 || sword.Weight() != 5 || sword.Examine() != "" {
		t.Fatalf("cache must win: bonuses=%v slot=%d value=%d weight=%d examine=%q",
			sword.Bonuses(), sword.EquipmentSlot(), sword.Value(), sword.Weight(), sword.Examine())
	}
	if len(sword.Requirements()) != 5 || sword.Requirements()[0] != 1 {
		t.Fatalf("requirements = %v", sword.Requirements())
	}
	if len(sword.DeathComponents()) != 1 || sword.DeathComponents()[0] != 1 {
		t.Fatalf("death components = %v", sword.DeathComponents())
	}
	if sword.CombatRequirement() != 10 || sword.WeaponInterfaceName() != "Sword" {
		t.Fatalf("combat req=%d iface=%q", sword.CombatRequirement(), sword.WeaponInterfaceName())
	}

	// No cache bonuses: the export's bonuses apply.
	thing := store.Item(4)
	if len(thing.Bonuses()) != 14 || thing.Bonuses()[0] != 1 || thing.Bonuses()[13] != 14 {
		t.Fatalf("thing bonuses = %v", thing.Bonuses())
	}
	// Map-form attack anims: 0 counts as missing.
	if thing.AttackAnim("stab") != 200 || thing.AttackAnim("slash") != -1 || thing.AttackAnim("crush") != -1 {
		t.Fatalf("map attack anims: stab=%d slash=%d crush=%d",
			thing.AttackAnim("stab"), thing.AttackAnim("slash"), thing.AttackAnim("crush"))
	}

	// Name mismatch: skipped.
	if store.Item(6).IsSellable() {
		t.Fatal("mismatched-name entry must be skipped")
	}
}

func TestItemGameplayObjectFormAndMissing(t *testing.T) {
	store := loadItems(t, withFiles(map[string]string{
		"item-gameplay.json": `{"2": {"id": 2, "name": "Bronze sword", "sellable": true}}`,
	}), itemCacheSpec())
	if !store.Item(2).IsSellable() {
		t.Fatal("object-form gameplay must apply")
	}
	// Missing file is tolerated (deliberate deviation).
	store2 := loadItems(t, minimalFiles(), itemCacheSpec())
	if store2.Item(2).IsSellable() {
		t.Fatal("no gameplay file must mean no overlay")
	}
}

func TestItemGameplayMalformedFails(t *testing.T) {
	defs, _ := newFakeCache(t, itemCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"item-gameplay.json": `[broken`}))
	if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
		t.Fatal("malformed item-gameplay.json must fail")
	}
}

func TestItemPrices(t *testing.T) {
	prices := `{"data": {
		"2": {"high": 100, "low": 50},
		"4": {"high": 50, "low": 100},
		"5": {"high": 0, "low": -5},
		"6": {"high": "lots"},
		"999": {"high": 10, "low": 10}
	}}`
	store := loadItems(t, withFiles(map[string]string{"item-prices.json": prices}), itemCacheSpec())
	sword := store.Item(2)
	if sword.GrandExchangeValue() != 100 || sword.GrandExchangeSellValue() != 50 || !sword.HasGrandExchangeValue() {
		t.Fatalf("sword GE = %d/%d", sword.GrandExchangeValue(), sword.GrandExchangeSellValue())
	}
	// Inverted quotes: buy is the max, sell the min.
	thing := store.Item(4)
	if thing.GrandExchangeValue() != 100 || thing.GrandExchangeSellValue() != 50 {
		t.Fatalf("thing GE = %d/%d", thing.GrandExchangeValue(), thing.GrandExchangeSellValue())
	}
	// Invalid quotes: no GE value, falls back to store value.
	if store.Item(5).HasGrandExchangeValue() || store.Item(5).GrandExchangeValue() != 50 {
		t.Fatal("item 5 must fall back to store value")
	}
	if store.Item(6).HasGrandExchangeValue() {
		t.Fatal("string quote must not apply")
	}
	// Note propagation.
	note := store.Item(3)
	if note.GrandExchangeValue() != 100 || note.GrandExchangeSellValue() != 50 {
		t.Fatalf("note GE = %d/%d", note.GrandExchangeValue(), note.GrandExchangeSellValue())
	}
}

func TestItemPricesBadShapeFails(t *testing.T) {
	defs, _ := newFakeCache(t, itemCacheSpec())
	for _, content := range []string{`{}`, `[]`, `{"data": []}`} {
		dir := fixtureDir(t, withFiles(map[string]string{"item-prices.json": content}))
		if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
			t.Fatalf("item-prices.json %s must fail", content)
		}
	}
}

func TestItemAlch(t *testing.T) {
	store := loadItems(t, withFiles(map[string]string{
		"item-gameplay.json": `[{"id": 2, "name": "Bronze sword", "highAlch": 99, "lowAlch": 98}]`,
		"item-alch.json": `{
			"2": {"highalch": 6, "lowalch": 4},
			"4": {"highalch": "x", "lowalch": 1.5},
			"5": {}
		}`,
	}), itemCacheSpec())
	// Alch runs after gameplay and wins.
	sword := store.Item(2)
	if sword.HighAlchValue() != 6 || sword.LowAlchValue() != 4 {
		t.Fatalf("sword alch = %d/%d", sword.HighAlchValue(), sword.LowAlchValue())
	}
	// Non-safe-int values become 0.
	thing := store.Item(4)
	if thing.HighAlchValue() != 0 || thing.LowAlchValue() != 0 {
		t.Fatalf("thing alch = %d/%d", thing.HighAlchValue(), thing.LowAlchValue())
	}
	// Missing values become 0 (the entry still counts as loaded).
	if store.Item(5).HighAlchValue() != 0 {
		t.Fatal("item 5 alch must be 0")
	}
	// Note propagation.
	if store.Item(3).HighAlchValue() != 6 {
		t.Fatalf("note alch = %d", store.Item(3).HighAlchValue())
	}
}

func TestItemAlchBadShapeFails(t *testing.T) {
	defs, _ := newFakeCache(t, itemCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"item-alch.json": `[]`}))
	if _, err := Load(dir, LoadOptions{Cache: defs}); err == nil {
		t.Fatal("array item-alch.json must fail")
	}
}

func TestItemOverrides(t *testing.T) {
	overrides := `{
		"2": {"value": 500, "tradeable": false, "attackSpeed": 3,
		      "bonuses": [1,1,1,1,1,1,1,1,1,1,1,1,1,1],
		      "requirements": [1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1,1],
		      "examine": "Overridden.", "grandExchangeValue": 777, "unknownField": 1},
		"4": {"attackSpeed": 0, "bonuses": [1,2,3], "examine": 42, "value": -1},
		"5": {"attackSpeed": 21},
		"999": {"value": 1}
	}`
	var logs logCapture
	defs, gamevals := newFakeCache(t, itemCacheSpec())
	dir := fixtureDir(t, withFiles(map[string]string{"item-overrides.json": overrides}))
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals, Logf: logs.logf})
	if err != nil {
		t.Fatal(err)
	}
	sword := store.Item(2)
	if sword.Value() != 500 || sword.IsTradeable() || sword.AttackSpeed() != 3 {
		t.Fatalf("sword overrides wrong: value=%d tradeable=%v speed=%d", sword.Value(), sword.IsTradeable(), sword.AttackSpeed())
	}
	if sword.Bonuses()[0] != 1 || len(sword.Requirements()) != 23 || sword.Examine() != "Overridden." {
		t.Fatal("sword bonuses/requirements/examine overrides wrong")
	}
	if sword.GrandExchangeValue() != 777 {
		t.Fatalf("sword GE override = %d", sword.GrandExchangeValue())
	}
	// Invalid fields are skipped, keeping cache values.
	thing := store.Item(4)
	if thing.AttackSpeed() != -1 || thing.Bonuses() != nil || thing.Value() != 100 || thing.Examine() != "" {
		t.Fatalf("thing must keep cache values: speed=%d bonuses=%v value=%d", thing.AttackSpeed(), thing.Bonuses(), thing.Value())
	}
	if store.Item(5).AttackSpeed() != -1 {
		t.Fatal("attackSpeed 21 must be rejected")
	}
	if !logs.contains("skipped unknown item 999") || !logs.contains("skipped invalid unknownField") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestItemOverridesExamineLength(t *testing.T) {
	long := strings.Repeat("x", 201)
	store := loadItems(t, withFiles(map[string]string{
		"item-overrides.json": `{"2": {"examine": "` + long + `"}}`,
	}), itemCacheSpec())
	if store.Item(2).Examine() != "" {
		t.Fatal("examine over 200 chars must be rejected")
	}
}

func TestBarrowsDegradation(t *testing.T) {
	objs := map[int][]byte{0: {0}}
	for i := 1; i <= 4860; i++ {
		objs[i] = []byte{0}
	}
	objs[4708] = objEntry("Ahrim's hood", objWearPos(0), objTradable())
	objs[4856] = objEntry("Ahrim's hood 100")
	objs[4860] = objEntry("Ahrim's hood 0")
	gameplay := `[{"id": 4708, "name": "Ahrim's hood", "requirements": [70,0,0,0,0], "weaponInterface": "None"}]`
	store := loadItems(t, withFiles(map[string]string{"item-gameplay.json": gameplay}), fakeCacheSpec{objs: objs})
	variant := store.Item(4856)
	if variant.EquipmentSlot() != 0 {
		t.Fatalf("variant slot = %d", variant.EquipmentSlot())
	}
	if len(variant.Requirements()) != 5 || variant.Requirements()[0] != 70 {
		t.Fatalf("variant requirements = %v", variant.Requirements())
	}
	if variant.WeaponInterfaceName() != "None" {
		t.Fatalf("variant iface = %q", variant.WeaponInterfaceName())
	}
	broken := store.Item(4860)
	bonuses := broken.Bonuses()
	if len(bonuses) != 14 {
		t.Fatalf("broken bonuses = %v", bonuses)
	}
	for i, b := range bonuses {
		if b != 0 {
			t.Fatalf("broken bonus[%d] = %v", i, b)
		}
	}
}

func TestBonusesFromParams(t *testing.T) {
	if _, ok := BonusesFromParams(nil); ok {
		t.Fatal("nil params must yield no bonuses")
	}
	if _, ok := BonusesFromParams(cache.ParamsMap{}); ok {
		t.Fatal("empty params must yield no bonuses")
	}
	bonuses, ok := BonusesFromParams(cache.ParamsMap{0: int32(3), 5: 2, 299: 25, 189: 4})
	if !ok || len(bonuses) != 14 {
		t.Fatalf("bonuses = %v ok=%v", bonuses, ok)
	}
	if bonuses[0] != 3 || bonuses[5] != 2 || bonuses[11] != 4 || bonuses[12] != 2.5 || bonuses[13] != 0 {
		t.Fatalf("bonuses = %v", bonuses)
	}
	// An unrelated param alone yields no bonuses.
	if _, ok := BonusesFromParams(cache.ParamsMap{999: 1}); ok {
		t.Fatal("unrelated param must yield no bonuses")
	}
}
