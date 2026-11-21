package cache

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// xteaEncryptBlock is the textbook XTEA encryption loop, implemented
// in this test as the inverse of Xtea.decrypt (Xtea.ts): sum starts
// at 0, v0 is mixed first, then sum advances, then v1. It exists only
// to produce ciphertexts for decrypt tests; it is not part of the
// package API (TSPS never encrypts).
func xteaEncryptBlock(v0, v1 uint32, k [4]uint32) (uint32, uint32) {
	const delta uint32 = 0x9e3779b9
	var sum uint32
	for i := 0; i < 32; i++ {
		v0 += (((v1 << 4) ^ (v1 >> 5)) + v1) ^ (sum + k[sum&3])
		sum += delta
		v1 += (((v0 << 4) ^ (v0 >> 5)) + v0) ^ (sum + k[(sum>>11)&3])
	}
	return v0, v1
}

func xteaEncrypt(data []byte, key [4]int32) []byte {
	out := append([]byte(nil), data...)
	k := [4]uint32{uint32(key[0]), uint32(key[1]), uint32(key[2]), uint32(key[3])}
	for i := 0; i+8 <= len(out); i += 8 {
		v0 := binary.BigEndian.Uint32(out[i:])
		v1 := binary.BigEndian.Uint32(out[i+4:])
		c0, c1 := xteaEncryptBlock(v0, v1, k)
		binary.BigEndian.PutUint32(out[i:], c0)
		binary.BigEndian.PutUint32(out[i+4:], c1)
	}
	return out
}

func TestXTEAValidKey(t *testing.T) {
	if XTEAValidKey([4]int32{0, 0, 0, 0}) {
		t.Error("all-zero key should be invalid")
	}
	if !XTEAValidKey([4]int32{0, 0, 0, 1}) {
		t.Error("key with one nonzero word should be valid")
	}
	if !XTEAValidKey([4]int32{-1, -2, -3, -4}) {
		t.Error("negative-word key should be valid")
	}
}

// TestXTEADecryptKnownVector checks one block against a fixed
// ciphertext. The vector was produced by the textbook encrypt loop
// above (plaintext "testdata", key {0x01234567, 0x89abcdef,
// 0xfedcba98, 0x76543210}) and cross-checked against an independent
// Python implementation of the same loop, which produced the
// identical ciphertext af2ea13bccfba4fe.
func TestXTEADecryptKnownVector(t *testing.T) {
	// Key words in hex: 0x01234567, 0x89abcdef, 0xfedcba98, 0x76543210
	// (the middle two written as negative int32 literals, since Go
	// rejects out-of-range int32 constants).
	key := [4]int32{0x01234567, -1985229329, -19088744, 0x76543210}
	ciphertext := []byte{0xaf, 0x2e, 0xa1, 0x3b, 0xcc, 0xfb, 0xa4, 0xfe}
	XTEADecryptRange(ciphertext, 0, len(ciphertext), key)
	if got := string(ciphertext); got != "testdata" {
		t.Errorf("decrypt = %q, want %q", got, "testdata")
	}
}

func TestXTEADecryptRoundTrip(t *testing.T) {
	keys := [][4]int32{
		{1, 2, 3, 4},
		{-1, 0, 123456789, -987654321},
		{0x7fffffff, -2147483648, 42, -42},
	}
	plaintexts := [][]byte{
		[]byte("12345678"),
		[]byte("sixteen-bytes!!"),
		bytes.Repeat([]byte("abcdefgh"), 10),
	}
	for _, key := range keys {
		for _, plain := range plaintexts {
			cipher := xteaEncrypt(plain, key)
			if bytes.Equal(cipher, plain) {
				t.Fatalf("encrypt did not change plaintext for key %v", key)
			}
			decrypted := append([]byte(nil), cipher...)
			XTEADecryptRange(decrypted, 0, len(decrypted), key)
			if !bytes.Equal(decrypted, plain) {
				t.Errorf("round-trip failed for key %v: got %q want %q", key, decrypted, plain)
			}
		}
	}
}

func TestXTEADecryptRangeSemantics(t *testing.T) {
	key := [4]int32{9, 8, 7, 6}
	// Only whole 8-byte blocks are decrypted; a trailing partial
	// block is untouched.
	plain := []byte("12345678XYZ")
	cipher := xteaEncrypt(plain, key)
	// xteaEncrypt also leaves the tail alone; corrupt the tail of the
	// ciphertext copy to prove decrypt does not touch it.
	cipher[8], cipher[9], cipher[10] = 'A', 'B', 'C'
	XTEADecryptRange(cipher, 0, len(cipher), key)
	if got := string(cipher[:8]); got != "12345678" {
		t.Errorf("first block = %q", got)
	}
	if got := string(cipher[8:]); got != "ABC" {
		t.Errorf("partial tail = %q, want untouched ABC", got)
	}
	// Subranges decrypt only the blocks inside them.
	data := xteaEncrypt([]byte("aaaaaaaaaaaaaaaa"), key)
	XTEADecryptRange(data, 8, 16, key)
	if got := string(data[8:]); got != "aaaaaaaa" {
		t.Errorf("second block = %q", got)
	}
	if got := string(data[:8]); got == "aaaaaaaa" {
		t.Error("first block was decrypted but should be untouched")
	}
	// Empty and inverted ranges are no-ops, not panics.
	data = []byte("12345678")
	XTEADecryptRange(data, 4, 4, key)
	XTEADecryptRange(data, 6, 2, key)
	if got := string(data); got != "12345678" {
		t.Errorf("empty/inverted range changed data: %q", got)
	}
	// An over-wide range is clamped to the slice: encrypt, then
	// decrypt with start/end outside the slice, and the block
	// round-trips.
	data = xteaEncrypt([]byte("12345678"), key)
	XTEADecryptRange(data, -100, 100, key)
	if got := string(data); got != "12345678" {
		t.Errorf("clamped decrypt = %q", got)
	}
	// A range fully outside the slice decrypts nothing.
	data = []byte("12345678")
	XTEADecryptRange(data, 100, 200, key)
	if got := string(data); got != "12345678" {
		t.Errorf("out-of-range decrypt changed data: %q", got)
	}
}
