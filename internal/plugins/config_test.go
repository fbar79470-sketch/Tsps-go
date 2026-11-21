package plugins

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

func boolPtr(b bool) *bool        { return &b }
func floatPtr(f float64) *float64 { return &f }

func validDecl() ConfigDeclaration {
	return ConfigDeclaration{Type: "boolean", Label: "Opt", Help: "Help.", Default: true, HasDefault: true}
}

func TestValidateDeclaration(t *testing.T) {
	for _, tc := range []struct {
		name string
		key  string
		decl ConfigDeclaration
		want []string // nil means valid
	}{
		{"valid boolean", "ns:opt", validDecl(), nil},
		{"valid number with range", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 5.0, HasDefault: true, Min: floatPtr(0), Max: floatPtr(10)}, nil},
		{"valid string", "ns:s", ConfigDeclaration{Type: "string", Label: "S", Help: "H.", Default: "x", HasDefault: true}, nil},
		{"valid enum", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true, Values: []ConfigEnumValue{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}}, nil},
		{"valid date", "ns:d", ConfigDeclaration{Type: "date", Label: "D", Help: "H.", Default: "2026-11-14", HasDefault: true}, nil},
		{"valid json", "ns:j", ConfigDeclaration{Type: "json", Label: "J", Help: "H.", Default: map[string]any{"a": 1.0}, HasDefault: true}, nil},
		{"valid json null default", "ns:j", ConfigDeclaration{Type: "json", Label: "J", Help: "H.", Default: nil, HasDefault: true}, nil},
		{"valid defaultText only", "ns:t", ConfigDeclaration{Type: "string", Label: "T", Help: "H.", DefaultText: "Set in world.json", HasDefaultText: true}, nil},
		{"valid number int default", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 5, HasDefault: true}, nil},

		{"key without colon", "nope", validDecl(), []string{"nope: key must be namespace:key"}},
		{"key with two colons", "a:b:c", validDecl(), []string{"a:b:c: key must be namespace:key"}},
		{"key with space", "a:b c", validDecl(), []string{"a:b c: key must be namespace:key"}},
		{"empty key", "", validDecl(), []string{": key must be namespace:key"}},
		{"key with empty namespace", ":opt", validDecl(), []string{":opt: key must be namespace:key"}},
		{"key with empty setting", "ns:", validDecl(), []string{"ns:: key must be namespace:key"}},

		{"bad type", "ns:o", ConfigDeclaration{Type: "float", Label: "L", Help: "H.", Default: 1.0, HasDefault: true}, []string{"ns:o: type must be one of boolean, number, string, enum, date, json"}},
		{"empty type", "ns:o", ConfigDeclaration{Label: "L", Help: "H.", Default: 1.0, HasDefault: true}, []string{"ns:o: type must be one of boolean, number, string, enum, date, json"}},
		{"capitalized type", "ns:o", ConfigDeclaration{Type: "Boolean", Label: "L", Help: "H.", Default: true, HasDefault: true}, []string{"ns:o: type must be one of boolean, number, string, enum, date, json"}},
		{"missing label", "ns:o", ConfigDeclaration{Type: "boolean", Help: "H.", Default: true, HasDefault: true}, []string{"ns:o: label is required"}},
		{"blank label", "ns:o", ConfigDeclaration{Type: "boolean", Label: "  ", Help: "H.", Default: true, HasDefault: true}, []string{"ns:o: label is required"}},
		{"missing help", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Default: true, HasDefault: true}, []string{"ns:o: help is required"}},
		{"blank help", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Help: "\t", Default: true, HasDefault: true}, []string{"ns:o: help is required"}},

		{"both default and defaultText", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Help: "H.", Default: true, HasDefault: true, DefaultText: "x", HasDefaultText: true}, []string{"ns:o: set exactly one of default or defaultText"}},
		{"neither default nor defaultText", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Help: "H."}, []string{"ns:o: set exactly one of default or defaultText"}},
		{"empty defaultText", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Help: "H.", DefaultText: "", HasDefaultText: true}, []string{"ns:o: defaultText must be a non-empty string"}},
		{"blank defaultText", "ns:o", ConfigDeclaration{Type: "boolean", Label: "L", Help: "H.", DefaultText: "  ", HasDefaultText: true}, []string{"ns:o: defaultText must be a non-empty string"}},

		{"enum without values", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true}, []string{"ns:e: enum needs a non-empty values list of { value, label }"}},
		{"enum with empty values", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true, Values: []ConfigEnumValue{}}, []string{"ns:e: enum needs a non-empty values list of { value, label }"}},
		{"enum with bool value", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true, Values: []ConfigEnumValue{{Value: true, Label: "T"}}}, []string{"ns:e: enum needs a non-empty values list of { value, label }"}},
		{"enum duplicate values", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true, Values: []ConfigEnumValue{{Value: "a", Label: "A"}, {Value: "a", Label: "A2"}}}, []string{"ns:e: enum values must be unique"}},
		{"enum duplicate numeric values across spellings", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: 1, HasDefault: true, Values: []ConfigEnumValue{{Value: 1, Label: "A"}, {Value: 1.0, Label: "B"}}}, []string{"ns:e: enum values must be unique"}},
		{"enum default not listed", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "c", HasDefault: true, Values: []ConfigEnumValue{{Value: "a", Label: "A"}, {Value: "b", Label: "B"}}}, []string{`ns:e: default is invalid: must be exactly one of "a", "b"`}},
		{"enum default wrong scalar kind", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: 5, HasDefault: true, Values: []ConfigEnumValue{{Value: "5", Label: "Five"}}}, []string{`ns:e: default is invalid: must be exactly one of "5"`}},
		{"values on string", "ns:s", ConfigDeclaration{Type: "string", Label: "S", Help: "H.", Default: "x", HasDefault: true, Values: []ConfigEnumValue{{Value: "x", Label: "X"}}}, []string{"ns:s: values is only for enum"}},
		{"values on number", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.0, HasDefault: true, Values: []ConfigEnumValue{{Value: 1.0, Label: "One"}}}, []string{"ns:n: values is only for enum"}},

		{"min on string", "ns:s", ConfigDeclaration{Type: "string", Label: "S", Help: "H.", Default: "x", HasDefault: true, Min: floatPtr(0)}, []string{"ns:s: min is only for number"}},
		{"max on boolean", "ns:b", ConfigDeclaration{Type: "boolean", Label: "B", Help: "H.", Default: true, HasDefault: true, Max: floatPtr(1)}, []string{"ns:b: max is only for number"}},
		{"integer on string", "ns:s", ConfigDeclaration{Type: "string", Label: "S", Help: "H.", Default: "x", HasDefault: true, Integer: boolPtr(false)}, []string{"ns:s: integer is only for number"}},
		{"exclusiveMin on enum", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: "a", HasDefault: true, Values: []ConfigEnumValue{{Value: "a", Label: "A"}}, ExclusiveMin: boolPtr(true)}, []string{"ns:e: exclusiveMin is only for number", "ns:e: exclusiveMin needs min"}},
		{"min NaN", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.0, HasDefault: true, Min: floatPtr(math.NaN())}, []string{"ns:n: min must be a finite number"}},
		{"max infinite", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.0, HasDefault: true, Max: floatPtr(math.Inf(1))}, []string{"ns:n: max must be a finite number"}},
		{"exclusiveMin without min", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.0, HasDefault: true, ExclusiveMin: boolPtr(true)}, []string{"ns:n: exclusiveMin needs min"}},
		{"min greater than max", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.0, HasDefault: true, Min: floatPtr(10), Max: floatPtr(5)}, []string{"ns:n: min must not be greater than max"}},

		{"boolean default as text", "ns:b", ConfigDeclaration{Type: "boolean", Label: "B", Help: "H.", Default: "true", HasDefault: true}, []string{"ns:b: default is invalid: must be true or false"}},
		{"number default as text", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: "5", HasDefault: true}, []string{"ns:n: default is invalid: must be a number, not text"}},
		{"string default as number", "ns:s", ConfigDeclaration{Type: "string", Label: "S", Help: "H.", Default: 5, HasDefault: true}, []string{"ns:s: default is invalid: must be text"}},
		{"date default as number", "ns:d", ConfigDeclaration{Type: "date", Label: "D", Help: "H.", Default: 20261114, HasDefault: true}, []string{"ns:d: default is invalid: must be text"}},
		{"enum default as bool", "ns:e", ConfigDeclaration{Type: "enum", Label: "E", Help: "H.", Default: true, HasDefault: true, Values: []ConfigEnumValue{{Value: "a", Label: "A"}}}, []string{"ns:e: default is invalid: must be one of the values"}},
		{"json default as func", "ns:j", ConfigDeclaration{Type: "json", Label: "J", Help: "H.", Default: func() {}, HasDefault: true}, []string{"ns:j: default is invalid: must be a JSON value"}},
		{"json default with NaN nested", "ns:j", ConfigDeclaration{Type: "json", Label: "J", Help: "H.", Default: []any{math.NaN()}, HasDefault: true}, []string{"ns:j: default is invalid: must be a JSON value"}},
		{"number default below min", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 3.0, HasDefault: true, Min: floatPtr(10)}, []string{"ns:n: default is invalid: must be at least 10"}},
		{"number default at exclusive min", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 0.0, HasDefault: true, Min: floatPtr(0), ExclusiveMin: boolPtr(true)}, []string{"ns:n: default is invalid: must be greater than 0"}},
		{"number default above max", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 11.0, HasDefault: true, Max: floatPtr(10)}, []string{"ns:n: default is invalid: must be at most 10"}},
		{"integer default fractional", "ns:n", ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 1.5, HasDefault: true, Integer: boolPtr(true)}, []string{"ns:n: default is invalid: must be a whole number"}},
		{"date default wrong shape", "ns:d", ConfigDeclaration{Type: "date", Label: "D", Help: "H.", Default: "14/11/2026", HasDefault: true}, []string{"ns:d: default is invalid: must be a date as YYYY-MM-DD"}},
		{"date default impossible day", "ns:d", ConfigDeclaration{Type: "date", Label: "D", Help: "H.", Default: "2026-02-30", HasDefault: true}, []string{"ns:d: default is invalid: must be a real calendar date"}},

		// The default check runs only when nothing else is wrong:
		// a bad label suppresses the bad-default problem.
		{"default check suppressed by earlier problems", "ns:b", ConfigDeclaration{Type: "boolean", Label: "", Help: "H.", Default: "not-a-bool", HasDefault: true}, []string{"ns:b: label is required"}},
		// Multiple independent problems accumulate in TS order.
		{"multiple problems in order", "bad key", ConfigDeclaration{Type: "nope", Label: "", Help: ""}, []string{
			"bad key: key must be namespace:key",
			"bad key: type must be one of boolean, number, string, enum, date, json",
			"bad key: label is required",
			"bad key: help is required",
			"bad key: set exactly one of default or defaultText",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := ValidateDeclaration(tc.key, tc.decl)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ValidateDeclaration(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestCheckConfigValue(t *testing.T) {
	num := func(d ConfigDeclaration) ConfigDeclaration { d.Type = "number"; return d }
	for _, tc := range []struct {
		name  string
		decl  ConfigDeclaration
		value any
		want  configCheck
	}{
		{"boolean true", ConfigDeclaration{Type: "boolean"}, true, configCheck{ok: true, value: true}},
		{"boolean rejects text", ConfigDeclaration{Type: "boolean"}, "true", configCheck{err: "must be true or false"}},
		{"number int", num(ConfigDeclaration{}), 5, configCheck{ok: true, value: 5.0}},
		{"number coerces string", num(ConfigDeclaration{}), "5", configCheck{ok: true, value: 5.0}},
		{"number coerces padded string", num(ConfigDeclaration{}), "  5.5 ", configCheck{ok: true, value: 5.5}},
		{"number rejects blank string", num(ConfigDeclaration{}), "  ", configCheck{err: "must be a number"}},
		{"number rejects text", num(ConfigDeclaration{}), "abc", configCheck{err: "must be a number"}},
		{"number rejects bool", num(ConfigDeclaration{}), true, configCheck{err: "must be a number"}},
		{"number rejects NaN", num(ConfigDeclaration{}), math.NaN(), configCheck{err: "must be a number"}},
		{"number rejects underscore string", num(ConfigDeclaration{}), "1_000", configCheck{err: "must be a number"}},
		{"integer accepts whole float", num(ConfigDeclaration{Integer: boolPtr(true)}), 5.0, configCheck{ok: true, value: 5.0}},
		{"integer rejects fraction", num(ConfigDeclaration{Integer: boolPtr(true)}), 5.5, configCheck{err: "must be a whole number"}},
		{"min boundary ok", num(ConfigDeclaration{Min: floatPtr(10)}), 10.0, configCheck{ok: true, value: 10.0}},
		{"exclusive min boundary rejected", num(ConfigDeclaration{Min: floatPtr(10), ExclusiveMin: boolPtr(true)}), 10.0, configCheck{err: "must be greater than 10"}},
		{"max boundary ok", num(ConfigDeclaration{Max: floatPtr(10)}), 10.0, configCheck{ok: true, value: 10.0}},
		{"string ok", ConfigDeclaration{Type: "string"}, "x", configCheck{ok: true, value: "x"}},
		{"string rejects number", ConfigDeclaration{Type: "string"}, 5, configCheck{err: "must be text"}},
		{"enum string match", ConfigDeclaration{Type: "enum", Values: []ConfigEnumValue{{Value: "a", Label: "A"}}}, "a", configCheck{ok: true, value: "a"}},
		{"enum string counts as number", ConfigDeclaration{Type: "enum", Values: []ConfigEnumValue{{Value: 5, Label: "Five"}}}, "5", configCheck{ok: true, value: 5}},
		{"enum number does not match string option", ConfigDeclaration{Type: "enum", Values: []ConfigEnumValue{{Value: "5", Label: "Five"}}}, 5, configCheck{err: "must be one of 5"}},
		{"enum no match lists values", ConfigDeclaration{Type: "enum", Values: []ConfigEnumValue{{Value: "a", Label: "A"}, {Value: 2, Label: "Two"}}}, "z", configCheck{err: "must be one of a, 2"}},
		{"date valid", ConfigDeclaration{Type: "date"}, "2024-02-29", configCheck{ok: true, value: "2024-02-29"}},
		{"date non-leap Feb 29", ConfigDeclaration{Type: "date"}, "2026-02-29", configCheck{err: "must be a real calendar date"}},
		{"date wrong shape", ConfigDeclaration{Type: "date"}, "2026-2-3", configCheck{err: "must be a date as YYYY-MM-DD"}},
		{"json accepts anything", ConfigDeclaration{Type: "json"}, nil, configCheck{ok: true, value: nil}},
		{"unknown type", ConfigDeclaration{Type: "nope"}, 1, configCheck{err: "unknown option type"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := checkConfigValue(tc.decl, tc.value); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("checkConfigValue = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestPluginConfigKey(t *testing.T) {
	for _, tc := range []struct{ plugin, setting, want string }{
		{"LeagueMode", "mode", "league:mode"},
		{"LEAGUEMODE", "mode", "league:mode"},
		{"RandomEvents", "rate", "random-events:rate"},
		{"Characters", "slots", "characters:slots"},
		{"Quests", "list", "quests:list"},
		{"league-mode", "mode", "league-mode:mode"}, // lowercase only: no stripping
		{"MyPlugin", "opt", "MyPlugin:opt"},         // case preserved for normal plugins
		{"Foo", "a:b", "Foo:a:b"},                   // no validation here; TS does none either
	} {
		if got := pluginConfigKey(tc.plugin, tc.setting); got != tc.want {
			t.Errorf("pluginConfigKey(%q, %q) = %q, want %q", tc.plugin, tc.setting, got, tc.want)
		}
	}
}

func TestHubGetPluginConfig(t *testing.T) {
	hub, _ := newLoaderTestHub(t, nil, HubOptions{PluginConfig: map[string]any{
		"MyPlugin:opt": "written",
		"P:null":       nil,
		"P:num":        "5",
	}})
	if got := hub.GetPluginConfig("MyPlugin:opt", "def"); got != "written" {
		t.Errorf("present key = %v, want written", got)
	}
	if got := hub.GetPluginConfig("missing:key", "def"); got != "def" {
		t.Errorf("missing key = %v, want def", got)
	}
	if got := hub.GetPluginConfig("", "def"); got != "def" {
		t.Errorf("empty key = %v, want def", got)
	}
	// Present-with-null returns nil, not the default (hasOwnProperty).
	if got := hub.GetPluginConfig("P:null", "def"); got != nil {
		t.Errorf("null key = %v, want nil", got)
	}
	// No coercion at read time: "5" stays a string.
	if got := hub.GetPluginConfig("P:num", 0); got != "5" {
		t.Errorf("value = %v (%T), want string 5", got, got)
	}
	// No prefixing at the hub level: a bare key does not find the
	// prefixed entry. Prefixing is Api.GetPluginConfig's job.
	if got := hub.GetPluginConfig("opt", "def"); got != "def" {
		t.Errorf("bare key = %v, want def", got)
	}
}

func TestCoreConfigDeclarationSetIsEmpty(t *testing.T) {
	if len(coreConfigDeclarations) != 0 {
		t.Errorf("core declaration set must be empty, has %d entries", len(coreConfigDeclarations))
	}
	hub, _ := newLoaderTestHub(t, nil, HubOptions{})
	if len(hub.ConfigDeclarations()) != 0 {
		t.Errorf("ConfigDeclarations with no plugins must be empty: %v", hub.ConfigDeclarations())
	}
}

func TestConfigDeclarationClashFirstLoadedWins(t *testing.T) {
	decl := func(label string) ConfigDeclaration {
		return ConfigDeclaration{Type: "boolean", Label: label, Help: "H.", Default: true, HasDefault: true}
	}
	noop := func(*Api) {}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "First", Config: map[string]ConfigDeclaration{"shared:opt": decl("First")}, Register: noop},
		{Name: "Second", Config: map[string]ConfigDeclaration{"shared:opt": decl("Second")}, Register: noop},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "config key shared:opt is declared by both First and Second; First is expected to own it") {
		t.Errorf("missing clash log:\n%s", log.String())
	}
	decls := hub.ConfigDeclarations()
	entry, ok := decls["shared:opt"]
	if !ok {
		t.Fatalf("shared:opt not recorded: %v", decls)
	}
	if entry.Owner != "First" || entry.Decl.Label != "First" {
		t.Errorf("owner = %+v, want First's declaration", entry)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"First", "Second"}) {
		t.Errorf("Loaded = %v, want both plugins (a clash never blocks loading)", got)
	}
}

func TestConfigDeclarationOwnershipGoesToFirstThatLoads(t *testing.T) {
	decl := ConfigDeclaration{Type: "boolean", Label: "L", Help: "H.", Default: true, HasDefault: true}
	hub, _ := newLoaderTestHub(t, []Plugin{
		{Name: "Crashing", Config: map[string]ConfigDeclaration{"shared:opt": decl}, Register: func(*Api) { panic("boom") }},
		{Name: "Survivor", Config: map[string]ConfigDeclaration{"shared:opt": decl}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	entry, ok := hub.ConfigDeclarations()["shared:opt"]
	if !ok || entry.Owner != "Survivor" {
		t.Errorf("owner = %+v (present=%v), want Survivor: a failed plugin owns nothing", entry, ok)
	}
}

func TestConfigDeclarationCoreKeyWins(t *testing.T) {
	// White-box: temporarily give the core a key to exercise the
	// core-wins branch. The shipped set is empty (see
	// TestCoreConfigDeclarationSetIsEmpty).
	coreConfigDeclarations["core:opt"] = ConfigDeclaration{Type: "boolean", Label: "Core", Help: "H.", Default: true, HasDefault: true}
	defer delete(coreConfigDeclarations, "core:opt")

	decl := ConfigDeclaration{Type: "boolean", Label: "Plugin", Help: "H.", Default: false, HasDefault: true}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Claimant", Config: map[string]ConfigDeclaration{"core:opt": decl}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	if !strings.Contains(log.String(), "config key core:opt is declared by both core and Claimant; core owns it") {
		t.Errorf("missing core-wins log:\n%s", log.String())
	}
	entry := hub.ConfigDeclarations()["core:opt"]
	if entry.Owner != "core" || entry.Decl.Label != "Core" {
		t.Errorf("core:opt = %+v, want the core declaration", entry)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Claimant"}) {
		t.Errorf("Loaded = %v: dropping a declaration must not block the plugin", got)
	}
}

func TestBadConfigDeclarationDroppedWithoutFailingLoad(t *testing.T) {
	bad := ConfigDeclaration{Type: "boolean", Label: "", Help: "H.", Default: true, HasDefault: true} // missing label
	good := ConfigDeclaration{Type: "boolean", Label: "Good", Help: "H.", Default: true, HasDefault: true}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "Mixed", Config: map[string]ConfigDeclaration{
			"mixed:bad":  bad,
			"mixed:good": good,
			"notakey":    good, // bad key shape
		}, Register: func(*Api) {}},
	}, HubOptions{})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, "Mixed: invalid config declaration, ignored: mixed:bad: label is required") {
		t.Errorf("missing bad-declaration log:\n%s", out)
	}
	if !strings.Contains(out, "notakey: key must be namespace:key") {
		t.Errorf("missing bad-key log:\n%s", out)
	}
	decls := hub.ConfigDeclarations()
	if _, ok := decls["mixed:good"]; !ok {
		t.Errorf("good declaration dropped: %v", decls)
	}
	if _, ok := decls["mixed:bad"]; ok {
		t.Errorf("bad declaration recorded: %v", decls)
	}
	if got := hub.Loaded(); !reflect.DeepEqual(got, []string{"Mixed"}) {
		t.Errorf("Loaded = %v, want Mixed loaded despite bad declarations", got)
	}
	if !strings.Contains(out, "active=1 disabled=0") {
		t.Errorf("missing counts log:\n%s", out)
	}
}

func TestWarnAboutInvalidConfigValues(t *testing.T) {
	decl := ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 20.0, HasDefault: true, Min: floatPtr(10)}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "P", Config: map[string]ConfigDeclaration{"p:n": decl}, Register: func(*Api) {}},
	}, HubOptions{PluginConfig: map[string]any{
		"p:n":          5.0,        // declared, invalid value: warn
		"undeclared:x": "junk",     // no declaration: silent
		"other:y":      "whatever", // no declaration: silent
	}})
	hub.Load()
	out := log.String()
	if !strings.Contains(out, `world.json pluginConfig "p:n" (P) must be at least 10; the value is used as written`) {
		t.Errorf("missing invalid-value warning:\n%s", out)
	}
	if strings.Contains(out, "undeclared:x") || strings.Contains(out, "other:y") {
		t.Errorf("undeclared keys must not warn:\n%s", out)
	}
	// The invalid value is still returned as written.
	if got := hub.GetPluginConfig("p:n", 0.0); got != 5.0 {
		t.Errorf("GetPluginConfig = %v, want 5 as written", got)
	}
}

func TestWarnAboutInvalidConfigValuesSkipsValid(t *testing.T) {
	decl := ConfigDeclaration{Type: "number", Label: "N", Help: "H.", Default: 20.0, HasDefault: true, Min: floatPtr(10)}
	hub, log := newLoaderTestHub(t, []Plugin{
		{Name: "P", Config: map[string]ConfigDeclaration{"p:n": decl}, Register: func(*Api) {}},
	}, HubOptions{PluginConfig: map[string]any{"p:n": 15.0}})
	hub.Load()
	if strings.Contains(log.String(), "pluginConfig") {
		t.Errorf("valid value must not warn:\n%s", log.String())
	}
}
