package game

import (
	"tsps-go/internal/definition"
)

// This file applies spawn records to the world at boot, before the
// engine starts. It ports the application half of the spawn
// loaders; the loading half (JSON parsing, validation, defaults)
// lives in internal/definition.
//
// NPC spawns: NpcSpawnDefinitionLoader.spawn
// (NpcSpawnDefinitionLoader.ts:306-316) creates the NPC at the spawn
// position, sets the wander radius on its movement coordinator,
// sets its facing, attaches the spawn lineage, and adds it to the
// world list. The Go form queues each NPC with QueueAddNPC: the
// first Process drains the add queue (world.go queue_add_npcs
// phase, cap 150 per tick), so spawns queued before the engine
// starts are picked up on the first tick. Spawning here is
// deliberately EAGER — every record becomes a live NPC — where
// TypeScript defers most spawns in pendingByRegion and activates
// them near players (activateNear). The deferred-activation
// machinery (pendingByRegion, the NPCActivation hook's sweep) is
// NOT STARTED; eager spawning is the honest static-world
// approximation. The wander radius, facing, and lineage ARE
// wired (Phase 16): the coordinator wanders spawned NPCs within
// their radius, and respawn clones keep the shared spawn-file
// lineage.
//
// What is NOT ported here: the spawn Name (a description set via
// setDescription in TypeScript) has no Go field to land on, so it
// is ignored explicitly; the content veto (emitCanSpawnNpc) has
// no plugin system behind it and never vetoes.
//
// Visible: NPC.visible defaults to true in TypeScript
// (NPC.ts:167, `private visible: boolean = true`), a field
// initializer — the spawn path never calls setVisible. The Go
// zero value is false, so SpawnNPCs sets Visible explicitly; an
// invisible NPC would never enter the visibility buckets
// (visibility.go skips !Visible) and no client would see it.
//
// Hitpoints: the NPC constructor (NPC.ts:190-199) takes
// hitpoints from the NPC definition, or 10 when there is no
// definition. "Definition" there is NpcDefinition
// (NPC.getDefinition, NPC.ts:855-857): NpcDefinition.forId never
// returns null — out-of-range ids get DEFAULT, whose hitpoints is
// 10 (NpcDefinition.ts:22,52) — hydrateFromCache replaces 10
// with the cache NpcType hitpoints only when it is positive
// (NpcDefinition.ts:116), and the JSON monsters dump can
// override further (NpcDefinitionLoader: hitpoints ??
// definition). The boot wiring supplies a source backed by the
// loaded Store (Store.NPC), which carries that full chain; the
// fallback when no source is supplied is 10, consistently: the
// same value a definition-less NPC gets in TypeScript, and the
// value the Go NPC.Hitpoints zero-means-unset convention
// distinguishes from "unset" (npc.go).
//
// The hitpoints source is a function, not a cache import:
// internal/game must not import internal/cache (the established
// seam convention — see RegionDataSource in collision.go), so
// the boot wiring in cmd/server supplies a Store-backed source
// and this package stays cache-free. internal/definition imports
// cache/config only, so game -> definition introduces no import
// cycle.

// NPCHitpoints resolves an NPC's starting hitpoints from its
// definition id. It must return a positive value; a
// non-positive return is clamped to DefaultNPCHitpoints (see
// SpawnNPCsWithHitpoints). A nil source means "no definitions
// available" and every NPC gets DefaultNPCHitpoints.
type NPCHitpoints func(npcID int) int

// DefaultNPCHitpoints is the hitpoints of an NPC with no
// definition: 10 (NPC.ts:195-199, NpcDefinition.ts:52).
const DefaultNPCHitpoints = 10

// NPCDefinitionSource resolves an NPC's definition for the
// drivers (size, aggression, respawn, hitpoints). It must never
// return nil for a served id; a nil return leaves the NPC on the
// DEFAULT shape (see NPCDefinition). A nil source means "no
// definitions available" and every NPC keeps the DEFAULT shape.
// The boot wiring supplies a Store-backed source (Store.NPC,
// which never returns nil); *definition.NpcDefinition satisfies
// NPCDefinition directly, so no adapter is needed (see the
// assertion below).
type NPCDefinitionSource func(npcID int) NPCDefinition

var _ NPCDefinition = (*definition.NpcDefinition)(nil)

// SpawnNPCs queues one NPC per spawn record, with default
// hitpoints (no definition source). It is SpawnNPCsWithHitpoints
// with a nil source; the boot wiring uses the latter with a
// Store-backed source when definitions are loaded.
func SpawnNPCs(w *World, spawns []definition.NpcSpawn) {
	SpawnNPCsWithHitpoints(w, spawns, nil)
}

// SpawnNPCsWithHitpoints queues one NPC per spawn record, with
// starting hitpoints from hp (or DefaultNPCHitpoints when hp is
// nil or returns a non-positive value). It is
// SpawnNPCsWithHitpointsAndDefinitions with a nil definition
// source: spawned NPCs keep the DEFAULT definition shape. The
// boot wiring uses the definitions form with a Store-backed
// source when definitions are loaded.
func SpawnNPCsWithHitpoints(w *World, spawns []definition.NpcSpawn, hp NPCHitpoints) {
	SpawnNPCsWithHitpointsAndDefinitions(w, spawns, hp, nil)
}

// SpawnNPCsWithHitpointsAndDefinitions queues one NPC per spawn
// record, with starting hitpoints from hp and the driver
// definition from defs (nil entries leave the DEFAULT shape).
// Each NPC is created at the spawn tile with Spawn set to that
// tile (NewNPC), Visible set (see the file header), Facing and
// spawn face set from the spawn's server Direction id
// (DirectionFromID; the loader masks the client direction to
// 0..7 and maps it through CLIENT_TO_SERVER_DIRECTION,
// NpcSpawnDefinitionLoader.ts), the wander radius set on its
// movement coordinator, and the shared spawn-file lineage
// attached (NpcSpawnDefinitionLoader.ts:306-316, spawn).
// Duplicate records queue duplicate NPCs, as in TypeScript,
// where several spawns can share a tile. An empty list is a
// no-op. Callers must call this before the engine starts (or
// accept that queued NPCs appear on a later tick); w must be
// non-nil.
func SpawnNPCsWithHitpointsAndDefinitions(w *World, spawns []definition.NpcSpawn, hp NPCHitpoints, defs NPCDefinitionSource) {
	for _, spawn := range spawns {
		loc := Location{X: spawn.X, Y: spawn.Y, Z: spawn.Z}
		npc := NewNPC(spawn.ID, loc)
		npc.Visible = true
		hitpoints := DefaultNPCHitpoints
		if hp != nil {
			if resolved := hp(spawn.ID); resolved > 0 {
				hitpoints = resolved
			}
		}
		npc.Hitpoints = hitpoints
		npc.Facing = DirectionFromID(spawn.Facing)
		npc.SetFace(DirectionFromID(spawn.Facing))
		npc.MovementCoordinator().SetRadius(spawn.WanderRadius)
		npc.SetSpawnLineage(NpcSpawnsLineage())
		if defs != nil {
			if d := defs(spawn.ID); d != nil {
				npc.SetDefinition(d)
			}
		}
		// spawn.Name (a description) has no Go field to land
		// on; ignoring it here is explicit, not an oversight.
		// spawn.Source names the contributing source, but the
		// lineage is the loader's shared one regardless of
		// source (see the file header), so it is not stored
		// per NPC.
		_ = spawn.Name
		_ = spawn.Source
		w.QueueAddNPC(npc)
	}
}

// SpawnObjects applies object-spawn records through the object
// registry: each record becomes a GameObject registered with
// playerUpdate=true, the Go form of
// ObjectSpawnDefinitionLoader.load
// (ObjectSpawnDefinitionLoader.ts:39-45: new GameObject(...),
// ObjectManager.register(object, true)). Registration performs:
// the object enters the tile map and the world indexes, its
// clipping is laid through MapObjects.add's
// RegionManager.addObjectClipping call (MapObjects.ts:229), and
// a spawn delta is recorded. Clipping behavior is preserved
// exactly against the old clipping-only path: for distinct
// tiles/shapes the same AddObjectClipping calls land in the same
// order, and for same-tile same-shape duplicates the replacement
// loop lifts the predecessor's clipping before laying the new
// object's — the same net clip grid either way. The visibility
// half that used to be an explicit gap is now real: registered
// spawns are visible to clients through the session layer's
// region-change snapshots and deltas.
//
// Deltas recorded here are cleared at the end: spawns apply at
// boot, before any player exists, and TS register sends reach
// zero network players at that point. Type/Face are the
// loader-resolved values (defaults 10/0 from
// ObjectSpawnDefinition); a record's Type of 0 is an explicit
// wall placement, never re-defaulted here. Spawned objects are
// core's own (spawnedBy ""), so shutdown's RemovePluginObjects
// leaves them standing.
//
// Duplicates: same-tile same-shape duplicates now COLLAPSE to
// the last record (TS register replaces same-type predecessors),
// where the old clipping-only path applied the same clip twice
// idempotently; different shapes on a tile still stack, as TS
// stacks spawns. The net clip grid is identical either way.
//
// Removal records (TS `remove: true`) arrive separately, via
// Store.ObjectSpawnRemovals, and are applied by
// SpawnObjectRemovals. The loader splits registrations from
// removals, so the file's interleave order is lost: all
// registrations apply before all removals. In TypeScript the two
// interleave in file order, but removals in practice target
// cache map objects, not other spawn entries, so the split
// changes nothing observable for the shipped data. (A removal
// naming a spawn entry that appears LATER in the file would now
// remove the registered spawn where TS file order would leave it
// standing — the same pre-existing split limitation, now visible
// in the registry as well as the clip grid.)
func SpawnObjects(w *World, spawns []definition.ObjectSpawn) {
	registry := w.GameObjects()
	for _, spawn := range spawns {
		registry.Register(NewGameObject(
			spawn.ID,
			Location{X: spawn.X, Y: spawn.Y, Z: spawn.Z},
			spawn.Type,
			spawn.Face,
			nil,
		), true)
	}
	// Boot-only: no players exist yet, so the recorded deltas
	// reach nobody (see the doc comment).
	registry.ClearDeltas()
}

// SpawnObjectRemovals applies object-spawn removal records
// through the object registry: each record is deregistered with
// playerUpdate=true, the Go form of
// ObjectSpawnDefinitionLoader.load
// (ObjectSpawnDefinitionLoader.ts:40-43: load the region's map
// files first — "Decode the base objects and collision before
// applying a saved removal" — then
// ObjectManager.deregister(object, true)). Deregistration lifts
// the clipping through MapObjects.remove's
// RegionManager.removeObjectClipping call, removes matching
// runtime objects from the world indexes, and — for map objects
// — records the removed-list entry that keeps the spot hidden
// on scene reloads. The visibility half that used to be an
// explicit gap is now real: removed map objects stay hidden for
// clients through the removed-list ordering on region change.
// Removing an object that was never registered (or whose
// definition is unknown) is a safe no-op, and the recorded
// deltas are cleared as in SpawnObjects: boot-time removals
// reach no player.
func SpawnObjectRemovals(w *World, removals []definition.ObjectSpawn) {
	registry := w.GameObjects()
	for _, spawn := range removals {
		w.RegionManager().LoadRegion(spawn.X, spawn.Y)
		registry.Deregister(NewGameObject(
			spawn.ID,
			Location{X: spawn.X, Y: spawn.Y, Z: spawn.Z},
			spawn.Type,
			spawn.Face,
			nil,
		), true)
	}
	registry.ClearDeltas()
}
