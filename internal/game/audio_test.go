package game

import (
	"reflect"
	"testing"

	"tsps-go/internal/music"
)

// The audio-settings tests drive the audioSettings seam type
// directly: the Player field and Get/SetAudioSettings methods
// are coordinator wiring (see audio.go).

var audioDefaults = map[int]int{
	18: 0, 19: 1, 168: 100, 169: 100, 872: 100, 3796: 100, 3109: 0, 3797: 0,
}

func TestAudioSettingsDefaults(t *testing.T) {
	a := newAudioSettings()
	if got := a.get(); !reflect.DeepEqual(got, audioDefaults) {
		t.Fatalf("newAudioSettings = %v, want %v", got, audioDefaults)
	}
	// The zero value reads as unset.
	var zero audioSettings
	if got := zero.get(); got != nil {
		t.Fatalf("zero audioSettings get = %v, want nil", got)
	}
}

func TestAudioSettingsSet(t *testing.T) {
	a := newAudioSettings()
	a.set(map[int]int{
		18:   1, // shuffle mode
		19:   1 | (1 << music.AreaTypeShift),
		168:  0,   // music muted
		3109: 42,  // packed: kept whole
		9999: 123, // unknown varp: ignored
	})
	want := map[int]int{
		18: 1, 19: 1 | (1 << music.AreaTypeShift), 168: 0, 169: 100,
		872: 100, 3796: 100, 3109: 42, 3797: 0,
	}
	if got := a.get(); !reflect.DeepEqual(got, want) {
		t.Fatalf("set = %v, want %v", got, want)
	}

	// Setting again resets to the defaults first: values from
	// the previous set do not leak through.
	a.set(nil)
	if got := a.get(); !reflect.DeepEqual(got, audioDefaults) {
		t.Fatalf("set(nil) = %v, want defaults %v", got, audioDefaults)
	}

	// A save missing a varp keeps that varp's default.
	a.set(map[int]int{18: 2})
	if got := a.get(); got[18] != 2 || got[168] != 100 || got[19] != 1 {
		t.Fatalf("partial set = %v, want mode 2 with other defaults", got)
	}
}

func TestAudioSettingsSetSetting(t *testing.T) {
	a := newAudioSettings()

	// The mode varp clamps to 0..2.
	if !a.setSetting(18, 5) || a.get()[18] != 2 {
		t.Fatalf("setSetting(18, 5) = %v, want stored as 2", a.get()[18])
	}
	if !a.setSetting(18, -3) || a.get()[18] != 0 {
		t.Fatalf("setSetting(18, -3) = %v, want stored as 0", a.get()[18])
	}
	// Volumes clamp to 0..100.
	if !a.setSetting(168, 150) || a.get()[168] != 100 {
		t.Fatalf("setSetting(168, 150) = %v, want 100", a.get()[168])
	}
	if !a.setSetting(169, -1) || a.get()[169] != 0 {
		t.Fatalf("setSetting(169, -1) = %v, want 0", a.get()[169])
	}
	// The music settings varp keeps only the client-owned bits.
	if !a.setSetting(19, -1) || a.get()[19] != music.SettingsMask {
		t.Fatalf("setSetting(19, -1) = %#x, want mask %#x", a.get()[19], music.SettingsMask)
	}
	// Packed varps keep the whole 32-bit value, including the
	// sign bit live sends.
	packed := (1 << 31) | (1 << 30) | 7
	if !a.setSetting(3109, packed) || a.get()[3109] != int(int32(packed)) {
		t.Fatalf("setSetting(3109, %#x) = %d, want %d", packed, a.get()[3109], int(int32(packed)))
	}
	// Unknown varps are rejected and change nothing.
	before := a.get()
	if a.setSetting(9999, 1) {
		t.Fatal("setSetting(9999, 1) = true, want false")
	}
	if got := a.get(); !reflect.DeepEqual(got, before) {
		t.Fatalf("rejected set changed settings to %v", got)
	}
}

func TestAudioSettingsGetIsCopy(t *testing.T) {
	a := newAudioSettings()
	got := a.get()
	got[18] = 2
	got[9999] = 1
	if again := a.get(); again[18] != 0 || len(again) != len(audioDefaults) {
		t.Fatalf("mutating the copy changed the stored settings: %v", again)
	}
}
