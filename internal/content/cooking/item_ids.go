package cooking

// knifeIDFallbacks maps the KNIFE_NAMES the cache has no
// gameval for to their ids.
//
// WHY THIS TABLE EXISTS. Common.Cooking.js knives() reads the
// names through core.ItemIds — the generated ItemIdentifiers
// table (UPPER_SNAKE), not the cache gamevals. Only "KNIFE"
// resolves through the cache gamevals at all ("knife" → 946);
// KNIFE_2..KNIFE_5 have no gameval of that form (probe,
// 2026-11-20, rev-242 seed cache: GetObjId("knife_2") …
// ("knife_5") all answer not-found). There is no systematic
// transform from ItemIdentifiers name to gameval name, so
// these 4 names resolve through this table instead — the
// fletching itemIDFallbacks precedent.
//
// GROUNDING. Every id below is transcribed from
// server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (upstream commit bfd81e3, itself generated from this cache),
// and every id's display name was verified against the real
// rev-242 cache in the load-prove test (all four read
// "Knife"). No id here is invented or guessed. The loader
// consults this table FIRST, before the gameval lookup, so a
// name in neither the table nor the cache is filtered out by
// knives(), exactly as TS filters a missing ItemIds entry.
var knifeIDFallbacks = map[string]int{
	"KNIFE_2": 947,   // Knife
	"KNIFE_3": 5605,  // Knife
	"KNIFE_4": 14293, // Knife
	"KNIFE_5": 28413, // Knife
}

// itemIDConstants maps the ItemIds names TS reads (not cache
// gamevals) to their numeric ids, transcribed from
// server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (upstream commit bfd81e3, itself generated from this cache).
// The rev-242 cache has NO gamevals of these names (probe,
// 2026-11-21: GetObjId("poison_karambwan"),
// GetObjId("cooking_gauntlets"), GetObjId("sacred_eel") and
// GetObjId("zulrahs_scales") all answer not-found), so any code
// that resolved them as gamevals failed closed —
// LoadCookerIDsFromCache failed the boot outright, and
// SacredEel could never match its eel. TS reads all four
// through core.ItemIds, so Go reads them from this table
// instead. Every id's display name was verified against the
// real rev-242 cache in the load-prove test.
var itemIDConstants = map[string]int{
	"POISON_KARAMBWAN":  3146,  // ItemIdentifiers.ts:2989, Poison karambwan
	"COOKING_GAUNTLETS": 775,   // ItemIdentifiers.ts:773, Cooking gauntlets
	"SACRED_EEL":        13339, // ItemIdentifiers.ts:11785, Sacred eel
	"ZULRAHS_SCALES":    12934, // ItemIdentifiers.ts:11398, Zulrah's scales
}
