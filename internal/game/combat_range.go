package game

// CombatRange and CombatPosition: the port of
// game/content/combat/CombatRange.ts and CombatPosition.ts —
// footprint-aware reach, routing, and line of sight.
//
// DEGENERATE BRANCHES. CombatPosition's deck logic (sailing)
// is not ported: location() is the fighter's own tile and
// area() is always "the same area" (both nil), so the area
// equality check in CanReach always passes. CrossFloor is not
// ported: any floor difference fails reach (TS allows
// non-melee cross-floor attacks where CrossFloor.canCross
// holds; Go has no such concept). Private areas are not
// ported: clipping reads through the combat clipping seam
// with no area parameter.

// combatBounds is the Bounds shape (CombatRange.ts): an
// inclusive footprint rectangle. Footprints extend north and
// east from the south-west tile only.
type combatBounds struct {
	minX, minY, maxX, maxY int
}

// boundsOf returns the footprint bounds of a fighter at loc
// (CombatRange.bounds): size floors at 1.
func boundsOf(loc Location, size int) combatBounds {
	if size < 1 {
		size = 1
	}
	return combatBounds{minX: loc.X, minY: loc.Y, maxX: loc.X + size - 1, maxY: loc.Y + size - 1}
}

// boundsDistance is the bounds gap (CombatRange.distance):
// the Chebyshev gap between two footprints, 0 when they
// overlap or touch diagonally... precisely: max(0, the four
// edge gaps). Orthogonally adjacent footprints read 1;
// overlapping footprints read 0.
func boundsDistance(a, b combatBounds) int {
	d := 0
	if v := b.minX - a.maxX; v > d {
		d = v
	}
	if v := a.minX - b.maxX; v > d {
		d = v
	}
	if v := b.minY - a.maxY; v > d {
		d = v
	}
	if v := a.minY - b.maxY; v > d {
		d = v
	}
	return d
}

// boundsOverlap reports whether two footprints share a tile
// (CombatRange.overlaps).
func boundsOverlap(a, b combatBounds) bool {
	return a.minX <= b.maxX && b.minX <= a.maxX && a.minY <= b.maxY && b.minY <= a.maxY
}

// boundsShareEdge reports whether two footprints touch along
// an edge (CombatRange.sharesEdge): orthogonally adjacent
// with overlapping extent on the shared axis. Diagonally
// adjacent footprints do not share an edge.
func boundsShareEdge(a, b combatBounds) bool {
	besideX := (a.maxX+1 == b.minX || b.maxX+1 == a.minX) && a.minY <= b.maxY && b.minY <= a.maxY
	besideY := (a.maxY+1 == b.minY || b.maxY+1 == a.minY) && a.minX <= b.maxX && b.minX <= a.maxX
	return besideX || besideY
}

// CombatPositionLocation is CombatPosition.location, degenerated:
// the fighter's own tile (no decks in Go).
func CombatPositionLocation(m *Mobile) Location {
	if m == nil {
		return Location{}
	}
	return m.Location()
}

// CombatPositionDistance is CombatPosition.distance: the
// Chebyshev distance between two fighters' tiles (-1 across
// floors, the Location.Distance convention).
func CombatPositionDistance(a, b *Mobile) int {
	if a == nil || b == nil {
		return -1
	}
	return a.Location().Distance(b.Location())
}

// EdgeDistance is CombatPosition.edgeDistance: the Chebyshev
// distance from a's tile to the nearest tile of b's footprint
// (0 inside the footprint; callers apply their own minimum).
// Across floors it reads -1 (Go convention; TS has no
// cross-floor edge distance outside CrossFloor).
func EdgeDistance(a, b *Mobile) int {
	if a == nil || b == nil {
		return -1
	}
	from := a.Location()
	to := b.Location()
	if from.Z != to.Z {
		return -1
	}
	size := combatantFor(b).Size()
	if size < 1 {
		size = 1
	}
	dx := 0
	if v := to.X - from.X; v > dx {
		dx = v
	}
	if v := from.X - (to.X + size - 1); v > dx {
		dx = v
	}
	dy := 0
	if v := to.Y - from.Y; v > dy {
		dy = v
	}
	if v := from.Y - (to.Y + size - 1); v > dy {
		dy = v
	}
	if dx > dy {
		return dx
	}
	return dy
}

// FootprintDistance is CombatRange.footprintDistance: the
// Chebyshev distance from point to an NPC footprint whose
// south-west tile is origin. The footprint extends size-1
// tiles north and east only. Across floors it reads a large
// value (TS Number.MAX_SAFE_INTEGER) unless allowCrossFloor.
func FootprintDistance(origin Location, size int, point Location, allowCrossFloor bool) int {
	if origin.Z != point.Z && !allowCrossFloor {
		return 1 << 60
	}
	extent := size - 1
	if extent < 0 {
		extent = 0
	}
	dx := 0
	if v := origin.X - point.X; v > dx {
		dx = v
	}
	if v := point.X - (origin.X + extent); v > dx {
		dx = v
	}
	dy := 0
	if v := origin.Y - point.Y; v > dy {
		dy = v
	}
	if v := point.Y - (origin.Y + extent); v > dy {
		dy = v
	}
	if dx > dy {
		return dx
	}
	return dy
}

// CanReach is CombatRange.canReach: whether attacker can hit
// target with method right now. Different floors always fail
// (no CrossFloor). Overlapping footprints on the same floor
// fail ("the tile underneath the NPC itself is not part of
// the attack range"). Beyond the method's attack distance
// fails. Built-into-scenery targets skip clipping (melee
// range 1 still needs a shared edge). Melee range 1 needs an
// open shared edge; everything else needs a projectile line
// (reversed for NPC attackers; PvP checks both directions).
func CanReach(attacker Combatant, method CombatMethod, target Combatant) bool {
	if attacker == nil || target == nil || attacker.Mobile() == nil || target.Mobile() == nil || method == nil {
		return false
	}
	attackerLoc := attacker.Mobile().Location()
	targetLoc := target.Mobile().Location()
	if attackerLoc.Z != targetLoc.Z {
		return false
	}
	attackerBounds := boundsOf(attackerLoc, attacker.Size())
	targetBounds := boundsOf(targetLoc, target.Size())
	attackRange := AttackDistance(attacker, method, target)
	if boundsOverlap(attackerBounds, targetBounds) || boundsDistance(attackerBounds, targetBounds) > attackRange {
		return false
	}
	if HasCombatFlag(target.Mobile(), CombatFlagBuiltIntoScenery) {
		return method.Type() != CombatTypeMelee || attackRange != 1 || boundsShareEdge(attackerBounds, targetBounds)
	}
	if method.Type() == CombatTypeMelee && attackRange == 1 {
		return hasOpenSharedEdge(attackerBounds, targetBounds, attackerLoc.Z)
	}
	// Extended melee and ranged/magic use the projectile line.
	if !attacker.IsPlayer() {
		return hasProjectileLine(targetBounds, attackerBounds, attackerLoc.Z)
	}
	forward := hasProjectileLine(attackerBounds, targetBounds, attackerLoc.Z)
	return forward && (!target.IsPlayer() || hasProjectileLine(targetBounds, attackerBounds, attackerLoc.Z))
}

// AttackDistance is CombatRange.attackDistance: the method's
// reach as the combat:attack-distance hook adjusts it,
// floored at 1. manualCast is true for a magic manual cast
// (a cast spell that is not the autocast spell).
func AttackDistance(attacker Combatant, method CombatMethod, target Combatant) int {
	if attacker == nil || method == nil {
		return 1
	}
	distance := method.AttackDistance(attacker.Mobile())
	if distance < 1 {
		distance = 1
	}
	if Hooks.OnCombatAttackDistance == nil {
		return distance
	}
	manualCast := false
	if method.Type() == CombatTypeMagic && attacker.IsPlayer() {
		combat := attacker.Mobile().GetCombat()
		cast := combat.CastSpell()
		manualCast = cast != nil && cast != combat.AutoCastSpell()
	}
	var targetMobile *Mobile
	if target != nil {
		targetMobile = target.Mobile()
	}
	e := &CombatAttackDistanceEvent{
		Attacker:   attacker.Mobile(),
		Target:     targetMobile,
		CombatType: method.Type(),
		ManualCast: manualCast,
		Distance:   distance,
	}
	Hooks.OnCombatAttackDistance(e)
	if e.Distance < 1 {
		e.Distance = 1
	}
	return e.Distance
}

// CombatRoute is CombatRange.route (same-floor half): route
// the attacker toward the target entity and report whether a
// route was found. The cross-floor clear-shot branch is not
// ported (no CrossFloor). Routing goes through the queue's
// applyWalkRoute (the queue-mutation half of
// PathFinder.applyRsmodRoute, moveNear): the post-movement
// phase stops the queue as soon as CanReach succeeds, so no
// separate attack-tile heuristic is needed. A nil queue
// answers false.
// CombatRoute routes attacker toward a tile from which it can
// attack target (CombatRange.route): TS calculateEntityRoute
// is shape-aware and stops beside the target footprint.
// Routing to the target's own tile would walk onto it — NPC
// occupancy is not in the clipping data — so the destination
// is the nearest tile at attack distance with an open
// approach (see combatRouteDestination).
func CombatRoute(attacker Combatant, target Combatant, method CombatMethod) bool {
	if attacker == nil || target == nil || attacker.Mobile() == nil || target.Mobile() == nil {
		return false
	}
	queue := attacker.MovementQueue()
	if queue == nil {
		return false
	}
	dest := combatRouteDestination(attacker, target, AttackDistance(attacker, method, target))
	queue.applyWalkRoute(dest.X, dest.Y)
	return queue.HasRoute()
}

// combatRouteDestination picks the route destination for a
// fighter out of reach. At attack distance 1 (melee) it is
// the nearest placement sharing an open edge with the target
// footprint (placements are enumerated as south-west corners
// so large attackers stop beside the footprint too); at
// longer range it is RangedDestination. Fallbacks: the
// nearest touching placement without an open edge, then the
// target tile itself.
func combatRouteDestination(attacker, target Combatant, attackDistance int) Location {
	targetLoc := target.Mobile().Location()
	attackerLoc := attacker.Mobile().Location()
	if attackDistance > 1 {
		return RangedDestination(attacker, target, attackDistance)
	}
	targetBounds := boundsOf(targetLoc, target.Size())
	size := attacker.Size()
	if size < 1 {
		size = 1
	}
	chebyshev := func(x, y int) int {
		dx := x - attackerLoc.X
		if dx < 0 {
			dx = -dx
		}
		dy := y - attackerLoc.Y
		if dy < 0 {
			dy = -dy
		}
		if dx > dy {
			return dx
		}
		return dy
	}
	var best, fallback Location
	bestOK, fallbackOK := false, false
	bestDist, fallbackDist := 1<<30, 1<<30
	bestBlocked := true
	for x := targetBounds.minX - size; x <= targetBounds.maxX+1; x++ {
		for y := targetBounds.minY - size; y <= targetBounds.maxY+1; y++ {
			cand := boundsOf(Location{X: x, Y: y, Z: targetLoc.Z}, size)
			if boundsOverlap(cand, targetBounds) || boundsDistance(cand, targetBounds) != 1 {
				continue
			}
			d := chebyshev(x, y)
			if d < fallbackDist {
				fallback, fallbackDist, fallbackOK = Location{X: x, Y: y, Z: targetLoc.Z}, d, true
			}
			if !boundsShareEdge(cand, targetBounds) || !hasOpenSharedEdge(cand, targetBounds, targetLoc.Z) {
				continue
			}
			blocked := combatClipping(x, y, targetLoc.Z) != 0
			if !bestOK || d < bestDist || (d == bestDist && bestBlocked && !blocked) {
				best, bestDist, bestBlocked, bestOK = Location{X: x, Y: y, Z: targetLoc.Z}, d, blocked, true
			}
		}
	}
	if bestOK {
		return best
	}
	if fallbackOK {
		return fallback
	}
	return targetLoc
}

// WithinApproachDistance is CombatRange.withinApproachDistance:
// whether source stands within range tiles of destination
// with an open projectile line (the walk-to reach predicate
// for at-range interactions).
func WithinApproachDistance(source Combatant, destination Location, attackRange int) bool {
	if source == nil || source.Mobile() == nil {
		return false
	}
	sourceLoc := source.Mobile().Location()
	if sourceLoc.Z != destination.Z {
		return false
	}
	if attackRange < 0 {
		attackRange = 0
	}
	sourceBounds := boundsOf(sourceLoc, source.Size())
	destinationBounds := combatBounds{minX: destination.X, minY: destination.Y, maxX: destination.X, maxY: destination.Y}
	if boundsDistance(sourceBounds, destinationBounds) > attackRange {
		return false
	}
	return hasProjectileLine(sourceBounds, destinationBounds, sourceLoc.Z)
}

// Beside is CombatRange.beside: footprints touch without
// overlapping (gap <= 1, no shared tile).
func Beside(a, b Combatant) bool {
	if a == nil || b == nil || a.Mobile() == nil || b.Mobile() == nil {
		return false
	}
	first := boundsOf(a.Mobile().Location(), a.Size())
	second := boundsOf(b.Mobile().Location(), b.Size())
	return !boundsOverlap(first, second) && boundsDistance(first, second) <= 1
}

// OverlapsEntities is CombatRange.overlapsEntities: same floor
// and shared footprint tiles.
func OverlapsEntities(a, b Combatant) bool {
	if a == nil || b == nil || a.Mobile() == nil || b.Mobile() == nil {
		return false
	}
	if a.Mobile().Location().Z != b.Mobile().Location().Z {
		return false
	}
	return boundsOverlap(boundsOf(a.Mobile().Location(), a.Size()), boundsOf(b.Mobile().Location(), b.Size()))
}

// RangedDestination is CombatRange.rangedDestination: a tile
// on the source's current side of the target, exactly range
// tiles from its footprint (the walk-to destination for
// at-range actions).
func RangedDestination(source, target Combatant, attackRange int) Location {
	sourceBounds := boundsOf(source.Mobile().Location(), source.Size())
	targetBounds := boundsOf(target.Mobile().Location(), target.Size())
	targetLoc := target.Mobile().Location()
	distance := attackRange
	if distance < 1 {
		distance = 1
	}
	closest := func(value, min, max int) int {
		if value < min {
			return min
		}
		if value > max {
			return max
		}
		return value
	}
	var x, y int
	switch {
	case sourceBounds.maxX < targetBounds.minX:
		x = targetBounds.minX - distance
	case sourceBounds.minX > targetBounds.maxX:
		x = targetBounds.maxX + distance
	default:
		x = closest(source.Mobile().Location().X, targetBounds.minX, targetBounds.maxX)
	}
	switch {
	case sourceBounds.maxY < targetBounds.minY:
		y = targetBounds.minY - distance
	case sourceBounds.minY > targetBounds.maxY:
		y = targetBounds.maxY + distance
	default:
		y = closest(source.Mobile().Location().Y, targetBounds.minY, targetBounds.maxY)
	}
	return Location{X: x, Y: y, Z: targetLoc.Z}
}

// hasOpenSharedEdge is CombatRange.hasOpenSharedEdge: some
// shared edge segment is open on both tiles (the cardinal
// edge masks plus UNKNOWN/UNLOADED must be clear on both
// sides).
func hasOpenSharedEdge(a, b combatBounds, z int) bool {
	if a.maxX+1 == b.minX || b.maxX+1 == a.minX {
		fromX, toX := a.maxX, b.minX
		if b.maxX+1 == a.minX {
			fromX, toX = a.minX, b.maxX
		}
		lo, hi := a.minY, a.maxY
		if b.minY > lo {
			lo = b.minY
		}
		if b.maxY < hi {
			hi = b.maxY
		}
		for y := lo; y <= hi; y++ {
			if hasOpenCardinalEdge(fromX, y, toX, y, z) {
				return true
			}
		}
	}
	if a.maxY+1 == b.minY || b.maxY+1 == a.minY {
		fromY, toY := a.maxY, b.minY
		if b.maxY+1 == a.minY {
			fromY, toY = a.minY, b.maxY
		}
		lo, hi := a.minX, a.maxX
		if b.minX > lo {
			lo = b.minX
		}
		if b.maxX < hi {
			hi = b.maxX
		}
		for x := lo; x <= hi; x++ {
			if hasOpenCardinalEdge(x, fromY, x, toY, z) {
				return true
			}
		}
	}
	return false
}

// hasOpenCardinalEdge is CombatRange.hasOpenCardinalEdge: the
// movement masks in the step direction (east 0x8, west 0x80,
// north 0x2, south 0x20 — the walk masks, not the projectile
// masks) plus UNKNOWN/UNLOADED must be clear on both tiles.
func hasOpenCardinalEdge(fromX, fromY, toX, toY, z int) bool {
	var fromMask, toMask int32
	switch {
	case toX > fromX:
		fromMask, toMask = 0x8, 0x80
	case toX < fromX:
		fromMask, toMask = 0x80, 0x8
	case toY > fromY:
		fromMask, toMask = 0x2, 0x20
	default:
		fromMask, toMask = 0x20, 0x2
	}
	invalid := int32(ClipUnknown | ClipUnloadedTile)
	return combatClipping(fromX, fromY, z)&(fromMask|invalid) == 0 &&
		combatClipping(toX, toY, z)&(toMask|invalid) == 0
}

// hasProjectileLine is CombatRange.hasProjectileLine via
// RegionManager.canProjectileAttackBounds: clamp each
// footprint to the edge nearest the other, then cast the ray.
func hasProjectileLine(source, destination combatBounds, z int) bool {
	coordinate := func(origin, other, size int) int {
		if origin >= other {
			return origin
		}
		if origin+size-1 <= other {
			return origin + size - 1
		}
		return other
	}
	sourceWidth := source.maxX - source.minX + 1
	sourceLength := source.maxY - source.minY + 1
	destWidth := destination.maxX - destination.minX + 1
	destLength := destination.maxY - destination.minY + 1
	startX := coordinate(source.minX, destination.minX, sourceWidth)
	startY := coordinate(source.minY, destination.minY, sourceLength)
	endX := coordinate(destination.minX, source.minX, destWidth)
	endY := coordinate(destination.minY, source.minY, destLength)
	return canProjectileRay(startX, startY, endX, endY, z)
}

// canProjectileRay is RegionManager.canProjectileRay: the
// Bresenham-style ray at 65536 scale with a half-tile offset.
// Directional projectile flags plus UNLOADED/UNKNOWN/
// PROJECTILE_TILE_BLOCKED block the ray; the end tile exempts
// PROJECTILE_TILE_BLOCKED. Dynamic NPC blockers
// (getProjectileClipping's World check) are NOT ported: no
// world NPC lookup exists in combat, so the ray reads plain
// clipping.
func canProjectileRay(startX, startY, endX, endY, z int) bool {
	if startX == endX && startY == endY {
		return true
	}
	deltaX := endX - startX
	deltaY := endY - startY
	east := deltaX >= 0
	north := deltaY >= 0
	xFlags := int32(ClipUnloadedTile | ClipUnknown | ProjectileTileBlocked)
	if east {
		xFlags |= ProjectileWestBlocked
	} else {
		xFlags |= ProjectileEastBlocked
	}
	yFlags := int32(ClipUnloadedTile | ClipUnknown | ProjectileTileBlocked)
	if north {
		yFlags |= ProjectileSouthBlocked
	} else {
		yFlags |= ProjectileNorthBlocked
	}
	const scale = 65536
	const halfTile = scale / 2
	abs := func(v int) int {
		if v < 0 {
			return -v
		}
		return v
	}
	if abs(deltaX) > abs(deltaY) {
		offsetX := 1
		if !east {
			offsetX = -1
		}
		scaledY := startY*scale + halfTile
		if !north {
			scaledY--
		}
		tangent := deltaY * scale / abs(deltaX) // truncates toward zero, like Math.trunc
		for x := startX; x != endX; {
			x += offsetX
			y := scaledY / scale // floor for non-negative scaledY; scaledY stays >= 0 on real maps
			if scaledY < 0 && scaledY%scale != 0 {
				y--
			}
			atEnd := x == endX && y == endY
			flags := xFlags
			if atEnd {
				flags &^= ProjectileTileBlocked
			}
			if combatClipping(x, y, z)&flags != 0 {
				return false
			}
			scaledY += tangent
			nextY := scaledY / scale
			if scaledY < 0 && scaledY%scale != 0 {
				nextY--
			}
			if nextY != y {
				nextAtEnd := x == endX && nextY == endY
				flags := yFlags
				if nextAtEnd {
					flags &^= ProjectileTileBlocked
				}
				if combatClipping(x, nextY, z)&flags != 0 {
					return false
				}
			}
		}
	} else {
		offsetY := 1
		if !north {
			offsetY = -1
		}
		scaledX := startX*scale + halfTile
		if !east {
			scaledX--
		}
		tangent := deltaX * scale / abs(deltaY)
		for y := startY; y != endY; {
			y += offsetY
			x := scaledX / scale
			if scaledX < 0 && scaledX%scale != 0 {
				x--
			}
			atEnd := x == endX && y == endY
			flags := yFlags
			if atEnd {
				flags &^= ProjectileTileBlocked
			}
			if combatClipping(x, y, z)&flags != 0 {
				return false
			}
			scaledX += tangent
			nextX := scaledX / scale
			if scaledX < 0 && scaledX%scale != 0 {
				nextX--
			}
			if nextX != x {
				nextAtEnd := nextX == endX && y == endY
				flags := xFlags
				if nextAtEnd {
					flags &^= ProjectileTileBlocked
				}
				if combatClipping(nextX, y, z)&flags != 0 {
					return false
				}
			}
		}
	}
	return true
}
