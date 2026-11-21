package main

import (
	"testing"

	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// TestInstallItemActionListenersNilHub pins the nil-hub
// degradation: the hook seams stay unset.
func TestInstallItemActionListenersNilHub(t *testing.T) {
	var deps listeners.Deps
	InstallItemActionListeners(&deps, nil)
	if deps.ItemActionHooks.EmitItemAction != nil || deps.ItemActionHooks.EmitCanUseItem != nil {
		t.Fatal("nil hub installed a hub seam")
	}
	// A nil deps is a no-op, not a panic.
	InstallItemActionListeners(nil, nil)
}

// TestInstallItemActionListenersTranslates pins the
// listener-to-hub event translation: every metadata field
// reaches the hook, the hook's claim copies back, and the
// gate answer passes through.
func TestInstallItemActionListenersTranslates(t *testing.T) {
	var got *plugins.PluginItemActionEvent
	var gated *plugins.CanUseItemEvent
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "ItemProbe",
		Register: func(api *plugins.Api) {
			api.OnItemAction(func(ev *plugins.PluginItemActionEvent) {
				got = ev
				ev.Handled = true
			})
			api.OnCanUseItem(func(ev *plugins.CanUseItemEvent) {
				gated = ev
				ev.Allow = plugins.BoolPtr(false)
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()

	var deps listeners.Deps
	InstallItemActionListeners(&deps, hub)
	p := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	ev := &listeners.ItemActionEvent{
		Player:      p,
		InterfaceID: game.InventoryInterfaceID,
		ItemID:      5001,
		Slot:        3,
		ClickType:   2,
		Option:      "Eat",
	}
	if !deps.ItemActionHooks.EmitItemAction(ev) {
		t.Fatal("emit returned unhandled")
	}
	if !ev.Handled {
		t.Fatal("Handled did not copy back to the listener event")
	}
	if got == nil {
		t.Fatal("hook never ran")
	}
	if got.Player != p || got.InterfaceID != game.InventoryInterfaceID ||
		got.ItemID != 5001 || got.Slot != 3 || got.ClickType != 2 || got.Option != "Eat" {
		t.Fatalf("translated event = %+v, want all fields copied", got)
	}
	allow := deps.ItemActionHooks.EmitCanUseItem(p, 5001, "action", "Eat")
	if allow == nil || *allow {
		t.Fatalf("gate answer = %v, want false", allow)
	}
	if gated == nil {
		t.Fatal("gate hook never ran")
	}
	if gated.Player != p || gated.ItemID != 5001 || gated.Action != "action" || gated.Option != "Eat" {
		t.Fatalf("gate event = %+v, want the listener's player/item/action/option", gated)
	}
}

// TestInstallItemActionListenersFirstAction pins the
// first-action family through the adapter: a clickType-1
// hook registered via OnItemFirstAction claims through the
// listener emit.
func TestInstallItemActionListenersFirstAction(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "FirstProbe",
		Register: func(api *plugins.Api) {
			api.OnItemFirstAction(func(ev *plugins.PluginItemActionEvent) bool {
				return true
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()

	var deps listeners.Deps
	InstallItemActionListeners(&deps, hub)
	p := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	first := &listeners.ItemActionEvent{
		Player: p, InterfaceID: game.InventoryInterfaceID,
		ItemID: 5001, Slot: 0, ClickType: 1,
	}
	if !deps.ItemActionHooks.EmitItemAction(first) {
		t.Fatal("first-action hook did not claim the clickType-1 event")
	}
	second := &listeners.ItemActionEvent{
		Player: p, InterfaceID: game.InventoryInterfaceID,
		ItemID: 5001, Slot: 0, ClickType: 2, Option: "Eat",
	}
	if deps.ItemActionHooks.EmitItemAction(second) {
		t.Fatal("first-action hook claimed a clickType-2 event")
	}
}

// TestInstallItemActionResolversNilSafe pins the nil
// degradation: a nil hub or nil store installs nothing and
// never panics. The resolver read itself (InventoryActions
// over the cache ObjType) is covered by the
// storeItemDefinitions tests behind containerswiring; a
// zero Store resolves every id to the DEFAULT shape, so an
// install over one is a no-op in behavior as well.
func TestInstallItemActionResolversNilSafe(t *testing.T) {
	hub := plugins.NewHub(nil, plugins.HubOptions{Logger: discardLogger()})
	InstallItemActionResolvers(nil, &definition.Store{}, nil)
	InstallItemActionResolvers(hub, nil, nil)
	InstallItemActionResolvers(hub, &definition.Store{}, nil)
}
