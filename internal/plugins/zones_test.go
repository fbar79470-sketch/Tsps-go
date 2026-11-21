package plugins

import (
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// zoneTestPlayer builds a bare player at loc. Movement is SetLocation:
// the zone driver reads the player's current location each emit.
func zoneTestPlayer(loc game.Location) *game.Player {
	return game.NewPlayer("zoneplayer", loc)
}

// zoneRect is the fixture rectangle: x 100-110, y 200-210, all planes.
func zoneRect() *Zone {
	return &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210}
}

func TestZoneRegistrationSeparateEntries(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "zone-plugin")
	zone := zoneRect()
	api.OnZoneEnter(zone, func(*ZoneEvent) {})
	api.OnZoneExit(zone, func(*ZoneEvent) {})
	if got := len(h.hooks.zoneHooks); got != 2 {
		t.Fatalf("zoneHooks = %d entries, want 2: enter and exit register independently", got)
	}
	enter, exit := h.hooks.zoneHooks[0], h.hooks.zoneHooks[1]
	if enter.onEnter == nil || enter.onExit != nil {
		t.Fatalf("enter entry has onEnter=%v onExit=%v, want only onEnter set", enter.onEnter != nil, enter.onExit != nil)
	}
	if exit.onExit == nil || exit.onEnter != nil {
		t.Fatalf("exit entry has onEnter=%v onExit=%v, want only onExit set", exit.onEnter != nil, exit.onExit != nil)
	}
	if enter.zone != zone || exit.zone != zone {
		t.Fatalf("entries carry zone %v/%v, want the registered pointer %v", enter.zone, exit.zone, zone)
	}
	if enter.pluginName != "zone-plugin" || exit.pluginName != "zone-plugin" {
		t.Fatalf("plugin names = %q/%q, want zone-plugin", enter.pluginName, exit.pluginName)
	}
}

func TestZoneRegistrationNilGuards(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "p")
	zone := zoneRect()
	handler := func(*ZoneEvent) {}
	api.OnZoneEnter(nil, handler)
	api.OnZoneEnter(zone, nil)
	api.OnZoneEnter(nil, nil)
	api.OnZoneExit(nil, handler)
	api.OnZoneExit(zone, nil)
	api.OnZoneExit(nil, nil)
	if got := len(h.hooks.zoneHooks); got != 0 {
		t.Fatalf("zoneHooks = %d entries, want 0: nil zones/handlers are silently ignored", got)
	}
	// A valid registration after the ignored ones still lands.
	api.OnZoneEnter(zone, handler)
	if got := len(h.hooks.zoneHooks); got != 1 {
		t.Fatalf("zoneHooks = %d entries, want 1", got)
	}
}

func TestIsInsideZone(t *testing.T) {
	zone := zoneRect()
	cases := []struct {
		name string
		zone *Zone
		loc  game.Location
		want bool
	}{
		{"center", zone, game.Location{X: 105, Y: 205, Z: 0}, true},
		{"min corner inclusive", zone, game.Location{X: 100, Y: 200, Z: 0}, true},
		{"max corner inclusive", zone, game.Location{X: 110, Y: 210, Z: 0}, true},
		{"one west", zone, game.Location{X: 99, Y: 205, Z: 0}, false},
		{"one east", zone, game.Location{X: 111, Y: 205, Z: 0}, false},
		{"one south", zone, game.Location{X: 105, Y: 199, Z: 0}, false},
		{"one north", zone, game.Location{X: 105, Y: 211, Z: 0}, false},
		{"nil levels all planes z2", zone, game.Location{X: 105, Y: 205, Z: 2}, true},
		{"empty levels match no plane", &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210, Levels: []int{}}, game.Location{X: 105, Y: 205, Z: 3}, false},
		{"levels match", &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210, Levels: []int{1}}, game.Location{X: 105, Y: 205, Z: 1}, true},
		{"levels mismatch", &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210, Levels: []int{1}}, game.Location{X: 105, Y: 205, Z: 0}, false},
		{"levels second match", &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210, Levels: []int{1, 2}}, game.Location{X: 105, Y: 205, Z: 2}, true},
		{"nil zone", nil, game.Location{X: 105, Y: 205, Z: 0}, false},
		{"inverted rect never matches", &Zone{MinX: 110, MaxX: 100, MinY: 200, MaxY: 210}, game.Location{X: 105, Y: 205, Z: 0}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isInsideZone(tc.zone, tc.loc); got != tc.want {
				t.Fatalf("isInsideZone(%v, %v) = %v, want %v", tc.zone, tc.loc, got, tc.want)
			}
		})
	}
}

func TestUpdatePlayerZonesTransitions(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	var events []string
	var gotZone *Zone
	var gotPlayer *game.Player
	api := newApi(h, "p")
	api.OnZoneEnter(zone, func(e *ZoneEvent) { events = append(events, "enter"); gotZone, gotPlayer = e.Zone, e.Player })
	api.OnZoneExit(zone, func(e *ZoneEvent) { events = append(events, "exit") })

	player := zoneTestPlayer(game.Location{X: 90, Y: 205, Z: 0})
	emit := func() { h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) }

	emit() // outside: silent
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	emit() // enter fires once
	emit() // stays silent while inside
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	emit() // exit fires once
	emit() // stays silent while outside
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	emit() // re-enter fires again

	want := []string{"enter", "exit", "enter"}
	if len(events) != len(want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	for i := range want {
		if events[i] != want[i] {
			t.Fatalf("events = %v, want %v", events, want)
		}
	}
	if gotZone != zone {
		t.Fatalf("event zone = %v, want the registered pointer %v", gotZone, zone)
	}
	if gotPlayer != player {
		t.Fatalf("event player = %v, want %v", gotPlayer, player)
	}
}

func TestZoneEnterOnlyAndExitOnlyEntries(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	var enters, exits int
	newApi(h, "enter-only").OnZoneEnter(zone, func(*ZoneEvent) { enters++ })
	newApi(h, "exit-only").OnZoneExit(zone, func(*ZoneEvent) { exits++ })

	player := zoneTestPlayer(game.Location{X: 90, Y: 205, Z: 0})
	emit := func() { h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) }
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	emit() // enter-only fires; exit-only stays silent
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	emit() // exit-only fires; enter-only stays silent
	if enters != 1 || exits != 1 {
		t.Fatalf("enters=%d exits=%d, want 1/1: an entry fires only its own direction", enters, exits)
	}
}

func TestZoneIdenticalRectanglesTrackIndependently(t *testing.T) {
	h, _ := newTestHub()
	var enters, exits int
	// Two enter entries and two exit entries over equal rectangles:
	// each is a separate hook index and fires on its own.
	for i := 0; i < 2; i++ {
		newApi(h, "p").OnZoneEnter(zoneRect(), func(*ZoneEvent) { enters++ })
		newApi(h, "p").OnZoneExit(zoneRect(), func(*ZoneEvent) { exits++ })
	}
	player := zoneTestPlayer(game.Location{X: 90, Y: 205, Z: 0})
	emit := func() { h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) }
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	emit()
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	emit()
	if enters != 2 || exits != 2 {
		t.Fatalf("enters=%d exits=%d, want 2/2: identical rectangles track independently by hook index", enters, exits)
	}
}

func TestZoneLevelsGateTransitions(t *testing.T) {
	h, _ := newTestHub()
	zone := &Zone{MinX: 100, MaxX: 110, MinY: 200, MaxY: 210, Levels: []int{1}}
	var events []string
	newApi(h, "p").OnZoneEnter(zone, func(*ZoneEvent) { events = append(events, "enter") })
	newApi(h, "p").OnZoneExit(zone, func(*ZoneEvent) { events = append(events, "exit") })

	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	emit := func() { h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) }
	emit() // inside the rect but on the wrong plane: silent
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 1})
	emit() // plane change into the zone: enter
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	emit() // plane change out: exit
	if len(events) != 2 || events[0] != "enter" || events[1] != "exit" {
		t.Fatalf("events = %v, want [enter exit]", events)
	}
}

func TestZoneUpdateRunsBeforeProcessHooks(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "p").OnZoneEnter(zoneRect(), func(*ZoneEvent) { order = append(order, "zone") })
	newApi(h, "p").OnPlayerProcess(func(*PlayerProcessEvent) { order = append(order, "process") })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(order) != 2 || order[0] != "zone" || order[1] != "process" {
		t.Fatalf("order = %v, want [zone process]: zone update runs before process hooks", order)
	}
}

func TestZoneUpdateRunsWithZeroProcessHooks(t *testing.T) {
	h, _ := newTestHub()
	entered := false
	newApi(h, "p").OnZoneEnter(zoneRect(), func(*ZoneEvent) { entered = true })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if !entered {
		t.Fatal("zone enter did not fire with zero process hooks registered")
	}
}

func TestZoneHookErrorIsolation(t *testing.T) {
	h, buf := newTestHub()
	var ran []string
	newApi(h, "boom").OnZoneEnter(zoneRect(), func(*ZoneEvent) {
		ran = append(ran, "boom")
		panic("kaboom")
	})
	newApi(h, "steady").OnZoneEnter(zoneRect(), func(*ZoneEvent) { ran = append(ran, "steady") })
	newApi(h, "p").OnPlayerProcess(func(*PlayerProcessEvent) { ran = append(ran, "process") })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	// The emit must return normally despite the panic.
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(ran) != 3 || ran[0] != "boom" || ran[1] != "steady" || ran[2] != "process" {
		t.Fatalf("ran = %v, want [boom steady process]: a throwing zone hook must not break the rest", ran)
	}
	logged := buf.String()
	if !strings.Contains(logged, "[plugins] zone enter hook threw (boom)") {
		t.Fatalf("log = %q, want the TS-shaped zone enter warning", logged)
	}
	if !strings.Contains(logged, "kaboom") {
		t.Fatalf("log = %q, want the recovered value", logged)
	}
	// The throwing hook's state change stuck: a second emit stays silent.
	ran = nil
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(ran) != 1 || ran[0] != "process" {
		t.Fatalf("second emit ran = %v, want [process]: no re-enter after a throwing enter", ran)
	}
}

func TestZoneExitHookErrorIsolation(t *testing.T) {
	h, buf := newTestHub()
	newApi(h, "boom").OnZoneExit(zoneRect(), func(*ZoneEvent) { panic("bye") })
	exited := false
	newApi(h, "steady").OnZoneExit(zoneRect(), func(*ZoneEvent) { exited = true })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) // enter (no enter hooks)
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) // exit: boom panics, steady runs
	if !exited {
		t.Fatal("steady exit hook did not run after the throwing exit hook")
	}
	if !strings.Contains(buf.String(), "[plugins] zone exit hook threw (boom)") {
		t.Fatalf("log = %q, want the TS-shaped zone exit warning", buf.String())
	}
}

func TestZoneDisconnectCleanup(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	var events []string
	newApi(h, "p").OnZoneEnter(zone, func(*ZoneEvent) { events = append(events, "enter") })
	newApi(h, "p").OnZoneExit(zone, func(*ZoneEvent) { events = append(events, "exit") })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) // enter
	if len(events) != 1 {
		t.Fatalf("events = %v, want [enter] before disconnect", events)
	}
	// Disconnect discards state silently: no exit hook fires.
	h.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: player, Username: "zoneplayer", Source: "removed"})
	if len(events) != 1 {
		t.Fatalf("events = %v after disconnect, want no exit fired", events)
	}
	// Moving out afterwards fires no exit (state is gone), and moving
	// back in fires enter again (fresh state).
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	player.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(events) != 2 || events[1] != "enter" {
		t.Fatalf("events = %v, want [enter enter]: disconnect discarded the active set", events)
	}
}

func TestZoneForgetPlayerZones(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	var events []string
	newApi(h, "p").OnZoneEnter(zone, func(*ZoneEvent) { events = append(events, "enter") })
	newApi(h, "p").OnZoneExit(zone, func(*ZoneEvent) { events = append(events, "exit") })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	h.ForgetPlayerZones(player)
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(events) != 1 {
		t.Fatalf("events = %v, want [enter]: forgetting discarded the set, so no exit fired", events)
	}
	// Forgetting a player with no state, and a nil player, are no-ops.
	h.ForgetPlayerZones(zoneTestPlayer(game.Location{}))
	h.ForgetPlayerZones(nil)
	var nilHub *Hub
	nilHub.ForgetPlayerZones(player)
}

func TestZoneLogoutDoesNotDiscard(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	exits := 0
	newApi(h, "p").OnZoneExit(zone, func(*ZoneEvent) { exits++ })
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player}) // inside, active set created
	h.EmitPlayerLogout(&PlayerLogoutEvent{Player: player, Username: "zoneplayer"})
	player.SetLocation(game.Location{X: 90, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if exits != 1 {
		t.Fatalf("exits = %d, want 1: logout must not discard zone state (only disconnect does)", exits)
	}
}

func TestZoneNoHooksCreatesNoState(t *testing.T) {
	h, _ := newTestHub()
	player := zoneTestPlayer(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
	if len(h.hooks.activeZones) != 0 {
		t.Fatalf("activeZones has %d entries, want 0: zero zone hooks means no state", len(h.hooks.activeZones))
	}
	// Nil-event and nil-player emits are no-ops, not panics.
	h.EmitPlayerProcess(nil)
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: nil})
	var nilHub *Hub
	nilHub.EmitPlayerProcess(&PlayerProcessEvent{Player: player})
}

func TestZonePerPlayerState(t *testing.T) {
	h, _ := newTestHub()
	zone := zoneRect()
	var entered []string
	newApi(h, "p").OnZoneEnter(zone, func(e *ZoneEvent) { entered = append(entered, e.Player.Username) })
	alice := game.NewPlayer("alice", game.Location{X: 105, Y: 205, Z: 0})
	bob := game.NewPlayer("bob", game.Location{X: 90, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: alice})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: bob})
	bob.SetLocation(game.Location{X: 105, Y: 205, Z: 0})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: bob})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: alice})
	if len(entered) != 2 || entered[0] != "alice" || entered[1] != "bob" {
		t.Fatalf("entered = %v, want [alice bob]: active sets are per player", entered)
	}
}
