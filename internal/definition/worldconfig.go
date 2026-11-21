package definition

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// This file ports WorldConfigStore.ts plus the world-config
// functions it calls: mergeWorldConfig/parseWorldDefinition
// (WorldDefinition.ts), normalizeWorldPluginConfig/
// isFreeToPlayWorld/preserveLegacyCrossFloorDefault
// (WorldPluginConfig.ts), checkConfigValue/validateDeclaration and
// the setting declarations (WorldSettingDeclarations.ts), and
// parsePluginSwitches/resolvePluginEnabled/pluginNameFromFile
// (PluginManager.ts). The Go config package has its own world
// parsing, but its merge/normalize helpers are unexported and its
// shapes differ; the store ports the TypeScript functions
// locally, as the summary of differences in each doc comment
// notes. ScopedPluginSettings (PluginManager bookkeeping for
// where a flattened setting came from) is not ported: no Go
// caller reads it.
//
// The runtime world-definition state in WorldDefinition.ts
// (WORLD_SPAWN, WORLD_ZONE_BOUNDARIES, isMembersArea, the free-land
// index) is NOT ported: it is server runtime state fed by the
// parsed definition, not part of the config store.

// WorldGameframes is WORLD_GAMEFRAMES (WorldDefinition.ts:24):
// the layouts world.json can force on every login.
var WorldGameframes = []string{"modern-resizable", "modern-fixed", "317-resizable", "317-fixed"}

// WorldConfigLayers is WorldConfigLayers (WorldConfigStore.ts:
// 15-24): the two world files normalized independently, with the
// raw local file hash. LocalError is set (and Local left empty)
// when world.local.json does not parse.
type WorldConfigLayers struct {
	Shipped     map[string]any
	Local       map[string]any
	LocalExists bool
	// LocalHash is the sha256 of world.local.json's raw bytes;
	// "" when the file is absent.
	LocalHash string
	// LocalError is "" when the local file parsed (or is absent).
	LocalError string
}

// WorldConfigChanges is WorldConfigChanges
// (WorldConfigStore.ts:28-32): what the page asks to change. A nil
// map value means "back to shipped" (the key leaves
// world.local.json). A nil section map means no edits to it.
type WorldConfigChanges struct {
	World        map[string]any
	Plugins      map[string]any
	PluginConfig map[string]any
}

// WorldConfigDiffEntry is one diff row: a changed path with its
// local-layer value before and after. A nil Before/After means
// the key was absent (or JSON null — the two are not
// distinguished here, as in the TS diff rows, whose undefined
// serializes the same way).
type WorldConfigDiffEntry struct {
	Path   string
	Before any
	After  any
}

// WorldConfigPlan is WorldConfigPlan (WorldConfigStore.ts:34-39).
type WorldConfigPlan struct {
	NextLocal map[string]any
	Diff      []WorldConfigDiffEntry
	Warnings  []string
	Errors    []string
}

// CatalogPlugin is CatalogPlugin (WorldConfigCatalog.ts:16-35).
type CatalogPlugin struct {
	Name           string
	File           string
	Area           string
	Description    string
	Members        bool
	DependsOn      []string
	DefaultEnabled bool
	ShippedSwitch  *bool
	LocalSwitch    *bool
	Enabled        bool
	Loaded         bool
	AwaitingLoad   bool
	PackageID      string
}

// CatalogOption is CatalogOption (WorldConfigCatalog.ts:37-44).
type CatalogOption struct {
	Key          string
	Owner        string
	Decl         ConfigOptionDeclaration
	Shipped      any
	HasShipped   bool
	Local        any
	HasLocal     bool
	OwnerEnabled bool
}

// WorldConfigCatalogData is WorldConfigCatalogData
// (WorldConfigCatalog.ts:46-51). The catalog itself is built by
// scanning the plugin directory (WorldConfigCatalog.build), which
// is plugin-system work outside this package; Plan consumes a
// caller-built catalog.
type WorldConfigCatalogData struct {
	Plugins      []CatalogPlugin
	Options      []CatalogOption
	World        []CatalogOption
	MembersWorld bool
}

// ConfigOptionType is ConfigOptionType
// (WorldSettingDeclarations.ts:3).
type ConfigOptionType string

const (
	ConfigTypeBoolean ConfigOptionType = "boolean"
	ConfigTypeNumber  ConfigOptionType = "number"
	ConfigTypeString  ConfigOptionType = "string"
	ConfigTypeEnum    ConfigOptionType = "enum"
	ConfigTypeDate    ConfigOptionType = "date"
	ConfigTypeJSON    ConfigOptionType = "json"
)

// ConfigEnumValue is one enum values entry.
type ConfigEnumValue struct {
	Value any
	Label string
}

// ConfigOptionDeclaration is ConfigOptionDeclaration
// (WorldSettingDeclarations.ts:5-20). Exactly one of Default
// (HasDefault) / DefaultText (HasDefaultText) is set. Min/Max are
// nil when unset. Values is nil except for enum options.
type ConfigOptionDeclaration struct {
	Type           ConfigOptionType
	Label          string
	Help           string
	Default        any
	HasDefault     bool
	DefaultText    string
	HasDefaultText bool
	Values         []ConfigEnumValue
	Min            *float64
	Max            *float64
	Integer        bool
	ExclusiveMin   bool
	Members        bool
}

// WorldSettingDeclaration is one entry of
// WORLD_SETTING_DECLARATIONS with its key, in declaration order.
type WorldSettingDeclaration struct {
	Key  string
	Decl ConfigOptionDeclaration
}

// WorldSettingDeclarations returns WORLD_SETTING_DECLARATIONS
// (WorldSettingDeclarations.ts:197-220) in declaration order:
// the scalar top-level world.json settings.
func WorldSettingDeclarations() []WorldSettingDeclaration {
	minZero := 0.0
	return []WorldSettingDeclaration{
		{Key: "name", Decl: ConfigOptionDeclaration{
			Type: "string", Label: "World name", Default: "Old School RuneScape", HasDefault: true,
			Help: "Shown on the world list. Plain text, 1 to 64 characters.",
		}},
		{Key: "description", Decl: ConfigOptionDeclaration{
			Type: "string", Label: "Description", Default: "", HasDefault: true,
			Help: "Short blurb shown with the world, at most 120 characters.",
		}},
		{Key: "experienceMultiplier", Decl: ConfigOptionDeclaration{
			Type: "number", Label: "Experience rate", Default: float64(1), HasDefault: true,
			Min: &minZero, ExclusiveMin: true,
			Help: "Global experience rate for all skills. 1 is the normal rate; 5 gives five times the experience.",
		}},
		{Key: "spawn", Decl: ConfigOptionDeclaration{
			Type: "json", Label: "Spawn point", DefaultText: "Required: set in world.json", HasDefaultText: true,
			Help: `Where new and stranded players appear, as { "x": 3222, "y": 3218, "z": 0 }.`,
		}},
		{Key: "gameframe", Decl: ConfigOptionDeclaration{
			Type: "enum", Label: "Game frame", DefaultText: "Client default", HasDefaultText: true,
			Values: []ConfigEnumValue{
				{Value: "modern-resizable", Label: "modern-resizable"},
				{Value: "modern-fixed", Label: "modern-fixed"},
				{Value: "317-resizable", Label: "317-resizable"},
				{Value: "317-fixed", Label: "317-fixed"},
			},
			Help: "Interface layout the web client uses for this world.",
		}},
	}
}

// CoreConfigDeclarations returns CORE_CONFIG_DECLARATIONS
// (WorldSettingDeclarations.ts:223-228): options the server core
// reads from pluginConfig, owned by "core".
func CoreConfigDeclarations() []WorldSettingDeclaration {
	return []WorldSettingDeclaration{
		{Key: "commands:permissions", Decl: ConfigOptionDeclaration{
			Type: "json", Label: "Command permissions", Default: map[string]any{}, HasDefault: true,
			Help: `Lowest rank that may run each command, e.g. { "items": "NONE", "teleports": "OWNER" }. Ranks: NONE, MODERATOR, ADMINISTRATOR, OWNER, DEVELOPER.`,
		}},
	}
}

// WorldSettingKeys returns the keys changes.world may carry: the
// declared top-level world settings, in declaration order
// (WorldConfigStore.worldSettingKeys).
func WorldSettingKeys() []string {
	decls := WorldSettingDeclarations()
	keys := make([]string, len(decls))
	for i, decl := range decls {
		keys[i] = decl.Key
	}
	return keys
}

// WorldConfigLayers reads both world-config layers from the
// Store's definitions directory on demand, so Load does not
// require world.json. See ReadWorldConfigLayers.
func (s *Store) WorldConfigLayers() (WorldConfigLayers, error) {
	return ReadWorldConfigLayers(s.dir)
}

// ReadWorldConfigLayers is WorldConfigStore.read
// (WorldConfigStore.ts:52-69): shipped world.json is required and
// must be an object; a missing world.local.json yields an empty
// local layer; an unparseable local file yields LocalError and an
// empty local layer, never an error return. Both layers are
// normalized independently.
func ReadWorldConfigLayers(dir string) (WorldConfigLayers, error) {
	shippedValue, err := readJSONValue(filepath.Join(dir, "world.json"))
	if err != nil {
		return WorldConfigLayers{}, err
	}
	shippedFile, ok := shippedValue.(map[string]any)
	if !ok {
		return WorldConfigLayers{}, fmt.Errorf("world.json must be an object")
	}
	shipped, err := NormalizeWorldPluginConfig(shippedFile)
	if err != nil {
		return WorldConfigLayers{}, err
	}
	localFile := filepath.Join(dir, "world.local.json")
	raw, err := os.ReadFile(localFile)
	if err != nil {
		if os.IsNotExist(err) {
			return WorldConfigLayers{Shipped: shipped, Local: map[string]any{}, LocalExists: false, LocalHash: ""}, nil
		}
		return WorldConfigLayers{}, fmt.Errorf("reading %s: %w", localFile, err)
	}
	hash := sha256Hex(raw)
	var parsed any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return WorldConfigLayers{Shipped: shipped, Local: map[string]any{}, LocalExists: true, LocalHash: hash, LocalError: err.Error()}, nil
	}
	parsedObject, ok := parsed.(map[string]any)
	if !ok {
		return WorldConfigLayers{Shipped: shipped, Local: map[string]any{}, LocalExists: true, LocalHash: hash, LocalError: "world.local.json must be an object"}, nil
	}
	local, err := NormalizeWorldPluginConfig(parsedObject)
	if err != nil {
		return WorldConfigLayers{Shipped: shipped, Local: map[string]any{}, LocalExists: true, LocalHash: hash, LocalError: err.Error()}, nil
	}
	return WorldConfigLayers{Shipped: shipped, Local: local, LocalExists: true, LocalHash: hash}, nil
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// isJSONObject is the isObject/isPlainObject check shared by the
// world-config files: a non-nil object that is not an array.
func isJSONObject(value any) bool {
	_, ok := value.(map[string]any)
	return ok
}

// deepCopyValue clones JSON-shaped values (structuredClone for
// the shapes involved), preserving Go scalar types.
func deepCopyValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, elem := range v {
			out[key] = deepCopyValue(elem)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = deepCopyValue(elem)
		}
		return out
	default:
		return value
	}
}

func deepCopyObject(obj map[string]any) map[string]any {
	if obj == nil {
		return map[string]any{}
	}
	copied, _ := deepCopyValue(obj).(map[string]any)
	return copied
}

// deepEqualJSON is the deepEqual of WorldConfigStore.ts:41-43
// (JSON.stringify comparison), with one documented deviation:
// Go's encoder sorts object keys, so objects that differ only in
// key order compare equal here, where TypeScript's key-order-
// sensitive stringify would call them different. Numbers compare
// by their JSON text, so int(1) and float64(1) are equal.
func deepEqualJSON(a, b any) bool {
	ab, errA := json.Marshal(a)
	bb, errB := json.Marshal(b)
	if errA != nil || errB != nil {
		return false
	}
	return string(ab) == string(bb)
}

// --- Plugin names and switches -----------------------------------

var pluginNameStrip = regexp.MustCompile(`[^a-z0-9]`)

// NormalizePluginName is normalizePluginName
// (WorldPluginConfig.ts:9-11): lowercase, strip everything but
// a-z0-9.
func NormalizePluginName(name string) string {
	return pluginNameStrip.ReplaceAllString(strings.ToLower(name), "")
}

// PluginNameFromFile is pluginNameFromFile (PluginManager.ts:
// 150-152): the file's base name with a trailing .plugin.js
// (case-insensitive) stripped.
func PluginNameFromFile(pluginPath string) string {
	base := path.Base(pluginPath)
	if len(base) >= 10 && strings.EqualFold(base[len(base)-10:], ".plugin.js") {
		return base[:len(base)-10]
	}
	return base
}

// ResolvePluginEnabled is resolvePluginEnabled
// (PluginManager.ts:159-172): the plugins switch by exported
// name, then by file name, then the legacy disabledPlugins
// default, then the plugin's own enabled (default on).
// defaultEnabled nil means "no legacy default"; pluginEnabled nil
// means "no plugin default" (which counts as on, as undefined
// !== false does in TS).
func ResolvePluginEnabled(switches map[string]bool, defaultEnabled *bool, exportedName, fileName string, pluginEnabled *bool) bool {
	if v, ok := switches[NormalizePluginName(exportedName)]; ok {
		return v
	}
	if v, ok := switches[NormalizePluginName(fileName)]; ok {
		return v
	}
	if defaultEnabled != nil {
		return *defaultEnabled
	}
	return pluginEnabled == nil || *pluginEnabled
}

// ParsePluginSwitches is PluginManager.parsePluginSwitches
// (PluginManager.ts:2247-2278): the plugin switches out of an
// already merged world config. It throws (returns an error) when
// they are malformed. The legacy disabledPlugins form means
// "these off, everything else on" and ignores `plugins`
// entirely, except that FreeToPlay always takes its explicit
// switch. The TS deprecation console.warn is not emitted here;
// the returned switches are identical.
func ParsePluginSwitches(worldConfig map[string]any) (map[string]bool, *bool, error) {
	if disabledValue, present := worldConfig["disabledPlugins"]; present {
		disabled, ok := disabledValue.([]any)
		if !ok {
			return nil, nil, fmt.Errorf("[plugins] world.json disabledPlugins must be a string[]")
		}
		for _, name := range disabled {
			text, ok := name.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, nil, fmt.Errorf("[plugins] world.json disabledPlugins must be a string[]")
			}
		}
		switches := make(map[string]bool, len(disabled))
		for _, name := range disabled {
			switches[NormalizePluginName(name.(string))] = false
		}
		freeToPlay := IsFreeToPlayWorld(worldConfig)
		switches["freetoplay"] = freeToPlay
		defaultEnabled := true
		return switches, &defaultEnabled, nil
	}
	pluginsValue, present := worldConfig["plugins"]
	if !present || pluginsValue == nil {
		return make(map[string]bool), nil, nil
	}
	plugins, ok := pluginsValue.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf(`[plugins] world.json "plugins" must map plugin names to true or false`)
	}
	switches := make(map[string]bool, len(plugins))
	for name, enabled := range plugins {
		enabledBool, ok := enabled.(bool)
		if !ok {
			return nil, nil, fmt.Errorf(`[plugins] world.json "plugins" must map plugin names to true or false`)
		}
		switches[NormalizePluginName(name)] = enabledBool
	}
	return switches, nil, nil
}

// --- World plugin config -----------------------------------------

// IsFreeToPlayWorld is isFreeToPlayWorld (WorldPluginConfig.ts:
// 14-23): the FreeToPlay switch, by normalized plugin name, with
// an object entry's enabled field counting. When several entries
// normalize to the same name the last one wins; iteration is over
// sorted keys, since Go maps carry no insertion order (the TS
// order is the file's key order — real configs carry one entry).
func IsFreeToPlayWorld(world map[string]any) bool {
	plugins, ok := world["plugins"].(map[string]any)
	if !ok {
		return false
	}
	enabled := false
	for _, name := range sortedKeys(plugins) {
		if NormalizePluginName(name) != "freetoplay" {
			continue
		}
		value := plugins[name]
		if entry, ok := value.(map[string]any); ok {
			value = entry["enabled"]
		}
		if b, ok := value.(bool); ok {
			enabled = b
		}
	}
	return enabled
}

func sortedKeys(obj map[string]any) []string {
	keys := make([]string, 0, len(obj))
	for key := range obj {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// legacyNamespaces is LEGACY_NAMESPACES (WorldPluginConfig.ts:
// 26-31): plugins whose exported names differ from their setting
// prefixes.
var legacyNamespaces = map[string]string{
	"characters":   "characters",
	"leaguemode":   "league",
	"quests":       "quests",
	"randomevents": "random-events",
}

// PluginConfigKey is pluginConfigKey (WorldPluginConfig.ts:33-35).
func PluginConfigKey(plugin, setting string) string {
	if prefix, ok := legacyNamespaces[strings.ToLower(plugin)]; ok {
		return prefix + ":" + setting
	}
	return plugin + ":" + setting
}

var pluginSettingKeyPattern = regexp.MustCompile(`^[^:\s]+$`)
var pluginSettingBadPlugin = regexp.MustCompile(`[:\s]`)

// NormalizeWorldPluginConfig is normalizeWorldPluginConfig
// (WorldPluginConfig.ts:38-63): object-form plugin entries split
// into a boolean switch plus flattened pluginConfig settings, so
// either syntax in the local file can override the shipped one.
// Each file is normalized separately. It returns an error for a
// non-boolean enabled or an invalid setting key, as the TS throw
// does. The scoped-settings recording (for PluginManager) is not
// ported.
func NormalizeWorldPluginConfig(world map[string]any) (map[string]any, error) {
	plugins, isObj := world["plugins"].(map[string]any)
	if !isObj {
		return migrateCrossFloorVisibility(world), nil
	}
	hasObjectEntry := false
	for _, entry := range plugins {
		if isJSONObject(entry) {
			hasObjectEntry = true
			break
		}
	}
	if !hasObjectEntry {
		return migrateCrossFloorVisibility(world), nil
	}
	switches := make(map[string]any)
	var settings [][2]any
	for _, plugin := range sortedKeys(plugins) {
		entry := plugins[plugin]
		entryObject, ok := entry.(map[string]any)
		if !ok {
			switches[plugin] = entry
			continue
		}
		enabled, hasEnabled := entryObject["enabled"]
		if hasEnabled {
			enabledBool, ok := enabled.(bool)
			if !ok {
				return nil, fmt.Errorf("plugins.%s.enabled must be true or false", plugin)
			}
			switches[plugin] = enabledBool
		}
		for _, key := range sortedKeys(entryObject) {
			if key == "enabled" {
				continue
			}
			if !pluginSettingKeyPattern.MatchString(key) || pluginSettingBadPlugin.MatchString(plugin) {
				return nil, fmt.Errorf("Invalid plugin setting plugins.%s.%s", plugin, key)
			}
			settings = append(settings, [2]any{PluginConfigKey(plugin, key), entryObject[key]})
		}
	}
	pluginConfig := make(map[string]any)
	if existing, ok := world["pluginConfig"].(map[string]any); ok {
		for key, value := range existing {
			pluginConfig[key] = value
		}
	}
	for _, setting := range settings {
		pluginConfig[setting[0].(string)] = setting[1]
	}
	normalized := make(map[string]any, len(world))
	for key, value := range world {
		normalized[key] = value
	}
	normalized["plugins"] = switches
	normalized["pluginConfig"] = pluginConfig
	return migrateCrossFloorVisibility(normalized), nil
}

// isCrossFloorName reports whether a plugin name is the CrossFloor
// plugin, by normalized name (WorldPluginConfig.ts:65-67).
func isCrossFloorName(name string) bool {
	return NormalizePluginName(name) == "crossfloor"
}

// migrateCrossFloorVisibility is migrateCrossFloorVisibility
// (WorldPluginConfig.ts:70-88): the legacy CrossFloor:visibility
// setting becomes the CrossFloor switch, per layer, so a local
// legacy value can still override a shipped switch.
func migrateCrossFloorVisibility(world map[string]any) map[string]any {
	if pluginsValue, present := world["plugins"]; present && !isJSONObject(pluginsValue) {
		return world
	}
	plugins, _ := world["plugins"].(map[string]any)
	var switchNames []string
	for _, name := range sortedKeys(plugins) {
		if isCrossFloorName(name) {
			switchNames = append(switchNames, name)
		}
	}
	config, _ := world["pluginConfig"].(map[string]any)
	visibility, hasVisibility := config["CrossFloor:visibility"]
	if !hasVisibility && (len(switchNames) == 0 || (len(switchNames) == 1 && switchNames[0] == "CrossFloor")) {
		return world
	}
	var enabled any
	if len(switchNames) > 0 {
		// The TS code takes the LAST switch in file order; Go
		// maps carry no file order, so the sorted-last name
		// wins. Real configs carry one CrossFloor entry.
		enabled = plugins[switchNames[len(switchNames)-1]]
	} else {
		enabled = visibility
	}
	result := make(map[string]any, len(world))
	for key, value := range world {
		result[key] = value
	}
	if hasVisibility {
		options := make(map[string]any, len(config))
		for key, value := range config {
			if key != "CrossFloor:visibility" {
				options[key] = value
			}
		}
		result["pluginConfig"] = options
	}
	if len(switchNames) > 0 {
		remaining := make(map[string]any, len(plugins))
		for name, value := range plugins {
			if !isCrossFloorName(name) {
				remaining[name] = value
			}
		}
		remaining["CrossFloor"] = enabled
		result["plugins"] = remaining
	} else if _, isBool := enabled.(bool); isBool {
		remaining := make(map[string]any, len(plugins))
		for name, value := range plugins {
			if !isCrossFloorName(name) {
				remaining[name] = value
			}
		}
		remaining["CrossFloor"] = enabled
		result["plugins"] = remaining
	}
	return result
}

// PreserveLegacyCrossFloorDefault is
// preserveLegacyCrossFloorDefault (WorldPluginConfig.ts:91-99):
// the old disabledPlugins list defaulted every plugin on, but
// CrossFloor was opt-in even on those worlds, so it is appended
// to the list unless configured on or the environment enables it.
func PreserveLegacyCrossFloorDefault(world map[string]any) map[string]any {
	disabled, ok := world["disabledPlugins"].([]any)
	if !ok {
		return world
	}
	for _, name := range disabled {
		if text, ok := name.(string); ok && isCrossFloorName(text) {
			return world
		}
	}
	var enabled bool
	configuredSet := false
	if plugins, ok := world["plugins"].(map[string]any); ok {
		if configured, present := plugins["CrossFloor"]; present {
			if b, ok := configured.(bool); ok {
				enabled = b
				configuredSet = true
			}
		}
	}
	if !configuredSet {
		env, present := os.LookupEnv("CROSS_FLOOR_VISIBILITY")
		if !present {
			env, _ = os.LookupEnv("CROSS_FLOOR_VISIBILITY_AND_COMBAT")
		}
		enabled = env == "1"
	}
	if enabled {
		return world
	}
	result := make(map[string]any, len(world))
	for key, value := range world {
		result[key] = value
	}
	next := append(append([]any{}, disabled...), "CrossFloor")
	result["disabledPlugins"] = next
	return result
}

// --- World definition --------------------------------------------

// MergeWorldConfig is mergeWorldConfig (WorldDefinition.ts:
// 52-64): world.json with world.local.json layered over it — each
// top-level key in local replaces shipped's, except pluginConfig
// and plugins, which merge key by key. Both layers are normalized
// first. It returns an error when either layer fails
// normalization, as the TS throw does.
func MergeWorldConfig(shipped, local map[string]any) (map[string]any, error) {
	normalizedShipped, err := NormalizeWorldPluginConfig(shipped)
	if err != nil {
		return nil, err
	}
	normalizedLocal, err := NormalizeWorldPluginConfig(local)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(normalizedShipped)+len(normalizedLocal))
	for key, value := range normalizedShipped {
		merged[key] = value
	}
	for key, value := range normalizedLocal {
		merged[key] = value
	}
	for _, key := range []string{"pluginConfig", "plugins"} {
		shippedValue, shippedOk := normalizedShipped[key].(map[string]any)
		localValue, localOk := normalizedLocal[key].(map[string]any)
		if shippedOk && localOk {
			combined := make(map[string]any, len(shippedValue)+len(localValue))
			for k, v := range shippedValue {
				combined[k] = v
			}
			for k, v := range localValue {
				combined[k] = v
			}
			merged[key] = combined
		}
	}
	return PreserveLegacyCrossFloorDefault(merged), nil
}

// WorldDefinitionError is WorldDefinitionValidationError
// (WorldDefinition.ts:41): a world-definition validation failure.
// The message text matches the TypeScript throws exactly.
type WorldDefinitionError struct {
	Message string
}

func (e *WorldDefinitionError) Error() string { return e.Message }

func worldDefinitionError(format string, args ...any) *WorldDefinitionError {
	return &WorldDefinitionError{Message: fmt.Sprintf(format, args...)}
}

// WorldPosition is WorldPosition (WorldDefinition.ts:8-12).
type WorldPosition struct {
	X, Y, Z int
}

// WorldZone is WorldZone (WorldDefinition.ts:14-21). Nil bounds
// mean the zone is unbounded (covers everything).
type WorldZone struct {
	MinX, MaxX, MinY, MaxY, Z *int
	Tags                      []string
}

// WorldDefinitionData is WorldDefinitionData
// (WorldDefinition.ts:29-46).
type WorldDefinitionData struct {
	Name                 string
	Spawn                WorldPosition
	Description          string
	Zones                []WorldZone
	ExperienceMultiplier float64
	Gameframe            string
	HasGameframe         bool
	MembersWorld         bool
	PluginConfig         map[string]any
	Plugins              map[string]any
	HasPlugins           bool
	DisabledPlugins      any
	HasDisabledPlugins   bool
}

func worldObject(value any, label string) (map[string]any, error) {
	obj, ok := value.(map[string]any)
	if !ok || obj == nil {
		return nil, worldDefinitionError("%s must be an object", label)
	}
	return obj, nil
}

func worldInteger(source map[string]any, key, label string) (int, error) {
	value, present := source[key]
	if !present {
		return 0, worldDefinitionError("%s.%s must be an integer", label, key)
	}
	n, ok := numberValue(value)
	if !ok || !isJSInteger(n) {
		return 0, worldDefinitionError("%s.%s must be an integer", label, key)
	}
	return jsTrunc(n), nil
}

func worldPositiveNumber(source map[string]any, key, label string) (float64, error) {
	value, present := source[key]
	if !present {
		return 0, worldDefinitionError("%s.%s must be a positive number", label, key)
	}
	n, ok := numberValue(value)
	if !ok || n <= 0 {
		return 0, worldDefinitionError("%s.%s must be a positive number", label, key)
	}
	return n, nil
}

func worldCoordinate(source map[string]any, key, label string) (int, error) {
	value, err := worldInteger(source, key, label)
	if err != nil {
		return 0, err
	}
	if value < 0 || value > 0x3fff {
		return 0, worldDefinitionError("%s.%s is outside the world", label, key)
	}
	return value, nil
}

func worldPlane(source map[string]any, label string) (int, error) {
	value, err := worldInteger(source, "z", label)
	if err != nil {
		return 0, err
	}
	if value < 0 || value > 3 {
		return 0, worldDefinitionError("%s.z must be between 0 and 3", label)
	}
	return value, nil
}

// jsWhitespace is the ECMAScript WhiteSpace + LineTerminator set
// String.trim removes (also used by jsParseNumber).
const jsWhitespace = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u202f\u205f\u3000\ufeff\u2028\u2029"

func jsTrim(s string) string { return strings.Trim(s, jsWhitespace) }

var worldNameBadChars = regexp.MustCompile(`[\x00-\x1f\x7f<>]`)

// ParseWorldPosition is parseWorldPosition
// (WorldDefinition.ts:103-110).
func ParseWorldPosition(value any, label string) (WorldPosition, error) {
	if label == "" {
		label = "world spawn"
	}
	position, err := worldObject(value, label)
	if err != nil {
		return WorldPosition{}, err
	}
	x, err := worldCoordinate(position, "x", label)
	if err != nil {
		return WorldPosition{}, err
	}
	y, err := worldCoordinate(position, "y", label)
	if err != nil {
		return WorldPosition{}, err
	}
	z, err := worldPlane(position, label)
	if err != nil {
		return WorldPosition{}, err
	}
	return WorldPosition{X: x, Y: y, Z: z}, nil
}

var zoneBoundKeys = []string{"minX", "maxX", "minY", "maxY", "z"}

// ParseWorldZone is parseWorldZone (WorldDefinition.ts:114-140).
func ParseWorldZone(value any, label string) (WorldZone, error) {
	if label == "" {
		label = "world zone"
	}
	zone, err := worldObject(value, label)
	if err != nil {
		return WorldZone{}, err
	}
	bounded := false
	for _, key := range zoneBoundKeys {
		if _, present := zone[key]; present {
			bounded = true
			break
		}
	}
	parsed := WorldZone{Tags: []string{}}
	if bounded {
		minX, err := worldCoordinate(zone, "minX", label)
		if err != nil {
			return WorldZone{}, err
		}
		maxX, err := worldCoordinate(zone, "maxX", label)
		if err != nil {
			return WorldZone{}, err
		}
		minY, err := worldCoordinate(zone, "minY", label)
		if err != nil {
			return WorldZone{}, err
		}
		maxY, err := worldCoordinate(zone, "maxY", label)
		if err != nil {
			return WorldZone{}, err
		}
		z, err := worldPlane(zone, label)
		if err != nil {
			return WorldZone{}, err
		}
		parsed.MinX, parsed.MaxX, parsed.MinY, parsed.MaxY, parsed.Z = &minX, &maxX, &minY, &maxY, &z
		if minX > maxX || minY > maxY {
			return WorldZone{}, worldDefinitionError("%s has reversed bounds", label)
		}
	}
	tagsValue, present := zone["tags"]
	if !present {
		return WorldZone{}, worldDefinitionError("%s.tags must be a non-empty array", label)
	}
	tags, ok := tagsValue.([]any)
	if !ok {
		return WorldZone{}, worldDefinitionError("%s.tags must be a non-empty array", label)
	}
	if bounded && len(tags) == 0 {
		return WorldZone{}, worldDefinitionError("%s.tags must be a non-empty array", label)
	}
	seenTags := make(map[string]bool)
	for _, tag := range tags {
		text, ok := tag.(string)
		if !ok {
			return WorldZone{}, worldDefinitionError("%s.tags must contain strings", label)
		}
		if seenTags[text] {
			continue
		}
		seenTags[text] = true
		parsed.Tags = append(parsed.Tags, text)
	}
	return parsed, nil
}

// ZoneContains is zoneContains (WorldDefinition.ts:143-146): true
// when the zone covers (x, y) on any plane; a zone without bounds
// covers everything.
func ZoneContains(zone WorldZone, x, y int) bool {
	if zone.MinX == nil {
		return true
	}
	return x >= *zone.MinX && x <= *zone.MaxX && y >= *zone.MinY && y <= *zone.MaxY
}

// ParseWorldDefinition is parseWorldDefinition
// (WorldDefinition.ts:148-188). Error messages match the
// TypeScript throws exactly.
func ParseWorldDefinition(value any) (*WorldDefinitionData, error) {
	world, err := worldObject(value, "world.json")
	if err != nil {
		return nil, err
	}
	zonesValue, present := world["zones"]
	if !present {
		return nil, worldDefinitionError("world.json zones must be an array")
	}
	zonesRaw, ok := zonesValue.([]any)
	if !ok {
		return nil, worldDefinitionError("world.json zones must be an array")
	}
	experienceMultiplier := 1.0
	if _, present := world["experienceMultiplier"]; present {
		multiplier, err := worldPositiveNumber(world, "experienceMultiplier", "world.json")
		if err != nil {
			return nil, err
		}
		experienceMultiplier = multiplier
	}
	if gameframeValue, present := world["gameframe"]; present {
		gameframe, ok := gameframeValue.(string)
		if !ok {
			return nil, worldDefinitionError("world.json gameframe must be one of %s", strings.Join(WorldGameframes, ", "))
		}
		valid := false
		for _, frame := range WorldGameframes {
			if gameframe == frame {
				valid = true
				break
			}
		}
		if !valid {
			return nil, worldDefinitionError("world.json gameframe must be one of %s", strings.Join(WorldGameframes, ", "))
		}
	}
	nameValue, namePresent := world["name"]
	name := "Old School RuneScape"
	if namePresent {
		nameText, ok := nameValue.(string)
		if !ok || jsTrim(nameText) == "" || utf16Len(jsTrim(nameText)) > 64 || worldNameBadChars.MatchString(nameText) {
			return nil, worldDefinitionError("world.json name must be plain text between 1 and 64 characters")
		}
		name = nameText
	}
	membersWorld := !IsFreeToPlayWorld(world)
	descriptionValue, descriptionPresent := world["description"]
	description := ""
	if descriptionPresent && descriptionValue != nil {
		descriptionText, ok := descriptionValue.(string)
		if !ok || utf16Len(descriptionText) > 120 {
			return nil, worldDefinitionError("world.json description must be a string of at most 120 characters")
		}
		description = descriptionText
	}
	zones := make([]WorldZone, 0, len(zonesRaw))
	for i, rawZone := range zonesRaw {
		parsed, err := ParseWorldZone(rawZone, fmt.Sprintf("world.json zones[%d]", i))
		if err != nil {
			return nil, err
		}
		zones = append(zones, parsed)
	}
	spawn, err := ParseWorldPosition(world["spawn"], "world.json spawn")
	if err != nil {
		return nil, err
	}
	if !membersWorld {
		inside := false
		for _, zone := range zones {
			hasF2P := false
			for _, tag := range zone.Tags {
				if tag == "f2p" {
					hasF2P = true
					break
				}
			}
			if hasF2P && ZoneContains(zone, spawn.X, spawn.Y) {
				inside = true
				break
			}
		}
		if !inside {
			return nil, worldDefinitionError(`FreeToPlay needs the spawn inside a zone tagged "f2p"`)
		}
	}
	data := &WorldDefinitionData{
		Name:                 jsTrim(name),
		Spawn:                spawn,
		Description:          jsTrim(description),
		Zones:                zones,
		ExperienceMultiplier: experienceMultiplier,
		MembersWorld:         membersWorld,
	}
	if gameframeValue, present := world["gameframe"]; present {
		data.Gameframe, _ = gameframeValue.(string)
		data.HasGameframe = true
	}
	if pluginConfig, ok := world["pluginConfig"].(map[string]any); ok {
		data.PluginConfig = pluginConfig
	}
	if plugins, ok := world["plugins"].(map[string]any); ok {
		data.Plugins = plugins
		data.HasPlugins = true
	}
	if disabled, present := world["disabledPlugins"]; present {
		data.DisabledPlugins = disabled
		data.HasDisabledPlugins = true
	}
	return data, nil
}

// --- Config value checking ---------------------------------------

// CheckConfigValue is checkConfigValue
// (WorldSettingDeclarations.ts:152-188): it validates (and for
// numbers, coerces) a value against its declaration. Booleans are
// not coerced. It returns the coerced value, or an error whose
// message matches the TS check error. A nil value means JSON null
// (valid for json options); callers skip absent keys before
// calling, as the TS plan loop skips undefined values.
func CheckConfigValue(decl ConfigOptionDeclaration, value any) (any, error) {
	switch decl.Type {
	case ConfigTypeBoolean:
		if b, ok := value.(bool); ok {
			return b, nil
		}
		return nil, fmt.Errorf("must be true or false")
	case ConfigTypeNumber:
		var n float64
		switch v := value.(type) {
		case float64:
			n = v
		case int:
			n = float64(v)
		case string:
			if strings.TrimSpace(v) == "" {
				return nil, fmt.Errorf("must be a number")
			}
			n = jsParseNumber(v)
		default:
			return nil, fmt.Errorf("must be a number")
		}
		if math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, fmt.Errorf("must be a number")
		}
		if decl.Integer && !isJSInteger(n) {
			return nil, fmt.Errorf("must be a whole number")
		}
		if decl.Min != nil {
			if decl.ExclusiveMin {
				if n <= *decl.Min {
					return nil, fmt.Errorf("must be greater than %s", jsFormatNumber(*decl.Min))
				}
			} else if n < *decl.Min {
				return nil, fmt.Errorf("must be at least %s", jsFormatNumber(*decl.Min))
			}
		}
		if decl.Max != nil && n > *decl.Max {
			return nil, fmt.Errorf("must be at most %s", jsFormatNumber(*decl.Max))
		}
		return n, nil
	case ConfigTypeString:
		if s, ok := value.(string); ok {
			return s, nil
		}
		return nil, fmt.Errorf("must be text")
	case ConfigTypeEnum:
		var asNumber any = value
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			asNumber = jsParseNumber(s)
		}
		for _, option := range decl.Values {
			if configValueStrictEqual(option.Value, value) {
				return option.Value, nil
			}
			if _, isNumber := option.Value.(float64); isNumber {
				if num, ok := asNumber.(float64); ok && configValueStrictEqual(option.Value, num) {
					return option.Value, nil
				}
			}
			if _, isNumber := option.Value.(int); isNumber {
				if num, ok := asNumber.(float64); ok && configValueStrictEqual(option.Value, num) {
					return option.Value, nil
				}
			}
		}
		values := make([]string, len(decl.Values))
		for i, option := range decl.Values {
			values[i] = jsString(option.Value, true)
		}
		return nil, fmt.Errorf("must be one of %s", strings.Join(values, ", "))
	case ConfigTypeDate:
		s, ok := value.(string)
		if !ok || !datePattern.MatchString(s) {
			return nil, fmt.Errorf("must be a date as YYYY-MM-DD")
		}
		if _, err := time.Parse("2006-01-02", s); err != nil {
			return nil, fmt.Errorf("must be a real calendar date")
		}
		return s, nil
	case ConfigTypeJSON:
		return value, nil
	default:
		return nil, fmt.Errorf("unknown option type")
	}
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// configValueStrictEqual is JavaScript === for the JSON-shaped
// values option declarations carry: numbers compare by value
// across Go int/float64, strings and booleans by content, nil by
// identity. NaN never equals, as in JS.
func configValueStrictEqual(a, b any) bool {
	an, aIsNum := configNumber(a)
	bn, bIsNum := configNumber(b)
	if aIsNum && bIsNum {
		return an == bn
	}
	if aIsNum || bIsNum {
		return false
	}
	switch av := a.(type) {
	case nil:
		return b == nil
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	default:
		return false
	}
}

func configNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

// ValidateDeclaration is validateDeclaration
// (WorldSettingDeclarations.ts:77-127): problems with a plugin's
// option declaration, as raw JSON-shaped data; an empty list
// means it is valid. Error messages match the TypeScript strings.
func ValidateDeclaration(key string, decl any) []string {
	var problems []string
	if !declarationKeyPattern.MatchString(key) {
		problems = append(problems, fmt.Sprintf("%s: key must be namespace:key", key))
	}
	declMap, ok := decl.(map[string]any)
	if !ok {
		problems = append(problems, fmt.Sprintf("%s: declaration must be an object", key))
		return problems
	}
	declType, _ := declMap["type"].(string)
	validType := false
	for _, t := range []string{"boolean", "number", "string", "enum", "date", "json"} {
		if declType == t {
			validType = true
			break
		}
	}
	if !validType {
		problems = append(problems, fmt.Sprintf("%s: type must be one of boolean, number, string, enum, date, json", key))
	}
	if label, ok := declMap["label"].(string); !ok || strings.TrimSpace(label) == "" {
		problems = append(problems, fmt.Sprintf("%s: label is required", key))
	}
	if help, ok := declMap["help"].(string); !ok || strings.TrimSpace(help) == "" {
		problems = append(problems, fmt.Sprintf("%s: help is required", key))
	}
	_, hasDefault := declMap["default"]
	_, hasText := declMap["defaultText"]
	if hasDefault == hasText {
		problems = append(problems, fmt.Sprintf("%s: set exactly one of default or defaultText", key))
	}
	if hasText {
		if text, ok := declMap["defaultText"].(string); !ok || strings.TrimSpace(text) == "" {
			problems = append(problems, fmt.Sprintf("%s: defaultText must be a non-empty string", key))
		}
	}
	knownFields := map[string]bool{
		"type": true, "label": true, "help": true, "default": true, "defaultText": true,
		"values": true, "min": true, "max": true, "integer": true, "exclusiveMin": true, "members": true,
	}
	for _, fieldName := range sortedKeys(declMap) {
		if !knownFields[fieldName] {
			problems = append(problems, fmt.Sprintf("%s: unknown field %s", key, fieldName))
		}
	}
	if declType == "enum" {
		values, ok := declMap["values"].([]any)
		validValues := ok && len(values) > 0
		if validValues {
			for _, v := range values {
				entry, ok := v.(map[string]any)
				if !ok {
					validValues = false
					break
				}
				_, hasString := entry["value"].(string)
				_, hasNumber := entry["value"].(float64)
				_, hasInt := entry["value"].(int)
				_, hasLabel := entry["label"].(string)
				if !(hasString || hasNumber || hasInt) || !hasLabel {
					validValues = false
					break
				}
			}
		}
		if !validValues {
			problems = append(problems, fmt.Sprintf("%s: enum needs a non-empty values list of { value, label }", key))
		} else {
			seen := make(map[string]bool)
			duplicate := false
			for _, v := range values {
				entry := v.(map[string]any)
				marker := enumValueMarker(entry["value"])
				if seen[marker] {
					duplicate = true
					break
				}
				seen[marker] = true
			}
			if duplicate {
				problems = append(problems, fmt.Sprintf("%s: enum values must be unique", key))
			}
		}
	} else if _, present := declMap["values"]; present {
		problems = append(problems, fmt.Sprintf("%s: values is only for enum", key))
	}
	for _, fieldName := range []string{"min", "max", "integer", "exclusiveMin"} {
		if declType != "number" {
			if _, present := declMap[fieldName]; present {
				problems = append(problems, fmt.Sprintf("%s: %s is only for number", key, fieldName))
			}
		}
	}
	for _, fieldName := range []string{"min", "max"} {
		if v, present := declMap[fieldName]; present {
			if _, ok := numberValue(v); !ok {
				problems = append(problems, fmt.Sprintf("%s: %s must be a finite number", key, fieldName))
			}
		}
	}
	for _, fieldName := range []string{"integer", "exclusiveMin", "members"} {
		if v, present := declMap[fieldName]; present {
			if _, ok := v.(bool); !ok {
				problems = append(problems, fmt.Sprintf("%s: %s must be true or false", key, fieldName))
			}
		}
	}
	if _, hasExclusive := declMap["exclusiveMin"]; hasExclusive {
		if _, hasMin := declMap["min"]; !hasMin {
			problems = append(problems, fmt.Sprintf("%s: exclusiveMin needs min", key))
		}
	}
	minNum, minOk := numberValue(declMap["min"])
	maxNum, maxOk := numberValue(declMap["max"])
	if minOk && maxOk && minNum > maxNum {
		problems = append(problems, fmt.Sprintf("%s: min must not be greater than max", key))
	}
	if hasDefault && len(problems) == 0 {
		defaultValue := declMap["default"]
		if typeProblem := defaultTypeProblem(ConfigOptionType(declType), defaultValue); typeProblem != "" {
			problems = append(problems, fmt.Sprintf("%s: default is invalid: %s", key, typeProblem))
		} else if declType == "enum" {
			values := declMap["values"].([]any)
			found := false
			var rendered []string
			for _, v := range values {
				entry := v.(map[string]any)
				if configValueStrictEqual(entry["value"], defaultValue) {
					found = true
				}
				rendered = append(rendered, jsonStringifyShort(entry["value"]))
			}
			if !found {
				problems = append(problems, fmt.Sprintf("%s: default is invalid: must be exactly one of %s", key, strings.Join(rendered, ", ")))
			}
		} else if declType != "json" {
			typed := declarationFromMap(declMap)
			if _, err := CheckConfigValue(typed, defaultValue); err != nil {
				problems = append(problems, fmt.Sprintf("%s: default is invalid: %s", key, err.Error()))
			}
		}
	}
	return problems
}

var declarationKeyPattern = regexp.MustCompile(`^[^:\s]+:[^:\s]+$`)

// enumValueMarker distinguishes enum values the way a JS Set
// does: numbers by value (NaN equals NaN), strings by content,
// with 1 and "1" distinct.
func enumValueMarker(value any) string {
	if n, ok := configNumber(value); ok {
		if math.IsNaN(n) {
			return "number:NaN"
		}
		return "number:" + strconv.FormatFloat(n, 'g', -1, 64)
	}
	if s, ok := value.(string); ok {
		return "string:" + s
	}
	return fmt.Sprintf("other:%v", value)
}

// jsonStringifyShort renders one JSON value the way
// JSON.stringify does for enum default errors.
func jsonStringifyShort(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "null"
	}
	return string(data)
}

// defaultTypeProblem is defaultTypeProblem
// (WorldSettingDeclarations.ts:28-41): a default must already
// have its declared type.
func defaultTypeProblem(optionType ConfigOptionType, value any) string {
	switch optionType {
	case ConfigTypeBoolean:
		if _, ok := value.(bool); ok {
			return ""
		}
		return "must be true or false"
	case ConfigTypeNumber:
		if _, ok := configNumber(value); ok {
			return ""
		}
		return "must be a number, not text"
	case ConfigTypeString, ConfigTypeDate:
		if _, ok := value.(string); ok {
			return ""
		}
		return "must be text"
	case ConfigTypeEnum:
		if _, ok := value.(string); ok {
			return ""
		}
		if _, ok := configNumber(value); ok {
			return ""
		}
		return "must be one of the values"
	case ConfigTypeJSON:
		if isJSONValue(value) {
			return ""
		}
		return "must be a JSON value"
	default:
		return "unknown option type"
	}
}

// isJSONValue is isJsonValue (WorldSettingDeclarations.ts:43-51).
func isJSONValue(value any) bool {
	switch v := value.(type) {
	case nil, string, bool:
		return true
	case float64:
		return !math.IsNaN(v) && !math.IsInf(v, 0)
	case int:
		return true
	case []any:
		for _, elem := range v {
			if !isJSONValue(elem) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, elem := range v {
			if !isJSONValue(elem) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// declarationFromMap converts a structurally valid raw declaration
// map into the typed form, for default checking.
func declarationFromMap(declMap map[string]any) ConfigOptionDeclaration {
	decl := ConfigOptionDeclaration{Type: ConfigOptionType(declMap["type"].(string))}
	if label, ok := declMap["label"].(string); ok {
		decl.Label = label
	}
	if help, ok := declMap["help"].(string); ok {
		decl.Help = help
	}
	if v, present := declMap["default"]; present {
		decl.Default = v
		decl.HasDefault = true
	}
	if text, ok := declMap["defaultText"].(string); ok {
		decl.DefaultText = text
		decl.HasDefaultText = true
	}
	if values, ok := declMap["values"].([]any); ok {
		for _, v := range values {
			entry, ok := v.(map[string]any)
			if !ok {
				continue
			}
			label, _ := entry["label"].(string)
			decl.Values = append(decl.Values, ConfigEnumValue{Value: entry["value"], Label: label})
		}
	}
	if n, ok := numberValue(declMap["min"]); ok {
		decl.Min = &n
	}
	if n, ok := numberValue(declMap["max"]); ok {
		decl.Max = &n
	}
	if b, ok := declMap["integer"].(bool); ok {
		decl.Integer = b
	}
	if b, ok := declMap["exclusiveMin"].(bool); ok {
		decl.ExclusiveMin = b
	}
	if b, ok := declMap["members"].(bool); ok {
		decl.Members = b
	}
	return decl
}

// --- Plan and write ----------------------------------------------

// PlanWorldConfig is WorldConfigStore.plan
// (WorldConfigStore.ts:82-241): it applies changes to the local
// layer and checks the merged result, never touching the disk.
// It returns an error only when merging the layers fails
// (normalization of an edited value throws in TS, outside the
// plan's own error collection); validation problems land in
// Plan.Errors instead. Diff rows iterate in sorted key order —
// TypeScript follows the layers' key insertion order, which Go
// maps do not preserve.
func PlanWorldConfig(layers WorldConfigLayers, changes WorldConfigChanges, catalog WorldConfigCatalogData) (WorldConfigPlan, error) {
	nextLocal := deepCopyObject(layers.Local)
	shippedPlugins, _ := layers.Shipped["plugins"].(map[string]any)
	if shippedPlugins == nil {
		shippedPlugins = map[string]any{}
	}
	shippedConfig, _ := layers.Shipped["pluginConfig"].(map[string]any)
	if shippedConfig == nil {
		shippedConfig = map[string]any{}
	}

	enabledSet := func(config map[string]any) map[string]bool {
		switches, defaultEnabled, err := ParsePluginSwitches(config)
		if err != nil {
			return nil
		}
		enabled := make(map[string]bool, len(catalog.Plugins))
		for _, p := range catalog.Plugins {
			var pluginEnabled *bool
			if !p.DefaultEnabled {
				disabled := false
				pluginEnabled = &disabled
			}
			enabled[p.Name] = ResolvePluginEnabled(switches, defaultEnabled, p.Name, PluginNameFromFile(p.File), pluginEnabled)
		}
		return enabled
	}

	// Legacy disabledPlugins becomes explicit plugins entries that
	// keep the enabled set identical: the legacy list means "these
	// off, everything else on" and ignores `plugins` entirely.
	if disabled, ok := nextLocal["disabledPlugins"].([]any); ok {
		mergedBefore, err := MergeWorldConfig(layers.Shipped, layers.Local)
		if err != nil {
			return WorldConfigPlan{}, err
		}
		legacy := enabledSet(mergedBefore)
		plugins := make(map[string]any)
		if existing, ok := nextLocal["plugins"].(map[string]any); ok {
			for key, value := range existing {
				plugins[key] = value
			}
		}
		for _, name := range disabled {
			if text, ok := name.(string); ok && strings.TrimSpace(text) != "" {
				plugins[text] = false
			}
		}
		delete(nextLocal, "disabledPlugins")
		nextLocal["plugins"] = plugins
		mergedAfter, err := MergeWorldConfig(layers.Shipped, nextLocal)
		if err != nil {
			return WorldConfigPlan{}, err
		}
		migrated := enabledSet(mergedAfter)
		if legacy != nil && migrated != nil {
			for _, p := range catalog.Plugins {
				if legacy[p.Name] != migrated[p.Name] {
					plugins[p.Name] = legacy[p.Name]
				}
			}
		}
	}

	var errorsList, warnings []string
	declarations := make(map[string]bool)
	for _, decl := range WorldSettingDeclarations() {
		declarations[decl.Key] = true
	}
	for _, key := range sortedKeys(changes.World) {
		value := changes.World[key]
		// Zones, plugins and pluginConfig are not "world"
		// settings; only declared scalars are.
		if !declarations[key] {
			errorsList = append(errorsList, fmt.Sprintf("world.%s is not a world setting", key))
			continue
		}
		if value == nil {
			delete(nextLocal, key)
			continue
		}
		if shippedValue, present := layers.Shipped[key]; present && deepEqualJSON(value, shippedValue) {
			delete(nextLocal, key)
		} else {
			nextLocal[key] = value
		}
	}
	applySection := func(section string, shipped map[string]any, edits map[string]any) {
		if edits == nil {
			return
		}
		target := make(map[string]any)
		if existing, ok := nextLocal[section].(map[string]any); ok {
			for key, value := range existing {
				target[key] = value
			}
		}
		for _, key := range sortedKeys(edits) {
			value := edits[key]
			if value == nil {
				delete(target, key)
				continue
			}
			if shippedValue, present := shipped[key]; present && deepEqualJSON(value, shippedValue) {
				delete(target, key)
			} else {
				target[key] = value
			}
		}
		if len(target) == 0 {
			delete(nextLocal, section)
		} else {
			nextLocal[section] = target
		}
	}
	applySection("plugins", shippedPlugins, changes.Plugins)
	applySection("pluginConfig", shippedConfig, changes.PluginConfig)

	var diff []WorldConfigDiffEntry
	addDiff := func(path string, before, after any) {
		if !deepEqualJSON(before, after) {
			diff = append(diff, WorldConfigDiffEntry{Path: path, Before: before, After: after})
		}
	}
	keySet := make(map[string]bool)
	for key := range layers.Local {
		keySet[key] = true
	}
	for key := range nextLocal {
		keySet[key] = true
	}
	keys := make([]string, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "plugins" || key == "pluginConfig" {
			before, _ := layers.Local[key].(map[string]any)
			after, _ := nextLocal[key].(map[string]any)
			subSet := make(map[string]bool)
			for sub := range before {
				subSet[sub] = true
			}
			for sub := range after {
				subSet[sub] = true
			}
			subs := make([]string, 0, len(subSet))
			for sub := range subSet {
				subs = append(subs, sub)
			}
			sort.Strings(subs)
			for _, sub := range subs {
				addDiff(key+"."+sub, before[sub], after[sub])
			}
		} else {
			addDiff(key, layers.Local[key], nextLocal[key])
		}
	}

	merged, err := MergeWorldConfig(layers.Shipped, nextLocal)
	if err != nil {
		return WorldConfigPlan{}, err
	}
	if _, err := ParseWorldDefinition(merged); err != nil {
		errorsList = append(errorsList, err.Error())
	}
	switches, defaultEnabled, switchErr := ParsePluginSwitches(merged)
	var switchState bool
	if switchErr != nil {
		errorsList = append(errorsList, switchErr.Error())
	} else {
		switchState = true
	}

	for _, option := range catalog.World {
		value, present := merged[option.Key]
		if !present {
			continue
		}
		if _, err := CheckConfigValue(option.Decl, value); err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %s", option.Key, err.Error()))
		}
	}
	declared := make(map[string]CatalogOption, len(catalog.Options))
	for _, option := range catalog.Options {
		declared[option.Key] = option
	}
	mergedConfig, _ := merged["pluginConfig"].(map[string]any)
	localConfig, _ := nextLocal["pluginConfig"].(map[string]any)
	membersWorld := !IsFreeToPlayWorld(merged)
	for _, key := range sortedKeys(mergedConfig) {
		value := mergedConfig[key]
		option, ok := declared[key]
		if !ok {
			if _, inLocal := localConfig[key]; inLocal {
				warnings = append(warnings, fmt.Sprintf("pluginConfig %q is not declared by any plugin; it is kept as written.", key))
			}
			continue
		}
		if _, err := CheckConfigValue(option.Decl, value); err != nil {
			errorsList = append(errorsList, fmt.Sprintf("%s: %s", key, err.Error()))
		} else if !membersWorld && option.Decl.Members {
			if _, inLocal := localConfig[key]; inLocal {
				warnings = append(warnings, fmt.Sprintf("%s is a members option but this is not a members world.", key))
			}
		}
	}

	if switchState {
		enabled := make(map[string]bool, len(catalog.Plugins))
		for _, p := range catalog.Plugins {
			var pluginEnabled *bool
			if !p.DefaultEnabled {
				disabled := false
				pluginEnabled = &disabled
			}
			enabled[p.Name] = ResolvePluginEnabled(switches, defaultEnabled, p.Name, PluginNameFromFile(p.File), pluginEnabled)
		}
		wasOff := make(map[string]bool)
		off := make(map[string]bool)
		for _, p := range catalog.Plugins {
			if !p.Enabled {
				wasOff[p.Name] = true
			}
			if !enabled[p.Name] {
				off[p.Name] = true
			}
		}
		byName := make(map[string]CatalogPlugin, len(catalog.Plugins))
		for _, p := range catalog.Plugins {
			byName[p.Name] = p
		}
		var cutOff func(p CatalogPlugin, seen map[string]bool) bool
		cutOff = func(p CatalogPlugin, seen map[string]bool) bool {
			for _, dep := range p.DependsOn {
				if off[dep] {
					return true
				}
				if depPlugin, ok := byName[dep]; ok && !seen[dep] {
					seen[dep] = true
					if cutOff(depPlugin, seen) {
						return true
					}
				}
			}
			return false
		}
		var newlyOff []string
		for _, p := range catalog.Plugins {
			if off[p.Name] && !wasOff[p.Name] {
				newlyOff = append(newlyOff, p.Name)
			}
		}
		var affected []string
		for _, p := range catalog.Plugins {
			if enabled[p.Name] && cutOff(p, map[string]bool{}) {
				affected = append(affected, p.Name)
			}
		}
		if len(newlyOff) > 0 && len(affected) > 0 {
			warnings = append(warnings, fmt.Sprintf("Turning off %s also stops plugins that depend on it: %s.", strings.Join(newlyOff, ", "), strings.Join(affected, ", ")))
		}
		if !membersWorld {
			var members []string
			for _, p := range catalog.Plugins {
				if p.Members && enabled[p.Name] {
					members = append(members, p.Name)
				}
			}
			if len(members) > 0 {
				warnings = append(warnings, fmt.Sprintf("Members plugins are switched on but this is not a members world, so they will not load: %s.", strings.Join(members, ", ")))
			}
		}
	}
	return WorldConfigPlan{NextLocal: nextLocal, Diff: diff, Warnings: warnings, Errors: errorsList}, nil
}

// WriteWorldConfig is WorldConfigStore.write
// (WorldConfigStore.ts:243-264): it replaces world.local.json
// atomically. It refuses a stale hash or an unparseable current
// file. The written JSON uses 2-space indent plus a trailing
// newline; Go sorts object keys, where TypeScript preserves
// insertion order — a documented deviation, unobservable to a
// JSON reader.
func WriteWorldConfig(dir string, nextLocal map[string]any, expectedHash string) error {
	file := filepath.Join(dir, "world.local.json")
	current, err := os.ReadFile(file)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("reading %s: %w", file, err)
	}
	var currentHash string
	if err == nil {
		currentHash = sha256Hex(current)
	}
	if currentHash != expectedHash {
		return fmt.Errorf("world.local.json changed since loaded; reload the page and try again")
	}
	if err == nil {
		var parsed any
		if err := json.Unmarshal(current, &parsed); err != nil {
			return fmt.Errorf("world.local.json does not parse (%s); fix it by hand first", err.Error())
		}
		if _, ok := parsed.(map[string]any); !ok {
			return fmt.Errorf("world.local.json does not parse as an object; fix it by hand first")
		}
	}
	temp := filepath.Join(dir, fmt.Sprintf(".world.local.json.%d.%d.tmp", os.Getpid(), time.Now().UnixMilli()))
	writeErr := func() error {
		var buf strings.Builder
		encoder := json.NewEncoder(&buf)
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(nextLocal); err != nil {
			return err
		}
		return os.WriteFile(temp, []byte(buf.String()), 0o644)
	}()
	if writeErr != nil {
		os.Remove(temp)
		return writeErr
	}
	if err := os.Rename(temp, file); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}
