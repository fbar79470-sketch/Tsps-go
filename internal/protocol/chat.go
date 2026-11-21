package protocol

import (
	"encoding/binary"
	"regexp"
)

// Chat message encoding: encodeChatMessage (ClientProtocol.ts
// lines 1185-1217), the ChatType channels (lines 1100-1125), the
// legacy colour rewrite (lines 1182-1184, 1207-1211), and the
// highlight resolver (lines 1127-1148). One server packet carries
// every chatbox line: CHAT_MESSAGE (opcode 120, var-short, -2 in
// the length table), whose payload is the text, a type byte, the
// sender name, a prefix, the sender's player index, and — for
// public chat only — the style bytes.
//
// The PacketSender entry points that produce these packets
// (PacketSender.ts:822-856) are sendMessage (a plain game
// message), sendTypedMessage (a game message on a ChatType
// channel, optionally from a named sender), sendRequestMessage
// (a trade/duel request line, kind "trade"), sendPublicChat
// (kind "public", with an optional style), sendPrivateMessage
// (kind "private_in", milestone 9), and sendSpecialMessage
// (kind "clan"/"channel", milestone 9). The encoder below serves
// all of them: the kind selects the default type byte, and an
// explicit chat type overrides it. Which entry points have Go
// senders is a layer above this file: the game layer exposes
// sendMessage/sendTypedMessage, public chat belongs to the chat
// listener, and the private/clan entry points are NOT STARTED.

// ChatType channels a game message can go to (the cache's chat
// types, as rsprox names them): the ChatType const
// (ClientProtocol.ts lines 1101-1125). Values are the wire type
// bytes.
const (
	// ChatTypeGame is the default game-message channel (0).
	ChatTypeGame = 0
	// ChatTypeAutotyper is autochat: a phrase the sender's
	// client repeats (90).
	ChatTypeAutotyper = 90
	// ChatTypeModAutotyper is autochat from a player with a mod
	// crown (91).
	ChatTypeModAutotyper = 91
	// ChatTypeBroadcast is a server broadcast (14).
	ChatTypeBroadcast = 14
	// ChatTypeFriendsChatNotification is shown on the
	// chat-channel tab as well (11).
	ChatTypeFriendsChatNotification = 11
	// ChatTypeSpam is filtered out with "Filter: on" (105).
	ChatTypeSpam = 105
	// ChatTypeTrade is a trade/duel status line (102).
	ChatTypeTrade = 102
	// ChatTypeNPCSay is an NPC's overhead line, shown as
	// "Name: text" with the NPC's name in From (116).
	ChatTypeNPCSay = 116
	// ChatTypeMesbox is a message box's text, logged in the
	// chatbox as live does when one opens (115).
	ChatTypeMesbox = 115
	// ChatTypeWelcome is the login greeting (108).
	ChatTypeWelcome = 108
	// ChatTypeNPCExamine is an NPC's examine (28).
	ChatTypeNPCExamine = 28
	// ChatTypeLocExamine is an object's examine (29).
	ChatTypeLocExamine = 29
)

// ChatKind is encodeChatMessage's messageType argument
// (ClientProtocol.ts line 1186): the family of chatbox line,
// which selects the default type byte when no explicit chat type
// is given. The zero value is ChatKindGame.
type ChatKind int

const (
	// ChatKindGame is a plain game message (default type 0).
	ChatKindGame ChatKind = iota
	// ChatKindPublic is another player's public chat (default
	// type 2).
	ChatKindPublic
	// ChatKindPrivateIn is an incoming private message
	// (default type 3). Milestone 9.
	ChatKindPrivateIn
	// ChatKindPrivateOut is an outgoing private message
	// (default type 6). Milestone 9.
	ChatKindPrivateOut
	// ChatKindChannel is a channel message (default type 9).
	// Milestone 9.
	ChatKindChannel
	// ChatKindClan is a clan message (default type 7).
	// Milestone 9.
	ChatKindClan
	// ChatKindTrade is a trade/duel request line (default
	// type 4).
	ChatKindTrade
	// ChatKindServer is a server message (default type 0).
	ChatKindServer
)

// defaultChatType is encodeChatMessage's defaultTypes table
// (ClientProtocol.ts lines 1197-1206). A kind outside the table
// cannot occur in TS (the argument is a string union); Go
// answers 0, the game default, rather than inventing a channel.
func (k ChatKind) defaultChatType() int {
	switch k {
	case ChatKindGame:
		return 0
	case ChatKindPublic:
		return 2
	case ChatKindPrivateIn:
		return 3
	case ChatKindPrivateOut:
		return 6
	case ChatKindChannel:
		return 9
	case ChatKindClan:
		return 7
	case ChatKindTrade:
		return 4
	case ChatKindServer:
		return 0
	default:
		return 0
	}
}

// ChatHighlight is one CHAT_HIGHLIGHTS entry
// (ClientProtocol.ts lines 1134-1139): the `<col=...>` value for
// a `@mes_hl_<colour>@` tag on each chatbox background.
type ChatHighlight struct {
	Opaque      string
	Transparent string
}

// ChatHighlights are the known highlight colours
// (CHAT_HIGHLIGHTS, ClientProtocol.ts lines 1134-1139). gre and
// red are from captures; ora is UNVERIFIED upstream (the classic
// orange, for both backgrounds); pur is Death Charge's.
var ChatHighlights = map[string]ChatHighlight{
	"gre": {Opaque: "06600c", Transparent: "229628"},
	"red": {Opaque: "ef1020", Transparent: "ef1020"},
	"ora": {Opaque: "ff9040", Transparent: "ff9040"},
	"pur": {Opaque: "6800bf", Transparent: "a53fff"},
}

var chatHighlightPattern = regexp.MustCompile(`@mes_hl_([a-z]+)@`)

// ResolveChatHighlights returns text with each known
// `@mes_hl_<colour>@` tag replaced by a `<col=...>` for the
// chatbox background (resolveChatHighlights,
// ClientProtocol.ts lines 1141-1148). An unknown colour keeps
// its tag verbatim. This is caller-side formatting: content
// resolves highlights before sending (ArceuusSpells.ts:369);
// the encoder never applies it. Only lowercase colour names
// match, as in the TS regex.
func ResolveChatHighlights(text string, transparent bool) string {
	return chatHighlightPattern.ReplaceAllStringFunc(text, func(tag string) string {
		colour := chatHighlightPattern.FindStringSubmatch(tag)[1]
		highlight, ok := ChatHighlights[colour]
		if !ok {
			return tag
		}
		if transparent {
			return "<col=" + highlight.Transparent + ">"
		}
		return "<col=" + highlight.Opaque + ">"
	})
}

// legacyChatColours maps the legacy `@xxx@` colour tags to hex
// colours (LEGACY_CHAT_COLOURS, ClientProtocol.ts lines
// 1182-1184). Existing server content uses legacy colours; both
// clients render native font markup, so the encoder rewrites
// known tags to `<col=...>` for game/server messages.
var legacyChatColours = map[string]string{
	"red": "ff0000", "gre": "00ff00", "blu": "0000ff", "yel": "ffff00",
	"cya": "00ffff", "mag": "ff00ff", "whi": "ffffff", "bla": "000000",
	"lre": "ff9040", "dre": "800000", "dbl": "000080",
	"or1": "ffb000", "or2": "ff7000", "or3": "ff3000",
	"gr1": "c0ff00", "gr2": "80ff00", "gr3": "40ff00",
}

var legacyChatColourPattern = regexp.MustCompile(`@([a-z0-9]{3})@`)

// resolveLegacyChatColours rewrites known legacy colour tags in
// text to `<col=...>` markup (the replace in encodeChatMessage,
// ClientProtocol.ts lines 1207-1211). Only exactly-three-character
// lowercase/digit tags match, and an unknown tag is kept
// verbatim. It applies to game and server messages only; the
// caller (EncodeChatMessage) enforces the kind restriction.
func resolveLegacyChatColours(text string) string {
	return legacyChatColourPattern.ReplaceAllStringFunc(text, func(tag string) string {
		colour := legacyChatColourPattern.FindStringSubmatch(tag)[1]
		hex, ok := legacyChatColours[colour]
		if !ok {
			return tag
		}
		return "<col=" + hex + ">"
	})
}

// chatStyleBytes renders a ChatStyle as wire bytes
// (chatStyleBytes, ClientProtocol.ts lines 1174-1177): colour,
// effect, then the pattern indices. A nil style renders no
// bytes. Colour and effect are masked to a byte, as TS masks
// with & 0xff; pattern entries are already bytes.
func chatStyleBytes(style *ChatStyle) []byte {
	if style == nil {
		return nil
	}
	out := []byte{byte(style.Colour & 0xff), byte(style.Effect & 0xff)}
	return append(out, style.Pattern...)
}

// OutgoingChatMessage is one encodeChatMessage call's arguments
// (ClientProtocol.ts lines 1185-1194). ChatType overrides the
// kind's default type byte only when HasChatType is set (the TS
// `chatType?: number` parameter); Style is public-chat-only in
// TS and is appended after the player index for clients that
// colour overhead text. PlayerID is the sender's player index,
// or -1 when there is none (game messages); it is masked to 16
// bits on the wire.
type OutgoingChatMessage struct {
	Kind        ChatKind
	Text        string
	From        string
	Prefix      string
	PlayerID    int
	ChatType    int
	HasChatType bool
	Style       *ChatStyle
}

// EncodeChatMessage encodes one chatbox line (encodeChatMessage,
// ClientProtocol.ts lines 1185-1217): CHAT_MESSAGE (opcode 120,
// var-short), with the payload laid out as the latin1
// NUL-terminated text, the type byte, the latin1 NUL-terminated
// sender name, the latin1 NUL-terminated prefix, the player id
// as a big-endian u16, and the style bytes. Game and server
// messages have their legacy `@xxx@` colour tags rewritten to
// `<col=...>` markup first; other kinds send the text verbatim.
// The type byte and player id are masked exactly as TS masks
// them (& 0xff / & 0xffff). It panics when the framed payload
// would exceed 65535 bytes, mirroring the TS encodeServerPacket
// throw; only a text over ~64 KiB can reach that.
func EncodeChatMessage(m OutgoingChatMessage) []byte {
	text := m.Text
	if m.Kind == ChatKindGame || m.Kind == ChatKindServer {
		text = resolveLegacyChatColours(text)
	}
	chatType := m.Kind.defaultChatType()
	if m.HasChatType {
		chatType = m.ChatType
	}
	payload := make([]byte, 0, len(text)+len(m.From)+len(m.Prefix)+6)
	payload = append(payload, latin1Bytes(text)...)
	payload = append(payload, byte(chatType&0xff))
	payload = append(payload, latin1Bytes(m.From)...)
	payload = append(payload, latin1Bytes(m.Prefix)...)
	id := make([]byte, 2)
	binary.BigEndian.PutUint16(id, uint16(m.PlayerID&0xffff))
	payload = append(payload, id...)
	payload = append(payload, chatStyleBytes(m.Style)...)
	return mustEncode(ServerChatMessage, payload)
}
