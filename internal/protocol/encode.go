package protocol

import "encoding/binary"

// EncodeWelcome encodes the WELCOME packet (opcode 0) sent immediately on
// connection: tickMs int32 + serverTime int32, fixed length 8, no length
// prefix (encodeWelcome, ClientProtocol.ts lines 2739-2744).
func EncodeWelcome(tickMs, serverTime int32) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(tickMs))
	binary.BigEndian.PutUint32(payload[4:8], uint32(serverTime))
	return mustEncode(ServerWelcome, payload)
}

// EncodeTick encodes the TICK packet (opcode 1): tick int32 + serverTime
// int32, fixed length 8, no length prefix (encodeTick, ClientProtocol.ts
// lines 2746-2750).
func EncodeTick(tick, serverTime int32) []byte {
	payload := make([]byte, 8)
	binary.BigEndian.PutUint32(payload[0:4], uint32(tick))
	binary.BigEndian.PutUint32(payload[4:8], uint32(serverTime))
	return mustEncode(ServerTick, payload)
}

// EncodeLoginResponse encodes the LOGIN_RESPONSE packet (opcode 3,
// 1-byte length prefix): success byte + errorCode int32, followed by the
// error message, display name, and login token as latin1 NUL-terminated
// strings (encodeLoginResponse, ClientProtocol.ts lines 2812-2826).
//
// All three strings are always present, even on failure: TS concatenates
// string(error), string(displayName), and string(loginToken) unconditionally,
// so a failure response still carries two empty (bare NUL) strings for the
// display name and token. It panics if the framed payload would exceed 255
// bytes, mirroring the TS packet() throw.
func EncodeLoginResponse(success bool, code int32, message, displayName, token string) []byte {
	payload := make([]byte, 0, 5+len(message)+len(displayName)+len(token)+3)
	fixed := make([]byte, 5)
	if success {
		fixed[0] = 1
	}
	binary.BigEndian.PutUint32(fixed[1:5], uint32(code))
	payload = append(payload, fixed...)
	payload = append(payload, latin1Bytes(message)...)
	payload = append(payload, latin1Bytes(displayName)...)
	payload = append(payload, latin1Bytes(token)...)
	return mustEncode(ServerLoginResponse, payload)
}

// EncodeRebuildNormal encodes the REBUILD_NORMAL packet (opcode 141,
// 2-byte length prefix): regionX u16, regionY u16, a reload byte that is 0
// when forceReload is set and 1 otherwise, the XTEA key count u16, then
// each key as 4 big-endian int32s (encodeRebuildNormal, ClientProtocol.ts
// lines 1819-1827). Note the inverted reload byte: it differs from
// EncodeRebuildRegion, where the byte is 1 when forceReload is set.
func EncodeRebuildNormal(regionX, regionY uint16, forceReload bool, xteaKeys [][4]int32) ([]byte, error) {
	payload := make([]byte, 7+len(xteaKeys)*16)
	binary.BigEndian.PutUint16(payload[0:2], regionX)
	binary.BigEndian.PutUint16(payload[2:4], regionY)
	if !forceReload {
		payload[4] = 1
	}
	binary.BigEndian.PutUint16(payload[5:7], uint16(len(xteaKeys)))
	for i, key := range xteaKeys {
		for j, value := range key {
			binary.BigEndian.PutUint32(payload[7+i*16+j*4:], uint32(value))
		}
	}
	return EncodePacket(ServerRebuildNormal, payload)
}

// EncodeRebuildRegion encodes the REBUILD_REGION packet (opcode 140, 2-byte
// length prefix) for a dynamic scene built from template chunks
// (encodeRebuildRegion, ClientProtocol.ts lines 1830-1863).
//
// Layout: regionY u16, forceReload byte (1 when set), regionX u16, XTEA key
// count u16 — note regionY precedes regionX, unlike EncodeRebuildNormal —
// then a bit-packed 4x13x13 chunk palette starting at bit offset 56 (one
// presence bit per chunk in plane/x/y order, plus 26 bits per present
// chunk; -1 means absent), then each XTEA key as 4 big-endian int32s.
func EncodeRebuildRegion(regionX, regionY uint16, forceReload bool, templateChunks [4][13][13]int32, xteaKeys [][4]int32) ([]byte, error) {
	const chunkCount = 4 * 13 * 13
	present := 0
	for plane := 0; plane < 4; plane++ {
		for x := 0; x < 13; x++ {
			for y := 0; y < 13; y++ {
				if templateChunks[plane][x][y] != -1 {
					present++
				}
			}
		}
	}
	bitBytes := (chunkCount + present*26 + 7) / 8
	payload := make([]byte, 7+bitBytes+len(xteaKeys)*16)
	binary.BigEndian.PutUint16(payload[0:2], regionY)
	if forceReload {
		payload[2] = 1
	}
	binary.BigEndian.PutUint16(payload[3:5], regionX)
	binary.BigEndian.PutUint16(payload[5:7], uint16(len(xteaKeys)))

	bitOffset := 56
	writeBits := func(value uint32, count int) {
		for bit := count - 1; bit >= 0; bit-- {
			if (value>>uint(bit))&1 != 0 {
				payload[bitOffset>>3] |= 1 << (7 - (bitOffset & 7))
			}
			bitOffset++
		}
	}
	for plane := 0; plane < 4; plane++ {
		for x := 0; x < 13; x++ {
			for y := 0; y < 13; y++ {
				chunk := templateChunks[plane][x][y]
				if chunk == -1 {
					writeBits(0, 1)
				} else {
					writeBits(1, 1)
					writeBits(uint32(chunk), 26)
				}
			}
		}
	}
	offset := 7 + bitBytes
	for _, key := range xteaKeys {
		for j, value := range key {
			binary.BigEndian.PutUint32(payload[offset+j*4:], uint32(value))
		}
		offset += 16
	}
	return EncodePacket(ServerRebuildRegion, payload)
}
