package main

import (
	"fmt"

	"tsps-go/internal/cache"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
)

// This file wires Phase 6 boot loading: custom-definition overlays,
// JSON definition loading, and spawn application. It is called
// from buildServer (server.go), after the cache-backed
// RegionManager is installed on the world and before the engine
// starts.
//
// ORDER. TypeScript boots CachePipeline, then loads plugins
// (whose register() calls apply the cache-editor overlay document
// and register custom items), then GameBuilder.loadStartupData
// runs the spawn loaders (Server.ts:279-290). The wiring follows
// that order: overlays first, so spawn validation inside
// definition.Load sees the extended definition counts (a spawn
// naming a custom NPC id must not be rejected as unsupported),
// then the definition load, then spawn application. Applying
// object spawns after the RegionManager is installed matters:
// SpawnObjects lays clipping on the world's manager, and an
// earlier call would clip the placeholder empty manager that
// SetRegionManager then replaces.
//
// FAILURE SEMANTICS. definition.Load encapsulates the per-file
// behavior (fatal vs warning vs default); a non-nil error from it
// is boot-fatal and is returned unchanged, never downgraded to a
// warning, and a successful load — even one that loaded zero
// spawns or logged warnings — is never upgraded to a fatal. The
// same holds for overlay registration: a malformed custom file
// or an invalid overlay row fails the boot, as the TypeScript
// throws do. A nil Store with a nil error is treated as an empty
// load, never a crash: the wiring guards every Store access.
//
// GROUND ITEMS. Ground-item spawns load with the Store and apply
// in buildServer (see grounditemswiring.go applyGroundItemSpawns),
// after this load returns: the manager needs the loaded Store for
// its definition-backed sources. Their count is logged here with
// the other spawn counts. Object spawns apply their clipping
// only; the object-registry / visibility half is an explicit gap
// documented in internal/game/spawns.go.

// loadGameDefinitions loads the definitions directory dir
// (relative to the data root, the same convention
// config.LoadWorld uses for data/definitions/world.json),
// applies custom overlays, and spawns NPCs and objects into
// world. defs and gamevals are the boot-loaded facades, or nil in
// NoCache mode. It returns the loaded Store (possibly nil when
// the load produced none) for callers that need the definitions
// after boot. logf reports the loaded counts; a nil logf
// suppresses the summary (the NoCache overlay note is suppressed
// with it).
func loadGameDefinitions(dir string, world *game.World, defs *cache.CacheDefinitions, gamevals *cache.Gamevals, logf func(format string, args ...any)) (*definition.Store, error) {
	// Overlays first: see the ORDER note above. In NoCache mode
	// this is a no-op that logs a note (cache.ApplyCustomOverlays).
	overlays, err := cache.ApplyCustomOverlays(dir, defs, logf)
	if err != nil {
		return nil, fmt.Errorf("applying custom definitions: %w", err)
	}

	store, err := definition.Load(dir, definition.LoadOptions{Cache: defs, Gamevals: gamevals, Logf: logf})
	if err != nil {
		return nil, fmt.Errorf("loading definitions: %w", err)
	}

	var npcSpawns []definition.NpcSpawn
	var objectSpawns []definition.ObjectSpawn
	var objectRemovals []definition.ObjectSpawn
	var groundItemSpawns []definition.GroundItemSpawn
	if store != nil {
		npcSpawns = store.NPCSpawns()
		objectSpawns = store.ObjectSpawns()
		objectRemovals = store.ObjectSpawnRemovals()
		groundItemSpawns = store.GroundItemSpawns()
	}
	game.SpawnNPCsWithHitpointsAndDefinitions(world, npcSpawns, npcHitpointsSource(store), npcDefinitionSource(store))
	game.SpawnObjects(world, objectSpawns)
	game.SpawnObjectRemovals(world, objectRemovals)
	// Ground-item spawns apply in buildServer, after this load
	// returns (see the file header); their count is logged here
	// with the other spawn counts.
	if logf != nil {
		logf("[definitions] loaded %d npc spawns, %d object spawns, %d object removals, %d ground-item spawns, %d custom definitions, %d custom items",
			len(npcSpawns), len(objectSpawns), len(objectRemovals), len(groundItemSpawns), overlays.Definitions, overlays.Items)
	}
	return store, nil
}

// npcHitpointsSource returns the hitpoints source for spawned
// NPCs: the loaded Store's NpcDefinition hitpoints
// (Store.NPC, mirroring NpcDefinition.forId), which carries the
// full TypeScript chain — the JSON monsters-dump override when
// present, else the cache NpcType hitpoints when positive, else
// the default 10 (NpcDefinition.ts:52,116;
// NpcDefinitionLoader). Store.NPC never returns nil: out-of-range
// ids and NoCache boots yield the DEFAULT shape (10). A nil
// Store yields a nil source, and every NPC gets the default.
func npcHitpointsSource(store *definition.Store) game.NPCHitpoints {
	if store == nil {
		return nil
	}
	return func(npcID int) int {
		return store.NPC(npcID).Hitpoints()
	}
}

// npcDefinitionSource returns the NPC definition source for
// spawned NPCs: the loaded Store's NpcDefinition (Store.NPC,
// mirroring NpcDefinition.forId), which *definition
// .NpcDefinition satisfies directly as a game.NPCDefinition.
// Store.NPC never returns nil. A nil store yields a nil
// source: spawned NPCs keep the DEFAULT definition shape.
func npcDefinitionSource(store *definition.Store) game.NPCDefinitionSource {
	if store == nil {
		return nil
	}
	return func(npcID int) game.NPCDefinition {
		return store.NPC(npcID)
	}
}
