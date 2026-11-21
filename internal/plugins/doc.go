// Package plugins is the Go plugin hub: the compile-time analogue of
// PluginManager (server/src/main/typescript/elvarg/plugins/PluginManager.ts,
// TSPS server commit bfd81e3). Content is ported to Go packages that
// register a Plugin value; the hub validates, orders, and runs their
// Register functions at boot, then dispatches engine events to the hooks
// they registered. The TypeScript client is unaffected: this package is
// server-only, as is the whole port.
//
// The hub is split across two halves. The loader half (this file plus
// plugin.go, hub.go, loader.go, config.go, switches.go) owns plugin
// identity, world.json switches, config declarations, dependency-ordered
// registration, and the load log. The hooks half (api.go, hooks.go,
// events.go, gates.go, core.go, login.go) owns the Api surface plugins
// code against, the hook registry, event payloads, gate contracts, the
// Core access surface, and login extensions. The seam between the halves
// is small: the loader holds a hookRegistry created by newHookRegistry,
// builds one Api per plugin through newApi, and calls
// Hub.emitPluginsLoaded at the end of Load. Which hook families exist,
// which of them the engine can fire today, and which are
// registered-but-unfired is documented with the hooks implementation;
// the consolidated table lives in PORT_STATUS.md (coordinator-owned).
// This file documents the loader only.
//
// COMPILE-TIME REGISTRATION — the deliberate deviation. TypeScript
// discovers *.plugin.js files under server/plugins/ at boot (up to
// MAX_PLUGIN_DEPTH = 2), require()s each one, and reads its exports.
// Go has no equivalent: content packages register themselves with
// Register, typically from init or from explicit wiring in cmd/server,
// and the hub loads from the explicit list NewHub receives. There is no
// directory scan, no file evaluation, and no hot reload; adding content
// means rebuilding the server.
//
// Gains: registration is type-checked at compile time, so a malformed
// plugin fails the build instead of failing at boot; load order is the
// deterministic registration order instead of filesystem order; there is
// no eval surface and no per-file require failure mode; dead content can
// be dropped by deleting an import. Losses: content can no longer be
// added by dropping a file into a directory; a compile error in any one
// content package breaks the whole build (TS isolates failures per
// file); the filename-derived identity TS uses (pluginNameFromFile) has
// to be carried explicitly as Plugin.ID; and there is no depth limit or
// discovery log, because there is nothing to discover.
//
// LOAD ORDER. Hub.Load mirrors loadFromDirectory
// (PluginManager.ts:681-793) with the discovery steps removed: the
// legacy-switch deprecation warning; the filename-switch pre-filter
// (by ID, before candidates are collected); unknown-switch warnings;
// the exported-name switch resolution plus members-only gating;
// config-declaration validation before ANY registration; multi-pass
// dependency-ordered registration with deferred re-queues; recording of
// the loaded plugins' declarations; warnings about config values that
// do not fit their declarations; the plugins:loaded emit; and the
// active/disabled/members-only counts log. Load is idempotent: a second
// call is a no-op. Every step logs through the injected *log.Logger
// with the TS "[plugins] ..." message shapes; the info/warn/error
// distinction in TS console output is not preserved, because the hub
// has a single logger.
//
// IDENTITY AND DEPENDENCIES. A plugin's exported name (Plugin.Name) is
// what players and other plugins see; its ID (Plugin.ID, defaulting to
// Name) is the stable key that plays the role of the TS filename in
// switch resolution. Switch lookups normalize both sides with
// NormalizeName (lowercase, strip non-alphanumeric). Dependency
// matching is by EXACT unnormalized name against loaded names, as in
// TS: a dependency on "base" does not match a plugin named "Base".
// DependsOn entries are trimmed, blank entries and self-dependencies
// are dropped with warnings, and duplicates are dropped keeping first
// order (normalizePluginDependencies, PluginManager.ts:2476-2520).
//
// CONFIG DECLARATIONS. Plugins declare the world.json pluginConfig
// options they read as Config values keyed "namespace:key". Declarations
// are validated before any registration; a bad or clashing declaration
// is logged and dropped and never stops the server. The Go core
// declaration set is EMPTY (see config.go): the core reads no
// pluginConfig options today, so no key is core-owned. Between plugins,
// the first claimant is expected to own a clashing key and ownership
// goes to the first of them that actually loads. The hub reads config
// values verbatim from the normalized map internal/config builds; it
// never re-normalizes keys and never coerces values at read time.
//
// SCOPE. This package ports the PluginManager/PluginApi SHAPE, not the
// engine systems content hooks into. Combat, dialogue, shops, trading,
// chat, NPC drivers, and the rest do not exist in Go yet, so most hook
// families have no production emit site; they are registered-but-unfired
// until their system lands, and families that cannot even be honestly
// typed today get no registration method at all. No content plugin is
// ported here beyond what the coordinator wires as the proof piece, and
// no embedded JS runtime or Node sidecar exists: both were ruled out by
// the decided Go-native strategy, and this package must never grow one.
package plugins
