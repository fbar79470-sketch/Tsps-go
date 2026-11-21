package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/persist"
)

// Spellbook and autocast login restore: the cmd/server adapter
// between the carried save (persist.PlayerSave) and the live
// Player state (internal/game). It follows the sailing/relations
// entry-load precedents in savewiring.go exactly: a small
// applyEntry* function called from prepareCarriedSave, in both
// the carried-save and new-character branches, before the
// player is queued, so no tick can read the state half-loaded.

// applyEntrySpellbook restores the carried save's spellbook onto
// the live player (PlayerSave.ts:641,
// player.setSpellbook(this.spellBook)): the book is session-only
// state on Player (default NORMAL), and the save holds its
// interface id. The bootstrap's tab-6 mount and varbit 4070
// still read the save directly (session/bootstrap.go); this sets
// the live state the spellbook and autocast systems read. A nil
// player or save is a no-op; an unknown interface id reads
// NORMAL (game.SpellbookForInterfaceID).
func applyEntrySpellbook(p *game.Player, carried *persist.PlayerSave) {
	if p == nil || carried == nil {
		return
	}
	p.SetSpellbook(game.SpellbookForInterfaceID(carried.SpellBook.InterfaceID))
}

// restoreEntryAutocast restores the carried save's autocast
// spell on login (Player.onLogin, Player.ts:795-800), through
// game.RestoreAutocastOnLogin: a spell whose book is not the
// player's clears it, a spell the wielded staff can't cast
// clears it, and a spell with no staff equipped is remembered
// as the default. It runs in AddPlayer after
// game.AssignWeaponInterface, mirroring the TS onLogin order
// (WeaponInterfaceManager.assign first, then the autocast
// restore). The carried save is the player's own save; a nil
// player or save is a no-op.
func restoreEntryAutocast(p *game.Player, carried *persist.PlayerSave) {
	if p == nil || carried == nil {
		return
	}
	game.RestoreAutocastOnLogin(p, carried.AutocastSpellId)
}

// InstallSpellListeners wires the magic listeners' spell seams
// (listeners.SpellHooks, see internal/listeners/spellhooks.go)
// for a player entering the world: the cache spell-name
// lookup, the spellbook read, the combat spell cast, the
// rune/disabled gates, and the tick-task scheduling for
// delayed spell effects. Called once per entering player in
// AddPlayer, next to InstallCombatListeners. Every seam is
// nil-safe: a nil definitions facade skips the cache-name
// fallback (ids resolve by id only), a nil world leaves the
// task seam unset (delayed spell effects cannot run), and the
// gate adapters read the game.Hooks spell fields at call time
// (populated by InstallCombatHooks in buildServer) so a hub
// installed after this call still takes effect.
//
// EmitSpellOnNpc is deliberately left unset: the Go hub has
// no onSpellOnNpc family (PluginManager.emitSpellOnNpc,
// PluginManager.ts:1967-1973, is NOT STARTED), so scripted
// casts proceed with no hook claiming them — the no-hooks
// default. The listener casts are standard-book casts, so the
// gate adapters pass SpellbookNormal; alchemy/telegrab carry
// no CombatSpell, so SpellDisabled receives nil for the spell.
func InstallSpellListeners(deps *listeners.Deps, defs *cache.CacheDefinitions, world *game.World) {
	if deps == nil {
		return
	}
	if defs != nil {
		deps.SpellHooks.SpellName = func(widgetID, itemID int) (string, bool) {
			return defs.GetSpellName(widgetID, itemID)
		}
	}
	deps.SpellHooks.SpellbookInterfaceID = func(p *game.Player) int {
		if p == nil {
			return game.SpellbookNormal.InterfaceID()
		}
		return p.Spellbook().InterfaceID()
	}
	deps.SpellHooks.CastSpell = func(attacker *game.Player, target *game.Mobile, spell game.CombatSpell) {
		if attacker == nil || target == nil || spell == nil {
			return
		}
		attacker.GetCombat().CastSpellOn(target, spell)
	}
	deps.SpellHooks.SpellRuneBypass = func(p *game.Player, spellID, itemID int) bool {
		if fn := game.Hooks.SpellRuneBypass; fn != nil {
			return fn(p, game.SpellbookNormal, spellID, itemID)
		}
		return false
	}
	deps.SpellHooks.SpellDisabled = func(p *game.Player, spellID int, spell any) bool {
		fn := game.Hooks.SpellDisabled
		if fn == nil {
			return false
		}
		var cs game.CombatSpell
		if s, ok := spell.(game.CombatSpell); ok {
			cs = s
		}
		return fn(p, game.SpellbookNormal, spellID, cs)
	}
	deps.SpellHooks.CheckSpellRuneSource = func(p *game.Player, missing []game.Item) bool {
		if fn := game.Hooks.CheckSpellRuneSource; fn != nil {
			return fn(p, missing)
		}
		return false
	}
	deps.SpellHooks.ConsumeSpellRuneSource = func(p *game.Player, missing []game.Item) {
		if fn := game.Hooks.ConsumeSpellRuneSource; fn != nil {
			fn(p, missing)
		}
	}
	if world != nil {
		deps.SpellHooks.ScheduleTask = func(task *game.Task) {
			if task != nil {
				world.Tasks().Schedule(task)
			}
		}
	}
}
