// Equip and unequip: EquipPacketListener.ts (287 lines, commit
// bfd81e3). equip() is lines 101-236, unequip() 240-283,
// resolveEquipmentSlot() 66-72, the displace helper 54-69, and
// EQUIPMENT_SLOTS at line 52.
//
// GAPS, documented at their sites below, never invented:
//   - resetWeapon (lines 74-89): the assign half runs —
//     equip/unequip call game.AssignWeaponInterface, which
//     re-selects the combat interface, fight type and varp 43
//     (WeaponInterfaceManager.assign). The special-attack
//     halves (special deactivate + sendSpecialAttackState)
//     and the autocast halves (Autocasting.setAutocast) stay
//     gaps: Worker C (spec orb) and Phase 27 (spells).
//   - BonusManager.update: no cached array to refresh — Go
//     computes bonuses live through game.ComputeBonuses on
//     every Combatant bonus read, so the equip/unequip sites
//     carry a note instead of an update call.
//   - Following clear (lines 131-137): no Following field, no
//     interaction targets; following is NOT STARTED.
//   - The interface-removal branch (lines 138-145): no
//     open-interface tracking exists, so no interface is ever open
//     and the branch never fires. The preservesInterfaceOnEquip
//     attribute read is likewise absent (no attribute state).
//   - Requirement and no-space refusal messages ARE sent
//     (Player.SendMessage): the equipment-requirements line
//     (EquipmentRequirements.message, subject "this") and
//     UNEQUIP_NO_SPACE_MESSAGE. The equipment-slot -1 case
//     logs server-side in TS (no player text), so nothing is
//     sent there.
//   - Plugin gates emitCanEquip / emitCanUnequip: no plugin
//     system. With no hooks both return null, which TS treats as
//     "proceed" (PluginManager.ts:1627-1638, 1706-1717), so the
//     gates degenerate to proceed.
//   - Sounds.sendSound: no sound system exists.
// The APPEARANCE flag TS sets directly is set in Go by the
// OnEquipmentChanged hook, which rebuilds the appearance block
// when Equipment.RefreshItems runs (see internal/game/equipment.go);
// equip/unequip do not set flags themselves.

package listeners

import (
	"tsps-go/internal/game"
)

// equipmentSlots is EQUIPMENT_SLOTS (EquipPacketListener.ts:52):
// the equipment slot for each child index of the equipment widget,
// in widget order. Gaps at 6/8/11 are preserved by the slot
// constants, not by this list.
var equipmentSlots = []int{
	game.HeadSlot, game.CapeSlot, game.AmuletSlot, game.WeaponSlot,
	game.BodySlot, game.ShieldSlot, game.LegSlot, game.HandsSlot,
	game.FeetSlot, game.RingSlot, game.AmmunitionSlot,
}

// resolveEquipmentSlot maps a widget-action target (group, child)
// to an equipment slot, or -1. EquipPacketListener.ts:66-72
// (resolveEquipmentSlot): the equipment screen (group 387, first
// child 15), the equipment tab (group 84, first child 10), and the
// worn-equipment side panel (group 12, first child 68). Any other
// group, or a child outside the slot list, reads -1.
func resolveEquipmentSlot(groupID, childID int) int {
	var firstChild int
	switch groupID {
	case 387:
		firstChild = 15
	case 84:
		firstChild = 10
	case 12:
		firstChild = 68
	default:
		return -1
	}
	index := childID - firstChild
	if index < 0 || index >= len(equipmentSlots) {
		return -1
	}
	return equipmentSlots[index]
}

// displace moves a worn item into the inventory slot the equipped
// item came from, or, when the worn item would join an existing
// stack there, deletes the slot's contents and adds the worn item
// to the stack. EquipPacketListener.ts:54-69. It returns false
// only when the stack join has no room (the inventory-full signal
// is a no-op in Go: see game.Inventory.Full). The slot item is
// read before it is deleted, as in TS.
func displace(p *game.Player, slot int, worn game.Item) bool {
	inv := p.Inventory()
	joinsStack := stackableOf(p, worn.ID())
	if joinsStack {
		joinsStack = false
		for _, held := range inv.Items() {
			if sameStack(held, worn) {
				joinsStack = true
				break
			}
		}
	}
	if !joinsStack {
		inv.SetItem(slot, worn)
		return true
	}
	if inv.GetAddableAmount(worn) < worn.Amount() {
		inv.Full()
		return false
	}
	inv.DeleteAtSlot(slot, inv.ItemAt(slot).Amount(), false)
	inv.Add(worn, false)
	return true
}

// EquipForBank is the bank side panel's equip seam
// (Bank.equipFromSide, Bank.ts:874-881): the bank calls
// EquipPacketListener.equip with Inventory.INTERFACE_ID
// (3214) after setting the preserve-interface attribute, so
// the hook passes InventoryInterfaceID, exactly as the
// inventory-click call sites do. The coordinator installs it
// as game.BankEquipHook.
func EquipForBank(p *game.Player, itemID, slot int) bool {
	return equip(p, itemID, slot, game.InventoryInterfaceID)
}

// UnequipForBank is the bank worn panel's unequip seam
// (Bank.wornSlotAction op 1, Bank.ts:1034-1041): the
// coordinator installs it as game.BankUnequipHook.
func UnequipForBank(p *game.Player, slot int) bool {
	return unequip(p, slot)
}

// equip wears the item in inventory slot, checking that it is id
// and that the open interface is the inventory interface.
// EquipPacketListener.ts:101-236 (equip). It returns true when the
// item was worn; false when any gate refused. The boolean is the
// Go form of the TS void: TS callers never read a result, but the
// Outcome for the packet needs to know whether a state change
// happened. interfaceID is the interface the click arrived on;
// with no open-interface tracking, call sites pass
// game.InventoryInterfaceID (3214) and the check always passes —
// see the degeneration note in the body.
func equip(p *game.Player, id, slot, interfaceID int) bool {
	if !playerReady(p) {
		return false
	}
	// TS: `if (!player || player.getHitpoints() <= 0) return;`
	// (line 102). Handle guarantees a non-nil player.
	if hitpoints(p) <= 0 {
		return false
	}
	inv := p.Inventory()
	eq := p.Equipment()
	// Slot bounds (line 103): TS reads the slot item and returns
	// when it is undefined; ItemAt returns the empty item
	// out of range, so the bounds check is explicit.
	if slot < 0 || slot >= inv.Capacity() {
		return false
	}
	// Slot/id check (line 104): the slot must hold the id the
	// packet named. An empty slot reads id -1, which never
	// matches a decoded id.
	if inv.ItemAt(slot).ID() != id {
		return false
	}

	// Following clear (lines 131-137): GAP. No Following field
	// and no interaction targets exist in Go; following is
	// NOT STARTED. A player mid-follow keeps following after
	// equipping; TS would stop the follow when the followed
	// target moved or was removed.

	// Interface-removal branch (lines 138-145): GAP, degenerates
	// to never firing. No open-interface tracking exists, so no
	// interface is ever open and the "interface is open and not
	// preserved" condition can never hold. The
	// preservesInterfaceOnEquip attribute read is likewise
	// absent: no attribute state exists.

	// stopSkillable (line 149): cancels the player's current
	// skill action through the skill slot (phase 20); a bare
	// player with no action running is unaffected.
	p.Skills().StopSkillable()

	// Interface check (line 150): the click must come from the
	// inventory interface (3214). With no open-interface
	// tracking, this degenerates to a check on the interface the
	// caller says the click arrived on; every Go call site
	// passes InventoryInterfaceID, so it always passes here.
	// A click arriving on any other interface is refused.
	if interfaceID != game.InventoryInterfaceID {
		return false
	}

	item := inv.ItemAt(slot).Clone()

	// Equipment requirements (lines 152-158): GetUnmetRequirement
	// takes the player's Skills as the levels and the definition
	// source. TS sends EquipmentRequirements.message(unmet)
	// (subject defaults to "this") and returns.
	if unmet := game.GetUnmetRequirement(p.Skills(), defs(p), item.ID()); unmet != nil {
		p.SendMessage(game.RequirementMessage(*unmet, "this"))
		return false
	}

	// Equipment slot (lines 159-163): -1 logs a server-side
	// error (Server.getLogger) and returns. No player text is
	// sent in TS, so nothing is sent here either; the return
	// is ported.
	equipmentSlot := equipmentSlotOf(p, item.ID())
	if equipmentSlot == -1 {
		return false
	}

	// emitCanEquip (lines 164-167): GAP, degenerates to proceed.
	// With no plugin hooks the emit returns null, which TS
	// treats as "not false" and proceeds
	// (PluginManager.ts:1627-1638).

	currentlyEquipped := eq.ItemAt(equipmentSlot)
	sameStack := sameStack(currentlyEquipped, item)
	if sameStack && stackableOf(p, item.ID()) {
		// Stackable merge (lines 169-180): the worn stack grows
		// by the inventory stack. No room signals full and
		// returns. The worn item keeps its identity (and meta,
		// equal by sameStack); only the amount changes.
		if eq.GetAddableAmount(item) < item.Amount() {
			inv.Full()
			return false
		}
		inv.DeleteAtSlot(slot, item.Amount(), false)
		merged := currentlyEquipped.Clone()
		merged.SetAmount(currentlyEquipped.Amount() + item.Amount())
		eq.SetItem(equipmentSlot, merged)
	} else if doubleHandedOf(p, item.ID()) && equipmentSlot == game.WeaponSlot {
		// Two-handed weapon (lines 183-196): needs a free
		// inventory slot when both weapon and shield are worn,
		// displaces the worn weapon into the clicked slot, clears
		// the shield, wears the weapon, and returns the shield
		// to the inventory.
		slotsRequired := 0
		if eq.IsSlotOccupied(game.ShieldSlot) && eq.IsSlotOccupied(game.WeaponSlot) {
			slotsRequired = 1
		}
		if inv.GetFreeSlots() < slotsRequired {
			inv.Full()
			return false
		}
		shield := eq.ItemAt(game.ShieldSlot)
		weapon := eq.ItemAt(game.WeaponSlot)
		if weapon.ID() == -1 {
			inv.SetItem(slot, game.NewItem(-1, 0))
		} else if !displace(p, slot, weapon) {
			return false
		}
		eq.SetItem(game.ShieldSlot, game.NewItem(-1, 0))
		eq.SetItem(game.WeaponSlot, item)
		if shield.ID() != -1 {
			inv.Add(shield, false)
		}
	} else if equipmentSlot == game.ShieldSlot && doubleHandedOf(p, eq.ItemAt(game.WeaponSlot).ID()) {
		// Shield against a two-handed weapon (lines 198-210):
		// displaces the weapon into the clicked slot, clears the
		// weapon, wears the shield. resetWeapon (line 206) is
		// a combat GAP; see the file comment.
		weapon := eq.ItemAt(game.WeaponSlot)
		if !displace(p, slot, weapon) {
			return false
		}
		eq.SetItem(game.WeaponSlot, game.NewItem(-1, 0))
		eq.SetItem(game.ShieldSlot, item)
		// resetWeapon: GAP (combat). See the file comment.
	} else {
		// Ordinary swap (lines 212-226): when the worn item
		// belongs to the same slot it is displaced into the
		// clicked slot, otherwise the clicked slot is cleared
		// and the worn item is overwritten. A displaced worn
		// item that cannot join a stack refuses the whole
		// equip.
		if currentlyEquipped.ID() != -1 && equipmentSlotOf(p, currentlyEquipped.ID()) == equipmentSlot {
			if !displace(p, slot, currentlyEquipped) {
				return false
			}
		} else {
			inv.SetItem(slot, game.NewItem(-1, 0))
		}
		eq.SetItem(equipmentSlot, item)
	}

	if equipmentSlot == game.WeaponSlot {
		// resetWeapon (line 228): the assign half runs here —
		// WeaponInterfaceManager.assign re-selects the combat
		// interface, fight type and varp 43 for the new weapon
		// (see game.AssignWeaponInterface). The special-attack
		// halves of resetWeapon stay gaps: setSpecialActivated
		// (false) and sendSpecialAttackState (false) belong to
		// the special-attack system (Worker C), and the
		// autocast halves need Autocasting.setAutocast (spells
		// NOT STARTED, Phase 27).
		game.AssignWeaponInterface(p)
	}

	// BonusManager.update (line 231): no cached array to
	// refresh. TS caches the bonus arrays and recomputes them
	// here; Go computes them live through
	// game.ComputeBonuses on every Combatant bonus read (see
	// internal/game/bonus.go), so the new equipment's bonuses
	// are already what the formulas read.

	// Refreshes (lines 232-234): equipment refreshes with the
	// equip change, inventory refreshes, and the appearance flag
	// is set. Go's RefreshItems takes no payload and the
	// OnEquipmentChanged hook rebuilds appearance; the flag is
	// set through that path, not here.
	eq.RefreshItems()
	inv.RefreshItems()
	// Sounds.sendSound: GAP. No sound system exists.
	return true
}

// unequipNoSpaceMessage is UNEQUIP_NO_SPACE_MESSAGE
// (EquipPacketListener.ts:243): the live wording for a worn item
// with nowhere to go, sent when unequip refuses for lack of
// inventory space (line 266).
const unequipNoSpaceMessage = "You don't have enough free space to do that."

// unequip removes the item in equipment slot to the inventory.
// EquipPacketListener.ts:240-283 (unequip). It returns true when
// the item was removed; false when any gate refused. TS returns
// the same boolean to its caller.
func unequip(p *game.Player, slot int) bool {
	if !playerReady(p) {
		return false
	}
	// TS: `if (!player || player.getHitpoints() <= 0) return
	// false;` (line 241). Handle guarantees a non-nil player.
	if hitpoints(p) <= 0 {
		return false
	}
	inv := p.Inventory()
	eq := p.Equipment()
	// Slot bounds (line 242): TS reads the slot item and returns
	// false when it is undefined; ItemAt returns the empty item
	// out of range, so the bounds check is explicit.
	if slot < 0 || slot >= eq.Capacity() {
		return false
	}
	item := eq.ItemAt(slot)
	// Empty slot (line 243): id < 0 returns false.
	if item.ID() < 0 {
		return false
	}

	// emitCanUnequip (lines 244-247): GAP, degenerates to
	// proceed. With no plugin hooks the emit returns null,
	// which TS treats as "not false" and proceeds
	// (PluginManager.ts:1706-1717).

	// Stack into an existing inventory stack when the item
	// stacks and a matching stack is held (lines 250-258).
	stackIntoExisting := stackableOf(p, item.ID())
	if stackIntoExisting {
		stackIntoExisting = false
		for _, held := range inv.Items() {
			if sameStack(held, item) {
				stackIntoExisting = true
				break
			}
		}
	}
	inventorySlot := inv.GetEmptySlot()
	if stackIntoExisting {
		if inv.GetAddableAmount(item) < item.Amount() {
			// UNEQUIP_NO_SPACE_MESSAGE (line 266).
			p.SendMessage(unequipNoSpaceMessage)
			return false
		}
	} else if inventorySlot == -1 {
		// UNEQUIP_NO_SPACE_MESSAGE, as above: TS checks
		// both no-space cases in one ternary.
		p.SendMessage(unequipNoSpaceMessage)
		return false
	}

	eq.SetItem(slot, game.NewItem(-1, 0))
	if stackIntoExisting {
		inv.Add(item, false)
	} else {
		inv.SetItem(inventorySlot, item)
	}

	// BonusManager.update (line 275): no cached array to
	// refresh; bonuses compute live through
	// game.ComputeBonuses (see the equip() note above).
	if slot == game.WeaponSlot {
		// resetWeapon + CombatSpecial.updateBar (lines
		// 277-278): the assign half of resetWeapon runs here
		// (see the equip() note above); updateBar's orb
		// packets are Worker C (spec orb).
		game.AssignWeaponInterface(p)
	}

	// Refreshes + appearance flag (lines 280-281): as in equip,
	// through the refresh hooks.
	eq.RefreshItems()
	inv.RefreshItems()
	// Sounds.sendSound: GAP. No sound system exists.
	return true
}
