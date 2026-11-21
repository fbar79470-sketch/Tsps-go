package game

// Game-message sending: Mobile.sendMessage (Mobile.ts:999-1006)
// and the PacketSender entry points it reaches
// (PacketSender.ts:822-831).
//
// TS defines exactly two game-message entry points on the player
// path, and this file ports both:
//
//   - sendMessage(message): Mobile.sendMessage guards the send —
//     it returns for a non-player, and for an unwatched bot (a
//     bot whose session has no network client,
//     Player.isUnwatchedBot, Player.ts:1538-1540) — then calls
//     PacketSender.sendMessage, which sends encodeChatMessage
//     ("game", message) with the default type 0.
//   - sendTypedMessage(chatType, message, from): PacketSender
//     sends encodeChatMessage("game", message, from, "", -1,
//     chatType) — a game message on one of the chatbox's
//     channels (spam, an NPC's line with its name as From, an
//     examine, ...).
//
// The other PacketSender message entry points are NOT ported
// here, each for a named reason: sendPublicChat belongs to the
// public-chat listener (it broadcasts one player's line to
// nearby players); sendRequestMessage belongs to trading (a
// trade/duel request line, kind "trade"); sendPrivateMessage
// and sendSpecialMessage belong to milestone 9 (private
// messaging, clans/channels); sendChatOptions sends filter
// settings, not a message. The protocol encoder serves all of
// them when their systems land; only the player-level methods
// are scoped here. World.sendMessage/sendStaffMessage
// (World.ts:183-200, 1159-1170) broadcast through
// player.sendMessage to every network player; no Go caller
// exists yet, so no broadcast helper is ported — the first
// caller adds a loop over SendMessage, with sendStaffMessage
// additionally needing the rights system (Player.isStaff).
//
// SEAM. The game layer must never import the protocol package,
// so — as with SendVarp/SendRunEnergy — the encoding lives in
// the session package. SessionPort itself lives in player.go,
// which this phase does not edit, so the chat sends are an
// OPTIONAL extension of it: ChatMessageSender. Player methods
// assert the attached session to that interface and drop the
// message when the session does not implement it, following
// the optional-interface precedent of ItemAnimationSource
// (appearance.go). *session.Session implements it, and the
// coordinator attaches a *session.Session as the SessionPort,
// so in production the assertion always succeeds. A future
// phase may promote these two methods onto SessionPort
// proper; the Player methods and the session methods would
// not change shape.
//
// CONCURRENCY. SendMessage/SendTypedMessage are safe to call
// from the tick goroutine (listener code) and from the login
// goroutine (hub login hooks), for the same reason SendVarp
// and SendRunEnergy are: Player.Session is assigned once at
// entry, before the player is queued and visible to the tick
// goroutine, and never reassigned, so the read races no write;
// the send itself goes through Session.SendPacket, which holds
// the session mutex and queues the packet for the next flush.
// A nil player, a nil session, or a session without the chat
// extension drops the message silently: session-less and test
// players have no client to inform, and the TS container call
// sites null-check the player before sending
// (ItemContainer.ts:539).
//
// BOT GUARD. The TS !isPlayer branch is structural here: the
// methods exist only on *Player, and NPCs have no SendMessage.
// The unwatched-bot branch is expressed through session
// presence: a bot with no attached session drops the message
// (no sender exists), while a bot WITH a session — an
// administrator-driven, watched bot (::botcontrol) — receives
// it, exactly as TS tells a watched bot what a player is told.
// This deliberately differs from the destination flag's
// all-bots exclusion (destination.go), which has no
// has-network-client distinction to key off; here session
// presence IS that distinction.

// ChatMessageSender is the optional SessionPort extension for
// game chat messages: the session side of Player.SendMessage /
// Player.SendTypedMessage. The session package implements it;
// see the file comment for why it is a separate interface.
type ChatMessageSender interface {
	// SendGameMessage queues a plain game message
	// (PacketSender.sendMessage, PacketSender.ts:822-825).
	SendGameMessage(message string)
	// SendTypedGameMessage queues a game message on the given
	// chat channel (PacketSender.sendTypedMessage,
	// PacketSender.ts:827-831). chatType is a protocol
	// ChatType value; from is the sender name shown for
	// channels that name one ("" for most).
	SendTypedGameMessage(chatType int, message, from string)
}

// SendMessage sends a game chat message to this player's
// client: Mobile.sendMessage (Mobile.ts:999-1006) through
// PacketSender.sendMessage (PacketSender.ts:822-825). The
// message goes to this player only; it is not broadcast. A
// nil player, a nil session, or a session without the chat
// extension drops the message. See the file comment for the
// bot guard and concurrency.
func (p *Player) SendMessage(text string) {
	if p == nil {
		return
	}
	if sender, ok := p.Session.(ChatMessageSender); ok {
		sender.SendGameMessage(text)
	}
}

// SendTypedMessage sends a game chat message on the given
// chatbox channel: PacketSender.sendTypedMessage
// (PacketSender.ts:827-831). chatType is a protocol ChatType
// value (e.g. ChatTypeSpam for a filterable line,
// ChatTypeNPCExamine for an NPC's examine text); from is the
// sender name shown for channels that name one (an NPC's name
// for ChatTypeNPCSay, "" otherwise). The Mobile.sendMessage
// guards do not apply in TS — sendTypedMessage is called
// directly on the PacketSender — but the nil/session guards
// do: a nil player, a nil session, or a session without the
// chat extension drops the message.
func (p *Player) SendTypedMessage(chatType int, message, from string) {
	if p == nil {
		return
	}
	if sender, ok := p.Session.(ChatMessageSender); ok {
		sender.SendTypedGameMessage(chatType, message, from)
	}
}
