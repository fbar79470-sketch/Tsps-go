package standardeffects

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Curse spell ids (the standardSpellDefs table,
// internal/game/combat_spells_data.go:82 CONFUSE, 103 WEAKEN,
// 124 CURSE, 250 VULNERABILITY, 264 ENFEEBLE, 313 STUN).
const (
	spellConfuseID       = 1153
	spellWeakenID        = 1157
	spellCurseID         = 1161
	spellVulnerabilityID = 1542
	spellEnfeebleID      = 1543
	spellStunID          = 1562
)

// curseDef is one curse: the skill it drains and the percent
// of the target's current level it takes
// (StandardCurses.plugin.js:3-6, CURSES).
type curseDef struct {
	skill   int
	percent int
}

// curseSpells maps curse spell id to its drain
// (StandardCurses.plugin.js:3-6).
var curseSpells = map[int]curseDef{
	spellConfuseID:       {game.SkillAttack, 5},
	spellWeakenID:        {game.SkillStrength, 5},
	spellCurseID:         {game.SkillDefence, 5},
	spellVulnerabilityID: {game.SkillDefence, 10},
	spellEnfeebleID:      {game.SkillStrength, 10},
	spellStunID:          {game.SkillAttack, 10},
}

// immuneMessage is what a curse on a drain-immune NPC says
// (StandardCurses.plugin.js:15-16, IMMUNE_MESSAGE). Recorded
// for Enfeeble (rsprox_transcripts/227/00095.txt:81131,
// 227/02174.txt:5460): the line on the click's tick, with no
// cast animation, runes or XP.
const immuneMessage = "Your target is currently immune to that spell."

// weakenedMessage is the line for a curse on a stat that is
// already lowered (StandardCurses.plugin.js:7-14,
// weakenedMessage). The attack curses' line is recorded
// (rsprox_transcripts/228/02224.txt ticks 1158 and 1275,
// Confuse or Stun on Dusk); no recording or Wiki page gives
// the strength or defence wording, so those keep the older
// line.
func weakenedMessage(skill int, npc bool) string {
	if skill == game.SkillAttack {
		return "Your foe's attack has already been weakened."
	}
	if npc {
		return "The spell has no effect because the NPC is already weakened."
	}
	return "The spell has no effect because the player is already weakened."
}

// curseDisabled is the onSpellDisabled gate
// (StandardCurses.plugin.js:35-50, disabled): a curse on an
// already-lowered stat is refused with the weakened line.
//
// The NPC branches cannot run in Go: drain immunity needs
// previewCurrentLevel and the already-lowered check needs an
// NPC skill manager, and Go has no mutable NPC skill state at
// all (npcCombatant.CurrentLevel reads the definition;
// internal/game/combatant.go: "Stat drains are NOT ported").
// An NPC target therefore passes the gate — the cast is paid,
// and the impact applies no drain (see curseImpact). A player
// target is gated fully.
func (s *standardEffects) curseDisabled(event *plugins.SpellDisabledEvent) {
	if event == nil || event.Player == nil {
		return
	}
	if event.Disabled != nil && *event.Disabled {
		return
	}
	def, ok := curseSpells[event.SpellID]
	if !ok {
		return
	}
	target := event.Player.GetCombat().Target()
	if target == nil {
		return
	}
	player := target.AsPlayer()
	if player == nil || player.Skills() == nil {
		// NPC target: no drain-immunity or already-lowered
		// seam exists — the cast is not refused here.
		return
	}
	if player.Skills().CurrentLevel(def.skill) >= player.Skills().MaxLevel(def.skill) {
		return
	}
	event.Player.SendMessage(weakenedMessage(def.skill, false))
	event.Disabled = plugins.BoolPtr(true)
}

// curseImpact is the "magic:utility-effect" handler
// (StandardCurses.plugin.js:52-80, applyCurse): a curse that
// lands drains the target's stat by its percent, floored.
// A target already below its base level gets the weakened
// line instead and no drain. A player target feels slightly
// weakened.
//
// The NPC drain is NOT ported (see curseDisabled): there is
// no way to read or set an NPC's current level in Go. The
// event is still marked handled for an NPC target, exactly as
// TS marks it before the drain — the spell has no fallback,
// so the observable difference is nil; the drain itself is
// the recorded residual.
//
// The "combat:stat-drain-modify" event TS emits before the
// drain (StandardCurses.plugin.js:71-73) is not emitted: no Go
// plugin subscribes to it, and the percent is a fixed
// per-spell constant.
func (s *standardEffects) curseImpact(payload any) {
	event, ok := payload.(*game.MagicUtilityEffectPayload)
	if !ok || event == nil || event.Spell == nil || event.Target == nil {
		return
	}
	if !event.Allowed || event.Handled {
		return
	}
	def, ok := curseSpells[event.Spell.SpellID()]
	if !ok {
		return
	}
	// Mobile carries no AsNPC: a target that is not a player
	// is an NPC (combat targets are only ever players or
	// NPCs), so the player branch below is the whole
	// distinction Go can draw.
	player := event.Target.AsPlayer()
	event.Handled = true
	if player == nil {
		// NPC target: no mutable skill state to drain.
		return
	}
	skills := player.Skills()
	if skills == nil {
		return
	}
	current := skills.CurrentLevel(def.skill)
	maximum := skills.MaxLevel(def.skill)
	if current < maximum {
		if attacker := event.Attacker.AsPlayer(); attacker != nil {
			attacker.SendMessage(weakenedMessage(def.skill, false))
		}
		return
	}
	decrease := current * def.percent / 100
	skills.SetCurrentLevelCombat(def.skill, current-decrease)
	skills.UpdateSkill(def.skill)
	player.SendMessage("You feel slightly weakened.")
}
