package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validWorldJSON = `{
	"name": "Test World",
	"description": "A test world.",
	"experienceMultiplier": 2,
	"gameframe": "modern-resizable",
	"spawn": {"x": 3222, "y": 3218, "z": 0},
	"zones": [{"minX": 3200, "maxX": 3300, "minY": 3200, "maxY": 3300, "z": 0, "tags": ["f2p", "lumbridge"]}]
}`

func writeWorldFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func sha256Of(t *testing.T, s string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestReadWorldConfigLayers(t *testing.T) {
	// No local file.
	dir := writeWorldFiles(t, map[string]string{"world.json": validWorldJSON})
	layers, err := ReadWorldConfigLayers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if layers.LocalExists || layers.LocalHash != "" || len(layers.Local) != 0 {
		t.Fatalf("layers = %+v", layers)
	}
	if layers.Shipped["name"] != "Test World" {
		t.Fatalf("shipped = %v", layers.Shipped)
	}

	// Local file present: hash of the raw bytes.
	local := `{"name": "Local World"}`
	dir = writeWorldFiles(t, map[string]string{"world.json": validWorldJSON, "world.local.json": local})
	layers, err = ReadWorldConfigLayers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !layers.LocalExists || layers.LocalHash != sha256Of(t, local) || layers.Local["name"] != "Local World" {
		t.Fatalf("layers = %+v", layers)
	}
	if layers.LocalError != "" {
		t.Fatalf("local error = %q", layers.LocalError)
	}

	// Unparseable local: captured, never thrown.
	bad := `{oops`
	dir = writeWorldFiles(t, map[string]string{"world.json": validWorldJSON, "world.local.json": bad})
	layers, err = ReadWorldConfigLayers(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !layers.LocalExists || layers.LocalHash != sha256Of(t, bad) || layers.LocalError == "" {
		t.Fatalf("layers = %+v", layers)
	}
	if len(layers.Local) != 0 {
		t.Fatalf("local must be empty: %v", layers.Local)
	}

	// Non-object local.
	dir = writeWorldFiles(t, map[string]string{"world.json": validWorldJSON, "world.local.json": `[]`})
	layers, err = ReadWorldConfigLayers(dir)
	if err != nil || layers.LocalError != "world.local.json must be an object" {
		t.Fatalf("layers = %+v err=%v", layers, err)
	}

	// Normalize error in the local file becomes LocalError.
	dir = writeWorldFiles(t, map[string]string{
		"world.json":       validWorldJSON,
		"world.local.json": `{"plugins": {"Foo": {"enabled": "yes"}}}`,
	})
	layers, err = ReadWorldConfigLayers(dir)
	if err != nil || !strings.Contains(layers.LocalError, "plugins.Foo.enabled must be true or false") {
		t.Fatalf("layers = %+v err=%v", layers, err)
	}
}

func TestReadWorldConfigLayersShippedErrors(t *testing.T) {
	// Missing world.json fails.
	if _, err := ReadWorldConfigLayers(t.TempDir()); err == nil {
		t.Fatal("missing world.json must fail")
	}
	// Non-object world.json fails.
	dir := writeWorldFiles(t, map[string]string{"world.json": `[]`})
	if _, err := ReadWorldConfigLayers(dir); err == nil || !strings.Contains(err.Error(), "world.json must be an object") {
		t.Fatalf("err = %v", err)
	}
	// Normalize error in the shipped file fails.
	dir = writeWorldFiles(t, map[string]string{"world.json": `{"plugins": {"Foo": {"enabled": 1}}}`})
	if _, err := ReadWorldConfigLayers(dir); err == nil {
		t.Fatal("shipped normalize error must fail")
	}
}

func TestStoreWorldConfigLayers(t *testing.T) {
	// Load does not require world.json.
	dir := fixtureDir(t, minimalFiles())
	store, err := Load(dir, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.WorldConfigLayers(); err == nil {
		t.Fatal("WorldConfigLayers without world.json must fail")
	}
	// With world.json present it reads on demand.
	dir2 := fixtureDir(t, withFiles(map[string]string{"world.json": validWorldJSON}))
	store2, err := Load(dir2, LoadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	layers, err := store2.WorldConfigLayers()
	if err != nil || layers.Shipped["name"] != "Test World" {
		t.Fatalf("layers = %+v err=%v", layers, err)
	}
}

func TestMergeWorldConfig(t *testing.T) {
	shipped := map[string]any{
		"name":         "Shipped",
		"plugins":      map[string]any{"A": true, "B": true},
		"pluginConfig": map[string]any{"a:x": 1.0, "a:y": 2.0},
	}
	local := map[string]any{
		"name":         "Local",
		"plugins":      map[string]any{"B": false},
		"pluginConfig": map[string]any{"a:y": 3.0, "a:z": 4.0},
	}
	merged, err := MergeWorldConfig(shipped, local)
	if err != nil {
		t.Fatal(err)
	}
	if merged["name"] != "Local" {
		t.Fatalf("name = %v", merged["name"])
	}
	plugins := merged["plugins"].(map[string]any)
	if plugins["A"] != true || plugins["B"] != false {
		t.Fatalf("plugins = %v", plugins)
	}
	config := merged["pluginConfig"].(map[string]any)
	if config["a:x"] != 1.0 || config["a:y"] != 3.0 || config["a:z"] != 4.0 {
		t.Fatalf("pluginConfig = %v", config)
	}
	// A normalize error in either layer fails the merge.
	if _, err := MergeWorldConfig(map[string]any{"plugins": map[string]any{"F": map[string]any{"enabled": 1}}}, nil); err == nil {
		t.Fatal("bad shipped layer must fail")
	}
}

func TestPreserveLegacyCrossFloorDefault(t *testing.T) {
	base := map[string]any{"disabledPlugins": []any{"Foo"}}
	// Env unset: CrossFloor is appended.
	t.Setenv("CROSS_FLOOR_VISIBILITY", "")
	t.Setenv("CROSS_FLOOR_VISIBILITY_AND_COMBAT", "")
	// t.Setenv("") sets the variable to ""; LookupEnv reports
	// present, so use os.Unsetenv with a restore instead.
	os.Unsetenv("CROSS_FLOOR_VISIBILITY")
	os.Unsetenv("CROSS_FLOOR_VISIBILITY_AND_COMBAT")
	out := PreserveLegacyCrossFloorDefault(base)
	disabled := out["disabledPlugins"].([]any)
	if len(disabled) != 2 || disabled[1] != "CrossFloor" {
		t.Fatalf("disabled = %v", disabled)
	}
	// Env enabled: untouched.
	t.Setenv("CROSS_FLOOR_VISIBILITY", "1")
	if out := PreserveLegacyCrossFloorDefault(base); len(out["disabledPlugins"].([]any)) != 1 {
		t.Fatalf("disabled = %v", out["disabledPlugins"])
	}
	// Already listed: untouched.
	listed := map[string]any{"disabledPlugins": []any{"CrossFloor"}}
	if out := PreserveLegacyCrossFloorDefault(listed); len(out["disabledPlugins"].([]any)) != 1 {
		t.Fatalf("disabled = %v", out["disabledPlugins"])
	}
	// Explicit switch on: untouched.
	switched := map[string]any{
		"disabledPlugins": []any{"Foo"},
		"plugins":         map[string]any{"CrossFloor": true},
	}
	if out := PreserveLegacyCrossFloorDefault(switched); len(out["disabledPlugins"].([]any)) != 1 {
		t.Fatalf("disabled = %v", out["disabledPlugins"])
	}
	// No disabledPlugins: untouched (same map).
	if out := PreserveLegacyCrossFloorDefault(map[string]any{}); len(out) != 0 {
		t.Fatalf("out = %v", out)
	}
}

func TestNormalizeWorldPluginConfig(t *testing.T) {
	world := map[string]any{
		"plugins": map[string]any{
			"LeagueMode": map[string]any{"enabled": true, "rate": 5.0},
			"Plain":      false,
			"RandomEvents": map[string]any{
				"enabled": false, "interval": 10.0,
			},
		},
		"pluginConfig": map[string]any{"other:k": 1.0},
	}
	normalized, err := NormalizeWorldPluginConfig(world)
	if err != nil {
		t.Fatal(err)
	}
	plugins := normalized["plugins"].(map[string]any)
	if plugins["LeagueMode"] != true || plugins["Plain"] != false || plugins["RandomEvents"] != false {
		t.Fatalf("plugins = %v", plugins)
	}
	config := normalized["pluginConfig"].(map[string]any)
	if config["league:rate"] != 5.0 || config["random-events:interval"] != 10.0 || config["other:k"] != 1.0 {
		t.Fatalf("pluginConfig = %v", config)
	}
	// No object entries: unchanged.
	plain := map[string]any{"plugins": map[string]any{"A": true}}
	if out, err := NormalizeWorldPluginConfig(plain); err != nil || out["plugins"].(map[string]any)["A"] != true {
		t.Fatalf("out = %v err=%v", out, err)
	}
	// Errors.
	for _, tc := range []map[string]any{
		{"plugins": map[string]any{"F": map[string]any{"enabled": "yes"}}},
		{"plugins": map[string]any{"F": map[string]any{"bad key": 1}}},
		{"plugins": map[string]any{"Bad Name": map[string]any{"x": 1}}},
	} {
		if _, err := NormalizeWorldPluginConfig(tc); err == nil {
			t.Fatalf("must fail: %v", tc)
		}
	}
}

func TestPluginConfigKey(t *testing.T) {
	if got := PluginConfigKey("LeagueMode", "rate"); got != "league:rate" {
		t.Fatalf("key = %q", got)
	}
	if got := PluginConfigKey("RandomEvents", "x"); got != "random-events:x" {
		t.Fatalf("key = %q", got)
	}
	if got := PluginConfigKey("Characters", "x"); got != "characters:x" {
		t.Fatalf("key = %q", got)
	}
	if got := PluginConfigKey("Quests", "x"); got != "quests:x" {
		t.Fatalf("key = %q", got)
	}
	if got := PluginConfigKey("MyPlugin", "x"); got != "MyPlugin:x" {
		t.Fatalf("key = %q", got)
	}
}

func TestPluginNamesAndSwitches(t *testing.T) {
	if got := NormalizePluginName("Free-To_Play!"); got != "freetoplay" {
		t.Fatalf("normalize = %q", got)
	}
	if got := PluginNameFromFile("FreeToPlay.plugin.js"); got != "FreeToPlay" {
		t.Fatalf("from file = %q", got)
	}
	if got := PluginNameFromFile("foo.PLUGIN.JS"); got != "foo" {
		t.Fatalf("from file = %q", got)
	}
	if got := PluginNameFromFile("plain.js"); got != "plain.js" {
		t.Fatalf("from file = %q", got)
	}

	// Legacy form: disabledPlugins, FreeToPlay exception, default
	// enabled true.
	switches, def, err := ParsePluginSwitches(map[string]any{
		"disabledPlugins": []any{"Foo", "FreeToPlay"},
		"plugins":         map[string]any{"FreeToPlay": true},
	})
	if err != nil || def == nil || !*def {
		t.Fatalf("switches=%v def=%v err=%v", switches, def, err)
	}
	if switches["foo"] != false || switches["freetoplay"] != true {
		t.Fatalf("switches = %v", switches)
	}
	// Modern form.
	switches, def, err = ParsePluginSwitches(map[string]any{
		"plugins": map[string]any{"Foo": true, "Bar": false},
	})
	if err != nil || def != nil || switches["foo"] != true || switches["bar"] != false {
		t.Fatalf("switches=%v def=%v err=%v", switches, def, err)
	}
	// Errors.
	for _, tc := range []map[string]any{
		{"disabledPlugins": "Foo"},
		{"disabledPlugins": []any{1}},
		{"disabledPlugins": []any{""}},
		{"plugins": []any{}},
		{"plugins": map[string]any{"A": "yes"}},
	} {
		if _, _, err := ParsePluginSwitches(tc); err == nil {
			t.Fatalf("must fail: %v", tc)
		}
	}
	// Absent plugins: empty switches, no default.
	switches, def, err = ParsePluginSwitches(map[string]any{})
	if err != nil || def != nil || len(switches) != 0 {
		t.Fatalf("switches=%v def=%v err=%v", switches, def, err)
	}
}

func TestResolvePluginEnabled(t *testing.T) {
	switches := map[string]bool{"myplugin": false}
	// Normalized name wins.
	if ResolvePluginEnabled(switches, nil, "MyPlugin", "myplugin.plugin.js", nil) {
		t.Fatal("normalized switch must disable")
	}
	// File name is the fallback.
	if !ResolvePluginEnabled(map[string]bool{"other": true}, nil, "MyPlugin", "other.plugin.js", nil) {
		t.Fatal("file-name switch must enable")
	}
	// Default enabled, then pluginEnabled, then true.
	defTrue := true
	if !ResolvePluginEnabled(map[string]bool{}, &defTrue, "X", "x.plugin.js", nil) {
		t.Fatal("defaultEnabled must win")
	}
	disabled := false
	if ResolvePluginEnabled(map[string]bool{}, nil, "X", "x.plugin.js", &disabled) {
		t.Fatal("pluginEnabled false must disable")
	}
	if !ResolvePluginEnabled(map[string]bool{}, nil, "X", "x.plugin.js", nil) {
		t.Fatal("no information must default to enabled")
	}
}

func TestIsFreeToPlayWorld(t *testing.T) {
	if IsFreeToPlayWorld(map[string]any{}) {
		t.Fatal("no plugins must not be F2P")
	}
	if !IsFreeToPlayWorld(map[string]any{"plugins": map[string]any{"FreeToPlay": true}}) {
		t.Fatal("FreeToPlay switch must make F2P")
	}
	if IsFreeToPlayWorld(map[string]any{"plugins": map[string]any{"FreeToPlay": false}}) {
		t.Fatal("FreeToPlay off must not be F2P")
	}
	if !IsFreeToPlayWorld(map[string]any{"plugins": map[string]any{"FreeToPlay": map[string]any{"enabled": true}}}) {
		t.Fatal("object entry enabled must make F2P")
	}
}

func TestParseWorldDefinition(t *testing.T) {
	world := map[string]any{
		"name":                 "  Trimmed  ",
		"description":          "Desc.",
		"experienceMultiplier": 2.5,
		"gameframe":            "317-fixed",
		"spawn":                map[string]any{"x": 3222.0, "y": 3218.0, "z": 0.0},
		"zones": []any{map[string]any{
			"minX": 3200.0, "maxX": 3300.0, "minY": 3200.0, "maxY": 3300.0, "z": 0.0,
			"tags": []any{"f2p", "lumbridge", "f2p"},
		}},
	}
	data, err := ParseWorldDefinition(world)
	if err != nil {
		t.Fatal(err)
	}
	if data.Name != "Trimmed" || data.Description != "Desc." || data.ExperienceMultiplier != 2.5 ||
		data.Gameframe != "317-fixed" || !data.HasGameframe || !data.MembersWorld {
		t.Fatalf("data = %+v", data)
	}
	if len(data.Zones) != 1 || len(data.Zones[0].Tags) != 2 || data.Zones[0].Tags[1] != "lumbridge" {
		t.Fatalf("zones = %+v", data.Zones)
	}
	if !ZoneContains(data.Zones[0], 3222, 3218) || ZoneContains(data.Zones[0], 100, 100) {
		t.Fatal("ZoneContains wrong")
	}

	// Defaults: name, multiplier, no gameframe. z is required
	// (TS plane() has no default).
	minimal := map[string]any{
		"spawn": map[string]any{"x": 1.0, "y": 2.0, "z": 0.0},
		"zones": []any{},
	}
	data, err = ParseWorldDefinition(minimal)
	if err != nil || data.Name != "Old School RuneScape" || data.ExperienceMultiplier != 1 || data.HasGameframe {
		t.Fatalf("data = %+v err=%v", data, err)
	}

	cases := []struct {
		name  string
		world map[string]any
		want  string
	}{
		{"not object", nil, "must be an object"},
		{"zones missing", map[string]any{"spawn": map[string]any{"x": 1.0, "y": 2.0}}, "zones must be an array"},
		{"zones not array", map[string]any{"zones": 1, "spawn": map[string]any{"x": 1.0, "y": 2.0}}, "zones must be an array"},
		{"xp zero", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "experienceMultiplier": 0.0}, "must be a positive number"},
		{"xp negative", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "experienceMultiplier": -1.0}, "must be a positive number"},
		{"bad gameframe", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "gameframe": "wide"}, "gameframe must be one of"},
		{"blank name", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "name": "  "}, "name must be plain text"},
		{"long name", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "name": strings.Repeat("x", 65)}, "name must be plain text"},
		{"name with <", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "name": "a<b"}, "name must be plain text"},
		{"long description", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0}, "description": strings.Repeat("x", 121)}, "description must be a string"},
		{"spawn missing", map[string]any{"zones": []any{}}, "spawn"},
		{"spawn x out of range", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 16384.0, "y": 2.0}}, "spawn"},
		{"spawn z out of range", map[string]any{"zones": []any{}, "spawn": map[string]any{"x": 1.0, "y": 2.0, "z": 4.0}}, "spawn"},
		{"reversed bounds", map[string]any{"zones": []any{map[string]any{"minX": 10.0, "maxX": 5.0, "minY": 0.0, "maxY": 1.0, "z": 0.0, "tags": []any{"a"}}}, "spawn": map[string]any{"x": 1.0, "y": 2.0}}, "reversed bounds"},
		{"bounded no tags", map[string]any{"zones": []any{map[string]any{"minX": 0.0, "maxX": 5.0, "minY": 0.0, "maxY": 1.0, "z": 0.0, "tags": []any{}}}, "spawn": map[string]any{"x": 1.0, "y": 2.0}}, "tags must be a non-empty array"},
		{"tags not strings", map[string]any{"zones": []any{map[string]any{"tags": []any{1}}}, "spawn": map[string]any{"x": 1.0, "y": 2.0}}, "tags must contain strings"},
	}
	for _, tc := range cases {
		_, err := ParseWorldDefinition(tc.world)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: err = %v, want containing %q", tc.name, err, tc.want)
		}
	}

	// Unbounded zone with empty tags is fine.
	data, err = ParseWorldDefinition(map[string]any{
		"zones": []any{map[string]any{"tags": []any{}}},
		"spawn": map[string]any{"x": 1.0, "y": 2.0, "z": 0.0},
	})
	if err != nil || data.Zones[0].MinX != nil {
		t.Fatalf("data = %+v err=%v", data, err)
	}

	// F2P: spawn must sit inside an f2p-tagged zone.
	f2p := map[string]any{
		"plugins": map[string]any{"FreeToPlay": true},
		"zones":   []any{map[string]any{"minX": 0.0, "maxX": 10.0, "minY": 0.0, "maxY": 10.0, "z": 0.0, "tags": []any{"f2p"}}},
		"spawn":   map[string]any{"x": 50.0, "y": 50.0, "z": 0.0},
	}
	if _, err := ParseWorldDefinition(f2p); err == nil || !strings.Contains(err.Error(), "FreeToPlay needs the spawn") {
		t.Fatalf("err = %v", err)
	}
	f2p["spawn"] = map[string]any{"x": 5.0, "y": 5.0, "z": 0.0}
	if data, err := ParseWorldDefinition(f2p); err != nil || data.MembersWorld {
		t.Fatalf("data = %+v err=%v", data, err)
	}
}

func TestCheckConfigValue(t *testing.T) {
	boolDecl := ConfigOptionDeclaration{Type: ConfigTypeBoolean}
	if _, err := CheckConfigValue(boolDecl, "true"); err == nil || err.Error() != "must be true or false" {
		t.Fatalf("err = %v", err)
	}
	if got, err := CheckConfigValue(boolDecl, true); err != nil || got != true {
		t.Fatalf("got=%v err=%v", got, err)
	}

	min, max := 1.0, 10.0
	numDecl := ConfigOptionDeclaration{Type: ConfigTypeNumber, Integer: true, Min: &min, Max: &max}
	if got, err := CheckConfigValue(numDecl, "5"); err != nil || got != 5.0 {
		t.Fatalf("numeric string: got=%v err=%v", got, err)
	}
	if _, err := CheckConfigValue(numDecl, 5.5); err == nil || err.Error() != "must be a whole number" {
		t.Fatalf("err = %v", err)
	}
	if _, err := CheckConfigValue(numDecl, 0.0); err == nil || err.Error() != "must be at least 1" {
		t.Fatalf("err = %v", err)
	}
	if _, err := CheckConfigValue(numDecl, 11.0); err == nil || err.Error() != "must be at most 10" {
		t.Fatalf("err = %v", err)
	}
	if _, err := CheckConfigValue(numDecl, "abc"); err == nil || err.Error() != "must be a number" {
		t.Fatalf("err = %v", err)
	}
	if _, err := CheckConfigValue(numDecl, ""); err == nil || err.Error() != "must be a number" {
		t.Fatalf("err = %v", err)
	}
	exclDecl := ConfigOptionDeclaration{Type: ConfigTypeNumber, Min: &min, ExclusiveMin: true}
	if _, err := CheckConfigValue(exclDecl, 1.0); err == nil || err.Error() != "must be greater than 1" {
		t.Fatalf("err = %v", err)
	}

	strDecl := ConfigOptionDeclaration{Type: ConfigTypeString}
	if _, err := CheckConfigValue(strDecl, 5.0); err == nil || err.Error() != "must be text" {
		t.Fatalf("err = %v", err)
	}

	enumDecl := ConfigOptionDeclaration{Type: ConfigTypeEnum, Values: []ConfigEnumValue{
		{Value: "a", Label: "A"}, {Value: float64(2), Label: "Two"},
	}}
	if got, err := CheckConfigValue(enumDecl, "a"); err != nil || got != "a" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if got, err := CheckConfigValue(enumDecl, "2"); err != nil || got != float64(2) {
		t.Fatalf("numeric string enum: got=%v err=%v", got, err)
	}
	if _, err := CheckConfigValue(enumDecl, "b"); err == nil || err.Error() != "must be one of a, 2" {
		t.Fatalf("err = %v", err)
	}

	dateDecl := ConfigOptionDeclaration{Type: ConfigTypeDate}
	if got, err := CheckConfigValue(dateDecl, "2026-02-28"); err != nil || got != "2026-02-28" {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := CheckConfigValue(dateDecl, "2026-02-30"); err == nil || err.Error() != "must be a real calendar date" {
		t.Fatalf("err = %v", err)
	}
	if _, err := CheckConfigValue(dateDecl, "28/02/2026"); err == nil || err.Error() != "must be a date as YYYY-MM-DD" {
		t.Fatalf("err = %v", err)
	}

	jsonDecl := ConfigOptionDeclaration{Type: ConfigTypeJSON}
	if got, err := CheckConfigValue(jsonDecl, nil); err != nil || got != nil {
		t.Fatalf("got=%v err=%v", got, err)
	}
	if _, err := CheckConfigValue(ConfigOptionDeclaration{Type: "bogus"}, 1); err == nil || err.Error() != "unknown option type" {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateDeclaration(t *testing.T) {
	good := map[string]any{
		"type": "boolean", "label": "Flag", "help": "Does a thing.", "default": true,
	}
	if problems := ValidateDeclaration("plug:flag", good); len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	cases := []struct {
		key  string
		decl any
		want string
	}{
		{"badkey", good, "key must be namespace:key"},
		{"a:b c", good, "key must be namespace:key"},
		{"plug:flag", "nope", "declaration must be an object"},
		{"plug:flag", map[string]any{"type": "bogus", "label": "L", "help": "H", "default": 1}, "type must be one of"},
		{"plug:flag", map[string]any{"type": "boolean", "help": "H", "default": true}, "label is required"},
		{"plug:flag", map[string]any{"type": "boolean", "label": "L", "default": true}, "help is required"},
		{"plug:flag", map[string]any{"type": "boolean", "label": "L", "help": "H"}, "exactly one of default or defaultText"},
		{"plug:flag", map[string]any{"type": "boolean", "label": "L", "help": "H", "default": true, "defaultText": "x"}, "exactly one of default or defaultText"},
		{"plug:flag", map[string]any{"type": "boolean", "label": "L", "help": "H", "defaultText": "  "}, "defaultText must be a non-empty string"},
		{"plug:flag", map[string]any{"type": "boolean", "label": "L", "help": "H", "default": true, "bogus": 1}, "unknown field bogus"},
		{"plug:flag", map[string]any{"type": "enum", "label": "L", "help": "H", "default": "a"}, "enum needs a non-empty values list"},
		{"plug:flag", map[string]any{"type": "enum", "label": "L", "help": "H", "default": "a", "values": []any{map[string]any{"value": "a", "label": "A"}, map[string]any{"value": "a", "label": "A2"}}}, "enum values must be unique"},
	}
	for _, tc := range cases {
		problems := ValidateDeclaration(tc.key, tc.decl)
		found := false
		for _, p := range problems {
			if strings.Contains(p, tc.want) {
				found = true
			}
		}
		if !found {
			t.Fatalf("key=%q decl=%v: problems=%v, want containing %q", tc.key, tc.decl, problems, tc.want)
		}
	}
}

func TestWorldSettingKeys(t *testing.T) {
	keys := WorldSettingKeys()
	want := []string{"name", "description", "experienceMultiplier", "spawn", "gameframe"}
	if len(keys) != len(want) {
		t.Fatalf("keys = %v", keys)
	}
	for i, k := range want {
		if keys[i] != k {
			t.Fatalf("keys = %v", keys)
		}
	}
	if len(CoreConfigDeclarations()) != 1 || CoreConfigDeclarations()[0].Key != "commands:permissions" {
		t.Fatal("core declarations wrong")
	}
}

func planLayers() WorldConfigLayers {
	return WorldConfigLayers{
		Shipped: map[string]any{
			"name":  "Shipped",
			"spawn": map[string]any{"x": 3222.0, "y": 3218.0, "z": 0.0},
			"zones": []any{map[string]any{"minX": 3200.0, "maxX": 3300.0, "minY": 3200.0, "maxY": 3300.0, "z": 0.0, "tags": []any{"f2p"}}},
		},
		Local: map[string]any{},
	}
}

func TestPlanWorldConfig(t *testing.T) {
	// World edit: stored, and diffed.
	plan, err := PlanWorldConfig(planLayers(), WorldConfigChanges{World: map[string]any{"name": "New Name"}}, WorldConfigCatalogData{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextLocal["name"] != "New Name" || len(plan.Errors) != 0 {
		t.Fatalf("plan = %+v", plan)
	}
	if len(plan.Diff) != 1 || plan.Diff[0].Path != "name" || plan.Diff[0].Before != nil || plan.Diff[0].After != "New Name" {
		t.Fatalf("diff = %+v", plan.Diff)
	}

	// Shipped-equal edit deletes the local key.
	layers := planLayers()
	layers.Local = map[string]any{"name": "Old Local"}
	plan, err = PlanWorldConfig(layers, WorldConfigChanges{World: map[string]any{"name": "Shipped"}}, WorldConfigCatalogData{})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := plan.NextLocal["name"]; present {
		t.Fatalf("nextLocal = %v", plan.NextLocal)
	}
	if len(plan.Diff) != 1 || plan.Diff[0].After != nil {
		t.Fatalf("diff = %+v", plan.Diff)
	}

	// Null resets to shipped.
	plan, err = PlanWorldConfig(layers, WorldConfigChanges{World: map[string]any{"name": nil}}, WorldConfigCatalogData{})
	if err != nil {
		t.Fatal(err)
	}
	if _, present := plan.NextLocal["name"]; present {
		t.Fatalf("nextLocal = %v", plan.NextLocal)
	}

	// Undeclared world key is an error.
	plan, err = PlanWorldConfig(planLayers(), WorldConfigChanges{World: map[string]any{"bogus": 1}}, WorldConfigCatalogData{})
	if err != nil || len(plan.Errors) != 1 || !strings.Contains(plan.Errors[0], "world.bogus is not a world setting") {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}

	// Section application, per-subkey diff.
	plan, err = PlanWorldConfig(planLayers(), WorldConfigChanges{
		Plugins:      map[string]any{"Foo": false},
		PluginConfig: map[string]any{"foo:rate": 5.0},
	}, WorldConfigCatalogData{})
	if err != nil {
		t.Fatal(err)
	}
	if plan.NextLocal["plugins"].(map[string]any)["Foo"] != false {
		t.Fatalf("nextLocal = %v", plan.NextLocal)
	}
	if len(plan.Diff) != 2 || plan.Diff[0].Path != "pluginConfig.foo:rate" || plan.Diff[1].Path != "plugins.Foo" {
		t.Fatalf("diff = %+v", plan.Diff)
	}

	// Invalid merged world is an error.
	plan, err = PlanWorldConfig(planLayers(), WorldConfigChanges{World: map[string]any{"experienceMultiplier": 0.0}}, WorldConfigCatalogData{})
	if err != nil || len(plan.Errors) == 0 {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
}

func TestPlanWorldConfigCatalogChecks(t *testing.T) {
	catalog := WorldConfigCatalogData{
		Options: []CatalogOption{
			{Key: "foo:flag", Decl: ConfigOptionDeclaration{Type: ConfigTypeBoolean, Label: "Flag"}},
			{Key: "foo:membersOnly", Decl: ConfigOptionDeclaration{Type: ConfigTypeBoolean, Label: "M", Members: true}},
		},
	}
	// Bad value for a declared option is an error.
	plan, err := PlanWorldConfig(planLayers(), WorldConfigChanges{
		PluginConfig: map[string]any{"foo:flag": "yes"},
	}, catalog)
	if err != nil || len(plan.Errors) != 1 || !strings.Contains(plan.Errors[0], "foo:flag: must be true or false") {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
	// Undeclared local key is a warning.
	plan, err = PlanWorldConfig(planLayers(), WorldConfigChanges{
		PluginConfig: map[string]any{"nope:key": 1.0},
	}, catalog)
	if err != nil || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "not declared by any plugin") {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
	// Members option on a free-to-play world warns.
	layers := planLayers()
	layers.Shipped["plugins"] = map[string]any{"FreeToPlay": true}
	plan, err = PlanWorldConfig(layers, WorldConfigChanges{
		PluginConfig: map[string]any{"foo:membersOnly": true},
	}, catalog)
	if err != nil || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "members option") {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
}

func TestPlanWorldConfigCutOffAndMembersWarnings(t *testing.T) {
	catalog := WorldConfigCatalogData{
		Plugins: []CatalogPlugin{
			{Name: "Base", File: "Base.plugin.js", DefaultEnabled: true, Enabled: true},
			{Name: "Addon", File: "Addon.plugin.js", DefaultEnabled: true, Enabled: true, DependsOn: []string{"Base"}},
		},
	}
	plan, err := PlanWorldConfig(planLayers(), WorldConfigChanges{
		Plugins: map[string]any{"Base": false},
	}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "Turning off Base also stops") || !strings.Contains(plan.Warnings[0], "Addon") {
		t.Fatalf("warnings = %v", plan.Warnings)
	}

	// Members plugins on a free-to-play world warn.
	layers := planLayers()
	layers.Shipped["plugins"] = map[string]any{"FreeToPlay": true}
	catalog2 := WorldConfigCatalogData{
		Plugins: []CatalogPlugin{
			{Name: "MembersOnly", File: "MembersOnly.plugin.js", DefaultEnabled: true, Enabled: true, Members: true},
		},
	}
	plan, err = PlanWorldConfig(layers, WorldConfigChanges{}, catalog2)
	if err != nil || len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "Members plugins are switched on") {
		t.Fatalf("plan = %+v err=%v", plan, err)
	}
}

func TestPlanWorldConfigLegacyMigration(t *testing.T) {
	layers := planLayers()
	layers.Local = map[string]any{"disabledPlugins": []any{"Foo"}}
	catalog := WorldConfigCatalogData{
		Plugins: []CatalogPlugin{
			{Name: "Foo", File: "Foo.plugin.js", DefaultEnabled: true, Enabled: false},
			{Name: "Bar", File: "Bar.plugin.js", DefaultEnabled: true, Enabled: true},
		},
	}
	plan, err := PlanWorldConfig(layers, WorldConfigChanges{}, catalog)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := plan.NextLocal["disabledPlugins"]; present {
		t.Fatalf("nextLocal = %v", plan.NextLocal)
	}
	plugins := plan.NextLocal["plugins"].(map[string]any)
	if plugins["Foo"] != false {
		t.Fatalf("plugins = %v", plugins)
	}
	// Bar needs no explicit entry: with no switch it resolves to
	// enabled, the same state the legacy list gave it. Only
	// plugins whose state would change get written.
	if len(plugins) != 1 {
		t.Fatalf("plugins = %v", plugins)
	}
}

func TestPlanWorldConfigMergeError(t *testing.T) {
	// A local layer that cannot normalize fails the plan.
	layers := planLayers()
	layers.Local = map[string]any{"plugins": map[string]any{"F": map[string]any{"enabled": 1}}}
	if _, err := PlanWorldConfig(layers, WorldConfigChanges{}, WorldConfigCatalogData{}); err == nil {
		t.Fatal("plan must fail on a bad local layer")
	}
}

func TestWriteWorldConfig(t *testing.T) {
	dir := writeWorldFiles(t, map[string]string{"world.json": validWorldJSON})
	// Absent file: expected hash "".
	if err := WriteWorldConfig(dir, map[string]any{"name": "Local"}, ""); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "world.local.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(raw), "}\n") || !strings.Contains(string(raw), `  "name": "Local"`) {
		t.Fatalf("written = %q", raw)
	}
	hash := sha256Of(t, string(raw))
	// Round-trip: the hash matches what Read reports.
	layers, err := ReadWorldConfigLayers(dir)
	if err != nil || layers.LocalHash != hash || layers.Local["name"] != "Local" {
		t.Fatalf("layers = %+v err=%v", layers, err)
	}
	// Stale hash refused.
	if err := WriteWorldConfig(dir, map[string]any{"name": "Other"}, "deadbeef"); err == nil || !strings.Contains(err.Error(), "changed since loaded") {
		t.Fatalf("err = %v", err)
	}
	// Second write with the fresh hash works.
	if err := WriteWorldConfig(dir, map[string]any{"name": "Other"}, hash); err != nil {
		t.Fatal(err)
	}
	// Unparseable current file refused (with its own hash).
	bad := "{broken"
	if err := os.WriteFile(filepath.Join(dir, "world.local.json"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteWorldConfig(dir, map[string]any{}, sha256Of(t, bad)); err == nil || !strings.Contains(err.Error(), "does not parse") {
		t.Fatalf("err = %v", err)
	}
	// Non-object current file refused.
	if err := os.WriteFile(filepath.Join(dir, "world.local.json"), []byte(`[]`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteWorldConfig(dir, map[string]any{}, sha256Of(t, `[]`)); err == nil || !strings.Contains(err.Error(), "does not parse as an object") {
		t.Fatalf("err = %v", err)
	}
	// No temp files left behind.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".tmp") {
			t.Fatalf("temp file left: %s", entry.Name())
		}
	}
}
