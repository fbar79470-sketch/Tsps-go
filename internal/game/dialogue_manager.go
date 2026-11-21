// DialogueManager: the per-player dialogue state machine
// (dialogues/DialogueManager.ts:16-159).
//
// STATE. An indexed entry map (dialogues), the current index, and an
// opaque owner. The manager is active exactly when the map holds the
// current index (isActive, DialogueManager.ts:52-54). Reset clears
// the map, sets the index to -1, and drops the owner
// (DialogueManager.ts:46-50). Note the asymmetry the port preserves:
// advance() off the end of a chain sends an interface removal WITHOUT
// resetting (startDialogueOption only removes; DialogueManager.ts:136-141),
// while advance() from an already-missing entry resets AND removes
// (DialogueManager.ts:90-96). Either way IsActive reports false and
// GetOwner/GetCurrent report nothing afterwards.
//
// SESSION-ONLY STATE (verified against TS): DialogueManager is a
// plain field initializer on Player (Player.ts:143) with a getter
// (Player.ts:1642-1644), and no dialogue state appears in
// PlayerSave/PlayerPersistence (grep-verified at commit bfd81e3: no
// "dialogue" references in either file). Dialogue state therefore
// does not persist: logging out mid-chain drops the chain, and the
// Go port keeps no owned save fields for it. The persist package is
// untouched.
//
// OWNER SCOPING. The owner is an opaque value the chain's starter
// attaches (startDialog/startDialogues' owner parameter). Visible
// entries are sent inside WithChatboxOwner(owner), so the chatbox
// they mount records the owner; ActionDialogue entries are sent with
// a nil presentation owner, because an arbitrary action callback may
// open a different feature's prompt, and only visible entries inherit
// the chain owner (DialogueManager.ts:142-147). When the chain has no
// owner, entries send directly, with no scoping call at all —
// including ActionDialogue entries, which then mount (if they mount
// anything) unscoped, exactly as TS sends them outside
// withChatboxOwner. GetOwner returns the owner only while active.
//
// THREADING. Tick-goroutine-only, like the rest of Player: chains
// start from turn-packet listeners and content hooks on the tick,
// and resumes arrive through the turn queue. The NPC-name resolver
// (SetNpcNameResolver) is set once at entry, before the player is
// visible to the tick goroutine, following the SetItemDefinitions
// pattern; it is read without a lock thereafter.

package game

// DialogueIdentifierTest is the only static-dialogue identifier
// (DialogueIdentifiers.TEST = 0, util/DialogueIdentifiers.ts:2).
const DialogueIdentifierTest = 0

// StaticDialogueDefinition is one STATIC_DIALOGUES entry
// (DialogueManager.ts:11-14): a factory for the builder plus the
// index the chain starts at.
type StaticDialogueDefinition struct {
	// Create builds a fresh builder for the dialogue. TS calls
	// create() on every startStaticDialogue, so each start gets
	// its own entries.
	Create func() DialogueBuilder
	// StartIndex is the index the chain starts at.
	StartIndex int
}

// StaticDialogues is the static-dialogue registry
// (DialogueManager.STATIC_DIALOGUES, DialogueManager.ts:17-19). TS
// holds exactly one entry, TEST; the Go map holds the same one.
// Like the TS Map, it is mutable; content registers further static
// dialogues by adding entries.
var StaticDialogues = map[int]StaticDialogueDefinition{
	DialogueIdentifierTest: {Create: func() DialogueBuilder { return NewTestStaticDialogue() }, StartIndex: 0},
}

// DialogueManager runs one player's dialogue chain. Build it with
// NewDialogueManager; Player carries one (see GetDialogueManager).
type DialogueManager struct {
	player *Player

	// dialogues holds the current chain's entries by index
	// (DialogueManager.ts:26).
	dialogues map[int]Dialogue
	// index is the current entry's index (DialogueManager.ts:31).
	// TS declares it without an initializer (undefined until the
	// first start/reset); Go starts it at -1, the post-reset
	// value, which is observably identical — no entry lives at
	// either key, so IsActive is false either way.
	index int
	// owner is the chain's opaque owner, or nil (DialogueManager.ts:32).
	owner any

	// npcNameResolver resolves an NPC definition id to its name
	// for NpcDialogue speaker lines (see SetNpcNameResolver). Nil
	// until the coordinator installs one.
	npcNameResolver func(id int) string
}

// NewDialogueManager returns a DialogueManager for player
// (DialogueManager.ts:39-41), reset to the inactive state.
func NewDialogueManager(player *Player) *DialogueManager {
	m := &DialogueManager{
		player:    player,
		dialogues: make(map[int]Dialogue),
		index:     -1,
	}
	return m
}

// SetNpcNameResolver installs the NPC-name lookup NpcDialogue uses
// when no explicit speaker name is set (NpcDialogue.ts:35:
// NpcDefinition.forId(npcId).getName()). The coordinator installs it
// once at entry over the loaded definition store (the
// SetItemDefinitions pattern); a nil resolver answers the DEFAULT
// definition's name for every id (see npcName). It is set before the
// player is visible to the tick goroutine and never reassigned.
func (m *DialogueManager) SetNpcNameResolver(resolver func(id int) string) {
	m.npcNameResolver = resolver
}

// npcName resolves id to its definition name, with underscores kept:
// the caller applies the underscore-to-space replacement
// (NpcDialogue.ts:35). Unwired, it answers the DEFAULT definition's
// name ("null"), exactly as TS NpcDefinition.forId answers for an
// unknown id.
func (m *DialogueManager) npcName(id int) string {
	if m.npcNameResolver != nil {
		return m.npcNameResolver(id)
	}
	return defaultNpcName
}

// Reset clears the chain: entries, index, and owner
// (DialogueManager.ts:46-50). It sends nothing.
func (m *DialogueManager) Reset() {
	m.dialogues = make(map[int]Dialogue)
	m.index = -1
	m.owner = nil
}

// IsActive reports whether the map holds the current index
// (DialogueManager.ts:52-54).
func (m *DialogueManager) IsActive() bool {
	_, ok := m.dialogues[m.index]
	return ok
}

// GetCurrent returns the entry currently shown, or nil when inactive
// (DialogueManager.ts:57-59).
func (m *DialogueManager) GetCurrent() Dialogue {
	if !m.IsActive() {
		return nil
	}
	return m.dialogues[m.index]
}

// GetOwner returns the chain's opaque owner, or nil when inactive
// (DialogueManager.ts:62-64). The owner is retained across the
// chain's native continuations (advance/resume keep it; only a new
// start or a reset replaces it).
func (m *DialogueManager) GetOwner() any {
	if !m.IsActive() {
		return nil
	}
	return m.owner
}

// CanContinue reports whether a resume for widgetID may advance the
// chain: active, and the widget's group is the mounted chatbox group
// (DialogueManager.ts:66-68: isChatboxInterface(widgetId >>> 16)).
// The unsigned shift matters: widget ids arrive as signed int32s,
// and TS >>> reinterprets them as uint32 first. With no
// DialogueSender attached, there is no chatbox to continue, so it
// reports false (see the DialogueSender gap in dialogue_entries.go).
func (m *DialogueManager) CanContinue(widgetID int) bool {
	if !m.IsActive() {
		return false
	}
	sender := m.player.dialogueSender()
	if sender == nil {
		return false
	}
	return sender.IsChatboxInterface(int(uint32(widgetID) >> 16))
}

// HandleResume handles a native dialogue continue / option resume
// (DialogueManager.ts:71-85), returning whether the resume belonged
// to this dialogue. It is the RESUME_PAUSEBUTTON dispatch target
// (NetworkBuilder.ts:950: handleResume(packet.widgetId,
// packet.childIndex), after the interface-action-click hook; the
// decode and dispatch live outside this package).
//
//   - An option resume (current entry is an OptionDialogue) must
//     carry the options widget's full uid and a one-based child
//     index within the option count; anything else returns false
//     without touching the chain. A valid one fires HandleOption
//     with the child index as the DialogueOption (see the resume
//     quirk on DialogueOption) and returns true. This branch does
//     NOT check CanContinue.
//   - Otherwise the resume must pass CanContinue, or it returns
//     false.
//   - A self-advancing ActionDialogue swallows the resume (returns
//     true without advancing): an overhead line or a held moment
//     leaves the previous chatbox up, and held Space would
//     otherwise end the chain under its timer (DialogueManager.ts:80-82).
//   - Anything else advances and returns true.
func (m *DialogueManager) HandleResume(widgetID, childIndex int) bool {
	current := m.GetCurrent()
	if options, ok := current.(*OptionDialogue); ok {
		if widgetID != OptionsWidgetID || childIndex < 1 || childIndex > len(options.GetOptions()) {
			return false
		}
		m.HandleOption(DialogueOption(childIndex))
		return true
	}
	if !m.CanContinue(widgetID) {
		return false
	}
	if action, ok := current.(*ActionDialogue); ok && action.IsSelfAdvancing() {
		return true
	}
	m.Advance()
	return true
}

// Advance starts the next entry (DialogueManager.ts:90-99). From a
// missing current entry it resets and removes the interface; from a
// live entry it starts index+1, which removes the interface (without
// resetting) when no entry lives there.
func (m *DialogueManager) Advance() {
	current, ok := m.dialogues[m.index]
	if !ok || current == nil {
		m.Reset()
		m.removeInterface()
		return
	}
	m.StartDialogue(m.index + 1)
}

// StartDialogue starts the entry at index in the current chain
// (DialogueManager.ts:101-104). Unlike StartDialog, it does not
// reset or replace the map: it repoints the index and presents
// whatever entry lives there (or removes the interface when none
// does).
func (m *DialogueManager) StartDialogue(index int) {
	m.index = index
	m.startDialogueOption()
}

// StartStaticDialogue starts a registered static dialogue by id
// (DialogueManager.ts:106-113), building a fresh builder from the
// registry and starting at its start index. It returns false when
// no dialogue is registered under id, leaving the current chain
// untouched.
func (m *DialogueManager) StartStaticDialogue(id int) bool {
	definition, ok := StaticDialogues[id]
	if !ok {
		return false
	}
	m.StartDialog(definition.Create(), definition.StartIndex)
	return true
}

// StartDialogues starts builder's chain at index 0
// (DialogueManager.ts:115-117). At most one owner is used; pass none
// for an unowned chain.
func (m *DialogueManager) StartDialogues(builder DialogueBuilder, owner ...any) {
	m.StartDialog(builder, 0, owner...)
}

// StartDialog starts builder's chain at index
// (DialogueManager.ts:119-124). A dynamic builder is built for the
// player first; the entries are then copied into a fresh chain with
// the given owner (or none).
func (m *DialogueManager) StartDialog(builder DialogueBuilder, index int, owner ...any) {
	if dynamic, ok := builder.(DynamicDialogueBuilder); ok {
		dynamic.Build(m.player)
	}
	m.startDialogueMap(builder.GetDialogues(), index, firstOwner(owner))
}

// startDialogueMap replaces the chain with entries, starting at
// index under owner (DialogueManager.ts:126-134): reset, record the
// owner, copy the entries, set the index, present.
func (m *DialogueManager) startDialogueMap(entries map[int]Dialogue, index int, owner any) {
	m.Reset()
	m.owner = owner
	for key, value := range entries {
		m.dialogues[key] = value
	}
	m.index = index
	m.startDialogueOption()
}

// startDialogueOption presents the current entry
// (DialogueManager.ts:136-148). A missing entry removes the
// interface and leaves the (inactive) chain in place. Otherwise the
// entry sends directly when the chain is unowned, or inside
// WithChatboxOwner when owned — with a nil presentation owner for
// ActionDialogue entries, which must scope any UI they present
// themselves, and the chain owner for visible entries. With no
// DialogueSender attached, the scoping call cannot run and the
// entry sends directly (dropping its frames); the owner is still
// recorded for GetOwner.
func (m *DialogueManager) startDialogueOption() {
	dialogue, ok := m.dialogues[m.index]
	if !ok || dialogue == nil {
		m.removeInterface()
		return
	}
	presentationOwner := m.owner
	if _, isAction := dialogue.(*ActionDialogue); isAction {
		presentationOwner = nil
	}
	if m.owner == nil {
		dialogue.Send(m.player)
		return
	}
	sender := m.player.dialogueSender()
	if sender == nil {
		dialogue.Send(m.player)
		return
	}
	sender.WithChatboxOwner(presentationOwner, func() { dialogue.Send(m.player) })
}

// HandleOption fires option on the current entry
// (DialogueManager.ts:150-157). When the current entry is not an
// OptionDialogue, it removes the interface instead; TS reads the
// map directly rather than through getCurrent, which is equivalent
// here because a missing entry is not an OptionDialogue either.
func (m *DialogueManager) HandleOption(option DialogueOption) {
	dialogue, ok := m.dialogues[m.index]
	if !ok {
		m.removeInterface()
		return
	}
	options, isOptions := dialogue.(*OptionDialogue)
	if !isOptions {
		m.removeInterface()
		return
	}
	options.Execute(option)
}

// removeInterface sends an interface removal through the player's
// sender, dropping it when no sender is attached (the DialogueSender
// gap: session-less players have no interface to remove).
func (m *DialogueManager) removeInterface() {
	if m.player == nil {
		return
	}
	if sender := m.player.dialogueSender(); sender != nil {
		sender.SendInterfaceRemoval()
	}
}

// firstOwner returns the first owner, or nil when none was passed.
// StartDialog/StartDialogues take a variadic owner to mirror TS's
// optional parameter; only the first is meaningful.
func firstOwner(owner []any) any {
	if len(owner) == 0 {
		return nil
	}
	return owner[0]
}
