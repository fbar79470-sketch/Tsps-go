package session

import (
	"strings"
	"unicode/utf16"

	"tsps-go/internal/persist"
)

// Name functions ported exactly from util/Misc.ts. Login name handling
// must match TS exactly: the same typed name must resolve to the same
// account, ban, and pending-name key on both implementations.

// FormatDisplayName renders a typed login name as shown in game: capitals
// kept, underscores as spaces. Port of Misc.formatDisplayName
// (Misc.ts:806-808): name.trim().replace(/_/g, " ").
func FormatDisplayName(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "_", " ")
}

// FormatNameForProtocol renders a name in protocol form: lowercased, with
// spaces as underscores. Port of Misc.formatNameForProtocol
// (Misc.ts:801-803): name.toLowerCase().replace(/ /g, "_"). Only the
// literal space character is replaced; other whitespace is left alone.
func FormatNameForProtocol(name string) string {
	return strings.ReplaceAll(strings.ToLower(name), " ", "_")
}

// IsValidName reports whether name is a usable login name. Port of
// Misc.isValidName (Misc.ts:746-748): the protocol form must match
// /^[a-z0-9_]+$/i. The empty string is invalid; the check runs on the
// protocol form, so "John Doe" is valid (it becomes "john_doe").
func IsValidName(name string) bool {
	s := FormatNameForProtocol(name)
	if s == "" {
		return false
	}
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

// NameToLong encodes a name as the base-37 long used for long usernames.
// It follows Misc.stringToLongBigInt (Misc.ts:769-786) with exact int64
// arithmetic: at most the first 12 characters are consumed, each step
// multiplies by 37 and adds 1-26 for a letter (case-insensitive) or 27-36
// for a digit; any other character multiplies by 37 and adds nothing.
// Trailing factors of 37 are then divided out, so trailing underscores
// and other non-alphanumeric characters do not change the value ("a_" and
// "a" encode identically). The result always fits in an int64: 12 digits
// of base 37 top out below 37^12 < 2^63.
//
// This equals Misc.stringToLong (Misc.ts:750-767) for every value exactly
// representable in a float64, and is the exact value where the float64
// version loses precision; TS stores long usernames with the BigInt
// version (NetworkBuilder enterWorld, setLongUsername).
//
// The 12-character cap counts UTF-16 code units, as JS string indexing
// does: a character outside the Basic Multilingual Plane consumes two
// units (and adds nothing, as its surrogate units match no class).
func NameToLong(name string) int64 {
	var l int64
	units := 0
	for _, r := range name {
		// UTF-16 units consumed by this rune: 2 for astral characters.
		n := 1
		if r > 0xFFFF {
			n = 2
		}
		for i := 0; i < n; i++ {
			if units >= 12 {
				break
			}
			units++
			l *= 37
			// Only a single-unit rune can be an ASCII letter or digit;
			// surrogate halves add nothing.
			if n == 1 {
				switch {
				case r >= 'A' && r <= 'Z':
					l += 1 + int64(r) - 65
				case r >= 'a' && r <= 'z':
					l += 1 + int64(r) - 97
				case r >= '0' && r <= '9':
					l += 27 + int64(r) - 48
				}
			}
		}
		if units >= 12 {
			break
		}
	}
	for l != 0 && l%37 == 0 {
		l /= 37
	}
	return l
}

// utf16Len returns the length of s in UTF-16 code units, matching JS
// string.length. TS login() measures the protocol name (1-12) and the
// password (1-64) this way (NetworkBuilder.ts:1408-1413).
func utf16Len(s string) int {
	return len(utf16.Encode([]rune(s)))
}

// pendingKey is the pendingNames key for a name: the protocol form of its
// login key. Port of pendingKey (NetworkBuilder.ts:325-327):
// Misc.formatNameForProtocol(DisplayNames.key(name)). persist.Key is the
// DisplayNames.key port.
func pendingKey(name string) string {
	return FormatNameForProtocol(persist.Key(name))
}
