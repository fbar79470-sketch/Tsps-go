package main

import (
	"log"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
	// Compiled-in content packages. Each blank import runs the
	// package's init, which registers its plugin value with the
	// hub's default registry (plugins.Register). Adding a ported
	// content piece to the server is adding one import line here;
	// removing one is deleting it. There is no file discovery: Go
	// has no dynamic plugin loading, so registration is
	// compile-time by design (see internal/plugins/doc.go).
	_ "tsps-go/internal/content/admincommands"
	_ "tsps-go/internal/content/cooking"
	_ "tsps-go/internal/content/dateminutes"
	_ "tsps-go/internal/content/fishing"
	_ "tsps-go/internal/content/fletching"
	_ "tsps-go/internal/content/gracefuloutfit"
	_ "tsps-go/internal/content/lumbridgeguide"
	_ "tsps-go/internal/content/seekingarrows"
	_ "tsps-go/internal/content/woodcutting"
	_ "tsps-go/internal/content/wornweights"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// newPluginHub builds the plugin hub from the compiled-in content
// registry and the world definition, and loads it: dependency
// ordering, config-declaration validation, switches, and
// members-gating all run inside Load, in TS loadFromDirectory
// order (PluginManager.ts:681-793). It runs before the world is
// built, as TS loads plugins before GameBuilder constructs the
// game; plugin Register functions are attach-only, so nothing
// they do needs the world to exist yet. A nil worldDef loads with
// no switches and no plugin config (tests only).
//
// defs carries the loaded cache definitions (nil in NoCache mode
// and in tests): content tables that resolve gameval names
// against the cache — Woodcutting's — must be installed before
// Load (see installWoodcuttingTables). A table load failure
// fails the boot. Callers pass the defs loaded by
// loadDefinitions, which buildServer runs before this call.
//
// The legacy whole-list disabledPlugins form is NOT honored: the
// config package does not carry it (see config.World), so there
// is nothing to feed HubOptions.DefaultEnabled with. The hub
// supports the form (plugins.ParseDisabledPlugins) for when a
// future config change surfaces it.
func newPluginHub(worldDef *config.World, logger *log.Logger, defs *cache.CacheDefinitions) (*plugins.Hub, error) {
	opts := plugins.HubOptions{Logger: logger}
	if worldDef != nil {
		opts.Switches = worldDef.Plugins
		opts.PluginConfig = worldDef.PluginConfig
		opts.MembersWorld = worldDef.MembersWorld
	}
	hub := plugins.NewHub(plugins.DefaultPlugins(), opts)
	// Engine-consumed attribute keys register before plugin loads
	// (see attributewiring.go): content ports of MakeOverMage /
	// Settings re-register the same keys with the same defaults.
	registerEngineAttributeKeys(logger)
	if err := installWoodcuttingTables(worldDef, defs); err != nil {
		return nil, err
	}
	if err := installFletchingTables(worldDef, defs); err != nil {
		return nil, err
	}
	if err := installFishingTables(worldDef, defs); err != nil {
		return nil, err
	}
	if err := installCookingTables(worldDef, defs); err != nil {
		return nil, err
	}
	hub.Load()
	return hub, nil
}

// installPluginWorldSeams wires the hub to the built world: the
// cycle source Core.Cycle() reads, and the per-tick player-process
// emit behind WorldHooks.PlayerProcess (TS: emitPlayerProcess,
// World.ts:1372). The emit's event carries no cycle, as in TS;
// content reads the cycle through Core.Cycle().
func installPluginWorldSeams(hub *plugins.Hub, world *game.World) {
	if hub == nil || world == nil {
		return
	}
	hub.SetCycleSource(world.ProcessCycle)
	world.Hooks.PlayerProcess = func(p *game.Player, cycle int64) {
		_ = cycle // See above: the event carries no cycle.
		hub.EmitPlayerProcess(&plugins.PlayerProcessEvent{Player: p})
	}
}
