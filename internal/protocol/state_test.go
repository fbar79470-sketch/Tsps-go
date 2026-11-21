package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeVarp(t *testing.T) {
	// Small form: opcode 40, id u16, value byte. 255 is the boundary.
	if got, want := EncodeVarp(5, 200), []byte{40, 0, 5, 200}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarp(5,200) = %x, want %x", got, want)
	}
	if got, want := EncodeVarp(5, 255), []byte{40, 0, 5, 255}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarp(5,255) = %x, want %x", got, want)
	}
	// Large form: opcode 41, id u16, value int32. 256 and negatives
	// take the large form.
	if got, want := EncodeVarp(5, 256), []byte{41, 0, 5, 0, 0, 1, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarp(5,256) = %x, want %x", got, want)
	}
	if got, want := EncodeVarp(5, -1), []byte{41, 0, 5, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarp(5,-1) = %x, want %x", got, want)
	}
	if got, want := EncodeVarp(5, 0), []byte{40, 0, 5, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarp(5,0) = %x, want %x", got, want)
	}
}

func TestEncodeVarpLong(t *testing.T) {
	want := []byte{44, 0, 7, 1, 2, 3, 4, 5, 6, 7, 8}
	if got := EncodeVarpLong(7, 0x0102030405060708); !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarpLong = %x, want %x", got, want)
	}
	if got, want := EncodeVarpLong(7, -1), []byte{44, 0, 7, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarpLong(-1) = %x, want %x", got, want)
	}
}

func TestEncodeVarbit(t *testing.T) {
	if got, want := EncodeVarbit(9, -5), []byte{42, 0, 9, 0xFF, 0xFF, 0xFF, 0xFB}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeVarbit(9,-5) = %x, want %x", got, want)
	}
}

func TestEncodeInventorySlot(t *testing.T) {
	// Small quantity: slot, item id + 1, one quantity byte.
	if got, want := EncodeInventorySlot(0, 4151, 1), []byte{51, 5, 0, 0, 0x10, 0x38, 1}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot = %x, want %x", got, want)
	}
	// 254 stays in the one-byte form.
	if got, want := EncodeInventorySlot(0, 4151, 254), []byte{51, 5, 0, 0, 0x10, 0x38, 254}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot(254) = %x, want %x", got, want)
	}
	// 255 switches to the OSRS large form: 255 marker + int32.
	if got, want := EncodeInventorySlot(0, 4151, 255), []byte{51, 9, 0, 0, 0x10, 0x38, 0xFF, 0, 0, 0, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot(255) = %x, want %x", got, want)
	}
	if got, want := EncodeInventorySlot(3, 100, 100000), []byte{51, 9, 0, 3, 0, 0x65, 0xFF, 0, 1, 0x86, 0xA0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot(100000) = %x, want %x", got, want)
	}
	// An empty slot: item -1 encodes as id 0, quantity 0.
	if got, want := EncodeInventorySlot(1, -1, 0), []byte{51, 5, 0, 1, 0, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot(empty) = %x, want %x", got, want)
	}
	// A negative quantity clamps to 0.
	if got, want := EncodeInventorySlot(1, 100, -3), []byte{51, 5, 0, 1, 0, 0x65, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySlot(-3) = %x, want %x", got, want)
	}
}

func TestEncodeInventorySnapshot(t *testing.T) {
	want := []byte{
		50, 0, 12,
		0, 2, // count
		0, 0, 0x10, 0x38, 1, // slot 0: 4151 x1
		0, 1, 0, 0, 0, // slot 1: empty
	}
	got := EncodeInventorySnapshot([]ItemSlot{{Slot: 0, ItemID: 4151, Quantity: 1}, {Slot: 1, ItemID: -1, Quantity: 0}})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySnapshot = %x, want %x", got, want)
	}
	// An empty snapshot is just the zero count.
	if got, want := EncodeInventorySnapshot(nil), []byte{50, 0, 2, 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventorySnapshot(nil) = %x, want %x", got, want)
	}
}

func TestEncodeInventoryStopTransmit(t *testing.T) {
	if got, want := EncodeInventoryStopTransmit(3214), []byte{56, 0x0C, 0x8E}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeInventoryStopTransmit = %x, want %x", got, want)
	}
	for _, bad := range []int{-1, 65536} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("EncodeInventoryStopTransmit(%d) did not panic", bad)
				}
			}()
			EncodeInventoryStopTransmit(bad)
		}()
	}
}

func TestEncodeBank(t *testing.T) {
	// Slot: item slot + flag byte (placeholder 1 | tab 2 << 1 = 5).
	want := []byte{53, 6, 0, 0, 0, 0x65, 5, 5}
	got := EncodeBankSlot(BankSlot{Slot: 0, ItemID: 100, Quantity: 5, Placeholder: true, Tab: 2})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeBankSlot = %x, want %x", got, want)
	}
	// Snapshot: capacity, count, then bank slots.
	wantSnap := []byte{
		52, 0, 10,
		0, 100, 0, 1,
		0, 0, 0, 0x65, 5, 5,
	}
	gotSnap := EncodeBankSnapshot(100, []BankSlot{{Slot: 0, ItemID: 100, Quantity: 5, Placeholder: true, Tab: 2}})
	if !bytes.Equal(gotSnap, wantSnap) {
		t.Fatalf("EncodeBankSnapshot = %x, want %x", gotSnap, wantSnap)
	}
	// A large quantity in the bank uses the same 255 + int32 form.
	if got, want := EncodeBankSlot(BankSlot{Slot: 0, ItemID: 100, Quantity: 300}),
		[]byte{53, 10, 0, 0, 0, 0x65, 0xFF, 0, 0, 1, 0x2C, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeBankSlot(large) = %x, want %x", got, want)
	}
}

func TestEncodeSkills(t *testing.T) {
	// One skill: id, xp int32, base, virtual, boost+128, current; then
	// total level u16 and combat level byte, plus the one zero slack
	// byte per skill the TS 10-byte allocation leaves.
	skills := []SkillView{{ID: 0, XP: 1000, BaseLevel: 10, VirtualLevel: 10, Boost: 2, CurrentLevel: 12}}
	wantSnap := []byte{70, 14, 1, 0, 0, 0, 3, 0xE8, 10, 10, 130, 12, 0, 100, 50, 0}
	if got := EncodeSkillsSnapshot(skills, 100, 50); !bytes.Equal(got, wantSnap) {
		t.Fatalf("EncodeSkillsSnapshot = %x, want %x", got, wantSnap)
	}
	wantDelta := []byte{71, 14, 1, 0, 0, 0, 3, 0xE8, 10, 10, 130, 12, 0, 100, 50, 0}
	if got := EncodeSkillsDelta(skills, 100, 50); !bytes.Equal(got, wantDelta) {
		t.Fatalf("EncodeSkillsDelta = %x, want %x", got, wantDelta)
	}
	// A negative boost wraps through +128 (boost -2 -> 126).
	neg := []SkillView{{ID: 1, XP: 0, BaseLevel: 10, VirtualLevel: 10, Boost: -2, CurrentLevel: 8}}
	wantNeg := []byte{70, 14, 1, 1, 0, 0, 0, 0, 10, 10, 126, 8, 0, 10, 3, 0}
	if got := EncodeSkillsSnapshot(neg, 10, 3); !bytes.Equal(got, wantNeg) {
		t.Fatalf("EncodeSkillsSnapshot(neg boost) = %x, want %x", got, wantNeg)
	}
}

func TestEncodeRunEnergy(t *testing.T) {
	// percent, running, units u16, weight signed, stamina u16, active u16.
	want := []byte{81, 100, 1, 0x27, 0x10, 0, 25, 0, 100, 0, 1}
	if got := EncodeRunEnergy(10000, true, 25, 100); !bytes.Equal(got, want) {
		t.Fatalf("EncodeRunEnergy = %x, want %x", got, want)
	}
	// No stamina: active is 0.
	wantIdle := []byte{81, 50, 0, 0x13, 0x88, 0, 0, 0, 0, 0, 0}
	if got := EncodeRunEnergy(5000, false, 0, 0); !bytes.Equal(got, wantIdle) {
		t.Fatalf("EncodeRunEnergy(idle) = %x, want %x", got, wantIdle)
	}
	// Clamps: units to 10000, weight to int16 range, stamina to u16.
	wantClamped := []byte{81, 100, 1, 0x27, 0x10, 0x80, 0x00, 0xFF, 0xFF, 0, 1}
	if got := EncodeRunEnergy(20000, true, -40000, 70000); !bytes.Equal(got, wantClamped) {
		t.Fatalf("EncodeRunEnergy(clamped) = %x, want %x", got, wantClamped)
	}
	if got := EncodeRunEnergy(-5, false, 0, -1); !bytes.Equal(got, []byte{81, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("EncodeRunEnergy(negative) = %x", got)
	}
}
