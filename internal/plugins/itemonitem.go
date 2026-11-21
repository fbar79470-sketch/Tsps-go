// Item-on-item hooks: the Go form of the onItemOnItem family
// of PluginApi (PluginTypes.ts:1197-1204) that the engine can
// fire since phase 33: onItemOnItem (plain and named forms),
// plus the Hub emit EmitItemOnItem. There is NO Api
// emitItemOnItem member in PluginTypes.ts (only
// emitItemOnObject, at 1407) and no createApi passthrough
// for it, so content registers handlers and the listener
// fires the hub emit; no Api.EmitItemOnItem is invented.
//
// The family rides the INVENTORY_USE_ON packet (opcode 241),
// not a packet of its own: ClientProtocol.ts:912-925 decodes
// kind 4 (the "inventory" target) as an inventory_use_on with
// an inventory target, and NetworkBuilder.ts:839-844 dispatches
// it to UseItemPacketListener.itemOnItem
// (UseItemPacketListener.ts:15-37). The Go decode already
// handles kind 4 (protocol.DecodeInventoryUseOn); the listener
// route lives in internal/listeners/useitem.go and the
// cmd/server adapter in objectlistenerwiring.go.
//
// The emit ports PluginManager.emitItemOnItem
// (PluginManager.ts:1925-1930) with label "item_on_item":
// every hook runs in registration order; each hook's
// registration wrapper skips once the event is handled. A
// nil Hub, a Hub without a registry, or a nil event returns
// false (the nil-event guard is Go-only; TS has no nil-event
// equivalent). The production call site is the
// inventory-use-on listener's item-on-item route, through
// the cmd/server adapter; Api.EmitItemOnItem exposes the
// same emit to content (PluginTypes.ts:1407, the
// emitItemOnItem member beside emitItemOnObject).

package plugins

import (
	"tsps-go/internal/game"
)

// PluginItemOnItemEvent is the payload of the onItemOnItem
// hooks. It mirrors PluginItemOnItemEvent
// (PluginTypes.ts:722-731): the player, the used item (the
// inventory stack as it stood when the use was clicked — TS
// captures the Item reference; Go carries a value copy, so a
// handler that mutates an item writes it back through the
// container at UsedItemSlot / UsedWithItemSlot), the used
// item's id and slot, the used-with item, its id and slot,
// and the handled flag. Both items are inventory items: the
// packet's inventory target carries two inventory slots.
type PluginItemOnItemEvent struct {
	Player           *game.Player
	UsedItem         game.Item
	UsedItemID       int
	UsedItemSlot     int
	UsedWithItem     game.Item
	UsedWithItemID   int
	UsedWithItemSlot int
	Handled          bool
}

// ItemOnItemHandler is a named onItemOnItem handler: false
// falls through, any other result claims the use (the emit
// sets Handled). It mirrors the `void | boolean` handler
// shape of the TS named overload (PluginTypes.ts:1199-1204),
// where a return of exactly false falls through and every
// other return (including undefined) claims the event
// (PluginManager.ts:4139). Plain onItemOnItem handlers
// instead return nothing and claim by setting event.Handled.
type ItemOnItemHandler func(*PluginItemOnItemEvent) bool

// OnItemOnItem registers a plain item-on-item hook,
// mirroring the first onItemOnItem overload
// (PluginTypes.ts:1197). The handler claims the use by
// setting event.Handled; unclaimed uses fall through to
// the next hook in registration order (EmitItemOnItem) and,
// when no hook claims the use, silently do nothing —
// itemOnItem has no "Nothing interesting happens."
// fallback (UseItemPacketListener.ts:15-37 ignores the
// emit's return). filter is optional (nil means no
// filter): a noted restriction (see ItemUseFilter). A nil
// handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:4107-4110).
func (a *Api) OnItemOnItem(handler func(*PluginItemOnItemEvent), filter *ItemUseFilter) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemOnItem") {
		return
	}
	noted, _ := itemUseFilterValues(filter)
	a.hub.hooks.addItemOnItem(hookEntry[*PluginItemOnItemEvent]{pluginName: a.name, handler: func(event *PluginItemOnItemEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		if noted != nil {
			if _, isNoted := a.hub.itemInfo(event.UsedItemID); isNoted != *noted {
				return
			}
			if _, isNoted := a.hub.itemInfo(event.UsedWithItemID); isNoted != *noted {
				return
			}
		}
		handler(event)
	}})
}

// OnItemOnItemNamed registers a named item-on-item hook,
// mirroring the second onItemOnItem overload
// (PluginTypes.ts:1199-1204): exact item-name matching in
// EITHER order (the event's items retain their original
// order), and the handler returns false to fall through.
// filter is optional (nil means no filter): a noted
// restriction applies to BOTH items (PluginManager.ts:4132:
// the hook is skipped when either item's noted state
// differs from the filter). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:4126-4128).
func (a *Api) OnItemOnItemNamed(itemName, otherItemName string, handler ItemOnItemHandler, filter *ItemUseFilter) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemOnItem") {
		return
	}
	noted, _ := itemUseFilterValues(filter)
	a.hub.hooks.addItemOnItem(hookEntry[*PluginItemOnItemEvent]{pluginName: a.name, handler: func(event *PluginItemOnItemEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		if noted != nil {
			if _, isNoted := a.hub.itemInfo(event.UsedItemID); isNoted != *noted {
				return
			}
			if _, isNoted := a.hub.itemInfo(event.UsedWithItemID); isNoted != *noted {
				return
			}
		}
		usedName, _ := a.hub.itemInfo(event.UsedItemID)
		withName, _ := a.hub.itemInfo(event.UsedWithItemID)
		if !((usedName == itemName && withName == otherItemName) ||
			(usedName == otherItemName && withName == itemName)) {
			return
		}
		if handler(event) {
			event.Handled = true
		}
	}})
}

// EmitItemOnItem runs an item-on-item use through the
// registered hooks and reports whether one claimed it,
// mirroring PluginManager.emitItemOnItem
// (PluginManager.ts:1925-1930) with label "item_on_item":
// every hook runs in registration order; each hook's
// registration wrapper skips once the event is handled.
// A nil Hub, a Hub without a registry, or a nil event
// returns false (the nil-event guard is Go-only; TS has
// no nil-event equivalent).
//
// The production call site is the inventory-use-on
// listener's item-on-item route. There is no Api emit
// passthrough: PluginTypes.ts declares no emitItemOnItem
// member (only emitItemOnObject, at 1407).
func (h *Hub) EmitItemOnItem(event *PluginItemOnItemEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	emitHooks(h.Logger(), "item_on_item", h.hooks.snapshotItemOnItem(), event)
	return event.Handled
}
