package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Tests for the bank opcode dispatch (bank.go, phase 46): the
// 243/244 deposit and 245 bank-move cases, the simple-click
// button-click fallback in the widget-action fallthrough, and
// the search-tab switch-slot route.

// recordingBankActions is a BankActions double recording every
// call, with scripted answers.
type recordingBankActions struct {
	depositInventory int
	depositEquipment int
	moves            []protocol.BankMove
	depositResult    bool
	moveResult       bool
}

func (b *recordingBankActions) DepositInventory(p *game.Player) bool {
	b.depositInventory++
	return b.depositResult
}

func (b *recordingBankActions) DepositEquipment(p *game.Player) bool {
	b.depositEquipment++
	return b.depositResult
}

func (b *recordingBankActions) Move(p *game.Player, move protocol.BankMove) bool {
	b.moves = append(b.moves, move)
	return b.moveResult
}

// openBank stages p with the bank open (status BANKING +
// interface 12, Bank.isOpen).
func openBank(p *game.Player) {
	p.SetStatus(game.PlayerStatusBanking)
	p.SetInterfaceId(game.BankMainInterfaceID)
}

func TestBankDepositDispatch(t *testing.T) {
	deposit := func(p *game.Player, equipment bool, deps Deps) Outcome {
		opcode := byte(protocol.ClientBankDepositInventory)
		if equipment {
			opcode = byte(protocol.ClientBankDepositEquipment)
		}
		return Handle(p, game.TurnPacket{Opcode: opcode}, 7, deps)
	}

	t.Run("nil player drops", func(t *testing.T) {
		for _, equipment := range []bool{false, true} {
			if got := deposit(nil, equipment, Deps{}); got.Reason != ReasonNoPlayer {
				t.Fatalf("equipment=%v: outcome = %+v, want ReasonNoPlayer", equipment, got)
			}
		}
	})

	t.Run("non-empty payload is malformed", func(t *testing.T) {
		p := testPlayer()
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientBankDepositInventory, Payload: []byte{0}}, 7, Deps{})
		if got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
			t.Fatalf("outcome = %+v, want malformed", got)
		}
	})

	t.Run("closed bank drops silently and never reaches the seam", func(t *testing.T) {
		p := testPlayer()
		bank := &recordingBankActions{depositResult: true}
		for _, equipment := range []bool{false, true} {
			if got := deposit(p, equipment, Deps{Bank: bank}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
				t.Fatalf("equipment=%v: outcome = %+v, want no-action", equipment, got)
			}
		}
		if bank.depositInventory != 0 || bank.depositEquipment != 0 {
			t.Fatal("closed bank reached the deposit seam")
		}
	})

	t.Run("open bank with no seam drops silently", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		if got := deposit(p, false, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("outcome = %+v, want no-action", got)
		}
	})

	t.Run("open bank deposits through the seam", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		bank := &recordingBankActions{depositResult: true}
		if got := deposit(p, false, Deps{Bank: bank}); !got.Implemented {
			t.Fatalf("outcome = %+v, want acted", got)
		}
		if bank.depositInventory != 1 || bank.depositEquipment != 0 {
			t.Fatalf("deposit calls = inv %d eq %d, want 1/0", bank.depositInventory, bank.depositEquipment)
		}
		if got := deposit(p, true, Deps{Bank: bank}); !got.Implemented {
			t.Fatalf("outcome = %+v, want acted", got)
		}
		if bank.depositEquipment != 1 {
			t.Fatalf("equipment deposits = %d, want 1", bank.depositEquipment)
		}
	})

	t.Run("refused deposit drops no-action", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		bank := &recordingBankActions{depositResult: false}
		if got := deposit(p, false, Deps{Bank: bank}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("outcome = %+v, want no-action", got)
		}
	})
}

func TestBankMoveDispatch(t *testing.T) {
	movePayload := []byte{0, 3, 0, 7, 1, 2} // from 3, to 7, insert, tab 2
	move := func(p *game.Player, payload []byte, deps Deps) Outcome {
		return Handle(p, game.TurnPacket{Opcode: protocol.ClientBankMove, Payload: payload}, 7, deps)
	}

	t.Run("nil player drops", func(t *testing.T) {
		if got := move(nil, movePayload, Deps{}); got.Reason != ReasonNoPlayer {
			t.Fatalf("outcome = %+v, want ReasonNoPlayer", got)
		}
	})

	t.Run("short payload is malformed even with no player", func(t *testing.T) {
		if got := move(nil, []byte{0, 1}, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
			t.Fatalf("outcome = %+v, want malformed", got)
		}
	})

	t.Run("closed bank drops silently and leaves insert mode alone", func(t *testing.T) {
		p := testPlayer()
		p.SetAttribute(game.BankInsertModeAttribute, false)
		bank := &recordingBankActions{moveResult: true}
		if got := move(p, movePayload, Deps{Bank: bank}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("outcome = %+v, want no-action", got)
		}
		if len(bank.moves) != 0 {
			t.Fatal("closed bank reached the move seam")
		}
		if game.BankInsertMode(p) {
			t.Fatal("closed-bank move set insert mode")
		}
	})

	t.Run("open bank sets insert mode then moves through the seam", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		p.SetAttribute(game.BankInsertModeAttribute, false)
		bank := &recordingBankActions{moveResult: true}
		if got := move(p, movePayload, Deps{Bank: bank}); !got.Implemented {
			t.Fatalf("outcome = %+v, want acted", got)
		}
		if !game.BankInsertMode(p) {
			t.Fatal("mode-1 move did not set insert mode")
		}
		if len(bank.moves) != 1 {
			t.Fatalf("moves = %d, want 1", len(bank.moves))
		}
		want := protocol.BankMove{From: 3, To: 7, Insert: true, Tab: 2, HasTab: true}
		if bank.moves[0] != want {
			t.Fatalf("move = %+v, want %+v", bank.moves[0], want)
		}
	})

	t.Run("swap mode clears insert mode", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		p.SetAttribute(game.BankInsertModeAttribute, true)
		bank := &recordingBankActions{moveResult: true}
		swap := []byte{0, 3, 0, 7, 0} // mode 0, no tab
		if got := move(p, swap, Deps{Bank: bank}); !got.Implemented {
			t.Fatalf("outcome = %+v, want acted", got)
		}
		if game.BankInsertMode(p) {
			t.Fatal("mode-0 move did not clear insert mode")
		}
		if len(bank.moves) != 1 || bank.moves[0].HasTab {
			t.Fatalf("moves = %+v, want one move with no tab", bank.moves)
		}
	})

	t.Run("open bank with no seam sets insert mode but drops no-action", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		p.SetAttribute(game.BankInsertModeAttribute, false)
		if got := move(p, movePayload, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("outcome = %+v, want no-action", got)
		}
		// The mode set is the drag's stated intent (see
		// bank.go): it lands even when the seam is unwired.
		if !game.BankInsertMode(p) {
			t.Fatal("unwired move did not set insert mode")
		}
	})

	t.Run("refused move drops no-action", func(t *testing.T) {
		p := testPlayer()
		openBank(p)
		bank := &recordingBankActions{moveResult: false}
		if got := move(p, movePayload, Deps{Bank: bank}); got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("outcome = %+v, want no-action", got)
		}
	})
}

func TestWidgetActionButtonClickFallback(t *testing.T) {
	// A simple IF_BUTTON click on a widget no branch claims:
	// group 500 (not bank, shop, combat, or inventory).
	msg := protocol.WidgetAction{
		WidgetID: int32(500<<16 | 7), GroupID: 500, ChildID: 7,
		HasButtonNum: true, ButtonNum: 1, Slot: -1, ItemID: -1, Simple: true,
	}

	t.Run("unclaimed simple click fires the button emit and skips the tail when claimed", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{buttonResult: true}
		got, access := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(hooks.buttonClicks) != 1 {
			t.Fatalf("button clicks = %d, want 1", len(hooks.buttonClicks))
		}
		if hooks.buttonClicks[0].ButtonID != int(msg.WidgetID) {
			t.Fatalf("button id = %d, want widget uid %d", hooks.buttonClicks[0].ButtonID, int(msg.WidgetID))
		}
		if len(access.optionCalls) != 0 {
			t.Fatalf("tail calls = %d, want 0 (claimed button click skips the tail)", len(access.optionCalls))
		}
	})

	t.Run("unclaimed simple click with no button claim runs the tail", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		got, access := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(hooks.buttonClicks) != 1 {
			t.Fatalf("button clicks = %d, want 1", len(hooks.buttonClicks))
		}
		if len(access.optionCalls) != 1 {
			t.Fatalf("tail calls = %d, want 1", len(access.optionCalls))
		}
	})

	t.Run("interface-action claim never reaches the button emit", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{emitResult: true}
		got, _ := widgetActionRoute(p, msg, Deps{InterfaceHooks: hooks.hooks()})
		if !got.Implemented {
			t.Fatalf("route = %+v, want acted", got)
		}
		if len(hooks.buttonClicks) != 0 {
			t.Fatal("claimed interface-action click reached the button emit")
		}
	})

	t.Run("non-simple clicks never reach the button emit", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		nonSimple := msg
		nonSimple.Simple = false
		got, _ := widgetActionRoute(p, nonSimple, Deps{InterfaceHooks: hooks.hooks()})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(hooks.buttonClicks) != 0 {
			t.Fatal("non-simple click reached the button emit")
		}
	})

	t.Run("nil button seam drops no-action", func(t *testing.T) {
		p := testPlayer()
		hooks := &recordingInterfaceHooks{}
		deps := Deps{InterfaceHooks: hooks.hooks()}
		deps.InterfaceHooks.EmitButtonClick = nil
		got, access := widgetActionRoute(p, msg, deps)
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("route = %+v, want no-action", got)
		}
		if len(access.optionCalls) != 1 {
			t.Fatalf("tail calls = %d, want 1", len(access.optionCalls))
		}
	})
}

func TestSwitchItemSlotBankSearchTab(t *testing.T) {
	// Searching routes the drag at the search tab (10),
	// not the interface-id tab (0).
	p := testPlayer()
	tab0 := p.Bank(0)
	tab0.SetItem(0, game.NewItem(itemBronzeSword, 1))
	tab0.SetItem(1, game.NewItem(itemIronSword, 1))
	tab0.SetItem(2, game.NewItem(itemRing, 1))
	search := p.Bank(game.BankSearchTabIndex)
	search.SetItem(0, game.NewItem(itemRing, 1))
	search.SetItem(1, game.NewItem(itemIronSword, 1))
	search.SetItem(2, game.NewItem(itemBronzeSword, 1))
	game.BankSetSearching(p, true)
	if !switchItemSlot(p, game.BankContainerStart, 0, 2) {
		t.Fatal("switchItemSlot refused a searching rearrange")
	}
	if got := search.ItemAt(0); got.ID() != itemBronzeSword {
		t.Errorf("search slot 0 = %+v, want bronze sword", got)
	}
	if got := search.ItemAt(2); got.ID() != itemRing {
		t.Errorf("search slot 2 = %+v, want ring", got)
	}
	// Tab 0 is untouched.
	if got := tab0.ItemAt(0); got.ID() != itemBronzeSword {
		t.Errorf("tab 0 slot 0 = %+v, want bronze sword (untouched)", got)
	}
	if got := tab0.ItemAt(2); got.ID() != itemRing {
		t.Errorf("tab 0 slot 2 = %+v, want ring (untouched)", got)
	}
}
