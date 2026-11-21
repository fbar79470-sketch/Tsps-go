package cache

import (
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
)

// DecodeContainer decodes one cache container: the bytes Store.Read
// returns for an archive. It mirrors Container.decode
// (cache/Container.ts:9-49).
//
// Layout: 1 compression byte, then a 4-byte big-endian compressed
// size. When key is non-nil and valid (see XTEAValidKey), the bytes
// from offset 5 through 5+4+size are XTEA-decrypted first — that
// range covers the 4-byte decompressed-size field plus the compressed
// payload. For CompressionNone the payload is the next size bytes.
// For Bzip2/Gzip a 4-byte decompressed size follows, then size bytes
// of compressed data; the decompressed length must equal the declared
// size or an error is returned.
//
// The input slice is not modified: decryption runs on a copy. (The
// TypeScript version decrypts its ByteBuffer in place; callers here
// keep their Store.Read result intact.)
func DecodeContainer(data []byte, key *[4]int32) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("Empty container")
	}
	// Work on a copy so XTEA decryption never mutates the caller's bytes.
	buf := NewByteBuffer(append([]byte(nil), data...))
	compressionByte, err := buf.ReadUnsignedByte()
	if err != nil {
		return nil, fmt.Errorf("decoding container: %w", err)
	}
	compression := Compression(compressionByte)
	size32, err := buf.ReadInt()
	if err != nil {
		return nil, fmt.Errorf("decoding container: %w", err)
	}
	if size32 < 0 {
		// TypeScript would feed a negative size to readBytes and
		// get a meaningless subarray; reject it explicitly.
		return nil, fmt.Errorf("decoding container: negative size %d", size32)
	}
	size := int(size32)
	if key != nil && XTEAValidKey(*key) {
		XTEADecryptRange(buf.Data, buf.Offset, buf.Offset+4+size, *key)
	}
	switch compression {
	case CompressionNone:
		payload, err := buf.ReadBytes(size)
		if err != nil {
			return nil, fmt.Errorf("decoding container: truncated uncompressed payload: %w", err)
		}
		return append([]byte(nil), payload...), nil
	case CompressionBzip2, CompressionGzip:
		actualSize32, err := buf.ReadInt()
		if err != nil {
			return nil, fmt.Errorf("decoding container: %w", err)
		}
		actualSize := uint32(actualSize32)
		compressed, err := buf.ReadBytes(size)
		if err != nil {
			return nil, fmt.Errorf("decoding container: truncated compressed payload: %w", err)
		}
		decompressed, err := decompress(compression, compressed, actualSize)
		if err != nil {
			return nil, err
		}
		if uint32(len(decompressed)) != actualSize {
			return nil, fmt.Errorf("Container: Size mismatch. Compressed: %d, Decompressed: %d, Type: %s",
				actualSize, len(decompressed), compression)
		}
		return decompressed, nil
	default:
		return nil, fmt.Errorf("Container: Unsupported compression: %d", compressionByte)
	}
}

// decompress inflates compressed data, reading at most actualSize+1
// bytes so a corrupt stream that expands without bound is caught by
// the caller's length check instead of exhausting memory. (The
// TypeScript decompressors read the whole stream; the length check in
// Container.decode is what rejects the result either way.)
func decompress(compression Compression, compressed []byte, actualSize uint32) ([]byte, error) {
	limit := int64(actualSize) + 1
	var reader io.Reader
	switch compression {
	case CompressionBzip2:
		// Bzip2.decompress (compression/Bzip2.ts:11-18) restores the
		// stripped 4-byte "BZh1" header before decompressing.
		reader = bzip2.NewReader(bytes.NewReader(append([]byte("BZh1"), compressed...)))
	case CompressionGzip:
		gz, err := gzip.NewReader(bytes.NewReader(compressed))
		if err != nil {
			return nil, fmt.Errorf("decoding container: gzip: %w", err)
		}
		defer gz.Close()
		reader = gz
	}
	decompressed, err := io.ReadAll(io.LimitReader(reader, limit))
	if err != nil {
		return nil, fmt.Errorf("decoding container: %s: %w", compression, err)
	}
	return decompressed, nil
}
