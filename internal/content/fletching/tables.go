// Fletching tables: the loader for data/definitions/fletching.json
// (upstream commit bfd81e3, imported verbatim with Tyler's
// approval — SHA-256
// c9f64bea417ecd909c04dd3d9633e052f4c295ad91cd9fdc6cbf0fbe6baeaea9).
//
// The resolved model (Recipe, Material, Tables) lives in
// shared.go (Worker C); this file holds the raw JSON structs
// and Load / LoadFromCache, which construct those types — see
// the loader contract in shared.go's model comment.
//
// The JSON names items by their ItemIdentifiers names
// (UPPER_SNAKE, e.g. "ARROW_SHAFT"). Where the cache has a
// gameval of the lowercase form (e.g. "LOGS" -> "logs"),
// Load resolves through it; 102 of the 277 names the file
// uses resolve that way. The other 175 have no (correct)
// gameval — the cache gamevals are unrelated Jagex names
// ("SHORTBOW_U_" is "unstrung_shortbow", "BRONZE_BOLTS" is
// "bolt"), and 3 lowercase names are another item's gameval
// entirely (OPAL_BOLTS/PEARL_BOLTS/BARBED_BOLTS) — so those
// resolve through the itemIDFallbacks table in item_ids.go,
// transcribed from upstream ItemIdentifiers.ts and verified
// by display name against the real rev-242 cache. A name in
// neither the table nor the cache fails Load with every
// missing name listed, and no id is invented. Seq names in
// the JSON are already gameval names; varbit names likewise;
// sound names resolve through the game sound-name table
// (internal/game/sounds.go).
//
// Group defaults are filled in, then entry overrides, exactly
// as Shared.Fletching.js resolveRecipe does
// (Shared.Fletching.js:112-140): ticks ?? 2, firstTick ?? 1,
// startTick undefined → 0 / null → none, animation false →
// none else name or "human_fletching", maxSets ?? 28
// (CREATION_ALL_SENTINEL), menu ?? "make".
//
// XP is float in the JSON (16.5, 33.3, 187.5, …) but Go stores
// integer experience and SkillManager.AddExperiences takes an
// int; the Shared loop truncates at the call site (shared.go
// makeBatch: int(xp)), the same known structural gap as
// woodcutting (woodcutting.go chopSuccess: int(xp); a 37.5-XP
// log grants 37). No float XP storage is invented here.
//
// The tables are read-only after Load.
package fletching

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
)

// DefaultPath is the repo-relative path of the imported recipe
// file (data/definitions/fletching.json). LoadFromCache uses it
// when no path is given.
const DefaultPath = "data/definitions/fletching.json"

// defaultAnimation is the animation a recipe plays when neither
// it nor its group names one (DEFAULT_ANIMATION,
// Shared.Fletching.js:11).
const defaultAnimation = "human_fletching"

// SpoolChargesVarbit is the varbit the bow-string spool's
// stored charges are sent on (Spool.Fletching.js:42).
const SpoolChargesVarbit = "bowstring_spool_charges"

// SpoolSizeVarbit is the varbit the spool's spare count is
// sent on (Spool.Fletching.js:43).
const SpoolSizeVarbit = "bowstring_spool_size"

// InstantVarbit is the "Make-X for darts and bolts" setting
// varbit (groups darts/bolts: instantVarbit).
const InstantVarbit = "option_fletch_dart_or_bolt_makex"

// ---- raw JSON shapes (data/fletching.json) ----

// rawMaterial is one [name, amount] material entry. The name
// is an item name or an "@alternative" key.
type rawMaterial struct {
	Name   string
	Amount int
}

// UnmarshalJSON parses a [name, amount] pair.
func (m *rawMaterial) UnmarshalJSON(data []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(data, &pair); err != nil {
		return err
	}
	if err := json.Unmarshal(pair[0], &m.Name); err != nil {
		return fmt.Errorf("material name: %w", err)
	}
	if err := json.Unmarshal(pair[1], &m.Amount); err != nil {
		return fmt.Errorf("material amount: %w", err)
	}
	return nil
}

// rawAlternative is one alternatives entry: a plain item name,
// or {"item": name, "usedOnly": true} — an alternative that
// works only as the used item, not the target
// (Shared.Fletching.js resolveMaterial).
type rawAlternative struct {
	Name     string
	UsedOnly bool
}

// UnmarshalJSON parses a string name or an {item, usedOnly}
// object.
func (a *rawAlternative) UnmarshalJSON(data []byte) error {
	var name string
	if err := json.Unmarshal(data, &name); err == nil {
		a.Name = name
		return nil
	}
	var obj struct {
		Item     string `json:"item"`
		UsedOnly bool   `json:"usedOnly"`
	}
	if err := json.Unmarshal(data, &obj); err != nil {
		return err
	}
	a.Name = obj.Item
	a.UsedOnly = obj.UsedOnly
	return nil
}

// rawEntry is one group or recipe entry. Pointers and
// RawMessages record presence so a recipe entry can override
// exactly the fields it sets, leaving the rest to its group
// (resolveRecipe spreads {...group, ...entry}).
type rawEntry struct {
	Group          string          `json:"group"`
	Output         string          `json:"output"`
	Amount         *int            `json:"amount"`
	AmountMax      *int            `json:"amountMax"`
	Level          int             `json:"level"`
	XP             float64         `json:"xp"`
	XPEach         *float64        `json:"xpEach"`
	Ticks          *int            `json:"ticks"`
	FirstTick      *int            `json:"firstTick"`
	StartTick      json.RawMessage `json:"startTick"`
	StartSound     *bool           `json:"startSound"`
	JewellersTicks *int            `json:"jewellersTicks"`
	KnifeBonus     *bool           `json:"knifeBonus"`
	Tool           *string         `json:"tool"`
	Held           []string        `json:"held"`
	Materials      []rawMaterial   `json:"materials"`
	Animation      json.RawMessage `json:"animation"`
	Sound          *string         `json:"sound"`
	Message        *string         `json:"message"`
	MissingTool    *string         `json:"missingTool"`
	InstantVarbit  *string         `json:"instantVarbit"`
	MaxSets        *int            `json:"maxSets"`
	Quest          []string        `json:"quest"`
	Unlock         *string         `json:"unlock"`
	Menu           *string         `json:"menu"`
}

// rawMenu is one menus entry.
type rawMenu struct {
	Title string `json:"title"`
	Mode  *int   `json:"mode"`
}

// rawDiaryTask is one diaryTasks entry.
type rawDiaryTask struct {
	Diary string   `json:"diary"`
	Task  string   `json:"task"`
	Items []string `json:"items"`
	Areas [][]int  `json:"areas"`
}

// rawFile is the whole fletching.json document.
type rawFile struct {
	Comment      string                      `json:"//"`
	Tools        map[string][]string         `json:"tools"`
	Alternatives map[string][]rawAlternative `json:"alternatives"`
	Menus        map[string]rawMenu          `json:"menus"`
	Groups       map[string]rawEntry         `json:"groups"`
	DiaryTasks   []rawDiaryTask              `json:"diaryTasks"`
	Recipes      []rawEntry                  `json:"recipes"`
}

// IDResolver resolves the cache gameval names the JSON is
// written in. It is the seam Load resolves through, so tests
// can drive Load from a hand-built fixture instead of a live
// cache; production passes LoadFromCache's adapter over
// *cache.CacheDefinitions. Item names arrive in their
// ItemIdentifiers (UPPER_SNAKE) form; the adapter lowercases
// them to gameval form (see itemGameval). Names in the
// itemIDFallbacks table never reach the resolver — the
// loader consults the table first (see resolver.obj). Sounds
// resolve through game.ResolveKnownSound, not the resolver.
type IDResolver interface {
	ObjID(name string) (int, bool, error)
	SeqID(name string) (int, bool, error)
	VarbitID(name string) (int, bool, error)
}

type cacheResolver struct{ defs *cache.CacheDefinitions }

func (r cacheResolver) ObjID(name string) (int, bool, error) {
	return r.defs.GetObjId(itemGameval(name))
}
func (r cacheResolver) SeqID(name string) (int, bool, error) { return r.defs.GetSeqId(name) }
func (r cacheResolver) VarbitID(name string) (int, bool, error) {
	return r.defs.GetVarbitId(name)
}

// itemGameval maps an ItemIdentifiers name to its cache
// gameval obj name: the lowercase form (e.g. "ARROW_SHAFT"
// → "arrow_shaft", "SHORTBOW_U_" → "shortbow_u_"). The
// woodcutting source tables use the same mapping when they
// write "bronze_axe" for ItemIds.BRONZE_AXE. It only answers
// for the 102 names whose gameval has that form; the rest
// resolve through itemIDFallbacks (see item_ids.go).
func itemGameval(name string) string {
	return strings.ToLower(name)
}

// resolver collects unresolved names so Load fails closed
// with every missing name listed, instead of stopping at the
// first — the woodcutting tables.go precedent.
type resolver struct {
	r       IDResolver
	missing []string
}

func (rs *resolver) obj(name string) int {
	// The fallback table goes first: 3 names' lowercase
	// form is another item's gameval (OPAL_BOLTS is
	// "opal_bolt" 3818, the enchanted bolts, not 879), so a
	// gameval lookup must never win for them. A name in
	// neither the table nor the cache is recorded missing.
	if id, ok := itemIDFallbacks[name]; ok {
		return id
	}
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

func (rs *resolver) varbit(name string) int {
	id, ok, err := rs.r.VarbitID(name)
	if err != nil {
		rs.missing = append(rs.missing, fmt.Sprintf("varbit %q: %v", name, err))
		return 0
	}
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("varbit %q", name))
	}
	return id
}

func (rs *resolver) sound(name string) game.Sound {
	sound, ok := game.ResolveKnownSound(name)
	if !ok {
		rs.missing = append(rs.missing, fmt.Sprintf("sound %q", name))
		return game.Sound{}
	}
	return sound
}

func (rs *resolver) err() error {
	if len(rs.missing) == 0 {
		return nil
	}
	return fmt.Errorf("fletching: unresolved cache ids: %v", rs.missing)
}

// LoadFromCache resolves the Fletching tables against the real
// cache definitions and the recipe file at path (DefaultPath
// when path is ""). A nil cache fails; a missing or malformed
// file fails; an unresolvable name fails listing every missing
// name.
func LoadFromCache(defs *cache.CacheDefinitions, path string) (*Tables, error) {
	if defs == nil {
		return nil, errors.New("fletching: nil cache definitions")
	}
	if path == "" {
		path = DefaultPath
	}
	return LoadFile(path, cacheResolver{defs: defs})
}

// LoadFile reads the recipe file at path and resolves it
// through r.
func LoadFile(path string, r IDResolver) (*Tables, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("fletching: reading %s: %w", path, err)
	}
	return Load(data, r)
}

// Load parses the fletching.json document in data and resolves
// every name through r, or returns an error listing every
// gameval name the cache does not know. No id is invented: an
// unresolvable name fails the load.
func Load(data []byte, r IDResolver) (*Tables, error) {
	if r == nil {
		return nil, errors.New("fletching: nil id resolver")
	}
	var raw rawFile
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("fletching: parsing fletching.json: %w", err)
	}
	if len(raw.Recipes) == 0 {
		return nil, errors.New("fletching: no recipes in fletching.json")
	}
	rs := &resolver{r: r}
	t := &Tables{
		ToolIDs:   make(map[string][]int, len(raw.Tools)),
		ItemIDs:   make(map[string]int),
		SeqIDs:    make(map[string]int),
		VarbitIDs: make(map[string]int),
		SoundIDs:  make(map[string]int),
	}

	// Menus first: recipes and groups name them.
	menus := make(map[string]Menu, len(raw.Menus))
	for name, rawMenu := range raw.Menus {
		menus[name] = Menu{Title: rawMenu.Title, Mode: rawMenu.Mode}
	}

	// Tools: every tools key resolves to item ids.
	for name, items := range raw.Tools {
		if len(items) == 0 {
			return nil, fmt.Errorf("fletching: tool %q has no items", name)
		}
		ids := make([]int, 0, len(items))
		for _, item := range items {
			id := rs.obj(item)
			t.ItemIDs[item] = id
			ids = append(ids, id)
		}
		t.ToolIDs[name] = ids
	}

	// Groups: resolve each group's defaults once, to validate
	// every name a group sets (animation, sound, menu, tool,
	// held, instant varbit) even before any recipe uses it.
	// The resolved group itself is discarded — recipes carry
	// the merged values.
	for name, entry := range raw.Groups {
		if _, err := rs.resolveRecipe(name, entry, raw, menus, t); err != nil {
			return nil, fmt.Errorf("fletching: group %q: %w", name, err)
		}
	}

	// Recipes: merge each entry over its group, then resolve.
	for i, entry := range raw.Recipes {
		if entry.Group == "" {
			return nil, fmt.Errorf("fletching: recipe %d has no group", i)
		}
		groupEntry, ok := raw.Groups[entry.Group]
		if !ok {
			return nil, fmt.Errorf("fletching: recipe %d names unknown group %q", i, entry.Group)
		}
		merged := mergeEntry(groupEntry, entry)
		recipe, err := rs.resolveRecipe(entry.Group, merged, raw, menus, t)
		if err != nil {
			return nil, fmt.Errorf("fletching: recipe %d: %w", i, err)
		}
		t.Recipes = append(t.Recipes, recipe)
	}

	// Diary tasks: items resolve to ids, rects as-is
	// (Diaries.Fletching.js init).
	for i, entry := range raw.DiaryTasks {
		task, err := rs.resolveDiaryTask(entry, t)
		if err != nil {
			return nil, fmt.Errorf("fletching: diary task %d: %w", i, err)
		}
		t.DiaryTasks = append(t.DiaryTasks, task)
	}

	// The named ids the loop and the units resolve outside
	// the recipes (see the Tables contract in shared.go):
	// the spool varbits live in Spool.Fletching.js, not in
	// the JSON, but the loader resolves them here so the
	// Spool unit reads ids, not names (§2).
	t.SpoolChargesVarbitID = rs.varbit(SpoolChargesVarbit)
	t.SpoolSizeVarbitID = rs.varbit(SpoolSizeVarbit)
	t.VarbitIDs[SpoolChargesVarbit] = t.SpoolChargesVarbitID
	t.VarbitIDs[SpoolSizeVarbit] = t.SpoolSizeVarbitID
	t.ArrowShaftID = rs.obj("ARROW_SHAFT")
	t.ItemIDs["ARROW_SHAFT"] = t.ArrowShaftID
	t.KnifeID = rs.obj("KNIFE")
	t.ItemIDs["KNIFE"] = t.KnifeID
	t.CelastrusBarkID = rs.obj("CELASTRUS_BARK")
	t.ItemIDs["CELASTRUS_BARK"] = t.CelastrusBarkID
	t.SpoolID = rs.obj("BOW_STRING_SPOOL")
	t.ItemIDs["BOW_STRING_SPOOL"] = t.SpoolID
	t.BowStringID = rs.obj("BOW_STRING")
	t.ItemIDs["BOW_STRING"] = t.BowStringID
	if id, ok := t.ItemIDs["FLETCHING_KNIFE"]; ok {
		t.FletchingKnifeID = id
	}
	if id, ok := t.ItemIDs["JEWELLERS_CHISEL"]; ok {
		t.JewellersChiselID = id
	}

	// Distinct-name counts, in the plan §1 sense: raw tokens
	// count "@feathers" once; expanded names count its 8
	// kinds separately.
	rawItemNames := make(map[string]bool)
	expandedItemNames := make(map[string]bool)
	for _, recipe := range raw.Recipes {
		rawItemNames[recipe.Output] = true
		for _, mat := range recipe.Materials {
			rawItemNames[mat.Name] = true
		}
	}
	// Expanded names need the raw names, not the ids: recount
	// from the raw recipes with alternatives expanded.
	for _, recipe := range raw.Recipes {
		expandedItemNames[recipe.Output] = true
		for _, mat := range recipe.Materials {
			if strings.HasPrefix(mat.Name, "@") {
				for _, alt := range raw.Alternatives[mat.Name] {
					expandedItemNames[alt.Name] = true
				}
			} else {
				expandedItemNames[mat.Name] = true
			}
		}
	}
	t.DistinctItemNames = len(rawItemNames)
	t.DistinctExpandedItemNames = len(expandedItemNames)
	t.ResolvedItemNames = len(t.ItemIDs)
	for name := range t.ItemIDs {
		if _, ok := itemIDFallbacks[name]; ok {
			t.FallbackItemNames++
		} else {
			t.GamevalItemNames++
		}
	}

	if err := rs.err(); err != nil {
		return nil, err
	}
	return t, nil
}

// mergeEntry overlays a recipe entry on its group's entry
// ({...group, ...entry}, Shared.Fletching.js:114): a field
// the recipe sets wins; a field it leaves unset keeps the
// group's value. Level, XP and materials are recipe-only in
// the file, so the recipe's values always win there.
func mergeEntry(group, entry rawEntry) rawEntry {
	merged := group
	merged.Group = entry.Group
	merged.Output = entry.Output
	merged.Level = entry.Level
	merged.XP = entry.XP
	if entry.Amount != nil {
		merged.Amount = entry.Amount
	}
	if entry.AmountMax != nil {
		merged.AmountMax = entry.AmountMax
	}
	if entry.XPEach != nil {
		merged.XPEach = entry.XPEach
	}
	if entry.Ticks != nil {
		merged.Ticks = entry.Ticks
	}
	if entry.FirstTick != nil {
		merged.FirstTick = entry.FirstTick
	}
	if len(entry.StartTick) > 0 {
		merged.StartTick = entry.StartTick
	}
	if entry.StartSound != nil {
		merged.StartSound = entry.StartSound
	}
	if entry.JewellersTicks != nil {
		merged.JewellersTicks = entry.JewellersTicks
	}
	if entry.KnifeBonus != nil {
		merged.KnifeBonus = entry.KnifeBonus
	}
	if entry.Tool != nil {
		merged.Tool = entry.Tool
	}
	if entry.Held != nil {
		merged.Held = entry.Held
	}
	if entry.Materials != nil {
		merged.Materials = entry.Materials
	}
	if len(entry.Animation) > 0 {
		merged.Animation = entry.Animation
	}
	if entry.Sound != nil {
		merged.Sound = entry.Sound
	}
	if entry.Message != nil {
		merged.Message = entry.Message
	}
	if entry.MissingTool != nil {
		merged.MissingTool = entry.MissingTool
	}
	if entry.InstantVarbit != nil {
		merged.InstantVarbit = entry.InstantVarbit
	}
	if entry.MaxSets != nil {
		merged.MaxSets = entry.MaxSets
	}
	if entry.Quest != nil {
		merged.Quest = entry.Quest
	}
	if entry.Unlock != nil {
		merged.Unlock = entry.Unlock
	}
	if entry.Menu != nil {
		merged.Menu = entry.Menu
	}
	return merged
}

// resolveRecipe resolves one merged entry into a Recipe,
// applying the resolveRecipe defaults (Shared.Fletching.js:
// 112-140). groupName is the recipe's group ("" when
// resolving a group itself, in which case Output is empty
// and no output id is resolved).
func (rs *resolver) resolveRecipe(groupName string, entry rawEntry, raw rawFile, menus map[string]Menu, t *Tables) (*Recipe, error) {
	recipe := &Recipe{Group: groupName}

	if entry.Output != "" {
		recipe.OutputID = rs.obj(entry.Output)
		t.ItemIDs[entry.Output] = recipe.OutputID
	}
	recipe.Amount = 1
	if entry.Amount != nil {
		recipe.Amount = *entry.Amount
	}
	recipe.AmountMax = recipe.Amount
	if entry.AmountMax != nil {
		recipe.AmountMax = *entry.AmountMax
	}
	recipe.Level = entry.Level
	recipe.XP = entry.XP
	if entry.XPEach != nil {
		recipe.XPEach = *entry.XPEach
		recipe.HasXPEach = true
	}
	recipe.Ticks = 2
	if entry.Ticks != nil {
		recipe.Ticks = *entry.Ticks
	}
	recipe.FirstTick = 1
	if entry.FirstTick != nil {
		recipe.FirstTick = *entry.FirstTick
	}
	// startTick: absent → 0 (the choice's own tick),
	// null → none, a number → that tick.
	if len(entry.StartTick) > 0 {
		var tick *int
		if string(entry.StartTick) != "null" {
			var value int
			if err := json.Unmarshal(entry.StartTick, &value); err != nil {
				return nil, fmt.Errorf("startTick: %w", err)
			}
			tick = &value
		}
		recipe.StartTick = tick
	} else {
		zero := 0
		recipe.StartTick = &zero
	}
	if entry.StartSound != nil {
		recipe.StartSound = *entry.StartSound
	}
	recipe.JewellersTicks = entry.JewellersTicks
	if entry.KnifeBonus != nil {
		recipe.KnifeBonus = *entry.KnifeBonus
	}
	if entry.Tool != nil {
		recipe.Tool = *entry.Tool
		if _, ok := t.ToolIDs[recipe.Tool]; !ok {
			return nil, fmt.Errorf("unknown tool %q", recipe.Tool)
		}
	}
	for _, held := range entry.Held {
		ids, ok := t.ToolIDs[held]
		if !ok {
			return nil, fmt.Errorf("unknown held tool %q", held)
		}
		recipe.Held = append(recipe.Held, HeldTool{Name: held, IDs: ids})
	}
	for _, mat := range entry.Materials {
		material, err := rs.resolveMaterial(mat, raw, t)
		if err != nil {
			return nil, err
		}
		recipe.Materials = append(recipe.Materials, material)
	}
	// animation: absent → the default, false → none, a name
	// → that seq.
	animationName := ""
	if len(entry.Animation) == 0 {
		animationName = defaultAnimation
	} else if string(entry.Animation) != "false" {
		if err := json.Unmarshal(entry.Animation, &animationName); err != nil {
			return nil, fmt.Errorf("animation: %w", err)
		}
	}
	if animationName != "" {
		recipe.AnimationID = rs.seq(animationName)
		recipe.HasAnimation = true
		t.SeqIDs[animationName] = recipe.AnimationID
	}
	if entry.Sound != nil {
		recipe.Sound = rs.sound(*entry.Sound)
		t.SoundIDs[*entry.Sound] = recipe.Sound.ID()
	}
	if entry.Message != nil {
		recipe.Message = *entry.Message
	}
	if entry.MissingTool != nil {
		recipe.MissingTool = *entry.MissingTool
	}
	if entry.InstantVarbit != nil {
		recipe.InstantVarbitID = rs.varbit(*entry.InstantVarbit)
		recipe.HasInstantVarbit = true
		t.VarbitIDs[*entry.InstantVarbit] = recipe.InstantVarbitID
	}
	recipe.MaxSets = creationAllSentinel
	if entry.MaxSets != nil {
		recipe.MaxSets = *entry.MaxSets
	}
	if entry.Quest != nil {
		if len(entry.Quest) != 2 {
			return nil, fmt.Errorf("quest %v: want [key, state]", entry.Quest)
		}
		if entry.Quest[1] != "started" && entry.Quest[1] != "complete" {
			return nil, fmt.Errorf("quest %v: unknown state %q", entry.Quest, entry.Quest[1])
		}
		recipe.Quest = &QuestGate{Key: entry.Quest[0], State: entry.Quest[1]}
	}
	if entry.Unlock != nil {
		recipe.Unlock = *entry.Unlock
	}
	menuName := defaultMenu
	if entry.Menu != nil {
		menuName = *entry.Menu
	}
	menu, ok := menus[menuName]
	if !ok {
		return nil, fmt.Errorf("unknown menu %q", menuName)
	}
	recipe.Menu = menu
	return recipe, nil
}

// resolveMaterial resolves one [name, amount] material:
// an "@alternative" key expands to every alternative in
// data order, marking the usedOnly ones
// (Shared.Fletching.js resolveMaterial).
func (rs *resolver) resolveMaterial(mat rawMaterial, raw rawFile, t *Tables) (Material, error) {
	if mat.Amount <= 0 {
		return Material{}, fmt.Errorf("material %q: amount %d, want > 0", mat.Name, mat.Amount)
	}
	names := []string{mat.Name}
	usedOnly := map[string]bool{}
	if strings.HasPrefix(mat.Name, "@") {
		alts, ok := raw.Alternatives[mat.Name]
		if !ok {
			return Material{}, fmt.Errorf("unknown alternative %q", mat.Name)
		}
		names = names[:0]
		for _, alt := range alts {
			names = append(names, alt.Name)
			if alt.UsedOnly {
				usedOnly[alt.Name] = true
			}
		}
	}
	material := Material{Amount: mat.Amount}
	for _, name := range names {
		id := rs.obj(name)
		t.ItemIDs[name] = id
		material.IDs = append(material.IDs, id)
		if usedOnly[name] {
			material.UsedOnlyIDs = append(material.UsedOnlyIDs, id)
		}
	}
	return material, nil
}

// resolveDiaryTask resolves one diaryTasks entry: items to
// ids, rects as-is (Diaries.Fletching.js init).
func (rs *resolver) resolveDiaryTask(entry rawDiaryTask, t *Tables) (*DiaryTask, error) {
	if entry.Diary == "" || entry.Task == "" {
		return nil, errors.New("diary task with no diary or task key")
	}
	if len(entry.Items) == 0 {
		return nil, fmt.Errorf("diary task %q has no items", entry.Task)
	}
	task := &DiaryTask{Diary: entry.Diary, Task: entry.Task}
	for _, name := range entry.Items {
		id := rs.obj(name)
		t.ItemIDs[name] = id
		task.ItemNames = append(task.ItemNames, name)
		task.ItemIDs = append(task.ItemIDs, id)
	}
	for _, area := range entry.Areas {
		if len(area) != 4 {
			return nil, fmt.Errorf("diary task %q: area %v, want [minX, maxX, minY, maxY]", entry.Task, area)
		}
		if area[0] > area[1] || area[2] > area[3] {
			return nil, fmt.Errorf("diary task %q: area %v is inverted", entry.Task, area)
		}
		task.Areas = append(task.Areas, DiaryArea{MinX: area[0], MaxX: area[1], MinY: area[2], MaxY: area[3]})
	}
	if len(task.Areas) == 0 {
		return nil, fmt.Errorf("diary task %q has no areas", entry.Task)
	}
	return task, nil
}

// ItemID returns the resolved id of a raw item name, or false
// when the file never names it.
func (t *Tables) ItemID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.ItemIDs[name]
	return id, ok
}

// SeqID returns the resolved id of an animation gameval name.
func (t *Tables) SeqID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.SeqIDs[name]
	return id, ok
}

// VarbitID returns the resolved id of a varbit name.
func (t *Tables) VarbitID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.VarbitIDs[name]
	return id, ok
}

// SoundID returns the synth id of a sound name.
func (t *Tables) SoundID(name string) (int, bool) {
	if t == nil {
		return 0, false
	}
	id, ok := t.SoundIDs[name]
	return id, ok
}

// RecipeByOutput returns the recipes that make an output item
// id, in JSON order, or nil.
func (t *Tables) RecipeByOutput(id int) []*Recipe {
	if t == nil {
		return nil
	}
	var out []*Recipe
	for _, recipe := range t.Recipes {
		if recipe.OutputID == id {
			out = append(out, recipe)
		}
	}
	return out
}
