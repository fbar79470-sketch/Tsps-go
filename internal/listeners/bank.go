package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Bank opcode dispatch: the bank deposit/move cases
// (NetworkBuilder.ts:860-878), reached from Handle for
// opcodes 243 (deposit inventory), 244 (deposit equipment),
// and 245 (bank move). All three require the bank open
// (game.BankIsOpen: status BANKING and interface id 12,
// Bank.ts:285-288); a closed bank drops the packet silently
// with ReasonNoAction, as TS returns early from the case.
//
// The deposit/move flows themselves live in internal/game
// (Worker B) behind the BankActions seam, following the
// ShopWidgetHandler precedent (shopwidget.go): the
// coordinator wires it in cmd/server to the bank flows, and
// a nil seam degrades to ReasonNoAction — the flows are not
// yet wired, so the packet is routed and dropped rather than
// answered with invented behaviour. The open check runs in
// the listener, not the seam: it needs no flow state, and a
// closed bank never reaches the flows at all.
//
// The bank-move order is TS order (NetworkBuilder.ts:860-878):
// the listener sets the insert mode from the packet's mode
// byte (mode 1 = insert, else swap) before the seam runs, and
// the seam resolves the display slots, checks the move stays
// in one tab, and runs the rearrange. The mode set is the
// drag's stated intent, so it lands even when the seam is
// unwired — exactly as TS sets it before calling
// bank.rearrange, which may then refuse the move.

// BankActions is the seam between the bank-opcode dispatch and
// the bank flow functions (internal/game, Worker B). The
// coordinator binds it to the bank flows at integration. Every
// method reports whether the action was consumed, following
// the TS return contracts: the deposit calls return false
// when the bank is not open (the listener's open check makes
// that unreachable in production) and the move returns false
// when the display slots do not resolve or the move crosses
// tabs. A nil handler means no bank flows are wired: the
// routes below return ReasonNoAction and the packet is
// dropped.
type BankActions interface {
	// DepositInventory deposits the whole inventory into the
	// bank (Bank.depositItems(inventory), Bank.ts:376-425).
	DepositInventory(p *game.Player) bool
	// DepositEquipment deposits all worn equipment into the
	// bank (Bank.depositItems(equipment)).
	DepositEquipment(p *game.Player) bool
	// Move rearranges one bank stack: resolve the from/to
	// display slots against the viewed tab (or the search
	// tab when searching), refuse a cross-tab move, then run
	// the rearrange in the player's current insert mode
	// (Bank.switchsItem / Bank.rearrange, Bank.ts:1438-1558,
	// 1067-1095). The insert mode is set by the listener
	// before this call, from the packet's mode byte.
	Move(p *game.Player, move protocol.BankMove) bool
}

// routeBankDeposit routes one decoded bank-deposit packet
// (opcodes 243/244, NetworkBuilder.ts:860-878): the deposit
// runs only while the bank is open, through the BankActions
// seam. A nil player drops with ReasonNoPlayer; a closed bank
// or an unwired seam drops with ReasonNoAction; a consumed
// deposit counts as acted.
func routeBankDeposit(p *game.Player, equipment bool, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if !game.BankIsOpen(p) {
		return noAction()
	}
	if deps.Bank == nil {
		return noAction()
	}
	var done bool
	if equipment {
		done = deps.Bank.DepositEquipment(p)
	} else {
		done = deps.Bank.DepositInventory(p)
	}
	if done {
		return acted()
	}
	return noAction()
}

// routeBankMove routes one decoded bank-move packet (opcode
// 245, NetworkBuilder.ts:860-878): the move runs only while
// the bank is open. The listener sets the insert mode from
// the packet's mode byte first (mode 1 = insert, else swap),
// then delegates the display-slot resolution, the same-tab
// check, and the rearrange to the BankActions seam. A nil
// player drops with ReasonNoPlayer; a closed bank or an
// unwired seam drops with ReasonNoAction (the insert-mode set
// still lands: it is the drag's stated intent, as in TS);
// a consumed move counts as acted.
func routeBankMove(p *game.Player, move protocol.BankMove, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if !game.BankIsOpen(p) {
		return noAction()
	}
	game.BankSetInsertMode(p, move.Insert)
	if deps.Bank == nil {
		return noAction()
	}
	if deps.Bank.Move(p, move) {
		return acted()
	}
	return noAction()
}
