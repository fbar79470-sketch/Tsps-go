package game

import (
	"reflect"
	"sync"
	"testing"
)

// The test-only command: a fixture registered ONLY by tests in
// this file, never shipped as content and never registered at
// boot. TS core defines no commands (see commands.go), so the
// registry is proven with this probe instead of a real command.
const probeCommand = "probe"

func newProbePlayer() *Player {
	return NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
}

// The registry satisfies the chat listener's dispatcher seam
// (listeners.CommandDispatcher) directly: DispatchCommand with
// no return value. The interface is restated here — instead of
// imported — because the game package cannot import listeners;
// the signatures must stay identical, and the coordinator's
// wiring assigns the registry to the seam without an adapter.
var _ interface{ DispatchCommand(*Player, string) } = NewCommandRegistry()

func TestParseCommandVectors(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ParsedCommand
		ok    bool
	}{
		{"plain", "players", ParsedCommand{Raw: "players", Base: "players", Parts: []string{"players"}}, true},
		{"args", "item 1351 1", ParsedCommand{Raw: "item 1351 1", Base: "item", Parts: []string{"item", "1351", "1"}}, true},
		{"case", "ITEM 1351", ParsedCommand{Raw: "ITEM 1351", Base: "item", Parts: []string{"item", "1351"}}, true},
		{"trimmed", "  yell hello  ", ParsedCommand{Raw: "yell hello", Base: "yell", Parts: []string{"yell", "hello"}}, true},
		{"runs of whitespace", "yell\t hello\n  world", ParsedCommand{Raw: "yell\t hello\n  world", Base: "yell", Parts: []string{"yell", "hello", "world"}}, true},
		{"raw keeps spacing", "yell  hello", ParsedCommand{Raw: "yell  hello", Base: "yell", Parts: []string{"yell", "hello"}}, true},
		{"nul to space", "item\x001351", ParsedCommand{Raw: "item 1351", Base: "item", Parts: []string{"item", "1351"}}, true},
		{"double colon variant", "::item 1351", ParsedCommand{Raw: "::item 1351", Base: "item", Parts: []string{"item", "1351"}}, true},
		{"slash variant", "/item 1351", ParsedCommand{Raw: "/item 1351", Base: "item", Parts: []string{"item", "1351"}}, true},
		{"mixed prefix", "://:item", ParsedCommand{Raw: "://:item", Base: "item", Parts: []string{"item"}}, true},
		{"trailing punctuation", "item, 1351", ParsedCommand{Raw: "item, 1351", Base: "item", Parts: []string{"item", "1351"}}, true},
		{"bang suffix", "players!", ParsedCommand{Raw: "players!", Base: "players", Parts: []string{"players"}}, true},
		{"underscore and digits", "my_cmd2 arg", ParsedCommand{Raw: "my_cmd2 arg", Base: "my_cmd2", Parts: []string{"my_cmd2", "arg"}}, true},
		{"digit start", "2fast", ParsedCommand{Raw: "2fast", Base: "2fast", Parts: []string{"2fast"}}, true},
		{"nbsp splits", "yell\u00a0hello", ParsedCommand{Raw: "yell\u00a0hello", Base: "yell", Parts: []string{"yell", "hello"}}, true},
		// U+0085 (NEL) is NOT JavaScript whitespace: it does not
		// split, and it terminates the [a-z0-9_] base run.
		{"nel does not split", "yell\u0085hello", ParsedCommand{Raw: "yell\u0085hello", Base: "yell", Parts: []string{"yell"}}, true},
		{"empty", "", ParsedCommand{}, false},
		{"whitespace only", "  \t ", ParsedCommand{}, false},
		{"nul only", "\x00", ParsedCommand{}, false},
		{"no base match", "!!!", ParsedCommand{}, false},
		{"dashes only", "---", ParsedCommand{}, false},
		{"prefix only", "::", ParsedCommand{}, false},
		{"prefix then punctuation", "::!!!", ParsedCommand{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := ParseCommand(tc.input)
			if ok != tc.ok {
				t.Fatalf("ParseCommand(%q) ok = %v, want %v", tc.input, ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("ParseCommand(%q) = %+v, want %+v", tc.input, got, tc.want)
			}
		})
	}
}

func TestRegisterNormalizationAndInvalidInput(t *testing.T) {
	r := NewCommandRegistry()
	called := false
	handler := func(e *CommandEvent) bool { called = true; return true }

	// Trim + lowercase: "  Probe " registers as "probe".
	r.Register("  Probe ", handler, RightsNone, "  probe desc  ")
	if got := r.RegisteredCommands(newProbePlayer()); len(got) != 1 || got[0].Command != probeCommand || got[0].Description != "probe desc" {
		t.Fatalf("RegisteredCommands = %+v, want one probe with trimmed description", got)
	}
	// Nil handler and empty names are silently ignored (no
	// panic, no registration).
	r.Register("nilhandler", nil, RightsNone, "")
	r.Register("", handler, RightsNone, "")
	r.Register("   ", handler, RightsNone, "")
	if got := r.RegisteredCommands(newProbePlayer()); len(got) != 1 {
		t.Fatalf("RegisteredCommands after invalid input = %+v, want only probe", got)
	}
	// Dispatch through the normalized name works.
	p := newProbePlayer()
	if !r.Execute(p, "PROBE") {
		t.Fatal("Execute(PROBE) = false, want true")
	}
	if !called {
		t.Error("handler was not called")
	}
}

func TestRegisterNoColonStripping(t *testing.T) {
	r := NewCommandRegistry()
	called := false
	// TS registerCommand does NOT strip "::": a name registered
	// with the prefix never matches a dispatched base (bases
	// never carry colons). It still lists in RegisteredCommands
	// under its registered (normalized) name.
	r.Register("::ghost", func(e *CommandEvent) bool { called = true; return true }, RightsNone, "")
	p := newProbePlayer()
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })
	if !r.Execute(p, "ghost") {
		t.Fatal("Execute(ghost) = false, want true (unknown-command path)")
	}
	if called {
		t.Error("::ghost handler ran for base ghost, want no match")
	}
	if len(sent) != 1 || sent[0] != "This command does not exist. Try ::players." {
		t.Errorf("sent = %v, want the unknown-command message", sent)
	}
}

func TestRegisterDuplicatesAppendAndOverwriteRights(t *testing.T) {
	r := NewCommandRegistry()
	var order []string
	first := func(e *CommandEvent) bool { order = append(order, "first"); return false }
	second := func(e *CommandEvent) bool { order = append(order, "second"); return true }
	r.Register(probeCommand, first, RightsDeveloper, "first desc")
	// Re-registration appends the handler and OVERWRITES the
	// floor with the latest registration's floor.
	r.Register(probeCommand, second, RightsNone, "")
	p := newProbePlayer()
	if !r.Execute(p, probeCommand) {
		t.Fatal("Execute = false, want true")
	}
	if !reflect.DeepEqual(order, []string{"first", "second"}) {
		t.Errorf("handler order = %v, want [first second]", order)
	}
	// The floor is now RightsNone: a NONE player passes. (With
	// the first registration's DEVELOPER floor the dispatch
	// would have denied before either handler ran.)
	if len(order) != 2 {
		t.Error("rights overwrite did not take effect")
	}
	// Description comes from the first non-empty registration.
	if got := r.RegisteredCommands(p); len(got) != 1 || got[0].Description != "first desc" {
		t.Errorf("RegisteredCommands = %+v, want first desc", got)
	}
}

func TestExecuteUnknownCommand(t *testing.T) {
	r := NewCommandRegistry()
	p := newProbePlayer()
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })
	if !r.Execute(p, "nosuchcommand arg") {
		t.Fatal("Execute = false, want true (dispatch ran)")
	}
	if len(sent) != 1 || sent[0] != "This command does not exist. Try ::players." {
		t.Errorf("sent = %v, want the unknown-command message", sent)
	}
}

func TestExecuteDeadPlayerAndUnparseable(t *testing.T) {
	r := NewCommandRegistry()
	called := false
	r.Register(probeCommand, func(e *CommandEvent) bool { called = true; return true }, RightsNone, "")
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })

	dead := newProbePlayer()
	dead.Skills().SetCurrentLevel(SkillHitpoints, 0, false)
	if r.Execute(dead, probeCommand) {
		t.Error("Execute(dead) = true, want false (silent refusal)")
	}
	if called || len(sent) != 0 {
		t.Error("dead player dispatch produced a handler call or a message")
	}

	p := newProbePlayer()
	for _, input := range []string{"", "   ", "!!!", "::"} {
		if r.Execute(p, input) {
			t.Errorf("Execute(%q) = true, want false", input)
		}
	}
	if called || len(sent) != 0 {
		t.Error("unparseable input produced a handler call or a message")
	}
	if r.Execute(nil, probeCommand) {
		t.Error("Execute(nil player) = true, want false")
	}
}

func TestExecuteTestOnlyCommandEndToEnd(t *testing.T) {
	r := NewCommandRegistry()
	p := newProbePlayer()
	var replies []string
	var seen *CommandEvent
	// The test-only command: records the event and replies
	// through a test-local fake standing in for
	// Player.SendMessage (which the phase 12 sendMessage work
	// owns; handlers reply via the player in production).
	r.Register(probeCommand, func(e *CommandEvent) bool {
		seen = e
		replies = append(replies, "probe saw "+e.Base)
		return true
	}, RightsNone, "Test probe")
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })
	if !r.Execute(p, "PROBE alpha  beta") {
		t.Fatal("Execute = false, want true")
	}
	if seen == nil {
		t.Fatal("handler never ran")
	}
	if seen.Player != p || seen.Raw != "PROBE alpha  beta" || seen.Base != probeCommand ||
		!reflect.DeepEqual(seen.Parts, []string{probeCommand, "alpha", "beta"}) {
		t.Errorf("event = %+v, want player/raw/base/parts of the probe dispatch", seen)
	}
	if !reflect.DeepEqual(replies, []string{"probe saw probe"}) {
		t.Errorf("replies = %v", replies)
	}
	if len(sent) != 0 {
		t.Errorf("engine messages sent = %v, want none (command was handled)", sent)
	}
}

func TestRightsGate(t *testing.T) {
	r := NewCommandRegistry()
	rights := RightsNone
	r.SetRightsSource(func(_ *Player) int { return rights })
	called := false
	r.Register("staffonly", func(e *CommandEvent) bool { called = true; return true }, RightsModerator, "Staff")
	p := newProbePlayer()
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })

	// Below the floor: denied, handler never runs, denial
	// counts as handled (no unknown-command message follows).
	if !r.Execute(p, "staffonly") {
		t.Fatal("Execute = false, want true (denial ran)")
	}
	if called {
		t.Error("handler ran for a player below the floor")
	}
	if len(sent) != 1 || sent[0] != "You do not have permission to use this command." {
		t.Errorf("sent = %v, want the denial message only", sent)
	}

	// At the floor: passes (floors are inclusive).
	rights = RightsModerator
	sent = nil
	called = false
	if !r.Execute(p, "staffonly") {
		t.Fatal("Execute = false, want true")
	}
	if !called || len(sent) != 0 {
		t.Errorf("called=%v sent=%v, want handler run and no message", called, sent)
	}

	// Above the floor: passes.
	rights = RightsDeveloper
	called = false
	if !r.Execute(p, "staffonly") || !called {
		t.Error("developer could not run a moderator command")
	}
}

func TestRightsPrecedence(t *testing.T) {
	r := NewCommandRegistry()
	rights := RightsNone
	r.SetRightsSource(func(_ *Player) int { return rights })
	r.Register("cmd", func(e *CommandEvent) bool { return true }, RightsNone, "")
	p := newProbePlayer()

	// Override beats registration.
	r.SetCommandRights("cmd", RightsOwner)
	if r.PlayerHasCommandRights(p, "cmd") {
		t.Error("override to OWNER did not restrict a NONE player")
	}
	// World.json permissions beat the override (lowering).
	r.SetPermissions(map[string]int{"cmd": RightsNone})
	if !r.PlayerHasCommandRights(p, "cmd") {
		t.Error("permissions NONE did not open the command over the override")
	}
	// World.json permissions beat the override (raising).
	r.SetPermissions(map[string]int{"cmd": RightsDeveloper})
	if r.PlayerHasCommandRights(p, "cmd") {
		t.Error("permissions DEVELOPER did not restrict over the override")
	}
	// No floor anywhere: everyone passes.
	if !r.PlayerHasCommandRights(p, "unregistered") {
		t.Error("unregistered base denied a player, want pass")
	}
	// SetCommandRights ignores empty names silently.
	r.SetCommandRights("  ", RightsDeveloper)
	if !r.PlayerHasCommandRights(p, "unregistered") {
		t.Error("empty-name override affected lookups")
	}
	// SetPermissions replaces: removing the entry restores the
	// override.
	r.SetPermissions(nil)
	if r.PlayerHasCommandRights(p, "cmd") {
		t.Error("clearing permissions did not restore the OWNER override")
	}
}

func TestEmitAnyHooksFirst(t *testing.T) {
	r := NewCommandRegistry()
	rights := RightsNone
	r.SetRightsSource(func(_ *Player) int { return rights })
	baseCalled := false
	r.Register("staffonly", func(e *CommandEvent) bool { baseCalled = true; return true }, RightsDeveloper, "")
	p := newProbePlayer()
	var sent []string
	r.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })

	// An any-command hook claiming the event skips base
	// handlers AND the rights gate (TS returns before both).
	r.SetCommandHooks(func(e *CommandEvent) { e.Handled = true })
	if !r.Emit(&CommandEvent{Player: p, Raw: "staffonly", Base: "staffonly", Parts: []string{"staffonly"}}) {
		t.Fatal("Emit = false, want true")
	}
	if baseCalled || len(sent) != 0 {
		t.Errorf("baseCalled=%v sent=%v, want neither (any-hook claimed it)", baseCalled, sent)
	}

	// An any-command hook that observes without handling lets
	// dispatch proceed to the rights gate.
	observed := false
	r.SetCommandHooks(func(e *CommandEvent) { observed = true })
	sent = nil
	if !r.Emit(&CommandEvent{Player: p, Raw: "staffonly", Base: "staffonly", Parts: []string{"staffonly"}}) {
		t.Fatal("Emit = false, want true (denial)")
	}
	if !observed || baseCalled {
		t.Errorf("observed=%v baseCalled=%v, want observed and gate-denied", observed, baseCalled)
	}
	if len(sent) != 1 {
		t.Errorf("sent = %v, want the denial message", sent)
	}
}

func TestEmitHandlerReturnFalseAndPanic(t *testing.T) {
	r := NewCommandRegistry()
	var order []string
	r.Register(probeCommand, func(e *CommandEvent) bool { order = append(order, "decline"); return false }, RightsNone, "")
	r.Register(probeCommand, func(e *CommandEvent) bool { order = append(order, "panic"); panic("boom") }, RightsNone, "")
	r.Register(probeCommand, func(e *CommandEvent) bool { order = append(order, "claim"); return true }, RightsNone, "")
	r.Register(probeCommand, func(e *CommandEvent) bool { order = append(order, "skipped"); return true }, RightsNone, "")
	p := newProbePlayer()
	if !r.Emit(&CommandEvent{Player: p, Raw: probeCommand, Base: probeCommand, Parts: []string{probeCommand}}) {
		t.Fatal("Emit = false, want true")
	}
	// Decline falls through, the panic is isolated, the claim
	// wins, and later handlers are skipped once handled.
	if !reflect.DeepEqual(order, []string{"decline", "panic", "claim"}) {
		t.Errorf("order = %v", order)
	}

	// All handlers declining (or panicking) leaves the event
	// unhandled, so Execute sends the unknown-command message.
	r2 := NewCommandRegistry()
	r2.Register(probeCommand, func(e *CommandEvent) bool { return false }, RightsNone, "")
	var sent []string
	r2.SetMessenger(func(_ *Player, msg string) { sent = append(sent, msg) })
	if !r2.Execute(p, probeCommand) {
		t.Fatal("Execute = false, want true")
	}
	if len(sent) != 1 || sent[0] != "This command does not exist. Try ::players." {
		t.Errorf("sent = %v, want the unknown-command message", sent)
	}
	if r.Emit(nil) {
		t.Error("Emit(nil) = true, want false")
	}
}

func TestRegisteredCommandsFilteringAndSorting(t *testing.T) {
	r := NewCommandRegistry()
	rights := RightsNone
	r.SetRightsSource(func(_ *Player) int { return rights })
	r.Register("zebra", func(e *CommandEvent) bool { return true }, RightsNone, "")
	r.Register("alpha", func(e *CommandEvent) bool { return true }, RightsModerator, "Alpha cmd")
	r.Register("mid", func(e *CommandEvent) bool { return true }, RightsNone, "Mid cmd")
	p := newProbePlayer()
	got := r.RegisteredCommands(p)
	want := []RegisteredCommand{{Command: "mid", Description: "Mid cmd"}, {Command: "zebra", Description: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RegisteredCommands(NONE) = %+v, want %+v", got, want)
	}
	rights = RightsModerator
	got = r.RegisteredCommands(p)
	want = []RegisteredCommand{{Command: "alpha", Description: "Alpha cmd"}, {Command: "mid", Description: "Mid cmd"}, {Command: "zebra", Description: ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("RegisteredCommands(MODERATOR) = %+v, want %+v", got, want)
	}
}

func TestCommandRegistryConcurrentDispatch(t *testing.T) {
	r := NewCommandRegistry()
	var mu sync.Mutex
	count := 0
	r.Register(probeCommand, func(e *CommandEvent) bool {
		mu.Lock()
		count++
		mu.Unlock()
		return true
	}, RightsNone, "")
	p := newProbePlayer()
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				r.Execute(p, "probe x")
			}
		}()
	}
	wg.Wait()
	if count != 400 {
		t.Errorf("handler ran %d times, want 400", count)
	}
}
