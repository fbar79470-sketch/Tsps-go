package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

func buttonTestPlayer() *game.Player {
	return interfaceTestPlayer()
}

func TestOnButtonClickOrderingAcrossLists(t *testing.T) {
	// The emit runs the onButtonClick hooks first, then the
	// onButton hooks (PluginManager.ts:2052-2074), each list
	// short-circuiting on handled — and a handled event never
	// reaches the second list.
	h, _ := newTestHub()
	var order []string
	newApi(h, "A").OnButtonClick(func(ev *ButtonClickEvent) {
		order = append(order, "click-a")
	})
	newApi(h, "B").OnButtonClick(func(ev *ButtonClickEvent) {
		order = append(order, "click-b")
	})
	newApi(h, "C").OnButton([]int{100}, func(ev *ButtonClickEvent) bool {
		order = append(order, "button-c")
		return true
	})
	newApi(h, "D").OnButton([]int{100}, func(ev *ButtonClickEvent) bool {
		order = append(order, "button-d")
		return true
	})
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 100}
	if !h.EmitButtonClick(ev) {
		t.Fatal("emit returned false, want handled")
	}
	if strings.Join(order, ",") != "click-a,click-b,button-c" {
		t.Fatalf("order = %v, want [click-a click-b button-c] (button-d short-circuited)", order)
	}
	if !ev.Handled {
		t.Error("event not marked handled")
	}
}

func TestOnButtonClickClaimSkipsOnButtonList(t *testing.T) {
	h, _ := newTestHub()
	ranButton := false
	newApi(h, "A").OnButtonClick(func(ev *ButtonClickEvent) {
		ev.Handled = true
	})
	newApi(h, "B").OnButton([]int{100}, func(ev *ButtonClickEvent) bool {
		ranButton = true
		return true
	})
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 100}
	if !h.EmitButtonClick(ev) {
		t.Fatal("emit returned false, want handled")
	}
	if ranButton {
		t.Fatal("onButton hook ran after onButtonClick claimed")
	}
}

func TestOnButtonClickUnclaimedRunsAll(t *testing.T) {
	h, _ := newTestHub()
	ran := 0
	for i := 0; i < 2; i++ {
		newApi(h, "P").OnButtonClick(func(ev *ButtonClickEvent) { ran++ })
	}
	newApi(h, "P").OnButton([]int{100}, func(ev *ButtonClickEvent) bool { ran++; return false })
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 100}
	if h.EmitButtonClick(ev) {
		t.Fatal("emit returned true with no claim")
	}
	if ran != 3 {
		t.Fatalf("ran = %d, want 3", ran)
	}
}

func TestOnButtonFilterAndClaim(t *testing.T) {
	h, _ := newTestHub()
	var seen []int
	newApi(h, "P").OnButton([]int{100, 200}, func(ev *ButtonClickEvent) bool {
		seen = append(seen, ev.ButtonID)
		return true
	})
	// Matching id claims.
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 200}
	if !h.EmitButtonClick(ev) || !ev.Handled {
		t.Fatal("matching button did not claim")
	}
	// Non-matching id never runs the handler.
	ev = &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 300}
	if h.EmitButtonClick(ev) {
		t.Fatal("non-matching button claimed")
	}
	if len(seen) != 1 || seen[0] != 200 {
		t.Fatalf("seen = %v, want [200]", seen)
	}
}

func TestOnButtonFalseFallsThrough(t *testing.T) {
	// A false return falls through to the next hook in the
	// same list; a later true claims.
	h, _ := newTestHub()
	var order []string
	newApi(h, "A").OnButton([]int{100}, func(ev *ButtonClickEvent) bool {
		order = append(order, "a")
		return false
	})
	newApi(h, "B").OnButton([]int{100}, func(ev *ButtonClickEvent) bool {
		order = append(order, "b")
		return true
	})
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 100}
	if !h.EmitButtonClick(ev) {
		t.Fatal("emit returned false")
	}
	if strings.Join(order, ",") != "a,b" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

func TestOnButtonInvalidIDs(t *testing.T) {
	h, buf := newTestHub()
	ran := false
	newApi(h, "P").OnButton([]int{-1, -5}, func(ev *ButtonClickEvent) bool {
		ran = true
		return true
	})
	if !strings.Contains(buf.String(), "attempted invalid button hook registration") {
		t.Fatalf("log = %q, want invalid-registration warning", buf.String())
	}
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 0}
	if h.EmitButtonClick(ev) || ran {
		t.Fatal("invalid-id registration ran")
	}

	// Partially invalid: keeps the valid ids, warns.
	buf.Reset()
	newApi(h, "P").OnButton([]int{50, -1, 50}, func(ev *ButtonClickEvent) bool {
		return true
	})
	if !strings.Contains(buf.String(), "dropped invalid ids (kept 1/3)") {
		t.Fatalf("log = %q, want dropped-ids warning", buf.String())
	}
	ev = &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 50}
	if !h.EmitButtonClick(ev) {
		t.Fatal("kept id 50 did not claim")
	}
}

func TestButtonNilHandlerIgnored(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "P").OnButtonClick(nil)
	newApi(h, "P").OnButton([]int{1}, nil)
	ev := &ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 1}
	if h.EmitButtonClick(ev) {
		t.Fatal("nil handlers claimed")
	}
}

func TestEmitButtonClickEntryGuards(t *testing.T) {
	h, _ := newTestHub()
	ran := false
	newApi(h, "P").OnButtonClick(func(ev *ButtonClickEvent) { ran = true })
	if h.EmitButtonClick(nil) {
		t.Fatal("nil event claimed")
	}
	if h.EmitButtonClick(&ButtonClickEvent{}) {
		t.Fatal("nil player claimed")
	}
	if h.EmitButtonClick(&ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 1, Handled: true}) {
		t.Fatal("pre-handled event claimed")
	}
	if ran {
		t.Fatal("guard case ran a hook")
	}
	if (&Hub{}).EmitButtonClick(&ButtonClickEvent{Player: buttonTestPlayer(), ButtonID: 1}) {
		t.Fatal("zero-value hub claimed")
	}
}
