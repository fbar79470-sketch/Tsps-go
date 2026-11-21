package music

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// fixtureAreas is a tiny hand-built modern music-areas file. Region
// 0 (x 0..63, y 0..63) lays out, from the south-west corner:
// tiles x=0..3 area 0, x=4..5 a gap, x=6..7 area 1, x=8 area 2
// (a run without "*count"), and nothing after that. Region 1
// (y 64..127) is all area 1. Region 2 (y 128..191) overflows the
// grid. Region 3 (y 192..255) has a malformed count that stops
// decoding. Plane 1 region 0 covers only two tiles with area 2,
// so the rest falls back to plane 0.
const fixtureAreas = `{"areas": [
  {"names": ["Zero"], "tracks": [11, 22]},
  {"names": ["One"], "tracks": [33]},
  {"names": ["Two"], "tracks": [44]}
], "tiles": {
  "0": {
    "0": "0*4,-*2,1*2,2",
    "1": "1*4096",
    "2": "0*5000",
    "3": "0*2,1*xx,2*2"
  },
  "1": {"0": "2*2"}
}}`

// fixtureClassic is a tiny hand-built classic music-areas file
// whose contents differ from the modern fixture at every tile,
// so a test can tell which map a lookup used.
const fixtureClassic = `{"areas": [
  {"names": ["Classic"], "tracks": [99]}
], "tiles": {"0": {"0": "0*4096"}}}`

func writeFixture(t *testing.T, modern, classic string) string {
	t.Helper()
	dir := t.TempDir()
	if modern != "" {
		if err := os.WriteFile(filepath.Join(dir, "music-areas.json"), []byte(modern), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if classic != "" {
		if err := os.WriteFile(filepath.Join(dir, "music-areas-classic.json"), []byte(classic), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestTracksAt(t *testing.T) {
	d := LoadAreas(writeFixture(t, fixtureAreas, fixtureClassic))
	cases := []struct {
		name    string
		x, y    int
		plane   int
		classic bool
		want    []int
	}{
		{"area zero start", 0, 0, 0, false, []int{11, 22}},
		{"area zero end", 3, 0, 0, false, []int{11, 22}},
		{"gap", 4, 0, 0, false, nil},
		{"gap end", 5, 0, 0, false, nil},
		{"area one", 6, 0, 0, false, []int{33}},
		{"run without count", 8, 0, 0, false, []int{44}},
		{"past the runs", 9, 0, 0, false, nil},
		{"next region row", 0, 64, 0, false, []int{33}},
		{"overflow clamped", 63, 191, 0, false, []int{11, 22}},
		{"plane one override", 0, 0, 1, false, []int{44}},
		{"plane fallback to zero", 2, 0, 1, false, []int{11, 22}},
		{"plane two falls back", 6, 0, 2, false, []int{33}},
		{"classic map", 0, 0, 0, true, []int{99}},
		{"classic plane fallback", 5, 5, 3, true, []int{99}},
		{"unmapped region", 500, 500, 0, false, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := d.TracksAt(tc.x, tc.y, tc.plane, tc.classic); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("TracksAt(%d, %d, %d, %v) = %v, want %v", tc.x, tc.y, tc.plane, tc.classic, got, tc.want)
			}
		})
	}
}

func TestAreaAtIdentity(t *testing.T) {
	d := LoadAreas(writeFixture(t, fixtureAreas, fixtureClassic))
	if _, id := d.AreaAt(0, 0, 0, false); id != 0 {
		t.Fatalf("AreaAt(0, 0) id = %d, want 0", id)
	}
	if _, id := d.AreaAt(3, 0, 0, false); id != 0 {
		t.Fatalf("AreaAt(3, 0) id = %d, want 0 (same area)", id)
	}
	if _, id := d.AreaAt(6, 0, 0, false); id != 1 {
		t.Fatalf("AreaAt(6, 0) id = %d, want 1", id)
	}
	if tracks, id := d.AreaAt(4, 0, 0, false); tracks != nil || id != -1 {
		t.Fatalf("AreaAt(gap) = %v, %d, want nil, -1", tracks, id)
	}
	// The plane fallback keeps the ground floor's area identity.
	if _, id := d.AreaAt(2, 0, 1, false); id != 0 {
		t.Fatalf("AreaAt(2, 0, plane 1) id = %d, want 0 (plane 0 fallback)", id)
	}
}

func TestMalformedRuns(t *testing.T) {
	d := LoadAreas(writeFixture(t, fixtureAreas, fixtureClassic))
	if got := d.TracksAt(0, 192, 0, false); !reflect.DeepEqual(got, []int{11, 22}) {
		t.Fatalf("TracksAt(0, 192) = %v, want [11 22]", got)
	}
	// The malformed "1*xx" run stops decoding: tiles after it,
	// including the "2*2" run, stay area-less.
	if got := d.TracksAt(2, 192, 0, false); got != nil {
		t.Fatalf("TracksAt(2, 192) = %v, want nil", got)
	}
}

func TestLoadAreasFailureIsEmptyNotFatal(t *testing.T) {
	// A missing directory: both maps empty, no panic.
	d := LoadAreas(filepath.Join(t.TempDir(), "nope"))
	if got := d.TracksAt(0, 0, 0, false); got != nil {
		t.Fatalf("missing dir TracksAt = %v, want nil", got)
	}
	if got := d.TracksAt(0, 0, 0, true); got != nil {
		t.Fatalf("missing dir classic TracksAt = %v, want nil", got)
	}

	// Malformed modern JSON empties only the modern map.
	d = LoadAreas(writeFixture(t, "{not json", fixtureClassic))
	if got := d.TracksAt(0, 0, 0, false); got != nil {
		t.Fatalf("malformed modern TracksAt = %v, want nil", got)
	}
	if got := d.TracksAt(0, 0, 0, true); !reflect.DeepEqual(got, []int{99}) {
		t.Fatalf("malformed modern classic TracksAt = %v, want [99]", got)
	}

	// An area without tracks fails the whole file, as TS
	// spreading undefined throws inside loadMusicAreas.
	d = LoadAreas(writeFixture(t, `{"areas": [{"names": ["X"]}], "tiles": {}}`, fixtureClassic))
	if got := d.TracksAt(0, 0, 0, false); got != nil {
		t.Fatalf("trackless area TracksAt = %v, want nil", got)
	}
}

func TestNilDataIsEmpty(t *testing.T) {
	var d *Data
	if got := d.TracksAt(3200, 3200, 0, false); got != nil {
		t.Fatalf("nil TracksAt = %v, want nil", got)
	}
	if tracks, id := d.AreaAt(3200, 3200, 0, false); tracks != nil || id != -1 {
		t.Fatalf("nil AreaAt = %v, %d, want nil, -1", tracks, id)
	}
	if length, ok := d.LengthTicks(147); length != 0 || ok {
		t.Fatalf("nil LengthTicks = %d, %v, want 0, false", length, ok)
	}
	d.SetLengths(nil) // must not panic
}

func TestSettingAndClassic(t *testing.T) {
	if got := Setting(nil, LoopBit, 1); got != 1 {
		t.Fatalf("Setting(nil, loop) = %d, want 1 (defaults)", got)
	}
	if Classic(nil) {
		t.Fatal("Classic(nil) = true, want false (defaults are modern)")
	}
	if got := Setting(map[int]int{SettingsVarp: 0}, LoopBit, 1); got != 0 {
		t.Fatalf("Setting(0, loop) = %d, want 0", got)
	}
	if got := Setting(map[int]int{ModeVarp: 1}, LoopBit, 1); got != 1 {
		t.Fatalf("Setting(no varp 19, loop) = %d, want 1 (defaults)", got)
	}
	if !Classic(map[int]int{SettingsVarp: 1 << AreaTypeShift}) {
		t.Fatal("Classic(area type 1) = false, want true")
	}
	if Classic(map[int]int{SettingsVarp: 2 << AreaTypeShift}) {
		t.Fatal("Classic(area type 2) = true, want false")
	}
	if got := Setting(map[int]int{SettingsVarp: 1<<AreaTypeShift | 1}, AreaTypeShift, 2); got != 1 {
		t.Fatalf("Setting(area type) = %d, want 1", got)
	}
	// The unsigned shift: -1 >>> 0 is all bits set.
	if got := Setting(map[int]int{SettingsVarp: -1}, LoopBit, 1); got != 1 {
		t.Fatalf("Setting(-1, loop) = %d, want 1", got)
	}
}

func TestPickAreaTrack(t *testing.T) {
	zero := func() float64 { return 0 }
	almostOne := func() float64 { return 0.99 }
	cases := []struct {
		name     string
		tracks   []int
		previous int
		random   func() float64
		replay   bool
		want     int
		wantOK   bool
	}{
		{"first track", []int{10, 20, 30}, -1, zero, true, 10, true},
		{"skips previous", []int{10, 20, 30}, 10, zero, true, 20, true},
		{"last of others", []int{10, 20, 30}, 10, almostOne, true, 30, true},
		{"only previous replays", []int{7}, 7, zero, true, 7, true},
		{"only previous no replay", []int{7}, 7, zero, false, 0, false},
		{"single fresh track", []int{7}, -1, zero, false, 7, true},
		{"no tracks", nil, -1, zero, true, 0, false},
		{"no tracks no replay", nil, -1, zero, false, 0, false},
		{"previous absent", []int{5, 6}, 99, zero, true, 5, true},
		{"draw wraps by modulo", []int{5, 6}, -1, func() float64 { return 1.5 }, true, 6, true},
		{"negative draw is no track", []int{5, 6}, -1, func() float64 { return -0.5 }, true, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := PickAreaTrack(tc.tracks, tc.previous, tc.random, tc.replay)
			if got != tc.want || ok != tc.wantOK {
				t.Fatalf("PickAreaTrack(%v, %d, replay=%v) = %d, %v, want %d, %v",
					tc.tracks, tc.previous, tc.replay, got, ok, tc.want, tc.wantOK)
			}
		})
	}
	if _, ok := PickAreaTrack([]int{1, 2}, -1, nil, true); !ok {
		t.Fatal("PickAreaTrack with nil random returned no track")
	}
}

// fakeRow is a hand-built DbTable row for SetLengths tests.
type fakeRow struct {
	id   int
	ints map[int]int
	strs map[int]string
}

func (r fakeRow) ID() int { return r.id }
func (r fakeRow) Int(column int, fallback ...int) int {
	if v, ok := r.ints[column]; ok {
		return v
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return 0
}
func (r fakeRow) String(column int) string { return r.strs[column] }
func (r fakeRow) Column(column int) []any  { return nil }

func TestSetLengths(t *testing.T) {
	d := &Data{}
	d.SetLengths([]Row{
		fakeRow{id: 1, ints: map[int]int{4: 5, 3: 100}, strs: map[int]string{1: "Alpha"}},
		fakeRow{id: 2, ints: map[int]int{4: 6, 3: 50}, strs: map[int]string{0: "Beta"}},
		fakeRow{id: 3, ints: map[int]int{4: 7, 3: 60}},                                    // no name: skipped
		fakeRow{id: 4, ints: map[int]int{3: 70}, strs: map[int]string{1: "NoId"}},         // no id: skipped
		fakeRow{id: 5, ints: map[int]int{4: 8, 3: 0}, strs: map[int]string{1: "ZeroLen"}}, // no length yet
		fakeRow{id: 6, ints: map[int]int{4: 8, 3: 70}, strs: map[int]string{1: "Later"}},  // first length wins
		fakeRow{id: 7, ints: map[int]int{4: 5, 3: 200}, strs: map[int]string{1: "Dup"}},   // duplicate id: ignored
		fakeRow{id: 8, ints: map[int]int{4: 9, 3: -5}, strs: map[int]string{1: "Neg"}},    // negative length: skipped
	})
	cases := []struct {
		track  int
		length int
		ok     bool
	}{
		{5, 100, true},
		{6, 50, true}, // name fell back to column 0
		{7, 0, false},
		{8, 70, true}, // the zero-length row did not claim the id
		{9, 0, false},
		{999, 0, false},
	}
	for _, tc := range cases {
		if length, ok := d.LengthTicks(tc.track); length != tc.length || ok != tc.ok {
			t.Fatalf("LengthTicks(%d) = %d, %v, want %d, %v", tc.track, length, ok, tc.length, tc.ok)
		}
	}

	// No rows at all: no lengths, the TS empty-cache behavior.
	empty := &Data{}
	empty.SetLengths(nil)
	if _, ok := empty.LengthTicks(5); ok {
		t.Fatal("empty SetLengths LengthTicks = true, want false")
	}
}

func TestConstants(t *testing.T) {
	if SilenceTrack != 147 || CurrentTrackVarp != 3883 || ModeVarp != 18 || SettingsVarp != 19 {
		t.Fatalf("constants drifted: silence=%d current=%d mode=%d settings=%d",
			SilenceTrack, CurrentTrackVarp, ModeVarp, SettingsVarp)
	}
	if AreaMode != 0 || ShuffleMode != 1 || SingleMode != 2 {
		t.Fatalf("modes drifted: %d %d %d", AreaMode, ShuffleMode, SingleMode)
	}
	if LoopBit != 0 || AreaTypeShift != 10 || DefaultSettings != 1 {
		t.Fatalf("setting constants drifted: loop=%d shift=%d default=%d", LoopBit, AreaTypeShift, DefaultSettings)
	}
	if want := 0x1 | (0x1 << 9) | (0x3 << 10) | (0x7ff << 14); SettingsMask != want {
		t.Fatalf("SettingsMask = %#x, want %#x", SettingsMask, want)
	}
}
