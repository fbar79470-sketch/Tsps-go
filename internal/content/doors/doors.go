package doors

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// The toggle engine: the Go port of toggleDoor / handleMappedDoor /
// handleDoubleDoor / handleWoodenGate / handleCompositeDoor /
// routeWallDoor / requestDoorOpen / readDoorState in
// server/plugins/objects/Doors.plugin.js (upstream HEAD bfd81e3).
// The catalog it swings against is Worker A's (catalog.go); the
// exception tables it reads are tables.go. Open state is tracked in
// memory (anchor key -> closed/current snapshots) with a 500-tick
// auto-close, exactly as upstream's OPEN_OBJECT_STATES.
//
// Deviations, deliberate and documented:
//   - No onRegionLoaded re-apply: Go has no region-loaded hook family
//     (internal/plugins/hooks.go:136), so an open door is not
//     re-applied after a region reload. The state survives in memory;
//     the leaves come back only when the door is clicked again.
//   - The wall-door route is registered directly in Register, not
//     deferred to onServerStartup as upstream does; the effect is the
//     same once the hub loads.
//   - Swaps use Deregister + Register throughout, never
//     ReplaceInPlace: a door that swings to a different tile/face
//     must use the pair (internal/game/objects.go:1137), and using
//     one path for every swing keeps the in-place cases identical.
//   - Private areas do not exist in Go, so anchor keys carry no area
//     suffix and diagonal approaches are keyed by *game.Player in a
//     plain map (deleted on use and on the next route), not a WeakMap.

// autoCloseTicks is the ticks an opened door/gate stays open before it
// reverts on its own (Doors.plugin.js:9-11 DOOR_AUTO_CLOSE_TICKS:
// 300 seconds, 500 ticks @ 600ms/tick).
const autoCloseTicks = 500

// coordOffsets is the per-face step a swinging leaf takes
// (Doors.plugin.js:906-909 COORD_OFFSETS): face 0 west, 1 north,
// 2 east, 3 south.
var coordOffsets = [4][2]int{{-1, 0}, {0, 1}, {1, 0}, {0, -1}}

// diagonalDoorNormals is the normal of each closed diagonal leaf, used
// to retain the opener's approach side (Doors.plugin.js:914
// DIAGONAL_DOOR_NORMALS).
var diagonalDoorNormals = [4][2]int{{1, -1}, {-1, -1}, {-1, 1}, {1, 1}}

// leftLeafOpenFaces / rightLeafOpenFaces are the faces double-door
// leaves turn to when they open, by closed face
// (Doors.plugin.js:917-918).
var leftLeafOpenFaces = [4]int{3, 0, 1, 0}
var rightLeafOpenFaces = [4]int{1, 2, 3, 2}

// leftLeafOffsets is where the left leaf sits from the right one, and
// how its id differs, per closed face
// (Doors.plugin.js:921 LEFT_LEAF_OFFSETS: [dx, dy, idDelta]).
var leftLeafOffsets = [4][3]int{{0, -1, -3}, {-1, 0, -3}, {0, 1, -3}, {-1, 0, 3}}

// gatePartnerOffsets is where a wooden gate's partner panel may stand
// (Doors.plugin.js:926-935 GATE_PARTNER_OFFSETS).
var gatePartnerOffsets = [8][2]int{{0, 1}, {0, -1}, {1, 0}, {-1, 0}, {-1, -1}, {-1, 1}, {1, -1}, {1, 1}}

// doorNamePattern is the route fallback name test
// (Doors.plugin.js:2091 /\b(?:doors?|doorway|gates?)\b/i).
var doorNamePattern = regexp.MustCompile(`(?i)\b(?:doors?|doorway|gates?)\b`)

// DoorToggleEvent is the payload of the "door:toggle" custom event
// (Doors.plugin.js:2033-2044 toggleDoor): emitted first, before the
// generic swing runs. A listener sets Handled to claim the click —
// the generic swing never runs — and content that passed its own
// check re-opens the door by emitting "door:open". Handlers receive
// a *DoorToggleEvent.
type DoorToggleEvent struct {
	Player     *game.Player
	Object     plugins.GameObject
	ObjectID   int
	Definition plugins.ObjectDefinition
	Location   game.Location
	ClickType  int
	Handled    bool
}

// DoorOpenEvent is the payload of the "door:open" custom event
// (Doors.plugin.js:2046-2075 requestDoorOpen): key-on-object and other
// content request an Open, never toggle an open door shut. Handlers
// receive a *DoorOpenEvent; setting Handled reports the door opened
// (or already open).
type DoorOpenEvent struct {
	Player     *game.Player
	Object     plugins.GameObject
	Definition plugins.ObjectDefinition
	Handled    bool
}

// DoorStateEvent is the payload of the "door:state" custom event
// (Doors.plugin.js:2078-2084 readDoorState): the shared world's
// current leaves, read without changing the door or its auto-close
// timer. Callers put the closed leaves they ask about in Objects;
// the handler replaces any currently-open closed leaf with its open
// leaves. Handlers receive a *DoorStateEvent.
type DoorStateEvent struct {
	Objects []plugins.GameObject
}

// doorSnapshot is one loc as the engine tracks it: id, shape, face,
// tile (Doors.plugin.js:1221-1235 toObjectSnapshot, minus the
// private-area half, which Go has no form of).
type doorSnapshot struct {
	id, typ, face int
	loc           game.Location
}

// openDoorState is one open door/gate/double door under one anchor key
// (Doors.plugin.js:939 OPEN_OBJECT_STATES): the closed snapshots, the
// current (open) snapshots, the closed id, and whether the close sound
// is a gate sound.
type openDoorState struct {
	closedID int
	closed   []doorSnapshot
	current  []doorSnapshot
	gate     bool
}

// diagonalApproach records a type-9 diagonal door's approach side
// (Doors.plugin.js:2086 DIAGONAL_DOOR_APPROACHES): the door object and
// the player's tile at route time, so a swinging leaf never lands on
// the opener.
type diagonalApproach struct {
	object plugins.GameObject
	source game.Location
}

// doorEngine is the toggle engine's state: the catalog it swings
// against, the object/task/sound surfaces it drives, the custom-event
// emit, and the open-door / diagonal-approach / pass-through state.
//
// objects/tasks are resolved lazily through resolveObjects/resolveTasks
// (see objectManager/taskManager): both surfaces are installed on the
// hub AFTER plugin Load (hub.SetObjectManager runs after newPluginHub in
// cmd/server/server.go, the TaskManager arrives with InstallSkillHooks),
// so capturing them at Register time reads nil. This is the Phase 32
// hub contract: surfaces installed after Load must be resolved lazily,
// never captured in Register.
type doorEngine struct {
	catalog         *Catalog
	objects         plugins.ObjectManagerAPI
	tasks           *game.TaskManager
	resolveObjects  func() plugins.ObjectManagerAPI
	resolveTasks    func() *game.TaskManager
	sounds          plugins.SoundsService
	emit            func(name string, payload any)
	states          map[string]*openDoorState
	diagonal        map[*game.Player]diagonalApproach
	passThroughOpen map[string]bool
	walkOvers       map[*game.Player]walkOverState
	walkOverToken   int
}

// installedCatalog is the catalog the wiring installs at boot
// (cmd/server/doorswiring.go), before hub Load. A nil catalog means
// NoCache mode (or a test that never installed one): doors stay shut
// and every toggle falls through.
var installedCatalog *Catalog

// SetCatalog installs the door catalog built from the loaded cache
// definitions. Called once at boot, before hub Load.
func SetCatalog(c *Catalog) { installedCatalog = c }

// newDoorEngine builds an engine over explicit surfaces (tests build
// one directly; Register builds one from the Api).
func newDoorEngine(objects plugins.ObjectManagerAPI, tasks *game.TaskManager, emit func(name string, payload any), catalog *Catalog) *doorEngine {
	return &doorEngine{
		catalog:         catalog,
		objects:         objects,
		tasks:           tasks,
		emit:            emit,
		states:          make(map[string]*openDoorState),
		diagonal:        make(map[*game.Player]diagonalApproach),
		passThroughOpen: make(map[string]bool),
		walkOvers:       make(map[*game.Player]walkOverState),
	}
}

// engineFromAPI builds an engine from a plugin Api: the plugin
// SoundsService and the generic custom-event emit
// (internal/plugins/sounds.go, internal/plugins/events.go). The object
// manager and task manager are NOT captured here — both are installed
// on the hub after Load (see the doorEngine comment), so they are
// resolved lazily per use through objectManager/taskManager. Capturing
// them here read nil live, so every swap silently no-op'd while the
// sound still played (Phase 48 live verification).
func engineFromAPI(api *plugins.Api) *doorEngine {
	var emit func(name string, payload any)
	var sounds plugins.SoundsService
	if api != nil {
		emit = api.EmitCustomEvent
		if core := api.Core(); core != nil {
			sounds = core.Sounds()
		}
	}
	e := newDoorEngine(nil, nil, emit, installedCatalog)
	if api != nil {
		e.resolveObjects = api.GetObjectManager
		e.resolveTasks = api.GetTaskManager
	}
	e.sounds = sounds
	return e
}

// objectManager returns the installed object manager, resolving it
// lazily on first use: the hub installs it after plugin Load, so a
// Register-time read is nil. A nil result is not cached — the next
// call re-asks, so an engine built before the install still picks the
// manager up once it lands. Tests pass an explicit manager to
// newDoorEngine and never set a resolver, so this returns it directly.
func (e *doorEngine) objectManager() plugins.ObjectManagerAPI {
	if e.objects != nil {
		return e.objects
	}
	if e.resolveObjects != nil {
		e.objects = e.resolveObjects()
	}
	return e.objects
}

// taskManager is objectManager for the engine TaskManager, installed
// on the hub with InstallSkillHooks after plugin Load.
func (e *doorEngine) taskManager() *game.TaskManager {
	if e.tasks != nil {
		return e.tasks
	}
	if e.resolveTasks != nil {
		e.tasks = e.resolveTasks()
	}
	return e.tasks
}

// Plugin is the registered Doors plugin value: name and description
// from the plugin file (Doors.plugin.js module.exports).
var Plugin = plugins.Plugin{
	Name:        "Doors",
	Description: "Opens and closes doors, gates and double doors, swapping between closed and open locs, with route handling and reapplied state on region load.",
	Register: func(api *plugins.Api) {
		engine := engineFromAPI(api)
		for _, name := range DoorNameList() {
			// Some gates (e.g. 60760/60763) expose "Release"
			// instead of "Open" (Doors.plugin.js:2134-2136).
			api.OnObjectInteractionNamed(name, map[string]plugins.ObjectInteractionHandler{
				"Open":    func(ev *plugins.ObjectInteractionEvent) bool { return engine.toggle(ev) },
				"Close":   func(ev *plugins.ObjectInteractionEvent) bool { return engine.toggle(ev) },
				"Release": func(ev *plugins.ObjectInteractionEvent) bool { return engine.toggle(ev) },
			})
		}
		api.OnObjectRoute(engine.route)
		api.OnCustomEvent("door:open", func(payload any) {
			if ev, ok := payload.(*DoorOpenEvent); ok {
				engine.requestOpen(ev)
			}
		})
		api.OnCustomEvent("door:state", func(payload any) {
			if ev, ok := payload.(*DoorStateEvent); ok {
				engine.readState(ev)
			}
		})
	},
}

func init() {
	plugins.Register(Plugin)
}

func locationKey(loc game.Location) string {
	return fmt.Sprintf("%d,%d,%d", loc.X, loc.Y, loc.Z)
}

func snapshotOf(obj plugins.GameObject) doorSnapshot {
	if obj == nil {
		return doorSnapshot{id: -1}
	}
	return doorSnapshot{id: obj.ID(), typ: obj.Type(), face: obj.Face() & 3, loc: obj.Location()}
}

func (s doorSnapshot) object() *game.GameObject {
	return game.NewGameObject(s.id, s.loc, s.typ, s.face, nil)
}

func snapshotsMatch(a doorSnapshot, obj plugins.GameObject) bool {
	if obj == nil {
		return false
	}
	return a.id == obj.ID() && a.typ == obj.Type() && a.face == obj.Face()&3 && a.loc == obj.Location()
}

// openID / closedID / isSelfOpening / woodenGate / doubleDoorFamily
// are the catalog lookups, nil-catalog safe: with no catalog every
// lookup misses and doors stay shut.
func (e *doorEngine) openID(closedID int) (int, bool) {
	if e.catalog == nil {
		return 0, false
	}
	return e.catalog.OpenID(closedID)
}

func (e *doorEngine) closedID(openID int) (int, bool) {
	if e.catalog == nil {
		return 0, false
	}
	return e.catalog.ClosedID(openID)
}

func (e *doorEngine) woodenGate(id int) (*WoodenGate, bool) {
	if e.catalog == nil {
		return nil, false
	}
	return e.catalog.WoodenGate(id)
}

func (e *doorEngine) doubleDoorFamily(id int) ([]int, bool) {
	if e.catalog == nil {
		return nil, false
	}
	return e.catalog.DoubleDoorFamily(id)
}

// isInPlace / isOwnTile / isDorgeshDoor / compositeOpening are the
// catalog lookups with the transcribed tables as fallback, so engine
// tests that construct Catalog literals directly (which carry no
// table state) still swing those doors. BuildCatalog copies the
// tables into the catalog, so a built catalog and the fallback agree;
// the fallback only matters for literals, and for *_reverse doors the
// catalog (which the scan augments) wins.
func (e *doorEngine) isInPlace(id int) bool {
	if e.catalog != nil && e.catalog.IsInPlace(id) {
		return true
	}
	return inPlaceDoorIDs[id]
}

func (e *doorEngine) isOwnTile(id int) bool {
	if e.catalog != nil && e.catalog.IsOwnTile(id) {
		return true
	}
	return ownTileDoorIDs[id]
}

func (e *doorEngine) isDorgeshDoor(id int) bool {
	if e.catalog != nil && e.catalog.IsDorgeshDoor(id) {
		return true
	}
	return dorgeshCityDoorIDs[id]
}

func (e *doorEngine) compositeOpening(id int) ([]CompositeLeaf, bool) {
	if e.catalog != nil {
		if leaves, ok := e.catalog.CompositeOpening(id); ok {
			return leaves, true
		}
	}
	leaves, ok := compositeDoorOpenings[id]
	return leaves, ok
}

// doorSound picks the door/gate sound for a swing
// (Doors.plugin.js:1158-1161 doorSound): a def named "Gate" plays the
// gate sounds (open 67 / close 66), anything else the door sounds
// (open 62 / close 60).
func doorSoundFor(defName string, opening bool) game.Sound {
	if defName == "Gate" {
		if opening {
			return game.SoundGateOpen
		}
		return game.SoundGateClose
	}
	if opening {
		return game.SoundDoorOpen
	}
	return game.SoundDoorClose
}

func (e *doorEngine) playSound(p *game.Player, sound game.Sound) {
	e.sounds.SendSound(p, sound)
}

func (e *doorEngine) defName(ev *plugins.ObjectInteractionEvent) string {
	if ev != nil && ev.Definition != nil {
		return ev.Definition.Name()
	}
	return ""
}

// toggle is toggleDoor (Doors.plugin.js:2031-2044), in TS order: the
// Dorgesh-Kaan refusal, the door:toggle emit first (a handled request
// claims the click and the generic swing never runs), then
// pass-through gate, composite, wooden gate, double door, and finally
// the single mapped door. Returning false falls through to the next
// handler; any other result claims the interaction.
func (e *doorEngine) toggle(ev *plugins.ObjectInteractionEvent) bool {
	if ev == nil || ev.Player == nil || ev.Object == nil {
		return false
	}
	objectID := ev.ObjectID
	if objectID == 0 {
		objectID = ev.Object.ID()
	}
	defID := objectID
	if ev.Definition != nil {
		defID = ev.Definition.ID()
	}
	if e.isDorgeshDoor(objectID) || e.isDorgeshDoor(defID) {
		return false
	}
	req := &DoorToggleEvent{
		Player:     ev.Player,
		Object:     ev.Object,
		ObjectID:   objectID,
		Definition: ev.Definition,
		Location:   ev.Location,
		ClickType:  ev.ClickType,
	}
	if e.emit != nil {
		e.emit("door:toggle", req)
	}
	if req.Handled {
		return true
	}
	p, loc := ev.Player, ev.Location
	defName := e.defName(ev)
	if e.handlePassThroughGate(p, objectID, loc, ev.Object) {
		return true
	}
	if e.handleCompositeDoor(p, ev.Object, objectID, defName) {
		return true
	}
	if e.handleWoodenGate(p, ev.Object, objectID, loc) {
		return true
	}
	if e.handleDoubleDoor(p, ev.Object, objectID, loc, defName) {
		return true
	}
	return e.handleMappedDoor(p, ev.Object, defID, loc, defName)
}

// resolveClosedID resolves the closed id for a clicked loc
// (Doors.plugin.js:1285-1291 resolveClosedId): a tracked open leaf
// keeps its retained original (several closed variants can share one
// open cache loc), else the catalog's closed->open map, else its
// reverse map. -1 when the id is no door at all.
func (e *doorEngine) resolveClosedID(obj plugins.GameObject, objectID int) int {
	for _, st := range e.states {
		for _, cur := range st.current {
			if snapshotsMatch(cur, obj) {
				return st.closedID
			}
		}
	}
	if _, ok := e.openID(objectID); ok {
		return objectID
	}
	if closed, ok := e.closedID(objectID); ok {
		return closed
	}
	return -1
}

// findOpenState finds the open state for a closed id at a tile
// (Doors.plugin.js:1271-1284 findOpenDoorState): the direct anchor
// (the closed tile's key) when it holds this closed id, else any
// single-leaf state whose closed or current leaf stands on the tile.
// Returns the anchor key and the state, or "", nil.
func (e *doorEngine) findOpenState(closedID int, loc game.Location) (string, *openDoorState) {
	if st, ok := e.states[locationKey(loc)]; ok && len(st.closed) == 1 && st.closedID == closedID {
		return locationKey(loc), st
	}
	for key, st := range e.states {
		if len(st.closed) != 1 || st.closedID != closedID {
			continue
		}
		if st.closed[0].loc == loc || st.current[0].loc == loc {
			return key, st
		}
	}
	return "", nil
}

// rememberOpenObjects records an open door and schedules its
// auto-close (Doors.plugin.js:1286-1290 rememberOpenObjects +
// scheduleAutoClose). Re-opening resets the timer: any task already
// keyed by the anchor is cancelled first.
func (e *doorEngine) rememberOpenObjects(anchorKey string, closed, current []doorSnapshot, closedID int, gate bool) {
	e.states[anchorKey] = &openDoorState{closedID: closedID, closed: closed, current: current, gate: gate}
	e.scheduleAutoClose(anchorKey)
}

// clearOpenDoor forgets an open door and cancels its auto-close
// (Doors.plugin.js:1292-1295 clearOpenDoor). The caller already swung
// the leaves back; this only drops the state and the timer.
func (e *doorEngine) clearOpenDoor(anchorKey string) {
	delete(e.states, anchorKey)
	if tasks := e.taskManager(); tasks != nil {
		tasks.CancelTasks(anchorKey)
	}
}

// scheduleAutoClose arms the 500-tick auto-close for an anchor
// (Doors.plugin.js:1301-1308 scheduleAutoClose).
func (e *doorEngine) scheduleAutoClose(anchorKey string) {
	tasks := e.taskManager()
	if tasks == nil {
		return
	}
	tasks.CancelTasks(anchorKey)
	task := game.NewTask(autoCloseTicks, func(cycle int64) bool {
		e.autoCloseDoor(anchorKey)
		return true
	})
	task.Key = anchorKey
	task.Name = "doors:auto-close"
	tasks.Schedule(task)
}

// autoCloseDoor reverts every leaf tracked under an anchor back to its
// closed snapshot and plays the close sound at the door
// (Doors.plugin.js:1297-1315 autoCloseDoor). The opener is gone by
// now, so the sound goes out as an area sound at the closed tile, the
// Go form of Sounds.sendSound on a non-player object.
func (e *doorEngine) autoCloseDoor(anchorKey string) {
	st := e.states[anchorKey]
	if st == nil {
		return
	}
	if objects := e.objectManager(); objects != nil {
		for _, cur := range st.current {
			objects.Deregister(cur.object(), true)
		}
		for _, closed := range st.closed {
			objects.Register(closed.object(), true)
		}
	}
	delete(e.states, anchorKey)
	if len(st.closed) == 0 {
		return
	}
	sound := game.SoundDoorClose
	if st.gate {
		sound = game.SoundGateClose
	}
	loc := st.closed[0].loc
	game.PlayAreaSound(game.PlayAreaSoundOptions{
		SoundID: sound.ID(),
		X:       loc.X,
		Y:       loc.Y,
		Level:   loc.Z,
		Loops:   sound.LoopType(),
		Delay:   sound.Delay(),
		Radius:  5,
	})
}

// swap deregisters the previous leaf and registers the next one, both
// with playerUpdate=true so nearby players see the swap on the next
// tick flush (Doors.plugin.js handleMappedDoor :1605-1606).
func (e *doorEngine) swap(prev, next doorSnapshot) {
	objects := e.objectManager()
	if objects == nil {
		return
	}
	objects.Deregister(prev.object(), true)
	objects.Register(next.object(), true)
}

// canWalkTo reports whether the player can walk to a tile, through the
// movement queue (Doors.plugin.js handleMappedDoor :1595
// player.getMovementQueue().canWalkTo). A nil queue reads as walkable.
func (e *doorEngine) canWalkTo(p *game.Player, at game.Location) bool {
	if p == nil {
		return false
	}
	q := p.MovementQueue()
	if q == nil {
		return true
	}
	return q.CanWalkTo(at, p.Location())
}

// handleMappedDoor swings a single mapped door
// (Doors.plugin.js:1540-1617), in TS order: resolve the closed id,
// work out whether it is open, compute the next leaf (one tile along
// the closed face, turned +1; closing turns -1 back to the closed
// snapshot), keep a type-9 diagonal leaf off the opener, swap, record
// or clear the state, and play the sound. In-place doors replace the
// model on the same tile; own-tile doors turn on the closed tile;
// self-opening doors keep their id and tile, tracked by state only.
func (e *doorEngine) handleMappedDoor(p *game.Player, obj plugins.GameObject, objectID int, loc game.Location, defName string) bool {
	if obj == nil {
		return false
	}
	closedID := e.resolveClosedID(obj, objectID)
	if closedID < 0 {
		return false
	}
	anchorKey, state := e.findOpenState(closedID, loc)
	open := state != nil
	openID, ok := e.openID(closedID)
	if !ok {
		return false
	}
	selfOpening := openID == closedID
	nextID := openID
	if open {
		nextID = closedID
	}
	active := snapshotOf(obj)
	typ := active.typ
	rotation := active.face
	inPlace := e.isInPlace(closedID)
	ownTile := e.isOwnTile(closedID)
	nextRotation := rotation
	if open {
		nextRotation = (rotation + 1) & 3
	}
	offsetIndex := nextRotation
	if typ == 9 {
		offsetIndex = (nextRotation + 1) & 3
	}
	dx, dy := 0, 0
	if !inPlace && !ownTile && !selfOpening {
		dx, dy = coordOffsets[offsetIndex][0], coordOffsets[offsetIndex][1]
	}
	approach, hasApproach := e.diagonal[p]
	delete(e.diagonal, p)
	var closedSnap *doorSnapshot
	if open && state != nil && len(state.closed) > 0 {
		closedSnap = &state.closed[0]
	}
	nextLoc := game.Location{X: active.loc.X + dx, Y: active.loc.Y + dy, Z: active.loc.Z}
	nextFace := rotation
	if closedSnap != nil {
		nextLoc = closedSnap.loc
		nextFace = closedSnap.face
	} else if !inPlace {
		delta := 1
		if open {
			delta = -1
		}
		nextFace = (rotation + delta) & 3
	}
	if typ == 9 && p.Location() == nextLoc {
		// A reachable leaf can swing onto the opener: keep their
		// original approach side clear
		// (Doors.plugin.js:1580-1600).
		closedLoc := nextLoc
		if !open {
			closedLoc = active.loc
		}
		normalIndex := rotation
		if open {
			normalIndex = nextFace
		}
		normal := diagonalDoorNormals[normalIndex]
		step := coordOffsets[rotation]
		if open {
			step = normal
		}
		source := p.Location()
		if hasApproach && approach.object == obj {
			source = approach.source
		}
		sideOf := func(at game.Location) int {
			return (at.X-closedLoc.X)*normal[0] + (at.Y-closedLoc.Y)*normal[1]
		}
		normalSide := signInt(sideOf(source))
		if normalSide == 0 {
			normalSide = signInt(sideOf(p.Location()))
		}
		if normalSide == 0 {
			normalSide = 1
		}
		reverse := 1
		if normalSide*(step[0]*normal[0]+step[1]*normal[1]) < 0 {
			reverse = -1
		}
		steps := [][2]int{
			{step[0] * reverse, step[1] * reverse},
			{normal[0] * normalSide, 0},
			{0, normal[1] * normalSide},
		}
		var destination *game.Location
		for _, s := range steps {
			at := game.Location{X: nextLoc.X + s[0], Y: nextLoc.Y + s[1], Z: nextLoc.Z}
			if sideOf(at)*normalSide > 0 && e.canWalkTo(p, at) {
				destination = &at
				break
			}
		}
		if destination == nil {
			return true
		}
		game.MovePlayerTo(p, *destination)
	}
	next := doorSnapshot{id: nextID, typ: typ, face: nextFace, loc: nextLoc}
	e.swap(active, next)
	if open {
		e.clearOpenDoor(anchorKey)
	} else {
		e.rememberOpenObjects(locationKey(active.loc), []doorSnapshot{active}, []doorSnapshot{next}, closedID, false)
	}
	e.playSound(p, doorSoundFor(defName, !open))
	return true
}

func signInt(n int) int {
	if n > 0 {
		return 1
	}
	if n < 0 {
		return -1
	}
	return 0
}

// gateHingeTransform is the wooden-gate pivot
// (Doors.plugin.js:1391-1411 gateHingeTransform, ported 1:1 from the
// original elvarg gate state manager): both panels land on the
// perpendicular tile line and face the same direction. Opening swings
// the hinge/extension to the hinge post's side; closing swings them
// back. Returns the new hinge tile, the new extension tile, and the
// face both panels take.
func gateHingeTransform(x, y, rotation int, opening bool) (hinge, extension [2]int, face int) {
	switch rotation & 0x3 {
	case 0:
		if opening {
			return [2]int{x - 1, y}, [2]int{x - 2, y}, 3
		}
		return [2]int{x, y - 1}, [2]int{x + 1, y - 1}, 1
	case 1:
		if opening {
			return [2]int{x, y + 1}, [2]int{x, y + 2}, 0
		}
		return [2]int{x - 1, y}, [2]int{x - 1, y - 1}, 2
	case 2:
		if opening {
			return [2]int{x + 1, y}, [2]int{x + 2, y}, 1
		}
		return [2]int{x, y + 1}, [2]int{x - 1, y + 1}, 3
	case 3:
		if opening {
			return [2]int{x, y - 1}, [2]int{x, y - 2}, 2
		}
		return [2]int{x + 1, y}, [2]int{x + 1, y + 1}, 0
	}
	return [2]int{x, y}, [2]int{x, y}, rotation & 0x3
}

// findAdjacentGatePartner finds a wooden gate's partner panel among
// the eight neighbouring tiles (Doors.plugin.js:1413-1421
// findAdjacentGatePartner), through the object manager's map lookup.
func (e *doorEngine) findAdjacentGatePartner(partnerID, x, y, z int) plugins.GameObject {
	objects := e.objectManager()
	if objects == nil {
		return nil
	}
	for _, d := range gatePartnerOffsets {
		if partner := objects.Get(partnerID, game.Location{X: x + d[0], Y: y + d[1], Z: z}); partner != nil {
			return partner
		}
	}
	return nil
}

// handleWoodenGate opens/closes a wooden gate by swinging its hinge
// and extension panels together (Doors.plugin.js:1424-1520). Both
// pieces are tracked as one open-object state so auto-close reverts
// the whole gate; the anchor is the CLOSED hinge tile so open and
// close compute the same key.
func (e *doorEngine) handleWoodenGate(p *game.Player, obj plugins.GameObject, objectID int, loc game.Location) bool {
	gate, ok := e.woodenGate(objectID)
	if !ok || obj == nil {
		return false
	}
	isClosed := objectID == gate.ClosedHinge || objectID == gate.ClosedExtension
	if !isClosed {
		// Closing a gate this plugin opened puts back exactly the
		// panels it replaced.
		for key, st := range e.states {
			if !strings.HasPrefix(key, "wooden-gate:") {
				continue
			}
			for _, cur := range st.current {
				if cur.id == objectID && cur.loc == loc {
					if tasks := e.taskManager(); tasks != nil {
						tasks.CancelTasks(key)
					}
					e.autoCloseDoor(key)
					return true
				}
			}
		}
	}
	isHinge := objectID == gate.ClosedHinge || objectID == gate.OpenHinge
	oldHingeID, oldExtensionID := gate.OpenHinge, gate.OpenExtension
	newHingeID, newExtensionID := gate.ClosedHinge, gate.ClosedExtension
	if isClosed {
		oldHingeID, oldExtensionID = gate.ClosedHinge, gate.ClosedExtension
		newHingeID, newExtensionID = gate.OpenHinge, gate.OpenExtension
	}
	partnerID := oldExtensionID
	if !isHinge {
		partnerID = oldHingeID
	}
	x, y, z := loc.X, loc.Y, loc.Z
	rotation := obj.Face() & 3
	partner := e.findAdjacentGatePartner(partnerID, x, y, z)
	if partner == nil {
		return false
	}
	clicked := doorSnapshot{id: objectID, typ: obj.Type(), face: rotation, loc: loc}
	partnerSnap := snapshotOf(partner)
	var hingeOld, extensionOld doorSnapshot
	if isHinge {
		hingeOld, extensionOld = clicked, partnerSnap
	} else {
		hingeOld, extensionOld = partnerSnap, clicked
	}
	hingeNewXY, extensionNewXY, newFace := gateHingeTransform(hingeOld.loc.X, hingeOld.loc.Y, hingeOld.face, isClosed)
	hingeNew := doorSnapshot{id: newHingeID, typ: hingeOld.typ, face: newFace, loc: game.Location{X: hingeNewXY[0], Y: hingeNewXY[1], Z: z}}
	extensionNew := doorSnapshot{id: newExtensionID, typ: extensionOld.typ, face: newFace, loc: game.Location{X: extensionNewXY[0], Y: extensionNewXY[1], Z: z}}
	if objects := e.objectManager(); objects != nil {
		objects.Deregister(hingeOld.object(), true)
		objects.Deregister(extensionOld.object(), true)
		objects.Register(hingeNew.object(), true)
		objects.Register(extensionNew.object(), true)
	}
	closedHingeX, closedHingeY := hingeOld.loc.X, hingeOld.loc.Y
	if !isClosed {
		closedHingeX, closedHingeY = hingeNewXY[0], hingeNewXY[1]
	}
	anchorKey := fmt.Sprintf("wooden-gate:%d:%s", gate.ClosedHinge, locationKey(game.Location{X: closedHingeX, Y: closedHingeY, Z: z}))
	if isClosed {
		e.rememberOpenObjects(anchorKey, []doorSnapshot{hingeOld, extensionOld}, []doorSnapshot{hingeNew, extensionNew}, gate.ClosedHinge, true)
	} else {
		e.clearOpenDoor(anchorKey)
	}
	e.playSound(p, doorSoundFor("Gate", isClosed))
	return true
}

// handleCompositeDoor opens a composite opening
// (Doors.plugin.js:1523-1538): one loc opens into the catalog's leaf
// list, each leaf relative to the closed loc's tile. A composite that
// is already open is a no-op: its open leaves carry no Open action,
// so a second swing cannot be clicked in production, and re-swinging
// would deregister leaves that are already open.
func (e *doorEngine) handleCompositeDoor(p *game.Player, obj plugins.GameObject, objectID int, defName string) bool {
	leaves, ok := e.compositeOpening(objectID)
	if !ok || obj == nil {
		return false
	}
	loc := obj.Location()
	anchorKey := locationKey(loc)
	if _, exists := e.states[anchorKey]; exists {
		return true
	}
	closed := snapshotOf(obj)
	opened := make([]doorSnapshot, 0, len(leaves))
	for _, leaf := range leaves {
		opened = append(opened, doorSnapshot{
			id:   leaf.ID,
			typ:  closed.typ,
			face: leaf.Face,
			loc:  game.Location{X: loc.X + leaf.DX, Y: loc.Y + leaf.DY, Z: loc.Z},
		})
	}
	if objects := e.objectManager(); objects != nil {
		objects.Deregister(closed.object(), true)
		for _, leaf := range opened {
			objects.Register(leaf.object(), true)
		}
	}
	e.rememberOpenObjects(anchorKey, []doorSnapshot{closed}, opened, objectID, false)
	e.playSound(p, doorSoundFor(defName, true))
	return true
}

// doubleOpenID resolves the open leaf a closed double-door leaf swings
// into (Doors.plugin.js:1759-1761 resolveOpenedDoubleDoorId): the
// special map first (the catalog's, which BuildCatalog fills from the
// transcribed table plus the catalog-discovered pairs), else the
// catalog pair, else closedId+1.
func (e *doorEngine) doubleOpenID(closedID int) int {
	if e.catalog != nil {
		if open, ok := e.catalog.SpecialDoubleDoorOpen(closedID); ok {
			return open
		}
	}
	if open, ok := e.openID(closedID); ok {
		return open
	}
	return closedID + 1
}

// findDoubleState finds the open double-door state whose current
// leaves include the clicked loc, if any.
func (e *doorEngine) findDoubleState(obj plugins.GameObject) (string, *openDoorState) {
	for key, st := range e.states {
		if len(st.closed) != 2 {
			continue
		}
		for _, cur := range st.current {
			if snapshotsMatch(cur, obj) {
				return key, st
			}
		}
	}
	return "", nil
}

// handleDoubleDoor swings both leaves of a double door together
// (Doors.plugin.js:1817-1845): the left leaf turns to
// LEFT_LEAF_OPEN_FACES[closed face], the right to
// RIGHT_LEAF_OPEN_FACES[closed face]; closing puts both back. A leaf
// only acts as a double door where both leaves stand side by side; a
// lone one falls through to the single-door swing.
func (e *doorEngine) handleDoubleDoor(p *game.Player, obj plugins.GameObject, objectID int, loc game.Location, defName string) bool {
	if obj == nil {
		return false
	}
	if key, st := e.findDoubleState(obj); st != nil {
		if objects := e.objectManager(); objects != nil {
			for _, cur := range st.current {
				objects.Deregister(cur.object(), true)
			}
			for _, closed := range st.closed {
				objects.Register(closed.object(), true)
			}
		}
		e.clearOpenDoor(key)
		e.playSound(p, doorSoundFor(defName, false))
		return true
	}
	closedID := e.resolveClosedID(obj, objectID)
	if closedID < 0 {
		// Family leaves the catalog scan never paired (e.g. Castle
		// door 24059): a family member that is not a known open leaf
		// is a closed leaf.
		if _, ok := e.doubleDoorFamily(objectID); ok {
			if closed, isOpen := e.closedID(objectID); !isOpen || closed == objectID {
				closedID = objectID
			}
		}
	}
	if closedID < 0 {
		return false
	}
	family, ok := e.doubleDoorFamily(closedID)
	if !ok {
		return false
	}
	face := obj.Face() & 3
	typ := obj.Type()
	// Find the partner leaf among the four neighbouring tiles: the
	// first family leaf standing there with the same face and type
	// (Doors.plugin.js:1706-1727 ensureDynamicDoubleDoorRecords).
	var partner plugins.GameObject
	var partnerClosedID int
search:
	for _, d := range coordOffsets {
		tile := game.Location{X: loc.X + d[0], Y: loc.Y + d[1], Z: loc.Z}
		for _, famID := range family {
			candidate := e.objectsGet(famID, tile)
			if candidate == nil || candidate.Face()&3 != face || candidate.Type() != typ {
				continue
			}
			// An open leaf standing on the tile is not a partner:
			// the reverse map resolves it to a different closed id.
			// A closed leaf the catalog never paired resolves to
			// nothing and is accepted as closed.
			if closed, isOpen := e.closedID(candidate.ID()); isOpen && closed != candidate.ID() {
				continue
			}
			partner, partnerClosedID = candidate, candidate.ID()
			break search
		}
	}
	if partner == nil {
		return false
	}
	leftOff := leftLeafOffsets[face]
	leftIsPartner := partner.Location().X-loc.X == leftOff[0] &&
		partner.Location().Y-loc.Y == leftOff[1] &&
		partnerClosedID == closedID+leftOff[2]
	rightIsPartner := loc.X-partner.Location().X == leftOff[0] &&
		loc.Y-partner.Location().Y == leftOff[1] &&
		partnerClosedID == closedID-leftOff[2]
	if !leftIsPartner && !rightIsPartner {
		return false
	}
	clickedClosed := snapshotOf(obj)
	partnerClosed := snapshotOf(partner)
	var leftClosed, rightClosed doorSnapshot
	if leftIsPartner {
		leftClosed, rightClosed = partnerClosed, clickedClosed
	} else {
		leftClosed, rightClosed = clickedClosed, partnerClosed
	}
	swingLeaf := func(closed doorSnapshot, openFaces [4]int) doorSnapshot {
		dx, dy := 0, 0
		if !e.isOwnTile(closed.id) {
			dx, dy = coordOffsets[closed.face][0], coordOffsets[closed.face][1]
		}
		return doorSnapshot{
			id:   e.doubleOpenID(closed.id),
			typ:  closed.typ,
			face: openFaces[closed.face],
			loc:  game.Location{X: closed.loc.X + dx, Y: closed.loc.Y + dy, Z: closed.loc.Z},
		}
	}
	leftOpen := swingLeaf(leftClosed, leftLeafOpenFaces)
	rightOpen := swingLeaf(rightClosed, rightLeafOpenFaces)
	if objects := e.objectManager(); objects != nil {
		objects.Deregister(leftClosed.object(), true)
		objects.Deregister(rightClosed.object(), true)
		objects.Register(leftOpen.object(), true)
		objects.Register(rightOpen.object(), true)
	}
	closedKeys := []string{
		fmt.Sprintf("%d:%s", leftClosed.id, locationKey(leftClosed.loc)),
		fmt.Sprintf("%d:%s", rightClosed.id, locationKey(rightClosed.loc)),
	}
	sort.Strings(closedKeys)
	anchorKey := "double:" + strings.Join(closedKeys, "|")
	e.rememberOpenObjects(anchorKey, []doorSnapshot{leftClosed, rightClosed}, []doorSnapshot{leftOpen, rightOpen}, leftClosed.id, false)
	e.playSound(p, doorSoundFor(defName, true))
	return true
}

// objectsGet is the map lookup for a partner leaf (MapObjects.get),
// nil-manager safe.
func (e *doorEngine) objectsGet(id int, loc game.Location) plugins.GameObject {
	objects := e.objectManager()
	if objects == nil {
		return nil
	}
	return objects.Get(id, loc)
}

// route is routeWallDoor (Doors.plugin.js:2088-2116): a closed straight
// wall door is used across its face, never from either end of the wall
// (blockAccessFlags |= 0x5); an open leaf keeps ordinary reach;
// diagonal (type 9) doors record the approach side so a swinging leaf
// never lands on the opener.
func (e *doorEngine) route(ev *plugins.ObjectRouteEvent) {
	if ev == nil || ev.Player == nil || ev.Object == nil {
		return
	}
	delete(e.diagonal, ev.Player)
	name := ""
	if ev.Definition != nil {
		name = ev.Definition.Name()
	}
	if !DoorNames[name] && !doorNamePattern.MatchString(name) {
		return
	}
	if ev.Object.Type() == 9 {
		e.diagonal[ev.Player] = diagonalApproach{object: ev.Object, source: ev.SourceLocation}
		return
	}
	if ev.Object.Type() != 0 {
		return
	}
	if isOpenLeaf(ev.Definition) {
		return
	}
	if e.isPassThroughOpenLeaf(ev.Object) {
		return
	}
	ev.BlockAccessFlags |= 0x5
}

// isOpenLeaf reports whether a definition is an open leaf: it offers
// "Close" and no "Open" (Doors.plugin.js:2118-2121 isOpenLeaf).
func isOpenLeaf(def plugins.ObjectDefinition) bool {
	if def == nil {
		return false
	}
	hasClose, hasOpen := false, false
	for _, action := range def.Interactions() {
		switch strings.ToLower(strings.TrimSpace(action)) {
		case "close":
			hasClose = true
		case "open":
			hasOpen = true
		}
	}
	return hasClose && !hasOpen
}

// isPassThroughOpenLeaf reports whether the object is an opened leaf
// of a pass-through gate (Doors.plugin.js:2110-2116
// isPassThroughOpenLeaf): it keeps its Open option but stands beside
// the doorway like any open leaf, so it keeps ordinary wall reach.
// The catalog keys pass-through gates by their closed leaves only, so
// this scans the transcribed gate table's opened leaves directly.
func (e *doorEngine) isPassThroughOpenLeaf(obj plugins.GameObject) bool {
	if obj == nil {
		return false
	}
	loc := obj.Location()
	for _, gate := range passThroughGates {
		for _, leaf := range gate.Opened {
			if leaf.ID == obj.ID() && leaf.X == loc.X && leaf.Y == loc.Y && leaf.Z == loc.Z {
				return true
			}
		}
	}
	return false
}

// requestOpen is requestDoorOpen (Doors.plugin.js:2046-2075): key-on-
// object and other content request an Open, never toggle an open door
// shut. A pass-through gate takes the request first (it may have no
// "Open" of its own); an already-open door is reported handled; else
// the click type is the "Open" option's index + 1 and the request runs
// through toggle (which emits door:toggle again, as upstream does).
func (e *doorEngine) requestOpen(ev *DoorOpenEvent) {
	if ev == nil || ev.Handled || ev.Player == nil || ev.Object == nil {
		return
	}
	p, obj := ev.Player, ev.Object
	objectID := obj.ID()
	loc := obj.Location()
	if e.handlePassThroughGate(p, objectID, loc, obj) {
		ev.Handled = true
		return
	}
	for _, st := range e.states {
		for _, cur := range st.current {
			if cur.id == objectID && cur.loc == loc {
				ev.Handled = true
				return
			}
		}
	}
	clickType := 0
	if ev.Definition != nil {
		for i, action := range ev.Definition.Interactions() {
			if strings.ToLower(strings.TrimSpace(action)) == "open" {
				clickType = i + 1
				break
			}
		}
	}
	if clickType < 1 {
		return
	}
	src := p.Location()
	ev.Handled = e.toggle(&plugins.ObjectInteractionEvent{
		Player:         p,
		Object:         obj,
		Definition:     ev.Definition,
		ObjectID:       objectID,
		ClickType:      clickType,
		Location:       loc,
		SourceLocation: &src,
	})
}

// readState is readDoorState (Doors.plugin.js:2078-2084): the shared
// world's current leaves, read without changing the door or its
// auto-close timer. Requested closed leaves that are currently open
// are replaced by their open leaves.
func (e *doorEngine) readState(ev *DoorStateEvent) {
	if ev == nil {
		return
	}
	current := append([]plugins.GameObject(nil), ev.Objects...)
	for _, st := range e.states {
		var replaced []plugins.GameObject
		for _, obj := range ev.Objects {
			for _, snap := range st.closed {
				if snapshotsMatch(snap, obj) {
					replaced = append(replaced, obj)
					break
				}
			}
		}
		if len(replaced) == 0 {
			continue
		}
		kept := current[:0]
		for _, obj := range current {
			drop := false
			for _, r := range replaced {
				if obj == r {
					drop = true
					break
				}
			}
			if !drop {
				kept = append(kept, obj)
			}
		}
		current = kept
		for _, snap := range st.current {
			current = append(current, snap.object())
		}
	}
	ev.Objects = current
}
