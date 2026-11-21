package persist

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"time"
)

// Remembered logins ("Stay logged in"): port of net/login/LoginTokens.ts.
//
// The client keeps a random token instead of the password; the account
// keeps only its SHA-256 in the save attribute "login:tokens". Each token
// login swaps the token for a new one, so a copied token stops working
// once either copy is used, and presenting the swapped-out token again
// revokes that browser's entry (LoginTokens.ts:1-7).

// LoginTokensAttribute is the save attribute key holding the token
// entries (LoginTokens.ts:9).
const LoginTokensAttribute = "login:tokens"

// LoginTokenLifetime is how long a remembered login lasts from its last
// use: 30 days (LoginTokens.ts:11-12, LOGIN_TOKEN_LIFETIME_MS).
const LoginTokenLifetime = 30 * 24 * time.Hour

// MaxLoginTokens is the number of browsers remembered per account; a new
// one drops the least recently used (LoginTokens.ts:13-14).
const MaxLoginTokens = 5

// LOGIN packet flags (LoginTokens.ts:16-22).
const (
	LoginFlagRemember        = 1 // LOGIN_FLAG_REMEMBER
	LoginFlagToken           = 2 // LOGIN_FLAG_TOKEN: password field holds a token
	LoginFlagCharacterSelect = 4 // LOGIN_FLAG_CHARACTER_SELECT
)

// TokenEntry is one remembered browser: port of LoginTokenEntry
// (LoginTokens.ts:24-30).
//
// DIVERGENCE FROM THE ASSIGNED SKETCH: TS has NO CreatedAt field. The
// stored shape is exactly {hash, previousHash?, lastUsed}, where lastUsed
// is epoch milliseconds. This struct matches TS; there is no created-at
// information in a TS save to port.
type TokenEntry struct {
	// Hash is the SHA-256 hex of the token (see HashToken).
	Hash string `json:"hash"`
	// PreviousHash is the hash of the token this one replaced; seeing it
	// again means the browser's token was copied. Omitted when empty.
	PreviousHash string `json:"previousHash,omitempty"`
	// LastUsed is the last use, stored as epoch milliseconds in JSON.
	LastUsed time.Time `json:"lastUsed"`
}

// tokenEntryJSON is the wire shape of TokenEntry, with lastUsed as a
// millisecond number (LoginTokens.ts:29: `lastUsed: number`).
type tokenEntryJSON struct {
	Hash         string `json:"hash"`
	PreviousHash string `json:"previousHash,omitempty"`
	LastUsed     int64  `json:"lastUsed"`
}

// MarshalJSON stores lastUsed as epoch milliseconds, matching TS.
func (e TokenEntry) MarshalJSON() ([]byte, error) {
	return json.Marshal(tokenEntryJSON{
		Hash:         e.Hash,
		PreviousHash: e.PreviousHash,
		LastUsed:     e.LastUsed.UnixMilli(),
	})
}

// UnmarshalJSON reads lastUsed as epoch milliseconds. A non-numeric
// lastUsed is an error here; the lenient filtering port (ReadTokens)
// drops such entries instead, as TS readLoginTokens does.
func (e *TokenEntry) UnmarshalJSON(data []byte) error {
	var raw tokenEntryJSON
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	e.Hash = raw.Hash
	e.PreviousHash = raw.PreviousHash
	e.LastUsed = time.UnixMilli(raw.LastUsed).UTC()
	return nil
}

// hashPattern matches a SHA-256 hex digest. Lowercase only, exactly as
// TS HASH_PATTERN (LoginTokens.ts:37: /^[0-9a-f]{64}$/, no /i flag):
// hashLoginToken emits lowercase hex, so an uppercase entry is malformed
// and dropped by ReadTokens.
var tokenHashPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// HashToken returns the SHA-256 hex of token, port of hashLoginToken
// (LoginTokens.ts:39-41). TS hashes the token encoded as latin1
// (update(token, "latin1")). Generated tokens are base64url (ASCII), so
// latin1 and UTF-8 agree for them; for a presented token containing
// non-ASCII characters the latin1 byte mapping is preserved here (each
// rune contributes its low byte, as Node's latin1 encoding does).
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(latin1Bytes(token)))
	return hex.EncodeToString(sum[:])
}

func latin1Bytes(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		out = append(out, byte(r))
	}
	return out
}

// NewToken generates a new random token: 32 random bytes, base64url
// encoded without padding (43 characters), exactly as issueLoginToken
// does (LoginTokens.ts:72: randomBytes(32).toString("base64url")).
func NewToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("persist: generating login token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ReadTokens parses the stored "login:tokens" attribute value, returning
// the entries still in date and dropping anything malformed. Port of
// readLoginTokens (LoginTokens.ts:41-57): a non-array value yields no
// entries; entries with a malformed hash, a non-finite/non-numeric
// lastUsed, or lastUsed more than LoginTokenLifetime before now are
// dropped; a malformed previousHash is dropped while the entry is kept.
// An entry last used in the future is kept (now-lastUsed is negative,
// matching the TS comparison `now - lastUsed > LOGIN_TOKEN_LIFETIME_MS`).
func ReadTokens(value json.RawMessage, now time.Time) []TokenEntry {
	if len(value) == 0 {
		return nil
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(value, &raw); err != nil {
		return nil
	}
	entries := make([]TokenEntry, 0, len(raw))
	for _, item := range raw {
		var decoded struct {
			Hash         json.RawMessage `json:"hash"`
			PreviousHash json.RawMessage `json:"previousHash"`
			LastUsed     json.RawMessage `json:"lastUsed"`
		}
		if err := json.Unmarshal(item, &decoded); err != nil {
			continue
		}
		var hash string
		if err := json.Unmarshal(decoded.Hash, &hash); err != nil || !tokenHashPattern.MatchString(hash) {
			continue
		}
		var lastUsedMs float64
		if err := json.Unmarshal(decoded.LastUsed, &lastUsedMs); err != nil {
			continue
		}
		lastUsed := time.UnixMilli(int64(lastUsedMs)).UTC()
		if now.Sub(lastUsed) > LoginTokenLifetime {
			continue
		}
		entry := TokenEntry{Hash: hash, LastUsed: lastUsed}
		var prev string
		if err := json.Unmarshal(decoded.PreviousHash, &prev); err == nil && tokenHashPattern.MatchString(prev) {
			entry.PreviousHash = prev
		}
		entries = append(entries, entry)
	}
	return entries
}

// validTokens filters already-decoded entries the way readLoginTokens
// does: malformed hashes and expired entries are dropped, malformed
// previous hashes are cleared. Used by VerifyAndRotate and IssueToken so
// callers passing decoded entries get the same filtering the login path
// applies to the raw attribute.
func validTokens(entries []TokenEntry, now time.Time) []TokenEntry {
	out := make([]TokenEntry, 0, len(entries))
	for _, e := range entries {
		if !tokenHashPattern.MatchString(e.Hash) {
			continue
		}
		if now.Sub(e.LastUsed) > LoginTokenLifetime {
			continue
		}
		if e.PreviousHash != "" && !tokenHashPattern.MatchString(e.PreviousHash) {
			e.PreviousHash = ""
		}
		out = append(out, e)
	}
	return out
}

// VerifyAndRotate checks a presented remembered-login token against the
// account's entries and, on a current-hash match, rotates it. It composes
// checkLoginToken + issueLoginToken + revokeLoginToken
// (LoginTokens.ts:59-88) into the shape the login flow uses
// (NetworkBuilder.ts:1525-1536).
//
//   - Current-hash match: a new token is issued; the entry keeps the old
//     hash as PreviousHash and LastUsed becomes now. Returns ok=true and
//     the new (plaintext) token; the caller must persist `updated` before
//     answering, as the TS login flow does.
//   - previousHash match: the token was copied and already rotated; that
//     browser's entry is removed from `updated` and ok=false is returned
//     (the caller must persist `updated` to effect the revocation, as
//     login() does via saveLoginTokens).
//   - No match: ok=false, newToken is "", and `updated` is the input with
//     expired/malformed entries dropped.
//
// Expired (>30 days since last use) and malformed entries are dropped
// from `updated` in all cases, matching readLoginTokens filtering before
// the check in login().
//
// DIVERGENCE (max-5 on rotate): TS issueLoginToken truncates to
// MAX_LOGIN_TOKENS only on the add path, not on rotation. VerifyAndRotate
// additionally truncates to MaxLoginTokens (least-recently-used first)
// when rotation leaves more than MaxLoginTokens valid entries. That
// state cannot arise through TS (the add path always truncates), so the
// difference is unreachable from TS-written saves; it enforces the
// stated cap for hand-built entry lists.
//
// If random token generation fails (crypto/rand error), ok=false is
// returned with no token issued: authentication fails closed.
func VerifyAndRotate(tokens []TokenEntry, presented string, now time.Time) (updated []TokenEntry, newToken string, ok bool) {
	entries := validTokens(tokens, now)
	hash := HashToken(presented)
	for i, e := range entries {
		if e.Hash == hash {
			token, err := NewToken()
			if err != nil {
				return entries, "", false
			}
			entries[i] = TokenEntry{
				Hash:         HashToken(token),
				PreviousHash: e.Hash,
				LastUsed:     now.UTC(),
			}
			if len(entries) > MaxLoginTokens {
				// See DIVERGENCE note above.
				sort.SliceStable(entries, func(a, b int) bool {
					return entries[a].LastUsed.After(entries[b].LastUsed)
				})
				entries = entries[:MaxLoginTokens]
			}
			return entries, token, true
		}
	}
	for i, e := range entries {
		if e.PreviousHash != "" && e.PreviousHash == hash {
			entries = append(entries[:i], entries[i+1:]...)
			return entries, "", false
		}
	}
	return entries, "", false
}

// IssueToken adds a new remembered browser (a password login with the
// remember flag). Port of the add path of issueLoginToken
// (LoginTokens.ts:68-84, index undefined): the new entry is appended,
// entries are sorted most-recently-used first, and the list is truncated
// to MaxLoginTokens. Expired/malformed input entries are dropped first,
// matching the readLoginTokens filtering login() applies before issuing
// (NetworkBuilder.ts:1564-1565).
func IssueToken(entries []TokenEntry, now time.Time) (updated []TokenEntry, token string, err error) {
	token, err = NewToken()
	if err != nil {
		return nil, "", err
	}
	next := validTokens(entries, now)
	next = append(next, TokenEntry{Hash: HashToken(token), LastUsed: now.UTC()})
	sort.SliceStable(next, func(a, b int) bool {
		return next[a].LastUsed.After(next[b].LastUsed)
	})
	if len(next) > MaxLoginTokens {
		next = next[:MaxLoginTokens]
	}
	return next, token, nil
}
