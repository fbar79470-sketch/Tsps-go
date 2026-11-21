package game

import "testing"

func TestLocationRegionFormulas(t *testing.T) {
	// Location.ts: getRegionX = (x >> 3) - 6; getRegionY = (y >> 3) - 6.
	// Spawn (3089, 3524, 0) from world.json (ANALYSIS Appendix B).
	spawn := Location{X: 3089, Y: 3524, Z: 0}
	if got := spawn.RegionX(); got != 380 {
		t.Fatalf("RegionX = %d, want 380", got)
	}
	if got := spawn.RegionY(); got != 434 {
		t.Fatalf("RegionY = %d, want 434", got)
	}
	// Region formula is NOT x>>6: 3089>>6 = 48, which must differ.
	if spawn.RegionX() == 3089>>6 {
		t.Fatal("RegionX must not equal x>>6")
	}
	origin := Location{}
	if got := origin.RegionX(); got != -6 {
		t.Fatalf("origin RegionX = %d, want -6", got)
	}
}

func TestLocationLocalAndDistance(t *testing.T) {
	a := Location{X: 100, Y: 100, Z: 0}
	b := Location{X: 103, Y: 101, Z: 0}
	// LocalX = x - 8*regionX(position); regionX(100) = 12-6 = 6.
	if got := b.LocalX(a); got != 103-48 {
		t.Fatalf("LocalX = %d, want %d", got, 103-48)
	}
	if got := a.Distance(b); got != 3 {
		t.Fatalf("Distance = %d, want 3 (Chebyshev)", got)
	}
	if !a.IsWithinDistance(b, 3) || a.IsWithinDistance(b, 2) {
		t.Fatal("IsWithinDistance box test wrong")
	}
	otherLevel := Location{X: 100, Y: 100, Z: 1}
	if a.Distance(otherLevel) != -1 {
		t.Fatal("Distance across levels must be -1 sentinel")
	}
	if a.IsWithinDistance(otherLevel, 100) {
		t.Fatal("IsWithinDistance across levels must be false")
	}
	if !a.IsViewableFrom(Location{X: 115, Y: 85, Z: 0}) {
		t.Fatal("15-tile view distance must include (115, 85)")
	}
	if a.IsViewableFrom(Location{X: 116, Y: 100, Z: 0}) {
		t.Fatal("16 tiles away must not be viewable")
	}
	if !a.Equals(Location{X: 100, Y: 100, Z: 0}) || a.Equals(b) {
		t.Fatal("Equals wrong")
	}
	if a.String() != "[100, 100, 0]" {
		t.Fatalf("String = %q", a.String())
	}
}

func TestFlagsBitset(t *testing.T) {
	var f Flags
	if f.IsUpdateRequired() {
		t.Fatal("empty flags must not require update")
	}
	f = f.Set(FlagAppearance).Set(FlagChat)
	if !f.Has(FlagAppearance) || !f.Has(FlagChat) || f.Has(FlagHit) {
		t.Fatal("Has/Set wrong")
	}
	if !f.IsUpdateRequired() {
		t.Fatal("non-empty flags must require update")
	}
	if got := f.String(); got != "{CHAT,APPEARANCE}" {
		t.Fatalf("String = %q, want declaration order", got)
	}
	f = f.Clear(FlagChat)
	if f.Has(FlagChat) || !f.Has(FlagAppearance) {
		t.Fatal("Clear wrong")
	}
	if f.Reset() != 0 || f.Reset().String() != "{}" {
		t.Fatal("Reset wrong")
	}
	// Flag numeric values must match Flag.ts declaration order.
	if FlagChat != 0 || FlagAppearance != 5 || FlagHit != 8 {
		t.Fatalf("flag values: chat=%d appearance=%d hit=%d", FlagChat, FlagAppearance, FlagHit)
	}
}

func TestDirectionTable(t *testing.T) {
	if DirNorth.Dx() != 0 || DirNorth.Dy() != 1 || DirNorth.Opposite() != DirSouth {
		t.Fatal("NORTH row wrong")
	}
	if DirNorthEast.Dx() != 1 || DirNorthEast.Dy() != 1 || !DirNorthEast.IsDiagonal() {
		t.Fatal("NORTH_EAST row wrong")
	}
	if DirEast.IsDiagonal() || DirNone.Dx() != 0 || DirNone.Opposite() != DirNone {
		t.Fatal("EAST/NONE rows wrong")
	}
	if DirectionFromID(1) != DirNorth || DirectionFromID(99) != DirNone {
		t.Fatal("DirectionFromID wrong")
	}
	if DirSouthWest.String() != "SOUTH_WEST" {
		t.Fatalf("String = %q", DirSouthWest.String())
	}
}

func TestMobileDefaultsAndReset(t *testing.T) {
	loc := Location{X: 10, Y: 20, Z: 1}
	m := NewMobile(loc)
	if m.Facing != DirNone || !m.LastKnownRegion.Equals(loc) {
		t.Fatal("NewMobile must start facing NONE with region = spawn")
	}
	if m.IsRegistered() {
		t.Fatal("must start unregistered")
	}
	m.Flags = m.Flags.Set(FlagAnimation)
	m.WalkingDirection = DirEast
	m.NeedsPlacement = true
	m.Facing = DirEast
	m.ResetUpdating()
	if m.Flags.IsUpdateRequired() || m.WalkingDirection != DirNone || m.NeedsPlacement {
		t.Fatal("ResetUpdating must clear flags, steps, placement")
	}
	if m.Facing != DirEast {
		t.Fatal("ResetUpdating must NOT clear facing (Mobile.ts:81-85)")
	}
	m.GetTimers().Registers(TimerKeyFood, 2)
	m.ProcessTimers(7)
	if m.GetTimers().Left(TimerKeyFood) != 1 {
		t.Fatal("ProcessTimers must tick the timer repository")
	}
}
