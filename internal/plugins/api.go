package plugins

import (
	"errors"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
)

// Api is one plugin's registration surface: the plugin name, the hub, and
// the hub's shared Core. The hub creates one per plugin at registration time
// via newApi, mirroring PluginManager.createApi (PluginManager.ts:2820+),
// which builds a fresh PluginApi object per plugin name. Api is the Go form
// of the PluginApi interface (PluginTypes.ts:999-1559) reduced to what this
// phase ports: the lifecycle hook registrations, custom-event subscribe and
// emit, per-plugin config reads, and the scoped Core. Everything else in
// PluginApi is listed, with the missing system named, in the unfired-family
// table in hooks.go.
//
// Hook-registration methods mirror the TS `typeof handler !== "function"`
// guards in createApi: a nil handler is ignored silently. Registration after
// Load() completes is ignored with a warning log: TS registers only during
// the load passes (loadPluginCandidatesWithDependencies,
// PluginManager.ts:2522-2568), so a post-load registration has no TS
// equivalent and is refused rather than silently accepted.
type Api struct {
	hub  *Hub
	name string
	core *Core
}

// newApi returns the Api for pluginName on hub. The loader worker calls it
// at registration time (for both the register and registerDeferred passes;
// TS reuses the same Api object for a plugin's deferred pass,
// PluginManager.ts:2582). A nil hub yields an Api whose methods are no-ops
// (GetPluginConfig returns the default, Core returns nil).
func newApi(h *Hub, pluginName string) *Api {
	a := &Api{hub: h, name: pluginName}
	if h != nil {
		a.core = h.core()
	}
	return a
}

// loaded reports whether the hub has finished Load(). It reads the seal in
// the hook registry (set at the end of emitPluginsLoaded, which Load()
// calls), so it needs no loader-owned Hub field beyond the contracted
// `hooks` field.
//
// Defined here (hooks worker) because the seal lives in the hook registry.
// Coordinator: if hub.go defines a method or field also named `loaded`,
// delete one side — a Hub field and a Hub method may not share the name.
func (h *Hub) loaded() bool {
	if h == nil || h.hooks == nil {
		return false
	}
	return h.hooks.isSealed()
}

// canRegister reports whether a hook registration from this Api may be
// stored: the hub must exist with a registry, and Load() must not have
// completed. A post-load attempt is logged with a warning and refused. The
// family argument is the TS registration name (e.g. "onPlayerLogin"), for
// greppability against the upstream source.
func (a *Api) canRegister(family string) bool {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return false
	}
	if a.hub.loaded() {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s: %s registration ignored (hub already loaded)", a.name, family)
		}
		return false
	}
	return true
}

// Name returns the owning plugin's name.
func (a *Api) Name() string {
	if a == nil {
		return ""
	}
	return a.name
}

// Core returns the hub's shared scoped Core (see core.go). It is the same
// instance for every plugin on the hub, mirroring the single frozen
// PluginCoreApi object TS builds once in getCoreApi
// (PluginManager.ts:2606+).
func (a *Api) Core() *Core {
	if a == nil {
		return nil
	}
	return a.core
}

// OnServerStartup registers a server-startup hook, mirroring onServerStartup
// (PluginTypes.ts:1010). The coordinator emits it once after the world is
// built and the engine starts (GameBuilder.ts:35). A nil handler is ignored
// silently, mirroring the createApi guard (PluginManager.ts:3257-3261).
func (a *Api) OnServerStartup(handler func(*ServerLifecycleEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onServerStartup") {
		return
	}
	a.hub.hooks.addServerStartup(hookEntry[*ServerLifecycleEvent]{pluginName: a.name, handler: handler})
}

// OnServerShutdown registers a server-shutdown hook, mirroring
// onServerShutdown (PluginTypes.ts:1011). The coordinator emits it on
// graceful shutdown (Server.ts:199). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3263-3267).
func (a *Api) OnServerShutdown(handler func(*ServerLifecycleEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onServerShutdown") {
		return
	}
	a.hub.hooks.addServerShutdown(hookEntry[*ServerLifecycleEvent]{pluginName: a.name, handler: handler})
}

// OnPlayerLogin registers a player-login hook, mirroring onPlayerLogin
// (PluginTypes.ts:1003). The coordinator emits it from enterWorld, after
// world entry and before the bootstrap (NetworkBuilder.ts:1694-1700). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3201-3212).
func (a *Api) OnPlayerLogin(handler func(*PlayerLoginEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerLogin") {
		return
	}
	a.hub.hooks.addPlayerLogin(hookEntry[*PlayerLoginEvent]{pluginName: a.name, handler: handler})
}

// OnPlayerLogout registers a player-logout hook, mirroring onPlayerLogout
// (PluginTypes.ts:1008). The coordinator emits it from the logout path,
// before the logout save (Player.ts:752-755). A nil handler is ignored
// silently, mirroring the createApi guard (PluginManager.ts:3229-3240).
func (a *Api) OnPlayerLogout(handler func(*PlayerLogoutEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerLogout") {
		return
	}
	a.hub.hooks.addPlayerLogout(hookEntry[*PlayerLogoutEvent]{pluginName: a.name, handler: handler})
}

// OnPlayerDisconnect registers a player-disconnect hook, mirroring
// onPlayerDisconnect (PluginTypes.ts:1007). The coordinator emits it from
// the remove-players phase, for non-bot registered players only
// (World.ts:1260-1264). A nil handler is ignored silently, mirroring the
// createApi guard (PluginManager.ts:3215-3226).
func (a *Api) OnPlayerDisconnect(handler func(*PlayerDisconnectEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerDisconnect") {
		return
	}
	a.hub.hooks.addPlayerDisconnect(hookEntry[*PlayerDisconnectEvent]{pluginName: a.name, handler: handler})
}

// OnPlayerProcess registers a per-player process hook, mirroring
// onPlayerProcess (PluginTypes.ts:1014). The coordinator emits it once per
// non-bot registered player per tick, after Player.Process
// (World.ts:1372). A nil handler is ignored silently, mirroring the
// createApi guard (PluginManager.ts:3297-3310).
func (a *Api) OnPlayerProcess(handler func(*PlayerProcessEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerProcess") {
		return
	}
	a.hub.hooks.addPlayerProcess(hookEntry[*PlayerProcessEvent]{pluginName: a.name, handler: handler})
}

// OnCommand registers an any-command hook, mirroring onCommand
// (PluginTypes.ts:1254). The hook observes EVERY command dispatch
// before the base handlers and the rights gate run; setting
// event.Handled claims the command and skips both (emitCommand,
// PluginManager.ts:2182-2189). The command registry fires these
// hooks through Hub.EmitCommandHooks (commands.go). A nil handler
// is ignored silently, mirroring the createApi guard
// (PluginManager.ts:4411-4414). The stored handler wraps the
// plugin's with the TS guard (PluginManager.ts:4418-4427): a nil
// event, an already-handled event, or an event with no player
// never reaches the plugin's handler. (The guard's raw/base/parts
// type checks have no Go form: the event type carries them.)
func (a *Api) OnCommand(handler func(*game.CommandEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCommand") {
		return
	}
	a.hub.hooks.addCommandHook(hookEntry[*game.CommandEvent]{pluginName: a.name, handler: func(event *game.CommandEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnSocialPacket registers a social-packet hook, mirroring
// onSocialPacket (PluginTypes.ts:1009). A hook that marks the
// event handled suppresses the engine's default handling of the
// packet — for public_chat, the broadcast
// (ChatPacketListener.ts:38-43); for the seven dispatch kinds
// the emit is the handling (NetworkBuilder.ts:894-911), so
// handled stops only later hooks. All eight packet kinds reach
// these hooks (see SocialPacketEvent): the chat listener fires
// public_chat and the social dispatch fires the rest, both
// through Hub.EmitSocialPacket (hooks.go). A nil handler is
// ignored silently, mirroring the createApi guard
// (PluginManager.ts:3243-3246). The stored handler wraps the
// plugin's with the TS guard (PluginManager.ts:3250-3252): a nil
// event, an already-handled event, or an event with no player
// never reaches the plugin's handler. (The guard's !packet check
// degenerates: Packet is a game.SocialPacket value field, always
// present.)
func (a *Api) OnSocialPacket(handler func(*SocialPacketEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onSocialPacket") {
		return
	}
	a.hub.hooks.addSocialPacket(hookEntry[*SocialPacketEvent]{pluginName: a.name, handler: func(event *SocialPacketEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// EmitCustomEvent emits a namespaced custom event to all subscribers, in
// subscription order, mirroring the PluginApi emitCustomEvent member
// (PluginTypes.ts:1409-1412) and PluginManager.emitCustomEvent
// (PluginManager.ts:1117-1136). Invalid names are silently ignored (see
// events.go). Emits are allowed at any time, including after Load().
func (a *Api) EmitCustomEvent(name string, payload any) {
	if a == nil || a.hub == nil {
		return
	}
	a.hub.EmitCustomEvent(name, payload)
}

// GetPluginConfig reads an option for this plugin, mirroring the PluginApi
// getPluginConfig member (PluginTypes.ts:1231-1235) and its createApi
// implementation (PluginManager.ts:4374-4375): a bare key (non-empty, no
// ":") is prefixed with the owning plugin's name via pluginConfigKey
// (config.go; WorldPluginConfig.ts:31-33, including the legacy namespace
// mapping); a key that already contains ":" is used as written. An empty
// key returns the default, mirroring the getPluginConfig guard
// (PluginManager.ts:2306-2315). The lookup itself delegates to
// Hub.GetPluginConfig, which reads the normalized world.json pluginConfig
// map (the Go config package already normalizes PluginConfig keys, so no
// normalization is duplicated here).
func (a *Api) GetPluginConfig(key string, def any) any {
	if key == "" || a == nil || a.hub == nil {
		return def
	}
	if !strings.Contains(key, ":") {
		key = pluginConfigKey(a.name, key)
	}
	return a.hub.GetPluginConfig(key, def)
}

// PersistAttribute registers a player attribute key for save/restore,
// mirroring the PluginApi persistAttribute member (PluginTypes.ts:1349-1351:
// "Save and restore this player attribute; values must be JSON-compatible.
// With a default, a value equal to it isn't saved, and a save without the
// key loads it.") and the createApi passthrough (PluginManager.ts:4616),
// which calls PlayerSave.persistAttribute directly. The passthrough has no
// load-phase guard, so this method has NO canRegister gate: calls are
// allowed any time, including after Load() completes.
//
// The default is variadic to mirror the optional TS parameter: zero or one
// default values. More than one panics: a programmer error with no TS
// equivalent. A single argument — even nil — counts as a default; Go has
// no undefined to mean "no default", and TS registers an explicit null
// default the same way (cloneAttribute, PlayerSave.ts:91).
//
// An error from the registry — a bad key (empty or whitespace-padded) or
// a default that is not JSON-compatible — panics, mirroring the TS throw
// (PlayerSave.persistAttribute, PlayerSave.ts:66-74). A panic during
// Register is recovered by the loader's registerCandidate
// (loader.go:264-270), which logs failed-to-initialize and drops the
// plugin, so a bad persistAttribute call during Register fails that
// plugin's load exactly as a TS throw does.
//
// A nil Api, or an Api with a nil hub, is a no-op: the nil check runs
// before the arity check, so even a malformed call on a nil Api does not
// panic. TS has no nil-Api case; this is the Go nil-safety convention of
// this file (see GetPluginConfig), not a TS behavior.
func (a *Api) PersistAttribute(key string, defaultValue ...any) {
	if a == nil || a.hub == nil {
		return
	}
	if len(defaultValue) > 1 {
		panic(errors.New("plugins: PersistAttribute accepts at most one default value"))
	}
	var def any
	hasDefault := len(defaultValue) == 1
	if hasDefault {
		def = defaultValue[0]
	}
	if err := persist.PersistAttribute(key, def, hasDefault); err != nil {
		panic(err)
	}
}

// RegisterCommand registers a command handler, mirroring the
// PluginApi registerCommand member (PluginTypes.ts:1261-1266) and
// its createApi implementation (PluginManager.ts:4450-4481),
// delegating to the hub's shared command registry
// (internal/game/commands.go), which ports the registration
// semantics exactly: names are trimmed and lowercased (no "::"
// stripping, no aliases), re-registration appends the handler
// and overwrites the rights floor, and descriptions are trimmed.
//
// minimumRights is the lowest PlayerRights id that may run the
// command (game.RightsNone and friends, PlayerRights.ts:2-6);
// the core denies everyone below the floor before the handler
// is called, so handlers never check rights themselves.
// RightsNone (0) means any player may run it: TS's omitted
// minimumRights (null) and NONE (0) are observably identical
// through every TS API (see the registry doc comment).
// description is shown and searched in the Commands interface.
//
// Invalid input — an empty name after normalization, or a nil
// handler — is silently ignored, mirroring the TS early returns
// (PluginManager.ts:4451-4457). registerCommand never throws,
// so unlike PersistAttribute it can never fail a plugin's load
// and the loader's recovery never sees it. Registration after
// Load() completes is ignored with a warning, like every other
// registration (see Api.canRegister). A nil Api, or an Api with
// a nil hub, is a no-op.
func (a *Api) RegisterCommand(command string, handler game.CommandHandler, minimumRights int, description string) {
	if handler == nil {
		return
	}
	if !a.canRegister("registerCommand") {
		return
	}
	if reg := a.hub.Commands(); reg != nil {
		reg.RegisterPlugin(a.name, command, handler, minimumRights, description)
	}
}

// GetRegisteredCommands returns the commands player may run,
// mirroring the PluginApi getRegisteredCommands member
// (PluginTypes.ts:1268) and its createApi passthrough
// (PluginManager.ts:4449), delegating to the hub's shared
// command registry. The list is rights-filtered (including
// world/plugin rank overrides), carries the first non-empty
// description per command, and is sorted by command name. A
// nil Api, a nil hub, or a nil player yields nil (a nil player
// reads as RightsNone through the registry's rights seam).
// This is a read, not a registration: it has no load-phase
// gate, mirroring the TS closure, which has no phase check.
func (a *Api) GetRegisteredCommands(player *game.Player) []game.RegisteredCommand {
	if a == nil || a.hub == nil {
		return nil
	}
	if reg := a.hub.Commands(); reg != nil {
		return reg.RegisteredCommands(player)
	}
	return nil
}

// SetCommandRights overrides the rank a command requires,
// whoever registered it, mirroring the PluginApi
// setCommandRights member (PluginTypes.ts:1273) and
// PluginManager.setCommandRights (PluginManager.ts:2158-2173),
// delegating to the hub's shared command registry. An empty
// name after normalization is silently ignored, mirroring the
// TS early return; the override wins over the registration
// floor but loses to world.json "commands:permissions".
// PlayerRights.NONE (game.RightsNone) opens the command to
// every player. Like GetRegisteredCommands, this is a state
// mutation with no load-phase gate: the TS closure has no
// phase check. A nil Api, or an Api with a nil hub, is a
// no-op.
func (a *Api) SetCommandRights(command string, minimumRights int) {
	if a == nil || a.hub == nil {
		return
	}
	if reg := a.hub.Commands(); reg != nil {
		reg.SetCommandRights(command, minimumRights)
	}
}
