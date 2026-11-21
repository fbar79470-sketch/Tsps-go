package game

import "math"

// SkillsHooks is the game-side skills event seam: the func fields
// SkillManager calls for the events TS emits through PluginManager,
// and the policy gate TS consults before granting experience. It
// mirrors the CombatHooks pattern in combat_hooks.go (func fields
// only; the game package must never import internal/plugins): the
// coordinator wiring populates the package-level SkillHooks var
// from the plugin hub, every call site checks for nil, and an
// unpopulated seam is inert — the gate is open with multiplier 1
// and the emits are no-ops, exactly the no-hooks behavior of
// PluginManager.emitCanGainExperience (PluginManager.ts:1665-1674,
// which answers {allow: null, multiplier: 1} with no hooks).
//
// Event shapes are grounded in PluginTypes.ts and the emit sites
// in SkillManager.ts. Where TS payloads carry a Skill object, the
// Go shape carries the skill index, matching the rest of this
// package (see skills.go IDENTITY).

// PlayerLevelUpEvent is PluginPlayerLevelUpEvent
// (PluginTypes.ts:94-99): a level-up that just landed.
// OldLevel is the base level before the gain, NewLevel the base
// level after it (SkillManager.ts:185-191: the maxLevel array
// value after the increment, read after the new levels are
// published through SetCurrentLevels).
type PlayerLevelUpEvent struct {
	Player          *Player
	Skill, OldLevel int
	NewLevel        int
}

// ExperienceGainedEvent is PluginExperienceGainedEvent
// (PluginTypes.ts:101-106): XP that just landed in a skill, after
// rates and the experience cap. Experience is the LANDED post-cap
// delta (SkillManager.ts:196: stored experience minus the
// pre-gain sanitized experience), never the pre-cap amount; it is
// not sent when the gain was refused, XP-locked, or already at
// the cap.
type ExperienceGainedEvent struct {
	Player     *Player
	Skill      int
	Experience int
}

// SkillsHooks holds the skills hook fields. All fields are
// optional; SkillManager calls them nil-safely. The wiring
// populates SkillHooks once at boot before ticking starts.
type SkillsHooks struct {
	// CanGainExperience is PluginManager.emitCanGainExperience
	// (PluginManager.ts:1665-1674): the XP gate. It answers the
	// hooks' combined verdict for a gain of experience in
	// skill for player: allow nil means no hook answered (the
	// gain proceeds), allow false refuses it, and multiplier
	// is the combined rate hooks set (1 when none set one; the
	// hub sanitizes it finite-and-positive else 1, and
	// AddExperienceWithMultipliers re-applies that sanitize at
	// the seam boundary so a misbehaving seam cannot poison
	// the rate). A deny returns before any send, per TS order
	// (SkillManager.ts:136-139).
	CanGainExperience func(player *Player, skill, experience int) (allow *bool, multiplier float64)
	// EmitPlayerLevelUp fires the player-level-up event
	// (SkillManager.ts:185-191, PluginManager.ts:1108-1116).
	EmitPlayerLevelUp func(*PlayerLevelUpEvent)
	// EmitExperienceGained fires the experience-gained event
	// (SkillManager.ts:196, PluginManager.ts:1101-1107).
	EmitExperienceGained func(*ExperienceGainedEvent)
}

// SkillHooks is the populated skills hook seam. The coordinator
// wiring assigns its fields at boot; SkillManager reads it
// nil-safely field by field. It is a var (not a pointer) so a
// zero SkillsHooks is a valid inert seam.
var SkillHooks SkillsHooks

// experienceRate is the Go value of
// GameConstants.EXPERIENCE_MULTIPLIER (GameConstants.ts:41, from
// world.json experienceMultiplier): the world's XP rate. The
// coordinator sets it once at boot from the loaded world
// definition; SkillManager reads it on the tick goroutine. Set
// before serving, like the hook seam above; no lock guards it.
var experienceRate = 1.0

// SetExperienceRate ports GameConstants.setExperienceRate
// (GameConstants.ts:119-124): set the world's XP rate, ignoring
// non-finite and non-positive values (the current rate is kept).
func SetExperienceRate(rate float64) {
	if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 {
		return
	}
	experienceRate = rate
}

// ExperienceRate returns the world's XP rate (default 1.0).
func ExperienceRate() float64 {
	return experienceRate
}

// SkillsExperienceGainedPayload is the "skills:experience-gained"
// custom-event payload (SkillManager.ts:197-202): the gain as it
// was added, after the rates and the tenths step, for content
// that reacts to experience itself (a league relic's bonus XP).
// Experience is the post-rate tenths amount as an int — Go
// stores integer experience, so the tenths below 1 are dropped
// at the same point TS's putInt truncates them for the exp drop
// (see AddExperienceWithMultipliers); it deliberately differs
// from ExperienceGainedEvent.Experience, the landed post-cap
// delta. Multiplied says whether the rates applied, and Rate is
// what they came to (1 when they did not apply). Emitted as a
// VALUE through Player.EmitCustomEvent, matching the
// interface-state custom events (interface_state.go); nothing
// reads the payload back.
type SkillsExperienceGainedPayload struct {
	Player     *Player
	Skill      int
	Experience int
	Multiplied bool
	Rate       float64
}

// SkillsCurrentLevelChangedPayload is the
// "skills:current-level-changed" custom-event payload
// (SkillManager.ts:530-535): the previous and new current
// (temporary) levels. Emitted as a value; see
// SkillsExperienceGainedPayload.
type SkillsCurrentLevelChangedPayload struct {
	Player          *Player
	Skill, Previous int
	Current         int
}

// SkillsRestorePayload is the "skills:restore" custom-event
// payload (SkillManager.ts:589-594): a restore or boost, whatever
// its source. Amount is MUTABLE by handlers (a league passive's
// Prayer gain); Max stays the caller's cap. Unlike the other two
// payloads it is emitted as a POINTER through
// Player.EmitCustomEvent, so a handler's mutation is visible to
// IncreaseCurrentLevel's read-back — the Go form of TS mutating
// the shared restore object. Handlers must type-assert
// *SkillsRestorePayload.
type SkillsRestorePayload struct {
	Player *Player
	Skill  int
	Amount int
	Max    int
}
