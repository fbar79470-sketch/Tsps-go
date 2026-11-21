package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/protocol"
)

func TestSendSoundEffectQueuesSoundPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendSoundEffect(2396, 1, 0, 1)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeSound(2396, protocol.SoundOptions{Loops: 1})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets[0], want)
	}
	// The volume argument never reaches the wire: a different
	// volume queues the identical frame.
	s2 := NewSession(NewHeadlessChannel(""))
	s2.SendSoundEffect(2396, 1, 0, 255)
	if !bytes.Equal(queuedPackets(t, s2)[0], want) {
		t.Errorf("volume 255 packet = %x, want %x", queuedPackets(t, s2)[0], want)
	}
}

func TestSendSoundForcesLoopsOne(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendSound(2396, 1, 7)
	packets := queuedPackets(t, s)
	want := protocol.EncodeSound(2396, protocol.SoundOptions{Loops: 1, Delay: 7})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets, want)
	}
}

func TestSendAreaSoundQueuesPositionedPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendAreaSound(2907, 3200, 3200, 0, 1, 3, 5, 2)
	packets := queuedPackets(t, s)
	want := protocol.EncodeSound(2907, protocol.SoundOptions{
		Positioned: true, X: 3200, Y: 3200, Loops: 1, Delay: 3, Radius: 5, Attenuation: 2,
	})
	if len(packets) != 1 || !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets, want)
	}
}

func TestCurrentMusicTrack(t *testing.T) {
	// Zero-value Session: no track.
	var zero Session
	if got := zero.CurrentMusicTrack(); got != -1 {
		t.Errorf("zero Session track = %d, want -1", got)
	}
	s := NewSession(NewHeadlessChannel(""))
	if got := s.CurrentMusicTrack(); got != -1 {
		t.Errorf("fresh Session track = %d, want -1", got)
	}
	s.music.lastMusicTrack = 98
	if got := s.CurrentMusicTrack(); got != 98 {
		t.Errorf("track = %d, want 98", got)
	}
}

func TestSendJingleWithCurrentTrackFirst(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.music.lastMusicTrack = 98
	s.SendJingle(50, 0)
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued packets = %d, want 2 (track + jingle)", len(packets))
	}
	wantTrack := protocol.EncodePlaySong(98, protocol.SongNoFades)
	if !bytes.Equal(packets[0], wantTrack) {
		t.Errorf("track-first packet = %x, want %x", packets[0], wantTrack)
	}
	wantJingle := protocol.EncodePlayJingle(50, 0)
	if !bytes.Equal(packets[1], wantJingle) {
		t.Errorf("jingle packet = %x, want %x", packets[1], wantJingle)
	}
}

func TestSendJingleWithoutTrack(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendJingle(50, 12)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodePlayJingle(50, 12)
	if !bytes.Equal(packets[0], want) {
		t.Errorf("packet = %x, want %x", packets[0], want)
	}
}
