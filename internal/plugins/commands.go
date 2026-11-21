package plugins

import (
	"encoding/json"
	"strings"

	"tsps-go/internal/game"
)

// Command-hub glue: the hub's shared command registry, the
// onCommand emit fragment, and the world.json command-permissions
// parser. The registry itself (registration, rights, dispatch)
// lives in internal/game/commands.go; this file is the hub side,
// mirroring the PluginManager command statics and helpers
// (PluginManager.ts:404-408, 2109-2206).

// Commands returns the hub's one shared command registry,
// creating it on first use. Api.RegisterCommand writes into it
// during Load; the coordinator reads it out after Load to
// install the dispatch seams (messenger, rights source,
// permissions, any-command hooks) and hand it to the chat
// listener path. A nil Hub, or a Hub without a registry (only
// possible for a Hub not built by NewHub), yields nil.
func (h *Hub) Commands() *game.CommandRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.sharedCommands()
}

// EmitCommandHooks runs the onCommand any-command hooks in
// registration order, through the per-hook error isolation.
// This is the first loop of PluginManager.emitCommand
// (PluginManager.ts:2183-2185) with error label "command": ALL
// hooks run (the loop has no break — the per-hook wrapper in
// Api.OnCommand skips once the event is handled), and the
// command registry reads event.Handled afterwards to decide
// whether base handlers and the rights gate run at all. A nil
// Hub, a Hub without a registry, or a nil event runs no hooks.
// The registry calls this through the seam the coordinator
// installs (Registry.SetCommandHooks); content never calls it
// directly.
func (h *Hub) EmitCommandHooks(event *game.CommandEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "command", h.hooks.snapshotCommandHooks(), event)
}

// commandPermissionRights maps the PlayerRights names world.json
// "commands:permissions" accepts to their rights ids, mirroring
// COMMAND_PERMISSION_RIGHTS (PluginManager.ts:206-214).
var commandPermissionRights = map[string]int{
	"NONE":          game.RightsNone,
	"MODERATOR":     game.RightsModerator,
	"ADMINISTRATOR": game.RightsAdministrator,
	"OWNER":         game.RightsOwner,
	"DEVELOPER":     game.RightsDeveloper,
}

// commandPermissionsTrimCutset is the JS trim cutset (WhiteSpace
// + LineTerminator), for the trims in commandPermissions
// (PluginManager.ts:2143-2144).
const commandPermissionsTrimCutset = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

// CommandPermissions parses the world.json pluginConfig
// "commands:permissions" option — { "<command>": "<PlayerRights
// name>" } — mirroring PluginManager.commandPermissions
// (PluginManager.ts:2137-2155). Each entry sets the lowest rank
// that may run that command, raising or lowering what it
// registered with. Names are trimmed, stripped of ONE leading
// "::" prefix, and lowercased; rights names are trimmed and
// uppercased before lookup. Entries with an empty name or an
// unrecognized/non-string rights value are warned about and
// skipped, with the TS message shape. A missing key, or a value
// that is not an object, yields an empty map with no warning,
// as in TS.
//
// TS parses lazily once and caches forever; the config is
// immutable after boot, so the coordinator calls this once at
// boot and installs the result on the registry
// (Registry.SetPermissions) — observably identical. When two
// entries normalize to the same name, one wins by map iteration
// order: the Go config map carries no insertion order, unlike
// TS Object.entries (the same nondeterminism the config package
// documents for identically normalizing names). The parse
// accepts map[string]any (JSON-decoded config) and
// map[string]string (hand-built config in tests).
func (h *Hub) CommandPermissions() map[string]int {
	out := make(map[string]int)
	if h == nil {
		return out
	}
	entries, ok := commandPermissionEntries(h.GetPluginConfig("commands:permissions", nil))
	if !ok {
		return out
	}
	for command, rights := range entries {
		name := strings.ToLower(strings.TrimPrefix(strings.Trim(command, commandPermissionsTrimCutset), "::"))
		rightsName, isString := rights.(string)
		id, known := 0, false
		if isString {
			id, known = commandPermissionRights[strings.ToUpper(strings.Trim(rightsName, commandPermissionsTrimCutset))]
		}
		if name == "" || !known {
			if logger := h.Logger(); logger != nil {
				logger.Printf("[plugins] commands:permissions ignores %s: %s",
					jsonString(command), jsonString(rights))
			}
			continue
		}
		out[name] = id
	}
	return out
}

// commandPermissionEntries extracts the {command: rights} pairs
// from a decoded "commands:permissions" value. It reports false
// when the value is not an object (TS: config is falsy, not an
// object, or an array — PluginManager.ts:2141).
func commandPermissionEntries(config any) (map[string]any, bool) {
	switch entries := config.(type) {
	case map[string]any:
		return entries, true
	case map[string]string:
		out := make(map[string]any, len(entries))
		for k, v := range entries {
			out[k] = v
		}
		return out, true
	default:
		return nil, false
	}
}

// jsonString renders v as TS JSON.stringify does, for the
// commands:permissions warning (PluginManager.ts:2146).
func jsonString(v any) string {
	if b, err := json.Marshal(v); err == nil {
		return string(b)
	}
	return "null"
}
