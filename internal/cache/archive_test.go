package cache

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

func TestDecodeArchiveSingleFile(t *testing.T) {
	payload := []byte("the whole archive is one file")
	archive, err := DecodeArchive(10, 3, 1, []int{3}, []int32{HashDjb2("only")}, payload)
	if err != nil {
		t.Fatalf("DecodeArchive: %v", err)
	}
	data, ok := archive.GetFile(3)
	if !ok || !bytes.Equal(data, payload) {
		t.Errorf("GetFile(3) = %q, %v", data, ok)
	}
	if _, ok := archive.GetFile(0); ok {
		t.Error("file 0 should not exist")
	}
	if got := archive.GetFileID("only"); got != 3 {
		t.Errorf("GetFileID = %d", got)
	}
	if _, ok := archive.GetFileNamed("only"); !ok {
		t.Error("GetFileNamed failed")
	}
	if got := archive.GetFileID("missing"); got != -1 {
		t.Errorf("GetFileID(missing) = %d", got)
	}
}

// buildMultiFileArchive assembles a multi-file payload: chunk data
// first (one slice per file per chunk), then the trailer of
// delta-encoded chunk sizes, then the chunk count byte.
func buildMultiFileArchive(t *testing.T, chunks [][][]byte) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	for _, chunk := range chunks {
		for _, fileData := range chunk {
			buf.Write(fileData)
		}
	}
	var tmp [4]byte
	for _, chunk := range chunks {
		last := 0
		for _, fileData := range chunk {
			delta := len(fileData) - last
			last = len(fileData)
			binary.BigEndian.PutUint32(tmp[:], uint32(int32(delta)))
			buf.Write(tmp[:])
		}
	}
	buf.WriteByte(byte(len(chunks)))
	return buf.Bytes()
}

func TestDecodeArchiveMultiFile(t *testing.T) {
	// Two files, two chunks: file 0 = "AAA"+"BB", file 2 = "C"+"DDDD".
	payload := buildMultiFileArchive(t, [][][]byte{
		{[]byte("AAA"), []byte("C")},
		{[]byte("BB"), []byte("DDDD")},
	})
	archive, err := DecodeArchive(7, 2, 2, []int{0, 2}, nil, payload)
	if err != nil {
		t.Fatalf("DecodeArchive: %v", err)
	}
	if data, ok := archive.GetFile(0); !ok || string(data) != "AAABB" {
		t.Errorf("file 0 = %q, %v", data, ok)
	}
	if data, ok := archive.GetFile(2); !ok || string(data) != "CDDDD" {
		t.Errorf("file 2 = %q, %v", data, ok)
	}
	if archive.GetFileID("anything") != -1 {
		t.Error("unnamed archive resolved a name")
	}
}

func TestDecodeArchiveErrors(t *testing.T) {
	if _, err := DecodeArchive(1, 0, 1, []int{0}, nil, nil); err == nil {
		t.Error("empty data decoded, want error")
	}
	if _, err := DecodeArchive(1, 0, 2, []int{0}, nil, []byte("x")); err == nil {
		t.Error("file id count mismatch decoded, want error")
	}
	// Trailer overruns the payload: chunk count byte claims 5 chunks
	// for 2 files, needing 40 trailer bytes in a 3-byte payload.
	if _, err := DecodeArchive(1, 1, 2, []int{0, 1}, nil, []byte{1, 2, 5}); err == nil {
		t.Error("overrunning trailer decoded, want error")
	}
	// Chunk data overruns the trailer: sizes claim more data than exists.
	payload := buildMultiFileArchive(t, [][][]byte{{[]byte("AAAA"), []byte("BB")}})
	// Corrupt the first size delta to a huge value.
	payload[len(payload)-9] = 0x7f
	if _, err := DecodeArchive(1, 1, 2, []int{0, 1}, nil, payload); err == nil || !strings.Contains(err.Error(), "overruns") {
		t.Errorf("oversized chunks: err = %v", err)
	}
}
