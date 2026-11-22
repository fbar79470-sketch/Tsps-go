package ladders

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ClimbMechanism is the climb mechanism behind Worker B's Climber seam
// (ladders.go): destination resolution plus the climb task. It is
// built by wiring (cmd/server, after doors) from a Resolver and
// the world's TaskManager, then installed with SetMechanism.
type ClimbMechanism struct {
	resolver  *Resolver
	scheduler Scheduler
}

// NewMechanism builds the climb mechanism. A nil resolver means
// no climb ever resolves (every climb falls through, as upstream
// does where the map has nothing at the other end); a nil
// scheduler means no climb ever starts.
func NewMechanism(resolver *Resolver, scheduler Scheduler) *ClimbMechanism {
	return &ClimbMechanism{resolver: resolver, scheduler: scheduler}
}

// Resolver returns the mechanism's destination resolver.
func (m *ClimbMechanism) Resolver() *Resolver {
	if m == nil {
		return nil
	}
	return m.resolver
}

// Climb runs one climb through the mechanism (Climber.Climb,
// Ladders.plugin.js: climbUp / climbDown):
//
//   - An explicit req.Destination is trusted verbatim.
//   - Else the map decides (ClimbLinks.destination via the
//     resolver, toEnd for Top-floor / Bottom-floor).
//   - Else the last resort is the player's own tile one plane
//     up/down — but only if an object of the same name sits right
//     above/below (hasObjectAbove / hasObjectBelow). A climb never
//     lands on the ladder's own (blocked) tile. Out of plane range
//     (z < 0 or z > 3) there is no climb.
//
// The animation is the climb animation for the direction, or none
// when req.Animate is false (walked stairs).
func (m *ClimbMechanism) Climb(req ClimbRequest) bool {
	if m == nil || req.Player == nil || m.resolver == nil {
		return false
	}
	dest := req.Destination
	if dest == nil {
		dest = m.resolve(req)
	}
	if dest == nil {
		return false
	}
	animation := NoAnimation
	if req.Animate {
		animation = ClimbDownAnimation
		if req.Delta >= 0 {
			animation = ClimbUpAnimation
		}
	}
	_, started := StartClimb(m.scheduler, PlayerClimber(req.Player), *dest, animation, req.OnArrival)
	return started
}

// CrossGangplank runs a gangplank "Cross" (Climber.CrossGangplank,
// Ladders.plugin.js: crossOption): dock to ship deck, no climb
// animation. The destination comes from the map only — there is
// no explicit-destination form.
func (m *ClimbMechanism) CrossGangplank(req ClimbRequest) bool {
	if m == nil || req.Player == nil || m.resolver == nil || req.Object == nil {
		return false
	}
	dest := m.resolver.CrossDestination(req.Object, req.Player.PrivateArea())
	if dest == nil {
		return false
	}
	_, started := StartClimb(m.scheduler, PlayerClimber(req.Player), *dest, NoAnimation, req.OnArrival)
	return started
}

// resolve finds the map destination for req, then the
// hasObjectAbove/hasObjectBelow fallback. Ladders.plugin.js:
// climbUp / climbDown + mapDestination + resolveDestination.
func (m *ClimbMechanism) resolve(req ClimbRequest) *game.Location {
	if req.Object == nil || req.Delta == 0 {
		return nil
	}
	player := req.Player
	from := player.Location()
	area := player.PrivateArea()
	if mapped := m.resolver.Destination(req.Object, req.Delta, from, area, req.ToEnd); mapped != nil {
		return mapped
	}
	base := req.Location
	if req.SourceLocation != nil {
		base = *req.SourceLocation
	}
	z := base.Z + req.Delta
	if z < 0 || z > MaxPlane {
		return nil
	}
	name := ""
	if req.Definition != nil {
		name = req.Definition.Name()
	}
	if name == "" {
		name = m.resolver.Name(req.Object.ID())
	}
	if name == "" {
		return nil
	}
	above := req.Delta > 0
	if (above && !m.resolver.HasObjectAbove(req.Location, name)) ||
		(!above && !m.resolver.HasObjectBelow(req.Location, name)) {
		return nil
	}
	return &game.Location{X: base.X, Y: base.Y, Z: z}
}

// --- Production adapters (no internal/cache import here; the Defs
// adapter lives in cmd/server on the doors cacheCatalogDefs
// precedent) ---

// registryObjects adapts *game.ObjectRegistry to MapObjects.
type registryObjects struct {
	registry *game.ObjectRegistry
}

// RegistryObjects adapts an ObjectRegistry to the resolver's
// MapObjects surface. A nil registry reads no objects.
func RegistryObjects(registry *game.ObjectRegistry) MapObjects {
	return &registryObjects{registry: registry}
}

func (a *registryObjects) ObjectsAtTile(loc game.Location) []plugins.GameObject {
	if a == nil || a.registry == nil {
		return nil
	}
	placed := a.registry.ObjectsAtTile(loc)
	out := make([]plugins.GameObject, 0, len(placed))
	for _, obj := range placed {
		out = append(out, obj)
	}
	return out
}

// RegionBlocked adapts a RegionManager to the resolver's
// BlockedFunc: RegionManager.BlockedInArea, the Go form of
// RegionManager.blocked (RegionManager.ts:547-549). A nil manager
// reads every tile as open.
func RegionBlocked(rm *game.RegionManager) BlockedFunc {
	return func(loc game.Location, area *game.PrivateArea) bool {
		if rm == nil {
			return false
		}
		return rm.BlockedInArea(loc, area)
	}
}

// RegionReached adapts a RegionManager to the resolver's
// ReachFunc: a RouteFinder built from the manager's area lookup,
// the Go form of PathFinder.reachedObject (PathFinder.ts:229-253
// via MovementQueue.ReachedObject). A nil manager reads every
// candidate as reachable. The finder is built per call —
// NewRouteFinder only stores the lookup — so the area of the
// climb in progress is always the one consulted.
func RegionReached(rm *game.RegionManager) ReachFunc {
	return func(from game.Location, area *game.PrivateArea, destX, destY, destWidth, destLength, locAngle, locShape, blockAccessFlags int) bool {
		if rm == nil {
			return true
		}
		finder := game.NewRouteFinder(rm.LookupForArea(area))
		return finder.ReachedAbsolute(game.ReachOptions{
			Level:            from.Z,
			SrcX:             from.X,
			SrcY:             from.Y,
			SrcSize:          1,
			DestX:            destX,
			DestY:            destY,
			DestWidth:        destWidth,
			DestLength:       destLength,
			LocAngle:         locAngle,
			LocShape:         locShape,
			BlockAccessFlags: blockAccessFlags,
		})
	}
}
