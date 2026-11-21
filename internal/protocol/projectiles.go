package protocol

import "encoding/binary"

// Projectile encoders: encodeProjectiles (ClientProtocol.ts:
// 3749-3771) and the ProjectileView type (:233-250).
//
// Framing: encodeServerPacket with PROJECTILES (opcode 84,
// ServerPackets.ts:65), length-table -2 (2-byte length
// prefix). A payload over 65535 bytes panics in mustEncode,
// mirroring the TS throw; that needs over 2,259 projectiles
// in one packet, which no sender produces (PacketSender
// .sendProjectile sends exactly one per call).
//
// Liveness: reached via sendClientPacket from
// PacketSender.sendProjectile (PacketSender.ts:1236-1284) —
// the LIVE queued path. The session sender owns the
// end-cycle fallback, the height x4 scaling, and the
// lock-on/source index mapping; this encoder writes the view
// verbatim.

// ProjectileActorKind is the packet's actor kind byte:
// 0 none, 1 player, 2 NPC (encodeProjectiles,
// ClientProtocol.ts:3767-3772: sourceActor/targetActor kind
// mapping). The zero value is "no actor", matching TS's
// undefined actor.
type ProjectileActorKind int

const (
	ProjectileActorNone   ProjectileActorKind = 0
	ProjectileActorPlayer ProjectileActorKind = 1
	ProjectileActorNPC    ProjectileActorKind = 2
)

// ProjectileActor is one packet actor reference: the actor a
// projectile locks onto (targetActor) or is fired by
// (sourceActor). Index is the actor's world index (0-based),
// as TS writes sourceActor/targetActor index verbatim; the
// negative-player/positive-NPC resolved-index mapping lives
// in the session sender (PacketSender.projectileActor,
// PacketSender.ts:1287-1291), not here.
type ProjectileActor struct {
	Kind  ProjectileActorKind
	Index int
}

// ProjectileView is one projectile in the packet
// (ProjectileView, ClientProtocol.ts:233-250): the id, the
// source and target tiles, the heights, the arc slope, the
// start position within the source tile, the start/end cycle
// offsets, and the optional target/source actors. Field
// order below follows the TS type; wire order is the
// encoder's.
type ProjectileView struct {
	ProjectileID     int
	Source           Tile
	Target           Tile
	SourceHeight     int
	EndHeight        int
	Slope            int
	StartPos         int
	StartCycleOffset int
	EndCycleOffset   int
	TargetActor      ProjectileActor
	SourceActor      ProjectileActor
}

// EncodeProjectiles encodes a projectile packet
// (encodeProjectiles, ClientProtocol.ts:3749-3771): u16
// count, then 29 bytes per projectile: id u16, source x u16,
// source y u16, source level byte, source height u16, target
// x u16, target y u16, target level byte, end height u16,
// slope byte, start position u16, start cycle offset u16,
// end cycle offset u16, source actor kind byte + index u16,
// target actor kind byte + index u16. An absent actor
// (Kind None) writes kind 0 and index 0, as TS writes
// `?? 0` for a missing actor; callers use the zero
// ProjectileActor for "absent" and must not set a nonzero
// index with Kind None, which TS cannot express.
func EncodeProjectiles(projectiles []ProjectileView) []byte {
	payload := make([]byte, 2+len(projectiles)*29)
	binary.BigEndian.PutUint16(payload[0:2], uint16(len(projectiles)))
	offset := 2
	for _, p := range projectiles {
		binary.BigEndian.PutUint16(payload[offset:], uint16(p.ProjectileID&0xffff))
		binary.BigEndian.PutUint16(payload[offset+2:], uint16(p.Source.X&0xffff))
		binary.BigEndian.PutUint16(payload[offset+4:], uint16(p.Source.Y&0xffff))
		payload[offset+6] = byte(p.Source.Level & 0xff)
		binary.BigEndian.PutUint16(payload[offset+7:], uint16(p.SourceHeight&0xffff))
		binary.BigEndian.PutUint16(payload[offset+9:], uint16(p.Target.X&0xffff))
		binary.BigEndian.PutUint16(payload[offset+11:], uint16(p.Target.Y&0xffff))
		payload[offset+13] = byte(p.Target.Level & 0xff)
		binary.BigEndian.PutUint16(payload[offset+14:], uint16(p.EndHeight&0xffff))
		payload[offset+16] = byte(p.Slope & 0xff)
		binary.BigEndian.PutUint16(payload[offset+17:], uint16(p.StartPos&0xffff))
		binary.BigEndian.PutUint16(payload[offset+19:], uint16(p.StartCycleOffset&0xffff))
		binary.BigEndian.PutUint16(payload[offset+21:], uint16(p.EndCycleOffset&0xffff))
		payload[offset+23] = byte(p.SourceActor.Kind)
		binary.BigEndian.PutUint16(payload[offset+24:], uint16(p.SourceActor.Index&0xffff))
		payload[offset+26] = byte(p.TargetActor.Kind)
		binary.BigEndian.PutUint16(payload[offset+27:], uint16(p.TargetActor.Index&0xffff))
		offset += 29
	}
	return mustEncode(ServerProjectiles, payload)
}
