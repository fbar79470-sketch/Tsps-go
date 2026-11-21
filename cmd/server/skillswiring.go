package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Skills wiring (phase 20): the cmd/server adapter between the
// game-side skills seam (internal/game skill_hooks.go), the plugin
// hub's skills families (internal/plugins skills.go), and the
// session's exp-drop sender. It follows the combatwiring.go
// precedent exactly: narrow func fields on the game seam,
// translation adapters here, where all sides are importable. Game
// never imports plugins; the session never imports plugins.
//
// Three entry points, wired by buildServer / AddPlayer in
// server.go:
//
//   - InstallSkillHooks, once in buildServer, before ticking
//     starts: populates the game.SkillHooks seam fields from the
//     hub and installs the engine TaskManager handle on the hub.
//   - installSkillExperienceRate, once in buildServer, after the
//     world definition loads: sets the world experience rate
//     from worldDef.ExperienceMultiplier.
//   - installSkillExpDropWiring, once per entering player in
//     AddPlayer's `if ep.Session != nil` wiring block, next to
//     installSkillsAppearanceWiring: installs the SkillManager
//     exp-drop hook calling Session.SendExpDrop.
//
// NIL SAFETY. A nil hub leaves the seam zero (inert: the gate
// stays open with multiplier 1, the emits are no-ops) and the
// TaskManager handle unset (Api.GetTaskManager returns nil). A
// nil world leaves the TaskManager handle unset but still
// installs the seam adapters. A nil player or session installs
// no exp-drop hook. With no content hooks registered, behavior
// is unchanged: the gate answers allow=nil/multiplier 1, the
// emits run zero hooks, and the exp-drop send has no wire
// effect (see installSkillExpDropWiring).

// InstallSkillHooks populates the game-side skills event seam
// from the hub and installs the engine TaskManager handle on the
// hub. A nil hub is a no-op: the seam stays zero (inert) and no
// handle is installed. Call once at boot, before ticking starts:
// the seam is read on every AddExperience call once the engine
// runs, and the TaskManager handle must exist before any content
// task is submitted.
func InstallSkillHooks(hub *plugins.Hub, world *game.World) {
	if hub == nil {
		return
	}
	// The gate: the hub emit's (*bool, float64) answer IS the
	// seam's answer (PluginManager.emitCanGainExperience,
	// PluginManager.ts:1665-1674 — allow nil when no hook
	// answered, a deny returning immediately with multiplier
	// reset to 1, the combined multiplier sanitized
	// finite-and-positive else 1). No translation is needed
	// beyond forwarding; the nil-hub default is unreachable
	// here because hub is non-nil, and the emit guards a nil
	// registry itself.
	game.SkillHooks.CanGainExperience = func(player *game.Player, skill, experience int) (*bool, float64) {
		return hub.EmitCanGainExperience(player, skill, experience)
	}
	// The level-up emit: the game event and the hub event carry
	// the same four fields (PluginPlayerLevelUpEvent,
	// PluginTypes.ts:94-99); the adapter copies them across.
	// A nil game event is a no-op (the hub emit guards nil
	// too; the check here keeps the adapter total).
	game.SkillHooks.EmitPlayerLevelUp = func(e *game.PlayerLevelUpEvent) {
		if e == nil {
			return
		}
		hub.EmitPlayerLevelUp(&plugins.PlayerLevelUpEvent{
			Player:   e.Player,
			Skill:    e.Skill,
			OldLevel: e.OldLevel,
			NewLevel: e.NewLevel,
		})
	}
	// The experience-gained emit: same shape translation
	// (PluginExperienceGainedEvent, PluginTypes.ts:102-106).
	// Experience is the landed post-cap delta on both sides,
	// never the pre-cap amount.
	game.SkillHooks.EmitExperienceGained = func(e *game.ExperienceGainedEvent) {
		if e == nil {
			return
		}
		hub.EmitExperienceGained(&plugins.ExperienceGainedEvent{
			Player:     e.Player,
			Skill:      e.Skill,
			Experience: e.Experience,
		})
	}
	// The engine TaskManager is the world's: the single instance
	// World.Process ticks in its task_manager phase (world.go).
	// Content reaches it through Api.GetTaskManager
	// (PluginTypes.ts:1377); a nil world leaves the handle unset
	// (nil until installed, per the hub contract).
	if world != nil {
		hub.SetTaskManager(world.Tasks())
	}
}

// installSkillExperienceRate sets the world experience rate the
// SkillManager applies to multiplied gains (GameConstants.
// EXPERIENCE_MULTIPLIER, GameConstants.ts:41, from world.json
// `experienceMultiplier`; config parses it with default 1,
// config/world.go:412-418). SetExperienceRate ignores
// non-finite/non-positive values, mirroring
// GameConstants.setExperienceRate, so passing the parsed value
// through unconditionally is safe. Call once at boot, before
// ticking starts.
func installSkillExperienceRate(rate float64) {
	game.SetExperienceRate(rate)
}

// installSkillExpDropWiring installs the SkillManager exp-drop
// hook for one entering player: the post-rate tenths value from
// AddExperience reaches Session.SendExpDrop at the TS position
// (SkillManager.ts:151, PacketSender.sendExpDrop,
// PacketSender.ts:1759-1767). The send has NO wire effect, in
// Go exactly as in TS: PlayerSession.write is a no-op at
// bfd81e3 (PlayerSession.ts:228-230), and the legacy opcode 116
// collides with WIDGET_SET_ROTATE_SPEED in the native table, so
// SendExpDrop builds the legacy layout and discards it (see
// session/skillsends.go and protocol/expdrop.go); the live XP
// display travels on the skills delta packets the update hook
// sends. The install still matters: it exercises the hook
// position, the skill guard, and the client-id translation, and
// if a native exp-drop encoder ever lands, only SendExpDrop
// changes. A nil player, session, or skill manager installs
// nothing. This is a parallel install next to
// installSkillsAppearanceWiring (appearancewiring.go), which
// owns the update hook; the two hooks are independent fields on
// the manager and neither install disturbs the other.
func installSkillExpDropWiring(p *game.Player, sess *session.Session) {
	if p == nil || sess == nil {
		return
	}
	skills := p.Skills()
	if skills == nil {
		return
	}
	skills.SetExpDropHook(func(skill, exp int) {
		sess.SendExpDrop(skill, exp)
	})
}
