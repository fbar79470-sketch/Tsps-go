package definition

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"tsps-go/internal/cache"
	"tsps-go/internal/config"
)

// DefaultSourcePriority is the priority registerSource assigns when
// a provider carries none (DefinitionLoader.ts:48: `?? 100`).
const DefaultSourcePriority = 100

// Source is one registered definition source: a provider object in
// DefinitionLoader.ts terms. Name defaults to the registering owner
// and Priority defaults to DefaultSourcePriority when nil, exactly
// as registerSource derives them (DefinitionLoader.ts:47-48).
// Load returns the decoded definitions value; loadSources counts a
// non-array return as a failure rather than an error, mirroring
// the Array.isArray check (DefinitionLoader.ts:86-92), so Load
// should return the value and a nil error in that case and reserve
// errors for read/parse failures.
type Source struct {
	Name     string
	Priority *int
	Load     func() (any, error)
}

// LoadedSource is one source whose load produced an array, in the
// shape of LoadedDefinitionSource<T> (DefinitionLoader.ts:12-17).
// Definitions holds the raw decoded JSON values; each loader
// coerces them with the JS Number() helpers in this file.
type LoadedSource struct {
	Name        string
	Owner       string
	Priority    int
	Definitions []any
}

// Registry is the explicit, caller-owned replacement for the
// global static sourcesByDefinition map in DefinitionLoader.ts:29-32.
// Sources are keyed by definition type, then by "owner:name". It
// must be used through the pointer NewRegistry returns. It is safe
// for concurrent use; in practice registration happens before Load
// and loading is single-threaded.
type Registry struct {
	mu     sync.Mutex
	byType map[string]map[string]registeredSource
}

type registeredSource struct {
	owner    string
	name     string
	priority int
	load     func() (any, error)
}

// NewRegistry returns an empty source registry.
func NewRegistry() *Registry {
	return &Registry{byType: make(map[string]map[string]registeredSource)}
}

func (r *Registry) lock()   { r.mu.Lock() }
func (r *Registry) unlock() { r.mu.Unlock() }

// IntPriority returns a Source priority pointer, for callers that
// set an explicit priority.
func IntPriority(priority int) *int { return &priority }

// Register adds a source for a definition type, mirroring
// DefinitionLoader.registerSource (DefinitionLoader.ts:34-74):
// the type and owner are trimmed and required, a missing name
// defaults to the owner, a missing priority defaults to 100, a
// missing load function is rejected, and registering the same
// owner:name twice for one type is an error. Error messages match
// the TypeScript throws. (The TypeScript finite-priority check has
// no Go equivalent: priorities are ints, always finite.)
func (r *Registry) Register(definitionType, owner string, source Source) error {
	defType := strings.TrimSpace(definitionType)
	sourceOwner := strings.TrimSpace(owner)
	sourceName := strings.TrimSpace(source.Name)
	if sourceName == "" {
		sourceName = sourceOwner
	}
	priority := DefaultSourcePriority
	if source.Priority != nil {
		priority = *source.Priority
	}
	if defType == "" {
		return errors.New("Definition source requires a definition type")
	}
	if sourceOwner == "" || sourceName == "" {
		return fmt.Errorf("Definition source %s requires an owner and name", defType)
	}
	if source.Load == nil {
		return fmt.Errorf("Definition source %s:%s requires load()", defType, sourceName)
	}
	r.lock()
	defer r.unlock()
	sources, ok := r.byType[defType]
	if !ok {
		sources = make(map[string]registeredSource)
		r.byType[defType] = sources
	}
	key := sourceOwner + ":" + sourceName
	if _, exists := sources[key]; exists {
		return fmt.Errorf("Definition source %s:%s is already registered", defType, key)
	}
	sources[key] = registeredSource{owner: sourceOwner, name: sourceName, priority: priority, load: source.Load}
	return nil
}

// RegisterFile registers a JSON file path as a source, mirroring
// the string form of DefinitionSource (DefinitionLoader.ts:41-46):
// the file is read and parsed lazily, when the loader runs — not
// at registration time — and a blank path is rejected. The source
// name defaults to the owner and the priority to 100, as the
// synthesized provider carries neither.
func (r *Registry) RegisterFile(definitionType, owner, path string) error {
	defType := strings.TrimSpace(definitionType)
	if strings.TrimSpace(path) == "" {
		unknown := defType
		if unknown == "" {
			unknown = "unknown"
		}
		return fmt.Errorf("Definition source %s requires a JSON file path", unknown)
	}
	return r.Register(definitionType, owner, Source{
		Load: func() (any, error) { return readJSONValue(path) },
	})
}

// SourceNames returns the registered source names for a definition
// type in load order, mirroring getSourceNames
// (DefinitionLoader.ts:76-78).
func (r *Registry) SourceNames(definitionType string) []string {
	ordered := r.orderedSources(definitionType)
	names := make([]string, len(ordered))
	for i, source := range ordered {
		names[i] = source.name
	}
	return names
}

// orderedSources returns a type's sources sorted by priority, then
// by name (DefinitionLoader.ts:110-121). The TypeScript tie-break
// is String.localeCompare; Go string ordering agrees with it for
// the ASCII source names in practice, and may differ for
// non-ASCII names — a documented edge, not a behavioral choice.
func (r *Registry) orderedSources(definitionType string) []registeredSource {
	r.lock()
	defer r.unlock()
	sources, ok := r.byType[strings.TrimSpace(definitionType)]
	if !ok {
		return nil
	}
	ordered := make([]registeredSource, 0, len(sources))
	for _, source := range sources {
		ordered = append(ordered, source)
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].priority != ordered[j].priority {
			return ordered[i].priority < ordered[j].priority
		}
		return ordered[i].name < ordered[j].name
	})
	return ordered
}

// loadSources runs a type's sources in order, mirroring
// DefinitionLoader.loadSources (DefinitionLoader.ts:80-108): a
// source whose load throws counts as a failure (logged as an
// error), a source returning a non-array value counts as a
// failure (logged as a warning), and every other source
// contributes its definitions. Failures never abort the load;
// callers report them through their own return values and logs,
// as the TypeScript loaders do (GameBuilder ignores them).
func (r *Registry) loadSources(definitionType string, logf func(format string, args ...any)) ([]LoadedSource, int) {
	var loaded []LoadedSource
	failures := 0
	for _, source := range r.orderedSources(definitionType) {
		value, err := source.load()
		if err != nil {
			failures++
			logf("[definitions] Failed to load %s:%s: %v", definitionType, source.name, err)
			continue
		}
		definitions, ok := value.([]any)
		if !ok {
			failures++
			logf("[definitions] %s:%s returned a non-array value", definitionType, source.name)
			continue
		}
		loaded = append(loaded, LoadedSource{
			Name:        source.name,
			Owner:       source.owner,
			Priority:    source.priority,
			Definitions: definitions,
		})
	}
	return loaded, failures
}

// sortLoadedSources orders core-plus-contributed sources by
// priority then name, the inline sort the GroundItemSpawn and Shop
// loaders apply after prepending their core source
// (GroundItemSpawnDefinitionLoader.ts:42-47,
// ShopDefinitionLoader.ts:67-72).
func sortLoadedSources(sources []LoadedSource) {
	sort.SliceStable(sources, func(i, j int) bool {
		if sources[i].Priority != sources[j].Priority {
			return sources[i].Priority < sources[j].Priority
		}
		return sources[i].Name < sources[j].Name
	})
}

// LoadOptions carries what Load needs beyond the definitions
// directory. Cache and Gamevals are both nil-allowed: with nil
// (NoCache boots) the loaders still load and the wrappers degrade
// gracefully — see the package comment for exactly how far that
// goes, and the per-site comments for each degradation.
type LoadOptions struct {
	// Cache is the cache definition facade, or nil for a NoCache
	// boot. When nil, cache-backed filters are skipped and the
	// wrappers return their TS DEFAULT shapes.
	Cache *cache.CacheDefinitions
	// Gamevals is the standalone gameval name facade, or nil.
	// Sequence names for NPC animation inference come from
	// Gamevals when present, else from Cache, else nowhere.
	Gamevals *cache.Gamevals
	// Logf receives load warnings and errors in the TypeScript
	// console.* formats ("[npc-spawns] ..."). Nil silences them.
	Logf func(format string, args ...any)
	// World is the parsed world definition, used only for the
	// members-world check that filters members-only ground items
	// (GroundItemSpawnDefinitionLoader.ts:62, via isMembersWorld).
	// Nil assumes a members world (the shipped default: the
	// FreeToPlay switch is off), so the filter is inactive.
	World *config.World
	// Sources is the definition-source registry Load reads
	// contributed sources from. Nil uses a fresh registry. Load
	// registers the core npc-spawns file sources into it (the
	// NpcSpawns plugin's job in TS); callers must not pre-register
	// those same owner:name pairs.
	Sources *Registry
	// PresetShopPricesEnabled mirrors the module-level flag
	// setPresetShopPricesEnabled in ShopDefinitionLoader.ts:40-44
	// (default false): only when true do requiresPresets shops
	// keep their configured prices and currency.
	PresetShopPricesEnabled bool
}

// Store is the loaded definition data: spawn lists, definition
// collections, and the cache-layered wrappers. It is built by Load
// and is safe for concurrent reads after Load returns; the wrapper
// accessors build fresh values per call, so callers may hold them
// without aliasing the Store's state.
type Store struct {
	dir      string
	cache    *cache.CacheDefinitions
	gamevals *cache.Gamevals
	logf     func(format string, args ...any)

	membersWorld      bool
	presetShopPrices  bool
	registry          *Registry
	seqNames          map[int]string
	seqIDsByName      map[string]int
	itemIDsByName     map[string]int
	itemIDsByNameDone bool

	npcSpawns       []NpcSpawn
	objectSpawns    []ObjectSpawn
	objectRemovals  []ObjectSpawn
	groundItems     []GroundItem
	interactions    map[int]*NpcInteractionDefinition
	interactionList []*NpcInteractionDefinition
	layouts         map[string]*InterfaceLayoutDefinition
	layoutsByID     map[int][]*InterfaceLayoutDefinition
	layoutList      []*InterfaceLayoutDefinition
	shops           map[int]*ShopDefinition
	shopList        []*ShopDefinition
	customShopIDs   map[int]bool
	itemOverlays    map[int]*itemOverlay
	npcOverlays     map[int]*npcOverlay
	npcExamines     map[int]string
	objectExamines  map[int]string
	equipmentSounds *equipmentSoundData
	npcCombatSounds map[int]npcSounds
}

// NPCSpawns returns the loaded NPC spawns, in merged load order.
func (s *Store) NPCSpawns() []NpcSpawn { return append([]NpcSpawn(nil), s.npcSpawns...) }

// ObjectSpawns returns the loaded object spawns, core file first
// then contributed sources in registry order. Entries marked
// remove in the source files are NOT included; see
// ObjectSpawnRemovals.
func (s *Store) ObjectSpawns() []ObjectSpawn {
	return append([]ObjectSpawn(nil), s.objectSpawns...)
}

// ObjectSpawnRemovals returns the object-spawn entries marked
// remove:true, which TypeScript applies as deregistration through
// ObjectManager (ObjectSpawnDefinitionLoader.ts:40-43). There is
// no ObjectManager in Go, so the consumer applies them.
func (s *Store) ObjectSpawnRemovals() []ObjectSpawn {
	return append([]ObjectSpawn(nil), s.objectRemovals...)
}

// GroundItemSpawns returns the loaded ground-item spawns in the
// contract shape. The richer records (members flag, pickup
// replacement) are available from GroundItems in the same order.
func (s *Store) GroundItemSpawns() []GroundItemSpawn {
	out := make([]GroundItemSpawn, len(s.groundItems))
	for i, item := range s.groundItems {
		out[i] = item.Spawn
	}
	return out
}

// GroundItems returns the loaded ground-item spawns with the fields
// the contract shape omits: the members flag and the pickup
// replacement. The order matches GroundItemSpawns.
func (s *Store) GroundItems() []GroundItem { return append([]GroundItem(nil), s.groundItems...) }

// Registry returns the source registry Load used, for callers that
// registered contributed sources and want to inspect them.
func (s *Store) Registry() *Registry { return s.registry }

// Load loads every definition collection from dir (the definitions
// directory, e.g. <root>/data/definitions) in GameBuilder
// loadStartupData order (GameBuilder.ts:44-50), then the
// item-gameplay/prices/alch/overrides application the TS item
// plugin performs, then the lazily-read examine/sound files. See
// the package comment for the required-vs-optional file table.
// A missing directory fails Load at the first required file
// (interface-layouts.json), exactly as the TypeScript loaders'
// readFileSync throws. Unlike DefinitionLoader.run()
// (DefinitionLoader.ts:125-135), which swallows load errors, Load
// returns them: GameBuilder calls load() directly, so in TS a
// required-file failure is fatal to boot, and Load follows the
// GameBuilder path.
func Load(dir string, opts LoadOptions) (*Store, error) {
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	registry := opts.Sources
	if registry == nil {
		registry = NewRegistry()
	}
	s := &Store{
		dir:              dir,
		cache:            opts.Cache,
		gamevals:         opts.Gamevals,
		logf:             logf,
		membersWorld:     opts.World == nil || opts.World.MembersWorld,
		presetShopPrices: opts.PresetShopPricesEnabled,
		registry:         registry,
		interactions:     make(map[int]*NpcInteractionDefinition),
		layouts:          make(map[string]*InterfaceLayoutDefinition),
		layoutsByID:      make(map[int][]*InterfaceLayoutDefinition),
		shops:            make(map[int]*ShopDefinition),
		customShopIDs:    make(map[int]bool),
		itemOverlays:     make(map[int]*itemOverlay),
		npcOverlays:      make(map[int]*npcOverlay),
		npcExamines:      make(map[int]string),
		objectExamines:   make(map[int]string),
		npcCombatSounds:  make(map[int]npcSounds),
		seqNames:         make(map[int]string),
		seqIDsByName:     make(map[string]int),
	}

	// Core npc-spawns registration: in TS the NpcSpawns plugin
	// registers the file as a string source (owner/name "NpcSpawns",
	// priority 100) plus the map editor's optional patch
	// (NpcSpawns.plugin.js:15-22). There is no plugin system here,
	// so Load registers both itself. The patch file is absent until
	// the editor exports one; its load returns [] then, so its
	// absence is never a failure.
	if err := registry.RegisterFile(npcSpawnsDefinitionType, "NpcSpawns", filepath.Join(dir, "npc-spawns.json")); err != nil {
		return nil, err
	}
	customSpawnsFile := filepath.Join(dir, "npc-spawns-custom.json")
	if err := registry.Register(npcSpawnsDefinitionType, "NpcSpawns", Source{
		Name:     "editor-custom",
		Priority: IntPriority(200),
		Load: func() (any, error) {
			value, err := readJSONValueOptional(customSpawnsFile)
			if err != nil {
				return nil, err
			}
			if value == nil {
				return []any{}, nil
			}
			return value, nil
		},
	}); err != nil {
		return nil, err
	}

	s.loadSeqNames()

	if err := s.loadInteractions(); err != nil {
		return nil, err
	}
	if err := s.loadLayouts(); err != nil {
		return nil, err
	}
	if err := s.loadObjectSpawns(); err != nil {
		return nil, err
	}
	if err := s.loadGroundItems(); err != nil {
		return nil, err
	}
	if err := s.loadNpcDefinitions(); err != nil {
		return nil, err
	}
	if err := s.loadNpcSpawns(); err != nil {
		return nil, err
	}
	if err := s.loadShops(); err != nil {
		return nil, err
	}
	// Item JSON application and the examine/sound files are not
	// part of loadStartupData (the item plugin runs earlier, the
	// examine/sound files are lazy in TS); they are independent
	// collections, so loading them last changes nothing observable.
	if err := s.loadItemJSON(); err != nil {
		return nil, err
	}
	if err := s.loadExaminesAndSounds(); err != nil {
		return nil, err
	}
	return s, nil
}

// loadSeqNames resolves the animation gameval names NPC animation
// inference needs (NpcDefinitionLoader familyRoles/capturedRoles
// call CacheDefinitions.getSeqName/getSeqNames). Gamevals wins
// when present; otherwise the Cache facade's own gamevals; with
// neither, inference runs with no names. A lookup error warns and
// yields no names rather than failing the load: without a cache
// there is no name table at all, and TS's getSeqNames on a
// cache-less boot throws from getState — the Go degradation is to
// infer nothing.
func (s *Store) loadSeqNames() {
	var names map[int]string
	var err error
	if s.gamevals != nil {
		names, err = s.gamevals.NamesOf(cache.GamevalSeq)
	}
	if names == nil && s.cache != nil {
		names, err = s.cache.GetSeqNames()
	}
	if err != nil {
		s.logf("[npc-definitions] sequence names unavailable: %v", err)
		return
	}
	// Iterate ids ascending so a name shared by several ids
	// resolves to the smallest, deterministically (Go map
	// iteration order is random; the TS gameval scan is ordered).
	ids := make([]int, 0, len(names))
	for id := range names {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		name := names[id]
		s.seqNames[id] = name
		if _, exists := s.seqIDsByName[name]; !exists {
			s.seqIDsByName[name] = id
		}
	}
}

// seqName returns an animation's gameval name, or "" when the cache
// has none for it (CacheDefinitions.getSeqName returns null).
func (s *Store) seqName(id int) string { return s.seqNames[id] }

// seqID returns an animation's id by gameval name, or -1
// (CacheDefinitions.getSeqId returns null; callers use ?? -1).
func (s *Store) seqID(name string) int {
	if id, ok := s.seqIDsByName[name]; ok {
		return id
	}
	return -1
}

// npcCount returns the cache NPC count, or -1 when unknown (nil
// cache). Callers treat -1 as "no bound known" and skip
// cache-backed range filters.
func (s *Store) npcCount() int {
	if s.cache == nil {
		return -1
	}
	return s.cache.GetCounts().NPCs
}

// itemCount returns the cache item count, or -1 when unknown.
func (s *Store) itemCount() int {
	if s.cache == nil {
		return -1
	}
	return s.cache.GetCounts().Items
}

// objectCount returns the cache object count, or -1 when unknown.
func (s *Store) objectCount() int {
	if s.cache == nil {
		return -1
	}
	return s.cache.GetCounts().Objects
}

// hasItem mirrors CacheDefinitions.hasItem for ground-item
// filtering, with the nil-cache degradation: without a cache
// every id is treated as supported, so fixture ground items
// survive a NoCache boot (see the package comment).
func (s *Store) hasItem(id int) bool {
	if s.cache == nil {
		return id > 0
	}
	return s.cache.HasItem(id)
}

// readJSONValue reads and parses one JSON file. A missing or
// malformed file is an error, as readFileSync + JSON.parse throw
// in TypeScript.
func readJSONValue(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return value, nil
}

// readJSONValueOptional reads and parses one JSON file that may be
// absent: a missing file yields (nil, nil), while a present but
// malformed file is an error — the readOptional shape in
// NpcDefinitionLoader.ts:503-510 (existsSync gate, then a throwing
// parse).
func readJSONValueOptional(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return value, nil
}

// readOptionalStringMap reads a JSON file that may be absent and
// must hold an object keyed by canonical integer ids, for the
// examine files this package reads eagerly (see
// loadExaminesAndSounds). Absent yields an empty map; a present
// but malformed file, or a non-object value, is an error.
// Non-string values read as "" (they never match a lookup), and
// non-canonical keys ("007") are skipped: TypeScript looks these
// maps up by String(id), which never produces such a key.
func readOptionalStringMap(path string, label string) (map[int]string, error) {
	value, err := readJSONValueOptional(path)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return make(map[int]string), nil
	}
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must contain an object", label)
	}
	out := make(map[int]string, len(obj))
	for key, raw := range obj {
		id, err := strconv.Atoi(key)
		if err != nil || strconv.Itoa(id) != key {
			continue
		}
		text, _ := raw.(string)
		out[id] = text
	}
	return out, nil
}

// --- JavaScript value coercion ------------------------------------
//
// The loaders coerce untyped JSON with Number(), Number.isInteger,
// Math.trunc and String(). These helpers reproduce those semantics
// over encoding/json values (nil, bool, float64, string, []any,
// map[string]any). JSON objects decoded by encoding/json carry no
// key order and no undefined; callers pass present=false for a
// missing field, which coerces like undefined (Number -> NaN).

// jsNumber is Number(value): NaN for undefined (present=false) and
// for values that do not convert, 0 for null and empty strings, 1/0
// for booleans, and the numeric value for numbers and numeric
// strings (including 0x/0b/0o and Infinity forms).
func jsNumber(value any, present bool) float64 {
	if !present {
		return math.NaN()
	}
	switch v := value.(type) {
	case nil:
		return 0
	case bool:
		if v {
			return 1
		}
		return 0
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n
		}
		return math.NaN()
	case string:
		return jsParseNumber(v)
	case []any:
		// Number([a,b]) is Number("a,b"): elements joined by ",".
		parts := make([]string, len(v))
		for i, elem := range v {
			parts[i] = jsString(elem, true)
		}
		return jsParseNumber(strings.Join(parts, ","))
	default:
		return math.NaN()
	}
}

// jsParseNumber is Number() applied to a string: surrounding
// ECMAScript whitespace is trimmed, "" becomes 0, and the rest must
// be a whole numeric literal (decimal, hex, binary, octal, or
// Infinity).
func jsParseNumber(s string) float64 {
	trimmed := strings.Trim(s, " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u202f\u205f\u3000\ufeff\u2028\u2029")
	if trimmed == "" {
		return 0
	}
	neg := false
	rest := trimmed
	if strings.HasPrefix(rest, "+") {
		rest = rest[1:]
	} else if strings.HasPrefix(rest, "-") {
		neg = true
		rest = rest[1:]
	}
	if rest == "Infinity" {
		if neg {
			return math.Inf(-1)
		}
		return math.Inf(1)
	}
	if len(rest) > 2 && rest[0] == '0' {
		switch rest[1] {
		case 'x', 'X':
			return jsParseRadix(rest[2:], 16, neg)
		case 'b', 'B':
			return jsParseRadix(rest[2:], 2, neg)
		case 'o', 'O':
			return jsParseRadix(rest[2:], 8, neg)
		}
	}
	n, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return math.NaN()
	}
	return n
}

func jsParseRadix(digits string, base int, neg bool) float64 {
	if digits == "" {
		return math.NaN()
	}
	n, err := strconv.ParseUint(digits, base, 64)
	if err != nil {
		return math.NaN()
	}
	if neg {
		return -float64(n)
	}
	return float64(n)
}

// jsString is String(value) for JSON values. Objects stringify to
// "[object Object]" and arrays join their elements with ",".
func jsString(value any, present bool) string {
	if !present {
		return "undefined"
	}
	switch v := value.(type) {
	case nil:
		return "null"
	case bool:
		if v {
			return "true"
		}
		return "false"
	case float64:
		return jsFormatNumber(v)
	case string:
		return v
	case []any:
		parts := make([]string, len(v))
		for i, elem := range v {
			if elem == nil {
				parts[i] = ""
			} else {
				parts[i] = jsString(elem, true)
			}
		}
		return strings.Join(parts, ",")
	default:
		return "[object Object]"
	}
}

// jsFormatNumber formats a float64 the way JavaScript's Number to
// string conversion does, for the integer-valued magnitudes JSON
// spawn names carry. Non-integers use Go's shortest repr, which
// matches JS for the magnitudes involved.
func jsFormatNumber(v float64) string {
	if math.IsNaN(v) {
		return "NaN"
	}
	if math.IsInf(v, 1) {
		return "Infinity"
	}
	if math.IsInf(v, -1) {
		return "-Infinity"
	}
	if v == math.Trunc(v) && math.Abs(v) < 1e21 {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// jsTrunc is Math.trunc.
func jsTrunc(v float64) int { return int(math.Trunc(v)) }

// isJSInteger is Number.isInteger: finite with no fractional part.
func isJSInteger(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v == math.Trunc(v) }

// isJSSafeInteger is Number.isSafeInteger.
func isJSSafeInteger(v float64) bool {
	return isJSInteger(v) && math.Abs(v) <= 9007199254740991
}

// field returns a JSON object's field and whether it was present.
// A nil object or a non-object yields absent, mirroring the ?.
// chains in the loaders (raw?.id is undefined when raw is null).
func field(obj any, key string) (any, bool) {
	m, ok := obj.(map[string]any)
	if !ok {
		return nil, false
	}
	v, present := m[key]
	return v, present
}

// numField is Number(obj[key]) with missing-field semantics.
func numField(obj any, key string) float64 {
	v, present := field(obj, key)
	return jsNumber(v, present)
}

// objField returns obj[key] when it is a JSON object, else nil.
func objField(obj any, key string) map[string]any {
	v, present := field(obj, key)
	if !present {
		return nil
	}
	m, _ := v.(map[string]any)
	return m
}

// intField reads a field as an integer the way Number.isInteger
// checks do: the value must already be a finite integer number —
// strings do NOT coerce ("5" fails, as Number.isInteger("5")
// does). Callers that want Number() coercion use numField.
func intField(obj any, key string) (int, bool) {
	v, present := field(obj, key)
	if !present {
		return 0, false
	}
	n, ok := numberValue(v)
	if !ok || !isJSInteger(n) {
		return 0, false
	}
	return jsTrunc(n), true
}

// numberValue reports whether a decoded JSON value is a finite
// number (for the loaders' Number.isFinite / typeof checks, which
// do NOT coerce strings).
func numberValue(v any) (float64, bool) {
	n, ok := v.(float64)
	if !ok || math.IsNaN(n) || math.IsInf(n, 0) {
		return 0, false
	}
	return n, true
}

// sortedNumericKeys returns a keyed map's keys in the order
// JavaScript Object.keys/Object.entries yields them for
// integer-like keys: ascending numeric order. Non-numeric keys
// follow in sorted order (their TS order is insertion order, which
// encoding/json does not preserve; the loaders' per-key outcomes
// are independent, so only iteration order differs, never results).
func sortedNumericKeys[V any](obj map[string]V) []string {
	var numeric []int
	var other []string
	for key := range obj {
		if n, err := strconv.Atoi(key); err == nil && strconv.Itoa(n) == key {
			numeric = append(numeric, n)
		} else {
			other = append(other, key)
		}
	}
	sort.Ints(numeric)
	sort.Strings(other)
	keys := make([]string, 0, len(obj))
	for _, n := range numeric {
		keys = append(keys, strconv.Itoa(n))
	}
	return append(keys, other...)
}

// asKeyedObject coerces a decoded JSON value to the keyed-object
// shape the NPC definition files use: an object as-is, an array
// keyed by index (as Object.keys of an array yields), anything
// else an empty map. Callers that must distinguish null handle it
// before calling.
func asKeyedObject(value any) map[string]any {
	switch v := value.(type) {
	case map[string]any:
		return v
	case []any:
		out := make(map[string]any, len(v))
		for i, elem := range v {
			out[strconv.Itoa(i)] = elem
		}
		return out
	default:
		return make(map[string]any)
	}
}

// normalizeName is the loader's normalizeName
// (NpcDefinitionLoader.ts:181): collapse whitespace runs to one
// space, trim, lowercase. Non-strings become "" (value ?? "").
func normalizeName(value any) string {
	s, _ := value.(string)
	fields := strings.Fields(s)
	return strings.ToLower(strings.Join(fields, " "))
}

// utf16Len is the length of s in UTF-16 code units, as JavaScript's
// .length counts it (used by the world-definition validation in
// worldconfig.go).
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		if r > 0xFFFF {
			n += 2
		} else {
			n++
		}
	}
	return n
}
