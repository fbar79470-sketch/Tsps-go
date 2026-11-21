package game

import "testing"

// testBoatSpec is a sloop-shaped spec: 8x16 deck, walkable tiles
// around the boarding tile, one solid loc.
func testBoatSpec() BoatSpec {
	return BoatSpec{
		Type:           "sloop",
		ConfigID:       7,
		TemplateChunkX: 100,
		TemplateChunkY: 100,
		SizeX:          8,
		SizeZ:          16,
		Hull:           BoatHull{OffsetX: -64, OffsetY: -256, Width: 128, Length: 512},
		DeckCentreX:    512,
		DeckCentreY:    1024,
		DeckLevel:      1,
		WalkableDeck: []DeckTile{
			{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 0, Y: 1}, {X: 1, Y: 1},
			{X: 4, Y: 8}, {X: 5, Y: 8}, {X: 4, Y: 9},
		},
		BoardingTile: DeckTile{X: 4, Y: 8},
		Locs: []BoatDeckLoc{
			{ID: 100, X: 0, Y: 0, Level: 1, Shape: 10, Rotation: 0, Blocks: true},
		},
	}
}

func testPlacement() BoatPlacement {
	return BoatPlacement{FineX: 3000 * FineUnitsPerTile, FineY: 3000 * FineUnitsPerTile, Level: 0, Angle: 0}
}

// stubCollision is a BoatCollisionSource over hand-set water and
// clips.
type stubCollision struct {
	water map[[2]int]bool
	clips map[[3]int]int32
}

func (s *stubCollision) IsWater(pos Location) bool { return s.water[[2]int{pos.X, pos.Y}] }
func (s *stubCollision) GetClipping(x, y, level int) int32 {
	return s.clips[[3]int{x, y, level}]
}

func TestBoatManagerSpawnAllocates(t *testing.T) {
	m := NewBoatManager(nil)
	first := m.Spawn(1, testBoatSpec(), testPlacement())
	if first == nil || first.EntityIndex != FirstBoatEntityIndex {
		t.Fatalf("first boat index = %v, want %d", first, FirstBoatEntityIndex)
	}
	if first.DeckRegionX != 1200 || first.DeckRegionY != 1200 {
		t.Errorf("first deck region = (%d, %d), want (1200, 1200)", first.DeckRegionX, first.DeckRegionY)
	}
	if first.OwnerPlayerID != 1 || first.ConfigID != 7 || first.Static {
		t.Errorf("first boat = %+v", first)
	}
	second := m.Spawn(2, testBoatSpec(), testPlacement())
	if second == nil || second.EntityIndex != FirstBoatEntityIndex+1 {
		t.Fatalf("second boat index = %v, want %d", second, FirstBoatEntityIndex+1)
	}
	if second.DeckRegionX != 1216 || second.DeckRegionY != 1200 {
		t.Errorf("second deck region = (%d, %d), want (1216, 1200)", second.DeckRegionX, second.DeckRegionY)
	}
	if m.GetBoat(3000) != first || m.GetBoat(3001) != second || m.GetBoat(3002) != nil {
		t.Error("GetBoat must resolve spawned indices only")
	}
	all := m.All()
	if len(all) != 2 || all[0] != first || all[1] != second {
		t.Errorf("All = %v, want spawn order", all)
	}
	spec, ok := m.GetSpec(first)
	if !ok || spec.Type != "sloop" {
		t.Errorf("GetSpec = %+v, %v", spec, ok)
	}
	if m.GetDeck(first) == nil {
		t.Error("GetDeck must return the deck")
	}
}

func TestBoatManagerSpawnStaticPreferredIndex(t *testing.T) {
	m := NewBoatManager(nil)
	static := m.SpawnStatic(0, testBoatSpec(), testPlacement(), 3500)
	if static == nil || static.EntityIndex != 3500 || !static.Static {
		t.Fatalf("static boat = %v, want index 3500 static", static)
	}
	// A taken preferred index falls back to allocation.
	next := m.SpawnStatic(0, testBoatSpec(), testPlacement(), 3500)
	if next == nil || next.EntityIndex == 3500 {
		t.Fatalf("taken preferred index must allocate, got %v", next)
	}
	// An out-of-range preferred index falls back to allocation.
	third := m.SpawnStatic(0, testBoatSpec(), testPlacement(), 2999)
	if third == nil || third.EntityIndex == 2999 {
		t.Fatalf("out-of-range preferred index must allocate, got %v", third)
	}
}

func TestBoatManagerAllocationWrapsAndExhausts(t *testing.T) {
	m := NewBoatManager(nil)
	m.lastEntityIndex = LastBoatEntityIndex - 1
	a := m.Spawn(1, testBoatSpec(), testPlacement())
	b := m.Spawn(1, testBoatSpec(), testPlacement())
	if a == nil || a.EntityIndex != LastBoatEntityIndex {
		t.Fatalf("a = %v, want %d", a, LastBoatEntityIndex)
	}
	if b == nil || b.EntityIndex != FirstBoatEntityIndex {
		t.Fatalf("b = %v, want wrap to %d", b, FirstBoatEntityIndex)
	}

	full := NewBoatManager(nil)
	for i := FirstBoatEntityIndex; i <= LastBoatEntityIndex; i++ {
		full.boats[i] = &activeBoat{}
	}
	if got := full.Spawn(1, testBoatSpec(), testPlacement()); got != nil {
		t.Errorf("Spawn with all indices taken = %v, want nil", got)
	}
}

func TestBoatManagerDispose(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	deck := m.GetDeck(boat)
	m.Dispose(boat)
	if m.GetBoat(boat.EntityIndex) != nil || len(m.All()) != 0 {
		t.Error("disposed boat must be gone")
	}
	if !deck.Destroyed() {
		t.Error("dispose must destroy the deck")
	}
	// Disposing again, or a foreign boat, is a no-op.
	m.Dispose(boat)
	other := NewBoat(BoatInit{EntityIndex: 3000})
	m.Dispose(other)
	if len(m.All()) != 0 {
		t.Error("foreign dispose must not affect the manager")
	}
}

func TestBoatManagerDeckLocChanges(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	loc := BoatDeckLoc{ID: 200, X: 1, Y: 1, Level: 1, Shape: 10, Rotation: 1}
	m.SetDeckLoc(boat, loc)
	count, changes := m.DeckLocChanges(boat, 0)
	if count != 1 || len(changes) != 1 || changes[0].ID != 200 {
		t.Errorf("after set: count=%d changes=%v", count, changes)
	}
	spec, _ := m.GetSpec(boat)
	found := false
	for _, l := range spec.Locs {
		if l.ID == 200 {
			found = true
		}
	}
	if !found {
		t.Error("set must replace the spec loc")
	}
	// Remove queues a removed change; update does not queue.
	m.RemoveDeckLoc(boat, loc)
	count, changes = m.DeckLocChanges(boat, 1)
	if count != 2 || len(changes) != 1 || !changes[0].Removed {
		t.Errorf("after remove: count=%d changes=%v", count, changes)
	}
	m.UpdateDeckLoc(boat, loc)
	count, _ = m.DeckLocChanges(boat, 0)
	if count != 2 {
		t.Errorf("update must not queue: count=%d", count)
	}
	// Removing a loc that isn't there queues nothing.
	m.RemoveDeckLoc(boat, BoatDeckLoc{X: 7, Y: 7, Shape: 10})
	count, _ = m.DeckLocChanges(boat, 0)
	if count != 2 {
		t.Errorf("no-op remove must not queue: count=%d", count)
	}
	// since past the end yields no changes.
	_, changes = m.DeckLocChanges(boat, 99)
	if len(changes) != 0 {
		t.Errorf("since=99 changes=%v, want none", changes)
	}
	// Unknown boats answer zero.
	count, changes = m.DeckLocChanges(NewBoat(BoatInit{EntityIndex: 3999}), 0)
	if count != 0 || changes != nil {
		t.Errorf("unknown boat changes = %d %v", count, changes)
	}
}

func boardTestPlayer(t *testing.T, m *BoatManager, boat *Boat, name string) *Player {
	t.Helper()
	p := NewPlayer(name, Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0})
	m.GetDeck(boat).Enter(p)
	return p
}

func TestBoatManagerGetBoatAboard(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	ashore := NewPlayer("ashore", Location{X: 3000, Y: 3000})
	if m.GetBoatAboard(&ashore.Mobile) != nil {
		t.Error("ashore player must not be aboard")
	}
	aboard := boardTestPlayer(t, m, boat, "aboard")
	if m.GetBoatAboard(&aboard.Mobile) != boat {
		t.Error("deck member must be aboard")
	}
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	w.SetBoatManager(m)
	if w.boatAboard(&aboard.Mobile) != boat || w.boatAboard(&ashore.Mobile) != nil {
		t.Error("world-manager helper must match")
	}
	m.Dispose(boat)
	if m.GetBoatAboard(&aboard.Mobile) != nil {
		t.Error("disposed boat must not report aboard")
	}
}

func TestBoatManagerRootLocation(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	aboard := boardTestPlayer(t, m, boat, "aboard")
	root := m.RootLocation(&aboard.Mobile)
	// The boarding tile (9604, 9608) centres on the boat's tile.
	if root.X != 3000 || root.Y != 3000 || root.Z != 0 {
		t.Errorf("root = %+v, want (3000, 3000, 0)", root)
	}
	ashore := NewPlayer("ashore", Location{X: 3100, Y: 3100, Z: 1})
	if root := m.RootLocation(&ashore.Mobile); root != ashore.Location() {
		t.Errorf("ashore root = %+v, want own tile", root)
	}
}

func TestBoatManagerDeckTileToward(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	aboard := boardTestPlayer(t, m, boat, "aboard")
	// A target east of the boat: the easternmost walkable deck
	// tile (9605, 9608) is nearest.
	tile, ok := m.DeckTileToward(&aboard.Mobile, Location{X: 3010, Y: 3000, Z: 0}, 1)
	if !ok {
		t.Fatal("must find a nearer deck tile")
	}
	if tile.X != 9605 || tile.Y != 9608 {
		t.Errorf("tile = %+v, want (9605, 9608)", tile)
	}
	// A target under the boat: no deck tile is closer than
	// standing on the boarding tile.
	if _, ok := m.DeckTileToward(&aboard.Mobile, Location{X: 3000, Y: 3000, Z: 0}, 1); ok {
		t.Error("must report no closer tile when already nearest")
	}
	// Ashore: no deck to walk.
	ashore := NewPlayer("ashore", Location{X: 3000, Y: 3000})
	if _, ok := m.DeckTileToward(&ashore.Mobile, Location{X: 3010, Y: 3000, Z: 0}, 1); ok {
		t.Error("ashore player must have no deck tile toward")
	}
}

func TestBoatManagerSyncArea(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	aboard := boardTestPlayer(t, m, boat, "aboard")
	// A deck counts as the main world.
	if area := m.SyncArea(&aboard.Mobile); area != nil {
		t.Errorf("deck sync area = %v, want nil", area)
	}
	ashore := NewPlayer("ashore", Location{X: 3000, Y: 3000})
	if area := m.SyncArea(&ashore.Mobile); area != nil {
		t.Errorf("ashore sync area = %v, want nil", area)
	}
	// A plain private area is its own sync area.
	plain := NewPrivateArea([]Boundary{NewBoundary(3000, 3010, 3000, 3010, 0)})
	plain.Enter(ashore)
	if area := m.SyncArea(&ashore.Mobile); area != plain {
		t.Errorf("private sync area = %v, want the area", area)
	}
}

func TestBoatManagerCanSee(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	owner := NewPlayer("owner", Location{X: 3000, Y: 3000})
	owner.SetIndex(1)
	other := NewPlayer("other", Location{X: 3000, Y: 3000})
	other.SetIndex(2)
	if !m.CanSeeBoat(&owner.Mobile, boat) || !m.CanSeeBoat(&other.Mobile, boat) {
		t.Error("a public boat must be visible to everyone")
	}
	boat.OwnerOnly = true
	if !m.CanSeeBoat(&owner.Mobile, boat) {
		t.Error("an owner-only boat must be visible to its owner")
	}
	if m.CanSeeBoat(&other.Mobile, boat) {
		t.Error("an owner-only boat must be hidden from others")
	}
	aboard := boardTestPlayer(t, m, boat, "aboard")
	if !m.CanSeeAboard(&owner.Mobile, &aboard.Mobile) {
		t.Error("the owner must see the aboard player")
	}
	if m.CanSeeAboard(&other.Mobile, &aboard.Mobile) {
		t.Error("others must not see aboard an owner-only boat")
	}
	if !m.CanSeeAboard(&other.Mobile, &owner.Mobile) {
		t.Error("an ashore actor must always be visible")
	}
}

func TestBoatManagerCanInteractWithBoat(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	near := NewPlayer("near", Location{X: 3010, Y: 3000, Z: 0})
	if !m.CanInteractWithBoat(&near.Mobile, boat.EntityIndex) {
		t.Error("a near viewer must interact")
	}
	far := NewPlayer("far", Location{X: 3100, Y: 3000, Z: 0})
	if m.CanInteractWithBoat(&far.Mobile, boat.EntityIndex) {
		t.Error("a far viewer must not interact")
	}
	otherPlane := NewPlayer("plane", Location{X: 3010, Y: 3000, Z: 1})
	if m.CanInteractWithBoat(&otherPlane.Mobile, boat.EntityIndex) {
		t.Error("another plane must not interact")
	}
	if m.CanInteractWithBoat(&near.Mobile, 3999) {
		t.Error("an unknown index must not interact")
	}
	boat.OwnerOnly = true
	near.SetIndex(99)
	if m.CanInteractWithBoat(&near.Mobile, boat.EntityIndex) {
		t.Error("an owner-only boat must refuse strangers")
	}
}

func TestBoatManagerSteering(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	helm := boardTestPlayer(t, m, boat, "helm")
	helm.SetIndex(10)
	helmID := 10
	boat.HelmPlayerID = &helmID
	other := boardTestPlayer(t, m, boat, "other")
	other.SetIndex(11)

	if !m.Steers(&helm.Mobile, boat) {
		t.Error("the helm holder must steer")
	}
	if m.Steers(&other.Mobile, boat) {
		t.Error("a passenger must not steer")
	}
	// A sea click from the helm sets the heading, snapped.
	if !m.SteerToward(&helm.Mobile, 3000, 2990) {
		t.Error("helm sea click must be consumed")
	}
	if boat.Heading != 0 {
		t.Errorf("heading = %d, want 0 (south)", boat.Heading)
	}
	// A deck click from the helmsman is consumed by doing
	// nothing.
	if !m.SteerToward(&helm.Mobile, 9604, 9608) {
		t.Error("helm deck click must be consumed")
	}
	if boat.Heading != 0 {
		t.Errorf("deck click must not change the heading, got %d", boat.Heading)
	}
	// A passenger's click is not consumed.
	if m.SteerToward(&other.Mobile, 3000, 2990) {
		t.Error("passenger click must not be consumed")
	}
	// SetHelmHeading: packed 4 is west (512); out of range
	// is ignored.
	m.SetHelmHeading(&helm.Mobile, 4)
	if boat.Heading != 512 {
		t.Errorf("packed heading 4 = %d, want 512", boat.Heading)
	}
	m.SetHelmHeading(&helm.Mobile, 16)
	if boat.Heading != 512 {
		t.Errorf("packed heading 16 must be ignored, got %d", boat.Heading)
	}
	// Heading listeners fire.
	fired := 0
	m.OnHeadingSet(func(player *Mobile, b *Boat) { fired++ })
	m.SetHelmHeading(&helm.Mobile, 8)
	if fired != 1 {
		t.Errorf("heading listeners fired %d times, want 1", fired)
	}
	// The owner steers while a crewmate holds the helm.
	boat.HelmPlayerID = nil
	boat.HelmCrew = &other.Mobile
	boat.OwnerPlayerID = 10
	if !m.Steers(&helm.Mobile, boat) {
		t.Error("the owner must steer while crew holds the helm")
	}
	// ... but the owner's deck click is not consumed then
	// (they walk the deck as usual).
	if m.SteerToward(&helm.Mobile, 9604, 9608) {
		t.Error("owner deck click with crew at helm must not be consumed")
	}
}

func TestBoatManagerIsSailable(t *testing.T) {
	water := &stubCollision{
		water: map[[2]int]bool{{3000, 3000}: true, {3001, 3000}: true},
		clips: map[[3]int]int32{{3001, 3000, 0}: solidObjectClip},
	}
	m := NewBoatManager(water)
	if !m.IsSailable(3000, 3000, 0) {
		t.Error("open water must be sailable")
	}
	if m.IsSailable(3001, 3000, 0) {
		t.Error("water with a solid object must not be sailable")
	}
	if m.IsSailable(3002, 3000, 0) {
		t.Error("land must not be sailable")
	}
	if m.IsSailable(3000, 3000, 1) {
		t.Error("another level must not be sailable")
	}
	if NewBoatManager(nil).IsSailable(3000, 3000, 0) {
		t.Error("a nil collision source must read no water")
	}
}

func TestBoatManagerTeleport(t *testing.T) {
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	angle := 512
	m.Teleport(boat, BoatTeleportTarget{FineX: 100, FineY: 200, Angle: &angle})
	if boat.FineX != 100 || boat.FineY != 200 || boat.PrevFineX != 100 || boat.PrevFineY != 200 {
		t.Errorf("teleported fine = (%d, %d) prev = (%d, %d)", boat.FineX, boat.FineY, boat.PrevFineX, boat.PrevFineY)
	}
	if boat.Angle != 512 || boat.Heading != 512 || boat.TeleportCount != 1 {
		t.Errorf("teleported angle/heading/count = %d/%d/%d", boat.Angle, boat.Heading, boat.TeleportCount)
	}
	m.Teleport(boat, BoatTeleportTarget{FineX: 300, FineY: 400})
	if boat.Angle != 512 {
		t.Error("a teleport without an angle must keep the facing")
	}
	if boat.TeleportCount != 2 {
		t.Errorf("teleport count = %d, want 2", boat.TeleportCount)
	}
}

func TestBoatManagerTick(t *testing.T) {
	water := &stubCollision{water: map[[2]int]bool{}}
	for x := 2990; x <= 3010; x++ {
		for y := 2990; y <= 3010; y++ {
			water.water[[2]int{x, y}] = true
		}
	}
	m := NewBoatManager(water)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	boat.MoveMode = BoatMoveModeFull
	after := 0
	m.OnAfterTick(func() { after++ })
	startY := boat.FineY
	m.Tick()
	if boat.PrevFineY != startY {
		t.Error("tick must record the previous position")
	}
	if boat.LastStep == nil || !boat.LastStep.Moved {
		t.Errorf("last step = %+v, want moved", boat.LastStep)
	}
	if after != 1 {
		t.Errorf("after-tick listeners ran %d times, want 1", after)
	}
}

func TestBoatManagerRefreshRegionsUnderBoats(t *testing.T) {
	m := NewBoatManager(nil)
	if m.RefreshRegionsUnderBoats() {
		t.Error("no boats must report no change")
	}
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	if !m.RefreshRegionsUnderBoats() {
		t.Error("a new boat must report a change")
	}
	if m.RefreshRegionsUnderBoats() {
		t.Error("an unmoved boat must report no change")
	}
	boat.FineX += 64 * FineUnitsPerTile
	if !m.RefreshRegionsUnderBoats() {
		t.Error("a boat crossing a 64-tile square must report a change")
	}
	// Static boats never report.
	static := m.SpawnStatic(0, testBoatSpec(), testPlacement())
	static.FineX += 128 * FineUnitsPerTile
	if m.RefreshRegionsUnderBoats() {
		t.Error("a moved static boat must report no change")
	}
}

func TestWorldHelpersDegenerate(t *testing.T) {
	w := NewWorld(Location{X: 3089, Y: 3524, Z: 0})
	p := NewPlayer("p", Location{X: 3000, Y: 3000})
	if w.rootLocationOf(&p.Mobile) != p.Location() {
		t.Error("no manager: root must be the own tile")
	}
	if w.syncAreaOf(&p.Mobile) != nil {
		t.Error("no manager: sync area must be nil")
	}
	if !w.canSeeAboardBy(&p.Mobile, &p.Mobile) {
		t.Error("no manager: aboard must be visible")
	}
	if w.boatAboard(&p.Mobile) != nil {
		t.Error("no manager: aboard must be nil")
	}
}

func TestIsDeckTile(t *testing.T) {
	if !IsDeckTile(9600, 9600) || !IsDeckTile(20000, 20000) {
		t.Error("deck-space tiles must report deck")
	}
	if IsDeckTile(9599, 9600) || IsDeckTile(9600, 9599) || IsDeckTile(3000, 3000) {
		t.Error("main-world tiles must not report deck")
	}
}
