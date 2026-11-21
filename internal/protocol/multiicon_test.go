package protocol

import (
	"bytes"
	"testing"
)

// EncodeMultiIcon is EncodeVarbit(4605, 0 or 1): opcode 42,
// id 4605 = 0x11FD, value as int32 (PacketSender.ts:119,
// 774-776; ClientProtocol.ts:1413-1418).
func TestEncodeMultiIcon(t *testing.T) {
	cases := []struct {
		value int
		want  []byte
	}{
		{0, []byte{42, 0x11, 0xFD, 0, 0, 0, 0}},
		{1, []byte{42, 0x11, 0xFD, 0, 0, 0, 1}},
		// Any nonzero value clamps to 1 (TS: value === 0 ? 0 : 1).
		{5, []byte{42, 0x11, 0xFD, 0, 0, 0, 1}},
		{-3, []byte{42, 0x11, 0xFD, 0, 0, 0, 1}},
	}
	for _, c := range cases {
		if got := EncodeMultiIcon(c.value); !bytes.Equal(got, c.want) {
			t.Errorf("EncodeMultiIcon(%d) = %x, want %x", c.value, got, c.want)
		}
	}
	if MulticombatAreaVarbit != 4605 {
		t.Errorf("MulticombatAreaVarbit = %d, want 4605", MulticombatAreaVarbit)
	}
}
