package session

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Relations senders: the session side of game.RelationsSender
// (relations.go), porting the PacketSender social sends.
//
// TWO FAMILIES, as in internal/protocol/social.go:
//
//   - The legacy PacketBuilder sends (sendFriendStatus,
//     sendFriend, sendDeleteFriend, sendAddIgnore,
//     sendDeleteIgnore) are deliberate NO-OPS here, exactly as
//     in TS: those methods hand a PacketBuilder to
//     session.write, which is a no-op at bfd81e3
//     (PlayerSession.write, PlayerSession.ts:228-230:
//     "Unported callers remain no-ops until they use a native
//     packet encoder"). The methods exist so the RelationsSender
//     assertion succeeds and relations calls degrade through
//     this documented no-op rather than through a missing
//     interface; they queue nothing. The legacy wire layouts
//     live on as pure encoders in the protocol package, which
//     must never be sent (see protocol/social.go). Live presence
//     travels on the friends-chat snapshot (opcode 121), sent by
//     content, not through these methods.
//
//   - The native sends (sendChatOptions, sendPrivateMessage,
//     sendSpecialMessage) go through sendClientPacket in TS and
//     queue real frames here, through the protocol encoders.

// Session must satisfy the game's relations seam structurally, as
// it satisfies game.SessionPort and game.ChatMessageSender (see
// sessionport_test.go and chat_sender_test.go): if either side
// drifts, the build fails here rather than in the coordinator.
var _ game.RelationsSender = (*Session)(nil)

// SendFriendStatus is the session side of
// RelationsSender.SendFriendStatus, porting
// PacketSender.sendFriendStatus (PacketSender.ts:778-783). It
// queues nothing: TS drops the PacketBuilder in the no-op
// write() (see the file header). The status argument is
// accepted and ignored, as TS accepts and drops the built
// packet; relations always passes 2.
func (s *Session) SendFriendStatus(status int) {
	// Deliberate no-op: PlayerSession.write is a no-op in TS.
}

// SendFriend is the session side of RelationsSender.SendFriend,
// porting PacketSender.sendFriend (PacketSender.ts:785-792). It
// queues nothing: TS drops the PacketBuilder in the no-op
// write() (see the file header). The world+9 rule lives in the
// protocol encoder (EncodeFriend), which has no sender.
func (s *Session) SendFriend(name int64, world int) {
	// Deliberate no-op: PlayerSession.write is a no-op in TS.
}

// SendDeleteFriend is the session side of
// RelationsSender.SendDeleteFriend, porting
// PacketSender.sendDeleteFriend (PacketSender.ts:794-799). It
// queues nothing: TS drops the PacketBuilder in the no-op
// write() (see the file header).
func (s *Session) SendDeleteFriend(name int64) {
	// Deliberate no-op: PlayerSession.write is a no-op in TS.
}

// SendAddIgnore is the session side of
// RelationsSender.SendAddIgnore, porting
// PacketSender.sendAddIgnore (PacketSender.ts:801-806). It
// queues nothing: TS drops the PacketBuilder in the no-op
// write() (see the file header).
func (s *Session) SendAddIgnore(name int64) {
	// Deliberate no-op: PlayerSession.write is a no-op in TS.
}

// SendDeleteIgnore is the session side of
// RelationsSender.SendDeleteIgnore, porting
// PacketSender.sendDeleteIgnore (PacketSender.ts:808-813). It
// queues nothing: TS drops the PacketBuilder in the no-op
// write() (see the file header).
func (s *Session) SendDeleteIgnore(name int64) {
	// Deliberate no-op: PlayerSession.write is a no-op in TS.
}

// SendChatOptions queues the chat-filter settings packet: the
// session side of RelationsSender.SendChatOptions, porting
// PacketSender.sendChatOptions (PacketSender.ts:365-371), which
// sends encodeChatFilterSettings through sendClientPacket — the
// one live relations send. The privateChat byte is the mapped
// value relations computes (ON 0, FRIENDS_ONLY 1, OFF 2), not
// the raw status.
func (s *Session) SendChatOptions(publicChat, privateChat, tradeChat int) {
	s.SendPacket(protocol.EncodeChatFilterSettings(publicChat, privateChat, tradeChat))
}

// SendPrivateMessage queues an incoming private message for this
// session's player from sender: the session side of
// RelationsSender.SendPrivateMessage, porting
// PacketSender.sendPrivateMessage (PacketSender.ts:850-856),
// which sends encodeChatMessage("private_in", text, from, "",
// index). The text is the packed payload unpacked with
// unpackPackedText, lowercased, then ucFirst-cased, as TS does
// (Misc.textUnpack + toLowerCase + Misc.ucFirst). From is the
// sender's display name and the index the sender's world index;
// a nil sender sends "" and index 0, as TS's `?.`/`??`
// fallbacks do. The packet joins the pending queue and flushes
// with the tick, like any other queued packet.
func (s *Session) SendPrivateMessage(sender *game.Player, message []byte, size int) {
	text := ucFirstPackedText(strings.ToLower(unpackPackedText(message, size)))
	from := ""
	index := 0
	if sender != nil {
		from = sender.GetDisplayName()
		index = sender.Index
	}
	s.SendPacket(protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindPrivateIn, Text: text, From: from, PlayerID: index,
	}))
}

// SendSpecialMessage queues a clan or channel message: the
// session side of PacketSender.sendSpecialMessage
// (PacketSender.ts:322-326), which sends encodeChatMessage(type
// === 16 ? "clan" : "channel", message, name) with the default
// player id -1. messageType 16 selects the clan kind (default
// type 7); any other value selects the channel kind (default
// type 9). No TS caller exists at bfd81e3 (the method is
// defined but never called) and no game interface needs it; it
// exists for content, next to the other chat sends.
func (s *Session) SendSpecialMessage(name string, messageType int, message string) {
	kind := protocol.ChatKindChannel
	if messageType == 16 {
		kind = protocol.ChatKindClan
	}
	s.SendPacket(protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: kind, Text: message, From: name, PlayerID: -1,
	}))
}

// packedTextTable is Misc.xlateTable (Misc.ts:157-212): the 61
// characters of the packed-chat alphabet, in table order. The
// first 13 entries encode in one nibble; the rest encode in two
// nibbles (table index + 195, split across a nibble pair).
var packedTextTable = []rune{
	' ', 'e', 't', 'a', 'o', 'i', 'h', 'n', 's', 'r', 'd', 'l', 'u',
	'm', 'w', 'c', 'y', 'f', 'g', 'p', 'b', 'v', 'k', 'x', 'j', 'q', 'z',
	'0', '1', '2', '3', '4', '5', '6', '7', '8', '9',
	' ', '!', '?', '.', ',', ':', ';', '(', ')', '-', '&', '*', '\\', '\'', '@', '#', '+', '=', '£', '$', '%', '"', '[', ']',
}

// unpackPackedText decodes packed chat text: the INTENDED
// algorithm of Misc.textUnpack (Misc.ts:343-362) — nibble pairs
// resolved through xlateTable, single-nibble values below 13,
// two-nibble values 13..15 opening an escape whose index is
// (high << 4) + low - 195.
//
// DEVIATION, DOCUMENTED. The TS implementation is broken: it
// writes parseInt(xlateTable[val]) — parseInt of a single
// letter — into the decode buffer, which is NaN for every
// non-digit character, and Uint8Array converts NaN to 0, so
// textUnpack returns a string of NUL bytes for any real text
// (verified by running the TS function in node: packing "hello"
// yields [97, 187, 64] and unpacking yields six NUL bytes).
// Porting the bug would make every private message arrive as
// NULs. The port below implements the algorithm textPack
// (Misc.ts:417-455) inverts — the evident intent, given the
// function's name, its sibling, and sendPrivateMessage's
// lowercase/ucFirst treatment of the result — and the
// coordinator's smoke test exercises PM delivery through it.
// No live TS caller exercises textUnpack (PlayerRelations
// .message has zero callers at bfd81e3), so no production TS
// behavior is diverged from.
//
// Faithful quirks, preserved: exactly size*2 nibbles are
// decoded, so a size larger than the payload reads missing
// bytes as 0 (spaces, as TS reads undefined >> shifts as 0);
// a size of 0 or less decodes nothing; a dangling high nibble
// at the end of the input is dropped (TS drops it the same
// way); and textPack's odd-length padding surfaces as a
// trailing space, which sendPrivateMessage does not trim.
func unpackPackedText(packed []byte, size int) string {
	var out []rune
	high := -1
	for i := 0; i < size*2; i++ {
		var b byte
		if i/2 < len(packed) {
			b = packed[i/2]
		}
		val := int((b >> (4 - 4*(i%2))) & 0xf)
		if high == -1 {
			if val < 13 {
				out = append(out, packedTextTable[val])
			} else {
				high = val
			}
			continue
		}
		out = append(out, packedTextTable[(high<<4)+val-195])
		high = -1
	}
	return string(out)
}

// ucFirstPackedText is Misc.ucFirst (Misc.ts:291-299): the input
// is lowercased (sendPrivateMessage lowercases before calling
// too, so this double-lowercase is faithful), then the first
// character is uppercased; a string of length 0 or 1 is
// uppercased whole. TS operates on UTF-16 code units; the
// unpacked alphabet is ASCII plus '£' (which uppercases to
// itself), so rune handling is identical here.
func ucFirstPackedText(s string) string {
	s = strings.ToLower(s)
	runes := []rune(s)
	if len(runes) <= 1 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(string(runes[0])) + string(runes[1:])
}
