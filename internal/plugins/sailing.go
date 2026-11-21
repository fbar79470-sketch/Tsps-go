package plugins

import "tsps-go/internal/game"

// Sailing hub surface: the world-entity interaction family and
// the typed Core members for boats (PluginTypes.ts:1066,
// 1404, 1681-1682). The registry add/snapshot methods live in
// this file, following the skills.go precedent; the
// worldEntityInteraction field itself is the one struct field
// this phase adds to hookRegistry (hooks.go).
//
// TS registration for this family (PluginManager.ts:3477-3480)
// checks only that the handler is a function — there is no
// guard wrapper — so Api.OnWorldEntityInteraction mirrors the
// plain nil + canRegister checks of the other families, with
// no additional wrapper.

// WorldEntityInteractionEvent is a click on a world entity
// (a boat): {player, entityIndex, clickType, handled}
// (PluginTypes.ts:199-204). A hook claims the click by setting
// Handled; the emit stops at the first claim.
type WorldEntityInteractionEvent struct {
	Player      *game.Player
	EntityIndex int
	ClickType   int
	Handled     bool
}

// --- hookRegistry storage for the world-entity family (see hooks.go) ---

func (r *hookRegistry) addWorldEntityInteraction(e hookEntry[*WorldEntityInteractionEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.worldEntityInteraction = append(r.worldEntityInteraction, e)
}

func (r *hookRegistry) snapshotWorldEntityInteraction() []hookEntry[*WorldEntityInteractionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*WorldEntityInteractionEvent](nil), r.worldEntityInteraction...)
}

func (r *hookRegistry) setSailing(s *game.Sailing) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sailing = s
}

func (r *hookRegistry) installedSailing() *game.Sailing {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sailing
}

// OnWorldEntityInteraction registers a world-entity
// interaction hook, mirroring the onWorldEntityInteraction
// member (PluginTypes.ts:1066) and its createApi registration
// (PluginManager.ts:3477-3480): a nil handler registers
// nothing, and registration after the hub has loaded is
// ignored with a warning (canRegister). There is no guard
// wrapper in TS, so there is none here.
func (a *Api) OnWorldEntityInteraction(handler func(*WorldEntityInteractionEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onWorldEntityInteraction") {
		return
	}
	a.hub.hooks.addWorldEntityInteraction(hookEntry[*WorldEntityInteractionEvent]{pluginName: a.name, handler: handler})
}

// EmitWorldEntityInteraction runs a world-entity click
// through the registered hooks and reports whether one
// claimed it, mirroring
// PluginManager.emitWorldEntityInteraction
// (PluginManager.ts:1265-1276) with label
// "world_entity_interaction": the entry guards (a nil event
// or player returns false; the TS non-integer entityIndex
// guard has no Go form — an int is always an integer), then
// the hooks in registration order, breaking once the event is
// handled, returning whether it ended handled. A pre-handled
// event runs no hooks and returns true, as the TS loop does.
// A nil Hub or a Hub without a registry returns false.
//
// The production call site is the world-entity listener
// (via the coordinator's Deps adapter);
// Api.EmitWorldEntityInteraction exposes the same emit to
// content (PluginTypes.ts:1404).
func (h *Hub) EmitWorldEntityInteraction(event *WorldEntityInteractionEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil {
		return false
	}
	hooks := h.hooks.snapshotWorldEntityInteraction()
	return runHandledGate(h.Logger(), "world_entity_interaction", hooks, event,
		func(e *WorldEntityInteractionEvent) bool { return e.Handled })
}

// EmitWorldEntityInteraction runs a world-entity click
// through the registered hooks and reports whether one
// claimed it, mirroring the PluginApi
// emitWorldEntityInteraction member (PluginTypes.ts:1404) and
// its createApi passthrough (PluginManager.ts:4696). It
// delegates to Hub.EmitWorldEntityInteraction. A nil Api, a
// nil hub, or a nil event returns false.
func (a *Api) EmitWorldEntityInteraction(event *WorldEntityInteractionEvent) bool {
	if a == nil || a.hub == nil {
		return false
	}
	return a.hub.EmitWorldEntityInteraction(event)
}

// SetSailing installs the world's Sailing runtime content reads
// through Core.Sailing. The coordinator calls it once at boot
// with the Sailing InstallSailing returns; calling it with nil
// clears the handle (Core.Sailing then returns nil). It is a
// no-op on a nil Hub or a Hub without a registry.
func (h *Hub) SetSailing(s *game.Sailing) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setSailing(s)
}

// Sailing returns the installed Sailing runtime, or nil until
// the coordinator installs one.
func (h *Hub) Sailing() *game.Sailing {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedSailing()
}

// Sailing returns the world's Sailing runtime: the
// core.Sailing member (PluginTypes.ts:1681). It is the Sailing
// installed with Hub.SetSailing at boot (nil until the
// coordinator installs one). Safe on a nil Core.
func (c *Core) Sailing() *game.Sailing {
	if c == nil {
		return nil
	}
	return c.hub.Sailing()
}

// BoatMoveModeValues is the BoatMoveMode enum surface: the
// core.BoatMoveMode member (PluginTypes.ts:1682), the typeof
// enum as a stateless value, following the ProjectileClass
// precedent. Safe on a nil Core: the surface is stateless.
type BoatMoveModeValues struct{}

// Stopped is the sails-down mode: the boat gathers no way.
func (BoatMoveModeValues) Stopped() game.BoatMoveMode { return game.BoatMoveModeStopped }

// Half is half sail: half the hull's base speed.
func (BoatMoveModeValues) Half() game.BoatMoveMode { return game.BoatMoveModeHalf }

// Full is full sail: the hull's base speed.
func (BoatMoveModeValues) Full() game.BoatMoveMode { return game.BoatMoveModeFull }

// Reverse is backing up under sail.
func (BoatMoveModeValues) Reverse() game.BoatMoveMode { return game.BoatMoveModeReverse }

// BoatMoveMode returns the BoatMoveMode enum surface: the
// core.BoatMoveMode member (PluginTypes.ts:1682). Safe on a
// nil Core: the surface is stateless.
func (c *Core) BoatMoveMode() BoatMoveModeValues {
	return BoatMoveModeValues{}
}
