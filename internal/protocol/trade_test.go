package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeTradeRequest(t *testing.T) {
	got := EncodeTradeRequest(1, "Bob")
	want := []byte{154, 6, 0, 1, 'B', 'o', 'b', 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeTradeRequest(1, Bob) = %v, want %v", got, want)
	}
}

func TestEncodeTradeRequestMasksPlayerID(t *testing.T) {
	// The id is masked to 16 bits, as TS masks with & 0xffff.
	got := EncodeTradeRequest(0x10001, "")
	want := []byte{154, 3, 0, 1, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeTradeRequest masked = %v, want %v", got, want)
	}
}

func TestEncodeTradeClose(t *testing.T) {
	got := EncodeTradeClose("Trade declined.")
	want := append([]byte{157, 16}, []byte("Trade declined.")...)
	want = append(want, 0)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeTradeClose = %v, want %v", got, want)
	}
	if got := EncodeTradeClose(""); !bytes.Equal(got, []byte{157, 1, 0}) {
		t.Fatalf("EncodeTradeClose(\"\") = %v, want [157 1 0]", got)
	}
}

func TestEncodeTradeOpen(t *testing.T) {
	self := TradePartyView{PlayerID: 1, Name: "A", Offers: []TradeOfferView{{Slot: 0, ItemID: 995, Quantity: 100}}}
	other := TradePartyView{PlayerID: 2, Name: "B"}
	got := EncodeTradeOpen("1:2", TradeStageOffer, self, other, "")
	// Framing: opcode 155, 2-byte length 28. Payload: "1:2\0",
	// stage 0, "\0", party(00 01 "A\0" 0 0 1, offer 00 00 03 e3
	// 00 00 00 64), party(00 02 "B\0" 0 0 0).
	want := []byte{155, 0, 28,
		'1', ':', '2', 0, 0, 0,
		0, 1, 'A', 0, 0, 0, 1, 0, 0, 0x03, 0xe3, 0, 0, 0, 100,
		0, 2, 'B', 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeTradeOpen = %v, want %v", got, want)
	}
}

func TestEncodeTradeUpdateConfirmStage(t *testing.T) {
	self := TradePartyView{PlayerID: -1, Name: "", Accepted: true, ConfirmAccepted: true}
	other := TradePartyView{PlayerID: 7, Name: "Zed", Accepted: false, ConfirmAccepted: true}
	got := EncodeTradeUpdate("3:4", TradeStageConfirm, self, other, "info")
	// Payload: "3:4\0", stage 1, "info\0", party(ff ff "\0" 1 1
	// 0), party(00 07 "Zed\0" 0 1 0). Length 4+1+5+6+9 = 25.
	want := []byte{156, 0, 25,
		'3', ':', '4', 0, 1, 'i', 'n', 'f', 'o', 0,
		0xff, 0xff, 0, 1, 1, 0,
		0, 7, 'Z', 'e', 'd', 0, 0, 1, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeTradeUpdate = %v, want %v", got, want)
	}
}

func TestEncodeTradePartyOfferMasks(t *testing.T) {
	// Slot and item id mask to 16 bits; quantity is a full int32.
	party := TradePartyView{PlayerID: 0, Offers: []TradeOfferView{{Slot: 0x10002, ItemID: 0x10003, Quantity: -5}}}
	got := EncodeTradeOpen("", TradeStageOffer, party, TradePartyView{}, "")
	// Payload = "\0" + stage + "\0" + self + other; self = 2 + 1
	// + 3 + 8 = 14 bytes; other = 2 + 1 + 3 = 6.
	payload := got[3:]
	self := payload[3 : 3+14]
	offer := self[6:]
	want := []byte{0, 2, 0, 3, 0xff, 0xff, 0xff, 0xfb}
	if !bytes.Equal(offer, want) {
		t.Fatalf("offer payload = %v, want %v", offer, want)
	}
}

func TestDecodeTradeAction(t *testing.T) {
	payload := []byte{0, 0, 5, 0, 0, 3, 0xe8, 0x03, 0xe3} // offer, slot 5, qty 1000, item 995
	got, err := DecodeTradeAction(payload)
	if err != nil {
		t.Fatalf("DecodeTradeAction: %v", err)
	}
	want := TradeAction{Action: TradeActionOffer, Slot: 5, Quantity: 1000, ItemID: 995, HasItemID: true}
	if got != want {
		t.Fatalf("DecodeTradeAction = %+v, want %+v", got, want)
	}
}

func TestDecodeTradeActionKinds(t *testing.T) {
	kinds := []TradeActionKind{
		TradeActionOffer, TradeActionRemove, TradeActionAccept,
		TradeActionDecline, TradeActionConfirmAccept, TradeActionConfirmDecline,
	}
	for i, kind := range kinds {
		payload := []byte{byte(i), 0, 0, 0, 0, 0, 0, 0xff, 0xff}
		got, err := DecodeTradeAction(payload)
		if err != nil {
			t.Fatalf("action %d: %v", i, err)
		}
		if got.Action != kind {
			t.Fatalf("action %d decoded as %v, want %v", i, got.Action, kind)
		}
		// 0xffff maps to no item id.
		if got.HasItemID {
			t.Fatalf("action %d: HasItemID with 0xffff", i)
		}
	}
}

func TestDecodeTradeActionErrors(t *testing.T) {
	if _, err := DecodeTradeAction([]byte{6, 0, 0, 0, 0, 0, 0, 0, 0}); err == nil {
		t.Fatal("action byte 6: expected error")
	}
	if _, err := DecodeTradeAction([]byte{0, 0}); err == nil {
		t.Fatal("truncated payload: expected error")
	}
	if _, err := DecodeTradeAction(nil); err == nil {
		t.Fatal("empty payload: expected error")
	}
}
