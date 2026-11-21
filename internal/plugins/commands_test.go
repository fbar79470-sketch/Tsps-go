package plugins

import (
	"reflect"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

func commandTestPlayer() *game.Player {
	return game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
}

// TestApiRegisterCommandThroughLoad proves the hub path end to
// end: a plugin registering a command through Api during Load,
// dispatched through the hub's shared registry. The command is a
// test fixture, never content.
func TestApiRegisterCommandThroughLoad(t *testing.T) {
	hub := NewHub([]Plugin{{
		Name: "ProbePlugin",
		Register: func(api *Api) {
			api.RegisterCommand("probe", func(e *game.CommandEvent) bool {
				return true
			}, game.RightsNone, "Probe command")
		},
	}}, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "ProbePlugin" {
		t.Fatalf("Loaded() = %v, want [ProbePlugin]", got)
	}
	reg := hub.Commands()
	if reg == nil {
		t.Fatal("Commands() is nil")
	}
	got := reg.RegisteredCommands(commandTestPlayer())
	want := []game.RegisteredCommand{{Command: "probe", Description: "Probe command"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RegisteredCommands = %+v, want %+v", got, want)
	}
	// The same instance is shared: a second accessor call and a
	// second Api see the same registry.
	if hub.Commands() != reg {
		t.Fatal("Commands() returned different instances")
	}
	// Dispatch through the registry with a fake messenger.
	p := commandTestPlayer()
	var sent []string
	reg.SetMessenger(func(_ *game.Player, msg string) { sent = append(sent, msg) })
	if !reg.Execute(p, "probe arg") {
		t.Fatal("Execute = false, want true")
	}
	if len(sent) != 0 {
		t.Errorf("sent = %v, want none (command was handled)", sent)
	}
}

// TestApiRegisterCommandInvalidInputIgnored proves the TS
// no-throw semantics: invalid registrations are silently
// ignored AND the plugin still loads — unlike PersistAttribute,
// a bad registerCommand never fails initialization.
func TestApiRegisterCommandInvalidInputIgnored(t *testing.T) {
	hub := NewHub([]Plugin{{
		Name: "SloppyPlugin",
		Register: func(api *Api) {
			api.RegisterCommand("", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
			api.RegisterCommand("   ", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
			api.RegisterCommand("nilhandler", nil, game.RightsNone, "")
		},
	}}, HubOptions{})
	hub.Load()
	if got := hub.Loaded(); len(got) != 1 || got[0] != "SloppyPlugin" {
		t.Fatalf("Loaded() = %v, want [SloppyPlugin] (invalid registrations must not drop the plugin)", got)
	}
	if got := hub.Commands().RegisteredCommands(commandTestPlayer()); len(got) != 0 {
		t.Fatalf("RegisteredCommands = %+v, want none", got)
	}
}

// TestApiRegisterCommandPostLoadIgnored proves the load-phase
// gate: registering after Load() completes is ignored with a
// warning, like every other registration.
func TestApiRegisterCommandPostLoadIgnored(t *testing.T) {
	h, buf := newTestHub()
	h.Load()
	a := newApi(h, "Late")
	a.RegisterCommand("late", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
	if got := h.Commands().RegisteredCommands(commandTestPlayer()); len(got) != 0 {
		t.Fatalf("RegisteredCommands = %+v, want none (post-load registration ignored)", got)
	}
	if !strings.Contains(buf.String(), "registerCommand registration ignored") {
		t.Errorf("log = %q, want the post-load warning", buf.String())
	}
}

func TestApiGetRegisteredCommandsAndSetCommandRights(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Cmds")
	a.RegisterCommand("open", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "Open")
	a.RegisterCommand("staff", func(e *game.CommandEvent) bool { return true }, game.RightsModerator, "Staff")
	p := commandTestPlayer()

	got := a.GetRegisteredCommands(p)
	want := []game.RegisteredCommand{{Command: "open", Description: "Open"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetRegisteredCommands(NONE) = %+v, want %+v", got, want)
	}
	// SetCommandRights opens the staff command to everyone,
	// whoever registered it — with no load-phase gate.
	a.SetCommandRights("staff", game.RightsNone)
	got = a.GetRegisteredCommands(p)
	want = []game.RegisteredCommand{{Command: "open", Description: "Open"}, {Command: "staff", Description: "Staff"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("GetRegisteredCommands after override = %+v, want %+v", got, want)
	}
	// Empty names are silently ignored.
	a.SetCommandRights("  ", game.RightsDeveloper)
	if got := a.GetRegisteredCommands(p); len(got) != 2 {
		t.Fatalf("GetRegisteredCommands = %+v, want 2 after empty override", got)
	}
	// Nil-safety: a nil Api and a nil player yield nil.
	var nilApi *Api
	if nilApi.GetRegisteredCommands(p) != nil || a.GetRegisteredCommands(nil) == nil {
		// GetRegisteredCommands(nil player) reads RightsNone and
		// returns the open commands; only the nil-Api case is nil.
		t.Error("nil handling wrong: nil Api must yield nil, nil player must list open commands")
	}
}

func TestOnCommandFiring(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "Watcher")
	var observed []string
	a.OnCommand(func(e *game.CommandEvent) {
		observed = append(observed, e.Base)
	})
	// A second hook claims the command: later hooks still run
	// (run-all loop) but their wrappers skip them once handled.
	claimed := false
	a.OnCommand(func(e *game.CommandEvent) {
		claimed = true
		e.Handled = true
	})
	skipped := false
	a.OnCommand(func(e *game.CommandEvent) { skipped = true })
	// Nil handlers are ignored silently.
	a.OnCommand(nil)

	reg := h.Commands()
	reg.SetCommandHooks(h.EmitCommandHooks)
	p := commandTestPlayer()
	var sent []string
	reg.SetMessenger(func(_ *game.Player, msg string) { sent = append(sent, msg) })
	// No base handler exists, but the any-command hook claims
	// the command, so no unknown-command message is sent.
	if !reg.Execute(p, "anything") {
		t.Fatal("Execute = false, want true")
	}
	if !reflect.DeepEqual(observed, []string{"anything"}) || !claimed || skipped {
		t.Errorf("observed=%v claimed=%v skipped=%v, want observed+claimed, third hook wrapper-skipped", observed, claimed, skipped)
	}
	if len(sent) != 0 {
		t.Errorf("sent = %v, want none (any-command hook handled it)", sent)
	}
	// A panicking any-command hook is isolated: the emit
	// continues and the registry still dispatches.
	h2, _ := newTestHub()
	b := newApi(h2, "Panicker")
	b.OnCommand(func(e *game.CommandEvent) { panic("boom") })
	reg2 := h2.Commands()
	reg2.SetCommandHooks(h2.EmitCommandHooks)
	reg2.Register("probe", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
	if !reg2.Execute(p, "probe") {
		t.Fatal("Execute after panicking hook = false, want true")
	}
}

func TestEmitSocialPacket(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ChatWatcher")
	p := commandTestPlayer()

	// No hooks: not handled.
	if h.EmitSocialPacket(&SocialPacketEvent{Player: p, Packet: game.SocialPacket{Kind: SocialPacketPublicChat, Text: "hi"}}) {
		t.Error("EmitSocialPacket with no hooks = true, want false")
	}
	// Entry guards: nil event, nil player, pre-handled.
	if h.EmitSocialPacket(nil) || h.EmitSocialPacket(&SocialPacketEvent{Packet: game.SocialPacket{Kind: SocialPacketPublicChat}}) ||
		h.EmitSocialPacket(&SocialPacketEvent{Player: p, Handled: true}) {
		t.Error("EmitSocialPacket entry guards failed: nil/nil-player/pre-handled must return false")
	}

	var order []string
	a.OnSocialPacket(func(e *SocialPacketEvent) { order = append(order, "observe") })
	a.OnSocialPacket(func(e *SocialPacketEvent) { order = append(order, "claim"); e.Handled = true })
	a.OnSocialPacket(func(e *SocialPacketEvent) { order = append(order, "skipped") })
	a.OnSocialPacket(nil) // nil handler ignored
	event := &SocialPacketEvent{Player: p, Packet: game.SocialPacket{Kind: SocialPacketPublicChat, Text: "hello", AutoChat: true}}
	if !h.EmitSocialPacket(event) {
		t.Fatal("EmitSocialPacket = false, want true")
	}
	// Short-circuit: the third hook never runs once handled.
	if !reflect.DeepEqual(order, []string{"observe", "claim"}) {
		t.Errorf("order = %v, want [observe claim]", order)
	}
	if !event.Handled {
		t.Error("event.Handled = false after claim")
	}
	// A nil Hub returns false.
	var nilHub *Hub
	if nilHub.EmitSocialPacket(event) {
		t.Error("nil Hub EmitSocialPacket = true, want false")
	}
}

func TestCommandPermissions(t *testing.T) {
	h, buf := newTestHubWithConfig(map[string]any{
		"commands:permissions": map[string]any{
			"items":       "NONE",
			"::teleports": "owner",
			"  Yell  ":    " moderator ",
			"bad-rights":  "WIZARD",
			"bad-type":    3,
			"":            "NONE",
			"::":          "NONE",
		},
	})
	got := h.CommandPermissions()
	want := map[string]int{
		"items":     game.RightsNone,
		"teleports": game.RightsOwner,
		"yell":      game.RightsModerator,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CommandPermissions() = %v, want %v", got, want)
	}
	logged := buf.String()
	for _, fragment := range []string{"bad-rights", "bad-type"} {
		if !strings.Contains(logged, fragment) {
			t.Errorf("log = %q, want a warning mentioning %s", logged, fragment)
		}
	}
	// ":: items" (space after the prefix) keeps its space: TS
	// strips "::" after trimming but does not re-trim.
	h2, _ := newTestHubWithConfig(map[string]any{
		"commands:permissions": map[string]any{":: items": "NONE"},
	})
	if got := h2.CommandPermissions(); !reflect.DeepEqual(got, map[string]int{" items": game.RightsNone}) {
		t.Errorf("CommandPermissions() = %v, want {\" items\": 0}", got)
	}
	// Missing key, non-object value, and nil hub: empty, silent.
	h3, buf3 := newTestHub()
	if got := h3.CommandPermissions(); len(got) != 0 {
		t.Errorf("CommandPermissions() missing key = %v, want empty", got)
	}
	if buf3.Len() != 0 {
		t.Errorf("log = %q, want silent", buf3.String())
	}
	h4, _ := newTestHubWithConfig(map[string]any{"commands:permissions": "not-an-object"})
	if got := h4.CommandPermissions(); len(got) != 0 {
		t.Errorf("CommandPermissions() non-object = %v, want empty", got)
	}
	var nilHub *Hub
	if got := nilHub.CommandPermissions(); len(got) != 0 {
		t.Errorf("nil hub CommandPermissions() = %v, want empty", got)
	}
	// Permissions installed on the registry gate dispatch.
	h5, _ := newTestHubWithConfig(map[string]any{
		"commands:permissions": map[string]any{"probe": "OWNER"},
	})
	reg := h5.Commands()
	reg.Register("probe", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
	reg.SetPermissions(h5.CommandPermissions())
	if reg.PlayerHasCommandRights(commandTestPlayer(), "probe") {
		t.Error("permissions OWNER did not restrict a NONE player")
	}
}
