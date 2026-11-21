package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

// World-entity interaction hub tests: registration
// (PluginManager.ts:3477-3480, typeof-function check only, no
// guard wrapper) and emit semantics (PluginManager.ts:1265-1276,
// first handled hook wins; Api passthrough 4696). Event shape:
// PluginTypes.ts:199-204. Core members: PluginTypes.ts:1681-1682.

func TestOnWorldEntityInteractionOrderingAndHandled(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "First").OnWorldEntityInteraction(func(ev *WorldEntityInteractionEvent) {
		order = append(order, "First")
	})
	newApi(h, "Second").OnWorldEntityInteraction(func(ev *WorldEntityInteractionEvent) {
		order = append(order, "Second")
		ev.Handled = true
	})
	newApi(h, "Third").OnWorldEntityInteraction(func(ev *WorldEntityInteractionEvent) {
		order = append(order, "Third")
	})
	p := testPlayer("alice")
	handled := h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: p, EntityIndex: 3000, ClickType: 2})
	if !handled {
		t.Fatal("the emit must return handled once a hook sets it")
	}
	if len(order) != 2 || order[0] != "First" || order[1] != "Second" {
		t.Fatalf("order = %v, want [First Second]", order)
	}
}

func TestOnWorldEntityInteractionUnhandled(t *testing.T) {
	h, _ := newTestHub()
	seen := 0
	newApi(h, "Watcher").OnWorldEntityInteraction(func(*WorldEntityInteractionEvent) { seen++ })
	handled := h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice"), EntityIndex: 3000, ClickType: 1})
	if handled || seen != 1 {
		t.Fatalf("handled=%v seen=%d, want false/1", handled, seen)
	}
}

func TestOnWorldEntityInteractionPreHandled(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Watcher").OnWorldEntityInteraction(func(*WorldEntityInteractionEvent) { calls++ })
	// TS checks handled before each hook, so a pre-handled
	// event runs zero hooks and still returns true.
	handled := h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice"), EntityIndex: 3000, ClickType: 1, Handled: true})
	if !handled || calls != 0 {
		t.Fatalf("handled=%v calls=%d, want true/0", handled, calls)
	}
}

func TestOnWorldEntityInteractionNilGuards(t *testing.T) {
	h, _ := newTestHub()
	// A nil handler registers nothing.
	newApi(h, "Nil").OnWorldEntityInteraction(nil)
	calls := 0
	newApi(h, "Watcher").OnWorldEntityInteraction(func(*WorldEntityInteractionEvent) { calls++ })
	if h.EmitWorldEntityInteraction(nil) {
		t.Error("a nil event must return false")
	}
	if h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{EntityIndex: 3000, ClickType: 1}) {
		t.Error("a player-less event must return false")
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
	// A nil hub is safe.
	var nilHub *Hub
	if nilHub.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice")}) {
		t.Error("a nil hub must return false")
	}
}

func TestOnWorldEntityInteractionPanicIsolated(t *testing.T) {
	h, _ := newTestHub()
	second := false
	newApi(h, "Panicker").OnWorldEntityInteraction(func(*WorldEntityInteractionEvent) { panic("boom") })
	newApi(h, "Second").OnWorldEntityInteraction(func(ev *WorldEntityInteractionEvent) {
		second = true
		ev.Handled = true
	})
	handled := h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice"), EntityIndex: 3000, ClickType: 1})
	if !handled || !second {
		t.Fatalf("handled=%v second=%v, want the emit to survive a panicking hook", handled, second)
	}
}

func TestOnWorldEntityInteractionAfterLoadIgnored(t *testing.T) {
	h, _ := newTestHub()
	h.Load() // seals the hub.
	calls := 0
	newApi(h, "Late").OnWorldEntityInteraction(func(*WorldEntityInteractionEvent) { calls++ })
	h.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice"), EntityIndex: 3000, ClickType: 1})
	if calls != 0 {
		t.Error("registration after Load must be ignored")
	}
}

func TestApiEmitWorldEntityInteractionDelegates(t *testing.T) {
	h, _ := newTestHub()
	var seen *WorldEntityInteractionEvent
	newApi(h, "Watcher").OnWorldEntityInteraction(func(ev *WorldEntityInteractionEvent) {
		seen = ev
		ev.Handled = true
	})
	api := newApi(h, "Caller")
	handled := api.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice"), EntityIndex: 3001, ClickType: 4})
	if !handled || seen == nil || seen.EntityIndex != 3001 || seen.ClickType != 4 {
		t.Fatalf("handled=%v seen=%+v", handled, seen)
	}
	// A nil Api is safe.
	var nilApi *Api
	if nilApi.EmitWorldEntityInteraction(&WorldEntityInteractionEvent{Player: testPlayer("alice")}) {
		t.Error("a nil Api must return false")
	}
}

func TestCoreSailing(t *testing.T) {
	m := game.NewBoatManager(nil)
	s := game.NewSailing(m, game.Location{X: 3089, Y: 3524, Z: 0})
	h, _ := newTestHub()
	if got := newApi(h, "Caller").Core().Sailing(); got != nil {
		t.Fatalf("Core.Sailing() = %v, want nil with none installed", got)
	}
	h.SetSailing(s)
	if got := newApi(h, "Caller").Core().Sailing(); got != s {
		t.Fatalf("Core.Sailing() = %v, want the installed Sailing", got)
	}
	h.SetSailing(nil)
	if got := newApi(h, "Caller").Core().Sailing(); got != nil {
		t.Errorf("Core.Sailing() = %v, want nil after clearing", got)
	}
}

func TestCoreBoatMoveMode(t *testing.T) {
	h, _ := newTestHub()
	modes := newApi(h, "Caller").Core().BoatMoveMode()
	if modes.Stopped() != game.BoatMoveModeStopped || modes.Half() != game.BoatMoveModeHalf ||
		modes.Full() != game.BoatMoveModeFull || modes.Reverse() != game.BoatMoveModeReverse {
		t.Errorf("modes = %d/%d/%d/%d, want %d/%d/%d/%d", modes.Stopped(), modes.Half(), modes.Full(), modes.Reverse(),
			game.BoatMoveModeStopped, game.BoatMoveModeHalf, game.BoatMoveModeFull, game.BoatMoveModeReverse)
	}
}
