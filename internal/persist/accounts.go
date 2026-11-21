package persist

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Accounts registry: port of game/entity/impl/player/Accounts.ts.
//
// One login name owns several character names. The account is keyed by
// its primary (login) name; every character name leads to one account.
// File shape (accounts.json):
//
//	{"accounts": {<key>: {"createdAt": N, "characters": [{"name", "createdAt", "lastPlayedAt"}...]}},
//	 "deleted": {<key>: {"name", "account", "deletedAt"}}}
//
// The file is written whole on every structural change. Last-played
// times change on every world entry, so TS keeps them in memory and
// writes them at most once per 30s, by the next structural write, or by
// flush() at shutdown. This port keeps the same in-memory/dirty
// distinction: TouchLastPlayed only marks dirty; Save writes; Flush
// writes only when dirty. There is NO background 30s timer in this
// package (TS scheduleWrite); the caller must call Flush periodically or
// at shutdown. That is a deliberate divergence: a hidden timer in a
// library package would outlive its owner.
//
// Unlike TS (static state), Accounts is an instance type: load it once
// with LoadAccounts and share the pointer. All methods are safe for
// concurrent use.

// MaxTombstonesPerAccount is the most deleted names one account may hold
// reserved; a delete beyond it is refused (Accounts.ts:47).
const MaxTombstonesPerAccount = 6

// LastPlayedWriteDelay is the longest a last-played time waits in memory
// before TS writes it (Accounts.ts:49, LAST_PLAYED_WRITE_DELAY_MS). This
// package does not run the timer; the constant documents the interval a
// caller flushing periodically should use.
const LastPlayedWriteDelayMs = 30_000

// AccountCharacter is one character owned by an account
// (Accounts.ts:7-13).
type AccountCharacter struct {
	// Name as the player typed it.
	Name string `json:"name"`
	// CreatedAt is epoch milliseconds.
	CreatedAt int64 `json:"createdAt"`
	// LastPlayedAt is the last login in epoch milliseconds, 0 for never.
	LastPlayedAt int64 `json:"lastPlayedAt"`
}

// AccountEntry is one account: createdAt plus its characters, the first
// being the primary (Accounts.ts:15-19).
type AccountEntry struct {
	CreatedAt  int64              `json:"createdAt"`
	Characters []AccountCharacter `json:"characters"`
}

// DeletedName is a deleted character's name: claimable only by the
// account that deleted it (Accounts.ts:26-33).
type DeletedName struct {
	// Name as the account had it.
	Name string `json:"name"`
	// Account that deleted it ("" for a tombstone written before this
	// was recorded: nobody's).
	Account string `json:"account"`
	// DeletedAt is epoch milliseconds.
	DeletedAt int64 `json:"deletedAt"`
}

// Accounts is the loaded accounts.json registry.
type Accounts struct {
	mu      sync.Mutex
	path    string
	entries map[string]*AccountEntry
	owners  map[string]string // character name key -> account key
	deleted map[string]DeletedName
	dirty   bool // last-played times changed since the file was written
}

// accountsFile is the on-disk shape of accounts.json.
type accountsFile struct {
	Accounts map[string]*AccountEntry `json:"accounts"`
	Deleted  map[string]DeletedName   `json:"deleted"`
}

// LoadAccounts reads the registry at path. A missing file loads as an
// empty registry, as in TS (Accounts.ts:66-70: ENOENT yields
// {accounts: {}}). A file that exists but cannot be parsed, is not a JSON
// object, or has a "deleted" value that is present but not an object is
// an error: TS refuses to load such a file so save() cannot replace it
// (Accounts.ts:73, 84-87), and every login is refused meanwhile.
//
// JSON key order in the written file follows Go's encoding/json (map
// keys sorted); TS writes Map insertion order. The content is identical;
// only key order differs.
func LoadAccounts(path string) (*Accounts, error) {
	a := &Accounts{
		path:    path,
		entries: map[string]*AccountEntry{},
		owners:  map[string]string{},
		deleted: map[string]DeletedName{},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return a, nil
		}
		return nil, unreadableAccounts(path, err)
	}
	// Decode via a raw map first so a present-but-not-object "deleted"
	// can be rejected exactly as TS rejects it (Accounts.ts:84-87),
	// including an explicit null.
	var top map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&top); err != nil {
		return nil, unreadableAccounts(path, err)
	}
	if top == nil {
		return nil, unreadableAccounts(path, errors.New("not a JSON object"))
	}
	if raw, ok := top["accounts"]; ok {
		var accounts map[string]*AccountEntry
		if err := json.Unmarshal(raw, &accounts); err != nil {
			return nil, unreadableAccounts(path, err)
		}
		for k, v := range accounts {
			if v == nil {
				v = &AccountEntry{}
			}
			if v.Characters == nil {
				v.Characters = []AccountCharacter{}
			}
			a.entries[k] = v
		}
	}
	if raw, ok := top["deleted"]; ok {
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || trimmed[0] != '{' {
			return nil, unreadableAccounts(path, errors.New(`"deleted" is not an object`))
		}
		var deleted map[string]DeletedName
		if err := json.Unmarshal(raw, &deleted); err != nil {
			return nil, unreadableAccounts(path, err)
		}
		for k, v := range deleted {
			// TS re-keys tombstones on load (Accounts.ts:89).
			a.deleted[Key(k)] = v
		}
	}
	a.reindex()
	return a, nil
}

func unreadableAccounts(file string, cause error) error {
	return fmt.Errorf("[accounts] %s could not be read (%v). It records which characters each account owns, the alts' passwords (through their primary) and deleted names. Every login is refused until it is repaired or restored from the backup taken together with the saves database: %w", file, cause, cause)
}

func (a *Accounts) reindex() {
	a.owners = map[string]string{}
	for account, entry := range a.entries {
		for _, ch := range entry.Characters {
			a.owners[Key(ch.Name)] = account
		}
	}
}

// Path returns the file this registry loads from and saves to.
func (a *Accounts) Path() string { return a.path }

// Save writes the whole registry to disk: temp file + rename, as in TS
// (Accounts.ts:96-103). Unlike the JSON save backend, TS does NOT fsync
// here; this port matches TS and does not fsync either. Save clears the
// dirty flag: every write carries pending last-played times.
func (a *Accounts) Save() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.saveLocked()
}

func (a *Accounts) saveLocked() error {
	target := a.path
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("[accounts] writing %s: %w", target, err)
	}
	// Ensure non-nil maps so the file always has both objects, as TS
	// writes {accounts: ..., deleted: ...} unconditionally.
	accounts := a.entries
	if accounts == nil {
		accounts = map[string]*AccountEntry{}
	}
	deleted := a.deleted
	if deleted == nil {
		deleted = map[string]DeletedName{}
	}
	data, err := json.MarshalIndent(accountsFile{Accounts: accounts, Deleted: deleted}, "", "  ")
	if err != nil {
		return fmt.Errorf("[accounts] writing %s: %w", target, err)
	}
	temporary := target + ".tmp"
	if err := os.WriteFile(temporary, data, 0o644); err != nil {
		return fmt.Errorf("[accounts] writing %s: %w", target, err)
	}
	if err := os.Rename(temporary, target); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("[accounts] writing %s: %w", target, err)
	}
	a.dirty = false
	return nil
}

// Flush writes pending last-played times now (shutdown). It writes only
// when dirty (Accounts.ts:111-114). Save errors are returned; the times
// stay pending.
func (a *Accounts) Flush() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.dirty {
		return nil
	}
	return a.saveLocked()
}

// Dirty reports whether last-played times are waiting to be written.
func (a *Accounts) Dirty() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.dirty
}

// AccountOf returns the account key owning this character name (the
// primary included), or ("", false) (Accounts.ts:133-136).
func (a *Accounts) AccountOf(typedName string) (accountKey string, found bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	account, ok := a.owners[Key(typedName)]
	return account, ok
}

// IsDeleted reports whether this name belonged to a deleted character:
// it can never be a character or login name again, except for the
// account that deleted it (Accounts.ts:139-142).
func (a *Accounts) IsDeleted(typedName string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	_, ok := a.deleted[Key(typedName)]
	return ok
}

// DeletedOf returns the tombstone for a name, if any (Accounts.ts:144-147).
func (a *Accounts) DeletedOf(name string) (DeletedName, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	t, ok := a.deleted[Key(name)]
	return t, ok
}

// DeletedCountOf counts how many deleted names this account holds
// reserved (Accounts.ts:150-156).
func (a *Accounts) DeletedCountOf(accountKey string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	account := Key(accountKey)
	count := 0
	for _, t := range a.deleted {
		if t.Account == account {
			count++
		}
	}
	return count
}

// CanDelete reports whether the account may delete another character
// (Accounts.ts:159-161).
func (a *Accounts) CanDelete(accountKey string) bool {
	return a.DeletedCountOf(accountKey) < MaxTombstonesPerAccount
}

// DeletedBy reports whether this deleted name was deleted by this
// account, which may create it again (Accounts.ts:164-167).
func (a *Accounts) DeletedBy(name, accountKey string) bool {
	t, ok := a.DeletedOf(name)
	return ok && t.Account != "" && t.Account == Key(accountKey)
}

// ReleaseDeleted frees a deleted name for anyone, returning its
// tombstone (staff action; Accounts.ts:175-189). On a write failure
// nothing changes and the error is returned.
func (a *Accounts) ReleaseDeleted(name string) (DeletedName, bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	nameKey := Key(name)
	tombstone, ok := a.deleted[nameKey]
	if !ok {
		return DeletedName{}, false, nil
	}
	delete(a.deleted, nameKey)
	if err := a.saveLocked(); err != nil {
		a.deleted[nameKey] = tombstone
		return DeletedName{}, false, err
	}
	return tombstone, true, nil
}

// EntryOf returns a copy of the account entry, if any (Accounts.ts:191-193).
func (a *Accounts) EntryOf(accountKey string) (AccountEntry, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.entries[Key(accountKey)]
	if !ok {
		return AccountEntry{}, false
	}
	cp := *entry
	cp.Characters = append([]AccountCharacter(nil), entry.Characters...)
	return cp, true
}

// CharactersOf returns a copy of the account's characters
// (Accounts.ts:195-197).
func (a *Accounts) CharactersOf(accountKey string) []AccountCharacter {
	entry, ok := a.EntryOf(accountKey)
	if !ok {
		return nil
	}
	return entry.Characters
}

// PrimaryOf returns the account's primary (first character) name, if any
// (Accounts.ts:199-201).
func (a *Accounts) PrimaryOf(accountKey string) (string, bool) {
	chars := a.CharactersOf(accountKey)
	if len(chars) == 0 {
		return "", false
	}
	return chars[0].Name, true
}

// IsPrimary reports whether name is the account's primary
// (Accounts.ts:203-206).
func (a *Accounts) IsPrimary(accountKey, name string) bool {
	primary, ok := a.PrimaryOf(accountKey)
	return ok && Key(primary) == Key(name)
}

// Ensure returns the account for this login name, creating it with the
// name as its first character when it has none (Accounts.ts:209-225).
// now is epoch milliseconds. It errors, changing nothing, when the name
// is a deleted character's name or the file cannot be written.
func (a *Accounts) Ensure(primaryName string, now int64) (AccountEntry, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	account := Key(primaryName)
	if existing, ok := a.entries[account]; ok {
		cp := *existing
		cp.Characters = append([]AccountCharacter(nil), existing.Characters...)
		return cp, nil
	}
	if _, deleted := a.deleted[account]; deleted {
		return AccountEntry{}, errors.New("That name belonged to a deleted character")
	}
	entry := &AccountEntry{
		CreatedAt:  now,
		Characters: []AccountCharacter{{Name: primaryName, CreatedAt: now, LastPlayedAt: 0}},
	}
	a.entries[account] = entry
	if err := a.saveLocked(); err != nil {
		delete(a.entries, account)
		return AccountEntry{}, err
	}
	a.reindex()
	cp := *entry
	cp.Characters = append([]AccountCharacter(nil), entry.Characters...)
	return cp, nil
}

// AddCharacter adds a character to the account. A name this account
// deleted comes back, its tombstone removed in the same write
// (Accounts.ts:232-251). It errors, changing nothing, when the name is
// owned, is another account's deleted name, the account does not exist,
// or the file cannot be written. now is epoch milliseconds.
func (a *Accounts) AddCharacter(accountKey, name string, now int64) (AccountCharacter, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.entries[Key(accountKey)]
	if !ok {
		return AccountCharacter{}, errors.New("No such account")
	}
	if _, owned := a.owners[Key(name)]; owned {
		return AccountCharacter{}, errors.New("That name is owned by an account")
	}
	nameKey := Key(name)
	tombstone, hasTombstone := a.deleted[nameKey]
	if hasTombstone && !(tombstone.Account != "" && tombstone.Account == Key(accountKey)) {
		return AccountCharacter{}, errors.New("That name belonged to a deleted character")
	}
	character := AccountCharacter{Name: name, CreatedAt: now, LastPlayedAt: 0}
	entry.Characters = append(entry.Characters, character)
	if hasTombstone {
		delete(a.deleted, nameKey)
	}
	if err := a.saveLocked(); err != nil {
		entry.Characters = entry.Characters[:len(entry.Characters)-1]
		if hasTombstone {
			a.deleted[nameKey] = tombstone
		}
		return AccountCharacter{}, err
	}
	a.reindex()
	return character, nil
}

// RemoveCharacter removes a character and keeps its name as a tombstone,
// in one write (Accounts.ts:258-278). It returns false, changing nothing,
// for the primary or a name the account doesn't have. It errors,
// changing nothing, when the account already holds
// MaxTombstonesPerAccount (check CanDelete first) or the file cannot be
// written. now is epoch milliseconds.
func (a *Accounts) RemoveCharacter(accountKey, name string, now int64) (bool, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.entries[Key(accountKey)]
	if !ok {
		return false, nil
	}
	if len(entry.Characters) > 0 && Key(entry.Characters[0].Name) == Key(name) {
		return false, nil // primary
	}
	index := -1
	for i, c := range entry.Characters {
		if Key(c.Name) == Key(name) {
			index = i
			break
		}
	}
	if index < 0 {
		return false, nil
	}
	if a.deletedCountLocked(accountKey) >= MaxTombstonesPerAccount {
		return false, errors.New("That account has deleted too many characters")
	}
	removed := entry.Characters[index]
	entry.Characters = append(entry.Characters[:index], entry.Characters[index+1:]...)
	nameKey := Key(removed.Name)
	before, hadBefore := a.deleted[nameKey]
	a.deleted[nameKey] = DeletedName{Name: removed.Name, Account: Key(accountKey), DeletedAt: now}
	if err := a.saveLocked(); err != nil {
		entry.Characters = append(entry.Characters, AccountCharacter{})
		copy(entry.Characters[index+1:], entry.Characters[index:])
		entry.Characters[index] = removed
		if hadBefore {
			a.deleted[nameKey] = before
		} else {
			delete(a.deleted, nameKey)
		}
		return false, err
	}
	a.reindex()
	return true, nil
}

func (a *Accounts) deletedCountLocked(accountKey string) int {
	account := Key(accountKey)
	count := 0
	for _, t := range a.deleted {
		if t.Account == account {
			count++
		}
	}
	return count
}

// TouchLastPlayed records a world entry in memory now; the file gets it
// on the next Save/Flush (Accounts.ts:281-287). Unknown names are
// ignored, as in TS.
func (a *Accounts) TouchLastPlayed(accountKey, name string, now int64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry, ok := a.entries[Key(accountKey)]
	if !ok {
		return
	}
	for i := range entry.Characters {
		if Key(entry.Characters[i].Name) == Key(name) {
			entry.Characters[i].LastPlayedAt = now
			a.dirty = true
			return
		}
	}
}

// NameAvailable reports whether accountKey may take name, considering
// ONLY the accounts registry: the name must not be owned by another
// account, and must not be a deleted name reserved by another account
// (a name this account deleted may be taken again: DeletedBy).
//
// PARTIAL — see DisplayNames: the full TS rule (DisplayNames.refusal)
// also refuses a name another account currently shows or holds as a
// display name, and a name shown by a live player. Those checks need
// DisplayNames state and world state that this registry does not hold.
// Use DisplayNames.NameAvailable for the full persist-level check and
// add the live-player check at the session layer. Do not treat a true
// result here as the final word for new-account creation.
func (a *Accounts) NameAvailable(name, accountKey string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	key := Key(name)
	if owner, ok := a.owners[key]; ok && owner != Key(accountKey) {
		return false
	}
	if tombstone, ok := a.deleted[key]; ok {
		if tombstone.Account == "" || tombstone.Account != Key(accountKey) {
			return false
		}
	}
	return true
}
