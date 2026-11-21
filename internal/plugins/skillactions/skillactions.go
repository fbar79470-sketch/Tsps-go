// Package skillactions is the Go form of the shared skill helpers in
// server/plugins/skills/SkillActions.js (30 lines) at commit bfd81e3:
// pauseForLevelUp and actionInterval. Skill content in TS requires
// SkillActions.js and shares these two functions across 15+ skill
// files; Go content imports this package instead.
//
// The package must NOT import internal/plugins: the emitter in
// ActionInterval is a plain func, so content passes
// api.EmitCustomEvent (or any func with the same shape) and this
// package stays decoupled from the hub, mirroring how the TS helper
// takes "the plugin api (or anything with emitCustomEvent)".
// Skill identity is a plain int index everywhere (Skill.ts
// declaration order), as in internal/game.
package skillactions

import "tsps-go/internal/game"

// LevelUpPopupOpenedEventName is the custom event whose payload is
// LevelUpPopupOpenedPayload: "level-up:popup-opened", emitted by the
// SkillLevelUp plugin (content, NOT STARTED in Go) when it opens the
// level-up pop-up. It is NOT the onPlayerLevelUp hook event —
// PluginPlayerLevelUpEvent has no pause field (see PHASE20_PLAN.md).
const LevelUpPopupOpenedEventName = "level-up:popup-opened"

// ActionIntervalEventName is the custom event ActionInterval emits:
// "skills:action-interval". Content may lower the payload's Ticks
// (a league relic's 2-tick pace); the tool saving applies after
// content's answer so the two stack.
const ActionIntervalEventName = "skills:action-interval"

// LevelUpPopupOpenedPayload is the "level-up:popup-opened"
// custom-event payload (SkillActions.js pauseForLevelUp's `event`):
// the player, the skill that levelled, and Pause, supplied by the
// SkillLevelUp plugin (openSkillPopup: `pause: (action, resume,
// cancel) => state.actions.set(...)`). Pause is nil when no pop-up
// is holding the action — in that case PauseForLevelUp stops the
// action instead of pausing it.
type LevelUpPopupOpenedPayload struct {
	Player *game.Player
	Skill  int
	// Pause registers the action with the pop-up: action is the
	// session being held, resume restarts it when the pop-up
	// closes, cancel stops it. Both closures are
	// identity-checked by PauseForLevelUp (see its doc comment).
	Pause func(action any, resume, cancel func())
}

// Session is one skill action's per-player session: the Go form of
// the session objects TS skill plugins keep in a Map keyed by
// player (Mining.plugin.js, Woodcutting.plugin.js,
// Fishing.plugin.js, Prayer.plugin.js). The only field the shared
// helper touches is the level-up pause flag; every session tick
// loop skips sessions whose flag is set. Sessions hold pointers
// in practice, and PauseForLevelUp's identity checks compare
// sessions with ==, so implementations MUST be comparable
// (pointer) values: a non-comparable session type panics the
// identity check, exactly as a non-identity key would break the
// TS Map comparison.
type Session interface {
	SetLevelUpPaused(bool)
}

// PauseForLevelUp holds a skill's existing session through a
// level-up dialogue, without restarting its work
// (pauseForLevelUp, SkillActions.js:2-14), exactly:
//
//   - No session for the player in sessions: return, doing nothing.
//   - No Pause on the event: stop(player) — there is no pop-up to
//     hold the action, so the action ends.
//   - Otherwise: mark the session paused and register resume/cancel
//     closures with Pause. Resume no-ops if the session was
//     replaced (the map no longer holds this session); otherwise
//     it clears the flag and calls onResume with the session.
//     Cancel stops the player only if the map still holds this
//     session.
//
// Go-only guards: a nil event, a nil session value in the map, a
// nil stop, and a nil onResume are all no-ops rather than panics.
// TS has no nil-event equivalent; the outcome for a real event is
// identical.
func PauseForLevelUp(event *LevelUpPopupOpenedPayload, sessions map[*game.Player]Session, stop func(*game.Player), onResume func(Session)) {
	if event == nil {
		return
	}
	player := event.Player
	session := sessions[player]
	if session == nil {
		return
	}
	if event.Pause == nil {
		if stop != nil {
			stop(player)
		}
		return
	}
	session.SetLevelUpPaused(true)
	event.Pause(session, func() {
		if sessions[player] != session {
			return
		}
		session.SetLevelUpPaused(false)
		if onResume != nil {
			onResume(session)
		}
	}, func() {
		if sessions[player] == session && stop != nil {
			stop(player)
		}
	})
}

// ActionIntervalPayload is the "skills:action-interval" request
// (SkillActions.js actionInterval's `request`): the player, the
// skill, the loop kind, and the ticks to the next repeated action.
// Content handlers mutate Ticks in place; ActionInterval reads it
// back after the emit, so the payload is always passed to the
// emitter as a *ActionIntervalPayload.
type ActionIntervalPayload struct {
	Player *game.Player
	Skill  int
	Kind   string
	Ticks  int
}

// ActionInterval returns the ticks to a skill's next repeated
// action (a roll, a made item, an offering, a cast): `ticks`
// unless content lowers it through the "skills:action-interval"
// event, minus `saving` (a tool's ticks off the pace, taken after
// content's answer so the two stack), never under 1
// (actionInterval, SkillActions.js:23-27: Math.max(1,
// (request.ticks|0) - saving)). A nil emitter emits nothing and
// the base ticks apply. The payload is emitted by pointer so a
// handler lowering Ticks is visible to the read-back.
func ActionInterval(emit func(name string, payload any), player *game.Player, skill int, kind string, ticks, saving int) int {
	request := &ActionIntervalPayload{Player: player, Skill: skill, Kind: kind, Ticks: ticks}
	if emit != nil {
		emit(ActionIntervalEventName, request)
	}
	if interval := request.Ticks - saving; interval > 1 {
		return interval
	}
	return 1
}
