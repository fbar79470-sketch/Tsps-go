// Tests for the entered-amount / entered-syntax prompt dispatch
// (prompts.go): the amount filters, the re-arm rule, and prompt
// abort.
package listeners

import (
	"encoding/binary"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestRouteDialogueAmount(t *testing.T) {
	route := func(p *game.Player, msg protocol.DialogueAmount) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeDialogueAmount(p, msg, drop, acted, noAction)
	}

	t.Run("nil player drops", func(t *testing.T) {
		if got := route(nil, protocol.DialogueAmount{Amount: 5}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
	})

	t.Run("no action armed is a no-action", func(t *testing.T) {
		if got := route(testPlayer(), protocol.DialogueAmount{Amount: 5}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
	})

	t.Run("positive amount executes and clears", func(t *testing.T) {
		p := testPlayer()
		var got int
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(amount int) { got = amount }})
		if out := route(p, protocol.DialogueAmount{Amount: 42}); !out.Implemented {
			t.Fatalf("route = %+v, want acted", out)
		}
		if got != 42 || p.GetEnteredAmountAction() != nil {
			t.Fatalf("executed=%d holder=%v, want 42/nil", got, p.GetEnteredAmountAction())
		}
	})

	t.Run("zero requires acceptsZero; rejected answers still clear", func(t *testing.T) {
		p := testPlayer()
		ran := false
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(int) { ran = true }})
		if out := route(p, protocol.DialogueAmount{Amount: 0}); !out.Implemented {
			t.Fatalf("route = %+v, want acted (dispatch consumes the packet)", out)
		}
		if ran || p.GetEnteredAmountAction() != nil {
			t.Fatal("zero without acceptsZero executed or survived")
		}

		p2 := testPlayer()
		ran = false
		p2.SetEnteredAmountAction(&game.EnteredAmountAction{AcceptsZero: true, Execute: func(int) { ran = true }})
		route(p2, protocol.DialogueAmount{Amount: 0})
		if !ran {
			t.Fatal("zero with acceptsZero did not execute")
		}
	})

	t.Run("negative requires acceptsNegative; rejected answers still clear", func(t *testing.T) {
		p := testPlayer()
		ran := false
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(int) { ran = true }})
		route(p, protocol.DialogueAmount{Amount: -3})
		if ran || p.GetEnteredAmountAction() != nil {
			t.Fatal("negative without acceptsNegative executed or survived")
		}

		p2 := testPlayer()
		ran = false
		p2.SetEnteredAmountAction(&game.EnteredAmountAction{AcceptsNegative: true, Execute: func(int) { ran = true }})
		route(p2, protocol.DialogueAmount{Amount: -3})
		if !ran {
			t.Fatal("negative with acceptsNegative did not execute")
		}
	})

	t.Run("re-arm: a replacement action survives, the same object clears", func(t *testing.T) {
		p := testPlayer()
		replacement := &game.EnteredAmountAction{Execute: func(int) {}}
		action := &game.EnteredAmountAction{Execute: func(int) {
			p.SetEnteredAmountAction(replacement)
		}}
		p.SetEnteredAmountAction(action)
		route(p, protocol.DialogueAmount{Amount: 1})
		if p.GetEnteredAmountAction() != replacement {
			t.Fatal("replacement action did not survive")
		}

		p2 := testPlayer()
		same := &game.EnteredAmountAction{}
		same.Execute = func(int) { p2.SetEnteredAmountAction(same) }
		p2.SetEnteredAmountAction(same)
		route(p2, protocol.DialogueAmount{Amount: 1})
		if p2.GetEnteredAmountAction() != nil {
			t.Fatal("same-object re-arm survived")
		}
	})

	t.Run("nil Execute is guarded", func(t *testing.T) {
		p := testPlayer()
		p.SetEnteredAmountAction(&game.EnteredAmountAction{})
		if out := route(p, protocol.DialogueAmount{Amount: 1}); !out.Implemented {
			t.Fatalf("route = %+v, want acted", out)
		}
		if p.GetEnteredAmountAction() != nil {
			t.Fatal("nil-Execute action survived")
		}
	})
}

func TestRouteDialogueInput(t *testing.T) {
	route := func(p *game.Player, msg protocol.DialogueInput) Outcome {
		drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
		acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
		noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
		return routeDialogueInput(p, msg, drop, acted, noAction)
	}

	t.Run("nil player drops, no action is a no-action", func(t *testing.T) {
		if got := route(nil, protocol.DialogueInput{Value: "x"}); got.Reason != ReasonNoPlayer {
			t.Fatalf("route = %+v, want ReasonNoPlayer", got)
		}
		if got := route(testPlayer(), protocol.DialogueInput{Value: "x"}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
	})

	t.Run("syntax executes unconditionally and clears", func(t *testing.T) {
		p := testPlayer()
		var got string
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(s string) { got = s }})
		if out := route(p, protocol.DialogueInput{Value: "hello"}); !out.Implemented {
			t.Fatalf("route = %+v, want acted", out)
		}
		if got != "hello" || p.GetEnteredSyntaxAction() != nil {
			t.Fatalf("executed=%q holder=%v", got, p.GetEnteredSyntaxAction())
		}
	})

	t.Run("re-arm rule matches the amount form", func(t *testing.T) {
		p := testPlayer()
		replacement := &game.EnteredSyntaxAction{Execute: func(string) {}}
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(string) {
			p.SetEnteredSyntaxAction(replacement)
		}})
		route(p, protocol.DialogueInput{Value: "x"})
		if p.GetEnteredSyntaxAction() != replacement {
			t.Fatal("replacement syntax action did not survive")
		}
	})

	t.Run("nil Execute is guarded", func(t *testing.T) {
		p := testPlayer()
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{})
		if out := route(p, protocol.DialogueInput{Value: "x"}); !out.Implemented {
			t.Fatalf("route = %+v, want acted", out)
		}
		if p.GetEnteredSyntaxAction() != nil {
			t.Fatal("nil-Execute action survived")
		}
	})
}

func TestAbortInputPrompt(t *testing.T) {
	t.Run("seam form delegates and never falls back", func(t *testing.T) {
		p := testPlayer()
		cancelled := false
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Cancel: func() { cancelled = true }})
		sends := &recordingInterfaceSends{}
		abortInputPrompt(p, Deps{InterfaceSends: sends.sends()})
		if sends.aborted != 1 {
			t.Fatalf("aborted = %d, want 1", sends.aborted)
		}
		if cancelled {
			t.Fatal("fallback cancel ran alongside the seam")
		}
		if p.GetEnteredAmountAction() == nil {
			t.Fatal("fallback cleared the holder alongside the seam")
		}
	})

	t.Run("fallback clears both holders and runs the amount cancel", func(t *testing.T) {
		p := testPlayer()
		cancelled := false
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Cancel: func() { cancelled = true }})
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(string) {}})
		abortInputPrompt(p, Deps{})
		if !cancelled {
			t.Fatal("amount Cancel did not run")
		}
		if p.GetEnteredAmountAction() != nil || p.GetEnteredSyntaxAction() != nil {
			t.Fatal("holders survived the fallback abort")
		}
	})

	t.Run("fallback with no cancel is safe, nil player is a no-op", func(t *testing.T) {
		p := testPlayer()
		p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(int) {}})
		abortInputPrompt(p, Deps{})
		if p.GetEnteredAmountAction() != nil {
			t.Fatal("holder survived")
		}
		abortInputPrompt(nil, Deps{})
	})
}

func TestHandleDialoguePackets(t *testing.T) {
	// End-to-end through Handle: opcodes 192/193/194 decode
	// and route to the prompt handlers; malformed payloads
	// drop malformed, never panic.
	p := testPlayer()
	var got int
	p.SetEnteredAmountAction(&game.EnteredAmountAction{Execute: func(amount int) { got = amount }})
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, 7)
	if out := Handle(p, game.TurnPacket{Opcode: protocol.ClientResumeCountdialog, Payload: payload}, 1, Deps{}); !out.Implemented {
		t.Fatalf("amount = %+v, want acted", out)
	}
	if got != 7 {
		t.Fatalf("executed = %d, want 7", got)
	}

	var text string
	p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(s string) { text = s }})
	for _, opcode := range []byte{protocol.ClientResumeNamedialog, protocol.ClientResumeStringdialog} {
		p.SetEnteredSyntaxAction(&game.EnteredSyntaxAction{Execute: func(s string) { text = s }})
		if out := Handle(p, game.TurnPacket{Opcode: opcode, Payload: []byte("abc\x00")}, 1, Deps{}); !out.Implemented {
			t.Fatalf("opcode %d = %+v, want acted", opcode, out)
		}
		if text != "abc" {
			t.Fatalf("opcode %d executed = %q, want abc", opcode, text)
		}
	}

	for _, tc := range []struct {
		opcode  byte
		payload []byte
	}{
		{protocol.ClientResumeCountdialog, nil},
		{protocol.ClientResumeCountdialog, []byte{0, 0}},
		{protocol.ClientResumeCountdialog, []byte{0, 0, 0, 0, 0}},
		{protocol.ClientResumeNamedialog, nil},
		{protocol.ClientResumeStringdialog, nil},
	} {
		if out := Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 1, Deps{}); out.Reason != ReasonMalformedPayload {
			t.Fatalf("opcode %d len %d = %+v, want malformed", tc.opcode, len(tc.payload), out)
		}
	}
}

func TestHandleInterfaceCloseAndWidget(t *testing.T) {
	p := testPlayer()
	p.SetInterfaceId(149)
	sends := &recordingInterfaceSends{}
	if out := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfClose, Payload: nil}, 1, Deps{InterfaceSends: sends.sends()}); !out.Implemented {
		t.Fatalf("close = %+v, want acted", out)
	}
	if sends.interrupted != 1 {
		t.Fatalf("interrupted = %d, want 1", sends.interrupted)
	}
	// A non-empty IF_CLOSE payload is malformed.
	if out := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfClose, Payload: []byte{1}}, 1, Deps{}); out.Reason != ReasonMalformedPayload {
		t.Fatalf("close with payload = %+v, want malformed", out)
	}

	// WIDGET close: action byte 1, group 300, modal 0.
	sends = &recordingInterfaceSends{}
	if out := Handle(p, game.TurnPacket{Opcode: protocol.ClientWidget, Payload: []byte{1, 1, 44, 0}}, 1, Deps{InterfaceSends: sends.sends()}); !out.Implemented {
		t.Fatalf("widget close = %+v, want acted", out)
	}
	// WIDGET open: action byte 0 — routed and ignored.
	if out := Handle(p, game.TurnPacket{Opcode: protocol.ClientWidget, Payload: []byte{0, 1, 44, 0}}, 1, Deps{InterfaceSends: sends.sends()}); out != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("widget open = %+v, want no-action", out)
	}
	// Truncated widget payloads are malformed.
	if out := Handle(p, game.TurnPacket{Opcode: protocol.ClientWidget, Payload: []byte{1, 1}}, 1, Deps{}); out.Reason != ReasonMalformedPayload {
		t.Fatalf("widget truncated = %+v, want malformed", out)
	}
}
