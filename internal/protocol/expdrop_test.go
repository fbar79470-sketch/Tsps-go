package protocol

import (
	"bytes"
	"testing"
)

// The expected bytes below are hand-computed from the TS layouts
// cited in expdrop.go, not copied from the encoder's output.

func TestEncodeExpDrop(t *testing.T) {
	// Skill client id 0 (Attack), 0 XP: opcode + id + four zero bytes.
	if got := EncodeExpDrop(0, 0); !bytes.Equal(got, []byte{116, 0, 0, 0, 0, 0}) {
		t.Errorf("EncodeExpDrop(0, 0) = %v, want [116 0 0 0 0 0]", got)
	}
	// Skill client id 3 (Hitpoints), 100 XP: 100 = 0x00000064.
	if got := EncodeExpDrop(3, 100); !bytes.Equal(got, []byte{116, 3, 0, 0, 0, 100}) {
		t.Errorf("EncodeExpDrop(3, 100) = %v, want [116 3 0 0 0 100]", got)
	}
	// The Construction/Hunter client ids, with a multi-byte XP
	// value: 0x01020304 renders big-endian.
	if got := EncodeExpDrop(22, 0x01020304); !bytes.Equal(got, []byte{116, 22, 1, 2, 3, 4}) {
		t.Errorf("EncodeExpDrop(22, 0x01020304) = %v, want [116 22 1 2 3 4]", got)
	}
	if got := EncodeExpDrop(21, 0x01020304); !bytes.Equal(got, []byte{116, 21, 1, 2, 3, 4}) {
		t.Errorf("EncodeExpDrop(21, 0x01020304) = %v, want [116 21 1 2 3 4]", got)
	}
	// The 200M cap: 200000000 = 0x0BEBC200.
	if got := EncodeExpDrop(5, 200000000); !bytes.Equal(got, []byte{116, 5, 0x0b, 0xeb, 0xc2, 0x00}) {
		t.Errorf("EncodeExpDrop(5, 200000000) = %x, want [116 5 0b eb c2 00]", got)
	}
	// PacketBuilder.put masks the id to 8 bits: 300 & 0xff = 44.
	if got := EncodeExpDrop(300, 1); !bytes.Equal(got, []byte{116, 44, 0, 0, 0, 1}) {
		t.Errorf("EncodeExpDrop(300, 1) = %v, want [116 44 0 0 0 1]", got)
	}
	// A negative value renders as int32 two's complement, as the
	// JS >> shifts in putInts do: -1 is four 0xff bytes.
	if got := EncodeExpDrop(1, -1); !bytes.Equal(got, []byte{116, 1, 0xff, 0xff, 0xff, 0xff}) {
		t.Errorf("EncodeExpDrop(1, -1) = %v, want [116 1 ff ff ff ff]", got)
	}
}

// TestEncodeExpDropNeverFramed pins why the legacy bytes must never
// be sent: opcode 116 is WIDGET_SET_ROTATE_SPEED in the native
// length table, fixed length 8, while the legacy frame is 6 bytes.
// Framing it through EncodePacket would be rejected, and sending
// the raw bytes would desync the client's frame decoder.
func TestEncodeExpDropNeverFramed(t *testing.T) {
	if length, ok := ServerPacketLength(LegacyExpDropOpcode); !ok || length != 8 {
		t.Fatalf("ServerPacketLength(116) = (%d, %v), want (8, true)", length, ok)
	}
	if got := EncodeExpDrop(0, 0); len(got) != 6 {
		t.Fatalf("len(EncodeExpDrop) = %d, want 6", len(got))
	}
	if _, err := EncodePacket(LegacyExpDropOpcode, []byte{0, 0, 0, 0, 0}); err == nil {
		t.Fatal("EncodePacket(116, 5-byte payload) succeeded, want the length-table rejection")
	}
}
