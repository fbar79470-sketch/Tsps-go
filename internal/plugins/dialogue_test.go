package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// Dialogue-family tests: registration, the first-answer emits,
// and the throw-and-continue failure mode in dialogue.go,
// grounded in PluginManager.ts (emitNpcDialogueVariant
// 1289-1304, emitNpcDialogueCondition 1306-1320, registration
// 3511-3521) and PluginTypes.ts (context/condition shapes
// 206-222, handler signatures 1108-1122). Fixtures only.

type fakeNpcDef struct {
	id         int
	name       string
	actions    []string
	size       int
	attackable bool
}

func (d *fakeNpcDef) ID() int            { return d.id }
func (d *fakeNpcDef) Name() string       { return d.name }
func (d *fakeNpcDef) Actions() []string  { return d.actions }
func (d *fakeNpcDef) Size() int          { return d.size }
func (d *fakeNpcDef) IsAttackable() bool { return d.attackable }

func testDialoguePlayer() *game.Player {
	return game.NewPlayer("testplayer", game.Location{X: 100, Y: 100, Z: 0})
}

func testDialogueContext() *NpcDialogueContext {
	return &NpcDialogueContext{
		Player:     testDialoguePlayer(),
		NPC:        game.NewNPC(100, game.Location{X: 101, Y: 100, Z: 0}),
		NpcID:      100,
		Definition: &fakeNpcDef{id: 100, name: "Bob", actions: []string{"Talk-to"}, size: 1},
		Pages: []NpcDialoguePage{
			{Page: "Transcript:Bob", Variants: []string{"standard-dialogue", "quest-stage-2"}},
		},
	}
}

func TestEmitNpcDialogueVariantFirstAnswerWins(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	ctx := testDialogueContext()
	var order []string
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		order = append(order, "first")
		if ev != ctx {
			t.Error("variant handler must receive the emit event")
		}
		return nil // Fall through.
	})
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		order = append(order, "second")
		return DialogueVariant("quest-stage-2")
	})
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		order = append(order, "third")
		return DialogueVariant("standard-dialogue")
	})
	choice := h.EmitNpcDialogueVariant(ctx)
	if choice == nil || choice.Variant != "quest-stage-2" || choice.HasPage {
		t.Fatalf("choice = %+v, want bare variant quest-stage-2", choice)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hooks ran %v, want [first second]: the first answer stops the gate", order)
	}
}

func TestEmitNpcDialogueVariantForms(t *testing.T) {
	// Page-qualified answer: page + variant preserved.
	h, _ := newTestHub()
	api := newApi(h, "P")
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return DialogueVariantOnPage("Transcript:Bob", "quest-stage-2")
	})
	choice := h.EmitNpcDialogueVariant(testDialogueContext())
	if choice == nil || choice.Page != "Transcript:Bob" || !choice.HasPage || choice.Variant != "quest-stage-2" {
		t.Fatalf("choice = %+v, want page Transcript:Bob variant quest-stage-2", choice)
	}

	// An empty-name answer falls through (the empty string is
	// falsy in TS), and the next hook's answer wins.
	h, _ = newTestHub()
	api = newApi(h, "P")
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return DialogueVariant("")
	})
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return DialogueVariant("standard-dialogue")
	})
	choice = h.EmitNpcDialogueVariant(testDialogueContext())
	if choice == nil || choice.Variant != "standard-dialogue" {
		t.Fatalf("choice = %+v, want standard-dialogue after the empty answer fell through", choice)
	}

	// A non-nil empty choice IS an answer (a TS object is
	// truthy even with an empty variant): later hooks never
	// run, and the emit returns the empty choice.
	h, _ = newTestHub()
	api = newApi(h, "P")
	secondRan := false
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return &NpcDialogueVariantChoice{}
	})
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		secondRan = true
		return DialogueVariant("standard-dialogue")
	})
	choice = h.EmitNpcDialogueVariant(testDialogueContext())
	if choice == nil {
		t.Fatal("an empty non-nil choice must answer, not fall through")
	}
	if secondRan {
		t.Fatal("the empty choice must stop later hooks")
	}
}

func TestEmitNpcDialogueVariantThrowContinues(t *testing.T) {
	h, buf := newTestHub()
	api := newApi(h, "P")
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		panic("boom")
	})
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return DialogueVariant("standard-dialogue")
	})
	choice := h.EmitNpcDialogueVariant(testDialogueContext())
	if choice == nil || choice.Variant != "standard-dialogue" {
		t.Fatalf("choice = %+v, want standard-dialogue after the throwing hook", choice)
	}
	// TS-exact warning: "threw", not "hook failed".
	if !strings.Contains(buf.String(), "[plugins] npc dialogue variant hook threw (P)") {
		t.Fatalf("log = %q, want the npc dialogue variant threw warning", buf.String())
	}
}

func TestEmitNpcDialogueVariantNoAnswer(t *testing.T) {
	// No hooks: nil.
	h, _ := newTestHub()
	if got := h.EmitNpcDialogueVariant(testDialogueContext()); got != nil {
		t.Fatalf("no hooks = %+v, want nil", got)
	}
	// All hooks fall through: nil.
	api := newApi(h, "P")
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice { return nil })
	if got := h.EmitNpcDialogueVariant(testDialogueContext()); got != nil {
		t.Fatalf("all fall through = %+v, want nil", got)
	}
	// Nil event, nil hub: nil, no panic.
	if got := h.EmitNpcDialogueVariant(nil); got != nil {
		t.Fatalf("nil event = %+v, want nil", got)
	}
	var nilHub *Hub
	if got := nilHub.EmitNpcDialogueVariant(testDialogueContext()); got != nil {
		t.Fatalf("nil hub = %+v, want nil", got)
	}
	// A nil handler registers nothing.
	api.OnNpcDialogueVariant(nil)
	if got := h.EmitNpcDialogueVariant(testDialogueContext()); got != nil {
		t.Fatalf("nil handler = %+v, want nil", got)
	}
}

func TestEmitNpcDialogueConditionFirstBooleanWins(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var order []string
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		order = append(order, "first")
		if ev.Text != "If the player already has the necessary items:" {
			t.Errorf("condition text = %q", ev.Text)
		}
		return nil // Fall through.
	})
	// False is an answer: the first boolean wins, even when
	// it is false, and later hooks never run.
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		order = append(order, "second")
		return boolPtr(false)
	})
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		order = append(order, "third")
		return boolPtr(true)
	})
	ctx := testDialogueContext()
	ev := NewNpcDialogueConditionEvent(*ctx, "If the player already has the necessary items:", "step-7", true)
	answer := h.EmitNpcDialogueCondition(ev)
	if answer == nil || *answer != false {
		t.Fatalf("answer = %+v, want false", answer)
	}
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Fatalf("hooks ran %v, want [first second]", order)
	}
	// The condition event carries the context fields.
	if ev.Player != ctx.Player || ev.NPC != ctx.NPC || ev.NpcID != 100 || ev.StepID != "step-7" || !ev.HasStepID {
		t.Fatalf("condition event = %+v, want the context plus text/stepId", ev)
	}
	if len(ev.Pages) != 1 || ev.Pages[0].Page != "Transcript:Bob" {
		t.Fatalf("condition pages = %+v, want the context pages", ev.Pages)
	}
}

func TestEmitNpcDialogueConditionThrowContinues(t *testing.T) {
	h, buf := newTestHub()
	api := newApi(h, "P")
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		panic("boom")
	})
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		return boolPtr(true)
	})
	answer := h.EmitNpcDialogueCondition(NewNpcDialogueConditionEvent(*testDialogueContext(), "text", "", false))
	if answer == nil || *answer != true {
		t.Fatalf("answer = %+v, want true after the throwing hook", answer)
	}
	if !strings.Contains(buf.String(), "[plugins] npc dialogue condition hook threw (P)") {
		t.Fatalf("log = %q, want the npc dialogue condition threw warning", buf.String())
	}
}

func TestEmitNpcDialogueConditionNoAnswer(t *testing.T) {
	h, _ := newTestHub()
	ev := NewNpcDialogueConditionEvent(*testDialogueContext(), "text", "", false)
	if got := h.EmitNpcDialogueCondition(ev); got != nil {
		t.Fatalf("no hooks = %+v, want nil", got)
	}
	api := newApi(h, "P")
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool { return nil })
	if got := h.EmitNpcDialogueCondition(ev); got != nil {
		t.Fatalf("all fall through = %+v, want nil", got)
	}
	if got := h.EmitNpcDialogueCondition(nil); got != nil {
		t.Fatalf("nil event = %+v, want nil", got)
	}
	var nilHub *Hub
	if got := nilHub.EmitNpcDialogueCondition(ev); got != nil {
		t.Fatalf("nil hub = %+v, want nil", got)
	}
	api.OnNpcDialogueCondition(nil)
	if got := h.EmitNpcDialogueCondition(ev); got != nil {
		t.Fatalf("nil handler = %+v, want nil", got)
	}
	// An event without a step id reports HasStepID false.
	if ev.HasStepID {
		t.Fatal("an event built without a step id must report HasStepID false")
	}
}

func TestDialogueRegistrationAfterLoadIgnored(t *testing.T) {
	h, _ := newTestHub()
	h.Load()
	api := newApi(h, "P")
	api.OnNpcDialogueVariant(func(ev *NpcDialogueContext) *NpcDialogueVariantChoice {
		return DialogueVariant("standard-dialogue")
	})
	api.OnNpcDialogueCondition(func(ev *NpcDialogueConditionEvent) *bool {
		return boolPtr(true)
	})
	if got := h.EmitNpcDialogueVariant(testDialogueContext()); got != nil {
		t.Fatalf("post-load variant registration = %+v, want nil", got)
	}
	ev := NewNpcDialogueConditionEvent(*testDialogueContext(), "text", "", false)
	if got := h.EmitNpcDialogueCondition(ev); got != nil {
		t.Fatalf("post-load condition registration = %+v, want nil", got)
	}
}

func TestNpcDialogueCustomEventNames(t *testing.T) {
	// Every npc-dialogue:* name rides the existing custom-event
	// surface: subscribing and emitting round-trips, which
	// proves each name passes the custom-event name rule.
	names := []string{
		NpcDialogueStartEventName,
		NpcDialogueShopEventName,
		NpcDialogueMenuEventName,
		NpcDialogueHookEventName,
		NpcDialogueChoiceEventName,
		NpcDialogueConditionEventName,
		NpcDialogueLineEventName,
		NpcDialogueActionEventName,
	}
	for _, name := range names {
		h, _ := newTestHub()
		api := newApi(h, "P")
		var seen []any
		api.OnCustomEvent(name, func(payload any) { seen = append(seen, payload) })
		h.EmitCustomEvent(name, "payload")
		if len(seen) != 1 || seen[0] != "payload" {
			t.Fatalf("event %q: seen %v, want one delivery", name, seen)
		}
		if name != "npc-dialogue:"+strings.SplitN(name, ":", 2)[1] {
			t.Fatalf("event name %q is not npc-dialogue namespaced", name)
		}
	}
}

func TestEmitNpcOperated(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var seen *NpcOperatedEvent
	api.OnCustomEvent(NpcOperatedEventName, func(payload any) {
		seen, _ = payload.(*NpcOperatedEvent)
	})
	player := testDialoguePlayer()
	npc := game.NewNPC(100, game.Location{X: 101, Y: 100, Z: 0})
	h.EmitNpcOperated(&NpcOperatedEvent{Player: player, NPC: npc, NpcID: 100, Option: "talk-to", HasOption: true})
	if seen == nil {
		t.Fatal("npc:operated must deliver to custom-event subscribers")
	}
	if seen.Player != player || seen.NPC != npc || seen.NpcID != 100 || seen.Option != "talk-to" || !seen.HasOption {
		t.Fatalf("operated = %+v, want the emitted payload", seen)
	}
	if NpcOperatedEventName != "npc:operated" {
		t.Fatalf("operated event name = %q, want npc:operated", NpcOperatedEventName)
	}
	// Nil event and nil hub emit nothing and never panic.
	h.EmitNpcOperated(nil)
	var nilHub *Hub
	nilHub.EmitNpcOperated(&NpcOperatedEvent{})
}

func TestDialogueVariantChoiceHelpers(t *testing.T) {
	if got := DialogueVariant(""); got != nil {
		t.Fatalf("DialogueVariant(\"\") = %+v, want nil (empty string falls through)", got)
	}
	if got := DialogueVariant("standard-dialogue"); got == nil || got.Variant != "standard-dialogue" || got.HasPage {
		t.Fatalf("DialogueVariant = %+v, want bare variant", got)
	}
	if got := DialogueVariantOnPage("Transcript:Bob", ""); got == nil || got.Page != "Transcript:Bob" || !got.HasPage {
		t.Fatalf("DialogueVariantOnPage = %+v, want a non-nil page-qualified choice", got)
	}
}

// promptTestSession is a minimal DialogueSender for the
// SendMultiChatboxPrompt tests: it embeds the (nil) SessionPort
// interface to satisfy the Player.Session field type and records
// the chatbox group so the prompt registry's ownership check
// sees group 219 mounted.
type promptTestSession struct {
	game.SessionPort
	chatboxID int
	removals  int
	calls     []string
}

func (s *promptTestSession) SendChatboxInterface(groupID int, unclamp bool) {
	s.chatboxID = groupID
	s.calls = append(s.calls, "chatbox")
}
func (s *promptTestSession) SendNpcHeadOnInterface(int, int)               {}
func (s *promptTestSession) SendPlayerHeadOnInterface(int)                 {}
func (s *promptTestSession) SendInterfaceAnimation(int, int)               {}
func (s *promptTestSession) SendString(string, int)                        {}
func (s *promptTestSession) SendClientScript(int, ...game.ClientScriptArg) {}
func (s *promptTestSession) SendInterfaceFlagsRange(int, int, int, int)    {}
func (s *promptTestSession) SendItemOnInterfaces(int, int, int)            {}
func (s *promptTestSession) SendInterfaceScript(int)                       {}
func (s *promptTestSession) SendInterfaceRemoval() {
	s.removals++
	s.chatboxID = -1
}
func (s *promptTestSession) IsChatboxInterface(groupID int) bool        { return groupID == s.chatboxID }
func (s *promptTestSession) WithChatboxOwner(owner any, present func()) { present() }

func TestSendMultiChatboxPrompt(t *testing.T) {
	h, _ := newTestHub()
	registry := game.NewMultiChatboxPrompt(nil)
	h.SetMultiChatboxPrompt(registry)
	api := newApi(h, "P")
	p := testDialoguePlayer()
	s := &promptTestSession{chatboxID: -1}
	p.Session = s

	var picked []int
	var texts []string
	cb := func(player *game.Player, index int, text string) {
		picked = append(picked, index)
		texts = append(texts, text)
	}
	if !api.SendMultiChatboxPrompt(p, "Pick one", "First", PromptCallback(cb), "Second", PromptCallback(cb)) {
		t.Fatal("SendMultiChatboxPrompt reported false for a valid prompt")
	}
	if s.chatboxID != 219 {
		t.Fatalf("chatboxID = %d, want 219", s.chatboxID)
	}
	// The pick flows through the registry: option 2, 1-based.
	if !registry.HandleInterfaceActionClick(p, game.OptionsWidgetID, 2, 219) {
		t.Fatal("pick not consumed")
	}
	if len(picked) != 1 || picked[0] != 1 || len(texts) != 1 || texts[0] != "Second" {
		t.Fatalf("picked = %v texts = %v, want [1] [Second]", picked, texts)
	}
}

func TestSendMultiChatboxPromptValidation(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	p := testDialoguePlayer()
	p.Session = &promptTestSession{chatboxID: -1}
	cb := PromptCallback(func(*game.Player, int, string) {})

	// No registry installed: false without panicking.
	if api.SendMultiChatboxPrompt(p, "T", "A", cb, "B", cb) {
		t.Fatal("uninstalled registry accepted the prompt")
	}
	h.SetMultiChatboxPrompt(game.NewMultiChatboxPrompt(nil))

	// Odd pair count.
	if api.SendMultiChatboxPrompt(p, "T", "A", cb, "B") {
		t.Fatal("odd pair count accepted")
	}
	// Non-string text.
	if api.SendMultiChatboxPrompt(p, "T", 42, cb) {
		t.Fatal("non-string text accepted")
	}
	// Non-callback value.
	if api.SendMultiChatboxPrompt(p, "T", "A", "not a callback") {
		t.Fatal("non-callback value accepted")
	}
	// No pairs at all.
	if api.SendMultiChatboxPrompt(p, "T") {
		t.Fatal("empty pairs accepted")
	}
	// Nil player.
	if api.SendMultiChatboxPrompt(nil, "T", "A", cb, "B", cb) {
		t.Fatal("nil player accepted")
	}
	// A rejected call shows nothing and records nothing.
	if _, _, ok := h.MultiChatboxPrompt().GetPending(p); ok {
		t.Fatal("rejected prompt left a pending entry")
	}
}

func TestMultiChatboxPromptHubInstall(t *testing.T) {
	var nilHub *Hub
	if nilHub.MultiChatboxPrompt() != nil {
		t.Fatal("nil hub answered a registry")
	}
	nilHub.SetMultiChatboxPrompt(game.NewMultiChatboxPrompt(nil)) // must not panic
	h, _ := newTestHub()
	if h.MultiChatboxPrompt() != nil {
		t.Fatal("fresh hub answered a registry before install")
	}
	registry := game.NewMultiChatboxPrompt(nil)
	h.SetMultiChatboxPrompt(registry)
	if h.MultiChatboxPrompt() != registry {
		t.Fatal("installed registry not returned")
	}
	h.SetMultiChatboxPrompt(nil)
	if h.MultiChatboxPrompt() != nil {
		t.Fatal("nil install did not clear the registry")
	}
}
