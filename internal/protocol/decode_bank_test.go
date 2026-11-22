package protocol

import "testing"

// Tests for the bank opcode decoders (phase 46): deposit
// inventory (243), deposit equipment (244), bank move (245).

func TestDecodeBankDepositInventory(t *testing.T) {
	if err := DecodeBankDepositInventory(nil); err != nil {
		t.Fatalf("empty payload errored: %v", err)
	}
	if err := DecodeBankDepositInventory([]byte{}); err != nil {
		t.Fatalf("empty slice errored: %v", err)
	}
	if err := DecodeBankDepositInventory([]byte{0}); err == nil {
		t.Fatal("non-empty payload did not error")
	}
}

func TestDecodeBankDepositEquipment(t *testing.T) {
	if err := DecodeBankDepositEquipment(nil); err != nil {
		t.Fatalf("empty payload errored: %v", err)
	}
	if err := DecodeBankDepositEquipment([]byte{1, 2}); err == nil {
		t.Fatal("non-empty payload did not error")
	}
}

func TestDecodeBankMove(t *testing.T) {
	// Full payload: from 3, to 7, mode 1 (insert), tab 2.
	msg, err := DecodeBankMove([]byte{0, 3, 0, 7, 1, 2})
	if err != nil {
		t.Fatalf("decode errored: %v", err)
	}
	if msg.From != 3 || msg.To != 7 || !msg.Insert || !msg.HasTab || msg.Tab != 2 {
		t.Fatalf("decoded = %+v, want {3 7 insert tab 2}", msg)
	}
	// No tab byte: tab stays absent.
	msg, err = DecodeBankMove([]byte{0, 3, 0, 7, 1})
	if err != nil {
		t.Fatalf("decode errored: %v", err)
	}
	if msg.From != 3 || msg.To != 7 || !msg.Insert || msg.HasTab {
		t.Fatalf("decoded = %+v, want {3 7 insert no tab}", msg)
	}
	// Mode 0 = swap; any other mode byte is also swap.
	for _, mode := range []byte{0, 2, 255} {
		msg, err := DecodeBankMove([]byte{0, 0, 0, 1, mode})
		if err != nil {
			t.Fatalf("mode %d errored: %v", mode, err)
		}
		if msg.Insert {
			t.Fatalf("mode %d decoded insert, want swap", mode)
		}
	}
	// Trailing bytes after the tab are ignored (variable
	// length, like the other variable-length decodes).
	msg, err = DecodeBankMove([]byte{0, 3, 0, 7, 0, 4, 9, 9})
	if err != nil {
		t.Fatalf("trailing-bytes decode errored: %v", err)
	}
	if msg.From != 3 || msg.To != 7 || msg.Insert || !msg.HasTab || msg.Tab != 4 {
		t.Fatalf("decoded = %+v, want {3 7 swap tab 4}", msg)
	}
	// Short payloads error (from/to/mode are all required).
	for _, payload := range [][]byte{nil, {}, {0}, {0, 1}, {0, 1, 0, 2}} {
		if _, err := DecodeBankMove(payload); err == nil {
			t.Fatalf("payload %v did not error", payload)
		}
	}
	// Slots read as unsigned shorts, like the inventory-move
	// decoder: 0xffff passes through as 65535, for the
	// downstream display-slot resolution to refuse.
	msg, err = DecodeBankMove([]byte{0xff, 0xff, 0, 0, 0})
	if err != nil {
		t.Fatalf("decode errored: %v", err)
	}
	if msg.From != 65535 || msg.To != 0 {
		t.Fatalf("decoded = %+v, want from 65535 to 0", msg)
	}
}
