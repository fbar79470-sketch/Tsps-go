package game

// Wilderness is game/content/wilderness/Wilderness.ts: the
// PvP/safe/multi-combat zone helpers over the world.json zone
// boundaries, plus the wilderness-level computation.
//
// TS implements it as statics over the module-global
// WORLD_ZONE_BOUNDARIES; Go uses one Wilderness value per
// world, constructed with NewWilderness. Instance state (the
// boundary source, the roof checker, the level-source
// registry) keeps tests isolated and the wiring explicit.
// The coordinator constructs exactly one, from the loaded
// world config (see internal/config ZoneBoundaries and the
// report for the adapter), and shares it with the
// AreaManager (SetWilderness), the drop listener, and any
// future movement/combat callers.
//
// This package must never import internal/config, so the zone
// boundaries arrive through the ZoneBoundarySource seam —
// the same seam pattern as RegionDataSource and
// ItemDefinitionSource: the coordinator's cmd/server adapter
// supplies tag -> []Boundary from the loaded config. The roof
// half of IsInSafeBuilding arrives through the RoofChecker
// seam for the same reason in reverse: RegionManager lives in
// this package, but the Wilderness must stay usable (and
// testable) without a loaded collision grid, so the check is
// injected; RegionRoofChecker adapts a *RegionManager.
//
// A zero Wilderness (or a nil boundary source) answers the
// empty-world reading: no PvP ground, no safe buildings, no
// multi-combat, level 0 except from registered level sources.
// LevelAt's strips are constants and need no source.
type Wilderness struct {
	boundaries ZoneBoundarySource
	roof       RoofChecker

	// Level sources in registration order (TS: a Map, whose
	// iteration order is insertion order, Wilderness.ts:14).
	// Re-registering a name replaces its source but keeps
	// its position, as Map.set does.
	levelSourceNames []string
	levelSources     map[string]func(x, y int) (int, bool)
}

// ZoneBoundarySource supplies the world zone boundaries for
// one tag, as WORLD_ZONE_BOUNDARIES does in TS
// (WorldDefinition.ts:229-235, 274-286). An unknown tag — or
// a tag with no zones — yields no boundaries. TS would throw
// on an unknown tag (indexing an absent Record key yields
// undefined, and `.some` on undefined throws); the tags
// Wilderness queries are the fixed set below, which the
// config view always seeds, so the empty reading is
// unreachable in production and fails soft in tests instead
// of panicking. Deliberate, documented deviation from the
// throw; the queried tags are "pvp", "safe",
// "all-buildings-safe", and "multi-combat".
type ZoneBoundarySource interface {
	// ZoneBoundaries returns the boundaries tagged tag, in
	// zone order (unbounded zones expand to four per-plane
	// boundaries first — see internal/config
	// ZoneBoundaries).
	ZoneBoundaries(tag string) []Boundary
}

// RoofChecker answers Region.isUnderRoof
// (Region.ts:60-64): whether the tile is under a roof on the
// given plane. RegionRoofChecker adapts a *RegionManager; a
// nil checker means the roof half of IsInSafeBuilding is
// always false.
type RoofChecker interface {
	IsUnderRoof(x, y, plane int) bool
}

// Located is anything with a world location, for IsIn (TS:
// Mobile, read via character?.getLocation?.(),
// Wilderness.ts:43-45). *Player and *NPC satisfy it through
// Mobile.Location.
type Located interface {
	Location() Location
}

// levelledAreas is Wilderness.LEVELLED_AREAS
// (Wilderness.ts:8-12): [minX, maxX, minY, maxY], where minY
// doubles as the level-1 line. Wilderness levels only exist
// on the original Wilderness map — the surface strip and the
// caves beneath it — so a PvP zone anywhere else is
// attackable but unlevelled (Wilderness.ts:47-52).
var levelledAreas = [][4]int{
	{2944, 3391, 3520, 4351},
	{2944, 3455, 9920, 10879},
}

// NewWilderness returns a Wilderness reading zone boundaries
// from source and roof state from roof. Either may be nil:
// a nil source means no zone boundaries exist, and a nil
// roof means no tile is under a roof (see the type comment).
func NewWilderness(source ZoneBoundarySource, roof RoofChecker) *Wilderness {
	return &Wilderness{boundaries: source, roof: roof}
}

// SetLevelSource registers a plugin-given level source under
// name (Wilderness.ts:16-19, setLevelSource). The source
// answers a wilderness level for a tile, or false to pass to
// the next source. Sources are consulted in registration
// order before the levelled strips (LevelAt); a reload
// replaces the source registered under the same name,
// keeping its position in the order.
func (w *Wilderness) SetLevelSource(name string, source func(x, y int) (int, bool)) {
	if w.levelSources == nil {
		w.levelSources = make(map[string]func(x, y int) (int, bool))
	}
	if _, ok := w.levelSources[name]; !ok {
		w.levelSourceNames = append(w.levelSourceNames, name)
	}
	w.levelSources[name] = source
}

// boundariesFor returns the boundaries for tag, or none when
// no source is installed. A nil receiver also yields none.
func (w *Wilderness) boundariesFor(tag string) []Boundary {
	if w == nil || w.boundaries == nil {
		return nil
	}
	return w.boundaries.ZoneBoundaries(tag)
}

// IsInSafeBuilding reports whether location is inside an
// "all-buildings-safe" zone AND under a roof
// (Wilderness.ts:21-25). Both halves are required: the tag
// alone does not make a tile safe, and a roof outside a
// safe-buildings zone does not either. With no roof checker
// installed the roof half is false, so this always answers
// false until the coordinator wires RegionRoofChecker.
func (w *Wilderness) IsInSafeBuilding(location Location) bool {
	for _, boundary := range w.boundariesFor("all-buildings-safe") {
		if boundary.Inside(location) {
			return w.roof != nil && w.roof.IsUnderRoof(location.X, location.Y, location.Z)
		}
	}
	return false
}

// IsPvpArea reports whether location is on any PvP-tagged
// ground, safe carve-outs included (Wilderness.ts:27-33).
// The overlay hangs off this: inside a PvP area the player
// gets either the skull or, in a safe spot, the skull with
// the red cross.
func (w *Wilderness) IsPvpArea(location Location) bool {
	for _, boundary := range w.boundariesFor("pvp") {
		if boundary.Inside(location) {
			return true
		}
	}
	return false
}

// IsInLocation reports whether location is in the Wilderness
// proper: PvP ground that is neither a safe zone nor a safe
// building (Wilderness.ts:35-41). TS returns false for a
// null location; Go callers always pass a Location value,
// so no nil case exists.
func (w *Wilderness) IsInLocation(location Location) bool {
	return !w.IsInSafeBuilding(location) && !w.isInSafeZone(location) && w.IsPvpArea(location)
}

// isInSafeZone reports whether location is inside any "safe"
// zone boundary (the middle term of isInLocation,
// Wilderness.ts:39).
func (w *Wilderness) isInSafeZone(location Location) bool {
	for _, boundary := range w.boundariesFor("safe") {
		if boundary.Inside(location) {
			return true
		}
	}
	return false
}

// IsIn reports whether m is in the Wilderness proper
// (Wilderness.ts:43-45, isIn: isInLocation of the character's
// location). A nil Located yields false, as TS yields false
// for a null character. (A non-nil interface holding a typed
// nil *Player still panics on Location, as any nil-receiver
// method call would; callers pass live actors.)
func (w *Wilderness) IsIn(m Located) bool {
	if m == nil {
		return false
	}
	return w.IsInLocation(m.Location())
}

// inWilderness reports whether p stands in the Wilderness proper,
// through the Wilderness installed on p's AreaManager (Wilderness
// .isIn, Wilderness.ts:43-45, as SkillManager.setLevel reads it,
// SkillManager.ts:239). A nil player, a player with no
// AreaManager (a standalone player), or an AreaManager with no
// Wilderness installed reads as not in the Wilderness — the
// AggressionEnv zero value's degeneration (see aggressionEnv in
// player.go). The coordinator installs the AreaManager when the
// player is added to the world and the Wilderness on it from the
// loaded world config, so in production this reads live zone
// state.
func (p *Player) inWilderness() bool {
	if p == nil {
		return false
	}
	wilderness := p.areaManager.Wilderness()
	if wilderness == nil {
		return false
	}
	return wilderness.IsIn(p)
}

// LevelAt returns the wilderness level at tile (x, y)
// (Wilderness.ts:53-66). Level sources are consulted first,
// in registration order; the first source that answers wins,
// including a level of 0 (TS: `level != null`). Then the two
// levelled strips: floor((y-minY)/8)+1. Anywhere else is 0.
// The plane plays no part: levelAt takes only x and y.
func (w *Wilderness) LevelAt(x, y int) int {
	if w != nil {
		for _, name := range w.levelSourceNames {
			if level, ok := w.levelSources[name](x, y); ok {
				return level
			}
		}
	}
	for _, strip := range levelledAreas {
		if x >= strip[0] && x <= strip[1] && y >= strip[2] && y <= strip[3] {
			return (y-strip[2])/8 + 1
		}
	}
	return 0
}

// IsMulti reports whether tile (x, y, z) is in a multi-combat
// zone (Wilderness.ts:68-72). Multi-combat zones are per
// plane (world.json z); TS defaults the plane to the surface
// (z = 0), and Go callers pass 0 explicitly for that default.
func (w *Wilderness) IsMulti(x, y, z int) bool {
	location := Location{X: x, Y: y, Z: z}
	for _, boundary := range w.boundariesFor("multi-combat") {
		if boundary.Inside(location) {
			return true
		}
	}
	return false
}

// IntersectsArea reports whether the rectangle
// [minX, maxX] x [minY, maxY] on plane z overlaps any PvP
// boundary (Wilderness.ts:74-95). A boundary without a
// height counts as plane 0 (TS: `boundary.height ?? 0`).
// The TS finite-number guards vanish: Go ints are always
// finite, so no input can trip them.
func (w *Wilderness) IntersectsArea(minX, maxX, minY, maxY, z int) bool {
	for _, boundary := range w.boundariesFor("pvp") {
		// Boundary.Height returns 0 when unset — the
		// `?? 0` reading (see boundary.go).
		if boundary.Height() == z &&
			boundary.X() <= maxX &&
			boundary.X2() >= minX &&
			boundary.Y() <= maxY &&
			boundary.Y2() >= minY {
			return true
		}
	}
	return false
}

// regionRoofChecker adapts a *RegionManager to RoofChecker.
type regionRoofChecker struct {
	rm *RegionManager
}

// IsUnderRoof reports whether the tile is under a roof,
// through RegionManager.getRegion + Region.isUnderRoof
// (Wilderness.ts:23-24, RegionManager.ts:100-106,
// Region.ts:60-64). getRegion triggers a region load when no
// load is in progress, which is Go's ensureRegion; an
// unindexed tile (no region even after the load attempt)
// yields false, as TS's `?.` + `?? false` does. A nil
// RegionManager yields false.
func (c regionRoofChecker) IsUnderRoof(x, y, plane int) bool {
	if c.rm == nil {
		return false
	}
	r := c.rm.ensureRegion(x, y)
	return r != nil && r.isUnderRoof(x, y, plane)
}

// RegionRoofChecker returns a RoofChecker reading roof state
// from rm. The coordinator passes it to NewWilderness; a nil
// rm yields a checker that always answers false.
func RegionRoofChecker(rm *RegionManager) RoofChecker {
	return regionRoofChecker{rm: rm}
}
