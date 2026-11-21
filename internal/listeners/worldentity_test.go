package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// World-entity interaction decode + dispatch: ClientProtocol.ts
// decode (lines 726-752) and NetworkBuilder.ts world_entity_option
// (lines 747-762) and set_heading (lines 741-743). The index
// fixture is 3000 (0x0BB8), the first boat entity index.

func worldEntityPayload(opcode byte) []byte {
	switch opcode {
	case protocol.NativeOpWorldEntity1: // shortLE + byteSub
		return []byte{0xB8, 0x0B, 0x00}
	case protocol.NativeOpWorldEntity2: // shortAdd + byteAdd
		return []byte{0x38, 0x0B, 0x00}
	case protocol.NativeOpWorldEntity3: // short + byteNeg
		return []byte{0x0B, 0xB8, 0x00}
	case protocol.NativeOpWorldEntity4: // byteSub + short
		return []byte{0x00, 0x0B, 0xB8}
	case protocol.NativeOpWorldEntity5: // byteAdd + shortLE
		return []byte{0x00, 0xB8, 0x0B}
	case protocol.NativeOpWorldEntity6: // shortAddLE
		return []byte{0x0B, 0x38}
	}
	return nil
}

func TestDecodeWorldEntityOption(t *testing.T) {
	for opcode := protocol.NativeOpWorldEntity1; opcode <= protocol.NativeOpWorldEntity6; opcode++ {
		msg, err := decodeWorldEntityOption(byte(opcode), worldEntityPayload(byte(opcode)))
		if err != nil {
			t.Fatalf("opcode %d: %v", opcode, err)
		}
		wantClick := int(opcode - protocol.NativeOpWorldEntity1 + 1)
		if msg.EntityIndex != 3000 || msg.ClickType != wantClick {
			t.Errorf("opcode %d: %+v, want index 3000 click %d", opcode, msg, wantClick)
		}
	}
}

func TestDecodeWorldEntityOptionMalformed(t *testing.T) {
	if _, err := decodeWorldEntityOption(protocol.NativeOpWorldEntity1, []byte{0xB8}); err == nil {
		t.Error("a truncated payload must fail")
	}
	if _, err := decodeWorldEntityOption(protocol.NativeOpWorldEntity6, []byte{0x0B, 0x38, 0x00}); err == nil {
		t.Error("trailing bytes must fail")
	}
	if _, err := decodeWorldEntityOption(protocol.NativeSetHeading, worldEntityPayload(protocol.NativeOpWorldEntity1)); err == nil {
		t.Error("an unknown opcode must fail")
	}
}

func TestDecodeSetHeading(t *testing.T) {
	heading, err := decodeSetHeading([]byte{0x1F})
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if heading != 15 {
		t.Errorf("heading = %d, want 15 (masked to 4 bits)", heading)
	}
	if _, err := decodeSetHeading([]byte{}); err == nil {
		t.Error("an empty payload must fail")
	}
	if _, err := decodeSetHeading([]byte{0x01, 0x02}); err == nil {
		t.Error("trailing bytes must fail")
	}
}

func worldEntityPlayer() *game.Player {
	p := game.NewPlayer("sailor", game.Location{X: 3000, Y: 3000, Z: 0})
	p.SetRegistered(true)
	return p
}

func TestHandleWorldEntityOption(t *testing.T) {
	p := worldEntityPlayer()
	var emitted *WorldEntityInteractionEvent
	deps := Deps{
		WorldEntityInteractable: func(player *game.Player, index int) bool { return index == 3000 },
		WorldEntityEmit: func(ev *WorldEntityInteractionEvent) bool {
			emitted = ev
			return true
		},
	}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity3, Payload: worldEntityPayload(protocol.NativeOpWorldEntity3)}, 7, deps)
	if !got.Recognized || !got.Implemented {
		t.Fatalf("outcome = %+v, want implemented", got)
	}
	if emitted == nil || emitted.Player != p || emitted.EntityIndex != 3000 || emitted.ClickType != 3 {
		t.Errorf("emitted = %+v", emitted)
	}
}

func TestHandleWorldEntityOptionGateRefusal(t *testing.T) {
	p := worldEntityPlayer()
	called := false
	deps := Deps{
		WorldEntityInteractable: func(*game.Player, int) bool { return false },
		WorldEntityEmit:         func(*WorldEntityInteractionEvent) bool { called = true; return true },
	}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity1, Payload: worldEntityPayload(protocol.NativeOpWorldEntity1)}, 7, deps)
	if !got.Recognized || got.Implemented || got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want a silent noAction", got)
	}
	if called {
		t.Error("a gate refusal must not emit")
	}
}

func TestHandleWorldEntityOptionNoEmit(t *testing.T) {
	// No emit seam installed (no hub wiring): the click is
	// still recognized and acted on — the gate passed.
	p := worldEntityPlayer()
	deps := Deps{WorldEntityInteractable: func(*game.Player, int) bool { return true }}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity2, Payload: worldEntityPayload(protocol.NativeOpWorldEntity2)}, 7, deps)
	if !got.Recognized || !got.Implemented {
		t.Fatalf("outcome = %+v, want implemented", got)
	}
}

func TestHandleWorldEntityOptionTradingSkip(t *testing.T) {
	p := worldEntityPlayer()
	p.Trading().SetState(game.TradeStateTradeScreen)
	called := false
	deps := Deps{
		WorldEntityInteractable: func(*game.Player, int) bool { called = true; return true },
		WorldEntityEmit:         func(*WorldEntityInteractionEvent) bool { called = true; return true },
	}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity1, Payload: worldEntityPayload(protocol.NativeOpWorldEntity1)}, 7, deps)
	if !got.Recognized || got.Implemented || got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want a noAction", got)
	}
	if called {
		t.Error("a trading player must drop the packet before the gate")
	}
}

func TestHandleWorldEntityOptionPreambleEmit(t *testing.T) {
	// The world-input preamble fires before the gate.
	p := worldEntityPlayer()
	var input *WorldInputEvent
	deps := Deps{
		WorldEntityInteractable: func(*game.Player, int) bool { return true },
		InterfaceHooks: InterfaceHooks{EmitWorldInput: func(ev *WorldInputEvent) bool {
			input = ev
			return false
		}},
	}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity4, Payload: worldEntityPayload(protocol.NativeOpWorldEntity4)}, 7, deps)
	if !got.Implemented {
		t.Fatalf("outcome = %+v", got)
	}
	if input == nil || input.PacketType != "world_entity_option" {
		t.Errorf("world input = %+v, want packet type world_entity_option", input)
	}
}

func TestHandleWorldEntityOptionMalformed(t *testing.T) {
	p := worldEntityPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity1, Payload: []byte{0xB8}}, 7, Deps{})
	if !got.Recognized || got.Reason != ReasonMalformedPayload {
		t.Fatalf("outcome = %+v, want malformed", got)
	}
}

func TestHandleWorldEntityOptionNoPlayer(t *testing.T) {
	got := Handle(nil, game.TurnPacket{Opcode: protocol.NativeOpWorldEntity1, Payload: worldEntityPayload(protocol.NativeOpWorldEntity1)}, 7, Deps{})
	if !got.Recognized || got.Reason != ReasonNoPlayer {
		t.Fatalf("outcome = %+v, want no-player", got)
	}
}

func TestHandleSetHeading(t *testing.T) {
	p := worldEntityPlayer()
	var gotHeading int
	var gotPlayer *game.Player
	deps := Deps{SetHelmHeading: func(player *game.Player, heading int) {
		gotPlayer = player
		gotHeading = heading
	}}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeSetHeading, Payload: []byte{0x1F}}, 7, deps)
	if !got.Recognized || !got.Implemented {
		t.Fatalf("outcome = %+v, want implemented", got)
	}
	if gotPlayer != p || gotHeading != 15 {
		t.Errorf("helm heading = player %v heading %d", gotPlayer, gotHeading)
	}
}

func TestHandleSetHeadingNoSeam(t *testing.T) {
	p := worldEntityPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeSetHeading, Payload: []byte{0x03}}, 7, Deps{})
	if !got.Recognized || got.Implemented || got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want noAction", got)
	}
}

func TestHandleSetHeadingMalformed(t *testing.T) {
	p := worldEntityPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeSetHeading, Payload: []byte{}}, 7, Deps{})
	if !got.Recognized || got.Reason != ReasonMalformedPayload {
		t.Fatalf("outcome = %+v, want malformed", got)
	}
}
