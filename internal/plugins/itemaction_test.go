package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

// Item-action family tests: the registration methods and emits
// in itemaction.go, grounded in PluginManager.ts
// (emitItemAction 2016-2043, emitCanUseItem 1654-1657,
// onItemAction 4253-4297, onItemFirstAction 4318-4330,
// onCanUseItem 3830-3832) and PluginTypes.ts (event shapes
// 702-708, 831-860, 1136, 334-342). Fixtures only: the item
// name and inventory-action resolvers are test fakes.

const (
	testNestItemID   = 5001
	testNestItemName = "Bird's nest"
)

func testItemActionPlayer() *game.Player {
	return game.NewPlayer("testplayer", game.Location{X: 100, Y: 100, Z: 0})
}

func testItemActionEvent(itemID, clickType int) *PluginItemActionEvent {
	return &PluginItemActionEvent{
		Player:      testItemActionPlayer(),
		InterfaceID: game.InventoryInterfaceID,
		ItemID:      itemID,
		Slot:        0,
		ClickType:   clickType,
	}
}

// installNestResolvers installs the item name/noted and
// inventory-actions resolvers a named nest handler needs: the
// nest reads "Search" at slot 1 and "Drop" at slot 5.
func installNestResolvers(h *Hub) {
	h.SetItemResolver(func(id int) (string, bool) {
		if id == testNestItemID {
			return testNestItemName, false
		}
		return "", false
	})
	h.SetItemActionsResolver(func(id int) []string {
		if id == testNestItemID {
			return []string{"Search", "", "", "", "Drop"}
		}
		return game.DefaultInventoryActions()
	})
}

func TestEmitItemActionPlainClaims(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var order []string
	api.OnItemAction(func(ev *PluginItemActionEvent) {
		order = append(order, "first")
		ev.Handled = true
	})
	api.OnItemAction(func(ev *PluginItemActionEvent) {
		order = append(order, "second")
	})
	if !h.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("emit returned unhandled, want handled")
	}
	if len(order) != 1 || order[0] != "first" {
		t.Fatalf("hooks ran %v, want [first]: a claimed click skips later hooks", order)
	}
}

func TestEmitItemActionRunAllUnhandled(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	ran := 0
	api.OnItemAction(func(ev *PluginItemActionEvent) { ran++ })
	api.OnItemAction(func(ev *PluginItemActionEvent) { ran++ })
	if h.EmitItemAction(testItemActionEvent(testNestItemID, 2)) {
		t.Fatal("emit returned handled, want unhandled")
	}
	if ran != 2 {
		t.Fatalf("ran %d hooks, want 2: the emit runs every main hook while unhandled", ran)
	}
}

func TestEmitItemActionFallbackRunsOnlyWhenUnhandled(t *testing.T) {
	// A fallback handler claims an unhandled click: the
	// named form with the fallback option files on the
	// fallback list, which runs after every main hook.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	var order []string
	api2.OnItemAction(func(ev *PluginItemActionEvent) { order = append(order, "main") })
	api2.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool {
			order = append(order, "fallback")
			return true
		},
	}, &ItemActionOptions{Fallback: true})
	installNestResolvers(h2)
	if !h2.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("fallback named handler did not claim the unhandled click")
	}
	if len(order) != 2 || order[0] != "main" || order[1] != "fallback" {
		t.Fatalf("hooks ran %v, want [main fallback]: fallbacks run after every main hook", order)
	}

	// A main hook claiming the click keeps the fallback from
	// running at all.
	h3, _ := newTestHub()
	api3 := newApi(h3, "P")
	api3.OnItemAction(func(ev *PluginItemActionEvent) { ev.Handled = true })
	fallbackRan := false
	api3.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool {
			fallbackRan = true
			return true
		},
	}, &ItemActionOptions{Fallback: true})
	installNestResolvers(h3)
	if !h3.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("emit returned unhandled, want handled by the main hook")
	}
	if fallbackRan {
		t.Fatal("fallback handler ran after a main hook claimed the click")
	}
}

func TestEmitItemActionNamedMatches(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	installNestResolvers(h)
	ran := 0
	api.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool {
			ran++
			return true
		},
		"Drop": func(ev *PluginItemActionEvent) bool {
			t.Error("Drop handler ran for a Search click")
			return true
		},
	}, nil)
	// ClickType 1 reads the inventory action at slot 1:
	// "Search".
	if !h.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("named Search handler did not claim the click")
	}
	if ran != 1 {
		t.Fatalf("Search handler ran %d times, want 1", ran)
	}
	// ClickType 2 reads the empty slot-2 action: no match.
	if h.EmitItemAction(testItemActionEvent(testNestItemID, 2)) {
		t.Fatal("named handler claimed a click whose slot has no action")
	}
	// A handler returning false falls through.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	installNestResolvers(h2)
	api2.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool { return false },
	}, nil)
	if h2.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("named handler returning false claimed the click")
	}
	// A different item name never matches.
	h3, _ := newTestHub()
	api3 := newApi(h3, "P")
	installNestResolvers(h3)
	api3.OnItemActionNamed("Clue nest (easy)", map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool { return true },
	}, nil)
	if h3.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("named handler matched the wrong item name")
	}
}

func TestEmitItemActionNamedWornInterface(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	installNestResolvers(h)
	ran := 0
	api.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Check": func(ev *PluginItemActionEvent) bool {
			ran++
			return true
		},
	}, nil)
	// On the equipment interface the event's option wins over
	// the inventory actions: a "Check" click matches.
	ev := testItemActionEvent(testNestItemID, 2)
	ev.InterfaceID = game.EquipmentInventoryInterfaceID
	ev.Option = "Check"
	if !h.EmitItemAction(ev) {
		t.Fatal("named Check handler did not claim the worn click")
	}
	if ran != 1 {
		t.Fatalf("Check handler ran %d times, want 1", ran)
	}
	// ClickType 11 is past the worn maximum (10): no match.
	ev = testItemActionEvent(testNestItemID, 11)
	ev.InterfaceID = game.EquipmentInventoryInterfaceID
	ev.Option = "Check"
	if h.EmitItemAction(ev) {
		t.Fatal("named handler matched a click past the worn maximum")
	}
	// An option-less worn click has no option to match (the
	// worn-param lookup is a gap): no match.
	ev = testItemActionEvent(testNestItemID, 2)
	ev.InterfaceID = game.EquipmentInventoryInterfaceID
	if h.EmitItemAction(ev) {
		t.Fatal("named handler matched an option-less worn click")
	}
}

func TestEmitItemActionNamedNoResolverDefaults(t *testing.T) {
	// With no resolvers installed the item name reads "" and
	// the actions read the ObjType defaults, so a named nest
	// handler never matches — the TS DEFAULT degradation.
	h, _ := newTestHub()
	api := newApi(h, "P")
	api.OnItemActionNamed(testNestItemName, map[string]ItemActionHandler{
		"Search": func(ev *PluginItemActionEvent) bool { return true },
	}, nil)
	if h.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("named handler matched with no resolvers installed")
	}
}

func TestEmitItemActionGuards(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	called := false
	api.OnItemAction(func(ev *PluginItemActionEvent) { called = true })
	if h.EmitItemAction(nil) {
		t.Fatal("nil event must return false")
	}
	if h.EmitItemAction(&PluginItemActionEvent{ItemID: testNestItemID}) {
		t.Fatal("event without a player must return false")
	}
	if called {
		t.Fatal("hook ran for a guarded event")
	}
	// A nil handler registers nothing and never panics.
	api.OnItemAction(nil)
	api.OnItemActionNamed(testNestItemName, nil, nil)
	api.OnItemFirstAction(nil)
	if h.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("emit returned handled with only the guard hook registered")
	}
	if !called {
		t.Fatal("the registered plain hook should run for a valid event")
	}
}

func TestOnItemFirstActionFilters(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	ran := 0
	api.OnItemFirstAction(func(ev *PluginItemActionEvent) bool {
		ran++
		return true
	})
	// ClickType 2 never reaches a first-action hook.
	if h.EmitItemAction(testItemActionEvent(testNestItemID, 2)) {
		t.Fatal("first-action hook claimed a non-first click")
	}
	if ran != 0 {
		t.Fatalf("first-action hook ran %d times for clickType 2, want 0", ran)
	}
	// ClickType 1 runs it and claims.
	if !h.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("first-action hook did not claim the first click")
	}
	if ran != 1 {
		t.Fatalf("first-action hook ran %d times, want 1", ran)
	}
	// A handler returning false falls through.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	api2.OnItemFirstAction(func(ev *PluginItemActionEvent) bool { return false })
	if h2.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("first-action hook returning false claimed the click")
	}
	// An already-handled click skips the first-action hook:
	// the plain hook registered first claims it.
	h3, _ := newTestHub()
	api3 := newApi(h3, "P")
	api3.OnItemAction(func(ev *PluginItemActionEvent) { ev.Handled = true })
	ran = 0
	api3.OnItemFirstAction(func(ev *PluginItemActionEvent) bool {
		ran++
		return true
	})
	if !h3.EmitItemAction(testItemActionEvent(testNestItemID, 1)) {
		t.Fatal("emit returned unhandled, want handled by the plain hook")
	}
	if ran != 0 {
		t.Fatalf("first-action hook ran %d times after a claim, want 0", ran)
	}
}

func TestEmitCanUseItemFirstAnswer(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	player := testItemActionPlayer()
	// No hooks: nil (proceed).
	if got := h.EmitCanUseItem(player, testNestItemID, "action", "Search"); got != nil {
		t.Fatalf("no-hooks answer = %v, want nil", got)
	}
	// A deny wins immediately; hooks after it never run.
	secondRan := false
	api.OnCanUseItem(func(ev *CanUseItemEvent) {
		ev.Allow = BoolPtr(false)
	})
	api.OnCanUseItem(func(ev *CanUseItemEvent) { secondRan = true })
	if got := h.EmitCanUseItem(player, testNestItemID, "action", "Search"); got == nil || *got {
		t.Fatalf("deny answer = %v, want false", got)
	}
	if secondRan {
		t.Fatal("second gate hook ran after a deny")
	}
	// An allow sticks: the first answer wins, so a later
	// deny never runs.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	api2.OnCanUseItem(func(ev *CanUseItemEvent) {
		if ev.ItemID != testNestItemID || ev.Action != "action" || ev.Option != "Search" {
			t.Errorf("gate event = %+v, want the listener's item/action/option", ev)
		}
		ev.Allow = BoolPtr(true)
	})
	api2.OnCanUseItem(func(ev *CanUseItemEvent) { ev.Allow = BoolPtr(false) })
	if got := h2.EmitCanUseItem(player, testNestItemID, "action", "Search"); got == nil || !*got {
		t.Fatalf("allow answer = %v, want true", got)
	}
	// Silence from every hook: nil (undecided).
	h3, _ := newTestHub()
	api3 := newApi(h3, "P")
	api3.OnCanUseItem(func(ev *CanUseItemEvent) {})
	if got := h3.EmitCanUseItem(player, testNestItemID, "action", ""); got != nil {
		t.Fatalf("silent answer = %v, want nil", got)
	}
	// A nil handler registers nothing.
	api3.OnCanUseItem(nil)
	if got := h3.EmitCanUseItem(player, testNestItemID, "action", ""); got != nil {
		t.Fatalf("answer after nil registration = %v, want nil", got)
	}
	// A nil hub yields nil.
	var nilHub *Hub
	if got := nilHub.EmitCanUseItem(player, testNestItemID, "action", ""); got != nil {
		t.Fatalf("nil-hub answer = %v, want nil", got)
	}
}
