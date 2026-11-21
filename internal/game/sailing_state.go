package game

import (
	"encoding/json"
	"math"
)

// Per-player boat state, ported from
// game/content/sailing/SailingState.ts. It PERSISTS via
// PlayerSave.sailing (PlayerSave.ts:185, 680, 810): entry loads
// it through normalizeSailingState and snapshots write it
// through normalizeSailingState, so the normalize rules below
// are the save shape. The JSON tags are the TS field names.
//
// NUMBER SHAPES. TS stores positions and hitpoints as JS
// numbers, and normalizeSailingState accepts any finite number
// for the finite() fields (positions, hitpoints, damage, mote
// counts) while requiring integers for names, part tiers, cargo,
// facilities, and tools. Go keeps the engine-read positions as
// ints (fine units and tiles are integral everywhere the engine
// uses them) and the never-engine-read hitpoints/damage/mote
// counts as float64, so a fractional saved value round-trips
// exactly. A fractional saved position truncates toward zero on
// normalize — no producer writes fractional positions (every
// placement comes from integral fine units), so the truncation
// is unreachable for real saves.

// BoatAt is a boat's place in fine units (1/128 tile) and its
// angle (SailingState.ts: BoatAt).
type BoatAt struct {
	FineX int `json:"fineX"`
	FineY int `json:"fineY"`
	Level int `json:"level"`
	Angle int `json:"angle"`
}

// BoatLocationKind is the kind of a BoatLocation: "docked",
// "at_sea", or "sunk".
type BoatLocationKind string

const (
	BoatLocationDocked BoatLocationKind = "docked"
	BoatLocationAtSea  BoatLocationKind = "at_sea"
	BoatLocationSunk   BoatLocationKind = "sunk"
)

// BoatLocation is where an owned boat is (SailingState.ts:
// BoatLocation). Exactly one shape is live, selected by Kind:
//   - docked: Dock holds the dock id; At holds the moored
//     position when the boat was left at a recorded spot.
//   - at_sea: FineX/FineY/Level/Angle hold the position;
//     HasDock + Dock hold the port it last docked at when
//     known.
//   - sunk: no other field is read.
type BoatLocation struct {
	Kind    BoatLocationKind
	Dock    string
	HasDock bool
	At      *BoatAt
	FineX   int
	FineY   int
	Level   int
	Angle   int
}

// DockedBoatLocation builds a docked location at dock, with no
// recorded position.
func DockedBoatLocation(dock string) BoatLocation {
	return BoatLocation{Kind: BoatLocationDocked, Dock: dock, HasDock: true}
}

// AtSeaBoatLocation builds an at-sea location, with the last
// port when dock is non-empty.
func AtSeaBoatLocation(fineX, fineY, level, angle int, dock string) BoatLocation {
	loc := BoatLocation{Kind: BoatLocationAtSea, FineX: fineX, FineY: fineY, Level: level, Angle: angle}
	if dock != "" {
		loc.Dock = dock
		loc.HasDock = true
	}
	return loc
}

// SunkBoatLocation is the lost location: a shipwright must
// recover the boat.
func SunkBoatLocation() BoatLocation { return BoatLocation{Kind: BoatLocationSunk} }

// MarshalJSON writes the TS union shape: docked carries dock and
// an optional at; at_sea carries the position and an optional
// dock; sunk carries only the kind.
func (l BoatLocation) MarshalJSON() ([]byte, error) {
	switch l.Kind {
	case BoatLocationDocked:
		type docked struct {
			Kind string  `json:"kind"`
			Dock string  `json:"dock"`
			At   *BoatAt `json:"at,omitempty"`
		}
		return json.Marshal(docked{Kind: string(l.Kind), Dock: l.Dock, At: l.At})
	case BoatLocationAtSea:
		type atSea struct {
			Kind  string `json:"kind"`
			FineX int    `json:"fineX"`
			FineY int    `json:"fineY"`
			Level int    `json:"level"`
			Angle int    `json:"angle"`
			Dock  string `json:"dock,omitempty"`
		}
		dock := ""
		if l.HasDock {
			dock = l.Dock
		}
		return json.Marshal(atSea{Kind: string(l.Kind), FineX: l.FineX, FineY: l.FineY, Level: l.Level, Angle: l.Angle, Dock: dock})
	default:
		type sunk struct {
			Kind string `json:"kind"`
		}
		return json.Marshal(sunk{Kind: string(BoatLocationSunk)})
	}
}

// CargoSlot is one slot of a boat's cargo hold.
type CargoSlot struct {
	ID     int `json:"id"`
	Amount int `json:"amount"`
}

// BoatName is a boat's name: three word numbers (1-based, 0 =
// none) into cache db rows 8545-8547.
type BoatName [3]int

// BoatParts are a boat's core parts, as tiers (0 = the base
// tier) into its type's part lists in the cache.
type BoatParts struct {
	Hull  int `json:"hull"`
	Keel  int `json:"keel"`
	Sails int `json:"sails"`
	Helm  int `json:"helm"`
	// Trim is a painted trim: its position in the type's trim
	// list. Nil means unset: the trim follows the hull.
	Trim *int `json:"trim,omitempty"`
	// Flag is the flag flown (1-3); Brazier records whether
	// the eternal brazier is built (1). Nil when none.
	Flag    *int `json:"flag,omitempty"`
	Brazier *int `json:"brazier,omitempty"`
}

// BaseParts returns the base-tier parts (SailingState.ts:
// baseParts).
func BaseParts() BoatParts {
	return BoatParts{Hull: 0, Keel: 0, Sails: 0, Helm: 0}
}

// OwnedBoat is one owned boat (SailingState.ts: OwnedBoat).
type OwnedBoat struct {
	Slot      int      `json:"slot"`
	Type      string   `json:"type"`
	Name      BoatName `json:"name"`
	Hitpoints float64  `json:"hitpoints"`
	// Damage is the damage taken since the boat was last
	// recovered: its hitpoints are its parts' maximum less
	// this.
	Damage float64 `json:"damage"`
	// WindCatcherOn marks a built wind catcher that is
	// switched on; WindMotes the motes it holds, and
	// WindMotesOwed how many pay full XP. Only true/positive
	// values survive normalize, so false/nil is absent.
	WindCatcherOn      bool     `json:"windCatcherOn,omitempty"`
	WindMotes          *float64 `json:"windMotes,omitempty"`
	WindMotesOwed      *float64 `json:"windMotesOwed,omitempty"`
	AnchorDown         bool     `json:"anchorDown,omitempty"`
	CrystalExtractorOn bool     `json:"crystalExtractorOn,omitempty"`
	CrystalMote        bool     `json:"crystalMote,omitempty"`
	Facilities         []int    `json:"facilities"`
	// HotspotData holds, per hotspot, what its facility holds
	// (the hotspot's extra_data varbit): a keg's drink. 0 =
	// nothing. Nil when no hotspot holds anything.
	HotspotData []int        `json:"hotspotData,omitempty"`
	Location    BoatLocation `json:"location"`
	// Cargo is the cargo hold, by slot; a nil entry is an
	// empty slot.
	Cargo []*CargoSlot `json:"cargo"`
	// Parts are the core parts built on this boat (upgraded
	// or downgraded at a shipyard).
	Parts BoatParts `json:"parts"`
}

// ReturnPoint is where Escape sends the player: the last
// gangplank, mooring point or buoy used.
type ReturnPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
	Z int `json:"z"`
}

// SailingState is the per-player boat state (SailingState.ts:
// SailingState).
type SailingState struct {
	Boats []OwnedBoat `json:"boats"`
	// ActiveBoatSlot is the boat the player is aboard, or
	// last set sail in. Nil is TS null.
	ActiveBoatSlot *int `json:"activeBoatSlot"`
	// ReturnPoint is where Escape sends the player. Nil is
	// TS null.
	ReturnPoint *ReturnPoint `json:"returnPoint"`
	// Tools are the tools-compartment slots holding their
	// tool; shared by all of the player's boats.
	Tools []int `json:"tools"`
	// MissingToolParts are parts of a stored multi-part tool
	// (diving gear) that are not in the compartment; absent
	// when none.
	MissingToolParts []int `json:"missingToolParts,omitempty"`
	// LastDock is the dock the player last docked at, and
	// LastStandardDock the last that was a port (not a
	// mooring point). Nil is TS null.
	LastDock         *string `json:"lastDock"`
	LastStandardDock *string `json:"lastStandardDock"`
	// LastMooringPoint is the last mooring point the player
	// docked at; absent until they dock at one.
	LastMooringPoint *string `json:"lastMooringPoint,omitempty"`
	// MooredDocks are the docks at which this player has
	// actually moored; absent on older saves.
	MooredDocks []string `json:"mooredDocks,omitempty"`
}

// EmptySailingState returns the empty state (SailingState.ts:
// emptySailingState).
func EmptySailingState() SailingState {
	return SailingState{
		Boats: []OwnedBoat{},
		Tools: []int{},
	}
}

// --- normalize (SailingState.ts: normalizeSailingState) ---
//
// The input is decoded JSON (map[string]any / []any / float64 /
// string / bool / nil), as encoding/json produces it. Anything
// malformed is dropped; missing state is empty.

// normalizeFinite reports whether v is a finite JSON number, as
// the TS finite() guard does.
func normalizeFinite(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, !math.IsNaN(n) && !math.IsInf(n, 0)
	case float32:
		f := float64(n)
		return f, !math.IsNaN(f) && !math.IsInf(f, 0)
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
	default:
		return 0, false
	}
}

// normalizeInteger reports whether v is an integer-valued JSON
// number, as Number.isInteger does.
func normalizeInteger(v any) (int, bool) {
	f, ok := normalizeFinite(v)
	if !ok || math.Trunc(f) != f {
		return 0, false
	}
	return int(f), true
}

func normalizeObject(raw any) (map[string]any, bool) {
	obj, ok := raw.(map[string]any)
	return obj, ok
}

func normalizeArray(raw any) ([]any, bool) {
	arr, ok := raw.([]any)
	return arr, ok
}

func normalizeAt(raw any) (BoatAt, bool) {
	obj, ok := normalizeObject(raw)
	if !ok {
		return BoatAt{}, false
	}
	fineX, okX := normalizeFinite(obj["fineX"])
	fineY, okY := normalizeFinite(obj["fineY"])
	angle, okA := normalizeFinite(obj["angle"])
	if !okX || !okY || !okA {
		return BoatAt{}, false
	}
	level := 0
	if lv, ok := normalizeFinite(obj["level"]); ok {
		level = int(lv)
	}
	return BoatAt{FineX: int(fineX), FineY: int(fineY), Level: level, Angle: int(angle)}, true
}

func normalizeLocation(raw any) BoatLocation {
	obj, ok := normalizeObject(raw)
	if !ok {
		return SunkBoatLocation()
	}
	if kind, _ := obj["kind"].(string); kind == "docked" {
		if dock, ok := obj["dock"].(string); ok {
			loc := DockedBoatLocation(dock)
			if at, ok := normalizeAt(obj["at"]); ok {
				loc.At = &at
			}
			return loc
		}
	}
	if kind, _ := obj["kind"].(string); kind == "at_sea" {
		if at, ok := normalizeAt(obj); ok {
			loc := AtSeaBoatLocation(at.FineX, at.FineY, at.Level, at.Angle, "")
			if dock, ok := obj["dock"].(string); ok {
				loc.Dock = dock
				loc.HasDock = true
			}
			return loc
		}
	}
	return SunkBoatLocation()
}

func normalizeName(raw any) BoatName {
	var name BoatName
	words, _ := normalizeArray(raw)
	for i := 0; i < 3; i++ {
		var word any
		if i < len(words) {
			word = words[i]
		}
		if n, ok := normalizeInteger(word); ok && n >= 0 && n <= 255 {
			name[i] = n
		}
	}
	return name
}

func normalizeParts(raw any) BoatParts {
	obj, _ := normalizeObject(raw)
	tier := func(value any) int {
		if n, ok := normalizeInteger(value); ok && n >= 0 && n <= 6 {
			return n
		}
		return 0
	}
	parts := BoatParts{
		Hull:  tier(obj["hull"]),
		Keel:  tier(obj["keel"]),
		Sails: tier(obj["sails"]),
		Helm:  tier(obj["helm"]),
	}
	cosmetic := func(value any, min, max int) (int, bool) {
		n, ok := normalizeInteger(value)
		return n, ok && n >= min && n <= max
	}
	// A trim below 7 is a hull tier's own "No trim": unset, so
	// it follows the hull.
	if n, ok := cosmetic(obj["trim"], 7, 255); ok {
		parts.Trim = &n
	}
	if n, ok := cosmetic(obj["flag"], 1, 255); ok {
		parts.Flag = &n
	}
	if n, ok := cosmetic(obj["brazier"], 1, 1); ok {
		parts.Brazier = &n
	}
	return parts
}

func normalizeCargo(raw any) []*CargoSlot {
	arr, ok := normalizeArray(raw)
	if !ok {
		return []*CargoSlot{}
	}
	cargo := make([]*CargoSlot, len(arr))
	for i, slotRaw := range arr {
		slot, ok := normalizeObject(slotRaw)
		if !ok {
			continue
		}
		id, okID := normalizeInteger(slot["id"])
		amount, okAmount := normalizeInteger(slot["amount"])
		if okID && id >= 0 && okAmount && amount > 0 {
			cargo[i] = &CargoSlot{ID: id, Amount: amount}
		}
	}
	return cargo
}

func normalizeHotspotData(raw any) []int {
	arr, ok := normalizeArray(raw)
	if !ok {
		return nil
	}
	data := make([]int, len(arr))
	anyPositive := false
	for i, value := range arr {
		if n, ok := normalizeInteger(value); ok && n > 0 {
			data[i] = n
			anyPositive = true
		}
	}
	if !anyPositive {
		return nil
	}
	return data
}

// NormalizeSailingState reads a saved sailing state, dropping
// anything malformed. Missing state is empty
// (SailingState.ts: normalizeSailingState). The input is decoded
// JSON; see ParseSailingStateJSON for the byte form.
func NormalizeSailingState(raw any) SailingState {
	obj, ok := normalizeObject(raw)
	if !ok {
		return EmptySailingState()
	}
	var boats []OwnedBoat
	seenSlots := make(map[int]bool)
	if rawBoats, ok := normalizeArray(obj["boats"]); ok {
		for _, boatRaw := range rawBoats {
			boat, ok := normalizeObject(boatRaw)
			if !ok {
				continue
			}
			slot, okSlot := normalizeInteger(boat["slot"])
			boatType, okType := boat["type"].(string)
			if !okSlot || !okType {
				continue
			}
			if seenSlots[slot] {
				continue
			}
			seenSlots[slot] = true
			owned := OwnedBoat{
				Slot:       slot,
				Type:       boatType,
				Name:       normalizeName(boat["name"]),
				Facilities: []int{},
				Location:   normalizeLocation(boat["location"]),
				Cargo:      normalizeCargo(boat["cargo"]),
				Parts:      normalizeParts(boat["parts"]),
			}
			if hp, ok := normalizeFinite(boat["hitpoints"]); ok {
				owned.Hitpoints = hp
			}
			if dmg, ok := normalizeFinite(boat["damage"]); ok && dmg > 0 {
				owned.Damage = dmg
			}
			if boat["windCatcherOn"] == true {
				owned.WindCatcherOn = true
			}
			if motes, ok := normalizeFinite(boat["windMotes"]); ok && motes > 0 {
				owned.WindMotes = &motes
			}
			if owed, ok := normalizeFinite(boat["windMotesOwed"]); ok && owed > 0 {
				owned.WindMotesOwed = &owed
			}
			if boat["anchorDown"] == true {
				owned.AnchorDown = true
			}
			if boat["crystalExtractorOn"] == true {
				owned.CrystalExtractorOn = true
			}
			if boat["crystalMote"] == true {
				owned.CrystalMote = true
			}
			if facilities, ok := normalizeArray(boat["facilities"]); ok {
				for _, f := range facilities {
					if n, ok := normalizeInteger(f); ok {
						owned.Facilities = append(owned.Facilities, n)
					}
				}
			}
			owned.HotspotData = normalizeHotspotData(boat["hotspotData"])
			boats = append(boats, owned)
		}
	}
	if boats == nil {
		boats = []OwnedBoat{}
	}
	state := SailingState{Boats: boats, Tools: []int{}}
	if slot, ok := normalizeInteger(obj["activeBoatSlot"]); ok && seenSlots[slot] {
		state.ActiveBoatSlot = &slot
	}
	if point, ok := normalizeObject(obj["returnPoint"]); ok {
		x, okX := normalizeFinite(point["x"])
		y, okY := normalizeFinite(point["y"])
		z, okZ := normalizeFinite(point["z"])
		if okX && okY && okZ {
			state.ReturnPoint = &ReturnPoint{X: int(x), Y: int(y), Z: int(z)}
		}
	}
	if tools, ok := normalizeArray(obj["tools"]); ok {
		seen := make(map[int]bool)
		for _, t := range tools {
			if n, ok := normalizeInteger(t); ok && !seen[n] {
				seen[n] = true
				state.Tools = append(state.Tools, n)
			}
		}
	}
	if parts, ok := normalizeArray(obj["missingToolParts"]); ok {
		seen := make(map[int]bool)
		for _, t := range parts {
			if n, ok := normalizeInteger(t); ok && !seen[n] {
				seen[n] = true
				state.MissingToolParts = append(state.MissingToolParts, n)
			}
		}
	}
	dockID := func(value any) *string {
		if s, ok := value.(string); ok {
			return &s
		}
		return nil
	}
	state.LastDock = dockID(obj["lastDock"])
	state.LastStandardDock = dockID(obj["lastStandardDock"])
	if s, ok := obj["lastMooringPoint"].(string); ok {
		state.LastMooringPoint = &s
	}
	if docks, ok := normalizeArray(obj["mooredDocks"]); ok {
		seen := make(map[string]bool)
		for _, d := range docks {
			if s, ok := d.(string); ok && len(s) > 0 && !seen[s] {
				seen[s] = true
				state.MooredDocks = append(state.MooredDocks, s)
			}
		}
	}
	return state
}

// ParseSailingStateJSON decodes saved sailing JSON and
// normalizes it. Empty, absent, or unparseable input is the
// empty state, as normalizeSailingState(undefined) is.
func ParseSailingStateJSON(data []byte) SailingState {
	if len(data) == 0 {
		return EmptySailingState()
	}
	var raw any
	if err := json.Unmarshal(data, &raw); err != nil {
		return EmptySailingState()
	}
	return NormalizeSailingState(raw)
}

// Normalized re-applies the normalize rules to live state, the
// Go form of fromPlayer's normalizeSailingState(player.
// getSailing()) (PlayerSave.ts:810): the state round-trips
// through its save JSON and back through NormalizeSailingState,
// so the snapshot writer sees exactly the normalized shape. A
// state that cannot marshal (unreachable: all fields are plain
// JSON values) yields the empty state.
func (s SailingState) Normalized() SailingState {
	data, err := json.Marshal(s)
	if err != nil {
		return EmptySailingState()
	}
	return ParseSailingStateJSON(data)
}

// MarshalSailingState marshals normalized live state for a
// snapshot. It never fails for a state built by this package.
func MarshalSailingState(s SailingState) ([]byte, error) {
	return json.Marshal(s.Normalized())
}
