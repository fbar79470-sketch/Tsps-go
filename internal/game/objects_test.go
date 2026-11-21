package game

import (
	"math"
	"testing"
)

// Fixtures-only object registry tests: hand-built GameObjects and
// fake region streams, no network, no cache binaries. Region
// fixtures reuse the regions_test.go builders (regionTestID,
// buildTestTerrain, buildTestObjectStream, fakeRegionSource,
// fakeObjectDefs) for the tile (3200, 3200) region.

var objectTestLoc = Location{X: 3200, Y: 3200, Z: 0}

func newObjectRegistryWithRegion(t *testing.T, terrain, objects []byte, defs map[int]ObjectDef) (*World, *ObjectRegistry) {
	t.Helper()
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{terrain: terrain, objects: objects, ok: true}
	w.SetRegionManager(NewRegionManager(src, &fakeObjectDefs{defs: defs}, true))
	return w, w.GameObjects()
}

func objectIDs(objs []*GameObject) []int {
	ids := make([]int, len(objs))
	for i, obj := range objs {
		ids[i] = obj.ID()
	}
	return ids
}

func TestGameObjectAccessorsAndFlags(t *testing.T) {
	area := &PrivateArea{}
	obj := NewGameObject(100, objectTestLoc, 10, 2, area)
	if obj.ID() != 100 || obj.Location() != objectTestLoc || obj.Type() != 10 || obj.Face() != 2 {
		t.Errorf("accessors: id=%d loc=%+v type=%d face=%d", obj.ID(), obj.Location(), obj.Type(), obj.Face())
	}
	if obj.PrivateArea() != area {
		t.Error("PrivateArea not stored")
	}
	if obj.Owner() != nil || obj.SpawnedBy() != "" || obj.IsBaseMap() || obj.IsMapOriginal() {
		t.Errorf("defaults: owner=%v spawnedBy=%q baseMap=%v mapOriginal=%v", obj.Owner(), obj.SpawnedBy(), obj.IsBaseMap(), obj.IsMapOriginal())
	}
	obj.SetType(4)
	obj.SetFace(1)
	if obj.Type() != 4 || obj.Face() != 1 {
		t.Errorf("setters: type=%d face=%d", obj.Type(), obj.Face())
	}
	if got := obj.MarkBaseMap(); got != obj || !obj.IsBaseMap() {
		t.Error("MarkBaseMap must chain and set the flag")
	}
	plain := NewGameObject(101, objectTestLoc, 10, 0, nil)
	if got := plain.MarkMapOriginal(); got != plain || !plain.IsMapOriginal() || !plain.IsBaseMap() {
		t.Error("MarkMapOriginal must chain and set both flags (GameObject.ts:57-60)")
	}
	player := NewPlayer("alice", objectTestLoc)
	if got := obj.SetOwner(player); got != obj || obj.Owner() != player {
		t.Error("SetOwner must chain and set the owner")
	}
	if got := obj.SetSpawnedBy("Doors"); got != obj || obj.SpawnedBy() != "Doors" {
		t.Error("SetSpawnedBy must chain and set the name")
	}
	obj.SetOwner(nil)
	obj.SetSpawnedBy("")
	if obj.Owner() != nil || obj.SpawnedBy() != "" {
		t.Error("nil/empty setters must clear")
	}
}

func TestGameObjectEquals(t *testing.T) {
	area := &PrivateArea{}
	base := NewGameObject(100, objectTestLoc, 10, 2, area)
	same := NewGameObject(100, objectTestLoc, 10, 2, area)
	if !base.Equals(same) {
		t.Error("identical fields must be equal")
	}
	// Owner, spawnedBy, and flags are NOT compared (GameObject.ts:157-163).
	same.SetOwner(NewPlayer("bob", objectTestLoc)).SetSpawnedBy("Doors").MarkMapOriginal()
	if !base.Equals(same) {
		t.Error("owner/spawnedBy/flags differences must not affect Equals")
	}
	for _, tc := range []struct {
		name  string
		other *GameObject
	}{
		{"id", NewGameObject(101, objectTestLoc, 10, 2, area)},
		{"type", NewGameObject(100, objectTestLoc, 11, 2, area)},
		{"face", NewGameObject(100, objectTestLoc, 10, 3, area)},
		{"loc", NewGameObject(100, Location{X: 3201, Y: 3200, Z: 0}, 10, 2, area)},
		{"area", NewGameObject(100, objectTestLoc, 10, 2, &PrivateArea{})},
		{"nil area", NewGameObject(100, objectTestLoc, 10, 2, nil)},
		{"nil", nil},
	} {
		if base.Equals(tc.other) {
			t.Errorf("%s: Equals must be false", tc.name)
		}
	}
	var nilObj *GameObject
	if nilObj.Equals(base) {
		t.Error("nil receiver must not equal")
	}
}

func TestGameObjectClone(t *testing.T) {
	area := &PrivateArea{}
	player := NewPlayer("alice", objectTestLoc)
	orig := NewGameObject(100, objectTestLoc, 10, 2, area)
	orig.SetOwner(player).SetSpawnedBy("Doors").MarkMapOriginal()
	clone := orig.Clone()
	if clone == orig {
		t.Fatal("Clone must return a new instance")
	}
	if !orig.Equals(clone) || clone.Owner() != player || clone.PrivateArea() != area {
		t.Error("Clone must carry id/loc/type/face/area/owner")
	}
	// TS clone constructs fresh: flags and spawnedBy do not carry.
	if clone.IsMapOriginal() || clone.IsBaseMap() || clone.SpawnedBy() != "" {
		t.Errorf("Clone must reset flags/spawnedBy: mapOriginal=%v baseMap=%v spawnedBy=%q",
			clone.IsMapOriginal(), clone.IsBaseMap(), clone.SpawnedBy())
	}
}

func TestGameObjectSize(t *testing.T) {
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	if got := obj.Size(ObjectDef{SizeX: 3, SizeY: 2}); got != 4 {
		t.Errorf("Size(3x2) = %d, want 4", got)
	}
	if got := obj.Size(ObjectDef{SizeX: 1, SizeY: 1}); got != 1 {
		t.Errorf("Size(1x1) = %d, want 1", got)
	}
	if got := obj.Size(ObjectDef{}); got != 1 {
		t.Errorf("Size(no def) = %d, want 1 (null-definition branch)", got)
	}
}

func TestGameObjectFootprintDistance(t *testing.T) {
	def12 := ObjectDef{SizeX: 3, SizeY: 2}
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil) // footprint x 3200-3202, y 3200-3201
	cases := []struct {
		name string
		from Location
		def  ObjectDef
		face int
		want int
	}{
		{"on footprint", Location{X: 3201, Y: 3200, Z: 0}, def12, 0, 0},
		{"adjacent east", Location{X: 3203, Y: 3200, Z: 0}, def12, 0, 1},
		{"two east", Location{X: 3204, Y: 3200, Z: 0}, def12, 0, 2},
		{"diagonal", Location{X: 3204, Y: 3203, Z: 0}, def12, 0, 2},
		{"other plane", Location{X: 3200, Y: 3200, Z: 1}, def12, 0, math.MaxInt},
		// Face 1 swaps sizes: footprint x 3200-3201, y 3200-3202.
		{"turned east edge", Location{X: 3202, Y: 3200, Z: 0}, def12, 1, 1},
		{"turned north edge", Location{X: 3200, Y: 3203, Z: 0}, def12, 1, 1},
		{"turned on footprint", Location{X: 3201, Y: 3202, Z: 0}, def12, 1, 0},
		{"no def reads 1x1", Location{X: 3202, Y: 3200, Z: 0}, ObjectDef{}, 0, 2},
	}
	for _, tc := range cases {
		obj.SetFace(tc.face)
		if got := obj.FootprintDistance(tc.from, tc.def); got != tc.want {
			t.Errorf("%s: FootprintDistance = %d, want %d", tc.name, got, tc.want)
		}
	}
	obj.SetFace(0)
	if !obj.IsWithinInteractionDistance(Location{X: 3204, Y: 3200, Z: 0}, def12) {
		t.Error("distance 2 must be within interaction distance")
	}
	if obj.IsWithinInteractionDistance(Location{X: 3205, Y: 3200, Z: 0}, def12) {
		t.Error("distance 3 must not be within interaction distance")
	}
	if obj.IsWithinInteractionDistance(Location{X: 3200, Y: 3200, Z: 1}, def12) {
		t.Error("other plane must not be within interaction distance")
	}
}

func TestRegisterAddsToWorldAndTileMap(t *testing.T) {
	r := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	if got := r.At(objectTestLoc); len(got) != 1 || got[0] != obj {
		t.Errorf("At = %v, want [obj]", objectIDs(got))
	}
	if got := r.WorldObjects(); len(got) != 1 || got[0] != obj {
		t.Errorf("WorldObjects = %v, want [obj]", objectIDs(got))
	}
	if got := r.Get(100, objectTestLoc); got != obj {
		t.Errorf("Get = %v, want obj", got)
	}
	deltas := r.Deltas()
	if len(deltas) != 1 || deltas[0].Object != obj || deltas[0].Removed {
		t.Errorf("Deltas = %+v, want one spawn delta", deltas)
	}
	// playerUpdate=false: world indexes only, no tile map, no delta.
	quiet := NewGameObject(101, Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	r.Register(quiet, false)
	if got := r.At(quiet.Location()); len(got) != 1 {
		t.Errorf("quiet At = %d, want 1", len(got))
	}
	if got := r.Get(101, quiet.Location()); got != nil {
		t.Errorf("quiet Get = %v, want nil (no perform)", got)
	}
	if got := len(r.Deltas()); got != 1 {
		t.Errorf("Deltas after quiet register = %d, want 1", got)
	}
	// Nil register is ignored.
	r.Register(nil, true)
	if got := len(r.WorldObjects()); got != 2 {
		t.Errorf("WorldObjects after nil register = %d, want 2", got)
	}
}

func TestRegisterSkipsMapOriginalInWorldIndex(t *testing.T) {
	r := NewObjectRegistry()
	orig := NewGameObject(100, objectTestLoc, 10, 0, nil)
	orig.MarkMapOriginal()
	r.Register(orig, true)
	if got := r.At(objectTestLoc); len(got) != 0 {
		t.Errorf("map original in world index: %v", objectIDs(got))
	}
	if got := len(r.WorldObjects()); got != 0 {
		t.Errorf("WorldObjects = %d, want 0", got)
	}
	if got := r.Get(100, objectTestLoc); got != orig {
		t.Error("map original must still enter the tile map via perform")
	}
	// ...unless instanced: a map original with an area IS indexed.
	instanced := NewGameObject(101, objectTestLoc, 10, 0, &PrivateArea{})
	instanced.MarkMapOriginal()
	r.Register(instanced, true)
	if got := r.At(objectTestLoc); len(got) != 1 || got[0] != instanced {
		t.Errorf("instanced map original At = %v, want [instanced]", objectIDs(got))
	}
}

func TestRegisterReplacesSameShape(t *testing.T) {
	r := NewObjectRegistry()
	first := NewGameObject(100, objectTestLoc, 10, 0, nil)
	second := NewGameObject(101, objectTestLoc, 10, 0, nil)
	r.Register(first, true)
	r.Register(second, true)
	if got := r.At(objectTestLoc); len(got) != 1 || got[0] != second {
		t.Errorf("At after replacement = %v, want [101]", objectIDs(got))
	}
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("predecessor must leave the tile map when playerUpdate is true")
	}
	if got := r.Get(101, objectTestLoc); got != second {
		t.Error("replacement must enter the tile map")
	}
	// The replacement's tile-map removal records NO delta of its own:
	// two spawns, nothing else.
	deltas := r.Deltas()
	if len(deltas) != 2 || deltas[0].Removed || deltas[1].Removed {
		t.Errorf("Deltas = %+v, want two spawn deltas", deltas)
	}

	// playerUpdate=false replacement: the predecessor leaves the
	// world index but KEEPS its tile-map entry and clipping.
	r2 := NewObjectRegistry()
	a := NewGameObject(100, objectTestLoc, 10, 0, nil)
	b := NewGameObject(101, objectTestLoc, 10, 0, nil)
	r2.Register(a, true)
	r2.Register(b, false)
	if got := r2.At(objectTestLoc); len(got) != 1 || got[0] != b {
		t.Errorf("quiet replacement At = %v, want [101]", objectIDs(got))
	}
	if got := r2.Get(100, objectTestLoc); got != a {
		t.Error("quiet replacement must keep the predecessor in the tile map")
	}
	if got := r2.Get(101, objectTestLoc); got != nil {
		t.Error("quiet replacement must not enter the tile map")
	}
}

func TestRegisterStacksDifferentTypeAndOwner(t *testing.T) {
	r := NewObjectRegistry()
	wall := NewGameObject(100, objectTestLoc, 0, 0, nil)
	solid := NewGameObject(101, objectTestLoc, 10, 0, nil)
	owned := NewGameObject(102, objectTestLoc, 10, 0, nil)
	owned.SetOwner(NewPlayer("alice", objectTestLoc))
	r.Register(wall, true)
	r.Register(solid, true)
	r.Register(owned, true)
	if got := r.At(objectTestLoc); len(got) != 3 {
		t.Errorf("At = %v, want 3 stacked objects", objectIDs(got))
	}
	if got := r.Get(100, objectTestLoc); got != wall {
		t.Error("wall must be in the tile map")
	}
	if got := r.Get(101, objectTestLoc); got != solid {
		t.Error("solid must be in the tile map")
	}
	if got := r.Get(102, objectTestLoc); got != nil {
		t.Error("owned object must never enter the tile map")
	}
}

func TestRegisterSameInstanceTwice(t *testing.T) {
	r := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	r.Register(obj, true)
	if got := r.At(objectTestLoc); len(got) != 1 {
		t.Errorf("At = %d, want 1", len(got))
	}
	if got := len(r.WorldObjects()); got != 1 {
		t.Errorf("WorldObjects = %d, want 1", got)
	}
	if got := len(r.ObjectsAtTile(objectTestLoc)); got != 1 {
		t.Errorf("tile map entries = %d, want 1 (identity dedup)", got)
	}
}

func TestRegisterNegativeIDDespawns(t *testing.T) {
	// TS perform retypes id -1 to DESPAWN (ObjectManager.ts:358-360),
	// but register still adds the object to the world list first.
	r := NewObjectRegistry()
	standing := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(standing, true)
	remover := NewGameObject(-1, objectTestLoc, 10, 0, nil)
	r.Register(remover, true)
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("id -1 register must remove the standing object from the tile map (field match)")
	}
	deltas := r.Deltas()
	if len(deltas) != 2 || !deltas[1].Removed || deltas[1].Object != remover {
		t.Errorf("Deltas = %+v, want spawn then removal", deltas)
	}
	if got := len(r.WorldObjects()); got != 1 {
		t.Errorf("WorldObjects = %d, want 1 (the -1 object itself, TS quirk)", got)
	}
}

func TestDeregisterRemovesAndDeltas(t *testing.T) {
	r := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	r.Deregister(obj, true)
	if got := r.At(objectTestLoc); len(got) != 0 {
		t.Errorf("At after deregister = %v", objectIDs(got))
	}
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("Get after deregister must be nil")
	}
	deltas := r.Deltas()
	if len(deltas) != 2 || deltas[0].Removed || !deltas[1].Removed {
		t.Errorf("Deltas = %+v, want spawn then removal", deltas)
	}
	// playerUpdate=false: world index leaves, tile map keeps, no delta.
	r2 := NewObjectRegistry()
	obj2 := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r2.Register(obj2, true)
	r2.Deregister(obj2, false)
	if got := r2.At(objectTestLoc); len(got) != 0 {
		t.Errorf("quiet deregister At = %v", objectIDs(got))
	}
	if got := r2.Get(100, objectTestLoc); got != obj2 {
		t.Error("quiet deregister must keep the tile-map entry")
	}
	if got := len(r2.Deltas()); got != 1 {
		t.Errorf("Deltas after quiet deregister = %d, want 1", got)
	}
	r2.Deregister(nil, true) // ignored, no panic
}

func TestDeregisterRemovedListOnlyForMapObjects(t *testing.T) {
	r := NewObjectRegistry()
	runtime := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(runtime, true)
	r.Deregister(runtime, true)
	if got := len(r.RemovedObjects()); got != 0 {
		t.Errorf("runtime object remembered: %d entries", got)
	}
	// A base-map object IS remembered, deduplicated by identity.
	base := NewGameObject(101, objectTestLoc, 10, 0, nil)
	base.MarkBaseMap()
	r.Register(base, true)
	r.Deregister(base, true)
	r.Deregister(base, true)
	removed := r.RemovedObjects()
	if len(removed) != 1 || removed[0] != base {
		t.Errorf("RemovedObjects = %v, want [base] once", objectIDs(removed))
	}
	// A private-area object counts as a map object for this purpose.
	r2 := NewObjectRegistry()
	instanced := NewGameObject(102, objectTestLoc, 10, 0, &PrivateArea{})
	r2.Register(instanced, true)
	r2.Deregister(instanced, true)
	if got := len(r2.RemovedObjects()); got != 1 {
		t.Errorf("instanced deregister RemovedObjects = %d, want 1", got)
	}
}

func TestRegisterOverRemovedSpot(t *testing.T) {
	r := NewObjectRegistry()
	base := NewGameObject(100, objectTestLoc, 10, 0, nil)
	base.MarkBaseMap()
	r.Register(base, true)
	r.Deregister(base, true)
	if got := len(r.RemovedObjects()); got != 1 {
		t.Fatalf("RemovedObjects = %d, want 1", got)
	}
	replacement := NewGameObject(101, objectTestLoc, 10, 0, nil)
	r.Register(replacement, true)
	if got := len(r.RemovedObjects()); got != 0 {
		t.Errorf("RemovedObjects after re-register = %d, want 0 (spliced)", got)
	}
	if !replacement.IsBaseMap() {
		t.Error("replacement taking a removed map object's place must be marked base-map")
	}
	// A different shape does NOT splice the removed entry.
	r2 := NewObjectRegistry()
	base2 := NewGameObject(100, objectTestLoc, 10, 0, nil)
	base2.MarkBaseMap()
	r2.Register(base2, true)
	r2.Deregister(base2, true)
	r2.Register(NewGameObject(101, objectTestLoc, 4, 0, nil), true)
	if got := len(r2.RemovedObjects()); got != 1 {
		t.Errorf("RemovedObjects after different-shape register = %d, want 1", got)
	}
}

func TestOwnerObjectsAndDiscard(t *testing.T) {
	r := NewObjectRegistry()
	alice := NewPlayer("alice", objectTestLoc)
	bob := NewPlayer("bob", objectTestLoc)
	owned := NewGameObject(100, objectTestLoc, 10, 0, nil)
	owned.SetOwner(alice)
	r.Register(owned, true)
	if got := r.Find(alice, 100, objectTestLoc); got != owned {
		t.Error("owner must find their object")
	}
	if got := r.Find(bob, 100, objectTestLoc); got != nil {
		t.Error("another player must not find an owned object")
	}
	if got := r.OwnerObjectsInScene(alice, 3200, 3200, 104); len(got) != 1 {
		t.Errorf("owner scene = %d, want 1", len(got))
	}
	if got := r.OwnerObjectsInScene(bob, 3200, 3200, 104); len(got) != 0 {
		t.Errorf("other player scene = %d, want 0", len(got))
	}
	// Owner objects ARE in the world indexes (addToWorld is
	// unconditional), even though they never enter the tile map.
	if got := r.At(objectTestLoc); len(got) != 1 {
		t.Errorf("At = %d, want 1 (owner object indexed)", len(got))
	}
	// DiscardPlayer drops only that player's owner entries.
	r.DiscardPlayer(alice)
	if got := r.Find(alice, 100, objectTestLoc); got != nil {
		t.Error("discarded player's object must be gone from the owner map")
	}
	if got := r.At(objectTestLoc); len(got) != 1 {
		t.Error("DiscardPlayer must not touch the world indexes (TS WeakMap behavior)")
	}
	r.DiscardPlayer(bob) // unknown player: no-op, no panic
	r.DiscardPlayer(nil)
}

func TestPrivateAreaObjectSkipsTileMapAndClipping(t *testing.T) {
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil), nil,
		map[int]ObjectDef{100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true}})
	obj := NewGameObject(100, objectTestLoc, 10, 0, &PrivateArea{})
	r.Register(obj, true)
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("private-area object must not enter the tile map")
	}
	if got := r.At(objectTestLoc); len(got) != 1 {
		t.Error("private-area object must enter the world index")
	}
	// No shared-grid clipping: TS clips it on a private grid that
	// does not exist in Go (documented collapse).
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping = %#x, want 0 (private-area object clips no shared grid)", got)
	}
}

func TestMapAddRemoveIdentityAndFieldSemantics(t *testing.T) {
	// In-package access to the MapObjects.add/remove halves.
	r := NewObjectRegistry()
	a := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.mapAdd(a)
	r.mapAdd(a) // identity dedup
	if got := len(r.ObjectsAtTile(objectTestLoc)); got != 1 {
		t.Errorf("tile entries after duplicate add = %d, want 1", got)
	}
	b := NewGameObject(100, objectTestLoc, 10, 0, nil) // distinct instance, same fields
	r.mapAdd(b)
	if got := len(r.ObjectsAtTile(objectTestLoc)); got != 2 {
		t.Errorf("tile entries after field-identical add = %d, want 2", got)
	}
	// Field-match removal clears EVERY matching entry.
	r.mapRemove(a)
	if got := len(r.ObjectsAtTile(objectTestLoc)); got != 0 {
		t.Errorf("tile entries after field-match remove = %d, want 0", got)
	}
}

func TestPreferReachable(t *testing.T) {
	r := NewObjectRegistry()
	deco := NewGameObject(100, objectTestLoc, 4, 0, nil)
	solid := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.mapAdd(deco)
	r.mapAdd(solid)
	if got := r.Get(100, objectTestLoc); got != solid {
		t.Error("get must prefer the non-decoration copy (preferReachable)")
	}
	r2 := NewObjectRegistry()
	only := NewGameObject(100, objectTestLoc, 5, 0, nil)
	r2.mapAdd(only)
	if got := r2.Get(100, objectTestLoc); got != only {
		t.Error("a lone decoration must still be returned")
	}
}

func TestGetTypeFirstMatchAndClamp(t *testing.T) {
	r := NewObjectRegistry()
	first := NewGameObject(100, objectTestLoc, 10, 0, nil)
	second := NewGameObject(101, objectTestLoc, 10, 1, nil)
	r.mapAdd(first)
	r.mapAdd(second)
	if got := r.GetType(objectTestLoc, 10); got != first {
		t.Error("GetType must return the first match in tile-map order")
	}
	if got := r.GetType(objectTestLoc, 4); got != nil {
		t.Error("GetType for an absent shape must be nil")
	}
	// Plane >= 4 clamps to plane 0.
	high := Location{X: 3200, Y: 3200, Z: 4}
	if got := r.GetType(high, 10); got != first {
		t.Error("GetType must clamp z>=4 to plane 0")
	}
	if got := r.Get(100, high); got != first {
		t.Error("Get must clamp z>=4 to plane 0")
	}
}

// fakeVariantSource is an ObjectVariantSource test double: mapping
// rewrites ids, missing reports forPlayer-null.
type fakeVariantSource struct {
	mapping map[int]int
	missing map[int]bool
	seenNil bool
	seenSet bool
}

func (f *fakeVariantSource) ObjectVariantForPlayer(id int, vars ObjectVarReader) (int, bool) {
	if vars == nil {
		f.seenNil = true
	} else {
		f.seenSet = true
	}
	if f.missing[id] {
		return 0, false
	}
	if v, ok := f.mapping[id]; ok {
		return v, true
	}
	return id, true
}

type fakeVarReader struct{ varbits map[int]int }

func (f fakeVarReader) GetVarp(id int) int   { return 0 }
func (f fakeVarReader) GetVarbit(id int) int { return f.varbits[id] }

func TestFindVariantResolution(t *testing.T) {
	player := NewPlayer("alice", objectTestLoc)
	r := NewObjectRegistry()
	entry := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.mapAdd(entry)
	// Without a source, resolution is the identity.
	if got := r.Find(player, 100, objectTestLoc); got != entry {
		t.Error("identity resolution must find the entry by its own id")
	}
	if got := r.Find(player, 101, objectTestLoc); got != nil {
		t.Error("identity resolution must not find another id")
	}
	// With a source mapping 100 -> 101, the entry answers for 101
	// (fallback scan) and still answers for 100 (raw get + guard).
	r.SetVariantSource(&fakeVariantSource{mapping: map[int]int{100: 101}})
	if got := r.Find(player, 101, objectTestLoc); got != entry {
		t.Error("variant resolution must find the entry under the variant id")
	}
	if got := r.Find(player, 100, objectTestLoc); got != entry {
		t.Error("raw get + non-null guard must still find the entry under its base id")
	}
	// A forPlayer-null entry is invisible to Find.
	r2 := NewObjectRegistry()
	hidden := NewGameObject(200, objectTestLoc, 10, 0, nil)
	r2.mapAdd(hidden)
	r2.SetVariantSource(&fakeVariantSource{missing: map[int]bool{200: true}})
	if got := r2.Find(player, 200, objectTestLoc); got != nil {
		t.Error("final forPlayer guard must null a null-resolving entry")
	}
}

func TestFindVarsSourcePlumbing(t *testing.T) {
	player := NewPlayer("alice", objectTestLoc)
	r := NewObjectRegistry()
	entry := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.mapAdd(entry)
	// A source that resolves through the var reader: id 100 reads
	// varbit 5 as its variant.
	varSource := &varsDrivenSource{}
	r.SetVariantSource(varSource)
	// No vars source: vars read nil, the source answers the base id.
	if got := r.Find(player, 100, objectTestLoc); got != entry {
		t.Error("nil vars must degrade to the base id")
	}
	if !varSource.sawNil {
		t.Error("variant source must receive a nil reader when no vars source is installed")
	}
	// With a vars source, varbit 5 = 77 resolves the entry to 77.
	r.SetVarsSource(func(p *Player) ObjectVarReader {
		return fakeVarReader{varbits: map[int]int{5: 77}}
	})
	if got := r.Find(player, 77, objectTestLoc); got != entry {
		t.Error("vars-driven resolution must find the entry under the varbit variant")
	}
}

// varsDrivenSource resolves id 100 to varbit 5 when vars exist,
// else to itself.
type varsDrivenSource struct{ sawNil bool }

func (s *varsDrivenSource) ObjectVariantForPlayer(id int, vars ObjectVarReader) (int, bool) {
	if vars == nil {
		s.sawNil = true
		return id, true
	}
	if id == 100 {
		return vars.GetVarbit(5), true
	}
	return id, true
}

func TestFindOwnerPriorityOverShared(t *testing.T) {
	alice := NewPlayer("alice", objectTestLoc)
	owned := NewGameObject(100, objectTestLoc, 10, 0, nil)
	owned.SetOwner(alice)
	shared := NewGameObject(101, objectTestLoc, 10, 0, nil)
	r := NewObjectRegistry()
	r.mapAdd(owned)
	r.mapAdd(shared)
	// Variant source maps the owned entry's id to the shared id:
	// the owner branch wins before the shared scan runs.
	r.SetVariantSource(&fakeVariantSource{mapping: map[int]int{100: 101}})
	if got := r.Find(alice, 101, objectTestLoc); got != owned {
		t.Error("owner branch must win over the shared scan")
	}
}

func TestFindBridgeTileFallback(t *testing.T) {
	player := NewPlayer("alice", objectTestLoc)
	r := NewObjectRegistry()
	ground := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.mapAdd(ground)
	above := Location{X: 3200, Y: 3200, Z: 1}
	if got := r.Find(player, 100, above); got != ground {
		t.Error("z-1 fallback must find a bridge-tile loc from the plane above")
	}
	twoAbove := Location{X: 3200, Y: 3200, Z: 2}
	if got := r.Find(player, 100, twoAbove); got != nil {
		t.Error("fallback drops exactly one plane")
	}
	if got := r.Find(player, 100, objectTestLoc); got != ground {
		t.Error("direct lookup must find the loc on its own plane")
	}
}

func TestFindDeveloperNotice(t *testing.T) {
	missing := Location{X: 3210, Y: 3210, Z: 0}
	dev := NewPlayer("dev", objectTestLoc)
	devSession := newFakeSession()
	dev.Session = devSession
	r := NewObjectRegistry()
	r.SetRightsSource(func(p *Player) int { return RightsDeveloper })
	if got := r.Find(dev, 42, missing); got != nil {
		t.Error("Find of a missing loc must be nil")
	}
	if len(devSession.messages) != 1 || devSession.messages[0] != "@red@Object with id 42 does not exist." {
		t.Errorf("developer notice = %v, want the exact TS text", devSession.messages)
	}
	// Non-developers get no notice.
	plain := NewPlayer("plain", objectTestLoc)
	plainSession := newFakeSession()
	plain.Session = plainSession
	r.SetRightsSource(func(p *Player) int {
		if p == dev {
			return RightsDeveloper
		}
		return RightsNone
	})
	r.Find(plain, 42, missing)
	if len(plainSession.messages) != 0 {
		t.Errorf("non-developer notice = %v, want none", plainSession.messages)
	}
	// No rights source: no notice for anyone.
	r2 := NewObjectRegistry()
	devSession.messages = nil
	r2.Find(dev, 42, missing)
	if len(devSession.messages) != 0 {
		t.Errorf("no-rights-source notice = %v, want none", devSession.messages)
	}
	// A found object draws no notice even for a developer.
	r3 := NewObjectRegistry()
	r3.SetRightsSource(func(p *Player) int { return RightsDeveloper })
	r3.mapAdd(NewGameObject(42, missing, 10, 0, nil))
	devSession.messages = nil
	if got := r3.Find(dev, 42, missing); got == nil {
		t.Fatal("Find must return the standing object")
	}
	if len(devSession.messages) != 0 {
		t.Errorf("found-object notice = %v, want none", devSession.messages)
	}
	// Nil player: no panic, no notice.
	if got := r.Find(nil, 42, missing); got != nil {
		t.Error("nil-player Find of a missing loc must be nil")
	}
}

func TestStillStands(t *testing.T) {
	player := NewPlayer("alice", objectTestLoc)
	r := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	if !r.StillStands(player, 100, objectTestLoc, obj, false) {
		t.Error("same instance must still stand")
	}
	// A different instance with equal fields still stands.
	lookalike := NewGameObject(100, objectTestLoc, 10, 0, nil)
	if !r.StillStands(player, 100, objectTestLoc, lookalike, false) {
		t.Error("equal-fields replacement must still stand")
	}
	// Changed shape: gone.
	r.Deregister(obj, true)
	r.Register(NewGameObject(100, objectTestLoc, 4, 0, nil), true)
	if r.StillStands(player, 100, objectTestLoc, obj, false) {
		t.Error("changed shape must not still stand")
	}
	// Missing entirely: gone.
	r2 := NewObjectRegistry()
	if r2.StillStands(player, 100, objectTestLoc, obj, false) {
		t.Error("missing loc must not still stand")
	}
	// Nil object, missing loc: current === object (both absent).
	if !r2.StillStands(player, 100, objectTestLoc, nil, false) {
		t.Error("nil object with missing loc must still stand (both absent)")
	}
	// quiet suppresses the developer notice; loud sends it.
	dev := NewPlayer("dev", objectTestLoc)
	devSession := newFakeSession()
	dev.Session = devSession
	r2.SetRightsSource(func(p *Player) int { return RightsDeveloper })
	r2.StillStands(dev, 100, objectTestLoc, obj, true)
	if len(devSession.messages) != 0 {
		t.Errorf("quiet StillStands sent %v", devSession.messages)
	}
	r2.StillStands(dev, 100, objectTestLoc, obj, false)
	if len(devSession.messages) != 1 {
		t.Errorf("loud StillStands sent %v, want one notice", devSession.messages)
	}
}

func TestInScene(t *testing.T) {
	r := NewObjectRegistry()
	inside := NewGameObject(1, Location{X: 3200, Y: 3200, Z: 0}, 10, 0, nil)
	upstairs := NewGameObject(2, Location{X: 3200, Y: 3200, Z: 3}, 10, 0, nil)
	nextRegion := NewGameObject(3, Location{X: 3264, Y: 3200, Z: 0}, 10, 0, nil)
	edge := NewGameObject(4, Location{X: 3303, Y: 3303, Z: 0}, 10, 0, nil)
	outside := NewGameObject(5, Location{X: 3304, Y: 3200, Z: 0}, 10, 0, nil)
	owned := NewGameObject(6, Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	owned.SetOwner(NewPlayer("alice", objectTestLoc))
	for _, obj := range []*GameObject{inside, upstairs, nextRegion, edge, outside, owned} {
		r.Register(obj, true)
	}
	got := r.InScene(3200, 3200, 104)
	want := map[int]bool{1: true, 2: true, 3: true, 4: true}
	if len(got) != len(want) {
		t.Fatalf("InScene ids = %v, want %v", objectIDs(got), want)
	}
	for _, obj := range got {
		if !want[obj.ID()] {
			t.Errorf("InScene included id %d", obj.ID())
		}
	}
	if got := r.InScene(3200, 3200, 0); len(got) != 0 {
		t.Errorf("zero-size scene = %d, want 0", len(got))
	}
}

func TestInRegion(t *testing.T) {
	r := NewObjectRegistry()
	a := NewGameObject(1, Location{X: 3200, Y: 3200, Z: 0}, 10, 0, nil)
	b := NewGameObject(2, Location{X: 3200, Y: 3200, Z: 2}, 10, 0, nil)
	c := NewGameObject(3, Location{X: 3264, Y: 3200, Z: 0}, 10, 0, nil)
	r.Register(a, true)
	r.Register(b, true)
	r.Register(c, true)
	// 3200>>6 = 50, 3264>>6 = 51: all planes in one region.
	if got := r.InRegion(50, 50); len(got) != 2 {
		t.Errorf("InRegion(50,50) = %v, want 2 (all planes)", objectIDs(got))
	}
	if got := r.InRegion(51, 50); len(got) != 1 || got[0] != c {
		t.Errorf("InRegion(51,50) = %v, want [3]", objectIDs(got))
	}
	if got := r.InRegion(0, 0); len(got) != 0 {
		t.Errorf("InRegion(0,0) = %v, want empty", objectIDs(got))
	}
}

func TestOwnerObjectsInSceneBounds(t *testing.T) {
	alice := NewPlayer("alice", objectTestLoc)
	r := NewObjectRegistry()
	near := NewGameObject(1, Location{X: 3200, Y: 3200, Z: 3}, 10, 0, nil)
	near.SetOwner(alice)
	far := NewGameObject(2, Location{X: 3304, Y: 3200, Z: 0}, 10, 0, nil)
	far.SetOwner(alice)
	r.Register(near, true)
	r.Register(far, true)
	got := r.OwnerObjectsInScene(alice, 3200, 3200, 104)
	if len(got) != 1 || got[0] != near {
		t.Errorf("OwnerObjectsInScene = %v, want [1] (all planes, x/y bounds)", objectIDs(got))
	}
}

func TestObjectsAtTile(t *testing.T) {
	r := NewObjectRegistry()
	a := NewGameObject(100, objectTestLoc, 10, 0, nil)
	b := NewGameObject(101, objectTestLoc, 4, 0, nil)
	r.mapAdd(a)
	r.mapAdd(b)
	got := r.ObjectsAtTile(objectTestLoc)
	if len(got) != 2 {
		t.Fatalf("ObjectsAtTile = %v, want 2", objectIDs(got))
	}
	high := Location{X: 3200, Y: 3200, Z: 4}
	if got := r.ObjectsAtTile(high); len(got) != 2 {
		t.Errorf("ObjectsAtTile z>=4 must clamp to plane 0: %v", objectIDs(got))
	}
	if got := r.ObjectsAtTile(Location{X: 3201, Y: 3200, Z: 0}); len(got) != 0 {
		t.Errorf("empty tile = %v", objectIDs(got))
	}
}

func TestExistsQuirk(t *testing.T) {
	r := NewObjectRegistry()
	if r.Exists(100, objectTestLoc) || r.ExistsLocation(objectTestLoc) {
		t.Error("empty tile must not exist")
	}
	r.Register(NewGameObject(100, objectTestLoc, 10, 0, nil), true)
	// The id argument is IGNORED (ObjectManager.ts:422-425).
	if !r.Exists(999, objectTestLoc) || !r.ExistsLocation(objectTestLoc) {
		t.Error("any runtime object on the tile counts, whatever its id")
	}
	// ExistsObject is identity: get() must return THIS instance.
	r2 := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r2.Register(obj, true)
	if !r2.ExistsObject(obj) {
		t.Error("registered instance must exist")
	}
	lookalike := NewGameObject(100, objectTestLoc, 10, 0, nil)
	if r2.ExistsObject(lookalike) {
		t.Error("an unregistered lookalike must not exist (get returns the registered instance)")
	}
	if r2.ExistsObject(nil) {
		t.Error("nil must not exist")
	}
}

func TestDeltasCopyAndClear(t *testing.T) {
	r := NewObjectRegistry()
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	got := r.Deltas()
	got[0].Object = nil // mutating the copy must not affect the registry
	if r.Deltas()[0].Object != obj {
		t.Error("Deltas must return a copy")
	}
	r.ClearDeltas()
	if len(r.Deltas()) != 0 {
		t.Error("ClearDeltas must empty the deltas")
	}
	r.ClearDeltas() // idempotent
}

func TestRemovePluginObjects(t *testing.T) {
	r := NewObjectRegistry()
	pluginObj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	pluginObj.SetSpawnedBy("Doors")
	coreObj := NewGameObject(101, Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	standIn := NewGameObject(102, Location{X: 3202, Y: 3200, Z: 0}, 10, 0, nil)
	standIn.SetSpawnedBy("Doors").MarkBaseMap()
	for _, obj := range []*GameObject{pluginObj, coreObj, standIn} {
		r.Register(obj, true)
	}
	r.ClearDeltas()
	if got := r.RemovePluginObjects(); got != 1 {
		t.Errorf("RemovePluginObjects = %d, want 1", got)
	}
	if got := r.At(objectTestLoc); len(got) != 0 {
		t.Error("plugin object must be deregistered")
	}
	if got := r.At(coreObj.Location()); len(got) != 1 {
		t.Error("core object must stay")
	}
	if got := r.At(standIn.Location()); len(got) != 1 {
		t.Error("base-map stand-in must stay")
	}
	deltas := r.Deltas()
	if len(deltas) != 1 || !deltas[0].Removed || deltas[0].Object != pluginObj {
		t.Errorf("Deltas = %+v, want one removal delta", deltas)
	}
	if got := r.RemovePluginObjects(); got != 0 {
		t.Errorf("second RemovePluginObjects = %d, want 0", got)
	}
}

func TestReplaceInPlace(t *testing.T) {
	r := NewObjectRegistry()
	outgoing := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(outgoing, true)
	r.ClearDeltas()
	incoming := NewGameObject(101, objectTestLoc, 10, 1, nil)
	r.ReplaceInPlace(outgoing, incoming)
	if got := r.At(objectTestLoc); len(got) != 1 || got[0] != incoming {
		t.Errorf("At after replace = %v, want [101]", objectIDs(got))
	}
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("outgoing must leave the tile map")
	}
	if got := r.Get(101, objectTestLoc); got != incoming {
		t.Error("incoming must enter the tile map")
	}
	// Exactly one spawn delta: no removal delta for the outgoing loc.
	deltas := r.Deltas()
	if len(deltas) != 1 || deltas[0].Removed || deltas[0].Object != incoming {
		t.Errorf("Deltas = %+v, want one spawn delta", deltas)
	}
	if got := len(r.RemovedObjects()); got != 0 {
		t.Errorf("runtime outgoing must not be remembered: %d", got)
	}
	// A map-object outgoing IS remembered, then the incoming
	// splice takes its place and inherits base-map.
	r2 := NewObjectRegistry()
	old := NewGameObject(100, objectTestLoc, 10, 0, nil)
	old.MarkBaseMap()
	r2.Register(old, true)
	r2.ReplaceInPlace(old, NewGameObject(101, objectTestLoc, 10, 1, nil))
	if got := len(r2.RemovedObjects()); got != 0 {
		t.Errorf("incoming must splice the outgoing removed entry: %d left", got)
	}
	if got := r2.At(objectTestLoc); len(got) != 1 || !got[0].IsBaseMap() {
		t.Error("incoming taking a removed map object's place must be base-map")
	}
	r.ReplaceInPlace(nil, incoming)
	r.ReplaceInPlace(outgoing, nil)
}

func TestHideShowLookup(t *testing.T) {
	_, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil),
		buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		map[int]ObjectDef{100: {SizeX: 1, SizeY: 1, ClipType: 2}},
	)
	player := NewPlayer("alice", objectTestLoc)
	base, ok := r.HideMapObjectFor(player, 100, objectTestLoc)
	if !ok || base == nil || base.ID() != 100 || !base.IsMapOriginal() {
		t.Errorf("HideMapObjectFor = (%v, %v), want the map original", base, ok)
	}
	if _, ok := r.HideMapObjectFor(player, 999, objectTestLoc); ok {
		t.Error("HideMapObjectFor of a missing loc must report false")
	}
	shown, ok := r.ShowMapObjectFor(player, 100, objectTestLoc)
	if !ok || shown != base {
		t.Errorf("ShowMapObjectFor = (%v, %v), want the same base object", shown, ok)
	}
	if _, ok := r.ShowMapObjectFor(player, 999, objectTestLoc); ok {
		t.Error("ShowMapObjectFor of a missing loc must report false")
	}
}

func TestRegionPopulation(t *testing.T) {
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil),
		buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
			{id: 200, placements: []placementSpec{{loc: packTestLoc(0, 1, 0), typ: 4, dir: 1}}},
		}),
		map[int]ObjectDef{
			100: {SizeX: 1, SizeY: 1, ClipType: 2},
			200: {SizeX: 1, SizeY: 1, ClipType: 2},
		})
	player := NewPlayer("alice", objectTestLoc)
	found := r.Find(player, 100, objectTestLoc)
	if found == nil {
		t.Fatal("Find must return the cache-map object")
	}
	if !found.IsMapOriginal() || !found.IsBaseMap() {
		t.Error("populated object must be map-original and base-map")
	}
	if found.Type() != 10 || found.Face() != 0 {
		t.Errorf("populated object type/face = %d/%d, want 10/0", found.Type(), found.Face())
	}
	// Map originals never enter the world indexes, and a load
	// records no deltas.
	if got := r.At(objectTestLoc); len(got) != 0 {
		t.Errorf("At = %v, want empty for a map original", objectIDs(got))
	}
	if got := len(r.WorldObjects()); got != 0 {
		t.Errorf("WorldObjects = %d, want 0", got)
	}
	if got := len(r.Deltas()); got != 0 {
		t.Errorf("Deltas = %d, want 0 (a load sends nothing)", got)
	}
	// A second lookup returns the SAME instance: no duplicates.
	if again := r.Find(player, 100, objectTestLoc); again != found {
		t.Error("second lookup must return the imported instance, not a duplicate")
	}
	// The second placement imports with its own type/face.
	other := r.Find(player, 200, Location{X: 3201, Y: 3200, Z: 0})
	if other == nil || other.Type() != 4 || other.Face() != 1 {
		t.Errorf("second placement = %+v, want type 4 face 1", other)
	}
	// The Placements accessor surfaces the decoded placements.
	placements, ok := w.RegionManager().Placements(regionTestID())
	if !ok || len(placements) != 2 {
		t.Fatalf("Placements = %v, %v, want 2 placements", placements, ok)
	}
	if w.RegionManager().RegionLoadSeq(regionTestID()) != 1 {
		t.Errorf("RegionLoadSeq = %d, want 1", w.RegionManager().RegionLoadSeq(regionTestID()))
	}
	if _, ok := w.RegionManager().Placements(9999); ok {
		t.Error("Placements of an unknown region must report false")
	}
}

func TestRegionPopulationReload(t *testing.T) {
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil),
		buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		map[int]ObjectDef{100: {SizeX: 1, SizeY: 1, ClipType: 2}},
	)
	player := NewPlayer("alice", objectTestLoc)
	first := r.Find(player, 100, objectTestLoc)
	if first == nil {
		t.Fatal("Find must return the map object")
	}
	// A runtime object on the same tile survives the reload.
	runtime := NewGameObject(101, objectTestLoc, 4, 0, nil)
	r.Register(runtime, true)
	if !w.RegionManager().ReloadRegion(regionTestID()) {
		t.Fatal("ReloadRegion must succeed")
	}
	second := r.Find(player, 100, objectTestLoc)
	if second == nil || second == first {
		t.Error("reload must re-import a FRESH map-original instance")
	}
	if got := len(r.ObjectsAtTile(objectTestLoc)); got != 2 {
		t.Errorf("tile entries after reload = %d, want 2 (fresh original + runtime)", got)
	}
	if got := r.At(objectTestLoc); len(got) != 1 || got[0] != runtime {
		t.Errorf("runtime object must survive reload in the world index: %v", objectIDs(got))
	}
	if w.RegionManager().RegionLoadSeq(regionTestID()) != 2 {
		t.Errorf("RegionLoadSeq after reload = %d, want 2", w.RegionManager().RegionLoadSeq(regionTestID()))
	}
}

func TestRegionPopulationRemovedSkip(t *testing.T) {
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil),
		buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		map[int]ObjectDef{100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true}},
	)
	player := NewPlayer("alice", objectTestLoc)
	if r.Find(player, 100, objectTestLoc) == nil {
		t.Fatal("Find must return the map object")
	}
	wantClip := int32(0x100 | 0x20000)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != wantClip {
		t.Fatalf("clipping after load = %#x, want %#x", got, wantClip)
	}
	// Deregister a FRESH object with the same fields (the spawn
	// removal shape): standsForMapObject sees the placed original.
	r.Deregister(NewGameObject(100, objectTestLoc, 10, 0, nil), true)
	if got := len(r.RemovedObjects()); got != 1 {
		t.Fatalf("RemovedObjects = %d, want 1", got)
	}
	if got := r.Find(player, 100, objectTestLoc); got != nil {
		t.Error("removed map object must be gone from the tile map")
	}
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after deregister = %#x, want 0", got)
	}
	// A reload must NOT resurrect it: the import skips the
	// placement and lifts the freshly laid clipping (replay).
	if !w.RegionManager().ReloadRegion(regionTestID()) {
		t.Fatal("ReloadRegion must succeed")
	}
	if got := r.Find(player, 100, objectTestLoc); got != nil {
		t.Error("reload must not resurrect a removed map object")
	}
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after reload = %#x, want 0 (replay lifts it)", got)
	}
}

func TestRegionPopulationBridgePlane(t *testing.T) {
	// The plane-1 tile's height value has bit 2 set, so a
	// placement at height 1 drops to plane 0 (the bridge-tile
	// adjustment, RegionManager.ts:993).
	terrain := buildTestTerrain(true, map[[3]int][]byte{{1, 0, 0}: heightTile(2)})
	_, r := newObjectRegistryWithRegion(t,
		terrain,
		buildTestObjectStream([]objectSpec{
			{id: 300, placements: []placementSpec{{loc: packTestLoc(1, 0, 0), typ: 10, dir: 0}}},
		}),
		map[int]ObjectDef{300: {SizeX: 1, SizeY: 1, ClipType: 2}},
	)
	player := NewPlayer("alice", objectTestLoc)
	ground := r.Find(player, 300, objectTestLoc)
	if ground == nil || ground.Location().Z != 0 {
		t.Fatalf("bridge placement must load at plane 0: %+v", ground)
	}
	above := Location{X: 3200, Y: 3200, Z: 1}
	if got := r.Find(player, 300, above); got != ground {
		t.Error("z-1 fallback must find the bridge-tile loc from above")
	}
}

func TestRegistryWithoutRegions(t *testing.T) {
	r := NewObjectRegistry()
	player := NewPlayer("alice", objectTestLoc)
	if got := r.Find(player, 100, objectTestLoc); got != nil {
		t.Error("no-region Find must be nil, not a panic")
	}
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("no-region Get must be nil")
	}
	if got := r.GetType(objectTestLoc, 10); got != nil {
		t.Error("no-region GetType must be nil")
	}
	// Registration still works (clipping is simply skipped).
	obj := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(obj, true)
	if got := r.Find(player, 100, objectTestLoc); got != obj {
		t.Error("registered object must be findable without regions")
	}
}

func TestSetRegionManagerDropsMapOriginals(t *testing.T) {
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil),
		buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		map[int]ObjectDef{100: {SizeX: 1, SizeY: 1, ClipType: 2}},
	)
	player := NewPlayer("alice", objectTestLoc)
	if r.Find(player, 100, objectTestLoc) == nil {
		t.Fatal("Find must return the map object")
	}
	runtime := NewGameObject(101, objectTestLoc, 4, 0, nil)
	r.Register(runtime, true)
	w.SetRegionManager(NewRegionManager(nil, nil, true))
	if got := r.Get(100, objectTestLoc); got != nil {
		t.Error("manager swap must drop imported map originals")
	}
	if got := r.Get(101, objectTestLoc); got != runtime {
		t.Error("manager swap must keep runtime objects")
	}
}

func TestWorldGameObjectsWiring(t *testing.T) {
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	if w.GameObjects() == nil {
		t.Fatal("GameObjects must be non-nil")
	}
	if w.GameObjects() != w.GameObjects() {
		t.Error("GameObjects must return the same registry")
	}
	// The registry follows SetRegionManager: population works
	// through the swapped-in manager.
	src := newFakeRegionSource([]int{regionTestID()})
	src.data[regionTestID()] = fakeRegionData{
		terrain: buildTestTerrain(true, nil),
		objects: buildTestObjectStream([]objectSpec{
			{id: 100, placements: []placementSpec{{loc: packTestLoc(0, 0, 0), typ: 10, dir: 0}}},
		}),
		ok: true,
	}
	w.SetRegionManager(NewRegionManager(src, &fakeObjectDefs{defs: map[int]ObjectDef{
		100: {SizeX: 1, SizeY: 1, ClipType: 2},
	}}, true))
	player := NewPlayer("alice", objectTestLoc)
	if got := w.GameObjects().Find(player, 100, objectTestLoc); got == nil {
		t.Error("registry must populate through the swapped-in manager")
	}
}

func TestWorldRemovalDiscardsOwnerObjects(t *testing.T) {
	w := NewWorld(Location{X: 3200, Y: 3200, Z: 0})
	alice := NewPlayer("alice", objectTestLoc)
	alice.Session = newFakeSession()
	w.QueueAddPlayer(alice)
	w.Process()
	if !alice.IsRegistered() {
		t.Fatal("player must be registered after the first tick")
	}
	owned := NewGameObject(100, objectTestLoc, 10, 0, nil)
	owned.SetOwner(alice)
	w.GameObjects().Register(owned, true)
	if got := w.GameObjects().Find(alice, 100, objectTestLoc); got != owned {
		t.Fatal("owner object must be findable before removal")
	}
	w.QueueRemovePlayer(alice.Index)
	w.Process()
	if alice.IsRegistered() {
		t.Fatal("player must be removed after the second tick")
	}
	if got := w.GameObjects().Find(alice, 100, objectTestLoc); got != nil {
		t.Error("player removal must discard the owner map (WeakMap deviation wiring)")
	}
	if got := w.GameObjects().OwnerObjectsInScene(alice, 3200, 3200, 104); len(got) != 0 {
		t.Errorf("owner scene after removal = %d, want 0", len(got))
	}
}

func TestRegisterReplacementLiftsPredecessorClipping(t *testing.T) {
	// Replacing a solid loc with a non-solid form must clear the
	// old clipping (ObjectManager.ts:253-255: playerUpdate
	// removes the predecessor through MapObjects.remove).
	w, r := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil), nil,
		map[int]ObjectDef{
			100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
			101: {SizeX: 1, SizeY: 1, ClipType: 0},
		})
	solid := NewGameObject(100, objectTestLoc, 10, 0, nil)
	r.Register(solid, true)
	want := int32(0x100 | 0x20000)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != want {
		t.Fatalf("clipping after solid register = %#x, want %#x", got, want)
	}
	empty := NewGameObject(101, objectTestLoc, 10, 0, nil)
	r.Register(empty, true)
	if got := w.RegionManager().GetClipping(3200, 3200, 0); got != 0 {
		t.Errorf("clipping after replacement = %#x, want 0 (predecessor clip lifted, replacement lays none)", got)
	}
	// A quiet replacement keeps the predecessor's clipping: the
	// tile-map removal (and its clip lift) runs only when
	// playerUpdate is true.
	w2, r2 := newObjectRegistryWithRegion(t,
		buildTestTerrain(true, nil), nil,
		map[int]ObjectDef{
			100: {SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true},
			101: {SizeX: 1, SizeY: 1, ClipType: 0},
		})
	r2.Register(NewGameObject(100, objectTestLoc, 10, 0, nil), true)
	r2.Register(NewGameObject(101, objectTestLoc, 10, 0, nil), false)
	if got := w2.RegionManager().GetClipping(3200, 3200, 0); got != want {
		t.Errorf("clipping after quiet replacement = %#x, want %#x (kept)", got, want)
	}
}
