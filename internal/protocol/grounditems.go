package protocol

import "encoding/binary"

// Ground-item encoders: encodeGroundItems / encodeGroundItemsDelta
// (ClientProtocol.ts:1490-1530), over the GroundItemView type
// (ClientProtocol.ts:260-275). Opcodes ServerGroundItems (54) and
// ServerGroundItemsDelta (55) carry length-table entries of -2
// (ServerPackets.ts:221-222; opcodes.go), so both packets frame with
// a 2-byte length prefix.
//
// The TS view type marks five fields optional (enabledOps,
// createdTick, privateUntilTick, expiresTick, ownerId) and the
// encoder resolves them with ?? defaults (-1/0/0/-1/0x1f). Go views
// carry plain ints and the VIEW BUILDER resolves those defaults —
// exactly what PacketSender.groundItemView does
// (PacketSender.ts:1461-1476), which always sets every field — so
// the encoder writes the fields verbatim (with the TS masks) and
// never invents a default. A zero-value view therefore encodes
// enabledOps 0 and ownerId 0, not the TS defaults; production views
// always come from the game layer's VisibleViews/perform path,
// which resolves every field.

// GroundItemBytes is the encoded size of one ground-item stack:
// GROUND_ITEM_BYTES (ClientProtocol.ts:1490).
const GroundItemBytes = 34

// GroundItemView is one ground-item stack as the client sees it:
// GroundItemView (ClientProtocol.ts:260-275), field order preserved.
// CreatedTick/PrivateUntilTick/ExpiresTick are world cycles; 0 for
// the latter two means the event never happens. OwnerID is the
// owning viewer's player index, or -1. Ownership is 0 (unowned), 1
// (viewer's own), or 2 (another player's); the TS type also allows
// 3, but no sender produces it. EnabledOps is the 5-bit ground
// action mask, resolved by the view builder (see the file header).
type GroundItemView struct {
	EnabledOps       int
	ID               int
	ItemID           int
	Quantity         int
	X, Y, Level      int
	CreatedTick      int
	PrivateUntilTick int
	ExpiresTick      int
	OwnerID          int
	IsPrivate        bool
	Ownership        int
}

// encodeGroundItem encodes one stack into GroundItemBytes bytes
// (ClientProtocol.ts:1492-1508): id int32, itemId u16, quantity
// int32, x u16, y u16, level byte, createdTick int32,
// privateUntilTick int32, expiresTick int32, ownerId int32,
// isPrivate byte, ownership byte, enabledOps byte masked to 5 bits.
// The TS `| 0` conversions are int32 truncation; the `& 0xffff`
// masks are uint16 truncation; a Buffer byte assignment masks to 8
// bits.
func encodeGroundItem(stack GroundItemView) []byte {
	payload := make([]byte, GroundItemBytes)
	binary.BigEndian.PutUint32(payload[0:4], uint32(int32(stack.ID)))
	binary.BigEndian.PutUint16(payload[4:6], uint16(stack.ItemID&0xffff))
	binary.BigEndian.PutUint32(payload[6:10], uint32(int32(stack.Quantity)))
	binary.BigEndian.PutUint16(payload[10:12], uint16(stack.X&0xffff))
	binary.BigEndian.PutUint16(payload[12:14], uint16(stack.Y&0xffff))
	payload[14] = byte(stack.Level)
	binary.BigEndian.PutUint32(payload[15:19], uint32(int32(stack.CreatedTick)))
	binary.BigEndian.PutUint32(payload[19:23], uint32(int32(stack.PrivateUntilTick)))
	binary.BigEndian.PutUint32(payload[23:27], uint32(int32(stack.ExpiresTick)))
	binary.BigEndian.PutUint32(payload[27:31], uint32(int32(stack.OwnerID)))
	if stack.IsPrivate {
		payload[31] = 1
	}
	payload[32] = byte(stack.Ownership)
	payload[33] = byte(stack.EnabledOps & 0x1f)
	return payload
}

// EncodeGroundItems encodes the full ground-item snapshot packet:
// serial int32 + stack count u16, then one record per stack
// (encodeGroundItems, ClientProtocol.ts:1510-1515). The session
// layer chunks snapshots to GroundItemsPerPacket stacks per packet;
// a direct call with so many stacks that the payload exceeds 65535
// bytes panics through mustEncode, mirroring the TS packet-length
// throw.
func EncodeGroundItems(serial int, stacks []GroundItemView) []byte {
	payload := make([]byte, 6+len(stacks)*GroundItemBytes)
	binary.BigEndian.PutUint32(payload[0:4], uint32(int32(serial)))
	binary.BigEndian.PutUint16(payload[4:6], uint16(len(stacks)))
	for i, stack := range stacks {
		copy(payload[6+i*GroundItemBytes:], encodeGroundItem(stack))
	}
	return mustEncode(ServerGroundItems, payload)
}

// EncodeGroundItemsDelta encodes the ground-item delta packet:
// serial int32 + upsert count u16, then one record per upsert, then
// the remove section (remove count u16 + one int32 id per removal)
// (encodeGroundItemsDelta, ClientProtocol.ts:1517-1528). The remove
// section is always present, even when empty (a bare u16 zero).
func EncodeGroundItemsDelta(serial int, upserts []GroundItemView, removes []int) []byte {
	payload := make([]byte, 6+len(upserts)*GroundItemBytes+2+len(removes)*4)
	binary.BigEndian.PutUint32(payload[0:4], uint32(int32(serial)))
	binary.BigEndian.PutUint16(payload[4:6], uint16(len(upserts)))
	for i, stack := range upserts {
		copy(payload[6+i*GroundItemBytes:], encodeGroundItem(stack))
	}
	offset := 6 + len(upserts)*GroundItemBytes
	binary.BigEndian.PutUint16(payload[offset:offset+2], uint16(len(removes)))
	for i, id := range removes {
		binary.BigEndian.PutUint32(payload[offset+2+i*4:], uint32(int32(id)))
	}
	return mustEncode(ServerGroundItemsDelta, payload)
}
