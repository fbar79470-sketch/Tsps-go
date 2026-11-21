package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/woodcutting"
)

// installWoodcuttingTables resolves the Woodcutting content
// tables against the loaded cache definitions and installs
// them on the registered Woodcutting plugin. It runs before
// hub Load (see newPluginHub): Register captures the shared
// instance and builds its 27 object registrations from the
// tables, so a call after Load is too late for them. A table
// load failure fails the boot, matching loadDefinitions: a
// cache whose gameval names do not resolve is a config error,
// not a degradation. With nil defs (NoCache mode) the tables
// stay uninstalled and every Woodcutting handler degrades to
// a no-op, the same no-definitions degradation as the rest of
// the server; the members-world flag is still installed, as
// it does not depend on the cache.
//
// The members halves of the TS checks arrive here too
// (Woodcutting.plugin.js:1218, 1505): the world definition's
// MembersWorld flag, and an isMembers lookup closed over the
// loaded definitions (CacheDefinitions.getItem(id)?.isMembers).
func installWoodcuttingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	if worldDef != nil {
		woodcutting.SetMembersWorld(worldDef.MembersWorld)
	}
	if defs == nil {
		return nil
	}
	tables, err := woodcutting.LoadFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading woodcutting tables: %w", err)
	}
	woodcutting.SetTables(tables)
	woodcutting.SetIsMembersItem(func(id int) bool {
		item := defs.GetItem(id)
		return item != nil && item.IsMembers
	})
	return nil
}
