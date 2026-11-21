// Package mining ports the Mining skill (Mining.plugin.js, 1,221
// lines, plus its eight sub-units) to Go.
//
// Grounding: ~/tsps-bfd81e3/server/plugins/skills/Mining.plugin.js
// and ~/tsps-bfd81e3/server/plugins/skills/mining/*.Mining.js, all
// read at bfd81e3. The entry plugin's ROCKS objectIds literals are
// STALE (they contradict the rev-242 cache: copper 7453/7484 are a
// door and a wall pile) and are NOT copied: TS never reads them in
// the loop (only in the table literals, stripped at :296 for veins);
// registration is by object name and depletion resolves from the
// runtime loc's gameval. The Go tables therefore store NO rock
// object ids.
//
// XP is int throughout (SkillManager.AddExperiences takes an int,
// so a rock's fractional XP truncates at the grant site — the
// same documented integer-truncation gap as gaps 16-19).
package mining

import (
	"fmt"
	"regexp"
	"sort"

	"tsps-go/internal/cache"
)

// ---- constants (Mining.plugin.js) ----

// Messages, sounds and scripts.
const (
	// NO_ORE_MESSAGE (plugin.js:33): mining a depleted rock.
	NoOreMessage = "There is currently no ore available in this rock."
	// NO_ORE_SOUND (plugin.js:35): played on a depleted rock/vein.
	NoOreSound = 2661
	// SWING_MESSAGE (plugin.js:38).
	SwingMessage = "You swing your pick at the rock."
	// NO_PICKAXE_MESSAGE (plugin.js:39).
	NoPickaxeMessage = "You need a pickaxe to mine this rock. You do not have a pickaxe which you have the Mining level to use."
	// MINING_SUCCESS_SOUND (plugin.js:40).
	MiningSuccessSound = 3600
	// SMELT_SOUND (plugin.js:41): infernal combust.
	SmeltSound = 2725
	// SMOKEPUFF_GRAPHIC / SMOKEPUFF_HEIGHT (plugin.js:42-43).
	SmokepuffGraphic = 86
	SmokepuffHeight  = 150
	// RESPAWN_TIMER_SCRIPT / ID / COLOUR (plugin.js:30-32).
	RespawnTimerScript = 5474
	RespawnTimerID     = 1
	RespawnTimerColour = 16765184
	// ORE_MINED_VARP / ATTRIBUTE (plugin.js:44-45).
	OreMinedVarp      = "tracking_ore_mined"
	OreMinedAttribute = "mining:ores-mined"
	// FIRST_ROLL_DELAY_TICKS (plugin.js:51).
	FirstRollDelayTicks = 2
	// SUCCESS_ROLL_SIDES (plugin.js:56).
	SuccessRollSides = 256
	// MAX_VISIBLE_LEVEL (plugin.js:57).
	MaxVisibleLevel = 99
	// GEM_ROLL_FACES / GLORY_GEM_ROLL_FACES (plugin.js:62-63).
	GemRollFaces      = 1
	GloryGemRollFaces = 3
	// PURE_ESSENCE_LEVEL (plugin.js:659).
	PureEssenceLevel = 30
	// NO_BREATHING_FILTER (plugin.js:656).
	NoBreathingFilter = "You'll choke on the fumes if you try to mine without protection."
	// NO_ORE_IN_VEIN (plugin.js:305).
	NoOreInVein = "There is no ore currently available in this vein."
	// DEPLETED_VEIN (plugin.js:304).
	DepletedVein = "Depleted vein"
)

// EmptyRockNames are the mined-out locs without their own gameval
// that still offer Mine (plugin.js:36).
var EmptyRockNames = []string{"Empty wall", "Wall"}

// ---- tables ----

// Pickaxe is one PICKAXES entry (plugin.js:73-114).
type Pickaxe struct {
	// ID is the item id (ItemIds.*).
	ID int
	// Key is the pickaxe's cache sequence key (e.g. "dragon").
	Key string
	// Standard marks the seven standard pickaxes (bronze..rune)
	// plus dragon (plugin.js:73-80: standard: true).
	Standard bool
	// RequiredLevel is the Mining level to use it.
	RequiredLevel int
	// RequiredAgility is the Agility level to wield it (crystal
	// pickaxes: 50; 0 means the default 1, plugin.js:503).
	RequiredAgility int
	// Rank overrides RequiredLevel in the PICKAXES_DESC sort
	// (inactive crystal 61, echo 72; 0 means RequiredLevel).
	Rank int
	// AttemptIntervalTicks is the roll interval.
	AttemptIntervalTicks int
	// FastRollChance is the 1/6 (dragon/infernal) or 1/4
	// (crystal) chance of a 1-tick-faster roll (members only).
	FastRollChance float64
	// HasFastRoll reports FastRollChance > 0.
	HasFastRoll bool
	// Animation is the swing sequence id.
	Animation int
	// Walls overrides the wall/sang swing sequence names where
	// the cache orders the words differently (plugin.js:86-90).
	Walls map[string]string
}

// Outcome is one weighted outcome of a rock (barronite, calcified,
// gem rocks, infernal shale): the item, amount range, weight, XP
// and success line.
type Outcome struct {
	ItemID  int
	Min     int
	Max     int
	Weight  int
	XP      float64
	Message string
}

// CascadeStep is one step of a cascading chart (sandstone,
// granite): the piece, its [low, high] chart, XP and line.
type CascadeStep struct {
	ItemID  int
	Low     int
	High    int
	XP      float64
	Message string
}

// DepletionSound is a rock's depletion area sound (barronite,
// calcified): sound id, delay, radius.
type DepletionSound struct {
	ID     int
	Delay  int
	Radius int
}

// Yield is a rock's multi-ore yield (nickel min-max, rubium
// min + 1/6 chance, amethyst min-max).
type Yield struct {
	Min    int
	Max    int     // 0 means unbounded (rubium: min + chance)
	Chance float64 // 0 means fixed count (nickel, amethyst)
}

// Rock is one ROCKS / ORE_VEINS / ESSENCE_ROCKS entry. The
// ObjectIDs field of the TS table is deliberately NOT carried:
// the literals are stale (see the package comment).
type Rock struct {
	// Name is the loc display name (objectName), the
	// registration key.
	Name string
	// Level is the Mining level required.
	Level int
	// XP is the Mining XP per ore (fractional; truncated at
	// the grant site).
	XP float64
	// OreID is the item id a success yields (oreId).
	OreID int
	// PetBase is the pet roll base (0 = no pet roll).
	PetBase int
	// Chance is the [low, high] success chart; nil for
	// cascades and infinite rocks (essence).
	Chance *[2]int
	// GloryChance is the gem rocks' better chart with a
	// charged glory worn.
	GloryChance *[2]int
	// Gems rolls the mining gem table before each ore roll.
	Gems bool
	// RespawnTicks is the mined-out respawn time.
	RespawnTicks int
	// Swing is "wall" or "sang" for wall rocks; "" otherwise.
	Swing string
	// Sound overrides the success sound (gem rocks 2655,
	// daeyalt 2998).
	Sound int
	// HasSound reports Sound != 0.
	HasSound bool
	// DepletionTicks is the timer after which the rock runs
	// out (barronite 33, calcified 117).
	DepletionTicks int
	// DepletesOnOre: the rock goes with the first ore after
	// its timer ran out (calcified).
	DepletesOnOre bool
	// CountEach counts every unit in tracking_ore_mined
	// (calcified shards).
	CountEach bool
	// DepletionSound is played on depletion (barronite,
	// calcified).
	DepletionSound *DepletionSound
	// DepletedRocks maps a gameval to its own mined-out loc
	// (calcified, gem rocks, veins).
	DepletedRocks map[string]string
	// DepletedRock is the rock's own mined-out loc gameval
	// (granite "rocks2", amethyst "amethystrock_empty",
	// daeyalt "area_sanguine_mine_minerocks_04").
	DepletedRock string
	// Outcomes are the weighted outcomes (barronite,
	// calcified, gem rocks, infernal shale).
	Outcomes []Outcome
	// Cascade is the cascading chart (sandstone, granite).
	Cascade []CascadeStep
	// Yield is the multi-ore yield (nickel, rubium, amethyst).
	Yield *Yield
	// DoubleFromLevel gives 2 ores from this level (rubium 72).
	DoubleFromLevel int
	// OreMessage overrides the success line.
	OreMessage string
	// GameMessage sends the success line as a game message
	// instead of spam (daeyalt).
	GameMessage bool
	// FullName overrides the ore name in the full-inventory
	// line (daeyalt: "ore").
	FullName string
	// RequiresBreathingFilter (volcanic sulphur).
	RequiresBreathingFilter bool
	// RuneEssence: the essence mine's outcrops (pure vs rune
	// essence by members + level 30).
	RuneEssence bool
	// Infinite: the rock never depletes (essence).
	Infinite bool
	// Silent: no success line (essence).
	Silent bool
	// EssenceByLevel: 1-4 essence by level (ancient essence).
	EssenceByLevel bool
	// Stages are the daeyalt face stages (_01.._03).
	Stages []string
	// EmptyMessage is sent on depletion (daeyalt).
	EmptyMessage string
	// Vein: true for ORE_VEINS entries (Abandoned Mine walls).
	Vein bool
}

// GemDrop is one GEM_TABLE row (plugin.js:60-72).
type GemDrop struct {
	// Name is the ItemIdentifiers gameval name ("" for the
	// 25/256 "nothing" row).
	Name string
	// ItemID is the resolved item id (0 for nothing).
	ItemID int
	// Weight is the row weight.
	Weight int
}

// Tables is the resolved Mining content: the source tables with
// numeric ids, plus the cache lookups the loop needs at runtime
// (loc gamevals, varp id, wall swing sequences, item names).
type Tables struct {
	Pickaxes     []Pickaxe
	PickaxesDesc []Pickaxe
	Rocks        []Rock
	RocksByName  map[string]*Rock
	EssenceRocks []Rock
	Veins        []Rock
	GemTable     []GemDrop
	// LocGamevalByID is the cache loc gameval per loc id.
	LocGamevalByID map[int]string
	// LocIDByGameval is the reverse lookup.
	LocIDByGameval map[string]int
	// ItemNameByID is the cache item display name per item id.
	ItemNameByID map[int]string
	// OresMinedVarpID is the cache varp id for
	// tracking_ore_mined (4526 in rev-242).
	OresMinedVarpID int
	// SwingSeqs maps a wall swing sequence name to its cache
	// seq id (human_mining_<key>_pickaxe_wall,
	// sang_mining_wall_<key>_pickaxe); absent names fall back
	// to the pickaxe's own animation.
	SwingSeqs map[string]int
}

// ItemName returns the cache display name for an item id, or ""
// when unknown.
func (t *Tables) ItemName(id int) string {
	if t == nil {
		return ""
	}
	return t.ItemNameByID[id]
}

// LocGameval returns the cache loc gameval for a loc id, or "".
func (t *Tables) LocGameval(id int) string {
	if t == nil {
		return ""
	}
	return t.LocGamevalByID[id]
}

// LocID returns the loc id for a cache loc gameval, or -1.
func (t *Tables) LocID(gameval string) int {
	if t == nil {
		return -1
	}
	if id, ok := t.LocIDByGameval[gameval]; ok {
		return id
	}
	return -1
}

// RockByName returns the rock registered under a loc display name.
func (t *Tables) RockByName(name string) *Rock {
	if t == nil {
		return nil
	}
	return t.RocksByName[name]
}

// DepletedRockID resolves a rock's mined-out loc id
// (depletedRockId, plugin.js:524-533): the rock's own depleted
// loc for this gameval, else "<gameval>_empty", else "rocks"
// plus the digit the gameval ends in (copperrock2 -> rocks2),
// else "rocks1". Returns -1 when nothing resolves.
func (t *Tables) DepletedRockID(rockID int, rock *Rock) int {
	if t == nil {
		return -1
	}
	gameval := t.LocGameval(rockID)
	if rock != nil {
		if own, ok := rock.DepletedRocks[gameval]; ok && own != "" {
			if id := t.LocID(own); id >= 0 {
				return id
			}
		}
		if rock.DepletedRock != "" {
			if id := t.LocID(rock.DepletedRock); id >= 0 {
				return id
			}
		}
	}
	if gameval != "" {
		if id := t.LocID(gameval + "_empty"); id >= 0 {
			return id
		}
	}
	if m := rockVariantPattern.FindStringSubmatch(gameval); m != nil {
		if id := t.LocID("rocks" + m[1]); id >= 0 {
			return id
		}
	}
	return t.LocID("rocks1")
}

var rockVariantPattern = regexp.MustCompile(`rock_?(\d)$`)

// SwingSeq resolves a pickaxe's swing sequence at a rock
// (swingAnimation, plugin.js:339-348): the rock's wall/sang
// swing where the cache orders the words differently (the
// pickaxe's Walls override), else the generated name; an
// unresolvable name falls back to the pickaxe's animation.
func (t *Tables) SwingSeq(pickaxe Pickaxe, rock *Rock) int {
	if rock == nil || rock.Swing == "" {
		return pickaxe.Animation
	}
	var seqName string
	if pickaxe.Walls != nil {
		if override, ok := pickaxe.Walls[rock.Swing]; ok {
			seqName = override
		}
	}
	if seqName == "" {
		switch rock.Swing {
		case "wall":
			seqName = "human_mining_" + pickaxe.Key + "_pickaxe_wall"
		case "sang":
			seqName = "sang_mining_wall_" + pickaxe.Key + "_pickaxe"
		}
	}
	if t != nil {
		if id, ok := t.SwingSeqs[seqName]; ok {
			return id
		}
	}
	return pickaxe.Animation
}

// ---- source tables (numeric ids from ItemIdentifiers.ts) ----

func sourcePickaxes() []Pickaxe {
	return []Pickaxe{
		{ID: 1265, Key: "bronze", Standard: true, RequiredLevel: 1, AttemptIntervalTicks: 8, Animation: 625},
		{ID: 1267, Key: "iron", Standard: true, RequiredLevel: 1, AttemptIntervalTicks: 7, Animation: 626},
		{ID: 1269, Key: "steel", Standard: true, RequiredLevel: 6, AttemptIntervalTicks: 6, Animation: 627},
		{ID: 12297, Key: "black", Standard: true, RequiredLevel: 11, AttemptIntervalTicks: 5, Animation: 3873},
		{ID: 1273, Key: "mithril", Standard: true, RequiredLevel: 21, AttemptIntervalTicks: 5, Animation: 629},
		{ID: 1271, Key: "adamant", Standard: true, RequiredLevel: 31, AttemptIntervalTicks: 4, Animation: 628},
		{ID: 1275, Key: "rune", Standard: true, RequiredLevel: 41, AttemptIntervalTicks: 3, Animation: 624},
		{ID: 11920, Key: "dragon", Standard: true, RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 7139},
		{ID: 23680, Key: "crystal", RequiredLevel: 71, RequiredAgility: 50, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 4, HasFastRoll: true, Animation: 8347},
		{ID: 13243, Key: "infernal", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 4482},
		{ID: 23276, Key: "gilded", RequiredLevel: 41, AttemptIntervalTicks: 3, Animation: 8313},
		{ID: 12797, Key: "dragon_pretty", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 642,
			Walls: map[string]string{"wall": "human_mining_dragon_pickaxe_pretty_wall", "sang": "sang_mining_wall_dragon_pickaxe_pretty"}},
		{ID: 20014, Key: "3a", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 7283},
		{ID: 23677, Key: "zalcano", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 8346},
		{ID: 25376, Key: "trailblazer_no_infernal", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 8887,
			Walls: map[string]string{"wall": "human_mining_trailblazer_pickaxe_no_infernal_wall", "sang": "sang_mining_wall_trailblazer_pickaxe_no_infernal"}},
		{ID: 30351, Key: "trailblazer_reloaded_no_infernal", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 11833,
			Walls: map[string]string{"wall": "human_mining_trailblazer_reloaded_pickaxe_no_infernal_wall", "sang": "sang_mining_wall_trailblazer_reloaded_pickaxe_no_infernal"}},
		{ID: 25063, Key: "trailblazer", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 11831},
		{ID: 30345, Key: "trailblazer_reloaded", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 11832},
		{ID: 13244, Key: "infernal", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 4482},
		{ID: 25369, Key: "trailblazer", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 11831},
		{ID: 30346, Key: "trailblazer_reloaded", RequiredLevel: 61, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 11832},
		{ID: 23682, Key: "crystal", RequiredLevel: 71, Rank: 61, RequiredAgility: 50, AttemptIntervalTicks: 3, FastRollChance: 1.0 / 6, HasFastRoll: true, Animation: 8347},
		{ID: 25112, Key: "crystal", RequiredLevel: 1, Rank: 72, AttemptIntervalTicks: 2, Animation: 8347},
	}
}

// pickaxesDesc sorts a copy of the pickaxes best-first
// (PICKAXES_DESC, plugin.js:115-116): rank (or required level)
// descending, then interval ascending.
func pickaxesDesc(pickaxes []Pickaxe) []Pickaxe {
	desc := make([]Pickaxe, len(pickaxes))
	copy(desc, pickaxes)
	rank := func(p Pickaxe) int {
		if p.Rank != 0 {
			return p.Rank
		}
		return p.RequiredLevel
	}
	sort.SliceStable(desc, func(i, j int) bool {
		if rank(desc[i]) != rank(desc[j]) {
			return rank(desc[i]) > rank(desc[j])
		}
		return desc[i].AttemptIntervalTicks < desc[j].AttemptIntervalTicks
	})
	return desc
}

func chart(low, high int) *[2]int { return &[2]int{low, high} }

func sourceRocks() []Rock {
	return []Rock{
		{Name: "Clay rocks", Level: 1, XP: 5, OreID: 434, PetBase: 741600, Chance: chart(128, 400), Gems: true, RespawnTicks: 2},
		{Name: "Copper rocks", Level: 1, XP: 17.5, OreID: 436, PetBase: 741600, Chance: chart(100, 350), Gems: true, RespawnTicks: 4},
		{Name: "Tin rocks", Level: 1, XP: 17.5, OreID: 438, PetBase: 741600, Chance: chart(100, 350), Gems: true, RespawnTicks: 4},
		{Name: "Blurite rocks", Level: 10, XP: 17.5, OreID: 668, Chance: chart(90, 350), Gems: true, RespawnTicks: 42},
		{Name: "Iron rocks", Level: 15, XP: 35, OreID: 440, PetBase: 741600, Chance: chart(96, 350), Gems: true, RespawnTicks: 9},
		{Name: "Barronite rocks", Level: 14, XP: 32, OreID: 25684, Chance: chart(80, 100), Gems: true, RespawnTicks: 100, Swing: "wall",
			DepletionTicks: 33, DepletionSound: &DepletionSound{ID: 2661, Delay: 10, Radius: 1},
			Outcomes: []Outcome{
				{ItemID: 25676, Min: 4, Max: 6, Weight: 76, XP: 16, Message: "You manage to mine some barronite shards."},
				{ItemID: 25684, Min: 1, Max: 1, Weight: 24, XP: 32, Message: "You manage to mine a barronite deposit."},
			}},
		{Name: "Calcified rocks", Level: 41, XP: 33, OreID: 29088, PetBase: 741600, Chance: chart(20, 200), Gems: true, RespawnTicks: 50, Swing: "wall",
			DepletionTicks: 117, DepletesOnOre: true, CountEach: true, DepletionSound: &DepletionSound{ID: 2661, Delay: 10, Radius: 1},
			DepletedRocks: map[string]string{
				"varlamore_mining_rock":   "varlamore_mining_rock_empty",
				"varlamore_mining_rock02": "varlamore_mining_rock_empty02",
				"varlamore_mining_rock03": "varlamore_mining_rock_empty03",
				"varlamore_mining_rock04": "varlamore_mining_rock_empty04",
			},
			Outcomes: []Outcome{
				{ItemID: 29381, Min: 1, Max: 3, Weight: 74, XP: 33, Message: "You manage to chip off some bone shards."},
				{ItemID: 29088, Min: 1, Max: 1, Weight: 1, XP: 36, Message: "You manage to mine a calcified deposit."},
			}},
		{Name: "Sandstone rocks", Level: 35, XP: 30, OreID: 6971, PetBase: 741600, RespawnTicks: 9,
			Cascade: []CascadeStep{
				{ItemID: 6977, Low: 4, High: 50, XP: 60, Message: "You manage to quarry some sandstone."},
				{ItemID: 6975, Low: 8, High: 75, XP: 50, Message: "You manage to quarry some sandstone."},
				{ItemID: 6973, Low: 16, High: 100, XP: 40, Message: "You manage to quarry some sandstone."},
				{ItemID: 6971, Low: 25, High: 200, XP: 30, Message: "You manage to quarry some sandstone."},
			}},
		{Name: "Granite rocks", Level: 45, XP: 50, OreID: 6979, PetBase: 741600, RespawnTicks: 9, DepletedRock: "rocks2",
			Cascade: []CascadeStep{
				{ItemID: 6983, Low: 6, High: 64, XP: 75, Message: "You manage to quarry some granite."},
				{ItemID: 6981, Low: 8, High: 75, XP: 60, Message: "You manage to quarry some granite."},
				{ItemID: 6979, Low: 16, High: 100, XP: 50, Message: "You manage to quarry some granite."},
			}},
		{Name: "Silver rocks", Level: 20, XP: 40, OreID: 442, PetBase: 741600, Chance: chart(25, 200), Gems: true, RespawnTicks: 100},
		{Name: "Coal rocks", Level: 30, XP: 50, OreID: 453, PetBase: 290640, Chance: chart(16, 100), Gems: true, RespawnTicks: 50},
		{Name: "Gold rocks", Level: 40, XP: 65, OreID: 444, PetBase: 296640, Chance: chart(7, 75), Gems: true, RespawnTicks: 100},
		{Name: "Mithril rocks", Level: 55, XP: 80, OreID: 447, PetBase: 148320, Chance: chart(4, 50), Gems: true, RespawnTicks: 200},
		{Name: "Adamantite rocks", Level: 70, XP: 95, OreID: 449, PetBase: 59328, Chance: chart(2, 25), Gems: true, RespawnTicks: 400},
		{Name: "Runite rocks", Level: 85, XP: 125, OreID: 451, PetBase: 42377, Chance: chart(1, 18), Gems: true, RespawnTicks: 1200},
		{Name: "Soft clay rocks", Level: 70, XP: 5, OreID: 1761, PetBase: 741600, Chance: chart(128, 400), Gems: true, RespawnTicks: 2, OreMessage: "You manage to mine some soft clay."},
		{Name: "Lead rocks", Level: 25, XP: 40.5, OreID: 31716, PetBase: 741600, Chance: chart(112, 278), Gems: true, RespawnTicks: 10},
		{Name: "Lovakite rocks", Level: 65, XP: 60, OreID: 13356, PetBase: 245562, Chance: chart(16, 100), Gems: true, RespawnTicks: 58},
		{Name: "Volcanic sulphur", Level: 42, XP: 35, OreID: 13571, PetBase: 710000, Chance: chart(100, 220), Gems: true, RespawnTicks: 42, RequiresBreathingFilter: true, OreMessage: "You manage to mine some volcanic sulphur."},
		{Name: "Nickel rocks", Level: 74, XP: 80.5, OreID: 31719, PetBase: 247200, Chance: chart(4, 50), Gems: true, RespawnTicks: 200, Yield: &Yield{Min: 2, Max: 16}},
		{Name: "Rubium rocks", Level: 48, XP: 30, OreID: 31722, Chance: chart(107, 230), Gems: true, RespawnTicks: 59, Yield: &Yield{Min: 2, Chance: 1.0 / 6}, DoubleFromLevel: 72, OreMessage: "You manage to mine some rubium splinters."},
		{Name: "Sunstone rocks", Level: 53, XP: 23, OreID: 34020, PetBase: 741600, Chance: chart(69, 301), RespawnTicks: 9},
		{Name: "Infernal shale rocks", Level: 78, XP: 13.2, OreID: 30846, Chance: chart(60, 180), Gems: true, RespawnTicks: 9,
			Outcomes: []Outcome{
				{ItemID: 30846, Weight: 1, XP: 13.2, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 19.6, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 21.7, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 27.5, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 30, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 32.5, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 38.3, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 40.4, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 46.8, Message: "You manage to mine some infernal shale."},
				{ItemID: 30846, Weight: 1, XP: 100, Message: "You manage to mine some infernal shale."},
			}},
		{Name: "Gem rocks", Level: 40, XP: 65, OreID: 1625, PetBase: 211886, Chance: chart(28, 70), GloryChance: chart(84, 210), RespawnTicks: 99, Sound: 2655, HasSound: true,
			DepletedRocks: map[string]string{"gemrock": "rocks2"},
			Outcomes: []Outcome{
				{ItemID: 1625, Weight: 60, XP: 65, Message: "You just mined an Opal!"},
				{ItemID: 1627, Weight: 30, XP: 65, Message: "You just mined a piece of Jade!"},
				{ItemID: 1629, Weight: 15, XP: 65, Message: "You just mined a Red Topaz!"},
				{ItemID: 1623, Weight: 9, XP: 65, Message: "You just mined a Sapphire!"},
				{ItemID: 1621, Weight: 5, XP: 65, Message: "You just mined an Emerald!"},
				{ItemID: 1619, Weight: 5, XP: 65, Message: "You just mined a Ruby!"},
				{ItemID: 1617, Weight: 4, XP: 65, Message: "You just mined a Diamond!"},
			}},
		{Name: "Amethyst crystals", Level: 92, XP: 240, OreID: 21347, PetBase: 46350, Chance: chart(-18, 10), Gems: true, RespawnTicks: 125, Swing: "wall", Yield: &Yield{Min: 2, Max: 3}, DepletedRock: "amethystrock_empty"},
		{Name: "Daeyalt rocks", Level: 20, XP: 17.5, OreID: 9632, Chance: chart(96, 350), RespawnTicks: 47, Swing: "sang", Sound: 2998, HasSound: true,
			OreMessage: "You manage to mine some ore.", GameMessage: true, FullName: "ore",
			Stages:       []string{"area_sanguine_mine_minerocks_01", "area_sanguine_mine_minerocks_02", "area_sanguine_mine_minerocks_03"},
			DepletedRock: "area_sanguine_mine_minerocks_04", EmptyMessage: "This rock face is now empty."},
	}
}

// sourceGemTable is the GEM_TABLE (plugin.js:60-72): a 1/256
// pre-roll before each ore roll; the 25-weight row is "nothing".
func sourceGemTable() []GemDrop {
	return []GemDrop{
		{Name: "uncut_opal", Weight: 60},
		{Name: "uncut_jade", Weight: 30},
		{Name: "uncut_red_topaz", Weight: 15},
		{Name: "uncut_sapphire", Weight: 9},
		{Name: "uncut_emerald", Weight: 5},
		{Name: "uncut_ruby", Weight: 5},
		{Name: "uncut_diamond", Weight: 4},
		{Name: "", Weight: 25},
	}
}

func sourceEssenceRocks() []Rock {
	return []Rock{
		{Name: "Rune Essence", Level: 1, XP: 5, OreID: 7936, RuneEssence: true, RespawnTicks: 0, Infinite: true, Silent: true},
		{Name: "Rune essence", Level: 1, XP: 5, OreID: 7936, RuneEssence: true, RespawnTicks: 0, Infinite: true, Silent: true},
		{Name: "Ancient essence crystals", Level: 75, XP: 13.5, OreID: 27616, Gems: true, RespawnTicks: 0, Infinite: true, EssenceByLevel: true, OreMessage: "You manage to mine some ancient essence."},
	}
}

// veinPositions are the four Abandoned Mine wall positions
// (plugin.js:284).
var veinPositions = []string{"l", "r", "mid", "single"}

// sourceVeins builds the ORE_VEINS (plugin.js:285-303): each of
// the seven veins inherits its base rock's stats, swings "wall",
// and depletes to lotr_mine_wall_depleted_<pos>.
func sourceVeins(rocks []Rock) []Rock {
	byName := make(map[string]Rock, len(rocks))
	for _, rock := range rocks {
		byName[rock.Name] = rock
	}
	type veinSpec struct {
		name       string
		base       string
		ore        string
		oreMessage string
	}
	specs := []veinSpec{
		{"Tin ore vein", "Tin rocks", "tin", ""},
		{"Copper ore vein", "Copper rocks", "copper", ""},
		{"Clay ore vein", "Clay rocks", "clay", ""},
		{"Coal ore vein", "Coal rocks", "coal", ""},
		{"Iron ore vein", "Iron rocks", "iron", ""},
		{"Mithril ore vein", "Mithril rocks", "mithril", ""},
		{"Adamant ore vein", "Adamantite rocks", "adamant", "You manage to mine some adamantite."},
	}
	veins := make([]Rock, 0, len(specs))
	for _, spec := range specs {
		base, ok := byName[spec.base]
		if !ok {
			continue
		}
		vein := base
		vein.Name = spec.name
		vein.Swing = "wall"
		vein.Vein = true
		if spec.oreMessage != "" {
			vein.OreMessage = spec.oreMessage
		}
		vein.DepletedRocks = make(map[string]string, len(veinPositions))
		for _, pos := range veinPositions {
			vein.DepletedRocks["lotr_mine_wall_"+spec.ore+"_"+pos] = "lotr_mine_wall_depleted_" + pos
		}
		veins = append(veins, vein)
	}
	return veins
}

// ---- loading ----

// LoadFromCache resolves the Mining tables against the loaded
// cache definitions: loc gamevals (for depletion and staging),
// item display names (for gem/ore/filter lines), the
// tracking_ore_mined varp id, gem item ids and the wall swing
// sequences. A gameval that does not resolve fails the boot,
// matching the other content loaders: a cache whose names do
// not resolve is a config error, not a degradation.
func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error) {
	if defs == nil {
		return nil, fmt.Errorf("mining: nil cache definitions")
	}
	t := &Tables{
		Pickaxes:     sourcePickaxes(),
		Rocks:        sourceRocks(),
		EssenceRocks: sourceEssenceRocks(),
		GemTable:     sourceGemTable(),
	}
	t.PickaxesDesc = pickaxesDesc(t.Pickaxes)
	t.Veins = sourceVeins(t.Rocks)
	t.RocksByName = make(map[string]*Rock, len(t.Rocks)+len(t.EssenceRocks)+len(t.Veins))
	for i := range t.Rocks {
		t.RocksByName[t.Rocks[i].Name] = &t.Rocks[i]
	}
	for i := range t.EssenceRocks {
		t.RocksByName[t.EssenceRocks[i].Name] = &t.EssenceRocks[i]
	}
	for i := range t.Veins {
		t.RocksByName[t.Veins[i].Name] = &t.Veins[i]
	}

	locNames, err := defs.GetLocNames()
	if err != nil {
		return nil, fmt.Errorf("mining: loc names: %w", err)
	}
	t.LocGamevalByID = locNames
	t.LocIDByGameval = make(map[string]int, len(locNames))
	for id, name := range locNames {
		t.LocIDByGameval[name] = id
	}
	objNames, err := defs.GetObjNames()
	if err != nil {
		return nil, fmt.Errorf("mining: obj names: %w", err)
	}
	t.ItemNameByID = objNames

	// Every gameval the depletion/stage chain can ask for must
	// resolve (depletedRockId, plugin.js:524-533, plus the
	// daeyalt stages and the vein depleted locs).
	needed := []string{"rocks1", "rocks2"}
	for _, rock := range t.Rocks {
		for _, target := range rock.DepletedRocks {
			needed = append(needed, target)
		}
		if rock.DepletedRock != "" {
			needed = append(needed, rock.DepletedRock)
		}
		for _, stage := range rock.Stages {
			needed = append(needed, stage)
		}
	}
	for _, vein := range t.Veins {
		for _, target := range vein.DepletedRocks {
			needed = append(needed, target)
		}
	}
	// The "<gameval>_empty" fallback is probed at runtime per
	// rock gameval; the known ones are verified here so a
	// missing empty loc is a boot error, not a silent rocks1.
	for _, empty := range []string{"leadrock1_empty", "amethystrock_empty", "varlamore_mining_rock_empty", "varlamore_mining_rock_empty02", "varlamore_mining_rock_empty03", "varlamore_mining_rock_empty04"} {
		needed = append(needed, empty)
	}
	seen := make(map[string]bool, len(needed))
	for _, name := range needed {
		if seen[name] {
			continue
		}
		seen[name] = true
		if _, ok := t.LocIDByGameval[name]; !ok {
			return nil, fmt.Errorf("mining: loc gameval %q not in cache", name)
		}
	}

	varpID, ok, err := defs.GetVarpId(OreMinedVarp)
	if err != nil {
		return nil, fmt.Errorf("mining: varp %q: %w", OreMinedVarp, err)
	}
	if !ok {
		return nil, fmt.Errorf("mining: varp %q not in cache", OreMinedVarp)
	}
	t.OresMinedVarpID = varpID

	for i := range t.GemTable {
		if t.GemTable[i].Name == "" {
			continue
		}
		id, ok, err := defs.GetObjId(t.GemTable[i].Name)
		if err != nil {
			return nil, fmt.Errorf("mining: gem %q: %w", t.GemTable[i].Name, err)
		}
		if !ok {
			return nil, fmt.Errorf("mining: gem %q not in cache", t.GemTable[i].Name)
		}
		t.GemTable[i].ItemID = id
	}

	// Wall swing sequences (swingAnimation, plugin.js:339-348):
	// the generated names for every pickaxe key, plus the
	// explicit Walls overrides.
	t.SwingSeqs = make(map[string]int)
	seqNames := make(map[string]bool)
	for _, p := range t.Pickaxes {
		seqNames["human_mining_"+p.Key+"_pickaxe_wall"] = true
		seqNames["sang_mining_wall_"+p.Key+"_pickaxe"] = true
		for _, override := range p.Walls {
			seqNames[override] = true
		}
	}
	for name := range seqNames {
		id, ok, err := defs.GetSeqId(name)
		if err != nil {
			return nil, fmt.Errorf("mining: seq %q: %w", name, err)
		}
		if ok {
			t.SwingSeqs[name] = id
		}
	}
	return t, nil
}

// DefaultTables returns the source tables without cache
// resolution (unit tests): loc lookups resolve nothing, so
// depletion falls back to -1 and wall swings to the pickaxe
// animation.
func DefaultTables() *Tables {
	t := &Tables{
		Pickaxes:     sourcePickaxes(),
		Rocks:        sourceRocks(),
		EssenceRocks: sourceEssenceRocks(),
		GemTable:     sourceGemTable(),
	}
	t.PickaxesDesc = pickaxesDesc(t.Pickaxes)
	t.Veins = sourceVeins(t.Rocks)
	t.RocksByName = make(map[string]*Rock, len(t.Rocks)+len(t.EssenceRocks)+len(t.Veins))
	for i := range t.Rocks {
		t.RocksByName[t.Rocks[i].Name] = &t.Rocks[i]
	}
	for i := range t.EssenceRocks {
		t.RocksByName[t.EssenceRocks[i].Name] = &t.EssenceRocks[i]
	}
	for i := range t.Veins {
		t.RocksByName[t.Veins[i].Name] = &t.Veins[i]
	}
	t.LocGamevalByID = map[int]string{}
	t.LocIDByGameval = map[string]int{}
	t.ItemNameByID = map[int]string{}
	t.SwingSeqs = map[string]int{}
	return t
}
