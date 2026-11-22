package main

import (
	"testing"

	"tsps-go/internal/content/ladders"
	"tsps-go/internal/game"
)

// installLaddersMechanism must install a mechanism on the Ladders
// package even with nil defs (NoCache mode): the Defs adapter reads
// empty, every climb falls through, and nothing panics. The
// real-cache path (defs resolve, climbs land) is proven by the
// smoke49 live legs, not here — cmd/server tests have no cache.
func TestInstallLaddersMechanismNilDefs(t *testing.T) {
	previous := ladders.Mechanism()
	defer ladders.SetMechanism(previous)

	world := game.NewWorld(game.Location{X: 3222, Y: 3219, Z: 0})
	installLaddersMechanism(world, nil)
	m := ladders.Mechanism()
	if m == nil {
		t.Fatal("installLaddersMechanism with nil defs must install a mechanism")
	}
	_, ok := (cacheLadderDefs{defs: nil}).Def(14880)
	if ok {
		t.Fatal("nil-defs adapter must not resolve any id")
	}
	// The resolver behind the installed mechanism reads empty defs,
	// so a climb against a real-looking object still falls through
	// instead of panicking.
	mech, ok := m.(*ladders.ClimbMechanism)
	if !ok || mech.Resolver() == nil {
		t.Fatal("installed mechanism must carry a resolver")
	}
}

func TestInstallLaddersMechanismNilWorld(t *testing.T) {
	// Must not panic; leaves the installed mechanism alone.
	installLaddersMechanism(nil, nil)
}
