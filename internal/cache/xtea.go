package cache

import "encoding/binary"

// XTEA constants from crypto/Xtea.ts.
const (
	// XTEAGoldenRatio is the XTEA delta, 0x9E3779B9.
	XTEAGoldenRatio uint32 = 0x9e3779b9
	// XTEARounds is the number of XTEA rounds.
	XTEARounds = 32
)

// xteaInitialSum is INITIAL_SUM in Xtea.ts: imul(GOLDEN_RATIO, ROUNDS),
// i.e. 0x9E3779B9 * 32 mod 2^32 = 0xC6EF3720.
const xteaInitialSum uint32 = 0xC6EF3720

// XTEAValidKey reports whether key is a usable XTEA key: four words,
// not all zero. It mirrors Xtea.isValidKey (crypto/Xtea.ts:10-16); the
// length check is structural here because the key is a [4]int32.
func XTEAValidKey(key [4]int32) bool {
	return key[0] != 0 || key[1] != 0 || key[2] != 0 || key[3] != 0
}

// XTEADecryptRange decrypts whole 8-byte blocks of data in place, from
// start (inclusive) to end (exclusive), with the 128-bit key. It
// mirrors Xtea.decrypt (crypto/Xtea.ts:18-37): 32 rounds, big-endian
// words, all arithmetic mod 2^32.
//
// Only complete blocks are decrypted: floor((end-start)/8) blocks, so
// a trailing partial block is left untouched. A range that runs past
// the slice is clamped to the slice instead of panicking (the
// TypeScript version reads undefined values out of range, which has
// no meaningful Go equivalent); an empty or inverted range decrypts
// nothing.
func XTEADecryptRange(data []byte, start, end int, key [4]int32) {
	if start < 0 {
		start = 0
	}
	if end > len(data) {
		end = len(data)
	}
	n := (end - start) / 8
	if n <= 0 {
		return
	}
	k := [4]uint32{uint32(key[0]), uint32(key[1]), uint32(key[2]), uint32(key[3])}
	for i := 0; i < n; i++ {
		offset := start + i*8
		sum := xteaInitialSum
		v0 := binary.BigEndian.Uint32(data[offset:])
		v1 := binary.BigEndian.Uint32(data[offset+4:])
		for j := 0; j < XTEARounds; j++ {
			v1 -= (((v0 << 4) ^ (v0 >> 5)) + v0) ^ (sum + k[(sum>>11)&3])
			sum -= XTEAGoldenRatio
			v0 -= (((v1 << 4) ^ (v1 >> 5)) + v1) ^ (sum + k[sum&3])
		}
		binary.BigEndian.PutUint32(data[offset:], v0)
		binary.BigEndian.PutUint32(data[offset+4:], v1)
	}
}
