package cache

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Store reads raw archive bytes from a cache directory on disk. It
// mirrors FileStore (cache/store/FileStore.ts): the small index files
// are loaded into memory at open time, while the large data file is
// read on demand through one file descriptor.
//
// Read is safe for concurrent use (reads go through ReadAt, and the
// in-memory indexes are never mutated after OpenStore). Close must
// not race with Read.
type Store struct {
	file         *os.File
	dataFileSize int64
	indexes      map[int][]byte
	hasMetaIndex bool

	mu     sync.Mutex
	closed bool
}

// OpenStore opens the cache directory: main_file_cache.dat2 plus every
// main_file_cache.idx* file in it (FileStore.ts:18-32). A directory
// entry whose name has a non-integer index suffix is skipped, as in
// the TypeScript version.
func OpenStore(dir string) (*Store, error) {
	dataPath := filepath.Join(dir, Dat2FileName)
	file, err := os.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("opening cache store in %s: %w", dir, err)
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("opening cache store in %s: %w", dir, err)
	}
	store := &Store{
		file:         file,
		dataFileSize: info.Size(),
		indexes:      make(map[int][]byte),
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("opening cache store in %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, IndexFilePrefix) {
			continue
		}
		indexID, err := strconv.Atoi(name[len(IndexFilePrefix):])
		if err != nil || indexID < 0 {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			file.Close()
			return nil, fmt.Errorf("opening cache store in %s: reading %s: %w", dir, name, err)
		}
		store.indexes[indexID] = contents
	}
	_, store.hasMetaIndex = store.indexes[MetaIndexID]
	return store, nil
}

// Close closes the data file descriptor. It is idempotent; closing an
// already-closed store returns nil.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.file.Close()
}

// Read returns the raw (still container-encoded) bytes of one archive.
// It mirrors FileStore.read (FileStore.ts:34-67):
//
//   - The index entry for the archive is a 6-byte sector cluster: a
//     3-byte size and a 3-byte first-sector number
//     (SectorCluster.decode, store/SectorCluster.ts:6-10).
//   - Sectors are followed through their next-sector links, validating
//     the index id, archive id, and chunk number of every sector.
//   - Archives with id > 65535 use the extended sector format: a
//     4-byte archive id, a 10-byte header, and 510 data bytes per
//     sector (Sector.decodeExtended, store/Sector.ts:37-48).
//   - The expected sector index id is indexId when the meta index
//     (idx255) is present, otherwise indexId+1 (FileStore.ts:43): in
//     caches without a meta index, sector headers are 1-based.
//
// Errors name the index and archive, matching the TypeScript messages.
func (s *Store) Read(indexID, archiveID int) ([]byte, error) {
	index, ok := s.indexes[indexID]
	if !ok {
		return nil, fmt.Errorf("Index %d not found", indexID)
	}
	clusterOffset := int64(archiveID) * SectorClusterSize
	if clusterOffset < 0 || clusterOffset+SectorClusterSize > int64(len(index)) {
		return nil, fmt.Errorf("Invalid ptr: %d, fileSize: %d, indexId: %d, archiveId: %d",
			clusterOffset, len(index), indexID, archiveID)
	}
	cluster, err := decodeSectorCluster(index[clusterOffset : clusterOffset+SectorClusterSize])
	if err != nil {
		return nil, err
	}
	data := make([]byte, cluster.size)
	sectorIndexID := indexID
	if !s.hasMetaIndex {
		sectorIndexID = indexID + 1
	}
	extended := archiveID > ExtendedArchiveIDThreshold
	headerSize := SectorHeaderSize
	sectorDataSize := SectorDataSize
	if extended {
		headerSize = SectorExtendedHeaderSize
		sectorDataSize = SectorExtendedDataSize
	}
	chunk := 0
	remaining := cluster.size
	sectorOffset := int64(cluster.sector) * SectorSize
	for remaining > 0 {
		dataSize := sectorDataSize
		if remaining < dataSize {
			dataSize = remaining
		}
		sectorBytes, err := s.readData(sectorOffset, int64(headerSize+dataSize))
		if err != nil {
			return nil, err
		}
		sector, err := decodeSector(sectorBytes, extended, dataSize)
		if err != nil {
			return nil, err
		}
		if sector.indexID != sectorIndexID || sector.archiveID != archiveID || sector.chunk != chunk {
			return nil, fmt.Errorf("Invalid cache sector for index %d, archive %d", indexID, archiveID)
		}
		copy(data[cluster.size-remaining:], sector.data)
		remaining -= dataSize
		chunk++
		sectorOffset = int64(sector.nextSector) * SectorSize
	}
	return data, nil
}

// readData reads length bytes at an absolute offset of the data file,
// mirroring FileStore.readData (FileStore.ts:73-82).
func (s *Store) readData(offset, length int64) ([]byte, error) {
	if offset < 0 || offset+length > s.dataFileSize {
		return nil, fmt.Errorf("Invalid cache sector offset %d", offset)
	}
	bytes := make([]byte, length)
	n, err := s.file.ReadAt(bytes, offset)
	if err != nil || int64(n) != length {
		if err == nil {
			err = fmt.Errorf("short read: got %d of %d bytes", n, length)
		}
		return nil, fmt.Errorf("Short cache read at %d: %w", offset, err)
	}
	return bytes, nil
}

// sectorCluster is one 6-byte index entry: the archive's total size
// and its first sector number (store/SectorCluster.ts).
type sectorCluster struct {
	size   int
	sector int
}

func decodeSectorCluster(entry []byte) (sectorCluster, error) {
	buf := NewByteBuffer(entry)
	size, err := buf.ReadMedium()
	if err != nil {
		return sectorCluster{}, fmt.Errorf("decoding sector cluster: %w", err)
	}
	sector, err := buf.ReadMedium()
	if err != nil {
		return sectorCluster{}, fmt.Errorf("decoding sector cluster: %w", err)
	}
	return sectorCluster{size: size, sector: sector}, nil
}

// sector is one decoded sector header plus its payload slice
// (store/Sector.ts).
type sector struct {
	indexID    int
	archiveID  int
	chunk      int
	nextSector int
	data       []byte
}

// decodeSector decodes a sector's header and dataSize payload bytes.
// Standard layout (Sector.decode, Sector.ts:28-35): archive id u16,
// chunk u16, next sector medium, index id u8, then data. Extended
// layout (Sector.decodeExtended, Sector.ts:37-48): archive id int32,
// then the same chunk / next sector / index id fields.
func decodeSector(sectorBytes []byte, extended bool, dataSize int) (sector, error) {
	var s sector
	buf := NewByteBuffer(sectorBytes)
	if extended {
		archiveID, err := buf.ReadInt()
		if err != nil {
			return s, fmt.Errorf("decoding cache sector: %w", err)
		}
		s.archiveID = int(archiveID)
	} else {
		archiveID, err := buf.ReadUnsignedShort()
		if err != nil {
			return s, fmt.Errorf("decoding cache sector: %w", err)
		}
		s.archiveID = archiveID
	}
	chunk, err := buf.ReadUnsignedShort()
	if err != nil {
		return s, fmt.Errorf("decoding cache sector: %w", err)
	}
	s.chunk = chunk
	nextSector, err := buf.ReadMedium()
	if err != nil {
		return s, fmt.Errorf("decoding cache sector: %w", err)
	}
	s.nextSector = nextSector
	indexID, err := buf.ReadUnsignedByte()
	if err != nil {
		return s, fmt.Errorf("decoding cache sector: %w", err)
	}
	s.indexID = indexID
	data, err := buf.ReadBytes(dataSize)
	if err != nil {
		return s, fmt.Errorf("decoding cache sector: %w", err)
	}
	s.data = data
	return s, nil
}
