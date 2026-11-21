package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Display names: port of game/entity/impl/player/DisplayNames.ts.
//
// What other players see, apart from the login name that keys the
// account. An account without an entry shows its login name. Every
// account's login name is reserved for it, and an account's current and
// held names are reserved for it, so a name always leads to one account.
// File shape (display-names.json):
//
//	{"accounts": {<accountKey>: {"name", "changedAt", "held": [{"name", "until"}...]}}}
//
// The file is written whole on every change. Unlike TS (static state),
// DisplayNames is an instance type; all methods are safe for concurrent
// use.

// HeldDisplayName is a name an account gave up, kept for it until Until
// so nobody else can take it straight away (DisplayNames.ts:9-12).
type HeldDisplayName struct {
	Name string `json:"name"`
	// Until is epoch milliseconds.
	Until int64 `json:"until"`
}

// DisplayNameEntry is one account's display-name record
// (DisplayNames.ts:14-21).
type DisplayNameEntry struct {
	// Name other players see, capitals as the player typed them.
	Name string `json:"name"`
	// ChangedAt is epoch milliseconds.
	ChangedAt int64             `json:"changedAt"`
	Held      []HeldDisplayName `json:"held"`
}

// DisplayNames is the loaded display-names.json registry.
type DisplayNames struct {
	mu      sync.Mutex
	path    string
	entries map[string]*DisplayNameEntry
	// owners indexes current and held names (name key -> owner + hold
	// expiry; current names have expiry +Inf). Held ones may have
	// expired; callers check Until against now.
	owners map[string]displayNameOwner
}

type displayNameOwner struct {
	account string
	until   int64 // math.MaxInt64 for a name currently shown
}

type displayNamesFile struct {
	Accounts map[string]*DisplayNameEntry `json:"accounts"`
}

// LoadDisplayNames reads the registry at path. A missing file loads as
// an empty registry, as in TS (DisplayNames.ts:38-43: ENOENT yields
// {accounts: {}}). Any other read or parse error is returned: TS
// rethrows non-ENOENT errors, and a file that can't be read keeps
// throwing so save() can't replace it.
func LoadDisplayNames(path string) (*DisplayNames, error) {
	d := &DisplayNames{
		path:    path,
		entries: map[string]*DisplayNameEntry{},
		owners:  map[string]displayNameOwner{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return d, nil
		}
		return nil, fmt.Errorf("[display-names] %s could not be read: %w", path, err)
	}
	var file displayNamesFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("[display-names] %s could not be read: %w", path, err)
	}
	for k, v := range file.Accounts {
		if v == nil {
			v = &DisplayNameEntry{}
		}
		if v.Held == nil {
			v.Held = []HeldDisplayName{}
		}
		d.entries[k] = v
	}
	d.reindex()
	return d, nil
}

// Path returns the file this registry loads from and saves to.
func (d *DisplayNames) Path() string { return d.path }

func (d *DisplayNames) reindex() {
	d.owners = map[string]displayNameOwner{}
	// Two accounts can both list a held name once one hold has run out:
	// the later hold wins, and a name someone shows now beats any hold
	// (DisplayNames.ts:56-65).
	for account, entry := range d.entries {
		for _, held := range entry.Held {
			key := Key(held.Name)
			if cur, ok := d.owners[key]; !ok || cur.until < held.Until {
				d.owners[key] = displayNameOwner{account: account, until: held.Until}
			}
		}
	}
	for account, entry := range d.entries {
		d.owners[Key(entry.Name)] = displayNameOwner{account: account, until: math.MaxInt64}
	}
}

// saveLocked writes the whole file: temp file + rename, as in TS
// (DisplayNames.ts:67-72). No fsync, matching TS.
func (d *DisplayNames) saveLocked() error {
	target := d.path
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("[display-names] writing %s: %w", target, err)
	}
	accounts := d.entries
	if accounts == nil {
		accounts = map[string]*DisplayNameEntry{}
	}
	data, err := json.MarshalIndent(displayNamesFile{Accounts: accounts}, "", "  ")
	if err != nil {
		return fmt.Errorf("[display-names] writing %s: %w", target, err)
	}
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return fmt.Errorf("[display-names] writing %s: %w", target, err)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("[display-names] writing %s: %w", target, err)
	}
	return nil
}

// EntryOf returns a copy of the account's display-name entry, if any
// (DisplayNames.ts:74-76).
func (d *DisplayNames) EntryOf(username string) (DisplayNameEntry, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.entries[Key(username)]
	if !ok {
		return DisplayNameEntry{}, false
	}
	cp := *entry
	cp.Held = append([]HeldDisplayName(nil), entry.Held...)
	return cp, true
}

// NameOf returns the name other players see for this account: its entry,
// or the formatted login name when it has none (DisplayNames.ts:79-81).
func (d *DisplayNames) NameOf(username string) string {
	if entry, ok := d.EntryOf(username); ok {
		return entry.Name
	}
	return FormatDisplayName(username)
}

// PreviousNameOf returns the account's most recent name still held for
// it, for friends lists ("" when none) (DisplayNames.ts:84-87).
func (d *DisplayNames) PreviousNameOf(username string, now time.Time) string {
	entry, ok := d.EntryOf(username)
	if !ok {
		return ""
	}
	nowMs := now.UnixMilli()
	held := ""
	for _, h := range entry.Held {
		if h.Until > nowMs {
			held = h.Name
		}
	}
	return held
}

// OwnerOf returns the account key whose current or held display name
// this is, or ("", false). Login names aren't included; expired holds
// don't count (DisplayNames.ts:90-94).
func (d *DisplayNames) OwnerOf(name string, now time.Time) (string, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	owner, ok := d.owners[Key(name)]
	if !ok || owner.until <= now.UnixMilli() {
		return "", false
	}
	return owner.account, true
}

// Resolve returns the account a typed name means: the one showing it (or
// holding it), otherwise the account logged in with it. It returns
// ("", false) for a login name whose account shows another name and no
// longer holds it, which other players shouldn't be able to look up by
// (DisplayNames.ts:102-110).
func (d *DisplayNames) Resolve(name string, now time.Time) (string, bool) {
	key := Key(name)
	if key == "" {
		return "", false
	}
	if owner, ok := d.OwnerOf(key, now); ok {
		return owner, true
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	entry, ok := d.entries[key]
	if ok && Key(entry.Name) != key {
		return "", false
	}
	return key, true
}

// Refusal returns why username can't take name (a sentence for the
// player), or "" when it can. Port of DisplayNames.refusal
// (DisplayNames.ts:116-128): another account's login name, current name
// or held name is taken, whether it is online or not.
//
// loginNameInUse answers whether any account logs in with the given
// (already normalized) key; pass LoginNameInUse or a session-layer
// equivalent. A nil function is treated as "no login name in use".
//
// NOT PORTED: shownByAnother (DisplayNames.ts:134-145) — whether a
// player online or logging in now shows this name for another account.
// That needs the live World and pending-login state, which this package
// does not have. The session layer must check live players separately
// before accepting a name (login() does: NetworkBuilder.ts check 8).
func (d *DisplayNames) Refusal(username, name string, now time.Time, loginNameInUse func(key string) bool) string {
	account := Key(username)
	key := Key(name)
	if key == account {
		return ""
	}
	if owner, ok := d.OwnerOf(key, now); ok {
		if owner != account {
			return "That name is not available."
		}
		return ""
	}
	if loginNameInUse != nil && loginNameInUse(key) {
		return "That name is not available."
	}
	return ""
}

// NameAvailable is Refusal == "" without the shownByAnother live-player
// check (see Refusal). accountKey is the account taking the name.
func (d *DisplayNames) NameAvailable(name, accountKey string, now time.Time, loginNameInUse func(key string) bool) bool {
	return d.Refusal(accountKey, name, now, loginNameInUse) == ""
}

// Set gives the account name. The name it had is held for it for holdMs,
// and a held name it takes back stops being held (DisplayNames.ts:153-173).
// now is the current time; holdMs is the hold duration in milliseconds.
// On a write failure nothing changes and the error is returned.
func (d *DisplayNames) Set(username, name string, now time.Time, holdMs int64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	account := Key(username)
	nowMs := now.UnixMilli()
	before, hadBefore := d.entries[account]
	var current string
	held := []HeldDisplayName{}
	if hadBefore {
		current = before.Name
		for _, h := range before.Held {
			if h.Until > nowMs && Key(h.Name) != Key(name) {
				held = append(held, h)
			}
		}
	} else {
		current = FormatDisplayName(username)
	}
	// The login name is held too: friends lists show it as the previous
	// name, and others can still find the account by it until the hold
	// runs out, as in OSRS (DisplayNames.ts:163-164).
	if Key(current) != Key(name) {
		held = append(held, HeldDisplayName{Name: current, Until: nowMs + holdMs})
	}
	d.entries[account] = &DisplayNameEntry{Name: name, ChangedAt: nowMs, Held: held}
	if err := d.saveLocked(); err != nil {
		if hadBefore {
			d.entries[account] = before
		} else {
			delete(d.entries, account)
		}
		return err
	}
	d.reindex()
	return nil
}

// LoginNameInUse reports whether any account logs in with this (already
// normalized) key, from persisted state only: an account's character, a
// deleted character's name (reserved, unless deleted by
// exceptDeletedBy), or a save existing on any profile. Port of the
// persisted part of DisplayNames.loginNameInUse (DisplayNames.ts:151-164).
//
// NOT PORTED: the live checks in TS — World.getPlayerByName and
// ClientConnection.isLoginPending (a player online or logging in now).
// The caller must add those at the session layer. saveExists may be nil
// (no save-existence check); exceptDeletedBy is an account key, or ""
// for no exception (TS: an undefined exceptDeletedBy means no
// exception; an empty-string key never equals a tombstone's non-empty
// account, and tombstones with account "" are nobody's, so "" is a safe
// no-exception value here).
func LoginNameInUse(accounts *Accounts, key, exceptDeletedBy string, saveExists func(name string, profile Profile) bool) bool {
	if accounts != nil {
		if _, ok := accounts.AccountOf(key); ok {
			return true
		}
		if accounts.IsDeleted(key) {
			if exceptDeletedBy == "" || !accounts.DeletedBy(key, exceptDeletedBy) {
				return true
			}
		}
	}
	if saveExists != nil {
		for _, profile := range Profiles {
			if saveExists(key, profile) {
				return true
			}
		}
	}
	return false
}
