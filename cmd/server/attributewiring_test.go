package main

import (
	"io"
	"log"
	"slices"
	"testing"

	"tsps-go/internal/config"
	"tsps-go/internal/persist"
)

// TestEngineAttributeKeysRegistered pins the boot-time registration of
// the attribute keys the engine itself consumes: the keys are present
// with the exact defaults the TS plugin registrations declare, and
// xp-drops:enabled has no default. The registry is process-global, so
// the test restores the booted state afterward.
func TestEngineAttributeKeysRegistered(t *testing.T) {
	persist.ResetAttributeRegistryForTest()
	defer func() {
		persist.ResetAttributeRegistryForTest()
		registerEngineAttributeKeys(log.Default())
	}()

	if _, err := newPluginHub(&config.World{}, log.New(io.Discard, "", 0), nil); err != nil {
		t.Fatalf("newPluginHub: %v", err)
	}

	keys := persist.PersistentAttributeKeys()
	wantDefaults := map[string]string{
		"appearance:pronouns":          "-1",
		"appearance:hw07-skin-colours": "false",
		"appearance:hw15-skin-colours": "false",
		"appearance:hw17-skin-colours": "false",
		"client-layout-root":           "161",
	}
	for key, wantJSON := range wantDefaults {
		if !slices.Contains(keys, key) {
			t.Errorf("key %q not registered", key)
			continue
		}
		got, ok := persist.AttributeDefaultJSON(key)
		if !ok || got != wantJSON {
			t.Errorf("default for %q = %q (present %v), want %q", key, got, ok, wantJSON)
		}
	}
	if !slices.Contains(keys, "xp-drops:enabled") {
		t.Error("key xp-drops:enabled not registered")
	}
	if _, ok := persist.AttributeDefaultJSON("xp-drops:enabled"); ok {
		t.Error("xp-drops:enabled has a default, want none (Settings.plugin.js:1232)")
	}
	if !slices.Contains(keys, "shop:quantity-mode") {
		t.Error("key shop:quantity-mode not registered")
	}
	if _, ok := persist.AttributeDefaultJSON("shop:quantity-mode"); ok {
		t.Error("shop:quantity-mode has a default, want none (ShopManager.ts:91)")
	}
	if slices.Contains(keys, "client-layout317") {
		t.Error("client-layout317 registered, want unregistered until the Settings milestone")
	}
}
