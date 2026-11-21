package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

// groundItemVector is one stack with every field set, for byte-exact
// vectors hand-computed from ClientProtocol.ts:1492-1508.
var groundItemVector = GroundItemView{
	EnabledOps:       0x1f,
	ID:               7,
	ItemID:           995,
	Quantity:         250,
	X:                3200,
	Y:                3200,
	Level:            0,
	CreatedTick:      100,
	PrivateUntilTick: 200,
	ExpiresTick:      400,
	OwnerID:          3,
	IsPrivate:        true,
	Ownership:        1,
}

// groundItemVectorBytes is the 34-byte record for groundItemVector:
// id 00 00 00 07, itemId 03 e3, quantity 00 00 00 fa, x 0c 80,
// y 0c 80, level 00, created 00 00 00 64, private-until 00 00 00 c8,
// expires 00 00 01 90, owner 00 00 00 03, private 01, ownership 01,
// ops 1f.
var groundItemVectorBytes = []byte{
	0x00, 0x00, 0x00, 0x07,
	0x03, 0xe3,
	0x00, 0x00, 0x00, 0xfa,
	0x0c, 0x80,
	0x0c, 0x80,
	0x00,
	0x00, 0x00, 0x00, 0x64,
	0x00, 0x00, 0x00, 0xc8,
	0x00, 0x00, 0x01, 0x90,
	0x00, 0x00, 0x00, 0x03,
	0x01,
	0x01,
	0x1f,
}

func TestEncodeGroundItemRecord(t *testing.T) {
	if got := encodeGroundItem(groundItemVector); !bytes.Equal(got, groundItemVectorBytes) {
		t.Fatalf("record = %x\nwant %x", got, groundItemVectorBytes)
	}
	if len(groundItemVectorBytes) != GroundItemBytes {
		t.Fatalf("vector length %d, want GroundItemBytes %d", len(groundItemVectorBytes), GroundItemBytes)
	}
}

func TestEncodeGroundItems(t *testing.T) {
	got := EncodeGroundItems(1, []GroundItemView{groundItemVector})
	// Opcode 54, 2-byte length prefix (payload 6 + 34 = 40), header
	// serial 1 + count 1, then the record.
	want := []byte{54, 0x00, 0x28, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01}
	want = append(want, groundItemVectorBytes...)
	if !bytes.Equal(got, want) {
		t.Fatalf("packet = %x\nwant %x", got, want)
	}
}

func TestEncodeGroundItemsEmpty(t *testing.T) {
	got := EncodeGroundItems(9, nil)
	want := []byte{54, 0x00, 0x06, 0x00, 0x00, 0x00, 0x09, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("packet = %x, want %x", got, want)
	}
}

func TestEncodeGroundItemsMultiple(t *testing.T) {
	second := groundItemVector
	second.ID = 8
	second.ItemID = 1234
	got := EncodeGroundItems(2, []GroundItemView{groundItemVector, second})
	if len(got) != 3+6+2*GroundItemBytes {
		t.Fatalf("length=%d, want %d", len(got), 3+6+2*GroundItemBytes)
	}
	if got[0] != ServerGroundItems {
		t.Fatalf("opcode=%d, want %d", got[0], ServerGroundItems)
	}
	if n := binary.BigEndian.Uint16(got[1:3]); n != 6+2*GroundItemBytes {
		t.Fatalf("length prefix=%d, want %d", n, 6+2*GroundItemBytes)
	}
	if !bytes.Equal(got[3+6:3+6+GroundItemBytes], groundItemVectorBytes) {
		t.Fatalf("first record = %x", got[3+6:3+6+GroundItemBytes])
	}
	secondBytes := got[3+6+GroundItemBytes:]
	if id := binary.BigEndian.Uint32(secondBytes[0:4]); id != 8 {
		t.Fatalf("second id=%d, want 8", id)
	}
	if item := binary.BigEndian.Uint16(secondBytes[4:6]); item != 1234 {
		t.Fatalf("second itemId=%d, want 1234", item)
	}
}

func TestEncodeGroundItemsDelta(t *testing.T) {
	got := EncodeGroundItemsDelta(9, []GroundItemView{groundItemVector}, []int{7, 8})
	// Opcode 55, payload 6 + 34 + 2 + 8 = 50, header serial 9 +
	// upsert count 1, record, remove section count 2 + ids 7, 8.
	want := []byte{55, 0x00, 0x32, 0x00, 0x00, 0x00, 0x09, 0x00, 0x01}
	want = append(want, groundItemVectorBytes...)
	want = append(want, 0x00, 0x02, 0x00, 0x00, 0x00, 0x07, 0x00, 0x00, 0x00, 0x08)
	if !bytes.Equal(got, want) {
		t.Fatalf("packet = %x\nwant %x", got, want)
	}
}

func TestEncodeGroundItemsDeltaEmpty(t *testing.T) {
	// The remove section is always present, even when empty.
	got := EncodeGroundItemsDelta(2, nil, nil)
	want := []byte{55, 0x00, 0x08, 0x00, 0x00, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(got, want) {
		t.Fatalf("packet = %x, want %x", got, want)
	}
}

func TestEncodeGroundItemsDeltaRemovesOnly(t *testing.T) {
	got := EncodeGroundItemsDelta(3, nil, []int{42})
	want := []byte{55, 0x00, 0x0c, 0x00, 0x00, 0x00, 0x03, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x2a}
	if !bytes.Equal(got, want) {
		t.Fatalf("packet = %x, want %x", got, want)
	}
}

func TestEncodeGroundItemMasking(t *testing.T) {
	// TS masks: id/quantity/serial `| 0` (int32), itemId/x/y
	// `& 0xffff`, level/ownership byte assignment (& 0xff),
	// enabledOps `& 0x1f`.
	v := GroundItemView{
		EnabledOps:       0x3f, // -> 0x1f
		ID:               -1,   // -> ff ff ff ff
		ItemID:           65541,
		Quantity:         -2,
		X:                -1,
		Y:                65537,
		Level:            0x123, // -> 0x23
		CreatedTick:      -1,
		PrivateUntilTick: 0,
		ExpiresTick:      0,
		OwnerID:          -1,
		Ownership:        0x101, // -> 0x01
	}
	rec := encodeGroundItem(v)
	if got := binary.BigEndian.Uint32(rec[0:4]); got != 0xffffffff {
		t.Fatalf("id=%x, want ffffffff", got)
	}
	if got := binary.BigEndian.Uint16(rec[4:6]); got != 5 {
		t.Fatalf("itemId=%d, want 5", got)
	}
	if got := binary.BigEndian.Uint32(rec[6:10]); got != 0xfffffffe {
		t.Fatalf("quantity=%x, want fffffffe", got)
	}
	if got := binary.BigEndian.Uint16(rec[10:12]); got != 0xffff {
		t.Fatalf("x=%x, want ffff", got)
	}
	if got := binary.BigEndian.Uint16(rec[12:14]); got != 1 {
		t.Fatalf("y=%d, want 1", got)
	}
	if rec[14] != 0x23 {
		t.Fatalf("level=%x, want 23", rec[14])
	}
	if got := binary.BigEndian.Uint32(rec[15:19]); got != 0xffffffff {
		t.Fatalf("created=%x, want ffffffff", got)
	}
	if rec[31] != 0 {
		t.Fatalf("isPrivate=%d, want 0", rec[31])
	}
	if rec[32] != 0x01 {
		t.Fatalf("ownership=%x, want 01", rec[32])
	}
	if rec[33] != 0x1f {
		t.Fatalf("enabledOps=%x, want 1f", rec[33])
	}
	// Serial truncates to int32 as well.
	pkt := EncodeGroundItems(-1, nil)
	if got := binary.BigEndian.Uint32(pkt[3:7]); got != 0xffffffff {
		t.Fatalf("serial=%x, want ffffffff", got)
	}
}
