package persist

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// Password hashing: port of util/PasswordUtil.ts.
//
// Stored format: "<saltHex>:<hashHex>", 16-byte random salt, 64-byte scrypt
// key (PasswordUtil.ts:7-13).
//
// SCRYPT PARAMETERS — VERIFICATION STATUS: PasswordUtil.ts calls Node's
// crypto.scrypt with NO options object, so the parameters are Node's
// defaults. Per Node's documentation those are N=16384, r=8, p=1. The
// parameters below were verified empirically against Node v24.22.1's
// crypto.scrypt using the exact call shape in PasswordUtil.ts
// (scrypt(password, saltHexString, 64)); see the golden vectors in
// password_test.go. They have NOT been verified against a real TSPS
// production hash: if the TS server ran under a Node version or build
// with different scrypt defaults, stored hashes would not verify. Do not
// change these values without re-running that check.
//
// Two details that must be preserved exactly:
//   - The salt passed to scrypt is the 32-character hex STRING, not the
//     decoded 16 bytes (PasswordUtil.ts:9-10 passes `salt`, the hex
//     string, to scryptAsync).
//   - The password is passed as a string, i.e. UTF-8 bytes, in both
//     Node and Go.
const (
	scryptN       = 16384
	scryptR       = 8
	scryptP       = 1
	scryptKeyLen  = 64
	scryptSaltLen = 16 // bytes; 32 hex characters when stored
)

var (
	// PasswordUtil.ts:16-17. Case-insensitive, matching the /i regexes.
	saltPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
	hashPattern = regexp.MustCompile(`^[0-9a-f]{128}$`)
)

// HashPassword generates a salted scrypt hash for password, in the
// "<saltHex>:<hashHex>" format. Port of
// PasswordUtil.generatePasswordHashWithSalt (PasswordUtil.ts:7-13).
func HashPassword(password string) (string, error) {
	salt := make([]byte, scryptSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("persist: generating password salt: %w", err)
	}
	saltHex := hex.EncodeToString(salt)
	key, err := scrypt.Key([]byte(password), []byte(saltHex), scryptN, scryptR, scryptP, scryptKeyLen)
	if err != nil {
		return "", fmt.Errorf("persist: hashing password: %w", err)
	}
	return saltHex + ":" + hex.EncodeToString(key), nil
}

// CheckPassword reports whether password matches the stored credential.
//
// Two stored shapes exist, and the branch is chosen exactly as in the TS
// login path (net/NetworkBuilder.ts:1539-1548):
//   - No ":" in stored: a legacy plaintext password from before hashing.
//     Exact match, and only when stored is non-empty (login() requires
//     `!!stored`). An empty stored value means "new account" in TS and
//     must be handled by the caller, not by this function: login()
//     proceeds to hash the supplied password when there is no stored
//     value at all. Note PasswordUtil.passwordsMatch itself would return
//     false for a plaintext value; the plaintext branch lives in login()
//     (and in NullPlayerPersistence.checkPassword), not in PasswordUtil.
//   - ":" present: strict scrypt verification per
//     PasswordUtil.passwordsMatch (PasswordUtil.ts:15-23): the salt and
//     hash are the first two ":"-separated segments (TS destructures
//     split(":") and ignores any further segments), both must match the
//     strict hex regexes, and the comparison is timing-safe. Malformed
//     stored values return false, never an error, matching TS.
func CheckPassword(password, stored string) bool {
	if !strings.Contains(stored, ":") {
		// Legacy plaintext branch (NetworkBuilder.ts:1542).
		return stored != "" && stored == password
	}
	parts := strings.Split(stored, ":")
	salt, hashHex := parts[0], parts[1]
	if !saltPattern.MatchString(strings.ToLower(salt)) || !hashPattern.MatchString(strings.ToLower(hashHex)) {
		return false
	}
	hash, err := hex.DecodeString(hashHex)
	if err != nil {
		return false
	}
	// TS derives hash.length bytes (PasswordUtil.ts:20); the regex above
	// guarantees 64.
	candidate, err := scrypt.Key([]byte(password), []byte(salt), scryptN, scryptR, scryptP, len(hash))
	if err != nil {
		return false
	}
	return len(candidate) == len(hash) && subtle.ConstantTimeCompare(candidate, hash) == 1
}
