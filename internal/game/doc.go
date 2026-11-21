// Package game is the foundation subset of the TSPS game world: the tick
// loop driver (Engine), the world cycle skeleton (World), entities
// (Entity/Mobile/Player/NPC), index allocation (MobileList), and tick-based
// tasks (TaskManager).
//
// This package is deliberately server-only and dependency-light: it imports
// only the standard library. It never imports the session, protocol, or
// persistence packages; outbound network access goes through the SessionPort
// interface, which the session package implements, and persistence is the
// coordinator's job (see the Engine's tick-error hook).
//
// Startup order (GameBuilder.ts, initialize/loadStartupData) — documented
// here only; the coordinator wires it, this package provides no builder:
//
//  1. Systems.init() — NPC implementation discovery.
//  2. RegionManager.init() — collision/region data.
//  3. Load startup data — sequence/enum preloads, poison data, punishments,
//     then definition loaders (NPC interactions, interface layouts, object
//     spawns, ground-item spawns, NPC definitions, NPC spawns, shops),
//     ShopManager.initialize(), Sailing.initialize().
//  4. GameEngine.init() — the tick loop starts.
//  5. PluginManager.emitServerStartup — plugins attach/start.
//
// All behavior in this package cites the TypeScript source file and line it
// was ported from. Where the TypeScript behavior depends on systems that do
// not exist yet in Go, the behavior is absent or an explicit hook — never a
// silent fake.
//
// Explicitly NOT STARTED in this package (do not mistake hooks for them):
// teleportation. Skills, containers
// (inventory/equipment/bank), appearance, ground items, timers,
// areas, dialogue, the object registry, shops, trading,
// duelling state, combat (including prayer), NPC drivers,
// boats/sailing, private areas, and template regions ARE
// ported in this package (see skills.go, container.go,
// appearance.go, grounditems.go, timers.go, areamanager.go,
// dialogue_manager.go, objects.go, shop_manager.go, trading.go,
// duelling.go, combat.go, npc_movement_coordinator.go,
// sailing.go, boat_manager.go, private_area.go,
// templated_instance_area.go); sync
// encoding lives in internal/protocol and persistence in
// internal/persist. Movement IS ported: MovementQueue + WalkRoute /
// RouteFinder + RegionManager collision (see movement_queue.go,
// pathfinder.go, routefinding.go, regions.go), wired through
// Player.OnMovementRequest by the coordinator. Visibility (update
// buckets and local player/NPC lists) IS ported — see visibility.go —
// but region gating of the NPC set is NOT STARTED, so every registered
// NPC is bucketed. Hook fields exist so later milestones can fill World phases
// without editing World.Process. The MovementRequest type and the
// Player.OnMovementRequest hook exist so decoded walk/gameclick input has
// somewhere to land; the wired server installs the hook with the
// MovementQueue request path.
package game
