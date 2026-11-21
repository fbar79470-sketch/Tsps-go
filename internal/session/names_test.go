package session

import "testing"

// Expected values computed from the TS algorithm (Misc.stringToLong /
// stringToLongBigInt, Misc.ts:750-786) with an independent Python
// implementation of the same loop.

func TestFormatDisplayName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"john_doe", "john doe"},
		{"John_Doe", "John Doe"},
		{"  john_doe  ", "john doe"},
		{"john", "john"},
		{"", ""},
		{"a__b", "a  b"},
		{"a b", "a b"},
	}
	for _, tc := range cases {
		if got := FormatDisplayName(tc.in); got != tc.want {
			t.Errorf("FormatDisplayName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFormatNameForProtocol(t *testing.T) {
	cases := []struct{ in, want string }{
		{"John Doe", "john_doe"},
		{"JOHN_DOE", "john_doe"},
		{"a b c", "a_b_c"},
		{"a\tb", "a\tb"}, // only literal spaces are replaced
		{"", ""},
		{"Already_Lower", "already_lower"},
	}
	for _, tc := range cases {
		if got := FormatNameForProtocol(tc.in); got != tc.want {
			t.Errorf("FormatNameForProtocol(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsValidName(t *testing.T) {
	valid := []string{"a", "john_doe", "abc123", "John Doe", "a1_", "Z"}
	for _, name := range valid {
		if !IsValidName(name) {
			t.Errorf("IsValidName(%q) = false, want true", name)
		}
	}
	invalid := []string{"", "a-b", "a b!", "a.b", "é", "a\tb", "a/b", "a:b"}
	for _, name := range invalid {
		if IsValidName(name) {
			t.Errorf("IsValidName(%q) = true, want false", name)
		}
	}
	// TS quirk, preserved exactly: a lone space becomes "_" in protocol
	// form, which matches the regex, so isValidName(" ") is true. The
	// login path still rejects it: FormatDisplayName trims the raw name
	// first, so the protocol name is empty and fails the length check.
	if !IsValidName(" ") {
		t.Error("IsValidName(\" \") = false, want true (TS quirk)")
	}
}

func TestNameToLong(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"", 0},
		{"a", 1},
		{"z", 26},
		{"0", 27},
		{"9", 36},
		{"aa", 38},
		{"ab", 39},
		{"a1", 65},
		{"12", 1065},
		{"test", 1020628},
		{"Test", 1020628}, // case-insensitive
		{"TEST", 1020628},
		{"a_", 1},  // trailing non-alphanumeric collapses
		{"_a", 1},  // leading underscore adds nothing
		{"a!", 1},  // other characters multiply but add nothing
		{"a__", 1}, // then trailing 37s are divided out
		{"john_doe", 988385663411},
		{"zzzzzzzzzzzz", 4754354226440025480}, // 12-char max value, exact in int64
		{"abcdefghijkl", 187939216216112118},
		{"abcdefghijklmnop", 187939216216112118}, // capped at 12 characters
	}
	for _, tc := range cases {
		if got := NameToLong(tc.in); got != tc.want {
			t.Errorf("NameToLong(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestNameToLongAstral(t *testing.T) {
	// An astral character consumes two UTF-16 units and adds nothing:
	// "a" + U+1F600 + "b" is 4 units: a, <surrogate>, <surrogate>, b.
	// l: 1 -> 37 -> 37*37=1369 -> 1369*37+2 = 50655.
	if got, want := NameToLong("a\U0001F600b"), int64(50655); got != want {
		t.Errorf("NameToLong(astral) = %d, want %d", got, want)
	}
}

func TestPendingKey(t *testing.T) {
	// pendingKey = protocol form of DisplayNames.key: case, underscores
	// and extra spaces don't count.
	for _, tc := range []struct{ in, want string }{
		{"John_Doe", "john_doe"},
		{"john doe", "john_doe"},
		{"  JOHN   DOE  ", "john_doe"},
		{"John__Doe", "john_doe"},
	} {
		if got := pendingKey(tc.in); got != tc.want {
			t.Errorf("pendingKey(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestUTF16Len(t *testing.T) {
	if got := utf16Len("abc"); got != 3 {
		t.Errorf("utf16Len(abc) = %d", got)
	}
	if got := utf16Len("a\U0001F600"); got != 3 {
		t.Errorf("utf16Len(a+astral) = %d, want 3", got)
	}
}
