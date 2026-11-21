package persist

import (
	"encoding/json"
	"math"
	"reflect"
	"sort"
	"testing"
	"time"
)

// resetRegistry restores the core seed before and after the test, so a
// test that registers keys can neither see nor leave pollution through
// the process-global registry.
func resetRegistry(t *testing.T) {
	t.Helper()
	ResetAttributeRegistryForTest()
	t.Cleanup(ResetAttributeRegistryForTest)
}

func TestCoreAttributeRegistrySeed(t *testing.T) {
	resetRegistry(t)
	// The seed is exactly the 22 legacy-field values plus the three
	// initial-set members plus the fromPlayer-registered combat key.
	keys := PersistentAttributeKeys()
	if len(keys) != 26 {
		t.Fatalf("seed has %d keys, want 26: %v", len(keys), keys)
	}
	want := map[string]bool{}
	for _, key := range legacyFieldAttributes {
		want[key] = true
	}
	want["npc-aggression:tolerance"] = true
	want["creation-menu:last-item"] = true
	want[LoginTokensAttribute] = true
	want["combat:style-by-category"] = true
	for _, key := range keys {
		if !want[key] {
			t.Fatalf("unexpected seed key %q", key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing seed keys: %v", want)
	}
	// The seed defaults are byte-identical to the pre-registry static
	// map: same keys, same default JSON strings.
	wantDefaults := map[string]string{
		"ring-of-recoil:damage":    "0",
		"login:tokens":             "[]",
		"bank:note-withdrawal":     "false",
		"bank:insert-mode":         "false",
		"bank:quantity-mode":       "0",
		"bank:custom-quantity":     "0",
		"bank:placeholders":        "false",
		"combat:style-by-category": "{}",
	}
	for key, wantJSON := range wantDefaults {
		got, ok := AttributeDefaultJSON(key)
		if !ok || got != wantJSON {
			t.Fatalf("AttributeDefaultJSON(%q) = (%q, %v), want (%q, true)", key, got, ok, wantJSON)
		}
	}
	if _, ok := AttributeDefaultJSON("shop:points"); ok {
		t.Fatal("shop:points must have no default")
	}
	// Registration order: the legacy values lead in TS declaration
	// order, then the three initial members, then the fromPlayer key.
	// ApplyAttributeDefaults exposes the default order: the two
	// attributeDefaults entries first, then the fromPlayer
	// registrations in call order.
	var applied []string
	ApplyAttributeDefaults(func(string) bool { return false }, func(key string, _ any) { applied = append(applied, key) })
	wantOrder := []string{
		"ring-of-recoil:damage", "login:tokens",
		"bank:note-withdrawal", "bank:insert-mode", "bank:quantity-mode",
		"bank:custom-quantity", "bank:placeholders", "combat:style-by-category",
	}
	if !reflect.DeepEqual(applied, wantOrder) {
		t.Fatalf("default application order = %v, want %v", applied, wantOrder)
	}
}

func TestPersistAttributeKeyValidation(t *testing.T) {
	resetRegistry(t)
	invalid := []string{
		"",
		" ",
		"\t\n",
		" padded",
		"padded ",
		"  padded  ",
		"\tkey",
		"key\n",
		"\u00a0key", // NBSP: JS trim removes it, so TS rejects it.
		"key\ufeff", // U+FEFF: JS trim removes it, so TS rejects it.
	}
	for _, key := range invalid {
		if err := PersistAttribute(key, nil, false); err == nil {
			t.Errorf("PersistAttribute(%q) = nil, want error", key)
		}
		if isPersistentAttributeKey(key) {
			t.Errorf("invalid key %q was registered", key)
		}
	}
	valid := []string{
		"key",
		"a:b",
		"a b",      // interior whitespace is allowed, as in TS.
		"a\tb",     // interior tab is allowed.
		"a\u00a0b", // interior NBSP is allowed.
		"ns:key-1",
	}
	for _, key := range valid {
		if err := PersistAttribute(key, nil, false); err != nil {
			t.Errorf("PersistAttribute(%q) = %v, want nil", key, err)
		}
		if !isPersistentAttributeKey(key) {
			t.Errorf("valid key %q was not registered", key)
		}
	}
}

func TestPersistAttributeDefaults(t *testing.T) {
	resetRegistry(t)
	// Registration stores canonical JSON: map keys sort.
	if err := PersistAttribute("test:obj", map[string]any{"b": 1, "a": 2}, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if got, ok := AttributeDefaultJSON("test:obj"); !ok || got != `{"a":2,"b":1}` {
		t.Fatalf("AttributeDefaultJSON(test:obj) = (%q, %v), want (`{\"a\":2,\"b\":1}`, true)", got, ok)
	}
	if err := PersistAttribute("test:int", 42, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if got, _ := AttributeDefaultJSON("test:int"); got != "42" {
		t.Fatalf("int default = %q, want 42", got)
	}
	if err := PersistAttribute("test:str", "hello", true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if got, _ := AttributeDefaultJSON("test:str"); got != `"hello"` {
		t.Fatalf("string default = %q, want quoted hello", got)
	}
	// Re-registration overwrites the default (TS Map.set overwrites).
	if err := PersistAttribute("test:int", 7, true); err != nil {
		t.Fatalf("PersistAttribute overwrite: %v", err)
	}
	if got, _ := AttributeDefaultJSON("test:int"); got != "7" {
		t.Fatalf("overwritten default = %q, want 7", got)
	}
	// Re-registration without a default leaves the default alone (TS:
	// defaultValue undefined means "no set").
	if err := PersistAttribute("test:int", nil, false); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if got, _ := AttributeDefaultJSON("test:int"); got != "7" {
		t.Fatalf("default after no-default re-register = %q, want 7", got)
	}
	// A key registered without a default has none.
	if err := PersistAttribute("test:nodefault", nil, false); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if _, ok := AttributeDefaultJSON("test:nodefault"); ok {
		t.Fatal("key registered without a default must have none")
	}
	// An explicit nil default IS a default: TS persistAttribute(key,
	// null) stores "null" (only undefined means "no default").
	if err := PersistAttribute("test:null", nil, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if got, ok := AttributeDefaultJSON("test:null"); !ok || got != "null" {
		t.Fatalf("null default = (%q, %v), want (\"null\", true)", got, ok)
	}
}

func TestPersistAttributeBadDefaultRegistersKey(t *testing.T) {
	resetRegistry(t)
	// TS adds the key before cloning the default, so a throwing
	// default leaves the key registered with no default stored.
	err := PersistAttribute("test:baddefault", func() {}, true)
	if err == nil {
		t.Fatal("func default must fail validation")
	}
	if !isPersistentAttributeKey("test:baddefault") {
		t.Fatal("key must stay registered after a failing default (TS order)")
	}
	if _, ok := AttributeDefaultJSON("test:baddefault"); ok {
		t.Fatal("failing default must not be stored")
	}
}

func TestPersistentAttributeKeysSortedCopy(t *testing.T) {
	resetRegistry(t)
	if err := PersistAttribute("zz:last", nil, false); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if err := PersistAttribute("aa:first", nil, false); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	keys := PersistentAttributeKeys()
	if !sort.StringsAreSorted(keys) {
		t.Fatalf("keys not sorted: %v", keys)
	}
	if keys[0] != "aa:first" || keys[len(keys)-1] != "zz:last" {
		t.Fatalf("sorted keys = %v", keys)
	}
	// Mutating the result must not affect the registry.
	keys[0] = "mutated"
	if got := PersistentAttributeKeys(); got[0] == "mutated" {
		t.Fatal("PersistentAttributeKeys must return a copy")
	}
}

func TestResetAttributeRegistryForTest(t *testing.T) {
	resetRegistry(t)
	if err := PersistAttribute("test:transient", 1, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if err := PersistAttribute("shop:points", 99, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	ResetAttributeRegistryForTest()
	if isPersistentAttributeKey("test:transient") {
		t.Fatal("reset must drop runtime registrations")
	}
	if _, ok := AttributeDefaultJSON("shop:points"); ok {
		t.Fatal("reset must restore the core defaults (shop:points has none)")
	}
	if !isPersistentAttributeKey("login:tokens") {
		t.Fatal("reset must restore the core seed")
	}
	if got, _ := AttributeDefaultJSON("ring-of-recoil:damage"); got != "0" {
		t.Fatalf("reset must restore core default JSON, got %q", got)
	}
}

func TestApplyAttributeDefaults(t *testing.T) {
	resetRegistry(t)
	if err := PersistAttribute("test:first", 1, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	if err := PersistAttribute("test:second", 2, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	// Runtime registrations apply after the core seed, in
	// registration order; present keys are skipped.
	present := map[string]bool{"ring-of-recoil:damage": true}
	var applied []string
	ApplyAttributeDefaults(
		func(key string) bool { return present[key] },
		func(key string, _ any) { applied = append(applied, key) },
	)
	for _, key := range applied {
		if key == "ring-of-recoil:damage" {
			t.Fatal("present key must be skipped")
		}
	}
	last := applied[len(applied)-2:]
	if !reflect.DeepEqual(last, []string{"test:first", "test:second"}) {
		t.Fatalf("runtime defaults applied as %v, want [test:first test:second] last", applied)
	}
	// Defaults decode fresh on every call: mutating one application
	// must not affect the next.
	if err := PersistAttribute("test:obj", map[string]any{"n": []any{1.0}}, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	collect := func() map[string]any {
		got := map[string]any{}
		ApplyAttributeDefaults(func(string) bool { return false }, func(key string, value any) { got[key] = value })
		return got
	}
	first := collect()["test:obj"].(map[string]any)
	first["n"] = []any{99.0}
	first["extra"] = true
	second := collect()["test:obj"].(map[string]any)
	if !reflect.DeepEqual(second, map[string]any{"n": []any{1.0}}) {
		t.Fatalf("second application aliased the first: %v", second)
	}
	// A default that decodes to a number arrives as float64 (JSON.parse
	// semantics), matching values decoded from a save.
	got := collect()
	if got["test:first"] != float64(1) {
		t.Fatalf("int default decoded as %T %v, want float64 1", got["test:first"], got["test:first"])
	}
}

func TestMarshalAttributeValueVectors(t *testing.T) {
	accept := []struct {
		name  string
		value any
		want  string
	}{
		{"nil", nil, "null"},
		{"bool", true, "true"},
		{"string", "x", `"x"`},
		{"int", -7, "-7"},
		{"int8", int8(-8), "-8"},
		{"int64", int64(1 << 60), "1152921504606846976"},
		{"uint", uint(9), "9"},
		{"uint64", uint64(1 << 63), "9223372036854775808"},
		{"float64", 1.5, "1.5"},
		{"float32", float32(2.5), "2.5"},
		{"empty slice", []any{}, "[]"},
		{"array", [2]int{1, 2}, "[1,2]"},
		{"byte array", [3]byte{1, 2, 3}, "[1,2,3]"},
		{"nested", map[string]any{"a": []any{1.0, "s", nil, true, map[string]any{}}}, `{"a":[1,"s",null,true,{}]}`},
		{"empty map", map[string]any{}, "{}"},
		{"nil slice", []any(nil), "null"},
		{"nil map", map[string]any(nil), "null"},
		{"named string", namedString("v"), `"v"`},
	}
	for _, tc := range accept {
		raw, err := MarshalAttributeValue("test:key", tc.value)
		if err != nil {
			t.Errorf("%s: MarshalAttributeValue = error %v, want %s", tc.name, err, tc.want)
			continue
		}
		if string(raw) != tc.want {
			t.Errorf("%s: MarshalAttributeValue = %s, want %s", tc.name, raw, tc.want)
		}
	}

	cyclicMap := map[string]any{}
	cyclicMap["self"] = cyclicMap
	cyclicSlice := []any{nil}
	cyclicSlice[0] = cyclicSlice
	shared := map[string]any{"n": 1.0}
	reject := []struct {
		name  string
		value any
	}{
		{"NaN", math.NaN()},
		{"+Inf", math.Inf(1)},
		{"-Inf", math.Inf(-1)},
		{"float32 NaN", float32(math.NaN())},
		{"nested NaN", map[string]any{"n": math.NaN()}},
		{"func", func() {}},
		{"nested func", []any{func() {}}},
		{"chan", make(chan int)},
		{"complex", complex(1, 2)},
		{"struct", struct{ A int }{A: 1}},
		{"time", time.Now()},
		{"pointer", new(int)},
		{"nil pointer", (*int)(nil)},
		{"int map", map[int]string{1: "a"}},
		{"byte slice", []byte{1, 2, 3}},
		{"cyclic map", cyclicMap},
		{"cyclic slice", cyclicSlice},
		{"uintptr", uintptr(1)},
	}
	for _, tc := range reject {
		if raw, err := MarshalAttributeValue("test:key", tc.value); err == nil {
			t.Errorf("%s: MarshalAttributeValue = %s, want error", tc.name, raw)
		}
	}
	// A shared (non-cyclic) reference is fine: it marshals twice, as
	// JSON.stringify does.
	if raw, err := MarshalAttributeValue("test:key", []any{shared, shared}); err != nil || string(raw) != `[{"n":1},{"n":1}]` {
		t.Fatalf("shared reference = (%s, %v), want ([{\"n\":1},{\"n\":1}], nil)", raw, err)
	}
}

type namedString string

func TestMigrateForEntryLiveRegistry(t *testing.T) {
	resetRegistry(t)
	// A plugin-registered key's camelCase form migrates, as in TS
	// once the plugin has registered the kebab form.
	if err := PersistAttribute("my-plugin:some-key", nil, false); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	save := &PlayerSave{
		Attributes: map[string]json.RawMessage{
			"myPlugin:someKey": json.RawMessage(`3`),
			"otherPlugin:key":  json.RawMessage(`4`),
		},
	}
	save.MigrateForEntry()
	if string(save.Attributes["my-plugin:some-key"]) != `3` {
		t.Fatalf("registered camelCase key did not migrate: %v", save.Attributes)
	}
	if _, ok := save.Attributes["myPlugin:someKey"]; ok {
		t.Fatal("camelCase form must be renamed away")
	}
	if string(save.Attributes["otherPlugin:key"]) != `4` {
		t.Fatal("unregistered key must be untouched")
	}
}

func TestOwnedStateAttributesCopy(t *testing.T) {
	base := applyTestSave()
	state := base.OwnedState()
	state.Attributes["shop:points"] = json.RawMessage(`99`)
	state.Attributes["new-key"] = json.RawMessage(`1`)
	if string(base.Attributes["shop:points"]) != `5` {
		t.Fatal("OwnedState attributes must be a copy")
	}
	if _, ok := base.Attributes["new-key"]; ok {
		t.Fatal("OwnedState attributes must be a copy")
	}
}

func TestWithOwnedAttributesOverwriteDropsUnknownKeys(t *testing.T) {
	resetRegistry(t)
	base := applyTestSave()
	// The carried save holds a persistent key, a default-valued key,
	// and an unknown key no live state knows about.
	base.Attributes["unknown:stale"] = json.RawMessage(`"old"`)
	state := OwnedState{
		Position:   *base.Position,
		Running:    base.Running,
		Attributes: map[string]json.RawMessage{"shop:points": json.RawMessage(`42`)},
	}
	clone := base.WithOwned(state)
	if string(clone.Attributes["shop:points"]) != `42` {
		t.Fatalf("owned attributes not written: %v", clone.Attributes)
	}
	if _, ok := clone.Attributes["unknown:stale"]; ok {
		t.Fatal("unknown carried key must be DROPPED by an owned overwrite (fromPlayer rebuild)")
	}
	if _, ok := clone.Attributes["login:tokens"]; ok {
		t.Fatal("carried key absent from live state must be dropped")
	}
	if len(clone.Attributes) != 1 {
		t.Fatalf("clone attributes = %v, want only shop:points", clone.Attributes)
	}
	// The receiver is untouched and the clone does not alias the
	// supplied map.
	if _, ok := base.Attributes["unknown:stale"]; !ok {
		t.Fatal("receiver mutated by WithOwned")
	}
	state.Attributes["shop:points"] = json.RawMessage(`1`)
	if string(clone.Attributes["shop:points"]) != `42` {
		t.Fatal("clone aliases the supplied attribute map")
	}
}

func TestWithOwnedAttributesNilPreserves(t *testing.T) {
	// Nil Attributes keeps the pre-phase-11 behavior: carried
	// attributes preserved, default-equal values stripped.
	base := applyTestSave()
	base.Attributes["unknown:stale"] = json.RawMessage(`"old"`)
	clone := base.WithOwned(OwnedState{Position: *base.Position, Running: base.Running})
	if string(clone.Attributes["shop:points"]) != `5` {
		t.Fatalf("nil Attributes must preserve carried values: %v", clone.Attributes)
	}
	if string(clone.Attributes["unknown:stale"]) != `"old"` {
		t.Fatal("nil Attributes must preserve unknown keys")
	}
	if _, ok := clone.Attributes["login:tokens"]; ok {
		t.Fatal("nil Attributes must still strip default-valued keys")
	}
}

func TestWithOwnedAttributesEmptyMapMeansNone(t *testing.T) {
	// A non-nil EMPTY map overwrites with {}: the player holds no
	// persistent attributes. It must stay non-nil so the save
	// marshals "attributes":{} as TS fromPlayer always writes an
	// attributes object.
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{
		Position:   *base.Position,
		Running:    base.Running,
		Attributes: map[string]json.RawMessage{},
	})
	if clone.Attributes == nil || len(clone.Attributes) != 0 {
		t.Fatalf("empty owned attributes = %v, want empty non-nil map", clone.Attributes)
	}
}

func TestWithOwnedAttributesStripsLiveDefaults(t *testing.T) {
	resetRegistry(t)
	// Default-stripping consults the LIVE registry: a
	// plugin-registered default strips a default-equal supplied value,
	// and a non-default value survives.
	if err := PersistAttribute("test:mode", 0, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	base := applyTestSave()
	clone := base.WithOwned(OwnedState{
		Position: *base.Position,
		Running:  base.Running,
		Attributes: map[string]json.RawMessage{
			"test:mode":   json.RawMessage(`0`),
			"shop:points": json.RawMessage(`5`),
		},
	})
	if _, ok := clone.Attributes["test:mode"]; ok {
		t.Fatal("default-equal value under a live-registered default must be stripped")
	}
	if string(clone.Attributes["shop:points"]) != `5` {
		t.Fatal("non-default value lost")
	}
	clone2 := base.WithOwned(OwnedState{
		Position:   *base.Position,
		Running:    base.Running,
		Attributes: map[string]json.RawMessage{"test:mode": json.RawMessage(`3`)},
	})
	if string(clone2.Attributes["test:mode"]) != `3` {
		t.Fatal("non-default value under a live-registered default must survive")
	}
}

func TestStripDefaultAttributesLiveRegistry(t *testing.T) {
	resetRegistry(t)
	if err := PersistAttribute("test:flag", false, true); err != nil {
		t.Fatalf("PersistAttribute: %v", err)
	}
	attrs := map[string]json.RawMessage{
		"test:flag":   json.RawMessage(`false`),
		"shop:points": json.RawMessage(`5`),
	}
	out := StripDefaultAttributes(attrs)
	if _, ok := out["test:flag"]; ok {
		t.Fatal("live-registered default must strip")
	}
	if string(out["shop:points"]) != `5` {
		t.Fatal("unregistered-default key lost")
	}
}
