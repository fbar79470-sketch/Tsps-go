package session

import (
	"sync"

	"tsps-go/internal/protocol"
)

// Per-session object sends: the ObjectManager.ts per-player scene
// logic (ObjectManager.ts:1-434) in Go form, behind the
// PacketSender.sendObject / sendObjectRemoval calls
// (PacketSender.ts:1313-1343) it makes. In TS the shown/missed/
// hidden maps live in static WeakMaps keyed by Player and the
// register/deregister indexes live in static Maps; in Go the
// per-player state lives on ObjectSync, one per session (the
// ground-item precedent: GroundItemSync in grounditems.go), and
// the world indexes live in the game registry, reached through
// the ObjectSource interface below. The session package never
// imports game, so the registry reaches this file as plain-data
// ObjectViews converted by the cmd/server adapter.
//
// FlushTick drives the sync from two places (see flush.go):
// DrainDeltas every tick (the perform() fan-out, which in TS
// runs synchronously at register/deregister time during the
// process phases) and OnRegionChange at the two flushClient
// sites (PlayerSession.ts:509-513 initial, 648-650 non-initial).
//
// Degenerations, all from systems that do not exist in Go:
//   - viewedArea is not wired (no SyncArea-to-object-sync seam),
//     so it is always nil and every registry object is treated
//     as a main-world object for sync purposes: the
//     viewedArea/privateArea comparisons in onRegionChange and
//     perform always pass, matchesBaseMapObject always answers
//     false (it requires a private area), and sendShown always
//     records the object (TS skips recording private-area
//     objects). Template objects are not synced at all.
//   - CrossFloor does not exist, so IsTileInScene requires the
//     tile's level to equal the replayed scene level (see
//     IsTileInScene). The onRegionChange scene checks never
//     consult the level — every plane is in the scene —
//     exactly as TS does.
//   - Map iteration order differs from TS: TS iterates its
//     insertion-ordered Maps for missed removals and hidden
//     objects, Go maps iterate randomly. The frames are
//     independent (one per tile/shape key), so the client's
//     end state is identical; only the wire order varies.
//
// Thread-safety: the source, the owner identity, and the three
// maps are guarded by the sync's own mutex, and SendPacket locks
// internally, so every method here is safe from the tick
// goroutine (FlushTick) and from any other goroutine (a future
// hide/show caller). Source queries run WITHOUT holding the
// mutex: the source reads world state, and holding this mutex
// across that call would invert the lock order against world
// code that sends while holding a world lock (the same
// snapshot discipline FlushTick uses for its FlushSource and
// GroundItemSync uses for its source). Methods snapshot the
// source and the state they need under the mutex, query the
// source, then apply map mutations under short locks.

// ObjectSceneSize is the scene edge in tiles: SCENE_SIZE
// (ObjectManager.ts:9). The scene box is
// [baseX, baseX+104) x [baseY, baseY+104), all planes.
const ObjectSceneSize = 104

// ObjectView is one GameObject as the sync sees it: the pinned
// registry surface (ID, Location, Type, Face, IsBaseMap,
// IsMapOriginal, SpawnedBy, Owner) projected to plain data so
// this package need not import game. The cmd/server adapter
// converts field for field. Owner is the owning player's
// identity (the *game.Player pointer, passed through as any) or
// nil for a global object; it must be comparable, because the
// sync compares it with == against the installed owner
// identity. IsBaseMap/IsMapOriginal/SpawnedBy are carried for
// the projection's fidelity — the sync reads only ID, the tile,
// Type, Face, and Owner — and sameObjectIdentity ignores them,
// as TS does (ObjectManager.ts:169-176 compares id, type, face,
// private area, owner, and location only).
type ObjectView struct {
	ID            int
	X, Y, Z       int
	Type          int
	Face          int
	IsBaseMap     bool
	IsMapOriginal bool
	SpawnedBy     string
	Owner         any
}

// ObjectDeltaView is one registry delta: the object plus whether
// it was removed (game.ObjectDelta: Object + Removed). A delta
// whose object id is -1 is treated as a removal even when
// Removed is false, as perform does (ObjectManager.ts:333-335).
type ObjectDeltaView struct {
	Object  ObjectView
	Removed bool
}

// ObjectSource supplies the world object state one session's
// sync reads: scene queries, the removed-objects list, standing
// map-object lookups, and per-tick deltas. The cmd/server
// adapter implements it over the game registry; all methods run
// on the tick goroutine, from FlushTick, without the sync's
// mutex held. Implementations must not mutate returned slices
// after returning.
//
// The method set mirrors the TS reads ObjectManager makes:
//   - SceneObjects: the objectsByRegion scene scan
//     (ObjectManager.ts:226-249), over the pinned
//     InScene(baseX, baseY, size) registry query.
//   - RemovedObjects: World.getRemovedObjects()
//     (ObjectManager.ts:198-212), the map objects removed
//     while the scene was away, replayed as removals.
//   - StandingObject: MapObjects.getType(location, type, null)
//     (ObjectManager.ts:104-105, MapObjects.ts:135-166), the
//     map's own loc at the tile in the shape slot, used to
//     restore the standing object after a missed removal.
//     The sync clamps z >= 4 to 0 before calling, as getType
//     does (MapObjects.ts:147-149); ok is false when no loc
//     stands there.
//   - MapObject: MapObjects.get(id, location, null)
//     (ObjectManager.ts:60, 72, MapObjects.ts:94-122), the
//     base map object a hide/show call names. The sync
//     clamps z >= 4 to 0 before calling, as get does
//     (MapObjects.ts:107-109); the registry's Get applies
//     the preferReachable rule (MapObjects.ts:128-133).
//   - OwnerSceneObjects: MapObjects.ownerObjectsInScene
//     (ObjectManager.ts:250-252, MapObjects.ts:35-48), this
//     player's owner objects in the scene box.
//   - Deltas: the registry deltas accumulated since the last
//     clear, for this cycle. The adapter answers through a
//     SharedObjectDeltaFeed: the registry's Deltas()/
//     ClearDeltas() pair is world-global, so the snapshot is
//     taken and cleared once per cycle and shared by every
//     session flushing that cycle (see SharedObjectDeltaFeed).
//     The sync itself never clears.
type ObjectSource interface {
	SceneObjects(baseX, baseY, size int) []ObjectView
	RemovedObjects() []ObjectView
	StandingObject(x, y, z, shape int) (ObjectView, bool)
	MapObject(id, x, y, z int) (ObjectView, bool)
	OwnerSceneObjects(baseX, baseY, size int) []ObjectView
	Deltas(cycle int64) []ObjectDeltaView
}

// objectShownKey is the client's own key for a shown loc: the
// tile plus the shape (ObjectManager.shownKey,
// ObjectManager.ts:32-34). Face is NOT part of the key: a
// replacement in the same tile/shape slot overwrites the shown
// entry, and a removal clears it whatever the face.
type objectShownKey struct {
	x, y, z int
	typ     int
}

func shownKeyOf(v ObjectView) objectShownKey {
	return objectShownKey{x: v.X, y: v.Y, z: v.Z, typ: v.Type}
}

// sameObjectIdentity ports ObjectManager.sameObjectIdentity
// (ObjectManager.ts:169-176): id, type, face, owner, and
// location. The private-area term degenerates: no private
// areas exist in Go, so both sides are always main-world.
func sameObjectIdentity(a, b ObjectView) bool {
	return a.ID == b.ID && a.Type == b.Type && a.Face == b.Face &&
		a.Owner == b.Owner && a.X == b.X && a.Y == b.Y && a.Z == b.Z
}

// ObjectSync is one session's object send state: the registry
// source, the owner's identity, and the shown/missed/hidden
// maps. Construct it with NewObjectSync; the zero value and a
// nil receiver send nothing (no session attached, no source
// installed).
type ObjectSync struct {
	session *Session

	mu            sync.Mutex
	source        ObjectSource
	ownerIdentity any
	shown         map[objectShownKey]ObjectView
	missed        map[objectShownKey]ObjectView
	hidden        map[objectShownKey]ObjectView
}

// NewObjectSync returns the object send state for s, with empty
// shown/missed/hidden maps (the TS WeakMap entries start empty
// per player, ObjectManager.ts:22-28).
func NewObjectSync(s *Session) *ObjectSync {
	return &ObjectSync{
		session: s,
		shown:   make(map[objectShownKey]ObjectView),
		missed:  make(map[objectShownKey]ObjectView),
		hidden:  make(map[objectShownKey]ObjectView),
	}
}

// SetObjectSource installs the registry source and the owner's
// identity the owner filter compares against. A nil source
// clears it, and the sync then sends nothing. The coordinator
// installs the adapter source at AddPlayer time, before the
// player is queued, so the first FlushTick already sends the
// scene snapshot.
func (o *ObjectSync) SetObjectSource(src ObjectSource, ownerIdentity any) {
	if o == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	// Lazily initialize the maps: a sync built without
	// NewObjectSync (zero value plus a session) must not panic
	// on its first recording write.
	if o.shown == nil {
		o.shown = make(map[objectShownKey]ObjectView)
	}
	if o.missed == nil {
		o.missed = make(map[objectShownKey]ObjectView)
	}
	if o.hidden == nil {
		o.hidden = make(map[objectShownKey]ObjectView)
	}
	o.source = src
	o.ownerIdentity = ownerIdentity
}

// ObjectSource returns the installed registry source, or nil
// when none is installed.
func (o *ObjectSync) ObjectSource() ObjectSource {
	if o == nil {
		return nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.source
}

// sendObject queues a loc add/change frame for the object:
// PacketSender.sendObject (PacketSender.ts:1313-1328) without
// its untyped-argument guards (callers always pass full
// views). It records nothing; sendShown is the recording form.
func (o *ObjectSync) sendObject(v ObjectView) {
	if o == nil || o.session == nil {
		return
	}
	o.session.SendPacket(protocol.EncodeLocAddChange(v.ID, v.X, v.Y, v.Z, v.Type, v.Face))
}

// sendObjectRemoval queues a loc removal frame for the object:
// PacketSender.sendObjectRemoval (PacketSender.ts:1330-1343).
func (o *ObjectSync) sendObjectRemoval(v ObjectView) {
	if o == nil || o.session == nil {
		return
	}
	o.session.SendPacket(protocol.EncodeLocDel(v.X, v.Y, v.Z, v.Type, v.Face))
}

// sendShown sends the object and records it as shown, clearing
// any missed removal for the same key: ObjectManager.sendShown
// (ObjectManager.ts:46-52). The private-area early return
// degenerates: no private areas exist, so every sent object is
// recorded. A nil receiver or a session-less sync records
// nothing (its maps may be uninitialized).
func (o *ObjectSync) sendShown(v ObjectView) {
	if o == nil || o.session == nil {
		return
	}
	o.sendObject(v)
	key := shownKeyOf(v)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.shown[key] = v
	delete(o.missed, key)
}

// inSceneBox reports whether the tile is inside the scene box
// rooted at (baseX, baseY): the bounds check onRegionChange
// applies to every loop (ObjectManager.ts:199-212 and peers).
// The level is never consulted: the scene holds all four
// planes, and the packets carry their own level
// (ObjectManager.ts:195-197).
func inSceneBox(x, y, baseX, baseY int) bool {
	return x >= baseX && x < baseX+ObjectSceneSize && y >= baseY && y < baseY+ObjectSceneSize
}

// IsTileInScene reports whether the tile is inside the scene
// this session last replayed: PlayerSession.isTileInScene
// (PlayerSession.ts:254-264). No replayed scene, or a tile
// outside the 104-box rooted at the replayed base, is out; a
// tile on the replayed level is in. The cross-floor term
// degenerates: CrossFloor has no Go equivalent (see flush.go),
// so an off-level tile is always out, matching the main-plane
// visibility the sync encoders use. Delta gating (perform)
// uses this; the snapshot loops use inSceneBox instead, as TS
// does.
func (s *Session) IsTileInScene(x, y, level int) bool {
	if s == nil || !s.hasReplayedScene ||
		x < s.replayedSceneBaseX || x >= s.replayedSceneBaseX+ObjectSceneSize ||
		y < s.replayedSceneBaseY || y >= s.replayedSceneBaseY+ObjectSceneSize {
		return false
	}
	return level == s.replayedSceneLevel
}

// OnRegionChange ports ObjectManager.onRegionChange
// (ObjectManager.ts:184-253): the full scene snapshot for the
// scene rooted at (baseX, baseY), in TS order — missed
// removals, base-map removals before additions (including
// replacements with a different shape), hidden-object
// removals, the region scan with sendShown recording, then the
// player's owner objects sent raw. Every loop is gated on the
// scene box in x/y only; every plane is included. With no
// source installed it sends nothing.
func (o *ObjectSync) OnRegionChange(baseX, baseY int) {
	src := o.ObjectSource()
	if o == nil || o.session == nil || src == nil {
		return
	}
	// viewedArea is always nil (the viewedArea seam is unwired,
	// see the file header), so missed removals always replay
	// (ObjectManager.ts:186).
	o.sendMissedRemovals(src, baseX, baseY)

	// Base-map removals before additions, including
	// replacements with a different shape
	// (ObjectManager.ts:190-212). The viewedArea/privateArea
	// check degenerates: every object is main-world.
	for _, removed := range src.RemovedObjects() {
		if !inSceneBox(removed.X, removed.Y, baseX, baseY) {
			continue
		}
		o.sendObjectRemoval(removed)
	}

	// Objects hidden from this player only, re-hidden on
	// every scene load (ObjectManager.ts:214-222).
	for _, hidden := range o.hiddenSnapshot() {
		if !inSceneBox(hidden.X, hidden.Y, baseX, baseY) {
			continue
		}
		o.sendObjectRemoval(hidden)
	}

	// Only the regions the scene covers (ObjectManager.ts:224-
	// 249), via the source's scene query. The owner filter
	// skips objects owned by another player; matchesBaseMap-
	// Object always answers false (it requires a private
	// area); the box check re-filters region-scan overhang,
	// as TS does.
	for _, object := range src.SceneObjects(baseX, baseY, ObjectSceneSize) {
		if !o.visibleToOwner(object) {
			continue
		}
		if !inSceneBox(object.X, object.Y, baseX, baseY) {
			continue
		}
		o.sendShown(object)
	}

	// Owner objects, sent raw — NOT sendShown: TS calls
	// sendObject directly here (ObjectManager.ts:250-252),
	// so owner objects are never recorded as shown by the
	// snapshot (perform's SPAWN path does record them).
	for _, object := range src.OwnerSceneObjects(baseX, baseY, ObjectSceneSize) {
		o.sendObject(object)
	}
}

// visibleToOwner reports whether the object may be sent to this
// session's owner: an unowned object is global; an owned object
// goes only to its owner (ObjectManager.ts:233-235).
func (o *ObjectSync) visibleToOwner(v ObjectView) bool {
	if v.Owner == nil || o == nil {
		return v.Owner == nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return v.Owner == o.ownerIdentity
}

// hiddenSnapshot copies the hidden map's values for iteration
// without holding the mutex across sends.
func (o *ObjectSync) hiddenSnapshot() []ObjectView {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]ObjectView, 0, len(o.hidden))
	for _, v := range o.hidden {
		out = append(out, v)
	}
	return out
}

// sendMissedRemovals replays removals of shown objects that
// happened while the tile was outside the player's scene, for
// the tiles now inside the new scene:
// ObjectManager.sendMissedRemovals (ObjectManager.ts:88-111).
// A removal also hides the map's own loc there, so the standing
// map object is put back instead — unless it too is on the
// removed list, in which case the removal frame goes out.
func (o *ObjectSync) sendMissedRemovals(src ObjectSource, baseX, baseY int) {
	o.mu.Lock()
	missed := make([]ObjectView, 0, len(o.missed))
	for _, v := range o.missed {
		missed = append(missed, v)
	}
	o.mu.Unlock()
	if len(missed) == 0 {
		return
	}
	removed := src.RemovedObjects()
	for _, object := range missed {
		if !inSceneBox(object.X, object.Y, baseX, baseY) {
			continue
		}
		key := shownKeyOf(object)
		o.mu.Lock()
		delete(o.missed, key)
		o.mu.Unlock()
		// The z >= 4 clamp is MapObjects.getType's
		// (MapObjects.ts:147-149), applied here so the
		// source sees normalized coordinates.
		z := object.Z
		if z >= 4 {
			z = 0
		}
		if standing, ok := src.StandingObject(object.X, object.Y, z, object.Type); ok && !isRemoved(standing, removed) {
			o.sendObject(standing)
		} else {
			o.sendObjectRemoval(object)
		}
	}
}

// isRemoved reports whether standing is on the removed list by
// object identity (ObjectManager.ts:106-107).
func isRemoved(standing ObjectView, removed []ObjectView) bool {
	for _, r := range removed {
		if sameObjectIdentity(r, standing) {
			return true
		}
	}
	return false
}

// recordMissedRemoval moves a shown object to the missed map
// when it is removed while its tile is outside the player's
// scene: ObjectManager.recordMissedRemoval
// (ObjectManager.ts:386-392). It fires only when the shown
// entry exists AND matches the removed object by identity; a
// replacement already shown in the slot is left alone.
func (o *ObjectSync) recordMissedRemoval(v ObjectView) {
	if o == nil || o.session == nil {
		return
	}
	key := shownKeyOf(v)
	o.mu.Lock()
	defer o.mu.Unlock()
	sent, ok := o.shown[key]
	if !ok || !sameObjectIdentity(sent, v) {
		return
	}
	delete(o.shown, key)
	o.missed[key] = v
}

// DrainDeltas ports the per-player fan-out of
// ObjectManager.perform (ObjectManager.ts:326-375) over one
// tick's registry deltas. In TS perform runs synchronously at
// register/deregister time, during the process phases; in Go
// the registry accumulates deltas and FlushTick drains them
// once per tick, before the region_updates block, gated on the
// replayed scene — the same gating perform sees, since the
// replay bookkeeping updates only at the end of FlushTick.
// An id of -1 forces the DESPAWN branch
// (ObjectManager.ts:333-335). Out-of-scene despawns record a
// missed removal; out-of-scene spawns send nothing and record
// nothing (the next snapshot covers them). In-scene despawns
// send the removal and clear the shown/missed entries for the
// key. With no source installed it sends nothing and consumes
// nothing.
func (o *ObjectSync) DrainDeltas(cycle int64) {
	src := o.ObjectSource()
	if o == nil || o.session == nil || src == nil {
		return
	}
	for _, delta := range src.Deltas(cycle) {
		object := delta.Object
		removed := delta.Removed || object.ID == -1
		if !o.visibleToOwner(object) {
			continue
		}
		// The viewedArea/privateArea check degenerates:
		// every object is main-world (see the file header).
		if !o.session.IsTileInScene(object.X, object.Y, object.Z) {
			if removed {
				o.recordMissedRemoval(object)
			}
			continue
		}
		if removed {
			o.sendObjectRemoval(object)
			key := shownKeyOf(object)
			o.mu.Lock()
			delete(o.shown, key)
			delete(o.missed, key)
			o.mu.Unlock()
		} else {
			o.sendShown(object)
		}
	}
}

// HideMapObjectFor hides the map object id at the tile for this
// player alone (idempotent): ObjectManager.hideMapObjectFor
// (ObjectManager.ts:55-64). The base lookup is MapObjects.get
// through the source; a missing base answers false and sends
// nothing. The removal frame goes out immediately, and the
// hidden entry re-sends the removal on every later scene
// load. The z >= 4 clamp is MapObjects.get's
// (MapObjects.ts:107-109).
func (o *ObjectSync) HideMapObjectFor(id, x, y, z int) bool {
	src := o.ObjectSource()
	if o == nil || o.session == nil || src == nil {
		return false
	}
	if z >= 4 {
		z = 0
	}
	base, ok := src.MapObject(id, x, y, z)
	if !ok {
		return false
	}
	o.mu.Lock()
	o.hidden[shownKeyOf(base)] = base
	o.mu.Unlock()
	o.sendObjectRemoval(base)
	return true
}

// ShowMapObjectFor undoes HideMapObjectFor: the player sees the
// map object again (ObjectManager.showMapObjectFor,
// ObjectManager.ts:67-77). The add frame goes out only when a
// hidden entry actually existed for the key.
func (o *ObjectSync) ShowMapObjectFor(id, x, y, z int) {
	src := o.ObjectSource()
	if o == nil || o.session == nil || src == nil {
		return
	}
	if z >= 4 {
		z = 0
	}
	base, ok := src.MapObject(id, x, y, z)
	if !ok {
		return
	}
	key := shownKeyOf(base)
	o.mu.Lock()
	_, existed := o.hidden[key]
	delete(o.hidden, key)
	o.mu.Unlock()
	if existed {
		o.sendObject(base)
	}
}

// Objects returns this session's object send state, or nil for
// a zero-value Session built without NewSession (all ObjectSync
// methods are nil-safe). The cmd/server adapter reaches the
// source install and the hide/show calls through it.
func (s *Session) Objects() *ObjectSync {
	if s == nil {
		return nil
	}
	return s.objects
}

// SharedObjectDeltaFeed memoizes one cycle's registry delta
// snapshot across every session flushing that cycle. The
// registry's Deltas()/ClearDeltas() pair is world-global: if
// each session drained and cleared it, the first session
// flushed would starve the rest. The feed's fetch closure —
// built by the cmd/server adapter as "snapshot Deltas, convert,
// ClearDeltas" — runs at most once per cycle, on the first
// Deltas call for that cycle; later calls in the same cycle
// return the cached snapshot. Deltas registered while no
// session flushes accumulate in the registry and are picked up
// by the next cycle's first flush. A nil feed, or a nil fetch,
// answers nil.
type SharedObjectDeltaFeed struct {
	mu      sync.Mutex
	fetched bool
	cycle   int64
	deltas  []ObjectDeltaView
	fetch   func() []ObjectDeltaView
}

// NewSharedObjectDeltaFeed returns a feed over fetch, which
// snapshots and clears the registry deltas. One feed serves a
// whole world; the adapter constructs it once at boot.
func NewSharedObjectDeltaFeed(fetch func() []ObjectDeltaView) *SharedObjectDeltaFeed {
	return &SharedObjectDeltaFeed{fetch: fetch}
}

// Deltas returns the delta snapshot for cycle, fetching once
// per cycle. The returned slice must be treated as immutable:
// every session flushing the cycle shares it.
func (f *SharedObjectDeltaFeed) Deltas(cycle int64) []ObjectDeltaView {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.fetched && f.cycle == cycle {
		return f.deltas
	}
	f.fetched = true
	f.cycle = cycle
	if f.fetch == nil {
		f.deltas = nil
	} else {
		f.deltas = f.fetch()
	}
	return f.deltas
}
