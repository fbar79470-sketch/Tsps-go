// Skills & experience hooks and the TaskManager member: the Go form of
// the skills families of PluginApi (PluginTypes.ts) that the engine can
// fire since phase 20: onPlayerLevelUp, onExperienceGained, and
// onCanGainExperience, plus the getTaskManager member.
//
// Every emit below ports its PluginManager.emit* body, including the
// entry guards, the registration wrappers, the hook ordering, and the
// short-circuit/run-all shape — the families differ from each other,
// and those differences are behavior content relies on:
//
//   - emitExperienceGained (PluginManager.ts:1101-1106) and
//     emitPlayerLevelUp (PluginManager.ts:1108-1116): run ALL hooks
//     through executeHook; the zero-hooks early return is
//     unobservable. emitPlayerLevelUp's registration wrapper skips
//     events without a player or skill, or with non-integer levels
//     (PluginManager.ts:3328-3346); in Go the skill and integer
//     checks degenerate (see OnPlayerLevelUp). emitExperienceGained
//     has NO registration wrapper (PluginManager.ts:3325-3327):
//     the handler runs unconditionally.
//   - emitCanGainExperience (PluginManager.ts:1665-1674): the
//     veto-plus-accumulator contract (gates.go names it as needing
//     its own emit — it must not be forced into the gate helpers).
//     Every hook runs against ONE shared mutable event; a deny
//     returns immediately with the multiplier reset to 1; the
//     final multiplier is sanitized (finite and > 0, else 1);
//     no hooks yields allow nil + multiplier 1. NO registration
//     wrapper (PluginManager.ts:3832-3834): the handler runs
//     unconditionally.
//
// SKILL IDENTITY: TS events carry the Skill object; Go events carry
// the skill ARRAY INDEX (0..23), matching the game-side SkillsHooks
// seam (internal/game/skill_hooks.go) and the session
// skillClientIDs table. There is no Skill type in Go — skill
// metadata arrives as game-package functions (SkillName and
// friends), not as an object — so the index is the only honest
// form. Content that needs the client id translates through the
// session table's rule (only Construction/Hunter differ); the hub
// never deals in wire ids.
//
// EMIT SITES (Worker A / coordinator; this file owns only the hub
// side): all three fire from SkillManager.addExperience
// (SkillManager.ts:130-202) through the game-side SkillsHooks seam,
// installed by the cmd/server adapter — the gate first
// (SkillManager.ts:138-141; a deny returns before any send), the
// level-up emit when the new level exceeds the starting level
// (SkillManager.ts:177-192), and the experience-gained emit on
// every landed gain with the post-cap delta
// (SkillManager.ts:194). The skills:experience-gained custom event
// (SkillManager.ts:198) is a SEPARATE emit with the post-rate
// tenths amount: the two experience values differ by design, and
// the custom event rides the existing custom-event surface, not
// these families.
//
// TASKMANAGER (getTaskManager, PluginTypes.ts:1377; createApi,
// PluginManager.ts:4666): TS returns the TaskManager class itself
// — a static-only singleton (private constructor, static submit /
// process, TaskManager.ts) — and content submits session-tick
// tasks through it (the SkillActions.js session pattern:
// api.getTaskManager().submit(new MiningTask(sessions))). Go has
// no statics: the engine TaskManager is the World's single
// instance (World.Tasks()), and the adapter installs it through
// Hub.SetTaskManager. Api.GetTaskManager returns it, or nil until
// installed. game.TaskManager.Schedule is TS submit (task.go).
// There is deliberately NO Core.TaskManager accessor: core.TaskManager
// (PluginTypes.ts:1627) stays in core.go's NOT-mirrored list —
// this phase ports the PluginApi member only, as the plan counts
// it (18 -> 19 non-hook members).
//
// NO CORE SKILL SURFACE: core.Skill / core.SkillManager
// (PluginTypes.ts:1612-1613; getCoreApi, PluginManager.ts:
// 2666-2667) stay in core.go's NOT-mirrored list. The Skill class
// has no Go type (skill identity is the array index), and
// SkillManager is per-player state reached through
// Player.Skills(), not a class mirror — there is nothing honest
// for a Core accessor to return. getSkillManager
// (PluginTypes.ts:1388) likewise stays table-only in hooks.go.
//
// NOT PORTED: the level-up feedback itself (chat message, graphic,
// sounds, pop-up) is the SkillLevelUp plugin's content, NOT
// STARTED; the level-up:popup-opened payload contract and the
// pauseForLevelUp / actionInterval helpers live in the
// skillactions subpackage. The content-to-content skills:* events
// (skills:produced, skills:actions-per-cycle, skills:level-boost,
// skills:xp-locked, skills:combat-xp-locked) have no engine emit
// sites and no hub surface. pressedSkill is dead code in TS (zero
// callers) and is not ported.

package plugins

import (
	"math"

	"tsps-go/internal/game"
)

// PlayerLevelUpEvent is the payload of the onPlayerLevelUp hooks.
// It mirrors PluginPlayerLevelUpEvent (PluginTypes.ts:94-99): the
// player, the skill, and the old and new levels. Skill is the
// array index (see the file header); OldLevel is the level before
// the gain and NewLevel the level after, including multi-level
// gains (SkillManager.ts:177-192).
type PlayerLevelUpEvent struct {
	Player   *game.Player
	Skill    int
	OldLevel int
	NewLevel int
}

// ExperienceGainedEvent is the payload of the onExperienceGained
// hooks. It mirrors PluginExperienceGainedEvent
// (PluginTypes.ts:102-106): the player, the skill, and the
// experience that LANDED — the gain less anything over the
// experience cap (SkillManager.ts:194). It is not sent when the
// gain was refused, XP-locked, or already at the cap. Skill is
// the array index (see the file header). This value differs by
// design from the skills:experience-gained custom event's
// post-rate tenths amount (SkillManager.ts:198).
type ExperienceGainedEvent struct {
	Player     *game.Player
	Skill      int
	Experience int
}

// CanGainExperienceEvent is the payload of the onCanGainExperience
// hooks. It mirrors PluginCanGainExperienceEvent
// (PluginTypes.ts:344-354): the player, the skill, the requested
// experience (pre-rate), the allow answer, and the multiplier.
// Skill is the array index (see the file header). Allow is a
// *bool: nil is TS null (undecided), non-nil is the hook's
// answer; a hook sets it through BoolPtr (gates.go). Multiplier
// starts at 1 and hooks MULTIPLY what they find, so several
// stack (a league profile's rate lives here); the emit
// sanitizes the final value (finite and > 0, else 1). The event
// passed to hooks is the ONE shared mutable event TS builds
// (PluginManager.ts:1667): hooks mutate it in place, and later
// hooks see earlier hooks' answers.
type CanGainExperienceEvent struct {
	Player     *game.Player
	Skill      int
	Experience int
	Allow      *bool
	Multiplier float64
}

// --- hookRegistry storage for the skills families (see hooks.go) ---

func (r *hookRegistry) addPlayerLevelUp(e hookEntry[*PlayerLevelUpEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerLevelUp = append(r.playerLevelUp, e)
}

func (r *hookRegistry) snapshotPlayerLevelUp() []hookEntry[*PlayerLevelUpEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerLevelUpEvent](nil), r.playerLevelUp...)
}

func (r *hookRegistry) addExperienceGained(e hookEntry[*ExperienceGainedEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.experienceGained = append(r.experienceGained, e)
}

func (r *hookRegistry) snapshotExperienceGained() []hookEntry[*ExperienceGainedEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ExperienceGainedEvent](nil), r.experienceGained...)
}

func (r *hookRegistry) addCanGainExperience(e hookEntry[*CanGainExperienceEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canGainExperience = append(r.canGainExperience, e)
}

func (r *hookRegistry) snapshotCanGainExperience() []hookEntry[*CanGainExperienceEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanGainExperienceEvent](nil), r.canGainExperience...)
}

func (r *hookRegistry) setTaskManager(tm *game.TaskManager) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.taskManager = tm
}

func (r *hookRegistry) installedTaskManager() *game.TaskManager {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.taskManager
}

// --- registration methods ---

// OnPlayerLevelUp registers a player-level-up hook, mirroring
// onPlayerLevelUp (PluginTypes.ts:1026). Hooks run in registration
// order, all of them (EmitPlayerLevelUp). A nil handler is ignored
// silently, mirroring the createApi guard (PluginManager.ts:
// 3328-3346). The stored handler wraps the plugin's with the TS
// guard: an event without a player never reaches the plugin's
// handler. (The guard's !skill and Number.isInteger(oldLevel /
// newLevel) checks have no Go form beyond the field types: Skill
// is an int with no absent value — 0 is Attack, a valid skill —
// and Go ints are always integers, so the wrapper checks the
// event and player only.)
func (a *Api) OnPlayerLevelUp(handler func(*PlayerLevelUpEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerLevelUp") {
		return
	}
	a.hub.hooks.addPlayerLevelUp(hookEntry[*PlayerLevelUpEvent]{pluginName: a.name, handler: func(event *PlayerLevelUpEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnExperienceGained registers an experience-gained hook, mirroring
// onExperienceGained (PluginTypes.ts:1028). Hooks run in
// registration order, all of them (EmitExperienceGained). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3325-3327). There is no registration wrapper:
// the TS registration pushes the bare handler, so a handler
// receives even a player-less event — unlike OnPlayerLevelUp,
// whose wrapper skips one.
func (a *Api) OnExperienceGained(handler func(*ExperienceGainedEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onExperienceGained") {
		return
	}
	a.hub.hooks.addExperienceGained(hookEntry[*ExperienceGainedEvent]{pluginName: a.name, handler: handler})
}

// OnCanGainExperience registers a can-gain-experience hook,
// mirroring onCanGainExperience (PluginTypes.ts:1137). Hooks run
// in registration order, all of them unless one denies
// (EmitCanGainExperience). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3832-3834).
// There is no registration wrapper: the TS registration pushes
// the bare handler.
func (a *Api) OnCanGainExperience(handler func(*CanGainExperienceEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanGainExperience") {
		return
	}
	a.hub.hooks.addCanGainExperience(hookEntry[*CanGainExperienceEvent]{pluginName: a.name, handler: handler})
}

// --- Hub emits ---

// EmitPlayerLevelUp runs the player-level-up hooks in registration
// order, mirroring PluginManager.emitPlayerLevelUp
// (PluginManager.ts:1108-1116) with label "player_level_up":
// every hook runs. A nil Hub, a Hub without a registry, or a nil
// event runs no hooks (the nil-event guard is Go-only; the
// registration wrappers would skip a nil event anyway, so the
// outcome is identical). Events without a player run no plugin
// handler, through the wrappers.
//
// The production call site is SkillManager.addExperience, through
// the game-side SkillsHooks seam (see the file header).
func (h *Hub) EmitPlayerLevelUp(event *PlayerLevelUpEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "player_level_up", h.hooks.snapshotPlayerLevelUp(), event)
}

// EmitExperienceGained runs the experience-gained hooks in
// registration order, mirroring
// PluginManager.emitExperienceGained (PluginManager.ts:1101-1106)
// with label "experience_gained": every hook runs. A nil Hub, a
// Hub without a registry, or a nil event runs no hooks (the
// nil-event guard is Go-only: TS has no nil-event equivalent,
// and a nil event reaching the bare handlers would panic inside
// executeHook's recovery — the guard keeps the emit total
// without changing any real call, since the production site
// always passes a real event). There is deliberately NO
// player guard: the TS registration has no wrapper, so a
// player-less event reaches the handlers, and Go matches.
//
// The production call site is SkillManager.addExperience, through
// the game-side SkillsHooks seam (see the file header).
func (h *Hub) EmitExperienceGained(event *ExperienceGainedEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "experience_gained", h.hooks.snapshotExperienceGained(), event)
}

// EmitCanGainExperience asks the registered hooks whether player
// may gain experience in skill, and at what combined multiplier,
// mirroring PluginManager.emitCanGainExperience
// (PluginManager.ts:1665-1674) with label "can_gain_experience":
// every hook runs against one shared mutable event (allow nil,
// multiplier 1); a deny returns immediately with the multiplier
// reset to 1, and hooks after it never run; the final multiplier
// is sanitized (finite and > 0, else 1); no hooks yields allow
// nil + multiplier 1. The returned allow is a fresh pointer
// (boolAnswer, gates.go), never a pointer a hook stored. A nil
// Hub or a Hub without a registry yields the no-hooks default.
//
// There is deliberately NO player guard: the TS emit builds the
// event from whatever player it is passed and the registration
// has no wrapper, so hooks receive exactly what was passed.
//
// The production call site is SkillManager.addExperience, through
// the game-side SkillsHooks seam: only a false answer refuses
// the gain (SkillManager.ts:138-141, `=== false`); the returned
// multiplier multiplies the world's rate (SkillManager.ts:144).
// See the file header for the veto-plus-accumulator contract.
func (h *Hub) EmitCanGainExperience(player *game.Player, skill, experience int) (*bool, float64) {
	if h == nil || h.hooks == nil {
		return nil, 1
	}
	hooks := h.hooks.snapshotCanGainExperience()
	if len(hooks) == 0 {
		return nil, 1
	}
	event := &CanGainExperienceEvent{Player: player, Skill: skill, Experience: experience, Multiplier: 1}
	for _, hook := range hooks {
		executeHook(h.Logger(), hook.pluginName, "can_gain_experience", hook.handler, event)
		if event.Allow != nil && !*event.Allow {
			return boolAnswer(false), 1
		}
	}
	multiplier := event.Multiplier
	if math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || multiplier <= 0 {
		multiplier = 1
	}
	if event.Allow == nil {
		return nil, multiplier
	}
	return boolAnswer(*event.Allow), multiplier
}

// --- TaskManager exposure ---

// SetTaskManager installs the engine TaskManager content reads
// through Api.GetTaskManager. The coordinator calls it once at
// boot with the World's instance (World.Tasks()); calling it
// with nil clears the handle (GetTaskManager then returns nil).
// It is a no-op on a nil Hub or a Hub without a registry.
func (h *Hub) SetTaskManager(tm *game.TaskManager) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setTaskManager(tm)
}

// TaskManager returns the installed engine TaskManager, or nil
// when none is installed (or the Hub is nil). It mirrors
// Hub.ObjectManager: the read half of the install seam, for the
// coordinator and tests.
func (h *Hub) TaskManager() *game.TaskManager {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedTaskManager()
}

// GetTaskManager returns the engine TaskManager, mirroring the
// PluginApi getTaskManager member (PluginTypes.ts:1377) and its
// createApi implementation (PluginManager.ts:4666), which
// returns the TaskManager class — the static-only singleton
// whose submit content calls to run session-tick tasks. The Go
// form is the *game.TaskManager the adapter installed through
// Hub.SetTaskManager (the World's single instance); Schedule is
// TS submit. It returns nil until the adapter installs it:
// content must check for nil before scheduling. This is a read,
// not a registration: it has no load-phase gate, mirroring the
// TS closure, which has no phase check. A nil Api, or an Api
// with a nil hub, yields nil.
func (a *Api) GetTaskManager() *game.TaskManager {
	if a == nil || a.hub == nil {
		return nil
	}
	return a.hub.TaskManager()
}
