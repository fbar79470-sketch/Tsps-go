package session

import (
	"math/rand"

	"tsps-go/internal/music"
	"tsps-go/internal/protocol"
)

// This file ports the music block of flushClient
// (PlayerSession.ts:375-452) plus its state fields (130-141) and
// helpers startAreaTrack/startShuffleTrack/areaTrackEnd
// (147-183), reduced to the ported state: no boats, no
// plugins, and areas only through the getMusicTrack override
// source (SetMusicAreaTrackSource).
//
// COORDINATOR WIRING (this file cannot declare it: the Session
// struct lives in session.go, which this milestone must not
// edit): Session needs a `music *musicState` field, initialized
// by NewSession with newMusicState(); a SetMusicData method
// storing the boot-loaded *music.Data (nil until wired, meaning
// the TS empty-areas/no-lengths behavior); and an updateMusic
// method called from FlushTick inside the region_updates
// position — after the owner/scene tile is computed, before the
// initial-sync block — passing the tick, the scene tile (with
// no boats the music tile is the owner's own tile), and the
// owner's audio settings snapshot:
//
//	func (s *Session) SetMusicData(d *music.Data) {
//		if s.music == nil {
//			s.music = newMusicState()
//		}
//		s.music.setData(d)
//	}
//
//	func (s *Session) updateMusic(tick int64, tile protocol.Tile, audio map[int]int) {
//		if s.music == nil {
//			s.music = newMusicState()
//		}
//		s.music.update(tick, tile, audio, s.SendPacket)
//	}
//
// The audio map is read-only input here: live varp_transmit
// application of audio settings (NetworkBuilder.ts:1167,
// applyAudioSettingFromClient) is NOT STARTED, so the map only
// ever holds the entry-time snapshot.
//
// Thread-safety: like the rest of the per-tick sync state, the
// music state is tick-goroutine-only — only FlushTick (via
// updateMusic) touches it. SetMusicData runs at AddPlayer time,
// before the first FlushTick, so no lock guards the data field.
//
// GAPS (documented, not worked around):
//   - The "music:unlock-track" events (PlayerSession.ts:412-416)
//     are not emitted: no plugin system exists. Entering an area
//     unlocks nothing in Go.
//   - The area getMusicTrack override (PlayerSession.ts:393-395)
//     is wired through SetMusicAreaTrackSource: the
//     coordinator installs a closure over the owner player
//     that asks the player's current area for a track via
//     game.CallAreaMusicTrack (nil area = no opinion). With
//     no source installed the selection is always the music
//     area's track list, as before. The
//     templated-instance tile mapping (instanceSourceTile)
//     is wired through the owner snapshot's MusicTile (see
//     flush.go): the adapter renders the source tile for
//     templated instances and the root-location tile
//     otherwise; see areaTrackOverride.
//   - Shuffle's "music:shuffle-track" plugin pick
//     (PlayerSession.ts:164-177) does not exist: startShuffle
//     runs the TS FALLBACK ONLY — replay the previous track iff
//     the loop setting is on, else nothing.
//   - Jingles (sendJingle, PacketSender.ts:285-291) send
//     through Session.SendJingle (sound_sender.go), which
//     reads the current track through CurrentMusicTrack
//     below (getCurrentMusicTrack, PlayerSession.ts:147-149).
//     setCurrentMusicTrack (PlayerSession.ts:151-153) has
//     no Go equivalent and no TS caller at bfd81e3, and
//     noteSongSent (PlayerSession.ts:195-200) is NOT
//     STARTED: nothing outside this state machine sends
//     songs, so no takeover from the area track can happen.
//     The startingAreaTrack reentrancy flag has no Go
//     equivalent either: sends here queue packets directly
//     and never call back into noteSongSent. (SendJingle's
//     track-first frame is a re-send of the current area
//     track, not a takeover, so it needs no noteSongSent.)
//   - The "audio:play-song" plugin event sendSong emits
//     (PacketSender.ts:270-279) is not emitted, and the
//     CURRENT_TRACK_VARP set to the playing track is
//     plugin-owned: this state machine sends the varp only with
//     -1 on song end, exactly as the flushClient block does.
//   - Music.instanceSourceTile (the templated-instance tile
//     mapping) is ported as
//     TemplatedInstanceArea.InstanceSourceTile and wired
//     through the adapter's MusicTile (see syncsource.go in
//     cmd/server): the caller passes the rendered music
//     tile, which is the source tile inside a templated
//     instance and the root-location tile otherwise.

// musicEndFades is MUSIC_END_FADES (PlayerSession.ts:51): a
// finished area track fades out into silence.
var musicEndFades = protocol.SongFades{FadeOutDelay: 0, FadeOutSpeed: 20, FadeInDelay: 0, FadeInSpeed: 0}

// musicState is the per-session music state machine. The field
// names mirror the PlayerSession fields (PlayerSession.ts:132-141):
// lastMusicArea is the music area the player was last in, as its
// track list; lastMusicTrack the playing track (-1 none);
// lastMusicMode the last music mode (-1 before the first flush);
// lastResolvedMusicTracks the last selection; musicTrackEndsAt
// the tick the playing area track ends on (-1); musicNextTrackAt
// the tick the next area track starts on (-1); musicEndedTrack
// the area track that just ended (-1); musicTick the current
// flush tick.
//
// Two Go-only fields carry TS reference semantics that slices
// cannot: hasLastMusicArea / hasLastResolved distinguish
// "undefined before the first flush" from a defined-but-empty
// track list (TS: lastMusicArea/lastResolvedMusicTracks are
// `| undefined`, and an empty array is truthy), and
// lastAreaID/lastAreaClassic carry the identity of
// lastMusicArea, which TS compares by track-list reference
// (musicArea !== lastMusicArea). A gap keeps the last area's
// identity; a fresh lookup takes the looked-up area's.
type musicState struct {
	data   *music.Data
	random func() float64

	// areaTrackSource, when set, asks the owner's current
	// area for a music-track override each tick
	// (PlayerSession.ts:393-395). It returns the track id
	// and true, or false for no opinion (no area, or the
	// area's getMusicTrack returns null). Installed by
	// SetMusicAreaTrackSource; nil until wired. Called on
	// the tick goroutine only, like update itself.
	areaTrackSource func() (int, bool)

	lastMusicArea           []int
	hasLastMusicArea        bool
	lastAreaID              int
	lastAreaClassic         bool
	lastMusicTrack          int
	lastMusicMode           int
	lastResolvedMusicTracks []int
	hasLastResolved         bool
	musicTrackEndsAt        int64
	musicNextTrackAt        int64
	musicEndedTrack         int
	musicTick               int64
}

// newMusicState returns the music state as TS constructs it:
// no track, no mode, no end/next times, and the default random
// source (math/rand, auto-seeded). Tests replace random with a
// deterministic draw.
func newMusicState() *musicState {
	return &musicState{
		random:           rand.Float64,
		lastAreaID:       -1,
		lastMusicTrack:   -1,
		lastMusicMode:    -1,
		musicTrackEndsAt: -1,
		musicNextTrackAt: -1,
		musicEndedTrack:  -1,
	}
}

// setData installs the boot-loaded music data. A nil Data means
// the TS empty-areas/no-lengths behavior: no track ever starts,
// and the state machine idles without sending.
func (m *musicState) setData(d *music.Data) {
	m.data = d
}

// update runs one tick of the music block. tile is the music
// tile (the owner's own tile: no instances or boats exist);
// audio is the owner's audio settings snapshot, read-only; send
// queues outbound packets (Session.SendPacket). It ports the
// region_updates music logic (PlayerSession.ts:375-452) line by
// line; the comments cite the TS lines each step mirrors.
func (m *musicState) update(tick int64, tile protocol.Tile, audio map[int]int, send func([]byte)) {
	// The tile's music area (PlayerSession.ts:383-387). A tile
	// with no music area (a gap between areas, an unmapped
	// spot) keeps the one the player came from.
	classic := music.Classic(audio)
	tileArea, areaID := m.data.AreaAt(tile.X, tile.Y, tile.Level, classic)
	musicArea, musicAreaID, musicAreaClassic := tileArea, areaID, classic
	areaChanged := false
	if len(tileArea) == 0 && m.hasLastMusicArea {
		musicArea, musicAreaID, musicAreaClassic = m.lastMusicArea, m.lastAreaID, m.lastAreaClassic
	} else {
		// Fresh lookup: the area changed unless this is the
		// same area of the same map as last tick. A gap id
		// (-1) never equals a real area; the classic flag
		// only distinguishes real areas (a gap's map is
		// irrelevant — TS compares the shared NO_TRACKS
		// reference, which has no map).
		areaChanged = !m.hasLastMusicArea || areaID != m.lastAreaID || (areaID >= 0 && classic != m.lastAreaClassic)
	}

	// Mode and replay (PlayerSession.ts:391-392). A missing
	// mode varp reads 0, which is AREA_MODE, as TS's `??`
	// default; Setting defaults a missing varp 19 to
	// DEFAULT_SETTINGS (loop on).
	musicMode := audio[music.ModeVarp]
	replay := music.Setting(audio, music.LoopBit, 1) == 1

	// The selection (PlayerSession.ts:393-400). The area's
	// getMusicTrack override, when it answers a valid track
	// id, replaces the music area's track list with the
	// single track; any other answer falls back to the
	// music area. The override is asked every tick, as TS
	// asks it on every flush.
	m.musicTick = tick
	tracks := musicArea
	if track, ok := m.areaTrackOverride(); ok && track >= 0 && track < 0xffff {
		tracks = []int{track}
	}
	selectionChanged := !sameMusicTracks(tracks, m.lastResolvedMusicTracks, m.hasLastResolved)
	// Area and Shuffle play one track after another; Single
	// keeps the one chosen.
	timed := musicMode == music.AreaMode || musicMode == music.ShuffleMode

	if areaChanged || musicMode != m.lastMusicMode || selectionChanged {
		previousMusicMode := m.lastMusicMode
		m.lastMusicArea = musicArea
		m.hasLastMusicArea = true
		m.lastAreaID = musicAreaID
		m.lastAreaClassic = musicAreaClassic
		m.lastMusicMode = musicMode
		m.lastResolvedMusicTracks = tracks
		m.hasLastResolved = true
		// A track that also belongs to the new area plays
		// on; otherwise one of the area's starts
		// (PlayerSession.ts:407-410). The pick is skipped
		// when keeping, exactly as the TS ternary skips it
		// (no random draw is consumed).
		keep := musicMode == music.AreaMode && previousMusicMode == music.AreaMode && containsTrack(tracks, m.lastMusicTrack)
		var track int
		hasTrack := false
		if keep {
			track, hasTrack = m.lastMusicTrack, true
		} else {
			previous := -1
			if musicMode == music.AreaMode {
				previous = m.musicEndedTrack
			}
			track, hasTrack = music.PickAreaTrack(tracks, previous, m.random, replay)
		}
		// GAP: the "music:unlock-track" events
		// (PlayerSession.ts:412-416) are not emitted — no
		// plugin system exists. See the file header.
		if musicMode == music.ShuffleMode {
			// Shuffle ignores areas. Switched to, it lets
			// the playing track finish (one without an end
			// time, a Single choice, gets its full length
			// from now); with nothing playing it starts
			// one, as it does on an area change after
			// another song took over
			// (PlayerSession.ts:417-427).
			idle := m.musicTrackEndsAt < 0 && m.musicNextTrackAt < 0
			if musicMode != previousMusicMode && m.lastMusicTrack >= 0 && idle {
				m.musicTrackEndsAt = m.areaTrackEnd(m.lastMusicTrack)
			} else if idle && (musicMode != previousMusicMode || areaChanged) {
				m.startShuffleTrack(-1, replay, protocol.SongFadeDefaults, send)
			}
		} else if musicMode != music.AreaMode || !hasTrack {
			m.musicTrackEndsAt = -1
			m.musicNextTrackAt = -1
			m.musicEndedTrack = -1
		} else if !keep {
			m.startAreaTrack(track, protocol.SongFadeDefaults, send)
		}
	} else if timed && m.musicNextTrackAt >= 0 && tick >= m.musicNextTrackAt {
		if musicMode == music.ShuffleMode {
			m.startShuffleTrack(m.musicEndedTrack, replay, protocol.SongNoFades, send)
		} else {
			track, hasTrack := music.PickAreaTrack(tracks, m.musicEndedTrack, m.random, replay)
			if !hasTrack {
				m.musicNextTrackAt = -1
			} else {
				m.startAreaTrack(track, protocol.SongNoFades, send)
			}
		}
	} else if timed && m.musicTrackEndsAt >= 0 && tick >= m.musicTrackEndsAt {
		// The track has finished: silence, no current
		// track, and the next one two ticks later
		// (PlayerSession.ts:440-449).
		m.musicTrackEndsAt = -1
		m.musicNextTrackAt = tick + 2
		m.musicEndedTrack = m.lastMusicTrack
		m.lastMusicTrack = -1
		send(protocol.EncodePlaySong(music.SilenceTrack, musicEndFades))
		send(protocol.EncodeVarp(music.CurrentTrackVarp, -1))
	}
}

// startAreaTrack starts an area track; it plays for its cache
// length plus a tick, then the area moves on
// (PlayerSession.ts:147-158). fades is the TS fades argument:
// SongFadeDefaults for a fresh start, SongNoFades for the
// next track after silence. Only the song packet is sent here:
// the CURRENT_TRACK_VARP set to the playing track is
// plugin-owned (see the file header).
func (m *musicState) startAreaTrack(track int, fades protocol.SongFades, send func([]byte)) {
	m.lastMusicTrack = track
	m.musicNextTrackAt = -1
	m.musicEndedTrack = -1
	m.musicTrackEndsAt = m.areaTrackEnd(track)
	send(protocol.EncodePlaySong(track, fades))
}

// startShuffleTrack is shuffle's next track
// (PlayerSession.ts:164-177) with the TS FALLBACK ONLY: with no
// plugin pick, the ended one again only when the loop setting
// is on, and nothing otherwise. When nothing plays, the timers
// clear and the state machine idles until the next change.
func (m *musicState) startShuffleTrack(previous int, replay bool, fades protocol.SongFades, send func([]byte)) {
	if !(replay && previous >= 0) {
		m.musicNextTrackAt = -1
		m.musicEndedTrack = -1
		return
	}
	m.startAreaTrack(previous, fades, send)
}

// areaTrackEnd is the tick a track ends on
// (PlayerSession.ts:179-182): the current music tick plus its
// cache length plus one, or -1 when the cache has no length.
func (m *musicState) areaTrackEnd(track int) int64 {
	length, ok := m.data.LengthTicks(track)
	if !ok {
		return -1
	}
	return m.musicTick + int64(length) + 1
}

// sameMusicTracks ports sameTracks (PlayerSession.ts:53-55): the
// lists are the same only when the previous one is defined and
// every element matches in order.
func sameMusicTracks(a, b []int, bDefined bool) bool {
	if !bDefined || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsTrack(tracks []int, track int) bool {
	for _, t := range tracks {
		if t == track {
			return true
		}
	}
	return false
}

// SetMusicData installs the boot-loaded music data (areas + track
// lengths). It runs at AddPlayer time, before the first FlushTick.
// A nil Data means the TS empty-areas/no-lengths behavior. The
// nil-music guard covers zero-value Sessions built without
// NewSession; NewSession always installs a state machine.
func (s *Session) SetMusicData(d *music.Data) {
	if s.music == nil {
		s.music = newMusicState()
	}
	s.music.setData(d)
}

// setAreaTrackSource installs the area-track override
// source (see the areaTrackSource field). A nil source
// removes the override.
func (m *musicState) setAreaTrackSource(src func() (int, bool)) {
	m.areaTrackSource = src
}

// areaTrackOverride asks the installed area-track source for
// this tick's override (PlayerSession.ts:393-395). With no
// source installed it reports no opinion. A panic from the
// source is recovered and reported as no opinion: TS routes
// the call through PluginManager.callArea, which logs a
// throw and returns null (PluginManager.ts:481-512), so a
// throwing getMusicTrack must never break the flush. (When
// the coordinator's closure routes through
// game.CallAreaMusicTrack with an isolating caller installed,
// the caller recovers first and this recover never fires;
// the double cover is deliberate — the source is an
// arbitrary closure, and the flush must not depend on which
// caller is installed.)
func (m *musicState) areaTrackOverride() (track int, ok bool) {
	if m.areaTrackSource == nil {
		return 0, false
	}
	defer func() {
		if recover() != nil {
			track, ok = 0, false
		}
	}()
	return m.areaTrackSource()
}

// SetMusicAreaTrackSource installs the closure updateMusic
// asks each tick for the area getMusicTrack override
// (PlayerSession.ts:393-395). The coordinator installs it
// when the session is attached to a player, with a closure
// over that player:
//
//	sess.SetMusicAreaTrackSource(func() (int, bool) {
//		area := player.Area()
//		if area == nil {
//			return 0, false
//		}
//		return game.CallAreaMusicTrack(area, player)
//	})
//
// The closure runs on the tick goroutine from updateMusic,
// so it must only read tick-goroutine state (the player's
// area, written by AreaManager.Process on the same
// goroutine). A nil source removes the override. The
// nil-music guard covers zero-value Sessions built without
// NewSession, as in SetMusicData.
func (s *Session) SetMusicAreaTrackSource(src func() (int, bool)) {
	if s.music == nil {
		s.music = newMusicState()
	}
	s.music.setAreaTrackSource(src)
}

// updateMusic runs one tick of the music block for the owner's tile
// and audio settings. It is called from FlushTick in the
// region_updates position. See musicState.update.
func (s *Session) updateMusic(tick int64, tile protocol.Tile, audio map[int]int) {
	if s.music == nil {
		s.music = newMusicState()
	}
	s.music.update(tick, tile, audio, s.SendPacket)
}

// CurrentMusicTrack returns the selected track last started,
// or -1 when none is playing: getCurrentMusicTrack
// (PlayerSession.ts:147-149), which returns lastMusicTrack
// — "what a jingle resumes". SendJingle reads it to send
// the current track with zero fades before the jingle
// (PacketSender.ts:285-291). Unlike the setters above, the
// getter does not create the state machine for a zero-value
// Session: a nil music state reads as no track (-1), which
// is what a fresh state machine holds. Tick-goroutine-only
// like the rest of the music state: SendJingle runs on the
// tick goroutine (game/content sends), never concurrently
// with updateMusic.
func (s *Session) CurrentMusicTrack() int {
	if s.music == nil {
		return -1
	}
	return s.music.lastMusicTrack
}
