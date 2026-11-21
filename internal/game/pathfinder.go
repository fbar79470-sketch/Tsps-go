package game

// Walk-route entry, ported from game/model/movement/path/PathFinder.ts
// at commit bfd81e3: calculateWalkRoute (PathFinder.ts:412-422) and the
// findRoute call shape of applyRsmodRoute (PathFinder.ts:145-195) only.
//
// WalkRoute calls FindRoute with locShape -1, moveNear true,
// maxWaypoints 25, a 1x1 destination, locAngle 0, and blockAccessFlags
// 0 — the exact options calculateWalkRoute passes through
// applyRsmodRoute (destWidth/destLength default 1, locAngle defaults 0,
// blockAccessFlags defaults 0, maxWaypoints defaults 25 in
// applyRsmodRoute). The queue mutation in applyRsmodRoute (reset,
// lastDestX/lastDestY, setRoute, setPathX/setPathY, addCheckpoint,
// syncDestinationFlagToRoute) is NOT ported here: the movement-queue
// worker owns applying routes to the queue, and WalkRoute returns the
// RouteResult for the caller to apply.
//
// NOT STARTED (their callers do not exist; no plugin system, no
// combat/entity/object interaction yet):
//   - calculateCombatRoute / calculateEntityRoute (PathFinder.ts)
//   - calculateRoute / calculateObjectRoute (PathFinder.ts)
//   - calculateGroundItemRoute (PathFinder.ts)
//   - calculateRouteToAny, isOnEntityRouteEnd, entityRouteArea (PathFinder.ts)
//   - reachedTile / reachedObj / reachedEntity / reachedObject
//     (PathFinder.ts) — ReachedAbsolute on RouteFinder is ported and is
//     what these wrap; the Mobile-taking wrappers are not.
//   - naiveEntityDestination, findWalkable, isDiagonalTiles (PathFinder.ts)
//   - emitNoPath and the plugin path-blocked events (no plugin system)
//   - PathFinder file logging (LOG_ENABLED / movement.log): omitted
//     entirely; no log files are created.
//
// The TS static RsmodRouteFinding instance (PathFinder.rsmodRouteFinding)
// has no Go equivalent: callers pass an explicit *RouteFinder, which
// keeps the tick-goroutine-only finder ownership visible.

// WalkRoute routes a walk click: the Go form of PathFinder.calculateWalkRoute
// (PathFinder.ts:412-422) reduced to its findRoute call shape (see the
// file comment). src is the actor's current tile; level comes from
// src.Z; size is the actor's footprint (clamped to >= 1 by FindRoute,
// matching the Math.max(size, 1) TS applies). The destination is a 1x1
// tile at (destX, destY) on the actor's level. A nil finder is replaced
// with a fresh open-ground finder (nil lookup), so WalkRoute never
// panics on a nil receiver; production callers pass a shared finder.
func WalkRoute(f *RouteFinder, src Location, size, destX, destY int) RouteResult {
	if f == nil {
		f = NewRouteFinder(nil)
	}
	return f.FindRoute(RouteOptions{
		Level:            src.Z,
		SrcX:             src.X,
		SrcY:             src.Y,
		SrcSize:          size,
		DestX:            destX,
		DestY:            destY,
		DestWidth:        1,
		DestLength:       1,
		LocAngle:         0,
		LocShape:         -1,
		BlockAccessFlags: 0,
		MaxWaypoints:     25,
		MoveNear:         true,
	})
}
