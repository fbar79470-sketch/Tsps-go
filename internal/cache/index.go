package cache

import "fmt"

// This file ports the dat2 cache-index facade: CacheIndex.ts
// (CacheIndex, CacheStoreIndexSync, CacheIndexDat2), composed from
// the already-ported Store, DecodeContainer, DecodeReferenceTable
// and DecodeArchive pieces.
//
// Upstream source: codec/rs/cache/CacheIndex.ts.

// RawStore reads raw archive bytes from one cache index. *Store
// satisfies it. It is the sync CacheStore surface CacheIndexDat2
// needs (CacheIndex.ts:297-299).
type RawStore interface {
	Read(indexID, archiveID int) ([]byte, error)
}

// ArchiveSource serves decoded archives of one cache index. It is
// the small interface the definition loaders, Gamevals and the
// facade build on: production code uses *CacheIndex (backed by the
// real Store), while unit tests substitute an in-memory fake.
type ArchiveSource interface {
	// GetArchive returns the decoded archive, decrypting its
	// container with key when key is non-nil and valid.
	GetArchive(archiveID int, key *[4]int32) (*Archive, error)
	// ArchiveCount is the number of archives the index lists.
	ArchiveCount() int
	// LastArchiveID is the highest archive id the index lists.
	LastArchiveID() int
	// FileCount is the number of files in one archive, or 0 when
	// the index does not list it.
	FileCount(archiveID int) int
}

// IndexFactory builds the ArchiveSource for one cache index. The
// facade uses it to reach the configs, gamevals and sprites indexes
// without depending on a concrete Store; production factories wrap
// NewCacheIndexFromStore, tests return fakes.
type IndexFactory func(indexID int) (ArchiveSource, error)

// CacheIndex is one cache index backed by a Store: its reference
// table plus archive decoding. It mirrors CacheIndexDat2
// (CacheIndex.ts:377-394) and the shared CacheIndex surface
// (CacheIndex.ts:225-285). Only the synchronous dat2 variant is
// ported; the legacy DAT and async variants are not used by the
// server facade.
type CacheIndex struct {
	id    int
	table *ReferenceTable
	store RawStore
}

// NewCacheIndexFromStore builds the index id from store, reading
// its reference table out of the meta index (255), mirroring
// CacheIndexDat2.fromStore (CacheIndex.ts:378-388). An empty meta
// archive yields the empty (INVALID_TABLE) table rather than an
// error, as decodeTable does (CacheIndex.ts:348-354).
func NewCacheIndexFromStore(id int, store *Store) (*CacheIndex, error) {
	return newCacheIndex(id, store)
}

func newCacheIndex(id int, store RawStore) (*CacheIndex, error) {
	data, err := store.Read(MetaIndexID, id)
	if err != nil {
		return nil, err
	}
	table, err := decodeIndexTable(id, data)
	if err != nil {
		return nil, err
	}
	return &CacheIndex{id: id, table: table, store: store}, nil
}

// decodeIndexTable decodes a reference table from meta-index
// archive bytes, mirroring decodeTable (CacheIndex.ts:348-354).
func decodeIndexTable(id int, data []byte) (*ReferenceTable, error) {
	if len(data) == 0 {
		return invalidReferenceTable(), nil
	}
	payload, err := DecodeContainer(data, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode index: %d (%v)", id, err)
	}
	table, err := DecodeReferenceTable(payload)
	if err != nil {
		return nil, fmt.Errorf("Failed to decode index: %d (%v)", id, err)
	}
	return table, nil
}

// invalidReferenceTable is ReferenceTable.INVALID_TABLE
// (ref/ReferenceTable.ts:6-21): protocol and revision -1, no
// archives, last archive id -1. The zero-value maps make every
// lookup report absence.
func invalidReferenceTable() *ReferenceTable {
	return &ReferenceTable{Protocol: -1, Revision: -1, ArchiveCount: 0, LastArchiveID: -1, ArchiveIDs: []int{}}
}

// ID returns the index id.
func (c *CacheIndex) ID() int { return c.id }

// Table returns the decoded reference table.
func (c *CacheIndex) Table() *ReferenceTable { return c.table }

// Named reports whether the reference table carries name hashes.
func (c *CacheIndex) Named() bool { return c.table.Named }

// ArchiveIDs lists every archive id in table order.
func (c *CacheIndex) ArchiveIDs() []int { return c.table.ArchiveIDs }

// ArchiveCount returns the number of archives in the table.
func (c *CacheIndex) ArchiveCount() int { return c.table.ArchiveCount }

// LastArchiveID returns the highest archive id in the table.
func (c *CacheIndex) LastArchiveID() int { return c.table.LastArchiveID }

// ArchiveReference returns the reference-table entry for one
// archive, or false when the table does not list it.
func (c *CacheIndex) ArchiveReference(archiveID int) (*ArchiveReference, bool) {
	return c.table.GetArchiveReference(archiveID)
}

// ArchiveID returns the archive id for a name, hashed with djb2, or
// -1 when the table is unnamed or has no such archive. It mirrors
// CacheIndex.getArchiveId (CacheIndex.ts:249-251).
func (c *CacheIndex) ArchiveID(name string) int {
	if id, ok := c.table.GetArchiveID(name); ok {
		return id
	}
	return -1
}

// FileIDs returns the file ids of one archive, or false when the
// table does not list it.
func (c *CacheIndex) FileIDs(archiveID int) ([]int, bool) {
	ref, ok := c.table.GetArchiveReference(archiveID)
	if !ok {
		return nil, false
	}
	return ref.FileIDs, true
}

// ArchiveExists reports whether the table lists the archive id.
func (c *CacheIndex) ArchiveExists(archiveID int) bool { return c.table.ArchiveExists(archiveID) }

// FileCount returns the number of files in one archive, or 0 when
// the table does not list it. It mirrors CacheIndex.getFileCount
// (CacheIndex.ts:261-263).
func (c *CacheIndex) FileCount(archiveID int) int {
	ref, ok := c.table.GetArchiveReference(archiveID)
	if !ok {
		return 0
	}
	return ref.FileCount
}

// GetArchive reads and decodes one archive: store read, container
// decode (with key when given), archive decode. It mirrors
// CacheIndexDat2.getArchive plus decodeArchiveData
// (CacheIndex.ts:356-375, 390-393), including the order: the store
// read happens before the reference-table check, so a read failure
// is reported even for an unlisted archive.
func (c *CacheIndex) GetArchive(id int, key *[4]int32) (*Archive, error) {
	data, err := c.store.Read(c.id, id)
	if err != nil {
		return nil, err
	}
	ref, ok := c.table.GetArchiveReference(id)
	if !ok {
		return nil, fmt.Errorf("Archive reference not found for: %d", id)
	}
	payload, err := DecodeContainer(data, key)
	if err != nil {
		return nil, err
	}
	return DecodeArchive(id, ref.LastFileID, ref.FileCount, ref.FileIDs, ref.FileNameHashes, payload)
}

// GetFile returns one file's bytes: the decoded archive's file, or
// found=false when the archive has no such file. Archive read and
// decode failures are errors, as getArchive throwing is in
// CacheStoreIndexSync.getFile (CacheIndex.ts:301-305).
func (c *CacheIndex) GetFile(archiveID, fileID int, key *[4]int32) ([]byte, bool, error) {
	archive, err := c.GetArchive(archiveID, key)
	if err != nil {
		return nil, false, err
	}
	data, ok := archive.GetFile(fileID)
	return data, ok, nil
}

// GetFileSmart resolves a file id the way CacheIndex.getFileSmart
// does (CacheIndex.ts:273-285): in a single-archive index the file
// lives in archive 0; otherwise each archive holds one file and the
// archive id is the requested id. A missing archive reports
// not-found rather than an error.
func (c *CacheIndex) GetFileSmart(id int, key *[4]int32) ([]byte, bool, error) {
	if c.ArchiveCount() == 1 {
		return c.GetFile(0, id, key)
	}
	if c.FileCount(id) >= 1 {
		return c.GetFile(id, 0, key)
	}
	return nil, false, nil
}
