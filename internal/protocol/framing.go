package protocol

import "fmt"

// Packet is one decoded client packet: its opcode and its payload bytes
// (without the opcode or length prefix). Payload is a copy owned by the
// Packet; retaining a Packet never retains the source frame.
type Packet struct {
	Opcode  byte
	Payload []byte
}

// DecodeFrame splits one WebSocket message into its concatenated packets,
// porting decodeClientPackets (ClientProtocol.ts lines 486-506). Each
// packet's payload length is asserted exactly: a packet whose declared
// length overruns the frame, a truncated header, or an unknown opcode is an
// error (in TS these throw and the connection is closed). An empty frame
// decodes to no packets.
func DecodeFrame(frame []byte) ([]Packet, error) {
	packets := []Packet{}
	offset := 0
	for offset < len(frame) {
		opcode := frame[offset]
		expected, ok := ClientPacketLength(opcode)
		if !ok {
			return nil, fmt.Errorf("protocol: unsupported client opcode %d", opcode)
		}
		var header int
		switch expected {
		case -2:
			header = 3
		case -1:
			header = 2
		default:
			header = 1
		}
		if offset+header > len(frame) {
			return nil, fmt.Errorf("protocol: truncated client opcode %d", opcode)
		}
		var length int
		switch expected {
		case -2:
			length = int(frame[offset+1])<<8 | int(frame[offset+2])
		case -1:
			length = int(frame[offset+1])
		default:
			length = expected
		}
		end := offset + header + length
		if end > len(frame) {
			return nil, fmt.Errorf("protocol: truncated client opcode %d", opcode)
		}
		payload := make([]byte, length)
		copy(payload, frame[offset+header:end])
		packets = append(packets, Packet{Opcode: opcode, Payload: payload})
		offset = end
	}
	return packets, nil
}

// EncodePacket frames one server packet (opcode + optional length prefix +
// payload), porting encodeServerPacket (ClientProtocol.ts lines 1053-1068):
// fixed-length opcodes require an exactly-sized payload, -1 opcodes reject
// payloads over 255 bytes, and -2 opcodes reject payloads over 65535 bytes.
// Unknown opcodes are an error.
func EncodePacket(opcode byte, payload []byte) ([]byte, error) {
	expected, ok := ServerPacketLength(opcode)
	if !ok {
		return nil, fmt.Errorf("protocol: unsupported server opcode %d", opcode)
	}
	if expected >= 0 {
		if len(payload) != expected {
			return nil, fmt.Errorf("protocol: invalid server packet length for opcode %d: expected %d, got %d", opcode, expected, len(payload))
		}
		out := make([]byte, 0, 1+len(payload))
		return append(append(out, opcode), payload...), nil
	}
	if expected == -1 {
		if len(payload) > 255 {
			return nil, fmt.Errorf("protocol: server packet %d exceeds byte length", opcode)
		}
		out := make([]byte, 0, 2+len(payload))
		out = append(out, opcode, byte(len(payload)))
		return append(out, payload...), nil
	}
	if len(payload) > 65535 {
		return nil, fmt.Errorf("protocol: server packet %d exceeds short length", opcode)
	}
	out := make([]byte, 0, 3+len(payload))
	out = append(out, opcode, byte(len(payload)>>8), byte(len(payload)))
	return append(out, payload...), nil
}

// mustEncode is EncodePacket for encoders whose payloads are constructed to
// always satisfy the table (mirroring TS, where encodeServerPacket/packet
// throw on violation). It panics on error; call sites document why the
// error is unreachable or what input makes it reachable.
func mustEncode(opcode byte, payload []byte) []byte {
	out, err := EncodePacket(opcode, payload)
	if err != nil {
		panic(err)
	}
	return out
}
