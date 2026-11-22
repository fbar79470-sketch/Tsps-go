package main

import (
	"tsps-go/internal/content/bank"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
	"tsps-go/internal/protocol"
)

// Bank-flow wiring: the cmd/server adapter between Worker B's
// bank core flows (internal/game: BankOpen, BankIsOpen,
// BankHandleButton, BankHandleWidgetAction — PHASE46_PLAN.md
// §4) and the BankBooths content package's Flows seam
// (internal/content/bank.Flows, installed via
// bank.InstallFlows).
//
// The flows are installed here, not in the content package:
// internal/game cannot import internal/plugins (the game
// layer never imports the plugin layer), so the
// *plugins.InterfaceActionClickEvent → game.BankWidgetPacket
// translation lives in this adapter. The game side takes a
// plain packet (BankWidgetPacket{GroupID, ChildID, ButtonNum,
// Option, Slot, ItemID}); the translation below maps the
// event's absent slot/item to -1, which is what the game side
// gates on (bankHandleWidget: `packet.Slot >= 0`,
// `packet.ItemID >= 0`), and the absent option to "" (the game
// side lower-cases and compares it against "release", so ""
// reads as "no option", the TS undefined form).
//
// The coordinator calls installBankFlows from buildServer
// after loadGameDefinitions (the installShopHubSurfaces
// precedent, server.go). Until then the content package falls
// through on every open/button/widget call instead of claiming
// a click the bank cannot answer (see
// internal/content/bank/bank.go).
//
// NOT wired here: the open-sequence packet sends themselves
// (script 917, mounts, flags, "Bank of <world>" string) —
// those go through Worker B's BankSender seam
// (game.SetBankSender), installed by installBankSender in
// containerswiring.go. With no sender installed BankOpen
// still sets status BANKING + interface 12 and refreshes tabs
// through the game layer; the client-visible sends wait for
// the sender.
func installBankFlows() {
	// Equip/unequip seams (bankstate.go): the bank side panel
	// equips and the worn panel unequips through the
	// listener's EquipPacketListener routes. Installed once
	// at boot, like the sender; tests that need them install
	// fakes and restore nil afterwards.
	game.BankEquipHook = listeners.EquipForBank
	game.BankUnequipHook = listeners.UnequipForBank
	bank.InstallFlows(bank.Flows{
		IsOpen: game.BankIsOpen,
		Open: func(p *game.Player) bool {
			// game.BankOpen is void and nil-safe; openBank
			// already checked !bankIsOpen and the player
			// non-nil, so the click is claimed.
			game.BankOpen(p)
			return true
		},
		HandleButton: game.BankHandleButton,
		HandleWidget: func(ev *plugins.InterfaceActionClickEvent) bool {
			if ev == nil || ev.Player == nil {
				return false
			}
			slot := -1
			if ev.HasSlot {
				slot = ev.Slot
			}
			itemID := -1
			if ev.HasItemID {
				itemID = ev.ItemID
			}
			option := ""
			if ev.HasOption {
				option = ev.Option
			}
			return game.BankHandleWidgetAction(ev.Player, game.BankWidgetPacket{
				GroupID:   ev.GroupID,
				ChildID:   ev.ChildID,
				ButtonNum: ev.Action,
				Option:    option,
				Slot:      slot,
				ItemID:    itemID,
			})
		},
	})
}

// bankActions implements listeners.BankActions (the 243/244/245
// listener seam, internal/listeners/bank.go) over Worker B's
// bank core flows. The listener sets the insert mode from the
// 245 packet before calling Move, so Move only resolves the
// display slots and rearranges (game.BankMoveSlots). The
// deposit calls report whether the bank was open — the
// listener's routeBankDeposit gate — so a deposit from a
// closed bank reads as unhandled rather than silently
// succeeding.
type bankActions struct{}

// bankListenerActions returns the BankActions implementation
// the coordinator installs as deps.Bank in AddPlayer
// (server.go). Stateless: one shared value serves every
// player.
func bankListenerActions() listeners.BankActions { return bankActions{} }

func (bankActions) DepositInventory(p *game.Player) bool {
	if p == nil || !game.BankIsOpen(p) {
		return false
	}
	game.BankDepositItems(p, p.Inventory(), false)
	return true
}

func (bankActions) DepositEquipment(p *game.Player) bool {
	if p == nil || !game.BankIsOpen(p) {
		return false
	}
	game.BankDepositItems(p, p.Equipment(), false)
	return true
}

func (bankActions) Move(p *game.Player, move protocol.BankMove) bool {
	return game.BankMoveSlots(p, move.From, move.To, move.Insert)
}
