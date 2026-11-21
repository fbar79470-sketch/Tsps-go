package game

import (
	"testing"
)

// interfaceRuntimeTestSession is a SessionPort test double that also
// implements InterfaceRuntimeSender, recording close calls and
// reporting configurable interruptible/prompt state.
type interfaceRuntimeTestSession struct {
	*fakeSession
	interruptible bool
	inputPrompt   bool
	closes        int
	opened        []int
	closed        []int
}

func newInterfaceRuntimeTestSession() *interfaceRuntimeTestSession {
	return &interfaceRuntimeTestSession{fakeSession: newFakeSession()}
}

func (s *interfaceRuntimeTestSession) HasInterruptibleInterface() bool {
	return s.interruptible
}

func (s *interfaceRuntimeTestSession) HasInputPrompt() bool { return s.inputPrompt }

func (s *interfaceRuntimeTestSession) CloseInterruptibleInterfaces() { s.closes++ }

// SendInterface/CloseInterface record the open/close calls, so
// the Player forwarding methods are observable. The embedded
// fakeSession does not implement them.
func (s *interfaceRuntimeTestSession) SendInterface(id int) { s.opened = append(s.opened, id) }
func (s *interfaceRuntimeTestSession) CloseInterface(groupID int) {
	s.closed = append(s.closed, groupID)
}

func TestPlayerSendAndCloseInterfaceForward(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	sess := newInterfaceRuntimeTestSession()
	p.Session = sess
	p.SendInterface(500)
	p.CloseInterface(500)
	if len(sess.opened) != 1 || sess.opened[0] != 500 {
		t.Fatalf("opened = %v, want [500]", sess.opened)
	}
	if len(sess.closed) != 1 || sess.closed[0] != 500 {
		t.Fatalf("closed = %v, want [500]", sess.closed)
	}
	// A session-less player drops both calls without panicking.
	p.Session = nil
	p.SendInterface(500)
	p.CloseInterface(500)
	// A nil player drops both calls without panicking.
	var nilPlayer *Player
	nilPlayer.SendInterface(500)
	nilPlayer.CloseInterface(500)
}

func TestInterfaceIdDefaultsToNegativeOne(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	if got := p.GetInterfaceId(); got != -1 {
		t.Fatalf("GetInterfaceId() = %d, want -1", got)
	}
}

func TestSetInterfaceIdEmitsChangedOnlyOnChange(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	var events []any
	p.EmitCustomEvent = func(name string, payload any) {
		events = append(events, [2]any{name, payload})
	}
	// Opening from -1: changed only, no closed.
	p.SetInterfaceId(300)
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	name, payload := events[0].([2]any)[0].(string), events[0].([2]any)[1]
	if name != InterfaceChangedEventName {
		t.Fatalf("event name = %q, want %q", name, InterfaceChangedEventName)
	}
	changed, ok := payload.(InterfaceChangedEvent)
	if !ok {
		t.Fatalf("payload type %T, want InterfaceChangedEvent", payload)
	}
	if changed.Player != p || changed.PreviousID != -1 || changed.InterfaceID != 300 {
		t.Fatalf("changed event = %+v, want player, -1 -> 300", changed)
	}
	// Same id again: nothing.
	events = nil
	p.SetInterfaceId(300)
	if len(events) != 0 {
		t.Fatalf("same-id set emitted %d events, want 0", len(events))
	}
	// Closing to -1: changed only (the closed event comes from
	// the sender's close path, not from here).
	p.SetInterfaceId(-1)
	if len(events) != 1 {
		t.Fatalf("close emitted %d events, want 1", len(events))
	}
	if events[0].([2]any)[0].(string) != InterfaceChangedEventName {
		t.Fatalf("close event = %q, want %q", events[0].([2]any)[0], InterfaceChangedEventName)
	}
	// Setting -1 again: nothing.
	events = nil
	p.SetInterfaceId(-1)
	if len(events) != 0 {
		t.Fatalf("redundant close emitted %d events, want 0", len(events))
	}
}

func TestSetInterfaceIdReplaceEmitsClosedThenChanged(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	p.SetInterfaceId(300)
	var names []string
	var closed InterfaceClosedEvent
	var changed InterfaceChangedEvent
	p.EmitCustomEvent = func(name string, payload any) {
		names = append(names, name)
		switch name {
		case InterfaceClosedEventName:
			closed = payload.(InterfaceClosedEvent)
		case InterfaceChangedEventName:
			changed = payload.(InterfaceChangedEvent)
		}
	}
	p.SetInterfaceId(301)
	if len(names) != 2 || names[0] != InterfaceClosedEventName || names[1] != InterfaceChangedEventName {
		t.Fatalf("events = %v, want [closed changed]", names)
	}
	if closed.Player != p || closed.InterfaceID != 300 {
		t.Fatalf("closed = %+v, want player + 300", closed)
	}
	if changed.PreviousID != 300 || changed.InterfaceID != 301 {
		t.Fatalf("changed = %+v, want 300 -> 301", changed)
	}
}

func TestSetInterfaceIdNilEmitIsSafe(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3200, Y: 3200})
	// No EmitCustomEvent installed: no panic, state still updates.
	p.SetInterfaceId(300)
	if got := p.GetInterfaceId(); got != 300 {
		t.Fatalf("GetInterfaceId() = %d, want 300", got)
	}
	var nilPlayer *Player
	nilPlayer.SetInterfaceId(300) // must not panic
	if got := nilPlayer.GetInterfaceId(); got != -1 {
		t.Fatalf("nil GetInterfaceId() = %d, want -1", got)
	}
}

func TestCloseInterruptibleInterfacesGate(t *testing.T) {
	newPlayerWithSender := func() (*Player, *interfaceRuntimeTestSession) {
		p := NewPlayer("alice", Location{X: 3200, Y: 3200})
		s := newInterfaceRuntimeTestSession()
		p.Session = s
		return p, s
	}

	// Nothing open: no close call.
	p, s := newPlayerWithSender()
	p.CloseInterruptibleInterfaces()
	if s.closes != 0 {
		t.Fatalf("closes = %d, want 0 (nothing open)", s.closes)
	}

	// Open interface id: close.
	p, s = newPlayerWithSender()
	p.SetInterfaceId(300)
	p.CloseInterruptibleInterfaces()
	if s.closes != 1 {
		t.Fatalf("closes = %d, want 1 (interface open)", s.closes)
	}

	// Sender interruptible state alone: close.
	p, s = newPlayerWithSender()
	s.interruptible = true
	p.CloseInterruptibleInterfaces()
	if s.closes != 1 {
		t.Fatalf("closes = %d, want 1 (sender interruptible)", s.closes)
	}

	// Sender input prompt alone: close.
	p, s = newPlayerWithSender()
	s.inputPrompt = true
	p.CloseInterruptibleInterfaces()
	if s.closes != 1 {
		t.Fatalf("closes = %d, want 1 (sender input prompt)", s.closes)
	}

	// Active dialogue alone: close.
	p, s = newPlayerWithSender()
	p.GetDialogueManager().StartDialogues(NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Hi.")))
	if !p.GetDialogueManager().IsActive() {
		t.Fatal("dialogue manager not active after StartDialogues")
	}
	p.CloseInterruptibleInterfaces()
	if s.closes != 1 {
		t.Fatalf("closes = %d, want 1 (dialogue active)", s.closes)
	}

	// Trading at the trade screen blocks the close even with an
	// open interface id.
	p, s = newPlayerWithSender()
	p.SetInterfaceId(335)
	p.Trading().SetState(TradeStateTradeScreen)
	p.CloseInterruptibleInterfaces()
	if s.closes != 0 {
		t.Fatalf("closes = %d, want 0 (TRADING gate)", s.closes)
	}

	// An outstanding trade request does NOT block: TRADING
	// starts at the trade screen.
	p, s = newPlayerWithSender()
	p.SetInterfaceId(335)
	p.Trading().SetState(TradeStateRequestedTrade)
	p.CloseInterruptibleInterfaces()
	if s.closes != 1 {
		t.Fatalf("closes = %d, want 1 (requested trade does not gate)", s.closes)
	}

	// Dueling blocks the close.
	p, s = newPlayerWithSender()
	p.SetInterfaceId(300)
	p.Dueling().SetState(DuelStateDuelScreen)
	p.CloseInterruptibleInterfaces()
	if s.closes != 0 {
		t.Fatalf("closes = %d, want 0 (DUELING gate)", s.closes)
	}

	// No sender: nothing to close, no panic.
	p = NewPlayer("alice", Location{X: 3200, Y: 3200})
	p.SetInterfaceId(300)
	p.CloseInterruptibleInterfaces()

	// A SessionPort without the extension: no close, no panic.
	p = NewPlayer("alice", Location{X: 3200, Y: 3200})
	p.Session = bareSessionPort{}
	p.SetInterfaceId(300)
	p.CloseInterruptibleInterfaces()
}
