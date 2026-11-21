package game

import (
	"strings"
	"testing"
)

// Name-hash vectors computed from the TS algorithm
// (Misc.stringToLongBigInt / longToString, Misc.ts:769-786, 814-837)
// with an independent implementation, mirroring the session package's
// names_test.go vectors where they overlap (notably the astral-plane
// case, which pins StringToLong == session.NameToLong).

func TestStringToLong(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"a", 1},
		{"b", 2},
		{"z", 26},
		{"0", 27},
		{"9", 36},
		{"aa", 38},
		{"ab", 39},
		// Trailing non-alphanumerics strip out: trailing factors
		// of 37 are divided away (Misc.ts:782-784).
		{"a_", 1},
		{"a__", 1},
		// Case-insensitive.
		{"A", 1},
		{"ALICE", 2494434},
		{"alice", 2494434},
		// A space multiplies by 37 and adds nothing, like "_".
		{"a b", 1371},
		{"Bob Smith", 8454104798272},
		{"John_Doe", 988385663411},
		{"test123", 51697909519},
		{"1abc", 1419730},
		// The 12-unit cap: the 13th "a" is ignored, so 13 a's
		// hash exactly like 12.
		{"aaaaaaaaaaaaa", 182859777940000980},
		// "john doe smith" is 14 units; only "john doe smi" counts.
		{"john doe smith", 1852393863324049672},
		// Maximum 12-digit values: all z's, all 9's (37^12 - 1).
		{"zzzzzzzzzzzz", 4754354226440025480},
		{"999999999999", 6582952005840035280},
		// Astral-plane character: two UTF-16 units, each
		// multiplying by 37 and adding nothing (session
		// names_test.go vector, pinning equivalence with
		// session.NameToLong).
		{"a\U0001F600b", 50655},
	}
	for _, tc := range cases {
		if got := StringToLong(tc.in); got != tc.want {
			t.Errorf("StringToLong(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
	// Every hash fits in an int64 with room: the largest possible
	// value is 37^12 - 1 < 2^63 - 1.
	if got := StringToLong("999999999999"); got > 1<<63-1 {
		t.Errorf("StringToLong max = %d, overflows int64", got)
	}
}

func TestLongToString(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, ""},
		{-1, ""},
		{-6582952005840035281, ""},
		{1, "a"},
		{26, "z"},
		{27, "0"},
		{36, "9"},
		{38, "aa"},
		{2494434, "alice"},
		{3295, "bob"},
		{8454104798272, "bob_smith"},
		{988385663411, "john_doe"},
		// 37 has a trailing factor of 37: its least-significant
		// digit is 0 ("_"), then 1 ("a").
		{37, "a_"},
		// 12-char cap: 37^12 needs 13 digits; only the 12
		// least-significant (all "_") survive.
		{6582952005840035281, "____________"},
		{6582952005840035281 + 1, "___________a"},
		{6582952005840035280, "999999999999"},
		{50655, "a__b"},
	}
	for _, tc := range cases {
		if got := LongToString(tc.in); got != tc.want {
			t.Errorf("LongToString(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestLongToStringRoundTrip(t *testing.T) {
	// Every hash StringToLong produces decodes to its canonical
	// form: lowercase, first 12 units, non-alphanumerics as "_".
	cases := map[string]string{
		"alice":          "alice",
		"ALICE":          "alice",
		"Bob Smith":      "bob_smith",
		"John_Doe":       "john_doe",
		"a b":            "a_b",
		"a_":             "a",
		"test123":        "test123",
		"john doe smith": "john_doe_smi",
		"a\U0001F600b":   "a__b",
	}
	for in, want := range cases {
		if got := LongToString(StringToLong(in)); got != want {
			t.Errorf("LongToString(StringToLong(%q)) = %q, want %q", in, got, want)
		}
	}
	// Re-encoding a decoded hash is stable: decoding never
	// produces a hash with trailing factors of 37.
	for _, hash := range []int64{1, 37, 38, 2494434, 50655, 6582952005840035280} {
		decoded := LongToString(hash)
		reencoded := StringToLong(decoded)
		// Strip trailing 37s from the original: the decode/re-encode
		// round trip equals the stripped original.
		stripped := hash
		for stripped != 0 && stripped%37 == 0 {
			stripped /= 37
		}
		if reencoded != stripped {
			t.Errorf("StringToLong(LongToString(%d)) = %d, want stripped %d (decoded %q)", hash, reencoded, stripped, decoded)
		}
	}
}

func TestFormatName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"alice", "Alice"},
		{"john_doe", "John Doe"},
		{"john doe", "John Doe"},
		// Only the FIRST space becomes "_": the second space
		// stays, and "smith" is not capitalized (JS replace
		// with a string pattern replaces one occurrence).
		{"john doe smith", "John Doe smith"},
		{"a", "A"},
		{"1abc", "1abc"},
		{"a_b_c", "A B C"},
	}
	for _, tc := range cases {
		if got := FormatName(tc.in); got != tc.want {
			t.Errorf("FormatName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFixName(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"a", "A"},
		{"A", "A"},
		{"alice", "Alice"},
		{"john_doe", "John Doe"},
		{"_a", " A"},
		{"a__b", "A  B"},
		{"a_b_c", "A B C"},
		{"ABC_DEF", "ABC DEF"},
		{"1abc", "1abc"},
		{"a1_b2", "A1 B2"},
		// Spaces are not underscores: no capitalization follows.
		{"john doe", "John doe"},
		// Trailing underscore becomes a trailing space.
		{"alice_", "Alice "},
	}
	for _, tc := range cases {
		if got := FixName(tc.in); got != tc.want {
			t.Errorf("FixName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatNameOfHash(t *testing.T) {
	// The relations call shape: FormatName(LongToString(hash)) is
	// the display form used in refusal/notice strings and world
	// lookups (PlayerRelations.ts:267, 299, 320, 347).
	if got := FormatName(LongToString(StringToLong("alice"))); got != "Alice" {
		t.Errorf("FormatName(LongToString(alice hash)) = %q, want %q", got, "Alice")
	}
	if got := FormatName(LongToString(StringToLong("bob_smith"))); got != "Bob Smith" {
		t.Errorf("FormatName(LongToString(bob_smith hash)) = %q, want %q", got, "Bob Smith")
	}
	if got := FormatName(LongToString(0)); got != "" {
		t.Errorf("FormatName(LongToString(0)) = %q, want empty", got)
	}
	// The valid-characters table has exactly the TS entries.
	if len(validPlayerCharacters) != 42 {
		t.Fatalf("validPlayerCharacters length = %d, want 42", len(validPlayerCharacters))
	}
	if !strings.HasPrefix(validPlayerCharacters, "_abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Errorf("validPlayerCharacters prefix = %q, want the TS table head", validPlayerCharacters)
	}
}
