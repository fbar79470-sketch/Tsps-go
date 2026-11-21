// Command registry and dispatch: the port of the plugin-owned command
// system — PluginManager.registerCommand / emitCommand /
// playerHasCommandRights (PluginManager.ts:2109-2206, 4411-4484) plus
// CommandPacketListener.execute (CommandPacketListener.ts:7-53).
//
// There is NO core command system in TS. The CommandManager imports in
// CommandPacketListener.ts:1-3 are commented out, the
// game/model/commands/ directory does not exist at commit bfd81e3, and
// the only dispatch path is PluginManager.emitCommand. Every command
// is registered by a plugin through api.registerCommand; the core
// defines none. This file therefore ports ONLY the registry and the
// dispatch core. Plugin commands are content and stay NOT STARTED;
// the one command in this package's tests is a fixture, never shipped.
//
// The registry lives in this package (not internal/plugins) because
// the chat listener path must reach the dispatcher without importing
// the plugin hub, following the GroundItemManager precedent (engine
// state in game, hub as registration facade — the PersistAttribute
// precedent, where Api delegates to the persist registry). The hub
// holds one registry per Hub (see internal/plugins/commands.go) and
// Api.RegisterCommand delegates into it; wiring installs the seams.
//
// SAVE/STATE DETERMINATION (phase 12 scope item 5): NOTHING about
// chat or commands is persisted by the engine, and no persist change
// was made:
//
//   - Registrations, rights floors, and overrides are process state
//     (PluginManager statics); no save field holds them.
//   - commands:permissions is world.json pluginConfig, read at boot
//     (PluginManager.ts:2137-2155), not save data.
//   - Player rights DO persist (PlayerSave.rights; applyToPlayer
//     PlayerSave.ts:638, fromPlayer :748), and the Go save already
//     preserves Rights verbatim (internal/persist/playersave.go).
//     The Go Player carries no rights field and nothing in Go
//     mutates rights, so preserved-verbatim round-trips exactly
//     what TS fromPlayer would write for an unchanged player. The
//     gate reads rights through the injected RightsSource seam
//     (the coordinator supplies the carried save's Rights.ID);
//     with no seam installed every player reads as RightsNone.
//     Making rights live owned state is a future phase's work
//     (it needs a Player field this phase does not add).
//   - Mutes are txt-file lists (PlayerPunishment, already ported
//     in internal/persist), not save fields — and the command path
//     bypasses the mute/filter check entirely: ChatPacketListener
//     routes "::" text to CommandPacketListener.execute BEFORE
//     allowChat runs (ChatPacketListener.ts:31-35).
//   - Audio settings are already carried (phase 8); friends,
//     ignores, and friends-chat state are milestone 9 and stay
//     preserved verbatim. The only command-adjacent persisted
//     attribute in the tree (YELL_UNTIL_ATTRIBUTE,
//     PlayerCommands.plugin.js:168) is content-registered and
//     stays unregistered until that content ports, per the phase
//     11 precedent for content keys.

package game

import (
	"log"
	"sort"
	"strings"
	"sync"
)

// Rights floors for command gating: PlayerRights ids
// (model/rights/PlayerRights.ts:2-6). Rights ids are sequential and
// ordered (none < moderator < administrator < owner < developer), so
// a command's requirement is a floor everyone at or above also
// clears (PluginManager.ts:2116-2119). The Go Player carries no
// rights yet; these ids are the values the RightsSource seam
// returns and the values registrations name.
const (
	RightsNone          = 0
	RightsModerator     = 1
	RightsAdministrator = 2
	RightsOwner         = 3
	RightsDeveloper     = 4
)

// CommandHandler handles one command event. The return value ports
// the TS handler result (PluginTypes.ts:1263: handler returns
// void | boolean; the registerCommand wrapper,
// PluginManager.ts:4470-4473, sets event.handled unless the result
// is exactly false): return false ONLY to decline handling and
// leave the event unhandled; any other outcome returns true. A
// handler that panics is isolated like every other hook (the
// panic is logged and the next handler runs); it does not mark
// the event handled.
//
// Handlers reply through the event's Player: in TS, handlers call
// player.sendMessage (Mobile.sendMessage, Mobile.ts:999-1006,
// single string argument). The Go Player.SendMessage is owned by
// the phase 12 sendMessage work; until it lands, handlers in
// tests reply through closures over test fakes, and the engine's
// own messages go through the registry's Messenger seam.
type CommandHandler func(*CommandEvent) bool

// CommandEvent is one command dispatch. It mirrors
// PluginCommandEvent (PluginTypes.ts:859-865): the player, the
// sanitized command text, the normalized base name, the split
// parts, and the handled flag handlers set by returning true.
type CommandEvent struct {
	Player *Player
	// Raw is the sanitized command text: the text after the "::"
	// prefix was stripped, with NULs replaced by spaces and
	// surrounding whitespace trimmed (CommandPacketListener.ts:11,
	// 27). Internal spacing is preserved as typed; Parts holds
	// the whitespace-split form.
	Raw string
	// Base is the normalized command name: the leading run of
	// [a-z0-9_] in the first token (after stripping leading ":"
	// and "/" characters), lowercased
	// (CommandPacketListener.ts:17-22).
	Base string
	// Parts is Raw split on runs of whitespace, with Parts[0]
	// replaced by Base (CommandPacketListener.ts:15, 23).
	// Arguments are Parts[1:]; there is no quoting or escaping.
	Parts []string
	// Handled reports whether a handler claimed the command.
	// Handlers set it by returning true; dispatch reads it.
	Handled bool
}

// RegisteredCommand is one row of getRegisteredCommands: the
// normalized command name and the first non-empty description
// registered for it (PluginManager.ts:2175-2180).
type RegisteredCommand struct {
	Command     string
	Description string
}

// commandEntry is one registered handler for a base name, with
// the owning plugin's name (for error labelling) and the
// description shown in the Commands interface.
type commandEntry struct {
	pluginName  string
	description string
	handler     CommandHandler
}

// CommandRegistry holds the registered commands and their rights
// floors, mirroring the PluginManager statics
// commandHandlersByBase, commandRights, and
// commandRightsOverrides (PluginManager.ts:404-408), plus the
// parsed world.json permissions (commandPermissions,
// PluginManager.ts:2137-2155).
//
// Registration semantics, exactly as TS defines them
// (registerCommand, PluginManager.ts:4450-4481):
//
//   - Names are normalized by trimming and lowercasing. There is
//     NO "::" stripping at registration (unlike the permissions
//     parser) and NO alias mechanism: one call registers one
//     name, and a handler wanted under two names is registered
//     twice.
//   - A nil handler or an empty normalized name is silently
//     ignored: TS returns without logging
//     (PluginManager.ts:4451-4457). There is no throw, so —
//     unlike PersistAttribute — invalid input never fails a
//     plugin's load.
//   - Re-registering a name APPENDS the handler (handlers
//     accumulate in registration order) and OVERWRITES the
//     rights floor with the latest registration's floor
//     (commandRights.set, PluginManager.ts:4458-4461).
//   - Descriptions are trimmed; a missing description is "".
//
// The rights floor model collapses one TS distinction that is
// unobservable: TS stores number | null (null = unrestricted,
// from an omitted or non-PlayerRights minimumRights,
// PluginManager.ts:2109-2114), while Go stores a plain int floor
// and treats RightsNone (0) as the unrestricted value. The two
// are identical through every TS API: playerHasCommandRights
// returns true for null (PluginManager.ts:2126-2127) and for 0
// (every rights id is >= 0), and getRegisteredCommands filters
// through the same check. Any int floor is stored as given —
// TS keeps whatever integer getId() returns, including values
// no player can reach.
//
// Concurrency: Register/SetCommandRights/SetPermissions run on
// the boot goroutine (during hub Load), Execute/Emit run on the
// tick goroutine. The mutex guards the maps and seams; handler
// execution never holds the lock (a handler registering a
// command mid-dispatch must not deadlock), so Emit snapshots
// the handler list first. Seams installed after boot (Set*)
// take effect on the next dispatch.
type CommandRegistry struct {
	mu          sync.RWMutex
	handlers    map[string][]commandEntry
	rights      map[string]int
	overrides   map[string]int
	permissions map[string]int

	// messenger delivers the engine's own command replies: the
	// rights-denial line (PluginManager.ts:2197) and the
	// unknown-command line (CommandPacketListener.ts:51). Both
	// go through player.sendMessage in TS (Mobile.sendMessage,
	// Mobile.ts:999-1006, single string argument), so the seam
	// is func(*Player, string). Nil means the replies are
	// dropped: the coordinator installs Player.SendMessage once
	// the phase 12 sendMessage work lands.
	messenger func(*Player, string)
	// rightsSource answers the player's rights id for the gate.
	// Nil means every player reads as RightsNone (the degenerate
	// behavior while the Go Player carries no rights; see the
	// file header). The coordinator supplies the carried save's
	// Rights.ID.
	rightsSource func(*Player) int
	// commandHooks runs the onCommand any-command hooks: the
	// first loop of emitCommand (PluginManager.ts:2183-2185).
	// The hub installs it (Hub.EmitCommandHooks); nil means no
	// any-command hooks exist.
	commandHooks func(*CommandEvent)
}

// NewCommandRegistry returns an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		handlers:    make(map[string][]commandEntry),
		rights:      make(map[string]int),
		overrides:   make(map[string]int),
		permissions: make(map[string]int),
	}
}

// normalizeCommandName ports the registerCommand / setCommandRights
// normalization (PluginManager.ts:4454, 2165): trim, lowercase.
// No "::" stripping: a name registered with a "::" prefix never
// matches a dispatched base (bases never carry colons).
func normalizeCommandName(name string) string {
	return strings.ToLower(strings.Trim(name, commandTrimCutset))
}

// Register adds handler for name with the given rights floor and
// description, following the registerCommand semantics in the
// CommandRegistry doc comment. A nil handler or an empty
// normalized name is silently ignored. The plugin name recorded
// for error labelling is "" (engine/test registration); the hub
// uses RegisterPlugin so panics label the owning plugin.
func (r *CommandRegistry) Register(name string, handler CommandHandler, minimumRights int, description string) {
	r.register("", name, handler, minimumRights, description)
}

// RegisterPlugin is Register with the owning plugin's name, for
// the hub's Api.RegisterCommand: TS hook errors label the plugin
// (PluginManager.executeHook, PluginManager.ts:440).
func (r *CommandRegistry) RegisterPlugin(pluginName, name string, handler CommandHandler, minimumRights int, description string) {
	r.register(pluginName, name, handler, minimumRights, description)
}

func (r *CommandRegistry) register(pluginName, name string, handler CommandHandler, minimumRights int, description string) {
	if r == nil || handler == nil {
		return
	}
	normalized := normalizeCommandName(name)
	if normalized == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rights[normalized] = minimumRights
	r.handlers[normalized] = append(r.handlers[normalized], commandEntry{
		pluginName:  pluginName,
		description: strings.Trim(description, commandTrimCutset),
		handler:     handler,
	})
}

// SetCommandRights overrides the rights floor for name, whoever
// registered it, mirroring PluginManager.setCommandRights
// (PluginManager.ts:2158-2173): trim + lowercase, silently ignore
// an empty name. Overrides take priority over registration
// floors but lose to world.json permissions. PlayerRights.NONE
// (0) opens the command to every player.
func (r *CommandRegistry) SetCommandRights(name string, minimumRights int) {
	if r == nil {
		return
	}
	normalized := normalizeCommandName(name)
	if normalized == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.overrides[normalized] = minimumRights
}

// SetPermissions installs the parsed world.json
// "commands:permissions" map (normalized names to required rights
// ids), replacing any previous map. The hub parses the config
// (Hub.CommandPermissions); this method stores the result. TS
// parses lazily once and caches forever
// (PluginManager.ts:2137-2155); the config is immutable after
// boot, so a boot-time install is observably identical. A nil
// map installs an empty map.
func (r *CommandRegistry) SetPermissions(permissions map[string]int) {
	if r == nil {
		return
	}
	cp := make(map[string]int, len(permissions))
	for name, id := range permissions {
		cp[name] = id
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.permissions = cp
}

// SetMessenger installs the engine-reply seam (see the
// CommandRegistry doc comment). A nil messenger drops replies.
func (r *CommandRegistry) SetMessenger(messenger func(*Player, string)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messenger = messenger
}

// SetRightsSource installs the rights-id seam (see the
// CommandRegistry doc comment). A nil source reads RightsNone.
func (r *CommandRegistry) SetRightsSource(rightsSource func(*Player) int) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rightsSource = rightsSource
}

// SetCommandHooks installs the onCommand any-command hook runner
// (see the CommandRegistry doc comment). A nil runner means no
// any-command hooks exist.
func (r *CommandRegistry) SetCommandHooks(hooks func(*CommandEvent)) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commandHooks = hooks
}

// rightsID answers p's rights id through the installed source,
// or RightsNone with no source or a nil player. (TS has no nil
// player case; the nil guard is the Go nil-safety convention.)
func (r *CommandRegistry) rightsID(p *Player) int {
	if r == nil || p == nil {
		return RightsNone
	}
	r.mu.RLock()
	fn := r.rightsSource
	r.mu.RUnlock()
	if fn == nil {
		return RightsNone
	}
	return fn(p)
}

// requiredFloor resolves the rights floor for base in TS
// priority order (playerHasCommandRights,
// PluginManager.ts:2120-2130): world.json permissions win over
// plugin overrides, which win over the registration floor. The
// second result reports whether any floor applies at all; with
// none, every player passes.
func (r *CommandRegistry) requiredFloor(base string) (int, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if id, ok := r.permissions[base]; ok {
		return id, true
	}
	if id, ok := r.overrides[base]; ok {
		return id, true
	}
	if id, ok := r.rights[base]; ok {
		return id, true
	}
	return 0, false
}

// PlayerHasCommandRights reports whether p may run base,
// mirroring PluginManager.playerHasCommandRights
// (PluginManager.ts:2120-2130). Base is used as given — TS
// looks up event.base, already normalized by the parser — so
// callers pass a normalized base. With no floor registered for
// base, every player passes.
func (r *CommandRegistry) PlayerHasCommandRights(p *Player, base string) bool {
	if r == nil {
		return true
	}
	floor, ok := r.requiredFloor(base)
	if !ok {
		return true
	}
	return r.rightsID(p) >= floor
}

// RegisteredCommands returns the commands p may run, mirroring
// PluginManager.getRegisteredCommands
// (PluginManager.ts:2175-2180): rights-filtered, description from
// the first registered handler with a non-empty description,
// sorted by command name.
//
// Deliberate deviation: TS sorts with localeCompare; Go sorts
// with string <. The two differ only for non-ASCII names, and
// dispatched bases are [a-z0-9_]+ by construction — the same
// documented deviation as the definition-registry tie-break.
func (r *CommandRegistry) RegisteredCommands(p *Player) []RegisteredCommand {
	if r == nil {
		return nil
	}
	rights := r.rightsID(p)
	r.mu.RLock()
	names := make([]string, 0, len(r.handlers))
	descriptions := make(map[string]string, len(r.handlers))
	for name, entries := range r.handlers {
		names = append(names, name)
		for _, e := range entries {
			if e.description != "" {
				descriptions[name] = e.description
				break
			}
		}
	}
	// Resolve floors under the same lock: permissions >
	// overrides > registration (see requiredFloor).
	floors := make(map[string]int, len(r.rights)+len(r.overrides)+len(r.permissions))
	for name, id := range r.rights {
		floors[name] = id
	}
	for name, id := range r.overrides {
		floors[name] = id
	}
	for name, id := range r.permissions {
		floors[name] = id
	}
	r.mu.RUnlock()
	sort.Strings(names)
	out := make([]RegisteredCommand, 0, len(names))
	for _, name := range names {
		if floor, ok := floors[name]; ok && rights < floor {
			continue
		}
		out = append(out, RegisteredCommand{Command: name, Description: descriptions[name]})
	}
	return out
}

// send delivers an engine command reply through the installed
// messenger, dropping it when no messenger is installed.
func (r *CommandRegistry) send(p *Player, message string) {
	if r == nil || p == nil {
		return
	}
	r.mu.RLock()
	messenger := r.messenger
	r.mu.RUnlock()
	if messenger != nil {
		messenger(p, message)
	}
}

// Emit runs one parsed command event through the plugin hooks,
// mirroring PluginManager.emitCommand
// (PluginManager.ts:2182-2206):
//
//  1. The onCommand any-command hooks run (all of them; the
//     per-hook wrapper skips once handled). If the event ends
//     handled, Emit returns true without consulting base
//     handlers or rights.
//  2. With no base handlers, Emit returns event.Handled
//     (false at this point).
//  3. The rights gate runs: a player below the floor is sent
//     "You do not have permission to use this command." and
//     Emit returns true — the denial counts as handled, so no
//     unknown-command message follows.
//  4. Base handlers run in registration order; each is skipped
//     once the event is handled (the registerCommand wrapper
//     semantics, PluginManager.ts:4466-4474), and a handler
//     returning false leaves the event unhandled for the next
//     handler. A panicking handler is logged and isolated and
//     does not mark the event handled.
//
// Emit returns event.Handled. A nil event returns false.
func (r *CommandRegistry) Emit(event *CommandEvent) bool {
	if r == nil || event == nil {
		return false
	}
	r.mu.RLock()
	hooks := r.commandHooks
	r.mu.RUnlock()
	if hooks != nil {
		hooks(event)
	}
	if event.Handled {
		return true
	}
	r.mu.RLock()
	entries := append([]commandEntry(nil), r.handlers[event.Base]...)
	r.mu.RUnlock()
	if len(entries) == 0 {
		return event.Handled
	}
	if !r.PlayerHasCommandRights(event.Player, event.Base) {
		r.send(event.Player, "You do not have permission to use this command.")
		return true
	}
	for _, e := range entries {
		if event.Handled {
			// The registerCommand wrapper skip
			// (PluginManager.ts:4467): later handlers never
			// run once an earlier one claimed the command.
			continue
		}
		if runCommandHandler(e, event) {
			event.Handled = true
		}
	}
	return event.Handled
}

// runCommandHandler runs one base handler with executeHook-style
// error isolation (PluginManager.executeHook,
// PluginManager.ts:436-444): a panic is recovered, logged with
// the TS message shape, and reported as unhandled so the next
// handler still runs. It returns the handler's result.
func runCommandHandler(e commandEntry, event *CommandEvent) (handled bool) {
	defer func() {
		if rec := recover(); rec != nil {
			pluginName := e.pluginName
			if pluginName == "" {
				pluginName = "engine"
			}
			log.Printf("[plugins] command hook failed (%s): %v", pluginName, rec)
			handled = false
		}
	}()
	return e.handler(event)
}

// ParsedCommand is the result of parsing command input: the
// sanitized text, the normalized base name, and the split parts.
// See ParseCommand.
type ParsedCommand struct {
	Raw   string
	Base  string
	Parts []string
}

// commandTrimCutset is the set of characters JavaScript's String
// trim removes (WhiteSpace + LineTerminator), used to reproduce
// the trims in CommandPacketListener.execute
// (CommandPacketListener.ts:11) and the registerCommand /
// permissions normalizations. It is the same cutset the chat
// listener uses for its "::" check (see internal/listeners).
const commandTrimCutset = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

// isJSWhitespace reports whether r is a JavaScript \s character
// (WhiteSpace + LineTerminator). It differs from unicode.IsSpace
// in exactly one rune: U+0085 (NEL) is space in Go but not in
// JS, so a hand-rolled predicate — not strings.Fields — splits
// command text exactly as TS split(/\s+/) does.
func isJSWhitespace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ',
		'\u00a0', '\u1680', '\u2028', '\u2029', '\u202f', '\u205f', '\u3000', '\ufeff':
		return true
	}
	return r >= '\u2000' && r <= '\u200a'
}

// splitJSWhitespace splits s on runs of JavaScript whitespace,
// mirroring String split(/\s+/) on a trimmed non-empty string:
// no empty elements, ever.
func splitJSWhitespace(s string) []string {
	var parts []string
	start := -1
	for i, r := range s {
		if isJSWhitespace(r) {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}

// ParseCommand sanitizes and splits command input, porting the
// parse in CommandPacketListener.execute
// (CommandPacketListener.ts:11-23). The input is the text AFTER
// the "::" prefix was stripped by ChatPacketListener.handleText
// (ChatPacketListener.ts:30-32), which also strips angle
// brackets, trims, and truncates to 80 characters before the
// "::" check — none of that is repeated here.
//
// Steps, in TS order: replace NULs with spaces, trim, split on
// runs of whitespace; strip leading ":" and "/" characters from
// the first token (accepting payload variants like "::item",
// "/item", and tokens with trailing punctuation); take the
// leading [a-z0-9_]+ run, lowercased, as the base; replace
// parts[0] with the base. There is no quoting: arguments are
// the whitespace-split tokens after the base.
//
// ParseCommand reports false when the input is empty after
// sanitizing or the first token has no [a-z0-9_] prefix — both
// cases TS returns silently from, with no message sent.
//
// Deliberate deviation: the [a-z0-9_] match is ASCII-only. JS
// case-insensitive matching without the /u flag additionally
// canonicalizes U+017F (long s) and U+212A (Kelvin sign) into
// the range; no real command name contains them, and
// reproducing the canonicalization would complicate the match
// for no observable command.
func ParseCommand(rawCommand string) (ParsedCommand, bool) {
	sanitized := strings.ReplaceAll(rawCommand, "\x00", " ")
	sanitized = strings.Trim(sanitized, commandTrimCutset)
	if sanitized == "" {
		return ParsedCommand{}, false
	}
	parts := splitJSWhitespace(sanitized)
	baseToken := strings.TrimLeft(parts[0], ":/")
	i := 0
	for i < len(baseToken) {
		c := baseToken[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
			i++
			continue
		}
		break
	}
	if i == 0 {
		return ParsedCommand{}, false
	}
	base := strings.ToLower(baseToken[:i])
	parts[0] = base
	return ParsedCommand{Raw: sanitized, Base: base, Parts: parts}, true
}

// DispatchCommand dispatches one "::" command for p, discarding
// Execute's result: it is the listeners.CommandDispatcher adapter
// (internal/listeners/chat.go), whose interface returns nothing —
// TS execute returns void, and the listener reports the dispatch
// as acted whenever a dispatcher is wired. The coordinator assigns
// the hub's registry directly to the listener seam; no wrapper
// type is needed.
func (r *CommandRegistry) DispatchCommand(p *Player, rawCommand string) {
	r.Execute(p, rawCommand)
}

// Execute dispatches one "::" command for p, porting
// CommandPacketListener.execute (CommandPacketListener.ts:7-53)
// end to end. The input contract is ParseCommand's: text after
// the "::" prefix was stripped.
//
// Order, in TS order: a dead player (hitpoints <= 0, read as the
// SkillManager's current Hitpoints level per Player.ts:329-331)
// is refused silently; unparseable input is refused silently;
// otherwise the event (player, raw, base, parts, handled=false)
// goes through Emit; when nothing handles it, a warning is
// logged (TS console.warn "[command] unhandled_command") and
// the player is sent "This command does not exist. Try
// ::players."
//
// Execute returns true when a command parsed and dispatch ran
// (handled, rights-denied, or unknown — a reply or warning was
// produced); false when the player is dead, the player is nil,
// or the input did not parse. The return value is a Go-only
// convenience for the listener seam: TS execute returns void.
//
// The hitpoints gate degenerates for a player without a
// SkillManager (only a zero-value Player; NewPlayer always
// installs one): with no skills to read, the gate passes. The
// unknown-command and denial replies go through the Messenger
// seam and are dropped when none is installed.
func (r *CommandRegistry) Execute(p *Player, rawCommand string) bool {
	if r == nil || p == nil {
		return false
	}
	if skills := p.Skills(); skills != nil && skills.CurrentLevel(SkillHitpoints) <= 0 {
		return false
	}
	parsed, ok := ParseCommand(rawCommand)
	if !ok {
		return false
	}
	event := &CommandEvent{
		Player: p,
		Raw:    parsed.Raw,
		Base:   parsed.Base,
		Parts:  parsed.Parts,
	}
	if r.Emit(event) {
		return true
	}
	log.Printf("[command] unhandled_command username=%s raw=%q sanitized=%q base=%s parts=%q",
		p.Username, rawCommand, parsed.Raw, parsed.Base, parsed.Parts)
	r.send(p, "This command does not exist. Try ::players.")
	return true
}
