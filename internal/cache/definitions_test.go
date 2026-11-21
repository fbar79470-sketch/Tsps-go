package cache

import (
	"errors"
	"testing"
)

func TestDetectCacheType(t *testing.T) {
	cases := []struct {
		info CacheInfo
		want CacheType
	}{
		{CacheInfo{Game: GameClassic, Revision: 1}, CacheTypeClassic},
		{CacheInfo{Game: GameRunescape, Revision: 100}, CacheTypeLegacy},
		{CacheInfo{Game: GameRunescape, Revision: 233}, CacheTypeLegacy},
		{CacheInfo{Game: GameRunescape, Revision: 234}, CacheTypeDat},
		{CacheInfo{Game: GameRunescape, Revision: 409}, CacheTypeDat},
		{CacheInfo{Game: GameRunescape, Revision: 410}, CacheTypeDat2},
		{CacheInfo{Game: GameOldschool, Revision: 1}, CacheTypeDat2},
		{CacheInfo{Game: GameOldschool, Revision: 242}, CacheTypeDat2},
	}
	for _, tc := range cases {
		got, err := DetectCacheType(tc.info)
		if err != nil || got != tc.want {
			t.Errorf("DetectCacheType(%+v) = %q, %v; want %q", tc.info, got, err, tc.want)
		}
	}
	if _, err := DetectCacheType(CacheInfo{Game: "bogus"}); err == nil {
		t.Error("unknown game detected, want error")
	}
}

func TestReadParamsMap(t *testing.T) {
	// Two entries: key 100 -> "hello" (string flag), key 200 -> -42.
	data := (&enc{}).u8(2).u8(1).medium(100).str("hello").u8(0).medium(200).i32(-42).done()
	params, err := ReadParamsMap(NewByteBuffer(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	if s, ok := ParamString(params, 100); !ok || s != "hello" {
		t.Errorf("param 100 = %q, %v", s, ok)
	}
	if n, ok := ParamInt(params, 200); !ok || n != -42 {
		t.Errorf("param 200 = %d, %v", n, ok)
	}
	if _, ok := ParamInt(params, 100); ok {
		t.Error("string param read as int")
	}
	if _, ok := ParamString(params, 200); ok {
		t.Error("int param read as string")
	}
	if _, ok := ParamInt(params, 999); ok {
		t.Error("missing param reported present")
	}
	// Merging into an existing map keeps old entries.
	data2 := (&enc{}).u8(1).u8(0).medium(300).i32(7).done()
	merged, err := ReadParamsMap(NewByteBuffer(data2), params)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged) != 3 {
		t.Errorf("merged params = %d entries, want 3", len(merged))
	}
	// Zero entries yields an empty map.
	empty, err := ReadParamsMap(NewByteBuffer([]byte{0}), nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("empty params = %v, %v", empty, err)
	}
	// Truncated block errors.
	if _, err := ReadParamsMap(NewByteBuffer([]byte{1, 1}), nil); !errors.Is(err, ErrBufferOverflow) {
		t.Errorf("truncated params error = %v, want buffer overflow", err)
	}
}

func TestDecodeLoopDat2Tolerance(t *testing.T) {
	// Opcode 2 (name) with an unterminated string: mid-opcode EOF
	// stops the loop cleanly for dat2 caches.
	obj := NewObjType(1, testInfo(242))
	if err := obj.Decode([]byte{2, 'A', 'B'}); err != nil {
		t.Errorf("dat2 truncated decode = %v, want nil", err)
	}
	if obj.Name != "null" {
		t.Errorf("name = %q after truncated opcode", obj.Name)
	}
	// Empty file decodes to defaults.
	obj2 := NewObjType(2, testInfo(242))
	if err := obj2.Decode(nil); err != nil {
		t.Errorf("empty decode = %v, want nil", err)
	}
	// No terminator but complete opcodes: EOF is an implicit terminator.
	obj3 := NewObjType(3, testInfo(242))
	if err := obj3.Decode((&enc{}).u8(2).str("Sword").done()); err != nil {
		t.Fatal(err)
	}
	if obj3.Name != "Sword" {
		t.Errorf("name = %q", obj3.Name)
	}
}

func TestDecodeLoopNonDat2Strict(t *testing.T) {
	// A runescape rev-300 (dat) cache throws on EOF without a terminator.
	// Non-dat2 caches terminate strings with 0x0a, not 0.
	obj := NewObjType(1, CacheInfo{Game: GameRunescape, Revision: 300})
	if err := obj.Decode([]byte{2, 'A', 0x0a}); !errors.Is(err, ErrBufferOverflow) {
		t.Errorf("non-dat2 unterminated decode = %v, want buffer overflow", err)
	}
	obj2 := NewObjType(2, CacheInfo{Game: GameRunescape, Revision: 300})
	if err := obj2.Decode((&enc{}).u8(2).bytes('S', 'w', 'o', 'r', 'd', 0x0a).u8(0).done()); err != nil {
		t.Errorf("terminated decode = %v, want nil", err)
	}
	if obj2.Name != "Sword" {
		t.Errorf("non-dat2 name = %q", obj2.Name)
	}
}

func TestDecodeLoopUnknownOpcode(t *testing.T) {
	obj := NewObjType(1, testInfo(242))
	err := obj.Decode([]byte{250, 0})
	if err == nil {
		t.Fatal("unknown opcode decoded, want error")
	}
	// Fields decoded before the bad opcode are retained (partial type).
	obj2 := NewObjType(2, testInfo(242))
	_ = obj2.Decode((&enc{}).u8(2).str("Kept").u8(250).u8(0).done())
	if obj2.Name != "Kept" {
		t.Errorf("name = %q, want partial decode retained", obj2.Name)
	}
}

func TestTypeCacheLRU(t *testing.T) {
	c := newTypeCache[string](2)
	c.put(1, "a")
	c.put(2, "b")
	if got, ok := c.get(1); !ok || got != "a" {
		t.Errorf("get(1) = %q, %v", got, ok)
	}
	c.put(3, "c") // evicts 2: 1 was touched most recently
	if _, ok := c.get(2); ok {
		t.Error("evicted entry still present")
	}
	if _, ok := c.get(1); !ok {
		t.Error("touched entry evicted")
	}
	c.clear()
	if _, ok := c.get(1); ok {
		t.Error("clear left entries")
	}
}

func TestArchiveTypeLoader(t *testing.T) {
	info := testInfo(242)
	archive := buildTestArchive(t, 10, map[int][]byte{
		0: (&enc{}).u8(2).str("Zero").u8(0).done(),
		5: (&enc{}).u8(2).str("Five").u8(0).done(),
	}, nil)
	loader := NewArchiveObjLoader(info, archive)
	// Count is one past the highest file id, not the file count.
	if got := loader.Count(); got != 6 {
		t.Errorf("Count = %d, want 6 (last file id 5 + 1)", got)
	}
	first := loader.Load(5)
	if first.Name != "Five" {
		t.Errorf("Load(5).Name = %q", first.Name)
	}
	if second := loader.Load(5); second != first {
		t.Error("Load did not return the cached instance")
	}
	// Unknown ids yield defaults.
	if got := loader.Load(3); got.Name != "null" {
		t.Errorf("Load(3).Name = %q, want default", got.Name)
	}
	loader.ClearCache()
	if got := loader.Load(5); got == first {
		t.Error("ClearCache did not drop the cached instance")
	}
	if loader.Archive() != archive {
		t.Error("Archive() mismatch")
	}
}

func TestArchiveTypeLoaderDecodeErrorCached(t *testing.T) {
	// A file whose opcode stream hits an unknown opcode yields a
	// partial type, cached like any other (BaseTypeLoader catches).
	info := testInfo(242)
	archive := buildTestArchive(t, 10, map[int][]byte{
		0: (&enc{}).u8(2).str("Partial").u8(250).u8(0).done(),
	}, nil)
	loader := NewArchiveObjLoader(info, archive)
	got := loader.Load(0)
	if got.Name != "Partial" {
		t.Errorf("partial Load Name = %q", got.Name)
	}
	if loader.Load(0) != got {
		t.Error("partial type not cached")
	}
}

func TestIndexTypeLoader(t *testing.T) {
	info := testInfo(242)
	arch0 := buildTestArchive(t, 0, map[int][]byte{
		0: (&enc{}).u8(2).str("a0").u8(0).done(),
		1: (&enc{}).u8(2).str("a1").u8(0).done(),
	}, nil)
	arch1 := buildTestArchive(t, 1, map[int][]byte{
		0: (&enc{}).u8(2).str("b0").u8(0).done(),
		1: (&enc{}).u8(2).str("b1").u8(0).done(),
		2: (&enc{}).u8(2).str("b2").u8(0).done(),
	}, nil)
	source := newFakeSource(map[int]*Archive{0: arch0, 1: arch1})
	loader := NewIndexTypeLoader(NewObjType, info, source, 2)
	// count = (2-1)*4 + fileCount(archive 1) = 4+3 = 7.
	if got := loader.Count(); got != 7 {
		t.Errorf("Count = %d, want 7", got)
	}
	// id 5 -> archive 1, file 1.
	if got := loader.Load(5); got.Name != "b1" {
		t.Errorf("Load(5).Name = %q, want b1", got.Name)
	}
	// id 1 -> archive 0, file 1.
	if got := loader.Load(1); got.Name != "a1" {
		t.Errorf("Load(1).Name = %q, want a1", got.Name)
	}
	// Missing archive yields a default.
	if got := loader.Load(100); got.Name != "null" {
		t.Errorf("Load(100).Name = %q, want default", got.Name)
	}
	if loader.Load(5).Name != "b1" {
		t.Error("cached load failed")
	}
	loader.ClearCache()
	if got := loader.Load(5); got.Name != "b1" {
		t.Errorf("Load after ClearCache = %q", got.Name)
	}
}

func TestDummyTypeLoader(t *testing.T) {
	loader := NewDummyBasLoader(testInfo(242))
	a, b := loader.Load(3), loader.Load(3)
	if a == b {
		t.Error("dummy loader returned the same instance twice")
	}
	if loader.Count() != 0 {
		t.Errorf("Count = %d, want 0", loader.Count())
	}
	loader.ClearCache() // no-op, must not panic
}

func TestDatTypeLoader(t *testing.T) {
	info := testInfo(242)
	dat := (&enc{}).u16(2).
		u8(1).u16(100).u8(3).u8(7).u8(0).
		u8(1).u16(200).u8(0).u8(4).u8(0).done()
	config := buildTestArchive(t, 2, map[int][]byte{0: dat}, map[int]string{0: "varbit.dat"})
	loader, err := LoadDatVarBitTypes(info, config)
	if err != nil {
		t.Fatal(err)
	}
	if loader.Count() != 2 {
		t.Errorf("Count = %d, want 2", loader.Count())
	}
	v0 := loader.Load(0)
	if v0.BaseVar != 100 || v0.StartBit != 3 || v0.EndBit != 7 || !v0.Defined() {
		t.Errorf("Load(0) = %+v", v0)
	}
	v1 := loader.Load(1)
	if v1.BaseVar != 200 || v1.StartBit != 0 || v1.EndBit != 4 {
		t.Errorf("Load(1) = %+v", v1)
	}
	if loader.Load(2) != nil {
		t.Error("out-of-range load should be nil")
	}
	// Missing .dat file errors.
	empty := buildTestArchive(t, 2, map[int][]byte{}, nil)
	if _, err := LoadDatVarBitTypes(info, empty); err == nil {
		t.Error("missing varbit.dat loaded, want error")
	}
}

func TestIndexedDatTypeLoader(t *testing.T) {
	info := testInfo(242)
	obj0 := (&enc{}).u8(2).str("First").u8(0).done()
	obj1 := (&enc{}).u8(2).str("Second").u8(0).done()
	// The .dat file carries a 2-byte header: TypeScript seeds the
	// first data offset from the index buffer's offset (2) after
	// reading the count (TypeLoader.ts:221).
	dat := append([]byte{0, 2}, obj0...)
	dat = append(dat, obj1...)
	idx := (&enc{}).u16(2).u16(len(obj0)).u16(len(obj1)).done()
	config := buildTestArchive(t, 2,
		map[int][]byte{0: dat, 1: idx},
		map[int]string{0: "obj.dat", 1: "obj.idx"})
	loader, err := LoadDatObjTypes(info, config)
	if err != nil {
		t.Fatal(err)
	}
	if loader.Count() != 2 {
		t.Errorf("Count = %d, want 2", loader.Count())
	}
	if got := loader.Load(0); got.Name != "First" {
		t.Errorf("Load(0).Name = %q", got.Name)
	}
	second := loader.Load(1)
	if second.Name != "Second" {
		t.Errorf("Load(1).Name = %q", second.Name)
	}
	// Out-of-range ids yield defaults.
	if got := loader.Load(9); got.Name != "null" {
		t.Errorf("Load(9).Name = %q", got.Name)
	}
	// ClearCache is a deliberate no-op override: the cached
	// instance survives.
	loader.ClearCache()
	if loader.Load(1) != second {
		t.Error("IndexedDat ClearCache cleared the cache, want no-op")
	}
	// Missing .idx errors.
	onlyDat := buildTestArchive(t, 2, map[int][]byte{0: dat}, map[int]string{0: "obj.dat"})
	if _, err := LoadDatObjTypes(info, onlyDat); err == nil {
		t.Error("missing obj.idx loaded, want error")
	}
}

func TestCacheInfoForActive(t *testing.T) {
	active := &ActiveCache{Name: "osrs-242_2026-11-03", Revision: 242, Directory: "/x", Timestamp: "2026-11-03T00:00:00Z"}
	info := CacheInfoForActive(active)
	if info.Name != active.Name || info.Game != GameOldschool || info.Environment != "live" ||
		info.Revision != 242 || info.Timestamp != active.Timestamp || info.Size != 0 {
		t.Errorf("CacheInfoForActive = %+v", info)
	}
}
