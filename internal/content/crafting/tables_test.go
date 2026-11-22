package crafting

import (
	"strings"
	"testing"

	"tsps-go/internal/cache"
)

// ---- load-proven tables (real rev-242 cache) ----

const seedCachePath = "/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03"

// loadRealTables loads the tables against the real rev-242
// cache, skipping when the seed cache is absent.
func loadRealTables(t *testing.T) (*Tables, *cache.CacheDefinitions) {
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
	// LoadFile takes the file by its package-relative path.
	tables, err := LoadFile("../../../data/definitions/crafting.json", cacheResolver{defs: defs})
	if err != nil {
		t.Fatalf("LoadFile: %v", err)
	}
	return tables, defs
}

// fallbackDisplayNames is every itemIDFallbacks id and the
// display name the real rev-242 cache must give it
// (transcribed from ItemIdentifiers.ts comments, verified
// here against the cache).
var fallbackDisplayNames = map[int]string{
	4823:  "Adamantite nails",          // ADAMANTITE_NAILS — ItemIdentifiers.ts:4324
	21350: "Amethyst arrowtips",        // AMETHYST_ARROWTIPS — ItemIdentifiers.ts:19697
	21338: "Amethyst bolt tips",        // AMETHYST_BOLT_TIPS — ItemIdentifiers.ts:19685
	21352: "Amethyst javelin tips",     // AMETHYST_JAVELIN_TIPS — ItemIdentifiers.ts:19699
	5376:  "Basket",                    // BASKET — ItemIdentifiers.ts:4690
	2503:  "Black d'hide body",         // BLACK_DHIDE_BODY — ItemIdentifiers.ts:2414
	2497:  "Black d'hide chaps",        // BLACK_DHIDE_CHAPS — ItemIdentifiers.ts:2408
	2491:  "Black d'hide vambraces",    // BLACK_DHIDE_VAMBRACES — ItemIdentifiers.ts:2402
	2509:  "Black dragon leather",      // BLACK_DRAGON_LEATHER — ItemIdentifiers.ts:2420
	10085: "Black spiky vambraces",     // BLACK_SPIKY_VAMBRACES — ItemIdentifiers.ts:8796
	3353:  "Blamish bark shell",        // BLAMISH_BARK_SHELL — ItemIdentifiers.ts:3134
	3351:  "Blamish blue shell",        // BLAMISH_BLUE_SHELL — ItemIdentifiers.ts:3132
	3361:  "Blamish blue shell",        // BLAMISH_BLUE_SHELL_3 — ItemIdentifiers.ts:3142
	3345:  "Blamish myre shell",        // BLAMISH_MYRE_SHELL — ItemIdentifiers.ts:3126
	3355:  "Blamish myre shell",        // BLAMISH_MYRE_SHELL_3 — ItemIdentifiers.ts:3136
	3349:  "Blamish ochre shell",       // BLAMISH_OCHRE_SHELL — ItemIdentifiers.ts:3130
	3359:  "Blamish ochre shell",       // BLAMISH_OCHRE_SHELL_3 — ItemIdentifiers.ts:3140
	3347:  "Blamish red shell",         // BLAMISH_RED_SHELL — ItemIdentifiers.ts:3128
	3357:  "Blamish red shell",         // BLAMISH_RED_SHELL_3 — ItemIdentifiers.ts:3138
	3329:  "Blood'n'tar snelm",         // BLOODNTAR_SNELM — ItemIdentifiers.ts:3110
	2499:  "Blue d'hide body",          // BLUE_DHIDE_BODY — ItemIdentifiers.ts:2410
	2493:  "Blue d'hide chaps",         // BLUE_DHIDE_CHAPS — ItemIdentifiers.ts:2404
	2487:  "Blue d'hide vambraces",     // BLUE_DHIDE_VAMBRACES — ItemIdentifiers.ts:2398
	2505:  "Blue dragon leather",       // BLUE_DRAGON_LEATHER — ItemIdentifiers.ts:2416
	10081: "Blue spiky vambraces",      // BLUE_SPIKY_VAMBRACES — ItemIdentifiers.ts:8792
	9434:  "Bolt mould",                // BOLT_MOULD — ItemIdentifiers.ts:8200
	1923:  "Bowl",                      // BOWL — ItemIdentifiers.ts:1836
	1921:  "Bowl of water",             // BOWL_OF_WATER — ItemIdentifiers.ts:1834
	11065: "Bracelet mould",            // BRACELET_MOULD — ItemIdentifiers.ts:9718
	3335:  "Broken bark snelm",         // BROKEN_BARK_SNELM — ItemIdentifiers.ts:3116
	4819:  "Bronze nails",              // BRONZE_NAILS — ItemIdentifiers.ts:4320
	3333:  "Bruise blue snelm",         // BRUISE_BLUE_SNELM — ItemIdentifiers.ts:3114
	1925:  "Bucket",                    // BUCKET — ItemIdentifiers.ts:1838
	1929:  "Bucket of water",           // BUCKET_OF_WATER — ItemIdentifiers.ts:1842
	688:   "Buttons",                   // BUTTONS — ItemIdentifiers.ts:686
	10981: "Cave goblin wire",          // CAVE_GOBLIN_WIRE — ItemIdentifiers.ts:9637
	4201:  "Conductor",                 // CONDUCTOR — ItemIdentifiers.ts:3792
	4200:  "Conductor mould",           // CONDUCTOR_MOULD — ItemIdentifiers.ts:3791
	29920: "Costume needle",            // COSTUME_NEEDLE — ItemIdentifiers.ts:27742
	9438:  "Crossbow string",           // CROSSBOW_STRING — ItemIdentifiers.ts:8204
	1633:  "Crushed gem",               // CRUSHED_GEM — ItemIdentifiers.ts:1566
	6748:  "Demonic sigil",             // DEMONIC_SIGIL — ItemIdentifiers.ts:5983
	6747:  "Demonic sigil mould",       // DEMONIC_SIGIL_MOULD — ItemIdentifiers.ts:5982
	1700:  "Diamond amulet",            // DIAMOND_AMULET — ItemIdentifiers.ts:1613
	1681:  "Diamond amulet (u)",        // DIAMOND_AMULET_U_ — ItemIdentifiers.ts:1600
	11092: "Diamond bracelet",          // DIAMOND_BRACELET — ItemIdentifiers.ts:9744
	33709: "Diamond sickle (b)",        // DIAMOND_SICKLE_B_ — ItemIdentifiers.ts:31271
	1702:  "Dragonstone amulet",        // DRAGONSTONE_AMULET — ItemIdentifiers.ts:1615
	1683:  "Dragonstone amulet (u)",    // DRAGONSTONE_AMULET_U_ — ItemIdentifiers.ts:1602
	11115: "Dragonstone bracelet",      // DRAGONSTONE_BRACELET — ItemIdentifiers.ts:9767
	1664:  "Dragon necklace",           // DRAGON_NECKLACE — ItemIdentifiers.ts:1590
	21652: "Drift net",                 // DRIFT_NET — ItemIdentifiers.ts:19956
	1696:  "Emerald amulet",            // EMERALD_AMULET — ItemIdentifiers.ts:1609
	1677:  "Emerald amulet (u)",        // EMERALD_AMULET_U_ — ItemIdentifiers.ts:1596
	11076: "Emerald bracelet",          // EMERALD_BRACELET — ItemIdentifiers.ts:9728
	22433: "Emerald sickle (b)",        // EMERALD_SICKLE_B_ — ItemIdentifiers.ts:20713
	4527:  "Empty candle lantern",      // EMPTY_CANDLE_LANTERN — ItemIdentifiers.ts:4104
	1980:  "Empty cup",                 // EMPTY_CUP — ItemIdentifiers.ts:1893
	6667:  "Empty fishbowl",            // EMPTY_FISHBOWL — ItemIdentifiers.ts:5902
	10980: "Empty light orb",           // EMPTY_LIGHT_ORB — ItemIdentifiers.ts:9636
	4525:  "Empty oil lamp",            // EMPTY_OIL_LAMP — ItemIdentifiers.ts:4102
	5350:  "Empty plant pot",           // EMPTY_PLANT_POT — ItemIdentifiers.ts:4664
	5418:  "Empty sack",                // EMPTY_SACK — ItemIdentifiers.ts:4732
	4155:  "Enchanted gem",             // ENCHANTED_GEM — ItemIdentifiers.ts:3752
	21270: "Eternal gem",               // ETERNAL_GEM — ItemIdentifiers.ts:19617
	29163: "Fox fur",                   // FOX_FUR — ItemIdentifiers.ts:27033
	1785:  "Glassblowing pipe",         // GLASSBLOWING_PIPE — ItemIdentifiers.ts:1698
	1692:  "Gold amulet",               // GOLD_AMULET — ItemIdentifiers.ts:1605
	1673:  "Gold amulet (u)",           // GOLD_AMULET_U_ — ItemIdentifiers.ts:1592
	11069: "Gold bracelet",             // GOLD_BRACELET_2 — ItemIdentifiers.ts:9721
	10099: "Graahk fur",                // GRAAHK_FUR — ItemIdentifiers.ts:8810
	1135:  "Green d'hide body",         // GREEN_DHIDE_BODY — ItemIdentifiers.ts:1070
	1099:  "Green d'hide chaps",        // GREEN_DHIDE_CHAPS — ItemIdentifiers.ts:1034
	1065:  "Green d'hide vambraces",    // GREEN_DHIDE_VAMBRACES — ItemIdentifiers.ts:1000
	1745:  "Green dragon leather",      // GREEN_DRAGON_LEATHER — ItemIdentifiers.ts:1658
	10079: "Green spiky vambraces",     // GREEN_SPIKY_VAMBRACES — ItemIdentifiers.ts:8790
	10814: "Hair",                      // HAIR — ItemIdentifiers.ts:9483
	22269: "Hard leather shield",       // HARD_LEATHER_SHIELD — ItemIdentifiers.ts:20551
	1599:  "Holy mould",                // HOLY_MOULD — ItemIdentifiers.ts:1532
	30085: "Hueycoatl hide",            // HUEYCOATL_HIDE — ItemIdentifiers.ts:27905
	30076: "Hueycoatl hide body",       // HUEYCOATL_HIDE_BODY — ItemIdentifiers.ts:27896
	30079: "Hueycoatl hide chaps",      // HUEYCOATL_HIDE_CHAPS — ItemIdentifiers.ts:27899
	30073: "Hueycoatl hide coif",       // HUEYCOATL_HIDE_COIF — ItemIdentifiers.ts:27893
	30082: "Hueycoatl hide vambraces",  // HUEYCOATL_HIDE_VAMBRACES — ItemIdentifiers.ts:27902
	29775: "Imcando hammer (off-hand)", // IMCANDO_HAMMER_OFF_HAND_ — ItemIdentifiers.ts:27611
	4820:  "Iron nails",                // IRON_NAILS — ItemIdentifiers.ts:4321
	21111: "Jade amulet",               // JADE_AMULET — ItemIdentifiers.ts:19458
	21102: "Jade amulet (u)",           // JADE_AMULET_U_ — ItemIdentifiers.ts:19449
	1935:  "Jug",                       // JUG — ItemIdentifiers.ts:1848
	1937:  "Jug of water",              // JUG_OF_WATER — ItemIdentifiers.ts:1850
	10113: "Kebbit claws",              // KEBBIT_CLAWS — ItemIdentifiers.ts:8824
	10103: "Kyatt fur",                 // KYATT_FUR — ItemIdentifiers.ts:8814
	4542:  "Lantern lens",              // LANTERN_LENS — ItemIdentifiers.ts:4119
	29303: "Large fur pouch",           // LARGE_FUR_POUCH — ItemIdentifiers.ts:27173
	29297: "Large meat pouch",          // LARGE_MEAT_POUCH — ItemIdentifiers.ts:27167
	10095: "Larupia fur",               // LARUPIA_FUR — ItemIdentifiers.ts:8806
	1129:  "Leather body",              // LEATHER_BODY — ItemIdentifiers.ts:1064
	10973: "Light orb",                 // LIGHT_ORB — ItemIdentifiers.ts:9629
	3420:  "Limestone brick",           // LIMESTONE_BRICK — ItemIdentifiers.ts:3201
	29301: "Medium fur pouch",          // MEDIUM_FUR_POUCH — ItemIdentifiers.ts:27171
	4822:  "Mithril nails",             // MITHRIL_NAILS — ItemIdentifiers.ts:4323
	3327:  "Myre snelm",                // MYRE_SNELM — ItemIdentifiers.ts:3108
	3331:  "Ochre snelm",               // OCHRE_SNELM — ItemIdentifiers.ts:3112
	6581:  "Onyx amulet",               // ONYX_AMULET — ItemIdentifiers.ts:5816
	6579:  "Onyx amulet (u)",           // ONYX_AMULET_U_ — ItemIdentifiers.ts:5814
	11130: "Onyx bracelet",             // ONYX_BRACELET — ItemIdentifiers.ts:9782
	21108: "Opal amulet",               // OPAL_AMULET — ItemIdentifiers.ts:19455
	21099: "Opal amulet (u)",           // OPAL_AMULET_U_ — ItemIdentifiers.ts:19446
	774:   "'perfect' necklace",        // PERFECT_NECKLACE — ItemIdentifiers.ts:772
	773:   "'perfect' ring",            // PERFECT_RING — ItemIdentifiers.ts:771
	2313:  "Pie dish",                  // PIE_DISH — ItemIdentifiers.ts:2226
	3339:  "Pointed blood'n'tar snelm", // POINTED_BLOODNTAR_SNELM — ItemIdentifiers.ts:3120
	3343:  "Pointed bruise blue snelm", // POINTED_BRUISE_BLUE_SNELM — ItemIdentifiers.ts:3124
	3337:  "Pointed myre snelm",        // POINTED_MYRE_SNELM — ItemIdentifiers.ts:3118
	3341:  "Pointed ochre snelm",       // POINTED_OCHRE_SNELM — ItemIdentifiers.ts:3122
	1931:  "Pot",                       // POT — ItemIdentifiers.ts:1844
	4440:  "Pot lid",                   // POT_LID — ItemIdentifiers.ts:4025
	10496: "Polished buttons",          // POLISHED_BUTTONS — ItemIdentifiers.ts:9188
	6041:  "Pre-nature amulet",         // PRE_NATURE_AMULET — ItemIdentifiers.ts:5355
	2501:  "Red d'hide body",           // RED_DHIDE_BODY — ItemIdentifiers.ts:2412
	2495:  "Red d'hide chaps",          // RED_DHIDE_CHAPS — ItemIdentifiers.ts:2406
	2489:  "Red d'hide vambraces",      // RED_DHIDE_VAMBRACES — ItemIdentifiers.ts:2400
	2507:  "Red dragon leather",        // RED_DRAGON_LEATHER — ItemIdentifiers.ts:2418
	10083: "Red spiky vambraces",       // RED_SPIKY_VAMBRACES — ItemIdentifiers.ts:8794
	7649:  "Rod mould",                 // ROD_MOULD — ItemIdentifiers.ts:6801
	1698:  "Ruby amulet",               // RUBY_AMULET — ItemIdentifiers.ts:1611
	1679:  "Ruby amulet (u)",           // RUBY_AMULET_U_ — ItemIdentifiers.ts:1598
	11085: "Ruby bracelet",             // RUBY_BRACELET — ItemIdentifiers.ts:9737
	24693: "Ruby sickle (b)",           // RUBY_SICKLE_B_ — ItemIdentifiers.ts:22771
	4824:  "Rune nails",                // RUNE_NAILS — ItemIdentifiers.ts:4325
	1694:  "Sapphire amulet",           // SAPPHIRE_AMULET — ItemIdentifiers.ts:1607
	1675:  "Sapphire amulet (u)",       // SAPPHIRE_AMULET_U_ — ItemIdentifiers.ts:1594
	11072: "Sapphire bracelet",         // SAPPHIRE_BRACELET_2 — ItemIdentifiers.ts:9724
	9382:  "Silver bolts (unf)",        // SILVER_BOLTS_UNF_ — ItemIdentifiers.ts:8180
	2963:  "Silver sickle (b)",         // SILVER_SICKLE_B_ — ItemIdentifiers.ts:2836
	7637:  "Silvthrill rod",            // SILVTHRILL_ROD_2 — ItemIdentifiers.ts:6789
	9436:  "Sinew",                     // SINEW — ItemIdentifiers.ts:8202
	11866: "Slayer ring (8)",           // SLAYER_RING_8_ — ItemIdentifiers.ts:10331
	21268: "Slayer ring (eternal)",     // SLAYER_RING_ETERNAL_ — ItemIdentifiers.ts:19615
	29299: "Small fur pouch",           // SMALL_FUR_POUCH — ItemIdentifiers.ts:27169
	29295: "Small meat pouch",          // SMALL_MEAT_POUCH — ItemIdentifiers.ts:27165
	6289:  "Snakeskin",                 // SNAKESKIN — ItemIdentifiers.ts:5584
	1761:  "Soft clay",                 // SOFT_CLAY — ItemIdentifiers.ts:1674
	10077: "Spiky vambraces",           // SPIKY_VAMBRACES — ItemIdentifiers.ts:8788
	1539:  "Steel nails",               // STEEL_NAILS — ItemIdentifiers.ts:1473
	2370:  "Steel studs",               // STEEL_STUDS — ItemIdentifiers.ts:2283
	3224:  "Strip of cloth",            // STRIP_OF_CLOTH — ItemIdentifiers.ts:3067
	29177: "Sunlight antelope fur",     // SUNLIGHT_ANTELOPE_FUR — ItemIdentifiers.ts:27047
	21114: "Topaz amulet",              // TOPAZ_AMULET — ItemIdentifiers.ts:19461
	21105: "Topaz amulet (u)",          // TOPAZ_AMULET_U_ — ItemIdentifiers.ts:19452
	1716:  "Unblessed symbol",          // UNBLESSED_SYMBOL — ItemIdentifiers.ts:1629
	1791:  "Unfired bowl",              // UNFIRED_BOWL — ItemIdentifiers.ts:1704
	28193: "Unfired cup",               // UNFIRED_CUP — ItemIdentifiers.ts:26120
	1789:  "Unfired pie dish",          // UNFIRED_PIE_DISH — ItemIdentifiers.ts:1702
	5352:  "Unfired plant pot",         // UNFIRED_PLANT_POT — ItemIdentifiers.ts:4666
	1787:  "Unfired pot",               // UNFIRED_POT — ItemIdentifiers.ts:1700
	4438:  "Unfired pot lid",           // UNFIRED_POT_LID — ItemIdentifiers.ts:4023
	1594:  "Unholy mould",              // UNHOLY_MOULD — ItemIdentifiers.ts:1527
	567:   "Unpowered orb",             // UNPOWERED_ORB — ItemIdentifiers.ts:566
	1722:  "Unpowered symbol",          // UNPOWERED_SYMBOL — ItemIdentifiers.ts:1635
	1720:  "Unstrung emblem",           // UNSTRUNG_EMBLEM — ItemIdentifiers.ts:1633
	1714:  "Unstrung symbol",           // UNSTRUNG_SYMBOL — ItemIdentifiers.ts:1627
	229:   "Vial",                      // VIAL — ItemIdentifiers.ts:228
	227:   "Vial of water",             // VIAL_OF_WATER — ItemIdentifiers.ts:226
	13383: "Xerician fabric",           // XERICIAN_FABRIC — ItemIdentifiers.ts:11829
	13385: "Xerician hat",              // XERICIAN_HAT — ItemIdentifiers.ts:11831
	13389: "Xerician robe",             // XERICIAN_ROBE — ItemIdentifiers.ts:11835
	13387: "Xerician top",              // XERICIAN_TOP — ItemIdentifiers.ts:11833
	19501: "Zenyte amulet (u)",         // ZENYTE_AMULET_U_ — ItemIdentifiers.ts:17930
	19532: "Zenyte bracelet",           // ZENYTE_BRACELET_2 — ItemIdentifiers.ts:17960
}

func TestLoadRealFile(t *testing.T) {
	tables, defs := loadRealTables(t)

	// Fallback display names, verified against the real cache.
	for id, want := range fallbackDisplayNames {
		item := defs.GetItem(id)
		if item == nil {
			t.Errorf("fallback item %d: missing from cache", id)
			continue
		}
		if item.Name != want {
			t.Errorf("fallback item %d: name %q, want %q", id, item.Name, want)
		}
	}

	// Section counts (PHASE44_PLAN.md: 16 sections).
	if got := len(tables.Leather); got != 9 {
		t.Errorf("leather materials = %d, want 9", got)
	}
	leatherRecipes := 0
	for _, material := range tables.Leather {
		leatherRecipes += len(material.Recipes)
	}
	if leatherRecipes != 32 {
		t.Errorf("leather recipes = %d, want 32", leatherRecipes)
	}
	if got := len(tables.HuntersPouches); got != 5 {
		t.Errorf("hunters pouches = %d, want 5", got)
	}
	if got := len(tables.Combine); got != 13 {
		t.Errorf("combine recipes = %d, want 13", got)
	}
	if got := len(tables.Gems); got != 10 {
		t.Errorf("gem recipes = %d, want 10", got)
	}
	if got := len(tables.Amethyst.Recipes); got != 4 {
		t.Errorf("amethyst recipes = %d, want 4", got)
	}
	if got := len(tables.Glassblowing.Recipes); got != 8 {
		t.Errorf("glassblowing recipes = %d, want 8", got)
	}
	if got := len(tables.Pottery.Recipes); got != 6 {
		t.Errorf("pottery recipes = %d, want 6", got)
	}
	if got := len(tables.Pottery.WaterContainers); got != 4 {
		t.Errorf("pottery water containers = %d, want 4", got)
	}
	if got := len(tables.Spinning.Recipes); got != 8 {
		t.Errorf("spinning recipes = %d, want 8", got)
	}
	if got := len(tables.Weaving.Recipes); got != 7 {
		t.Errorf("weaving recipes = %d, want 7", got)
	}
	if got := len(tables.Gold.Types); got != 4 {
		t.Errorf("gold types = %d, want 4", got)
	}
	goldProducts := 0
	for _, typ := range tables.Gold.Types {
		goldProducts += len(typ.Products)
	}
	if goldProducts != 32 {
		t.Errorf("gold products = %d, want 32", goldProducts)
	}
	if got := len(tables.Silver.Products); got != 20 {
		t.Errorf("silver products = %d, want 20", got)
	}
	if got := len(tables.Perfect); got != 2 {
		t.Errorf("perfect recipes = %d, want 2", got)
	}
	if got := len(tables.StrungAmulets.Pairs); got != 14 {
		t.Errorf("strung amulet pairs = %d, want 14", got)
	}
	if got := len(tables.Battlestaves.Recipes); got != 4 {
		t.Errorf("battlestaves recipes = %d, want 4", got)
	}
	if got := len(tables.Snelms.Recipes); got != 9 {
		t.Errorf("snelm recipes = %d, want 9", got)
	}
	if got := len(tables.GemSickles); got != 3 {
		t.Errorf("gem sickle recipes = %d, want 3", got)
	}

	// All 271 JSON names resolve, plus the 24 unit tool names
	// Load resolves outside the JSON (extraItemNames).
	if got := len(tables.ItemIDs); got != 295 {
		t.Errorf("ItemIDs = %d, want 295 (271 JSON + 24 unit names)", got)
	}

	// Spot checks resolved with correct ids (probe-verified).
	spot := map[string]int{
		"FLAX": 1779, "BOW_STRING": 1777, "GOLD_BAR": 2357,
		"RING_MOULD": 1592, "NEEDLE": 1733, "THREAD": 1734,
		"CHISEL": 1755, "MOLTEN_GLASS": 1775, "AMETHYST": 21347,
		"LIMESTONE": 3211, "SOFT_CLAY": 1761, "BUTTONS": 688,
		"POLISHED_BUTTONS": 10496, "BASKET": 5376,
		"LIMESTONE_BRICK": 3420, "LIGHT_ORB": 10973,
		"EMPTY_LIGHT_ORB": 10980, "CAVE_GOBLIN_WIRE": 10981,
	}
	for name, want := range spot {
		got, ok := tables.ItemID(name)
		if !ok || got != want {
			t.Errorf("ItemID(%q) = %d, %v, want %d", name, got, ok, want)
		}
	}

	// Enums 4318 (33 entries) / 4319 (20 entries), keyed by
	// (interfaceId<<16)|component.
	if got := len(tables.GoldLastType); got != 33 {
		t.Errorf("gold enum 4318 entries = %d, want 33", got)
	}
	if got := tables.GoldLastType[(446<<16)|8]; got != 1 {
		t.Errorf("gold enum (446,8) = %d, want 1", got)
	}
	if got := len(tables.SilverLastType); got != 20 {
		t.Errorf("silver enum 4319 entries = %d, want 20", got)
	}
	if got := tables.SilverLastType[(6<<16)|7]; got != 1 {
		t.Errorf("silver enum (6,7) = %d, want 1", got)
	}

	// Varbits.
	varbits := map[string]int{
		"crafting_gold_item_lasttype":   13892,
		"crafting_silver_item_lasttype": 13893,
		"slayer_ring_unlocked":          3207,
	}
	for name, want := range varbits {
		got, ok := tables.VarbitIDs[name]
		if !ok || got != want {
			t.Errorf("VarbitIDs[%q] = %d, %v, want %d", name, got, ok, want)
		}
	}

	// Sequences present with the right names.
	seqs := map[string]int{
		"human_spinningwheel_60":     13139,
		"spinningwheel":              466,
		"farming_useloom":            2270,
		"human_amethystcutting":      6295,
		"human_opalcutting":          890,
		"human_glassblowing":         884,
		"human_battlestaff_crafting": 7531,
		"human_leather_crafting":     1249,
		"human_furnace":              899,
		"human_potterywheel":         883,
		"potteryoven_quick":          1317,
		"human_limestonecutting":     1309,
	}
	for name, want := range seqs {
		got, ok := tables.SeqIDs[name]
		if !ok || got != want {
			t.Errorf("SeqIDs[%q] = %d, %v, want %d", name, got, ok, want)
		}
	}

	// Gold / silver quantity buttons ("x" is the enter-amount
	// component, not a quantity).
	if tables.Gold.Quantities[60] != 1 || tables.Gold.Quantities[61] != 5 ||
		tables.Gold.Quantities[62] != 10 || tables.Gold.Quantities[64] != 28 ||
		tables.Gold.XComponent != 63 {
		t.Errorf("gold quantities = %v x=%d, want {60:1 61:5 62:10 64:28} x=63", tables.Gold.Quantities, tables.Gold.XComponent)
	}
	if tables.Silver.Quantities[32] != 1 || tables.Silver.Quantities[33] != 5 ||
		tables.Silver.Quantities[34] != 10 || tables.Silver.Quantities[36] != 28 ||
		tables.Silver.XComponent != 35 {
		t.Errorf("silver quantities = %v x=%d, want {32:1 33:5 34:10 36:28} x=35", tables.Silver.Quantities, tables.Silver.XComponent)
	}

	// Zenyte gem-cut XP is whatever the JSON says: 50 in the
	// imported file (the OSRS value is 200 — the loader ports
	// the JSON, it does not second-guess it).
	zenyte := tables.GemByUncut(19496) // UNCUT_ZENYTE
	if zenyte == nil || zenyte.XP != 50 {
		t.Errorf("zenyte cut XP = %v, want 50 (JSON value)", zenyte)
	}
	if cutID, xp, ok := tables.CutGem(19496); !ok || cutID != 19493 || xp != 50 {
		t.Errorf("CutGem(uncut zenyte) = %d, %v, %v, want 19493, 50, true", cutID, xp, ok)
	}

	// Leather order is the file order (menu order).
	if len(tables.Leather) < 2 || tables.Leather[0].MaterialName != "LEATHER" ||
		tables.Leather[1].MaterialName != "HARD_LEATHER" {
		t.Errorf("leather order = %v, want LEATHER, HARD_LEATHER first", tables.Leather)
	}
	leatherGloves := tables.LeatherByMaterial(1741) // LEATHER
	if len(leatherGloves) != 7 || leatherGloves[0].ItemID != 1059 || leatherGloves[0].XP != 13.8 {
		t.Errorf("leather gloves = %v, want 7 recipes, gloves 1059 at 13.8 XP", leatherGloves)
	}

	// Silver component names resolve as-is (already lowercase
	// gameval form): opal_ring resolves to OPAL_RING 21081,
	// holy_symbol does not (it is a script label, not an obj
	// gameval) — and neither fails the load.
	if got := tables.Silver.Products[0].ComponentObjID; got != 21081 {
		t.Errorf("silver opal_ring component obj id = %d, want 21081 (OPAL_RING)", got)
	}
	if tables.Silver.Products[12].ComponentObjID != 0 { // holy_symbol
		t.Errorf("silver holy_symbol component obj id = %d, want 0 (not an obj gameval)", tables.Silver.Products[12].ComponentObjID)
	}
}

// ---- fixture loads (no cache) ----

// stubResolver answers every name with a canned id, except
// the names listed in missing — the fixture Load tests
// fail closed on those.
type stubResolver struct {
	missing map[string]bool
	seqs    map[string]int
}

func (s stubResolver) ObjID(name string) (int, bool, error) {
	if s.missing[name] {
		return 0, false, nil
	}
	return 4242, true, nil
}

func (s stubResolver) SeqID(name string) (int, bool, error) {
	if id, ok := s.seqs[name]; ok {
		return id, true, nil
	}
	if s.missing[name] {
		return 0, false, nil
	}
	return 99, true, nil
}

func (s stubResolver) VarbitID(name string) (int, bool, error) {
	if s.missing[name] {
		return 0, false, nil
	}
	return 77, true, nil
}

func (s stubResolver) EnumValues(id int) (map[int]int, error) {
	return map[int]int{1: 1}, nil
}

func (s stubResolver) ObjNames() (map[int]string, error) {
	return map[int]string{}, nil
}

func (s stubResolver) InterfaceID(name string) (int, bool, error) {
	return 0, false, nil
}

const minimalCraftingJSON = `{"leather": {"LEATHER": [{"item": "LEATHER_GLOVES", "leather": 1, "level": 1, "xp": 13.8, "made": "a pair of leather gloves"}]}, "gems": {"recipes": [{"uncut": "UNCUT_SAPPHIRE", "cut": "SAPPHIRE", "plural": "sapphires", "level": 20, "xp": 50, "seq": 888, "seqName": "human_sapphirecutting"}]}, "amethyst": {"seq": 6295, "seqName": "human_amethystcutting", "recipes": []}, "glassblowing": {"seq": 884, "seqName": "human_glassblowing", "sound": 2724, "soundDelay": 10, "recipes": []}, "pottery": {"seq": 883, "seqName": "human_potterywheel", "ovenSeq": 1317, "ovenSeqName": "potteryoven_quick", "sound": 2588, "ovenSound": 2725, "recipes": [], "waterContainers": []}, "spinning": {"seq": 13139, "seqName": "human_spinningwheel_60", "wheelSeq": 466, "wheelSeqName": "spinningwheel", "sound": 2590, "recipes": []}, "weaving": {"seq": 2270, "seqName": "farming_useloom", "sound": 2587, "soundDelay": 2, "recipes": []}, "gold": {"gems": [null], "types": [], "slayerRing": {"component": 16, "mould": "RING_MOULD", "level": 75, "xp": 15, "unlockVarbit": "slayer_ring_unlocked", "products": []}, "quantities": {"60": 1}}, "silver": {"products": [], "quantities": {"32": 1}}, "perfect": {"recipes": []}, "strungAmulets": {"xp": 4, "pairs": []}, "battlestaves": {"seq": 7531, "seqName": "human_battlestaff_crafting", "recipes": []}, "snelms": {"level": 15, "xp": 32.5, "ticks": 2, "seq": 890, "seqName": "human_opalcutting", "recipes": []}, "gemSickles": {"recipes": []}, "huntersPouches": {"recipes": []}, "combine": {"recipes": []}}`

func TestLoadFixture(t *testing.T) {
	r := stubResolver{seqs: map[string]int{
		"human_sapphirecutting": 888, "human_amethystcutting": 6295,
		"human_glassblowing": 884, "human_potterywheel": 883,
		"potteryoven_quick": 1317, "human_spinningwheel_60": 13139,
		"spinningwheel": 466, "farming_useloom": 2270,
		"human_battlestaff_crafting": 7531, "human_opalcutting": 890,
		"human_furnace": 899, "human_limestonecutting": 1309,
	}}
	tables, err := Load([]byte(minimalCraftingJSON), r)
	if err != nil {
		t.Fatalf("Load fixture: %v", err)
	}
	if len(tables.Leather) != 1 || len(tables.Gems) != 1 {
		t.Fatalf("fixture tables = %+v", tables)
	}
	if got := tables.Gems[0].CutID; got != 4242 {
		t.Errorf("fixture sapphire cut id = %d, want stub 4242", got)
	}
}

func TestLoadFailsOnUnresolvableName(t *testing.T) {
	r := stubResolver{
		missing: map[string]bool{"leather_gloves": true},
		seqs: map[string]int{
			"human_sapphirecutting": 888, "human_amethystcutting": 6295,
			"human_glassblowing": 884, "human_potterywheel": 883,
			"potteryoven_quick": 1317, "human_spinningwheel_60": 13139,
			"spinningwheel": 466, "farming_useloom": 2270,
			"human_battlestaff_crafting": 7531, "human_opalcutting": 890,
			"human_furnace": 899, "human_limestonecutting": 1309,
		},
	}
	_, err := Load([]byte(minimalCraftingJSON), r)
	if err == nil {
		t.Fatal("Load with an unresolvable name = nil, want an error naming it")
	}
	if !strings.Contains(err.Error(), "LEATHER_GLOVES") {
		t.Errorf("Load error = %v, want it to name LEATHER_GLOVES", err)
	}
}

func TestLoadFromCacheDefaultPath(t *testing.T) {
	// LoadFromCache reads data/definitions/crafting.json
	// relative to the repo root; the test chdirs there.
	_, defs := loadRealTables(t)
	t.Chdir("../../../")
	tables, err := LoadFromCache(defs)
	if err != nil {
		t.Fatalf("LoadFromCache: %v", err)
	}
	if len(tables.ItemIDs) != 295 {
		t.Errorf("LoadFromCache ItemIDs = %d, want 293", len(tables.ItemIDs))
	}
}

// TestObjectNameResolverInstalled proves the item-on-object
// router can resolve object names once wiring installs the
// resolver (the Phase 43 live-found gap, do NOT repeat it):
// without a resolver every name resolves to "", and the
// test fails if the resolver is never installed. The ids
// are the cache-probed live ids the smoke harnesses use
// (Furnace 24009 at Edgeville; the potter's wheel and
// spinning wheel ids are resolved by name in the live legs).
func TestObjectNameResolverInstalled(t *testing.T) {
	if got := shared.objectNameOf(24009); got != "" {
		t.Fatalf("objectNameOf(24009) without a resolver = %q, want \"\"", got)
	}
	if ObjectNameResolverInstalled() {
		t.Fatal("ObjectNameResolverInstalled() = true without a resolver")
	}
	names := map[int]string{24009: "Furnace", 39620: "Rusted anvil"}
	SetObjectNameResolver(func(id int) string { return names[id] })
	t.Cleanup(func() { SetObjectNameResolver(nil) })
	if !ObjectNameResolverInstalled() {
		t.Fatal("ObjectNameResolverInstalled() = false after installing")
	}
	if got := shared.objectNameOf(24009); got != "Furnace" {
		t.Errorf("objectNameOf(24009) = %q, want %q", got, "Furnace")
	}
	if got := shared.objectNameOf(39620); got != "Rusted anvil" {
		t.Errorf("objectNameOf(39620) = %q, want %q", got, "Rusted anvil")
	}
	if got := shared.objectNameOf(99999); got != "" {
		t.Errorf("objectNameOf(99999) = %q, want \"\"", got)
	}
	SetObjectNameResolver(nil)
	if got := shared.objectNameOf(24009); got != "" {
		t.Errorf("objectNameOf(24009) after clearing = %q, want \"\"", got)
	}
}
