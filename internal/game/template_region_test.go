package game

import "testing"

// templateTerrainNew builds a new-format terrain stream where
// only the first tiles of plane 0 and the first tile of plane 1
// carry opcodes; every other tile is the terminator.
func templateTerrainNew() []byte {
	return buildTestTerrain(true, map[[3]int][]byte{
		{0, 0, 0}: append(tileTypeUShort(50), 0, 0),             // settings 1 (blocked)
		{0, 0, 1}: append(tileTypeUShort(51), 0, 0),             // settings 2 (bridge)
		{0, 0, 2}: {0, 1, 0x7f},                                 // opcode 1 + height byte: no settings
		{0, 0, 3}: append(tileTypeUShort(49), 0x12, 0x34, 0, 0), // opcode <=49: skip one ushort
		{0, 0, 4}: append(tileTypeUShort(82), 0, 0),             // opcode >81: no-op, then terminator
		{0, 1, 0}: append(tileTypeUShort(81), 0, 0),             // settings 32 (top of the range)
		{1, 0, 0}: append(tileTypeUShort(52), 0, 0),             // settings 3 on plane 1
	})
}

func TestDecodeTemplateRegionTerrainNewFormat(t *testing.T) {
	region := decodeTemplateRegion(templateTerrainNew(), nil, true)
	if len(region.Settings) != 4*64*64 {
		t.Fatalf("Settings length = %d, want %d", len(region.Settings), 4*64*64)
	}
	if len(region.Chunks) != 256 {
		t.Fatalf("Chunks length = %d, want 256", len(region.Chunks))
	}
	cases := []struct {
		z, x, y int
		want    uint8
	}{
		{0, 0, 0, 1},
		{0, 0, 1, 2},
		{0, 0, 2, 0}, // opcode 1 sets no settings
		{0, 0, 3, 0}, // opcode 49 skips, sets nothing
		{0, 0, 4, 0}, // opcode 82 matches no branch
		{0, 1, 0, 32},
		{1, 0, 0, 3},
		{0, 5, 5, 0}, // terminator-only tile
		{3, 63, 63, 0},
	}
	for _, c := range cases {
		if got := region.Settings[(c.z<<12)|(c.x<<6)|c.y]; got != c.want {
			t.Errorf("settings[%d,%d,%d] = %d, want %d", c.z, c.x, c.y, got, c.want)
		}
	}
}

func TestDecodeTemplateRegionTerrainOldFormat(t *testing.T) {
	terrain := buildTestTerrain(false, map[[3]int][]byte{
		{0, 0, 0}: {50, 0},       // settings 1
		{0, 0, 1}: {1, 0x7f},     // opcode 1 + height byte
		{0, 0, 2}: {49, 0xab, 0}, // opcode <=49: skip one byte
		{0, 0, 3}: {82, 0},       // opcode >81: no-op
		{0, 0, 4}: {53, 0},       // settings 4
	})
	region := decodeTemplateRegion(terrain, nil, false)
	cases := []struct {
		x, y int
		want uint8
	}{
		{0, 0, 1},
		{0, 1, 0},
		{0, 2, 0},
		{0, 3, 0},
		{0, 4, 4},
	}
	for _, c := range cases {
		if got := region.Settings[(c.x<<6)|c.y]; got != c.want {
			t.Errorf("settings[0,%d,%d] = %d, want %d", c.x, c.y, got, c.want)
		}
	}
}

func TestDecodeTemplateRegionTruncatedTerrain(t *testing.T) {
	// A truncated stream reads past the end as 0 (terminators):
	// only the tiles before the truncation carry settings.
	terrain := []byte{0, 50, 0, 0} // tile (0,0,0): opcode 50, terminator; then EOF
	region := decodeTemplateRegion(terrain, nil, true)
	if region.Settings[0] != 1 {
		t.Fatalf("settings[0] = %d, want 1", region.Settings[0])
	}
	for i, s := range region.Settings[1:] {
		if s != 0 {
			t.Fatalf("settings[%d] = %d, want 0 (EOF reads as terminator)", i+1, s)
		}
	}
	// An empty-but-present terrain array still decodes (TS: an
	// empty Uint8Array is truthy).
	empty := decodeTemplateRegion([]byte{}, nil, true)
	if empty == nil || len(empty.Settings) != 4*64*64 {
		t.Fatal("empty terrain did not decode to a zeroed region")
	}
}

func TestDecodeTemplateRegionObjects(t *testing.T) {
	// Hand-encoded object stream (see the smart encodings in the
	// test body): id 100 with two locs, id 105 with one loc.
	objects := []byte{
		101,        // idDelta 101 -> id 100
		0x82, 0x95, // packedDelta 661 -> packed 660 (plane 0, x 10, y 20)
		41,         // attributes: type 10, face 1
		0x8f, 0x75, // packedDelta 3957 -> packed 4616 (plane 1, x 8, y 8)
		3,  // attributes: type 0, face 3
		0,  // end of id-100 group
		5,  // idDelta 5 -> id 105
		1,  // packedDelta 1 -> packed 0 (plane 0, x 0, y 0)
		10, // attributes: type 2, face 2
		0,  // end of id-105 group
		0,  // end of stream
	}
	region := decodeTemplateRegion([]byte{}, objects, true)
	loc1 := TemplateLoc{ID: 100, LocalX: 10, LocalY: 20, Plane: 0, Type: 10, Face: 1}
	loc2 := TemplateLoc{ID: 100, LocalX: 8, LocalY: 8, Plane: 1, Type: 0, Face: 3}
	loc3 := TemplateLoc{ID: 105, LocalX: 0, LocalY: 0, Plane: 0, Type: 2, Face: 2}
	key1 := TemplateChunkKey(0, 1, 2)
	if got := region.Chunks[key1]; len(got) != 1 || got[0] != loc1 {
		t.Fatalf("chunks[%d] = %v, want [%v]", key1, got, loc1)
	}
	key2 := TemplateChunkKey(1, 1, 1)
	if got := region.Chunks[key2]; len(got) != 1 || got[0] != loc2 {
		t.Fatalf("chunks[%d] = %v, want [%v]", key2, got, loc2)
	}
	key3 := TemplateChunkKey(0, 0, 0)
	if got := region.Chunks[key3]; len(got) != 1 || got[0] != loc3 {
		t.Fatalf("chunks[%d] = %v, want [%v]", key3, got, loc3)
	}
	// No other chunk holds locs.
	for i, chunk := range region.Chunks {
		if i == key1 || i == key2 || i == key3 {
			continue
		}
		if len(chunk) != 0 {
			t.Fatalf("chunks[%d] = %v, want empty", i, chunk)
		}
	}
}

func TestDecodeTemplateRegionNoObjects(t *testing.T) {
	region := decodeTemplateRegion([]byte{}, nil, true)
	for i, chunk := range region.Chunks {
		if len(chunk) != 0 {
			t.Fatalf("chunks[%d] = %v, want empty with nil object data", i, chunk)
		}
	}
}

func TestTemplateChunkKey(t *testing.T) {
	cases := []struct {
		plane, cx, cy int
		want          int
	}{
		{0, 0, 0, 0},
		{3, 7, 7, 255},
		{1, 1, 1, 73},
		{0, 1, 2, 10},
		// Chunk coordinates mask to the 8 chunks of a region.
		{2, 9, 15, (2 << 6) | (1 << 3) | 7},
		{0, 8, 8, 0},
	}
	for _, c := range cases {
		if got := TemplateChunkKey(c.plane, c.cx, c.cy); got != c.want {
			t.Errorf("TemplateChunkKey(%d,%d,%d) = %d, want %d", c.plane, c.cx, c.cy, got, c.want)
		}
	}
}

func TestTemplateRegionCacheSemantics(t *testing.T) {
	defer SetTemplateRegionSource(nil, false)
	src := newFakeRegionSource([]int{100, 200})
	src.data[100] = fakeRegionData{terrain: templateTerrainNew(), objects: nil, ok: true}
	src.data[200] = fakeRegionData{terrain: nil, objects: nil, ok: true} // present, no terrain
	SetTemplateRegionSource(src, true)

	// Decode-once: two Gets, one source read, same pointer.
	first := GetTemplateRegion(100)
	second := GetTemplateRegion(100)
	if first == nil || first != second {
		t.Fatal("GetTemplateRegion did not return the cached decode")
	}
	if src.calls[100] != 1 {
		t.Fatalf("source reads for 100 = %d, want 1", src.calls[100])
	}
	if first.Settings[0] != 1 {
		t.Fatal("cached decode has wrong settings")
	}

	// Misses cache too: absent region and terrain-less region
	// answer nil without re-reading.
	if GetTemplateRegion(999) != nil || GetTemplateRegion(999) != nil {
		t.Fatal("absent region did not answer nil")
	}
	if src.calls[999] != 1 {
		t.Fatalf("source reads for absent 999 = %d, want 1 (miss cached)", src.calls[999])
	}
	if GetTemplateRegion(200) != nil || GetTemplateRegion(200) != nil {
		t.Fatal("terrain-less region did not answer nil")
	}
	if src.calls[200] != 1 {
		t.Fatalf("source reads for 200 = %d, want 1 (miss cached)", src.calls[200])
	}

	// ReadTemplateRegion decodes fresh every time and neither
	// reads nor writes the cache.
	third := ReadTemplateRegion(100)
	if third == nil || third == first {
		t.Fatal("ReadTemplateRegion returned the cached pointer")
	}
	if src.calls[100] != 2 {
		t.Fatalf("source reads for 100 = %d, want 2 after one Read", src.calls[100])
	}
	fourth := ReadTemplateRegion(100)
	if fourth == third {
		t.Fatal("ReadTemplateRegion cached its decode")
	}
	if got := GetTemplateRegion(100); got != first {
		t.Fatal("ReadTemplateRegion disturbed the cache")
	}
	if src.calls[100] != 3 {
		t.Fatalf("source reads for 100 = %d, want 3", src.calls[100])
	}

	// A source swap clears the cache: the new source is
	// consulted, and the old decode is gone.
	src2 := newFakeRegionSource([]int{100})
	src2.data[100] = fakeRegionData{terrain: []byte{}, objects: nil, ok: true}
	SetTemplateRegionSource(src2, true)
	if got := GetTemplateRegion(100); got == nil || got == first || got.Settings[0] != 0 {
		t.Fatal("source swap did not clear the cache")
	}
	if src2.calls[100] != 1 {
		t.Fatalf("new source reads = %d, want 1", src2.calls[100])
	}
}

func TestTemplateRegionNilSource(t *testing.T) {
	defer SetTemplateRegionSource(nil, false)
	SetTemplateRegionSource(nil, true)
	if GetTemplateRegion(100) != nil {
		t.Fatal("GetTemplateRegion with no source did not answer nil")
	}
	if ReadTemplateRegion(100) != nil {
		t.Fatal("ReadTemplateRegion with no source did not answer nil")
	}
}
