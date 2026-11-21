package plugins

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// gateEvent is the shared fake event for the mutation-based gate tests: a
// tri-state allow (nil = undecided, mirroring TS allow: boolean | null),
// an optional reason with presence, and a handled flag.
type gateEvent struct {
	allow     *bool
	reason    string
	hasReason bool
	handled   bool
}

func allowAccessor(e *gateEvent) *bool { return e.allow }
func resetAllow(e *gateEvent)          { e.allow = nil }
func reasonAccessor(e *gateEvent) (string, bool) {
	return e.reason, e.hasReason
}
func handledAccessor(e *gateEvent) bool { return e.handled }

func gateLogger() (*log.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return log.New(&buf, "", 0), &buf
}

// gateHook builds a hookEntry that runs fn, recording the visit in ran.
func gateHook(name string, ran *[]string, fn func(*gateEvent)) hookEntry[*gateEvent] {
	return hookEntry[*gateEvent]{pluginName: name, handler: func(e *gateEvent) {
		*ran = append(*ran, name)
		fn(e)
	}}
}

func allowHook(name string, ran *[]string, allow bool) hookEntry[*gateEvent] {
	return gateHook(name, ran, func(e *gateEvent) { e.allow = boolAnswer(allow) })
}

func silentHook(name string, ran *[]string) hookEntry[*gateEvent] {
	return gateHook(name, ran, func(*gateEvent) {})
}

func TestAllowVetoGate(t *testing.T) {
	logger, _ := gateLogger()
	cases := []struct {
		name  string
		hooks func(ran *[]string) []hookEntry[*gateEvent]
		want  *bool
		ran   []string
	}{
		{"all silent is undecided", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), silentHook("b", ran)}
		}, nil, []string{"a", "b"}},
		{"allow sticks through later silence", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{allowHook("a", ran, true), silentHook("b", ran)}
		}, boolAnswer(true), []string{"a", "b"}},
		{"later deny vetoes an earlier allow", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{allowHook("a", ran, true), allowHook("b", ran, false), silentHook("c", ran)}
		}, boolAnswer(false), []string{"a", "b"}},
		{"deny returns immediately", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{allowHook("a", ran, false), allowHook("b", ran, true)}
		}, boolAnswer(false), []string{"a"}},
		{"no hooks is undecided", func(*[]string) []hookEntry[*gateEvent] { return nil }, nil, nil},
	}
	for _, c := range cases {
		var ran []string
		got := runAllowVetoGate(logger, "can_attack", c.hooks(&ran), &gateEvent{}, resetAllow, allowAccessor)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if len(ran) != len(c.ran) {
			t.Errorf("%s: ran %v, want %v", c.name, ran, c.ran)
		} else {
			for i := range ran {
				if ran[i] != c.ran[i] {
					t.Errorf("%s: ran %v, want %v", c.name, ran, c.ran)
				}
			}
		}
	}
}

func TestAllowVetoGateResetsBeforeEachHook(t *testing.T) {
	// TS resets event.allow to null before each hook (emitCanAttack): the
	// second hook must observe undecided even though the first allowed.
	logger, _ := gateLogger()
	var observed []*bool
	hooks := []hookEntry[*gateEvent]{
		allowHook("a", new([]string), true),
		{pluginName: "b", handler: func(e *gateEvent) { observed = append(observed, e.allow) }},
	}
	got := runAllowVetoGate(logger, "can_attack", hooks, &gateEvent{}, resetAllow, allowAccessor)
	if len(observed) != 1 || observed[0] != nil {
		t.Fatalf("second hook observed allow = %v, want nil (reset)", observed)
	}
	if got == nil || !*got {
		t.Fatalf("result = %v, want allow (the first hook's allow sticks)", got)
	}
}

func TestFirstAnswerGate(t *testing.T) {
	logger, _ := gateLogger()
	cases := []struct {
		name  string
		hooks func(ran *[]string) []hookEntry[*gateEvent]
		want  *bool
		ran   []string
	}{
		{"all silent is undecided", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), silentHook("b", ran)}
		}, nil, []string{"a", "b"}},
		{"first allow wins immediately", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{allowHook("a", ran, true), allowHook("b", ran, false)}
		}, boolAnswer(true), []string{"a"}},
		{"first deny wins immediately", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), allowHook("b", ran, false), allowHook("c", ran, true)}
		}, boolAnswer(false), []string{"a", "b"}},
		{"no hooks is undecided", func(*[]string) []hookEntry[*gateEvent] { return nil }, nil, nil},
	}
	for _, c := range cases {
		var ran []string
		got := runFirstAnswerGate(logger, "can_teleport", c.hooks(&ran), &gateEvent{}, allowAccessor)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if len(ran) != len(c.ran) {
			t.Errorf("%s: ran %v, want %v", c.name, ran, c.ran)
		}
	}
}

func TestReasonGate(t *testing.T) {
	logger, _ := gateLogger()
	deny := func(name string, ran *[]string, reason string, has bool) hookEntry[*gateEvent] {
		return gateHook(name, ran, func(e *gateEvent) {
			e.allow = boolAnswer(false)
			e.reason, e.hasReason = reason, has
		})
	}
	cases := []struct {
		name  string
		hooks func(ran *[]string) []hookEntry[*gateEvent]
		want  string
		ran   []string
	}{
		{"all silent allows", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran)}
		}, "", []string{"a"}},
		{"allow returns immediately with no reason", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{allowHook("a", ran, true), deny("b", ran, "nope", true)}
		}, "", []string{"a"}},
		{"deny without reason uses the default", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{deny("a", ran, "", false)}
		}, "You can't log out right now.", []string{"a"}},
		{"deny with reason returns the reason", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{deny("a", ran, "In combat!", true)}
		}, "In combat!", []string{"a"}},
		{"deny with present-but-empty reason returns empty (TS ?? semantics)", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{deny("a", ran, "", true)}
		}, "", []string{"a"}},
		{"deny after silence uses the default", func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), deny("b", ran, "", false), allowHook("c", ran, true)}
		}, "You can't log out right now.", []string{"a", "b"}},
		{"no hooks allows", func(*[]string) []hookEntry[*gateEvent] { return nil }, "", nil},
	}
	for _, c := range cases {
		var ran []string
		got := runReasonGate(logger, "can_logout", c.hooks(&ran), &gateEvent{}, allowAccessor, reasonAccessor, defaultLogoutRefusal)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
		if len(ran) != len(c.ran) {
			t.Errorf("%s: ran %v, want %v", c.name, ran, c.ran)
		}
	}
	if defaultLogoutRefusal != "You can't log out right now." {
		t.Fatalf("defaultLogoutRefusal = %q, want the TS literal", defaultLogoutRefusal)
	}
}

func TestHandledGate(t *testing.T) {
	logger, _ := gateLogger()
	handle := func(name string, ran *[]string) hookEntry[*gateEvent] {
		return gateHook(name, ran, func(e *gateEvent) { e.handled = true })
	}
	cases := []struct {
		name    string
		initial bool
		hooks   func(ran *[]string) []hookEntry[*gateEvent]
		want    bool
		ran     []string
	}{
		{"none handles", false, func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), silentHook("b", ran)}
		}, false, []string{"a", "b"}},
		{"first handles short-circuits", false, func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{handle("a", ran), handle("b", ran)}
		}, true, []string{"a"}},
		{"second handles", false, func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{silentHook("a", ran), handle("b", ran), handle("c", ran)}
		}, true, []string{"a", "b"}},
		{"pre-handled runs zero hooks and returns true", true, func(ran *[]string) []hookEntry[*gateEvent] {
			return []hookEntry[*gateEvent]{handle("a", ran)}
		}, true, nil},
		{"no hooks is unhandled", false, func(*[]string) []hookEntry[*gateEvent] { return nil }, false, nil},
	}
	for _, c := range cases {
		var ran []string
		got := runHandledGate(logger, "npc_interaction", c.hooks(&ran), &gateEvent{handled: c.initial}, handledAccessor)
		if got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
		if len(ran) != len(c.ran) {
			t.Errorf("%s: ran %v, want %v", c.name, ran, c.ran)
		}
	}
}

func TestGatePanicIsolation(t *testing.T) {
	logger, buf := gateLogger()
	panicking := hookEntry[*gateEvent]{pluginName: "boom", handler: func(*gateEvent) { panic("gate-boom") }}
	var ran []string

	// Allow-veto: panic, then a deny still vetoes.
	got := runAllowVetoGate(logger, "can_attack", []hookEntry[*gateEvent]{panicking, allowHook("b", &ran, false)}, &gateEvent{}, resetAllow, allowAccessor)
	if got == nil || *got {
		t.Fatalf("allow-veto after panic = %v, want deny", got)
	}
	// First-answer: panic, then an allow still answers.
	got = runFirstAnswerGate(logger, "can_teleport", []hookEntry[*gateEvent]{panicking, allowHook("b", &ran, true)}, &gateEvent{}, allowAccessor)
	if got == nil || !*got {
		t.Fatalf("first-answer after panic = %v, want allow", got)
	}
	// Handled: panic, then a handler still handles.
	handled := runHandledGate(logger, "npc_interaction", []hookEntry[*gateEvent]{panicking, gateHook("b", &ran, func(e *gateEvent) { e.handled = true })}, &gateEvent{}, handledAccessor)
	if !handled {
		t.Fatal("handled gate after panic = false, want true")
	}
	if !strings.Contains(buf.String(), "[plugins] can_attack hook failed (boom)") {
		t.Fatalf("log = %q, want the can_attack failure line", buf.String())
	}
	if !strings.Contains(buf.String(), "gate-boom") {
		t.Fatalf("log = %q, want the recovered value", buf.String())
	}
}

func TestDialogueFirstAnswer(t *testing.T) {
	logger, buf := gateLogger()
	type ctx struct{ id int }
	isNonEmpty := func(s string) bool { return s != "" }
	answer := func(name string, ran *[]string, result string) dialogueHook[*ctx, string] {
		return dialogueHook[*ctx, string]{pluginName: name, handler: func(*ctx) string {
			*ran = append(*ran, name)
			return result
		}}
	}

	// First truthy answer wins; falsy answers fall through.
	var ran []string
	got, ok := runFirstDialogueAnswer(logger, "npc dialogue variant", []dialogueHook[*ctx, string]{
		answer("a", &ran, ""), answer("b", &ran, "variant-b"), answer("c", &ran, "variant-c"),
	}, &ctx{}, isNonEmpty)
	if !ok || got != "variant-b" {
		t.Fatalf("got (%q, %v), want (variant-b, true)", got, ok)
	}
	if len(ran) != 2 || ran[1] != "b" {
		t.Fatalf("ran %v, want [a b]", ran)
	}

	// No answer yields zero/false.
	ran = nil
	got, ok = runFirstDialogueAnswer(logger, "npc dialogue variant", []dialogueHook[*ctx, string]{
		answer("a", &ran, ""), answer("b", &ran, ""),
	}, &ctx{}, isNonEmpty)
	if ok || got != "" {
		t.Fatalf("got (%q, %v), want (\"\", false)", got, ok)
	}

	// A panicking hook is warned with "threw" (not "hook failed") and the
	// gate continues with the next hook.
	ran = nil
	panicking := dialogueHook[*ctx, string]{pluginName: "boom", handler: func(*ctx) string { panic("variant-boom") }}
	got, ok = runFirstDialogueAnswer(logger, "npc dialogue variant", []dialogueHook[*ctx, string]{
		panicking, answer("b", &ran, "variant-b"),
	}, &ctx{}, isNonEmpty)
	if !ok || got != "variant-b" {
		t.Fatalf("after panic got (%q, %v), want (variant-b, true)", got, ok)
	}
	if want := "[plugins] npc dialogue variant hook threw (boom)"; !strings.Contains(buf.String(), want) {
		t.Fatalf("log = %q, want %q", buf.String(), want)
	}
	if strings.Contains(buf.String(), "hook failed") {
		t.Fatalf("log = %q, want no executeHook-style failure line for dialogue hooks", buf.String())
	}
}

func TestDialogueConditionFirstBoolean(t *testing.T) {
	// emitNpcDialogueCondition: the first boolean wins; null/undefined
	// (nil *bool here) falls through.
	logger, _ := gateLogger()
	type ctx struct{ text string }
	isBool := func(b *bool) bool { return b != nil }
	answer := func(name string, result *bool) dialogueHook[*ctx, *bool] {
		return dialogueHook[*ctx, *bool]{pluginName: name, handler: func(*ctx) *bool { return result }}
	}
	got, ok := runFirstDialogueAnswer(logger, "npc dialogue condition", []dialogueHook[*ctx, *bool]{
		answer("a", nil), answer("b", boolAnswer(false)), answer("c", boolAnswer(true)),
	}, &ctx{}, isBool)
	if !ok || got == nil || *got {
		t.Fatalf("got (%v, %v), want (false, true): the first boolean wins", got, ok)
	}
	got, ok = runFirstDialogueAnswer(logger, "npc dialogue condition", []dialogueHook[*ctx, *bool]{
		answer("a", nil),
	}, &ctx{}, isBool)
	if ok || got != nil {
		t.Fatalf("got (%v, %v), want (nil, false)", got, ok)
	}
}
