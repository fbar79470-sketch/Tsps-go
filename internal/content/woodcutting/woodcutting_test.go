package woodcutting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- fixtures ----

// loadFixtureTables loads the real tables over Worker A's
// fixture resolver (tables_test.go): every gameval name the
// source tables use maps to a deterministic fake id, with a
// separate range per kind. No live cache is involved.
func loadFixtureTables(t *testing.T) *Tables {
	t.Helper()
	tables, err := Load(newFixtureResolver())
	if err != nil {
		t.Fatalf("Load(fixture) = %v", err)
	}
	return tables
}

// newTestWoodcutting builds an instance over fixture tables
// with a capturing emit func and deterministic randomness
// (randFloat/randInt overridable per test through the returned
// instance's fields).
func newTestWoodcutting(t *testing.T, tables *Tables) (*woodcutting, *[]emittedEvent) {
	t.Helper()
	var emitted []emittedEvent
	w := newWoodcutting(tables)
	w.emit = func(name string, payload any) {
		emitted = append(emitted, emittedEvent{name: name, payload: payload})
	}
	return w, &emitted
}

type emittedEvent struct {
	name    string
	payload any
}

func newTestPlayer(name string, x, y int) *game.Player {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: 0})
	p.SetRegistered(true)
	return p
}

func setLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func axeByItem(tables *Tables, item string) *Axe {
	for i := range tables.Axes {
		if tables.Axes[i].Item == item {
			return &tables.Axes[i]
		}
	}
	return nil
}

func treeByName(tables *Tables, name string) *Tree {
	for _, tree := range tables.Trees {
		if tree.Name == name {
			return tree
		}
	}
	return nil
}

// fakeObjectDef is a plugins.ObjectDefinition naming one object.
type fakeObjectDef struct {
	name         string
	interactions []string
}

func (d fakeObjectDef) ID() int                { return 0 }
func (d fakeObjectDef) Name() string           { return d.name }
func (d fakeObjectDef) SizeX() int             { return 1 }
func (d fakeObjectDef) SizeY() int             { return 1 }
func (d fakeObjectDef) BlockingMask() int      { return 0 }
func (d fakeObjectDef) Interactions() []string { return d.interactions }

// fakeObjects is a minimal in-memory plugins.ObjectManagerAPI
// for session tests: objects live on tiles in registration
// order. mapOriginals models the tile map's map-original
// entries (what the production registry's Get reads): a tree
// that stands only as a map original is found by Get but not
// by ObjectsAt, exactly as in production.
type fakeObjects struct {
	byTile       map[game.Location][]plugins.GameObject
	mapOriginals map[game.Location][]plugins.GameObject
	// removed is the removed-objects list (World.
	// getRemovedObjects): entries tests record with
	// addRemoved, read back by RemovedObjects.
	removed []plugins.GameObject
}

func newFakeObjects() *fakeObjects {
	return &fakeObjects{
		byTile:       make(map[game.Location][]plugins.GameObject),
		mapOriginals: make(map[game.Location][]plugins.GameObject),
	}
}

// addRemoved records obj as a removed object, modelling the
// session layer's region-change removal list.
func (f *fakeObjects) addRemoved(obj plugins.GameObject) {
	if obj == nil {
		return
	}
	f.removed = append(f.removed, obj)
}

func (f *fakeObjects) RemovedObjects() []plugins.GameObject { return f.removed }

// placeMapOriginal stands obj as a map original only: Get finds
// it, ObjectsAt does not.
func (f *fakeObjects) placeMapOriginal(obj plugins.GameObject) {
	if obj == nil {
		return
	}
	loc := obj.Location()
	f.mapOriginals[loc] = append(f.mapOriginals[loc], obj)
}

func (f *fakeObjects) removeFrom(list map[game.Location][]plugins.GameObject, obj plugins.GameObject) {
	loc := obj.Location()
	kept := list[loc][:0]
	for _, o := range list[loc] {
		if o != obj {
			kept = append(kept, o)
		}
	}
	list[loc] = kept
}

func (f *fakeObjects) Register(obj plugins.GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	loc := obj.Location()
	f.byTile[loc] = append(f.byTile[loc], obj)
}

func (f *fakeObjects) Deregister(obj plugins.GameObject, playerUpdate bool) {
	if obj == nil {
		return
	}
	f.removeFrom(f.byTile, obj)
	f.removeFrom(f.mapOriginals, obj)
}

func (f *fakeObjects) ReplaceInPlace(outgoing, incoming plugins.GameObject) {
	if outgoing == nil || incoming == nil {
		return
	}
	loc := outgoing.Location()
	// Production ReplaceInPlace removes the outgoing from the
	// tile map (mapRemove) as well as the runtime indexes: a
	// felled map-original tree stops answering Get.
	f.removeFrom(f.mapOriginals, outgoing)
	for i, o := range f.byTile[loc] {
		if o == outgoing {
			f.byTile[loc][i] = incoming
			return
		}
	}
	f.byTile[loc] = append(f.byTile[loc], incoming)
}

func (f *fakeObjects) ObjectsAt(loc game.Location) []plugins.GameObject {
	return f.byTile[loc]
}

func (f *fakeObjects) ObjectsInRegion(regionX, regionY int) []plugins.GameObject { return nil }

func (f *fakeObjects) Exists(id int, loc game.Location) bool {
	return len(f.byTile[loc]) > 0
}

// Get answers the map-original lookup: map originals first,
// then runtime registrations, by id — the production tile-map
// order for a single-id match.
func (f *fakeObjects) Get(id int, loc game.Location) plugins.GameObject {
	for _, o := range f.mapOriginals[loc] {
		if o.ID() == id {
			return o
		}
	}
	for _, o := range f.byTile[loc] {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

func (f *fakeObjects) Find(player *game.Player, id int, loc game.Location) plugins.GameObject {
	for _, o := range f.byTile[loc] {
		if o.ID() == id {
			return o
		}
	}
	return nil
}

func (f *fakeObjects) StillStands(player *game.Player, id int, loc game.Location, obj plugins.GameObject) bool {
	return f.Find(player, id, loc) == obj
}

func (f *fakeObjects) RemovePluginObjects() int { return 0 }

// placeTree registers a tree object in the fake world at
// (3201, 3200, 0), next to the test player's spawn.
func placeTree(tables *Tables, objects *fakeObjects, tree *Tree) *game.GameObject {
	obj := game.NewGameObject(tree.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(obj, true)
	return obj
}

// ---- cut chance ----

// TestCalculateCutChance pins the success formula
// (Woodcutting.plugin.js:758-767): successes = 1 +
// floor(low*(99-L)/98) + floor(high*(L-1)/98), chance =
// successes/256 clamped [0,1]. Values are hand-computed from
// the table rows, not copied from the implementation.
func TestCalculateCutChance(t *testing.T) {
	tables := loadFixtureTables(t)
	normal := treeByName(tables, "normal tree")
	oak := treeByName(tables, "oak")
	scrapey := treeByName(tables, "scrapey tree")
	blisterwood := treeByName(tables, "blisterwood")
	bronze := axeByItem(tables, "bronze_axe")
	dragon := axeByItem(tables, "dragon_axe")
	if normal == nil || oak == nil || scrapey == nil || blisterwood == nil || bronze == nil || dragon == nil {
		t.Fatal("fixture tables missing a tree or axe")
	}
	cases := []struct {
		name       string
		level      int
		tree       *Tree
		axe        *Axe
		wantChance float64
	}{
		// Normal/bronze: L1 → 1+64+0 = 65; L99 → 1+0+200 = 201.
		{"normal bronze L1", 1, normal, bronze, 65.0 / 256},
		{"normal bronze L99", 99, normal, bronze, 201.0 / 256},
		// Normal/dragon: L1 → 1+240+0 = 241; L99 → 1+0+750 = 751 → clamped to 1.
		{"normal dragon L1", 1, normal, dragon, 241.0 / 256},
		{"normal dragon L99", 99, normal, dragon, 1},
		// Oak/bronze L15 → 1+27+14 = 42.
		{"oak bronze L15", 15, oak, bronze, 42.0 / 256},
		// Scrapey rounded variant: L1 → 23, L50 → 49, L99 → 75.
		{"scrapey bronze L1", 1, scrapey, bronze, 23.0 / 256},
		{"scrapey bronze L50", 50, scrapey, bronze, 49.0 / 256},
		{"scrapey bronze L99", 99, scrapey, bronze, 75.0 / 256},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := calculateCutChance(tc.level, tc.tree, tc.axe); got != tc.wantChance {
				t.Errorf("calculateCutChance = %v, want %v", got, tc.wantChance)
			}
		})
	}
}

// TestCutChanceRowNullScaling pins the blisterwood null-row
// rule (cutChanceRange, plugin.js:748-756): a missing tier
// scales off the normal chart — iron: low round(15*96/64) =
// 23, high round(50*300/200) = 75 — and the chance follows:
// L50 → 1+11+37 = 49.
func TestCutChanceRowNullScaling(t *testing.T) {
	tables := loadFixtureTables(t)
	blisterwood := treeByName(tables, "blisterwood")
	iron := axeByItem(tables, "iron_axe")
	if blisterwood == nil || iron == nil {
		t.Fatal("fixture tables missing blisterwood or iron axe")
	}
	row := CutChanceRow(blisterwood.CutChance, iron.Tier)
	if !row.Has || row.Low != 23 || row.High != 75 {
		t.Fatalf("CutChanceRow(blisterwood, iron) = %+v, want {Low:23 High:75 Has:true}", row)
	}
	if got := calculateCutChance(50, blisterwood, iron); got != 49.0/256 {
		t.Errorf("calculateCutChance(50, blisterwood, iron) = %v, want %v", got, 49.0/256)
	}
	// The present rows pass through untouched.
	row = CutChanceRow(blisterwood.CutChance, 0)
	if !row.Has || row.Low != 15 || row.High != 50 {
		t.Errorf("CutChanceRow(blisterwood, bronze) = %+v, want {Low:15 High:50 Has:true}", row)
	}
	row = CutChanceRow(blisterwood.CutChance, 7)
	if !row.Has || row.Low != 61 || row.High != 186 {
		t.Errorf("CutChanceRow(blisterwood, dragon) = %+v, want {Low:61 High:186 Has:true}", row)
	}
}

// TestCalculateCutChanceNilGuards pins the nil guards: no tree
// or axe is a zero chance, never a panic.
func TestCalculateCutChanceNilGuards(t *testing.T) {
	tables := loadFixtureTables(t)
	normal := treeByName(tables, "normal tree")
	bronze := axeByItem(tables, "bronze_axe")
	if calculateCutChance(50, nil, bronze) != 0 {
		t.Error("nil tree: want 0")
	}
	if calculateCutChance(50, normal, nil) != 0 {
		t.Error("nil axe: want 0")
	}
}

// TestRollWeightedIndex pins the weighted-roll walk
// (rollWeighted, plugin.js:996-1006): a 1-based roll lands on
// the first entry that takes it to 0 or below; an over-total
// roll lands on the last entry.
func TestRollWeightedIndex(t *testing.T) {
	weights := []int{65, 32, 1, 1, 1}
	cases := []struct{ roll, want int }{
		{1, 0}, {65, 0}, {66, 1}, {97, 1}, {98, 2}, {99, 3}, {100, 4}, {101, 4},
	}
	for _, tc := range cases {
		if got := rollWeightedIndex(weights, tc.roll); got != tc.want {
			t.Errorf("rollWeightedIndex(%d) = %d, want %d", tc.roll, got, tc.want)
		}
	}
}

// ---- axes ----

// TestFindBestUsableAxe pins best-first selection
// (plugin.js:710-733): the best held usable axe wins, and
// level gates apply.
func TestFindBestUsableAxe(t *testing.T) {
	tables := loadFixtureTables(t)
	bronze := axeByItem(tables, "bronze_axe")
	rune := axeByItem(tables, "rune_axe")
	if bronze == nil || rune == nil {
		t.Fatal("fixture tables missing axes")
	}
	w, _ := newTestWoodcutting(t, tables)
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 41)
	p.Inventory().Adds(bronze.ID, 1)
	p.Inventory().Adds(rune.ID, 1)
	if got := w.findBestUsableAxe(p); got == nil || got.ID != rune.ID {
		t.Errorf("best axe = %v, want rune %d", got, rune.ID)
	}
	// No axe held → the woodcutting:tool fallback answers nil.
	q := newTestPlayer("bob", 3200, 3200)
	setLevel(q, game.SkillWoodcutting, 99)
	if got := w.findBestUsableAxe(q); got != nil {
		t.Errorf("no axe held: got %v, want nil", got)
	}
	// Level gate: a level-1 player cannot use the rune axe.
	setLevel(p, game.SkillWoodcutting, 1)
	if got := w.findBestUsableAxe(p); got == nil || got.ID != bronze.ID {
		t.Errorf("level-1 best axe = %v, want bronze %d", got, bronze.ID)
	}
}

// TestFindBestUsableAxeFellingPromotion pins the felling rule
// (plugin.js:724-728): same tier + same level + rations
// available promotes the felling axe; no rations keeps the
// standard axe.
func TestFindBestUsableAxeFellingPromotion(t *testing.T) {
	tables := loadFixtureTables(t)
	rune := axeByItem(tables, "rune_axe")
	runeFelling := axeByItem(tables, "rune_axe_2h")
	if rune == nil || runeFelling == nil {
		t.Fatal("fixture tables missing rune axes")
	}
	w, _ := newTestWoodcutting(t, tables)
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 41)
	p.Inventory().Adds(rune.ID, 1)
	p.Inventory().Adds(runeFelling.ID, 1)
	if got := w.findBestUsableAxe(p); got == nil || got.ID != rune.ID {
		t.Fatalf("no rations: best axe = %v, want rune %d", got, rune.ID)
	}
	p.Inventory().Adds(forestersRationID, 1)
	if got := w.findBestUsableAxe(p); got == nil || got.ID != runeFelling.ID {
		t.Errorf("with rations: best axe = %v, want rune felling %d", got, runeFelling.ID)
	}
}

// TestFindBestUsableAxeMembersFilter pins the members-axe
// filter (choppingAxeEligible, plugin.js:1221-1227): on a free
// world a members axe is skipped. The filter is reachable only
// through the integration hooks (no Api accessor exists), so
// the test drives them directly.
func TestFindBestUsableAxeMembersFilter(t *testing.T) {
	tables := loadFixtureTables(t)
	bronze := axeByItem(tables, "bronze_axe")
	rune := axeByItem(tables, "rune_axe")
	if bronze == nil || rune == nil {
		t.Fatal("fixture tables missing axes")
	}
	w, _ := newTestWoodcutting(t, tables)
	w.membersWorld = false
	w.isMembersItem = func(id int) bool { return id == rune.ID }
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 41)
	p.Inventory().Adds(bronze.ID, 1)
	p.Inventory().Adds(rune.ID, 1)
	if got := w.findBestUsableAxe(p); got == nil || got.ID != bronze.ID {
		t.Errorf("free world best axe = %v, want bronze %d", got, bronze.ID)
	}
	// On a members world the same axe is eligible again.
	w.membersWorld = true
	if got := w.findBestUsableAxe(p); got == nil || got.ID != rune.ID {
		t.Errorf("members world best axe = %v, want rune %d", got, rune.ID)
	}
}

// TestCanUseAxe pins the Agility/Firemaking gates
// (plugin.js:714-721): a crystal axe needs 50 Agility, an
// infernal axe 85 Firemaking.
func TestCanUseAxe(t *testing.T) {
	tables := loadFixtureTables(t)
	crystal := axeByItem(tables, "crystal_axe")
	infernal := axeByItem(tables, "infernal_axe")
	if crystal == nil || infernal == nil {
		t.Fatal("fixture tables missing crystal/infernal axes")
	}
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	setLevel(p, game.SkillAgility, 1)
	setLevel(p, game.SkillFiremaking, 1)
	if canUseAxe(p, crystal) {
		t.Error("crystal axe with Agility 1: want unusable")
	}
	if canUseAxe(p, infernal) {
		t.Error("infernal axe with Firemaking 1: want unusable")
	}
	setLevel(p, game.SkillAgility, 50)
	setLevel(p, game.SkillFiremaking, 85)
	if !canUseAxe(p, crystal) || !canUseAxe(p, infernal) {
		t.Error("requirements met: want both usable")
	}
}

// ---- depletion / stumps ----

// TestShouldDepleteTree pins the depletion rules
// (plugin.js:1005-1022) with deterministic randomness.
func TestShouldDepleteTree(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	normal := treeByName(tables, "normal tree")
	dramen := treeByName(tables, "dramen")
	oak := treeByName(tables, "oak")
	juniper := treeByName(tables, "mature juniper")
	if normal == nil || dramen == nil || oak == nil || juniper == nil {
		t.Fatal("fixture tables missing trees")
	}
	loc := game.Location{X: 3201, Y: 3200, Z: 0}
	state := func(tree *Tree) *session {
		return &session{tree: tree, objectID: 1, location: loc}
	}
	// Non-multi always falls.
	if !w.shouldDepleteTree(state(normal)) {
		t.Error("normal tree (non-multi): want deplete")
	}
	// depletes:false never falls.
	if w.shouldDepleteTree(state(dramen)) {
		t.Error("dramen (depletes false): want no deplete")
	}
	// Despawn timer: at 0 it falls, above 0 it stands.
	w.despawnTimers[treeTimerKey(1, loc)] = &despawnTimer{remaining: 0, max: 45, lastChopTick: -1}
	if !w.shouldDepleteTree(state(oak)) {
		t.Error("oak with timer 0: want deplete")
	}
	w.despawnTimers[treeTimerKey(1, loc)] = &despawnTimer{remaining: 10, max: 45, lastChopTick: -1}
	if w.shouldDepleteTree(state(oak)) {
		t.Error("oak with timer 10: want no deplete")
	}
	// 1-in-N roll: a 1 depletes, anything else does not.
	w.randInt = func(min, max int) int { return 1 }
	if !w.shouldDepleteTree(state(juniper)) {
		t.Error("juniper with roll 1: want deplete")
	}
	w.randInt = func(min, max int) int { return 2 }
	if w.shouldDepleteTree(state(juniper)) {
		t.Error("juniper with roll 2: want no deplete")
	}
	// Fallback roll 0-15 < 2: 0 and 1 deplete, 2 does not.
	scrapey := treeByName(tables, "scrapey tree")
	if scrapey == nil {
		t.Fatal("fixture tables missing scrapey tree")
	}
	w.randInt = func(min, max int) int { return 1 }
	if !w.shouldDepleteTree(state(scrapey)) {
		t.Error("scrapey with fallback roll 1: want deplete")
	}
	w.randInt = func(min, max int) int { return 2 }
	if w.shouldDepleteTree(state(scrapey)) {
		t.Error("scrapey with fallback roll 2: want no deplete")
	}
}

// TestResolveStumpID pins the stump resolution order that
// survives without a CacheDefinitions seam: the tree's own
// stump id, else the default stump. The model-map and
// treeId+1 legs are NOT STARTED (see resolveStumpID).
func TestResolveStumpID(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	normal := treeByName(tables, "normal tree")
	hollow := treeByName(tables, "hollow")
	if normal == nil || hollow == nil {
		t.Fatal("fixture tables missing trees")
	}
	if normal.StumpID == 0 {
		t.Fatal("fixture normal tree has no stump id")
	}
	if got := w.resolveStumpID(normal.ObjectIDs[0], normal); got != normal.StumpID {
		t.Errorf("resolveStumpID(normal) = %d, want %d", got, normal.StumpID)
	}
	if hollow.StumpID != 0 {
		t.Fatal("fixture hollow tree unexpectedly has a stump id")
	}
	if got := w.resolveStumpID(0, hollow); got != tables.DefaultTreeStumpID {
		t.Errorf("resolveStumpID(hollow) = %d, want default %d", got, tables.DefaultTreeStumpID)
	}
}

// TestStartWoodcuttingFacesTree pins the face-tree step
// (plugin.js:1279-1285): reset() clears the face the object
// click set, so start faces the tree again, forced.
func TestStartWoodcuttingFacesTree(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	p := newTestPlayer("facer", 3200, 3200)
	obj := startChopping(t, w, tables, objects, p)
	face, ok := p.PositionToFace()
	if !ok {
		t.Fatal("start: want a face target set")
	}
	if !face.Equals(obj.Location()) {
		t.Errorf("face target = %v, want tree tile %v", face, obj.Location())
	}
}

// TestWithinInteractionDistance pins the footprint rule
// (GameObject.isWithinInteractionDistance, GameObject.ts:151-155):
// the gap to the loc's rectangle, axes swapped on an odd face,
// is at most 2. With a 1x1 footprint this is the Chebyshev
// distance.
func TestWithinInteractionDistance(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	normal := treeByName(tables, "normal tree")
	if normal == nil || len(normal.ObjectIDs) == 0 {
		t.Fatal("fixture tables missing normal tree")
	}
	id := normal.ObjectIDs[0]
	loc := game.Location{X: 3200, Y: 3200, Z: 0}
	obj := game.NewGameObject(id, loc, 10, 0, nil)
	w.tables.ObjectSize = map[int]ObjectSize{id: {SizeX: 1, SizeY: 1}}
	cases := []struct {
		name       string
		size       ObjectSize
		face       int
		player     game.Location
		wantWithin bool
	}{
		{"1x1 two away", ObjectSize{1, 1}, 0, game.Location{X: 3202, Y: 3200, Z: 0}, true},
		{"1x1 three away", ObjectSize{1, 1}, 0, game.Location{X: 3203, Y: 3200, Z: 0}, false},
		{"1x1 diagonal two", ObjectSize{1, 1}, 0, game.Location{X: 3202, Y: 3202, Z: 0}, true},
		{"1x1 diagonal three", ObjectSize{1, 1}, 0, game.Location{X: 3203, Y: 3203, Z: 0}, false},
		{"2x2 far edge", ObjectSize{2, 2}, 0, game.Location{X: 3203, Y: 3200, Z: 0}, true},
		{"2x2 beyond far edge", ObjectSize{2, 2}, 0, game.Location{X: 3204, Y: 3200, Z: 0}, false},
		{"2x1 face 0, three east", ObjectSize{2, 1}, 0, game.Location{X: 3203, Y: 3200, Z: 0}, true},
		{"2x1 face 1, three east", ObjectSize{2, 1}, 1, game.Location{X: 3203, Y: 3200, Z: 0}, false},
		{"2x1 face 1, two east two north", ObjectSize{2, 1}, 1, game.Location{X: 3202, Y: 3202, Z: 0}, true},
	}
	for _, tc := range cases {
		w.tables.ObjectSize[id] = tc.size
		if got := w.withinInteractionDistance(obj, tc.player, id, tc.face); got != tc.wantWithin {
			t.Errorf("%s: within = %v, want %v", tc.name, got, tc.wantWithin)
		}
	}
	// An unknown object id falls back to a 1x1 footprint.
	if got := w.withinInteractionDistance(obj, game.Location{X: 3202, Y: 3200, Z: 0}, id+999999, 0); !got {
		t.Error("unknown id: want the 1x1 fallback to allow two away")
	}
	if got := w.withinInteractionDistance(obj, game.Location{X: 3203, Y: 3200, Z: 0}, id+999999, 0); got {
		t.Error("unknown id: want the 1x1 fallback to refuse three away")
	}
}

// TestFakeObjectsRemovedObjects pins the fake's removed-objects
// list: addRemoved records entries RemovedObjects reads back.
func TestFakeObjectsRemovedObjects(t *testing.T) {
	objects := newFakeObjects()
	if got := objects.RemovedObjects(); len(got) != 0 {
		t.Fatalf("fresh fake: RemovedObjects = %d entries, want 0", len(got))
	}
	obj := game.NewGameObject(1276, game.Location{X: 3200, Y: 3200, Z: 0}, 10, 0, nil)
	objects.addRemoved(obj)
	got := objects.RemovedObjects()
	if len(got) != 1 || got[0] != obj {
		t.Errorf("RemovedObjects = %v, want [%v]", got, obj)
	}
}

// ---- nest tables ----

// TestNestTables pins the nest table totals (plugin.js:150-179):
// 1,011 seed slots, 100 ring slots, and the searchable/seed
// id sets.
func TestNestTables(t *testing.T) {
	tables := loadFixtureTables(t)
	seedTotal := 0
	for _, seed := range tables.NestSeeds {
		seedTotal += seed.Weight
	}
	if len(tables.NestSeeds) != 19 || seedTotal != 1011 {
		t.Errorf("nest seeds: %d entries totalling %d, want 19 totalling 1011", len(tables.NestSeeds), seedTotal)
	}
	ringTotal := 0
	for _, ring := range tables.NestRings {
		ringTotal += ring.Weight
	}
	if len(tables.NestRings) != 5 || ringTotal != 100 {
		t.Errorf("nest rings: %d entries totalling %d, want 5 totalling 100", len(tables.NestRings), ringTotal)
	}
	if len(tables.SearchableNestIDs) != 6 {
		t.Errorf("searchable nests: %d ids, want 6", len(tables.SearchableNestIDs))
	}
	if len(tables.SeedNestIDs) != 2 {
		t.Errorf("seed nests: %d ids, want 2", len(tables.SeedNestIDs))
	}
}

// TestRollBirdNestID pins the 100-slot nest roll
// (plugin.js:1013-1023) with deterministic rolls: seed 1-65,
// ring 66-97, red 98, green 99, blue 100 — and 60 seed slots
// with a strung rabbit foot.
func TestRollBirdNestID(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	p := newTestPlayer("alice", 3200, 3200)
	roll := 0
	w.randInt = func(min, max int) int { return roll }
	cases := []struct {
		roll int
		want int
	}{
		{1, tables.BirdNests.SeedNest}, {65, tables.BirdNests.SeedNest},
		{66, tables.BirdNests.RingNest}, {97, tables.BirdNests.RingNest},
		{98, tables.BirdNests.RedEggNest}, {99, tables.BirdNests.GreenEggNest},
		{100, tables.BirdNests.BlueEggNest},
	}
	for _, tc := range cases {
		roll = tc.roll
		if got := w.rollBirdNestID(p); got != tc.want {
			t.Errorf("roll %d: nest = %d, want %d", tc.roll, got, tc.want)
		}
	}
	// Strung rabbit foot: 60 seed slots, so roll 61 is a ring.
	p.Equipment().Set(game.AmuletSlot, game.NewItem(strungRabbitFootID, 1))
	roll = 61
	if got := w.rollBirdNestID(p); got != tables.BirdNests.RingNest {
		t.Errorf("rabbit foot roll 61: nest = %d, want ring %d", got, tables.BirdNests.RingNest)
	}
}

// ---- XP ----

// TestLumberjackMultiplier pins the outfit bonus
// (plugin.js:820-824): hat 0.4%, top 0.8%, legs 0.6%, boots
// 0.2%, plus 0.5% for all four.
func TestLumberjackMultiplier(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	p := newTestPlayer("alice", 3200, 3200)
	if got := w.lumberjackMultiplier(p); got != 1 {
		t.Errorf("no pieces: multiplier = %v, want 1", got)
	}
	hat := tables.LumberjackOutfit[0]
	top := tables.LumberjackOutfit[1]
	legs := tables.LumberjackOutfit[2]
	boots := tables.LumberjackOutfit[3]
	p.Equipment().Set(game.HeadSlot, game.NewItem(hat.ItemIDs[0], 1))
	if got := w.lumberjackMultiplier(p); got != 1.004 {
		t.Errorf("hat only: multiplier = %v, want 1.004", got)
	}
	p.Equipment().Set(game.BodySlot, game.NewItem(top.ItemIDs[0], 1))
	p.Equipment().Set(game.LegSlot, game.NewItem(legs.ItemIDs[0], 1))
	p.Equipment().Set(game.FeetSlot, game.NewItem(boots.ItemIDs[0], 1))
	if got := w.lumberjackMultiplier(p); got != 1.025 {
		t.Errorf("full set: multiplier = %v, want 1.025", got)
	}
	// A Forestry piece counts the same as a Lumberjack piece.
	p.Equipment().Set(game.HeadSlot, game.NewItem(hat.ItemIDs[1], 1))
	if got := w.lumberjackMultiplier(p); got != 1.025 {
		t.Errorf("forestry hat full set: multiplier = %v, want 1.025", got)
	}
}

// TestLogsPerChop pins the Kandarin headgear rule
// (KandarinHeadgear.Woodcutting.js): two regular logs with
// headgear 1-4 worn on a members world, else one.
func TestLogsPerChop(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	normal := treeByName(tables, "normal tree")
	oak := treeByName(tables, "oak")
	p := newTestPlayer("alice", 3200, 3200)
	if got := w.logsPerChop(p, normal.LogID); got != 1 {
		t.Errorf("no headgear: logs = %d, want 1", got)
	}
	p.Equipment().Set(game.HeadSlot, game.NewItem(kandarinHeadgearIDs[0], 1))
	if got := w.logsPerChop(p, normal.LogID); got != 2 {
		t.Errorf("headgear 1: logs = %d, want 2", got)
	}
	p.Equipment().Set(game.HeadSlot, game.NewItem(kandarinHeadgearIDs[3], 1))
	if got := w.logsPerChop(p, normal.LogID); got != 2 {
		t.Errorf("headgear 4: logs = %d, want 2", got)
	}
	if got := w.logsPerChop(p, oak.LogID); got != 1 {
		t.Errorf("headgear oak logs: logs = %d, want 1", got)
	}
	w.membersWorld = false
	if got := w.logsPerChop(p, normal.LogID); got != 1 {
		t.Errorf("free world: logs = %d, want 1", got)
	}
}

// ---- nest searching ----

// TestSearchBirdNest pins the nest search
// (plugin.js:1072-1112) with deterministic rolls.
func TestSearchBirdNest(t *testing.T) {
	tables := loadFixtureTables(t)
	w, emitted := newTestWoodcutting(t, tables)
	// Deterministic rolls: first seed (acorn), first ring (gold).
	w.randInt = func(min, max int) int { return 1 }

	// A seed nest yields the rolled seed plus an empty nest.
	p := newTestPlayer("alice", 3200, 3200)
	p.Inventory().Adds(tables.BirdNests.SeedNest, 1)
	if !w.searchBirdNest(p, tables.BirdNests.SeedNest) {
		t.Fatal("seed nest: want searched")
	}
	if p.Inventory().Contains(tables.BirdNests.SeedNest) {
		t.Error("seed nest: nest not consumed")
	}
	if !p.Inventory().Contains(tables.BirdNests.EmptyNest) {
		t.Error("seed nest: want an empty nest added")
	}
	seedID := tables.NestSeeds[0].ID
	if !p.Inventory().Contains(seedID) {
		t.Errorf("seed nest: want seed %d added", seedID)
	}
	found := false
	for _, e := range *emitted {
		if e.name == nestSearchEventName {
			if ev, ok := e.payload.(*NestSearchEvent); ok && ev.ItemID == seedID {
				found = true
			}
		}
	}
	if !found {
		t.Error("seed nest: want a woodcutting:nest-search emit")
	}

	// A ring nest yields the rolled ring plus an empty nest.
	q := newTestPlayer("bob", 3200, 3200)
	q.Inventory().Adds(tables.BirdNests.RingNest, 1)
	if !w.searchBirdNest(q, tables.BirdNests.RingNest) {
		t.Fatal("ring nest: want searched")
	}
	if !q.Inventory().Contains(tables.NestRings[0].ID) {
		t.Errorf("ring nest: want ring %d added", tables.NestRings[0].ID)
	}

	// Egg nests give the matching egg.
	for nestID, eggID := range map[int]int{
		tables.BirdNests.RedEggNest:   redBirdEggID,
		tables.BirdNests.GreenEggNest: greenBirdEggID,
		tables.BirdNests.BlueEggNest:  blueBirdEggID,
	} {
		r := newTestPlayer("egg", 3200, 3200)
		r.Inventory().Adds(nestID, 1)
		if !w.searchBirdNest(r, nestID) {
			t.Fatalf("egg nest %d: want searched", nestID)
		}
		if !r.Inventory().Contains(eggID) {
			t.Errorf("egg nest %d: want egg %d", nestID, eggID)
		}
	}

	// An unsearchable nest is left alone.
	r := newTestPlayer("plain", 3200, 3200)
	r.Inventory().Adds(tables.BirdNests.EmptyNest, 1)
	if w.searchBirdNest(r, tables.BirdNests.EmptyNest) {
		t.Error("empty nest: want not searchable")
	}

	// A full inventory keeps the nest: no consumption, no empty
	// nest, and the statement dialogue opens.
	full2 := newTestPlayer("full2", 3200, 3200)
	full2.Inventory().Adds(tables.BirdNests.SeedNest, 1)
	for i := 0; i < 27; i++ {
		full2.Inventory().Adds(9000+i, 1)
	}
	if !full2.Inventory().IsFull() {
		t.Fatal("setup: inventory not full")
	}
	if !w.searchBirdNest(full2, tables.BirdNests.SeedNest) {
		t.Fatal("full inventory: want the dialogue path taken")
	}
	if !full2.Inventory().Contains(tables.BirdNests.SeedNest) {
		t.Error("full inventory: nest must be kept")
	}
	if full2.Inventory().Contains(tables.BirdNests.EmptyNest) {
		t.Error("full inventory: no empty nest must be added")
	}
}

// ---- sessions ----

// startChopping starts a session on a fixture normal tree for
// the given player, returning the tree object.
func startChopping(t *testing.T, w *woodcutting, tables *Tables, objects *fakeObjects, p *game.Player) *game.GameObject {
	t.Helper()
	normal := treeByName(tables, "normal tree")
	bronze := axeByItem(tables, "bronze_axe")
	if normal == nil || bronze == nil {
		t.Fatal("fixture tables missing normal tree or bronze axe")
	}
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	obj := placeTree(tables, objects, normal)
	if !w.startWoodcutting(p, obj, normal) {
		t.Fatal("startWoodcutting failed")
	}
	return obj
}

// TestStartWoodcuttingGuards pins the start guards
// (plugin.js:1260-1320): no axe, low level, and a missing tree
// each refuse with a message and no session.
func TestStartWoodcuttingGuards(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	normal := treeByName(tables, "normal tree")
	bronze := axeByItem(tables, "bronze_axe")
	obj := placeTree(tables, objects, normal)

	// No axe.
	p := newTestPlayer("noaxe", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	if w.startWoodcutting(p, obj, normal) {
		t.Error("no axe: want start refused")
	}
	if _, ok := w.sessions[p]; ok {
		t.Error("no axe: want no session")
	}
	// Low level.
	q := newTestPlayer("low", 3200, 3200)
	setLevel(q, game.SkillWoodcutting, 1)
	q.Inventory().Adds(bronze.ID, 1)
	oak := treeByName(tables, "oak")
	oakObj := game.NewGameObject(oak.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(oakObj, true)
	if w.startWoodcutting(q, oakObj, oak) {
		t.Error("level 1 on oak: want start refused")
	}
	// Tree gone (never registered).
	r := newTestPlayer("gone", 3200, 3200)
	setLevel(r, game.SkillWoodcutting, 99)
	r.Inventory().Adds(bronze.ID, 1)
	missing := game.NewGameObject(normal.ObjectIDs[0], game.Location{X: 3300, Y: 3300, Z: 0}, 10, 0, nil)
	if w.startWoodcutting(r, missing, normal) {
		t.Error("missing tree: want start refused")
	}
}

// TestTreeAtFindsMapOriginal is the regression test for the
// Phase 32 live smoke failure: a real map tree stands only as
// a map original — ObjectsAt never sees it — so treeAt must
// find it through the Get leg, or chopping can never start
// ("You can't reach that tree right now.").
func TestTreeAtFindsMapOriginal(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	normal := treeByName(tables, "normal tree")
	bronze := axeByItem(tables, "bronze_axe")
	obj := game.NewGameObject(normal.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.placeMapOriginal(obj)
	if len(objects.ObjectsAt(obj.Location())) != 0 {
		t.Fatal("setup: a map original must not appear in ObjectsAt")
	}
	p := newTestPlayer("maporiginal", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	if !w.startWoodcutting(p, obj, normal) {
		t.Fatal("map-original tree: want start to succeed")
	}
	if _, ok := w.sessions[p]; !ok {
		t.Error("map-original tree: want a session after start")
	}
}

// TestTreeAtNilAfterStumpSwap pins the depletion half of the
// Get leg: once the stump replaces the map-original tree,
// treeAt answers nil for the tree id, so the session stops
// instead of chopping a felled tree.
func TestTreeAtNilAfterStumpSwap(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	normal := treeByName(tables, "normal tree")
	obj := game.NewGameObject(normal.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.placeMapOriginal(obj)
	if got := w.treeAt(obj.ID(), obj.Location(), nil, obj.Type(), obj.Face()); got == nil {
		t.Fatal("setup: treeAt must find the map-original tree")
	}
	stump := game.NewGameObject(normal.StumpID, obj.Location(), obj.Type(), obj.Face(), nil)
	objects.ReplaceInPlace(obj, stump)
	if got := w.treeAt(obj.ID(), obj.Location(), nil, obj.Type(), obj.Face()); got != nil {
		t.Errorf("after stump swap: treeAt = %v, want nil", got)
	}
	// The stump itself is still findable under its own id.
	if got := w.treeAt(normal.StumpID, obj.Location(), nil, obj.Type(), obj.Face()); got == nil {
		t.Error("after stump swap: treeAt must find the stump under its own id")
	}
}

// TestSessionTickChopsLogs pins the live loop for a normal
// tree: the first roll lands 2 ticks after start, the log and
// (truncated) XP land, the non-multi tree depletes to its
// stump, and the session stops.
func TestSessionTickChopsLogs(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.randFloat = func() float64 { return 0 } // every roll succeeds
	p := newTestPlayer("alice", 3200, 3200)
	obj := startChopping(t, w, tables, objects, p)
	normal := treeByName(tables, "normal tree")
	if _, ok := w.sessions[p]; !ok {
		t.Fatal("want a session after start")
	}
	// Tick 1: before the first roll, nothing lands.
	w.processWoodcuttingTick(1)
	if p.Inventory().Contains(normal.LogID) {
		t.Error("tick 1: no log yet")
	}
	// Tick 2: the first roll lands a log and the tree falls.
	w.processWoodcuttingTick(2)
	if !p.Inventory().Contains(normal.LogID) {
		t.Error("tick 2: want a log in the inventory")
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 25 {
		t.Errorf("Woodcutting XP = %d, want 25", got)
	}
	if logsChoppedCount(p) != 1 {
		t.Errorf("logs chopped = %d, want 1", logsChoppedCount(p))
	}
	if _, ok := w.sessions[p]; ok {
		t.Error("tree depleted: want the session stopped")
	}
	if got := objects.ObjectsAt(obj.Location()); len(got) != 1 || got[0].ID() != normal.StumpID {
		t.Errorf("tile after depletion: want the stump %d, got %v", normal.StumpID, got)
	}
}

// TestSessionStopsOnWalkAway pins the distance stop
// (plugin.js:1361-1366): a player who walks away stops
// chopping.
func TestSessionStopsOnWalkAway(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.randFloat = func() float64 { return 1 } // every roll misses
	p := newTestPlayer("alice", 3200, 3200)
	startChopping(t, w, tables, objects, p)
	p.SetLocation(game.Location{X: 3210, Y: 3200, Z: 0})
	w.processWoodcuttingTick(2)
	if _, ok := w.sessions[p]; ok {
		t.Error("walked away: want the session stopped")
	}
}

// TestSessionStopsOnTreeGone pins the tree-gone stop
// (plugin.js:1345-1348): a deregistered tree stops chopping.
func TestSessionStopsOnTreeGone(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.randFloat = func() float64 { return 1 }
	p := newTestPlayer("alice", 3200, 3200)
	obj := startChopping(t, w, tables, objects, p)
	objects.Deregister(obj, true)
	w.processWoodcuttingTick(2)
	if _, ok := w.sessions[p]; ok {
		t.Error("tree gone: want the session stopped")
	}
}

// TestSessionStopsOnDisconnect pins the disconnect stop
// (plugin.js:1526-1528): no animation reset, no session left.
func TestSessionStopsOnDisconnect(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	p := newTestPlayer("alice", 3200, 3200)
	startChopping(t, w, tables, objects, p)
	w.onPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p})
	if _, ok := w.sessions[p]; ok {
		t.Error("disconnect: want the session stopped")
	}
}

// TestSessionStopsOnInventoryFull pins the inventory-full stop
// (plugin.js:1378-1384): a full inventory at the tick stops
// chopping without an animation reset.
func TestSessionStopsOnInventoryFull(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.randFloat = func() float64 { return 1 }
	p := newTestPlayer("alice", 3200, 3200)
	startChopping(t, w, tables, objects, p)
	// Fill the rest of the inventory with junk.
	for i := 0; i < 27; i++ {
		p.Inventory().Adds(9000+i, 1)
	}
	if !p.Inventory().IsFull() {
		t.Fatal("setup: inventory not full")
	}
	w.processWoodcuttingTick(2)
	if _, ok := w.sessions[p]; ok {
		t.Error("inventory full: want the session stopped")
	}
}

// TestSessionStopsOnMovement pins the movement stop
// (plugin.js:1336-1340): a non-empty movement queue stops
// chopping.
func TestSessionStopsOnMovement(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.randFloat = func() float64 { return 1 }
	p := newTestPlayer("alice", 3200, 3200)
	startChopping(t, w, tables, objects, p)
	p.MovementQueue().AddCheckpoint(game.Location{X: 3205, Y: 3200, Z: 0})
	if p.MovementQueue().Size() == 0 {
		t.Fatal("setup: movement queue took no checkpoint")
	}
	w.processWoodcuttingTick(2)
	if _, ok := w.sessions[p]; ok {
		t.Error("moving: want the session stopped")
	}
}

// TestDepleteTreeRespawn pins the stump regrowth
// (TreeRespawnTask, plugin.js:1137-1165): the delayed one-shot
// task deregisters the stump and re-registers the original
// tree, so the tile holds the tree again.
func TestDepleteTreeRespawn(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	tasks := game.NewTaskManager()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	w.tasks = tasks
	w.randFloat = func() float64 { return 0 }
	p := newTestPlayer("alice", 3200, 3200)
	obj := startChopping(t, w, tables, objects, p)
	normal := treeByName(tables, "normal tree")
	w.processWoodcuttingTick(2)
	if got := objects.ObjectsAt(obj.Location()); len(got) != 1 || got[0].ID() != normal.StumpID {
		t.Fatalf("after depletion: want the stump, got %v", objects.ObjectsAt(obj.Location()))
	}
	for cycle := int64(1); cycle <= 200; cycle++ {
		tasks.Process(cycle)
	}
	got := objects.ObjectsAt(obj.Location())
	if len(got) != 1 || got[0].ID() != obj.ID() {
		t.Errorf("after regrowth: want the tree %d, got %v", obj.ID(), got)
	}
}

// TestGroupBonus pins the Forestry group bonus
// (plugin.js:777-793): +1 per chopper on the same tree, 0 on a
// normal tree or a requested session.
func TestGroupBonus(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	oak := treeByName(tables, "oak")
	oakObj := game.NewGameObject(oak.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(oakObj, true)
	bronze := axeByItem(tables, "bronze_axe")
	chop := func(name string) *game.Player {
		p := newTestPlayer(name, 3200, 3200)
		setLevel(p, game.SkillWoodcutting, 99)
		p.Inventory().Adds(bronze.ID, 1)
		if !w.startWoodcutting(p, oakObj, oak) {
			t.Fatalf("startWoodcutting(%s) failed", name)
		}
		return p
	}
	a := chop("a")
	b := chop("b")
	if got := w.groupBonus(a, w.sessions[a]); got != 2 {
		t.Errorf("two choppers: bonus = %d, want 2", got)
	}
	if got := w.groupBonus(b, w.sessions[b]); got != 2 {
		t.Errorf("two choppers (b): bonus = %d, want 2", got)
	}
	// A normal tree pays no bonus even with two choppers.
	normal := treeByName(tables, "normal tree")
	w.sessions[a].tree = normal
	if got := w.groupBonus(a, w.sessions[a]); got != 0 {
		t.Errorf("normal tree: bonus = %d, want 0", got)
	}
	// A requested session pays no bonus.
	w.sessions[a].tree = oak
	w.sessions[a].requested = true
	if got := w.groupBonus(a, w.sessions[a]); got != 0 {
		t.Errorf("requested session: bonus = %d, want 0", got)
	}
}

// TestLevelUpPause pins the level-up hold
// (skillactions.PauseForLevelUp): a pop-up pauses the session,
// its resume clears the flag.
func TestLevelUpPause(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	p := newTestPlayer("alice", 3200, 3200)
	startChopping(t, w, tables, objects, p)
	var resume func()
	event := &skillactions.LevelUpPopupOpenedPayload{
		Player: p,
		Skill:  game.SkillWoodcutting,
		Pause: func(action any, resumeFn, cancel func()) {
			resume = resumeFn
		},
	}
	w.pauseWoodcutting(event)
	if sess := w.sessions[p]; sess == nil || !sess.levelUpPaused {
		t.Fatal("want the session paused")
	}
	// A paused session is skipped by the tick.
	w.randFloat = func() float64 { return 0 }
	w.processWoodcuttingTick(2)
	if p.Inventory().Contains(treeByName(tables, "normal tree").LogID) {
		t.Error("paused session: no log must land")
	}
	if resume == nil {
		t.Fatal("want a resume closure captured")
	}
	resume()
	if sess := w.sessions[p]; sess == nil || sess.levelUpPaused {
		t.Error("want the session resumed")
	}
}

// ---- handlers ----

// TestHandleChopClaims pins handleChop (plugin.js:1478-1489):
// a tree click is always claimed, even when the start fails,
// and a non-tree click falls through.
func TestHandleChopClaims(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	normal := treeByName(tables, "normal tree")
	obj := placeTree(tables, objects, normal)
	def := fakeObjectDef{name: "Tree", interactions: []string{"Chop down", "Chop Down"}}
	// No axe: the start fails, but the click is still claimed.
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: obj, Definition: def, ObjectID: obj.ID(),
		ClickType: 1, Option: "chop down", HasOption: true, Location: obj.Location(),
	}
	if !w.handleChop(event) {
		t.Error("tree click with no axe: want claimed")
	}
	if _, ok := w.sessions[p]; ok {
		t.Error("failed start: want no session")
	}
	// A non-tree click falls through.
	other := game.NewGameObject(999999, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	otherEvent := &plugins.ObjectInteractionEvent{
		Player: p, Object: other, Definition: fakeObjectDef{name: "Rock", interactions: []string{"Mine"}},
		ObjectID: 999999, ClickType: 1, Option: "mine", HasOption: true, Location: other.Location(),
	}
	if w.handleChop(otherEvent) {
		t.Error("non-tree click: want not claimed")
	}
}

// TestHandleChopPestControl pins the object-id lookup: the
// pest-control tree (TREES_BY_OBJECT_ID, plugin.js:665) is
// found by id even with no matching definition name.
func TestHandleChopPestControl(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	pest := tables.PestControlTree
	if pest == nil || len(pest.ObjectIDs) == 0 {
		t.Fatal("fixture tables missing the pest-control tree")
	}
	bronze := axeByItem(tables, "bronze_axe")
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	obj := game.NewGameObject(pest.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(obj, true)
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: obj, Definition: fakeObjectDef{name: "Tree", interactions: []string{"Chop down"}},
		ObjectID: obj.ID(), ClickType: 1, Option: "chop down", HasOption: true, Location: obj.Location(),
	}
	if !w.handleChop(event) {
		t.Fatal("pest-control click: want claimed")
	}
	sess := w.sessions[p]
	if sess == nil || sess.tree != pest {
		t.Error("pest-control click: want a session on the pest-control tree")
	}
	if sess != nil && (sess.tree.LogLine || sess.tree.CountsLogs || sess.tree.RespawnTimer) {
		t.Error("pest-control tree: logLine/countsLogs/respawnTimer must be false")
	}
}

// TestRequestedChop pins the farmed-tree lookup and the
// removeOnly clone (plugin.js:1491-1506).
func TestRequestedChop(t *testing.T) {
	tables := loadFixtureTables(t)
	objects := newFakeObjects()
	w, _ := newTestWoodcutting(t, tables)
	w.objects = objects
	redwood := treeByName(tables, "redwood")
	if redwood == nil {
		t.Fatal("fixture tables missing redwood")
	}
	bronze := axeByItem(tables, "bronze_axe")
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	obj := game.NewGameObject(redwood.ObjectIDs[0], game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(obj, true)
	// A farmed redwood is found by log id before the wild one.
	request := &ChopRequest{Player: p, Object: obj, LogID: redwood.LogID}
	w.requestedChop(request)
	if !request.Handled {
		t.Fatal("farmed redwood chop: want handled")
	}
	sess := w.sessions[p]
	if sess == nil {
		t.Fatal("farmed redwood chop: want a session")
	}
	if sess.tree.RespawnTicks != FarmedRedwoodRegrowTicks || sess.tree.DepleteChance != FarmedRedwoodDepletionRoll {
		t.Errorf("farmed redwood: respawn %d / deplete %d, want %d / %d",
			sess.tree.RespawnTicks, sess.tree.DepleteChance, FarmedRedwoodRegrowTicks, FarmedRedwoodDepletionRoll)
	}
	if !sess.requested {
		t.Error("requested chop: want the session marked requested")
	}
	// removeOnly clones to no-log/no-XP.
	w.stopWoodcutting(p, false)
	remove := &ChopRequest{Player: p, Object: obj, LogID: redwood.LogID, RemoveOnly: true}
	w.requestedChop(remove)
	if !remove.Handled {
		t.Fatal("removeOnly chop: want handled")
	}
	sess = w.sessions[p]
	if sess == nil || sess.tree.LogID != -1 || sess.tree.XPReward != 0 || sess.tree.Multi {
		t.Errorf("removeOnly clone: %+v, want log -1, xp 0, multi false", sess.tree)
	}
}

// TestRequestedAxe pins the axe request (plugin.js:1508-1514).
func TestRequestedAxe(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	bronze := axeByItem(tables, "bronze_axe")
	rune := axeByItem(tables, "rune_axe")
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	// Nil item id asks for the best usable axe.
	any := &AxeRequest{Player: p}
	w.requestedAxe(any)
	if any.Axe == nil || any.Axe.ID != bronze.ID {
		t.Errorf("any axe: got %v, want bronze %d", any.Axe, bronze.ID)
	}
	// A named usable axe answers itself.
	named := &AxeRequest{Player: p, ItemID: &bronze.ID}
	w.requestedAxe(named)
	if named.Axe == nil || named.Axe.ID != bronze.ID {
		t.Errorf("named bronze: got %v, want bronze", named.Axe)
	}
	// A named usable axe answers itself — possession is NOT
	// checked (plugin.js:1512-1513 tests eligibility and
	// canUseAxe only).
	runeID := rune.ID
	unheld := &AxeRequest{Player: p, ItemID: &runeID}
	w.requestedAxe(unheld)
	if unheld.Axe == nil || unheld.Axe.ID != rune.ID {
		t.Errorf("unheld but usable rune: got %v, want rune", unheld.Axe)
	}
	// A named axe the player cannot use answers nil.
	crystal := axeByItem(tables, "crystal_axe")
	setLevel(p, game.SkillAgility, 1)
	crystalID := crystal.ID
	unusable := &AxeRequest{Player: p, ItemID: &crystalID}
	w.requestedAxe(unusable)
	if unusable.Axe != nil {
		t.Errorf("crystal with Agility 1: got %v, want nil", unusable.Axe)
	}
	// An unknown item id answers nil.
	unknownID := 123456
	unknown := &AxeRequest{Player: p, ItemID: &unknownID}
	w.requestedAxe(unknown)
	if unknown.Axe != nil {
		t.Errorf("unknown id: got %v, want nil", unknown.Axe)
	}
}

// TestObjectRegistrationCount pins the 27 object registrations
// (plugin.js:1543-1549): normal tree 4 names x 2 actions, 19
// others x 1; scrapey 0.
func TestObjectRegistrationCount(t *testing.T) {
	tables := loadFixtureTables(t)
	count := 0
	for _, tree := range tables.Trees {
		count += len(tree.ObjectNames) * len(tree.Actions)
	}
	if count != 27 {
		t.Errorf("object registrations = %d, want 27", count)
	}
}

// TestPluginIdentity pins the plugin value verbatim
// (module.exports, plugin.js:1556-1559).
func TestPluginIdentity(t *testing.T) {
	if Plugin.Name != "Woodcutting" {
		t.Errorf("Name = %q, want Woodcutting", Plugin.Name)
	}
	want := "Woodcutting: chopping trees with the Forestry despawn timer, stump respawns, the logs-chopped tracker, and bird's nest and ring drops."
	if Plugin.Description != want {
		t.Errorf("Description = %q, want %q", Plugin.Description, want)
	}
	if Plugin.Members {
		t.Error("Members = true, want false (members gating is per-feature)")
	}
	if Plugin.Register == nil {
		t.Error("Register = nil")
	}
}

// TestHubRegistrationFires pins the whole wiring through a
// hub: tables installed, hub loaded, a tree click starts a
// session through the named object hook, and the tick task
// chops a log.
func TestHubRegistrationFires(t *testing.T) {
	tables := loadFixtureTables(t)
	SetTables(tables)
	defer SetTables(nil)
	objects := newFakeObjects()
	tasks := game.NewTaskManager()
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.SetTaskManager(tasks)
	hub.SetObjectManager(objects)
	hub.Load()
	if len(hub.Loaded()) != 1 || hub.Loaded()[0] != "Woodcutting" {
		t.Fatalf("hub loaded = %v, want [Woodcutting]", hub.Loaded())
	}
	normal := treeByName(tables, "normal tree")
	bronze := axeByItem(tables, "bronze_axe")
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 99)
	p.Inventory().Adds(bronze.ID, 1)
	obj := placeTree(tables, objects, normal)
	// Drive the shared instance's randomness: every roll
	// succeeds. (The hub test reaches the shared instance
	// through SetTables; the rand override is test-only.)
	origRandFloat := shared.randFloat
	shared.randFloat = func() float64 { return 0 }
	defer func() { shared.randFloat = origRandFloat }()
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: obj,
		Definition: fakeObjectDef{name: "Tree", interactions: []string{"Chop down", "Chop Down"}},
		ObjectID:   obj.ID(), ClickType: 1, Option: "chop down", HasOption: true,
		Location: obj.Location(),
	}
	if !hub.EmitObjectInteraction(event) {
		t.Fatal("tree click through the hub: want handled")
	}
	if _, ok := shared.sessions[p]; !ok {
		t.Fatal("tree click through the hub: want a session")
	}
	// Three task ticks: the first roll lands on the second.
	for cycle := int64(1); cycle <= 3; cycle++ {
		tasks.Process(cycle)
	}
	if !p.Inventory().Contains(normal.LogID) {
		t.Error("after 3 ticks: want a log in the inventory")
	}
	if p.Skills().Experience(game.SkillWoodcutting) != 25 {
		t.Errorf("Woodcutting XP = %d, want 25", p.Skills().Experience(game.SkillWoodcutting))
	}
}
