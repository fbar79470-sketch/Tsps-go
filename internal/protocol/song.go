package protocol

import "encoding/binary"

// SongFades are MidiSongV2 fades, in client cycles (20 ms),
// porting the SongFades type (ClientProtocol.ts line 2796).
type SongFades struct {
	FadeOutDelay int
	FadeOutSpeed int
	FadeInDelay  int
	FadeInSpeed  int
}

// SongFadeDefaults is SONG_FADE_DEFAULTS (ClientProtocol.ts
// lines 2797-2798): what live sends for a region change (RSProx's
// MidiSongV2 defaults; every region change in the rsprox
// captures). It is also encodePlaySong's default when the caller
// passes no fades.
var SongFadeDefaults = SongFades{FadeOutDelay: 0, FadeOutSpeed: 60, FadeInDelay: 60, FadeInSpeed: 0}

// SongNoFades is SONG_NO_FADES (ClientProtocol.ts lines
// 2799-2800): what live sends with the current track right
// before a jingle, and what the music state machine sends with
// the next track after a gap of silence.
var SongNoFades = SongFades{FadeOutDelay: 0, FadeOutSpeed: 0, FadeInDelay: 0, FadeInSpeed: 0}

// EncodePlaySong encodes a song start (encodePlaySong,
// ClientProtocol.ts lines 2802-2810): PLAY_SONG (opcode 133,
// fixed length 10, no length prefix — TS calls packet() with
// lengthBytes 0): track id u16, then the four fades as u16s.
// Every value is masked to 16 bits, as TS masks each with
// & 0xffff. The length-table entry for opcode 133 is 10, so
// mustEncode cannot fail on this fixed 10-byte payload.
func EncodePlaySong(trackID int, fades SongFades) []byte {
	payload := make([]byte, 10)
	binary.BigEndian.PutUint16(payload[0:2], uint16(trackID&0xffff))
	binary.BigEndian.PutUint16(payload[2:4], uint16(fades.FadeOutDelay&0xffff))
	binary.BigEndian.PutUint16(payload[4:6], uint16(fades.FadeOutSpeed&0xffff))
	binary.BigEndian.PutUint16(payload[6:8], uint16(fades.FadeInDelay&0xffff))
	binary.BigEndian.PutUint16(payload[8:10], uint16(fades.FadeInSpeed&0xffff))
	return mustEncode(ServerPlaySong, payload)
}
