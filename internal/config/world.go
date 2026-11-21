package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

// Location is a tile position in the world: x/y 0-16383, plane z 0-3.
// It mirrors WorldPosition in WorldDefinition.ts.
type Location struct {
	X, Y, Z int
}

// Zone is one world zone from world.json. A zone without bounds
// (Bounded == false) covers the whole world; a bounded zone covers
// the inclusive x/y rectangle [MinX, MaxX] x [MinY, MaxY]. Z is
// stored but matching is x/y only, per zoneContains in
// WorldDefinition.ts. Tags are deduplicated, in first-seen order.
type Zone struct {
	MinX, MaxX, MinY, MaxY, Z int
	Tags                      []string
	Bounded                   bool
}

// World is the parsed, validated world definition: world.json plus the
// optional world.local.json overlay. It mirrors WorldDefinitionData in
// WorldDefinition.ts, except the legacy whole-list disabledPlugins form
// is not carried: it is only read by PluginManager, and this struct is
// the config surface other Go packages code against. (The
// preserveLegacyCrossFloorDefault step, which only appends "CrossFloor"
// to disabledPlugins, is therefore not applied; see LoadWorld.)
type World struct {
	Name        string
	Description string
	// Gameframe is "" when world.json does not force a layout, else one
	// of modern-resizable, modern-fixed, 317-resizable, 317-fixed.
	Gameframe string
	Spawn     Location
	// ExperienceMultiplier is positive; 1 when world.json omits it.
	ExperienceMultiplier float64
	Zones                []Zone
	// Plugins maps plugin names to their on/off switch. Only boolean
	// switches are carried: a plugin entry given as an object
	// contributes its "enabled" flag here and its other options to
	// PluginConfig (see normalizeWorldPluginConfig in
	// WorldPluginConfig.ts); a scalar non-boolean entry is dropped,
	// matching the TypeScript behavior of validating plugin switches
	// where they are read rather than at world-definition parse time.
	Plugins map[string]bool
	// PluginConfig holds per-plugin settings keyed "Plugin:setting"
	// (with the legacy namespace mapping of WorldPluginConfig.ts).
	// Numbers are float64, as in the TypeScript JSON values.
	PluginConfig map[string]any
	// MembersWorld is the negation of the FreeToPlay plugin switch.
	MembersWorld bool

	// freeLandIndex is built by LoadWorld: "f2p" zones indexed per
	// 64x64 map square, as in WorldDefinition.ts (freeLandBySquare).
	// Worlds built literally (not via LoadWorld) have freeLandIndexed
	// false, and IsMembersArea falls back to a direct zone scan so
	// the result is still correct.
	freeLandIndexed  bool
	freeEverywhere   bool
	freeLandBySquare map[int]freeLandEntry
}

// freeLandEntry is one 64x64 square's free-land record: either some
// "f2p" zone covers the whole square, or the list of "f2p" zones
// overlapping it (checked individually by IsMembersArea).
type freeLandEntry struct {
	coversAll bool
	zones     []Zone
}

// worldGameframes are the layouts world.json may force on every login.
// WORLD_GAMEFRAMES in WorldDefinition.ts.
var worldGameframes = []string{"modern-resizable", "modern-fixed", "317-resizable", "317-fixed"}

// LoadWorld reads <dataDir>/definitions/world.json plus the optional
// <dataDir>/definitions/world.local.json overlay and returns the
// validated world definition.
//
// Layering (readWorldConfig / mergeWorldConfig in WorldDefinition.ts):
// each top-level key in the local file replaces the shipped value,
// except pluginConfig and plugins, which merge key-by-key. Each layer
// is plugin-config-normalized separately first, so either syntax in
// the local file can override the shipped one.
//
// Validation errors use the same messages as parseWorldDefinition in
// WorldDefinition.ts and name the bad field. I/O and JSON syntax
// errors name the file that failed.
func LoadWorld(dataDir string) (*World, error) {
	definitionsDir := filepath.Join(dataDir, "definitions")
	shippedPath := filepath.Join(definitionsDir, "world.json")
	shipped, err := readWorldJSONFile(shippedPath, "world.json")
	if err != nil {
		return nil, err
	}

	localPath := filepath.Join(definitionsDir, "world.local.json")
	local, err := readWorldJSONFile(localPath, "world.local.json")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// No overlay: normalize the shipped layer alone, as
			// readWorldConfig does when world.local.json is absent.
			normalized, err := normalizeWorldPluginConfig(shipped)
			if err != nil {
				return nil, err
			}
			return parseWorldDefinition(normalized)
		}
		return nil, err
	}
	merged, err := mergeWorldConfig(shipped, local)
	if err != nil {
		return nil, err
	}
	return parseWorldDefinition(merged)
}

// readWorldJSONFile reads and parses one world config file. The
// returned value is always a JSON object. A missing file surfaces as
// an error wrapping fs.ErrNotExist so LoadWorld can tell "no overlay"
// from a real failure.
func readWorldJSONFile(path, label string) (map[string]any, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	// JSON.parse rejects trailing content; a single Decode does not.
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("parsing %s: unexpected trailing JSON value", path)
		}
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	obj, err := asJSONObject(value, label)
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// mergeWorldConfig is the layering rule of readWorldConfig, for
// callers that already hold both layers (WorldDefinition.ts).
func mergeWorldConfig(shipped, local map[string]any) (map[string]any, error) {
	var err error
	shipped, err = normalizeWorldPluginConfig(shipped)
	if err != nil {
		return nil, err
	}
	local, err = normalizeWorldPluginConfig(local)
	if err != nil {
		return nil, err
	}
	merged := make(map[string]any, len(shipped)+len(local))
	for k, v := range shipped {
		merged[k] = v
	}
	for k, v := range local {
		merged[k] = v
	}
	for _, key := range []string{"pluginConfig", "plugins"} {
		shippedValue, shippedOK := shipped[key].(map[string]any)
		localValue, localOK := local[key].(map[string]any)
		if shippedOK && localOK {
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
	// preserveLegacyCrossFloorDefault is intentionally not applied:
	// it only appends "CrossFloor" to disabledPlugins, which World
	// does not carry (see the World doc comment).
	return merged, nil
}

// asJSONObject requires a JSON object, mirroring object() in
// WorldDefinition.ts.
func asJSONObject(value any, label string) (map[string]any, error) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%s must be an object", label)
	}
	return obj, nil
}

// jsonInteger converts a decoded JSON number to an int, requiring an
// integral value, mirroring the Number.isInteger check in integer()
// (WorldDefinition.ts). JSON numbers decode as json.Number because
// readWorldJSONFile uses UseNumber.
func jsonInteger(value any) (int, bool) {
	num, ok := value.(json.Number)
	if !ok {
		return 0, false
	}
	f, err := num.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || math.Trunc(f) != f {
		return 0, false
	}
	if f < math.MinInt || f > math.MaxInt {
		return 0, false
	}
	return int(f), true
}

func integerField(source map[string]any, key, label string) (int, error) {
	value, ok := source[key]
	if !ok {
		return 0, fmt.Errorf("%s.%s must be an integer", label, key)
	}
	n, ok := jsonInteger(value)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be an integer", label, key)
	}
	return n, nil
}

func positiveNumberField(source map[string]any, key, label string) (float64, error) {
	num, ok := source[key].(json.Number)
	if !ok {
		return 0, fmt.Errorf("%s.%s must be a positive number", label, key)
	}
	f, err := num.Float64()
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) || f <= 0 {
		return 0, fmt.Errorf("%s.%s must be a positive number", label, key)
	}
	return f, nil
}

func coordinateField(source map[string]any, key, label string) (int, error) {
	value, err := integerField(source, key, label)
	if err != nil {
		return 0, err
	}
	if value < 0 || value > 0x3fff {
		return 0, fmt.Errorf("%s.%s is outside the world", label, key)
	}
	return value, nil
}

func planeField(source map[string]any, label string) (int, error) {
	value, err := integerField(source, "z", label)
	if err != nil {
		return 0, err
	}
	if value < 0 || value > 3 {
		return 0, fmt.Errorf("%s.z must be between 0 and 3", label)
	}
	return value, nil
}

// parseWorldPosition mirrors parseWorldPosition in WorldDefinition.ts.
func parseWorldPosition(value any, label string) (Location, error) {
	var loc Location
	position, err := asJSONObject(value, label)
	if err != nil {
		return loc, err
	}
	x, err := coordinateField(position, "x", label)
	if err != nil {
		return loc, err
	}
	y, err := coordinateField(position, "y", label)
	if err != nil {
		return loc, err
	}
	z, err := planeField(position, label)
	if err != nil {
		return loc, err
	}
	return Location{X: x, Y: y, Z: z}, nil
}

var zoneBoundKeys = []string{"minX", "maxX", "minY", "maxY", "z"}

// parseWorldZone mirrors parseWorldZone in WorldDefinition.ts. A zone
// is bounded when any bound key is present (even with a null value,
// which then fails integer validation, as in TypeScript where null
// !== undefined).
func parseWorldZone(value any, label string) (Zone, error) {
	var zone Zone
	raw, err := asJSONObject(value, label)
	if err != nil {
		return zone, err
	}
	bounded := false
	for _, key := range zoneBoundKeys {
		if _, ok := raw[key]; ok {
			bounded = true
			break
		}
	}
	zone.Bounded = bounded
	if bounded {
		minX, err := coordinateField(raw, "minX", label)
		if err != nil {
			return zone, err
		}
		maxX, err := coordinateField(raw, "maxX", label)
		if err != nil {
			return zone, err
		}
		minY, err := coordinateField(raw, "minY", label)
		if err != nil {
			return zone, err
		}
		maxY, err := coordinateField(raw, "maxY", label)
		if err != nil {
			return zone, err
		}
		z, err := planeField(raw, label)
		if err != nil {
			return zone, err
		}
		zone.MinX, zone.MaxX, zone.MinY, zone.MaxY, zone.Z = minX, maxX, minY, maxY, z
		if minX > maxX || minY > maxY {
			return zone, fmt.Errorf("%s has reversed bounds", label)
		}
	}
	tagsValue, ok := raw["tags"]
	if !ok {
		return zone, fmt.Errorf("%s.tags must be a non-empty array", label)
	}
	tags, ok := tagsValue.([]any)
	if !ok {
		return zone, fmt.Errorf("%s.tags must be a non-empty array", label)
	}
	if bounded && len(tags) == 0 {
		return zone, fmt.Errorf("%s.tags must be a non-empty array", label)
	}
	seen := make(map[string]bool, len(tags))
	for _, tag := range tags {
		s, ok := tag.(string)
		if !ok {
			return zone, fmt.Errorf("%s.tags must contain strings", label)
		}
		if seen[s] {
			continue
		}
		seen[s] = true
		zone.Tags = append(zone.Tags, s)
	}
	if zone.Tags == nil {
		zone.Tags = []string{}
	}
	return zone, nil
}

// ZoneContains reports whether the zone covers (x, y) on any plane; a
// zone without bounds covers everything. It mirrors zoneContains in
// WorldDefinition.ts: matching is x/y only, the zone's Z is ignored.
func (w *World) ZoneContains(z Zone, x, y int) bool {
	if !z.Bounded {
		return true
	}
	return x >= z.MinX && x <= z.MaxX && y >= z.MinY && y <= z.MaxY
}

// jsTrimSpace is the set of characters JavaScript's String trim
// removes (WhiteSpace + LineTerminator), which strings.TrimSpace does
// not match exactly (notably U+FEFF).
const jsTrimSpace = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

func jsTrim(s string) string {
	return strings.Trim(s, jsTrimSpace)
}

// utf16Length is the length TypeScript's string .length reports:
// UTF-16 code units, not runes or bytes.
func utf16Length(s string) int {
	return len(utf16.Encode([]rune(s)))
}

var nameForbidden = regexp.MustCompile(`[\x00-\x1f\x7f<>]`)

// parseWorldDefinition mirrors parseWorldDefinition in
// WorldDefinition.ts, including the order checks run in (which decides
// which error surfaces first when several fields are bad).
func parseWorldDefinition(world map[string]any) (*World, error) {
	zonesValue, ok := world["zones"]
	if !ok {
		return nil, fmt.Errorf("world.json zones must be an array")
	}
	rawZones, ok := zonesValue.([]any)
	if !ok {
		return nil, fmt.Errorf("world.json zones must be an array")
	}

	experienceMultiplier := 1.0
	if _, ok := world["experienceMultiplier"]; ok {
		mult, err := positiveNumberField(world, "experienceMultiplier", "world.json")
		if err != nil {
			return nil, err
		}
		experienceMultiplier = mult
	}

	gameframe := ""
	if value, ok := world["gameframe"]; ok {
		s, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("world.json gameframe must be one of %s", strings.Join(worldGameframes, ", "))
		}
		allowed := false
		for _, frame := range worldGameframes {
			if s == frame {
				allowed = true
				break
			}
		}
		if !allowed {
			return nil, fmt.Errorf("world.json gameframe must be one of %s", strings.Join(worldGameframes, ", "))
		}
		gameframe = s
	}

	nameValue, ok := world["name"]
	if !ok {
		nameValue = "Old School RuneScape"
	}
	name, ok := nameValue.(string)
	if !ok || jsTrim(name) == "" || utf16Length(jsTrim(name)) > 64 || nameForbidden.MatchString(name) {
		return nil, fmt.Errorf("world.json name must be plain text between 1 and 64 characters")
	}

	membersWorld := !isFreeToPlayWorld(world)

	descriptionValue, ok := world["description"]
	if !ok || descriptionValue == nil {
		descriptionValue = ""
	}
	description, ok := descriptionValue.(string)
	if !ok || utf16Length(description) > 120 {
		return nil, fmt.Errorf("world.json description must be a string of at most 120 characters")
	}

	zones := make([]Zone, 0, len(rawZones))
	for i, rawZone := range rawZones {
		zone, err := parseWorldZone(rawZone, fmt.Sprintf("world.json zones[%d]", i))
		if err != nil {
			return nil, err
		}
		zones = append(zones, zone)
	}

	spawn, err := parseWorldPosition(world["spawn"], "world.json spawn")
	if err != nil {
		return nil, err
	}

	result := &World{
		Name:                 jsTrim(name),
		Description:          jsTrim(description),
		Gameframe:            gameframe,
		Spawn:                spawn,
		ExperienceMultiplier: experienceMultiplier,
		Zones:                zones,
		MembersWorld:         membersWorld,
	}
	// Players outside free land are sent to the spawn, so on a
	// free-to-play world it must be inside an "f2p" zone.
	if !membersWorld {
		inside := false
		for _, zone := range zones {
			if hasTag(zone, "f2p") && result.ZoneContains(zone, spawn.X, spawn.Y) {
				inside = true
				break
			}
		}
		if !inside {
			return nil, fmt.Errorf("FreeToPlay needs the spawn inside a zone tagged \"f2p\"")
		}
	}

	// Malformed config is ignored here as in
	// PluginManager.loadPluginConfig, not fatal; plugin switches are
	// validated where they are read, by PluginManager.
	if pluginConfig, ok := world["pluginConfig"].(map[string]any); ok {
		result.PluginConfig = convertJSONNumbers(pluginConfig).(map[string]any)
	}
	if plugins, ok := world["plugins"].(map[string]any); ok {
		result.Plugins = make(map[string]bool, len(plugins))
		for name, entry := range plugins {
			if enabled, ok := entry.(bool); ok {
				result.Plugins[name] = enabled
			}
		}
	}

	result.buildFreeLandIndex()
	return result, nil
}

func hasTag(zone Zone, tag string) bool {
	for _, t := range zone.Tags {
		if t == tag {
			return true
		}
	}
	return false
}

// convertJSONNumbers deep-converts decoded JSON values so numbers
// become float64 (the TypeScript representation) instead of
// json.Number. Objects and arrays are copied; other values pass
// through unchanged.
func convertJSONNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		f, _ := v.Float64()
		return f
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, elem := range v {
			out[k] = convertJSONNumbers(elem)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, elem := range v {
			out[i] = convertJSONNumbers(elem)
		}
		return out
	default:
		return value
	}
}

// IsMembersArea reports whether (x, y) is members-only land: true on
// a free-to-play world for tiles outside every "f2p" zone, false on a
// members world or when an unbounded "f2p" zone covers everything.
// Zones match on x/y only, so one box covers every plane. It mirrors
// isMembersArea in WorldDefinition.ts, using the per-64x64-square
// index built by LoadWorld (or a direct scan for Worlds built
// literally). Zones must not be mutated after LoadWorld: the index
// is built once at load time, as in TypeScript where the definition
// is a module-level singleton.
func (w *World) IsMembersArea(x, y int) bool {
	if w == nil || w.MembersWorld {
		return false
	}
	if !w.freeLandIndexed {
		return w.isMembersAreaScan(x, y)
	}
	if w.freeEverywhere {
		return false
	}
	entry, ok := w.freeLandBySquare[((x>>6)<<8)|(y>>6)]
	if !ok {
		return true
	}
	if entry.coversAll {
		return false
	}
	for _, zone := range entry.zones {
		if w.ZoneContains(zone, x, y) {
			return false
		}
	}
	return true
}

func (w *World) isMembersAreaScan(x, y int) bool {
	for _, zone := range w.Zones {
		if hasTag(zone, "f2p") && w.ZoneContains(zone, x, y) {
			return false
		}
	}
	return true
}

// buildFreeLandIndex indexes "f2p" zones per 64x64 map square, as
// indexFreeLand does in WorldDefinition.ts: a square is marked
// covered when one zone spans it whole, otherwise it holds the zones
// overlapping it.
func (w *World) buildFreeLandIndex() {
	w.freeLandBySquare = make(map[int]freeLandEntry)
	w.freeEverywhere = false
	for _, zone := range w.Zones {
		if !hasTag(zone, "f2p") {
			continue
		}
		if !zone.Bounded {
			w.freeEverywhere = true
			continue
		}
		for squareX := zone.MinX >> 6; squareX <= zone.MaxX>>6; squareX++ {
			for squareY := zone.MinY >> 6; squareY <= zone.MaxY>>6; squareY++ {
				key := (squareX << 8) | squareY
				entry := w.freeLandBySquare[key]
				if entry.coversAll {
					continue
				}
				coversSquare := zone.MinX <= squareX<<6 && zone.MaxX >= (squareX<<6)+63 &&
					zone.MinY <= squareY<<6 && zone.MaxY >= (squareY<<6)+63
				if coversSquare {
					w.freeLandBySquare[key] = freeLandEntry{coversAll: true}
				} else {
					entry.zones = append(entry.zones, zone)
					w.freeLandBySquare[key] = entry
				}
			}
		}
	}
	w.freeLandIndexed = true
}

// --- Plugin config normalization (WorldPluginConfig.ts) ---

// normalizePluginName lowercases a plugin name and strips everything
// but a-z0-9, mirroring normalizePluginName in WorldPluginConfig.ts.
func normalizePluginName(name string) string {
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// isFreeToPlayWorld reports whether the FreeToPlay plugin switch is
// enabled, mirroring isFreeToPlayWorld in WorldPluginConfig.ts. A
// plugin entry given as an object contributes its "enabled" value; a
// scalar entry contributes itself; only boolean values count.
//
// Note: when several plugin names normalize to "freetoplay" (e.g.
// "FreeToPlay" and "free_to_play"), TypeScript lets the last one in
// file order win. Go maps do not preserve file order, so that
// pathological case resolves by map iteration order here. Real
// configs have at most one such entry.
func isFreeToPlayWorld(world map[string]any) bool {
	plugins, ok := world["plugins"].(map[string]any)
	if !ok {
		return false
	}
	enabled := false
	for name, entry := range plugins {
		if normalizePluginName(name) != "freetoplay" {
			continue
		}
		value := entry
		if obj, ok := entry.(map[string]any); ok {
			value = obj["enabled"]
		}
		if b, ok := value.(bool); ok {
			enabled = b
		}
	}
	return enabled
}

// legacyNamespaces maps lowercased plugin names to their existing
// setting prefixes. LEGACY_NAMESPACES in WorldPluginConfig.ts.
var legacyNamespaces = map[string]string{
	"characters":   "characters",
	"leaguemode":   "league",
	"quests":       "quests",
	"randomevents": "random-events",
}

// pluginConfigKey builds the flat "Plugin:setting" key for a plugin
// setting, mirroring pluginConfigKey in WorldPluginConfig.ts.
func pluginConfigKey(plugin, setting string) string {
	if ns, ok := legacyNamespaces[strings.ToLower(plugin)]; ok {
		return ns + ":" + setting
	}
	return plugin + ":" + setting
}

var pluginSettingKeyPattern = regexp.MustCompile(`^[^:\s]+$`)
var pluginNameBadChars = regexp.MustCompile(`[:\s]`)

// normalizeWorldPluginConfig splits object-form plugin entries into
// on/off switches plus flat pluginConfig settings, mirroring
// normalizeWorldPluginConfig in WorldPluginConfig.ts. It returns a
// new map; the input is not modified.
func normalizeWorldPluginConfig(world map[string]any) (map[string]any, error) {
	plugins, isObject := world["plugins"].(map[string]any)
	hasObjectEntry := false
	if isObject {
		for _, entry := range plugins {
			if _, ok := entry.(map[string]any); ok {
				hasObjectEntry = true
				break
			}
		}
	}
	if !isObject || !hasObjectEntry {
		return migrateCrossFloorVisibility(world), nil
	}

	switches := make(map[string]any, len(plugins))
	settings := make(map[string]any)
	for plugin, entry := range plugins {
		obj, ok := entry.(map[string]any)
		if !ok {
			switches[plugin] = entry
			continue
		}
		enabledValue, hasEnabled := obj["enabled"]
		if hasEnabled {
			enabled, ok := enabledValue.(bool)
			if !ok {
				return nil, fmt.Errorf("plugins.%s.enabled must be true or false", plugin)
			}
			switches[plugin] = enabled
		}
		for key, value := range obj {
			if key == "enabled" {
				continue
			}
			if !pluginSettingKeyPattern.MatchString(key) || pluginNameBadChars.MatchString(plugin) {
				return nil, fmt.Errorf("Invalid plugin setting plugins.%s.%s", plugin, key)
			}
			settings[pluginConfigKey(plugin, key)] = value
		}
	}
	result := make(map[string]any, len(world)+1)
	for k, v := range world {
		result[k] = v
	}
	result["plugins"] = switches
	pluginConfig := make(map[string]any)
	if existing, ok := world["pluginConfig"].(map[string]any); ok {
		for k, v := range existing {
			pluginConfig[k] = v
		}
	}
	for k, v := range settings {
		pluginConfig[k] = v
	}
	result["pluginConfig"] = pluginConfig
	return migrateCrossFloorVisibility(result), nil
}

func isBool(v any) bool {
	_, ok := v.(bool)
	return ok
}

func isCrossFloorName(name string) bool {
	return normalizePluginName(name) == "crossfloor"
}

// migrateCrossFloorVisibility converts the legacy CrossFloor
// visibility setting into the CrossFloor plugin switch, mirroring
// migrateCrossFloorVisibility in WorldPluginConfig.ts. It returns a
// new map when it changes anything, otherwise the input unchanged.
func migrateCrossFloorVisibility(world map[string]any) map[string]any {
	if pluginsValue, ok := world["plugins"]; ok {
		if _, isObject := pluginsValue.(map[string]any); !isObject {
			return world
		}
	}
	plugins, _ := world["plugins"].(map[string]any)
	type pluginEntry struct {
		name  string
		value any
	}
	var switches []pluginEntry
	for name, entry := range plugins {
		if isCrossFloorName(name) {
			switches = append(switches, pluginEntry{name, entry})
		}
	}
	config, _ := world["pluginConfig"].(map[string]any)
	visibility, hasVisibility := config["CrossFloor:visibility"]
	if !hasVisibility && (len(switches) == 0 || (len(switches) == 1 && switches[0].name == "CrossFloor")) {
		return world
	}
	var enabled any
	if len(switches) > 0 {
		// TypeScript takes the last switch in file order; with
		// several CrossFloor-spelled entries the choice here
		// follows map order (see isFreeToPlayWorld's note).
		enabled = switches[len(switches)-1].value
	} else {
		enabled = visibility
	}
	result := make(map[string]any, len(world))
	for k, v := range world {
		result[k] = v
	}
	if hasVisibility {
		options := make(map[string]any, len(config))
		for k, v := range config {
			if k != "CrossFloor:visibility" {
				options[k] = v
			}
		}
		result["pluginConfig"] = options
	}
	if len(switches) > 0 || isBool(enabled) {
		kept := make(map[string]any, len(plugins)+1)
		for name, entry := range plugins {
			if !isCrossFloorName(name) {
				kept[name] = entry
			}
		}
		kept["CrossFloor"] = enabled
		result["plugins"] = kept
	}
	return result
}
