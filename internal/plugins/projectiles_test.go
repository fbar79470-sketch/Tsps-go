package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

func TestCoreProjectileSurface(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "p").Core()
	cls := core.Projectile()
	if cls.LegacyMagicEndBase() != 46 {
		t.Errorf("LegacyMagicEndBase = %d, want 46", cls.LegacyMagicEndBase())
	}
	origin := game.Location{X: 100, Y: 100}
	victim := game.NewNPC(1, game.Location{X: 110, Y: 100})
	victim.SetIndex(7)
	source := game.NewPlayer("shooter", origin)
	p := cls.Create(source, victim, 42, 0, 20, 43, 31)
	if p.ProjectileID() != 42 || p.LockonTargetIndex() != 8 {
		t.Errorf("Create = id %d lockon %d, want 42/8", p.ProjectileID(), p.LockonTargetIndex())
	}
	if p.Start() != origin || p.End() != (game.Location{X: 110, Y: 100}) {
		t.Errorf("Create start/end = %v/%v", p.Start(), p.End())
	}
	raw := cls.New(origin, game.Location{X: 110, Y: 100}, nil, 7, 5, 60, 10, 20, nil)
	if raw.ProjectileID() != 7 || raw.LockonTargetIndex() != 0 {
		t.Errorf("New = id %d lockon %d, want 7/0", raw.ProjectileID(), raw.LockonTargetIndex())
	}
	if got := cls.CentreOf(victim); got != (game.Location{X: 110, Y: 100}) {
		t.Errorf("CentreOf = %v", got)
	}
	if got := cls.FlightDistance(origin, game.Location{X: 110, Y: 100}, nil); got != 10 {
		t.Errorf("FlightDistance = %d, want 10", got)
	}
	if got := cls.ArrivalCycles(origin, game.Location{X: 110, Y: 100}, 40, 10); got != 140 {
		t.Errorf("ArrivalCycles = %d, want 140", got)
	}
	if got := cls.ArrivalTicks(origin, game.Location{X: 110, Y: 100}, 40, 10); got != 5 {
		t.Errorf("ArrivalTicks = %d, want 5", got)
	}
	if got := cls.TicksToLand(31); got != 2 {
		t.Errorf("TicksToLand = %d, want 2", got)
	}
	// Safe on a nil Core.
	var nilCore *Core
	if nilCore.Projectile().LegacyMagicEndBase() != 46 {
		t.Error("nil Core Projectile() failed")
	}
}
