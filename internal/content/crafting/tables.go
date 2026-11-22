// Tables + loader: the resolved form of data/definitions/crafting.json
// (upstream commit bfd81e3, imported verbatim with Tyler's approval —
// SHA-256 bdeeb5b364c348b9565a425520630cd509a1d1d46aea7447f5ea86b040ae79eb).
//
// The JSON names items by their ItemIdentifiers (UPPER_SNAKE) names.
// Load resolves every name through the IDResolver — production passes
// LoadFromCache's adapter over *cache.CacheDefinitions — fallback table
// first (item_ids.go, transcribed from ItemIdentifiers.ts), then the
// lowercase gameval, and fails closed listing every name neither source
// resolves. No id is invented.
//
// Limestone and light orbs have NO section in crafting.json (confirmed
// 2026-11-21 — only one LIGHT_ORB token, as a glassblowing product, and
// no LIMESTONE token): their data is hardcoded in the TS units (Limestone
// level 12 / 6 XP, LightOrbs level 87 / 104 XP) and the loader does NOT
// fail on their absence; the units hardcode those constants. No JSON
// section is invented for them.
//
// Silver componentName values are already lowercase gameval form (e.g.
// "opal_ring") and resolve as-is, without lowercasing; 9 of the 20 are
// not obj gamevals at all (holy_symbol, unholy_symbol, sickle,
// lightning_rod, crossbow_bolt, ivandis, opal/jade/topaz_amulet), so the
// component's obj id resolves leniently (0 when unknown) and never fails
// the load — the name is a script/component label, not an item.
//
// XP is float in the JSON and stays float in the tables; truncation to
// int happens at the XP-grant call site (shared.go addCraftingXp), the
// same known structural gap as woodcutting/fletching/cooking/mining/
// smithing (gap 27a). The zenyte gem-cut XP is whatever the JSON says
// (89/50 in the imported file — the OSRS value is 200, but the loader
// ports the JSON, it does not second-guess it).
//
// The tables are read-only after Load.
package crafting

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"tsps-go/internal/cache"
)

// DefaultPath is the repo-relative path of the imported crafting
// file (data/definitions/crafting.json). LoadFromCache uses it.
const DefaultPath = "data/definitions/crafting.json"

// Gold and silver jewellery enum ids (Data.Crafting.js / cache
// config archive 8): the last-item tables for interfaces 446/6,
// keyed by (interfaceId<<16)|component.
const (
	goldLastTypeEnum   = 4318
	silverLastTypeEnum = 4319
)

// loadVarbits are the varbit gameval names the Jewellery unit
// resolves (Jewellery.Crafting.js): the gold/silver last-item
// varbits and the slayer-ring unlock. All three must resolve;
// a missing one fails the load.
var loadVarbits = []string{
	"crafting_gold_item_lasttype",
	"crafting_silver_item_lasttype",
	"slayer_ring_unlocked",
}

// extraItemNames are the ItemIdentifiers names the TS units use
// outside crafting.json (tools, bars, clay, orbs — see
// item_ids.go): resolved at load into Tables.ItemIDs so the
// units read them through tables.ItemID, the same way they read
// JSON names. Names already in the JSON resolve once.
var extraItemNames = []string{
	"AMETHYST",
	"BATTLESTAFF",
	"BUTTONS",
	"CAVE_GOBLIN_WIRE",
	"CHISEL",
	"CLAY",
	"COSTUME_NEEDLE",
	"CRUSHED_GEM",
	"GLASSBLOWING_PIPE",
	"GOLD_BAR",
	"IMCANDO_HAMMER",
	"IMCANDO_HAMMER_OFF_HAND_",
	"JEWELLERS_CHISEL",
	"LIGHT_ORB",
	"LIMESTONE",
	"LIMESTONE_BRICK",
	"MOLTEN_GLASS",
	"NEEDLE",
	"PERFECT_GOLD_BAR",
	"POLISHED_BUTTONS",
	"SILVER_BAR",
	"SILVER_SICKLE_B_",
	"SOFT_CLAY",
	"THREAD",
}

// extraSeqNames are the seq gameval names the TS units use that
// crafting.json never names (Jewellery's human_furnace anim,
// Limestone's human_limestonecutting anim): resolved at load and
// stored in Tables.SeqIDs for the units.
var extraSeqNames = []string{"human_furnace", "human_limestonecutting"}

// ---- resolved model ----

// Ingredient is one resolved [item, amount] pair.
type Ingredient struct {
	ItemID int
	Amount int
}

// LeatherRecipe is one resolved leather recipe (one row of a
// leather material's list): the product, how much leather it
// takes, its level and XP, the "You make ..." phrase (Made) and
// the menu label (Label, "" when the JSON sets none — the menu
// falls back to the cache item name).
type LeatherRecipe struct {
	ItemID  int
	Leather int
	Level   int
	XP      float64
	Made    string
	Label   string
	Members bool
}

// LeatherMaterial is one leather section entry (a material and
// the recipes it sews, in file order — Tables.Leather keeps the
// JSON's material order for the menu).
type LeatherMaterial struct {
	MaterialID   int
	MaterialName string
	Recipes      []LeatherRecipe
}

// HunterPouchRecipe is one resolved huntersPouches recipe: the
// fur and how many it takes, the pouch it makes, the pouch an
// upgrade turns From (0 when the recipe makes a fresh pouch),
// its level and XP.
type HunterPouchRecipe struct {
	FurID   int
	Amount  int
	FromID  int
	HasFrom bool
	ItemID  int
	Level   int
	XP      float64
	Members bool
}

// CombineRecipe is one resolved combine recipe: the two items
// used on each other (Pair, in JSON order) and how many of each
// (PairAmounts, default [1, 1]), the extra materials consumed
// (Materials), the tools kept (ToolIDs), the product, its level
// and XP, and the ticks and animation the loop uses.
type CombineRecipe struct {
	Pair        [2]int
	PairAmounts [2]int
	Materials   []Ingredient
	ToolIDs     []int
	ItemID      int
	Level       int
	XP          float64
	Ticks       int
	SeqID       int
	SeqName     string
	Members     bool
}

// CrushData is a gem's crush chart (gems recipes' crush field):
// the Wiki success-chart [low, high] values out of 256 at
// levels 1/99 and the XP a crushed gem pays.
type CrushData struct {
	Low  int
	High int
	XP   float64
}

// GemRecipe is one resolved gems recipe: the uncut and cut gem,
// the plural for the level refusal, its level and XP, its
// cutting animation, and its crush chart when the JSON sets one
// (HasCrush).
type GemRecipe struct {
	UncutID  int
	CutID    int
	Plural   string
	Level    int
	XP       float64
	SeqID    int
	SeqName  string
	Members  bool
	Crush    CrushData
	HasCrush bool
}

// AmethystRecipe is one resolved amethyst recipe: the product,
// how many one amethyst makes, its level and XP.
type AmethystRecipe struct {
	ItemID  int
	Amount  int
	Level   int
	XP      float64
	Members bool
}

// AmethystData is the resolved amethyst section: the shared
// cutting animation and the four products.
type AmethystData struct {
	SeqID   int
	SeqName string
	Recipes []AmethystRecipe
}

// GlassblowingRecipe is one resolved glassblowing recipe: the
// product, its menu label and its name in "You make a ..." (Name;
// Made overrides it when set), its level and XP.
type GlassblowingRecipe struct {
	ItemID  int
	Label   string
	Name    string
	Made    string
	Level   int
	XP      float64
	Members bool
}

// GlassblowingData is the resolved glassblowing section: the
// shared animation and sound (with its delay) and the recipes.
type GlassblowingData struct {
	SeqID      int
	SeqName    string
	SoundID    int
	SoundDelay int
	Recipes    []GlassblowingRecipe
}

// PotteryRecipe is one resolved pottery recipe: the name and
// plural, the unfired and fired items, its level, the shaping
// and firing XP, how many pieces one soft clay shapes (Shaped,
// default 1), the menu label for a shaped set (ShapeLabel), the
// shape/put/remove spam lines, whether it can crack (Cracks,
// default true — cups set false), and the quest unlock when the
// JSON sets one (HasUnlock).
type PotteryRecipe struct {
	Name          string
	Plural        string
	UnfiredID     int
	FiredID       int
	Level         int
	ShapingXP     float64
	FiringXP      float64
	Shaped        int
	ShapeLabel    string
	ShapeMessage  string
	PutMessage    string
	RemoveMessage string
	Cracks        bool
	Members       bool
	UnlockQuest   string
	UnlockStage   int
	HasUnlock     bool
}

// PotteryData is the resolved pottery section: the wheel and
// oven animations, the shaping and firing sounds, the recipes,
// and the water-container pairs (a full container and the empty
// one mixing clay leaves behind).
type PotteryData struct {
	SeqID           int
	SeqName         string
	OvenSeqID       int
	OvenSeqName     string
	SoundID         int
	OvenSoundID     int
	Recipes         []PotteryRecipe
	WaterContainers []Ingredient // ItemID is the full container, Amount is the empty container's id
}

// SpinningRecipe is one resolved spinning recipe: the product,
// the materials any one of which makes it (MaterialIDs, in
// preference order) and how much of it one item takes (Amount),
// its level and XP.
type SpinningRecipe struct {
	ItemID      int
	MaterialIDs []int
	Amount      int
	Level       int
	XP          float64
	Members     bool
}

// SpinningData is the resolved spinning section: the wheel
// animation (player + wheel seqs) and sound, and the recipes.
type SpinningData struct {
	SeqID        int
	SeqName      string
	WheelSeqID   int
	WheelSeqName string
	SoundID      int
	Recipes      []SpinningRecipe
}

// WeavingRecipe is one resolved weaving recipe, shaped like a
// spinning recipe.
type WeavingRecipe struct {
	ItemID      int
	MaterialIDs []int
	Amount      int
	Level       int
	XP          float64
	Members     bool
}

// WeavingData is the resolved weaving section: the loom
// animation and sound (with its delay) and the recipes.
type WeavingData struct {
	SeqID      int
	SeqName    string
	SoundID    int
	SoundDelay int
	Recipes    []WeavingRecipe
}

// GoldProduct is one resolved gold jewellery product row: the
// product item, its level and XP. The gem slot it belongs to is
// the row's index in its type's Products (gold.gems order:
// plain, sapphire, emerald, ruby, diamond, dragonstone, onyx,
// zenyte).
type GoldProduct struct {
	ItemID int
	Level  int
	XP     float64
}

// GoldType is one resolved gold jewellery type (ring, necklace,
// amulet, bracelet): the interface components of its eight gem
// slots, its mould, and its eight products in gem order.
type GoldType struct {
	Name       string
	Components []int
	MouldID    int
	Products   []GoldProduct
}

// GoldSlayerRing is the resolved gold slayerRing section: the
// extra ring button's component and mould, its level and XP, the
// varbit name gating it, and the gem → ring products (GemID is
// the carried gem, RingID the ring it makes, in preference
// order: eternal first).
type GoldSlayerRing struct {
	Component    int
	MouldID      int
	Level        int
	XP           float64
	UnlockVarbit string
	Products     []Ingredient // ItemID is the gem, Amount is the ring
}

// GoldData is the resolved gold section: the gem slots (GemIDs,
// -1 for the plain slot), the four types, the slayer ring, and
// the quantity buttons (Quantities maps component → amount; the
// "x" button's component is XComponent).
type GoldData struct {
	GemIDs     []int
	Types      []GoldType
	SlayerRing GoldSlayerRing
	Quantities map[int]int
	XComponent int
}

// SilverProduct is one resolved silver jewellery product: the
// interface component and its gameval name, the product, its
// level and XP, its mould, the extra items consumed besides the
// bar (ExtraIDs), how many it makes (Amount, default 1), and
// whether it is members.
type SilverProduct struct {
	Component      int
	ComponentName  string
	ComponentObjID int // the component name's obj id, 0 when it is not an obj gameval (9 of 20)
	ItemID         int
	Level          int
	XP             float64
	MouldID        int
	ExtraIDs       []int
	Amount         int
	Members        bool
}

// SilverData is the resolved silver section: the products and
// the quantity buttons (Quantities maps component → amount;
// the "x" button's component is XComponent).
type SilverData struct {
	Products   []SilverProduct
	Quantities map[int]int
	XComponent int
}

// PerfectRecipe is one resolved perfect jewellery recipe: the
// display name ("'perfect' ring"), the product, its mould, its
// level and XP.
type PerfectRecipe struct {
	Name    string
	ItemID  int
	MouldID int
	Level   int
	XP      float64
}

// StrungAmuletPair is one resolved strungAmulets pair: the
// unstrung item, what it strings into, the noun for the spam
// line (Noun, "" when the JSON sets none), and the string
// material (MaterialID — BALL_OF_WOOL unless the JSON names
// another).
type StrungAmuletPair struct {
	UnstrungID int
	StrungID   int
	Noun       string
	MaterialID int
}

// StrungAmuletData is the resolved strungAmulets section: the
// XP stringing pays and the pairs.
type StrungAmuletData struct {
	XP    float64
	Pairs []StrungAmuletPair
}

// BattlestavesRecipe is one resolved battlestaves recipe: the
// charged orb, the staff it makes, its level and XP.
type BattlestavesRecipe struct {
	OrbID  int
	ItemID int
	Level  int
	XP     float64
}

// BattlestavesData is the resolved battlestaves section: the
// shared animation and the four recipes.
type BattlestavesData struct {
	SeqID   int
	SeqName string
	Recipes []BattlestavesRecipe
}

// SnelmRecipe is one resolved snelms recipe: the blamish shell
// and the snelm it makes.
type SnelmRecipe struct {
	ShellID int
	ItemID  int
}

// SnelmsData is the resolved snelms section: the shared level,
// XP, ticks and animation, and the nine recipes.
type SnelmsData struct {
	Level   int
	XP      float64
	Ticks   int
	SeqID   int
	SeqName string
	Recipes []SnelmRecipe
}

// GemSickleRecipe is one resolved gemSickles recipe: the gem's
// display name (GemName, e.g. "Ruby" — a display name, not an
// item id; the gem item is the cut gem of that name), its name
// and article in messages, the sickle it makes, its XP and its
// cutting animation.
type GemSickleRecipe struct {
	GemName   string
	Name      string
	Article   string
	ProductID int
	XP        float64
	SeqID     int
	SeqName   string
}

// Tables is the resolved crafting data the loader returns: the
// sixteen JSON sections, resolved, plus the id maps the units
// resolve outside the tables. See the model comment above for
// the loader contract.
type Tables struct {
	Leather        []LeatherMaterial
	HuntersPouches []HunterPouchRecipe
	Combine        []CombineRecipe
	Gems           []GemRecipe
	Amethyst       AmethystData
	Glassblowing   GlassblowingData
	Pottery        PotteryData
	Spinning       SpinningData
	Weaving        WeavingData
	Gold           GoldData
	Silver         SilverData
	Perfect        []PerfectRecipe
	StrungAmulets  StrungAmuletData
	Battlestaves   BattlestavesData
	Snelms         SnelmsData
	GemSickles     []GemSickleRecipe

	// ItemIDs maps every ItemIdentifiers name the file uses (plus
	// the tool names the units use outside the JSON, resolved
	// through the same fallback table) to its resolved id.
	ItemIDs map[string]int
	// SeqIDs maps every animation gameval name the file and the
	// units use to its resolved seq id.
	SeqIDs map[string]int
	// VarbitIDs maps the three jewellery varbit names to their
	// ids (13892/13893/3207 in rev-242).
	VarbitIDs map[string]int
	// GoldLastType is enum 4318: (446<<16)|component -> slot
	// index (33 entries in rev-242).
	GoldLastType map[int]int
	// SilverLastType is enum 4319: (6<<16)|component -> slot
	// index (20 entries in rev-242).
	SilverLastType map[int]int
}

// ---- raw JSON shapes (data/crafting.json) ----

// rawPair is one [name, amount] pair.
type rawPair struct {
	Name   string
	Amount int
}

// UnmarshalJSON parses a [name, amount] pair.
func (p *rawPair) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[0], &p.Name); err != nil {
		return fmt.Errorf("pair name: %w", err)
	}
	if err := json.Unmarshal(pair[1], &p.Amount); err != nil {
		return fmt.Errorf("pair amount: %w", err)
	}
	return nil
}

type rawLeatherRecipe struct {
	Item    string  `json:"item"`
	Leather int     `json:"leather"`
	Level   int     `json:"level"`
	XP      float64 `json:"xp"`
	Made    string  `json:"made"`
	Label   string  `json:"label"`
	Members bool    `json:"members"`
}

// rawLeatherSection is the leather section: a material name ->
// recipes map, keeping the file's material order (the menu
// order) — encoding/json into a map would lose it.
type rawLeatherSection struct {
	Order      []string
	ByMaterial map[string][]rawLeatherRecipe
}

// UnmarshalJSON parses the leather object, recording its keys
// in file order. The "$comment" key carries prose, not recipes,
// and is dropped before the per-material decode.
func (s *rawLeatherSection) UnmarshalJSON(data []byte) error {
	keys, err := orderedObjectKeys(data)
	if err != nil {
		return err
	}
	var rawEntries map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawEntries); err != nil {
		return err
	}
	delete(rawEntries, "$comment")
	byMaterial := make(map[string][]rawLeatherRecipe, len(rawEntries))
	for material, entry := range rawEntries {
		var recipes []rawLeatherRecipe
		if err := json.Unmarshal(entry, &recipes); err != nil {
			return fmt.Errorf("crafting: leather %q: %w", material, err)
		}
		byMaterial[material] = recipes
	}
	var order []string
	for _, key := range keys {
		if key != "$comment" {
			order = append(order, key)
		}
	}
	s.Order, s.ByMaterial = order, byMaterial
	return nil
}

// orderedObjectKeys returns a JSON object's keys in file order
// (encoding/json maps do not keep it).
func orderedObjectKeys(data []byte) ([]string, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected JSON object")
	}
	var keys []string
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, fmt.Errorf("expected string key")
		}
		keys = append(keys, key)
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return nil, err
		}
	}
	return keys, nil
}

type rawHunterPouch struct {
	Fur     string  `json:"fur"`
	Amount  int     `json:"amount"`
	From    string  `json:"from"`
	Item    string  `json:"item"`
	Level   int     `json:"level"`
	XP      float64 `json:"xp"`
	Members bool    `json:"members"`
}

type rawCombine struct {
	Pair        [2]string `json:"pair"`
	PairAmounts []int     `json:"pairAmounts"`
	Materials   []rawPair `json:"materials"`
	Tools       []string  `json:"tools"`
	Item        string    `json:"item"`
	Level       int       `json:"level"`
	XP          float64   `json:"xp"`
	Ticks       int       `json:"ticks"`
	Seq         int       `json:"seq"`
	SeqName     string    `json:"seqName"`
	Members     bool      `json:"members"`
}

type rawCrush struct {
	Low  int     `json:"low"`
	High int     `json:"high"`
	XP   float64 `json:"xp"`
}

type rawGem struct {
	Uncut   string    `json:"uncut"`
	Cut     string    `json:"cut"`
	Plural  string    `json:"plural"`
	Level   int       `json:"level"`
	XP      float64   `json:"xp"`
	Seq     int       `json:"seq"`
	SeqName string    `json:"seqName"`
	Members bool      `json:"members"`
	Crush   *rawCrush `json:"crush"`
}

type rawAmethyst struct {
	Item    string  `json:"item"`
	Amount  int     `json:"amount"`
	Level   int     `json:"level"`
	XP      float64 `json:"xp"`
	Members bool    `json:"members"`
}

type rawGlassblowing struct {
	Item    string  `json:"item"`
	Label   string  `json:"label"`
	Name    string  `json:"name"`
	Made    string  `json:"made"`
	Level   int     `json:"level"`
	XP      float64 `json:"xp"`
	Members bool    `json:"members"`
}

type rawPottery struct {
	Name          string  `json:"name"`
	Plural        string  `json:"plural"`
	Unfired       string  `json:"unfired"`
	Fired         string  `json:"fired"`
	Level         int     `json:"level"`
	ShapingXP     float64 `json:"shapingXp"`
	FiringXP      float64 `json:"firingXp"`
	Members       bool    `json:"members"`
	Shaped        *int    `json:"shaped"`
	ShapeLabel    string  `json:"shapeLabel"`
	ShapeMessage  string  `json:"shapeMessage"`
	PutMessage    string  `json:"putMessage"`
	RemoveMessage string  `json:"removeMessage"`
	Cracks        *bool   `json:"cracks"`
	Unlock        *struct {
		Quest string `json:"quest"`
		Stage int    `json:"stage"`
	} `json:"unlock"`
}

type rawSpinning struct {
	Item      string   `json:"item"`
	Materials []string `json:"materials"`
	Amount    int      `json:"amount"`
	Level     int      `json:"level"`
	XP        float64  `json:"xp"`
	Members   bool     `json:"members"`
}

type rawGoldProduct struct {
	Item  string
	Level int
	XP    float64
}

// UnmarshalJSON parses a [item, level, xp] triple.
func (p *rawGoldProduct) UnmarshalJSON(data []byte) error {
	var triple [3]json.RawMessage
	if err := json.Unmarshal(data, &triple); err != nil {
		return err
	}
	if err := json.Unmarshal(triple[0], &p.Item); err != nil {
		return fmt.Errorf("gold product item: %w", err)
	}
	if err := json.Unmarshal(triple[1], &p.Level); err != nil {
		return fmt.Errorf("gold product level: %w", err)
	}
	if err := json.Unmarshal(triple[2], &p.XP); err != nil {
		return fmt.Errorf("gold product xp: %w", err)
	}
	return nil
}

type rawGoldType struct {
	Name       string           `json:"name"`
	Components []int            `json:"components"`
	Mould      string           `json:"mould"`
	Products   []rawGoldProduct `json:"products"`
}

type rawSlayerRingProduct struct {
	Gem  string
	Ring string
}

// UnmarshalJSON parses a [gem, ring] pair.
func (p *rawSlayerRingProduct) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[0], &p.Gem); err != nil {
		return fmt.Errorf("slayer ring gem: %w", err)
	}
	if err := json.Unmarshal(pair[1], &p.Ring); err != nil {
		return fmt.Errorf("slayer ring product: %w", err)
	}
	return nil
}

type rawFile struct {
	Leather        rawLeatherSection `json:"leather"`
	HuntersPouches struct {
		Recipes []rawHunterPouch `json:"recipes"`
	} `json:"huntersPouches"`
	Combine struct {
		Recipes []rawCombine `json:"recipes"`
	} `json:"combine"`
	Gems struct {
		Recipes []rawGem `json:"recipes"`
	} `json:"gems"`
	Amethyst struct {
		Seq     int           `json:"seq"`
		SeqName string        `json:"seqName"`
		Recipes []rawAmethyst `json:"recipes"`
	} `json:"amethyst"`
	Glassblowing struct {
		Seq        int               `json:"seq"`
		SeqName    string            `json:"seqName"`
		Sound      int               `json:"sound"`
		SoundDelay int               `json:"soundDelay"`
		Recipes    []rawGlassblowing `json:"recipes"`
	} `json:"glassblowing"`
	Pottery struct {
		Seq             int          `json:"seq"`
		SeqName         string       `json:"seqName"`
		OvenSeq         int          `json:"ovenSeq"`
		OvenSeqName     string       `json:"ovenSeqName"`
		Sound           int          `json:"sound"`
		OvenSound       int          `json:"ovenSound"`
		Recipes         []rawPottery `json:"recipes"`
		WaterContainers [][2]string  `json:"waterContainers"`
	} `json:"pottery"`
	Spinning struct {
		Seq          int           `json:"seq"`
		SeqName      string        `json:"seqName"`
		WheelSeq     int           `json:"wheelSeq"`
		WheelSeqName string        `json:"wheelSeqName"`
		Sound        int           `json:"sound"`
		Recipes      []rawSpinning `json:"recipes"`
	} `json:"spinning"`
	Weaving struct {
		Seq        int           `json:"seq"`
		SeqName    string        `json:"seqName"`
		Sound      int           `json:"sound"`
		SoundDelay int           `json:"soundDelay"`
		Recipes    []rawSpinning `json:"recipes"`
	} `json:"weaving"`
	Gold struct {
		Gems       []*string     `json:"gems"`
		Types      []rawGoldType `json:"types"`
		SlayerRing struct {
			Component    int                    `json:"component"`
			Mould        string                 `json:"mould"`
			Level        int                    `json:"level"`
			XP           float64                `json:"xp"`
			UnlockVarbit string                 `json:"unlockVarbit"`
			Products     []rawSlayerRingProduct `json:"products"`
		} `json:"slayerRing"`
		Quantities map[string]json.RawMessage `json:"quantities"`
	} `json:"gold"`
	Silver struct {
		Products []struct {
			Component     int      `json:"component"`
			ComponentName string   `json:"componentName"`
			Item          string   `json:"item"`
			Level         int      `json:"level"`
			XP            float64  `json:"xp"`
			Mould         string   `json:"mould"`
			Extra         []string `json:"extra"`
			Amount        *int     `json:"amount"`
			Members       bool     `json:"members"`
		} `json:"products"`
		Quantities map[string]json.RawMessage `json:"quantities"`
	} `json:"silver"`
	Perfect struct {
		Recipes []struct {
			Name  string  `json:"name"`
			Item  string  `json:"item"`
			Mould string  `json:"mould"`
			Level int     `json:"level"`
			XP    float64 `json:"xp"`
		} `json:"recipes"`
	} `json:"perfect"`
	StrungAmulets struct {
		XP    float64    `json:"xp"`
		Pairs [][]string `json:"pairs"`
	} `json:"strungAmulets"`
	Battlestaves struct {
		Seq     int    `json:"seq"`
		SeqName string `json:"seqName"`
		Recipes []struct {
			Orb   string  `json:"orb"`
			Item  string  `json:"item"`
			Level int     `json:"level"`
			XP    float64 `json:"xp"`
		} `json:"recipes"`
	} `json:"battlestaves"`
	Snelms struct {
		Level   int     `json:"level"`
		XP      float64 `json:"xp"`
		Ticks   int     `json:"ticks"`
		Seq     int     `json:"seq"`
		SeqName string  `json:"seqName"`
		Recipes []struct {
			Shell string `json:"shell"`
			Item  string `json:"item"`
		} `json:"recipes"`
	} `json:"snelms"`
	GemSickles struct {
		Recipes []struct {
			GemName string  `json:"gemName"`
			Name    string  `json:"name"`
			Article string  `json:"article"`
			Product string  `json:"product"`
			XP      float64 `json:"xp"`
			Seq     int     `json:"seq"`
			SeqName string  `json:"seqName"`
		} `json:"recipes"`
	} `json:"gemSickles"`
}

// IDResolver resolves the names the JSON file uses. It is the
// seam Load resolves through, so tests can drive Load from a
// hand-built fixture instead of a live cache; production passes
// LoadFromCache's adapter over *cache.CacheDefinitions. Item
// names arrive in their ItemIdentifiers (UPPER_SNAKE) form; the
// adapter lowercases them to gameval form (see itemGameval).
// Names in the itemIDFallbacks table never reach the resolver
// — the loader consults the table first (see resolver.obj).
// EnumValues answers a cache enum's key -> value entries
// (config archive 8), empty when the enum does not exist;
// ObjNames answers id -> obj gameval name; InterfaceID answers
// an interface's group id by gameval name. SeqID answers a seq
// gameval name; VarbitID answers a varbit gameval name.
type IDResolver interface {
	ObjID(name string) (int, bool, error)
	SeqID(name string) (int, bool, error)
	VarbitID(name string) (int, bool, error)
	EnumValues(id int) (map[int]int, error)
	ObjNames() (map[int]string, error)
	InterfaceID(name string) (int, bool, error)
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error) {
	return r.defs.GetObjId(name)
}

func (r cacheResolver) SeqID(name string) (int, bool, error) {
	return r.defs.GetSeqId(name)
}

func (r cacheResolver) VarbitID(name string) (int, bool, error) {
	return r.defs.GetVarbitId(name)
}

func (r cacheResolver) EnumValues(id int) (map[int]int, error) {
	values, err := r.defs.GetEnumValues(id)
	if err != nil {
		return nil, err
	}
	out := make(map[int]int, len(values))
	for key, value := range values {
		// DecodeEnumValues stores int values as int32
		// (ByteBuffer.ReadInt) and string values as string.
		switch n := value.(type) {
		case int32:
			out[key] = int(n)
		case int:
			out[key] = n
		default:
			return nil, fmt.Errorf("crafting: enum %d key %d: non-int value %T", id, key, value)
		}
	}
	return out, nil
}

func (r cacheResolver) ObjNames() (map[int]string, error) {
	return r.defs.GetObjNames()
}

func (r cacheResolver) InterfaceID(name string) (int, bool, error) {
	return r.defs.GetInterfaceId(name)
}

// itemGameval maps an ItemIdentifiers name to its cache gameval
// obj name: the lowercase form (e.g. "FLAX" -> "flax"). It only
// answers for the names whose gameval has that form; the rest
// resolve through itemIDFallbacks (see item_ids.go).
func itemGameval(name string) string {
	return strings.ToLower(name)
}

// resolver collects unresolved names so Load fails closed with
// every missing name listed, instead of stopping at the first —
// the woodcutting/fletching/cooking/smithing tables.go precedent.
// It also records every name that resolves, into itemIDs, so
// Tables.ItemIDs is filled as a side effect of resolution.
type resolver struct {
	r       IDResolver
	missing []string
	itemIDs map[string]int
}

// obj resolves one ItemIdentifiers name, recording it in
// itemIDs. The fallback table goes first: a name's lowercase
// form may be another item's gameval, so a gameval lookup must
// never win for it. A name in neither the table nor the cache
// is recorded missing.
func (rs *resolver) obj(name string) int {
	id, _ := rs.objOK(name)
	return id
}

func (rs *resolver) objOK(name string) (int, bool) {
	if id, ok := itemIDFallbacks[name]; ok {
		if rs.itemIDs == nil {
			rs.itemIDs = make(map[string]int)
		}
		rs.itemIDs[name] = id
		return id, true
	}
	id, ok, err := rs.r.ObjID(itemGameval(name))
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0, false
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
		return 0, false
	}
	if rs.itemIDs == nil {
		rs.itemIDs = make(map[string]int)
	}
	rs.itemIDs[name] = id
	return id, true
}

// objGameval resolves a name that is already in gameval form
// (a silver componentName such as "opal_ring") as-is, without
// lowercasing. Lenient: it reports ok=false and records
// nothing when the cache does not know the name — 9 of the 20
// silver component names are not obj gamevals at all.
func (rs *resolver) objGameval(name string) (int, bool) {
	id, ok, err := rs.r.ObjID(name)
	if err != nil || !ok {
		return 0, false
	}
	return id, true
}

// seq resolves a seq gameval name and checks it against the
// numeric id the JSON carries for it: a mismatch is a data
// error, not a fallback. It records the name in seqIDs.
func (rs *resolver) seq(name string, want int, seqIDs map[string]int) int {
	id, ok, err := rs.r.SeqID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q", name))
		return 0
	}
	if want != 0 && id != want {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q: cache id %d, want JSON id %d", name, id, want))
		return 0
	}
	seqIDs[name] = id
	return id
}

// seqOnly resolves a seq gameval name the JSON never numbers
// (the extraSeqNames) and records it in seqIDs.
func (rs *resolver) seqOnly(name string, seqIDs map[string]int) int {
	id, ok, err := rs.r.SeqID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("seq %q", name))
		return 0
	}
	seqIDs[name] = id
	return id
}

// varbit resolves a varbit gameval name, recording it in
// varbitIDs; a missing varbit fails the load.
func (rs *resolver) varbit(name string, varbitIDs map[string]int) int {
	id, ok, err := rs.r.VarbitID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("varbit %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("varbit %q", name))
		return 0
	}
	varbitIDs[name] = id
	return id
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("crafting: unresolved cache ids: %v", rs.missing)
}

// parseQuantities parses a jewellery quantities map
// (component -> amount, or "x" for the enter-amount button):
// numeric entries land in out, the "x" entry's component in
// xComponent. A malformed entry fails the load.
func parseQuantities(raw map[string]json.RawMessage) (out map[int]int, xComponent int, err error) {
	out = make(map[int]int, len(raw))
	for key, value := range raw {
		if strings.HasPrefix(key, "$") {
			continue // "$comment" prose, not a button
		}
		var component int
		if _, err := fmt.Sscanf(key, "%d", &component); err != nil {
			return nil, 0, fmt.Errorf("crafting: quantities key %q: %w", key, err)
		}
		var amount int
		if json.Unmarshal(value, &amount) == nil {
			out[component] = amount
			continue
		}
		var text string
		if json.Unmarshal(value, &text) == nil && text == "x" {
			xComponent = component
			continue
		}
		return nil, 0, fmt.Errorf("crafting: quantities component %d: bad value %s", component, string(value))
	}
	return out, xComponent, nil
}

// LoadFromCache resolves the Crafting tables against the real
// cache definitions and the data file at its default path. A
// nil cache fails; a missing or malformed file fails; an
// unresolvable name fails listing every missing name.
func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("crafting: nil cache definitions")
	}
	return LoadFile(DefaultPath, cacheResolver{defs: defs})
}

// LoadFile reads the crafting file at path and resolves it
// through r.
func LoadFile(path string, r IDResolver) (*Tables, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("crafting: reading %s: %w", path, err)
	}
	return Load(data, r)
}

// Load parses the crafting.json document in data and resolves
// every name through r, or returns an error listing every
// gameval name the cache does not know. No id is invented: an
// unresolvable name fails the load.
func Load(data []byte, r IDResolver) (*Tables, error) {
	if r == nil {
		return nil, errors.New("crafting: nil id resolver")
	}
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("crafting: parsing crafting.json: %w", err)
	}
	if len(raw.Leather.Order) == 0 {
		return nil, errors.New("crafting: no leather materials in crafting.json")
	}
	if len(raw.Gems.Recipes) == 0 {
		return nil, errors.New("crafting: no gem recipes in crafting.json")
	}
	rs := &resolver{r: r}
	t := &Tables{
		ItemIDs:        make(map[string]int),
		SeqIDs:         make(map[string]int),
		VarbitIDs:      make(map[string]int),
		GoldLastType:   make(map[int]int),
		SilverLastType: make(map[int]int),
	}

	// Leather (in file order).
	for _, material := range raw.Leather.Order {
		if material == "$comment" {
			continue
		}
		recipes := raw.Leather.ByMaterial[material]
		entry := LeatherMaterial{
			MaterialID:   rs.obj(material),
			MaterialName: material,
		}
		for _, row := range recipes {
			entry.Recipes = append(entry.Recipes, LeatherRecipe{
				ItemID:  rs.obj(row.Item),
				Leather: row.Leather,
				Level:   row.Level,
				XP:      row.XP,
				Made:    row.Made,
				Label:   row.Label,
				Members: row.Members,
			})
		}
		t.Leather = append(t.Leather, entry)
	}

	// Hunters' pouches.
	for _, row := range raw.HuntersPouches.Recipes {
		recipe := HunterPouchRecipe{
			FurID:   rs.obj(row.Fur),
			Amount:  row.Amount,
			ItemID:  rs.obj(row.Item),
			Level:   row.Level,
			XP:      row.XP,
			Members: row.Members,
		}
		if row.From != "" {
			recipe.FromID, recipe.HasFrom = rs.obj(row.From), true
		}
		t.HuntersPouches = append(t.HuntersPouches, recipe)
	}

	// Combine.
	for i, row := range raw.Combine.Recipes {
		recipe := CombineRecipe{
			Level:   row.Level,
			XP:      row.XP,
			Ticks:   row.Ticks,
			SeqName: row.SeqName,
			Members: row.Members,
		}
		recipe.Pair[0] = rs.obj(row.Pair[0])
		recipe.Pair[1] = rs.obj(row.Pair[1])
		recipe.PairAmounts = [2]int{1, 1}
		for j, amount := range row.PairAmounts {
			if j < 2 {
				recipe.PairAmounts[j] = amount
			}
		}
		for _, mat := range row.Materials {
			if mat.Amount <= 0 {
				return nil, fmt.Errorf("crafting: combine recipe %d material %q: amount %d, want > 0", i, mat.Name, mat.Amount)
			}
			recipe.Materials = append(recipe.Materials, Ingredient{ItemID: rs.obj(mat.Name), Amount: mat.Amount})
		}
		for _, tool := range row.Tools {
			recipe.ToolIDs = append(recipe.ToolIDs, rs.obj(tool))
		}
		recipe.ItemID = rs.obj(row.Item)
		recipe.SeqID = rs.seq(row.SeqName, row.Seq, t.SeqIDs)
		t.Combine = append(t.Combine, recipe)
	}

	// Gems.
	for _, row := range raw.Gems.Recipes {
		recipe := GemRecipe{
			UncutID: rs.obj(row.Uncut),
			CutID:   rs.obj(row.Cut),
			Plural:  row.Plural,
			Level:   row.Level,
			XP:      row.XP,
			SeqName: row.SeqName,
			Members: row.Members,
		}
		recipe.SeqID = rs.seq(row.SeqName, row.Seq, t.SeqIDs)
		if row.Crush != nil {
			recipe.Crush = CrushData{Low: row.Crush.Low, High: row.Crush.High, XP: row.Crush.XP}
			recipe.HasCrush = true
		}
		t.Gems = append(t.Gems, recipe)
	}

	// Amethyst.
	t.Amethyst.SeqName = raw.Amethyst.SeqName
	t.Amethyst.SeqID = rs.seq(raw.Amethyst.SeqName, raw.Amethyst.Seq, t.SeqIDs)
	for _, row := range raw.Amethyst.Recipes {
		t.Amethyst.Recipes = append(t.Amethyst.Recipes, AmethystRecipe{
			ItemID:  rs.obj(row.Item),
			Amount:  row.Amount,
			Level:   row.Level,
			XP:      row.XP,
			Members: row.Members,
		})
	}

	// Glassblowing.
	t.Glassblowing.SeqName = raw.Glassblowing.SeqName
	t.Glassblowing.SeqID = rs.seq(raw.Glassblowing.SeqName, raw.Glassblowing.Seq, t.SeqIDs)
	t.Glassblowing.SoundID = raw.Glassblowing.Sound
	t.Glassblowing.SoundDelay = raw.Glassblowing.SoundDelay
	for _, row := range raw.Glassblowing.Recipes {
		t.Glassblowing.Recipes = append(t.Glassblowing.Recipes, GlassblowingRecipe{
			ItemID:  rs.obj(row.Item),
			Label:   row.Label,
			Name:    row.Name,
			Made:    row.Made,
			Level:   row.Level,
			XP:      row.XP,
			Members: row.Members,
		})
	}

	// Pottery.
	t.Pottery.SeqName = raw.Pottery.SeqName
	t.Pottery.SeqID = rs.seq(raw.Pottery.SeqName, raw.Pottery.Seq, t.SeqIDs)
	t.Pottery.OvenSeqName = raw.Pottery.OvenSeqName
	t.Pottery.OvenSeqID = rs.seq(raw.Pottery.OvenSeqName, raw.Pottery.OvenSeq, t.SeqIDs)
	t.Pottery.SoundID = raw.Pottery.Sound
	t.Pottery.OvenSoundID = raw.Pottery.OvenSound
	for _, row := range raw.Pottery.Recipes {
		recipe := PotteryRecipe{
			Name:          row.Name,
			Plural:        row.Plural,
			UnfiredID:     rs.obj(row.Unfired),
			FiredID:       rs.obj(row.Fired),
			Level:         row.Level,
			ShapingXP:     row.ShapingXP,
			FiringXP:      row.FiringXP,
			Shaped:        1,
			ShapeLabel:    row.ShapeLabel,
			ShapeMessage:  row.ShapeMessage,
			PutMessage:    row.PutMessage,
			RemoveMessage: row.RemoveMessage,
			Cracks:        true,
			Members:       row.Members,
		}
		if row.Shaped != nil {
			recipe.Shaped = *row.Shaped
		}
		if row.Cracks != nil {
			recipe.Cracks = *row.Cracks
		}
		if row.Unlock != nil {
			recipe.UnlockQuest, recipe.UnlockStage, recipe.HasUnlock = row.Unlock.Quest, row.Unlock.Stage, true
		}
		t.Pottery.Recipes = append(t.Pottery.Recipes, recipe)
	}
	for _, pair := range raw.Pottery.WaterContainers {
		t.Pottery.WaterContainers = append(t.Pottery.WaterContainers, Ingredient{
			ItemID: rs.obj(pair[0]),
			Amount: rs.obj(pair[1]),
		})
	}

	// Spinning.
	t.Spinning.SeqName = raw.Spinning.SeqName
	t.Spinning.SeqID = rs.seq(raw.Spinning.SeqName, raw.Spinning.Seq, t.SeqIDs)
	t.Spinning.WheelSeqName = raw.Spinning.WheelSeqName
	t.Spinning.WheelSeqID = rs.seq(raw.Spinning.WheelSeqName, raw.Spinning.WheelSeq, t.SeqIDs)
	t.Spinning.SoundID = raw.Spinning.Sound
	for _, row := range raw.Spinning.Recipes {
		recipe := SpinningRecipe{
			ItemID:  rs.obj(row.Item),
			Amount:  row.Amount,
			Level:   row.Level,
			XP:      row.XP,
			Members: row.Members,
		}
		for _, material := range row.Materials {
			recipe.MaterialIDs = append(recipe.MaterialIDs, rs.obj(material))
		}
		t.Spinning.Recipes = append(t.Spinning.Recipes, recipe)
	}

	// Weaving.
	t.Weaving.SeqName = raw.Weaving.SeqName
	t.Weaving.SeqID = rs.seq(raw.Weaving.SeqName, raw.Weaving.Seq, t.SeqIDs)
	t.Weaving.SoundID = raw.Weaving.Sound
	t.Weaving.SoundDelay = raw.Weaving.SoundDelay
	for _, row := range raw.Weaving.Recipes {
		recipe := WeavingRecipe{
			ItemID:  rs.obj(row.Item),
			Amount:  row.Amount,
			Level:   row.Level,
			XP:      row.XP,
			Members: row.Members,
		}
		for _, material := range row.Materials {
			recipe.MaterialIDs = append(recipe.MaterialIDs, rs.obj(material))
		}
		t.Weaving.Recipes = append(t.Weaving.Recipes, recipe)
	}

	// Gold.
	for _, gem := range raw.Gold.Gems {
		if gem == nil {
			t.Gold.GemIDs = append(t.Gold.GemIDs, -1)
			continue
		}
		t.Gold.GemIDs = append(t.Gold.GemIDs, rs.obj(*gem))
	}
	for _, row := range raw.Gold.Types {
		typ := GoldType{
			Name:       row.Name,
			Components: append([]int(nil), row.Components...),
			MouldID:    rs.obj(row.Mould),
		}
		for _, product := range row.Products {
			typ.Products = append(typ.Products, GoldProduct{
				ItemID: rs.obj(product.Item),
				Level:  product.Level,
				XP:     product.XP,
			})
		}
		t.Gold.Types = append(t.Gold.Types, typ)
	}
	t.Gold.SlayerRing = GoldSlayerRing{
		Component:    raw.Gold.SlayerRing.Component,
		MouldID:      rs.obj(raw.Gold.SlayerRing.Mould),
		Level:        raw.Gold.SlayerRing.Level,
		XP:           raw.Gold.SlayerRing.XP,
		UnlockVarbit: raw.Gold.SlayerRing.UnlockVarbit,
	}
	for _, product := range raw.Gold.SlayerRing.Products {
		t.Gold.SlayerRing.Products = append(t.Gold.SlayerRing.Products, Ingredient{
			ItemID: rs.obj(product.Gem),
			Amount: rs.obj(product.Ring),
		})
	}
	quantities, xComponent, err := parseQuantities(raw.Gold.Quantities)
	if err != nil {
		return nil, err
	}
	t.Gold.Quantities, t.Gold.XComponent = quantities, xComponent

	// Silver.
	for _, row := range raw.Silver.Products {
		product := SilverProduct{
			Component:     row.Component,
			ComponentName: row.ComponentName,
			ItemID:        rs.obj(row.Item),
			Level:         row.Level,
			XP:            row.XP,
			MouldID:       rs.obj(row.Mould),
			Amount:        1,
			Members:       row.Members,
		}
		if id, ok := rs.objGameval(row.ComponentName); ok {
			product.ComponentObjID = id
		}
		for _, extra := range row.Extra {
			product.ExtraIDs = append(product.ExtraIDs, rs.obj(extra))
		}
		if row.Amount != nil {
			product.Amount = *row.Amount
		}
		t.Silver.Products = append(t.Silver.Products, product)
	}
	quantities, xComponent, err = parseQuantities(raw.Silver.Quantities)
	if err != nil {
		return nil, err
	}
	t.Silver.Quantities, t.Silver.XComponent = quantities, xComponent

	// Perfect.
	for _, row := range raw.Perfect.Recipes {
		t.Perfect = append(t.Perfect, PerfectRecipe{
			Name:    row.Name,
			ItemID:  rs.obj(row.Item),
			MouldID: rs.obj(row.Mould),
			Level:   row.Level,
			XP:      row.XP,
		})
	}

	// Strung amulets: pairs are [unstrung, strung] with an
	// optional noun and an optional material (default
	// BALL_OF_WOOL).
	t.StrungAmulets.XP = raw.StrungAmulets.XP
	for i, pair := range raw.StrungAmulets.Pairs {
		if len(pair) < 2 || len(pair) > 4 {
			return nil, fmt.Errorf("crafting: strungAmulets pair %d: %d entries, want 2-4", i, len(pair))
		}
		entry := StrungAmuletPair{
			UnstrungID: rs.obj(pair[0]),
			StrungID:   rs.obj(pair[1]),
		}
		if len(pair) >= 3 {
			entry.Noun = pair[2]
		}
		material := "BALL_OF_WOOL"
		if len(pair) >= 4 {
			material = pair[3]
		}
		entry.MaterialID = rs.obj(material)
		t.StrungAmulets.Pairs = append(t.StrungAmulets.Pairs, entry)
	}

	// Battlestaves.
	t.Battlestaves.SeqName = raw.Battlestaves.SeqName
	t.Battlestaves.SeqID = rs.seq(raw.Battlestaves.SeqName, raw.Battlestaves.Seq, t.SeqIDs)
	for _, row := range raw.Battlestaves.Recipes {
		t.Battlestaves.Recipes = append(t.Battlestaves.Recipes, BattlestavesRecipe{
			OrbID:  rs.obj(row.Orb),
			ItemID: rs.obj(row.Item),
			Level:  row.Level,
			XP:     row.XP,
		})
	}

	// Snelms.
	t.Snelms.Level = raw.Snelms.Level
	t.Snelms.XP = raw.Snelms.XP
	t.Snelms.Ticks = raw.Snelms.Ticks
	t.Snelms.SeqName = raw.Snelms.SeqName
	t.Snelms.SeqID = rs.seq(raw.Snelms.SeqName, raw.Snelms.Seq, t.SeqIDs)
	for _, row := range raw.Snelms.Recipes {
		t.Snelms.Recipes = append(t.Snelms.Recipes, SnelmRecipe{
			ShellID: rs.obj(row.Shell),
			ItemID:  rs.obj(row.Item),
		})
	}

	// Gem sickles: gemName is a display name ("Ruby"), not an
	// item id — it is stored as-is, never resolved.
	for _, row := range raw.GemSickles.Recipes {
		recipe := GemSickleRecipe{
			GemName:   row.GemName,
			Name:      row.Name,
			Article:   row.Article,
			ProductID: rs.obj(row.Product),
			XP:        row.XP,
			SeqName:   row.SeqName,
		}
		recipe.SeqID = rs.seq(row.SeqName, row.Seq, t.SeqIDs)
		t.GemSickles = append(t.GemSickles, recipe)
	}

	// The jewellery enums (4318 gold, 4319 silver): a cache
	// read failure fails the load; an empty enum is a data
	// error, not a degradation.
	goldEnum, err := r.EnumValues(goldLastTypeEnum)
	if err != nil {
		return nil, fmt.Errorf("crafting: enum %d (gold last type): %w", goldLastTypeEnum, err)
	}
	if len(goldEnum) == 0 {
		return nil, fmt.Errorf("crafting: enum %d (gold last type): no entries", goldLastTypeEnum)
	}
	t.GoldLastType = goldEnum
	silverEnum, err := r.EnumValues(silverLastTypeEnum)
	if err != nil {
		return nil, fmt.Errorf("crafting: enum %d (silver last type): %w", silverLastTypeEnum, err)
	}
	if len(silverEnum) == 0 {
		return nil, fmt.Errorf("crafting: enum %d (silver last type): no entries", silverLastTypeEnum)
	}
	t.SilverLastType = silverEnum

	// The jewellery varbits.
	for _, name := range loadVarbits {
		rs.varbit(name, t.VarbitIDs)
	}

	// The seq names the TS units use that the JSON never
	// numbers (human_furnace, human_limestonecutting).
	for _, name := range extraSeqNames {
		rs.seqOnly(name, t.SeqIDs)
	}

	// The ItemIdentifiers names the TS units use outside the
	// JSON (tools, bars, clay, orbs): resolved into ItemIDs so
	// the units read them through tables.ItemID.
	for _, name := range extraItemNames {
		rs.obj(name)
	}

	// Every name the resolver resolved is in rs.itemIDs.
	for name, id := range rs.itemIDs {
		t.ItemIDs[name] = id
	}

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

// ItemID returns the resolved id of an ItemIdentifiers name,
// or false when the file never names it (and no unit tool name
// resolved it either).
func (t *Tables) ItemID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.ItemIDs[name]
	return id, ok
}

// GemByUncut returns the gem recipe for an uncut gem item id,
// or nil when the item is not a cuttable gem.
func (t *Tables) GemByUncut(uncutID int) *GemRecipe {
	if t == nil {
		return nil
	}
	for i := range t.Gems {
		if t.Gems[i].UncutID == uncutID {
			return &t.Gems[i]
		}
	}
	return nil
}

// CutGem answers the "crafting:cut-gem" query: the cut gem and
// XP for an uncut gem id, or ok=false when the item is not a
// cuttable gem.
func (t *Tables) CutGem(uncutID int) (cutID int, xp float64, ok bool) {
	gem := t.GemByUncut(uncutID)
	if gem == nil {
		return 0, 0, false
	}
	return gem.CutID, gem.XP, true
}

// LeatherByMaterial returns the leather recipes for a material
// item id, in file order, or nil when the material sews
// nothing.
func (t *Tables) LeatherByMaterial(materialID int) []LeatherRecipe {
	if t == nil {
		return nil
	}
	for _, material := range t.Leather {
		if material.MaterialID == materialID {
			return material.Recipes
		}
	}
	return nil
}

// SnelmByShell returns the snelm recipe for a blamish shell
// item id, or nil.
func (t *Tables) SnelmByShell(shellID int) *SnelmRecipe {
	if t == nil {
		return nil
	}
	for i := range t.Snelms.Recipes {
		if t.Snelms.Recipes[i].ShellID == shellID {
			return &t.Snelms.Recipes[i]
		}
	}
	return nil
}

// SpinningByMaterial returns the spinning recipes that use a
// material item id, in file order, or nil.
func (t *Tables) SpinningByMaterial(materialID int) []SpinningRecipe {
	if t == nil {
		return nil
	}
	var out []SpinningRecipe
	for _, recipe := range t.Spinning.Recipes {
		for _, id := range recipe.MaterialIDs {
			if id == materialID {
				out = append(out, recipe)
				break
			}
		}
	}
	return out
}
