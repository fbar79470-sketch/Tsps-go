// Package woodcutting holds the Woodcutting skill tables: the
// verbatim data of server/plugins/skills/Woodcutting.plugin.js
// (upstream commit bfd81e3), resolved against the cache at load
// time. Ids are SYMBOLIC in the plugin (ItemIds.* / ObjectIds.*);
// the tables below store the cache gameval names for those ids
// (for example ItemIds.BRONZE_AXE is gameval "bronze_axe") and
// Load resolves every name through an IDResolver —
// LoadFromCache adapts *cache.CacheDefinitions, so no numeric id
// in this file is guessed: a name the cache does not know fails
// Load with the exact name listed. Values that are numeric
// literals in the plugin itself (animation ids, sounds, ticks,
// XP, weights, pet bases) are copied verbatim as literals and
// cited to the plugin line.
//
// Line numbers in comments cite Woodcutting.plugin.js unless
// another file is named. The tables are read-only after Load;
// Worker B owns the chopping loop that consumes them.
package woodcutting

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// AxeSpeedByTier is the per-tier chop speed (Woodcutting.plugin.js:182).
// Wintertodt's own supply roll reads it; it follows the axe's
// cut-chance tier.
var AxeSpeedByTier = [8]float64{0.03, 0.05, 0.09, 0.11, 0.13, 0.16, 0.19, 0.25}

// ---- constants (Woodcutting.plugin.js:55-74, 125-128, 769) ----

const (
	// WoodcuttingActionIntervalTicks is the ticks between log rolls
	// (WOODCUTTING_ACTION_INTERVAL_TICKS, plugin.js:55).
	WoodcuttingActionIntervalTicks = 4
	// ChopAnimationIntervalTicks is the ticks between chop animations
	// (CHOP_ANIMATION_INTERVAL_TICKS, plugin.js:56).
	ChopAnimationIntervalTicks = 4
	// FirstRollDelayTicks is the delay before the first roll and
	// animation after starting (FIRST_ROLL_DELAY_TICKS, plugin.js:57).
	FirstRollDelayTicks = 2
	// InventoryFullSound is the sound on a full inventory
	// (INVENTORY_FULL_SOUND, plugin.js:60).
	InventoryFullSound = 2277
	// RespawnTimerScript is the client script drawing a depleted
	// tree's respawn countdown over its stump
	// (RESPAWN_TIMER_SCRIPT, plugin.js:62).
	RespawnTimerScript = 5474
	// RespawnTimerID is the overlay timer id (plugin.js:63).
	RespawnTimerID = 2
	// RespawnTimerColour is the overlay timer colour (plugin.js:64).
	RespawnTimerColour = 16765184
	// MultiTreeDepletionRollMax is the fallback depletion roll bound
	// for multi-log trees (MULTI_TREE_DEPLETION_ROLL_MAX, plugin.js:69).
	MultiTreeDepletionRollMax = 15
	// MultiTreeDepletionThreshold is the fallback depletion threshold
	// (MULTI_TREE_DEPLETION_THRESHOLD, plugin.js:70).
	MultiTreeDepletionThreshold = 2
	// BirdNestDropChance is the 1-in-N nest chance per successful roll
	// (BIRD_NEST_DROP_CHANCE, plugin.js:74).
	BirdNestDropChance = 256
	// NestFallSound is the sound when a nest falls (NEST_FALL_SOUND,
	// plugin.js:125).
	NestFallSound = 1516
	// WoodcuttingCapeNestMultiplier is the nest-chance multiplier for
	// a worn Woodcutting cape (plugin.js:128).
	WoodcuttingCapeNestMultiplier = 1.1
	// GroupBonusMax caps the Forestry group Woodcutting bonus
	// (GROUP_BONUS_MAX, plugin.js:769).
	GroupBonusMax = 10
	// LumberjackSetBonus is the extra XP multiplier for the full
	// Lumberjack outfit (LUMBERJACK_SET_BONUS, plugin.js:136).
	LumberjackSetBonus = 0.005
	// LumberjackSetText is the equipment-effects set-text id for the
	// full Lumberjack outfit (enum 3047; plugin.js:138).
	LumberjackSetText = 17
	// FarmedRedwoodDepletionRoll is the farmed redwood's 1-in-N
	// depletion roll (farming/Redwood.Farming.js:5, DEPLETION_ROLL).
	FarmedRedwoodDepletionRoll = 11
	// FarmedRedwoodRegrowTicks is the farmed redwood's regrow timer
	// (farming/Redwood.Farming.js:6, REGROW_TICKS).
	FarmedRedwoodRegrowTicks = 200
	// FarmedHardwoodRespawnTicks is the regrow timer shared by the
	// farmed camphor, ironwood and rosewood (plugin.js:667-670).
	FarmedHardwoodRespawnTicks = 150
)

const (
	// SwingMessage is spammed when chopping starts (plugin.js:58).
	SwingMessage = "You swing your axe at the tree."
	// NoAxeMessage is sent when no usable axe is held (plugin.js:59).
	NoAxeMessage = "You need an axe to chop down this tree.<br>You do not have an axe which you have the woodcutting level to use."
	// NestFullMessage is the statement shown when a nest cannot be
	// searched for lack of room (NEST_FULL_MESSAGE, plugin.js:126).
	NestFullMessage = "Your inventory is too full to take anything out of the bird's nest."
	// LogsChoppedVarp is the gameval name of the logs-chopped
	// tracker varp (LOGS_CHOPPED_VARP, plugin.js:66); resolved to
	// Tables.LogsChoppedVarpID at Load.
	LogsChoppedVarp = "tracking_logs_chopped"
	// LogsChoppedAttribute is the persisted logs-chopped attribute
	// (LOGS_CHOPPED_ATTRIBUTE, plugin.js:67).
	LogsChoppedAttribute = "woodcutting:logs-chopped"
)

// SeersVillage is the Kandarin diary's Seers' Village region: the
// maples inside it pay the diary bonus (plugin.js:145). The box is
// cut off west of x 2700; the four maples at x 2679-2684 belong to
// the diary's "Woods west of Sinclair Mansion" and pay 100.
var SeersVillage = Region{MinX: 2700, MaxX: 2780, MinY: 3430, MaxY: 3530, Z: 0}

// Region is an inclusive x/y box on one plane.
type Region struct {
	MinX, MaxX, MinY, MaxY, Z int
}

// CutRange is one [low, high] cut-chance row for an axe tier
// (Woodcutting.plugin.js:234-251). Has is false for a null row:
// blisterwood lists only bronze and dragon, and missing tiers
// scale off the normal chart (see CutChanceRow).
type CutRange struct {
	Low, High int
	Has       bool
}

// CutChart is the eight [low, high] rows for axe tiers
// bronze..dragon.
type CutChart [8]CutRange

// CutChance is the cut-chance chart per tree kind
// (Woodcutting.plugin.js:234-251): [low, high] per axe tier
// (bronze, iron, steel, black, mithril, adamant, rune, dragon).
// A log is rolled every 4 ticks with chance
// (1 + floor(low*(99-L)/98) + floor(high*(L-1)/98)) / 256.
// Achey, burnt, dramen and juniper trees use the normal chart;
// arctic pine and jatoba share maple's. The Wiki lists camphor
// black as 17/71, above adamant; kept as published.
var CutChance = map[string]CutChart{

	"normal": {
		{Low: 64, High: 200, Has: true},
		{Low: 96, High: 300, Has: true},
		{Low: 128, High: 400, Has: true},
		{Low: 144, High: 450, Has: true},
		{Low: 160, High: 500, Has: true},
		{Low: 192, High: 600, Has: true},
		{Low: 224, High: 700, Has: true},
		{Low: 240, High: 750, Has: true},
	},
	"oak": {
		{Low: 32, High: 100, Has: true},
		{Low: 48, High: 150, Has: true},
		{Low: 64, High: 200, Has: true},
		{Low: 72, High: 225, Has: true},
		{Low: 80, High: 250, Has: true},
		{Low: 96, High: 300, Has: true},
		{Low: 112, High: 350, Has: true},
		{Low: 120, High: 375, Has: true},
	},
	"willow": {
		{Low: 16, High: 50, Has: true},
		{Low: 24, High: 75, Has: true},
		{Low: 32, High: 100, Has: true},
		{Low: 36, High: 112, Has: true},
		{Low: 40, High: 125, Has: true},
		{Low: 48, High: 150, Has: true},
		{Low: 56, High: 175, Has: true},
		{Low: 60, High: 187, Has: true},
	},
	"teak": {
		{Low: 15, High: 46, Has: true},
		{Low: 23, High: 70, Has: true},
		{Low: 31, High: 93, Has: true},
		{Low: 35, High: 102, Has: true},
		{Low: 39, High: 117, Has: true},
		{Low: 47, High: 140, Has: true},
		{Low: 55, High: 164, Has: true},
		{Low: 60, High: 190, Has: true},
	},
	"maple": {
		{Low: 8, High: 25, Has: true},
		{Low: 12, High: 37, Has: true},
		{Low: 16, High: 50, Has: true},
		{Low: 18, High: 56, Has: true},
		{Low: 20, High: 62, Has: true},
		{Low: 24, High: 75, Has: true},
		{Low: 28, High: 87, Has: true},
		{Low: 30, High: 93, Has: true},
	},
	"mahogany": {
		{Low: 8, High: 25, Has: true},
		{Low: 12, High: 38, Has: true},
		{Low: 16, High: 50, Has: true},
		{Low: 18, High: 54, Has: true},
		{Low: 20, High: 63, Has: true},
		{Low: 25, High: 75, Has: true},
		{Low: 29, High: 88, Has: true},
		{Low: 34, High: 94, Has: true},
	},
	"yew": {
		{Low: 4, High: 12, Has: true},
		{Low: 6, High: 19, Has: true},
		{Low: 8, High: 25, Has: true},
		{Low: 9, High: 28, Has: true},
		{Low: 10, High: 31, Has: true},
		{Low: 12, High: 37, Has: true},
		{Low: 14, High: 44, Has: true},
		{Low: 15, High: 47, Has: true},
	},
	"magic": {
		{Low: 2, High: 6, Has: true},
		{Low: 3, High: 9, Has: true},
		{Low: 4, High: 12, Has: true},
		{Low: 5, High: 13, Has: true},
		{Low: 5, High: 15, Has: true},
		{Low: 6, High: 18, Has: true},
		{Low: 7, High: 21, Has: true},
		{Low: 7, High: 22, Has: true},
	},
	"redwood": {
		{Low: 2, High: 6, Has: true},
		{Low: 3, High: 9, Has: true},
		{Low: 4, High: 12, Has: true},
		{Low: 4, High: 14, Has: true},
		{Low: 5, High: 15, Has: true},
		{Low: 6, High: 18, Has: true},
		{Low: 7, High: 21, Has: true},
		{Low: 7, High: 30, Has: true},
	},
	"hollow": {
		{Low: 18, High: 26, Has: true},
		{Low: 28, High: 40, Has: true},
		{Low: 36, High: 54, Has: true},
		{Low: 42, High: 57, Has: true},
		{Low: 46, High: 68, Has: true},
		{Low: 59, High: 81, Has: true},
		{Low: 64, High: 94, Has: true},
		{Low: 67, High: 101, Has: true},
	},
	"blisterwood": {
		{Low: 15, High: 50, Has: true},
		{Has: false},
		{Has: false},
		{Has: false},
		{Has: false},
		{Has: false},
		{Has: false},
		{Low: 61, High: 186, Has: true},
	},
	"camphor": {
		{Low: 8, High: 23, Has: true},
		{Low: 11, High: 36, Has: true},
		{Low: 15, High: 47, Has: true},
		{Low: 17, High: 71, Has: true},
		{Low: 19, High: 59, Has: true},
		{Low: 23, High: 71, Has: true},
		{Low: 27, High: 83, Has: true},
		{Low: 32, High: 89, Has: true},
	},
	"ironwood": {
		{Low: 6, High: 21, Has: true},
		{Low: 10, High: 38, Has: true},
		{Low: 13, High: 42, Has: true},
		{Low: 15, High: 46, Has: true},
		{Low: 17, High: 53, Has: true},
		{Low: 21, High: 63, Has: true},
		{Low: 24, High: 75, Has: true},
		{Low: 29, High: 80, Has: true},
	},
	"rosewood": {
		{Low: 6, High: 18, Has: true},
		{Low: 9, High: 28, Has: true},
		{Low: 12, High: 37, Has: true},
		{Low: 13, High: 40, Has: true},
		{Low: 15, High: 47, Has: true},
		{Low: 18, High: 56, Has: true},
		{Low: 21, High: 66, Has: true},
		{Low: 25, High: 70, Has: true},
	},
}

// AxeDef is one row of the AXES source table: the item is named by
// its cache gameval name and resolved at Load.
type AxeDef struct {
	// Item is the cache gameval name of the axe item.
	Item string
	// RequiredLevel is the Woodcutting level to use the axe.
	RequiredLevel int
	// Tier indexes the CUT_CHANCE rows (0 bronze .. 7 dragon).
	Tier int
	// AnimationID is the chop animation (RuneLite WOODCUTTING_*
	// constant, a verbatim literal in the plugin).
	AnimationID int
	// Felling marks a felling axe (Forester's rations enable its
	// extra XP and reduced log yield).
	Felling bool
	// Standard marks a standard bronze-to-dragon axe (bots only
	// ever get these; findBestUsableAxeByLevel).
	Standard bool
	// RequiredAgility is 0 when the axe has no Agility requirement.
	RequiredAgility int
	// RequiredFiremaking is 0 when the axe has no Firemaking
	// requirement.
	RequiredFiremaking int
	// Speed overrides AxeSpeedByTier[Tier] when non-zero (charged
	// infernal axes: 0.3).
	Speed float64
}

// AxeDefs is the AXES table (Woodcutting.plugin.js:193-227), worst
// to best: a player uses the last axe here they hold and can use.
// NOTE: the plugin lists 30 entries, not 31 — the Phase 32 plan
// §1 says 31, which is wrong; this table is verbatim from the
// plugin (30 axe(...) calls). Tier indexes CUT_CHANCE: 3rd age,
// crystal and infernal axes cut like dragon, gilded like rune and
// blessed like mithril. Felling axes cut like their standard
// counterpart. Animation ids are RuneLite's WOODCUTTING_*
// constants; blessed has none of its own and uses mithril's. There
// is NO blessed axe: it "does not retain the functionality of the
// mithril axe used to create it" and only chops the undead trees
// (plugin.js:202-203).
var AxeDefs = []AxeDef{

	{Item: "bronze_axe_2h", RequiredLevel: 1, Tier: 0, AnimationID: 10064, Felling: true},                                 // ItemIds.BRONZE_FELLING_AXE (plugin.js:194)
	{Item: "bronze_axe", RequiredLevel: 1, Tier: 0, AnimationID: 879, Standard: true},                                     // ItemIds.BRONZE_AXE (plugin.js:195)
	{Item: "iron_axe_2h", RequiredLevel: 1, Tier: 1, AnimationID: 10065, Felling: true},                                   // ItemIds.IRON_FELLING_AXE (plugin.js:196)
	{Item: "iron_axe", RequiredLevel: 1, Tier: 1, AnimationID: 877, Standard: true},                                       // ItemIds.IRON_AXE (plugin.js:197)
	{Item: "steel_axe_2h", RequiredLevel: 6, Tier: 2, AnimationID: 10066, Felling: true},                                  // ItemIds.STEEL_FELLING_AXE (plugin.js:198)
	{Item: "steel_axe", RequiredLevel: 6, Tier: 2, AnimationID: 875, Standard: true},                                      // ItemIds.STEEL_AXE (plugin.js:199)
	{Item: "black_axe_2h", RequiredLevel: 11, Tier: 3, AnimationID: 10067, Felling: true},                                 // ItemIds.BLACK_FELLING_AXE (plugin.js:200)
	{Item: "black_axe", RequiredLevel: 11, Tier: 3, AnimationID: 873, Standard: true},                                     // ItemIds.BLACK_AXE (plugin.js:201)
	{Item: "mithril_axe_2h", RequiredLevel: 21, Tier: 4, AnimationID: 10068, Felling: true},                               // ItemIds.MITHRIL_FELLING_AXE (plugin.js:204)
	{Item: "mithril_axe", RequiredLevel: 21, Tier: 4, AnimationID: 871, Standard: true},                                   // ItemIds.MITHRIL_AXE (plugin.js:205)
	{Item: "adamant_axe_2h", RequiredLevel: 31, Tier: 5, AnimationID: 10069, Felling: true},                               // ItemIds.ADAMANT_FELLING_AXE (plugin.js:206)
	{Item: "adamant_axe", RequiredLevel: 31, Tier: 5, AnimationID: 869, Standard: true},                                   // ItemIds.ADAMANT_AXE (plugin.js:207)
	{Item: "trail_gilded_axe", RequiredLevel: 41, Tier: 6, AnimationID: 8303},                                             // ItemIds.GILDED_AXE (plugin.js:208)
	{Item: "rune_axe_2h", RequiredLevel: 41, Tier: 6, AnimationID: 10070, Felling: true},                                  // ItemIds.RUNE_FELLING_AXE (plugin.js:209)
	{Item: "rune_axe", RequiredLevel: 41, Tier: 6, AnimationID: 867, Standard: true},                                      // ItemIds.RUNE_AXE (plugin.js:210)
	{Item: "3a_axe_2h", RequiredLevel: 61, Tier: 7, AnimationID: 10074, Felling: true},                                    // ItemIds._3RD_AGE_FELLING_AXE (plugin.js:211)
	{Item: "dragon_axe_2h", RequiredLevel: 61, Tier: 7, AnimationID: 10071, Felling: true},                                // ItemIds.DRAGON_FELLING_AXE (plugin.js:212)
	{Item: "3a_axe", RequiredLevel: 61, Tier: 7, AnimationID: 7264},                                                       // ItemIds._3RD_AGE_AXE (plugin.js:213)
	{Item: "dragon_axe", RequiredLevel: 61, Tier: 7, AnimationID: 2846, Standard: true},                                   // ItemIds.DRAGON_AXE (plugin.js:214)
	{Item: "infernal_axe_empty", RequiredLevel: 61, Tier: 7, AnimationID: 2117, RequiredFiremaking: 85},                   // ItemIds.INFERNAL_AXE_UNCHARGED_ (plugin.js:215)
	{Item: "trailblazer_axe_empty", RequiredLevel: 61, Tier: 7, AnimationID: 8778, RequiredFiremaking: 85},                // ItemIds.INFERNAL_AXE_UNCHARGED__2 (plugin.js:216)
	{Item: "trailblazer_reloaded_axe_empty", RequiredLevel: 61, Tier: 7, AnimationID: 8778, RequiredFiremaking: 85},       // ItemIds.INFERNAL_AXE_UNCHARGED__4 (plugin.js:217)
	{Item: "crystal_axe_inactive", RequiredLevel: 71, Tier: 7, AnimationID: 8324, RequiredAgility: 50},                    // ItemIds.CRYSTAL_AXE_INACTIVE_ (plugin.js:218)
	{Item: "crystal_axe_2h_inactive", RequiredLevel: 71, Tier: 7, AnimationID: 10072, Felling: true, RequiredAgility: 50}, // ItemIds.CRYSTAL_FELLING_AXE_INACTIVE_ (plugin.js:219)
	{Item: "crystal_axe_2h", RequiredLevel: 71, Tier: 7, AnimationID: 10072, Felling: true, RequiredAgility: 50},          // ItemIds.CRYSTAL_FELLING_AXE (plugin.js:220)
	{Item: "crystal_axe", RequiredLevel: 71, Tier: 7, AnimationID: 8324, RequiredAgility: 50},                             // ItemIds.CRYSTAL_AXE (plugin.js:221)
	{Item: "infernal_axe", RequiredLevel: 61, Tier: 7, AnimationID: 2117, RequiredFiremaking: 85, Speed: 0.3},             // ItemIds.INFERNAL_AXE (plugin.js:222)
	{Item: "trailblazer_axe", RequiredLevel: 61, Tier: 7, AnimationID: 8778, RequiredFiremaking: 85, Speed: 0.3},          // ItemIds.INFERNAL_AXE_OR_ (plugin.js:223)
	{Item: "trailblazer_reloaded_axe", RequiredLevel: 61, Tier: 7, AnimationID: 8778, RequiredFiremaking: 85, Speed: 0.3}, // ItemIds.INFERNAL_AXE_OR__3 (plugin.js:224)
	{Item: "league_trailblazer_axe", RequiredLevel: 1, Tier: 7, AnimationID: 8324},                                        // ItemIds.ECHO_AXE (plugin.js:226)
}

// DiaryBonus is a tree's diary XP bonus (maple only): the diary,
// tier and percent, paid inside Region.
type DiaryBonus struct {
	Diary   string
	Tier    string
	Percent int
	Region  Region
}

// TreeDef is one row of the TREES source table. Ids are named by
// cache gameval name and resolved at Load; trees with no ObjectIDs
// (hollow, arctic pine, juniper, jatoba, blisterwood, burnt,
// camphor, ironwood, rosewood) are reachable only by object NAME,
// so the port looks them up by name, not id.
type TreeDef struct {
	Name        string
	ObjectNames []string
	Actions     []string
	// RequiredLevel is the Woodcutting level to chop the tree.
	RequiredLevel int
	// XPReward is the XP per log.
	XPReward float64
	// LogItem is the gameval name of the log item.
	LogItem string
	// PetBase is the skilling-pet roll base, or 0 when the tree
	// has none (scrapey, dramen, burnt).
	PetBase int
	// ObjectIDs are the gameval names of the tree's loc ids.
	ObjectIDs []string
	// CutChance names a chart in CutChance; empty means the
	// inline CutChanceChart below applies (scrapey only).
	CutChance      string
	CutChanceChart CutChart
	// RoundedCutChance selects the rounded success formula
	// (scrapey only).
	RoundedCutChance bool
	// RespawnTicks is the stump's regrow delay; RespawnTicksMax,
	// when non-zero, makes it a random roll in
	// [RespawnTicks, RespawnTicksMax].
	RespawnTicks    int
	RespawnTicksMax int
	// Stump is the gameval name of the fallback stump loc, or ""
	// when the tree has none (stump resolution falls back to the
	// model map, then treeId+1, then the default stump).
	Stump string
	// Multi is true when several players can chop the tree at once.
	Multi bool
	// DespawnTicks is the Forestry despawn timer, or 0 when the
	// tree has none.
	DespawnTicks int
	// DepleteChance is the 1-in-N depletion roll for multi trees
	// without a despawn timer, or 0 when the tree has none.
	DepleteChance int
	// Depletes is nil for the default (the tree falls); false
	// when the tree never depletes (dramen, jatoba).
	Depletes *bool
	// LogMessage replaces the default "You get some ..." line.
	LogMessage string
	// BirdNests is nil for the default (bird nests can drop);
	// false when the tree gives clue nests only, never bird nests
	// (normal, achey, redwood).
	BirdNests *bool
	// DiaryXP is the diary bonus, or nil (maple only).
	DiaryXP *DiaryBonus
	// Interrupt* is the blisterwood spider-jump interruption: the
	// message, the seq gameval name and the sound. Empty when the
	// tree has none.
	InterruptMessage   string
	InterruptAnimation string
	InterruptSound     int
}

func boolPtr(b bool) *bool { return &b }

// TreeDefs is the TREES table (Woodcutting.plugin.js:254-651), 21
// entries in plugin order.
var TreeDefs = []TreeDef{

	{ // scrapey tree (plugin.js:255-269)
		Name:          "scrapey tree",
		ObjectNames:   []string{},
		RequiredLevel: 1,
		XPReward:      25,
		LogItem:       "brew_scrapey_logs", // ItemIds.SCRAPEY_TREE_LOGS
		CutChanceChart: CutChart{
			{Low: 23, High: 75, Has: true},
			{Low: 36, High: 113, Has: true},
			{Low: 49, High: 151, Has: true},
			{Low: 55, High: 159, Has: true},
			{Low: 62, High: 191, Has: true},
			{Low: 76, High: 226, Has: true},
			{Low: 85, High: 264, Has: true},
			{Low: 90, High: 286, Has: true},
		},
		RoundedCutChance: true,
		RespawnTicks:     24,
		Stump:            "brew_scrapey_tree_stump", // ObjectIds.SCRAPEY_TREE_2
		Multi:            true,
		DepleteChance:    8,
	},
	{ // normal tree (plugin.js:270-321)
		Name:          "normal tree",
		ObjectNames:   []string{"Tree", "Dead tree", "Evergreen tree", "Dying tree"},
		Actions:       []string{"Chop down", "Chop-down"},
		RequiredLevel: 1,
		XPReward:      25,
		LogItem:       "logs", // ItemIds.LOGS
		PetBase:       317647,
		ObjectIDs: []string{ // ObjectIds.EVERGREEN_TREE..DEAD_TREE_20
			"evergreen_vsnowy_large", "evergreen_snowy_large", "evergreen", "evergreen_large",
			"max_evergreen", "evergreen_vsnowy", "evergreen_snowy", "xmas22_evergreen_snowy_large",
			"xmas22_evergreen_vsnowy", "kharazi_jungle_tree2", "tree", "lighttree",
			"tree2", "tree3", "lighttree2", "deadtree1",
			"deadtree1_large", "deadtree4", "lightdeadtree1", "deadtree2",
			"deadtree2_dark", "deadtree3", "deadtree2_snowy", "snowtree1",
			"snowtree2", "snowtree3", "deadtree2_swamp", "deadtree6",
			"deadtree_burnt", "regicide_tree_large2", "regicide_tree_large3", "regicide_tree_small",
			"mdaughter_impassable_tree", "mdaughter_impassable_tree2", "mdaughter_passable_tree",
		},
		CutChance:       "normal",
		RespawnTicks:    59,
		RespawnTicksMax: 98,
		Stump:           "treestump2", // ObjectIds.TREE_STUMP_2
		Multi:           false,
		BirdNests:       boolPtr(false),
	},
	{ // achey tree (plugin.js:322-336)
		Name:          "achey tree",
		ObjectNames:   []string{"Achey Tree"},
		Actions:       []string{"Chop"},
		RequiredLevel: 1,
		XPReward:      25,
		LogItem:       "achey_tree_logs", // ItemIds.ACHEY_TREE_LOGS
		PetBase:       317647,
		ObjectIDs: []string{ // ObjectIds.ACHEY_TREE
			"achey_tree",
		},
		CutChance:       "normal",
		RespawnTicks:    59,
		RespawnTicksMax: 98,
		Stump:           "achey_tree_stump", // ObjectIds.ACHEY_TREE_STUMP
		Multi:           false,
		BirdNests:       boolPtr(false),
	},
	{ // oak (plugin.js:337-357)
		Name:          "oak",
		ObjectNames:   []string{"Oak tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 15,
		XPReward:      37.5,
		LogItem:       "oak_logs", // ItemIds.OAK_LOGS
		PetBase:       361146,
		ObjectIDs: []string{ // ObjectIds.ARCTIC_PINE_TREE..OAK_TREE_18
			"arctic_pine", "poh_big_tree3_4", "poh_small_tree3_5", "oak_tree_seedling",
			"oak_tree_1", "oak_tree_2", "oak_tree_3", "oak_tree_fullygrown_1",
			"oak_tree_fullygrown_2", "newbieoaktree", "oaktree", "drill_oaktree",
			"tut2_oak", "tut2_oak_noop", "tree_oak_default01", "gim_oaktree",
			"avium_oak_1", "xmas24_oaktree01_snow01", "oaktree_noop",
		},
		CutChance:    "oak",
		RespawnTicks: 14,
		Stump:        "oaktree_stump", // ObjectIds.TREE_STUMP_16
		Multi:        true,
		DespawnTicks: 45,
	},
	{ // willow (plugin.js:358-376)
		Name:          "willow",
		ObjectNames:   []string{"Willow tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 30,
		XPReward:      67.5,
		LogItem:       "willow_logs", // ItemIds.WILLOW_LOGS
		PetBase:       289286,
		ObjectIDs: []string{ // ObjectIds.WILLOW_TREE..WILLOW_TREE_14
			"poh_big_tree4_4", "poh_small_tree4_5", "willow_tree_seedling", "willow_tree_1",
			"willow_tree_2", "willow_tree_3", "willow_tree_4", "willow_tree_5",
			"willow_tree_fullygrown_1", "willow_tree_fullygrown_2", "willowtree", "willow_tree2",
			"willow_tree3", "willow_tree4",
		},
		CutChance:    "willow",
		RespawnTicks: 14,
		Stump:        "willow_tree_stump_new", // ObjectIds.TREE_STUMP_35
		Multi:        true,
		DespawnTicks: 50,
	},
	{ // teak (plugin.js:377-395)
		Name:          "teak",
		ObjectNames:   []string{"Teak tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 35,
		XPReward:      85,
		LogItem:       "teak_logs", // ItemIds.TEAK_LOGS
		PetBase:       264336,
		ObjectIDs: []string{ // ObjectIds.TEAK_TREE..TEAK_TREE_13
			"teaktree", "misc_dummy_teaktree", "teak_tree_seedling", "teak_tree_1",
			"teak_tree_2", "teak_tree_3", "teak_tree_4", "teak_tree_5",
			"teak_tree_6", "teak_tree_fullygrown_claimxp", "teak_tree_fullygrown", "prif_teaktree",
			"teaktree_update",
		},
		CutChance:    "teak",
		RespawnTicks: 15,
		Stump:        "teak_stump", // ObjectIds.TREE_STUMP_32
		Multi:        true,
		DespawnTicks: 50,
	},
	{ // dramen (plugin.js:396-409)
		Name:          "dramen",
		ObjectNames:   []string{"Dramen tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 36,
		XPReward:      0,
		LogItem:       "dramen_branch", // ItemIds.DRAMEN_BRANCH
		ObjectIDs: []string{ // ObjectIds.DRAMEN_TREE
			"dramentree",
		},
		CutChance:    "normal",
		RespawnTicks: 17,
		Multi:        true,
		Depletes:     boolPtr(false),
		LogMessage:   "You cut a branch from the Dramen tree.",
	},
	{ // maple (plugin.js:410-430)
		Name:          "maple",
		ObjectNames:   []string{"Maple tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 45,
		XPReward:      100,
		LogItem:       "maple_logs", // ItemIds.MAPLE_LOGS
		PetBase:       221918,
		ObjectIDs: []string{ // ObjectIds.MAPLE_TREE..MAPLE_TREE_18
			"mapletree", "poh_big_tree5_4", "misc_dummy_mapletree", "poh_small_tree5_5",
			"maple_tree_seedling", "maple_tree_1", "maple_tree_2", "maple_tree_3",
			"maple_tree_4", "maple_tree_5", "maple_tree_6", "maple_tree_7",
			"maple_tree_fullygrown_1", "maple_tree_fullygrown_2", "prif_mapletree", "prif_mapletree_stump",
			"mapletree_update", "mapletree_update_stump",
		},
		CutChance:    "maple",
		RespawnTicks: 59,
		Stump:        "maple_tree_stump_new", // ObjectIds.TREE_STUMP_36
		Multi:        true,
		DespawnTicks: 100,
		DiaryXP:      &DiaryBonus{Diary: "kandarin", Tier: "medium", Percent: 10, Region: SeersVillage},
	},
	{ // mahogany (plugin.js:431-449)
		Name:          "mahogany",
		ObjectNames:   []string{"Mahogany tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 50,
		XPReward:      125,
		LogItem:       "mahogany_logs", // ItemIds.MAHOGANY_LOGS
		PetBase:       220623,
		ObjectIDs: []string{ // ObjectIds.MAHOGANY_TREE..MAHOGANY_TREE_14
			"mahoganytree", "mahogany_tree_1", "mahogany_tree_2", "mahogany_tree_3",
			"mahogany_tree_4", "mahogany_tree_5", "mahogany_tree_6", "mahogany_tree_7",
			"mahogany_tree_8", "mahogany_tree_9", "mahogany_tree_fullygrown_claimxp", "mahogany_tree_fullygrown",
			"prif_mahoganytree", "mahoganytree_update",
		},
		CutChance:    "mahogany",
		RespawnTicks: 14,
		Stump:        "mahogany_stump", // ObjectIds.TREE_STUMP_31
		Multi:        true,
		DespawnTicks: 100,
	},
	{ // yew (plugin.js:450-469)
		Name:          "yew",
		ObjectNames:   []string{"Yew tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 60,
		XPReward:      175,
		LogItem:       "yew_logs", // ItemIds.YEW_LOGS
		PetBase:       145013,
		ObjectIDs: []string{ // ObjectIds.YEW_TREE..YEW_TREE_19
			"poh_big_tree6_4", "poh_small_tree6_5", "yew_tree_1", "yew_tree_2",
			"yew_tree_3", "yew_tree_4", "yew_tree_5", "yew_tree_6",
			"yew_tree_7", "yew_tree_8", "yew_tree_9", "yew_tree_fullygrown_1",
			"yew_tree_fullygrown_2", "yewtree", "prif_yewtree", "yewtree_update",
			"tree_yew_default01", "yewtree_noop", "yewtree_update2",
		},
		CutChance:    "yew",
		RespawnTicks: 99,
		Stump:        "yew_tree_stump_new", // ObjectIds.TREE_STUMP_38
		Multi:        true,
		DespawnTicks: 190,
	},
	{ // magic (plugin.js:470-489)
		Name:          "magic",
		ObjectNames:   []string{"Magic tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 75,
		XPReward:      250,
		LogItem:       "magic_logs", // ItemIds.MAGIC_LOGS
		PetBase:       72321,
		ObjectIDs: []string{ // ObjectIds.MAGIC_TREE..MAGIC_TREE_18
			"magictree", "poh_big_tree7_4", "poh_small_tree7_5", "magic_tree_seedling",
			"magic_tree_1", "magic_tree_2", "magic_tree_3", "magic_tree_4",
			"magic_tree_5", "magic_tree_6", "magic_tree_7", "magic_tree_8",
			"magic_tree_9", "magic_tree_10", "magic_tree_11", "magic_tree_fullygrown_1",
			"magic_tree_fullygrown_2", "prif_magictree",
		},
		CutChance:    "magic",
		RespawnTicks: 199,
		Stump:        "magic_tree_stump_new", // ObjectIds.TREE_STUMP_37
		Multi:        true,
		DespawnTicks: 390,
	},
	{ // redwood (plugin.js:490-536)
		Name:          "redwood",
		ObjectNames:   []string{"Redwood tree"},
		Actions:       []string{"Cut"},
		RequiredLevel: 90,
		XPReward:      380,
		LogItem:       "redwood_logs", // ItemIds.REDWOOD_LOGS
		PetBase:       72321,
		ObjectIDs: []string{ // ObjectIds.REDWOOD_TREE..REDWOOD_TREE_121
			"redwoodtree_l", "redwoodtree_l_cut", "redwoodtree_r", "redwoodtree_r_cut",
			"redwood_tree_blank", "redwood_growing_0_blank_3x3", "redwood_growing_0_blank_3x2", "redwood_tree_seedling_0_5",
			"redwood_tree_1_0_5", "redwood_tree_2_0_5", "redwood_tree_3_0_5", "redwood_tree_4_0_5",
			"redwood_tree_5_0_5", "redwood_tree_6_0_1", "redwood_tree_6_0_2", "redwood_tree_6_0_3",
			"redwood_tree_6_0_4", "redwood_tree_6_0_5", "redwood_tree_6_0_6", "redwood_tree_6_0_7",
			"redwood_tree_6_0_8", "redwood_tree_6_0_9", "redwood_tree_7_0_1", "redwood_tree_7_0_2",
			"redwood_tree_7_0_3", "redwood_tree_7_0_4", "redwood_tree_7_0_5", "redwood_tree_7_0_6",
			"redwood_tree_7_0_7", "redwood_tree_7_0_8", "redwood_tree_7_0_9", "redwood_tree_7_1_1",
			"redwood_tree_7_1_2", "redwood_tree_7_1_3", "redwood_tree_7_1_4", "redwood_tree_7_1_5",
			"redwood_tree_7_1_6", "redwood_tree_7_1_7", "redwood_tree_7_1_8", "redwood_tree_7_1_9",
			"redwood_tree_8_0_1", "redwood_tree_8_0_2", "redwood_tree_8_0_3", "redwood_tree_8_0_4",
			"redwood_tree_8_0_5", "redwood_tree_8_0_6", "redwood_tree_8_0_7", "redwood_tree_8_0_8",
			"redwood_tree_8_0_9", "redwood_tree_8_1_1", "redwood_tree_8_1_2", "redwood_tree_8_1_3",
			"redwood_tree_8_1_4", "redwood_tree_8_1_5", "redwood_tree_8_1_6", "redwood_tree_8_1_7",
			"redwood_tree_8_1_8", "redwood_tree_8_1_9", "redwood_tree_9_0_1", "redwood_tree_9_0_2",
			"redwood_tree_9_0_3", "redwood_tree_9_0_4", "redwood_tree_9_0_5", "redwood_tree_9_0_6",
			"redwood_tree_9_0_7", "redwood_tree_9_0_8", "redwood_tree_9_0_9", "redwood_tree_9_1_1",
			"redwood_tree_9_1_2", "redwood_tree_9_1_3", "redwood_tree_9_1_4", "redwood_tree_9_1_5",
			"redwood_tree_9_1_6", "redwood_tree_9_1_7", "redwood_tree_9_1_8", "redwood_tree_9_1_9",
			"redwood_tree_fullygrown_0_1", "redwood_tree_fullygrown_0_2", "redwood_tree_fullygrown_0_3", "redwood_tree_fullygrown_0_4",
			"redwood_tree_fullygrown_0_5", "redwood_tree_fullygrown_0_6", "redwood_tree_fullygrown_0_7", "redwood_tree_fullygrown_0_8",
			"redwood_tree_fullygrown_0_9", "redwood_tree_fullygrown_1_1", "redwood_tree_fullygrown_1_2", "redwood_tree_fullygrown_1_3",
			"redwood_tree_fullygrown_1_4", "redwood_tree_fullygrown_1_5", "redwood_tree_fullygrown_1_6", "redwood_tree_fullygrown_1_7",
			"redwood_tree_fullygrown_1_8", "redwood_tree_fullygrown_1_9", "redwood_tree_fullygrown_2_1", "redwood_tree_fullygrown_0_1_claimxp",
			"redwood_tree_fullygrown_0_2_claimxp", "redwood_tree_fullygrown_0_3_claimxp", "redwood_tree_fullygrown_0_4_claimxp", "redwood_tree_fullygrown_0_5_claimxp",
			"redwood_tree_fullygrown_0_6_claimxp", "redwood_tree_fullygrown_0_7_claimxp", "redwood_tree_fullygrown_0_8_claimxp", "redwood_tree_fullygrown_0_9_claimxp",
			"redwood_tree_fullygrown_1_1_claimxp", "redwood_tree_fullygrown_1_2_claimxp", "redwood_tree_fullygrown_1_3_claimxp", "redwood_tree_fullygrown_1_4_claimxp",
			"redwood_tree_fullygrown_1_5_claimxp", "redwood_tree_fullygrown_1_6_claimxp", "redwood_tree_fullygrown_1_7_claimxp", "redwood_tree_fullygrown_1_8_claimxp",
			"redwood_tree_fullygrown_1_9_claimxp", "redwood_tree_fullygrown_2_1_claimxp", "redwood_tree_fullygrown_1_2_empty", "redwood_tree_fullygrown_1_4_empty",
			"redwood_tree_fullygrown_1_6_empty", "redwood_tree_fullygrown_1_8_empty", "tree_redwood_default01", "tree_redwood_default02",
			"tree_redwood_default03",
		},
		CutChance:    "redwood",
		RespawnTicks: 199,
		Multi:        true,
		DespawnTicks: 440,
		BirdNests:    boolPtr(false),
	},
	{ // hollow (plugin.js:537-548)
		Name:          "hollow",
		ObjectNames:   []string{"Hollow tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 45,
		XPReward:      82.5,
		LogItem:       "hollow_bark", // ItemIds.BARK
		PetBase:       214367,
		CutChance:     "hollow",
		RespawnTicks:  43,
		Multi:         true,
		DespawnTicks:  60,
	},
	{ // arctic pine (plugin.js:549-561)
		Name:          "arctic pine",
		ObjectNames:   []string{"Arctic pine tree"},
		Actions:       []string{"Cut down"},
		RequiredLevel: 54,
		XPReward:      40,
		LogItem:       "arctic_pine_log", // ItemIds.ARCTIC_PINE_LOGS
		PetBase:       145758,
		CutChance:     "maple",
		RespawnTicks:  14,
		Stump:         "arctic_pine_tree_stump", // ObjectIds.TREE_STUMP_49
		Multi:         true,
		DespawnTicks:  140,
	},
	{ // mature juniper (plugin.js:562-573)
		Name:          "mature juniper",
		ObjectNames:   []string{"Mature juniper tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 42,
		XPReward:      35,
		LogItem:       "juniper_logs", // ItemIds.JUNIPER_LOGS
		PetBase:       360000,
		CutChance:     "normal",
		RespawnTicks:  13,
		Multi:         true,
		DepleteChance: 16,
	},
	{ // jatoba (plugin.js:574-585)
		Name:          "jatoba",
		ObjectNames:   []string{"Jatoba tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 40,
		XPReward:      92,
		LogItem:       "jatoba_logs", // ItemIds.JATOBA_LOGS
		PetBase:       264336,
		CutChance:     "maple",
		RespawnTicks:  14,
		Multi:         true,
		Depletes:      boolPtr(false),
	},
	{ // blisterwood (plugin.js:586-602)
		Name:               "blisterwood",
		ObjectNames:        []string{"Blisterwood tree"},
		Actions:            []string{"Chop"},
		RequiredLevel:      62,
		XPReward:           76,
		LogItem:            "blisterwood_logs", // ItemIds.BLISTERWOOD_LOGS
		PetBase:            289286,
		CutChance:          "blisterwood",
		RespawnTicks:       0,
		Multi:              true,
		DepleteChance:      10,
		InterruptMessage:   "A small spider jumps at you from the log you just cut.",
		InterruptAnimation: "tbw_cleanup_player_surprise_stepback",
		InterruptSound:     3604,
	},
	{ // burnt tree (plugin.js:603-614)
		Name:            "burnt tree",
		ObjectNames:     []string{"Burnt tree"},
		Actions:         []string{"Chop down"},
		RequiredLevel:   1,
		XPReward:        25,
		LogItem:         "charcoal", // ItemIds.CHARCOAL
		CutChance:       "normal",
		RespawnTicks:    59,
		RespawnTicksMax: 98,
		Multi:           false,
	},
	{ // camphor (plugin.js:615-626)
		Name:          "camphor",
		ObjectNames:   []string{"Camphor tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 66,
		XPReward:      143.5,
		LogItem:       "camphor_logs", // ItemIds.CAMPHOR_LOGS
		PetBase:       145013,
		CutChance:     "camphor",
		RespawnTicks:  99,
		Multi:         true,
		DespawnTicks:  200,
	},
	{ // ironwood (plugin.js:627-638)
		Name:          "ironwood",
		ObjectNames:   []string{"Ironwood tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 80,
		XPReward:      175,
		LogItem:       "ironwood_logs", // ItemIds.IRONWOOD_LOGS
		PetBase:       72321,
		CutChance:     "ironwood",
		RespawnTicks:  199,
		Multi:         true,
		DespawnTicks:  400,
	},
	{ // rosewood (plugin.js:639-650)
		Name:          "rosewood",
		ObjectNames:   []string{"Rosewood tree"},
		Actions:       []string{"Chop down"},
		RequiredLevel: 92,
		XPReward:      212.5,
		LogItem:       "rosewood_logs", // ItemIds.ROSEWOOD_LOGS
		PetBase:       72321,
		CutChance:     "rosewood",
		RespawnTicks:  205,
		Multi:         true,
		DespawnTicks:  458,
	},
}

// BirdNestDef names the BIRD_NESTS ids by gameval name
// (Woodcutting.plugin.js:102-112).
type BirdNestDef struct {
	RedEggNest     string
	GreenEggNest   string
	BlueEggNest    string
	SeedNest       string
	LegacySeedNest string
	RingNest       string
	EmptyNest      string
}

// BirdNestDefs is the BIRD_NESTS table (plugin.js:102-112).
var BirdNestDefs = BirdNestDef{

	RedEggNest:     "bird_nest_egg_red",       // ItemIds.BIRD_NEST
	GreenEggNest:   "bird_nest_egg_green",     // ItemIds.BIRD_NEST_2
	BlueEggNest:    "bird_nest_egg_blue",      // ItemIds.BIRD_NEST_3
	SeedNest:       "bird_nest_seeds_jan2019", // ItemIds.BIRD_NEST_10
	LegacySeedNest: "bird_nest_seeds",         // ItemIds.BIRD_NEST_4
	RingNest:       "bird_nest_ring",          // ItemIds.BIRD_NEST_5
	EmptyNest:      "bird_nest_empty",         // ItemIds.BIRD_NEST_6
}

// NestSeedDef is one row of the NEST_SEEDS source table: the seed
// item by gameval name, the text the search line names it by, and
// its weight in the 1,011-slot table
// (Woodcutting.plugin.js:150-170; OSRS Wiki seed nest table).
// Magic, mahogany and redwood seeds are unrecorded and follow the
// yew and teak lines.
type NestSeedDef struct {
	Item   string
	Text   string
	Weight int
}

// NestSeedDefs is the NEST_SEEDS table (plugin.js:150-170), 19 seeds.
var NestSeedDefs = []NestSeedDef{

	{Item: "acorn", Text: "an acorn", Weight: 214},                              // ItemIds.ACORN
	{Item: "apple_tree_seed", Text: "an apple tree seed", Weight: 170},          // ItemIds.APPLE_TREE_SEED
	{Item: "willow_seed", Text: "a willow tree seed", Weight: 135},              // ItemIds.WILLOW_SEED
	{Item: "banana_tree_seed", Text: "a banana tree seed", Weight: 108},         // ItemIds.BANANA_TREE_SEED
	{Item: "orange_tree_seed", Text: "an orange tree seed", Weight: 85},         // ItemIds.ORANGE_TREE_SEED
	{Item: "curry_tree_seed", Text: "a curry tree seed", Weight: 68},            // ItemIds.CURRY_TREE_SEED
	{Item: "maple_seed", Text: "a maple tree seed", Weight: 54},                 // ItemIds.MAPLE_SEED
	{Item: "pineapple_tree_seed", Text: "a pineapple plant seed", Weight: 42},   // ItemIds.PINEAPPLE_SEED
	{Item: "papaya_tree_seed", Text: "a papaya tree seed", Weight: 34},          // ItemIds.PAPAYA_TREE_SEED
	{Item: "yew_seed", Text: "a yew tree seed", Weight: 27},                     // ItemIds.YEW_SEED
	{Item: "palm_tree_seed", Text: "a palm tree seed", Weight: 22},              // ItemIds.PALM_TREE_SEED
	{Item: "calquat_tree_seed", Text: "a calquat tree seed", Weight: 17},        // ItemIds.CALQUAT_TREE_SEED
	{Item: "spirit_tree_seed", Text: "a Spirit Tree seed", Weight: 11},          // ItemIds.SPIRIT_SEED
	{Item: "dragonfruit_tree_seed", Text: "a dragonfruit tree seed", Weight: 6}, // ItemIds.DRAGONFRUIT_TREE_SEED
	{Item: "magic_tree_seed", Text: "a magic tree seed", Weight: 5},             // ItemIds.MAGIC_SEED
	{Item: "teak_seed", Text: "a teak seed", Weight: 4},                         // ItemIds.TEAK_SEED
	{Item: "mahogany_seed", Text: "a mahogany seed", Weight: 4},                 // ItemIds.MAHOGANY_SEED
	{Item: "celastrus_tree_seed", Text: "a celastrus tree seed", Weight: 3},     // ItemIds.CELASTRUS_SEED
	{Item: "redwood_tree_seed", Text: "a redwood tree seed", Weight: 2},         // ItemIds.REDWOOD_TREE_SEED
}

// NestRingDef is one row of the NEST_RINGS source table: the ring
// item by gameval name, its name, and its weight
// (Woodcutting.plugin.js:173-179; OSRS Wiki ring nest table).
type NestRingDef struct {
	Item   string
	Name   string
	Weight int
}

// NestRingDefs is the NEST_RINGS table (plugin.js:173-179), 5 rings.
var NestRingDefs = []NestRingDef{

	{Item: "gold_ring", Name: "gold", Weight: 35},         // ItemIds.GOLD_RING
	{Item: "sapphire_ring", Name: "sapphire", Weight: 40}, // ItemIds.SAPPHIRE_RING
	{Item: "emerald_ring", Name: "emerald", Weight: 15},   // ItemIds.EMERALD_RING
	{Item: "ruby_ring", Name: "ruby", Weight: 9},          // ItemIds.RUBY_RING
	{Item: "diamond_ring", Name: "diamond", Weight: 1},    // ItemIds.DIAMOND_RING
}

// LumberjackPieceDef is one row of the LUMBERJACK_OUTFIT source
// table: the equipment slot, the accepted item gameval names
// (Lumberjack or Forestry piece), and the XP bonus
// (Woodcutting.plugin.js:130-135). Hat 0.4%, top 0.8%, legs 0.6%,
// boots 0.2%, plus 0.5% for all four; Forestry pieces count.
type LumberjackPieceDef struct {
	Slot  int
	Items []string
	Bonus float64
}

// LumberjackOutfitDefs is the LUMBERJACK_OUTFIT table (plugin.js:130-135).
var LumberjackOutfitDefs = []LumberjackPieceDef{

	{Slot: game.HeadSlot, Items: []string{"ramble_lumberjack_hat", "forestry_lumberjack_hat"}, Bonus: 0.004},     // ItemIds.LUMBERJACK_HAT / ItemIds.FORESTRY_HAT
	{Slot: game.BodySlot, Items: []string{"ramble_lumberjack_top", "forestry_lumberjack_top"}, Bonus: 0.008},     // ItemIds.LUMBERJACK_TOP / ItemIds.FORESTRY_TOP
	{Slot: game.LegSlot, Items: []string{"ramble_lumberjack_legs", "forestry_lumberjack_legs"}, Bonus: 0.006},    // ItemIds.LUMBERJACK_LEGS / ItemIds.FORESTRY_LEGS
	{Slot: game.FeetSlot, Items: []string{"ramble_lumberjack_boots", "forestry_lumberjack_boots"}, Bonus: 0.002}, // ItemIds.LUMBERJACK_BOOTS / ItemIds.FORESTRY_BOOTS
}

// NormalTreeStumpByModelDef maps a tree model's id to its stump
// loc's gameval name (Woodcutting.plugin.js:25-51). Older trees
// share models but not stumps, and some sit next to an unrelated
// stump id; key their stump by the tree's model. Willow
// willow_tree2/3/4 fall to willow_tree2or3or4_stump (rsprox
// recording 293).
var NormalTreeStumpByModelDef = map[int]string{

	1570:  "treestump2",                   // ObjectIds.TREE_STUMP_2
	1637:  "treestump2",                   // ObjectIds.TREE_STUMP_2
	1667:  "treestump2",                   // ObjectIds.TREE_STUMP_2
	1715:  "deadtree1_large_stump",        // ObjectIds.TREE_STUMP_7
	1716:  "deadtree2_stump",              // ObjectIds.TREE_STUMP_11
	1718:  "deadtree3_stump",              // ObjectIds.TREE_STUMP_14
	1719:  "deadtree6_stump",              // ObjectIds.TREE_STUMP_18
	1700:  "deadtree_burnt_stump",         // ObjectIds.TREE_STUMP_19
	23908: "deadtree2_stump_snow",         // ObjectIds.TREE_STUMP_50
	1611:  "evergreen_large_stump",        // ObjectIds.TREE_STUMP_15
	1614:  "evergreen_large_stump",        // ObjectIds.TREE_STUMP_15
	1688:  "evergreen_large_stump",        // ObjectIds.TREE_STUMP_15
	1681:  "yewtree_stump",                // ObjectIds.TREE_STUMP_17
	1682:  "yewtree_stump",                // ObjectIds.TREE_STUMP_17
	1683:  "yewtree_stump",                // ObjectIds.TREE_STUMP_17
	14747: "fai_varrock_dead_tree_stump",  // ObjectIds.DYING_TREE_STUMP
	3944:  "regicide_tree_stump",          // ObjectIds.TREE_STUMP_22
	4146:  "hollow_tree_stump",            // ObjectIds.TREE_STUMP_20
	4144:  "hollow_tree_stump_big",        // ObjectIds.TREE_STUMP_24
	16293: "arctic_pine_tree_stump",       // ObjectIds.TREE_STUMP_49
	33650: "fossil_deadtree_large1_stump", // ObjectIds.DEAD_TREE_STUMP
	33625: "fossil_deadtree_small1_stump", // ObjectIds.DEAD_TREE_STUMP_2
	5528:  "willow_tree2or3or4_stump",     // ObjectIds.TREE_STUMP_33
	5530:  "willow_tree2or3or4_stump",     // ObjectIds.TREE_STUMP_33
}

// DefaultTreeStump is the gameval name of the default stump loc
// (DEFAULT_TREE_STUMP_ID, ObjectIds.TREE_STUMP_2, plugin.js:22).
const DefaultTreeStump = "treestump2"

// ---- resolved tables ----

// Axe is a resolved AxeDef: the item id is filled in by Load.
type Axe struct {
	ID                 int
	Item               string
	RequiredLevel      int
	Tier               int
	AnimationID        int
	Speed              float64
	Felling            bool
	Standard           bool
	RequiredAgility    int
	RequiredFiremaking int
}

// Tree is a resolved TreeDef: the log, object and stump ids are
// filled in by Load. LogLine, CountsLogs and RespawnTimer default
// true; only the pest-control clone sets them false.
type Tree struct {
	Name               string
	ObjectNames        []string
	Actions            []string
	RequiredLevel      int
	XPReward           float64
	LogID              int
	LogItem            string
	PetBase            int
	ObjectIDs          []int
	CutChance          CutChart
	RoundedCutChance   bool
	RespawnTicks       int
	RespawnTicksMax    int
	StumpID            int
	Stump              string
	Multi              bool
	DespawnTicks       int
	DepleteChance      int
	Depletes           bool
	LogMessage         string
	BirdNests          bool
	LogLine            bool
	CountsLogs         bool
	RespawnTimer       bool
	DiaryXP            *DiaryBonus
	InterruptMessage   string
	InterruptAnimation string
	// InterruptAnimationID is the seq id of InterruptAnimation,
	// resolved at Load (0 when the tree has none).
	InterruptAnimationID int
	InterruptSound       int
}

// BirdNests is the resolved BIRD_NESTS table.
type BirdNests struct {
	RedEggNest     int
	GreenEggNest   int
	BlueEggNest    int
	SeedNest       int
	LegacySeedNest int
	RingNest       int
	EmptyNest      int
}

// NestSeed is a resolved NestSeedDef.
type NestSeed struct {
	ID     int
	Text   string
	Weight int
}

// NestRing is a resolved NestRingDef.
type NestRing struct {
	ID     int
	Name   string
	Weight int
}

// LumberjackPiece is a resolved LumberjackPieceDef.
type LumberjackPiece struct {
	Slot    int
	ItemIDs []int
	Bonus   float64
}

// Tables is the whole resolved Woodcutting table set, as Load
// builds it from the source tables above.
type Tables struct {
	// Axes is AXES worst to best; AxesBestFirst is its reverse.
	Axes          []Axe
	AxesBestFirst []Axe
	// CutChance is the chart map (numeric; no resolution needed).
	CutChance map[string]CutChart
	// Trees is TREES in plugin order.
	Trees []*Tree
	// TreesByName maps each tree object name to its tree
	// (TREES_BY_NAME, plugin.js:653). Name-only trees
	// (hollow, arctic pine, juniper, jatoba, blisterwood,
	// burnt, camphor, ironwood, rosewood) are reachable only
	// through this map.
	TreesByName map[string]*Tree
	// TreesByObjectID holds ONLY the pest-control tree
	// (TREES_BY_OBJECT_ID, plugin.js:665): the sole entry in
	// TREES addressable by object id outside the name map.
	TreesByObjectID map[int]*Tree
	// PestControlTree is the normal-tree clone Pest Control's
	// island trees chop as (plugin.js:656-665).
	PestControlTree *Tree
	// FarmedTrees is FARMED_TREES (plugin.js:671-676): the
	// farmed camphor, ironwood and rosewood (respawn 150
	// ticks), then the farmed redwood (despawn 0, 1-in-11
	// depletion, regrow 200 ticks).
	FarmedTrees []*Tree
	// TreeLogIDs is the distinct log ids of the bot-tracked
	// trees (those with object ids), in TREES order
	// (TREE_LOG_IDS, plugin.js:680-682).
	TreeLogIDs []int
	// NormalTreeStumpByModel maps tree model id to stump loc id
	// (plugin.js:25-51).
	NormalTreeStumpByModel map[int]int
	// DefaultTreeStumpID is the default stump loc id
	// (plugin.js:22).
	DefaultTreeStumpID int
	// StumpByTreeID is the per-tree stump memo (resolveStumpId's
	// stumpIdByTreeId, plugin.js:1172-1185): tree object id to
	// stump loc id, computed by resolveObjectDetails from the
	// cache object definitions at LoadFromCache time. It is nil
	// for fixture-loaded tables, whose resolveStumpID falls back
	// to the tree's own StumpID then the default.
	StumpByTreeID map[int]int
	// ObjectSize is the footprint of one tree loc id
	// (CacheDefinitions.getObject SizeX/SizeY), used by the
	// interaction-distance check (GameObject.
	// isWithinInteractionDistance). It is nil for
	// fixture-loaded tables, whose distance check falls back
	// to 1x1 — the pre-Phase-33 Chebyshev behaviour.
	ObjectSize map[int]ObjectSize
	// BirdNests is the resolved BIRD_NESTS table.
	BirdNests BirdNests
	// NestSeeds is the resolved NEST_SEEDS table.
	NestSeeds []NestSeed
	// NestRings is the resolved NEST_RINGS table.
	NestRings []NestRing
	// SeedNestIDs is SEED_NEST_IDS (plugin.js:114): both seed nests.
	SeedNestIDs map[int]bool
	// SearchableNestIDs is SEARCHABLE_NEST_IDS (plugin.js:116-123):
	// the three egg nests, both seed nests and the ring nest.
	SearchableNestIDs map[int]bool
	// LumberjackOutfit is the resolved LUMBERJACK_OUTFIT table.
	LumberjackOutfit []LumberjackPiece
	// LogsChoppedVarpID is the id of the "tracking_logs_chopped"
	// varp (plugin.js:66).
	LogsChoppedVarpID int
	// RakeID is the unnoted rake's id (ThickVines.Woodcutting.js:
	// resolveRakeId scans the cache items for an unnoted "rake";
	// the gameval "rake" resolves to the same id — 5341, the
	// scan's first unnoted "rake" in id order (probe, 2026-11-20:
	// 5341 unnoted, 5342 noted, 13880 a second unnoted rake) —
	// so Load resolves it by gameval name like every other item
	// id in this file).
	RakeID int
}

// IDResolver resolves the cache gameval names the source tables
// are written in. It is the seam Load resolves through, so tests
// can drive Load from a hand-built fixture instead of a live
// cache; production passes LoadFromCache's adapter over
// *cache.CacheDefinitions.
type IDResolver interface {
	ObjID(name string) (int, bool, error)
	LocID(name string) (int, bool, error)
	SeqID(name string) (int, bool, error)
	VarpID(name string) (int, bool, error)
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error)  { return r.defs.GetObjId(name) }
func (r cacheResolver) LocID(name string) (int, bool, error)  { return r.defs.GetLocId(name) }
func (r cacheResolver) SeqID(name string) (int, bool, error)  { return r.defs.GetSeqId(name) }
func (r cacheResolver) VarpID(name string) (int, bool, error) { return r.defs.GetVarpId(name) }

// ObjectSize is a loc's footprint (SizeX/SizeY) from its cache
// object definition.
type ObjectSize struct {
	SizeX, SizeY int
}

// ObjectDetail is the slice of a cache object definition the
// stump and footprint computations need (resolveStumpId /
// nextIdStump, plugin.js:1155-1185; GameObject.
// footprintDistance): the name, the action slots ("" = absent),
// the first model id, and the footprint.
type ObjectDetail struct {
	Name         string
	Actions      []string
	FirstModelID int
	HasModel     bool
	SizeX, SizeY int
}

// ObjectDefSource resolves cache object definitions by loc id.
// Production passes LoadFromCache's adapter over
// *cache.CacheDefinitions; tests pass a hand-built source.
type ObjectDefSource interface {
	ObjectDef(id int) (ObjectDetail, bool)
}

func (r cacheResolver) ObjectDef(id int) (ObjectDetail, bool) {
	def := r.defs.GetObject(id)
	if def == nil {
		return ObjectDetail{}, false
	}
	detail := ObjectDetail{
		Name:    def.Name,
		Actions: def.Actions,
		SizeX:   def.SizeX,
		SizeY:   def.SizeY,
	}
	if len(def.Models) > 0 && len(def.Models[0]) > 0 {
		detail.FirstModelID = def.Models[0][0]
		detail.HasModel = true
	}
	return detail, true
}

// nextIDStump is the treeId+1 leg (nextIdStump,
// plugin.js:1161-1170): the next loc id is a stump when it has
// no actions and is named like a stump, or — when allowSameName
// — when it carries the tree's own name with a different first
// model. It returns 0 when the leg does not apply.
func nextIDStump(treeID int, treeDef ObjectDetail, src ObjectDefSource, allowSameName bool) int {
	next, ok := src.ObjectDef(treeID + 1)
	if !ok {
		return 0
	}
	for _, action := range next.Actions {
		if action != "" {
			return 0
		}
	}
	isStump := strings.Contains(strings.ToLower(next.Name), "stump")
	isDepletedVariant := allowSameName && next.Name == treeDef.Name &&
		(next.HasModel != treeDef.HasModel || next.FirstModelID != treeDef.FirstModelID)
	if isStump || isDepletedVariant {
		return treeID + 1
	}
	return 0
}

// resolveObjectDetails fills StumpByTreeID and ObjectSize from
// the cache object definitions (resolveStumpId,
// plugin.js:1172-1185, run once per tree object id at load, the
// TS stumpIdByTreeId memo). Order per id: model map, then the
// treeId+1 legs (option-less stump, then same-name variant),
// then the tree's own StumpID, then the default stump. A tree
// object id with no definition keeps the pre-Phase-33 fallback
// (StumpID, then default) and a 1x1 footprint — the load never
// fails on a missing loc definition.
func (t *Tables) resolveObjectDetails(src ObjectDefSource) {
	if t == nil || src == nil {
		return
	}
	t.StumpByTreeID = make(map[int]int)
	t.ObjectSize = make(map[int]ObjectSize)
	trees := append(append([]*Tree(nil), t.Trees...), t.FarmedTrees...)
	for _, tree := range trees {
		if tree == nil {
			continue
		}
		for _, id := range tree.ObjectIDs {
			if _, done := t.StumpByTreeID[id]; done {
				continue
			}
			def, ok := src.ObjectDef(id)
			if ok {
				// TS clamps the footprint axes to at least 1
				// (footprintDistance, GameObject.ts:145-146);
				// a definition without sizes reads as 1x1.
				sizeX, sizeY := def.SizeX, def.SizeY
				if sizeX < 1 {
					sizeX = 1
				}
				if sizeY < 1 {
					sizeY = 1
				}
				t.ObjectSize[id] = ObjectSize{SizeX: sizeX, SizeY: sizeY}
			}
			stump := 0
			if ok && def.HasModel {
				if s, hit := t.NormalTreeStumpByModel[def.FirstModelID]; hit {
					stump = s
				}
			}
			if stump == 0 && ok {
				if s := nextIDStump(id, def, src, false); s != 0 {
					stump = s
				} else if s := nextIDStump(id, def, src, true); s != 0 {
					stump = s
				}
			}
			if stump == 0 {
				stump = tree.StumpID
			}
			if stump == 0 {
				stump = t.DefaultTreeStumpID
			}
			t.StumpByTreeID[id] = stump
		}
	}
}

// LoadFromCache resolves the Woodcutting tables against the real
// cache definitions (CacheDefinitions.idForName,
// internal/cache/facade.go:721, via the Get*Id accessors).
func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("woodcutting: nil cache definitions")
	}
	t, err := Load(cacheResolver{defs: defs})
	if err != nil {
		return nil, err
	}
	t.resolveObjectDetails(cacheResolver{defs: defs})
	return t, nil
}

// resolver collects unresolved names so Load fails closed with
// every missing name listed, instead of stopping at the first.
type resolver struct {
	r       IDResolver
	missing []string
}

func (rs *resolver) obj(name string) int {
	id, ok, err := rs.r.ObjID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
	}
	return id
}

func (rs *resolver) loc(name string) int {
	id, ok, err := rs.r.LocID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("loc %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("loc %q", name))
	}
	return id
}

func (rs *resolver) seq(name string) int {
	id, ok, err := rs.r.SeqID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q", name))
	}
	return id
}

func (rs *resolver) varp(name string) int {
	id, ok, err := rs.r.VarpID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("varp %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("varp %q", name))
	}
	return id
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("woodcutting: unresolved cache ids: %v", rs.missing)
}

// Load resolves every source table through r and returns the
// resolved Tables, or an error listing every gameval name the
// cache does not know. No id is invented: an unresolvable name
// fails the load.
func Load(r IDResolver) (*Tables, error) {
	if r == nil {
		return nil, errors.New("woodcutting: nil id resolver")
	}
	rs := &resolver{r: r}
	t := &Tables{CutChance: CutChance}

	for _, def := range AxeDefs {
		speed := def.Speed
		if speed == 0 {
			speed = AxeSpeedByTier[def.Tier]
		}
		t.Axes = append(t.Axes, Axe{
			ID: rs.obj(def.Item), Item: def.Item,
			RequiredLevel: def.RequiredLevel, Tier: def.Tier,
			AnimationID: def.AnimationID, Speed: speed,
			Felling: def.Felling, Standard: def.Standard,
			RequiredAgility:    def.RequiredAgility,
			RequiredFiremaking: def.RequiredFiremaking,
		})
	}
	t.AxesBestFirst = make([]Axe, len(t.Axes))
	for i, axe := range t.Axes {
		t.AxesBestFirst[len(t.Axes)-1-i] = axe
	}

	for _, def := range TreeDefs {
		tree := &Tree{
			Name: def.Name, ObjectNames: def.ObjectNames,
			Actions: def.Actions, RequiredLevel: def.RequiredLevel,
			XPReward: def.XPReward, LogItem: def.LogItem,
			PetBase:          def.PetBase,
			RoundedCutChance: def.RoundedCutChance,
			RespawnTicks:     def.RespawnTicks,
			RespawnTicksMax:  def.RespawnTicksMax,
			Stump:            def.Stump, Multi: def.Multi,
			DespawnTicks:  def.DespawnTicks,
			DepleteChance: def.DepleteChance,
			Depletes:      def.Depletes == nil || *def.Depletes,
			LogMessage:    def.LogMessage,
			BirdNests:     def.BirdNests == nil || *def.BirdNests,
			LogLine:       true, CountsLogs: true, RespawnTimer: true,
			DiaryXP:            def.DiaryXP,
			InterruptMessage:   def.InterruptMessage,
			InterruptAnimation: def.InterruptAnimation,
			InterruptSound:     def.InterruptSound,
		}
		tree.LogID = rs.obj(def.LogItem)
		for _, name := range def.ObjectIDs {
			tree.ObjectIDs = append(tree.ObjectIDs, rs.loc(name))
		}
		if def.CutChance != "" {
			chart, ok := CutChance[def.CutChance]
			if !ok {
				return nil, fmt.Errorf("woodcutting: tree %q names unknown cut-chance chart %q", def.Name, def.CutChance)
			}
			tree.CutChance = chart
		} else {
			tree.CutChance = def.CutChanceChart
		}
		if def.Stump != "" {
			tree.StumpID = rs.loc(def.Stump)
		}
		if def.InterruptAnimation != "" {
			tree.InterruptAnimationID = rs.seq(def.InterruptAnimation)
		}
		t.Trees = append(t.Trees, tree)
	}

	t.TreesByName = make(map[string]*Tree)
	for _, tree := range t.Trees {
		for _, name := range tree.ObjectNames {
			t.TreesByName[name] = tree
		}
	}

	// Pest-control clone (plugin.js:656-665): the normal tree with
	// its own object ids, saying nothing, showing no stump
	// countdown and leaving the logs-chopped count alone. Kept
	// out of TREES, so bots don't look for it.
	var normal *Tree
	for _, tree := range t.Trees {
		if tree.Name == "normal tree" {
			normal = tree
			break
		}
	}
	if normal == nil {
		return nil, errors.New("woodcutting: no \"normal tree\" entry for the pest-control clone")
	}
	pest := *normal
	pest.Name = "pest control tree"
	pest.ObjectNames = []string{}
	pest.ObjectIDs = []int{rs.loc("pest_tree"), rs.loc("pest_tree2")}
	pest.LogLine = false
	pest.CountsLogs = false
	pest.RespawnTimer = false
	t.PestControlTree = &pest
	t.TreesByObjectID = make(map[int]*Tree)
	for _, id := range pest.ObjectIDs {
		t.TreesByObjectID[id] = &pest
	}

	// Farmed trees (plugin.js:667-676): farmed sailing hardwoods
	// chop like the wild trees but regrow on farming's own patch
	// timer; the farmed redwood keeps pre-Forestry depletion.
	byName := make(map[string]*Tree, len(t.Trees))
	for _, tree := range t.Trees {
		byName[tree.Name] = tree
	}
	for _, name := range []string{"camphor", "ironwood", "rosewood"} {
		base, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("woodcutting: no %q entry for the farmed trees", name)
		}
		clone := *base
		clone.RespawnTicks = FarmedHardwoodRespawnTicks
		t.FarmedTrees = append(t.FarmedTrees, &clone)
	}
	redwood, ok := byName["redwood"]
	if !ok {
		return nil, errors.New("woodcutting: no \"redwood\" entry for the farmed trees")
	}
	farmedRedwood := *redwood
	farmedRedwood.DespawnTicks = 0
	farmedRedwood.DepleteChance = FarmedRedwoodDepletionRoll
	farmedRedwood.RespawnTicks = FarmedRedwoodRegrowTicks
	t.FarmedTrees = append(t.FarmedTrees, &farmedRedwood)

	seen := make(map[int]bool)
	for _, tree := range t.Trees {
		if len(tree.ObjectIDs) == 0 {
			continue
		}
		if !seen[tree.LogID] {
			seen[tree.LogID] = true
			t.TreeLogIDs = append(t.TreeLogIDs, tree.LogID)
		}
	}

	t.NormalTreeStumpByModel = make(map[int]int, len(NormalTreeStumpByModelDef))
	for model, name := range NormalTreeStumpByModelDef {
		t.NormalTreeStumpByModel[model] = rs.loc(name)
	}
	t.DefaultTreeStumpID = rs.loc(DefaultTreeStump)

	t.BirdNests = BirdNests{
		RedEggNest:     rs.obj(BirdNestDefs.RedEggNest),
		GreenEggNest:   rs.obj(BirdNestDefs.GreenEggNest),
		BlueEggNest:    rs.obj(BirdNestDefs.BlueEggNest),
		SeedNest:       rs.obj(BirdNestDefs.SeedNest),
		LegacySeedNest: rs.obj(BirdNestDefs.LegacySeedNest),
		RingNest:       rs.obj(BirdNestDefs.RingNest),
		EmptyNest:      rs.obj(BirdNestDefs.EmptyNest),
	}
	for _, def := range NestSeedDefs {
		t.NestSeeds = append(t.NestSeeds, NestSeed{ID: rs.obj(def.Item), Text: def.Text, Weight: def.Weight})
	}
	for _, def := range NestRingDefs {
		t.NestRings = append(t.NestRings, NestRing{ID: rs.obj(def.Item), Name: def.Name, Weight: def.Weight})
	}
	t.SeedNestIDs = map[int]bool{t.BirdNests.SeedNest: true, t.BirdNests.LegacySeedNest: true}
	t.SearchableNestIDs = map[int]bool{
		t.BirdNests.RedEggNest: true, t.BirdNests.GreenEggNest: true,
		t.BirdNests.BlueEggNest: true, t.BirdNests.SeedNest: true,
		t.BirdNests.LegacySeedNest: true, t.BirdNests.RingNest: true,
	}

	for _, def := range LumberjackOutfitDefs {
		piece := LumberjackPiece{Slot: def.Slot, Bonus: def.Bonus}
		for _, name := range def.Items {
			piece.ItemIDs = append(piece.ItemIDs, rs.obj(name))
		}
		t.LumberjackOutfit = append(t.LumberjackOutfit, piece)
	}

	t.LogsChoppedVarpID = rs.varp(LogsChoppedVarp)

	// The ThickVines rake (see RakeID above).
	t.RakeID = rs.obj("rake")

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

// TreeByName returns the tree choppable under an object name, or
// nil. This is the only lookup for the name-only trees.
func (t *Tables) TreeByName(name string) *Tree {
	if t == nil {
		return nil
	}
	return t.TreesByName[name]
}

// TreeByObjectID returns the tree for a pest-control object id,
// or nil. Only the pest-control tree is addressable by id.
func (t *Tables) TreeByObjectID(id int) *Tree {
	if t == nil {
		return nil
	}
	return t.TreesByObjectID[id]
}

// AxeByID returns the axe with an item id, or nil.
func (t *Tables) AxeByID(id int) *Axe {
	if t == nil {
		return nil
	}
	for i := range t.Axes {
		if t.Axes[i].ID == id {
			return &t.Axes[i]
		}
	}
	return nil
}

// CutChanceRow returns the [low, high] row for an axe tier,
// scaling a missing row off the normal chart exactly as
// cutChanceRange does (Woodcutting.plugin.js:748-756): a null
// row scales by the ratio of the normal chart's row for that
// tier to its bronze row.
func CutChanceRow(chart CutChart, tier int) CutRange {
	if row := chart[tier]; row.Has {
		return row
	}
	base := chart[0]
	normal := CutChance["normal"][tier]
	normalBase := CutChance["normal"][0]
	if !base.Has || !normal.Has || !normalBase.Has {
		return CutRange{}
	}
	roundDiv := func(num, den int) int {
		return int(math.Round(float64(num) / float64(den)))
	}
	return CutRange{
		Low:  roundDiv(base.Low*normal.Low, normalBase.Low),
		High: roundDiv(base.High*normal.High, normalBase.High),
		Has:  true,
	}
}
