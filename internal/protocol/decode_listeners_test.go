package protocol

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
)

// Encoder helpers for listener fixtures: each encodes a value the way a
// client does for the matching Reader method. shortAdd/shortAddLE live
// in decode_movement_test.go (same package).

func encShort(v int) []byte { return []byte{byte(v >> 8), byte(v)} }

func encShortLE(v int) []byte { return []byte{byte(v), byte(v >> 8)} }

func encSignedShort(v int) []byte { return encShort(v & 0xffff) }

func encByteSub(v int) []byte { return []byte{byte(128 - v)} }

func encByteNeg(v int) []byte { return []byte{byte(-v)} }

func encByteAdd(v int) []byte { return []byte{byte(v + 128)} }

func encInt(v int32) []byte {
	return []byte{byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)}
}

func encIntLE(v int32) []byte {
	return []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
}

// encIntME inverts Reader.IntME: bytes b0 b1 b2 b3 map to
// (b2<<24)|(b3<<16)|(b0<<8)|b1.
func encIntME(v int32) []byte {
	return []byte{byte(v >> 8), byte(v), byte(v >> 24), byte(v >> 16)}
}

// encIntIME inverts Reader.IntIME: bytes b0 b1 b2 b3 map to
// (b1<<24)|(b0<<16)|(b3<<8)|b2.
func encIntIME(v int32) []byte {
	return []byte{byte(v >> 16), byte(v >> 24), byte(v), byte(v >> 8)}
}

func encString(s string) []byte { return append([]byte(s), 0) }

func concat(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestDecodeNpcOption(t *testing.T) {
	// Index 300, modifierFlags as noted per branch. Note opcode 57
	// decodes as clickType 5 and opcode 76 as clickType 1.
	cases := []struct {
		opcode  byte
		want    NpcOption
		payload []byte
	}{
		{NativeOpNpc1Alt, NpcOption{300, 1, 1}, concat([]byte{1}, shortAddLE(300))},
		{NativeOpNpc2, NpcOption{300, 2, 0}, concat(shortAddLE(300), []byte{0})},
		{NativeOpNpc3, NpcOption{300, 3, 1}, concat(shortAdd(300), encByteNeg(1))},
		{NativeOpNpc4, NpcOption{300, 4, 2}, concat(encByteNeg(2), encShortLE(300))},
		{NativeOpNpc1, NpcOption{300, 5, 1}, concat(encByteAdd(1), encShortLE(300))},
	}
	for _, tc := range cases {
		got, err := DecodeNpcOption(tc.opcode, tc.payload)
		if err != nil {
			t.Errorf("opcode %d: %v", tc.opcode, err)
			continue
		}
		if got != tc.want {
			t.Errorf("opcode %d: got %+v, want %+v", tc.opcode, got, tc.want)
		}
	}
	// Opcode 50 (OPNPC5) has no decode branch: rejected, not decoded
	// as an NPC option.
	if _, err := DecodeNpcOption(NativeOpNpc5, []byte{0, 0, 0}); err == nil {
		t.Error("DecodeNpcOption(50): nil error, want rejection (no decode branch)")
	}
	if _, err := DecodeNpcOption(99, []byte{0, 0, 0}); err == nil {
		t.Error("DecodeNpcOption(99): nil error, want rejection")
	}
}

func TestDecodeNpcOptionMalformed(t *testing.T) {
	for _, payload := range [][]byte{nil, {1}, {1, 2}, {1, 2, 3, 4}} {
		if _, err := DecodeNpcOption(NativeOpNpc1Alt, payload); err == nil {
			t.Errorf("DecodeNpcOption(76, %v): nil error", payload)
		}
	}
	// All-0xFF bytes of the right length decode to garbage values
	// without panicking or erroring: no field is range-checked.
	if _, err := DecodeNpcOption(NativeOpNpc1Alt, []byte{0xFF, 0xFF, 0xFF}); err != nil {
		t.Errorf("DecodeNpcOption garbage: %v", err)
	}
}

func TestDecodeObjectOption(t *testing.T) {
	// id 1234 at (3089, 3524), modifierFlags 1.
	cases := []struct {
		opcode  byte
		payload []byte
	}{
		{NativeOpLoc1, concat(shortAdd(3089), encShortLE(3524), encByteNeg(1), shortAddLE(1234))},
		{NativeOpLoc2, concat(shortAddLE(3089), shortAddLE(3524), encByteSub(1), encShort(1234))},
		{NativeOpLoc3, concat(encShortLE(3524), encShortLE(1234), shortAddLE(3089), encByteSub(1))},
		{NativeOpLoc4, concat(shortAdd(3089), encShortLE(1234), shortAdd(3524), encByteNeg(1))},
		{NativeOpLoc5, concat(encShort(3089), encByteAdd(1), shortAdd(3524), shortAdd(1234))},
	}
	for i, tc := range cases {
		want := ObjectOption{ID: 1234, X: 3089, Y: 3524, ClickType: i + 1, HasClickType: true, ModifierFlags: 1}
		got, err := DecodeObjectOption(tc.opcode, tc.payload)
		if err != nil {
			t.Errorf("opcode %d: %v", tc.opcode, err)
			continue
		}
		if got != want {
			t.Errorf("opcode %d: got %+v, want %+v", tc.opcode, got, want)
		}
	}
	if _, err := DecodeObjectOption(99, make([]byte, 7)); err == nil {
		t.Error("DecodeObjectOption(99): nil error, want rejection")
	}
	for _, payload := range [][]byte{nil, make([]byte, 6), make([]byte, 8)} {
		if _, err := DecodeObjectOption(NativeOpLoc1, payload); err == nil {
			t.Errorf("DecodeObjectOption(96, len %d): nil error", len(payload))
		}
	}
}

func TestDecodeLocInteract(t *testing.T) {
	payload := concat(encShort(1234), encShort(3089), encShort(3524), []byte{0xFF}, encString("Bank"), []byte{2})
	got, err := DecodeLocInteract(payload)
	if err != nil {
		t.Fatalf("DecodeLocInteract: %v", err)
	}
	want := ObjectOption{ID: 1234, X: 3089, Y: 3524, ClickType: 2, HasClickType: true, Action: "Bank", HasAction: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Empty action + zero click type decode to absent.
	payload = concat(encShort(1), encShort(2), encShort(3), []byte{0}, encString(""), []byte{0})
	got, err = DecodeLocInteract(payload)
	if err != nil {
		t.Fatalf("DecodeLocInteract absent: %v", err)
	}
	if got.HasAction || got.HasClickType || got.Action != "" || got.ClickType != 0 {
		t.Errorf("absent: got %+v", got)
	}
	// Trailing bytes are ignored, as in TS.
	payload = concat(encShort(1), encShort(2), encShort(3), []byte{0}, encString("x"), []byte{1}, []byte{9, 9, 9})
	if _, err := DecodeLocInteract(payload); err != nil {
		t.Errorf("trailing bytes: %v", err)
	}
	for _, payload := range [][]byte{nil, encShort(1), concat(encShort(1), encShort(2), encShort(3), []byte{0}, []byte("unterminated"))} {
		if _, err := DecodeLocInteract(payload); err == nil {
			t.Errorf("DecodeLocInteract(%v): nil error", payload)
		}
	}
}

func TestDecodePlayerOption(t *testing.T) {
	// Index 7, modifierFlags 1.
	cases := []struct {
		opcode  byte
		option  int
		payload []byte
	}{
		{NativeOpPlayer1, 1, concat(encByteSub(1), encShort(7))},
		{NativeOpPlayer2, 2, concat([]byte{1}, encShort(7))},
		{NativeOpPlayer3, 3, concat(encByteSub(1), encShort(7))},
		{NativeOpPlayer4, 4, concat(encShort(7), encByteNeg(1))},
		{NativeOpPlayer5, 5, concat(shortAddLE(7), encByteSub(1))},
		{NativeOpPlayer6, 6, concat(encShort(7), encByteNeg(1))},
		{NativeOpPlayer7, 7, concat(shortAdd(7), encByteSub(1))},
		{NativeOpPlayer8, 8, concat([]byte{1}, shortAdd(7))},
	}
	for _, tc := range cases {
		want := PlayerOption{Index: 7, Option: tc.option, ModifierFlags: 1}
		got, err := DecodePlayerOption(tc.opcode, tc.payload)
		if err != nil {
			t.Errorf("opcode %d: %v", tc.opcode, err)
			continue
		}
		if got != want {
			t.Errorf("opcode %d: got %+v, want %+v", tc.opcode, got, want)
		}
	}
	if _, err := DecodePlayerOption(99, []byte{0, 0, 0}); err == nil {
		t.Error("DecodePlayerOption(99): nil error, want rejection")
	}
	for _, payload := range [][]byte{nil, {0, 0}, {0, 0, 0, 0}} {
		if _, err := DecodePlayerOption(NativeOpPlayer1, payload); err == nil {
			t.Errorf("DecodePlayerOption(44, %v): nil error", payload)
		}
	}
}

func TestDecodeInteract(t *testing.T) {
	got, err := DecodeInteract(concat([]byte{0}, encShort(9)))
	if err != nil || got != (PlayerOption{Index: 9, Option: 3}) {
		t.Errorf("byte 0: got %+v, err %v, want option 3", got, err)
	}
	got, err = DecodeInteract(concat([]byte{1}, encShort(9)))
	if err != nil || got != (PlayerOption{Index: 9, Option: 2}) {
		t.Errorf("byte 1: got %+v, err %v, want option 2", got, err)
	}
	got, err = DecodeInteract(concat([]byte{0xFF}, encShort(9)))
	if err != nil || got.Option != 2 {
		t.Errorf("byte 0xFF: got %+v, err %v, want option 2", got, err)
	}
	// Trailing bytes ignored (variable-length opcode).
	if _, err := DecodeInteract(concat([]byte{0}, encShort(9), []byte{7})); err != nil {
		t.Errorf("trailing: %v", err)
	}
	if _, err := DecodeInteract([]byte{0}); err == nil {
		t.Error("DecodeInteract short: nil error")
	}
}

func TestDecodeGroundItemAction(t *testing.T) {
	// Item 995 at (3089, 3524), modifierFlags 1.
	cases := []struct {
		opcode  byte
		option  int
		payload []byte
	}{
		{NativeOpObj1, 1, concat(encByteSub(1), encShortLE(3524), shortAdd(995), shortAdd(3089))},
		{NativeOpObj2, 2, concat(shortAdd(3089), encShortLE(3524), encShort(995), []byte{1})},
		{NativeOpObj3, 3, concat(encShortLE(995), shortAdd(3089), shortAddLE(3524), encByteSub(1))},
		{NativeOpObj4, 4, concat(shortAddLE(995), shortAddLE(3524), encByteNeg(1), shortAdd(3089))},
		{NativeOpObj5, 5, concat(shortAdd(995), encShortLE(3089), encShortLE(3524), encByteSub(1))},
	}
	for _, tc := range cases {
		want := GroundItemAction{ItemID: 995, X: 3089, Y: 3524, OptionIndex: tc.option, HasOptionIndex: true, ModifierFlags: 1}
		got, err := DecodeGroundItemAction(tc.opcode, tc.payload)
		if err != nil {
			t.Errorf("opcode %d: %v", tc.opcode, err)
			continue
		}
		if got != want {
			t.Errorf("opcode %d: got %+v, want %+v", tc.opcode, got, want)
		}
	}
	// EXAMINE_OBJ: option "Examine", no option index, no Ctrl byte.
	got, err := DecodeGroundItemAction(NativeExamineObj, concat(encShort(995), encShortLE(3524), encShortLE(3089)))
	if err != nil {
		t.Fatalf("EXAMINE_OBJ: %v", err)
	}
	want := GroundItemAction{ItemID: 995, X: 3089, Y: 3524, Option: "Examine", HasOption: true}
	if got != want {
		t.Errorf("EXAMINE_OBJ: got %+v, want %+v", got, want)
	}
	if _, err := DecodeGroundItemAction(99, make([]byte, 7)); err == nil {
		t.Error("DecodeGroundItemAction(99): nil error, want rejection")
	}
	for _, payload := range [][]byte{nil, make([]byte, 6), make([]byte, 8)} {
		if _, err := DecodeGroundItemAction(NativeOpObj1, payload); err == nil {
			t.Errorf("DecodeGroundItemAction(102, len %d): nil error", len(payload))
		}
	}
	if _, err := DecodeGroundItemAction(NativeExamineObj, make([]byte, 7)); err == nil {
		t.Error("DecodeGroundItemAction(104, 7 bytes): nil error, want trailing rejection")
	}
}

func TestDecodeGroundItemActionHigh(t *testing.T) {
	payload := concat(encInt(424242), encShort(3089), encShort(3524), []byte{0xFF}, encShort(995), encInt(-7), encString("Take"), []byte{1})
	got, err := DecodeGroundItemActionHigh(payload)
	if err != nil {
		t.Fatalf("DecodeGroundItemActionHigh: %v", err)
	}
	want := GroundItemAction{ItemID: 995, X: 3089, Y: 3524, OptionIndex: 1, HasOptionIndex: true, Option: "Take", HasOption: true, StackID: 424242, HasStackID: true}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Empty option + zero index decode to absent; negative stack id kept.
	payload = concat(encInt(-1), encShort(1), encShort(2), []byte{0}, encShort(3), encInt(0), encString(""), []byte{0})
	got, err = DecodeGroundItemActionHigh(payload)
	if err != nil {
		t.Fatalf("absent: %v", err)
	}
	if got.HasOption || got.HasOptionIndex || got.StackID != -1 || !got.HasStackID {
		t.Errorf("absent: got %+v", got)
	}
	if _, err := DecodeGroundItemActionHigh(encInt(1)); err == nil {
		t.Error("short: nil error")
	}
	if _, err := DecodeGroundItemActionHigh(concat(encInt(1), encShort(1), encShort(2), []byte{0}, encShort(3), encInt(0), []byte("no-nul"))); err == nil {
		t.Error("unterminated option: nil error")
	}
}

func TestDecodeItemOnX(t *testing.T) {
	const widgetID = int32(0x12345678)
	// ItemOnPlayer (65).
	got, err := DecodeItemOnPlayer(concat(shortAddLE(5), shortAddLE(995), shortAdd(3), encInt(widgetID), encByteAdd(1)))
	if err != nil {
		t.Fatalf("DecodeItemOnPlayer: %v", err)
	}
	if got != (ItemOnPlayer{TargetIndex: 5, ItemID: 995, Slot: 3, WidgetID: widgetID, ModifierFlags: 1}) {
		t.Errorf("DecodeItemOnPlayer: %+v", got)
	}
	// ItemOnNpc (36).
	gotNpc, err := DecodeItemOnNpc(concat(shortAddLE(3), []byte{1}, encIntME(widgetID), encShortLE(995), shortAddLE(300)))
	if err != nil {
		t.Fatalf("DecodeItemOnNpc: %v", err)
	}
	if gotNpc != (ItemOnNpc{TargetIndex: 300, ItemID: 995, Slot: 3, WidgetID: widgetID, ModifierFlags: 1}) {
		t.Errorf("DecodeItemOnNpc: %+v", gotNpc)
	}
	// ItemOnObject (86).
	gotObj, err := DecodeItemOnObject(concat(shortAddLE(3), shortAdd(1234), encIntLE(widgetID), encByteSub(1), encShortLE(3089), encShort(3524), shortAddLE(995)))
	if err != nil {
		t.Fatalf("DecodeItemOnObject: %v", err)
	}
	if gotObj != (ItemOnObject{ObjectID: 1234, X: 3089, Y: 3524, ItemID: 995, Slot: 3, WidgetID: widgetID, ModifierFlags: 1}) {
		t.Errorf("DecodeItemOnObject: %+v", gotObj)
	}
	// ItemOnGround (79): item id read last.
	gotGround, err := DecodeItemOnGround(concat(shortAddLE(995), shortAdd(3524), encIntME(widgetID), shortAdd(3089), shortAddLE(3), encByteSub(1), encShortLE(4151)))
	if err != nil {
		t.Fatalf("DecodeItemOnGround: %v", err)
	}
	if gotGround != (ItemOnGround{GroundItemID: 995, X: 3089, Y: 3524, ItemID: 4151, Slot: 3, WidgetID: widgetID, ModifierFlags: 1}) {
		t.Errorf("DecodeItemOnGround: %+v", gotGround)
	}
	// Malformed: short and trailing for each fixed length.
	if _, err := DecodeItemOnPlayer(make([]byte, 10)); err == nil {
		t.Error("DecodeItemOnPlayer short: nil error")
	}
	if _, err := DecodeItemOnPlayer(make([]byte, 12)); err == nil {
		t.Error("DecodeItemOnPlayer trailing: nil error")
	}
	if _, err := DecodeItemOnNpc(make([]byte, 10)); err == nil {
		t.Error("DecodeItemOnNpc short: nil error")
	}
	if _, err := DecodeItemOnObject(make([]byte, 14)); err == nil {
		t.Error("DecodeItemOnObject short: nil error")
	}
	if _, err := DecodeItemOnObject(make([]byte, 16)); err == nil {
		t.Error("DecodeItemOnObject trailing: nil error")
	}
	if _, err := DecodeItemOnGround(make([]byte, 14)); err == nil {
		t.Error("DecodeItemOnGround short: nil error")
	}
}

func TestDecodeSpellOnX(t *testing.T) {
	const widgetID = int32(-1234567) // Negative widget ids must survive the endian transforms.
	// SpellOnPlayer (32).
	got, err := DecodeSpellOnPlayer(concat(encByteNeg(1), encShortLE(1162), encShortLE(9), encIntIME(widgetID), encShortLE(5)))
	if err != nil {
		t.Fatalf("DecodeSpellOnPlayer: %v", err)
	}
	if got != (SpellOnPlayer{TargetIndex: 5, SpellWidget: widgetID, SpellChild: 9, SpellItemID: 1162, ModifierFlags: 1}) {
		t.Errorf("DecodeSpellOnPlayer: %+v", got)
	}
	// SpellOnNpc (75).
	gotNpc, err := DecodeSpellOnNpc(concat(encShort(300), encIntLE(widgetID), encShort(9), shortAdd(1162), encByteAdd(1)))
	if err != nil {
		t.Fatalf("DecodeSpellOnNpc: %v", err)
	}
	if gotNpc != (SpellOnNpc{TargetIndex: 300, SpellWidget: widgetID, SpellChild: 9, SpellItemID: 1162, ModifierFlags: 1}) {
		t.Errorf("DecodeSpellOnNpc: %+v", gotNpc)
	}
	// SpellOnObject (2).
	gotObj, err := DecodeSpellOnObject(concat(shortAddLE(3524), shortAdd(1234), shortAdd(9), encIntLE(widgetID), encShort(3089), encShortLE(1162), encByteNeg(1)))
	if err != nil {
		t.Fatalf("DecodeSpellOnObject: %v", err)
	}
	if gotObj != (SpellOnObject{ObjectID: 1234, X: 3089, Y: 3524, SpellWidget: widgetID, SpellChild: 9, SpellItemID: 1162, ModifierFlags: 1}) {
		t.Errorf("DecodeSpellOnObject: %+v", gotObj)
	}
	// SpellOnGround (94): spell item id read last; ground (not loc) target.
	gotGround, err := DecodeSpellOnGround(concat(encIntLE(widgetID), shortAdd(9), shortAdd(995), shortAddLE(3089), encShort(3524), []byte{1}, shortAddLE(1168)))
	if err != nil {
		t.Fatalf("DecodeSpellOnGround: %v", err)
	}
	if gotGround != (SpellOnGround{GroundItemID: 995, X: 3089, Y: 3524, SpellWidget: widgetID, SpellChild: 9, SpellItemID: 1168, ModifierFlags: 1}) {
		t.Errorf("DecodeSpellOnGround: %+v", gotGround)
	}
	if _, err := DecodeSpellOnPlayer(make([]byte, 10)); err == nil {
		t.Error("DecodeSpellOnPlayer short: nil error")
	}
	if _, err := DecodeSpellOnNpc(make([]byte, 12)); err == nil {
		t.Error("DecodeSpellOnNpc trailing: nil error")
	}
	if _, err := DecodeSpellOnObject(make([]byte, 14)); err == nil {
		t.Error("DecodeSpellOnObject short: nil error")
	}
	if _, err := DecodeSpellOnGround(make([]byte, 16)); err == nil {
		t.Error("DecodeSpellOnGround trailing: nil error")
	}
}

func TestDecodeAppearance(t *testing.T) {
	payload := []byte{1}
	for _, kit := range []int{0, 1, -1, 2, 3, 4, 5} {
		payload = append(payload, encSignedShort(kit)...)
	}
	payload = append(payload, 1, 2, 3, 4, 5)
	got, err := DecodeAppearance(payload)
	if err != nil {
		t.Fatalf("DecodeAppearance: %v", err)
	}
	want := Appearance{Gender: 1, Kits: [7]int{0, 1, -1, 2, 3, 4, 5}, Colors: [5]int{1, 2, 3, 4, 5}}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if len(payload) != 20 {
		t.Errorf("fixture length %d, want 20 (table length, not the enum comment's 13)", len(payload))
	}
	for _, p := range [][]byte{nil, make([]byte, 19), make([]byte, 21)} {
		if _, err := DecodeAppearance(p); err == nil {
			t.Errorf("DecodeAppearance(len %d): nil error", len(p))
		}
	}
}

func TestDecodeChat(t *testing.T) {
	t.Run("public no style", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{0}, encString("hello")))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if got.Kind != 0 || got.Clan || got.MessageType != ChatMessagePublic || got.Text != "hello" || got.AutoChat || got.Style != nil {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("autochat kind 3", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{3}, encString("buying gf")))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if got.MessageType != ChatMessagePublic || !got.AutoChat || got.Text != "buying gf" {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("game and friends kinds", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{1}, encString("g")))
		if err != nil || got.MessageType != ChatMessageGame {
			t.Errorf("kind 1: %+v, %v", got, err)
		}
		got, err = DecodeChat(concat([]byte{2}, encString("f")))
		if err != nil || got.MessageType != ChatMessageFriendsChat {
			t.Errorf("kind 2: %+v, %v", got, err)
		}
	})
	t.Run("unknown kind falls back to public", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{9}, encString("x")))
		if err != nil || got.MessageType != ChatMessagePublic || got.AutoChat {
			t.Errorf("kind 9: %+v, %v", got, err)
		}
	})
	t.Run("clan kind 4 signed type", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{4}, []byte{0xFF}, encString("clan msg")))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if !got.Clan || got.ClanType != -1 || got.Text != "clan msg" || got.MessageType != "" {
			t.Errorf("got %+v", got)
		}
		got, err = DecodeChat(concat([]byte{4}, []byte{2}, encString("c")))
		if err != nil || !got.Clan || got.ClanType != 2 {
			t.Errorf("clan type 2: %+v, %v", got, err)
		}
	})
	t.Run("style colour and effect", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{0}, encString("hi"), []byte{2, 1}))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if got.Style == nil || got.Style.Colour != 2 || got.Style.Effect != 1 || got.Style.Pattern != nil {
			t.Errorf("style: %+v", got.Style)
		}
	})
	t.Run("pattern style colour 13", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{0}, encString("hi"), []byte{13, 0, 7}))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if got.Style == nil || got.Style.Colour != 13 || !bytes.Equal(got.Style.Pattern, []byte{7}) {
			t.Errorf("pattern style: %+v", got.Style)
		}
	})
	t.Run("pattern style colour 20 needs 8 bytes", func(t *testing.T) {
		pattern := []byte{1, 2, 3, 4, 5, 6, 7, 8}
		got, err := DecodeChat(concat([]byte{0}, encString("hi"), []byte{20, 5}, pattern))
		if err != nil {
			t.Fatalf("DecodeChat: %v", err)
		}
		if got.Style == nil || !bytes.Equal(got.Style.Pattern, pattern) {
			t.Errorf("pattern style: %+v", got.Style)
		}
		// Too few pattern bytes: style dropped, not an error.
		got, err = DecodeChat(concat([]byte{0}, encString("hi"), []byte{20, 5, 1, 2}))
		if err != nil || got.Style != nil {
			t.Errorf("short pattern: %+v, %v", got.Style, err)
		}
	})
	t.Run("zero style and invalid style mean no style", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{0}, encString("hi"), []byte{0, 0}))
		if err != nil || got.Style != nil {
			t.Errorf("zero style: %+v, %v", got.Style, err)
		}
		got, err = DecodeChat(concat([]byte{0}, encString("hi"), []byte{21, 0}))
		if err != nil || got.Style != nil {
			t.Errorf("colour 21: %+v, %v", got.Style, err)
		}
		got, err = DecodeChat(concat([]byte{0}, encString("hi"), []byte{2, 6}))
		if err != nil || got.Style != nil {
			t.Errorf("effect 6: %+v, %v", got.Style, err)
		}
		// One trailing byte is not a style.
		got, err = DecodeChat(concat([]byte{0}, encString("hi"), []byte{2}))
		if err != nil || got.Style != nil {
			t.Errorf("one trailing byte: %+v, %v", got.Style, err)
		}
	})
	t.Run("trailing bytes after a valid style are ignored", func(t *testing.T) {
		got, err := DecodeChat(concat([]byte{0}, encString("hi"), []byte{2, 1, 9, 9}))
		if err != nil || got.Style == nil || got.Style.Colour != 2 {
			t.Errorf("trailing: %+v, %v", got.Style, err)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		for _, payload := range [][]byte{nil, {0}, {0, 'x'}, {4}, {4, 1}} {
			if _, err := DecodeChat(payload); err == nil {
				t.Errorf("DecodeChat(%v): nil error", payload)
			} else if len(payload) > 1 && !errors.Is(err, ErrUnterminatedString) && !errors.Is(err, ErrUnexpectedEnd) {
				t.Errorf("DecodeChat(%v): err %v", payload, err)
			}
		}
	})
}

func TestDecodeInventoryUse(t *testing.T) {
	got, err := DecodeInventoryUse(concat(encShort(5), encShort(995), encInt(-12345), encString("Wield")))
	if err != nil {
		t.Fatalf("DecodeInventoryUse: %v", err)
	}
	if got != (InventoryAction{Slot: 5, ItemID: 995, WidgetID: 3214, Option: "Wield"}) {
		t.Errorf("got %+v", got)
	}
	// Empty option is kept verbatim (no || undefined in the branch).
	got, err = DecodeInventoryUse(concat(encShort(0), encShort(1), encInt(0), encString("")))
	if err != nil || got.Option != "" {
		t.Errorf("empty option: %+v, %v", got, err)
	}
	if _, err := DecodeInventoryUse(concat(encShort(0), encShort(1), encInt(0), []byte("no-nul"))); err == nil {
		t.Error("unterminated option: nil error")
	}
	if _, err := DecodeInventoryUse(encShort(0)); err == nil {
		t.Error("short: nil error")
	}
}

func TestDecodeInventoryUseOn(t *testing.T) {
	t.Run("npc with tile", func(t *testing.T) {
		got, err := DecodeInventoryUseOn(concat(encShort(3), encShort(995), []byte{0}, encShort(300), []byte{1}, encShort(3089), encShort(3524), []byte{0}))
		if err != nil {
			t.Fatalf("DecodeInventoryUseOn: %v", err)
		}
		want := InventoryUseOn{Slot: 3, ItemID: 995, Target: UseOnTarget{Kind: UseOnTargetNpc, ID: 300, HasTile: true, X: 3089, Y: 3524, Level: 0}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("loc without tile", func(t *testing.T) {
		got, err := DecodeInventoryUseOn(concat(encShort(1), encShort(2), []byte{1}, encShort(1234), []byte{0}, []byte{2}))
		if err != nil {
			t.Fatalf("DecodeInventoryUseOn: %v", err)
		}
		if got.Target.Kind != UseOnTargetLoc || got.Target.HasTile || got.Target.Level != 2 {
			t.Errorf("got %+v", got.Target)
		}
	})
	t.Run("ground and player kinds", func(t *testing.T) {
		got, err := DecodeInventoryUseOn(concat(encShort(1), encShort(2), []byte{2}, encShort(995), []byte{0}, []byte{0}))
		if err != nil || got.Target.Kind != UseOnTargetGround {
			t.Errorf("ground: %+v, %v", got.Target, err)
		}
		got, err = DecodeInventoryUseOn(concat(encShort(1), encShort(2), []byte{3}, encShort(7), []byte{0}, []byte{0}))
		if err != nil || got.Target.Kind != UseOnTargetPlayer {
			t.Errorf("player: %+v, %v", got.Target, err)
		}
	})
	t.Run("unknown kind falls back to npc", func(t *testing.T) {
		got, err := DecodeInventoryUseOn(concat(encShort(1), encShort(2), []byte{7}, encShort(9), []byte{0}, []byte{0}))
		if err != nil || got.Target.Kind != UseOnTargetNpc {
			t.Errorf("kind 7: %+v, %v", got.Target, err)
		}
	})
	t.Run("inventory kind 4", func(t *testing.T) {
		// Level byte is read before the kind check; the id short is
		// read but unused for inventory targets.
		got, err := DecodeInventoryUseOn(concat(encShort(3), encShort(995), []byte{4}, encShort(0), []byte{0}, []byte{0}, encShort(5), encShort(4151)))
		if err != nil {
			t.Fatalf("DecodeInventoryUseOn: %v", err)
		}
		want := InventoryUseOn{Slot: 3, ItemID: 995, Target: UseOnTarget{Kind: UseOnTargetInventory, ID: 0, Level: 0, Slot: 5, ItemID: 4151}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("inventory kind 4 with tile bytes present", func(t *testing.T) {
		// The tile bytes and level byte are read before the kind
		// check, even for inventory targets.
		got, err := DecodeInventoryUseOn(concat(encShort(3), encShort(995), []byte{4}, encShort(0), []byte{1}, encShort(3089), encShort(3524), []byte{1}, encShort(5), encShort(4151)))
		if err != nil {
			t.Fatalf("DecodeInventoryUseOn: %v", err)
		}
		if got.Target.Kind != UseOnTargetInventory || got.Target.Slot != 5 || got.Target.ItemID != 4151 || got.Target.Level != 1 {
			t.Errorf("got %+v", got.Target)
		}
	})
	t.Run("malformed", func(t *testing.T) {
		// Missing level byte; missing tile bytes; missing inventory target.
		bad := [][]byte{
			concat(encShort(1), encShort(2), []byte{0}, encShort(3), []byte{0}),
			concat(encShort(1), encShort(2), []byte{0}, encShort(3), []byte{1}, encShort(4)),
			concat(encShort(1), encShort(2), []byte{4}, encShort(0), []byte{0}, []byte{0}, encShort(5)),
			nil,
		}
		for _, payload := range bad {
			if _, err := DecodeInventoryUseOn(payload); err == nil {
				t.Errorf("DecodeInventoryUseOn(%v): nil error", payload)
			}
		}
	})
}

func TestDecodeInventoryMove(t *testing.T) {
	got, err := DecodeInventoryMove(concat(encShort(2), encShort(5)))
	if err != nil {
		t.Fatalf("DecodeInventoryMove: %v", err)
	}
	if got != (InventoryMove{From: 2, To: 5, WidgetID: 3214}) {
		t.Errorf("got %+v", got)
	}
	for _, payload := range [][]byte{nil, {0, 0, 0}, {0, 0, 0, 0, 0}} {
		if _, err := DecodeInventoryMove(payload); err == nil {
			t.Errorf("DecodeInventoryMove(%v): nil error", payload)
		}
	}
}

func TestDecodeWidgetAction(t *testing.T) {
	t.Run("IF_BUTTON simple", func(t *testing.T) {
		// Widget id (198<<16)|5: group 198, child 5.
		got, err := DecodeWidgetAction(NativeIfButton, encInt((198<<16)|5))
		if err != nil {
			t.Fatalf("DecodeWidgetAction: %v", err)
		}
		want := WidgetAction{WidgetID: (198 << 16) | 5, GroupID: 198, ChildID: 5, ButtonNum: 1, HasButtonNum: true, Slot: -1, ItemID: -1, Simple: true}
		if got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	})
	t.Run("unsigned group extraction", func(t *testing.T) {
		// Widget id -1 (0xFFFFFFFF): group 65535, child 65535.
		got, err := DecodeWidgetAction(NativeIfButton, encInt(-1))
		if err != nil {
			t.Fatalf("DecodeWidgetAction: %v", err)
		}
		if got.GroupID != 65535 || got.ChildID != 65535 {
			t.Errorf("got group %d child %d", got.GroupID, got.ChildID)
		}
	})
	t.Run("IF_BUTTON_SUB raw slot and subOp", func(t *testing.T) {
		// Slot 0xffff is passed through raw; item 0xffff becomes -1;
		// subOpId is byte+1.
		got, err := DecodeWidgetAction(NativeIfButtonSub, concat(encInt(0x00C60005), encShort(0xffff), encShort(0xffff), []byte{3, 2}))
		if err != nil {
			t.Fatalf("DecodeWidgetAction: %v", err)
		}
		if got.Slot != 0xffff || got.ItemID != -1 || got.ButtonNum != 3 || got.SubOpID != 3 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("IF_BUTTONX op index", func(t *testing.T) {
		got, err := DecodeWidgetAction(NativeIfButtonX, concat(encInt(0x00C60005), encShort(7), encShort(995), []byte{11}))
		if err != nil {
			t.Fatalf("DecodeWidgetAction: %v", err)
		}
		if got.ButtonNum != 11 || got.Slot != 7 || got.ItemID != 995 {
			t.Errorf("got %+v", got)
		}
		// 0xffff slot/item decode to absent.
		got, err = DecodeWidgetAction(NativeIfButtonX, concat(encInt(0), encShort(0xffff), encShort(0xffff), []byte{12}))
		if err != nil || got.Slot != -1 || got.ItemID != -1 {
			t.Errorf("absent: %+v, %v", got, err)
		}
	})
	t.Run("IF_BUTTON1..10 mapping", func(t *testing.T) {
		opcodes := []byte{NativeIfButton1, NativeIfButton2, NativeIfButton3, NativeIfButton4, NativeIfButton5, NativeIfButton6, NativeIfButton7, NativeIfButton8, NativeIfButton9, NativeIfButton10}
		for i, opcode := range opcodes {
			got, err := DecodeWidgetAction(opcode, concat(encInt(0x00C60005), encShort(3), encShort(995)))
			if err != nil {
				t.Errorf("opcode %d: %v", opcode, err)
				continue
			}
			if got.ButtonNum != i+1 || got.Slot != 3 || got.ItemID != 995 {
				t.Errorf("opcode %d: got %+v, want buttonNum %d", opcode, got, i+1)
			}
		}
	})
	t.Run("malformed", func(t *testing.T) {
		if _, err := DecodeWidgetAction(99, make([]byte, 8)); err == nil {
			t.Error("opcode 99: nil error")
		}
		if _, err := DecodeWidgetAction(NativeIfButton, make([]byte, 5)); err == nil {
			t.Error("IF_BUTTON trailing: nil error")
		}
		if _, err := DecodeWidgetAction(NativeIfButton1, make([]byte, 7)); err == nil {
			t.Error("IF_BUTTON1 short: nil error")
		}
		if _, err := DecodeWidgetAction(NativeIfButtonSub, make([]byte, 9)); err == nil {
			t.Error("IF_BUTTON_SUB short: nil error")
		}
		if _, err := DecodeWidgetAction(NativeIfButtonX, make([]byte, 10)); err == nil {
			t.Error("IF_BUTTONX trailing: nil error")
		}
	})
}

func TestDecodeWidgetActionHigh(t *testing.T) {
	// widgetId, group 3214? No: group/child are separate shorts (149, 0
	// here); option "Wield", target "", opId 0, buttonNum 2, skipped
	// shorts/byte, slot -1, item 0xffff.
	payload := concat(encInt(0x00950000), encShort(149), encShort(0), encString("Wield"), encString(""), []byte{0, 2}, encShort(0), encShort(0), []byte{0}, encSignedShort(-1), encShort(0xffff))
	got, err := DecodeWidgetActionHigh(payload)
	if err != nil {
		t.Fatalf("DecodeWidgetActionHigh: %v", err)
	}
	want := WidgetAction{WidgetID: 0x00950000, GroupID: 149, ChildID: 0, ButtonNum: 2, HasButtonNum: true, Option: "Wield", HasOption: true, Slot: -1, ItemID: -1}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Present opId, target, slot, item id.
	payload = concat(encInt(1), encShort(2), encShort(3), encString(""), encString("target"), []byte{4, 0}, encShort(9), encShort(8), []byte{7}, encSignedShort(3), encShort(995))
	got, err = DecodeWidgetActionHigh(payload)
	if err != nil {
		t.Fatalf("DecodeWidgetActionHigh: %v", err)
	}
	if got.OpID != 4 || !got.HasOpID || got.HasButtonNum || got.Target != "target" || !got.HasTarget || got.Slot != 3 || got.ItemID != 995 || got.HasOption {
		t.Errorf("got %+v", got)
	}
	if _, err := DecodeWidgetActionHigh(encInt(1)); err == nil {
		t.Error("short: nil error")
	}
}

func TestDecodeWidgetTarget(t *testing.T) {
	got, err := DecodeWidgetTarget(concat(encIntIME(0x00950000), shortAddLE(3), encIntLE(0x00A10002), encShortLE(5), encShort(0xffff), shortAddLE(995)))
	if err != nil {
		t.Fatalf("DecodeWidgetTarget: %v", err)
	}
	want := WidgetTarget{TargetWidgetID: 0x00950000, TargetSlot: 3, TargetItemID: 995, SourceWidgetID: 0x00A10002, SourceSlot: 5, SourceItemID: -1}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	// Target item id is raw (0xffff stays 65535); source item id maps.
	got, err = DecodeWidgetTarget(concat(encIntIME(0), shortAddLE(0), encIntLE(0), encShortLE(0), encShort(1162), shortAddLE(0xffff)))
	if err != nil || got.TargetItemID != 0xffff || got.SourceItemID != 1162 {
		t.Errorf("raw target item: %+v, %v", got, err)
	}
	for _, payload := range [][]byte{nil, make([]byte, 15), make([]byte, 17)} {
		if _, err := DecodeWidgetTarget(payload); err == nil {
			t.Errorf("DecodeWidgetTarget(len %d): nil error", len(payload))
		}
	}
}

func TestDecodeWidgetDrag(t *testing.T) {
	got, err := DecodeWidgetDrag(concat(encShortLE(995), encIntLE(0x00950000), encShort(4151), shortAddLE(3), encIntME(0x00A10002), encShortLE(5)))
	if err != nil {
		t.Fatalf("DecodeWidgetDrag: %v", err)
	}
	want := WidgetDrag{TargetItemID: 995, TargetWidgetID: 0x00950000, SourceItemID: 4151, SourceSlot: 3, SourceWidgetID: 0x00A10002, TargetSlot: 5}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
	for _, payload := range [][]byte{nil, make([]byte, 15), make([]byte, 17)} {
		if _, err := DecodeWidgetDrag(payload); err == nil {
			t.Errorf("DecodeWidgetDrag(len %d): nil error", len(payload))
		}
	}
}

func TestDecodeLocalTrigger(t *testing.T) {
	// blockLength 12: no args. opcodeParam 3, child -1 (0xffff LE),
	// widget id, item 0xffff -> -1.
	payload := concat(encShort(12), encIntLE(3), encShortLE(0xffff), encIntLE(0x00950000), encShortLE(0xffff))
	got, err := DecodeLocalTrigger(payload)
	if err != nil {
		t.Fatalf("DecodeLocalTrigger: %v", err)
	}
	if got.OpcodeParam != 3 || got.ChildIndex != -1 || got.WidgetID != 0x00950000 || got.ItemID != -1 || len(got.ArgsData) != 0 {
		t.Errorf("got %+v", got)
	}
	// blockLength 15: 3 args bytes.
	payload = concat(encShort(15), encIntLE(1), encShortLE(4), encIntLE(2), encShortLE(995), []byte{9, 8, 7})
	got, err = DecodeLocalTrigger(payload)
	if err != nil {
		t.Fatalf("DecodeLocalTrigger args: %v", err)
	}
	if !bytes.Equal(got.ArgsData, []byte{9, 8, 7}) || got.ChildIndex != 4 || got.ItemID != 995 {
		t.Errorf("got %+v", got)
	}
	// Bad block lengths: below 12, and disagreeing with remaining.
	bad := [][]byte{
		concat(encShort(11), make([]byte, 11)),
		concat(encShort(12), make([]byte, 13)),
		concat(encShort(20), make([]byte, 12)),
		nil,
		{0},
	}
	for _, payload := range bad {
		if _, err := DecodeLocalTrigger(payload); err == nil {
			t.Errorf("DecodeLocalTrigger(%v): nil error", payload)
		}
	}
}

// TestListenerDecodersNoPanicOnGarbage sweeps every fixed-length
// listener decoder with all-0xFF payloads of the table length: all
// must decode (garbage values, no range checks) without panicking.
func TestListenerDecodersNoPanicOnGarbage(t *testing.T) {
	full := func(n int) []byte {
		b := make([]byte, n)
		for i := range b {
			b[i] = 0xFF
		}
		return b
	}
	if _, err := DecodeNpcOption(NativeOpNpc1Alt, full(3)); err != nil {
		t.Errorf("npc: %v", err)
	}
	if _, err := DecodeObjectOption(NativeOpLoc1, full(7)); err != nil {
		t.Errorf("object: %v", err)
	}
	if _, err := DecodePlayerOption(NativeOpPlayer1, full(3)); err != nil {
		t.Errorf("player: %v", err)
	}
	if _, err := DecodeGroundItemAction(NativeOpObj1, full(7)); err != nil {
		t.Errorf("ground: %v", err)
	}
	if _, err := DecodeItemOnPlayer(full(11)); err != nil {
		t.Errorf("itemOnPlayer: %v", err)
	}
	if _, err := DecodeItemOnNpc(full(11)); err != nil {
		t.Errorf("itemOnNpc: %v", err)
	}
	if _, err := DecodeItemOnObject(full(15)); err != nil {
		t.Errorf("itemOnObject: %v", err)
	}
	if _, err := DecodeItemOnGround(full(15)); err != nil {
		t.Errorf("itemOnGround: %v", err)
	}
	if _, err := DecodeSpellOnPlayer(full(11)); err != nil {
		t.Errorf("spellOnPlayer: %v", err)
	}
	if _, err := DecodeSpellOnNpc(full(11)); err != nil {
		t.Errorf("spellOnNpc: %v", err)
	}
	if _, err := DecodeSpellOnObject(full(15)); err != nil {
		t.Errorf("spellOnObject: %v", err)
	}
	if _, err := DecodeSpellOnGround(full(15)); err != nil {
		t.Errorf("spellOnGround: %v", err)
	}
	if _, err := DecodeAppearance(full(20)); err != nil {
		t.Errorf("appearance: %v", err)
	}
	if _, err := DecodeInventoryMove(full(4)); err != nil {
		t.Errorf("inventoryMove: %v", err)
	}
	if _, err := DecodeWidgetAction(NativeIfButton1, full(8)); err != nil {
		t.Errorf("widgetAction: %v", err)
	}
	if _, err := DecodeWidgetTarget(full(16)); err != nil {
		t.Errorf("widgetTarget: %v", err)
	}
	if _, err := DecodeWidgetDrag(full(16)); err != nil {
		t.Errorf("widgetDrag: %v", err)
	}
}

func TestDecodeVarpTransmit(t *testing.T) {
	cases := []struct {
		name   string
		varpID int
		value  int32
	}{
		{"run toggle on", 173, 1},
		{"run toggle off", 173, 0},
		{"run toggle nonzero value", 173, 2},
		{"run toggle negative value", 173, -1},
		{"combat style", 43, 3},
		{"auto-retaliate", 172, 0},
		{"zero varp", 0, 0},
		{"max varp id", 65535, -1},
		{"high-bit varp id stays unsigned", 32768, 1},
		{"max int value", 173, 2147483647},
		{"min int value", 173, -2147483648},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := DecodeVarpTransmit(concat(encShort(tc.varpID), encInt(tc.value)))
			if err != nil {
				t.Fatalf("DecodeVarpTransmit: %v", err)
			}
			want := VarpTransmit{VarpID: tc.varpID, Value: tc.value}
			if got != want {
				t.Errorf("got %+v, want %+v", got, want)
			}
		})
	}
	// Truncated and over-long payloads error: the opcode is
	// fixed-length 6.
	truncated := [][]byte{nil, {}, {0}, {0, 173}, {0, 173, 0, 0, 0}}
	for _, payload := range truncated {
		if _, err := DecodeVarpTransmit(payload); !errors.Is(err, ErrUnexpectedEnd) {
			t.Errorf("payload len %d: err %v, want ErrUnexpectedEnd", len(payload), err)
		}
	}
	if _, err := DecodeVarpTransmit(concat(encShort(173), encInt(1), []byte{0})); err == nil {
		t.Error("7-byte payload: want trailing-bytes error, got nil")
	}
}

// TestDecodeExamine pins the examine_npc / examine_object decodes:
// shortAdd for opcode 9, shortAddLE for opcode 85 (ClientProtocol.ts
// lines 557, 559). The id 291 (0x0123) exercises both transforms.
func TestDecodeExamine(t *testing.T) {
	if got, err := DecodeExamineNpc([]byte{0xA3, 0x01}); err != nil || got.ID != 291 {
		t.Errorf("DecodeExamineNpc = %+v, %v; want ID 291, nil", got, err)
	}
	if got, err := DecodeExamineLoc([]byte{0x01, 0xA3}); err != nil || got.ID != 291 {
		t.Errorf("DecodeExamineLoc = %+v, %v; want ID 291, nil", got, err)
	}
	if got, err := DecodeExamineNpc([]byte{0x80, 0x00}); err != nil || got.ID != 0 {
		t.Errorf("DecodeExamineNpc zero = %+v, %v; want ID 0, nil", got, err)
	}
	for name, p := range map[string][]byte{"empty": nil, "short": {0xA3}, "long": {0xA3, 0x01, 0x00}} {
		if _, err := DecodeExamineNpc(p); err == nil {
			t.Errorf("DecodeExamineNpc(%s): nil error", name)
		}
		if _, err := DecodeExamineLoc(p); err == nil {
			t.Errorf("DecodeExamineLoc(%s): nil error", name)
		}
	}
}
