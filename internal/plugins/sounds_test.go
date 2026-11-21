package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

func TestCoreSoundSurface(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "p").Core()
	sound := core.Sound().New(100, 7, 30, 1)
	if sound.ID() != 100 || sound.Volume() != 7 || sound.Delay() != 30 || sound.LoopType() != 1 {
		t.Errorf("Sound().New = %d/%d/%d/%d, want 100/7/30/1", sound.ID(), sound.Volume(), sound.Delay(), sound.LoopType())
	}
	if s, ok := core.Sound().ResolveKnownSound("LEVEL_UP"); !ok || s != game.SoundLevelUp {
		t.Errorf("ResolveKnownSound = %v/%v, want LEVEL_UP", s, ok)
	}
	names := core.Sound().KnownSoundNames()
	if len(names) != 161 {
		t.Errorf("KnownSoundNames = %d, want 161", len(names))
	}
	// Safe on a nil Core: the surface is stateless.
	var nilCore *Core
	if nilCore.Sound().New(1, 1, 0, 0).ID() != 1 {
		t.Error("nil Core Sound().New failed")
	}
	if _, ok := nilCore.Sound().ResolveKnownSound(2396); !ok {
		t.Error("nil Core ResolveKnownSound failed")
	}
}

func TestCoreSoundsServiceDelegates(t *testing.T) {
	h, _ := newTestHub()
	core := newApi(h, "p").Core()
	// Delegation without a session or player source: all
	// four sends drop silently, none panic.
	p := game.NewPlayer("nobody", game.Location{X: 3200, Y: 3200})
	core.Sounds().SendSound(p, game.SoundLevelUp)
	core.Sounds().SendSoundEffect(p, 2396, 0, 0, 1)
	core.Sounds().PlayAreaSoundAt(p, game.SoundLevelUp, 5)
	core.Sounds().PlayAreaSound(game.PlayAreaSoundOptions{SoundID: 2396, X: 3200, Y: 3200})
	var nilCore *Core
	nilCore.Sounds().SendSound(nil, game.SoundLevelUp)
}
