package protocol

import "strings"

// Dialogue frame sequences: the multi-packet sends PacketSender
// performs for dialogue (PacketSender.ts, Dialogue.ts), composed from
// the primitive widget encoders in widgets.go. The primitives already
// cover every individual dialogue frame (chatbox mount pieces,
// chathead, animation, strings, client scripts, flags ranges, item
// placement, run-script); this file adds only the sequences that are
// more than one packet or carry sender-side state:
//
//   - the chatbox mount (sendChatboxInterface) and chatbox close
//     (closeSubInterface / closeTrackedInterfaces), including the
//     chatmodal-unclamp varbit, whose emission depends on the
//     sender's previous unclamped state;
//   - the chathead text block (Dialogue.sendChatText), one fixed
//     four-frame sequence shared by the NPC and player lines;
//   - the mesbox chat line (Dialogue.sendBoxChatLine), the text
//     transform plus the typed game message.
//
// The session-side DialogueSender implementation (see
// internal/game/dialogue_entries.go) calls these in the order the
// dialogue entries call its methods. Each function returns the
// frames in wire order; callers queue them in order.

// ChatboxModalTargetUID is the chatbox modal container a chatbox
// interface mounts into: group 162's child 568
// (CHATBOX_MODAL_TARGET_UID, PacketSender.ts:112). Modal chatbox
// content mounts one level deeper than the 161:96 chatbox panel,
// into 162's own CHATMODAL container — targeting the panel directly
// would replace the whole chat frame.
const ChatboxModalTargetUID = (162 << 16) | 568

// ChatboxUnclampVarbit is the varbit that unclamps the chatbox modal
// container (CHATMODAL_UNCLAMP_VARBIT, PacketSender.ts:113): 1 while
// an unclamped chatbox (the option dialogue) is mounted, 0 otherwise.
const ChatboxUnclampVarbit = 10670

// setTextAlignScriptID is cache script 600, if_settextalign
// (Dialogue.ts:26).
const setTextAlignScriptID = 600

// boxChatLineLimit is the longest chat copy of a box's text
// (Dialogue.BOX_CHAT_LINE_LIMIT, Dialogue.ts:6).
const boxChatLineLimit = 200

// EncodeChatboxInterfaceFrames renders a chatbox mount
// (PacketSender.sendChatboxInterface, PacketSender.ts:598-620): the
// unclamp varbit when state requires it, then the modal-target
// unhide and the open-sub mount (type 0) of groupID. unclamp is the
// mount's unclamp request; wasUnclamped is whether the sender's
// previous chatbox was unclamped. setChatboxUnclamped
// (PacketSender.ts:1670-1676) sends the varbit exactly when
// `unclamp || wasUnclamped`, with value 1 for an unclamped mount
// and 0 for a clamped mount following an unclamped one — so a
// clamped mount after a clamped mount sends no varbit, and the
// varbit frame, when present, precedes the mount frames. The
// group/owner bookkeeping and the interface-closed event around the
// mount are sender-side state, not frames, and live in the
// DialogueSender implementation.
func EncodeChatboxInterfaceFrames(groupID int, unclamp, wasUnclamped bool) [][]byte {
	frames := [][]byte{}
	if unclamp || wasUnclamped {
		value := 0
		if unclamp {
			value = 1
		}
		frames = append(frames, EncodeVarbit(ChatboxUnclampVarbit, value))
	}
	frames = append(frames, EncodeWidgetSetHidden(ChatboxModalTargetUID, false))
	frames = append(frames, EncodeWidgetOpenSub(ChatboxModalTargetUID, groupID, 0, WidgetOpenSubOptions{}))
	return frames
}

// EncodeChatboxCloseFrames renders a chatbox close: the close-sub
// frame for the modal target plus the modal-target hide
// (closeSubInterface, PacketSender.ts:1678-1688, plus the hide in
// closeTrackedInterfaces/closeCurrentChatbox, PacketSender.ts:931,
// 967). wasUnclamped is whether the mounted chatbox was unclamped:
// closing one sends the unclamp varbit back to 0 first
// (closeSubInterface calls setChatboxUnclamped(false), which sends
// only when the chatbox was unclamped). The group/owner clearing
// and the interface-closed event are sender-side, not frames.
func EncodeChatboxCloseFrames(wasUnclamped bool) [][]byte {
	frames := [][]byte{}
	if wasUnclamped {
		frames = append(frames, EncodeVarbit(ChatboxUnclampVarbit, 0))
	}
	frames = append(frames, EncodeWidgetCloseSub(ChatboxModalTargetUID))
	frames = append(frames, EncodeWidgetSetHidden(ChatboxModalTargetUID, true))
	return frames
}

// EncodeChatTextFrames renders a chathead box's text block
// (Dialogue.sendChatText, Dialogue.ts:24-35): the text string, the
// set-text-align client script (1, 1, line height 16, text uid),
// the "Click here to continue" string, and the pause-button flags
// range on the continue component. The text and continue strings
// carry no serverOwned argument, matching TS sendString calls
// without one.
func EncodeChatTextFrames(text string, textUID, continueUID int) [][]byte {
	return [][]byte{
		EncodeWidgetSetText(textUID, text, nil),
		EncodeRunClientScript(setTextAlignScriptID, []ScriptArg{
			ScriptArgInt(1), ScriptArgInt(1), ScriptArgInt(16), ScriptArgInt(textUID),
		}),
		EncodeWidgetSetText(continueUID, "Click here to continue", nil),
		EncodeWidgetSetFlagsRange(continueUID, -1, -1, 1),
	}
}

// EncodeBoxChatLine renders a message/item/double-item box's chat
// line (Dialogue.sendBoxChatLine, Dialogue.ts:18-22): the box's text
// as a MESBOX game message, with "<br>" replaced by spaces and the
// line cut to 200 characters. It returns nil for empty text (a blank
// box writes nothing). The send is sendTypedMessage with
// ChatType.MESBOX (PacketSender.ts:827-831): kind "game", no sender,
// player id -1. The game layer performs the same transform in
// sendBoxChatLine (it cannot import this package); the two are
// tested to agree on the text, and this function owns the bytes.
func EncodeBoxChatLine(text string) []byte {
	if text == "" {
		return nil
	}
	line := truncateUTF16Units(strings.ReplaceAll(text, "<br>", " "), boxChatLineLimit)
	return EncodeChatMessage(OutgoingChatMessage{
		Kind:        ChatKindGame,
		Text:        line,
		PlayerID:    -1,
		ChatType:    ChatTypeMesbox,
		HasChatType: true,
	})
}

// truncateUTF16Units cuts s to at most limit UTF-16 code units,
// reproducing String.prototype.slice(0, limit) for the BMP text
// dialogue carries: 1 unit per BMP rune, 2 per astral rune, cutting
// on a rune boundary. A lone astral rune straddling the limit is
// dropped whole where TS slice would split its surrogate pair — the
// only divergence, and only for astral text at exactly the limit.
// (Mirrored in internal/game/dialogue_entries.go, which cannot
// import this package.)
func truncateUTF16Units(s string, limit int) string {
	units := 0
	for i, r := range s {
		w := 1
		if r > 0xffff {
			w = 2
		}
		if units+w > limit {
			return s[:i]
		}
		units += w
	}
	return s
}
