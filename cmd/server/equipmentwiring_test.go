package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Equipment-effects wiring tests: installEquipmentEffectsWiring
// flushes the modifiers a plugin registered during Hub.Load into
// the world's EquipmentEffects registry, in registration order,
// and tolerates nils. Fixtures only: a hub with a test plugin, a
// bare world, no cache.

func TestInstallEquipmentEffectsWiringFlushesQueue(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "GracefulProbe",
		Register: func(api *plugins.Api) {
			api.RegisterRunEnergyRestoreModifier(func(_ *game.Player, units float64) float64 {
				return units * 1.2
			})
			api.RegisterRunEnergyDrainModifier(func(_ *game.Player, amount float64) float64 {
				return amount * 0.5
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()
	world := game.NewWorld(game.Location{X: 3200, Y: 3200, Z: 0})
	// Before the install the world's registry is empty.
	p := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200, Z: 0})
	if got := world.EquipmentEffects().ApplyRestoreModifiers(p, 20); got != 20 {
		t.Fatalf("pre-install restore apply = %v, want 20", got)
	}
	installEquipmentEffectsWiring(hub, world)
	// After the install the queued modifiers apply through the
	// world's registry.
	if got := world.EquipmentEffects().ApplyRestoreModifiers(p, 20); got != 24 {
		t.Fatalf("post-install restore apply = %v, want 24", got)
	}
	if got := world.EquipmentEffects().ApplyDrainModifiers(p, 59); got != 29.5 {
		t.Fatalf("post-install drain apply = %v, want 29.5", got)
	}
	// The hub reports the installed registry.
	if hub.RunEnergyModifierRegistry() == nil {
		t.Fatal("hub must report an installed modifier registry after wiring")
	}
}

func TestInstallEquipmentEffectsWiringNilTolerance(t *testing.T) {
	world := game.NewWorld(game.Location{X: 3200, Y: 3200, Z: 0})
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	installEquipmentEffectsWiring(nil, world)
	installEquipmentEffectsWiring(hub, nil)
	installEquipmentEffectsWiring(nil, nil)
	// The unwired hub still reports no registry.
	if hub.RunEnergyModifierRegistry() != nil {
		t.Fatal("nil-tolerant installs must not install a registry")
	}
}
