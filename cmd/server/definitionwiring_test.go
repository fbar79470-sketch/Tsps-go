package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Fixture-only wiring tests: hand-built definition files in
// t.TempDir, no network, no real data files. Every fixture dir
// carries the files definition.Load requires (see
// internal/definition/doc.go): interface-layouts.json,
// object-spawns.json, ground-items.json, npc-combat-defs.json,
// npc-animations.json, shops.json, custom-shops.json.
// npc-spawns.json is registry-loaded and tolerated when missing;
// custom-definitions.json / custom-items.json are overlay files
// tolerated when missing.

func writeWiringFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	required := map[string]string{
		"interface-layouts.json": "{}",
		"object-spawns.json":     "[]",
		"ground-items.json":      "[]",
		"npc-combat-defs.json":   "{}",
		"npc-animations.json":    "{}",
		"shops.json":             "[]",
		"custom-shops.json":      "[]",
	}
	for name, content := range files {
		required[name] = content
	}
	for name, content := range required {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadGameDefinitionsSpawnsNPCs(t *testing.T) {
	dir := writeWiringFixture(t, map[string]string{
		"npc-spawns.json": `[
			{"id": 123, "x": 3090, "y": 3525, "level": 0, "direction": 0, "wanderRadius": 5, "name": "Shopkeeper"},
			{"id": 456, "x": 3100, "y": 3530, "level": 1}
		]`,
	})
	var logs strings.Builder
	logf := func(format string, args ...any) { logfLine(&logs, format, args...) }
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	store, err := loadGameDefinitions(dir, w, nil, nil, logf)
	if err != nil {
		t.Fatalf("loadGameDefinitions: %v", err)
	}
	if store == nil {
		t.Fatal("store is nil")
	}
	w.Process()
	npcs := w.NPCs().All()
	if len(npcs) != 2 {
		t.Fatalf("NPCs = %d, want 2", len(npcs))
	}
	byID := map[int]*game.NPC{}
	for _, n := range npcs {
		byID[n.ID] = n
	}
	// Client direction 0 maps to server direction 5 (south-west).
	if byID[123].Facing != game.DirSouthWest {
		t.Errorf("NPC 123 Facing = %v, want DirSouthWest", byID[123].Facing)
	}
	if !byID[123].Visible || byID[123].Hitpoints != game.DefaultNPCHitpoints {
		t.Errorf("NPC 123 Visible=%v Hitpoints=%d", byID[123].Visible, byID[123].Hitpoints)
	}
	// No direction given and no cache: the south default.
	if byID[456].Facing != game.DirSouth {
		t.Errorf("NPC 456 Facing = %v, want DirSouth", byID[456].Facing)
	}
	if !strings.Contains(logs.String(), "loaded 2 npc spawns, 0 object spawns") {
		t.Errorf("summary log missing counts: %q", logs.String())
	}
}

func TestLoadGameDefinitionsEmptyDir(t *testing.T) {
	// Required files present but empty, no npc-spawns.json: a
	// successful load with zero spawns, boot continues.
	dir := writeWiringFixture(t, nil)
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	if _, err := loadGameDefinitions(dir, w, nil, nil, nil); err != nil {
		t.Fatalf("loadGameDefinitions: %v", err)
	}
	w.Process()
	if got := w.NPCs().Len(); got != 0 {
		t.Errorf("NPCs = %d, want 0", got)
	}
}

func TestLoadGameDefinitionsMissingDir(t *testing.T) {
	// A missing definitions directory fails Load at the first
	// required file (interface-layouts.json), exactly as the
	// TypeScript loaders' readFileSync throws. The wiring must
	// surface the error, never swallow it or panic.
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	_, err := loadGameDefinitions(filepath.Join(t.TempDir(), "no-such-dir"), w, nil, nil, nil)
	if err == nil {
		t.Fatal("expected an error for a missing definitions directory")
	}
	if !strings.Contains(err.Error(), "interface-layouts.json") {
		t.Errorf("error = %v, want it to name the first required file", err)
	}
}

func TestLoadGameDefinitionsMalformedRequiredFile(t *testing.T) {
	dir := writeWiringFixture(t, map[string]string{
		"object-spawns.json": "{not json",
	})
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	if _, err := loadGameDefinitions(dir, w, nil, nil, nil); err == nil {
		t.Fatal("expected an error for malformed object-spawns.json")
	}
}

func TestLoadGameDefinitionsObjectSpawnsAndRemovals(t *testing.T) {
	dir := writeWiringFixture(t, map[string]string{
		"object-spawns.json": `[
			{"id": 100, "position": {"x": 3200, "y": 3200, "z": 0}, "type": 10, "face": 0},
			{"id": 200, "position": {"x": 3201, "y": 3200, "z": 0}, "type": 10, "face": 0, "remove": true}
		]`,
	})
	var logs strings.Builder
	w := game.NewWorld(game.Location{X: 3200, Y: 3200, Z: 0})
	store, err := loadGameDefinitions(dir, w, nil, nil, func(format string, args ...any) { logfLine(&logs, format, args...) })
	if err != nil {
		t.Fatalf("loadGameDefinitions: %v", err)
	}
	if got := len(store.ObjectSpawns()); got != 1 {
		t.Errorf("ObjectSpawns = %d, want 1", got)
	}
	if got := len(store.ObjectSpawnRemovals()); got != 1 {
		t.Errorf("ObjectSpawnRemovals = %d, want 1", got)
	}
	if !strings.Contains(logs.String(), "1 object spawns, 1 object removals") {
		t.Errorf("summary log missing removal count: %q", logs.String())
	}
}

func TestLoadGameDefinitionsGroundItemsLoaded(t *testing.T) {
	dir := writeWiringFixture(t, map[string]string{
		"ground-items.json": `[
			{"id": 995, "location": {"x": 3090, "y": 3525, "z": 0}, "amt": 10, "respawn": 100}
		]`,
	})
	var logs strings.Builder
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	store, err := loadGameDefinitions(dir, w, nil, nil, func(format string, args ...any) { logfLine(&logs, format, args...) })
	if err != nil {
		t.Fatalf("loadGameDefinitions: %v", err)
	}
	if got := len(store.GroundItemSpawns()); got != 1 {
		t.Errorf("GroundItemSpawns = %d, want 1", got)
	}
	if got := len(store.GroundItems()); got != 1 {
		t.Fatalf("GroundItems = %d, want 1", got)
	}
	if !strings.Contains(logs.String(), "1 ground-item spawns,") {
		t.Errorf("summary log missing ground-item count: %q", logs.String())
	}
	if strings.Contains(logs.String(), "deferred") {
		t.Errorf("summary log still calls ground items deferred: %q", logs.String())
	}
	// Application happens in buildServer (see grounditemswiring.go):
	// the loaded record registers with the manager and becomes
	// visible to a nearby player.
	mgr := newGroundItemManager(w, store)
	applyGroundItemSpawns(mgr, store)
	viewer := game.NewPlayer("Viewer", game.Location{X: 3090, Y: 3525, Z: 0})
	views := mgr.VisibleViews(viewer, 0)
	if len(views) != 1 {
		t.Fatalf("VisibleViews = %d stacks, want 1", len(views))
	}
	if views[0].ItemID != 995 || views[0].Quantity != 10 || views[0].X != 3090 || views[0].Y != 3525 {
		t.Errorf("view = %+v, want coins x10 at (3090, 3525)", views[0])
	}
}

func TestLoadGameDefinitionsNoCacheIgnoresCustomFiles(t *testing.T) {
	// In NoCache mode the overlay step is a no-op that never
	// reads the custom files, even when they are present (and
	// even malformed — the cache tests cover that path).
	dir := writeWiringFixture(t, map[string]string{
		"custom-definitions.json": "{malformed",
		"custom-items.json":       "{malformed",
	})
	w := game.NewWorld(game.Location{X: 3089, Y: 3524, Z: 0})
	if _, err := loadGameDefinitions(dir, w, nil, nil, nil); err != nil {
		t.Fatalf("loadGameDefinitions: %v", err)
	}
}

func TestBuildServerLoadsDefinitions(t *testing.T) {
	// buildServer-style test in the savewiring_test.go manner:
	// a fixture data root with the required definition files
	// plus npc-spawns.json boots, and the spawned NPC is queued
	// before the engine starts.
	writeTestRoot(t)
	defs := filepath.Join("data", "definitions")
	required := map[string]string{
		"interface-layouts.json": "{}",
		"object-spawns.json":     "[]",
		"ground-items.json":      "[]",
		"npc-combat-defs.json":   "{}",
		"npc-animations.json":    "{}",
		"shops.json":             "[]",
		"custom-shops.json":      "[]",
		"npc-spawns.json":        `[{"id": 42, "x": 3090, "y": 3525, "level": 0, "direction": 1}]`,
	}
	for name, content := range required {
		if err := os.WriteFile(filepath.Join(defs, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	srv, err := buildServer(Options{
		NoCache:     true,
		TickRateMs:  600,
		SaveBackend: "memory",
		Log:         log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatalf("buildServer: %v", err)
	}
	t.Cleanup(srv.Stop)
	srv.world.Process()
	npcs := srv.world.NPCs().All()
	if len(npcs) != 1 {
		t.Fatalf("NPCs = %d, want 1", len(npcs))
	}
	if npcs[0].ID != 42 || npcs[0].Loc != (game.Location{X: 3090, Y: 3525, Z: 0}) {
		t.Errorf("NPC = id %d at %+v", npcs[0].ID, npcs[0].Loc)
	}
	if !npcs[0].Visible {
		t.Error("spawned NPC is not visible")
	}
}

func logfLine(sb *strings.Builder, format string, args ...any) {
	// Minimal Sprintf-based logf for capturing wiring logs.
	sb.WriteString(fmt.Sprintf(format, args...))
	sb.WriteByte('\n')
}
