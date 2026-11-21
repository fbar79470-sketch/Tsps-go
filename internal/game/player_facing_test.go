package game

import "testing"

// TestPlayerSetPositionToFace pins the player facing seam
// (Mobile.ts:620-641, setPositionToFace, as NPC.SetPositionToFace
// does for NPCs): facing updates toward the tile, the flag is set,
// and the stored tile is a copy.
func TestPlayerSetPositionToFace(t *testing.T) {
	p := NewPlayer("alice", Location{X: 10, Y: 10, Z: 0})
	p.Facing = DirSouth
	east := Location{X: 11, Y: 10, Z: 0}
	p.SetPositionToFace(&east)
	if p.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST", p.Facing)
	}
	if !p.Flags.Has(FlagFacePosition) {
		t.Error("FlagFacePosition not set")
	}
	if got, ok := p.PositionToFace(); !ok || !got.Equals(east) {
		t.Errorf("PositionToFace = %+v, %v, want %+v, true", got, ok, east)
	}
	// Same coordinates again: no-op, no flag.
	p.ResetUpdating()
	same := Location{X: 11, Y: 10, Z: 0}
	p.SetPositionToFace(&same)
	if p.Flags.Has(FlagFacePosition) {
		t.Error("same-coordinate SetPositionToFace must not flag")
	}
	if p.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST still", p.Facing)
	}
	// The stored tile is a copy: mutating the caller's location
	// must not move the face target.
	east.X = 99
	if got, _ := p.PositionToFace(); got.X != 11 {
		t.Errorf("PositionToFace X = %d, want 11 (stored tile must be a copy)", got.X)
	}
	// Clearing: flagged, unless already clear.
	p.SetPositionToFace(nil)
	if _, ok := p.PositionToFace(); ok {
		t.Error("nil SetPositionToFace must clear the face target")
	}
	if !p.Flags.Has(FlagFacePosition) {
		t.Error("clearing SetPositionToFace must flag")
	}
	p.ResetUpdating()
	p.SetPositionToFace(nil)
	if p.Flags.Has(FlagFacePosition) {
		t.Error("clearing an already-clear face target must not flag")
	}
	// A same-tile target leaves Facing untouched.
	p.Facing = DirNorth
	here := Location{X: 10, Y: 10, Z: 0}
	p.SetPositionToFace(&here)
	if p.Facing != DirNorth {
		t.Errorf("same-tile SetPositionToFace: Facing = %v, want NORTH", p.Facing)
	}
}

// TestPlayerForcePositionToFace pins the forced form
// (Mobile.ts:644-654, forcePositionToFace): it always flags, even
// when the target has the same coordinates as the current one —
// the ThickVines faceObject path (ThickVines.Woodcutting.js:
// faceObject prefers forcePositionToFace).
func TestPlayerForcePositionToFace(t *testing.T) {
	p := NewPlayer("alice", Location{X: 10, Y: 10, Z: 0})
	p.Facing = DirSouth
	east := Location{X: 11, Y: 10, Z: 0}
	p.ForcePositionToFace(&east)
	if p.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST", p.Facing)
	}
	if !p.Flags.Has(FlagFacePosition) {
		t.Error("FlagFacePosition not set")
	}
	// Same coordinates again: still flags (no short-circuit).
	p.ResetUpdating()
	same := Location{X: 11, Y: 10, Z: 0}
	p.ForcePositionToFace(&same)
	if !p.Flags.Has(FlagFacePosition) {
		t.Error("ForcePositionToFace must flag even for the same coordinates")
	}
	// A nil receiver is a no-op, never a panic.
	var nilPlayer *Player
	nilPlayer.SetPositionToFace(&east)
	nilPlayer.ForcePositionToFace(&east)
	nilPlayer.SetDirection(DirEast)
	if _, ok := nilPlayer.PositionToFace(); ok {
		t.Error("nil player must report no face target")
	}
}

// TestPlayerSetDirection pins the direction form
// (Mobile.ts:689-691, setDirection): facing the adjacent tile in
// the given direction.
func TestPlayerSetDirection(t *testing.T) {
	p := NewPlayer("alice", Location{X: 10, Y: 10, Z: 0})
	p.SetDirection(DirNorth)
	if p.Facing != DirNorth {
		t.Errorf("Facing = %v, want NORTH", p.Facing)
	}
	if got, ok := p.PositionToFace(); !ok || got.X != 10 || got.Y != 11 {
		t.Errorf("PositionToFace = %+v, %v, want {10 11 0}, true", got, ok)
	}
}
