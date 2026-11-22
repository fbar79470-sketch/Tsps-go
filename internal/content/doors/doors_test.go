package doors

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fakeSession is a minimal game.SessionPort that records sound-effect
// sends, standing in for the session layer in hub tests (copied from
// the WildernessDitch test pattern).
type fakeSession struct {
	effects []soundEffectCall
}

type soundEffectCall struct {
	soundID, loopType, delay, volume int
}

func (f *fakeSession) SendPacket(packet []byte) {}
func (f *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (f *fakeSession) SendDestination(x, y int) {}
func (f *fakeSession) SendVarp(id, value int)   {}
func (f *fakeSession) FlushTick(cycle int64) error {
	return nil
}
func (f *fakeSession) Close(code int, reason string) {}
func (f *fakeSession) IsOpen() bool                  { return true }
func (f *fakeSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	f.effects = append(f.effects, soundEffectCall{soundID, loopType, delay, volume})
}
func (f *fakeSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}

// fakeObjectDef is a minimal plugins.ObjectDefinition with a chosen
// name and interactions.
type fakeObjectDef struct {
	id           int
	name         string
	interactions []string
}

func (d fakeObjectDef) ID() int                { return d.id }
func (d fakeObjectDef) Name() string           { return d.name }
func (d fakeObjectDef) SizeX() int             { return 1 }
func (d fakeObjectDef) SizeY() int             { return 1 }
func (d fakeObjectDef) BlockingMask() int      { return 0 }
func (d fakeObjectDef) Interactions() []string { return d.interactions }

func doorDef(id int, name string) fakeObjectDef {
	return fakeObjectDef{id: id, name: name, interactions: []string{"Open", "Close", "", "", ""}}
}

func closeOnlyDef(id int, name string) fakeObjectDef {
	return fakeObjectDef{id: id, name: name, interactions: []string{"Close", "", "", "", ""}}
}

// swapCall records one object-manager Register/Deregister call.
type swapCall struct {
	op            string // "register" or "deregister"
	id, typ, face int
	loc           game.Location
}

// fakeObjectManager is a minimal plugins.ObjectManagerAPI recording
// swaps, with seeded map originals for the Get lookup double doors
// and wooden gates resolve their partners through.
type fakeObjectManager struct {
	calls      []swapCall
	registered []plugins.GameObject
	seeded     map[game.Location][]plugins.GameObject
}

func newFakeObjectManager() *fakeObjectManager {
	return &fakeObjectManager{seeded: make(map[game.Location][]plugins.GameObject)}
}

func (m *fakeObjectManager) seed(obj plugins.GameObject) {
	loc := obj.Location()
	m.seeded[loc] = append(m.seeded[loc], obj)
}

func (m *fakeObjectManager) Register(obj plugins.GameObject, playerUpdate bool) {
	m.calls = append(m.calls, swapCall{"register", obj.ID(), obj.Type(), obj.Face(), obj.Location()})
	m.registered = append(m.registered, obj)
}

func (m *fakeObjectManager) Deregister(obj plugins.GameObject, playerUpdate bool) {
	m.calls = append(m.calls, swapCall{"deregister", obj.ID(), obj.Type(), obj.Face(), obj.Location()})
	kept := m.registered[:0]
	for _, r := range m.registered {
		if r.ID() == obj.ID() && r.Location() == obj.Location() && r.Type() == obj.Type() && r.Face() == obj.Face() {
			continue
		}
		kept = append(kept, r)
	}
	m.registered = kept
}

func (m *fakeObjectManager) ReplaceInPlace(outgoing, incoming plugins.GameObject) {
	m.calls = append(m.calls, swapCall{"replace", incoming.ID(), incoming.Type(), incoming.Face(), incoming.Location()})
}

func (m *fakeObjectManager) ObjectsAt(loc game.Location) []plugins.GameObject {
	var out []plugins.GameObject
	for _, r := range m.registered {
		if r.Location() == loc {
			out = append(out, r)
		}
	}
	return out
}

func (m *fakeObjectManager) ObjectsInRegion(regionX, regionY int) []plugins.GameObject {
	return append([]plugins.GameObject(nil), m.registered...)
}

func (m *fakeObjectManager) Exists(id int, loc game.Location) bool {
	return len(m.ObjectsAt(loc)) > 0
}

func (m *fakeObjectManager) Get(id int, loc game.Location) plugins.GameObject {
	for _, obj := range m.seeded[loc] {
		if obj.ID() == id {
			return obj
		}
	}
	for _, obj := range m.registered {
		if obj.ID() == id && obj.Location() == loc {
			return obj
		}
	}
	return nil
}

func (m *fakeObjectManager) Find(player *game.Player, id int, loc game.Location) plugins.GameObject {
	return m.Get(id, loc)
}

func (m *fakeObjectManager) StillStands(player *game.Player, id int, loc game.Location, obj plugins.GameObject) bool {
	return m.Get(id, loc) != nil
}

func (m *fakeObjectManager) RemovePluginObjects() int { return 0 }
func (m *fakeObjectManager) RemovedObjects() []plugins.GameObject {
	return nil
}

// hasSwap reports whether the manager recorded the given swap call.
func (m *fakeObjectManager) hasSwap(op string, id int, loc game.Location, face int) bool {
	for _, c := range m.calls {
		if c.op == op && c.id == id && c.loc == loc && c.face == face {
			return true
		}
	}
	return false
}

// emitCall records one custom-event emit.
type emitCall struct {
	name    string
	payload any
}

// emitRecorder records custom-event emits, optionally claiming
// door:toggle requests.
type emitRecorder struct {
	calls       []emitCall
	claimToggle bool
}

func (r *emitRecorder) emit(name string, payload any) {
	r.calls = append(r.calls, emitCall{name, payload})
	if r.claimToggle {
		if req, ok := payload.(*DoorToggleEvent); ok {
			req.Handled = true
		}
	}
}

func (r *emitRecorder) toggles() int {
	n := 0
	for _, c := range r.calls {
		if c.name == "door:toggle" {
			n++
		}
	}
	return n
}

func newTestPlayer(name string, x, y, z int) (*game.Player, *fakeSession) {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: z})
	sess := &fakeSession{}
	p.Session = sess
	return p, sess
}

func newTestEngine(mgr *fakeObjectManager, tasks *game.TaskManager, rec *emitRecorder, catalog *Catalog) *doorEngine {
	return newDoorEngine(mgr, tasks, rec.emit, catalog)
}

// clickEvent builds an Open/Close click on a loc at loc with the
// given shape and face, from a player standing at src.
func clickEvent(p *game.Player, id int, loc game.Location, typ, face int, def fakeObjectDef, clickType int) *plugins.ObjectInteractionEvent {
	obj := game.NewGameObject(id, loc, typ, face, nil)
	src := p.Location()
	return &plugins.ObjectInteractionEvent{
		Player:         p,
		Object:         obj,
		Definition:     def,
		ObjectID:       id,
		ClickType:      clickType,
		Option:         "Open",
		HasOption:      true,
		Location:       loc,
		SourceLocation: &src,
	}
}

// withTableState populates a test Catalog literal's instance tables
// from the transcribed package tables, mirroring what BuildCatalog
// does for them (wooden gates, double-door families / partners /
// special open ids). Engine tests construct Catalog literals directly
// and never depend on BuildCatalog; without this the literal's
// WoodenGate / DoubleDoorFamily lookups would miss.
func withTableState(c *Catalog) *Catalog {
	c.woodenGates = map[int]*WoodenGate{}
	for i := range woodenGates {
		gate := woodenGates[i]
		for _, id := range []int{gate.ClosedHinge, gate.ClosedExtension, gate.OpenHinge, gate.OpenExtension} {
			if _, ok := c.woodenGates[id]; !ok {
				g := gate
				c.woodenGates[id] = &g
			}
		}
	}
	c.doubleDoorFamilies = map[int][]int{}
	for _, family := range doubleDoorIDFamilies {
		ids := append([]int(nil), family...)
		for _, id := range ids {
			c.doubleDoorFamilies[id] = ids
		}
	}
	c.doubleDoorPartners = map[int][]int{}
	for _, pair := range specialDoubleDoorPairs {
		a, b := pair[0], pair[1]
		c.doubleDoorPartners[a] = append(c.doubleDoorPartners[a], b)
		c.doubleDoorPartners[b] = append(c.doubleDoorPartners[b], a)
	}
	c.specialDoubleDoorOpen = map[int]int{}
	for closed, open := range specialDoubleDoorOpenIDsByClosedID {
		c.specialDoubleDoorOpen[closed] = open
	}
	return c
}

// singleCatalog builds a Catalog literal with one closed->open pair
// (the §3 seam: engine tests construct Catalog literals directly and
// never depend on BuildCatalog).
func singleCatalog(closed, open int) *Catalog {
	return &Catalog{
		ClosedToOpen: map[int]int{closed: open},
		OpenToClosed: map[int]int{open: closed},
	}
}

func runTicks(t *testing.T, tasks *game.TaskManager, n int) {
	t.Helper()
	for cycle := int64(1); cycle <= int64(n); cycle++ {
		tasks.Process(cycle)
	}
}

// TestPluginMetadata pins the registered name and description against
// the plugin file (Doors.plugin.js module.exports).
func TestPluginMetadata(t *testing.T) {
	if Plugin.Name != "Doors" {
		t.Errorf("Plugin.Name = %q, want %q", Plugin.Name, "Doors")
	}
	want := "Opens and closes doors, gates and double doors, swapping between closed and open locs, with route handling and reapplied state on region load."
	if Plugin.Description != want {
		t.Errorf("Plugin.Description = %q, want %q", Plugin.Description, want)
	}
}

// TestToggleSingleDoorOpenClose drives the live Lumbridge single door
// 1535 -> 1536 at (3225,3240,0) face 2 through the engine: opening
// swaps the leaf one tile along the closed face (to (3226,3240,0),
// turned +1 to face 3) with the door-open sound, and closing swaps it
// back with the door-close sound and cancels the auto-close timer.
func TestToggleSingleDoorOpenClose(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, sess := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	def := doorDef(1535, "Door")
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, def, 1)) {
		t.Fatal("Open 1535: want handled")
	}
	if !mgr.hasSwap("deregister", 1535, loc, 2) {
		t.Fatalf("swaps = %+v, want deregister 1535 at %+v face 2", mgr.calls, loc)
	}
	if !mgr.hasSwap("register", 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want register 1536 at (3226,3240,0) face 3", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 62 {
		t.Fatalf("sound effects = %+v, want one door-open (62)", sess.effects)
	}
	if !tasks.HasActiveTask(locationKey(loc), "doors:auto-close") {
		t.Fatal("after opening: want an auto-close task keyed by the closed tile")
	}
	// Close the open leaf.
	if !e.toggle(clickEvent(p, 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 0, 3, closeOnlyDef(1536, "Door"), 1)) {
		t.Fatal("Close 1536: want handled")
	}
	if !mgr.hasSwap("deregister", 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want deregister 1536", mgr.calls)
	}
	if !mgr.hasSwap("register", 1535, loc, 2) {
		t.Fatalf("swaps = %+v, want register 1535 back at %+v face 2", mgr.calls, loc)
	}
	if len(sess.effects) != 2 || sess.effects[1].soundID != 60 {
		t.Fatalf("sound effects = %+v, want second sound door-close (60)", sess.effects)
	}
	if tasks.HasActiveTask(locationKey(loc), "doors:auto-close") {
		t.Fatal("after manual close: want the auto-close task cancelled")
	}
}

// TestLazySurfaceResolution is the Phase 48 live-verification regression:
// the object manager and task manager are installed on the hub AFTER
// plugin Load (hub.SetObjectManager runs after newPluginHub in
// cmd/server/server.go; the TaskManager arrives with InstallSkillHooks),
// so capturing them at Register time read nil live and every swap
// silently no-op'd while the sound still played. The engine resolves
// both lazily per use: resolvers that answer nil until the install,
// then the real surfaces, must still produce the swap and arm the
// auto-close timer on the first toggle after the install.
func TestLazySurfaceResolution(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newDoorEngine(nil, nil, rec.emit, singleCatalog(1535, 1536))
	installed := false
	e.resolveObjects = func() plugins.ObjectManagerAPI {
		if !installed {
			return nil
		}
		return mgr
	}
	e.resolveTasks = func() *game.TaskManager {
		if !installed {
			return nil
		}
		return tasks
	}
	// The install lands after Register, as it does at boot.
	installed = true
	p, sess := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	def := doorDef(1535, "Door")
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, def, 1)) {
		t.Fatal("Open 1535 after install: want handled")
	}
	if !mgr.hasSwap("deregister", 1535, loc, 2) {
		t.Fatalf("swaps = %+v, want deregister 1535 at %+v face 2", mgr.calls, loc)
	}
	if !mgr.hasSwap("register", 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want register 1536 at (3226,3240,0) face 3", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 62 {
		t.Fatalf("sound effects = %+v, want one door-open (62)", sess.effects)
	}
	if !tasks.HasActiveTask(locationKey(loc), "doors:auto-close") {
		t.Fatal("after opening: want an auto-close task keyed by the closed tile")
	}
}

// TestAutoClose opens a door and runs 500 ticks: the auto-close task
// reverts the leaf to its closed snapshot.
func TestAutoClose(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, doorDef(1535, "Door"), 1)) {
		t.Fatal("Open 1535: want handled")
	}
	runTicks(t, tasks, 499)
	if mgr.hasSwap("register", 1535, loc, 2) {
		t.Fatal("after 499 ticks: door reverted early")
	}
	runTicks(t, tasks, 1)
	if !mgr.hasSwap("register", 1535, loc, 2) {
		t.Fatalf("after 500 ticks: swaps = %+v, want the closed leaf registered back", mgr.calls)
	}
	if len(e.states) != 0 {
		t.Fatalf("after auto-close: states = %d, want 0", len(e.states))
	}
}

// TestReopenResetsAutoClose pins the timer reset: re-opening after a
// manual close arms a fresh 500 ticks, not the remainder of the first.
func TestReopenResetsAutoClose(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	def := doorDef(1535, "Door")
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, def, 1)) {
		t.Fatal("Open 1535: want handled")
	}
	runTicks(t, tasks, 400)
	if !e.toggle(clickEvent(p, 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 0, 3, closeOnlyDef(1536, "Door"), 1)) {
		t.Fatal("Close 1536: want handled")
	}
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, def, 1)) {
		t.Fatal("Open 1535 again: want handled")
	}
	runTicks(t, tasks, 400)
	if len(e.states) != 1 {
		t.Fatalf("400 ticks after re-open: states = %d, want 1 (timer reset)", len(e.states))
	}
	runTicks(t, tasks, 100)
	if len(e.states) != 0 {
		t.Fatalf("500 ticks after re-open: states = %d, want 0", len(e.states))
	}
}

// TestDoorToggleHandledShortCircuit pins the door:toggle guard: a
// listener that sets Handled claims the click and the generic swing
// never runs — no swaps, no sound.
func TestDoorToggleHandledShortCircuit(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{claimToggle: true}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, sess := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, doorDef(1535, "Door"), 1)) {
		t.Fatal("handled toggle: want claimed (true)")
	}
	if len(mgr.calls) != 0 {
		t.Fatalf("swaps = %+v, want none", mgr.calls)
	}
	if len(sess.effects) != 0 {
		t.Fatalf("sound effects = %+v, want none", sess.effects)
	}
	if rec.toggles() != 1 {
		t.Fatalf("door:toggle emits = %d, want 1", rec.toggles())
	}
}

// TestDorgeshRefusal pins the Dorgesh-Kaan refusal
// (Doors.plugin.js:2033): those two ids return false before any
// emit, so LocTeleports owns them.
func TestDorgeshRefusal(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(6919, 6921))
	p, sess := newTestPlayer("alice", 3000, 3000, 0)
	loc := game.Location{X: 3000, Y: 3001, Z: 0}
	if e.toggle(clickEvent(p, 6919, loc, 0, 2, doorDef(6919, "Door"), 1)) {
		t.Fatal("Dorgesh door 6919: want NOT handled (false)")
	}
	if len(mgr.calls) != 0 || len(sess.effects) != 0 || len(rec.calls) != 0 {
		t.Fatalf("swaps = %+v, sounds = %+v, emits = %+v, want none", mgr.calls, sess.effects, rec.calls)
	}
	// The second Dorgesh id, via the definition id.
	if e.toggle(clickEvent(p, 6920, loc, 0, 2, doorDef(6920, "Door"), 1)) {
		t.Fatal("Dorgesh door 6920: want NOT handled (false)")
	}
}

// TestInPlaceDoor pins the in-place case (Doors.plugin.js:213): the
// Pollnivneach curtain 1533 -> 1534 replaces the model on the same
// tile, face unchanged.
func TestInPlaceDoor(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1533, 1534))
	p, _ := newTestPlayer("alice", 3220, 3220, 0)
	loc := game.Location{X: 3220, Y: 3221, Z: 0}
	if !e.toggle(clickEvent(p, 1533, loc, 0, 1, doorDef(1533, "Curtain"), 1)) {
		t.Fatal("Open curtain 1533: want handled")
	}
	if !mgr.hasSwap("register", 1534, loc, 1) {
		t.Fatalf("swaps = %+v, want register 1534 on the same tile face 1", mgr.calls)
	}
	if !e.toggle(clickEvent(p, 1534, loc, 0, 1, closeOnlyDef(1534, "Curtain"), 1)) {
		t.Fatal("Close curtain 1534: want handled")
	}
	if !mgr.hasSwap("register", 1533, loc, 1) {
		t.Fatalf("swaps = %+v, want register 1533 back on the same tile", mgr.calls)
	}
}

// TestSelfOpeningDoor pins the self-opening case: O.DOOR_67 (2184)
// keeps its id and tile, tracked by state only — the face turns +1 on
// open and back on close. (The Ranging Guild door 11665 is
// self-opening too, but it is also a pass-through gate, so the
// pass-through path takes its clicks first.)
func TestSelfOpeningDoor(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(2184, 2184))
	p, sess := newTestPlayer("alice", 3000, 3000, 0)
	loc := game.Location{X: 3000, Y: 3001, Z: 0}
	if !e.toggle(clickEvent(p, 2184, loc, 0, 0, doorDef(2184, "Door"), 1)) {
		t.Fatal("Open 2184: want handled")
	}
	if !mgr.hasSwap("register", 2184, loc, 1) {
		t.Fatalf("swaps = %+v, want register 2184 on the same tile face 1", mgr.calls)
	}
	if len(e.states) != 1 {
		t.Fatalf("states = %d, want 1 (self-opening is tracked by state only)", len(e.states))
	}
	if !e.toggle(clickEvent(p, 2184, loc, 0, 1, closeOnlyDef(2184, "Door"), 1)) {
		t.Fatal("Close 2184: want handled")
	}
	if !mgr.hasSwap("register", 2184, loc, 0) {
		t.Fatalf("swaps = %+v, want register 2184 back face 0", mgr.calls)
	}
	if len(sess.effects) != 2 || sess.effects[0].soundID != 62 || sess.effects[1].soundID != 60 {
		t.Fatalf("sound effects = %+v, want door-open then door-close", sess.effects)
	}
}

// TestOwnTileDoor pins the own-tile case: Ardougne's 4636 -> 4639
// turns on the closed tile instead of stepping along its face.
func TestOwnTileDoor(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(4636, 4639))
	p, _ := newTestPlayer("alice", 2570, 3270, 0)
	loc := game.Location{X: 2570, Y: 3271, Z: 0}
	if !e.toggle(clickEvent(p, 4636, loc, 0, 0, doorDef(4636, "Door"), 1)) {
		t.Fatal("Open 4636: want handled")
	}
	if !mgr.hasSwap("register", 4639, loc, 1) {
		t.Fatalf("swaps = %+v, want register 4639 on the closed tile face 1", mgr.calls)
	}
}

// TestDoubleDoorPairSwing drives the live Lumbridge double door
// 1521 + 1524 at (3244,3216,0) + (3243,3216,0) face 3: both leaves
// swing together (left 1524 -> 1525 face 0, right 1521 -> 1522 face 2,
// each a tile along the closed face), and closing puts both back.
func TestDoubleDoorPairSwing(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	catalog := withTableState(&Catalog{
		ClosedToOpen: map[int]int{1521: 1522, 1524: 1525},
		OpenToClosed: map[int]int{1522: 1521, 1525: 1524},
	})
	e := newTestEngine(mgr, tasks, rec, catalog)
	mgr.seed(game.NewGameObject(1524, game.Location{X: 3243, Y: 3216, Z: 0}, 0, 3, nil))
	p, sess := newTestPlayer("alice", 3244, 3215, 0)
	loc := game.Location{X: 3244, Y: 3216, Z: 0}
	if !e.toggle(clickEvent(p, 1521, loc, 0, 3, doorDef(1521, "Large door"), 1)) {
		t.Fatal("Open double door 1521: want handled")
	}
	if !mgr.hasSwap("register", 1525, game.Location{X: 3243, Y: 3215, Z: 0}, 0) {
		t.Fatalf("swaps = %+v, want left leaf 1525 at (3243,3215,0) face 0", mgr.calls)
	}
	if !mgr.hasSwap("register", 1522, game.Location{X: 3244, Y: 3215, Z: 0}, 2) {
		t.Fatalf("swaps = %+v, want right leaf 1522 at (3244,3215,0) face 2", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 62 {
		t.Fatalf("sound effects = %+v, want one door-open (62)", sess.effects)
	}
	if !e.toggle(clickEvent(p, 1522, game.Location{X: 3244, Y: 3215, Z: 0}, 0, 2, closeOnlyDef(1522, "Large door"), 1)) {
		t.Fatal("Close double door 1522: want handled")
	}
	if !mgr.hasSwap("register", 1521, loc, 3) || !mgr.hasSwap("register", 1524, game.Location{X: 3243, Y: 3216, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want both leaves back on their closed tiles face 3", mgr.calls)
	}
	if len(e.states) != 0 {
		t.Fatalf("states = %d, want 0 after closing", len(e.states))
	}
}

// TestDoubleDoorLoneLeafFallsBack pins the lone-leaf rule: a double
// door leaf with no partner standing beside it stays a single door
// and swings on its own.
func TestDoubleDoorLoneLeafFallsBack(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	catalog := withTableState(&Catalog{
		ClosedToOpen: map[int]int{1521: 1522, 1524: 1525},
		OpenToClosed: map[int]int{1522: 1521, 1525: 1524},
	})
	e := newTestEngine(mgr, tasks, rec, catalog)
	p, _ := newTestPlayer("alice", 3244, 3215, 0)
	loc := game.Location{X: 3244, Y: 3216, Z: 0}
	if !e.toggle(clickEvent(p, 1521, loc, 0, 3, doorDef(1521, "Large door"), 1)) {
		t.Fatal("Open lone leaf 1521: want handled")
	}
	if !mgr.hasSwap("register", 1522, game.Location{X: 3244, Y: 3215, Z: 0}, 0) {
		t.Fatalf("swaps = %+v, want single swing 1522 at (3244,3215,0) face 0", mgr.calls)
	}
	if mgr.hasSwap("register", 1525, game.Location{X: 3243, Y: 3215, Z: 0}, 0) {
		t.Fatalf("swaps = %+v, want no partner leaf swung", mgr.calls)
	}
}

// TestWoodenGateBothPanels drives the live Lumbridge wooden gate
// 12986 + 12987 at (3213,3261,0) + (3213,3262,0) face 0: opening
// moves BOTH panels (hinge 12986 -> 12988 at (3212,3261,0) face 3,
// extension 12987 -> 12989 at (3211,3261,0) face 3) with the gate-open
// sound, and closing puts both back.
func TestWoodenGateBothPanels(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, withTableState(&Catalog{}))
	mgr.seed(game.NewGameObject(12987, game.Location{X: 3213, Y: 3262, Z: 0}, 0, 0, nil))
	p, sess := newTestPlayer("alice", 3213, 3260, 0)
	loc := game.Location{X: 3213, Y: 3261, Z: 0}
	if !e.toggle(clickEvent(p, 12986, loc, 0, 0, doorDef(12986, "Gate"), 1)) {
		t.Fatal("Open wooden gate 12986: want handled")
	}
	if !mgr.hasSwap("register", 12988, game.Location{X: 3212, Y: 3261, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want hinge 12988 at (3212,3261,0) face 3", mgr.calls)
	}
	if !mgr.hasSwap("register", 12989, game.Location{X: 3211, Y: 3261, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want extension 12989 at (3211,3261,0) face 3", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 67 {
		t.Fatalf("sound effects = %+v, want one gate-open (67)", sess.effects)
	}
	if !e.toggle(clickEvent(p, 12988, game.Location{X: 3212, Y: 3261, Z: 0}, 0, 3, closeOnlyDef(12988, "Gate"), 1)) {
		t.Fatal("Close wooden gate 12988: want handled")
	}
	if !mgr.hasSwap("register", 12986, loc, 0) || !mgr.hasSwap("register", 12987, game.Location{X: 3213, Y: 3262, Z: 0}, 0) {
		t.Fatalf("swaps = %+v, want both panels back on their closed tiles face 0", mgr.calls)
	}
	if len(e.states) != 0 {
		t.Fatalf("states = %d, want 0 after closing", len(e.states))
	}
}

// TestWoodenGateNoPartnerStaysShut pins the partner guard: a wooden
// gate panel with no partner beside it does not swing.
func TestWoodenGateNoPartnerStaysShut(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, withTableState(&Catalog{}))
	p, _ := newTestPlayer("alice", 3213, 3260, 0)
	loc := game.Location{X: 3213, Y: 3261, Z: 0}
	if e.toggle(clickEvent(p, 12986, loc, 0, 0, doorDef(12986, "Gate"), 1)) {
		t.Fatal("Open partnerless gate 12986: want NOT handled")
	}
	if len(mgr.calls) != 0 {
		t.Fatalf("swaps = %+v, want none", mgr.calls)
	}
}

// TestGateHingeTransform pins gateHingeTransform
// (Doors.plugin.js:1391-1411) for all four rotations, opening and
// closing.
func TestGateHingeTransform(t *testing.T) {
	cases := []struct {
		name                     string
		x, y, rotation           int
		opening                  bool
		wantHinge, wantExtension [2]int
		wantFace                 int
	}{
		{"face 0 opening", 10, 10, 0, true, [2]int{9, 10}, [2]int{8, 10}, 3},
		{"face 0 closing", 10, 10, 0, false, [2]int{10, 9}, [2]int{11, 9}, 1},
		{"face 1 opening", 10, 10, 1, true, [2]int{10, 11}, [2]int{10, 12}, 0},
		{"face 1 closing", 10, 10, 1, false, [2]int{9, 10}, [2]int{9, 9}, 2},
		{"face 2 opening", 10, 10, 2, true, [2]int{11, 10}, [2]int{12, 10}, 1},
		{"face 2 closing", 10, 10, 2, false, [2]int{10, 11}, [2]int{9, 11}, 3},
		{"face 3 opening", 10, 10, 3, true, [2]int{10, 9}, [2]int{10, 8}, 2},
		{"face 3 closing", 10, 10, 3, false, [2]int{11, 10}, [2]int{11, 11}, 0},
	}
	for _, c := range cases {
		hinge, ext, face := gateHingeTransform(c.x, c.y, c.rotation, c.opening)
		if hinge != c.wantHinge || ext != c.wantExtension || face != c.wantFace {
			t.Errorf("%s: gateHingeTransform = (%v, %v, %d), want (%v, %v, %d)",
				c.name, hinge, ext, face, c.wantHinge, c.wantExtension, c.wantFace)
		}
	}
}

// TestCompositeDoor pins the composite opening: the Tree Gnome
// Stronghold main gate 190 opens into two leaves (191 on its own tile
// face 2, 192 three tiles east face 0).
func TestCompositeDoor(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, &Catalog{})
	p, sess := newTestPlayer("alice", 2000, 2000, 0)
	loc := game.Location{X: 2000, Y: 2001, Z: 0}
	if !e.toggle(clickEvent(p, 190, loc, 10, 0, doorDef(190, "Gate"), 1)) {
		t.Fatal("Open composite gate 190: want handled")
	}
	if !mgr.hasSwap("register", 191, loc, 2) {
		t.Fatalf("swaps = %+v, want leaf 191 on the closed tile face 2", mgr.calls)
	}
	if !mgr.hasSwap("register", 192, game.Location{X: 2003, Y: 2001, Z: 0}, 0) {
		t.Fatalf("swaps = %+v, want leaf 192 three tiles east face 0", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 67 {
		t.Fatalf("sound effects = %+v, want one gate-open (67)", sess.effects)
	}
}

// TestRouteClosedWallDoorFlags pins routeWallDoor
// (Doors.plugin.js:2088-2116): a closed straight wall door gets
// blockAccessFlags |= 0x5; an open leaf keeps ordinary reach; a
// diagonal (type 9) door records the approach side instead; a
// non-door name is ignored.
func TestRouteClosedWallDoorFlags(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}

	closed := &plugins.ObjectRouteEvent{
		Player: p, Object: game.NewGameObject(1535, loc, 0, 2, nil),
		Definition: doorDef(1535, "Door"), ObjectID: 1535,
		SourceLocation: p.Location(),
	}
	e.route(closed)
	if closed.BlockAccessFlags != 0x5 {
		t.Fatalf("closed wall door: BlockAccessFlags = %#x, want 0x5", closed.BlockAccessFlags)
	}
	// An existing flag is preserved, not replaced.
	closed.BlockAccessFlags = 0x2
	e.route(closed)
	if closed.BlockAccessFlags != 0x7 {
		t.Fatalf("closed wall door with flag 0x2: BlockAccessFlags = %#x, want 0x7", closed.BlockAccessFlags)
	}

	open := &plugins.ObjectRouteEvent{
		Player: p, Object: game.NewGameObject(1536, game.Location{X: 3226, Y: 3240, Z: 0}, 0, 3, nil),
		Definition: closeOnlyDef(1536, "Door"), ObjectID: 1536,
		SourceLocation: p.Location(),
	}
	e.route(open)
	if open.BlockAccessFlags != 0 {
		t.Fatalf("open leaf: BlockAccessFlags = %#x, want 0 (ordinary reach)", open.BlockAccessFlags)
	}

	diag := &plugins.ObjectRouteEvent{
		Player: p, Object: game.NewGameObject(11665, game.Location{X: 2658, Y: 3438, Z: 0}, 9, 0, nil),
		Definition: doorDef(11665, "Guild door"), ObjectID: 11665,
		SourceLocation: p.Location(),
	}
	e.route(diag)
	if diag.BlockAccessFlags != 0 {
		t.Fatalf("diagonal door: BlockAccessFlags = %#x, want 0", diag.BlockAccessFlags)
	}
	if approach, ok := e.diagonal[p]; !ok || approach.source != p.Location() {
		t.Fatalf("diagonal door: approach recorded = %+v, want source %+v", approach, p.Location())
	}

	other := &plugins.ObjectRouteEvent{
		Player: p, Object: game.NewGameObject(1234, loc, 0, 2, nil),
		Definition: doorDef(1234, "Table"), ObjectID: 1234,
		SourceLocation: p.Location(),
	}
	e.route(other)
	if other.BlockAccessFlags != 0 {
		t.Fatalf("non-door: BlockAccessFlags = %#x, want 0", other.BlockAccessFlags)
	}
}

// TestDiagonalDoorNudge pins the type-9 approach-side rule
// (Doors.plugin.js:1580-1600): when the open leaf would land on the
// opener, the opener is moved aside first and the door still swings.
func TestDiagonalDoorNudge(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(5000, 5001))
	// The player stands on the tile the leaf would swing onto.
	p, _ := newTestPlayer("alice", 10, 11, 0)
	loc := game.Location{X: 10, Y: 10, Z: 0}
	if !e.toggle(clickEvent(p, 5000, loc, 9, 0, doorDef(5000, "Door"), 1)) {
		t.Fatal("Open diagonal door 5000: want handled")
	}
	if p.Location() != (game.Location{X: 9, Y: 11, Z: 0}) {
		t.Fatalf("player location = %+v, want (9,11,0) — moved off the leaf's landing tile", p.Location())
	}
	if !mgr.hasSwap("register", 5001, game.Location{X: 10, Y: 11, Z: 0}, 1) {
		t.Fatalf("swaps = %+v, want open leaf 5001 at (10,11,0) face 1", mgr.calls)
	}
}

// TestDoorOpenEvent pins requestDoorOpen (Doors.plugin.js:2046-2075):
// a door:open request swings a shut door and reports handled; a
// second request on the already-open door reports handled without
// swinging it shut; a loc with no "Open" option is not handled.
func TestDoorOpenEvent(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	ev := &DoorOpenEvent{Player: p, Object: game.NewGameObject(1535, loc, 0, 2, nil), Definition: doorDef(1535, "Door")}
	e.requestOpen(ev)
	if !ev.Handled {
		t.Fatal("door:open on a shut door: want handled")
	}
	if !mgr.hasSwap("register", 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want the door opened", mgr.calls)
	}
	calls := len(mgr.calls)
	ev2 := &DoorOpenEvent{Player: p, Object: game.NewGameObject(1536, game.Location{X: 3226, Y: 3240, Z: 0}, 0, 3, nil), Definition: closeOnlyDef(1536, "Door")}
	e.requestOpen(ev2)
	if !ev2.Handled {
		t.Fatal("door:open on an open door: want handled (already open)")
	}
	if len(mgr.calls) != calls {
		t.Fatalf("swaps after second request = %+v, want no further swaps", mgr.calls)
	}
	ev3 := &DoorOpenEvent{Player: p, Object: game.NewGameObject(9999, loc, 0, 2, nil), Definition: closeOnlyDef(9999, "Door")}
	e.requestOpen(ev3)
	if ev3.Handled {
		t.Fatal("door:open on a loc with no Open option: want NOT handled")
	}
}

// TestDoorStateEvent pins readDoorState (Doors.plugin.js:2078-2084):
// a requested closed leaf that is currently open is replaced by its
// open leaf; a shut door reads back unchanged.
func TestDoorStateEvent(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	closed := game.NewGameObject(1535, loc, 0, 2, nil)
	ev := &DoorStateEvent{Objects: []plugins.GameObject{closed}}
	e.readState(ev)
	if len(ev.Objects) != 1 || ev.Objects[0] != plugins.GameObject(closed) {
		t.Fatalf("shut door state = %+v, want the closed leaf back", ev.Objects)
	}
	if !e.toggle(clickEvent(p, 1535, loc, 0, 2, doorDef(1535, "Door"), 1)) {
		t.Fatal("Open 1535: want handled")
	}
	ev2 := &DoorStateEvent{Objects: []plugins.GameObject{closed}}
	e.readState(ev2)
	if len(ev2.Objects) != 1 || ev2.Objects[0].ID() != 1536 || ev2.Objects[0].Location() != (game.Location{X: 3226, Y: 3240, Z: 0}) {
		t.Fatalf("open door state = %+v, want the open leaf 1536 at (3226,3240,0)", ev2.Objects)
	}
}

// TestHubToggleFlow drives one Open through the hub like the
// WildernessDitch test: the named registration claims the click.
func TestHubToggleFlow(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{MembersWorld: true})
	hub.SetObjectManager(mgr)
	hub.SetTaskManager(tasks)
	SetCatalog(singleCatalog(1535, 1536))
	defer SetCatalog(nil)
	hub.Load()
	if len(hub.Loaded()) != 1 || hub.Loaded()[0] != "Doors" {
		t.Fatalf("hub loaded = %v, want [Doors]", hub.Loaded())
	}
	p, sess := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	if !hub.EmitObjectInteraction(clickEvent(p, 1535, loc, 0, 2, doorDef(1535, "Door"), 1)) {
		t.Fatal("Open 1535 through the hub: want handled")
	}
	if !mgr.hasSwap("register", 1536, game.Location{X: 3226, Y: 3240, Z: 0}, 3) {
		t.Fatalf("swaps = %+v, want the open leaf registered", mgr.calls)
	}
	if len(sess.effects) != 1 || sess.effects[0].soundID != 62 {
		t.Fatalf("sound effects = %+v, want one door-open (62)", sess.effects)
	}
}

// TestUnknownDoorFallsThrough pins the no-catalog / unknown-id path:
// a door the catalog does not know returns false so the click falls
// through to the next handler.
func TestUnknownDoorFallsThrough(t *testing.T) {
	mgr := newFakeObjectManager()
	tasks := game.NewTaskManager()
	rec := &emitRecorder{}
	e := newTestEngine(mgr, tasks, rec, singleCatalog(1535, 1536))
	p, _ := newTestPlayer("alice", 3225, 3239, 0)
	loc := game.Location{X: 3225, Y: 3240, Z: 0}
	if e.toggle(clickEvent(p, 9999, loc, 0, 2, doorDef(9999, "Door"), 1)) {
		t.Fatal("unknown door 9999: want NOT handled")
	}
	if len(mgr.calls) != 0 {
		t.Fatalf("swaps = %+v, want none", mgr.calls)
	}
	// With no catalog at all, even a known id stays shut.
	e2 := newTestEngine(mgr, tasks, rec, nil)
	if e2.toggle(clickEvent(p, 1535, loc, 0, 2, doorDef(1535, "Door"), 1)) {
		t.Fatal("no catalog: want NOT handled")
	}
}
