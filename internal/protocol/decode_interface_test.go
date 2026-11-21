package protocol

import (
	"encoding/binary"
	"testing"
)

func TestDecodeDialogueAmount(t *testing.T) {
	payload := make([]byte, 4)
	amount := int32(-7)
	binary.BigEndian.PutUint32(payload, uint32(amount))
	got, err := DecodeDialogueAmount(payload)
	if err != nil {
		t.Fatalf("DecodeDialogueAmount: %v", err)
	}
	if got.Amount != -7 {
		t.Fatalf("Amount = %d, want -7", got.Amount)
	}
	if _, err := DecodeDialogueAmount([]byte{0, 0, 0}); err == nil {
		t.Error("short payload must error")
	}
	if _, err := DecodeDialogueAmount([]byte{0, 0, 0, 0, 0}); err == nil {
		t.Error("trailing byte must error")
	}
}

func TestDecodeDialogueInput(t *testing.T) {
	got, err := DecodeDialogueInput([]byte{'h', 'i', 0})
	if err != nil {
		t.Fatalf("DecodeDialogueInput: %v", err)
	}
	if got.Value != "hi" {
		t.Fatalf("Value = %q, want hi", got.Value)
	}
	// Trailing bytes after the terminator are ignored, as in TS.
	got, err = DecodeDialogueInput([]byte{'a', 0, 'j', 'u', 'n', 'k'})
	if err != nil || got.Value != "a" {
		t.Fatalf("trailing bytes: got %+v, %v", got, err)
	}
	if _, err := DecodeDialogueInput([]byte{'n', 'o', 'n', 'u', 'l'}); err == nil {
		t.Error("unterminated string must error")
	}
	if _, err := DecodeDialogueInput(nil); err == nil {
		t.Error("empty payload must error")
	}
}

func TestDecodeWidget(t *testing.T) {
	got, err := DecodeWidget([]byte{1, 0, 12, 1})
	if err != nil {
		t.Fatalf("DecodeWidget: %v", err)
	}
	if got.Action != WidgetClosed || got.GroupID != 12 || !got.Modal {
		t.Fatalf("got %+v, want close/12/modal", got)
	}
	got, err = DecodeWidget([]byte{0, 1, 44, 0})
	if err != nil {
		t.Fatalf("DecodeWidget: %v", err)
	}
	if got.Action != WidgetOpened || got.GroupID != 300 || got.Modal {
		t.Fatalf("got %+v, want open/300/non-modal", got)
	}
	// Any nonzero action byte means close.
	got, err = DecodeWidget([]byte{7, 0, 1, 0})
	if err != nil || got.Action != WidgetClosed {
		t.Fatalf("action 7: got %+v, %v", got, err)
	}
	// Trailing bytes are ignored, as in TS.
	if _, err := DecodeWidget([]byte{1, 0, 12, 0, 9, 9}); err != nil {
		t.Fatalf("trailing bytes: %v", err)
	}
	if _, err := DecodeWidget([]byte{1, 0}); err == nil {
		t.Error("short payload must error")
	}
}

func TestDecodeInterfaceClose(t *testing.T) {
	if err := DecodeInterfaceClose(nil); err != nil {
		t.Fatalf("empty payload: %v", err)
	}
	if err := DecodeInterfaceClose([]byte{}); err != nil {
		t.Fatalf("empty slice: %v", err)
	}
	if err := DecodeInterfaceClose([]byte{0}); err == nil {
		t.Error("non-empty payload must error")
	}
}

func TestEncodeTriggerOnDialogAbort(t *testing.T) {
	got := EncodeTriggerOnDialogAbort()
	// Opcode 123, fixed length 0: a single opcode byte, no length
	// prefix and no payload.
	if len(got) != 1 || got[0] != ServerTriggerOndialogabort {
		t.Fatalf("EncodeTriggerOnDialogAbort = %v, want [123]", got)
	}
}
