package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/mining"
)

// installMiningTables resolves the Mining content tables against
// the loaded cache definitions and installs them on the
// registered Mining plugin. It runs before hub Load (see
// newPluginHub): Register captures the shared instance and
// builds its object registrations from the tables, so a call
// after Load is too late for them. A table load failure fails
// the boot, matching loadDefinitions: a cache whose gameval
// names do not resolve is a config error, not a degradation.
// With nil defs (NoCache mode) the tables stay uninstalled and
// every Mining handler degrades to a no-op, the same
// no-definitions degradation as the rest of the server; the
// members-world flag is still installed, as it does not depend
// on the cache.
//
// The members halves of the TS checks arrive here too
// (Mining.plugin.js:497, 515): the world definition's
// MembersWorld flag, and an isMembers lookup closed over the
// loaded definitions (CacheDefinitions.getItem(id)?.isMembers).
func installMiningTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	if worldDef != nil {
		mining.SetMembersWorld(worldDef.MembersWorld)
	}
	if defs == nil {
		return nil
	}
	tables, err := mining.LoadFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading mining tables: %w", err)
	}
	mining.SetTables(tables)
	mining.SetIsMembersItem(func(id int) bool {
		item := defs.GetItem(id)
		return item != nil && item.IsMembers
	})
	return nil
}
