package ladders

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fixtureDefs is a Defs over a literal map.
type fixtureDefs map[int]ObjectDef

func (f fixtureDefs) Def(id int) (ObjectDef, bool) {
	def, ok := f[id]
	return def, ok
}

// fixtureObjects is a MapObjects over a literal tile map.
type fixtureObjects struct {
	byTile map[game.Location][]plugins.GameObject
}

func (f *fixtureObjects) ObjectsAtTile(loc game.Location) []plugins.GameObject {
	return f.byTile[loc]
}

func (f *fixtureObjects) put(obj *game.GameObject) {
	if f.byTile == nil {
		f.byTile = map[game.Location][]plugins.GameObject{}
	}
	loc := obj.Location()
	f.byTile[loc] = append(f.byTile[loc], obj)
}

func obj(id, x, y, z, typ, face int) *game.GameObject {
	return game.NewGameObject(id, game.Location{X: x, Y: y, Z: z}, typ, face, nil)
}

func def(name string, sizeX, sizeY int, actions ...string) ObjectDef {
	return ObjectDef{Name: name, SizeX: sizeX, SizeY: sizeY, Actions: actions}
}

func testResolver(defs fixtureDefs, objects *fixtureObjects) *Resolver {
	return &Resolver{Defs: defs, Objects: objects}
}

func TestClimbsOptionMatching(t *testing.T) {
	defs := fixtureDefs{
		1:  def("Ladder", 1, 1, "Climb-up"),
		2:  def("Ladder", 1, 1, "Climb Up"),
		3:  def("Ladder", 1, 1, "CLIMB-UP"),
		4:  def("Ladder", 1, 1, "Climb-down"),
		5:  def("Ladder", 1, 1, "Climb"),
		6:  def("Staircase", 1, 1, "Top-floor"),
		7:  def("Staircase", 1, 1, "Bottom-Floor"),
		8:  def("Staircase", 1, 1, "Ascend"),
		9:  def("Staircase", 1, 1, "Descend"),
		10: def("Staircase", 1, 1, "Walk-Up"),
		11: def("Ladder", 1, 1, "Open"),
	}
	r := testResolver(defs, &fixtureObjects{})
	cases := []struct {
		id, dir int
		want    bool
	}{
		{1, Up, true}, {1, Down, false},
		{2, Up, true}, {3, Up, true}, {4, Up, false}, {4, Down, true},
		{5, Up, true}, {5, Down, true},
		{6, Up, true}, {6, Down, false},
		{7, Down, true}, {7, Up, false},
		{8, Up, true}, {8, Down, false},
		{9, Down, true}, {9, Up, false},
		{10, Up, true}, {11, Up, false}, {11, Down, false},
		{999, Up, false},
	}
	for _, c := range cases {
		if got := r.Climbs(c.id, c.dir); got != c.want {
			t.Errorf("Climbs(%d, %d) = %v, want %v", c.id, c.dir, got, c.want)
		}
	}
}

func TestDeclaredPairBothDirections(t *testing.T) {
	// Wizards' Tower basement ladder (climb-links.json:18-20):
	// up 2148 at (3103,9576,0), down 2147 at (3104,3162,0), no
	// landing overrides — the landing is computed.
	defs := fixtureDefs{
		2147: def("Ladder", 1, 1, "Climb-down"),
		2148: def("Ladder", 1, 1, "Climb-up"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(2147, 3104, 3162, 0, 10, 3))
	objects.put(obj(2148, 3103, 9576, 0, 10, 3))
	r := testResolver(defs, objects)

	up := r.Destination(obj(2147, 3104, 3162, 0, 10, 3), Down, game.Location{X: 3104, Y: 3162, Z: 0}, nil, false)
	// Climbing down on 2147 goes to the declared other end 2148;
	// the landing is the tile adjacent to it nearest the hint
	// (3104,9576): (3104,9576,0).
	if up == nil || *up != (game.Location{X: 3104, Y: 9576, Z: 0}) {
		t.Fatalf("Destination(down 2147) = %v, want (3104,9576,0)", up)
	}
	down := r.Destination(obj(2148, 3103, 9576, 0, 10, 3), Up, game.Location{X: 3103, Y: 9576, Z: 0}, nil, false)
	if down == nil || *down != (game.Location{X: 3103, Y: 3162, Z: 0}) {
		t.Fatalf("Destination(up 2148) = %v, want (3103,3162,0)", down)
	}
	// Wrong direction on a declared endpoint resolves nowhere.
	if got := r.OtherEnd(obj(2147, 3104, 3162, 0, 10, 3), Up); got != nil {
		t.Errorf("OtherEnd(2147, up) = %v, want nil", got.Location())
	}
}

func TestDeclaredLandingOverride(t *testing.T) {
	// Port Sarim Rat Pits manhole (climb-links.json:11-15):
	// landing (3018,3233,0) up, downLanding (2962,9650,0) down —
	// returned directly, without consulting the other end.
	defs := fixtureDefs{
		10309: def("Ladder", 1, 1, "Climb-up"),
		10321: def("Manhole", 1, 1, "Climb-down"),
	}
	r := testResolver(defs, &fixtureObjects{})
	up := r.Destination(obj(10309, 2962, 9651, 0, 10, 0), Up, game.Location{X: 2962, Y: 9651, Z: 0}, nil, false)
	if up == nil || *up != (game.Location{X: 3018, Y: 3233, Z: 0}) {
		t.Fatalf("Destination(up 10309) = %v, want (3018,3233,0)", up)
	}
	down := r.Destination(obj(10321, 3018, 3232, 0, 10, 0), Down, game.Location{X: 3018, Y: 3232, Z: 0}, nil, false)
	if down == nil || *down != (game.Location{X: 2962, Y: 9650, Z: 0}) {
		t.Fatalf("Destination(down 10321) = %v, want (2962,9650,0)", down)
	}
}

func TestSameColumnSearch(t *testing.T) {
	defs := fixtureDefs{
		100: def("Ladder", 1, 1, "Climb-up"),
		101: def("Ladder", 1, 1, "Climb-down"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(100, 100, 100, 0, 10, 0))
	objects.put(obj(101, 102, 101, 1, 10, 0))
	r := testResolver(defs, objects)
	got := r.Destination(obj(100, 100, 100, 0, 10, 0), Up, game.Location{X: 100, Y: 100, Z: 0}, nil, false)
	if got == nil || *got != (game.Location{X: 101, Y: 100, Z: 1}) {
		t.Fatalf("Destination = %v, want (101,100,1)", got)
	}
	// The other end 6 tiles away is outside SEARCH_RADIUS + 1.
	objects2 := &fixtureObjects{}
	objects2.put(obj(100, 100, 100, 0, 10, 0))
	objects2.put(obj(101, 106, 100, 1, 10, 0))
	r2 := testResolver(defs, objects2)
	if got := r2.Destination(obj(100, 100, 100, 0, 10, 0), Up, game.Location{X: 100, Y: 100, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (far end) = %v, want nil", got)
	}
}

func TestUndergroundOffset(t *testing.T) {
	// Lumbridge cellar trapdoor (3209,3216,0) <-> cellar ladder
	// (3209,9616,0): the 6400-tile underground offset.
	defs := fixtureDefs{
		14880: def("Trapdoor", 1, 1, "Climb-down"),
		17385: def("Ladder", 1, 1, "Climb-up"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(14880, 3209, 3216, 0, 22, 0))
	objects.put(obj(17385, 3209, 9616, 0, 10, 3))
	r := testResolver(defs, objects)

	down := r.Destination(obj(14880, 3209, 3216, 0, 22, 0), Down, game.Location{X: 3209, Y: 3216, Z: 0}, nil, false)
	if down == nil || *down != (game.Location{X: 3208, Y: 9616, Z: 0}) {
		t.Fatalf("Destination(down trapdoor) = %v, want (3208,9616,0)", down)
	}
	up := r.Destination(obj(17385, 3209, 9616, 0, 10, 3), Up, game.Location{X: 3209, Y: 9616, Z: 0}, nil, false)
	if up == nil || *up != (game.Location{X: 3208, Y: 3216, Z: 0}) {
		t.Fatalf("Destination(up cellar ladder) = %v, want (3208,3216,0)", up)
	}
}

func TestInertSegmentChainSkip(t *testing.T) {
	defs := fixtureDefs{
		200: def("Ladder", 1, 1, "Climb-up"),
		201: def("Ladder", 1, 1), // inert: same name, no climb option
		202: def("Ladder", 1, 1, "Climb-down"),
		203: def("Ladder", 1, 1, "Climb-down"),
	}
	// One inert segment on plane 1, live end on plane 2.
	objects := &fixtureObjects{}
	objects.put(obj(200, 50, 50, 0, 10, 0))
	objects.put(obj(201, 50, 50, 1, 10, 0))
	objects.put(obj(202, 50, 50, 2, 10, 0))
	r := testResolver(defs, objects)
	got := r.Destination(obj(200, 50, 50, 0, 10, 0), Up, game.Location{X: 50, Y: 50, Z: 0}, nil, false)
	if got == nil || got.Z != 2 {
		t.Fatalf("Destination (one inert) = %v, want plane 2", got)
	}
	// Two inert segments (planes 1, 2), live end on plane 3.
	objects2 := &fixtureObjects{}
	objects2.put(obj(200, 50, 50, 0, 10, 0))
	objects2.put(obj(201, 50, 50, 1, 10, 0))
	objects2.put(obj(201, 50, 50, 2, 10, 0))
	objects2.put(obj(203, 50, 50, 3, 10, 0))
	r2 := testResolver(defs, objects2)
	got = r2.Destination(obj(200, 50, 50, 0, 10, 0), Up, game.Location{X: 50, Y: 50, Z: 0}, nil, false)
	if got == nil || got.Z != 3 {
		t.Fatalf("Destination (two inert) = %v, want plane 3", got)
	}
	// No inert segment and no live end on plane 1: no climb.
	objects3 := &fixtureObjects{}
	objects3.put(obj(200, 50, 50, 0, 10, 0))
	objects3.put(obj(202, 50, 50, 2, 10, 0))
	r3 := testResolver(defs, objects3)
	if got := r3.Destination(obj(200, 50, 50, 0, 10, 0), Up, game.Location{X: 50, Y: 50, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (gap, no inert) = %v, want nil", got)
	}
	// A different-named object on plane 1 does not unlock the skip.
	objects4 := &fixtureObjects{}
	objects4.put(obj(200, 50, 50, 0, 10, 0))
	objects4.put(obj(201, 50, 50, 1, 10, 0))
	objects4.put(obj(202, 50, 50, 2, 10, 0))
	defs4 := fixtureDefs{
		200: def("Ladder", 1, 1, "Climb-up"),
		201: def("Staircase", 1, 1),
		202: def("Ladder", 1, 1, "Climb-down"),
	}
	r4 := testResolver(defs4, objects4)
	if got := r4.Destination(obj(200, 50, 50, 0, 10, 0), Up, game.Location{X: 50, Y: 50, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (other-name segment) = %v, want nil", got)
	}
}

func TestNoOtherEnd(t *testing.T) {
	defs := fixtureDefs{100: def("Ladder", 1, 1, "Climb-up")}
	objects := &fixtureObjects{}
	objects.put(obj(100, 100, 100, 0, 10, 0))
	r := testResolver(defs, objects)
	if got := r.Destination(obj(100, 100, 100, 0, 10, 0), Up, game.Location{X: 100, Y: 100, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (lone ladder) = %v, want nil", got)
	}
	// An object above that climbs the SAME way is not the other end.
	defs[101] = def("Ladder", 1, 1, "Climb-up")
	objects.put(obj(101, 100, 100, 1, 10, 0))
	if got := r.Destination(obj(100, 100, 100, 0, 10, 0), Up, game.Location{X: 100, Y: 100, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (same-way above) = %v, want nil", got)
	}
}

func TestLandingTileNearestAndBlocked(t *testing.T) {
	defs := fixtureDefs{
		100: def("Ladder", 1, 1, "Climb-up"),
		101: def("Ladder", 1, 1, "Climb-down"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(100, 199, 200, 0, 10, 0))
	objects.put(obj(101, 200, 200, 1, 10, 0))
	// The nearest landing (199,200,1) is blocked: the next
	// nearest in scan order wins.
	r := testResolver(defs, objects)
	r.Blocked = func(loc game.Location, area *game.PrivateArea) bool {
		return loc == game.Location{X: 199, Y: 200, Z: 1}
	}
	got := r.Destination(obj(100, 199, 200, 0, 10, 0), Up, game.Location{X: 199, Y: 200, Z: 0}, nil, false)
	if got == nil || *got != (game.Location{X: 199, Y: 199, Z: 1}) {
		t.Fatalf("Destination (nearest blocked) = %v, want (199,199,1)", got)
	}
	// Every landing tile blocked: no destination.
	r.Blocked = func(loc game.Location, area *game.PrivateArea) bool { return true }
	if got := r.Destination(obj(100, 199, 200, 0, 10, 0), Up, game.Location{X: 199, Y: 200, Z: 0}, nil, false); got != nil {
		t.Errorf("Destination (all blocked) = %v, want nil", got)
	}
	// The reach check excludes a tile the stand-in cannot use the
	// object from.
	r2 := testResolver(defs, objects)
	r2.Reached = func(from game.Location, area *game.PrivateArea, destX, destY, destWidth, destLength, locAngle, locShape, blockAccessFlags int) bool {
		return from != game.Location{X: 199, Y: 199, Z: 1}
	}
	got = r2.Destination(obj(100, 199, 200, 0, 10, 0), Up, game.Location{X: 199, Y: 200, Z: 0}, nil, false)
	if got == nil || *got != (game.Location{X: 199, Y: 200, Z: 1}) {
		t.Fatalf("Destination (reach excludes nearest) = %v, want (199,200,1)", got)
	}
}

func TestToEndChaining(t *testing.T) {
	defs := fixtureDefs{
		300: def("Staircase", 1, 1, "Climb-up", "Top-floor"),
		301: def("Staircase", 1, 1, "Climb-up", "Climb-down"),
		302: def("Staircase", 1, 1, "Climb-up", "Climb-down"),
		303: def("Staircase", 1, 1, "Climb-down", "Bottom-floor"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(300, 70, 70, 0, 10, 0))
	objects.put(obj(301, 70, 70, 1, 10, 0))
	objects.put(obj(302, 70, 70, 2, 10, 0))
	objects.put(obj(303, 70, 70, 3, 10, 0))
	r := testResolver(defs, objects)
	from := game.Location{X: 70, Y: 70, Z: 0}
	one := r.Destination(obj(300, 70, 70, 0, 10, 0), Up, from, nil, false)
	if one == nil || one.Z != 1 {
		t.Fatalf("Destination (toEnd=false) = %v, want plane 1", one)
	}
	end := r.Destination(obj(300, 70, 70, 0, 10, 0), Up, from, nil, true)
	if end == nil || end.Z != 3 {
		t.Fatalf("Destination (toEnd=true) = %v, want plane 3", end)
	}
	// Down from the top with toEnd lands on the ground floor.
	top := r.Destination(obj(303, 70, 70, 3, 10, 0), Down, game.Location{X: 70, Y: 70, Z: 3}, nil, true)
	if top == nil || top.Z != 0 {
		t.Fatalf("Destination (down toEnd) = %v, want plane 0", top)
	}
}

func TestCrossDestination(t *testing.T) {
	defs := fixtureDefs{
		400: def("Gangplank", 1, 1, "Cross"),
		401: def("Gangplank", 1, 1, "Cross"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(400, 100, 100, 0, 10, 0))
	objects.put(obj(401, 100, 101, 1, 10, 0))
	r := testResolver(defs, objects)
	got := r.CrossDestination(obj(400, 100, 100, 0, 10, 0), nil)
	// The landing carries on 3 tiles past the deck half: the
	// nearest adjacent tile to the (100,104) hint is (100,102,1).
	if got == nil || *got != (game.Location{X: 100, Y: 102, Z: 1}) {
		t.Fatalf("CrossDestination = %v, want (100,102,1)", got)
	}
	// Back across, from the deck to the dock.
	back := r.CrossDestination(obj(401, 100, 101, 1, 10, 0), nil)
	if back == nil || *back != (game.Location{X: 100, Y: 99, Z: 0}) {
		t.Fatalf("CrossDestination (back) = %v, want (100,99,0)", back)
	}
	// No other half: no crossing.
	objects2 := &fixtureObjects{}
	objects2.put(obj(400, 100, 100, 0, 10, 0))
	r2 := testResolver(defs, objects2)
	if got := r2.CrossDestination(obj(400, 100, 100, 0, 10, 0), nil); got != nil {
		t.Errorf("CrossDestination (no half) = %v, want nil", got)
	}
	// A non-Cross object never crosses.
	if got := r.CrossDestination(obj(100, 100, 100, 0, 10, 0), nil); got != nil {
		t.Errorf("CrossDestination (not a gangplank) = %v, want nil", got)
	}
}

func TestHasObjectAboveBelow(t *testing.T) {
	defs := fixtureDefs{
		100: def("Staircase", 1, 1, "Climb-up"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(100, 10, 11, 1, 10, 0))
	r := testResolver(defs, objects)
	if !r.HasObjectAbove(game.Location{X: 10, Y: 10, Z: 0}, "Staircase") {
		t.Error("HasObjectAbove = false, want true")
	}
	if r.HasObjectAbove(game.Location{X: 20, Y: 20, Z: 0}, "Staircase") {
		t.Error("HasObjectAbove (far) = true, want false")
	}
	if r.HasObjectAbove(game.Location{X: 10, Y: 10, Z: 0}, "Ladder") {
		t.Error("HasObjectAbove (wrong name) = true, want false")
	}
	if !r.HasObjectBelow(game.Location{X: 10, Y: 10, Z: 2}, "Staircase") {
		t.Error("HasObjectBelow = false, want true")
	}
	// Out of plane range: no climb.
	if r.HasObjectAbove(game.Location{X: 10, Y: 10, Z: 3}, "Staircase") {
		t.Error("HasObjectAbove (plane 3) = true, want false")
	}
	if r.HasObjectBelow(game.Location{X: 10, Y: 10, Z: 0}, "Staircase") {
		t.Error("HasObjectBelow (plane 0) = true, want false")
	}
}

func TestFootprintTurned(t *testing.T) {
	defs := fixtureDefs{100: def("Staircase", 2, 1, "Climb-up")}
	r := testResolver(defs, &fixtureObjects{})
	// Face 0: width 2, length 1.
	if _, _, w, l := r.Footprint(obj(100, 10, 10, 0, 10, 0)); w != 2 || l != 1 {
		t.Errorf("Footprint face 0 = %dx%d, want 2x1", w, l)
	}
	// Face 1 (quarter turn): sizes swap.
	if _, _, w, l := r.Footprint(obj(100, 10, 10, 0, 10, 1)); w != 1 || l != 2 {
		t.Errorf("Footprint face 1 = %dx%d, want 1x2", w, l)
	}
}

func TestDeclaredPairCount(t *testing.T) {
	if n := DeclaredPairCount(); n != 7 {
		t.Errorf("DeclaredPairCount() = %d, want 7", n)
	}
}
