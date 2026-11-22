// Switch-slot: SwitchItemSlotPacketListener.ts (56 lines, commit
// bfd81e3). The listener moves one item slot to another, in the
// bank or in the inventory, depending on the interface the drag
// arrived on.
//
// Degenerations, documented at their sites:
//   - The hitpoints gate is implementable via Skills and is
//     ported.
//   - The bank path reads the open bank tab from the interface id
//     (interfaceId - Bank.CONTAINER_START), or tab 10 when the
//     player is searching the bank. No bank-search state exists
//     in Go, so searching degenerates to false and the tab always
//     comes from the interface id.
//   - Bank.rearrange takes insert mode as a parameter in Go. TS
//     reads the BANK_INSERT_MODE_ATTRIBUTE player attribute
//     (player.insertModeReturn, Player.ts:1452-1454); the listener
//     reads the same attribute from the player's store and passes
//     it in, so insert mode is live and defaults to false (swap
//     mode) when the attribute is unset.
//   - Bank.reconfigureTabs (moving into/out of slot 0 of a tab)
//     is a tab-UI concern with no tab state in Go; see
//     internal/game/bank.go.
//   - The inventory path fires for interface ids 3214 (inventory),
//     5064 (bank inventory), and 1688 (equipment inventory). TS
//     knows the open interface from player state; with no
//     open-interface tracking in Go, the interface id arrives in
//     the packet, and any of the three ids selects the inventory
//     swap. Bank.handleDrag — which would intercept drags while
//     the bank is open — always returns false in Go (no bank-open
//     state), so every drag falls through to this listener, as
//     TS falls through when the bank is closed
//     (Bank.ts:607-617).

package listeners

import (
	"tsps-go/internal/game"
)

// switchItemSlot moves the item at fromSlot to toSlot on the
// interface interfaceID. SwitchItemSlotPacketListener.ts:20-55
// (move). It returns true when a move happened; false when any
// gate refused. TS move() returns void; the boolean is the Go
// form the packet Outcome needs.
func switchItemSlot(p *game.Player, interfaceID, fromSlot, toSlot int) bool {
	if !playerReady(p) {
		return false
	}
	// TS: `if (!player || player.getHitpoints() <= 0) return;`
	// (line 20). Handle guarantees a non-nil player.
	if hitpoints(p) <= 0 {
		return false
	}

	// Bank path (lines 23-36): interface ids
	// [Bank.CONTAINER_START, +TOTAL_BANK_TABS) = [50300, 50311).
	// The tab is the search tab (10) when searching, else the
	// interface id offset (BankSearching, bankstate.go).
	if interfaceID >= game.BankContainerStart && interfaceID < game.BankContainerStart+game.TotalBankTabs {
		tab := interfaceID - game.BankContainerStart
		if game.BankSearching(p) {
			tab = game.BankSearchTabIndex
		}
		bank := p.Bank(tab)
		if fromSlot < 0 || fromSlot >= bank.Capacity() ||
			toSlot < 0 || toSlot >= bank.Capacity() ||
			toSlot == fromSlot {
			return false
		}
		// Insert mode: the player's bank:insert-mode attribute,
		// strictly true (Player.insertModeReturn,
		// Player.ts:1452-1454). Unset reads false (swap mode),
		// as in TS; Bank.rearrange (Bank.ts:1067-1068) branches
		// on the same value.
		bank.Rearrange(fromSlot, toSlot, p.GetAttribute(game.BankInsertModeAttribute) == true)
		return true
	}

	// Inventory path (lines 38-55): interfaces 3214 (inventory),
	// 5064 (bank inventory), 1688 (equipment inventory). A swap
	// plus a refresh. Any other interface id does nothing.
	switch interfaceID {
	case game.InventoryInterfaceID,
		game.BankInventoryInterfaceID,
		game.EquipmentInventoryInterfaceID:
	default:
		return false
	}
	inv := p.Inventory()
	if fromSlot < 0 || fromSlot >= inv.Capacity() ||
		toSlot < 0 || toSlot >= inv.Capacity() ||
		toSlot == fromSlot {
		return false
	}
	inv.Swap(fromSlot, toSlot)
	inv.RefreshItems()
	return true
}
