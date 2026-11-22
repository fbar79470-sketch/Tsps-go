package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/crafting"
)

// installCraftingTables resolves the Crafting content tables
// against the loaded cache definitions and installs them on
// the registered Crafting plugin, plus the members-world flag
// and the object-name resolver. It runs before hub Load (see
// newPluginHub): Register captures the shared instance, so a
// call after Load is too late for the tables. A table load
// failure fails the boot, matching loadDefinitions: a cache
// whose gameval names do not resolve is a config error, not a
// degradation. With nil defs (NoCache mode) the tables stay
// uninstalled and every Crafting handler degrades to a no-op,
// the same no-definitions degradation as the rest of the
// server; the members-world flag is still installed, as it does
// not depend on the cache.
//
// The members halves of the TS checks arrive here too
// (Shared.Crafting.js available): the world definition's
// MembersWorld flag (the cooking SetMembersWorld precedent).
// Crafting is F2P (Members: false on the plugin) — the flag
// only gates members recipes, as TS does it. The object-name
// resolver DOES belong here: installCraftingTables installs it
// from the loaded definitions (the smithing SetObjectNameResolver
// precedent), so the item-on-object router can match furnace,
// wheel, loom and oven names; without it the router degrades to
// a no-op (the Phase 43 live-found gap — do NOT repeat it; the
// crafting loader test fails when the resolver is missing).
func installCraftingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	if worldDef != nil {
		crafting.SetMembersWorld(worldDef.MembersWorld)
	}
	if defs == nil {
		return nil
	}
	tables, err := crafting.LoadFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading crafting tables: %w", err)
	}
	crafting.SetTables(tables)
	crafting.SetObjectNameResolver(func(id int) string {
		def := defs.GetObject(id)
		if def == nil {
			return ""
		}
		return def.Name
	})
	return nil
}
