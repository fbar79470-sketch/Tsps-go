// Package standardeffects is the Go port of the five
// server/plugins/combat + plugins/items plugins that complete
// the standard spellbook's binds, curses and freezes
// (Phase 45, worker P2):
//
//   - StandardBindings (plugins/combat/StandardBindings.plugin.js):
//     Bind, Snare and Entangle hold the target, with the
//     Swampbark armour hold bonus.
//   - StandardCurses (plugins/combat/StandardCurses.plugin.js):
//     Confuse, Weaken, Curse, Vulnerability, Enfeeble and Stun
//     drain the target's Attack, Strength or Defence.
//   - Freezes (plugins/combat/Freezes.plugin.js): the aftermath
//     of a landed freeze — the red message for ice spells, the
//     expiry warning, and releasing a player-applied freeze when
//     the two stray over 10 tiles apart.
//   - TeleBlock (plugins/combat/TeleBlock.plugin.js): the
//     per-tick Tele Block driver — warn 30 seconds before expiry
//     and remove the block outside the Wilderness.
//   - ElementalStaves (plugins/items/ElementalStaves.plugin.js):
//     elemental staves, battlestaves, the Kodai wand, the
//     twinflame staff and the Shadowflame quadrant supply their
//     element's runes to spells automatically.
//
// The emit side already existed: combatEffectSpell.spellEffect
// (internal/game/combat_spells.go) emits
// "magic:utility-effect-check" then "magic:utility-effect"
// with *game.MagicUtilityEffectPayload, and the binds and
// curses have no spell-owned fallback — their application is
// this package. Tele Block's own effect lives in the engine
// (teleblockSpellEffect); this package only drives its timer.
//
// Event inventory:
//
//   - Listens to onSpellDisabled (both the bind and the curse
//     gates), "magic:cast-committed" (the Swampbark hold
//     bonus), "magic:utility-effect" (bind + curse impact),
//     "combat:frozen" (the freeze aftermath — emitted by the
//     bind consumer here and, for ice freezes, by the Ancient
//     consumer, because Go's FreezeTicks does not emit it),
//     "equipment:effects" (the Swampbark set text — no producer
//     in Go, so the handler never runs), onPlayerProcess (the
//     TeleBlock tick driver) and onSpellRuneBypass (the staff
//     rune supply).
//   - Emits "combat:frozen" (game.CombatFrozenEventName) when
//     a bind freeze lands, and answers the
//     "elemental-staves:runes" query (no querier in Go yet).
//
// Faithfulness notes, documented not hidden:
//
//   - NPC curse drains are NOT ported: Go has no mutable NPC
//     skill state (npcCombatant.CurrentLevel reads the
//     definition; internal/game/combatant.go: "Stat drains are
//     NOT ported"), so there is no way to read an NPC's current
//     level, test drain immunity (previewCurrentLevel), or set
//     a drained level. A curse cast on an NPC spends its runes
//     and grants its XP but applies no drain. Player targets
//     drain fully.
//   - The "combat:stat-drain-modify" event TS emits before a
//     drain (StandardCurses.plugin.js) is not emitted: no Go
//     plugin subscribes to it, and the drain percent is a fixed
//     per-spell constant.
//   - The Devil's element offhand supply is NOT ported: TS
//     gates it on Demonic Pacts (a Leagues system with no Go
//     seam), so a Devil's element in the shield slot supplies
//     nothing here.
//   - The freeze messages go out through SendMessage, not the
//     typed SPAM channel TS uses: the spam chat-type constant
//     is unexported in internal/game, and no content package
//     sends typed messages.
//   - XP is granted by the spell itself through the engine's
//     normal path; this package grants no XP and changes no XP
//     math (the known integer-truncation gap is untouched).
//   - There is no SkillLevelUp emitter in Go; the drain's
//     "You feel slightly weakened." line is sent, and nothing
//     pauses or notifies on level changes.
package standardeffects

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// standardEffects is the shared state behind the five plugin
// values: the Api (for the custom-event emit and the lazy
// TaskManager / Wilderness resolution) plus the test overrides
// (the woodcutting taskManager / resolveWilderness pattern:
// the live Api value wins when present, a directly-set field
// is the unit-test override).
type standardEffects struct {
	api        *plugins.Api
	tasks      *game.TaskManager
	wilderness *game.Wilderness
}

// shared is the one standardEffects behind all five plugins.
var shared = &standardEffects{}

// taskManager resolves the world's TaskManager lazily: the
// hub-contract lesson (PHASE45_PLAN.md §3) — surfaces
// installed after plugin Load must be resolved lazily, never
// captured in Register. The directly-set tasks field is the
// unit-test override.
func (s *standardEffects) taskManager() *game.TaskManager {
	if s == nil {
		return nil
	}
	if s.api != nil {
		if tm := s.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return s.tasks
}

// resolveWilderness returns the world's Wilderness for the
// TeleBlock PvP check, or nil (the woodcutting
// resolveWilderness pattern: the live Api value wins when
// present; the directly-set wilderness field is the unit-test
// override). The Api's AreaManager is *game.AreaManager in
// production (installed via Hub.SetAreaManager); a
// differently-typed manager reads as no Wilderness.
func (s *standardEffects) resolveWilderness() *game.Wilderness {
	if s == nil {
		return nil
	}
	if s.wilderness != nil {
		return s.wilderness
	}
	if s.api != nil {
		if am, ok := s.api.GetAreaManager().(*game.AreaManager); ok && am != nil {
			return am.Wilderness()
		}
	}
	return nil
}

// Plugin is the registered Standard Bindings plugin value:
// name, description, and members flag verbatim
// (StandardBindings.plugin.js, module.exports), attach-only
// register.
var BindingsPlugin = plugins.Plugin{
	Name:        "Standard Bindings",
	Description: "Standard-spellbook Bind, Snare and Entangle rules, including Swampbark armour.",
	Members:     false,
	Register: func(api *plugins.Api) {
		shared.api = api
		api.OnSpellDisabled(shared.bindDisabled)
		api.OnCustomEvent("magic:cast-committed", shared.bindCommitted)
		api.OnCustomEvent("magic:utility-effect", shared.bindImpact)
		api.OnCustomEvent("equipment:effects", shared.addSwampbarkSetText)
	},
}

// Plugin is the registered Standard Curses plugin value: name,
// description, and members flag verbatim
// (StandardCurses.plugin.js, module.exports), attach-only
// register.
var CursesPlugin = plugins.Plugin{
	Name:        "Standard Curses",
	Description: "Standard-spellbook Confuse, Weaken and Curse, Vulnerability, Enfeeble and Stun stat drains.",
	Members:     false,
	Register: func(api *plugins.Api) {
		shared.api = api
		api.OnSpellDisabled(shared.curseDisabled)
		api.OnCustomEvent("magic:utility-effect", shared.curseImpact)
	},
}

// Plugin is the registered Freezes plugin value: name,
// description, and members flag verbatim
// (Freezes.plugin.js, module.exports), attach-only register.
var FreezesPlugin = plugins.Plugin{
	Name:        "Freezes",
	Description: "Freeze aftermath: red messages for ice spells, expiry warnings, and releasing a player-applied freeze when the two stray over 10 tiles apart.",
	Members:     false,
	Register: func(api *plugins.Api) {
		shared.api = api
		api.OnCustomEvent(game.CombatFrozenEventName, shared.freezeApplied)
	},
}

// Plugin is the registered TeleBlock plugin value: name,
// description, and members flag verbatim
// (TeleBlock.plugin.js, module.exports), attach-only register.
var TeleBlockPlugin = plugins.Plugin{
	Name:        "TeleBlock",
	Description: "Tele Block follow-up: warns 30 seconds before it expires and keeps it on in the Ferox Enclave.",
	Members:     false,
	Register: func(api *plugins.Api) {
		shared.api = api
		api.OnPlayerProcess(shared.teleblockTick)
	},
}

// Plugin is the registered ElementalStaves plugin value: name,
// description, and members flag verbatim
// (ElementalStaves.plugin.js, module.exports), attach-only
// register.
var StavesPlugin = plugins.Plugin{
	Name:        "ElementalStaves",
	Description: "Elemental staves, battlestaves, the Kodai wand, the twinflame staff, the Shadowflame quadrant and the Devil's element supply their element's runes to spells automatically.",
	Members:     false,
	Register: func(api *plugins.Api) {
		shared.api = api
		api.OnSpellRuneBypass(shared.staveRuneBypass)
		api.OnCustomEvent("elemental-staves:runes", shared.reportSuppliedRunes)
	},
}

func init() {
	plugins.Register(BindingsPlugin)
	plugins.Register(CursesPlugin)
	plugins.Register(FreezesPlugin)
	plugins.Register(TeleBlockPlugin)
	plugins.Register(StavesPlugin)
}
