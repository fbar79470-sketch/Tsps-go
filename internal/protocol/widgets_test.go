package protocol

import (
	"bytes"
	"testing"
)

func boolPtr(v bool) *bool { return &v }

func TestEncodeWidgetOpenCloseRoot(t *testing.T) {
	if got, want := EncodeWidgetOpen(161, true), []byte{100, 0, 0xA1, 1}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetOpen(modal) = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetOpen(161, false), []byte{100, 0, 0xA1, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetOpen(non-modal) = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetClose(161), []byte{101, 0, 0xA1}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetClose = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetRoot(548), []byte{102, 2, 0x24}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetRoot = %x, want %x", got, want)
	}
}

func TestEncodeWidgetOpenSubMinimal(t *testing.T) {
	// The magic-tab mount sendTabInterface(6, ...) sends: target uid
	// (161<<16)|82, group 218, type 1, zero counts for varps, varbits,
	// hidden, pre- and post-scripts.
	want := []byte{
		103, 0, 12,
		0, 0xA1, 0, 0x52, 0, 0xDA, 1, 0, 0, 0, 0, 0,
	}
	got := EncodeWidgetOpenSub((161<<16)|82, 218, 1, WidgetOpenSubOptions{})
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetOpenSub(minimal) = %x, want %x", got, want)
	}
}

func TestEncodeWidgetOpenSubOptions(t *testing.T) {
	// Varps/varbits use u16 keys here; keys sort ascending (TS
	// Object.entries order), so 3 precedes 20. One hidden uid, one
	// post-script with one int argument.
	opts := WidgetOpenSubOptions{
		Varps:      map[int]int{20: 1, 3: 2},
		HiddenUIDs: []int{7},
		PostScripts: []WidgetScript{
			{ScriptID: 99, Args: []ScriptArg{ScriptArgInt(1)}},
		},
	}
	want := []byte{
		103, 0, 38,
		0, 0xA1, 0, 0x52, 0, 0xDA, 1, // header
		2, 0, 3, 0, 0, 0, 2, 0, 20, 0, 0, 0, 1, // varps (sorted)
		0,             // varbits
		1, 0, 0, 0, 7, // hidden
		0,                                  // pre-scripts
		1, 0, 0, 0, 0x63, 1, 1, 0, 0, 0, 1, // post-scripts
	}
	got := EncodeWidgetOpenSub((161<<16)|82, 218, 1, opts)
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetOpenSub(options) = %x, want %x", got, want)
	}
}

func TestEncodeWidgetCloseSub(t *testing.T) {
	if got, want := EncodeWidgetCloseSub((161<<16)|82), []byte{104, 0, 0xA1, 0, 0x52}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetCloseSub = %x, want %x", got, want)
	}
}

func TestEncodeWidgetSetText(t *testing.T) {
	uid := (161 << 16) | 5
	// No serverOwned: no trailing byte.
	if got, want := EncodeWidgetSetText(uid, "Hi", nil),
		[]byte{105, 0, 7, 0, 0xA1, 0, 5, 'H', 'i', 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetText = %x, want %x", got, want)
	}
	// serverOwned set: trailing 1.
	if got, want := EncodeWidgetSetText(uid, "Hi", boolPtr(true)),
		[]byte{105, 0, 8, 0, 0xA1, 0, 5, 'H', 'i', 0, 1}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetText(owned) = %x, want %x", got, want)
	}
	// serverOwned false: trailing 0.
	if got, want := EncodeWidgetSetText(uid, "Hi", boolPtr(false)),
		[]byte{105, 0, 8, 0, 0xA1, 0, 5, 'H', 'i', 0, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetText(unowned) = %x, want %x", got, want)
	}
}

func TestEncodeWidgetSetters(t *testing.T) {
	uid := (149 << 16) | 3
	if got, want := EncodeWidgetSetHidden(uid, true), []byte{106, 0, 0x95, 0, 3, 1}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetHidden = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetModel(uid, -1), []byte{117, 0, 0x95, 0, 3, 0xFF, 0xFF, 0xFF, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetModel = %x, want %x", got, want)
	}
	// Colour is masked to 15 bits: 0xffff -> 0x7fff.
	if got, want := EncodeWidgetSetColour(uid, 0xffff), []byte{119, 0, 0x95, 0, 3, 0x7F, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetColour = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetPosition(uid, -5, 300), []byte{118, 0, 0x95, 0, 3, 0xFF, 0xFB, 1, 0x2C}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetPosition = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetAngle(uid, 100, 200, 300), []byte{113, 0, 0x95, 0, 3, 0, 100, 0, 200, 1, 0x2C}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetAngle = %x, want %x", got, want)
	}
	// Rotate speed packs y in the high short, x in the low short.
	if got, want := EncodeWidgetSetRotateSpeed(uid, 2, -1), []byte{116, 0, 0x95, 0, 3, 0xFF, 0xFF, 0, 2}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetRotateSpeed = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetItem(uid, 4151, 3), []byte{107, 0, 0x95, 0, 3, 0x10, 0x37, 0, 0, 0, 3}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetItem = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetNpcHead(uid, 1234), []byte{108, 0, 0x95, 0, 3, 4, 0xD2}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetNpcHead = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetPlayerHead(uid), []byte{115, 0, 0x95, 0, 3}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetPlayerHead = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetFlags(uid, 0x1E), []byte{111, 0, 0x95, 0, 3, 0, 0, 0, 0x1E}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetFlags = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetFlagsRange(MainInventoryWidgetUID, 0, 27, MainInventorySlotFlags),
		[]byte{109, 0, 0x95, 0, 0, 0, 0, 0, 27, 0, 0x12, 0x07, 0xFE}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetFlagsRange = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetFlagsRangeV2(uid, -1, -1, 0x406, 0xFF),
		[]byte{112, 0, 0x95, 0, 3, 0xFF, 0xFF, 0xFF, 0xFF, 0, 0, 4, 6, 0, 0, 0, 0xFF}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetFlagsRangeV2 = %x, want %x", got, want)
	}
	if got, want := EncodeWidgetSetAnimation(uid, 808), []byte{114, 0, 0x95, 0, 3, 3, 0x28}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetSetAnimation = %x, want %x", got, want)
	}
}

func TestEncodeWidgetRunScript(t *testing.T) {
	// Script 828, the bootstrap membership script: one int argument,
	// empty var sections, no inventories section (nil map writes
	// nothing).
	want := []byte{
		110, 0, 14,
		0, 0, 3, 0x3C, // script id 828
		1, 1, 0, 0, 0, 1, // args: [1]
		0, 0, // varps
		0, 0, // varbits
	}
	if got := EncodeWidgetRunScript(828, []ScriptArg{ScriptArgInt(1)}, nil, nil, nil); !bytes.Equal(got, want) {
		t.Fatalf("EncodeWidgetRunScript(828) = %x, want %x", got, want)
	}
	// Mixed argument tags: string 0, number 1, bigint 2.
	got := EncodeWidgetRunScript(1,
		[]ScriptArg{ScriptArgString("hi"), ScriptArgInt(-2), ScriptArgLong(0x0102030405060708)},
		nil, nil, nil)
	wantMixed := []byte{
		110, 0, 27,
		0, 0, 0, 1,
		3,
		0, 'h', 'i', 0,
		1, 0xFF, 0xFF, 0xFF, 0xFE,
		2, 1, 2, 3, 4, 5, 6, 7, 8,
		0, 0, 0, 0,
	}
	if !bytes.Equal(got, wantMixed) {
		t.Fatalf("EncodeWidgetRunScript(mixed) = %x, want %x", got, wantMixed)
	}
	// Varps/varbits use int32 keys headed by a u16 count (unlike the
	// open-sub pairs). A non-nil empty inventories map writes a zero
	// count byte; nil writes nothing (covered by the 828 case above).
	gotVars := EncodeWidgetRunScript(5, nil,
		map[int]int{10: 7}, map[int]int{20: 3}, map[int]ScriptInventorySnapshot{})
	wantVars := []byte{
		110, 0, 26,
		0, 0, 0, 5,
		0,                             // no args
		0, 1, 0, 0, 0, 10, 0, 0, 0, 7, // varps
		0, 1, 0, 0, 0, 20, 0, 0, 0, 3, // varbits
		0, // inventories: empty but present
	}
	if !bytes.Equal(gotVars, wantVars) {
		t.Fatalf("EncodeWidgetRunScript(vars) = %x, want %x", gotVars, wantVars)
	}
	// Script id -1 with one inventory: the sendInventory shape.
	gotInv := EncodeWidgetRunScript(-1, nil, nil, nil,
		map[int]ScriptInventorySnapshot{94: {Capacity: 2, Slots: []ItemSlot{{Slot: 0, ItemID: 100, Quantity: 1}}}})
	wantInv := []byte{
		110, 0, 21,
		0xFF, 0xFF, 0xFF, 0xFF,
		0,    // no args
		0, 0, // varps
		0, 0, // varbits
		1,                 // one inventory
		0, 94, 0, 2, 0, 1, // id, capacity, slot count
		0, 0, 0, 0x65, 1, // slot
	}
	if !bytes.Equal(gotInv, wantInv) {
		t.Fatalf("EncodeWidgetRunScript(inventory) = %x, want %x", gotInv, wantInv)
	}
}

func TestEncodeRunClientScript(t *testing.T) {
	// The bootstrap camera script: id 626, no arguments.
	if got, want := EncodeRunClientScript(626, nil), []byte{170, 0, 3, 2, 0x72, 0}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeRunClientScript(626) = %x, want %x", got, want)
	}
	if got, want := EncodeRunClientScript(106, []ScriptArg{ScriptArgInt(3)}),
		[]byte{170, 0, 8, 0, 106, 1, 1, 0, 0, 0, 3}; !bytes.Equal(got, want) {
		t.Fatalf("EncodeRunClientScript(args) = %x, want %x", got, want)
	}
}
