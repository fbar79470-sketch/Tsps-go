package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Object-family tests: the registration methods, emits, and
// gate contracts in objects.go, grounded in PluginManager.ts
// (emitObjectInteraction 1170-1212, emitObjectRoute 1214-1224,
// emitItemOnObject 1915-1923, registerObjectClickHook
// 3006-3040, onObjectInteraction 3423-3451, onItemOnObject
// 4077-4116) and PluginTypes.ts (event shapes). Fixtures only.

type fakeObject struct {
	id   int
	loc  game.Location
	typ  int
	face int
}

func (o *fakeObject) ID() int                 { return o.id }
func (o *fakeObject) Location() game.Location { return o.loc }
func (o *fakeObject) Type() int               { return o.typ }
func (o *fakeObject) Face() int               { return o.face }

type fakeObjectDef struct {
	id           int
	name         string
	sizeX, sizeY int
	mask         int
	interactions []string
}

func (d *fakeObjectDef) ID() int                { return d.id }
func (d *fakeObjectDef) Name() string           { return d.name }
func (d *fakeObjectDef) SizeX() int             { return d.sizeX }
func (d *fakeObjectDef) SizeY() int             { return d.sizeY }
func (d *fakeObjectDef) BlockingMask() int      { return d.mask }
func (d *fakeObjectDef) Interactions() []string { return d.interactions }

type fakeObjectManager struct {
	registered   []GameObject
	deregistered []GameObject
	replaced     [][2]GameObject
	at           map[game.Location][]GameObject
	inRegion     map[[2]int][]GameObject
	exists       map[game.Location]bool
	found        GameObject
	stands       bool
	removed      int
}

func (m *fakeObjectManager) Register(obj GameObject, playerUpdate bool) {
	_ = playerUpdate
	m.registered = append(m.registered, obj)
}
func (m *fakeObjectManager) Deregister(obj GameObject, playerUpdate bool) {
	_ = playerUpdate
	m.deregistered = append(m.deregistered, obj)
}
func (m *fakeObjectManager) ReplaceInPlace(outgoing, incoming GameObject) {
	m.replaced = append(m.replaced, [2]GameObject{outgoing, incoming})
}
func (m *fakeObjectManager) ObjectsAt(loc game.Location) []GameObject { return m.at[loc] }
func (m *fakeObjectManager) ObjectsInRegion(regionX, regionY int) []GameObject {
	return m.inRegion[[2]int{regionX, regionY}]
}
func (m *fakeObjectManager) Exists(id int, loc game.Location) bool { _ = id; return m.exists[loc] }
func (m *fakeObjectManager) Get(id int, loc game.Location) GameObject {
	_, _ = id, loc
	return m.found
}
func (m *fakeObjectManager) Find(player *game.Player, id int, loc game.Location) GameObject {
	_, _, _ = player, id, loc
	return m.found
}
func (m *fakeObjectManager) StillStands(player *game.Player, id int, loc game.Location, obj GameObject) bool {
	_, _, _, _ = player, id, loc, obj
	return m.stands
}
func (m *fakeObjectManager) RemovePluginObjects() int { return m.removed }

// RemovedObjects is the fake's removed-objects list: the fake
// records no removals, so it is always empty.
func (m *fakeObjectManager) RemovedObjects() []GameObject { return nil }

func testObjectPlayer() *game.Player {
	return game.NewPlayer("testplayer", game.Location{X: 100, Y: 100, Z: 0})
}

func testObjectDef() *fakeObjectDef {
	return &fakeObjectDef{
		id: 100, name: "Oak tree", sizeX: 1, sizeY: 1,
		interactions: []string{"Chop down", "Examine"},
	}
}

func TestEmitObjectRouteStopsOnDestination(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var order []string
	dest := game.Location{X: 105, Y: 100, Z: 0}
	api.OnObjectRoute(func(ev *ObjectRouteEvent) {
		order = append(order, "first")
		ev.Destination = &dest
		ev.BlockAccessFlags = 4
	})
	api.OnObjectRoute(func(ev *ObjectRouteEvent) {
		order = append(order, "second")
	})
	ev := &ObjectRouteEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		Definition: testObjectDef(), ObjectID: 100, ClickType: 1,
		SourceLocation: game.Location{X: 100, Y: 100, Z: 0},
	}
	h.EmitObjectRoute(ev)
	if len(order) != 1 || order[0] != "first" {
		t.Fatalf("route hooks ran %v, want [first]: the emit stops once a destination is set", order)
	}
	if ev.Destination == nil || *ev.Destination != dest {
		t.Fatalf("destination = %+v, want %+v", ev.Destination, dest)
	}
	if ev.BlockAccessFlags != 4 {
		t.Fatalf("blockAccessFlags = %d, want 4", ev.BlockAccessFlags)
	}
}

func TestEmitObjectRouteGuards(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	called := false
	api.OnObjectRoute(func(ev *ObjectRouteEvent) { called = true })
	h.EmitObjectRoute(nil)
	if called {
		t.Fatal("nil route event must run no hooks")
	}
	h.EmitObjectRoute(&ObjectRouteEvent{Object: &fakeObject{}})
	if called {
		t.Fatal("route event without a player must run no hooks")
	}
	h.EmitObjectRoute(&ObjectRouteEvent{Player: testObjectPlayer()})
	if called {
		t.Fatal("route event without an object must run no hooks")
	}
	// A nil handler registers nothing and never panics.
	api.OnObjectRoute(nil)
	h.EmitObjectRoute(&ObjectRouteEvent{Player: testObjectPlayer(), Object: &fakeObject{}})
	if !called {
		t.Fatal("the registered route hook should run for a valid event")
	}
}

func TestEmitObjectInteractionGuards(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	called := false
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { called = true })
	player := testObjectPlayer()
	obj := &fakeObject{id: 100}
	if h.EmitObjectInteraction(nil) {
		t.Fatal("nil event must return false")
	}
	if h.EmitObjectInteraction(&ObjectInteractionEvent{Object: obj}) {
		t.Fatal("event without a player must return false")
	}
	if h.EmitObjectInteraction(&ObjectInteractionEvent{Player: player}) {
		t.Fatal("event without an object must return false")
	}
	if h.EmitObjectInteraction(&ObjectInteractionEvent{Player: player, Object: obj, Handled: true}) {
		t.Fatal("pre-handled event must return false")
	}
	if called {
		t.Fatal("no guard case may run a hook")
	}
}

func TestEmitObjectInteractionNoHooksEarlyReturn(t *testing.T) {
	h, _ := newTestHub()
	h.SetObjectDefinitionResolver(func(id int, player *game.Player) ObjectDefinition {
		return testObjectDef()
	})
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1,
	}
	if h.EmitObjectInteraction(ev) {
		t.Fatal("no-hooks emit must return false")
	}
	if ev.Definition != nil || ev.HasOption {
		t.Fatal("the no-hooks early return runs before the definition/option fill-ins")
	}
}

func TestEmitObjectInteractionFillIns(t *testing.T) {
	h, _ := newTestHub()
	h.SetObjectDefinitionResolver(func(id int, player *game.Player) ObjectDefinition {
		if id == 100 {
			return testObjectDef()
		}
		return nil
	})
	api := newApi(h, "P")
	var seenDef ObjectDefinition
	var seenOption string
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) {
		seenDef = ev.Definition
		seenOption = ev.Option
		ev.Handled = true
	})
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1,
	}
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("claiming hook should make the emit return true")
	}
	if seenDef == nil || seenDef.Name() != "Oak tree" {
		t.Fatalf("definition fill-in = %+v, want the resolved Oak tree definition", seenDef)
	}
	// The fill-in is the RAW interaction text, not lower-cased:
	// TS fills option ??= interactions[clickType-1] ?? "".
	if seenOption != "Chop down" {
		t.Fatalf("option fill-in = %q, want %q", seenOption, "Chop down")
	}
	// A supplied option is never overwritten.
	seenOption = ""
	ev2 := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Option: "chop down", HasOption: true,
		Definition: testObjectDef(),
	}
	h.EmitObjectInteraction(ev2)
	if seenOption != "chop down" {
		t.Fatalf("supplied option = %q, want it preserved", seenOption)
	}
}

func TestEmitObjectInteractionBeforeClaim(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	mainRan := false
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { mainRan = true })
	api.OnBeforeObjectInteraction(func(ev *ObjectInteractionEvent) { ev.Handled = true })
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Definition: testObjectDef(),
	}
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("before-hook claim must return true")
	}
	if mainRan {
		t.Fatal("a before-hook claim must skip every main hook")
	}
	// An unclaimed before hook leaves the event for the mains.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	api2.OnBeforeObjectInteraction(func(ev *ObjectInteractionEvent) {})
	api2.OnObjectInteraction(func(ev *ObjectInteractionEvent) { mainRan = true; ev.Handled = true })
	mainRan = false
	ev2 := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Definition: testObjectDef(),
	}
	if !h2.EmitObjectInteraction(ev2) {
		t.Fatal("unclaimed before hook must fall through to the mains")
	}
	if !mainRan {
		t.Fatal("main hook should run after an unclaimed before hook")
	}
}

func TestEmitObjectInteractionMergedOrder(t *testing.T) {
	h, _ := newTestHub()
	h.SetObjectDefinitionResolver(func(id int, player *game.Player) ObjectDefinition {
		return testObjectDef()
	})
	api := newApi(h, "P")
	var order []string
	// Interleaved registration: generic, id, name, generic.
	// The emit merges them in original registration order.
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { order = append(order, "generic-1") })
	api.OnObjectClick([]int{100}, 1, func(ev *ObjectInteractionEvent) bool {
		order = append(order, "id")
		return false
	})
	api.OnObjectInteractionNamed("Oak tree", map[string]ObjectInteractionHandler{
		"Chop down": func(ev *ObjectInteractionEvent) bool {
			order = append(order, "name")
			return false
		},
	})
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) {
		order = append(order, "generic-2")
		ev.Handled = true
	})
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { order = append(order, "generic-3") })
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1,
	}
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("claiming hook should make the emit return true")
	}
	want := []string{"generic-1", "id", "name", "generic-2"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Fatalf("hook order = %v, want %v (and the emit must stop once handled)", order, want)
	}
}

func TestEmitObjectInteractionNameNeedsDefinition(t *testing.T) {
	// With no definition (and no resolver), the name list is
	// not consulted: TS looks up objectHooksByName with
	// undefined and matches nothing.
	h, _ := newTestHub()
	api := newApi(h, "P")
	ran := false
	api.OnObjectInteractionNamed("", map[string]ObjectInteractionHandler{
		"Chop down": func(ev *ObjectInteractionEvent) bool { ran = true; return true },
	})
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1,
	}
	if h.EmitObjectInteraction(ev) || ran {
		t.Fatal("a name hook registered under \"\" must not match an event with no definition")
	}
}

func TestOnObjectInteractionNamedMatching(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var ran []string
	api.OnObjectInteractionNamed("Oak tree", map[string]ObjectInteractionHandler{
		"Chop down": func(ev *ObjectInteractionEvent) bool {
			ran = append(ran, "chop")
			return false // Fall through: the next hook may claim.
		},
		"Examine": func(ev *ObjectInteractionEvent) bool {
			ran = append(ran, "examine")
			return true
		},
		"Unused": nil, // A nil handler in the map is dropped at registration.
	})
	// The lookup uses the RAW interaction text at the
	// click-type slot, not the lower-cased event option: the
	// event option here is lower-cased, the definition slot
	// holds "Chop down", and the handler still matches.
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Option: "chop down", HasOption: true,
		Definition: testObjectDef(),
	}
	if h.EmitObjectInteraction(ev) {
		t.Fatal("a false return must fall through without claiming")
	}
	if len(ran) != 1 || ran[0] != "chop" {
		t.Fatalf("ran = %v, want [chop]", ran)
	}
	// Click type 2 selects "Examine", which claims.
	ev.ClickType = 2
	ev.Option = "examine"
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("a true return must claim the interaction")
	}
	// A different object name never matches.
	ev.Definition = &fakeObjectDef{id: 101, name: "Willow tree", interactions: []string{"Chop down", "Examine"}}
	ev.ClickType = 1
	ran = nil
	if h.EmitObjectInteraction(ev) || len(ran) != 0 {
		t.Fatalf("name mismatch must not run the hook: ran = %v", ran)
	}
	// Click types outside 1..5 never match.
	ev.Definition = testObjectDef()
	ev.ClickType = 0
	ran = nil
	h.EmitObjectInteraction(ev)
	if len(ran) != 0 {
		t.Fatalf("clickType 0 must not match a named hook: ran = %v", ran)
	}
	// A nil actions map registers nothing.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	api2.OnObjectInteractionNamed("Oak tree", nil)
	if h2.hooks.objectHookCount() != 0 {
		t.Fatal("a nil actions map must register no hook")
	}
}

func TestOnObjectClickMatching(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var ran []int
	api.OnObjectClick([]int{100}, 2, func(ev *ObjectInteractionEvent) bool {
		ran = append(ran, ev.ObjectID)
		return true
	})
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 2, Definition: testObjectDef(),
	}
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("matching id + clickType must run the hook")
	}
	ev.ClickType = 1
	ran = nil
	h.EmitObjectInteraction(ev)
	if len(ran) != 0 {
		t.Fatal("a different clickType must not match an id hook")
	}
	ev.ClickType = 2
	ev.ObjectID = 101
	ev.Object = &fakeObject{id: 101}
	ran = nil
	h.EmitObjectInteraction(ev)
	if len(ran) != 0 {
		t.Fatal("a different id must not match an id hook")
	}
	// The First..Fifth helpers fix the click type.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	firstRan := false
	api2.OnObjectFirstClick([]int{100}, func(ev *ObjectInteractionEvent) bool {
		firstRan = true
		return true
	})
	ev2 := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Definition: testObjectDef(),
	}
	if !h2.EmitObjectInteraction(ev2) || !firstRan {
		t.Fatal("OnObjectFirstClick must match clickType 1")
	}
	ev2.ClickType = 5
	firstRan = false
	h2.EmitObjectInteraction(ev2)
	if firstRan {
		t.Fatal("OnObjectFirstClick must not match clickType 5")
	}
}

func TestOnObjectClickValidation(t *testing.T) {
	h, buf := newTestHub()
	api := newApi(h, "P")
	// Invalid click types warn and register nothing.
	api.OnObjectClick([]int{100}, 0, func(ev *ObjectInteractionEvent) bool { return true })
	api.OnObjectClick([]int{100}, 6, func(ev *ObjectInteractionEvent) bool { return true })
	if h.hooks.objectHookCount() != 0 {
		t.Fatal("invalid click types must register nothing")
	}
	if !strings.Contains(buf.String(), "attempted invalid object click hook registration") ||
		!strings.Contains(buf.String(), "clickType=0") {
		t.Fatalf("log = %q, want invalid-clickType warnings", buf.String())
	}
	// No valid ids: warn and drop.
	buf.Reset()
	api.OnObjectClick([]int{-1, -5}, 1, func(ev *ObjectInteractionEvent) bool { return true })
	if h.hooks.objectHookCount() != 0 {
		t.Fatal("no valid ids must register nothing")
	}
	if !strings.Contains(buf.String(), "invalid") {
		t.Fatalf("log = %q, want an invalid-registration warning", buf.String())
	}
	// Partially invalid: warn, keep the valid ids, deduplicate.
	buf.Reset()
	ran := 0
	api.OnObjectClick([]int{100, 100, -1, 101}, 1, func(ev *ObjectInteractionEvent) bool {
		ran++
		return true
	})
	if h.hooks.objectHookCount() != 1 {
		t.Fatalf("one hook (one order) should be filed, got count %d", h.hooks.objectHookCount())
	}
	if !strings.Contains(buf.String(), "dropped invalid ids") {
		t.Fatalf("log = %q, want a dropped-ids warning", buf.String())
	}
	for _, id := range []int{100, 101} {
		ev := &ObjectInteractionEvent{
			Player: testObjectPlayer(), Object: &fakeObject{id: id},
			ObjectID: id, ClickType: 1, Definition: testObjectDef(),
		}
		if !h.EmitObjectInteraction(ev) {
			t.Fatalf("kept id %d must match", id)
		}
	}
	if ran != 2 {
		t.Fatalf("hook ran %d times, want 2 (once per kept id, deduplicated)", ran)
	}
	// A nil handler registers nothing, silently.
	buf.Reset()
	api.OnObjectClick([]int{100}, 1, nil)
	if h.hooks.objectHookCount() != 1 {
		t.Fatal("a nil handler must register nothing")
	}
	if buf.Len() != 0 {
		t.Fatalf("a nil handler must warn nothing, got %q", buf.String())
	}
}

func TestEmitObjectInteractionErrorIsolation(t *testing.T) {
	h, buf := newTestHub()
	api := newApi(h, "P")
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { panic("boom") })
	secondRan := false
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) {
		secondRan = true
		ev.Handled = true
	})
	ev := &ObjectInteractionEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ClickType: 1, Definition: testObjectDef(),
	}
	if !h.EmitObjectInteraction(ev) {
		t.Fatal("a panicking hook must not prevent the next hook from claiming")
	}
	if !secondRan {
		t.Fatal("the second hook must run after the first panics")
	}
	if !strings.Contains(buf.String(), "object_interaction hook failed (P): boom") {
		t.Fatalf("log = %q, want the per-hook failure line", buf.String())
	}
}

func TestEmitItemOnObjectMainThenFallback(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var order []string
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) {
		order = append(order, "main-1")
	}, nil)
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) {
		order = append(order, "main-2")
		ev.Handled = true
	}, nil)
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) {
		order = append(order, "fallback")
	}, &ItemUseFilter{Fallback: true})
	ev := &ItemOnObjectEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ItemID: 995, ItemSlot: 0, InterfaceType: 3214,
	}
	if !h.EmitItemOnObject(ev) {
		t.Fatal("a claiming main hook must make the emit return true")
	}
	// Both mains run; the fallback wrapper skips once handled.
	if strings.Join(order, ",") != "main-1,main-2" {
		t.Fatalf("order = %v, want [main-1 main-2]", order)
	}
	// When no main claims, the fallback runs.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	order = nil
	api2.OnItemOnObject(func(ev *ItemOnObjectEvent) { order = append(order, "main") }, nil)
	api2.OnItemOnObject(func(ev *ItemOnObjectEvent) {
		order = append(order, "fallback")
		ev.Handled = true
	}, &ItemUseFilter{Fallback: true})
	ev2 := &ItemOnObjectEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ItemID: 995, ItemSlot: 0, InterfaceType: 3214,
	}
	if !h2.EmitItemOnObject(ev2) {
		t.Fatal("a claiming fallback must make the emit return true")
	}
	if strings.Join(order, ",") != "main,fallback" {
		t.Fatalf("order = %v, want [main fallback]", order)
	}
	// A main hook that claims skips the LATER mains too: the
	// wrapper checks Handled before calling.
	h3, _ := newTestHub()
	api3 := newApi(h3, "P")
	order = nil
	api3.OnItemOnObject(func(ev *ItemOnObjectEvent) {
		order = append(order, "main-1")
		ev.Handled = true
	}, nil)
	api3.OnItemOnObject(func(ev *ItemOnObjectEvent) { order = append(order, "main-2") }, nil)
	ev3 := &ItemOnObjectEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ItemID: 995, ItemSlot: 0, InterfaceType: 3214,
	}
	h3.EmitItemOnObject(ev3)
	if strings.Join(order, ",") != "main-1" {
		t.Fatalf("order = %v, want [main-1]: later mains skip once handled", order)
	}
	// Nil event returns false without panicking.
	if h.EmitItemOnObject(nil) {
		t.Fatal("nil item-on-object event must return false")
	}
}

func TestOnItemOnObjectNotedFilter(t *testing.T) {
	h, _ := newTestHub()
	h.SetItemResolver(func(id int) (string, bool) {
		if id == 996 {
			return "Coins", true
		}
		return "Coins", false
	})
	api := newApi(h, "P")
	notedOnly, unnotedOnly := false, false
	yes, no := true, false
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) { notedOnly = true }, &ItemUseFilter{Noted: &yes})
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) { unnotedOnly = true }, &ItemUseFilter{Noted: &no})
	base := &ItemOnObjectEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ItemSlot: 0, InterfaceType: 3214,
	}
	noted := *base
	noted.ItemID = 996
	h.EmitItemOnObject(&noted)
	if !notedOnly || unnotedOnly {
		t.Fatalf("noted use: notedOnly=%v unnotedOnly=%v, want true/false", notedOnly, unnotedOnly)
	}
	notedOnly, unnotedOnly = false, false
	plain := *base
	plain.ItemID = 995
	h.EmitItemOnObject(&plain)
	if notedOnly || !unnotedOnly {
		t.Fatalf("unnoted use: notedOnly=%v unnotedOnly=%v, want false/true", notedOnly, unnotedOnly)
	}
	// The filter values are copied at registration: mutating
	// the caller's filter afterwards changes nothing.
	yes = false
	notedOnly, unnotedOnly = false, false
	h.EmitItemOnObject(&noted)
	if !notedOnly || unnotedOnly {
		t.Fatal("mutating the filter after registration must not change the hook")
	}
}

func TestOnItemOnObjectNamedMatching(t *testing.T) {
	h, _ := newTestHub()
	h.SetItemResolver(func(id int) (string, bool) { return "Bucket of water", false })
	h.SetObjectDefinitionResolver(func(id int, player *game.Player) ObjectDefinition {
		return &fakeObjectDef{id: id, name: "Fire"}
	})
	api := newApi(h, "P")
	ran := false
	api.OnItemOnObjectNamed("Bucket of water", "Fire", func(ev *ItemOnObjectEvent) bool {
		ran = true
		return false // Fall through: must not claim.
	}, nil)
	ev := &ItemOnObjectEvent{
		Player: testObjectPlayer(), Object: &fakeObject{id: 100},
		ObjectID: 100, ItemID: 1, ItemSlot: 0, InterfaceType: 3214,
	}
	if h.EmitItemOnObject(ev) || !ran {
		t.Fatalf("exact name match must run the hook without claiming: handled=%v ran=%v", ev.Handled, ran)
	}
	// A different item name never matches.
	h.SetItemResolver(func(id int) (string, bool) { return "Bucket", false })
	ran = false
	h.EmitItemOnObject(ev)
	if ran {
		t.Fatal("an item-name mismatch must not run a named hook")
	}
}

func TestObjectManagerAccessors(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	if api.GetObjectManager() != nil || h.ObjectManager() != nil || api.Core().ObjectManager() != nil {
		t.Fatal("no manager is installed by default")
	}
	mgr := &fakeObjectManager{}
	h.SetObjectManager(mgr)
	if api.GetObjectManager() != ObjectManagerAPI(mgr) {
		t.Fatal("Api.GetObjectManager must return the installed manager")
	}
	if h.ObjectManager() != ObjectManagerAPI(mgr) {
		t.Fatal("Hub.ObjectManager must return the installed manager")
	}
	if api.Core().ObjectManager() != ObjectManagerAPI(mgr) {
		t.Fatal("Core.ObjectManager must return the same installed manager")
	}
	// The interface methods delegate to the fake.
	obj := &fakeObject{id: 7, loc: game.Location{X: 1, Y: 2, Z: 0}}
	api.GetObjectManager().Register(obj, true)
	if len(mgr.registered) != 1 || mgr.registered[0] != GameObject(obj) {
		t.Fatal("Register must reach the installed manager")
	}
	// Clearing the manager (nil install) un-installs it.
	h.SetObjectManager(nil)
	if api.GetObjectManager() != nil {
		t.Fatal("installing nil must clear the manager")
	}
	// Nil-safe reads.
	var nilApi *Api
	if nilApi.GetObjectManager() != nil {
		t.Fatal("nil Api must read a nil manager")
	}
	var nilHub *Hub
	if nilHub.ObjectManager() != nil {
		t.Fatal("nil Hub must read a nil manager")
	}
	var nilCore *Core
	if nilCore.ObjectManager() != nil {
		t.Fatal("nil Core must read a nil manager")
	}
}

func TestApiEmitsDelegateToHub(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) { ev.Handled = true })
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) { ev.Handled = true }, nil)
	player := testObjectPlayer()
	obj := &fakeObject{id: 100}
	if !api.EmitObjectInteraction(&ObjectInteractionEvent{Player: player, Object: obj, ObjectID: 100, ClickType: 1, Definition: testObjectDef()}) {
		t.Fatal("Api.EmitObjectInteraction must delegate to the hub emit")
	}
	if !api.EmitItemOnObject(&ItemOnObjectEvent{Player: player, Object: obj, ObjectID: 100}) {
		t.Fatal("Api.EmitItemOnObject must delegate to the hub emit")
	}
	var nilApi *Api
	if nilApi.EmitObjectInteraction(nil) || nilApi.EmitItemOnObject(nil) {
		t.Fatal("nil Api emits must return false")
	}
}

func TestEmitObjectOperated(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var got *ObjectOperatedEvent
	api.OnCustomEvent(ObjectOperatedEventName, func(payload any) {
		got, _ = payload.(*ObjectOperatedEvent)
	})
	obj := &fakeObject{id: 100, loc: game.Location{X: 100, Y: 100, Z: 0}}
	h.EmitObjectOperated(&ObjectOperatedEvent{
		Player: testObjectPlayer(), Object: obj, ObjectID: 100,
		Option: "open", Location: obj.Location(),
	})
	if got == nil || got.ObjectID != 100 || got.Option != "open" || got.Location != obj.Location() {
		t.Fatalf("operated payload = %+v, want object 100 option \"open\"", got)
	}
	// A nil event emits nothing and never panics.
	h.EmitObjectOperated(nil)
}

func TestObjectRegistrationAfterLoadRefused(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	api := newApi(h, "Late")
	api.OnObjectRoute(func(ev *ObjectRouteEvent) {})
	api.OnObjectInteraction(func(ev *ObjectInteractionEvent) {})
	api.OnObjectInteractionNamed("X", map[string]ObjectInteractionHandler{"A": func(ev *ObjectInteractionEvent) bool { return true }})
	api.OnBeforeObjectInteraction(func(ev *ObjectInteractionEvent) {})
	api.OnObjectClick([]int{1}, 1, func(ev *ObjectInteractionEvent) bool { return true })
	api.OnItemOnObject(func(ev *ItemOnObjectEvent) {}, nil)
	if h.hooks.objectHookCount() != 0 || len(h.hooks.snapshotObjectRoute()) != 0 || len(h.hooks.snapshotBeforeObject()) != 0 {
		t.Fatal("post-load object registrations must all be refused")
	}
	if !strings.Contains(buf.String(), "registration ignored (hub already loaded)") {
		t.Fatalf("log = %q, want the load-phase refusal warning", buf.String())
	}
}
