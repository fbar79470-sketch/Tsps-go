package game

// Route finding, ported whole and faithfully from
// game/model/movement/path/RsmodRouteFinding.ts (1,556 lines) at commit
// bfd81e3. This is the BFS pathfinder MovementQueue actually calls (via
// PathFinder.applyRsmodRoute -> findRoute); no algorithm here is guessed.
//
// Concurrency: a RouteFinder holds mutable BFS arrays (directions,
// distances, ring buffers) reused across calls, exactly like the single
// static finder instance TS reuses on one event loop. It is NOT safe for
// concurrent use: tick-goroutine-only, the same convention as the local
// lists in visibility.go. No mutex is added deliberately.
//
// Private areas: TS threads a PrivateArea through every lookup. Private
// areas do not exist in Go, so there is no privateArea parameter
// anywhere here; the injected CollisionLookup takes (x, y, level) only
// and the collapse is documented at NewRouteFinder. Callers that need
// region-relative clipping bake it into the lookup closure.
//
// Integer semantics: TS uses JS numbers (float64) with bitwise ops that
// coerce to int32. Coordinates here are int (all values are small and
// exact); clipping flags, directions, and distances are int32, matching
// the TS Int32Array storage and the int32 coercion of every `&` in TS.
// Go's >> on ints is an arithmetic shift, matching TS >>. The one place
// JS shift semantics needed care is rotateBlockAccess (see its comment);
// this file has no float division and no Math.trunc equivalent.

// Search-map and ring-buffer geometry (RsmodRouteFinding.ts:3-9).
const (
	searchMapSize    = 128
	ringBufferSize   = 4096
	distanceSentinel = 99_999_999
	// srcDirectionSentinel marks the source cell in the directions array;
	// waypoint extraction stops when it walks back to a cell whose
	// direction is this value only indirectly (it stops at the source
	// coordinates), but the sentinel also keeps the source marked
	// visited (directionAt != 0) so the BFS never re-queues it.
	srcDirectionSentinel = 99
	// Alternative-route (closest-approach) limits
	// (RsmodRouteFinding.ts:7-9).
	maxAlternativeRouteLowestCost              = 1000
	maxAlternativeRouteSeekRange               = 100
	maxAlternativeRouteDistanceFromDestination = 10
)

// Direction bits stored in the directions array. A cell's direction is
// the direction BACK toward the cell the BFS came from, so waypoint
// extraction walks the stored directions from the destination to the
// source (RsmodRouteFinding.ts:11-18).
const (
	dirNorth int32 = 0x1
	dirEast  int32 = 0x2
	dirSouth int32 = 0x4
	dirWest  int32 = 0x8
)

const (
	dirSouthWest int32 = dirWest | dirSouth
	dirNorthWest int32 = dirWest | dirNorth
	dirSouthEast int32 = dirEast | dirSouth
	dirNorthEast int32 = dirEast | dirNorth
)

// Block-access bits: which sides of a destination may not be approached
// from (RsmodRouteFinding.ts:20-23). These live in the int
// BlockAccessFlags option, not in the int32 clipping flags.
const (
	blockAccessNorth = 0x1
	blockAccessEast  = 0x2
	blockAccessSouth = 0x4
	blockAccessWest  = 0x8
)

// Clipping flag bits (RsmodRouteFinding.ts:25-33). WALL_* bits describe
// walls on a tile as seen from that tile; LOC marks a solid object
// footprint; GROUND_DECOR marks a blocking floor decoration;
// BLOCK_WALK marks terrain-blocked tiles.
const (
	wallNorthWest int32 = 0x1
	wallNorth     int32 = 0x2
	wallNorthEast int32 = 0x4
	wallEast      int32 = 0x8
	wallSouthEast int32 = 0x10
	wallSouth     int32 = 0x20
	wallSouthWest int32 = 0x40
	wallWest      int32 = 0x80
	locFlag       int32 = 0x100
	groundDecor   int32 = 0x40000
	blockWalk     int32 = 0x200000
)

// Composite blocking masks (RsmodRouteFinding.ts:35-50). canMove passes
// when (tileFlag & mask) == 0. The mask names describe the direction of
// travel being tested: BLOCK_WEST is the mask a step westward must clear
// on the destination tile (a wall on the destination tile's east side
// blocks entry from the west, hence WALL_EAST in BLOCK_WEST).
const (
	floorBlocked           = blockWalk | groundDecor
	blockWest              = wallEast | locFlag | floorBlocked
	blockEast              = wallWest | locFlag | floorBlocked
	blockSouth             = wallNorth | locFlag | floorBlocked
	blockNorth             = wallSouth | locFlag | floorBlocked
	blockSouthWest         = wallNorth | wallNorthEast | wallEast | locFlag | floorBlocked
	blockSouthEast         = wallNorthWest | wallNorth | wallWest | locFlag | floorBlocked
	blockNorthWest         = wallEast | wallSouthEast | wallSouth | locFlag | floorBlocked
	blockNorthEast         = wallSouth | wallSouthWest | wallWest | locFlag | floorBlocked
	blockNorthAndSouthEast = wallNorth | wallNorthEast | wallEast | wallSouthEast | wallSouth | locFlag | floorBlocked
	blockNorthAndSouthWest = wallNorthWest | wallNorth | wallSouth | wallSouthWest | wallWest | locFlag | floorBlocked
	blockNorthEastAndWest  = wallNorthWest | wallNorth | wallNorthEast | wallEast | wallWest | locFlag | floorBlocked
	blockSouthEastAndWest  = wallEast | wallSouthEast | wallSouth | wallSouthWest | wallWest | locFlag | floorBlocked
)

// Exit strategies selected from locShape by exitStrategy
// (RsmodRouteFinding.ts:52-56).
const (
	wallStrategy               = 0
	wallDecoStrategy           = 1
	rectangleStrategy          = 2
	noStrategy                 = 3
	rectangleExclusiveStrategy = 4
)

// wallDecorDiagonalNoOffsetShape is locShape 7, the diagonal wall
// decoration whose reach angle flips by 2 (RsmodRouteFinding.ts:58).
const wallDecorDiagonalNoOffsetShape = 7

// RouteOptions is one findRoute call: the Go form of FindRouteOptions
// (RsmodRouteFinding.ts: FindRouteOptions type), minus privateArea (see
// the file comment) and with TS's `?:` defaults made explicit by
// FindRoute: DestWidth/DestLength <= 0 mean 1, MaxWaypoints <= 0 means
// 25. All other fields are used literally. SrcSize must be >= 1; a
// smaller value is clamped to 1, matching the Math.max(size, 1) every TS
// call site applies (PathFinder.ts applyRsmodRoute and friends).
type RouteOptions struct {
	Level, SrcX, SrcY, SrcSize int
	DestX, DestY               int
	DestWidth, DestLength      int
	LocAngle, LocShape         int
	BlockAccessFlags           int
	MaxWaypoints               int
	MoveNear                   bool
}

// ReachOptions is one reachedAbsolute call: the Go form of ReachOptions
// (RsmodRouteFinding.ts: ReachOptions type), minus privateArea.
// DestWidth/DestLength <= 0 mean 1, matching FindRoute.
type ReachOptions struct {
	Level, SrcX, SrcY, SrcSize int
	DestX, DestY               int
	DestWidth, DestLength      int
	LocAngle, LocShape         int
	BlockAccessFlags           int
}

// RouteWaypoint is one extracted checkpoint. The Go form of RsmodWaypoint
// (RsmodRouteFinding.ts: RsmodWaypoint type). Z is always the search
// level: routes never change planes.
type RouteWaypoint struct {
	X, Y, Z int
}

// RouteResult is the outcome of FindRoute: the Go form of
// RsmodRouteResult (RsmodRouteFinding.ts: RsmodRouteResult type).
// Success=false means no route and no approach point (waypoints empty,
// EndX/EndY are the source). Alternative=true means the route ends at a
// closest-approach point rather than the destination (moveNear fallback);
// EndX/EndY are the final waypoint's tile (the BFS end cell), which for
// a capped route is still the true route end even when the last
// waypoint was dropped by the maxWaypoints cap.
type RouteResult struct {
	Waypoints   []RouteWaypoint
	Alternative bool
	Success     bool
	EndX, EndY  int
}

// RouteFinder is the BFS route finder: the Go form of RsmodRouteFinding
// (RsmodRouteFinding.ts: RsmodRouteFinding class). The BFS arrays are
// reused across calls and reset at the start of each FindRoute. NOT safe
// for concurrent use; tick-goroutine-only (see the file comment).
type RouteFinder struct {
	directions     [searchMapSize * searchMapSize]int32
	distances      [searchMapSize * searchMapSize]int32
	validLocalX    [ringBufferSize]int32
	validLocalY    [ringBufferSize]int32
	currLocalX     int
	currLocalY     int
	bufReaderIndex int
	bufWriterIndex int
	getClipping    CollisionLookup
}

// NewRouteFinder builds a finder reading clipping through lookup: the Go
// form of the RsmodRouteFinding constructor
// (RsmodRouteFinding.ts: constructor), with the privateArea argument
// collapsed away (private areas do not exist in Go). A nil lookup means
// open ground (every tile answers 0), matching TS missing-region
// semantics (RegionManager.getClipping returns 0 for a missing region);
// TS instead lazily binds RegionManager.getClipping on first use, which
// has no Go equivalent — the coordinator wires the RegionManager lookup
// in explicitly.
func NewRouteFinder(lookup CollisionLookup) *RouteFinder {
	return &RouteFinder{getClipping: lookup}
}

// rotateDimensions swaps a footprint dimension pair for odd angles
// (RsmodRouteFinding.ts: rotateDimensions). angle&1 in Go matches TS
// (angle & 0x1) !== 0 for all ints, including negatives.
func rotateDimensions(angle, a, b int) int {
	if angle&0x1 != 0 {
		return b
	}
	return a
}

// rotateBlockAccess rotates block-access bits by angle quarter-turns
// (RsmodRouteFinding.ts: rotateBlockAccess). TS shift counts are taken
// mod 32 by the JS << and >> operators; Go panics on a negative shift
// count and shifts by the full count otherwise, so the counts are masked
// with &31 to reproduce JS semantics for out-of-range angles (in
// practice locAngle is 0-3).
func rotateBlockAccess(angle, blockAccessFlags int) int {
	blockAccessFlags &= 0xf
	if angle == 0 {
		return blockAccessFlags
	}
	return (((blockAccessFlags << uint(angle&31)) & 0xf) | (blockAccessFlags >> uint((4-angle)&31))) & 0xf
}

// collides reports whether the src rectangle overlaps the dest rectangle
// (RsmodRouteFinding.ts: collides). Note the asymmetric last line in TS
// (srcY < destY+destLength && destY < srcLength+srcY): ported verbatim,
// it is the standard rectangle-overlap test with the operands ordered as
// written.
func collides(srcX, srcY, destX, destY, srcWidth, srcLength, destWidth, destLength int) bool {
	if srcX >= destX+destWidth || srcX+srcWidth <= destX {
		return false
	}
	return srcY < destY+destLength && destY < srcLength+srcY
}

// FindRoute searches for a route from (SrcX, SrcY) to the destination
// rectangle: the Go form of findRoute (RsmodRouteFinding.ts: findRoute).
// The search map is centered on the source (base = src - 64), so
// destinations more than 64 tiles away in either axis can never be
// reached and fall back to the closest-approach scan when MoveNear is
// set. See RouteOptions for the <= 0 defaults.
func (f *RouteFinder) FindRoute(opts RouteOptions) RouteResult {
	level := opts.Level
	srcX := opts.SrcX
	srcY := opts.SrcY
	srcSize := opts.SrcSize
	if srcSize < 1 {
		// TS call sites pass Math.max(size, 1); routeFindBig with a
		// size of 0 would index outside the search map in Go (TS
		// Int32Array out-of-bounds reads return undefined and are
		// skipped). Clamping keeps invalid input sane.
		srcSize = 1
	}
	destX := opts.DestX
	destY := opts.DestY
	destWidth := opts.DestWidth
	if destWidth <= 0 {
		destWidth = 1
	}
	destLength := opts.DestLength
	if destLength <= 0 {
		destLength = 1
	}
	locAngle := opts.LocAngle
	locShape := opts.LocShape
	moveNear := opts.MoveNear
	blockAccessFlags := opts.BlockAccessFlags
	// Real OSRS/RSMod caps an extracted path at 25 checkpoints - a
	// longer route stops at the 25th and needs a fresh click to continue.
	maxWaypoints := opts.MaxWaypoints
	if maxWaypoints <= 0 {
		maxWaypoints = 25
	}

	f.reset()

	baseX := srcX - (searchMapSize >> 1)
	baseY := srcY - (searchMapSize >> 1)
	localSrcX := srcX - baseX
	localSrcY := srcY - baseY
	localDestX := destX - baseX
	localDestY := destY - baseY

	f.appendDirection(localSrcX, localSrcY, srcDirectionSentinel, 0)

	var pathFound bool
	if srcSize == 1 {
		pathFound = f.routeFindSize1(baseX, baseY, level, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
	} else {
		pathFound = f.routeFindBig(baseX, baseY, level, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags)
	}

	if !pathFound {
		if !moveNear {
			return RouteResult{Waypoints: []RouteWaypoint{}, Alternative: false, Success: false, EndX: srcX, EndY: srcY}
		}
		foundApproachPoint := f.findClosestApproachPoint(
			localDestX,
			localDestY,
			rotateDimensions(locAngle, destWidth, destLength),
			rotateDimensions(locAngle, destLength, destWidth),
		)
		if !foundApproachPoint {
			return RouteResult{Waypoints: []RouteWaypoint{}, Alternative: false, Success: false, EndX: srcX, EndY: srcY}
		}
	}

	endLocalX := f.currLocalX
	endLocalY := f.currLocalY
	waypoints := []RouteWaypoint{}
	nextDirection := f.directionAt(f.currLocalX, f.currLocalY)
	currentDirection := int32(-1)

	// Bounded exactly like RouteFinding.findRoute's `for (i in directions.indices)`.
	// A direction of 0 advances neither axis, so an unvisited or corrupt cell would
	// otherwise spin this loop forever and hang the game tick.
	// Waypoint extraction walks the stored (reverse) directions from the
	// end cell back to the source, emitting one waypoint per run of equal
	// directions (direction-change compression). When the run count
	// exceeds maxWaypoints, the oldest-emitted (destination-side) waypoint
	// is popped before each new unshift, so the kept waypoints are the
	// first maxWaypoints checkpoints from the source and the route stops
	// at the 25th; EndX/EndY still report the true end cell.
	for guard := len(f.directions); guard > 0 && (f.currLocalX != localSrcX || f.currLocalY != localSrcY); guard-- {
		if currentDirection != nextDirection {
			currentDirection = nextDirection
			if len(waypoints) >= maxWaypoints {
				waypoints = waypoints[:len(waypoints)-1]
			}
			waypoints = append([]RouteWaypoint{{X: baseX + f.currLocalX, Y: baseY + f.currLocalY, Z: level}}, waypoints...)
		}
		if currentDirection&dirEast != 0 {
			f.currLocalX++
		} else if currentDirection&dirWest != 0 {
			f.currLocalX--
		}
		if currentDirection&dirNorth != 0 {
			f.currLocalY++
		} else if currentDirection&dirSouth != 0 {
			f.currLocalY--
		}
		nextDirection = f.directionAt(f.currLocalX, f.currLocalY)
	}

	return RouteResult{
		Waypoints:   waypoints,
		Alternative: !pathFound,
		Success:     true,
		EndX:        baseX + endLocalX,
		EndY:        baseY + endLocalY,
	}
}

// ReachedAbsolute reports whether an actor already standing at
// (SrcX, SrcY) has reached the destination under the locShape exit
// strategy, without searching: the Go form of reachedAbsolute
// (RsmodRouteFinding.ts: reachedAbsolute). It reads clipping only; it
// does not touch the BFS arrays.
func (f *RouteFinder) ReachedAbsolute(opts ReachOptions) bool {
	srcSize := opts.SrcSize
	if srcSize < 1 {
		srcSize = 1
	}
	destWidth := opts.DestWidth
	if destWidth <= 0 {
		destWidth = 1
	}
	destLength := opts.DestLength
	if destLength <= 0 {
		destLength = 1
	}
	baseX := opts.SrcX - (searchMapSize >> 1)
	baseY := opts.SrcY - (searchMapSize >> 1)
	return f.reached(
		baseX,
		baseY,
		opts.Level,
		opts.SrcX-baseX,
		opts.SrcY-baseY,
		opts.DestX-baseX,
		opts.DestY-baseY,
		destWidth,
		destLength,
		srcSize,
		opts.LocAngle,
		opts.LocShape,
		opts.BlockAccessFlags,
	)
}

// routeFindSize1 is the BFS for a 1x1 actor (RsmodRouteFinding.ts:
// routeFindSize1). Expansion order is fixed and tie-break-relevant:
// west, east, south, north, then the four diagonals (south-west,
// south-east, north-west, north-east). Each neighbor stores the REVERSE
// direction (the step back toward the current cell), which is what
// waypoint extraction follows. A diagonal step additionally requires
// both orthogonal neighbours to be passable (no corner cutting): the
// diagonal tile must clear its diagonal mask and the two side tiles
// must clear their axis masks. A cell is enqueued only while its
// direction is 0 (unvisited); the source starts marked with the
// sentinel so it is never re-queued.
func (f *RouteFinder) routeFindSize1(
	baseX, baseY, level, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int,
) bool {
	relativeSearchSize := searchMapSize - 1
	for f.bufWriterIndex != f.bufReaderIndex {
		f.currLocalX = int(f.validLocalX[f.bufReaderIndex])
		f.currLocalY = int(f.validLocalY[f.bufReaderIndex])
		f.bufReaderIndex = (f.bufReaderIndex + 1) & (ringBufferSize - 1)

		if f.reached(baseX, baseY, level, f.currLocalX, f.currLocalY, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags) {
			return true
		}

		nextDistance := f.distanceAt(f.currLocalX, f.currLocalY) + 1

		x := f.currLocalX - 1
		y := f.currLocalY
		if f.currLocalX > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockWest) {
			f.appendDirection(x, y, dirEast, nextDistance)
		}

		x = f.currLocalX + 1
		y = f.currLocalY
		if f.currLocalX < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockEast) {
			f.appendDirection(x, y, dirWest, nextDistance)
		}

		x = f.currLocalX
		y = f.currLocalY - 1
		if f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouth) {
			f.appendDirection(x, y, dirNorth, nextDistance)
		}

		x = f.currLocalX
		y = f.currLocalY + 1
		if f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockNorth) {
			f.appendDirection(x, y, dirSouth, nextDistance)
		}

		x = f.currLocalX - 1
		y = f.currLocalY - 1
		if f.currLocalX > 0 &&
			f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouthWest) &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY, level), blockWest) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX, y, level), blockSouth) {
			f.appendDirection(x, y, dirNorthEast, nextDistance)
		}

		x = f.currLocalX + 1
		y = f.currLocalY - 1
		if f.currLocalX < relativeSearchSize &&
			f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouthEast) &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY, level), blockEast) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX, y, level), blockSouth) {
			f.appendDirection(x, y, dirNorthWest, nextDistance)
		}

		x = f.currLocalX - 1
		y = f.currLocalY + 1
		if f.currLocalX > 0 &&
			f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockNorthWest) &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY, level), blockWest) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX, y, level), blockNorth) {
			f.appendDirection(x, y, dirSouthEast, nextDistance)
		}

		x = f.currLocalX + 1
		y = f.currLocalY + 1
		if f.currLocalX < relativeSearchSize &&
			f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockNorthEast) &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY, level), blockEast) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX, y, level), blockNorth) {
			f.appendDirection(x, y, dirSouthWest, nextDistance)
		}
	}
	return false
}

// routeFindBig is the BFS for an actor of size > 1 (RsmodRouteFinding.ts:
// routeFindBig). The actor's position is its south-west (lowest x/y)
// footprint corner; every step checks the whole leading edge of the
// footprint, not a single tile. Axis steps check the two footprint
// corners on the leading edge plus each interior edge tile with the
// wide composite masks; diagonal steps check a corner tile plus, for
// each footprint row/column, the pair of edge tiles the footprint
// sweeps. Expansion order matches routeFindSize1 (W, E, S, N, SW, SE,
// NW, NE) and the stored directions are likewise reversed. The search
// bound shrinks by the footprint: relativeSearchSize is
// SEARCH_MAP_SIZE - srcSize, so the whole footprint stays in the map.
func (f *RouteFinder) routeFindBig(
	baseX, baseY, level, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int,
) bool {
	relativeSearchSize := searchMapSize - srcSize
	for f.bufWriterIndex != f.bufReaderIndex {
		f.currLocalX = int(f.validLocalX[f.bufReaderIndex])
		f.currLocalY = int(f.validLocalY[f.bufReaderIndex])
		f.bufReaderIndex = (f.bufReaderIndex + 1) & (ringBufferSize - 1)

		if f.reached(baseX, baseY, level, f.currLocalX, f.currLocalY, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags) {
			return true
		}

		nextDistance := f.distanceAt(f.currLocalX, f.currLocalY) + 1

		x := f.currLocalX - 1
		y := f.currLocalY
		if f.currLocalX > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouthWest) &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY+srcSize-1, level), blockNorthWest) {
			blocked := false
			for i := 1; i < srcSize-1; i++ {
				if !canMove(f.flagAt(baseX, baseY, x, f.currLocalY+i, level), blockNorthAndSouthEast) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirEast, nextDistance)
			}
		}

		x = f.currLocalX + 1
		y = f.currLocalY
		if f.currLocalX < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, y, level), blockSouthEast) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, f.currLocalY+srcSize-1, level), blockNorthEast) {
			blocked := false
			for i := 1; i < srcSize-1; i++ {
				if !canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, f.currLocalY+i, level), blockNorthAndSouthWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirWest, nextDistance)
			}
		}

		x = f.currLocalX
		y = f.currLocalY - 1
		if f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouthWest) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize-1, y, level), blockSouthEast) {
			blocked := false
			for i := 1; i < srcSize-1; i++ {
				if !canMove(f.flagAt(baseX, baseY, f.currLocalX+i, y, level), blockNorthEastAndWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirNorth, nextDistance)
			}
		}

		x = f.currLocalX
		y = f.currLocalY + 1
		if f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY+srcSize, level), blockNorthWest) &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize-1, f.currLocalY+srcSize, level), blockNorthEast) {
			blocked := false
			for i := 1; i < srcSize-1; i++ {
				if !canMove(f.flagAt(baseX, baseY, x+i, f.currLocalY+srcSize, level), blockSouthEastAndWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirSouth, nextDistance)
			}
		}

		x = f.currLocalX - 1
		y = f.currLocalY - 1
		if f.currLocalX > 0 &&
			f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, y, level), blockSouthWest) {
			blocked := false
			for i := 1; i < srcSize; i++ {
				if !canMove(f.flagAt(baseX, baseY, x, f.currLocalY+i-1, level), blockNorthAndSouthEast) ||
					!canMove(f.flagAt(baseX, baseY, f.currLocalX+i-1, y, level), blockNorthEastAndWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirNorthEast, nextDistance)
			}
		}

		x = f.currLocalX + 1
		y = f.currLocalY - 1
		if f.currLocalX < relativeSearchSize &&
			f.currLocalY > 0 &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, y, level), blockSouthEast) {
			blocked := false
			for i := 1; i < srcSize; i++ {
				if !canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, f.currLocalY+i-1, level), blockNorthAndSouthWest) ||
					!canMove(f.flagAt(baseX, baseY, f.currLocalX+i, y, level), blockNorthEastAndWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirNorthWest, nextDistance)
			}
		}

		x = f.currLocalX - 1
		y = f.currLocalY + 1
		if f.currLocalX > 0 &&
			f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, x, f.currLocalY+srcSize, level), blockNorthWest) {
			blocked := false
			for i := 1; i < srcSize; i++ {
				if !canMove(f.flagAt(baseX, baseY, x, f.currLocalY+i, level), blockNorthAndSouthEast) ||
					!canMove(f.flagAt(baseX, baseY, f.currLocalX+i-1, f.currLocalY+srcSize, level), blockSouthEastAndWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirSouthEast, nextDistance)
			}
		}

		x = f.currLocalX + 1
		y = f.currLocalY + 1
		if f.currLocalX < relativeSearchSize &&
			f.currLocalY < relativeSearchSize &&
			f.directionAt(x, y) == 0 &&
			canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, f.currLocalY+srcSize, level), blockNorthEast) {
			blocked := false
			for i := 1; i < srcSize; i++ {
				if !canMove(f.flagAt(baseX, baseY, f.currLocalX+i, f.currLocalY+srcSize, level), blockSouthEastAndWest) ||
					!canMove(f.flagAt(baseX, baseY, f.currLocalX+srcSize, f.currLocalY+i, level), blockNorthAndSouthWest) {
					blocked = true
					break
				}
			}
			if !blocked {
				f.appendDirection(x, y, dirSouthWest, nextDistance)
			}
		}
	}
	return false
}

// findClosestApproachPoint scans the 21x21 square around the destination
// for the visited cell nearest the (already rotation-adjusted) dest
// rectangle, for the moveNear fallback (RsmodRouteFinding.ts:
// findClosestApproachPoint). Cells outside the search map or with a BFS
// distance >= 100 are skipped. Cost is squared Euclidean distance to
// the rectangle (0 inside it); a strictly lower cost wins, and ties go
// to the cell with the smaller BFS distance — the scan order (x
// ascending, y ascending) breaks remaining ties toward lower x, then
// lower y, because a tie with an equal distance does not replace the
// incumbent. On success currLocalX/currLocalY are left at the approach
// point and FindRoute extracts waypoints to it. width/length arrive
// already rotated by the caller.
func (f *RouteFinder) findClosestApproachPoint(localDestX, localDestY, width, length int) bool {
	lowestCost := maxAlternativeRouteLowestCost
	maxAlternativePath := maxAlternativeRouteSeekRange
	alternativeRouteRange := maxAlternativeRouteDistanceFromDestination
	for x := localDestX - alternativeRouteRange; x <= localDestX+alternativeRouteRange; x++ {
		for y := localDestY - alternativeRouteRange; y <= localDestY+alternativeRouteRange; y++ {
			if x < 0 || y < 0 || x >= searchMapSize || y >= searchMapSize ||
				f.distanceAt(x, y) >= maxAlternativeRouteSeekRange {
				continue
			}
			var dx int
			switch {
			case x < localDestX:
				dx = localDestX - x
			case x > localDestX+width-1:
				dx = x - (width + localDestX - 1)
			default:
				dx = 0
			}
			var dy int
			switch {
			case y < localDestY:
				dy = localDestY - y
			case y > localDestY+length-1:
				dy = y - (localDestY + length - 1)
			default:
				dy = 0
			}
			cost := dx*dx + dy*dy
			if cost < lowestCost || (cost == lowestCost && maxAlternativePath > int(f.distanceAt(x, y))) {
				f.currLocalX = x
				f.currLocalY = y
				lowestCost = cost
				maxAlternativePath = int(f.distanceAt(x, y))
			}
		}
	}
	return lowestCost != maxAlternativeRouteLowestCost
}

// reached reports whether the BFS cell (localSrcX, localSrcY) counts as
// having reached the destination under the locShape exit strategy
// (RsmodRouteFinding.ts: reached). Two shortcuts precede the strategy
// dispatch: standing exactly on the destination tile reaches under every
// strategy except rectangle-exclusive, and overlapping the destination
// tile reaches under the wall strategies. The wall-strategy branch
// additionally rejects approach from a block-access side before
// consulting the wall tables.
func (f *RouteFinder) reached(
	baseX, baseY, level, localSrcX, localSrcY, localDestX, localDestY, destWidth, destLength, srcSize, locAngle, locShape, blockAccessFlags int,
) bool {
	srcX := localSrcX + baseX
	srcY := localSrcY + baseY
	destX := localDestX + baseX
	destY := localDestY + baseY
	exitStrategy := f.exitStrategy(locShape)
	if exitStrategy != rectangleExclusiveStrategy && srcX == destX && srcY == destY {
		return true
	}
	if (exitStrategy == wallStrategy || exitStrategy == wallDecoStrategy) &&
		collides(srcX, srcY, destX, destY, srcSize, srcSize, 1, 1) {
		return true
	}
	switch exitStrategy {
	case wallStrategy:
		// Route search and operation use the same rotated approach-side restrictions.
		access := rotateBlockAccess(locAngle, blockAccessFlags)
		if (srcX+srcSize <= destX && (access&blockAccessWest) != 0) ||
			(srcX > destX && (access&blockAccessEast) != 0) ||
			(srcY+srcSize <= destY && (access&blockAccessSouth) != 0) ||
			(srcY > destY && (access&blockAccessNorth) != 0) {
			return false
		}
		if srcSize == 1 {
			return f.reachWall1(level, srcX, srcY, destX, destY, locShape, locAngle)
		}
		return f.reachWallN(level, srcX, srcY, destX, destY, srcSize, locShape, locAngle)
	case wallDecoStrategy:
		if srcSize == 1 {
			return f.reachWallDeco1(level, srcX, srcY, destX, destY, locShape, locAngle)
		}
		return f.reachWallDecoN(level, srcX, srcY, destX, destY, srcSize, locShape, locAngle)
	case rectangleStrategy:
		return f.reachRectangle(level, srcX, srcY, destX, destY, srcSize, destWidth, destLength, locAngle, blockAccessFlags, false)
	case rectangleExclusiveStrategy:
		return f.reachRectangle(level, srcX, srcY, destX, destY, srcSize, destWidth, destLength, locAngle, blockAccessFlags, true)
	default:
		return false
	}
}

// reachRectangle is the rectangle exit strategy (RsmodRouteFinding.ts:
// reachRectangle): overlap with the (rotated) destination rectangle
// reaches, unless exclusive, in which case overlap explicitly does NOT
// reach and only an adjacent, un-walled side tile does. TS writes the
// overlap test as `if (exclusive ? overlaps : overlaps)` — a redundant
// ternary whose branches are identical; the Go form tests overlaps once
// and branches on exclusive inside, which is the same behavior.
func (f *RouteFinder) reachRectangle(level, srcX, srcY, destX, destY, srcSize, destWidth, destLength, locAngle, blockAccessFlags int, exclusive bool) bool {
	rotatedWidth := rotateDimensions(locAngle, destWidth, destLength)
	rotatedLength := rotateDimensions(locAngle, destLength, destWidth)
	rotatedBlockAccess := rotateBlockAccess(locAngle, blockAccessFlags)
	overlaps := collides(srcX, srcY, destX, destY, srcSize, srcSize, rotatedWidth, rotatedLength)
	if overlaps {
		if !exclusive {
			return true
		}
		return false
	}
	if srcSize == 1 {
		return f.reachRectangle1(level, srcX, srcY, destX, destY, rotatedWidth, rotatedLength, rotatedBlockAccess)
	}
	return f.reachRectangleN(level, srcX, srcY, destX, destY, srcSize, srcSize, rotatedWidth, rotatedLength, rotatedBlockAccess)
}

// reachRectangle1 is the 1x1 rectangle adjacency test
// (RsmodRouteFinding.ts: reachRectangle1): standing on a tile directly
// beside a side of the rectangle reaches, provided the wall bit facing
// the rectangle is clear on the actor's tile and the side is not
// block-accessed. Diagonal neighbours do not reach.
func (f *RouteFinder) reachRectangle1(level, srcX, srcY, destX, destY, destWidth, destLength, blockAccessFlags int) bool {
	east := destX + destWidth - 1
	north := destY + destLength - 1

	if srcX == destX-1 &&
		srcY >= destY &&
		srcY <= north &&
		(f.absoluteFlag(srcX, srcY, level)&wallEast) == 0 &&
		(blockAccessFlags&blockAccessWest) == 0 {
		return true
	}
	if srcX == east+1 &&
		srcY >= destY &&
		srcY <= north &&
		(f.absoluteFlag(srcX, srcY, level)&wallWest) == 0 &&
		(blockAccessFlags&blockAccessEast) == 0 {
		return true
	}
	if srcY+1 == destY &&
		srcX >= destX &&
		srcX <= east &&
		(f.absoluteFlag(srcX, srcY, level)&wallNorth) == 0 &&
		(blockAccessFlags&blockAccessSouth) == 0 {
		return true
	}
	return srcY == north+1 &&
		srcX >= destX &&
		srcX <= east &&
		(f.absoluteFlag(srcX, srcY, level)&wallSouth) == 0 &&
		(blockAccessFlags&blockAccessNorth) == 0
}

// reachRectangleN is the multi-tile rectangle adjacency test
// (RsmodRouteFinding.ts: reachRectangleN): the actor's footprint edge
// flush against a rectangle side reaches if any tile along the shared
// edge has its facing wall bit clear. The four sides are an if/else-if
// chain in TS — only the first matching side is tested — ported
// verbatim. Note the wall checks read the DESTINATION-side edge tiles
// (destEast-1 / destX / destNorth-1 / destY), unlike reachRectangle1
// which reads the actor's tile.
func (f *RouteFinder) reachRectangleN(level, srcX, srcY, destX, destY, srcWidth, srcLength, destWidth, destLength, blockAccessFlags int) bool {
	srcEast := srcX + srcWidth
	srcNorth := srcLength + srcY
	destEast := destWidth + destX
	destNorth := destLength + destY
	if destEast == srcX && (blockAccessFlags&blockAccessEast) == 0 {
		fromY := max(srcY, destY)
		toY := min(srcNorth, destNorth)
		for sideY := fromY; sideY < toY; sideY++ {
			if (f.absoluteFlag(destEast-1, sideY, level) & wallEast) == 0 {
				return true
			}
		}
	} else if srcEast == destX && (blockAccessFlags&blockAccessWest) == 0 {
		fromY := max(srcY, destY)
		toY := min(srcNorth, destNorth)
		for sideY := fromY; sideY < toY; sideY++ {
			if (f.absoluteFlag(destX, sideY, level) & wallWest) == 0 {
				return true
			}
		}
	} else if srcY == destNorth && (blockAccessFlags&blockAccessNorth) == 0 {
		fromX := max(srcX, destX)
		toX := min(srcEast, destEast)
		for sideX := fromX; sideX < toX; sideX++ {
			if (f.absoluteFlag(sideX, destNorth-1, level) & wallNorth) == 0 {
				return true
			}
		}
	} else if destY == srcNorth && (blockAccessFlags&blockAccessSouth) == 0 {
		fromX := max(srcX, destX)
		toX := min(srcEast, destEast)
		for sideX := fromX; sideX < toX; sideX++ {
			if (f.absoluteFlag(sideX, destY, level) & wallSouth) == 0 {
				return true
			}
		}
	}
	return false
}

// reachWall1 is the 1x1 wall reach table (RsmodRouteFinding.ts:
// reachWall1): for locShape 0 (straight wall), 2 (diagonal wall), and 9
// (wall corner), which neighbouring tiles count as reaching depends on
// locAngle, and some of those tiles additionally require the facing
// BLOCK_* mask to be clear on the actor's tile. Other shapes and angles
// fall through to false. Go's switch does not fall through, matching
// the TS inner-switch-plus-break structure.
func (f *RouteFinder) reachWall1(level, srcX, srcY, destX, destY, locShape, locAngle int) bool {
	switch locShape {
	case 0:
		switch locAngle {
		case 0:
			return (srcX == destX-1 && srcY == destY) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockSouth) == 0)
		case 1:
			return (srcX == destX && srcY == destY+1) ||
				(srcX == destX-1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockWest) == 0) ||
				(srcX == destX+1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockEast) == 0)
		case 2:
			return (srcX == destX+1 && srcY == destY) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockSouth) == 0)
		case 3:
			return (srcX == destX && srcY == destY-1) ||
				(srcX == destX-1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockWest) == 0) ||
				(srcX == destX+1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockEast) == 0)
		}
	case 2:
		switch locAngle {
		case 0:
			return (srcX == destX-1 && srcY == destY) ||
				(srcX == destX && srcY == destY+1) ||
				(srcX == destX+1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockEast) == 0) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockSouth) == 0)
		case 1:
			return (srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&blockWest) == 0) ||
				(srcX == destX && srcY == destY+1) ||
				(srcX == destX+1 && srcY == destY) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockSouth) == 0)
		case 2:
			return (srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&blockWest) == 0) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX+1 && srcY == destY) ||
				(srcX == destX && srcY == destY-1)
		case 3:
			return (srcX == destX-1 && srcY == destY) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX+1 &&
					srcY == destY &&
					(f.absoluteFlag(srcX, srcY, level)&blockEast) == 0) ||
				(srcX == destX && srcY == destY-1)
		}
	case 9:
		return (srcX == destX &&
			srcY == destY+1 &&
			(f.absoluteFlag(srcX, srcY, level)&wallSouth) == 0) ||
			(srcX == destX &&
				srcY == destY-1 &&
				(f.absoluteFlag(srcX, srcY, level)&wallNorth) == 0) ||
			(srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallEast) == 0) ||
			(srcX == destX+1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallWest) == 0)
	}
	return false
}

// reachWallN is the multi-tile wall reach table (RsmodRouteFinding.ts:
// reachWallN): the footprint-edge analogue of reachWall1. The wall
// checks read tiles on the footprint edge facing the destination
// (east/destX/srcX columns and srcY/north rows as written per case),
// not the actor's south-west corner.
func (f *RouteFinder) reachWallN(level, srcX, srcY, destX, destY, srcSize, locShape, locAngle int) bool {
	east := srcX + srcSize - 1
	north := srcY + srcSize - 1
	switch locShape {
	case 0:
		switch locAngle {
		case 0:
			return (srcX == destX-srcSize && srcY <= destY && north >= destY) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY+1 &&
					(f.absoluteFlag(destX, srcY, level)&blockNorth) == 0) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY-srcSize &&
					(f.absoluteFlag(destX, north, level)&blockSouth) == 0)
		case 1:
			return (destX >= srcX && destX <= east && srcY == destY+1) ||
				(srcX == destX-srcSize &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(east, destY, level)&blockWest) == 0) ||
				(srcX == destX+1 &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(srcX, destY, level)&blockEast) == 0)
		case 2:
			return (srcX == destX+1 && srcY <= destY && north >= destY) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY+1 &&
					(f.absoluteFlag(destX, srcY, level)&blockNorth) == 0) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY-srcSize &&
					(f.absoluteFlag(destX, north, level)&blockSouth) == 0)
		case 3:
			return (destX >= srcX && destX <= east && srcY == destY-srcSize) ||
				(srcX == destX-srcSize &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(east, destY, level)&blockWest) == 0) ||
				(srcX == destX+1 &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(srcX, destY, level)&blockEast) == 0)
		}
	case 2:
		switch locAngle {
		case 0:
			return (srcX == destX-srcSize && srcY <= destY && north >= destY) ||
				(destX >= srcX && destX <= east && srcY == destY+1) ||
				(srcX == destX+1 &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(srcX, destY, level)&blockEast) == 0) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY-srcSize &&
					(f.absoluteFlag(destX, north, level)&blockSouth) == 0)
		case 1:
			return (srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&blockWest) == 0) ||
				(destX >= srcX && destX <= east && srcY == destY+1) ||
				(srcX == destX+1 && srcY <= destY && north >= destY) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY-srcSize &&
					(f.absoluteFlag(destX, north, level)&blockSouth) == 0)
		case 2:
			return (srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&blockWest) == 0) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY+1 &&
					(f.absoluteFlag(destX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX+1 && srcY <= destY && north >= destY) ||
				(destX >= srcX && destX <= east && srcY == destY-srcSize)
		case 3:
			return (srcX == destX-srcSize && srcY <= destY && north >= destY) ||
				(destX >= srcX &&
					destX <= east &&
					srcY == destY+1 &&
					(f.absoluteFlag(destX, srcY, level)&blockNorth) == 0) ||
				(srcX == destX+1 &&
					srcY <= destY &&
					north >= destY &&
					(f.absoluteFlag(srcX, destY, level)&blockEast) == 0) ||
				(destX >= srcX && destX <= east && srcY == destY-srcSize)
		}
	case 9:
		return (destX >= srcX &&
			destX <= east &&
			srcY == destY+1 &&
			(f.absoluteFlag(destX, srcY, level)&blockNorth) == 0) ||
			(destX >= srcX &&
				destX <= east &&
				srcY == destY-srcSize &&
				(f.absoluteFlag(destX, north, level)&blockSouth) == 0) ||
			(srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&blockWest) == 0) ||
			(srcX == destX+1 &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(srcX, destY, level)&blockEast) == 0)
	}
	return false
}

// reachWallDeco1 is the 1x1 wall-decoration reach table
// (RsmodRouteFinding.ts: reachWallDeco1). Only the diagonal forms 6/7
// reach from a neighbouring tile (7 with the no-offset angle flip). The
// straight wall decorations 4/5 are operated from their own tile, as
// the client's reach strategy does: live players walk onto a wall
// lever's tile before pulling it (rsprox_transcripts/224/01444.txt,
// 233/00454.txt). Shape 8 reaches from any orthogonal neighbour with
// the facing wall bit clear.
func (f *RouteFinder) reachWallDeco1(level, srcX, srcY, destX, destY, locShape, locAngle int) bool {
	if locShape >= 6 && locShape <= 7 {
		angle := locAngle
		if locShape == wallDecorDiagonalNoOffsetShape {
			angle = (locAngle + 2) & 0x3
		}
		switch angle {
		case 0:
			return (srcX == destX+1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallWest) == 0) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&wallNorth) == 0)
		case 1:
			return (srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallEast) == 0) ||
				(srcX == destX &&
					srcY == destY-1 &&
					(f.absoluteFlag(srcX, srcY, level)&wallNorth) == 0)
		case 2:
			return (srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallEast) == 0) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&wallSouth) == 0)
		case 3:
			return (srcX == destX+1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallWest) == 0) ||
				(srcX == destX &&
					srcY == destY+1 &&
					(f.absoluteFlag(srcX, srcY, level)&wallSouth) == 0)
		}
	} else if locShape == 8 {
		return (srcX == destX &&
			srcY == destY+1 &&
			(f.absoluteFlag(srcX, srcY, level)&wallSouth) == 0) ||
			(srcX == destX &&
				srcY == destY-1 &&
				(f.absoluteFlag(srcX, srcY, level)&wallNorth) == 0) ||
			(srcX == destX-1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallEast) == 0) ||
			(srcX == destX+1 &&
				srcY == destY &&
				(f.absoluteFlag(srcX, srcY, level)&wallWest) == 0)
	}
	return false
}

// reachWallDecoN is the multi-tile wall-decoration reach table
// (RsmodRouteFinding.ts: reachWallDecoN), the footprint-edge analogue
// of reachWallDeco1 with the same shape-6/7 angle flip and shape-8
// orthogonal-neighbour rule.
func (f *RouteFinder) reachWallDecoN(level, srcX, srcY, destX, destY, srcSize, locShape, locAngle int) bool {
	east := srcX + srcSize - 1
	north := srcY + srcSize - 1
	if locShape >= 6 && locShape <= 7 {
		angle := locAngle
		if locShape == wallDecorDiagonalNoOffsetShape {
			angle = (locAngle + 2) & 0x3
		}
		switch angle {
		case 0:
			return (srcX == destX+1 &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(srcX, destY, level)&wallWest) == 0) ||
				(srcX <= destX &&
					srcY == destY-srcSize &&
					east >= destX &&
					(f.absoluteFlag(destX, north, level)&wallNorth) == 0)
		case 1:
			return (srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&wallEast) == 0) ||
				(srcX <= destX &&
					srcY == destY-srcSize &&
					east >= destX &&
					(f.absoluteFlag(destX, north, level)&wallNorth) == 0)
		case 2:
			return (srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&wallEast) == 0) ||
				(srcX <= destX &&
					srcY == destY+1 &&
					east >= destX &&
					(f.absoluteFlag(destX, srcY, level)&wallSouth) == 0)
		case 3:
			return (srcX == destX+1 &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(srcX, destY, level)&wallWest) == 0) ||
				(srcX <= destX &&
					srcY == destY+1 &&
					east >= destX &&
					(f.absoluteFlag(destX, srcY, level)&wallSouth) == 0)
		}
	} else if locShape == 8 {
		return (srcX <= destX &&
			srcY == destY+1 &&
			east >= destX &&
			(f.absoluteFlag(destX, srcY, level)&wallSouth) == 0) ||
			(srcX <= destX &&
				srcY == destY-srcSize &&
				east >= destX &&
				(f.absoluteFlag(destX, north, level)&wallNorth) == 0) ||
			(srcX == destX-srcSize &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(east, destY, level)&wallEast) == 0) ||
			(srcX == destX+1 &&
				srcY <= destY &&
				north >= destY &&
				(f.absoluteFlag(srcX, destY, level)&wallWest) == 0)
	}
	return false
}

// exitStrategy maps a locShape to its exit strategy
// (RsmodRouteFinding.ts: exitStrategy). Note the fallthrough order: -2
// is rectangle-exclusive, -1 is no-strategy, 0-3 and 9 are walls, the
// remaining shapes below 9 are wall decorations, 10-11 and 22 are
// rectangles, and anything else (including 12-21 and 23+) is
// no-strategy.
func (f *RouteFinder) exitStrategy(locShape int) int {
	if locShape == -2 {
		return rectangleExclusiveStrategy
	}
	if locShape == -1 {
		return noStrategy
	}
	if (locShape >= 0 && locShape <= 3) || locShape == 9 {
		return wallStrategy
	}
	if locShape < 9 {
		return wallDecoStrategy
	}
	if (locShape >= 10 && locShape <= 11) || locShape == 22 {
		return rectangleStrategy
	}
	return noStrategy
}

// canMove reports whether tileFlag clears blockFlag
// (RsmodRouteFinding.ts: canMove). TS coerces both operands to int32
// for `&`; the flags are int32 here already.
func canMove(tileFlag, blockFlag int32) bool {
	return (tileFlag & blockFlag) == 0
}

// absoluteFlag reads the clipping flags for one absolute tile through
// the injected lookup (RsmodRouteFinding.ts: absoluteFlag, minus the
// privateArea argument). A nil lookup answers 0 (open ground); see
// NewRouteFinder for why TS's lazy RegionManager binding is not
// reproduced.
func (f *RouteFinder) absoluteFlag(x, y, level int) int32 {
	if f.getClipping == nil {
		return 0
	}
	return f.getClipping(x, y, level)
}

// flagAt reads the clipping flags for a search-map-local tile
// (RsmodRouteFinding.ts: flagAt).
func (f *RouteFinder) flagAt(baseX, baseY, localX, localY, level int) int32 {
	return f.absoluteFlag(baseX+localX, baseY+localY, level)
}

// appendDirection records a visited cell and enqueues it on the ring
// buffer (RsmodRouteFinding.ts: appendDirection). The writer index wraps
// with & (RING_BUFFER_SIZE - 1); the buffer holds 4096 entries and the
// BFS can enqueue more cells than that on open ground, so older entries
// are overwritten once the ring wraps — TS behaves identically, and the
// directions array (not the ring) is the visited set, so a dropped
// queue entry only prunes that branch.
func (f *RouteFinder) appendDirection(x, y int, direction int32, distance int32) {
	index := f.index(x, y)
	f.directions[index] = direction
	f.distances[index] = distance
	f.validLocalX[f.bufWriterIndex] = int32(x)
	f.validLocalY[f.bufWriterIndex] = int32(y)
	f.bufWriterIndex = (f.bufWriterIndex + 1) & (ringBufferSize - 1)
}

// reset clears the BFS state for a new search (RsmodRouteFinding.ts:
// reset): directions to 0 (unvisited), distances to the sentinel, ring
// indices to 0. The ring arrays themselves are not cleared — the
// indices make stale entries unreachable, as in TS.
func (f *RouteFinder) reset() {
	for i := range f.directions {
		f.directions[i] = 0
		f.distances[i] = distanceSentinel
	}
	f.bufReaderIndex = 0
	f.bufWriterIndex = 0
}

// index maps a search-map-local tile to its array slot
// (RsmodRouteFinding.ts: index): x-major, x * SEARCH_MAP_SIZE + y.
func (f *RouteFinder) index(x, y int) int {
	return x*searchMapSize + y
}

// directionAt reads the stored reverse direction for a local tile
// (RsmodRouteFinding.ts: directionAt).
func (f *RouteFinder) directionAt(x, y int) int32 {
	return f.directions[f.index(x, y)]
}

// distanceAt reads the stored BFS distance for a local tile
// (RsmodRouteFinding.ts: distanceAt).
func (f *RouteFinder) distanceAt(x, y int) int32 {
	return f.distances[f.index(x, y)]
}
