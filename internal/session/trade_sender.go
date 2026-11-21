package session

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// TradeSender implementation: the session side of game.TradeSender,
// porting the PacketSender surface Trading itself needs
// (PacketSender.ts). Each method mirrors one PacketSender call,
// cited at the method; the wire encoders live in internal/protocol
// (trade.go for the trade frames, widgets.go for the interface
// primitives, chat.go for the request line). Strings, flags
// ranges, and interface removal ride DialogueSender; inventory
// container sends ride Inventory.RefreshItems; game/typed
// messages ride SendGameMessage/SendTypedGameMessage — see
// game.TradeSender.
//
// A non-nil *Session is assumed: the game layer reaches these
// methods only through the SessionPort assertion in
// Player.tradeSender, which yields nil for a session-less
// player, so no call ever lands on a nil receiver. A zero-value
// Session drops every send through SendPacket's closed-channel
// check, as the other sender files do.

var _ game.TradeSender = (*Session)(nil)

// SendTradeRequest sends a trade-request notice (encodeTradeRequest,
// ClientProtocol.ts:2131-2135): the requester's index and display
// name. The client shows "<name> wishes to trade with you." and
// makes it clickable; the companion chat line is
// SendTradeRequestMessage, a separate packet.
func (s *Session) SendTradeRequest(playerID int, name string) {
	s.SendPacket(protocol.EncodeTradeRequest(playerID, name))
}

// SendTradeOpen sends a trade-window open (encodeTradeOpen,
// ClientProtocol.ts:2144-2146) with an empty info string, as
// Trading.sendState calls it (Trading.ts:581-592).
func (s *Session) SendTradeOpen(sessionID string, stage game.TradeStage, self, other game.TradeParty) {
	s.SendPacket(protocol.EncodeTradeOpen(sessionID, convertTradeStage(stage), convertTradeParty(self), convertTradeParty(other), ""))
}

// SendTradeUpdate sends a trade-window state refresh
// (encodeTradeUpdate, ClientProtocol.ts:2148-2150) with an empty
// info string, as Trading.sendState calls it.
func (s *Session) SendTradeUpdate(sessionID string, stage game.TradeStage, self, other game.TradeParty) {
	s.SendPacket(protocol.EncodeTradeUpdate(sessionID, convertTradeStage(stage), convertTradeParty(self), convertTradeParty(other), ""))
}

// SendTradeClose sends a trade-window close (encodeTradeClose,
// ClientProtocol.ts:2152-2154) with the reason string
// (Trading.abort's message, e.g. "Trade declined.").
func (s *Session) SendTradeClose(reason string) {
	s.SendPacket(protocol.EncodeTradeClose(reason))
}

// SendTradeRequestMessage sends the clickable request line
// (encodeChatMessage, kind "trade", type 101, Trading.ts:225-228
// via sendMessage: "wishes to trade with you." from the named
// player at the given index, with an empty prefix). The explicit
// chat type 101 (TRADE_REQUEST_CHAT_TYPE) overrides the trade
// kind's default type 4: HasChatType carries it, as
// SendTypedGameMessage does for typed lines.
func (s *Session) SendTradeRequestMessage(fromName string, fromIndex int) {
	s.SendPacket(protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind:        protocol.ChatKindTrade,
		Text:        "wishes to trade with you.",
		From:        fromName,
		Prefix:      "",
		PlayerID:    fromIndex,
		ChatType:    101,
		HasChatType: true,
	}))
}

// SendSubInterface mounts a widget group into a parent component
// (PacketSender.sendSubInterface, PacketSender.ts:1544-1551;
// encodeWidgetOpenSub with no options). Trading mounts the
// offer/confirm groups and the inventory group through it. It
// delegates to the interface runtime's MountSubInterface
// (interface_sender.go), which tracks the mount, sends the
// flags-range frame for 149 mounts, and emits interface:opened.
func (s *Session) SendSubInterface(targetUID, groupID, typ int) {
	s.MountSubInterface(targetUID, groupID, typ)
}

// SendInterfaceScriptArgs runs a widget script with integer
// arguments (PacketSender.sendInterfaceScript,
// PacketSender.ts:1706-1718; encodeWidgetRunScript). Trading's
// only call is script 3617 with the inventory group uid.
// DialogueSender.SendInterfaceScript covers only the
// no-argument form.
func (s *Session) SendInterfaceScriptArgs(scriptID int, args []int) {
	scriptArgs := make([]protocol.ScriptArg, len(args))
	for i, arg := range args {
		scriptArgs[i] = protocol.ScriptArgInt(arg)
	}
	s.SendPacket(protocol.EncodeWidgetRunScript(scriptID, scriptArgs, nil, nil, nil))
}

// convertTradeStage maps the game-layer stage onto the protocol
// stage byte (0 offer, 1 confirm; anything else is confirm, as
// encodeTrade's stage check does).
func convertTradeStage(stage game.TradeStage) protocol.TradeStage {
	if stage == game.TradeStageConfirm {
		return protocol.TradeStageConfirm
	}
	return protocol.TradeStageOffer
}

// convertTradeParty maps one side of a trade window onto its
// protocol form, field for field.
func convertTradeParty(party game.TradeParty) protocol.TradePartyView {
	offers := make([]protocol.TradeOfferView, len(party.Offers))
	for i, offer := range party.Offers {
		offers[i] = protocol.TradeOfferView{Slot: offer.Slot, ItemID: offer.ItemID, Quantity: offer.Quantity}
	}
	return protocol.TradePartyView{
		PlayerID:        party.PlayerID,
		Name:            party.Name,
		Accepted:        party.Accepted,
		ConfirmAccepted: party.ConfirmAccepted,
		Offers:          offers,
	}
}
