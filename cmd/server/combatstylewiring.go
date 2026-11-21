package main

import (
	"os"
	"path/filepath"

	"tsps-go/internal/game"
	"tsps-go/internal/persist"
)

// Combat-style wiring: boot-time loading of the combat style
// registry (data/definitions/item-combat-styles.json) and its
// installation on the game package, following the
// loadMusicData precedent (musicwiring.go) exactly: the file
// is read from the definitions directory dir, verified
// (SHA-256) and parsed by game.LoadCombatStyles, and the
// registry is installed once, before ticking starts.
//
// A missing, unreadable, tampered or unparsable file never
// fails the boot: the loader logs and installs the
// UNARMED-only fallback registry (game.InstallCombatStyles
// with nil), so every weapon reads as unarmed — the same
// degraded behavior the server had before combat styles
// existed, rather than refusing to start. The file ships in
// the repo (verbatim upstream import, SHA-pinned; see
// internal/game/combat_styles.go), so a fallback install in
// production means the definitions directory is damaged and
// the log line is the signal.
//
// Hookup (coordinator, server.go buildServer, next to the
// loadMusicData call):
//
//	InstallCombatStyles(filepath.Join("data", "definitions"), logger.Printf)
func InstallCombatStyles(dir string, logf func(format string, args ...any)) {
	raw, err := os.ReadFile(filepath.Join(dir, game.CombatStylesFile))
	if err != nil {
		if logf != nil {
			logf("[combat] failed to load %s: %v (combat styles fall back to unarmed)", game.CombatStylesFile, err)
		}
		game.InstallCombatStyles(nil)
		return
	}
	reg, err := game.LoadCombatStyles(raw)
	if err != nil {
		if logf != nil {
			logf("[combat] failed to load %s: %v (combat styles fall back to unarmed)", game.CombatStylesFile, err)
		}
		game.InstallCombatStyles(nil)
		return
	}
	game.InstallCombatStyles(reg)
	if logf != nil {
		logf("[combat] styles loaded: %d fight types, %d weapon interfaces", reg.FightTypeCount(), reg.WeaponInterfaceCount())
	}
}

// applyEntryFightType restores the carried save's fightType
// onto the live player at login (PlayerSave.applyToPlayer,
// PlayerSave.ts:642), through game.RestoreSavedFightType:
// the saved value resolved against the equipped weapon's
// offered styles, set when it resolves, left alone
// otherwise. It follows the applyEntrySpellbook precedent in
// spellwiring.go exactly: a small applyEntry* function called
// from prepareCarriedSave, in both the carried-save and
// new-character branches, before the player is queued, so no
// tick can read the state half-loaded. It runs after the
// equipment restore (applyEntryLiveState) because the
// resolution needs the equipped weapon, and before the
// login AssignWeaponInterface in AddPlayer, so the assign's
// "current fight type when offered" branch can honor the
// saved value when no per-category style was ever
// remembered. The carried save is the player's own save; a
// nil player or save is a no-op.
func applyEntryFightType(p *game.Player, carried *persist.PlayerSave) {
	if p == nil || carried == nil {
		return
	}
	game.RestoreSavedFightType(p, carried.FightType)
}
