// Social-packet dispatch: the clan_chat, clan_channel_action,
// friends_chat_action, private_message, chat_filter, and
// report_abuse cases inline in dispatch() (NetworkBuilder.ts
// lines 905-911), plus the friends_chat arm of the "chat" case
// (lines 898-903).
//
// TS routes EVERY social packet to PluginManager.emitSocialPacket
// and takes no other action, guarded on `if (this.player)`:
// there is no core default handling for add-friend, private
// messages, chat filters, or abuse reports (Phase 19 decision
// D1). The FriendsList/Clans content ports own that behavior;
// core only decodes, guards, builds the packet, and emits. The
// public_chat variant is NOT routed here: the chat listener
// synthesizes it before broadcasting (see the emit point in
// chat.go), through the separate ChatHooks.SocialIntercept seam,
// because an intercepted public line suppresses the broadcast —
// a consequence no other social kind has.
//
// The packet built here is game.SocialPacket (contract C1): one
// flat struct, kind plus every variant's fields, with unused
// fields at zero. The CHAT kinds convert in socialPacketFromChat
// below; the five dedicated opcodes map field-for-field in
// Handle. The decoded chat STYLE never enters the packet: the
// PluginTypes union has no style field, and handleText consumes
// the style itself when it broadcasts (see game/socialpacket.go).

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// socialPacketFromChat converts a decoded CHAT message of the clan
// or friends-chat kind to its social packet. Clan messages (kind
// 4) carry the signed clan type (-1 is the listened guest clan)
// and the text; friends-chat messages (kind 2) carry the text and
// MessageType "friends_chat". AutoChat stays false: kind 3
// autochat decodes as messageType "public" and never reaches the
// social event as "chat", and the TS chat variant carries no
// autoChat field. The caller guarantees the kind is clan or
// friends_chat; any other kind yields the zero packet, which no
// caller emits.
func socialPacketFromChat(msg protocol.ChatMessage) game.SocialPacket {
	if msg.Clan {
		return game.SocialPacket{
			Kind:     game.SocialPacketClanChat,
			ClanType: msg.ClanType,
			Text:     msg.Text,
		}
	}
	if msg.MessageType == protocol.ChatMessageFriendsChat {
		return game.SocialPacket{
			Kind:        game.SocialPacketChat,
			Text:        msg.Text,
			MessageType: protocol.ChatMessageFriendsChat,
		}
	}
	return game.SocialPacket{}
}

// emitSocialPacket runs the social-packet emit for one packet and
// discards the handled result: TS core takes no action beyond the
// emit, so whether a hook claimed the packet changes nothing in
// dispatch. A nil SocialEmit means no hooks are wired (the
// no-hooks default): the packet is routed and nothing observes
// it. p is non-nil: Handle guards the nil case before emitting,
// as TS guards the dispatch cases on `if (this.player)`.
func emitSocialPacket(p *game.Player, pkt game.SocialPacket, deps Deps) {
	if deps.SocialEmit != nil {
		deps.SocialEmit(p, pkt)
	}
}
