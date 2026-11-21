package protocol

import "encoding/binary"

// Sound encoders: encodeSound / encodePlayJingle
// (ClientProtocol.ts:2753-2790).
//
// Framing: both TS functions use the legacy packet() helper
// (ClientProtocol.ts:1083-1094), not encodeServerPacket, but
// the opcodes are the same ServerPacketId values (SOUND = 131,
// PLAY_JINGLE = 132, verified in both the ServerPacket enum at
// ClientProtocol.ts:50-51 and the ServerPacketId enum at
// ServerPackets.ts:119-120) and the framings coincide with the
// length table: encodeSound uses packet()'s default 1-byte
// length prefix (table entry -1), and encodePlayJingle uses
// lengthBytes 0 (table entry 5, fixed). mustEncode therefore
// frames both exactly as TS does.
//
// Liveness: both encoders are reached via sendClientPacket
// from PacketSender (sendSoundEffect / sendSound /
// sendAreaSound / sendJingle, PacketSender.ts:252-306) — the
// LIVE queued path, not the no-op write(). See
// internal/session/sound_sender.go. The stale comment on
// sendSound (PacketSender.ts:265-266, claiming opcodes 174/175)
// is wrong; the encoder governs, as the phase 21 plan notes.

// SoundOptions are encodeSound's options object
// (ClientProtocol.ts:2757-2765). Positioned selects the
// 13-byte positioned form (x/y/level present); without it the
// payload is 8 bytes. TS decides from `options.x !==
// undefined`; Go has no undefined, so callers set Positioned
// explicitly. Loops/Delay/Radius/Attenuation follow the TS
// defaults when zero: loops clamps to a minimum of 1 (TS:
// Math.max(1, loops ?? 1)), delay to a minimum of 0, and
// radius/attenuation to 0..31.
type SoundOptions struct {
	Positioned  bool
	X, Y, Level int
	Loops       int
	Delay       int
	Radius      int
	Attenuation int
}

// EncodeSound encodes a sound effect (encodeSound,
// ClientProtocol.ts:2753-2782): SOUND (opcode 131, 1-byte
// length prefix): soundId u16, positioned flag byte, then —
// positioned only — x u16, y u16, level byte; then loops byte
// (minimum 1), delay u16 (minimum 0), radius byte and
// attenuation byte (each clamped 0..31). Every value is
// masked exactly as TS masks it. There is no volume field on
// the wire: PacketSender.sendSoundEffect accepts a volume and
// drops it (PacketSender.ts:252-261 passes only loops/delay
// to this encoder); see the session sender.
func EncodeSound(soundID int, opts SoundOptions) []byte {
	var payload []byte
	offset := 0
	if opts.Positioned {
		payload = make([]byte, 13)
	} else {
		payload = make([]byte, 8)
	}
	binary.BigEndian.PutUint16(payload[offset:], uint16(soundID&0xffff))
	offset += 2
	if opts.Positioned {
		payload[offset] = 1
	}
	offset++
	if opts.Positioned {
		binary.BigEndian.PutUint16(payload[offset:], uint16(opts.X&0xffff))
		binary.BigEndian.PutUint16(payload[offset+2:], uint16(opts.Y&0xffff))
		payload[offset+4] = byte(opts.Level & 0xff)
		offset += 5
	}
	loops := opts.Loops
	if loops < 1 {
		loops = 1
	}
	payload[offset] = byte(loops & 0xff)
	offset++
	delay := opts.Delay
	if delay < 0 {
		delay = 0
	}
	binary.BigEndian.PutUint16(payload[offset:], uint16(delay&0xffff))
	offset += 2
	payload[offset] = byte(clampInt(opts.Radius, 0, 31))
	offset++
	payload[offset] = byte(clampInt(opts.Attenuation, 0, 31))
	return mustEncode(ServerSound, payload)
}

// EncodePlayJingle encodes a jingle (encodePlayJingle,
// ClientProtocol.ts:2785-2790): PLAY_JINGLE (opcode 132,
// fixed length 5, no length prefix): jingleId u16, then the
// delay as a 3-byte IME value in SHUFFLED order — bytes 2..4
// are value>>16, value, value>>8. The shuffle is copied
// exactly; do not "fix" it. The delay is clamped to
// 0..0xffffff, as TS clamps it.
func EncodePlayJingle(jingleID, delay int) []byte {
	payload := make([]byte, 5)
	value := clampInt(delay, 0, 0xffffff)
	binary.BigEndian.PutUint16(payload[0:2], uint16(jingleID&0xffff))
	payload[2] = byte(value >> 16)
	payload[3] = byte(value)
	payload[4] = byte(value >> 8)
	return mustEncode(ServerPlayJingle, payload)
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
