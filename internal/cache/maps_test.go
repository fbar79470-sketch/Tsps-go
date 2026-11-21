package cache

import (
	"errors"
	"testing"
)

func namedMapsSource(t *testing.T) *fakeSource {
	t.Helper()
	terrain := buildTestArchive(t, 100, map[int][]byte{0: []byte("terrain")}, nil)
	objects := buildTestArchive(t, 101, map[int][]byte{0: []byte("objects")}, nil)
	source := newFakeSource(map[int]*Archive{100: terrain, 101: objects})
	source.named = true
	source.names["m50_60"] = 100
	source.names["l50_60"] = 101
	return source
}

func TestCacheMapsNamed(t *testing.T) {
	source := namedMapsSource(t)
	keys := Keys{101: [4]int32{1, 2, 3, 4}}
	maps := NewCacheMaps(source, keys)
	region := (50 << 8) | 60
	terrain, objects, ok := maps.GetArchiveIDs(region)
	if !ok || terrain != 100 || objects != 101 {
		t.Errorf("GetArchiveIDs = %d %d %v", terrain, objects, ok)
	}
	if _, _, ok := maps.GetArchiveIDs((51 << 8) | 60); ok {
		t.Error("missing region resolved")
	}
	data, ok := maps.GetRegion(region)
	if !ok || string(data.TerrainData) != "terrain" || string(data.ObjectData) != "objects" {
		t.Errorf("GetRegion = %+v %v", data, ok)
	}
	// Terrain is read without a key; objects with the XTEA key
	// for the object archive id.
	if source.lastKey[100] != nil {
		t.Error("terrain read passed a key")
	}
	if source.lastKey[101] == nil || *source.lastKey[101] != keys[101] {
		t.Errorf("object key = %v", source.lastKey[101])
	}
	// A region with no key still loads, with a zero key.
	keys2 := Keys{}
	maps2 := NewCacheMaps(source, keys2)
	if _, ok := maps2.GetRegion(region); !ok {
		t.Error("keyless region failed")
	}
	if source.lastKey[101] == nil || *source.lastKey[101] != [4]int32{} {
		t.Errorf("keyless object key = %v", source.lastKey[101])
	}
	ids := maps.GetRegionIDs()
	if len(ids) != 1 || ids[0] != region {
		t.Errorf("GetRegionIDs = %v", ids)
	}
}

func TestCacheMapsUnnamed(t *testing.T) {
	t.Helper()
	region := (50 << 8) | 60
	archive := buildTestArchive(t, region, map[int][]byte{0: []byte("terrain"), 1: []byte("objects")}, nil)
	big := buildTestArchive(t, 70000, map[int][]byte{0: []byte("x")}, nil)
	source := newFakeSource(map[int]*Archive{region: archive, 70000: big})
	maps := NewCacheMaps(source, Keys{region: [4]int32{9, 9, 9, 9}})
	terrain, objects, ok := maps.GetArchiveIDs(region)
	if !ok || terrain != region || objects != region {
		t.Errorf("GetArchiveIDs = %d %d %v", terrain, objects, ok)
	}
	data, ok := maps.GetRegion(region)
	if !ok || string(data.TerrainData) != "terrain" || string(data.ObjectData) != "objects" {
		t.Errorf("GetRegion = %+v %v", data, ok)
	}
	if _, ok := maps.GetRegion(12345); ok {
		t.Error("missing region loaded")
	}
	ids := maps.GetRegionIDs()
	if len(ids) != 1 || ids[0] != region {
		t.Errorf("GetRegionIDs = %v (70000 must be filtered)", ids)
	}
}

func TestCacheMapsMissingObjectFile(t *testing.T) {
	terrainOnly := buildTestArchive(t, 100, map[int][]byte{0: []byte("terrain")}, nil)
	objectsEmpty := buildTestArchive(t, 101, map[int][]byte{}, nil)
	source := newFakeSource(map[int]*Archive{100: terrainOnly, 101: objectsEmpty})
	source.named = true
	source.names["m50_60"] = 100
	source.names["l50_60"] = 101
	maps := NewCacheMaps(source, Keys{})
	data, ok := maps.GetRegion((50 << 8) | 60)
	if !ok || string(data.TerrainData) != "terrain" || data.ObjectData != nil {
		t.Errorf("missing object file = %+v %v", data, ok)
	}
}

// failingFileSource wraps a fake source and fails GetFile for one archive.
type failingFileSource struct {
	*fakeSource
	failArchive int
}

func (f *failingFileSource) GetFile(archiveID, fileID int, key *[4]int32) ([]byte, bool, error) {
	if archiveID == f.failArchive {
		return nil, false, errTestFailure
	}
	return f.fakeSource.GetFile(archiveID, fileID, key)
}

var errTestFailure = errors.New("test failure")

func TestCacheMapsReadFailure(t *testing.T) {
	source := &failingFileSource{fakeSource: namedMapsSource(t), failArchive: 100}
	maps := NewCacheMaps(source, Keys{})
	if _, ok := maps.GetRegion((50 << 8) | 60); ok {
		t.Error("terrain read failure yielded a region")
	}
	source2 := &failingFileSource{fakeSource: namedMapsSource(t), failArchive: 101}
	maps2 := NewCacheMaps(source2, Keys{})
	if _, ok := maps2.GetRegion((50 << 8) | 60); ok {
		t.Error("object read failure yielded a region")
	}
}

func TestCacheMapsFromStore(t *testing.T) {
	dir := t.TempDir()
	table := buildRefTable(6, 1, 0x1, []refArchive{
		{idDelta: 100, nameHash: HashDjb2("m50_60"), crc: 1, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{1}},
		{idDelta: 1, nameHash: HashDjb2("l50_60"), crc: 2, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{2}},
	})
	writeStoreFixture(t, dir,
		map[int][]byte{IndexMaps: plainContainer(table)},
		map[int]map[int][]byte{IndexMaps: {
			100: plainContainer([]byte("terrain")),
			101: plainContainer([]byte("objects")),
		}})
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	maps, err := NewCacheMapsFromStore(store, Keys{})
	if err != nil {
		t.Fatal(err)
	}
	data, ok := maps.GetRegion((50 << 8) | 60)
	if !ok || string(data.TerrainData) != "terrain" || string(data.ObjectData) != "objects" {
		t.Errorf("store GetRegion = %+v %v", data, ok)
	}
}
