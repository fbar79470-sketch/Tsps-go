package woodcutting

import (
	"sort"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// fixtureResolver is a hand-built IDResolver: every gameval name
// the source tables use maps to a deterministic fake id, with a
// separate range per kind. No live cache is involved.
type fixtureResolver struct {
	obj  map[string]int
	loc  map[string]int
	seq  map[string]int
	varp map[string]int
}

func (f fixtureResolver) ObjID(name string) (int, bool, error) {
	id, ok := f.obj[name]
	return id, ok, nil
}

func (f fixtureResolver) LocID(name string) (int, bool, error) {
	id, ok := f.loc[name]
	return id, ok, nil
}

func (f fixtureResolver) SeqID(name string) (int, bool, error) {
	id, ok := f.seq[name]
	return id, ok, nil
}

func (f fixtureResolver) VarpID(name string) (int, bool, error) {
	id, ok := f.varp[name]
	return id, ok, nil
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// newFixtureResolver collects every gameval name the source
// tables reference and assigns deterministic fake ids.
func newFixtureResolver() fixtureResolver {
	objs := map[string]bool{}
	locs := map[string]bool{}
	for _, def := range AxeDefs {
		objs[def.Item] = true
	}
	for _, def := range TreeDefs {
		objs[def.LogItem] = true
		for _, name := range def.ObjectIDs {
			locs[name] = true
		}
		if def.Stump != "" {
			locs[def.Stump] = true
		}
	}
	for _, name := range []string{BirdNestDefs.RedEggNest, BirdNestDefs.GreenEggNest,
		BirdNestDefs.BlueEggNest, BirdNestDefs.SeedNest, BirdNestDefs.LegacySeedNest,
		BirdNestDefs.RingNest, BirdNestDefs.EmptyNest} {
		objs[name] = true
	}
	for _, def := range NestSeedDefs {
		objs[def.Item] = true
	}
	for _, def := range NestRingDefs {
		objs[def.Item] = true
	}
	for _, def := range LumberjackOutfitDefs {
		for _, name := range def.Items {
			objs[name] = true
		}
	}
	for _, name := range NormalTreeStumpByModelDef {
		locs[name] = true
	}
	locs[DefaultTreeStump] = true
	// The ThickVines rake (Tables.RakeID).
	objs["rake"] = true
	locs["pest_tree"] = true
	locs["pest_tree2"] = true
	assign := func(names map[string]bool, base int) map[string]int {
		out := make(map[string]int, len(names))
		for i, name := range sortedKeys(names) {
			out[name] = base + i
		}
		return out
	}
	return fixtureResolver{
		obj:  assign(objs, 1000),
		loc:  assign(locs, 2000),
		seq:  map[string]int{"tbw_cleanup_player_surprise_stepback": 3000},
		varp: map[string]int{LogsChoppedVarp: 4000},
	}
}

func axeDef(item string) AxeDef {
	for _, def := range AxeDefs {
		if def.Item == item {
			return def
		}
	}
	return AxeDef{}
}

func treeDef(name string) TreeDef {
	for _, def := range TreeDefs {
		if def.Name == name {
			return def
		}
	}
	return TreeDef{}
}

// The plugin lists 30 axes, not 31: the Phase 32 plan §1 says
// 31, which is wrong — there are 30 axe(...) calls at
// Woodcutting.plugin.js:193-227. The table is verbatim.
func TestSourceTableCounts(t *testing.T) {
	if got := len(AxeDefs); got != 30 {
		t.Errorf("AxeDefs = %d, want 30", got)
	}
	if got := len(TreeDefs); got != 21 {
		t.Errorf("TreeDefs = %d, want 21", got)
	}
	if got := len(CutChance); got != 14 {
		t.Errorf("CutChance charts = %d, want 14", got)
	}
	if got := len(NestSeedDefs); got != 19 {
		t.Errorf("NestSeedDefs = %d, want 19", got)
	}
	if got := len(NestRingDefs); got != 5 {
		t.Errorf("NestRingDefs = %d, want 5", got)
	}
	if got := len(LumberjackOutfitDefs); got != 4 {
		t.Errorf("LumberjackOutfitDefs = %d, want 4", got)
	}
	if got := len(NormalTreeStumpByModelDef); got != 24 {
		t.Errorf("NormalTreeStumpByModelDef = %d, want 24", got)
	}
}

func TestSourceSpotValues(t *testing.T) {
	bronze := axeDef("bronze_axe")
	if bronze.Tier != 0 || bronze.AnimationID != 879 || bronze.RequiredLevel != 1 || !bronze.Standard {
		t.Errorf("bronze axe def = %+v, want tier 0 anim 879 level 1 standard", bronze)
	}
	if got := AxeSpeedByTier[bronze.Tier]; got != 0.03 {
		t.Errorf("bronze axe speed = %v, want 0.03", got)
	}
	dragon := axeDef("dragon_axe")
	if dragon.Tier != 7 || dragon.AnimationID != 2846 || !dragon.Standard {
		t.Errorf("dragon axe def = %+v, want tier 7 anim 2846 standard", dragon)
	}
	echo := axeDef("league_trailblazer_axe")
	if echo.Tier != 7 || echo.RequiredLevel != 1 {
		t.Errorf("echo axe def = %+v, want tier 7 level 1", echo)
	}
	infernal := axeDef("infernal_axe")
	if infernal.Speed != 0.3 {
		t.Errorf("charged infernal axe speed = %v, want 0.3", infernal.Speed)
	}
	crystal := axeDef("crystal_axe")
	if crystal.RequiredAgility != 50 || crystal.Tier != 7 {
		t.Errorf("crystal axe def = %+v, want agility 50 tier 7", crystal)
	}

	normal := treeDef("normal tree")
	if normal.RequiredLevel != 1 || normal.XPReward != 25 {
		t.Errorf("normal tree = level %d xp %v, want 1 / 25", normal.RequiredLevel, normal.XPReward)
	}
	if len(normal.ObjectIDs) != 35 {
		t.Errorf("normal tree object ids = %d, want 35", len(normal.ObjectIDs))
	}
	if normal.RespawnTicks != 59 || normal.RespawnTicksMax != 98 {
		t.Errorf("normal tree respawn = %d..%d, want 59..98", normal.RespawnTicks, normal.RespawnTicksMax)
	}
	if normal.BirdNests == nil || *normal.BirdNests {
		t.Error("normal tree should have birdNests false")
	}
	yew := treeDef("yew")
	if yew.RequiredLevel != 60 || yew.XPReward != 175 {
		t.Errorf("yew = level %d xp %v, want 60 / 175", yew.RequiredLevel, yew.XPReward)
	}
	magic := treeDef("magic")
	if magic.RequiredLevel != 75 || magic.XPReward != 250 {
		t.Errorf("magic = level %d xp %v, want 75 / 250", magic.RequiredLevel, magic.XPReward)
	}
	redwood := treeDef("redwood")
	if len(redwood.ObjectIDs) != 121 {
		t.Errorf("redwood object ids = %d, want 121", len(redwood.ObjectIDs))
	}
	if redwood.RequiredLevel != 90 || redwood.XPReward != 380 {
		t.Errorf("redwood = level %d xp %v, want 90 / 380", redwood.RequiredLevel, redwood.XPReward)
	}
	// Name-only trees carry no object ids.
	for _, name := range []string{"hollow", "arctic pine", "mature juniper", "jatoba",
		"blisterwood", "burnt tree", "camphor", "ironwood", "rosewood"} {
		if got := len(treeDef(name).ObjectIDs); got != 0 {
			t.Errorf("%s object ids = %d, want 0 (name-only tree)", name, got)
		}
		if len(treeDef(name).ObjectNames) == 0 {
			t.Errorf("%s has no object names", name)
		}
	}
	// Blisterwood lists only bronze and dragon rows.
	blister := CutChance["blisterwood"]
	if !blister[0].Has || !blister[7].Has {
		t.Error("blisterwood should have bronze and dragon rows")
	}
	for tier := 1; tier <= 6; tier++ {
		if blister[tier].Has {
			t.Errorf("blisterwood tier %d should be a null row", tier)
		}
	}
	// The Wiki lists camphor black as 17/71, above adamant; kept
	// as published.
	if got := CutChance["camphor"][3]; got != (CutRange{Low: 17, High: 71, Has: true}) {
		t.Errorf("camphor black row = %+v, want {17 71}", got)
	}
	// Scrapey is the only inline chart, with the rounded formula.
	scrapey := treeDef("scrapey tree")
	if scrapey.CutChance != "" || !scrapey.RoundedCutChance {
		t.Errorf("scrapey chart = %q rounded %v, want inline + rounded", scrapey.CutChance, scrapey.RoundedCutChance)
	}
	if got := scrapey.CutChanceChart[0]; got != (CutRange{Low: 23, High: 75, Has: true}) {
		t.Errorf("scrapey bronze row = %+v, want {23 75}", got)
	}
	// Nest weight totals: 1,011 seed slots, 100 ring slots.
	seedTotal := 0
	for _, def := range NestSeedDefs {
		seedTotal += def.Weight
	}
	if seedTotal != 1011 {
		t.Errorf("nest seed weights sum = %d, want 1011", seedTotal)
	}
	ringTotal := 0
	for _, def := range NestRingDefs {
		ringTotal += def.Weight
	}
	if ringTotal != 100 {
		t.Errorf("nest ring weights sum = %d, want 100", ringTotal)
	}
	// Lumberjack bonuses: hat .004, top .008, legs .006, boots .002.
	wantBonus := map[int]float64{
		game.HeadSlot: 0.004, game.BodySlot: 0.008,
		game.LegSlot: 0.006, game.FeetSlot: 0.002,
	}
	for _, def := range LumberjackOutfitDefs {
		if wantBonus[def.Slot] != def.Bonus {
			t.Errorf("lumberjack slot %d bonus = %v, want %v", def.Slot, def.Bonus, wantBonus[def.Slot])
		}
		if len(def.Items) != 2 {
			t.Errorf("lumberjack slot %d items = %d, want 2 (Lumberjack + Forestry)", def.Slot, len(def.Items))
		}
	}
}

func TestLoadFixture(t *testing.T) {
	fix := newFixtureResolver()
	tables, err := Load(fix)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(tables.Axes); got != 30 {
		t.Errorf("Axes = %d, want 30", got)
	}
	if got := len(tables.Trees); got != 21 {
		t.Errorf("Trees = %d, want 21", got)
	}
	// Worst-to-best order is preserved; best-first is its reverse.
	if tables.Axes[0].Item != "bronze_axe_2h" || tables.Axes[1].Item != "bronze_axe" {
		t.Errorf("Axes[0..1] = %q %q, want bronze felling then bronze", tables.Axes[0].Item, tables.Axes[1].Item)
	}
	if tables.AxesBestFirst[0].Item != "league_trailblazer_axe" {
		t.Errorf("AxesBestFirst[0] = %q, want the echo axe", tables.AxesBestFirst[0].Item)
	}
	if tables.AxesBestFirst[len(tables.AxesBestFirst)-1].Item != "bronze_axe_2h" {
		t.Errorf("AxesBestFirst last = %q, want bronze felling", tables.AxesBestFirst[len(tables.AxesBestFirst)-1].Item)
	}
	// Spot values resolve through the fixture ids.
	bronze := tables.AxeByID(fix.obj["bronze_axe"])
	if bronze == nil || bronze.Tier != 0 || bronze.AnimationID != 879 || bronze.Speed != 0.03 {
		t.Errorf("resolved bronze axe = %+v, want tier 0 anim 879 speed 0.03", bronze)
	}
	if got := tables.AxeByID(fix.obj["infernal_axe"]); got == nil || got.Speed != 0.3 {
		t.Errorf("resolved infernal axe speed = %v, want 0.3", got)
	}
	if got := tables.AxeByID(999999); got != nil {
		t.Errorf("AxeByID(unknown) = %+v, want nil", got)
	}
	normal := tables.TreeByName("Tree")
	if normal == nil || normal.Name != "normal tree" || normal.RequiredLevel != 1 || normal.XPReward != 25 {
		t.Errorf("TreeByName(Tree) = %+v, want the normal tree", normal)
	}
	if got := normal.LogID; got != fix.obj["logs"] {
		t.Errorf("normal tree log id = %d, want %d", got, fix.obj["logs"])
	}
	if len(normal.ObjectIDs) != 35 || normal.ObjectIDs[0] != fix.loc["evergreen_vsnowy_large"] {
		t.Errorf("normal tree object ids = %d starting %d", len(normal.ObjectIDs), normal.ObjectIDs[0])
	}
	yew := tables.TreeByName("Yew tree")
	if yew == nil || yew.RequiredLevel != 60 || yew.XPReward != 175 {
		t.Errorf("yew = %+v, want level 60 xp 175", yew)
	}
	magicTree := tables.TreeByName("Magic tree")
	if magicTree == nil || magicTree.RequiredLevel != 75 || magicTree.XPReward != 250 {
		t.Errorf("magic = %+v, want level 75 xp 250", magicTree)
	}
	redwood := tables.TreeByName("Redwood tree")
	if redwood == nil || len(redwood.ObjectIDs) != 121 {
		t.Errorf("redwood object ids = %d, want 121", len(redwood.ObjectIDs))
	}
	// Name-only trees resolve by name, never by id.
	hollow := tables.TreeByName("Hollow tree")
	if hollow == nil || hollow.Name != "hollow" || len(hollow.ObjectIDs) != 0 {
		t.Errorf("TreeByName(Hollow tree) = %+v, want the name-only hollow tree", hollow)
	}
	if tables.TreeByName("No such tree") != nil {
		t.Error("TreeByName(unknown) should be nil")
	}
	// Defaults: depletes and bird nests are true unless TS says false.
	if tables.TreeByName("Dramen tree").Depletes {
		t.Error("dramen should never deplete")
	}
	if !tables.TreeByName("Oak tree").Depletes || !tables.TreeByName("Oak tree").BirdNests {
		t.Error("oak should deplete and drop bird nests")
	}
	if tables.TreeByName("Tree").BirdNests {
		t.Error("normal tree should not drop bird nests")
	}
	if tables.TreeByName("Maple tree").DiaryXP == nil || tables.TreeByName("Maple tree").DiaryXP.Percent != 10 {
		t.Errorf("maple diary bonus = %+v, want kandarin medium 10%%", tables.TreeByName("Maple tree").DiaryXP)
	}
	blister := tables.TreeByName("Blisterwood tree")
	if blister == nil || blister.InterruptAnimationID != fix.seq["tbw_cleanup_player_surprise_stepback"] ||
		blister.InterruptSound != 3604 {
		t.Errorf("blisterwood interrupt = %+v, want seq %d sound 3604", blister, fix.seq["tbw_cleanup_player_surprise_stepback"])
	}
	// Pest-control clone: the ONLY tree by object id.
	pest := tables.PestControlTree
	if pest == nil || pest.Name != "pest control tree" || pest.LogLine || pest.CountsLogs || pest.RespawnTimer {
		t.Errorf("pest control tree = %+v, want silent clone", pest)
	}
	if got := tables.TreeByObjectID(fix.loc["pest_tree"]); got != pest {
		t.Errorf("TreeByObjectID(pest_tree) = %+v, want the pest clone", got)
	}
	if got := tables.TreeByObjectID(fix.loc["pest_tree2"]); got != pest {
		t.Errorf("TreeByObjectID(pest_tree2) = %+v, want the pest clone", got)
	}
	if got := tables.TreeByObjectID(fix.loc["tree"]); got != nil {
		t.Errorf("TreeByObjectID(tree) = %+v, want nil (only pest control is by id)", got)
	}
	if got := len(tables.TreesByObjectID); got != 2 {
		t.Errorf("TreesByObjectID = %d entries, want 2", got)
	}
	// Farmed trees: hardwoods regrow in 150 ticks, then redwood.
	if got := len(tables.FarmedTrees); got != 4 {
		t.Fatalf("FarmedTrees = %d, want 4", got)
	}
	for i, name := range []string{"camphor", "ironwood", "rosewood"} {
		if tables.FarmedTrees[i].Name != name || tables.FarmedTrees[i].RespawnTicks != 150 {
			t.Errorf("FarmedTrees[%d] = %s respawn %d, want %s respawn 150",
				i, tables.FarmedTrees[i].Name, tables.FarmedTrees[i].RespawnTicks, name)
		}
	}
	farmedRedwood := tables.FarmedTrees[3]
	if farmedRedwood.Name != "redwood" || farmedRedwood.DespawnTicks != 0 ||
		farmedRedwood.DepleteChance != 11 || farmedRedwood.RespawnTicks != 200 {
		t.Errorf("farmed redwood = %+v, want despawn 0 deplete 11 respawn 200", farmedRedwood)
	}
	// Tree log ids: the 11 bot-tracked trees, in TREES order.
	if got := len(tables.TreeLogIDs); got != 11 {
		t.Errorf("TreeLogIDs = %d, want 11", got)
	}
	if len(tables.TreeLogIDs) > 0 && tables.TreeLogIDs[0] != fix.obj["logs"] {
		// Scrapey has no object ids, so the first bot-tracked log
		// is the normal tree's logs.
		t.Errorf("TreeLogIDs[0] = %d, want logs %d", tables.TreeLogIDs[0], fix.obj["logs"])
	}
	// Stump map and default stump.
	if got := tables.NormalTreeStumpByModel[1570]; got != fix.loc["treestump2"] {
		t.Errorf("stump for model 1570 = %d, want %d", got, fix.loc["treestump2"])
	}
	if got := tables.NormalTreeStumpByModel[5528]; got != fix.loc["willow_tree2or3or4_stump"] {
		t.Errorf("stump for model 5528 = %d, want willow stump %d", got, fix.loc["willow_tree2or3or4_stump"])
	}
	if tables.DefaultTreeStumpID != fix.loc["treestump2"] {
		t.Errorf("DefaultTreeStumpID = %d, want %d", tables.DefaultTreeStumpID, fix.loc["treestump2"])
	}
	// Nests.
	if tables.BirdNests.SeedNest != fix.obj["bird_nest_seeds_jan2019"] {
		t.Errorf("seed nest = %d, want %d", tables.BirdNests.SeedNest, fix.obj["bird_nest_seeds_jan2019"])
	}
	if len(tables.NestSeeds) != 19 || len(tables.NestRings) != 5 {
		t.Errorf("nests = %d seeds %d rings, want 19/5", len(tables.NestSeeds), len(tables.NestRings))
	}
	if tables.NestSeeds[0].Text != "an acorn" || tables.NestSeeds[0].Weight != 214 {
		t.Errorf("first nest seed = %+v, want acorn 214", tables.NestSeeds[0])
	}
	if len(tables.SeedNestIDs) != 2 || len(tables.SearchableNestIDs) != 6 {
		t.Errorf("nest id sets = %d seed %d searchable, want 2/6", len(tables.SeedNestIDs), len(tables.SearchableNestIDs))
	}
	if !tables.SearchableNestIDs[tables.BirdNests.RingNest] || tables.SearchableNestIDs[tables.BirdNests.EmptyNest] {
		t.Error("ring nest is searchable; the empty nest is not")
	}
	// Lumberjack outfit and the logs-chopped varp.
	if len(tables.LumberjackOutfit) != 4 {
		t.Fatalf("LumberjackOutfit = %d, want 4", len(tables.LumberjackOutfit))
	}
	hat := tables.LumberjackOutfit[0]
	if hat.Slot != game.HeadSlot || hat.Bonus != 0.004 || len(hat.ItemIDs) != 2 ||
		hat.ItemIDs[0] != fix.obj["ramble_lumberjack_hat"] {
		t.Errorf("lumberjack hat = %+v, want head slot bonus 0.004", hat)
	}
	if tables.LogsChoppedVarpID != fix.varp[LogsChoppedVarp] {
		t.Errorf("LogsChoppedVarpID = %d, want %d", tables.LogsChoppedVarpID, fix.varp[LogsChoppedVarp])
	}
}

func TestLoadUnresolvedName(t *testing.T) {
	fix := newFixtureResolver()
	delete(fix.obj, "bronze_axe")
	delete(fix.loc, "pest_tree")
	if _, err := Load(fix); err == nil {
		t.Fatal("Load with missing names should fail")
	} else if !strings.Contains(err.Error(), `"bronze_axe"`) || !strings.Contains(err.Error(), `"pest_tree"`) {
		t.Errorf("Load error = %v, want both missing names listed", err)
	}
}

func TestLoadNilInputs(t *testing.T) {
	if _, err := Load(nil); err == nil {
		t.Error("Load(nil) should fail")
	}
	if _, err := LoadFromCache(nil); err == nil {
		t.Error("LoadFromCache(nil) should fail")
	}
	var nilTables *Tables
	if nilTables.TreeByName("Tree") != nil || nilTables.TreeByObjectID(1) != nil || nilTables.AxeByID(1) != nil {
		t.Error("nil-table lookups should return nil")
	}
}

func TestCutChanceRow(t *testing.T) {
	// Present rows pass through untouched.
	if got := CutChanceRow(CutChance["yew"], 7); got != (CutRange{Low: 15, High: 47, Has: true}) {
		t.Errorf("yew dragon row = %+v, want {15 47}", got)
	}
	// Blisterwood iron (tier 1) scales off normal: bronze [15,50],
	// normal iron [96,300], normal bronze [64,200] ->
	// low round(15*96/64) = 23 (22.5 rounds up), high 50*300/200 = 75.
	if got := CutChanceRow(CutChance["blisterwood"], 1); got != (CutRange{Low: 23, High: 75, Has: true}) {
		t.Errorf("blisterwood iron row = %+v, want {23 75}", got)
	}
	// Blisterwood dragon (tier 7) is listed, not scaled.
	if got := CutChanceRow(CutChance["blisterwood"], 7); got != (CutRange{Low: 61, High: 186, Has: true}) {
		t.Errorf("blisterwood dragon row = %+v, want {61 186}", got)
	}
}

// fakeObjectDefSource is a hand-built ObjectDefSource keyed by loc id.
type fakeObjectDefSource struct {
	defs map[int]ObjectDetail
}

func (s fakeObjectDefSource) ObjectDef(id int) (ObjectDetail, bool) {
	d, ok := s.defs[id]
	return d, ok
}

// TestResolveObjectDetails pins the stump memo + footprint sizes
// (resolveStumpId, plugin.js:1155-1185): the model map, then the
// treeId+1 option-less stump, then the treeId+1 same-name variant,
// then the tree's own stumpId, then the default — with the actions
// check blocking the treeId+1 legs and a missing definition
// falling through to the tree's own stumpId.
func TestResolveObjectDetails(t *testing.T) {
	tables, err := Load(newFixtureResolver())
	if err != nil {
		t.Fatalf("Load(fixture) = %v", err)
	}
	normal := treeByName(tables, "normal tree")
	oak := treeByName(tables, "oak")
	willow := treeByName(tables, "willow")
	maple := treeByName(tables, "maple")
	teak := treeByName(tables, "teak")
	if normal == nil || oak == nil || willow == nil || maple == nil || teak == nil {
		t.Fatal("fixture tables missing trees")
	}
	// One model key from the resolved model map, for the
	// model-map leg.
	var modelStump int
	var modelID int
	for m, s := range tables.NormalTreeStumpByModel {
		modelID, modelStump = m, s
		break
	}
	if modelStump == 0 {
		t.Fatal("fixture model map is empty")
	}
	nid, oid, wid, mid := normal.ObjectIDs[0], oak.ObjectIDs[0], willow.ObjectIDs[0], maple.ObjectIDs[0]
	src := fakeObjectDefSource{defs: map[int]ObjectDetail{
		nid:     {Name: "Tree", FirstModelID: modelID, HasModel: true, SizeX: 2, SizeY: 1},
		oid:     {Name: "Oak tree", FirstModelID: 999991, HasModel: true, SizeX: 1, SizeY: 1},
		oid + 1: {Name: "Oak stump"},
		wid:     {Name: "Willow tree", FirstModelID: 999992, HasModel: true, SizeX: 1, SizeY: 1},
		wid + 1: {Name: "Willow tree", FirstModelID: 999993, HasModel: true},
		mid:     {Name: "Maple tree", FirstModelID: 999994, HasModel: true, SizeX: 1, SizeY: 1},
		mid + 1: {Name: "Maple tree", Actions: []string{"Chop down"}},
	}}
	tables.resolveObjectDetails(src)
	if got := tables.StumpByTreeID[nid]; got != modelStump {
		t.Errorf("normal stump = %d, want model-map stump %d", got, modelStump)
	}
	if got := tables.StumpByTreeID[oid]; got != oid+1 {
		t.Errorf("oak stump = %d, want treeId+1 stump %d", got, oid+1)
	}
	if got := tables.StumpByTreeID[wid]; got != wid+1 {
		t.Errorf("willow stump = %d, want same-name variant %d", got, wid+1)
	}
	if got := tables.StumpByTreeID[mid]; got != maple.StumpID {
		t.Errorf("maple stump = %d, want own stumpId %d (actions block the treeId+1 legs)", got, maple.StumpID)
	}
	// Teak has no definition in the source: its own stumpId.
	tid := teak.ObjectIDs[0]
	if got := tables.StumpByTreeID[tid]; got != teak.StumpID {
		t.Errorf("teak stump = %d, want own stumpId %d (missing definition)", got, teak.StumpID)
	}
	if got := tables.ObjectSize[nid]; got != (ObjectSize{SizeX: 2, SizeY: 1}) {
		t.Errorf("normal footprint = %+v, want {2 1}", got)
	}
	if got := tables.ObjectSize[oid]; got != (ObjectSize{SizeX: 1, SizeY: 1}) {
		t.Errorf("oak footprint = %+v, want {1 1}", got)
	}
	// A same-name next id with the SAME model is not a variant.
	same := fakeObjectDefSource{defs: map[int]ObjectDetail{
		wid:     {Name: "Willow tree", FirstModelID: 999992, HasModel: true},
		wid + 1: {Name: "Willow tree", FirstModelID: 999992, HasModel: true},
	}}
	fresh, err := Load(newFixtureResolver())
	if err != nil {
		t.Fatalf("Load(fixture) = %v", err)
	}
	fresh.resolveObjectDetails(same)
	if got := fresh.StumpByTreeID[wid]; got != willow.StumpID {
		t.Errorf("willow same-model stump = %d, want own stumpId %d", got, willow.StumpID)
	}
}
