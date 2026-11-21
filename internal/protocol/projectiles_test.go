package protocol

import (
	"bytes"
	"testing"
)

func TestEncodeProjectilesEmpty(t *testing.T) {
	got := EncodeProjectiles(nil)
	// Opcode 84, 2-byte length 2, count 0.
	want := []byte{84, 0, 2, 0, 0}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeProjectiles empty = %x, want %x", got, want)
	}
}

func TestEncodeProjectilesOneEntry(t *testing.T) {
	got := EncodeProjectiles([]ProjectileView{{
		ProjectileID:     100,
		Source:           Tile{X: 3200, Y: 3200, Level: 0},
		Target:           Tile{X: 3205, Y: 3200, Level: 0},
		SourceHeight:     172,
		EndHeight:        124,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 51,
		EndCycleOffset:   96,
		TargetActor:      ProjectileActor{Kind: ProjectileActorNPC, Index: 5},
	}})
	// 2-byte length 31: count 1, then the 29-byte entry.
	want := []byte{
		84, 0, 31,
		0, 1,
		0, 100, // id
		0x0c, 0x80, 0x0c, 0x80, 0, // source tile
		0, 172, // source height
		0x0c, 0x85, 0x0c, 0x80, 0, // target tile
		0, 124, // end height
		16,    // slope
		0, 64, // start pos
		0, 51, // start cycle offset
		0, 96, // end cycle offset
		0, 0, 0, // no source actor
		2, 0, 5, // target actor: NPC 5
	}
	if !bytes.Equal(got, want) {
		t.Errorf("EncodeProjectiles one = %x, want %x", got, want)
	}
	if len(got) != 3+31 {
		t.Errorf("EncodeProjectiles length = %d, want 34", len(got))
	}
}

func TestEncodeProjectilesActors(t *testing.T) {
	got := EncodeProjectiles([]ProjectileView{{
		ProjectileID: 1,
		SourceActor:  ProjectileActor{Kind: ProjectileActorPlayer, Index: 3},
		TargetActor:  ProjectileActor{Kind: ProjectileActorPlayer, Index: 9},
	}})
	// Source actor at payload offset 2+23, target at 2+26
	// (plus the 3-byte frame header).
	if got[3+2+23] != 1 || got[3+2+24] != 0 || got[3+2+25] != 3 {
		t.Errorf("source actor bytes = %x, want kind 1 index 3", got[3+2+23:3+2+26])
	}
	if got[3+2+26] != 1 || got[3+2+28] != 9 {
		t.Errorf("target actor bytes = %x, want kind 1 index 9", got[3+2+26:3+2+29])
	}
}

func TestEncodeProjectilesMasks(t *testing.T) {
	got := EncodeProjectiles([]ProjectileView{{
		ProjectileID: 0x10001, Slope: 0x100, StartPos: -1,
		Source: Tile{X: -1, Y: 0x10000, Level: 0x101},
	}})
	// id & 0xffff = 1, slope byte = 0, startPos & 0xffff =
	// 0xffff, source x 0xffff, y 0, level byte 1.
	payload := got[3:]
	if payload[2] != 0 || payload[3] != 1 {
		t.Errorf("masked id = %x, want 0001", payload[2:4])
	}
	if payload[4] != 0xff || payload[5] != 0xff || payload[6] != 0 || payload[7] != 0 || payload[8] != 1 {
		t.Errorf("masked source tile = %x, want ffff 0000 01", payload[4:9])
	}
	if payload[18] != 0 {
		t.Errorf("masked slope = %d, want 0", payload[18])
	}
	if payload[19] != 0xff || payload[20] != 0xff {
		t.Errorf("masked startPos = %x, want ffff", payload[19:21])
	}
}
