package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/session"
)

// Object wiring: the cmd/server adapter between the game layer's
// object registry (internal/game: GameObject / ObjectRegistry /
// World.GameObjects) and the session layer's per-session object
// sync (session.ObjectSync). It follows the grounditemswiring.go
// pattern: the game layer never imports protocol or session, the
// session layer never imports game, and this package imports
// both and converts between them.
//
// The binding goes through the ObjectRef interface and the
// ObjectRegistryQueries closure bundle below. The ObjectRef
// method set is exactly the GameObject surface the sync reads
// (ID, Location, Type, Face, IsBaseMap, IsMapOriginal,
// SpawnedBy, Owner), asserted at compile time below; the
// queries bundle exists because Go slices are not covariant —
// a method returning []*game.GameObject cannot satisfy an
// interface method returning []ObjectRef — so the registry is
// reached through closures that convert with a loop (see
// NewRegistryQueries).
//
// COORDINATOR CALL SITES (exact):
//   - buildServer (server.go): construct ONE shared delta feed
//     for the world and keep the queries bundle on the adapter:
//
//     a.objectQueries = NewRegistryQueries(a.world.GameObjects())
//     a.objectFeed = NewObjectDeltaFeed(a.objectQueries)
//
//     The worldAdapter needs two new fields (objectQueries,
//     objectFeed). With a nil registry (World.GameObjects()
//     answers nil before the registry exists — it never does
//     in production), NewRegistryQueries answers empty queries
//     and the sync sends nothing.
//   - AddPlayer (server.go, next to installGroundItemSource):
//     install the per-session source before queueing, so the
//     first FlushTick already sends the scene snapshot:
//
//     InstallObjectSync(ep.Session,
//         NewRegistryObjectSource(a.objectQueries, p, a.objectFeed), p)

// ObjectRef is the GameObject surface the sync reads: the
// GameObject methods ID, Location, Type, Face, IsBaseMap,
// IsMapOriginal, SpawnedBy, and Owner. Location returns
// game.Location (value type, X/Y/Z ints); Owner returns the
// owning player or nil. PrivateArea is deliberately absent:
// private areas are milestone 11 (a placeholder type no
// production code constructs), and the registry's InScene
// already excludes private-area objects, so the sync treats
// every object it sees as main-world.
type ObjectRef interface {
	ID() int
	Location() game.Location
	Type() int
	Face() int
	IsBaseMap() bool
	IsMapOriginal() bool
	SpawnedBy() string
	Owner() *game.Player
}

// *game.GameObject satisfies ObjectRef: the registry's method
// set is the surface this adapter was written against.
var _ ObjectRef = (*game.GameObject)(nil)

// ObjectDeltaRef is one registry delta over an ObjectRef:
// game.ObjectDelta (Object + Removed) in adapter form.
type ObjectDeltaRef struct {
	Object  ObjectRef
	Removed bool
}

// ObjectRegistryQueries is the registry surface the session
// sync needs, as closures. Every query runs on the tick
// goroutine, from FlushTick, without the session mutex held;
// queries must not mutate registry state. A nil query
// degrades to an empty answer (see registryObjectSource),
// except Deltas/ClearDeltas, which the feed tolerates as
// "no deltas". Build the bundle with NewRegistryQueries.
type ObjectRegistryQueries struct {
	// InScene answers the registry InScene(baseX, baseY, size)
	// query: the global main-world objects in the scene box.
	// The registry excludes owner objects and private-area
	// objects from it; owner objects arrive through
	// OwnerInScene, and the session's owner filter is
	// retained as the TS-faithful guard.
	InScene func(baseX, baseY, size int) []ObjectRef
	// Removed answers the registry RemovedObjects() query:
	// World.getRemovedObjects(), the map objects removed
	// while scenes were away, replayed as removals.
	Removed func() []ObjectRef
	// Get answers the registry Get(id, loc) query:
	// MapObjects.get with a nil private area, loading the
	// region first, with the z >= 4 clamp and the
	// preferReachable rule. The bool is false when no such
	// loc stands there; the ref must be ignored then (a nil
	// *game.GameObject converts to a non-nil interface, so
	// the bool, not a nil check, carries the answer).
	Get func(id int, loc game.Location) (ObjectRef, bool)
	// GetType answers the registry GetType(loc, typ) query:
	// MapObjects.getType with a nil private area, loading
	// the region first, first match in tile-map order, no
	// preferReachable step. The bool carries the answer,
	// as for Get.
	GetType func(loc game.Location, typ int) (ObjectRef, bool)
	// OwnerInScene answers the registry
	// OwnerInScene(player, baseX, baseY, size) query:
	// MapObjects.ownerObjectsInScene for one player. The
	// owner is a parameter (not a captured closure) because
	// the bundle is built once per world and shared by
	// every session's source.
	OwnerInScene func(owner *game.Player, baseX, baseY, size int) []ObjectRef
	// Deltas answers the registry Deltas() query: the
	// deltas accumulated since the last ClearDeltas.
	Deltas func() []ObjectDeltaRef
	// ClearDeltas is the registry ClearDeltas(): the feed
	// calls it once per cycle, after snapshotting.
	ClearDeltas func()
}

// NewRegistryQueries builds the queries bundle over reg,
// converting []*game.GameObject to []ObjectRef with a loop
// (Go slices are not covariant). A nil registry yields a
// bundle whose queries all answer empty, so the sync sends
// nothing instead of crashing the flush.
func NewRegistryQueries(reg *game.ObjectRegistry) ObjectRegistryQueries {
	if reg == nil {
		return ObjectRegistryQueries{}
	}
	refs := func(objs []*game.GameObject) []ObjectRef {
		if objs == nil {
			return nil
		}
		out := make([]ObjectRef, len(objs))
		for i, obj := range objs {
			out[i] = obj
		}
		return out
	}
	return ObjectRegistryQueries{
		InScene: func(baseX, baseY, size int) []ObjectRef {
			return refs(reg.InScene(baseX, baseY, size))
		},
		Removed: func() []ObjectRef {
			return refs(reg.RemovedObjects())
		},
		Get: func(id int, loc game.Location) (ObjectRef, bool) {
			obj := reg.Get(id, loc)
			return obj, obj != nil
		},
		GetType: func(loc game.Location, typ int) (ObjectRef, bool) {
			obj := reg.GetType(loc, typ)
			return obj, obj != nil
		},
		OwnerInScene: func(owner *game.Player, baseX, baseY, size int) []ObjectRef {
			return refs(reg.OwnerObjectsInScene(owner, baseX, baseY, size))
		},
		Deltas: func() []ObjectDeltaRef {
			deltas := reg.Deltas()
			if deltas == nil {
				return nil
			}
			out := make([]ObjectDeltaRef, len(deltas))
			for i, d := range deltas {
				out[i] = ObjectDeltaRef{Object: d.Object, Removed: d.Removed}
			}
			return out
		},
		ClearDeltas: reg.ClearDeltas,
	}
}

// convertObjectView converts one registry object to its session
// view, field for field. A nil owner (or a nil ref) becomes a
// nil Owner interface: a typed-nil *game.Player stored in an
// any would compare non-nil and break the owner filter, so the
// nil check is explicit. A nil ref converts to the zero view,
// which the sync treats as an unowned loc id 0 — callers must
// not pass nil refs from production queries.
func convertObjectView(ref ObjectRef) session.ObjectView {
	if ref == nil {
		return session.ObjectView{}
	}
	loc := ref.Location()
	view := session.ObjectView{
		ID:            ref.ID(),
		X:             loc.X,
		Y:             loc.Y,
		Z:             loc.Z,
		Type:          ref.Type(),
		Face:          ref.Face(),
		IsBaseMap:     ref.IsBaseMap(),
		IsMapOriginal: ref.IsMapOriginal(),
		SpawnedBy:     ref.SpawnedBy(),
	}
	if owner := ref.Owner(); owner != nil {
		view.Owner = owner
	}
	return view
}

// convertObjectViews converts a slice of refs to views,
// preserving order. A nil query result converts to nil.
func convertObjectViews(refs []ObjectRef) []session.ObjectView {
	if refs == nil {
		return nil
	}
	views := make([]session.ObjectView, len(refs))
	for i, ref := range refs {
		views[i] = convertObjectView(ref)
	}
	return views
}

// registryObjectSource implements session.ObjectSource over a
// queries bundle. One instance serves one session (it carries
// that session's owner for the OwnerInScene query); all
// instances share one delta feed. Nil queries degrade to empty
// answers, so a partially bound registry sends a partial sync
// instead of crashing the flush.
type registryObjectSource struct {
	queries ObjectRegistryQueries
	owner   *game.Player
	feed    *session.SharedObjectDeltaFeed
}

// NewRegistryObjectSource returns the session.ObjectSource for
// one player over the queries bundle, sharing feed for deltas.
// owner selects whose owner objects OwnerSceneObjects answers;
// the owner filter itself compares view Owner values against
// the identity InstallObjectSync installs, not this parameter.
// A nil feed degrades Deltas to empty.
func NewRegistryObjectSource(queries ObjectRegistryQueries, owner *game.Player, feed *session.SharedObjectDeltaFeed) session.ObjectSource {
	return &registryObjectSource{queries: queries, owner: owner, feed: feed}
}

// SceneObjects answers the scene scan through InScene.
func (s *registryObjectSource) SceneObjects(baseX, baseY, size int) []session.ObjectView {
	if s == nil || s.queries.InScene == nil {
		return nil
	}
	return convertObjectViews(s.queries.InScene(baseX, baseY, size))
}

// RemovedObjects answers the removed-objects list.
func (s *registryObjectSource) RemovedObjects() []session.ObjectView {
	if s == nil || s.queries.Removed == nil {
		return nil
	}
	return convertObjectViews(s.queries.Removed())
}

// StandingObject answers MapObjects.getType(location, type,
// null) through the registry GetType query, which loads the
// region first and returns the first match in tile-map order
// with no preferReachable step (MapObjects.ts:135-166). The
// session pre-clamps z >= 4 to 0; the registry clamps again,
// idempotently.
func (s *registryObjectSource) StandingObject(x, y, z, shape int) (session.ObjectView, bool) {
	if s == nil || s.queries.GetType == nil {
		return session.ObjectView{}, false
	}
	ref, ok := s.queries.GetType(game.Location{X: x, Y: y, Z: z}, shape)
	if !ok {
		return session.ObjectView{}, false
	}
	return convertObjectView(ref), true
}

// MapObject answers MapObjects.get(id, location, null) through
// the registry Get query, which loads the region first and
// applies the z >= 4 clamp and the preferReachable rule
// (MapObjects.ts:94-122, 128-133). The session pre-clamps
// z >= 4 to 0; the registry clamps again, idempotently.
func (s *registryObjectSource) MapObject(id, x, y, z int) (session.ObjectView, bool) {
	if s == nil || s.queries.Get == nil {
		return session.ObjectView{}, false
	}
	ref, ok := s.queries.Get(id, game.Location{X: x, Y: y, Z: z})
	if !ok {
		return session.ObjectView{}, false
	}
	return convertObjectView(ref), true
}

// OwnerSceneObjects answers the player's owner objects in the
// scene box through the OwnerInScene query, passing this
// source's owner.
func (s *registryObjectSource) OwnerSceneObjects(baseX, baseY, size int) []session.ObjectView {
	if s == nil || s.queries.OwnerInScene == nil {
		return nil
	}
	return convertObjectViews(s.queries.OwnerInScene(s.owner, baseX, baseY, size))
}

// Deltas answers one cycle's delta snapshot through the shared
// feed (see session.SharedObjectDeltaFeed).
func (s *registryObjectSource) Deltas(cycle int64) []session.ObjectDeltaView {
	if s == nil || s.feed == nil {
		return nil
	}
	return s.feed.Deltas(cycle)
}

// NewObjectDeltaFeed returns the world-shared delta feed over
// the queries bundle: each fetch snapshots Deltas, converts,
// and clears, exactly once per cycle. A nil Deltas query
// answers no deltas; a nil ClearDeltas skips the clear (the
// registry then re-reports the same deltas until bound — the
// coordinator must bind both together).
func NewObjectDeltaFeed(queries ObjectRegistryQueries) *session.SharedObjectDeltaFeed {
	return session.NewSharedObjectDeltaFeed(func() []session.ObjectDeltaView {
		if queries.Deltas == nil {
			return nil
		}
		refs := queries.Deltas()
		deltas := make([]session.ObjectDeltaView, len(refs))
		for i, d := range refs {
			deltas[i] = session.ObjectDeltaView{Object: convertObjectView(d.Object), Removed: d.Removed}
		}
		if queries.ClearDeltas != nil {
			queries.ClearDeltas()
		}
		return deltas
	})
}

// InstallObjectSync installs the object source on the session's
// ObjectSync, with ownerIdentity as the identity the owner
// filter compares view Owners against (the *game.Player
// pointer). A nil session or a nil sync installs nothing. It
// runs at AddPlayer time, before the player is queued, so the
// first FlushTick already sends the scene snapshot.
func InstallObjectSync(sess *session.Session, src session.ObjectSource, ownerIdentity any) {
	if sess == nil {
		return
	}
	sync := sess.Objects()
	if sync == nil {
		return
	}
	sync.SetObjectSource(src, ownerIdentity)
}

// Ensure registryObjectSource satisfies the session interface
// at compile time.
var _ session.ObjectSource = (*registryObjectSource)(nil)
