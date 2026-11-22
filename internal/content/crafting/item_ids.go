// Fallback item ids for the ItemIdentifiers names in
// data/definitions/crafting.json (plus the tool/item names the
// Crafting units use outside the JSON) that the rev-242 cache
// has no gameval for.
//
// WHY THIS TABLE EXISTS. The JSON names items by their
// ItemIdentifiers (UPPER_SNAKE) names and Load resolves them
// through the cache gamevals (lowercase form, the
// Fletching/Cooking/Smithing pattern). 159 JSON names have no
// (correct) gameval — their gamevals are unrelated names
// (e.g. BASKET is not "basket" in the cache), or the name is
// a display-form variant (AMULET_U_, BRACELET_2, SICKLE_B_).
// Eleven further names the units use outside the JSON
// (BUTTONS, CAVE_GOBLIN_WIRE, COSTUME_NEEDLE, CRUSHED_GEM,
// GLASSBLOWING_PIPE, IMCANDO_HAMMER_OFF_HAND_, LIGHT_ORB,
// LIMESTONE_BRICK, POLISHED_BUTTONS, SILVER_SICKLE_B_,
// SOFT_CLAY) resolve the same way;
// EMPTY_LIGHT_ORB is both a glassblowing product and a
// LightOrbs input. There is no systematic transform from
// ItemIdentifiers name to gameval name, so these resolve
// through this table instead.
//
// GROUNDING. Every id below is transcribed from
// server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (upstream commit bfd81e3, itself generated from this cache),
// with the file line per entry, and every id's display name
// was verified against the real rev-242 cache in the
// load-prove test (tables_test.go). No id here is invented
// or guessed. The loader consults this table FIRST, before
// the gameval lookup, so a name in neither the table nor the
// cache fails Load listing every missing name.
//
// Limestone and light orbs have NO section in crafting.json
// (confirmed 2026-11-21): LIMESTONE_BRICK, LIGHT_ORB and
// CAVE_GOBLIN_WIRE are here only because the Limestone and
// LightOrbs units hardcode them (level 12 / 6 XP and level
// 87 / 104 XP in the TS units); no JSON section is invented
// for them.
package crafting

var itemIDFallbacks = map[string]int{
	"ADAMANTITE_NAILS":          4823,  // Adamantite nails — ItemIdentifiers.ts:4324
	"AMETHYST_ARROWTIPS":        21350, // Amethyst arrowtips — ItemIdentifiers.ts:19697
	"AMETHYST_BOLT_TIPS":        21338, // Amethyst bolt tips — ItemIdentifiers.ts:19685
	"AMETHYST_JAVELIN_TIPS":     21352, // Amethyst javelin tips — ItemIdentifiers.ts:19699
	"BASKET":                    5376,  // Basket — ItemIdentifiers.ts:4690
	"BLACK_DHIDE_BODY":          2503,  // Black d'hide body — ItemIdentifiers.ts:2414
	"BLACK_DHIDE_CHAPS":         2497,  // Black d'hide chaps — ItemIdentifiers.ts:2408
	"BLACK_DHIDE_VAMBRACES":     2491,  // Black d'hide vambraces — ItemIdentifiers.ts:2402
	"BLACK_DRAGON_LEATHER":      2509,  // Black dragon leather — ItemIdentifiers.ts:2420
	"BLACK_SPIKY_VAMBRACES":     10085, // Black spiky vambraces — ItemIdentifiers.ts:8796
	"BLAMISH_BARK_SHELL":        3353,  // Blamish bark shell — ItemIdentifiers.ts:3134
	"BLAMISH_BLUE_SHELL":        3351,  // Blamish blue shell — ItemIdentifiers.ts:3132
	"BLAMISH_BLUE_SHELL_3":      3361,  // Blamish blue shell — ItemIdentifiers.ts:3142
	"BLAMISH_MYRE_SHELL":        3345,  // Blamish myre shell — ItemIdentifiers.ts:3126
	"BLAMISH_MYRE_SHELL_3":      3355,  // Blamish myre shell — ItemIdentifiers.ts:3136
	"BLAMISH_OCHRE_SHELL":       3349,  // Blamish ochre shell — ItemIdentifiers.ts:3130
	"BLAMISH_OCHRE_SHELL_3":     3359,  // Blamish ochre shell — ItemIdentifiers.ts:3140
	"BLAMISH_RED_SHELL":         3347,  // Blamish red shell — ItemIdentifiers.ts:3128
	"BLAMISH_RED_SHELL_3":       3357,  // Blamish red shell — ItemIdentifiers.ts:3138
	"BLOODNTAR_SNELM":           3329,  // Blood'n'tar snelm — ItemIdentifiers.ts:3110
	"BLUE_DHIDE_BODY":           2499,  // Blue d'hide body — ItemIdentifiers.ts:2410
	"BLUE_DHIDE_CHAPS":          2493,  // Blue d'hide chaps — ItemIdentifiers.ts:2404
	"BLUE_DHIDE_VAMBRACES":      2487,  // Blue d'hide vambraces — ItemIdentifiers.ts:2398
	"BLUE_DRAGON_LEATHER":       2505,  // Blue dragon leather — ItemIdentifiers.ts:2416
	"BLUE_SPIKY_VAMBRACES":      10081, // Blue spiky vambraces — ItemIdentifiers.ts:8792
	"BOLT_MOULD":                9434,  // Bolt mould — ItemIdentifiers.ts:8200
	"BOWL":                      1923,  // Bowl — ItemIdentifiers.ts:1836
	"BOWL_OF_WATER":             1921,  // Bowl of water — ItemIdentifiers.ts:1834
	"BRACELET_MOULD":            11065, // Bracelet mould — ItemIdentifiers.ts:9718
	"BROKEN_BARK_SNELM":         3335,  // Broken bark snelm — ItemIdentifiers.ts:3116
	"BRONZE_NAILS":              4819,  // Bronze nails — ItemIdentifiers.ts:4320
	"BRUISE_BLUE_SNELM":         3333,  // Bruise blue snelm — ItemIdentifiers.ts:3114
	"BUCKET":                    1925,  // Bucket — ItemIdentifiers.ts:1838
	"BUCKET_OF_WATER":           1929,  // Bucket of water — ItemIdentifiers.ts:1842
	"BUTTONS":                   688,   // Buttons — ItemIdentifiers.ts:686
	"CAVE_GOBLIN_WIRE":          10981, // Cave goblin wire — ItemIdentifiers.ts:9637
	"CONDUCTOR":                 4201,  // Conductor — ItemIdentifiers.ts:3792
	"CONDUCTOR_MOULD":           4200,  // Conductor mould — ItemIdentifiers.ts:3791
	"COSTUME_NEEDLE":            29920, // Costume needle — ItemIdentifiers.ts:27742
	"CROSSBOW_STRING":           9438,  // Crossbow string — ItemIdentifiers.ts:8204
	"CRUSHED_GEM":               1633,  // Crushed gem — ItemIdentifiers.ts:1566
	"DEMONIC_SIGIL":             6748,  // Demonic sigil — ItemIdentifiers.ts:5983
	"DEMONIC_SIGIL_MOULD":       6747,  // Demonic sigil mould — ItemIdentifiers.ts:5982
	"DIAMOND_AMULET":            1700,  // Diamond amulet — ItemIdentifiers.ts:1613
	"DIAMOND_AMULET_U_":         1681,  // Diamond amulet (u) — ItemIdentifiers.ts:1600
	"DIAMOND_BRACELET":          11092, // Diamond bracelet — ItemIdentifiers.ts:9744
	"DIAMOND_SICKLE_B_":         33709, // Diamond sickle (b) — ItemIdentifiers.ts:31271
	"DRAGONSTONE_AMULET":        1702,  // Dragonstone amulet — ItemIdentifiers.ts:1615
	"DRAGONSTONE_AMULET_U_":     1683,  // Dragonstone amulet (u) — ItemIdentifiers.ts:1602
	"DRAGONSTONE_BRACELET":      11115, // Dragonstone bracelet — ItemIdentifiers.ts:9767
	"DRAGON_NECKLACE":           1664,  // Dragon necklace — ItemIdentifiers.ts:1590
	"DRIFT_NET":                 21652, // Drift net — ItemIdentifiers.ts:19956
	"EMERALD_AMULET":            1696,  // Emerald amulet — ItemIdentifiers.ts:1609
	"EMERALD_AMULET_U_":         1677,  // Emerald amulet (u) — ItemIdentifiers.ts:1596
	"EMERALD_BRACELET":          11076, // Emerald bracelet — ItemIdentifiers.ts:9728
	"EMERALD_SICKLE_B_":         22433, // Emerald sickle (b) — ItemIdentifiers.ts:20713
	"EMPTY_CANDLE_LANTERN":      4527,  // Empty candle lantern — ItemIdentifiers.ts:4104
	"EMPTY_CUP":                 1980,  // Empty cup — ItemIdentifiers.ts:1893
	"EMPTY_FISHBOWL":            6667,  // Empty fishbowl — ItemIdentifiers.ts:5902
	"EMPTY_LIGHT_ORB":           10980, // Empty light orb — ItemIdentifiers.ts:9636
	"EMPTY_OIL_LAMP":            4525,  // Empty oil lamp — ItemIdentifiers.ts:4102
	"EMPTY_PLANT_POT":           5350,  // Empty plant pot — ItemIdentifiers.ts:4664
	"EMPTY_SACK":                5418,  // Empty sack — ItemIdentifiers.ts:4732
	"ENCHANTED_GEM":             4155,  // Enchanted gem — ItemIdentifiers.ts:3752
	"ETERNAL_GEM":               21270, // Eternal gem — ItemIdentifiers.ts:19617
	"FOX_FUR":                   29163, // Fox fur — ItemIdentifiers.ts:27033
	"GLASSBLOWING_PIPE":         1785,  // Glassblowing pipe — ItemIdentifiers.ts:1698
	"GOLD_AMULET":               1692,  // Gold amulet — ItemIdentifiers.ts:1605
	"GOLD_AMULET_U_":            1673,  // Gold amulet (u) — ItemIdentifiers.ts:1592
	"GOLD_BRACELET_2":           11069, // Gold bracelet — ItemIdentifiers.ts:9721
	"GRAAHK_FUR":                10099, // Graahk fur — ItemIdentifiers.ts:8810
	"GREEN_DHIDE_BODY":          1135,  // Green d'hide body — ItemIdentifiers.ts:1070
	"GREEN_DHIDE_CHAPS":         1099,  // Green d'hide chaps — ItemIdentifiers.ts:1034
	"GREEN_DHIDE_VAMBRACES":     1065,  // Green d'hide vambraces — ItemIdentifiers.ts:1000
	"GREEN_DRAGON_LEATHER":      1745,  // Green dragon leather — ItemIdentifiers.ts:1658
	"GREEN_SPIKY_VAMBRACES":     10079, // Green spiky vambraces — ItemIdentifiers.ts:8790
	"HAIR":                      10814, // Hair — ItemIdentifiers.ts:9483
	"HARD_LEATHER_SHIELD":       22269, // Hard leather shield — ItemIdentifiers.ts:20551
	"HOLY_MOULD":                1599,  // Holy mould — ItemIdentifiers.ts:1532
	"HUEYCOATL_HIDE":            30085, // Hueycoatl hide — ItemIdentifiers.ts:27905
	"HUEYCOATL_HIDE_BODY":       30076, // Hueycoatl hide body — ItemIdentifiers.ts:27896
	"HUEYCOATL_HIDE_CHAPS":      30079, // Hueycoatl hide chaps — ItemIdentifiers.ts:27899
	"HUEYCOATL_HIDE_COIF":       30073, // Hueycoatl hide coif — ItemIdentifiers.ts:27893
	"HUEYCOATL_HIDE_VAMBRACES":  30082, // Hueycoatl hide vambraces — ItemIdentifiers.ts:27902
	"IMCANDO_HAMMER_OFF_HAND_":  29775, // Imcando hammer (off-hand) — ItemIdentifiers.ts:27611
	"IRON_NAILS":                4820,  // Iron nails — ItemIdentifiers.ts:4321
	"JADE_AMULET":               21111, // Jade amulet — ItemIdentifiers.ts:19458
	"JADE_AMULET_U_":            21102, // Jade amulet (u) — ItemIdentifiers.ts:19449
	"JUG":                       1935,  // Jug — ItemIdentifiers.ts:1848
	"JUG_OF_WATER":              1937,  // Jug of water — ItemIdentifiers.ts:1850
	"KEBBIT_CLAWS":              10113, // Kebbit claws — ItemIdentifiers.ts:8824
	"KYATT_FUR":                 10103, // Kyatt fur — ItemIdentifiers.ts:8814
	"LANTERN_LENS":              4542,  // Lantern lens — ItemIdentifiers.ts:4119
	"LARGE_FUR_POUCH":           29303, // Large fur pouch — ItemIdentifiers.ts:27173
	"LARGE_MEAT_POUCH":          29297, // Large meat pouch — ItemIdentifiers.ts:27167
	"LARUPIA_FUR":               10095, // Larupia fur — ItemIdentifiers.ts:8806
	"LEATHER_BODY":              1129,  // Leather body — ItemIdentifiers.ts:1064
	"LIGHT_ORB":                 10973, // Light orb — ItemIdentifiers.ts:9629
	"LIMESTONE_BRICK":           3420,  // Limestone brick — ItemIdentifiers.ts:3201
	"MEDIUM_FUR_POUCH":          29301, // Medium fur pouch — ItemIdentifiers.ts:27171
	"MITHRIL_NAILS":             4822,  // Mithril nails — ItemIdentifiers.ts:4323
	"MYRE_SNELM":                3327,  // Myre snelm — ItemIdentifiers.ts:3108
	"OCHRE_SNELM":               3331,  // Ochre snelm — ItemIdentifiers.ts:3112
	"ONYX_AMULET":               6581,  // Onyx amulet — ItemIdentifiers.ts:5816
	"ONYX_AMULET_U_":            6579,  // Onyx amulet (u) — ItemIdentifiers.ts:5814
	"ONYX_BRACELET":             11130, // Onyx bracelet — ItemIdentifiers.ts:9782
	"OPAL_AMULET":               21108, // Opal amulet — ItemIdentifiers.ts:19455
	"OPAL_AMULET_U_":            21099, // Opal amulet (u) — ItemIdentifiers.ts:19446
	"PERFECT_NECKLACE":          774,   // 'perfect' necklace — ItemIdentifiers.ts:772
	"PERFECT_RING":              773,   // 'perfect' ring — ItemIdentifiers.ts:771
	"PIE_DISH":                  2313,  // Pie dish — ItemIdentifiers.ts:2226
	"POINTED_BLOODNTAR_SNELM":   3339,  // Pointed blood'n'tar snelm — ItemIdentifiers.ts:3120
	"POINTED_BRUISE_BLUE_SNELM": 3343,  // Pointed bruise blue snelm — ItemIdentifiers.ts:3124
	"POINTED_MYRE_SNELM":        3337,  // Pointed myre snelm — ItemIdentifiers.ts:3118
	"POINTED_OCHRE_SNELM":       3341,  // Pointed ochre snelm — ItemIdentifiers.ts:3122
	"POT":                       1931,  // Pot — ItemIdentifiers.ts:1844
	"POT_LID":                   4440,  // Pot lid — ItemIdentifiers.ts:4025
	"POLISHED_BUTTONS":          10496, // Polished buttons — ItemIdentifiers.ts:9188
	"PRE_NATURE_AMULET":         6041,  // Pre-nature amulet — ItemIdentifiers.ts:5355
	"RED_DHIDE_BODY":            2501,  // Red d'hide body — ItemIdentifiers.ts:2412
	"RED_DHIDE_CHAPS":           2495,  // Red d'hide chaps — ItemIdentifiers.ts:2406
	"RED_DHIDE_VAMBRACES":       2489,  // Red d'hide vambraces — ItemIdentifiers.ts:2400
	"RED_DRAGON_LEATHER":        2507,  // Red dragon leather — ItemIdentifiers.ts:2418
	"RED_SPIKY_VAMBRACES":       10083, // Red spiky vambraces — ItemIdentifiers.ts:8794
	"ROD_MOULD":                 7649,  // Rod mould — ItemIdentifiers.ts:6801
	"RUBY_AMULET":               1698,  // Ruby amulet — ItemIdentifiers.ts:1611
	"RUBY_AMULET_U_":            1679,  // Ruby amulet (u) — ItemIdentifiers.ts:1598
	"RUBY_BRACELET":             11085, // Ruby bracelet — ItemIdentifiers.ts:9737
	"RUBY_SICKLE_B_":            24693, // Ruby sickle (b) — ItemIdentifiers.ts:22771
	"RUNE_NAILS":                4824,  // Rune nails — ItemIdentifiers.ts:4325
	"SAPPHIRE_AMULET":           1694,  // Sapphire amulet — ItemIdentifiers.ts:1607
	"SAPPHIRE_AMULET_U_":        1675,  // Sapphire amulet (u) — ItemIdentifiers.ts:1594
	"SAPPHIRE_BRACELET_2":       11072, // Sapphire bracelet — ItemIdentifiers.ts:9724
	"SILVER_BOLTS_UNF_":         9382,  // Silver bolts (unf) — ItemIdentifiers.ts:8180
	"SILVER_SICKLE_B_":          2963,  // Silver sickle (b) — ItemIdentifiers.ts:2836
	"SILVTHRILL_ROD_2":          7637,  // Silvthrill rod — ItemIdentifiers.ts:6789
	"SINEW":                     9436,  // Sinew — ItemIdentifiers.ts:8202
	"SLAYER_RING_8_":            11866, // Slayer ring (8) — ItemIdentifiers.ts:10331
	"SLAYER_RING_ETERNAL_":      21268, // Slayer ring (eternal) — ItemIdentifiers.ts:19615
	"SMALL_FUR_POUCH":           29299, // Small fur pouch — ItemIdentifiers.ts:27169
	"SMALL_MEAT_POUCH":          29295, // Small meat pouch — ItemIdentifiers.ts:27165
	"SNAKESKIN":                 6289,  // Snakeskin — ItemIdentifiers.ts:5584
	"SOFT_CLAY":                 1761,  // Soft clay — ItemIdentifiers.ts:1674
	"SPIKY_VAMBRACES":           10077, // Spiky vambraces — ItemIdentifiers.ts:8788
	"STEEL_NAILS":               1539,  // Steel nails — ItemIdentifiers.ts:1473
	"STEEL_STUDS":               2370,  // Steel studs — ItemIdentifiers.ts:2283
	"STRIP_OF_CLOTH":            3224,  // Strip of cloth — ItemIdentifiers.ts:3067
	"SUNLIGHT_ANTELOPE_FUR":     29177, // Sunlight antelope fur — ItemIdentifiers.ts:27047
	"TOPAZ_AMULET":              21114, // Topaz amulet — ItemIdentifiers.ts:19461
	"TOPAZ_AMULET_U_":           21105, // Topaz amulet (u) — ItemIdentifiers.ts:19452
	"UNBLESSED_SYMBOL":          1716,  // Unblessed symbol — ItemIdentifiers.ts:1629
	"UNFIRED_BOWL":              1791,  // Unfired bowl — ItemIdentifiers.ts:1704
	"UNFIRED_CUP":               28193, // Unfired cup — ItemIdentifiers.ts:26120
	"UNFIRED_PIE_DISH":          1789,  // Unfired pie dish — ItemIdentifiers.ts:1702
	"UNFIRED_PLANT_POT":         5352,  // Unfired plant pot — ItemIdentifiers.ts:4666
	"UNFIRED_POT":               1787,  // Unfired pot — ItemIdentifiers.ts:1700
	"UNFIRED_POT_LID":           4438,  // Unfired pot lid — ItemIdentifiers.ts:4023
	"UNHOLY_MOULD":              1594,  // Unholy mould — ItemIdentifiers.ts:1527
	"UNPOWERED_ORB":             567,   // Unpowered orb — ItemIdentifiers.ts:566
	"UNPOWERED_SYMBOL":          1722,  // Unpowered symbol — ItemIdentifiers.ts:1635
	"UNSTRUNG_EMBLEM":           1720,  // Unstrung emblem — ItemIdentifiers.ts:1633
	"UNSTRUNG_SYMBOL":           1714,  // Unstrung symbol — ItemIdentifiers.ts:1627
	"VIAL":                      229,   // Vial — ItemIdentifiers.ts:228
	"VIAL_OF_WATER":             227,   // Vial of water — ItemIdentifiers.ts:226
	"XERICIAN_FABRIC":           13383, // Xerician fabric — ItemIdentifiers.ts:11829
	"XERICIAN_HAT":              13385, // Xerician hat — ItemIdentifiers.ts:11831
	"XERICIAN_ROBE":             13389, // Xerician robe — ItemIdentifiers.ts:11835
	"XERICIAN_TOP":              13387, // Xerician top — ItemIdentifiers.ts:11833
	"ZENYTE_AMULET_U_":          19501, // Zenyte amulet (u) — ItemIdentifiers.ts:17930
	"ZENYTE_BRACELET_2":         19532, // Zenyte bracelet — ItemIdentifiers.ts:17960
}
