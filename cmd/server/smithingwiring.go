package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/smithing"
)

// installSmithingTables resolves the Smithing content tables
// against the loaded cache definitions and installs them on
// the registered Smithing plugin. It runs before hub Load (see
// newPluginHub): Register builds indexes from the tables, so a
// call after Load is too late for them. A table load failure
// fails the boot, matching loadDefinitions: a cache whose
// gameval names do not resolve is a config error, not a
// degradation. With nil defs (NoCache mode) the tables stay
// uninstalled and every Smithing handler degrades to a no-op,
// the same no-definitions degradation as the rest of the
// server.
//
// The world definition carries no Smithing table-level state:
// the members gating TS does (Smelting.Smithing.js
// smeltMenuRecipes filters members recipes by world; anvil
// members slots carry their own flag in the tables) is content
// behavior the Smithing units apply at use time, so no
// SetMembersWorld / SetIsMembersItem call belongs here — the
// Fletching precedent (fletchingwiring.go). The object-name
// resolver DOES belong here: installSmithingTables installs it
// from the loaded definitions (the cooking SetObjectInfo
// precedent), so the item-on-object router can match furnace
// and anvil names; without it the router degrades to a no-op
// (gap 26f). worldDef stays in the signature for the shared
// install shape.
func installSmithingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	_ = worldDef
	if defs == nil {
		return nil
	}
	tables, err := smithing.LoadFromCache(defs)
	if err != nil {
		return fmt.Errorf("loading smithing tables: %w", err)
	}
	smithing.SetTables(tables)
	smithing.SetObjectNameResolver(func(id int) string {
		def := defs.GetObject(id)
		if def == nil {
			return ""
		}
		return def.Name
	})
	return nil
}
