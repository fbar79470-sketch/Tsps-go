package main

import (
	"fmt"
	"path/filepath"

	"tsps-go/internal/cache"
	"tsps-go/internal/session"
)

// loadedDefinitions bundles everything boot-time definition loading
// produces from the ensured cache: the open store (kept for the
// server lifetime — the facades read through it lazily), the three
// definition facades, and the region XTEA keys. It is nil in
// NoCache mode, where no cache directory exists.
type loadedDefinitions struct {
	store    *cache.Store
	defs     *cache.CacheDefinitions
	maps     *cache.CacheMaps
	gamevals *cache.Gamevals
	keys     cache.Keys
}

// loadDefinitions opens the ensured cache directory and constructs
// the definition facades over it: CacheDefinitions (the decoded
// NPC/item/object/animation loaders), CacheMaps (region terrain and
// object data), and Gamevals (the cache's own id names). It mirrors
// the first CacheDefinitions.getState() call in TS, which builds the
// same loaders from the CachePipeline store
// (CacheDefinitions.ts:104-160), except that here the construction
// happens once at boot instead of lazily on first use, so a corrupt
// cache fails the boot instead of the first login.
//
// The XTEA keys are reloaded from keys.json: EnsureCache already
// validated the file exists and parses, but the pipeline does not
// hand the parsed keys back (the TS version keeps them in a
// module-level map; this port keeps no global mutable state), so
// this step reads them again for CacheMaps and the world adapter.
//
// A failure at any step fails the boot with a wrapped error: when a
// cache is present but unreadable, starting anyway would serve
// empty or wrong definitions silently (every definition lookup
// would answer a default object), which is worse than refusing to
// start. The store is closed on any failure after it opens; on
// success the caller owns it for the server lifetime and closes it
// in Server.Stop.
func loadDefinitions(active *cache.ActiveCache) (*loadedDefinitions, error) {
	store, err := cache.OpenStore(active.Directory)
	if err != nil {
		return nil, fmt.Errorf("opening cache store: %w", err)
	}
	keys, err := cache.LoadKeys(filepath.Join(active.Directory, "keys.json"))
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("loading XTEA keys: %w", err)
	}
	defs, err := cache.NewCacheDefinitionsFromStore(cache.CacheInfoForActive(active), store)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("loading cache definitions: %w", err)
	}
	maps, err := cache.NewCacheMapsFromStore(store, keys)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("loading cache maps: %w", err)
	}
	gamevals, err := cache.NewGamevalsFromStore(store)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("loading gamevals: %w", err)
	}
	return &loadedDefinitions{store: store, defs: defs, maps: maps, gamevals: gamevals, keys: keys}, nil
}

// itemWeights adapts CacheDefinitions to session.ItemWeightSource:
// the definition half of Player.getCarriedWeightGrams
// (Player.ts:1287-1311). The zero value (nil defs) answers 0 for
// every id, but production wiring only installs it when
// definitions loaded; LoginDeps.ItemWeights stays nil in NoCache
// mode.
type itemWeights struct {
	defs *cache.CacheDefinitions
}

func (w itemWeights) ItemWeightGrams(id int) int {
	if w.defs == nil || id < 0 {
		return 0
	}
	return objWeightGrams(w.defs.GetItem(id))
}

// objWeightGrams returns the effective carried weight, in grams, of
// one occupied slot holding item: the definition weight, or 0 for
// stackable or noted items, mirroring the itemGrams closure in
// getCarriedWeightGrams (Player.ts:1290-1296):
//
//	definition.isStackable() || definition.isNoted() ? 0 : definition.getWeight()
//
// "Stackable" and "noted" are grounded in ItemDefinition's cache
// hydration (ItemDefinition.ts:146,149,155): stackable is
// cached.stackability === ObjStackability.ALWAYS (opcode 11 sets
// ALWAYS, opcode 160 sets NEVER, and the SOMETIMES default counts
// as NOT stackable — only ALWAYS weighs nothing), noted is
// cached.noteTemplate !== -1 (opcode 98), and weight is
// cached.weight (opcode 75, a signed short, so graceful-style
// negative weights pass through unchanged).
//
// A nil item answers 0. In practice GetItem never returns nil (the
// loaders answer unknown ids with a default-initialized ObjType,
// whose weight is 0), so a missing id also contributes 0 through
// the weight field; the nil guard covers only a loader that breaks
// that contract. That differs from TS for out-of-range ids, where
// ItemDefinition.forId returns the DEFAULT definition whose weight
// is undefined and the total becomes NaN; returning 0 is the
// deliberate hardening, matching what an empty slot contributes.
func objWeightGrams(item *cache.ObjType) int {
	if item == nil {
		return 0
	}
	if item.Stackability == cache.ObjStackabilityAlways || item.NoteTemplate != -1 {
		return 0
	}
	return item.Weight
}

// Ensure itemWeights satisfies the session interface at compile time.
var _ session.ItemWeightSource = itemWeights{}
