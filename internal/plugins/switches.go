package plugins

import (
	"errors"
	"strings"
)

// errDisabledPluginsNotStringArray is the ParseDisabledPlugins
// validation error, carrying the TS throw message verbatim
// (PluginManager.ts:2257).
var errDisabledPluginsNotStringArray = errors.New("[plugins] world.json disabledPlugins must be a string[]")

// NormalizeName lowercases a plugin name and strips everything but
// a-z0-9, mirroring normalizePluginName (WorldPluginConfig.ts:8-10).
// Switch keys and switch lookups are both normalized, so "My Plugin",
// "my_plugin", and "MYPLUGIN" are the same switch. (internal/config
// carries an identical unexported copy for world.json parsing; the
// hub must not depend on the config package's copy, so the six lines
// are duplicated here with the same citation.)
func NormalizeName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// ResolveEnabled reports whether a plugin is switched on, porting
// resolvePluginEnabled exactly (PluginManager.ts:159-171): the
// exported-name switch wins, then the file-name switch (the plugin's
// ID in Go), then the legacy disabledPlugins default, then the
// plugin's own Enabled (nil counts as on, per TS
// pluginEnabled !== false).
//
// switches must be keyed by normalized name (see NormalizeName);
// NewHub normalizes HubOptions.Switches, so callers may pass raw
// world.json keys. A nil switches map behaves as an empty one: every
// lookup misses and the fallbacks decide.
func ResolveEnabled(switches map[string]bool, defaultEnabled *bool, exportedName, fileName string, pluginEnabled *bool) bool {
	if enabled, ok := switches[NormalizeName(exportedName)]; ok {
		return enabled
	}
	if enabled, ok := switches[NormalizeName(fileName)]; ok {
		return enabled
	}
	if defaultEnabled != nil {
		return *defaultEnabled
	}
	return pluginEnabled == nil || *pluginEnabled
}

// ParseDisabledPlugins ports the disabledPlugins branch of
// parsePluginSwitches (PluginManager.ts:2249-2266): the deprecated
// world.json list form. Every listed name becomes a normalized switch
// set to false, and the FreeToPlay switch is forced to
// freeToPlayEnabled afterwards, so FreeToPlay always takes its
// explicit switch even when the list names it. Callers pair the
// returned map with HubOptions.DefaultEnabled pointing at true: the
// list form defaults every plugin on, and the returned map carries
// only the explicit offs.
//
// A blank or whitespace-only name is an error, mirroring the TS throw
// ("[plugins] world.json disabledPlugins must be a string[]"). The
// deprecation warning itself is logged by Hub.Load when
// DefaultEnabled is non-nil, matching the TS log site (parse time,
// during load).
//
// The modern world.json "plugins" map needs no equivalent parser: the
// config package already validates it into World.Plugins, and NewHub
// normalizes the keys. ParseDisabledPlugins exists so the legacy form
// stays portable if a caller ever reads the raw list — the config
// package does NOT apply it (see internal/config/world.go), so the
// deprecated list form is honored only through HubOptions (Switches
// built here plus DefaultEnabled), never through World.
func ParseDisabledPlugins(disabled []string, freeToPlayEnabled bool) (map[string]bool, error) {
	switches := make(map[string]bool, len(disabled)+1)
	for _, name := range disabled {
		if strings.TrimSpace(name) == "" {
			return nil, errDisabledPluginsNotStringArray
		}
		switches[NormalizeName(name)] = false
	}
	switches["freetoplay"] = freeToPlayEnabled
	return switches, nil
}
