package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeDestination(t *testing.T) {
	// DESTINATION (opcode 87) is a fixed-length-4 packet, so the
	// framed form is opcode + 4 payload bytes with no length prefix
	// (EncodePacket's fixed-length branch, framing.go).
	if ServerDestination != 87 {
		t.Fatalf("ServerDestination = %d, want 87", ServerDestination)
	}
	if n, ok := ServerPacketLength(ServerDestination); !ok || n != 4 {
		t.Fatalf("ServerPacketLength(DESTINATION) = %d, %v, want 4, true", n, ok)
	}
	// Hand-computed from the TS layout (encodeDestination,
	// ClientProtocol.ts:2214-2218): UInt16BE x then UInt16BE y, both
	// masked to 16 bits (x & 0xffff, y & 0xffff).
	cases := []struct {
		name string
		x, y int
		want []byte
	}{
		{"zero", 0, 0, []byte{87, 0, 0, 0, 0}},
		// 3089 = 0x0C11, 3524 = 0x0DC4.
		{"lumbridge tile", 3089, 3524, []byte{87, 0x0C, 0x11, 0x0D, 0xC4}},
		{"max u16", 65535, 65535, []byte{87, 0xFF, 0xFF, 0xFF, 0xFF}},
		// 65536 & 0xffff = 0, 65537 & 0xffff = 1.
		{"overflow masks", 65536, 65537, []byte{87, 0, 0, 0, 1}},
		// -1 & 0xffff = 0xFFFF, -2 & 0xffff = 0xFFFE.
		{"negative masks", -1, -2, []byte{87, 0xFF, 0xFF, 0xFF, 0xFE}},
		// 131071 = 0x1FFFF masks to 0xFFFF; -65536 masks to 0.
		{"large and negative", 131071, -65536, []byte{87, 0xFF, 0xFF, 0, 0}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := EncodeDestination(c.x, c.y); !bytes.Equal(got, c.want) {
				t.Fatalf("EncodeDestination(%d, %d) = %x, want %x", c.x, c.y, got, c.want)
			}
		})
	}
}
