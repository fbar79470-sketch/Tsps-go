package protocol

import (
	"errors"
	"testing"
)

// shortAddLE encodes v the way a client does for Reader.ShortAddLE:
// high byte first, low byte with a +128 adjustment.
func shortAddLE(v int) []byte {
	return []byte{byte(v >> 8), byte(v + 128)}
}

// shortAdd encodes v the way a client does for Reader.ShortAdd: low
// byte (with a +128 adjustment) first, high byte second.
func shortAdd(v int) []byte {
	return []byte{byte(v + 128), byte(v >> 8)}
}

func TestDecodeMoveGameclick(t *testing.T) {
	// worldY=3524 shortAddLE, modifierFlags=2 byteNeg (encoded as -2
	// mod 256 = 0xFE), worldX=3089 shortAddLE, target loc id 0 shortAdd.
	payload := append(shortAddLE(3524), 0xFE)
	payload = append(payload, shortAddLE(3089)...)
	payload = append(payload, shortAdd(0)...)
	x, y, mods, target, err := DecodeMoveGameclick(payload)
	if err != nil {
		t.Fatalf("DecodeMoveGameclick: %v", err)
	}
	if x != 3089 || y != 3524 || mods != 2 || target != 0 {
		t.Errorf("decoded = (%d, %d, mods %d, target %d), want (3089, 3524, 2, 0)", x, y, mods, target)
	}
}

func TestDecodeMoveGameclickTargetAndWrap(t *testing.T) {
	// A nonzero target loc id, and coordinates whose low bytes wrap
	// the +128 adjustment (low byte 0x00 encodes as 0x80).
	payload := append(shortAddLE(0x0d00), byte(0)) // modifierFlags 0: -0
	payload = append(payload, shortAddLE(0x0c80)...)
	payload = append(payload, shortAdd(1234)...)
	x, y, mods, target, err := DecodeMoveGameclick(payload)
	if err != nil {
		t.Fatalf("DecodeMoveGameclick: %v", err)
	}
	if x != 0x0c80 || y != 0x0d00 || mods != 0 || target != 1234 {
		t.Errorf("decoded = (%d, %d, mods %d, target %d), want (3200, 3328, 0, 1234)", x, y, mods, target)
	}
	// modifierFlags 255: byteNeg decodes -1 mod 256 = 255.
	payload = append(shortAddLE(100), byte(1)) // -1 encodes as 1
	payload = append(payload, shortAddLE(200)...)
	payload = append(payload, shortAdd(0)...)
	_, _, mods, _, err = DecodeMoveGameclick(payload)
	if err != nil {
		t.Fatalf("DecodeMoveGameclick: %v", err)
	}
	if mods != 255 {
		t.Errorf("modifierFlags = %d, want 255", mods)
	}
}

func TestDecodeMoveGameclickMalformed(t *testing.T) {
	for _, payload := range [][]byte{
		nil,
		{0x0d, 0x44, 0xfe, 0x0c, 0x91, 0x80}, // 6 bytes: short
		{0x0d, 0x44, 0xfe, 0x0c, 0x91, 0x80, 0, 0}, // 8 bytes: trailing
	} {
		if _, _, _, _, err := DecodeMoveGameclick(payload); err == nil {
			t.Errorf("DecodeMoveGameclick(%v): nil error", payload)
		} else if len(payload) < 7 && !errors.Is(err, ErrUnexpectedEnd) {
			t.Errorf("DecodeMoveGameclick(%v): err = %v, want ErrUnexpectedEnd", payload, err)
		}
	}
}

func TestDecodeWalkFull(t *testing.T) {
	payload := []byte{0x0c, 0x11, 0x0d, 0xc4, 0x03} // (3089, 3524), flags 3
	x, y, run, mods, err := DecodeWalkFull(payload)
	if err != nil {
		t.Fatalf("DecodeWalkFull: %v", err)
	}
	if x != 3089 || y != 3524 || !run || mods != 1 {
		t.Errorf("decoded = (%d, %d, run %v, mods %d), want (3089, 3524, true, 1)", x, y, run, mods)
	}
	// flags 0x04: run bit clear, modifierFlags 2.
	x, y, run, mods, err = DecodeWalkFull([]byte{0, 1, 0, 2, 0x04})
	if err != nil {
		t.Fatalf("DecodeWalkFull: %v", err)
	}
	if x != 1 || y != 2 || run || mods != 2 {
		t.Errorf("decoded = (%d, %d, run %v, mods %d), want (1, 2, false, 2)", x, y, run, mods)
	}
	// flags 0xff: run set, modifierFlags 127.
	_, _, run, mods, err = DecodeWalkFull([]byte{0, 1, 0, 2, 0xff})
	if err != nil {
		t.Fatalf("DecodeWalkFull: %v", err)
	}
	if !run || mods != 127 {
		t.Errorf("flags 0xff: run %v mods %d, want true/127", run, mods)
	}
	// Agreement with DecodeWalk on the coordinates.
	dx, dy, err := DecodeWalk(payload)
	if err != nil {
		t.Fatalf("DecodeWalk: %v", err)
	}
	if dx != 3089 || dy != 3524 {
		t.Errorf("DecodeWalk = (%d, %d), want (3089, 3524)", dx, dy)
	}
}

func TestDecodeWalkFullMalformed(t *testing.T) {
	for _, payload := range [][]byte{
		nil,
		{0x0c, 0x11, 0x0d, 0xc4},          // 4 bytes: short
		{0x0c, 0x11, 0x0d, 0xc4, 0x00, 0}, // 6 bytes: trailing
	} {
		if _, _, _, _, err := DecodeWalkFull(payload); err == nil {
			t.Errorf("DecodeWalkFull(%v): nil error", payload)
		} else if len(payload) < 5 && !errors.Is(err, ErrUnexpectedEnd) {
			t.Errorf("DecodeWalkFull(%v): err = %v, want ErrUnexpectedEnd", payload, err)
		}
	}
}
