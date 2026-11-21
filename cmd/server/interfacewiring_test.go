package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

func TestInstallInterfaceRuntimeWiresPlayerAndSession(t *testing.T) {
	p := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200})
	sess := session.NewSession(session.NewHeadlessChannel(""))
	p.Session = sess
	installInterfaceRuntime(sess, p, nil, nil, nil)

	// SendInterface records the id on the player through the dep.
	sess.SendInterface(300)
	if got := p.GetInterfaceId(); got != 300 {
		t.Fatalf("GetInterfaceId() = %d, want 300", got)
	}
	// The player-side close gate reaches the session through the
	// InterfaceRuntimeSender extension and resets the id.
	p.CloseInterruptibleInterfaces()
	if got := p.GetInterfaceId(); got != -1 {
		t.Fatalf("GetInterfaceId() = %d, want -1 after close", got)
	}
	// No layouts installed: configured references are unknown.
	if _, err := sess.SendConfiguredInterface("bank"); err == nil {
		t.Fatal("SendConfiguredInterface without layouts: error = nil")
	}
}

func TestInstallInterfaceRuntimeEmitsThroughHub(t *testing.T) {
	var events []string
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "test",
		Register: func(api *plugins.Api) {
			api.OnCustomEvent("interface:changed", func(event any) { events = append(events, "changed") })
			api.OnCustomEvent("interface:closed", func(event any) { events = append(events, "closed") })
			api.OnCustomEvent("interface:opened", func(event any) { events = append(events, "opened") })
		},
	}}, plugins.HubOptions{})
	hub.Load()
	p := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200})
	sess := session.NewSession(session.NewHeadlessChannel(""))
	p.Session = sess
	installInterfaceRuntime(sess, p, nil, nil, hub)

	sess.SendInterface(300)
	// setInterfaceId(-1 -> 300) emits changed; the mount emits
	// opened.
	if len(events) != 2 || events[0] != "changed" || events[1] != "opened" {
		t.Fatalf("events = %v, want [changed opened]", events)
	}
	events = nil
	sess.SendInterface(301)
	// Replace: closed(300), changed, opened.
	if len(events) != 3 || events[0] != "closed" || events[1] != "changed" || events[2] != "opened" {
		t.Fatalf("events = %v, want [closed changed opened]", events)
	}
}

func TestInstallInterfaceRuntimeNilTolerant(t *testing.T) {
	p := game.NewPlayer("alice", game.Location{X: 3200, Y: 3200})
	sess := session.NewSession(session.NewHeadlessChannel(""))
	// None of these may panic.
	installInterfaceRuntime(nil, p, nil, nil, nil)
	installInterfaceRuntime(sess, nil, nil, nil, nil)
	installInterfaceRuntime(sess, p, nil, nil, nil)
}
