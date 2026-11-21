package persist

import (
	"encoding/hex"
	"strings"
	"testing"
)

// All passwords/tokens in these tests are fixed synthetic values, never
// real credentials.

func TestHashPasswordFormat(t *testing.T) {
	stored, err := HashPassword("test-password-1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	parts := strings.Split(stored, ":")
	if len(parts) != 2 {
		t.Fatalf("stored %q: want exactly one colon", stored)
	}
	salt, hash := parts[0], parts[1]
	if len(salt) != 32 || len(hash) != 128 {
		t.Fatalf("stored lengths salt=%d hash=%d, want 32/128", len(salt), len(hash))
	}
	if _, err := hex.DecodeString(salt); err != nil {
		t.Fatalf("salt not hex: %v", err)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		t.Fatalf("hash not hex: %v", err)
	}
	// Two hashes of the same password differ (random salt).
	other, err := HashPassword("test-password-1")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if stored == other {
		t.Fatal("two hashes of the same password are identical; salt must be random")
	}
}

func TestCheckPasswordRoundTrip(t *testing.T) {
	stored, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !CheckPassword("correct horse battery staple", stored) {
		t.Fatal("correct password did not verify")
	}
	if CheckPassword("correct horse battery staples", stored) {
		t.Fatal("wrong password verified")
	}
	if CheckPassword("", stored) {
		t.Fatal("empty password verified against a hash")
	}
}

// TestCheckPasswordNodeGoldenVectors verifies the scrypt parameters
// against Node's crypto.scrypt defaults, using the exact call shape in
// PasswordUtil.ts (scrypt(password, saltHexString, 64)). Generated with
// Node v24.22.1:
//
//	node -e '...scrypt("correct horse battery staple", "0123...cdef", 64)...'
//
// These vectors pin N=16384, r=8, p=1 and the "salt as hex string"
// detail. They are NOT production TSPS hashes.
func TestCheckPasswordNodeGoldenVectors(t *testing.T) {
	vectors := []struct{ password, stored string }{
		{
			"correct horse battery staple",
			"0123456789abcdef0123456789abcdef:b9e4c0458defb164feba9d9ffbec86e4ddffb2a590e4afa11b8a170fbd1512180c3a5eefbb59f1d53515369c1639cc3cf7f291109291b9082c04aebd75e4c8cb",
		},
		{
			"hunter2",
			"aabbccddeeff00112233445566778899:0334243d4ce826cc89e705493115dee6c4776db76e972337ed24ecd8b5c06082fb3c0d0c257a7a0222a89eb7d36d6eb16ebb135e36706abe4e117d6fee75701a",
		},
	}
	for _, v := range vectors {
		if !CheckPassword(v.password, v.stored) {
			t.Fatalf("Node golden vector did not verify for password %q", v.password)
		}
		if CheckPassword(v.password+"x", v.stored) {
			t.Fatalf("wrong password verified against Node golden vector for %q", v.password)
		}
	}
}

func TestCheckPasswordLegacyPlaintext(t *testing.T) {
	// NetworkBuilder.ts:1542: no ":" => exact match against the stored
	// plaintext, only when stored is non-empty.
	if !CheckPassword("oldpassword", "oldpassword") {
		t.Fatal("legacy plaintext exact match failed")
	}
	if CheckPassword("oldpasswor", "oldpassword") {
		t.Fatal("legacy plaintext near-miss verified")
	}
	if CheckPassword("oldpassword", "Oldpassword") {
		t.Fatal("legacy plaintext match must be case-sensitive")
	}
	if CheckPassword("", "") {
		t.Fatal("empty stored value must not verify (new-account sentinel)")
	}
	if CheckPassword("anything", "") {
		t.Fatal("empty stored value must not verify")
	}
}

func TestCheckPasswordMalformed(t *testing.T) {
	salt := strings.Repeat("a", 32)
	hash := strings.Repeat("b", 128)
	cases := []string{
		salt + ":short",                       // hash too short
		"short:" + hash,                       // salt too short
		strings.Repeat("g", 32) + ":" + hash,  // non-hex salt
		salt + ":" + strings.Repeat("g", 128), // non-hex hash
		salt + ":",                            // empty hash
		":" + hash,                            // empty salt
		":",                                   // both empty
		salt + ":" + hash[:127] + ":",         // trailing segment ignored by TS split, but hash segment is short
	}
	for _, stored := range cases {
		if CheckPassword("whatever", stored) {
			t.Fatalf("malformed stored value %q verified", stored)
		}
	}
	// A valid salt:hash with an extra trailing segment: TS destructures
	// split(":") and ignores further segments, so this still verifies.
	// (Uses a Node golden vector with ":extra" appended.)
	stored := "0123456789abcdef0123456789abcdef:b9e4c0458defb164feba9d9ffbec86e4ddffb2a590e4afa11b8a170fbd1512180c3a5eefbb59f1d53515369c1639cc3cf7f291109291b9082c04aebd75e4c8cb:extra"
	if !CheckPassword("correct horse battery staple", stored) {
		t.Fatal("stored value with extra trailing segment should verify per TS destructuring")
	}
	// Uppercasing the salt changes the scrypt input (the salt is the hex
	// STRING), so it must not verify against a hash made with the
	// lowercase salt string.
	upperSalt := "0123456789ABCDEF0123456789ABCDEF:b9e4c0458defb164feba9d9ffbec86e4ddffb2a590e4afa11b8a170fbd1512180c3a5eefbb59f1d53515369c1639cc3cf7f291109291b9082c04aebd75e4c8cb"
	if CheckPassword("correct horse battery staple", upperSalt) {
		t.Fatal("uppercased salt string must not verify against lowercase-salt hash")
	}
}
