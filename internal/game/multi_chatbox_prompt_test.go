package game

import (
	"strings"
	"testing"
	"time"
)

func TestMultiChatboxPromptShowFrames(t *testing.T) {
	p, s := newDialoguePlayer()
	r := NewMultiChatboxPrompt(nil)
	picked := -1
	options := []MenuOption{
		{Label: "First", Select: func() { picked = 0 }},
		{Label: "Second", Select: func() { picked = 1 }},
	}
	if !r.ShowPrompt("NpcDialogues", p, "Select an option", options) {
		t.Fatal("ShowPrompt reported false for a valid prompt")
	}
	// Frame order (MultiChatboxPrompt.ts:92-100): script 2379,
	// chatbox 219 unclamped, client script 58 with title +
	// "|"-joined options, flags range over the option slots.
	want := []string{
		"ifaceScript(2379)",
		"chatbox(219,true)",
		`script(58,["Select an option" "First|Second"])`,
		"flags(14352385,1,2,1)",
	}
	if len(s.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", s.calls, want)
	}
	for i, call := range want {
		if s.calls[i] != call {
			t.Fatalf("call[%d] = %q, want %q (all: %v)", i, s.calls[i], call, s.calls)
		}
	}
	if s.chatboxID != 219 {
		t.Fatalf("chatboxID = %d, want 219", s.chatboxID)
	}
	_ = picked
}

func TestMultiChatboxPromptShowResetsManager(t *testing.T) {
	p, _ := newDialoguePlayer()
	p.GetDialogueManager().StartDialogues(NewDialogueChainBuilder().Add(NewStatementDialogue(0, "hi")))
	if !p.GetDialogueManager().IsActive() {
		t.Fatal("manager should be active before the prompt")
	}
	r := NewMultiChatboxPrompt(nil)
	if !r.ShowPrompt("NpcDialogues", p, "Pick", []MenuOption{{Label: "A", Select: func() {}}, {Label: "B", Select: func() {}}}) {
		t.Fatal("ShowPrompt reported false")
	}
	if p.GetDialogueManager().IsActive() {
		t.Fatal("manager still active after ShowPrompt reset it")
	}
}

func TestMultiChatboxPromptValidation(t *testing.T) {
	newPlayer := func() (*Player, *dialogueTestSession) { return newDialoguePlayer() }
	valid := func() []MenuOption {
		return []MenuOption{{Label: "A", Select: func() {}}, {Label: "B", Select: func() {}}}
	}
	t.Run("nil player", func(t *testing.T) {
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", nil, "T", valid()) {
			t.Fatal("nil player accepted")
		}
	})
	t.Run("no sender", func(t *testing.T) {
		p := NewPlayer("nosession", Location{X: 3089, Y: 3524, Z: 0})
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "T", valid()) {
			t.Fatal("player without a DialogueSender accepted")
		}
	})
	t.Run("blank title", func(t *testing.T) {
		p, s := newPlayer()
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "   ", valid()) {
			t.Fatal("blank title accepted")
		}
		if len(s.calls) != 0 {
			t.Fatalf("frames sent for a rejected prompt: %v", s.calls)
		}
	})
	t.Run("no options", func(t *testing.T) {
		p, _ := newPlayer()
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "T", nil) {
			t.Fatal("empty option list accepted")
		}
	})
	t.Run("blank label", func(t *testing.T) {
		p, _ := newPlayer()
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "T", []MenuOption{{Label: " ", Select: func() {}}, {Label: "B", Select: func() {}}}) {
			t.Fatal("blank label accepted")
		}
	})
	t.Run("nil callback", func(t *testing.T) {
		p, _ := newPlayer()
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "T", []MenuOption{{Label: "A"}, {Label: "B", Select: func() {}}}) {
			t.Fatal("nil callback accepted")
		}
	})
	t.Run("six options", func(t *testing.T) {
		p, _ := newPlayer()
		opts := make([]MenuOption, 6)
		for i := range opts {
			opts[i] = MenuOption{Label: "O", Select: func() {}}
		}
		if NewMultiChatboxPrompt(nil).ShowPrompt("x", p, "T", opts) {
			t.Fatal("six options accepted")
		}
	})
	t.Run("single option padded with Cancel", func(t *testing.T) {
		p, s := newPlayer()
		r := NewMultiChatboxPrompt(nil)
		if !r.ShowPrompt("x", p, "T", []MenuOption{{Label: "Only", Select: func() {}}}) {
			t.Fatal("single option rejected")
		}
		if !strings.Contains(strings.Join(s.calls, " "), `"Only|Cancel"`) {
			t.Fatalf("script 58 did not pad with Cancel: %v", s.calls)
		}
		title, labels, ok := r.GetPending(p)
		if !ok || title != "T" || len(labels) != 2 || labels[1] != "Cancel" {
			t.Fatalf("GetPending = (%q, %v, %v), want (T, [Only Cancel], true)", title, labels, ok)
		}
		// Picking Cancel (option 2) consumes the click and runs
		// the no-op without complaint.
		if !r.HandleInterfaceActionClick(p, OptionsWidgetID, 2, 219) {
			t.Fatal("Cancel pick not consumed")
		}
	})
}

func TestMultiChatboxPromptSelect(t *testing.T) {
	p, s := newDialoguePlayer()
	r := NewMultiChatboxPrompt(nil)
	var picked []int
	options := []MenuOption{
		{Label: "A", Select: func() { picked = append(picked, 0) }},
		{Label: "B", Select: func() { picked = append(picked, 1) }},
		{Label: "C", Select: func() { picked = append(picked, 2) }},
	}
	if !r.ShowPrompt("NpcDialogues", p, "Pick", options) {
		t.Fatal("ShowPrompt reported false")
	}
	// Option 2 is picked with childIndex 2 (1-based).
	if !r.HandleInterfaceActionClick(p, OptionsWidgetID, 2, 219) {
		t.Fatal("valid pick not consumed")
	}
	if len(picked) != 1 || picked[0] != 1 {
		t.Fatalf("picked = %v, want [1]", picked)
	}
	// The pick closes the interface.
	if s.removals != 1 {
		t.Fatalf("removals = %d, want 1", s.removals)
	}
	// The prompt is gone: a second pick finds nothing.
	if r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
		t.Fatal("stale pick consumed after the prompt was answered")
	}
	if len(picked) != 1 {
		t.Fatalf("picked = %v, want exactly one pick", picked)
	}
}

func TestMultiChatboxPromptSelectOutOfRange(t *testing.T) {
	p, s := newDialoguePlayer()
	r := NewMultiChatboxPrompt(nil)
	called := false
	if !r.ShowPrompt("x", p, "T", []MenuOption{{Label: "A", Select: func() { called = true }}, {Label: "B", Select: func() { called = true }}}) {
		t.Fatal("ShowPrompt reported false")
	}
	for _, action := range []int{0, 3, -1} {
		if r.HandleInterfaceActionClick(p, OptionsWidgetID, action, 219) {
			t.Fatalf("out-of-range action %d consumed", action)
		}
	}
	if called {
		t.Fatal("callback ran for an out-of-range pick")
	}
	if s.removals != 0 {
		t.Fatalf("removals = %d, want 0: an out-of-range pick must not close the prompt", s.removals)
	}
	// The prompt survives: a valid pick still works.
	if !r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
		t.Fatal("valid pick after out-of-range picks not consumed")
	}
}

func TestMultiChatboxPromptWrongWidget(t *testing.T) {
	p, _ := newDialoguePlayer()
	r := NewMultiChatboxPrompt(nil)
	called := false
	if !r.ShowPrompt("x", p, "T", []MenuOption{{Label: "A", Select: func() { called = true }}, {Label: "B", Select: func() {}}}) {
		t.Fatal("ShowPrompt reported false")
	}
	// Neither the options widget nor group 219: not this prompt.
	if r.HandleInterfaceActionClick(p, (231<<16)|5, 1, 231) {
		t.Fatal("click on another widget consumed")
	}
	if called {
		t.Fatal("callback ran for a foreign click")
	}
	// Group 219 alone (buttonId elsewhere) still routes: TS
	// accepts buttonId == OPTIONS_WIDGET_ID OR groupId == 219.
	if !r.HandleInterfaceActionClick(p, 12345, 1, 219) {
		t.Fatal("group-219 click not consumed")
	}
}

func TestMultiChatboxPromptOwnership(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	newRegistry := func() *MultiChatboxPrompt {
		r := NewMultiChatboxPrompt(nil)
		r.now = func() time.Time { return now }
		return r
	}
	show := func(r *MultiChatboxPrompt, p *Player) {
		if !r.ShowPrompt("x", p, "T", []MenuOption{{Label: "A", Select: func() {}}, {Label: "B", Select: func() {}}}) {
			t.Fatal("ShowPrompt reported false")
		}
	}
	t.Run("expired", func(t *testing.T) {
		p, _ := newDialoguePlayer()
		r := newRegistry()
		show(r, p)
		now = now.Add(PromptTTL + time.Second)
		if r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
			t.Fatal("expired prompt consumed the click")
		}
		if _, _, ok := r.GetPending(p); ok {
			t.Fatal("expired prompt still pending")
		}
	})
	t.Run("manager active supersedes", func(t *testing.T) {
		p, _ := newDialoguePlayer()
		r := newRegistry()
		show(r, p)
		p.GetDialogueManager().StartDialogues(NewDialogueChainBuilder().Add(NewStatementDialogue(0, "hi")))
		if r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
			t.Fatal("prompt consumed the click after a native dialogue started")
		}
	})
	t.Run("chatbox moved supersedes", func(t *testing.T) {
		p, s := newDialoguePlayer()
		r := newRegistry()
		show(r, p)
		s.chatboxID = 231 // another group mounted since
		if r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
			t.Fatal("prompt consumed the click after the chatbox moved")
		}
	})
	t.Run("forget", func(t *testing.T) {
		p, _ := newDialoguePlayer()
		r := newRegistry()
		show(r, p)
		r.Forget(p)
		if _, _, ok := r.GetPending(p); ok {
			t.Fatal("prompt still pending after Forget")
		}
	})
}

func TestMultiChatboxPromptCallbackPanicRecovered(t *testing.T) {
	p, _ := newDialoguePlayer()
	var warnings []string
	r := NewMultiChatboxPrompt(func(format string, args ...any) {
		warnings = append(warnings, format)
	})
	if !r.ShowPrompt("BoomPlugin", p, "T", []MenuOption{{Label: "A", Select: func() { panic("boom") }}, {Label: "B", Select: func() {}}}) {
		t.Fatal("ShowPrompt reported false")
	}
	if !r.HandleInterfaceActionClick(p, OptionsWidgetID, 1, 219) {
		t.Fatal("panicking callback's click not consumed")
	}
	if len(warnings) == 0 {
		t.Fatal("callback panic produced no log line")
	}
	if _, _, ok := r.GetPending(p); ok {
		t.Fatal("prompt still pending after its callback panicked")
	}
}

func TestMultiChatboxPromptPrunesExpiredOnShow(t *testing.T) {
	now := time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC)
	r := NewMultiChatboxPrompt(nil)
	r.now = func() time.Time { return now }
	p1, _ := newDialoguePlayer()
	p2, _ := newDialoguePlayer()
	opts := []MenuOption{{Label: "A", Select: func() {}}, {Label: "B", Select: func() {}}}
	if !r.ShowPrompt("x", p1, "T", opts) {
		t.Fatal("ShowPrompt reported false")
	}
	now = now.Add(PromptTTL + time.Second)
	if !r.ShowPrompt("x", p2, "T", opts) {
		t.Fatal("second ShowPrompt reported false")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.pending[p1]; ok {
		t.Fatal("expired entry for p1 survived the prune on show")
	}
	if _, ok := r.pending[p2]; !ok {
		t.Fatal("fresh entry for p2 missing")
	}
}
