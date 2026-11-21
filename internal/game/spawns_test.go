package game

import (
	"testing"

	"tsps-go/internal/definition"
)

func TestSpawnNPCsQueuedWithSpawnFields(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	SpawnNPCs(w, []definition.NpcSpawn{
		{ID: 123, X: 3090, Y: 3525, Z: 0, Facing: 6, WanderRadius: 5, Name: "Shopkeeper", Source: "core"},
		{ID: 456, X: 3100, Y: 3530, Z: 1, Facing: 1, WanderRadius: 0},
	})
	// Nothing is registered before the first tick: spawns queue.
	if got := w.NPCs().Len(); got != 0 {
		t.Fatalf("NPCs before first tick = %d, want 0", got)
	}
	w.Process()
	npcs := w.NPCs().All()
	if len(npcs) != 2 {
		t.Fatalf("NPCs after first tick = %d, want 2", len(npcs))
	}
	byID := map[int]*NPC{}
	for _, n := range npcs {
		byID[n.ID] = n
	}
	first := byID[123]
	if first == nil {
		t.Fatal("NPC 123 missing")
	}
	if first.Loc != (Location{X: 3090, Y: 3525, Z: 0}) {
		t.Errorf("Loc = %+v", first.Loc)
	}
	if first.Spawn != first.Loc {
		t.Errorf("Spawn = %+v, want Loc %+v", first.Spawn, first.Loc)
	}
	if !first.Visible {
		t.Error("Visible = false, want true (NPC.ts field default)")
	}
	if first.Hitpoints != DefaultNPCHitpoints {
		t.Errorf("Hitpoints = %d, want %d (no definition source)", first.Hitpoints, DefaultNPCHitpoints)
	}
	if first.Facing != DirSouth {
		t.Errorf("Facing = %v, want DirSouth (server direction 6)", first.Facing)
	}
	if !first.IsRegistered() {
		t.Error("NPC 123 not registered after first tick")
	}
	second := byID[456]
	if second == nil {
		t.Fatal("NPC 456 missing")
	}
	if second.Loc.Z != 1 || second.Facing != DirNorth {
		t.Errorf("second NPC: Loc=%+v Facing=%v", second.Loc, second.Facing)
	}
}

func TestSpawnNPCsHitpointsSource(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	SpawnNPCsWithHitpoints(w, []definition.NpcSpawn{
		{ID: 1, X: 3090, Y: 3525, Facing: 6},
		{ID: 2, X: 3091, Y: 3525, Facing: 6},
		{ID: 3, X: 3092, Y: 3525, Facing: 6},
	}, func(id int) int {
		switch id {
		case 1:
			return 250 // cache hitpoints
		case 2:
			return 0 // unknown/definition-less: clamped to default
		default:
			return -1 // loader default (-1): clamped to default
		}
	})
	w.Process()
	byID := map[int]*NPC{}
	for _, n := range w.NPCs().All() {
		byID[n.ID] = n
	}
	if byID[1].Hitpoints != 250 {
		t.Errorf("NPC 1 Hitpoints = %d, want 250", byID[1].Hitpoints)
	}
	if byID[2].Hitpoints != DefaultNPCHitpoints {
		t.Errorf("NPC 2 Hitpoints = %d, want %d", byID[2].Hitpoints, DefaultNPCHitpoints)
	}
	if byID[3].Hitpoints != DefaultNPCHitpoints {
		t.Errorf("NPC 3 Hitpoints = %d, want %d", byID[3].Hitpoints, DefaultNPCHitpoints)
	}
}

func TestSpawnNPCsFacingMapping(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	var spawns []definition.NpcSpawn
	for facing := 0; facing < 8; facing++ {
		spawns = append(spawns, definition.NpcSpawn{ID: 100 + facing, X: 3090 + facing, Y: 3525, Facing: facing})
	}
	spawns = append(spawns, definition.NpcSpawn{ID: 200, X: 3099, Y: 3525, Facing: 99})
	SpawnNPCs(w, spawns)
	w.Process()
	byID := map[int]*NPC{}
	for _, n := range w.NPCs().All() {
		byID[n.ID] = n
	}
	for facing := 0; facing < 8; facing++ {
		if byID[100+facing].Facing != Direction(facing) {
			t.Errorf("Facing %d -> %v, want Direction(%d)", facing, byID[100+facing].Facing, facing)
		}
	}
	if byID[200].Facing != DirNone {
		t.Errorf("invalid facing 99 -> %v, want DirNone", byID[200].Facing)
	}
}

func TestSpawnNPCsEmptyIsNoop(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	SpawnNPCs(w, nil)
	SpawnNPCs(w, []definition.NpcSpawn{})
	w.Process()
	if got := w.NPCs().Len(); got != 0 {
		t.Errorf("NPCs = %d, want 0", got)
	}
}

func TestSpawnNPCsDuplicatesKept(t *testing.T) {
	// TypeScript keeps stacked spawns (several records can share
	// a tile); duplicates queue as separate NPCs.
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	record := definition.NpcSpawn{ID: 7, X: 3090, Y: 3525, Facing: 6, WanderRadius: 5}
	SpawnNPCs(w, []definition.NpcSpawn{record, record, record})
	w.Process()
	if got := w.NPCs().Len(); got != 3 {
		t.Errorf("NPCs = %d, want 3", got)
	}
}

func TestSpawnNPCsStayStill(t *testing.T) {
	// The stationary rule: a radius-0 spawned NPC never moves,
	// whatever the world ticks do.
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	SpawnNPCs(w, []definition.NpcSpawn{{ID: 9, X: 3090, Y: 3525, Facing: 6, WanderRadius: 0}})
	for i := 0; i < 20; i++ {
		w.Process()
	}
	npcs := w.NPCs().All()
	if len(npcs) != 1 {
		t.Fatalf("NPCs = %d, want 1", len(npcs))
	}
	if npcs[0].Loc != (Location{X: 3090, Y: 3525, Z: 0}) {
		t.Errorf("NPC moved to %+v", npcs[0].Loc)
	}
}

func TestSpawnNPCsWanderWithinRadius(t *testing.T) {
	// Phase 16 drivers: a spawned NPC with a wander radius
	// wanders, but never leaves the radius around its spawn.
	// (Only the bound is asserted: whether it moves on any
	// given tick is a dice roll.)
	spawn := Location{X: 3090, Y: 3525, Z: 0}
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	SpawnNPCs(w, []definition.NpcSpawn{{ID: 9, X: spawn.X, Y: spawn.Y, Facing: 6, WanderRadius: 32}})
	for i := 0; i < 50; i++ {
		w.Process()
		npcs := w.NPCs().All()
		if len(npcs) != 1 {
			t.Fatalf("tick %d: NPCs = %d, want 1", i, len(npcs))
		}
		if d := npcs[0].Loc.Distance(spawn); d < 0 || d > 32 {
			t.Fatalf("tick %d: NPC at %+v, %d from spawn, outside radius 32", i, npcs[0].Loc, d)
		}
	}
}

func TestSpawnObjectsAppliesClipping(t *testing.T) {
	// A region with empty terrain; object 100 is a solid 1x1 that
	// blocks projectiles. The spawn is applied BEFORE the region
	// loads: AddObjectClipping must load the region first, so the
	// clip lands on the decoded grid instead of being lost.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	w.SetRegionManager(NewRegionManager(src, defs, true))
	if w.RegionManager().IsRegionLoaded(regionTestID()) {
		t.Fatal("region already loaded before spawn")
	}
	SpawnObjects(w, []definition.ObjectSpawn{{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	if !w.RegionManager().IsRegionLoaded(regionTestID()) {
		t.Error("region not loaded by spawn application")
	}
	want := int32(0x100 | 0x20000)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != want {
		t.Errorf("clipping = %#x, want %#x", got, want)
	}
	if got := w.RegionManager().GetClipping(3201, 3200, 0); got != 0 {
		t.Errorf("neighbor clipping = %#x, want 0", got)
	}
}

func TestSpawnObjectsUnknownDefinitionIsNoop(t *testing.T) {
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	w.SetRegionManager(NewRegionManager(src, &fakeObjectDefs{defs: map[int]ObjectDef{}}, true))
	SpawnObjects(w, []definition.ObjectSpawn{{ID: 999, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping = %#x, want 0 (no definition)", got)
	}
}

func TestSpawnObjectsNoCacheManagerIsSafe(t *testing.T) {
	// The default empty manager (NoCache) has no regions indexed:
	// spawning objects must be a safe no-op, never a panic.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	SpawnObjects(w, []definition.ObjectSpawn{{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	SpawnObjects(w, nil)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping = %#x, want 0", got)
	}
}

func TestSpawnObjectRemovalsLiftClipping(t *testing.T) {
	// Clip a tile with a spawn, then remove it: the tile must be
	// walkable again. This mirrors a `remove: true` row lifting a
	// map object's clipping.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	w.SetRegionManager(NewRegionManager(src, defs, true))
	placement := definition.ObjectSpawn{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}
	SpawnObjects(w, []definition.ObjectSpawn{placement})
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got == 0 {
		t.Fatal("spawn did not clip the tile")
	}
	SpawnObjectRemovals(w, []definition.ObjectSpawn{placement})
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after removal = %#x, want 0", got)
	}
}

func TestSpawnObjectRemovalsWithoutSpawnIsSafe(t *testing.T) {
	// Removing clipping that was never applied (or whose
	// definition is unknown) must be a safe no-op.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	w.SetRegionManager(NewRegionManager(src, &fakeObjectDefs{defs: map[int]ObjectDef{}}, true))
	SpawnObjectRemovals(w, []definition.ObjectSpawn{
		{ID: 999, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0},
	})
	SpawnObjectRemovals(w, nil)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping = %#x, want 0", got)
	}
}

func TestSpawnObjectsRegisterInRegistry(t *testing.T) {
	// Spawns register in the object registry (visible to clients
	// through the session layer) AND keep their clipping.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	w.SetRegionManager(NewRegionManager(src, defs, true))
	SpawnObjects(w, []definition.ObjectSpawn{{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	loc := Location{X: 3200, Y: 3200, Z: 0}
	registered := w.GameObjects().At(loc)
	if len(registered) != 1 || registered[0].ID() != 100 {
		t.Fatalf("registry At = %v, want [100]", objectIDs(registered))
	}
	if registered[0].SpawnedBy() != "" {
		t.Errorf("spawnedBy = %q, want core's own", registered[0].SpawnedBy())
	}
	player := NewPlayer("alice", loc)
	if got := w.GameObjects().Find(player, 100, loc); got != registered[0] {
		t.Error("spawned object must be findable in the tile map")
	}
	want := int32(0x100 | 0x20000)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != want {
		t.Errorf("clipping = %#x, want %#x (preserved)", got, want)
	}
	// Boot-time deltas are discarded: no player exists yet.
	if got := len(w.GameObjects().Deltas()); got != 0 {
		t.Errorf("Deltas after boot spawns = %d, want 0", got)
	}
}

func TestSpawnObjectsSameShapeCollapseDifferentShapeStacks(t *testing.T) {
	// TS register replaces same-shape predecessors: same-tile
	// same-type duplicates collapse to the last record, while
	// different shapes stack.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	SpawnObjects(w, []definition.ObjectSpawn{
		{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0},
		{ID: 101, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0},
		{ID: 102, X: 3200, Y: 3200, Z: 0, Type: 4, Face: 0},
	})
	loc := Location{X: 3200, Y: 3200, Z: 0}
	got := w.GameObjects().At(loc)
	if len(got) != 2 {
		t.Fatalf("At = %v, want [101 102]", objectIDs(got))
	}
	player := NewPlayer("alice", loc)
	if found := w.GameObjects().Find(player, 100, loc); found != nil {
		t.Error("replaced same-shape spawn must be gone from the tile map")
	}
	if found := w.GameObjects().Find(player, 101, loc); found == nil {
		t.Error("last same-shape spawn must stand")
	}
	if found := w.GameObjects().Find(player, 102, loc); found == nil {
		t.Error("different-shape spawn must stack")
	}
}

func TestSpawnObjectsNoCacheRegisters(t *testing.T) {
	// NoCache: the registry still holds the spawn; clipping is
	// skipped (no regions indexed) and nothing panics.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	SpawnObjects(w, []definition.ObjectSpawn{{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	loc := Location{X: 3200, Y: 3200, Z: 0}
	if got := w.GameObjects().At(loc); len(got) != 1 {
		t.Errorf("At = %v, want [100]", objectIDs(got))
	}
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping = %#x, want 0", got)
	}
}

func TestSpawnObjectRemovalsDeregisterSpawn(t *testing.T) {
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: buildTestTerrain(true, nil), objects: nil, ok: true}
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	w.SetRegionManager(NewRegionManager(src, defs, true))
	placement := definition.ObjectSpawn{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}
	SpawnObjects(w, []definition.ObjectSpawn{placement})
	SpawnObjectRemovals(w, []definition.ObjectSpawn{placement})
	loc := Location{X: 3200, Y: 3200, Z: 0}
	if got := w.GameObjects().At(loc); len(got) != 0 {
		t.Errorf("At after removal = %v, want empty", objectIDs(got))
	}
	player := NewPlayer("alice", loc)
	if got := w.GameObjects().Find(player, 100, loc); got != nil {
		t.Error("removed spawn must not be findable")
	}
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after removal = %#x, want 0", got)
	}
	// A runtime spawn is not a map object: no removed-list entry.
	if got := len(w.GameObjects().RemovedObjects()); got != 0 {
		t.Errorf("RemovedObjects = %d, want 0", got)
	}
	if got := len(w.GameObjects().Deltas()); got != 0 {
		t.Errorf("Deltas after boot removals = %d, want 0", got)
	}
}

func TestSpawnObjectRemovalsHideMapObject(t *testing.T) {
	// A `remove: true` row naming a cache-map object: the region
	// loads first, the object leaves the tile map, the clipping
	// lifts, and the removed-list entry keeps it hidden across a
	// region reload.
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{
		terrain: buildTestTerrain(true, nil),
		objects: buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		ok: true,
	}
	defs := &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
	}}
	w.SetRegionManager(NewRegionManager(src, defs, true))
	SpawnObjectRemovals(w, []definition.ObjectSpawn{{ID: 100, X: 3200, Y: 3200, Z: 0, Type: 10, Face: 0}})
	loc := Location{X: 3200, Y: 3200, Z: 0}
	player := NewPlayer("alice", loc)
	if got := w.GameObjects().Find(player, 100, loc); got != nil {
		t.Error("removed map object must not be findable")
	}
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after removal = %#x, want 0", got)
	}
	if got := len(w.GameObjects().RemovedObjects()); got != 1 {
		t.Fatalf("RemovedObjects = %d, want 1", got)
	}
	if !w.RegionManager().ReloadRegion(regionTestID()) {
		t.Fatal("ReloadRegion must succeed")
	}
	if got := w.GameObjects().Find(player, 100, loc); got != nil {
		t.Error("reload must not resurrect a spawn-removed map object")
	}
}
