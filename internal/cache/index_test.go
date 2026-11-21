package cache

import (
	"errors"
	"fmt"
	"sort"
	"testing"
)

// writeStoreFixture builds a real Store directory: meta maps index
// id -> meta-archive bytes (the reference-table container), and
// indexes maps index id -> archive id -> container bytes. Sectors
// are chained in ascending id order.
func writeStoreFixture(t *testing.T, dir string, meta map[int][]byte, indexes map[int]map[int][]byte) {
	t.Helper()
	withMeta(t, dir)
	sector := 0
	var sectors [][]byte
	chain := func(archiveID, indexID int, data []byte) int {
		first := sector
		if len(data) == 0 {
			return first
		}
		for off := 0; off < len(data); {
			end := off + 512
			if end > len(data) {
				end = len(data)
			}
			next := 0
			if end < len(data) {
				next = sector + 1
			}
			sectors = append(sectors, buildSector(false, archiveID, (off/512), next, indexID, data[off:end]))
			sector++
			off = end
		}
		return first
	}
	metaIDs := make([]int, 0, len(meta))
	for id := range meta {
		metaIDs = append(metaIDs, id)
	}
	sort.Ints(metaIDs)
	metaEntries := map[int][2]int{}
	for _, id := range metaIDs {
		metaEntries[id] = [2]int{len(meta[id]), chain(id, MetaIndexID, meta[id])}
	}
	writeIndex(t, dir, MetaIndexID, metaEntries)
	indexIDs := make([]int, 0, len(indexes))
	for id := range indexes {
		indexIDs = append(indexIDs, id)
	}
	sort.Ints(indexIDs)
	for _, indexID := range indexIDs {
		archiveIDs := make([]int, 0, len(indexes[indexID]))
		for id := range indexes[indexID] {
			archiveIDs = append(archiveIDs, id)
		}
		sort.Ints(archiveIDs)
		entries := map[int][2]int{}
		for _, archiveID := range archiveIDs {
			data := indexes[indexID][archiveID]
			entries[archiveID] = [2]int{len(data), chain(archiveID, indexID, data)}
		}
		writeIndex(t, dir, indexID, entries)
	}
	writeDat2(t, dir, sectors...)
}

// plainContainer wraps payload in an uncompressed container.
func plainContainer(payload []byte) []byte {
	out := []byte{0, 0, 0, 0, 0}
	out[1] = byte(len(payload) >> 24)
	out[2] = byte(len(payload) >> 16)
	out[3] = byte(len(payload) >> 8)
	out[4] = byte(len(payload))
	return append(out, payload...)
}

func refContainer(t *testing.T, table []byte) []byte {
	t.Helper()
	return plainContainer(table)
}

func TestCacheIndexFromStore(t *testing.T) {
	dir := t.TempDir()
	table := buildRefTable(6, 1234, 0x1, []refArchive{
		{idDelta: 6, nameHash: HashDjb2("npcs"), crc: 1, revision: 1, fileDeltas: []int{0, 2}, fileHashes: []int32{10, 20}},
		{idDelta: 3, nameHash: HashDjb2("locs"), crc: 2, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{30}},
	})
	archive6 := buildMultiFileArchive(t, [][][]byte{{[]byte("file0"), []byte("file2")}})
	archive9 := []byte("solo")
	writeStoreFixture(t, dir,
		map[int][]byte{2: refContainer(t, table)},
		map[int]map[int][]byte{2: {
			6: plainContainer(archive6),
			9: plainContainer(archive9),
		}})
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	index, err := NewCacheIndexFromStore(2, store)
	if err != nil {
		t.Fatal(err)
	}
	if index.ID() != 2 || index.ArchiveCount() != 2 || index.LastArchiveID() != 9 || !index.Named() {
		t.Errorf("header: id=%d count=%d last=%d named=%v", index.ID(), index.ArchiveCount(), index.LastArchiveID(), index.Named())
	}
	if got := index.Table().Revision; got != 1234 {
		t.Errorf("table revision = %d", got)
	}
	if got := index.ArchiveID("npcs"); got != 6 {
		t.Errorf("ArchiveID(npcs) = %d", got)
	}
	if got := index.ArchiveID("missing"); got != -1 {
		t.Errorf("ArchiveID(missing) = %d", got)
	}
	if !index.ArchiveExists(9) || index.ArchiveExists(77) {
		t.Error("ArchiveExists")
	}
	ids, ok := index.FileIDs(6)
	if !ok || len(ids) != 2 || ids[0] != 0 || ids[1] != 2 {
		t.Errorf("FileIDs(6) = %v %v", ids, ok)
	}
	if _, ok := index.FileIDs(77); ok {
		t.Error("FileIDs(77) found")
	}
	if index.FileCount(6) != 2 || index.FileCount(77) != 0 {
		t.Error("FileCount")
	}
	archive, err := index.GetArchive(6, nil)
	if err != nil {
		t.Fatal(err)
	}
	if data, ok := archive.GetFile(0); !ok || string(data) != "file0" {
		t.Errorf("file 0 = %q %v", data, ok)
	}
	if data, ok := archive.GetFile(2); !ok || string(data) != "file2" {
		t.Errorf("file 2 = %q %v", data, ok)
	}
	if _, ok := archive.GetFile(1); ok {
		t.Error("gap file 1 present")
	}
	// Zero key: no decryption, same result.
	if _, err := index.GetArchive(6, &[4]int32{}); err != nil {
		t.Errorf("zero-key GetArchive: %v", err)
	}
	data, ok, err := index.GetFile(6, 2, nil)
	if err != nil || !ok || string(data) != "file2" {
		t.Errorf("GetFile = %q %v %v", data, ok, err)
	}
	if _, ok, err := index.GetFile(6, 1, nil); err != nil || ok {
		t.Errorf("GetFile gap = %v %v", ok, err)
	}
	// GetFileSmart: multi-archive index, archive 9 holds file 0.
	data, ok, err = index.GetFileSmart(9, nil)
	if err != nil || !ok || string(data) != "solo" {
		t.Errorf("GetFileSmart(9) = %q %v %v", data, ok, err)
	}
	if _, ok, err := index.GetFileSmart(77, nil); err != nil || ok {
		t.Errorf("GetFileSmart(77) = %v %v", ok, err)
	}
	// Unlisted archive: the store read fails first.
	if _, err := index.GetArchive(77, nil); err == nil {
		t.Error("unlisted archive read, want error")
	}
}

func TestCacheIndexEmptyMeta(t *testing.T) {
	dir := t.TempDir()
	writeStoreFixture(t, dir, map[int][]byte{2: {}}, nil)
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	index, err := NewCacheIndexFromStore(2, store)
	if err != nil {
		t.Fatal(err)
	}
	if index.ArchiveCount() != 0 || index.LastArchiveID() != -1 || index.Table().Protocol != -1 || index.Table().Revision != -1 {
		t.Errorf("invalid table: %+v", index.Table())
	}
}

func TestCacheIndexCorruptMeta(t *testing.T) {
	dir := t.TempDir()
	writeStoreFixture(t, dir, map[int][]byte{2: []byte{0x7f, 1, 2, 3}}, nil)
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := NewCacheIndexFromStore(2, store); err == nil {
		t.Error("corrupt meta decoded, want error")
	} else if !contains(err.Error(), "Failed to decode index: 2") {
		t.Errorf("error = %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// fakeRawStore is a RawStore backed by a map, for index error paths.
type fakeRawStore struct {
	data map[[2]int][]byte
	err  error
}

func (f *fakeRawStore) Read(indexID, archiveID int) ([]byte, error) {
	if f.err != nil {
		return nil, f.err
	}
	data, ok := f.data[[2]int{indexID, archiveID}]
	if !ok {
		return nil, fmt.Errorf("no such archive %d:%d", indexID, archiveID)
	}
	return data, nil
}

func TestCacheIndexReadBeforeCheck(t *testing.T) {
	table := buildRefTable(6, 1, 0, []refArchive{
		{idDelta: 0, crc: 1, revision: 1, fileDeltas: []int{0}},
	})
	meta := plainContainer(table)
	// A store read error propagates even for an unlisted archive.
	store := &fakeRawStore{data: map[[2]int][]byte{{MetaIndexID, 2}: meta}, err: errors.New("disk gone")}
	if _, err := newCacheIndex(2, store); err == nil || err.Error() != "disk gone" {
		t.Errorf("meta read error = %v", err)
	}
	store.err = nil
	index, err := newCacheIndex(2, store)
	if err != nil {
		t.Fatal(err)
	}
	// The store returns bytes for archive 5, but the table does
	// not list it: the reference check fires after the read.
	store.data[[2]int{2, 5}] = plainContainer([]byte("x"))
	if _, err := index.GetArchive(5, nil); err == nil || !contains(err.Error(), "Archive reference not found for: 5") {
		t.Errorf("unlisted GetArchive = %v", err)
	}
	// Single-archive index: GetFileSmart reads archive 0.
	store.data[[2]int{2, 0}] = plainContainer([]byte("only"))
	data, ok, err := index.GetFileSmart(0, nil)
	if err != nil || !ok || string(data) != "only" {
		t.Errorf("single-archive GetFileSmart = %q %v %v", data, ok, err)
	}
}
