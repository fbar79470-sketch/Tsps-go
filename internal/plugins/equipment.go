// Run-energy modifier registration: the Go form of the
// registerRunEnergyRestoreModifier / registerRunEnergyDrainModifier
// members of PluginApi (PluginTypes.ts:1491-1498) and their
// createApi delegations (PluginManager.ts:4761-4768), which push
// onto the EquipmentEffects module registries.
//
// The queue-until-installed shape follows the
// RegisterNpcCombatMethodProvider precedent (combat.go): plugin
// Register functions run during Hub.Load, which the coordinator
// runs BEFORE the world (and its EquipmentEffects registry) is
// built, so modifiers registered before installation are queued
// in the hook registry and flushed in registration order when
// the coordinator installs the registry with
// Hub.SetRunEnergyModifierRegistry. Queueing — not an error, not
// a panic, not silent loss — is the only behavior that preserves
// the TS outcome (the modifier ends up registered) across the
// load-before-world ordering. Registration order is reduce order
// in the apply, so the flush preserves the order modifiers were
// registered, and modifiers registered after installation push
// directly, after the flushed ones.
//
// The registry seam follows the AreaRegistry precedent
// (areas.go): RunEnergyModifierRegistry is a narrow interface
// declared here, in the plugins package, so game never imports
// plugins; the coordinator's cmd/server adapter implements it
// over the concrete *game.EquipmentEffects. A nil registry means
// uninstalled: registration queues, and
// SetRunEnergyModifierRegistry(nil) clears the installed registry
// while KEEPING the queue — a later install still flushes
// everything, so no modifier is ever lost to an install/clear
// cycle.
//
// The other EquipmentEffects register members
// (registerRunEnergyStatusModifier, the hit/prayer/special
// families) stay table-only in hooks.go: their registries do not
// exist in Go yet, and each lands with the system that consumes
// it.

package plugins

import (
	"tsps-go/internal/game"
)

// RunEnergyModifierRegistry is the narrow modifier-registry
// surface Api.RegisterRunEnergyRestoreModifier /
// Api.RegisterRunEnergyDrainModifier push onto: the Go form of
// the runEnergyRestoreModifiers / runEnergyDrainModifiers pushes
// in registerRunEnergyRestoreModifier /
// registerRunEnergyDrainModifier (EquipmentEffects.ts:199-209).
// The interface carries the two register methods alone: content
// registers modifiers, it never applies them (the formulas do,
// through the game registry) and there is no deregistration in
// TS, so none is offered here.
type RunEnergyModifierRegistry interface {
	// RegisterRestoreModifier appends a restore modifier to
	// the registry, in registration order (the apply reduce
	// consults modifiers in order).
	RegisterRestoreModifier(modifier game.RunEnergyRestoreModifier)
	// RegisterDrainModifier appends a drain modifier to the
	// registry, in registration order.
	RegisterDrainModifier(modifier game.RunEnergyDrainModifier)
}

// SetRunEnergyModifierRegistry installs the registry
// Api.RegisterRunEnergyRestoreModifier /
// Api.RegisterRunEnergyDrainModifier push onto, flushing every
// modifier queued before installation, in registration order
// within each family. The coordinator calls it once at boot,
// after the world (and its EquipmentEffects registry) exists;
// calling it with nil clears the installed registry but keeps
// the queue (a later non-nil install flushes it), mirroring
// SetAreaRegistry. It is a no-op on a nil Hub or a Hub without
// a registry. The flush runs outside the registry lock, as
// SetAreaRegistry's does. The two families flush independently
// (restore first, then drain): TS keeps them in separate
// arrays, so cross-family registration order is unobservable.
func (h *Hub) SetRunEnergyModifierRegistry(reg RunEnergyModifierRegistry) {
	if h == nil || h.hooks == nil {
		return
	}
	restore, drain := h.hooks.installRunEnergyModifierRegistry(reg)
	for _, modifier := range restore {
		reg.RegisterRestoreModifier(modifier)
	}
	for _, modifier := range drain {
		reg.RegisterDrainModifier(modifier)
	}
}

// RunEnergyModifierRegistry returns the installed modifier
// registry, or nil when none is installed (or the Hub is nil).
// It mirrors Hub.AreaRegistry: the read half of the install
// seam, for the coordinator and tests.
func (h *Hub) RunEnergyModifierRegistry() RunEnergyModifierRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedRunEnergyModifierRegistry()
}

// runEnergyRestoreModifierRegistrationTarget returns the
// installed modifier registry for a restore modifier to be
// pushed onto, or queues the modifier (in registration order)
// and returns nil when no registry is installed yet. The caller
// pushes onto a returned registry outside the lock. It mirrors
// areaRegistrationTarget (hooks.go). A nil modifier is never
// queued: the Api method filters nils before calling here,
// mirroring the TS non-function guard at the register site.
func (r *hookRegistry) runEnergyRestoreModifierRegistrationTarget(modifier game.RunEnergyRestoreModifier) RunEnergyModifierRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runEnergyModifierRegistry != nil {
		return r.runEnergyModifierRegistry
	}
	r.queuedRunEnergyRestoreModifiers = append(r.queuedRunEnergyRestoreModifiers, modifier)
	return nil
}

// runEnergyDrainModifierRegistrationTarget is the drain-family
// form of runEnergyRestoreModifierRegistrationTarget.
func (r *hookRegistry) runEnergyDrainModifierRegistrationTarget(modifier game.RunEnergyDrainModifier) RunEnergyModifierRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.runEnergyModifierRegistry != nil {
		return r.runEnergyModifierRegistry
	}
	r.queuedRunEnergyDrainModifiers = append(r.queuedRunEnergyDrainModifiers, modifier)
	return nil
}

// installRunEnergyModifierRegistry installs reg and returns the
// modifiers queued before installation, in registration order
// within each family, for the caller to push outside the lock.
// Installing nil clears the registry but keeps the queue. It
// mirrors installAreaRegistry (hooks.go).
func (r *hookRegistry) installRunEnergyModifierRegistry(reg RunEnergyModifierRegistry) ([]game.RunEnergyRestoreModifier, []game.RunEnergyDrainModifier) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runEnergyModifierRegistry = reg
	if reg == nil {
		return nil, nil
	}
	restore := r.queuedRunEnergyRestoreModifiers
	drain := r.queuedRunEnergyDrainModifiers
	r.queuedRunEnergyRestoreModifiers = nil
	r.queuedRunEnergyDrainModifiers = nil
	return restore, drain
}

func (r *hookRegistry) installedRunEnergyModifierRegistry() RunEnergyModifierRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.runEnergyModifierRegistry
}

// RegisterRunEnergyRestoreModifier registers a run-energy
// restore modifier, mirroring the PluginApi
// registerRunEnergyRestoreModifier member
// (PluginTypes.ts:1491-1493) and its createApi implementation
// (PluginManager.ts:4761-4764), which delegates to
// EquipmentEffects.registerRunEnergyRestoreModifier. With no
// registry installed yet — the normal case during Hub.Load,
// which runs before the world is built — the modifier is queued
// and flushed in order by SetRunEnergyModifierRegistry; it is
// never lost and never silently dropped.
//
// RegisterRunEnergyRestoreModifier has NO load-phase gate, like
// RegisterArea and RegisterNpcCombatMethodProvider: the TS
// closure has no phase check, and both load-time registration
// (queued until the registry is installed) and later
// registration (direct) are legitimate — TS pushes onto the
// module array whenever it is called. A nil Api, or an Api with
// a nil hub, is a no-op. A nil modifier is ignored silently,
// mirroring the TS `typeof modifier !== "function"` guard
// (EquipmentEffects.ts:200-202).
func (a *Api) RegisterRunEnergyRestoreModifier(modifier game.RunEnergyRestoreModifier) {
	if a == nil || a.hub == nil || a.hub.hooks == nil || modifier == nil {
		return
	}
	if reg := a.hub.hooks.runEnergyRestoreModifierRegistrationTarget(modifier); reg != nil {
		reg.RegisterRestoreModifier(modifier)
	}
}

// RegisterRunEnergyDrainModifier registers a run-energy drain
// modifier, mirroring the PluginApi registerRunEnergyDrainModifier
// member (PluginTypes.ts:1494-1498) and its createApi
// implementation (PluginManager.ts:4765-4768), which delegates to
// EquipmentEffects.registerRunEnergyDrainModifier. Queueing,
// gating, and nil handling are exactly as
// RegisterRunEnergyRestoreModifier's; the TS guard is the
// `typeof modifier === "function"` check
// (EquipmentEffects.ts:206-208).
func (a *Api) RegisterRunEnergyDrainModifier(modifier game.RunEnergyDrainModifier) {
	if a == nil || a.hub == nil || a.hub.hooks == nil || modifier == nil {
		return
	}
	if reg := a.hub.hooks.runEnergyDrainModifierRegistrationTarget(modifier); reg != nil {
		reg.RegisterDrainModifier(modifier)
	}
}
