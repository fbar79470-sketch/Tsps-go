package cache

import "fmt"

// Archive is one decoded archive: a bundle of files sharing one
// container. It mirrors Archive (cache/Archive.ts) for the dat2
// format only; Archive.decodeOld (the legacy DAT format) is not
// ported.
type Archive struct {
	ID             int
	LastFileID     int
	FileCount      int
	FileIDs        []int
	FileNameHashes []int32 // nil when the reference table is unnamed

	files            map[int][]byte
	fileNameHashToID map[int32]int
}

// DecodeArchive splits container payload bytes into the archive's
// files, using the file layout from the archive's reference-table
// entry. It mirrors Archive.decode (cache/Archive.ts:103-160).
//
// A single-file archive's file is the whole payload. A multi-file
// archive ends with a trailer: a chunk count byte, preceded by
// chunkCount*fileCount big-endian int32 chunk-size deltas (cumulative
// per chunk); the file data itself is stored chunk by chunk, one
// slice per file per chunk, at the start of the payload.
//
// The single-file case shares the backing array of data (as the
// TypeScript version shares its ByteBuffer); callers must not mutate
// data after decoding. Multi-file data is copied into fresh buffers.
//
// Corrupt layouts — a trailer that overruns the payload, negative
// chunk sizes, or chunk data running into the trailer — return an
// error; the TypeScript version reads past the end silently in those
// cases (ByteBuffer subarray clamping).
func DecodeArchive(id, lastFileID, fileCount int, fileIDs []int, fileNameHashes []int32, data []byte) (*Archive, error) {
	if fileCount < 0 {
		return nil, fmt.Errorf("decoding archive %d: negative file count %d", id, fileCount)
	}
	if len(fileIDs) != fileCount {
		return nil, fmt.Errorf("decoding archive %d: %d file ids for %d files", id, len(fileIDs), fileCount)
	}
	if fileNameHashes != nil && len(fileNameHashes) != fileCount {
		return nil, fmt.Errorf("decoding archive %d: %d file name hashes for %d files", id, len(fileNameHashes), fileCount)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("decoding archive %d: empty data", id)
	}
	archive := &Archive{
		ID:               id,
		LastFileID:       lastFileID,
		FileCount:        fileCount,
		FileIDs:          append([]int(nil), fileIDs...),
		FileNameHashes:   append([]int32(nil), fileNameHashes...),
		files:            make(map[int][]byte, fileCount),
		fileNameHashToID: make(map[int32]int),
	}
	for i, fileID := range fileIDs {
		if fileNameHashes != nil {
			archive.fileNameHashToID[fileNameHashes[i]] = fileID
		}
	}

	if fileCount == 1 {
		archive.files[lastFileID] = data
		return archive, nil
	}

	buf := NewByteBuffer(data)
	// The last byte is the chunk count (Archive.ts:115-116).
	buf.Offset = buf.Len() - 1
	chunks, err := buf.ReadUnsignedByte()
	if err != nil {
		return nil, fmt.Errorf("decoding archive %d: %w", id, err)
	}
	trailerStart := buf.Len() - 1 - chunks*(fileCount*4)
	if trailerStart < 0 {
		return nil, fmt.Errorf("decoding archive %d: chunk trailer overruns payload", id)
	}
	buf.Offset = trailerStart
	chunkSizes := make([]int, chunks*fileCount)
	fileSizes := make([]int, fileCount)
	for chunk := 0; chunk < chunks; chunk++ {
		// Chunk sizes are delta-encoded per chunk (Archive.ts:122-129).
		// The TypeScript accumulation is float64, not 32-bit
		// wrapping, so accumulate in int here.
		lastFileSize := 0
		for fileIdx := 0; fileIdx < fileCount; fileIdx++ {
			delta, err := buf.ReadInt()
			if err != nil {
				return nil, fmt.Errorf("decoding archive %d: %w", id, err)
			}
			lastFileSize += int(delta)
			chunkSize := lastFileSize
			if chunkSize < 0 {
				return nil, fmt.Errorf("decoding archive %d: negative chunk size", id)
			}
			chunkSizes[chunk*fileCount+fileIdx] = chunkSize
			fileSizes[fileIdx] += chunkSize
		}
	}
	totalData := 0
	for _, size := range fileSizes {
		totalData += size
	}
	if totalData > trailerStart {
		return nil, fmt.Errorf("decoding archive %d: chunk data overruns trailer", id)
	}
	fileData := make([][]byte, fileCount)
	for fileIdx := range fileData {
		fileData[fileIdx] = make([]byte, 0, fileSizes[fileIdx])
	}
	buf.Offset = 0
	for chunk := 0; chunk < chunks; chunk++ {
		for fileIdx := 0; fileIdx < fileCount; fileIdx++ {
			chunkSize := chunkSizes[chunk*fileCount+fileIdx]
			bytes, err := buf.ReadBytes(chunkSize)
			if err != nil {
				return nil, fmt.Errorf("decoding archive %d: %w", id, err)
			}
			fileData[fileIdx] = append(fileData[fileIdx], bytes...)
		}
	}
	for fileIdx, fileID := range fileIDs {
		archive.files[fileID] = fileData[fileIdx]
	}
	return archive, nil
}

// GetFile returns one file's bytes, or false when the archive has no
// such file. The returned slice must not be mutated.
func (a *Archive) GetFile(id int) ([]byte, bool) {
	if a == nil {
		return nil, false
	}
	data, ok := a.files[id]
	return data, ok
}

// GetFileID returns the file id for a name, hashed with djb2
// (Archive.decode uses StringUtil.hashDjb2), or -1 when the archive
// is unnamed or has no such file.
func (a *Archive) GetFileID(name string) int {
	if a == nil {
		return -1
	}
	id, ok := a.fileNameHashToID[HashDjb2(name)]
	if !ok {
		return -1
	}
	return id
}

// GetFileNamed returns one file's bytes by name, or false when the
// archive has no such file.
func (a *Archive) GetFileNamed(name string) ([]byte, bool) {
	return a.GetFile(a.GetFileID(name))
}
