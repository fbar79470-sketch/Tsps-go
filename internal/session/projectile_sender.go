package session

import (
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Projectile sender: the session side of
// PacketSender.sendProjectile (PacketSender.ts:1236-1284).
// It goes through sendClientPacket in TS — the LIVE queued
// path — and queues one EncodeProjectiles frame here
// through SendPacket.
//
// The TS method takes untyped start/end/lockon/source
// arguments and resolves them; the Go form takes game
// Locations and pre-resolved signed indices (players
// negative, NPCs positive, 0 none), which is exactly the
// integer form TS resolveProjectileTargetIndex accepts so
// target binding stays stable (PacketSender.ts:1293-1297).
// The game-layer Projectile resolves both indices at
// construction and calls this sender through the
// ProjectileSender extension.
//
// Three TS behaviors live here, not in the encoder:
//   - The end-cycle fallback (PacketSender.ts:1267-1272):
//     an end at or before the launch is drawn for one
//     client cycle and vanishes on the caster, so an end
//     cycle offset at or below delay+1 falls back to a
//     5-cycles-a-tile flight (delay + 5 + tiles*5, where
//     tiles is the Chebyshev distance between start and
//     end). The fallback test is `endCycleOffset <=
//     delay + 1` AFTER the max(delay+1, speed) clamp, so
//     it fires exactly when speed <= delay+1.
//   - The height scaling (PacketSender.ts:1278-1279):
//     heights are multiplied by 4 before encoding.
//   - The actor mapping (PacketSender.projectileActor,
//     PacketSender.ts:1287-1291): a negative resolved
//     index is a player (index -index-1), a positive one
//     an NPC (index-1), 0 no actor.

// Session must satisfy the game's projectile seam
// structurally, as it satisfies game.SoundSender (see
// sound_sender.go).
var _ game.ProjectileSender = (*Session)(nil)

// SendProjectile queues one projectile frame, porting
// PacketSender.sendProjectile (PacketSender.ts:1236-1284).
// offset is accepted and ignored: TS declares it and never
// reads it. lockonIndex and sourceIndex are pre-resolved
// signed indices (see the file header). distanceOffset is
// the start position within the source tile (TS
// distanceOffset, default 64); angle is the arc slope (TS
// default 16). Heights arrive as TS floats in quarter-tiles
// and are scaled x4 here (PacketSender.ts:1278-1279), so
// 40.75 -> 163 and 36.5 -> 146 exactly. The packet joins
// the pending queue and flushes with the tick.
func (s *Session) SendProjectile(start, end game.Location, offset, speed, projectileID int, startHeight, endHeight float64, lockonIndex, delay, angle, distanceOffset, sourceIndex int) {
	// Offset deliberately unused: TS never reads it.
	_ = offset
	endCycleOffset := delay + 1
	if speed > endCycleOffset {
		endCycleOffset = speed
	}
	if endCycleOffset <= delay+1 {
		// TS: start.getDistance(end) — Chebyshev, or
		// MAX_SAFE_INTEGER across planes, which `| 0`
		// turns into -1 and Math.max(0, .) into 0.
		// Location.Distance returns -1 across planes,
		// and the same max(0, .) gives 0: identical.
		tiles := start.Distance(end)
		if tiles < 0 {
			tiles = 0
		}
		endCycleOffset = delay + 5 + tiles*5
	}
	// TS reads the target level as end.getZ?.() ??
	// start.getZ?.() ?? 0; a Go Location always has a Z,
	// so the target level is always the end tile's and
	// the fallback is unexpressible.
	s.SendPacket(protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     projectileID,
		Source:           protocol.Tile{X: start.X, Y: start.Y, Level: start.Z},
		Target:           protocol.Tile{X: end.X, Y: end.Y, Level: end.Z},
		SourceHeight:     int(math.Round(startHeight * 4)),
		EndHeight:        int(math.Round(endHeight * 4)),
		Slope:            angle,
		StartPos:         distanceOffset,
		StartCycleOffset: delay,
		EndCycleOffset:   endCycleOffset,
		TargetActor:      projectileActor(lockonIndex),
		SourceActor:      projectileActor(sourceIndex),
	}}))
}

// projectileActor maps a resolved lock-on index to the
// packet's actor reference (PacketSender.projectileActor,
// PacketSender.ts:1287-1291): negative to a player,
// positive to an NPC, 0 to no actor.
func projectileActor(index int) protocol.ProjectileActor {
	if index < 0 {
		return protocol.ProjectileActor{Kind: protocol.ProjectileActorPlayer, Index: -index - 1}
	}
	if index > 0 {
		return protocol.ProjectileActor{Kind: protocol.ProjectileActorNPC, Index: index - 1}
	}
	return protocol.ProjectileActor{}
}
