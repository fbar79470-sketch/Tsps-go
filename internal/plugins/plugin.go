package plugins

import "sync"

// Plugin is one compiled-in content plugin: the Go analogue of a
// PluginModule export (PluginTypes.ts:1772-1786). Where TS reads these
// fields off a required *.plugin.js module, Go content declares them
// in a struct literal and hands the value to Register or NewHub.
//
// Name is the exported name: what dependency matching, config-clash
// logs, and Loaded report. It is required and non-blank; a blank Name
// falls back to ID at load time, mirroring the TS fallback to the file
// name when plugin.name is missing (collectPluginLoadCandidates,
// PluginManager.ts:2448-2452). Leading and trailing whitespace is
// trimmed at load time, as TS trims plugin.name.
//
// ID is the stable identifier: the compile-time analogue of the TS
// filename key (pluginNameFromFile, PluginManager.ts:148-150). It
// defaults to Name. ID participates in switch resolution as the
// file-name lookup (see ResolveEnabled) and in the deferred re-queue
// sort order (see loader.go); it is also the name used in the
// failed-to-initialize log, where TS logs the file basename.
type Plugin struct {
	Name        string
	ID          string
	Description string
	// Members marks members-only content: skipped on a free-to-play
	// world (PluginManager.ts:769-773). TS reads members === true;
	// a zero value means the plugin loads on both world types.
	Members bool
	// Enabled is the plugin's own default switch (TS enabled?:
	// boolean, PluginTypes.ts:1781-1782). Nil means default on;
	// false keeps the plugin off unless world.json turns it on.
	// World.json switches and the legacy default both beat it;
	// see ResolveEnabled.
	Enabled *bool
	// DependsOn names the plugins that must load first, matched by
	// exact unnormalized name. Blank entries, self-dependencies,
	// and duplicates are dropped with warnings at load time.
	DependsOn []string
	// Config declares the world.json pluginConfig options this
	// plugin reads, keyed "namespace:key". Nil means the plugin
	// declares nothing. Declarations are validated before any
	// registration; see ConfigDeclaration and checkConfigDeclarations.
	Config map[string]ConfigDeclaration
	// Register attaches the plugin's hooks. Nil means the plugin
	// is skipped with a warning, mirroring the missing
	// register(api) export check in collectPluginLoadCandidates
	// (PluginManager.ts:2441-2447). A panic in Register is
	// recovered: the plugin is not marked loaded and its
	// dependents stay unresolved.
	Register func(api *Api)
	// RegisterDeferred, when non-nil, runs in the NEXT dependency
	// pass after this plugin's own Register, with cleared
	// dependencies, in ID order among deferred candidates (TS
	// sorts by pluginPath; Go sorts by ID — the documented
	// analogue). It receives the same Api Register got. A panic
	// in RegisterDeferred is recovered and logged the same way
	// as a Register failure, but the plugin stays loaded.
	RegisterDeferred func(api *Api)
}

// defaultRegistry is the package-level registry content packages
// append to with Register. Registration happens on one goroutine at
// boot, but the mutex guards it anyway: init-order and explicit wiring
// must never race.
var (
	defaultRegistryMu sync.Mutex
	defaultRegistry   []Plugin
)

// Register appends p to the default registry. Content packages call it
// from init or from explicit wiring; production builds a Hub from
// DefaultPlugins. Register performs no validation and no
// deduplication: blank names, nil Register functions, and duplicate
// names are all handled (warned, skipped, or loaded) by Hub.Load,
// mirroring what TS discovery tolerates per file.
func Register(p Plugin) {
	defaultRegistryMu.Lock()
	defer defaultRegistryMu.Unlock()
	defaultRegistry = append(defaultRegistry, p)
}

// DefaultPlugins returns a copy of the default registry in
// registration order. Mutating the returned slice does not affect the
// registry, and later Register calls do not affect a snapshot already
// taken: NewHub copies the list it receives again.
func DefaultPlugins() []Plugin {
	defaultRegistryMu.Lock()
	defer defaultRegistryMu.Unlock()
	out := make([]Plugin, len(defaultRegistry))
	copy(out, defaultRegistry)
	return out
}
