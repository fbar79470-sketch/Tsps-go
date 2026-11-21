// Package protocol implements the TSPS custom binary wire protocol: the
// framing, opcode/length tables, inbound Reader, bit writer, and the
// verified framed-packet encoders and typed packet decoders.
//
// Source: TSPS server (commit bfd81e3), server/src/main/typescript/elvarg/net/protocol/:
// ClientProtocol.ts (Reader, decode/encode, packet layouts),
// ClientPackets.ts, NativeClientPackets.ts, ServerPackets.ts (opcode/length
// tables), BitWriter.ts. Every opcode, length, and layout in this package
// cites the TS file and line it was verified against.
//
// Framing (ClientProtocol.ts decodeClientPackets, lines 486-506, and
// encodeServerPacket, lines 1053-1068): one WebSocket message holds one or
// more concatenated packets. Each packet is a 1-byte opcode followed by an
// optional length prefix: table value -1 means a 1-byte length, -2 means a
// 2-byte big-endian length, and >= 0 means a fixed length with no prefix.
// All multi-byte integers are big-endian unless a Reader method says
// otherwise; strings are latin1, NUL-terminated. There is no ISAAC, no RSA,
// and no encryption on the live path; the legacy PacketBuilder/PacketEncoder/
// IsaacRandom code is dead and deliberately not ported.
//
// Ported encoders (sync.go, appearance.go, state.go, widgets.go,
// bootstrap_encode.go, chat.go, grounditems.go, song.go,
// destination.go, hideops.go): the player/NPC sync encoders
// (encodePlayerSync, encodeNpcSync — note the TS name is
// encodeNpcSync, not encodeNpcInfo — encodeInitialPlayerSync,
// encodePlayerAppearance), the handshake / default-animations /
// gameframe-bootstrap / logout-response encoders, varp/varbit,
// inventory/bank, skills, run-energy, and widget encoders, the
// chat-message encoder (encodeChatMessage, with the ChatType
// channels, legacy colour rewrite, and highlight resolver), the
// ground-item snapshot/delta encoders, the play-song encoder,
// the destination-flag encoder, and the hide-NPC-ops encoder.
// The sync encoders are pure functions over view/state types;
// nothing in this package maintains visibility or local lists.
// The per-tick sync encoders are invoked by the session flush
// (FlushTick) every tick; see PORT_STATUS.md.
//
// Not ported (left absent rather than stubbed): clan, camera, shop,
// trade, projectile, sound jingles, world-entity, region
// replacement, interaction-mode, loc change/del/merge/anim, and
// the InteractionMode/LocMerge/WorldMapProtocol helpers whose only
// consumers are those encoders.
package protocol
