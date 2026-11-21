package woodcutting

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// capturingSession is a minimal SessionPort + ChatMessageSender
// test double recording plain game messages, so vine/clue message
// lines can be asserted (a bare NewPlayer drops them silently).
type capturingSession struct {
	messages []string
	typed    [][3]any
}

func (s *capturingSession) SendPacket([]byte)                 {}
func (s *capturingSession) SendRunEnergy(int, bool, int, int) {}
func (s *capturingSession) SendDestination(int, int)          {}
func (s *capturingSession) SendVarp(int, int)                 {}
func (s *capturingSession) FlushTick(int64) error             { return nil }
func (s *capturingSession) Close(int, string)                 {}
func (s *capturingSession) IsOpen() bool                      { return true }
func (s *capturingSession) SendGameMessage(message string)    { s.messages = append(s.messages, message) }
func (s *capturingSession) SendTypedGameMessage(chatType int, message, from string) {
	s.typed = append(s.typed, [3]any{chatType, message, from})
}

// newVinePlayer builds a test player at (3200, 3200) with a
// capturing session attached.
func newVinePlayer(name string) (*game.Player, *capturingSession) {
	p := newTestPlayer(name, 3200, 3200)
	session := &capturingSession{}
	p.Session = session
	return p, session
}

// placeVine registers a vine object at (3201, 3200, 0) — the tile
// east of the test player's spawn — and returns it.
func placeVine(objects *fakeObjects, id int) *game.GameObject {
	obj := game.NewGameObject(id, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(obj, true)
	return obj
}

// vineEvent builds an ObjectInteractionEvent for a vine click with
// the given definition.
func vineEvent(player *game.Player, obj plugins.GameObject, def plugins.ObjectDefinition) *plugins.ObjectInteractionEvent {
	return &plugins.ObjectInteractionEvent{
		Player:     player,
		Object:     obj,
		Definition: def,
		ObjectID:   obj.ID(),
		Option:     "Chop",
	}
}

// TestVineGroupFor pins group classification (groupFor): by cache
// name + own option. The Brimhaven Dungeon's "Vines" are chopped
// down, not cleared — a "Vines" with a Chop option belongs to
// neither group, so no tool use on it may clear it.
func TestVineGroupFor(t *testing.T) {
	cases := []struct {
		name    string
		options []string
		want    vineGroup
		ok      bool
	}{
		{"Thick vine", []string{"Chop"}, vineGroupChop, true},
		{"Thick vines", []string{"Chop"}, vineGroupChop, true},
		{"Tendrils", []string{"Chop"}, vineGroupChop, true},
		{"Vines", []string{"Clear"}, vineGroupClear, true},
		// Brimhaven: "Vines" with a Chop option is neither group.
		{"Vines", []string{"Chop"}, "", false},
		{"Vines", []string{"Chop down"}, "", false},
		// Wrong option for the name is neither group.
		{"Thick vine", []string{"Clear"}, "", false},
		{"Tendrils", []string{"Clear"}, "", false},
		{"Vines", []string{}, "", false},
		{"Tree", []string{"Chop down"}, "", false},
		{"", []string{"Chop"}, "", false},
	}
	for _, tc := range cases {
		got, ok := vineGroupFor(tc.name, tc.options)
		if got != tc.want || ok != tc.ok {
			t.Errorf("vineGroupFor(%q, %v) = %q, %v, want %q, %v", tc.name, tc.options, got, ok, tc.want, tc.ok)
		}
	}
}

// TestVineKey pins the pending key (vineKey): "<id>:<x>,<y>,<z>".
func TestVineKey(t *testing.T) {
	obj := game.NewGameObject(30646, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	if got, want := vineKey(obj), "30646:3201,3200,0"; got != want {
		t.Errorf("vineKey = %q, want %q", got, want)
	}
}

// TestAxeDisplayNamesCoverAxes pins the tool-use registration
// table: every AxeDefs gameval has exactly one display name, and
// the distinct names are the 27 cache display names the hub
// matches by (probe, 2026-11-20). A missing or wrong name would
// silently never fire its registration.
func TestAxeDisplayNamesCoverAxes(t *testing.T) {
	seen := make(map[string]int)
	for _, entry := range thickVineAxeDisplays {
		seen[entry.Item]++
	}
	if len(thickVineAxeDisplays) != len(AxeDefs) {
		t.Errorf("thickVineAxeDisplays = %d entries, want %d (one per AxeDefs)", len(thickVineAxeDisplays), len(AxeDefs))
	}
	for _, def := range AxeDefs {
		if seen[def.Item] != 1 {
			t.Errorf("axe %q: %d display entries, want 1", def.Item, seen[def.Item])
		}
	}
	if got := len(thickVineAxeDisplayNames()); got != 27 {
		t.Errorf("distinct axe display names = %d, want 27", got)
	}
	for _, want := range []string{"Bronze axe", "Dragon axe", "Infernal axe (uncharged)", "Infernal axe (or)", "Crystal axe", "Echo axe", "3rd Age axe", "Gilded axe"} {
		found := false
		for _, name := range thickVineAxeDisplayNames() {
			if name == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("display names lack %q", want)
		}
	}
}

// TestPlayerAxeIgnoresWieldLevels pins playerAxe (findAnyAxe):
// the best axe held anywhere wins, with no Woodcutting gate — a
// level-1 player with a dragon axe in the inventory cuts with it.
func TestPlayerAxeIgnoresWieldLevels(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	bronze := axeByItem(tables, "bronze_axe")
	dragon := axeByItem(tables, "dragon_axe")
	if bronze == nil || dragon == nil {
		t.Fatal("fixture tables missing axes")
	}
	p := newTestPlayer("alice", 3200, 3200)
	setLevel(p, game.SkillWoodcutting, 1)
	if got := w.playerAxe(p); got != nil {
		t.Errorf("no axe: playerAxe = %+v, want nil", got)
	}
	p.Inventory().Adds(dragon.ID, 1)
	if got := w.playerAxe(p); got == nil || got.ID != dragon.ID {
		t.Errorf("dragon in inventory at level 1: playerAxe = %+v, want dragon", got)
	}
	// Best-first: bronze equipped as well still yields dragon.
	p.Equipment().Set(game.WeaponSlot, game.NewItem(bronze.ID, 1))
	if got := w.playerAxe(p); got == nil || got.ID != dragon.ID {
		t.Errorf("dragon + bronze: playerAxe = %+v, want dragon", got)
	}
	// Only bronze: bronze, even wielded below its own level gate
	// — playerAxe never checks levels at all.
	p2 := newTestPlayer("bob", 3200, 3200)
	setLevel(p2, game.SkillWoodcutting, 1)
	p2.Inventory().Adds(bronze.ID, 1)
	if got := w.playerAxe(p2); got == nil || got.ID != bronze.ID {
		t.Errorf("bronze only: playerAxe = %+v, want bronze", got)
	}
}

// TestResolveRakeID pins the rake resolution (resolveRakeId): the
// fixture tables carry the gameval "rake" id, and hasRake reads
// inventory or equipment.
func TestResolveRakeID(t *testing.T) {
	tables := loadFixtureTables(t)
	if tables.RakeID == 0 {
		t.Fatal("fixture tables RakeID = 0")
	}
	w, _ := newTestWoodcutting(t, tables)
	if got := w.resolveRakeID(); got != tables.RakeID {
		t.Errorf("resolveRakeID = %d, want %d", got, tables.RakeID)
	}
	p := newTestPlayer("alice", 3200, 3200)
	if w.hasRake(p) {
		t.Error("no rake: hasRake = true")
	}
	p.Inventory().Adds(tables.RakeID, 1)
	if !w.hasRake(p) {
		t.Error("rake in inventory: hasRake = false")
	}
	p2 := newTestPlayer("bob", 3200, 3200)
	p2.Equipment().Set(game.WeaponSlot, game.NewItem(tables.RakeID, 1))
	if !w.hasRake(p2) {
		t.Error("rake equipped: hasRake = false")
	}
}

// TestChopVegetationFlow pins the whole chop flow (useTool): face
// + start message + axe animation at the click, nothing before
// +2 ticks, then deregister + 2 Woodcutting XP + done message,
// and regrow after 49 ticks.
func TestChopVegetationFlow(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 1)
	bronze := axeByItem(tables, "bronze_axe")
	p.Inventory().Adds(bronze.ID, 1)
	obj := placeVine(objects, 30646)
	def := fakeObjectDef{name: "Thick vine", interactions: []string{"Chop"}}
	if !w.chopVegetation(vineEvent(p, obj, def)) {
		t.Fatal("chopVegetation did not claim the click")
	}
	// Face: the player turns east toward the vine (the player
	// facing seam), and the flag is set.
	if p.Facing != game.DirEast {
		t.Errorf("Facing = %v, want EAST", p.Facing)
	}
	if !p.Flags.Has(game.FlagFacePosition) {
		t.Error("FlagFacePosition not set on the chop swing")
	}
	if anim := p.Animation(); anim == nil || anim.ID != bronze.AnimationID {
		t.Errorf("animation = %+v, want id %d", anim, bronze.AnimationID)
	}
	if len(session.messages) != 1 || session.messages[0] != "You swing your axe at the thick vine." {
		t.Errorf("messages = %v, want [You swing your axe at the thick vine.]", session.messages)
	}
	// Nothing clears before +2 ticks.
	tm.Process(1)
	if w.plantAt(obj) == nil {
		t.Error("plant cleared after 1 tick, want still standing")
	}
	if p.Skills().Experience(game.SkillWoodcutting) != 0 {
		t.Error("XP paid before the +2 tick delay")
	}
	tm.Process(2)
	if w.plantAt(obj) != nil {
		t.Error("plant still standing after the +2 tick delay")
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 2 {
		t.Errorf("Woodcutting XP = %d, want 2", got)
	}
	if len(session.messages) != 2 || session.messages[1] != "You chop the thick vine down." {
		t.Errorf("messages = %v, want done message second", session.messages)
	}
	// Still gone at +48 ticks after the clear; back at +49.
	for c := int64(3); c <= 50; c++ {
		tm.Process(c)
	}
	if w.plantAt(obj) != nil {
		t.Error("plant regrew before 49 ticks")
	}
	tm.Process(51)
	if got := w.plantAt(obj); got == nil || got.ID() != 30646 {
		t.Errorf("plantAt after regrow = %+v, want the vine back", got)
	}
}

// TestChopVegetationDoubleClickPaysOnce pins the pending rule: a
// second click before the +2 ticks pays nothing extra — one start
// message, one XP grant, one regrow.
func TestChopVegetationDoubleClickPaysOnce(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 1)
	bronze := axeByItem(tables, "bronze_axe")
	p.Inventory().Adds(bronze.ID, 1)
	obj := placeVine(objects, 30648)
	def := fakeObjectDef{name: "Thick vines", interactions: []string{"Chop"}}
	event := vineEvent(p, obj, def)
	if !w.chopVegetation(event) {
		t.Fatal("first chopVegetation did not claim")
	}
	if !w.chopVegetation(event) {
		t.Fatal("second chopVegetation did not claim")
	}
	if len(session.messages) != 1 {
		t.Errorf("messages = %v, want exactly one start message", session.messages)
	}
	tm.Process(1)
	tm.Process(2)
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 2 {
		t.Errorf("Woodcutting XP = %d, want 2 (double click pays once)", got)
	}
}

// TestChopVegetationGuards pins the click guards: already chopped
// and no axe both message and claim, without starting a swing.
func TestChopVegetationGuards(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm

	// No axe: message, claim, no pending, no task.
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 1)
	obj := placeVine(objects, 30646)
	def := fakeObjectDef{name: "Thick vine", interactions: []string{"Chop"}}
	if !w.chopVegetation(vineEvent(p, obj, def)) {
		t.Error("no-axe chopVegetation did not claim")
	}
	if len(session.messages) != 1 || session.messages[0] != thickVineNoAxeMessage {
		t.Errorf("messages = %v, want [%q]", session.messages, thickVineNoAxeMessage)
	}
	if len(w.vines.pending) != 0 {
		t.Error("no-axe click left a pending clearing")
	}

	// Already chopped: the plant is gone, so the click messages.
	p2, session2 := newVinePlayer("bob")
	setLevel(p2, game.SkillWoodcutting, 1)
	p2.Inventory().Adds(axeByItem(tables, "bronze_axe").ID, 1)
	obj2 := placeVine(objects, 30645)
	def2 := fakeObjectDef{name: "Tendrils", interactions: []string{"Chop"}}
	if !w.chopVegetation(vineEvent(p2, obj2, def2)) {
		t.Fatal("chop did not claim")
	}
	tm.Process(1)
	tm.Process(2)
	if w.plantAt(obj2) != nil {
		t.Fatal("plant did not clear")
	}
	before := len(session2.messages)
	if !w.chopVegetation(vineEvent(p2, obj2, def2)) {
		t.Error("already-chopped chopVegetation did not claim")
	}
	if len(session2.messages) != before+1 || session2.messages[before] != thickVineAlreadyChoppedMessage {
		t.Errorf("messages = %v, want already-chopped message", session2.messages)
	}
	if got := p2.Skills().Experience(game.SkillWoodcutting); got != 2 {
		t.Errorf("Woodcutting XP = %d, want 2 (no second payment)", got)
	}
}

// TestUseToolDeadPlayerSkipsClear pins the delayed guard: a player
// who dies (or disconnects) before the +2 ticks clears nothing
// and pays nothing — and the pending entry is still dropped, so a
// later chop works.
func TestUseToolDeadPlayerSkipsClear(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm
	p, _ := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 1)
	p.Inventory().Adds(axeByItem(tables, "bronze_axe").ID, 1)
	obj := placeVine(objects, 30646)
	def := fakeObjectDef{name: "Thick vine", interactions: []string{"Chop"}}
	if !w.chopVegetation(vineEvent(p, obj, def)) {
		t.Fatal("chop did not claim")
	}
	setLevel(p, game.SkillHitpoints, 0)
	tm.Process(1)
	tm.Process(2)
	if w.plantAt(obj) == nil {
		t.Error("dead player's plant cleared, want still standing")
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 0 {
		t.Errorf("Woodcutting XP = %d, want 0", got)
	}
	if len(w.vines.pending) != 0 {
		t.Error("pending entry not dropped after the dead-player skip")
	}
}

// TestClearVegetationFlow pins the rake flow (clearUse): Farming
// XP 2, rake animation 2273, and the clear messages.
func TestClearVegetationFlow(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm
	p, session := newVinePlayer("alice")
	p.Inventory().Adds(tables.RakeID, 1)
	obj := placeVine(objects, 30644)
	def := fakeObjectDef{name: "Vines", interactions: []string{"Clear"}}
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: obj, Definition: def, ObjectID: obj.ID(), Option: "Clear",
	}
	if !w.clearVegetation(event) {
		t.Fatal("clearVegetation did not claim the click")
	}
	if anim := p.Animation(); anim == nil || anim.ID != thickVineRakeAnimation {
		t.Errorf("animation = %+v, want id %d", anim, thickVineRakeAnimation)
	}
	if len(session.messages) != 1 || session.messages[0] != "You rake the vines." {
		t.Errorf("messages = %v, want [You rake the vines.]", session.messages)
	}
	tm.Process(1)
	tm.Process(2)
	if w.plantAt(obj) != nil {
		t.Error("plant still standing after the +2 tick delay")
	}
	if got := p.Skills().Experience(game.SkillFarming); got != 2 {
		t.Errorf("Farming XP = %d, want 2", got)
	}
	if got := p.Skills().Experience(game.SkillWoodcutting); got != 0 {
		t.Errorf("Woodcutting XP = %d, want 0 (clear pays Farming)", got)
	}
	if len(session.messages) != 2 || session.messages[1] != "You clear the vines." {
		t.Errorf("messages = %v, want done message second", session.messages)
	}
}

// TestClearVegetationGuards pins the clear guards: no rake and
// already cleared both message and claim.
func TestClearVegetationGuards(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	w.tasks = game.NewTaskManager()

	p, session := newVinePlayer("alice")
	obj := placeVine(objects, 30650)
	def := fakeObjectDef{name: "Vines", interactions: []string{"Clear"}}
	event := &plugins.ObjectInteractionEvent{
		Player: p, Object: obj, Definition: def, ObjectID: obj.ID(), Option: "Clear",
	}
	if !w.clearVegetation(event) {
		t.Error("no-rake clearVegetation did not claim")
	}
	if len(session.messages) != 1 || session.messages[0] != thickVineNoRakeMessage {
		t.Errorf("messages = %v, want [%q]", session.messages, thickVineNoRakeMessage)
	}

	// Already cleared: chop it down first via the rake path, then
	// click again.
	p.Inventory().Adds(tables.RakeID, 1)
	if !w.clearVegetation(event) {
		t.Fatal("clear did not claim")
	}
	w.tasks.Process(1)
	w.tasks.Process(2)
	before := len(session.messages)
	if !w.clearVegetation(event) {
		t.Error("already-cleared clearVegetation did not claim")
	}
	if len(session.messages) != before+1 || session.messages[before] != thickVineAlreadyClearedMessage {
		t.Errorf("messages = %v, want already-cleared message", session.messages)
	}
}

// TestToolOnVegetation pins the tool-use dispatch (toolOnVegetation):
// an axe on a chop plant chops, a non-axe falls through, a rake on
// a clear plant clears, and an axe on a clear plant falls through.
func TestToolOnVegetation(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	tm := game.NewTaskManager()
	w.tasks = tm
	bronze := axeByItem(tables, "bronze_axe")
	logID := treeByName(tables, "normal tree").LogID

	// Axe on a chop plant: claims and chops.
	p, session := newVinePlayer("alice")
	setLevel(p, game.SkillWoodcutting, 1)
	obj := placeVine(objects, 30646)
	ev := &plugins.ItemOnObjectEvent{Player: p, Object: obj, ObjectID: obj.ID(), ItemID: bronze.ID}
	if !w.toolOnChopVegetation(ev, "Thick vine") {
		t.Error("axe on Thick vine: tool did not claim")
	}
	if len(session.messages) != 1 || session.messages[0] != "You swing your axe at the thick vine." {
		t.Errorf("messages = %v, want the swing line", session.messages)
	}

	// A non-axe item on a chop plant: falls through.
	ev2 := &plugins.ItemOnObjectEvent{Player: p, Object: obj, ObjectID: obj.ID(), ItemID: logID}
	if w.toolOnChopVegetation(ev2, "Thick vine") {
		t.Error("logs on Thick vine: tool claimed, want fall-through")
	}

	// Rake on a chop plant: falls through (no axe id).
	ev3 := &plugins.ItemOnObjectEvent{Player: p, Object: obj, ObjectID: obj.ID(), ItemID: tables.RakeID}
	if w.toolOnChopVegetation(ev3, "Thick vine") {
		t.Error("rake on Thick vine: tool claimed, want fall-through")
	}

	// Axe on a clear plant: falls through.
	clearObj := placeVine(objects, 30644)
	ev4 := &plugins.ItemOnObjectEvent{Player: p, Object: clearObj, ObjectID: clearObj.ID(), ItemID: bronze.ID}
	if w.toolOnClearVegetation(ev4, "Vines") {
		t.Error("axe on Vines: tool claimed, want fall-through")
	}

	// Rake on a clear plant: claims and clears.
	p.Inventory().Adds(tables.RakeID, 1)
	ev5 := &plugins.ItemOnObjectEvent{Player: p, Object: clearObj, ObjectID: clearObj.ID(), ItemID: tables.RakeID}
	if !w.toolOnClearVegetation(ev5, "Vines") {
		t.Error("rake on Vines: tool did not claim")
	}
	tm.Process(1)
	tm.Process(2)
	if got := p.Skills().Experience(game.SkillFarming); got != 2 {
		t.Errorf("Farming XP = %d, want 2", got)
	}

	// Already chopped via tool use: message, claim, no second pay.
	tm.Process(3)
	tm.Process(4)
	before := len(session.messages)
	if !w.toolOnChopVegetation(ev, "Thick vine") {
		t.Error("axe on chopped vine: tool did not claim")
	}
	if len(session.messages) != before+1 || session.messages[before] != thickVineAlreadyChoppedMessage {
		t.Errorf("messages = %v, want already-chopped message", session.messages)
	}
}

// TestPlantAt pins the live-plant lookup: a map original is found,
// a deregistered plant is not, and a runtime registration is.
func TestPlantAt(t *testing.T) {
	tables := loadFixtureTables(t)
	w, _ := newTestWoodcutting(t, tables)
	objects := newFakeObjects()
	w.objects = objects
	obj := game.NewGameObject(30646, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.placeMapOriginal(obj)
	if got := w.plantAt(obj); got == nil {
		t.Error("map-original plant not found")
	}
	objects.Deregister(obj, true)
	if got := w.plantAt(obj); got != nil {
		t.Error("deregistered plant still found")
	}
	objects.Register(obj, true)
	if got := w.plantAt(obj); got == nil {
		t.Error("runtime plant not found")
	}
	// A different id on the same tile is not the plant.
	other := game.NewGameObject(30644, game.Location{X: 3201, Y: 3200, Z: 0}, 10, 0, nil)
	objects.Register(other, true)
	if got := w.plantAt(other); got == nil || got.ID() != 30644 {
		t.Errorf("plantAt(other) = %+v, want the Vines object", got)
	}
}
