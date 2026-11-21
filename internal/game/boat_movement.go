package game

// Per-tick boat movement, ported from
// game/content/sailing/BoatMovement.ts, as live (rsprox, Sailing
// recordings 856, 907, 908, 1030-1032, 3457, 3461, 3578, 3579;
// docs/sailing.md):
//
//   - the target speed: full sail the hull's base speed (wooden
//     1.5 tiles a tick, teak 2, camphor 2.5), half sail half
//     that, reverse half a tile; a Barracuda Trial's rapids add
//     to it up to the hull's speed cap; a burst of speed (a
//     trim, a released wind mote) adds half a tile on top,
//     never past the cap and a half-tile step;
//   - the speed ramps toward the target by the sails'
//     acceleration each tick, up and down (half a tile, a tile
//     with camphor or better sails), so a boat coasts to a stop
//     when its sails come down; hitting rocks or land stops it
//     dead;
//   - turning: 128 angle units a tick, at no cost to speed,
//     moving along the new facing.
//
// The turn rate is also rsmod's (BoatMovement.kt, ISC license).
// Reverse keeps the bow on the helm heading and backs up.

const (
	// TurnRate is TURN_RATE: angle units turned per tick.
	TurnRate = 128
	// FullSailSpeed is FULL_SAIL_SPEED: a raft's (wooden hull)
	// full sail speed; each boat sails at its hull's base speed.
	FullSailSpeed = 192
	// HalfSailSpeed is HALF_SAIL_SPEED.
	HalfSailSpeed = FullSailSpeed / 2
	// ReverseSpeed is REVERSE_SPEED: half a tile a tick
	// whatever the hull (rsprox 3461, 908).
	ReverseSpeed = 64
	// BurstStep is BURST_STEP: a burst of speed's step past the
	// sails, and the most anything takes a boat past its cap.
	BurstStep = 64
	// moveStep is MOVE_STEP: a move is checked in steps of at
	// most half a tile, so a fast boat can't hop over land.
	moveStep = 64
)

// BoatStep is what one tick's move did (BoatMovement.ts:
// BoatStep).
type BoatStep struct {
	Moved  bool
	Turned bool
	// Blocked reports that the hull hit something it can't
	// sail over this tick.
	Blocked bool
}

// SailVelocity is the speed the sails drive the boat at, in fine
// units a tick along the bow (negative in reverse).
func SailVelocity(boat *Boat) int {
	target := TargetSpeed(boat)
	if boat.MoveMode == BoatMoveModeReverse {
		return -target
	}
	return target
}

// TickBoat moves one boat one tick: the velocity ramps toward
// the sails' target, the boat swings toward its heading (turning
// costs no speed, and a stopped boat turns on the spot), then it
// sails along the new facing in half-tile checked steps,
// stopping dead on rocks or land.
func TickBoat(boat *Boat, isSailable SailableTileCheck) BoatStep {
	step := BoatStep{}
	target := SailVelocity(boat)
	boat.Velocity += clampInt(target-boat.Velocity, -boat.Acceleration, boat.Acceleration)

	// Turning doesn't cost speed: the boat swings towards its
	// heading, then sails on at its speed along the new facing.
	// A stopped boat turns on the spot at the same rate (rsprox:
	// crew-combat, sails lowered at the helm, then 640 -> 1280
	// by 128 a tick without moving).
	delta := TurnAngleDelta(boat.Angle, boat.Heading)
	if delta != 0 {
		angle := NormalizeAngle(boat.Angle + clampInt(delta, -TurnRate, TurnRate))
		if CanOccupy(boat, boat.FineX, boat.FineY, angle, isSailable) {
			boat.Angle = angle
			step.Turned = true
		}
	}
	if boat.Velocity == 0 {
		return step
	}

	distance := float64(absInt(boat.Velocity)) * maxFloat(1, boat.SpeedMultiplier)
	travel := boat.Angle
	if boat.Velocity < 0 {
		travel = ReverseAngle(boat.Angle)
	}
	startX, startY := boat.FineX, boat.FineY
	for travelled := 0.0; travelled < distance; {
		travelled = minFloat(distance, travelled+moveStep)
		dx, dy := AngleToFineDelta(travel, travelled)
		x, y := startX+dx, startY+dy
		if !CanOccupy(boat, x, y, boat.Angle, isSailable) {
			// Run aground (rocks, land): the way is lost
			// (rsprox 3578).
			step.Blocked = true
			boat.Velocity = 0
			break
		}
		applyBoatPosition(boat, x, y, &step)
	}
	return step
}

// sailSpeed is what the sails give: full the base speed, half
// half of it, reverse half a tile, none when stopped.
func sailSpeed(boat *Boat) int {
	switch boat.MoveMode {
	case BoatMoveModeFull:
		return boat.BaseSpeed
	case BoatMoveModeHalf:
		return boat.BaseSpeed / 2
	case BoatMoveModeReverse:
		return ReverseSpeed
	default:
		return 0
	}
}

func sumInts(values map[string]int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

// TargetSpeed is the speed the boat is heading for: the sails,
// plus what pushes them under the hull's cap (a trial's rapids;
// a lightning strike takes off), plus a burst past it (at most a
// step past the cap), held under any limit (a fetid pool, an
// anchor), backing up too.
func TargetSpeed(boat *Boat) int {
	sails := sailSpeed(boat)
	if sails <= 0 {
		return 0
	}
	if boat.MoveMode == BoatMoveModeReverse {
		target := sails
		for _, limit := range boat.SpeedLimits {
			target = min(target, limit)
		}
		return max(0, target)
	}
	target := max(0, min(sails+sumInts(boat.SpeedBonuses), boat.SpeedCap))
	burst := sumInts(boat.BurstBonuses)
	if boat.BoostSpeed != nil {
		burst += *boat.BoostSpeed
	}
	if burst > 0 {
		target = min(target+burst, boat.SpeedCap+BurstStep)
	}
	for _, limit := range boat.SpeedLimits {
		target = min(target, limit)
	}
	return max(0, target)
}

func applyBoatPosition(boat *Boat, x, y int, step *BoatStep) {
	if x == boat.FineX && y == boat.FineY {
		return
	}
	boat.FineX = x
	boat.FineY = y
	step.Moved = true
}

func clampInt(v, lo, hi int) int {
	return min(max(v, lo), hi)
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
