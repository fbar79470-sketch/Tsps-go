// Fallback item ids for the ItemIdentifiers names in
// data/definitions/smithing.json that the rev-242 cache has no
// gameval for.
//
// WHY THIS TABLE EXISTS. The JSON names items by their
// ItemIdentifiers (UPPER_SNAKE) names and Load resolves them
// through the cache gamevals (lowercase form, the
// Fletching/Cooking pattern). Five names have no (correct)
// gameval: BLURITE_BOLTS_UNF_ and BLURITE_LIMBS are
// "xbows_crossbow_bolts_blurite_unfeathered" /
// "xbows_crossbow_limbs_blurite" in the cache, BUCKET_OF_SAND
// is "bucket_sand", BUCKET (the molten-glass byproduct) has no
// "bucket" gameval at all, and there is no steel_cannonball
// gameval at all — and no steel cannonball item in the
// 31906-31916 range (bronze 31906, iron 31908, mithril 31910,
// adamant 31912, rune 31914; 31916 dragon, 31922 steel
// chainshot, 31936 steel incendiary). There is no systematic
// transform from ItemIdentifiers name to gameval name, so
// these five resolve through this table instead — the
// fletching itemIDFallbacks / cooking itemIDConstants
// precedent. (The plan's probe counted 4 missing names; it
// did not cover the molten-glass byproduct BUCKET, which the
// load-prove test caught.)
//
// GROUNDING. Every id below is transcribed from
// server/src/main/typescript/elvarg/util/ItemIdentifiers.ts
// (upstream commit bfd81e3, itself generated from this cache),
// and every id's display name was verified against the real
// rev-242 cache in the load-prove test. No id here is invented
// or guessed. The loader consults this table FIRST, before the
// gameval lookup, so a name in neither the table nor the cache
// fails Load listing every missing name.
//
// STEEL_CANNONBALL. ItemIdentifiers.ts:5 sets it to 2 —
// item 2 IS the Cannonball (the standard cannonball the
// Dwarf Cannon fires; steel is the only cannonball kind in
// game). TS resolves the steel cannonball entry through
// ItemIdentifiers, not the cache, so Go keeps the entry with
// BallID 2, matching TS load behavior. The entry is NOT
// dropped: the constant exists, so the plan's drop branch
// does not apply.
package smithing

var itemIDFallbacks = map[string]int{
	"BLURITE_BOLTS_UNF_": 9376, // Blurite bolts (unf) — ItemIdentifiers.ts:8174
	"BLURITE_LIMBS":      9422, // Blurite limbs — ItemIdentifiers.ts:8188
	"BUCKET":             1925, // Bucket — ItemIdentifiers.ts:1838
	"BUCKET_OF_SAND":     1783, // Bucket of sand — ItemIdentifiers.ts:1696
	"STEEL_CANNONBALL":   2,    // Steel cannonball — ItemIdentifiers.ts:5; item 2 is the Cannonball
}
