package main

import (
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/session"
)

// Interface-action wiring: the cmd/server adapter between the
// interface-action / prompt listeners (internal/listeners), the
// plugin hub (internal/plugins), and the per-player session
// (internal/session). The listeners never import the plugins or
// session packages; this package imports all three, as the
// object and combat adapters do.
//
// The coordinator wires it in AddPlayer's Deps block, after
// InstallShopListeners:
//
//	InstallInterfaceActionListeners(&deps, a.plugins)
//
// A nil hub leaves the InterfaceHooks seams unset: the
// listeners degrade to their documented no-op/drop behavior
// (clicks unhandled, custom events unemitted). The
// InterfaceSends seams install regardless of the hub: they
// assert the session methods per call, and a nil or
// session-less player degrades to the listeners' fallbacks.

// InstallInterfaceActionListeners installs the interface-action
// hub emits and the session-side interface sends on deps.
func InstallInterfaceActionListeners(deps *listeners.Deps, hub *plugins.Hub) {
	if deps == nil {
		return
	}
	deps.InterfaceSends = listeners.InterfaceSends{
		AbortInputPrompt: func(p *game.Player) {
			if sess := interfaceSession(p); sess != nil {
				sess.AbortInputPrompt()
			}
		},
		CloseInterface: func(p *game.Player, groupID int) {
			if sess := interfaceSession(p); sess != nil {
				sess.CloseInterface(groupID)
			}
		},
		CloseInterruptibleInterfaces: func(p *game.Player) {
			if sess := interfaceSession(p); sess != nil {
				sess.CloseInterruptibleInterfaces()
			}
		},
		HasInterruptibleInterface: func(p *game.Player) bool {
			if sess := interfaceSession(p); sess != nil {
				return sess.HasInterruptibleInterface()
			}
			return false
		},
	}
	if hub == nil {
		return
	}
	deps.InterfaceHooks = listeners.InterfaceHooks{
		RefusesWhileBusy: hub.RefusesWhileBusy,
		EmitInterfaceActionClick: func(ev *listeners.InterfaceActionClickEvent) bool {
			if ev == nil {
				return false
			}
			hubEvent := translateInterfaceActionClickEvent(ev)
			handled := hub.EmitInterfaceActionClick(hubEvent)
			ev.Handled = hubEvent.Handled
			return handled || hubEvent.Handled
		},
		EmitWidgetAction: func(ev *listeners.WidgetActionEvent) bool {
			if ev == nil {
				return false
			}
			hubEvent := &plugins.WidgetActionEvent{Player: ev.Player, Packet: ev.Packet}
			hub.EmitCustomEvent("player:widget-action", hubEvent)
			ev.Handled = hubEvent.Handled
			return hubEvent.Handled
		},
		EmitWorldInput: func(ev *listeners.WorldInputEvent) bool {
			if ev == nil {
				return false
			}
			hubEvent := &plugins.WorldInputEvent{Player: ev.Player, PacketType: ev.PacketType, Packet: ev.Packet}
			hub.EmitCustomEvent("player:world-input", hubEvent)
			ev.Handled = hubEvent.Handled
			return hubEvent.Handled
		},
		EmitInterfaceClose: func(ev *listeners.InterfaceCloseEvent) bool {
			if ev == nil {
				return false
			}
			hubEvent := &plugins.InterfaceCloseEvent{Player: ev.Player}
			hub.EmitCustomEvent("interface:close", hubEvent)
			ev.Handled = hubEvent.Handled
			return hubEvent.Handled
		},
	}
}

// interfaceSession returns the player's live session, or nil
// when the player has none attached or the session is not a
// *session.Session (tests attach seam fakes).
func interfaceSession(p *game.Player) *session.Session {
	if p == nil {
		return nil
	}
	sess, _ := p.Session.(*session.Session)
	return sess
}

// translateInterfaceActionClickEvent copies a listener-facing
// click event into the hub-facing form, field for field. The
// two structs mirror each other (same metadata set); a field
// added to one must be added here, or content sees a zero.
func translateInterfaceActionClickEvent(ev *listeners.InterfaceActionClickEvent) *plugins.InterfaceActionClickEvent {
	return &plugins.InterfaceActionClickEvent{
		Player:            ev.Player,
		ButtonID:          ev.ButtonID,
		Action:            ev.Action,
		OpID:              ev.OpID,
		HasOpID:           ev.HasOpID,
		GroupID:           ev.GroupID,
		HasGroupID:        ev.HasGroupID,
		ChildID:           ev.ChildID,
		HasChildID:        ev.HasChildID,
		ItemID:            ev.ItemID,
		HasItemID:         ev.HasItemID,
		Slot:              ev.Slot,
		HasSlot:           ev.HasSlot,
		Option:            ev.Option,
		HasOption:         ev.HasOption,
		SubOpID:           ev.SubOpID,
		HasSubOpID:        ev.HasSubOpID,
		TargetWidgetID:    ev.TargetWidgetID,
		HasTargetWidgetID: ev.HasTargetWidgetID,
		TargetSlot:        ev.TargetSlot,
		HasTargetSlot:     ev.HasTargetSlot,
		TargetItemID:      ev.TargetItemID,
		HasTargetItemID:   ev.HasTargetItemID,
		SourceWidgetID:    ev.SourceWidgetID,
		HasSourceWidgetID: ev.HasSourceWidgetID,
		SourceSlot:        ev.SourceSlot,
		HasSourceSlot:     ev.HasSourceSlot,
		SourceItemID:      ev.SourceItemID,
		HasSourceItemID:   ev.HasSourceItemID,
		ArgsData:          ev.ArgsData,
		ScriptTrigger:     ev.ScriptTrigger,
		Handled:           ev.Handled,
	}
}
