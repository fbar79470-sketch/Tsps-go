package ladders

import (
	"testing"

	"tsps-go/internal/game"
)

// fakeDefinition is a plugins.ObjectDefinition fake carrying a name.
type fakeDefinition struct {
	name string
}

func (d fakeDefinition) ID() int                { return 0 }
func (d fakeDefinition) Name() string           { return d.name }
func (d fakeDefinition) SizeX() int             { return 1 }
func (d fakeDefinition) SizeY() int             { return 1 }
func (d fakeDefinition) BlockingMask() int      { return 0 }
func (d fakeDefinition) Interactions() []string { return nil }

func mechanismFixture(t *testing.T) (*ClimbMechanism, *game.TaskManager) {
	t.Helper()
	defs := fixtureDefs{
		100: def("Ladder", 1, 1, "Climb-up"),
		101: def("Ladder", 1, 1, "Climb-down"),
		400: def("Gangplank", 1, 1, "Cross"),
		401: def("Gangplank", 1, 1, "Cross"),
	}
	objects := &fixtureObjects{}
	objects.put(obj(100, 50, 50, 0, 10, 0))
	objects.put(obj(101, 50, 50, 1, 10, 0))
	objects.put(obj(400, 100, 100, 0, 10, 0))
	objects.put(obj(401, 100, 101, 1, 10, 0))
	tm := game.NewTaskManager()
	return NewMechanism(testResolver(defs, objects), tm), tm
}

func TestMechanismExplicitDestinationTrusted(t *testing.T) {
	m, tm := mechanismFixture(t)
	p := game.NewPlayer("climber", game.Location{X: 50, Y: 50, Z: 0})
	dest := game.Location{X: 3000, Y: 3000, Z: 2}
	arrived := false
	ok := m.Climb(ClimbRequest{
		Player: p, Delta: +1, Animate: true,
		Destination: &dest, OnArrival: func() { arrived = true },
	})
	if !ok {
		t.Fatal("Climb with explicit destination refused")
	}
	tm.Process(1) // animation tick
	if p.Location() != (game.Location{X: 50, Y: 50, Z: 0}) {
		t.Errorf("moved on tick 1, want animation only")
	}
	tm.Process(2) // move tick
	if p.Location() != dest || !arrived {
		t.Errorf("Location=%v arrived=%v after tick 2, want %v + arrival", p.Location(), arrived, dest)
	}
}

func TestMechanismMapDestination(t *testing.T) {
	m, tm := mechanismFixture(t)
	p := game.NewPlayer("climber", game.Location{X: 50, Y: 50, Z: 0})
	arrived := false
	ok := m.Climb(ClimbRequest{
		Player: p, Object: obj(100, 50, 50, 0, 10, 0), ObjectID: 100,
		Location: game.Location{X: 50, Y: 50, Z: 0}, Delta: +1, Animate: true,
		OnArrival: func() { arrived = true },
	})
	if !ok {
		t.Fatal("Climb with map destination refused")
	}
	tm.Process(1)
	tm.Process(2)
	if p.Location().Z != 1 || !arrived {
		t.Errorf("Location=%v arrived=%v, want plane 1 + arrival", p.Location(), arrived)
	}
}

func TestMechanismAnimateFalseMovesFirstTick(t *testing.T) {
	m, tm := mechanismFixture(t)
	p := game.NewPlayer("climber", game.Location{X: 50, Y: 50, Z: 0})
	dest := game.Location{X: 3000, Y: 3000, Z: 2}
	if !m.Climb(ClimbRequest{Player: p, Delta: +1, Animate: false, Destination: &dest}) {
		t.Fatal("Climb refused")
	}
	tm.Process(1)
	if p.Location() != dest {
		t.Errorf("Location=%v after tick 1, want %v (no animation delay)", p.Location(), dest)
	}
}

func TestMechanismFallbackAbove(t *testing.T) {
	// No other end on the map, but a Ladder sits right above the
	// clicked one: the fallback lands on the player's own tile one
	// plane up.
	defs := fixtureDefs{100: def("Ladder", 1, 1, "Climb-up")}
	objects := &fixtureObjects{}
	objects.put(obj(100, 50, 50, 0, 10, 0))
	objects.put(obj(100, 51, 50, 1, 10, 0))
	tm := game.NewTaskManager()
	m := NewMechanism(testResolver(defs, objects), tm)
	p := game.NewPlayer("climber", game.Location{X: 51, Y: 50, Z: 0})
	source := game.Location{X: 51, Y: 50, Z: 0}
	if !m.Climb(ClimbRequest{
		Player: p, Object: obj(100, 50, 50, 0, 10, 0), ObjectID: 100,
		Definition: fakeDefinition{name: "Ladder"},
		Location:   game.Location{X: 50, Y: 50, Z: 0}, SourceLocation: &source,
		Delta: +1, Animate: false,
	}) {
		t.Fatal("Climb with same-name object above refused")
	}
	tm.Process(1)
	if p.Location() != (game.Location{X: 51, Y: 50, Z: 1}) {
		t.Errorf("Location=%v, want (51,50,1)", p.Location())
	}
}

func TestMechanismNoDestinationRefuses(t *testing.T) {
	// A lone ladder: nothing at the other end, nothing above.
	defs := fixtureDefs{100: def("Ladder", 1, 1, "Climb-up")}
	objects := &fixtureObjects{}
	objects.put(obj(100, 50, 50, 0, 10, 0))
	tm := game.NewTaskManager()
	m := NewMechanism(testResolver(defs, objects), tm)
	p := game.NewPlayer("climber", game.Location{X: 50, Y: 50, Z: 0})
	if m.Climb(ClimbRequest{
		Player: p, Object: obj(100, 50, 50, 0, 10, 0), ObjectID: 100,
		Definition: fakeDefinition{name: "Ladder"},
		Location:   game.Location{X: 50, Y: 50, Z: 0}, Delta: +1, Animate: true,
	}) {
		t.Error("Climb with no other end started, want refusal")
	}
	if tm.Count() != 0 {
		t.Errorf("TaskManager count = %d, want 0", tm.Count())
	}
}

func TestMechanismCrossGangplank(t *testing.T) {
	m, tm := mechanismFixture(t)
	p := game.NewPlayer("climber", game.Location{X: 100, Y: 100, Z: 0})
	arrived := false
	ok := m.CrossGangplank(ClimbRequest{
		Player: p, Object: obj(400, 100, 100, 0, 10, 0), ObjectID: 400,
		Location: game.Location{X: 100, Y: 100, Z: 0}, OnArrival: func() { arrived = true },
	})
	if !ok {
		t.Fatal("CrossGangplank refused")
	}
	tm.Process(1) // no animation: the move lands on the first tick
	if p.Location() != (game.Location{X: 100, Y: 102, Z: 1}) || !arrived {
		t.Errorf("Location=%v arrived=%v, want (100,102,1) + arrival", p.Location(), arrived)
	}
}
