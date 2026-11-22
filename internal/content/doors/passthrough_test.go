package doors

import (
	"testing"

	"tsps-go/internal/game"
)

// TestFamilyGateAt pins familyGateAt (Doors.plugin.js:1851-1867): a
// family gate puts INVISWALL on the door's tile and its open leaf on
// the tile past the blocked edge, turned a quarter, with the boundary
// crossed on the blocked edge — x for face 0, x + 1 for face 2,
// y + 1 for face 1, y for face 3.
func TestFamilyGateAt(t *testing.T) {
	family := &PassThroughFamily{OpenLeaf: 1542, HasOpenLeaf: true, OpenSoundID: 62, CloseTicks: 2}
	cases := []struct {
		name          string
		face          int
		wantOpen      game.Location
		wantOpenFace  int
		wantCrossX    int
		wantHasCrossX bool
		wantCrossY    int
		wantHasCrossY bool
	}{
		{"face 0 west: leaf west, cross x", 0, game.Location{X: 2905, Y: 3543, Z: 1}, 1, 2906, true, 0, false},
		{"face 1 north: leaf north, cross y + 1", 1, game.Location{X: 2906, Y: 3544, Z: 1}, 2, 0, false, 3544, true},
		{"face 2 east: leaf east, cross x + 1", 2, game.Location{X: 2907, Y: 3543, Z: 1}, 3, 2907, true, 0, false},
		{"face 3 south: leaf south, cross y", 3, game.Location{X: 2906, Y: 3542, Z: 1}, 0, 0, false, 3543, true},
	}
	for _, c := range cases {
		gate, ok := familyGateAt(family, 3747, 2906, 3543, 1, c.face)
		if !ok {
			t.Fatalf("%s: familyGateAt = not ok", c.name)
		}
		if len(gate.Closed) != 1 || gate.Closed[0].ID != 3747 {
			t.Fatalf("%s: closed = %+v, want the door leaf", c.name, gate.Closed)
		}
		if len(gate.Opened) != 2 || gate.Opened[0].ID != namelessInviswall {
			t.Fatalf("%s: opened[0] = %+v, want INVISWALL on the door tile", c.name, gate.Opened)
		}
		open := gate.Opened[1]
		if open.ID != 1542 || (game.Location{X: open.X, Y: open.Y, Z: open.Z}) != c.wantOpen || open.Face != c.wantOpenFace {
			t.Errorf("%s: open leaf = %+v, want id 1542 at %+v face %d", c.name, open, c.wantOpen, c.wantOpenFace)
		}
		if gate.CrossX != c.wantCrossX || gate.HasCrossX != c.wantHasCrossX || gate.CrossY != c.wantCrossY || gate.HasCrossY != c.wantHasCrossY {
			t.Errorf("%s: cross = (%d,%v)/(%d,%v), want (%d,%v)/(%d,%v)",
				c.name, gate.CrossX, gate.HasCrossX, gate.CrossY, gate.HasCrossY,
				c.wantCrossX, c.wantHasCrossX, c.wantCrossY, c.wantHasCrossY)
		}
		if !gate.WalkOver || gate.CloseTicks != 2 || gate.OpenSoundID != 62 {
			t.Errorf("%s: gate = walkOver %v closeTicks %d sound %d, want true/2/62",
				c.name, gate.WalkOver, gate.CloseTicks, gate.OpenSoundID)
		}
	}
	// A family with no resolved open leaf builds no gate.
	if _, ok := familyGateAt(&PassThroughFamily{OpenSoundID: 62, CloseTicks: 2}, 2596, 1, 2, 0, 0); ok {
		t.Error("family with no open leaf: want no gate")
	}
}

// TestCarryAcrossGatePlan pins carryAcrossGate's math
// (Doors.plugin.js:1885-1900) on the West Ardougne wall gate: the
// player is carried across the span on their own row, in both
// directions, and a player more than a tile from the crossing start
// is not carried.
func TestCarryAcrossGatePlan(t *testing.T) {
	gate := &PassThroughGate{
		CrossBetween:    [2]int{2556, 2559},
		HasCrossBetween: true,
		CrossRows:       []int{3299, 3300},
	}
	west := game.Location{X: 2556, Y: 3299, Z: 0}
	from, delta, dir, ok := carryAcrossGatePlan(west, gate, 3299, 0)
	if !ok || from != west || delta != (game.Location{X: 3, Y: 0}) || dir != 1 {
		t.Errorf("west side: plan = (%+v, %+v, %d, %v), want ((2556,3299,0), (3,0), 1, true)", from, delta, dir, ok)
	}
	east := game.Location{X: 2559, Y: 3300, Z: 0}
	from, delta, dir, ok = carryAcrossGatePlan(east, gate, 3300, 0)
	if !ok || from != east || delta != (game.Location{X: -3, Y: 0}) || dir != 3 {
		t.Errorf("east side: plan = (%+v, %+v, %d, %v), want ((2559,3300,0), (-3,0), 3, true)", from, delta, dir, ok)
	}
	far := game.Location{X: 2554, Y: 3299, Z: 0}
	if _, _, _, ok := carryAcrossGatePlan(far, gate, 3299, 0); ok {
		t.Error("player two tiles away: want not carried")
	}
	// A single-row gate crosses on that row whatever the clicked
	// leaf's own y is.
	single := &PassThroughGate{
		CrossBetween:    [2]int{100, 103},
		HasCrossBetween: true,
		CrossRows:       []int{50},
	}
	from, delta, dir, ok = carryAcrossGatePlan(game.Location{X: 100, Y: 51, Z: 0}, single, 51, 0)
	if !ok || from != (game.Location{X: 100, Y: 50, Z: 0}) || delta != (game.Location{X: 3, Y: 0}) || dir != 1 {
		t.Errorf("single-row gate: plan = (%+v, %+v, %d, %v), want ((100,50,0), (3,0), 1, true)", from, delta, dir, ok)
	}
}

// TestCarryOneTilePlan pins carryOneTile's math
// (Doors.plugin.js:1907-1921): one tile over the boundary, in both
// directions, on the gate's row/column only.
func TestCarryOneTilePlan(t *testing.T) {
	gate := &PassThroughGate{CrossX: 2848, HasCrossX: true, Carry: true}
	delta, dir, ok := carryOneTilePlan(game.Location{X: 2847, Y: 10107, Z: 1}, gate, 2848, 10107, 1)
	if !ok || delta != (game.Location{X: 1}) || dir != 1 {
		t.Errorf("west of the boundary: plan = (%+v, %d, %v), want ((1,0), 1, true)", delta, dir, ok)
	}
	delta, dir, ok = carryOneTilePlan(game.Location{X: 2848, Y: 10107, Z: 1}, gate, 2848, 10107, 1)
	if !ok || delta != (game.Location{X: -1}) || dir != 3 {
		t.Errorf("on the boundary: plan = (%+v, %d, %v), want ((-1,0), 3, true)", delta, dir, ok)
	}
	if _, _, ok := carryOneTilePlan(game.Location{X: 2847, Y: 10108, Z: 1}, gate, 2848, 10107, 1); ok {
		t.Error("off the gate's row: want not carried")
	}
	if _, _, ok := carryOneTilePlan(game.Location{X: 2846, Y: 10107, Z: 1}, gate, 2848, 10107, 1); ok {
		t.Error("two tiles from the boundary: want not carried")
	}
	yGate := &PassThroughGate{CrossY: 9600, HasCrossY: true, Carry: true}
	delta, dir, ok = carryOneTilePlan(game.Location{X: 2836, Y: 9600, Z: 0}, yGate, 2836, 9600, 0)
	if !ok || delta != (game.Location{Y: -1}) || dir != 2 {
		t.Errorf("on a crossY boundary: plan = (%+v, %d, %v), want ((0,-1), 2, true)", delta, dir, ok)
	}
	delta, dir, ok = carryOneTilePlan(game.Location{X: 2836, Y: 9599, Z: 0}, yGate, 2836, 9600, 0)
	if !ok || delta != (game.Location{Y: 1}) || dir != 0 {
		t.Errorf("south of a crossY boundary: plan = (%+v, %d, %v), want ((0,1), 0, true)", delta, dir, ok)
	}
	if _, _, ok := carryOneTilePlan(game.Location{X: 2836, Y: 9601, Z: 0}, yGate, 2836, 9600, 0); ok {
		t.Error("two tiles north of a crossY boundary: want not carried")
	}
}

// TestDiagonalPlan pins carryAcrossDiagonal's math
// (Doors.plugin.js:1957-1972) on the Ranging Guild entrance: the
// opener crosses corner to corner, outside to inside when entering
// and back when leaving.
func TestDiagonalPlan(t *testing.T) {
	gate := &PassThroughGate{
		Closed:                []PassThroughLeaf{{ID: 11665, X: 2658, Y: 3438, Z: 0, Face: 0, Shape: 9}},
		CrossDiagonalOutside:  [2]int{2657, 3439},
		CrossDiagonalInside:   [2]int{2659, 3437},
		HasCrossDiagonal:      true,
		CrossDiagonalEastward: true,
	}
	outside := game.Location{X: 2657, Y: 3439, Z: 0}
	from, delta, dir, ok := diagonalPlan(outside, gate, 0)
	if !ok || from != outside || delta != (game.Location{X: 2, Y: -2}) || dir != 1 {
		t.Errorf("entering: plan = (%+v, %+v, %d, %v), want ((2657,3439,0), (2,-2), 1, true)", from, delta, dir, ok)
	}
	inside := game.Location{X: 2659, Y: 3437, Z: 0}
	from, delta, dir, ok = diagonalPlan(inside, gate, 0)
	if !ok || from != inside || delta != (game.Location{X: -2, Y: 2}) || dir != 3 {
		t.Errorf("leaving: plan = (%+v, %+v, %d, %v), want ((2659,3437,0), (-2,2), 3, true)", from, delta, dir, ok)
	}
	if _, _, _, ok := diagonalPlan(game.Location{X: 2655, Y: 3439, Z: 0}, gate, 0); ok {
		t.Error("player two tiles from the corner: want not carried")
	}
}

// TestStepDestination pins stepThroughGate's ordinary destination
// (Doors.plugin.js:1975-1998): the tile just across the crossed
// boundary, on the gate's row or column.
func TestStepDestination(t *testing.T) {
	xGate := &PassThroughGate{CrossX: 3264, HasCrossX: true}
	if got := stepDestination(game.Location{X: 3263, Y: 3405, Z: 0}, xGate, 3264, 3405, 0); got != (game.Location{X: 3264, Y: 3405, Z: 0}) {
		t.Errorf("west of crossX: destination = %+v, want (3264,3405,0)", got)
	}
	if got := stepDestination(game.Location{X: 3264, Y: 3405, Z: 0}, xGate, 3264, 3405, 0); got != (game.Location{X: 3263, Y: 3405, Z: 0}) {
		t.Errorf("on crossX: destination = %+v, want (3263,3405,0)", got)
	}
	yGate := &PassThroughGate{CrossY: 3543, HasCrossY: true}
	if got := stepDestination(game.Location{X: 2906, Y: 3542, Z: 1}, yGate, 2906, 3543, 1); got != (game.Location{X: 2906, Y: 3543, Z: 1}) {
		t.Errorf("north of crossY: destination = %+v, want (2906,3543,1)", got)
	}
	if got := stepDestination(game.Location{X: 2906, Y: 3544, Z: 1}, yGate, 2906, 3543, 1); got != (game.Location{X: 2906, Y: 3542, Z: 1}) {
		t.Errorf("south of crossY: destination = %+v, want (2906,3542,1)", got)
	}
}

// TestPassThroughGateCarry drives the Troll Stronghold prison door
// (carry gate) through the engine: opening swaps the closed leaf for
// the open leaf + inviswall with the gate's own sound, carries the
// opener one tile across, and shuts 2 ticks later.
func TestPassThroughGateCarry(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, &Catalog{})
	p, sess := newTestPlayer("alice", 2847, 10107, 1)
	loc := game.Location{X: 2848, Y: 10107, Z: 1}
	if !e.toggle(clickEvent(p, 3780, loc, 0, 0, doorDef(3780, "Prison door"), 1)) {
		t.Fatal("Open prison door 3780: want handled")
	}
	if !mgr.hasSwap("deregister", 3780, loc, 0) {
		t.Fatalf("swaps = %+v, want deregister 3780", mgr.calls)
	}
	if !mgr.hasSwap("register", 3781, game.Location{X: 2847, Y: 10107, Z: 1}, 1) {
		t.Fatalf("swaps = %+v, want open leaf 3781 at (2847,10107,1) face 1", mgr.calls)
	}
	if !mgr.hasSwap("register", namelessInviswall, loc, 0) {
		t.Fatalf("swaps = %+v, want INVISWALL on the doorway", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 62 {
		t.Fatalf("sound effects = %+v, want one gate sound (62)", sess.effects)
	}
	runTicks(t, tasks, 1)
	if p.Location() != (game.Location{X: 2848, Y: 10107, Z: 1}) {
		t.Fatalf("after 1 tick: player = %+v, want carried across to (2848,10107,1)", p.Location())
	}
	runTicks(t, tasks, 1)
	if !mgr.hasSwap("register", 3780, loc, 0) {
		t.Fatalf("after 2 ticks: swaps = %+v, want the gate shut again", mgr.calls)
	}
	if len(e.passThroughOpen) != 0 {
		t.Fatalf("pass-through open set = %d, want 0 after the close timer", len(e.passThroughOpen))
	}
}

// TestPassThroughGateWalkOver drives Harold's door (walkOver gate)
// through the engine: opening swaps in the inviswall + open leaf,
// the opener's one step ignores clipping, the flag clears within 2
// ticks, and the gate shuts 2 ticks later.
func TestPassThroughGateWalkOver(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, &Catalog{})
	p, sess := newTestPlayer("alice", 2906, 3544, 1)
	loc := game.Location{X: 2906, Y: 3543, Z: 1}
	if !e.toggle(clickEvent(p, 3747, loc, 0, 3, doorDef(3747, "Door"), 1)) {
		t.Fatal("Open Harold's door 3747: want handled")
	}
	if !mgr.hasSwap("register", namelessHaroldDoorOpen, game.Location{X: 2906, Y: 3542, Z: 1}, 0) {
		t.Fatalf("swaps = %+v, want open leaf at (2906,3542,1) face 0", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 81 {
		t.Fatalf("sound effects = %+v, want one gate sound (81)", sess.effects)
	}
	if len(e.walkOvers) != 1 {
		t.Fatalf("walkOvers = %d, want 1 crossing in flight", len(e.walkOvers))
	}
	if q := p.MovementQueue(); q == nil || q.Size() != 1 {
		t.Fatalf("movement queue size = %d, want 1 checkpoint across", queueSize(p))
	}
	runTicks(t, tasks, 2)
	if len(e.walkOvers) != 0 {
		t.Fatalf("walkOvers after 2 ticks = %d, want 0 (flag cleared)", len(e.walkOvers))
	}
	if !mgr.hasSwap("register", 3747, loc, 3) {
		t.Fatalf("after 2 ticks: swaps = %+v, want the gate shut again", mgr.calls)
	}
}

func queueSize(p *game.Player) int {
	if q := p.MovementQueue(); q != nil {
		return q.Size()
	}
	return -1
}

// TestPassThroughGateOpenWhileOpen steps a second player through a
// gate that is already open, without re-swinging it.
func TestPassThroughGateOpenWhileOpen(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, &Catalog{})
	p1, _ := newTestPlayer("alice", 2847, 10107, 1)
	loc := game.Location{X: 2848, Y: 10107, Z: 1}
	if !e.toggle(clickEvent(p1, 3780, loc, 0, 0, doorDef(3780, "Prison door"), 1)) {
		t.Fatal("Open prison door 3780: want handled")
	}
	calls := len(mgr.calls)
	p2, _ := newTestPlayer("bob", 2847, 10107, 1)
	if !e.toggle(clickEvent(p2, 3780, loc, 0, 0, doorDef(3780, "Prison door"), 1)) {
		t.Fatal("Second player at an open gate: want handled")
	}
	if len(mgr.calls) != calls {
		t.Fatalf("swaps after second click = %+v, want no re-swing", mgr.calls)
	}
	runTicks(t, tasks, 1)
	if p2.Location() != (game.Location{X: 2848, Y: 10107, Z: 1}) {
		t.Fatalf("after 1 tick: second player = %+v, want carried across", p2.Location())
	}
}

// TestPassThroughWestArdougneCloseTicks pins the West Ardougne wall
// gate's 5-tick close timer (Doors.plugin.js: the 5-tick entry in
// PASS_THROUGH_GATES).
func TestPassThroughWestArdougneCloseTicks(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, &Catalog{})
	p, _ := newTestPlayer("alice", 2556, 3299, 0)
	loc := game.Location{X: 2557, Y: 3299, Z: 0}
	if !e.toggle(clickEvent(p, 8739, loc, 10, 2, doorDef(8739, "Gate"), 1)) {
		t.Fatal("Open West Ardougne gate 8739: want handled")
	}
	if !mgr.hasSwap("deregister", 8739, loc, 2) {
		t.Fatalf("swaps = %+v, want deregister 8739", mgr.calls)
	}
	runTicks(t, tasks, 4)
	if mgr.hasSwap("register", 8739, loc, 2) {
		t.Fatal("after 4 ticks: gate shut early (want the 5-tick timer)")
	}
	runTicks(t, tasks, 1)
	if !mgr.hasSwap("register", 8739, loc, 2) {
		t.Fatalf("after 5 ticks: swaps = %+v, want the gate shut again", mgr.calls)
	}
}
