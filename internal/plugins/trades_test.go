package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Trade-family tests: the registration methods, emits, gate
// contracts, and TradeState surface in trades.go, grounded in
// PluginManager.ts (emitCanTrade 1456-1469, emitTradeRequest
// 1471-1481, emitTradeCompleted 1483-1491, registration guards
// 3673-3714), PluginTypes.ts (event shapes 417-442), and
// Trading.ts (canTrade 142-147, acceptTrade dual emit 323-330,
// TradeState enum 655). Fixtures only: newTestHub and testPlayer
// come from api_test.go / hooks_test.go.

func TestOnCanTradeFirstAnswer(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Gate")
	calls := 0
	a.OnCanTrade(func(ev *CanTradeEvent) {
		calls++
		// Silence: no answer.
	})
	a.OnCanTrade(func(ev *CanTradeEvent) {
		calls++
		ev.Allow = BoolPtr(false)
	})
	a.OnCanTrade(func(ev *CanTradeEvent) {
		calls++
		ev.Allow = BoolPtr(true)
	})
	got := h.EmitCanTrade(testPlayer("alice"), testPlayer("bob"))
	if got == nil || *got != false {
		t.Fatalf("EmitCanTrade = %v, want false (first answer wins)", got)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third hook never runs)", calls)
	}
	// Silence and zero hooks yield nil (the trade is allowed).
	h2, _ := newTestHub()
	newApi(h2, "Silent").OnCanTrade(func(*CanTradeEvent) {})
	if got := h2.EmitCanTrade(testPlayer("alice"), testPlayer("bob")); got != nil {
		t.Fatalf("silent EmitCanTrade = %v, want nil", got)
	}
	h3, _ := newTestHub()
	if got := h3.EmitCanTrade(testPlayer("alice"), testPlayer("bob")); got != nil {
		t.Fatalf("zero-hook EmitCanTrade = %v, want nil", got)
	}
}

func TestOnCanTradeWrapper(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Watcher").OnCanTrade(func(*CanTradeEvent) { calls++ })
	// A nil player or target runs no handler (the registration
	// wrapper), and the emit yields nil.
	if got := h.EmitCanTrade(nil, testPlayer("bob")); got != nil {
		t.Fatalf("nil-player EmitCanTrade = %v, want nil", got)
	}
	if got := h.EmitCanTrade(testPlayer("alice"), nil); got != nil {
		t.Fatalf("nil-target EmitCanTrade = %v, want nil", got)
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0", calls)
	}
	var nilHub *Hub
	if got := nilHub.EmitCanTrade(testPlayer("alice"), testPlayer("bob")); got != nil {
		t.Fatalf("nil-hub EmitCanTrade = %v, want nil", got)
	}
	newApi(h, "Nil").OnCanTrade(nil)
}

func TestOnTradeRequestShortCircuit(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Request")
	calls := 0
	a.OnTradeRequest(func(ev *TradeRequestEvent) {
		calls++
		// Silence: unhandled.
	})
	a.OnTradeRequest(func(ev *TradeRequestEvent) {
		calls++
		ev.Handled = true
	})
	a.OnTradeRequest(func(ev *TradeRequestEvent) {
		calls++
	})
	ev := &TradeRequestEvent{Player: testPlayer("alice"), Target: testPlayer("bob")}
	if !h.EmitTradeRequest(ev) {
		t.Fatal("EmitTradeRequest must return true once handled")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (third hook never runs)", calls)
	}
	if !ev.Handled {
		t.Fatal("the event must stay handled after the emit")
	}
	// Unhandled: false, all hooks ran.
	h2, _ := newTestHub()
	calls2 := 0
	newApi(h2, "Silent").OnTradeRequest(func(*TradeRequestEvent) { calls2++ })
	newApi(h2, "Silent2").OnTradeRequest(func(*TradeRequestEvent) { calls2++ })
	ev2 := &TradeRequestEvent{Player: testPlayer("alice"), Target: testPlayer("bob")}
	if h2.EmitTradeRequest(ev2) {
		t.Fatal("unhandled EmitTradeRequest must return false")
	}
	if calls2 != 2 {
		t.Fatalf("calls = %d, want 2 (unhandled runs all)", calls2)
	}
}

func TestOnTradeRequestEntryGuards(t *testing.T) {
	h, _ := newTestHub()
	calls := 0
	newApi(h, "Watcher").OnTradeRequest(func(*TradeRequestEvent) { calls++ })
	// Pre-handled: false, zero hooks run (unlike the bare gate
	// loop shape — the entry guard returns false).
	pre := &TradeRequestEvent{Player: testPlayer("alice"), Target: testPlayer("bob"), Handled: true}
	if h.EmitTradeRequest(pre) {
		t.Fatal("pre-handled EmitTradeRequest must return false")
	}
	// Nil event / missing player / missing target: false.
	if h.EmitTradeRequest(nil) {
		t.Fatal("nil-event EmitTradeRequest must return false")
	}
	if h.EmitTradeRequest(&TradeRequestEvent{Target: testPlayer("bob")}) {
		t.Fatal("player-less EmitTradeRequest must return false")
	}
	if h.EmitTradeRequest(&TradeRequestEvent{Player: testPlayer("alice")}) {
		t.Fatal("target-less EmitTradeRequest must return false")
	}
	if calls != 0 {
		t.Fatalf("calls = %d, want 0 (guards run no hooks)", calls)
	}
	var nilHub *Hub
	if nilHub.EmitTradeRequest(&TradeRequestEvent{Player: testPlayer("a"), Target: testPlayer("b")}) {
		t.Fatal("nil-hub EmitTradeRequest must return false")
	}
	newApi(h, "Nil").OnTradeRequest(nil)
}

func TestOnTradeCompletedRunAll(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Completed")
	calls := 0
	a.OnTradeCompleted(func(ev *TradeCompletedEvent) {
		calls++
		if len(ev.Received) != 1 || ev.Received[0].ID() != 995 {
			t.Errorf("received = %+v, want one coins stack", ev.Received)
		}
		if len(ev.Given) != 1 || ev.Given[0].ID() != 4151 {
			t.Errorf("given = %+v, want one whip stack", ev.Given)
		}
	})
	a.OnTradeCompleted(func(*TradeCompletedEvent) { calls++ })
	// A panicking hook is isolated: the next hook still runs.
	a.OnTradeCompleted(func(*TradeCompletedEvent) { panic("boom") })
	a.OnTradeCompleted(func(*TradeCompletedEvent) { calls++ })
	ev := &TradeCompletedEvent{
		Player:   testPlayer("alice"),
		Partner:  testPlayer("bob"),
		Received: []game.Item{game.NewItem(995, 1000)},
		Given:    []game.Item{game.NewItem(4151, 1)},
	}
	h.EmitTradeCompleted(ev)
	if calls != 3 {
		t.Fatalf("calls = %d, want 3 (run-all, panic isolated)", calls)
	}
	// Guards: nil event / missing player / missing partner run
	// no hooks.
	calls = 0
	h.EmitTradeCompleted(nil)
	h.EmitTradeCompleted(&TradeCompletedEvent{Partner: testPlayer("bob")})
	h.EmitTradeCompleted(&TradeCompletedEvent{Player: testPlayer("alice")})
	if calls != 0 {
		t.Fatalf("calls after guarded emits = %d, want 0", calls)
	}
	var nilHub *Hub
	nilHub.EmitTradeCompleted(ev)
	newApi(h, "Nil").OnTradeCompleted(nil)
}

func TestTradeRegistrationAfterLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	newApi(h, "Late").OnCanTrade(func(*CanTradeEvent) {})
	newApi(h, "Late").OnTradeRequest(func(*TradeRequestEvent) {})
	newApi(h, "Late").OnTradeCompleted(func(*TradeCompletedEvent) {})
	for _, family := range []string{"onCanTrade", "onTradeRequest", "onTradeCompleted"} {
		if !strings.Contains(buf.String(), family) {
			t.Fatalf("log = %q, want the post-load refusal for %s", buf.String(), family)
		}
	}
}

func TestTradeStateOrder(t *testing.T) {
	// Trading.ts enum order: NONE, REQUESTED_TRADE, TRADE_SCREEN,
	// ACCEPTED_TRADE_SCREEN, CONFIRM_SCREEN, ACCEPTED_CONFIRM_SCREEN.
	if TradeStateNone != 0 || TradeStateRequested != 1 || TradeStateScreen != 2 ||
		TradeStateAcceptedScreen != 3 || TradeStateConfirm != 4 || TradeStateAcceptedConfirm != 5 {
		t.Fatalf("TradeState values = %d/%d/%d/%d/%d/%d, want 0/1/2/3/4/5",
			TradeStateNone, TradeStateRequested, TradeStateScreen,
			TradeStateAcceptedScreen, TradeStateConfirm, TradeStateAcceptedConfirm)
	}
	// The ordinal comparisons TS relies on (Trading.ts:229, 575).
	if !(TradeStateScreen >= TradeStateScreen) || !(TradeStateConfirm >= TradeStateConfirm) {
		t.Fatal("TradeState ordinals must support the >= stage comparisons")
	}
	if TradeStateRequested >= TradeStateScreen {
		t.Fatal("REQUESTED_TRADE must sort before TRADE_SCREEN")
	}
}

func TestCoreTradeState(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "Trader").Core()
	// Uninstalled: no state to read.
	if state, ok := core.TradeState(testPlayer("alice")); ok || state != TradeStateNone {
		t.Fatalf("uninstalled TradeState = %d/%v, want 0/false", state, ok)
	}
	var nilCore *Core
	if _, ok := nilCore.TradeState(testPlayer("alice")); ok {
		t.Fatal("nil-Core TradeState must report false")
	}
	// Installed: the reader answers, and its false propagates.
	h.SetTradeStateReader(func(player *game.Player) (TradeState, bool) {
		if player == nil {
			return TradeStateNone, false
		}
		return TradeStateConfirm, true
	})
	if state, ok := core.TradeState(testPlayer("alice")); !ok || state != TradeStateConfirm {
		t.Fatalf("TradeState = %d/%v, want 4/true", state, ok)
	}
	if _, ok := core.TradeState(nil); ok {
		t.Fatal("nil-player TradeState must report false")
	}
	// Clearing uninstalls.
	h.SetTradeStateReader(nil)
	if _, ok := core.TradeState(testPlayer("alice")); ok {
		t.Fatal("cleared TradeState must report false")
	}
	if h.TradeStateReader() != nil {
		t.Fatal("TradeStateReader getter must be nil after clearing")
	}
}
