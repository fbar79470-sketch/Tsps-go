package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeLocAddChangeByteExact(t *testing.T) {
	// Id 1234 (0x04d2) at absolute tile (3200, 3201), level 1,
	// shape 10, rotation 2: opcode 134, 1-byte length prefix 8,
	// then id/x/y u16s, level, and (10<<2)|2 = 42.
	want := []byte{134, 8, 0x04, 0xd2, 0x0c, 0x80, 0x0c, 0x81, 1, 42}
	if got := EncodeLocAddChange(1234, 3200, 3201, 1, 10, 2); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocAddChange = %v, want %v", got, want)
	}
}

func TestEncodeLocAddChangeMasking(t *testing.T) {
	// TS masks id/x/y with & 0xffff and rotation with & 3; the
	// shape shift truncates through the byte assignment.
	// Id 0x10005 -> 0x0005, x -1 -> 0xffff, rotation 5 -> 1,
	// shape 100 -> byte((100<<2)|1) = byte(401) = 145.
	want := []byte{134, 8, 0x00, 0x05, 0xff, 0xff, 0x00, 0x00, 0, 145}
	if got := EncodeLocAddChange(0x10005, -1, 0, 0, 100, 5); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocAddChange masked = %v, want %v", got, want)
	}
	// Level truncates to a byte, as the Buffer assignment does.
	got := EncodeLocAddChange(1, 0, 0, 0x101, 0, 0)
	if got[8] != 1 {
		t.Fatalf("EncodeLocAddChange level byte = %d, want 1", got[8])
	}
}

func TestEncodeLocAddChangeWithOpFlags(t *testing.T) {
	// The 9-byte form: the 8-byte layout plus opFlags masked
	// to 8 bits (0x105 -> 0x05: op1 and op3 shown).
	want := []byte{134, 9, 0x00, 0x01, 0x00, 0x02, 0x00, 0x03, 0, 4, 0x05}
	if got := EncodeLocAddChangeWithOpFlags(1, 2, 3, 0, 1, 0, 0x105); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocAddChangeWithOpFlags = %v, want %v", got, want)
	}
}

func TestEncodeLocDelByteExact(t *testing.T) {
	// Removal at absolute tile (3200, 3201), level 2, shape 10,
	// rotation 3: opcode 135, length prefix 6, x/y u16s, level,
	// and (10<<2)|3 = 43. No id: the client clears the
	// tile/shape slot.
	want := []byte{135, 6, 0x0c, 0x80, 0x0c, 0x81, 2, 43}
	if got := EncodeLocDel(3200, 3201, 2, 10, 3); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocDel = %v, want %v", got, want)
	}
	// Masking: x wraps, rotation masks (7 & 3 = 3).
	want = []byte{135, 6, 0x00, 0x05, 0xff, 0xff, 0, 3}
	if got := EncodeLocDel(0x10005, -1, 0, 0, 7); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocDel masked = %v, want %v", got, want)
	}
}

func TestEncodeLocAnimByteExact(t *testing.T) {
	// Id 1234 at (3200, 3201), level 0, shape 10, rotation 1,
	// animation 827 (0x033b): opcode 136, fixed length 10, NO
	// length prefix.
	want := []byte{136, 0x04, 0xd2, 0x0c, 0x80, 0x0c, 0x81, 0, 41, 0x03, 0x3b}
	if got := EncodeLocAnim(1234, 3200, 3201, 0, 10, 1, 827); !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocAnim = %v, want %v", got, want)
	}
	// Animation -1 (the sendObjectAnimation no-id fallback)
	// encodes as 0xffff.
	got := EncodeLocAnim(1, 0, 0, 0, 0, 0, -1)
	if !bytes.Equal(got[9:11], []byte{0xff, 0xff}) {
		t.Fatalf("EncodeLocAnim(-1) animation bytes = %v, want [255 255]", got[9:11])
	}
}

func TestEncodeLocMergeByteExact(t *testing.T) {
	// Opcode 138, fixed length 18, no length prefix: locId,
	// tile, level, shape/rotation, playerIndex, start, end,
	// then the four signed-byte bounds.
	merge := LocMerge{
		LocID: 1234, X: 3200, Y: 3201, Level: 1, Shape: 10, Rotation: 2,
		PlayerIndex: 5, Start: 10, End: 20,
		MinX: -5, MinZ: -3, MaxX: 5, MaxZ: 3,
	}
	want := []byte{138,
		0x04, 0xd2, 0x0c, 0x80, 0x0c, 0x81, 1, 42,
		0x00, 0x05, 0x00, 0x0a, 0x00, 0x14,
		0xfb, 0xfd, 0x05, 0x03}
	got, err := EncodeLocMerge(merge)
	if err != nil {
		t.Fatalf("EncodeLocMerge: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeLocMerge = %v, want %v", got, want)
	}
}

func TestEncodeLocMergeValidation(t *testing.T) {
	base := LocMerge{
		LocID: 1, X: 3200, Y: 3200, Level: 0, Shape: 10, Rotation: 0,
		PlayerIndex: 1, Start: 0, End: 1,
		MinX: -128, MinZ: -128, MaxX: 127, MaxZ: 127,
	}
	// The boundary values themselves are valid.
	if _, err := EncodeLocMerge(base); err != nil {
		t.Fatalf("EncodeLocMerge(boundaries): %v", err)
	}
	// Reversed bounds are valid on the wire (LocMerge.ts:8).
	reversed := base
	reversed.MinX, reversed.MaxX = 10, -10
	if _, err := EncodeLocMerge(reversed); err != nil {
		t.Fatalf("EncodeLocMerge(reversed bounds): %v", err)
	}
	cases := []struct {
		name string
		mut  func(*LocMerge)
	}{
		{"locId", func(m *LocMerge) { m.LocID = 0x10000 }},
		{"x", func(m *LocMerge) { m.X = -1 }},
		{"z", func(m *LocMerge) { m.Y = 0x10000 }},
		{"level", func(m *LocMerge) { m.Level = 4 }},
		{"shape", func(m *LocMerge) { m.Shape = 32 }},
		{"rotation", func(m *LocMerge) { m.Rotation = 4 }},
		{"playerIndex", func(m *LocMerge) { m.PlayerIndex = 2048 }},
		{"start", func(m *LocMerge) { m.Start = 0x10000 }},
		{"end equals start", func(m *LocMerge) { m.End = m.Start }},
		{"end below start", func(m *LocMerge) { m.End = m.Start - 1 }},
		{"minX", func(m *LocMerge) { m.MinX = -129 }},
		{"minZ", func(m *LocMerge) { m.MinZ = 128 }},
		{"maxX", func(m *LocMerge) { m.MaxX = 128 }},
		{"maxZ", func(m *LocMerge) { m.MaxZ = -129 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := base
			tc.mut(&m)
			if got, err := EncodeLocMerge(m); err == nil {
				t.Fatalf("EncodeLocMerge(%s) = %v, want error", tc.name, got)
			}
		})
	}
}

func TestLocLengthTable(t *testing.T) {
	// Length-table entries from ServerPackets.ts:273-277:
	// ADD_CHANGE/DEL are -1 (1-byte prefix), MERGE is fixed 18,
	// ANIM is fixed 10.
	cases := []struct {
		opcode byte
		length int
	}{
		{ServerLocAddChange, -1},
		{ServerLocDel, -1},
		{ServerLocMerge, 18},
		{ServerLocAnim, 10},
	}
	for _, tc := range cases {
		length, ok := ServerPacketLength(tc.opcode)
		if !ok || length != tc.length {
			t.Fatalf("ServerPacketLength(%d) = %d, %v, want %d, true", tc.opcode, length, ok, tc.length)
		}
	}
	if ServerLocAddChange != 134 || ServerLocDel != 135 || ServerLocAnim != 136 || ServerLocMerge != 138 {
		t.Fatalf("loc opcodes = %d/%d/%d/%d, want 134/135/136/138",
			ServerLocAddChange, ServerLocDel, ServerLocAnim, ServerLocMerge)
	}
}
