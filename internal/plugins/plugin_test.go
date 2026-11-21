package plugins

import (
	"testing"
)

func TestRegisterAndDefaultPlugins(t *testing.T) {
	// The registry is package-global and shared with every test in
	// the package, so assert relative growth, not absolute contents.
	before := DefaultPlugins()
	Register(Plugin{Name: "RegistryProbeA"})
	Register(Plugin{Name: "RegistryProbeB", ID: "probe-b"})
	after := DefaultPlugins()
	if len(after) != len(before)+2 {
		t.Fatalf("DefaultPlugins grew by %d, want 2", len(after)-len(before))
	}
	if after[len(after)-2].Name != "RegistryProbeA" || after[len(after)-1].Name != "RegistryProbeB" {
		t.Errorf("registry order wrong at tail: %v", after[len(after)-2:])
	}
	if after[len(after)-1].ID != "probe-b" {
		t.Errorf("registry must keep the value as registered: %+v", after[len(after)-1])
	}

	// DefaultPlugins returns a copy: mutating it must not affect
	// the registry, and a snapshot must not see later Registers.
	snapshot := DefaultPlugins()
	snapshot[0].Name = "MUTATED"
	Register(Plugin{Name: "RegistryProbeC"})
	fresh := DefaultPlugins()
	if fresh[0].Name == "MUTATED" {
		t.Error("mutating a DefaultPlugins snapshot affected the registry")
	}
	if len(snapshot) != len(after) {
		t.Errorf("snapshot grew after a later Register: %d -> %d", len(after), len(snapshot))
	}
	if fresh[len(fresh)-1].Name != "RegistryProbeC" {
		t.Errorf("fresh snapshot missing the new plugin: %v", fresh[len(fresh)-1])
	}
}
