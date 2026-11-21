package main

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// loadDefinitions is tested against synthetic directories only: no
// network, no real cache. The happy path against a real rev-242
// cache is the coordinator's live smoke test (see the report); a
// synthetic Store cannot be built without reimplementing the
// sector/container encoding the cache package's own test helpers
// own, and those helpers are test-only code in another package.

func TestLoadDefinitionsErrorsCleanly(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		active := &cache.ActiveCache{
			Name:      testTarget,
			Revision:  242,
			Directory: filepath.Join(t.TempDir(), "no-such-cache"),
		}
		if _, err := loadDefinitions(active); err == nil || !strings.Contains(err.Error(), "opening cache store") {
			t.Fatalf("loadDefinitions = %v, want an opening-cache-store error", err)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		active := &cache.ActiveCache{Name: testTarget, Revision: 242, Directory: t.TempDir()}
		if _, err := loadDefinitions(active); err == nil || !strings.Contains(err.Error(), "opening cache store") {
			t.Fatalf("loadDefinitions = %v, want an opening-cache-store error", err)
		}
	})

	t.Run("missing keys.json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main_file_cache.dat2"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		active := &cache.ActiveCache{Name: testTarget, Revision: 242, Directory: dir}
		if _, err := loadDefinitions(active); err == nil || !strings.Contains(err.Error(), "loading XTEA keys") {
			t.Fatalf("loadDefinitions = %v, want a loading-XTEA-keys error", err)
		}
	})

	t.Run("malformed keys.json", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main_file_cache.dat2"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "keys.json"), []byte("{bad json"), 0o644); err != nil {
			t.Fatal(err)
		}
		active := &cache.ActiveCache{Name: testTarget, Revision: 242, Directory: dir}
		if _, err := loadDefinitions(active); err == nil || !strings.Contains(err.Error(), "loading XTEA keys") {
			t.Fatalf("loadDefinitions = %v, want a loading-XTEA-keys error", err)
		}
	})

	t.Run("unreadable cache fails facade construction", func(t *testing.T) {
		// An empty data file opens as a store, but no reference
		// table can load from it, so facade construction — the
		// fail-boot path — must error rather than serve empty
		// definitions.
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "main_file_cache.dat2"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "keys.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		active := &cache.ActiveCache{Name: testTarget, Revision: 242, Directory: dir}
		if _, err := loadDefinitions(active); err == nil || !strings.Contains(err.Error(), "loading cache definitions") {
			t.Fatalf("loadDefinitions = %v, want a loading-cache-definitions error", err)
		}
	})
}

func TestNoCacheBootHasNilDefinitions(t *testing.T) {
	srv := testServer(t, 10)
	if srv.active != nil {
		t.Errorf("active = %+v, want nil in NoCache mode", srv.active)
	}
	if srv.loaded != nil {
		t.Errorf("loaded = %+v, want nil in NoCache mode", srv.loaded)
	}
	if srv.loginDeps.ItemWeights != nil {
		t.Errorf("ItemWeights = %v, want nil in NoCache mode", srv.loginDeps.ItemWeights)
	}
	if got := srv.adapter.XteaKey(12345); got != [4]int32{} {
		t.Errorf("XteaKey = %v, want zeros in NoCache mode", got)
	}
}

func TestXteaKey(t *testing.T) {
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	adapter := newWorldAdapter(game.NewWorld(spawn), spawn, time.Second)

	// Nil keys (NoCache mode): the zero fallback.
	if got := adapter.XteaKey(12850); got != [4]int32{} {
		t.Errorf("XteaKey(nil keys) = %v, want zeros", got)
	}

	adapter.keys = cache.Keys{12850: {1, -2, 3, -4}}
	if got := adapter.XteaKey(12850); got != [4]int32{1, -2, 3, -4} {
		t.Errorf("XteaKey(12850) = %v, want {1 -2 3 -4}", got)
	}
	if got := adapter.XteaKey(9999); got != [4]int32{} {
		t.Errorf("XteaKey(unknown region) = %v, want zeros", got)
	}
}

func TestObjWeightGrams(t *testing.T) {
	if got := objWeightGrams(nil); got != 0 {
		t.Errorf("objWeightGrams(nil) = %d, want 0", got)
	}
	plain := cache.NewObjType(1, cache.CacheInfo{})
	plain.Weight = 2500
	if got := objWeightGrams(plain); got != 2500 {
		t.Errorf("plain item = %d, want 2500", got)
	}
	stackable := cache.NewObjType(2, cache.CacheInfo{})
	stackable.Weight = 2500
	stackable.Stackability = cache.ObjStackabilityAlways // opcode 11
	if got := objWeightGrams(stackable); got != 0 {
		t.Errorf("stackable item = %d, want 0", got)
	}
	never := cache.NewObjType(3, cache.CacheInfo{})
	never.Weight = 2500
	never.Stackability = cache.ObjStackabilityNever // opcode 160: NOT stackable per TS === ALWAYS
	if got := objWeightGrams(never); got != 2500 {
		t.Errorf("never-stackable item = %d, want 2500", got)
	}
	noted := cache.NewObjType(4, cache.CacheInfo{})
	noted.Weight = 2500
	noted.NoteTemplate = 5 // opcode 98
	if got := objWeightGrams(noted); got != 0 {
		t.Errorf("noted item = %d, want 0", got)
	}
	// A note link alone (opcode 97, no template) is the unnoted item:
	// it still weighs.
	unnoted := cache.NewObjType(5, cache.CacheInfo{})
	unnoted.Weight = 2500
	unnoted.Note = 4 // opcode 97
	if got := objWeightGrams(unnoted); got != 2500 {
		t.Errorf("unnoted item with note link = %d, want 2500", got)
	}
	// Graceful-style negative weights pass through unchanged.
	light := cache.NewObjType(6, cache.CacheInfo{})
	light.Weight = -1500
	if got := objWeightGrams(light); got != -1500 {
		t.Errorf("negative-weight item = %d, want -1500", got)
	}
	// A default definition (what loaders answer for unknown ids)
	// weighs 0.
	if got := objWeightGrams(cache.NewObjType(999, cache.CacheInfo{})); got != 0 {
		t.Errorf("default definition = %d, want 0", got)
	}
}

func TestItemWeightsNilDefs(t *testing.T) {
	var w itemWeights
	if got := w.ItemWeightGrams(1); got != 0 {
		t.Errorf("nil-defs ItemWeightGrams = %d, want 0", got)
	}
	if got := w.ItemWeightGrams(-1); got != 0 {
		t.Errorf("nil-defs ItemWeightGrams(-1) = %d, want 0", got)
	}
}

// fakeArchiveSource is a minimal in-memory cache.ArchiveSource for
// building a real CacheDefinitions facade without a Store: the
// cache package's own fake lives in its test files and cannot be
// imported here.
type fakeArchiveSource struct {
	archives map[int]*cache.Archive
}

func (f *fakeArchiveSource) GetArchive(archiveID int, key *[4]int32) (*cache.Archive, error) {
	return f.archives[archiveID], nil
}

func (f *fakeArchiveSource) ArchiveCount() int { return len(f.archives) }

func (f *fakeArchiveSource) LastArchiveID() int {
	max := -1
	for id := range f.archives {
		if id > max {
			max = id
		}
	}
	return max
}

func (f *fakeArchiveSource) FileCount(archiveID int) int {
	if a, ok := f.archives[archiveID]; ok {
		return a.FileCount
	}
	return 0
}

// buildArchive assembles one archive from file id -> opcode bytes,
// using cache.DecodeArchive (the exported constructor). Multi-file
// archives use a single-chunk payload: concatenated file data, then
// delta-encoded chunk sizes, then the chunk-count byte.
func buildArchive(t *testing.T, id int, files map[int][]byte) *cache.Archive {
	t.Helper()
	ids := make([]int, 0, len(files))
	for fileID := range files {
		ids = append(ids, fileID)
	}
	sort.Ints(ids)
	lastFileID := -1
	if len(ids) > 0 {
		lastFileID = ids[len(ids)-1]
	}
	var payload []byte
	if len(ids) == 1 {
		payload = files[ids[0]]
	} else {
		for _, fileID := range ids {
			payload = append(payload, files[fileID]...)
		}
		var tmp [4]byte
		last := 0
		for _, fileID := range ids {
			binary.BigEndian.PutUint32(tmp[:], uint32(int32(len(files[fileID])-last)))
			payload = append(payload, tmp[:]...)
			last = len(files[fileID])
		}
		payload = append(payload, 1) // one chunk
	}
	archive, err := cache.DecodeArchive(id, lastFileID, len(ids), ids, nil, payload)
	if err != nil {
		t.Fatalf("DecodeArchive(%d): %v", id, err)
	}
	return archive
}

// objFile encodes one item definition: opcode 2 name, opcode 75
// weight, opcode 11 stackable, opcodes 97/98 note link/template,
// terminated by opcode 0.
func objFile(name string, weight int, stackable bool, note, noteTemplate int) []byte {
	var b []byte
	b = append(b, 2)
	b = append(b, name...)
	b = append(b, 0)
	if stackable {
		b = append(b, 11)
	}
	var tmp [2]byte
	b = append(b, 75)
	binary.BigEndian.PutUint16(tmp[:], uint16(int16(weight)))
	b = append(b, tmp[:]...)
	if note >= 0 {
		b = append(b, 97)
		binary.BigEndian.PutUint16(tmp[:], uint16(note))
		b = append(b, tmp[:]...)
	}
	if noteTemplate >= 0 {
		b = append(b, 98)
		binary.BigEndian.PutUint16(tmp[:], uint16(noteTemplate))
		b = append(b, tmp[:]...)
	}
	return append(b, 0)
}

func TestItemWeightsOverSyntheticFacade(t *testing.T) {
	empty := func(id int) *cache.Archive { return buildArchive(t, id, map[int][]byte{0: {0}}) }
	configs := &fakeArchiveSource{archives: map[int]*cache.Archive{
		cache.ConfigNPCs:      empty(cache.ConfigNPCs),
		cache.ConfigLocs:      empty(cache.ConfigLocs),
		cache.ConfigSeqs:      empty(cache.ConfigSeqs),
		cache.ConfigVarbits:   empty(cache.ConfigVarbits),
		cache.ConfigSpotAnims: empty(cache.ConfigSpotAnims),
		cache.ConfigIdentkits: empty(cache.ConfigIdentkits),
		cache.ConfigObjs: buildArchive(t, cache.ConfigObjs, map[int][]byte{
			0: {0},
			1: objFile("Bronze sword", 2500, false, -1, -1),
			2: objFile("Coins", 500, true, -1, -1),
			3: objFile("Noted sword", 999, false, 1, 4),
			4: objFile("Note template", 0, false, -1, -1),
			5: objFile("Graceful hood", -1500, false, -1, -1),
		}),
	}}
	info := cache.CacheInfo{Name: testTarget, Game: cache.GameOldschool, Environment: "live", Revision: 242, Timestamp: "2026-11-03"}
	defs, err := cache.NewCacheDefinitions(info, func(indexID int) (cache.ArchiveSource, error) {
		if indexID == cache.IndexConfigs {
			return configs, nil
		}
		t.Fatalf("unexpected index request %d", indexID)
		return nil, nil
	})
	if err != nil {
		t.Fatalf("NewCacheDefinitions: %v", err)
	}
	counts := defs.GetCounts()
	if counts.Items != 6 {
		t.Fatalf("item count = %d, want 6", counts.Items)
	}

	w := itemWeights{defs: defs}
	cases := []struct {
		id   int
		want int
	}{
		{0, 0},     // empty definition
		{1, 2500},  // plain weighted item
		{2, 0},     // stackable (opcode 11)
		{3, 0},     // noted (opcodes 97/98; post-processing also forces stackable)
		{4, 0},     // note template itself weighs 0
		{5, -1500}, // graceful-style negative weight passes through
		{99, 0},    // unknown id: default definition, weight 0
		{-1, 0},    // negative id: guarded without consulting the loader
	}
	for _, tc := range cases {
		if got := w.ItemWeightGrams(tc.id); got != tc.want {
			t.Errorf("ItemWeightGrams(%d) = %d, want %d", tc.id, got, tc.want)
		}
	}
}
