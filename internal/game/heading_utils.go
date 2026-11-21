package game

import "math"

// 2048-unit angle helpers for boat headings, ported from
// game/content/sailing/HeadingUtils.ts (ported from rsmod's
// HeadingUtils.kt, ISC license). Angle 0 faces south and angles
// increase clockwise when viewed from above: 512 = west,
// 1024 = north, 1536 = east.

const (
	// AngleUnits is ANGLE_UNITS (HeadingUtils.ts): the full circle.
	AngleUnits = 2048
	// PackedHeadingScale is PACKED_HEADING_SCALE: SET_HEADING sends
	// 0-15; each step is 128 angle units.
	PackedHeadingScale = 128
)

const unitsPerRadian = AngleUnits / (2 * math.Pi)

// NormalizeAngle ports normalizeAngle (HeadingUtils.ts): the angle
// truncated and wrapped into 0..2047.
func NormalizeAngle(angle int) int {
	return ((angle % AngleUnits) + AngleUnits) % AngleUnits
}

// ReverseAngle ports reverseAngle: the angle facing the opposite way.
func ReverseAngle(angle int) int {
	return NormalizeAngle(angle + 1024)
}

// PackedHeadingToAngle ports packedHeadingToAngle: a 0-15 helm
// heading to angle units.
func PackedHeadingToAngle(heading int) int {
	return NormalizeAngle(heading * PackedHeadingScale)
}

// AngleFromCoordDelta ports angleFromCoordDelta: a tile delta to an
// angle, keeping all 16 helm headings distinct. The zero delta maps
// to 0.
func AngleFromCoordDelta(dx, dy int) int {
	if dx == 0 && dy == 0 {
		return 0
	}
	return int(math.Round(math.Atan2(float64(-dx), float64(-dy))*unitsPerRadian)) & (AngleUnits - 1)
}

// TurnAngleDelta ports turnAngleDelta: the signed turn delta while
// under sail. It prefers the clockwise arc when it is at most half
// a circle, so reversing course traces a clockwise half-turn
// instead of spinning the long way.
func TurnAngleDelta(from, to int) int {
	clockwise := (NormalizeAngle(to) - NormalizeAngle(from) + AngleUnits) % AngleUnits
	if clockwise == 0 {
		return 0
	}
	if clockwise <= 1024 {
		return clockwise
	}
	return clockwise - AngleUnits
}

// AngleToFineDelta ports angleToFineDelta: the fine-unit (1/128
// tile) movement delta for travelling speed units along angle.
// Speed is a float64, as in TS: the final partial step of a move
// can be fractional when the speed multiplier is.
func AngleToFineDelta(angle int, speed float64) (dx, dy int) {
	if speed == 0 {
		return 0, 0
	}
	radians := float64(NormalizeAngle(angle)) * math.Pi / 1024
	// The int conversion turns -0 into 0, as the TS `+ 0` does.
	return int(math.Round(-math.Sin(radians) * speed)),
		int(math.Round(-math.Cos(radians) * speed))
}
