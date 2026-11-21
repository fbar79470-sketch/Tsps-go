package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Run-energy equipment-effects wiring: the cmd/server adapter
// between the plugin hub's registerRunEnergy* surface and the
// world's EquipmentEffects registry.
//
// TS keeps the modifier registries as module-level arrays in
// EquipmentEffects.ts, so a plugin's registration lands directly.
// Go keeps one registry per World (see
// internal/game/equipment_effects.go): plugin Register functions
// run during Hub.Load, before the world exists, so the hub
// queues modifiers until the coordinator installs the registry
// here, and SetRunEnergyModifierRegistry flushes the queue in
// registration order. The install runs in buildServer, right
// after installPluginWorldSeams, before the engine starts — so
// every queued modifier is in the world's registry before any
// player ticks, and the World installs the registry on players
// at add.
//
// The "equipment:weights" emit needs no wiring here: it rides
// the Player.EmitCustomEvent sink installInterfaceRuntime
// installs on entering sessions (interfacewiring.go), which is
// installed before QueueAddPlayer and before the login hooks
// fire. There is deliberately no second seam: the weight
// calculation emits through the same sink the interface runtime
// installs.

// equipmentEffectsRegistryAdapter adapts a *game.EquipmentEffects
// to the hub's plugins.RunEnergyModifierRegistry surface (the
// AreaRegistry precedent: the adapter lives here, in the package
// that imports both sides, and game never imports plugins).
// Api.RegisterRunEnergyRestoreModifier /
// Api.RegisterRunEnergyDrainModifier push content modifiers onto
// the world's registry through it.
type equipmentEffectsRegistryAdapter struct {
	effects *game.EquipmentEffects
}

var _ plugins.RunEnergyModifierRegistry = equipmentEffectsRegistryAdapter{}

// RegisterRestoreModifier appends a restore modifier to the
// world's registry, in registration order.
func (a equipmentEffectsRegistryAdapter) RegisterRestoreModifier(modifier game.RunEnergyRestoreModifier) {
	a.effects.RegisterRestoreModifier(modifier)
}

// RegisterDrainModifier appends a drain modifier to the world's
// registry, in registration order.
func (a equipmentEffectsRegistryAdapter) RegisterDrainModifier(modifier game.RunEnergyDrainModifier) {
	a.effects.RegisterDrainModifier(modifier)
}

// installEquipmentEffectsWiring installs the world's
// EquipmentEffects registry on the hub, flushing every modifier
// queued during plugin loading into it, in registration order.
// A nil hub or world installs nothing. It must run before the
// engine starts (see the file header); the world's registry is
// never nil (World.EquipmentEffects lazily creates one), so the
// adapter always has a live target.
func installEquipmentEffectsWiring(hub *plugins.Hub, world *game.World) {
	if hub == nil || world == nil {
		return
	}
	hub.SetRunEnergyModifierRegistry(equipmentEffectsRegistryAdapter{effects: world.EquipmentEffects()})
}
