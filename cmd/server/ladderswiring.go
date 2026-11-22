package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/content/ladders"
	"tsps-go/internal/game"
)

// installLaddersMechanism builds the climb mechanism from the loaded
// cache definitions and the world's object registry, region manager
// and task manager, and installs it on the Ladders content package.
// Called after the RegionManager is installed (the Blocked/Reach
// adapters capture that manager — SetRegionManager swaps the
// pointer, so an earlier install would hold the empty open-ground
// manager) and before the engine starts. The plugin itself loads
// earlier (hub Load, before the world exists); the mechanism is
// consulted only per click, so a post-Load install is the designed
// order (ladders.go: SetMechanism "called once at boot"). With nil
// defs (NoCache mode) the Defs adapter reads empty and every climb
// falls through — the same no-definitions degradation as the rest
// of the server (doors' installDoorsCatalog precedent).
func installLaddersMechanism(world *game.World, defs *cache.CacheDefinitions) {
	if world == nil {
		return
	}
	resolver := &ladders.Resolver{
		Defs:    cacheLadderDefs{defs: defs},
		Objects: ladders.RegistryObjects(world.GameObjects()),
		Blocked: ladders.RegionBlocked(world.RegionManager()),
		Reached: ladders.RegionReached(world.RegionManager()),
	}
	ladders.SetMechanism(ladders.NewMechanism(resolver, world.Tasks()))
}

// cacheLadderDefs adapts *cache.CacheDefinitions to the ladders.Defs
// surface: Name/Actions/SizeX/SizeY/ClipMask straight off the loc
// type (internal/cache/loc.go). A nil defs reads as an empty cache.
type cacheLadderDefs struct {
	defs *cache.CacheDefinitions
}

func (a cacheLadderDefs) Def(id int) (ladders.ObjectDef, bool) {
	if a.defs == nil {
		return ladders.ObjectDef{}, false
	}
	loc := a.defs.GetObject(id)
	if loc == nil {
		return ladders.ObjectDef{}, false
	}
	return ladders.ObjectDef{
		Name:     loc.Name,
		Actions:  append([]string(nil), loc.Actions...),
		SizeX:    loc.SizeX,
		SizeY:    loc.SizeY,
		ClipMask: loc.ClipMask,
	}, true
}
