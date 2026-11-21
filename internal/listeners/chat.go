// Chat: ChatPacketListener.ts:1-74 (commit bfd81e3), reached from the
// "chat" case of dispatch() (NetworkBuilder.ts:894-903) for public
// messages only. Friends-chat and clan kinds route to the plugin
// social packet instead (Handle emits them through
// Deps.SocialEmit — see social.go); game-kind chat (kind 1) is
// ignored by the TS dispatch switch entirely.
//
// handleChat ports handleText in TS order:
//
//  1. Normalize: strip angle brackets, trim, take the first 80
//     characters (line 30).
//  2. "::" prefix routes to the command listener BEFORE any chat
//     gate (lines 31-34) — a muted player can still run commands.
//     The command text passed on is everything after "::",
//     untrimmed; CommandPacketListener.execute sanitizes it
//     (CommandPacketListener.ts:11-12). The registry itself is
//     Worker 3's; see CommandDispatcher.
//  3. allowChat (lines 9-27): empty text refuses silently; a muted
//     account or IP refuses with "You are muted and cannot chat.";
//     a blocked word refuses with "Your message did not make it
//     past the filter." There are no other core gates: no rate
//     limit, no hitpoints gate, no chat-mode check on the sender.
//     (Chat is a turn packet — it is in neither IMMEDIATE_PACKETS
//     nor WORLD_INTERACTIONS, NetworkBuilder.ts:263-305 — so no
//     stall, trading, or interface-close handling applies.)
//  4. The social-packet plugin event (lines 38-44): a plugin that
//     marks the event handled suppresses the broadcast. See the
//     marked emit point in handleChat and ChatHooks.SocialIntercept.
//  5. forceChat(iconPrefix + text) (line 47): the overhead text,
//     carried to nearby clients by the next sync frame (the
//     FORCED_CHAT update block; the encoder already exists in
//     internal/protocol/sync.go). It is set even when the
//     broadcast below reaches nobody.
//  6. Broadcast (lines 52-73): a CHAT_MESSAGE frame to the speaker,
//     the speaker's local players, and the nearby-players
//     supplement for asymmetric local lists at the 255-player
//     cap, deduplicated by index, skipping recipients whose
//     relations gate rejects the line (the speaker always
//     receives their own). The frame carries the PLAIN text, the
//     icon-prefixed display name as `from`, the speaker's index,
//     the autochat type override (90, or 91 with a crown prefix),
//     and the decoded style.
//
// GAPS, documented at their sites below, never invented:
//   - Chat icons are empty until content sets them through
//     Player.SetChatIcons; no core code sets icons, so crowns
//     never render from core behavior alone.
//   - The relations gate degenerates to the mode-0/empty-list
//     evaluation (see game.CanReceivePublicChatFrom): friends,
//     ignores, and chat-filter modes are milestone 9.
//   - Mute data, the social-packet intercept, command dispatch,
//     broadcast delivery, and the nearby-players supplement all
//     arrive through ChatHooks and degrade to no-ops when unwired
//     (see Deps.Chat). Refusal lines, by contrast, go through
//     Player.SendMessage directly (Worker 1's game/message.go),
//     as TS calls player.sendMessage from the listener; with no
//     chat-capable session attached the text drops silently but
//     the gate still refuses. In particular, with no Delivery
//     wired, forceChat still sets the overhead text but no
//     chatbox frame is sent to anyone.

package listeners

import (
	"strconv"
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// CommandDispatcher dispatches a "::" command: the seam between the
// chat listener and the command registry (CommandPacketListener,
// CommandPacketListener.ts:7-53). rawCommand is the normalized
// chat text after the "::" prefix, exactly as TS passes it
// (ChatPacketListener.ts:32: text.slice(2)) — untrimmed, with any
// NUL bytes intact; sanitizing, base-token parsing, gating, and
// the unhandled-command reply are the dispatcher's job, per
// CommandPacketListener.execute. The coordinator wires Worker 3's
// registry here; when unset (nil), "::" text drops with
// ReasonCommand, the pre-phase-12 behavior.
type CommandDispatcher interface {
	DispatchCommand(player *game.Player, rawCommand string)
}

// PublicChatMessage is one broadcast copy of a public-chat line:
// the arguments to PacketSender.sendPublicChat
// (PacketSender.ts:843-848). Text is the plain normalized text
// (no icon prefix); From is the icon prefix plus the speaker's
// display name; SpeakerIndex is the speaker's world index;
// ChatType is the autochat override (nil for ordinary chat, which
// encodes as the default public type 2); Style is the decoded
// colour/effect, or nil when the packet carried none.
type PublicChatMessage struct {
	Text         string
	From         string
	SpeakerIndex int
	AutoChat     bool
	ChatType     *int
	Style        *protocol.ChatStyle
}

// ChatDelivery sends one broadcast copy of a public-chat line to
// one recipient: the Go form of
// recipient.getPacketSender().sendPublicChat(...) (ChatPacketListener
// .ts:66-72). The coordinator wires Worker 1's chat encoder plus
// the recipient's session here; when unset (nil), handleChat sets
// the overhead text but sends no chatbox frame. Implementations
// must tolerate a recipient with no session (a nil session means
// no client is attached; TS PacketSender.sendMessage is likewise
// guarded through the session).
type ChatDelivery interface {
	DeliverPublicChat(recipient *game.Player, msg PublicChatMessage)
}

// ChatHooks supplies the chat listener's world seams. Every field
// is optional; a nil field degrades to the documented no-op, so a
// bare Deps{} still runs the listener faithfully as far as its own
// state reaches (normalization, gates that need no data, forceChat,
// local-list recipient computation). The coordinator wires the
// fields in handleTurnPacket; see Deps.Chat.
type ChatHooks struct {
	// Commands dispatches "::" text to the command registry.
	// Nil means ReasonCommand drops (see CommandDispatcher).
	Commands CommandDispatcher

	// Delivery sends broadcast copies to recipients. Nil means
	// no chatbox frame is sent (see ChatDelivery).
	Delivery ChatDelivery

	// NearbyPlayers returns the bucket-collected player
	// candidates around the speaker: World.NearbyPlayersForUpdate
	// (World.ts:703-705). It feeds only the asymmetric-supplement
	// term of the recipient list; nil means local-list-only
	// recipients, which is exact except at the 255-player cap
	// (see chatRecipients).
	NearbyPlayers func(speaker *game.Player) []*game.Player

	// Muted reports whether the account (username) or the host
	// address is muted: PlayerPunishment.muted(username) OR
	// PlayerPunishment.IPMuted(host) (PlayerPunishment.ts:127-130,
	// 146-150). The coordinator wires the persist.Punishment
	// lists here; nil means nobody is muted. The host argument
	// is the player's RemoteAddr (host-only, port stripped, as
	// TS remoteAddress is).
	Muted func(username, host string) bool

	// SocialIntercept runs the social-packet plugin event for a
	// public-chat line and reports whether a plugin marked it
	// handled (see the marked emit point in handleChat). The
	// coordinator wires the hub's onSocialPacket emit here; nil
	// means no plugin intercepts, the no-hooks default.
	SocialIntercept func(player *game.Player, text string, autoChat bool) bool
}

// jsTrimCutset is the set of characters JavaScript's String trim
// removes (WhiteSpace + LineTerminator), used to reproduce the
// text normalization in ChatPacketListener.handleText
// (ChatPacketListener.ts line 30).
const jsTrimCutset = " \t\n\v\f\r\u00a0\u1680\u2000\u2001\u2002\u2003\u2004\u2005\u2006\u2007\u2008\u2009\u200a\u2028\u2029\u202f\u205f\u3000\ufeff"

// normalizeChatText reproduces the handleText normalization
// (ChatPacketListener.ts line 30): String(value), strip "<" and
// ">", trim, take the first 80 characters.
//
// Deliberate deviation, at the site: JS slice(0, 80) counts UTF-16
// code units, so an astral-plane character (a surrogate pair)
// counts 2 and can be split mid-pair; Go counts runes, so 80
// runes are kept whole. The difference only shows for text with
// astral characters past position 80, and the wire encoding is
// latin1 (which truncates every rune above U+00FF to its low byte
// anyway), so no client-visible text changes.
func normalizeChatText(value string) string {
	stripped := strings.ReplaceAll(strings.ReplaceAll(value, "<", ""), ">", "")
	trimmed := strings.Trim(stripped, jsTrimCutset)
	runes := []rune(trimmed)
	if len(runes) > 80 {
		runes = runes[:80]
	}
	return string(runes)
}

// blockedWords is Misc.BLOCKED_WORDS (Misc.ts:134-144), verbatim
// and in TS order. Note the strip-then-check order in handleText:
// the "<"-bearing entries ("<img", "<img=", "<col=", "<shad=")
// can never match, because normalization already removed every
// "<" and ">" before allowChat runs. They are kept verbatim so a
// future change to either side stays faithful to the TS list.
var blockedWords = []string{
	".com",
	".net",
	".org",
	"<img",
	"@cr",
	"<img=",
	":tradereq:",
	":duelreq:",
	"<col=",
	"<shad=",
}

// blockedWord reports whether text contains any blocked word
// (Misc.blockedWord, Misc.ts:702-709: case-sensitive substring
// match against Misc.BLOCKED_WORDS; the unused local shadowing
// the list in TS is not reproduced).
func blockedWord(text string) bool {
	for _, word := range blockedWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// chatIconPrefix renders the icon prefix prepended to the
// speaker's overhead text and display name (ChatPacketListener
// .ts:44-46): one "<img=N>" per chat icon, joined with no
// separator. With no icons the prefix is "".
func chatIconPrefix(icons []int) string {
	var prefix strings.Builder
	for _, icon := range icons {
		prefix.WriteString("<img=")
		prefix.WriteString(strconv.Itoa(icon))
		prefix.WriteString(">")
	}
	return prefix.String()
}

// chatTypeFor resolves the sendPublicChat chatType override
// (ChatPacketListener.ts:70): nil for ordinary chat; AUTOTYPER
// for autochat, MODAUTOTYPER when the sender has a crown prefix
// (hasIcons: a non-empty icon prefix). The constants are
// protocol.ChatTypeAutotyper/ChatTypeModAutotyper (ChatType,
// ClientProtocol.ts lines 1103-1106); nil means the default
// public type 2 (encodeChatMessage, ClientProtocol.ts:1199-1208).
func chatTypeFor(autoChat, hasIcons bool) *int {
	if !autoChat {
		return nil
	}
	chatType := protocol.ChatTypeAutotyper
	if hasIcons {
		chatType = protocol.ChatTypeModAutotyper
	}
	return &chatType
}

// allowChat is ChatPacketListener.allowChat (ChatPacketListener
// .ts:9-27): empty text refuses silently; a muted account/IP or a
// blocked word refuses after sending the refusal line with
// Player.SendMessage (Mobile.ts:999-1005), which drops the text
// silently when the player has no chat-capable session. The mute
// check needs the Muted hook; with no hook wired, nobody reads
// as muted.
func allowChat(p *game.Player, text string, hooks ChatHooks) bool {
	if text == "" {
		return false
	}
	if hooks.Muted != nil && hooks.Muted(p.Username, p.RemoteAddr) {
		p.SendMessage("You are muted and cannot chat.")
		return false
	}
	if blockedWord(text) {
		p.SendMessage("Your message did not make it past the filter.")
		return false
	}
	return true
}

// chatRecipients builds the broadcast recipient list
// (ChatPacketListener.ts:52-58): the speaker, the speaker's local
// players, and the nearby-players supplement — candidates from
// NearbyPlayers whose own local list includes the speaker, for
// the asymmetric case at the 255-player cap ("Local lists can be
// asymmetric at the 255-player cap. Include clients that render
// the speaker even when the speaker cannot render them."). The
// list is a fresh slice: the speaker's local list is live
// tick-goroutine state that must never be mutated or retained
// (see game.Player.LocalPlayers), and the supplement filter reads
// each candidate's live list. Deduplication, nil-skipping, and
// the relations gate happen in the send loop in handleChat, as
// in TS (lines 59-65). With no NearbyPlayers hook the supplement
// term is empty (local-list-only recipients).
func chatRecipients(speaker *game.Player, hooks ChatHooks) []*game.Player {
	recipients := make([]*game.Player, 0, len(speaker.LocalPlayers())+1)
	recipients = append(recipients, speaker)
	recipients = append(recipients, speaker.LocalPlayers()...)
	if hooks.NearbyPlayers == nil {
		return recipients
	}
	for _, candidate := range hooks.NearbyPlayers(speaker) {
		if candidate == nil {
			continue
		}
		for _, local := range candidate.LocalPlayers() {
			if local == speaker {
				recipients = append(recipients, candidate)
				break
			}
		}
	}
	return recipients
}

// handleChat runs ChatPacketListener.handleText
// (ChatPacketListener.ts:29-74) for one decoded public-chat
// message. p is non-nil: Handle guards the nil case before
// routing (TS guards the dispatch case on `if (this.player)`).
// It returns whether a game action was taken (forceChat set, or a
// command dispatched) and, when not, the drop reason: ReasonCommand
// for "::" text with no dispatcher wired, ReasonNoAction for a
// refused gate, an intercepted line, or empty text. Handle maps
// the result onto Outcome.
func handleChat(p *game.Player, msg protocol.ChatMessage, hooks ChatHooks) (bool, DropReason) {
	text := normalizeChatText(msg.Text)
	if strings.HasPrefix(text, "::") {
		// Command sub-dispatch (lines 31-34), BEFORE allowChat:
		// muted players can still run commands. The text after
		// "::" is passed verbatim; CommandPacketListener
		// .execute sanitizes it.
		if hooks.Commands == nil {
			return false, ReasonCommand
		}
		hooks.Commands.DispatchCommand(p, text[2:])
		return true, ""
	}
	if !allowChat(p, text, hooks) {
		return false, ReasonNoAction
	}
	// === SOCIAL-PACKET EMIT POINT ===
	// PluginManager.emitSocialPacket({player, packet: {type:
	// "public_chat", text, autoChat}, handled: false})
	// (ChatPacketListener.ts:38-44; the event shape is
	// PluginSocialPacketEvent, PluginTypes.ts:67-80; the emit
	// runs hooks in registration order until one marks the
	// event handled and returns event.handled,
	// PluginManager.ts:836-843). A handled event suppresses the
	// broadcast AND the forceChat below: the intercept check
	// runs before either. The coordinator wires the hub emit
	// through ChatHooks.SocialIntercept; until then the
	// no-hooks default (not intercepted) applies. Do NOT add a
	// hub registration method here — Worker 3 owns the hub
	// surface; this hook is the only emit site this listener
	// needs.
	if hooks.SocialIntercept != nil && hooks.SocialIntercept(p, text, msg.AutoChat) {
		return false, ReasonNoAction
	}
	icons := p.ChatIcons()
	iconPrefix := chatIconPrefix(icons)
	// Overhead text (line 47): set before the broadcast loop
	// and unconditionally — it shows even when the broadcast
	// reaches nobody.
	p.ForceChat(iconPrefix + text)
	out := PublicChatMessage{
		Text:         text,
		From:         iconPrefix + p.GetDisplayName(),
		SpeakerIndex: p.Index,
		AutoChat:     msg.AutoChat,
		ChatType:     chatTypeFor(msg.AutoChat, len(icons) > 0),
		Style:        msg.Style,
	}
	sent := make(map[int]bool)
	for _, recipient := range chatRecipients(p, hooks) {
		if recipient == nil || sent[recipient.Index] {
			continue
		}
		// The speaker always receives their own line; every
		// other recipient must pass the relations gate with
		// the packet's autoChat flag (lines 60-65).
		if recipient != p && !recipient.CanReceivePublicChatFrom(p, msg.AutoChat) {
			continue
		}
		sent[recipient.Index] = true
		if hooks.Delivery != nil {
			hooks.Delivery.DeliverPublicChat(recipient, out)
		}
	}
	return true, ""
}
