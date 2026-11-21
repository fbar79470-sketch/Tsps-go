package persist

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Player punishment lists: port of util/PlayerPunishment.ts.
//
// Four plain-text files under the saves directory, one entry per line:
//
//	Bans.txt, IPBans.txt, IPMutes.txt, Mutes.txt
//
// File format (PlayerPunishment.ts:66-76): the file is split on '\n'
// with NO trimming — a trailing newline yields a final empty entry, and
// '\r' characters from CRLF files stay attached to entries (and so never
// match a lookup). This port preserves that parsing exactly: entries are
// stored verbatim. Name entries are stored in FormatPlayerName form
// (lowercased input, first/after-separator letters capitalized).
//
// Unlike TS (static state, async queued writes), Punishment is an
// instance type and mutations write synchronously. TS addToFile appends
// asynchronously and deleteFromFile rewrites asynchronously, with errors
// surfaced by flush(); synchronous writes fail at the call site instead,
// which is strictly safer for a login gate. Flush exists for API
// symmetry and always returns nil: there is nothing queued to await.
// All methods are safe for concurrent use.

const (
	bansFile    = "Bans.txt"
	ipBansFile  = "IPBans.txt"
	ipMutesFile = "IPMutes.txt"
	mutesFile   = "Mutes.txt"
)

// Punishment holds the loaded ban/mute lists for one saves directory.
type Punishment struct {
	mu             sync.Mutex
	dir            string
	ipsBanned      map[string]struct{}
	ipsMuted       map[string]struct{}
	accountsBanned map[string]struct{}
	accountsMuted  map[string]struct{}
}

// LoadPunishment reads the four list files from savesDir, creating the
// directory if needed (PlayerPunishment.init, PlayerPunishment.ts:43-46,
// ensures the directories exist). Missing files load as empty lists, as
// in TS initializeList (which returns early when the file doesn't
// exist). A file that exists but cannot be read is an error; TS logs
// and continues in initializeList, but silently dropping a ban list at
// a login gate is worse than refusing to start, so this port fails
// loudly. (DIVERGENCE, deliberate.)
func LoadPunishment(savesDir string) (*Punishment, error) {
	p := &Punishment{
		dir:            savesDir,
		ipsBanned:      map[string]struct{}{},
		ipsMuted:       map[string]struct{}{},
		accountsBanned: map[string]struct{}{},
		accountsMuted:  map[string]struct{}{},
	}
	if err := os.MkdirAll(savesDir, 0o755); err != nil {
		return nil, fmt.Errorf("[punishments] creating %s: %w", savesDir, err)
	}
	lists := []struct {
		file string
		set  map[string]struct{}
	}{
		{ipBansFile, p.ipsBanned},
		{bansFile, p.accountsBanned},
		{ipMutesFile, p.ipsMuted},
		{mutesFile, p.accountsMuted},
	}
	for _, l := range lists {
		if err := loadPunishmentList(filepath.Join(savesDir, l.file), l.set); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// loadPunishmentList is initializeList (PlayerPunishment.ts:66-76):
// split on '\n', no trimming, duplicates kept (a set here is equivalent
// for every lookup; TS ban()/mute() also push duplicates into the
// in-memory array unconditionally, which only affects list length, never
// a lookup result).
func loadPunishmentList(path string, set map[string]struct{}) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("[punishments] reading %s: %w", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		set[line] = struct{}{}
	}
	return nil
}

// Dir returns the saves directory these lists live in.
func (p *Punishment) Dir() string { return p.dir }

// Flush awaits nothing: writes in this port are synchronous, so there
// are no queued writes or stored write errors as in TS flush()
// (PlayerPunishment.ts:13-16). It always returns nil.
func (p *Punishment) Flush() error { return nil }

// Reload re-reads all four files from disk, replacing the in-memory
// lists (cf. PlayerPunishment.init resetting the lists, and
// reloadIPBans/reloadIPMutes). On a read error the previous lists are
// kept and the error is returned.
func (p *Punishment) Reload() error {
	fresh, err := LoadPunishment(p.dir)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.ipsBanned = fresh.ipsBanned
	p.ipsMuted = fresh.ipsMuted
	p.accountsBanned = fresh.accountsBanned
	p.accountsMuted = fresh.accountsMuted
	return nil
}

// ReloadIPBans re-reads IPBans.txt only (PlayerPunishment.ts:156-159).
func (p *Punishment) ReloadIPBans() error { return p.reloadOne(ipBansFile, &p.ipsBanned) }

// ReloadIPMutes re-reads IPMutes.txt only (PlayerPunishment.ts:161-164).
func (p *Punishment) ReloadIPMutes() error { return p.reloadOne(ipMutesFile, &p.ipsMuted) }

func (p *Punishment) reloadOne(file string, dst *map[string]struct{}) error {
	set := map[string]struct{}{}
	if err := loadPunishmentList(filepath.Join(p.dir, file), set); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	*dst = set
	return nil
}

// Banned reports whether the player is banned
// (PlayerPunishment.ts:117-120). The name is lowercased then formatted,
// exactly as TS does before the lookup.
func (p *Punishment) Banned(player string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.accountsBanned[FormatPlayerName(strings.ToLower(player))]
	return ok
}

// Muted reports whether the player is muted
// (PlayerPunishment.ts:133-136).
func (p *Punishment) Muted(player string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.accountsMuted[FormatPlayerName(strings.ToLower(player))]
	return ok
}

// IPBanned reports whether the IP is banned. An empty/blank IP is never
// banned (PlayerPunishment.ts:144-146). The IP is matched verbatim — no
// trimming — against the stored lines.
func (p *Punishment) IPBanned(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.ipsBanned[ip]
	return strings.TrimSpace(ip) != "" && ok
}

// IPMuted reports whether the IP is muted (PlayerPunishment.ts:148-150).
func (p *Punishment) IPMuted(ip string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.ipsMuted[ip]
	return strings.TrimSpace(ip) != "" && ok
}

// Ban bans the player, appending to Bans.txt unless already present
// (PlayerPunishment.ts:96-102). Unlike TS, this port does NOT trigger a
// save of the online account (saveOnlineAccount needs the World); the
// caller must request a save for an online player separately.
func (p *Punishment) Ban(player string) error {
	return p.addName(bansFile, player)
}

// Mute mutes the player, appending to Mutes.txt unless already present
// (PlayerPunishment.ts:104-110).
func (p *Punishment) Mute(player string) error {
	return p.addName(mutesFile, player)
}

// Unban unbans the player, removing the line from Bans.txt
// (PlayerPunishment.ts:152-157).
func (p *Punishment) Unban(player string) error {
	return p.removeName(bansFile, player)
}

// Unmute unmutes the player, removing the line from Mutes.txt
// (PlayerPunishment.ts:159-164).
func (p *Punishment) Unmute(player string) error {
	return p.removeName(mutesFile, player)
}

// ToggleBan toggles the ban, returning whether the player is banned
// after the change (PlayerPunishment.ts:123-130).
func (p *Punishment) ToggleBan(player string) (bool, error) {
	if p.Banned(player) {
		return false, p.Unban(player)
	}
	return true, p.Ban(player)
}

// ToggleMute toggles the mute, returning whether the player is muted
// after the change (PlayerPunishment.ts:139-147).
func (p *Punishment) ToggleMute(player string) (bool, error) {
	if p.Muted(player) {
		return false, p.Unmute(player)
	}
	return true, p.Mute(player)
}

// AddBannedIP bans an IP, appending to IPBans.txt unless already present
// (PlayerPunishment.ts:78-83).
func (p *Punishment) AddBannedIP(ip string) error {
	return p.addRaw(ipBansFile, ip)
}

// AddMutedIP mutes an IP, appending to IPMutes.txt unless already present
// (PlayerPunishment.ts:84-89).
func (p *Punishment) AddMutedIP(ip string) error {
	return p.addRaw(ipMutesFile, ip)
}

// RemoveBannedIP unbans an IP (PlayerPunishment.ts:91-94).
func (p *Punishment) RemoveBannedIP(ip string) error {
	return p.removeRaw(ipBansFile, ip)
}

// RemoveMutedIP unmutes an IP (PlayerPunishment.ts:96-99).
func (p *Punishment) RemoveMutedIP(ip string) error {
	return p.removeRaw(ipMutesFile, ip)
}

// setFor resolves the in-memory set for a list file. Callers must hold p.mu.
func (p *Punishment) setFor(file string) map[string]struct{} {
	switch file {
	case bansFile:
		return p.accountsBanned
	case mutesFile:
		return p.accountsMuted
	case ipBansFile:
		return p.ipsBanned
	default: // ipMutesFile
		return p.ipsMuted
	}
}

func (p *Punishment) addName(file, player string) error {
	return p.addRaw(file, FormatPlayerName(strings.ToLower(player)))
}

func (p *Punishment) removeName(file, player string) error {
	return p.removeRaw(file, FormatPlayerName(strings.ToLower(player)))
}

// addRaw is addToFile (PlayerPunishment.ts:178-180): append entry + '\n',
// unless already present. The in-memory set is updated only after the
// file write succeeds (TS updates memory first and writes async; on a
// write failure TS keeps the in-memory entry and records the error for
// flush(). Failing the call instead keeps memory and disk consistent.)
func (p *Punishment) addRaw(file, entry string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.setFor(file)
	if _, ok := set[entry]; ok {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(p.dir, file), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("[punishments] appending to %s: %w", file, err)
	}
	if _, err := f.WriteString(entry + "\n"); err != nil {
		_ = f.Close()
		return fmt.Errorf("[punishments] appending to %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("[punishments] appending to %s: %w", file, err)
	}
	set[entry] = struct{}{}
	return nil
}

// removeRaw is deleteFromFile (PlayerPunishment.ts:166-175): rewrite the
// file with every line except entry, joined by '\n' (a missing file
// reads as "" and the rewrite creates it, matching TS).
func (p *Punishment) removeRaw(file, entry string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	set := p.setFor(file)
	path := filepath.Join(p.dir, file)
	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("[punishments] reading %s: %w", file, err)
	}
	kept := make([]string, 0)
	if len(data) > 0 {
		for _, line := range strings.Split(string(data), "\n") {
			if line != entry {
				kept = append(kept, line)
			}
		}
	}
	if err := os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		return fmt.Errorf("[punishments] writing %s: %w", file, err)
	}
	delete(set, entry)
	return nil
}
