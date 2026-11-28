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
	// (phase 38, no members flag) loads first, then BankBooths
	// (phase 46, no members flag — F2P), then DateMinutes
	// (phase 10), Doors (phase 48, no members flag) between
	// DateMinutes and LumbridgeGuide, Ladders (phase 49, no
	// members flag) between Doors and LumbridgeGuide,
	// LumbridgeGuide (phase 23, no members flag), NpcDrops
	// (phase 50, no members flag) between LumbridgeGuide and
	// Shopkeepers (phase 41, no members flag), the phase-45
	// standard-book effects (Standard Bindings, Standard Curses,
	// Freezes, TeleBlock, ElementalStaves, no members flags)
	// load between Shopkeepers and WildernessDitch (phase 40,
	// no members flag), WornWeights (phase 22), Enchant
	// Jewellery (phase 45, no members flag — F2P) between
	// WornWeights and Fishing, and Woodcutting (phase 32, no
	// members flag). Fishing (phase 35, no members flag — F2P)
	// loads between EnchantJewellery and Mining. Woodcutting
	// loads after WornWeights in the observed load order (Go
	// init order, not the order of the blank imports in
	// pluginwiring.go). Mining (phase 42, no members flag —
	// F2P) loads between Fishing and Woodcutting. Cooking
	// (phase 36, no members flag — F2P) loads after
	// Woodcutting. Crafting (phase 44, no members flag — F2P)
	// loads between Cooking and Smithing.
	hub, err := newPluginHub(&config.World{}, log.Default(), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	loaded := hub.Loaded()
	want := []string{"AdminCommands", "BankBooths", "DateMinutes", "Doors", "Ladders", "LumbridgeGuide", "NpcDrops", "Shopkeepers", "Standard Bindings", "Standard Curses", "Freezes", "TeleBlock", "ElementalStaves", "WildernessDitch", "WornWeights", "EnchantJewellery", "Fishing", "Mining", "Woodcutting", "Cooking", "Crafting", "Smithing"}
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
	// LumbridgeGuide in blank-import order, Ladders (phase 49,
	// no members flag) between GracefulOutfit and LumbridgeGuide,
	// and SeekingArrows
	// (members: true, SeekingArrows.plugin.js, phase 24)
	// between LumbridgeGuide and WornWeights; Shopkeepers
	// (phase 41, no members flag) loads between SeekingArrows
	// and the phase-45 standard-book effects (Standard
	// Bindings, Standard Curses, Freezes, TeleBlock,
	// ElementalStaves, no members flags), which load between
	// Shopkeepers and WildernessDitch; WildernessDitch (phase
	// 40, no members flag) loads between the standard-book
	// effects and WornWeights; EnchantJewellery (phase 45, no
	// members flag — F2P) loads between WornWeights and
	// Fishing; Fletching (members: true, phase 34) loads
	// between Fishing and Mining; Fishing (phase 35, no members
	// flag — F2P) loads between EnchantJewellery and Fletching,
	// and Woodcutting (phase 32, no members flag) loads before
	// Cooking (phase 36, no members flag — F2P). Mining (phase
	// 42, no members flag — F2P) loads between Fletching and
	// Woodcutting. Smithing (phase 43, no members flag — F2P)
	// loads last, after Crafting (phase 44, no members flag —
	// F2P), which loads between Cooking and Smithing.
	// AdminCommands (phase 38, no members flag) loads first,
	// before DateMinutes, and BankBooths (phase 46, no members
	// flag — F2P) loads second, between AdminCommands and
	// DateMinutes.
	hub, err := newPluginHub(&config.World{MembersWorld: true}, log.Default(), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	loaded := hub.Loaded()
	want := []string{"AdminCommands", "BankBooths", "DateMinutes", "Doors", "GracefulOutfit", "Ladders", "LumbridgeGuide", "NpcDrops", "SeekingArrows", "Shopkeepers", "Standard Bindings", "Standard Curses", "Freezes", "TeleBlock", "ElementalStaves", "WildernessDitch", "WornWeights", "EnchantJewellery", "Fishing", "Fletching", "Mining", "Woodcutting", "Cooking", "Crafting", "Smithing"}
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
	// BankBooths, Doors, Ladders, LumbridgeGuide, Shopkeepers, WildernessDitch, WornWeights,
	// Crafting, Woodcutting, Fishing, Mining, Cooking and Smithing (no switches)
	// still load (AdminCommands first; BankBooths second, between
	// AdminCommands and LumbridgeGuide; NpcDrops (phase 50, no
	// members flag) between LumbridgeGuide and Shopkeepers;
	// Shopkeepers between NpcDrops and the phase-45 standard-book effects
	// (Standard Bindings, Standard Curses, Freezes, TeleBlock,
	// ElementalStaves), which load between Shopkeepers and
	// WildernessDitch; EnchantJewellery between WornWeights and
	// Fishing; Fishing between EnchantJewellery and Mining;
	// Smithing last, after Crafting);
	// GracefulOutfit stays gated out of the free world.
	// Crafting loads between Cooking and Smithing.
	var buf bytes.Buffer
	hub, err := newPluginHub(&config.World{Plugins: map[string]bool{"DateMinutes": false}}, log.New(&buf, "", 0), nil)
	if err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}
	if loaded := hub.Loaded(); len(loaded) != 21 || loaded[0] != "AdminCommands" || loaded[1] != "BankBooths" || loaded[2] != "Doors" || loaded[3] != "Ladders" || loaded[4] != "LumbridgeGuide" || loaded[5] != "NpcDrops" || loaded[6] != "Shopkeepers" || loaded[7] != "Standard Bindings" || loaded[8] != "Standard Curses" || loaded[9] != "Freezes" || loaded[10] != "TeleBlock" || loaded[11] != "ElementalStaves" || loaded[12] != "WildernessDitch" || loaded[13] != "WornWeights" || loaded[14] != "EnchantJewellery" || loaded[15] != "Fishing" || loaded[16] != "Mining" || loaded[17] != "Woodcutting" || loaded[18] != "Cooking" || loaded[19] != "Crafting" || loaded[20] != "Smithing" {
		t.Fatalf("Loaded() = %v, want [AdminCommands BankBooths Doors Ladders LumbridgeGuide NpcDrops Shopkeepers Standard Bindings Standard Curses Freezes TeleBlock ElementalStaves WildernessDitch WornWeights EnchantJewellery Fishing Mining Woodcutting Cooking Crafting Smithing]", loaded)
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
