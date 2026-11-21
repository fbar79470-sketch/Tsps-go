package persist

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePunishmentFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return dir
}

func TestPunishmentParsing(t *testing.T) {
	dir := writePunishmentFixture(t, map[string]string{
		"Bans.txt":    "Zezima\nBad Actor\n",
		"IPBans.txt":  "203.0.113.7\n",
		"Mutes.txt":   "Chatty\n",
		"IPMutes.txt": "198.51.100.9\n198.51.100.10",
	})
	p, err := LoadPunishment(dir)
	if err != nil {
		t.Fatalf("LoadPunishment: %v", err)
	}
	// Name lookups are case/underscore-insensitive via FormatPlayerName.
	for _, name := range []string{"zezima", "ZEZIMA", "Zezima", "bad actor", "Bad_Actor"} {
		if !p.Banned(name) {
			t.Fatalf("Banned(%q) = false", name)
		}
	}
	if p.Banned("Good Player") {
		t.Fatal("unbanned player reported banned")
	}
	if !p.IPBanned("203.0.113.7") || p.IPBanned("203.0.113.8") {
		t.Fatal("IPBanned wrong")
	}
	if p.IPBanned("") || p.IPBanned("   ") {
		t.Fatal("blank IP must never be banned")
	}
	// IP match is verbatim: surrounding spaces don't match.
	if p.IPBanned(" 203.0.113.7") {
		t.Fatal("IP with spaces must not match verbatim stored entry")
	}
	if !p.Muted("chatty") || p.Muted("quiet") {
		t.Fatal("Muted wrong")
	}
	if !p.IPMuted("198.51.100.9") || !p.IPMuted("198.51.100.10") || p.IPMuted("198.51.100.11") {
		t.Fatal("IPMuted wrong (last line without trailing newline must count)")
	}
	if err := p.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}
}

func TestPunishmentMissingFilesEmpty(t *testing.T) {
	dir := t.TempDir() // no files at all
	p, err := LoadPunishment(dir)
	if err != nil {
		t.Fatalf("LoadPunishment: %v", err)
	}
	if p.Banned("Anyone") || p.IPBanned("1.2.3.4") || p.Muted("Anyone") || p.IPMuted("1.2.3.4") {
		t.Fatal("empty punishment lists must match nothing")
	}
	// A missing saves directory is created.
	nested := filepath.Join(t.TempDir(), "deep", "saves")
	if _, err := LoadPunishment(nested); err != nil {
		t.Fatalf("LoadPunishment nested: %v", err)
	}
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("saves dir not created: %v", err)
	}
}

func TestPunishmentMutationsRoundTrip(t *testing.T) {
	dir := writePunishmentFixture(t, map[string]string{})
	p, _ := LoadPunishment(dir)
	if err := p.Ban("New Offender"); err != nil {
		t.Fatalf("Ban: %v", err)
	}
	if err := p.Ban("new offender"); err != nil { // duplicate: no second line
		t.Fatalf("Ban duplicate: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "Bans.txt"))
	if string(data) != "New Offender\n" {
		t.Fatalf("Bans.txt = %q, want %q", data, "New Offender\n")
	}
	if err := p.Mute("Loud Mouth"); err != nil {
		t.Fatalf("Mute: %v", err)
	}
	if err := p.AddBannedIP("203.0.113.99"); err != nil {
		t.Fatalf("AddBannedIP: %v", err)
	}
	if err := p.AddMutedIP("198.51.100.99"); err != nil {
		t.Fatalf("AddMutedIP: %v", err)
	}
	// Reload from disk: mutations persisted.
	p2, _ := LoadPunishment(dir)
	if !p2.Banned("new offender") || !p2.Muted("loud mouth") || !p2.IPBanned("203.0.113.99") || !p2.IPMuted("198.51.100.99") {
		t.Fatal("mutations not persisted")
	}
	// Unban/unmute remove the lines.
	if err := p2.Unban("NEW OFFENDER"); err != nil {
		t.Fatalf("Unban: %v", err)
	}
	if err := p2.Unmute("loud mouth"); err != nil {
		t.Fatalf("Unmute: %v", err)
	}
	if err := p2.RemoveBannedIP("203.0.113.99"); err != nil {
		t.Fatalf("RemoveBannedIP: %v", err)
	}
	if p2.Banned("new offender") || p2.Muted("loud mouth") || p2.IPBanned("203.0.113.99") {
		t.Fatal("removals did not take effect in memory")
	}
	p3, _ := LoadPunishment(dir)
	if p3.Banned("new offender") || p3.Muted("loud mouth") || p3.IPBanned("203.0.113.99") {
		t.Fatal("removals not persisted")
	}
	// Toggle behavior.
	banned, err := p3.ToggleBan("Toggle Me")
	if err != nil || !banned {
		t.Fatalf("ToggleBan first = (%v, %v)", banned, err)
	}
	banned, err = p3.ToggleBan("Toggle Me")
	if err != nil || banned {
		t.Fatalf("ToggleBan second = (%v, %v)", banned, err)
	}
}

func TestPunishmentReload(t *testing.T) {
	dir := writePunishmentFixture(t, map[string]string{"IPBans.txt": "203.0.113.1\n"})
	p, _ := LoadPunishment(dir)
	if err := os.WriteFile(filepath.Join(dir, "IPBans.txt"), []byte("203.0.113.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := p.ReloadIPBans(); err != nil {
		t.Fatalf("ReloadIPBans: %v", err)
	}
	if p.IPBanned("203.0.113.1") || !p.IPBanned("203.0.113.2") {
		t.Fatal("ReloadIPBans did not pick up the new file")
	}
}

const displayNamesFixture = `{
  "accounts": {
    "zezima": {"name": "Zezima", "changedAt": 1700000000000, "held": [{"name": "Old Zez", "until": 1800000000000}]},
    "holder": {"name": "Holder", "changedAt": 1700000000000, "held": [{"name": "Stale Hold", "until": 1000000000000}]}
  }
}`

func TestDisplayNames(t *testing.T) {
	now := time.Date(2026, 11, 13, 15, 0, 0, 0, time.UTC)
	path := filepath.Join(t.TempDir(), "display-names.json")
	if err := os.WriteFile(path, []byte(displayNamesFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := LoadDisplayNames(path)
	if err != nil {
		t.Fatalf("LoadDisplayNames: %v", err)
	}
	if got := d.NameOf("zezima"); got != "Zezima" {
		t.Fatalf("NameOf = %q", got)
	}
	if got := d.NameOf("no_entry_here"); got != "no entry here" {
		t.Fatalf("NameOf fallback = %q", got)
	}
	owner, ok := d.OwnerOf("old zez", now)
	if !ok || owner != "zezima" {
		t.Fatalf("OwnerOf held name = (%q, %v)", owner, ok)
	}
	// Expired hold (until 1000000000000 < now) does not count.
	if _, ok := d.OwnerOf("stale hold", now); ok {
		t.Fatal("expired hold still owns the name")
	}
	if got := d.PreviousNameOf("zezima", now); got != "Old Zez" {
		t.Fatalf("PreviousNameOf = %q", got)
	}
	// Resolve: shown name -> owner; unknown name -> itself (login key).
	if acct, ok := d.Resolve("Zezima", now); !ok || acct != "zezima" {
		t.Fatalf("Resolve shown = (%q, %v)", acct, ok)
	}
	if acct, ok := d.Resolve("Somebody New", now); !ok || acct != "somebody new" {
		t.Fatalf("Resolve unknown = (%q, %v)", acct, ok)
	}
	// Refusal: another account's held name is taken; own is fine.
	if d.Refusal("other", "Old Zez", now, nil) == "" {
		t.Fatal("another account's held name must be refused")
	}
	if d.Refusal("zezima", "Old Zez", now, nil) != "" {
		t.Fatal("own held name must be allowed")
	}
	if d.Refusal("zezima", "Zezima", now, nil) != "" {
		t.Fatal("own login name must be allowed")
	}
	loginInUse := func(key string) bool { return key == "taken login" }
	if d.Refusal("other", "Taken Login", now, loginInUse) == "" {
		t.Fatal("login name in use must be refused")
	}
	if d.Refusal("other", "Free Name", now, loginInUse) != "" {
		t.Fatal("free name must be allowed")
	}
	// Set: renames, holds the old name, persists.
	if err := d.Set("zezima", "Zezima II", now, 7*24*60*60*1000); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := d.NameOf("zezima"); got != "Zezima II" {
		t.Fatalf("after Set, NameOf = %q", got)
	}
	if got := d.PreviousNameOf("zezima", now); got != "Zezima" {
		t.Fatalf("after Set, PreviousNameOf = %q (old name must be held)", got)
	}
	d2, err := LoadDisplayNames(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := d2.NameOf("zezima"); got != "Zezima II" {
		t.Fatal("Set did not persist")
	}
}

func TestDisplayNamesMissingFileEmpty(t *testing.T) {
	d, err := LoadDisplayNames(filepath.Join(t.TempDir(), "display-names.json"))
	if err != nil {
		t.Fatalf("missing file must load empty: %v", err)
	}
	if d.NameOf("Anyone") != "Anyone" {
		t.Fatal("empty registry NameOf fallback wrong")
	}
}

func TestLoginNameInUse(t *testing.T) {
	a, _ := LoadAccounts(writeAccountsFixture(t, accountsFixture))
	if !LoginNameInUse(a, "zezima", "", nil) {
		t.Fatal("owned name must be in use")
	}
	if !LoginNameInUse(a, "old name", "", nil) {
		t.Fatal("tombstoned name must be in use")
	}
	if LoginNameInUse(a, "old name", "zezima", nil) {
		t.Fatal("tombstone deleted by the excepted account must not count")
	}
	if LoginNameInUse(a, "fresh", "", nil) {
		t.Fatal("fresh name must not be in use")
	}
	saves := func(name string, profile Profile) bool { return name == "saved" && profile == ProfileLeague }
	if !LoginNameInUse(a, "saved", "", saves) {
		t.Fatal("name with a save on any profile must be in use")
	}
}
