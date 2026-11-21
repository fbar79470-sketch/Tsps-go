package cache

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeMedium(buf []byte, v int) {
	buf[0] = byte(v >> 16)
	buf[1] = byte(v >> 8)
	buf[2] = byte(v)
}

// buildSector assembles one 520-byte sector: header + payload,
// zero-padded. Standard layout: archive u16, chunk u16, next medium,
// index u8. Extended: archive int32, then the same tail.
func buildSector(extended bool, archiveID, chunk, nextSector, indexID int, payload []byte) []byte {
	sector := make([]byte, SectorSize)
	if extended {
		binary.BigEndian.PutUint32(sector[0:4], uint32(archiveID))
		binary.BigEndian.PutUint16(sector[4:6], uint16(chunk))
		writeMedium(sector[6:9], nextSector)
		sector[9] = byte(indexID)
		copy(sector[SectorExtendedHeaderSize:], payload)
	} else {
		binary.BigEndian.PutUint16(sector[0:2], uint16(archiveID))
		binary.BigEndian.PutUint16(sector[2:4], uint16(chunk))
		writeMedium(sector[4:7], nextSector)
		sector[7] = byte(indexID)
		copy(sector[SectorHeaderSize:], payload)
	}
	return sector
}

// writeIndex writes an index file with the given archive entries
// (archive id -> {size, first sector}).
func writeIndex(t *testing.T, dir string, indexID int, entries map[int][2]int) {
	t.Helper()
	maxID := -1
	for id := range entries {
		if id > maxID {
			maxID = id
		}
	}
	contents := make([]byte, (maxID+1)*SectorClusterSize)
	for id, entry := range entries {
		off := id * SectorClusterSize
		writeMedium(contents[off:off+3], entry[0])
		writeMedium(contents[off+3:off+6], entry[1])
	}
	path := filepath.Join(dir, IndexFilePrefix+itoa(indexID))
	if err := os.WriteFile(path, contents, 0o644); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func writeDat2(t *testing.T, dir string, sectors ...[]byte) {
	t.Helper()
	var data []byte
	for _, s := range sectors {
		data = append(data, s...)
	}
	if err := os.WriteFile(filepath.Join(dir, Dat2FileName), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// withMeta writes a dummy idx255 so the store uses the meta-index
// sector-id rule (sector index id == index id).
func withMeta(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, MetaFileName), make([]byte, 150), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStoreSingleSector(t *testing.T) {
	dir := t.TempDir()
	withMeta(t, dir)
	writeIndex(t, dir, 2, map[int][2]int{6: {5, 0}})
	writeDat2(t, dir, buildSector(false, 6, 0, 0, 2, []byte("hello")))
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Read(2, 6)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Errorf("Read = %q", data)
	}
}

func TestStoreMultiSector(t *testing.T) {
	dir := t.TempDir()
	withMeta(t, dir)
	payload := bytes.Repeat([]byte("abcdefgh"), 150) // 1200 bytes
	writeIndex(t, dir, 2, map[int][2]int{9: {len(payload), 0}})
	writeDat2(t, dir,
		buildSector(false, 9, 0, 1, 2, payload[0:512]),
		buildSector(false, 9, 1, 2, 2, payload[512:1024]),
		buildSector(false, 9, 2, 0, 2, payload[1024:]),
	)
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Read(2, 9)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Error("multi-sector payload mismatch")
	}
}

func TestStoreExtendedArchiveID(t *testing.T) {
	dir := t.TempDir()
	withMeta(t, dir)
	payload := []byte("extended!!")
	writeIndex(t, dir, 5, map[int][2]int{70000: {len(payload), 0}})
	writeDat2(t, dir, buildSector(true, 70000, 0, 0, 5, payload))
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Read(5, 70000)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Errorf("Read = %q", data)
	}
}

func TestStoreExtendedMultiSector(t *testing.T) {
	dir := t.TempDir()
	withMeta(t, dir)
	payload := bytes.Repeat([]byte("x"), 600) // 510 + 90
	writeIndex(t, dir, 5, map[int][2]int{70001: {len(payload), 0}})
	writeDat2(t, dir,
		buildSector(true, 70001, 0, 1, 5, payload[0:510]),
		buildSector(true, 70001, 1, 0, 5, payload[510:]),
	)
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Read(5, 70001)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, payload) {
		t.Error("extended multi-sector payload mismatch")
	}
}

func TestStoreWithoutMetaIndex(t *testing.T) {
	dir := t.TempDir()
	// No idx255: sector index ids are indexId+1.
	writeIndex(t, dir, 2, map[int][2]int{6: {3, 0}})
	writeDat2(t, dir, buildSector(false, 6, 0, 0, 3, []byte("abc")))
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	data, err := store.Read(2, 6)
	if err != nil {
		t.Fatalf("Read without meta index: %v", err)
	}
	if string(data) != "abc" {
		t.Errorf("Read = %q", data)
	}
}

func TestStoreSectorValidation(t *testing.T) {
	newStore := func(t *testing.T, sector []byte) *Store {
		t.Helper()
		dir := t.TempDir()
		withMeta(t, dir)
		writeIndex(t, dir, 2, map[int][2]int{6: {5, 0}})
		writeDat2(t, dir, sector)
		store, err := OpenStore(dir)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { store.Close() })
		return store
	}
	cases := []struct {
		name   string
		sector []byte
	}{
		{"wrong index id", buildSector(false, 6, 0, 0, 3, []byte("hello"))},
		{"wrong archive id", buildSector(false, 7, 0, 0, 2, []byte("hello"))},
		{"wrong chunk", buildSector(false, 6, 1, 0, 2, []byte("hello"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newStore(t, tc.sector)
			if _, err := store.Read(2, 6); err == nil || !strings.Contains(err.Error(), "Invalid cache sector") {
				t.Errorf("err = %v", err)
			}
		})
	}
}

func TestStoreErrors(t *testing.T) {
	dir := t.TempDir()
	withMeta(t, dir)
	writeIndex(t, dir, 2, map[int][2]int{6: {5, 0}, 7: {5, 100}})
	writeDat2(t, dir, buildSector(false, 6, 0, 0, 2, []byte("hello")))
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, err := store.Read(99, 0); err == nil || !strings.Contains(err.Error(), "Index 99 not found") {
		t.Errorf("missing index: err = %v", err)
	}
	if _, err := store.Read(2, 100); err == nil || !strings.Contains(err.Error(), "Invalid ptr") {
		t.Errorf("bad ptr: err = %v", err)
	}
	if _, err := store.Read(2, -1); err == nil || !strings.Contains(err.Error(), "Invalid ptr") {
		t.Errorf("negative archive: err = %v", err)
	}
	if _, err := store.Read(2, 7); err == nil || !strings.Contains(err.Error(), "Invalid cache sector offset") {
		t.Errorf("sector past EOF: err = %v", err)
	}
	// Zero-size archive reads empty without touching the data file.
	writeIndex(t, dir, 3, map[int][2]int{0: {0, 0}})
	store2, err := OpenStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer store2.Close()
	data, err := store2.Read(3, 0)
	if err != nil || len(data) != 0 {
		t.Errorf("empty archive: data = %v, err = %v", data, err)
	}
}

func TestStoreOpenErrorsAndSkips(t *testing.T) {
	// Missing dat2.
	if _, err := OpenStore(t.TempDir()); err == nil {
		t.Error("OpenStore without dat2 succeeded, want error")
	}
	// Index files with non-integer suffixes are skipped, not errors.
	dir := t.TempDir()
	withMeta(t, dir)
	if err := os.WriteFile(filepath.Join(dir, IndexFilePrefix+"ABC"), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, IndexFilePrefix), []byte("junk"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeIndex(t, dir, 2, map[int][2]int{6: {1, 0}})
	writeDat2(t, dir, buildSector(false, 6, 0, 0, 2, []byte("z")))
	store, err := OpenStore(dir)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer store.Close()
	if _, err := store.Read(2, 6); err != nil {
		t.Errorf("Read: %v", err)
	}
	// Close is idempotent.
	if err := store.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}
