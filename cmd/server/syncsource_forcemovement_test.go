package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// TestSnapshotForcedMovement covers the Phase 40 snapshot
// population (PlayerSession.ts:742-789): the landing tile is
// present whenever a force movement is active, the glide view only
// while FlagForcedMovement fired, the movement's animation rides
// the animation block only when no animation is already set, and
// the direction maps through [1024, 1536, 0, 512].
func TestSnapshotForcedMovement(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3520, Z: 0}
	p := game.NewPlayer("alice", spawn)
	p.SetForceMovement(&game.ForceMovement{
		Start:        spawn,
		Delta:        game.Location{X: 0, Y: 3},
		Speed:        0,
		ReverseSpeed: 70,
		Direction:    0,
		Animation:    6132,
	})

	// Mid-glide, after ResetUpdating cleared the install tick's
	// flag: no glide view, but the landing tile is present, and no
	// animation rides along.
	p.ResetUpdating()
	snap := playerSnapshot(p, p)
	if snap.ForcedMovement != nil {
		t.Errorf("no flag: ForcedMovement = %+v, want nil", snap.ForcedMovement)
	}
	if snap.ForcedMovementEnd == nil || *snap.ForcedMovementEnd != (protocol.Tile{X: 3089, Y: 3523, Level: 0}) {
		t.Errorf("no flag: ForcedMovementEnd = %+v, want (3089, 3523, 0)", snap.ForcedMovementEnd)
	}
	if snap.Animation != nil {
		t.Errorf("no flag: Animation = %+v, want nil", snap.Animation)
	}

	// Flag set: the glide view appears with the ditch's deltas and
	// cycle offsets, and the jump animation becomes the animation
	// block.
	p.Flags = p.Flags.Set(game.FlagForcedMovement)
	snap = playerSnapshot(p, p)
	wantView := &protocol.ForcedMovementView{
		StartDeltaX: 0, StartDeltaY: 0, EndDeltaX: 0, EndDeltaY: 3,
		StartCycleOffset: 0, EndCycleOffset: 70, Direction: 1024,
	}
	if snap.ForcedMovement == nil || *snap.ForcedMovement != *wantView {
		t.Errorf("flag set: ForcedMovement = %+v, want %+v", snap.ForcedMovement, wantView)
	}
	if snap.ForcedMovementEnd == nil || *snap.ForcedMovementEnd != (protocol.Tile{X: 3089, Y: 3523, Level: 0}) {
		t.Errorf("flag set: ForcedMovementEnd = %+v, want (3089, 3523, 0)", snap.ForcedMovementEnd)
	}
	if snap.Animation == nil || snap.Animation.ID != 6132 || snap.Animation.Delay != 0 {
		t.Errorf("flag set: Animation = %+v, want id 6132 delay 0", snap.Animation)
	}

	// An animation already set on the mobile wins: the movement's
	// animation does not override it.
	p.PerformAnimation(422, 1)
	snap = playerSnapshot(p, p)
	if snap.Animation == nil || snap.Animation.ID != 422 || snap.Animation.Delay != 1 {
		t.Errorf("own animation: Animation = %+v, want id 422 delay 1", snap.Animation)
	}

	// ResetUpdating clears the flag (and the animation), so the
	// glide view goes away — but the landing tile stays, because
	// the force movement is still active.
	p.ResetUpdating()
	snap = playerSnapshot(p, p)
	if snap.ForcedMovement != nil {
		t.Errorf("after reset: ForcedMovement = %+v, want nil", snap.ForcedMovement)
	}
	if snap.ForcedMovementEnd == nil || *snap.ForcedMovementEnd != (protocol.Tile{X: 3089, Y: 3523, Level: 0}) {
		t.Errorf("after reset: ForcedMovementEnd = %+v, want (3089, 3523, 0)", snap.ForcedMovementEnd)
	}
}

// TestForcedMovementDirection pins the PlayerSession.ts:777 facing
// table: [1024, 1536, 0, 512][direction], with an out-of-range
// direction passing through unchanged.
func TestForcedMovementDirection(t *testing.T) {
	cases := []struct{ dir, want int }{
		{0, 1024}, {1, 1536}, {2, 0}, {3, 512}, {5, 5}, {-1, -1},
	}
	for _, c := range cases {
		if got := forcedMovementDirection(c.dir); got != c.want {
			t.Errorf("forcedMovementDirection(%d) = %d, want %d", c.dir, got, c.want)
		}
	}
}
