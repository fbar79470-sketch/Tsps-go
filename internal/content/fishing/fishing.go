// Package fishing is the Go port of the Fishing skill plugin
// (server/plugins/skills/Fishing.plugin.js, 1,134 lines, commit
// bfd81e3): the core fishing loop — sessions, the 1-tick task,
// catch chance, rolls, landing — plus the sub-units that plug
// into it (Worker C's files). Tables live in tables.go (Worker
// A); this file owns the loop that consumes them. Line numbers
// in comments cite Fishing.plugin.js unless another file is
// named.
//
// The port is faithful: same plugin name, description, and
// members flag, same attach-only register, same guard order in
// startFishing, hasToolRequirements and processFishingTick,
// same roll/landing order. Deviations forced by missing Go
// seams are named inline and in the report; nothing is
// silently invented.
//
// Event inventory:
//
//   - Listens to onNpcInteraction for both spot names and all
//     10 options (plugin.js:1126-1132), produced by the
//     NPC-option listener through the cmd/server adapter.
//   - Listens to "fishing:tool" / "fishing:interval" /
//     "fishing:catch-chance" / "fishing:attempt" /
//     "fishing:catch" (custom-event queries/requests from
//     other content — league relics; no Go emitter exists
//     yet, so these are answer-only).
//   - Emits "fishing:barehand-unlocked" (query, allowed
//     defaults false), "fishing:continue" (query, handled
//     defaults false), "fishing:junk" (fire-and-forget),
//     "fishing:success" (fire-and-forget, with a stop flag),
//     "diary:is-complete" (query, complete defaults false),
//     "storage:room" / "storage:gather" / "storage:amount" /
//     "storage:take" (no storage content), "pets:skilling-roll"
//     is NOT emitted here: the pet roll rides the
//     "fishing:success" event's petBase (plugin.js:887-895),
//     the Woodcutting "woodcutting:success" precedent.
//     Every query degrades to its documented default when
//     unanswered.
//   - Listens to "level-up:popup-opened" via
//     skillactions.PauseForLevelUp; the emitter (SkillLevelUp
//     feedback content) is NOT STARTED, so the pause never fires
//     yet and level-up currently does NOT pause fishing.
//   - Listens to onPlayerDisconnect (stop session, no animation
//     reset).
//   - "player:interaction-cleared" / "player:emote" are NOT
//     registered: no Go emitter exists for either event
//     (Woodcutting header precedent — follow it, do not invent
//     an emitter).
//
// State lives in a *fishing struct created per Plugin value —
// NO package-level mutable maps (the Phase 21 race fix): the
// shared instance behind Plugin is the only one, and tests
// build their own. The tick task owns the tick counter.
//
// Worker C sub-units (angleroutfit.go, crystalharpoon.go,
// infernalharpoon.go, karambwan.go, radasblessing.go,
// spiritflakes.go, cluebottles.go, guild.go) plug into the
// loop through the methods named in the loop below; their
// Register calls go at the marked spot in Register.
package fishing

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- loop constants ----
// The data constants live in tables.go; these are the loop's own
// literals, kept here with the code that reads them.

const (
	// spamChatType is the chat channel for fishing lines: the
	// spam channel (protocol.ChatTypeSpam, 105 — the game
	// package cannot import protocol, so the value is spelled
	// out with its provenance, as woodcutting does).
	spamChatType = 105
)

// ---- custom-event names ----

const (
	fishingToolEventName        = "fishing:tool"
	fishingIntervalEventName    = "fishing:interval"
	fishingCatchChanceEventName = "fishing:catch-chance"
	fishingAttemptEventName     = "fishing:attempt"
	fishingCatchEventName       = "fishing:catch"
	fishingSuccessEventName     = "fishing:success"
	fishingJunkEventName        = "fishing:junk"
	fishingContinueEventName    = "fishing:continue"
	barehandUnlockedEventName   = "fishing:barehand-unlocked"
	diaryCompleteEventName      = "diary:is-complete"
	storageRoomEventName        = "storage:room"
	storageGatherEventName      = "storage:gather"
	storageAmountEventName      = "storage:amount"
	storageTakeEventName        = "storage:take"
)

// ---- custom-event payloads ----
// Each mirrors the TS request object of the same name; a handler
// answers by mutating the payload in place, exactly as TS does.

// FishingToolRequest is the "fishing:tool" fallback query
// (plugin.js:707-709): content may stand in for the tool (a
// league relic's harpoon that fishes as any of them).
// Variants carries the best-first list; Variant answers nil
// when nothing stands in.
type FishingToolRequest struct {
	Player   *game.Player
	Variants []ToolVariant
	Variant  *ToolVariant
}

// FishingIntervalRequest is the "fishing:interval" query
// (plugin.js:712-716): content may adjust the ticks to the
// next catch roll (a league relic's faster harpoon). Ticks
// carries the tool interval in and the adjusted ticks out;
// the loop floors it at 1.
type FishingIntervalRequest struct {
	Player *game.Player
	Ticks  int
}

// FishingCatchChanceRequest is the "fishing:catch-chance"
// query (plugin.js:937-940): content may adjust one roll's
// chance. Chance carries the computed chance in and the
// adjusted chance out; the loop clamps it to [0, 1].
type FishingCatchChanceRequest struct {
	Player *game.Player
	ToolID int
	ItemID int
	Chance float64
}

// FishingAttemptRequest is the "fishing:attempt" query
// (plugin.js:1065-1068): content may turn a miss into a hit
// (a league relic's second chance) by setting Success.
type FishingAttemptRequest struct {
	Player  *game.Player
	Success bool
}

// FishingCatchEvent is the "fishing:catch" emit
// (plugin.js:1079-1080): the catch a roll landed, which
// content may replace before landCatch runs. Caught is the
// live slice: handlers mutate it in place.
type FishingCatchEvent struct {
	Player *game.Player
	NPCID  int
	Caught []*Fish
}

// FishingSuccessEvent is the "fishing:success" emit
// (plugin.js:887-895): one landed fish kind — the skill, the
// pet roll base, and whether the catch was barehanded. A
// listener may take over the session (the tutorial stops
// after one catch) by setting Stop. Junk never emits it.
type FishingSuccessEvent struct {
	Player   *game.Player
	Skill    int
	ItemID   int
	PetBase  int
	Barehand bool
	Stop     bool
}

// FishingJunkEvent is the "fishing:junk" emit
// (plugin.js:866-868): a landed non-fish (a casket, an old
// boot), for a league's "Obtain a Casket from Fishing" or an
// old boot.
type FishingJunkEvent struct {
	Player *game.Player
	ItemID int
	Amount int
}

// FishingContinueRequest is the "fishing:continue" query
// (plugin.js:1000-1004): content may keep fishing when its
// spot would normally end the session. Handled defaults
// false.
type FishingContinueRequest struct {
	Player  *game.Player
	Handled bool
}

// BarehandUnlockedRequest is the "fishing:barehand-unlocked"
// query (plugin.js:576-580): Barbarian Training answers
// whether Otto has taught barehanded fishing. Allowed
// defaults false: no answer locks it.
type BarehandUnlockedRequest struct {
	Player  *game.Player
	Allowed bool
}

// DiaryCompleteRequest is the "diary:is-complete" query
// (plugin.js:929-934): Complete answers whether the player
// has the diary tier done; it defaults false.
type DiaryCompleteRequest struct {
	Player   *game.Player
	Diary    string
	Tier     string
	Complete bool
}

// FishingStorageRoomRequest is the "storage:room" query
// (plugin.js:747-752): Room answers whether an open container
// (the fish barrel) has room for the item; it defaults false.
type FishingStorageRoomRequest struct {
	Player *game.Player
	ItemID int
	Skill  int
	Room   bool
}

// FishingStorageGatherRequest is the "storage:gather" request
// (plugin.js:755-760): storage takes the catch first; Stored
// answers how many it took, and the remainder goes to the
// inventory. XP is the Fishing XP the catch is worth, for
// content that banks XP with the fish.
type FishingStorageGatherRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Stored int
	Skill  int
	XP     float64
}

// FishingStorageAmountRequest is the "storage:amount" query
// (plugin.js:733-737): Amount carries the inventory count in
// and the total (inventory + storage) out.
type FishingStorageAmountRequest struct {
	Player *game.Player
	ItemID int
	Amount int
}

// FishingStorageTakeRequest is the "storage:take" request
// (plugin.js:739-744): Taken answers how many of Amount
// storage handed over.
type FishingStorageTakeRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Taken  int
}

// ---- session state ----

// fishingSession is one player's fishing session (the
// per-player object Fishing.plugin.js keeps in its
// activeSessions Map, plugin.js:986-998). It implements
// skillactions.Session, so PauseForLevelUp can hold it
// through a level-up pop-up; it must stay a pointer
// (comparable) value for the helper's identity checks.
//
// The session holds the NPC pointer, not the world NPC index:
// Api exposes no NPC-by-index lookup (TS reads
// World.getNpcs().get(session.npcIndex) each tick,
// plugin.js:1013), so the tick validates the pointer
// instead — still registered, still the same content id,
// still on the remembered tile. A despawned spot's pointer
// fails IsRegistered and the session stops, which is the
// TS null-NPC leg; a respawned spot under the same index is
// a different NPC object and never resumes an old session,
// where TS would resume it if the id matched — the stricter
// Go behaviour is documented, not silently equivalent.
type fishingSession struct {
	npc               *game.NPC
	npcID             int
	spotX             int
	spotY             int
	tool              *FishingTool
	nextAnimationTick int64
	nextCatchTick     int64
	// followMessageTick is the tick the follow message fires
	// on, or 0 when the tool has none (TS null,
	// plugin.js:996). Ticks start at 1, so 0 never fires.
	followMessageTick int64
	levelUpPaused     bool
}

// SetLevelUpPaused marks the session held by a level-up pop-up
// (skillactions.Session).
func (s *fishingSession) SetLevelUpPaused(paused bool) {
	if s != nil {
		s.levelUpPaused = paused
	}
}

// fishing is the plugin's per-registration state: the TS
// module-level state (activeSessions, fishingTick, pluginApi)
// as struct fields, per the Phase 21 race fix. One instance
// serves the registered Plugin value; tests build their own
// with newFishing.
type fishing struct {
	api    *plugins.Api
	tables *Tables
	ids    *FishingIDs
	// emit is the Register-time Api.EmitCustomEvent, held so
	// emitCustomEvent can run before any hub surface that
	// installs after plugin Load is resolved. Nil in unit
	// tests, which set it directly (the woodcutting
	// precedent).
	emit func(name string, payload any)
	// tasks is the direct TaskManager override (unit tests
	// set it); production leaves it nil and resolves through
	// the api instead — a Register-time capture would read
	// nil, because the TaskManager installs after Load.
	tasks *game.TaskManager
	// taskScheduled guards ensureTask: the 1-tick global task
	// schedules once, on the first cast.
	taskScheduled bool
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
	sessions      map[*game.Player]*fishingSession
	tick          int64
	randFloat     func() float64
	randInt       func(min, max int) int
}

// newFishing builds a fishing instance over tables (nil tables
// degrade every handler to a no-op; see Register).
func newFishing(tables *Tables) *fishing {
	return &fishing{
		tables:       tables,
		sessions:     make(map[*game.Player]*fishingSession),
		membersWorld: true,
		randFloat:    func() float64 { return rand.Float64() },
		randInt: func(min, max int) int {
			if max <= min {
				return min
			}
			return min + rand.IntN(max-min+1)
		},
	}
}

// shared is the instance behind Plugin; SetTables, SetIDs,
// SetMembersWorld and SetIsMembersItem operate on it.
var shared = newFishing(nil)

// SetTables installs the resolved tables for the registered
// Plugin value. There is no CacheDefinitions accessor on Api,
// so the coordinator's integration step must call this with
// LoadFromCache's result BEFORE hub Load: Register captures
// the shared instance, so a call before Load reaches every
// handler; a call after Load is too late for the spot
// registrations. With no tables installed, every handler
// degrades to a no-op (startFishingInteraction claims
// nothing, the tick task runs empty).
func SetTables(t *Tables) {
	shared.tables = t
}

// SetIDs installs Worker C's resolved ids (LoadIDs) for the
// registered Plugin value, before hub Load like SetTables.
// With no ids installed, the sub-units that need them degrade
// to no-ops; the core loop never reads ids.
func SetIDs(ids *FishingIDs) {
	shared.ids = ids
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

// plugin builds the registered Plugin value: name,
// description, and members flag verbatim (module.exports,
// Fishing.plugin.js:1114), attach-only register.
//
// MEMBERS GATING: members:false. Fishing is F2P — no
// members:true gate on the plugin; members filtering is
// per-tool/per-fish, as TS does it (findTool's usableHere,
// rollCatch's members-fish skip).
func (f *fishing) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Fishing",
		Description: "Fishing at all spot types: nets, cages, harpoons, rods and karambwan vessels, with tool animations, rolls and catches.",
		Members:     false,
		Register: func(api *plugins.Api) {
			f.api = api
			f.emit = api.EmitCustomEvent
			// The 1-tick global task is NOT scheduled here, and
			// NOT on server startup either: the TaskManager
			// installs on the hub after plugin Load (nil until
			// installed, per the hub contract — see
			// taskManager), so a Register-time capture reads
			// nil; and a startup hook would call Schedule off
			// the tick goroutine (Server.Start emits startup
			// after the engine is already ticking), which the
			// TaskManager forbids — Schedule/Process must run
			// on the tick goroutine. The task schedules once
			// on the first cast instead (ensureTask, from
			// startFishing, which runs on the tick goroutine
			// through the turn-packet drain) — the Woodcutting
			// pattern (woodcutting.go:616-660). TS submits
			// its FishingTask at register (plugin.js:1102);
			// Go defers it to the first cast, as Woodcutting
			// defers its own.
			api.OnPlayerDisconnect(f.onPlayerDisconnect)
			api.OnCustomEvent(skillactions.LevelUpPopupOpenedEventName, f.pauseFishing)
			// Sub-unit attach calls, in TS attach order
			// (plugin.js:1103-1111): InfernalHarpoon,
			// CrystalHarpoon, AnglerOutfit. RadasBlessing,
			// SpiritFlakes, Guild, KarambwanVessel and
			// ClueBottles register no hooks of their own —
			// the core loop calls into them (landExtra,
			// catchLevel, hasToolRequirements, startFishing,
			// landCatch). MinnowPlatform is deferred
			// (PHASE35_PLAN.md §7) and attaches nothing.
			f.registerInfernalHarpoon(api)
			f.registerCrystalHarpoon(api)
			f.registerAnglerOutfit(api)
			if f.tables != nil {
				actions := make(map[string]plugins.NpcInteractionHandler, len(FishingSpotOptions))
				for _, option := range FishingSpotOptions {
					actions[option] = f.startFishingInteraction
				}
				for _, name := range FishingSpotNames {
					api.OnNpcInteraction(name, actions)
				}
			}
		},
	}
}

// Plugin is the registered Fishing plugin value.
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
func (f *fishing) taskManager() *game.TaskManager {
	if f.api != nil {
		if tm := f.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return f.tasks
}

// ensureTask schedules the 1-tick global FishingTask once, on
// the lazily-resolved TaskManager. Idempotent: a second call
// (a second cast) schedules nothing. A nil TaskManager
// schedules nothing and retries on the next call. Callers run
// on the tick goroutine (startFishing, through the
// turn-packet drain), which is the only goroutine the
// TaskManager allows Schedule from.
func (f *fishing) ensureTask() {
	if f.taskScheduled {
		return
	}
	tm := f.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		f.tick++
		f.processFishingTick(f.tick)
		return false
	})
	task.Name = "FishingTask"
	tm.Schedule(task)
	f.taskScheduled = true
}

// emitCustomEvent emits a custom event through the captured
// emit func; a nil emit (before Register) drops the emit,
// which is the no-hooks default for every query. It is
// nil-safe: a nil receiver, a nil emit with no Api, or a nil
// Api all drop the emit instead of panicking. When the emit
// field is set (Register captured api.EmitCustomEvent — the
// woodcutting.go emit-field precedent) it runs; otherwise
// the live Api's EmitCustomEvent runs.
func (f *fishing) emitCustomEvent(name string, payload any) {
	if f == nil {
		return
	}
	if f.emit != nil {
		f.emit(name, payload)
		return
	}
	if f.api == nil {
		return
	}
	f.api.EmitCustomEvent(name, payload)
}

// fishingLevel reads the player's current Fishing level
// (getFishingLevel, plugin.js:586-588). A nil player or
// missing skill manager reads 0 (Go nil-safety; TS never
// calls with a nil player).
func fishingLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	skills := player.Skills()
	if skills == nil {
		return 0
	}
	return skills.CurrentLevel(game.SkillFishing)
}

// maxFishingLevel reads the player's max (real) Fishing level
// (getMaxLevel, plugin.js:411-412: the minnow net's
// unboostable gate). A nil player or missing skill manager
// reads 0.
func maxFishingLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	skills := player.Skills()
	if skills == nil {
		return 0
	}
	return skills.MaxLevel(game.SkillFishing)
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

// sendSpam sends a filterable chatbox line (spam,
// plugin.js:948-950: sendTypedMessage(ChatType.SPAM, text)).
func (f *fishing) sendSpam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(spamChatType, text, "")
}

// statement opens a message box (statement, plugin.js:945-947):
// StatementDialogue(0, text) + EndDialogue(1) through the
// dialogue manager, so "Click here to continue" closes it —
// the Go form is game.SendStatementDialogue with canContinue
// true (internal/game/dialogue_entries.go:622).
func (f *fishing) statement(player *game.Player, text string) {
	game.SendStatementDialogue(player, text, true)
}

// inventoryFull sends the full-inventory stop (inventoryFull,
// plugin.js:953-957): reset animation, sound 2277, and the
// "You can't carry any more fish." statement.
func (f *fishing) inventoryFull(player *game.Player) {
	if player == nil {
		return
	}
	player.PerformAnimation(game.ResetAnimationID, 0)
	game.SendSoundEffect(player, FullInventorySound, 1, 0, 1)
	f.statement(player, FullInventoryStatement)
}

// ---- storage queries (Woodcutting's request-type pattern) ----
// The "storage:*" custom events are the tackle-box / fish
// barrel seam (plugin.js:733-760). Every helper degrades to
// its documented default when unanswered: no storage content
// exists in Go, so storage never holds anything.

// storageRoom reports whether an open container would take
// this fish (storageRoom, plugin.js:747-752): the
// "storage:room" query, room defaults false.
func (f *fishing) storageRoom(player *game.Player, itemID int) bool {
	request := &FishingStorageRoomRequest{Player: player, ItemID: itemID, Skill: game.SkillFishing, Room: false}
	f.emitCustomEvent(storageRoomEventName, request)
	return request.Room
}

// storageGather gives the catch to storage first
// (giveCatch's storage leg, plugin.js:755-760): the
// "storage:gather" request. It returns how many storage took
// (Stored), 0 when unanswered; the caller inventories the
// rest.
func (f *fishing) storageGather(player *game.Player, itemID, amount int, xp float64) int {
	request := &FishingStorageGatherRequest{Player: player, ItemID: itemID, Amount: amount, Stored: 0, Skill: game.SkillFishing, XP: xp}
	f.emitCustomEvent(storageGatherEventName, request)
	return request.Stored
}

// storageAmount reports how many of the item the player holds
// across inventory + storage (storedAmount, plugin.js:733-
// 737): the inventory count in, the total out through the
// "storage:amount" query. Unanswered it is the inventory
// count.
func (f *fishing) storageAmount(player *game.Player, itemID int) int {
	amount := 0
	if player != nil {
		if inventory := player.Inventory(); inventory != nil {
			amount = inventory.GetAmount(itemID)
		}
	}
	request := &FishingStorageAmountRequest{Player: player, ItemID: itemID, Amount: amount}
	f.emitCustomEvent(storageAmountEventName, request)
	return request.Amount
}

// storageTake takes amount of the item out of storage
// (useBait's storage leg, plugin.js:739-744): the
// "storage:take" request. It returns how many storage handed
// over (Taken), 0 when unanswered.
func (f *fishing) storageTake(player *game.Player, itemID, amount int) int {
	request := &FishingStorageTakeRequest{Player: player, ItemID: itemID, Amount: amount, Taken: 0}
	f.emitCustomEvent(storageTakeEventName, request)
	return request.Taken
}

// ---- levels, quests, tools ----

// questAllows reports whether the player may fish a
// quest-gated tool (questAllows, plugin.js:677-681). Go has
// no QuestRuntime, so every gate always passes — which is
// TS-faithful: TS fails OPEN when the quest is not
// registered ("A quest gates its fish only once this server
// implements it, so an unwritten quest does not lock its
// fishing away for good"), and neither "Swan Song" nor
// "Barbarian Training" is registered in Go.
func (f *fishing) questAllows(player *game.Player, name string) bool {
	return true
}

// catchLevel is the level catch rolls use (catchLevel,
// plugin.js:655-657): the visible level capped at 99, plus
// the Fishing Guild's invisible +7 (guildInvisibleBoost,
// Worker C; 0 until the Guild unit lands).
func (f *fishing) catchLevel(player *game.Player) int {
	return min(99, fishingLevel(player)) + f.guildInvisibleBoost(player)
}

// meetsLevels reports whether the player meets every
// other-skill requirement (meetsLevels, plugin.js:659-661).
func (f *fishing) meetsLevels(player *game.Player, requires []SkillRequirement) bool {
	for _, req := range requires {
		if skillLevel(player, req.Skill) < req.Level {
			return false
		}
	}
	return true
}

// findTool finds the best variant of the tool the player can
// use, carried or wielded; nil if none (findTool,
// plugin.js:683-700). Variants run best-first; a variant
// needs its Fishing level and its Agility level (absent
// reads 1, the TS `?? 1` default), and one of its ids held
// (an empty ids list means bare hands). Members tools are no
// tool on free worlds (usableHere, plugin.js:686 — the
// Mining/Woodcutting rule): a members item counts as not
// held there. With no variant held, the "fishing:tool"
// fallback query may stand in (a league relic's harpoon);
// unanswered it stays nil.
func (f *fishing) findTool(player *game.Player, tool *FishingTool) *ToolVariant {
	if player == nil || tool == nil {
		return nil
	}
	level := fishingLevel(player)
	agility := skillLevel(player, game.SkillAgility)
	usableHere := func(id int) bool {
		if f.membersWorld {
			return true
		}
		if f.isMembersItem == nil {
			return true
		}
		return !f.isMembersItem(id)
	}
	has := func(id int) bool {
		if !usableHere(id) {
			return false
		}
		if inventory := player.Inventory(); inventory != nil && inventory.Contains(id) {
			return true
		}
		if equipment := player.Equipment(); equipment != nil && equipment.Contains(id) {
			return true
		}
		return false
	}
	for i := range tool.Variants {
		variant := &tool.Variants[i]
		reqAgility := variant.Agility
		if reqAgility < 1 {
			reqAgility = 1
		}
		if variant.Level > level || agility < reqAgility {
			continue
		}
		if len(variant.IDs) == 0 {
			return variant
		}
		for _, id := range variant.IDs {
			if has(id) {
				return variant
			}
		}
	}
	request := &FishingToolRequest{Player: player, Variants: tool.Variants, Variant: nil}
	f.emitCustomEvent(fishingToolEventName, request)
	return request.Variant
}

// catchInterval is the ticks to the next catch roll
// (catchInterval, plugin.js:712-716): the tool's interval,
// as content adjusts it through the "fishing:interval"
// query (a league relic's faster harpoon); floored at 1.
func (f *fishing) catchInterval(player *game.Player, tool *FishingTool) int {
	request := &FishingIntervalRequest{Player: player, Ticks: tool.IntervalTicks()}
	f.emitCustomEvent(fishingIntervalEventName, request)
	if request.Ticks < 1 {
		return 1
	}
	return request.Ticks
}

// findBait finds the bait the tool uses (findBait,
// plugin.js:728-731): inventory first, then storage
// ("storage:amount"). It returns the bait item id, or 0
// when the player has none.
func (f *fishing) findBait(player *game.Player, tool *FishingTool) int {
	if player == nil || tool == nil {
		return 0
	}
	inventory := player.Inventory()
	for _, id := range tool.Bait {
		if inventory != nil && inventory.Contains(id) {
			return id
		}
	}
	for _, id := range tool.Bait {
		if f.storageAmount(player, id) > 0 {
			return id
		}
	}
	return 0
}

// useBait spends one bait (useBait, plugin.js:739-744):
// inventory first, else the "storage:take" query
// (unanswered → not consumed).
func (f *fishing) useBait(player *game.Player, bait int) {
	if player == nil || bait == 0 {
		return
	}
	if inventory := player.Inventory(); inventory != nil && inventory.Contains(bait) {
		inventory.DeleteNumber(bait, 1)
		return
	}
	f.storageTake(player, bait, 1)
}

// hasRoom reports whether this fish has somewhere to go
// (hasRoom, plugin.js:764-767): a free slot, an open fish
// barrel ("storage:room"), or its own stackable stack.
func (f *fishing) hasRoom(player *game.Player, fish *Fish) bool {
	if player == nil || fish == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil {
		return false
	}
	if inventory.GetFreeSlots() > 0 {
		return true
	}
	if f.storageRoom(player, fish.ID) {
		return true
	}
	if inventory.Contains(fish.ID) {
		if defs := player.ItemDefinitions(); defs != nil && defs.IsStackable(fish.ID) {
			return true
		}
	}
	return false
}

// inventoryBlocks reports whether a full inventory stops
// fishing (inventoryBlocks, plugin.js:770-775): a full
// inventory blocks unless one of the tool's fish stacks in
// it or an open fish barrel has room ("storage:room").
func (f *fishing) inventoryBlocks(player *game.Player, tool *FishingTool) bool {
	if player == nil || tool == nil {
		return false
	}
	inventory := player.Inventory()
	if inventory == nil || !inventory.IsFull() {
		return false
	}
	for _, fish := range tool.Fish {
		if fish == nil {
			continue
		}
		if inventory.Contains(fish.ID) {
			if defs := player.ItemDefinitions(); defs != nil && defs.IsStackable(fish.ID) {
				return false
			}
		}
	}
	for _, fish := range tool.Fish {
		if fish == nil {
			continue
		}
		if f.storageRoom(player, fish.ID) {
			return false
		}
	}
	return true
}

// giveCatch gives the catch to the player (giveCatch,
// plugin.js:755-760): storage takes what it can
// ("storage:gather"); the remainder goes to the inventory.
func (f *fishing) giveCatch(player *game.Player, itemID, amount int, xp float64) {
	if player == nil || amount <= 0 {
		return
	}
	stored := f.storageGather(player, itemID, amount, xp)
	if amount <= stored {
		return
	}
	if inventory := player.Inventory(); inventory != nil {
		inventory.Add(game.NewItem(itemID, amount-stored), true)
	}
}

// hasToolRequirements checks the player can fish with this
// tool, in exact TS order (hasToolRequirements,
// plugin.js:770-813): Fishing level (unboostable → max level,
// else current; the levelStatement box or "You need a Fishing
// level of at least N to do this.") → requires skills → quest
// (always passes in Go — see questAllows) → findTool variant
// (noToolStatement box, else noToolMessage chat line, else
// the default line) → worn gloves ("You need to wear ice
// gloves to handle these eels.") → bait (findBait, else the
// noBaitStatement box or the default bait line) / vessel
// ready (KarambwanVessel.ready). It returns the usable
// variant, or nil when the player cannot fish.
func (f *fishing) hasToolRequirements(player *game.Player, tool *FishingTool) *ToolVariant {
	if player == nil || tool == nil {
		return nil
	}
	level := fishingLevel(player)
	if tool.Unboostable {
		level = maxFishingLevel(player)
	}
	if level < tool.Level {
		if tool.LevelStatement != nil {
			f.statement(player, tool.LevelStatement(tool.Level))
		} else {
			player.SendMessage(fmt.Sprintf(DefaultLevelMessage, tool.Level))
		}
		return nil
	}
	for _, req := range tool.Requires {
		if skillLevel(player, req.Skill) < req.Level {
			player.SendMessage(fmt.Sprintf("You need a %s level of at least %d to do this.", game.SkillName(req.Skill), req.Level))
			return nil
		}
	}
	if tool.Quest != "" && !f.questAllows(player, tool.Quest) {
		player.SendMessage(fmt.Sprintf("You need to complete %s to fish here.", tool.Quest))
		return nil
	}
	variant := f.findTool(player, tool)
	if variant == nil {
		if tool.NoToolStatement != "" {
			f.statement(player, tool.NoToolStatement)
		} else {
			message := tool.NoToolMessage
			if message == "" {
				message = DefaultNoToolMessage
			}
			player.SendMessage(message)
		}
		return nil
	}
	if len(tool.Worn) > 0 {
		worn := false
		if equipment := player.Equipment(); equipment != nil {
			for _, id := range tool.Worn {
				if equipment.Contains(id) {
					worn = true
					break
				}
			}
		}
		if !worn {
			player.SendMessage(IceGlovesMessage)
			return nil
		}
	}
	// A stand-in tool may fish without bait (`baitless`, a
	// league relic's bare hands — plugin.js:801): Go's
	// ToolVariant carries no Baitless field (Worker A), and
	// no "fishing:tool" answerer exists in Go, so every
	// variant found here needs its bait. Documented, not
	// silently equivalent.
	if (len(tool.Bait) > 0 && f.findBait(player, tool) == 0) || (tool.Vessel && !f.karambwanReady(player)) {
		if tool.NoBaitStatement != "" {
			f.statement(player, tool.NoBaitStatement)
		} else {
			player.SendMessage(DefaultNoBaitMessage)
		}
		return nil
	}
	return variant
}

// hasHarpoon reports whether the player carries or wields any
// harpoon (hasHarpoon, plugin.js:575-578): any HARPOONS
// variant id, inventory or equipment.
func (f *fishing) hasHarpoon(player *game.Player) bool {
	if f.tables == nil || player == nil {
		return false
	}
	inventory := player.Inventory()
	equipment := player.Equipment()
	for _, variant := range f.tables.Harpoons {
		for _, id := range variant.IDs {
			if inventory != nil && inventory.Contains(id) {
				return true
			}
			if equipment != nil && equipment.Contains(id) {
				return true
			}
		}
	}
	return false
}

// barehandTaught reports whether Otto has taught barehanded
// fishing (barehandTaught, plugin.js:576-580): the
// "fishing:barehand-unlocked" query, unanswered → locked
// (the members-only Barbarian Training is not loaded in Go).
func (f *fishing) barehandTaught(player *game.Player) bool {
	request := &BarehandUnlockedRequest{Player: player, Allowed: false}
	f.emitCustomEvent(barehandUnlockedEventName, request)
	return request.Allowed
}

// getSpotTool finds the tool for a spot click (getSpotTool,
// plugin.js:549-562): SPOT_TOOLS_BY_NPC first, else
// SPOT_TOOLS_BY_OPTIONS by the spot's joined non-empty option
// list, then the harpoon→barehand fallback (BAREHAND_SHARK /
// BAREHAND_HARPOON when the player has no harpoon and
// fishing:barehand-unlocked answers true; unanswered →
// locked, so the harpoon's own refusal plays). It returns nil
// when the spot fishes no tool for the clicked option —
// false leaves non-fishing spots to others.
func (f *fishing) getSpotTool(npcID int, actions []string, clickType int, player *game.Player) *FishingTool {
	if f.tables == nil {
		return nil
	}
	var option string
	if clickType >= 1 && clickType <= len(actions) {
		option = actions[clickType-1]
	}
	var tools map[string]*FishingTool
	if byNPC := f.tables.SpotTools(npcID); byNPC != nil {
		tools = byNPC
	} else {
		var joined []string
		for _, action := range actions {
			if action != "" {
				joined = append(joined, action)
			}
		}
		tools = f.tables.SpotToolsByOptions[strings.Join(joined, "/")]
	}
	if tools == nil || option == "" {
		return nil
	}
	tool := tools[option]
	if tool == nil {
		return nil
	}
	if tool.Name == "SHARK_HARPOON" && player != nil && !f.hasHarpoon(player) && f.barehandTaught(player) {
		return f.tables.ToolByName("BAREHAND_SHARK")
	}
	if tool.Name == "HARPOON" && player != nil && !f.hasHarpoon(player) && f.barehandTaught(player) {
		return f.tables.ToolByName("BAREHAND_HARPOON")
	}
	return tool
}

// continuesFishing reports whether content keeps fishing when
// a spot would normally end the session (continuesFishing,
// plugin.js:1000-1004): the "fishing:continue" query,
// handled defaults false.
func (f *fishing) continuesFishing(player *game.Player) bool {
	request := &FishingContinueRequest{Player: player, Handled: false}
	f.emitCustomEvent(fishingContinueEventName, request)
	return request.Handled
}

// ---- catch chance, rolls, landing ----

// catchChance returns the per-roll catch chance (catchChance,
// plugin.js:725-732): interpolate low (level 1) to high
// (level 99) out of 256, with the tool bonus scaling low and
// high first, floored, as the Wiki's dragon/crystal charts do.
// Note the single floor: TS floors the SUM
// (low·(99−level)/98 + high·(level−1)/98 + 0.5), not each
// term (unlike Woodcutting's cut chance, which floors each).
func catchChance(level int, fish *Fish, bonus int, roll int) float64 {
	if fish == nil || roll < 0 || roll >= len(fish.Rolls) {
		return 0
	}
	baseLow, baseHigh := fish.Rolls[roll][0], fish.Rolls[roll][1]
	low := baseLow * bonus / 100
	high := baseHigh * bonus / 100
	value := math.Floor(float64(low*(99-level))/98 + float64(high*(level-1))/98 + 0.5)
	return math.Min(1, (1+value)/256)
}

// tableCatchChance is catchChance for a drop table
// (rollCatch's table leg, plugin.js:937): the table's rolls
// row 0, at the tool bonus.
func tableCatchChance(level int, table *DropTable, bonus int) float64 {
	if table == nil || len(table.Rolls) == 0 {
		return 0
	}
	standIn := &Fish{Rolls: table.Rolls}
	return catchChance(level, standIn, bonus, 0)
}

// rollTable lands one item off a drop table (rollTable,
// plugin.js:920-928): the pre-rolls first, each at its own
// chance, else one weighted pick (an over-total roll lands on
// the last entry, as in TS), plus any tertiaries on top.
func (f *fishing) rollTable(table *DropTable) []*Fish {
	if table == nil {
		return nil
	}
	var caught *Fish
	for _, pre := range table.PreRoll {
		if f.randFloat() < pre.Chance {
			caught = pre.Fish
			break
		}
	}
	if caught == nil {
		total := 0
		for _, weighted := range table.Weighted {
			total += weighted.Weight
		}
		pick := f.randFloat() * float64(total)
		for _, weighted := range table.Weighted {
			pick -= float64(weighted.Weight)
			if pick < 0 {
				caught = weighted.Fish
				break
			}
		}
		if caught == nil && len(table.Weighted) > 0 {
			caught = table.Weighted[len(table.Weighted)-1].Fish
		}
	}
	if caught == nil {
		return nil
	}
	out := []*Fish{caught}
	for _, tertiary := range table.Tertiary {
		if f.randFloat() < tertiary.Chance {
			out = append(out, tertiary.Fish)
		}
	}
	return out
}

// rollsFor returns a fish's charts for this player (rollsFor,
// plugin.js:929-934): a diary's better roll once the diary is
// done. The "diary:is-complete" query is unanswered in Go, so
// the standard rolls always win in production.
func (f *fishing) rollsFor(player *game.Player, fish *Fish) *Fish {
	if fish == nil || fish.DiaryRolls == nil {
		return fish
	}
	better := fish.DiaryRolls
	request := &DiaryCompleteRequest{Player: player, Diary: better.Diary, Tier: better.Tier, Complete: false}
	f.emitCustomEvent(diaryCompleteEventName, request)
	if !request.Complete {
		return fish
	}
	charted := *fish
	charted.Rolls = better.Rolls
	return &charted
}

// rollCatch rolls one attempt (rollCatch, plugin.js:936-956):
// a table tool rolls its table at the table chance, else the
// cascade — fish highest-first, skipping members fish on free
// worlds, filtering by level and requires, with diary rolls
// and a "fishing:catch-chance" query per roll (unanswered →
// the computed chance). Multi tools land every success; other
// tools return on the first.
func (f *fishing) rollCatch(player *game.Player, tool *FishingTool, bonus int) []*Fish {
	if player == nil || tool == nil {
		return nil
	}
	level := f.catchLevel(player)
	if tool.Table != nil {
		if f.randFloat() < tableCatchChance(level, tool.Table, bonus) {
			return f.rollTable(tool.Table)
		}
		return nil
	}
	var caught []*Fish
	for _, fish := range tool.Fish {
		if fish == nil {
			continue
		}
		if !f.membersWorld && f.isMembersItem != nil && f.isMembersItem(fish.ID) {
			continue
		}
		if fishingLevel(player) < fish.Level || !f.meetsLevels(player, fish.Requires) {
			continue
		}
		charted := f.rollsFor(player, fish)
		for roll := range charted.Rolls {
			chance := catchChance(level, charted, bonus, roll)
			request := &FishingCatchChanceRequest{Player: player, ToolID: tool.ID, ItemID: fish.ID, Chance: chance}
			f.emitCustomEvent(fishingCatchChanceEventName, request)
			chance = math.Max(0, math.Min(1, request.Chance))
			if f.randFloat() < chance {
				caught = append(caught, fish)
				if !tool.Multi {
					return caught
				}
			}
		}
	}
	return caught
}

// bestCatch returns the best fish the player can land with
// this tool (bestCatch, plugin.js:719-724): the first
// non-junk fish at or below the player's level that meets its
// requirements — or a table roll. The "fishing:attempt" second
// chance lands this. Note TS checks no members gate here (the
// rollCatch cascade does); Go matches.
func (f *fishing) bestCatch(player *game.Player, tool *FishingTool) []*Fish {
	if player == nil || tool == nil {
		return nil
	}
	if tool.Table != nil {
		return f.rollTable(tool.Table)
	}
	for _, fish := range tool.Fish {
		if fish == nil || fish.Junk {
			continue
		}
		if fishingLevel(player) >= fish.Level && f.meetsLevels(player, fish.Requires) {
			return []*Fish{fish}
		}
	}
	return nil
}

// bigOrFish swaps a trophy fish in (bigOrFish, plugin.js:94-
// 96): a 1-in-n chance (1/1000 bass, 1/2500 swordfish, 1/5000
// shark) to land the big fish in place of the base fish, with
// the base fish's XP and pet base. The trophy is a shallow
// copy of the base fish with the trophy id, caught text and
// message; Big is cleared so it cannot re-roll.
func (f *fishing) bigOrFish(fish *Fish) *Fish {
	if fish == nil || fish.Big == nil || fish.Big.Chance <= 0 {
		return fish
	}
	if f.randFloat() >= 1.0/float64(fish.Big.Chance) {
		return fish
	}
	big := fish.Big
	trophy := *fish
	trophy.ID = big.ID
	trophy.Item = big.Item
	trophy.Caught = big.BigCaught()
	trophy.Message = func(_ int) string { return big.BigMessage() }
	trophy.Big = nil
	return &trophy
}

// landExtra doubles a catch through Rada's blessing and
// spirit flakes (landExtra, plugin.js:779-792): one roll
// covers both — the flakes' 50% first, then the blessing's
// chance on top (rollExtraFish, Worker C). No experience or
// bait for the extra. A blessing extra on junk never lands
// (the flakes double junk too). An infernal harpoon gives the
// extra its own chance to be cooked, with the blessing line
// still shown. The extra needs room of its own.
func (f *fishing) landExtra(player *game.Player, variant *ToolVariant, fish *Fish, amount int, flaked bool) {
	if player == nil || fish == nil {
		return
	}
	// SpiritFlakes.EXTRA_FISH_CHANCE (SpiritFlakes.Fishing.js),
	// the named constant from Worker C's spiritflakes.go.
	flakeChance := 0.0
	if flaked {
		flakeChance = SpiritFlakeExtraFishChance
	}
	source := f.rollExtraFish(player, flakeChance)
	if source == "" || (source == "blessing" && fish.Junk) {
		return
	}
	// RadasBlessing.EXTRA_FISH_MESSAGE (RadasBlessing.Fishing.
	// js), the named constant from Worker C's radasblessing.go.
	message := ""
	if source == "blessing" {
		message = RadaBlessingExtraFishMessage
	}
	if variant != nil && variant.Infernal && f.tryCookFish(player, fish.ID) {
		if message != "" {
			f.sendSpam(player, message)
		}
		return
	}
	if !f.hasRoom(player, fish) {
		return
	}
	f.giveCatch(player, fish.ID, amount, 0)
	if message != "" {
		f.sendSpam(player, message)
	}
}

// landCatch lands a batch (landCatch, plugin.js:838-900): the
// angler multiplier on XP; hasRoom per fish (a full inventory
// that still takes karambwanji loses the shrimps — the loop
// breaks on the first fish with nowhere to go); bigOrFish;
// amount (karambwanji floor(level/5)+1, minnows by level);
// a flake consumed per landed fish (SpiritFlakes.
// consumeFlake, not for PARK_NET); giveCatch (storage first,
// rest to inventory); the catch message; junk →
// "fishing:junk"; infernal cook attempt per fish
// (tryCookFish — never fires: no "cooking:raw-xp" answerer,
// the infernal-axe precedent); crystal charge per caught
// fish; XP via SkillManager.AddExperiences + extraXp skills;
// landExtra per landed fish (Rada/flakes); clue bottles once
// per batch (skip for the tutorial spot id 3317);
// "fishing:success" per fish kind with petBase (no pet system
// in Go — emit honestly, no answerer, the Woodcutting
// "woodcutting:success" precedent); bait consumed only on
// catch, one per cast. It returns whether a listener took
// over the session (event.stop).
//
// XP INTEGER TRUNCATION: Go stores integer experience and
// AddExperiences takes an int, so the float XP is truncated
// toward zero at the grant site: a 26.1-XP minnow catch grants
// 26, a 7.5-XP park find grants 7. The same known structural
// gap as Woodcutting (gap 16) and Fletching (gap 17) —
// documented here, not rounded away.
func (f *fishing) landCatch(player *game.Player, tool *FishingTool, variant *ToolVariant, caught []*Fish, spotID int) bool {
	if player == nil || tool == nil {
		return false
	}
	xpMultiplier := f.anglerXPMultiplier(player)
	type landedEntry struct {
		fish   *Fish
		kind   *Fish
		amount int
		flaked bool
	}
	var landed []landedEntry
	for _, kind := range caught {
		fish := f.bigOrFish(kind)
		if !f.hasRoom(player, fish) {
			break
		}
		amount := fish.AmountFor(fishingLevel(player), maxFishingLevel(player))
		flaked := tool.Name != "PARK_NET" && f.consumeFlake(player)
		landed = append(landed, landedEntry{fish: fish, kind: kind, amount: amount, flaked: flaked})
		if variant == nil || !variant.Infernal || !f.tryCookFish(player, fish.ID) {
			f.giveCatch(player, fish.ID, amount, fish.XP*xpMultiplier)
			f.sendSpam(player, fish.MessageFor(amount))
			// Junk (a casket, an old boot) is no fish, so it is
			// its own event: "fishing:junk" { player, itemId,
			// amount }.
			if fish.Junk {
				f.emitCustomEvent(fishingJunkEventName, &FishingJunkEvent{Player: player, ItemID: fish.ID, Amount: amount})
			}
		}
		if variant != nil && variant.Crystal {
			f.tryUseCrystalCharge(player)
		}
		if skills := player.Skills(); skills != nil {
			skills.AddExperiences(game.SkillFishing, int(fish.XP*xpMultiplier))
			for _, extra := range fish.ExtraXP {
				skills.AddExperiences(extra.Skill, extra.XP)
			}
		}
	}
	// Rada's blessing and the flakes roll once for each fish,
	// after the whole catch has its slots.
	for _, entry := range landed {
		f.landExtra(player, variant, entry.fish, entry.amount, entry.flaked)
	}
	// One bottle roll set per natural catch batch; Rada's extra
	// is not another action. The tutorial pond is outside the
	// supported scope; no retail exclusion is inferred. The
	// park uses its own base even though its successful catches
	// are labelled junk.
	if spotID != NPCFishingSpot43 {
		var clueFish *Fish
		for _, entry := range landed {
			if f.tables != nil && f.tables.IsClueBottleFish(entry.kind) {
				clueFish = entry.kind
				break
			}
		}
		clueBase := 0
		if len(landed) > 0 {
			switch {
			case tool.ClueBase != 0:
				clueBase = tool.ClueBase
			case tool.Name == "PARK_NET":
				clueBase = ParkClueBase
			case clueFish != nil:
				clueBase = clueFish.PetBase
			}
		}
		if clueBase > 0 {
			f.rollClueBottles(player, clueBase)
		}
	}
	// A big net catch rolls the pet once per kind of fish in
	// it. A listener may take over the session (the tutorial
	// stops after one catch).
	stop := false
	seen := make(map[*Fish]bool)
	for _, entry := range landed {
		if entry.fish.Junk || seen[entry.fish] {
			continue
		}
		seen[entry.fish] = true
		barehand := variant != nil && variant.Barehand
		event := &FishingSuccessEvent{Player: player, Skill: game.SkillFishing, ItemID: entry.fish.ID, PetBase: entry.fish.PetBase, Barehand: barehand, Stop: false}
		f.emitCustomEvent(fishingSuccessEventName, event)
		if event.Stop {
			stop = true
		}
	}
	// Bait and feathers are only used up by a catch, one per
	// cast.
	if bait := f.findBait(player, tool); bait != 0 {
		f.useBait(player, bait)
	}
	return stop
}

// ---- sessions: start / stop / tick ----

// stopFishing stops the player's fishing (stopFishing,
// plugin.js:815-823): a missing session is a no-op; the
// animation resets unless the caller keeps it (the live
// inventory-full and disconnect stops keep the last cast
// animation running).
func (f *fishing) stopFishing(player *game.Player, resetAnimation bool) {
	if player == nil {
		return
	}
	if _, ok := f.sessions[player]; !ok {
		return
	}
	delete(f.sessions, player)
	if resetAnimation {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// onPlayerDisconnect stops the session with no animation reset
// (plugin.js:1116-1118).
func (f *fishing) onPlayerDisconnect(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	f.stopFishing(event.Player, false)
}

// startFishing starts a fishing session (startFishing,
// plugin.js:955-977) in exact order: hasToolRequirements →
// inventoryBlocks (full inventory → inventoryFull) → stop old
// session (no anim reset) → store session (nextAnimationTick
// tick+5, nextCatchTick tick+min(firstRoll, catchInterval),
// followMessageTick +1 if any) → spot onStart (nil in this
// phase — MinnowPlatform deferred) → vessel onStart
// (Karambwan load) → spam startMessage → perform start
// animation. It always returns true once a tool was found:
// requirement failures are claimed too (their messages are
// the plugin's own), exactly as TS returns true on every
// path — startFishingInteraction returns false only when no
// tool was found at all.
func (f *fishing) startFishing(player *game.Player, npc *game.NPC, tool *FishingTool) bool {
	if player == nil || npc == nil || tool == nil {
		return false
	}
	if f.hasToolRequirements(player, tool) == nil {
		return true
	}
	if f.inventoryBlocks(player, tool) {
		f.inventoryFull(player)
		return true
	}
	f.stopFishing(player, false)
	f.ensureTask()
	// Lazily init the sessions map: newFishing builds it, but
	// a directly-constructed instance (a test helper that
	// sets tables/ids field by field) must not panic on the
	// first cast.
	if f.sessions == nil {
		f.sessions = make(map[*game.Player]*fishingSession)
	}
	// nextCatchTick: tick + min(firstRoll ?? ∞, catchInterval)
	// (plugin.js:994): the first roll comes at the tool's
	// firstRoll when one is named and sooner than the
	// (query-adjusted) interval, else at the interval.
	nextCatch := f.catchInterval(player, tool)
	if tool.HasFirstRoll && tool.FirstRoll < nextCatch {
		nextCatch = tool.FirstRoll
	}
	session := &fishingSession{
		npc:               npc,
		npcID:             npc.ContentID(),
		spotX:             npc.Location().X,
		spotY:             npc.Location().Y,
		tool:              tool,
		nextAnimationTick: f.tick + FishingAnimationIntervalTicks,
		nextCatchTick:     f.tick + int64(nextCatch),
	}
	if tool.FollowMessage != "" {
		session.followMessageTick = f.tick + 1
	}
	f.sessions[player] = session
	// tool.spot?.onStart: no spot module exists in Go (the
	// MinnowPlatform mechanics are deferred, PHASE35_PLAN.md
	// §7), so a non-empty Spot name runs nothing here.
	if tool.Vessel {
		f.karambwanLoad(player)
	}
	f.sendSpam(player, tool.StartMessage)
	player.PerformAnimation(tool.AnimationID, 0)
	return true
}

// processFishingTick ticks every session (FishingTask.execute,
// plugin.js:1001-1085) in exact order: dead/unregistered →
// delete; moving / in combat → stop; level-up paused → skip;
// NPC gone/wrong id → stop; spot moved: stop, and if no spot
// module and "fishing:continue" answers handled and same z
// and within 24 → walkToEntity and restart within 2
// (validated again inside the callback) → stop if player >2
// tiles from spot → hasToolRequirements re-check each tick
// (stop if fail) → inventoryBlocks → stop (no anim reset) +
// inventoryFull → follow message at +1 tick + re-cast anim →
// loop animation every 5 ticks when !animateOnRoll → catch
// tick: next roll at catchInterval ("fishing:interval" query,
// min 1), loop animation on roll when animateOnRoll →
// rollCatch → "fishing:attempt" query (unanswered → keep the
// miss) → vessel bait per attempt → miss message →
// catchAnimation → spot takesCatch (nil this phase) →
// "fishing:catch" event → landCatch → stop if stopAfterCatch
// / no tool requirements / random 1/90 spot move
// (non-stationary, no spot module, "fishing:continue"
// unanswered).
//
// SEAM GAPS: the force-movement skip (player.getForceMovement
// () != null) cannot run — no ForceMovement field or system
// exists in Go (movement_queue.go documents its absence) — so
// a forced move never pauses a session here. The NPC lookup
// reads the session's NPC pointer instead of the world NPC
// list (see fishingSession); the id + tile checks are the
// same. The spot-move walk uses MovementQueue.WalkToEntity
// (the Phase 17 seam), with game.NPCTarget.
func (f *fishing) processFishingTick(currentTick int64) {
	if f.tables == nil {
		return
	}
	for player, session := range f.sessions {
		if player == nil || !player.IsRegistered() || skillLevel(player, game.SkillHitpoints) <= 0 {
			delete(f.sessions, player)
			continue
		}
		moving := false
		if mq := player.MovementQueue(); mq != nil && (mq.Size() > 0 || mq.IsMoving()) {
			moving = true
		}
		inCombat := false
		if combat := player.GetCombat(); combat != nil && combat.Target() != nil {
			inCombat = true
		}
		if moving || inCombat {
			f.stopFishing(player, true)
			continue
		}
		if session.levelUpPaused {
			continue
		}
		npc := session.npc
		if npc == nil || !npc.IsRegistered() || npc.ContentID() != session.npcID {
			f.stopFishing(player, true)
			continue
		}
		npcLoc := npc.Location()
		if npcLoc.X != session.spotX || npcLoc.Y != session.spotY {
			f.stopFishing(player, true)
			// A spot module that moves its own spots (the
			// minnow platform) is never followed: the Wiki's
			// Endless Harvest notes, "You will not chase
			// minnows, and will stop harvesting when they
			// move." A non-empty Spot name means a module
			// owns the spot, so no follow either.
			if session.tool.Spot == "" && f.continuesFishing(player) &&
				player.Location().Z == npcLoc.Z && player.Location().IsWithinDistance(npcLoc, 24) {
				tool := session.tool
				spotNPCID := session.npcID
				followNPC := npc
				if mq := player.MovementQueue(); mq != nil {
					// The normal interaction route cancels on
					// a manual click and validates the target.
					mq.WalkToEntity(game.NPCTarget(followNPC), func() {
						if !player.IsRegistered() || skillLevel(player, game.SkillHitpoints) <= 0 ||
							followNPC == nil || !followNPC.IsRegistered() || followNPC.ContentID() != spotNPCID ||
							player.Location().Z != followNPC.Location().Z ||
							!player.Location().IsWithinDistance(followNPC.Location(), 2) || !f.continuesFishing(player) {
							return
						}
						face := followNPC.Location()
						player.SetPositionToFace(&face)
						f.startFishing(player, followNPC, tool)
					})
				}
			}
			continue
		}
		if !player.Location().IsWithinDistance(npcLoc, 2) {
			f.stopFishing(player, true)
			continue
		}
		variant := f.hasToolRequirements(player, session.tool)
		if variant == nil {
			f.stopFishing(player, true)
			continue
		}
		if f.inventoryBlocks(player, session.tool) {
			f.stopFishing(player, false)
			f.inventoryFull(player)
			continue
		}
		if session.followMessageTick != 0 && currentTick == session.followMessageTick {
			f.sendSpam(player, session.tool.FollowMessage)
			player.PerformAnimation(session.tool.AnimationID, 0)
		}
		// rsprox captures: the animation restarts on every roll,
		// caught or not.
		if !session.tool.AnimateOnRoll && currentTick >= session.nextAnimationTick {
			player.PerformAnimation(session.tool.LoopAnimationID, 0)
			session.nextAnimationTick = currentTick + FishingAnimationIntervalTicks
		}
		if currentTick < session.nextCatchTick {
			continue
		}
		session.nextCatchTick = currentTick + int64(f.catchInterval(player, session.tool))
		if session.tool.AnimateOnRoll {
			player.PerformAnimation(session.tool.LoopAnimationID, 0)
		}
		caught := f.rollCatch(player, session.tool, variant.Bonus)
		// Content may turn a miss into a catch
		// ("fishing:attempt", a league relic's second chance):
		// the best fish the player can land.
		attempt := &FishingAttemptRequest{Player: player, Success: len(caught) > 0}
		f.emitCustomEvent(fishingAttemptEventName, attempt)
		if len(caught) == 0 && attempt.Success {
			caught = f.bestCatch(player, session.tool)
		}
		// A vessel's bait goes with every attempt, caught or
		// not (Wiki: Raw karambwan).
		if session.tool.Vessel {
			f.karambwanUseBait(player)
		}
		if len(caught) == 0 {
			if session.tool.MissMessage != "" {
				f.sendSpam(player, session.tool.MissMessage)
			}
			continue
		}
		catchAnimation := caught[0].CatchAnimation
		if catchAnimation == 0 {
			catchAnimation = session.tool.CatchAnimationID
		}
		if catchAnimation != 0 {
			player.PerformAnimation(catchAnimation, 0)
		}
		// tool.spot?.takesCatch: no spot module exists in Go
		// (MinnowPlatform deferred), so no catch is ever
		// taken over here.
		catchRequest := &FishingCatchEvent{Player: player, NPCID: session.npcID, Caught: caught}
		f.emitCustomEvent(fishingCatchEventName, catchRequest)
		stopAfterCatch := f.landCatch(player, session.tool, variant, catchRequest.Caught, session.npcID)
		// Spots without their own movement still stop at random
		// instead of moving. A full inventory waits for the next
		// tick's check, which says so.
		if stopAfterCatch || f.hasToolRequirements(player, session.tool) == nil ||
			(session.tool.Spot == "" && !session.tool.Stationary && f.randInt(1, 90) == 1 && !f.continuesFishing(player)) {
			f.stopFishing(player, true)
		}
	}
}

// pauseFishing holds the session through a level-up pop-up
// (pauseFishing, plugin.js:1109-1111): skillactions.
// PauseForLevelUp with the sessions converted to the helper's
// map type and a stop callback that keeps the animation.
func (f *fishing) pauseFishing(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(f.sessions))
	for player, sess := range f.sessions {
		sessions[player] = sess
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		f.stopFishing(player, false)
	}, nil)
}

// ---- spot interaction ----

// startFishingInteraction handles a spot click
// (startFishingInteraction, plugin.js:1121-1124): look the
// tool up by spot NPC id, definition actions and click type,
// return false when the spot fishes no tool (leaving
// non-fishing spots to others), else start fishing — which
// always claims the click once a tool was found.
func (f *fishing) startFishingInteraction(event *plugins.NpcInteractionEvent) bool {
	if event == nil || f.tables == nil {
		return false
	}
	tool := f.getSpotTool(event.NpcID, event.Actions, event.ClickType, event.Player)
	if tool == nil {
		return false
	}
	return f.startFishing(event.Player, event.NPC, tool)
}
