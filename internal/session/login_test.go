package session

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"tsps-go/internal/persist"
)

// memoryStore is a SaveStore fixture keyed by persist.StorageKey.
type memoryStore struct {
	saves   map[string]*persist.PlayerSave
	loadErr error
	saveErr error
	saved   []string
}

func newMemoryStore() *memoryStore { return &memoryStore{saves: map[string]*persist.PlayerSave{}} }

func (m *memoryStore) Load(username string, profile persist.Profile) (*persist.PlayerSave, error) {
	if m.loadErr != nil {
		return nil, m.loadErr
	}
	if s, ok := m.saves[persist.StorageKey(username, profile)]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *memoryStore) Save(username string, profile persist.Profile, save *persist.PlayerSave) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.saved = append(m.saved, persist.StorageKey(username, profile))
	m.saves[persist.StorageKey(username, profile)] = save
	return nil
}

func (m *memoryStore) put(username string, profile persist.Profile, save *persist.PlayerSave) {
	m.saves[persist.StorageKey(username, profile)] = save
}

// reservations records ReserveName/ReleaseName calls.
type reservations struct {
	held     map[string]bool
	reserved []string
	released []string
}

func newReservations() *reservations { return &reservations{held: map[string]bool{}} }

func (r *reservations) reserve(key string) bool {
	if r.held[key] {
		return false
	}
	r.held[key] = true
	r.reserved = append(r.reserved, key)
	return true
}

func (r *reservations) release(key string) {
	delete(r.held, key)
	r.released = append(r.released, key)
}

type loginFixture struct {
	deps  *LoginDeps
	store *memoryStore
	resv  *reservations
	now   time.Time
}

func newLoginFixture(t *testing.T) *loginFixture {
	t.Helper()
	dir := t.TempDir()
	accounts, err := persist.LoadAccounts(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	punishment, err := persist.LoadPunishment(filepath.Join(dir, "saves"))
	if err != nil {
		t.Fatal(err)
	}
	displayNames, err := persist.LoadDisplayNames(filepath.Join(dir, "display-names.json"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	store := newMemoryStore()
	resv := newReservations()
	deps := &LoginDeps{
		ActiveRevision: 242,
		Accounts:       accounts,
		Punishment:     punishment,
		DisplayNames:   displayNames,
		Saves:          store,
		ReserveName:    resv.reserve,
		ReleaseName:    resv.release,
		Now:            func() time.Time { return now },
	}
	return &loginFixture{deps: deps, store: store, resv: resv, now: now}
}

// seedAccount creates an account with primary + optional alts and a
// scrypt-hashed save for the primary on the main profile.
func (f *loginFixture) seedAccount(t *testing.T, primary, password string, alts ...string) {
	t.Helper()
	if _, err := f.deps.Accounts.Ensure(primary, f.now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	for _, alt := range alts {
		accountKey, _ := f.deps.Accounts.AccountOf(primary)
		if _, err := f.deps.Accounts.AddCharacter(accountKey, alt, f.now.UnixMilli()); err != nil {
			t.Fatal(err)
		}
	}
	hash, err := persist.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	f.store.put(primary, persist.ProfileMain, &persist.PlayerSave{
		DisplayName:          primary,
		PasswordHashWithSalt: hash,
	})
}

func auth(t *testing.T, f *loginFixture, username, password string, revision int32, flags byte) (*PendingLogin, int32, string) {
	t.Helper()
	return Authenticate(f.deps, username, password, revision, flags, "1.2.3.4")
}

// releaseAll simulates connection cleanup between sequential logins on
// one fixture: a successful Authenticate leaves its name reservations
// held (as TS pendingNames does until cleanup/enterWorld), so the next
// login for the same account would otherwise fail with code 5.
func (f *loginFixture) releaseAll() {
	for key := range f.resv.held {
		f.resv.release(key)
	}
}

func TestAuthenticateRevisionMismatch(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	if p, code, msg := auth(t, f, "Alice", "secret", 241, 0); p != nil || code != 6 || msg != "Please close the client and reload to update." {
		t.Errorf("revision mismatch: pending=%v code=%d msg=%q", p, code, msg)
	}
}

func TestAuthenticateInvalidNamesAndPasswords(t *testing.T) {
	f := newLoginFixture(t)
	cases := []struct{ user, pass string }{
		{"", "x"},
		{" ", "x"},                 // trims to empty before the length check
		{"thisnameistoolong", "x"}, // 17 chars > 12
		{"bad name!", "x"},
		{"a-b", "x"},
		{"Alice", ""},
		{"Alice", strings.Repeat("x", 65)},
	}
	for _, tc := range cases {
		if p, code, _ := auth(t, f, tc.user, tc.pass, 242, 0); p != nil || code != 3 {
			t.Errorf("Authenticate(%q, pass len %d) = pending=%v code=%d, want code 3", tc.user, len(tc.pass), p, code)
		}
	}
	// A 12-character name and 64-character password are accepted shape-wise
	// (new account path succeeds).
	if p, code, _ := auth(t, f, "TwelveChars1", strings.Repeat("x", 64), 242, 0); p == nil || code != 0 {
		t.Errorf("boundary lengths: pending=%v code=%d", p, code)
	}
}

func TestAuthenticateBannedAndIPBanned(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	if err := f.deps.Punishment.Ban("Alice"); err != nil {
		t.Fatal(err)
	}
	if p, code, msg := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 4 || msg != "Your account has been disabled." {
		t.Errorf("banned: pending=%v code=%d msg=%q", p, code, msg)
	}

	f2 := newLoginFixture(t)
	f2.seedAccount(t, "Bob", "secret")
	if err := f2.deps.Punishment.AddBannedIP("1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if p, code, msg := auth(t, f2, "Bob", "secret", 242, 0); p != nil || code != 27 || msg != "Your IP address has been banned." {
		t.Errorf("IP banned: pending=%v code=%d msg=%q", p, code, msg)
	}
}

func TestAuthenticateBannedPrimaryViaAlt(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret", "Alicia")
	if err := f.deps.Punishment.Ban("Alice"); err != nil {
		t.Fatal(err)
	}
	if p, code, _ := auth(t, f, "Alicia", "secret", 242, 0); p != nil || code != 4 {
		t.Errorf("alt of banned primary: pending=%v code=%d, want 4", p, code)
	}
}

func TestAuthenticateDeletedName(t *testing.T) {
	dir := t.TempDir()
	// Seed a tombstone directly: "Bob" deleted by account "alice".
	accountsJSON := `{"accounts": {}, "deleted": {"bob": {"name": "Bob", "account": "alice", "deletedAt": 1}}}`
	if err := os.WriteFile(filepath.Join(dir, "accounts.json"), []byte(accountsJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newLoginFixture(t)
	accounts, err := persist.LoadAccounts(filepath.Join(dir, "accounts.json"))
	if err != nil {
		t.Fatal(err)
	}
	f.deps.Accounts = accounts
	if p, code, msg := auth(t, f, "Bob", "secret", 242, 0); p != nil || code != 3 || !strings.Contains(msg, "deleted character") {
		t.Errorf("deleted name: pending=%v code=%d msg=%q", p, code, msg)
	}
}

func TestAuthenticateAlreadyOnlineAndWorldFull(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	f.deps.IsOnline = func(key string) bool { return key == "alice" }
	if p, code, msg := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 5 || msg != "Your account is already logged in." {
		t.Errorf("online: pending=%v code=%d msg=%q", p, code, msg)
	}
	// World-full takes precedence over already-online for the code.
	f.deps.WorldFull = func() bool { return true }
	if p, code, msg := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 2 || msg != "This world is full." {
		t.Errorf("full+online: pending=%v code=%d msg=%q", p, code, msg)
	}
	// World-full alone.
	f.deps.IsOnline = nil
	if p, code, _ := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 2 {
		t.Errorf("full: pending=%v code=%d, want 2", p, code)
	}
}

func TestAuthenticatePendingReservationConflict(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	f.resv.held["alice"] = true // another login holds the account key
	if p, code, _ := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 5 {
		t.Errorf("reserved: pending=%v code=%d, want 5", p, code)
	}
}

func TestAuthenticateReservationReleasedOnFailure(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	// Wrong password: the reservation made in step 5 must be released.
	if p, code, _ := auth(t, f, "Alice", "wrong", 242, 0); p != nil || code != 3 {
		t.Fatalf("wrong password: pending=%v code=%d", p, code)
	}
	if len(f.resv.held) != 0 {
		t.Errorf("reservations leaked after failure: %v", f.resv.held)
	}
	if len(f.resv.released) == 0 {
		t.Error("ReleaseName not called on failure")
	}
	// Success: reservations held (account + character keys coincide for
	// a primary login, so exactly one key is held).
	p, code, _ := auth(t, f, "Alice", "secret", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("success: pending=%v code=%d", p, code)
	}
	if !f.resv.held["alice"] {
		t.Errorf("account key not held after success: %v", f.resv.held)
	}
}

func TestAuthenticateNewAccount(t *testing.T) {
	f := newLoginFixture(t)
	p, code, msg := auth(t, f, "Newbie", "hunter2", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("new account: pending=%v code=%d msg=%q", p, code, msg)
	}
	if p.Username != "Newbie" || p.CredentialName != "Newbie" || p.DisplayName != "Newbie" {
		t.Errorf("pending names: %+v", p)
	}
	if p.Profile != "main" || p.Save != nil {
		t.Errorf("pending profile/save: %q %v", p.Profile, p.Save)
	}
	if !strings.Contains(p.PasswordHash, ":") || p.PasswordHash == "hunter2" {
		t.Errorf("new account password not hashed: %q", p.PasswordHash)
	}
	if p.LoginToken != "" {
		t.Errorf("LoginToken without remember flag: %q", p.LoginToken)
	}
	if p.CharacterSelect {
		t.Error("CharacterSelect set without flag")
	}
	// The hash verifies.
	if !persist.CheckPassword("hunter2", p.PasswordHash) {
		t.Error("new account hash does not verify")
	}
}

func TestAuthenticateExistingPassword(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	p, code, _ := auth(t, f, "alice", "secret", 242, 0) // case-insensitive login
	if p == nil || code != 0 {
		t.Fatalf("existing: pending=%v code=%d", p, code)
	}
	if p.Username != "Alice" {
		t.Errorf("Username = %q, want saved capitals %q", p.Username, "Alice")
	}
	if p.Save == nil || p.Save.DisplayName != "Alice" {
		t.Errorf("Save = %+v", p.Save)
	}
	stored, _ := f.store.Load("Alice", persist.ProfileMain)
	if p.PasswordHash != stored.PasswordHashWithSalt {
		t.Error("scrypt match did not keep the stored hash")
	}
}

func TestAuthenticateLegacyPlaintextUpgraded(t *testing.T) {
	f := newLoginFixture(t)
	if _, err := f.deps.Accounts.Ensure("Old", f.now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	f.store.put("Old", persist.ProfileMain, &persist.PlayerSave{
		DisplayName:          "Old",
		PasswordHashWithSalt: "plaintext-pw",
	})
	p, code, _ := auth(t, f, "Old", "plaintext-pw", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("legacy plaintext: pending=%v code=%d", p, code)
	}
	if !strings.Contains(p.PasswordHash, ":") {
		t.Errorf("legacy password not upgraded to a hash: %q", p.PasswordHash)
	}
	if !persist.CheckPassword("plaintext-pw", p.PasswordHash) {
		t.Error("upgraded hash does not verify")
	}
	// A wrong password against a legacy plaintext fails.
	f.releaseAll()
	if p, code, _ := auth(t, f, "Old", "nope", 242, 0); p != nil || code != 3 {
		t.Errorf("legacy wrong password: pending=%v code=%d", p, code)
	}
}

func TestAuthenticateEmptyStoredHashAdoptsPassword(t *testing.T) {
	f := newLoginFixture(t)
	if _, err := f.deps.Accounts.Ensure("NoHash", f.now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	f.store.put("NoHash", persist.ProfileMain, &persist.PlayerSave{DisplayName: "NoHash"})
	p, code, _ := auth(t, f, "NoHash", "anything", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("empty stored hash: pending=%v code=%d, want success (adopts password)", p, code)
	}
	if !strings.Contains(p.PasswordHash, ":") {
		t.Errorf("adopted password not hashed: %q", p.PasswordHash)
	}
}

func TestAuthenticateCredentialFallback(t *testing.T) {
	// League world, only a main save exists: the main save vouches.
	f := newLoginFixture(t)
	f.deps.Profile = persist.ProfileLeague
	f.seedAccount(t, "Alice", "secret")
	p, code, _ := auth(t, f, "Alice", "secret", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("main fallback on league world: pending=%v code=%d", p, code)
	}
	if p.Profile != "league" {
		t.Errorf("Profile = %q, want league", p.Profile)
	}
	if p.Save != nil {
		t.Error("Save should be nil: no league save exists (new league character)")
	}

	// Main world, only a hardcore save exists: any other profile's save
	// vouches (NetworkBuilder.ts:1482-1485).
	f2 := newLoginFixture(t)
	if _, err := f2.deps.Accounts.Ensure("Hard", f2.now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	hash, _ := persist.HashPassword("secret")
	f2.store.put("Hard", persist.ProfileHardcore, &persist.PlayerSave{DisplayName: "Hard", PasswordHashWithSalt: hash})
	p, code, _ = auth(t, f2, "Hard", "secret", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("other-profile fallback: pending=%v code=%d", p, code)
	}
	if p.Save != nil {
		t.Error("Save should be nil: no main save exists")
	}
}

func TestAuthenticateAltLogin(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret", "Alicia")
	// The alt has its own save, made with different capitals.
	f.store.put("Alicia", persist.ProfileMain, &persist.PlayerSave{DisplayName: "ALICIA"})
	p, code, _ := auth(t, f, "alicia", "secret", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("alt login: pending=%v code=%d", p, code)
	}
	if p.Username != "ALICIA" {
		t.Errorf("alt Username = %q, want save capitals %q", p.Username, "ALICIA")
	}
	if p.CredentialName != "Alice" {
		t.Errorf("CredentialName = %q, want Alice", p.CredentialName)
	}
	if p.Save == nil || p.Save.DisplayName != "ALICIA" {
		t.Errorf("alt Save = %+v", p.Save)
	}
	// Both the account key and the alt character key are reserved.
	if !f.resv.held["alice"] || !f.resv.held["alicia"] {
		t.Errorf("held reservations = %v, want alice + alicia", f.resv.held)
	}
}

func TestAuthenticateNameTakenByDisplayName(t *testing.T) {
	f := newLoginFixture(t)
	// Another account shows "Taken".
	if err := f.deps.DisplayNames.Set("Other", "Taken", f.now, int64(24*time.Hour/time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if p, code, msg := auth(t, f, "Taken", "secret", 242, 0); p != nil || code != 3 || msg != "That name is not available. Please choose another." {
		t.Errorf("taken display name: pending=%v code=%d msg=%q", p, code, msg)
	}
	// The owning account itself may still log in under its own key.
	if p, code, _ := auth(t, f, "Other", "secret", 242, 0); p == nil || code != 0 {
		t.Errorf("owner login: pending=%v code=%d", p, code)
	}
}

func TestAuthenticateTokenLoginRotateAndRevoke(t *testing.T) {
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	save, _ := f.store.Load("Alice", persist.ProfileMain)
	entries, token, err := persist.IssueToken(nil, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.SetTokens(entries); err != nil {
		t.Fatal(err)
	}
	// Valid token: success, rotated token issued, old hash kept as previous.
	p, code, _ := auth(t, f, "Alice", token, 242, persist.LoginFlagToken)
	if p == nil || code != 0 {
		t.Fatalf("token login: pending=%v code=%d", p, code)
	}
	if p.LoginToken == "" || p.LoginToken == token {
		t.Errorf("rotated LoginToken = %q (presented %q)", p.LoginToken, token)
	}
	after, _ := f.store.Load("Alice", persist.ProfileMain)
	got := after.Tokens(f.now)
	if len(got) != 1 || got[0].PreviousHash != persist.HashToken(token) || got[0].Hash != persist.HashToken(p.LoginToken) {
		t.Errorf("rotated entries = %+v", got)
	}
	// Presenting the rotated-out token revokes the entry and fails.
	f.releaseAll()
	p2, code, _ := auth(t, f, "Alice", token, 242, persist.LoginFlagToken)
	if p2 != nil || code != 3 {
		t.Errorf("reused token: pending=%v code=%d, want 3", p2, code)
	}
	after, _ = f.store.Load("Alice", persist.ProfileMain)
	if got := after.Tokens(f.now); len(got) != 0 {
		t.Errorf("reused token did not revoke: %+v", got)
	}
	// The rotated token also stops working once revoked.
	f.releaseAll()
	p3, code, _ := auth(t, f, "Alice", p.LoginToken, 242, persist.LoginFlagToken)
	if p3 != nil || code != 3 {
		t.Errorf("revoked rotated token: pending=%v code=%d, want 3", p3, code)
	}
}

func TestAuthenticateTokenLoginFailures(t *testing.T) {
	// No save at all: token login fails even for a well-formed token.
	f := newLoginFixture(t)
	if p, code, _ := auth(t, f, "Ghost", "sometoken", 242, persist.LoginFlagToken); p != nil || code != 3 {
		t.Errorf("token login without save: pending=%v code=%d, want 3", p, code)
	}
	// Save exists, token unknown: fails, nothing persisted.
	f.seedAccount(t, "Alice", "secret")
	before, _ := f.store.Load("Alice", persist.ProfileMain)
	if p, code, _ := auth(t, f, "Alice", "unknown-token", 242, persist.LoginFlagToken); p != nil || code != 3 {
		t.Errorf("unknown token: pending=%v code=%d, want 3", p, code)
	}
	if got := before.Tokens(f.now); len(got) != 0 {
		t.Errorf("invalid token wrote entries: %+v", got)
	}
}

func TestAuthenticateRememberFlag(t *testing.T) {
	// With a save: token issued and persisted.
	f := newLoginFixture(t)
	f.seedAccount(t, "Alice", "secret")
	p, code, _ := auth(t, f, "Alice", "secret", 242, persist.LoginFlagRemember)
	if p == nil || code != 0 {
		t.Fatalf("remember: pending=%v code=%d", p, code)
	}
	if p.LoginToken == "" {
		t.Fatal("remember flag issued no token")
	}
	save, _ := f.store.Load("Alice", persist.ProfileMain)
	entries := save.Tokens(f.now)
	if len(entries) != 1 || entries[0].Hash != persist.HashToken(p.LoginToken) {
		t.Errorf("persisted entries = %+v", entries)
	}
	// New account: token issued, entries carried on the pending login
	// (no save exists to persist them to).
	f2 := newLoginFixture(t)
	p2, code, _ := auth(t, f2, "Fresh", "secret", 242, persist.LoginFlagRemember)
	if p2 == nil || code != 0 {
		t.Fatalf("remember new account: pending=%v code=%d", p2, code)
	}
	if p2.LoginToken == "" {
		t.Fatal("remember new account issued no token")
	}
	if len(p2.LoginTokens) != 1 || p2.LoginTokens[0].Hash != persist.HashToken(p2.LoginToken) {
		t.Errorf("carried entries = %+v", p2.LoginTokens)
	}
	// The issued token then works for a token login once a save holds it.
	f2.releaseAll()
	f2.store.put("Fresh", persist.ProfileMain, &persist.PlayerSave{DisplayName: "Fresh", PasswordHashWithSalt: p2.PasswordHash})
	freshSave, _ := f2.store.Load("Fresh", persist.ProfileMain)
	if err := freshSave.SetTokens(p2.LoginTokens); err != nil {
		t.Fatal(err)
	}
	p3, code, _ := auth(t, f2, "Fresh", p2.LoginToken, 242, persist.LoginFlagToken)
	if p3 == nil || code != 0 {
		t.Errorf("login with remembered token: pending=%v code=%d", p3, code)
	}
}

func TestAuthenticateCharacterSelectFlagRecorded(t *testing.T) {
	f := newLoginFixture(t)
	p, code, _ := auth(t, f, "Picker", "secret", 242, persist.LoginFlagCharacterSelect)
	if p == nil || code != 0 {
		t.Fatalf("character select flag: pending=%v code=%d", p, code)
	}
	if !p.CharacterSelect {
		t.Error("CharacterSelect not recorded")
	}
}

func TestAuthenticateUnmadeSave(t *testing.T) {
	f := newLoginFixture(t)
	if _, err := f.deps.Accounts.Ensure("Unmade", f.now.UnixMilli()); err != nil {
		t.Fatal(err)
	}
	hash, _ := persist.HashPassword("secret")
	save := &persist.PlayerSave{DisplayName: "Unmade", PasswordHashWithSalt: hash, Unmade: true}
	entries, _, err := persist.IssueToken(nil, f.now)
	if err != nil {
		t.Fatal(err)
	}
	if err := save.SetTokens(entries); err != nil {
		t.Fatal(err)
	}
	f.store.put("Unmade", persist.ProfileMain, save)
	p, code, _ := auth(t, f, "Unmade", "secret", 242, 0)
	if p == nil || code != 0 {
		t.Fatalf("unmade: pending=%v code=%d", p, code)
	}
	if p.Save != nil {
		t.Error("unmade save should enter as a new character (Save nil)")
	}
	if len(p.LoginTokens) != 1 {
		t.Errorf("unmade save tokens not carried: %+v", p.LoginTokens)
	}
}

func TestAuthenticateBusyOnErrors(t *testing.T) {
	// Save load failure -> 8, reservation released.
	f := newLoginFixture(t)
	f.store.loadErr = errors.New("disk on fire")
	if p, code, msg := auth(t, f, "Alice", "secret", 242, 0); p != nil || code != 8 || msg != "The login server is busy." {
		t.Errorf("load error: pending=%v code=%d msg=%q", p, code, msg)
	}
	if len(f.resv.held) != 0 {
		t.Errorf("reservation leaked after load error: %v", f.resv.held)
	}
	// Nil accounts -> 8.
	f2 := newLoginFixture(t)
	f2.deps.Accounts = nil
	if p, code, _ := auth(t, f2, "Alice", "secret", 242, 0); p != nil || code != 8 {
		t.Errorf("nil accounts: pending=%v code=%d, want 8", p, code)
	}
	// Nil deps -> 8.
	if p, code, _ := Authenticate(nil, "Alice", "secret", 242, 0, ""); p != nil || code != 8 {
		t.Errorf("nil deps: pending=%v code=%d, want 8", p, code)
	}
}

func TestAuthenticateNilOptionals(t *testing.T) {
	// No punishment registry, no display names, no store, no
	// reservation/online/world callbacks: a new account still logs in.
	deps := &LoginDeps{ActiveRevision: 242}
	accounts, err := persist.LoadAccounts(filepath.Join(t.TempDir(), "a.json"))
	if err != nil {
		t.Fatal(err)
	}
	deps.Accounts = accounts
	p, code, _ := Authenticate(deps, "Solo", "pw", 242, 0, "")
	if p == nil || code != 0 {
		t.Fatalf("minimal deps: pending=%v code=%d", p, code)
	}
}
