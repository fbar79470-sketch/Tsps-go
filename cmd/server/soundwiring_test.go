package main

import (
	"testing"

	"tsps-go/internal/game"
)

// soundWiringFakeSession is a SessionPort + SoundSender
// test double recording area-sound sends.
type soundWiringFakeSession struct {
	open  bool
	areas [][8]int // soundID, x, y, level, loops, delay, radius, attenuation
}

func (f *soundWiringFakeSession) SendPacket(packet []byte) {}
func (f *soundWiringFakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (f *soundWiringFakeSession) SendDestination(x, y int) {}
func (f *soundWiringFakeSession) SendVarp(id, value int)   {}
func (f *soundWiringFakeSession) FlushTick(cycle int64) error {
	return nil
}
func (f *soundWiringFakeSession) Close(code int, reason string) { f.open = false }
func (f *soundWiringFakeSession) IsOpen() bool                  { return f.open }
func (f *soundWiringFakeSession) SendSoundEffect(soundID, loopType, delay, volume int) {
}
func (f *soundWiringFakeSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
	f.areas = append(f.areas, [8]int{soundID, x, y, level, loops, delay, radius, attenuation})
}

func TestInstallSoundWiringBroadcastReachesWorldPlayers(t *testing.T) {
	t.Cleanup(func() { game.SetNetworkPlayerSource(nil) })
	world := game.NewWorld(game.Location{X: 3200, Y: 3200})
	hearer := game.NewPlayer("hearer", game.Location{X: 3205, Y: 3200})
	sess := &soundWiringFakeSession{open: true}
	hearer.Session = sess
	if _, ok := world.Players().Add(hearer); !ok {
		t.Fatal("Add hearer failed")
	}
	InstallSoundWiring(world)
	game.PlayAreaSound(game.PlayAreaSoundOptions{SoundID: 2907, X: 3200, Y: 3200, Radius: 5})
	if len(sess.areas) != 1 {
		t.Fatalf("area sends = %d, want 1", len(sess.areas))
	}
	want := [8]int{2907, 3200, 3200, 0, 0, 0, 5, 0}
	if sess.areas[0] != want {
		t.Errorf("area send = %v, want %v", sess.areas[0], want)
	}
}

func TestInstallSoundWiringNilWorldIsNoop(t *testing.T) {
	t.Cleanup(func() { game.SetNetworkPlayerSource(nil) })
	game.SetNetworkPlayerSource(nil)
	InstallSoundWiring(nil)
	// Unwired: a broadcast reaches nobody and panics nothing.
	game.PlayAreaSound(game.PlayAreaSoundOptions{SoundID: 2907, X: 3200, Y: 3200})
}

func TestInstallSoundWiringSeesLaterPlayers(t *testing.T) {
	t.Cleanup(func() { game.SetNetworkPlayerSource(nil) })
	world := game.NewWorld(game.Location{X: 3200, Y: 3200})
	InstallSoundWiring(world)
	// A player added AFTER the install is visible: the
	// source reads the live list on every broadcast.
	late := game.NewPlayer("late", game.Location{X: 3205, Y: 3200})
	sess := &soundWiringFakeSession{open: true}
	late.Session = sess
	if _, ok := world.Players().Add(late); !ok {
		t.Fatal("Add late failed")
	}
	game.PlayAreaSound(game.PlayAreaSoundOptions{SoundID: 100, X: 3200, Y: 3200})
	if len(sess.areas) != 1 {
		t.Fatalf("area sends = %d, want 1", len(sess.areas))
	}
}
