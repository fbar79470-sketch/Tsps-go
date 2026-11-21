// Package fishing holds the Fishing skill tables: the verbatim
// data of server/plugins/skills/Fishing.plugin.js (upstream
// commit bfd81e3) plus fishing/Camdozaal.Fishing.js, resolved
// against the cache at load time.
//
// There is NO data file for Fishing (SKILLS_SURVEY.md §5, and
// the full read behind PHASE35_PLAN.md confirms it): every table
// below is transcribed from the plugin JS with file:line
// citations, the Phase 33 transcription precedent. Ids are
// SYMBOLIC in the plugin (ItemIds.*); the defs store the cache
// gameval name for each id (for example ItemIds.RAW_SHRIMPS is
// gameval "raw_shrimp", ItemIds.BIG_SWORDFISH is
// "poh_trophydrop_swordfish") and Load resolves every name
// through an IDResolver — LoadFromCache adapts
// *cache.CacheDefinitions, so no numeric id in this file is
// guessed: a name the cache does not know fails Load with the
// exact name listed. The gameval names were verified against
// the real rev-242 cache by probe (2026-11-20): all 82 distinct
// item names resolve, so this package needs NO fallback table —
// unlike Fletching, whose JSON names have no (correct) gameval
// for 176 items. Values that are numeric literals in the plugin
// itself (animation ids, ticks, XP, pet bases, NPC ids) are
// copied verbatim as literals and cited to the plugin line.
// NPC ids come from NpcIdentifiers.ts (numeric there too);
// every spot NPC id's display name was verified against the
// real rev-242 cache ("Fishing spot" / "Rod Fishing spot").
//
// Line numbers in comments cite Fishing.plugin.js unless
// another file is named. The tables are read-only after Load;
// the core loop (Worker B) and the sub-units (Worker D)
// consume them.
package fishing

import (
	"errors"
	"fmt"
	"math/rand"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// ---- constants (Fishing.plugin.js:23-60) ----

const (
	// FishingActionIntervalTicks is the ticks between resource
	// rolls for rods; spots the table leaves out keep it
	// (FISHING_ACTION_INTERVAL_TICKS, plugin.js:23).
	FishingActionIntervalTicks = 5
	// NetCageHarpoonIntervalTicks is the ticks between rolls
	// for net, cage and harpoon spots
	// (NET_CAGE_HARPOON_INTERVAL_TICKS, plugin.js:24).
	NetCageHarpoonIntervalTicks = 6
	// KarambwanIntervalTicks is the ticks between rolls for
	// karambwan vessels (KARAMBWAN_INTERVAL_TICKS,
	// plugin.js:25).
	KarambwanIntervalTicks = 4
	// NetCageFirstRollTicks is the delay before the first roll
	// for nets and cages (NET_CAGE_FIRST_ROLL_TICKS,
	// plugin.js:28).
	NetCageFirstRollTicks = 3
	// HarpoonRodFirstRollTicks is the delay before the first
	// roll for harpoons and rods
	// (HARPOON_ROD_FIRST_ROLL_TICKS, plugin.js:29).
	HarpoonRodFirstRollTicks = 4
	// FishingAnimationIntervalTicks is how often tools without
	// animateOnRoll restart their animation
	// (FISHING_ANIMATION_INTERVAL_TICKS, plugin.js:31).
	FishingAnimationIntervalTicks = 5
	// FullInventorySound is the sound on a full inventory
	// (FULL_INVENTORY_SOUND, plugin.js:44).
	FullInventorySound = 2277
	// ParkClueBase is the clue-bottle base the Fortis park
	// pond uses (PARK_CLUE_BASE, plugin.js:530: the catfish
	// base, per the Wiki).
	ParkClueBase = 152120
	// MinnowAttemptIntervalTicks is the minnow platform's
	// roll interval (ATTEMPT_INTERVAL_TICKS,
	// MinnowPlatform.Fishing.js:42). The minnow mechanics
	// themselves are deferred (PHASE35_PLAN.md §7); only the
	// Fish entry and this constant live here.
	MinnowAttemptIntervalTicks = 2
)

const (
	// AnimHarpoon is the harpoon animation (ANIM.HARPOON,
	// plugin.js:36-39).
	AnimHarpoon = 618
	// AnimCage is the lobster-pot animation (ANIM.CAGE).
	AnimCage = 619
	// AnimBigNet is the big-net animation (ANIM.BIG_NET).
	AnimBigNet = 620
	// AnimNet is the small-net animation (ANIM.NET).
	AnimNet = 621
	// AnimRod is the rod cast animation (ANIM.ROD).
	AnimRod = 622
	// AnimRodOnSpot is the rod wait-on-spot animation
	// (ANIM.ROD_ON_SPOT).
	AnimRodOnSpot = 623
	// AnimVessel is the karambwan-vessel animation
	// (ANIM.VESSEL).
	AnimVessel = 1193
	// AnimBarbarianRod is the barbarian-rod cast animation
	// (ANIM.BARBARIAN_ROD).
	AnimBarbarianRod = 9349
	// AnimBarbarianRodOnSpot is the barbarian-rod wait
	// animation (ANIM.BARBARIAN_ROD_ON_SPOT).
	AnimBarbarianRodOnSpot = 9350
	// AnimBarehandStart is the barehanded start animation
	// (plugin.js:360-363: 6703).
	AnimBarehandStart = 6703
	// AnimBarehandLoop is the barehanded ready-loop animation
	// (plugin.js:360-363: 6704).
	AnimBarehandLoop = 6704
	// AnimBarehandSharkCatch is the barehanded shark
	// finishing animation (plugin.js:360-363: 6706; 6705 is
	// frame-identical).
	AnimBarehandSharkCatch = 6706
	// AnimBarehandSwordfishCatch is the barehanded swordfish
	// finishing animation (plugin.js:370-373: 6707, inferred
	// as the shark's captured _1; no capture lands one).
	AnimBarehandSwordfishCatch = 6707
	// AnimBarehandTunaCatch is the barehanded tuna finishing
	// animation (plugin.js:366-369: 6711).
	AnimBarehandTunaCatch = 6711
)

const (
	// CastNetMessage is the small/big-net start line
	// (CAST_NET, plugin.js:47).
	CastNetMessage = "You cast out your net..."
	// CastLineMessage is the bait-rod start line (CAST_LINE,
	// plugin.js:48).
	CastLineMessage = "You cast out your line..."
	// CastAndAttemptMessage is the fly-rod / barbarian-rod /
	// pike-rod start line (CAST_AND_ATTEMPT, plugin.js:49).
	CastAndAttemptMessage = "You cast out your line...<br>You attempt to catch a fish."
	// StartHarpoonMessage is the harpoon start line
	// (START_HARPOON, plugin.js:50).
	StartHarpoonMessage = "You start harpooning fish."
	// StartBarehandMessage is the barehanded start line
	// (START_BAREHAND, plugin.js:52).
	StartBarehandMessage = "You start to lure the fish."
	// FullInventoryStatement is the full-inventory message
	// box (FULL_INVENTORY_STATEMENT, plugin.js:43).
	FullInventoryStatement = "You can't carry any more fish."
	// NoFishingBaitStatement is the missing-bait box
	// (NO_FISHING_BAIT, plugin.js:55).
	NoFishingBaitStatement = "You don't have any fishing bait."
	// NoFeathersStatement is the missing-feathers box
	// (NO_FEATHERS, plugin.js:56).
	NoFeathersStatement = "You don't have any feathers left."
	// NoBarbarianBaitStatement is the missing barbarian-bait
	// box (NO_BARBARIAN_BAIT, plugin.js:57).
	NoBarbarianBaitStatement = "You don't have any suitable bait left."
	// DefaultStartMessage is the start line for tools that
	// name none (FishingTool's startMessage default,
	// plugin.js:128; the dark-crab pot is the one tool that
	// relies on it).
	DefaultStartMessage = "You begin to fish.."
	// DefaultNoToolMessage is the chat line for tools that
	// name no refusal (FishingTool's fallback in
	// hasToolRequirements, plugin.js:792).
	DefaultNoToolMessage = "You don't have the right tool to fish there."
	// DefaultNoBaitMessage is the chat line for tools with
	// bait but no noBaitStatement (hasToolRequirements,
	// plugin.js:805).
	DefaultNoBaitMessage = "You do not have the required bait."
	// IceGlovesMessage is the worn-gloves refusal
	// (hasToolRequirements, plugin.js:798).
	IceGlovesMessage = "You need to wear ice gloves to handle these eels."
	// DefaultLevelMessage is the level-gate chat line for
	// tools with no levelStatement (hasToolRequirements,
	// plugin.js:774-775); %d is the tool's level.
	DefaultLevelMessage = "You need a Fishing level of at least %d to do this."
)

// HarpoonLevelMessage is the harpoon level-gate box
// (harpoonLevel, plugin.js:59).
func HarpoonLevelMessage(level int) string {
	return fmt.Sprintf("You need to be at least level %d Fishing to harpoon these fish.", level)
}

// BaitLevelMessage is the bait-rod level-gate box (baitLevel,
// plugin.js:60).
func BaitLevelMessage(level int) string {
	return fmt.Sprintf("You need at least level %d Fishing to bait these fish.", level)
}

// BarehandSharkLevelMessage is the barehanded-shark level-gate
// box (BAREHAND_SHARK levelStatement, plugin.js:364).
func BarehandSharkLevelMessage(level int) string {
	return fmt.Sprintf("You need to be at least level %d Fishing to catch sharks with your bare hands.", level)
}

// BarehandTunaLevelMessage is the barehanded-tuna level-gate
// box (BAREHAND_HARPOON levelStatement, plugin.js:373).
func BarehandTunaLevelMessage(level int) string {
	return fmt.Sprintf("You need to be at least level %d Fishing to catch tuna with your bare hands.", level)
}

// MinnowAmountByLevel is the minnows-per-catch table
// (MINNOWS_BY_LEVEL, MinnowPlatform.Fishing.js:46): 10 at 82,
// 11 at 85, 12 at 90, 13 at 95, 14 at 99. Entries are [from
// level, amount], highest first; the first entry whose level
// the player's MAX Fishing level reaches wins.
var MinnowAmountByLevel = [][2]int{{99, 14}, {95, 13}, {90, 12}, {85, 11}, {0, 10}}

// MinnowCatchChart is the minnow success chart [low, high]
// (CATCH_CHART, MinnowPlatform.Fishing.js:43) — a fitted
// estimate, not a measured curve; the file says so.
var MinnowCatchChart = [2]int{24, 148}

// FishingSpotNames are the NPC names fishing registers for
// (FISHING_SPOT_NAMES, plugin.js:532).
var FishingSpotNames = []string{"Fishing spot", "Rod Fishing spot"}

// FishingSpotOptions are the NPC options fishing registers
// for (FISHING_SPOT_OPTIONS, plugin.js:533).
var FishingSpotOptions = []string{
	"Small Net", "Net", "Big Net", "Bait", "Lure",
	"Cage", "Harpoon", "Use-rod", "Fish", "Cast",
}

// ---- spot NPC ids (numeric in NpcIdentifiers.ts) ----

// Every id below is transcribed from
// server/src/main/typescript/elvarg/util/NpcIdentifiers.ts
// (upstream commit bfd81e3) and its display name was verified
// against the real rev-242 cache by probe (2026-11-20): all
// are "Fishing spot" except the two Rod Fishing spots noted.
const (
	// NPCFishingSpot2 is FISHING_SPOT_2
	// (NpcIdentifiers.ts:1374).
	NPCFishingSpot2 = 1497
	// NPCFishingSpot3 is FISHING_SPOT_3
	// (NpcIdentifiers.ts:1375).
	NPCFishingSpot3 = 1498
	// NPCFishingSpot4 is FISHING_SPOT_4
	// (NpcIdentifiers.ts:1376).
	NPCFishingSpot4 = 1499
	// NPCFishingSpot5 is FISHING_SPOT_5
	// (NpcIdentifiers.ts:1377).
	NPCFishingSpot5 = 1500
	// NPCFishingSpot6 is FISHING_SPOT_6
	// (NpcIdentifiers.ts:6067).
	NPCFishingSpot6 = 6784
	// NPCFishingSpot30 is FISHING_SPOT_30
	// (NpcIdentifiers.ts:1407).
	NPCFishingSpot30 = 1530
	// NPCFishingSpot40 is FISHING_SPOT_40
	// (NpcIdentifiers.ts:2449).
	NPCFishingSpot40 = 2653
	// NPCFishingSpot41 is FISHING_SPOT_41
	// (NpcIdentifiers.ts:2450).
	NPCFishingSpot41 = 2654
	// NPCFishingSpot42 is FISHING_SPOT_42
	// (NpcIdentifiers.ts:2451).
	NPCFishingSpot42 = 2655
	// NPCFishingSpot43 is FISHING_SPOT_43
	// (NpcIdentifiers.ts:3071).
	NPCFishingSpot43 = 3317
	// NPCFishingSpot55 is FISHING_SPOT_55
	// (NpcIdentifiers.ts:4002).
	NPCFishingSpot55 = 4316
	// NPCFishingSpot58 is FISHING_SPOT_58
	// (NpcIdentifiers.ts:4358).
	NPCFishingSpot58 = 4710
	// NPCFishingSpot60 is FISHING_SPOT_60
	// (NpcIdentifiers.ts:4360).
	NPCFishingSpot60 = 4712
	// NPCFishingSpot61 is FISHING_SPOT_61
	// (NpcIdentifiers.ts:4361).
	NPCFishingSpot61 = 4713
	// NPCFishingSpot62 is FISHING_SPOT_62
	// (NpcIdentifiers.ts:4362).
	NPCFishingSpot62 = 4714
	// NPCFishingSpot63 is FISHING_SPOT_63
	// (NpcIdentifiers.ts:4564).
	NPCFishingSpot63 = 4928
	// NPCFishingSpot68 is FISHING_SPOT_68
	// (NpcIdentifiers.ts:5843).
	NPCFishingSpot68 = 6488
	// NPCFishingSpot87 is FISHING_SPOT_87
	// (NpcIdentifiers.ts:6895).
	NPCFishingSpot87 = 7730
	// NPCFishingSpot88 is FISHING_SPOT_88
	// (NpcIdentifiers.ts:6896).
	NPCFishingSpot88 = 7731
	// NPCFishingSpot89 is FISHING_SPOT_89
	// (NpcIdentifiers.ts:6897).
	NPCFishingSpot89 = 7732
	// NPCFishingSpot90 is FISHING_SPOT_90
	// (NpcIdentifiers.ts:6898).
	NPCFishingSpot90 = 7733
	// NPCFishingSpot92 is FISHING_SPOT_92
	// (NpcIdentifiers.ts:8892).
	NPCFishingSpot92 = 10653
	// NPCFishingSpot101 is FISHING_SPOT_101
	// (NpcIdentifiers.ts:11220).
	NPCFishingSpot101 = 13329
	// NPCFishingSpot130 is FISHING_SPOT_130
	// (NpcIdentifiers.ts:12821).
	NPCFishingSpot130 = 15384
	// NPCRodFishingSpot16 is ROD_FISHING_SPOT_16
	// (NpcIdentifiers.ts:6090); a "Rod Fishing spot".
	NPCRodFishingSpot16 = 6825
	// NPCRodFishingSpot20 is ROD_FISHING_SPOT_20
	// (NpcIdentifiers.ts:6843); a "Rod Fishing spot".
	NPCRodFishingSpot20 = 7676
	// NPCCamdozaalFishingSpot1 is the first Camdozaal spot
	// (Fishing.plugin.js:474: cache
	// camdozaal_fishingspot_1).
	NPCCamdozaalFishingSpot1 = 10686
	// NPCCamdozaalFishingSpot2 is the second Camdozaal spot
	// (Fishing.plugin.js:474: cache
	// camdozaal_fishingspot_2).
	NPCCamdozaalFishingSpot2 = 10687
	// NPCCamdozaalFishingSpot3 is the third Camdozaal spot
	// (Fishing.plugin.js:474: cache
	// camdozaal_fishingspot_3).
	NPCCamdozaalFishingSpot3 = 10688
)

// ---- resolved types ----

// SkillRequirement is one [skill, level] pair: a requirement
// to fish (Fish.Requires, FishingTool.Requires) or a level
// another skill must reach. Skill is a game.Skill* index.
type SkillRequirement struct {
	Skill int
	Level int
}

// SkillXP is one [skill, xp] pair: extra XP another skill
// gains per catch (Fish.ExtraXP).
type SkillXP struct {
	Skill int
	XP    int
}

// AmountFunc is how many of a fish one catch lands. It takes
// the player's current Fishing level and max Fishing level:
// karambwanji uses the current level (floor(level/5)+1,
// plugin.js:297-301), minnows the max level
// (MinnowPlatform.minnowsPerCatch,
// MinnowPlatform.Fishing.js:85-87). Nil means 1.
type AmountFunc func(currentLevel, maxLevel int) int

// MessageFunc is a fish's catch message for the landed
// amount. Nil means the default "You catch <caught>."
// (Fish constructor, plugin.js:62-91).
type MessageFunc func(amount int) string

// LevelStatementFunc is a tool's level-gate message box for
// its level (FishingTool.levelStatement, plugin.js:103-144).
// Nil means the default "You need a Fishing level of at
// least N to do this." line.
type LevelStatementFunc func(level int) string

// BigFish is a trophy fish that lands in place of its base
// fish (Fish.big, plugin.js:62-91): [item id, 1-in-n chance,
// name]. It keeps the base fish's XP and pet base (Wiki: Big
// bass); only the id, the caught text and the message change.
type BigFish struct {
	// ID is the trophy item's id.
	ID int
	// Item is the trophy item's gameval name.
	Item string
	// Chance is the 1-in-n chance (1000 bass, 2500
	// swordfish, 5000 shark).
	Chance int
	// Name is the fish name ("bass", "swordfish", "shark").
	Name string
}

// BigCaught is what "You catch ..." names for the trophy
// ("an enormous bass").
func (b *BigFish) BigCaught() string {
	if b == nil {
		return ""
	}
	return "an enormous " + b.Name
}

// BigMessage is the trophy catch line ("You catch an enormous
// bass!").
func (b *BigFish) BigMessage() string {
	if b == nil {
		return ""
	}
	return "You catch an enormous " + b.Name + "!"
}

// DiaryRolls is a fish's better chart once a diary tier is
// complete (DARK_CRAB.diaryRolls, plugin.js:289-292): the
// diary, the tier, and the replacement rolls.
type DiaryRolls struct {
	Diary string
	Tier  string
	Rolls [][2]int
}

// Fish is one resolved FISH entry (plugin.js:216-313): the
// catchable thing, its level, its success charts, its XP and
// its catch line.
type Fish struct {
	// Name is the TS FISH key ("SHRIMP", "TUNA_BAREHAND").
	Name string
	// ID is the item id.
	ID int
	// Item is the item's gameval name.
	Item string
	// Level is the Fishing level to catch it.
	Level int
	// Rolls are the [low, high] success-chart rows (out of
	// 256, at levels 1 and 99); most fish have one, mackerel
	// two. Park-pond finds have none — they land off the
	// park table, never off a chart.
	Rolls [][2]int
	// XP is the Fishing XP per catch, as a float64: minnow
	// 26.1 and the park finds' 7.5 are NOT integers.
	// Truncation to int happens at the XP grant site (the
	// core loop), not here — the same known structural gap
	// as woodcutting/fletching (a 26.1-XP catch grants 26).
	XP float64
	// Caught is what "You catch ..." names ("some raw
	// shrimps", "a shark").
	Caught string
	// PetBase is the Heron roll base, or 0 where the Wiki
	// gives no rate and no pet roll happens.
	PetBase int
	// Requires are the other-skill levels barbarian fishing
	// needs.
	Requires []SkillRequirement
	// ExtraXP is the XP other skills gain per catch.
	ExtraXP []SkillXP
	// Amount is how many one catch lands; nil means 1.
	Amount AmountFunc
	// Message is the catch message for the landed amount;
	// nil means the default "You catch <caught>.".
	Message MessageFunc
	// Junk marks a non-fish (big-net boots, park finds): no
	// pet roll and no "fishing:success" event.
	Junk bool
	// CatchAnimation is this fish's own finishing animation,
	// in place of the tool's (barehanded fishing), or 0.
	CatchAnimation int
	// Big is the trophy fish, or nil.
	Big *BigFish
	// DiaryRolls is the diary's better chart, or nil.
	DiaryRolls *DiaryRolls
}

// AmountFor returns how many of this fish one catch lands for
// a player at currentLevel (maxLevel for the minnow table).
func (f *Fish) AmountFor(currentLevel, maxLevel int) int {
	if f == nil || f.Amount == nil {
		return 1
	}
	return f.Amount(currentLevel, maxLevel)
}

// MessageFor returns the catch line for the landed amount.
func (f *Fish) MessageFor(amount int) string {
	if f == nil {
		return ""
	}
	if f.Message != nil {
		return f.Message(amount)
	}
	return "You catch " + f.Caught + "."
}

// ToolVariant is one item (or item set) that counts as a tool,
// best first (FishingTool.variants, plugin.js:103-144).
type ToolVariant struct {
	// IDs are the item ids; empty means bare hands (the
	// barehand variants' ids: []).
	IDs []int
	// Items are the items' gameval names.
	Items []string
	// Level is the Fishing level to use this variant.
	Level int
	// Bonus is the catch-chance bonus percentage (100 plain,
	// 120 dragon/infernal, 135 crystal/echo).
	Bonus int
	// Agility is the Agility level to use this variant, or 0
	// for the TS default of 1 (crystal: 50).
	Agility int
	// Crystal marks a charged crystal harpoon (one charge per
	// caught fish).
	Crystal bool
	// Infernal marks a charged infernal harpoon (may cook
	// what it catches).
	Infernal bool
	// Barehand marks a barehanded variant (no item at all).
	Barehand bool
}

// TableRoll is one pre-roll / tertiary table entry: the fish
// and its per-catch chance (rollTable, plugin.js:826-838).
type TableRoll struct {
	Fish   *Fish
	Chance float64
}

// TableWeight is one weighted table entry: the fish and its
// weight (rollTable, plugin.js:826-838).
type TableWeight struct {
	Fish   *Fish
	Weight int
}

// DropTable is a spot's drop table (FishingTool.table,
// plugin.js:103-144): a catch lands one item off it, not off
// the fish charts. Only PARK_NET has one.
type DropTable struct {
	// Rolls is the table's catch-chance chart, as [low,
	// high] rows out of 256 (the park's [[20, 20]]).
	Rolls [][2]int
	// PreRoll entries are tried first, each at its own
	// chance; the first hit wins.
	PreRoll []TableRoll
	// Weighted entries supply one pick when no pre-roll hit.
	Weighted []TableWeight
	// Tertiary entries roll on top of the catch, each at its
	// own chance.
	Tertiary []TableRoll
}

// FishingTool is one resolved TOOLS entry (plugin.js:321-459):
// a way of fishing, with its tool items, its fish cascade,
// its timings and its messages.
type FishingTool struct {
	// Name is the TS TOOLS key ("NET", "SHARK_HARPOON").
	Name string
	// ID is the tool's item id.
	ID int
	// Item is the tool item's gameval name.
	Item string
	// Level is the Fishing level to use the tool.
	Level int
	// AnimationID plays with the start line.
	AnimationID int
	// LoopAnimationID plays on every roll (or every few
	// ticks without animateOnRoll); the same as
	// AnimationID unless the tool names another.
	LoopAnimationID int
	// CatchAnimationID plays on a successful roll, or 0.
	CatchAnimationID int
	// Fish is the cascade, highest level first; the first
	// roll that succeeds is the catch, unless Multi.
	Fish []*Fish
	// Variants are the items that count as this tool, best
	// first.
	Variants []ToolVariant
	// Bait are the bait item ids, any one used up per catch.
	Bait []int
	// BaitItems are the bait items' gameval names.
	BaitItems []string
	// Worn are the item ids one of which must be equipped
	// (ice gloves / smiths gloves (i) for infernal eels).
	Worn []int
	// WornItems are the worn items' gameval names.
	WornItems []string
	// Quest is the quest that gates the tool ("Swan Song",
	// "Barbarian Training"), or "".
	Quest string
	// Multi means every roll stands on its own and one cast
	// can land several items (big net).
	Multi bool
	// Requires are the other-skill levels the tool needs
	// (barbarian rod: 15 Strength + Agility).
	Requires []SkillRequirement
	// Interval is the ticks between attempts, or
	// FishingActionIntervalTicks when the tool names none.
	Interval int
	// IntervalFunc gives the ticks between attempts when
	// they vary (pike: 6 one time in 5, else 5); nil means
	// the fixed Interval.
	IntervalFunc func() int
	// FirstRoll is the ticks from the start line to the first
	// roll; HasFirstRoll is false when the tool names none
	// (then the first roll comes at Interval).
	FirstRoll    int
	HasFirstRoll bool
	// AnimateOnRoll means the loop animation restarts on
	// every roll; false restarts it every
	// FishingAnimationIntervalTicks instead (minnow net).
	AnimateOnRoll bool
	// Unboostable means the level gate reads the max level,
	// not the current one (minnow net: 82).
	Unboostable bool
	// Spot is the spot module's name when one moves its own
	// spots and can take over a catch ("MinnowPlatform"),
	// or "" (deferred: no Go module exists yet).
	Spot string
	// Vessel marks a tool whose bait lives in the tool
	// itself, used up by every attempt (karambwan vessel).
	Vessel bool
	// Stationary means the spot never moves (park pond,
	// karambwan vessel, Camdozaal).
	Stationary bool
	// StartMessage is the cast line.
	StartMessage string
	// FollowMessage is the second start line, a tick after
	// the first, or "".
	FollowMessage string
	// MissMessage is the line on a missed attempt, or "".
	MissMessage string
	// NoToolStatement is the message box without the tool,
	// or "" (then NoToolMessage, or the default line).
	NoToolStatement string
	// NoBaitStatement is the message box without the bait,
	// or "" (then the default bait line).
	NoBaitStatement string
	// NoToolMessage is the chat line without the tool, or ""
	// (then the default line).
	NoToolMessage string
	// LevelStatement is the level-gate message box for the
	// tool's level; nil means the default line.
	LevelStatement LevelStatementFunc
	// Table is the drop table, or nil.
	Table *DropTable
	// Barehand marks a barehanded tool (no tool item).
	Barehand bool
	// ClueBase overrides the clue-bottle base for this tool
	// (Camdozaal big net: 100000), or 0.
	ClueBase int
}

// IntervalTicks returns the ticks to the next catch roll for
// this tool (rollInterval, plugin.js:161-163).
func (t *FishingTool) IntervalTicks() int {
	if t == nil {
		return FishingActionIntervalTicks
	}
	if t.IntervalFunc != nil {
		return t.IntervalFunc()
	}
	return t.Interval
}

// Tables is the whole resolved Fishing table set, as Load
// builds it from the source defs below.
type Tables struct {
	// Tools maps the TS TOOLS names (24) to their tools.
	Tools map[string]*FishingTool
	// Fish maps the TS FISH names (46) plus the Camdozaal
	// fish (4) to their fish.
	Fish map[string]*Fish
	// SpotToolsByOptions maps a spot's joined option list
	// ("Small Net/Bait", "Cage/Harpoon", ...) to the tool
	// per option (SPOT_TOOLS_BY_OPTIONS, plugin.js:461-471).
	SpotToolsByOptions map[string]map[string]*FishingTool
	// SpotToolsByNPC maps a spot NPC id to the tool per
	// option (SPOT_TOOLS_BY_NPC, plugin.js:479-521): 29
	// entries, Camdozaal included.
	SpotToolsByNPC map[int]map[string]*FishingTool
	// CamdozaalTools maps the Camdozaal option names
	// ("Small Net", "Big Net") to their tools
	// (CAMDOZAAL_TOOLS, plugin.js:475).
	CamdozaalTools map[string]*FishingTool
	// Harpoons is the HARPOONS variant list, best first
	// (plugin.js:176-199): the variants the HARPOON and
	// SHARK_HARPOON tools use, and the list hasHarpoon
	// checks (plugin.js:575-578).
	Harpoons []ToolVariant
	// FishingRods, FlyFishingRods, OilyFishingRods and
	// BarbarianRods are the rod variant sets (plugin.js:
	// 201-205); pearl rods fish exactly as their plain
	// versions.
	FishingRods     []ToolVariant
	FlyFishingRods  []ToolVariant
	OilyFishingRods []ToolVariant
	BarbarianRods   []ToolVariant
	// ClueBottleFish is the CLUE_BOTTLE_FISH set
	// (plugin.js:523-529), keyed by TS FISH name.
	ClueBottleFish map[string]bool
	// DistinctItemNames is the count of distinct gameval
	// item names the tables resolve.
	DistinctItemNames int
	// ResolvedItemNames is the count of distinct item ids
	// resolved (same as DistinctItemNames: every name
	// resolves through the cache, no fallback table).
	ResolvedItemNames int
	// GamevalItemNames is the count resolved through cache
	// gamevals (all of them).
	GamevalItemNames int
	// FallbackItemNames is the count resolved through a
	// fallback table: 0 — Fishing needs none (see the
	// package comment).
	FallbackItemNames int
}

// FishByName returns the fish for a TS FISH name, or nil.
func (t *Tables) FishByName(name string) *Fish {
	if t == nil {
		return nil
	}
	return t.Fish[name]
}

// ToolByName returns the tool for a TS TOOLS name, or nil.
func (t *Tables) ToolByName(name string) *FishingTool {
	if t == nil {
		return nil
	}
	return t.Tools[name]
}

// IsClueBottleFish reports whether a fish can yield a clue
// bottle (CLUE_BOTTLE_FISH, plugin.js:523-529).
func (t *Tables) IsClueBottleFish(f *Fish) bool {
	if t == nil || f == nil {
		return false
	}
	return t.ClueBottleFish[f.Name]
}

// SpotTools returns the tools for a spot NPC id, or nil when
// the spot fishes by its option list instead (getSpotTool,
// plugin.js:549-552).
func (t *Tables) SpotTools(npcID int) map[string]*FishingTool {
	if t == nil {
		return nil
	}
	return t.SpotToolsByNPC[npcID]
}

// ---- source defs (transcribed; resolved by Load) ----

// fishDef is one FISH entry before resolution: the item is
// named by its cache gameval name. Name is the TS FISH key.
type fishDef struct {
	Name           string
	Item           string
	Level          int
	Rolls          [][2]int
	XP             float64
	Caught         string
	PetBase        int
	Requires       []SkillRequirement
	ExtraXP        []SkillXP
	Amount         AmountFunc
	Message        MessageFunc
	Junk           bool
	CatchAnimation int
	Big            *bigDef
	DiaryRolls     *DiaryRolls
}

// bigDef is a Fish.big entry before resolution.
type bigDef struct {
	Item   string
	Chance int
	Name   string
}

// karambwanjiAmount is KARAMBWANJI's amount (plugin.js:
// 297-301): floor(current level / 5) + 1 a catch.
func karambwanjiAmount(currentLevel, _ int) int {
	return currentLevel/5 + 1
}

// minnowAmount is MINNOW's amount (MinnowPlatform.
// minnowsPerCatch, MinnowPlatform.Fishing.js:85-87): the
// MINNOWS_BY_LEVEL table keyed on the MAX level.
func minnowAmount(_, maxLevel int) int {
	for _, row := range MinnowAmountByLevel {
		if maxLevel >= row[0] {
			return row[1]
		}
	}
	return 10
}

// FishDefs is the FISH table (Fishing.plugin.js:216-313), 46
// entries in plugin order. Levels, XP and charts are each
// fish's Wiki page; pet bases are the Wiki's "Heron" rates.
// Gameval names are the cache's (probe, 2026-11-20, rev-242):
// "raw_shrimp" is ItemIds.RAW_SHRIMPS, "poh_trophydrop_*"
// the big fish, "oystershell" the oyster, "giant_frogspawn"
// the frog spawn, "mort_slimey_eel" the slimy eel,
// "snakeboss_eel" the sacred eel, "tbwt_raw_karambwan(ji)"
// the karambwan(ji), "broken_glass" the park glass,
// "big_net" the big fishing net,
// "varlamore_thieving_house_key" the house keys,
// "varlamore_key_half_2" the loop half,
// "brut_spawning_trout/salmon" the leaping trout/salmon,
// "brut_sturgeon" the leaping sturgeon.
var FishDefs = []fishDef{
	// SHRIMP (plugin.js:217)
	{Name: "SHRIMP", Item: "raw_shrimp", Level: 1, Rolls: [][2]int{{48, 256}}, XP: 10, Caught: "some raw shrimps", PetBase: 870330},
	// ANCHOVY (plugin.js:218)
	{Name: "ANCHOVY", Item: "raw_anchovies", Level: 15, Rolls: [][2]int{{24, 128}}, XP: 40, Caught: "some raw anchovies", PetBase: 870330},
	// SARDINE (plugin.js:219)
	{Name: "SARDINE", Item: "raw_sardine", Level: 5, Rolls: [][2]int{{32, 192}}, XP: 20, Caught: "a raw sardine", PetBase: 1056000},
	// HERRING (plugin.js:220)
	{Name: "HERRING", Item: "raw_herring", Level: 10, Rolls: [][2]int{{24, 128}}, XP: 30, Caught: "a raw herring", PetBase: 1056000},
	// TROUT (plugin.js:222): the fly rod's catch lines have no
	// full stop (rsprox captures, 8 recordings).
	{Name: "TROUT", Item: "raw_trout", Level: 20, Rolls: [][2]int{{32, 192}}, XP: 50, Caught: "a raw trout", PetBase: 923616,
		Message: func(_ int) string { return "You catch a raw trout" }},
	// PIKE (plugin.js:223)
	{Name: "PIKE", Item: "raw_pike", Level: 25, Rolls: [][2]int{{16, 96}}, XP: 60, Caught: "a pike", PetBase: 305792},
	// SALMON (plugin.js:224): no full stop, as TROUT.
	{Name: "SALMON", Item: "raw_salmon", Level: 30, Rolls: [][2]int{{16, 96}}, XP: 70, Caught: "a raw salmon", PetBase: 923616,
		Message: func(_ int) string { return "You catch a raw salmon" }},
	// TUNA (plugin.js:225)
	{Name: "TUNA", Item: "raw_tuna", Level: 35, Rolls: [][2]int{{8, 64}}, XP: 80, Caught: "a raw tuna", PetBase: 257770},
	// LOBSTER (plugin.js:226)
	{Name: "LOBSTER", Item: "raw_lobster", Level: 40, Rolls: [][2]int{{6, 95}}, XP: 90, Caught: "a lobster", PetBase: 116129},
	// SWORDFISH (plugin.js:227-228)
	{Name: "SWORDFISH", Item: "raw_swordfish", Level: 50, Rolls: [][2]int{{4, 48}}, XP: 100, Caught: "a raw swordfish", PetBase: 257770,
		Big: &bigDef{Item: "poh_trophydrop_swordfish", Chance: 2500, Name: "swordfish"}},
	// SHARK (plugin.js:229-232)
	{Name: "SHARK", Item: "raw_shark", Level: 76, Rolls: [][2]int{{3, 40}}, XP: 110, Caught: "a shark", PetBase: 82243,
		Message: func(_ int) string { return "You catch a shark!" },
		Big:     &bigDef{Item: "poh_trophydrop_shark", Chance: 5000, Name: "shark"}},
	// SHARK_BAREHAND (plugin.js:234-237): barehand shark gives
	// 11 Strength XP too (Wiki: Barbarian Training).
	{Name: "SHARK_BAREHAND", Item: "raw_shark", Level: 76, Rolls: [][2]int{{3, 40}}, XP: 110, Caught: "a shark", PetBase: 82243,
		Message: func(_ int) string { return "You catch a shark!" }, ExtraXP: []SkillXP{{Skill: game.SkillStrength, XP: 11}},
		Big: &bigDef{Item: "poh_trophydrop_shark", Chance: 5000, Name: "shark"}},
	// TUNA_BAREHAND (plugin.js:239-242): 55 Fishing and 35
	// Strength (80 XP, 8 Strength XP), on the harpoon charts;
	// catch animation 6711 (rsprox_transcripts/237/01149.txt).
	{Name: "TUNA_BAREHAND", Item: "raw_tuna", Level: 55, Rolls: [][2]int{{8, 64}}, XP: 80, Caught: "a raw tuna", PetBase: 257770,
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 35}}, ExtraXP: []SkillXP{{Skill: game.SkillStrength, XP: 8}},
		CatchAnimation: AnimBarehandTunaCatch},
	// SWORDFISH_BAREHAND (plugin.js:243-246): 70 Fishing and
	// 50 Strength (100 XP, 10 Strength XP); catch animation
	// 6707, as the shark's captured _1 — no capture lands a
	// swordfish.
	{Name: "SWORDFISH_BAREHAND", Item: "raw_swordfish", Level: 70, Rolls: [][2]int{{4, 48}}, XP: 100, Caught: "a raw swordfish", PetBase: 257770,
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 50}}, ExtraXP: []SkillXP{{Skill: game.SkillStrength, XP: 10}},
		CatchAnimation: AnimBarehandSwordfishCatch,
		Big:            &bigDef{Item: "poh_trophydrop_swordfish", Chance: 2500, Name: "swordfish"}},
	// Big-net junk (plugin.js:249-255): charts from the Wiki;
	// XP drops give 1 for boots, gloves and seaweed, 10 for
	// oysters and caskets (rsprox captures 1102, 1718).
	{Name: "LEATHER_BOOTS", Item: "leather_boots", Level: 16, Rolls: [][2]int{{10, 10}}, XP: 1, Caught: "some boots", Junk: true},
	{Name: "SEAWEED", Item: "seaweed", Level: 16, Rolls: [][2]int{{10, 10}}, XP: 1, Caught: "some seaweed", Junk: true},
	{Name: "LEATHER_GLOVES", Item: "leather_gloves", Level: 16, Rolls: [][2]int{{5, 5}}, XP: 1, Caught: "some gloves", Junk: true},
	{Name: "OYSTER", Item: "oystershell", Level: 16, Rolls: [][2]int{{3, 7}}, XP: 10, Caught: "an oyster shell", Junk: true},
	{Name: "CASKET", Item: "casket", Level: 16, Rolls: [][2]int{{1, 2}}, XP: 10, Caught: "a casket", Junk: true},
	// MACKEREL (plugin.js:256): two charts, so one cast can
	// net two.
	{Name: "MACKEREL", Item: "raw_mackerel", Level: 16, Rolls: [][2]int{{5, 65}, {10, 10}}, XP: 20, Caught: "a raw mackerel", PetBase: 1147827},
	// COD (plugin.js:257)
	{Name: "COD", Item: "raw_cod", Level: 23, Rolls: [][2]int{{4, 55}}, XP: 45, Caught: "a raw cod", PetBase: 1147827},
	// BASS (plugin.js:258-259)
	{Name: "BASS", Item: "raw_bass", Level: 46, Rolls: [][2]int{{3, 40}}, XP: 100, Caught: "a raw bass", PetBase: 1147827,
		Big: &bigDef{Item: "poh_trophydrop_bass", Chance: 1000, Name: "bass"}},
	// MONKFISH (plugin.js:260)
	{Name: "MONKFISH", Item: "raw_monkfish", Level: 62, Rolls: [][2]int{{48, 90}}, XP: 120, Caught: "a raw monkfish", PetBase: 138583},
	// FROG_SPAWN (plugin.js:262)
	{Name: "FROG_SPAWN", Item: "giant_frogspawn", Level: 33, Rolls: [][2]int{{16, 96}}, XP: 75, Caught: "some frog spawn"},
	// CAVE_EEL (plugin.js:263)
	{Name: "CAVE_EEL", Item: "raw_cave_eel", Level: 38, Rolls: [][2]int{{10, 80}}, XP: 80, Caught: "a raw cave eel", PetBase: 257770},
	// SLIMY_EEL (plugin.js:265): 80 XP in the caves.
	{Name: "SLIMY_EEL", Item: "mort_slimey_eel", Level: 28, Rolls: [][2]int{{10, 80}}, XP: 80, Caught: "a raw slimy eel"},
	// SWAMP_SLIMY_EEL (plugin.js:266): 65 XP in Mort Myre.
	{Name: "SWAMP_SLIMY_EEL", Item: "mort_slimey_eel", Level: 28, Rolls: [][2]int{{10, 80}}, XP: 65, Caught: "a raw slimy eel"},
	// LAVA_EEL (plugin.js:267)
	{Name: "LAVA_EEL", Item: "raw_lava_eel", Level: 53, Rolls: [][2]int{{16, 96}}, XP: 60, Caught: "a raw lava eel"},
	// INFERNAL_EEL (plugin.js:268-270)
	{Name: "INFERNAL_EEL", Item: "infernal_eel", Level: 80, Rolls: [][2]int{{30, 93}}, XP: 95, Caught: "an infernal eel", PetBase: 165000,
		Message: func(_ int) string {
			return "You catch an infernal eel. It hardens as you handle it with your ice gloves."
		}},
	// SACRED_EEL (plugin.js:271)
	{Name: "SACRED_EEL", Item: "snakeboss_eel", Level: 87, Rolls: [][2]int{{0, 60}}, XP: 105, Caught: "a sacred eel", PetBase: 99000},
	// ANGLERFISH (plugin.js:272)
	{Name: "ANGLERFISH", Item: "raw_anglerfish", Level: 82, Rolls: [][2]int{{3, 36}}, XP: 120, Caught: "a raw anglerfish", PetBase: 78649},
	// DARK_CRAB (plugin.js:273-277): the standard roll is
	// 3-40, but with the elite Wilderness Diary "you roll
	// 13-55", and "if that fails, you still do the standard
	// roll" (Mod Ash).
	{Name: "DARK_CRAB", Item: "raw_dark_crab", Level: 85, Rolls: [][2]int{{3, 40}}, XP: 130, Caught: "a raw dark crab", PetBase: 149434,
		DiaryRolls: &DiaryRolls{Diary: "wilderness", Tier: "elite", Rolls: [][2]int{{13, 55}, {3, 40}}}},
	// KARAMBWANJI (plugin.js:279-283): floor(level/5)+1 a
	// catch, 5 XP and one pet roll per catch.
	{Name: "KARAMBWANJI", Item: "tbwt_raw_karambwanji", Level: 5, Rolls: [][2]int{{100, 250}}, XP: 5, Caught: "some Karambwanji", PetBase: 443697,
		Amount:  karambwanjiAmount,
		Message: func(amount int) string { return fmt.Sprintf("You catch %d Karambwanji.", amount) }},
	// KARAMBWAN (plugin.js:284-287)
	{Name: "KARAMBWAN", Item: "tbwt_raw_karambwan", Level: 65, Rolls: [][2]int{{5, 160}}, XP: 50, Caught: "a Karambwan", PetBase: 170874,
		Message: func(_ int) string { return "You catch a Karambwan!" }},
	// MINNOW (plugin.js:289-292): 26.1 XP, 10-14 a catch by
	// max level; the chart [24, 148] is the fitted estimate
	// (MinnowPlatform.Fishing.js:43). The platform mechanics
	// are deferred (PHASE35_PLAN.md §7); the entry lives here.
	{Name: "MINNOW", Item: "minnow", Level: 82, Rolls: [][2]int{{24, 148}}, XP: 26.1, Caught: "some minnows", PetBase: 977778,
		Amount: minnowAmount},
	// Park-pond finds (plugin.js:294-309): Civitas illa
	// Fortis's park pond, 7.5 XP a catch whatever it lands,
	// nothing a fish ("You find <what>."). The loop half is
	// the moon key's tertiary, on top of the catch, at 0 XP.
	{Name: "PARK_BROKEN_GLASS", Item: "broken_glass", Level: 1, XP: 7.5, Caught: "some broken glass", Junk: true,
		Message: func(_ int) string { return "You find some broken glass." }},
	{Name: "PARK_OLD_BOOT", Item: "old_boot", Level: 1, XP: 7.5, Caught: "an old boot", Junk: true,
		Message: func(_ int) string { return "You find an old boot." }},
	{Name: "PARK_BIG_FISHING_NET", Item: "big_net", Level: 1, XP: 7.5, Caught: "a big fishing net", Junk: true,
		Message: func(_ int) string { return "You find a big fishing net." }},
	{Name: "PARK_CASKET", Item: "casket", Level: 1, XP: 7.5, Caught: "a casket", Junk: true,
		Message: func(_ int) string { return "You find a casket." }},
	{Name: "PARK_HOUSE_KEYS", Item: "varlamore_thieving_house_key", Level: 1, XP: 7.5, Caught: "some house keys", Junk: true,
		Message: func(_ int) string { return "You find some house keys." }},
	{Name: "PARK_JADE", Item: "jade", Level: 1, XP: 7.5, Caught: "a jade", Junk: true,
		Message: func(_ int) string { return "You find a jade." }},
	{Name: "PARK_RED_TOPAZ", Item: "red_topaz", Level: 1, XP: 7.5, Caught: "a red topaz", Junk: true,
		Message: func(_ int) string { return "You find a red topaz." }},
	{Name: "PARK_LOOP_HALF", Item: "varlamore_key_half_2", Level: 1, XP: 0, Caught: "a loop half of a key", Junk: true,
		Message: func(_ int) string { return "You find a loop half of a key." }},
	// Leaping fish (plugin.js:311-313): barbarian fishing
	// needs and trains Strength/Agility (barbarian(),
	// plugin.js:207-211).
	{Name: "LEAPING_TROUT", Item: "brut_spawning_trout", Level: 48, Rolls: [][2]int{{32, 192}}, XP: 50, Caught: "a leaping trout", PetBase: 1280862,
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 15}, {Skill: game.SkillAgility, Level: 15}},
		ExtraXP:  []SkillXP{{Skill: game.SkillStrength, XP: 5}, {Skill: game.SkillAgility, XP: 5}}},
	{Name: "LEAPING_SALMON", Item: "brut_spawning_salmon", Level: 58, Rolls: [][2]int{{16, 96}}, XP: 70, Caught: "a leaping salmon", PetBase: 1280862,
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 30}, {Skill: game.SkillAgility, Level: 30}},
		ExtraXP:  []SkillXP{{Skill: game.SkillStrength, XP: 6}, {Skill: game.SkillAgility, XP: 6}}},
	{Name: "LEAPING_STURGEON", Item: "brut_sturgeon", Level: 70, Rolls: [][2]int{{8, 64}}, XP: 80, Caught: "a leaping sturgeon", PetBase: 1280862,
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 45}, {Skill: game.SkillAgility, Level: 45}},
		ExtraXP:  []SkillXP{{Skill: game.SkillStrength, XP: 7}, {Skill: game.SkillAgility, XP: 7}}},
}

// ---- tool variants ----

// toolVariantDef is one FishingTool.variants entry before
// resolution: items by gameval name. Items empty means bare
// hands.
type toolVariantDef struct {
	Items    []string
	Level    int
	Bonus    int
	Agility  int
	Crystal  bool
	Infernal bool
	Barehand bool
}

// HarpoonVariantDefs is the HARPOONS list
// (plugin.js:176-199), best first: dragon-tier harpoons catch
// 20% more and a charged crystal harpoon 35% more (bonus is a
// percentage of the chart's low/high). The inactive crystal
// harpoon works as a dragon harpoon, and a barb-tail is a
// plain harpoon you can wield. Gamevals (probe, 2026-11-20):
// "league_trailblazer_harpoon" is the echo harpoon,
// "gauntlet_harpoon" the crystal harpoon (3),
// "trailblazer_harpoon(_empty)" the infernal/dragon (or)
// harpoons, "trailblazer_reloaded_harpoon(_empty)" the (or)
// reloaded ones, "hunting_barbed_harpoon" the barb-tail.
var HarpoonVariantDefs = []toolVariantDef{
	// Echo harpoon (Wiki): a crystal harpoon with no
	// requirement and no charges.
	{Items: []string{"league_trailblazer_harpoon"}, Level: 1, Bonus: 135},
	// Charged crystal harpoons consume one charge per fish.
	{Items: []string{"crystal_harpoon", "gauntlet_harpoon"}, Level: 71, Bonus: 135, Agility: 50, Crystal: true},
	// Charged infernal harpoons can cook what they catch.
	{Items: []string{"infernal_harpoon", "trailblazer_harpoon", "trailblazer_reloaded_harpoon"}, Level: 75, Bonus: 120, Infernal: true},
	{Items: []string{"infernal_harpoon_empty", "trailblazer_harpoon_empty", "trailblazer_reloaded_harpoon_empty"}, Level: 75, Bonus: 120},
	{Items: []string{"crystal_harpoon_inactive"}, Level: 71, Bonus: 120, Agility: 50},
	{Items: []string{"dragon_harpoon", "trailblazer_harpoon_no_infernal", "trailblazer_reloaded_harpoon_no_infernal"}, Level: 61, Bonus: 120},
	{Items: []string{"harpoon", "hunting_barbed_harpoon"}, Level: 1, Bonus: 100},
}

// rodVariantDefs builds one rod variant set (rod(),
// plugin.js:201): pearl rods fish exactly as their plain
// versions.
func rodVariantDefs(items ...string) []toolVariantDef {
	return []toolVariantDef{{Items: items, Level: 1, Bonus: 100}}
}

// FishingRodVariantDefs is FISHING_RODS (plugin.js:202).
var FishingRodVariantDefs = rodVariantDefs("fishing_rod", "fishingrod_pearl")

// FlyFishingRodVariantDefs is FLY_FISHING_RODS
// (plugin.js:203).
var FlyFishingRodVariantDefs = rodVariantDefs("fly_fishing_rod", "fishingrod_pearl_fly")

// OilyFishingRodVariantDefs is OILY_FISHING_RODS
// (plugin.js:204).
var OilyFishingRodVariantDefs = rodVariantDefs("oily_fishing_rod", "fishingrod_pearl_oily")

// BarbarianRodVariantDefs is BARBARIAN_RODS (plugin.js:205).
var BarbarianRodVariantDefs = rodVariantDefs("brut_fishing_rod", "fishingrod_pearl_brut")

// PikeInterval is the pike rod's interval (pikeInterval,
// plugin.js:165-167): 6 ticks one time in 5, else 5.
func PikeInterval() int {
	if rand.Float64() < 0.2 {
		return NetCageHarpoonIntervalTicks
	}
	return FishingActionIntervalTicks
}

func boolPtr(b bool) *bool { return &b }

// ---- tool defs ----

// dropTableDef is a FishingTool.table entry before
// resolution: fish by TS FISH name.
type dropTableDef struct {
	Rolls    [][2]int
	PreRoll  []tableRollDef
	Weighted []tableWeightDef
	Tertiary []tableRollDef
}

type tableRollDef struct {
	Fish   string
	Chance float64
}

type tableWeightDef struct {
	Fish   string
	Weight int
}

// toolDef is one TOOLS entry before resolution. Zero values
// follow the FishingTool constructor defaults
// (plugin.js:103-144): Interval 0 means
// FishingActionIntervalTicks (5); LoopAnimation 0 means
// AnimationID; StartMessage "" means DefaultStartMessage;
// AnimateOnRoll nil means true; FirstRoll without
// HasFirstRoll means none (first roll at Interval); Variants
// nil means the single default variant [{ids: [tool id],
// level 1, bonus 100}].
type toolDef struct {
	Name            string
	Item            string
	Level           int
	Animation       int
	LoopAnimation   int
	CatchAnimation  int
	Fish            []string
	Variants        []toolVariantDef
	Bait            []string
	Worn            []string
	Quest           string
	Multi           bool
	Requires        []SkillRequirement
	Interval        int
	IntervalFunc    func() int
	FirstRoll       int
	HasFirstRoll    bool
	AnimateOnRoll   *bool
	Unboostable     bool
	Spot            string
	Vessel          bool
	Stationary      bool
	StartMessage    string
	FollowMessage   string
	MissMessage     string
	NoToolStatement string
	NoBaitStatement string
	NoToolMessage   string
	LevelStatement  LevelStatementFunc
	Table           *dropTableDef
	Barehand        bool
	ClueBase        int
}

// ToolDefs is the TOOLS table (Fishing.plugin.js:321-459), 24
// entries in plugin order. The NET / CAGE / HARPOON / ROD /
// BAIT_ROD spreads (plugin.js:315-319) are expanded inline:
// NET = {firstRoll 3, start CAST_NET}; CAGE = {firstRoll 3};
// HARPOON = {firstRoll 4, start START_HARPOON, level
// harpoonLevel}; ROD = {anim 622, loop 623, firstRoll 4};
// BAIT_ROD = ROD + {start CAST_LINE, noBait NO_FISHING_BAIT,
// level baitLevel}.
var ToolDefs = []toolDef{
	// NET (plugin.js:322-325)
	{Name: "NET", Item: "net", Level: 1, Animation: AnimNet, Fish: []string{"ANCHOVY", "SHRIMP"},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true, StartMessage: CastNetMessage},
	// FISHING_ROD (plugin.js:327-330): sea bait says its two
	// lines a tick apart, casting again with the second
	// (rsprox captures 75, 1098, 1710).
	{Name: "FISHING_ROD", Item: "fishing_rod", Level: 5, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"HERRING", "SARDINE"}, Variants: FishingRodVariantDefs, Bait: []string{"fishing_bait"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		FollowMessage: "You attempt to catch a fish.", NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// PIKE_ROD (plugin.js:331-334)
	{Name: "PIKE_ROD", Item: "fishing_rod", Level: 25, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"PIKE"}, Variants: FishingRodVariantDefs, Bait: []string{"fishing_bait"},
		IntervalFunc: PikeInterval, FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true,
		StartMessage: CastAndAttemptMessage, NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// FLY_FISHING_ROD (plugin.js:335-338)
	{Name: "FLY_FISHING_ROD", Item: "fly_fishing_rod", Level: 20, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"SALMON", "TROUT"}, Variants: FlyFishingRodVariantDefs, Bait: []string{"feather"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastAndAttemptMessage,
		NoBaitStatement: NoFeathersStatement},
	// HARPOON (plugin.js:339-342)
	{Name: "HARPOON", Item: "harpoon", Level: 35, Animation: AnimHarpoon, Fish: []string{"SWORDFISH", "TUNA"},
		Variants: HarpoonVariantDefs, Interval: NetCageHarpoonIntervalTicks,
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: StartHarpoonMessage, LevelStatement: HarpoonLevelMessage},
	// SHARK_HARPOON (plugin.js:343-346)
	{Name: "SHARK_HARPOON", Item: "harpoon", Level: 76, Animation: AnimHarpoon, Fish: []string{"SHARK"},
		Variants: HarpoonVariantDefs, Interval: NetCageHarpoonIntervalTicks,
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: StartHarpoonMessage, LevelStatement: HarpoonLevelMessage},
	// BAREHAND_SHARK (plugin.js:350-357): 96 Fishing and 76
	// Strength, 110 Fishing XP and 11 Strength XP; barehand
	// speed and catch rate equal a harpoon's; the empty ids
	// variant means "no tool, bare hands".
	{Name: "BAREHAND_SHARK", Item: "harpoon", Level: 96, Animation: AnimBarehandStart, LoopAnimation: AnimBarehandLoop,
		CatchAnimation: AnimBarehandSharkCatch, Fish: []string{"SHARK_BAREHAND"},
		Variants: []toolVariantDef{{Items: []string{}, Level: 96, Bonus: 100, Barehand: true}},
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 76}},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true,
		Barehand: true, StartMessage: StartBarehandMessage, LevelStatement: BarehandSharkLevelMessage},
	// BAREHAND_HARPOON (plugin.js:359-363): the tuna and
	// swordfish spots' Harpoon without a harpoon.
	{Name: "BAREHAND_HARPOON", Item: "harpoon", Level: 55, Animation: AnimBarehandStart, LoopAnimation: AnimBarehandLoop,
		Fish:     []string{"SWORDFISH_BAREHAND", "TUNA_BAREHAND"},
		Variants: []toolVariantDef{{Items: []string{}, Level: 55, Bonus: 100, Barehand: true}},
		Requires: []SkillRequirement{{Skill: game.SkillStrength, Level: 35}},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true,
		Barehand: true, StartMessage: StartBarehandMessage, LevelStatement: BarehandTunaLevelMessage},
	// LOBSTER_POT (plugin.js:364-367)
	{Name: "LOBSTER_POT", Item: "lobster_pot", Level: 40, Animation: AnimCage, Fish: []string{"LOBSTER"},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true,
		StartMessage: "You attempt to catch a lobster."},
	// BIG_NET (plugin.js:370-374): every roll stands on its
	// own, in Mod Ash's order, with the junk first as the
	// capture's lines are.
	{Name: "BIG_NET", Item: "big_net", Level: 16, Animation: AnimBigNet,
		Fish:  []string{"LEATHER_BOOTS", "SEAWEED", "LEATHER_GLOVES", "OYSTER", "CASKET", "MACKEREL", "COD", "BASS"},
		Multi: true, Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true,
		StartMessage: CastNetMessage},
	// MONKFISH_NET (plugin.js:375-377)
	{Name: "MONKFISH_NET", Item: "net", Level: 62, Animation: AnimNet, Fish: []string{"MONKFISH"},
		Quest: "Swan Song", FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true, StartMessage: CastNetMessage},
	// FROG_SPAWN_NET (plugin.js:378)
	{Name: "FROG_SPAWN_NET", Item: "net", Level: 33, Animation: AnimNet, Fish: []string{"FROG_SPAWN"},
		FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true, StartMessage: CastNetMessage},
	// CAVE_EEL_ROD (plugin.js:379-382)
	{Name: "CAVE_EEL_ROD", Item: "fishing_rod", Level: 28, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"CAVE_EEL", "SLIMY_EEL"}, Variants: FishingRodVariantDefs, Bait: []string{"fishing_bait"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// SWAMP_EEL_ROD (plugin.js:383-386)
	{Name: "SWAMP_EEL_ROD", Item: "fishing_rod", Level: 28, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"SWAMP_SLIMY_EEL"}, Variants: FishingRodVariantDefs, Bait: []string{"fishing_bait"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// LAVA_EEL_ROD (plugin.js:387-390)
	{Name: "LAVA_EEL_ROD", Item: "oily_fishing_rod", Level: 53, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"LAVA_EEL"}, Variants: OilyFishingRodVariantDefs, Bait: []string{"fishing_bait"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// INFERNAL_EEL_ROD (plugin.js:392-396): rsprox capture 543
	// only shows the cast animation here, repeated — so the
	// loop animation is the cast (622), not the on-spot one.
	{Name: "INFERNAL_EEL_ROD", Item: "oily_fishing_rod", Level: 80, Animation: AnimRod, LoopAnimation: AnimRod,
		Fish: []string{"INFERNAL_EEL"}, Variants: OilyFishingRodVariantDefs, Bait: []string{"fishing_bait"},
		Worn:      []string{"ice_gloves", "smithing_uniform_gloves_ice"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// SACRED_EEL_ROD (plugin.js:397-400)
	{Name: "SACRED_EEL_ROD", Item: "fishing_rod", Level: 87, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"SACRED_EEL"}, Variants: FishingRodVariantDefs, Bait: []string{"fishing_bait"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		NoBaitStatement: NoFishingBaitStatement, LevelStatement: BaitLevelMessage},
	// ANGLERFISH_ROD (plugin.js:401-404): noBaitStatement
	// null — no box, the generic bait line instead.
	{Name: "ANGLERFISH_ROD", Item: "fishing_rod", Level: 82, Animation: AnimRod, LoopAnimation: AnimRodOnSpot,
		Fish: []string{"ANGLERFISH"}, Variants: FishingRodVariantDefs, Bait: []string{"piscarilius_sandworms"},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastLineMessage,
		LevelStatement: BaitLevelMessage},
	// DARK_CRAB_POT (plugin.js:405-407): the one tool that
	// relies on the default start line ("You begin to
	// fish..") and the default 5-tick interval.
	{Name: "DARK_CRAB_POT", Item: "lobster_pot", Level: 85, Animation: AnimCage, Fish: []string{"DARK_CRAB"},
		Bait: []string{"wilderness_fishing_bait"}, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true},
	// MINNOW_NET (plugin.js:409-414): 82 Fishing that boosts
	// can't reach, though boosts still speed up the catch;
	// its 2-tick rolls are too often to restart the net
	// animation on each. Spot MinnowPlatform is deferred
	// (PHASE35_PLAN.md §7).
	{Name: "MINNOW_NET", Item: "net", Level: 82, Animation: AnimNet, Fish: []string{"MINNOW"},
		Interval: MinnowAttemptIntervalTicks, Spot: "MinnowPlatform", Unboostable: true, AnimateOnRoll: boolPtr(false),
		StartMessage: CastNetMessage},
	// BARBARIAN_ROD (plugin.js:415-422): any of these baits
	// the rod; requires 15 Strength + Agility (the leaping
	// trout's requirements).
	{Name: "BARBARIAN_ROD", Item: "brut_fishing_rod", Level: 48, Animation: AnimBarbarianRod, LoopAnimation: AnimBarbarianRodOnSpot,
		Fish: []string{"LEAPING_STURGEON", "LEAPING_SALMON", "LEAPING_TROUT"}, Variants: BarbarianRodVariantDefs,
		Bait:      []string{"fishing_bait", "feather", "brut_fish_cuts", "brut_roe", "brut_caviar"},
		Quest:     "Barbarian Training",
		Requires:  []SkillRequirement{{Skill: game.SkillStrength, Level: 15}, {Skill: game.SkillAgility, Level: 15}},
		FirstRoll: HarpoonRodFirstRollTicks, HasFirstRoll: true, StartMessage: CastAndAttemptMessage,
		NoBaitStatement: NoBarbarianBaitStatement},
	// KARAMBWANJI_NET (plugin.js:424-427): karambwanji first,
	// else shrimps (Wiki: Fishing spot (karambwanji)).
	{Name: "KARAMBWANJI_NET", Item: "net", Level: 5, Animation: AnimNet, Fish: []string{"KARAMBWANJI", "SHRIMP"},
		Interval: NetCageHarpoonIntervalTicks, FirstRoll: NetCageFirstRollTicks, HasFirstRoll: true, StartMessage: CastNetMessage},
	// PARK_NET (plugin.js:429-448): the park pond — level 1,
	// never moves, one item off its table a catch, a roll a
	// tick; the wrong net is a chat line. The Wiki gives no
	// catch rate: 21/256 a tick is the recording's (6 catches
	// in about 66 ticks, rsprox capture 237/01098).
	{Name: "PARK_NET", Item: "big_net", Level: 1, Animation: AnimBigNet, Stationary: true,
		Interval: 1, FirstRoll: 1, HasFirstRoll: true,
		Fish: []string{"PARK_BROKEN_GLASS", "PARK_OLD_BOOT", "PARK_BIG_FISHING_NET", "PARK_CASKET", "PARK_HOUSE_KEYS",
			"PARK_JADE", "PARK_RED_TOPAZ", "PARK_LOOP_HALF"},
		Table: &dropTableDef{
			Rolls: [][2]int{{20, 20}},
			PreRoll: []tableRollDef{
				{Fish: "PARK_JADE", Chance: 1.0 / 100},
				{Fish: "PARK_RED_TOPAZ", Chance: 1.0 / 505},
			},
			Weighted: []tableWeightDef{
				{Fish: "PARK_BROKEN_GLASS", Weight: 3},
				{Fish: "PARK_OLD_BOOT", Weight: 2},
				{Fish: "PARK_BIG_FISHING_NET", Weight: 2},
				{Fish: "PARK_CASKET", Weight: 2},
				{Fish: "PARK_HOUSE_KEYS", Weight: 1},
			},
			Tertiary: []tableRollDef{{Fish: "PARK_LOOP_HALF", Chance: 1.0 / 1014}},
		},
		StartMessage: CastNetMessage, NoToolMessage: "This is the wrong size net for fishing here."},
	// KARAMBWAN_VESSEL (plugin.js:450-458): the spots never
	// move (Wiki: Fishing).
	{Name: "KARAMBWAN_VESSEL", Item: "tbwt_karambwan_vessel", Level: 65, Animation: AnimVessel, Fish: []string{"KARAMBWAN"},
		Variants: []toolVariantDef{{Items: []string{"tbwt_karambwan_vessel_loaded_with_karambwanji", "tbwt_karambwan_vessel"}, Level: 1, Bonus: 100}},
		Interval: KarambwanIntervalTicks, Vessel: true, Stationary: true,
		StartMessage:    "You lower the Karambwan vessel.",
		MissMessage:     "A Karambwan deftly snatches the Karambwanji from your vessel!",
		NoToolStatement: "You need a Karambwan vessel to fish here."},
}

// ---- spot maps (source defs: option -> TS TOOLS name) ----

// SpotToolsByOptionsDef is SPOT_TOOLS_BY_OPTIONS
// (plugin.js:461-471): a spot fishes by its full option list
// in the cache, so every id with the same options behaves
// alike. "Bait" means sea fish on a "Small Net/Bait" spot but
// pike on a river "Lure/Bait" spot.
var SpotToolsByOptionsDef = map[string]map[string]string{
	"Small Net/Bait":  {"Small Net": "NET", "Bait": "FISHING_ROD"},
	"Lure/Bait":       {"Lure": "FLY_FISHING_ROD", "Bait": "PIKE_ROD"},
	"Cage/Harpoon":    {"Cage": "LOBSTER_POT", "Harpoon": "HARPOON"},
	"Big Net/Harpoon": {"Big Net": "BIG_NET", "Harpoon": "SHARK_HARPOON"},
	// Older spots label the big net "Net".
	"Net/Harpoon": {"Net": "BIG_NET", "Harpoon": "SHARK_HARPOON"},
	"Use-rod":     {"Use-rod": "BARBARIAN_ROD"},
	// Only the Wilderness dark crab spots offer a lone "Cage".
	"Cage": {"Cage": "DARK_CRAB_POT"},
}

// frogSpawnAndEels is FROG_SPAWN_AND_EELS (plugin.js:473):
// frog spawn and eels, underground around Lumbridge and
// Dorgesh-Kaan.
var frogSpawnAndEels = map[string]string{
	"Small Net": "FROG_SPAWN_NET", "Net": "FROG_SPAWN_NET", "Bait": "CAVE_EEL_ROD",
}

// SpotToolsByNPCDef is SPOT_TOOLS_BY_NPC
// (plugin.js:479-521): option lists that mean different fish
// at different spots, so they go by id. Camdozaal tools are
// the CAMDOZAAL_SMALL_NET / CAMDOZAAL_BIG_NET entries from
// camdozaal.go.
var SpotToolsByNPCDef = map[int]map[string]string{
	NPCCamdozaalFishingSpot1: {"Small Net": "CAMDOZAAL_SMALL_NET", "Big Net": "CAMDOZAAL_BIG_NET"},
	NPCCamdozaalFishingSpot2: {"Small Net": "CAMDOZAAL_SMALL_NET", "Big Net": "CAMDOZAAL_BIG_NET"},
	NPCCamdozaalFishingSpot3: {"Small Net": "CAMDOZAAL_SMALL_NET", "Big Net": "CAMDOZAAL_BIG_NET"},
	// Tutorial Island's shrimp pond only offers "Net".
	NPCFishingSpot43: {"Net": "NET"},
	// Lumbridge Swamp's small net/bait spot labels its net "Net".
	NPCFishingSpot30: {"Net": "NET", "Bait": "FISHING_ROD"},
	// Piscatoris: small net for monkfish, harpoon for
	// tuna/swordfish.
	NPCFishingSpot55: {"Net": "MONKFISH_NET", "Harpoon": "HARPOON"},
	// Frog spawn and eels.
	NPCFishingSpot2:  frogSpawnAndEels,
	NPCFishingSpot3:  frogSpawnAndEels,
	NPCFishingSpot4:  frogSpawnAndEels,
	NPCFishingSpot5:  frogSpawnAndEels,
	NPCFishingSpot92: frogSpawnAndEels,
	// Mort Myre swamp.
	NPCFishingSpot40: {"Bait": "SWAMP_EEL_ROD"},
	NPCFishingSpot41: {"Bait": "SWAMP_EEL_ROD"},
	NPCFishingSpot42: {"Bait": "SWAMP_EEL_ROD"},
	// Lava eels: Taverley Dungeon, Lava Maze and the rest.
	NPCFishingSpot63:  {"Bait": "LAVA_EEL_ROD"},
	NPCFishingSpot6:   {"Bait": "LAVA_EEL_ROD"},
	NPCFishingSpot130: {"Bait": "LAVA_EEL_ROD"},
	// Zul-Andra.
	NPCFishingSpot68: {"Bait": "SACRED_EEL_ROD"},
	// Port Piscarilius.
	NPCRodFishingSpot16: {"Bait": "ANGLERFISH_ROD"},
	// Mor Ul Rek.
	NPCRodFishingSpot20: {"Bait": "INFERNAL_EEL_ROD"},
	// Kylie Minnow's platform at the Fishing Guild.
	NPCFishingSpot87: {"Small Net": "MINNOW_NET"},
	NPCFishingSpot88: {"Small Net": "MINNOW_NET"},
	NPCFishingSpot89: {"Small Net": "MINNOW_NET"},
	NPCFishingSpot90: {"Small Net": "MINNOW_NET"},
	// Karamja: karambwanji in the Holy Lake, karambwan north
	// of the shipyard and by Lubufu.
	NPCFishingSpot58: {"Net": "KARAMBWANJI_NET"},
	NPCFishingSpot60: {"Fish": "KARAMBWAN_VESSEL"},
	NPCFishingSpot61: {"Fish": "KARAMBWAN_VESSEL"},
	NPCFishingSpot62: {"Fish": "KARAMBWAN_VESSEL"},
	// Civitas illa Fortis's park pond, its lone "Cast".
	NPCFishingSpot101: {"Cast": "PARK_NET"},
}

// ClueBottleFishNames is CLUE_BOTTLE_FISH
// (plugin.js:523-529): clue-bottle bases are the documented
// heron bases for these catches. Other catches with no sourced
// base remain unsupported here; this is not a claim that they
// cannot yield bottles.
var ClueBottleFishNames = []string{
	"SHRIMP", "ANCHOVY", "SARDINE", "HERRING", "TROUT", "SALMON",
	"PIKE", "TUNA", "LOBSTER", "SWORDFISH", "SHARK", "SHARK_BAREHAND",
	"TUNA_BAREHAND", "SWORDFISH_BAREHAND", "MACKEREL", "COD", "BASS",
	"MONKFISH", "CAVE_EEL", "INFERNAL_EEL", "SACRED_EEL", "ANGLERFISH",
	"DARK_CRAB", "KARAMBWANJI", "KARAMBWAN", "MINNOW",
	"LEAPING_TROUT", "LEAPING_SALMON", "LEAPING_STURGEON",
}

// ---- loader ----

// IDResolver resolves the cache gameval names the source defs
// are written in. It is the seam Load resolves through, so
// tests can drive Load from a hand-built fixture instead of a
// live cache; production passes LoadFromCache's adapter over
// *cache.CacheDefinitions. Fishing needs obj ids only:
// animations, ticks and NPC ids are numeric literals in the
// plugin (unlike Fletching, which also resolves seqs and
// varbits).
type IDResolver interface {
	ObjID(name string) (int, bool, error)
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error) { return r.defs.GetObjId(name) }

// LoadFromCache resolves the Fishing tables against the real
// cache definitions. There is no data file and no path: every
// table is transcribed in this package (see the package
// comment), so unlike Fletching's LoadFromCache there is no
// path argument. A nil cache fails; an unresolvable name fails
// listing every missing name.
func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("fishing: nil cache definitions")
	}
	return Load(cacheResolver{defs: defs})
}

// resolver collects unresolved names so Load fails closed with
// every missing name listed, instead of stopping at the first
// — the woodcutting tables.go precedent.
type resolver struct {
	r       IDResolver
	missing []string
	seen    map[string]int
}

func (rs *resolver) obj(name string) int {
	if id, ok := rs.seen[name]; ok {
		return id
	}
	id, ok, err := rs.r.ObjID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
		return 0
	}
	rs.seen[name] = id
	return id
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("fishing: unresolved cache ids: %v", rs.missing)
}

// Load resolves every source def through r and returns the
// resolved Tables, or an error listing every gameval name the
// cache does not know. No id is invented: an unresolvable name
// fails the load.
func Load(r IDResolver) (*Tables, error) {
	if r == nil {
		return nil, errors.New("fishing: nil id resolver")
	}
	rs := &resolver{r: r, seen: make(map[string]int)}
	t := &Tables{
		Tools:              make(map[string]*FishingTool, len(ToolDefs)+len(camdozaalToolDefs)),
		Fish:               make(map[string]*Fish, len(FishDefs)+len(camdozaalFishDefs)),
		SpotToolsByOptions: make(map[string]map[string]*FishingTool, len(SpotToolsByOptionsDef)),
		SpotToolsByNPC:     make(map[int]map[string]*FishingTool, len(SpotToolsByNPCDef)),
		CamdozaalTools:     make(map[string]*FishingTool, len(camdozaalToolDefs)),
		ClueBottleFish:     make(map[string]bool, len(ClueBottleFishNames)),
	}

	// Fish first: tools reference them by TS name.
	for _, def := range FishDefs {
		t.Fish[def.Name] = rs.resolveFish(def)
	}
	for _, def := range camdozaalFishDefs {
		t.Fish[def.Name] = rs.resolveFish(def)
	}

	// All tools (TS TOOLS + Camdozaal) resolve into one
	// lookup; only the TS TOOLS names enter Tables.Tools.
	allTools := make(map[string]*FishingTool, len(ToolDefs)+len(camdozaalToolDefs))
	for _, def := range ToolDefs {
		tool, err := rs.resolveTool(def, t.Fish)
		if err != nil {
			return nil, err
		}
		t.Tools[def.Name] = tool
		allTools[def.Name] = tool
	}
	for name, def := range camdozaalToolDefs {
		tool, err := rs.resolveTool(def, t.Fish)
		if err != nil {
			return nil, err
		}
		allTools[name] = tool
	}
	// Camdozaal tools are keyed by their option names, as
	// CAMDOZAAL_TOOLS is (Camdozaal.Fishing.js:13-16).
	t.CamdozaalTools["Small Net"] = allTools["CAMDOZAAL_SMALL_NET"]
	t.CamdozaalTools["Big Net"] = allTools["CAMDOZAAL_BIG_NET"]

	// Variant sets (plugin.js:176-205).
	for _, def := range HarpoonVariantDefs {
		t.Harpoons = append(t.Harpoons, rs.resolveVariant(def))
	}
	for _, def := range FishingRodVariantDefs {
		t.FishingRods = append(t.FishingRods, rs.resolveVariant(def))
	}
	for _, def := range FlyFishingRodVariantDefs {
		t.FlyFishingRods = append(t.FlyFishingRods, rs.resolveVariant(def))
	}
	for _, def := range OilyFishingRodVariantDefs {
		t.OilyFishingRods = append(t.OilyFishingRods, rs.resolveVariant(def))
	}
	for _, def := range BarbarianRodVariantDefs {
		t.BarbarianRods = append(t.BarbarianRods, rs.resolveVariant(def))
	}

	// Spot maps, by tool name.
	for key, options := range SpotToolsByOptionsDef {
		resolved := make(map[string]*FishingTool, len(options))
		for option, name := range options {
			tool, ok := allTools[name]
			if !ok {
				return nil, fmt.Errorf("fishing: spot options %q name unknown tool %q", key, name)
			}
			resolved[option] = tool
		}
		t.SpotToolsByOptions[key] = resolved
	}
	for npcID, options := range SpotToolsByNPCDef {
		resolved := make(map[string]*FishingTool, len(options))
		for option, name := range options {
			tool, ok := allTools[name]
			if !ok {
				return nil, fmt.Errorf("fishing: spot npc %d names unknown tool %q", npcID, name)
			}
			resolved[option] = tool
		}
		t.SpotToolsByNPC[npcID] = resolved
	}

	for _, name := range ClueBottleFishNames {
		if _, ok := t.Fish[name]; !ok {
			return nil, fmt.Errorf("fishing: clue-bottle fish %q is not a fish", name)
		}
		t.ClueBottleFish[name] = true
	}

	t.DistinctItemNames = len(rs.seen)
	t.ResolvedItemNames = len(rs.seen)
	t.GamevalItemNames = len(rs.seen)
	t.FallbackItemNames = 0

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

func (rs *resolver) resolveFish(def fishDef) *Fish {
	fish := &Fish{
		Name: def.Name, Item: def.Item, Level: def.Level,
		Rolls: def.Rolls, XP: def.XP, Caught: def.Caught,
		PetBase: def.PetBase, Requires: def.Requires,
		ExtraXP: def.ExtraXP, Amount: def.Amount,
		Message: def.Message, Junk: def.Junk,
		CatchAnimation: def.CatchAnimation,
		DiaryRolls:     def.DiaryRolls,
	}
	fish.ID = rs.obj(def.Item)
	if def.Big != nil {
		fish.Big = &BigFish{
			ID: rs.obj(def.Big.Item), Item: def.Big.Item,
			Chance: def.Big.Chance, Name: def.Big.Name,
		}
	}
	return fish
}

func (rs *resolver) resolveVariant(def toolVariantDef) ToolVariant {
	variant := ToolVariant{
		Items: def.Items, Level: def.Level, Bonus: def.Bonus,
		Agility: def.Agility, Crystal: def.Crystal,
		Infernal: def.Infernal, Barehand: def.Barehand,
	}
	for _, name := range def.Items {
		variant.IDs = append(variant.IDs, rs.obj(name))
	}
	return variant
}

func (rs *resolver) resolveTool(def toolDef, fish map[string]*Fish) (*FishingTool, error) {
	tool := &FishingTool{
		Name: def.Name, Item: def.Item, Level: def.Level,
		AnimationID: def.Animation, CatchAnimationID: def.CatchAnimation,
		Quest: def.Quest, Multi: def.Multi, Requires: def.Requires,
		IntervalFunc: def.IntervalFunc, FirstRoll: def.FirstRoll,
		HasFirstRoll: def.HasFirstRoll, Unboostable: def.Unboostable,
		Spot: def.Spot, Vessel: def.Vessel, Stationary: def.Stationary,
		FollowMessage: def.FollowMessage, MissMessage: def.MissMessage,
		NoToolStatement: def.NoToolStatement,
		NoBaitStatement: def.NoBaitStatement,
		NoToolMessage:   def.NoToolMessage,
		LevelStatement:  def.LevelStatement, Barehand: def.Barehand,
		ClueBase: def.ClueBase,
	}
	tool.ID = rs.obj(def.Item)
	tool.LoopAnimationID = def.Animation
	if def.LoopAnimation != 0 {
		tool.LoopAnimationID = def.LoopAnimation
	}
	tool.Interval = FishingActionIntervalTicks
	if def.Interval != 0 {
		tool.Interval = def.Interval
	}
	tool.AnimateOnRoll = true
	if def.AnimateOnRoll != nil {
		tool.AnimateOnRoll = *def.AnimateOnRoll
	}
	tool.StartMessage = def.StartMessage
	if tool.StartMessage == "" {
		tool.StartMessage = DefaultStartMessage
	}
	for _, name := range def.Fish {
		f, ok := fish[name]
		if !ok {
			return nil, fmt.Errorf("fishing: tool %q names unknown fish %q", def.Name, name)
		}
		tool.Fish = append(tool.Fish, f)
	}
	variants := def.Variants
	if variants == nil {
		variants = []toolVariantDef{{Items: []string{def.Item}, Level: 1, Bonus: 100}}
	}
	for _, vdef := range variants {
		tool.Variants = append(tool.Variants, rs.resolveVariant(vdef))
	}
	for _, name := range def.Bait {
		tool.Bait = append(tool.Bait, rs.obj(name))
		tool.BaitItems = append(tool.BaitItems, name)
	}
	for _, name := range def.Worn {
		tool.Worn = append(tool.Worn, rs.obj(name))
		tool.WornItems = append(tool.WornItems, name)
	}
	if def.Table != nil {
		table := &DropTable{Rolls: def.Table.Rolls}
		for _, roll := range def.Table.PreRoll {
			f, ok := fish[roll.Fish]
			if !ok {
				return nil, fmt.Errorf("fishing: tool %q table pre-roll names unknown fish %q", def.Name, roll.Fish)
			}
			table.PreRoll = append(table.PreRoll, TableRoll{Fish: f, Chance: roll.Chance})
		}
		for _, weight := range def.Table.Weighted {
			f, ok := fish[weight.Fish]
			if !ok {
				return nil, fmt.Errorf("fishing: tool %q table weighted names unknown fish %q", def.Name, weight.Fish)
			}
			table.Weighted = append(table.Weighted, TableWeight{Fish: f, Weight: weight.Weight})
		}
		for _, roll := range def.Table.Tertiary {
			f, ok := fish[roll.Fish]
			if !ok {
				return nil, fmt.Errorf("fishing: tool %q table tertiary names unknown fish %q", def.Name, roll.Fish)
			}
			table.Tertiary = append(table.Tertiary, TableRoll{Fish: f, Chance: roll.Chance})
		}
		tool.Table = table
	}
	return tool, nil
}
