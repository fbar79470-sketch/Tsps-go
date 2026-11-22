// Shared request/event types: the crafting package's ONE home for
// the custom-event payloads its units emit, answer, and listen to.
// No duplicates: the unit worker uses these definitions.
//
// Every payload is emitted by pointer so a handler's in-place
// mutation is visible to the emitter (the woodcutting/fletching/
// cooking query precedent). Every query defaults fail-closed when
// unanswered: the request's zero value is the "no content answered"
// answer, documented at the emit site.
package crafting

import (
	"tsps-go/internal/game"
)

// Event names the crafting units emit or answer.
const (
	// LevelBoostEventName is the "skills:level-boost" query
	// (Shared.Crafting.js craftingLevel): content may raise the
	// checked level (a league relic's invisible +12).
	LevelBoostEventName = "skills:level-boost"
	// ActionsPerCycleEventName is the "skills:actions-per-cycle"
	// query (Shared.Crafting.js actionsPerCycle): content may
	// make one cycle make more items.
	ActionsPerCycleEventName = "skills:actions-per-cycle"
	// ActionIntervalEventName is the "skills:action-interval"
	// query (SkillActions.js actionInterval, via
	// skillactions.ActionInterval): content may lower the ticks
	// between a loop's items.
	ActionIntervalEventName = "skills:action-interval"
	// SkillsProducedEventName is the "skills:produced" emit
	// (Shared.Crafting.js produced).
	SkillsProducedEventName = "skills:produced"
	// CutGemEventName is the "crafting:cut-gem" query
	// (Gems.Crafting.js answerCutGem): the cut gem and XP for
	// an uncut gem id, for content that cuts as it mines.
	CutGemEventName = "crafting:cut-gem"
	// InteractionClearedEventName is the
	// "player:interaction-cleared" stop (Crafting.plugin.js):
	// walking away / interaction cleared stops the active
	// make-many loop.
	InteractionClearedEventName = "player:interaction-cleared"
	// CraftingMadeEventName is the "crafting:made" emit
	// (Leather/Snelms/Jewellery): a crafted item, for content
	// that listens (no Go consumer yet).
	CraftingMadeEventName = "crafting:made"
	// PotteryFiredEventName is the "pottery:fired" emit
	// (Pottery.Crafting.js makePottery): an oven really fired
	// a piece (a cracked one never emits), for content that
	// listens (no Go consumer yet).
	PotteryFiredEventName = "pottery:fired"
	// LightOrbMadeEventName is the "crafting:light-orb-made"
	// emit (LightOrbs.Crafting.js): an empty light orb was
	// completed with cave goblin wire, for content that
	// listens (no Go consumer yet).
	LightOrbMadeEventName = "crafting:light-orb-made"
	// GemSickleMadeEventName is the "crafting:gem-sickle-made"
	// emit (GemSickles.Crafting.js craft): a gem was embedded
	// in a silver sickle (b), for the quest that wanted it (no
	// Go consumer yet).
	GemSickleMadeEventName = "crafting:gem-sickle-made"
	// GemSickleAllowedEventName is the
	// "crafting:gem-sickle-allowed" query
	// (GemSickles.Crafting.js allowed): the quest that wants
	// a gem sickle answers whether the player may make it now.
	// No Go answerer exists, so the gate fails closed (gap
	// 27d).
	GemSickleAllowedEventName = "crafting:gem-sickle-allowed"
	// QuestStageEventName is the "quest:get-stage" query
	// (Pottery.Crafting.js recipeUnlocked): the quest runtime
	// answers the player's saved stage for a quest key. No Go
	// answerer exists, so the pot-lid gate fails closed (gap
	// 27d).
	QuestStageEventName = "quest:get-stage"
	// DiaryCompleteEventName is the "diary:is-complete" query
	// (Spinning.Crafting.js spinTicks): the diary runtime
	// answers whether a diary tier is complete. No Go
	// answerer exists, so the Seers' 2-tick flax rule never
	// applies (gap 27f).
	DiaryCompleteEventName = "diary:is-complete"
	// CraftingSpunEventName is the "crafting:spun" emit
	// (Spinning.Crafting.js makeOne): one spun item, emitted
	// PER ITEM with amount 1, after the item and the XP land —
	// Fletching's spool listens (fletching.CraftingSpunEvent).
	CraftingSpunEventName = "crafting:spun"
)

// LevelBoostRequest is the "skills:level-boost" query
// (Shared.Crafting.js craftingLevel): content may raise the
// checked level; Boost defaults 0 and is clamped >= 0 at the
// read site, so an unanswered query reads the plain current
// level. No Go answerer exists (gap 27c).
type LevelBoostRequest struct {
	Player *game.Player
	Skill  int
	Boost  int
}

// ActionsPerCycleRequest is the "skills:actions-per-cycle"
// query (Shared.Crafting.js actionsPerCycle): content may make
// one cycle make more items; Actions defaults 1 and is clamped
// >= 1 at the read site. No Go answerer exists (gap 27c).
type ActionsPerCycleRequest struct {
	Player  *game.Player
	Skill   int
	Kind    string
	Actions int
}

// ProducedEvent is the "skills:produced" emit
// (Shared.Crafting.js produced): the player, the skill, the
// produced item, how many, the XP the action paid (float, as TS
// emits it — the int-truncated XP is what lands in the
// SkillManager), the loop kind ("" when TS passes null), and the
// materials one product used ([id, amount] pairs, nil when TS
// passes null). No Go consumer exists yet (gap 27c).
type ProducedEvent struct {
	Player    *game.Player
	Skill     int
	ItemID    int
	Amount    int
	XP        float64
	Kind      string
	Materials [][2]int
}

// CutGemRequest is the "crafting:cut-gem" query
// (Gems.Crafting.js answerCutGem): UncutID asks what the uncut
// gem cuts into; CutID and XP answer it, and stay zero when the
// item is not a cuttable gem. No Go emitter exists yet.
type CutGemRequest struct {
	UncutID int
	CutID   int
	XP      float64
}

// InteractionClearedEvent is the "player:interaction-cleared"
// payload (Crafting.plugin.js stopCraftingOnInteractionCleared):
// the player whose interaction cleared.
type InteractionClearedEvent struct {
	Player *game.Player
}

// CraftingMadeEvent is the "crafting:made" emit (Leather /
// Snelms / Jewellery): the player and the item made; Furnace
// marks a furnace-made piece (Jewellery only).
type CraftingMadeEvent struct {
	Player  *game.Player
	ItemID  int
	Furnace bool
}

// PotteryFiredEvent is the "pottery:fired" emit
// (Pottery.Crafting.js makePottery): the player, the oven
// object that fired the piece, and the fired item.
type PotteryFiredEvent struct {
	Player   *game.Player
	ObjectID int
	ItemID   int
}

// LightOrbMadeEvent is the "crafting:light-orb-made" emit
// (LightOrbs.Crafting.js): the player and the completed light
// orb.
type LightOrbMadeEvent struct {
	Player *game.Player
	ItemID int
}

// GemSickleMadeEvent is the "crafting:gem-sickle-made" emit
// (GemSickles.Crafting.js craft): the player and the sickle
// made.
type GemSickleMadeEvent struct {
	Player  *game.Player
	Product int
}

// GemSickleAllowedRequest is the
// "crafting:gem-sickle-allowed" query
// (GemSickles.Crafting.js allowed): the quest that wants the
// sickle answers by setting Allowed; it defaults false, so
// an unanswered query leaves the use alone (gap 27d).
type GemSickleAllowedRequest struct {
	Player  *game.Player
	Product int
	Allowed bool
}

// QuestStageRequest is the "quest:get-stage" query
// (Pottery.Crafting.js recipeUnlocked): the quest runtime
// answers the player's saved stage for Quest; Stage defaults
// 0, so an unanswered query reads stage 0 and the pot-lid
// unlock (stage 34) stays locked (gap 27d).
type QuestStageRequest struct {
	Player *game.Player
	Quest  string
	Stage  int
}

// DiaryCompleteRequest is the "diary:is-complete" query
// (Spinning.Crafting.js spinTicks): the diary runtime
// answers whether the player completed Diary's Tier.
// Complete defaults false, so an unanswered query reads
// incomplete and the Seers' 2-tick flax rule never applies
// (gap 27f).
type DiaryCompleteRequest struct {
	Player   *game.Player
	Diary    string
	Tier     string
	Complete bool
}
