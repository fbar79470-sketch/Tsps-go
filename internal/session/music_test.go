package session

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"tsps-go/internal/music"
	"tsps-go/internal/protocol"
)

// The session music tests drive musicState directly: the Session
// field and updateMusic wrapper are coordinator wiring (see
// music.go), so the state machine is tested through its own seam
// with a capturing send function. Fixture areas are tiny and
// hand-built; no real data files, no cache, no network.

// musicFixtureAreas lays out region 0 (x 0..63, y 0..63) as:
// tiles x=0..1 area 0 [10, 20], x=2..3 area 1 [20, 30] (which
// shares track 20 with area 0, for the keep-track rule), x=4..5
// area 2 [40], x=6..7 a gap.
const musicFixtureAreas = `{"areas": [
  {"tracks": [10, 20]},
  {"tracks": [20, 30]},
  {"tracks": [40]}
], "tiles": {"0": {"0": "0*2,1*2,2*2,-*2"}}}`

// musicFixtureClassic covers the same tiles with one area [50],
// so classic lookups are distinguishable from modern ones.
const musicFixtureClassic = `{"areas": [
  {"tracks": [50]}
], "tiles": {"0": {"0": "0*8"}}}`

// musicLengthRow is a hand-built DbTable 44 row: id in column 4,
// name in column 1, length in column 3.
type musicLengthRow struct {
	id, length int
	name       string
}

func (r musicLengthRow) ID() int { return r.id }
func (r musicLengthRow) Int(column int, fallback ...int) int {
	switch column {
	case 4:
		return r.id
	case 3:
		return r.length
	default:
		if len(fallback) > 0 {
			return fallback[0]
		}
		return 0
	}
}
func (r musicLengthRow) String(column int) string {
	if column == 1 {
		return r.name
	}
	return ""
}
func (r musicLengthRow) Column(column int) []any { return nil }

func musicTestData(t *testing.T) *music.Data {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "music-areas.json"), []byte(musicFixtureAreas), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "music-areas-classic.json"), []byte(musicFixtureClassic), 0o644); err != nil {
		t.Fatal(err)
	}
	d := music.LoadAreas(dir)
	d.SetLengths([]music.Row{
		musicLengthRow{id: 10, length: 5, name: "Ten"},
		musicLengthRow{id: 20, length: 5, name: "Twenty"},
		musicLengthRow{id: 30, length: 5, name: "Thirty"},
		musicLengthRow{id: 40, length: 3, name: "Forty"},
		musicLengthRow{id: 50, length: 5, name: "Fifty"},
	})
	return d
}

// musicSink captures packets the state machine sends.
type musicSink struct {
	packets [][]byte
}

func (s *musicSink) send(p []byte) { s.packets = append(s.packets, p) }

// drain decodes the captured packets in send order, using the
// shared splitServerPackets helper.
func (s *musicSink) drain(t *testing.T) []protocol.Packet {
	t.Helper()
	var frame []byte
	for _, p := range s.packets {
		frame = append(frame, p...)
	}
	s.packets = nil
	if len(frame) == 0 {
		return nil
	}
	return splitServerPackets(t, frame)
}

func (s *musicSink) drainSongs(t *testing.T) []musicSong {
	t.Helper()
	var songs []musicSong
	for _, p := range s.drain(t) {
		if p.Opcode != protocol.ServerPlaySong {
			t.Fatalf("unexpected opcode %d in music output", p.Opcode)
		}
		songs = append(songs, parseSongPacket(t, p.Payload))
	}
	return songs
}

type musicSong struct {
	track int
	fades protocol.SongFades
}

func parseSongPacket(t *testing.T, payload []byte) musicSong {
	t.Helper()
	if len(payload) != 10 {
		t.Fatalf("song payload length %d, want 10", len(payload))
	}
	u16 := func(i int) int { return int(binary.BigEndian.Uint16(payload[i : i+2])) }
	return musicSong{
		track: u16(0),
		fades: protocol.SongFades{
			FadeOutDelay: u16(2),
			FadeOutSpeed: u16(4),
			FadeInDelay:  u16(6),
			FadeInSpeed:  u16(8),
		},
	}
}

// scriptedRand returns draws from values in order, repeating the
// last, and counts how many draws were consumed — which pins
// down exactly when the state machine picks (and when the
// keep-track rule skips the pick entirely).
type scriptedRand struct {
	values []float64
	next   int
	draws  int
}

func (r *scriptedRand) draw() float64 {
	r.draws++
	if r.next < len(r.values) {
		v := r.values[r.next]
		r.next++
		return v
	}
	return r.values[len(r.values)-1]
}

func musicAreaAudio(mode, settings int) map[int]int {
	return map[int]int{music.ModeVarp: mode, music.SettingsVarp: settings}
}

func musicTile(x int) protocol.Tile { return protocol.Tile{X: x, Y: 0, Level: 0} }

func TestMusicFirstTickStartsTrack(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	rnd := &scriptedRand{values: []float64{0}}
	st.random = rnd.draw
	var sink musicSink

	st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 10 || songs[0].fades != protocol.SongFadeDefaults {
		t.Fatalf("first tick songs = %+v, want one song track 10 with default fades", songs)
	}
	// Staying in the area sends nothing more.
	st.update(1, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("second tick sent %d packets, want 0", len(got))
	}
	if rnd.draws != 1 {
		t.Fatalf("draws = %d, want 1 (no repick while staying)", rnd.draws)
	}
}

func TestMusicKeepTrackOnOverlappingArea(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	// 0.6 picks index 1 of [10, 20]: track 20, which area 1
	// also holds.
	rnd := &scriptedRand{values: []float64{0.6, 0}}
	st.random = rnd.draw
	var sink musicSink
	audio := musicAreaAudio(music.AreaMode, 1)

	st.update(0, musicTile(0), audio, sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 || songs[0].track != 20 {
		t.Fatalf("first tick songs = %+v, want track 20", songs)
	}
	// Moving into the overlapping area keeps the track: no
	// packet, and no random draw (the TS ternary skips the
	// pick when keeping).
	st.update(1, musicTile(2), audio, sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("overlapping area sent %d packets, want 0", len(got))
	}
	if rnd.draws != 1 {
		t.Fatalf("draws = %d, want 1 (keep skips the pick)", rnd.draws)
	}
	// Moving into a disjoint area starts one of its tracks.
	st.update(2, musicTile(4), audio, sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 40 || songs[0].fades != protocol.SongFadeDefaults {
		t.Fatalf("disjoint area songs = %+v, want track 40 with default fades", songs)
	}
}

func TestMusicGapKeepsArea(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink
	audio := musicAreaAudio(music.AreaMode, 1)

	st.update(0, musicTile(0), audio, sink.send)
	sink.drain(t)
	// The gap keeps the area: nothing is sent, and returning to
	// the area afterwards is not a change either.
	for _, tick := range []int64{1, 2} {
		st.update(tick, musicTile(6), audio, sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("gap tick %d sent %d packets, want 0", tick, len(got))
		}
	}
	st.update(3, musicTile(0), audio, sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("return from gap sent %d packets, want 0", len(got))
	}
}

func TestMusicGapOnFirstTick(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink
	audio := musicAreaAudio(music.AreaMode, 1)

	// Starting in a gap: no area, no track, no packets — but
	// the state machine records the empty area, so entering
	// the area next tick is a change that starts a track.
	st.update(0, musicTile(6), audio, sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("first-tick gap sent %d packets, want 0", len(got))
	}
	st.update(1, musicTile(0), audio, sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("entering area songs = %+v, want track 10", songs)
	}
}

func TestMusicSongEndSilenceAndNextTrack(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink
	audio := musicAreaAudio(music.AreaMode, 1)

	st.update(0, musicTile(0), audio, sink.send)
	sink.drain(t)
	// Track 10 plays 5 ticks: areaTrackEnd = 0 + 5 + 1 = 6.
	for tick := int64(1); tick < 6; tick++ {
		st.update(tick, musicTile(0), audio, sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("tick %d sent %d packets, want 0", tick, len(got))
		}
	}
	// Tick 6: silence with the music-end fades, then the
	// current-track varp set to -1.
	st.update(6, musicTile(0), audio, sink.send)
	packets := sink.drain(t)
	if len(packets) != 2 {
		t.Fatalf("song-end packets = %d, want 2 (song + varp)", len(packets))
	}
	song := parseSongPacket(t, packets[0].Payload)
	if packets[0].Opcode != protocol.ServerPlaySong || song.track != music.SilenceTrack || song.fades != musicEndFades {
		t.Fatalf("song-end packet = opcode %d %+v, want song %d with end fades", packets[0].Opcode, song, music.SilenceTrack)
	}
	id, value := parseVarp(t, packets[1].Opcode, packets[1].Payload)
	if id != music.CurrentTrackVarp || value != -1 {
		t.Fatalf("song-end varp = %d=%d, want %d=-1", id, value, music.CurrentTrackVarp)
	}
	// Tick 7: the next track is 2 ticks later, not yet.
	st.update(7, musicTile(0), audio, sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("tick 7 sent %d packets, want 0", len(got))
	}
	// Tick 8: the next track skips the ended one (track 10) and
	// starts with NO fades.
	st.update(8, musicTile(0), audio, sink.send)
	songs := sink.drainSongs(t)
	if len(songs) != 1 || songs[0].track != 20 || songs[0].fades != protocol.SongNoFades {
		t.Fatalf("next-track songs = %+v, want track 20 with no fades", songs)
	}
}

func TestMusicSingleModeClearsTimers(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink

	// Single mode from login: the state machine never starts a
	// track (the chosen track is plugin-started) and clears
	// the timers.
	st.update(0, musicTile(0), musicAreaAudio(music.SingleMode, 1), sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("single-mode login sent %d packets, want 0", len(got))
	}
	// Switching into Single from Area clears the end timer: the
	// track never ends on its own afterwards.
	st.update(1, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	sink.drain(t)
	st.update(2, musicTile(0), musicAreaAudio(music.SingleMode, 1), sink.send)
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("switch to single sent %d packets, want 0", len(got))
	}
	for tick := int64(3); tick < 12; tick++ {
		st.update(tick, musicTile(0), musicAreaAudio(music.SingleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("single tick %d sent %d packets, want 0", tick, len(got))
		}
	}
}

func TestMusicShuffleFallback(t *testing.T) {
	t.Run("switch with nothing playing starts nothing", func(t *testing.T) {
		st := newMusicState()
		st.setData(musicTestData(t))
		st.random = (&scriptedRand{values: []float64{0}}).draw
		var sink musicSink
		// The TS fallback replays the previous track only
		// when there is one; with previous -1, shuffle
		// idles with no plugin pick.
		st.update(0, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("shuffle login sent %d packets, want 0", len(got))
		}
	})

	t.Run("replay on after end replays previous", func(t *testing.T) {
		st := newMusicState()
		st.setData(musicTestData(t))
		st.random = (&scriptedRand{values: []float64{0}}).draw
		var sink musicSink
		st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
		sink.drain(t)
		// Switching to shuffle lets the playing track finish.
		st.update(1, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("switch to shuffle sent %d packets, want 0", len(got))
		}
		for tick := int64(2); tick < 6; tick++ {
			st.update(tick, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		}
		sink.drain(t)
		st.update(6, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		sink.drain(t) // silence + varp
		st.update(7, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("tick 7 sent %d packets, want 0", len(got))
		}
		// Tick 8: the fallback replays track 10, no fades.
		st.update(8, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		songs := sink.drainSongs(t)
		if len(songs) != 1 || songs[0].track != 10 || songs[0].fades != protocol.SongNoFades {
			t.Fatalf("shuffle replay songs = %+v, want track 10 with no fades", songs)
		}
	})

	t.Run("replay off after end plays nothing", func(t *testing.T) {
		st := newMusicState()
		st.setData(musicTestData(t))
		st.random = (&scriptedRand{values: []float64{0}}).draw
		var sink musicSink
		noLoop := musicAreaAudio(music.AreaMode, 0)
		st.update(0, musicTile(0), noLoop, sink.send)
		sink.drain(t)
		shuffleNoLoop := musicAreaAudio(music.ShuffleMode, 0)
		for tick := int64(1); tick <= 6; tick++ {
			st.update(tick, musicTile(0), shuffleNoLoop, sink.send)
		}
		sink.drain(t) // silence + varp at tick 6
		st.update(8, musicTile(0), shuffleNoLoop, sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("shuffle no-replay tick 8 sent %d packets, want 0", len(got))
		}
		// Idling forever: no timers left to fire.
		st.update(20, musicTile(0), shuffleNoLoop, sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("shuffle no-replay tick 20 sent %d packets, want 0", len(got))
		}
	})

	t.Run("switch to shuffle gives a timerless track its length", func(t *testing.T) {
		st := newMusicState()
		st.setData(musicTestData(t))
		st.random = (&scriptedRand{values: []float64{0}}).draw
		var sink musicSink
		st.update(0, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
		sink.drain(t)
		// Single clears the timers but keeps the track.
		st.update(1, musicTile(0), musicAreaAudio(music.SingleMode, 1), sink.send)
		sink.drain(t)
		// Shuffle, idle with a track playing, gives it its
		// full length from now: ends at 2 + 5 + 1 = 8.
		st.update(2, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 0 {
			t.Fatalf("switch to shuffle sent %d packets, want 0", len(got))
		}
		for tick := int64(3); tick < 8; tick++ {
			st.update(tick, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
			if got := sink.drain(t); len(got) != 0 {
				t.Fatalf("tick %d sent %d packets, want 0", tick, len(got))
			}
		}
		st.update(8, musicTile(0), musicAreaAudio(music.ShuffleMode, 1), sink.send)
		if got := sink.drain(t); len(got) != 2 {
			t.Fatalf("tick 8 sent %d packets, want 2 (silence + varp)", len(got))
		}
	})
}

func TestMusicNoDataSendsNothing(t *testing.T) {
	st := newMusicState() // no setData: nil music data
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink
	for tick := int64(0); tick < 5; tick++ {
		st.update(tick, musicTile(int(tick)), musicAreaAudio(music.AreaMode, 1), sink.send)
	}
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("nil data sent %d packets, want 0", len(got))
	}

	// Empty loaded data (a directory with no files) behaves
	// the same.
	st = newMusicState()
	st.setData(music.LoadAreas(t.TempDir()))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	for tick := int64(0); tick < 5; tick++ {
		st.update(tick, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	}
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("empty data sent %d packets, want 0", len(got))
	}
}

func TestMusicNoLengthsNeverEnds(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "music-areas.json"), []byte(musicFixtureAreas), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "music-areas-classic.json"), []byte(musicFixtureClassic), 0o644); err != nil {
		t.Fatal(err)
	}
	d := music.LoadAreas(dir) // no SetLengths: no cache lengths
	st := newMusicState()
	st.setData(d)
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink
	audio := musicAreaAudio(music.AreaMode, 1)

	st.update(0, musicTile(0), audio, sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 {
		t.Fatalf("first tick songs = %+v, want 1", songs)
	}
	// With no length the track has no end time: it plays until
	// the area changes.
	for tick := int64(1); tick < 100; tick++ {
		st.update(tick, musicTile(0), audio, sink.send)
	}
	if got := sink.drain(t); len(got) != 0 {
		t.Fatalf("no-length ticks sent %d packets, want 0", len(got))
	}
}

func TestMusicAudioDefaults(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink

	// A nil audio map reads as Area mode, loop on, modern
	// areas: a track starts on the first tick.
	st.update(0, musicTile(0), nil, sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("nil audio songs = %+v, want track 10", songs)
	}
}

func TestMusicClassicAreas(t *testing.T) {
	st := newMusicState()
	st.setData(musicTestData(t))
	st.random = (&scriptedRand{values: []float64{0}}).draw
	var sink musicSink

	classic := musicAreaAudio(music.AreaMode, 1|(1<<music.AreaTypeShift))
	st.update(0, musicTile(0), classic, sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 || songs[0].track != 50 {
		t.Fatalf("classic songs = %+v, want track 50", songs)
	}
	// Switching back to modern areas reselects from the modern
	// map: the area identity includes the map.
	st.update(1, musicTile(0), musicAreaAudio(music.AreaMode, 1), sink.send)
	if songs := sink.drainSongs(t); len(songs) != 1 || songs[0].track != 10 {
		t.Fatalf("modern songs = %+v, want track 10", songs)
	}
}
