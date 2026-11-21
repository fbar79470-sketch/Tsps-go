package protocol

import (
	"bytes"
	"testing"
)

func TestWriteWorldEntityPosition(t *testing.T) {
	cases := []struct {
		name string
		pos  WorldEntityPosition
		want []byte
	}{
		{"allZero", WorldEntityPosition{}, []byte{0}},
		{"xByte", WorldEntityPosition{X: 1}, []byte{1, 1}},
		{"mixedWidths", WorldEntityPosition{X: 200, Y: -200, Z: 70000, Orientation: -5},
			[]byte{0x7a, 0, 200, 0xff, 0x38, 0, 1, 0x11, 0x70, 0xfb}},
		{"byteBounds", WorldEntityPosition{X: -128, Y: 127}, []byte{0x05, 0x80, 0x7f}},
		{"shortBound", WorldEntityPosition{X: 128}, []byte{0x02, 0, 128}},
		{"intBound", WorldEntityPosition{X: 32768}, []byte{0x03, 0, 0, 0x80, 0}},
		{"negativeInt", WorldEntityPosition{Z: -70000}, []byte{0x30, 0xff, 0xfe, 0xee, 0x90}},
	}
	for _, tc := range cases {
		if got := writeWorldEntityPosition(nil, tc.pos); !bytes.Equal(got, tc.want) {
			t.Errorf("position %s = %x, want %x", tc.name, got, tc.want)
		}
	}
}

func TestEncodeWorldEntityInfoEmpty(t *testing.T) {
	got, err := EncodeWorldEntityInfo(nil, nil)
	if err != nil {
		t.Fatalf("EncodeWorldEntityInfo empty: %v", err)
	}
	// Opcode 143, 1-byte length 1, count 0.
	if want := []byte{143, 1, 0}; !bytes.Equal(got, want) {
		t.Errorf("EncodeWorldEntityInfo empty = %x, want %x", got, want)
	}
}

func TestEncodeWorldEntityInfoUpdates(t *testing.T) {
	// Despawn: type byte only, no mask byte.
	got, err := EncodeWorldEntityInfo([]WorldEntityUpdate{{UpdateType: WorldEntityDespawn}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{143, 2, 1, 0}; !bytes.Equal(got, want) {
		t.Errorf("despawn = %x, want %x", got, want)
	}
	// Unchanged: type byte + 0 mask byte, no delta.
	got, err = EncodeWorldEntityInfo([]WorldEntityUpdate{{UpdateType: WorldEntityUnchanged}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{143, 3, 1, 1, 0}; !bytes.Equal(got, want) {
		t.Errorf("unchanged = %x, want %x", got, want)
	}
	// Move with a delta: type, position, mask byte.
	delta := WorldEntityPosition{X: 10}
	got, err = EncodeWorldEntityInfo([]WorldEntityUpdate{{UpdateType: WorldEntityMove, Delta: &delta}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{143, 5, 1, 2, 1, 10, 0}; !bytes.Equal(got, want) {
		t.Errorf("move = %x, want %x", got, want)
	}
	// Snap with a nil delta: zero position (flags byte only).
	got, err = EncodeWorldEntityInfo([]WorldEntityUpdate{{UpdateType: WorldEntitySnap}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := []byte{143, 4, 1, 3, 0, 0}; !bytes.Equal(got, want) {
		t.Errorf("snap nil delta = %x, want %x", got, want)
	}
}

func TestEncodeWorldEntityInfoSpawn(t *testing.T) {
	got, err := EncodeWorldEntityInfo(nil, []WorldEntitySpawn{{
		EntityIndex: 3, SizeX: 10, SizeZ: 10, ConfigID: 5, DrawMode: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []byte{143, 10, 0, 0, 3, 10, 10, 0, 5, 0, 1, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("spawn = %x, want %x", got, want)
	}
}

func TestEncodeWorldEntityInfoTooLong(t *testing.T) {
	// 200 spawns overflow the -1 length table; TS throws.
	spawns := make([]WorldEntitySpawn, 200)
	if _, err := EncodeWorldEntityInfo(nil, spawns); err == nil {
		t.Error("200 spawns: want length-table error")
	}
}

func emptyChunks() [4][13][13]int32 {
	var chunks [4][13][13]int32
	for p := range chunks {
		for x := range chunks[p] {
			for y := range chunks[p][x] {
				chunks[p][x][y] = -1
			}
		}
	}
	return chunks
}

func TestEncodeRebuildWorldEntityEmpty(t *testing.T) {
	got, err := EncodeRebuildWorldEntity(1, 2, 13, 13, 100, 200, emptyChunks(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Header 18 bytes (note regionY twice, regionX twice),
	// then 85 zero bit bytes for 676 absent chunks.
	if len(got) != 3+18+85 {
		t.Fatalf("rebuild length = %d, want %d", len(got), 3+18+85)
	}
	wantHeader := []byte{142, 0, 103, 0, 1, 0, 2, 13, 13, 0, 100, 0, 200, 0, 200, 0, 0, 100, 0, 0, 0}
	if !bytes.Equal(got[:len(wantHeader)], wantHeader) {
		t.Errorf("rebuild header = %x, want %x", got[:len(wantHeader)], wantHeader)
	}
	for i, b := range got[len(wantHeader):] {
		if b != 0 {
			t.Errorf("bit byte %d = %d, want 0", i, b)
		}
	}
}

func TestEncodeRebuildWorldEntityOneChunk(t *testing.T) {
	chunks := emptyChunks()
	chunks[0][0][0] = 0
	got, err := EncodeRebuildWorldEntity(1, 2, 13, 13, 100, 200, chunks, nil)
	if err != nil {
		t.Fatal(err)
	}
	// 676 presence bits + 26 chunk bits = 702 bits = 88 bytes.
	if len(got) != 3+18+88 {
		t.Fatalf("rebuild length = %d, want %d", len(got), 3+18+88)
	}
	// First bit set (chunk present), next 26 zero.
	if got[21] != 0x80 {
		t.Errorf("first bit byte = %x, want 80", got[21])
	}
}

func TestEncodeRebuildWorldEntityPaletteMatchesRebuildRegion(t *testing.T) {
	chunks := emptyChunks()
	chunks[0][0][0] = 0x1234567 & 0x3ffffff
	chunks[3][12][12] = 1
	chunks[1][5][7] = 0x2aaaaaa
	keys := [][4]int32{{1, 2, 3, 4}}
	entity, err := EncodeRebuildWorldEntity(9, 8, 13, 13, 50, 60, chunks, keys)
	if err != nil {
		t.Fatal(err)
	}
	region, err := EncodeRebuildRegion(50, 60, false, chunks, keys)
	if err != nil {
		t.Fatal(err)
	}
	// The bit sections must be identical: 676 + 3*26 = 754
	// bits = 95 bytes (754/8 = 94.25 -> 95).
	entityBits := entity[3+18 : 3+18+95]
	regionBits := region[3+7 : 3+7+95]
	if !bytes.Equal(entityBits, regionBits) {
		t.Errorf("palette bits differ:\nentity %x\nregion %x", entityBits, regionBits)
	}
	// XTEA keys land after the bits in both.
	if !bytes.Equal(entity[len(entity)-16:], region[len(region)-16:]) {
		t.Errorf("key tails differ: %x vs %x", entity[len(entity)-16:], region[len(region)-16:])
	}
}

func TestEncodeRebuildWorldEntityKeys(t *testing.T) {
	keys := [][4]int32{{0x01020304, -1, 0, 0x7fffffff}}
	got, err := EncodeRebuildWorldEntity(0, 0, 1, 1, 0, 0, emptyChunks(), keys)
	if err != nil {
		t.Fatal(err)
	}
	tail := got[len(got)-16:]
	want := []byte{1, 2, 3, 4, 0xff, 0xff, 0xff, 0xff, 0, 0, 0, 0, 0x7f, 0xff, 0xff, 0xff}
	if !bytes.Equal(tail, want) {
		t.Errorf("key tail = %x, want %x", tail, want)
	}
	// Key count field at framed offsets 18..19 (payload
	// offsets 15..16, after the second regionX): count 1.
	if got[18] != 0 || got[19] != 1 {
		t.Errorf("key count bytes = %x %x, want 00 01", got[18], got[19])
	}
}
