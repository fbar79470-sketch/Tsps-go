package game

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// buildPackBytes assembles a .pack file: the 28-byte header plus
// the object and terrain sections (passed through as given —
// callers gzip them when testing compressed sections).
func buildPackBytes(regionID, objectFile, terrainFile int, objects, terrain []byte) []byte {
	pack := make([]byte, 28+len(objects)+len(terrain))
	binary.BigEndian.PutUint32(pack[0:4], 1)
	binary.BigEndian.PutUint32(pack[4:8], uint32(int32(objectFile)))
	binary.BigEndian.PutUint32(pack[8:12], uint32(int32(terrainFile)))
	binary.BigEndian.PutUint32(pack[12:16], uint32(regionID>>8))
	binary.BigEndian.PutUint32(pack[16:20], uint32(regionID&0xff))
	binary.BigEndian.PutUint32(pack[20:24], uint32(len(objects)))
	copy(pack[24:], objects)
	binary.BigEndian.PutUint32(pack[24+len(objects):], uint32(len(terrain)))
	copy(pack[28+len(objects):], terrain)
	return pack
}

func TestBuildRegionReplacementPacketUncompressed(t *testing.T) {
	packet, err := BuildRegionReplacementPacket(0x1234, true, []byte{1, 2, 3}, []byte{4, 5})
	if err != nil {
		t.Fatal(err)
	}
	if packet.Opcode != RegionReplacementOpcode {
		t.Fatalf("opcode = %d, want %d", packet.Opcode, RegionReplacementOpcode)
	}
	want := []byte{0x12, 0x34, 1, 0, 3, 0, 2, 1, 2, 3, 4, 5}
	if !bytes.Equal(packet.Payload, want) {
		t.Fatalf("payload = %v, want %v", packet.Payload, want)
	}
	// allowReload=false clears the flag byte; terrain-only
	// writes a zero object length.
	packet, err = BuildRegionReplacementPacket(257, false, []byte{9}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want = []byte{0x01, 0x01, 0, 0, 1, 0, 0, 9}
	if !bytes.Equal(packet.Payload, want) {
		t.Fatalf("terrain-only payload = %v, want %v", packet.Payload, want)
	}
}

func TestRegionReplacementPacketFrame(t *testing.T) {
	packet, err := BuildRegionReplacementPacket(257, true, []byte{1}, []byte{2})
	if err != nil {
		t.Fatal(err)
	}
	// Matching flag: the stored payload is returned as-is.
	_, same := packet.Frame(true)
	if len(same) == 0 || &same[0] != &packet.Payload[0] {
		t.Fatal("Frame with a matching flag copied the payload")
	}
	// Differing flag: a copy with the flag flipped; the stored
	// payload is untouched.
	_, flipped := packet.Frame(false)
	if flipped[2] != 0 {
		t.Fatalf("flipped flag = %d, want 0", flipped[2])
	}
	if packet.Payload[2] != 1 {
		t.Fatal("Frame mutated the stored payload")
	}
	if &flipped[0] == &packet.Payload[0] {
		t.Fatal("Frame with a differing flag aliased the payload")
	}
}

func TestBuildRegionReplacementPacketCompressed(t *testing.T) {
	terrain := make([]byte, 70000) // zeros: deflates well
	objects := []byte{1, 2, 3}
	packet, err := BuildRegionReplacementPacket(257, true, terrain, objects)
	if err != nil {
		t.Fatal(err)
	}
	if packet.Opcode != RegionReplacementCompressedOpcode {
		t.Fatalf("opcode = %d, want %d", packet.Opcode, RegionReplacementCompressedOpcode)
	}
	if len(packet.Payload) <= 11 {
		t.Fatalf("payload length = %d, want a header plus deflate data", len(packet.Payload))
	}
	if packet.Payload[0] != 0x01 || packet.Payload[1] != 0x01 || packet.Payload[2] != 1 {
		t.Fatalf("header = %v, want region 257 + flag 1", packet.Payload[:3])
	}
	if got := binary.BigEndian.Uint32(packet.Payload[3:7]); got != uint32(len(terrain)) {
		t.Fatalf("raw terrain length = %d, want %d", got, len(terrain))
	}
	if got := binary.BigEndian.Uint32(packet.Payload[7:11]); got != uint32(len(objects)) {
		t.Fatalf("raw object length = %d, want %d", got, len(objects))
	}
	// The deflate body inflates to terrain+objects. Byte
	// exactness of the deflate stream is NOT asserted: Go's
	// flate need not match node's zlib byte for byte.
	reader, err := zlib.NewReader(bytes.NewReader(packet.Payload[11:]))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	want := append(append([]byte(nil), terrain...), objects...)
	if !bytes.Equal(raw, want) {
		t.Fatal("inflated body does not round-trip terrain+objects")
	}
}

func TestBuildRegionReplacementPacketErrors(t *testing.T) {
	if _, err := BuildRegionReplacementPacket(257, true, nil, nil); err == nil || !strings.Contains(err.Error(), "invalid section lengths") {
		t.Fatalf("empty terrain err = %v, want invalid section lengths", err)
	}
	huge := make([]byte, 16*1024*1024+1)
	if _, err := BuildRegionReplacementPacket(257, true, huge, nil); err == nil || !strings.Contains(err.Error(), "invalid section lengths") {
		t.Fatalf("oversize raw err = %v, want invalid section lengths", err)
	}
	// Incompressible data that stays over the payload cap even
	// deflated: deterministic pseudo-random bytes.
	random := make([]byte, 70000)
	state := uint32(1)
	for i := range random {
		state = state*1664525 + 1013904223
		random[i] = byte(state >> 24)
	}
	if _, err := BuildRegionReplacementPacket(257, true, random, nil); err == nil || !strings.Contains(err.Error(), "too large to stream") {
		t.Fatalf("incompressible err = %v, want too large to stream", err)
	}
}

func TestMapRegionReplacementRegisterAndGet(t *testing.T) {
	mgr := NewMapRegionReplacementManager()
	terrain := []byte{1, 2, 3}
	objects := buildTemplateObjectStream([]objectGroup{
		{100, [][2]int{{packTemplateLoc(0, 0, 0), 41}, {packTemplateLoc(0, 1, 0), 3}}},
		{105, [][2]int{{packTemplateLoc(0, 0, 0), 10}}},
	})
	result, err := mgr.ReplaceMapRegionData(257, terrain, objects, "my-plugin")
	if err != nil {
		t.Fatal(err)
	}
	if result.RegionID != 257 || result.Source != "my-plugin" || result.TerrainBytes != 3 ||
		result.ObjectBytes != len(objects) || result.ObjectCount != 3 {
		t.Fatalf("result = %+v, want region 257, 3 terrain bytes, 3 placements", result)
	}
	gotTerrain, gotObjects, ok := mgr.GetReplacementMapData(257)
	if !ok || !bytes.Equal(gotTerrain, terrain) || !bytes.Equal(gotObjects, objects) {
		t.Fatal("GetReplacementMapData did not return the stored data")
	}
	// Copies: mutating the returned slices does not affect the
	// stored data.
	gotTerrain[0] = 99
	gotObjects[0] = 99
	again, _, _ := mgr.GetReplacementMapData(257)
	if again[0] != 1 {
		t.Fatal("GetReplacementMapData exposed the stored slice")
	}
	// The installer's own slices are copied too.
	terrain[0] = 88
	again, _, _ = mgr.GetReplacementMapData(257)
	if again[0] != 1 {
		t.Fatal("register aliased the caller's terrain slice")
	}
	if src, ok := mgr.GetSource(257); !ok || src != "my-plugin" {
		t.Fatalf("GetSource = (%q, %v)", src, ok)
	}
	if _, ok := mgr.GetSource(999); ok {
		t.Fatal("GetSource true for an unreplaced region")
	}

	// A second install overwrites, and ids sort ascending.
	if _, err := mgr.ReplaceMapRegionData(100, []byte{7}, nil, "other"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReplaceMapRegionData(257, []byte{8}, nil, "second"); err != nil {
		t.Fatal(err)
	}
	if ids := mgr.GetRegionIDs(); len(ids) != 2 || ids[0] != 100 || ids[1] != 257 {
		t.Fatalf("GetRegionIDs = %v, want [100 257]", ids)
	}
	if src, _ := mgr.GetSource(257); src != "second" {
		t.Fatal("re-install did not overwrite the source")
	}
	if _, objs, _ := mgr.GetReplacementMapData(257); objs != nil {
		t.Fatal("terrain-only re-install kept objects")
	}

	// Remove reports presence; a second remove reports absence.
	if !mgr.RemoveMapRegion(257) {
		t.Fatal("RemoveMapRegion false for an installed region")
	}
	if mgr.RemoveMapRegion(257) {
		t.Fatal("RemoveMapRegion true for an absent region")
	}
	if _, _, ok := mgr.GetReplacementMapData(257); ok {
		t.Fatal("removed region still returns data")
	}

	// Invalid region ids are refused everywhere.
	for _, id := range []int{-1, 0x10000} {
		if _, err := mgr.ReplaceMapRegionData(id, []byte{1}, nil, "x"); err == nil {
			t.Fatalf("region %d accepted", id)
		}
		if _, err := mgr.ReplaceMapRegionTerrain(id, []byte{1}, "x"); err == nil {
			t.Fatalf("terrain region %d accepted", id)
		}
	}
	// Empty terrain is refused at install time.
	if _, err := mgr.ReplaceMapRegionData(257, nil, nil, "x"); err == nil {
		t.Fatal("empty terrain accepted")
	}
}

func TestMapRegionReplacementNativePacket(t *testing.T) {
	mgr := NewMapRegionReplacementManager()
	if packet, err := mgr.NativeRegionPacket(257); packet != nil || err != nil {
		t.Fatalf("no source: (%v, %v), want (nil, nil)", packet, err)
	}
	src := newFakeRegionSource([]int{257})
	src.data[257] = fakeRegionData{terrain: []byte{1, 2}, objects: []byte{3}, ok: true}
	mgr.SetNativeSource(src)
	packet, err := mgr.NativeRegionPacket(257)
	if err != nil || packet == nil {
		t.Fatalf("native packet = (%v, %v)", packet, err)
	}
	if packet.Opcode != 144 || !bytes.Equal(packet.Payload, []byte{0x01, 0x01, 1, 0, 2, 0, 1, 1, 2, 3}) {
		t.Fatalf("native packet = %+v", packet)
	}
	if packet, err := mgr.NativeRegionPacket(999); packet != nil || err != nil {
		t.Fatalf("absent region: (%v, %v), want (nil, nil)", packet, err)
	}
	src.data[258] = fakeRegionData{terrain: nil, objects: nil, ok: true}
	if packet, err := mgr.NativeRegionPacket(258); packet != nil || err != nil {
		t.Fatalf("terrain-less region: (%v, %v), want (nil, nil)", packet, err)
	}
	var nilMgr *MapRegionReplacementManager
	if packet, err := nilMgr.NativeRegionPacket(257); packet != nil || err != nil {
		t.Fatalf("nil manager: (%v, %v)", packet, err)
	}
}

func TestMapRegionReplacementPackRoundTrip(t *testing.T) {
	mgr := NewMapRegionReplacementManager()
	mgr.SetArchiveIDs(func(regionID int) (int, int, bool) {
		if regionID == 257 {
			return 11, 22, true
		}
		return 0, 0, false
	})
	terrain := []byte{1, 2, 3}
	objects := []byte{4, 5}
	if _, err := mgr.ReplaceMapRegionData(257, terrain, objects, "editor"); err != nil {
		t.Fatal(err)
	}
	pack, ok := mgr.GetRegionPack(257, nil)
	if !ok {
		t.Fatal("GetRegionPack false for an installed region")
	}
	// Header fields: version, object file, terrain file, region
	// x/y, object length, terrain length.
	if got := binary.BigEndian.Uint32(pack[0:4]); got != 1 {
		t.Fatalf("pack version = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(pack[4:8]); got != 22 {
		t.Fatalf("pack object file = %d, want 22", got)
	}
	if got := binary.BigEndian.Uint32(pack[8:12]); got != 11 {
		t.Fatalf("pack terrain file = %d, want 11", got)
	}
	if got := binary.BigEndian.Uint32(pack[12:16]); got != 1 {
		t.Fatalf("pack region x = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint32(pack[16:20]); got != 1 {
		t.Fatalf("pack region y = %d, want 1", got)
	}
	if len(pack) != 28+len(objects)+len(terrain) {
		t.Fatalf("pack length = %d, want %d", len(pack), 28+len(objects)+len(terrain))
	}
	// The pack loads back into a fresh manager with the same data.
	path := filepath.Join(t.TempDir(), "257.pack")
	if err := os.WriteFile(path, pack, 0o600); err != nil {
		t.Fatal(err)
	}
	mgr2 := NewMapRegionReplacementManager()
	mgr2.SetArchiveIDs(func(regionID int) (int, int, bool) {
		if regionID == 257 {
			return 11, 22, true
		}
		return 0, 0, false
	})
	if _, err := mgr2.ReplaceMapRegion(257, path); err != nil {
		t.Fatal(err)
	}
	gotTerrain, gotObjects, _ := mgr2.GetReplacementMapData(257)
	if !bytes.Equal(gotTerrain, terrain) || !bytes.Equal(gotObjects, objects) {
		t.Fatal("pack round-trip changed the data")
	}

	// No archive seam: no pack. Terrain-only: no pack without an
	// override, a pack with one.
	mgr3 := NewMapRegionReplacementManager()
	if _, err := mgr3.ReplaceMapRegionData(257, terrain, objects, "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr3.GetRegionPack(257, nil); ok {
		t.Fatal("GetRegionPack true without the archive seam")
	}
	mgr.SetArchiveIDs(func(regionID int) (int, int, bool) { return 11, 22, true })
	if _, err := mgr.ReplaceMapRegionTerrain(257, terrain, "x"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mgr.GetRegionPack(257, nil); ok {
		t.Fatal("GetRegionPack true for a terrain-only replacement without override")
	}
	pack, ok = mgr.GetRegionPack(257, objects)
	if !ok {
		t.Fatal("GetRegionPack false with an object override")
	}
	if got := binary.BigEndian.Uint32(pack[20:24]); got != uint32(len(objects)) {
		t.Fatalf("override pack object length = %d", got)
	}
}

func TestMapRegionReplacementPackValidation(t *testing.T) {
	archiveIDs := func(regionID int) (int, int, bool) {
		if regionID == 257 {
			return 11, 22, true
		}
		return 0, 0, false
	}
	writePack := func(t *testing.T, pack []byte) string {
		t.Helper()
		path := filepath.Join(t.TempDir(), "257.pack")
		if err := os.WriteFile(path, pack, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	terrain, objects := []byte{1, 2}, []byte{3}

	// Gzipped sections decode on load.
	gzPack := buildPackBytes(257, 22, 11, gzipBytes(t, objects), gzipBytes(t, terrain))
	mgr := NewMapRegionReplacementManager()
	mgr.SetArchiveIDs(archiveIDs)
	if _, err := mgr.ReplaceMapRegion(257, writePack(t, gzPack)); err != nil {
		t.Fatalf("gzipped pack: %v", err)
	}
	if got, gotObjs, _ := mgr.GetReplacementMapData(257); !bytes.Equal(got, terrain) || !bytes.Equal(gotObjs, objects) {
		t.Fatal("gzipped pack sections did not decode")
	}

	cases := []struct {
		name   string
		pack   []byte
		noSeam bool
		want   string
	}{
		{"short file", []byte{1, 2, 3}, false, "too short"},
		{"bad version", func() []byte {
			p := buildPackBytes(257, 22, 11, objects, terrain)
			binary.BigEndian.PutUint32(p[0:4], 2)
			return p
		}(), false, "does not match"},
		{"region mismatch", buildPackBytes(258, 22, 11, objects, terrain), false, "does not match"},
		{"archive mismatch", buildPackBytes(257, 99, 11, objects, terrain), false, "archive IDs"},
		{"no archive seam", buildPackBytes(257, 22, 11, objects, terrain), true, "cannot be checked"},
		{"truncated first entry", buildPackBytes(257, 22, 11, objects, terrain)[:30], false, "truncated"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			m := NewMapRegionReplacementManager()
			if !c.noSeam {
				m.SetArchiveIDs(archiveIDs)
			}
			if _, err := m.ReplaceMapRegion(257, writePack(t, c.pack)); err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
			if _, _, ok := m.GetReplacementMapData(257); ok {
				t.Fatal("rejected pack installed a replacement")
			}
		})
	}

	// A non-.pack single file is refused; a missing file errors.
	mgr2 := NewMapRegionReplacementManager()
	mgr2.SetArchiveIDs(archiveIDs)
	if _, err := mgr2.ReplaceMapRegion(257, "region.dat"); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("non-pack err = %v", err)
	}
	if _, err := mgr2.ReplaceMapRegion(257, filepath.Join(t.TempDir(), "missing.pack")); err == nil {
		t.Fatal("missing pack file accepted")
	}
	if _, err := mgr2.ReplaceMapRegion(257, "  "); err == nil {
		t.Fatal("empty source accepted")
	}
}

func TestMapRegionReplacementFromFiles(t *testing.T) {
	dir := t.TempDir()
	terrainPath := filepath.Join(dir, "terrain.dat")
	objectPath := filepath.Join(dir, "objects.dat")
	terrain, objects := []byte{1, 2, 3}, []byte{4, 5}
	if err := os.WriteFile(terrainPath, terrain, 0o600); err != nil {
		t.Fatal(err)
	}
	// The object file may be gzip-compressed.
	if err := os.WriteFile(objectPath, gzipBytes(t, objects), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := NewMapRegionReplacementManager()
	result, err := mgr.ReplaceMapRegionFromFiles(257, terrainPath, objectPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(result.Source, "[") || !strings.Contains(result.Source, "terrain.dat") {
		t.Fatalf("pair source = %q, want [terrain, object] form", result.Source)
	}
	gotTerrain, gotObjects, _ := mgr.GetReplacementMapData(257)
	if !bytes.Equal(gotTerrain, terrain) || !bytes.Equal(gotObjects, objects) {
		t.Fatal("file-pair data mismatch")
	}
	// Missing and empty files error.
	if _, err := mgr.ReplaceMapRegionFromFiles(258, filepath.Join(dir, "nope.dat"), objectPath); err == nil {
		t.Fatal("missing terrain file accepted")
	}
	emptyPath := filepath.Join(dir, "empty.dat")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReplaceMapRegionFromFiles(258, emptyPath, objectPath); err == nil || !strings.Contains(err.Error(), "empty map data") {
		t.Fatalf("empty file err = %v", err)
	}
}

func TestLoadRegionReplacementDir(t *testing.T) {
	dir := t.TempDir()
	archiveIDs := func(regionID int) (int, int, bool) { return 11, 22, true }
	pack257 := buildPackBytes(257, 22, 11, []byte{3}, []byte{1, 2})
	pack259 := buildPackBytes(259, 22, 11, []byte{6}, []byte{4, 5})
	badPack := buildPackBytes(300, 22, 11, []byte{9}, []byte{8})
	binary.BigEndian.PutUint32(badPack[0:4], 2) // bad version
	files := map[string][]byte{
		"257.pack":        pack257,
		"258.terrain.dat": []byte{7, 7, 7},
		"259.pack":        pack259,
		"259.terrain.dat": []byte{9, 9, 9}, // loses to the .pack
		"300.pack":        badPack,         // skipped with a warning
		"notes.txt":       []byte("ignore me"),
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "400.pack"), 0o700); err != nil {
		t.Fatal(err)
	}
	mgr := NewMapRegionReplacementManager()
	mgr.SetArchiveIDs(archiveIDs)
	var warnings []string
	LoadRegionReplacementDir(dir, mgr, func(msg string) { warnings = append(warnings, msg) })

	if ids := mgr.GetRegionIDs(); len(ids) != 3 || ids[0] != 257 || ids[1] != 258 || ids[2] != 259 {
		t.Fatalf("installed ids = %v, want [257 258 259]", ids)
	}
	// The .pack wins for 259: the terrain.dat bytes never land.
	got, _, _ := mgr.GetReplacementMapData(259)
	if !bytes.Equal(got, []byte{4, 5}) {
		t.Fatalf("region 259 terrain = %v, want the .pack bytes", got)
	}
	// The loose .terrain.dat loads raw as a terrain-only
	// replacement.
	got, objs, _ := mgr.GetReplacementMapData(258)
	if !bytes.Equal(got, []byte{7, 7, 7}) || objs != nil {
		t.Fatalf("region 258 = (%v, %v), want raw terrain, nil objects", got, objs)
	}
	joined := strings.Join(warnings, "\n")
	if !strings.Contains(joined, "ignoring 259.terrain.dat") {
		t.Fatalf("warnings lack the .pack-wins line: %v", warnings)
	}
	if !strings.Contains(joined, "skipping region replacement") {
		t.Fatalf("warnings lack the bad-pack skip line: %v", warnings)
	}

	// A missing directory is a no-op; a nil manager is a no-op.
	LoadRegionReplacementDir(filepath.Join(dir, "missing"), mgr, func(string) { t.Error("warned for a missing dir") })
	LoadRegionReplacementDir(dir, nil, nil)
}

// recordingSink captures sent replacement packets: the opcode
// and the region id + allowReload flag parsed from the payload.
type recordingSink struct {
	calls   int
	regions []int
	flags   []byte
	opcodes []int
	fail    bool
}

func (s *recordingSink) sink(opcode int, payload []byte) bool {
	s.calls++
	if s.fail {
		return false
	}
	s.opcodes = append(s.opcodes, opcode)
	if len(payload) >= 3 {
		s.regions = append(s.regions, int(payload[0])<<8|int(payload[1]))
		s.flags = append(s.flags, payload[2])
	}
	return true
}

func replacementManagerWith(t *testing.T, ids ...int) *MapRegionReplacementManager {
	t.Helper()
	mgr := NewMapRegionReplacementManager()
	for _, id := range ids {
		if _, err := mgr.ReplaceMapRegionData(id, []byte{1}, []byte{2}, "test"); err != nil {
			t.Fatal(err)
		}
	}
	return mgr
}

func TestMapRegionReplacementSends(t *testing.T) {
	mgr := replacementManagerWith(t, 300, 257, 258)

	// All replacements, in region-id order, flag true.
	rec := &recordingSink{}
	if sent := mgr.SendAllReplacements(rec.sink); sent != 3 {
		t.Fatalf("SendAll sent %d, want 3", sent)
	}
	if len(rec.regions) != 3 || rec.regions[0] != 257 || rec.regions[1] != 258 || rec.regions[2] != 300 {
		t.Fatalf("SendAll regions = %v, want [257 258 300]", rec.regions)
	}
	for _, f := range rec.flags {
		if f != 1 {
			t.Fatalf("SendAll flag = %d, want 1", f)
		}
	}

	// One replacement; missing regions and nil sinks fail.
	rec = &recordingSink{}
	if !mgr.SendReplacement(rec.sink, 258, true) {
		t.Fatal("SendReplacement false for an installed region")
	}
	if len(rec.regions) != 1 || rec.regions[0] != 258 {
		t.Fatalf("SendReplacement regions = %v", rec.regions)
	}
	if mgr.SendReplacement(rec.sink, 999, true) {
		t.Fatal("SendReplacement true for a missing region")
	}
	if mgr.SendReplacement(nil, 258, true) {
		t.Fatal("SendReplacement true with a nil sink")
	}
	// A failing sink reports the failure.
	failing := &recordingSink{fail: true}
	if mgr.SendReplacement(failing.sink, 258, true) {
		t.Fatal("SendReplacement true with a failing sink")
	}

	// Visible from tile (96,96) radius 6: regions (0..2, 0..2),
	// so 257 and 258 — not 300 (region (1,44)).
	rec = &recordingSink{}
	if sent := mgr.SendVisibleReplacements(rec.sink, 96, 96, 6, nil, true); sent != 2 {
		t.Fatalf("SendVisible sent %d, want 2", sent)
	}
	if len(rec.regions) != 2 || rec.regions[0] != 257 || rec.regions[1] != 258 {
		t.Fatalf("SendVisible regions = %v, want [257 258]", rec.regions)
	}
	// Exclusions skip; allowReload=false clears the flag.
	rec = &recordingSink{}
	if sent := mgr.SendVisibleReplacements(rec.sink, 96, 96, 6, []int{258}, false); sent != 1 {
		t.Fatalf("SendVisible excluded sent %d, want 1", sent)
	}
	if rec.regions[0] != 257 || rec.flags[0] != 0 {
		t.Fatalf("SendVisible excluded = (%v, %v)", rec.regions, rec.flags)
	}
	// Non-visible is the complement.
	rec = &recordingSink{}
	if sent := mgr.SendNonVisibleReplacements(rec.sink, 96, 96, 6); sent != 1 {
		t.Fatalf("SendNonVisible sent %d, want 1", sent)
	}
	if len(rec.regions) != 1 || rec.regions[0] != 300 {
		t.Fatalf("SendNonVisible regions = %v, want [300]", rec.regions)
	}

	// SendRegionPacket frames with allowReload=true even when
	// the packet was built with false.
	packet, err := BuildRegionReplacementPacket(257, false, []byte{1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rec = &recordingSink{}
	if !mgr.SendRegionPacket(rec.sink, packet) {
		t.Fatal("SendRegionPacket false")
	}
	if rec.flags[0] != 1 {
		t.Fatalf("SendRegionPacket flag = %d, want 1", rec.flags[0])
	}
	if mgr.SendRegionPacket(nil, packet) {
		t.Fatal("SendRegionPacket true with a nil sink")
	}
}

func TestComputeVisibleRegionIDs(t *testing.T) {
	// Corner tile: negative region coordinates are skipped.
	ids := computeVisibleRegionIDs(0, 0, 6)
	if len(ids) != 1 {
		t.Fatalf("corner ids = %v, want only region 0", ids)
	}
	if _, ok := ids[0]; !ok {
		t.Fatalf("corner ids = %v, want region 0", ids)
	}
	// Far corner: regions (31..32, 31..32).
	ids = computeVisibleRegionIDs(2047, 2047, 6)
	want := map[int]struct{}{
		(31 << 8) | 31: {},
		(31 << 8) | 32: {},
		(32 << 8) | 31: {},
		(32 << 8) | 32: {},
	}
	if len(ids) != len(want) {
		t.Fatalf("far-corner ids = %v, want %v", ids, want)
	}
	for id := range want {
		if _, ok := ids[id]; !ok {
			t.Fatalf("far-corner ids = %v, want %v", ids, want)
		}
	}
	// Radius 0: exactly the tile's own region.
	ids = computeVisibleRegionIDs(96, 96, 0)
	if len(ids) != 1 {
		t.Fatalf("radius-0 ids = %v, want only 257", ids)
	}
	if _, ok := ids[257]; !ok {
		t.Fatalf("radius-0 ids = %v, want 257", ids)
	}
}

func TestReplacementRegionSource(t *testing.T) {
	base := newFakeRegionSource([]int{257, 258})
	base.data[257] = fakeRegionData{terrain: []byte{1}, objects: []byte{2}, ok: true}
	base.data[258] = fakeRegionData{terrain: []byte{3}, objects: []byte{4}, ok: true}
	mgr := NewMapRegionReplacementManager()
	if _, err := mgr.ReplaceMapRegionTerrain(257, []byte{9}, "test"); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.ReplaceMapRegionData(258, []byte{8}, []byte{7}, "test"); err != nil {
		t.Fatal(err)
	}
	wrapped := &ReplacementRegionSource{Base: base, Replacements: mgr}

	// Terrain-only replacement: replacement terrain wins, cache
	// objects stand.
	terrain, objects, ok := wrapped.RegionData(257)
	if !ok || !bytes.Equal(terrain, []byte{9}) || !bytes.Equal(objects, []byte{2}) {
		t.Fatalf("terrain-only = (%v, %v, %v)", terrain, objects, ok)
	}
	// Full replacement: both win.
	terrain, objects, ok = wrapped.RegionData(258)
	if !ok || !bytes.Equal(terrain, []byte{8}) || !bytes.Equal(objects, []byte{7}) {
		t.Fatalf("full = (%v, %v, %v)", terrain, objects, ok)
	}
	// No replacement: the base answers.
	if _, _, ok := wrapped.RegionData(999); ok {
		t.Fatal("absent region answered ok")
	}
	// Region ids delegate to the base: replacements never add
	// ids to the index.
	if ids := wrapped.RegionIDs(); len(ids) != 2 {
		t.Fatalf("RegionIDs = %v", ids)
	}
	// Nil halves degrade: nil base answers replacements only;
	// nil manager answers the base only.
	onlyReplacements := &ReplacementRegionSource{Replacements: mgr}
	if terrain, objects, ok := onlyReplacements.RegionData(257); !ok || !bytes.Equal(terrain, []byte{9}) || objects != nil {
		t.Fatalf("nil base = (%v, %v, %v)", terrain, objects, ok)
	}
	onlyBase := &ReplacementRegionSource{Base: base}
	if terrain, _, ok := onlyBase.RegionData(257); !ok || !bytes.Equal(terrain, []byte{1}) {
		t.Fatalf("nil manager = (%v, %v)", terrain, ok)
	}
	var nilSource *ReplacementRegionSource
	if _, _, ok := nilSource.RegionData(257); ok {
		t.Fatal("nil wrapper answered ok")
	}
	if nilSource.RegionIDs() != nil {
		t.Fatal("nil wrapper listed ids")
	}
}
