package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	"tsps-go/internal/content/fletching"
)

// installFletchingTables resolves the Fletching content tables
// against the loaded cache definitions and installs them on
// the registered Fletching plugin. It runs before hub Load (see
// newPluginHub): Register captures the shared instance and
// builds its pair index from the tables, so a call after Load
// is too late for them. A table load failure fails the boot,
// matching loadDefinitions: a cache whose gameval names do not
// resolve is a config error, not a degradation. With nil defs
// (NoCache mode) the tables stay uninstalled and every
// Fletching handler degrades to a no-op, the same
// no-definitions degradation as the rest of the server.
//
// The world definition carries no Fletching item-level state:
// unlike Woodcutting (members axes, members rations),
// Fletching has no item-level members check anywhere in its
// TS — the gate is plugin-level only (Plugin.Members, dropped
// by the hub on a free world), so no SetMembersWorld /
// SetIsMembersItem call belongs here. worldDef stays in the
// signature for the shared install shape.
func installFletchingTables(worldDef *config.World, defs *cache.CacheDefinitions) error {
	_ = worldDef
	if defs == nil {
		return nil
	}
	tables, err := fletching.LoadFromCache(defs, fletching.DefaultPath)
	if err != nil {
		return fmt.Errorf("loading fletching tables: %w", err)
	}
	fletching.SetTables(tables)
	return nil
}
