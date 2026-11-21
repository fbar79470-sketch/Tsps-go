package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// TestObjectHooksItemOnItemTranslates pins the
// listener-to-hub event translation for the onItemOnItem
// family (phase 33): every field reaches the hook, the
// hook's claim copies back, and a nil hub yields the zero
// (no-hooks) hook set.
func TestObjectHooksItemOnItemTranslates(t *testing.T) {
	var got *plugins.PluginItemOnItemEvent
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "ItemOnItemProbe",
		Register: func(api *plugins.Api) {
			api.OnItemOnItem(func(ev *plugins.PluginItemOnItemEvent) {
				got = ev
				ev.Handled = true
			}, nil)
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()

	hooks := ObjectHooks(hub)
	if hooks.EmitItemOnItem == nil {
		t.Fatal("ObjectHooks must install EmitItemOnItem")
	}
	p := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	ev := &listeners.ItemOnItemEvent{
		Player:           p,
		UsedItem:         game.NewItem(23962, 5),
		UsedItemID:       23962,
		UsedItemSlot:     0,
		UsedWithItem:     game.NewItem(23673, 1),
		UsedWithItemID:   23673,
		UsedWithItemSlot: 1,
	}
	if !hooks.EmitItemOnItem(ev) {
		t.Fatal("emit returned unhandled")
	}
	if !ev.Handled {
		t.Fatal("Handled did not copy back to the listener event")
	}
	if got == nil {
		t.Fatal("hook never ran")
	}
	if got.Player != p || got.UsedItemID != 23962 || got.UsedItemSlot != 0 ||
		got.UsedWithItemID != 23673 || got.UsedWithItemSlot != 1 {
		t.Fatalf("translated event = %+v, want all fields copied", got)
	}
	if got.UsedItem.ID() != 23962 || got.UsedItem.Amount() != 5 ||
		got.UsedWithItem.ID() != 23673 || got.UsedWithItem.Amount() != 1 {
		t.Fatalf("translated items = %v / %v, want 23962x5 / 23673x1", got.UsedItem, got.UsedWithItem)
	}

	// A nil hub yields the zero value: no emit is installed.
	if ObjectHooks(nil).EmitItemOnItem != nil {
		t.Fatal("nil hub installed an item-on-item seam")
	}
}
