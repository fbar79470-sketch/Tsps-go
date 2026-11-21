// UseItemPacketListener, item-on-object paths only: item_on_object
// (opcode 86, OPLOCU) and the loc kind of INVENTORY_USE_ON (241),
// porting itemOnObject (UseItemPacketListener.ts:39-91) and the
// inventory_use_on dispatch case (NetworkBuilder.ts:836-857).
//
// itemOnObject routes through the SAME walk-to-object dispatch as
// object clicks: the item is verified, the loc is looked up, and
// walkToObject runs the useItem arrival closure (no clickType
// argument — the route event carries clickType 0). The arrival
// closure re-checks still-stands, resets, emits item-on-object,
// and — when no plugin handles the use — sends "Nothing
// interesting happens." The captured item reference is passed
// through; unlike itemOnNpc, the inventory is NOT re-verified on
// arrival.
//
// Boundary: the other itemOn* paths stay decode-drop with
// ReasonUseItem. itemOnNpc/itemOnPlayer need walkToEntity; phase 16
// decided (see item-on-NPC in listeners.go) that combat
// targeting ports NO reusable walk-to-entity — the combat
// approach is internal to the Combat process — and onItemOnNpc /
// onNpcRoute stay table-only, so item-on-NPC stays a decode-drop
// even with combat landed; itemOnGroundItem needs
// emitItemOnGroundItem (family unfired). spellOnObject (opcode
// 2) stays decode-drop: magic is NOT STARTED and onSpellOnObject
// stays table-only. The spell click still sets the click-run
// override (it is in INTERACTION_CLICKS,
// NetworkBuilder.ts:246-260) before dropping.
//
// itemOnItem is implemented since phase 33: the inventory kind
// of INVENTORY_USE_ON (241) routes here, not through a
// walk-to. There is no separate item-on-item opcode —
// ClientProtocol.ts:912-925 decodes kind 4 as the inventory
// target, and NetworkBuilder.ts:839-844 dispatches it to
// itemOnItem (UseItemPacketListener.ts:15-37).
//
// Gates: itemOnObject has NO hitpoints/busy gate — only the slot
// bounds check, the item identity check, and the canUse gate.
// canUse (emitCanUseItem, first-answer, null when no hooks)
// degenerates to allow in Go: onCanUseItem stays table-only, so
// no hook can refuse a use.
//
// Definition: TS itemOnObject does not look the definition up;
// walkToObject's buildObjectRouteSpec resolves forPlayer ?? forId
// (MovementQueue.ts:1357-1358). Go resolves the same way —
// ObjectForPlayer first, ObjectByID as fallback — because the
// route spec needs the footprint and mask. When neither resolves
// the packet returns silently: the registry's own forPlayer
// variant rule normally fails the lookup first in that case.
// The route event always carries the resolved definition; TS
// would carry undefined when forPlayer fails, a difference with
// no observable effect on the route (the spec uses the same
// fallback).

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// routeItemOnObject routes one decoded item_on_object (OPLOCU)
// message. The click-run override is set before routing, even
// when routing then refuses the packet (the INTERACTION_CLICKS
// preamble + the ground-item precedent). An unwired lookup or
// definition source drops with ReasonUseItem; a refused packet
// returns noAction, the silent return TS gives.
func routeItemOnObject(p *game.Player, msg protocol.ItemOnObject, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if q := p.MovementQueue(); q != nil {
		q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
	}
	if deps.Objects == nil || deps.ObjectDefs == nil {
		return drop(ReasonUseItem)
	}
	if itemOnObject(p, int(msg.WidgetID), msg.ObjectID, msg.ItemID, msg.Slot, msg.X, msg.Y, cycle, deps) {
		return acted()
	}
	return noAction()
}

// routeInventoryUseOn routes one decoded INVENTORY_USE_ON
// message (NetworkBuilder.ts:836-857). The inventory kind
// routes to item-on-item (no walk, no click-run override —
// NetworkBuilder.ts:713-719 excludes kind == inventory from
// INTERACTION_CLICKS); the loc kind with a tile routes to
// item-on-object, with interfaceType 3214 (the inventory
// widget) and no modifier flags (the decoder exposes none —
// the existing protocol shape). Every other kind (npc,
// player, ground) drops with ReasonUseItem: those paths need
// walkToEntity or unfired emit families (see the file
// header). The click-run override is set for every kind
// EXCEPT inventory (NetworkBuilder.ts:713-719: inventory_use_on
// is in INTERACTION_CLICKS except when kind == inventory),
// with flags 0 — the packet carries no modifier flags —
// even for kinds that then drop.
func routeInventoryUseOn(p *game.Player, msg protocol.InventoryUseOn, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	target := msg.Target
	if target.Kind != protocol.UseOnTargetInventory {
		if q := p.MovementQueue(); q != nil {
			q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), 0))
		}
	}
	if target.Kind == protocol.UseOnTargetInventory {
		if itemOnItem(p, msg, deps) {
			return acted()
		}
		return noAction()
	}
	if target.Kind != protocol.UseOnTargetLoc || !target.HasTile {
		return drop(ReasonUseItem)
	}
	if deps.Objects == nil || deps.ObjectDefs == nil {
		return drop(ReasonUseItem)
	}
	if itemOnObject(p, 3214, target.ID, msg.ItemID, msg.Slot, target.X, target.Y, cycle, deps) {
		return acted()
	}
	return noAction()
}

// ItemOnItemEvent is the listener-facing form of the
// item-on-item payload (PluginItemOnItemEvent,
// PluginTypes.ts:722-731): the player, the used item and the
// used-with item (the inventory stacks as they stood when
// the use was clicked — TS captures the Item references),
// their ids and slots, and the handled flag. The listener
// event carries game.Item value copies; a handler that
// mutates an item writes it back through the inventory at
// the carried slot.
type ItemOnItemEvent struct {
	Player           *game.Player
	UsedItem         game.Item
	UsedItemID       int
	UsedItemSlot     int
	UsedWithItem     game.Item
	UsedWithItemID   int
	UsedWithItemSlot int
	Handled          bool
}

// itemOnItem ports UseItemPacketListener.itemOnItem
// (UseItemPacketListener.ts:15-37). It reports whether the
// use was verified and emitted. There is no hitpoints/busy
// gate and no walk-to: the use is emitted immediately, and
// an unhandled use falls through silently — TS ignores the
// emit's return, so there is no "Nothing interesting
// happens." fallback here (unlike itemOnObject).
//
// Verification mirrors TS, with one deliberate deviation:
// both slots must be inside the inventory (lines 17-22 —
// an out-of-range slot returns undefined in TS, which
// fails the !usedItem check), and both slots must hold
// items. TS does NOT verify the packet's item ids against
// the slots (unlike itemOnObject, line 50): it reads the
// items from the slots and emits them. The canUse gate
// (line 26) degenerates to allow (see the useitem.go file
// header). With no emit installed the use still counts as
// acted: TS emits to zero hooks and returns silently.
//
// DEVIATION: an empty slot holds Item(-1, 0) in TS
// (ItemContainer.ts:55-56 fills every slot), which is
// truthy, so literal TS would emit an event with id -1 to
// plain handlers. Go refuses an empty slot instead: no
// legitimate client sends a use for an empty slot, and no
// content can claim id -1, so the refusal is
// unobservable except to a crafted packet.
func itemOnItem(p *game.Player, msg protocol.InventoryUseOn, deps Deps) bool {
	if !playerReady(p) {
		return false
	}
	inv := p.Inventory()
	if msg.Slot < 0 || msg.Slot >= inv.Capacity() ||
		msg.Target.Slot < 0 || msg.Target.Slot >= inv.Capacity() {
		return false
	}
	used := inv.ItemAt(msg.Slot)
	with := inv.ItemAt(msg.Target.Slot)
	if used.ID() < 0 || with.ID() < 0 {
		return false
	}
	ev := &ItemOnItemEvent{
		Player:           p,
		UsedItem:         used,
		UsedItemID:       used.ID(),
		UsedItemSlot:     msg.Slot,
		UsedWithItem:     with,
		UsedWithItemID:   with.ID(),
		UsedWithItemSlot: msg.Target.Slot,
	}
	if deps.ObjectHooks.EmitItemOnItem != nil {
		ev.Handled = deps.ObjectHooks.EmitItemOnItem(ev)
	}
	return true
}

// itemOnObject ports UseItemPacketListener.itemOnObject
// (UseItemPacketListener.ts:39-91). It reports whether the
// walk-to started. There is no hitpoints/busy gate.
func itemOnObject(p *game.Player, interfaceType, objectID, itemID, itemSlot, objectX, objectY int, cycle int64, deps Deps) bool {
	if !playerReady(p) {
		return false
	}

	// Slot bounds (line 46): the slot must be inside the
	// inventory. The item must exist and its id must match the
	// packet's (line 50). An empty slot reads id -1, which
	// never matches a decoded id. The canUse gate (line 50)
	// degenerates to allow (see the file header).
	inv := p.Inventory()
	if itemSlot < 0 || itemSlot >= inv.Capacity() {
		return false
	}
	item := inv.ItemAt(itemSlot)
	if item.ID() != itemID {
		return false
	}

	// Location (line 54): the packet x/y with the player's
	// plane. Lookup (line 55): MapObjects.getPrivateArea. A
	// miss returns silently.
	loc := game.Location{X: objectX, Y: objectY, Z: p.Location().Z}
	obj := deps.Objects.Find(p, objectID, loc)
	if obj == nil {
		return false
	}

	// Definition: forPlayer ?? forId on the object's id (see
	// the file header). Base definition for the type 10/11
	// footprint rule, falling back to the resolved definition.
	def, ok := deps.ObjectDefs.ObjectForPlayer(obj.ID(), p)
	if !ok {
		if b, ok := deps.ObjectDefs.ObjectByID(obj.ID()); ok {
			def = b
		} else {
			return false
		}
	}
	base := def
	if b, ok := deps.ObjectDefs.ObjectByID(obj.ID()); ok {
		base = b
	}

	// The walk-to-object validity check re-looks the object up
	// by its own id and tile (MovementQueue.ts:1686, 1823).
	stillStands := func() bool {
		return objectStillStands(deps.Objects, p, obj.ID(), obj.Location(), obj)
	}

	// Arrival closure: useItem (lines 60-89). No clickType
	// argument: the route event carries clickType 0.
	arrival := func() {
		// Still-stands re-check (line 61), by the packet
		// id and the click tile: return if gone.
		if !objectStillStands(deps.Objects, p, objectID, loc, obj) {
			return
		}
		// Reset + walkToReset, and the facing call (a gap
		// — no facing system).
		q := p.MovementQueue()
		q.Reset(true)
		q.CancelWalkTo()

		// Emit (lines 66-84): the captured item, the
		// object's id and tile. The inventory is NOT
		// re-verified.
		ev := &ItemOnObjectEvent{
			Player:        p,
			Object:        obj,
			ObjectID:      obj.ID(),
			Item:          item,
			ItemID:        itemID,
			ItemSlot:      itemSlot,
			InterfaceType: interfaceType,
			Location:      obj.Location(),
		}
		handled := false
		if deps.ObjectHooks.EmitItemOnObject != nil {
			handled = deps.ObjectHooks.EmitItemOnObject(ev)
		}
		if handled || ev.Handled {
			return
		}
		// Unhandled fallback (lines 86-88): the message.
		// No operated emit: item-on-object never emits
		// object:operated.
		p.SendMessage("Nothing interesting happens.")
	}

	target := game.ObjectTarget{
		ID:           obj.ID(),
		Location:     obj.Location(),
		Type:         obj.Type(),
		Face:         obj.Face(),
		SizeX:        def.SizeX(),
		SizeY:        def.SizeY(),
		BaseSizeX:    base.SizeX(),
		BaseSizeY:    base.SizeY(),
		BlockingMask: def.BlockingMask(),
		Name:         def.Name(),
		Interactions: def.Interactions(),
	}
	p.MovementQueue().WalkToObject(target, arrival, 0, cycle, game.WalkToObjectHooks{
		Route:       translateRouteHook(deps, p, obj, def, 0),
		StillStands: stillStands,
	})
	return true
}
