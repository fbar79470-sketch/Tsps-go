package doors

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// Pass-through gates (Doors.plugin.js:2000-2029 handlePassThroughGate
// and :1849-1998 stepThroughGate / carryAcrossGate / carryOneTile /
// walkOverGate / carryAcrossDiagonal): gates that let the player
// through and shut behind them, never staying open. Opening swaps the
// closed leaves for the opened leaves (an inviswall blocker on the
// doorway plus open leaves past the edge), steps the player across,
// and swaps back after CloseTicks (2, or 5 for the West Ardougne wall
// gate). The gate data itself lives in tables.go (Worker A); this file
// owns the crossing math (kept pure and unit-tested) and the engine
// that runs it.

// gateWalkAnimation is the walk animation forced movement plays while
// carrying a player across a pass-through gate
// (Doors.plugin.js:1883 GATE_WALK_ANIMATION).
const gateWalkAnimation = 819

// passThroughDefaultCloseTicks is the close timer when a gate table
// entry carries no CloseTicks (Doors.plugin.js:2024-2029; the plan
// §3 closeTicks timer: 2, or 5 for the West Ardougne wall gate).
const passThroughDefaultCloseTicks = 2

// walkOverMaxTicks caps how long a walkOver opener keeps the
// ignore-clipping flag (Doors.plugin.js:11-13 WALK_OVER_MAX_TICKS).
const walkOverMaxTicks = 2

// walkOverState is one player's in-flight walkOver crossing
// (Doors.plugin.js:1953-1955 WALKING_OVER): the token identifying this
// crossing and whether the player had the ignore-clipping flag before
// it, so ending the crossing restores the prior state. Go has no
// getter for the queue's ignore-clipping flag (MovementQueue only
// exposes SetIgnoreClipping), so previous is always false in practice:
// this package is the only setter.
type walkOverState struct {
	token    int
	previous bool
}

// pastEdgeOffsets is the tile offset past a wall loc's blocked edge,
// by face (Doors.plugin.js:1847 PAST_EDGE).
var pastEdgeOffsets = [4][2]int{{-1, 0}, {0, 1}, {1, 0}, {0, -1}}

// familyGateAt builds a pass-through gate for one placement of a
// PASS_THROUGH_FAMILIES door (Doors.plugin.js:1851-1867 familyGateAt):
// INVISWALL on the door's tile and its open leaf on the tile past the
// edge the door blocks, turned a quarter (face + 1), with the family
// sound and close timer; the opener walks over the blocker. The
// boundary crossed is the blocked edge: x for face 0 (west), x + 1 for
// face 2 (east), y + 1 for face 1 (north), y for face 3 (south). A
// family with no resolved open leaf (HasOpenLeaf false — the cache
// `<name>_open` loc the catalog resolves at build time) builds no
// gate: nil, false.
func familyGateAt(family *PassThroughFamily, objectID, x, y, z, face int) (*PassThroughGate, bool) {
	if family == nil || !family.HasOpenLeaf {
		return nil, false
	}
	f := face & 3
	d := pastEdgeOffsets[f]
	gate := &PassThroughGate{
		Closed: []PassThroughLeaf{{ID: objectID, X: x, Y: y, Z: z, Face: f}},
		Opened: []PassThroughLeaf{
			{ID: namelessInviswall, X: x, Y: y, Z: z, Face: f},
			{ID: family.OpenLeaf, X: x + d[0], Y: y + d[1], Z: z, Face: (f + 1) & 3},
		},
		WalkOver:    true,
		CloseTicks:  family.CloseTicks,
		OpenSoundID: family.OpenSoundID,
	}
	if f == 0 || f == 2 {
		// The boundary crossed is the blocked edge: x for west, x + 1 for east.
		if f == 0 {
			gate.CrossX = x
		} else {
			gate.CrossX = x + 1
		}
		gate.HasCrossX = true
	} else {
		// y + 1 for north, y for south.
		if f == 1 {
			gate.CrossY = y + 1
		} else {
			gate.CrossY = y
		}
		gate.HasCrossY = true
	}
	return gate, true
}

// carryAcrossGatePlan is the pure crossing math of carryAcrossGate
// (Doors.plugin.js:1885-1900): a thick gate whose tiles the map
// blocks, carried across it on the player's own row, in both
// directions. Returns the tile the glide starts from, the glide
// delta, and the facing direction (1 east / 3 west). ok is false when
// the player stands more than a tile from the crossing start —
// too far away to be carried.
func carryAcrossGatePlan(here game.Location, gate *PassThroughGate, y, z int) (from, delta game.Location, direction int, ok bool) {
	low, high := gate.CrossBetween[0], gate.CrossBetween[1]
	row := y
	for _, r := range gate.CrossRows {
		if r == here.Y {
			row = here.Y
			break
		}
	}
	if len(gate.CrossRows) == 1 {
		row = gate.CrossRows[0]
	}
	eastward := here.X < (low+high)/2
	if eastward {
		from = game.Location{X: low, Y: row, Z: z}
	} else {
		from = game.Location{X: high, Y: row, Z: z}
	}
	if absInt(here.X-from.X) > 1 || absInt(here.Y-from.Y) > 1 {
		return game.Location{}, game.Location{}, 0, false
	}
	span := high - low
	if eastward {
		return from, game.Location{X: span, Y: 0}, 1, true
	}
	return from, game.Location{X: -span, Y: 0}, 3, true
}

// carryOneTilePlan is the pure crossing math of carryOneTile
// (Doors.plugin.js:1907-1921): a gate that keeps a collision blocker
// on its doorway while open, crossed by carrying the opener the one
// tile over (forced movement ignores the blocker). Returns the glide
// delta and the facing direction. ok is false when the player is not
// on the gate's row/column or not exactly one tile from the boundary.
func carryOneTilePlan(here game.Location, gate *PassThroughGate, x, y, z int) (delta game.Location, direction int, ok bool) {
	acrossX := gate.HasCrossX
	if acrossX {
		if here.Y != y {
			return game.Location{}, 0, false
		}
	} else if here.X != x {
		return game.Location{}, 0, false
	}
	cross := gate.CrossY
	if acrossX {
		cross = gate.CrossX
	}
	position := here.Y
	if acrossX {
		position = here.X
	}
	d := cross - position
	if position >= cross {
		d = cross - 1 - position
	}
	if absInt(d) != 1 {
		return game.Location{}, 0, false
	}
	if acrossX {
		if d > 0 {
			return game.Location{X: d}, 1, true
		}
		return game.Location{X: d}, 3, true
	}
	if d > 0 {
		return game.Location{Y: d}, 0, true
	}
	return game.Location{Y: d}, 2, true
}

// diagonalPlan is the pure crossing math of carryAcrossDiagonal
// (Doors.plugin.js:1957-1972): a diagonal door crossed corner to
// corner (the Ranging Guild's). Entering means moving from the
// outside corner to the inside corner; leaving reverses it. The
// eastwardFrom predicate is the gate's own
// (Doors.plugin.js:846: here.x - here.y <= closed.x - closed.y for
// the Ranging Guild gate, where closed is the gate's closed tile);
// CrossDiagonalEastward selects that predicate. Returns the glide
// start, delta, and facing direction (1 east / 3 west). ok is false
// when the player stands more than a tile from the crossing start.
func diagonalPlan(here game.Location, gate *PassThroughGate, z int) (from, delta game.Location, direction int, ok bool) {
	closed := gate.Closed[0]
	entering := here.X-here.Y <= closed.X-closed.Y
	if !gate.CrossDiagonalEastward {
		entering = here.X <= closed.X
	}
	var fromXY, toXY [2]int
	if entering {
		fromXY, toXY = gate.CrossDiagonalOutside, gate.CrossDiagonalInside
	} else {
		fromXY, toXY = gate.CrossDiagonalInside, gate.CrossDiagonalOutside
	}
	if absInt(here.X-fromXY[0]) > 1 || absInt(here.Y-fromXY[1]) > 1 {
		return game.Location{}, game.Location{}, 0, false
	}
	from = game.Location{X: fromXY[0], Y: fromXY[1], Z: z}
	if entering {
		return from, game.Location{X: toXY[0] - fromXY[0], Y: toXY[1] - fromXY[1]}, 1, true
	}
	return from, game.Location{X: toXY[0] - fromXY[0], Y: toXY[1] - fromXY[1]}, 3, true
}

// stepDestination is the ordinary / walkOver destination of
// stepThroughGate (Doors.plugin.js:1975-1998): the tile just across
// the crossed boundary, on the gate's row (crossX) or column
// (crossY). x, y, z are the clicked loc's tile.
func stepDestination(here game.Location, gate *PassThroughGate, x, y, z int) game.Location {
	if gate.HasCrossX {
		if here.X < gate.CrossX {
			return game.Location{X: gate.CrossX, Y: y, Z: z}
		}
		return game.Location{X: gate.CrossX - 1, Y: y, Z: z}
	}
	if here.Y < gate.CrossY {
		return game.Location{X: x, Y: gate.CrossY, Z: z}
	}
	return game.Location{X: x, Y: gate.CrossY - 1, Z: z}
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

// passThroughGateAt resolves the pass-through gate for a clicked loc:
// a placed gate whose closed leaf with this id stands at this tile
// (the catalog's key), else a gate whose closed OR opened leaves
// include this id at this tile — a click on an already-open leaf
// still resolves its gate so the player steps through — else the
// family gate built from the loc's own tile and face
// (Doors.plugin.js:1869-1871 passThroughGateAt + familyGateAt).
func (e *doorEngine) passThroughGateAt(objectID int, loc game.Location, face int) (*PassThroughGate, bool) {
	if e.catalog != nil {
		if gate, ok := e.catalog.PassThroughGate(objectID, loc.X, loc.Y, loc.Z); ok {
			return gate, true
		}
		if family, ok := e.catalog.PassThroughFamily(objectID); ok {
			if gate, ok := familyGateAt(family, objectID, loc.X, loc.Y, loc.Z, face); ok {
				return gate, true
			}
		}
	}
	for _, gate := range passThroughGates {
		for _, leaf := range append(append([]PassThroughLeaf(nil), gate.Closed...), gate.Opened...) {
			if leaf.ID == objectID && leaf.X == loc.X && leaf.Y == loc.Y && leaf.Z == loc.Z {
				return gate, true
			}
		}
	}
	return nil, false
}

// swapLeaves deregisters every leaf in remove and registers every leaf
// in add (Doors.plugin.js:1877-1880 swapLocs), through the object
// manager with playerUpdate=true so nearby players see the swap on the
// next tick flush.
func (e *doorEngine) swapLeaves(remove, add []PassThroughLeaf) {
	objects := e.objectManager()
	if objects == nil {
		return
	}
	for _, leaf := range remove {
		objects.Deregister(leafObject(leaf), true)
	}
	for _, leaf := range add {
		objects.Register(leafObject(leaf), true)
	}
}

func leafObject(leaf PassThroughLeaf) *game.GameObject {
	return game.NewGameObject(leaf.ID, game.Location{X: leaf.X, Y: leaf.Y, Z: leaf.Z}, leaf.Shape, leaf.Face, nil)
}

// passThroughAnchorKey is the anchor a pass-through opening is tracked
// under (Doors.plugin.js:2008-2010): "pass-through:" + the first
// closed leaf's tile.
func passThroughAnchorKey(gate *PassThroughGate) string {
	if len(gate.Closed) == 0 {
		return "pass-through:?"
	}
	first := gate.Closed[0]
	return "pass-through:" + locationKey(game.Location{X: first.X, Y: first.Y, Z: first.Z})
}

// handlePassThroughGate opens a pass-through gate, steps the player
// across it and shuts it again (Doors.plugin.js:2000-2029). A gate
// already open lets the player step through it while it stands open.
// The opening sound only is played; the gate shuts silently. The
// close timer is keyed by the anchor so re-opening resets it.
func (e *doorEngine) handlePassThroughGate(p *game.Player, objectID int, loc game.Location, obj plugins.GameObject) bool {
	face := 0
	if obj != nil {
		face = obj.Face()
	}
	gate, ok := e.passThroughGateAt(objectID, loc, face)
	if !ok {
		return false
	}
	anchorKey := passThroughAnchorKey(gate)
	if e.passThroughOpen[anchorKey] {
		e.stepThroughGate(p, gate, loc)
		return true
	}
	e.swapLeaves(gate.Closed, gate.Opened)
	e.passThroughOpen[anchorKey] = true
	soundID := gate.OpenSoundID
	if soundID == 0 {
		soundID = game.SoundGateOpen.ID()
	}
	e.playSound(p, game.NewSound(soundID, 1, 0, 0))
	e.stepThroughGate(p, gate, loc)
	closeTicks := gate.CloseTicks
	if closeTicks <= 0 {
		closeTicks = passThroughDefaultCloseTicks
	}
	if tasks := e.taskManager(); tasks != nil {
		task := game.NewTask(closeTicks, func(cycle int64) bool {
			delete(e.passThroughOpen, anchorKey)
			e.swapLeaves(gate.Opened, gate.Closed)
			return true
		})
		task.Key = anchorKey
		task.Name = "doors:pass-through-close"
		tasks.Schedule(task)
	}
	return true
}

// stepThroughGate moves the player across an open pass-through gate
// (Doors.plugin.js:1975-1998), in TS order: diagonal, thick
// (crossBetween), carry, else walkOver or an ordinary queue step.
func (e *doorEngine) stepThroughGate(p *game.Player, gate *PassThroughGate, loc game.Location) {
	if p == nil {
		return
	}
	if gate.HasCrossDiagonal {
		e.carryAcrossDiagonal(p, gate, loc.Z)
		return
	}
	if gate.HasCrossBetween {
		e.carryAcrossGate(p, gate, loc.Y, loc.Z)
		return
	}
	if gate.Carry {
		e.carryOneTile(p, gate, loc.X, loc.Y, loc.Z)
		return
	}
	dest := stepDestination(p.Location(), gate, loc.X, loc.Y, loc.Z)
	if gate.WalkOver {
		e.walkOverGate(p, dest)
		return
	}
	if q := p.MovementQueue(); q != nil {
		q.Reset(true)
		q.AddCheckpoint(dest)
	}
}

// carryAcrossGate carries the player across a thick gate, on their own
// row, in both directions (Doors.plugin.js:1885-1900). The midpoint of
// the span decides the side; a player standing north of the leaf shares
// its x, so the gate's crossBetween (not the leaf's own tile) says
// which side is which. The glide takes span ticks at reverseSpeed
// 30*span.
func (e *doorEngine) carryAcrossGate(p *game.Player, gate *PassThroughGate, y, z int) {
	tasks := e.taskManager()
	if tasks == nil {
		return
	}
	from, delta, direction, ok := carryAcrossGatePlan(p.Location(), gate, y, z)
	if !ok {
		return
	}
	if p.Location() != from {
		game.MovePlayerTo(p, from)
	}
	span := gate.CrossBetween[1] - gate.CrossBetween[0]
	tasks.Schedule(game.NewForceMovementTask(p, span, &game.ForceMovement{
		Start:        from,
		Delta:        delta,
		Speed:        0,
		ReverseSpeed: 30 * span,
		Direction:    direction,
		Animation:    gateWalkAnimation,
	}).Task())
}

// carryOneTile carries the opener the one tile over a gate that keeps
// a collision blocker on its doorway (Doors.plugin.js:1907-1921);
// forced movement ignores the blocker.
func (e *doorEngine) carryOneTile(p *game.Player, gate *PassThroughGate, x, y, z int) {
	tasks := e.taskManager()
	if tasks == nil {
		return
	}
	delta, direction, ok := carryOneTilePlan(p.Location(), gate, x, y, z)
	if !ok {
		return
	}
	tasks.Schedule(game.NewForceMovementTask(p, 1, &game.ForceMovement{
		Start:        p.Location(),
		Delta:        delta,
		Speed:        0,
		ReverseSpeed: 30,
		Direction:    direction,
		Animation:    gateWalkAnimation,
	}).Task())
}

// carryAcrossDiagonal carries the opener corner to corner across a
// diagonal door (Doors.plugin.js:1957-1972), in one tick.
func (e *doorEngine) carryAcrossDiagonal(p *game.Player, gate *PassThroughGate, z int) {
	tasks := e.taskManager()
	if tasks == nil {
		return
	}
	from, delta, direction, ok := diagonalPlan(p.Location(), gate, z)
	if !ok {
		return
	}
	if p.Location() != from {
		game.MovePlayerTo(p, from)
	}
	tasks.Schedule(game.NewForceMovementTask(p, 1, &game.ForceMovement{
		Start:        from,
		Delta:        delta,
		Speed:        0,
		ReverseSpeed: 30,
		Direction:    direction,
		Animation:    gateWalkAnimation,
	}).Task())
}

// endWalkOver ends a walkOver crossing (Doors.plugin.js:1944-1951):
// the token must match, and the ignore-clipping flag is restored only
// when the player did not have it before.
func (e *doorEngine) endWalkOver(p *game.Player, token int) {
	state, ok := e.walkOvers[p]
	if !ok || state.token != token {
		return
	}
	delete(e.walkOvers, p)
	if !state.previous {
		if q := p.MovementQueue(); q != nil {
			q.SetIgnoreClipping(false)
		}
	}
}

// walkOverGate crosses a gate that keeps a blocker on its doorway with
// an ordinary walk (Doors.plugin.js:1962-1974): the opener's one step
// ignores clipping and the flag ends once they are across (at most
// WALK_OVER_MAX_TICKS), so the blocker stops everyone else and no
// forced movement is sent.
func (e *doorEngine) walkOverGate(p *game.Player, destination game.Location) {
	e.endWalkOver(p, e.walkOverToken)
	e.walkOverToken++
	token := e.walkOverToken
	e.walkOvers[p] = walkOverState{token: token, previous: false}
	if q := p.MovementQueue(); q != nil {
		q.SetIgnoreClipping(true)
		q.Reset(true)
		q.AddCheckpoint(destination)
	}
	tasks := e.taskManager()
	if tasks == nil {
		return
	}
	ticks := walkOverMaxTicks
	task := game.NewTask(1, func(cycle int64) bool {
		if p.Location() == destination {
			e.endWalkOver(p, token)
			return true
		}
		ticks--
		if ticks <= 0 {
			e.endWalkOver(p, token)
			return true
		}
		if q := p.MovementQueue(); q != nil && q.Size() == 0 {
			e.endWalkOver(p, token)
			return true
		}
		return false
	})
	task.Key = p
	task.Name = "doors:walk-over"
	tasks.Schedule(task)
}
