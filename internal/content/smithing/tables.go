// Smithing tables: the loader for data/definitions/smithing.json
// and data/definitions/smithing-anvil-layout.json (upstream
// commit bfd81e3, imported verbatim with Tyler's approval —
// SHA-256 0ce29227df26dc849d5e6ea505cf3fded7c368ca291bb71e33044d200a2ad5af
// and ab2b2222589d6bf8bc65a0326d5441b54e38e68b328d48d0334f00d3c5c91524).
//
// The JSON names items by their ItemIdentifiers names
// (UPPER_SNAKE, e.g. "BRONZE_BAR"). Where the cache has a
// gameval of the lowercase form (e.g. "BRONZE_BAR" ->
// "bronze_bar"), Load resolves through it; the 5 names with no
// (correct) gameval resolve through the itemIDFallbacks table
// in item_ids.go, transcribed from upstream
// ItemIdentifiers.ts and verified by display name against the
// real rev-242 cache. A name in neither the table nor the
// cache fails Load with every missing name listed, and no id
// is invented. Spear/hasta keys are already lowercase obj
// gamevals (Data.Smithing.js:126-135) and resolve through the
// same lookup — lowercasing them is the identity.
//
// The anvil products' level, bars used and amount made come
// from cache enums 844 (amount), 845 (bars) and 846 (level),
// config archive 8, keyed by product item id — never from the
// layout file, which holds only slots. Enum 846 stores 0 for
// exactly two products, 1205 (Bronze dagger) and 1351 (Bronze
// axe), the only level-1 anvil products: 0 is read as level 1.
// Enum 1101 (slotIndex, keyed by (interfaceId<<16)|component
// per Data.Smithing.js:62-89) is used when the rev-242 cache
// has it; when it does not, slotIndex is derived from layout
// order (1-based within the bar's slots, matching the "1-based
// slot index" the varbit smithing_item_lasttype holds).
//
// XP is float in the JSON (smelting 6.2, 12.5, …; xpPerBar
// 12.5-75) and stays float in the tables; truncation to int
// happens at the XP-grant call site (the content workers),
// the same known structural gap as woodcutting/fletching/
// cooking/mining (gap 26a). An anvil product's XP is
// productXp[item] when the file sets one (it sets none —
// productXp is {}), else xpPerBar[bar] x bars used
// (Data.Smithing.js productsByBar).
//
// The tables are read-only after Load.
package smithing

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"tsps-go/internal/cache"
)

// DefaultPath is the repo-relative path of the imported
// smithing file (data/definitions/smithing.json).
// LoadFromCache uses it when no path is given.
const DefaultPath = "data/definitions/smithing.json"

// DefaultLayoutPath is the repo-relative path of the imported
// anvil-layout file
// (data/definitions/smithing-anvil-layout.json). LoadFromCache
// uses it; LoadFile derives the layout path from its argument
// instead (same directory, same file name).
const DefaultLayoutPath = "data/definitions/smithing-anvil-layout.json"

// amountEnum, barsEnum, levelEnum and slotIndexEnum are the
// cache enums Data.Smithing.js reads (config archive 8).
const (
	amountEnum    = 844
	barsEnum      = 845
	levelEnum     = 846
	slotIndexEnum = 1101
)

// smithingInterfaceName is the cache interface name whose group
// id keys enum 1101 ((interfaceId<<16)|component).
const smithingInterfaceName = "smithing"

// ---- resolved model (frozen API, PHASE43_PLAN.md §1) ----

// Ingredient is one resolved [item, amount] pair: a smelting
// ingredient or a molten-glass byproduct.
type Ingredient struct {
	ItemID int
	Amount int
}

// SmeltRecipe is one resolved smithing.json smelting row
// (Data.Smithing.js load). BarID and Bar both hold the bar's
// resolved item id; content code should use BarID
// (SmeltByBar). Quest is the quest key ("the_knights_sword")
// or "". Skill is the skill the recipe grants ("CRAFTING" for
// molten glass) or "". HasSuccessChance marks the iron 50%
// fail; HasTiming marks a row that sets interval/firstDelay/
// finishDelay (molten glass only).
type SmeltRecipe struct {
	Name                                                     string
	BarID                                                    int
	Bar                                                      int
	Level                                                    int
	XP                                                       float64
	Ingredients                                              []Ingredient
	BarPhrase, SmeltMessage, RetrieveMessage, MissingMessage string
	Quest                                                    string
	SuccessChance                                            float64
	HasSuccessChance                                         bool
	FailMessage                                              string
	Members                                                  bool
	Furnace                                                  string
	NotInMenu                                                bool
	Skill                                                    string
	Byproducts                                               []Ingredient
	Interval, FirstDelay, FinishDelay                        int
	HasTiming                                                bool
}

// CannonballRecipe is one resolved smithing.json cannonballs
// row (Data.Smithing.js cannonballs): the ball a bar pours
// into, its level, and the XP per bar smelted.
type CannonballRecipe struct {
	BallID   int
	BarID    int
	Level    int
	XPPerBar float64
}

// BluriteProduct is one resolved smithing.json blurite row
// (Data.Smithing.js bluriteProducts): a blurite-bar anvil
// product (The Knight's Sword), made on the blurite bar —
// resolve that bar through the Blurite smelt recipe's BarID.
type BluriteProduct struct {
	ItemID int
	Level  int
	XP     float64
	Amount int
	Phrase string
}

// SpearRecipe is one resolved smithing.json spears row
// (Data.Smithing.js spears): a bar + matching log make a
// spear or a hasta at a barbarian anvil.
type SpearRecipe struct {
	BarID   int
	LogID   int
	Level   int
	XP      float64
	SpearID int
	HastaID int
}

// AnvilProduct is one resolved anvil-layout slot
// (Data.Smithing.js productsByBar): the product a bar smiths
// at an anvil, with its level/bars/amount from cache enums
// 846/845/844. BarType is the layout's bar type (1 bronze ..
// 7 lovakite); Metal is the bar's gameval without "_bar"
// ("bronze", "iron", …) for "You hammer the <metal> and
// make …"; Obj is the slot's obj gameval; Phrase is the
// objPhrases entry for it, else the phrases entry for its
// label, else "a <label>". XP is xpPerBar[bar] x bars used
// (productXp is empty, so no product overrides it).
type AnvilProduct struct {
	BarID     int
	BarType   int
	Component int
	SlotIndex int
	ItemID    int
	Level     int
	Bars      int
	Amount    int
	Metal     string
	Obj       string
	Label     string
	Phrase    string
	XP        float64
	Members   bool
	Quest     string
}

// Tables is the resolved smithing data the loader returns:
// the furnace recipes, the cannonballs, the blurite products,
// the barbarian spears, the anvil products (all 188 slots,
// in layout order), and the phrase / quest-name / xp-per-bar
// maps. See the model comment above for the loader contract.
type Tables struct {
	Smelting    []SmeltRecipe
	Cannonballs []CannonballRecipe
	Blurite     []BluriteProduct
	Spears      []SpearRecipe
	Anvil       []AnvilProduct
	Phrases     map[string]string
	ObjPhrases  map[string]string
	QuestNames  map[string]string
	XPPerBar    map[int]float64
}

// ---- raw JSON shapes ----

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

type rawSmeltRecipe struct {
	Name            string    `json:"name"`
	Bar             string    `json:"bar"`
	Level           int       `json:"level"`
	XP              float64   `json:"xp"`
	Ingredients     []rawPair `json:"ingredients"`
	BarPhrase       string    `json:"barPhrase"`
	SmeltMessage    string    `json:"smeltMessage"`
	RetrieveMessage string    `json:"retrieveMessage"`
	MissingMessage  string    `json:"missingMessage"`
	Quest           string    `json:"quest"`
	SuccessChance   *float64  `json:"successChance"`
	FailMessage     string    `json:"failMessage"`
	Members         bool      `json:"members"`
	Furnace         string    `json:"furnace"`
	NotInMenu       bool      `json:"notInMenu"`
	Skill           string    `json:"skill"`
	Byproducts      []rawPair `json:"byproducts"`
	Interval        *int      `json:"interval"`
	FirstDelay      *int      `json:"firstDelay"`
	FinishDelay     *int      `json:"finishDelay"`
}

type rawCannonball struct {
	Ball     string  `json:"ball"`
	Bar      string  `json:"bar"`
	Level    int     `json:"level"`
	XPPerBar float64 `json:"xpPerBar"`
}

type rawBlurite struct {
	Item   string  `json:"item"`
	Level  int     `json:"level"`
	XP     float64 `json:"xp"`
	Amount int     `json:"amount"`
	Phrase string  `json:"phrase"`
}

type rawSpear struct {
	Bar   string  `json:"bar"`
	Log   string  `json:"log"`
	Level int     `json:"level"`
	XP    float64 `json:"xp"`
	Spear string  `json:"spear"`
	Hasta string  `json:"hasta"`
}

type rawSmithingFile struct {
	Smelting    []rawSmeltRecipe   `json:"smelting"`
	Cannonballs []rawCannonball    `json:"cannonballs"`
	Blurite     []rawBlurite       `json:"blurite"`
	Spears      []rawSpear         `json:"spears"`
	Phrases     map[string]string  `json:"phrases"`
	ObjPhrases  map[string]string  `json:"objPhrases"`
	QuestNames  map[string]string  `json:"questNames"`
	XPPerBar    map[string]float64 `json:"xpPerBar"`
	ProductXP   map[string]float64 `json:"productXp"`
}

type rawLayoutSlot struct {
	Component int    `json:"component"`
	Item      int    `json:"item"`
	Obj       string `json:"obj"`
	Label     string `json:"label"`
	Quest     string `json:"quest"`
	Members   bool   `json:"members"`
}

type rawLayoutBar struct {
	BarType int             `json:"barType"`
	Bar     int             `json:"bar"`
	Obj     string          `json:"obj"`
	Slots   []rawLayoutSlot `json:"slots"`
}

type rawLayoutFile struct {
	Bars []rawLayoutBar `json:"bars"`
}

// SmithingData is the parsed pair of imported data files
// Load resolves: smithing.json (furnace recipes, XP, phrases)
// and smithing-anvil-layout.json (anvil slots). LoadFile
// builds it from disk; tests build it from fixture JSON.
type SmithingData struct {
	Smithing rawSmithingFile
	Layout   rawLayoutFile
}

// parseSmithingData parses the two documents' JSON bytes.
func parseSmithingData(smithingJSON, layoutJSON []byte) (*SmithingData, error) {
	var data SmithingData
	if err := json.Unmarshal(smithingJSON, &data.Smithing); err != nil {
		return nil, fmt.Errorf("smithing: parsing smithing.json: %w", err)
	}
	if err := json.Unmarshal(layoutJSON, &data.Layout); err != nil {
		return nil, fmt.Errorf("smithing: parsing smithing-anvil-layout.json: %w", err)
	}
	return &data, nil
}

// IDResolver resolves the names the JSON files use. It is the
// seam Load resolves through, so tests can drive Load from a
// hand-built fixture instead of a live cache; production
// passes LoadFromCache's adapter over *cache.CacheDefinitions.
// Item names arrive in their ItemIdentifiers (UPPER_SNAKE)
// form; the adapter lowercases them to gameval form (see
// itemGameval). Spear/hasta keys arrive already lowercase and
// pass through unchanged. Names in the itemIDFallbacks table
// never reach the resolver — the loader consults the table
// first (see resolver.obj). EnumValues answers a cache enum's
// key -> value entries (config archive 8), empty when the enum
// does not exist; ObjNames answers id -> obj gameval name;
// InterfaceID answers an interface's group id by gameval name.
type IDResolver interface {
	ObjID(name string) (int, bool, error)
	EnumValues(id int) (map[int]int, error)
	ObjNames() (map[int]string, error)
	InterfaceID(name string) (int, bool, error)
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error) {
	return r.defs.GetObjId(itemGameval(name))
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
			return nil, fmt.Errorf("smithing: enum %d key %d: non-int value %T", id, key, value)
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

// itemGameval maps an ItemIdentifiers name to its cache
// gameval obj name: the lowercase form (e.g. "BRONZE_BAR" ->
// "bronze_bar"). It only answers for the names whose gameval
// has that form; the rest resolve through itemIDFallbacks (see
// item_ids.go).
func itemGameval(name string) string {
	return strings.ToLower(name)
}

// resolver collects unresolved names so Load fails closed
// with every missing name listed, instead of stopping at the
// first — the woodcutting/fletching/cooking tables.go
// precedent.
type resolver struct {
	r       IDResolver
	missing []string
}

func (rs *resolver) obj(name string) int {
	id, _ := rs.objOK(name)
	return id
}

// objOK resolves one item name, reporting whether it
// resolved. The fallback table goes first: a name's lowercase
// form may be another item's gameval, so a gameval lookup must
// never win for it. A name in neither the table nor the cache
// is recorded missing.
func (rs *resolver) objOK(name string) (int, bool) {
	if id, ok := itemIDFallbacks[name]; ok {
		return id, true
	}
	id, ok, err := rs.r.ObjID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0, false
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
		return 0, false
	}
	return id, true
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("smithing: unresolved cache ids: %v", rs.missing)
}

// LoadFromCache resolves the Smithing tables against the real
// cache definitions and the data files at their default paths.
// A nil cache fails; a missing or malformed file fails; an
// unresolvable name fails listing every missing name.
func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("smithing: nil cache definitions")
	}
	return LoadFile(DefaultPath, cacheResolver{defs: defs})
}

// LoadFile reads the smithing file at path and the anvil-layout
// file beside it (same directory, "smithing-anvil-layout.json")
// and resolves both through r.
func LoadFile(path string, r IDResolver) (*Tables, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("smithing: reading %s: %w", path, err)
	}
	layoutPath := filepath.Join(filepath.Dir(path), "smithing-anvil-layout.json")
	layout, err := os.ReadFile(layoutPath)
	if err != nil {
		return nil, fmt.Errorf("smithing: reading %s: %w", layoutPath, err)
	}
	parsed, err := parseSmithingData(data, layout)
	if err != nil {
		return nil, err
	}
	return Load(parsed, r)
}

// Load parses the SmithingData documents in data and resolves
// every name through r, or returns an error listing every
// gameval name the cache does not know. No id is invented: an
// unresolvable name fails the load.
func Load(data *SmithingData, r IDResolver) (*Tables, error) {
	if data == nil {
		return nil, errors.New("smithing: nil smithing data")
	}
	if r == nil {
		return nil, errors.New("smithing: nil id resolver")
	}
	raw := data.Smithing
	if len(raw.Smelting) == 0 {
		return nil, errors.New("smithing: no smelting recipes in smithing.json")
	}
	if len(raw.Cannonballs) == 0 {
		return nil, errors.New("smithing: no cannonballs in smithing.json")
	}
	if len(raw.Blurite) == 0 {
		return nil, errors.New("smithing: no blurite products in smithing.json")
	}
	if len(raw.Spears) == 0 {
		return nil, errors.New("smithing: no spears in smithing.json")
	}
	if len(data.Layout.Bars) == 0 {
		return nil, errors.New("smithing: no bars in smithing-anvil-layout.json")
	}
	rs := &resolver{r: r}
	t := &Tables{
		Phrases:    make(map[string]string, len(raw.Phrases)),
		ObjPhrases: make(map[string]string, len(raw.ObjPhrases)),
		QuestNames: make(map[string]string, len(raw.QuestNames)),
		XPPerBar:   make(map[int]float64, len(raw.XPPerBar)),
	}
	for k, v := range raw.Phrases {
		t.Phrases[k] = v
	}
	for k, v := range raw.ObjPhrases {
		t.ObjPhrases[k] = v
	}
	for k, v := range raw.QuestNames {
		t.QuestNames[k] = v
	}

	// xpPerBar, keyed by resolved bar id
	// (Data.Smithing.js productsByBar).
	xpPerBar := make(map[int]float64, len(raw.XPPerBar))
	for name, xp := range raw.XPPerBar {
		id := rs.obj(name)
		xpPerBar[id] = xp
		t.XPPerBar[id] = xp
	}
	// productXp overrides, keyed by resolved product id (the
	// file sets none — productXp is {} — but a future import
	// may; TS applies them the same way).
	productXP := make(map[int]float64, len(raw.ProductXP))
	for name, xp := range raw.ProductXP {
		productXP[rs.obj(name)] = xp
	}

	// Smelting (Data.Smithing.js load): every recipe first,
	// so a bad name throws on every load, never leaving a
	// partial list.
	for i, row := range raw.Smelting {
		if row.Bar == "" {
			return nil, fmt.Errorf("smithing: smelting recipe %d has no bar", i)
		}
		recipe := SmeltRecipe{
			Name:            row.Name,
			Level:           row.Level,
			XP:              row.XP,
			BarPhrase:       row.BarPhrase,
			SmeltMessage:    row.SmeltMessage,
			RetrieveMessage: row.RetrieveMessage,
			MissingMessage:  row.MissingMessage,
			Quest:           row.Quest,
			FailMessage:     row.FailMessage,
			Members:         row.Members,
			Furnace:         row.Furnace,
			NotInMenu:       row.NotInMenu,
			Skill:           row.Skill,
		}
		recipe.BarID = rs.obj(row.Bar)
		recipe.Bar = recipe.BarID
		for _, ing := range row.Ingredients {
			if ing.Amount <= 0 {
				return nil, fmt.Errorf("smithing: smelting recipe %d ingredient %q: amount %d, want > 0", i, ing.Name, ing.Amount)
			}
			recipe.Ingredients = append(recipe.Ingredients, Ingredient{ItemID: rs.obj(ing.Name), Amount: ing.Amount})
		}
		if row.SuccessChance != nil {
			recipe.SuccessChance, recipe.HasSuccessChance = *row.SuccessChance, true
		}
		for _, by := range row.Byproducts {
			if by.Amount <= 0 {
				return nil, fmt.Errorf("smithing: smelting recipe %d byproduct %q: amount %d, want > 0", i, by.Name, by.Amount)
			}
			recipe.Byproducts = append(recipe.Byproducts, Ingredient{ItemID: rs.obj(by.Name), Amount: by.Amount})
		}
		if row.Interval != nil || row.FirstDelay != nil || row.FinishDelay != nil {
			recipe.HasTiming = true
			if row.Interval != nil {
				recipe.Interval = *row.Interval
			}
			if row.FirstDelay != nil {
				recipe.FirstDelay = *row.FirstDelay
			}
			if row.FinishDelay != nil {
				recipe.FinishDelay = *row.FinishDelay
			}
		}
		t.Smelting = append(t.Smelting, recipe)
	}

	// Cannonballs (Data.Smithing.js cannonballs).
	for i, row := range raw.Cannonballs {
		if row.Ball == "" || row.Bar == "" {
			return nil, fmt.Errorf("smithing: cannonball %d has no ball or bar", i)
		}
		t.Cannonballs = append(t.Cannonballs, CannonballRecipe{
			BallID:   rs.obj(row.Ball),
			BarID:    rs.obj(row.Bar),
			Level:    row.Level,
			XPPerBar: row.XPPerBar,
		})
	}

	// Blurite (Data.Smithing.js bluriteProducts).
	for i, row := range raw.Blurite {
		if row.Item == "" {
			return nil, fmt.Errorf("smithing: blurite product %d has no item", i)
		}
		t.Blurite = append(t.Blurite, BluriteProduct{
			ItemID: rs.obj(row.Item),
			Level:  row.Level,
			XP:     row.XP,
			Amount: row.Amount,
			Phrase: row.Phrase,
		})
	}

	// Spears (Data.Smithing.js spears): bar and log are
	// ItemIdentifiers names; spear and hasta are lowercase
	// obj gamevals.
	for i, row := range raw.Spears {
		if row.Bar == "" || row.Log == "" || row.Spear == "" || row.Hasta == "" {
			return nil, fmt.Errorf("smithing: spear %d has no bar, log, spear or hasta", i)
		}
		t.Spears = append(t.Spears, SpearRecipe{
			BarID:   rs.obj(row.Bar),
			LogID:   rs.obj(row.Log),
			Level:   row.Level,
			XP:      row.XP,
			SpearID: rs.obj(row.Spear),
			HastaID: rs.obj(row.Hasta),
		})
	}

	// Anvil products (Data.Smithing.js productsByBar): the
	// enums first — a cache read failure fails the load.
	amounts, err := r.EnumValues(amountEnum)
	if err != nil {
		return nil, fmt.Errorf("smithing: enum %d (amount): %w", amountEnum, err)
	}
	barsUsed, err := r.EnumValues(barsEnum)
	if err != nil {
		return nil, fmt.Errorf("smithing: enum %d (bars): %w", barsEnum, err)
	}
	levels, err := r.EnumValues(levelEnum)
	if err != nil {
		return nil, fmt.Errorf("smithing: enum %d (level): %w", levelEnum, err)
	}
	objNames, err := r.ObjNames()
	if err != nil {
		return nil, fmt.Errorf("smithing: obj names: %w", err)
	}
	// Slot indexes come from enum 1101 when the cache has it
	// (keyed by (interfaceId<<16)|component); otherwise they
	// are derived from layout order (see the file header).
	ifaceID := 0
	hasSlotIndex := false
	slotIndex := map[int]int{}
	if id, ok, err := r.InterfaceID(smithingInterfaceName); err == nil && ok {
		if values, err := r.EnumValues(slotIndexEnum); err == nil && len(values) > 0 {
			ifaceID, slotIndex, hasSlotIndex = id, values, true
		}
	}
	for _, bar := range data.Layout.Bars {
		if len(bar.Slots) == 0 {
			return nil, fmt.Errorf("smithing: layout bar %d has no slots", bar.Bar)
		}
		// The metal in "You hammer the <metal> and make …":
		// the bar's gameval without "_bar".
		metal := strings.TrimSuffix(objNames[bar.Bar], "_bar")
		for i, slot := range bar.Slots {
			if slot.Obj == "" {
				return nil, fmt.Errorf("smithing: layout bar %d slot %d has no obj", bar.Bar, i)
			}
			// The layout's item id must be exactly the id its
			// obj gameval resolves to — the probe verified
			// 188/188 with 0 mismatches; a mismatch is a data
			// error, not a fallback.
			id, ok := rs.objOK(slot.Obj)
			if ok && id != slot.Item {
				return nil, fmt.Errorf("smithing: layout bar %d slot %q: obj resolves to %d, want layout item %d", bar.Bar, slot.Obj, id, slot.Item)
			}
			level, ok := levels[slot.Item]
			if !ok {
				return nil, fmt.Errorf("smithing: enum %d has no level for item %d (%s)", levelEnum, slot.Item, slot.Obj)
			}
			// Enum 846 stores 0 for exactly the level-1
			// products (1205 Bronze dagger, 1351 Bronze axe):
			// 0 means level 1.
			if level == 0 {
				level = 1
			}
			bars, ok := barsUsed[slot.Item]
			if !ok {
				bars = 1 // TS: bars.get(slot.item) ?? 1
			}
			amount, ok := amounts[slot.Item]
			if !ok {
				amount = 1 // TS: amounts.get(slot.item) ?? 1
			}
			index, ok := slotIndex[(ifaceID<<16)|slot.Component]
			if !hasSlotIndex {
				// Enum 1101 absent: 1-based layout order
				// within the bar's slots (TS: ?? 0 is the
				// enum path's fallback; the derivation path
				// counts slots from 1, as the varbit holds a
				// 1-based slot index).
				index = i + 1
			}
			xp, ok := productXP[slot.Item]
			if !ok {
				xp = xpPerBar[bar.Bar] * float64(bars)
			}
			t.Anvil = append(t.Anvil, AnvilProduct{
				BarID:     bar.Bar,
				BarType:   bar.BarType,
				Component: slot.Component,
				SlotIndex: index,
				ItemID:    slot.Item,
				Level:     level,
				Bars:      bars,
				Amount:    amount,
				Metal:     metal,
				Obj:       slot.Obj,
				Label:     slot.Label,
				Phrase:    phraseFor(raw, slot),
				XP:        xp,
				Members:   slot.Members,
				Quest:     slot.Quest,
			})
		}
	}

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

// phraseLabelSuffix strips a trailing " (N)" tier from a slot
// label (Data.Smithing.js: label.replace(/ \(\d\)$/, "")).
var phraseLabelSuffix = regexp.MustCompile(` \(\d\)$`)

// phraseFor resolves an anvil slot's "make" phrase: the
// objPhrases entry for its obj gameval wins, else the phrases
// entry for its label (tier suffix stripped), else
// "a <label>" (Data.Smithing.js productsByBar).
func phraseFor(raw rawSmithingFile, slot rawLayoutSlot) string {
	if phrase, ok := raw.ObjPhrases[slot.Obj]; ok {
		return phrase
	}
	if phrase, ok := raw.Phrases[phraseLabelSuffix.ReplaceAllString(slot.Label, "")]; ok {
		return phrase
	}
	return "a " + strings.ToLower(slot.Label)
}

// SmeltByBar returns the smelting recipe for a bar item id, or
// nil when no recipe smelts that bar.
func (t *Tables) SmeltByBar(barID int) *SmeltRecipe {
	if t == nil {
		return nil
	}
	for i := range t.Smelting {
		if t.Smelting[i].BarID == barID {
			return &t.Smelting[i]
		}
	}
	return nil
}

// AnvilByBar returns the anvil products for a bar item id, in
// layout order, or nil when the bar has none.
func (t *Tables) AnvilByBar(barID int) []AnvilProduct {
	if t == nil {
		return nil
	}
	var out []AnvilProduct
	for _, product := range t.Anvil {
		if product.BarID == barID {
			out = append(out, product)
		}
	}
	return out
}

// AnvilByComponent returns the anvil product for a bar type
// and interface component, or nil when there is none.
func (t *Tables) AnvilByComponent(barType, component int) *AnvilProduct {
	if t == nil {
		return nil
	}
	for i := range t.Anvil {
		if t.Anvil[i].BarType == barType && t.Anvil[i].Component == component {
			return &t.Anvil[i]
		}
	}
	return nil
}

// ---- package state (frozen API) ----

// tables is the installed resolved tables, or nil when the
// server runs without them (NoCache mode). Content handlers
// no-op when it is nil.
var tables *Tables

// SetTables installs the resolved tables. cmd/server calls it
// BEFORE hub Load: Register builds indexes from the tables, so
// a call after Load is too late for them.
func SetTables(t *Tables) { tables = t }

// TablesLoaded reports whether the tables are installed.
func TablesLoaded() bool { return tables != nil }
