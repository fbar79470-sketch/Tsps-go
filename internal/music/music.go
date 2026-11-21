package music

import (
	"encoding/json"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

// Music constants (Music.ts). The session state machine and the
// player audio settings read these; the values are the TS
// `public static readonly` fields, quoted here with their TS
// names.
const (
	// SilenceTrack is SILENCE_TRACK: the silent track, sent when
	// an area track finishes.
	SilenceTrack = 147
	// CurrentTrackVarp is CURRENT_TRACK_VARP: the native varp
	// holding the playing track's DB row, -1 between tracks.
	CurrentTrackVarp = 3883
	// ModeVarp is MODE_VARP (musicplay): 0 Area, 1 Shuffle,
	// 2 Single.
	ModeVarp = 18
	// AreaMode, ShuffleMode, SingleMode are AREA_MODE,
	// SHUFFLE_MODE, SINGLE_MODE.
	AreaMode    = 0
	ShuffleMode = 1
	SingleMode  = 2
	// SettingsVarp is SETTINGS_VARP (musicloop): the music
	// settings the client sets and transmits, as varbits of
	// this varp.
	SettingsVarp = 19
	// LoopBit is LOOP_BIT (music_enableloop): when no other
	// track can play as one ends, it restarts. On by default.
	LoopBit = 0
	// AreaTypeShift is AREA_TYPE_SHIFT (music_area_mode, two
	// bits): 0 Modern areas, 1 Classic.
	AreaTypeShift = 10
	// SettingsMask is SETTINGS_MASK: the client-owned varbits
	// of varp 19 (the loop, unlock message, area type,
	// playlist, text shadow, hidden tracks, login mode,
	// default track, playlist update and selection mode). The
	// varp's other bits are the server's.
	SettingsMask = 0x1 | (0x1 << 9) | (0x3 << 10) | (0x7ff << 14)
	// DefaultSettings is DEFAULT_SETTINGS: the loop bit on,
	// everything else off.
	DefaultSettings = 1 << LoopBit
)

// noArea is NO_AREA (Music.ts): a tile with no music area.
const noArea = -1

// gridTiles is the tile count of one 64x64 region grid.
const gridTiles = 64 * 64

// areasMap is one MusicAreas value (Music.ts): each area's track
// list, and per plane and 64x64 region the RLE run string of the
// area of every tile. Track lists are shared, never mutated:
// TS freezes them, and TracksAt hands out the stored slice.
type areasMap struct {
	areas [][]int
	tiles map[int]map[int]string
}

// gridKey identifies one decoded region grid: the map (modern or
// classic), the plane, and the region id. TS keys its
// DECODED_GRIDS cache per map by (plane << 16) | region; the
// struct key says the same thing without packing.
type gridKey struct {
	classic bool
	plane   int
	region  int
}

// Data is the loaded music state: both area maps plus the track
// lengths from DbTable 44. LoadAreas fills the maps; SetLengths
// fills the lengths. A nil *Data behaves as the empty music
// state (see the package doc).
//
// Thread-safety: LoadAreas and SetLengths run at boot, before
// the Data is shared. After that, TracksAt/AreaAt/LengthTicks
// are safe for concurrent use: the decoded-grid cache is
// mutex-guarded and the lengths map is read-only.
type Data struct {
	modern  areasMap
	classic areasMap

	mu     sync.Mutex
	grids  map[gridKey][]int16
	length map[int]int
}

// LoadAreas loads the modern and classic music-area files from
// dir, porting the module-level MODERN_AREAS/CLASSIC_AREAS
// initialization through loadMusicAreas (Music.ts). A file that
// cannot be read or parsed warns and loads as empty — never
// fatal, exactly as TS, which catches every load error, warns
// (`[music] failed to load <file>`), and returns empty areas.
// The returned Data is never nil.
func LoadAreas(dir string) *Data {
	d := &Data{grids: make(map[gridKey][]int16)}
	d.modern = loadAreasFile(filepath.Join(dir, "music-areas.json"), "music-areas.json")
	d.classic = loadAreasFile(filepath.Join(dir, "music-areas-classic.json"), "music-areas-classic.json")
	return d
}

// areasFile is the JSON shape loadMusicAreas reads: areas with
// track lists, and tiles keyed by plane then region. Only
// `tracks` is read per area, as in TS; the names/wiki fields
// the generator also writes are ignored. Tracks is a pointer so
// a missing (or null) tracks field fails the file, matching TS,
// where spreading an undefined tracks list throws inside the
// try and empties the whole map.
type areasFile struct {
	Areas []struct {
		Tracks *[]int `json:"tracks"`
	} `json:"areas"`
	Tiles map[string]map[string]string `json:"tiles"`
}

func loadAreasFile(path, name string) areasMap {
	empty := areasMap{tiles: make(map[int]map[int]string)}
	raw, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[music] failed to load %s: %v", name, err)
		return empty
	}
	var data areasFile
	if err := json.Unmarshal(raw, &data); err != nil {
		log.Printf("[music] failed to load %s: %v", name, err)
		return empty
	}
	out := areasMap{
		areas: make([][]int, 0, len(data.Areas)),
		tiles: make(map[int]map[int]string),
	}
	for _, area := range data.Areas {
		if area.Tracks == nil {
			log.Printf("[music] failed to load %s: area without tracks", name)
			return empty
		}
		out.areas = append(out.areas, *area.Tracks)
	}
	for planeKey, regions := range data.Tiles {
		plane, err := strconv.Atoi(planeKey)
		if err != nil {
			continue // TS: Number(key) is NaN, a map key no lookup ever matches.
		}
		planeTiles := make(map[int]string, len(regions))
		for regionKey, runs := range regions {
			region, err := strconv.Atoi(regionKey)
			if err != nil {
				continue
			}
			planeTiles[region] = runs
		}
		out.tiles[plane] = planeTiles
	}
	return out
}

// decodeGrid decodes one region's RLE run string into the area
// of each tile, porting regionGrid's decode loop (Music.ts): the
// grid starts all NO_AREA, and each "id*count" run fills that
// many tiles row by row from the region's south-west corner,
// "-" filling nothing. A run without "*count" covers one tile.
// Fills past the end of the grid are clamped, as a TypedArray
// fill clamps them.
//
// Malformed-run behavior mirrors the JS where it is cheap to do
// so: an unparseable count stops decoding (TS gets NaN, which
// poisons the tile cursor so later fills are no-ops), and an
// unparseable id fills 0 (an Int16Array converts NaN to 0).
// Negative counts can only move the cursor backwards in TS
// (TypedArray fill counts a negative start from the end); Go
// skips those fills instead. The generated files contain none
// of these cases.
func decodeGrid(runs string) []int16 {
	grid := make([]int16, gridTiles)
	for i := range grid {
		grid[i] = noArea
	}
	tile := 0
	for _, run := range strings.Split(runs, ",") {
		id, count, hasCount := strings.Cut(run, "*")
		length := 1
		if hasCount {
			n, err := strconv.Atoi(count)
			if err != nil {
				break
			}
			length = n
		}
		if id != "-" {
			area, err := strconv.Atoi(id)
			if err != nil {
				area = 0
			}
			if tile >= 0 && tile < gridTiles {
				end := tile + length
				if end > gridTiles {
					end = gridTiles
				}
				for i := tile; i < end; i++ {
					grid[i] = int16(area)
				}
			}
		}
		tile += length
	}
	return grid
}

// grid returns the decoded grid for a region, cached like
// DECODED_GRIDS (Music.ts). A nil grid is a cached miss: no
// runs string exists for the region (or it is empty, which TS
// treats as missing via its `if (runs)` guard).
func (d *Data) grid(classic bool, plane, region int) []int16 {
	key := gridKey{classic: classic, plane: plane, region: region}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.grids == nil {
		d.grids = make(map[gridKey][]int16)
	}
	if g, ok := d.grids[key]; ok {
		return g
	}
	m := &d.modern
	if classic {
		m = &d.classic
	}
	var g []int16
	if runs, ok := m.tiles[plane][region]; ok && runs != "" {
		g = decodeGrid(runs)
	}
	d.grids[key] = g
	return g
}

// AreaAt returns the tracks of the music area at a tile plus the
// area's index in its map, or (nil, -1) when the tile has no
// area. It ports tracksAt (Music.ts): the region id is
// ((x >> 6) << 8) | (y >> 6) and the tile index is
// ((y & 63) << 6) | (x & 63); a floor with no area of its own
// plays the ground floor's (the plane fallback to 0). The
// region computation uses int32 arithmetic to mirror the JS
// 32-bit bitwise ops exactly.
//
// The index is the area's identity: the session state machine
// compares it (with the classic flag) to detect area changes,
// which TS does by comparing track-list references
// (PlayerSession.ts:401). Two areas with identical track lists
// are still different areas. The returned slice is the stored
// area list: callers must not mutate it.
func (d *Data) AreaAt(x, y, plane int, classic bool) ([]int, int) {
	if d == nil {
		return nil, noArea
	}
	region := int((int32(x>>6) << 8) | int32(y>>6))
	tile := ((y & 63) << 6) | (x & 63)
	area := noArea
	if g := d.grid(classic, plane, region); g != nil {
		area = int(g[tile])
	}
	if area == noArea && plane != 0 {
		if g := d.grid(classic, 0, region); g != nil {
			area = int(g[tile])
		}
	}
	if area == noArea {
		return nil, noArea
	}
	m := &d.modern
	if classic {
		m = &d.classic
	}
	if area < 0 || area >= len(m.areas) {
		// TS: map.areas[area] ?? NO_TRACKS — an out-of-range
		// area id reads as no tracks.
		return nil, noArea
	}
	return m.areas[area], area
}

// TracksAt returns the tracks of the music area at a tile, the
// same list for every tile of the area, or nil when the tile
// has no area. It ports tracksAt (Music.ts); see AreaAt for the
// lookup rules. The returned slice is the stored area list:
// callers must not mutate it.
func (d *Data) TracksAt(x, y, plane int, classic bool) []int {
	tracks, _ := d.AreaAt(x, y, plane, classic)
	return tracks
}

// Setting reads a bit field of the player's music settings
// (varp 19) from their saved audio settings, porting
// Music.setting (Music.ts): the varp value, or DEFAULT_SETTINGS
// when the settings carry no varp 19 entry, shifted right
// unsigned by bit and masked to bits bits. A nil settings map
// reads as the defaults, as TS reads undefined.
func Setting(settings map[int]int, bit, bits int) int {
	value, ok := settings[SettingsVarp]
	if !ok {
		value = DefaultSettings
	}
	return int(uint32(value)>>uint(bit)) & ((1 << bits) - 1)
}

// Classic reports whether the player uses the Classic music
// areas, porting Music.classic (Music.ts): the two-bit area
// type field equals 1.
func Classic(settings map[int]int) bool {
	return Setting(settings, AreaTypeShift, 2) == 1
}

// PickAreaTrack picks a random track of the area, porting
// pickAreaTrack (Music.ts): when one has just finished, the
// next is another of the area's tracks; when there is no other,
// it plays again only with replay (the loop setting). It
// returns false when no track can play (no tracks, or only the
// previous one without replay). random supplies the [0, 1)
// draw — TS defaults to Math.random; a nil random uses the
// math/rand global, auto-seeded since Go 1.20. The index
// computation mirrors TS exactly
// (choices[floor(random() * length) % length]).
func PickAreaTrack(tracks []int, previous int, random func() float64, replay bool) (int, bool) {
	var others []int
	for _, track := range tracks {
		if track != previous {
			others = append(others, track)
		}
	}
	choices := others
	if len(others) == 0 && replay {
		choices = tracks
	}
	if len(choices) == 0 {
		return 0, false
	}
	if random == nil {
		random = rand.Float64
	}
	i := int(math.Floor(random()*float64(len(choices)))) % len(choices)
	if i < 0 {
		// A negative draw: TS indexes choices with a
		// negative number and gets undefined — no track.
		return 0, false
	}
	return choices[i], true
}

// Row is the DbTable row surface cachedTracks reads (Music.ts).
// It deliberately does not import internal/cache: the boot
// wiring passes rows that satisfy it. *cache.DbRowType satisfies
// this interface directly (ID/Int/String/Column), so the wiring
// needs no adapter — only the slice conversion from
// GetDbTableRows(44).
//
// Length extraction consults Int and String only. ID and Column
// mirror the TS DbRow surface: cachedTracks uses the row id for
// its parent-row lookup and column 5 for the unlock slot, both
// of which feed the plugin-owned unlock system (NOT STARTED),
// not lengths.
type Row interface {
	ID() int
	Int(column int, fallback ...int) int
	String(column int) string
	Column(column int) []any
}

// SetLengths builds the track-length table from the DbTable 44
// rows, porting the cachedTracks filter plus the lengthsById
// build (Music.ts): a row contributes only when its id
// (column 4, default -1) is >= 0 and it has a name (column 1,
// falling back to column 0), and only lengths (column 3) > 0
// are kept — the first length per id wins. Rows after the first
// length for an id, and rows without a usable length, do not
// overwrite it, exactly as the TS `lengthTicks !== undefined &&
// !lengthsById.has(id)` guard.
//
// SetLengths replaces any previous table. It runs once at boot,
// before the Data is shared; LengthTicks is safe for concurrent
// use after that. With no rows (no cache), every LengthTicks
// lookup misses and area tracks never end on their own — the TS
// no-lengths behavior.
func (d *Data) SetLengths(rows []Row) {
	if d == nil {
		return
	}
	lengths := make(map[int]int, len(rows))
	for _, row := range rows {
		if row == nil {
			continue
		}
		id := row.Int(4, -1)
		name := row.String(1)
		if name == "" {
			name = row.String(0)
		}
		if id < 0 || name == "" {
			continue
		}
		length := row.Int(3, -1)
		if length <= 0 {
			continue
		}
		if _, ok := lengths[id]; !ok {
			lengths[id] = length
		}
	}
	d.length = lengths
}

// LengthTicks reports how many ticks a track plays for, or
// false when the cache has no length for it, porting
// lengthTicks (Music.ts). A nil Data reports no length.
func (d *Data) LengthTicks(trackID int) (int, bool) {
	if d == nil {
		return 0, false
	}
	length, ok := d.length[trackID]
	return length, ok
}
