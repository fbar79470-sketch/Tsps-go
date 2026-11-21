package protocol

import (
	"bytes"
	"testing"
)

func TestEncodePlaySongByteExact(t *testing.T) {
	// Track 363 (0x016b) with the default fades: opcode 133, no
	// length prefix, then track + 0/60/60/0 as big-endian u16s.
	want := []byte{133, 0x01, 0x6b, 0, 0, 0, 60, 0, 60, 0, 0}
	if got := EncodePlaySong(363, SongFadeDefaults); !bytes.Equal(got, want) {
		t.Fatalf("EncodePlaySong(363, defaults) = %v, want %v", got, want)
	}
	// The silence track with the music-end fades (PlayerSession
	// MUSIC_END_FADES {0,20,0,0}): the song-end packet.
	want = []byte{133, 0, 147, 0, 0, 0, 20, 0, 0, 0, 0}
	fades := SongFades{FadeOutDelay: 0, FadeOutSpeed: 20, FadeInDelay: 0, FadeInSpeed: 0}
	if got := EncodePlaySong(147, fades); !bytes.Equal(got, want) {
		t.Fatalf("EncodePlaySong(147, end fades) = %v, want %v", got, want)
	}
	// No fades: all four fade fields zero.
	want = []byte{133, 0, 7, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := EncodePlaySong(7, SongNoFades); !bytes.Equal(got, want) {
		t.Fatalf("EncodePlaySong(7, no fades) = %v, want %v", got, want)
	}
}

func TestEncodePlaySongMasking(t *testing.T) {
	// Every value is masked to 16 bits, as TS masks with
	// & 0xffff: a track id above the range wraps, and a
	// negative fade keeps its low 16 bits.
	got := EncodePlaySong(0x10005, SongFades{FadeOutDelay: -1, FadeOutSpeed: 0x1003c, FadeInDelay: 0, FadeInSpeed: 0})
	want := []byte{133, 0, 5, 0xff, 0xff, 0, 60, 0, 0, 0, 0}
	if !bytes.Equal(got, want) {
		t.Fatalf("EncodePlaySong masked = %v, want %v", got, want)
	}
}

func TestEncodePlaySongLengthTable(t *testing.T) {
	// The length table must carry the fixed 10-byte payload
	// encodePlaySong writes; mustEncode would panic otherwise.
	length, ok := ServerPacketLength(ServerPlaySong)
	if !ok || length != 10 {
		t.Fatalf("ServerPacketLength(133) = %d, %v, want 10, true", length, ok)
	}
	if ServerPlaySong != 133 {
		t.Fatalf("ServerPlaySong = %d, want 133", ServerPlaySong)
	}
}
