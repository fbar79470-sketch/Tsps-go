package main

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"tsps-go/internal/config"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func TestNewPluginHubLoadsCompiledInContent(t *testing.T) {
	// A free world (MembersWorld unset) loads the compiled-in
	// content except the members-only GracefulOutfit and
	// SeekingArrows, which the members gate drops: AdminCommands
	// (phase 38, no members flag) loads first, then DateMinutes
	// (phase 10), LumbridgeGuide (phase 23, no members flag),
	// Shopkeepers (phase 41, no members flag), WildernessDitch
	// (phase 40, no members flag), WornWeights (phase 22), and
	// Woodcutting (phase 32, no members flag) load, and Fishing
	// (phase 35, no members flag — F2P) loads between WornWeights
	// and Woodcutting. Woodcutting loads after WornWeights in
	// the observed load order (Go init order, not the order of
	// the blank imports in pluginwiring.go). Cooking (phase 36,
	// no members flag — F2P) loads last.
	hub, err := newPluginHub(&config.World{}, log.Default(), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	loaded := hub.Loaded()
	want := []string{"AdminCommands", "DateMinutes", "LumbridgeGuide", "Shopkeepers", "WildernessDitch", "WornWeights", "Fishing", "Woodcutting", "Cooking"}
	if len(loaded) != len(want) {
		t.Fatalf("Loaded() = %v, want %v", loaded, want)
	}
	for i, name := range want {
		if loaded[i] != name {
			t.Fatalf("Loaded() = %v, want %v", loaded, want)
		}
	}
}

func TestNewPluginHubLoadsMembersContentOnMembersWorld(t *testing.T) {
	// A members world loads GracefulOutfit too (members: true,
	// GracefulOutfit.plugin.js), between DateMinutes and
	// LumbridgeGuide in blank-import order, and SeekingArrows
	// (members: true, SeekingArrows.plugin.js, phase 24)
	// between LumbridgeGuide and WornWeights; Shopkeepers
	// (phase 41, no members flag) loads between SeekingArrows
	// and WildernessDitch; WildernessDitch (phase 40, no
	// members flag) loads between Shopkeepers and WornWeights;
	// Fletching (members: true, phase 34) loads between
	// WornWeights and Woodcutting; Fishing (phase 35, no members
	// flag — F2P) loads between WornWeights and Fletching, and
	// Woodcutting (phase 32, no members flag) loads before
	// Cooking (phase 36, no members flag — F2P), which loads
	// last. AdminCommands (phase 38, no members flag) loads
	// first, before DateMinutes.
	hub, err := newPluginHub(&config.World{MembersWorld: true}, log.Default(), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	loaded := hub.Loaded()
	want := []string{"AdminCommands", "DateMinutes", "GracefulOutfit", "LumbridgeGuide", "SeekingArrows", "Shopkeepers", "WildernessDitch", "WornWeights", "Fishing", "Fletching", "Woodcutting", "Cooking"}
	if len(loaded) != len(want) {
		t.Fatalf("Loaded() = %v, want %v", loaded, want)
	}
	for i, name := range want {
		if loaded[i] != name {
			t.Fatalf("Loaded() = %v, want %v", loaded, want)
		}
	}
}

func TestNewPluginHubSwitchDisablesDateMinutes(t *testing.T) {
	// A world.json switch turning DateMinutes off keeps it out of
	// the loaded set, and the hub logs it as disabled. AdminCommands,
	// LumbridgeGuide, Shopkeepers, WildernessDitch, WornWeights,
	// Woodcutting, Fishing and Cooking (no switches) still load
	// (AdminCommands first; Shopkeepers between LumbridgeGuide and
	// WildernessDitch; Fishing between WornWeights and Woodcutting;
	// Cooking last); GracefulOutfit stays gated out of the free world.
	var buf bytes.Buffer
	hub, err := newPluginHub(&config.World{Plugins: map[string]bool{"DateMinutes": false}}, log.New(&buf, "", 0), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	if loaded := hub.Loaded(); len(loaded) != 8 || loaded[0] != "AdminCommands" || loaded[1] != "LumbridgeGuide" || loaded[2] != "Shopkeepers" || loaded[3] != "WildernessDitch" || loaded[4] != "WornWeights" || loaded[5] != "Fishing" || loaded[6] != "Woodcutting" || loaded[7] != "Cooking" {
		t.Fatalf("Loaded() = %v, want [AdminCommands LumbridgeGuide Shopkeepers WildernessDitch WornWeights Fishing Woodcutting Cooking]", loaded)
	}
	if !strings.Contains(buf.String(), "disabled=") {
		t.Errorf("hub log %q does not report the disabled plugin", buf.String())
	}
}

func TestInstallPluginWorldSeamsEmitsProcess(t *testing.T) {
	var processCalls int
	var core *plugins.Core
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "Recorder",
		Register: func(api *plugins.Api) {
			core = api.Core()
			api.OnPlayerProcess(func(*plugins.PlayerProcessEvent) { processCalls++ })
		},
	}}, plugins.HubOptions{})
	hub.Load()
	if core == nil {
		t.Fatal("Register did not run: no Core captured")
	}

	spawn := game.Location{X: 3200, Y: 3200, Z: 0}
	world := game.NewWorld(spawn)
	installPluginWorldSeams(hub, world)
	if world.Hooks.PlayerProcess == nil {
		t.Fatal("PlayerProcess hook not installed")
	}
	// The cycle source reads the world's process cycle.
	if got := core.Cycle(); got != 0 {
		t.Fatalf("cycle before any tick = %d, want 0", got)
	}
	p := game.NewPlayer("recorder", spawn)
	world.QueueAddPlayer(p)
	world.Process()
	if processCalls != 1 {
		t.Fatalf("processCalls = %d, want 1", processCalls)
	}
	if got := core.Cycle(); got != 1 {
		t.Fatalf("cycle after one tick = %d, want 1", got)
	}
}
