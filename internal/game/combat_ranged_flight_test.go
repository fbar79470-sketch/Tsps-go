package game

import "testing"

// Projectile-flight divergence tests (phase 26): the duplicate
// flight methods in combat_ranged.go against TS Projectile
// (Projectile.ts:82-89 arrivalCycles centres both ends,
// 112-120 flightDistance: west = end.X - offset, east edge =
// west + size - 1, size clamped to >= 1). Every expected value
// below is worked by hand from that formula — never from the old
// Go code, which used west + offset*2 for the east edge and
// measured from the attacker's south-west tile instead of its
// centre. Attackers are size-1 players unless a sized NPC is
// built; targets are NPCs with the fake definition's size set.
//
// Worked examples (attacker east of the target exposes the east
// edge; attacker west exposes the west edge, which never
// diverged):
//
//   - size 2, SW (10,0), attacker (13,0): offset 0, west 10,
//     east 11, dx = 13-11 = 2 (old Go: east 10, dx 3).
//   - size 4, SW (10,0), attacker (14,0): offset 1, west 9,
//     east 12, dx = 14-12 = 2 (old Go: east 11, dx 3).
//   - size 4, SW (10,0), attacker (0,0): west 9, dx = 9-0 = 9
//     (west edge: old Go agreed).
//   - size 3, SW (10,0), attacker (13,0): offset 1, west 9,
//     east 11, dx = 13-11 = 2 (odd sizes agreed).
//   - size 6, SW (10,0), attacker (15,0): offset 2, west 8,
//     east 13, dx = 15-13 = 2 (old Go: east 12, dx 3).
//   - size 2, SW (0,10), attacker (0,13): north edge, south 10,
//     north 11, dy = 13-11 = 2 (old Go: north 10, dy 3).
//   - size 3 attacker NPC, SW (0,0), size-1 target (10,0):
//     attacker centre (1,0), dx = 10-1 = 9 (old Go from the
//     south-west tile: 10).
//   - size 4 target SW (10,0), attacker (11,0): the centred
//     start (11,0) lies inside the footprint [9..12], dx/dy 0.

func sizedTestNPC(t *testing.T, id, size, x, y int) *NPC {
	t.Helper()
	def := newCombatFakeDef()
	def.size = size
	return combatTestNPC(t, id, x, y, def)
}

func TestProjectileFlightDistanceEvenSizes(t *testing.T) {
	run := func(name string, ax, ay, size, tx, ty, want int) {
		t.Run(name, func(t *testing.T) {
			a, _ := combatTestPlayer(t, "a", ax, ay)
			target := sizedTestNPC(t, 1, size, tx, ty)
			if got := projectileFlightDistance(&a.Mobile, &target.Mobile); got != want {
				t.Fatalf("flightDistance = %d, want %d", got, want)
			}
		})
	}
	run("size2-east", 13, 0, 2, 10, 0, 2)
	run("size4-east", 14, 0, 4, 10, 0, 2)
	run("size4-west", 0, 0, 4, 10, 0, 9)
	run("size3-east", 13, 0, 3, 10, 0, 2)
	run("size6-east", 15, 0, 6, 10, 0, 2)
	run("size2-north", 0, 13, 2, 0, 10, 2)
	run("size1-baseline", 0, 0, 1, 10, 0, 10)
	run("inside-footprint", 11, 0, 4, 10, 0, 0)

	// Attacker centring: a size-3 NPC attacker fires from its
	// centre (1,0), not its south-west tile (0,0).
	t.Run("size3-attacker-centre", func(t *testing.T) {
		npcAttacker := sizedTestNPC(t, 1, 3, 0, 0)
		target, _ := combatTestPlayer(t, "t", 10, 0)
		if got := projectileFlightDistance(&npcAttacker.Mobile, &target.Mobile); got != 9 {
			t.Fatalf("flightDistance = %d, want 9", got)
		}
	})

	// Nil ends read 0.
	if got := projectileFlightDistance(nil, nil); got != 0 {
		t.Fatalf("nil flightDistance = %d, want 0", got)
	}
}

func TestProjectileArrivalTicksEvenSizes(t *testing.T) {
	// The ranged NPC hit delay (46 + 5 cycles a tile, ceil / 30)
	// over the fixed flight distance: size-2 target east of the
	// attacker flies 2 tiles, so 46 + 10 = 56 cycles -> 2 ticks
	// (the old flight distance of 3 gave 61 -> 3 ticks).
	a, _ := combatTestPlayer(t, "a", 13, 0)
	target := sizedTestNPC(t, 1, 2, 10, 0)
	if got := projectileArrivalTicks(&a.Mobile, &target.Mobile, 46, 5); got != 2 {
		t.Fatalf("arrivalTicks = %d, want 2", got)
	}

	// The duplicate and the canonical FlightDistance now agree
	// on the same geometry: centred start (13,0), centred end
	// (10,0) for the size-2 target, footprint [10..11], dx 2.
	if got := FlightDistance(CentreOf(a), CentreOf(target), target); got != 2 {
		t.Fatalf("canonical FlightDistance = %d, want 2", got)
	}
}
