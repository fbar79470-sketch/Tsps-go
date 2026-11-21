// Drop: DropItemPacketListener.ts:39-120 (commit bfd81e3).
//
// GAPS, documented at their sites below, never invented:
//   - isTeleportingReturn: no teleporting state exists in Go —
//     nothing can be teleporting, so the gate degenerates to pass.
//   - busy() + sendInterfaceRemoval: busy() reads interface id,
//     hitpoints, placement/teleporting, status, and force
//     movement (Player.ts:831-849). With no interface, status, or
//     force-movement state, busy() degenerates to the hitpoints
//     term, which the earlier gate already passed; the interface
//     removal send has no interface system to send to.
//   - The DEVELOPER debug message: no rights system exists to
//     evaluate the gate (the chat encoder exists now; the
//     gate is the gap, not the send). See the call site.
//   - Plugin events item:drop-warning, item:drop policy, and
//     item:discarded: no plugin system. With no hooks the policy
//     emit returns handled=false (proceed) and the warning emit
//     leaves warned=false (proceed) (PluginManager.ts:2045-2050).
//   - Wilderness: the registerGlobal branch is implemented
//     in dropItemChecked behind a wilderness-check seam
//     (Wilderness.isIn, DropItemPacketListener.ts:107-111),
//     but UNWIRED: dropItem — the function the dispatch call
//     site uses — passes a nil check, so every drop still
//     goes through registers() (owner-visible, goes global).
//     Wiring needs a Deps field carrying the check plus a
//     call-site change in listeners.go (neither is this
//     file's to make); see dropItemChecked.
//   - The destroy-item interface (undroppable items): a widget
//     flow needing interface, attribute, and chatbox-string
//     systems that do not exist. Undroppable items can neither be
//     dropped nor destroyed.
//   - Sounds.sendSound: no sound system exists.

package listeners

import (
	"tsps-go/internal/game"
)

// dropOutcome is the result of dropItem: TS drop() returns void,
// but the packet Outcome needs to distinguish a drop that
// happened from a gate refusal and from the destroy-interface gap.
type dropOutcome int

const (
	// dropRefused: a gate refused the packet; nothing changed.
	dropRefused dropOutcome = iota
	// dropDone: the item was registered on the floor and the slot
	// cleared.
	dropDone
	// dropDestroy: the item is undroppable; TS would open the
	// destroy-item interface (gap). Nothing changed.
	dropDestroy
	// dropNoGroundItems: the drop was otherwise valid but no
	// ground-item store is wired (Deps.GroundItems is nil), so
	// the item could not be registered. Nothing changed.
	dropNoGroundItems
)

// dropItem drops the item in inventory slot, checking that it is
// id and that the click arrived on the inventory interface.
// DropItemPacketListener.ts:39-120 (drop). confirmed is the TS
// drop-warning confirmation flag; with no plugin system it is
// always false at the call site, and the warning path that would
// re-invoke with confirmed=true never runs.
//
// dropItem passes a nil Wilderness check to dropItemChecked:
// the production dispatch path has no Wilderness wired (see
// the file comment), so every drop takes the registers()
// branch. The coordinator wires the check by switching the
// call site in listeners.go to dropItemChecked with a
// Wilderness-backed check.
func dropItem(p *game.Player, id, interfaceID, slot int, groundItems GroundItems) dropOutcome {
	return dropItemChecked(p, id, interfaceID, slot, groundItems, nil)
}

// dropItemChecked is dropItem with the Wilderness check
// supplied: wildernessIn reports whether a location is in the
// Wilderness (Wilderness.isIn, DropItemPacketListener.ts:107).
// A nil check degenerates to "never in the Wilderness", which
// is the dropItem behavior. The check is a func(Location)
// bool rather than a *game.Wilderness so this package needs
// no new dependency: the coordinator passes
// func(loc game.Location) bool { return wilderness.IsInLocation(loc) }
// (or IsIn(p)) from the world's Wilderness.
func dropItemChecked(p *game.Player, id, interfaceID, slot int, groundItems GroundItems, wildernessIn func(game.Location) bool) dropOutcome {
	if !playerReady(p) {
		return dropRefused
	}
	// TS: `if (!player || player.getHitpoints() <= 0) return;`
	// (line 39). Handle guarantees a non-nil player.
	if hitpoints(p) <= 0 {
		return dropRefused
	}

	// isTeleportingReturn gate (line 43): GAP, degenerates to
	// pass. No teleporting state exists in Go; nothing can be
	// teleporting, so the condition can never hold.

	// Interface check (line 44): the click must come from the
	// inventory interface (3214). With no open-interface
	// tracking, this degenerates to a check on the interface the
	// caller says the click arrived on; the Go call site passes
	// InventoryInterfaceID, so it always passes here. A click
	// arriving on any other interface is refused.
	if interfaceID != game.InventoryInterfaceID {
		return dropRefused
	}

	// Duplicate hitpoints gate (line 45): TS checks again after
	// the interface check. Ported for shape; the value cannot
	// have changed.
	if hitpoints(p) <= 0 {
		return dropRefused
	}

	inv := p.Inventory()
	// Slot bounds (line 48): TS returns when the slot item is
	// undefined; ItemAt returns the empty item out of range, so
	// the bounds check is explicit.
	if slot < 0 || slot >= inv.Capacity() {
		return dropRefused
	}

	// busy() gate (lines 49-51): GAP, degenerates to pass. See
	// the file comment: every busy() term except hitpoints reads
	// state that does not exist in Go, and hitpoints already
	// passed. The sendInterfaceRemoval the gate would send has
	// no interface system to send to.

	item := inv.ItemAt(slot)
	// Slot/id/amount checks (lines 52-56): the slot must hold the
	// id the packet named, with a positive amount. An empty slot
	// reads id -1, which never matches a decoded id.
	if item.ID() != id || item.Amount() <= 0 {
		return dropRefused
	}

	// DEVELOPER debug message (lines 57-59): GAP. TS sends
	// "Drop item: <id>." when the player's rights are
	// DEVELOPER. No rights system exists in Go, so the gate
	// cannot be evaluated: sending unconditionally would spam
	// every player, and never sending is correct for every
	// non-developer. The message stays unsent until rights
	// land; the chat encoder is no longer the blocker.

	// sendInterfaceRemoval (line 63): GAP. No interface system
	// exists to remove an interface from.

	// stopSkillable (line 64): cancels the player's current
	// skill action through the skill slot (phase 20); a bare
	// player with no action running is unaffected.
	p.Skills().StopSkillable()

	// emitItemDropPolicy (lines 65-68): GAP, degenerates to
	// proceed. With no plugin hooks the emit returns
	// handled=false, and TS proceeds only when handled is false
	// (PluginManager.ts:2045-2050).

	if item.IsDropable(defs(p)) {
		// Drop-warning plugin event (lines 76-88): GAP,
		// degenerates to proceed. With no hooks the event
		// leaves warned=false, so TS never returns early and
		// never re-invokes with confirmed=true. The
		// `confirmed` parameter therefore never matters in Go.

		// Wilderness check (DropItemPacketListener.ts:107-111):
		// Wilderness.isIn
		// selects registerGlobal (SEEN_BY_EVERYONE,
		// goesGlobal=false) over registers()
		// (ItemOnGroundManager.ts:366-392: registerGlobal
		// vs registerLocation with default options).
		// With a nil check this always takes the
		// registers() branch (see dropItem); the
		// coordinator wires the check through
		// dropItemChecked.
		if groundItems == nil {
			return dropNoGroundItems
		}
		state, goesGlobal := game.GroundItemSeenByPlayer, true
		if wildernessIn != nil && wildernessIn(p.Location()) {
			state, goesGlobal = game.GroundItemSeenByEveryone, false
		}
		stack := game.NewItemOnGround(
			state,
			p.Username,
			p.Location(),
			item.ID(),
			item.Amount(),
			itemMetaToGround(item.Meta()),
			goesGlobal, -1, false, nil,
		)
		groundItems.Register(stack)

		// Clear the slot and refresh (lines 95-96). TS clears
		// with setItem(slot, new Item(-1, 0)).
		inv.SetItem(slot, game.NewItem(-1, 0))
		inv.RefreshItems()

		// item:discarded event (line 97): GAP. No plugin
		// system. Sounds.sendSound (line 98): GAP. No sound
		// system.
		return dropDone
	}

	// Undroppable (lines 100-118): destroyItemInterface — a
	// widget flow (chatbox interface 584, the
	// destroy-item:pending attribute, interface strings, and the
	// confirmation flag) needing interface, attribute, and
	// widget systems that do not exist. GAP: the item is neither
	// dropped nor destroyed.
	return dropDestroy
}
