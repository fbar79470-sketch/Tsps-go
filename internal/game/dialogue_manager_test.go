package game

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// dialogueTestSession is a SessionPort + ChatMessageSender +
// DialogueSender test double recording every call in order. Its
// chatbox state mirrors the PacketSender state machine the real
// session implementation must keep (PacketSender.ts:228-231, 609,
// 946-951, 1670-1676): mounts record the presenting owner, removal
// closes the mounted chatbox, and WithChatboxOwner restores the
// previous presenting owner. Tests assert on calls (the ordered
// call log) and on ownersSeen (the presenting owner at each mount).
type dialogueTestSession struct {
	open bool

	calls      []string
	sent       [][]byte
	messages   []string
	typed      [][3]any // [chatType, message, from]
	ownersSeen []any
	withOwners int
	removals   int
	chatboxID  int
	chatboxOwn any
	presenting any
	unclamped  bool
}

func newDialogueTestSession() *dialogueTestSession {
	return &dialogueTestSession{open: true, chatboxID: -1}
}

func (f *dialogueTestSession) SendPacket(packet []byte)          { f.sent = append(f.sent, packet) }
func (f *dialogueTestSession) SendRunEnergy(int, bool, int, int) {}
func (f *dialogueTestSession) SendDestination(int, int)          {}
func (f *dialogueTestSession) SendVarp(int, int)                 {}
func (f *dialogueTestSession) FlushTick(int64) error             { return nil }
func (f *dialogueTestSession) Close(int, string)                 { f.open = false }
func (f *dialogueTestSession) IsOpen() bool                      { return f.open }

func (f *dialogueTestSession) SendGameMessage(message string) {
	f.messages = append(f.messages, message)
	f.calls = append(f.calls, fmt.Sprintf("message(%q)", message))
}

func (f *dialogueTestSession) SendTypedGameMessage(chatType int, message, from string) {
	f.typed = append(f.typed, [3]any{chatType, message, from})
	f.calls = append(f.calls, fmt.Sprintf("typed(%d,%q,%q)", chatType, message, from))
}

func (f *dialogueTestSession) SendChatboxInterface(groupID int, unclamp bool) {
	f.calls = append(f.calls, fmt.Sprintf("chatbox(%d,%v)", groupID, unclamp))
	f.chatboxOwn = f.presenting
	f.chatboxID = groupID
	f.unclamped = unclamp
	f.ownersSeen = append(f.ownersSeen, f.presenting)
}

func (f *dialogueTestSession) SendNpcHeadOnInterface(npcID, uid int) {
	f.calls = append(f.calls, fmt.Sprintf("npcHead(%d,%d)", npcID, uid))
}

func (f *dialogueTestSession) SendPlayerHeadOnInterface(uid int) {
	f.calls = append(f.calls, fmt.Sprintf("playerHead(%d)", uid))
}

func (f *dialogueTestSession) SendInterfaceAnimation(uid, animationID int) {
	f.calls = append(f.calls, fmt.Sprintf("anim(%d,%d)", uid, animationID))
}

func (f *dialogueTestSession) SendString(text string, uid int) {
	f.calls = append(f.calls, fmt.Sprintf("string(%d,%q)", uid, text))
}

func (f *dialogueTestSession) SendClientScript(scriptID int, args ...ClientScriptArg) {
	parts := make([]string, len(args))
	for i, a := range args {
		if a.IsString() {
			parts[i] = fmt.Sprintf("%q", a.StringValue())
		} else {
			parts[i] = fmt.Sprintf("%d", a.IntValue())
		}
	}
	f.calls = append(f.calls, fmt.Sprintf("script(%d,[%s])", scriptID, strings.Join(parts, " ")))
}

func (f *dialogueTestSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	f.calls = append(f.calls, fmt.Sprintf("flags(%d,%d,%d,%d)", uid, fromSlot, toSlot, flags))
}

func (f *dialogueTestSession) SendItemOnInterfaces(uid, itemID, amount int) {
	f.calls = append(f.calls, fmt.Sprintf("item(%d,%d,%d)", uid, itemID, amount))
}

func (f *dialogueTestSession) SendInterfaceScript(scriptID int) {
	f.calls = append(f.calls, fmt.Sprintf("ifaceScript(%d)", scriptID))
}

func (f *dialogueTestSession) SendInterfaceRemoval() {
	f.calls = append(f.calls, "removal")
	f.removals++
	// Removal closes the mounted chatbox, as closeTrackedInterfaces
	// does for the chatbox target (PacketSender.ts:920-933).
	f.chatboxID = -1
	f.chatboxOwn = nil
	f.unclamped = false
}

func (f *dialogueTestSession) IsChatboxInterface(groupID int) bool {
	return groupID == f.chatboxID
}

func (f *dialogueTestSession) WithChatboxOwner(owner any, present func()) {
	f.withOwners++
	previous := f.presenting
	f.presenting = owner
	defer func() { f.presenting = previous }()
	present()
}

// newDialoguePlayer returns a player wired to a recording session.
func newDialoguePlayer() (*Player, *dialogueTestSession) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	s := newDialogueTestSession()
	p.Session = s
	return p, s
}

func TestDialogueManagerInactiveInitially(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	if m == nil {
		t.Fatal("GetDialogueManager returned nil for a NewPlayer")
	}
	if m.IsActive() {
		t.Error("fresh manager is active")
	}
	if m.GetCurrent() != nil {
		t.Error("fresh manager has a current entry")
	}
	if m.GetOwner() != nil {
		t.Error("fresh manager has an owner")
	}
	if m.CanContinue((231 << 16) | 5) {
		t.Error("fresh manager can continue")
	}
	if m.HandleResume((231<<16)|5, 0) {
		t.Error("fresh manager handled a resume")
	}
	// Advance from a missing entry resets and removes the interface
	// (DialogueManager.ts:90-96).
	m.Advance()
	if s.removals != 1 {
		t.Errorf("removals = %d, want 1", s.removals)
	}
	if m.IsActive() {
		t.Error("manager active after advance from empty")
	}
}

func TestDialogueManagerStartAdvanceEnd(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Hello.")).
		Add(NewStatementDialogue(1, "A statement.")).
		Add(NewEndDialogue(2)))
	if !m.IsActive() {
		t.Fatal("manager inactive after start")
	}
	if got := m.GetCurrent().GetIndex(); got != 0 {
		t.Fatalf("current index = %d, want 0", got)
	}
	if len(s.calls) == 0 || !strings.HasPrefix(s.calls[0], "chatbox(217,") {
		t.Fatalf("first call = %v, want a 217 chatbox mount", s.calls)
	}
	m.Advance()
	if got := m.GetCurrent().GetIndex(); got != 1 {
		t.Fatalf("current index after advance = %d, want 1", got)
	}
	m.Advance()
	// The end entry sends a removal but does not reset the manager:
	// the index still points at a live entry.
	if got := m.GetCurrent().GetIndex(); got != 2 {
		t.Fatalf("current index after second advance = %d, want 2", got)
	}
	if !m.IsActive() {
		t.Error("manager inactive while pointing at the end entry")
	}
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1 (the end entry)", s.removals)
	}
	// Advancing past the end removes the interface without resetting.
	m.Advance()
	if m.IsActive() {
		t.Error("manager active after advancing past the end")
	}
	if s.removals != 2 {
		t.Fatalf("removals = %d, want 2", s.removals)
	}
	// A further advance resets and removes again.
	m.Advance()
	if s.removals != 3 {
		t.Fatalf("removals = %d, want 3", s.removals)
	}
}

func TestDialogueManagerStartDialogueMissingIndexKeepsMap(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Hello.")))
	m.StartDialogue(99)
	if m.IsActive() {
		t.Error("manager active after starting a missing index")
	}
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1", s.removals)
	}
	// The map was NOT reset: index 0 still presents.
	m.StartDialogue(0)
	if !m.IsActive() {
		t.Error("manager inactive after re-starting index 0")
	}
	if got := m.GetCurrent().GetIndex(); got != 0 {
		t.Fatalf("current index = %d, want 0", got)
	}
}

func TestDialogueManagerReset(t *testing.T) {
	p, _ := newDialoguePlayer()
	m := p.GetDialogueManager()
	owner := &struct{ name string }{"owner"}
	m.StartDialogues(NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Hi.")), owner)
	m.Reset()
	if m.IsActive() || m.GetCurrent() != nil || m.GetOwner() != nil {
		t.Error("reset manager still holds state")
	}
}

func TestDialogueManagerOwnerScoping(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	owner := &struct{ name string }{"quest"}
	mountedByAction := -1
	action := NewActionDialogue(1, DialogueActionFunc(func() {
		// The action opens a different feature's prompt: it mounts
		// a chatbox itself, inside the presentation the manager
		// scoped for it.
		s.SendChatboxInterface(229, false)
		mountedByAction = len(s.ownersSeen) - 1
	}))
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Hi.")).
		Add(action).
		Add(NewPlayerDialogue(2, "Back.")), owner)
	if m.GetOwner() != any(owner) {
		t.Error("GetOwner did not return the chain owner")
	}
	if s.withOwners != 1 {
		t.Fatalf("WithChatboxOwner calls = %d, want 1 (the first entry)", s.withOwners)
	}
	m.Advance() // the action entry: presentation owner must be nil
	if s.withOwners != 2 {
		t.Fatalf("WithChatboxOwner calls = %d, want 2", s.withOwners)
	}
	if mountedByAction < 0 {
		t.Fatal("action did not mount")
	}
	if s.ownersSeen[mountedByAction] != nil {
		t.Errorf("action presentation owner = %v, want nil", s.ownersSeen[mountedByAction])
	}
	m.Advance() // visible entry again: the chain owner returns
	if got := s.ownersSeen[len(s.ownersSeen)-1]; got != any(owner) {
		t.Errorf("last mount owner = %v, want the chain owner", got)
	}
	if m.GetOwner() != any(owner) {
		t.Error("owner not retained across continuations")
	}
	// The presenting owner is restored after each presentation.
	if s.presenting != nil {
		t.Errorf("presenting owner = %v after presentations, want nil", s.presenting)
	}
}

func TestDialogueManagerUnownedChainSendsDirectly(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Hi.")).
		Add(NewActionDialogue(1, DialogueActionFunc(func() {}))))
	if s.withOwners != 0 {
		t.Errorf("WithChatboxOwner calls = %d, want 0 for an unowned chain", s.withOwners)
	}
	m.Advance()
	if s.withOwners != 0 {
		t.Errorf("WithChatboxOwner calls = %d after advancing an unowned chain, want 0", s.withOwners)
	}
}

func TestDialogueManagerHandleResume(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "One.")).
		Add(NewPlayerDialogue(1, "Two.")))
	chatboxUID := (217 << 16) | 5
	otherUID := (231 << 16) | 5

	// A resume for a group that is not the mounted chatbox is refused.
	if m.HandleResume(otherUID, 0) {
		t.Error("resume for a non-chatbox group handled")
	}
	if got := m.GetCurrent().GetIndex(); got != 0 {
		t.Fatalf("index after refused resume = %d, want 0", got)
	}
	// A resume for the mounted chatbox advances.
	if !m.HandleResume(chatboxUID, 0) {
		t.Error("resume for the chatbox group refused")
	}
	if got := m.GetCurrent().GetIndex(); got != 1 {
		t.Fatalf("index after resume = %d, want 1", got)
	}
	// Advancing past the end removes the interface; the chatbox is
	// gone, so a further resume is refused.
	if !m.HandleResume(chatboxUID, 0) {
		t.Error("resume advancing past the end refused")
	}
	if m.HandleResume(chatboxUID, 0) {
		t.Error("resume handled after the chain ended")
	}
	_ = s
}

func TestDialogueManagerHandleResumeSelfAdvancing(t *testing.T) {
	p, _ := newDialoguePlayer()
	m := p.GetDialogueManager()
	advanced := false
	self := NewActionDialogue(0, DialogueActionFunc(func() { advanced = true })).WithSelfAdvancing(true)
	m.StartDialogues(NewDialogueChainBuilder().
		Add(self).
		Add(NewPlayerDialogue(1, "Next.")))
	if !advanced {
		t.Fatal("action did not run on start")
	}
	// The chatbox still shows whatever was up before (nothing here,
	// so mount one first via a direct sender call to satisfy
	// CanContinue's chatbox check through the manager's own path:
	// instead start a visible chain first in a second scenario).
	// Here no chatbox is mounted, so the resume is refused before
	// the self-advancing branch is reached.
	if m.HandleResume((217<<16)|5, 0) {
		t.Error("resume handled with no chatbox mounted")
	}

	// Second scenario: a visible entry, then a self-advancing action
	// reached by advance. The previous chatbox stays mounted (the
	// action sends no frames), so CanContinue passes — and the
	// self-advancing branch swallows the resume.
	p2, _ := newDialoguePlayer()
	m2 := p2.GetDialogueManager()
	m2.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "One.")).
		Add(NewActionDialogue(1, DialogueActionFunc(func() {})).WithSelfAdvancing(true)).
		Add(NewPlayerDialogue(2, "Three.")))
	m2.Advance()
	if got := m2.GetCurrent().GetIndex(); got != 1 {
		t.Fatalf("index = %d, want 1 (the action)", got)
	}
	if !m2.HandleResume((217<<16)|5, 0) {
		t.Error("self-advancing resume refused")
	}
	if got := m2.GetCurrent().GetIndex(); got != 1 {
		t.Fatalf("index after self-advancing resume = %d, want 1 (no advance)", got)
	}

	// A non-self-advancing action advances on resume.
	p3, _ := newDialoguePlayer()
	m3 := p3.GetDialogueManager()
	m3.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "One.")).
		Add(NewActionDialogue(1, DialogueActionFunc(func() {}))).
		Add(NewPlayerDialogue(2, "Three.")))
	m3.Advance()
	if !m3.HandleResume((217<<16)|5, 0) {
		t.Error("plain-action resume refused")
	}
	if got := m3.GetCurrent().GetIndex(); got != 2 {
		t.Fatalf("index after plain-action resume = %d, want 2", got)
	}
}

func TestDialogueManagerHandleResumeNoSender(t *testing.T) {
	p := NewPlayer("nosession", Location{X: 3089, Y: 3524, Z: 0})
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Hi.")))
	if m.CanContinue((217 << 16) | 5) {
		t.Error("CanContinue true with no sender")
	}
	if m.HandleResume((217<<16)|5, 0) {
		t.Error("HandleResume true with no sender")
	}
}

type recordingOptionAction struct {
	received []DialogueOption
}

func (a *recordingOptionAction) ExecuteOption(option DialogueOption) {
	a.received = append(a.received, option)
}

func TestDialogueManagerHandleResumeOptions(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	action := &recordingOptionAction{}
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewOptionDialogue(0, action, "One", "Two", "Three")))

	// Wrong widget id, zero child, and out-of-range child all refuse.
	for _, tc := range []struct{ widget, child int }{
		{(219 << 16) | 2, 1},
		{OptionsWidgetID, 0},
		{OptionsWidgetID, 4},
		{OptionsWidgetID, -1},
	} {
		if m.HandleResume(tc.widget, tc.child) {
			t.Errorf("HandleResume(%d, %d) handled, want refused", tc.widget, tc.child)
		}
	}
	if len(action.received) != 0 {
		t.Fatalf("action received %v from refused resumes", action.received)
	}
	// A valid resume fires the action with the one-based child index.
	if !m.HandleResume(OptionsWidgetID, 2) {
		t.Error("valid option resume refused")
	}
	if !reflect.DeepEqual(action.received, []DialogueOption{2}) {
		t.Errorf("action received %v, want [2] (one-based child index)", action.received)
	}
	// The option branch does not check the chatbox: it fires even
	// after the chatbox is gone (removal closes it in the fake).
	s.SendInterfaceRemoval()
	if !m.HandleResume(OptionsWidgetID, 1) {
		t.Error("option resume refused after chatbox close")
	}
	if !reflect.DeepEqual(action.received, []DialogueOption{2, 1}) {
		t.Errorf("action received %v, want [2 1]", action.received)
	}
}

func TestDialogueManagerHandleOption(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	action := &recordingOptionAction{}
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewOptionDialogue(0, action, "One", "Two")))
	// A direct call passes the enum value through.
	m.HandleOption(DialogueOptionFirst)
	if !reflect.DeepEqual(action.received, []DialogueOption{DialogueOptionFirst}) {
		t.Errorf("action received %v, want [0]", action.received)
	}

	// A non-option current entry removes the interface.
	p2, s2 := newDialoguePlayer()
	m2 := p2.GetDialogueManager()
	m2.StartDialogues(NewDialogueChainBuilder().Add(NewStatementDialogue(0, "Text.")))
	m2.HandleOption(DialogueOptionFirst)
	if s2.removals != 1 {
		t.Errorf("removals = %d, want 1 for option on a non-option entry", s2.removals)
	}

	// Inactive: removal too.
	p3, s3 := newDialoguePlayer()
	p3.GetDialogueManager().HandleOption(DialogueOptionFirst)
	if s3.removals != 1 {
		t.Errorf("removals = %d, want 1 for option with no chain", s3.removals)
	}
	_ = s
}

func TestDialogueManagerStaticDialogue(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	if !m.StartStaticDialogue(DialogueIdentifierTest) {
		t.Fatal("StartStaticDialogue(TEST) returned false")
	}
	if !m.IsActive() {
		t.Fatal("static dialogue inactive after start")
	}
	if got := m.GetCurrent().GetIndex(); got != 0 {
		t.Fatalf("static start index = %d, want 0", got)
	}
	m.Advance()
	m.Advance()
	current := m.GetCurrent()
	npc, ok := current.(*NpcDialogue)
	if !ok {
		t.Fatalf("third static entry is %T, want *NpcDialogue", current)
	}
	if npc.GetNpcID() != 6797 || npc.GetText() != "okay great." {
		t.Errorf("third static entry = npc %d %q, want 6797 %q", npc.GetNpcID(), npc.GetText(), "okay great.")
	}

	// A miss returns false and leaves the chain untouched.
	callsBefore := len(s.calls)
	if m.StartStaticDialogue(999) {
		t.Error("StartStaticDialogue(999) returned true")
	}
	if m.GetCurrent() != Dialogue(current) {
		t.Error("missed static start disturbed the chain")
	}
	if len(s.calls) != callsBefore {
		t.Error("missed static start sent frames")
	}
}

// dynamicTestBuilder is a DynamicDialogueBuilder for tests: Build adds
// one entry naming the player.
type dynamicTestBuilder struct {
	*DialogueChainBuilder
	builtFor *Player
	builds   int
}

func (b *dynamicTestBuilder) Build(p *Player) {
	b.builds++
	b.builtFor = p
	b.Add(NewPlayerDialogue(0, "Hello, "+p.GetDisplayName()+"."))
}

func TestDialogueManagerDynamicBuilder(t *testing.T) {
	p, _ := newDialoguePlayer()
	m := p.GetDialogueManager()
	builder := &dynamicTestBuilder{DialogueChainBuilder: NewDialogueChainBuilder()}
	m.StartDialog(builder, 0)
	if builder.builds != 1 {
		t.Fatalf("Build calls = %d, want 1", builder.builds)
	}
	if builder.builtFor != p {
		t.Error("Build received the wrong player")
	}
	current, ok := m.GetCurrent().(*PlayerDialogue)
	if !ok {
		t.Fatalf("current entry is %T, want *PlayerDialogue", m.GetCurrent())
	}
	if current.GetText() != "Hello, testplayer." {
		t.Errorf("dynamic text = %q", current.GetText())
	}
	// Starting again rebuilds (TS calls build on every startDialog).
	m.StartDialog(builder, 0)
	if builder.builds != 2 {
		t.Fatalf("Build calls = %d, want 2", builder.builds)
	}
}

func TestDialogueManagerSparseIndices(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	builder := NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "Zero.")).
		Add(NewPlayerDialogue(2, "Two."))
	m.StartDialogues(builder)
	m.Advance() // index 1 is missing: removal, inactive
	if m.IsActive() {
		t.Error("manager active after advancing into a gap")
	}
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1", s.removals)
	}
	// Starting directly at index 2 presents that entry.
	m.StartDialog(builder, 2)
	if got := m.GetCurrent().GetIndex(); got != 2 {
		t.Fatalf("current index = %d, want 2", got)
	}
}

func TestDialogueManagerCopiesBuilderEntries(t *testing.T) {
	p, s := newDialoguePlayer()
	m := p.GetDialogueManager()
	builder := NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Zero."))
	m.StartDialogues(builder)
	// Mutating the builder afterwards must not affect the running chain.
	builder.Add(NewPlayerDialogue(1, "Late."))
	m.Advance()
	if m.IsActive() {
		t.Error("manager picked up a builder entry added after start")
	}
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1", s.removals)
	}
}

func TestDialogueManagerStartReplacesChain(t *testing.T) {
	p, _ := newDialoguePlayer()
	m := p.GetDialogueManager()
	owner1 := &struct{ name string }{"one"}
	owner2 := &struct{ name string }{"two"}
	m.StartDialogues(NewDialogueChainBuilder().
		Add(NewPlayerDialogue(0, "A.")).
		Add(NewPlayerDialogue(1, "B.")), owner1)
	m.Advance()
	m.StartDialogues(NewDialogueChainBuilder().Add(NewStatementDialogue(0, "C.")), owner2)
	if got := m.GetCurrent().GetIndex(); got != 0 {
		t.Fatalf("index after restart = %d, want 0", got)
	}
	if _, ok := m.GetCurrent().(*StatementDialogue); !ok {
		t.Fatalf("current entry is %T after restart, want *StatementDialogue", m.GetCurrent())
	}
	if m.GetOwner() != any(owner2) {
		t.Error("owner not replaced by the new chain")
	}
	m.Advance() // past the single-entry chain: owner drops with activity
	if m.GetOwner() != nil {
		t.Error("GetOwner non-nil after the chain ended")
	}
}

func TestDialogueManagerUnsignedWidgetShift(t *testing.T) {
	// A widget id whose signed int32 form is negative must still
	// match its group: TS uses >>> (DialogueManager.ts:67).
	p, _ := newDialoguePlayer()
	m := p.GetDialogueManager()
	m.StartDialogues(NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Hi.")))
	// Group 217 as a signed 32-bit uid with the high bit set via a
	// large child: (217<<16)|0x8000 as int32 is positive here, so
	// instead use a group above 32767: group 40000's uid is
	// negative as int32. Mount group 40000 through the sender and
	// check CanContinue with the negative uid.
	uid := (40000 << 16) | 5
	negative := int(int32(uint32(uid)))
	if negative >= 0 {
		t.Fatalf("test setup: uid %d is not negative as int32", negative)
	}
	p.Session.(DialogueSender).SendChatboxInterface(40000, false)
	if !m.CanContinue(negative) {
		t.Error("CanContinue false for a negative uid of the mounted group")
	}
}
