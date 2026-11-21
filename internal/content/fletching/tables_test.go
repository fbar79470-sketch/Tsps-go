package fletching

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// fixtureResolver is a hand-built IDResolver: every gameval
// name the fixture JSON uses maps to a deterministic fake id,
// with a separate range per kind. No live cache is involved.
type fixtureResolver struct {
	obj    map[string]int
	seq    map[string]int
	varbit map[string]int
}

func (f fixtureResolver) ObjID(name string) (int, bool, error) {
	id, ok := f.obj[name]
	return id, ok, nil
}

func (f fixtureResolver) SeqID(name string) (int, bool, error) {
	id, ok := f.seq[name]
	return id, ok, nil
}

func (f fixtureResolver) VarbitID(name string) (int, bool, error) {
	id, ok := f.varbit[name]
	return id, ok, nil
}

// fixtureJSON is a small fletching.json exercising every
// merge/default branch: group defaults, recipe overrides,
// animation absent/false/named, startTick absent/null/number,
// ticks 0, float XP, xpEach + amountMax, quest, unlock, held,
// a menu override, and an "@alternative" with usedOnly.
const fixtureJSON = `{
  "tools": {
    "knife": ["KNIFE", "FLETCHING_KNIFE"],
    "plainKnife": ["KNIFE"],
    "chisel": ["CHISEL", "JEWELLERS_CHISEL"],
    "hammer": ["HAMMER"]
  },
  "alternatives": {
    "@feathers": ["FEATHER", "RED_FEATHER", {"item": "GRYPHON_FEATHER", "usedOnly": true}]
  },
  "menus": {
    "make": {"title": "What would you like to make?"},
    "string": {"title": "How many would you like to string?", "mode": 10},
    "feather": {"title": "How many sets of 15 do you wish to feather?", "mode": 3}
  },
  "groups": {
    "logs": {"tool": "knife", "ticks": 3, "firstTick": 2, "knifeBonus": true, "menu": "make", "animation": "human_fletching"},
    "stringing": {"ticks": 2, "startTick": null, "menu": "string", "message": "You add a string to the bow."},
    "boltTips": {"tool": "chisel", "ticks": 5, "firstTick": 4, "jewellersTicks": 2, "menu": "make", "sound": "GEM_CUTTING", "startSound": true}
  },
  "diaryTasks": [
    {"diary": "western", "task": "fletch-an-oak-shortbow-in-the-gnome-stronghold", "items": ["OAK_SHORTBOW"], "areas": [[2370, 2500, 3380, 3520]]}
  ],
  "recipes": [
    {"group": "logs", "output": "ARROW_SHAFT", "amount": 15, "level": 1, "xp": 5, "message": "You carefully cut the wood into {amount} arrow shafts.", "materials": [["LOGS", 1]]},
    {"group": "logs", "output": "OAK_SHORTBOW_U_", "level": 20, "xp": 16.5, "message": "You carefully cut the wood into a shortbow.", "materials": [["OAK_LOGS", 1]]},
    {"group": "stringing", "output": "OAK_SHORTBOW", "level": 20, "xp": 16.5, "animation": "stringing_oak_shortbow", "materials": [["OAK_SHORTBOW_U_", 1], ["BOW_STRING", 1]]},
    {"group": "stringing", "output": "SHORTBOW", "level": 5, "xp": 5, "animation": "stringing_shortbow", "materials": [["SHORTBOW_U_", 1], ["BOW_STRING", 1]]},
    {"group": "boltTips", "output": "OPAL_BOLT_TIPS", "amount": 12, "level": 11, "xp": 1.5, "animation": "human_opalcutting", "materials": [["OPAL", 1]]},
    {"group": "logs", "output": "WOLFBONE_ARROWTIPS", "amount": 2, "amountMax": 6, "level": 5, "xp": 10, "xpEach": 2.5, "ticks": 2, "animation": "human_fletching", "quest": ["big_chompy_bird_hunting", "started"], "materials": [["WOLF_BONES", 1]]},
    {"group": "logs", "output": "HEADLESS_ARROW", "amount": 15, "level": 1, "xp": 15, "animation": "human_fletching_add_feather", "menu": "feather", "message": "You attach feathers to {amount} arrow shafts.", "materials": [["ARROW_SHAFT", 15], ["@feathers", 15]]},
    {"group": "logs", "output": "SHORTBOW_U_", "level": 5, "xp": 5, "animation": false, "materials": [["LOGS", 1]]}
  ]
}`

func newFixtureResolver() fixtureResolver {
	objs := []string{"KNIFE", "FLETCHING_KNIFE", "CHISEL", "JEWELLERS_CHISEL", "HAMMER",
		"LOGS", "OAK_LOGS", "ARROW_SHAFT", "OAK_SHORTBOW_U_", "OAK_SHORTBOW",
		"FEATHER", "RED_FEATHER", "GRYPHON_FEATHER", "HEADLESS_ARROW",
		"BOW_STRING", "SHORTBOW_U_", "SHORTBOW", "OPAL", "OPAL_BOLT_TIPS",
		"WOLF_BONES", "WOLFBONE_ARROWTIPS"}
	obj := make(map[string]int, len(objs))
	for i, name := range objs {
		obj[name] = 1000 + i
	}
	seqs := []string{"human_fletching", "stringing_oak_shortbow", "stringing_shortbow",
		"human_opalcutting", "human_fletching_add_feather"}
	seq := make(map[string]int, len(seqs))
	for i, name := range seqs {
		seq[name] = 2000 + i
	}
	return fixtureResolver{
		obj:    obj,
		seq:    seq,
		varbit: map[string]int{"option_fletch_dart_or_bolt_makex": 3000, "bowstring_spool_charges": 3001, "bowstring_spool_size": 3002},
	}
}

func loadFixture(t *testing.T) *Tables {
	t.Helper()
	tables, err := Load([]byte(fixtureJSON), newFixtureResolver())
	if err != nil {
		t.Fatalf("Load(fixture): %v", err)
	}
	return tables
}

func recipeByOutputName(t *testing.T, tables *Tables, output string) *Recipe {
	t.Helper()
	id, ok := tables.ItemID(output)
	if !ok {
		t.Fatalf("fixture has no item %q", output)
	}
	recipes := tables.RecipeByOutput(id)
	if len(recipes) != 1 {
		t.Fatalf("RecipeByOutput(%s) = %d recipes, want 1", output, len(recipes))
	}
	return recipes[0]
}

func TestLoadFixtureCounts(t *testing.T) {
	tables := loadFixture(t)
	if got := len(tables.Recipes); got != 8 {
		t.Errorf("Recipes = %d, want 8", got)
	}
	// Groups and menus live only in the raw file (the loader
	// merges groups into recipes and resolves menus into
	// them); count them from the fixture document itself.
	var raw rawFile
	if err := json.Unmarshal([]byte(fixtureJSON), &raw); err != nil {
		t.Fatalf("parsing fixture JSON: %v", err)
	}
	if got := len(raw.Groups); got != 3 {
		t.Errorf("fixture groups = %d, want 3", got)
	}
	if got := len(raw.Menus); got != 3 {
		t.Errorf("fixture menus = %d, want 3", got)
	}
	if got := len(tables.DiaryTasks); got != 1 {
		t.Errorf("DiaryTasks = %d, want 1", got)
	}
	if got := len(tables.ToolIDs); got != 4 {
		t.Errorf("ToolIDs = %d, want 4", got)
	}
	// Raw tokens: 14 item names, "@feathers" counted once.
	if tables.DistinctItemNames != 14 {
		t.Errorf("DistinctItemNames = %d, want 14", tables.DistinctItemNames)
	}
	// Expanded: "@feathers" is 3 kinds, not 1.
	if tables.DistinctExpandedItemNames != 16 {
		t.Errorf("DistinctExpandedItemNames = %d, want 16", tables.DistinctExpandedItemNames)
	}
	// Resolved adds the 5 tool items, none of which a recipe
	// material names, plus the 2 named ids the loader
	// resolves for the loop (BOW_STRING_SPOOL, CELASTRUS_BARK).
	if tables.ResolvedItemNames != 23 {
		t.Errorf("ResolvedItemNames = %d, want 23", tables.ResolvedItemNames)
	}
}

func TestLoadFixtureDefaultsAndOverrides(t *testing.T) {
	tables := loadFixture(t)
	fix := newFixtureResolver()

	// Logs group defaults flow into the arrow-shaft recipe:
	// knife tool, ticks 3, firstTick 2, knife bonus, make
	// menu, human_fletching animation, startTick 0 (absent).
	shafts := recipeByOutputName(t, tables, "ARROW_SHAFT")
	if shafts.Tool != "knife" || shafts.Ticks != 3 || shafts.FirstTick != 2 || !shafts.KnifeBonus {
		t.Errorf("arrow shafts = tool %q ticks %d firstTick %d knifeBonus %v, want knife/3/2/true",
			shafts.Tool, shafts.Ticks, shafts.FirstTick, shafts.KnifeBonus)
	}
	if knifeIDs := tables.ToolIDs["knife"]; len(knifeIDs) != 2 || knifeIDs[0] != fix.obj["KNIFE"] || knifeIDs[1] != fix.obj["FLETCHING_KNIFE"] {
		t.Errorf("ToolIDs[knife] = %v, want knife + fletching knife", knifeIDs)
	}
	if shafts.Menu.Title != "What would you like to make?" {
		t.Errorf("arrow shafts menu title = %q, want the make title", shafts.Menu.Title)
	}
	if !shafts.HasAnimation || shafts.AnimationID != fix.seq["human_fletching"] {
		t.Errorf("arrow shafts animation = has %v id %d, want human_fletching/%d",
			shafts.HasAnimation, shafts.AnimationID, fix.seq["human_fletching"])
	}
	if shafts.StartTick == nil || *shafts.StartTick != 0 {
		t.Errorf("arrow shafts startTick = %v, want 0 (absent means the choice's own tick)", shafts.StartTick)
	}
	if shafts.Amount != 15 || shafts.AmountMax != 15 || shafts.Level != 1 || shafts.XP != 5 {
		t.Errorf("arrow shafts = amount %d/%d level %d xp %v, want 15/15/1/5",
			shafts.Amount, shafts.AmountMax, shafts.Level, shafts.XP)
	}
	if shafts.MaxSets != creationAllSentinel {
		t.Errorf("arrow shafts maxSets = %d, want the 28 sentinel", shafts.MaxSets)
	}

	// Stringing group: startTick null means no opening, and
	// the message comes from the group.
	oakBow := recipeByOutputName(t, tables, "OAK_SHORTBOW")
	if oakBow.StartTick != nil {
		t.Errorf("oak shortbow startTick = %v, want nil (JSON null)", oakBow.StartTick)
	}
	if oakBow.Message != "You add a string to the bow." {
		t.Errorf("oak shortbow message = %q, want the group message", oakBow.Message)
	}
	if oakBow.Menu.Title != "How many would you like to string?" || oakBow.Menu.Mode == nil || *oakBow.Menu.Mode != 10 {
		t.Errorf("oak shortbow menu = %q mode %v, want string/10", oakBow.Menu.Title, oakBow.Menu.Mode)
	}
	if oakBow.Ticks != 2 {
		t.Errorf("oak shortbow ticks = %d, want the group 2", oakBow.Ticks)
	}

	// Bolt-tips group: jeweller's pace, start sound, gem
	// cutting sound, chisel tool.
	tips := recipeByOutputName(t, tables, "OPAL_BOLT_TIPS")
	if tips.JewellersTicks == nil || *tips.JewellersTicks != 2 || !tips.StartSound {
		t.Errorf("opal bolt tips = jewellersTicks %v startSound %v, want 2/true", tips.JewellersTicks, tips.StartSound)
	}
	if tips.Sound.ID() != 2586 {
		t.Errorf("opal bolt tips sound id = %d, want GEM_CUTTING/2586", tips.Sound.ID())
	}
	if tips.Tool != "chisel" {
		t.Errorf("opal bolt tips tool = %q, want chisel", tips.Tool)
	}

	// animation: false plays none.
	unstrung := recipeByOutputName(t, tables, "SHORTBOW_U_")
	if unstrung.HasAnimation || unstrung.AnimationID != 0 {
		t.Errorf("shortbow (u) animation = has %v id %d, want none (animation: false)", unstrung.HasAnimation, unstrung.AnimationID)
	}

	// The headless-arrow recipe overrides the logs menu with
	// "feather" and expands "@feathers" to 3 kinds, with the
	// gryphon feather used-only.
	headless := recipeByOutputName(t, tables, "HEADLESS_ARROW")
	if headless.Menu.Title != "How many sets of 15 do you wish to feather?" || headless.Menu.Mode == nil || *headless.Menu.Mode != 3 {
		t.Errorf("headless arrows menu = %q mode %v, want feather/3", headless.Menu.Title, headless.Menu.Mode)
	}
	if len(headless.Materials) != 2 {
		t.Fatalf("headless arrows materials = %d, want 2", len(headless.Materials))
	}
	feathers := headless.Materials[1]
	if len(feathers.IDs) != 3 || feathers.Amount != 15 {
		t.Errorf("feathers = %d ids amount %d, want 3/15", len(feathers.IDs), feathers.Amount)
	}
	// RED_FEATHER resolves through the fallback table
	// (10088); FEATHER and the used-only GRYPHON_FEATHER
	// resolve through the resolver — in production their
	// lowercase gamevals exist ("feather", "gryphon_feather"
	// → 31235), so they correctly have no fallback entry.
	if feathers.IDs[0] != fix.obj["FEATHER"] || feathers.IDs[1] != itemIDFallbacks["RED_FEATHER"] || feathers.IDs[2] != fix.obj["GRYPHON_FEATHER"] {
		t.Errorf("feathers ids = %v, want FEATHER + RED_FEATHER + GRYPHON_FEATHER", feathers.IDs)
	}
	if len(feathers.UsedOnlyIDs) != 1 || feathers.UsedOnlyIDs[0] != fix.obj["GRYPHON_FEATHER"] {
		t.Errorf("feathers usedOnly = %v, want the gryphon feather only (%d, via the resolver)",
			feathers.UsedOnlyIDs, fix.obj["GRYPHON_FEATHER"])
	}

	// Random-yield recipe: amountMax, xpEach, quest gate,
	// recipe-level ticks override.
	wolfbone := recipeByOutputName(t, tables, "WOLFBONE_ARROWTIPS")
	if wolfbone.Amount != 2 || wolfbone.AmountMax != 6 {
		t.Errorf("wolfbone arrowtips = amount %d/%d, want 2/6", wolfbone.Amount, wolfbone.AmountMax)
	}
	if !wolfbone.HasXPEach || wolfbone.XPEach != 2.5 {
		t.Errorf("wolfbone arrowtips xpEach = %v/%v, want 2.5/true", wolfbone.XPEach, wolfbone.HasXPEach)
	}
	if wolfbone.Quest == nil || wolfbone.Quest.Key != "big_chompy_bird_hunting" || wolfbone.Quest.State != "started" {
		t.Errorf("wolfbone arrowtips quest = %+v, want big_chompy_bird_hunting/started", wolfbone.Quest)
	}
	if wolfbone.Ticks != 2 {
		t.Errorf("wolfbone arrowtips ticks = %d, want the recipe override 2, not the group 3", wolfbone.Ticks)
	}

	// XP truncation at the Shared loop's call site (shared.go
	// makeBatch: int(xp)): 16.5 grants 16, 1.5 grants 1, 2.5
	// each grants 2.
	if got := int(oakBow.XP); got != 16 {
		t.Errorf("oak shortbow int(XP) = %d, want 16 (16.5 truncated)", got)
	}
	if got := int(tips.XP); got != 1 {
		t.Errorf("opal bolt tips int(XP) = %d, want 1 (1.5 truncated)", got)
	}
	if got := int(wolfbone.XPEach); got != 2 {
		t.Errorf("wolfbone int(XPEach) = %d, want 2 (2.5 truncated)", got)
	}
	if got := int(shafts.XP); got != 5 {
		t.Errorf("arrow shafts int(XP) = %d, want 5", got)
	}
	if shafts.HasXPEach {
		t.Error("arrow shafts should have no xpEach")
	}

	// Spool varbits resolve, and the named tool ids are
	// exposed for the Shared loop's knife saving / chisel pace.
	if tables.SpoolChargesVarbitID != 3001 || tables.SpoolSizeVarbitID != 3002 {
		t.Errorf("spool varbits = %d/%d, want 3001/3002", tables.SpoolChargesVarbitID, tables.SpoolSizeVarbitID)
	}
	if tables.FletchingKnifeID != fix.obj["FLETCHING_KNIFE"] {
		t.Errorf("FletchingKnifeID = %d, want %d", tables.FletchingKnifeID, fix.obj["FLETCHING_KNIFE"])
	}
	if tables.JewellersChiselID != fix.obj["JEWELLERS_CHISEL"] {
		t.Errorf("JewellersChiselID = %d, want %d", tables.JewellersChiselID, fix.obj["JEWELLERS_CHISEL"])
	}
	if id, ok := tables.VarbitID("bowstring_spool_charges"); !ok || id != 3001 {
		t.Errorf("VarbitID(spool charges) = %d/%v, want 3001/true", id, ok)
	}
	if id, ok := tables.SoundID("GEM_CUTTING"); !ok || id != 2586 {
		t.Errorf("SoundID(GEM_CUTTING) = %d/%v, want 2586/true", id, ok)
	}
	if _, ok := tables.ItemID("NO_SUCH_ITEM"); ok {
		t.Error("ItemID(unknown) should be false")
	}
	var nilTables *Tables
	if _, ok := nilTables.ItemID("LOGS"); ok {
		t.Error("nil-table ItemID should be false")
	}
	if nilTables.RecipeByOutput(1) != nil {
		t.Error("nil-table RecipeByOutput should return nil")
	}
	if _, ok := nilTables.SeqID("human_fletching"); ok {
		t.Error("nil-table SeqID should be false")
	}
	if _, ok := nilTables.VarbitID("bowstring_spool_charges"); ok {
		t.Error("nil-table VarbitID should be false")
	}
	if _, ok := nilTables.SoundID("CUTTING"); ok {
		t.Error("nil-table SoundID should be false")
	}
}

func TestLoadFixtureDiaryTask(t *testing.T) {
	tables := loadFixture(t)
	if len(tables.DiaryTasks) != 1 {
		t.Fatalf("DiaryTasks = %d, want 1", len(tables.DiaryTasks))
	}
	task := tables.DiaryTasks[0]
	if task.Diary != "western" || task.Task != "fletch-an-oak-shortbow-in-the-gnome-stronghold" {
		t.Errorf("diary task = %q/%q, want western/fletch-an-oak-shortbow-in-the-gnome-stronghold", task.Diary, task.Task)
	}
	fix := newFixtureResolver()
	if len(task.ItemIDs) != 1 || task.ItemIDs[0] != fix.obj["OAK_SHORTBOW"] {
		t.Errorf("diary task items = %v, want [%d]", task.ItemIDs, fix.obj["OAK_SHORTBOW"])
	}
	if len(task.Areas) != 1 || task.Areas[0] != (DiaryArea{MinX: 2370, MaxX: 2500, MinY: 3380, MaxY: 3520}) {
		t.Errorf("diary task areas = %v, want the Gnome Stronghold rect", task.Areas)
	}
	if !task.Areas[0].Contains(game.Location{X: 2400, Y: 3400, Z: 0}) {
		t.Error("Gnome Stronghold rect should contain (2400, 3400)")
	}
	if task.Areas[0].Contains(game.Location{X: 2369, Y: 3400, Z: 0}) {
		t.Error("Gnome Stronghold rect should not contain x 2369")
	}
}

// realFileResolver builds a deterministic fixture resolver
// from the names the real fletching.json itself uses, so the
// real-file test proves the parse + merge + counts without a
// live cache. Cache resolution itself is proven by the
// load-prove run against the rev-242 cache (see the phase
// outcome notes); this test pins the file's shape.
func realFileResolver(t *testing.T, data []byte) fixtureResolver {
	t.Helper()
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parsing fletching.json: %v", err)
	}
	objs := map[string]bool{}
	seqs := map[string]bool{}
	varbits := map[string]bool{
		SpoolChargesVarbit: true,
		SpoolSizeVarbit:    true,
	}
	for _, items := range raw.Tools {
		for _, name := range items {
			objs[name] = true
		}
	}
	collectSeq := func(rawMsg json.RawMessage) {
		if len(rawMsg) == 0 || string(rawMsg) == "false" {
			return
		}
		var name string
		if err := json.Unmarshal(rawMsg, &name); err != nil {
			t.Fatalf("animation %s: %v", rawMsg, err)
		}
		seqs[name] = true
	}
	for _, entry := range raw.Groups {
		collectSeq(entry.Animation)
		if entry.InstantVarbit != nil {
			varbits[*entry.InstantVarbit] = true
		}
	}
	for _, entry := range raw.Recipes {
		objs[entry.Output] = true
		for _, mat := range entry.Materials {
			if strings.HasPrefix(mat.Name, "@") {
				for _, alt := range raw.Alternatives[mat.Name] {
					objs[alt.Name] = true
				}
			} else {
				objs[mat.Name] = true
			}
		}
		collectSeq(entry.Animation)
		if entry.InstantVarbit != nil {
			varbits[*entry.InstantVarbit] = true
		}
	}
	for _, task := range raw.DiaryTasks {
		for _, name := range task.Items {
			objs[name] = true
		}
	}
	assign := func(names map[string]bool, base int) map[string]int {
		keys := make([]string, 0, len(names))
		for name := range names {
			keys = append(keys, name)
		}
		sort.Strings(keys)
		out := make(map[string]int, len(keys))
		for i, name := range keys {
			out[name] = base + i
		}
		return out
	}
	return fixtureResolver{
		obj:    assign(objs, 1000),
		seq:    assign(seqs, 2000),
		varbit: assign(varbits, 3000),
	}
}

func TestLoadRealFile(t *testing.T) {
	data, err := os.ReadFile("../../../data/definitions/fletching.json")
	if err != nil {
		t.Fatalf("reading fletching.json: %v", err)
	}
	tables, err := Load(data, realFileResolver(t, data))
	if err != nil {
		t.Fatalf("Load(real file): %v", err)
	}
	if got := len(tables.Recipes); got != 170 {
		t.Errorf("Recipes = %d, want 170", got)
	}
	// Groups and menus live only in the raw file; count them
	// from the document (see TestLoadFixtureCounts).
	var rawDoc rawFile
	if err := json.Unmarshal(data, &rawDoc); err != nil {
		t.Fatalf("parsing fletching.json: %v", err)
	}
	if got := len(rawDoc.Groups); got != 13 {
		t.Errorf("Groups = %d, want 13", got)
	}
	if got := len(rawDoc.Menus); got != 8 {
		t.Errorf("Menus = %d, want 8", got)
	}
	if got := len(tables.DiaryTasks); got != 4 {
		t.Errorf("DiaryTasks = %d, want 4", got)
	}
	if tables.DistinctItemNames != 265 {
		t.Errorf("DistinctItemNames = %d, want 265", tables.DistinctItemNames)
	}
	if tables.DistinctExpandedItemNames != 272 {
		t.Errorf("DistinctExpandedItemNames = %d, want 272", tables.DistinctExpandedItemNames)
	}
	if tables.ResolvedItemNames != 278 {
		t.Errorf("ResolvedItemNames = %d, want 278 (277 + BOW_STRING_SPOOL)", tables.ResolvedItemNames)
	}
	if tables.FallbackItemNames != 176 {
		t.Errorf("FallbackItemNames = %d, want 176", tables.FallbackItemNames)
	}
	if tables.GamevalItemNames != 102 {
		t.Errorf("GamevalItemNames = %d, want 102", tables.GamevalItemNames)
	}
	if got := len(tables.SeqIDs); got != 68 {
		t.Errorf("SeqIDs = %d, want 68", got)
	}
	if got := len(tables.SoundIDs); got != 3 {
		t.Errorf("SoundIDs = %d, want 3 (CUTTING, CROSSBOW_LIMBS, GEM_CUTTING)", got)
	}
	if got := len(tables.VarbitIDs); got != 3 {
		t.Errorf("VarbitIDs = %d, want 3 (instant + 2 spool)", got)
	}
	// Spot values from the real file.
	shafts := tables.RecipeByOutput(mustItemID(t, tables, "ARROW_SHAFT"))
	if len(shafts) != 7 {
		t.Errorf("arrow-shaft recipes = %d, want 7 (one per log)", len(shafts))
	}
	oakShafts := shafts[1]
	if oakShafts.Amount != 30 || oakShafts.Level != 15 || oakShafts.XP != 10 {
		t.Errorf("oak arrow shafts = amount %d level %d xp %v, want 30/15/10", oakShafts.Amount, oakShafts.Level, oakShafts.XP)
	}
	willowBow := recipeByOutputName(t, tables, "WILLOW_SHORTBOW_U_")
	if willowBow.XP != 33.3 || int(willowBow.XP) != 33 {
		t.Errorf("willow shortbow (u) xp = %v/%d, want 33.3/33", willowBow.XP, int(willowBow.XP))
	}
	runeArrow := recipeByOutputName(t, tables, "RUNE_ARROW")
	if runeArrow.XP != 187.5 || int(runeArrow.XP) != 187 {
		t.Errorf("rune arrow xp = %v/%d, want 187.5/187", runeArrow.XP, int(runeArrow.XP))
	}
	ogreShafts := recipeByOutputName(t, tables, "OGRE_ARROW_SHAFT")
	if ogreShafts.AmountMax != 6 || !ogreShafts.HasXPEach || ogreShafts.XPEach != 1.6 || int(ogreShafts.XPEach) != 1 {
		t.Errorf("ogre arrow shafts = amountMax %d xpEach %v/%d, want 6/1.6/1",
			ogreShafts.AmountMax, ogreShafts.XPEach, int(ogreShafts.XPEach))
	}
	if ogreShafts.Quest == nil || ogreShafts.Quest.Key != "big_chompy_bird_hunting" {
		t.Errorf("ogre arrow shafts quest = %+v, want big_chompy_bird_hunting", ogreShafts.Quest)
	}
	blowpipe := recipeByOutputName(t, tables, "TOXIC_BLOWPIPE_EMPTY_")
	if blowpipe.Ticks != 0 {
		t.Errorf("toxic blowpipe ticks = %d, want 0 (instant, no task)", blowpipe.Ticks)
	}
	grapple2 := recipeByOutputName(t, tables, "MITH_GRAPPLE_2")
	if grapple2.HasAnimation || grapple2.AnimationID != 0 {
		t.Errorf("mithril grapple (2) animation = has %v id %d, want none (animation: false)", grapple2.HasAnimation, grapple2.AnimationID)
	}
	if grapple2.StartTick != nil {
		t.Errorf("mithril grapple (2) startTick = %v, want nil (JSON null)", grapple2.StartTick)
	}
	shield := recipeByOutputName(t, tables, "OAK_SHIELD")
	if shield.StartTick == nil || *shield.StartTick != 1 {
		t.Errorf("oak shield startTick = %v, want 1 (shields group)", shield.StartTick)
	}
	if shield.Ticks != 7 || shield.FirstTick != 6 {
		t.Errorf("oak shield ticks = %d firstTick %d, want 7/6", shield.Ticks, shield.FirstTick)
	}
	darts := recipeByOutputName(t, tables, "DRAGON_DART")
	if !darts.HasInstantVarbit || darts.InstantVarbitID == 0 {
		t.Errorf("dragon darts instant varbit = has %v id %d, want true/non-zero", darts.HasInstantVarbit, darts.InstantVarbitID)
	}
	if id, ok := tables.VarbitID(InstantVarbit); !ok || id != darts.InstantVarbitID {
		t.Errorf("VarbitID(%s) = %d/%v, want the darts varbit %d", InstantVarbit, id, ok, darts.InstantVarbitID)
	}
	boltTip := recipeByOutputName(t, tables, "OPAL_BOLT_TIPS")
	if boltTip.JewellersTicks == nil || *boltTip.JewellersTicks != 2 || !boltTip.StartSound {
		t.Errorf("opal bolt tips = jewellersTicks %v startSound %v, want 2/true (boltTips group)", boltTip.JewellersTicks, boltTip.StartSound)
	}
	pearlTips := tables.RecipeByOutput(mustItemID(t, tables, "PEARL_BOLT_TIPS"))
	if len(pearlTips) != 2 {
		t.Errorf("pearl bolt-tip recipes = %d, want 2 (pearl + pearls)", len(pearlTips))
	}
	broadArrows := recipeByOutputName(t, tables, "BROAD_ARROWS")
	if broadArrows.Unlock != "Broader Fletching" {
		t.Errorf("broad arrows unlock = %q, want Broader Fletching", broadArrows.Unlock)
	}
	// Diary tasks: keys, rects and items as the JSON has them.
	wantDiary := []struct {
		diary, task string
		items       []string
		areas       int
	}{
		{"western", "fletch-an-oak-shortbow-in-the-gnome-stronghold", []string{"OAK_SHORTBOW"}, 1},
		{"western", "fletch-a-magic-longbow-in-tirannwn", []string{"MAGIC_LONGBOW"}, 2},
		{"desert", "fletch-some-dragon-darts-at-the-bedabin-camp", []string{"DRAGON_DART"}, 1},
		{"kandarin", "string-a-maple-shortbow-in-seers-village-bank", []string{"MAPLE_SHORTBOW"}, 1},
	}
	for i, want := range wantDiary {
		got := tables.DiaryTasks[i]
		if got.Diary != want.diary || got.Task != want.task {
			t.Errorf("diary task %d = %q/%q, want %q/%q", i, got.Diary, got.Task, want.diary, want.task)
		}
		if len(got.ItemNames) != len(want.items) || got.ItemNames[0] != want.items[0] {
			t.Errorf("diary task %d items = %v, want %v", i, got.ItemNames, want.items)
		}
		if len(got.Areas) != want.areas {
			t.Errorf("diary task %d areas = %d, want %d", i, len(got.Areas), want.areas)
		}
	}
}

func mustItemID(t *testing.T, tables *Tables, name string) int {
	t.Helper()
	id, ok := tables.ItemID(name)
	if !ok {
		t.Fatalf("no item %q", name)
	}
	return id
}

func TestLoadFallbackTable(t *testing.T) {
	// The fixture names that have no (correct) gameval must
	// still resolve when the resolver knows nothing about
	// them: the fallback table answers instead.
	// (GRYPHON_FEATHER is not one of them: its lowercase
	// gameval "gryphon_feather" resolves in the real cache,
	// so it has no fallback entry — see the probe.)
	fix := newFixtureResolver()
	fallbackNames := []string{"SHORTBOW_U_", "OAK_SHORTBOW_U_", "OPAL_BOLT_TIPS", "WOLFBONE_ARROWTIPS"}
	for _, name := range fallbackNames {
		delete(fix.obj, name)
	}
	tables, err := Load([]byte(fixtureJSON), fix)
	if err != nil {
		t.Fatalf("Load without fallback names in the resolver: %v", err)
	}
	for _, name := range fallbackNames {
		got, ok := tables.ItemID(name)
		if !ok || got != itemIDFallbacks[name] {
			t.Errorf("ItemID(%s) = %d/%v, want fallback id %d", name, got, ok, itemIDFallbacks[name])
		}
	}
	// A name in neither the table nor the cache still fails.
	delete(fix.obj, "OAK_LOGS")
	if _, err := Load([]byte(fixtureJSON), fix); err == nil {
		t.Fatal("Load with OAK_LOGS unknown should fail")
	} else if !strings.Contains(err.Error(), `"OAK_LOGS"`) {
		t.Errorf("Load error = %v, want OAK_LOGS listed", err)
	}
	// The 3 wrong-gameval names resolve to the
	// ItemIdentifiers ids, never to the enchanted bolts the
	// lowercase gamevals name.
	for _, name := range []string{"OPAL_BOLTS", "PEARL_BOLTS", "BARBED_BOLTS"} {
		if got := itemIDFallbacks[name]; got == 3818 || got == 3822 || got == 3826 {
			t.Errorf("fallback %s = %d, the enchanted-bolt id", name, got)
		}
	}
	if itemIDFallbacks["OPAL_BOLTS"] != 879 || itemIDFallbacks["PEARL_BOLTS"] != 880 || itemIDFallbacks["BARBED_BOLTS"] != 881 {
		t.Errorf("wrong-gameval fallbacks = %d/%d/%d, want 879/880/881",
			itemIDFallbacks["OPAL_BOLTS"], itemIDFallbacks["PEARL_BOLTS"], itemIDFallbacks["BARBED_BOLTS"])
	}
	if got := len(itemIDFallbacks); got != 176 {
		t.Errorf("itemIDFallbacks = %d entries, want 176", got)
	}
}

func TestLoadUnresolvedNames(t *testing.T) {
	// One missing name per kind: all four must be listed.
	fix := newFixtureResolver()
	delete(fix.obj, "OAK_LOGS")
	delete(fix.seq, "stringing_oak_shortbow")
	delete(fix.varbit, "bowstring_spool_charges")
	if _, err := Load([]byte(fixtureJSON), fix); err == nil {
		t.Fatal("Load with missing names should fail")
	} else {
		for _, want := range []string{`"OAK_LOGS"`, `"stringing_oak_shortbow"`, `"bowstring_spool_charges"`} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("Load error = %v, want %s listed", err, want)
			}
		}
	}
	// Sounds resolve through the game sound-name table, so an
	// unknown sound name fails the same way.
	badSound := strings.Replace(fixtureJSON, `"GEM_CUTTING"`, `"NO_SUCH_SOUND"`, 1)
	if _, err := Load([]byte(badSound), newFixtureResolver()); err == nil {
		t.Fatal("Load with an unknown sound should fail")
	} else if !strings.Contains(err.Error(), `"NO_SUCH_SOUND"`) {
		t.Errorf("Load error = %v, want NO_SUCH_SOUND listed", err)
	}
}

func TestLoadBadInputs(t *testing.T) {
	if _, err := Load(nil, newFixtureResolver()); err == nil {
		t.Error("Load(nil data) should fail")
	}
	if _, err := Load([]byte(`{broken`), newFixtureResolver()); err == nil {
		t.Error("Load(malformed JSON) should fail")
	}
	if _, err := Load([]byte(`{"recipes": []}`), newFixtureResolver()); err == nil {
		t.Error("Load(no recipes) should fail")
	}
	if _, err := Load([]byte(fixtureJSON), nil); err == nil {
		t.Error("Load(nil resolver) should fail")
	}
	if _, err := LoadFromCache(nil, ""); err == nil {
		t.Error("LoadFromCache(nil cache) should fail")
	}
	if _, err := LoadFile("no-such-file.json", newFixtureResolver()); err == nil {
		t.Error("LoadFile(missing file) should fail")
	}
	// Unknown group, menu, tool and alternative each fail.
	unknownGroup := strings.Replace(fixtureJSON, `"group": "logs", "output": "ARROW_SHAFT"`, `"group": "nope", "output": "ARROW_SHAFT"`, 1)
	if _, err := Load([]byte(unknownGroup), newFixtureResolver()); err == nil {
		t.Error("Load(unknown group) should fail")
	}
	unknownMenu := strings.Replace(fixtureJSON, `"menu": "feather"`, `"menu": "nope"`, 1)
	if _, err := Load([]byte(unknownMenu), newFixtureResolver()); err == nil {
		t.Error("Load(unknown menu) should fail")
	}
	unknownTool := strings.Replace(fixtureJSON, `"tool": "knife"`, `"tool": "nope"`, 1)
	if _, err := Load([]byte(unknownTool), newFixtureResolver()); err == nil {
		t.Error("Load(unknown tool) should fail")
	}
	unknownAlt := strings.Replace(fixtureJSON, `"@feathers"`, `"@nope"`, 1)
	if _, err := Load([]byte(unknownAlt), newFixtureResolver()); err == nil {
		t.Error("Load(unknown alternative) should fail")
	}
	badQuest := strings.Replace(fixtureJSON, `"started"`, `"whenever"`, 1)
	if _, err := Load([]byte(badQuest), newFixtureResolver()); err == nil {
		t.Error("Load(unknown quest state) should fail")
	}
}
