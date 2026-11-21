package plugins

import (
	"encoding/json"
	"fmt"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// ConfigEnumValue is one option of an enum declaration: the value a
// config file may hold plus the label the world config page shows.
// Value must be a string or a number (see ValidateDeclaration);
// Label may be empty, matching the TS check, which only requires it
// to be a string.
type ConfigEnumValue struct {
	Value any
	Label string
}

// ConfigDeclaration mirrors ConfigOptionDeclaration
// (WorldSettingDeclarations.ts:5-21): one world.json pluginConfig
// option a plugin declares, keyed "namespace:key" in Plugin.Config.
//
// Presence tracking: Go has no undefined, so every optional field
// carries its own presence signal. Default is paired with HasDefault
// (a JSON null default is a real default for type "json", so Default
// == nil alone cannot mean "unset"); DefaultText is paired with
// HasDefaultText (an empty defaultText is a validation error, not an
// absence); Values is nil when unset (a non-nil empty slice counts as
// set, and fails enum validation); Min, Max, Integer, and
// ExclusiveMin are pointers so "unset" is distinguishable from a zero
// value — the number-only-field rules fire on presence, even for
// Integer pointing at false, exactly as the TS undefined checks do.
// Members is a plain bool because its only validation rule is
// "must be true or false", which a Go bool satisfies by construction.
//
// The TS "unknown field" rule has no Go analogue: a struct literal
// cannot carry an undeclared field, so ValidateDeclaration cannot see
// one. That check degenerates away rather than being emulated.
type ConfigDeclaration struct {
	Type    string
	Label   string
	Help    string
	Default any
	// HasDefault reports whether Default was set. Exactly one of
	// HasDefault and HasDefaultText must be true.
	HasDefault bool
	// DefaultText describes the default in words, when it is not a
	// plain value.
	DefaultText string
	// HasDefaultText reports whether DefaultText was set.
	HasDefaultText bool
	// Values is enum-only.
	Values []ConfigEnumValue
	// Min, Max, Integer, ExclusiveMin are number-only.
	Min          *float64
	Max          *float64
	Integer      *bool
	ExclusiveMin *bool
	// Members hides the option on free-to-play worlds.
	Members bool
}

// DeclaredConfig is one recorded declaration: its owning plugin and
// the declaration itself, mirroring the { owner, decl } entries of
// getConfigDeclarations (PluginManager.ts:2318-2333).
type DeclaredConfig struct {
	Owner string
	Decl  ConfigDeclaration
}

// coreConfigDeclarations mirrors CORE_CONFIG_DECLARATIONS
// (WorldSettingDeclarations.ts:183-190). The Go core declaration set
// is EMPTY: the Go core reads no pluginConfig options today, so no key
// is core-owned and the core-wins branch of checkConfigDeclarations
// never fires. If the coordinator later gives the core a declared
// option (TS core owns "commands:permissions"), it is added here and
// both the clash check and ConfigDeclarations pick it up; until then,
// documenting the empty set — rather than porting the TS entry for a
// commands system that does not exist in Go — is the honest state.
var coreConfigDeclarations = map[string]ConfigDeclaration{}

var configKeyPattern = regexp.MustCompile(`^[^:\s]+:[^:\s]+$`)

var configOptionTypes = []string{"boolean", "number", "string", "enum", "date", "json"}

// ValidateDeclaration ports validateDeclaration
// (WorldSettingDeclarations.ts:69-128): the problems with one option
// declaration, each prefixed with the key. A nil return means the
// declaration is valid. Problem order follows TS: key shape, type,
// label, help, exactly-one-of-default, defaultText shape, enum and
// number field rules, range rules, and finally the default-value
// check — which runs only when nothing else is wrong, as in TS.
func ValidateDeclaration(key string, decl ConfigDeclaration) []string {
	var problems []string
	problem := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(key+": "+format, args...))
	}
	if !configKeyPattern.MatchString(key) {
		problem("key must be namespace:key")
	}
	if !slices.Contains(configOptionTypes, decl.Type) {
		problem("type must be one of %s", strings.Join(configOptionTypes, ", "))
	}
	if strings.TrimSpace(decl.Label) == "" {
		problem("label is required")
	}
	if strings.TrimSpace(decl.Help) == "" {
		problem("help is required")
	}
	if decl.HasDefault == decl.HasDefaultText {
		problem("set exactly one of default or defaultText")
	}
	if decl.HasDefaultText && strings.TrimSpace(decl.DefaultText) == "" {
		problem("defaultText must be a non-empty string")
	}
	if decl.Type == "enum" {
		if !validEnumValues(decl.Values) {
			problem("enum needs a non-empty values list of { value, label }")
		} else if !uniqueEnumValues(decl.Values) {
			problem("enum values must be unique")
		}
	} else if decl.Values != nil {
		problem("values is only for enum")
	}
	// The TS loops run in field order min, max, integer,
	// exclusiveMin; the boolean-type loop (integer, exclusiveMin,
	// members) degenerates in Go because the fields are typed.
	if decl.Type != "number" {
		if decl.Min != nil {
			problem("min is only for number")
		}
		if decl.Max != nil {
			problem("max is only for number")
		}
		if decl.Integer != nil {
			problem("integer is only for number")
		}
		if decl.ExclusiveMin != nil {
			problem("exclusiveMin is only for number")
		}
	}
	if decl.Min != nil && !isFiniteNumber(*decl.Min) {
		problem("min must be a finite number")
	}
	if decl.Max != nil && !isFiniteNumber(*decl.Max) {
		problem("max must be a finite number")
	}
	if decl.ExclusiveMin != nil && decl.Min == nil {
		problem("exclusiveMin needs min")
	}
	if decl.Min != nil && decl.Max != nil && isFiniteNumber(*decl.Min) && isFiniteNumber(*decl.Max) && *decl.Min > *decl.Max {
		problem("min must not be greater than max")
	}
	if decl.HasDefault && len(problems) == 0 {
		if typeProblem := defaultTypeProblem(decl.Type, decl.Default); typeProblem != "" {
			problem("default is invalid: %s", typeProblem)
		} else if decl.Type == "enum" {
			if !enumContains(decl.Values, decl.Default) {
				quoted := make([]string, len(decl.Values))
				for i, v := range decl.Values {
					quoted[i] = jsonStringify(v.Value)
				}
				problem("default is invalid: must be exactly one of %s", strings.Join(quoted, ", "))
			}
		} else if decl.Type != "json" {
			if check := checkConfigValue(decl, decl.Default); !check.ok {
				problem("default is invalid: %s", check.err)
			}
		}
	}
	return problems
}

// defaultTypeProblem ports defaultTypeProblem
// (WorldSettingDeclarations.ts:32-46): a default must already have
// its declared type — no "5" for a number, no "true" for a boolean.
// It returns "" when the value has the right type.
func defaultTypeProblem(typ string, value any) string {
	switch typ {
	case "boolean":
		if _, ok := value.(bool); ok {
			return ""
		}
		return "must be true or false"
	case "number":
		if _, ok := numberValue(value); ok {
			return ""
		}
		return "must be a number, not text"
	case "string", "date":
		if _, ok := value.(string); ok {
			return ""
		}
		return "must be text"
	case "enum":
		if _, ok := value.(string); ok {
			return ""
		}
		if _, ok := numberValue(value); ok {
			return ""
		}
		return "must be one of the values"
	case "json":
		if isJSONValue(value) {
			return ""
		}
		return "must be a JSON value"
	}
	return ""
}

// configCheck is the Go shape of ConfigCheck
// (WorldSettingDeclarations.ts:23): a validated (and, for numbers,
// coerced) value, or an error message.
type configCheck struct {
	ok    bool
	value any
	err   string
}

// checkConfigValue ports checkConfigValue
// (WorldSettingDeclarations.ts:130-172): it validates (and for
// numbers, coerces) a value against its declaration. Booleans are not
// coerced. The hub uses it for declaration defaults and for warning
// about world.json values; the coerced value is discarded in both
// uses — TS warns "the value is used as written", and GetPluginConfig
// returns values as written.
func checkConfigValue(decl ConfigDeclaration, value any) configCheck {
	switch decl.Type {
	case "boolean":
		if b, ok := value.(bool); ok {
			return configCheck{ok: true, value: b}
		}
		return configCheck{err: "must be true or false"}
	case "number":
		n, ok := coerceConfigNumber(value)
		if !ok || !isFiniteNumber(n) {
			return configCheck{err: "must be a number"}
		}
		if decl.Integer != nil && *decl.Integer && n != math.Trunc(n) {
			return configCheck{err: "must be a whole number"}
		}
		if decl.Min != nil {
			if decl.ExclusiveMin != nil && *decl.ExclusiveMin {
				if n <= *decl.Min {
					return configCheck{err: fmt.Sprintf("must be greater than %s", jsNumberString(*decl.Min))}
				}
			} else if n < *decl.Min {
				return configCheck{err: fmt.Sprintf("must be at least %s", jsNumberString(*decl.Min))}
			}
		}
		if decl.Max != nil && n > *decl.Max {
			return configCheck{err: fmt.Sprintf("must be at most %s", jsNumberString(*decl.Max))}
		}
		return configCheck{ok: true, value: n}
	case "string":
		if s, ok := value.(string); ok {
			return configCheck{ok: true, value: s}
		}
		return configCheck{err: "must be text"}
	case "enum":
		// Numeric options are read with Number(), so "5" counts as
		// 5 — but only against numeric option values, and the
		// returned value is the declared option value, not the
		// input.
		asNumber := value
		if s, ok := value.(string); ok && strings.TrimSpace(s) != "" {
			if n, ok := coerceConfigNumber(s); ok {
				asNumber = n
			} else {
				asNumber = math.NaN()
			}
		}
		for _, v := range decl.Values {
			if enumValueEquals(v.Value, value) {
				return configCheck{ok: true, value: v.Value}
			}
			if f, ok := numberValue(v.Value); ok {
				if g, ok := numberValue(asNumber); ok && f == g {
					return configCheck{ok: true, value: v.Value}
				}
			}
		}
		listed := make([]string, len(decl.Values))
		for i, v := range decl.Values {
			listed[i] = jsString(v.Value)
		}
		return configCheck{err: fmt.Sprintf("must be one of %s", strings.Join(listed, ", "))}
	case "date":
		s, ok := value.(string)
		if !ok || !datePattern.MatchString(s) {
			return configCheck{err: "must be a date as YYYY-MM-DD"}
		}
		if t, err := time.Parse("2006-01-02", s); err != nil || t.Format("2006-01-02") != s {
			return configCheck{err: "must be a real calendar date"}
		}
		return configCheck{ok: true, value: s}
	case "json":
		// TS accepts anything but undefined; Go has no undefined,
		// so every value — including nil, the JSON null — passes.
		return configCheck{ok: true, value: value}
	default:
		return configCheck{err: "unknown option type"}
	}
}

var datePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// numberValue converts the Go spellings of a JSON number to float64.
// JavaScript has one number type; Go content and decoded JSON spell
// it several ways (float64 from the config package's JSON conversion,
// ints from struct literals), and all of them are numbers here.
// Strings are never numbers: coercion is coerceConfigNumber's job.
func numberValue(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int8:
		return float64(v), true
	case int16:
		return float64(v), true
	case int32:
		return float64(v), true
	case int64:
		return float64(v), true
	case uint:
		return float64(v), true
	case uint8:
		return float64(v), true
	case uint16:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		return 0, false
	}
}

func isFiniteNumber(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// coerceConfigNumber ports the number arm's coercion: a number passes
// through, a non-blank string goes through Number(), and anything
// else is not a number. JavaScript's Number() trims surrounding
// whitespace, accepts exponents and Infinity, and rejects underscores
// and hex/binary/octal spellings are accepted ("0x10" is 16) — Go's
// ParseFloat covers the common spellings but not the 0x/0b/0o string
// forms, a documented approximation: config values arrive from JSON
// as numbers already, so string coercion only ever sees hand-written
// spellings like "5". Underscores are rejected explicitly because
// ParseFloat accepts Go-style "1_000" where Number() yields NaN.
func coerceConfigNumber(value any) (float64, bool) {
	if f, ok := numberValue(value); ok {
		return f, true
	}
	s, ok := value.(string)
	if !ok {
		return 0, false
	}
	trimmed := strings.TrimSpace(s)
	if trimmed == "" || strings.Contains(trimmed, "_") {
		return 0, false
	}
	f, err := strconv.ParseFloat(trimmed, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

// isJSONValue ports isJsonValue (WorldSettingDeclarations.ts:48-58):
// null, strings, booleans, finite numbers, arrays, and plain objects,
// recursively. Only map[string]any and []any count as objects and
// arrays: a Go struct is not a plain JSON object in this check, even
// though encoding/json would marshal one.
func isJSONValue(value any) bool {
	switch v := value.(type) {
	case nil, string, bool:
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
		if f, ok := numberValue(value); ok {
			return isFiniteNumber(f)
		}
		return false
	}
}

// validEnumValues checks the enum values shape: a non-empty list
// whose every entry holds a string or number value. The label check
// degenerates in Go (Label is typed string, and TS accepts an empty
// label).
func validEnumValues(values []ConfigEnumValue) bool {
	if len(values) == 0 {
		return false
	}
	for _, v := range values {
		if _, ok := v.Value.(string); ok {
			continue
		}
		if _, ok := numberValue(v.Value); !ok {
			return false
		}
	}
	return true
}

// uniqueEnumValues checks enum value uniqueness with Set semantics
// (WorldSettingDeclarations.ts:102): SameValueZero, under which NaN
// equals NaN and -0 equals +0. enumValueKey normalizes both cases;
// it must not be reused for default matching, which uses ===.
func uniqueEnumValues(values []ConfigEnumValue) bool {
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		key := enumValueKey(v.Value)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func enumValueKey(value any) string {
	if s, ok := value.(string); ok {
		return "s:" + s
	}
	f, _ := numberValue(value)
	if f == 0 {
		f = 0 // normalize -0 to +0, as SameValueZero does
	}
	return "n:" + strconv.FormatFloat(f, 'g', -1, 64)
}

// enumValueEquals is === for enum scalars: strings equal strings,
// numbers equal numbers by value (NaN never equals, -0 equals +0),
// and a string never equals a number.
func enumValueEquals(a, b any) bool {
	if s, ok := a.(string); ok {
		t, ok := b.(string)
		return ok && s == t
	}
	f, ok := numberValue(a)
	if !ok {
		return false
	}
	g, ok := numberValue(b)
	return ok && f == g
}

func enumContains(values []ConfigEnumValue, value any) bool {
	for _, v := range values {
		if enumValueEquals(v.Value, value) {
			return true
		}
	}
	return false
}

// jsNumberString renders a float64 the way JavaScript's String()
// does in TS template literals ("5", "0.5", "1e+21"): encoding/json's
// float rendering matches for every finite value, and the non-finite
// spellings are handled explicitly.
func jsNumberString(f float64) string {
	if math.IsNaN(f) {
		return "NaN"
	}
	if math.IsInf(f, 1) {
		return "Infinity"
	}
	if math.IsInf(f, -1) {
		return "-Infinity"
	}
	b, err := json.Marshal(f)
	if err != nil {
		return strconv.FormatFloat(f, 'g', -1, 64)
	}
	return string(b)
}

// jsString renders a value the way JavaScript's String() does for the
// enum error list: strings raw, numbers as jsNumberString.
func jsString(value any) string {
	if s, ok := value.(string); ok {
		return s
	}
	if f, ok := numberValue(value); ok {
		return jsNumberString(f)
	}
	return fmt.Sprintf("%v", value)
}

// jsonStringify renders a scalar the way JSON.stringify does for the
// enum-default error message: strings quoted, numbers bare, NaN and
// infinities as null.
func jsonStringify(value any) string {
	if f, ok := numberValue(value); ok && !isFiniteNumber(f) {
		return "null"
	}
	b, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprintf("%v", value)
	}
	return string(b)
}

// legacyNamespaces maps lowercased plugin names to their existing
// setting prefixes: LEGACY_NAMESPACES in WorldPluginConfig.ts:26-31.
// The exported names of these plugins differ from the prefixes their
// settings already use, so bare-key prefixing must map them.
var legacyNamespaces = map[string]string{
	"characters":   "characters",
	"leaguemode":   "league",
	"quests":       "quests",
	"randomevents": "random-events",
}

// pluginConfigKey builds the flat "Plugin:setting" key for a plugin
// setting, mirroring pluginConfigKey (WorldPluginConfig.ts:33-35).
// The lookup lowercases the plugin name WITHOUT stripping characters
// (unlike NormalizeName): "LeagueMode" maps to "league", but
// "league-mode" does not. Api.GetPluginConfig (api.go) uses this to
// prefix bare keys; Hub.GetPluginConfig does not prefix. (The config
// package carries an identical copy it applies when flattening
// object-form plugin entries; the hub reads the already-normalized
// map and must not apply the mapping a second time — see
// GetPluginConfig.)
func pluginConfigKey(plugin, setting string) string {
	if ns, ok := legacyNamespaces[strings.ToLower(plugin)]; ok {
		return ns + ":" + setting
	}
	return plugin + ":" + setting
}

// GetPluginConfig returns the value of world.json pluginConfig[key],
// or def when the key is unset, mirroring getPluginConfig
// (PluginManager.ts:2306-2315). The lookup is verbatim: no prefixing
// and no coercion — a key present with a JSON null returns nil, and a
// value that does not fit its declaration is returned as written (the
// mismatch is warned about at load time, never corrected). An empty
// key returns def. Bare-key prefixing
// (pluginConfigKey(pluginName, key), PluginManager.ts:4374-4375) lives
// in Api.GetPluginConfig, which knows the owning plugin; the hub
// level has no plugin to prefix with.
//
// The hub reads the normalized map internal/config builds
// (World.PluginConfig): object-form plugin entries were already split
// into switches plus flat pluginConfig settings with the legacy
// namespace mapping applied. The hub must not re-normalize keys.
func (h *Hub) GetPluginConfig(key string, def any) any {
	if key == "" {
		return def
	}
	if value, ok := h.pluginConfig[key]; ok {
		return value
	}
	return def
}

// ConfigDeclarations returns every option a loaded plugin (or the
// core) declares, keyed by pluginConfig key, mirroring
// getConfigDeclarations (PluginManager.ts:2318-2333). The returned
// map is a fresh copy. Before Load runs it holds only the core set,
// which is currently empty.
func (h *Hub) ConfigDeclarations() map[string]DeclaredConfig {
	out := make(map[string]DeclaredConfig, len(coreConfigDeclarations)+len(h.declarations))
	for key, decl := range coreConfigDeclarations {
		out[key] = DeclaredConfig{Owner: "core", Decl: decl}
	}
	for key, entry := range h.declarations {
		out[key] = entry
	}
	return out
}

// checkConfigDeclarations ports checkConfigDeclarations
// (PluginManager.ts:2336-2384): the valid option declarations of the
// plugins about to load, computed BEFORE any registration and only
// for plugins that will load. A malformed config, an invalid
// declaration, or a key the core declares is logged and dropped; a
// key two plugins declare is logged and kept for both, and the first
// of them that loads owns it (settled in
// recordLoadedConfigDeclarations). Accepted declarations are stored
// on each candidate; candidates with a nil Config map get none.
//
// Key iteration is sorted: TS iterates object entries in definition
// order, which a Go map cannot reproduce, so sorted order keeps logs
// deterministic. Clash ownership depends on candidate order, not key
// order, and is unaffected.
//
// Deviation, deliberate: TS tracks core ownership in the same owners
// map under the literal name "core", so a plugin actually named
// "core" would be mistaken for the core. Go keeps the core set
// separate, which implements the same rule without the collision.
func (h *Hub) checkConfigDeclarations(toLoad []*loadCandidate) {
	owners := make(map[string]string)
	for _, c := range toLoad {
		if c.plugin.Config == nil {
			continue
		}
		kept := make(map[string]ConfigDeclaration, len(c.plugin.Config))
		for _, key := range slices.Sorted(maps.Keys(c.plugin.Config)) {
			decl := c.plugin.Config[key]
			problems := ValidateDeclaration(key, decl)
			if len(problems) > 0 {
				h.logf("[plugins] %s: invalid config declaration, ignored: %s", c.name, strings.Join(problems, "; "))
				continue
			}
			if _, coreOwned := coreConfigDeclarations[key]; coreOwned {
				h.logf("[plugins] config key %s is declared by both core and %s; core owns it and %s's declaration is ignored", key, c.name, c.name)
				continue
			}
			if existing, ok := owners[key]; ok {
				// Not settled here: the first of the claimants
				// that actually loads owns the key.
				h.logf("[plugins] config key %s is declared by both %s and %s; %s is expected to own it, and ownership goes to the first of them that loads", key, existing, c.name, existing)
			} else {
				owners[key] = c.name
			}
			kept[key] = decl
		}
		c.accepted = kept
	}
}

// recordLoadedConfigDeclarations ports recordLoadedConfigDeclarations
// (PluginManager.ts:2386-2405): after registration, the accepted
// declarations of plugins that actually loaded become the recorded
// set, in load-candidate order, with the first loaded claimant owning
// each clashing key. Plugins whose Register failed contribute
// nothing, even though their declarations were accepted.
func (h *Hub) recordLoadedConfigDeclarations(toLoad []*loadCandidate) {
	h.declarations = make(map[string]DeclaredConfig)
	loaded := make(map[string]bool, len(h.loadedNames))
	for _, name := range h.loadedNames {
		loaded[name] = true
	}
	for _, c := range toLoad {
		if c.accepted == nil || !loaded[c.name] {
			continue
		}
		for _, key := range slices.Sorted(maps.Keys(c.accepted)) {
			if _, ok := h.declarations[key]; ok {
				continue // an earlier loaded claimant owns it
			}
			h.declarations[key] = DeclaredConfig{Owner: c.name, Decl: c.accepted[key]}
		}
	}
}

// warnAboutInvalidConfigValues ports the value half of
// warnAboutInvalidConfigValues (PluginManager.ts:2407-2430): every
// configured value with a recorded declaration is checked, and a
// value that does not fit is warned about — the value is still used
// as written. Keys without declarations are silently ignored, as in
// TS. Iteration is sorted for deterministic logs.
//
// The scoped-settings half of the TS function is NOT ported: it warns
// when an object-form plugins.<plugin>.<setting> entry has no
// declared key, and that needs the ScopedPluginSettings map built
// during world-config normalization. internal/config does not
// surface scoped settings (its normalizeWorldPluginConfig flattens
// entries without recording their origin), so the hub cannot tell a
// scoped setting from a flat pluginConfig key. If the coordinator
// wants those warnings, the config package must expose the scoped
// map and HubOptions must carry it; until then the gap is documented
// here rather than approximated.
func (h *Hub) warnAboutInvalidConfigValues() {
	declarations := h.ConfigDeclarations()
	for _, key := range slices.Sorted(maps.Keys(h.pluginConfig)) {
		entry, ok := declarations[key]
		if !ok {
			continue
		}
		if check := checkConfigValue(entry.Decl, h.pluginConfig[key]); !check.ok {
			h.logf("[plugins] world.json pluginConfig %q (%s) %s; the value is used as written", key, entry.Owner, check.err)
		}
	}
}
