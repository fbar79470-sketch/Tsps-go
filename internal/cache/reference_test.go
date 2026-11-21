package cache

import (
	"bytes"
	"encoding/binary"
	"strings"
	"testing"
)

// refTableFixture builds a reference table with the given protocol,
// following the ReferenceTable.decode layout: protocol byte, revision
// (protocols 6-7), flags, archive count, cumulative archive id
// deltas, name hashes, CRCs, sizes (flag 0x4), revisions, file
// counts, cumulative file id deltas, file name hashes.
type refArchive struct {
	idDelta    int
	nameHash   int32
	crc        int32
	revision   int32
	fileDeltas []int
	fileHashes []int32
}

func buildRefTable(protocol int, revision int32, flags byte, archives []refArchive) []byte {
	buf := &bytes.Buffer{}
	buf.WriteByte(byte(protocol))
	if protocol > 5 {
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], uint32(revision))
		buf.Write(tmp[:])
	}
	buf.WriteByte(flags)
	writeCount := func(v int) {
		var tmp [4]byte
		if protocol == 7 {
			// Big smart, small-value form: unsigned short.
			binary.BigEndian.PutUint16(tmp[:2], uint16(v))
			buf.Write(tmp[:2])
		} else {
			binary.BigEndian.PutUint16(tmp[:2], uint16(v))
			buf.Write(tmp[:2])
		}
	}
	writeInt := func(v int32) {
		var tmp [4]byte
		binary.BigEndian.PutUint32(tmp[:], uint32(v))
		buf.Write(tmp[:])
	}
	writeCount(len(archives))
	for _, a := range archives {
		writeCount(a.idDelta)
	}
	if flags&0x1 != 0 {
		for _, a := range archives {
			writeInt(a.nameHash)
		}
	}
	if flags&0x2 != 0 {
		for range archives {
			buf.Write(make([]byte, 64))
		}
	}
	for _, a := range archives {
		writeInt(a.crc)
	}
	if flags&0x4 != 0 {
		for range archives {
			buf.Write(make([]byte, 8))
		}
	}
	for _, a := range archives {
		writeInt(a.revision)
	}
	for _, a := range archives {
		writeCount(len(a.fileDeltas))
	}
	for _, a := range archives {
		for _, d := range a.fileDeltas {
			writeCount(d)
		}
	}
	if flags&0x1 != 0 {
		for _, a := range archives {
			for _, h := range a.fileHashes {
				writeInt(h)
			}
		}
	}
	return buf.Bytes()
}

func TestDecodeReferenceTableProtocol6(t *testing.T) {
	// Two archives: ids 5 (delta 5) and 8 (delta 3). Archive 5 has
	// files 0 and 2; archive 8 has file 7. Names on.
	data := buildRefTable(6, 1234, 0x1, []refArchive{
		{idDelta: 5, nameHash: 111, crc: -222, revision: 3, fileDeltas: []int{0, 2}, fileHashes: []int32{10, 20}},
		{idDelta: 3, nameHash: 333, crc: 444, revision: 4, fileDeltas: []int{7}, fileHashes: []int32{30}},
	})
	table, err := DecodeReferenceTable(data)
	if err != nil {
		t.Fatalf("DecodeReferenceTable: %v", err)
	}
	if table.Protocol != 6 || table.Revision != 1234 || !table.Named || table.UsesWhirlpool {
		t.Errorf("header: %+v", table)
	}
	if table.ArchiveCount != 2 || table.LastArchiveID != 8 {
		t.Errorf("counts: %d / last %d", table.ArchiveCount, table.LastArchiveID)
	}
	if len(table.ArchiveIDs) != 2 || table.ArchiveIDs[0] != 5 || table.ArchiveIDs[1] != 8 {
		t.Errorf("ArchiveIDs = %v", table.ArchiveIDs)
	}
	if !table.ArchiveExists(5) || table.ArchiveExists(6) {
		t.Error("ArchiveExists wrong")
	}
	ref, ok := table.GetArchiveReference(5)
	if !ok {
		t.Fatal("archive 5 missing")
	}
	if ref.NameHash != 111 || ref.CRC != -222 || ref.Revision != 3 || ref.FileCount != 2 || ref.LastFileID != 2 {
		t.Errorf("archive 5 ref: %+v", ref)
	}
	if len(ref.FileIDs) != 2 || ref.FileIDs[0] != 0 || ref.FileIDs[1] != 2 {
		t.Errorf("archive 5 FileIDs = %v", ref.FileIDs)
	}
	if len(ref.FileNameHashes) != 2 || ref.FileNameHashes[1] != 20 {
		t.Errorf("archive 5 FileNameHashes = %v", ref.FileNameHashes)
	}
	fileRef, ok := ref.GetFileReference(2)
	if !ok || fileRef.ID != 2 || fileRef.ArchiveID != 5 || fileRef.NameHash != 20 {
		t.Errorf("file ref: %+v, %v", fileRef, ok)
	}
	if _, ok := ref.GetFileReference(1); ok {
		t.Error("file 1 should not exist in archive 5")
	}
	if len(table.ArchiveReferences()) != 2 {
		t.Error("ArchiveReferences length")
	}
	// Name lookup of an unknown name fails (a positive lookup case
	// is covered by TestDecodeReferenceTableNameLookup).
	if _, ok := table.GetArchiveID("definitely-not-present"); ok {
		t.Error("unknown name resolved")
	}
}

func TestDecodeReferenceTableNameLookup(t *testing.T) {
	// Build a table whose archive name hash is HashDjb2("l42_56"),
	// a map-archive-style name, and verify GetArchiveID finds it.
	name := "l42_56"
	data := buildRefTable(6, 0, 0x1, []refArchive{
		{idDelta: 9, nameHash: HashDjb2(name), crc: 1, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{0}},
	})
	table, err := DecodeReferenceTable(data)
	if err != nil {
		t.Fatal(err)
	}
	id, ok := table.GetArchiveID(name)
	if !ok || id != 9 {
		t.Errorf("GetArchiveID(%q) = %d, %v, want 9, true", name, id, ok)
	}
}

func TestDecodeReferenceTableProtocol7AndFlags(t *testing.T) {
	// Protocol 7, no revision check beyond parsing, with whirlpool
	// (0x2) and sizes (0x4) flags set; no names.
	data := buildRefTable(7, 99, 0x6, []refArchive{
		{idDelta: 1, crc: 7, revision: 8, fileDeltas: []int{3}},
	})
	table, err := DecodeReferenceTable(data)
	if err != nil {
		t.Fatalf("DecodeReferenceTable: %v", err)
	}
	if table.Protocol != 7 || table.Revision != 99 || table.Named || !table.UsesWhirlpool {
		t.Errorf("header: %+v", table)
	}
	ref, ok := table.GetArchiveReference(1)
	if !ok || ref.CRC != 7 || ref.Revision != 8 || len(ref.Whirlpool) != 64 || ref.FileNameHashes != nil {
		t.Errorf("ref: %+v, %v", ref, ok)
	}
	if _, ok := table.GetArchiveID("x"); ok {
		t.Error("unnamed table resolved a name")
	}
}

func TestDecodeReferenceTableProtocol5(t *testing.T) {
	data := buildRefTable(5, 0, 0x0, []refArchive{
		{idDelta: 0, crc: 1, revision: 1, fileDeltas: []int{0}},
	})
	table, err := DecodeReferenceTable(data)
	if err != nil {
		t.Fatal(err)
	}
	if table.Protocol != 5 || table.Revision != 0 {
		t.Errorf("protocol 5 header: %+v", table)
	}
}

func TestDecodeReferenceTableErrors(t *testing.T) {
	if _, err := DecodeReferenceTable(nil); err == nil {
		t.Error("empty table decoded, want error")
	}
	if _, err := DecodeReferenceTable([]byte{4, 0}); err == nil || !strings.Contains(err.Error(), "Invalid protocol: 4") {
		t.Errorf("protocol 4: err = %v", err)
	}
	if _, err := DecodeReferenceTable([]byte{8, 0}); err == nil || !strings.Contains(err.Error(), "Invalid protocol: 8") {
		t.Errorf("protocol 8: err = %v", err)
	}
	// Truncated mid-table.
	full := buildRefTable(6, 1, 0x1, []refArchive{{idDelta: 1, nameHash: 1, crc: 1, revision: 1, fileDeltas: []int{0}, fileHashes: []int32{1}}})
	if _, err := DecodeReferenceTable(full[:len(full)-3]); err == nil {
		t.Error("truncated table decoded, want error")
	}
	// Absurd archive count (would OOM if allocated blindly).
	huge := []byte{6, 0, 0, 0, 0, 0, 0xff, 0xff}
	if _, err := DecodeReferenceTable(huge); err == nil {
		t.Error("huge archive count decoded, want error")
	}
}
