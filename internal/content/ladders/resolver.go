// The destination resolver: the Go port of
// server/plugins/objects/ClimbLinks.js (upstream HEAD bfd81e3).
//
// The resolver is a pure function of narrow interfaces (Defs,
// MapObjects, BlockedFunc, ReachFunc — the doors CatalogDefs
// precedent), so it is unit-testable with fixture defs/objects.
//
// Grounding answer (first grounding question): runtime map-object
// query DOES exist in Go — no startup cache scan is needed.
// ObjectRegistry.ObjectsAtTile (internal/game/objects.go:1314)
// syncs the tile's region on demand (syncRegion: the
// RegionManager loads the region and its Placements are imported
// as map-original GameObjects), so placed map objects on any
// plane are queryable at runtime exactly as upstream's
// MapObjects.getHash lookup is. The resolver reads through the
// MapObjects interface; production wiring adapts it over the
// ObjectRegistry (see RegistryObjects in mechanism.go).
package ladders

import (
	"math"
	"sort"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Direction of a climb: Up is +1 plane, Down is -1.
// ClimbLinks.js: UP = 1, DOWN = -1.
const (
	Up   = 1
	Down = -1
)

// UndergroundOffset is how far north underground areas lie from the
// surface above them. ClimbLinks.js: UNDERGROUND_OFFSET = 6400.
const UndergroundOffset = 6400

// SearchRadius is how far around the clicked object the other end
// may lie. ClimbLinks.js: SEARCH_RADIUS = 4 (the same-column search
// passes SEARCH_RADIUS + 1).
const SearchRadius = 4

// GangplankRadius is how far from a gangplank its other half lies.
// ClimbLinks.js: GANGPLANK_RADIUS = 2.
const GangplankRadius = 2

// MaxPlane is the top plane index. ClimbLinks.js: MAX_PLANE = 3
// (also the toEnd loop bound).
const MaxPlane = 3

// upOptions / downOptions are the climb options per direction,
// lower-cased for case-insensitive matching. ClimbLinks.js:
// UP_OPTIONS / DOWN_OPTIONS (which also carry "climb",
// "top-floor" / "bottom-floor"; Ladders.plugin.js:40-50 spells the
// same sets without those two).
var climbUpOptions = map[string]bool{
	"climb-up": true, "climb": true, "top-floor": true,
	"climb up": true, "walk-up": true, "ascend": true,
}

var climbDownOptions = map[string]bool{
	"climb-down": true, "climb": true, "bottom-floor": true,
	"climb down": true, "walk-down": true, "descend": true,
}

// ObjectDef is the loc-definition surface the resolver needs: the
// Go form of the ObjectDefinition accessors ClimbLinks reads
// (getName, getInteractions, getSizeX/getSizeY, getBlockingMask).
type ObjectDef struct {
	Name string
	// Actions are the menu interactions, "" for absent slots.
	Actions []string
	SizeX   int
	SizeY   int
	// ClipMask is the blocking mask (getBlockingMask, the cache
	// clipMask).
	ClipMask int
}

// Defs resolves an object id to its ObjectDef, or ok=false when no
// definition exists. Production wiring adapts it over the cache
// definitions (the doors cacheCatalogDefs precedent — the adapter
// lives in cmd/server, not here, so this package never imports
// internal/cache).
type Defs interface {
	Def(id int) (ObjectDef, bool)
}

// MapObjects answers the placed objects on one tile: the Go form
// of MapObjects.getHash lookups (ClimbLinks.js objectsAround).
// Production wiring adapts it over *game.ObjectRegistry
// (ObjectsAtTile, which loads the region on demand).
type MapObjects interface {
	ObjectsAtTile(loc game.Location) []plugins.GameObject
}

// BlockedFunc answers whether a tile is blocked for walking in
// the given private area (nil area = the shared world): the Go
// form of RegionManager.blocked (RegionManager.ts:547-549).
// Production wiring adapts it over RegionManager.BlockedInArea.
// A nil BlockedFunc reads every tile as open.
type BlockedFunc func(loc game.Location, area *game.PrivateArea) bool

// ReachFunc answers whether a size-1 entity standing on `from`
// has reached the object footprint (destX/destY, destWidth/
// destLength unturned sizes, locAngle face, locShape type,
// blockAccessFlags blocking mask): the Go form of
// PathFinder.reachedObject (PathFinder.ts:229-253). Production
// wiring adapts it over a RouteFinder built from the
// RegionManager's area lookup (see RegionReached in
// mechanism.go). A nil ReachFunc reads every candidate as
// reachable.
type ReachFunc func(from game.Location, area *game.PrivateArea, destX, destY, destWidth, destLength, locAngle, locShape, blockAccessFlags int) bool

// Resolver resolves where a climb leads, from the map itself
// (ClimbLinks.js). Defs and Objects are required; Blocked and
// Reached default to open/reachable when nil.
type Resolver struct {
	Defs    Defs
	Objects MapObjects
	Blocked BlockedFunc
	Reached ReachFunc
}

func (r *Resolver) def(id int) (ObjectDef, bool) {
	if r == nil || r.Defs == nil {
		return ObjectDef{}, false
	}
	return r.Defs.Def(id)
}

// Name returns the definition name for id, or "" when unknown.
func (r *Resolver) Name(id int) string {
	def, ok := r.def(id)
	if !ok {
		return ""
	}
	return def.Name
}

// Options returns the lower-cased non-empty interactions of id.
// ClimbLinks.js: options().
func (r *Resolver) Options(id int) []string {
	def, ok := r.def(id)
	if !ok {
		return nil
	}
	var out []string
	for _, a := range def.Actions {
		if a = strings.ToLower(strings.TrimSpace(a)); a != "" {
			out = append(out, a)
		}
	}
	return out
}

// Climbs reports whether id has a climb option in direction.
// ClimbLinks.js: climbs().
func (r *Resolver) Climbs(id, direction int) bool {
	wanted := climbDownOptions
	if direction == Up {
		wanted = climbUpOptions
	}
	for _, option := range r.Options(id) {
		if wanted[option] {
			return true
		}
	}
	return false
}

// HasCross reports whether id offers the gangplank "Cross"
// option. ClimbLinks.js: crossDestination's options check.
func (r *Resolver) HasCross(id int) bool {
	for _, option := range r.Options(id) {
		if option == "cross" {
			return true
		}
	}
	return false
}

// Footprint returns the tiles an object covers: its size, turned
// with its rotation. ClimbLinks.js: footprint().
func (r *Resolver) Footprint(obj plugins.GameObject) (x, y, width, length int) {
	if obj == nil {
		return 0, 0, 1, 1
	}
	loc := obj.Location()
	def, _ := r.def(obj.ID())
	sizeX, sizeY := def.SizeX, def.SizeY
	if sizeX < 1 {
		sizeX = 1
	}
	if sizeY < 1 {
		sizeY = 1
	}
	width, length = sizeX, sizeY
	if (obj.Face() & 1) == 1 {
		width, length = sizeY, sizeX
	}
	return loc.X, loc.Y, max(width, 1), max(length, 1)
}

// DistanceToFootprint returns the Chebyshev distance from (x, y)
// to the nearest tile of obj's footprint. ClimbLinks.js:
// distanceToFootprint(). A different plane reads MaxInt, the Go
// form of TS Number.POSITIVE_INFINITY (GameObject.ts:137-149,
// via FootprintDistance).
func (r *Resolver) DistanceToFootprint(x, y int, obj plugins.GameObject) int {
	if obj == nil {
		return math.MaxInt
	}
	fx, fy, width, length := r.Footprint(obj)
	return distanceToRect(x, y, fx, fy, width, length)
}

func distanceToRect(x, y, rx, ry, width, length int) int {
	dx := max(rx-x, 0, x-(rx+width-1))
	dy := max(ry-y, 0, y-(ry+length-1))
	return max(dx, dy)
}

type objectKey struct {
	id  int
	loc game.Location
}

func (r *Resolver) objectsAt(loc game.Location) []plugins.GameObject {
	if r == nil || r.Objects == nil {
		return nil
	}
	return r.Objects.ObjectsAtTile(loc)
}

// objectsAround returns the placed objects in the square of
// radius around (x, y) on plane z, deduplicated by id+tile.
// ClimbLinks.js: objectsAround(). An out-of-range plane reads
// empty, as TS returns [] for z < 0 or z > MAX_PLANE.
func (r *Resolver) objectsAround(x, y, z, radius int) []plugins.GameObject {
	if z < 0 || z > MaxPlane {
		return nil
	}
	seen := map[objectKey]bool{}
	var out []plugins.GameObject
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			for _, obj := range r.objectsAt(game.Location{X: x + dx, Y: y + dy, Z: z}) {
				key := objectKey{id: obj.ID(), loc: obj.Location()}
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, obj)
			}
		}
	}
	return out
}

// objectsOnFootprint returns the objects covering the same
// footprint as obj on plane z (not a broad search).
// ClimbLinks.js: objectsOnFootprint().
func (r *Resolver) objectsOnFootprint(obj plugins.GameObject, z int) []plugins.GameObject {
	if obj == nil {
		return nil
	}
	fx, fy, width, length := r.Footprint(obj)
	seen := map[objectKey]bool{}
	var out []plugins.GameObject
	for x := fx; x < fx+width; x++ {
		for y := fy; y < fy+length; y++ {
			for _, candidate := range r.objectsAround(x, y, z, 0) {
				key := objectKey{id: candidate.ID(), loc: candidate.Location()}
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, candidate)
			}
		}
	}
	return out
}

// nearest returns the candidate climbing back in -direction,
// nearest by footprint distance from (x, y); ties keep the
// first candidate, matching TS's stable sort of the
// objectsAround order. ClimbLinks.js: otherEnd's nearest().
func (r *Resolver) nearest(candidates []plugins.GameObject, direction, x, y int) plugins.GameObject {
	var best plugins.GameObject
	bestDist := math.MaxInt
	for _, candidate := range candidates {
		if !r.Climbs(candidate.ID(), -direction) {
			continue
		}
		if d := r.DistanceToFootprint(x, y, candidate); d < bestDist {
			best, bestDist = candidate, d
		}
	}
	return best
}

// inertSameColumnSegment returns a decorative/inert segment of
// the same climb sitting exactly in obj's column on plane z: a
// same-name, non-climbable object. Only such an object unlocks
// skipping that plane. ClimbLinks.js: inertSameColumnSegment().
func (r *Resolver) inertSameColumnSegment(obj plugins.GameObject, z int) plugins.GameObject {
	if obj == nil {
		return nil
	}
	name := r.Name(obj.ID())
	if name == "" {
		return nil
	}
	for _, candidate := range r.objectsOnFootprint(obj, z) {
		if r.Name(candidate.ID()) == name &&
			!r.Climbs(candidate.ID(), Up) && !r.Climbs(candidate.ID(), Down) {
			return candidate
		}
	}
	return nil
}

// OtherEnd returns the object at the other end of obj going
// direction: one that climbs back the other way, on the next
// plane around the object, else underground (or back up from
// it). ClimbLinks.js: otherEnd().
func (r *Resolver) OtherEnd(obj plugins.GameObject, direction int) plugins.GameObject {
	if obj == nil {
		return nil
	}
	from := obj.Location()
	if entry, ok := declaredLinkFor(obj.ID(), from); ok {
		if entry.direction != direction {
			return nil
		}
		for _, candidate := range r.objectsAround(entry.otherX, entry.otherY, entry.otherZ, 0) {
			if candidate.ID() == entry.otherID {
				return candidate
			}
		}
		return nil
	}
	fx, fy, width, length := r.Footprint(obj)
	centreX := fx + (width >> 1)
	centreY := fy + (length >> 1)
	sameColumn := func() plugins.GameObject {
		z := from.Z + direction
		if z < 0 || z > MaxPlane {
			return nil
		}
		if first := r.nearest(r.objectsAround(centreX, centreY, z, SearchRadius+1), direction, centreX, centreY); first != nil {
			return first
		}
		if r.inertSameColumnSegment(obj, z) == nil {
			return nil
		}
		// Follow a chain of same-column, same-name inert
		// segments (bounded by the planes).
		for {
			z += direction
			if z < 0 || z > MaxPlane {
				return nil
			}
			if end := r.nearest(r.objectsOnFootprint(obj, z), direction, centreX, centreY); end != nil {
				return end
			}
			if r.inertSameColumnSegment(obj, z) == nil {
				return nil
			}
		}
	}
	underground := func(y int) plugins.GameObject {
		return r.nearest(r.objectsAround(centreX, y, 0, SearchRadius), direction, centreX, y)
	}
	// Going up from underground, the surface comes first; going
	// down from the ground floor, there is no plane below, only
	// the underground.
	if direction == Up && from.Y >= UndergroundOffset {
		if end := underground(centreY - UndergroundOffset); end != nil {
			return end
		}
		return sameColumn()
	}
	if direction == Down && from.Z == 0 {
		if from.Y < UndergroundOffset {
			return underground(centreY + UndergroundOffset)
		}
		return nil
	}
	return sameColumn()
}

func (r *Resolver) blocked(loc game.Location, area *game.PrivateArea) bool {
	if r == nil || r.Blocked == nil {
		return false
	}
	return r.Blocked(loc, area)
}

func (r *Resolver) reached(from game.Location, area *game.PrivateArea, obj plugins.GameObject, def ObjectDef) bool {
	if r == nil || r.Reached == nil {
		return true
	}
	return r.Reached(from, area, obj.Location().X, obj.Location().Y,
		def.SizeX, def.SizeY, obj.Face(), obj.Type(), def.ClipMask)
}

// LandingTile returns the walkable tiles next to obj from which it
// can be used, nearest to hint first. ClimbLinks.js: landingTile().
// The reach check passes the definition's unturned sizes, face,
// type and blocking mask, exactly as TS passes
// def.getSizeX()/getSizeY(), object.getFace(), object.getType(),
// def.getBlockingMask() to PathFinder.reachedObject; the
// candidate ring itself uses the turned footprint.
func (r *Resolver) LandingTile(obj plugins.GameObject, hint game.Location, area *game.PrivateArea) *game.Location {
	if obj == nil {
		return nil
	}
	def, _ := r.def(obj.ID())
	fx, fy, width, length := r.Footprint(obj)
	z := obj.Location().Z
	var candidates []game.Location
	for x := fx - 1; x <= fx+width; x++ {
		for y := fy - 1; y <= fy+length; y++ {
			if distanceToRect(x, y, fx, fy, width, length) != 1 {
				continue
			}
			tile := game.Location{X: x, Y: y, Z: z}
			if r.blocked(tile, area) {
				continue
			}
			if !r.reached(tile, area, obj, def) {
				continue
			}
			candidates = append(candidates, tile)
		}
	}
	away := func(tile game.Location) int {
		return abs(tile.X-hint.X) + abs(tile.Y-hint.Y)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return away(candidates[i]) < away(candidates[j])
	})
	if len(candidates) == 0 {
		return nil
	}
	landing := candidates[0]
	return &landing
}

// Destination returns where climbing obj in direction puts a player
// standing on `from`, or nil when the map has nothing at the other
// end. ClimbLinks.js: destination(). A declared landing wins over
// the computed landing tile; toEnd keeps going while the next end
// climbs on in the same direction (max 3 more floors).
func (r *Resolver) Destination(obj plugins.GameObject, direction int, from game.Location, area *game.PrivateArea, toEnd bool) *game.Location {
	if obj == nil || !r.Climbs(obj.ID(), direction) {
		return nil
	}
	loc := obj.Location()
	if entry, ok := declaredLinkFor(obj.ID(), loc); ok && entry.direction == direction && entry.landing != nil {
		landing := *entry.landing
		return &landing
	}
	end := r.OtherEnd(obj, direction)
	if end == nil {
		return nil
	}
	for floors := 0; toEnd && floors < MaxPlane && r.Climbs(end.ID(), direction); floors++ {
		next := r.OtherEnd(end, direction)
		if next == nil {
			break
		}
		end = next
	}
	shift := end.Location().Y - loc.Y
	hint := game.Location{X: from.X, Y: from.Y}
	if abs(shift) >= UndergroundOffset/2 {
		hint.Y += shift
	}
	return r.LandingTile(end, hint, area)
}

// CrossDestination returns where crossing obj (a gangplank
// offering "Cross") puts the player: the landing carries on 3
// tiles past the other half in the direction of travel.
// ClimbLinks.js: crossDestination().
func (r *Resolver) CrossDestination(obj plugins.GameObject, area *game.PrivateArea) *game.Location {
	if obj == nil || !r.HasCross(obj.ID()) {
		return nil
	}
	from := obj.Location()
	plane := 1
	if from.Z != 0 {
		plane = from.Z - 1
	}
	name := r.Name(obj.ID())
	if name == "" {
		return nil
	}
	var best plugins.GameObject
	bestDist := math.MaxInt
	for _, candidate := range r.objectsAround(from.X, from.Y, plane, GangplankRadius) {
		if r.Name(candidate.ID()) != name || !r.HasCross(candidate.ID()) {
			continue
		}
		if d := r.DistanceToFootprint(from.X, from.Y, candidate); d < bestDist {
			best, bestDist = candidate, d
		}
	}
	if best == nil {
		return nil
	}
	end := best.Location()
	stepX := sign(end.X - from.X)
	stepY := sign(end.Y - from.Y)
	hint := game.Location{X: end.X + stepX*3, Y: end.Y + stepY*3, Z: end.Z}
	return r.LandingTile(best, hint, area)
}

// HasObjectOnFloor reports whether an object named name sits on
// the plane delta away, within the 3x3 tiles around location.
// Ladders.plugin.js: hasObjectOnFloor(). A climb never lands on
// the ladder's own (blocked) tile; the last resort is the
// player's own tile one plane up/down, when an object of the same
// name is right above/below.
func (r *Resolver) HasObjectOnFloor(loc game.Location, name string, delta int) bool {
	if name == "" {
		return false
	}
	z := loc.Z + delta
	if z < 0 || z > MaxPlane {
		return false
	}
	for dx := -1; dx <= 1; dx++ {
		for dy := -1; dy <= 1; dy++ {
			for _, obj := range r.objectsAt(game.Location{X: loc.X + dx, Y: loc.Y + dy, Z: z}) {
				if r.Name(obj.ID()) == name {
					return true
				}
			}
		}
	}
	return false
}

// HasObjectAbove reports whether an object named name sits on the
// plane above location. Ladders.plugin.js: hasObjectAbove().
func (r *Resolver) HasObjectAbove(loc game.Location, name string) bool {
	return r.HasObjectOnFloor(loc, name, 1)
}

// HasObjectBelow reports whether an object named name sits on the
// plane below location. Ladders.plugin.js: hasObjectBelow().
func (r *Resolver) HasObjectBelow(loc game.Location, name string) bool {
	return r.HasObjectOnFloor(loc, name, -1)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func sign(n int) int {
	switch {
	case n > 0:
		return 1
	case n < 0:
		return -1
	default:
		return 0
	}
}
