package cooking

import (
	"strings"
	"testing"
)

// ---- fake resolver (hand-built fixtures only; no live cache) ----

type fakeResolver struct {
	objs    map[string]int
	seqs    map[string]int
	varbits map[string]int
	members map[int]bool
}

func (f fakeResolver) ObjID(name string) (int, bool, error) {
	id, ok := f.objs[name]
	return id, ok, nil
}
func (f fakeResolver) SeqID(name string) (int, bool, error) {
	id, ok := f.seqs[name]
	return id, ok, nil
}
func (f fakeResolver) VarbitID(name string) (int, bool, error) {
	id, ok := f.varbits[name]
	return id, ok, nil
}
func (f fakeResolver) IsMembersItem(id int) bool { return f.members[id] }

func fixtureResolver() fakeResolver {
	return fakeResolver{
		objs: map[string]int{
			"raw_shrimp": 100, "shrimp": 101, "burnt_shrimp": 102,
			"raw_beef": 110, "cooked_meat": 111, "burnt_meat": 112,
			"raw_chicken": 113, "cooked_chicken": 114, "burnt_chicken": 115,
			"raw_guppy": 120, "guppy": 121, "ruined_guppy": 122,
			"pot_flour": 200, "pot_empty": 201,
			"bucket_water": 210, "bucket_empty": 211,
			"bowl_water": 212, "bowl_empty": 213,
			"jug_water": 214, "jug_empty": 215,
			"bread_dough": 220, "pastry_dough": 221, "pizza_base": 222, "uncooked_pitta_bread": 223,
			"swamp_tar": 230, "rawswamppaste": 231,
			"piedish": 232, "pie_shell": 233,
			"uncooked_meat_pie": 234,
			"half_baked_bowl":   240, "chocolate_bar": 241, "equa_leaves": 242,
			"chocolate_dust": 243, "pot_of_cream": 244, "gnomebowl_mould": 245,
			"chocolate_bomb": 246,
			"grapes":         300, "jug_unfermented_wine": 301, "jug_wine": 302, "jug_bad_wine": 303,
			"zamorak_grapes": 310, "jug_unfermented_zamorak_wine": 311, "wine_of_zamorak": 312,
			"pot_of_butter": 320, "pot_of_not_garlic_butter": 321, "bucket_milk": 322,
			"cheese": 323, "drunk_dragon": 9578,
		},
		seqs: map[string]int{
			"player_churns_milk_short": 9001, "player_churns_milk_medium": 9002,
			"player_churns_milk_long": 9003,
		},
		varbits: map[string]int{"cooking_success_count": 7001, "busy": 7002},
		members: map[int]bool{246: true, 223: true},
	}
}

const fixtureJSON = `{
  "cookables": [
    {"raw":"raw_shrimp","cooked":"shrimp","burnt":"burnt_shrimp","level":1,"xp":30,"chance":{"fire":[128,512],"range":[128,512]},"name":"shrimps","article":"some"},
    {"raw":"raw_shrimp","cooked":"cooked_meat","burnt":"burnt_meat","level":1,"xp":99,"name":"shrimps"},
    {"raw":"raw_guppy","cooked":"guppy","burnt":"ruined_guppy","level":7,"xp":12,"stopBurn":42,"preparationStopBurn":36,"facility":"camdozaal_preparation_table","name":"guppy"},
    {"raw":9576,"cooked":"drunk_dragon","burnt":"drunk_dragon","level":32,"xp":130,"stopBurn":38,"failsWithoutBurning":true,"name":"mixed dragon","gnomeTutorial":true,"rangeOnly":true,"firstCookTicks":1}
  ],
  "doughs": {
    "flour": ["pot_flour", "pot_empty"],
    "waters": [["bucket_water", "bucket_empty"], ["bowl_water", "bowl_empty"], ["jug_water", "jug_empty"]],
    "products": [
      {"product": "bread_dough", "level": 1, "message": "You mix the water and flour to make some bread dough."},
      {"product": "pastry_dough", "level": 1, "message": "You mix the water and flour to make some pastry dough."},
      {"product": "pizza_base", "level": 35, "message": "You mix the water and flour to make a pizza base."},
      {"product": "uncooked_pitta_bread", "level": 58, "message": "You mix the water and flour to make some pitta dough."}
    ]
  },
  "recipes": [
    {"product":"rawswamppaste","level":1,"ingredients":["pot_flour","swamp_tar"],"returns":["pot_empty"]},
    {"product":"pie_shell","level":1,"ingredients":["pastry_dough","piedish"]},
    {"product":"uncooked_meat_pie","level":20,"ingredients":["cooked_meat","pie_shell"]},
    {"product":"uncooked_meat_pie","level":20,"ingredients":["cooked_chicken","pie_shell"]},
    {"product":9563,"level":42,"xp":50,"ingredients":[["half_baked_bowl",1],["chocolate_bar",4],["equa_leaves",1]],"returns":["gnomebowl_mould"],"timing":[1,2]},
    {"product":"chocolate_bomb","level":1,"xp":110,"ingredients":[[9564,1],["chocolate_dust",1],["pot_of_cream",2]],"timing":[1,2]}
  ],
  "wines": [
    {"grapes": "grapes", "unfermented": "jug_unfermented_wine", "wine": "jug_wine", "bad": "jug_bad_wine", "level": 35, "xp": 200, "chance": [48, 352], "pendingLimit": 60000},
    {"grapes": "zamorak_grapes", "unfermented": "jug_unfermented_zamorak_wine", "wine": "wine_of_zamorak", "bad": "jug_bad_wine", "level": 65, "xp": 200, "chance": [-125, 260]}
  ],
  "churn": {
    "labels": {"pot_of_cream": "Cream", "pot_of_butter": "Butter", "cheese": "Cheese"},
    "inputs": [
      {"input": "pot_of_butter", "name": "butter"},
      {"input": "bucket_milk", "name": "milk", "returns": "bucket_empty"}
    ],
    "products": [
      {"input": "bucket_milk", "product": "pot_of_cream", "name": "cream", "level": 21, "xp": 18, "ticks": 10, "seq": "player_churns_milk_short"},
      {"input": "pot_of_butter", "product": "cheese", "name": "cheese", "level": 48, "xp": 23.5, "ticks": 10, "seq": "player_churns_milk_short"}
    ]
  }
}`

func TestLoadFixtureCounts(t *testing.T) {
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(tables.CookablesByRaw); got != 3 {
		t.Errorf("cookables = %d, want 3 (duplicate raw_shrimp collapses)", got)
	}
	if got := len(tables.RecipesByProduct); got != 5 {
		t.Errorf("recipes by product = %d, want 5 (meat-pie rows share a product)", got)
	}
	if got := len(tables.Wines); got != 2 {
		t.Errorf("wines = %d, want 2", got)
	}
	if got := len(tables.Doughs.Products); got != 4 {
		t.Errorf("dough products = %d, want 4", got)
	}
	if got := len(tables.Churn.Inputs); got != 2 {
		t.Errorf("churn inputs = %d, want 2", got)
	}
	if got := len(tables.Churn.Products); got != 2 {
		t.Errorf("churn products = %d, want 2", got)
	}
}

func TestLoadFirstWinsCookable(t *testing.T) {
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	shrimp := tables.CookableFor(100)
	if shrimp == nil {
		t.Fatal("CookableFor(raw_shrimp) = nil")
	}
	// First row wins: cooked shrimp (101), xp 30 — not the
	// second row's cooked meat (111), xp 99.
	if shrimp.CookedID != 101 || shrimp.XP != 30 {
		t.Errorf("shrimp = cooked %d xp %v, want cooked 101 xp 30 (first wins)", shrimp.CookedID, shrimp.XP)
	}
	if shrimp.Article != "some" || shrimp.Name != "shrimps" {
		t.Errorf("shrimp name/article = %q %q, want shrimps/some", shrimp.Name, shrimp.Article)
	}
	if got := shrimp.Chance["fire"]; got != [2]int{128, 512} {
		t.Errorf("shrimp fire chance = %v, want [128 512]", got)
	}
}

func TestLoadFacilityAndNumericCookable(t *testing.T) {
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	guppy := tables.CookableFor(120)
	if guppy == nil {
		t.Fatal("CookableFor(raw_guppy) = nil")
	}
	if guppy.Facility != "camdozaal_preparation_table" || !guppy.HasStopBurn || guppy.StopBurn != 42 ||
		!guppy.HasPreparationStopBurn || guppy.PreparationStopBurn != 36 {
		t.Errorf("guppy = facility %q stopBurn %d/%v prep %d/%v, want camdozaal_preparation_table 42 36",
			guppy.Facility, guppy.StopBurn, guppy.HasStopBurn, guppy.PreparationStopBurn, guppy.HasPreparationStopBurn)
	}
	if guppy.Chance != nil {
		t.Errorf("guppy chance = %v, want nil (stopBurn-only row)", guppy.Chance)
	}
	// Numeric raw id passes through (Common.Cooking.js:30).
	dragon := tables.CookableFor(9576)
	if dragon == nil {
		t.Fatal("CookableFor(9576) = nil")
	}
	if !dragon.FailsWithoutBurning || !dragon.GnomeTutorial || !dragon.RangeOnly ||
		!dragon.HasFirstCookTicks || dragon.FirstCookTicks != 1 {
		t.Errorf("dragon = %+v, want failsWithoutBurning + gnomeTutorial + rangeOnly + firstCookTicks 1", dragon)
	}
}

func TestLoadRecipeDefaultsAndLastWins(t *testing.T) {
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	paste := tables.RecipeForProduct(231)
	if paste == nil {
		t.Fatal("RecipeForProduct(rawswamppaste) = nil")
	}
	// xp ?? 0, timing ?? [1, 2] (Common.Cooking.js:52-53).
	if paste.XP != 0 || len(paste.Timing) != 2 || paste.Timing[0] != 1 || paste.Timing[1] != 2 {
		t.Errorf("paste xp/timing = %v/%v, want 0/[1 2]", paste.XP, paste.Timing)
	}
	if len(paste.Ingredients) != 2 || paste.Ingredients[1] != (Ingredient{ID: 230, Count: 1}) {
		t.Errorf("paste ingredients = %v, want [pot_flour 200, swamp_tar 230]", paste.Ingredients)
	}
	if len(paste.Returns) != 1 || paste.Returns[0] != (Ingredient{ID: 201, Count: 1}) {
		t.Errorf("paste returns = %v, want [pot_empty 201]", paste.Returns)
	}
	// Last-wins product lookup: the chicken meat-pie row wins.
	pie := tables.RecipeForProduct(234)
	if pie == nil {
		t.Fatal("RecipeForProduct(uncooked_meat_pie) = nil")
	}
	if pie.Ingredients[0].ID != 114 {
		t.Errorf("meat pie first ingredient = %d, want cooked_chicken 114 (last wins)", pie.Ingredients[0].ID)
	}
	// Both rows stay reachable by ingredient.
	if got := len(tables.RecipesForIngredient(111)); got != 1 {
		t.Errorf("recipes for cooked_meat = %d, want 1", got)
	}
	if got := len(tables.RecipesForIngredient(114)); got != 1 {
		t.Errorf("recipes for cooked_chicken = %d, want 1", got)
	}
	// Numeric product + [name, count] ingredients + members flag.
	bowl := tables.RecipeForProduct(9563)
	if bowl == nil {
		t.Fatal("RecipeForProduct(9563) = nil")
	}
	if bowl.XP != 50 || bowl.Ingredients[1] != (Ingredient{ID: 241, Count: 4}) {
		t.Errorf("bowl xp/ingredients = %v/%v, want 50/[half_baked_bowl 240, chocolate_bar 241x4, equa_leaves 242]",
			bowl.XP, bowl.Ingredients)
	}
	bomb := tables.RecipeForProduct(246)
	if bomb == nil || !bomb.Members {
		t.Errorf("chocolate_bomb members = %v, want true (cache flag)", bomb == nil || bomb.Members)
	}
	if bomb != nil && bomb.Ingredients[0] != (Ingredient{ID: 9564, Count: 1}) {
		t.Errorf("bomb first ingredient = %v, want numeric 9564 x1", bomb.Ingredients[0])
	}
}

func TestLoadDoughsWinesChurn(t *testing.T) {
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	doughs := tables.Doughs
	if doughs.FlourID != 200 || doughs.PotID != 201 {
		t.Errorf("flour/pot = %d/%d, want 200/201", doughs.FlourID, doughs.PotID)
	}
	if len(doughs.Waters) != 3 || doughs.Waters[0] != (DoughWater{ID: 210, EmptyID: 211}) {
		t.Errorf("waters = %v, want 3 starting [210 211]", doughs.Waters)
	}
	if doughs.Products[3].ProductID != 223 || doughs.Products[3].Level != 58 || !doughs.Products[3].Members {
		t.Errorf("pitta = %+v, want product 223 level 58 members", doughs.Products[3])
	}
	if len(tables.Wines) != 2 || tables.Wines[0].PendingLimit != 60000 || !tables.Wines[0].HasPendingLimit {
		t.Errorf("wine 0 pendingLimit = %d/%v, want 60000/true", tables.Wines[0].PendingLimit, tables.Wines[0].HasPendingLimit)
	}
	if tables.Wines[1].HasPendingLimit {
		t.Error("wine 1 has pending limit, want none (zamorak grapes)")
	}
	if tables.Wines[0].Chance != [2]int{48, 352} || tables.Wines[1].Chance != [2]int{-125, 260} {
		t.Errorf("wine chances = %v / %v, want [48 352] / [-125 260]", tables.Wines[0].Chance, tables.Wines[1].Chance)
	}
	churn := tables.Churn
	if churn.Labels["pot_of_cream"] != "Cream" || churn.LabelsByProductID[244] != "Cream" {
		t.Errorf("churn labels = %v / %v, want Cream for pot_of_cream/244", churn.Labels, churn.LabelsByProductID)
	}
	if len(churn.Inputs) != 2 || !churn.Inputs[1].HasReturn || churn.Inputs[1].ReturnID != 211 {
		t.Errorf("churn milk input = %+v, want returns bucket_empty 211", churn.Inputs[1])
	}
	if churn.Products[1].XP != 23.5 || churn.Products[1].SeqID != 9001 {
		t.Errorf("churn cheese = xp %v seq %d, want 23.5/9001", churn.Products[1].XP, churn.Products[1].SeqID)
	}
}

func TestLoadMissingNameFailsClosed(t *testing.T) {
	bad := `{"cookables": [{"raw":"no_such_item","cooked":"shrimp","burnt":"burnt_shrimp","level":1,"xp":30,"name":"x"}],
		"recipes": [{"product":"pie_shell","level":1,"ingredients":["pastry_dough","piedish"]}],
		"doughs": {"flour": ["pot_flour", "pot_empty"], "waters": [["bucket_water", "bucket_empty"]], "products": []},
		"wines": [], "churn": {"labels": {}, "inputs": [], "products": []}}`
	_, err := Load([]byte(bad), fixtureResolver())
	if err == nil {
		t.Fatal("Load with unknown obj name succeeded, want error")
	}
	if !strings.Contains(err.Error(), "no_such_item") {
		t.Errorf("error = %v, want it to name no_such_item", err)
	}
}

func TestLoadNilResolverAndNilCache(t *testing.T) {
	if _, err := Load([]byte(fixtureJSON), nil); err == nil {
		t.Error("Load(nil resolver) succeeded, want error")
	}
	if _, err := LoadFromCache(nil, ""); err == nil {
		t.Error("LoadFromCache(nil defs) succeeded, want error")
	}
	if _, err := Load([]byte(`{bad json`), fixtureResolver()); err == nil {
		t.Error("Load(malformed JSON) succeeded, want error")
	}
}

func TestLoadKnivesFallback(t *testing.T) {
	// The fixture resolver knows no knife gamevals: KNIFE_2..5
	// still resolve through the knifeIDFallbacks table
	// (transcribed from ItemIdentifiers.ts), while KNIFE —
	// the "knife" gameval — stays absent, and Load still
	// succeeds (Common.Cooking.js knives() filters missing
	// names out).
	tables, err := Load([]byte(fixtureJSON), fixtureResolver())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := []int{947, 5605, 14293, 28413}
	if len(tables.KnifeIDs) != len(want) {
		t.Fatalf("knife ids = %v, want %v", tables.KnifeIDs, want)
	}
	for i, id := range want {
		if tables.KnifeIDs[i] != id {
			t.Errorf("knife ids[%d] = %d, want %d", i, tables.KnifeIDs[i], id)
		}
	}
	if len(tables.VarbitIDs) != 2 {
		t.Errorf("varbit ids = %v, want the 2 load varbits", tables.VarbitIDs)
	}
}
