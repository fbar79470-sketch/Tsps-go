package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestSendProjectileQueuesFrame(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendProjectile(
		game.Location{X: 3200, Y: 3200}, game.Location{X: 3205, Y: 3200},
		0, 96, 100, 43, 31, 6, 51, 16, 64, 0,
	)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	// Heights scaled x4 (43->172, 31->124); lock-on 6 is NPC
	// index 5; no source actor.
	want := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     100,
		Source:           protocol.Tile{X: 3200, Y: 3200},
		Target:           protocol.Tile{X: 3205, Y: 3200},
		SourceHeight:     172,
		EndHeight:        124,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 51,
		EndCycleOffset:   96,
		TargetActor:      protocol.ProjectileActor{Kind: protocol.ProjectileActorNPC, Index: 5},
	}})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets[0], want)
	}
}

func TestSendProjectileFractionalHeights(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// A bow's TS heights (WeaponProfile.ts:60: 40.75/36.5)
	// send times four: 163/146 exactly, not the rounded
	// 41/37 ints' 164/148.
	s.SendProjectile(
		game.Location{X: 3200, Y: 3200}, game.Location{X: 3205, Y: 3200},
		0, 96, 100, 40.75, 36.5, 6, 51, 16, 64, 0,
	)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     100,
		Source:           protocol.Tile{X: 3200, Y: 3200},
		Target:           protocol.Tile{X: 3205, Y: 3200},
		SourceHeight:     163,
		EndHeight:        146,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 51,
		EndCycleOffset:   96,
		TargetActor:      protocol.ProjectileActor{Kind: protocol.ProjectileActorNPC, Index: 5},
	}})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets[0], want)
	}
}

func TestSendProjectileActorMapping(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	// Lock-on -4 is player index 3; source 8 is NPC index 7.
	s.SendProjectile(
		game.Location{X: 1, Y: 1}, game.Location{X: 2, Y: 2},
		0, 50, 1, 10, 10, -4, 0, 16, 64, 8,
	)
	packets := queuedPackets(t, s)
	want := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:   1,
		Source:         protocol.Tile{X: 1, Y: 1},
		Target:         protocol.Tile{X: 2, Y: 2},
		SourceHeight:   40,
		EndHeight:      40,
		Slope:          16,
		StartPos:       64,
		EndCycleOffset: 50,
		TargetActor:    protocol.ProjectileActor{Kind: protocol.ProjectileActorPlayer, Index: 3},
		SourceActor:    protocol.ProjectileActor{Kind: protocol.ProjectileActorNPC, Index: 7},
	}})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets, want)
	}
	// The offset argument is ignored: different offsets queue
	// identical frames.
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendProjectile(
		game.Location{X: 1, Y: 1}, game.Location{X: 2, Y: 2},
		99, 50, 1, 10, 10, -4, 0, 16, 64, 8,
	)
	if !bytes.Equal(queuedPackets(t, s2)[0], want) {
		t.Error("offset changed the frame; TS ignores it")
	}
}

func TestSendProjectileEndCycleFallback(t *testing.T) {
	// speed <= delay+1 falls back to delay + 5 + tiles*5.
	s := NewSession(NewHeadlessChannel(""))
	s.SendProjectile(
		game.Location{X: 3200, Y: 3200}, game.Location{X: 3205, Y: 3200},
		0, 10, 1, 10, 10, 0, 20, 16, 64, 0,
	)
	packets := queuedPackets(t, s)
	// delay 20, 5 tiles: 20 + 5 + 25 = 50.
	want := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     1,
		Source:           protocol.Tile{X: 3200, Y: 3200},
		Target:           protocol.Tile{X: 3205, Y: 3200},
		SourceHeight:     40,
		EndHeight:        40,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 20,
		EndCycleOffset:   50,
	}})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("fallback packet = %x, want %x", packets, want)
	}
	// speed exactly delay+1 also falls back (the TS test is
	// `<=` after the clamp).
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendProjectile(
		game.Location{X: 3200, Y: 3200}, game.Location{X: 3200, Y: 3200},
		0, 21, 1, 10, 10, 0, 20, 16, 64, 0,
	)
	want2 := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     1,
		Source:           protocol.Tile{X: 3200, Y: 3200},
		Target:           protocol.Tile{X: 3200, Y: 3200},
		SourceHeight:     40,
		EndHeight:        40,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 20,
		EndCycleOffset:   25, // 20 + 5 + 0 tiles
	}})
	if !bytes.Equal(queuedPackets(t, s2)[0], want2) {
		t.Errorf("boundary fallback = %x, want %x", queuedPackets(t, s2)[0], want2)
	}
	// Cross-plane end: distance -1 -> 0 tiles, delay + 5.
	s3 := NewSession(NewHeadlessChannel(""))
	s3.SendProjectile(
		game.Location{X: 3200, Y: 3200, Z: 0}, game.Location{X: 3210, Y: 3210, Z: 1},
		0, 10, 1, 10, 10, 0, 20, 16, 64, 0,
	)
	want3 := protocol.EncodeProjectiles([]protocol.ProjectileView{{
		ProjectileID:     1,
		Source:           protocol.Tile{X: 3200, Y: 3200, Level: 0},
		Target:           protocol.Tile{X: 3210, Y: 3210, Level: 1},
		SourceHeight:     40,
		EndHeight:        40,
		Slope:            16,
		StartPos:         64,
		StartCycleOffset: 20,
		EndCycleOffset:   25,
	}})
	if !bytes.Equal(queuedPackets(t, s3)[0], want3) {
		t.Errorf("cross-plane fallback = %x, want %x", queuedPackets(t, s3)[0], want3)
	}
}
