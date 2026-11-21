package protocol

import "encoding/binary"

// World-entity encoders: encodeRebuildWorldEntity /
// encodeWorldEntityInfo (ClientProtocol.ts:1942-1997) and
// the WorldEntityPosition / WorldEntityUpdate /
// WorldEntitySpawn types (:1870-1895).
//
// Framing: encodeServerPacket with REBUILD_WORLDENTITY
// (opcode 142, length-table -2) and WORLDENTITY_INFO
// (opcode 143, length-table -1), verified in the
// ServerPacketId enum (ServerPackets.ts:124-125). Both
// return errors: a world-entity info payload over 255 bytes
// (or a rebuild payload over 65535) is rejected by the
// length table, mirroring the TS throw (the
// EncodeRebuildRegion precedent, encode.go).
//
// Liveness: driven by WorldEntitySync.flush, called from
// flushClient (PlayerSession.ts:616) — live. The Go flush
// that calls these encoders is Worker C's (the omission
// site at internal/session/flush.go:429); this file owns
// ONLY the encoders.

// WorldEntityPosition is a world entity's position
// (WorldEntityPosition, ClientProtocol.ts:1871-1876): fine
// units (1/128 tile) on X and Z, Y the vertical fine unit,
// and an orientation out of 2048.
type WorldEntityPosition struct {
	X           int
	Y           int
	Z           int
	Orientation int
}

// WorldEntityUpdateKind is the updateType of a
// WorldEntityUpdate: 0 despawn, 1 unchanged, 2 move
// smoothly by Delta, 3 snap by Delta
// (ClientProtocol.ts:1879-1881).
type WorldEntityUpdateKind int

const (
	WorldEntityDespawn   WorldEntityUpdateKind = 0
	WorldEntityUnchanged WorldEntityUpdateKind = 1
	WorldEntityMove      WorldEntityUpdateKind = 2
	WorldEntitySnap      WorldEntityUpdateKind = 3
)

// WorldEntityUpdate is one per-tick update for an entity
// the client already has (WorldEntityUpdate,
// ClientProtocol.ts:1879-1882). Delta is read only for
// Move/Snap updates; a nil Delta encodes as all zeros, as
// TS defaults a missing delta to {x: 0, y: 0, z: 0,
// orientation: 0}.
type WorldEntityUpdate struct {
	UpdateType WorldEntityUpdateKind
	Delta      *WorldEntityPosition
}

// WorldEntitySpawn is one new world entity
// (WorldEntitySpawn, ClientProtocol.ts:1884-1890).
type WorldEntitySpawn struct {
	EntityIndex int
	SizeX       int
	SizeZ       int
	ConfigID    int
	DrawMode    int
	Position    WorldEntityPosition
}

// worldEntityPositionWidth is the width function of
// writeWorldEntityPosition (ClientProtocol.ts:1912-1914):
// 0 = zero and not sent, 1 = byte, 2 = short, 3 = int.
// Values are truncated to int32 first (TS `| 0`).
func worldEntityPositionWidth(value int32) int {
	switch {
	case value == 0:
		return 0
	case value >= -128 && value <= 127:
		return 1
	case value >= -32768 && value <= 32767:
		return 2
	default:
		return 3
	}
}

// writeWorldEntityPosition appends a position in the
// fewest-bytes form (writeWorldEntityPosition,
// ClientProtocol.ts:1911-1923): a flags byte of 2-bit
// widths in x, y, z, orientation order (width <<
// (index*2)), then each nonzero-width value in order as a
// byte, big-endian short, or big-endian int. The TS
// ByteWriter masks exactly as the Go conversions do
// (u8: & 0xff; u16: big-endian; i32: big-endian).
func writeWorldEntityPosition(out []byte, position WorldEntityPosition) []byte {
	values := [4]int32{int32(position.X), int32(position.Y), int32(position.Z), int32(position.Orientation)}
	flags := 0
	for i, value := range values {
		flags |= worldEntityPositionWidth(value) << (i * 2)
	}
	out = append(out, byte(flags))
	for _, value := range values {
		switch worldEntityPositionWidth(value) {
		case 1:
			out = append(out, byte(value))
		case 2:
			var buf [2]byte
			binary.BigEndian.PutUint16(buf[:], uint16(value))
			out = append(out, buf[:]...)
		case 3:
			var buf [4]byte
			binary.BigEndian.PutUint32(buf[:], uint32(value))
			out = append(out, buf[:]...)
		}
	}
	return out
}

// EncodeRebuildWorldEntity builds a world entity's own
// scene (a boat deck) on the client from cache template
// chunks (encodeRebuildWorldEntity, ClientProtocol.ts:
// 1942-1985), like EncodeRebuildRegion does for the main
// map.
//
// Layout: entityIndex u16, configId u16, sizeX byte, sizeZ
// byte, regionX u16, regionY u16, regionY u16 AGAIN, a 0
// force-reload byte, regionX u16 AGAIN, XTEA key count
// u16, a 0 build-areas byte — the duplicated regionY /
// regionX fields are copied exactly as TS writes them —
// then the bit-packed 4x13x13 chunk palette (one presence
// bit per chunk in plane/x/y order, plus 26 bits per
// present chunk; -1 means absent), then each XTEA key as
// 4 big-endian int32s. The palette section is byte for
// byte the EncodeRebuildRegion palette for the same
// chunks: both use the same MSB-first BitWriter (TS
// BitWriter.writeBits(count, value); Go WriteBits(value,
// count) — reversed arguments, identical packing).
func EncodeRebuildWorldEntity(entityIndex, configID, sizeX, sizeZ, regionX, regionY int, templateChunks [4][13][13]int32, xteaKeys [][4]int32) ([]byte, error) {
	out := make([]byte, 0, 18+len(xteaKeys)*16)
	var u16buf [2]byte
	putU16 := func(value int) {
		binary.BigEndian.PutUint16(u16buf[:], uint16(value&0xffff))
		out = append(out, u16buf[:]...)
	}
	putU16(entityIndex)
	putU16(configID)
	out = append(out, byte(sizeX), byte(sizeZ))
	putU16(regionX)
	putU16(regionY)
	putU16(regionY)
	out = append(out, 0)
	putU16(regionX)
	putU16(len(xteaKeys))
	out = append(out, 0)
	bits := NewBitWriter()
	for plane := 0; plane < 4; plane++ {
		for x := 0; x < 13; x++ {
			for y := 0; y < 13; y++ {
				chunk := templateChunks[plane][x][y]
				if chunk == -1 {
					bits.WriteBits(0, 1)
				} else {
					bits.WriteBits(1, 1)
					bits.WriteBits(uint32(chunk), 26)
				}
			}
		}
	}
	out = append(out, bits.Bytes()...)
	for _, key := range xteaKeys {
		var keybuf [16]byte
		for j, value := range key {
			binary.BigEndian.PutUint32(keybuf[j*4:], uint32(value))
		}
		out = append(out, keybuf[:]...)
	}
	return EncodePacket(ServerRebuildWorldEntity, out)
}

// EncodeWorldEntityInfo encodes the per-tick world entity
// list for one viewer (encodeWorldEntityInfo,
// ClientProtocol.ts:1986-1997): a count byte for the
// updates, then one entry per update (updateType byte;
// a position delta for Move/Snap; a 0 byte for every
// update except Despawn — "no animation or action mask"),
// then one entry per spawn (entityIndex u16, sizeX byte,
// sizeZ byte, configId u16, position, drawMode byte, 0
// byte). Updates are in the client's entity order; spawns
// follow.
func EncodeWorldEntityInfo(updates []WorldEntityUpdate, spawns []WorldEntitySpawn) ([]byte, error) {
	out := []byte{byte(len(updates))}
	for _, update := range updates {
		out = append(out, byte(update.UpdateType))
		if update.UpdateType >= 2 {
			delta := WorldEntityPosition{}
			if update.Delta != nil {
				delta = *update.Delta
			}
			out = writeWorldEntityPosition(out, delta)
		}
		if update.UpdateType != 0 {
			out = append(out, 0)
		}
	}
	for _, spawn := range spawns {
		var buf [2]byte
		binary.BigEndian.PutUint16(buf[:], uint16(spawn.EntityIndex&0xffff))
		out = append(out, buf[:]...)
		out = append(out, byte(spawn.SizeX), byte(spawn.SizeZ))
		binary.BigEndian.PutUint16(buf[:], uint16(spawn.ConfigID&0xffff))
		out = append(out, buf[:]...)
		out = writeWorldEntityPosition(out, spawn.Position)
		out = append(out, byte(spawn.DrawMode), 0)
	}
	return EncodePacket(ServerWorldEntityInfo, out)
}
