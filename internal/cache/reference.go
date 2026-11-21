package cache

import "fmt"

// ReferenceTable is the decoded index of one cache index: which
// archives exist, their CRCs, revisions, name hashes, and file lists.
// It mirrors ReferenceTable (cache/ref/ReferenceTable.ts).
//
// CRCs and revisions are int32, as the TypeScript DataView getInt32
// reads produce signed values. IDs and counts are int.
type ReferenceTable struct {
	// Protocol is the table format version: 5, 6, or 7.
	Protocol int
	// Revision is the table revision (0 for protocol 5, which has none).
	Revision int32
	// Named reports whether the table carries name hashes.
	Named bool
	// UsesWhirlpool reports whether the table carries whirlpool hashes.
	UsesWhirlpool bool
	// ArchiveCount is the number of archives in the table.
	ArchiveCount int
	// LastArchiveID is the highest archive id (ids are cumulative
	// deltas, so the last one read is the largest).
	LastArchiveID int
	// ArchiveIDs lists every archive id, in table order.
	ArchiveIDs []int

	archiveIndex   map[int]int
	nameHashes     []int32
	whirlpools     [][]byte
	crcs           []int32
	revisions      []int32
	fileCounts     []int
	lastFileIDs    []int
	fileIDs        [][]int
	fileNameHashes [][]int32
	nameHashToID   map[int32]int
}

// ArchiveReference describes one archive in a reference table. It
// mirrors ArchiveReference (cache/ref/ArchiveReference.ts).
type ArchiveReference struct {
	ID             int
	NameHash       int32
	Whirlpool      []byte // nil when the table has no whirlpool hashes
	CRC            int32
	Revision       int32
	FileCount      int
	LastFileID     int
	FileIDs        []int
	FileNameHashes []int32 // nil when the table is unnamed

	fileIndex map[int]int
}

// ArchiveFileReference describes one file within an archive. It
// mirrors ArchiveFileReference (cache/ref/ArchiveFileReference.ts).
type ArchiveFileReference struct {
	ID        int
	ArchiveID int
	NameHash  int32
}

// DecodeReferenceTable decodes a reference table from container
// payload bytes (the output of DecodeContainer for a meta-index
// archive). It mirrors ReferenceTable.decode
// (cache/ref/ReferenceTable.ts:64-168), including the flag handling:
// names (0x1), whirlpool hashes (0x2), and per-archive sizes (0x4,
// skipped over). The 0x8 flag (uncompressed CRCs) is read but unused
// in the TypeScript decoder, and likewise here.
//
// Corrupt tables return an error instead of allocating unbounded
// slices: every archive id delta and file id delta consumes at least
// two bytes, so counts larger than half the input size are rejected
// before allocation.
func DecodeReferenceTable(data []byte) (*ReferenceTable, error) {
	buf := NewByteBuffer(data)
	protocol, err := buf.ReadUnsignedByte()
	if err != nil {
		return nil, fmt.Errorf("decoding reference table: %w", err)
	}
	if protocol < 5 || protocol > 7 {
		return nil, fmt.Errorf("Invalid protocol: %d", protocol)
	}
	var revision int32
	if protocol > 5 {
		revision, err = buf.ReadInt()
		if err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
	}
	flag, err := buf.ReadUnsignedByte()
	if err != nil {
		return nil, fmt.Errorf("decoding reference table: %w", err)
	}
	hasNames := flag&0x1 != 0
	hasWhirlpool := flag&0x2 != 0
	hasSizes := flag&0x4 != 0
	// flag & 0x8 (hasUncompressedCrcs) is parsed and ignored, as in
	// ReferenceTable.decode.

	archiveCount, err := readCount(buf, protocol)
	if err != nil {
		return nil, fmt.Errorf("decoding reference table: %w", err)
	}
	if archiveCount < 0 || archiveCount*2 > len(data) {
		return nil, fmt.Errorf("decoding reference table: implausible archive count %d for %d bytes", archiveCount, len(data))
	}

	table := &ReferenceTable{
		Protocol:       protocol,
		Revision:       revision,
		Named:          hasNames,
		UsesWhirlpool:  hasWhirlpool,
		ArchiveCount:   archiveCount,
		ArchiveIDs:     make([]int, archiveCount),
		archiveIndex:   make(map[int]int, archiveCount),
		nameHashes:     make([]int32, archiveCount),
		whirlpools:     make([][]byte, archiveCount),
		crcs:           make([]int32, archiveCount),
		revisions:      make([]int32, archiveCount),
		fileCounts:     make([]int, archiveCount),
		lastFileIDs:    make([]int, archiveCount),
		fileIDs:        make([][]int, archiveCount),
		fileNameHashes: make([][]int32, archiveCount),
		nameHashToID:   make(map[int32]int),
	}

	lastArchiveID := 0
	for i := 0; i < archiveCount; i++ {
		delta, err := readCount(buf, protocol)
		if err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
		lastArchiveID += delta
		table.ArchiveIDs[i] = lastArchiveID
		table.archiveIndex[lastArchiveID] = i
	}
	table.LastArchiveID = lastArchiveID

	if hasNames {
		for i := 0; i < archiveCount; i++ {
			hash, err := buf.ReadInt()
			if err != nil {
				return nil, fmt.Errorf("decoding reference table: %w", err)
			}
			table.nameHashes[i] = hash
		}
	}
	if hasWhirlpool {
		for i := 0; i < archiveCount; i++ {
			hash, err := buf.ReadBytes(64)
			if err != nil {
				return nil, fmt.Errorf("decoding reference table: %w", err)
			}
			table.whirlpools[i] = append([]byte(nil), hash...)
		}
	}
	for i := 0; i < archiveCount; i++ {
		crc, err := buf.ReadInt()
		if err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
		table.crcs[i] = crc
	}
	if hasSizes {
		// Two 4-byte sizes per archive, skipped (ReferenceTable.ts:111-113).
		if _, err := buf.ReadBytes(archiveCount * 8); err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
	}
	for i := 0; i < archiveCount; i++ {
		rev, err := buf.ReadInt()
		if err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
		table.revisions[i] = rev
	}

	totalFiles := 0
	for i := 0; i < archiveCount; i++ {
		count, err := readCount(buf, protocol)
		if err != nil {
			return nil, fmt.Errorf("decoding reference table: %w", err)
		}
		if count < 0 {
			return nil, fmt.Errorf("decoding reference table: negative file count %d for archive %d", count, table.ArchiveIDs[i])
		}
		totalFiles += count
		if totalFiles*2 > len(data) {
			return nil, fmt.Errorf("decoding reference table: implausible file count %d for %d bytes", totalFiles, len(data))
		}
		table.fileCounts[i] = count
	}
	for archiveIdx := 0; archiveIdx < archiveCount; archiveIdx++ {
		ids := make([]int, table.fileCounts[archiveIdx])
		lastFileID := 0
		for fileIdx := range ids {
			delta, err := readCount(buf, protocol)
			if err != nil {
				return nil, fmt.Errorf("decoding reference table: %w", err)
			}
			lastFileID += delta
			ids[fileIdx] = lastFileID
		}
		table.fileIDs[archiveIdx] = ids
		table.lastFileIDs[archiveIdx] = lastFileID
	}

	if hasNames {
		for archiveIdx := 0; archiveIdx < archiveCount; archiveIdx++ {
			hashes := make([]int32, table.fileCounts[archiveIdx])
			for fileIdx := range hashes {
				hash, err := buf.ReadInt()
				if err != nil {
					return nil, fmt.Errorf("decoding reference table: %w", err)
				}
				hashes[fileIdx] = hash
			}
			table.fileNameHashes[archiveIdx] = hashes
		}
		for i, id := range table.ArchiveIDs {
			table.nameHashToID[table.nameHashes[i]] = id
		}
	}
	return table, nil
}

// readCount reads an archive count, archive id delta, file count, or
// file id delta: a big smart for protocol 7, an unsigned short
// otherwise (ReferenceTable.decode).
func readCount(buf *ByteBuffer, protocol int) (int, error) {
	if protocol == 7 {
		return buf.ReadBigSmart()
	}
	return buf.ReadUnsignedShort()
}

// GetArchiveID returns the archive id for a name, hashed with djb2, or
// false when the table is unnamed or has no such archive. It mirrors
// ReferenceTable.getArchiveId (ReferenceTable.ts:196-198).
func (t *ReferenceTable) GetArchiveID(name string) (int, bool) {
	if t == nil || !t.Named {
		return 0, false
	}
	id, ok := t.nameHashToID[HashDjb2(name)]
	return id, ok
}

// ArchiveExists reports whether the table lists the archive id.
func (t *ReferenceTable) ArchiveExists(id int) bool {
	if t == nil {
		return false
	}
	_, ok := t.archiveIndex[id]
	return ok
}

// GetArchiveReference returns the reference for one archive, or false
// when the table does not list it. It mirrors
// ReferenceTable.getArchiveReference (ReferenceTable.ts:204-235).
func (t *ReferenceTable) GetArchiveReference(id int) (*ArchiveReference, bool) {
	if t == nil {
		return nil, false
	}
	i, ok := t.archiveIndex[id]
	if !ok {
		return nil, false
	}
	fileIndex := make(map[int]int, t.fileCounts[i])
	for fileIdx, fileID := range t.fileIDs[i] {
		fileIndex[fileID] = fileIdx
	}
	return &ArchiveReference{
		ID:             id,
		NameHash:       t.nameHashes[i],
		Whirlpool:      t.whirlpools[i],
		CRC:            t.crcs[i],
		Revision:       t.revisions[i],
		FileCount:      t.fileCounts[i],
		LastFileID:     t.lastFileIDs[i],
		FileIDs:        t.fileIDs[i],
		FileNameHashes: t.fileNameHashes[i],
		fileIndex:      fileIndex,
	}, true
}

// ArchiveReferences returns a reference for every archive in table order.
func (t *ReferenceTable) ArchiveReferences() []*ArchiveReference {
	if t == nil {
		return nil
	}
	refs := make([]*ArchiveReference, len(t.ArchiveIDs))
	for i, id := range t.ArchiveIDs {
		ref, _ := t.GetArchiveReference(id)
		refs[i] = ref
	}
	return refs
}

// GetFileReference returns the reference for one file in the archive,
// or false when the archive has no such file. It mirrors
// ArchiveReference.getFileReference (ArchiveReference.ts:17-28), with
// one correction: the TypeScript version passes the file id as the
// ArchiveFileReference's archiveId (a latent upstream bug —
// ArchiveReference.getFileReference forwards its own `id` parameter
// into the archiveId slot). Here ArchiveID is the archive's id.
func (r *ArchiveReference) GetFileReference(id int) (*ArchiveFileReference, bool) {
	if r == nil {
		return nil, false
	}
	i, ok := r.fileIndex[id]
	if !ok {
		return nil, false
	}
	var nameHash int32
	if r.FileNameHashes != nil {
		nameHash = r.FileNameHashes[i]
	}
	return &ArchiveFileReference{ID: r.FileIDs[i], ArchiveID: r.ID, NameHash: nameHash}, true
}

// FileReferences returns a reference for every file in the archive.
func (r *ArchiveReference) FileReferences() []*ArchiveFileReference {
	if r == nil {
		return nil
	}
	refs := make([]*ArchiveFileReference, len(r.FileIDs))
	for i, id := range r.FileIDs {
		ref, _ := r.GetFileReference(id)
		refs[i] = ref
	}
	return refs
}
