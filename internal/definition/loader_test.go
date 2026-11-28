package definition

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestRegistryOrdering(t *testing.T) {
	r := NewRegistry()
	load := func() (any, error) { return []any{}, nil }
	if err := r.Register("t", "owner-b", Source{Name: "zebra", Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("t", "owner-a", Source{Name: "alpha", Priority: IntPriority(50), Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("t", "owner-c", Source{Name: "mid", Priority: IntPriority(50), Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("t", "owner-d", Source{Load: load}); err != nil {
		t.Fatal(err)
	}
	// Priority first, then name; the unnamed source defaults its
	// name to its owner and its priority to 100.
	want := []string{"alpha", "mid", "owner-d", "zebra"}
	got := r.SourceNames("t")
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("SourceNames = %v, want %v", got, want)
	}
}

func TestRegistryDuplicatesAndValidation(t *testing.T) {
	r := NewRegistry()
	load := func() (any, error) { return []any{}, nil }
	if err := r.Register("t", "owner", Source{Name: "a", Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("t", "owner", Source{Name: "a", Load: load}); err == nil {
		t.Fatal("duplicate owner:name must fail")
	}
	// Same name, different owner: fine. Same pair, other type: fine.
	if err := r.Register("t", "other", Source{Name: "a", Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register("u", "owner", Source{Name: "a", Load: load}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(" ", "owner", Source{Load: load}); err == nil {
		t.Fatal("blank type must fail")
	}
	if err := r.Register("t", " ", Source{Load: load}); err == nil {
		t.Fatal("blank owner must fail")
	}
	if err := r.Register("t", "owner", Source{Name: "b"}); err == nil {
		t.Fatal("missing load must fail")
	}
	if err := r.RegisterFile("t", "owner", " "); err == nil {
		t.Fatal("blank file path must fail")
	}
}

func TestRegisterFileIsLazy(t *testing.T) {
	dir := t.TempDir()
	r := NewRegistry()
	path := dir + "/late.json"
	if err := r.RegisterFile("t", "owner", path); err != nil {
		t.Fatal(err)
	}
	// Written after registration: the read happens at load time.
	if err := os.WriteFile(path, []byte(`[{"id": 1}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	loaded, failures := r.loadSources("t", func(string, ...any) {})
	if failures != 0 || len(loaded) != 1 || len(loaded[0].Definitions) != 1 {
		t.Fatalf("loaded=%v failures=%d", loaded, failures)
	}
}

func TestLoadSourcesFailureCounting(t *testing.T) {
	r := NewRegistry()
	r.Register("t", "bad", Source{Name: "throws", Load: func() (any, error) { return nil, errors.New("boom") }})
	r.Register("t", "bad", Source{Name: "nonarray", Load: func() (any, error) { return map[string]any{}, nil }})
	r.Register("t", "good", Source{Name: "ok", Load: func() (any, error) { return []any{float64(1)}, nil }})
	var logs logCapture
	loaded, failures := r.loadSources("t", logs.logf)
	if failures != 2 {
		t.Fatalf("failures = %d, want 2", failures)
	}
	if len(loaded) != 1 || loaded[0].Name != "ok" {
		t.Fatalf("loaded = %v", loaded)
	}
	if !logs.contains("Failed to load") || !logs.contains("non-array") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestLoadMinimalNilCache(t *testing.T) {
	dir := fixtureDir(t, minimalFiles())
	var logs logCapture
	store, err := Load(dir, LoadOptions{Logf: logs.logf})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(store.NPCSpawns()); got != 0 {
		t.Fatalf("NPCSpawns = %d", got)
	}
	if got := len(store.ObjectSpawns()); got != 0 {
		t.Fatalf("ObjectSpawns = %d", got)
	}
	if got := len(store.GroundItemSpawns()); got != 0 {
		t.Fatalf("GroundItemSpawns = %d", got)
	}
	if got := len(store.Shops()); got != 0 {
		t.Fatalf("Shops = %d", got)
	}
	// Nil-cache wrappers degrade to DEFAULT shapes.
	def := store.NPC(0)
	if def.ID() != -1 || def.Name() != "null" {
		t.Fatalf("NPC(0) = id %d name %q", def.ID(), def.Name())
	}
	item := store.Item(0)
	if item.ID() != -1 || item.Name() != "" {
		t.Fatalf("Item(0) = id %d name %q", item.ID(), item.Name())
	}
	if _, ok := store.Object(0); ok {
		t.Fatal("Object(0) must be absent with no cache")
	}
	// Optional-file warnings were logged.
	if !logs.contains("monsters-complete.json missing") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestLoadNilLogf(t *testing.T) {
	dir := fixtureDir(t, minimalFiles())
	if _, err := Load(dir, LoadOptions{}); err != nil {
		t.Fatalf("Load with nil Logf: %v", err)
	}
}

func TestLoadMissingDirectory(t *testing.T) {
	_, err := Load(t.TempDir()+"/no-such-dir", LoadOptions{})
	if err == nil {
		t.Fatal("missing directory must fail")
	}
	if !strings.Contains(err.Error(), "interface-layouts.json") {
		t.Fatalf("error = %v, want interface-layouts.json", err)
	}
}

func TestLoadMissingRequiredFiles(t *testing.T) {
	for _, name := range []string{
		"interface-layouts.json",
		"object-spawns.json",
		"ground-items.json",
		"npc-combat-defs.json",
		"npc-animations.json",
		"npc-drops.json",
		"shops.json",
		"custom-shops.json",
	} {
		t.Run(name, func(t *testing.T) {
			dir := fixtureDir(t, withoutFiles(name))
			if _, err := Load(dir, LoadOptions{}); err == nil {
				t.Fatalf("missing %s must fail", name)
			}
		})
	}
}

func TestLoadMalformedRequiredFile(t *testing.T) {
	dir := fixtureDir(t, withFiles(map[string]string{"shops.json": `{not json`}))
	if _, err := Load(dir, LoadOptions{}); err == nil {
		t.Fatal("malformed shops.json must fail")
	}
}

func TestLoadMalformedOptionalFileFails(t *testing.T) {
	// readOptional parses when the file exists: malformed JSON is
	// fatal, exactly as the TS JSON.parse throw is.
	dir := fixtureDir(t, withFiles(map[string]string{"monsters-complete.json": `[1,2`}))
	if _, err := Load(dir, LoadOptions{}); err == nil {
		t.Fatal("malformed monsters-complete.json must fail")
	}
}

func TestLoadMalformedNpcSpawnsIsNotFatal(t *testing.T) {
	// npc-spawns.json loads through a source: a failure is
	// counted, and the boot continues with zero spawns.
	dir := fixtureDir(t, withFiles(map[string]string{"npc-spawns.json": `{oops`}))
	var logs logCapture
	store, err := Load(dir, LoadOptions{Logf: logs.logf})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := len(store.NPCSpawns()); got != 0 {
		t.Fatalf("NPCSpawns = %d", got)
	}
	if !logs.contains("Failed to load") {
		t.Fatalf("logs = %q", logs.joined())
	}
}

func TestLoadNonArrayCoreFilesFail(t *testing.T) {
	for _, tc := range []struct{ name, content string }{
		{"object-spawns.json", `{}`},
		{"ground-items.json", `{}`},
		{"npc-drops.json", `[]`},
		{"shops.json", `{}`},
		{"interface-layouts.json", `[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureDir(t, withFiles(map[string]string{tc.name: tc.content}))
			if _, err := Load(dir, LoadOptions{}); err == nil {
				t.Fatalf("%s with wrong shape must fail", tc.name)
			}
		})
	}
}

func TestLoadStatusSchemaVersion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{"version 1", `{"schema_version": 1, "npcs": {}}`, false},
		{"version 2", `{"schema_version": 2, "npcs": {}}`, true},
		{"string version", `{"schema_version": "1", "npcs": {}}`, true},
		{"missing version", `{"npcs": {}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixtureDir(t, withFiles(map[string]string{"npc-status-immunities.json": tc.content}))
			_, err := Load(dir, LoadOptions{})
			if tc.wantErr && err == nil {
				t.Fatal("must fail")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("Load: %v", err)
			}
			if tc.wantErr && !strings.Contains(err.Error(), "Unsupported NPC status immunity schema") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}
