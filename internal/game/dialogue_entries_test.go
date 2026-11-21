package game

import (
	"reflect"
	"strings"
	"testing"
)

func assertCalls(t *testing.T, s *dialogueTestSession, want []string) {
	t.Helper()
	if !reflect.DeepEqual(s.calls, want) {
		t.Fatalf("calls = %v\nwant = %v", s.calls, want)
	}
}

func TestNpcDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	p.GetDialogueManager().SetNpcNameResolver(func(id int) string {
		if id == 6797 {
			return "King_Roald"
		}
		return "Someone"
	})
	NewNpcDialogue(0, 6797, "Hello there.").Send(p)
	assertCalls(t, s, []string{
		"chatbox(231,false)",
		"npcHead(6797,15138818)", // (231<<16)|2
		"anim(15138818,589)",     // CALM
		`string(15138820,"King Roald")`,
		`string(15138822,"Hello there.")`,
		"script(600,[1 1 16 15138822])",
		`string(15138821,"Click here to continue")`,
		"flags(15138821,-1,-1,1)",
	})
}

func TestNpcDialogueNameResolution(t *testing.T) {
	// No resolver: the DEFAULT definition name, "null".
	p, s := newDialoguePlayer()
	NewNpcDialogue(0, 1234, "Hi.").Send(p)
	if !strings.Contains(strings.Join(s.calls, "\n"), `string(15138820,"null")`) {
		t.Fatalf("unwired name not %q: %v", "null", s.calls)
	}

	// A resolver returning an empty name yields "" (TS `|| ""`).
	p2, s2 := newDialoguePlayer()
	p2.GetDialogueManager().SetNpcNameResolver(func(int) string { return "" })
	NewNpcDialogue(0, 1234, "Hi.").Send(p2)
	if !strings.Contains(strings.Join(s2.calls, "\n"), `string(15138820,"")`) {
		t.Fatalf("empty resolved name not sent as %q: %v", "", s2.calls)
	}

	// An explicit speaker name wins, including an explicit "".
	p3, s3 := newDialoguePlayer()
	p3.GetDialogueManager().SetNpcNameResolver(func(int) string { return "Resolved" })
	NewNpcDialogue(0, 1234, "Hi.").WithSpeakerName("???").Send(p3)
	if !strings.Contains(strings.Join(s3.calls, "\n"), `string(15138820,"???")`) {
		t.Fatalf("speaker override not used: %v", s3.calls)
	}
	p4, s4 := newDialoguePlayer()
	p4.GetDialogueManager().SetNpcNameResolver(func(int) string { return "Resolved" })
	NewNpcDialogue(0, 1234, "Hi.").WithSpeakerName("").Send(p4)
	if !strings.Contains(strings.Join(s4.calls, "\n"), `string(15138820,"")`) {
		t.Fatalf("explicit empty speaker name did not win: %v", s4.calls)
	}
}

func TestNpcDialogueExpressionAndGroup(t *testing.T) {
	p, s := newDialoguePlayer()
	NewNpcDialogue(0, 1, "Ha!").
		WithExpression(DialogueExpressionEvilLaughShort).
		WithGroupID(232).
		Send(p)
	assertCalls(t, s, []string{
		"chatbox(232,false)",
		"npcHead(1,15204354)", // (232<<16)|2
		"anim(15204354,609)",
		`string(15204356,"null")`,
		`string(15204358,"Ha!")`,
		"script(600,[1 1 16 15204358])",
		`string(15204357,"Click here to continue")`,
		"flags(15204357,-1,-1,1)",
	})
}

func TestNpcDialogueSendHeld(t *testing.T) {
	p, s := newDialoguePlayer()
	NewNpcDialogue(0, 1, "Watch.").SendHeld(p)
	want := []string{
		"chatbox(231,false)",
		"npcHead(1,15138818)",
		"anim(15138818,589)",
		`string(15138820,"null")`,
		`string(15138822,"Watch.")`,
		"script(600,[1 1 16 15138822])",
		`string(15138821,"Click here to continue")`,
		"flags(15138821,-1,-1,1)",
		`string(15138821,"")`,
		"flags(15138821,-1,-1,0)",
	}
	assertCalls(t, s, want)
}

func TestNpcDialogueGetters(t *testing.T) {
	d := NewNpcDialogue(3, 42, "Text.")
	if d.GetIndex() != 3 || d.GetNpcID() != 42 || d.GetText() != "Text." {
		t.Errorf("getters = (%d, %d, %q)", d.GetIndex(), d.GetNpcID(), d.GetText())
	}
}

func TestPlayerDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	p.DisplayName = "Display Name"
	NewPlayerDialogue(0, "I say.").WithExpression(DialogueExpressionHappy).Send(p)
	assertCalls(t, s, []string{
		"chatbox(217,false)",
		"playerHead(14221314)", // (217<<16)|2
		"anim(14221314,588)",   // HAPPY
		`string(14221316,"Display Name")`,
		`string(14221318,"I say.")`,
		"script(600,[1 1 16 14221318])",
		`string(14221317,"Click here to continue")`,
		"flags(14221317,-1,-1,1)",
	})

	// Without a display name, the username is shown.
	p2, s2 := newDialoguePlayer()
	NewPlayerDialogue(0, "Hi.").Send(p2)
	if !strings.Contains(strings.Join(s2.calls, "\n"), `string(14221316,"testplayer")`) {
		t.Fatalf("username not used as speaker name: %v", s2.calls)
	}
	// Default expression is CALM.
	if !strings.Contains(strings.Join(s2.calls, "\n"), "anim(14221314,589)") {
		t.Fatalf("default expression not CALM: %v", s2.calls)
	}
}

func TestStatementLineHeight(t *testing.T) {
	cases := []struct {
		text string
		want int
	}{
		{"", 0},
		{"One line.", 0},
		{"One<br>Two", 31},
		{"One<br>Two<br>Three", 24},
		{"One<br>Two<br>Three<br>Four", 17},
		{"1<br>2<br>3<br>4<br>5", 0},
		{"a<br>", 31},          // trailing break: ["a", ""] is 2 paragraphs
		{"<br>", 31},           // ["", ""] is 2 paragraphs
		{"a<BR>b", 31},         // case-insensitive
		{"a<br/>b", 31},        // self-closed
		{"a<br />b", 31},       // spaced self-close
		{"a<br  />b<br>c", 24}, // mixed forms
	}
	for _, tc := range cases {
		if got := StatementLineHeight(tc.text); got != tc.want {
			t.Errorf("StatementLineHeight(%q) = %d, want %d", tc.text, got, tc.want)
		}
	}
}

func TestStatementDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	NewStatementDialogue(0, "First<br>Second").Send(p)
	assertCalls(t, s, []string{
		`typed(115,"First Second","")`, // chat line FIRST, <br> -> space
		"chatbox(229,false)",
		`string(15007747,"First<br>Second")`, // (229<<16)|3, raw text
		"script(600,[1 1 31 15007747])",      // 2 lines -> 31
		`string(15007748,"Click here to continue")`,
		"flags(15007748,-1,-1,1)",
	})
}

func TestStatementDialogueNoContinue(t *testing.T) {
	p, s := newDialoguePlayer()
	SendStatementDialogue(p, "Wait for it.", false)
	assertCalls(t, s, []string{
		`typed(115,"Wait for it.","")`,
		"chatbox(229,false)",
		`string(15007747,"Wait for it.")`,
		"script(600,[1 1 0 15007747])",
		`string(15007748,"")`,
		"flags(15007748,-1,-1,0)",
	})
}

func TestSendBoxChatLine(t *testing.T) {
	// Empty text writes nothing.
	p, s := newDialoguePlayer()
	sendBoxChatLine(p, "")
	if len(s.typed) != 0 {
		t.Errorf("empty box text wrote %v", s.typed)
	}
	// Only the exact "<br>" becomes a space; "<br/>" is kept.
	sendBoxChatLine(p, "a<br>b<br/>c<BR>d")
	if len(s.typed) != 1 {
		t.Fatalf("typed = %v, want 1 line", s.typed)
	}
	if got := s.typed[0]; got != [3]any{115, "a b<br/>c<BR>d", ""} {
		t.Errorf("typed[0] = %v", got)
	}
	// Long lines stop at 200 characters.
	long := strings.Repeat("x", 250)
	sendBoxChatLine(p, long)
	if got := s.typed[1][1].(string); len(got) != 200 {
		t.Errorf("truncated line length = %d, want 200", len(got))
	}
}

func TestTruncateUTF16Units(t *testing.T) {
	if got := truncateUTF16Units("abcdef", 3); got != "abc" {
		t.Errorf("ascii truncation = %q", got)
	}
	if got := truncateUTF16Units("ab", 10); got != "ab" {
		t.Errorf("short text = %q", got)
	}
	// An astral rune costs 2 units and is dropped whole at the limit.
	if got := truncateUTF16Units("a\U0001F600b", 2); got != "a" {
		t.Errorf("astral at limit = %q, want %q", got, "a")
	}
	if got := truncateUTF16Units("a\U0001F600b", 3); got != "a\U0001F600" {
		t.Errorf("astral within limit = %q", got)
	}
}

func TestItemStatementDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	d := NewItemStatementDialogue(0, 995, "You get some coins.")
	if d.GetItemID() != 995 || d.GetText() != "You get some coins." {
		t.Errorf("getters = (%d, %q)", d.GetItemID(), d.GetText())
	}
	d.Send(p)
	assertCalls(t, s, []string{
		`typed(115,"You get some coins.","")`,
		"chatbox(193,false)",
		`script(2868,["Click here to continue"])`,
		"flags(12648448,0,1,1)", // (193<<16)|0
		"item(12648449,995,400)",
		`string(12648450,"You get some coins.")`,
	})
}

func TestDoubleItemStatementDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	d := NewDoubleItemStatementDialogue(0, 1351, 590, "You get an axe and a tinderbox.")
	first, second := d.GetItemIDs()
	if first != 1351 || second != 590 {
		t.Errorf("GetItemIDs = (%d, %d)", first, second)
	}
	d.Send(p)
	assertCalls(t, s, []string{
		`typed(115,"You get an axe and a tinderbox.","")`,
		"chatbox(11,false)",
		"flags(720900,-1,-1,1)", // (11<<16)|4
		`string(720900,"Click here to continue")`,
		"item(720897,1351,400)",
		"item(720899,590,400)",
		`string(720898,"You get an axe and a tinderbox.")`,
	})
}

func TestOptionDialogueSend(t *testing.T) {
	p, s := newDialoguePlayer()
	action := &recordingOptionAction{}
	d := NewOptionDialogue(0, action, "Yes", "No")
	if d.GetTitle() != "" {
		t.Errorf("unset title = %q, want empty", d.GetTitle())
	}
	d.Send(p)
	assertCalls(t, s, []string{
		"ifaceScript(2379)",
		"chatbox(219,true)",
		`script(58,["Select an Option" "Yes|No"])`,
		"flags(14352385,1,2,1)", // OptionsWidgetID, 1..2
	})
	if OptionsWidgetID != (219<<16)|1 {
		t.Errorf("OptionsWidgetID = %d, want %d", OptionsWidgetID, (219<<16)|1)
	}

	// A set title replaces the default; SetTitle chains.
	p2, s2 := newDialoguePlayer()
	NewOptionDialogue(0, action, "A").SetTitle("Pick one").Send(p2)
	if !strings.Contains(strings.Join(s2.calls, "\n"), `"Pick one" "A"`) {
		t.Fatalf("set title not sent: %v", s2.calls)
	}
	// An explicitly empty title also defaults (TS `title || ...`).
	p3, s3 := newDialoguePlayer()
	NewOptionDialogue(0, action, "A").SetTitle("").Send(p3)
	if !strings.Contains(strings.Join(s3.calls, "\n"), `"Select an Option"`) {
		t.Fatalf("empty title did not default: %v", s3.calls)
	}
	// Options are returned live and in order.
	if got := d.GetOptions(); !reflect.DeepEqual(got, []string{"Yes", "No"}) {
		t.Errorf("GetOptions = %v", got)
	}
}

func TestOptionDialogueExecute(t *testing.T) {
	action := &recordingOptionAction{}
	d := NewOptionDialogue(0, action, "A", "B")
	d.Execute(DialogueOptionThird)
	if !reflect.DeepEqual(action.received, []DialogueOption{DialogueOptionThird}) {
		t.Errorf("received = %v", action.received)
	}
	// A nil action is a no-op, not a panic.
	NewOptionDialogue(0, nil, "A").Execute(DialogueOptionFirst)
}

func TestDialogueOptionValues(t *testing.T) {
	want := []DialogueOption{0, 1, 2, 3, 4}
	got := []DialogueOption{
		DialogueOptionFirst, DialogueOptionSecond, DialogueOptionThird,
		DialogueOptionFourth, DialogueOptionFifth,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("option values = %v, want %v", got, want)
	}
}

func TestActionDialogue(t *testing.T) {
	ran := 0
	d := NewActionDialogue(0, DialogueActionFunc(func() { ran++ }))
	if d.IsSelfAdvancing() {
		t.Error("default self-advancing is true")
	}
	d.Send(nil) // actions ignore the player entirely
	if ran != 1 {
		t.Errorf("action ran %d times, want 1", ran)
	}
	if !d.WithSelfAdvancing(true).IsSelfAdvancing() {
		t.Error("WithSelfAdvancing did not set the flag")
	}
	// A nil action is a no-op (documented deviation from TS, which throws).
	NewActionDialogue(0, nil).Send(nil)
}

func TestEndDialogue(t *testing.T) {
	p, s := newDialoguePlayer()
	NewEndDialogue(0).Send(p)
	assertCalls(t, s, []string{"removal"})
}

func TestDialogueEntriesDropWithoutSender(t *testing.T) {
	// Nil session: every entry drops silently.
	p := NewPlayer("nosession", Location{X: 3089, Y: 3524, Z: 0})
	entries := []Dialogue{
		NewNpcDialogue(0, 1, "Hi."),
		NewPlayerDialogue(0, "Hi."),
		NewStatementDialogue(0, "Hi."),
		NewItemStatementDialogue(0, 995, "Hi."),
		NewDoubleItemStatementDialogue(0, 1, 2, "Hi."),
		NewOptionDialogue(0, nil, "A"),
		NewEndDialogue(0),
	}
	for _, e := range entries {
		e.Send(p) // must not panic
	}
	// A session without the DialogueSender extension drops too.
	p2 := NewPlayer("plain", Location{X: 3089, Y: 3524, Z: 0})
	p2.Session = newFakeSession()
	for _, e := range entries {
		e.Send(p2)
	}
	// Nil player: no panic anywhere, including the static sends.
	for _, e := range entries {
		e.Send(nil)
	}
	SendPlayerDialogue(nil, "Hi.", DialogueExpressionCalm)
	SendStatementDialogue(nil, "Hi.", true)
	SendItemStatementDialogue(nil, 995, "Hi.")
	SendDoubleItemStatementDialogue(nil, 1, 2, "Hi.")
	SendOptionDialogue(nil, "Title", []string{"A"})
	NewNpcDialogue(0, 1, "Hi.").SendHeld(nil)
}

func TestDialogueExpressionValues(t *testing.T) {
	want := map[DialogueExpression]int{
		DialogueExpressionHappy:               588,
		DialogueExpressionCalm:                589,
		DialogueExpressionCalmContinued:       590,
		DialogueExpressionDefault:             591,
		DialogueExpressionEvil:                592,
		DialogueExpressionEvilContinued:       593,
		DialogueExpressionDelightedEvil:       594,
		DialogueExpressionAnnoyed:             595,
		DialogueExpressionDistressed:          596,
		DialogueExpressionDistressedContinued: 597,
		DialogueExpressionDisorientedLeft:     600,
		DialogueExpressionDisorientedRight:    601,
		DialogueExpressionUninterested:        602,
		DialogueExpressionSleepy:              603,
		DialogueExpressionPlainEvil:           604,
		DialogueExpressionLaughing:            605,
		DialogueExpressionLaughing2:           608,
		DialogueExpressionLongerLaughing:      606,
		DialogueExpressionLongerLaughing2:     607,
		DialogueExpressionEvilLaughShort:      609,
		DialogueExpressionSlightlySad:         610,
		DialogueExpressionSad:                 599,
		DialogueExpressionVerySad:             611,
		DialogueExpressionOther:               612,
		DialogueExpressionNearTears:           598,
		DialogueExpressionNearTears2:          613,
		DialogueExpressionAngry1:              614,
		DialogueExpressionAngry2:              615,
		DialogueExpressionAngry3:              616,
		DialogueExpressionAngry4:              617,
	}
	if len(want) != 30 {
		t.Fatalf("test table has %d expressions, want 30", len(want))
	}
	for expr, id := range want {
		if expr.ID() != id {
			t.Errorf("expression %d: ID() = %d, want %d", int(expr), expr.ID(), id)
		}
	}
	// An arbitrary id converts directly (TS `number | DialogueExpression`).
	if DialogueExpression(1234).ID() != 1234 {
		t.Error("arbitrary expression id did not round-trip")
	}
}
