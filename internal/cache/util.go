package cache

import (
	"hash/crc32"
	"strings"
	"unicode/utf16"
)

// CRC32 returns the IEEE CRC-32 checksum of data, as crc32 in
// util/Crc32.ts does: polynomial 0xEDB88320, initial value 0xFFFFFFFF,
// final XOR 0xFFFFFFFF. The Go standard library's IEEE table computes
// exactly this checksum.
func CRC32(data []byte) uint32 {
	return crc32.ChecksumIEEE(data)
}

// HashDjb2 is StringUtil.hashDjb2 (util/StringUtil.ts): Dan Bernstein's
// djb2 hash, modified to start from 0 instead of 5381. It iterates
// UTF-16 code units, as charCodeAt does, and all arithmetic wraps at
// 32 bits. Used for archive and file name lookups in reference tables
// and archives.
func HashDjb2(s string) int32 {
	var hash int32
	for _, unit := range utf16.Encode([]rune(s)) {
		hash = (hash << 5) - hash + int32(unit)
	}
	return hash
}

// HashOld is StringUtil.hashOld (util/StringUtil.ts): the legacy name
// hash over the uppercased name, iterating UTF-16 code units with
// 32-bit wrapping arithmetic. Used by legacy (non-dat2) cache layouts.
func HashOld(name string) int32 {
	var hash int32
	for _, unit := range utf16.Encode([]rune(strings.ToUpper(name))) {
		hash = hash*61 + int32(unit) - 32
	}
	return hash
}
