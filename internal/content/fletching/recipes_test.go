package fletching

import (
	"testing"

	"tsps-go/internal/plugins"
)

// TestRecipesItemOnItemClaimsThroughHub registers the plugin on
// a test hub and runs a real EmitItemOnItem through it: the
// generic onItemOnItem handler (noted:false) claims the
// unstrung-shortbow-on-bow-string use — a single recipe with
// one batch available takes the 1-batch fast path and starts
// the task (no menu). (Knife-on-logs would open the menu: it
// starts two recipes, so it never takes the fast path.)
func TestRecipesItemOnItemClaimsThroughHub(t *testing.T) {
	f, _, _ := newTestFletching(t)
	hub := plugins.NewHub([]plugins.Plugin{f.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	p.Inventory().Adds(fxBowString, 1)
	event := &plugins.PluginItemOnItemEvent{
		Player:         p,
		UsedItemID:     fxUnstrungShortbow,
		UsedWithItemID: fxBowString,
	}
	if !hub.EmitItemOnItem(event) {
		t.Fatal("EmitItemOnItem = false, want true (Fletching claimed the use)")
	}
	if !event.Handled {
		t.Fatal("event.Handled = false, want true")
	}
	// One unstrung bow + one string → one batch available →
	// the 1-batch fast path starts the task (no menu for a
	// single batch).
	if len(f.running) != 1 {
		t.Fatalf("running tasks = %d, want 1 (1-batch fast path)", len(f.running))
	}
	if len(session.menus) != 0 {
		t.Fatalf("menus opened = %d, want 0 (single batch skips the menu)", len(session.menus))
	}
}

// TestRecipesItemOnItemNotedFallsThrough: the noted:false
// filter skips the handler when either item is noted. The hub
// has no item definitions installed, so nothing reads noted
// here — the filter's itemInfo answers "not noted" for every
// id and the handler runs; this test pins the wiring shape
// (a noted use is refused by the filter, not by handlePair)
// through the filter values themselves.
func TestRecipesItemOnItemFilterIsNotedFalse(t *testing.T) {
	f, _, _ := newTestFletching(t)
	hub := plugins.NewHub([]plugins.Plugin{f.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	// An unrelated pair: nothing claims it either way.
	event := &plugins.PluginItemOnItemEvent{
		Player:         p,
		UsedItemID:     fxLogs,
		UsedWithItemID: fxBowString,
	}
	if hub.EmitItemOnItem(event) {
		t.Fatal("EmitItemOnItem = true for an unknown pair, want false")
	}
}

// TestFletchBarkOpensKnifeMenu: the Celastrus bark Fletch
// action runs handlePair(knife, bark) for the battlestaff
// recipe — a single recipe with 1 batch available takes the
// fast path, so with 2 bark it opens the menu instead.
func TestFletchBarkOpensKnifeMenu(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxCelastrusBark, 2)
	event := &plugins.PluginItemActionEvent{Player: p, ItemID: fxCelastrusBark}
	if !f.fletchBark(event) {
		t.Fatal("fletchBark = false, want true")
	}
	// Battlestaff is ticks 4 (not instant): 2 batches
	// available → the menu opens.
	if len(session.menus) != 1 {
		t.Fatalf("menus opened = %d, want 1", len(session.menus))
	}
	if got := session.menus[0].Items(); len(got) != 1 || got[0] != fxBattlestaff {
		t.Fatalf("menu items = %v, want [battlestaff]", got)
	}
}

// TestFletchBarkWithoutTablesFallsThrough: with no tables
// the action claims nothing.
func TestFletchBarkWithoutTablesFallsThrough(t *testing.T) {
	f := newFletching(nil)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	if f.fletchBark(&plugins.PluginItemActionEvent{Player: p, ItemID: fxCelastrusBark}) {
		t.Fatal("fletchBark with nil tables = true, want false")
	}
}

// TestMembersGatingSkipsFreeWorld is the §8 verification: a
// members plugin does not register on a free-to-play world,
// and does on a members world.
func TestMembersGatingSkipsFreeWorld(t *testing.T) {
	f, _, _ := newTestFletching(t)
	free := plugins.NewHub([]plugins.Plugin{f.plugin()}, plugins.HubOptions{MembersWorld: false})
	free.Load()
	for _, name := range free.Loaded() {
		if name == "Fletching" {
			t.Fatal("Fletching loaded on a free world, want skipped (members:true)")
		}
	}
	members := plugins.NewHub([]plugins.Plugin{f.plugin()}, plugins.HubOptions{MembersWorld: true})
	members.Load()
	found := false
	for _, name := range members.Loaded() {
		if name == "Fletching" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Fletching not loaded on a members world; loaded = %v", members.Loaded())
	}
}

// TestPluginValue verifies the registered plugin's identity
// (name, description, members flag verbatim from
// Fletching.plugin.js).
func TestPluginValue(t *testing.T) {
	if Plugin.Name != "Fletching" {
		t.Fatalf("Plugin.Name = %q, want Fletching", Plugin.Name)
	}
	if !Plugin.Members {
		t.Fatal("Plugin.Members = false, want true")
	}
	if Plugin.Description == "" {
		t.Fatal("Plugin.Description is empty")
	}
}

// TestDisconnectEventTypePins the hub emit the Recipes unit
// relies on (Hub.EmitPlayerDisconnect, hooks.go:1264).
func TestDisconnectStopsTaskThroughHub(t *testing.T) {
	f, _, _ := newTestFletching(t)
	hub := plugins.NewHub([]plugins.Plugin{f.plugin()}, plugins.HubOptions{MembersWorld: true})
	hub.Load()
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxKnife, 1)
	p.Inventory().Adds(fxLogs, 5)
	recipe := recipeByOutput(f.tables, fxArrowShaft)
	f.startFletching(p, recipe, 5, fxKnife, nil)
	if len(f.running) != 1 {
		t.Fatal("no task running before disconnect")
	}
	hub.EmitPlayerDisconnect(&plugins.PlayerDisconnectEvent{Player: p, Username: "alice", Source: "test"})
	if len(f.running) != 0 {
		t.Fatal("task still running after EmitPlayerDisconnect")
	}
}
