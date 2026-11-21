package game

import "testing"

func TestNormalizeAngle(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 0}, {2047, 2047}, {2048, 0}, {2049, 1},
		{-1, 2047}, {-2048, 0}, {-2049, 2047}, {4096, 0},
	}
	for _, c := range cases {
		if got := NormalizeAngle(c.in); got != c.want {
			t.Errorf("NormalizeAngle(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestReverseAngle(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 1024}, {512, 1536}, {1024, 0}, {1536, 512}, {2047, 1023},
	}
	for _, c := range cases {
		if got := ReverseAngle(c.in); got != c.want {
			t.Errorf("ReverseAngle(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPackedHeadingToAngle(t *testing.T) {
	if got := PackedHeadingToAngle(0); got != 0 {
		t.Errorf("heading 0 = %d, want 0", got)
	}
	if got := PackedHeadingToAngle(4); got != 512 {
		t.Errorf("heading 4 = %d, want 512", got)
	}
	if got := PackedHeadingToAngle(15); got != 1920 {
		t.Errorf("heading 15 = %d, want 1920", got)
	}
	if got := PackedHeadingToAngle(16); got != 0 {
		t.Errorf("heading 16 wraps to %d, want 0", got)
	}
}

func TestAngleFromCoordDelta(t *testing.T) {
	// Angle 0 faces south (decreasing y); 512 west, 1024 north,
	// 1536 east. The zero delta maps to 0.
	cases := []struct {
		dx, dy, want int
	}{
		{0, 0, 0},
		{0, -1, 0},   // south
		{-1, 0, 512}, // west
		{0, 1, 1024}, // north
		{1, 0, 1536}, // east
		{-1, -1, 256},
		{1, 1, 1280},
	}
	for _, c := range cases {
		if got := AngleFromCoordDelta(c.dx, c.dy); got != c.want {
			t.Errorf("AngleFromCoordDelta(%d, %d) = %d, want %d", c.dx, c.dy, got, c.want)
		}
	}
}

func TestTurnAngleDelta(t *testing.T) {
	cases := []struct {
		from, to, want int
	}{
		{0, 0, 0},
		{0, 512, 512},
		{0, 1536, -512},
		{0, 1024, 1024}, // half circle prefers clockwise
		{1024, 0, 1024}, // half circle prefers clockwise
		{2000, 100, 148},
		{100, 2000, -148},
		{2048, 0, 0}, // normalized inputs
	}
	for _, c := range cases {
		if got := TurnAngleDelta(c.from, c.to); got != c.want {
			t.Errorf("TurnAngleDelta(%d, %d) = %d, want %d", c.from, c.to, got, c.want)
		}
	}
}

func TestAngleToFineDelta(t *testing.T) {
	// South (0) decreases y; north (1024) increases y; west
	// (512) decreases x; east (1536) increases x.
	cases := []struct {
		angle, wantX, wantY int
		speed               float64
	}{
		{0, 0, -128, 128},
		{1024, 0, 128, 128},
		{512, -128, 0, 128},
		{1536, 128, 0, 128},
		{0, 0, 0, 0},
		{256, -91, -91, 128}, // southwest diagonal rounds
	}
	for _, c := range cases {
		x, y := AngleToFineDelta(c.angle, c.speed)
		if x != c.wantX || y != c.wantY {
			t.Errorf("AngleToFineDelta(%d, %v) = (%d, %d), want (%d, %d)", c.angle, c.speed, x, y, c.wantX, c.wantY)
		}
	}
}
