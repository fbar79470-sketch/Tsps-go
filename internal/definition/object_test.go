package definition

import (
	"testing"
)

type testVarReader struct {
	varps   map[int]int
	varbits map[int]int
}

func (r testVarReader) GetVarp(id int) int   { return r.varps[id] }
func (r testVarReader) GetVarbit(id int) int { return r.varbits[id] }

func objectCacheSpec() fakeCacheSpec {
	return fakeCacheSpec{
		locs: map[int][]byte{
			0: {0},
			1: locEntry("Bank booth", locDesc("Where you bank."), locSize(2, 1), locAction(0, "Bank")),
			2: locEntry("Tree", locClipTypeDecoration(), locInteractive(1), locMapFunction(5), locClipMask(3), locAction(0, "Chop")),
			3: locEntry("Door", locClipTypeNone()),
			4: locEntry("MultiLoc", locTransforms(1, -1, 5, 6)),
			5: locEntry("Form A"),
			6: locEntry("Form B"),
			7: locEntry("Bad transform", locTransforms(2, -1, -1, 6)),
			8: locEntry("Varp morph", locTransforms(-1, 3, 5, 6)),
		},
	}
}

func loadObjects(t *testing.T, files map[string]string, spec fakeCacheSpec) *Store {
	t.Helper()
	defs, gamevals := newFakeCache(t, spec)
	dir := fixtureDir(t, files)
	store, err := Load(dir, LoadOptions{Cache: defs, Gamevals: gamevals})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	return store
}

func TestObjectHydrateFromCache(t *testing.T) {
	store := loadObjects(t, minimalFiles(), objectCacheSpec())

	booth, ok := store.Object(1)
	if !ok || booth.ID() != 1 || booth.Name() != "Bank booth" || booth.Description() != "Where you bank." {
		t.Fatalf("booth = %+v ok=%v", booth, ok)
	}
	if booth.SizeX() != 2 || booth.SizeY() != 1 || booth.Size() != 2 {
		t.Fatalf("booth size = %d/%d/%d", booth.SizeX(), booth.SizeY(), booth.Size())
	}
	if !booth.IsSolid() || booth.IsClippedDecoration() || booth.IsMultiLoc() {
		t.Fatal("booth flags wrong")
	}
	actions := booth.Interactions()
	if len(actions) != 5 || actions[0] != "Bank" || actions[1] != "" || actions[4] != "" {
		t.Fatalf("booth interactions = %v", actions)
	}
	if booth.MinimapFunction() != -1 {
		t.Fatalf("booth map function = %d", booth.MinimapFunction())
	}

	tree, ok := store.Object(2)
	if !ok || !tree.IsClippedDecoration() || !tree.IsSolid() || !tree.HasActions() {
		t.Fatalf("tree = %+v ok=%v", tree, ok)
	}
	if tree.MinimapFunction() != 5 || tree.BlockingMask() != 3 {
		t.Fatalf("tree mapfn=%d mask=%d", tree.MinimapFunction(), tree.BlockingMask())
	}

	// No actions at all: Interactions is nil, not an empty slice.
	door, ok := store.Object(3)
	if !ok || door.Interactions() != nil {
		t.Fatalf("door interactions = %v", door.Interactions())
	}
	if door.IsSolid() {
		t.Fatal("clipType 0 must not be solid")
	}
}

func TestObjectOutOfRangeAndNilCache(t *testing.T) {
	store := loadObjects(t, minimalFiles(), objectCacheSpec())
	if _, ok := store.Object(999); ok {
		t.Fatal("out-of-range object must return ok=false")
	}
	if _, ok := store.Object(-1); ok {
		t.Fatal("negative object id must return ok=false")
	}
	dir := fixtureDir(t, minimalFiles())
	nilStore, err := Load(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := nilStore.Object(1); ok {
		t.Fatal("nil-cache object must return ok=false")
	}
}

func TestObjectExamine(t *testing.T) {
	store := loadObjects(t, withFiles(map[string]string{
		"object-examines.json":          `{"1": "Wiki booth.", "2": "Wiki tree."}`,
		"object-examine-overrides.json": `{"1": "Override booth."}`,
	}), objectCacheSpec())
	// Overrides win over the wiki file.
	booth, _ := store.Object(1)
	if got := booth.Examine(); got != "Override booth." {
		t.Fatalf("booth examine = %q", got)
	}
	// Wiki file beats the cache description.
	tree, _ := store.Object(2)
	if got := tree.Examine(); got != "Wiki tree." {
		t.Fatalf("tree examine = %q", got)
	}
	// No map entry: the cache description.
	// (Booth's desc is masked by the override above; use a fresh store.)
	store2 := loadObjects(t, minimalFiles(), objectCacheSpec())
	booth2, _ := store2.Object(1)
	if got := booth2.Examine(); got != "Where you bank." {
		t.Fatalf("booth examine = %q", got)
	}
	// No desc anywhere: generic line.
	door, _ := store2.Object(3)
	if got := door.Examine(); got != "It's a Door." {
		t.Fatalf("door examine = %q", got)
	}
}

func TestObjectBarrowsSize(t *testing.T) {
	locs := map[int][]byte{0: {0}}
	for i := 1; i <= 20672; i++ {
		locs[i] = []byte{0}
	}
	locs[20667] = locEntry("Barrows A", locSize(1, 1))
	locs[20668] = locEntry("Barrows B", locSize(1, 1))
	locs[20669] = locEntry("Barrows C", locSize(1, 1))
	locs[20670] = locEntry("Barrows D", locSize(1, 1))
	locs[20671] = locEntry("Barrows E", locSize(1, 1))
	locs[20672] = locEntry("Barrows F", locSize(1, 1))
	store := loadObjects(t, minimalFiles(), fakeCacheSpec{locs: locs})
	for _, id := range []int{20667, 20668, 20669, 20670, 20672} {
		def, ok := store.Object(id)
		if !ok || def.Size() != 2 {
			t.Fatalf("object %d size = %d ok=%v", id, def.Size(), ok)
		}
	}
	def, _ := store.Object(20671)
	if def.Size() != 3 {
		t.Fatalf("object 20671 size = %d", def.Size())
	}
}

func TestObjectForPlayer(t *testing.T) {
	store := loadObjects(t, minimalFiles(), objectCacheSpec())

	// Varbit-driven morph.
	morph, ok := store.ObjectForPlayer(4, testVarReader{varbits: map[int]int{1: 0}})
	if !ok || morph.ID() != 5 {
		t.Fatalf("varbit 0 = %+v ok=%v", morph, ok)
	}
	morph, ok = store.ObjectForPlayer(4, testVarReader{varbits: map[int]int{1: 1}})
	if !ok || morph.ID() != 6 {
		t.Fatalf("varbit 1 = %+v ok=%v", morph, ok)
	}
	// Index 2 names -1: no object (unlike LocType.Transform, which
	// falls through to the default).
	if _, ok := store.ObjectForPlayer(4, testVarReader{varbits: map[int]int{1: 2}}); ok {
		t.Fatal("varbit 2 must yield no object")
	}
	// Out of range: the default (last) transform, which is -1.
	if _, ok := store.ObjectForPlayer(4, testVarReader{varbits: map[int]int{1: 99}}); ok {
		t.Fatal("varbit 99 must yield no object")
	}
	// Varp-driven morph.
	morph, ok = store.ObjectForPlayer(8, testVarReader{varps: map[int]int{3: 1}})
	if !ok || morph.ID() != 6 {
		t.Fatalf("varp 1 = %+v ok=%v", morph, ok)
	}
	// In-range -1: no object.
	if _, ok := store.ObjectForPlayer(7, testVarReader{varbits: map[int]int{2: 0}}); ok {
		t.Fatal("in-range -1 transform must yield no object")
	}
	// Non-multiLoc returns itself.
	self, ok := store.ObjectForPlayer(1, testVarReader{})
	if !ok || self.ID() != 1 {
		t.Fatalf("self = %+v ok=%v", self, ok)
	}
	// Nil reader: no vars set, index -1, default transform.
	if _, ok := store.ObjectForPlayer(4, nil); ok {
		t.Fatal("nil reader must yield no object for this morph")
	}
	// Unknown id stays unknown.
	if _, ok := store.ObjectForPlayer(999, testVarReader{}); ok {
		t.Fatal("unknown id must stay unknown")
	}
}
