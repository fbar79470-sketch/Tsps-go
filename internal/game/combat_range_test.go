package game

import "testing"

func TestCanReachMelee(t *testing.T) {
	setCombatCycle(t, 100)
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	// Orthogonal adjacency: shared open edge.
	b, _ := combatTestPlayer(t, "b", 3001, 3000)
	if !CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&b.Mobile)) {
		t.Errorf("orthogonally adjacent melee should reach")
	}
	// Diagonal adjacency: no shared edge, melee cannot reach.
	c, _ := combatTestPlayer(t, "c", 3001, 3001)
	if CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&c.Mobile)) {
		t.Errorf("diagonal melee should not reach")
	}
	// Two tiles away: out of melee range.
	d, _ := combatTestPlayer(t, "d", 3002, 3000)
	if CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&d.Mobile)) {
		t.Errorf("melee at distance 2 should not reach")
	}
	// Overlap: never reachable.
	e, _ := combatTestPlayer(t, "e", 3000, 3000)
	if CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&e.Mobile)) {
		t.Errorf("overlapping fighters should not reach")
	}
	// Cross-floor: never reachable.
	f, _ := combatTestPlayer(t, "f", 3001, 3000)
	f.SetLocation(Location{X: 3001, Y: 3000, Z: 1})
	if CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&f.Mobile)) {
		t.Errorf("cross-floor melee should not reach")
	}
}

func TestCanReachRangedProjectileLine(t *testing.T) {
	setCombatCycle(t, 100)
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	b, _ := combatTestPlayer(t, "b", 3005, 3000)
	if !CanReach(combatantFor(&a.Mobile), RangedMethod, combatantFor(&b.Mobile)) {
		t.Errorf("open ground ranged should reach at distance 5")
	}
	// A projectile-blocking tile between them breaks the line.
	SetCombatClippingSource(func(x, y, z int) int32 {
		if x == 3002 && y == 3000 {
			return 0x20000 // TILE_BLOCKED
		}
		return 0
	})
	t.Cleanup(func() { SetCombatClippingSource(nil) })
	if CanReach(combatantFor(&a.Mobile), RangedMethod, combatantFor(&b.Mobile)) {
		t.Errorf("blocked projectile line should not reach")
	}
}

func TestFootprintAndEdgeDistance(t *testing.T) {
	// Size-1 at origin, point 3 east: footprint distance 3.
	if got := FootprintDistance(Location{X: 3000, Y: 3000}, 1, Location{X: 3003, Y: 3000}, false); got != 3 {
		t.Errorf("footprint distance = %d, want 3", got)
	}
	// Size-3 occupies 3000..3002: point at 3004 is distance 2.
	if got := FootprintDistance(Location{X: 3000, Y: 3000}, 3, Location{X: 3004, Y: 3000}, false); got != 2 {
		t.Errorf("size-3 footprint distance = %d, want 2", got)
	}
	// Inside the footprint: 0.
	if got := FootprintDistance(Location{X: 3000, Y: 3000}, 3, Location{X: 3001, Y: 3001}, false); got != 0 {
		t.Errorf("inside footprint = %d, want 0", got)
	}
	// Edge distance (Chebyshev to nearest footprint tile).
	g, _ := combatTestPlayer(t, "g", 3000, 3000)
	h, _ := combatTestPlayer(t, "h", 3002, 3001)
	if got := EdgeDistance(&g.Mobile, &h.Mobile); got != 2 {
		t.Errorf("edge distance = %d, want 2", got)
	}
}

func TestAttackDistanceHook(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	b, _ := combatTestPlayer(t, "b", 3002, 3000)
	if CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&b.Mobile)) {
		t.Fatalf("melee at distance 2 should not reach by default")
	}
	Hooks.OnCombatAttackDistance = func(e *CombatAttackDistanceEvent) { e.Distance = 2 }
	if !CanReach(combatantFor(&a.Mobile), MeleeMethod, combatantFor(&b.Mobile)) {
		t.Errorf("attack-distance hook should extend melee reach to 2")
	}
}

func TestCombatRouteDestination(t *testing.T) {
	attacker, _ := combatTestPlayer(t, "a", 3000, 3000)
	target, _ := combatTestPlayer(t, "b", 3002, 3001)
	// Melee: the nearest tile sharing an open edge — (3001,3001),
	// one step from the attacker, orthogonally beside the target.
	dest := combatRouteDestination(combatantFor(&attacker.Mobile), combatantFor(&target.Mobile), 1)
	if dest != (Location{X: 3001, Y: 3001, Z: 0}) {
		t.Errorf("melee destination = %v, want [3001, 3001, 0]", dest)
	}
	// Ranged: the approach-axis tile at attack distance.
	far, _ := combatTestPlayer(t, "c", 3012, 3000)
	dest = combatRouteDestination(combatantFor(&attacker.Mobile), combatantFor(&far.Mobile), 6)
	if dest != (Location{X: 3006, Y: 3000, Z: 0}) {
		t.Errorf("ranged destination = %v, want [3006, 3000, 0]", dest)
	}
}

func TestBesideAndOverlap(t *testing.T) {
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	b, _ := combatTestPlayer(t, "b", 3001, 3001)
	if !Beside(combatantFor(&a.Mobile), combatantFor(&b.Mobile)) {
		t.Errorf("diagonal neighbours are beside each other")
	}
	c, _ := combatTestPlayer(t, "c", 3000, 3000)
	if Beside(combatantFor(&a.Mobile), combatantFor(&c.Mobile)) {
		t.Errorf("overlapping fighters are not beside each other")
	}
	if !OverlapsEntities(combatantFor(&a.Mobile), combatantFor(&c.Mobile)) {
		t.Errorf("same tile should overlap")
	}
}
