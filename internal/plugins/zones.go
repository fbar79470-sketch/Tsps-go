// Zone hooks: the Go form of onZoneEnter/onZoneExit (PluginTypes.ts:1015-1018),
// driven by updatePlayerZones from emitPlayerProcess
// (PluginManager.ts:881-889 and 892-951; registration at 3311-3321).
//
// TS keeps ONE list, zoneHooks (PluginManager.ts:326-331): each entry carries
// a zone plus ONLY onEnter or ONLY onExit. A plugin registering both gets two
// independent entries, and the per-player active sets are keyed by hook INDEX
// into that list — two entries with equal rectangles track independently.
// Go mirrors the shape exactly: one zoneHooks list of zoneHook entries, each
// with exactly one handler set, and active sets of hook indexes.
//
// updatePlayerZones runs FIRST inside emitPlayerProcess, BEFORE the process
// hooks, and it runs even when there are zero process hooks: it returns early
// only when there are zero zone hooks or no player/location
// (PluginManager.ts:893-899). Hub.EmitPlayerProcess (hooks.go) calls
// updatePlayerZones in that same position; the Go location guard degenerates
// (see isInsideZone).
//
// Error isolation follows the dialogue precedent, not executeHook: TS catches
// per hook and warns "[plugins] zone enter hook threw (<pluginName>)" /
// "[plugins] zone exit hook threw (<pluginName>)"
// (PluginManager.ts:918, 930), and the update continues with the next
// hook. There is no PluginContext in Go (no thread-locals); the plugin name
// in the log line carries the attribution, as everywhere else in the hub.
//
// Active-set storage: TS uses a WeakMap keyed by player
// (PluginManager.ts:332), so a departed player's entry dies with the player
// object and a relogin — a new player object — starts with no active zones.
// Go has no weak map: activeZones is an ordinary map keyed by player pointer,
// and Hub.ForgetPlayerZones discards the entry. EmitPlayerDisconnect calls it:
// the disconnect emit is the removal-phase path (World.ts:1260-1264), the
// moment the player object leaves the world. EmitPlayerLogout does NOT
// discard: logout is not object death in TS either (the WeakMap entry lives
// until the object is collected), and a relogin creates a new *Player, so a
// fresh login always starts with an empty set. No exit hooks fire on
// disconnect: TS never calls updatePlayerZones there, it just lets the entry
// die — discarding silently is the faithful behavior.

package plugins

import (
	"log"

	"tsps-go/internal/game"
)

// Zone mirrors PluginZone (PluginTypes.ts:284-291): a rectangular player
// zone. Levels omitted means all planes; in Go, a nil Levels slice
// means all planes, and a non-nil slice restricts to the listed
// levels — including an explicitly empty slice, which matches NO
// plane, exactly as TS treats an empty levels array ([] is truthy,
// so !zone.levels is false, and [].includes(z) is false;
// PluginManager.ts:950). The nil/empty distinction is observable
// in Go (Levels == nil), so the TS reading ports exactly.
//
// No rectangle validation exists, mirroring TS: registration accepts any
// rectangle, including an inverted one (which simply never matches, since
// the inclusive comparisons fail).
type Zone struct {
	MinX int
	MaxX int
	MinY int
	MaxY int
	// Levels restricts the zone to these height levels. Nil
	// means all planes; an empty non-nil slice matches none.
	Levels []int
}

// ZoneEvent mirrors PluginZoneEvent (PluginTypes.ts:293-296): the player
// and the zone entered or left. Zone is the registered *Zone pointer, as
// TS passes the registered zone object (hook.zone); a handler that mutates
// it mutates the registered zone, exactly as in TS.
type ZoneEvent struct {
	Player *game.Player
	Zone   *Zone
}

// zoneHook is one entry in the zone hook list: the owning plugin's name,
// the zone, and exactly ONE of onEnter/onExit — the Go form of the
// zoneHooks element type (PluginManager.ts:326-331). Registration sets
// only one field; the other stays nil, and updatePlayerZones checks for
// its presence before firing, as TS does (hook.onEnter / hook.onExit).
type zoneHook struct {
	pluginName string
	zone       *Zone
	onEnter    func(*ZoneEvent)
	onExit     func(*ZoneEvent)
}

// zoneTransition is one state change updatePlayerZones must deliver: the
// hook whose active state flipped, and the direction. Transitions are
// computed and applied to the active set under the registry lock, then
// fired outside it (see updatePlayerZones): a handler runs without the
// lock held, matching the hub's emit convention, while the state change
// is already recorded — as in TS, where active.add/delete runs before
// the handler call (PluginManager.ts:912, 924).
type zoneTransition struct {
	hook  zoneHook
	enter bool
}

// OnZoneEnter registers a zone-enter hook, mirroring onZoneEnter
// (PluginTypes.ts:1016). Each call pushes ONE entry carrying ONLY onEnter
// (PluginManager.ts:3311-3316); registering both enter and exit for the
// same zone pushes TWO independent entries, tracked by separate hook
// indexes. A nil zone or a nil handler is silently ignored, mirroring the
// createApi guard (PluginManager.ts:3312: typeof handler !== "function"
// || !zone). The zone pointer is stored as given: later mutation of the
// caller's Zone is visible to the driver, as in TS, which stores the
// zone object reference.
func (a *Api) OnZoneEnter(zone *Zone, handler func(*ZoneEvent)) {
	if handler == nil || zone == nil {
		return
	}
	if !a.canRegister("onZoneEnter") {
		return
	}
	a.hub.hooks.addZoneHook(zoneHook{pluginName: a.name, zone: zone, onEnter: handler})
}

// OnZoneExit registers a zone-exit hook, mirroring onZoneExit
// (PluginTypes.ts:1018). Each call pushes ONE entry carrying ONLY onExit
// (PluginManager.ts:3317-3321); see OnZoneEnter for the independence and
// guard semantics, which are identical.
func (a *Api) OnZoneExit(zone *Zone, handler func(*ZoneEvent)) {
	if handler == nil || zone == nil {
		return
	}
	if !a.canRegister("onZoneExit") {
		return
	}
	a.hub.hooks.addZoneHook(zoneHook{pluginName: a.name, zone: zone, onExit: handler})
}

// isInsideZone ports PluginManager.isInsideZone (PluginManager.ts:939-951):
// the inclusive rectangle test (x >= minX && x <= maxX && y >= minY &&
// y <= maxY) plus the levels test (levels omitted = all planes, else the
// level must be included). A nil zone never matches (TS: if (!zone)
// return false — unreachable through registration, which rejects nil
// zones, but kept for parity).
//
// Two Go-only readings: TS reads the coordinates through getX/getY/getZ
// with ?? fallbacks to .x/.y/.z and 0; Go reads the Location value's
// fields directly — the fallback chain has no Go form, because a Location
// always carries all three coordinates. And TS returns early when
// player.getLocation() yields nothing (PluginManager.ts:896-899); Go
// Mobile.Location returns a value, never nothing, so that guard
// degenerates to always-present — the zero Location (0,0,0) is a real
// tile and zones covering it match it.
//
// The levels test ports TS exactly: a nil Levels slice (levels
// omitted) matches all planes; a non-nil slice must include the
// level, so an explicitly empty slice matches no plane (TS:
// !zone.levels is false for [], and [].includes(z) is false).
func isInsideZone(zone *Zone, loc game.Location) bool {
	if zone == nil {
		return false
	}
	if !(loc.X >= zone.MinX && loc.X <= zone.MaxX && loc.Y >= zone.MinY && loc.Y <= zone.MaxY) {
		return false
	}
	if zone.Levels == nil {
		return true
	}
	for _, level := range zone.Levels {
		if level == loc.Z {
			return true
		}
	}
	return false
}

// zoneTransitions snapshots the zone hooks, computes the enter/exit
// transitions for player at loc against the player's active set, applies
// the state changes, and returns the transitions that have a handler to
// fire, in hook-index order. It ports the loop body of updatePlayerZones
// (PluginManager.ts:905-936): inside === active means no transition;
// otherwise the set flips and the matching handler (if any) fires. The
// whole computation runs under the registry lock, so the active set can
// never observe a half-applied update; handlers run later, outside the
// lock (see updatePlayerZones).
//
// With zero zone hooks it returns nil WITHOUT creating an active set,
// mirroring the TS early return (PluginManager.ts:893). With hooks
// present it creates the player's set on first use, mirroring the
// WeakMap get-or-create (PluginManager.ts:900-904) — even when this call
// produces no transitions, the set exists afterwards, as in TS.
func (r *hookRegistry) zoneTransitions(player *game.Player, loc game.Location) []zoneTransition {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.zoneHooks) == 0 {
		return nil
	}
	active := r.activeZones[player]
	if active == nil {
		active = make(map[int]struct{})
		if r.activeZones == nil {
			r.activeZones = make(map[*game.Player]map[int]struct{})
		}
		r.activeZones[player] = active
	}
	var transitions []zoneTransition
	for index, hook := range r.zoneHooks {
		inside := isInsideZone(hook.zone, loc)
		_, wasActive := active[index]
		if inside == wasActive {
			continue
		}
		if inside {
			active[index] = struct{}{}
			if hook.onEnter != nil {
				transitions = append(transitions, zoneTransition{hook: hook, enter: true})
			}
		} else {
			delete(active, index)
			if hook.onExit != nil {
				transitions = append(transitions, zoneTransition{hook: hook, enter: false})
			}
		}
	}
	return transitions
}

// updatePlayerZones drives onZoneEnter/onZoneExit for one player from
// their current location, porting PluginManager.updatePlayerZones
// (PluginManager.ts:892-937). EmitPlayerProcess calls it FIRST, before
// the process hooks (PluginManager.ts:881-882). A nil player runs
// nothing (the TS !player guard); the TS !location guard degenerates —
// see isInsideZone. Each firing builds a fresh ZoneEvent, as TS builds
// a fresh {player, zone} object per hook call. A throwing hook is
// warned and the update continues (see runZoneHook).
func (h *Hub) updatePlayerZones(player *game.Player) {
	if h == nil || h.hooks == nil || player == nil {
		return
	}
	for _, transition := range h.hooks.zoneTransitions(player, player.Location()) {
		event := &ZoneEvent{Player: player, Zone: transition.hook.zone}
		if transition.enter {
			runZoneHook(h.Logger(), "zone enter", transition.hook, event, transition.hook.onEnter)
		} else {
			runZoneHook(h.Logger(), "zone exit", transition.hook, event, transition.hook.onExit)
		}
	}
}

// runZoneHook runs one zone handler with the TS warn-and-continue shape:
// a panicking handler is recovered, logged as "[plugins] zone enter hook
// threw (<pluginName>)" / "[plugins] zone exit hook threw (<pluginName>)"
// plus the recovered value, and the update continues. It mirrors the
// try/catch in updatePlayerZones (PluginManager.ts:915-921, 927-933) and
// follows the dialogue-hook precedent (runDialogueHook, gates.go): zone
// hooks do NOT go through executeHook, whose "hook failed" shape is a
// different TS message. A nil logger falls back to log.Default().
func runZoneHook(logger *log.Logger, label string, hook zoneHook, event *ZoneEvent, handler func(*ZoneEvent)) {
	defer func() {
		if r := recover(); r != nil {
			if logger == nil {
				logger = log.Default()
			}
			logger.Printf("[plugins] %s hook threw (%s): %v", label, hook.pluginName, r)
		}
	}()
	handler(event)
}

// ForgetPlayerZones discards one player's zone active set: the Go analogue
// of the TS WeakMap entry dying with the player object
// (PluginManager.ts:332). EmitPlayerDisconnect calls it for every player
// leaving the world, so the coordinator needs no separate call; the
// method is exported for direct use (and tests) and is safe to call for a
// player with no state. No exit hooks fire: discarding is silent, as the
// WeakMap collection is. A nil Hub, a Hub without a registry, or a nil
// player is a no-op.
func (h *Hub) ForgetPlayerZones(player *game.Player) {
	if h == nil || h.hooks == nil || player == nil {
		return
	}
	h.hooks.forgetZoneState(player)
}
