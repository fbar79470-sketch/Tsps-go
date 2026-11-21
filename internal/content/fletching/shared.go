// Package fletching is the Go port of the Fletching skill plugin
// (server/plugins/skills/Fletching.plugin.js, 36 lines, commit
// bfd81e3): every recipe in data/fletching.json, the make-many
// CreationMenu, the 1-tick FletchingTask, the bow string spool,
// and the fletching:shafts answer. Line numbers in comments cite
// server/plugins/skills/fletching/Shared.Fletching.js unless
// another file is named.
//
// The port is faithful: same plugin name, description, and
// members flag, same attach-only register, same guard order in
// handlePair and makeBatch, same task pacing (firstTick,
// knife saving, jeweller's-chisel pace), same partial-set
// handling, same random yields, same spool capacity tiers.
// Deviations forced by missing Go seams are named inline and in
// the Phase 34 report; nothing is silently invented.
//
// File split (Phase 34 work split, PHASE34_PLAN.md §11):
//
//   - shared.go (this file, Worker C): the resolved recipe model
//     (Recipe, Material, Tables — the Go form of resolveRecipe's
//     output) and the shared loop: pair index, level/actions
//     queries, gates, material planning, makeBatch, the task,
//     startFletching, openMenu, handlePair.
//   - recipes.go (Worker C): the Recipes unit's hook registrations.
//   - spool.go (Worker C): the Spool unit.
//   - fletching.go (Worker C): the plugin value, the shared
//     instance, answerShafts, and the Register wiring.
//   - tables.go (Worker A): the raw fletching.json structs and
//     Load / LoadFromCache, which resolve every gameval name
//     through the cache and return a *Tables. The resolved model
//     lives here, not there, so the loader constructs these
//     types; see the Tables doc comment for the loader contract.
//   - diaries.go (Worker A): the Diaries unit; registered from
//     fletching.go's Register in TS UNITS order.
//
// Event inventory:
//
//   - Listens to onItemOnItem (generic, noted:false — Recipes),
//     onItemActionNamed "Celastrus bark" Fletch (Recipes),
//     onPlayerDisconnect (Recipes), "level-up:popup-opened"
//     (Recipes, via skillactions.PauseForLevelUp; the emitter is
//     NOT STARTED, so the pause never fires yet).
//   - Listens to onPlayerLogin + onItemActionNamed "Bow string
//     spool" Fill/Check/Empty + onItemOnItemNamed bow string /
//     spool and spool / spool + "crafting:spun" (Spool; the
//     crafting:spun producer is NOT STARTED, so the listener is
//     registered but never fires).
//   - Answers "fletching:shafts" (no Go consumer yet — the
//     Woodcutting league relic is unported).
//   - Emits "skills:level-boost" (default boost 0), "skills:
//     actions-per-cycle" (default 1), "skills:action-interval"
//     (via skillactions.ActionInterval), "quest:is-complete" /
//     "quest:is-started" and "slayer:has-unlock" (both FAIL
//     CLOSED when unanswered — quests/Slayer are unported, so
//     gated recipes are silently unavailable), "fletching:made"
//     and "skills:produced" (the Diaries unit listens to the
//     former; no consumer exists yet for the latter's diary
//     side — diary:task has no consumer).
//
// State lives in a *fletching created per Plugin value — NO
// package-level mutable maps (the Phase 21 race fix): the
// shared instance behind Plugin is the only one, and tests
// build their own with newFletching. The task owns its tick
// counter.
//
// Known degradations (plan §10, documented not fixed here):
// XP integer truncation (SkillManager.AddExperiences takes an
// int; 37.5 pays 37, the woodcutting gap-16 precedent);
// quest/Slayer-gated recipes locked until those systems exist;
// crafting:spun has no producer; level-up:popup-opened is never
// emitted; fletching:shafts has no consumer; no SkillLevelUp
// emitter.
package fletching

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- constants (Shared.Fletching.js:20-23) ----

const (
	// creationAllSentinel is CREATION_ALL_SENTINEL (Shared
	// .Fletching.js:20): 28, the menu's "make all" amount and
	// the default maxSets.
	creationAllSentinel = 28
	// defaultMenu is DEFAULT_MENU (Shared.Fletching.js:21).
	defaultMenu = "make"
	// presetText is PRESET_TEXT (Shared.Fletching.js:23).
	presetText = "Preset items cannot be used for this."
	// spamChatType is the chat channel for recipe lines: the spam
	// channel (protocol.ChatTypeSpam, 105 — the game package
	// cannot import protocol, so the value is spelled out with
	// its provenance, as woodcutting spells out its varp id).
	spamChatType = 105
)

// ---- resolved recipe model ----
// These are the Go form of resolveRecipe's output
// (Shared.Fletching.js:128-167): every name already turned
// into an id by Worker A's loader (tables.go). The loader
// contract:
//
//   - One *Recipe per fletching.json recipes entry, in data
//     order, group defaults filled in before entry overrides
//     (group defaults: ticks, firstTick, startTick, menu,
//     animation, sound, message, missingTool, knifeBonus,
//     tool, held, instantVarbit, maxSets — see the JSON).
//   - Materials resolved through the alternatives table
//     (@feathers etc.): IDs in data order, UsedOnlyIDs the
//     usedOnly entries' ids (GRYPHON_FEATHER).
//   - Animation: false in the JSON (or the group) reads
//     HasAnimation false; otherwise AnimationID is the
//     CacheDefinitions.GetSeqId value of the animation name
//     ("human_fletching" when the entry and its group name
//     none).
//   - Sound: the zero game.Sound when the entry and its group
//     name none, else game.ResolveKnownSound's value for the
//     sound name (CROSSBOW_LIMBS / CUTTING / GEM_CUTTING).
//   - StartTick: nil when the JSON (entry or group) sets null
//     (no opening); otherwise a pointer to the value, with
//     undefined reading 0 (the choice's own tick).
//   - InstantVarbitID: the GetVarbitId value of the
//     instantVarbit name, HasInstantVarbit false when the
//     entry and its group name none.
//   - MaxSets: 28 when the entry and its group name none.
//   - Quest: nil when the entry names none; Unlock "" likewise.
//   - Menu: the menus entry (title, mode) for the recipe's menu
//     name ("make" when the entry and its group name none).
//   - Tables.ToolIDs maps every tools entry name to its ids in
//     data order; FletchingKnifeID / JewellersChiselID are the
//     FLETCHING_KNIFE / JEWELLERS_CHISEL ids; ArrowShaftID is
//     ARROW_SHAFT (the task's knife-saving delay + answerShafts);
//     KnifeID is KNIFE and CelastrusBarkID is CELASTRUS_BARK
//     (the Recipes unit's Fletch action); SpoolID / BowStringID
//     are BOW_STRING_SPOOL / BOW_STRING (the Spool unit);
//     SpoolChargesVarbitID / SpoolSizeVarbitID are the
//     bowstring_spool_charges / bowstring_spool_size varbit ids.
//     A nil *Tables (or a nil field) degrades every handler to
//     a no-op, the woodcutting precedent.

// Material is one resolved recipe material
// (resolveMaterial, Shared.Fletching.js:119-127).
type Material struct {
	// IDs are the material's item ids in data order
	// (alternatives expanded).
	IDs []int
	// UsedOnlyIDs are the ids that work only as the used item,
	// not the target (GRYPHON_FEATHER).
	UsedOnlyIDs []int
	// Amount is how many of the material one batch takes.
	Amount int
}

// HeldTool is one resolved held tool (resolveRecipe's held,
// Shared.Fletching.js:144): a tools-entry name and its ids.
type HeldTool struct {
	Name string
	IDs  []int
}

// Menu is one resolved make-many menu (DATA.menus,
// Shared.Fletching.js:132-133).
type Menu struct {
	Title string
	// Mode is the CreationMenu mode, nil when the menu sets
	// none (only some menus pass one through).
	Mode *int
}

// QuestGate is one resolved quest gate (resolveRecipe's quest,
// Shared.Fletching.js:164): the quest key and the state the
// recipe needs ("complete" or "started").
type QuestGate struct {
	Key   string
	State string
}

// Recipe is one resolved fletching recipe (resolveRecipe,
// Shared.Fletching.js:128-167).
type Recipe struct {
	Group     string
	OutputID  int
	Amount    int
	AmountMax int
	Level     int
	// XP is the recipe's XP per batch (float in the JSON).
	XP float64
	// XPEach pays per item made when set (ogre arrow shafts,
	// wolfbone arrowtips); HasXPEach marks it.
	XPEach    float64
	HasXPEach bool
	Ticks     int
	FirstTick int
	// StartTick is the tick the opening animation plays on
	// (nil: no opening, the first batch animates).
	StartTick  *int
	StartSound bool
	// JewellersTicks is the jeweller's-chisel pace, nil when
	// the recipe sets none.
	JewellersTicks *int
	KnifeBonus     bool
	// Tool is the tools-entry name the recipe needs
	// ("knife", "plainKnife", "chisel", "hammer"), "" when none.
	Tool      string
	Held      []HeldTool
	Materials []Material
	// AnimationID is the resolved seq id; HasAnimation false
	// plays none ("animation": false).
	AnimationID  int
	HasAnimation bool
	// Sound is the resolved sound; the zero Sound plays none.
	Sound game.Sound
	// Message is the spam line with {amount} to fill, "" when
	// the recipe sets none.
	Message string
	// MissingTool is the recipe's own missing-tool line, "" when
	// it sets none (the generic line is built instead).
	MissingTool string
	// InstantVarbitID is the resolved instant varbit;
	// HasInstantVarbit false means the recipe is never instant.
	InstantVarbitID  int
	HasInstantVarbit bool
	MaxSets          int
	Quest            *QuestGate
	// Unlock is the Slayer unlock name, "" when none.
	Unlock string
	Menu   Menu
}

// Tables is the resolved fletching data Worker A's loader
// returns: the recipes in data order plus the ids the loop
// and the units resolve outside the recipes. See the model
// comment above for the loader contract.
type Tables struct {
	Recipes []*Recipe
	// ToolIDs maps tools-entry names ("knife", "plainKnife",
	// "chisel", "hammer") to their item ids in data order.
	ToolIDs map[string][]int
	// FletchingKnifeID is FLETCHING_KNIFE (knife saving).
	FletchingKnifeID int
	// JewellersChiselID is JEWELLERS_CHISEL (bolt-tip pace).
	JewellersChiselID int
	// ArrowShaftID is ARROW_SHAFT (task knife-saving delay,
	// answerShafts).
	ArrowShaftID int
	// KnifeID is KNIFE (the Celastrus bark Fletch action).
	KnifeID int
	// CelastrusBarkID is CELASTRUS_BARK (same).
	CelastrusBarkID int
	// SpoolID is BOW_STRING_SPOOL; BowStringID is BOW_STRING.
	SpoolID     int
	BowStringID int
	// SpoolChargesVarbitID / SpoolSizeVarbitID are the
	// bowstring_spool_charges / bowstring_spool_size varbit ids.
	SpoolChargesVarbitID int
	SpoolSizeVarbitID    int

	// ---- Worker A additions (Phase 34 loader) ----
	// The loader fills these alongside the fields above; the
	// loop never reads them, but the Diaries unit and the
	// loader tests do. They are read-only after Load.
	// DiaryTasks is every JSON diaryTasks entry, items
	// resolved to ids, rects as-is (diaries.go).
	DiaryTasks []*DiaryTask
	// ItemIDs maps every raw item name the file uses
	// (ItemIdentifiers form, e.g. "ARROW_SHAFT") plus the
	// named ids below (BOW_STRING_SPOOL et al.) to its
	// resolved id.
	ItemIDs map[string]int
	// SeqIDs maps every animation gameval name the file uses
	// to its resolved seq id.
	SeqIDs map[string]int
	// VarbitIDs maps every varbit name the loader resolves
	// (the darts/bolts instant varbit plus the two spool
	// varbits) to its id.
	VarbitIDs map[string]int
	// SoundIDs maps every sound name the file uses to its
	// synth id.
	SoundIDs map[string]int
	// DistinctItemNames is the count of distinct raw item
	// names the recipes use, counting an "@alternative"
	// token once (265 for the imported file — the plan §1
	// count).
	DistinctItemNames int
	// DistinctExpandedItemNames is the count of distinct
	// item names with alternatives expanded (272: the 8
	// "@feathers" kinds count as 8, not 1).
	DistinctExpandedItemNames int
	// ResolvedItemNames is the count of distinct item names
	// resolved in total, tools and the named ids included.
	ResolvedItemNames int
	// FallbackItemNames is the count of distinct item names
	// resolved through the itemIDFallbacks table; the rest
	// resolved through the cache gamevals (GamevalItemNames).
	FallbackItemNames int
	GamevalItemNames  int
}

// ---- custom-event payloads ----
// Each mirrors the TS request object of the same name; a handler
// answers by mutating the payload in place, exactly as TS does.
// Every payload is emitted by pointer so the mutation is visible
// to the emitter (the woodcutting query precedent).

// LevelBoostRequest is the "skills:level-boost" query
// (Shared.Fletching.js:219-223): content may raise the checked
// level (a league relic's invisible +12); Boost defaults 0.
type LevelBoostRequest struct {
	Player *game.Player
	Skill  int
	Boost  int
}

// ActionsPerCycleRequest is the "skills:actions-per-cycle" query
// (Shared.Fletching.js:228-232): content may make one cycle
// make more batches (a league relic's all at once); Actions
// defaults 1.
type ActionsPerCycleRequest struct {
	Player  *game.Player
	Skill   int
	Kind    string
	Actions int
}

// QuestStateRequest is the "quest:is-complete" /
// "quest:is-started" query (Shared.Fletching.js:246-250):
// content answers Started / Complete; both default false, so
// an unanswered query FAILS CLOSED (quests are unported).
type QuestStateRequest struct {
	Player   *game.Player
	Key      string
	Started  bool
	Complete bool
}

// SlayerUnlockRequest is the "slayer:has-unlock" query
// (Shared.Fletching.js:252-256): content answers Unlocked; it
// defaults false, so an unanswered query FAILS CLOSED (Slayer
// is unported).
type SlayerUnlockRequest struct {
	Player   *game.Player
	Name     string
	Unlocked bool
}

// PlanInput is one consumed material (consumePlan,
// Shared.Fletching.js:396-406): the item id and the amount
// taken, virtual or not.
type PlanInput struct {
	ItemID int
	Amount int
}

// FletchingMadeEvent is the "fletching:made" emit (makeBatch,
// Shared.Fletching.js:445-448): the player, the output and how
// many, and the action payload (inputs, station, locations).
// StationObjectID is nil: fletching has no station object.
type FletchingMadeEvent struct {
	Player          *game.Player
	ItemID          int
	Amount          int
	InputItemIDs    []int
	Inputs          []PlanInput
	OutputItemID    int
	StationObjectID *int
	Location        game.Location
	StartLocation   *game.Location
}

// SkillsProducedEvent is the "skills:produced" emit (makeBatch,
// Shared.Fletching.js:449-450): the same action payload plus
// the skill, the kind, and the XP this batch paid (float, as
// TS pays it — the int-truncated XP is what lands in the
// SkillManager).
type SkillsProducedEvent struct {
	Player          *game.Player
	Skill           int
	ItemID          int
	Amount          int
	XP              float64
	Kind            string
	InputItemIDs    []int
	Inputs          []PlanInput
	OutputItemID    int
	StationObjectID *int
	Location        game.Location
	StartLocation   *game.Location
}

// ShaftsRequest is the "fletching:shafts" query
// (Fletching.plugin.js answerShafts): LogID asks what the log
// fletches into; ItemID / Amount / XP answer it, and stay zero
// when no arrow-shaft recipe uses the log.
type ShaftsRequest struct {
	LogID  int
	ItemID int
	Amount int
	XP     float64
}

// CraftingSpunEvent is the "crafting:spun" emit the spool
// listens to (Spool.Fletching.js absorbSpun): the spun item
// and how many. No Go producer exists yet (Crafting is
// unported), so this listener is registered but never fires.
type CraftingSpunEvent struct {
	Player *game.Player
	ItemID int
	Amount int
}

// provenance is the action provenance makeBatch spreads into
// its emits (Shared.Fletching.js:441-444): the location where
// the click started. Nil provenance means no start location.
type provenance struct {
	startLocation game.Location
}

// ---- CreationMenu (Worker B, plan §3) ----
// The make-many menu goes through Worker B's CreationMenu
// (internal/game/creationmenu.go): game.NewCreationMenu builds
// it, and the player's session sends it through the
// game.CreationMenuSender assertion (the DialogueSender
// pattern). openMenu builds the menu and hands it to the
// sender; a session without the sender opens nothing (a
// session-less player has no client to show it to).

// virtualMaterialSource is a material held somewhere other
// than the inventory (registerVirtualMaterial,
// Shared.Fletching.js:204-206): the spool's bow strings.
// Count answers how many the player holds; Take removes up
// to amount and answers how many it took.
type virtualMaterialSource struct {
	count func(*game.Player) int
	take  func(*game.Player, int) int
}

// ---- per-instance state ----

// fletching is the plugin's per-registration state: the TS
// module-level state (api, recipes, recipesByPair,
// virtualMaterials, triggerAliases, tasks, toolIds,
// fletchingKnifeId, jewellersChiselId) as struct fields, per
// the Phase 21 race fix. One instance serves the registered
// Plugin value; tests build their own with newFletching.
type fletching struct {
	tables *Tables
	// api is the Register-time Api, held so the hub surfaces
	// that install after plugin Load (TaskManager) can be
	// resolved lazily at use time through taskManager. Nil in
	// unit tests, which set tasks directly.
	api  *plugins.Api
	emit func(name string, payload any)
	// tasks is the directly-set TaskManager override (unit
	// tests set it); production leaves it nil and resolves
	// through the api instead — a Register-time capture would
	// read nil, because the TaskManager installs after Load.
	tasks *game.TaskManager
	// running maps each player to their running FletchingTask
	// (Shared.Fletching.js tasks Map).
	running map[*game.Player]*fletchingTask
	// recipesByPair maps "<lower id>:<higher id>" to the recipes
	// the pair starts, in data order.
	recipesByPair map[string][]*Recipe
	// virtualMaterials maps a material id to its elsewhere
	// source (the spool's bow strings).
	virtualMaterials map[int]virtualMaterialSource
	// triggerAliases maps an item id to the material id it
	// stands in for when used (spool -> bow string).
	triggerAliases map[int]int
	// randInt rolls an inclusive random int (the amount roll in
	// makeBatch); tests override it for determinism.
	randInt func(min, max int) int
	// noteID resolves an item id to its note id (the spool's
	// noted bow string); the test override wins, else the
	// player's item-definition source (EntTrunk precedent).
	noteID func(id int) int
}

// newFletching builds a fletching instance over tables (nil
// tables degrade every handler to a no-op; see Register).
func newFletching(tables *Tables) *fletching {
	f := &fletching{
		tables:           tables,
		running:          make(map[*game.Player]*fletchingTask),
		recipesByPair:    make(map[string][]*Recipe),
		virtualMaterials: make(map[int]virtualMaterialSource),
		triggerAliases:   make(map[int]int),
		randInt: func(min, max int) int {
			if max <= min {
				return min
			}
			return min + rand.IntN(max-min+1)
		},
	}
	f.indexRecipes()
	return f
}

// recipes returns the resolved recipes, or nil without tables.
func (f *fletching) recipes() []*Recipe {
	if f == nil || f.tables == nil {
		return nil
	}
	return f.tables.Recipes
}

// toolIDs returns the ids for a tools-entry name, or nil
// without tables.
func (f *fletching) toolIDs(name string) []int {
	if f == nil || f.tables == nil {
		return nil
	}
	return f.tables.ToolIDs[name]
}

// setTables installs resolved tables on the instance and
// rebuilds the pair index (and the spool's virtual-material
// registration, which needs the bow-string id). The
// coordinator calls this BEFORE hub Load: Register captures
// the shared instance, so a call before Load reaches every
// handler.
func (f *fletching) setTables(t *Tables) {
	if f == nil {
		return
	}
	f.tables = t
	f.indexRecipes()
}

// emitEvent emits a custom event through the captured emit
// func; a nil emit (before Register) drops the emit, which is
// the no-hooks default for every query.
func (f *fletching) emitEvent(name string, payload any) {
	if f == nil || f.emit == nil {
		return
	}
	f.emit(name, payload)
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract — cmd/server installs it from the built world),
// so a Register-time capture reads nil in production. The live
// Api value wins when present; the directly-set tasks field is
// the unit-test override. (The woodcutting ensureTask pattern,
// Phase 32 hub-contract lesson: lazy resolution, Schedule only
// on the tick goroutine.)
func (f *fletching) taskManager() *game.TaskManager {
	if f.api != nil {
		if tm := f.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return f.tasks
}

// ---- pair index (Shared.Fletching.js:169-202) ----

func pairKey(a, b int) string {
	if a < b {
		return fmt.Sprintf("%d:%d", a, b)
	}
	return fmt.Sprintf("%d:%d", b, a)
}

func (f *fletching) addPair(a, b int, recipe *Recipe) {
	key := pairKey(a, b)
	list := f.recipesByPair[key]
	for _, existing := range list {
		if existing == recipe {
			return
		}
	}
	f.recipesByPair[key] = append(list, recipe)
}

// triggerPairs returns the pairs that start a recipe
// (Shared.Fletching.js:184-195): the tool on the first
// material, or the first material on the second.
func (f *fletching) triggerPairs(recipe *Recipe) [][2]int {
	if recipe == nil || len(recipe.Materials) == 0 {
		return nil
	}
	if recipe.Tool != "" {
		var pairs [][2]int
		for _, tool := range f.toolIDs(recipe.Tool) {
			for _, base := range recipe.Materials[0].IDs {
				pairs = append(pairs, [2]int{tool, base})
			}
		}
		return pairs
	}
	if len(recipe.Materials) < 2 {
		return nil
	}
	var pairs [][2]int
	for _, a := range recipe.Materials[0].IDs {
		for _, b := range recipe.Materials[1].IDs {
			pairs = append(pairs, [2]int{a, b})
		}
	}
	return pairs
}

// indexRecipes builds the pair index from the tables
// (indexRecipes, Shared.Fletching.js:197-202), and registers
// the spool's virtual bow-string source + trigger alias
// (Spool.Fletching.js init, which runs after Shared's init in
// the plugin's UNITS order). Rebuilding clears the old index,
// so a second SetTables replaces the first cleanly.
func (f *fletching) indexRecipes() {
	f.recipesByPair = make(map[string][]*Recipe)
	f.virtualMaterials = make(map[int]virtualMaterialSource)
	f.triggerAliases = make(map[int]int)
	for _, recipe := range f.recipes() {
		for _, pair := range f.triggerPairs(recipe) {
			f.addPair(pair[0], pair[1], recipe)
		}
	}
	if f.tables == nil || f.tables.BowStringID == 0 || f.tables.SpoolID == 0 {
		return
	}
	f.registerVirtualMaterial(f.tables.BowStringID, virtualMaterialSource{
		count: f.spoolSourceCount,
		take:  f.spoolSourceTake,
	})
	f.addTriggerAlias(f.tables.SpoolID, f.tables.BowStringID)
}

// registerVirtualMaterial registers a virtual material source
// (Shared.Fletching.js:204-206).
func (f *fletching) registerVirtualMaterial(materialID int, source virtualMaterialSource) {
	f.virtualMaterials[materialID] = source
}

// addTriggerAlias lets another item start the recipes of a
// material it stands in for (Shared.Fletching.js:208-218),
// and re-indexes the alias into the existing pairs.
func (f *fletching) addTriggerAlias(aliasID, materialID int) {
	f.triggerAliases[aliasID] = materialID
	for _, recipe := range f.recipes() {
		for _, pair := range f.triggerPairs(recipe) {
			if pair[0] == materialID {
				f.addPair(aliasID, pair[1], recipe)
			}
			if pair[1] == materialID {
				f.addPair(pair[0], aliasID, recipe)
			}
		}
	}
}

// recipesFor returns the recipes a pair starts, dropping any
// whose material is used-only for the *target* id b
// (Shared.Fletching.js:220-223).
func (f *fletching) recipesFor(a, b int) []*Recipe {
	list := f.recipesByPair[pairKey(a, b)]
	var out []*Recipe
	for _, recipe := range list {
		usedOnly := false
		for _, material := range recipe.Materials {
			for _, id := range material.UsedOnlyIDs {
				if id == b {
					usedOnly = true
					break
				}
			}
			if usedOnly {
				break
			}
		}
		if !usedOnly {
			out = append(out, recipe)
		}
	}
	return out
}

// hasPair reports whether any recipe is indexed under the pair
// (recipesByPair.has, Shared.Fletching.js handlePair:529).
func (f *fletching) hasPair(a, b int) bool {
	_, ok := f.recipesByPair[pairKey(a, b)]
	return ok
}

// ---- level, actions, gates (Shared.Fletching.js:225-266) ----

// fletchingLevel is the Fletching level the skill checks: the
// current level, raised by content ("skills:level-boost", a
// league relic's invisible +12; default boost 0 — unanswered
// is fine).
func (f *fletching) fletchingLevel(player *game.Player) int {
	if player == nil || player.Skills() == nil {
		return 0
	}
	request := &LevelBoostRequest{Player: player, Skill: game.SkillFletching, Boost: 0}
	f.emitEvent("skills:level-boost", request)
	boost := request.Boost
	if boost < 0 {
		boost = 0
	}
	return player.Skills().CurrentLevel(game.SkillFletching) + boost
}

// actionsPerCycle is how many batches one cycle makes
// ("skills:actions-per-cycle", a league relic's all at once);
// 1 unless content says more.
func (f *fletching) actionsPerCycle(player *game.Player, recipe *Recipe) int {
	request := &ActionsPerCycleRequest{Player: player, Skill: game.SkillFletching, Kind: recipe.Group, Actions: 1}
	f.emitEvent("skills:actions-per-cycle", request)
	if request.Actions < 1 {
		return 1
	}
	return request.Actions
}

// questState answers a recipe's quest gate (questState,
// Shared.Fletching.js:246-250): the "quest:is-complete" /
// "quest:is-started" query, FAILING CLOSED when unanswered
// (quests are unported — request.complete / request.started
// stay false, so the gate reads locked).
func (f *fletching) questState(player *game.Player, quest *QuestGate) bool {
	if quest == nil {
		return true
	}
	request := &QuestStateRequest{Player: player, Key: quest.Key}
	if quest.State == "complete" {
		f.emitEvent("quest:is-complete", request)
		return request.Complete == true
	}
	f.emitEvent("quest:is-started", request)
	return request.Started == true
}

// hasSlayerUnlock answers a recipe's Slayer unlock
// (Shared.Fletching.js:252-256): the "slayer:has-unlock"
// query, FAILING CLOSED when unanswered (Slayer is unported).
func (f *fletching) hasSlayerUnlock(player *game.Player, name string) bool {
	request := &SlayerUnlockRequest{Player: player, Name: name}
	f.emitEvent("slayer:has-unlock", request)
	return request.Unlocked == true
}

// unlocked reports whether a recipe's quest and Slayer gates
// pass (Shared.Fletching.js:259-264). A recipe whose quest or
// Slayer unlock is missing is left out of the menu, as live
// leaves it out.
func (f *fletching) unlocked(player *game.Player, recipe *Recipe) bool {
	if recipe == nil {
		return false
	}
	if recipe.Quest != nil && !f.questState(player, recipe.Quest) {
		return false
	}
	if recipe.Unlock != "" && !f.hasSlayerUnlock(player, recipe.Unlock) {
		return false
	}
	return true
}

// ---- inventory helpers (Shared.Fletching.js:268-394) ----

// carries reports whether the player holds any of the ids in
// the inventory or worn (carries, Shared.Fletching.js:268-270).
func carries(player *game.Player, ids []int) bool {
	if player == nil {
		return false
	}
	for _, id := range ids {
		if inventory := player.Inventory(); inventory != nil && inventory.Contains(id) {
			return true
		}
		if equipment := player.Equipment(); equipment != nil && equipment.Contains(id) {
			return true
		}
	}
	return false
}

// restricted reports whether an item copy keeps restrictions
// that bar it from converting (restricted,
// Shared.Fletching.js:273-277): preset, untradeable, or
// unbankable. Go reads the meta flags directly (Item.
// IsPresetItem / IsUntradeable / IsUnbankable, Item.ts:197-205).
func restricted(item game.Item) bool {
	return item.IsPresetItem() || item.IsUntradeable() || item.IsUnbankable()
}

// plainAmount counts the unrestricted copies of id in the
// inventory (plainAmount, Shared.Fletching.js:280-285), in slot
// order. Go always has the items list, so the getAmount
// fallback has no Go form.
func plainAmount(inventory *game.Inventory, id int) int {
	if inventory == nil {
		return 0
	}
	total := 0
	for _, item := range inventory.Items() {
		if item.ID() == id && !restricted(item) {
			total += item.Amount()
		}
	}
	return total
}

// countFunc counts copies of an id in an inventory:
// plainAmount by default, or the raw getAmount for the
// preset-only check in handlePair.
type countFunc func(*game.Inventory, int) int

// heldAmount counts how many of a material the player can use,
// a virtual source's (the spool's) included (heldAmount,
// Shared.Fletching.js:288-297).
func (f *fletching) heldAmount(player *game.Player, material Material, count countFunc) int {
	if player == nil {
		return 0
	}
	if count == nil {
		count = plainAmount
	}
	inventory := player.Inventory()
	total := 0
	for _, id := range material.IDs {
		total += count(inventory, id)
		if source, ok := f.virtualMaterials[id]; ok && source.count != nil {
			total += source.count(player)
		}
	}
	return total
}

// fullBatches counts the full sets the player can make
// (Shared.Fletching.js:299-306).
func (f *fletching) fullBatches(player *game.Player, recipe *Recipe) int {
	if recipe == nil || len(recipe.Materials) == 0 {
		return 0
	}
	batches := int(^uint(0) >> 1) // MaxInt
	for _, material := range recipe.Materials {
		if held := f.heldAmount(player, material, nil); material.Amount > 0 {
			if n := held / material.Amount; n < batches {
				batches = n
			}
		} else {
			batches = 0
		}
	}
	if batches == int(^uint(0)>>1) {
		return 0
	}
	return batches
}

// partialSize counts the last partial set of a one-for-one
// recipe (Shared.Fletching.js:314-319): 0 unless amount > 1,
// amountMax == amount, and every material amount == recipe
// amount; then held % amount, 0 when nothing is held.
func (f *fletching) partialSize(player *game.Player, recipe *Recipe) int {
	if recipe == nil || !(recipe.Amount > 1) || recipe.AmountMax != recipe.Amount {
		return 0
	}
	for _, material := range recipe.Materials {
		if material.Amount != recipe.Amount {
			return 0
		}
	}
	held := int(^uint(0) >> 1)
	for _, material := range recipe.Materials {
		if n := f.heldAmount(player, material, nil); n < held {
			held = n
		}
	}
	if held <= 0 {
		return 0
	}
	return held % recipe.Amount
}

// scaledRecipe scales a recipe down to a partial set
// (Shared.Fletching.js:321-324): amount/amountMax become the
// partial size, XP scales with it, every material amount
// becomes the partial size. xpEach (when set) still pays per
// item made.
func scaledRecipe(recipe *Recipe, made int) *Recipe {
	if recipe == nil {
		return nil
	}
	scaled := *recipe
	scaled.Amount = made
	scaled.AmountMax = made
	scaled.XP = (recipe.XP * float64(made)) / float64(recipe.Amount)
	materials := make([]Material, len(recipe.Materials))
	for i, material := range recipe.Materials {
		materials[i] = material
		materials[i].Amount = made
	}
	scaled.Materials = materials
	return &scaled
}

// batchesAvailable counts the batches the player can make:
// full sets, and a last partial one (Shared.Fletching.js:327-329).
func (f *fletching) batchesAvailable(player *game.Player, recipe *Recipe) int {
	n := f.fullBatches(player, recipe)
	if f.partialSize(player, recipe) > 0 {
		n++
	}
	return n
}

// hasTools reports whether the player holds the recipe's tool
// and held tools (Shared.Fletching.js:331-334).
func (f *fletching) hasTools(player *game.Player, recipe *Recipe) bool {
	if recipe == nil {
		return false
	}
	if recipe.Tool != "" && !carries(player, f.toolIDs(recipe.Tool)) {
		return false
	}
	for _, tool := range recipe.Held {
		if !carries(player, tool.IDs) {
			return false
		}
	}
	return true
}

// takeOrder is the order a material's ids are taken in: the
// clicked one, then data order (Shared.Fletching.js:337-340).
// The clicked id resolves through the trigger aliases first
// (a spool click counts as its bow string).
func (f *fletching) takeOrder(material Material, via int) []int {
	clicked := via
	if aliased, ok := f.triggerAliases[via]; ok {
		clicked = aliased
	}
	found := false
	for _, id := range material.IDs {
		if id == clicked {
			found = true
			break
		}
	}
	if !found {
		return material.IDs
	}
	ordered := []int{clicked}
	for _, id := range material.IDs {
		if id != clicked {
			ordered = append(ordered, id)
		}
	}
	return ordered
}

// materialPlanResult is the plan TS builds: per-entry
// {itemId, amount, virtual} plus the complete flag. The
// entries keep the virtual flag (TS {itemId, amount, virtual},
// Shared.Fletching.js:348-385); inputs is the same plan
// without the flag, for the space check and the payload.
type materialPlanResult struct {
	entries  []planEntry
	inputs   []PlanInput
	complete bool
}

// planEntry is one material-plan entry: the item, the amount,
// and whether it comes from a virtual source (TS {itemId,
// amount, virtual}, Shared.Fletching.js:348-385).
type planEntry struct {
	itemID  int
	amount  int
	virtual bool
}

// materialPlan plans taking a recipe's materials, starting
// with the item the player clicked (Shared.Fletching.js:348-385):
// the feather they used, or the spool's bow strings when they
// used the spool. Virtual-first when via is an alias, else
// inventory-first, then virtual — the exact order matters.
// Preset (and other restricted) copies are never part of the
// plan. Complete reports whether every material was fully
// planned.
func (f *fletching) materialPlan(player *game.Player, recipe *Recipe, via int) materialPlanResult {
	inventory := player.Inventory()
	inventoryLeft := make(map[int]int)
	virtualLeft := make(map[int]int)
	var entries []planEntry
	complete := true
	sourceFirst := false
	if _, ok := f.triggerAliases[via]; ok {
		sourceFirst = true
	}
	for _, material := range recipe.Materials {
		remaining := material.Amount
		for _, id := range f.takeOrder(material, via) {
			source, hasSource := f.virtualMaterials[id]
			if hasSource && source.count != nil {
				if _, seen := virtualLeft[id]; !seen {
					virtualLeft[id] = source.count(player)
				}
			}
			if hasSource && sourceFirst && remaining > 0 {
				fromSource := min(remaining, virtualLeft[id])
				if fromSource > 0 {
					entries = append(entries, planEntry{itemID: id, amount: fromSource, virtual: true})
				}
				virtualLeft[id] -= fromSource
				remaining -= fromSource
			}
			if _, seen := inventoryLeft[id]; !seen {
				inventoryLeft[id] = plainAmount(inventory, id)
			}
			fromInventory := min(remaining, inventoryLeft[id])
			if fromInventory > 0 {
				entries = append(entries, planEntry{itemID: id, amount: fromInventory, virtual: false})
			}
			inventoryLeft[id] -= fromInventory
			remaining -= fromInventory
			if hasSource && !sourceFirst && remaining > 0 {
				fromSource := min(remaining, virtualLeft[id])
				if fromSource > 0 {
					entries = append(entries, planEntry{itemID: id, amount: fromSource, virtual: true})
				}
				virtualLeft[id] -= fromSource
				remaining -= fromSource
			}
			if remaining <= 0 {
				break
			}
		}
		if remaining > 0 {
			complete = false
		}
	}
	inputs := make([]PlanInput, len(entries))
	for i, entry := range entries {
		inputs[i] = PlanInput{ItemID: entry.itemID, Amount: entry.amount}
	}
	return materialPlanResult{entries: entries, inputs: inputs, complete: complete}
}

// canGrantAfterConsumption reports whether the output fits at
// amountMax after the plan's inventory inputs are taken
// (Shared.Fletching.js:387-394). Go has no getCopiedItems /
// Inventory(player).setItems pair: the preview is a fresh
// inventory over a copy of the player's items (Items returns
// deep copies; SetItems copies again), and takePlain runs on
// the preview over the plan's non-virtual entries. A nil
// inventory previews as fitting, the TS fallback when
// getCopiedItems is missing.
func (f *fletching) canGrantAfterConsumption(player *game.Player, outputID, amount int, plan []planEntry) bool {
	if player == nil {
		return true
	}
	inventory := player.Inventory()
	if inventory == nil {
		return true
	}
	preview := game.NewInventory(player)
	preview.SetItems(inventory.Items())
	for _, entry := range plan {
		if !entry.virtual {
			takePlain(preview, entry.itemID, entry.amount)
		}
	}
	return preview.GetAddableAmount(game.NewItem(outputID, amount)) >= amount
}

// takePlain takes amount of id off the inventory's
// unrestricted copies, in slot order (Shared.Fletching.js:
// 396-407). Go always has deleteAtSlot / getItems, so the
// deleteNumber fallback has no Go form. It returns how many
// were actually taken.
func takePlain(inventory *game.Inventory, id, amount int) int {
	if inventory == nil || amount <= 0 {
		return 0
	}
	remaining := amount
	taken := 0
	for slot, item := range inventory.Items() {
		if remaining <= 0 {
			break
		}
		if item.ID() != id || restricted(item) {
			continue
		}
		take := min(remaining, item.Amount())
		inventory.DeleteAtSlot(slot, take, true)
		remaining -= take
		taken += take
	}
	return taken
}

// consumePlan consumes a plan's inputs (consumePlan,
// Shared.Fletching.js:409-419): virtual entries through their
// source, inventory entries through takePlain. It returns the
// consumed inputs (item id + amount, in plan order) for the
// action payload.
func (f *fletching) consumePlan(player *game.Player, plan []planEntry) []PlanInput {
	inventory := player.Inventory()
	var inputs []PlanInput
	for _, entry := range plan {
		if entry.virtual {
			if source, ok := f.virtualMaterials[entry.itemID]; ok && source.take != nil {
				source.take(player, entry.amount)
			}
		} else if inventory != nil {
			takePlain(inventory, entry.itemID, entry.amount)
		}
		inputs = append(inputs, PlanInput{ItemID: entry.itemID, Amount: entry.amount})
	}
	return inputs
}

// formatMessage fills {amount} in a recipe message or menu
// title (Shared.Fletching.js:421-423).
func formatMessage(message string, amount int) string {
	return strings.ReplaceAll(message, "{amount}", fmt.Sprintf("%d", amount))
}

// ---- varbit access ----
// Player has no GetVarbit/SendVarbit method (SessionPort
// carries neither); the session implements both, so content
// asserts the small interfaces, the examine-price precedent
// (game/examine_price.go). A session-less player reads 0 (the
// varbit default) and sends nowhere.

type varbitReader interface {
	GetVarbit(id int) int
}

type varbitSender interface {
	SendVarbit(id, value int)
}

func getVarbit(player *game.Player, id int) int {
	if player == nil || player.Session == nil {
		return 0
	}
	if reader, ok := player.Session.(varbitReader); ok {
		return reader.GetVarbit(id)
	}
	return 0
}

func sendVarbit(player *game.Player, id, value int) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(varbitSender); ok {
		sender.SendVarbit(id, value)
	}
}

// ---- makeBatch (Shared.Fletching.js:425-454) ----

// animate plays a recipe's animation, or nothing for a recipe
// that plays none (animationOf, Shared.Fletching.js:107-112).
func animate(player *game.Player, recipe *Recipe) {
	if player == nil || recipe == nil || !recipe.HasAnimation {
		return
	}
	player.PerformAnimation(recipe.AnimationID, 0)
}

// sendMessage sends a plain game message, dropping nil players.
func sendMessage(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendMessage(text)
}

// makeBatch makes one batch: checks, materials out, product in,
// XP, animation, sound and message — in exact TS order (level
// gate → tools/availability → partial scaling → complete plan
// → random amount roll → space check at amountMax → consume →
// add → animate → sound → spam message → XP → emits). It
// returns false when nothing was made (and, except for the
// level gate and the space refusal, silently).
//
// XP INTEGER TRUNCATION (plan §2, the woodcutting gap-16
// precedent): the JSON pays float XP (16.5, 37.5); Go
// SkillManager.AddExperiences takes an int, so the batch XP is
// truncated (37.5 → 37) at the call. The skills:produced emit
// still carries the float XP, as TS emits it.
func (f *fletching) makeBatch(player *game.Player, wholeRecipe *Recipe, via int, prov *provenance) bool {
	if player == nil || wholeRecipe == nil {
		return false
	}
	if f.fletchingLevel(player) < wholeRecipe.Level {
		sendMessage(player, fmt.Sprintf("You need a Fletching level of at least %d to do this.", wholeRecipe.Level))
		return false
	}
	if !f.hasTools(player, wholeRecipe) || f.batchesAvailable(player, wholeRecipe) <= 0 {
		return false
	}
	recipe := wholeRecipe
	if f.fullBatches(player, wholeRecipe) <= 0 {
		recipe = scaledRecipe(wholeRecipe, f.partialSize(player, wholeRecipe))
	}
	plan := f.materialPlan(player, recipe, via)
	if !plan.complete {
		return false
	}
	amount := recipe.Amount
	if recipe.AmountMax > recipe.Amount {
		amount = recipe.Amount + f.randInt(0, recipe.AmountMax-recipe.Amount)
	}
	// The batch must fit at its largest roll, or nothing is
	// taken: a batch has fixed inputs, so it refuses rather
	// than making part and losing the rest of the output.
	if !f.canGrantAfterConsumption(player, recipe.OutputID, recipe.AmountMax, plan.entries) {
		sendMessage(player, "You don't have enough inventory space.")
		return false
	}
	inputs := f.consumePlan(player, plan.entries)
	if inventory := player.Inventory(); inventory != nil {
		inventory.AddItem(game.NewItem(recipe.OutputID, amount))
	}
	animate(player, recipe)
	if recipe.Sound.ID() > 0 {
		game.SendSound(player, recipe.Sound)
	}
	if recipe.Message != "" {
		player.SendTypedMessage(spamChatType, formatMessage(recipe.Message, amount), "")
	}
	xp := recipe.XP
	if recipe.HasXPEach {
		xp = recipe.XPEach * float64(amount)
	}
	if xp > 0 {
		if skills := player.Skills(); skills != nil {
			skills.AddExperiences(game.SkillFletching, int(xp))
		}
	}
	action := f.actionPayload(player, inputs, recipe, prov)
	f.emitEvent(MadeEventName, &FletchingMadeEvent{
		Player:          player,
		ItemID:          recipe.OutputID,
		Amount:          amount,
		InputItemIDs:    action.inputItemIDs,
		Inputs:          action.inputs,
		OutputItemID:    recipe.OutputID,
		StationObjectID: nil,
		Location:        action.location,
		StartLocation:   action.startLocation,
	})
	f.emitEvent("skills:produced", &SkillsProducedEvent{
		Player:          player,
		Skill:           game.SkillFletching,
		ItemID:          recipe.OutputID,
		Amount:          amount,
		XP:              xp,
		Kind:            recipe.Group,
		InputItemIDs:    action.inputItemIDs,
		Inputs:          action.inputs,
		OutputItemID:    recipe.OutputID,
		StationObjectID: nil,
		Location:        action.location,
		StartLocation:   action.startLocation,
	})
	return true
}

// actionPayload builds the shared action payload makeBatch
// spreads into both emits (Shared.Fletching.js:436-444):
// the distinct input ids, the inputs, the output, the null
// station, the current location, and the start location when
// provenance was supplied.
type actionPayloadResult struct {
	inputItemIDs  []int
	inputs        []PlanInput
	location      game.Location
	startLocation *game.Location
}

func (f *fletching) actionPayload(player *game.Player, inputs []PlanInput, recipe *Recipe, prov *provenance) actionPayloadResult {
	seen := make(map[int]bool)
	var ids []int
	for _, input := range inputs {
		if !seen[input.ItemID] {
			seen[input.ItemID] = true
			ids = append(ids, input.ItemID)
		}
	}
	payload := actionPayloadResult{inputItemIDs: ids, inputs: inputs}
	if player != nil {
		payload.location = player.Location()
	}
	if prov != nil {
		start := prov.startLocation
		payload.startLocation = &start
	}
	return payload
}

// ---- interval (Shared.Fletching.js:456-477) ----

// interval is the ticks to the next batch: the recipe's, its
// tool's; content may make it faster ("skills:action-interval",
// a league relic's 2-tick pace), and a fletching knife's tick
// comes off after that, so the two stack.
func (f *fletching) interval(player *game.Player, recipe *Recipe, delayKnifeSaving bool) int {
	saving := 0
	if !delayKnifeSaving {
		saving = f.knifeSaving(player, recipe)
	}
	return skillactions.ActionInterval(f.emit, player, game.SkillFletching, recipe.Group, f.toolInterval(player, recipe), saving)
}

// toolInterval is the recipe's pace, or the jeweller's-chisel
// pace when the player carries a jeweller's chisel
// (Shared.Fletching.js:468-471). Inventory only, as in TS.
func (f *fletching) toolInterval(player *game.Player, recipe *Recipe) int {
	if recipe.JewellersTicks != nil && f.tables != nil {
		if inventory := player.Inventory(); inventory != nil && inventory.Contains(f.tables.JewellersChiselID) {
			return *recipe.JewellersTicks
		}
	}
	return recipe.Ticks
}

// knifeSaving is a carried or worn fletching knife's one tick
// off a knife recipe's pace (Shared.Fletching.js:474-477).
func (f *fletching) knifeSaving(player *game.Player, recipe *Recipe) int {
	if recipe.KnifeBonus && f.tables != nil && carries(player, []int{f.tables.FletchingKnifeID}) {
		return 1
	}
	return 0
}

// playOpening plays the animation (and the sound, with
// startSound) that opens the loop before the first batch
// (Shared.Fletching.js:480-483).
func (f *fletching) playOpening(player *game.Player, recipe *Recipe) {
	animate(player, recipe)
	if recipe.StartSound && recipe.Sound.ID() > 0 {
		game.SendSound(player, recipe.Sound)
	}
}

// ---- task (Shared.Fletching.js:485-553) ----

// stopFletching stops the player's running task, with an
// optional animation reset (Shared.Fletching.js:485-491).
func (f *fletching) stopFletching(player *game.Player, resetAnimation bool) {
	if f == nil || player == nil {
		return
	}
	task, ok := f.running[player]
	if !ok || task == nil {
		return
	}
	delete(f.running, player)
	task.stop()
	if resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// pauseFletching holds the running task through a level-up
// pop-up (Shared.Fletching.js:493-495, via
// skillactions.PauseForLevelUp): the resume replays the
// recipe animation, as TS passes (task) => task
// .resumeAnimation(). The emitter is NOT STARTED, so this
// never fires in production yet.
func (f *fletching) pauseFletching(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(f.running))
	for player, task := range f.running {
		sessions[player] = task
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		f.stopFletching(player, false)
	}, func(session skillactions.Session) {
		if task, ok := session.(*fletchingTask); ok && task != nil {
			task.resumeAnimation()
		}
	})
}

// fletchingTask is one player's FletchingTask (createTask,
// Shared.Fletching.js:497-551): a 1-tick task on the
// lazily-resolved TaskManager. It implements
// skillactions.Session, so PauseForLevelUp can hold it; it must
// stay a pointer (comparable) value for the helper's identity
// checks.
type fletchingTask struct {
	owner         *fletching
	player        *game.Player
	recipe        *Recipe
	batches       int
	via           int
	prov          *provenance
	ticks         int
	nextTick      int
	remaining     int
	levelUpPaused bool
	gameTask      *game.Task
}

// SetLevelUpPaused marks the task held by a level-up pop-up
// (skillactions.Session).
func (t *fletchingTask) SetLevelUpPaused(paused bool) {
	if t != nil {
		t.levelUpPaused = paused
	}
}

// stop stops the underlying game task; the map entry is
// removed by stopFletching (TS FletchingTask.stop also deletes
// the map entry — here stopFletching owns that, so a task
// stopped from inside execute does not double-delete).
func (t *fletchingTask) stop() {
	if t != nil && t.gameTask != nil {
		t.gameTask.Stop()
	}
}

// resumeAnimation replays the recipe animation when a
// level-up pop-up closes (FletchingTask.resumeAnimation,
// Shared.Fletching.js:513-515).
func (t *fletchingTask) resumeAnimation() {
	if t == nil || t.owner == nil {
		return
	}
	animate(t.player, t.recipe)
}

// hitpoints reads the player's current Hitpoints, or 0 without
// a skill manager (the task's death check,
// Shared.Fletching.js:518).
func hitpoints(player *game.Player) int {
	if player == nil || player.Skills() == nil {
		return 0
	}
	return player.Skills().CurrentLevel(game.SkillHitpoints)
}

// execute runs one task tick. It returns done=true when the
// task stopped itself (the manager drops it); false keeps it
// ticking. Order mirrors FletchingTask.execute
// (Shared.Fletching.js:517-549): registered/alive →
// level-up pause → tick counter + startTick opening →
// nextTick gate → knife-saving delay for the first two
// arrow-shaft logs → interval → actionsPerCycle batches,
// stopping when a batch fails (animation reset only when the
// FIRST action failed) or when no batches remain.
//
// SEAM GAP (the woodcutting force-movement gap, same cause):
// the force-movement stop (player.getForceMovement() != null)
// cannot run — no ForceMovement field or system exists in Go
// (movement_queue.go documents its absence) — so a forced
// move never stops a fletching task here.
func (t *fletchingTask) execute() bool {
	f := t.owner
	player := t.player
	recipe := t.recipe
	if player == nil || !player.IsRegistered() || hitpoints(player) <= 0 {
		f.stopFletching(player, false)
		return true
	}
	if t.levelUpPaused {
		return false
	}
	t.ticks++
	if recipe.StartTick != nil && t.ticks == *recipe.StartTick {
		f.playOpening(player, recipe)
	}
	if t.ticks < t.nextTick {
		return false
	}
	// The first two arrow-shaft logs use the normal pace even
	// with a fletching knife.
	delayKnifeSaving := t.remaining == t.batches && f.tables != nil && recipe.OutputID == f.tables.ArrowShaftID
	t.nextTick = t.ticks + f.interval(player, recipe, delayKnifeSaving)
	actions := f.actionsPerCycle(player, recipe)
	for done := 0; done < actions; done++ {
		if !f.makeBatch(player, recipe, t.via, t.prov) {
			f.stopFletching(player, done == 0)
			return true
		}
		t.remaining--
		if t.remaining <= 0 || f.batchesAvailable(player, recipe) <= 0 {
			f.stopFletching(player, false)
			return true
		}
	}
	return false
}

// createTask builds the player's FletchingTask for up to
// batches batches (createTask, Shared.Fletching.js:497-553).
// The game task is a 1-tick task (NewTask(1, ...)): the
// world's phase order runs task_manager before
// process_players, so a task scheduled on the click's turn
// drain first executes on the NEXT cycle — exactly TS's "the
// task's first execute is the cycle after the click".
func (f *fletching) createTask(player *game.Player, recipe *Recipe, batches, via int, prov *provenance) *fletchingTask {
	task := &fletchingTask{
		owner:     f,
		player:    player,
		recipe:    recipe,
		batches:   batches,
		via:       via,
		prov:      prov,
		nextTick:  recipe.FirstTick,
		remaining: batches,
	}
	gameTask := game.NewTask(1, func(cycle int64) bool {
		return task.execute()
	})
	gameTask.Name = "FletchingTask"
	gameTask.Key = player
	task.gameTask = gameTask
	return task
}

// ---- startFletching / openMenu / isInstant / handlePair ----

// startFletching starts making up to requested batches (28 =
// all), the first on the recipe's first tick
// (Shared.Fletching.js:556-575).
func (f *fletching) startFletching(player *game.Player, recipe *Recipe, requested, via int, prov *provenance) {
	if player == nil || recipe == nil {
		return
	}
	if f.fletchingLevel(player) < recipe.Level {
		sendMessage(player, fmt.Sprintf("You need a Fletching level of at least %d to do this.", recipe.Level))
		return
	}
	available := f.batchesAvailable(player, recipe)
	if available <= 0 || requested <= 0 {
		return
	}
	batches := available
	if requested != creationAllSentinel {
		batches = min(requested, available)
	}
	batches = min(batches, recipe.MaxSets)
	f.stopFletching(player, false)
	if recipe.Ticks == 0 {
		f.makeBatch(player, recipe, via, prov)
		return
	}
	tm := f.taskManager()
	if tm == nil {
		// No TaskManager yet (the hub installs it after Load):
		// schedule nothing and retry on the next click, the
		// woodcutting ensureTask shape. A half-registered task
		// that could never tick would be worse than a lost
		// start.
		return
	}
	task := f.createTask(player, recipe, batches, via, prov)
	f.running[player] = task
	tm.Schedule(task.gameTask)
	if recipe.StartTick != nil && *recipe.StartTick == 0 {
		f.playOpening(player, recipe)
	}
}

// openMenu opens the skillmulti menu for the recipes a pair
// starts, each offered up to what can be made
// (Shared.Fletching.js:578-594). It stops any running task
// first; the max amount is the most batches any choice can
// make (at least 1), capped at 28; the title fills with the
// first choice's amount; the mode passes through when set
// (0 when the menu sets none, CreationMenu's default). The
// menu is built with game.NewCreationMenu and sent through
// the session's CreationMenuSender; a session without the
// sender opens nothing.
func (f *fletching) openMenu(player *game.Player, choices []*Recipe, via int, prov *provenance) {
	if player == nil || len(choices) == 0 {
		return
	}
	f.stopFletching(player, false)
	byOutput := make(map[int]*Recipe, len(choices))
	var itemIDs []int
	maxAmount := 1
	for _, recipe := range choices {
		byOutput[recipe.OutputID] = recipe
		itemIDs = append(itemIDs, recipe.OutputID)
		if n := min(f.batchesAvailable(player, recipe), recipe.MaxSets); n > maxAmount {
			maxAmount = n
		}
	}
	maxAmount = min(maxAmount, creationAllSentinel)
	mode := 0
	if choices[0].Menu.Mode != nil {
		mode = *choices[0].Menu.Mode
	}
	menu := game.NewCreationMenu(
		formatMessage(choices[0].Menu.Title, choices[0].Amount),
		itemIDs,
		func(outputID, amount int) {
			if recipe := byOutput[outputID]; recipe != nil {
				f.startFletching(player, recipe, amount, via, prov)
			}
		},
		game.CreationMenuOptions{MaxAmount: &maxAmount, Mode: mode},
	)
	if player.Session == nil {
		return
	}
	sender, ok := player.Session.(game.CreationMenuSender)
	if !ok {
		return
	}
	lastItem, hasLastItem := player.CreationMenuLastItem()
	sender.SendCreationMenu(menu, player.ItemName, lastItem, hasLastItem)
}

// isInstant reports whether a recipe makes one set per click:
// darts and bolts while the "Make-X for darts and bolts"
// setting is off (its varbit 0), as every rsprox capture
// shows (Shared.Fletching.js:597-602).
func (f *fletching) isInstant(player *game.Player, recipe *Recipe) bool {
	return recipe.HasInstantVarbit && getVarbit(player, recipe.InstantVarbitID) == 0
}

// missingToolMessage is the missing-tool line for a recipe
// (Shared.Fletching.js:604-608): its own line, else the
// generic "You'll need a <tool> to do that." with the first
// held tool's name, else the tool name.
func missingToolMessage(recipe *Recipe) string {
	if recipe.MissingTool != "" {
		return recipe.MissingTool
	}
	tool := recipe.Tool
	if len(recipe.Held) > 0 {
		tool = recipe.Held[0].Name
	}
	return fmt.Sprintf("You'll need a %s to do that.", tool)
}

// handlePair handles item on item: the recipes the pair starts.
// It returns true when Fletching took the click, in exact TS
// order (Shared.Fletching.js:616-664): no recipes for the pair
// → "Nothing interesting happens." only if the pair is known,
// else return false; unlocked filter → none → "Nothing
// interesting happens."; missing-tool message only when ALL
// choices lack tools; usable filter (tools + batches > 0);
// empty → preset-only check → PRESET_TEXT or silent (empty
// spool); single usable recipe → instant path (as many as
// actionsPerCycle in one click) or 1-batch fast path;
// otherwise the menu.
func (f *fletching) handlePair(player *game.Player, usedID, usedWithID int) bool {
	if player == nil {
		return false
	}
	all := f.recipesFor(usedID, usedWithID)
	if len(all) == 0 {
		if !f.hasPair(usedID, usedWithID) {
			return false
		}
		sendMessage(player, "Nothing interesting happens.")
		return true
	}
	var choices []*Recipe
	for _, recipe := range all {
		if f.unlocked(player, recipe) {
			choices = append(choices, recipe)
		}
	}
	if len(choices) == 0 {
		sendMessage(player, "Nothing interesting happens.")
		return true
	}
	var missing *Recipe
	for _, recipe := range choices {
		if !f.hasTools(player, recipe) {
			missing = recipe
			break
		}
	}
	if missing != nil {
		allLackTools := true
		for _, recipe := range choices {
			if f.hasTools(player, recipe) {
				allLackTools = false
				break
			}
		}
		if allLackTools {
			sendMessage(player, missingToolMessage(missing))
			return true
		}
	}
	var usable []*Recipe
	for _, recipe := range choices {
		if f.hasTools(player, recipe) && f.batchesAvailable(player, recipe) > 0 {
			usable = append(usable, recipe)
		}
	}
	// An empty spool on an unstrung bow does nothing at all,
	// not even "Nothing interesting happens."
	if len(usable) == 0 {
		everyCopy := func(inventory *game.Inventory, id int) int {
			if inventory == nil {
				return 0
			}
			return inventory.GetAmount(id)
		}
		onlyPreset := false
		for _, recipe := range choices {
			if !f.hasTools(player, recipe) {
				continue
			}
			allHeld := true
			for _, material := range recipe.Materials {
				if f.heldAmount(player, material, everyCopy) < material.Amount {
					allHeld = false
					break
				}
			}
			if allHeld {
				onlyPreset = true
				break
			}
		}
		if onlyPreset {
			sendMessage(player, presetText)
		}
		return true
	}
	via := usedID
	prov := &provenance{startLocation: player.Location()}

	if len(usable) == 1 {
		recipe := usable[0]
		if recipe.Ticks == 0 || f.isInstant(player, recipe) {
			f.stopFletching(player, false)
			// The instant path (darts and bolts by default)
			// makes as many as content asks for in the one click.
			actions := f.actionsPerCycle(player, recipe)
			for done := 0; done < actions && f.batchesAvailable(player, recipe) > 0; done++ {
				if !f.makeBatch(player, recipe, via, prov) {
					break
				}
			}
			return true
		}
		if f.batchesAvailable(player, recipe) == 1 {
			f.startFletching(player, recipe, 1, via, prov)
			return true
		}
	}
	f.openMenu(player, usable, via, prov)
	return true
}
