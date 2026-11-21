package cache

import (
	"encoding/binary"
	"fmt"
	"sort"
	"testing"
)

// Shared fixtures for the definition decoder and facade tests.
// All fixtures are hand-built byte streams; no cache binaries, no
// network.

// testInfo returns an oldschool CacheInfo at the given revision,
// the shape CacheDefinitions.getState builds.
func testInfo(revision int) CacheInfo {
	return CacheInfo{
		Name:        "osrs-242_2026-11-03",
		Game:        GameOldschool,
		Environment: "live",
		Revision:    revision,
		Timestamp:   "2026-11-03",
		Size:        0,
	}
}

// enc builds opcode streams byte by byte.
type enc struct {
	buf []byte
}

func (e *enc) u8(v int) *enc {
	e.buf = append(e.buf, byte(v))
	return e
}

func (e *enc) i8(v int) *enc {
	e.buf = append(e.buf, byte(int8(v)))
	return e
}

func (e *enc) u16(v int) *enc {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], uint16(v))
	e.buf = append(e.buf, tmp[:]...)
	return e
}

func (e *enc) i16(v int) *enc {
	var tmp [2]byte
	binary.BigEndian.PutUint16(tmp[:], uint16(int16(v)))
	e.buf = append(e.buf, tmp[:]...)
	return e
}

func (e *enc) i32(v int) *enc {
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(int32(v)))
	e.buf = append(e.buf, tmp[:]...)
	return e
}

func (e *enc) medium(v int) *enc {
	e.buf = append(e.buf, byte(v>>16), byte(v>>8), byte(v))
	return e
}

func (e *enc) str(s string) *enc {
	e.buf = append(e.buf, s...)
	e.buf = append(e.buf, 0)
	return e
}

func (e *enc) bytes(b ...byte) *enc {
	e.buf = append(e.buf, b...)
	return e
}

// smart writes an unsigned smart: one byte below 128, else a short
// biased by 0x8000.
func (e *enc) smart(v int) *enc {
	if v < 128 {
		return e.u8(v)
	}
	return e.u16(v + 0x8000)
}

// smartMin1 writes an unsigned smart minus one: values are stored
// biased by 1.
func (e *enc) smartMin1(v int) *enc {
	return e.smart(v + 1)
}

// bigSmart writes a big smart: a short when the value fits below
// 32767, else an int with the top bit set.
func (e *enc) bigSmart(v int) *enc {
	if v < 32767 {
		return e.u16(v)
	}
	return e.i32(v | -0x80000000)
}

// varInt2 writes a little-endian base-128 varint.
func (e *enc) varInt2(v int) *enc {
	for {
		b := v & 0x7f
		v >>= 7
		if v == 0 {
			e.buf = append(e.buf, byte(b))
			return e
		}
		e.buf = append(e.buf, byte(b|0x80))
	}
}

func (e *enc) done() []byte { return e.buf }

// buildTestArchive builds an *Archive holding files, via
// DecodeArchive, so tests exercise the real archive layout. Files
// are stored in ascending id order in a single chunk; names maps
// file ids to djb2-hashed names for GetFileNamed tests.
func buildTestArchive(t *testing.T, id int, files map[int][]byte, names map[int]string) *Archive {
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
	var hashes []int32
	if names != nil {
		hashes = make([]int32, len(ids))
		for i, fileID := range ids {
			hashes[i] = HashDjb2(names[fileID])
		}
	}
	var payload []byte
	if len(ids) == 1 {
		payload = files[ids[0]]
	} else if len(ids) == 0 {
		payload = []byte{0} // zero chunks; no files
	} else {
		chunk := make([][]byte, len(ids))
		for i, fileID := range ids {
			chunk[i] = files[fileID]
		}
		payload = buildMultiFileArchive(t, [][][]byte{chunk})
	}
	archive, err := DecodeArchive(id, lastFileID, len(ids), ids, hashes, payload)
	if err != nil {
		t.Fatalf("buildTestArchive(%d): %v", id, err)
	}
	return archive
}

// fakeSource is an in-memory ArchiveSource (and namedArchiveSource)
// for facade, gamevals and maps tests.
type fakeSource struct {
	archives map[int]*Archive
	names    map[string]int // archive name -> id
	named    bool
	calls    map[int]int // GetArchive call counts per archive id
	lastKey  map[int]*[4]int32
}

func newFakeSource(archives map[int]*Archive) *fakeSource {
	return &fakeSource{archives: archives, names: map[string]int{}, calls: map[int]int{}, lastKey: map[int]*[4]int32{}}
}

func (f *fakeSource) GetArchive(archiveID int, key *[4]int32) (*Archive, error) {
	f.calls[archiveID]++
	f.lastKey[archiveID] = key
	archive, ok := f.archives[archiveID]
	if !ok {
		return nil, fmt.Errorf("Archive reference not found for: %d", archiveID)
	}
	return archive, nil
}

func (f *fakeSource) ArchiveCount() int {
	return len(f.archives)
}

func (f *fakeSource) LastArchiveID() int {
	max := -1
	for id := range f.archives {
		if id > max {
			max = id
		}
	}
	return max
}

func (f *fakeSource) FileCount(archiveID int) int {
	if a, ok := f.archives[archiveID]; ok {
		return a.FileCount
	}
	return 0
}

func (f *fakeSource) Named() bool { return f.named }

func (f *fakeSource) ArchiveID(name string) int {
	if id, ok := f.names[name]; ok {
		return id
	}
	return -1
}

func (f *fakeSource) ArchiveExists(id int) bool {
	_, ok := f.archives[id]
	return ok
}

func (f *fakeSource) ArchiveIDs() []int {
	ids := make([]int, 0, len(f.archives))
	for id := range f.archives {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func (f *fakeSource) GetFile(archiveID, fileID int, key *[4]int32) ([]byte, bool, error) {
	archive, err := f.GetArchive(archiveID, key)
	if err != nil {
		return nil, false, err
	}
	data, ok := archive.GetFile(fileID)
	return data, ok, nil
}

// fakeFactory routes index ids to fake sources and records which
// indexes were requested, for lazy-loading assertions.
type fakeFactory struct {
	sources map[int]ArchiveSource
	asked   map[int]int
}

func (f *fakeFactory) factory(indexID int) (ArchiveSource, error) {
	f.asked[indexID]++
	source, ok := f.sources[indexID]
	if !ok {
		return nil, fmt.Errorf("no fake source for index %d", indexID)
	}
	return source, nil
}

// mapLoader is a TypeLoader backed by a fixed map, for transform
// and count-variant tests.
type mapLoader[T any] struct {
	types map[int]T
	def   func(id int) T
}

func (l mapLoader[T]) Load(id int) T {
	if v, ok := l.types[id]; ok {
		return v
	}
	return l.def(id)
}

func (l mapLoader[T]) Count() int  { return len(l.types) }
func (l mapLoader[T]) ClearCache() {}

// fakeVars is a VarReader backed by maps.
type fakeVars struct {
	varps   map[int]int
	varbits map[int]int
}

func (v fakeVars) GetVarp(id int) int   { return v.varps[id] }
func (v fakeVars) GetVarbit(id int) int { return v.varbits[id] }
