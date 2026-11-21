// Player facing: the player form of Mobile's position-to-face
// machinery (Mobile.ts:587-598, 605-656, 689-691).
//
// TS Mobile carries positionToFace for every mobile; Go ported it
// for NPCs only (NPC.positionToFace + NPC.SetPositionToFace,
// npc_interaction.go:250-283), leaving players with no equivalent
// — the Phase 32 gap 16c. This file is the minimal player seam,
// built for ThickVines (ThickVines.Woodcutting.js: faceObject
// calls forcePositionToFace else setPositionToFace) and reused by
// the coordinator for the face-tree residual. The semantics mirror
// the NPC methods exactly: same-coordinate short-circuit in
// SetPositionToFace, always-flag in ForcePositionToFace, Facing
// updated toward the tile, the stored tile a copy, FlagFacePosition
// flagged.
//
// SYNC GAP: the stored tile and the flag are state only. The
// player sync snapshot (internal/session/flush.go,
// SyncPlayerSnapshot) carries no FaceTile/Rotation — only NPC
// views do (createNpcView) — so the flush encoder does not yet
// send a player's face-position update to clients. The logic
// state (Facing + PositionToFace) is correct from here on; wiring
// the encoder to it is a separate sync step, not part of this seam.
package game

// updatePlayerFacingDirection turns the player toward target, when
// the target is on another tile (Mobile.ts:613-618,
// updateFacingDirection, as NPC.updateFacingDirection does for
// NPCs). A nil target leaves Facing untouched.
func (p *Player) updatePlayerFacingDirection(target *Location) {
	if p == nil || target == nil {
		return
	}
	if dir := DirectionFromDeltas(target.X-p.Loc.X, target.Y-p.Loc.Y); dir != DirNone {
		p.Facing = dir
	}
}

// SetPositionToFace turns the player toward a tile (Mobile.ts:620-
// 641, setPositionToFace, as NPC.SetPositionToFace does for NPCs):
// a no-op when the new target has the same coordinates as the
// current one, or when both are nil; otherwise Facing updates
// toward the tile and FlagFacePosition is flagged. A nil target
// clears the face target (flagged, unless already clear). The
// stored tile is a copy: callers cannot mutate the player's face
// target through the pointer they passed. A nil player is a no-op.
func (p *Player) SetPositionToFace(position *Location) {
	if p == nil {
		return
	}
	current := p.positionToFace
	if current != nil && position != nil && current.Equals(*position) {
		return
	}
	if current == nil && position == nil {
		return
	}
	p.updatePlayerFacingDirection(position)
	if position == nil {
		p.positionToFace = nil
	} else {
		stored := *position
		p.positionToFace = &stored
	}
	p.Flags = p.Flags.Set(FlagFacePosition)
}

// ForcePositionToFace turns the player toward a tile, always
// flagging the face update (Mobile.ts:644-654,
// forcePositionToFace): like SetPositionToFace, but without the
// same-coordinate short-circuit. The field can already hold the
// target while the client still shows another direction (walking
// turned it), so the same-coordinate short-circuit would skip the
// packet and leave the player facing their last walk direction.
// A nil player is a no-op; a nil target clears the face target
// (flagged).
func (p *Player) ForcePositionToFace(position *Location) {
	if p == nil {
		return
	}
	p.updatePlayerFacingDirection(position)
	if position == nil {
		p.positionToFace = nil
	} else {
		stored := *position
		p.positionToFace = &stored
	}
	p.Flags = p.Flags.Set(FlagFacePosition)
}

// PositionToFace returns the face-target tile and whether one is
// set (Mobile.ts:587-589, getPositionToFace, as
// NPC.PositionToFace does for NPCs). A nil player reports none.
func (p *Player) PositionToFace() (Location, bool) {
	if p == nil || p.positionToFace == nil {
		return Location{}, false
	}
	return *p.positionToFace, true
}

// SetDirection turns the player toward direction, facing the
// adjacent tile in that direction (Mobile.ts:689-691,
// setDirection, as NPC.SetDirection does for NPCs).
func (p *Player) SetDirection(direction Direction) {
	if p == nil {
		return
	}
	target := p.Location().Transform(direction.Dx(), direction.Dy())
	p.SetPositionToFace(&target)
}
