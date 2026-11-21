package main

import (
	"log"

	"tsps-go/internal/persist"
)

// registerEngineAttributeKeys declares the persistent attributes the
// Go engine itself consumes, mirroring the api.persistAttribute calls
// the TS content plugins make at load time. In TS these keys persist
// only because MakeOverMage and Settings register them; the Go port
// has no port of either plugin yet, but the engine already reads the
// keys (Appearance pronouns/skin unlocks, the gameframe bootstrap's
// layout root and xp-drops flag), so without a registration the first
// snapshot would silently drop them from saves — PlayerSave.fromPlayer
// rebuilds attributes from the persistent set alone
// (PlayerSave.ts:811-816). Registering here, before the hub loads,
// keeps the deployed-TS behavior: the values round-trip, and a save
// without a key loads its default. When the MakeOverMage / Settings
// ports land they re-register the same keys with the same defaults,
// which is a no-op for the registry (re-registration overwrites the
// default without moving the key).
//
// NOT registered: "client-layout317" (Settings.plugin.js:112,1314,
// default false) — nothing in the Go engine reads it, so it stays
// unregistered until the Settings milestone ports the 317 gameframe
// branch. Every other Settings/MakeOverMage key (keybinds, settings
// values, migrated flags) is likewise unregistered until its content
// port declares it.
func registerEngineAttributeKeys(logger *log.Logger) {
	// Key, default, hasDefault. Defaults are verbatim from the TS
	// registrations; xp-drops:enabled is registered with no default
	// (Settings.plugin.js:1232).
	registrations := []struct {
		key        string
		def        any
		hasDefault bool
	}{
		// Appearance.PRONOUNS_ATTRIBUTE (Appearance.ts:17);
		// MakeOverMage.plugin.js:224.
		{"appearance:pronouns", -1, true},
		// Appearance.SKIN_UNLOCKS (Appearance.ts:27-29);
		// MakeOverMage.plugin.js:168.
		{"appearance:hw07-skin-colours", false, true},
		{"appearance:hw15-skin-colours", false, true},
		{"appearance:hw17-skin-colours", false, true},
		// CLIENT_LAYOUT_ATTRIBUTE (Settings.plugin.js:142) with
		// DEFAULT_GAMEFRAME_ROOT = 161 (:113); :1313.
		{"client-layout-root", 161, true},
		// XPDROPS_ENABLED_ATTRIBUTE (ClientProtocol.ts:2663);
		// Settings.plugin.js:1232 (no default).
		{"xp-drops:enabled", nil, false},
		// SHOP_QUANTITY_MODE_ATTRIBUTE (ShopManager.ts:91);
		// initialize() registers it with no default.
		{"shop:quantity-mode", nil, false},
	}
	for _, r := range registrations {
		// The keys and defaults are literals pinned by
		// TestEngineAttributeKeysRegistered; a failure here is a
		// programming error, not a runtime condition. Log and
		// continue rather than failing boot: the engine degrades
		// to the unregistered behavior (values dropped on
		// snapshot) instead of refusing to start.
		if err := persist.PersistAttribute(r.key, r.def, r.hasDefault); err != nil && logger != nil {
			logger.Printf("[attributes] failed to register engine attribute key %q: %v", r.key, err)
		}
	}
}
