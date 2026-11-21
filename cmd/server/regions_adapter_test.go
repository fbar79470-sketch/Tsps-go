package main

import (
	"testing"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// Adapter tests build real CacheDefinitions / CacheMaps facades over
// in-memory fakes (the fakeArchiveSource / buildArchive helpers from
// definitions_test.go, same package) and hand-built definition and
// map bytes. No network, no cache files.

// locFile encodes one object definition: opcode 2 name, opcode 14
// sizeX, opcode 15 sizeY, opcode 27 clipType 1, opcode 17 clipType 0
// + no projectile block, opcode 18 no projectile block, terminated
// by opcode 0. Negative sizes mean "leave the default".
func locFile(name string, sizeX, sizeY int, clipType int, blocksProjectile bool) []byte {
	var b []byte
	b = append(b, 2)
	b = append(b, name...)
	b = append(b, 0)
	if sizeX >= 0 {
		b = append(b, 14, byte(sizeX))
	}
	if sizeY >= 0 {
		b = append(b, 15, byte(sizeY))
	}
	switch clipType {
	case 0:
		b = append(b, 17)
	case 1:
		b = append(b, 27)
	}
	if !blocksProjectile && clipType != 0 {
		b = append(b, 18)
	}
	return append(b, 0)
}

func testRegionDefs(t *testing.T) *cache.CacheDefinitions {
	t.Helper()
	empty := func(id int) *cache.Archive { return buildArchive(t, id, map[int][]byte{0: {0}}) }
	configs := &fakeArchiveSource{archives: map[int]*cache.Archive{
		cache.ConfigNPCs: empty(cache.ConfigNPCs),
		cache.ConfigLocs: buildArchive(t, cache.ConfigLocs, map[int][]byte{
			0: {0}, // defaults: 1x1, clipType 2, blocks projectiles
			1: locFile("Table", 3, 2, 2, true),
			2: locFile("Rug", -1, -1, 1, true),
			3: locFile("Ghost", -1, -1, 0, false),
			4: locFile("Fence", -1, -1, 2, false),
		}),
		cache.ConfigObjs:      empty(cache.ConfigObjs),
		cache.ConfigSeqs:      empty(cache.ConfigSeqs),
		cache.ConfigVarbits:   empty(cache.ConfigVarbits),
		cache.ConfigSpotAnims: empty(cache.ConfigSpotAnims),
		cache.ConfigIdentkits: empty(cache.ConfigIdentkits),
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
	return defs
}

func TestCacheRegionSourcesObjectDef(t *testing.T) {
	defs := testRegionDefs(t)
	if got := defs.GetCounts().Objects; got != 5 {
		t.Fatalf("object count = %d, want 5", got)
	}
	s := newCacheRegionSources(nil, defs)
	cases := []struct {
		id   int
		want game.ObjectDef
		ok   bool
	}{
		{0, game.ObjectDef{SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: true}, true},
		{1, game.ObjectDef{SizeX: 3, SizeY: 2, ClipType: 2, BlocksProjectile: true}, true},
		{2, game.ObjectDef{SizeX: 1, SizeY: 1, ClipType: 1, BlocksProjectile: true}, true},
		{3, game.ObjectDef{SizeX: 1, SizeY: 1, ClipType: 0, BlocksProjectile: false}, true},
		{4, game.ObjectDef{SizeX: 1, SizeY: 1, ClipType: 2, BlocksProjectile: false}, true},
		{5, game.ObjectDef{}, false},  // at the count bound: no definition
		{99, game.ObjectDef{}, false}, // beyond the bound
		{-1, game.ObjectDef{}, false},
	}
	for _, c := range cases {
		got, ok := s.ObjectDef(c.id)
		if ok != c.ok || got != c.want {
			t.Errorf("ObjectDef(%d) = %+v, %v; want %+v, %v", c.id, got, ok, c.want, c.ok)
		}
	}
	// A nil definitions facade answers no definitions.
	nilSources := newCacheRegionSources(nil, nil)
	if _, ok := nilSources.ObjectDef(1); ok {
		t.Error("ObjectDef with nil defs must return ok=false")
	}
}

// fakeMapIndex is a minimal in-memory cache.MapIndex for building a
// real CacheMaps facade without a Store.
type fakeMapIndex struct {
	archives map[int]*cache.Archive
	names    map[string]int
}

func (f *fakeMapIndex) GetArchive(archiveID int, key *[4]int32) (*cache.Archive, error) {
	return f.archives[archiveID], nil
}

func (f *fakeMapIndex) ArchiveCount() int { return len(f.archives) }

func (f *fakeMapIndex) LastArchiveID() int {
	max := -1
	for id := range f.archives {
		if id > max {
			max = id
		}
	}
	return max
}

func (f *fakeMapIndex) FileCount(archiveID int) int {
	if a, ok := f.archives[archiveID]; ok {
		return a.FileCount
	}
	return 0
}

func (f *fakeMapIndex) Named() bool { return true }

func (f *fakeMapIndex) ArchiveID(name string) int {
	if id, ok := f.names[name]; ok {
		return id
	}
	return -1
}

func (f *fakeMapIndex) ArchiveExists(id int) bool {
	_, ok := f.archives[id]
	return ok
}

func (f *fakeMapIndex) ArchiveIDs() []int {
	ids := make([]int, 0, len(f.archives))
	for id := range f.archives {
		ids = append(ids, id)
	}
	return ids
}

func (f *fakeMapIndex) GetFile(archiveID, fileID int, key *[4]int32) ([]byte, bool, error) {
	a, ok := f.archives[archiveID]
	if !ok {
		return nil, false, nil
	}
	data, found := a.GetFile(fileID)
	return data, found, nil
}

// testRegionTerrain builds new-format terrain bytes for one region:
// every tile terminates, except tile (0, 10, 10), which carries
// height value 1 (blocked).
func testRegionTerrain(t *testing.T) []byte {
	t.Helper()
	var out []byte
	for z := 0; z < 4; z++ {
		for x := 0; x < 64; x++ {
			for y := 0; y < 64; y++ {
				if z == 0 && x == 10 && y == 10 {
					out = append(out, 0, 50, 0, 0)
				} else {
					out = append(out, 0, 0)
				}
			}
		}
	}
	return out
}

func TestCacheRegionSourcesRegionData(t *testing.T) {
	terrain := testRegionTerrain(t)
	objects := []byte{0} // empty object stream: readSmart 0 terminates
	regionID := (50 << 8) | 60
	source := &fakeMapIndex{
		archives: map[int]*cache.Archive{
			100: buildArchive(t, 100, map[int][]byte{0: terrain}),
			101: buildArchive(t, 101, map[int][]byte{0: objects}),
		},
		names: map[string]int{"m50_60": 100, "l50_60": 101},
	}
	maps := cache.NewCacheMaps(source, cache.Keys{})
	s := newCacheRegionSources(maps, nil)

	ids := s.RegionIDs()
	if len(ids) != 1 || ids[0] != regionID {
		t.Fatalf("RegionIDs = %v, want [%d]", ids, regionID)
	}
	gotTerrain, gotObjects, ok := s.RegionData(regionID)
	if !ok {
		t.Fatal("RegionData must succeed")
	}
	if len(gotTerrain) != len(terrain) || string(gotTerrain) != string(terrain) {
		t.Error("terrain bytes differ")
	}
	if string(gotObjects) != string(objects) {
		t.Errorf("object bytes = %v, want %v", gotObjects, objects)
	}
	if _, _, ok := s.RegionData((51 << 8) | 60); ok {
		t.Error("missing region must return ok=false")
	}

	// A nil maps facade answers no regions.
	nilSources := newCacheRegionSources(nil, nil)
	if ids := nilSources.RegionIDs(); len(ids) != 0 {
		t.Errorf("RegionIDs with nil maps = %v, want empty", ids)
	}
	if _, _, ok := nilSources.RegionData(regionID); ok {
		t.Error("RegionData with nil maps must return ok=false")
	}
}

func TestCacheRegionSourcesEndToEnd(t *testing.T) {
	// The adapter feeds a real RegionManager: the blocked terrain
	// tile decodes into clipping through the whole seam.
	terrain := testRegionTerrain(t)
	source := &fakeMapIndex{
		archives: map[int]*cache.Archive{
			100: buildArchive(t, 100, map[int][]byte{0: terrain}),
			101: buildArchive(t, 101, map[int][]byte{0: []byte{0}}),
		},
		names: map[string]int{"m50_60": 100, "l50_60": 101},
	}
	maps := cache.NewCacheMaps(source, cache.Keys{})
	s := newCacheRegionSources(maps, testRegionDefs(t))
	m := game.NewRegionManager(s, s, true)
	if m.RegionCount() != 1 {
		t.Fatalf("RegionCount = %d, want 1", m.RegionCount())
	}
	absX, absY := 50*64, 60*64
	m.LoadRegion(absX, absY)
	if !m.IsRegionLoaded((50 << 8) | 60) {
		t.Fatal("region did not load through the adapter")
	}
	if got := m.GetClipping(absX+10, absY+10, 0); got != 0x200000 {
		t.Errorf("blocked tile through adapter = %#x, want 0x200000", got)
	}
	if got := m.GetClipping(absX+11, absY+10, 0); got != 0 {
		t.Errorf("open tile through adapter = %#x, want 0", got)
	}
	if m.CanMove(absX+9, absY+10, absX+10, absY+10, 0, 1, 1) {
		t.Error("CanMove into the blocked tile must fail")
	}
}
