// Shared request/event types: the cooking package's ONE home for
// the custom-event payloads its units emit, answer, and listen to
// (PHASE36_PLAN.md §3 shared contract). No duplicates: Workers B
// and C use these definitions. Two exceptions live elsewhere, by
// the plan: "cooking:raw-xp" uses *fishing.CookingRawXPRequest
// (internal/content/fishing/infernalharpoon.go — do NOT redefine
// it), and "skillcape:has-perk" uses the game package types
// (game.SkillcapeHasPerkEventName / game.SkillcapePerkRequest).
//
// Every payload is emitted by pointer so a handler's in-place
// mutation is visible to the emitter (the woodcutting/fletching
// query precedent). Every query defaults fail-closed when
// unanswered: the request's zero value is the "no content
// answered" answer, documented at the emit site.
package cooking

import (
	"tsps-go/internal/game"
)

// Event names the cooking units emit or answer.
const (
	// LevelBoostEventName is the "skills:level-boost" query
	// (Common.Cooking.js:121-125).
	LevelBoostEventName = "skills:level-boost"
	// ActionsPerCycleEventName is the "skills:actions-per-cycle"
	// query (Common.Cooking.js:128-132).
	ActionsPerCycleEventName = "skills:actions-per-cycle"
	// SkillcapeHasPerkEventName is the "skillcape:has-perk"
	// query (Common.Cooking.js:135-139); the constant lives in
	// the game package — this alias keeps cooking call sites
	// on the plan's naming.
	SkillcapeHasPerkEventName = game.SkillcapeHasPerkEventName
	// SkillsProducedEventName is the "skills:produced" emit
	// (Recipes.Cooking.js:91, Cookers.Cooking.js cookOne).
	SkillsProducedEventName = "skills:produced"
	// CookingRawXPEventName is the "cooking:raw-xp" query
	// (Cookers.Cooking.js answerRawXp): the payload is
	// *fishing.CookingRawXPRequest, NOT a type in this package.
	CookingRawXPEventName = "cooking:raw-xp"
	// CookingCookedEventName is the "cooking:cooked" query
	// (Cookers.Cooking.js answerCooked).
	CookingCookedEventName = "cooking:cooked"
	// CookingBurnEventName is the "cooking:burn" emit
	// (Cookers.Cooking.js cookOne): a handler may flip Burn.
	CookingBurnEventName = "cooking:burn"
	// CookingBurntEventName is the "cooking:burnt" emit
	// (Cookers.Cooking.js cookOne).
	CookingBurntEventName = "cooking:burnt"
	// CookingSuccessEventName is the "cooking:success" emit
	// (Cookers.Cooking.js cookOne).
	CookingSuccessEventName = "cooking:success"
	// CookingCountEventName is the "cooking:count" query
	// (Cookers.Cooking.js countCookEvent).
	CookingCountEventName = "cooking:count"
	// ItemsObtainedEventName is the "items:obtained" emit
	// (Cookers.Cooking.js cookOne).
	ItemsObtainedEventName = "items:obtained"
	// SacredEelEventName is the "cooking:sacred-eel-dissected"
	// emit (SacredEel.Cooking.js dissect).
	SacredEelEventName = "cooking:sacred-eel-dissected"
	// DiaryCompleteEventName is the "diary:is-complete" query
	// (Cookers.Cooking.js Hosidius bonus).
	DiaryCompleteEventName = "diary:is-complete"
	// QuestStateEventName is the "quest:is-complete" /
	// "quest:is-started" query family (Cookers.Cooking.js
	// canCook: the Lumbridge range and duringQuest gates).
	QuestStateEventName = "quest:is-complete"
	// QuestStartedEventName is the "quest:is-started" query.
	QuestStartedEventName = "quest:is-started"
	// RumoursCompletedEventName is the "hunter:rumours-completed"
	// query (Cookers.Cooking.js rumoursMet).
	RumoursCompletedEventName = "hunter:rumours-completed"
)

// LevelBoostRequest is the "skills:level-boost" query
// (Common.Cooking.js:121-125): content may raise the checked
// level (a league relic's invisible +12); Boost defaults 0 and
// is clamped >= 0 at the read site, so an unanswered query
// reads the plain current level.
type LevelBoostRequest struct {
	Player *game.Player
	Skill  int
	Boost  int
}

// ActionsPerCycleRequest is the "skills:actions-per-cycle" query
// (Common.Cooking.js:128-132): content may make one cycle make
// more items (a league relic's all at once); Actions defaults 1
// and is clamped >= 1 at the read site.
type ActionsPerCycleRequest struct {
	Player  *game.Player
	Skill   int
	Kind    string
	Actions int
}

// DiaryCompleteRequest is the "diary:is-complete" query
// (Cookers.Cooking.js Hosidius Kitchen bonus): Complete
// defaults false, so an unanswered query reads the diary as
// incomplete (the Hosidius bonus is 12, never 25, until a
// diary answerer exists).
type DiaryCompleteRequest struct {
	Player   *game.Player
	Diary    string
	Tier     string
	Complete bool
}

// QuestStateRequest is the "quest:is-complete" /
// "quest:is-started" query (Cookers.Cooking.js canCook):
// content answers Started / Complete; both default false, so
// an unanswered query FAILS CLOSED (quests are unported — the
// Lumbridge Castle range stays refused and a duringQuest food
// never cooks).
type QuestStateRequest struct {
	Player   *game.Player
	Key      string
	Started  bool
	Complete bool
}

// RumoursCompletedRequest is the "hunter:rumours-completed"
// query (Cookers.Cooking.js rumoursMet): Completed defaults 0,
// so an unanswered query reads no rumours done and a rumours
// food never cooks (Hunter is unported — fail-closed).
type RumoursCompletedRequest struct {
	Player    *game.Player
	Completed int
}

// BurnRequest is the "cooking:burn" emit (Cookers.Cooking.js
// cookOne): the raw food, the item it would become, and whether
// it burns. A handler may flip Burn after the emit; the caller
// reads it after, as TS does. No Go consumers exist yet.
type BurnRequest struct {
	Player *game.Player
	RawID  int
	ItemID int
	Burn   bool
}

// BurntEvent is the "cooking:burnt" emit (Cookers.Cooking.js
// cookOne): the raw food and the burnt item it became.
type BurntEvent struct {
	Player *game.Player
	RawID  int
	ItemID int
}

// SuccessEvent is the "cooking:success" emit
// (Cookers.Cooking.js cookOne): the player, the skill, the raw
// food, the cooked item, how many, where the player stood, and
// the cooker object (its id and location).
type SuccessEvent struct {
	Player         *game.Player
	Skill          int
	RawID          int
	ItemID         int
	Amount         int
	Location       game.Location
	ObjectID       int
	ObjectLocation game.Location
}

// ProducedEvent is the "skills:produced" emit
// (Recipes.Cooking.js:91, Cookers.Cooking.js cookOne, and the
// Wine/Churn/SacredEel/LeapingFish/Gnome units): the player, the
// skill, the produced item, how many, the XP the action paid
// (float, as TS emits it — the int-truncated XP is what lands
// in the SkillManager), and the kind ("make", "cook",
// "ferment", "churn", "sacred-eel", "leaping-fish").
type ProducedEvent struct {
	Player *game.Player
	Skill  int
	ItemID int
	Amount int
	XP     float64
	Kind   string
}

// CookedRequest is the "cooking:cooked" query
// (Cookers.Cooking.js:650-656 answerCooked): RawID asks what
// the raw food cooks into; CookedID and XP answer it, and stay
// zero when the food is not cookable or is facility-only. XP is
// the JSON float, as TS sets it (cookable.xp). No Go emitter
// exists yet — define the request type here, document the TS
// shape.
type CookedRequest struct {
	Player   *game.Player
	RawID    int
	CookedID int
	XP       float64
}

// CountEvent is the "cooking:count" query (Cookers.Cooking.js
// countCookEvent): Burnt reports whether the count is for a
// burn (count 0) or a success (count +1). The count itself
// lives in the cooking:success-count attribute, not here.
type CountEvent struct {
	Player *game.Player
	Burnt  bool
}

// ObtainedEvent is the "items:obtained" emit
// (Cookers.Cooking.js cookOne): the player, the cooked item,
// the raw food it came from, how many, the source ("cooking")
// and where the player stood. No Go consumers exist yet.
type ObtainedEvent struct {
	Player   *game.Player
	ItemID   int
	RawID    int
	Amount   int
	Source   string
	Location game.Location
}

// SacredEelEvent is the "cooking:sacred-eel-dissected" emit
// (SacredEel.Cooking.js dissect): the player, the dissected eel
// item, and how many Zulrah's scales it gave.
type SacredEelEvent struct {
	Player *game.Player
	ItemID int
	Scales int
}
