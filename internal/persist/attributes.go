package persist

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// Persistent attribute registry: the process-global persistent-key set
// plus the attribute-default map, mirroring the PlayerSave statics
// (game/entity/impl/player/persistence/PlayerSave.ts:52-101):
// persistentAttributeKeys, attributeDefaults, persistAttribute,
// applyAttributeDefaults, migrateAttributeKey's set lookup, and
// cloneAttribute's JSON-compatibility rule.
//
// TS keeps these as mutable statics on the PlayerSave class; plugins
// extend them at load time through api.persistAttribute, and fromPlayer
// re-registers the bank/combat keys on every save. Go mirrors that with
// one RWMutex-guarded registry: registration happens at boot (plugin
// Register, content init), reads happen on the tick goroutine (entry
// application, snapshots) and the connection goroutine (entry), so
// every access goes through the mutex. The registry is process-global
// and monotonic in production: keys are never unregistered, exactly as
// the TS Set/Map are never cleared.
//
// This package imports no other tsps-go package; that layering is
// deliberate (see doc.go) and is preserved here: the registry stores
// canonical JSON strings, never live values.

// corePersistentAttributeKeys is the core seed of the persistent set,
// in TS Set insertion order (PlayerSave.ts:52-57 plus the fromPlayer
// registrations at :793-798): the 22 LEGACY_FIELD_ATTRIBUTES values in
// declaration order, then AggressionTolerance.ATTRIBUTE
// ("npc-aggression:tolerance", AggressionTolerance.ts:33),
// CREATION_MENU_LAST_ITEM_ATTRIBUTE ("creation-menu:last-item",
// PacketSender.ts:165), and LOGIN_TOKENS_ATTRIBUTE ("login:tokens",
// LoginTokens.ts:9). The fromPlayer registrations re-add the five bank
// keys (already present via the legacy values, so a Set.add is a
// no-op that does not move them) and append
// "combat:style-by-category" (FightType.ts,
// COMBAT_STYLE_BY_CATEGORY_ATTRIBUTE). The order is pinned by
// TestCoreAttributeRegistrySeed against legacyFieldAttributes, so the
// seed cannot drift from the legacy map silently.
var corePersistentAttributeKeys = []string{
	"loyalty:title",
	"skills:xp-locked",
	"bounty-hunter:target-teleport-unlocked",
	"prayer:preserve-unlocked",
	"prayer:rigour-unlocked",
	"prayer:augury-unlocked",
	"ring-of-recoil:damage",
	"crystal-bow:shots-in-stage",
	"crystal-bow:tracked-item",
	"killstreaks:total-kills",
	"killstreaks:streak",
	"killstreaks:highest-streak",
	"killstreaks:deaths",
	"killstreaks:recent-kills",
	"shop:points",
	"pest-control:points",
	"runecrafting:pouches",
	"bank:note-withdrawal",
	"bank:insert-mode",
	"bank:quantity-mode",
	"bank:custom-quantity",
	"bank:placeholders",
	"npc-aggression:tolerance",
	"creation-menu:last-item",
	"login:tokens",
	"combat:style-by-category",
}

// coreAttributeDefault is one seeded default: the key and the canonical
// JSON of its default value (the JSON.stringify of the TS default).
type coreAttributeDefault struct {
	key  string
	json string
}

// coreAttributeDefaults is the core seed of the default map, in TS Map
// insertion order: the two attributeDefaults entries (PlayerSave.ts:60-63),
// then the fromPlayer persistAttribute registrations (:793-798) in call
// order. A value equal to its default is NOT saved, and a save without
// the key loads the default (PlayerSave.ts:59 comment;
// applyAttributeDefaults). In TS the fromPlayer registrations land on
// the first save; Go seeds them at boot, as the previous static map did
// — the observable difference (defaults visible before any save) is the
// pre-existing behavior, preserved here, not a new one.
var coreAttributeDefaults = []coreAttributeDefault{
	{"ring-of-recoil:damage", "0"},
	{"login:tokens", "[]"},
	{"bank:note-withdrawal", "false"},
	{"bank:insert-mode", "false"},
	{"bank:quantity-mode", "0"},
	{"bank:custom-quantity", "0"},
	{"bank:placeholders", "false"},
	{"combat:style-by-category", "{}"},
}

// persistentAttributeRegistry holds the live persistent set and default
// map, with insertion-order slices: Go maps do not iterate in insertion
// order, and TS iterates both the Set (applyToPlayer, fromPlayer) and
// the Map (applyAttributeDefaults) in insertion order. The default
// order is observable (ApplyAttributeDefaults applies in order); the
// key order is retained for the same reason even though no current
// caller depends on it. Re-registration never moves an entry: a JS
// Map.set / Set.add on an existing key keeps its position, and this
// registry matches that.
type persistentAttributeRegistry struct {
	mu           sync.RWMutex
	keys         map[string]bool
	keyOrder     []string
	defaults     map[string]string
	defaultOrder []string
}

// newPersistentAttributeRegistry returns a registry holding only the
// core seed, with fresh maps and slices on every call.
func newPersistentAttributeRegistry() *persistentAttributeRegistry {
	r := &persistentAttributeRegistry{
		keys:     make(map[string]bool, len(corePersistentAttributeKeys)),
		defaults: make(map[string]string, len(coreAttributeDefaults)),
	}
	for _, key := range corePersistentAttributeKeys {
		r.keys[key] = true
		r.keyOrder = append(r.keyOrder, key)
	}
	for _, d := range coreAttributeDefaults {
		r.defaults[d.key] = d.json
		r.defaultOrder = append(r.defaultOrder, d.key)
	}
	return r
}

// attributeRegistry is the process-global registry. It is never
// replaced, only mutated under its mutex (ResetAttributeRegistryForTest
// swaps its contents, not the pointer), so readers holding the package
// variable always see the same mutex.
var attributeRegistry = newPersistentAttributeRegistry()

// jsWhitespace is the ECMAScript WhiteSpace + LineTerminator set that
// String.prototype.trim removes: TAB, VT, FF, SP, NBSP, U+1680,
// U+2000-U+200A, U+2028, U+2029, U+202F, U+205F, U+3000, U+FEFF, plus
// LF/CR. Go's strings.TrimSpace uses the Unicode space set instead,
// which misses U+FEFF; persistAttribute's key check must match the TS
// `key !== key.trim()` test exactly, so the cutset is spelled out.
const jsWhitespace = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

// validPersistentAttributeKey reports whether key passes the
// persistAttribute key check (PlayerSave.ts:67): a non-empty string
// with no surrounding whitespace (`!key.trim() || key !== key.trim()`
// throws in TS). Interior whitespace is allowed, as in TS.
func validPersistentAttributeKey(key string) bool {
	return key != "" && strings.Trim(key, jsWhitespace) == key
}

// PersistAttribute ports PlayerSave.persistAttribute
// (PlayerSave.ts:66-74): it adds key to the persistent set and, when
// hasDefault, validates defaultValue with the cloneAttribute rules and
// stores its canonical JSON as the key's default. Re-registration
// overwrites the default (TS Map.set overwrites) and re-adding a key is
// a no-op for the set; neither moves the key's insertion position.
//
// An invalid key (empty or whitespace-padded) returns an error and
// registers nothing. A default that fails the cloneAttribute rules
// returns an error AFTER the key is registered: TS adds the key before
// cloning the default, so a throwing default leaves the key in the set
// with no default stored, and this port matches that order. Callers
// that must not register on a bad default (the plugin hub, which panics
// on the error and drops the plugin) accept the TS behavior: the key
// stays registered, exactly as a TS throw would leave it.
//
// Safe for concurrent use: registration happens at boot, reads on the
// tick and connection goroutines.
func PersistAttribute(key string, defaultValue any, hasDefault bool) error {
	if !validPersistentAttributeKey(key) {
		return fmt.Errorf("persist: persistent attribute keys must be non-empty strings without surrounding whitespace: %q", key)
	}
	var canonical string
	var valueErr error
	if hasDefault {
		var raw json.RawMessage
		raw, valueErr = MarshalAttributeValue(key, defaultValue)
		canonical = string(raw)
	}
	attributeRegistry.mu.Lock()
	defer attributeRegistry.mu.Unlock()
	// TS adds the key before cloning the default (see the doc
	// comment): a failing default still leaves the key registered.
	if !attributeRegistry.keys[key] {
		attributeRegistry.keys[key] = true
		attributeRegistry.keyOrder = append(attributeRegistry.keyOrder, key)
	}
	if valueErr != nil {
		return valueErr
	}
	if hasDefault {
		if _, ok := attributeRegistry.defaults[key]; !ok {
			attributeRegistry.defaultOrder = append(attributeRegistry.defaultOrder, key)
		}
		attributeRegistry.defaults[key] = canonical
	}
	return nil
}

// PersistentAttributeKeys returns a sorted copy of all registered
// persistent keys: the core seed plus every runtime registration.
// Callers mutating the result never affect the registry.
func PersistentAttributeKeys() []string {
	attributeRegistry.mu.RLock()
	defer attributeRegistry.mu.RUnlock()
	keys := append([]string(nil), attributeRegistry.keyOrder...)
	sort.Strings(keys)
	return keys
}

// ResetAttributeRegistryForTest restores the registry to the initial
// core seed: the core keys and core defaults only, in seed order. It
// exists so tests that register keys cannot pollute each other through
// the process-global registry. TEST-ONLY: production code must never
// call it — the registry is monotonic in TS, and resetting it mid-run
// would silently unpersist keys plugins registered.
func ResetAttributeRegistryForTest() {
	fresh := newPersistentAttributeRegistry()
	attributeRegistry.mu.Lock()
	defer attributeRegistry.mu.Unlock()
	attributeRegistry.keys = fresh.keys
	attributeRegistry.keyOrder = fresh.keyOrder
	attributeRegistry.defaults = fresh.defaults
	attributeRegistry.defaultOrder = fresh.defaultOrder
}

// isPersistentAttributeKey reports whether key is in the live
// persistent set. It is the migrateAttributeKey lookup
// (PlayerSave.ts:84-89) against the live registry.
func isPersistentAttributeKey(key string) bool {
	attributeRegistry.mu.RLock()
	defer attributeRegistry.mu.RUnlock()
	return attributeRegistry.keys[key]
}

// attributeDefaultJSON returns the canonical JSON of key's registered
// default, if any, from the live registry. AttributeDefaultJSON
// (playersave.go) is the exported form.
func attributeDefaultJSON(key string) (string, bool) {
	attributeRegistry.mu.RLock()
	defer attributeRegistry.mu.RUnlock()
	v, ok := attributeRegistry.defaults[key]
	return v, ok
}

// MarshalAttributeValue validates value with the cloneAttribute rules
// and returns its canonical JSON (encoding/json marshal; map keys sort,
// so the encoding is canonical and StripDefaultAttributes comparisons
// against stored defaults are stable). It ports the
// JSON.parse(JSON.stringify(...)) half of cloneAttribute
// (PlayerSave.ts:92-101): the value that reaches a save is the
// re-encoded form, never an alias of live state.
//
// Allowed: nil, string, boolean, finite numbers, arrays, and plain
// objects. Anything else returns an error naming the key, as the TS
// throw names it.
func MarshalAttributeValue(key string, value any) (json.RawMessage, error) {
	if err := validateAttributeValue(value); err != nil {
		return nil, fmt.Errorf("persist: persistent attribute %q must contain only JSON-compatible values: %s", key, err)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		// Unreachable for a validated value (every accepted kind
		// marshals), but a marshaler failure must fail the save
		// rather than write a partial value.
		return nil, fmt.Errorf("persist: persistent attribute %q must contain only JSON-compatible values: %s", key, err)
	}
	return json.RawMessage(raw), nil
}

// ApplyAttributeDefaults ports applyAttributeDefaults
// (PlayerSave.ts:76-80): for each registered default IN REGISTRATION
// ORDER (core seed first in TS order, then runtime registrations), if
// !has(key), set(key, freshly-decoded default). The stored default JSON
// is decoded to `any` on every call — the JSON.parse equivalent — so
// each application gets fresh values: mutating one player's defaulted
// object or array can never affect another player's. has/set are called
// without holding the registry lock: a set callback must never be able
// to deadlock against a concurrent registration.
func ApplyAttributeDefaults(has func(key string) bool, set func(key string, value any)) {
	attributeRegistry.mu.RLock()
	defaults := make([]coreAttributeDefault, 0, len(attributeRegistry.defaultOrder))
	for _, key := range attributeRegistry.defaultOrder {
		defaults = append(defaults, coreAttributeDefault{key: key, json: attributeRegistry.defaults[key]})
	}
	attributeRegistry.mu.RUnlock()
	for _, d := range defaults {
		if has(d.key) {
			continue
		}
		var value any
		if err := json.Unmarshal([]byte(d.json), &value); err != nil {
			// Unreachable: stored defaults are json.Marshal output
			// or the core seed's literals, all valid JSON. Skip
			// rather than set a wrong value if the invariant ever
			// breaks; there is no error channel in the TS shape.
			continue
		}
		set(d.key, value)
	}
}

// validateAttributeValue enforces the cloneAttribute replacer rule
// (PlayerSave.ts:92-101) over a Go value by reflection: nil, strings,
// booleans, finite numbers, arrays, and plain objects pass; everything
// else fails. The TS test for a plain object is prototype-based
// (Object.prototype or null): class instances, Dates, Maps, and typed
// arrays throw. The Go analogues:
//
//   - Structs are rejected: a Go struct is a class instance, not a
//     plain object. map[string]any is the plain-object form, and
//     values decoded from save JSON always take that form. (This
//     rejects time.Time and friends too — they must be stored as
//     numbers or strings, as TS stores dates.)
//   - Pointers are rejected: TS has no pointer kind, and a pointer to
//     an otherwise-valid value is not a JSON value. Callers pass
//     values, not references.
//   - []byte is rejected: encoding/json marshals a byte slice as a
//     base64 string, not an array of numbers, so accepting it would
//     silently store a different JSON shape than the value's array
//     form. TS rejects Uint8Array the same way (not Array, not a
//     plain object). A [N]byte array marshals as a JSON array and is
//     accepted like any other array.
//   - Map keys must be strings (any string kind); a map[int]... has no
//     JSON object form. (encoding/json would stringify integer keys,
//     but TS object keys are always strings, and the silent rewrite
//     would break the canonical comparison.)
//   - Cyclic maps/slices are rejected: TS JSON.stringify throws on a
//     cycle, and the walk must not recurse forever first. Shared
//     (non-cyclic) references are fine — only ancestors count, so the
//     same map appearing twice as siblings passes and marshals twice,
//     as JSON.stringify does.
func validateAttributeValue(value any) error {
	return validateAttributeReflect(reflect.ValueOf(value), nil)
}

// attributeVisit identifies one map/slice identity on the walk stack,
// for cycle detection: the kind plus the pointer to its storage. A nil
// or empty slice reports pointer 0 and carries no identity — it cannot
// be part of a cycle.
type attributeVisit struct {
	kind reflect.Kind
	ptr  uintptr
}

func validateAttributeReflect(v reflect.Value, ancestors map[attributeVisit]bool) error {
	if !v.IsValid() {
		return nil // nil interface: JSON null.
	}
	switch v.Kind() {
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return validateAttributeReflect(v.Elem(), ancestors)
	case reflect.Bool, reflect.String,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return nil
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			return errors.New("non-finite number")
		}
		return nil
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Uint8 {
			return errors.New("byte slice (marshals as a base64 string, not an array)")
		}
		// Only slices can cycle (arrays hold values, not references
		// to themselves); check the slice identity against ancestors.
		if v.Kind() == reflect.Slice && v.Pointer() != 0 {
			visit := attributeVisit{kind: reflect.Slice, ptr: v.Pointer()}
			if ancestors[visit] {
				return errors.New("cyclic value")
			}
			if ancestors == nil {
				ancestors = map[attributeVisit]bool{}
			}
			ancestors[visit] = true
			defer delete(ancestors, visit)
		}
		for i := 0; i < v.Len(); i++ {
			if err := validateAttributeReflect(v.Index(i), ancestors); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if v.Type().Key().Kind() != reflect.String {
			return fmt.Errorf("map with non-string keys (%s)", v.Type().Key())
		}
		if v.Pointer() != 0 {
			visit := attributeVisit{kind: reflect.Map, ptr: v.Pointer()}
			if ancestors[visit] {
				return errors.New("cyclic value")
			}
			if ancestors == nil {
				ancestors = map[attributeVisit]bool{}
			}
			ancestors[visit] = true
			defer delete(ancestors, visit)
		}
		for _, k := range v.MapKeys() {
			if err := validateAttributeReflect(v.MapIndex(k), ancestors); err != nil {
				return err
			}
		}
		return nil
	case reflect.Struct:
		return fmt.Errorf("struct value (%s): only plain objects (map[string]any) are JSON-compatible", v.Type())
	default:
		return fmt.Errorf("unsupported value kind %s", v.Kind())
	}
}
