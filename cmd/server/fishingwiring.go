package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/fishing"
	"tsps-go/internal/game"
)

// installFishingTables resolves the Fishing content tables
// against the loaded cache definitions and installs them on
// the registered Fishing plugin, plus Worker C's resolved ids
// (LoadIDs). It runs before hub Load (see newPluginHub):
// Register captures the shared instance and builds its spot
// registrations from the tables, so a call after Load is too
// late for them. A table or ids load failure fails the boot,
// matching loadDefinitions: a cache whose gameval names do
// not resolve is a config error, not a degradation. With nil
// defs (NoCache mode) the tables stay uninstalled and every
// Fishing handler degrades to a no-op, the same
// no-definitions degradation as the rest of the server; the
// members-world flag is still installed, as it does not
// depend on the cache.
//
// The members halves of the TS checks arrive here too
// (Fishing.plugin.js:686, 939): the world definition's
// MembersWorld flag, and an isMembers lookup closed over the
// loaded definitions (CacheDefinitions.getItem(id)?.isMembers).
// Fishing is F2P (Members: false on the plugin) — the flag
// only gates members tools and members fish, as TS does it.
func installFishingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	if worldDef != nil {
		fishing.SetMembersWorld(worldDef.MembersWorld)
	}
	if defs == nil {
		return nil
	}
	tables, err := fishing.LoadFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading fishing tables: %w", err)
	}
	fishing.SetTables(tables)
	ids, err := fishing.LoadIDsFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading fishing ids: %w", err)
	}
	fishing.SetIDs(ids)
	fishing.SetIsMembersItem(func(id int) bool {
		item := defs.GetItem(id)
		return item != nil && item.IsMembers
	})
	return nil
}

// installFishingWilderness installs the world's Wilderness on
// the Fishing content package, for the imbued-ring clue-bottle
// doubling check (cluebottles.go: hasWildernessRing). It runs
// in buildServer right after newAreaManagerAndWilderness —
// the Wilderness does not exist until then (it is built after
// hub Load, once the RegionManager is installed), and the
// handle is only read at catch time on the tick goroutine, so
// installing it here, before the engine starts, is in time.
// A nil Wilderness reads as not in the Wilderness, so the
// ring never doubles without one.
func installFishingWilderness(w *game.Wilderness) {
	fishing.SetWilderness(w)
}
