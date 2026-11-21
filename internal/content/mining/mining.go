// Mining: the core loop (Mining.plugin.js:1-1221). The sub-units
// live in their own files and hang off the same *mining receiver.
package mining

import (
	"math"
	"math/rand"
	"regexp"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
	"tsps-go/internal/protocol"
)

// ---- session state ----

// objectPrivateArea reads a plugins.GameObject's private area:
// the concrete *game.GameObject carries it (the woodcutting
// privateAreaOf precedent); any other implementation reads as
// the shared world.
func objectPrivateArea(obj plugins.GameObject) *game.PrivateArea {
	if concrete, ok := obj.(*game.GameObject); ok && concrete != nil {
		return concrete.PrivateArea()
	}
	return nil
}

// withinInteractionDistance reports whether a skilling action
// on this loc may continue from that tile: within 2 tiles of
// its footprint (GameObject.ts:151-153). The concrete
// *game.GameObject answers directly; other implementations
// (an NPC rock) answer through the same shape; the fallback
// is Chebyshev distance on the same plane.
func withinInteractionDistance(obj plugins.GameObject, from game.Location) bool {
	if obj == nil {
		return false
	}
	if withDist, ok := obj.(interface {
		IsWithinInteractionDistance(game.Location, game.ObjectDef) bool
	}); ok {
		return withDist.IsWithinInteractionDistance(from, game.ObjectDef{SizeX: 1, SizeY: 1})
	}
	at := obj.Location()
	if from.Z != at.Z {
		return false
	}
	dx := from.X - at.X
	if dx < 0 {
		dx = -dx
	}
	dy := from.Y - at.Y
	if dy < 0 {
		dy = -dy
	}
	if dx < dy {
		return dy <= 2
	}
	return dx <= 2
}

// ---- session state ----

// session is one player's active mining (startMining,
// plugin.js:667-701).
type session struct {
	rock          *Rock
	pickaxe       Pickaxe
	objectID      int
	location      game.Location
	privateArea   *game.PrivateArea
	swingTick     int64 // -1 means the swing already played
	hasSwingTick  bool
	nextRollTick  int64
	levelUpPaused bool
	npcRock       *npcRockState
}

// npcRockState is a rock that is an NPC (mining:mine-npc-rock,
// plugin.js:1137-1159): the NPC and its stands() check.
type npcRockState struct {
	npc    *game.NPC
	stands func() bool
}

// SetLevelUpPaused implements skillactions.Session.
func (s *session) SetLevelUpPaused(paused bool) { s.levelUpPaused = paused }

// yieldState is one standing rock's multi-ore yield progress
// (ROCK_YIELDS, plugin.js:332-333).
type yieldState struct {
	mined int
	total int
}

// gloveSaveState is one standing rock's glove saves
// (ROCK_GLOVE_SAVES, plugin.js:336-338).
type gloveSaveState struct {
	used          int
	baseExhausted bool
}

// rockTimerState is one standing rock's depletion countdown
// (ROCK_TIMERS, plugin.js:342-343).
type rockTimerState struct {
	left int
	tick int64
}

// mining is the shared plugin instance behind Plugin.
type mining struct {
	api           *plugins.Api
	emit          func(name string, payload any)
	tables        *Tables
	membersWorld  bool
	isMembersItem func(id int) bool
	tasks         *game.TaskManager
	objects       plugins.ObjectManagerAPI
	taskScheduled bool
	tick          int64
	sessions      map[*game.Player]*session
	rollReady     map[*game.Player]int64
	rockYields    map[plugins.GameObject]*yieldState
	gloveSaves    map[plugins.GameObject]*gloveSaveState
	rockTimers    map[plugins.GameObject]*rockTimerState
	randFloat     func() float64
	randInt       func(min, max int) int
	// Sub-unit state.
	water   *calcifiedWaterState
	ancient *ancientState
}

func newMining(tables *Tables) *mining {
	return &mining{
		tables:       tables,
		sessions:     make(map[*game.Player]*session),
		rollReady:    make(map[*game.Player]int64),
		rockYields:   make(map[plugins.GameObject]*yieldState),
		gloveSaves:   make(map[plugins.GameObject]*gloveSaveState),
		rockTimers:   make(map[plugins.GameObject]*rockTimerState),
		water:        newCalcifiedWaterState(),
		ancient:      newAncientState(),
		membersWorld: true,
		randFloat:    func() float64 { return rand.Float64() },
		randInt: func(min, max int) int {
			if max <= min {
				return min
			}
			return min + rand.Intn(max-min+1)
		},
	}
}

// shared is the instance behind Plugin; SetTables and friends
// operate on it (the woodcutting precedent).
var shared = newMining(nil)

func init() {
	plugins.Register(Plugin)
}

// SetTables installs the resolved tables for the registered
// Plugin value. cmd/server calls it BEFORE hub Load: Register
// captures the shared instance and builds its object
// registrations from the tables, so a call after Load is too
// late for them. With no tables installed, every handler
// degrades to a no-op.
func SetTables(t *Tables) { shared.tables = t }

// SetMembersWorld installs the members-world flag (TS:
// WorldDefinition.isMembersWorld()). cmd/server calls it before
// hub Load with the world definition's flag; it defaults true.
func SetMembersWorld(members bool) { shared.membersWorld = members }

// SetIsMembersItem installs the item-members lookup (TS:
// CacheDefinitions.getItem(id)?.isMembers). cmd/server calls it
// before hub Load with a closure over the loaded cache
// definitions; nil means every item counts.
func SetIsMembersItem(fn func(id int) bool) { shared.isMembersItem = fn }

// Tables returns the installed tables (for tests and wiring).
func TablesOf() *Tables { return shared.tables }

// ---- plugin ----

var Plugin = shared.plugin()

func (m *mining) plugin() plugins.Plugin {
	return plugins.Plugin{
		Name:        "Mining",
		Description: "Mining: rocks and ores with pickaxes, respawn timers and depleted rocks, plus events other content uses for pickaxe and ore checks.",
		Members:     false,
		Register: func(api *plugins.Api) {
			m.api = api
			m.emit = api.EmitCustomEvent
			m.register(api)
		},
	}
}

// taskManager resolves the world's TaskManager lazily: the hub
// installs it after plugin Load (nil until installed, per the
// hub contract), so a Register-time capture reads nil in
// production. The live Api value wins when present; the
// directly-set tasks field is the unit-test override.
func (m *mining) taskManager() *game.TaskManager {
	if m.api != nil {
		if tm := m.api.GetTaskManager(); tm != nil {
			return tm
		}
	}
	return m.tasks
}

// objectManager resolves the ObjectManagerAPI lazily, same
// contract as taskManager.
func (m *mining) objectManager() plugins.ObjectManagerAPI {
	if m.api != nil {
		if om := m.api.GetObjectManager(); om != nil {
			return om
		}
	}
	return m.objects
}

// ensureTask schedules the 1-tick global MiningTask once, on
// the lazily-resolved TaskManager. Idempotent. A nil
// TaskManager schedules nothing and retries on the next call.
// Callers run on the tick goroutine (startMining, through the
// turn-packet drain), which is the only goroutine the
// TaskManager allows Schedule from.
func (m *mining) ensureTask() {
	if m.taskScheduled {
		return
	}
	tm := m.taskManager()
	if tm == nil {
		return
	}
	task := game.NewTask(1, func(cycle int64) bool {
		m.tick++
		m.processMiningTick(m.tick)
		m.tickWater()
		m.tickAncient()
		return false
	})
	task.Name = "MiningTask"
	tm.Schedule(task)
	m.taskScheduled = true
}

// emitEvent emits a custom event through the captured emit func;
// a nil emit (before Register) drops the emit, which is the
// no-hooks default for every query.
func (m *mining) emitEvent(name string, payload any) {
	if m == nil || m.emit == nil {
		return
	}
	m.emit(name, payload)
}

// ---- small helpers ----

// miningLevel reads the player's visible Mining level
// (getMiningLevel, plugin.js:408-410).
func miningLevel(player *game.Player) int {
	if player == nil {
		return 0
	}
	return player.Skills().CurrentLevel(game.SkillMining)
}

// rollMiningLevel is the level the success rolls use: the
// visible level capped at 99 plus invisible boosts other
// plugins add ("mining:level-boost"), which do count past 99
// (getRollMiningLevel, plugin.js:417-423).
func (m *mining) rollMiningLevel(player *game.Player) int {
	boost := 0
	if m != nil {
		req := &levelBoostRequest{Player: player}
		m.emitEvent("mining:level-boost", req)
		boost = req.Boost
	}
	if boost < 0 {
		boost = 0
	}
	level := miningLevel(player)
	if level > MaxVisibleLevel {
		level = MaxVisibleLevel
	}
	return level + boost
}

// levelBoostRequest is the "mining:level-boost" payload.
type levelBoostRequest struct {
	Player *game.Player
	Boost  int
}

// successNumerator is the Wiki skilling success numerator out of
// 256 for a chart's [low, high] at this level
// (successNumerator, plugin.js:426-429).
func successNumerator(chart [2]int, level int) int {
	low, high := chart[0], chart[1]
	numerator := 1 + int(math.Floor(float64(low*(99-level))/98+float64(high*(level-1))/98+0.5))
	if numerator < 0 {
		return 0
	}
	if numerator > SuccessRollSides {
		return SuccessRollSides
	}
	return numerator
}

// rollSucceeds rolls one success check (rollSucceeds,
// plugin.js:431-433).
func (m *mining) rollSucceeds(chart [2]int, level int) bool {
	return int(m.randFloat()*SuccessRollSides) < successNumerator(chart, level)
}

// rollInterval is the ticks to the next roll: the pickaxe's
// interval, one less on a dragon/infernal/crystal speed-up
// (members only) (rollInterval, plugin.js:436-440).
func (m *mining) rollInterval(pickaxe Pickaxe) int {
	interval := pickaxe.AttemptIntervalTicks
	if pickaxe.HasFastRoll && m.membersWorld && m.randFloat() < pickaxe.FastRollChance {
		interval--
	}
	if interval < 1 {
		interval = 1
	}
	return interval
}

// chargedGloryPattern matches a charged glory / eternal glory
// amulet name (plugin.js:65).
var chargedGloryPattern = regexp.MustCompile(`^Amulet of (glory ?\(t?[1-6]\)|eternal glory)$`)

// wearsChargedGlory reports whether the player wears a charged
// glory (wearsChargedGlory, plugin.js:442-447).
func (m *mining) wearsChargedGlory(player *game.Player) bool {
	if player == nil || m.tables == nil {
		return false
	}
	amulet := player.Equipment().ItemAt(game.AmuletSlot)
	if !amulet.IsValid() {
		return false
	}
	return chargedGloryPattern.MatchString(m.tables.ItemName(amulet.ID()))
}

// rollGemTable runs the gem pre-roll (rollGemTable,
// plugin.js:452-460): 1/256, and 3/256 more with a charged
// glory. A hit returns the gem item id, or 0 for the "nothing"
// row; a miss returns -1.
func (m *mining) rollGemTable(player *game.Player) int {
	if m.tables == nil {
		return -1
	}
	hit := func(faces int) bool {
		return int(m.randFloat()*SuccessRollSides) >= SuccessRollSides-faces
	}
	if !hit(GemRollFaces) && !(m.wearsChargedGlory(player) && hit(GloryGemRollFaces)) {
		return -1
	}
	total := 0
	for _, row := range m.tables.GemTable {
		total += row.Weight
	}
	if total <= 0 {
		return -1
	}
	roll := m.randFloat() * float64(total)
	for _, row := range m.tables.GemTable {
		roll -= float64(row.Weight)
		if roll < 0 {
			return row.ItemID
		}
	}
	return m.tables.GemTable[0].ItemID
}

// gemMessage is "You just found a Sapphire!" / "an Emerald!"
// (gemMessage, plugin.js:463-467).
func (m *mining) gemMessage(itemID int) string {
	name := ""
	if m.tables != nil {
		name = m.tables.ItemName(itemID)
	}
	name = strings.TrimPrefix(name, "Uncut ")
	if name == "" {
		name = "Gem"
	}
	name = strings.ToUpper(name[:1]) + name[1:]
	article := "a"
	if strings.ContainsRune("AEIOU", rune(name[0])) {
		article = "an"
	}
	return "You just found " + article + " " + name + "!"
}

// rollCascade runs a cascading chart (rollCascade,
// plugin.js:471-473): the first size whose roll succeeds, else
// nil.
func (m *mining) rollCascade(rock *Rock, level int) *CascadeStep {
	for i := range rock.Cascade {
		step := &rock.Cascade[i]
		if m.rollSucceeds([2]int{step.Low, step.High}, level) {
			return step
		}
	}
	return nil
}

// breathingFilterPattern matches a gas mask, facemask or slayer
// helmet (plugin.js:655).
var breathingFilterPattern = regexp.MustCompile(`(?i)gas mask|facemask|slayer helmet`)

// wearsBreathingFilter reports whether the worn head slot is a
// breathing filter (wearsBreathingFilter, plugin.js:658-662).
func (m *mining) wearsBreathingFilter(player *game.Player) bool {
	if player == nil || m.tables == nil {
		return false
	}
	head := player.Equipment().ItemAt(game.HeadSlot)
	if !head.IsValid() {
		return false
	}
	return breathingFilterPattern.MatchString(m.tables.ItemName(head.ID()))
}

// forMiner is the essence mine's outcrop for this player: rune
// essence on a free world or below 30 Mining, else pure essence
// (forMiner, plugin.js:661-665).
func (m *mining) forMiner(player *game.Player, rock *Rock) *Rock {
	if rock == nil || !rock.RuneEssence {
		return rock
	}
	if m.membersWorld && miningLevel(player) >= PureEssenceLevel {
		return rock
	}
	copy := *rock
	copy.OreID = 1436 // ItemIds.RUNE_ESSENCE
	return &copy
}

// findBestPickaxe returns the best pickaxe the player can use
// (findBestPickaxe, plugin.js:486-506): wielded or carried, best
// rank first, members items skipped on a free world, then the
// "mining:tool" answerer.
func (m *mining) findBestPickaxe(player *game.Player) *Pickaxe {
	if player == nil || m.tables == nil {
		return nil
	}
	level := miningLevel(player)
	agility := player.Skills().CurrentLevel(game.SkillAgility)
	weapon := player.Equipment().ItemAt(game.WeaponSlot)
	weaponID := -1
	if weapon.IsValid() {
		weaponID = weapon.ID()
	}
	for i := range m.tables.PickaxesDesc {
		pickaxe := &m.tables.PickaxesDesc[i]
		if level < pickaxe.RequiredLevel {
			continue
		}
		requiredAgility := pickaxe.RequiredAgility
		if requiredAgility == 0 {
			requiredAgility = 1
		}
		if agility < requiredAgility {
			continue
		}
		if weaponID == pickaxe.ID || player.Inventory().Contains(pickaxe.ID) {
			if !m.membersWorld && m.isMembersItem != nil && m.isMembersItem(pickaxe.ID) {
				continue
			}
			return pickaxe
		}
	}
	req := &toolRequest{Player: player, Pickaxes: m.tables.PickaxesDesc}
	m.emitEvent("mining:tool", req)
	return req.Pickaxe
}

// toolRequest is the "mining:tool" payload.
type toolRequest struct {
	Player   *game.Player
	Pickaxes []Pickaxe
	Pickaxe  *Pickaxe
}

// pickaxeRequest is the "mining:pickaxe" payload.
type pickaxeRequest struct {
	ItemID  int
	Pickaxe *Pickaxe
}

// bestPickaxeRequest is the "mining:best-pickaxe" payload.
type bestPickaxeRequest struct {
	Player  *game.Player
	Pickaxe *Pickaxe
}

// rollIntervalRequest is the "mining:roll-interval" payload.
type rollIntervalRequest struct {
	Pickaxe *Pickaxe
	Ticks   *int
}

// gemRollRequest is the "mining:gem-roll" payload.
type gemRollRequest struct {
	Player *game.Player
	Gem    bool
}

// answerBestPickaxe answers "mining:best-pickaxe"
// (plugin.js:476-480).
func (m *mining) answerBestPickaxe(payload any) {
	req, ok := payload.(*bestPickaxeRequest)
	if !ok || req == nil || req.Player == nil || req.Pickaxe != nil {
		return
	}
	req.Pickaxe = m.findBestPickaxe(req.Player)
}

// answerPickaxe answers "mining:pickaxe" (plugin.js:483-487):
// the pickaxe `itemId` is (Mining's own list), or nil; no level
// check, only "is it a pickaxe".
func (m *mining) answerPickaxe(payload any) {
	req, ok := payload.(*pickaxeRequest)
	if !ok || req == nil || req.Pickaxe != nil {
		return
	}
	if m.tables == nil {
		return
	}
	for i := range m.tables.Pickaxes {
		if m.tables.Pickaxes[i].ID == req.ItemID {
			req.Pickaxe = &m.tables.Pickaxes[i]
			return
		}
	}
}

// answerRollInterval answers "mining:roll-interval"
// (plugin.js:490-494).
func (m *mining) answerRollInterval(payload any) {
	req, ok := payload.(*rollIntervalRequest)
	if !ok || req == nil || req.Pickaxe == nil || req.Ticks != nil {
		return
	}
	ticks := m.rollInterval(*req.Pickaxe)
	req.Ticks = &ticks
}

// answerGemRoll answers "mining:gem-roll" (plugin.js:791-798).
func (m *mining) answerGemRoll(payload any) {
	req, ok := payload.(*gemRollRequest)
	if !ok || req == nil || req.Player == nil {
		return
	}
	gem := m.rollGemTable(req.Player)
	if gem < 0 {
		return
	}
	m.giveGem(req.Player, gem)
	req.Gem = true
}

// ---- messaging ----

// spam sends a spam-channel line (spam, plugin.js:704-706).
func spam(player *game.Player, text string) {
	if player == nil {
		return
	}
	player.SendTypedMessage(protocol.ChatTypeSpam, text, "")
}

// messageBox shows a failure as a statement dialogue
// (messageBox, plugin.js:637-641; the cooking/woodcutting
// precedent).
func messageBox(player *game.Player, text string) {
	if player == nil {
		return
	}
	if dm := player.GetDialogueManager(); dm != nil {
		dm.StartDialogues(game.NewDialogueChainBuilder().Add(game.NewStatementDialogue(0, text)))
	}
}

// ---- inventory ----

// inventoryBlocks reports whether a full inventory stops mining
// (inventoryBlocks, plugin.js:972-985): a full inventory stops
// unless the ore stacks onto what the player holds or an open
// container ("storage:room") has room for it.
func (m *mining) inventoryBlocks(player *game.Player, oreID int) bool {
	if player == nil {
		return true
	}
	inv := player.Inventory()
	if inv == nil || !inv.IsFull() {
		return false
	}
	// A full inventory still takes an ore that stacks onto what
	// the player holds (ItemDefinition.forId(oreId)?.isStackable()
	// in TS): GetAddableAmount is > 0 exactly then.
	if inv.GetAddableAmount(game.NewItem(oreID, 1)) > 0 {
		return false
	}
	req := &storageRoomRequest{Player: player, ItemID: oreID, Skill: game.SkillMining}
	m.emitEvent("storage:room", req)
	return !req.Room
}

// storageRoomRequest is the "storage:room" payload.
type storageRoomRequest struct {
	Player *game.Player
	ItemID int
	Skill  int
	Room   bool
}

// isStackable reports whether an item stacks, from the cache
// item definition (ItemDefinition.forId(oreId)?.isStackable()).
// With no tables it reads false (fail-closed: a full inventory
// blocks).
func (m *mining) isStackable(oreID int) bool {
	if m.tables == nil {
		return false
	}
	// The tables carry names only; stackability comes from the
	// item definition source installed at boot. The mining
	// tables resolve stackable ids from the known stackable ore
	// ids (rubium splinters stack; ores do not).
	return oreID == 31722 // RUBIUM_SPLINTERS
}

// inventoryFull sends the full-inventory message box
// (inventoryFull, plugin.js:643-647).
func (m *mining) inventoryFull(player *game.Player, oreID int, rock *Rock) {
	name := ""
	if rock != nil && rock.FullName != "" {
		name = rock.FullName
	} else if m.tables != nil {
		name = strings.ToLower(m.tables.ItemName(oreID))
	}
	if name == "" {
		name = "ore"
	}
	messageBox(player, "Your inventory is too full to hold any more "+name+".")
}

// oreMessage is "You manage to mine some iron." — the first word
// of the rock's name (oreMessage, plugin.js:649-651).
func oreMessage(rock *Rock) string {
	first := rock.Name
	if i := strings.IndexByte(first, ' '); i >= 0 {
		first = first[:i]
	}
	return "You manage to mine some " + strings.ToLower(first) + "."
}

// successLine is the line for an ore, with " and give it to
// <taker>" when someone takes it off the player (successLine,
// plugin.js:736-739).
func successLine(rock *Rock, outcome *Outcome, taker string) string {
	line := rock.OreMessage
	if outcome != nil && outcome.Message != "" {
		line = outcome.Message
	} else if line == "" {
		line = oreMessage(rock)
	}
	if taker != "" {
		return strings.TrimSuffix(line, ".") + ", and give it to " + taker + "."
	}
	return line
}

// ---- XP / counting ----

// addMiningXP grants Mining XP with the prospector multiplier
// (plugin.js:935): fractional XP truncates at the grant site
// (int(xp)) — the documented integer-truncation gap.
func (m *mining) addMiningXP(player *game.Player, xp float64) {
	if player == nil || xp <= 0 {
		return
	}
	player.Skills().AddExperiences(game.SkillMining, int(xp*m.prospectorMultiplier(player)))
}

// countOre counts kept ore in tracking_ore_mined (countOre,
// plugin.js:1053-1060): the attribute plus the varp send. This
// plugin is the counter's only writer.
func (m *mining) countOre(player *game.Player, amount int) {
	if player == nil || amount <= 0 {
		return
	}
	mined := 0
	if v, ok := player.GetAttribute(OreMinedAttribute).(int); ok {
		mined = v
	} else if v, ok := player.GetAttribute(OreMinedAttribute).(float64); ok {
		mined = int(v)
	}
	mined += amount
	player.SetAttribute(OreMinedAttribute, mined)
	if m.tables != nil && m.tables.OresMinedVarpID > 0 {
		player.SendVarp(m.tables.OresMinedVarpID, mined)
	}
}

// countKeptOre answers "mining:count-ore" (plugin.js:1063-1065).
func (m *mining) countKeptOre(payload any) {
	req, ok := payload.(*countOreRequest)
	if !ok || req == nil || req.Amount <= 0 {
		return
	}
	m.countOre(req.Player, req.Amount)
}

// countOreRequest is the "mining:count-ore" payload.
type countOreRequest struct {
	Player *game.Player
	Amount int
}

// answerOresMined answers "mining:ores-mined"
// (plugin.js:1068-1070).
func (m *mining) answerOresMined(payload any) {
	req, ok := payload.(*oresMinedRequest)
	if !ok || req == nil || req.Player == nil {
		return
	}
	if v, ok := req.Player.GetAttribute(OreMinedAttribute).(int); ok {
		req.Count = v
	} else if v, ok := req.Player.GetAttribute(OreMinedAttribute).(float64); ok {
		req.Count = int(v)
	}
}

// oresMinedRequest is the "mining:ores-mined" payload.
type oresMinedRequest struct {
	Player *game.Player
	Count  int
}

// sendOresMined sends the counter on login (sendOresMined,
// plugin.js:1073-1078).
func (m *mining) sendOresMined(player *game.Player) {
	if player == nil || m.tables == nil || m.tables.OresMinedVarpID <= 0 {
		return
	}
	mined := 0
	if v, ok := player.GetAttribute(OreMinedAttribute).(int); ok {
		mined = v
	} else if v, ok := player.GetAttribute(OreMinedAttribute).(float64); ok {
		mined = int(v)
	}
	if mined > 0 {
		player.SendVarp(m.tables.OresMinedVarpID, mined)
	}
}

// ---- start / stop ----

// startMining begins a mining session (startMining,
// plugin.js:667-701). False when the rock cannot be mined:
// no breathing filter, no pickaxe, too low a level, or a full
// inventory. The skilling timer carries over between rocks: a
// roll waits for the last one's interval.
func (m *mining) startMining(player *game.Player, rockObject plugins.GameObject, rock *Rock) bool {
	if player == nil || rockObject == nil || rock == nil {
		return false
	}
	rock = m.forMiner(player, rock)
	if rock.RequiresBreathingFilter && !m.wearsBreathingFilter(player) {
		player.SendMessage(NoBreathingFilter)
		return false
	}
	pickaxe := m.findBestPickaxe(player)
	if pickaxe == nil {
		messageBox(player, NoPickaxeMessage)
		return false
	}
	if miningLevel(player) < rock.Level {
		messageBox(player, "You need a Mining level of "+itoa(rock.Level)+" to mine this rock.")
		return false
	}
	if m.inventoryBlocks(player, rock.OreID) {
		m.inventoryFull(player, rock.OreID, rock)
		return false
	}
	m.stopMining(player, false)
	nextRoll := m.tick + FirstRollDelayTicks
	if ready, ok := m.rollReady[player]; ok && ready > nextRoll {
		nextRoll = ready
	}
	m.sessions[player] = &session{
		rock:         rock,
		pickaxe:      *pickaxe,
		objectID:     rockObject.ID(),
		location:     rockObject.Location(),
		privateArea:  objectPrivateArea(rockObject),
		swingTick:    m.tick + 1,
		hasSwingTick: true,
		nextRollTick: nextRoll,
	}
	m.ensureTask()
	return true
}

// stopMining ends a player's mining session (stopMining,
// plugin.js:509-518), resetting the animation unless told not
// to. A player with no session is a no-op.
func (m *mining) stopMining(player *game.Player, resetAnim bool) {
	if player == nil {
		return
	}
	if _, ok := m.sessions[player]; !ok {
		return
	}
	delete(m.sessions, player)
	if resetAnim {
		player.PerformAnimation(game.ResetAnimationID, 0)
	}
}

// swing plays the first swing the tick after the click (swing,
// plugin.js:710-714).
func (m *mining) swing(player *game.Player, state *session) {
	state.hasSwingTick = false
	spam(player, SwingMessage)
	player.PerformAnimation(m.tables.SwingSeq(state.pickaxe, state.rock), 0)
}

// rockTimerRanOut counts a depletion-timer rock down on each
// tick someone mines it (rockTimerRanOut, plugin.js:718-729).
func (m *mining) rockTimerRanOut(rockObject plugins.GameObject, rock *Rock) bool {
	if rock == nil || rock.DepletionTicks <= 0 {
		return false
	}
	timer, ok := m.rockTimers[rockObject]
	if !ok {
		timer = &rockTimerState{left: rock.DepletionTicks, tick: -1}
		m.rockTimers[rockObject] = timer
	}
	if timer.tick != m.tick {
		timer.tick = m.tick
		timer.left--
	}
	return timer.left <= 0
}

// rollOutcome picks one of a rock's weighted outcomes
// (rollOutcome, plugin.js:743-749).
func (m *mining) rollOutcome(rock *Rock) *Outcome {
	total := 0
	for i := range rock.Outcomes {
		total += rock.Outcomes[i].Weight
	}
	if total <= 0 {
		return nil
	}
	roll := m.randFloat() * float64(total)
	for i := range rock.Outcomes {
		roll -= float64(rock.Outcomes[i].Weight)
		if roll < 0 {
			return &rock.Outcomes[i]
		}
	}
	return &rock.Outcomes[len(rock.Outcomes)-1]
}

// rockChart is the chart a rock's ore roll uses: a charged glory
// has gem rocks roll their own, better one (rockChart,
// plugin.js:752-754).
func (m *mining) rockChart(player *game.Player, rock *Rock) *[2]int {
	if rock.GloryChance != nil && m.wearsChargedGlory(player) {
		return rock.GloryChance
	}
	return rock.Chance
}

// attemptResult is one roll's result (rollAttempt,
// plugin.js:769-783): a gem-table hit, the ore outcome, or nil
// for a miss.
type attemptResult struct {
	gem     int // -1 = no gem, >= 0 = gem item id (0 = nothing row)
	hasGem  bool
	outcome *Outcome
}

// rollAttempt runs one roll (rollAttempt, plugin.js:769-783).
func (m *mining) rollAttempt(player *game.Player, rock *Rock) *attemptResult {
	if rock.Gems {
		if gem := m.rollGemTable(player); gem >= 0 {
			return &attemptResult{gem: gem, hasGem: true}
		}
	}
	level := m.rollMiningLevel(player)
	if rock.Cascade != nil {
		if step := m.rollCascade(rock, level); step != nil {
			return &attemptResult{outcome: &Outcome{ItemID: step.ItemID, Min: 1, Max: 1, Weight: 1, XP: step.XP, Message: step.Message}}
		}
		return nil
	}
	if chart := m.rockChart(player, rock); chart != nil && !m.rollSucceeds(*chart, level) {
		return nil
	}
	var outcome *Outcome
	if rock.Outcomes != nil {
		outcome = m.rollOutcome(rock)
	}
	return &attemptResult{outcome: outcome}
}

// rollChance is the chance (0-1) that an ore roll on this rock
// succeeds, for "mining:attempt": 1 with no chart, -1 for a
// cascade (rollChance, plugin.js:757-762).
func (m *mining) rollChance(player *game.Player, rock *Rock) float64 {
	if rock.Cascade != nil {
		return -1
	}
	chart := m.rockChart(player, rock)
	if chart == nil {
		return 1
	}
	return float64(successNumerator(*chart, m.rollMiningLevel(player))) / SuccessRollSides
}

// giveGem delivers a gem-table hit (giveGem, plugin.js:786-799):
// the gem (or nothing), no XP, and the rock is left as it was.
// With no room anywhere the gem falls to the floor rather than
// vanishing.
func (m *mining) giveGem(player *game.Player, itemID int) {
	if itemID <= 0 {
		return
	}
	m.emitEvent("mining:gem", &gemEvent{Player: player, ItemID: itemID})
	player.SendMessage(m.gemMessage(itemID))
	m.deliverOre(player, m.prepareOre(player, itemID, 1, 0))
}

// gemEvent is the "mining:gem" payload.
type gemEvent struct {
	Player *game.Player
	ItemID int
}

// outcomeAmount rolls one outcome's amount (outcomeAmount,
// plugin.js:801-803).
func (m *mining) outcomeAmount(outcome *Outcome) int {
	if outcome == nil || outcome.Max <= outcome.Min {
		if outcome == nil {
			return 1
		}
		return outcome.Min
	}
	return m.randInt(outcome.Min, outcome.Max)
}

// oreAmount is one ore a success, or two from a rock's
// doubleFromLevel (oreAmount, plugin.js:806-810).
func (m *mining) oreAmount(player *game.Player, rock *Rock) int {
	if rock.EssenceByLevel {
		return m.ancientEssenceAmount(miningLevel(player))
	}
	if rock.DoubleFromLevel > 0 && miningLevel(player) >= rock.DoubleFromLevel {
		return 2
	}
	return 1
}

// ---- the tick ----

// processMiningTick runs one tick of the global MiningTask
// (MiningTask.execute, plugin.js:813-899).
func (m *mining) processMiningTick(cycle int64) {
	for player, state := range m.sessions {
		if player == nil || !player.IsRegistered() || player.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
			delete(m.sessions, player)
			continue
		}
		// Global tasks precede player turns; the last step may
		// have drained the route.
		movement := player.MovementQueue()
		if (movement != nil && (movement.Size() > 0 || movement.IsMoving())) || player.ForceMovement() != nil {
			m.stopMining(player, true)
			continue
		}
		if state.levelUpPaused {
			continue
		}
		var rockObject plugins.GameObject
		if state.npcRock != nil {
			rockObject = m.npcRockTarget(state.npcRock)
		} else if om := m.objectManager(); om != nil {
			rockObject = om.Get(state.objectID, state.location)
		}
		if rockObject == nil {
			m.stopMining(player, true)
			continue
		}
		if !withinInteractionDistance(rockObject, player.Location()) {
			m.stopMining(player, true)
			continue
		}
		pickaxe := m.findBestPickaxe(player)
		if pickaxe == nil || miningLevel(player) < state.rock.Level ||
			(state.rock.RequiresBreathingFilter && !m.wearsBreathingFilter(player)) {
			m.stopMining(player, true)
			continue
		}
		state.pickaxe = *pickaxe
		if m.inventoryBlocks(player, state.rock.OreID) {
			m.inventoryFull(player, state.rock.OreID, state.rock)
			m.stopMining(player, true)
			continue
		}
		if state.hasSwingTick && cycle >= state.swingTick {
			m.swing(player, state)
		}
		// A rock that `depletesOnOre` waits for the next ore
		// once its countdown has ended.
		timerRanOut := m.rockTimerRanOut(rockObject, state.rock)
		if timerRanOut && !state.rock.DepletesOnOre {
			delete(m.rockTimers, rockObject)
			if m.depleteRock(player, rockObject, state.rock, false) {
				m.stopMining(player, true)
				continue
			}
		}
		if cycle < state.nextRollTick {
			continue
		}
		// Content may mine faster ("skills:action-interval").
		state.nextRollTick = cycle + int64(skillactions.ActionInterval(m.emitEvent, player, game.SkillMining, "mine", m.rollInterval(state.pickaxe), 0))
		m.rollReady[player] = state.nextRollTick
		player.PerformAnimation(m.tables.SwingSeq(state.pickaxe, state.rock), 0)

		result := m.rollAttempt(player, state.rock)
		// Content may turn a miss into a hit ("mining:attempt");
		// a gem roll is never a miss.
		attempt := &attemptEvent{
			Player: player, Object: rockObject, Rock: state.rock.Name,
			OreID: state.rock.OreID, Success: result != nil,
			Chance: m.rollChance(player, state.rock),
		}
		m.emitEvent("mining:attempt", attempt)
		landed := result
		if landed == nil && attempt.Success {
			var outcome *Outcome
			if state.rock.Outcomes != nil {
				outcome = m.rollOutcome(state.rock)
			}
			landed = &attemptResult{outcome: outcome}
		}
		if landed == nil {
			continue
		}
		if landed.hasGem {
			m.giveGem(player, landed.gem)
			continue
		}
		m.handleSuccess(player, state, rockObject, landed.outcome, timerRanOut)
	}
}

// attemptEvent is the "mining:attempt" payload.
type attemptEvent struct {
	Player  *game.Player
	Object  plugins.GameObject
	Rock    string
	OreID   int
	Success bool
	Chance  float64
}

// handleSuccess delivers one successful roll (the MiningTask
// success path, plugin.js:900-968).
func (m *mining) handleSuccess(player *game.Player, state *session, rockObject plugins.GameObject, outcome *Outcome, timerRanOut bool) {
	oreID := state.rock.OreID
	if outcome != nil {
		oreID = outcome.ItemID
	}
	amount := 1
	if outcome != nil && outcome.Min != 0 {
		amount = m.outcomeAmount(outcome)
	} else {
		amount = m.oreAmount(player, state.rock)
	}
	xp := state.rock.XP
	if outcome != nil {
		xp = outcome.XP
	}
	// Content may change what a success yields
	// ("mining:ore-yield"). `spend` runs once the ore is really
	// kept, so a charge is not used on an ore that content
	// takes off the player or a pickaxe burns.
	oreYield := &oreYieldEvent{Player: player, Rock: state.rock.Name, OreID: oreID, Amount: amount}
	m.emitEvent("mining:ore-yield", oreYield)
	oreID, amount = oreYield.OreID, oreYield.Amount
	// Content may take the ore off the player as it is mined
	// ("mining:hand-over").
	handOver := &handOverEvent{Player: player, Object: rockObject, ItemID: oreID, Amount: amount}
	m.emitEvent("mining:hand-over", handOver)
	var pendingOre *game.Item
	kept := false
	if handOver.Taker == "" {
		if m.tryCombustOre(player, state.pickaxe.ID, oreID) {
			player.PerformGraphic(SmokepuffGraphic, SmokepuffHeight, 0)
			game.SendSoundEffect(player, SmeltSound, 1, 0, 1)
		} else {
			pendingOre = m.prepareOre(player, oreID, amount, xp)
			if state.rock.CountEach {
				m.countOre(player, amount)
			} else {
				m.countOre(player, 1)
			}
			kept = true
			if oreYield.Spend != nil {
				oreYield.Spend()
			}
		}
		sound := MiningSuccessSound
		if state.rock.HasSound {
			sound = state.rock.Sound
		}
		game.SendSoundEffect(player, sound, 1, 0, 1)
	}
	if !state.rock.Silent {
		line := successLine(state.rock, outcome, handOver.Taker)
		if state.rock.GameMessage {
			player.SendMessage(line)
		} else {
			spam(player, line)
		}
	}
	// Only a charged crystal pickaxe mining spends a charge.
	if state.pickaxe.ID == 23680 { // ItemIds.CRYSTAL_PICKAXE
		m.tryUseCrystalCharge(player)
	}
	m.addMiningXP(player, xp)
	// OSRS gives a winning pet the last backpack slot before
	// the resource.
	petRoll := &petRollEvent{Player: player, Skill: game.SkillMining, PetBase: state.rock.PetBase}
	m.emitEvent("pets:skilling-roll", petRoll)
	m.deliverOre(player, pendingOre)
	if kept && state.rock.Outcomes == nil {
		m.mineExtraOre(player, state.rock, outcome, oreID)
	}
	m.emitEvent("mining:success", &successEvent{
		Player: player, Skill: game.SkillMining, PetBase: state.rock.PetBase,
		ItemID: oreID, PetRolled: petRoll.Handled, PickaxeID: state.pickaxe.ID,
		ObjectID: state.objectID, Location: state.location,
	})
	if state.npcRock != nil {
		m.stopMining(player, true)
		m.emitEvent("mining:npc-rock-mined", &npcRockMinedEvent{Player: player, NPC: state.npcRock.npc})
		return
	}
	if state.rock.DepletesOnOre && timerRanOut {
		delete(m.rockTimers, rockObject)
		if m.depleteRock(player, rockObject, state.rock, false) {
			m.stopMining(player, true)
			return
		}
	}
	if state.rock.Infinite || state.rock.DepletionTicks > 0 {
		if m.inventoryBlocks(player, state.rock.OreID) {
			m.inventoryFull(player, state.rock.OreID, state.rock)
			m.stopMining(player, true)
			return
		}
		return
	}
	// A staged rock steps down and the player mines on; a rock
	// with a `yield` may hold more ore.
	if next := m.nextStageID(rockObject, state.rock); next >= 0 {
		replacement := m.replaceRock(rockObject, next)
		if replacement != nil {
			state.objectID = replacement.ID()
		}
		return
	}
	if m.rockHolds(rockObject, state.rock) {
		return
	}
	// A saved finite-yield rock has already spent its normal
	// yield; do not draw a fresh yield until the rock really
	// depletes and respawns.
	if saved, ok := m.gloveSaves[rockObject]; ok && saved.baseExhausted {
		if m.depleteRock(player, rockObject, state.rock, false) {
			m.stopMining(player, true)
		}
		return
	}
	heldByGloves := m.gloveHolds(player, rockObject, state.rock)
	if m.depleteRock(player, rockObject, state.rock, heldByGloves) {
		m.stopMining(player, true)
	}
}

// oreYieldEvent is the "mining:ore-yield" payload.
type oreYieldEvent struct {
	Player *game.Player
	Rock   string
	OreID  int
	Amount int
	Spend  func()
}

// handOverEvent is the "mining:hand-over" payload.
type handOverEvent struct {
	Player *game.Player
	Object plugins.GameObject
	ItemID int
	Amount int
	Taker  string
}

// petRollEvent is the "pets:skilling-roll" payload.
type petRollEvent struct {
	Player  *game.Player
	Skill   int
	PetBase int
	Handled bool
}

// successEvent is the "mining:success" payload
// (plugin.js:934): everything that emits it with a petBase
// (rocks, crashed stars) rolls clue geodes.
type successEvent struct {
	Player    *game.Player
	Skill     int
	PetBase   int
	ItemID    int
	PetRolled bool
	PickaxeID int
	ObjectID  int
	Location  game.Location
}

// npcRockMinedEvent is the "mining:npc-rock-mined" payload.
type npcRockMinedEvent struct {
	Player *game.Player
	NPC    *game.NPC
}

// ---- ore delivery ----

// giveOre gives ore to a player (giveOre, plugin.js:1001-1014):
// an open container takes it first ("storage:gather"); the rest
// goes to the inventory. Returns the amount kept.
func (m *mining) giveOre(player *game.Player, oreID, amount int, xp float64) int {
	if player == nil || amount <= 0 {
		return 0
	}
	req := &gatherRequest{Player: player, ItemID: oreID, Amount: amount, Skill: game.SkillMining, XP: xp}
	m.emitEvent("storage:gather", req)
	stored := req.Stored
	if stored < 0 {
		stored = 0
	}
	if stored > amount {
		stored = amount
	}
	remaining := amount - stored
	if remaining <= 0 {
		return stored
	}
	item := game.NewItem(oreID, remaining)
	accepted := player.Inventory().GetAddableAmount(item)
	if accepted > remaining {
		accepted = remaining
	}
	if accepted > 0 {
		clone := item
		clone.SetAmount(accepted)
		player.Inventory().Add(clone, true)
	}
	return stored + accepted
}

// gatherRequest is the "storage:gather" payload.
type gatherRequest struct {
	Player *game.Player
	ItemID int
	Amount int
	Stored int
	Skill  int
	XP     float64
}

// prepareOre resolves storage first; a winning pet can take
// room reserved for the remaining ore (prepareOre,
// plugin.js:1017-1021). Returns the inventory item, or nil when
// storage took it all.
func (m *mining) prepareOre(player *game.Player, oreID, amount int, xp float64) *game.Item {
	if player == nil || amount <= 0 {
		return nil
	}
	req := &gatherRequest{Player: player, ItemID: oreID, Amount: amount, Skill: game.SkillMining, XP: xp}
	m.emitEvent("storage:gather", req)
	if amount > req.Stored {
		item := game.NewItem(oreID, amount-req.Stored)
		return &item
	}
	return nil
}

// deliverOre adds an ore to the inventory, dropping the
// overflow on the ground (deliverOre, plugin.js:1023-1029).
func (m *mining) deliverOre(player *game.Player, ore *game.Item) {
	if player == nil || ore == nil {
		return
	}
	amount := ore.Amount()
	accepted := player.Inventory().GetAddableAmount(*ore)
	if accepted > amount {
		accepted = amount
	}
	if accepted > 0 {
		clone := *ore
		clone.SetAmount(accepted)
		player.Inventory().Add(clone, true)
	}
	if accepted < amount {
		rest := *ore
		rest.SetAmount(amount - accepted)
		if game.Hooks.DropGroundItem != nil {
			game.Hooks.DropGroundItem(rest, player.Location(), player)
		}
	}
}

// mineExtraOre grants enhancer bonus ores ("mining:extra-ore",
// plugin.js:1032-1050). Without room for it, neither ore nor XP
// is given.
func (m *mining) mineExtraOre(player *game.Player, rock *Rock, outcome *Outcome, minedOreID int) {
	if rock == nil || rock.Infinite {
		return
	}
	rockOreID := rock.OreID
	if outcome != nil {
		rockOreID = outcome.ItemID
	}
	oreID := minedOreID
	if oreID == 0 {
		oreID = rockOreID
	}
	xp := rock.XP
	if outcome != nil {
		xp = outcome.XP
	}
	req := &extraOreRequest{Player: player, OreID: rockOreID, RockLevel: rock.Level}
	m.emitEvent("mining:extra-ore", req)
	for _, bonus := range req.Bonuses {
		if m.inventoryBlocks(player, oreID) || m.giveOre(player, oreID, 1, xp) != 1 {
			break
		}
		if bonus.Message != "" {
			spam(player, bonus.Message)
		}
		m.countOre(player, 1)
		m.addMiningXP(player, xp)
	}
}

// extraOreRequest is the "mining:extra-ore" payload.
type extraOreRequest struct {
	Player    *game.Player
	OreID     int
	RockLevel int
	Bonuses   []ExtraOreBonus
}

// ExtraOreBonus is one "mining:extra-ore" bonus.
type ExtraOreBonus struct {
	Message string
}

// ---- yield / gloves ----

// rockHolds reports whether a rock with a `yield` still stands
// after this ore (rockHolds, plugin.js:352-368).
func (m *mining) rockHolds(rockObject plugins.GameObject, rock *Rock) bool {
	if rock == nil || rock.Yield == nil {
		return false
	}
	taken, ok := m.rockYields[rockObject]
	if !ok {
		total := 0
		if rock.Yield.Max > 0 {
			total = m.randInt(rock.Yield.Min, rock.Yield.Max)
		}
		taken = &yieldState{mined: 0, total: total}
	}
	taken.mined++
	var holds bool
	if rock.Yield.Chance == 0 {
		holds = taken.mined < taken.total
	} else {
		holds = !(taken.mined >= rock.Yield.Min && m.randFloat() < rock.Yield.Chance)
	}
	if holds {
		m.rockYields[rockObject] = taken
	} else {
		delete(m.rockYields, rockObject)
	}
	return holds
}

// gloveSaveCount is the extra successful gathers the gloves
// actually worn at this rock allow (gloveSaves,
// plugin.js:371-384).
func (m *mining) gloveSaveCount(player *game.Player, rock *Rock) int {
	if player == nil || !m.membersWorld {
		return 0
	}
	hands := player.Equipment().ItemAt(game.HandsSlot)
	tier := 0
	switch hands.ID() {
	case 21343: // ItemIds.MINING_GLOVES
		tier = 1
	case 21345: // ItemIds.SUPERIOR_MINING_GLOVES
		tier = 2
	case 21392: // ItemIds.EXPERT_MINING_GLOVES
		tier = 3
	}
	return m.gloveSavesForTier(tier, rock)
}

// gloveSavesForTier applies the glove tier to a rock's glove
// sets (GLOVE_STANDARD_ROCKS etc., plugin.js:334-338).
func (m *mining) gloveSavesForTier(tier int, rock *Rock) int {
	if tier == 0 || rock == nil {
		return 0
	}
	if gloveStandardRocks[rock.Name] {
		return tier
	}
	if gloveSuperiorRocks[rock.Name] {
		if tier-1 > 0 {
			return tier - 1
		}
		return 0
	}
	if tier == 3 && gloveExpertRocks[rock.Name] {
		return 1
	}
	return 0
}

var gloveStandardRocks = map[string]bool{
	"Copper rocks": true, "Tin rocks": true, "Iron rocks": true,
	"Silver rocks": true, "Lead rocks": true, "Coal rocks": true, "Gold rocks": true,
}
var gloveSuperiorRocks = map[string]bool{
	"Mithril rocks": true, "Adamantite rocks": true,
}
var gloveExpertRocks = map[string]bool{
	"Sandstone rocks": true, "Sunstone rocks": true, "Runite rocks": true, "Amethyst crystals": true,
}

// gloveHolds spends one glove save after the rock's ordinary
// yield has run out (gloveHolds, plugin.js:387-395).
func (m *mining) gloveHolds(player *game.Player, rockObject plugins.GameObject, rock *Rock) bool {
	saved, ok := m.gloveSaves[rockObject]
	if !ok {
		saved = &gloveSaveState{}
		m.gloveSaves[rockObject] = saved
	}
	if saved.used >= m.gloveSaveCount(player, rock) {
		return false
	}
	saved.used++
	saved.baseExhausted = true
	return true
}

// ---- depletion / respawn ----

// respawnTicksFor is how long a mined-out rock stays empty
// (respawnTicksFor, plugin.js:631-636): the rock's time, which
// content may shorten ("mining:respawn-ticks").
func (m *mining) respawnTicksFor(rockObject plugins.GameObject, rock *Rock) int {
	req := &respawnTicksRequest{Object: rockObject, Rock: rock.Name, Ticks: rock.RespawnTicks}
	m.emitEvent("mining:respawn-ticks", req)
	if req.Ticks > 0 {
		return req.Ticks
	}
	return rock.RespawnTicks
}

// respawnTicksRequest is the "mining:respawn-ticks" payload.
type respawnTicksRequest struct {
	Object plugins.GameObject
	Rock   string
	Ticks  int
}

// replaceRock swaps another loc into a rock's place (replaceRock,
// plugin.js:545-554). Returns the replacement, or nil when there
// is no object manager.
func (m *mining) replaceRock(rockObject plugins.GameObject, locID int) plugins.GameObject {
	om := m.objectManager()
	if om == nil || rockObject == nil {
		return nil
	}
	replacement := game.NewGameObject(locID, rockObject.Location(), rockObject.Type(), rockObject.Face(), objectPrivateArea(rockObject))
	om.Deregister(rockObject, true)
	om.Register(replacement, true)
	return replacement
}

// nextStageID is a staged rock's next stage after this ore
// (nextStageId, plugin.js:557-562), or -1 on its last.
func (m *mining) nextStageID(rockObject plugins.GameObject, rock *Rock) int {
	if rock == nil || len(rock.Stages) == 0 || m.tables == nil {
		return -1
	}
	stage := -1
	for i, name := range rock.Stages {
		if name == m.tables.LocGameval(rockObject.ID()) {
			stage = i
			break
		}
	}
	if stage >= 0 && stage < len(rock.Stages)-1 {
		return m.tables.LocID(rock.Stages[stage+1])
	}
	return -1
}

// depleteRock mines a rock out unless content keeps it standing
// ("mining:deplete-rock", plugin.js:565-592). True when it went.
func (m *mining) depleteRock(player *game.Player, rockObject plugins.GameObject, rock *Rock, heldByGloves bool) bool {
	gloveSaveAdditive := heldByGloves && rock.Yield == nil && len(rock.Stages) == 0
	event := &depleteRockEvent{Player: player, Object: rockObject, Rock: rock.Name, Handled: heldByGloves, GloveSaveAdditive: gloveSaveAdditive}
	m.emitEvent("mining:deplete-rock", event)
	if event.Handled {
		return false
	}
	// A staged rock grows back whole, from whichever stage it
	// was mined out.
	whole := rockObject
	if len(rock.Stages) > 0 && m.tables != nil {
		if first := m.tables.LocID(rock.Stages[0]); first >= 0 {
			whole = game.NewGameObject(first, rockObject.Location(), rockObject.Type(), rockObject.Face(), objectPrivateArea(rockObject))
		}
	}
	m.exhaustRock(rockObject, rock, []*game.Player{player}, whole)
	if rock.EmptyMessage != "" {
		player.SendMessage(rock.EmptyMessage)
	}
	return true
}

// depleteRockEvent is the "mining:deplete-rock" payload.
type depleteRockEvent struct {
	Player            *game.Player
	Object            plugins.GameObject
	Rock              string
	Handled           bool
	GloveSaveAdditive bool
	Water             bool
}

// minersKeepStanding reports whether content keeps a rock
// standing for the players mining it ("mining:deplete-rock"
// with water: true, plugin.js:595-608).
func (m *mining) minersKeepStanding(rockObject plugins.GameObject, rock *Rock) bool {
	for player, state := range m.sessions {
		if state.objectID != rockObject.ID() || !state.location.Equals(rockObject.Location()) {
			continue
		}
		event := &depleteRockEvent{Player: player, Object: rockObject, Rock: rock.Name, Water: true}
		m.emitEvent("mining:deplete-rock", event)
		if event.Handled {
			return true
		}
	}
	return false
}

// exhaustRock mines a rock out with nothing to keep it standing
// (exhaustRock, plugin.js:611-629): its mined-out loc,
// depletion sound and respawn, and the respawn timer for each
// viewer.
func (m *mining) exhaustRock(rockObject plugins.GameObject, rock *Rock, viewers []*game.Player, whole plugins.GameObject) plugins.GameObject {
	delete(m.rockTimers, rockObject)
	delete(m.rockYields, rockObject)
	delete(m.gloveSaves, rockObject)
	depletedID := -1
	if m.tables != nil {
		depletedID = m.tables.DepletedRockID(rockObject.ID(), rock)
	}
	var depleted plugins.GameObject
	if depletedID >= 0 {
		depleted = m.replaceRock(rockObject, depletedID)
	}
	if rock.DepletionSound != nil && depleted != nil {
		at := depleted.Location()
		game.PlayAreaSound(game.PlayAreaSoundOptions{
			SoundID: rock.DepletionSound.ID, Delay: rock.DepletionSound.Delay,
			Radius: rock.DepletionSound.Radius, X: at.X, Y: at.Y, Level: at.Z,
		})
	}
	respawnTicks := m.respawnTicksFor(rockObject, rock)
	if depleted != nil {
		m.scheduleRespawn(respawnTicks, whole, depleted)
		for _, viewer := range viewers {
			m.sendRespawnTimer(viewer, depleted, respawnTicks)
		}
	}
	return depleted
}

// scheduleRespawn schedules a mined-out rock's respawn
// (RockRespawnTask, plugin.js:383-399): when the timer fires,
// the standing depleted loc is removed and the original rock
// registered again.
func (m *mining) scheduleRespawn(delayTicks int, original, depleted plugins.GameObject) {
	tm := m.taskManager()
	if tm == nil {
		return
	}
	if delayTicks < 1 {
		delayTicks = 1
	}
	task := game.NewTask(delayTicks, func(cycle int64) bool {
		if om := m.objectManager(); om != nil {
			if existing := om.Get(depleted.ID(), depleted.Location()); existing != nil {
				om.Deregister(existing, true)
			}
			om.Register(original, true)
		}
		return true
	})
	task.Name = "MiningRockRespawn"
	tm.Schedule(task)
}

// sendRespawnTimer sends a depleted rock's respawn countdown
// (sendRespawnTimer, plugin.js:536-542): the packed coordinate
// (z<<28)|(x<<14)|y, depleted id/type, timer id 1, ticks,
// colour 16765184, 0. Sent to the miner AND all local players
// (the woodcutting precedent).
func (m *mining) sendRespawnTimer(player *game.Player, depleted plugins.GameObject, ticks int) {
	if depleted == nil {
		return
	}
	location := depleted.Location()
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
			game.ClientScriptInt(depleted.ID()),
			game.ClientScriptInt(depleted.Type()),
			game.ClientScriptInt(RespawnTimerID),
			game.ClientScriptInt(ticks),
			game.ClientScriptInt(RespawnTimerColour),
			game.ClientScriptInt(0))
	}
}

// clientScriptSender is the SendClientScript half of the
// PacketSender interface (the woodcutting precedent).
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

// ---- interaction handlers ----

// mineDepletedRock handles Mine on a depleted rock or vein
// (mineDepletedRock, plugin.js:1081-1110): it only says it is
// empty (sound 2661 + the no-ore line).
func (m *mining) mineDepletedRock(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || m.tables == nil {
		return false
	}
	if event.Definition.Name() == DepletedVein {
		m.stopMining(event.Player, false)
		event.Player.PerformAnimation(game.ResetAnimationID, 0)
		game.SendSoundEffect(event.Player, NoOreSound, 1, 0, 1)
		spam(event.Player, NoOreInVein)
		event.Handled = true
		return true
	}
	gameval := m.tables.LocGameval(event.ObjectID)
	depleted := gameval == "rocks1" || gameval == "rocks2" ||
		emptyRockPattern.MatchString(gameval) ||
		containsString(EmptyRockNames, event.Definition.Name())
	if !depleted {
		return false
	}
	m.stopMining(event.Player, false)
	event.Player.PerformAnimation(game.ResetAnimationID, 0)
	game.SendSoundEffect(event.Player, NoOreSound, 1, 0, 1)
	event.Player.SendMessage(NoOreMessage)
	event.Handled = true
	return true
}

var emptyRockPattern = regexp.MustCompile(`_empty\d*$`)

func containsString(list []string, want string) bool {
	for _, item := range list {
		if item == want {
			return true
		}
	}
	return false
}

// handleMine handles Mine on a rock (handleMine,
// plugin.js:1113-1121).
func (m *mining) handleMine(event *plugins.ObjectInteractionEvent) bool {
	if event == nil || event.Player == nil || m.tables == nil {
		return false
	}
	rock := m.tables.RockByName(event.Definition.Name())
	if rock == nil {
		return false
	}
	if m.startMining(event.Player, event.Object, rock) {
		event.Handled = true
		return true
	}
	return false
}

// NpcRockRequest is the "mining:mine-npc-rock" payload
// (plugin.js:1124-1134).
type NpcRockRequest struct {
	Player   *game.Player
	NPC      *game.NPC
	RockName string
	Stands   func() bool
}

// mineNpcRock mines a rock that is an NPC (plugin.js:1124-1134).
func (m *mining) mineNpcRock(payload any) {
	req, ok := payload.(*NpcRockRequest)
	if !ok || req == nil || req.Player == nil || req.NPC == nil || req.Stands == nil {
		return
	}
	if m.tables == nil {
		return
	}
	rock := m.tables.RockByName(req.RockName)
	if rock == nil {
		return
	}
	target := m.npcRockTarget(&npcRockState{npc: req.NPC, stands: req.Stands})
	if target == nil {
		return
	}
	if m.startMining(req.Player, target, rock) {
		if state, ok := m.sessions[req.Player]; ok {
			state.npcRock = &npcRockState{npc: req.NPC, stands: req.Stands}
		}
	}
}

// npcRockTarget is the loop's view of an NPC rock
// (npcRockTarget, plugin.js:1137-1159).
func (m *mining) npcRockTarget(state *npcRockState) plugins.GameObject {
	if state == nil || state.npc == nil || state.stands == nil || !state.stands() {
		return nil
	}
	return &npcGameObject{npc: state.npc}
}

// npcGameObject adapts an NPC to the GameObject interface the
// loop reads (id, location, private area, interaction distance).
type npcGameObject struct {
	npc *game.NPC
}

func (o *npcGameObject) ID() int                        { return o.npc.ContentID() }
func (o *npcGameObject) Location() game.Location        { return o.npc.Location() }
func (o *npcGameObject) Type() int                      { return 0 }
func (o *npcGameObject) Face() int                      { return 0 }
func (o *npcGameObject) PrivateArea() *game.PrivateArea { return nil }

// IsWithinInteractionDistance for an NPC rock: within 1 tile
// (npcRockTarget, plugin.js:1153-1157).
func (o *npcGameObject) IsWithinInteractionDistance(from game.Location, def game.ObjectDef) bool {
	at := o.npc.Location()
	dx := from.X - at.X
	if dx < 0 {
		dx = -dx
	}
	dy := from.Y - at.Y
	if dy < 0 {
		dy = -dy
	}
	max := dx
	if dy > max {
		max = dy
	}
	return max <= 1 && from.Z == at.Z
}

// interruptMining ends the session on a cleared interaction
// (interruptMining, plugin.js:1162-1164).
func (m *mining) interruptMining(payload any) {
	req, ok := payload.(*interactionClearedEvent)
	if !ok || req == nil {
		return
	}
	m.stopMining(req.Player, true)
}

// interactionClearedEvent is the "player:interaction-cleared"
// payload.
type interactionClearedEvent struct {
	Player *game.Player
}

// interruptMiningForEmote ends the session on an accepted emote,
// keeping the emote's animation (plugin.js:1167-1169).
func (m *mining) interruptMiningForEmote(payload any) {
	req, ok := payload.(*emoteEvent)
	if !ok || req == nil {
		return
	}
	m.stopMining(req.Player, false)
}

// emoteEvent is the "player:emote" payload.
type emoteEvent struct {
	Player *game.Player
}

// endMiningSession ends the session on disconnect, without
// resetting the shared roll timer (endMiningSession,
// plugin.js:1172-1175).
func (m *mining) endMiningSession(event *plugins.PlayerDisconnectEvent) {
	if event == nil {
		return
	}
	m.stopMining(event.Player, false)
	delete(m.rollReady, event.Player)
}

// pauseMining holds the session through a level-up pop-up
// (pauseMining, plugin.js:1178-1180).
func (m *mining) pauseMining(payload any) {
	event, ok := payload.(*skillactions.LevelUpPopupOpenedPayload)
	if !ok || event == nil {
		return
	}
	sessions := make(map[*game.Player]skillactions.Session, len(m.sessions))
	for player, sess := range m.sessions {
		sessions[player] = sess
	}
	skillactions.PauseForLevelUp(event, sessions, func(player *game.Player) {
		m.stopMining(player, false)
	}, nil)
}

// ---- register ----

func (m *mining) register(api *plugins.Api) {
	if m.tables == nil {
		return
	}
	api.OnPlayerDisconnect(m.endMiningSession)
	api.OnCustomEvent("player:interaction-cleared", m.interruptMining)
	api.OnCustomEvent("player:emote", m.interruptMiningForEmote)
	api.OnCustomEvent("level-up:popup-opened", m.pauseMining)
	api.OnCustomEvent("mining:best-pickaxe", m.answerBestPickaxe)
	api.OnCustomEvent("mining:pickaxe", m.answerPickaxe)
	api.OnCustomEvent("mining:roll-interval", m.answerRollInterval)
	api.OnCustomEvent("mining:gem-roll", m.answerGemRoll)
	api.PersistAttribute(OreMinedAttribute, 0)
	api.OnPlayerLogin(func(event *plugins.PlayerLoginEvent) {
		if event != nil {
			m.sendOresMined(event.Player)
		}
	})
	api.OnCustomEvent("mining:count-ore", m.countKeptOre)
	api.OnCustomEvent("mining:mine-npc-rock", m.mineNpcRock)
	api.OnCustomEvent("mining:ores-mined", m.answerOresMined)
	api.OnObjectInteractionNamed("Rocks", map[string]plugins.ObjectInteractionHandler{"Mine": m.mineDepletedRock})
	for _, name := range EmptyRockNames {
		api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{"Mine": m.mineDepletedRock})
	}
	api.OnObjectInteractionNamed(DepletedVein, map[string]plugins.ObjectInteractionHandler{"Mine": m.mineDepletedRock})
	for _, rock := range allRocks(m.tables) {
		name := rock.Name
		api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{"Mine": m.handleMine})
	}
	m.registerCrystalPickaxe(api)
	m.registerInfernalPickaxe(api)
	m.registerClueGeodes(api)
	m.registerCalcifiedWater(api)
	m.registerSaltpetre(api)
	m.registerVarrockArmour(api)
	m.registerProspectorOutfit(api)
	m.registerAncientEssence(api)
}

// allRocks returns ROCKS + ORE_VEINS + ESSENCE_ROCKS
// (ALL_ROCKS, plugin.js:307).
func allRocks(t *Tables) []Rock {
	if t == nil {
		return nil
	}
	out := make([]Rock, 0, len(t.Rocks)+len(t.Veins)+len(t.EssenceRocks))
	out = append(out, t.Rocks...)
	out = append(out, t.Veins...)
	out = append(out, t.EssenceRocks...)
	return out
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
