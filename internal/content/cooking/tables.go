// Tables + loader: the resolved form of data/definitions/cooking.json
// (upstream commit bfd81e3, imported verbatim with Tyler's approval —
// SHA-256 5760fe60c7d58d140ed5fb6ea3168c72c2ae02640702d2a66a369156b4ff5788).
//
// The JSON names items by their cache gameval names (lowercase, e.g.
// "raw_shrimp"), and a few items by numeric id where the cache names
// them ambiguously (Common.Cooking.js:29-30: obj() accepts a numeric
// id). Load resolves every name through the IDResolver — production
// passes LoadFromCache's adapter over *cache.CacheDefinitions —
// and fails closed listing every name the cache does not know. No id
// is invented. The one exception is the knife names and the two
// varbits Common/Cookers resolve by name at call time (Common
// .Cooking.js knives(), sendVarbit): TS filters a missing knife
// out and skips an unresolvable varbit, so the loader resolves
// those leniently instead of failing.
//
// First-wins rule for duplicate raw ids (Common.Cooking.js:39-41);
// RecipesByProduct is last-wins, matching TS Map.set in index()
// (Common.Cooking.js:49-57) — the meat-pie rows, for example, share
// a product and the later row wins the product lookup, while both
// rows stay reachable through RecipesByIngredient.
//
// XP is float in the JSON (82.5, 211.3, 40.5, …) but Go stores
// integer experience and SkillManager.AddExperiences takes an int;
// the grant sites truncate at the call (common.go addXP), the same
// known structural gap as woodcutting/fletching. No float XP storage
// is invented here.
//
// The tables are read-only after Load.
package cooking

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"tsps-go/internal/cache"
)

// DefaultPath is the repo-relative path of the imported cooking
// file (data/definitions/cooking.json). LoadFromCache uses it when
// no path is given.
const DefaultPath = "data/definitions/cooking.json"

// ---- resolved model ----
// These are the Go form of index()'s output (Common.Cooking.js:
// 35-76): every name already turned into an id by the loader.

// Ingredient is one resolved recipe ingredient or return
// (Common.Cooking.js ingredient, lines 31-33): a bare name reads
// count 1, a [name, count] pair reads its count.
type Ingredient struct {
	ID    int
	Count int
}

// Cookable is one resolved cooking.json cookables row
// (Common.Cooking.js cookables(), lines 38-44): the raw item, what
// it cooks and burns into, its level, XP and burn chart, and the
// row's optional flags. Chance keys are the Wiki chart names
// ("fire", "range", "lumbridge", "gauntlets"); a row without a
// chart has StopBurn instead (HasStopBurn false when the row sets
// neither — no row does that, but the loader does not invent one).
type Cookable struct {
	RawID    int
	CookedID int
	BurntID  int
	Level    int
	// XP is the row's XP (float in the JSON; truncated at the
	// grant site — see the file header).
	XP float64
	// Chance maps a cooker kind to its [low, high] Wiki values
	// out of 256. Nil when the row sets no chance chart.
	Chance map[string][2]int
	// StopBurn is the level the food stops burning at, when the
	// row sets one; HasStopBurn marks it.
	StopBurn    int
	HasStopBurn bool
	Name        string
	Article     string
	Message     string
	BurnMessage string
	// Facility is the preparation facility a facility-only food
	// cooks at ("camdozaal_preparation_table"); "" when the row
	// sets none. Cookers must NOT cook a facility food.
	Facility string
	// DuringQuest is the quest key a food needs started (only
	// tbwt_raw_karambwanji → "tai_bwo_wannai_trio"); "" otherwise.
	DuringQuest string
	// Rumours is the hunter-rumours threshold a food needs
	// (25 or 50); HasRumours marks it.
	Rumours    int
	HasRumours bool
	// FailsWithoutBurning is the drunk-dragon flag (9576): the
	// food fails without burning and pays no XP.
	FailsWithoutBurning bool
	// ReturnItemID is the container returned on a cook
	// (crunchy_tray, batta_tin, cake_tin); HasReturnItem marks it.
	ReturnItemID  int
	HasReturnItem bool
	// ReturnOnBurn returns the container even when the food
	// burns (only uncooked_cake).
	ReturnOnBurn bool
	// FirstCookTicks is the first-cook delay override (9576 → 1,
	// raw_gnomebowl / 9563 → 4); HasFirstCookTicks marks it.
	FirstCookTicks    int
	HasFirstCookTicks bool
	// PreparationStopBurn is the preparation-table stop-burn
	// level (36 / 45 / 54 / 70 for the facility fish);
	// HasPreparationStopBurn marks it.
	PreparationStopBurn    int
	HasPreparationStopBurn bool
	// RangeOnly foods cook on a range, never on a fire.
	RangeOnly bool
	// GnomeTutorial foods are the gnome-tutorial half-made items.
	GnomeTutorial bool
}

// Recipe is one resolved cooking.json recipes row
// (Common.Cooking.js index(), lines 46-57): the product, its
// level, XP and message, its ingredients and returns, and its
// timing (default [1, 2], Common.Cooking.js:52-53). Members is
// the cache item's isMembers flag, resolved at load
// (Recipes.Cooking.js isMembersRecipe); MembersOverride is the
// row's own `members` field, nil in the imported file (no row
// sets one) — it wins over the cache when set.
type Recipe struct {
	ProductID int
	Level     int
	// XP is the row's XP (float in the JSON; 0 when the row sets
	// none — Common.Cooking.js:52 `xp ?? 0`).
	XP          float64
	Message     string
	Ingredients []Ingredient
	Returns     []Ingredient
	// Timing is the ticks before each item; the last value
	// repeats (Recipes.Cooking.js start). Usually [1, 2];
	// dough recipes use [1, 1, 2] (DOUGH_TIMING).
	Timing          []int
	Members         bool
	MembersOverride *bool
}

// DoughWater is one doughs waters pair (Common.Cooking.js:68):
// the water item and the empty container it leaves behind.
type DoughWater struct {
	ID      int
	EmptyID int
}

// DoughProduct is one doughs products row (Common.Cooking.js:70):
// the dough, its level and its mix message. Members is the cache
// item's isMembers flag, resolved at load.
type DoughProduct struct {
	ProductID int
	Level     int
	Message   string
	Members   bool
}

// DoughData is the resolved doughs section (Common.Cooking.js:
// 66-72): the flour and its pot, the waters, and the four doughs.
type DoughData struct {
	FlourID  int
	PotID    int
	Waters   []DoughWater
	Products []DoughProduct
}

// Wine is one resolved cooking.json wines row
// (Common.Cooking.js wineData(), lines 73-76): the grapes, the
// unfermented jug, the wine and the bad wine, its level, XP and
// [low, high] chance, and its pending limit when the row sets
// one (grapes only, 60000).
type Wine struct {
	GrapesID        int
	UnfermentedID   int
	WineID          int
	BadID           int
	Level           int
	XP              float64
	Chance          [2]int
	PendingLimit    int
	HasPendingLimit bool
}

// ChurnInput is one resolved churn inputs row (Churn.Cooking.js
// churnData): the input item, its display name ("butter" /
// "cream" / "milk"), and the container it returns when the row
// sets one (bucket_milk → bucket_empty).
type ChurnInput struct {
	InputID   int
	Name      string
	ReturnID  int
	HasReturn bool
}

// ChurnProduct is one resolved churn products row: the input it
// churns, the product, its display name, level, XP (float in
// the JSON; truncated at the grant site) and ticks, and the
// churn animation seq id.
type ChurnProduct struct {
	InputID   int
	ProductID int
	Name      string
	Level     int
	XP        float64
	Ticks     int
	SeqID     int
}

// ChurnData is the resolved churn section (Churn.Cooking.js
// churnData): the labels (product gameval name → menu label),
// the inputs, and the products. LabelsByProductID is the same
// labels map keyed by resolved product id, for the menu.
type ChurnData struct {
	Labels            map[string]string
	LabelsByProductID map[int]string
	Inputs            []ChurnInput
	Products          []ChurnProduct
}

// Tables is the resolved cooking data the loader returns: the
// cookables by raw id, the recipes by ingredient and by product,
// the doughs, the wines and the churn data, plus the id maps the
// units resolve outside the tables. See the model comment above
// for the loader contract.
type Tables struct {
	// CookablesByRaw maps a raw item id to its cookable row;
	// first row wins per raw id (Common.Cooking.js:39-41).
	CookablesByRaw map[int]*Cookable
	// RecipesByIngredient maps an ingredient id to the recipes
	// that use it, in data order (Common.Cooking.js:53-56).
	RecipesByIngredient map[int][]*Recipe
	// RecipesByProduct maps a product id to its recipe;
	// last row wins (Common.Cooking.js:52, Map.set).
	RecipesByProduct map[int]*Recipe
	Doughs           *DoughData
	Wines            []*Wine
	Churn            *ChurnData

	// ItemIDs maps every gameval item name the file uses to its
	// resolved id.
	ItemIDs map[string]int
	// SeqIDs maps every animation gameval name the file uses
	// (the churn seqs) to its resolved seq id.
	SeqIDs map[string]int
	// VarbitIDs maps every varbit name the loader resolves
	// (Common.sendVarbit's call-time names, resolved once at
	// load — see loadVarbits) to its id. A name the cache does
	// not know is absent, and the call site skips it, as TS
	// does (Common.Cooking.js:158-161).
	VarbitIDs map[string]int
	// KnifeIDs are the KNIFE_NAMES ids the cache knows
	// (Common.Cooking.js knives(): missing names filtered out).
	KnifeIDs []int
}

// ---- raw JSON shapes (data/cooking.json) ----

// rawID is a gameval name or a numeric item id
// (Common.Cooking.js obj(), lines 29-30).
type rawID struct {
	Name    string
	ID      int
	Numeric bool
}

// UnmarshalJSON parses a string name or a numeric id.
func (r *rawID) UnmarshalJSON(data []byte) error {
	var id int
	if err := json.Unmarshal(data, &id); err == nil {
		r.ID, r.Numeric = id, true
		return nil
	}
	var name string
	if err := json.Unmarshal(data, &name); err != nil {
		return fmt.Errorf("item id: %w", err)
	}
	r.Name = name
	return nil
}

// rawIngredient is one ingredients/returns entry: a bare name
// (count 1) or a [name, count] pair (Common.Cooking.js
// ingredient, lines 31-33).
type rawIngredient struct {
	Item  rawID
	Count int
}

// UnmarshalJSON parses a string/number or a [name, count] pair.
func (r *rawIngredient) UnmarshalJSON(data []byte) error {
	var single rawID
	if err := json.Unmarshal(data, &single); err == nil {
		r.Item, r.Count = single, 1
		return nil
	}
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[0], &r.Item); err != nil {
		return fmt.Errorf("ingredient name: %w", err)
	}
	if err := json.Unmarshal(pair[1], &r.Count); err != nil {
		return fmt.Errorf("ingredient count: %w", err)
	}
	return nil
}

type rawCookable struct {
	Raw                 rawID             `json:"raw"`
	Cooked              rawID             `json:"cooked"`
	Burnt               rawID             `json:"burnt"`
	Level               int               `json:"level"`
	XP                  float64           `json:"xp"`
	Chance              map[string][2]int `json:"chance"`
	StopBurn            *int              `json:"stopBurn"`
	Name                string            `json:"name"`
	Article             string            `json:"article"`
	Message             string            `json:"message"`
	BurnMessage         string            `json:"burnMessage"`
	Facility            string            `json:"facility"`
	DuringQuest         string            `json:"duringQuest"`
	Rumours             *int              `json:"rumours"`
	FailsWithoutBurning bool              `json:"failsWithoutBurning"`
	ReturnItem          *rawID            `json:"returnItem"`
	ReturnOnBurn        bool              `json:"returnOnBurn"`
	FirstCookTicks      *int              `json:"firstCookTicks"`
	PreparationStopBurn *int              `json:"preparationStopBurn"`
	RangeOnly           bool              `json:"rangeOnly"`
	GnomeTutorial       bool              `json:"gnomeTutorial"`
}

type rawRecipe struct {
	Product     rawID           `json:"product"`
	Level       int             `json:"level"`
	XP          *float64        `json:"xp"`
	Message     string          `json:"message"`
	Ingredients []rawIngredient `json:"ingredients"`
	Returns     []rawIngredient `json:"returns"`
	Timing      []int           `json:"timing"`
	Members     *bool           `json:"members"`
}

type rawDoughProduct struct {
	Product rawID  `json:"product"`
	Level   int    `json:"level"`
	Message string `json:"message"`
}

type rawDoughs struct {
	Flour    [2]string         `json:"flour"`
	Waters   [][2]string       `json:"waters"`
	Products []rawDoughProduct `json:"products"`
}

type rawWine struct {
	Grapes       string  `json:"grapes"`
	Unfermented  string  `json:"unfermented"`
	Wine         string  `json:"wine"`
	Bad          string  `json:"bad"`
	Level        int     `json:"level"`
	XP           float64 `json:"xp"`
	Chance       [2]int  `json:"chance"`
	PendingLimit *int    `json:"pendingLimit"`
}

type rawChurnInput struct {
	Input   string  `json:"input"`
	Name    string  `json:"name"`
	Returns *string `json:"returns"`
}

type rawChurnProduct struct {
	Input   string  `json:"input"`
	Product string  `json:"product"`
	Name    string  `json:"name"`
	Level   int     `json:"level"`
	XP      float64 `json:"xp"`
	Ticks   int     `json:"ticks"`
	Seq     string  `json:"seq"`
}

type rawChurn struct {
	Labels   map[string]string `json:"labels"`
	Inputs   []rawChurnInput   `json:"inputs"`
	Products []rawChurnProduct `json:"products"`
}

type rawFile struct {
	Cookables []rawCookable `json:"cookables"`
	Doughs    rawDoughs     `json:"doughs"`
	Recipes   []rawRecipe   `json:"recipes"`
	Wines     []rawWine     `json:"wines"`
	Churn     rawChurn      `json:"churn"`
}

// IDResolver resolves the cache gameval names the JSON is
// written in. It is the seam Load resolves through, so tests
// can drive Load from a hand-built fixture instead of a live
// cache; production passes LoadFromCache's adapter over
// *cache.CacheDefinitions. IsMembersItem answers the cache
// item's members flag (Recipes.Cooking.js isMembersRecipe
// reads CacheDefinitions.getItem(id).isMembers).
type IDResolver interface {
	ObjID(name string) (int, bool, error)
	SeqID(name string) (int, bool, error)
	VarbitID(name string) (int, bool, error)
	IsMembersItem(id int) bool
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error) {
	return r.defs.GetObjId(name)
}
func (r cacheResolver) SeqID(name string) (int, bool, error) { return r.defs.GetSeqId(name) }
func (r cacheResolver) VarbitID(name string) (int, bool, error) {
	return r.defs.GetVarbitId(name)
}
func (r cacheResolver) IsMembersItem(id int) bool {
	if r.defs == nil {
		return false
	}
	if item := r.defs.GetItem(id); item != nil {
		return item.IsMembers
	}
	return false
}

// resolver collects unresolved names so Load fails closed
// with every missing name listed, instead of stopping at the
// first — the woodcutting/fletching tables.go precedent. It
// also records every name that resolves, into itemIDs, so
// Tables.ItemIDs is filled as a side effect of resolution.
type resolver struct {
	r       IDResolver
	missing []string
	itemIDs map[string]int
}

func (rs *resolver) objName(name string) int {
	id, ok, err := rs.r.ObjID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("obj %q", name))
		return 0
	}
	if rs.itemIDs == nil {
		rs.itemIDs = make(map[string]int)
	}
	rs.itemIDs[name] = id
	return id
}

// obj resolves a rawID: a numeric id passes through
// (Common.Cooking.js:30), a name resolves through the cache.
func (rs *resolver) obj(id rawID) int {
	if id.Numeric {
		return id.ID
	}
	return rs.objName(id.Name)
}

// tryObj resolves a name leniently, for the knife names
// (Common.Cooking.js knives() filters missing names out
// instead of failing): it reports ok=false and records
// nothing when the cache does not know the name.
func (rs *resolver) tryObj(name string) (int, bool) {
	id, ok, err := rs.r.ObjID(name)
	if err != nil || !ok {
		return 0, false
	}
	return id, true
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

// tryVarbit resolves a varbit name leniently (Common.Cooking.js
// sendVarbit skips an unresolvable name): ok=false records
// nothing when the cache does not know it.
func (rs *resolver) tryVarbit(name string) (int, bool) {
	id, ok, err := rs.r.VarbitID(name)
	if err != nil || !ok {
		return 0, false
	}
	return id, true
}

func (rs *resolver) ingredient(entry rawIngredient) Ingredient {
	return Ingredient{ID: rs.obj(entry.Item), Count: entry.Count}
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("cooking: unresolved cache ids: %v", rs.missing)
}

// loadVarbits is the varbit-name set Common.sendVarbit and the
// Cookers unit resolve by gameval name at call time
// (Common.Cooking.js:158-161, Cookers.Cooking.js
// SUCCESS_COUNT_VARBIT / BUSY_VARBIT): TS resolves the name on
// every send and skips the send when it does not resolve, so
// Go resolves the names once at load and the call site skips
// an absent entry — the same observable behavior. A name the
// cache does not know is NOT a load failure.
var loadVarbits = []string{"cooking_success_count", "busy"}

// LoadFromCache resolves the Cooking tables against the real
// cache definitions and the recipe file at path (DefaultPath
// when path is ""). A nil cache fails; a missing or malformed
// file fails; an unresolvable name fails listing every missing
// name.
func LoadFromCache(defs *cache.CacheDefinitions, path string) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("cooking: nil cache definitions")
	}
	if path == "" {
		path = DefaultPath
	}
	return LoadFile(path, cacheResolver{defs: defs})
}

// LoadFile reads the cooking file at path and resolves it
// through r.
func LoadFile(path string, r IDResolver) (*Tables, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cooking: reading %s: %w", path, err)
	}
	return Load(data, r)
}

// Load parses the cooking.json document in data and resolves
// every name through r, or returns an error listing every
// gameval name the cache does not know. No id is invented: an
// unresolvable name fails the load.
func Load(data []byte, r IDResolver) (*Tables, error) {
	if r == nil {
		return nil, errors.New("cooking: nil id resolver")
	}
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("cooking: parsing cooking.json: %w", err)
	}
	if len(raw.Cookables) == 0 {
		return nil, errors.New("cooking: no cookables in cooking.json")
	}
	if len(raw.Recipes) == 0 {
		return nil, errors.New("cooking: no recipes in cooking.json")
	}
	rs := &resolver{r: r}
	t := &Tables{
		CookablesByRaw:      make(map[int]*Cookable, len(raw.Cookables)),
		RecipesByIngredient: make(map[int][]*Recipe),
		RecipesByProduct:    make(map[int]*Recipe, len(raw.Recipes)),
		ItemIDs:             make(map[string]int),
		SeqIDs:              make(map[string]int),
		VarbitIDs:           make(map[string]int),
	}

	// Cookables: first row wins per raw id
	// (Common.Cooking.js:39-41).
	for _, row := range raw.Cookables {
		cookable := &Cookable{
			RawID:               rs.obj(row.Raw),
			CookedID:            rs.obj(row.Cooked),
			BurntID:             rs.obj(row.Burnt),
			Level:               row.Level,
			XP:                  row.XP,
			Chance:              row.Chance,
			Name:                row.Name,
			Article:             row.Article,
			Message:             row.Message,
			BurnMessage:         row.BurnMessage,
			Facility:            row.Facility,
			DuringQuest:         row.DuringQuest,
			FailsWithoutBurning: row.FailsWithoutBurning,
			ReturnOnBurn:        row.ReturnOnBurn,
			RangeOnly:           row.RangeOnly,
			GnomeTutorial:       row.GnomeTutorial,
		}
		if row.StopBurn != nil {
			cookable.StopBurn, cookable.HasStopBurn = *row.StopBurn, true
		}
		if row.Rumours != nil {
			cookable.Rumours, cookable.HasRumours = *row.Rumours, true
		}
		if row.ReturnItem != nil {
			cookable.ReturnItemID, cookable.HasReturnItem = rs.obj(*row.ReturnItem), true
		}
		if row.FirstCookTicks != nil {
			cookable.FirstCookTicks, cookable.HasFirstCookTicks = *row.FirstCookTicks, true
		}
		if row.PreparationStopBurn != nil {
			cookable.PreparationStopBurn, cookable.HasPreparationStopBurn = *row.PreparationStopBurn, true
		}
		if _, ok := t.CookablesByRaw[cookable.RawID]; !ok {
			t.CookablesByRaw[cookable.RawID] = cookable
		}
	}

	// Recipes: xp ?? 0, timing ?? [1, 2]
	// (Common.Cooking.js:52-53). RecipesByProduct is
	// last-wins (Map.set); RecipesByIngredient appends in
	// data order.
	for _, row := range raw.Recipes {
		recipe := &Recipe{
			ProductID:       rs.obj(row.Product),
			Level:           row.Level,
			Message:         row.Message,
			MembersOverride: row.Members,
		}
		if row.XP != nil {
			recipe.XP = *row.XP
		}
		recipe.Timing = []int{1, 2}
		if len(row.Timing) > 0 {
			recipe.Timing = append([]int(nil), row.Timing...)
		}
		for _, ing := range row.Ingredients {
			recipe.Ingredients = append(recipe.Ingredients, rs.ingredient(ing))
		}
		for _, ret := range row.Returns {
			recipe.Returns = append(recipe.Returns, rs.ingredient(ret))
		}
		recipe.Members = r.IsMembersItem(recipe.ProductID)
		t.RecipesByProduct[recipe.ProductID] = recipe
		for _, ing := range recipe.Ingredients {
			t.RecipesByIngredient[ing.ID] = append(t.RecipesByIngredient[ing.ID], recipe)
		}
	}

	// Doughs (Common.Cooking.js:66-72).
	doughs := &DoughData{
		FlourID: rs.objName(raw.Doughs.Flour[0]),
		PotID:   rs.objName(raw.Doughs.Flour[1]),
	}
	t.ItemIDs[raw.Doughs.Flour[0]] = doughs.FlourID
	t.ItemIDs[raw.Doughs.Flour[1]] = doughs.PotID
	for _, pair := range raw.Doughs.Waters {
		water := DoughWater{ID: rs.objName(pair[0]), EmptyID: rs.objName(pair[1])}
		t.ItemIDs[pair[0]] = water.ID
		t.ItemIDs[pair[1]] = water.EmptyID
		doughs.Waters = append(doughs.Waters, water)
	}
	for _, row := range raw.Doughs.Products {
		product := DoughProduct{
			ProductID: rs.obj(row.Product),
			Level:     row.Level,
			Message:   row.Message,
		}
		product.Members = r.IsMembersItem(product.ProductID)
		doughs.Products = append(doughs.Products, product)
	}
	t.Doughs = doughs

	// Wines (Common.Cooking.js:73-76).
	for _, row := range raw.Wines {
		wine := &Wine{
			GrapesID:      rs.objName(row.Grapes),
			UnfermentedID: rs.objName(row.Unfermented),
			WineID:        rs.objName(row.Wine),
			BadID:         rs.objName(row.Bad),
			Level:         row.Level,
			XP:            row.XP,
			Chance:        row.Chance,
		}
		if row.PendingLimit != nil {
			wine.PendingLimit, wine.HasPendingLimit = *row.PendingLimit, true
		}
		t.ItemIDs[row.Grapes] = wine.GrapesID
		t.ItemIDs[row.Unfermented] = wine.UnfermentedID
		t.ItemIDs[row.Wine] = wine.WineID
		t.ItemIDs[row.Bad] = wine.BadID
		t.Wines = append(t.Wines, wine)
	}

	// Churn (Churn.Cooking.js churnData).
	churn := &ChurnData{
		Labels:            make(map[string]string, len(raw.Churn.Labels)),
		LabelsByProductID: make(map[int]string, len(raw.Churn.Labels)),
	}
	for name, label := range raw.Churn.Labels {
		churn.Labels[name] = label
	}
	for _, row := range raw.Churn.Inputs {
		input := ChurnInput{
			InputID: rs.objName(row.Input),
			Name:    row.Name,
		}
		if row.Returns != nil {
			input.ReturnID, input.HasReturn = rs.objName(*row.Returns), true
			t.ItemIDs[*row.Returns] = input.ReturnID
		}
		t.ItemIDs[row.Input] = input.InputID
		churn.Inputs = append(churn.Inputs, input)
	}
	for _, row := range raw.Churn.Products {
		product := ChurnProduct{
			InputID:   rs.objName(row.Input),
			ProductID: rs.objName(row.Product),
			Name:      row.Name,
			Level:     row.Level,
			XP:        row.XP,
			Ticks:     row.Ticks,
			SeqID:     rs.seq(row.Seq),
		}
		t.ItemIDs[row.Input] = product.InputID
		t.ItemIDs[row.Product] = product.ProductID
		t.SeqIDs[row.Seq] = product.SeqID
		churn.Products = append(churn.Products, product)
	}
	for name, label := range churn.Labels {
		if id, ok := t.ItemIDs[name]; ok {
			churn.LabelsByProductID[id] = label
		}
	}
	t.Churn = churn

	// Knives (Common.Cooking.js knives()): the ItemIds names
	// resolve through the knifeIDFallbacks table first, then
	// the cache gamevals (lowercase form — "KNIFE" is the
	// "knife" gameval, 946); a name in neither is filtered
	// out, never a load failure.
	for _, name := range knifeNames {
		if id, ok := knifeIDFallbacks[name]; ok {
			t.KnifeIDs = append(t.KnifeIDs, id)
			t.ItemIDs[name] = id
			continue
		}
		if id, ok := rs.tryObj(strings.ToLower(name)); ok {
			t.KnifeIDs = append(t.KnifeIDs, id)
			t.ItemIDs[name] = id
		}
	}

	// Varbits (Common.Cooking.js sendVarbit): unresolvable
	// names are skipped at the call site, never a load failure.
	for _, name := range loadVarbits {
		if id, ok := rs.tryVarbit(name); ok {
			t.VarbitIDs[name] = id
		}
	}

	// Every name the resolver resolved is in rs.itemIDs; the
	// explicit per-section assignments above agree with it.
	for name, id := range rs.itemIDs {
		t.ItemIDs[name] = id
	}

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

// ItemID returns the resolved id of a gameval item name, or
// false when the file never names it.
func (t *Tables) ItemID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.ItemIDs[name]
	return id, ok
}

// CookableFor returns the cookable row for a raw item id, or
// nil when the item is not cookable.
func (t *Tables) CookableFor(rawID int) *Cookable {
	if t == nil {
		return nil
	}
	return t.CookablesByRaw[rawID]
}

// RecipeForProduct returns the recipe that makes a product id
// (last row wins), or nil.
func (t *Tables) RecipeForProduct(productID int) *Recipe {
	if t == nil {
		return nil
	}
	return t.RecipesByProduct[productID]
}

// RecipesForIngredient returns the recipes that use an
// ingredient id, in data order, or nil.
func (t *Tables) RecipesForIngredient(ingredientID int) []*Recipe {
	if t == nil {
		return nil
	}
	return t.RecipesByIngredient[ingredientID]
}
