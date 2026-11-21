package definition

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// NpcSpawn is one loaded NPC spawn: the NpcSpawnDefinition data
// (NpcSpawnDefinition.ts + DefaultSpawnDefinition.ts) as plain
// data. It carries no methods the consumer needs.
type NpcSpawn struct {
	ID      int
	X, Y, Z int
	// Facing is a game.Direction-compatible id (Direction.valueOf
	// order: 0 north-west, 1 north, 2 north-east, 3 west, 4 east,
	// 5 south-west, 6 south, 7 south-east). The loader always
	// yields a facing — an explicit direction, the cache
	// spawnDirection, or south — so the "0 when the loader yields
	// no facing" case never arises from this loader; a Facing of
	// 0 means north-west, a real facing.
	Facing int
	// WanderRadius is as loaded: the entry's wanderRadius clamped
	// to >= 0, or 5 (DEFAULT_WANDER_RADIUS,
	// NpcSpawnDefinitionLoader.ts:36) when absent. It is never -1
	// from this loader; the definition's radius is nullable in TS
	// only for directly-constructed definitions.
	WanderRadius int
	// Name is the entry's name (String(raw.name ?? "")); it becomes
	// the spawn definition's description in TS.
	Name string
	// Source is the contributing source's name ("NpcSpawns" for
	// the core file, as in TS — see the package comment). "core"
	// is only the NpcSpawnDefinition constructor default
	// (NpcSpawnDefinition.ts:19), which the loader never produces.
	Source string
}

// ObjectSpawn is one loaded object spawn: the ObjectSpawnDefinition
// data (ObjectSpawnDefinition.ts + DefaultSpawnDefinition.ts) as
// plain data, with the loader's defaults applied (type 10, face 0).
type ObjectSpawn struct {
	ID      int
	X, Y, Z int
	Type    int
	Face    int
}

// Definition type names, mirroring the loaders' DEFINITION_TYPE
// constants.
const (
	npcSpawnsDefinitionType    = "npc_spawns"
	objectSpawnsDefinitionType = "object_spawns"
	groundItemsDefinitionType  = "ground_items"
	shopsDefinitionType        = "shops"
	npcInteractionsType        = "npc_interactions"
)

// defaultWanderRadius is DEFAULT_WANDER_RADIUS
// (NpcSpawnDefinitionLoader.ts:36).
const defaultWanderRadius = 5

// defaultClientDirection is DEFAULT_CLIENT_DIRECTION
// (NpcSpawnDefinitionLoader.ts:38): south, how live OSRS NPCs
// stand when nothing turns them.
const defaultClientDirection = 1

// clientToServerDirection and cacheToClientDirection are the
// CLIENT_TO_SERVER_DIRECTION and CACHE_TO_CLIENT_DIRECTION tables
// (NpcSpawnDefinitionLoader.ts:43,49). Both tables hold the same
// values in TS: the spawn `direction` counts from south-west as
// the client's Direction does, and the cache's spawnDirection
// counts from north-west as the game does.
var clientToServerDirection = [8]int{5, 6, 7, 3, 4, 0, 1, 2}
var cacheToClientDirection = [8]int{5, 6, 7, 3, 4, 0, 1, 2}

// loadNpcSpawns ports NpcSpawnDefinitionLoader.load()
// (NpcSpawnDefinitionLoader.ts:57-140): merge contributed sources
// in priority order with the one-for-one replace/remove pairing,
// convert each surviving record, drop invalid and cache-unsupported
// entries, and keep the result in merged order. The TS loader then
// calls NpcSpawnDefinition.replace and applyToWorld (spawning NPCs
// into the World, region-deferred); spawning into the world is the
// spawns worker's job, not this package's, so Load stops at the
// definitions. The plugin veto (emitCanSpawnNpc, line 124) has no
// plugin system behind it here and never vetoes.
func (s *Store) loadNpcSpawns() error {
	loaded, failures := s.registry.loadSources(npcSpawnsDefinitionType, s.logf)

	// Merge sources in priority order, applying records in file
	// order (NpcSpawnDefinitionLoader.ts:67-110). Several spawns
	// can share a tile, so records pair one for one: a later
	// source's record replaces one earlier record with its key
	// (the oldest left from an earlier source), and a remove
	// marker drops one, the oldest surviving. The slot a remove
	// frees is a one-shot credit for that source: its next record
	// with the key fills it instead of replacing a survivor.
	type mergedEntry struct {
		raw    any
		source string
	}
	var merged []*mergedEntry
	indicesByKey := make(map[string][]int)
	totalCandidates := 0
	for _, source := range loaded {
		totalCandidates += len(source.Definitions)
		freedSlots := make(map[string]int)
		for _, raw := range source.Definitions {
			key, hasKey := npcSpawnKey(raw)
			if isObject(raw) && boolField(raw, "remove") {
				if !hasKey {
					continue
				}
				survivors := indicesByKey[key]
				if len(survivors) == 0 {
					continue
				}
				dropped := survivors[0]
				indicesByKey[key] = survivors[1:]
				merged[dropped] = nil
				freedSlots[key]++
				continue
			}
			if hasKey {
				survivors := indicesByKey[key]
				freed := freedSlots[key]
				replaced := -1
				if freed > 0 {
					freedSlots[key] = freed - 1
				} else {
					for i, index := range survivors {
						if merged[index] != nil && merged[index].source != source.Name {
							replaced = i
							break
						}
					}
				}
				if replaced >= 0 {
					merged[survivors[replaced]] = nil
					survivors = append(survivors[:replaced], survivors[replaced+1:]...)
				}
				survivors = append(survivors, len(merged))
				indicesByKey[key] = survivors
			}
			merged = append(merged, &mergedEntry{raw: raw, source: source.Name})
		}
	}

	var definitions []NpcSpawn
	invalid := 0
	unsupported := 0
	for _, entry := range merged {
		if entry == nil {
			continue
		}
		spawn, ok := s.toNpcSpawn(entry.raw, entry.source)
		if !ok {
			invalid++
			continue
		}
		// TS drops ids at or past the cache NPC count
		// (line 120). With no cache the count is unknown and
		// the entry is kept — see the package comment.
		if count := s.npcCount(); count >= 0 && spawn.ID >= count {
			unsupported++
			continue
		}
		definitions = append(definitions, spawn)
	}
	s.npcSpawns = definitions
	// TS logs active/deferred counts from applyToWorld; this
	// package spawns nothing, so the log carries the load counts
	// only. The warn-vs-debug choice mirrors line 133.
	if invalid > 0 || unsupported > 0 || failures > 0 {
		s.logf("[npc-spawns] Loaded %d definitions from %s (candidates=%d, invalid=%d, unsupported=%d, failures=%d)",
			len(definitions), joinSourceNames(loaded), totalCandidates, invalid, unsupported, failures)
	} else {
		s.logf("[npc-spawns] Loaded %d definitions from %s (candidates=%d, invalid=%d, unsupported=%d)",
			len(definitions), joinSourceNames(loaded), totalCandidates, invalid, unsupported)
	}
	return nil
}

// npcSpawnKey is spawnKey (NpcSpawnDefinitionLoader.ts:147-166):
// the stable identity for overrides and removals (type id plus
// tile and plane), or no key when the id/coords do not coerce to
// finite numbers, the id is negative, or a `key` name cannot
// resolve. `key` resolution needs the generated NpcIdentifiers
// table, which is not ported: any non-empty key is unresolvable
// here, so keyed entries never merge and toNpcSpawn rejects them
// with a warning, as TS rejects an unknown NpcIdentifiers key.
func npcSpawnKey(raw any) (string, bool) {
	if !isObject(raw) {
		return "", false
	}
	if key, present := field(raw, "key"); present {
		if name, ok := key.(string); ok && len(name) > 0 {
			return "", false
		}
	}
	idNum := numField(raw, "id")
	xNum := numField(raw, "x")
	yNum := numField(raw, "y")
	zNum := numField(raw, "level")
	if !isFiniteNumber(idNum) || jsTrunc(idNum) < 0 ||
		!isFiniteNumber(xNum) || !isFiniteNumber(yNum) || !isFiniteNumber(zNum) {
		return "", false
	}
	return fmt.Sprintf("%d:%d:%d:%d", jsTrunc(idNum), jsTrunc(xNum), jsTrunc(yNum), jsTrunc(zNum)), true
}

func isFiniteNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// toNpcSpawn is toDefinition (NpcSpawnDefinitionLoader.ts:168-210).
// It returns false for entries whose id/coords do not coerce to
// finite numbers, whose id is negative, or whose `key` cannot
// resolve. Note there is no z range check in TS, and none here:
// only finiteness is required. Direction: an explicit finite
// direction (truncated, & 7), else the cache spawnDirection
// translated through the tables when the id is in cache range,
// else south. With no cache the id is never "in range", so the
// default south applies — the same fallback TS uses for an
// out-of-range id.
func (s *Store) toNpcSpawn(raw any, source string) (NpcSpawn, bool) {
	if key, present := field(raw, "key"); present {
		if name, ok := key.(string); ok && len(name) > 0 {
			s.logf("[npc-spawns] Unknown NpcIdentifiers key %q in %s", name, source)
			return NpcSpawn{}, false
		}
	}
	idNum := numField(raw, "id")
	xNum := numField(raw, "x")
	yNum := numField(raw, "y")
	zNum := numField(raw, "level")
	if !isFiniteNumber(idNum) || jsTrunc(idNum) < 0 ||
		!isFiniteNumber(xNum) || !isFiniteNumber(yNum) || !isFiniteNumber(zNum) {
		return NpcSpawn{}, false
	}
	id, x, y, z := jsTrunc(idNum), jsTrunc(xNum), jsTrunc(yNum), jsTrunc(zNum)

	// Direction (NpcSpawnDefinitionLoader.ts:193-198): an explicit
	// finite direction (truncated, & 7) wins; otherwise the cache
	// spawnDirection translated through the tables when the id is
	// in cache range; otherwise south. Number.isFinite does not
	// coerce, so a non-number direction falls to the cache branch,
	// and with no cache to the south default — the same fallback
	// TS uses for an out-of-range id.
	clientDirection := defaultClientDirection
	if dir, ok := numberValue(mustNum(raw, "direction")); ok {
		clientDirection = jsTrunc(dir) & 7
	} else if s.cache != nil && id < s.npcCount() {
		clientDirection = cacheToClientDirection[s.cache.GetNpc(id).SpawnDirection&7]
	}
	facing := clientToServerDirection[clientDirection&7]
	radius := defaultWanderRadius
	if n, ok := numberValue(mustNum(raw, "wanderRadius")); ok {
		radius = jsTrunc(n)
		if radius < 0 {
			radius = 0
		}
	}
	name := ""
	if v, present := field(raw, "name"); present && v != nil {
		name = jsString(v, true)
	}
	return NpcSpawn{
		ID:           id,
		X:            x,
		Y:            y,
		Z:            z,
		Facing:       facing,
		WanderRadius: radius,
		Name:         name,
		Source:       source,
	}, true
}

// mustNum returns a field's raw value (nil when absent); used where
// the TS code passes raw.field to Number.isFinite, which does not
// coerce.
func mustNum(raw any, key string) any {
	v, _ := field(raw, key)
	return v
}

// loadObjectSpawns ports ObjectSpawnDefinitionLoader.load()
// (ObjectSpawnDefinitionLoader.ts:20-48): the core file is
// REQUIRED (readFileSync/JSON.parse throw, a non-array throws),
// contributed sources append after it in registry order (no
// re-sort), and every entry is validated — the first invalid entry
// fails the whole load. Valid entries become spawns (or removals,
// when remove is true); TS registers/deregisters them through
// ObjectManager immediately, which this package does not do — the
// consumer applies both lists.
func (s *Store) loadObjectSpawns() error {
	filePath := filepath.Join(s.dir, "object-spawns.json")
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", filePath, err)
	}
	var core any
	if err := json.Unmarshal(content, &core); err != nil {
		return fmt.Errorf("parsing %s: %w", filePath, err)
	}
	coreDefs, ok := core.([]any)
	if !ok {
		return fmt.Errorf("%s: expected object spawn array", filePath)
	}
	loaded, _ := s.registry.loadSources(objectSpawnsDefinitionType, s.logf)
	defs := append(append([]any{}, coreDefs...), loadedDefinitions(loaded)...)
	for _, def := range defs {
		spawn, remove, err := parseObjectSpawn(def)
		if err != nil {
			return err
		}
		if remove {
			s.objectRemovals = append(s.objectRemovals, spawn)
		} else {
			s.objectSpawns = append(s.objectSpawns, spawn)
		}
	}
	return nil
}

// parseObjectSpawn validates one raw object spawn exactly as
// ObjectSpawnDefinitionLoader.ts:28-35 does: integer id >= 0,
// integer x/y/z with z in 0..3, type (when present) an integer in
// 0..22, remove (when present) a boolean, face (when present) an
// integer in 0..3. Defaults come from the ObjectSpawnDefinition
// field initializers (ObjectSpawnDefinition.ts:71-72): face 0,
// type 10.
func parseObjectSpawn(def any) (ObjectSpawn, bool, error) {
	invalid := func() (ObjectSpawn, bool, error) {
		// TS stringifies the entry into the error
		// (JSON.stringify(def)); Go's marshal of the decoded
		// value is the same content with sorted keys.
		data, _ := json.Marshal(def)
		return ObjectSpawn{}, false, fmt.Errorf("Invalid object spawn: %s", data)
	}
	id, ok := intField(def, "id")
	if !ok || id < 0 {
		return invalid()
	}
	position := objField(def, "position")
	if position == nil {
		return invalid()
	}
	x, okX := intField(position, "x")
	y, okY := intField(position, "y")
	z, okZ := intField(position, "z")
	if !okX || !okY || !okZ || z < 0 || z > 3 {
		return invalid()
	}
	spawnType := 10
	if v, present := field(def, "type"); present {
		n, ok := numberValue(v)
		if !ok || !isJSInteger(n) || n < 0 || n > 22 {
			return invalid()
		}
		spawnType = jsTrunc(n)
	}
	remove := false
	if v, present := field(def, "remove"); present {
		b, ok := v.(bool)
		if !ok {
			return invalid()
		}
		remove = b
	}
	face := 0
	if v, present := field(def, "face"); present {
		n, ok := numberValue(v)
		if !ok || !isJSInteger(n) || n < 0 || n > 3 {
			return invalid()
		}
		face = jsTrunc(n)
	}
	return ObjectSpawn{ID: id, X: x, Y: y, Z: z, Type: spawnType, Face: face}, remove, nil
}

// loadedDefinitions flattens contributed sources' definitions in
// order, as the object-spawn loader's [core, ...sources].flat()
// does (ObjectSpawnDefinitionLoader.ts:26).
func loadedDefinitions(sources []LoadedSource) []any {
	var out []any
	for _, source := range sources {
		out = append(out, source.Definitions...)
	}
	return out
}

func joinSourceNames(sources []LoadedSource) string {
	if len(sources) == 0 {
		return "none"
	}
	names := make([]string, len(sources))
	for i, source := range sources {
		names[i] = source.Name
	}
	return joinPlus(names)
}

func joinPlus(names []string) string {
	out := ""
	for i, name := range names {
		if i > 0 {
			out += "+"
		}
		out += name
	}
	return out
}

// boolField reports whether a field is present and exactly true
// (the TS `raw.remove === true` check).
func boolField(obj any, key string) bool {
	v, present := field(obj, key)
	b, ok := v.(bool)
	return present && ok && b
}

// isObject reports whether a decoded JSON value is an object.
func isObject(v any) bool {
	_, ok := v.(map[string]any)
	return ok
}
