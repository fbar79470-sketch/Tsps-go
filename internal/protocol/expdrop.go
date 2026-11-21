package protocol

import "encoding/binary"

// Exp-drop encoding: the LEGACY sendExpDrop layout.
//
// sendExpDrop (PacketSender.ts:1759-1767) builds a PacketBuilder with
// the legacy opcode 116, puts the skill's client id, puts the
// experience as an int, and hands the builder to session.write —
// which is a NO-OP in TS at bfd81e3 (PlayerSession.write,
// PlayerSession.ts:228-230: "Unported callers remain no-ops until
// they use a native packet encoder"). These bytes NEVER reach a
// client in TS, and no native exp-drop encoder exists in
// ClientProtocol.ts. The opcode is a legacy OSRS id, NOT a
// ServerPacketId value: 116 collides with WIDGET_SET_ROTATE_SPEED
// (fixed length 8, ServerPackets.ts:92,251) in the native table.
// This encoder is therefore NOT in the server length table
// (opcodes.go stays a verbatim transcription of ServerPackets.ts),
// it is NOT framed with mustEncode/EncodePacket (which would
// reject the 5-byte payload against the length-8 entry), and it
// must NEVER be sent with Session.SendPacket: opcode 116 on the
// wire would be parsed by the client as a rotate-speed packet and
// desync the stream. It reproduces the PacketBuilder FIXED framing
// exactly (one opcode byte, no length prefix, PacketBuilder's
// default PacketType.FIXED) as a pure layout function with
// byte-vector tests, so the legacy layout is on record; the
// session sender for it is a deliberate no-op mirroring TS
// write(). This is the Phase 19 legacy family precedent (the
// friend/ignore/clan-option encoders in social.go); the live XP
// display travels on the skills snapshot/delta packets
// (opcodes 70/71, state.go).

// LegacyExpDropOpcode is the sendExpDrop opcode (116). It collides
// with ServerWidgetSetRotateSpeed (also 116) in the native table;
// the two are unrelated packet families. It exists only so the
// legacy encoder below names the opcode it emits, and MUST NOT be
// added to serverPacketLengths: see the file header.
const LegacyExpDropOpcode = 116

// EncodeExpDrop encodes an experience-drop send (sendExpDrop,
// PacketSender.ts:1759-1767): the legacy opcode 116, one byte
// carrying the CLIENT skill id, and the experience as a 4-byte
// big-endian int.
//
// Byte semantics, verified against the PacketBuilder implementation
// (PacketBuilder.ts), not assumed:
//
//   - put (PacketBuilder.ts:126-129) delegates to puts with
//     ValueType.STANDARD, which writes value & 0xff as one byte
//     (PacketBuilder.ts:254-271). The client id is masked to 8
//     bits the same way.
//   - putInt (PacketBuilder.ts:186-189) delegates to putInts with
//     STANDARD/BIG, which writes put(value >> 24), put(value >>
//     16), put(value >> 8), puts(value) (PacketBuilder.ts:
//     302-309): four bytes, big-endian, each masked to 8 bits.
//     The JS >> shifts convert the float argument to int32
//     (truncating toward zero); uint32(int32(exp)) reproduces
//     that exactly, following the clanI32 precedent (social.go).
//     Callers pass the already-tenths-rounded integer TS computes
//     before the call (SkillManager.ts:149-153); the float-to-int
//     truncation itself lives in TS's >>, mirrored here by the
//     int32 conversion.
//   - new PacketBuilder(116) defaults to PacketType.FIXED
//     (PacketBuilder.ts:28-36), so the framing is the opcode byte
//     plus the payload with no length prefix (legacyPacket,
//     social.go).
//
// clientSkillID is the CLIENT id (Skill.getClientId), NOT the
// array index: only Construction (index 21 -> 22) and Hunter
// (index 22 -> 21) differ. Callers translate; the session sender
// owns the translation table, mirroring SendSkillDelta. It has
// no producer that sends it; TS drops it in write().
func EncodeExpDrop(clientSkillID, exp int) []byte {
	payload := make([]byte, 5)
	payload[0] = byte(clientSkillID & 0xff)
	binary.BigEndian.PutUint32(payload[1:5], uint32(int32(exp)))
	return legacyPacket(LegacyExpDropOpcode, payload)
}
