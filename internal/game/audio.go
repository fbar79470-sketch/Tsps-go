package game

import "tsps-go/internal/music"

// Player audio settings: the port of the audio-settings surface in
// Player.ts — the audioSettings field (line 233), DEFAULT_AUDIO_SETTINGS
// (lines 70-81), getAudioSettings/setAudioSettings/setAudioSetting
// (lines 894-922). The settings are the varps the client transmits
// for volumes and music options (mode, loop, area type, mute-restore
// levels); the session music state machine reads them through the
// per-tick snapshot, and nothing in Go mutates them after entry.
//
// COORDINATOR WIRING (this file cannot declare it: the Player struct
// lives in player.go, which this milestone must not edit): Player
// needs an `audioSettings audioSettings` field, initialized by
// NewPlayer with newAudioSettings(), plus two thin methods:
//
//	func (p *Player) GetAudioSettings() map[int]int { return p.audioSettings.get() }
//	func (p *Player) SetAudioSettings(settings map[int]int) { p.audioSettings.set(settings) }
//
// The wiring calls SetAudioSettings with the save's AudioSettings
// at entry (applyToPlayer, PlayerSave.ts:650), and the flush
// adapter snapshots GetAudioSettings into the per-tick view. The
// save merge PRESERVES audio settings verbatim (fromPlayer,
// PlayerSave.ts:758, copies getAudioSettings): no persist change
// is needed or wanted — Go never mutates the settings after
// entry, so the preserved save value and the live value agree.
//
// NOT STARTED: applyAudioSettingFromClient (Player.ts:925-935,
// the live varp_transmit application at NetworkBuilder.ts:1167
// plus its "audio:setting-changed" plugin event). setSetting
// below ports only the validation/storage half TS shares with
// setAudioSettings; no caller applies live client changes yet.
//
// TICK-GOROUTINE DISCIPLINE: like the rest of Player, the
// settings are set once at entry (before the player is queued)
// and read on the tick goroutine. get returns a copy, so
// snapshot readers can never mutate the stored map.

// defaultAudioSettings is DEFAULT_AUDIO_SETTINGS
// (Player.ts:70-81): mode 0 (Area), the music settings varp at
// its default (loop on), the four volumes at 100, and the two
// packed mute-restore varps at 0. The order is the TS
// declaration order (ascending varp id, as Object.keys returns
// integer-like keys); set() iterates it so a save missing a
// varp keeps that varp's default.
var defaultAudioSettings = [...]struct {
	varpID int
	value  int
}{
	{18, 0},
	{music.SettingsVarp, music.DefaultSettings},
	{168, 100},
	{169, 100},
	{872, 100},
	{3796, 100},
	{3109, 0},
	{3797, 0},
}

// packedAudioVarps is PACKED_AUDIO_VARPS (Player.ts:82): the
// mute-restore levels, packed so they are not clamped to 100.
func isPackedAudioVarp(varpID int) bool {
	return varpID == 3109 || varpID == 3797
}

// audioSettings holds one player's audio settings: the
// audioSettings field (Player.ts:233). The zero value holds no
// map and reads as unset (get returns nil); NewPlayer must
// initialize it with newAudioSettings so a live player always
// carries the defaults TS field initializers produce.
type audioSettings struct {
	values map[int]int
}

// newAudioSettings returns the settings as TS constructs them:
// a copy of DEFAULT_AUDIO_SETTINGS.
func newAudioSettings() audioSettings {
	a := audioSettings{values: make(map[int]int, len(defaultAudioSettings))}
	for _, d := range defaultAudioSettings {
		a.values[d.varpID] = d.value
	}
	return a
}

// set ports setAudioSettings (Player.ts:898-904): reset to the
// defaults, then apply each default varp's saved value through
// setSetting. A nil settings map (or a save without audio
// settings) leaves the defaults in place, as TS returns early
// for a missing or non-object argument. Keys outside the
// defaults are ignored: TS iterates only the default varps.
func (a *audioSettings) set(settings map[int]int) {
	*a = newAudioSettings()
	for _, d := range defaultAudioSettings {
		if value, ok := settings[d.varpID]; ok {
			a.setSetting(d.varpID, value)
		}
	}
}

// setSetting ports setAudioSetting (Player.ts:906-922): validate
// and store one audio varp. Unknown varps are rejected (TS
// checks membership in DEFAULT_AUDIO_SETTINGS); the music
// settings varp keeps only its client-owned bits
// (Music.SETTINGS_MASK); the packed varps keep the whole 32-bit
// value (live sends 3109 with bits 30 and 31 set, so negative —
// clamping it to 0 would wipe the saved levels); every other
// varp clamps to 0..2 for the mode varp and 0..100 for volumes.
// It reports whether the value was stored.
func (a *audioSettings) setSetting(varpID, value int) bool {
	known := false
	for _, d := range defaultAudioSettings {
		if d.varpID == varpID {
			known = true
			break
		}
	}
	if !known {
		return false
	}
	if a.values == nil {
		a.values = make(map[int]int, len(defaultAudioSettings))
	}
	switch {
	case varpID == music.SettingsVarp:
		a.values[varpID] = value & music.SettingsMask
	case isPackedAudioVarp(varpID):
		// `Math.trunc(value) | 0`: the whole 32-bit value,
		// as the client holds it.
		a.values[varpID] = int(int32(value))
	default:
		maximum := 100
		if varpID == music.ModeVarp {
			maximum = 2
		}
		clamped := value
		if clamped < 0 {
			clamped = 0
		}
		if clamped > maximum {
			clamped = maximum
		}
		a.values[varpID] = clamped
	}
	return true
}

// get returns a copy of the settings map, or nil when no
// settings were ever stored (the zero value). TS
// getAudioSettings returns the live record; the copy keeps
// snapshot readers from mutating player state.
func (a *audioSettings) get() map[int]int {
	if a.values == nil {
		return nil
	}
	out := make(map[int]int, len(a.values))
	for varpID, value := range a.values {
		out[varpID] = value
	}
	return out
}

// GetAudioSettings returns a copy of the player's audio settings
// (getAudioSettings, Player.ts:894-896), or nil for a zero-value
// Player. The music state machine reads the snapshot through the
// per-tick flush view.
func (p *Player) GetAudioSettings() map[int]int { return p.audioSettings.get() }

// SetAudioSettings installs the player's audio settings
// (setAudioSettings, Player.ts:898-904), resetting to the defaults
// and then applying the known varps from settings. The wiring
// calls it once at entry with the save's audioSettings
// (applyToPlayer, PlayerSave.ts:650).
func (p *Player) SetAudioSettings(settings map[int]int) { p.audioSettings.set(settings) }
