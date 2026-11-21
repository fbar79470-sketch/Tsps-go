package main

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// TestInstallInterfaceActionListenersNilHub pins the nil-hub
// degradation: the hub seams stay unset while the session sends
// install (they assert per call and no-op without a session).
func TestInstallInterfaceActionListenersNilHub(t *testing.T) {
	var deps listeners.Deps
	InstallInterfaceActionListeners(&deps, nil)
	hooks := deps.InterfaceHooks
	if hooks.RefusesWhileBusy != nil || hooks.EmitInterfaceActionClick != nil ||
		hooks.EmitWidgetAction != nil || hooks.EmitWorldInput != nil ||
		hooks.EmitInterfaceClose != nil {
		t.Fatal("nil hub installed a hub seam")
	}
	sends := deps.InterfaceSends
	if sends.AbortInputPrompt == nil || sends.CloseInterface == nil ||
		sends.CloseInterruptibleInterfaces == nil || sends.HasInterruptibleInterface == nil {
		t.Fatal("nil hub left a send seam unset")
	}
	p := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	sends.AbortInputPrompt(p)
	sends.CloseInterface(p, 300)
	sends.CloseInterruptibleInterfaces(p)
	if sends.HasInterruptibleInterface(p) {
		t.Fatal("session-less player reports an interruptible interface")
	}
}

// TestInstallInterfaceActionListenersTranslates pins the
// listener-to-hub event translation: every metadata field
// reaches the hook, and the hook's claim copies back.
func TestInstallInterfaceActionListenersTranslates(t *testing.T) {
	var got *plugins.InterfaceActionClickEvent
	hub := plugins.NewHub([]plugins.Plugin{{
		Name: "InterfaceProbe",
		Register: func(api *plugins.Api) {
			api.RefuseWhileBusy([]int{buttonID})
			api.OnInterfaceActionButton([]int{buttonID}, func(ev *plugins.InterfaceActionClickEvent) bool {
				got = ev
				return true
			})
		},
	}}, plugins.HubOptions{Logger: discardLogger()})
	hub.Load()

	var deps listeners.Deps
	InstallInterfaceActionListeners(&deps, hub)
	p := game.NewPlayer("testplayer", game.Location{X: 3000, Y: 3000})
	ev := &listeners.InterfaceActionClickEvent{
		Player:   p,
		ButtonID: buttonID,
		Action:   3,
		GroupID:  300, HasGroupID: true,
		ChildID: 7, HasChildID: true,
		Slot: 2, HasSlot: true,
		Option: "Buy", HasOption: true,
		SubOpID: 1, HasSubOpID: true,
		ArgsData:      []byte{9},
		ScriptTrigger: true,
	}
	if !deps.InterfaceHooks.EmitInterfaceActionClick(ev) {
		t.Fatal("emit returned unhandled")
	}
	if !ev.Handled {
		t.Fatal("Handled did not copy back to the listener event")
	}
	if got == nil {
		t.Fatal("hook never ran")
	}
	if got.Player != p || got.ButtonID != buttonID || got.Action != 3 ||
		got.GroupID != 300 || !got.HasGroupID || got.ChildID != 7 ||
		got.Slot != 2 || !got.HasSlot || got.Option != "Buy" ||
		got.SubOpID != 1 || len(got.ArgsData) != 1 || !got.ScriptTrigger {
		t.Fatalf("translated event = %+v, want all fields copied", got)
	}
	if !deps.InterfaceHooks.RefusesWhileBusy(buttonID) {
		t.Fatal("RefusesWhileBusy not wired to the hub")
	}
}

const buttonID = (300 << 16) | 7
