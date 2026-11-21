// Package woodcutting is the Go port of the Woodcutting skill plugin
// (server/plugins/skills/Woodcutting.plugin.js, 1,594 lines, commit
// bfd81e3): the core chopping loop — sessions, cut chance, axes,
// felling rations, the lumberjack bonus, the Seers maple diary
// bonus, Kandarin headgear, bird nests, Forestry despawn timers,
// depletion to stumps with respawn overlays, and the logs-chopped
// tracker. Tables live in tables.go (Worker A); this file owns the
// loop that consumes them. Line numbers in comments cite
// Woodcutting.plugin.js unless another file is named.
//
// The port is faithful: same plugin name, description, and members
// flag, same attach-only register, same guard order in
// startWoodcutting and processWoodcuttingTick, same success-block
// order, same depletion rules, same nest tables. Deviations forced
// by missing Go seams are named inline and in the report; nothing
// is silently invented.
//
// Event inventory:
//
//   - Listens to onObjectInteraction (named, 27 registrations over
//     the tree names/actions — normal tree 4 names x 2 actions, 19
//     others x 1; scrapey has no object names), produced by the
//     object-action listener through the cmd/server adapter.
//   - Listens to "woodcutting:chop" / "woodcutting:axe" /
//     "woodcutting:group-bonus" / "woodcutting:logs-chopped"
//     (custom-event queries/requests from other content —
//     farming patches, Trouble Brewing, the dramen tree; no Go
//     emitter exists yet, so these are answer-only).
//   - Emits "woodcutting:validate-tree" (query, allow defaults
//     true), "skills:action-interval" (via skillactions),
//     "woodcutting:attempt" (query around the roll),
//     "woodcutting:deplete-tree" (query, handled defaults false),
//     "woodcutting:nest-chance" / "woodcutting:nest-drop" /
//     "woodcutting:nest-search" (league fragments; no listeners),
//     "woodcutting:success" (fire-and-forget; Guild listens —
//     deferred), "pets:skilling-roll" (no answerer),
//     "diary:is-complete" (no answerer), "storage:room" /
//     "storage:gather" / "storage:amount" / "storage:take" (no
//     storage content), "skillcape:has-perk" (no answerer —
//     game.SkillcapePerkRequest, the GracefulOutfit precedent).
//     Every query degrades to its documented default when
//     unanswered.
//   - Listens to "level-up:popup-opened" via
//     skillactions.PauseForLevelUp; the emitter (SkillLevelUp
//     feedback content) is NOT STARTED, so the pause never fires
//     yet and level-up currently does NOT pause chopping.
//   - Listens to "equipment:effects" for the Lumberjack set text
//     (GracefulOutfit precedent); the producer (EquipmentStats)
//     is NOT STARTED, so the handler never fires.
//   - Listens to onPlayerLogin (logs-chopped varp re-send) and
//     onPlayerDisconnect (stop session, no animation reset).
//   - "player:emote" / "player:interaction-cleared" are NOT
//     registered: no Go emitter exists for either event.
//   - The onItemFirstAction registration for nest searching is
//     the coordinator's integration step (Worker C builds the
//     family); this package exposes SearchBirdNest for it and
//     does not invent the family.
//
// State lives in a *woodcutting struct created per Plugin value —
// NO package-level mutable maps (the Phase 21 race fix): the
// shared instance behind Plugin is the only one, and tests build
// their own. The tick task owns the tick counter.
//
// Deferred units (stubs named inline, never built here): Guild
// (isInGuild false, invisibleBoost 0). CrystalAxe and
// InfernalAxe are ported (crystalaxe.go, infernalaxe.go);
// the infernal burn never fires yet because Firemaking's
// "firemaking:log-xp" answerer is unported — see
// infernalaxe.go.
// EntTrunk and Shrine landed in Phase 33 (enttrunk.go,
// shrine.go, guilddata.go). ThickVines and ClueNests landed in
// Phase 33 (thickvines.go, cluenests.go).
package woodcutting

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- loop constants (Woodcutting.plugin.js:55-74, 125-128) ----
// The data constants live in tables.go; these are the loop's own
// literals, kept here with the code that reads them.

const (
	// spamChatType is the chat channel for chop lines: the spam
	// channel (protocol.ChatTypeSpam, 105 — the game package
	// cannot import protocol, so the value is spelled out with
	// its provenance, as dateminutes spells out its varp id).
	spamChatType = 105
)

// ---- item ids resolved outside the tables ----
// These three ids belong to units Worker A did not table
// (KandarinHeadgear, the ration, the rabbit foot); each is
// grounded in upstream ItemIdentifiers.ts and cross-checked
// against the real rev-242 seed cache (see the per-const notes).

const (
	// forestersRationID is ItemIds.FORESTERS_RATION
	// (ItemIdentifiers.ts:26088); the cache gameval
	// "forestry_ration" resolves to the same id (probe,
	// 2026-11-20).
	forestersRationID = 28157
	// strungRabbitFootID is ItemIds.STRUNG_RABBIT_FOOT
	// (ItemIdentifiers.ts:8842); the cache gameval
	// "hunting_strung_rabbit_foot" resolves to the same id
	// (probe, 2026-11-20).
	strungRabbitFootID = 10132
	// redBirdEggID / blueBirdEggID / greenBirdEggID are
	// ItemIds.BIRDS_EGG / BIRDS_EGG_2 / BIRDS_EGG_3
	// (ItemIdentifiers.ts:4575-4577); the cache gamevals
	// "bird_egg_red" / "bird_egg_blue" / "bird_egg_green"
	// resolve to the same ids (probe, 2026-11-20).
	redBirdEggID   = 5076
	blueBirdEggID  = 5077
	greenBirdEggID = 5078
)

// kandarinHeadgearIDs are ItemIds.KANDARIN_HEADGEAR_1..4
// (ItemIdentifiers.ts:11601-11604). The cache knows these ids
// under the renamed gamevals "seers_headband_easy/medium/hard/
// elite" (probe, 2026-11-20) — the "kandarin_headgear_N"
// gameval names do not exist, so the ItemIdentifiers ids are
// the grounding, not a name lookup.
var kandarinHeadgearIDs = []int{13137, 13138, 13139, 13140}

// ---- custom-event names ----

const (
	validateTreeEventName  = "woodcutting:validate-tree"
	chopEventName          = "woodcutting:chop"
	axeEventName           = "woodcutting:axe"
	groupBonusEventName    = "woodcutting:group-bonus"
	attemptEventName       = "woodcutting:attempt"
	depleteTreeEventName   = "woodcutting:deplete-tree"
	nestChanceEventName    = "woodcutting:nest-chance"
	nestDropEventName      = "woodcutting:nest-drop"
	nestSearchEventName    = "woodcutting:nest-search"
	successEventName       = "woodcutting:success"
	logsChoppedEventName   = "woodcutting:logs-chopped"
	diaryCompleteEventName = "diary:is-complete"
	storageRoomEventName   = "storage:room"
	storageGatherEventName = "storage:gather"
	storageAmountEventName = "storage:amount"
	storageTakeEventName   = "storage:take"
	petsSkillingRollEvent  = "pets:skilling-roll"
	skillcapeHasPerkEvent  = game.SkillcapeHasPerkEventName
	// toolEventName is the "woodcutting:tool" fallback query
	// (plugin.js:731-732): content may stand in for the axe (a
	// league relic's bare hands). It is a different event from
	// "woodcutting:axe" (the requestedAxe listener); do not
	// confuse the two.
	toolEventName = "woodcutting:tool"
)

// ---- custom-event payloads ----
// Each mirrors the TS request object of the same name; a handler
// answers by mutating the payload in place, exactly as TS does.

// ValidateTreeRequest is the "woodcutting:validate-tree" query
// (plugin.js:1245, 1353): content may refuse the tree by
// setting Allow false; it defaults true.
type ValidateTreeRequest struct {
	Player *game.Player
	Object plugins.GameObject
	Allow  bool
}

// ChopRequest is the "woodcutting:chop" request (plugin.js:1489):
// another plugin asks this one to chop a tree (farming patches,
// Trouble Brewing, the dramen tree). RemoveOnly marks a
// no-log/no-XP chop (farming fruit-tree patches); Handled
// reports whether chopping started.
type ChopRequest struct {
	Player     *game.Player
	Object     plugins.GameObject
	LogID      int
	RemoveOnly bool
	Handled    bool
}

// AxeRequest is the "woodcutting:axe" request (plugin.js:1502):
// ItemID nil asks for the player's best usable axe; otherwise
// the named axe, when usable. Axe answers nil when there is none.
type AxeRequest struct {
	Player *game.Player
	ItemID *int
	Axe    *Axe
}

// ToolRequest is the "woodcutting:tool" fallback query
// (plugin.js:730-732): content may stand in for the axe when
// the player holds no usable one. Axes carries the best-first
// list; Axe answers nil when nothing stands in.
type ToolRequest struct {
	Player *game.Player
	Axes   []Axe
	Axe    *Axe
}

// GroupBonusRequest is the "woodcutting:group-bonus" query
// (plugin.js:786): Players answers the choppers at the player's
// tree, 0 when they are not chopping a tree that gives one.
type GroupBonusRequest struct {
	Player  *game.Player
	Players int
}

// LogsChoppedRequest is the "woodcutting:logs-chopped" query
// (plugin.js:942): Count answers the player's logs-chopped
// total, 0 by default.
type LogsChoppedRequest struct {
	Player *game.Player
	Count  int
}

// AttemptRequest is the "woodcutting:attempt" query
// (plugin.js:1398): content may turn a miss into a hit (a
// league relic's second chance) by setting Success.
type AttemptRequest struct {
	Player  *game.Player
	Object  plugins.GameObject
	LogID   int
	Success bool
}

// DepleteTreeRequest is the "woodcutting:deplete-tree" query
// (plugin.js:1195): content may keep the tree standing (a
// league relic's endless trees) by setting Handled true.
type DepleteTreeRequest struct {
	Player       *game.Player
	Object       plugins.GameObject
	RespawnTicks int
	Handled      bool
}

// NestChanceRequest is the "woodcutting:nest-chance" query
// (plugin.js:1042): content may make nests commoner by raising
// Multiplier above 1; it defaults 1.
type NestChanceRequest struct {
	Player     *game.Player
	Multiplier float64
}

// NestDropRequest is the "woodcutting:nest-drop" query
// (plugin.js:1056): content may take the nest before it falls
// (a league fragment's banked nests) by setting Handled true;
// the log still comes then.
type NestDropRequest struct {
	Player  *game.Player
	ItemID  int
	Handled bool
}

// NestSearchEvent is the "woodcutting:nest-search" emit
// (plugin.js:1100): the seed taken from a searched nest.
type NestSearchEvent struct {
	Player *game.Player
	ItemID int
}

// WoodcuttingSuccessEvent is the "woodcutting:success" emit
// (plugin.js:1432): a successful chop — the skill, the pet
// roll, the log, and the axe. LogAmount is 0 on a felling
// skip, 1 on a burnt log, and the headgear-doubled amount
// otherwise.
type WoodcuttingSuccessEvent struct {
	Player    *game.Player
	Skill     int
	PetBase   int
	PetRolled bool
	LogID     int
	Location  game.Location
	LogAmount int
	AxeID     int
}

// DiaryCompleteRequest is the "diary:is-complete" query
// (plugin.js:814): Complete answers whether the player has the
// diary tier done; it defaults false.
type DiaryCompleteRequest struct {
	Player   *game.Player
	Diary    string
	Tier     string
	Complete bool
}

// StorageRoomRequest is the "storage:room" query
// (plugin.js:81): Room answers whether an open log basket has
// room for the item; it defaults false.
type StorageRoomRequest struct {
	Player *game.Player
	ItemID int
	Skill  int
	Room   bool
}

// StorageGatherRequest is the "storage:gather" request
// (plugin.js:89): storage takes the logs first; Stored answers
// how many it took, and the remainder is the log item.
type StorageGatherRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Stored int
	Skill  int
	XP     float64
}

// StorageAmountRequest is the "storage:amount" query
// (plugin.js:856): Amount carries the inventory count in and
// the total (inventory + storage) out.
type StorageAmountRequest struct {
	Player *game.Player
	ItemID int
	Amount int
}

// StorageTakeRequest is the "storage:take" request
// (plugin.js:874): Taken answers how many of Amount storage
// handed over.
type StorageTakeRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Taken  int
}

// SkillingRollRequest is the "pets:skilling-roll" query
// (plugin.js:1429): Handled answers whether a pet was rolled
// for; no answerer exists in Go.
type SkillingRollRequest struct {
	Player  *game.Player
	Skill   int
	PetBase int
	Handled bool
}

// ---- session state ----

// session is one player's chopping session (the per-player
// object Woodcutting.plugin.js keeps in its activeSessions Map,
// plugin.js:1292-1301). It implements skillactions.Session, so
// PauseForLevelUp can hold it through a level-up pop-up; it
// must stay a pointer (comparable) value for the helper's
// identity checks.
type session struct {
	tree              *Tree
	axe               Axe
	objectID          int
	location          game.Location
	area              *game.PrivateArea
	objType           int
	objFace           int
	nextActionTick    int64
	nextAnimationTick int64
	requested         bool
	levelUpPaused     bool
}

// SetLevelUpPaused marks the session held by a level-up pop-up
// (skillactions.Session).
func (s *session) SetLevelUpPaused(paused bool) {
	if s != nil {
		s.levelUpPaused = paused
	}
}

// despawnTimer is one Forestry despawn timer
// (plugin.js:947-975): it counts down one per tick while anyone
// chops the tree and regenerates one per tick while nobody does,
// and is deleted at full.
type despawnTimer struct {
	remaining    int
	max          int
	lastChopTick int64
}

// woodcutting is the plugin's per-registration state: the TS
// module-level state (activeSessionsRef, treeDespawnTimers,
// woodcuttingTick, pluginApi) as struct fields, per the Phase 21
// race fix. One instance serves the registered Plugin value;
// tests build their own with newWoodcutting.
type woodcutting struct {
	tables *Tables
	// regularLogID is the normal tree's log id (ItemIds.LOGS):
	// Kandarin headgear doubles only regular logs.
	regularLogID int
	// api is the Register-time Api, held so the hub surfaces
	// that install after plugin Load (TaskManager, ObjectManager)
	// can be resolved lazily at use time through taskManager /
	// objectManager. Nil in unit tests, which set tasks and
	// objects directly.
	api  *plugins.Api
	emit func(name string, payload any)
	// tasks and objects are direct overrides (unit tests set
	// them); production leaves them nil and resolves through
	// the api instead — a Register-time capture would read nil,
	// because both surfaces install after Load.
	tasks   *game.TaskManager
	objects plugins.ObjectManagerAPI
	// taskScheduled guards ensureTask: the 1-tick global task
	// schedules once, on the first chop.
	taskScheduled bool
	// vines is the ThickVines unit's per-registration state
	// (pending clearings, cached axe map, rake id); see
	// thickvines.go.
	vines *thickVines
	// wilderness is the *game.Wilderness the clue-nest ring check
	// reads (resolveWilderness, cluenests.go): the unit-test
	// override; production leaves it nil and resolves lazily
	// through the Api's AreaManager.
	wilderness *game.Wilderness
	// membersWorld defaults true: the TS no-world fixture path
	// allows everything, and the dev world is a members world.
	// cmd/server installs the real flag through
	// SetMembersWorld before hub Load.
	membersWorld bool
	// isMembersItem answers an item definition's members flag;
	// nil means every item counts. cmd/server installs it
	// through SetIsMembersItem before hub Load, backed by the
	// loaded cache definitions.
	isMembersItem func(id int) bool
	sessions      map[*game.Player]*session
	despawnTimers map[string]*despawnTimer
	tick          int64
	// ent is the Ent trunk sub-unit (EntTrunk.Woodcutting.js,
	// Phase 33, Worker A) and shrine is the shrine sub-unit
	// (Shrine.Woodcutting.js, Phase 33, Worker A): each holds
	// its own state and registers its own hooks through
	// Register below.
	ent       *entTrunk
	shrine    *entShrine
	randFloat func() float64
	randInt   func(min, max int) int
}

// newWoodcutting builds a woodcutting instance over tables (nil
// tables degrade every handler to a no-op; see Register).
func newWoodcutting(tables *Tables) *woodcutting {
	w := &woodcutting{
		tables:        tables,
		sessions:      make(map[*game.Player]*session),
		despawnTimers: make(map[string]*despawnTimer),
		vines:         newThickVines(),
		membersWorld:  true,
		randFloat:     func() float64 { return rand.Float64() },
		randInt: func(min, max int) int {
			if max <= min {
				return min
			}
			return min + rand.IntN(max-min+1)
		},
	}
	if tables != nil {
		w.regularLogID = regularLogID(tables)
	}
	w.ent = newEntTrunk(w)
	w.shrine = newEntShrine(w)
	return w
}

// regularLogID finds the normal tree's log id: the regular log
// Kandarin headgear doubles (KandarinHeadgear.Woodcutting.js:
// logsPerChop compares against ItemIds.LOGS).
func regularLogID(tables *Tables) int {
	for _, tree := range tables.Trees {
		if tree.Name == "normal tree" {
			return tree.LogID
		}
	}
	return 0
}

// shared is the instance behind Plugin; SetTables and
// SearchBirdNest operate on it.
var shared = newWoodcutting(nil)

// SetTables installs the resolved tables for the registered
// Plugin value. There is no CacheDefinitions accessor on Api,
// so the coordinator's integration step must call this with
// LoadFromCache's result BEFORE hub Load: Register captures
// the shared instance, so a call before Load reaches every
// handler; a call after Load is too late for the object
// registrations. With no tables installed, every handler
// degrades to a no-op (handleChop claims nothing, the tick
// task runs empty).
func SetTables(t *Tables) {
	shared.tables = t
	if t != nil {
		shared.regularLogID = regularLogID(t)
	} else {
		shared.regularLogID = 0
	}
}

// SetMembersWorld installs the members-world flag for the
// registered Plugin value (TS: pluginApi.core.WorldDefinition.
// isMembersWorld()). cmd/server calls it before hub Load with
// the world definition's flag; it defaults true (the TS
// no-world fixture path allows everything, and the dev world
// is a members world).
func SetMembersWorld(members bool) {
	shared.membersWorld = members
}

// SetIsMembersItem installs the item-members lookup for the
// registered Plugin value (TS: CacheDefinitions.getItem(id)?.
// isMembers). cmd/server calls it before hub Load with a
// closure over the loaded cache definitions; nil means every
// item counts (the TS no-world fixture path).
func SetIsMembersItem(fn func(id int) bool) {
	shared.isMembersItem = fn
}

// SearchBirdNest searches a bird's nest in the player's
// inventory (see searchBirdNest): Register wires it through
// api.OnItemFirstAction (plugin.js:1560-1566). It delegates to
// the shared instance, so it needs SetTables first; with no
// tables it returns false.
func SearchBirdNest(player *game.Player, nestID int) bool {
	return shared.searchBirdNest(player, nestID)
}

// plugin builds the registered Plugin value: name, description,
// and members flag verbatim (module.exports, plugin.js:1557),
// attach-only register.
func (w *woodcutting) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Woodcutting",
		Description: "Woodcutting: chopping trees with the Forestry despawn timer, stump respawns, the logs-chopped tracker, and bird's nest and ring drops.",
		Members:     false,
		Register: func(api *plugins.Api) {
			w.api = api
			w.emit = api.EmitCustomEvent
			// The 1-tick global task is NOT scheduled here, and
			// NOT on server startup either: the TaskManager
			// installs on the hub after plugin Load (nil until
			// installed, per the hub contract — see taskManager),
			// so a Register-time capture reads nil; and a startup
			// hook would call Schedule off the tick goroutine
			// (Server.Start emits startup after the engine is
			// already ticking), which the TaskManager forbids —
			// Schedule/Process must run on the tick goroutine.
			// The task schedules once on the first chop instead
			// (ensureTask, from startWoodcutting, which runs on
			// the tick goroutine through the turn-packet drain).
			api.OnCustomEvent(chopEventName, w.requestedChop)
			api.OnCustomEvent(axeEventName, w.requestedAxe)
			api.OnCustomEvent(groupBonusEventName, w.answerGroupBonus)
			api.OnPlayerDisconnect(w.onPlayerDisconnect)
			api.OnCustomEvent(skillactions.LevelUpPopupOpenedEventName, w.pauseWoodcutting)
			api.PersistAttribute(LogsChoppedAttribute, 0)
			api.OnPlayerLogin(w.onPlayerLogin)
			api.OnCustomEvent(logsChoppedEventName, w.answerLogsChopped)
			// Ent trunks and the shrine register their own
			// hooks (Phase 33, Worker A): onNpcDeath /
			// onNpcClick / onPlayerDisconnect for the trunks,
			// the persisted offerings attribute and the named
			// item-on-object hook for the shrine.
			w.ent.register(api)
			w.shrine.register(api)
			api.OnCustomEvent(game.EquipmentEffectsEventName, w.addLumberjackSetText)
			// Bird's nests: the first action searches the nest
			// (plugin.js:1560-1566). searchBirdNest returns
			// false for any other item, so the click falls
			// through to the next handler.
			api.OnItemFirstAction(func(event *plugins.PluginItemActionEvent) bool {
				return w.searchBirdNest(event.Player, event.ItemID)
			})
			// Crystal and infernal axes: the Check actions and
			// the shard recharge (CrystalAxe.Woodcutting.js,
			// InfernalAxe.Woodcutting.js).
			w.registerCrystalAxe(api)
			w.registerInfernalAxe(api)
			// Object registrations: one onObjectInteraction call
			// per (name, action) pair (plugin.js:1543-1549) —
			// normal tree 4 names x 2 actions, 19 others x 1;
			// scrapey has no object names. The count is pinned
			// by TestObjectRegistrationCount.
			if w.tables != nil {
				for _, tree := range w.tables.Trees {
					for _, name := range tree.ObjectNames {
						for _, action := range tree.Actions {
							api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{action: w.handleChop})
						}
					}
				}
			}
			// Sub-units (attach-only, in TS attach order):
			// ThickVines (plugin.js:1541, ThickVines.attach) and
			// ClueNests (plugin.js:1541, ClueNests.attach). Each
			// registers its own hooks; see thickvines.go and
			// cluenests.go.
			w.registerThickVines(api)
			w.registerClueNests(api)
		},
	}
}

// Plugin is the registered Woodcutting plugin value.
var Plugin = shared.plugin()

func init() {
	plugins.Register(Plugin)
}

// ---- small helpers ----

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract — cmd/server installs it from the built world),
// so a Register-time capture reads nil in production. The live
// Api value wins when present; the directly-set tasks field is
// the unit-test override.
func (w *woodcutting) taskManager() *game.TaskManager {
	if w.api != nil {
		if tm := w.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return w.tasks
}

// objectManager resolves the ObjectManagerAPI lazily, same
// contract as taskManager: the hub installs it after plugin
// Load (cmd/server, from the built world's registry). The live
// Api value wins when present; the directly-set objects field
// is the unit-test override.
func (w *woodcutting) objectManager() plugins.ObjectManagerAPI {
	if w.api != nil {
		if m := w.api.GetObjectManager(); m != nil {
			return m
		}
	}
	return w.objects
}

// ensureTask schedules the 1-tick global WoodcuttingTask once,
// on the lazily-resolved TaskManager. Idempotent: a second call
// (a second chop) schedules nothing. A nil TaskManager
// schedules nothing and retries on the next call. Callers run
// on the tick goroutine (startWoodcutting, through the
// turn-packet drain), which is the only goroutine the
// TaskManager allows Schedule from.
func (w *woodcutting) ensureTask() {
	if w.taskScheduled {
		return
	}
	tm := w.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		w.tick++
		w.processWoodcuttingTick(w.tick)
		w.regenerateTreeDespawnTimers(w.tick)
		return false
	})
	task.Name = "WoodcuttingTask"
	tm.Schedule(task)
	w.taskScheduled = true
}

// emitEvent emits a custom event through the captured emit func;
// a nil emit (before Register) drops the emit, which is the
// no-hooks default for every query.
func (w *woodcutting) emitEvent(name string, payload any) {
	if w == nil || w.emit == nil {
		return
	}
	w.emit(name, payload)
}

// privateAreaOf returns the instanced area a plugins.GameObject
// belongs to, or nil for a shared-world object (or an object
// whose concrete type is not *game.GameObject — the listener
// supplies *game.GameObject in production).
func privateAreaOf(obj plugins.GameObject) *game.PrivateArea {
	if concrete, ok := obj.(*game.GameObject); ok && concrete != nil {
		return concrete.PrivateArea()
	}
	return nil
}

// woodcuttingLevel reads the player's current Woodcutting level
// (getWoodcuttingLevel, plugin.js:692). A nil player or missing
// skill manager reads 0 (Go nil-safety; TS never calls with a
// nil player).
func woodcuttingLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	skills := player.Skills()
	if skills == nil {
		return 0
	}
	return skills.CurrentLevel(game.SkillWoodcutting)
}

// skillLevel reads another skill's current level, or 0 without
// a skill manager.
func skillLevel(player *game.Player, skill int) int {
	if player == nil {
		return 0
	}
	skills := player.Skills()
	if skills == nil {
		return 0
	}
	return skills.CurrentLevel(skill)
}

// isWearing reports whether the player's equipment slot holds
// one of the ids (isWearing, plugin.js:1002; TS reads the slot
// item and tests inclusion).
func isWearing(player *game.Player, slot int, itemIDs []int) bool {
	if player == nil {
		return false
	}
	equipment := player.Equipment()
	if equipment == nil {
		return false
	}
	id := equipment.ItemAt(slot).ID()
	for _, want := range itemIDs {
		if id == want {
			return true
		}
	}
	return false
}

// sendSpam sends a filterable chatbox line (sendSpam,
// plugin.js:929: sendTypedMessage(ChatType.SPAM, text)).
func (w *woodcutting) sendSpam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(spamChatType, text, "")
}

// dropGroundItem drops an item at the player's tile through the
// engine ground-item seam: game.Hooks.DropGroundItem, installed
// by the cmd/server adapter onto the GroundItemManager. There
// is NO Api.GetItemOnGroundManager — the hook is the only
// content-reachable ground-item path, and a nil hook drops the
// item silently (the seam's documented gap).
func (w *woodcutting) dropGroundItem(player *game.Player, item game.Item) {
	if player == nil || game.Hooks.DropGroundItem == nil {
		return
	}
	game.Hooks.DropGroundItem(item, player.Location(), player)
}

// logsChoppedCount reads the player's logs-chopped attribute
// (answerLogsChopped's read, plugin.js:943): an int attribute,
// 0 when absent or of another type.
func logsChoppedCount(player *game.Player) int {
	if player == nil {
		return 0
	}
	switch v := player.GetAttribute(LogsChoppedAttribute).(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ---- cut chance ----

// calculateCutChance returns the per-roll log chance for a
// level, tree, and axe (calculateCutChance, plugin.js:758-767):
// successes = 1 + floor(low*(99-L)/98) + floor(high*(L-1)/98)
// (rounded variant for scrapey only), chance = successes/256
// clamped [0,1]. The row comes from CutChanceRow, which scales
// a null row off the normal chart (cutChanceRange,
// plugin.js:748-756). Float math reproduces TS Math.floor /
// Math.round exactly, including the negative-numerator edge
// past level 99 (boosted levels), where Go integer division
// would truncate toward zero instead of flooring.
func calculateCutChance(level int, tree *Tree, axe *Axe) float64 {
	if tree == nil || axe == nil {
		return 0
	}
	row := CutChanceRow(tree.CutChance, axe.Tier)
	if !row.Has {
		return 0
	}
	var successes float64
	if tree.RoundedCutChance {
		successes = math.Round(float64(row.Low*(99-level)+row.High*(level-1)) / 98)
	} else {
		successes = 1 + math.Floor(float64(row.Low*(99-level))/98) + math.Floor(float64(row.High*(level-1))/98)
	}
	return math.Min(1, math.Max(0, successes/256))
}

// rollLog rolls one log (rollLog, plugin.js:799-802): the
// effective level is the current Woodcutting level plus the
// guild boost (stub 0 — Guild deferred) plus the group bonus.
func (w *woodcutting) rollLog(player *game.Player, tree *Tree, axe *Axe, state *session) bool {
	level := woodcuttingLevel(player) + invisibleBoost(player) + w.groupBonus(player, state)
	return w.randFloat() < calculateCutChance(level, tree, axe)
}

// groupBonus is the Forestry group bonus (groupBonus,
// plugin.js:777-793): +1 per chopper on the same object id +
// location + private area (including self), capped at 10; 0 for
// requested sessions, normal trees, or in-guild players (the
// guild stub never reports in-guild, so the exclusion never
// fires). A level-up dialogue holds the session without
// chopping, so a paused OTHER chopper does not count; the
// player themself counts even while paused.
func (w *woodcutting) groupBonus(player *game.Player, state *session) int {
	if state == nil || state.requested || state.tree == nil || state.tree.Name == "normal tree" || isInGuild(player) {
		return 0
	}
	chopping := 0
	for other, otherSession := range w.sessions {
		if otherSession == nil || otherSession.requested {
			continue
		}
		if otherSession.objectID != state.objectID || otherSession.area != state.area {
			continue
		}
		if !otherSession.location.Equals(state.location) {
			continue
		}
		if other == nil || !other.IsRegistered() {
			continue
		}
		if otherSession.levelUpPaused && other != player {
			continue
		}
		chopping++
	}
	return min(GroupBonusMax, chopping)
}

// answerGroupBonus answers the "woodcutting:group-bonus" query
// (answerGroupBonus, plugin.js:791-793).
func (w *woodcutting) answerGroupBonus(payload any) {
	request, ok := payload.(*GroupBonusRequest)
	if !ok || request == nil {
		return
	}
	var state *session
	if request.Player != nil {
		state = w.sessions[request.Player]
	}
	request.Players = w.groupBonus(request.Player, state)
}

// ---- axes ----

// canUseAxe reports whether the player may use the axe
// (canUseAxe, plugin.js:714-721): Woodcutting >= required, and
// Agility / Firemaking >= the axe's requirement (absent reads
// 1, the TS `?? 1` default).
func canUseAxe(player *game.Player, axe *Axe) bool {
	if axe == nil {
		return false
	}
	reqAgility := axe.RequiredAgility
	if reqAgility < 1 {
		reqAgility = 1
	}
	reqFiremaking := axe.RequiredFiremaking
	if reqFiremaking < 1 {
		reqFiremaking = 1
	}
	return woodcuttingLevel(player) >= axe.RequiredLevel &&
		skillLevel(player, game.SkillAgility) >= reqAgility &&
		skillLevel(player, game.SkillFiremaking) >= reqFiremaking
}

// choppingAxeEligible skips members axes on free worlds
// (choppingAxeEligible, plugin.js:1221-1227). The two halves
// of the TS check arrive as integration hooks: membersWorld
// from SetMembersWorld (world definition) and isMembersItem
// from SetIsMembersItem (loaded cache definitions), both
// installed by cmd/server before hub Load. With neither
// installed — the unit-test path — every axe counts, the TS
// behaviour on a members world (and the TS no-world fixture
// path, which also allows every axe).
func (w *woodcutting) choppingAxeEligible(axe *Axe) bool {
	if axe == nil {
		return false
	}
	if w.membersWorld {
		return true
	}
	if w.isMembersItem == nil {
		return true
	}
	return !w.isMembersItem(axe.ID)
}

// findBestUsableAxe finds the player's best usable axe
// (findBestUsableAxe, plugin.js:710-733): best-first over the
// axes, equipped weapon slot or inventory, with felling
// promotion (same tier + same level + rations available → the
// felling axe), and the "woodcutting:tool" fallback query when
// no axe is held (a league relic's bare hands; unanswered →
// nil). Infernal axes and no-ration promotions never promote:
// the TS guard skips promotion for non-felling axes with a
// Firemaking requirement, and for axes with no rations.
func (w *woodcutting) findBestUsableAxe(player *game.Player) *Axe {
	if w.tables == nil || player == nil {
		return nil
	}
	equippedWeaponID := -1
	if equipment := player.Equipment(); equipment != nil {
		equippedWeaponID = equipment.ItemAt(game.WeaponSlot).ID()
	}
	inventory := player.Inventory()
	holds := func(id int) bool {
		return equippedWeaponID == id || (inventory != nil && inventory.Contains(id))
	}
	for i := range w.tables.AxesBestFirst {
		axe := &w.tables.AxesBestFirst[i]
		if !holds(axe.ID) || !canUseAxe(player, axe) || !w.choppingAxeEligible(axe) {
			continue
		}
		if !axe.Felling && axe.RequiredFiremaking == 0 {
			for j := range w.tables.AxesBestFirst {
				candidate := &w.tables.AxesBestFirst[j]
				if candidate.Felling && candidate.Tier == axe.Tier && candidate.RequiredLevel == axe.RequiredLevel &&
					holds(candidate.ID) && canUseAxe(player, candidate) && w.choppingAxeEligible(candidate) {
					if w.availableFellingRations(player) > 0 {
						return candidate
					}
					break
				}
			}
		}
		return axe
	}
	request := &ToolRequest{Player: player, Axes: w.tables.AxesBestFirst}
	w.emitEvent(toolEventName, request)
	return request.Axe
}

// findBestUsableAxeByLevel returns the best standard axe for a
// bare Woodcutting level (findBestUsableAxeByLevel,
// plugin.js:736-744): bots only ever get the standard
// bronze-to-dragon axes.
func (w *woodcutting) findBestUsableAxeByLevel(level int, eligible func(*Axe) bool) *Axe {
	if w.tables == nil {
		return nil
	}
	if eligible == nil {
		eligible = func(*Axe) bool { return true }
	}
	for i := range w.tables.AxesBestFirst {
		axe := &w.tables.AxesBestFirst[i]
		if axe.Standard && level >= axe.RequiredLevel && eligible(axe) {
			return axe
		}
	}
	return nil
}

// ---- felling rations ----

// availableFellingRations counts the player's Forester's
// rations (availableFellingRations, plugin.js:851-857): the
// inventory count, with storage able to add to it through the
// "storage:amount" query (unanswered → the inventory count).
// Members only; 0 on a free world.
func (w *woodcutting) availableFellingRations(player *game.Player) int {
	if !w.membersWorld || player == nil {
		return 0
	}
	amount := 0
	if inventory := player.Inventory(); inventory != nil {
		amount = inventory.GetAmount(forestersRationID)
	}
	request := &StorageAmountRequest{Player: player, ItemID: forestersRationID, Amount: amount}
	w.emitEvent(storageAmountEventName, request)
	return request.Amount
}

// useFellingRation spends one ration for a felling-axe chop
// (useFellingRation, plugin.js:864-875): inventory first, else
// the "storage:take" query (unanswered → not consumed).
// Members only.
func (w *woodcutting) useFellingRation(player *game.Player, axe *Axe) bool {
	if axe == nil || !axe.Felling || !w.membersWorld || player == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory != nil && inventory.Contains(forestersRationID) {
		inventory.DeleteNumber(forestersRationID, 1)
		return true
	}
	request := &StorageTakeRequest{Player: player, ItemID: forestersRationID, Amount: 1, Taken: 0}
	w.emitEvent(storageTakeEventName, request)
	return request.Taken == 1
}

// fellingChop resolves a felling-axe chop (fellingChop,
// plugin.js:895-898): a spent ration adds +10% XP and skips
// the log 20% of the time. The cut chance and cadence do not
// change.
func (w *woodcutting) fellingChop(player *game.Player, axe *Axe) (bonus, skipLog bool) {
	bonus = w.useFellingRation(player, axe)
	skipLog = bonus && w.randFloat() < 0.2
	return bonus, skipLog
}

// rollForesterRationEnergy rolls the ration energy restore
// (rollForesterRationEnergy, plugin.js:881-893): 30%, +2000
// run-energy units when below 9000, members only. A felling
// axe's chop already ate its ration, so that ration restores
// the energy with its own line and no second one is spent; a
// regular axe spends one (inventory first, else storage).
func (w *woodcutting) rollForesterRationEnergy(player *game.Player, axe *Axe, fellingRationUsed bool) bool {
	if player == nil || axe == nil || !w.membersWorld {
		return false
	}
	sendEnergy := func() {
		player.AddRunEnergyUnits(2000)
		player.SendRunEnergy(player.GetRunEnergyUnits(), player.IsRunning(), player.GetWeightKg(), 0)
	}
	if axe.Felling {
		if !fellingRationUsed || player.GetRunEnergyUnits() >= 9000 || w.randFloat() >= 0.3 {
			return false
		}
		sendEnergy()
		player.SendMessage("The Forester's ration restores some of your energy.")
		return true
	}
	if defs := player.ItemDefinitions(); defs != nil && defs.IsDoubleHanded(axe.ID) {
		return false
	}
	holds := false
	if inventory := player.Inventory(); inventory != nil && inventory.Contains(axe.ID) {
		holds = true
	}
	if !holds {
		if equipment := player.Equipment(); equipment != nil && equipment.ItemAt(game.WeaponSlot).ID() == axe.ID {
			holds = true
		}
	}
	if !holds {
		return false
	}
	if player.GetRunEnergyUnits() >= 9000 || w.availableFellingRations(player) < 1 {
		return false
	}
	if w.randFloat() >= 0.3 {
		return false
	}
	inventory := player.Inventory()
	if inventory != nil && inventory.Contains(forestersRationID) {
		inventory.DeleteNumber(forestersRationID, 1)
	} else {
		request := &StorageTakeRequest{Player: player, ItemID: forestersRationID, Amount: 1, Taken: 0}
		w.emitEvent(storageTakeEventName, request)
		if request.Taken != 1 {
			return false
		}
	}
	sendEnergy()
	player.SendMessage("You consume a Forester's ration and feel energised.")
	return true
}

// ---- XP ----

// treeBaseXP is the tree's XP per log, with the diary reward a
// tier-complete player earns at that tree's site (treeBaseXp,
// plugin.js:805-818): the Kandarin medium diary pays +10% on
// maples inside Seers' Village. The "diary:is-complete" query
// is unanswered in Go, so no bonus lands in production.
func (w *woodcutting) treeBaseXP(player *game.Player, tree *Tree, location game.Location) float64 {
	if tree == nil {
		return 0
	}
	reward := tree.DiaryXP
	if reward == nil {
		return tree.XPReward
	}
	region := reward.Region
	if location.Z != region.Z || location.X < region.MinX || location.X > region.MaxX ||
		location.Y < region.MinY || location.Y > region.MaxY {
		return tree.XPReward
	}
	request := &DiaryCompleteRequest{Player: player, Diary: reward.Diary, Tier: reward.Tier, Complete: false}
	w.emitEvent(diaryCompleteEventName, request)
	if request.Complete {
		return tree.XPReward * float64(100+reward.Percent) / 100
	}
	return tree.XPReward
}

// lumberjackMultiplier is the Lumberjack XP multiplier
// (plugin.js:820-824): per-piece bonuses (Forestry pieces
// count) plus 0.5% for all four.
func (w *woodcutting) lumberjackMultiplier(player *game.Player) float64 {
	if w.tables == nil || player == nil {
		return 1
	}
	worn := 0
	bonus := 0.0
	for _, piece := range w.tables.LumberjackOutfit {
		if isWearing(player, piece.Slot, piece.ItemIDs) {
			bonus += piece.Bonus
			worn++
		}
	}
	if worn == len(w.tables.LumberjackOutfit) && worn > 0 {
		bonus += LumberjackSetBonus
	}
	return 1 + bonus
}

// isFullLumberjack reports whether the player wears the whole
// Lumberjack outfit (addLumberjackSetText's check,
// plugin.js:827-831).
func (w *woodcutting) isFullLumberjack(player *game.Player) bool {
	if w.tables == nil || player == nil {
		return false
	}
	for _, piece := range w.tables.LumberjackOutfit {
		if !isWearing(player, piece.Slot, piece.ItemIDs) {
			return false
		}
	}
	return len(w.tables.LumberjackOutfit) > 0
}

// addLumberjackSetText adds the full-outfit set text to an
// "equipment:effects" request (addLumberjackSetText,
// plugin.js:826-831; enum 3047, set-text 17). The producer
// (EquipmentStats) is NOT STARTED in Go, so this handler is
// registered but never fires — the GracefulOutfit precedent.
func (w *woodcutting) addLumberjackSetText(payload any) {
	request, ok := payload.(*game.EquipmentEffectsRequest)
	if !ok || request == nil || request.Player == nil {
		return
	}
	if w.isFullLumberjack(request.Player) {
		request.SetTexts = append(request.SetTexts, LumberjackSetText)
	}
}

// logsPerChop is the Kandarin headgear rule
// (KandarinHeadgear.Woodcutting.js: logsPerChop): two regular
// logs while headgear 1-4 is worn, else one. It is a members'
// benefit: a headgear saved as worn stays on a free world,
// where FreeToPlay only blocks equipping it — so the members
// check is on the world, not the item. No extra XP.
func (w *woodcutting) logsPerChop(player *game.Player, logID int) int {
	if logID != w.regularLogID || !w.membersWorld {
		return 1
	}
	if isWearing(player, game.HeadSlot, kandarinHeadgearIDs) {
		return 2
	}
	return 1
}

// ---- bird nests ----

// rollWeightedIndex returns the index of the entry a 1-based
// roll lands in (rollWeighted, plugin.js:996-1006): the roll
// walks the weights down and lands on the first entry that
// takes it to 0 or below; an over-total roll lands on the last
// entry, as in TS.
func rollWeightedIndex(weights []int, roll int) int {
	for i, weight := range weights {
		roll -= weight
		if roll <= 0 {
			return i
		}
	}
	return len(weights) - 1
}

// rollBirdNestID rolls which nest falls (rollBirdNestId,
// plugin.js:1013-1023): 100 slots — seed 65 (60 with a strung
// rabbit foot), ring 32, one per egg colour.
func (w *woodcutting) rollBirdNestID(player *game.Player) int {
	if w.tables == nil {
		return 0
	}
	seedWeight := 65
	if isWearing(player, game.AmuletSlot, []int{strungRabbitFootID}) {
		seedWeight = 60
	}
	weights := []int{seedWeight, 32, 1, 1, 1}
	ids := []int{
		w.tables.BirdNests.SeedNest,
		w.tables.BirdNests.RingNest,
		w.tables.BirdNests.RedEggNest,
		w.tables.BirdNests.GreenEggNest,
		w.tables.BirdNests.BlueEggNest,
	}
	return ids[rollWeightedIndex(weights, w.randInt(1, 100))]
}

// wearsWoodcuttingCape answers the Woodcutting cape perk query
// (wearsWoodcuttingCape, plugin.js:1026-1030): the
// "skillcape:has-perk" query, unanswered in Go, so Result stays
// false and nests are never 10% likelier in production.
func (w *woodcutting) wearsWoodcuttingCape(player *game.Player) bool {
	request := &game.SkillcapePerkRequest{Player: player, Skill: "WOODCUTTING", Result: false}
	w.emitEvent(skillcapeHasPerkEvent, request)
	return request.Result
}

// maybeDropBirdNest drops a bird's nest on a successful roll
// (maybeDropBirdNest, plugin.js:1032-1061): members only, 1/256
// per successful roll (x1.1 with the Woodcutting cape perk, x
// the "woodcutting:nest-chance" multiplier when over 1). The
// nest pre-empts the log entirely when it falls — no log, no
// log line, no XP — but clue-nest rolling and depletion still
// run. Returns true when a nest fell to the ground.
func (w *woodcutting) maybeDropBirdNest(player *game.Player) bool {
	if player == nil || w.tables == nil {
		return false
	}
	// Members only, as the felling rations are. (A fixture
	// with no world allows them, as choppingAxeEligible does:
	// membersWorld defaults true.)
	if !w.membersWorld {
		return false
	}
	boost := &NestChanceRequest{Player: player, Multiplier: 1}
	w.emitEvent(nestChanceEventName, boost)
	multiplier := boost.Multiplier
	if multiplier <= 1 {
		multiplier = 1
	}
	chance := 1.0 / BirdNestDropChance
	if w.wearsWoodcuttingCape(player) {
		chance = WoodcuttingCapeNestMultiplier / BirdNestDropChance
	}
	chance *= multiplier
	if w.randFloat() >= chance {
		return false
	}
	nestID := w.rollBirdNestID(player)
	drop := &NestDropRequest{Player: player, ItemID: nestID, Handled: false}
	w.emitEvent(nestDropEventName, drop)
	if drop.Handled {
		return false
	}
	w.dropGroundItem(player, game.NewItem(nestID, 1))
	game.SendSoundEffect(player, NestFallSound, 1, 0, 1)
	player.SendMessage("<col=ff0000>A bird's nest falls out of the tree.</col>")
	return true
}

// rollNestSeed rolls a seed from the nest table (rollNestSeed,
// plugin.js:1064-1066).
func (w *woodcutting) rollNestSeed() *NestSeed {
	if w.tables == nil || len(w.tables.NestSeeds) == 0 {
		return nil
	}
	weights := make([]int, len(w.tables.NestSeeds))
	total := 0
	for i, seed := range w.tables.NestSeeds {
		weights[i] = seed.Weight
		total += seed.Weight
	}
	return &w.tables.NestSeeds[rollWeightedIndex(weights, w.randInt(1, total))]
}

// rollNestRing rolls a ring from the nest table (rollNestRing,
// plugin.js:1068-1070).
func (w *woodcutting) rollNestRing() *NestRing {
	if w.tables == nil || len(w.tables.NestRings) == 0 {
		return nil
	}
	weights := make([]int, len(w.tables.NestRings))
	total := 0
	for i, ring := range w.tables.NestRings {
		weights[i] = ring.Weight
		total += ring.Weight
	}
	return &w.tables.NestRings[rollWeightedIndex(weights, w.randInt(1, total))]
}

// searchBirdNest searches a bird's nest (searchBirdNest,
// plugin.js:1072-1112): seed nests roll NEST_SEEDS (needs a
// free slot + addable), ring nests roll NEST_RINGS, egg nests
// give the matching bird's egg. The nest is consumed and an
// empty nest added; the seed line is a spam message, rings and
// eggs arrive silently. A full inventory shows the statement
// dialogue and keeps the nest. Returns false when the item is
// not a searchable nest.
func (w *woodcutting) searchBirdNest(player *game.Player, nestID int) bool {
	if player == nil || w.tables == nil || !w.tables.SearchableNestIDs[nestID] {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	freeSlots := inventory.GetFreeSlots()
	var seed *NestSeed
	if freeSlots > 0 && w.tables.SeedNestIDs[nestID] {
		seed = w.rollNestSeed()
	}
	if freeSlots <= 0 || (seed != nil && inventory.GetAddableAmount(game.NewItem(seed.ID, 1)) < 1) {
		if dm := player.GetDialogueManager(); dm != nil {
			dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, NestFullMessage)))
		}
		return true
	}
	inventory.DeleteNumber(nestID, 1)
	inventory.Adds(w.tables.BirdNests.EmptyNest, 1)
	if seed != nil {
		inventory.Adds(seed.ID, 1)
		w.emitEvent(nestSearchEventName, &NestSearchEvent{Player: player, ItemID: seed.ID})
		w.sendSpam(player, fmt.Sprintf("You take %s out of the bird's nest.", seed.Text))
		return true
	}
	if nestID == w.tables.BirdNests.RingNest {
		if ring := w.rollNestRing(); ring != nil {
			inventory.Adds(ring.ID, 1)
		}
		return true
	}
	var eggID int
	switch nestID {
	case w.tables.BirdNests.RedEggNest:
		eggID = redBirdEggID
	case w.tables.BirdNests.GreenEggNest:
		eggID = greenBirdEggID
	case w.tables.BirdNests.BlueEggNest:
		eggID = blueBirdEggID
	}
	if eggID != 0 {
		inventory.Adds(eggID, 1)
	}
	return true
}

// ---- inventory / log delivery ----

// inventoryBlocks reports whether a full inventory stops
// chopping (inventoryBlocks, plugin.js:76-85): a full inventory
// blocks unless an open log basket has room ("storage:room"
// query, room defaults false).
func (w *woodcutting) inventoryBlocks(player *game.Player, logID int) bool {
	if player == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil || !inventory.IsFull() {
		return false
	}
	request := &StorageRoomRequest{Player: player, ItemID: logID, Skill: game.SkillWoodcutting, Room: false}
	w.emitEvent(storageRoomEventName, request)
	return !request.Room
}

// prepareLog takes the log through storage first (prepareLog,
// plugin.js:88-92): storage takes what it can; the remainder
// is the log item, or nil when storage took it all (a -1 log id
// — a removeOnly chop — yields nothing).
func (w *woodcutting) prepareLog(player *game.Player, logID int, xp float64, amount int) *game.Item {
	if player == nil || logID < 0 {
		return nil
	}
	request := &StorageGatherRequest{Player: player, ItemID: logID, Amount: amount, Stored: 0, Skill: game.SkillWoodcutting, XP: xp}
	w.emitEvent(storageGatherEventName, request)
	if request.Stored >= amount {
		return nil
	}
	item := game.NewItem(logID, amount-request.Stored)
	return &item
}

// deliverLog gives the log to the player (deliverLog,
// plugin.js:94-99): into the inventory up to the addable
// amount, the overflow to the ground.
func (w *woodcutting) deliverLog(player *game.Player, log *game.Item) {
	if player == nil || log == nil {
		return
	}
	inventory := player.Inventory()
	if inventory == nil {
		w.dropGroundItem(player, *log)
		return
	}
	fits := min(inventory.GetAddableAmount(*log), log.Amount())
	if fits > 0 {
		inventory.Add(game.NewItem(log.ID(), fits), true)
	}
	if fits < log.Amount() {
		w.dropGroundItem(player, game.NewItem(log.ID(), log.Amount()-fits))
	}
}

// logMessage is the line for a cut log (logMessage,
// plugin.js:922-927): the tree's own line, else "You get some
// <log name>." with the name lower-cased.
func (w *woodcutting) logMessage(player *game.Player, tree *Tree) string {
	if tree == nil {
		return ""
	}
	if tree.LogMessage != "" {
		return tree.LogMessage
	}
	name := ""
	if player != nil {
		name = strings.ToLower(player.ItemName(tree.LogID))
	}
	return fmt.Sprintf("You get some %s.", name)
}

// sendInventoryFull sends the full-inventory refusal (live names
// what didn't fit, with the full-inventory sound;
// sendInventoryFull, plugin.js:937-941).
func (w *woodcutting) sendInventoryFull(player *game.Player, logID int) {
	if player == nil {
		return
	}
	game.SendSoundEffect(player, InventoryFullSound, 1, 0, 1)
	name := strings.ToLower(player.ItemName(logID))
	player.SendMessage(fmt.Sprintf("Your inventory is too full to hold any more %s.", name))
}

// countLog counts one chopped log toward the tracker (countLog,
// plugin.js:944-948): the attribute plus the varp send. The
// varp id is the tables' resolved LogsChoppedVarpID — the
// CacheDefinitions.GetVarpId("tracking_logs_chopped") value
// (4525 on the rev-242 seed cache).
func (w *woodcutting) countLog(player *game.Player) {
	if player == nil {
		return
	}
	chopped := logsChoppedCount(player) + 1
	player.SetAttribute(LogsChoppedAttribute, chopped)
	if w.tables != nil {
		player.SendVarp(w.tables.LogsChoppedVarpID, chopped)
	}
}

// answerLogsChopped answers the "woodcutting:logs-chopped"
// query (answerLogsChopped, plugin.js:942-944).
func (w *woodcutting) answerLogsChopped(payload any) {
	request, ok := payload.(*LogsChoppedRequest)
	if !ok || request == nil {
		return
	}
	request.Count = logsChoppedCount(request.Player)
}

// sendLogsChopped re-sends the logs-chopped varp on login
// (sendLogsChopped, plugin.js:951-956): only when the count is
// above 0.
func (w *woodcutting) sendLogsChopped(player *game.Player) {
	if player == nil || w.tables == nil {
		return
	}
	if chopped := logsChoppedCount(player); chopped > 0 {
		player.SendVarp(w.tables.LogsChoppedVarpID, chopped)
	}
}

// onPlayerLogin is the login handler: re-send the varp.
func (w *woodcutting) onPlayerLogin(event *plugins.PlayerLoginEvent) {
	if event == nil {
		return
	}
	w.sendLogsChopped(event.Player)
}

// ---- respawn overlay ----

// clientScriptSender is the SendClientScript half of the
// PacketSender interface (internal/game/dialogue_entries.go).
// The game layer's SessionPort does NOT carry it, so content
// reaches it by asserting the player's session to this local
// interface: the session package's Session implements it, and
// a session without it (a bot, a test) drops the script
// silently.
type clientScriptSender interface {
	SendClientScript(scriptID int, args ...game.ClientScriptArg)
}

// sendClientScript runs a client script for one player when
// their session supports it.
func sendClientScript(player *game.Player, scriptID int, args ...game.ClientScriptArg) {
	if player == nil || player.Session == nil {
		return
	}
	if sender, ok := player.Session.(clientScriptSender); ok {
		sender.SendClientScript(scriptID, args...)
	}
}

// sendRespawnTimer sends the stump's respawn countdown to the
// chopper AND all local players (sendRespawnTimer,
// plugin.js:959-968): the packed coordinate (z<<28)|(x<<14)|y,
// stump id/type, timer id 2, ticks, colour 16765184, 0.
func (w *woodcutting) sendRespawnTimer(player *game.Player, stump plugins.GameObject, ticks int) {
	if stump == nil {
		return
	}
	location := stump.Location()
	coordinate := (location.Z << 28) | (location.X << 14) | location.Y
	viewers := []*game.Player{player}
	if player != nil {
		viewers = append(viewers, player.LocalPlayers()...)
	}
	for _, viewer := range viewers {
		if viewer == nil {
			continue
		}
		sendClientScript(viewer, RespawnTimerScript,
			game.ClientScriptInt(coordinate),
			game.ClientScriptInt(stump.ID()),
			game.ClientScriptInt(stump.Type()),
			game.ClientScriptInt(RespawnTimerID),
			game.ClientScriptInt(ticks),
			game.ClientScriptInt(RespawnTimerColour),
			game.ClientScriptInt(0),
		)
	}
}

// ---- tree lookup, timers, depletion ----

// treeTimerKey keys a Forestry timer (treeTimerKey,
// plugin.js:971-973): "<objectId>:<x>,<y>,<z>".
func treeTimerKey(objectID int, location game.Location) string {
	return fmt.Sprintf("%d:%d,%d,%d", objectID, location.X, location.Y, location.Z)
}

// matchesTreeObject is the treeAt match predicate
// (plugin.js:1236-1238): same id, private area, tile, type,
// and face.
func matchesTreeObject(obj plugins.GameObject, id int, location game.Location, area *game.PrivateArea, typ, face int) bool {
	return obj != nil && obj.ID() == id && privateAreaOf(obj) == area &&
		obj.Location().Equals(location) && obj.Type() == typ && obj.Face() == face
}

// treeAt returns the current tree (treeAt, plugin.js:1232-1258):
// MapObjects.get first (the map original, loading the region),
// else the runtime object still standing at the session's
// tile, or nil. The Get leg is what finds an untouched map
// tree: ObjectsAt sees runtime registrations only, and map
// originals never enter the world indexes, so without it no
// real tree is ever reachable. The lazy template-copy half is
// ported through PrivateArea.ResolveObject (nil variantFor =
// identity); a stump, replacement, or remembered removal in
// the shape slot hides the lazy original, including removals
// from the world's removed-objects list (RemovedObjects,
// World.ts:1099-1101).
func (w *woodcutting) treeAt(id int, location game.Location, area *game.PrivateArea, typ, face int) plugins.GameObject {
	if area != nil && area.Destroyed() {
		return nil
	}
	objects := w.objectManager()
	if area == nil && objects != nil {
		// MapObjects.get (plugin.js:1239): the map original.
		// The registry's Get loads the region and reads the
		// tile map, so an untouched map tree is found; a stump
		// swapped in for it reads back under the stump's id,
		// so Get answers nil for the tree id and the session
		// stops. A non-matching object answers nil without
		// trying the runtime leg, as TS does (the ?? only
		// falls through on a null Get).
		if current := objects.Get(id, location); current != nil {
			if matchesTreeObject(current, id, location, area, typ, face) {
				return current
			}
			return nil
		}
	}
	if objects != nil {
		var current plugins.GameObject
		for _, obj := range objects.ObjectsAt(location) {
			if obj != nil && obj.ID() == id && privateAreaOf(obj) == area {
				current = obj
				break
			}
		}
		if current != nil {
			if matchesTreeObject(current, id, location, area, typ, face) {
				return current
			}
			return nil
		}
	}
	if area == nil || !area.UsesTemplateMap() {
		return nil
	}
	tree := area.ResolveObject(id, location, nil)
	if tree == nil || !matchesTreeObject(tree, id, location, area, typ, face) {
		return nil
	}
	// The template index still describes the original map after
	// depletion. A stump, replacement, or remembered removal in
	// that shape slot hides the lazy original.
	sameSlot := func(obj *game.GameObject) bool {
		return obj != nil && obj.PrivateArea() == area && obj.Type() == tree.Type() && obj.Location().Equals(location)
	}
	for _, obj := range area.GetObjects() {
		if sameSlot(obj) {
			return nil
		}
	}
	if objects != nil {
		for _, obj := range objects.ObjectsAt(location) {
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
	return tree
}

// withinInteractionDistance reports whether a skilling action
// on the loc may continue from the player's tile
// (GameObject.isWithinInteractionDistance,
// GameObject.ts:151-155): the footprint distance to the loc's
// rectangle — size from the resolved object definitions, 1x1
// when unknown — is at most 2. An odd face swaps the footprint
// axes. With a 1x1 footprint this is the Chebyshev distance,
// so fixture tables (no sizes resolved) behave exactly as
// before.
func (w *woodcutting) withinInteractionDistance(obj plugins.GameObject, from game.Location, objectID, face int) bool {
	sizeX, sizeY := 1, 1
	if w.tables != nil {
		if size, ok := w.tables.ObjectSize[objectID]; ok {
			sizeX, sizeY = size.SizeX, size.SizeY
		}
	}
	width, length := sizeX, sizeY
	if face&1 == 1 {
		width, length = sizeY, sizeX
	}
	gap := func(point, start, size int) int {
		return max(0, start-point, point-(start+size-1))
	}
	loc := obj.Location()
	return max(gap(from.X, loc.X, width), gap(from.Y, loc.Y, length)) <= 2
}

// several players on one tree still only drain it once per
// tick (the lastChopTick guard).
func (w *woodcutting) tickTreeDespawnTimer(state *session, currentTick int64) {
	if state == nil || state.tree == nil || state.tree.DespawnTicks == 0 {
		return
	}
	key := treeTimerKey(state.objectID, state.location)
	timer, ok := w.despawnTimers[key]
	if !ok {
		timer = &despawnTimer{remaining: state.tree.DespawnTicks, max: state.tree.DespawnTicks, lastChopTick: -1}
		w.despawnTimers[key] = timer
	}
	if timer.lastChopTick != currentTick {
		timer.lastChopTick = currentTick
		timer.remaining = max(0, timer.remaining-1)
	}
}

// regenerateTreeDespawnTimers regrows idle Forestry timers
// (regenerateTreeDespawnTimers, plugin.js:994-1003): +1 per tick
// while nobody chops, deleted at full.
func (w *woodcutting) regenerateTreeDespawnTimers(currentTick int64) {
	for key, timer := range w.despawnTimers {
		if timer.lastChopTick == currentTick {
			continue
		}
		timer.remaining++
		if timer.remaining >= timer.max {
			delete(w.despawnTimers, key)
		}
	}
}

// shouldDepleteTree reports whether the tree falls after this
// log (shouldDepleteTree, plugin.js:1005-1022): never when
// depletes is false; always when not multi; else the Forestry
// timer at 0 (despawn trees) / the 1-in-N roll (depleteChance
// trees) / the fallback roll 0-15 < 2.
func (w *woodcutting) shouldDepleteTree(state *session) bool {
	if state == nil || state.tree == nil {
		return false
	}
	tree := state.tree
	if !tree.Depletes {
		return false
	}
	if !tree.Multi {
		return true
	}
	if tree.DespawnTicks != 0 {
		timer := w.despawnTimers[treeTimerKey(state.objectID, state.location)]
		return timer != nil && timer.remaining <= 0
	}
	if tree.DepleteChance != 0 {
		return w.randInt(1, tree.DepleteChance) == 1
	}
	return w.randInt(0, MultiTreeDepletionRollMax) < MultiTreeDepletionThreshold
}

// rollRespawnTicks rolls the stump's regrow delay
// (rollRespawnTicks, plugin.js:1295-1299): the range when a max
// is present, else the flat ticks.
func (w *woodcutting) rollRespawnTicks(tree *Tree) int {
	if tree == nil {
		return 0
	}
	if tree.RespawnTicksMax != 0 {
		return w.randInt(tree.RespawnTicks, tree.RespawnTicksMax)
	}
	return tree.RespawnTicks
}

// resolveStumpID resolves the stump for a felled tree
// (resolveStumpId, plugin.js:1172-1185): the per-tree memo
// computed at LoadFromCache time (model map, then the treeId+1
// option-less stump, then the treeId+1 same-name-different-model
// variant), then the tree's own stumpId, then the default
// stump. Fixture-loaded tables have no memo, so they fall
// through to the tree's own stumpId and the default, the
// pre-Phase-33 behaviour.
func (w *woodcutting) resolveStumpID(treeID int, tree *Tree) int {
	if w.tables == nil {
		return 0
	}
	if stump, ok := w.tables.StumpByTreeID[treeID]; ok {
		return stump
	}
	if tree != nil && tree.StumpID != 0 {
		return tree.StumpID
	}
	return w.tables.DefaultTreeStumpID
}

// depleteTree fells the tree (depleteTree, plugin.js:1212-1228):
// the "woodcutting:deplete-tree" query (handled defaults false
// → falls); the stump swap via ReplaceInPlace; the respawn
// overlay unless respawnTimer is false; a delayed one-shot Task
// for regrowth that deregisters the stump if present and
// re-registers the original tree (always re-register, so
// clients get an explicit spawn update), aborting if the
// private area is destroyed. Returns false when content kept
// the tree standing (the session continues) or when no object
// manager is installed.
func (w *woodcutting) depleteTree(player *game.Player, treeObject plugins.GameObject, tree *Tree) bool {
	if treeObject == nil || tree == nil {
		return false
	}
	respawnTicks := w.rollRespawnTicks(tree)
	event := &DepleteTreeRequest{Player: player, Object: treeObject, RespawnTicks: respawnTicks, Handled: false}
	w.emitEvent(depleteTreeEventName, event)
	if event.Handled {
		return false
	}
	objects := w.objectManager()
	if objects == nil {
		return false
	}
	area := privateAreaOf(treeObject)
	stump := game.NewGameObject(w.resolveStumpID(treeObject.ID(), tree), treeObject.Location(), treeObject.Type(), treeObject.Face(), area)
	objects.ReplaceInPlace(treeObject, stump)
	if tree.RespawnTimer {
		w.sendRespawnTimer(player, stump, respawnTicks)
	}
	if tm := w.taskManager(); tm != nil {
		delay := max(1, respawnTicks)
		original := treeObject
		task := game.NewTask(delay, func(cycle int64) bool {
			if area != nil && area.Destroyed() {
				return true
			}
			if objects != nil {
				for _, obj := range objects.ObjectsAt(stump.Location()) {
					if obj != nil && obj.ID() == stump.ID() && privateAreaOf(obj) == area {
						objects.Deregister(obj, true)
						break
					}
				}
				objects.Register(original, true)
			}
			return true
		})
		task.Name = "WoodcuttingTreeRespawn"
		tm.Schedule(task)
	}
	return true
}

// ---- sessions: start / stop / tick ----

// stopWoodcutting stops the player's chopping (stopWoodcutting,
// plugin.js:1114-1121): a missing session is a no-op; the
// animation resets unless the caller keeps it (the live
// inventory-full and blisterwood stops keep the last chop
// animation running).
func (w *woodcutting) stopWoodcutting(player *game.Player, resetAnimation bool) {
	if player == nil {
		return
	}
	if _, ok := w.sessions[player]; !ok {
		return
	}
	delete(w.sessions, player)
	if resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// onPlayerDisconnect stops the session with no animation reset
// (plugin.js:1526-1528).
func (w *woodcutting) onPlayerDisconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	w.stopWoodcutting(event.Player, false)
}

// startWoodcutting starts a chopping session (startWoodcutting,
// plugin.js:1260-1320) in exact order: validate-tree query →
// best usable axe or NO_AXE_MESSAGE → tree level check →
// inventory check → tree still there → stopSkillable + stop old
// session + combat reset + face tree → store session
// (nextActionTick/nextAnimationTick = tick + 2) → spam
// SWING_MESSAGE + play axe animation.
//
// The face-tree step (plugin.js:1279-1285) runs through the
// Phase 33 player-facing seam (Player.ForcePositionToFace):
// reset() clears the face the object click set, so the tree is
// faced again, forced, because the field can already hold it
// while the client was turned by walking. The state + flag are
// set; the player sync snapshot does not yet carry a face tile,
// so clients do not see the turn until that encoder lands.
func (w *woodcutting) startWoodcutting(player *game.Player, obj plugins.GameObject, tree *Tree) bool {
	if player == nil || obj == nil || tree == nil {
		return false
	}
	request := &ValidateTreeRequest{Player: player, Object: obj, Allow: true}
	w.emitEvent(validateTreeEventName, request)
	if !request.Allow {
		return false
	}
	axe := w.findBestUsableAxe(player)
	if axe == nil {
		player.SendMessage(NoAxeMessage)
		return false
	}
	if woodcuttingLevel(player) < tree.RequiredLevel {
		player.SendMessage(fmt.Sprintf("You need a Woodcutting level of %d to chop down this tree.", tree.RequiredLevel))
		return false
	}
	if tree.LogID >= 0 && w.inventoryBlocks(player, tree.LogID) {
		w.sendInventoryFull(player, tree.LogID)
		return false
	}
	location := obj.Location()
	existing := w.treeAt(obj.ID(), location, privateAreaOf(obj), obj.Type(), obj.Face())
	if existing == nil {
		player.SendMessage("You can't reach that tree right now.")
		return false
	}
	if skills := player.Skills(); skills != nil {
		skills.StopSkillable()
	}
	w.stopWoodcutting(player, false)
	if combat := player.GetCombat(); combat != nil {
		combat.Reset()
	}
	// reset() clears the face the object click set, so face the
	// tree again — forced, because the field can already hold
	// it while the client was turned by walking
	// (plugin.js:1279-1285).
	player.ForcePositionToFace(&location)
	// The global tick task schedules here if it has not yet
	// (see ensureTask): the session only ticks while the task
	// runs.
	w.ensureTask()
	w.sessions[player] = &session{
		tree:              tree,
		axe:               *axe,
		objectID:          obj.ID(),
		location:          location,
		area:              privateAreaOf(obj),
		objType:           obj.Type(),
		objFace:           obj.Face(),
		nextActionTick:    w.tick + FirstRollDelayTicks,
		nextAnimationTick: w.tick + FirstRollDelayTicks,
	}
	w.sendSpam(player, SwingMessage)
	player.PerformAnimation(axe.AnimationID, 0)
	return true
}

// processWoodcuttingTick ticks every session
// (processWoodcuttingTick, plugin.js:1322-1477) in exact order:
// dead/unregistered → delete; movement queue non-empty →
// stop; level-up paused → skip; tree gone → stop;
// validate-tree → stop; out of interaction distance → stop;
// re-find axe every tick (none / axe level lost / tree level
// lost → message + stop); inventory full → full message + stop
// WITHOUT animation reset; tick despawn timer; animation every
// 4 ticks; action gate (nextActionTick, via the
// "skills:action-interval" query, 4 ticks, floor 1);
// "woodcutting:attempt" query around rollLog; miss → continue;
// success → nest pre-emption else the success block; then the
// depletion check.
//
// SEAM GAPS: the force-movement skip (player.getForceMovement
// () != null) cannot run — no ForceMovement field or system
// exists in Go (movement_queue.go documents its absence) — so
// a forced move never pauses a session here. The interaction
// distance is the TS footprint rule since Phase 33
// (withinInteractionDistance, GameObject.ts:151-155).
func (w *woodcutting) processWoodcuttingTick(currentTick int64) {
	if w.tables == nil {
		return
	}
	for player, state := range w.sessions {
		if player == nil || !player.IsRegistered() || skillLevel(player, game.SkillHitpoints) <= 0 {
			delete(w.sessions, player)
			continue
		}
		if mq := player.MovementQueue(); mq != nil && (mq.Size() > 0 || mq.IsMoving()) {
			w.stopWoodcutting(player, true)
			continue
		}
		if state.levelUpPaused {
			continue
		}
		activeTree := w.treeAt(state.objectID, state.location, state.area, state.objType, state.objFace)
		if activeTree == nil {
			w.stopWoodcutting(player, true)
			continue
		}
		validate := &ValidateTreeRequest{Player: player, Object: activeTree, Allow: true}
		w.emitEvent(validateTreeEventName, validate)
		if !validate.Allow {
			w.stopWoodcutting(player, true)
			continue
		}
		if !w.withinInteractionDistance(activeTree, player.Location(), state.objectID, state.objFace) {
			w.stopWoodcutting(player, true)
			continue
		}
		axe := w.findBestUsableAxe(player)
		if axe == nil {
			player.SendMessage(NoAxeMessage)
			w.stopWoodcutting(player, true)
			continue
		}
		wcLevel := woodcuttingLevel(player)
		if wcLevel < axe.RequiredLevel {
			player.SendMessage(NoAxeMessage)
			w.stopWoodcutting(player, true)
			continue
		}
		if wcLevel < state.tree.RequiredLevel {
			player.SendMessage(fmt.Sprintf("You need a Woodcutting level of %d to chop down this tree.", state.tree.RequiredLevel))
			w.stopWoodcutting(player, true)
			continue
		}
		state.axe = *axe
		if state.tree.LogID >= 0 && w.inventoryBlocks(player, state.tree.LogID) {
			w.sendInventoryFull(player, state.tree.LogID)
			w.stopWoodcutting(player, false)
			continue
		}
		w.tickTreeDespawnTimer(state, currentTick)
		if currentTick >= state.nextAnimationTick {
			player.PerformAnimation(state.axe.AnimationID, 0)
			state.nextAnimationTick = currentTick + ChopAnimationIntervalTicks
		}
		if currentTick < state.nextActionTick {
			continue
		}
		state.nextActionTick = currentTick + int64(skillactions.ActionInterval(w.emit, player, game.SkillWoodcutting, "chop", WoodcuttingActionIntervalTicks, 0))
		attempt := &AttemptRequest{Player: player, Object: activeTree, LogID: state.tree.LogID, Success: w.rollLog(player, state.tree, &state.axe, state)}
		w.emitEvent(attemptEventName, attempt)
		if !attempt.Success {
			continue
		}
		// A nest falls instead of that roll's log: no log, line
		// or XP. Trees without bird nests still roll clue nests
		// below, in chopSuccess.
		if state.tree.LogID >= 0 && !(state.tree.BirdNests && w.maybeDropBirdNest(player)) {
			w.chopSuccess(player, state)
		}
		if w.shouldDepleteTree(state) {
			delete(w.despawnTimers, treeTimerKey(state.objectID, state.location))
			if state.tree.InterruptMessage != "" {
				player.PerformAnimation(state.tree.InterruptAnimationID, 0)
				game.SendSoundEffect(player, state.tree.InterruptSound, 1, 0, 1)
				w.sendSpam(player, state.tree.InterruptMessage)
				w.stopWoodcutting(player, false)
				continue
			}
			// A tree content keeps standing is chopped on, its
			// despawn timer starting afresh.
			if !w.depleteTree(player, activeTree, state.tree) {
				continue
			}
			game.SendSound(player, game.SoundWoodcuttingTreeDown)
			w.stopWoodcutting(player, true)
			continue
		}
	}
}

// chopSuccess runs the success block (plugin.js:1417-1458) in
// TS order: felling ration → ration energy → XP computed (base
// x lumberjack, +10% if rationed; diary query inside base) →
// logAmount via headgear → if not skipLog: burn check → prepareLog
// → countLog unless countsLogs is false → log line unless skipLog or
// logLine is false → crystal charge → XP granted unconditionally on success
// via AddExperiences — including burnt/skipLog cases,
// excluding nest pre-emption → pets:skilling-roll query emit
// → deliverLog → emit woodcutting:success → clue-nest roll
// (rollClueNests, cluenests.go).
//
// XP is float in TS (37.5, 67.5, ...) but Go stores integer
// experience and AddExperiences takes an int, so the amount is
// truncated toward zero: a 37.5-XP log grants 37. The half-XP
// remainder is a known integer-experience deviation, not a
// rounding choice made here.
func (w *woodcutting) chopSuccess(player *game.Player, state *session) {
	tree := state.tree
	fellingBonus, skipLog := w.fellingChop(player, &state.axe)
	w.rollForesterRationEnergy(player, &state.axe, fellingBonus)
	baseXP := w.treeBaseXP(player, tree, state.location) * w.lumberjackMultiplier(player)
	xp := baseXP
	if fellingBonus {
		xp = baseXP + baseXP/10
	}
	logAmount := w.logsPerChop(player, tree.LogID)
	var pendingLog *game.Item
	burnt := false
	if !skipLog {
		burnt = w.tryBurnLog(player, state.axe.ID, tree.LogID)
		if !burnt {
			pendingLog = w.prepareLog(player, tree.LogID, xp, logAmount)
			// A removeOnly chop (log id -1) counts no log: the
			// plan's "no-log/no-XP clone". TS would count it;
			// see the requestedChop note.
			if tree.CountsLogs && tree.LogID >= 0 {
				w.countLog(player)
			}
		}
	}
	// A burnt log still says it was cut; a felling skip gives no
	// log line. A removeOnly chop says nothing.
	if !skipLog && tree.LogLine && tree.LogID >= 0 {
		w.sendSpam(player, w.logMessage(player, tree))
	}
	// Only the charged crystal axe doing the chopping spends a
	// charge: one carried beside the axe in use is left alone.
	if state.axe.Item == "crystal_axe" || state.axe.Item == "crystal_axe_2h" {
		w.tryUseCrystalCharge(player, state.axe.ID)
	}
	if skills := player.Skills(); skills != nil {
		skills.AddExperiences(game.SkillWoodcutting, int(xp))
	}
	petRoll := &SkillingRollRequest{Player: player, Skill: game.SkillWoodcutting, PetBase: tree.PetBase, Handled: false}
	w.emitEvent(petsSkillingRollEvent, petRoll)
	w.deliverLog(player, pendingLog)
	eventLogAmount := logAmount
	if skipLog {
		eventLogAmount = 0
	} else if burnt {
		eventLogAmount = 1
	}
	w.emitEvent(successEventName, &WoodcuttingSuccessEvent{
		Player:    player,
		Skill:     game.SkillWoodcutting,
		PetBase:   tree.PetBase,
		PetRolled: petRoll.Handled,
		LogID:     tree.LogID,
		Location:  state.location,
		LogAmount: eventLogAmount,
		AxeID:     state.axe.ID,
	})
	// Clue nests roll on every successful log, after the success
	// emit (plugin.js:1441, ClueNests.rollClueNests).
	w.rollClueNests(player, tree)
}

// ---- object interaction + requested handlers ----

// handleChop handles a tree click (handleChop, plugin.js:1478-
// 1489): look up by object id (pest-control) then by definition
// name, return silently if no tree, and ALWAYS claim the event
// (even on start failure — fail messages are the plugin's own).
func (w *woodcutting) handleChop(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || w.tables == nil {
		return false
	}
	tree := w.tables.TreeByObjectID(event.ObjectID)
	if tree == nil && event.Definition != nil {
		tree = w.tables.TreeByName(event.Definition.Name())
	}
	if tree == nil {
		return false
	}
	w.startWoodcutting(event.Player, event.Object, tree)
	return true
}

// requestedChop answers a "woodcutting:chop" request
// (requestedChop, plugin.js:1491-1506): find the tree by logId
// in FARMED_TREES then TREES (farmed hardwoods share log ids
// with the wild trees but regrow on farming's own timer);
// removeOnly marks a no-log/no-XP clone (logId -1, xp 0, not
// multi). The session is marked requested: no Forestry group
// bonus. Another plugin's tree takes no group bonus.
func (w *woodcutting) requestedChop(payload any) {
	request, ok := payload.(*ChopRequest)
	if !ok || request == nil || w.tables == nil {
		return
	}
	var tree *Tree
	for _, candidate := range w.tables.FarmedTrees {
		if candidate.LogID == request.LogID {
			tree = candidate
			break
		}
	}
	if tree == nil {
		for _, candidate := range w.tables.Trees {
			if candidate.LogID == request.LogID {
				tree = candidate
				break
			}
		}
	}
	if tree == nil {
		return
	}
	if request.RemoveOnly {
		clone := *tree
		clone.LogID = -1
		clone.XPReward = 0
		clone.Multi = false
		tree = &clone
	}
	request.Handled = w.startWoodcutting(request.Player, request.Object, tree)
	if request.Handled {
		if sess := w.sessions[request.Player]; sess != nil {
			sess.requested = true
		}
	}
}

// requestedAxe answers a "woodcutting:axe" request
// (requestedAxe, plugin.js:1508-1514): the named axe when
// usable, else the player's best usable axe; a members axe
// answers no request on a free world, as it chops no tree
// there.
func (w *woodcutting) requestedAxe(payload any) {
	request, ok := payload.(*AxeRequest)
	if !ok || request == nil || w.tables == nil {
		return
	}
	if request.ItemID == nil {
		request.Axe = w.findBestUsableAxe(request.Player)
		return
	}
	candidate := w.tables.AxeByID(*request.ItemID)
	if candidate != nil && w.choppingAxeEligible(candidate) && canUseAxe(request.Player, candidate) {
		request.Axe = candidate
	} else {
		request.Axe = nil
	}
}

// pauseWoodcutting holds the session through a level-up pop-up
// (pauseWoodcutting, plugin.js:1516-1518): skillactions.
// PauseForLevelUp with the sessions converted to the helper's
// map type and a stop callback that keeps the animation.
func (w *woodcutting) pauseWoodcutting(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(w.sessions))
	for player, sess := range w.sessions {
		sessions[player] = sess
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		w.stopWoodcutting(player, false)
	}, nil)
}

// ---- deferred stubs (explicit, named) ----

// isInGuild reports whether the player is inside the
// Woodcutting Guild (Guild.Woodcutting.js: isInGuild).
// STUB: the Guild unit is deferred (it needs the
// woodcutting-guild.json import, which needs Tyler's
// approval), so no player is ever in the guild: the group-bonus
// guild exclusion never fires and the guild boost is never
// applied.
func isInGuild(player *game.Player) bool {
	return false
}

// invisibleBoost is the guild's +7 invisible Woodcutting boost
// (Guild.Woodcutting.js: invisibleBoost).
// STUB: the Guild unit is deferred, so the boost is always 0.
func invisibleBoost(player *game.Player) int {
	return 0
}
