package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/cooking"
)

// installCookingTables resolves the Cooking content tables
// against the loaded cache definitions and installs them on
// the registered Cooking plugin, plus the Cookers ids
// (LoadCookerIDsFromCache), the item-members lookup and the
// object-info lookup. It runs before hub Load (see
// newPluginHub): Register captures the shared instance, so a
// call after Load is too late for the tables. A table or ids
// load failure fails the boot, matching loadDefinitions: a
// cache whose gameval names do not resolve is a config error,
// not a degradation. With nil defs (NoCache mode) the tables
// stay uninstalled and every Cooking handler degrades to a
// no-op, the same no-definitions degradation as the rest of
// the server; the members-world flag is still installed, as
// it does not depend on the cache.
//
// The members halves of the TS checks arrive here too
// (Cookers.Cooking.js availableOnWorld, Recipes.Cooking.js
// isMembersRecipe): the world definition's MembersWorld flag,
// and an isMembers lookup closed over the loaded definitions
// (CacheDefinitions.getItem(id)?.isMembers). Cooking is F2P
// (Members: false on the plugin) — the flag only gates members
// foods and members recipes, as TS does it. The object-info
// lookup closes over the loaded definitions too
// (CacheDefinitions.getObject(id): name, actions, footprint) —
// the item-on-object path has no event Definition to read.
func installCookingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	if worldDef != nil {
		cooking.SetMembersWorld(worldDef.MembersWorld)
	}
	if defs == nil {
		return nil
	}
	tables, err := cooking.LoadFromCache(defs, cooking.DefaultPath)
	if err != nil {
		return fmt.Errorf("loading cooking tables: %w", err)
	}
	cooking.SetTables(tables)
	ids, err := cooking.LoadCookerIDsFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading cooking ids: %w", err)
	}
	cooking.SetCookerIDs(ids)
	cooking.SetGamevalResolver(defs)
	cooking.SetIsMembersItem(func(id int) bool {
		item := defs.GetItem(id)
		return item != nil && item.IsMembers
	})
	cooking.SetObjectInfo(func(id int) (cooking.ObjectInfo, bool) {
		def := defs.GetObject(id)
		if def == nil {
			return cooking.ObjectInfo{}, false
		}
		return cooking.ObjectInfo{
			Name:         def.Name,
			Interactions: def.Actions,
			SizeX:        def.SizeX,
			SizeY:        def.SizeY,
		}, true
	})
	return nil
}
