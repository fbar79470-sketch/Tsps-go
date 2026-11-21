// Common.Cooking.js (192 lines, commit bfd81e3): the shared
// helpers every Cooking unit uses — the success-chance interp
// formula, the level / actions / cape queries, the message and
// varbit helpers, the knives, the XP grant, and the 1-tick
// keyed step runner — plus the cooking:raw-xp and
// cooking:cooked answerers (Worker A owns them).
//
// XP INTEGER TRUNCATION (the woodcutting/fletching structural
// gap): the JSON pays float XP (82.5, 211.3, 40.5); Go
// SkillManager.AddExperiences takes an int, so addXP truncates
// at the grant site (82.5 → 82). The skills:produced emit still
// carries the float XP, as TS emits it. No rounding is invented.
package cooking

import (
	"math"
	"strings"

	"tsps-go/internal/content/fishing"
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// FreeToPlayItemMessage is FreeToPlay.ITEM_MESSAGE
// (FreeToPlay.plugin.js:8): the refusal line for a members
// recipe (or members food) on a free world. No Go constant
// existed for it; it is defined once here.
const FreeToPlayItemMessage = "To use this item please login to a members' world."

// knifeNames is KNIFE_NAMES (Common.Cooking.js:164): every
// knife a knife-on-item cut accepts, resolved via ItemIds with
// missing names filtered out (the loader resolves them
// leniently into Tables.KnifeIDs).
var knifeNames = []string{"KNIFE", "KNIFE_2", "KNIFE_3", "KNIFE_4", "KNIFE_5"}

// spamChatType is the chat channel for recipe lines: the spam
// channel (protocol.ChatTypeSpam, 105 — the game package cannot
// import protocol, so the value is spelled out with its
// provenance, as fletching spells it out).
const spamChatType = 105

// ---- success chance (Common.Cooking.js:115-118) ----

// successChance is the chance of cooking a food at level, from
// its low and high Wiki values out of 256 (Wiki: Module:Skilling
// success chart, `interp`): a straight line from low at level 1
// to high at 99, divided by 256, clamped to [0, 1]. NOTE: uncapped
// at 99 — levels above 99 keep raising it, as in TS (the same
// formula family as Fishing catchChance).
func successChance(low, high, level int) float64 {
	value := math.Floor(float64(low*(99-level))/98+float64(high*(level-1))/98+0.5) + 1
	return math.Min(math.Max(value/256, 0), 1)
}

// ---- level, actions, cape (Common.Cooking.js:121-139) ----

// cookingLevel is the Cooking level the skill checks: the
// current level, raised by content ("skills:level-boost", a
// league relic's invisible +12). No Go answerer exists, so the
// boost defaults 0 (fail-closed, documented at the emit site).
func (c *cooking) cookingLevel(player *game.Player) int {
	if player == nil || player.Skills() == nil {
		return 0
	}
	request := &LevelBoostRequest{Player: player, Skill: game.SkillCooking, Boost: 0}
	c.emitEvent(LevelBoostEventName, request)
	boost := request.Boost
	if boost < 0 {
		boost = 0
	}
	return player.Skills().CurrentLevel(game.SkillCooking) + boost
}

// actionsPerCycle is how many items one cycle makes
// ("skills:actions-per-cycle", a league relic's all at once); 1
// unless content says more. No Go answerer exists, so the
// default is 1 (fail-closed, documented at the emit site).
func (c *cooking) actionsPerCycle(player *game.Player, kind string) int {
	request := &ActionsPerCycleRequest{Player: player, Skill: game.SkillCooking, Kind: kind, Actions: 1}
	c.emitEvent(ActionsPerCycleEventName, request)
	if request.Actions < 1 {
		return 1
	}
	return request.Actions
}

// wearsCookingCape reports whether the player wears a Cooking
// cape (or max cape), which never burns food
// (Common.Cooking.js:135-139): the "skillcape:has-perk" query
// with the EXISTING game types (game.SkillcapeHasPerkEventName
// / game.SkillcapePerkRequest). No Go answerer exists, so
// Result stays false and the cape never prevents burns — the
// honest no-hooks default. Do NOT redefine those types.
func (c *cooking) wearsCookingCape(player *game.Player) bool {
	request := &game.SkillcapePerkRequest{Player: player, Skill: "COOKING", Result: false}
	c.emitEvent(game.SkillcapeHasPerkEventName, request)
	return request.Result == true
}

// ---- messages, varbits, articles (Common.Cooking.js:142-162) ----

// withArticle is "a shark", "an anglerfish", "some shrimps",
// "the giant krill" (Common.Cooking.js:142-144): an explicit
// article wins, else the vowel rule ("an" before a vowel).
func withArticle(name, article string) string {
	if article != "" {
		return article + " " + name
	}
	if len(name) > 0 && strings.ContainsRune("aeiouAEIOU", rune(name[0])) {
		return "an " + name
	}
	return "a " + name
}

// spam sends a SPAM-channel chat line (Common.Cooking.js:146-148).
func (c *cooking) spam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(spamChatType, text, "")
}

// mesbox opens a statement dialogue (Common.Cooking.js:150-153):
// a DialogueChainBuilder with a single StatementDialogue(0,
// text), the woodcutting precedent.
func (c *cooking) mesbox(player *game.Player, text string) {
	if player == nil {
		return
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, text)))
	}
}

// varbitSender is the session-side varbit send, reached through
// a type assertion on the player's Session (the fletching
// getVarbit/sendVarbit precedent in shared.go).
type varbitSender interface {
	SendVarbit(id, value int)
}

// sendVarbit sends a varbit by gameval name
// (Common.Cooking.js:157-161): the id is the loader-resolved
// one (Tables.VarbitIDs); an unresolvable name sends nothing,
// as TS skips it. A session-less player sends nowhere.
func (c *cooking) sendVarbit(player *game.Player, name string, value int) {
	if player == nil || player.Session == nil || c == nil || c.tables == nil {
		return
	}
	id, ok := c.tables.VarbitIDs[name]
	if !ok {
		return
	}
	if sender, ok := player.Session.(varbitSender); ok {
		sender.SendVarbit(id, value)
	}
}

// ---- knives, XP (Common.Cooking.js:164-175) ----

// knives returns every knife id the cache knows
// (Common.Cooking.js:164-167): the loader-resolved KNIFE_NAMES
// ids, missing names already filtered out.
func (c *cooking) knives() []int {
	if c == nil || c.tables == nil {
		return nil
	}
	return c.tables.KnifeIDs
}

// addXP grants Cooking XP (Common.Cooking.js:173-175): a no-op
// when xp <= 0. Fractional XP truncates at the grant site
// (int(xp)) — see the file header.
func (c *cooking) addXP(player *game.Player, xp float64) {
	if player == nil || xp <= 0 {
		return
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillCooking, int(xp))
	}
}

// ---- the keyed step runner (Common.Cooking.js:177-185) ----

// run runs step(tick) every tick, as a task keyed to the player
// so a walk or a click cancels it (Common.Cooking.js run). The
// task is a 1-tick task on the lazily-resolved TaskManager — the
// Woodcutting/Fletching pattern: NEVER capture the TaskManager
// at Register time, it installs after Load. step returns false
// to stop. The run stops when the player unregisters or dies,
// as TS does. A nil TaskManager schedules nothing (the caller
// retries on the next click, the fletching startFletching
// shape). The key is the player's index, matching TS
// TaskManager.cancelTasks(player.getIndex()).
func (c *cooking) run(player *game.Player, step func(tick int64) bool) {
	if player == nil || step == nil {
		return
	}
	tm := c.taskManager()
	if tm == nil {
		return
	}
	var tick int64
	task := game.NewTask(1, func(cycle int64) bool {
		if !player.IsRegistered() || hitpoints(player) <= 0 {
			return true
		}
		tick++
		return !step(tick)
	})
	task.Name = "CookingTask"
	task.Key = player.Index
	tm.Schedule(task)
}

// hitpoints reads the player's current Hitpoints, or 0 without
// a skill manager (the fletching task's death check,
// shared.go hitpoints).
func hitpoints(player *game.Player) int {
	if player == nil || player.Skills() == nil {
		return 0
	}
	return player.Skills().CurrentLevel(game.SkillHitpoints)
}

// ---- lookups ----

// itemID resolves a gameval item name the file uses, or false
// when the file never names it (Common.Cooking.js gameval("obj",
// name), which throws on unknown — Go call sites check ok
// instead; prepareGnomebowl is the caller).
func (c *cooking) itemID(name string) (int, bool) {
	if c == nil || c.tables == nil {
		return 0, false
	}
	return c.tables.ItemID(name)
}

// cookableFor returns the cookable row for a raw item id, or nil.
func (c *cooking) cookableFor(rawID int) *Cookable {
	if c == nil || c.tables == nil {
		return nil
	}
	return c.tables.CookableFor(rawID)
}

// ---- answerers (Cookers.Cooking.js answerRawXp / answerCooked) ----

// answerRawXP answers the "cooking:raw-xp" query: the Cooking XP
// for cooking request.RawID, e.g. for the infernal harpoon. This
// is the Phase 35 unblock: Fishing's tryCookFish
// (internal/content/fishing/infernalharpoon.go:195-215) emits
// &CookingRawXPRequest{Player, RawID} and reads HasXP/XP after.
// The answerer MUST type-assert to *fishing.CookingRawXPRequest
// — a second request type for this event would silently never
// match. A hit (the cookable exists and is not facility-only)
// sets XP (int-truncated, as the fishing request carries an
// int) and HasXP=true; a miss leaves the request unanswered.
// Facility foods (raw_guppy, raw_cavefish, raw_tetra,
// raw_catfish) never answer: Cookers must NOT cook them.
func (c *cooking) answerRawXP(payload any) {
	request, ok := payload.(*fishing.CookingRawXPRequest)
	if !ok || request == nil {
		return
	}
	cookable := c.cookableFor(request.RawID)
	if cookable == nil || cookable.Facility != "" {
		return
	}
	request.XP = int(cookable.XP)
	request.HasXP = true
}

// answerCooked answers the "cooking:cooked" query
// (Cookers.Cooking.js:650-656): what request.RawID cooks into
// and the XP, for content that cooks as it gathers (a league
// relic). A hit (the cookable exists and is not facility-only)
// sets CookedID + XP; a miss leaves the request unanswered. No
// Go emitter exists yet.
func (c *cooking) answerCooked(payload any) {
	request, ok := payload.(*CookedRequest)
	if !ok || request == nil {
		return
	}
	cookable := c.cookableFor(request.RawID)
	if cookable == nil || cookable.Facility != "" {
		return
	}
	request.CookedID = cookable.CookedID
	request.XP = cookable.XP
}

// ---- CreationMenu send (Phase 34 seam) ----

// sendCreationMenu opens a menu on the client through the
// session's CreationMenuSender (the fletching openMenu
// precedent, shared.go:1618-1623): a session without the sender
// opens nothing.
func (c *cooking) sendCreationMenu(player *game.Player, menu *game.CreationMenu) {
	if player == nil || menu == nil || player.Session == nil {
		return
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
}

// registerAnswerers wires the two query answerers Worker A owns.
// Called from Register (cooking.go); Worker B must NOT register
// them again.
func (c *cooking) registerAnswerers(api *plugins.Api) {
	if api == nil {
		return
	}
	api.OnCustomEvent(CookingRawXPEventName, c.answerRawXP)
	api.OnCustomEvent(CookingCookedEventName, c.answerCooked)
}
