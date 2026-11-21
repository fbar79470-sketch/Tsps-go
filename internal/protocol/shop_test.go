package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestEncodeShopClose(t *testing.T) {
	if got := EncodeShopClose(); !bytes.Equal(got, []byte{ServerShopClose}) {
		t.Fatalf("EncodeShopClose = %v, want [%d]", got, ServerShopClose)
	}
}

func TestEncodeShopOpenEmptyStock(t *testing.T) {
	got := EncodeShopOpen("0", "General Store", 995, true, 1, 1, nil)
	// Opcode + u16 length prefix, then latin1 NUL-terminated id
	// and name, then the 7-byte header.
	want := []byte{ServerShopOpen, 0, 0} // length patched below
	payload := []byte("0\x00General Store\x00")
	header := []byte{0x03, 0xe3, 1, 1, 1, 0, 0} // 995, general, modes, 0 rows
	payload = append(payload, header...)
	want[1] = byte(len(payload) >> 8)
	want[2] = byte(len(payload))
	want = append(want, payload...)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeShopOpen empty =\n%x\nwant\n%x", got, want)
	}
}

func TestEncodeShopOpenOneSlot(t *testing.T) {
	stock := []ShopSlotView{{Slot: 0, ItemID: 1933, Quantity: 5, DefaultQuantity: 10, PriceEach: 3, SellPrice: 2}}
	got := EncodeShopOpen("12", "Gerrant's Fishy Business", 995, false, 1, 1, stock)
	// Decode the frame back through DecodeFrame's server-side
	// shape by hand: opcode, u16 length, payload.
	if got[0] != ServerShopOpen {
		t.Fatalf("opcode = %d, want %d", got[0], ServerShopOpen)
	}
	length := int(got[1])<<8 | int(got[2])
	payload := got[3:]
	if len(payload) != length {
		t.Fatalf("payload length = %d, declared %d", len(payload), length)
	}
	// id "12\x00", name + NUL, header, one 20-byte slot.
	wantPrefix := []byte("12\x00Gerrant's Fishy Business\x00")
	if !bytes.Equal(payload[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("id/name prefix = %q, want %q", payload[:len(wantPrefix)], wantPrefix)
	}
	rest := payload[len(wantPrefix):]
	if len(rest) != 7+20 {
		t.Fatalf("header+slots = %d bytes, want 27", len(rest))
	}
	if got, want := binary.BigEndian.Uint16(rest[0:2]), uint16(995); got != want {
		t.Fatalf("currency item id = %d, want %d", got, want)
	}
	if rest[2] != 0 || rest[3] != 1 || rest[4] != 1 {
		t.Fatalf("general/modes = %v, want [0 1 1]", rest[2:5])
	}
	if got := binary.BigEndian.Uint16(rest[5:7]); got != 1 {
		t.Fatalf("stock count = %d, want 1", got)
	}
	slot := rest[7:]
	if got := binary.BigEndian.Uint16(slot[0:2]); got != 0 {
		t.Fatalf("slot = %d, want 0", got)
	}
	if got := binary.BigEndian.Uint16(slot[2:4]); got != 1933 {
		t.Fatalf("item id = %d, want 1933", got)
	}
	if got := binary.BigEndian.Uint32(slot[4:8]); got != 5 {
		t.Fatalf("quantity = %d, want 5", got)
	}
	if got := binary.BigEndian.Uint32(slot[8:12]); got != 10 {
		t.Fatalf("default quantity = %d, want 10", got)
	}
	if got := binary.BigEndian.Uint32(slot[12:16]); got != 3 {
		t.Fatalf("price each = %d, want 3", got)
	}
	if got := binary.BigEndian.Uint32(slot[16:20]); got != 2 {
		t.Fatalf("sell price = %d, want 2", got)
	}
}

func TestEncodeShopOpenMasks(t *testing.T) {
	// TS masks slot/item id with & 0xffff and truncates the
	// 32-bit fields with | 0.
	stock := []ShopSlotView{{Slot: 0x10001, ItemID: 0x1FFFF, Quantity: -1, DefaultQuantity: 1 << 33, PriceEach: 0, SellPrice: -5}}
	got := EncodeShopOpen("1", "S", -1, false, 0x101, 0x102, stock)
	payload := got[3:]
	rest := payload[len("1\x00S\x00"):]
	if got := binary.BigEndian.Uint16(rest[0:2]); got != 0xffff {
		t.Fatalf("currency item id = %d, want 0xffff", got)
	}
	if rest[3] != 1 || rest[4] != 2 {
		t.Fatalf("mode bytes = [%d %d], want [1 2]", rest[3], rest[4])
	}
	slot := rest[7:]
	if got := binary.BigEndian.Uint16(slot[0:2]); got != 1 {
		t.Fatalf("slot = %d, want 1", got)
	}
	if got := binary.BigEndian.Uint16(slot[2:4]); got != 0xffff {
		t.Fatalf("item id = %d, want 0xffff", got)
	}
	if got := int32(binary.BigEndian.Uint32(slot[4:8])); got != -1 {
		t.Fatalf("quantity = %d, want -1", got)
	}
	if got := int32(binary.BigEndian.Uint32(slot[8:12])); got != 0 {
		t.Fatalf("default quantity = %d, want 0 (truncated)", got)
	}
	if got := int32(binary.BigEndian.Uint32(slot[16:20])); got != -5 {
		t.Fatalf("sell price = %d, want -5", got)
	}
}
