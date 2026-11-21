package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadWorldBasic(t *testing.T) {
	w, err := LoadWorld("testdata/basic")
	if err != nil {
		t.Fatalf("LoadWorld: %v", err)
	}
	if w.Name != "Old School RuneScape" {
		t.Errorf("Name = %q", w.Name)
	}
	if w.Description != "" {
		t.Errorf("Description = %q", w.Description)
	}
	if w.Gameframe != "" {
		t.Errorf("Gameframe = %q, want empty", w.Gameframe)
	}
	if w.Spawn != (Location{X: 3089, Y: 3524, Z: 0}) {
		t.Errorf("Spawn = %+v", w.Spawn)
	}
	if w.ExperienceMultiplier != 1 {
		t.Errorf("ExperienceMultiplier = %v", w.ExperienceMultiplier)
	}
	if !w.MembersWorld {
		t.Error("MembersWorld = false, want true (no FreeToPlay switch)")
	}
	if got := w.Plugins["TutorialIsland"]; got {
		t.Errorf("Plugins[TutorialIsland] = %v, want false", got)
	}
	if len(w.Zones) != 2 {
		t.Fatalf("len(Zones) = %d, want 2", len(w.Zones))
	}
	if w.Zones[0].Bounded {
		t.Error("Zones[0] should be unbounded (global zone)")
	}
	if got := w.Zones[0].Tags; len(got) != 1 || got[0] != "blast-furnace" {
		t.Errorf("Zones[0].Tags = %v", got)
	}
	z := w.Zones[1]
	if !z.Bounded || z.MinX != 3324 || z.MaxX != 3392 || z.MinY != 3201 || z.MaxY != 3266 || z.Z != 0 {
		t.Errorf("Zones[1] = %+v", z)
	}
}

func TestLoadWorldOverlay(t *testing.T) {
	w, err := LoadWorld("testdata/overlay")
	if err != nil {
		t.Fatalf("LoadWorld: %v", err)
	}
	// Top-level keys replace: experienceMultiplier comes from the overlay.
	if w.ExperienceMultiplier != 5 {
		t.Errorf("ExperienceMultiplier = %v, want 5 (overlay wins)", w.ExperienceMultiplier)
	}
	// Untouched top-level keys keep shipped values.
	if w.Name != "My World" || w.Gameframe != "317-fixed" {
		t.Errorf("Name = %q, Gameframe = %q", w.Name, w.Gameframe)
	}
	if w.Description != "A local world." {
		t.Errorf("Description = %q, want trimmed", w.Description)
	}
	// plugins merge key-by-key: FreeToPlay survives, TutorialIsland is
	// overridden, Extra is added.
	if !w.Plugins["FreeToPlay"] || w.Plugins["TutorialIsland"] || !w.Plugins["Extra"] {
		t.Errorf("Plugins = %v", w.Plugins)
	}
	if w.MembersWorld {
		t.Error("MembersWorld = true, want false (FreeToPlay on)")
	}
	// pluginConfig merges key-by-key too.
	if w.PluginConfig["Shared:keep"] != "local" {
		t.Errorf("PluginConfig[Shared:keep] = %v, want local", w.PluginConfig["Shared:keep"])
	}
	if w.PluginConfig["Example:rate"] != 3.0 {
		t.Errorf("PluginConfig[Example:rate] = %v (%T), want 3.0", w.PluginConfig["Example:rate"], w.PluginConfig["Example:rate"])
	}
	if w.PluginConfig["Shared:extra"] != true {
		t.Errorf("PluginConfig[Shared:extra] = %v", w.PluginConfig["Shared:extra"])
	}
	// Duplicate tags are deduplicated in first-seen order.
	if got := w.Zones[0].Tags; len(got) != 2 || got[0] != "f2p" || got[1] != "pvp" {
		t.Errorf("Zones[0].Tags = %v", got)
	}
}

func TestLoadWorldObjectPluginEntries(t *testing.T) {
	w, err := LoadWorld("testdata/objectplugins")
	if err != nil {
		t.Fatalf("LoadWorld: %v", err)
	}
	if !w.Plugins["FreeToPlay"] || w.Plugins["LeagueMode"] {
		t.Errorf("Plugins = %v", w.Plugins)
	}
	if w.MembersWorld {
		t.Error("MembersWorld = true, want false (FreeToPlay object entry enabled)")
	}
	// Object options become flat settings; LeagueMode maps to the
	// legacy "league" namespace.
	if w.PluginConfig["FreeToPlay:maxLevel"] != 20.0 {
		t.Errorf("PluginConfig[FreeToPlay:maxLevel] = %v", w.PluginConfig["FreeToPlay:maxLevel"])
	}
	if w.PluginConfig["league:season"] != 3.0 {
		t.Errorf("PluginConfig[league:season] = %v", w.PluginConfig["league:season"])
	}
	if w.PluginConfig["Pre:existing"] != 1.0 {
		t.Errorf("PluginConfig[Pre:existing] = %v", w.PluginConfig["Pre:existing"])
	}
	if w.ExperienceMultiplier != 1 {
		t.Errorf("ExperienceMultiplier = %v, want default 1", w.ExperienceMultiplier)
	}
}

// writeWorld writes worldJSON as <dir>/definitions/world.json (and
// optionally a local overlay) and returns dir.
func writeWorld(t *testing.T, worldJSON string, localJSON ...string) string {
	t.Helper()
	dir := t.TempDir()
	defs := filepath.Join(dir, "definitions")
	if err := os.MkdirAll(defs, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(defs, "world.json"), []byte(worldJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	if len(localJSON) > 0 {
		if err := os.WriteFile(filepath.Join(defs, "world.local.json"), []byte(localJSON[0]), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

const validWorld = `{"name":"Test","spawn":{"x":3200,"y":3200,"z":0},"zones":[{"tags":["f2p"]}]}`

func TestLoadWorldValidationErrors(t *testing.T) {
	cases := []struct {
		name    string
		json    string
		wantErr string // substring of the error
	}{
		{"missing zones", `{"name":"T","spawn":{"x":1,"y":1,"z":0}}`, "world.json zones must be an array"},
		{"zones not array", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":{}}`, "world.json zones must be an array"},
		{"zero xp", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[],"experienceMultiplier":0}`, "world.json.experienceMultiplier must be a positive number"},
		{"negative xp", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[],"experienceMultiplier":-2}`, "world.json.experienceMultiplier must be a positive number"},
		{"string xp", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[],"experienceMultiplier":"2"}`, "world.json.experienceMultiplier must be a positive number"},
		{"bad gameframe", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[],"gameframe":"huge"}`, "world.json gameframe must be one of"},
		{"empty name", `{"name":"   ","spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "world.json name must be plain text between 1 and 64 characters"},
		{"name with angle bracket", `{"name":"A<B","spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "world.json name must be plain text between 1 and 64 characters"},
		{"name too long", `{"name":"` + strings.Repeat("a", 65) + `","spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "world.json name must be plain text between 1 and 64 characters"},
		{"name with control char", `{"name":"A\u0001B","spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "world.json name must be plain text between 1 and 64 characters"},
		{"description too long", `{"name":"T","description":"` + strings.Repeat("d", 121) + `","spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "world.json description must be a string of at most 120 characters"},
		{"spawn x out of range", `{"name":"T","spawn":{"x":16384,"y":1,"z":0},"zones":[]}`, "world.json spawn.x is outside the world"},
		{"spawn y negative", `{"name":"T","spawn":{"x":1,"y":-1,"z":0},"zones":[]}`, "world.json spawn.y is outside the world"},
		{"spawn z out of range", `{"name":"T","spawn":{"x":1,"y":1,"z":4},"zones":[]}`, "world.json spawn.z must be between 0 and 3"},
		{"spawn x fractional", `{"name":"T","spawn":{"x":1.5,"y":1,"z":0},"zones":[]}`, "world.json spawn.x must be an integer"},
		{"spawn missing", `{"name":"T","zones":[]}`, "world.json spawn must be an object"},
		{"zone missing bound", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[{"minX":0,"tags":["pvp"]}]}`, "world.json zones[0].maxX must be an integer"},
		{"zone reversed bounds", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[{"minX":10,"maxX":5,"minY":0,"maxY":5,"z":0,"tags":["pvp"]}]}`, "world.json zones[0] has reversed bounds"},
		{"zone tags not array", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[{"tags":"pvp"}]}`, "world.json zones[0].tags must be a non-empty array"},
		{"bounded zone empty tags", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[{"minX":0,"maxX":5,"minY":0,"maxY":5,"z":0,"tags":[]}]}`, "world.json zones[0].tags must be a non-empty array"},
		{"zone non-string tag", `{"name":"T","spawn":{"x":1,"y":1,"z":0},"zones":[{"tags":[1]}]}`, "world.json zones[0].tags must contain strings"},
		{"f2p spawn outside free land", `{"name":"T","plugins":{"FreeToPlay":true},"spawn":{"x":5000,"y":5000,"z":0},"zones":[{"minX":0,"maxX":100,"minY":0,"maxY":100,"z":0,"tags":["f2p"]}]}`, `FreeToPlay needs the spawn inside a zone tagged "f2p"`},
		{"plugin enabled not bool", `{"name":"T","plugins":{"Foo":{"enabled":"yes"}},"spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "plugins.Foo.enabled must be true or false"},
		{"plugin setting with colon", `{"name":"T","plugins":{"Foo":{"bad:key":1}},"spawn":{"x":1,"y":1,"z":0},"zones":[]}`, "Invalid plugin setting plugins.Foo.bad:key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeWorld(t, tc.json)
			_, err := LoadWorld(dir)
			if err == nil {
				t.Fatalf("LoadWorld succeeded, want error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadWorldFileErrors(t *testing.T) {
	// Missing world.json.
	if _, err := LoadWorld(t.TempDir()); err == nil {
		t.Error("LoadWorld on empty dir succeeded, want error")
	} else if !strings.Contains(err.Error(), "world.json") {
		t.Errorf("error = %q, want it to name world.json", err.Error())
	}
	// world.json not an object.
	dir := writeWorld(t, `[1,2,3]`)
	if _, err := LoadWorld(dir); err == nil || !strings.Contains(err.Error(), "world.json must be an object") {
		t.Errorf("error = %v, want 'world.json must be an object'", err)
	}
	// Malformed JSON.
	dir = writeWorld(t, `{oops`)
	if _, err := LoadWorld(dir); err == nil || !strings.Contains(err.Error(), "parsing") {
		t.Errorf("error = %v, want a parse error", err)
	}
	// Trailing garbage after the JSON value.
	dir = writeWorld(t, validWorld+` {}`)
	if _, err := LoadWorld(dir); err == nil {
		t.Error("LoadWorld accepted trailing JSON, want error")
	}
	// Malformed overlay is fatal too.
	dir = writeWorld(t, validWorld, `{bad`)
	if _, err := LoadWorld(dir); err == nil {
		t.Error("LoadWorld accepted malformed overlay, want error")
	}
	// Defaults: omitted name/description/experienceMultiplier.
	dir = writeWorld(t, `{"spawn":{"x":1,"y":1,"z":0},"zones":[]}`)
	w, err := LoadWorld(dir)
	if err != nil {
		t.Fatalf("LoadWorld: %v", err)
	}
	if w.Name != "Old School RuneScape" || w.Description != "" || w.ExperienceMultiplier != 1 {
		t.Errorf("defaults: %+v", w)
	}
}

func TestZoneContains(t *testing.T) {
	w := &World{}
	unbounded := Zone{Tags: []string{"f2p"}}
	if !w.ZoneContains(unbounded, 0, 0) || !w.ZoneContains(unbounded, 16383, 16383) {
		t.Error("unbounded zone should cover everything")
	}
	// A nil *World receiver must still work: matching uses only the zone.
	var nilWorld *World
	if !nilWorld.ZoneContains(unbounded, 5, 5) {
		t.Error("ZoneContains on nil receiver failed for unbounded zone")
	}
	bounded := Zone{MinX: 10, MaxX: 20, MinY: 30, MaxY: 40, Z: 0, Bounded: true}
	for _, tc := range []struct {
		x, y int
		want bool
	}{
		{10, 30, true}, {20, 40, true}, {15, 35, true},
		{9, 30, false}, {21, 30, false}, {10, 29, false}, {10, 41, false},
	} {
		if got := w.ZoneContains(bounded, tc.x, tc.y); got != tc.want {
			t.Errorf("ZoneContains(%d, %d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestIsMembersArea(t *testing.T) {
	// F2P world with one bounded f2p rectangle (3000..3063 x 3200..3263,
	// exactly one 64x64 square) plus a partial-square rectangle.
	json := `{"name":"F2P","plugins":{"FreeToPlay":true},"spawn":{"x":3030,"y":3230,"z":0},"zones":[` +
		`{"minX":3000,"maxX":3063,"minY":3200,"maxY":3263,"z":0,"tags":["f2p"]},` +
		`{"minX":4000,"maxX":4010,"minY":4000,"maxY":4010,"z":0,"tags":["f2p"]},` +
		`{"minX":0,"maxX":100,"minY":0,"maxY":100,"z":0,"tags":["pvp"]}]}`
	w, err := LoadWorld(writeWorld(t, json))
	if err != nil {
		t.Fatalf("LoadWorld: %v", err)
	}
	if w.MembersWorld {
		t.Fatal("want a free-to-play world")
	}
	cases := []struct {
		x, y int
		want bool // true = members area
	}{
		{3030, 3230, false}, // inside the full-square f2p zone
		{3000, 3200, false}, // square corner
		{3063, 3263, false}, // square corner
		{3064, 3230, true},  // just outside
		{4005, 4005, false}, // inside the partial-square f2p zone
		{4020, 4020, true},  // same square, outside the zone
		{50, 50, true},      // pvp zone is not f2p
		{10000, 10000, true},
	}
	for _, tc := range cases {
		if got := w.IsMembersArea(tc.x, tc.y); got != tc.want {
			t.Errorf("IsMembersArea(%d, %d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
	// The same world built literally (no index) must agree: the
	// fallback scan and the square index are equivalent.
	literal := &World{MembersWorld: false, Zones: w.Zones}
	for _, tc := range cases {
		if got := literal.IsMembersArea(tc.x, tc.y); got != tc.want {
			t.Errorf("literal IsMembersArea(%d, %d) = %v, want %v", tc.x, tc.y, got, tc.want)
		}
	}
}

func TestIsMembersAreaMembersWorldAndGlobalF2P(t *testing.T) {
	// Members world: nothing is a members area.
	w, err := LoadWorld(writeWorld(t, validWorld))
	if err != nil {
		t.Fatal(err)
	}
	if w.IsMembersArea(0, 0) || w.IsMembersArea(16383, 16383) {
		t.Error("members world must have no members areas")
	}
	// F2P world with an unbounded f2p zone: nothing is a members area.
	w, err = LoadWorld(writeWorld(t, `{"name":"F","plugins":{"FreeToPlay":true},"spawn":{"x":1,"y":1,"z":0},"zones":[{"tags":["f2p"]}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if w.IsMembersArea(0, 0) || w.IsMembersArea(16383, 16383) {
		t.Error("global f2p zone must leave no members areas")
	}
	// A nil world is not a members area anywhere.
	var nilWorld *World
	if nilWorld.IsMembersArea(0, 0) {
		t.Error("nil world IsMembersArea = true")
	}
}
