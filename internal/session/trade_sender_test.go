package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// The trade sender delegates to the protocol encoders; these tests
// pin the delegation (stage mapping, request-line shape, script
// args) through a headless session, as the dialogue sender tests
// do. Opcode/payload byte vectors live with the encoders
// (internal/protocol/trade_test.go).

func TestTradeSenderFrames(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendTradeRequest(7, "Zezima")
	self := game.TradeParty{PlayerID: 3, Name: "Alice", Offers: []game.TradePartyOffer{{Slot: 0, ItemID: 995, Quantity: 100}}}
	other := game.TradeParty{PlayerID: 7, Name: "Zezima"}
	s.SendTradeOpen("3:7", game.TradeStageOffer, self, other)
	s.SendTradeUpdate("3:7", game.TradeStageConfirm,
		game.TradeParty{PlayerID: 3, Name: "Alice", Accepted: true}, other)
	s.SendTradeClose("Trade declined.")
	s.SendTradeRequestMessage("Alice", 3)
	s.SendSubInterface((161<<16)|16, 335, 0)
	s.SendInterfaceScriptArgs(3617, []int{336 << 16})

	packets := queuedPackets(t, s)
	want := [][]byte{
		protocol.EncodeTradeRequest(7, "Zezima"),
		protocol.EncodeTradeOpen("3:7", protocol.TradeStageOffer,
			protocol.TradePartyView{PlayerID: 3, Name: "Alice", Offers: []protocol.TradeOfferView{{Slot: 0, ItemID: 995, Quantity: 100}}},
			protocol.TradePartyView{PlayerID: 7, Name: "Zezima"}, ""),
		protocol.EncodeTradeUpdate("3:7", protocol.TradeStageConfirm,
			protocol.TradePartyView{PlayerID: 3, Name: "Alice", Accepted: true},
			protocol.TradePartyView{PlayerID: 7, Name: "Zezima"}, ""),
		protocol.EncodeTradeClose("Trade declined."),
		protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
			Kind: protocol.ChatKindTrade, Text: "wishes to trade with you.",
			From: "Alice", Prefix: "", PlayerID: 3, ChatType: 101, HasChatType: true,
		}),
		protocol.EncodeWidgetOpenSub((161<<16)|16, 335, 0, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetRunScript(3617, []protocol.ScriptArg{protocol.ScriptArgInt(336 << 16)}, nil, nil, nil),
	}
	if len(packets) != len(want) {
		t.Fatalf("queued %d packets, want %d", len(packets), len(want))
	}
	for i := range want {
		if !bytes.Equal(packets[i], want[i]) {
			t.Errorf("packet %d = %x, want %x", i, packets[i], want[i])
		}
	}
}

func TestConvertTradeStage(t *testing.T) {
	if got := convertTradeStage(game.TradeStageOffer); got != protocol.TradeStageOffer {
		t.Fatalf("offer stage = %v", got)
	}
	if got := convertTradeStage(game.TradeStageConfirm); got != protocol.TradeStageConfirm {
		t.Fatalf("confirm stage = %v", got)
	}
	// Anything else reads as offer (encodeTrade's stage check).
	if got := convertTradeStage(game.TradeStage(99)); got != protocol.TradeStageOffer {
		t.Fatalf("unknown stage = %v, want offer", got)
	}
}
