package persist

import (
	"encoding/base64"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Fixed test time: 2026-11-13T15:00:00Z.
var tokenNow = time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)

func TestHashTokenSHA256Hex(t *testing.T) {
	// SHA-256 of "abc" is a well-known value; HashToken must emit
	// lowercase hex of the latin1 bytes.
	got := HashToken("abc")
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("HashToken(abc) = %s, want %s", got, want)
	}
	if len(HashToken("test-token")) != 64 {
		t.Fatal("hash must be 64 hex chars")
	}
}

func TestNewTokenFormat(t *testing.T) {
	tok, err := NewToken()
	if err != nil {
		t.Fatalf("NewToken: %v", err)
	}
	// 32 bytes base64url without padding = 43 chars, URL-safe alphabet.
	if len(tok) != 43 {
		t.Fatalf("token length %d, want 43", len(tok))
	}
	raw, err := base64.RawURLEncoding.DecodeString(tok)
	if err != nil || len(raw) != 32 {
		t.Fatalf("token is not 32 bytes base64url: %v (len %d)", err, len(raw))
	}
	other, _ := NewToken()
	if tok == other {
		t.Fatal("two tokens identical; must be random")
	}
}

func entryFor(t *testing.T, token string, lastUsed time.Time) TokenEntry {
	t.Helper()
	return TokenEntry{Hash: HashToken(token), LastUsed: lastUsed}
}

func TestVerifyAndRotate(t *testing.T) {
	entries := []TokenEntry{entryFor(t, "browser-token-1", tokenNow.Add(-time.Hour))}
	updated, newToken, ok := VerifyAndRotate(entries, "browser-token-1", tokenNow)
	if !ok {
		t.Fatal("valid token rejected")
	}
	if newToken == "" || newToken == "browser-token-1" {
		t.Fatal("rotation must issue a fresh token")
	}
	if len(updated) != 1 {
		t.Fatalf("updated has %d entries, want 1", len(updated))
	}
	if updated[0].Hash != HashToken(newToken) {
		t.Fatal("rotated entry hash does not match the new token")
	}
	if updated[0].PreviousHash != HashToken("browser-token-1") {
		t.Fatal("rotated entry must keep the old hash as previousHash")
	}
	if !updated[0].LastUsed.Equal(tokenNow) {
		t.Fatalf("rotated lastUsed = %v, want %v", updated[0].LastUsed, tokenNow)
	}
	// The new token verifies on the next login.
	if _, _, ok := VerifyAndRotate(updated, newToken, tokenNow.Add(time.Hour)); !ok {
		t.Fatal("rotated token did not verify on next use")
	}
}

func TestVerifyAndRotateReuseRevokes(t *testing.T) {
	entries := []TokenEntry{entryFor(t, "browser-token-1", tokenNow.Add(-time.Hour))}
	updated, _, ok := VerifyAndRotate(entries, "browser-token-1", tokenNow)
	if !ok {
		t.Fatal("valid token rejected")
	}
	// Presenting the swapped-out token again revokes the browser's entry.
	updated2, newToken, ok := VerifyAndRotate(updated, "browser-token-1", tokenNow.Add(time.Hour))
	if ok {
		t.Fatal("reused token accepted")
	}
	if newToken != "" {
		t.Fatal("reused token must not issue a token")
	}
	if len(updated2) != 0 {
		t.Fatalf("revoked entry not removed: %d entries remain", len(updated2))
	}
	// Other browsers are unaffected by a revocation.
	entries = []TokenEntry{
		entryFor(t, "browser-token-1", tokenNow.Add(-time.Hour)),
		entryFor(t, "browser-token-2", tokenNow.Add(-2*time.Hour)),
	}
	updated, _, _ = VerifyAndRotate(entries, "browser-token-1", tokenNow)
	updated, _, ok = VerifyAndRotate(updated, "browser-token-1", tokenNow.Add(time.Hour))
	if ok || len(updated) != 1 || updated[0].Hash != HashToken("browser-token-2") {
		t.Fatalf("revocation removed the wrong entry: ok=%v entries=%v", ok, updated)
	}
}

func TestVerifyAndRotateNoMatchAndExpiry(t *testing.T) {
	entries := []TokenEntry{entryFor(t, "browser-token-1", tokenNow.Add(-time.Hour))}
	updated, newToken, ok := VerifyAndRotate(entries, "unknown-token", tokenNow)
	if ok || newToken != "" {
		t.Fatal("unknown token accepted")
	}
	if len(updated) != 1 {
		t.Fatal("no-match must keep the existing entries")
	}
	// Expired: last used 31 days ago (> 30-day lifetime) => dropped, no match.
	expired := []TokenEntry{entryFor(t, "old-token", tokenNow.Add(-31*24*time.Hour))}
	updated, _, ok = VerifyAndRotate(expired, "old-token", tokenNow)
	if ok {
		t.Fatal("expired token accepted")
	}
	if len(updated) != 0 {
		t.Fatal("expired entry must be dropped from updated")
	}
	// Exactly at the boundary: now-lastUsed == lifetime is NOT expired
	// (TS: now - lastUsed > LIFETIME).
	boundary := []TokenEntry{entryFor(t, "edge-token", tokenNow.Add(-LoginTokenLifetime))}
	if _, _, ok := VerifyAndRotate(boundary, "edge-token", tokenNow); !ok {
		t.Fatal("token exactly at the lifetime boundary rejected")
	}
	// Malformed entries are dropped without affecting valid ones.
	mixed := []TokenEntry{
		{Hash: "not-a-hash", LastUsed: tokenNow},
		entryFor(t, "good-token", tokenNow.Add(-time.Hour)),
	}
	updated, _, ok = VerifyAndRotate(mixed, "good-token", tokenNow)
	if !ok || len(updated) != 1 {
		t.Fatalf("malformed entry handling wrong: ok=%v len=%d", ok, len(updated))
	}
	// Uppercase hashes are malformed per the TS lowercase-only pattern.
	upper := []TokenEntry{{Hash: strings.ToUpper(HashToken("upper-token")), LastUsed: tokenNow}}
	if _, _, ok := VerifyAndRotate(upper, "upper-token", tokenNow); ok {
		t.Fatal("uppercase hash entry must be dropped as malformed")
	}
}

func TestIssueTokenMaxFiveLRU(t *testing.T) {
	var entries []TokenEntry
	// Five browsers, oldest first.
	for i := 0; i < 5; i++ {
		entries = append(entries, TokenEntry{
			Hash:     HashToken("browser-" + string(rune('a'+i))),
			LastUsed: tokenNow.Add(time.Duration(-5+i) * time.Hour),
		})
	}
	updated, tok, err := IssueToken(entries, tokenNow)
	if err != nil {
		t.Fatalf("IssueToken: %v", err)
	}
	if len(updated) != MaxLoginTokens {
		t.Fatalf("after issue: %d entries, want %d", len(updated), MaxLoginTokens)
	}
	if updated[0].Hash != HashToken(tok) {
		t.Fatal("newest entry must sort first")
	}
	// The least recently used (browser-a) must have been evicted.
	for _, e := range updated {
		if e.Hash == HashToken("browser-a") {
			t.Fatal("LRU entry was not evicted")
		}
	}
	// Rotating when more than 5 valid entries exist truncates to 5.
	many := append(append([]TokenEntry{}, entries...), entryFor(t, "sixth", tokenNow.Add(-30*time.Minute)))
	updated, _, ok := VerifyAndRotate(many, "sixth", tokenNow)
	if !ok {
		t.Fatal("valid token in oversized list rejected")
	}
	if len(updated) != MaxLoginTokens {
		t.Fatalf("oversized list after rotate: %d entries, want %d", len(updated), MaxLoginTokens)
	}
}

func TestReadTokens(t *testing.T) {
	hash := HashToken("stored-token")
	raw := json.RawMessage(`[
		{"hash": "` + hash + `", "previousHash": "` + HashToken("prev-token") + `", "lastUsed": ` + strconv.FormatInt(tokenNow.Add(-time.Hour).UnixMilli(), 10) + `},
		{"hash": "bad", "lastUsed": ` + strconv.FormatInt(tokenNow.UnixMilli(), 10) + `},
		{"hash": "` + HashToken("expired") + `", "lastUsed": ` + strconv.FormatInt(tokenNow.Add(-40*24*time.Hour).UnixMilli(), 10) + `},
		{"hash": "` + HashToken("nostamp") + `"},
		"junk",
		{"hash": "` + HashToken("badprev") + `", "previousHash": "notahash", "lastUsed": ` + strconv.FormatInt(tokenNow.UnixMilli(), 10) + `}
	]`)
	entries := ReadTokens(raw, tokenNow)
	if len(entries) != 2 {
		t.Fatalf("ReadTokens returned %d entries, want 2 (valid + badprev)", len(entries))
	}
	if entries[0].Hash != hash || entries[0].PreviousHash != HashToken("prev-token") {
		t.Fatalf("first entry wrong: %+v", entries[0])
	}
	if entries[1].PreviousHash != "" {
		t.Fatal("malformed previousHash must be dropped, entry kept")
	}
	if got := ReadTokens(json.RawMessage(`{"not":"an array"}`), tokenNow); len(got) != 0 {
		t.Fatal("non-array attribute must yield no entries")
	}
	if got := ReadTokens(nil, tokenNow); len(got) != 0 {
		t.Fatal("missing attribute must yield no entries")
	}
}

func TestTokenEntryJSONShape(t *testing.T) {
	// Stored shape must be exactly {hash, previousHash?, lastUsed(ms)}.
	e := TokenEntry{Hash: HashToken("x"), PreviousHash: HashToken("y"), LastUsed: tokenNow}
	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["lastUsed"] != float64(tokenNow.UnixMilli()) {
		t.Fatalf("lastUsed = %v, want ms %d", m["lastUsed"], tokenNow.UnixMilli())
	}
	var back TokenEntry
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("round-trip: %v", err)
	}
	if back != e {
		t.Fatalf("round-trip mismatch: %+v vs %+v", back, e)
	}
	// previousHash omitted when empty.
	e2 := TokenEntry{Hash: HashToken("x"), LastUsed: tokenNow}
	data2, _ := json.Marshal(e2)
	if strings.Contains(string(data2), "previousHash") {
		t.Fatalf("empty previousHash must be omitted: %s", data2)
	}
}
