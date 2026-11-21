package fletching

// itemIDFallbacks maps the fletching.json item names the
// cache has no (correct) gameval for to their ids.
//
// WHY THIS TABLE EXISTS. The JSON names items by their
// ItemIdentifiers names (UPPER_SNAKE). Only 102 of the
// 278 names the loader resolves (277 the file uses, plus
// BOW_STRING_SPOOL, which the spool unit needs) resolve
// through the cache gamevals at all (lowercased, e.g.
// "LOGS" -> "logs"), and 3 of those resolve to the WRONG
// item: the lowercase name is another item's gameval
// (OPAL_BOLTS -> "opal_bolt" is 3818, the enchanted
// opal bolts, not 879; PEARL_BOLTS -> 3822, not 880;
// BARBED_BOLTS -> 3826, not 881). The other 173 names
// have no gameval of that form at all — the cache
// gamevals are unrelated Jagex names (SHORTBOW_U_ is
// "unstrung_shortbow", BRONZE_BOLTS is "bolt",
// YELLOW_FEATHER is "hunting_desert_feather"). There is
// no systematic transform from ItemIdentifiers name to
// gameval name, so these 176 names resolve through this
// table instead (probe, 2026-11-20, rev-242 seed cache).
//
// GROUNDING. Every id below is transcribed from
// server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (upstream commit bfd81e3, itself generated from this
// cache), and every id's display name was verified
// against the real rev-242 cache in the probe — the same
// grounding the woodcutting ClueNests unit uses for ids
// outside the gameval tables. No id here is invented or
// guessed. The loader consults this table FIRST, before
// the gameval lookup, so the 3 wrong-gameval names can
// never resolve to the wrong item; a name in neither the
// table nor the cache still fails Load.
var itemIDFallbacks = map[string]int{
	"ADAMANTITE_LIMBS":          9429,  // Adamantite limbs
	"ADAMANTITE_NAILS":          4823,  // Adamantite nails
	"ADAMANT_ARROWTIPS":         43,    // Adamant arrowtips
	"ADAMANT_BOLTS":             9143,  // Adamant bolts
	"ADAMANT_BOLTS_UNF_":        9380,  // Adamant bolts(unf)
	"ADAMANT_BRUTAL":            4798,  // Adamant brutal
	"ADAMANT_CROSSBOW":          9183,  // Adamant crossbow
	"ADAMANT_CROSSBOW_U_":       9463,  // Adamant crossbow (u)
	"ADAMANT_JAVELIN_TIPS":      19578, // Adamant javelin tips
	"AMETHYST_ARROWTIPS":        21350, // Amethyst arrowtips
	"AMETHYST_BOLT_TIPS":        21338, // Amethyst bolt tips
	"AMETHYST_BROAD_BOLTS":      21316, // Amethyst broad bolts
	"AMETHYST_JAVELIN_TIPS":     21352, // Amethyst javelin tips
	"BARBED_BOLTS":              881,   // Barbed bolts
	"BARB_BOLTTIPS":             47,    // Barb bolttips
	"BLACK_BRUTAL":              4788,  // Black brutal
	"BLACK_NAILS":               4821,  // Black nails
	"BLUE_FEATHER":              10089, // Blue feather
	"BLURITE_BOLTS":             9139,  // Blurite bolts
	"BLURITE_BOLTS_UNF_":        9376,  // Blurite bolts (unf)
	"BLURITE_CROSSBOW":          9176,  // Blurite crossbow
	"BLURITE_CROSSBOW_U_":       9456,  // Blurite crossbow (u)
	"BLURITE_LIMBS":             9422,  // Blurite limbs
	"BOW_STRING_SPOOL":          31052, // Bow string spool
	"BROAD_ARROWHEADS":          11874, // Broad arrowheads
	"BROAD_ARROWS":              4150,  // Broad arrows
	"BROAD_BOLTS":               11875, // Broad bolts
	"BRONZE_ARROWTIPS":          39,    // Bronze arrowtips
	"BRONZE_BOLTS":              877,   // Bronze bolts
	"BRONZE_BOLTS_UNF_":         9375,  // Bronze bolts (unf)
	"BRONZE_BRUTAL":             4773,  // Bronze brutal
	"BRONZE_CROSSBOW":           9174,  // Bronze crossbow
	"BRONZE_CROSSBOW_U_":        9454,  // Bronze crossbow (u)
	"BRONZE_JAVELIN_TIPS":       19570, // Bronze javelin tips
	"BRONZE_LIMBS":              9420,  // Bronze limbs
	"BRONZE_NAILS":              4819,  // Bronze nails
	"CAMPHOR_BLOWPIPE_EMPTY_":   31577, // Camphor blowpipe (empty)
	"CELASTRUS_BARK":            22935, // Celastrus bark
	"COMP_OGRE_BOW":             4827,  // Comp ogre bow
	"CRAWS_BOW_U_":              22547, // Craw's bow (u)
	"CROSSBOW_STRING":           9438,  // Crossbow string
	"DIAMOND_BOLTS":             9340,  // Diamond bolts
	"DIAMOND_BOLT_TIPS":         9192,  // Diamond bolt tips
	"DIAMOND_DRAGON_BOLTS":      21969, // Diamond dragon bolts
	"DRAGONSTONE_BOLTS":         9341,  // Dragonstone bolts
	"DRAGONSTONE_BOLT_TIPS":     9193,  // Dragonstone bolt tips
	"DRAGONSTONE_DRAGON_BOLTS":  21971, // Dragonstone dragon bolts
	"DRAGON_ARROWTIPS":          11237, // Dragon arrowtips
	"DRAGON_BOLTS_2":            21905, // Dragon bolts
	"DRAGON_BOLTS_UNF_":         21930, // Dragon bolts (unf)
	"DRAGON_CROSSBOW":           21902, // Dragon crossbow
	"DRAGON_CROSSBOW_U_":        21921, // Dragon crossbow (u)
	"DRAGON_JAVELIN_TIPS":       19582, // Dragon javelin tips
	"DRAGON_LIMBS":              21918, // Dragon limbs
	"EMERALD_BOLTS":             9338,  // Emerald bolts
	"EMERALD_BOLT_TIPS":         9190,  // Emerald bolt tips
	"EMERALD_DRAGON_BOLTS":      21965, // Emerald dragon bolts
	"FANGS_OF_VENENATIS":        27670, // Fangs of Venenatis
	"FLIGHTED_OGRE_ARROW":       2865,  // Flighted ogre arrow
	"GREENMAN_CARVING":          31024, // Greenman carving
	"HEAVY_FRAME":               19589, // Heavy frame
	"HUNTERS_CROSSBOW":          10156, // Hunters' crossbow
	"HUNTERS_SPEAR":             29305, // Hunter's spear
	"HUNTERS_SUNLIGHT_CROSSBOW": 28869, // Hunters' sunlight crossbow
	"HUNTER_SPEAR_TIPS":         29311, // Hunter spear tips
	"INCOMPLETE_HEAVY_BALLISTA": 19598, // Incomplete heavy ballista
	"INCOMPLETE_LIGHT_BALLISTA": 19595, // Incomplete light ballista
	"IRONWOOD_BLOWPIPE_EMPTY_":  31581, // Ironwood blowpipe (empty)
	"IRON_ARROWTIPS":            40,    // Iron arrowtips
	"IRON_BOLTS":                9140,  // Iron bolts
	"IRON_BOLTS_UNF_":           9377,  // Iron bolts (unf)
	"IRON_BRUTAL":               4778,  // Iron brutal
	"IRON_CROSSBOW":             9177,  // Iron crossbow
	"IRON_CROSSBOW_U_":          9457,  // Iron crossbow (u)
	"IRON_JAVELIN_TIPS":         19572, // Iron javelin tips
	"IRON_LIMBS":                9423,  // Iron limbs
	"IRON_NAILS":                4820,  // Iron nails
	"JADE_BOLTS":                9335,  // Jade bolts
	"JADE_BOLT_TIPS":            9187,  // Jade bolt tips
	"JADE_DRAGON_BOLTS":         21957, // Jade dragon bolts
	"JERBOA_TAIL":               29166, // Jerboa tail
	"KEBBIT_BOLTS":              10158, // Kebbit bolts
	"KEBBIT_SPIKE":              10105, // Kebbit spike
	"LIGHT_FRAME":               19586, // Light frame
	"LONGBOW_U_":                48,    // Longbow (u)
	"LONG_KEBBIT_BOLTS":         10159, // Long kebbit bolts
	"LONG_KEBBIT_SPIKE":         10107, // Long kebbit spike
	"MAGIC_LONGBOW_U_":          70,    // Magic longbow (u)
	"MAGIC_SHORTBOW_U_":         72,    // Magic shortbow (u)
	"MAGIC_STOCK":               21952, // Magic stock
	"MAHOGANY_STOCK":            9450,  // Mahogany stock
	"MAPLE_LONGBOW_U_":          62,    // Maple longbow (u)
	"MAPLE_SHORTBOW_U_":         64,    // Maple shortbow (u)
	"MAPLE_STOCK":               9448,  // Maple stock
	"MITHRIL_ARROWTIPS":         42,    // Mithril arrowtips
	"MITHRIL_BOLTS":             9142,  // Mithril bolts
	"MITHRIL_BOLTS_UNF_":        9379,  // Mithril bolts (unf)
	"MITHRIL_BRUTAL":            4793,  // Mithril brutal
	"MITHRIL_CROSSBOW":          9181,  // Mithril crossbow
	"MITHRIL_CROSSBOW_U_":       9461,  // Mithril crossbow (u)
	"MITHRIL_JAVELIN_TIPS":      19576, // Mithril javelin tips
	"MITHRIL_LIMBS":             9427,  // Mithril limbs
	"MITHRIL_NAILS":             4822,  // Mithril nails
	"MITH_GRAPPLE":              9418,  // Mith grapple
	"MITH_GRAPPLE_2":            9419,  // Mith grapple
	"MITH_GRAPPLE_TIP":          9416,  // Mith grapple tip
	"MONKEY_TAIL":               19610, // Monkey tail
	"MOONLIGHT_ANTELOPE_ANTLER": 29171, // Moonlight antelope antler
	"MOONLIGHT_ANTLER_BOLTS":    28878, // Moonlight antler bolts
	"OAK_LONGBOW_U_":            56,    // Oak longbow (u)
	"OAK_SHORTBOW_U_":           54,    // Oak shortbow (u)
	"OAK_STOCK":                 9442,  // Oak stock
	"ONYX_BOLTS":                9342,  // Onyx bolts
	"ONYX_BOLT_TIPS":            9194,  // Onyx bolt tips
	"ONYX_DRAGON_BOLTS":         21973, // Onyx dragon bolts
	"OPAL_BOLTS":                879,   // Opal bolts
	"OPAL_BOLT_TIPS":            45,    // Opal bolt tips
	"OPAL_DRAGON_BOLTS":         21955, // Opal dragon bolts
	"ORANGE_FEATHER":            10091, // Orange feather
	"OYSTER_PEARL":              411,   // Oyster pearl
	"OYSTER_PEARLS":             413,   // Oyster pearls
	"PEARL_BOLTS":               880,   // Pearl bolts
	"PEARL_BOLT_TIPS":           46,    // Pearl bolt tips
	"PEARL_DRAGON_BOLTS":        21959, // Pearl dragon bolts
	"RED_FEATHER":               10088, // Red feather
	"ROSEWOOD_BLOWPIPE_EMPTY_":  31585, // Rosewood blowpipe (empty)
	"RUBY_BOLTS":                9339,  // Ruby bolts
	"RUBY_BOLT_TIPS":            9191,  // Ruby bolt tips
	"RUBY_DRAGON_BOLTS":         21967, // Ruby dragon bolts
	"RUNE_ARROWTIPS":            44,    // Rune arrowtips
	"RUNE_BRUTAL":               4803,  // Rune brutal
	"RUNE_CROSSBOW":             9185,  // Rune crossbow
	"RUNE_JAVELIN_TIPS":         19580, // Rune javelin tips
	"RUNE_NAILS":                4824,  // Rune nails
	"RUNITE_BOLTS":              9144,  // Runite bolts
	"RUNITE_BOLTS_UNF_":         9381,  // Runite bolts (unf)
	"RUNITE_CROSSBOW_U_":        9465,  // Runite crossbow (u)
	"RUNITE_LIMBS":              9431,  // Runite limbs
	"SAPPHIRE_BOLTS":            9337,  // Sapphire bolts
	"SAPPHIRE_BOLT_TIPS":        9189,  // Sapphire bolt tips
	"SAPPHIRE_DRAGON_BOLTS":     21963, // Sapphire dragon bolts
	"SHORTBOW_U_":               50,    // Shortbow (u)
	"SILVER_BOLTS":              9145,  // Silver bolts
	"SILVER_BOLTS_UNF_":         9382,  // Silver bolts (unf)
	"STEEL_ARROWTIPS":           41,    // Steel arrowtips
	"STEEL_BOLTS":               9141,  // Steel bolts
	"STEEL_BOLTS_UNF_":          9378,  // Steel bolts (unf)
	"STEEL_BRUTAL":              4783,  // Steel brutal
	"STEEL_CROSSBOW":            9179,  // Steel crossbow
	"STEEL_CROSSBOW_U_":         9459,  // Steel crossbow (u)
	"STEEL_JAVELIN_TIPS":        19574, // Steel javelin tips
	"STEEL_LIMBS":               9425,  // Steel limbs
	"STEEL_NAILS":               1539,  // Steel nails
	"STRIPY_FEATHER":            10087, // Stripy feather
	"SUNLIGHT_ANTELOPE_ANTLER":  29168, // Sunlight antelope antler
	"SUNLIGHT_ANTLER_BOLTS":     28872, // Sunlight antler bolts
	"TANZANITE_FANG":            12922, // Tanzanite fang
	"TEAK_STOCK":                9446,  // Teak stock
	"TOPAZ_BOLTS":               9336,  // Topaz bolts
	"TOPAZ_BOLT_TIPS":           9188,  // Topaz bolt tips
	"TOPAZ_DRAGON_BOLTS":        21961, // Topaz dragon bolts
	"TOXIC_BLOWPIPE_EMPTY_":     12924, // Toxic blowpipe (empty)
	"UNFINISHED_BROAD_BOLTS":    11876, // Unfinished broad bolts
	"UNSTRUNG_COMP_BOW":         4825,  // Unstrung comp bow
	"UNSTRUNG_HEAVY_BALLISTA":   19607, // Unstrung heavy ballista
	"UNSTRUNG_LIGHT_BALLISTA":   19604, // Unstrung light ballista
	"WEBWEAVER_BOW_U_":          27652, // Webweaver bow (u)
	"WILLOW_LONGBOW_U_":         58,    // Willow longbow (u)
	"WILLOW_SHORTBOW_U_":        60,    // Willow shortbow (u)
	"WILLOW_STOCK":              9444,  // Willow stock
	"WOLFBONE_ARROWTIPS":        2861,  // Wolfbone arrowtips
	"WOODEN_STOCK":              9440,  // Wooden stock
	"YELLOW_FEATHER":            10090, // Yellow feather
	"YEW_LONGBOW_U_":            66,    // Yew longbow (u)
	"YEW_SHORTBOW_U_":           68,    // Yew shortbow (u)
	"YEW_STOCK":                 9452,  // Yew stock
}
