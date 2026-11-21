package session

import "tsps-go/internal/protocol"

// SendGameMessage queues a plain game chat message: the session
// side of game.ChatMessageSender.SendGameMessage, porting
// PacketSender.sendMessage (PacketSender.ts:822-825), which
// sends encodeChatMessage("game", message) with the default
// type 0, no sender, and player id -1. The packet joins the
// pending queue and flushes with the tick like any other
// queued packet; SendPacket's mutex makes this safe to call
// from the tick goroutine and the login goroutine alike.
func (s *Session) SendGameMessage(message string) {
	s.SendPacket(protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: message, PlayerID: -1,
	}))
}

// SendTypedGameMessage queues a game chat message on the given
// chatbox channel: the session side of
// game.ChatMessageSender.SendTypedGameMessage, porting
// PacketSender.sendTypedMessage (PacketSender.ts:827-831),
// which sends encodeChatMessage("game", message, from, "",
// -1, chatType). chatType is a protocol.ChatType value; from
// is the sender name shown for channels that name one.
func (s *Session) SendTypedGameMessage(chatType int, message, from string) {
	s.SendPacket(protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: message, From: from, PlayerID: -1,
		ChatType: chatType, HasChatType: true,
	}))
}
