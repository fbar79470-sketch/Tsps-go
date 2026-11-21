package plugins

import (
	"fmt"
	"log"
)

// HubOptions configures a Hub. All fields are read at NewHub time;
// later mutation of the passed maps does not affect the hub, because
// NewHub copies them.
type HubOptions struct {
	// Logger receives all hub output with the TS "[plugins] ..."
	// message shapes. Nil means log.Default(); the nil fallback in
	// NewHub is the only log.Default() call in the package.
	Logger *log.Logger
	// Switches maps plugin names to their world.json on/off switch,
	// from internal/config World.Plugins. Keys may be raw or
	// normalized: NewHub normalizes them with NormalizeName, so
	// passing World.Plugins unchanged is correct, and passing
	// already-normalized keys is harmless (normalization is
	// idempotent). When two raw names normalize to the same key,
	// one wins by map iteration order — the same nondeterminism
	// the config package documents for identically normalizing
	// names. A nil map means no switches are set.
	Switches map[string]bool
	// DefaultEnabled carries the legacy disabledPlugins default:
	// non-nil only when the deprecated list form is in use, in
	// which case it points at true (the list form defaults every
	// plugin on; see ParseDisabledPlugins). Nil when the modern
	// "plugins" map is in use. Hub.Load logs the TS deprecation
	// warning whenever this is non-nil. The config package does
	// NOT apply the legacy form, so honoring it is entirely the
	// caller's job: build Switches with ParseDisabledPlugins and
	// set this field.
	DefaultEnabled *bool
	// PluginConfig is the normalized world.json pluginConfig map,
	// from internal/config World.PluginConfig. Keys are already
	// flat "Plugin:setting" keys with the legacy namespace mapping
	// applied; the hub reads them verbatim and never re-normalizes.
	PluginConfig map[string]any
	// MembersWorld reports whether this is a members world (the
	// negation of the FreeToPlay plugin switch, computed by
	// internal/config). On a free-to-play world, members-only
	// plugins are gated out at load time.
	MembersWorld bool
	// DisablePlayerBots is reserved: TS drops the PlayerBots
	// plugin file when --disablePlayerBots is passed or
	// DISABLE_PLAYER_BOTS=1 (PluginManager.ts:710-721). No bot
	// content exists in Go, and bot content is identified in TS by
	// file path, which compile-time registration has no analogue
	// of — so the option is currently accepted and unused. It is
	// kept in the options shape so wiring code can pass the flag
	// through without the hub inventing a name-based filter.
	DisablePlayerBots bool
}

// Hub is one plugin manager instance: the Go analogue of the static
// PluginManager class, as an instance so tests can build isolated
// hubs. A Hub is built with NewHub, loaded once with Load, and then
// serves hook emits for the life of the server.
//
// Concurrency: Load runs on the boot goroutine, like TS loading on
// the single JS thread, and must not be called concurrently. After
// Load returns, Loaded, GetPluginConfig, and ConfigDeclarations are
// safe to call from any goroutine (they read state Load finished
// writing); hook emits go through the hook registry, which guards
// its own lists. The package-level registry in plugin.go is the only
// mutex-guarded state here.
type Hub struct {
	plugins []Plugin
	logger  *log.Logger

	switches       map[string]bool
	defaultEnabled *bool
	pluginConfig   map[string]any
	membersWorld   bool
	// disablePlayerBots is reserved; see HubOptions.DisablePlayerBots.
	disablePlayerBots bool

	// hooks is the hook registry (hooks.go), created in NewHub so
	// it exists before any registration runs.
	hooks *hookRegistry

	initialized  bool
	loadedNames  []string
	declarations map[string]DeclaredConfig
}

// NewHub builds a Hub from an explicit plugin list and options.
// Production passes DefaultPlugins(); tests pass hand-built lists.
// The list, switches, and plugin config are copied; switch keys are
// normalized (see HubOptions.Switches).
func NewHub(list []Plugin, opts HubOptions) *Hub {
	logger := opts.Logger
	if logger == nil {
		logger = log.Default()
	}
	switches := make(map[string]bool, len(opts.Switches))
	for name, enabled := range opts.Switches {
		switches[NormalizeName(name)] = enabled
	}
	pluginConfig := make(map[string]any, len(opts.PluginConfig))
	for key, value := range opts.PluginConfig {
		pluginConfig[key] = value
	}
	plugins := make([]Plugin, len(list))
	copy(plugins, list)
	return &Hub{
		plugins:           plugins,
		logger:            logger,
		switches:          switches,
		defaultEnabled:    opts.DefaultEnabled,
		pluginConfig:      pluginConfig,
		membersWorld:      opts.MembersWorld,
		disablePlayerBots: opts.DisablePlayerBots,
		hooks:             newHookRegistry(),
		declarations:      make(map[string]DeclaredConfig),
	}
}

// Load validates, orders, and registers the hub's plugins, in TS
// loadFromDirectory order (PluginManager.ts:681-793) with the
// filesystem-discovery steps removed: legacy-switch warning,
// filename-switch pre-filter, unknown-switch warnings, switch
// resolution plus members gating, config-declaration validation
// before ANY registration, dependency-ordered registration,
// declaration recording, invalid-value warnings, the plugins:loaded
// emit, and the counts log. See doc.go for the full order.
//
// Load is idempotent: a second call is a no-op (the TS initialized
// guard, PluginManager.ts:684-687), silently. It never fails: bad
// declarations, failed initializations, and unresolved dependencies
// are all logged and skipped, never fatal — a bad plugin must never
// stop the server.
//
// The discovery early-returns have no Go analogue and are not
// reproduced: with no plugins registered, Load still runs the whole
// pipeline (emitting plugins:loaded with no names and logging
// active=0 disabled=0), because there is no directory whose absence
// could cut the load short. The persistence-override log
// (PluginManager.ts:782-784) is likewise omitted: no Go mechanism
// registers a persistence override.
func (h *Hub) Load() {
	if h.initialized {
		return
	}
	h.initialized = true

	if h.defaultEnabled != nil {
		h.logf(`[plugins] world.json disabledPlugins is deprecated and overrides "plugins" and plugin defaults except FreeToPlay; replace it with "plugins": { "Name": false }`)
	}
	candidates, disabledCount, enabledCount := h.collectCandidates()
	h.warnUnknownSwitches(candidates)
	toLoad, membersOnlyCount, newlyDisabled := h.filterToLoad(candidates)
	disabledCount += newlyDisabled
	h.checkConfigDeclarations(toLoad)
	h.loadWithDependencies(toLoad)
	h.recordLoadedConfigDeclarations(toLoad)
	h.warnAboutInvalidConfigValues()
	h.emitPluginsLoaded()
	membersSuffix := ""
	if membersOnlyCount > 0 {
		membersSuffix = fmt.Sprintf(" members-only=%d", membersOnlyCount)
	}
	h.logf("[plugins] active=%d disabled=%d%s", len(h.loadedNames), disabledCount, membersSuffix)
	if enabledCount > 0 && len(h.loadedNames) == 0 {
		// TS names the plugin directory here; Go has no directory
		// to name, so the message ends at "loaded".
		h.logf("[plugins] no valid plugins loaded")
	}
}

// Loaded returns the loaded plugin names in load order: the Go
// analogue of PluginManager.loadedPlugins. The returned slice is a
// copy. Duplicate exported names appear once per loaded plugin, as
// in TS, which never deduplicates loadedPlugins.
func (h *Hub) Loaded() []string {
	out := make([]string, len(h.loadedNames))
	copy(out, h.loadedNames)
	return out
}

// Logger returns the logger hub output goes through: the injected
// HubOptions.Logger, or log.Default() when none was given. It is nil
// only for a nil Hub. The hooks half (api.go, hooks.go, events.go,
// core.go) calls it to log registration refusals and hook failures
// through the same logger Load uses.
//
// The unexported Hub.loaded method (api.go) is unrelated: it reports
// whether the hook registry sealed at the end of Load. The loaded
// names live in the loadedNames field, because a field and a method
// may not share the name `loaded`.
func (h *Hub) Logger() *log.Logger {
	if h == nil {
		return nil
	}
	return h.logger
}

// newApi builds the Api one plugin registers against, delegating to
// the newApi constructor in api.go (which owns the Api type). The
// loader calls this once per plugin, before its Register runs, and
// reuses the same Api for its RegisterDeferred.
func (h *Hub) newApi(pluginName string) *Api {
	return newApi(h, pluginName)
}

// logf writes one hub log line through the injected logger. Callers
// include the "[plugins] " prefix in the format string, preserving
// the TS message shapes.
func (h *Hub) logf(format string, args ...any) {
	h.logger.Printf(format, args...)
}
