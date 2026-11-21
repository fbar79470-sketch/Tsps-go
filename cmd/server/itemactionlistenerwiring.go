package main

import (
	"tsps-go/internal/cache"
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// Item-action-listener wiring: the cmd/server adapter between the
// item-action listener (internal/listeners), the definition Store /
// cache facade, and the plugin hub (internal/plugins). The
// listeners never import the definition or plugins packages; the
// hub never imports listener types; this package imports all four,
// as the object-listener adapter does.
//
// The coordinator wires it in two places in server.go (both edits
// are the coordinator's — this file only defines the adapters):
//
//   - In buildServer, next to InstallObjectResolvers (which
//     installs the hub's item name/noted resolver):
//     InstallItemActionResolvers(hub, store, defs) for the hub's
//     item-actions resolver, so named onItemAction matching can
//     read the inventory actions.
//   - In AddPlayer's Deps block, next to InstallObjectListeners:
//     InstallItemActionListeners(&deps, a.plugins).
//
// A nil hub degrades each adapter to its documented no-op: the
// listener emits run their no-hooks defaults and the resolver
// install does nothing.

// ItemActionHooks adapts the hub's item-action emits to
// listeners.ItemActionHooks. A nil hub yields the zero value:
// every emit degrades to its no-hooks default. The listener
// event struct and the hub event struct share the same fields,
// so the translation is a field copy; the Handled claim is
// written back onto the listener event.
func ItemActionHooks(hub *plugins.Hub) listeners.ItemActionHooks {
	if hub == nil {
		return listeners.ItemActionHooks{}
	}
	return listeners.ItemActionHooks{
		EmitItemAction: func(ev *listeners.ItemActionEvent) bool {
			hubEv := &plugins.PluginItemActionEvent{
				Player:      ev.Player,
				InterfaceID: ev.InterfaceID,
				ItemID:      ev.ItemID,
				Slot:        ev.Slot,
				ClickType:   ev.ClickType,
				Option:      ev.Option,
				SubOpID:     ev.SubOpID,
				SubOption:   ev.SubOption,
				Handled:     ev.Handled,
			}
			handled := hub.EmitItemAction(hubEv)
			ev.Handled = hubEv.Handled
			return handled
		},
		EmitCanUseItem: func(player *game.Player, itemID int, action, option string) *bool {
			return hub.EmitCanUseItem(player, itemID, action, option)
		},
	}
}

// InstallItemActionListeners fills the item-action seams on
// deps: the hub hook adapters. A nil deps is a no-op; a nil hub
// yields the zero (no-hooks) hook set. The coordinator calls it
// once per entering player, in AddPlayer's Deps block.
func InstallItemActionListeners(deps *listeners.Deps, hub *plugins.Hub) {
	if deps == nil {
		return
	}
	deps.ItemActionHooks = ItemActionHooks(hub)
}

// InstallItemActionResolvers installs the hub's item-actions
// resolver over the Store/cache facade, reusing the
// storeItemDefinitions InventoryActions read (the action lists
// live on the cache ObjType, not on the JSON-layered
// ItemDefinition — see containerswiring.go). A nil hub or nil
// store installs nothing: named onItemAction matching then
// reads the ObjType defaults (only the Drop option can match).
// defs is the cache facade the action-list methods read (nil in
// NoCache mode: action lists read the ObjType defaults).
func InstallItemActionResolvers(hub *plugins.Hub, store *definition.Store, defs *cache.CacheDefinitions) {
	if hub == nil || store == nil {
		return
	}
	src := storeItemDefinitions{store: store, defs: defs}
	hub.SetItemActionsResolver(src.InventoryActions)
}
