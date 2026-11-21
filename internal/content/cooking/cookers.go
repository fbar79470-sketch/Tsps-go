// Cookers.Cooking.js (717 lines, commit bfd81e3): cooking on fires,
// ranges and ovens — the core loop: sessions, the ONE global 1-tick
// task, cookTick / cookStep / cookOne, the CreationMenu (mode 6),
// the gates, the Hosidius Kitchen bonus, gauntlets/cape, deliberate
// burns, karambwan, the success-count tracker, and the
// cooking:raw-xp / cooking:cooked answerers (the answerer FUNCTIONS
// live in common.go — Worker A owns them — and are REGISTERED once
// in register(), also by Worker A; this file must NOT register them
// again).
//
// The port is faithful: same timing (first 2, second 3, interval 4,
// karambwan re-click 1), same gate order in canCook, same cookOne
// order (deliberate burn → selected revalidation → gnome slot logic
// → return-item room check → burn emit → success /
// fails-without-burning / burn), same menu titles and mode, same
// success-count varbit behaviour. Deviations forced by missing Go
// seams are named inline; nothing is silently invented.
//
// Event inventory:
//
//   - Listens to onItemOnObject (handleCook, noted:false; refuseCook
//     as fallback:true — the hub runs main hooks first, then
//     fallback hooks, each wrapper skipping once Handled, so the
//     fallback runs only when nothing claimed the use:
//     internal/plugins/objects.go EmitItemOnObject, the same
//     claim rule Phase 33 documented for onItemOnItem).
//   - Listens to onObjectInteractionNamed "Cook" on the 11 cooker
//     names (handleCookClick).
//   - Listens to onPlayerLogin (success-count resend) and
//     onPlayerDisconnect (stop session, no animation reset).
//   - Listens to "level-up:popup-opened" via
//     skillactions.PauseForLevelUp; the emitter (SkillLevelUp
//     feedback content) is NOT STARTED, so the pause never fires
//     yet. The resume callback is nil: TS Cookers passes no
//     onResume (pauseForLevelUp's is optional), so resuming only
//     clears the flag.
//   - Listens to "cooking:count" (countCookEvent).
//   - Emits "skills:level-boost" (default boost 0),
//     "skills:actions-per-cycle" (default 1),
//     "skills:action-interval" (via skillactions),
//     "diary:is-complete" (default complete false — the Hosidius
//     bonus is 12, never 25), "quest:is-complete" /
//     "quest:is-started" (default false — FAIL CLOSED: the
//     Lumbridge Castle range stays refused, a duringQuest food
//     never cooks), "hunter:rumours-completed" (default 0 —
//     FAIL CLOSED: a rumours food never cooks),
//     "skillcape:has-perk" (default false — the cape never
//     prevents burns), "cooking:burn" (a handler may flip Burn;
//     no Go consumers), "cooking:burnt", "cooking:success",
//     "items:obtained", "skills:produced" (kind "cook"; no Go
//     consumers for any of the last four).
//
// State lives in a *cooking struct — NO package-level mutable
// maps (the Phase 21 race fix): sessions live on the instance,
// and the global task's cycle counter with them. Tests build
// their own with newCooking.
package cooking

import (
	"fmt"
	"math"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- constants (Cookers.Cooking.js:13-33) ----

const (
	// firstCookTicks is FIRST_COOK_TICKS: ticks from the choice
	// (or the arrival) to the first food.
	firstCookTicks = 2
	// secondCookTicks is SECOND_COOK_TICKS: ticks from the first
	// food to the second.
	secondCookTicks = 3
	// cookIntervalTicks is COOK_INTERVAL_TICKS: ticks between
	// later foods.
	cookIntervalTicks = 4
	// karambwanReclickTicks is KARAMBWAN_RECLICK_TICKS: ticks
	// from a manual re-use of raw karambwan while it cooks to
	// the next food.
	karambwanReclickTicks = 1
	// cookMenuMode is COOK_MENU_MODE: clientscript
	// skillmulti_setup's first argument on every captured
	// cooking menu.
	cookMenuMode = 6
	// highestStopLevel is HIGHEST_STOP_LEVEL: levels the
	// stop-burn search tries; past this a food is treated as
	// never stopping (the MaxInt sentinel in stopBurnLevel).
	highestStopLevel = 200
)

// Menu titles (Cookers.Cooking.js:25-26).
const (
	oneTypeTitle   = "How many would you like to cook?"
	manyTypesTitle = "What would you like to cook?"
)

// Messages (Cookers.Cooking.js:27-31).
const (
	nothingToCookMessage  = "You haven't got anything to cook."
	cannotCookMessage     = "You can't cook that."
	notUsefulMessage      = "You don't feel that would be useful now."
	presetItemMessage     = "Preset items cannot be used for this."
	lumbridgeRangeMessage = "You need to complete Cook's Assistant to use this range."
)

// Varbit / attribute names (Cookers.Cooking.js:32-36).
const (
	successCountVarbit    = "cooking_success_count"
	successCountAttribute = "cooking:success-count"
	busyVarbit            = "busy"
)

// Attribute keys (Cookers.Cooking.js:71-72).
const (
	karambwanTaughtAttribute = "quest.tai_bwo_wannai_trio.tinsay"
	karambwanTaughtValue     = 8
)

// cookingRangeObjectID is ObjectIdentifiers.COOKING_RANGE
// (server/src/main/typescript/elvarg/util/ObjectIdentifiers.ts:
// 102: COOKING_RANGE = 114, "Cooking range"): TS compares the
// object id against this numeric constant, so the Go port does
// the same — it is the pinned upstream identifier, not a
// guessed id. The load-prove step must confirm the cache agrees
// (defs.GetObject(114).Name == "Cooking range").
const cookingRangeObjectID = 114

// clayOven3ObjectID is ObjectIdentifiers.CLAY_OVEN_3
// (ObjectIdentifiers.ts:11662: CLAY_OVEN_3 = 21302, "Clay
// oven"): the Hosidius Kitchen oven. Same grounding as
// cookingRangeObjectID above; the load-prove step must confirm
// the cache agrees (defs.GetObject(21302).Name == "Clay oven").
// It cannot be resolved by gameval name: three locs share the
// "Clay oven" display name, and TS itself uses the numeric id.
const clayOven3ObjectID = 21302

// Hosidius Kitchen (Cookers.Cooking.js:137-139): rect x
// 1674–1684, y 3616–3626, z 0; bonus 12, or 25 with the elite
// Kourend & Kebos Diary. The diary query has NO Go answerer,
// so the bonus is 12; never 25 — documented, not invented.
const (
	hosidiusMinX       = 1674
	hosidiusMaxX       = 1684
	hosidiusMinY       = 3616
	hosidiusMaxY       = 3626
	hosidiusBonus      = 12
	hosidiusEliteBonus = 25
)

// FireObjectNames is FIRE_OBJECT_NAMES (Cookers.Cooking.js:
// 122-123): the fires (and house firepits) that cook with
// human_firecooking.
var FireObjectNames = map[string]bool{
	"Fire": true, "Forester's Campfire": true, "Firepit": true,
	"Firepit with hook": true, "Firepit with pot": true,
}

// houseRangeNames is HOUSE_RANGE_NAMES (Cookers.Cooking.js:
// 124): a house's ovens and ranges, which also cook with
// human_firecooking (rsprox capture 269).
var houseRangeNames = map[string]bool{
	"Small oven": true, "Large oven": true, "Steel range": true,
	"Fancy range": true,
}

// rangeObjectNames is RANGE_OBJECT_NAMES (Cookers.Cooking.js:
// 127): the ranges and ovens that cook with human_cooking.
// The cache spells the clay ovens two ways.
var rangeObjectNames = map[string]bool{
	"Cooking range": true, "Range": true, "Stove": true,
	"Oven": true, "Clay oven": true, "Clay Oven": true,
}

// CookableObjectNames is COOKABLE_OBJECT_NAMES (Cookers
// .Cooking.js:128): the union of the fire, house-range and
// range names.
var CookableObjectNames = func() map[string]bool {
	out := make(map[string]bool, len(FireObjectNames)+len(houseRangeNames)+len(rangeObjectNames))
	for name := range FireObjectNames {
		out[name] = true
	}
	for name := range houseRangeNames {
		out[name] = true
	}
	for name := range rangeObjectNames {
		out[name] = true
	}
	return out
}()

// cookClickObjectNames are the 11 object names handleCookClick
// registers for (Cookers.Cooking.js attachCookers), in TS
// order.
var cookClickObjectNames = []string{
	"Fire", "Range", "Cooking range", "Stove", "Oven",
	"Clay oven", "Clay Oven", "Small oven", "Large oven",
	"Steel range", "Fancy range",
}

// ---- per-instance state ----

// selectedInput is the gnome-tutorial selected input
// (Cookers.Cooking.js startCooking's `selected`): the inventory
// item and the slot it was used from. The item is a value copy
// of the click-time stack; validSelectedInput re-checks the
// slot still holds an equal item (Go Items are values, so
// Equals — id, amount, meta — is the reference-equality
// equivalent).
type selectedInput struct {
	slot int
	item game.Item
}

// cookSession is one player's cooking session
// (Cookers.Cooking.js activeSessions entries). It implements
// skillactions.Session, so PauseForLevelUp can hold it; it must
// stay a pointer (comparable) value for the helper's identity
// checks. remaining counts down from the chosen amount;
// math.MaxInt is the TS Infinity default (cook everything).
type cookSession struct {
	cookable   *Cookable
	deliberate bool
	selected   *selectedInput
	alternates []*Cookable
	animation  int
	cooker     string // "fire" or "range"
	objectID   int
	objectType int
	objectFace int
	// sizeX/sizeY are the cooker's footprint, resolved at
	// start (1x1 when unknown — the woodcutting precedent for
	// the interaction-distance check).
	sizeX, sizeY  int
	location      game.Location
	privateArea   *game.PrivateArea
	nextCookTick  int64
	cooked        int
	remaining     int
	levelUpPaused bool
}

// SetLevelUpPaused marks the session held by a level-up pop-up
// (skillactions.Session).
func (s *cookSession) SetLevelUpPaused(paused bool) {
	if s != nil {
		s.levelUpPaused = paused
	}
}

// ---- small helpers ----

// restricted reports whether an item copy keeps restrictions
// that bar it from cooking (restricted, Shared.Fletching.js
// pattern; Cookers.Cooking.js validSelectedInput): preset,
// untradeable, or unbankable.
func restricted(item game.Item) bool {
	return item.IsPresetItem() || item.IsUntradeable() || item.IsUnbankable()
}

// attrInt reads a numeric player attribute as an int (TS
// Number(attr) || 0): ints, int64s and floats all read; anything
// else reads 0.
func attrInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case float32:
		return int(v)
	default:
		return 0
	}
}

// itemDefs returns the player's item-definition source, or nil.
func itemDefs(player *game.Player) game.ItemDefinitionSource {
	if player == nil {
		return nil
	}
	return player.ItemDefinitions()
}

// stackableOf reports whether an item stacks, or false without
// a definition source (the nil-source default).
func stackableOf(player *game.Player, id int) bool {
	if src := itemDefs(player); src != nil {
		return src.IsStackable(id)
	}
	return false
}

// notedOf reports whether an item is a bank note, or false
// without a definition source.
func notedOf(player *game.Player, id int) bool {
	if src := itemDefs(player); src != nil {
		return src.IsNoted(id)
	}
	return false
}

// objectManager resolves the ObjectManagerAPI lazily, the same
// contract as taskManager: the hub installs it after plugin
// Load (cmd/server, from the built world's registry). The live
// Api value wins when present; the directly-set objects field
// is the unit-test override.
func (c *cooking) objectManager() plugins.ObjectManagerAPI {
	if c == nil {
		return nil
	}
	if c.api != nil {
		if m := c.api.GetObjectManager(); m != nil {
			return m
		}
	}
	return c.objects
}

// isMembersItemOf answers an item definition's members flag;
// nil means every item counts as non-members (the woodcutting
// SetIsMembersItem precedent: the TS no-world fixture path).
func (c *cooking) isMembersItemOf(id int) bool {
	if c == nil || c.isMembersItem == nil {
		return false
	}
	return c.isMembersItem(id)
}

// objectInfoOf resolves an object's display info, or false
// when no info func is installed or the id is unknown.
func (c *cooking) objectInfoOf(id int) (ObjectInfo, bool) {
	if c == nil || c.objectInfo == nil {
		return ObjectInfo{}, false
	}
	return c.objectInfo(id)
}

// objectNameOf resolves an object's display name, or "" when
// unknown.
func (c *cooking) objectNameOf(id int) string {
	if info, ok := c.objectInfoOf(id); ok {
		return info.Name
	}
	return ""
}

// onRange reports whether an object id is a range or oven
// rather than a fire, by its cache name (Cookers.Cooking.js
// onRange); false without a name.
func (c *cooking) onRange(objectID int) bool {
	name := c.objectNameOf(objectID)
	return name != "" && !FireObjectNames[name]
}

// cookerKind is the cooker kind a session gives; without one,
// the object's ("range" when there is no object either)
// (Cookers.Cooking.js cookerKind). The cache-existence leg
// reads through the installed object info.
func (c *cooking) cookerKind(cooker string, objectID int) string {
	if cooker != "" {
		return cooker
	}
	if objectID != 0 && !c.onRange(objectID) {
		if _, ok := c.objectInfoOf(objectID); ok {
			return "fire"
		}
	}
	return "range"
}

// ---- Cookers-resolved ids ----

// rawKarambwanID is ItemIds.RAW_KARAMBWAN: the JSON names the
// karambwan row "tbwt_raw_karambwan", so the loader-resolved
// ItemIDs map carries it.
func (c *cooking) rawKarambwanID() int {
	id, _ := c.itemID("tbwt_raw_karambwan")
	return id
}

// cookedMeatID / burntMeatID / cookedChickenID /
// burntChickenID are the DELIBERATE_BURN ids
// (Cookers.Cooking.js deliberateBurnFor): the JSON names them,
// so the loader-resolved ItemIDs map carries them.
func (c *cooking) cookedMeatID() int {
	id, _ := c.itemID("cooked_meat")
	return id
}

func (c *cooking) burntMeatID() int {
	id, _ := c.itemID("burnt_meat")
	return id
}

func (c *cooking) cookedChickenID() int {
	id, _ := c.itemID("cooked_chicken")
	return id
}

func (c *cooking) burntChickenID() int {
	id, _ := c.itemID("burnt_chicken")
	return id
}

// ---- deliberate burn (Cookers.Cooking.js:40-54) ----

// deliberateBurnFor maps cooked meat/chicken to their
// deliberate-burn cookable: cooked meat and chicken can be
// burnt on purpose (Wiki: Burnt chicken). The returned
// cookable has no chance chart — cookOne handles it before
// any roll — and the second result marks it deliberate (the
// Cookable type has no deliberate field; the flag travels
// with the session, see startCooking).
func (c *cooking) deliberateBurnFor(itemID int) (*Cookable, bool) {
	if itemID == c.cookedMeatID() && itemID != 0 {
		return &Cookable{
			RawID: itemID, CookedID: c.burntMeatID(), BurntID: c.burntMeatID(),
			Level: 1, XP: 0, StopBurn: 1, HasStopBurn: true,
			Name: "meat", Message: "You deliberately burn the nicely cooked meat.",
		}, true
	}
	if itemID == c.cookedChickenID() && itemID != 0 {
		return &Cookable{
			RawID: itemID, CookedID: c.burntChickenID(), BurntID: c.burntChickenID(),
			Level: 1, XP: 0, StopBurn: 1, HasStopBurn: true,
			Name: "chicken", Message: "You deliberately burn the nicely cooked chicken.",
		}, true
	}
	return nil, false
}

// ---- availability, karambwan (Cookers.Cooking.js:56-86) ----

// cookableAvailableOnWorld reports whether a cookable may be
// used on this world (Cookers.Cooking.js availableOnWorld): a
// members world allows everything; otherwise the raw item must
// not be members and the cookable must not be gnome-tutorial.
// It is named cookableAvailableOnWorld because recipes.go
// already owns availableOnWorld for *Recipe.
func (c *cooking) cookableAvailableOnWorld(cookable *Cookable) bool {
	if cookable == nil {
		return false
	}
	if c != nil && c.membersWorld {
		return true
	}
	return !cookable.GnomeTutorial && !c.isMembersItemOf(cookable.RawID)
}

// cookableForUse returns the cookable for an item id, or nil:
// the table row or a deliberate burn, excluding facility
// foods and foods unavailable on this world (Cookers.Cooking.js
// cookableFor). It is named cookableForUse because common.go
// already owns the table-only lookup under the TS name
// cookableFor.
func (c *cooking) cookableForUse(itemID int) (*Cookable, bool) {
	cookable, deliberate := c.deliberateBurnFor(itemID)
	if cookable == nil {
		cookable = c.cookableFor(itemID)
	}
	if cookable == nil || cookable.Facility != "" || !c.cookableAvailableOnWorld(cookable) {
		return nil, false
	}
	return cookable, deliberate
}

// untrainedKarambwan reports whether the player cooks a raw
// karambwan without Tinsay's training (Cookers.Cooking.js
// untrainedKarambwan): the "quest.tai_bwo_wannai_trio.tinsay"
// attribute below 8.
func (c *cooking) untrainedKarambwan(player *game.Player, cookable *Cookable) bool {
	if cookable == nil || cookable.RawID != c.rawKarambwanID() || cookable.RawID == 0 {
		return false
	}
	if player == nil {
		return false
	}
	return attrInt(player.GetAttribute(karambwanTaughtAttribute)) < karambwanTaughtValue
}

// cookedResult is what a cookable cooks into for this player
// (Cookers.Cooking.js cookedResult): an untrained raw karambwan
// cooks into a poison karambwan, xp 80, with its own message.
// The returned cookable is a copy; the table row is never
// mutated.
func (c *cooking) cookedResult(player *game.Player, cookable *Cookable) *Cookable {
	if cookable == nil || !c.untrainedKarambwan(player, cookable) {
		return cookable
	}
	out := *cookable
	out.CookedID = c.cookerIDs.PoisonKarambwanID
	out.XP = 80
	out.Message = "You cook the karambwan. It looks very dodgy."
	return &out
}

// withArticleOf is Common.withArticle for a cookable
// (Cookers.Cooking.js withArticle).
func withArticleOf(cookable *Cookable) string {
	if cookable == nil {
		return ""
	}
	return withArticle(cookable.Name, cookable.Article)
}

// successMessage is the cooked line (Cookers.Cooking.js
// successMessage): the row's message, else the default.
func successMessage(cookable *Cookable) string {
	if cookable == nil {
		return ""
	}
	if cookable.Message != "" {
		return cookable.Message
	}
	return "You successfully cook " + withArticleOf(cookable) + "."
}

// burnMessage is the burnt line (Cookers.Cooking.js
// burnMessage): the row's burn message, else the default.
func burnMessage(cookable *Cookable) string {
	if cookable == nil {
		return ""
	}
	if cookable.BurnMessage != "" {
		return cookable.BurnMessage
	}
	return "You accidentally burn the " + cookable.Name + "."
}

// cookAnimationFor returns the cook animation seq id for an
// object name (Cookers.Cooking.js cookAnimationFor): fires and
// a house's firepits, ovens and ranges play human_firecooking,
// every other range and oven plays human_cooking.
func (c *cooking) cookAnimationFor(name string) int {
	if FireObjectNames[name] || houseRangeNames[name] {
		return c.cookerIDs.FireCookingSeqID
	}
	return c.cookerIDs.CookingSeqID
}

// ---- Hosidius, gauntlets, chance (Cookers.Cooking.js:141-230) ----

// wearsCookingGauntlets reports whether the player wears
// cooking gauntlets in the hands slot (Cookers.Cooking.js
// wearsCookingGauntlets).
func (c *cooking) wearsCookingGauntlets(player *game.Player) bool {
	if player == nil || player.Equipment() == nil {
		return false
	}
	return player.Equipment().ItemAt(game.HandsSlot).ID() == c.cookerIDs.CookingGauntletsID &&
		c.cookerIDs.CookingGauntletsID != 0
}

// isHosidiusKitchen reports whether an object is the Hosidius
// Kitchen's clay oven (Cookers.Cooking.js isHosidiusKitchen):
// CLAY_OVEN_3 on z 0 inside the 1674–1684 × 3616–3626 rect.
// The mess's oven next door is a different object id and gets
// no bonus.
func isHosidiusKitchen(objectID int, location game.Location) bool {
	if objectID != clayOven3ObjectID {
		return false
	}
	return location.Z == 0 && location.X >= hosidiusMinX && location.X <= hosidiusMaxX &&
		location.Y >= hosidiusMinY && location.Y <= hosidiusMaxY
}

// hosidiusBonus is the Hosidius Kitchen bonus (Cookers.Cooking
// .js hosidiusBonus): 12, or 25 with the elite Kourend & Kebos
// Diary. The "diary:is-complete" query has NO Go answerer, so
// Complete stays false and the bonus is 12 — documented, not
// invented (see the file header).
func (c *cooking) hosidiusBonus(player *game.Player, objectID int, location game.Location) int {
	if !isHosidiusKitchen(objectID, location) {
		return 0
	}
	request := &DiaryCompleteRequest{Player: player, Diary: "kourend", Tier: "elite", Complete: false}
	c.emitEvent(DiaryCompleteEventName, request)
	if request.Complete {
		return hosidiusEliteBonus
	}
	return hosidiusBonus
}

// chanceValues is the [low, high] values a food cooks with
// here (Cookers.Cooking.js chanceValues): worn cooking
// gauntlets for the foods they help, the Lumbridge Castle
// range for the foods it helps, else the fire's or the range's,
// plus the Hosidius Kitchen's bonus (ranges only). Nil for a
// food without Wiki values.
func (c *cooking) chanceValues(player *game.Player, cookable *Cookable, objectID int, cooker string, location game.Location) *[2]int {
	if cookable == nil || cookable.Chance == nil {
		return nil
	}
	chance := cookable.Chance
	kind := c.cookerKind(cooker, objectID)
	var values [2]int
	ok := false
	if gv, has := chance["gauntlets"]; has && c.wearsCookingGauntlets(player) {
		values, ok = gv, true
	} else if lv, has := chance["lumbridge"]; has && objectID == cookingRangeObjectID {
		values, ok = lv, true
	} else if kind == "fire" {
		if v, has := chance["fire"]; has {
			values, ok = v, true
		} else if v, has := chance["range"]; has {
			values, ok = v, true
		}
	} else {
		if v, has := chance["range"]; has {
			values, ok = v, true
		} else if v, has := chance["fire"]; has {
			values, ok = v, true
		}
	}
	if !ok {
		return nil
	}
	bonus := 0
	if kind != "fire" {
		bonus = c.hosidiusBonus(player, objectID, location)
	}
	return &[2]int{values[0] + bonus, values[1] + bonus}
}

// stopBurnLevel is the level a food stops burning at here
// (Cookers.Cooking.js stopBurnLevel): the first level 1..200
// whose success chance reaches 1; without Wiki values, the
// row's stopBurn; math.MaxInt when never (the TS Infinity —
// the fallback-line arithmetic in successChanceFor reads the
// same with MaxInt as with Infinity).
func (c *cooking) stopBurnLevel(player *game.Player, cookable *Cookable, objectID int, cooker string, location game.Location) int {
	if cookable == nil {
		return math.MaxInt
	}
	values := c.chanceValues(player, cookable, objectID, cooker, location)
	if values == nil {
		return cookable.StopBurn
	}
	for level := 1; level <= highestStopLevel; level++ {
		if successChance(values[0], values[1], level) >= 1 {
			return level
		}
	}
	return math.MaxInt
}

// successChanceFor is the chance of cooking a food here at the
// player's Cooking level (Cookers.Cooking.js successChance):
// the cooking cape → 1; else the Wiki values via the interp
// formula; else the fallback line — 58% at the food's level
// rising to 100% at its stopBurn.
func (c *cooking) successChanceFor(player *game.Player, cookable *Cookable, objectID int, cooker string, location game.Location) float64 {
	if cookable == nil {
		return 0
	}
	if c.wearsCookingCape(player) {
		return 1
	}
	level := c.cookingLevel(player)
	values := c.chanceValues(player, cookable, objectID, cooker, location)
	if values != nil {
		return successChance(values[0], values[1], level)
	}
	stopBurn := c.stopBurnLevel(player, cookable, objectID, cooker, location)
	if level >= stopBurn || stopBurn <= cookable.Level {
		return 1
	}
	burnChance := 42 - float64(level-cookable.Level)*42/float64(stopBurn-cookable.Level)
	return 1 - burnChance/100
}

// isSuccess is the roll (Cookers.Cooking.js isSuccess): a roll
// at or above the burn chance cooks the food (a roll of 0 burns
// anything that can burn).
func (c *cooking) isSuccess(player *game.Player, cookable *Cookable, objectID int, cooker string, location game.Location) bool {
	return c.randFloat() >= 1-c.successChanceFor(player, cookable, objectID, cooker, location)
}

// ---- sessions: stop / count / login (Cookers.Cooking.js:232-262) ----

// stopCooking stops the player's cooking session (Cookers
// .Cooking.js stopCooking): delete it; if nothing cooked yet,
// clear the busy varbit; reset the animation when asked.
func (c *cooking) stopCooking(player *game.Player, resetAnimation bool) {
	if c == nil || player == nil {
		return
	}
	session, ok := c.sessions[player]
	if !ok || session == nil {
		return
	}
	delete(c.sessions, player)
	if session.cooked == 0 {
		c.sendVarbit(player, busyVarbit, 0)
	}
	if resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// countCook moves the cooking_success_count (Cookers.Cooking.js
// countCook): up one for a cooked food, back to 0 on a burn
// (rsprox captures 1098, 1102, 4103).
func (c *cooking) countCook(player *game.Player, burnt bool) {
	if player == nil {
		return
	}
	count := 0
	if !burnt {
		count = attrInt(player.GetAttribute(successCountAttribute)) + 1
	}
	player.SetAttribute(successCountAttribute, count)
	c.sendVarbit(player, successCountVarbit, count)
}

// countCookEvent answers the "cooking:count" query (Cookers
// .Cooking.js countCookEvent): a cook made elsewhere (the
// Gauntlet's range) moves the same count.
func (c *cooking) countCookEvent(payload any) {
	event, ok := payload.(*CountEvent)
	if !ok || event == nil || event.Player == nil {
		return
	}
	c.countCook(event.Player, event.Burnt)
}

// sendSuccessCount resends the success count on login, only
// when it is above 0 (Cookers.Cooking.js sendSuccessCount).
func (c *cooking) sendSuccessCount(event *plugins.PlayerLoginEvent) {
	if event == nil || event.Player == nil {
		return
	}
	count := attrInt(event.Player.GetAttribute(successCountAttribute))
	if count > 0 {
		c.sendVarbit(event.Player, successCountVarbit, count)
	}
}

// isCookingActive reports whether the player has a cooking
// session (Cookers.Cooking.js isCookingActive).
func (c *cooking) isCookingActive(player *game.Player) bool {
	if c == nil || player == nil {
		return false
	}
	_, ok := c.sessions[player]
	return ok
}

// IsCookingActive is the package-level form, on the shared
// instance (Cooking.plugin.js isCookingActive export).
func IsCookingActive(player *game.Player) bool {
	return shared.isCookingActive(player)
}

// ---- gates (Cookers.Cooking.js:264-330) ----

// canCook reports whether the player may cook a food here, and
// says why not (Cookers.Cooking.js canCook) — gate order, all
// with messages: members; gnome-tutorial on a fire; the
// Lumbridge Castle range (quest:is-complete cooks_assistant —
// NO Go answerer, so the range is refused fail-closed);
// level (untrained karambwan → 1); duringQuest; rumours.
func (c *cooking) canCook(player *game.Player, objectID int, objectName string, cookable *Cookable) bool {
	if cookable == nil || player == nil {
		return false
	}
	if !c.cookableAvailableOnWorld(cookable) {
		player.SendMessage(FreeToPlayItemMessage)
		return false
	}
	if cookable.GnomeTutorial && FireObjectNames[objectName] {
		c.mesbox(player, cannotCookMessage)
		return false
	}
	if objectID == cookingRangeObjectID {
		request := &QuestStateRequest{Player: player, Key: "cooks_assistant", Complete: false}
		c.emitEvent(QuestStateEventName, request)
		if !request.Complete {
			player.SendMessage(lumbridgeRangeMessage)
			return false
		}
	}
	requiredLevel := cookable.Level
	if c.untrainedKarambwan(player, cookable) {
		requiredLevel = 1
	}
	if c.cookingLevel(player) < requiredLevel {
		c.mesbox(player, fmt.Sprintf("You need a Cooking level of %d to cook %s.", requiredLevel, withArticleOf(cookable)))
		return false
	}
	if !c.duringQuestMet(player, cookable) {
		// Wiki (Karambwanji): raw karambwanji cook only during
		// Tai Bwo Wannai Trio.
		player.SendMessage(notUsefulMessage)
		return false
	}
	if !c.rumoursMet(player, cookable) {
		// Wiki (Hunter meat): the better meats need Hunters'
		// Rumours; the line is ours, the Wiki has none.
		c.mesbox(player, fmt.Sprintf("You need to have completed %d Hunters' Rumours to cook %s.", cookable.Rumours, withArticleOf(cookable)))
		return false
	}
	return true
}

// duringQuestMet is the Wiki's quest gate (Cookers.Cooking.js
// duringQuestMet): a duringQuest food cooks only while that
// quest is under way.
func (c *cooking) duringQuestMet(player *game.Player, cookable *Cookable) bool {
	if cookable == nil || cookable.DuringQuest == "" {
		return true
	}
	return c.questInProgress(player, cookable.DuringQuest)
}

// rumoursMet is the Wiki's Hunters' Rumours gate (Cookers
// .Cooking.js rumoursMet): a rumours food needs that many
// rumours completed.
func (c *cooking) rumoursMet(player *game.Player, cookable *Cookable) bool {
	if cookable == nil || !cookable.HasRumours {
		return true
	}
	return c.rumoursCompleted(player) >= cookable.Rumours
}

// canCookFood reports whether the player may cook a food at
// all, without messages (Cookers.Cooking.js canCookFood): not
// a facility food, a members' world, the Cooking level, and
// the quest and rumour gates. Bots pick their food with it;
// canCook adds the cooker's own checks and tells a player why
// not.
func (c *cooking) canCookFood(player *game.Player, cookable *Cookable) bool {
	if cookable == nil || player == nil {
		return false
	}
	requiredLevel := cookable.Level
	if c.untrainedKarambwan(player, cookable) {
		requiredLevel = 1
	}
	return cookable.Facility == "" && c.cookableAvailableOnWorld(cookable) &&
		c.cookingLevel(player) >= requiredLevel &&
		c.duringQuestMet(player, cookable) && c.rumoursMet(player, cookable)
}

// CanCookFood is the package-level form, on the shared
// instance (Cooking.plugin.js canCookFood export).
func CanCookFood(player *game.Player, cookable *Cookable) bool {
	return shared.canCookFood(player, cookable)
}

// questInProgress reports whether a quest is started and not
// yet complete (Cookers.Cooking.js questInProgress): both
// queries unanswered in Go, so it reads false — a duringQuest
// food never cooks (fail-closed, documented).
func (c *cooking) questInProgress(player *game.Player, key string) bool {
	started := &QuestStateRequest{Player: player, Key: key, Started: false}
	c.emitEvent(QuestStartedEventName, started)
	complete := &QuestStateRequest{Player: player, Key: key, Complete: false}
	c.emitEvent(QuestStateEventName, complete)
	return started.Started && !complete.Complete
}

// rumoursCompleted is how many Hunters' Rumours the player has
// finished (Cookers.Cooking.js rumoursCompleted): the
// "hunter:rumours-completed" query, unanswered in Go → 0
// (fail-closed, documented).
func (c *cooking) rumoursCompleted(player *game.Player) int {
	request := &RumoursCompletedRequest{Player: player, Completed: 0}
	c.emitEvent(RumoursCompletedEventName, request)
	if request.Completed < 0 {
		return 0
	}
	return request.Completed
}

// ---- start / offer / menu (Cookers.Cooking.js:332-430) ----

// validSelectedInput reports whether the gnome-tutorial
// selected input still holds (Cookers.Cooking.js
// validSelectedInput): an integer slot, the slot still holds
// the same item, the id is the raw, the amount is 1, and the
// item is not preset / untradeable / unbankable.
func validSelectedInput(player *game.Player, cookable *Cookable, selected *selectedInput) bool {
	if selected == nil || cookable == nil || player == nil || player.Inventory() == nil {
		return false
	}
	items := player.Inventory().Items()
	if selected.slot < 0 || selected.slot >= len(items) {
		return false
	}
	item := items[selected.slot]
	return item.Equals(selected.item) && item.ID() == cookable.RawID &&
		item.Amount() == 1 && !restricted(item)
}

// startCooking starts cooking amount of a food (Cookers.Cooking
// .js startCooking): canCook, the selected-input check, the
// inventory check, then the session. Re-using raw karambwan
// while it cooks (the Wiki's 1-tick method) makes the next one
// due KARAMBWAN_RECLICK_TICKS later instead of restarting the
// first-food delay — and when nothing cooked yet, the previous
// nextCookTick is kept, so repeated clicks never shorten the
// captured first-food delay. amount is math.MaxInt for "all".
func (c *cooking) startCooking(player *game.Player, object plugins.GameObject, cookable *Cookable, amount int, selected *selectedInput, alternates []*Cookable, deliberate bool) bool {
	if player == nil || object == nil || cookable == nil {
		return false
	}
	objectName := c.objectNameOf(object.ID())
	if !c.canCook(player, object.ID(), objectName, cookable) {
		return false
	}
	if selected != nil && !validSelectedInput(player, cookable, selected) {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil || !inventory.Contains(cookable.RawID) {
		return false
	}
	previous, hasPrevious := c.sessions[player]
	reusing := hasPrevious && previous != nil && previous.cookable != nil &&
		previous.cookable.RawID == cookable.RawID && cookable.RawID == c.rawKarambwanID() && cookable.RawID != 0
	c.stopCooking(player, false)
	c.ensureCookTask()
	firstTicks := firstCookTicks
	if cookable.HasFirstCookTicks {
		firstTicks = cookable.FirstCookTicks
	}
	nextCookTick := c.cookCycle + int64(firstTicks)
	if reusing {
		if previous.cooked == 0 {
			nextCookTick = previous.nextCookTick
		} else {
			nextCookTick = c.cookCycle + int64(karambwanReclickTicks)
		}
	}
	sizeX, sizeY := 1, 1
	if info, ok := c.objectInfoOf(object.ID()); ok {
		sizeX, sizeY = info.SizeX, info.SizeY
	}
	if sizeX < 1 {
		sizeX = 1
	}
	if sizeY < 1 {
		sizeY = 1
	}
	cooked := 0
	if reusing {
		cooked = previous.cooked
	}
	c.sessions[player] = &cookSession{
		cookable: cookable, deliberate: deliberate, selected: selected, alternates: alternates,
		animation: c.cookAnimationFor(objectName),
		cooker:    cookerKindForName(objectName),
		objectID:  object.ID(), objectType: object.Type(), objectFace: object.Face(),
		sizeX: sizeX, sizeY: sizeY,
		location: object.Location(), privateArea: privateAreaOf(object),
		nextCookTick: nextCookTick, cooked: cooked, remaining: amount,
	}
	if !reusing || previous.cooked == 0 {
		c.sendVarbit(player, busyVarbit, 1)
	}
	return true
}

// cookerKindForName is the session's cooker kind from the
// object's name: a fire name cooks on a fire, anything else on
// a range (Cookers.Cooking.js startCooking's cooker field).
func cookerKindForName(name string) string {
	if FireObjectNames[name] {
		return "fire"
	}
	return "range"
}

// carriedCookables is the raw foods in the inventory this
// cooker takes, once each, in inventory order (Cookers.Cooking
// .js carriedCookables): skip noted items, facility foods,
// foods unavailable on this world, and rangeOnly foods on a
// fire; dedup by raw id.
func (c *cooking) carriedCookables(player *game.Player, onFire bool) []*Cookable {
	if player == nil || player.Inventory() == nil {
		return nil
	}
	found := make(map[int]bool)
	var out []*Cookable
	for _, item := range player.Inventory().Items() {
		if item.IsEmpty() || notedOf(player, item.ID()) {
			continue
		}
		cookable := c.cookableFor(item.ID())
		if cookable == nil || cookable.Facility != "" || !c.cookableAvailableOnWorld(cookable) {
			continue
		}
		if cookable.RangeOnly && onFire {
			continue
		}
		if found[cookable.RawID] {
			continue
		}
		found[cookable.RawID] = true
		out = append(out, cookable)
	}
	return out
}

// offerCook opens the cook menu, or cooks without one (Cookers
// .Cooking.js offerCook): one food is "How many would you like
// to cook?" showing the raw item; several are "What would you
// like to cook?" showing what they cook into, grouped by cooked
// product (a karambwan groups by poison-vs-cooked). One raw
// item on its own cooks without a menu; left-clicking a range
// with one raw item cooks without delay (2-tick cooking:
// cookStep at once, then stopCooking).
func (c *cooking) offerCook(player *game.Player, object plugins.GameObject, cookables []*Cookable, singleClick bool, selected *selectedInput) {
	if player == nil || object == nil {
		return
	}
	inventory := player.Inventory()
	if len(cookables) == 1 {
		cookable := cookables[0]
		_, deliberate := c.cookableForUse(cookable.RawID)
		if !c.canCook(player, object.ID(), c.objectNameOf(object.ID()), cookable) {
			return
		}
		carried := 0
		if inventory != nil {
			carried = inventory.GetAmount(cookable.RawID)
		}
		if carried <= 1 {
			if c.startCooking(player, object, cookable, 1, selected, nil, deliberate) &&
				singleClick && !cookable.HasFirstCookTicks {
				// Wiki Cooking#2-tick_cooking: left-clicking
				// a range with one raw item cooks without
				// delay.
				if session := c.sessions[player]; session != nil {
					c.cookStep(player, session, c.cookCycle)
				}
				c.stopCooking(player, false)
			}
			return
		}
		c.openCookMenu(player, oneTypeTitle, []int{cookable.RawID}, carried,
			func(id, amount int) {
				c.startCooking(player, object, cookable, amount, selected, nil, deliberate)
			})
		return
	}
	byProduct := make(map[int][]*Cookable)
	var products []int
	for _, cookable := range cookables {
		product := c.cookedResult(player, cookable).CookedID
		if _, ok := byProduct[product]; !ok {
			products = append(products, product)
		}
		byProduct[product] = append(byProduct[product], cookable)
	}
	most := 0
	for _, group := range byProduct {
		total := 0
		for _, cookable := range group {
			if inventory != nil {
				total += inventory.GetAmount(cookable.RawID)
			}
		}
		if total > most {
			most = total
		}
	}
	c.openCookMenu(player, manyTypesTitle, products, most,
		func(id, amount int) {
			group := byProduct[id]
			if len(group) == 0 {
				return
			}
			cookable := group[0]
			_, deliberate := c.cookableForUse(cookable.RawID)
			c.startCooking(player, object, cookable, amount, nil, group[1:], deliberate)
		})
}

// openCookMenu opens a CreationMenu (Cookers.Cooking.js
// openCookMenu): mode 6, maxAmount and lastAmount the most
// carried of one food.
func (c *cooking) openCookMenu(player *game.Player, title string, items []int, most int, choose func(itemID, amount int)) {
	if player == nil || len(items) == 0 || choose == nil {
		return
	}
	if most < 1 {
		most = 1
	}
	menu := game.NewCreationMenu(title, items, choose,
		game.CreationMenuOptions{Mode: cookMenuMode, MaxAmount: &most, LastAmount: &most})
	c.sendCreationMenu(player, menu)
}

// ---- current cooker (Cookers.Cooking.js currentCooker) ----

// privateAreaOf returns the instanced area a plugins.GameObject
// belongs to, or nil for a shared-world object (or an object
// whose concrete type is not *game.GameObject — the listener
// supplies *game.GameObject in production). The woodcutting
// treeAt precedent.
func privateAreaOf(obj plugins.GameObject) *game.PrivateArea {
	if concrete, ok := obj.(*game.GameObject); ok && concrete != nil {
		return concrete.PrivateArea()
	}
	return nil
}

// matchesCooker is the currentCooker match predicate: same id,
// private area, tile, type, and face.
func matchesCooker(obj plugins.GameObject, session *cookSession) bool {
	return obj != nil && session != nil && obj.ID() == session.objectID &&
		privateAreaOf(obj) == session.privateArea &&
		obj.Location().Equals(session.location) &&
		obj.Type() == session.objectType && obj.Face() == session.objectFace
}

// currentCooker returns the active cooker, or nil (Cookers
// .Cooking.js currentCooker): the cooker can be unchanged
// scenery lazily copied into a template. MapObjects.get first
// (the map original, loading the region), else the runtime
// object still standing at the session's tile, or nil. The lazy
// template-copy half runs through PrivateArea.ResolveObject
// (nil variantFor = identity); a stump, replacement, or
// remembered removal in the shape slot hides the lazy original,
// including removals from the world's removed-objects list
// (RemovedObjects, World.ts:1099-1101). The woodcutting treeAt
// shape, adapted to the cooker match.
func (c *cooking) currentCooker(session *cookSession) plugins.GameObject {
	if session == nil {
		return nil
	}
	if session.privateArea != nil && session.privateArea.Destroyed() {
		return nil
	}
	objects := c.objectManager()
	if session.privateArea == nil && objects != nil {
		// MapObjects.get (Cookers.Cooking.js currentCooker):
		// the map original. A non-matching object answers nil
		// without trying the runtime leg, as TS does (the ??
		// only falls through on a null Get).
		if current := objects.Get(session.objectID, session.location); current != nil {
			if matchesCooker(current, session) {
				return current
			}
			return nil
		}
	}
	if objects != nil {
		for _, obj := range objects.ObjectsAt(session.location) {
			if obj != nil && obj.ID() == session.objectID && privateAreaOf(obj) == session.privateArea {
				if matchesCooker(obj, session) {
					return obj
				}
				return nil
			}
		}
	}
	if session.privateArea == nil || !session.privateArea.UsesTemplateMap() {
		return nil
	}
	cooker := session.privateArea.ResolveObject(session.objectID, session.location, nil)
	if cooker == nil || !matchesCooker(cooker, session) {
		return nil
	}
	// The template index still describes the original map after
	// depletion. A stump, replacement, or remembered removal in
	// that shape slot hides the lazy original.
	sameSlot := func(obj *game.GameObject) bool {
		return obj != nil && obj.PrivateArea() == session.privateArea &&
			obj.Type() == session.objectType && obj.Location().Equals(session.location)
	}
	for _, obj := range session.privateArea.GetObjects() {
		if sameSlot(obj) {
			return nil
		}
	}
	if objects != nil {
		for _, obj := range objects.ObjectsAt(session.location) {
			if concrete, ok := obj.(*game.GameObject); ok && sameSlot(concrete) {
				return nil
			}
		}
		for _, obj := range objects.RemovedObjects() {
			if concrete, ok := obj.(*game.GameObject); ok && sameSlot(concrete) {
				return nil
			}
		}
	}
	return cooker
}

// withinInteractionDistance reports whether the player stands
// close enough to keep cooking at the session's cooker
// (GameObject.isWithinInteractionDistance,
// GameObject.ts:151-155): the footprint distance to the loc's
// rectangle — size from the session (resolved at start, 1x1
// when unknown — the woodcutting precedent; Go has no
// object-definition source in the session) — is at most 2. An
// odd face swaps the footprint axes.
func (session *cookSession) withinInteractionDistance(from game.Location) bool {
	if session == nil {
		return false
	}
	width, length := session.sizeX, session.sizeY
	if session.objectFace&1 == 1 {
		width, length = session.sizeY, session.sizeX
	}
	gap := func(point, start, size int) int {
		return max(0, start-point, point-(start+size-1))
	}
	loc := session.location
	return max(gap(from.X, loc.X, width), gap(from.Y, loc.Y, length)) <= 2
}

// ---- the tick loop (Cookers.Cooking.js cookTick / cookStep) ----

// cookTick steps every cooking player once a tick (Cookers
// .Cooking.js cookTick): unregistered → delete; HP ≤ 0 → stop
// (busy cleared, as stopping any other way does); a
// non-empty movement queue or moving → stop (animation reset);
// level-up pause → skip; nothing left or no raw → stop;
// cooker gone or out of interaction distance → stop; before
// nextCookTick → wait; else cookStep.
//
// SEAM GAP (the woodcutting/fletching force-movement gap, same
// cause): the force-movement stop (player.getForceMovement()
// != null) cannot run — no ForceMovement field or system exists
// in Go (movement_queue.go documents its absence) — so a forced
// move never stops a cooking session here.
func (c *cooking) cookTick(cycle int64) {
	if c == nil {
		return
	}
	for player, session := range c.sessions {
		if player == nil || !player.IsRegistered() {
			delete(c.sessions, player)
			continue
		}
		// Dying before the first food is cooked must still
		// clear busy, as stopping any other way does.
		if hitpoints(player) <= 0 {
			c.stopCooking(player, false)
			continue
		}
		// Global tasks precede player turns; the last step may
		// have drained the route.
		movement := player.MovementQueue()
		if movement != nil && (movement.Size() > 0 || movement.IsMoving()) {
			c.stopCooking(player, true)
			continue
		}
		if session.levelUpPaused {
			continue
		}
		if session.remaining <= 0 || !c.carriedRaw(player, session) {
			c.stopCooking(player, true)
			continue
		}
		object := c.currentCooker(session)
		if object == nil || !session.withinInteractionDistance(player.Location()) {
			c.stopCooking(player, true)
			continue
		}
		if cycle < session.nextCookTick {
			continue
		}
		c.cookStep(player, session, cycle)
	}
}

// carriedRaw reports whether a raw is left to cook, moving on
// to the menu entry's next raw with the same product once one
// runs out (Cookers.Cooking.js carriedRaw).
func (c *cooking) carriedRaw(player *game.Player, session *cookSession) bool {
	if player == nil || session == nil || player.Inventory() == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory.Contains(session.cookable.RawID) {
		return true
	}
	for _, next := range session.alternates {
		if inventory.Contains(next.RawID) && c.canCookFood(player, next) {
			session.cookable = next
			return true
		}
	}
	return false
}

// cookStep cooks one cycle's foods (Cookers.Cooking.js
// cookStep): pace 3 for the first food, 4 after; content may
// cook faster ("skills:action-interval", a league relic's
// 2-tick pace); BUSY varbit 0 once the first food starts;
// cooked++ and remaining-- for the first food, then up to
// actionsPerCycle foods, each further one counted again;
// cookOne false stops cooking.
func (c *cooking) cookStep(player *game.Player, session *cookSession, cycle int64) {
	if player == nil || session == nil {
		return
	}
	// Content may cook faster ("skills:action-interval", a
	// league relic's 2-tick pace).
	pace := secondCookTicks
	if session.cooked != 0 {
		pace = cookIntervalTicks
	}
	session.nextCookTick = cycle + int64(skillactions.ActionInterval(c.emit, player, game.SkillCooking, "cook", pace, 0))
	if session.cooked == 0 {
		c.sendVarbit(player, busyVarbit, 0)
	}
	session.cooked++
	session.remaining--
	game.SendSound(player, game.SoundCookingCook)
	player.PerformAnimation(session.animation, 0)

	// Content may cook several in the one cycle
	// ("skills:actions-per-cycle", a league relic's all at
	// once), but never past the amount chosen: the first is
	// already counted above, each further one is counted here.
	actions := min(c.actionsPerCycle(player, "cook"), session.remaining+1)
	for done := 0; done < actions && c.carriedRaw(player, session); done++ {
		if done > 0 {
			session.remaining--
		}
		if !c.cookOne(player, session) {
			c.stopCooking(player, true)
			break
		}
	}
}

// ---- cookOne (Cookers.Cooking.js cookOne) ----

// cookOne cooks one food, in exact TS order: deliberate burn;
// selected-input revalidation; gnome-tutorial slot logic
// (the single non-preset slot; "Preset items cannot be used
// for this."; stackable guards; returnItem needs a free slot);
// non-gnome returnItem needs a free slot BEFORE anything
// changes, then the raw is deleted; the "cooking:burn" emit
// (a handler may flip Burn — read after); gnome revalidation
// + deleteAtSlot; success (addable check, add cooked,
// returnItem, spam, count, emits, XP) / failsWithoutBurning
// (the burnt item, count, NO XP, NO message) / burn (burnt,
// returnOnBurn, "cooking:burnt", spam, count). It returns
// false when cooking must stop, true otherwise (TS returns
// undefined to continue).
func (c *cooking) cookOne(player *game.Player, session *cookSession) bool {
	if player == nil || session == nil || session.cookable == nil {
		return false
	}
	cookable := c.cookedResult(player, session.cookable)
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	if !c.cookableAvailableOnWorld(cookable) {
		return false
	}
	if session.deliberate {
		inventory.DeleteNumber(session.cookable.RawID, 1)
		inventory.AddItem(game.NewItem(cookable.CookedID, 1))
		c.spam(player, successMessage(cookable))
		return true
	}
	if session.selected != nil && !validSelectedInput(player, session.cookable, session.selected) {
		return false
	}
	rawSlot := -1
	if cookable.GnomeTutorial {
		if session.selected != nil {
			rawSlot = session.selected.slot
		} else {
			for slot, item := range inventory.Items() {
				if !item.IsEmpty() && item.ID() == session.cookable.RawID && item.Amount() == 1 && !restricted(item) {
					rawSlot = slot
					break
				}
			}
		}
		if rawSlot < 0 {
			player.SendMessage(presetItemMessage)
			return false
		}
	}
	var rawItem game.Item
	rawFound := false
	if cookable.GnomeTutorial {
		items := inventory.Items()
		if rawSlot >= 0 && rawSlot < len(items) {
			rawItem, rawFound = items[rawSlot], true
		}
	} else {
		for _, item := range inventory.Items() {
			if !item.IsEmpty() && item.ID() == session.cookable.RawID {
				rawItem, rawFound = item, true
				break
			}
		}
	}
	rawRestore := game.NewItem(session.cookable.RawID, 1)
	if rawFound {
		rawRestore = rawItem.Clone()
		rawRestore.SetAmount(1)
	}
	if cookable.GnomeTutorial {
		// Success may return the tray/tin as a second
		// nonstackable item. Reserve that slot before the roll
		// or any mutation; burns retain the burnt item.
		if stackableOf(player, session.cookable.RawID) || stackableOf(player, cookable.CookedID) ||
			(cookable.HasReturnItem && stackableOf(player, cookable.ReturnItemID)) {
			return false
		}
		if cookable.HasReturnItem && inventory.GetFreeSlots() < 1 {
			inventory.Full()
			return false
		}
	} else {
		// A cake tin comes back as well as the cake: room for
		// it before anything changes.
		if cookable.HasReturnItem && inventory.GetFreeSlots() < 1 {
			inventory.Full()
			return false
		}
		inventory.DeleteNumber(session.cookable.RawID, 1)
	}
	burnRequest := &BurnRequest{
		Player: player,
		RawID:  session.cookable.RawID,
		ItemID: cookable.CookedID,
		Burn:   !c.isSuccess(player, session.cookable, session.objectID, session.cooker, session.location),
	}
	c.emitEvent(CookingBurnEventName, burnRequest)
	if cookable.GnomeTutorial {
		// The burn hook may run other content: revalidate the
		// selected input and the reserved capacity before
		// consuming anything.
		current := inventory.ItemAt(rawSlot)
		if !current.Equals(rawItem) || current.ID() != session.cookable.RawID ||
			current.Amount() != 1 || restricted(current) ||
			(cookable.HasReturnItem && inventory.GetFreeSlots() < 1) {
			return false
		}
		inventory.DeleteAtSlot(rawSlot, 1, true)
		session.selected = nil
	}
	if !burnRequest.Burn {
		output := game.NewItem(cookable.CookedID, 1)
		if inventory.GetAddableAmount(output) == 0 {
			inventory.AddItem(rawRestore)
			return true
		}
		before := inventory.GetAmount(cookable.CookedID)
		inventory.AddItem(output)
		amount := inventory.GetAmount(cookable.CookedID) - before
		if amount < 0 {
			amount = 0
		}
		if amount == 0 {
			inventory.AddItem(rawRestore)
			return true
		}
		if cookable.HasReturnItem {
			inventory.AddItem(game.NewItem(cookable.ReturnItemID, 1))
		}
		c.spam(player, successMessage(cookable))
		c.countCook(player, false)
		if amount > 0 {
			location := session.location
			c.emitEvent(ItemsObtainedEventName, &ObtainedEvent{
				Player: player, ItemID: cookable.CookedID, RawID: session.cookable.RawID,
				Amount: amount, Source: "cooking", Location: location,
			})
		}
		c.emitEvent(CookingSuccessEventName, &SuccessEvent{
			Player: player, Skill: game.SkillCooking, RawID: session.cookable.RawID,
			ItemID: cookable.CookedID, Amount: amount, Location: session.location,
			ObjectID: session.objectID, ObjectLocation: session.location,
		})
		c.addXP(player, cookable.XP)
		c.emitEvent(SkillsProducedEventName, &ProducedEvent{
			Player: player, Skill: game.SkillCooking, ItemID: cookable.CookedID,
			Amount: 1, XP: cookable.XP, Kind: "cook",
		})
	} else if cookable.FailsWithoutBurning {
		// Wiki (Drunk dragon): "sometimes no experience is
		// gained" until 38 Cooking, but "you will always
		// receive a finished drunk dragon": the item, no XP, no
		// burn line (none is recorded for it).
		inventory.AddItem(game.NewItem(session.cookable.BurntID, 1))
		c.countCook(player, false)
	} else {
		inventory.AddItem(game.NewItem(session.cookable.BurntID, 1))
		if cookable.ReturnOnBurn && cookable.HasReturnItem {
			inventory.AddItem(game.NewItem(cookable.ReturnItemID, 1))
		}
		// "cooking:burnt" { player, rawId, itemId }: the food
		// burnt (a league's "Burn Some Food").
		c.emitEvent(CookingBurntEventName, &BurntEvent{
			Player: player, RawID: session.cookable.RawID, ItemID: session.cookable.BurntID,
		})
		c.spam(player, burnMessage(cookable))
		c.countCook(player, true)
	}
	return true
}

// ---- handlers: isCooker / handleCook / refuseCook / handleCookClick ----

// isCookerByName is the isCooker predicate over a resolved
// name and interactions (Cookers.Cooking.js isCooker): a fire,
// or a range or oven with a Cook option. A Forester's Campfire
// has no Cook action — food is used on it directly.
func isCookerByName(name string, interactions []string) bool {
	if !CookableObjectNames[name] {
		return false
	}
	hasAction := false
	for _, action := range interactions {
		if action != "" {
			hasAction = true
			if action == "Cook" {
				return true
			}
		}
	}
	return !hasAction || FireObjectNames[name]
}

// IsCooker reports whether a definition is a cooker this use
// or click reaches (Cookers.Cooking.js isCooker, Cooking.plugin
// .js isCooker export): a fire, or a range or oven with a Cook
// option.
func IsCooker(def plugins.ObjectDefinition) bool {
	if def == nil {
		return false
	}
	return isCookerByName(def.Name(), def.Interactions())
}

// isCookerObject resolves an object id through the installed
// object info and applies the isCooker predicate; false without
// info (the item-on-object path has no event Definition — the
// hub's EmitItemOnObject carries none — so the info func is the
// only source; cmd/server installs it from the cache).
func (c *cooking) isCookerObject(objectID int) bool {
	info, ok := c.objectInfoOf(objectID)
	if !ok {
		return false
	}
	return isCookerByName(info.Name, info.Interactions)
}

// handleCook handles item-on-object for cooking (Cookers
// .Cooking.js handleCook): isCooker; cookableFor (rangeOnly on
// a fire → return, unclaimed); claim; gnome-tutorial selected
// input validation; offerCook.
func (c *cooking) handleCook(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || event.Object == nil {
		return
	}
	info, hasInfo := c.objectInfoOf(event.ObjectID)
	if !hasInfo || !isCookerByName(info.Name, info.Interactions) {
		return
	}
	cookable, _ := c.cookableForUse(event.ItemID)
	if cookable == nil {
		return
	}
	if cookable.RangeOnly && FireObjectNames[info.Name] {
		return
	}
	event.Handled = true
	var selected *selectedInput
	if cookable.GnomeTutorial {
		selected = &selectedInput{item: event.Item, slot: event.ItemSlot}
		if !validSelectedInput(event.Player, cookable, selected) {
			return
		}
	}
	c.offerCook(event.Player, event.Object, []*Cookable{cookable}, false, selected)
}

// refuseCook refuses any other use on a cooker (Cookers.Cooking
// .js refuseCook): isCooker → claim, mesbox CANNOT_COOK.
// Registered as the fallback, so it runs only when no earlier
// handler claimed the use (EmitItemOnObject runs main hooks
// first, then fallback hooks; see the file header).
func (c *cooking) refuseCook(event *plugins.ItemOnObjectEvent) {
	if event == nil || event.Player == nil || event.Object == nil {
		return
	}
	if !c.isCookerObject(event.ObjectID) {
		return
	}
	event.Handled = true
	c.mesbox(event.Player, cannotCookMessage)
}

// handleCookClick handles the Cook option on a cooker (Cookers
// .Cooking.js handleCookClick): the carried cookables; empty →
// mesbox NOTHING_TO_COOK; else offerCook with singleClick. It
// always claims the click (a mesbox still claims it, as in TS,
// where the handler returns undefined). The definition comes
// from the event when the emit filled it in, else the installed
// object info.
func (c *cooking) handleCookClick(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || event.Object == nil {
		return false
	}
	name := ""
	if event.Definition != nil {
		name = event.Definition.Name()
	}
	if name == "" {
		name = c.objectNameOf(event.ObjectID)
	}
	onFire := FireObjectNames[name]
	cookables := c.carriedCookables(event.Player, onFire)
	if len(cookables) == 0 {
		c.mesbox(event.Player, nothingToCookMessage)
		return true
	}
	c.offerCook(event.Player, event.Object, cookables, true, nil)
	return true
}

// ---- pause / disconnect / task (Cookers.Cooking.js pauseCooking etc.) ----

// pauseCooking holds the cooking session through a level-up
// pop-up (Cookers.Cooking.js pauseCooking, via
// skillactions.PauseForLevelUp): the resume replays nothing —
// TS passes no onResume, so resuming only clears the flag.
// Sessions must implement skillactions.Session (pointer
// identity matters — the fletching shared.go:1385 call shape).
func (c *cooking) pauseCooking(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil || c == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(c.sessions))
	for player, session := range c.sessions {
		sessions[player] = session
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		c.stopCooking(player, false)
	}, nil)
}

// stopOnDisconnect stops the player's session on disconnect,
// without an animation reset (Cookers.Cooking.js
// stopOnDisconnect).
func (c *cooking) stopOnDisconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil || event.Player == nil {
		return
	}
	c.stopCooking(event.Player, false)
}

// ensureCookTask schedules the ONE global 1-tick task for all
// cooking players, on the first cook (Cookers.Cooking.js
// ensureCookTask): game.NewTask(1, ...) on the lazily-resolved
// TaskManager — NEVER captured at Register time (Phase 32
// hub-contract lesson). The task owns its cycle counter (the
// TS cookingTask.cycle); Schedule runs on the tick goroutine
// (startCooking runs through the turn-packet drain). A nil
// TaskManager schedules nothing and retries on the next cook,
// the woodcutting ensureTask shape.
func (c *cooking) ensureCookTask() {
	if c == nil || c.taskScheduled {
		return
	}
	tm := c.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		c.cookCycle++
		c.cookTick(c.cookCycle)
		return false
	})
	task.Name = "CookingTask"
	tm.Schedule(task)
	c.taskScheduled = true
}

// registerCookers wires the Cookers unit (Cookers.Cooking.js
// attachCookers), in TS order: persist the success-count
// attribute; login / disconnect; the level-up pause; the
// cooking:count answer; handleCook then refuseCook as the
// fallback; the 11 Cook-option object interactions. The
// cooking:raw-xp / cooking:cooked answerers are NOT registered
// here — Worker A registers them once in register() (common.go
// registerAnswerers).
func (c *cooking) registerCookers(api *plugins.Api) {
	if api == nil {
		return
	}
	api.PersistAttribute(successCountAttribute, 0)
	api.OnPlayerLogin(c.sendSuccessCount)
	api.OnPlayerDisconnect(c.stopOnDisconnect)
	api.OnCustomEvent(skillactions.LevelUpPopupOpenedEventName, c.pauseCooking)
	api.OnCustomEvent(CookingCountEventName, c.countCookEvent)
	unnoted := &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false)}
	api.OnItemOnObject(c.handleCook, unnoted)
	api.OnItemOnObject(c.refuseCook, &plugins.ItemUseFilter{Noted: plugins.BoolPtr(false), Fallback: true})
	for _, name := range cookClickObjectNames {
		api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{"Cook": c.handleCookClick})
	}
}
