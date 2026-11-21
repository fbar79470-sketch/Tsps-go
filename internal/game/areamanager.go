package game

// AreaManager is game/model/areas/AreaManager.ts: the ordered
// registry of areas (registration order = priority order) plus
// the per-tick enter/leave/process algorithm.
//
// TS implements it as statics (AreaManager.areas,
// AreaManager.process); Go uses one manager value per world,
// constructed with NewAreaManager and shared by the tick loop
// and the hub's registerArea. There is no deregistration: TS
// only ever pushes onto the array (PluginManager.ts:3523-3526),
// and even destroyed private areas stay registered — so none
// is offered here. Tests construct a fresh manager per case.
//
// The manager is tick-goroutine-only: Process runs from
// Player.process (Player.ts:553, after movement, walk-to, and
// aggression) and NPC.process (NPC.ts:378), and the hint map
// and search index are unguarded. Register must happen before
// ticking starts or between ticks; registering mid-tick is
// safe only in the sense that the next lookup rebuilds the
// search index (see ensureAreaSearchIndex), but concurrent
// registration from another goroutine is not supported —
// areas register at boot/plugin load, as in TS.
//
// WILDERNESS FALLBACK. InMulti falls back to the Wilderness
// zone boundaries when the actor's area is not multi
// (AreaManager.ts:109-115). The manager holds an optional
// *Wilderness for that check, installed with SetWilderness.
// With none installed the fallback degenerates to false: an
// area-less player in a multi-combat zone never reports
// multi. The coordinator MUST install the world's Wilderness
// at boot; see the report.
type AreaManager struct {
	areas      []*Area
	hints      map[AreaActor]areaHint
	index      *areaSearchIndex
	wilderness *Wilderness
}

// NewAreaManager returns an empty manager: no areas, no
// hints, no Wilderness fallback (see SetWilderness).
func NewAreaManager() *AreaManager {
	return &AreaManager{hints: make(map[AreaActor]areaHint)}
}

// Register appends area to the registry. Registration order
// is priority order: Get returns the first registered area
// containing the position. Register panics on a nil area —
// TS would push it and throw opaquely on the first lookup;
// failing at registration names the mistake. (The hub's
// registerArea sets the area's plugin name before calling
// this; see Area.SetPluginName.)
func (m *AreaManager) Register(area *Area) {
	if area == nil {
		panic("game: AreaManager.Register called with a nil area")
	}
	m.areas = append(m.areas, area)
}

// Areas returns the registered areas in registration order.
// The slice is a copy; mutating it does not affect the
// registry.
func (m *AreaManager) Areas() []*Area {
	out := make([]*Area, len(m.areas))
	copy(out, m.areas)
	return out
}

// SetWilderness installs the Wilderness used by the InMulti
// fallback (AreaManager.ts:109-115). A nil Wilderness means
// the fallback always answers false; see the type comment.
func (m *AreaManager) SetWilderness(w *Wilderness) { m.wilderness = w }

// Wilderness returns the installed Wilderness, or nil when
// none was installed. The player aggression environment
// reads it (NpcAggression.isAggressiveTo needs the
// Wilderness level term); a nil Wilderness reads as no
// wilderness, matching the AggressionEnv zero value.
func (m *AreaManager) Wilderness() *Wilderness {
	if m == nil {
		return nil
	}
	return m.wilderness
}

// Forget drops actor's area hint. TS keeps hints in a
// WeakMap (AreaManager.ts:11), so a garbage-collected actor's
// hint vanishes with it; Go has no weak map, so hints live in
// an ordinary map keyed by actor identity, and the entry —
// which keeps the departed actor alive — must be dropped
// explicitly. The coordinator MUST call Forget when a Mobile
// leaves the world (logout/removal), or the hint map grows
// by one entry per departed actor forever. (Hints key by
// identity, so a stale entry cannot alias a later actor that
// reuses the same MobileList slot; the leak is the hazard.)
// Forgetting is also what re-enables resolution after a
// nil-area hint goes stale: see Process — a stationary actor
// with a nil-area hint skips lookups until it moves or the
// hint is forgotten.
func (m *AreaManager) Forget(actor AreaActor) {
	delete(m.hints, actor)
}

// MultiIconActor is the optional AreaActor extension for the
// multi-combat icon (Player.ts:1182-1191, getMultiIcon /
// setMultiIcon). AreaManager.Process reaches it through a
// type assertion gated on IsPlayer, so NPCs — which have no
// multi icon — need no stub methods, and an actor that does
// not implement it simply never has its icon updated. The
// coordinator implements it on *Player (see the report).
//
// The initial value matters: TS declares `multiIcon` without
// initializing it (Player.ts:159), so the first Process
// comparison (`undefined != 0`, AreaManager.ts:91) is true
// and the first tick ALWAYS sends the icon, even 0. The Go
// *Player must therefore start with a sentinel outside
// {0, 1} (e.g. -1), so the first comparison likewise sends;
// starting at 0 would silently skip the initial varbit send.
type MultiIconActor interface {
	// MultiIcon returns the last icon value sent (0 single,
	// 1 multi), or the unset sentinel before the first send.
	MultiIcon() int
	// SetMultiIcon records icon and sends it to the client
	// when it changed (Player.ts:1186-1191: the equality
	// check lives in setMultiIcon itself, so this method
	// must change-detect even though Process also compares).
	SetMultiIcon(icon int)
}

// MultiIconSender is the optional SessionPort extension for
// the multi-combat icon send: the session side of
// Player.SetMultiIcon. SessionPort itself lives in player.go
// (which this phase does not edit), so — following the
// ChatMessageSender precedent (message.go) — the send is a
// separate interface the coordinator's SetMultiIcon asserts
// the attached session to, dropping the send when the
// session does not implement it. *session.Session implements
// it (internal/session/multiicon.go), and the coordinator
// attaches a *session.Session as the SessionPort, so in
// production the assertion always succeeds. A future phase
// may promote SendMultiIcon onto SessionPort proper.
type MultiIconSender interface {
	// SendMultiIcon queues the multi-combat varbit packet
	// (PacketSender.sendMultiIcon, PacketSender.ts:774-776).
	SendMultiIcon(value int)
}

// Process runs one tick of area processing for actor, porting
// AreaManager.process (AreaManager.ts:18-107) step for step.
// The comments cite the TS lines each block mirrors; the
// order matters and is preserved exactly, including two
// subtleties called out below.
//
// SUBTLETY 1 — the multi-icon block reads STALE area state
// when the actor just left an area: setArea(area) runs only
// AFTER the multi-icon block (AreaManager.ts:97), and
// Leave does not clear the actor's area, so when area is
// nil this tick, InMulti still sees the previous tick's
// area. This port preserves that: InMulti reads
// actor.Area(), and SetArea is called only where TS calls
// setArea. Do not "fix" it by clearing the area before the
// multi-icon block.
//
// SUBTLETY 2 — previousArea: TS declares it
// (AreaManager.ts:26), assigns it on leave (:42), and never
// reads it again. It is dead; no Go equivalent exists.
//
// The bot downsampling around the Player call site
// (Player.ts:547-554, bots process areas every third tick
// while idle) belongs to the coordinator's Player.Process
// wiring, not to this method: TS calls AreaManager.process
// unconditionally from NPC.process and conditionally from
// Player.process.
func (m *AreaManager) Process(actor AreaActor) {
	position := actor.Location()
	hintX, hintY, hintZ := position.X, position.Y, position.Z
	area := actor.Area()
	hint, hasHint := m.hints[actor]

	boundaryIndex := -1
	var resolved *areaSearchResult

	// AreaManager.ts:30-45: still inside the current area?
	if area != nil {
		hintBoundary := -1
		if hasHint && hint.area == area {
			hintBoundary = hint.boundaryIndex
		}
		boundaryIndex = findBoundaryIndex(position, area, hintBoundary)
		if boundaryIndex != -1 &&
			(!hasHint || hint.area != area ||
				hint.x != hintX || hint.y != hintY || hint.z != hintZ) &&
			m.includes(area) {
			// Moving inside a broad area can enter a
			// higher-priority overlapping area
			// (AreaManager.ts:36).
			r := m.getWithBoundaryIndex(position)
			resolved = &r
			if resolved.area != area {
				boundaryIndex = -1
			}
		}
		if boundaryIndex == -1 {
			callAreaLeave(area, actor, false)
			area = nil
		}
	}

	// AreaManager.ts:47-65: resolve a new area, if any.
	if area == nil {
		if resolved != nil {
			area = resolved.area
			boundaryIndex = resolved.boundaryIndex
		} else if hasHint && hint.area == nil &&
			hint.x == hintX && hint.y == hintY && hint.z == hintZ {
			// Stationary with a nil-area hint: still
			// nowhere. No lookup (AreaManager.ts:51-56).
			area = nil
		} else {
			r := m.getWithBoundaryIndex(position)
			resolved = &r
			area = resolved.area
			boundaryIndex = resolved.boundaryIndex
		}
		if area != nil {
			callAreaEnter(area, actor)
		}
	}

	// AreaManager.ts:67-84: process the area. The area is
	// recorded FIRST: process() may move the actor into
	// another area (Pest Control), which is only detectable
	// against what was recorded.
	if area != nil {
		actor.SetArea(area)
		processedArea := area
		callAreaProcess(area, actor)
		if actor.Area() != processedArea {
			area = actor.Area()
			currentPosition := actor.Location()
			if area == nil {
				boundaryIndex = -1
			} else {
				boundaryIndex = findBoundaryIndex(currentPosition, area, -1)
			}
			hintX, hintY, hintZ = currentPosition.X, currentPosition.Y, currentPosition.Z
		}
	}

	// AreaManager.ts:86-94: multi-icon update. InMulti reads
	// actor.Area() — see Subtlety 1 in the method comment.
	if actor.IsPlayer() {
		if mc, ok := actor.(MultiIconActor); ok {
			multiIcon := 0
			if m.InMulti(actor) {
				multiIcon = 1
			}
			if mc.MultiIcon() != multiIcon {
				mc.SetMultiIcon(multiIcon)
			}
		}
	}

	// AreaManager.ts:96-105: record the area and the hint. A
	// process() teleport within the same area still needs a
	// priority lookup next tick: the hint position is the
	// START-of-tick position unless process() changed the
	// area, in which case it is the post-process position
	// (set above).
	actor.SetArea(area)
	m.hints[actor] = areaHint{
		area:          area,
		boundaryIndex: boundaryIndex,
		x:             hintX,
		y:             hintY,
		z:             hintZ,
	}
}

// includes reports whether area is in the registry, by
// identity (AreaManager.ts:35, `areas.includes(area)`).
func (m *AreaManager) includes(area *Area) bool {
	for _, a := range m.areas {
		if a == area {
			return true
		}
	}
	return false
}

// InMulti reports whether actor is in multi-combat: its
// area's isMulti, or the Wilderness multi-combat zones at
// its location (AreaManager.ts:109-115). The area half reads
// actor.Area(), with the staleness behavior described on
// Process. With no Wilderness installed the zone half is
// false.
func (m *AreaManager) InMulti(actor AreaActor) bool {
	if area := actor.Area(); area != nil && area.IsMulti() {
		return true
	}
	if m.wilderness == nil {
		return false
	}
	loc := actor.Location()
	return m.wilderness.IsMulti(loc.X, loc.Y, loc.Z)
}

// Get returns the first registered area containing position,
// or nil (AreaManager.ts:123-125). Priority is registration
// order; within an area, boundary order decides only the
// (hint-only) boundary index, never which area wins.
func (m *AreaManager) Get(position Location) *Area {
	return m.getWithBoundaryIndex(position).area
}

// Inside reports whether position lies within any of area's
// boundaries (AreaManager.ts:146-148).
func (m *AreaManager) Inside(position Location, area *Area) bool {
	return findBoundaryIndex(position, area, -1) != -1
}

// areaSearchResult is AreaManager.ts:321-324 (AreaSearchResult).
type areaSearchResult struct {
	area          *Area
	boundaryIndex int
}

// areaHint is AreaManager.ts:326-332 (AreaHint): the last
// Process resolution for one actor, used as a fast path next
// tick.
type areaHint struct {
	area          *Area
	boundaryIndex int
	x, y, z       int
}

// getWithBoundaryIndex returns the first matching area and
// its matching boundary index (AreaManager.ts:127-138),
// scanning candidates in priority order.
func (m *AreaManager) getWithBoundaryIndex(position Location) areaSearchResult {
	for _, candidate := range m.getCandidateBoundaries(position) {
		if boundaryMatches(position, candidate.area, candidate.boundaryIndex) {
			return areaSearchResult{area: candidate.area, boundaryIndex: candidate.boundaryIndex}
		}
	}
	return areaSearchResult{area: nil, boundaryIndex: -1}
}

// findBoundaryIndex returns the index of the first boundary
// of area containing position, trying hintBoundaryIndex
// first (AreaManager.ts:150-166). It returns -1 when the
// area has no boundaries or none contains the position. A
// hint of -1 (or out of range) simply never matches first,
// and the scan skips the hint index only when it equals a
// scanned index — exactly as TS skips `index !==
// hintBoundaryIndex` (a -1 hint skips nothing).
func findBoundaryIndex(position Location, area *Area, hintBoundaryIndex int) int {
	boundaries := area.GetBoundaries()
	if len(boundaries) == 0 {
		return -1
	}
	if boundaryMatches(position, area, hintBoundaryIndex) {
		return hintBoundaryIndex
	}
	for index := range boundaries {
		if index != hintBoundaryIndex && boundaryMatches(position, area, index) {
			return index
		}
	}
	return -1
}

// boundaryMatches reports whether the boundary at
// boundaryIndex of area contains position
// (AreaManager.ts:168-178). An out-of-range index never
// matches.
func boundaryMatches(position Location, area *Area, boundaryIndex int) bool {
	boundaries := area.GetBoundaries()
	if boundaryIndex < 0 || boundaryIndex >= len(boundaries) {
		return false
	}
	return boundaries[boundaryIndex].Inside(position)
}

// The bucketed search index (AreaManager.ts:180-318) is a
// PERFORMANCE structure, ported faithfully rather than
// replaced by a linear scan: the merge order between
// bucketed and fallback candidates (by registration order,
// bucketed-first on ties) determines which boundary index a
// lookup returns when one area holds both small and huge
// boundaries, and the faithful port preserves even that
// hint-level behavior. A linear scan would return the same
// AREA in every case (first match by registration order,
// boundary order within an area) — the only observable
// difference would be the hint boundary index — but the
// index is small, and fidelity removes all doubt.
//
// Buckets are 64x64 tiles per plane; a boundary covering
// more than 1024 buckets, or one without a height (TS:
// non-finite z), goes on the fallback list, which is merged
// with the bucket's candidates by registration order.
const (
	areaIndexBucketSize = 64
	maxIndexedBuckets   = 1024
)

// areaBucketKey identifies one search bucket. TS keys buckets
// by the string `${z}:${bucketX}:${bucketY}`
// (AreaManager.ts:287-291); a struct key partitions identically
// and avoids formatting.
type areaBucketKey struct {
	z, bucketX, bucketY int
}

// indexedBoundaryRef is AreaManager.ts:334-338
// (IndexedBoundaryRef).
type indexedBoundaryRef struct {
	area          *Area
	boundaryIndex int
	areaOrder     int
}

// areaSearchIndex is AreaManager.ts:340-345
// (AreaSearchIndexState).
type areaSearchIndex struct {
	areaCount     int
	boundaryCount int
	bucketed      map[areaBucketKey][]indexedBoundaryRef
	fallback      []indexedBoundaryRef
}

// getCandidateBoundaries returns the boundaries to test for
// position, in priority order (AreaManager.ts:180-202): the
// position's bucket merged with the fallback list, or the
// fallback alone when the bucket is empty or the index holds
// no buckets at all.
func (m *AreaManager) getCandidateBoundaries(position Location) []indexedBoundaryRef {
	state := m.ensureAreaSearchIndex()
	if len(state.bucketed) == 0 {
		return state.fallback
	}
	key := areaBucketKey{
		z:       position.Z,
		bucketX: toAreaBucketCoordinate(position.X),
		bucketY: toAreaBucketCoordinate(position.Y),
	}
	bucketCandidates := state.bucketed[key]
	if len(bucketCandidates) == 0 {
		return state.fallback
	}
	if len(state.fallback) == 0 {
		return bucketCandidates
	}
	return mergeIndexedBoundaryRefs(bucketCandidates, state.fallback)
}

// ensureAreaSearchIndex returns the cached search index,
// rebuilding it when the registry changed
// (AreaManager.ts:204-281). The invalidation rule is the TS
// one: rebuild unless BOTH the area count and the total
// boundary count match the cached build. Mutating a
// boundary's coordinates in place without changing the
// counts therefore leaves a stale index — in TS too. Areas
// are registered at boot and boundaries are treated as
// immutable after registration, so this matches practice.
func (m *AreaManager) ensureAreaSearchIndex() *areaSearchIndex {
	boundaryCount := 0
	for _, area := range m.areas {
		boundaryCount += len(area.GetBoundaries())
	}
	if m.index != nil && m.index.areaCount == len(m.areas) && m.index.boundaryCount == boundaryCount {
		return m.index
	}

	bucketed := make(map[areaBucketKey][]indexedBoundaryRef)
	var fallback []indexedBoundaryRef
	for areaOrder, area := range m.areas {
		boundaries := area.GetBoundaries()
		if len(boundaries) == 0 {
			continue
		}
		for boundaryIndex, boundary := range boundaries {
			entry := indexedBoundaryRef{area: area, boundaryIndex: boundaryIndex, areaOrder: areaOrder}
			// TS routes non-finite coordinates or height to
			// the fallback (AreaManager.ts:238-247). Go ints
			// are always finite, so only the height-less
			// case survives: a boundary without a height
			// (TS: undefined, non-finite) cannot be
			// bucketed by plane.
			if !boundary.HasHeight() {
				fallback = append(fallback, entry)
				continue
			}
			minBucketX := toAreaBucketCoordinate(boundary.X())
			maxBucketX := toAreaBucketCoordinate(boundary.X2())
			minBucketY := toAreaBucketCoordinate(boundary.Y())
			maxBucketY := toAreaBucketCoordinate(boundary.Y2())
			// A map-wide boundary (a PvP world's zone)
			// would fill ~65k buckets per plane; one
			// rectangle test on the fallback path is
			// cheaper than indexing it
			// (AreaManager.ts:253-255).
			if (maxBucketX-minBucketX+1)*(maxBucketY-minBucketY+1) > maxIndexedBuckets {
				fallback = append(fallback, entry)
				continue
			}
			for bucketX := minBucketX; bucketX <= maxBucketX; bucketX++ {
				for bucketY := minBucketY; bucketY <= maxBucketY; bucketY++ {
					key := areaBucketKey{z: boundary.Height(), bucketX: bucketX, bucketY: bucketY}
					bucketed[key] = append(bucketed[key], entry)
				}
			}
		}
	}
	m.index = &areaSearchIndex{
		areaCount:     len(m.areas),
		boundaryCount: boundaryCount,
		bucketed:      bucketed,
		fallback:      fallback,
	}
	return m.index
}

// toAreaBucketCoordinate maps a tile coordinate to its
// 64-tile bucket (AreaManager.ts:283-285). Go integer
// division truncates toward zero, as Math.trunc does.
func toAreaBucketCoordinate(value int) int {
	return value / areaIndexBucketSize
}

// mergeIndexedBoundaryRefs merges two candidate lists that
// are each in registration order, preserving that order
// (AreaManager.ts:293-318). On equal registration order the
// primary (bucketed) entry comes first — the `<=`
// comparison.
func mergeIndexedBoundaryRefs(primary, secondary []indexedBoundaryRef) []indexedBoundaryRef {
	merged := make([]indexedBoundaryRef, 0, len(primary)+len(secondary))
	i, j := 0, 0
	for i < len(primary) && j < len(secondary) {
		if primary[i].areaOrder <= secondary[j].areaOrder {
			merged = append(merged, primary[i])
			i++
		} else {
			merged = append(merged, secondary[j])
			j++
		}
	}
	merged = append(merged, primary[i:]...)
	merged = append(merged, secondary[j:]...)
	return merged
}
