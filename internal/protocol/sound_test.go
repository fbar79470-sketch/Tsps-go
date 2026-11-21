package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeSoundUnpositioned(t *testing.T) {
	got := EncodeSound(2396, SoundOptions{Loops: 1})
	// Opcode 131, 1-byte length 8: id 0x095c, positioned 0,
	// loops 1, delay 0, radius 0, attenuation 0.
	want := []byte{131, 8, 0x09, 0x5c, 0, 1, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeSound unpositioned = %x, want %x", got, want)
	}
}

func TestEncodeSoundPositioned(t *testing.T) {
	got := EncodeSound(2907, SoundOptions{
		Positioned: true, X: 3200, Y: 3200, Level: 0,
		Loops: 1, Delay: 10, Radius: 5, Attenuation: 0,
	})
	// Opcode 131, length 13: id 0x0b5b, positioned 1, x/y
	// 0x0c80, level 0, loops 1, delay 10, radius 5, atten 0.
	want := []byte{131, 13, 0x0b, 0x5b, 1, 0x0c, 0x80, 0x0c, 0x80, 0, 1, 0, 10, 5, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeSound positioned = %x, want %x", got, want)
	}
}

func TestEncodeSoundClamps(t *testing.T) {
	got := EncodeSound(100, SoundOptions{Loops: 0, Delay: -5, Radius: 99, Attenuation: -3})
	// Loops 0 clamps to 1, delay -5 to 0, radius 99 to 31,
	// attenuation -3 to 0.
	want := []byte{131, 8, 0, 100, 0, 1, 0, 0, 31, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeSound clamps = %x, want %x", got, want)
	}
	// Loops above 255: Math.max(1, 300) truncated to a byte,
	// as the TS Buffer assignment truncates.
	got = EncodeSound(100, SoundOptions{Loops: 300})
	if got[5] != 44 {
		t.Errorf("EncodeSound loops 300 byte = %d, want 44", got[5])
	}
	// Positioned with Y/level unset: TS writes (y ?? 0) and
	// (level ?? 0).
	got = EncodeSound(100, SoundOptions{Positioned: true, X: 7})
	want = []byte{131, 13, 0, 100, 1, 0, 7, 0, 0, 0, 1, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeSound positioned defaults = %x, want %x", got, want)
	}
}

func TestEncodePlayJingleShuffledIME(t *testing.T) {
	got := EncodePlayJingle(100, 0x123456)
	// Opcode 132, fixed 5, no length prefix: id u16, then
	// the shuffled delay bytes value>>16, value, value>>8.
	want := []byte{132, 0, 100, 0x12, 0x56, 0x34}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodePlayJingle = %x, want %x", got, want)
	}
}

func TestEncodePlayJingleClamps(t *testing.T) {
	if got := EncodePlayJingle(1, -1); !bytes.Equal(got, []byte{132, 0, 1, 0, 0, 0}) {
		t.Errorf("EncodePlayJingle delay -1 = %x, want zero delay", got)
	}
	if got := EncodePlayJingle(1, 0x1000000); !bytes.Equal(got, []byte{132, 0, 1, 0xff, 0xff, 0xff}) {
		t.Errorf("EncodePlayJingle delay overflow = %x, want 0xffffff", got)
	}
	if got := EncodePlayJingle(1, 0); !bytes.Equal(got, []byte{132, 0, 1, 0, 0, 0}) {
		t.Errorf("EncodePlayJingle delay 0 = %x", got)
	}
}
