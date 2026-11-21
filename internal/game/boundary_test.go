package game

import "testing"

func TestBoundaryInside(t *testing.T) {
	b := NewBoundary(10, 20, 30, 40, 1)
	inside := []Location{
		{X: 10, Y: 30, Z: 1}, // min corner
		{X: 20, Y: 40, Z: 1}, // max corner
		{X: 15, Y: 35, Z: 1}, // middle
	}
	for _, loc := range inside {
		if !b.Inside(loc) {
			t.Errorf("Inside(%v) = false, want true", loc)
		}
	}
	outside := []Location{
		{X: 9, Y: 35, Z: 1},  // x below
		{X: 21, Y: 35, Z: 1}, // x above
		{X: 15, Y: 29, Z: 1}, // y below
		{X: 15, Y: 41, Z: 1}, // y above
		{X: 15, Y: 35, Z: 0}, // wrong plane
		{X: 15, Y: 35, Z: 2}, // wrong plane
	}
	for _, loc := range outside {
		if b.Inside(loc) {
			t.Errorf("Inside(%v) = true, want false", loc)
		}
	}
}

func TestBoundaryGetters(t *testing.T) {
	b := NewBoundary(1, 2, 3, 4, 0)
	if b.X() != 1 || b.X2() != 2 || b.Y() != 3 || b.Y2() != 4 {
		t.Errorf("getters = %d %d %d %d, want 1 2 3 4", b.X(), b.X2(), b.Y(), b.Y2())
	}
	if !b.HasHeight() || b.Height() != 0 {
		t.Errorf("height = (%d, %v), want (0, true)", b.Height(), b.HasHeight())
	}
}

// A boundary without a height matches NO location: TS loose
// equality makes `undefined == z` false for every z
// (Boundary.ts:34). In particular it does NOT behave as a
// plane-0 or any-plane boundary.
func TestBoundaryWithoutHeightMatchesNothing(t *testing.T) {
	b := NewBoundary(0, 100, 0, 100)
	if b.HasHeight() {
		t.Fatal("HasHeight = true, want false")
	}
	// Height() reads 0 for the `?? 0` use; Inside must still
	// refuse every plane, including 0.
	if b.Height() != 0 {
		t.Errorf("Height() = %d, want 0", b.Height())
	}
	for z := 0; z < 4; z++ {
		if b.Inside(Location{X: 50, Y: 50, Z: z}) {
			t.Errorf("Inside(z=%d) = true, want false", z)
		}
	}
}

func TestBoundaryZeroValueMatchesNothing(t *testing.T) {
	var b Boundary
	if b.Inside(Location{}) {
		t.Error("zero Boundary Inside = true, want false (no height)")
	}
}

func TestNewBoundaryTooManyHeightsPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("NewBoundary with two heights did not panic")
		}
	}()
	_ = NewBoundary(0, 1, 0, 1, 0, 1)
}
