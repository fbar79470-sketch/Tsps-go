package main

import (
	"tsps-go/internal/config"
	"tsps-go/internal/content/enchantjewellery"
)

// installEnchantJewellery installs the members-world flag on
// the registered Enchant Jewellery plugin (the cooking
// SetMembersWorld precedent). It runs before hub Load (see
// newPluginHub): Register captures the shared instance, so a
// call after Load is too late. Enchant Jewellery has no tables
// to resolve against the cache — the spell table is the
// inline ENCHANT_SPELLS transcription, and the spells
// themselves register with the game spell system through
// InstallStandardSpells — so there is nothing else to install.
// The plugin is F2P (Members: false); the flag only gates the
// members spells (Lvl-5, Lvl-6, Lvl-7), as TS does it.
func installEnchantJewellery(worldDef *config.World) {
	if worldDef != nil {
		enchantjewellery.SetMembersWorld(worldDef.MembersWorld)
	}
}
