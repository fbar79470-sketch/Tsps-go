package mining

import (
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
	tables, err := LoadFromCache(defs)
	if err != nil {
		t.Fatalf("LoadFromCache: %v", err)
	}
	return tables
}

func TestLoadFromCacheReal(t *testing.T) {
	tables := loadRealTables(t)
	if len(tables.Pickaxes) != 23 {
		t.Errorf("pickaxes = %d, want 23", len(tables.Pickaxes))
	}
	if len(tables.Rocks) != 26 {
		t.Errorf("rocks = %d, want 26", len(tables.Rocks))
	}
	if len(tables.Veins) != 7 {
		t.Errorf("veins = %d, want 7", len(tables.Veins))
	}
	if len(tables.EssenceRocks) != 3 {
		t.Errorf("essence rocks = %d, want 3", len(tables.EssenceRocks))
	}
	if len(tables.GemTable) != 8 {
		t.Errorf("gem rows = %d, want 8", len(tables.GemTable))
	}
	if tables.OresMinedVarpID != 4526 {
		t.Errorf("ores-mined varp = %d, want 4526", tables.OresMinedVarpID)
	}
	// Every gem row resolves to its item.
	for _, row := range tables.GemTable {
		if row.Name != "" && row.ItemID <= 0 {
			t.Errorf("gem %q unresolved", row.Name)
		}
	}
	// Wall swing sequences resolve for the keys that have them.
	if got := tables.SwingSeqs["human_mining_bronze_pickaxe_wall"]; got <= 0 {
		t.Errorf("bronze wall swing unresolved")
	}
}

func TestPickaxesDescOrder(t *testing.T) {
	tables := DefaultTables()
	desc := tables.PickaxesDesc
	if len(desc) != 23 {
		t.Fatalf("desc = %d, want 23", len(desc))
	}
	// Echo (rank 72) first, then crystal (71), then the 61s
	// (dragon first by interval 3 — all 3), then rune (41).
	if desc[0].ID != 25112 {
		t.Errorf("desc[0] = %d, want echo 25112", desc[0].ID)
	}
	if desc[1].ID != 23680 {
		t.Errorf("desc[1] = %d, want crystal 23680", desc[1].ID)
	}
	// Inactive crystal (rank 61) sorts with the dragon tier,
	// not with its 71 requirement.
	found := -1
	for i, p := range desc {
		if p.ID == 23682 {
			found = i
			break
		}
	}
	if found < 0 {
		t.Fatal("inactive crystal missing from desc")
	}
	if found < 2 || found > 14 {
		t.Errorf("inactive crystal at desc[%d], want in the 61 tier (2..14)", found)
	}
	// Bronze last (rank 1, interval 8).
	if desc[len(desc)-1].ID != 1265 {
		t.Errorf("desc last = %d, want bronze 1265", desc[len(desc)-1].ID)
	}
}

func TestDepletedRockID(t *testing.T) {
	tables := DefaultTables()
	tables.LocGamevalByID = map[int]string{
		10943: "copperrock1", 11161: "copperrock2",
		11360: "tinrock1", 11361: "tinrock2",
		11380: "gemrock1", 11381: "gemrock",
		11386: "sandstone", 11387: "granite",
		11388: "amethystrock1", 11393: "amethystrock_empty",
		58917: "leadrock1", 58918: "leadrock1_empty",
		17962: "area_sanguine_mine_minerocks_01",
		17965: "area_sanguine_mine_minerocks_04",
		11390: "rocks1", 11391: "rocks2",
	}
	tables.LocIDByGameval = map[string]int{}
	for id, name := range tables.LocGamevalByID {
		tables.LocIDByGameval[name] = id
	}
	rockByName := func(name string) *Rock { return tables.RockByName(name) }
	cases := []struct {
		rockID int
		rock   string
		want   int
	}{
		{10943, "Copper rocks", 11390}, // copperrock1 -> rocks1 (digit fallback)
		{11161, "Copper rocks", 11391}, // copperrock2 -> rocks2
		{11360, "Tin rocks", 11390},
		{11361, "Tin rocks", 11391},
		{11381, "Gem rocks", 11391},         // gemrock -> rocks2 (own map)
		{11380, "Gem rocks", 11390},         // gemrock1 -> rocks1 (digit fallback)
		{58917, "Lead rocks", 58918},        // leadrock1 -> leadrock1_empty
		{11388, "Amethyst crystals", 11393}, // amethyst -> amethystrock_empty
		{17962, "Daeyalt rocks", 17965},     // daeyalt stage 1 -> _04 via depletedRock
		{11387, "Granite rocks", 11391},     // granite -> rocks2 (own)
	}
	for _, c := range cases {
		rock := rockByName(c.rock)
		if rock == nil {
			t.Fatalf("rock %q missing", c.rock)
		}
		if got := tables.DepletedRockID(c.rockID, rock); got != c.want {
			t.Errorf("DepletedRockID(%d, %s) = %d, want %d", c.rockID, c.rock, got, c.want)
		}
	}
	// An unknown rock falls back to rocks1.
	if got := tables.DepletedRockID(999999, rockByName("Copper rocks")); got != 11390 {
		t.Errorf("unknown rock depleted = %d, want rocks1 11390", got)
	}
}

func TestSwingSeqFallback(t *testing.T) {
	tables := DefaultTables()
	bronze := tables.Pickaxes[0]
	rock := tables.RockByName("Copper rocks")
	if got := tables.SwingSeq(bronze, rock); got != bronze.Animation {
		t.Errorf("plain rock swing = %d, want pickaxe animation %d", got, bronze.Animation)
	}
	// A wall rock with no resolved sequences falls back too.
	barronite := tables.RockByName("Barronite rocks")
	if got := tables.SwingSeq(bronze, barronite); got != bronze.Animation {
		t.Errorf("wall swing without seq = %d, want %d", got, bronze.Animation)
	}
	// A resolved sequence wins.
	tables.SwingSeqs["human_mining_bronze_pickaxe_wall"] = 9999
	if got := tables.SwingSeq(bronze, barronite); got != 9999 {
		t.Errorf("wall swing with seq = %d, want 9999", got)
	}
	// The dragon_pretty override orders the words differently.
	pretty := tables.PickaxesDesc[0]
	for i := range tables.Pickaxes {
		if tables.Pickaxes[i].ID == 12797 {
			pretty = tables.Pickaxes[i]
		}
	}
	tables.SwingSeqs["human_mining_dragon_pickaxe_pretty_wall"] = 8888
	if got := tables.SwingSeq(pretty, barronite); got != 8888 {
		t.Errorf("dragon_pretty wall swing = %d, want 8888", got)
	}
}
