// Area registration and the isolating area-method caller: the Go form of
// registerArea (PluginTypes.ts:1382) and PluginManager.callArea
// (PluginManager.ts:481-512).
//
// registerArea (PluginManager.ts:3523-3527) sets the area's pluginName (and
// profileName, which has no Go equivalent — see Area.SetPluginName) and
// pushes the area onto the AreaManager registry. TS pushes onto a static
// array that always exists. Go cannot: plugin Register functions run during
// Hub.Load, which the coordinator runs BEFORE the world (and its
// AreaManager) is built, so the registry does not exist yet at registration
// time. Api.RegisterArea therefore QUEUES areas registered before
// installation and flushes them in registration order when the coordinator
// installs the registry with Hub.SetAreaRegistry. Queueing — not an error,
// not a panic, not silent loss — is the only behavior that preserves the TS
// outcome (the area ends up registered, named for its plugin) across the
// load-before-world ordering. Registration order is priority order in
// AreaManager.Get, so the flush preserves the order areas were registered,
// and areas registered after installation push directly, after the flushed
// ones.
//
// The registry seam follows the ObjectManagerAPI precedent (objects.go):
// AreaRegistry is a narrow interface declared here, in the plugins package,
// so game never imports plugins; the coordinator's cmd/server adapter
// implements it over the concrete *game.AreaManager. A nil registry means
// uninstalled: RegisterArea queues, and SetAreaRegistry(nil) clears the
// installed registry while KEEPING the queue — a later install still flushes
// everything, so no area is ever lost to an install/clear cycle.
//
// callArea runs one area method the way hooks run: under the registering
// plugin's context, timed under pluginperf when enabled, and a throw is
// logged instead of escaping into the actor's tick (where it would log a
// player out), returning null (PluginManager.ts:481-512). The engine
// exposes the seam as game.AreaMethodCaller + game.SetAreaMethodCaller
// (area.go); Hub.AreaMethodCaller returns the isolating implementation.
// No perf timing is ported (pluginperf is not ported — the existing hub
// precedent, see executeHook), and there is no PluginContext in Go; the
// log line labels the call with the area's plugin name (falling back to
// the area name), matching the TS label expression
// (area.pluginName ?? area.getName(), PluginManager.ts:487, 500).
//
// AUDIT (phase 15, timer/area members of PluginApi/PluginCoreApi):
//
//   - getAreaManager (PluginTypes.ts:1380) stayed table-only
//     through Phase 15: it returns the AreaManager class
//     itself — the full static surface, including the
//     engine-internal Process tick plumbing and the mutable
//     areas array. Phase 21 ports it as Api.GetAreaManager
//     (instancing.go), returning the narrow AreaManagerQuery
//     (get/inside/inMulti) installed through
//     Hub.SetAreaManager; Process stays engine-internal.
//     The registration use is covered by Api.RegisterArea.
//   - Timer access has NO PluginApi surface. getTimers is a Mobile method
//     only (Mobile.ts:963-965, ported as Mobile.GetTimers); content
//     reaches timers through the *game.Player on event payloads, like
//     attributes (Api.PersistAttribute aside) and the dialogue manager.
//     PluginCoreApi carries TimerKey and SecondsTimer (PluginTypes.ts:1636,
//     1638), but Go content imports the game package directly and reads
//     the TimerKey constants there; no Core mirror is needed, and both
//     stay in core.go's NOT-mirrored list.
//   - setLevelSource is a Wilderness static (Wilderness.ts:16-19), with no
//     PluginApi member; the Go Wilderness.SetLevelSource exists and is
//     reached through the world's Wilderness instance, not the hub.
//     core.Wilderness, core.Boundary, core.PolygonalBoundary, core.Area,
//     core.PrivateArea, and core.TemplatedInstanceArea stay in core.go's
//     NOT-mirrored list: Go content constructs areas and boundaries with
//     game.NewArea / game.NewBoundary directly, and milestone 11 adds
//     no Core mirror for the area classes either.

package plugins

import (
	"log"

	"tsps-go/internal/game"
)

// AreaRegistry is the narrow area-registry surface Api.RegisterArea pushes
// onto: the Go form of the AreaManager.areas push in registerArea
// (PluginManager.ts:3523-3527). TS pushes onto the AreaManager static array;
// Go pushes through this interface, installed by the coordinator at boot
// from the concrete registry (phase 15, Workers A/B) behind a thin adapter
// in cmd/server: the adapter only forwards Register to
// *game.AreaManager.Register. The interface carries Register alone: content
// registers areas, it never queries the registry (get/inside/inMulti belong
// to the getAreaManager query surface — see Api.GetAreaManager in
// instancing.go), and there is no deregistration in TS, so none is
// offered here.
type AreaRegistry interface {
	// Register appends area to the registry. Registration order is
	// priority order (AreaManager.Register, areamanager.go).
	Register(area *game.Area)
}

// SetAreaRegistry installs the AreaRegistry Api.RegisterArea pushes onto,
// flushing every area queued before installation, in registration order.
// The coordinator calls it once at boot, after the world's AreaManager
// exists and before ticking starts; calling it with nil clears the
// installed registry but keeps the queue (a later non-nil install flushes
// it). It is a no-op on a nil Hub or a Hub without a registry. The flush
// runs outside the registry lock: Register implementations are
// coordinator adapters over the game registry, and holding the hub lock
// across them would serialize boot wiring behind hook registration.
func (h *Hub) SetAreaRegistry(reg AreaRegistry) {
	if h == nil || h.hooks == nil {
		return
	}
	queued := h.hooks.installAreaRegistry(reg)
	for _, area := range queued {
		reg.Register(area)
	}
}

// AreaRegistry returns the installed AreaRegistry, or nil when none is
// installed (or the Hub is nil). It mirrors Hub.ObjectManager: the read
// half of the install seam, for the coordinator and tests.
func (h *Hub) AreaRegistry() AreaRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedAreaRegistry()
}

// RegisterArea adds area to the AreaManager registry, mirroring the
// PluginApi registerArea member (PluginTypes.ts:1382) and its createApi
// implementation (PluginManager.ts:3523-3527): the area's plugin name is
// set to the registering plugin's name (via Area.SetPluginName — the
// profileName assignment has no Go equivalent, as area.go documents),
// then the area is pushed onto the registry. With no registry installed
// yet — the normal case during Hub.Load, which runs before the world is
// built — the area is queued and flushed in order by SetAreaRegistry;
// it is never lost and never silently dropped.
//
// RegisterArea has NO load-phase gate, unlike the hook registrations:
// the TS closure has no phase check, and both load-time registration
// (queued) and later registration (direct) are legitimate — TS pushes
// onto the static array whenever it is called. A nil Api, or an Api with
// a nil hub, is a no-op (the nil-safety convention of this package's Api
// methods). A nil area PANICS, mirroring the TS throw: the TS closure
// assigns area.pluginName with no guard, so a null area throws a
// TypeError; a panic during Register is recovered by the loader, which
// logs failed-to-initialize and drops the plugin, exactly as a TS throw
// fails that plugin's load.
func (a *Api) RegisterArea(area *game.Area) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if area == nil {
		panic("plugins: RegisterArea called with a nil area")
	}
	area.SetPluginName(a.name)
	if reg := a.hub.hooks.areaRegistrationTarget(area); reg != nil {
		reg.Register(area)
	}
}

// isolatingAreaCaller is the hub's game.AreaMethodCaller: it runs each area
// method with per-call isolation, porting PluginManager.callArea
// (PluginManager.ts:481-512) without the perf branch. A panicking method is
// recovered, logged as "[plugins] area <method> failed (<pluginName or area
// name>)" plus the recovered value, and the caller receives the zero value
// (MusicTrack: no opinion) — the Go form of callArea's catch-and-return-null.
// The method names in the log lines are the TS callArea method strings:
// "enter", "leave", "process", "getMusicTrack" (the call sites in Area.ts,
// AreaManager.ts, and PlayerSession.ts pass those exact strings).
type isolatingAreaCaller struct {
	logger *log.Logger
}

// Compile-time proof that the isolating caller satisfies the engine seam.
var _ game.AreaMethodCaller = isolatingAreaCaller{}

// AreaMethodCaller returns the isolating game.AreaMethodCaller the engine
// routes area method calls through. The coordinator installs it at boot
// with game.SetAreaMethodCaller(hub.AreaMethodCaller()), before the world
// starts ticking. It is safe to call on a nil Hub: the caller then logs
// through log.Default(), matching executeHook's nil-logger fallback.
func (h *Hub) AreaMethodCaller() game.AreaMethodCaller {
	return isolatingAreaCaller{logger: h.Logger()}
}

// areaLabel answers the callArea failure label for area:
// area.pluginName ?? area.getName() (PluginManager.ts:487, 500). A nil area
// labels "": no TS call site passes a null area (the music site guards
// with ?. and AreaManager only calls with registered areas), so a nil
// area here is a Go-only defensive path — the recovered panic is still
// logged, with an empty label, rather than panicking while logging.
func (c isolatingAreaCaller) areaLabel(area *game.Area) string {
	if area == nil {
		return ""
	}
	if name := area.PluginName(); name != "" {
		return name
	}
	return area.GetName()
}

// fail logs one isolated area-method failure through the hub logger,
// falling back to log.Default() when none was injected.
func (c isolatingAreaCaller) fail(method string, area *game.Area, recovered any) {
	logger := c.logger
	if logger == nil {
		logger = log.Default()
	}
	logger.Printf("[plugins] area %s failed (%s): %v", method, c.areaLabel(area), recovered)
}

// Enter runs area.Enter with isolation (callArea "enter").
func (c isolatingAreaCaller) Enter(area *game.Area, actor game.AreaActor) {
	defer func() {
		if r := recover(); r != nil {
			c.fail("enter", area, r)
		}
	}()
	area.Enter(actor)
}

// Leave runs area.Leave with isolation (callArea "leave").
func (c isolatingAreaCaller) Leave(area *game.Area, actor game.AreaActor, logout bool) {
	defer func() {
		if r := recover(); r != nil {
			c.fail("leave", area, r)
		}
	}()
	area.Leave(actor, logout)
}

// Process runs area.Process with isolation (callArea "process").
func (c isolatingAreaCaller) Process(area *game.Area, actor game.AreaActor) {
	defer func() {
		if r := recover(); r != nil {
			c.fail("process", area, r)
		}
	}()
	area.Process(actor)
}

// MusicTrack runs area.GetMusicTrack with isolation (callArea
// "getMusicTrack"). On failure it returns (0, false) — no opinion — the
// Go form of the TS null return, meaning the normal map/instance track
// is retained (see Area.GetMusicTrack).
func (c isolatingAreaCaller) MusicTrack(area *game.Area, player *game.Player) (track int, ok bool) {
	defer func() {
		if r := recover(); r != nil {
			c.fail("getMusicTrack", area, r)
			track, ok = 0, false
		}
	}()
	return area.GetMusicTrack(player)
}
