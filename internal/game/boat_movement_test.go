package game

import "testing"

func allSailable(x, y, level int) bool { return true }

func TestTargetSpeed(t *testing.T) {
	boat := testBoat()
	// Stopped: no speed whatever the bonuses.
	boat.MoveMode = BoatMoveModeStopped
	boat.SpeedBonuses["rapids"] = 100
	if got := TargetSpeed(boat); got != 0 {
		t.Errorf("stopped target = %d, want 0", got)
	}
	// Full sail: the base speed.
	boat.MoveMode = BoatMoveModeFull
	boat.SpeedBonuses = map[string]int{}
	if got := TargetSpeed(boat); got != 192 {
		t.Errorf("full target = %d, want 192", got)
	}
	// Half sail: half.
	boat.MoveMode = BoatMoveModeHalf
	if got := TargetSpeed(boat); got != 96 {
		t.Errorf("half target = %d, want 96", got)
	}
	// Reverse: half a tile, whatever the hull.
	boat.MoveMode = BoatMoveModeReverse
	if got := TargetSpeed(boat); got != 64 {
		t.Errorf("reverse target = %d, want 64", got)
	}
	// Rapids add up to the cap; a burst goes a step past it.
	boat.MoveMode = BoatMoveModeFull
	boat.SpeedBonuses["rapids"] = 100
	boat.BurstBonuses["portal"] = 100
	if got := TargetSpeed(boat); got != 192+64 {
		t.Errorf("rapids+burst target = %d, want 256", got)
	}
	// A limit holds it under, backing up too.
	boat.SpeedBonuses = map[string]int{}
	boat.BurstBonuses = map[string]int{}
	boat.SpeedLimits["pool"] = 50
	if got := TargetSpeed(boat); got != 50 {
		t.Errorf("limited target = %d, want 50", got)
	}
	boat.MoveMode = BoatMoveModeReverse
	if got := TargetSpeed(boat); got != 50 {
		t.Errorf("limited reverse target = %d, want 50", got)
	}
	// A negative bonus (a lightning strike) takes speed off.
	boat.MoveMode = BoatMoveModeFull
	boat.SpeedLimits = map[string]int{}
	boat.SpeedBonuses["strike"] = -100
	if got := TargetSpeed(boat); got != 92 {
		t.Errorf("struck target = %d, want 92", got)
	}
}

func TestSailVelocitySign(t *testing.T) {
	boat := testBoat()
	boat.MoveMode = BoatMoveModeFull
	if got := SailVelocity(boat); got != 192 {
		t.Errorf("full velocity = %d, want 192", got)
	}
	boat.MoveMode = BoatMoveModeReverse
	if got := SailVelocity(boat); got != -64 {
		t.Errorf("reverse velocity = %d, want -64", got)
	}
}

func TestTickBoatRampsAndSails(t *testing.T) {
	boat := testBoat()
	boat.MoveMode = BoatMoveModeFull
	startY := boat.FineY
	// Tick 1: velocity ramps 0 -> 64 (acceleration), sails
	// south 64 fine units along angle 0.
	step := TickBoat(boat, allSailable)
	if boat.Velocity != 64 {
		t.Errorf("velocity after 1 tick = %d, want 64", boat.Velocity)
	}
	if !step.Moved || step.Turned || step.Blocked {
		t.Errorf("step = %+v, want moved only", step)
	}
	if boat.FineY != startY-64 || boat.FineX != 3000*FineUnitsPerTile {
		t.Errorf("position = (%d, %d), want (384000, %d)", boat.FineX, boat.FineY, startY-64)
	}
	// Tick 2: velocity 128; tick 3: 192 (full speed).
	TickBoat(boat, allSailable)
	if boat.Velocity != 128 {
		t.Errorf("velocity after 2 ticks = %d, want 128", boat.Velocity)
	}
	TickBoat(boat, allSailable)
	if boat.Velocity != 192 {
		t.Errorf("velocity after 3 ticks = %d, want 192", boat.Velocity)
	}
	// Sails down: coasts to a stop by 64 a tick.
	boat.MoveMode = BoatMoveModeStopped
	TickBoat(boat, allSailable)
	if boat.Velocity != 128 {
		t.Errorf("coasting velocity = %d, want 128", boat.Velocity)
	}
}

func TestTickBoatTurnsOnTheSpot(t *testing.T) {
	boat := testBoat()
	boat.Heading = 512
	step := TickBoat(boat, allSailable)
	if boat.Angle != 128 {
		t.Errorf("angle = %d, want 128 (one turn step)", boat.Angle)
	}
	if !step.Turned || step.Moved {
		t.Errorf("step = %+v, want turned only", step)
	}
	// A blocked turn (no sailable water at the new angle)
	// leaves the angle alone.
	boat2 := testBoat()
	boat2.Heading = 512
	step = TickBoat(boat2, func(x, y, level int) bool { return false })
	if boat2.Angle != 0 || step.Turned {
		t.Errorf("blocked turn: angle = %d, step = %+v, want angle 0 unturned", boat2.Angle, step)
	}
}

func TestTickBoatRunsAground(t *testing.T) {
	boat := testBoat()
	boat.MoveMode = BoatMoveModeFull
	boat.Velocity = 192
	// Water everywhere except the tile the hull newly covers
	// on its first half-tile step south (2999, 2995: the
	// hull trails south of the boat's position).
	blocked := func(x, y, level int) bool {
		return !(x == 2999 && y == 2995)
	}
	startY := boat.FineY
	step := TickBoat(boat, blocked)
	if !step.Blocked || boat.Velocity != 0 {
		t.Errorf("aground: step = %+v, velocity = %d, want blocked with velocity 0", step, boat.Velocity)
	}
	if boat.FineY != startY {
		t.Errorf("aground moved to fineY %d, want no move from %d", boat.FineY, startY)
	}
}

func TestTickBoatReverses(t *testing.T) {
	boat := testBoat()
	boat.MoveMode = BoatMoveModeReverse
	startY := boat.FineY
	TickBoat(boat, allSailable)
	// Backing up: the bow stays on angle 0, the boat moves
	// north (opposite the facing).
	if boat.FineY != startY+64 {
		t.Errorf("reversing fineY = %d, want %d", boat.FineY, startY+64)
	}
	if boat.Angle != 0 {
		t.Errorf("reversing angle = %d, want 0 (bow on heading)", boat.Angle)
	}
}

func TestTickBoatSpeedMultiplier(t *testing.T) {
	boat := testBoat()
	boat.MoveMode = BoatMoveModeFull
	boat.Velocity = 192
	boat.SpeedMultiplier = 2
	startY := boat.FineY
	TickBoat(boat, allSailable)
	if boat.FineY != startY-384 {
		t.Errorf("doubled fineY = %d, want %d", boat.FineY, startY-384)
	}
	// A multiplier below 1 still sails the full distance.
	boat2 := testBoat()
	boat2.MoveMode = BoatMoveModeFull
	boat2.Velocity = 192
	boat2.SpeedMultiplier = 0.5
	startY = boat2.FineY
	TickBoat(boat2, allSailable)
	if boat2.FineY != startY-192 {
		t.Errorf("halved-multiplier fineY = %d, want %d", boat2.FineY, startY-192)
	}
}
