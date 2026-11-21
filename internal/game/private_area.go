package game

import "sync"

// PrivateArea ports game/model/areas/impl/PrivateArea.ts (156 lines):
// an Area that owns its members and its own per-tile clip overrides,
// destroyed when its last player leaves. TS implements it as an
// abstract class extending Area; Go implements it as a struct
// embedding *Area, following the frozen Phase 21 contract 1 — the
// embedded *Area is the identity AreaManager, Mobile.Area, and the
// sounds area rule compare, and PrivateArea.Add sets the actor's
// Area to it (TS entity.setArea(this), PrivateArea.ts:52).
//
// IDENTITY AND THE EMBEDDED AREA. The Area methods (Enter, Leave,
// HasMember, GetPlayers, GetNPCs, GetBoundaries, GetName) are
// promoted from the embedded *Area, so a PrivateArea behaves as an
// Area everywhere one is expected. The two places TS overrides
// Area behavior are wired through the embedded Area's hook fields,
// which NewPrivateArea owns:
//
//   - PostEnterHook runs Add (TS postEnter, PrivateArea.ts:30-32).
//   - PostLeaveHook runs postLeave: Remove, then Destroy when no
//     players remain (TS postLeave, PrivateArea.ts:23-28).
//
// Content that embeds *PrivateArea and needs different leave
// behavior (a boat deck lives as long as its boat —
// BoatDeckArea.ts postLeave removes without destroying) replaces
// the embedded Area's PostLeaveHook after construction; the hook
// fields are exported for exactly that. Do not replace
// PostEnterHook: Add is what records membership and fires the
// entered event.
//
// MEMBERSHIP, TWO LISTS. TS keeps one entities array holding
// Mobiles and GameObjects together (Entity[]). Go keeps two:
// entities ([]AreaActor) for mobiles, and an objects list for
// GameObjects, because GameObject is not a Mobile and cannot
// satisfy AreaActor (no world-list index, no IsPlayer/IsNPC).
// Entities() returns the mobiles; GetObjects() returns the
// objects. GameObjects join the objects list from their
// constructor (NewGameObject calls AttachObject, mirroring
// GameObject.ts:33-35, privateArea.add(this)), and leave it via
// DetachObject (TS detach, called by ObjectManager.deregister).
// Template locs are created with the area and immediately
// detached again — they belong to the scene, not to the spawned
// entities (TemplatedInstanceArea.ts objectFor).
//
// CLIPS. TS stores clips in a Map keyed by "x,y,z"
// (PrivateArea.ts:133-152). The default store here is the same
// map, keyed by Location. Subclasses override the clip behavior
// in TS (TemplatedInstanceArea's grid, BoatDeckArea's deck
// rules); Go routes the clip methods through a ClipStore, and
// the subclass installs its store with SetClipStore. The default
// map store stays reachable through BaseClipStore, which is the
// Go form of the TS `super.hasClip` / `super.getClip` fallback a
// deck area consults before its own rules.
//
// DESTRUCTION. Destroy mirrors PrivateArea.ts:57-90: the
// world-cleanup hook runs first (NPC removal queues, object
// deregistration, ground-item deregistration — all world-owned
// systems this type cannot reach), then the member and object
// lists clear, the clips clear, and the area is marked
// destroyed. The hook is a package-level seam installed once at
// boot by the coordinator (SetPrivateAreaDestroyHook), following
// the AreaMethodCaller precedent: with no hook installed,
// Destroy only clears local state. Destroy is idempotent: a
// second call returns immediately, as TS returns when destroyed
// is already set.
//
// TRACKING. TS tracks private areas nowhere centrally: they are
// never registered with AreaManager (PrivateArea.ts:59, "never
// pass through enter() while the area is unregistered"), and no
// World list holds them — content holds the reference (a boat
// holds its deck area; instance content holds its instance).
// The privateAreaIndex below is NOT ownership tracking: it is
// the reverse index Entity.getPrivateArea needs (Entity.ts:68-70
// returns the area only when it "carries objects", i.e. is a
// PrivateArea — a duck-type check Go cannot express on the
// concrete *Area), mapping each embedded *Area back to its
// PrivateArea so Mobile.PrivateArea can answer. Entries are
// removed on Destroy; a destroyed area therefore resolves to
// nil, where TS would still return the (destroyed, clip-less)
// area — the observable difference is nil, because a destroyed
// area has no members left (Destroy runs when the last player
// leaves, and members' clip reads fall back to the region grid
// in both runtimes once the clips clear).
//
// PERSISTENCE. Nothing about a private area persists: PlayerSave
// carries no area, instance, or template fields (verified by
// grep over persistence/PlayerSave.ts at bfd81e3), and areas are
// rebuilt by content after login. All state here is session-only.
//
// CONCURRENCY. Tick-goroutine-only, like Area and AreaManager,
// except the area index, which is mutex-guarded because content
// may construct areas off-tick and Mobile.PrivateArea may be
// read from any goroutine that holds a Mobile.
type PrivateArea struct {
	*Area

	entities []AreaActor
	objects  []*GameObject

	// clips is the active clip store: the map store by default,
	// replaced by SetClipStore for grid/deck areas.
	clips ClipStore
	// baseClips is the map store, created lazily: the TS clips
	// Map, and the `super` fallback for installed stores.
	baseClips *mapClipStore

	destroyed bool

	// templated is the TemplatedInstanceArea this area belongs
	// to, or nil. It lets the *PrivateArea view answer the
	// template questions (UsesTemplateMap, GetTemplateObjects,
	// ResolveObject) that TS answers by virtual dispatch on the
	// subclass instance, since Go callers that recover the area
	// through Mobile.PrivateArea hold the *PrivateArea, not the
	// outer value. Set only by NewTemplatedInstanceArea.
	templated *TemplatedInstanceArea

	// countsAsMainWorld is the countsAsMainWorld answer (TS:
	// false default, PrivateArea.ts:105-107; BoatDeckArea
	// overrides it to true). Set through SetCountsAsMainWorld.
	countsAsMainWorld bool

	// GroundItemDespawnDelayHook answers the area's default
	// floor-item lifetime in ticks (TS: groundItemDespawnDelay,
	// PrivateArea.ts:110-112, null default), consulted by the
	// ground-item manager. A nil hook means no opinion. It
	// follows the Area hook-field precedent. No Go consumer
	// reads it yet: ground items carry no area reference (see
	// the destroy-hook note in Destroy).
	GroundItemDespawnDelayHook func() (int, bool)
}

// ClipStore is the per-tile clip-override surface a private area
// consults: the Go form of the PrivateArea clip methods
// (PrivateArea.ts:133-152) as an installable store, so grid and
// deck areas can replace the map behavior TS expresses with
// method overrides. Masks are int32, as RegionManager reads
// them. A store must be total over Locations: HasClip reports
// whether the store has an opinion at the tile, and GetClip is
// only consulted when HasClip is true (RegionManager.getClipping
// checks HasClip first, RegionManager.ts:472-480).
type ClipStore interface {
	HasClip(loc Location) bool
	GetClip(loc Location) int32
	SetClip(loc Location, mask int32)
	RemoveClip(loc Location)
}

// mapClipStore is the default ClipStore: the TS clips Map
// (PrivateArea.ts:133-152), keyed by Location instead of the
// "x,y,z" string — the same partition, without formatting.
type mapClipStore struct {
	clips map[Location]int32
}

func (s *mapClipStore) HasClip(loc Location) bool {
	if s == nil || s.clips == nil {
		return false
	}
	_, ok := s.clips[loc]
	return ok
}

func (s *mapClipStore) GetClip(loc Location) int32 {
	if s == nil || s.clips == nil {
		return 0
	}
	return s.clips[loc]
}

func (s *mapClipStore) SetClip(loc Location, mask int32) {
	if s == nil {
		return
	}
	if s.clips == nil {
		s.clips = make(map[Location]int32)
	}
	s.clips[loc] = mask
}

func (s *mapClipStore) RemoveClip(loc Location) {
	if s == nil || s.clips == nil {
		return
	}
	delete(s.clips, loc)
}

// privateAreaIndex maps each private area's embedded *Area back
// to the *PrivateArea, for Mobile.PrivateArea (see the type
// comment). Guarded because construction and lookup may happen
// off the tick goroutine.
var (
	privateAreaIndexMu sync.RWMutex
	privateAreaIndex   = make(map[*Area]*PrivateArea)
)

func registerPrivateArea(pa *PrivateArea) {
	if pa == nil || pa.Area == nil {
		return
	}
	privateAreaIndexMu.Lock()
	privateAreaIndex[pa.Area] = pa
	privateAreaIndexMu.Unlock()
}

func unregisterPrivateArea(pa *PrivateArea) {
	if pa == nil || pa.Area == nil {
		return
	}
	privateAreaIndexMu.Lock()
	if privateAreaIndex[pa.Area] == pa {
		delete(privateAreaIndex, pa.Area)
	}
	privateAreaIndexMu.Unlock()
}

// privateAreaForArea returns the PrivateArea whose embedded Area
// is area, or nil when area is not a private area's. It is the
// Go form of the Entity.getPrivateArea duck-type check
// (Entity.ts:68-70).
func privateAreaForArea(area *Area) *PrivateArea {
	if area == nil {
		return nil
	}
	privateAreaIndexMu.RLock()
	defer privateAreaIndexMu.RUnlock()
	return privateAreaIndex[area]
}

// privateAreaDestroyHook runs the world-owned half of Destroy
// (see the type comment). Installed once at boot by the
// coordinator via SetPrivateAreaDestroyHook; nil means Destroy
// only clears local state.
var privateAreaDestroyHook func(pa *PrivateArea)

// SetPrivateAreaDestroyHook installs the world-cleanup half of
// PrivateArea.Destroy: NPC queue removal, object deregistration,
// and ground-item deregistration for the area's members. A nil
// hook restores local-only destruction. Call once at boot,
// before ticking starts; the hook runs on the tick goroutine
// from Destroy, with the member and object lists still intact,
// and must not call Destroy re-entrantly.
func SetPrivateAreaDestroyHook(fn func(pa *PrivateArea)) {
	privateAreaDestroyHook = fn
}

// newPrivateArea builds a PrivateArea named name over
// boundaries. NewPrivateArea is the public form (name
// "PrivateArea", the TS constructor name); NewTemplatedInstanceArea
// passes its own constructor name through this variant, since
// Area.GetName stands in for the TS constructor name
// (Area.ts:122-124) and log labels use it.
func newPrivateArea(name string, boundaries []Boundary) *PrivateArea {
	pa := &PrivateArea{Area: NewArea(name, boundaries)}
	// TS postEnter/postLeave (PrivateArea.ts:23-32), wired
	// through the embedded Area's hook fields, which this type
	// owns (see the type comment). PostLeave runs Remove and
	// destroys the area when no players remain; the player
	// count is read from the embedded Area's member map, which
	// Leave has already deleted from — exactly the TS order
	// (Area.leave deletes, then postLeave runs, Area.ts:38-49).
	pa.Area.PostEnterHook = func(actor AreaActor) { pa.Add(actor) }
	pa.Area.PostLeaveHook = func(actor AreaActor, logout bool) {
		_ = logout
		pa.postLeave(actor)
	}
	registerPrivateArea(pa)
	return pa
}

// NewPrivateArea returns a private area over boundaries,
// named "PrivateArea" (the TS constructor name). A nil
// boundaries slice means the area covers nothing, as for
// NewArea. This is frozen Phase 21 contract 1: Worker C's
// BoatDeckArea constructs through it.
func NewPrivateArea(boundaries []Boundary) *PrivateArea {
	return newPrivateArea("PrivateArea", boundaries)
}

// postLeave runs the TS postLeave body (PrivateArea.ts:23-28):
// remove the actor, then destroy the area when no players
// remain. It runs from the embedded Area's PostLeaveHook, i.e.
// from Area.Leave — direct Remove calls do NOT destroy, exactly
// as TS remove() does not.
func (pa *PrivateArea) postLeave(actor AreaActor) {
	pa.Remove(actor)
	if pa.Area != nil && len(pa.Area.GetPlayers()) == 0 {
		pa.Destroy()
	}
}

// Add records actor as a member and sets the actor's Area to
// the embedded *Area (PrivateArea.ts:45-56). Adding an actor
// that is already a member is a no-op for the list but still
// sets the area, as TS sets it unconditionally. When the actor
// newly joins and is a player, the "private-area:entered"
// custom event fires through the player's installed
// EmitCustomEvent seam (nil-safe when uninstalled), with the
// TS payload shape {player, area} (PrivateArea.ts:54-55).
// Unlike Area.Enter, Add does not make the actor's previous
// area hear it leave: TS add() has no previous-area step (only
// Area.enter does, Area.ts:20-25).
func (pa *PrivateArea) Add(actor AreaActor) {
	if pa == nil || actor == nil {
		return
	}
	entered := true
	for _, e := range pa.entities {
		if e == actor {
			entered = false
			break
		}
	}
	if entered {
		pa.entities = append(pa.entities, actor)
	}
	actor.SetArea(pa.Area)
	if entered && actor.IsPlayer() {
		if p := actor.AsPlayer(); p != nil && p.EmitCustomEvent != nil {
			p.EmitCustomEvent("private-area:entered", PrivateAreaEnteredPayload{Player: p, Area: pa})
		}
	}
}

// PrivateAreaEnteredPayload is the "private-area:entered"
// custom-event payload: {player, area}
// (PrivateArea.ts:54-55). Area is the *PrivateArea the player
// joined; when it belongs to a templated instance,
// Area.Templated() recovers the outer area.
type PrivateAreaEnteredPayload struct {
	Player *Player
	Area   *PrivateArea
}

// Remove detaches actor and clears the actor's area
// (PrivateArea.ts:34-37). It does not destroy the area; only
// postLeave does.
func (pa *PrivateArea) Remove(actor AreaActor) {
	if pa == nil || actor == nil {
		return
	}
	pa.Detach(actor)
	actor.SetArea(nil)
}

// Detach drops actor from the member list without touching the
// actor's area (PrivateArea.ts:39-41).
func (pa *PrivateArea) Detach(actor AreaActor) {
	if pa == nil || actor == nil {
		return
	}
	kept := pa.entities[:0]
	for _, e := range pa.entities {
		if e != actor {
			kept = append(kept, e)
		}
	}
	pa.entities = kept
}

// Entities returns the member mobiles, in join order (the TS
// entities array, minus GameObjects — see the type comment).
// The slice is a copy; mutating it does not affect membership.
func (pa *PrivateArea) Entities() []AreaActor {
	if pa == nil {
		return nil
	}
	return append([]AreaActor(nil), pa.entities...)
}

// GetObjects returns the area's spawned objects, in attach
// order (TS getObjects, PrivateArea.ts:94-102, filtering the
// entities array for GameObjects). Template locs are not
// included: they are detached at creation. The slice is a
// copy.
func (pa *PrivateArea) GetObjects() []*GameObject {
	if pa == nil {
		return nil
	}
	return append([]*GameObject(nil), pa.objects...)
}

// AttachObject records obj as one of the area's spawned
// objects, once (TS add() applied to a GameObject, via the
// GameObject constructor, GameObject.ts:33-35). A nil object
// is ignored. The GameObject constructor calls this; content
// that builds objects by other means must call it explicitly.
func (pa *PrivateArea) AttachObject(obj *GameObject) {
	if pa == nil || obj == nil {
		return
	}
	for _, o := range pa.objects {
		if o == obj {
			return
		}
	}
	pa.objects = append(pa.objects, obj)
}

// DetachObject drops obj from the spawned-object list (TS
// detach, PrivateArea.ts:39-41, as ObjectManager.deregister
// calls it). It does not deregister the object from the world
// registry; that is the registry's job.
func (pa *PrivateArea) DetachObject(obj *GameObject) {
	if pa == nil || obj == nil {
		return
	}
	kept := pa.objects[:0]
	for _, o := range pa.objects {
		if o != obj {
			kept = append(kept, o)
		}
	}
	pa.objects = kept
}

// Destroy ends the instance (PrivateArea.ts:57-90): the
// world-cleanup hook runs first (NPCs queued for removal and
// spliced from the add queue, spawned objects deregistered,
// the removed-objects splice, instance ground items
// deregistered — all world-owned, all in the installed hook),
// then the member and object lists clear, the clips clear, and
// the area is marked destroyed and unindexed. Destroy is
// idempotent. Members' Area() pointers are NOT cleared, exactly
// as TS leaves them: the next AreaManager tick re-resolves
// each actor (a member still inside the boundaries leaves
// through the normal path; in practice Destroy runs when the
// area is already empty).
//
// GAPS in the world half, named honestly: the hook the
// coordinator installs can queue registered NPCs for removal
// and deregister spawned objects through public World/registry
// API, but it cannot splice pending NPCs from the World's add
// queue (no API removes from it) and cannot deregister
// instance ground items (Go ground items carry no area
// reference — ItemOnGround has no privateArea field, where TS
// ItemOnGround.ts:30 does). Both need coordinator-owned API
// before the hook can be complete; see the Phase 21 report.
func (pa *PrivateArea) Destroy() {
	if pa == nil || pa.destroyed {
		return
	}
	if hook := privateAreaDestroyHook; hook != nil {
		hook(pa)
	}
	pa.entities = nil
	pa.objects = nil
	if pa.baseClips != nil && pa.baseClips.clips != nil {
		clear(pa.baseClips.clips)
	}
	pa.destroyed = true
	unregisterPrivateArea(pa)
}

// Destroyed reports whether Destroy has run (TS isDestroyed,
// PrivateArea.ts:154-156).
func (pa *PrivateArea) Destroyed() bool {
	return pa != nil && pa.destroyed
}

// CountsAsMainWorld reports whether members see, and are seen
// from, the main world around them (PrivateArea.ts:104-107:
// false default; a boat deck returns true). BoatDeckArea sets
// it through SetCountsAsMainWorld; the sync-area rule
// (BoatManager.syncArea) reads it.
func (pa *PrivateArea) CountsAsMainWorld() bool {
	return pa != nil && pa.countsAsMainWorld
}

// SetCountsAsMainWorld sets the countsAsMainWorld answer. It is
// the Go form of the TS method override: a deck area calls it
// with true at construction.
func (pa *PrivateArea) SetCountsAsMainWorld(v bool) {
	if pa == nil {
		return
	}
	pa.countsAsMainWorld = v
}

// GroundItemDespawnDelay answers the area's default floor-item
// lifetime in ticks (TS groundItemDespawnDelay,
// PrivateArea.ts:110-112, null default). The second result is
// false when the area has no opinion (the TS null); the hook
// field supplies an override.
func (pa *PrivateArea) GroundItemDespawnDelay() (int, bool) {
	if pa != nil && pa.GroundItemDespawnDelayHook != nil {
		return pa.GroundItemDespawnDelayHook()
	}
	return 0, false
}

// UsesTemplateMap reports whether the area's scene is copied
// from cache template chunks (PrivateArea.ts:115-117: false
// default; TemplatedInstanceArea overrides it to true). The
// answer comes from the templated back-pointer, since callers
// holding the *PrivateArea view cannot see the outer type.
func (pa *PrivateArea) UsesTemplateMap() bool {
	return pa != nil && pa.templated != nil
}

// Templated returns the TemplatedInstanceArea this area
// belongs to, or nil when it is an ordinary private area. It
// is the bridge from the *PrivateArea view (Mobile.PrivateArea,
// the entered-event payload) to the template surface
// (BuildScenePalette, GetChunk, scene version).
func (pa *PrivateArea) Templated() *TemplatedInstanceArea {
	if pa == nil {
		return nil
	}
	return pa.templated
}

// GetTemplateObjects returns the template locs on a tile
// (PrivateArea.ts:120-122: empty default; the templated
// override returns the scene's locs). It delegates to the
// templated area when one backs this area.
func (pa *PrivateArea) GetTemplateObjects(loc Location) []*GameObject {
	if pa == nil || pa.templated == nil {
		return nil
	}
	return pa.templated.GetTemplateObjects(loc)
}

// ResolveObject validates a client-only dynamic loc on demand
// (PrivateArea.ts:125-127: null default; the templated
// override matches a template loc by id, resolving multi-loc
// variants through variantFor). variantFor maps a template
// loc's id to the id the player sees (the Go form of
// ObjectDefinition.forPlayer, ObjectDefinition.ts:81-93); a
// nil variantFor means identity (no multi-loc resolution).
// It delegates to the templated area when one backs this area.
func (pa *PrivateArea) ResolveObject(id int, loc Location, variantFor func(entryID int) int) *GameObject {
	if pa == nil || pa.templated == nil {
		return nil
	}
	return pa.templated.ResolveObject(id, loc, variantFor)
}

// clipStore returns the active clip store, creating the default
// map store on first use. It tolerates a zero-value PrivateArea
// (no constructor call): the store exists independently of the
// embedded Area.
func (pa *PrivateArea) clipStore() ClipStore {
	if pa.clips == nil {
		pa.clips = pa.baseClipStore()
	}
	return pa.clips
}

// baseClipStore returns the map store, creating it on first
// use.
func (pa *PrivateArea) baseClipStore() *mapClipStore {
	if pa.baseClips == nil {
		pa.baseClips = &mapClipStore{clips: make(map[Location]int32)}
	}
	return pa.baseClips
}

// BaseClipStore returns the default map clip store: the TS
// clips Map, and the `super` fallback an installed store
// consults before its own rules (BoatDeckArea.ts getClip
// checks super.hasClip first). The returned store is live:
// writes through it are visible to the area.
func (pa *PrivateArea) BaseClipStore() ClipStore {
	if pa == nil {
		return &mapClipStore{}
	}
	return pa.baseClipStore()
}

// SetClipStore installs the clip store the clip methods
// delegate to (the Go form of the TS method overrides in
// TemplatedInstanceArea and BoatDeckArea). A nil store
// restores the default map store. The map store's contents
// survive the swap: installing a grid store does not clear
// map overrides, and restoring the map store reveals them
// again.
func (pa *PrivateArea) SetClipStore(s ClipStore) {
	if pa == nil {
		return
	}
	if s == nil {
		pa.clips = pa.baseClipStore()
		return
	}
	pa.clips = s
}

// HasClip reports whether the area has a clip opinion at loc
// (PrivateArea.ts:146-148). For the default map store that
// means an override was set; grid and deck stores answer by
// their own rules.
func (pa *PrivateArea) HasClip(loc Location) bool {
	if pa == nil {
		return false
	}
	return pa.clipStore().HasClip(loc)
}

// GetClip returns the clip mask at loc, or 0 when the store
// has no entry (PrivateArea.ts:150-152, `?? 0`). Callers must
// check HasClip first when the distinction matters:
// RegionManager.getClipping does.
func (pa *PrivateArea) GetClip(loc Location) int32 {
	if pa == nil {
		return 0
	}
	return pa.clipStore().GetClip(loc)
}

// SetClip records the clip mask at loc (PrivateArea.ts:140-142).
func (pa *PrivateArea) SetClip(loc Location, mask int32) {
	if pa == nil {
		return
	}
	pa.clipStore().SetClip(loc, mask)
}

// RemoveClip drops the clip override at loc (PrivateArea.ts:
// 144-146). Grid stores interpret removal as resetting the
// cell to 0 (TemplatedInstanceArea.ts removeClip), not as
// unmapping the tile.
func (pa *PrivateArea) RemoveClip(loc Location) {
	if pa == nil {
		return
	}
	pa.clipStore().RemoveClip(loc)
}
