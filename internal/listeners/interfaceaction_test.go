// Tests for the interface-action dispatch port (interfaceaction.go):
// the opcode handle gates, the hub emit seam, the widget-action
// fallthrough, widget-target / local-trigger / widget /
// interface-close routing, and the WORLD_INTERACTIONS preamble.
package listeners

import (
	"reflect"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// recordingInterfaceHooks is an InterfaceHooks double recording
// every emit it receives, with scripted answers.
type recordingInterfaceHooks struct {
	refuse     map[int]bool
	emitResult bool
	emits      []*InterfaceActionClickEvent
	widget     []WidgetActionEvent
	worldInput []WorldInputEvent
	closeEvts  []InterfaceCloseEvent
	// closeResult is the handled answer for EmitInterfaceClose;
	// widgetResult and worldInputResult likewise.
	widgetResult, worldInputResult, closeResult bool
	buttonClicks                                []ButtonClickEvent
	buttonResult                                bool
}

func (h *recordingInterfaceHooks) RefusesWhileBusy(buttonID int) bool {
	return h.refuse[buttonID]
}

func (h *recordingInterfaceHooks) EmitInterfaceActionClick(ev *InterfaceActionClickEvent) bool {
	h.emits = append(h.emits, ev)
	return h.emitResult
}

func (h *recordingInterfaceHooks) EmitWidgetAction(ev *WidgetActionEvent) bool {
	h.widget = append(h.widget, *ev)
	return h.widgetResult
}

func (h *recordingInterfaceHooks) EmitWorldInput(ev *WorldInputEvent) bool {
	h.worldInput = append(h.worldInput, *ev)
	return h.worldInputResult
}

func (h *recordingInterfaceHooks) EmitInterfaceClose(ev *InterfaceCloseEvent) bool {
	h.closeEvts = append(h.closeEvts, *ev)
	return h.closeResult
}

func (h *recordingInterfaceHooks) EmitButtonClick(ev *ButtonClickEvent) bool {
	h.buttonClicks = append(h.buttonClicks, *ev)
	return h.buttonResult
}

func (h *recordingInterfaceHooks) hooks() InterfaceHooks {
	return InterfaceHooks{
		RefusesWhileBusy:         h.RefusesWhileBusy,
		EmitInterfaceActionClick: h.EmitInterfaceActionClick,
		EmitWidgetAction:         h.EmitWidgetAction,
		EmitWorldInput:           h.EmitWorldInput,
		EmitInterfaceClose:       h.EmitInterfaceClose,
		EmitButtonClick:          h.EmitButtonClick,
	}
}

// recordingInterfaceSends is an InterfaceSends double recording
// every send it receives.
type recordingInterfaceSends struct {
	aborted          int
	closed           []int
	interrupted      int
	hasInterruptible bool
}

func (s *recordingInterfaceSends) sends() InterfaceSends {
	return InterfaceSends{
		AbortInputPrompt:             func(*game.Player) { s.aborted++ },
		CloseInterface:               func(_ *game.Player, groupID int) { s.closed = append(s.closed, groupID) },
		CloseInterruptibleInterfaces: func(*game.Player) { s.interrupted++ },
		HasInterruptibleInterface:    func(*game.Player) bool { return s.hasInterruptible },
	}
}

// interfaceRuntimeSession is a game.SessionPort double that
// also implements game.InterfaceRuntimeSender, so the
// Player-form CloseInterruptibleInterfaces is observable: the
// player form only calls the sender when something is open
// (interface id, dialogue, or the sender's own state), so
// tests stage hasInterruptible or an interface id.
type interfaceRuntimeSession struct {
	recordingSession
	closes           int
	hasInterruptible bool
	hasPrompt        bool
}

func (s *interfaceRuntimeSession) CloseInterruptibleInterfaces() { s.closes++ }
func (s *interfaceRuntimeSession) HasInterruptibleInterface() bool {
	return s.hasInterruptible
}
func (s *interfaceRuntimeSession) HasInputPrompt() bool { return s.hasPrompt }

var _ game.InterfaceRuntimeSender = (*interfaceRuntimeSession)(nil)

// deadPlayer kills p by draining hitpoints to 0.
func deadPlayer(t *testing.T) *game.Player {
	t.Helper()
	p := testPlayer()
	p.Skills().SetCurrentLevel(game.SkillHitpoints, 0, false)
	if hitpoints(p) != 0 {
		t.Fatalf("hitpoints = %d, want 0", hitpoints(p))
	}
	return p
}

func TestHandleInterfaceActionClickGates(t *testing.T) {
	button := int((149 << 16) | 3) // inventory group, child 3

	t.Run("nil event and nil player return false", func(t *testing.T) {
		hooks := &recordingInterfaceHooks{emitResult: true}
		if handleInterfaceActionClick(nil, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("nil event claimed")
		}
		if handleInterfaceActionClick(&InterfaceActionClickEvent{ButtonID: button}, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("nil player claimed")
		}
		if len(hooks.emits) != 0 {
			t.Fatalf("emits = %d, want 0", len(hooks.emits))
		}
	})

	t.Run("dead player returns false without emitting", func(t *testing.T) {
		hooks := &recordingInterfaceHooks{emitResult: true}
		ev := &InterfaceActionClickEvent{Player: deadPlayer(t), ButtonID: button, Action: 1}
		if handleInterfaceActionClick(ev, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("dead player claimed")
		}
		if len(hooks.emits) != 0 {
			t.Fatalf("emits = %d, want 0", len(hooks.emits))
		}
	})

	t.Run("refusing button while locked sends busy message and claims", func(t *testing.T) {
		p, sess := messageTestPlayer()
		p.SetAttribute(agilityObstacleAttribute, true)
		hooks := &recordingInterfaceHooks{refuse: map[int]bool{button: true}, emitResult: true}
		ev := &InterfaceActionClickEvent{Player: p, ButtonID: button, Action: 1}
		if !handleInterfaceActionClick(ev, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("busy refusal did not claim")
		}
		if len(hooks.emits) != 0 {
			t.Fatalf("emits = %d, want 0 (refusal precedes the emit)", len(hooks.emits))
		}
		if !reflect.DeepEqual(sess.messages, []string{busyMessage}) {
			t.Fatalf("messages = %v, want [%q]", sess.messages, busyMessage)
		}
	})

	t.Run("refusing button while unlocked proceeds to the emit", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{refuse: map[int]bool{button: true}, emitResult: true}
		ev := &InterfaceActionClickEvent{Player: p, ButtonID: button, Action: 1}
		if !handleInterfaceActionClick(ev, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("unlocked click did not reach the emit")
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
	})

	t.Run("locked without refusal proceeds to the emit", func(t *testing.T) {
		p := testPlayer()
		p.SetAttribute(agilityObstacleAttribute, true)
		hooks := &recordingInterfaceHooks{emitResult: false}
		ev := &InterfaceActionClickEvent{Player: p, ButtonID: button, Action: 1}
		if handleInterfaceActionClick(ev, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("unclaimed emit returned true")
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
	})

	t.Run("unwired hooks never claim", func(t *testing.T) {
		p := testPlayer()
		ev := &InterfaceActionClickEvent{Player: p, ButtonID: button, Action: 1}
		if handleInterfaceActionClick(ev, Deps{}) {
			t.Fatal("unwired emit claimed")
		}
	})
}

func TestEmitInterfaceActionClickUngated(t *testing.T) {
	// The dialogue_continue pre-branch calls the emit
	// directly: a dead player can still answer a prompt, so
	// no gate runs here.
	hooks := &recordingInterfaceHooks{emitResult: true}
	ev := &InterfaceActionClickEvent{Player: deadPlayer(t), ButtonID: 1, Action: 1}
	if !emitInterfaceActionClick(ev, Deps{InterfaceHooks: hooks.hooks()}) {
		t.Fatal("ungated emit did not claim")
	}
	// The nil guard still applies.
	if emitInterfaceActionClick(nil, Deps{InterfaceHooks: hooks.hooks()}) {
		t.Fatal("nil event claimed")
	}
	// A pre-handled event counts as claimed (the dual-claim
	// contract: the event's Handled flag claims it). The
	// seam still forwards it; the hub's entry guard, not
	// the seam, is what keeps hooks from re-firing.
	if !emitInterfaceActionClick(&InterfaceActionClickEvent{Player: testPlayer(), Handled: true}, Deps{InterfaceHooks: hooks.hooks()}) {
		t.Fatal("pre-handled event unclaimed")
	}
	if len(hooks.emits) != 2 {
		t.Fatalf("emits = %d, want 2", len(hooks.emits))
	}
}

// widgetActionRoute is the routeWidgetAction harness: the
// dialogue tail runs against a fake dialogue seam.
func widgetActionRoute(p *game.Player, msg protocol.WidgetAction, deps Deps) (Outcome, *fakeDialogueAccess) {
	access := &fakeDialogueAccess{active: true, current: struct{}{}}
	if deps.Dialogue == nil {
		deps.Dialogue = access
	}
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
	return routeWidgetAction(p, msg, deps, drop, acted, noAction), access
}

func TestRouteWidgetActionFallthrough(t *testing.T) {
	msg := protocol.WidgetAction{
		WidgetID: int32(500<<16 | 7), GroupID: 500, ChildID: 7,
		HasButtonNum: true, ButtonNum: 3, Slot: 4, ItemID: 995,
		HasOpID: true, OpID: 2, HasOption: true, Option: "Wield", SubOpID: 9,
	}

	t.Run("unhandled fallthrough builds the event and runs the tail", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		got, access := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.ButtonID != int(msg.WidgetID) || ev.Action != 3 || ev.GroupID != 500 || ev.ChildID != 7 {
			t.Fatalf("event ids = %d/%d/%d/%d, want widget/3/500/7", ev.ButtonID, ev.Action, ev.GroupID, ev.ChildID)
		}
		if !ev.HasOpID || ev.OpID != 2 || !ev.HasItemID || ev.ItemID != 995 || !ev.HasSlot || ev.Slot != 4 ||
			!ev.HasOption || ev.Option != "Wield" || !ev.HasSubOpID || ev.SubOpID != 9 {
			t.Fatalf("event metadata = %+v, want all fields present", ev)
		}
		if len(access.optionCalls) != 1 {
			t.Fatalf("tail calls = %d, want 1", len(access.optionCalls))
		}
	})

	t.Run("absent fields stay absent", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		bare := protocol.WidgetAction{WidgetID: int32(500<<16 | 7), GroupID: 500, ChildID: 7, Slot: -1, ItemID: -1}
		widgetActionRoute(p, bare, Deps{InterfaceHooks: hooks.hooks()})
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.HasOpID || ev.HasItemID || ev.HasSlot || ev.HasOption || ev.HasSubOpID {
			t.Fatalf("event metadata = %+v, want all absent", ev)
		}
		if ev.Action != 0 {
			t.Fatalf("action = %d, want 0 (no button number)", ev.Action)
		}
	})

	t.Run("handled fallthrough skips the tail", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{emitResult: true}
		got, access := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(access.optionCalls) != 0 {
			t.Fatalf("tail calls = %d, want 0", len(access.optionCalls))
		}
	})

	t.Run("widget-action event handled skips everything", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{widgetResult: true, emitResult: true}
		got, access := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(hooks.emits) != 0 || len(access.optionCalls) != 0 {
			t.Fatal("handled widget-action event did not skip the case")
		}
		if len(hooks.widget) != 1 || hooks.widget[0].Packet != any(msg) {
			t.Fatalf("widget events = %+v, want the decoded message", hooks.widget)
		}
	})
}

func TestRouteWidgetTarget(t *testing.T) {
	msg := protocol.WidgetTarget{
		SourceWidgetID: int32(149<<16 | 0), SourceSlot: 3,
		TargetWidgetID: int32(500<<16 | 2),
	}
	route := func(p *game.Player, m protocol.WidgetTarget, deps Deps) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		return routeWidgetTarget(p, m, 0, deps, drop, acted)
	}

	t.Run("nil player drops", func(t *testing.T) {
		if got := route(nil, msg, Deps{}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
	})

	t.Run("inventory-group target closes interruptible interfaces first", func(t *testing.T) {
		p := testPlayer()
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		p.Session = sess
		hooks := &recordingInterfaceHooks{}
		onInventory := msg
		onInventory.TargetWidgetID = int32(149<<16 | 2)
		got := route(p, onInventory, Deps{InterfaceHooks: hooks.hooks()})
		if got.Reason != ReasonWidgetTarget {
			t.Fatalf("route = %+v, want ReasonWidgetTarget", got)
		}
		if sess.closes != 1 {
			t.Fatalf("interruptible closes = %d, want 1", sess.closes)
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.ButtonID != int(onInventory.TargetWidgetID) || ev.Action != 3 {
			t.Fatalf("button/action = %d/%d, want target widget/3", ev.ButtonID, ev.Action)
		}
		if ev.TargetWidgetID != onInventory.TargetWidgetID || ev.TargetSlot != 0 || ev.SourceWidgetID != msg.SourceWidgetID || ev.SourceSlot != 3 {
			t.Fatalf("target/source = %+v", ev)
		}
	})
	t.Run("non-inventory target metadata", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		got := route(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if got.Reason != ReasonWidgetTarget {
			t.Fatalf("route = %+v, want ReasonWidgetTarget", got)
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.ButtonID != int(msg.TargetWidgetID) || ev.Action != 3 {
			t.Fatalf("button/action = %d/%d, want target widget/3", ev.ButtonID, ev.Action)
		}
		if ev.TargetWidgetID != msg.TargetWidgetID || ev.TargetSlot != 0 || ev.SourceWidgetID != msg.SourceWidgetID || ev.SourceSlot != 3 {
			t.Fatalf("target/source = %+v", ev)
		}
	})

	t.Run("non-inventory target does not close", func(t *testing.T) {
		p := testPlayer()
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		p.Session = sess
		got := route(p, msg, Deps{})
		if got.Reason != ReasonWidgetTarget {
			t.Fatalf("route = %+v, want ReasonWidgetTarget", got)
		}
		if sess.closes != 0 {
			t.Fatalf("interruptible closes = %d, want 0", sess.closes)
		}
	})

	t.Run("handled fallback acts", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{emitResult: true}
		if got := route(p, msg, Deps{InterfaceHooks: hooks.hooks()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
	})
}

func TestRouteLocalTrigger(t *testing.T) {
	route := func(p *game.Player, m protocol.LocalTrigger, deps Deps) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeLocalTrigger(p, m, deps, drop, acted, noAction)
	}

	t.Run("op trigger maps param, child, slot, item", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		msg := protocol.LocalTrigger{WidgetID: int32(219<<16 | 1), OpcodeParam: 5, ChildIndex: 3, ItemID: 7, ArgsData: []byte{9}}
		got := route(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.ButtonID != int(msg.WidgetID) || ev.Action != 5 || ev.ChildID != 3 || !ev.HasSlot || ev.Slot != 3 ||
			!ev.HasItemID || ev.ItemID != 7 || ev.ScriptTrigger || !reflect.DeepEqual(ev.ArgsData, []byte{9}) {
			t.Fatalf("event = %+v", ev)
		}
	})

	t.Run("script trigger maps child from the widget low bits", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		msg := protocol.LocalTrigger{WidgetID: int32(219<<16 | 4), OpcodeParam: 0, ChildIndex: -1, ItemID: -1}
		route(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		ev := hooks.emits[0]
		if ev.Action != 0 || ev.ChildID != 4 || ev.HasSlot || ev.HasItemID || !ev.ScriptTrigger {
			t.Fatalf("event = %+v", ev)
		}
	})

	t.Run("out-of-range param is a script trigger", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		route(p, protocol.LocalTrigger{WidgetID: int32(219<<16 | 4), OpcodeParam: 11}, Deps{InterfaceHooks: hooks.hooks()})
		if !hooks.emits[0].ScriptTrigger {
			t.Fatal("param 11 did not map to a script trigger")
		}
	})

	t.Run("handled trigger acts, nil player drops", func(t *testing.T) {
		hooks := &recordingInterfaceHooks{emitResult: true}
		msg := protocol.LocalTrigger{WidgetID: int32(219<<16 | 1), OpcodeParam: 1}
		if got := route(testPlayer(), msg, Deps{InterfaceHooks: hooks.hooks()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if got := route(nil, msg, Deps{}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
	})
}

func TestWorldInputPreamble(t *testing.T) {
	msg := protocol.GroundItemAction{X: 3090, Y: 3524}

	t.Run("nil player passes without closing; dead players still close", func(t *testing.T) {
		if worldInputPreamble(nil, "ground_item_action", msg, Deps{}) {
			t.Fatal("nil player skipped")
		}
		// The preamble has no hitpoints gate (TS runs it
		// before the listener gates): a dead player passes
		// without skipping, and the close still runs.
		dead := deadPlayer(t)
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		dead.Session = sess
		if worldInputPreamble(dead, "ground_item_action", msg, Deps{}) {
			t.Fatal("dead player skipped")
		}
		if sess.closes != 1 {
			t.Fatalf("closes = %d, want 1", sess.closes)
		}
	})

	t.Run("trading player skips without closing or emitting", func(t *testing.T) {
		p := testPlayer()
		p.Trading().SetState(game.TradeStateTradeScreen)
		if p.Trading().Status() != game.PlayerStatusTrading {
			t.Fatalf("status = %v, want TRADING", p.Trading().Status())
		}
		hooks := &recordingInterfaceHooks{}
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		p.Session = sess
		if !worldInputPreamble(p, "ground_item_action", msg, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("trading player did not skip")
		}
		if len(hooks.worldInput) != 0 || sess.closes != 0 {
			t.Fatal("trading skip emitted or closed")
		}
	})

	t.Run("handled world-input skips without closing", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{worldInputResult: true}
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		p.Session = sess
		if !worldInputPreamble(p, "npc_option", msg, Deps{InterfaceHooks: hooks.hooks()}) {
			t.Fatal("handled world-input did not skip")
		}
		if sess.closes != 0 {
			t.Fatalf("closes = %d, want 0", sess.closes)
		}
		if len(hooks.worldInput) != 1 || hooks.worldInput[0].PacketType != "npc_option" {
			t.Fatalf("world-input events = %+v", hooks.worldInput)
		}
	})

	t.Run("unhandled world-input closes interruptible interfaces", func(t *testing.T) {
		p := testPlayer()
		sess := &interfaceRuntimeSession{hasInterruptible: true}
		p.Session = sess
		if worldInputPreamble(p, "object_option", msg, Deps{}) {
			t.Fatal("unhandled world-input skipped")
		}
		if sess.closes != 1 {
			t.Fatalf("closes = %d, want 1", sess.closes)
		}
	})

	t.Run("preserve-on-equip exception", func(t *testing.T) {
		p := testPlayer()
		p.SetInterfaceId(149)
		sess := &interfaceRuntimeSession{}
		p.Session = sess
		p.SetAttribute(preserveInterfaceOnEquipAttribute, 149)
		if worldInputPreamble(p, "inventory_action", msg, Deps{}) {
			t.Fatal("preserved equip skipped")
		}
		if sess.closes != 0 {
			t.Fatalf("closes = %d, want 0", sess.closes)
		}
		// Float64 is what a save round-trip produces.
		p.SetAttribute(preserveInterfaceOnEquipAttribute, float64(149))
		if worldInputPreamble(p, "inventory_action", msg, Deps{}) {
			t.Fatal("preserved equip (float64) skipped")
		}
		if sess.closes != 0 {
			t.Fatalf("closes = %d, want 0", sess.closes)
		}
		// Mismatched id, or a non-inventory packet, closes.
		p.SetAttribute(preserveInterfaceOnEquipAttribute, 150)
		worldInputPreamble(p, "inventory_action", msg, Deps{})
		if sess.closes != 1 {
			t.Fatalf("closes = %d, want 1", sess.closes)
		}
		p.SetAttribute(preserveInterfaceOnEquipAttribute, 149)
		worldInputPreamble(p, "npc_option", msg, Deps{})
		if sess.closes != 2 {
			t.Fatalf("closes = %d, want 2", sess.closes)
		}
	})

	t.Run("unwired seams degrade to pass", func(t *testing.T) {
		if worldInputPreamble(testPlayer(), "ground_item_action", msg, Deps{}) {
			t.Fatal("unwired preamble skipped")
		}
	})
}

func TestRouteWidgetClose(t *testing.T) {
	route := func(p *game.Player, m protocol.Widget, deps Deps) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeWidgetClose(p, m, deps, drop, acted, noAction)
	}

	t.Run("close action closes through the seam", func(t *testing.T) {
		sends := &recordingInterfaceSends{}
		got := route(testPlayer(), protocol.Widget{Action: protocol.WidgetClosed, GroupID: 300}, Deps{InterfaceSends: sends.sends()})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if !reflect.DeepEqual(sends.closed, []int{300}) {
			t.Fatalf("closed = %v, want [300]", sends.closed)
		}
	})

	t.Run("open action is routed and ignored", func(t *testing.T) {
		sends := &recordingInterfaceSends{}
		got := route(testPlayer(), protocol.Widget{Action: protocol.WidgetOpened, GroupID: 300}, Deps{InterfaceSends: sends.sends()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(sends.closed) != 0 {
			t.Fatalf("closed = %v, want none", sends.closed)
		}
	})

	t.Run("nil player drops, unwired seam degrades", func(t *testing.T) {
		if got := route(nil, protocol.Widget{Action: protocol.WidgetClosed}, Deps{}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
		if got := route(testPlayer(), protocol.Widget{Action: protocol.WidgetClosed, GroupID: 300}, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
	})
}

func TestRouteInterfaceClose(t *testing.T) {
	route := func(p *game.Player, deps Deps) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeInterfaceClose(p, deps, drop, acted, noAction)
	}

	t.Run("nil player drops", func(t *testing.T) {
		if got := route(nil, Deps{}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
	})

	t.Run("handled close event aborts the prompt and skips the rest", func(t *testing.T) {
		p := testPlayer()
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(int) {}})
		hooks := &recordingInterfaceHooks{closeResult: true}
		sends := &recordingInterfaceSends{}
		if got := route(p, Deps{InterfaceHooks: hooks.hooks(), InterfaceSends: sends.sends()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if sends.aborted != 1 || sends.interrupted != 0 || len(sends.closed) != 0 {
			t.Fatalf("aborted=%d interrupted=%d closed=%v", sends.aborted, sends.interrupted, sends.closed)
		}
	})

	t.Run("overlay branch clears the attribute, aborts, closes the overlay", func(t *testing.T) {
		p := testPlayer()
		p.SetAttribute(closeOnInterfaceCloseAttribute, 320)
		hooks := &recordingInterfaceHooks{}
		sends := &recordingInterfaceSends{}
		if got := route(p, Deps{InterfaceHooks: hooks.hooks(), InterfaceSends: sends.sends()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if p.HasAttribute(closeOnInterfaceCloseAttribute) {
			t.Fatal("overlay attribute survived")
		}
		if sends.aborted != 1 || !reflect.DeepEqual(sends.closed, []int{320}) || sends.interrupted != 0 {
			t.Fatalf("aborted=%d closed=%v interrupted=%d", sends.aborted, sends.closed, sends.interrupted)
		}
	})

	t.Run("non-integer overlay attribute is ignored", func(t *testing.T) {
		p := testPlayer()
		p.SetAttribute(closeOnInterfaceCloseAttribute, "320")
		sends := &recordingInterfaceSends{}
		if got := route(p, Deps{InterfaceSends: sends.sends()}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(sends.closed) != 0 {
			t.Fatalf("closed = %v, want none", sends.closed)
		}
	})

	t.Run("something open closes through the sender seam", func(t *testing.T) {
		p := testPlayer()
		p.SetInterfaceId(149)
		sends := &recordingInterfaceSends{}
		if got := route(p, Deps{InterfaceSends: sends.sends()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if sends.interrupted != 1 {
			t.Fatalf("interrupted = %d, want 1", sends.interrupted)
		}
	})

	t.Run("armed prompt counts as something open", func(t *testing.T) {
		p := testPlayer()
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(string) {}})
		sends := &recordingInterfaceSends{}
		if got := route(p, Deps{InterfaceSends: sends.sends()}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if sends.interrupted != 1 {
			t.Fatalf("interrupted = %d, want 1", sends.interrupted)
		}
	})

	t.Run("nothing open is a no-action", func(t *testing.T) {
		sends := &recordingInterfaceSends{}
		if got := route(testPlayer(), Deps{InterfaceSends: sends.sends()}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if sends.interrupted != 0 || len(sends.closed) != 0 || sends.aborted != 0 {
			t.Fatalf("aborted=%d interrupted=%d closed=%v", sends.aborted, sends.interrupted, sends.closed)
		}
	})

	t.Run("unwired seams fall back to the player form", func(t *testing.T) {
		// The player form is session-less here, so it is a
		// no-op, but the route still counts as acted when
		// something is open.
		p := testPlayer()
		p.SetInterfaceId(149)
		if got := route(p, Deps{}); !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if got := route(testPlayer(), Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
	})
}

// creationMenuSession is a game.SessionPort +
// game.CreationMenuSender test double recording CloseCreationMenu
// calls, so the creation-menu resume branch can prove it closes
// the menu through the session.
type creationMenuSession struct {
	recordingSession
	closeCalls int
}

var _ game.SessionPort = (*creationMenuSession)(nil)
var _ game.CreationMenuSender = (*creationMenuSession)(nil)

func (s *creationMenuSession) SendCreationMenu(menu *game.CreationMenu, itemName func(id int) string, lastItem int, hasLastItem bool) {
}

func (s *creationMenuSession) CloseCreationMenu() { s.closeCalls++ }

func TestRouteDialogueResumeCreationMenu(t *testing.T) {
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }

	newMenuPlayer := func() (*game.Player, *[]menuExecuted) {
		executed := &[]menuExecuted{}
		p := testPlayer()
		menu := game.NewCreationMenu("What would you like to make?", []int{52, 53},
			func(itemID, amount int) { *executed = append(*executed, menuExecuted{itemID, amount}) },
			game.CreationMenuOptions{})
		p.SetCreationMenu(menu)
		return p, executed
	}
	route := func(p *game.Player, widgetID int32, childIndex int, deps Deps) Outcome {
		return routeDialogueResume(p, protocol.DialogueContinue{WidgetID: widgetID, ChildIndex: childIndex}, deps, drop, acted, noAction)
	}
	menuWidget := func(child int) int32 { return int32((270 << 16) | child) }

	t.Run("first item with quantity", func(t *testing.T) {
		p, executed := newMenuPlayer()
		sess := &creationMenuSession{}
		p.Session = sess
		hooks := &recordingInterfaceHooks{}
		access := &fakeDialogueAccess{resumeResult: true}
		got := route(p, menuWidget(15), 5, Deps{InterfaceHooks: hooks.hooks(), Dialogue: access})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{52, 5}) {
			t.Fatalf("executed = %v, want [{52 5}]", *executed)
		}
		if got, ok := p.CreationMenuLastItem(); !ok || got != 0 {
			t.Fatalf("last item = %d %v, want 0 true", got, ok)
		}
		if p.GetCreationMenu() != nil {
			t.Fatal("menu still open after the click")
		}
		if sess.closeCalls != 1 {
			t.Fatalf("close calls = %d, want 1", sess.closeCalls)
		}
		// The pre-branch runs BEFORE the emit and the resume.
		if len(hooks.emits) != 0 || len(access.resumeCalls) != 0 {
			t.Fatalf("emits = %d, resume calls = %d, want 0 0", len(hooks.emits), len(access.resumeCalls))
		}
	})

	t.Run("second item defaults to one", func(t *testing.T) {
		p, executed := newMenuPlayer()
		got := route(p, menuWidget(16), 0, Deps{Dialogue: &fakeDialogueAccess{resumeResult: true}})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{53, 1}) {
			t.Fatalf("executed = %v, want [{53 1}]", *executed)
		}
		if got, ok := p.CreationMenuLastItem(); !ok || got != 1 {
			t.Fatalf("last item = %d %v, want 1 true", got, ok)
		}
	})

	t.Run("quantity clamps to the menu maximum", func(t *testing.T) {
		p, executed := newMenuPlayer()
		got := route(p, menuWidget(15), 100, Deps{})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{52, 28}) {
			t.Fatalf("executed = %v, want [{52 28}]", *executed)
		}
	})

	t.Run("quantity clamps to the raw maxAmount option", func(t *testing.T) {
		executed := &[]menuExecuted{}
		p := testPlayer()
		menu := game.NewCreationMenu("t", []int{52},
			func(itemID, amount int) { *executed = append(*executed, menuExecuted{itemID, amount}) },
			game.CreationMenuOptions{MaxAmount: creationMenuMaxAmountPtr(50)})
		p.SetCreationMenu(menu)
		got := route(p, menuWidget(15), 100, Deps{})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{52, 50}) {
			t.Fatalf("executed = %v, want [{52 50}]", *executed)
		}
	})

	t.Run("negative quantity defaults to one", func(t *testing.T) {
		p, executed := newMenuPlayer()
		got := route(p, menuWidget(15), -1, Deps{})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{52, 1}) {
			t.Fatalf("executed = %v, want [{52 1}]", *executed)
		}
	})

	t.Run("session-less player still executes", func(t *testing.T) {
		p, executed := newMenuPlayer()
		// No session attached: the close frames are skipped, but
		// the attribute, the clear, and the execute still run.
		got := route(p, menuWidget(15), 1, Deps{})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(*executed) != 1 || (*executed)[0] != (menuExecuted{52, 1}) {
			t.Fatalf("executed = %v, want [{52 1}]", *executed)
		}
		if p.GetCreationMenu() != nil {
			t.Fatal("menu still open after the click")
		}
	})

	for _, tc := range []struct {
		name   string
		widget int32
		child  int
	}{
		{"child below the first item", menuWidget(14), 1},
		{"wrong group", int32((219 << 16) | 15), 1},
		{"child past the items", menuWidget(17), 1},
	} {
		t.Run("falls through: "+tc.name, func(t *testing.T) {
			p, executed := newMenuPlayer()
			hooks := &recordingInterfaceHooks{}
			access := &fakeDialogueAccess{resumeResult: true}
			got := route(p, tc.widget, tc.child, Deps{InterfaceHooks: hooks.hooks(), Dialogue: access})
			if !got.Implemented {
				t.Fatalf("route = %+v, want acted", got)
			}
			if len(*executed) != 0 {
				t.Fatalf("executed = %v, want none", *executed)
			}
			if p.GetCreationMenu() == nil {
				t.Fatal("menu cleared by a fall-through click")
			}
			if len(hooks.emits) != 1 || len(access.resumeCalls) != 1 {
				t.Fatalf("emits = %d, resume calls = %d, want 1 1", len(hooks.emits), len(access.resumeCalls))
			}
		})
	}

	t.Run("no menu open keeps the old order", func(t *testing.T) {
		// Regression: with no creation menu, a group-270 resume
		// behaves exactly as before — emit first, then resume.
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		access := &fakeDialogueAccess{resumeResult: true}
		got := route(p, menuWidget(15), 5, Deps{InterfaceHooks: hooks.hooks(), Dialogue: access})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(hooks.emits) != 1 || len(access.resumeCalls) != 1 {
			t.Fatalf("emits = %d, resume calls = %d, want 1 1", len(hooks.emits), len(access.resumeCalls))
		}
	})
}

func creationMenuMaxAmountPtr(v int) *int { return &v }

// menuExecuted is one creation-menu action invocation.
type menuExecuted struct {
	itemID, amount int
}

func TestRouteDialogueResumeEmitFirst(t *testing.T) {
	msg := protocol.DialogueContinue{WidgetID: int32(219<<16 | 1), ChildIndex: 2}

	t.Run("claiming emit skips the dialogue resume", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{emitResult: true}
		access := &fakeDialogueAccess{resumeResult: true}
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		got := routeDialogueResume(p, msg, Deps{InterfaceHooks: hooks.hooks(), Dialogue: access}, drop, acted, noAction)
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(access.resumeCalls) != 0 {
			t.Fatalf("resume calls = %d, want 0", len(access.resumeCalls))
		}
		if len(hooks.emits) != 1 {
			t.Fatalf("emits = %d, want 1", len(hooks.emits))
		}
		ev := hooks.emits[0]
		if ev.ButtonID != int(msg.WidgetID) || ev.Action != 2 || ev.GroupID != 219 || ev.ChildID != 1 || ev.HasSlot {
			t.Fatalf("event = %+v", ev)
		}
	})

	t.Run("unclaimed emit falls through to the dialogue resume", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		access := &fakeDialogueAccess{resumeResult: true}
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		got := routeDialogueResume(p, msg, Deps{InterfaceHooks: hooks.hooks(), Dialogue: access}, drop, acted, noAction)
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(access.resumeCalls) != 1 {
			t.Fatalf("resume calls = %d, want 1", len(access.resumeCalls))
		}
	})
}

// dragPayload encodes a widget-drag payload in the decoder's
// read order: target item (short LE), target widget (int LE),
// source item (short BE), source slot (shortAdd LE),
// source widget (int ME), target slot (short LE).
func dragPayload(targetItem int, targetWidget int32, sourceItem, sourceSlot int, sourceWidget int32, targetSlot int) []byte {
	p := make([]byte, 0, 16)
	p = append(p, byte(targetItem), byte(targetItem>>8))
	p = append(p, byte(targetWidget), byte(targetWidget>>8), byte(targetWidget>>16), byte(targetWidget>>24))
	p = append(p, byte(sourceItem>>8), byte(sourceItem))
	p = append(p, byte(sourceSlot>>8), byte(sourceSlot+128))
	p = append(p, byte(sourceWidget>>8), byte(sourceWidget), byte(sourceWidget>>24), byte(sourceWidget>>16))
	p = append(p, byte(targetSlot), byte(targetSlot>>8))
	return p
}

func TestWidgetDragWidgetActionEmit(t *testing.T) {
	// The widget_drag case emits player:widget-action before
	// the move: a handled event skips the move.
	p := testPlayer()
	setInv(p, 0, itemBronzeSword, 1)
	hooks := &recordingInterfaceHooks{widgetResult: true}
	payload := dragPayload(995, int32(149<<16|0), 995, 0, int32(149<<16|0), 1)
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButtond, Payload: payload}, 1, Deps{InterfaceHooks: hooks.hooks()})
	if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("route = %+v, want no-action", got)
	}
	if p.Inventory().ItemAt(1).ID() != -1 || p.Inventory().ItemAt(0).ID() != itemBronzeSword {
		t.Fatal("handled widget-drag event did not skip the move")
	}
	if len(hooks.widget) != 1 {
		t.Fatalf("widget events = %d, want 1", len(hooks.widget))
	}
}
