package persist

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKey(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Zezima", "zezima"},
		{"Test_Player", "test player"},
		{"  Spaced   Out  Name ", "spaced out name"},
		{"a__b", "a b"},
		{"", ""},
		{"UPPER_lower", "upper lower"},
	}
	for _, c := range cases {
		if got := Key(c.in); got != c.want {
			t.Fatalf("Key(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPlayerName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"zezima", "Zezima"},
		{"test player", "Test Player"},
		// Only the FIRST underscore becomes a space (JS replace), but
		// the letter after EVERY underscore is capitalized.
		{"test_player_one", "Test Player_One"},
		{"a-b", "A-B"},
		{"", ""},
		{"x", "X"},
	}
	for _, c := range cases {
		if got := FormatPlayerName(c.in); got != c.want {
			t.Fatalf("FormatPlayerName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatDisplayName(t *testing.T) {
	if got := FormatDisplayName("  Test_Player "); got != "Test Player" {
		t.Fatalf("FormatDisplayName = %q", got)
	}
}

func TestNormalizeUsername(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Zezima", "zezima"},
		{"Test Player", "test_player"},
		{"Test_Player", "test_player"},
		{"  Weird--Name!! ", "weird_name"},
		{"", "player"},
		{"!!!", "player"},
		{"a", "a"},
	}
	for _, c := range cases {
		if got := NormalizeUsername(c.in); got != c.want {
			t.Fatalf("NormalizeUsername(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

const accountsFixture = `{
  "accounts": {
    "zezima": {
      "createdAt": 1700000000000,
      "characters": [
        {"name": "Zezima", "createdAt": 1700000000000, "lastPlayedAt": 1700000100000},
        {"name": "Zezima Alt", "createdAt": 1700000200000, "lastPlayedAt": 0}
      ]
    }
  },
  "deleted": {
    "old name": {"name": "Old Name", "account": "zezima", "deletedAt": 1700000300000},
    "someone else": {"name": "Someone Else", "account": "other", "deletedAt": 1700000400000},
    "nobody": {"name": "Nobody", "account": "", "deletedAt": 1700000500000}
  }
}`

func writeAccountsFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "accounts.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	return path
}

func TestAccountsLoadAndLookup(t *testing.T) {
	a, err := LoadAccounts(writeAccountsFixture(t, accountsFixture))
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	for _, name := range []string{"Zezima", "zezima", "ZEZIMA", "Zezima Alt", "zezima_alt", "ZEZIMA  ALT"} {
		key, ok := a.AccountOf(name)
		if !ok || key != "zezima" {
			t.Fatalf("AccountOf(%q) = (%q, %v), want (zezima, true)", name, key, ok)
		}
	}
	if _, ok := a.AccountOf("Nobody Here"); ok {
		t.Fatal("unknown name resolved to an account")
	}
	if !a.IsDeleted("Old Name") || !a.IsDeleted("old_name") {
		t.Fatal("tombstoned name not reported deleted")
	}
	if a.IsDeleted("Zezima") {
		t.Fatal("live name reported deleted")
	}
	if !a.DeletedBy("Old Name", "zezima") {
		t.Fatal("DeletedBy must be true for the deleting account")
	}
	if a.DeletedBy("Old Name", "other") || a.DeletedBy("Nobody", "zezima") {
		t.Fatal("DeletedBy must be false for other accounts / nobody's tombstone")
	}
	if got := a.DeletedCountOf("zezima"); got != 1 {
		t.Fatalf("DeletedCountOf = %d, want 1", got)
	}
	if !a.CanDelete("zezima") {
		t.Fatal("account with 1 tombstone must be able to delete")
	}
	primary, ok := a.PrimaryOf("zezima")
	if !ok || primary != "Zezima" {
		t.Fatalf("PrimaryOf = (%q, %v)", primary, ok)
	}
	if !a.IsPrimary("zezima", "zezima") || a.IsPrimary("zezima", "Zezima Alt") {
		t.Fatal("IsPrimary wrong")
	}
	if len(a.CharactersOf("zezima")) != 2 {
		t.Fatal("CharactersOf wrong")
	}
}

func TestAccountsMissingFileIsEmpty(t *testing.T) {
	a, err := LoadAccounts(filepath.Join(t.TempDir(), "nope", "accounts.json"))
	if err != nil {
		t.Fatalf("missing file must load empty: %v", err)
	}
	if _, ok := a.AccountOf("Anyone"); ok {
		t.Fatal("empty registry resolved a name")
	}
}

func TestAccountsMalformed(t *testing.T) {
	for _, content := range []string{
		`{not json`,
		`[1,2,3]`,
		`{"accounts": {}, "deleted": []}`,
		`{"accounts": {}, "deleted": null}`,
		`{"accounts": {}, "deleted": "junk"}`,
	} {
		if _, err := LoadAccounts(writeAccountsFixture(t, content)); err == nil {
			t.Fatalf("content %q must fail to load", content)
		}
	}
	// A file from before tombstones (no "deleted") loads fine.
	a, err := LoadAccounts(writeAccountsFixture(t, `{"accounts": {}}`))
	if err != nil {
		t.Fatalf("pre-tombstone file must load: %v", err)
	}
	if a.IsDeleted("anything") {
		t.Fatal("no tombstones expected")
	}
}

func TestAccountsEnsureAddRemoveAndSave(t *testing.T) {
	path := writeAccountsFixture(t, accountsFixture)
	a, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	// Ensure an existing account returns it without writing.
	if _, err := a.Ensure("Zezima", 1700000600000); err != nil {
		t.Fatalf("Ensure existing: %v", err)
	}
	// Ensure a new account creates + persists it.
	if _, err := a.Ensure("New Player", 1700000600000); err != nil {
		t.Fatalf("Ensure new: %v", err)
	}
	// Reclaiming a name this account deleted removes the tombstone.
	if _, err := a.AddCharacter("zezima", "Old Name", 1700000700000); err != nil {
		t.Fatalf("AddCharacter reclaim: %v", err)
	}
	if a.IsDeleted("Old Name") {
		t.Fatal("reclaimed name still tombstoned")
	}
	// Another account's deleted name is refused.
	if _, err := a.AddCharacter("zezima", "Someone Else", 1700000700000); err == nil {
		t.Fatal("taking another account's deleted name must fail")
	}
	// Owned names are refused.
	if _, err := a.AddCharacter("new player", "Zezima", 1700000700000); err == nil {
		t.Fatal("taking an owned name must fail")
	}
	// Removing a non-primary creates a tombstone; removing the primary fails.
	removed, err := a.RemoveCharacter("zezima", "Zezima Alt", 1700000800000)
	if err != nil || !removed {
		t.Fatalf("RemoveCharacter = (%v, %v)", removed, err)
	}
	if !a.IsDeleted("Zezima Alt") {
		t.Fatal("removed character must be tombstoned")
	}
	removed, err = a.RemoveCharacter("zezima", "Zezima", 1700000800000)
	if err != nil || removed {
		t.Fatalf("removing the primary must return false: (%v, %v)", removed, err)
	}
	// ReleaseDeleted frees the name.
	tomb, ok, err := a.ReleaseDeleted("Zezima Alt")
	if err != nil || !ok || tomb.Name != "Zezima Alt" {
		t.Fatalf("ReleaseDeleted = (%+v, %v, %v)", tomb, ok, err)
	}
	if a.IsDeleted("Zezima Alt") {
		t.Fatal("released name still tombstoned")
	}
	// Reload from disk: everything persisted.
	b, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, ok := b.AccountOf("New Player"); !ok {
		t.Fatal("new account not persisted")
	}
	if _, ok := b.AccountOf("Old Name"); !ok {
		t.Fatal("reclaimed character not persisted")
	}
	if b.IsDeleted("Old Name") || b.IsDeleted("Zezima Alt") {
		t.Fatal("tombstones wrong after reload")
	}
	// The file has both top-level objects and no temp file remains.
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"accounts"`) || !strings.Contains(string(data), `"deleted"`) {
		t.Fatalf("saved file missing top-level objects: %s", data)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temp file left behind after save")
	}
}

func TestAccountsTombstoneCap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	a, err := LoadAccounts(path)
	if err != nil {
		t.Fatalf("LoadAccounts: %v", err)
	}
	if _, err := a.Ensure("Cap Test", 1000); err != nil {
		t.Fatalf("Ensure: %v", err)
	}
	for i := 0; i < MaxTombstonesPerAccount+1; i++ {
		name := "Cap Alt " + string(rune('A'+i))
		if _, err := a.AddCharacter("cap test", name, 1000); err != nil {
			t.Fatalf("AddCharacter %s: %v", name, err)
		}
	}
	for i := 0; i < MaxTombstonesPerAccount; i++ {
		name := "Cap Alt " + string(rune('A'+i))
		removed, err := a.RemoveCharacter("cap test", name, 2000)
		if err != nil || !removed {
			t.Fatalf("RemoveCharacter %s = (%v, %v)", name, removed, err)
		}
	}
	if a.CanDelete("cap test") {
		t.Fatal("account at the tombstone cap must not be able to delete")
	}
	if _, err := a.RemoveCharacter("cap test", "Cap Alt G", 2000); err == nil {
		t.Fatal("deleting beyond the cap must fail")
	}
	if _, ok := a.AccountOf("Cap Alt G"); !ok {
		t.Fatal("failed delete must leave the character owned")
	}
}

func TestAccountsTouchLastPlayedAndFlush(t *testing.T) {
	path := writeAccountsFixture(t, accountsFixture)
	a, _ := LoadAccounts(path)
	a.TouchLastPlayed("zezima", "Zezima Alt", 1799999999999)
	if !a.Dirty() {
		t.Fatal("TouchLastPlayed must mark dirty")
	}
	// Not yet on disk.
	before, _ := os.ReadFile(path)
	if strings.Contains(string(before), "1799999999999") {
		t.Fatal("last-played time must not be written until flush")
	}
	if err := a.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if a.Dirty() {
		t.Fatal("Flush must clear dirty")
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "1799999999999") {
		t.Fatal("last-played time not written by flush")
	}
	// Unknown names are ignored.
	a.TouchLastPlayed("zezima", "Ghost", 123)
	if a.Dirty() {
		t.Fatal("unknown name must not mark dirty")
	}
}

func TestAccountsNameAvailable(t *testing.T) {
	a, _ := LoadAccounts(writeAccountsFixture(t, accountsFixture))
	// Owned by another account.
	if a.NameAvailable("Zezima", "other") {
		t.Fatal("owned name available to another account")
	}
	// Owned by self: available.
	if !a.NameAvailable("Zezima", "zezima") {
		t.Fatal("own name unavailable to self")
	}
	// Deleted by another / nobody: unavailable.
	if a.NameAvailable("Someone Else", "zezima") || a.NameAvailable("Nobody", "zezima") {
		t.Fatal("another's tombstone available")
	}
	// Deleted by self: available again.
	if !a.NameAvailable("Old Name", "zezima") {
		t.Fatal("own tombstone unavailable to self")
	}
	// Fresh name: available.
	if !a.NameAvailable("Brand New", "zezima") {
		t.Fatal("fresh name unavailable")
	}
}

func TestAccountsEnsureDeletedNameRefused(t *testing.T) {
	a, _ := LoadAccounts(writeAccountsFixture(t, accountsFixture))
	if _, err := a.Ensure("Old Name", 1000); err == nil {
		t.Fatal("Ensure must refuse a deleted name")
	}
}
