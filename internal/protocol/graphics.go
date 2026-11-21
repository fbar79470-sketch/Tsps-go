package protocol

import (
	"encoding/binary"
	"fmt"
)

// Graphic encoders: encodeTileSpotAnim (ClientProtocol.ts:
// 1586-1596) and encodeNpcSpotAnimationSpecific (:156-166).
//
// Framing: encodeServerPacket with SPOT_ANIM (opcode 83,
// length-table -1) and NPC_SPOTANIM_SPECIFIC (opcode 25,
// fixed length 9), verified in the ServerPacketId enum
// (ServerPackets.ts:29, 64).
//
// Liveness differs between the two, and the difference
// matters:
//
//   - encodeTileSpotAnim is reached via sendClientPacket
//     from PacketSender.sendGraphic / sendGlobalGraphic
//     (PacketSender.ts:1217-1234) — the LIVE queued path.
//   - encodeNpcSpotAnimationSpecific is reached via
//     queueNpcSpecificEffect from
//     PacketSender.sendNpcSpotAnimationSpecific
//     (PacketSender.ts:215-218), NOT via sendClientPacket
//     directly: the packet waits in the per-session
//     NpcSpecificEffects queue and sends after the NPC
//     sync only if the NPC is still visible at the same
//     index (PlayerSession.ts:632-641). The Go session
//     sender uses QueueNpcEffect for exactly this reason;
//     queuing the frame with SendPacket would send it even
//     for an NPC the viewer cannot see. See
//     internal/session/effects.go and graphic_sender.go.
//
// No Graphic class exists in Go (TS game/model/Graphic.ts):
// callers pass the id/height/delay primitives the encoders
// need. The Core `Graphic` / `GraphicHeight` / `Priority`
// members stay table-only (see internal/plugins/core.go).

// EncodeTileSpotAnim encodes a spot animation on a world
// tile (encodeTileSpotAnim, ClientProtocol.ts:1586-1596):
// SPOT_ANIM (opcode 83, 1-byte length prefix): spotId u16,
// target-type byte 2 (the client hands type 2 to its world
// spot-animation renderer), x u16, y u16, level byte,
// height byte, delay u16. All values masked as TS masks
// them.
func EncodeTileSpotAnim(spotID, x, y, level, height, delay int) []byte {
	payload := make([]byte, 11)
	binary.BigEndian.PutUint16(payload[0:2], uint16(spotID&0xffff))
	payload[2] = 2
	binary.BigEndian.PutUint16(payload[3:5], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[5:7], uint16(y&0xffff))
	payload[7] = byte(level & 0xff)
	payload[8] = byte(height & 0xff)
	binary.BigEndian.PutUint16(payload[9:11], uint16(delay&0xffff))
	return mustEncode(ServerSpotAnim, payload)
}

// effectInteger validates one effect field exactly as
// effectInteger does (ClientProtocol.ts:142-145), returning
// the TS-shaped RangeError message.
func effectInteger(name string, value, min, max int) error {
	if value < min || value > max {
		return fmt.Errorf("%s outside %d..%d", name, min, max)
	}
	return nil
}

// EncodeNpcSpotAnimationSpecific encodes a viewer-specific
// spot animation on one NPC (encodeNpcSpotAnimationSpecific,
// ClientProtocol.ts:156-166): NPC_SPOTANIM_SPECIFIC
// (opcode 25, fixed length 9, no length prefix): NPC index
// u16, spotanim id u16 (-1 clears the slot, encoding as
// 0xffff), slot byte, height u16, delay u16. Actor spot
// effects use independent slots; -1 clears only the
// requested slot. Ranges follow effectInteger exactly:
// index 0..65534, id -1..65534, slot 0..255, height
// 0..65535, delay 0..65535; out-of-range values return an
// error, mirroring the TS throw (the EncodeLocMerge
// precedent, loc.go).
func EncodeNpcSpotAnimationSpecific(index, id, slot, height, delay int) ([]byte, error) {
	for _, check := range []struct {
		name       string
		value, min int
		max        int
	}{
		{"NPC index", index, 0, 65534},
		{"spotanim", id, -1, 65534},
		{"spotanim slot", slot, 0, 255},
		{"spotanim height", height, 0, 65535},
		{"spotanim delay", delay, 0, 65535},
	} {
		if err := effectInteger(check.name, check.value, check.min, check.max); err != nil {
			return nil, err
		}
	}
	payload := make([]byte, 9)
	binary.BigEndian.PutUint16(payload[0:2], uint16(index))
	binary.BigEndian.PutUint16(payload[2:4], uint16(id&0xffff))
	payload[4] = byte(slot)
	binary.BigEndian.PutUint16(payload[5:7], uint16(height))
	binary.BigEndian.PutUint16(payload[7:9], uint16(delay))
	return EncodePacket(ServerNpcSpotanimSpecific, payload)
}
