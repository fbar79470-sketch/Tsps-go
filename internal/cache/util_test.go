package cache

import "testing"

func TestCRC32(t *testing.T) {
	// The standard IEEE check value.
	if got := CRC32([]byte("123456789")); got != 0xCBF43926 {
		t.Errorf("CRC32(123456789) = %08x, want cbf43926", got)
	}
	if got := CRC32(nil); got != 0 {
		t.Errorf("CRC32(empty) = %08x, want 0", got)
	}
}

func TestHashDjb2(t *testing.T) {
	if got := HashDjb2(""); got != 0 {
		t.Errorf("HashDjb2(\"\") = %d", got)
	}
	if got := HashDjb2("a"); got != 97 {
		t.Errorf("HashDjb2(\"a\") = %d, want 97", got)
	}
	// Hand-computed: hash("ab") = ((0*31+97)*31+98) = 3105.
	if got := HashDjb2("ab"); got != 3105 {
		t.Errorf("HashDjb2(\"ab\") = %d, want 3105", got)
	}
	// Case-sensitive: djb2 does not uppercase.
	if HashDjb2("A") == HashDjb2("a") {
		t.Error("HashDjb2 should be case-sensitive")
	}
	// Deterministic across calls.
	if HashDjb2("l42_56") != HashDjb2("l42_56") {
		t.Error("HashDjb2 not deterministic")
	}
}

func TestHashOld(t *testing.T) {
	// Hand-computed: hash("A") = 0*61+65-32 = 33.
	if got := HashOld("A"); got != 33 {
		t.Errorf("HashOld(\"A\") = %d, want 33", got)
	}
	// hash("AB") = 33*61+66-32 = 2047.
	if got := HashOld("AB"); got != 2047 {
		t.Errorf("HashOld(\"AB\") = %d, want 2047", got)
	}
	// Case-insensitive: the name is uppercased first.
	if HashOld("ab") != HashOld("AB") {
		t.Error("HashOld should be case-insensitive")
	}
	if got := HashOld(""); got != 0 {
		t.Errorf("HashOld(\"\") = %d", got)
	}
}
