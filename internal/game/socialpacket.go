package game

// SocialPacket is the ONE packet struct for social traffic (contract
// C1): the TS PluginSocialPacketEvent packet union, flattened.
//
// TS grounding (commit bfd81e3):
//   - The union itself: PluginSocialPacketEvent, PluginTypes.ts:67-79.
//     Eight members: clan_chat, clan_channel_action,
//     friends_chat_action, private_message, chat_filter, report_abuse,
//     chat (friends_chat only), public_chat.
//   - The wire decodes that produce six of the members:
//     ClientProtocol.ts:844-904 (CHAT kind 4 clan_chat, CLAN_CHANNEL_ACTION,
//     FRIENDS_CHAT_ACTION, PRIVATE_MESSAGE, CHAT_FILTER, REPORT_ABUSE) and
//     the ClientMessage union, ClientProtocol.ts:304-320.
//   - The public_chat member is synthesized by the chat listener, not
//     decoded: ChatPacketListener.handleText, ChatPacketListener.ts:38-43
//     ({type: "public_chat", text, autoChat}).
//   - The chat member reaches the social event only for friends_chat
//     lines: NetworkBuilder.ts:895-904 routes CHAT with
//     messageType "friends_chat" to emitSocialPacket, while public CHAT
//     goes through handleText and game CHAT is dropped.
//
// FLATTENED, NOT A UNION. Go has no discriminated union, so one struct
// carries Kind plus every variant's fields; fields that do not belong
// to a packet's kind stay at their zero values. In particular the
// friends_chat_action member's nested action object (FriendsChatAction,
// ClientProtocol.ts:276-284: {action, name?, rank?}) is flattened into
// Action/Name/Rank: "leave" carries neither Name nor Rank, only
// "set_friend_rank" carries Rank, and the rest carry Name.
//
// NO STYLE FIELD. The CHAT decode also reads a chat style (colour and
// effect, ClientProtocol.ts:852-860), but the style never enters the
// social-event packet: the PluginTypes union has no style field, and
// handleText consumes the style itself when it broadcasts. Style stays
// on the decode/broadcast path, not here.
//
// NO BEHAVIOR. This struct is plain data: no methods, no validation,
// no clamping. Producers (the session decode, the chat listener) fill
// it; consumers (the hub's onSocialPacket emit, the friends-chat
// content) read Kind and the fields their kind defines.
//
// RELATION TO plugins.SocialPacket. The plugins package already defines
// a SocialPacket (Phase 12, hooks.go) carrying only the public_chat
// variant's fields (Kind/Text/AutoChat), because that was the only kind
// with a producer. This game type is the full contract-C1 packet; the
// game layer must never import plugins, so the two types coexist until
// the hub worker unifies them (plugins.SocialPacketEvent should carry
// this type, or convert into it). Flagged for the coordinator: do not
// add a second full copy — migrate the plugins type onto this one.

// SocialPacketKind names the packet variant carried by a SocialPacket:
// the `type` discriminant of the TS PluginSocialPacketEvent packet union
// (PluginTypes.ts:70-78). The string values are the TS discriminant
// strings, verbatim.
type SocialPacketKind string

// The eight social packet kinds, in TS union order
// (PluginTypes.ts:70-78), with public_chat last in TS but first here:
// it is the only kind the engine produces today, and the constant
// already existed under this name in the plugins package.
const (
	// SocialPacketPublicChat is a public-chat line synthesized by the
	// chat listener (ChatPacketListener.ts:38-43). Fields: Text,
	// AutoChat.
	SocialPacketPublicChat SocialPacketKind = "public_chat"
	// SocialPacketClanChat is a clan-chat line (CHAT kind 4,
	// ClientProtocol.ts:852-855). Fields: ClanType, Text. ClanType
	// is signed: -1 is the listened guest clan.
	SocialPacketClanChat SocialPacketKind = "clan_chat"
	// SocialPacketClanChannelAction is a clan-channel moderation
	// action (ClientProtocol.ts:863-867). Fields: Action ("kick",
	// "ban" or "mute"), Name, Value.
	SocialPacketClanChannelAction SocialPacketKind = "clan_channel_action"
	// SocialPacketFriendsChatAction is a friends-chat list/channel
	// action (ClientProtocol.ts:869-891). Fields: Action ("join",
	// "leave", "kick", "add_friend", "remove_friend",
	// "set_friend_rank", "add_ignore" or "remove_ignore"), Name,
	// Rank (set_friend_rank only; signed).
	SocialPacketFriendsChatAction SocialPacketKind = "friends_chat_action"
	// SocialPacketPrivateMessage is a private message
	// (ClientProtocol.ts:893-894). Fields: Recipient, Text.
	SocialPacketPrivateMessage SocialPacketKind = "private_message"
	// SocialPacketChatFilter is a chat-filter settings change
	// (ClientProtocol.ts:895-900). Fields: PublicMode, PrivateMode,
	// TradeMode. Values are raw client bytes; SetChatModes clamps
	// them (see relations.go).
	SocialPacketChatFilter SocialPacketKind = "chat_filter"
	// SocialPacketReportAbuse is an abuse report
	// (ClientProtocol.ts:901-904). Fields: Name, RuleID, Mute.
	SocialPacketReportAbuse SocialPacketKind = "report_abuse"
	// SocialPacketChat is a friends-chat line (CHAT with messageType
	// "friends_chat", NetworkBuilder.ts:898-903). Fields: Text,
	// MessageType ("friends_chat" on this kind), AutoChat (always
	// false from the decode: kind 3 autochat decodes as messageType
	// "public" and never reaches the social event as "chat").
	SocialPacketChat SocialPacketKind = "chat"
)

// SocialPacket is one social packet: Kind plus every variant's fields.
// Only the fields documented on the packet's kind carry meaning; the
// rest are zero. See the file header for the TS grounding and the
// flattening rationale.
type SocialPacket struct {
	Kind SocialPacketKind
	// Text is the chat text: public_chat (listener-normalized),
	// clan_chat, private_message, and chat (friends_chat) kinds.
	Text string
	// AutoChat reports whether the client's autochat timer sent the
	// message: public_chat only.
	AutoChat bool
	// MessageType is the CHAT message type: chat only, always
	// "friends_chat" on the social event (see the file header).
	MessageType string
	// ClanType is the clan a clan_chat line targets: clan_chat only.
	// Signed: -1 is the listened guest clan
	// (ClientProtocol.ts:853).
	ClanType int
	// Action is the requested action: clan_channel_action ("kick",
	// "ban", "mute") and friends_chat_action (see
	// SocialPacketFriendsChatAction) kinds.
	Action string
	// Name is the player or channel the action names:
	// clan_channel_action, friends_chat_action (except "leave"),
	// and report_abuse kinds.
	Name string
	// Value is the clan-channel action's value byte:
	// clan_channel_action only (ClientProtocol.ts:865).
	Value int
	// Rank is the rank to set: friends_chat_action with Action
	// "set_friend_rank" only, signed (ClientProtocol.ts:873-875).
	Rank int
	// Recipient is the private message's addressee: private_message
	// only.
	Recipient string
	// PublicMode, PrivateMode, and TradeMode are the chat-filter
	// settings: chat_filter only, raw client bytes.
	PublicMode  int
	PrivateMode int
	TradeMode   int
	// RuleID is the abuse report's internal rule id: report_abuse
	// only (ClientProtocol.ts:902).
	RuleID int
	// Mute is the abuse report's mute request: report_abuse only.
	Mute bool
}
