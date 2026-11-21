package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

func interfaceTestPlayer() *game.Player {
	p := testDialoguePlayer()
	p.Session = &promptTestSession{chatboxID: -1}
	return p
}

func TestOnInterfaceActionClickOrderingAndShortCircuit(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "A").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		order = append(order, "a")
	})
	newApi(h, "B").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		order = append(order, "b")
		ev.Handled = true
	})
	newApi(h, "C").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		order = append(order, "c")
	})
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 100, Action: 1}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("emit returned false, want handled")
	}
	if strings.Join(order, ",") != "a,b" {
		t.Fatalf("order = %v, want [a b] (short-circuit after B claims)", order)
	}
	if !ev.Handled {
		t.Error("event not marked handled")
	}
}

func TestOnInterfaceActionClickUnclaimedRunsAll(t *testing.T) {
	h, _ := newTestHub()
	ran := 0
	for i := 0; i < 3; i++ {
		newApi(h, "P").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) { ran++ })
	}
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 100, Action: 1}
	if h.EmitInterfaceActionClick(ev) {
		t.Fatal("emit returned true with no claim")
	}
	if ran != 3 {
		t.Fatalf("ran = %d, want 3", ran)
	}
}

func TestOnInterfaceActionButtonFilterAndClaim(t *testing.T) {
	h, _ := newTestHub()
	var seen []int
	newApi(h, "P").OnInterfaceActionButton([]int{100, 200}, func(ev *InterfaceActionClickEvent) bool {
		seen = append(seen, ev.ButtonID)
		return true
	})
	// Matching id claims.
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 200, Action: 1}
	if !h.EmitInterfaceActionClick(ev) || !ev.Handled {
		t.Fatal("matching button did not claim")
	}
	// Non-matching id never runs the handler.
	ev = &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 300, Action: 1}
	if h.EmitInterfaceActionClick(ev) {
		t.Fatal("non-matching button claimed")
	}
	if len(seen) != 1 || seen[0] != 200 {
		t.Fatalf("seen = %v, want [200]", seen)
	}
}

func TestOnInterfaceActionButtonFalseFallsThrough(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "A").OnInterfaceActionButton([]int{100}, func(ev *InterfaceActionClickEvent) bool {
		order = append(order, "a")
		return false
	})
	newApi(h, "B").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		order = append(order, "b")
		ev.Handled = true
	})
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 100, Action: 1}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("emit returned false")
	}
	if strings.Join(order, ",") != "a,b" {
		t.Fatalf("order = %v, want [a b]", order)
	}
}

func TestInterfaceActionSharedRegistrationOrder(t *testing.T) {
	// Plain and button hooks share ONE list in registration
	// order (PluginManager.ts:394-396): a button hook
	// registered between two plain hooks runs between them.
	h, _ := newTestHub()
	var order []string
	newApi(h, "A").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) { order = append(order, "plain1") })
	newApi(h, "B").OnInterfaceActionButton([]int{100}, func(ev *InterfaceActionClickEvent) bool {
		order = append(order, "button")
		return false
	})
	newApi(h, "C").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) { order = append(order, "plain2") })
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 100, Action: 1}
	h.EmitInterfaceActionClick(ev)
	if strings.Join(order, ",") != "plain1,button,plain2" {
		t.Fatalf("order = %v, want [plain1 button plain2]", order)
	}
}

func TestOnInterfaceActionButtonInvalidIDs(t *testing.T) {
	h, buf := newTestHub()
	ran := false
	newApi(h, "P").OnInterfaceActionButton([]int{-1, -5}, func(ev *InterfaceActionClickEvent) bool {
		ran = true
		return true
	})
	if !strings.Contains(buf.String(), "attempted invalid interface_action_button") {
		t.Fatalf("log = %q, want invalid-registration warning", buf.String())
	}
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 0, Action: 1}
	if h.EmitInterfaceActionClick(ev) || ran {
		t.Fatal("invalid-id registration ran")
	}

	// Partially invalid: keeps the valid ids, warns.
	buf.Reset()
	newApi(h, "P").OnInterfaceActionButton([]int{50, -1, 50}, func(ev *InterfaceActionClickEvent) bool {
		return true
	})
	if !strings.Contains(buf.String(), "dropped invalid ids (kept 1/3)") {
		t.Fatalf("log = %q, want dropped-ids warning", buf.String())
	}
	ev = &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 50, Action: 1}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("kept id 50 did not claim")
	}
}

func TestInterfaceActionNilHandlerIgnored(t *testing.T) {
	h, _ := newTestHub()
	newApi(h, "P").OnInterfaceActionClick(nil)
	newApi(h, "P").OnInterfaceActionButton([]int{1}, nil)
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 1, Action: 1}
	if h.EmitInterfaceActionClick(ev) {
		t.Fatal("nil handlers claimed")
	}
}

func TestEmitInterfaceActionClickEntryGuards(t *testing.T) {
	h, _ := newTestHub()
	ran := false
	newApi(h, "P").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) { ran = true })
	if h.EmitInterfaceActionClick(nil) {
		t.Error("nil event returned true")
	}
	if h.EmitInterfaceActionClick(&InterfaceActionClickEvent{ButtonID: 1, Action: 1}) {
		t.Error("nil player returned true")
	}
	if h.EmitInterfaceActionClick(&InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 1, Action: 1, Handled: true}) {
		t.Error("pre-handled event returned true")
	}
	if ran {
		t.Error("guarded emit ran a hook")
	}
	var nilHub *Hub
	if nilHub.EmitInterfaceActionClick(&InterfaceActionClickEvent{Player: interfaceTestPlayer()}) {
		t.Error("nil hub returned true")
	}
}

func TestEmitInterfaceActionClickPanicIsolation(t *testing.T) {
	h, buf := newTestHub()
	newApi(h, "Bad").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		panic("boom")
	})
	ran := false
	newApi(h, "Good").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		ran = true
		ev.Handled = true
	})
	ev := &InterfaceActionClickEvent{Player: interfaceTestPlayer(), ButtonID: 1, Action: 1}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("emit returned false after panic isolation")
	}
	if !ran {
		t.Error("hook after the panicking hook did not run")
	}
	if !strings.Contains(buf.String(), "interface_action_click hook failed (Bad)") {
		t.Fatalf("log = %q, want panic line", buf.String())
	}
}

func TestRefuseWhileBusy(t *testing.T) {
	h, _ := newTestHub()
	if h.RefusesWhileBusy(100) {
		t.Fatal("empty hub refuses")
	}
	newApi(h, "P").RefuseWhileBusy([]int{100, 200})
	if !h.RefusesWhileBusy(100) || !h.RefusesWhileBusy(200) {
		t.Fatal("registered ids not refused")
	}
	if h.RefusesWhileBusy(300) {
		t.Fatal("unregistered id refused")
	}
	// Negatives are skipped silently.
	newApi(h, "P").RefuseWhileBusy([]int{-1})
	if h.RefusesWhileBusy(-1) {
		t.Fatal("negative id refused")
	}
	var nilHub *Hub
	if nilHub.RefusesWhileBusy(100) {
		t.Error("nil hub refuses")
	}
}

func TestEmitInterfaceActionClickPromptFirst(t *testing.T) {
	h, _ := newTestHub()
	prompts := game.NewMultiChatboxPrompt(nil)
	h.SetMultiChatboxPrompt(prompts)
	p := interfaceTestPlayer()
	picked := -1
	if !prompts.ShowPrompt("P", p, "Pick", []game.MenuOption{
		{Label: "First", Select: func() { picked = 0 }},
		{Label: "Second", Select: func() { picked = 1 }},
	}) {
		t.Fatal("ShowPrompt reported false")
	}
	hookRan := false
	newApi(h, "P").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) { hookRan = true })
	// Option 2 via the action form: buttonId is the options
	// widget (219 << 16) | 1, action is the 1-based option.
	ev := &InterfaceActionClickEvent{
		Player: p, ButtonID: (219 << 16) | 1, Action: 2,
		GroupID: 219, HasGroupID: true, ChildID: 1, HasChildID: true,
	}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("prompt click did not claim")
	}
	if picked != 1 {
		t.Fatalf("picked = %d, want 1", picked)
	}
	if hookRan {
		t.Error("hook ran after the prompt consumed the click")
	}
	if !ev.Handled {
		t.Error("event not marked handled by the prompt check")
	}
}

func TestEmitInterfaceActionClickPromptSlotForm(t *testing.T) {
	// The slot form: the option number comes from the slot
	// when present, not the action (MultiChatboxPrompt.ts:
	// 141-145) — the local-trigger/widget path.
	h, _ := newTestHub()
	prompts := game.NewMultiChatboxPrompt(nil)
	h.SetMultiChatboxPrompt(prompts)
	p := interfaceTestPlayer()
	picked := -1
	if !prompts.ShowPrompt("P", p, "Pick", []game.MenuOption{
		{Label: "First", Select: func() { picked = 0 }},
		{Label: "Second", Select: func() { picked = 1 }},
	}) {
		t.Fatal("ShowPrompt reported false")
	}
	ev := &InterfaceActionClickEvent{
		Player: p, ButtonID: (219 << 16) | 1, Action: 99,
		GroupID: 219, HasGroupID: true, Slot: 1, HasSlot: true,
	}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("slot-form prompt click did not claim")
	}
	if picked != 0 {
		t.Fatalf("picked = %d, want 0 (slot 1)", picked)
	}
}

func TestEmitInterfaceActionClickPromptMissFallsToHooks(t *testing.T) {
	h, _ := newTestHub()
	prompts := game.NewMultiChatboxPrompt(nil)
	h.SetMultiChatboxPrompt(prompts)
	p := interfaceTestPlayer()
	if !prompts.ShowPrompt("P", p, "Pick", []game.MenuOption{
		{Label: "First", Select: func() {}},
		{Label: "Second", Select: func() {}},
	}) {
		t.Fatal("ShowPrompt reported false")
	}
	hookRan := false
	newApi(h, "P").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		hookRan = true
		ev.Handled = true
	})
	// A click on another group does not touch the prompt.
	ev := &InterfaceActionClickEvent{
		Player: p, ButtonID: (500 << 16) | 3, Action: 1,
		GroupID: 500, HasGroupID: true,
	}
	if !h.EmitInterfaceActionClick(ev) {
		t.Fatal("non-prompt click did not reach hooks")
	}
	if !hookRan {
		t.Error("hook did not run for a non-prompt click")
	}
	// The prompt is still pending.
	if _, _, ok := prompts.GetPending(p); !ok {
		t.Error("pending prompt lost after an unrelated click")
	}
}

func TestEmitInterfaceActionClickNoRegistrySkipsPrompt(t *testing.T) {
	h, _ := newTestHub()
	hookRan := false
	newApi(h, "P").OnInterfaceActionClick(func(ev *InterfaceActionClickEvent) {
		hookRan = true
		ev.Handled = true
	})
	ev := &InterfaceActionClickEvent{
		Player: interfaceTestPlayer(), ButtonID: (219 << 16) | 1, Action: 1,
		GroupID: 219, HasGroupID: true,
	}
	if !h.EmitInterfaceActionClick(ev) || !hookRan {
		t.Fatal("no-registry emit did not reach hooks")
	}
}

func TestInterfaceCustomEventPayloads(t *testing.T) {
	// The player:widget-action / player:world-input /
	// interface:close custom events ride the generic bus; the
	// caller reads Handled back after the emit (the handled
	// short-circuit lives at the call site, as in TS).
	h, _ := newTestHub()
	p := interfaceTestPlayer()

	newApi(h, "P").OnCustomEvent("player:widget-action", func(payload any) {
		payload.(*WidgetActionEvent).Handled = true
	})
	wev := &WidgetActionEvent{Player: p, Packet: "widget-action-packet"}
	h.EmitCustomEvent("player:widget-action", wev)
	if !wev.Handled {
		t.Error("widget-action handler did not set Handled")
	}

	newApi(h, "P").OnCustomEvent("player:world-input", func(payload any) {
		ev := payload.(*WorldInputEvent)
		if ev.PacketType != "npc_option" {
			t.Errorf("PacketType = %q, want npc_option", ev.PacketType)
		}
		ev.Handled = true
	})
	iev := &WorldInputEvent{Player: p, PacketType: "npc_option", Packet: "npc-packet"}
	h.EmitCustomEvent("player:world-input", iev)
	if !iev.Handled {
		t.Error("world-input handler did not set Handled")
	}

	newApi(h, "P").OnCustomEvent("interface:close", func(payload any) {
		payload.(*InterfaceCloseEvent).Handled = true
	})
	cev := &InterfaceCloseEvent{Player: p}
	h.EmitCustomEvent("interface:close", cev)
	if !cev.Handled {
		t.Error("interface:close handler did not set Handled")
	}
}
