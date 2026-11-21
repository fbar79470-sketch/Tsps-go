package protocol

import "encoding/binary"

// EncodeDestination encodes the destination flag (encodeDestination,
// ClientProtocol.ts lines 2214-2218): DESTINATION (opcode 87, fixed
// length 4): world X u16, world Y u16. TS masks both coordinates to
// 16 bits; the same masking applies here. The flag marks the tile the
// client's destination marker renders on; (0, 0) is the value TS sends
// when the movement queue is empty (syncDestinationFlagToRoute,
// MovementQueue.ts:446-461).
func EncodeDestination(x, y int) []byte {
	payload := make([]byte, 4)
	binary.BigEndian.PutUint16(payload[0:2], uint16(x&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(y&0xffff))
	return mustEncode(ServerDestination, payload)
}
