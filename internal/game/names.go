package game

import (
	"strings"
	"unicode/utf16"
)

// Player-name functions: the Misc ports the relations system needs.
//
// TS grounding (commit bfd81e3, server/src/main/typescript/elvarg/util/Misc.ts):
//   - stringToLongBigInt, Misc.ts:769-786 (StringToLong here).
//   - formatName, Misc.ts:810-812 (FormatName here).
//   - longToString, Misc.ts:814-837 (LongToString here).
//   - fixName, Misc.ts:839-857 (FixName here).
//   - VALID_PLAYER_CHARACTERS, Misc.ts:24-67 (validPlayerCharacters here).
//
// LOCAL COPIES. The game layer must not import internal/session (import
// cycle: the session package holds a *Player), so these are ported locally,
// following the formatNameForProtocol/formatDisplayName precedent in
// grounditems.go. session.NameToLong is the canonical copy of
// stringToLongBigInt and the two must stay identical: the long username
// the login path stores (session.NameToLong, set at enterWorld,
// NetworkBuilder.ts:1662) is the hash relations decode with LongToString
// and re-encode with StringToLong (IsFriendWith). The shared vectors in
// names_test.go (including the astral-plane case from session's
// names_test.go) pin the equivalence.
//
// INT64 HASHES. TS stores long usernames as bigint; Go stores them as
// int64. Every hash fits: at most 12 base-37 digits, and 37^12 =
// 6582952005840035281 < 2^63-1, so no hash overflows and no big-int
// arithmetic is needed anywhere in this file.

// validPlayerCharacters is Misc.VALID_PLAYER_CHARACTERS
// (Misc.ts:24-67), verbatim and in order: "_" at 0, "a"-"z" at 1-26,
// "0"-"9" at 27-36, then "[", "]", "/", "-" and " " at 37-41. LongToString
// indexes it with value % 37, so only indices 0-36 are ever reached; the
// last five entries are kept so the table stays the TS table, not a
// truncation of it. A string constant carries it: every entry is one
// ASCII character, so byte indexing is the table lookup.
const validPlayerCharacters = "_abcdefghijklmnopqrstuvwxyz0123456789[]/- "

// StringToLong encodes a name as the base-37 long used for long
// usernames: Misc.stringToLongBigInt (Misc.ts:769-786). At most the
// first 12 characters are consumed; each step multiplies by 37 and adds
// 1-26 for a letter (case-insensitive) or 27-36 for a digit; any other
// character multiplies by 37 and adds nothing. Trailing factors of 37
// are then divided out, so trailing underscores and other
// non-alphanumeric characters do not change the value ("a_" and "a"
// encode identically).
//
// The 12-character cap counts UTF-16 code units, as JS string indexing
// does: the name is encoded to UTF-16 first and each unit is classified
// exactly as charAt(i) would classify it — a surrogate half matches no
// letter/digit class and adds nothing, while still consuming a unit and
// a multiply. This is the same rule session.NameToLong implements; see
// the file header.
func StringToLong(name string) int64 {
	var l int64
	units := utf16.Encode([]rune(name))
	for i := 0; i < len(units) && i < 12; i++ {
		c := units[i]
		l *= 37
		switch {
		case c >= 'A' && c <= 'Z':
			l += 1 + int64(c) - 65
		case c >= 'a' && c <= 'z':
			l += 1 + int64(c) - 97
		case c >= '0' && c <= '9':
			l += 27 + int64(c) - 48
		}
	}
	for l != 0 && l%37 == 0 {
		l /= 37
	}
	return l
}

// LongToString decodes a base-37 name hash back to a name:
// Misc.longToString (Misc.ts:814-837). A value <= 0 decodes to "".
// Otherwise the base-37 digits are read least-significant first (at
// most 12, the cap in TS), mapped through VALID_PLAYER_CHARACTERS, and
// reversed. Digits that encoded as "multiply by 37, add nothing"
// (spaces, underscores, punctuation) all decode as "_": the hash
// cannot tell them apart, so LongToString(StringToLong("bob smith"))
// is "bob_smith".
//
// LongToString is not a left inverse of StringToLong for hashes with
// trailing factors of 37 (StringToLong strips those on encode, so no
// name encodes to such a hash); it is exact for every hash StringToLong
// produces. A hash larger than any 12-digit value decodes to its 12
// least-significant digits, as in TS — the loop cap, not the value,
// bounds the output.
func LongToString(value int64) string {
	if value <= 0 {
		return ""
	}
	chars := make([]byte, 0, 12)
	for value != 0 && len(chars) < 12 {
		chars = append(chars, validPlayerCharacters[value%37])
		value /= 37
	}
	for i, j := 0, len(chars)-1; i < j; i, j = i+1, j-1 {
		chars[i], chars[j] = chars[j], chars[i]
	}
	return string(chars)
}

// FormatName renders a hash-decoded name for display in a message:
// Misc.formatName (Misc.ts:810-812): fixName after replacing the FIRST
// space with an underscore. Only the first space is replaced — JS
// String.replace with a string pattern replaces one occurrence — so
// FormatName("john doe smith") is "John Doe smith", with the second
// space left alone and its word uncapitalized. Relations calls it on
// LongToString output, which contains no spaces at all (they decode as
// "_"), so the replacement is a no-op on that path and the behavior
// that matters there is FixName's.
func FormatName(name string) string {
	return FixName(strings.Replace(name, " ", "_", 1))
}

// FixName capitalizes a name: Misc.fixName (Misc.ts:839-857). Every "_"
// becomes a space and capitalizes a following lowercase letter; the
// first character is capitalized when it is a lowercase letter. Other
// characters — including capitals, digits, and spaces — are untouched.
// The empty string stays empty.
//
// TS splits the string into UTF-16 code units; this port iterates
// runes. The two agree on every input the comparisons can distinguish:
// the "_" and "a"-"z" tests match only ASCII, and a non-ASCII rune
// (or a surrogate half) fails them alike and passes through unchanged.
func FixName(name string) string {
	if len(name) == 0 {
		return name
	}
	ac := []rune(name)
	for j := range ac {
		if ac[j] == '_' {
			ac[j] = ' '
			if j+1 < len(ac) && ac[j+1] >= 'a' && ac[j+1] <= 'z' {
				ac[j+1] = ac[j+1] + 'A' - 'a'
			}
		}
	}
	if ac[0] >= 'a' && ac[0] <= 'z' {
		ac[0] = ac[0] + 'A' - 'a'
	}
	return string(ac)
}
