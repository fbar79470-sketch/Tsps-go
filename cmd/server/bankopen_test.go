package main

import (
	"reflect"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestBankSenderOpenFrames(t *testing.T) {
	rec := &recordingSessionPort{}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = rec
	p.Bank(0).SetItem(0, game.NewItem(995, 100))
	p.Bank(1).SetItem(0, game.NewItem(4151, 1))
	view := game.BankOpenView{
		Opening: true,
		Varps:   map[int]int{548: 1, 262: 4, 4611: 0},
		Varbits: map[int]int{
			3755: 0, 3958: 0, 3959: 0, 3960: 0, 4150: 0, 4170: 0,
			4171: 1, 4172: 0, 4173: 0, 4174: 0, 4175: 0, 4176: 0,
			4177: 0, 4178: 0, 4179: 0, 5450: 1, 6590: 0,
		},
		ResetViewContainer: true,
	}
	bankSender{worldName: "Test World"}.SendBankOpen(p, view)
	want := [][]byte{
		protocol.EncodeWidgetRunScript(917, []protocol.ScriptArg{
			protocol.ScriptArgInt(-1), protocol.ScriptArgInt(-2),
		}, nil, nil, nil),
		// Varps/varbits in ascending id order.
		protocol.EncodeVarp(262, 4),
		protocol.EncodeVarp(548, 1),
		protocol.EncodeVarp(4611, 0),
		protocol.EncodeVarbit(3755, 0),
		protocol.EncodeVarbit(3958, 0),
		protocol.EncodeVarbit(3959, 0),
		protocol.EncodeVarbit(3960, 0),
		protocol.EncodeVarbit(4150, 0),
		protocol.EncodeVarbit(4170, 0),
		protocol.EncodeVarbit(4171, 1),
		protocol.EncodeVarbit(4172, 0),
		protocol.EncodeVarbit(4173, 0),
		protocol.EncodeVarbit(4174, 0),
		protocol.EncodeVarbit(4175, 0),
		protocol.EncodeVarbit(4176, 0),
		protocol.EncodeVarbit(4177, 0),
		protocol.EncodeVarbit(4178, 0),
		protocol.EncodeVarbit(4179, 0),
		protocol.EncodeVarbit(5450, 1),
		protocol.EncodeVarbit(6590, 0),
		// The two mounts, then the five flags ranges.
		protocol.EncodeWidgetOpenSub((161<<16)|16, 12, 0, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetOpenSub((161<<16)|74, 15, 3, protocol.WidgetOpenSubOptions{}),
		protocol.EncodeWidgetSetFlagsRangeV2((12<<16)|12, 0, 1409, game.BankItemSlotFlags, game.BankItemSlotOps),
		protocol.EncodeWidgetSetFlagsRange((12<<16)|12, game.BankTabDropSlotOffset, game.BankTabDropSlotOffset+9, 1<<20),
		protocol.EncodeWidgetSetFlagsRange((12<<16)|10, game.BankTabButtonSlotOffset, game.BankTabButtonSlotOffset+9, game.BankTabButtonFlags),
		protocol.EncodeWidgetSetFlagsRange((15<<16)|3, 0, 27, 3278846),
		protocol.EncodeWidgetSetFlagsRange((15<<16)|4, 0, 27, game.BankSideWornFlags),
		// Varbit 12393, the title string, the view reset.
		protocol.EncodeVarbit(12393, 1),
		protocol.EncodeWidgetSetText((12<<16)|3, "Bank of Test World", nil),
		protocol.EncodeVarbit(game.BankViewContainerVarbit, 0),
		protocol.EncodeVarp(game.BankViewContainerObjVarp, -1),
		// sendBankRefresh: tab-count varbits, the snapshot,
		// the inventory snapshot.
		protocol.EncodeVarbit(4171, 1),
		protocol.EncodeVarbit(4172, 0),
		protocol.EncodeVarbit(4173, 0),
		protocol.EncodeVarbit(4174, 0),
		protocol.EncodeVarbit(4175, 0),
		protocol.EncodeVarbit(4176, 0),
		protocol.EncodeVarbit(4177, 0),
		protocol.EncodeVarbit(4178, 0),
		protocol.EncodeVarbit(4179, 0),
		protocol.EncodeBankSnapshot(game.MaxBankSlots, bankLayoutSlots(p, nil)),
		protocol.EncodeInventorySnapshot(itemSlots(p.Inventory().Items())),
	}
	if len(rec.packets) != len(want) {
		t.Fatalf("sent %d frames, want %d", len(rec.packets), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(rec.packets[i], want[i]) {
			t.Errorf("frame %d = %v, want %v", i, rec.packets[i], want[i])
		}
	}

	// Without the view reset, the reset frames are absent and
	// the sequence otherwise matches.
	rec.packets = nil
	view.ResetViewContainer = false
	bankSender{worldName: "Test World"}.SendBankOpen(p, view)
	for _, packet := range rec.packets {
		if reflect.DeepEqual(packet, protocol.EncodeVarbit(game.BankViewContainerVarbit, 0)) ||
			reflect.DeepEqual(packet, protocol.EncodeVarp(game.BankViewContainerObjVarp, -1)) {
			t.Error("view reset frames sent without ResetViewContainer")
		}
	}
	if len(rec.packets) != len(want)-2 {
		t.Fatalf("sent %d frames without reset, want %d", len(rec.packets), len(want)-2)
	}

	// Nil player/session send nothing and never panic.
	bankSender{}.SendBankOpen(nil, view)
	bankSender{}.SendBankOpen(game.NewPlayer("nosession", game.Location{}), view)
	bankSender{}.SendAmountPrompt(nil, "Enter amount:")
	bankSender{}.SendInputPrompt(nil, "What do you wish to search for?")
}

func TestBankSenderPrompts(t *testing.T) {
	rec := &recordingSessionPort{}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = rec
	bankSender{}.SendAmountPrompt(p, "Enter amount:")
	bankSender{}.SendInputPrompt(p, "What do you wish to search for?")
	want := [][]byte{
		protocol.EncodeRunClientScript(108, []protocol.ScriptArg{protocol.ScriptArgString("Enter amount:")}),
		protocol.EncodeRunClientScript(110, []protocol.ScriptArg{
			protocol.ScriptArgString("What do you wish to search for?"),
			protocol.ScriptArgInt(0),
		}),
	}
	if len(rec.packets) != len(want) {
		t.Fatalf("sent %d frames, want %d", len(rec.packets), len(want))
	}
	for i := range want {
		if !reflect.DeepEqual(rec.packets[i], want[i]) {
			t.Errorf("frame %d = %v, want %v", i, rec.packets[i], want[i])
		}
	}
}

func TestInstallBankSenderWiresGameSender(t *testing.T) {
	rec := &recordingSessionPort{}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = rec
	installBankSender("Test World")
	t.Cleanup(func() { game.SetBankSender(nil) })
	game.BankOpen(p)
	if !game.BankIsOpen(p) {
		t.Fatal("BankOpen did not open the bank")
	}
	if len(rec.packets) == 0 {
		t.Fatal("no frames sent after installBankSender")
	}
	if !reflect.DeepEqual(rec.packets[0], protocol.EncodeWidgetRunScript(917, []protocol.ScriptArg{
		protocol.ScriptArgInt(-1), protocol.ScriptArgInt(-2),
	}, nil, nil, nil)) {
		t.Errorf("first frame = %v, want script 917", rec.packets[0])
	}
	found := false
	for _, packet := range rec.packets {
		if reflect.DeepEqual(packet, protocol.EncodeWidgetSetText((12<<16)|3, "Bank of Test World", nil)) {
			found = true
		}
	}
	if !found {
		t.Error("title frame \"Bank of Test World\" not sent")
	}
}

func TestBankListenerActions(t *testing.T) {
	rec := &recordingSessionPort{}
	p := game.NewPlayer("alice", game.Location{X: 3000, Y: 3000})
	p.Session = rec
	actions := bankListenerActions()
	// Closed bank: everything refuses.
	if actions.DepositInventory(p) || actions.DepositEquipment(p) ||
		actions.Move(p, protocol.BankMove{From: 0, To: 1}) {
		t.Fatal("bank actions ran with the bank closed")
	}
	game.BankOpen(p)
	if !game.BankIsOpen(p) {
		t.Fatal("BankOpen did not open the bank")
	}
	p.Inventory().SetItem(0, game.NewItem(995, 10))
	if !actions.DepositInventory(p) {
		t.Fatal("DepositInventory returned false with the bank open")
	}
	if got := p.Bank(0).GetAmount(995); got != 10 {
		t.Fatalf("bank coins after deposit-inventory = %d, want 10", got)
	}
	p.Equipment().SetItem(0, game.NewItem(2, 1))
	if !actions.DepositEquipment(p) {
		t.Fatal("DepositEquipment returned false with the bank open")
	}
	if got := p.Bank(0).GetAmount(2); got != 1 {
		t.Fatalf("bank cannonballs after deposit-worn = %d, want 1", got)
	}
	// 245 move: swap the two tab-0 stacks.
	p.Bank(0).SetItem(0, game.NewItem(4151, 1))
	p.Bank(0).SetItem(1, game.NewItem(1323, 1))
	if !actions.Move(p, protocol.BankMove{From: 0, To: 1}) {
		t.Fatal("Move returned false with the bank open")
	}
	layout := game.BankLayout(p)
	if len(layout) < 2 || layout[0].Item.ID() != 1323 || layout[1].Item.ID() != 4151 {
		t.Fatalf("layout after move = %v, want scimitar before whip", layout)
	}
}
