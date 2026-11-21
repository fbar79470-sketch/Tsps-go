// Dialogue entries: the Dialogue interface and every entry type
// (dialogues/entries/Dialogue.ts, entries/impl/*.ts), the
// DialogueOption / DialogueOptionAction types, and the packet-sender
// seam entries send through.
//
// TS entries call Player.getPacketSender() directly. Go's game layer
// must never import the protocol package (see doc.go), so entries send
// through DialogueSender, an OPTIONAL SessionPort extension following
// the ChatMessageSender precedent (message.go): the session package
// implements it, Player methods assert the attached session to it, and
// a session without it drops the entry's frames silently. Each method
// mirrors one PacketSender method, cited below; the wire encoders live
// in internal/protocol (widgets.go for the primitives,
// dialogue_encode.go for the multi-packet chatbox sequences).
//
// SESSION-SIDE GAP (coordinator integration): no DialogueSender
// implementation exists yet. Until the session package implements it,
// dialogue entries send nothing in production and
// DialogueManager.CanContinue always reports false. The implementation
// is specified method-by-method in the interface comment, including
// the chatbox-owner state machine PacketSender keeps
// (PacketSender.ts:228-231, 609, 941-973, 1670-1676) and the
// FrameUpdater absence (see SendString).

package game

import (
	"regexp"
	"strings"
)

// Dialogue is one entry of a dialogue chain: the abstract Dialogue
// class (entries/Dialogue.ts:4-43). TS carries the entry's index as a
// constructor-set field read by DialogueBuilder.add; Go carries it in
// the interface (GetIndex) because builders key entries by it, and
// every entry embeds dialogueBase to provide it. Send opens the
// entry's interface (or runs its action) for player.
type Dialogue interface {
	// Send opens this entry for p. A nil player, or a player whose
	// session does not implement DialogueSender, drops the entry
	// silently (see the file comment); TS would throw on a null
	// player, but Go call sites pass real players and the guard
	// only covers session-less/test players.
	Send(p *Player)
	// GetIndex returns the chain index this entry was built with
	// (Dialogue.getIndex, Dialogue.ts:39-41).
	GetIndex() int
}

// dialogueBase provides the index storage for every entry
// (Dialogue.ts:7-11, 39-41).
type dialogueBase struct {
	index int
}

// GetIndex returns the entry's chain index.
func (b dialogueBase) GetIndex() int { return b.index }

// ClientScriptArg is one sendClientScript / run-script argument: a
// number or a string (the `(number | string)[]` args of
// PacketSender.sendClientScript, PacketSender.ts:1754-1757). The
// protocol package has its own ScriptArg; the game layer cannot
// import it, so this is the game-side mirror, translated by the
// session-side DialogueSender implementation.
type ClientScriptArg struct {
	isString bool
	num      int
	str      string
}

// ClientScriptInt builds a number script argument.
func ClientScriptInt(value int) ClientScriptArg {
	return ClientScriptArg{num: value}
}

// ClientScriptString builds a string script argument.
func ClientScriptString(value string) ClientScriptArg {
	return ClientScriptArg{isString: true, str: value}
}

// IsString reports whether the argument is a string (false: a number).
func (a ClientScriptArg) IsString() bool { return a.isString }

// IntValue returns the argument's number value. It is meaningful only
// when IsString is false.
func (a ClientScriptArg) IntValue() int { return a.num }

// StringValue returns the argument's string value. It is meaningful
// only when IsString is true.
func (a ClientScriptArg) StringValue() string { return a.str }

// DialogueSender is the optional SessionPort extension through which
// dialogue entries send their frames: the PacketSender surface
// dialogue itself needs (see the file comment). Method order below
// follows the PacketSender file. The session-side implementation
// sends each call's packet(s) through Session.SendPacket in call
// order; dialogue entries rely on the order (a chatbox mount must
// precede the strings written into it).
//
// CHATBOX STATE. The implementation keeps, per player, the
// PacketSender chatbox state (PacketSender.ts:228-231):
//   - chatboxGroupId, -1 when no chatbox is mounted;
//   - chatboxOwner, the owner recorded by the last
//     SendChatboxInterface call from the presenting owner in
//     force at the time (PacketSender.ts:609);
//   - presentingChatboxOwner, the owner WithChatboxOwner scopes
//     to the presentation running inside it, restored afterwards
//     (PacketSender.ts:946-951);
//   - chatboxUnclamped, whether the mounted chatbox was mounted
//     unclamped (PacketSender.ts:1670-1676).
//
// SendChatboxInterface records group + owner and emits
// protocol.EncodeChatboxInterfaceFrames; SendInterfaceRemoval's
// minimal faithful scope is the chatbox close
// (protocol.EncodeChatboxCloseFrames) when a chatbox is mounted,
// and nothing otherwise — the tracked-sub-interface close, the
// main-interface close, the bank-search end, and the
// "interface:closed" custom event (PacketSender.ts:1011-1025,
// 913-933) belong to the interface runtime (milestone 8) and are
// NOT ported. IsChatboxInterface answers groupId == chatboxGroupId
// (PacketSender.ts:972-974).
type DialogueSender interface {
	// SendChatboxInterface mounts groupID in the chatbox modal
	// container (PacketSender.sendChatboxInterface,
	// PacketSender.ts:598-620): the unclamp varbit when state
	// requires it, then the modal-target unhide + open-sub
	// frames (protocol.EncodeChatboxInterfaceFrames), recording
	// group + presenting owner. unclamp is the TS default-false
	// parameter; only the option dialogue passes true.
	SendChatboxInterface(groupID int, unclamp bool)
	// SendNpcHeadOnInterface sets an NPC head on a component
	// (PacketSender.sendNpcHeadOnInterface,
	// PacketSender.ts:437-440; encodeWidgetSetNpcHead).
	SendNpcHeadOnInterface(npcID, uid int)
	// SendPlayerHeadOnInterface sets the player's head on a
	// component (PacketSender.sendPlayerHeadOnInterface,
	// PacketSender.ts:432-435; encodeWidgetSetPlayerHead).
	SendPlayerHeadOnInterface(uid int)
	// SendInterfaceAnimation sets a component's animation
	// (PacketSender.sendInterfaceAnimation,
	// PacketSender.ts:495-498; encodeWidgetSetAnimation).
	SendInterfaceAnimation(uid, animationID int)
	// SendString writes text into a component
	// (PacketSender.sendString, PacketSender.ts:677-684;
	// encodeWidgetSetText with no serverOwned argument).
	// DEVIATION: TS dedups through the per-player FrameUpdater
	// (FrameUpdater.ts: shouldUpdate suppresses a write identical
	// to the cached text, and sendChatboxInterface clears the
	// mounted group's cache). Go has no FrameUpdater, so every
	// SendString sends — identical bytes to TS whenever the
	// chatbox was just (re)mounted, which is every dialogue
	// entry's case; a repeated identical write to an already
	// mounted group resends where TS would suppress it.
	SendString(text string, uid int)
	// SendClientScript runs a client script
	// (PacketSender.sendClientScript, PacketSender.ts:1754-1757;
	// encodeRunClientScript).
	SendClientScript(scriptID int, args ...ClientScriptArg)
	// SendInterfaceFlagsRange sets option flags over a slot
	// range (PacketSender.sendInterfaceFlagsRange,
	// PacketSender.ts:1699-1705; encodeWidgetSetFlagsRange —
	// dialogue never passes ops, so never the V2 form).
	SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int)
	// SendItemOnInterfaces sets an item on a component
	// (PacketSender.sendItemOnInterfaces, PacketSender.ts:629-633;
	// encodeWidgetSetItem). Dialogue passes the item zoom (400)
	// as the amount.
	SendItemOnInterfaces(uid, itemID, amount int)
	// SendInterfaceScript runs a widget script with no arguments
	// (PacketSender.sendInterfaceScript, PacketSender.ts:1706-1718;
	// encodeWidgetRunScript). Dialogue's only call is script
	// 2379 with no args, varps, varbits, or inventories.
	SendInterfaceScript(scriptID int)
	// SendInterfaceRemoval closes the player's interfaces
	// (PacketSender.sendInterfaceRemoval, PacketSender.ts:1011-1025).
	// Minimal scope: the mounted chatbox close, if any; see the
	// interface comment for what is NOT ported.
	SendInterfaceRemoval()
	// IsChatboxInterface reports whether groupID is the mounted
	// chatbox group (PacketSender.isChatboxInterface,
	// PacketSender.ts:972-974).
	IsChatboxInterface(groupID int) bool
	// WithChatboxOwner scopes owner to chatboxes mounted
	// synchronously by present, restoring the previous
	// presenting owner afterwards (PacketSender.withChatboxOwner,
	// PacketSender.ts:946-951). Only mounts inside present
	// record owner; mounts elsewhere record whatever owner (or
	// none) was presenting then.
	WithChatboxOwner(owner any, present func())
}

// dialogueSender returns the player's DialogueSender, or nil when
// the player is nil or the session does not implement it (the
// ChatMessageSender assertion pattern, message.go).
func (p *Player) dialogueSender() DialogueSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(DialogueSender)
	return sender
}

// Chatbox widget groups and component children, transcribed from the
// entry files (group ids confirmed against the active cache in TS;
// see the per-entry comments).
const (
	// npcDialogueGroupID is CHAT_LEFT (231, NpcDialogue.ts:7).
	npcDialogueGroupID = 231
	// playerDialogueGroupID is CHAT_RIGHT (217, PlayerDialogue.ts:7).
	playerDialogueGroupID = 217
	// statementDialogueGroupID is MESSAGEBOX (229, StatementDialogue.ts:7).
	statementDialogueGroupID = 229
	// itemStatementGroupID is OBJECTBOX (193, ItemStatementDialogue.ts:6).
	itemStatementGroupID = 193
	// doubleItemStatementGroupID is OBJECTBOX_DOUBLE (11,
	// DoubleItemStatementDialogue.ts:9).
	doubleItemStatementGroupID = 11
	// optionDialogueGroupID is the options group (219, OptionDialogue.ts:7).
	optionDialogueGroupID = 219
)

// OptionsWidgetID is the options list component
// (OptionDialogue.OPTIONS_WIDGET_ID, OptionDialogue.ts:7):
// (219 << 16) | 1. The manager's option-resume branch matches the
// resume packet's full widget id against it (DialogueManager.ts:74).
const OptionsWidgetID = (optionDialogueGroupID << 16) | 1

// Script ids and flags dialogue entries use.
const (
	// setTextAlignScriptID is cache script 600, if_settextalign
	// (Dialogue.ts:26, StatementDialogue.ts:45).
	setTextAlignScriptID = 600
	// objboxSetButtonsScriptID is objbox_setbuttons, which builds
	// the "Click here to continue" button (ItemStatementDialogue.ts:11).
	objboxSetButtonsScriptID = 2868
	// optionInterfaceScriptID is the script the option dialogue
	// runs before mounting (OptionDialogue.ts:45).
	optionInterfaceScriptID = 2379
	// optionSelectScriptID is the client script that fills the
	// option dialogue's title and options (OptionDialogue.ts:47).
	optionSelectScriptID = 58
	// pauseButtonFlag is the PAUSE_BUTTON flags value (1) marking a
	// "Click here to continue" component (Dialogue.ts:27 and the
	// entry files).
	pauseButtonFlag = 1
	// chatTextLineHeight is the line height native chat_left /
	// chat_right text widgets use (Dialogue.ts:29).
	chatTextLineHeight = 16
	// itemStatementZoom is the item zoom passed in the count field
	// of item-on-interface frames (ItemStatementDialogue.ts:13,
	// DoubleItemStatementDialogue.ts:15).
	itemStatementZoom = 400
	// boxChatLineLimit is the longest chat copy of a box's text
	// (Dialogue.BOX_CHAT_LINE_LIMIT, Dialogue.ts:6).
	boxChatLineLimit = 200
	// chatTypeMesbox is ChatType.MESBOX (115, ClientProtocol.ts:1118;
	// protocol.ChatTypeMesbox). The game layer cannot import the
	// protocol package, so the value is restated here with its cite,
	// as SendTypedMessage's chatType argument is a protocol ChatType
	// value (message.go).
	chatTypeMesbox = 115
)

// widgetUID packs a group id and child id into a component uid
// ((group << 16) | child), the UID form every entry file uses.
func widgetUID(groupID, childID int) int {
	return (groupID << 16) | childID
}

// sendBoxChatLine writes a message/item/double-item box's text to the
// chat as a mesbox line as the box opens
// (Dialogue.sendBoxChatLine, Dialogue.ts:18-22): line breaks become
// spaces and the line stops at 200 characters. A box with no text
// writes nothing. TS replaces only the exact string "<br>" (not
// "<br/>" or other casings) and slices to 200 UTF-16 code units;
// truncateUTF16Units reproduces the slice at rune boundaries (see
// its comment).
func sendBoxChatLine(p *Player, text string) {
	if text == "" {
		return
	}
	line := truncateUTF16Units(strings.ReplaceAll(text, "<br>", " "), boxChatLineLimit)
	p.SendTypedMessage(chatTypeMesbox, line, "")
}

// truncateUTF16Units cuts s to at most limit UTF-16 code units,
// reproducing String.prototype.slice(0, limit) for the BMP text
// dialogue carries. Units are counted per rune (1 for BMP runes, 2
// for astral runes); the cut lands on a rune boundary, so a lone
// astral rune straddling the limit is dropped whole where TS slice
// would split its surrogate pair — the only divergence, and only
// for text with astral characters at exactly the limit.
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

// sendDialogueChatText writes a chathead box's text and its
// "Click here to continue" (Dialogue.sendChatText, Dialogue.ts:24-35):
// the text string, the set-text-align script (1, 1, line height 16,
// text uid), the continue string, and the pause-button flags range.
func sendDialogueChatText(sender DialogueSender, text string, textUID, continueUID int) {
	sender.SendString(text, textUID)
	sender.SendClientScript(setTextAlignScriptID,
		ClientScriptInt(1), ClientScriptInt(1),
		ClientScriptInt(chatTextLineHeight), ClientScriptInt(textUID))
	sender.SendString("Click here to continue", continueUID)
	sender.SendInterfaceFlagsRange(continueUID, -1, -1, pauseButtonFlag)
}

// defaultNpcName is the name NpcDefinition.forId returns for an
// unknown id: the DEFAULT definition's name, "null"
// (NpcDefinition.ts:26, 89-101). The Go definition Store's default
// shape carries the same name (definition/npc.go), so an unwired
// resolver and a definition lookup of an unknown id agree.
const defaultNpcName = "null"

// DialogueOption is one option of an OptionDialogue
// (DialogueOption.ts:1-7: FIRST_OPTION..FIFTH_OPTION, 0..4).
//
// RESUME QUIRK (port faithfully, do not "fix"): the manager's resume
// branch passes the resume packet's one-based child index straight
// through as a DialogueOption (DialogueManager.ts:76), so a handler
// reached via a click receives 1 for the first option, 2 for the
// second — NOT DialogueOptionFirst (0). The enum values are what
// direct HandleOption callers pass; resume callers receive child
// indices. ExecuteOption handlers must compare against the value
// they actually receive for their path.
type DialogueOption int

// Option values (DialogueOption.ts:2-6).
const (
	DialogueOptionFirst  DialogueOption = 0
	DialogueOptionSecond DialogueOption = 1
	DialogueOptionThird  DialogueOption = 2
	DialogueOptionFourth DialogueOption = 3
	DialogueOptionFifth  DialogueOption = 4
)

// DialogueOptionAction handles an option choice
// (DialogueOptionAction.ts:3-5).
type DialogueOptionAction interface {
	// ExecuteOption runs the handler for the chosen option. See
	// DialogueOption's resume quirk for the values a resume
	// delivers versus a direct call.
	ExecuteOption(option DialogueOption)
}

// DialogueAction is an ActionDialogue's callback (Action,
// game/model/Action.ts:1-3: execute()).
type DialogueAction interface {
	// Execute runs the action. It may open other UI or advance
	// the chain itself (see ActionDialogue's self-advancing flag).
	Execute()
}

// DialogueActionFunc adapts a plain function to DialogueAction.
type DialogueActionFunc func()

// Execute runs the function.
func (f DialogueActionFunc) Execute() {
	if f != nil {
		f()
	}
}

// NpcDialogue is an NPC chathead line (entries/impl/NpcDialogue.ts).
// The NPC is carried as a definition id (npcId: number,
// NpcDialogue.ts:8, 12) — never as a live NPC object: the chathead
// frame takes the id, and the speaker name resolves through the
// definition lookup at send time (NpcDialogue.ts:35).
//
// DEVIATION (Go has no optional constructor parameters): TS takes
// speakerName, expression, and groupId as optional constructor
// parameters (NpcDialogue.ts:12-14). Go takes the required
// parameters in NewNpcDialogue and the optional ones through the
// chainable WithSpeakerName / WithExpression / WithGroupID setters,
// defaulting exactly as TS does (no speaker override, CALM,
// group 231).
type NpcDialogue struct {
	dialogueBase
	npcID       int
	text        string
	speakerName *string
	expression  DialogueExpression
	groupID     int
}

// NewNpcDialogue builds an NPC line for npcID with text, the CALM
// expression, group 231, and the speaker name resolved from the NPC
// definition at send time (NpcDialogue.ts:12-18).
func NewNpcDialogue(index, npcID int, text string) *NpcDialogue {
	return &NpcDialogue{
		dialogueBase: dialogueBase{index: index},
		npcID:        npcID,
		text:         text,
		expression:   DialogueExpressionCalm,
		groupID:      npcDialogueGroupID,
	}
}

// WithSpeakerName overrides the speaker name shown above the line
// (the speakerName constructor parameter, NpcDialogue.ts:13). An
// explicitly set empty name wins over the definition name, as TS
// nullish coalescing (??) only falls through for undefined/null —
// hence the pointer: unset, not empty, means "resolve".
func (d *NpcDialogue) WithSpeakerName(name string) *NpcDialogue {
	d.speakerName = &name
	return d
}

// WithExpression sets the chathead expression (the expression
// constructor parameter, NpcDialogue.ts:13).
func (d *NpcDialogue) WithExpression(expression DialogueExpression) *NpcDialogue {
	d.expression = expression
	return d
}

// WithGroupID sets the chatbox group the line mounts in (the groupId
// constructor parameter, NpcDialogue.ts:14). All component uids
// derive from it, as in TS.
func (d *NpcDialogue) WithGroupID(groupID int) *NpcDialogue {
	d.groupID = groupID
	return d
}

// GetNpcID returns the NPC definition id (NpcDialogue.ts:19-21).
func (d *NpcDialogue) GetNpcID() int { return d.npcID }

// GetText returns the line (NpcDialogue.ts:23-25).
func (d *NpcDialogue) GetText() string { return d.text }

// speakerName resolves the name above the line
// (NpcDialogue.ts:34-36): the explicit override when set, else the
// NPC definition's name with underscores replaced by spaces, else "".
// The definition lookup goes through the player's DialogueManager
// (see DialogueManager.SetNpcNameResolver): the game layer cannot
// import the definition package, and the manager is the dialogue
// subsystem's per-player state holder. Unwired, the name is the
// DEFAULT definition's "null", exactly as TS forId answers for an
// unknown id.
func (d *NpcDialogue) resolveSpeakerName(p *Player) string {
	if d.speakerName != nil {
		return *d.speakerName
	}
	name := defaultNpcName
	if p != nil {
		if m := p.GetDialogueManager(); m != nil {
			name = m.npcName(d.npcID)
		}
	}
	return strings.ReplaceAll(name, "_", " ")
}

// Send opens the NPC line (NpcDialogue.ts:27-39): mount the group,
// set the NPC head, animate it with the expression, write the
// speaker name, then the chat text + continue.
func (d *NpcDialogue) Send(p *Player) {
	if p == nil {
		return
	}
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	headUID := widgetUID(d.groupID, 2)
	sender.SendChatboxInterface(d.groupID, false)
	sender.SendNpcHeadOnInterface(d.npcID, headUID)
	sender.SendInterfaceAnimation(headUID, d.expression.ID())
	sender.SendString(d.resolveSpeakerName(p), widgetUID(d.groupID, 4))
	sendDialogueChatText(sender, d.text, widgetUID(d.groupID, 6), widgetUID(d.groupID, 5))
}

// SendHeld sends the line with its "Click here to continue" held back:
// blank and not clickable, for a cutscene's camera move
// (NpcDialogue.sendHeld, NpcDialogue.ts:42-48).
func (d *NpcDialogue) SendHeld(p *Player) {
	d.Send(p)
	if p == nil {
		return
	}
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	continueUID := widgetUID(d.groupID, 5)
	sender.SendString("", continueUID)
	sender.SendInterfaceFlagsRange(continueUID, -1, -1, 0)
}

// PlayerDialogue is the player's own chathead line
// (entries/impl/PlayerDialogue.ts), in CHAT_RIGHT (217).
//
// DEVIATION: TS takes the expression as an optional constructor
// parameter (PlayerDialogue.ts:15). Go takes it through the
// chainable WithExpression setter, defaulting to CALM as TS does.
type PlayerDialogue struct {
	dialogueBase
	text       string
	expression DialogueExpression
}

// NewPlayerDialogue builds a player line with the CALM expression
// (PlayerDialogue.ts:15-19).
func NewPlayerDialogue(index int, text string) *PlayerDialogue {
	return &PlayerDialogue{
		dialogueBase: dialogueBase{index: index},
		text:         text,
		expression:   DialogueExpressionCalm,
	}
}

// WithExpression sets the chathead expression (the expression
// constructor parameter, PlayerDialogue.ts:15).
func (d *PlayerDialogue) WithExpression(expression DialogueExpression) *PlayerDialogue {
	d.expression = expression
	return d
}

// GetText returns the line (PlayerDialogue.ts:21-23).
func (d *PlayerDialogue) GetText() string { return d.text }

// Send opens the player line (PlayerDialogue.ts:25-27, via the static
// send at lines 29-36).
func (d *PlayerDialogue) Send(p *Player) {
	SendPlayerDialogue(p, d.text, d.expression)
}

// SendPlayerDialogue opens a player chathead line with the given
// expression (PlayerDialogue.send, PlayerDialogue.ts:29-36): mount
// group 217, set the player head, animate it, write the player's
// display name, then the chat text + continue.
func SendPlayerDialogue(p *Player, text string, expression DialogueExpression) {
	if p == nil {
		return
	}
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	headUID := widgetUID(playerDialogueGroupID, 2)
	sender.SendChatboxInterface(playerDialogueGroupID, false)
	sender.SendPlayerHeadOnInterface(headUID)
	sender.SendInterfaceAnimation(headUID, expression.ID())
	sender.SendString(p.GetDisplayName(), widgetUID(playerDialogueGroupID, 4))
	sendDialogueChatText(sender, text, widgetUID(playerDialogueGroupID, 6), widgetUID(playerDialogueGroupID, 5))
}

// StatementDialogue is a MESSAGEBOX statement
// (entries/impl/StatementDialogue.ts), in group 229.
type StatementDialogue struct {
	dialogueBase
	text string
}

// NewStatementDialogue builds a statement (StatementDialogue.ts:15-18).
func NewStatementDialogue(index int, text string) *StatementDialogue {
	return &StatementDialogue{
		dialogueBase: dialogueBase{index: index},
		text:         text,
	}
}

// GetText returns the statement (StatementDialogue.ts:20-22).
func (d *StatementDialogue) GetText() string { return d.text }

// Send opens the statement, continuable (StatementDialogue.ts:24-26,
// via the static send with canContinue defaulting to true).
func (d *StatementDialogue) Send(p *Player) {
	SendStatementDialogue(p, d.text, true)
}

// statementLineHeights spreads two, three, and four drawn lines over
// the box; one line, or five and more, keep the font's own spacing
// (0) (StatementDialogue.LINE_HEIGHTS, StatementDialogue.ts:12).
var statementLineHeights = map[int]int{2: 31, 3: 24, 4: 17}

// breakTag splits text on <br> tags (ChatTextLayout.lineCount,
// ChatTextLayout.ts:88: /<br\s*\/?>/i).
var breakTag = regexp.MustCompile(`(?i)<br\s*/?>`)

// StatementLineHeight returns the text's line height
// (StatementDialogue.lineHeight, StatementDialogue.ts:33-36): the
// drawn-line count (via ChatTextLayout.lineCount at the 479-pixel
// text width) mapped through LINE_HEIGHTS.
//
// DEVIATION — the no-cache branch only: ChatTextLayout.lineCount
// counts client-wrapped lines using the cache font's advances
// (ChatTextLayout.ts:87-91), and the Go port has no font metrics, so
// this always takes lineCount's no-advances branch, where only the
// "<br>"s count (paragraph count). Long single-paragraph statements
// therefore read 1 line (height 0) where a cache-backed TS reads
// the wrapped count. The full ChatTextLayout port (font advances,
// wrap, pages) is NOT STARTED; when it lands, this function takes
// the wrapped count.
func StatementLineHeight(text string) int {
	if text == "" {
		return 0
	}
	lines := len(breakTag.Split(text, -1))
	return statementLineHeights[lines]
}

// SendStatementDialogue opens a message box
// (StatementDialogue.send, StatementDialogue.ts:43-54): the text goes
// to the chat first, then the group mounts, the text is written with
// its line-height alignment script, and the continue line is written
// (or blanked, without canContinue, as OSRS shows while something
// plays out before the player may carry on). canContinue is TS's
// default-true parameter, explicit here because Go has no default
// parameters.
func SendStatementDialogue(p *Player, text string, canContinue bool) {
	if p == nil {
		return
	}
	sendBoxChatLine(p, text)
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	textUID := widgetUID(statementDialogueGroupID, 3)
	continueUID := widgetUID(statementDialogueGroupID, 4)
	continueText := "Click here to continue"
	continueFlags := pauseButtonFlag
	if !canContinue {
		continueText = ""
		continueFlags = 0
	}
	sender.SendChatboxInterface(statementDialogueGroupID, false)
	sender.SendString(text, textUID)
	sender.SendClientScript(setTextAlignScriptID,
		ClientScriptInt(1), ClientScriptInt(1),
		ClientScriptInt(StatementLineHeight(text)), ClientScriptInt(textUID))
	sender.SendString(continueText, continueUID)
	sender.SendInterfaceFlagsRange(continueUID, -1, -1, continueFlags)
}

// ItemStatementDialogue is an OBJECTBOX message beside an item
// (entries/impl/ItemStatementDialogue.ts), in group 193.
type ItemStatementDialogue struct {
	dialogueBase
	itemID int
	text   string
}

// NewItemStatementDialogue builds an item statement
// (ItemStatementDialogue.ts:18-22).
func NewItemStatementDialogue(index, itemID int, text string) *ItemStatementDialogue {
	return &ItemStatementDialogue{
		dialogueBase: dialogueBase{index: index},
		itemID:       itemID,
		text:         text,
	}
}

// GetItemID returns the item id (ItemStatementDialogue.ts:24-26).
func (d *ItemStatementDialogue) GetItemID() int { return d.itemID }

// GetText returns the message (ItemStatementDialogue.ts:28-30).
func (d *ItemStatementDialogue) GetText() string { return d.text }

// Send opens the item box (ItemStatementDialogue.ts:32-34, via the
// static send at lines 37-45).
func (d *ItemStatementDialogue) Send(p *Player) {
	SendItemStatementDialogue(p, d.itemID, d.text)
}

// SendItemStatementDialogue opens an item box
// (ItemStatementDialogue.send, ItemStatementDialogue.ts:37-45): the
// text goes to the chat first, then the group mounts, the
// set-buttons script builds the continue button, the universe
// component's flags range is set, the item is placed at zoom 400,
// and the text is written.
func SendItemStatementDialogue(p *Player, itemID int, text string) {
	if p == nil {
		return
	}
	sendBoxChatLine(p, text)
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	sender.SendChatboxInterface(itemStatementGroupID, false)
	sender.SendClientScript(objboxSetButtonsScriptID, ClientScriptString("Click here to continue"))
	sender.SendInterfaceFlagsRange(widgetUID(itemStatementGroupID, 0), 0, 1, pauseButtonFlag)
	sender.SendItemOnInterfaces(widgetUID(itemStatementGroupID, 1), itemID, itemStatementZoom)
	sender.SendString(text, widgetUID(itemStatementGroupID, 2))
}

// DoubleItemStatementDialogue is an OBJECTBOX_DOUBLE message between
// two items (entries/impl/DoubleItemStatementDialogue.ts), in group 11.
type DoubleItemStatementDialogue struct {
	dialogueBase
	firstItemID  int
	secondItemID int
	text         string
}

// NewDoubleItemStatementDialogue builds a double-item statement
// (DoubleItemStatementDialogue.ts:21-26).
func NewDoubleItemStatementDialogue(index, firstItemID, secondItemID int, text string) *DoubleItemStatementDialogue {
	return &DoubleItemStatementDialogue{
		dialogueBase: dialogueBase{index: index},
		firstItemID:  firstItemID,
		secondItemID: secondItemID,
		text:         text,
	}
}

// GetItemIDs returns the two item ids
// (DoubleItemStatementDialogue.ts:28-30, which returns a tuple).
func (d *DoubleItemStatementDialogue) GetItemIDs() (int, int) {
	return d.firstItemID, d.secondItemID
}

// GetText returns the message (DoubleItemStatementDialogue.ts:32-34).
func (d *DoubleItemStatementDialogue) GetText() string { return d.text }

// Send opens the double-item box (DoubleItemStatementDialogue.ts:36-38,
// via the static send at lines 41-50).
func (d *DoubleItemStatementDialogue) Send(p *Player) {
	SendDoubleItemStatementDialogue(p, d.firstItemID, d.secondItemID, d.text)
}

// SendDoubleItemStatementDialogue opens a double-item box
// (DoubleItemStatementDialogue.send, DoubleItemStatementDialogue.ts:41-50):
// the text goes to the chat first, then the group mounts, the
// continue component's flags range is set, the continue string is
// written, both items are placed at zoom 400, and the text is
// written. Note the order differs from the single-item box (flags
// and continue string before the items) — transcribed, not unified.
func SendDoubleItemStatementDialogue(p *Player, firstItemID, secondItemID int, text string) {
	if p == nil {
		return
	}
	sendBoxChatLine(p, text)
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	continueUID := widgetUID(doubleItemStatementGroupID, 4)
	sender.SendChatboxInterface(doubleItemStatementGroupID, false)
	sender.SendInterfaceFlagsRange(continueUID, -1, -1, pauseButtonFlag)
	sender.SendString("Click here to continue", continueUID)
	sender.SendItemOnInterfaces(widgetUID(doubleItemStatementGroupID, 1), firstItemID, itemStatementZoom)
	sender.SendItemOnInterfaces(widgetUID(doubleItemStatementGroupID, 3), secondItemID, itemStatementZoom)
	sender.SendString(text, widgetUID(doubleItemStatementGroupID, 2))
}

// OptionDialogue is a "Select an Option" choice list
// (entries/impl/OptionDialogue.ts), in group 219.
type OptionDialogue struct {
	dialogueBase
	action  DialogueOptionAction
	title   string
	options []string
}

// NewOptionDialogue builds an option dialogue
// (OptionDialogue.ts:12-16). options are the choice labels in order;
// the title defaults to "Select an Option" at send time until
// SetTitle sets it.
func NewOptionDialogue(index int, action DialogueOptionAction, options ...string) *OptionDialogue {
	return &OptionDialogue{
		dialogueBase: dialogueBase{index: index},
		action:       action,
		options:      options,
	}
}

// GetTitle returns the question above the options ("" when unset;
// OptionDialogue.ts:18-20 returns undefined, whose Go zero value is
// "").
func (d *OptionDialogue) GetTitle() string { return d.title }

// SetTitle sets the question above the options and returns the
// dialogue for chaining (OptionDialogue.setTitle,
// OptionDialogue.ts:23-26).
func (d *OptionDialogue) SetTitle(title string) *OptionDialogue {
	d.title = title
	return d
}

// GetOptions returns the choice labels, live (OptionDialogue.ts:28-30).
func (d *OptionDialogue) GetOptions() []string { return d.options }

// Execute runs the action for the chosen option; a nil action is a
// no-op (OptionDialogue.execute, OptionDialogue.ts:32-37).
func (d *OptionDialogue) Execute(option DialogueOption) {
	if d.action == nil {
		return
	}
	d.action.ExecuteOption(option)
}

// Send opens the option dialogue (OptionDialogue.ts:39-41, via the
// static send at lines 43-49).
func (d *OptionDialogue) Send(p *Player) {
	SendOptionDialogue(p, d.title, d.options)
}

// SendOptionDialogue opens an option dialogue
// (OptionDialogue.send, OptionDialogue.ts:43-49): run script 2379,
// mount group 219 unclamped, run client script 58 with the title
// ("Select an Option" when unset or empty) and the options joined
// by "|", then set the flags range over the option slots.
func SendOptionDialogue(p *Player, title string, options []string) {
	if p == nil {
		return
	}
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	if title == "" {
		title = "Select an Option"
	}
	sender.SendInterfaceScript(optionInterfaceScriptID)
	sender.SendChatboxInterface(optionDialogueGroupID, true)
	sender.SendClientScript(optionSelectScriptID,
		ClientScriptString(title), ClientScriptString(strings.Join(options, "|")))
	sender.SendInterfaceFlagsRange(OptionsWidgetID, 1, len(options), pauseButtonFlag)
}

// ActionDialogue runs an action instead of showing an entry
// (entries/impl/ActionDialogue.ts).
//
// DEVIATION: TS takes selfAdvancing as an optional constructor
// parameter defaulting to false (ActionDialogue.ts:12). Go takes it
// through the chainable WithSelfAdvancing setter.
type ActionDialogue struct {
	dialogueBase
	action        DialogueAction
	selfAdvancing bool
}

// NewActionDialogue builds an action entry (ActionDialogue.ts:12-16).
func NewActionDialogue(index int, action DialogueAction) *ActionDialogue {
	return &ActionDialogue{
		dialogueBase: dialogueBase{index: index},
		action:       action,
	}
}

// WithSelfAdvancing marks the action as moving the chain on itself
// (a timer), so a Continue on whatever chatbox is still showing must
// not advance past it (ActionDialogue.ts:8-11, DialogueManager.ts:82).
func (d *ActionDialogue) WithSelfAdvancing(selfAdvancing bool) *ActionDialogue {
	d.selfAdvancing = selfAdvancing
	return d
}

// IsSelfAdvancing reports whether the action advances the chain
// itself (ActionDialogue.ts:22-24).
func (d *ActionDialogue) IsSelfAdvancing() bool { return d.selfAdvancing }

// Send runs the action (ActionDialogue.ts:18-20). It sends no frames
// and ignores the player, as in TS. DEVIATION: a nil action is a
// no-op here; TS would throw calling execute() on it. Content must
// not build action entries with a nil action.
func (d *ActionDialogue) Send(_ *Player) {
	if d.action == nil {
		return
	}
	d.action.Execute()
}

// EndDialogue closes the dialogue interface
// (entries/impl/EndDialogue.ts).
type EndDialogue struct {
	dialogueBase
}

// NewEndDialogue builds an end entry (EndDialogue.ts:5-7).
func NewEndDialogue(index int) *EndDialogue {
	return &EndDialogue{dialogueBase: dialogueBase{index: index}}
}

// Send removes the interface (EndDialogue.ts:9-11). Note it does NOT
// reset the manager: the manager's index still points at this entry
// until the next start or advance (which resets when the entry is
// gone) — transcribed, not "fixed".
func (d *EndDialogue) Send(p *Player) {
	if p == nil {
		return
	}
	sender := p.dialogueSender()
	if sender == nil {
		return
	}
	sender.SendInterfaceRemoval()
}

// Ensure the entry types satisfy Dialogue at compile time.
var (
	_ Dialogue = (*NpcDialogue)(nil)
	_ Dialogue = (*PlayerDialogue)(nil)
	_ Dialogue = (*OptionDialogue)(nil)
	_ Dialogue = (*StatementDialogue)(nil)
	_ Dialogue = (*ItemStatementDialogue)(nil)
	_ Dialogue = (*DoubleItemStatementDialogue)(nil)
	_ Dialogue = (*ActionDialogue)(nil)
	_ Dialogue = (*EndDialogue)(nil)
)
