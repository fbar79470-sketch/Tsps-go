package main

import (
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

func TestWireCommandRegistry(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "Probe",
		Register: func(api *plugins.Api) {
			api.RegisterCommand("probe", func(e *game.CommandEvent) bool { return true }, game.RightsNone, "")
			api.OnCommand(func(e *game.CommandEvent) {
				if e.Base == "claimed" {
					e.Handled = true
				}
			})
		},
	}}, plugins.HubOptions{
		Logger:       log.Default(),
		PluginConfig: map[string]any{"commands:permissions": map[string]any{"probe": "OWNER"}},
	})
	hub.Load()

	var sent []string
	rights := game.RightsNone
	reg := wireCommandRegistry(hub,
		func(_ *game.Player, msg string) { sent = append(sent, msg) },
		func(_ *game.Player) int { return rights })
	if reg == nil || reg != hub.Commands() {
		t.Fatal("wireCommandRegistry did not return the hub's registry")
	}
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})

	// Permissions installed: world.json OWNER floor denies a
	// NONE player, and the denial goes through the messenger.
	if !reg.Execute(p, "probe") {
		t.Fatal("Execute = false, want true (denial ran)")
	}
	if len(sent) != 1 || sent[0] != "You do not have permission to use this command." {
		t.Fatalf("sent = %v, want the denial message", sent)
	}
	// Raising the rights source opens the command.
	rights = game.RightsOwner
	sent = nil
	if !reg.Execute(p, "probe") || len(sent) != 0 {
		t.Fatalf("Execute as OWNER dispatched wrong: sent = %v", sent)
	}
	// Any-command hooks wired: the OnCommand hook claims
	// "claimed" with no base handler and no message.
	sent = nil
	if !reg.Execute(p, "claimed") || len(sent) != 0 {
		t.Fatalf("Execute(claimed) wrong: sent = %v", sent)
	}
	// The registry satisfies the listener seam shape: a
	// DispatchCommand method taking (player, raw text).
	var _ interface{ DispatchCommand(*game.Player, string) } = reg

	if wireCommandRegistry(nil, nil, nil) != nil {
		t.Error("wireCommandRegistry(nil hub) != nil")
	}
}

func TestSocialPacketIntercept(t *testing.T) {
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "Watcher",
		Register: func(api *plugins.Api) {
			api.OnSocialPacket(func(e *plugins.SocialPacketEvent) {
				if e.Packet.Text == "stop" {
					e.Handled = true
				}
			})
		},
	}}, plugins.HubOptions{Logger: log.Default()})
	hub.Load()
	intercept := socialPacketIntercept(hub)
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
	if !intercept(p, "stop", false) {
		t.Error("intercept(stop) = false, want true")
	}
	if intercept(p, "hello", true) {
		t.Error("intercept(hello) = true, want false")
	}
	if socialPacketIntercept(nil)(p, "stop", false) {
		t.Error("nil-hub intercept = true, want false")
	}
}
