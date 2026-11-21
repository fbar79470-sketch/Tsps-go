// Instancing hub surface: the PluginApi and PluginCoreApi members
// that let content query areas, replace map regions at runtime,
// and read template regions (phase 21, migration-plan milestone
// 11). This file owns ONLY the hub side; the engine behavior
// lives in internal/game (private_area.go,
// templated_instance_area.go, template_region.go,
// map_region_replacement.go).
//
// PORTED MEMBERS (PluginTypes.ts / PluginManager.ts):
//
//   - getAreaManager (PluginTypes.ts:1380; createApi,
//     PluginManager.ts:4671-4672): TS returns the AreaManager
//     class. Go returns the narrow AreaManagerQuery surface —
//     the query methods content needs (Get, Inside, InMulti) —
//     not the engine-internal Process/tick surface. The
//     *game.AreaManager satisfies it. Nil until the adapter
//     installs it through Hub.SetAreaManager.
//   - replaceMapRegion (PluginTypes.ts:1319;
//     PluginManager.ts:4510-4542): the .pack-file and
//     [terrain, object] pair forms arrive as two Go methods
//     (ReplaceMapRegion and ReplaceMapRegionFromFiles), because
//     Go has no string | [string, string] union. Both validate,
//     warn, and apply exactly as TS does (see the methods).
//   - replaceMapRegionData (PluginTypes.ts:1327;
//     PluginManager.ts:4543-4563): in-memory replacement with
//     the ownership rule — never stack over another owner's map.
//   - restoreMapRegion (PluginTypes.ts:1333;
//     PluginManager.ts:4564-4595): hand a region back to the
//     native map; the native packet is built FIRST, and without
//     one the replacement is kept.
//   - core.MapRegionReplacementManager (PluginTypes.ts:1705),
//     core.readTemplateRegion (PluginTypes.ts:1706), and
//     core.getTemplateRegion (PluginTypes.ts:1758): Core
//     accessors delegating to the installed manager and the
//     game template-region functions.
//
// APPLY SEMANTICS (applyPluginRegionReplacement,
// PluginManager.ts:217-262): after every successful replace,
// clipping reloads (failure logged, streaming continues) and
// the replacement streams to connected players; one info line
// summarizes the result. The reload and stream functions arrive
// through RegionReplacementWiring — the hub must not reach into
// the World or the session layer itself.
//
// NO NEW HOOK FAMILIES: grep over PluginTypes.ts and
// PluginManager.ts finds no onPrivateArea / onInstance /
// onTemplate / onArea registration surface. Private-area entry
// is the private-area:entered custom event, fired through the
// existing custom-event surface (see private_area.go); the
// firable-family count is unchanged by this file.
//
// NO CORE MIRROR for PrivateArea / TemplatedInstanceArea
// themselves, or for rotateChunkTile / rotateChunkLoc /
// templateChunkKey (PluginTypes.ts:1759-1761): content
// constructs areas through game.NewPrivateArea /
// game.NewTemplatedInstanceArea and calls the pure game
// functions directly, following the TimerKey precedent (a
// game-package value type with no Core mirror). Core stays a
// query surface.
//
// STORAGE: install state lives in a package-level sync.Map
// keyed by *Hub, NOT in hookRegistry fields — hooks.go is
// owned by another worker this phase, and this file must never
// edit it. The map entries are immutable snapshots replaced
// wholesale by the boot-only setters; getters Load them and
// are safe for concurrent use after boot. The coordinator may
// fold this state into hookRegistry later (the skills.go /
// objects.go precedent); the Hub/Api/Core method shapes would
// not change.

package plugins

import (
	"sync"

	"tsps-go/internal/game"
)

// AreaManagerQuery is the narrow AreaManager surface content
// reads through Api.GetAreaManager: the query methods of the TS
// AreaManager class (AreaManager.ts), excluding the
// engine-internal Process/tick plumbing, which stays the
// World's job. *game.AreaManager satisfies it.
type AreaManagerQuery interface {
	// Get returns the area whose boundaries contain the position
	// (highest-priority match wins), or nil.
	Get(position game.Location) *game.Area
	// Inside reports whether the position lies within the area's
	// boundaries.
	Inside(position game.Location, area *game.Area) bool
	// InMulti reports whether the actor stands in a multi-combat
	// area.
	InMulti(actor game.AreaActor) bool
}

// RegionReplacementWiring carries the engine functions the
// region-replacement Api members need. The coordinator installs
// it once at boot through Hub.SetRegionReplacementWiring (see
// cmd/server/instancingwiring.go).
type RegionReplacementWiring struct {
	// Manager is the world's MapRegionReplacementManager. The
	// replace/restore members are inert (warn, refuse) until it
	// is installed.
	Manager *game.MapRegionReplacementManager
	// ReloadRegion reloads one region's clipping after a
	// replace or restore, reporting whether the region was
	// reloaded (RegionManager.reloadRegion). Nil means clipping
	// never reloads; the apply step logs it as not reloaded and
	// continues, as TS does when the require fails.
	ReloadRegion func(regionID int) bool
	// StreamToConnected streams one region's replacement to
	// every connected player (the per-player visibility rule
	// lives in the game manager's SendReplacement) and reports
	// how many players received it. Nil streams to nobody.
	StreamToConnected func(regionID int) int
	// StreamPacketToConnected streams one replacement packet to
	// every connected player and reports whether every send
	// succeeded. Nil sends to nobody and reports success (the
	// no-connected-players shape). Used by restore.
	StreamPacketToConnected func(packet *game.RegionReplacementPacket) bool
}

// instancingInstall is one hub's installed instancing state: an
// immutable snapshot replaced wholesale by the setters.
type instancingInstall struct {
	areaManager  *game.AreaManager
	replacements RegionReplacementWiring
}

// instancingInstalls maps hubs to their installed instancing
// state. See the file header for why this lives here instead of
// in hookRegistry.
var instancingInstalls sync.Map // map[*Hub]*instancingInstall

func loadInstancing(h *Hub) *instancingInstall {
	if h == nil {
		return nil
	}
	if v, ok := instancingInstalls.Load(h); ok {
		return v.(*instancingInstall)
	}
	return nil
}

func storeInstancing(h *Hub, mutate func(*instancingInstall)) {
	if h == nil {
		return
	}
	next := &instancingInstall{}
	if cur := loadInstancing(h); cur != nil {
		*next = *cur
	}
	mutate(next)
	instancingInstalls.Store(h, next)
}

// SetAreaManager installs the engine AreaManager content reads
// through Api.GetAreaManager. The coordinator calls it once at
// boot with the World's instance (World.Areas()); calling it
// with nil clears the handle (GetAreaManager then returns nil).
// It is a no-op on a nil Hub.
func (h *Hub) SetAreaManager(am *game.AreaManager) {
	storeInstancing(h, func(in *instancingInstall) { in.areaManager = am })
}

// AreaManager returns the installed engine AreaManager, or nil
// when none is installed (or the Hub is nil). It mirrors
// Hub.ObjectManager: the read half of the install seam, for the
// coordinator and tests.
func (h *Hub) AreaManager() *game.AreaManager {
	if in := loadInstancing(h); in != nil {
		return in.areaManager
	}
	return nil
}

// SetRegionReplacementWiring installs the region-replacement
// wiring content's replace/restore members act through. The
// coordinator calls it once at boot; calling it with the zero
// value clears the wiring. It is a no-op on a nil Hub.
func (h *Hub) SetRegionReplacementWiring(w RegionReplacementWiring) {
	storeInstancing(h, func(in *instancingInstall) { in.replacements = w })
}

// RegionReplacementWiring returns the installed wiring, or the
// zero value when none is installed (or the Hub is nil).
func (h *Hub) RegionReplacementWiring() RegionReplacementWiring {
	if in := loadInstancing(h); in != nil {
		return in.replacements
	}
	return RegionReplacementWiring{}
}

// GetAreaManager returns the engine AreaManager as the narrow
// AreaManagerQuery surface, mirroring the PluginApi
// getAreaManager member (PluginTypes.ts:1380) and its createApi
// implementation (PluginManager.ts:4671-4672), which returns
// the AreaManager class. It returns nil until the adapter
// installs it through Hub.SetAreaManager: content must check
// for nil. This is a read, not a registration: it has no
// load-phase gate, mirroring the TS closure, which has no phase
// check. A nil Api, or an Api with a nil hub, yields nil.
func (a *Api) GetAreaManager() AreaManagerQuery {
	if a == nil || a.hub == nil {
		return nil
	}
	am := a.hub.AreaManager()
	if am == nil {
		return nil
	}
	return am
}

// ReplaceMapRegion replaces a region from a .pack file,
// mirroring the string form of replaceMapRegion
// (PluginTypes.ts:1319; PluginManager.ts:4510-4542). An invalid
// region id (outside 0..0xffff) warns and returns; a replace
// failure logs an error. On success it applies the replacement
// (reload clipping, stream to connected players). The pair
// form of the TS union source is ReplaceMapRegionFromFiles.
// With no manager installed it warns and returns (a Go-only
// guard: TS always has the manager). A nil Api, or an Api with
// a nil hub, is a silent no-op.
func (a *Api) ReplaceMapRegion(regionID int, source string) {
	if a == nil || a.hub == nil {
		return
	}
	if !validRegionID(regionID) {
		a.warnf("%s attempted invalid replaceMapRegion regionId=%d", a.name, regionID)
		return
	}
	mgr := a.hub.RegionReplacementWiring().Manager
	if mgr == nil {
		a.warnf("%s replaceMapRegion ignored (no region replacement manager installed)", a.name)
		return
	}
	result, err := mgr.ReplaceMapRegion(regionID, source)
	if err != nil {
		a.errorf("%s failed to replace map region %d: %v", a.name, regionID, err)
		return
	}
	a.applyRegionReplacement(regionID, result)
}

// ReplaceMapRegionFromFiles replaces a region from a
// [terrain, object] file pair, mirroring the pair form of the
// replaceMapRegion union source (PluginTypes.ts:1319;
// PluginManager.ts:4510-4542). Validation, warning, and apply
// semantics match ReplaceMapRegion.
func (a *Api) ReplaceMapRegionFromFiles(regionID int, terrainPath, objectPath string) {
	if a == nil || a.hub == nil {
		return
	}
	if !validRegionID(regionID) {
		a.warnf("%s attempted invalid replaceMapRegion regionId=%d", a.name, regionID)
		return
	}
	mgr := a.hub.RegionReplacementWiring().Manager
	if mgr == nil {
		a.warnf("%s replaceMapRegion ignored (no region replacement manager installed)", a.name)
		return
	}
	result, err := mgr.ReplaceMapRegionFromFiles(regionID, terrainPath, objectPath)
	if err != nil {
		a.errorf("%s failed to replace map region %d: %v", a.name, regionID, err)
		return
	}
	a.applyRegionReplacement(regionID, result)
}

// ReplaceMapRegionData replaces a region with in-memory map
// data (decoded terrain and loc files), mirroring
// replaceMapRegionData (PluginTypes.ts:1327;
// PluginManager.ts:4543-4563). It returns false when refused:
// an invalid region id, empty terrain data, a region owned by
// another plugin (never stack over another owner's map —
// restoring would lose it), no installed manager, or a replace
// failure. objectData may be nil (TS null: terrain-only
// replacement); an empty non-nil slice is accepted, as TS
// accepts an empty Uint8Array for objects. On success it
// applies the replacement and returns true. The replacement is
// registered under this plugin's name, so only it may restore
// the region later.
func (a *Api) ReplaceMapRegionData(regionID int, terrainData, objectData []byte) bool {
	if a == nil || a.hub == nil {
		return false
	}
	if !validRegionID(regionID) || len(terrainData) == 0 {
		a.warnf("%s attempted invalid replaceMapRegionData regionId=%d", a.name, regionID)
		return false
	}
	mgr := a.hub.RegionReplacementWiring().Manager
	if mgr == nil {
		a.warnf("%s replaceMapRegionData ignored (no region replacement manager installed)", a.name)
		return false
	}
	if owner, ok := mgr.GetSource(regionID); ok && owner != a.name {
		a.warnf("%s refused: region %d is replaced by %s", a.name, regionID, owner)
		return false
	}
	result, err := mgr.ReplaceMapRegionData(regionID, terrainData, objectData, a.name)
	if err != nil {
		a.errorf("%s failed to replace map region %d: %v", a.name, regionID, err)
		return false
	}
	a.applyRegionReplacement(regionID, result)
	return true
}

// RestoreMapRegion drops this plugin's runtime replacement,
// mirroring restoreMapRegion (PluginTypes.ts:1333;
// PluginManager.ts:4564-4595). Only the plugin that replaced a
// region may hand it back: any other owner (or no replacement
// at all) returns false silently. The native map packet is
// built FIRST — without one, the replacement is kept so the
// server's clipping and every client still agree. After
// removal, clipping reloads and every connected client gets
// the native map; one failed send does not stop the rest, and
// the return reports whether every send succeeded.
func (a *Api) RestoreMapRegion(regionID int) bool {
	if a == nil || a.hub == nil {
		return false
	}
	wiring := a.hub.RegionReplacementWiring()
	mgr := wiring.Manager
	if mgr == nil {
		return false
	}
	// Only the plugin that replaced a region may hand it back.
	owner, ok := mgr.GetSource(regionID)
	if !ok || owner != a.name {
		return false
	}
	// Build the native map first: without one, keep the
	// replacement so the server's clipping and every client
	// still agree.
	packet, err := mgr.NativeRegionPacket(regionID)
	if err != nil {
		a.errorf("%s could not encode native map region %d: %v", a.name, regionID, err)
		return false
	}
	if packet == nil {
		a.warnf("%s kept region %d: the cache has no native map for it", a.name, regionID)
		return false
	}
	mgr.RemoveMapRegion(regionID)
	if wiring.ReloadRegion != nil {
		wiring.ReloadRegion(regionID)
	}
	// Every connected client gets the native map; one failed
	// send does not stop the rest.
	delivered := true
	if wiring.StreamPacketToConnected != nil {
		delivered = wiring.StreamPacketToConnected(packet)
	}
	return delivered
}

// applyRegionReplacement mirrors applyPluginRegionReplacement
// (PluginManager.ts:217-262): reload clipping (failure logged
// by the absence of the reloaded flag — TS logs only on a
// throw, which Go's bool-returning reload cannot produce —
// streaming continues either way), stream the replacement to
// connected players, and log one summary line.
func (a *Api) applyRegionReplacement(regionID int, result game.ReplaceMapRegionResult) {
	wiring := a.hub.RegionReplacementWiring()
	clippingReloaded := false
	if wiring.ReloadRegion != nil {
		clippingReloaded = wiring.ReloadRegion(regionID)
	}
	streamedPlayers := 0
	if wiring.StreamToConnected != nil {
		streamedPlayers = wiring.StreamToConnected(regionID)
	}
	if logger := a.hub.Logger(); logger != nil {
		logger.Printf("[plugins] %s replaced map region regionId=%d source=%s terrainBytes=%d objectBytes=%d objectCount=%d clippingReloaded=%v streamedPlayers=%d",
			a.name, regionID, result.Source, result.TerrainBytes, result.ObjectBytes, result.ObjectCount, clippingReloaded, streamedPlayers)
	}
}

// validRegionID mirrors the TS region-id guard
// (PluginManager.ts:4511, 4545): an integer in 0..0xffff. Go
// ints are always integers, so only the range is checked.
func validRegionID(regionID int) bool {
	return regionID >= 0 && regionID <= 0xffff
}

// warnf logs a plugin warning through the hub logger, mirroring
// the TS console.warn calls in the createApi closures. It is a
// no-op when the Api, hub, or logger is nil.
func (a *Api) warnf(format string, args ...any) {
	if a == nil || a.hub == nil {
		return
	}
	if logger := a.hub.Logger(); logger != nil {
		logger.Printf("[plugins] "+format, args...)
	}
}

// errorf logs a plugin error through the hub logger, mirroring
// the TS console.error calls. It is a no-op when the Api, hub,
// or logger is nil.
func (a *Api) errorf(format string, args ...any) {
	if a == nil || a.hub == nil {
		return
	}
	if logger := a.hub.Logger(); logger != nil {
		logger.Printf("[plugins] "+format, args...)
	}
}

// MapRegionReplacementManager returns the installed region
// replacement manager: the core.MapRegionReplacementManager
// equivalent (PluginTypes.ts:1705), installed by the
// coordinator through Hub.SetRegionReplacementWiring. Nil when
// no wiring is installed (or the Core is nil).
func (c *Core) MapRegionReplacementManager() *game.MapRegionReplacementManager {
	if c == nil || c.hub == nil {
		return nil
	}
	return c.hub.RegionReplacementWiring().Manager
}

// ReadTemplateRegion decodes a template region fresh from the
// cache, mirroring core.readTemplateRegion
// (PluginTypes.ts:1706; TemplateRegionData.ts readTemplateRegion).
// It bypasses the decode-once cache: every call re-reads and
// re-decodes. It returns nil when the region has no map data
// or no template source is installed. See
// game.ReadTemplateRegion.
func (c *Core) ReadTemplateRegion(regionID int) *game.TemplateRegion {
	return game.ReadTemplateRegion(regionID)
}

// GetTemplateRegion returns the decode-once cached template
// region, mirroring core.getTemplateRegion
// (PluginTypes.ts:1758; TemplateRegionData.ts getTemplateRegion).
// It returns nil when the region has no map data or no template
// source is installed. See game.GetTemplateRegion.
func (c *Core) GetTemplateRegion(regionID int) *game.TemplateRegion {
	return game.GetTemplateRegion(regionID)
}
