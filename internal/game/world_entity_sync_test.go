package game

import "testing"

func syncTestSetup(t *testing.T) (*BoatManager, *WorldEntitySync, *Player) {
	t.Helper()
	m := NewBoatManager(nil)
	sync := NewWorldEntitySync(m)
	viewer := NewPlayer("viewer", Location{X: 3000, Y: 3000, Z: 0})
	viewer.SetIndex(1)
	return m, sync, viewer
}

func TestWorldEntitySyncSpawn(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	plan := sync.Flush(viewer)
	if len(plan.Scenes) != 1 {
		t.Fatalf("scenes = %d, want 1", len(plan.Scenes))
	}
	scene := plan.Scenes[0]
	if scene.EntityIndex != boat.EntityIndex || scene.ConfigID != 7 || scene.SizeX != 8 || scene.SizeZ != 16 {
		t.Errorf("scene = %+v", scene)
	}
	if scene.RegionX != 1200 || scene.RegionY != 1200 {
		t.Errorf("scene region = (%d, %d), want (1200, 1200)", scene.RegionX, scene.RegionY)
	}
	if scene.RegionID != ((100>>3)<<8)|(100>>3) {
		t.Errorf("scene regionID = %d, want %d", scene.RegionID, ((100>>3)<<8)|(100>>3))
	}
	// The template zones sit at the scene centre: [plane][6][6]
	// and [plane][6][7] (1x2 zones), the rest -1.
	for plane := 0; plane < 4; plane++ {
		if scene.TemplateChunks[plane][6][6] == -1 || scene.TemplateChunks[plane][6][7] == -1 {
			t.Errorf("plane %d: centre zones must be packed", plane)
		}
		if scene.TemplateChunks[plane][0][0] != -1 || scene.TemplateChunks[plane][6][8] != -1 {
			t.Errorf("plane %d: non-template chunks must be -1", plane)
		}
	}
	// packTemplateChunk: plane in bits 24-25, chunkX in 14-23,
	// chunkY in 3-13, rotation 0.
	want := int32(((0 & 3) << 24) | ((100 & 0x3ff) << 14) | ((100 & 0x7ff) << 3))
	if scene.TemplateChunks[0][6][6] != want {
		t.Errorf("chunk[0][6][6] = %d, want %d", scene.TemplateChunks[0][6][6], want)
	}
	if !plan.HasInfo || len(plan.Spawns) != 1 {
		t.Fatalf("HasInfo=%v spawns=%d, want a spawn", plan.HasInfo, len(plan.Spawns))
	}
	spawn := plan.Spawns[0]
	if spawn.EntityIndex != boat.EntityIndex || spawn.DrawMode != 0 ||
		spawn.Position != (WorldEntityPosition{X: 3000 * FineUnitsPerTile, Y: 0, Z: 3000 * FineUnitsPerTile, Orientation: 0}) {
		t.Errorf("spawn = %+v", spawn)
	}
	// Spawn locs: the spec's full deck loc set, at absolute
	// deck tiles.
	if len(plan.SpawnLocs) != 1 || plan.SpawnLocs[0].Loc.ID != 100 || plan.SpawnLocs[0].X != 9600 || plan.SpawnLocs[0].Y != 9600 {
		t.Errorf("spawn locs = %+v", plan.SpawnLocs)
	}
	// A second flush with nothing moved sends nothing.
	again := sync.Flush(viewer)
	if !again.IsEmpty() {
		t.Errorf("second flush = %+v, want empty", again)
	}
	if again.HasInfo {
		t.Error("an unchanged flush must not send the info packet")
	}
}

func TestWorldEntitySyncStaticShipLocs(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	spec := testBoatSpec()
	spec.SyncLocs = []BoatDeckLoc{{ID: 500, X: 2, Y: 2, Level: 1, Shape: 10}}
	m.SpawnStatic(0, spec, testPlacement())
	plan := sync.Flush(viewer)
	// A static ship sends only its explicit sync locs, not
	// the template's own locs.
	if len(plan.SpawnLocs) != 1 || plan.SpawnLocs[0].Loc.ID != 500 {
		t.Errorf("static spawn locs = %+v", plan.SpawnLocs)
	}
}

func TestWorldEntitySyncMoveAndSnap(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	sync.Flush(viewer)
	// Sail 128 fine units north: a move update with the delta.
	boat.FineY += 128
	plan := sync.Flush(viewer)
	if !plan.HasInfo || len(plan.Updates) != 1 {
		t.Fatalf("updates = %+v, HasInfo=%v", plan.Updates, plan.HasInfo)
	}
	update := plan.Updates[0]
	if update.Kind != WorldEntityMove || update.Delta == nil || update.Delta.Z != 128 || update.Delta.X != 0 || update.Delta.Orientation != 0 {
		t.Errorf("update = %+v, want a move by z=128", update)
	}
	// Teleport nearby: a snap update, and the position
	// re-bases.
	m.Teleport(boat, BoatTeleportTarget{FineX: 3010 * FineUnitsPerTile, FineY: 3010 * FineUnitsPerTile})
	plan = sync.Flush(viewer)
	if len(plan.Updates) != 1 || plan.Updates[0].Kind != WorldEntitySnap {
		t.Errorf("teleport updates = %+v, want a snap", plan.Updates)
	}
	// No further change: empty again (the snap re-based the
	// position, so no move follows).
	if again := sync.Flush(viewer); !again.IsEmpty() {
		t.Errorf("post-snap flush = %+v, want empty", again)
	}
}

func TestWorldEntitySyncDespawn(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	sync.Flush(viewer)
	m.Dispose(boat)
	plan := sync.Flush(viewer)
	if !plan.HasInfo || len(plan.Updates) != 1 || plan.Updates[0].Kind != WorldEntityDespawn {
		t.Errorf("despawn updates = %+v, HasInfo=%v", plan.Updates, plan.HasInfo)
	}
	if again := sync.Flush(viewer); !again.IsEmpty() {
		t.Errorf("post-despawn flush = %+v, want empty", again)
	}
}

func TestWorldEntitySyncOutOfRange(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	sync.Flush(viewer)
	// Sail out of view distance: despawned.
	boat.FineX = 3100 * FineUnitsPerTile
	plan := sync.Flush(viewer)
	if len(plan.Updates) != 1 || plan.Updates[0].Kind != WorldEntityDespawn {
		t.Errorf("out-of-range updates = %+v", plan.Updates)
	}
	// An index despawned this tick cannot respawn in the same
	// flush: sailing back takes effect next flush.
	boat.FineX = 3000 * FineUnitsPerTile
	plan = sync.Flush(viewer)
	if len(plan.Spawns) != 1 {
		t.Errorf("returning spawns = %+v, want a respawn next flush", plan.Spawns)
	}
}

func TestWorldEntitySyncAboardAlwaysShown(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	// The viewer stands on the deck: the boat is shown even
	// though the deck tile is thousands of tiles from the
	// boat's tile (the root tile is what counts).
	viewer.SetLocation(Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0})
	m.GetDeck(boat).Enter(viewer)
	plan := sync.Flush(viewer)
	if len(plan.Spawns) != 1 {
		t.Fatalf("aboard spawns = %+v, want the boat", plan.Spawns)
	}
	// Sail far away: still shown (aboard always wins).
	boat.FineX = 5000 * FineUnitsPerTile
	boat.FineY = 5000 * FineUnitsPerTile
	plan = sync.Flush(viewer)
	if len(plan.Updates) != 1 || plan.Updates[0].Kind == WorldEntityDespawn {
		t.Errorf("aboard far away updates = %+v, want a move, not a despawn", plan.Updates)
	}
}

func TestWorldEntitySyncCaps(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	for i := 0; i < 10; i++ {
		m.Spawn(1, testBoatSpec(), testPlacement())
	}
	plan := sync.Flush(viewer)
	if len(plan.Spawns) != 4 {
		t.Errorf("first flush spawns = %d, want 4 (new-per-tick cap)", len(plan.Spawns))
	}
	plan = sync.Flush(viewer)
	if len(plan.Spawns) != 4 {
		t.Errorf("second flush spawns = %d, want 4", len(plan.Spawns))
	}
	plan = sync.Flush(viewer)
	if len(plan.Spawns) != 0 || plan.HasInfo {
		t.Errorf("third flush = %+v, want empty (8 in view, none new)", plan)
	}
	// Eight boats are in view: moving one sends all eight
	// updates (one move, seven unchanged).
	m.All()[0].FineX += 128
	plan = sync.Flush(viewer)
	if len(plan.Updates) != 8 {
		t.Errorf("updates = %d, want 8", len(plan.Updates))
	}
}

func TestWorldEntitySyncDeckLocChanges(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	sync.Flush(viewer)
	loc := BoatDeckLoc{ID: 200, X: 1, Y: 1, Level: 1, Shape: 10, Rotation: 1}
	m.SetDeckLoc(boat, loc)
	plan := sync.Flush(viewer)
	// A deck-loc change alone does not set HasInfo, but the
	// changed loc is still sent.
	if plan.HasInfo {
		t.Error("a loc-only change must not send the info packet")
	}
	if len(plan.ChangedLocs) != 1 || plan.ChangedLocs[0].Loc.ID != 200 {
		t.Errorf("changed locs = %+v", plan.ChangedLocs)
	}
	if plan.IsEmpty() {
		t.Error("a loc-only change must not be an empty plan")
	}
	// Removing it sends a removed change.
	m.RemoveDeckLoc(boat, loc)
	plan = sync.Flush(viewer)
	if len(plan.ChangedLocs) != 1 || !plan.ChangedLocs[0].Loc.Removed {
		t.Errorf("removed locs = %+v", plan.ChangedLocs)
	}
	// Caught up: empty.
	if again := sync.Flush(viewer); !again.IsEmpty() {
		t.Errorf("caught-up flush = %+v, want empty", again)
	}
}

func TestWorldEntitySyncOwnerOnly(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	boat := m.Spawn(99, testBoatSpec(), testPlacement())
	boat.OwnerOnly = true
	plan := sync.Flush(viewer)
	if !plan.IsEmpty() {
		t.Errorf("owner-only flush = %+v, want empty for a stranger", plan)
	}
	viewer.SetIndex(99)
	plan = sync.Flush(viewer)
	if len(plan.Spawns) != 1 {
		t.Errorf("owner flush spawns = %+v, want the boat", plan.Spawns)
	}
}

func TestWorldEntitySyncForget(t *testing.T) {
	m, sync, viewer := syncTestSetup(t)
	m.Spawn(1, testBoatSpec(), testPlacement())
	sync.Flush(viewer)
	sync.Forget(viewer)
	// Forgotten: the next flush spawns the boat again, as if
	// the viewer had never seen it.
	plan := sync.Flush(viewer)
	if len(plan.Spawns) != 1 || len(plan.Scenes) != 1 {
		t.Errorf("post-forget plan = %+v", plan)
	}
}

func TestWorldEntitySyncNoManager(t *testing.T) {
	sync := NewWorldEntitySync(nil)
	viewer := NewPlayer("viewer", Location{X: 3000, Y: 3000})
	if plan := sync.Flush(viewer); !plan.IsEmpty() {
		t.Errorf("no-manager flush = %+v, want empty", plan)
	}
}

func TestWorldEntitiesFlushAndForget(t *testing.T) {
	m, _, viewer := syncTestSetup(t)
	m.Spawn(1, testBoatSpec(), testPlacement())
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	w.SetBoatManager(m)
	plan := w.WorldEntities().Flush(viewer)
	if len(plan.Spawns) != 1 {
		t.Fatalf("world flush spawns = %+v", plan.Spawns)
	}
	w.ForgetWorldEntityViewer(viewer)
	plan = w.WorldEntities().Flush(viewer)
	if len(plan.Spawns) != 1 {
		t.Errorf("post-forget world flush spawns = %+v", plan.Spawns)
	}
}
