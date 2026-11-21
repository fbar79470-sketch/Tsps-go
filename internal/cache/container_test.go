package cache

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"strings"
	"testing"
)

func buildContainer(t *testing.T, compression Compression, payload, compressed []byte, actualSize uint32) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	buf.WriteByte(byte(compression))
	size := len(payload)
	if compression != CompressionNone {
		size = len(compressed)
	}
	var tmp [4]byte
	binary.BigEndian.PutUint32(tmp[:], uint32(size))
	buf.Write(tmp[:])
	if compression != CompressionNone {
		binary.BigEndian.PutUint32(tmp[:], actualSize)
		buf.Write(tmp[:])
		buf.Write(compressed)
	} else {
		buf.Write(payload)
	}
	return buf.Bytes()
}

func gzipCompress(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeContainerNone(t *testing.T) {
	payload := []byte("hello, cache world")
	decoded, err := DecodeContainer(buildContainer(t, CompressionNone, payload, nil, 0), nil)
	if err != nil {
		t.Fatalf("DecodeContainer: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Errorf("decoded = %q, want %q", decoded, payload)
	}
	// A zero key must not trigger decryption.
	zero := [4]int32{}
	decoded, err = DecodeContainer(buildContainer(t, CompressionNone, payload, nil, 0), &zero)
	if err != nil || !bytes.Equal(decoded, payload) {
		t.Errorf("zero-key decode = %q, %v", decoded, err)
	}
}

func TestDecodeContainerGzip(t *testing.T) {
	payload := []byte(strings.Repeat("gzip payload ", 50))
	compressed := gzipCompress(t, payload)
	decoded, err := DecodeContainer(buildContainer(t, CompressionGzip, nil, compressed, uint32(len(payload))), nil)
	if err != nil {
		t.Fatalf("DecodeContainer: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Error("gzip container did not round-trip")
	}
}

// bzip2PayloadCompressed is the payload below compressed with
// `bzip2 -c -9`, with the 4-byte "BZh9" header stripped — the form
// cache containers store (Bzip2.decompress restores a "BZh1" header).
var bzip2Payload = []byte("hello bzip2 world, this is a cache container payload.")

var bzip2PayloadCompressed = []byte{
	0x31, 0x41, 0x59, 0x26, 0x53, 0x59, 0x69, 0x42, 0xf9, 0x59, 0x00, 0x00, 0x0d, 0x19, 0x80, 0x40,
	0x05, 0x10, 0x00, 0x3e, 0x65, 0xdc, 0xb0, 0x20, 0x00, 0x54, 0x50, 0x34, 0xd0, 0xc8, 0xc9, 0x88,
	0x35, 0x43, 0xca, 0x7a, 0x69, 0xa2, 0x7a, 0x7a, 0x91, 0xea, 0x19, 0x68, 0x42, 0xcf, 0x80, 0x6f,
	0x98, 0xda, 0xa8, 0xc7, 0x27, 0xad, 0x0d, 0xbe, 0x9f, 0xba, 0x7b, 0x49, 0x8a, 0xfa, 0x50, 0x66,
	0x96, 0xf6, 0x64, 0x12, 0xb2, 0x38, 0xf8, 0xbb, 0x92, 0x29, 0xc2, 0x84, 0x83, 0x4a, 0x17, 0xca,
	0xc8,
}

func TestDecodeContainerBzip2(t *testing.T) {
	decoded, err := DecodeContainer(buildContainer(t, CompressionBzip2, nil, bzip2PayloadCompressed, uint32(len(bzip2Payload))), nil)
	if err != nil {
		t.Fatalf("DecodeContainer: %v", err)
	}
	if !bytes.Equal(decoded, bzip2Payload) {
		t.Errorf("decoded = %q, want %q", decoded, bzip2Payload)
	}
}

func TestDecodeContainerXTEA(t *testing.T) {
	key := [4]int32{11, 22, 33, 44}
	payload := []byte("xtea-protected container payload, 40 bytes!!")
	compressed := gzipCompress(t, payload)
	container := buildContainer(t, CompressionGzip, nil, compressed, uint32(len(payload)))
	// Encrypt the same range Container.decode decrypts: bytes
	// [5, 5+4+size), in whole 8-byte blocks. The test encryptor
	// leaves a trailing partial block untouched, exactly as the
	// decoder's decrypt does, so the fixture round-trips for any
	// compressed length.
	size := len(compressed)
	encrypted := append([]byte(nil), container...)
	region := xteaEncrypt(encrypted[5:5+4+size], key)
	copy(encrypted[5:], region)
	decoded, err := DecodeContainer(encrypted, &key)
	if err != nil {
		t.Fatalf("DecodeContainer with key: %v", err)
	}
	if !bytes.Equal(decoded, payload) {
		t.Errorf("decoded = %q, want %q", decoded, payload)
	}
	// Decoding the same bytes without the key must fail (the
	// decompressed-size field is garbage, so either decompression
	// or the size check rejects it).
	if _, err := DecodeContainer(encrypted, nil); err == nil {
		t.Error("encrypted container decoded without a key, want error")
	}
	// The input slice must not be mutated by decoding.
	before := append([]byte(nil), encrypted...)
	if _, err := DecodeContainer(encrypted, &key); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(encrypted, before) {
		t.Error("DecodeContainer mutated its input")
	}
}

func TestDecodeContainerErrors(t *testing.T) {
	if _, err := DecodeContainer(nil, nil); err == nil || !strings.Contains(err.Error(), "Empty container") {
		t.Errorf("empty: err = %v", err)
	}
	if _, err := DecodeContainer([]byte{0x00, 0x00}, nil); err == nil {
		t.Error("truncated header decoded, want error")
	}
	// Unsupported compression byte.
	if _, err := DecodeContainer([]byte{0x07, 0, 0, 0, 0}, nil); err == nil || !strings.Contains(err.Error(), "Unsupported compression: 7") {
		t.Errorf("bad compression: err = %v", err)
	}
	// Negative size.
	if _, err := DecodeContainer([]byte{0x00, 0xff, 0xff, 0xff, 0xff}, nil); err == nil {
		t.Error("negative size decoded, want error")
	}
	// Truncated None payload.
	if _, err := DecodeContainer([]byte{0x00, 0, 0, 0, 10, 'a', 'b'}, nil); err == nil {
		t.Error("truncated payload decoded, want error")
	}
	// Size mismatch: gzip stream decompresses to 5 bytes, header says 6.
	payload := []byte("12345")
	compressed := gzipCompress(t, payload)
	if _, err := DecodeContainer(buildContainer(t, CompressionGzip, nil, compressed, 6), nil); err == nil || !strings.Contains(err.Error(), "Size mismatch") {
		t.Errorf("size mismatch: err = %v", err)
	}
	// Corrupt gzip stream.
	if _, err := DecodeContainer(buildContainer(t, CompressionGzip, nil, []byte("not gzip at all...."), 4), nil); err == nil {
		t.Error("corrupt gzip decoded, want error")
	}
	// Corrupt bzip2 stream.
	if _, err := DecodeContainer(buildContainer(t, CompressionBzip2, nil, bytes.Repeat([]byte{0xaa}, 20), 4), nil); err == nil {
		t.Error("corrupt bzip2 decoded, want error")
	}
}
