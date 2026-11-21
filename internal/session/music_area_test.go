package session

import (
	"testing"

	"tsps-go/internal/music"
)

// The area getMusicTrack override (PlayerSession.ts:393-395):
// when the player's area answers a valid track id, the
// music-area track list is replaced by that single track, asked
// every tick. These tests drive musicState directly, as the
// other music tests do.

func TestMusicAreaTrackOverride(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	rnd := &scriptedRand{values: []float64{0}}
	st.random = rnd.draw
	st.areaTrackSource = func() (int, bool) { return 40, true }
	var sink musicSink

	// Tile 0 is music area [10, 20]; the override replaces the
	// list with [40], so the first tick plays 40.
	st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 40 {
		t.Fatalf("songs = %+v, want one song track 40", songs)
	}
}

func TestMusicAreaTrackOverrideInvalidFallsBack(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	rnd := &scriptedRand{values: []float64{0}}
	st.random = rnd.draw
	// 0xffff is out of range (valid: 0 <= t < 0xffff), so the
	// music area's own tracks win.
	st.areaTrackSource = func() (int, bool) { return 0xffff, true }
	var sink musicSink

	st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("songs = %+v, want one song track 10 (area fallback)", songs)
	}
}

func TestMusicAreaTrackOverrideNoOpinionFallsBack(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	rnd := &scriptedRand{values: []float64{0}}
	st.random = rnd.draw
	st.areaTrackSource = func() (int, bool) { return 0, false }
	var sink musicSink

	st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("songs = %+v, want one song track 10 (area fallback)", songs)
	}
}

func TestMusicAreaTrackOverridePanicFallsBack(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	rnd := &scriptedRand{values: []float64{0}}
	st.random = rnd.draw
	// A throwing source is the callArea analogue: recovered,
	// treated as no opinion, and the tick continues.
	st.areaTrackSource = func() (int, bool) { panic("area boom") }
	var sink musicSink

	st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("songs = %+v, want one song track 10 (area fallback)", songs)
	}
}

func TestSetMusicAreaTrackSourceInstalls(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	src := func() (int, bool) { return 7, true }
	s.SetMusicAreaTrackSource(src)
	if s.music == nil || s.music.areaTrackSource == nil {
		t.Fatal("SetMusicAreaTrackSource must install the source")
	}
	// Nil removes the override.
	s.SetMusicAreaTrackSource(nil)
	if s.music.areaTrackSource != nil {
		t.Fatal("nil source must remove the override")
	}
}
