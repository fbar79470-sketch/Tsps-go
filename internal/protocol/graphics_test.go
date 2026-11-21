package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeTileSpotAnim(t *testing.T) {
	got := EncodeTileSpotAnim(100, 3200, 3201, 1, 50, 5)
	// Opcode 83, length 11: id, target type 2, x, y, level,
	// height, delay.
	want := []byte{83, 11, 0, 100, 2, 0x0c, 0x80, 0x0c, 0x81, 1, 50, 0, 5}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeTileSpotAnim = %x, want %x", got, want)
	}
}

func TestEncodeTileSpotAnimMasks(t *testing.T) {
	got := EncodeTileSpotAnim(-1, -1, 0x10000, 0x101, 0x101, -1)
	want := []byte{83, 11, 0xff, 0xff, 2, 0xff, 0xff, 0, 0, 1, 1, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeTileSpotAnim masks = %x, want %x", got, want)
	}
}

func TestEncodeNpcSpotAnimationSpecific(t *testing.T) {
	got, err := EncodeNpcSpotAnimationSpecific(10, 200, 1, 100, 30)
	if err != nil {
		t.Fatal(err)
	}
	// Opcode 25, fixed 9, no length prefix.
	want := []byte{25, 0, 10, 0, 200, 1, 0, 100, 0, 30}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeNpcSpotAnimationSpecific = %x, want %x", got, want)
	}
	// -1 clears the slot, encoding as 0xffff.
	got, err = EncodeNpcSpotAnimationSpecific(0, -1, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte{25, 0, 0, 0xff, 0xff, 0, 0, 0, 0, 0}) {
		t.Errorf("clear slot = %x", got)
	}
}

func TestEncodeNpcSpotAnimationSpecificRanges(t *testing.T) {
	cases := []struct {
		name            string
		index, id, slot int
		height, delay   int
		wantErr         bool
	}{
		{"indexMax", 65534, 0, 0, 0, 0, false},
		{"indexTooHigh", 65535, 0, 0, 0, 0, true},
		{"indexNegative", -1, 0, 0, 0, 0, true},
		{"idMax", 0, 65534, 0, 0, 0, false},
		{"idTooHigh", 0, 65535, 0, 0, 0, true},
		{"idTooLow", 0, -2, 0, 0, 0, true},
		{"slotMax", 0, 0, 255, 0, 0, false},
		{"slotTooHigh", 0, 0, 256, 0, 0, true},
		{"heightMax", 0, 0, 0, 65535, 0, false},
		{"heightTooHigh", 0, 0, 0, 65536, 0, true},
		{"delayMax", 0, 0, 0, 0, 65535, false},
		{"delayNegative", 0, 0, 0, 0, -1, true},
	}
	for _, tc := range cases {
		_, err := EncodeNpcSpotAnimationSpecific(tc.index, tc.id, tc.slot, tc.height, tc.delay)
		if tc.wantErr && err == nil {
			t.Errorf("%s: want error, got none", tc.name)
		}
		if !tc.wantErr && err != nil {
			t.Errorf("%s: want no error, got %v", tc.name, err)
		}
	}
	if _, err := EncodeNpcSpotAnimationSpecific(65535, 0, 0, 0, 0); err == nil || err.Error() != "NPC index outside 0..65534" {
		t.Errorf("index error = %v, want TS-shaped message", err)
	}
}
