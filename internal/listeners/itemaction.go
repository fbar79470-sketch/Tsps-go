// Item action: ItemActionPacketListener.ts (147 lines, commit
// bfd81e3). handleAction (lines 78-108) reads the item, emits a
// plugin event, and — for first-action clicks — handleFirstAction
// (lines 110-147) falls back to Equip for wieldables. The plugin
// event (emitItemAction / emitCanUseItem, PluginManager.ts:
// 1654-1665, 2016-2043) fires through the ItemActionHooks seam
// in Deps, wired to the hub by the cmd/server adapter (see
// itemactionlistenerwiring.go); with no hooks installed the
// emits degrade to their no-hooks defaults (gate proceeds,
// event unhandled), exactly as TS behaves with no hooks
// registered. The equip fallback reuses equip() from equip.go;
// every other plugin action stays a drop with ReasonItemAction.
// resolveInventoryWidgetAction (lines 40-62) and isDropOption
// (lines 64-76) are ported for the dispatcher.
//
// GAPS, documented at their sites:
//   - The widget-action emit sites (NetworkBuilder.ts:996-1011
//     equipment branch, 1103-1114 subOpId branch): the Go
//     widget-action path has no equipment-slot branch and no
//     subOpId emit, so SubOpID is 0 and SubOption "" from both
//     emit sites here; the hub's subOption fill-in and the
//     item:operated custom emit are likewise unported (see
//     internal/plugins/itemaction.go).
//   - clearPendingAction (Player.ts:860-868): only its walk-to
//     cancellation is ported (MovementQueue.CancelWalkTo). Its
//     combat clearInteraction, following clear, and plugin
//     pending-action event are gaps: no combat, following, or
//     plugin systems exist.
//   - The 9520 Wilderness message IS sent (it is unconditional
//     in TS: no Wilderness check guards it). The 2545
//     already-unlocked message IS sent when the
//     bounty-hunter:target-teleport-unlocked attribute is true;
//     its busy() branch stays a gap (busy() degenerates to the
//     hitpoints term, already passed, so the branch is
//     unreachable — no busy state exists to evaluate).
//   - sendInterfaceRemoval: no interface system exists.

package listeners

import (
	"tsps-go/internal/game"
)

// ItemActionEvent is the listener-side item-action event: the
// Go form of PluginItemActionEvent (PluginTypes.ts:831-843)
// as the listener builds it. It mirrors the hub event field
// for field (see internal/plugins/itemaction.go); the
// cmd/server adapter translates it one-to-one and copies the
// Handled claim back. SubOpID is 0 and SubOption "" from both
// emit sites here (see the file header): the widget-action
// subOpId path that supplies them has no Go emit site.
type ItemActionEvent struct {
	Player      *game.Player
	InterfaceID int
	ItemID      int
	Slot        int
	ClickType   int
	Option      string
	SubOpID     int
	SubOption   string
	Handled     bool
}

// ItemActionHooks carries the item-action hub emits as func
// fields, following the ObjectHooks precedent
// (objectaction.go). Every field is optional: nil means no
// hooks, which degenerates to the no-hooks defaults (the gate
// proceeds, the event is unhandled). The coordinator wires
// these to the hub in cmd/server/itemactionlistenerwiring.go.
type ItemActionHooks struct {
	// EmitItemAction fires the onItemAction /
	// onItemFirstAction hooks (main, then fallback) and
	// reports whether one claimed the click. The event's
	// Handled flag carries the same claim; either one
	// claiming is enough.
	EmitItemAction func(*ItemActionEvent) bool
	// EmitCanUseItem fires the onCanUseItem gate hooks and
	// reports the first decided answer: nil is undecided
	// (proceed), non-nil false refuses the click. Action is
	// "action" at this call site; option is the clicked menu
	// option ("" when the packet carried none).
	EmitCanUseItem func(player *game.Player, itemID int, action, option string) *bool
}

// itemActionResult is the result of handleAction: TS returns a
// boolean (handled), but the packet Outcome needs to distinguish
// a gate refusal from "reached the plugin event unhandled".
type itemActionResult int

const (
	// itemActionRefused: a gate or item check failed; the click
	// was not an action on a real item.
	itemActionRefused itemActionResult = iota
	// itemActionHandled: the click was handled — a plugin
	// claimed it, the gate refused it, or (first actions) the
	// equip fallback ran. TS handleFirstAction returns true
	// after equipping even when equip itself refused
	// internally, and TS handleAction returns the
	// emitItemAction result directly, so "handled" means the
	// click was consumed, not that an item moved.
	itemActionHandled
	// itemActionPlugin: the click reached the plugin event and
	// no plugin handled it. The item stays where it was.
	itemActionPlugin
)

// dropOptions is DROP_OPTIONS (ItemActionPacketListener.ts:17-22):
// the option verbs that mean "drop" when the option text is
// present. Comparison is against the lowercased option.
var dropOptions = map[string]bool{
	"drop": true, "destroy": true, "discard": true, "release": true,
}

// isDropOption reports whether an inventory action is a drop
// action. ItemActionPacketListener.ts:64-76: with a non-empty
// (lowercased) option, the option must be one of DROP_OPTIONS;
// with an empty option, the action is a drop only when the packet
// carried option index 5. optionIndex is nil when the packet
// carried no option index (INVENTORY_USE has no such field).
func isDropOption(option string, optionIndex *int) bool {
	if option != "" {
		return dropOptions[option]
	}
	return optionIndex != nil && *optionIndex == 5
}

// resolveInventoryWidgetAction maps a widget operation on an
// inventory item to the (option index, option text) pair the
// inventory-action router consumes.
// ItemActionPacketListener.ts:40-62 (resolveInventoryWidgetAction):
// widget op 10 is always Examine; op 1 is the shift-click option;
// ops 2, 3, 4, 6, 7 map to action indices 0..4 (op 5 is Use and
// never reaches this resolver). An op with no corresponding
// option, or an empty option, resolves to nothing. The second
// return value reports whether a resolution exists. The option
// index is 1-based, as TS returns index+1.
//
// The action lists come from the player's action source when one
// is installed, else the ObjType defaults (see helpers.go): with
// the defaults, only op 10 (Examine) and the Drop option resolve.
func resolveInventoryWidgetAction(p *game.Player, itemID, widgetOp int) (optionIndex int, option string, ok bool) {
	if widgetOp == 10 {
		return 10, "Examine", true
	}
	src := actionSource(p)
	actions := inventoryActions(src, itemID)
	var index int
	if widgetOp == 1 {
		index = shiftClickIndex(src, itemID)
	} else {
		index = -1
		for i, op := range []int{2, 3, 4, 6, 7} {
			if op == widgetOp {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(actions) {
		return 0, "", false
	}
	option = actions[index]
	if option == "" {
		return 0, "", false
	}
	return index + 1, option, true
}

// itemContainerFor returns the container an item action reads:
// the equipment container for interface 1688, the inventory for
// every other interface. ItemActionPacketListener.ts:96-97
// (itemContainer): TS returns equipment when the interface id is
// Equipment.INVENTORY_INTERFACE_ID, else the inventory.
func itemContainerFor(p *game.Player, interfaceID int) game.Container {
	if interfaceID == game.EquipmentInventoryInterfaceID {
		return p.Equipment()
	}
	return p.Inventory()
}

// handleAction is the shared entry for inventory item actions.
// ItemActionPacketListener.ts:78-108 (handleAction). clickType 1
// is a first-action click (handleFirstAction); any other
// clickType reads the item and emits the plugin event. hooks
// carries the hub emits (see ItemActionHooks); a zero value
// runs the no-hooks defaults.
func handleAction(p *game.Player, interfaceID, itemID, slot, clickType int, option string, hooks ItemActionHooks) itemActionResult {
	if !playerReady(p) {
		return itemActionRefused
	}
	// emitCanUseItem (lines 79-81): the gate runs first, before
	// the walk-to cancellation and the click-type dispatch, for
	// first and non-first clicks alike. A refusal returns
	// handled (TS returns true), so the click is consumed and
	// never drops; an undecided or allow answer proceeds. With
	// no hooks installed the gate is nil and the click proceeds.
	if hooks.EmitCanUseItem != nil {
		if allow := hooks.EmitCanUseItem(p, itemID, "action", option); allow != nil && !*allow {
			return itemActionHandled
		}
	}

	// clearPendingAction (line 84): only when the container is
	// the inventory — TS checks itemContainer(...) !==
	// player.equipment, which holds for every interface except
	// 1688. Only the walk-to cancellation is ported; see the
	// file comment.
	if interfaceID != game.EquipmentInventoryInterfaceID {
		p.MovementQueue().CancelWalkTo()
		// clearPendingAction's combat clearInteraction,
		// following clear, and plugin pending-action event:
		// GAPs. No combat, following, or plugin systems
		// exist.
	}

	if clickType == 1 {
		return handleFirstAction(p, interfaceID, itemID, slot, option, hooks)
	}

	// Non-first click (lines 89-106): the slot must hold the
	// named item, then the plugin event runs.
	container := itemContainerFor(p, interfaceID)
	if container.ItemAt(slot).ID() != itemID {
		return itemActionRefused
	}
	// emitItemAction: with no hooks installed the emit is nil
	// and the click is unhandled (handled=false,
	// PluginManager.ts:2016-2022), so the item stays put. A
	// handled click returns handled, so the caller (the drop
	// branch) does not drop the item.
	if hooks.EmitItemAction != nil {
		ev := &ItemActionEvent{
			Player: p, InterfaceID: interfaceID, ItemID: itemID,
			Slot: slot, ClickType: clickType, Option: option,
		}
		if hooks.EmitItemAction(ev) {
			return itemActionHandled
		}
	}
	return itemActionPlugin
}

// handleFirstAction runs a first-action click: gates, the plugin
// event, the equip fallback for wieldables on the inventory
// interface, and the hard-coded item special cases.
// ItemActionPacketListener.ts:110-147 (handleFirstAction).
func handleFirstAction(p *game.Player, interfaceID, itemID, slot int, option string, hooks ItemActionHooks) itemActionResult {
	container := itemContainerFor(p, interfaceID)
	// Slot bounds and slot/id checks (lines 111-113): TS
	// returns false when the slot is out of range or holds a
	// different item. ItemAt returns the empty item out of
	// range, so the bounds check is explicit.
	if slot < 0 || slot >= container.Capacity() {
		return itemActionRefused
	}
	if container.ItemAt(slot).ID() != itemID {
		return itemActionRefused
	}

	// Teleport/dead gates (lines 114-116): isTeleportingReturn
	// degenerates to pass (no teleporting state exists; see
	// drop.go). The hitpoints gate is ported.
	if hitpoints(p) <= 0 {
		return itemActionRefused
	}

	// Re-read (line 117): TS re-reads the slot after the gates
	// and returns false when the item changed. No concurrent
	// mutation can happen between the reads in Go, but the
	// check is ported for shape.
	currentItem := container.ItemAt(slot)
	if currentItem.ID() != itemID {
		return itemActionRefused
	}

	// emitItemAction (lines 118-120): a handled click returns
	// handled before any fallback runs, so the equip fallback
	// below never fires for it. With no hooks installed the
	// emit is nil and TS proceeds to the fallbacks below
	// (PluginManager.ts:2016-2022).
	if hooks.EmitItemAction != nil {
		ev := &ItemActionEvent{
			Player: p, InterfaceID: interfaceID, ItemID: itemID,
			Slot: slot, ClickType: 1, Option: option,
		}
		if hooks.EmitItemAction(ev) {
			return itemActionHandled
		}
	}

	// Equip fallback (lines 121-129): on the inventory
	// interface, a wieldable item equips. TS reads the item
	// from the inventory (not the container) and checks the
	// equipment slot is an integer >= 0; Go ints are always
	// integers, and a nil definition source reads -1. The
	// fallback returns true whether or not equip itself
	// refused internally — equip's gates are silent in TS.
	if interfaceID == game.InventoryInterfaceID {
		item := p.Inventory().ItemAt(slot)
		if equipmentSlotOf(p, item.ID()) >= 0 {
			equip(p, itemID, slot, interfaceID)
			return itemActionHandled
		}
	}

	// sendInterfaceRemoval (line 132): GAP. No interface system
	// exists.

	// Hard-coded special cases (lines 134-145). Every case
	// returns false in TS (unhandled) except the 2545 branches
	// that return true; the ported behavior matches, with the
	// messages sent through Player.SendMessage.
	switch itemID {
	case 9520:
		// ItemActionPacketListener.ts:134-136: the message
		// is unconditional — TS checks no Wilderness state
		// here, it only ever sends this line.
		p.SendMessage("You cannot use this in the Wilderness!")
	case 2545:
		// ItemActionPacketListener.ts:138-146. The busy()
		// branch is a gap: busy() degenerates to the
		// hitpoints term (already > 0), so "You cannot do
		// that right now." can never fire here — no busy
		// state exists to evaluate. The attribute branch
		// IS ported: attributes exist (phase 11), and TS
		// compares strictly against true.
		if unlocked, _ := p.GetAttribute("bounty-hunter:target-teleport-unlocked").(bool); unlocked {
			p.SendMessage("You have already unlocked that teleport.")
			return itemActionHandled
		}
	case 12873, 12875, 12879, 12881, 12883, 12877:
		// Empty cases in TS: the items are consumed by
		// plugin handlers only, which already ran above
		// (the emitItemAction site) and did not claim them.
	}
	return itemActionPlugin
}

// wieldOption reports whether an inventory option verb is a wield
// verb: NetworkBuilder.ts:1761-1762 matches /^(wield|wear|equip|
// hold)$/ against the lowercased option — an exact match, not a
// prefix match, so "wielding" does not wield.
func wieldOption(option string) bool {
	switch option {
	case "wield", "wear", "equip", "hold":
		return true
	default:
		return false
	}
}
