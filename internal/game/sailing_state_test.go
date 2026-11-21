package game

import (
	"encoding/json"
	"reflect"
	"testing"
)

func parseState(t *testing.T, data string) SailingState {
	t.Helper()
	return ParseSailingStateJSON([]byte(data))
}

func TestParseSailingStateEmpty(t *testing.T) {
	for _, data := range []string{"", "null", "42", `"str"`, "[1,2]", "{invalid", "[]"} {
		state := parseState(t, data)
		if len(state.Boats) != 0 || state.ActiveBoatSlot != nil || state.ReturnPoint != nil || len(state.Tools) != 0 {
			t.Errorf("ParseSailingStateJSON(%q) = %+v, want empty state", data, state)
		}
	}
	empty := EmptySailingState()
	if len(empty.Boats) != 0 || len(empty.Tools) != 0 {
		t.Errorf("EmptySailingState = %+v, want empty boats/tools", empty)
	}
}

func TestNormalizeSailingStateBoats(t *testing.T) {
	state := parseState(t, `{"boats": [
		{"slot": 0, "type": "sloop", "name": [1, 2, 300], "hitpoints": 100.5, "damage": -5,
		 "windCatcherOn": true, "windMotes": 3, "anchorDown": "yes",
		 "facilities": [1, "x", 2.5, 3], "location": {"kind": "docked", "dock": "catherby"},
		 "cargo": [{"id": 1, "amount": 5}, {"id": -1, "amount": 5}, {"id": 2, "amount": 0}, null, "junk"],
		 "parts": {"hull": 2, "keel": 9, "sails": -1, "helm": 1, "trim": 5, "flag": 2, "brazier": 2}},
		{"slot": 0, "type": "dupe"},
		{"slot": "x", "type": "bad"},
		{"slot": 1},
		{"slot": 2, "type": "raft", "location": {"kind": "at_sea", "fineX": 100, "fineY": 200, "angle": 512, "dock": "ardougne"}}
	], "activeBoatSlot": 2, "tools": [1, 1, 2, "x"], "missingToolParts": [3, 3],
	"lastDock": "catherby", "lastStandardDock": 7, "lastMooringPoint": "island",
	"mooredDocks": ["catherby", "", "catherby", "ardougne"]}`)
	if len(state.Boats) != 2 {
		t.Fatalf("boats = %d, want 2 (dupe/bad/missing-type dropped)", len(state.Boats))
	}
	boat := state.Boats[0]
	if boat.Slot != 0 || boat.Type != "sloop" {
		t.Errorf("boat 0 = slot %d type %q", boat.Slot, boat.Type)
	}
	// Name words clamp: 300 is out of range, dropped to 0.
	if boat.Name != (BoatName{1, 2, 0}) {
		t.Errorf("name = %v, want [1 2 0]", boat.Name)
	}
	if boat.Hitpoints != 100.5 {
		t.Errorf("hitpoints = %v, want 100.5", boat.Hitpoints)
	}
	if boat.Damage != 0 {
		t.Errorf("damage = %v, want 0 (negative dropped)", boat.Damage)
	}
	if !boat.WindCatcherOn {
		t.Error("windCatcherOn must survive (=== true)")
	}
	if boat.WindMotes == nil || *boat.WindMotes != 3 {
		t.Errorf("windMotes = %v, want 3", boat.WindMotes)
	}
	if boat.AnchorDown {
		t.Error("anchorDown must not survive a non-true value")
	}
	if !reflect.DeepEqual(boat.Facilities, []int{1, 3}) {
		t.Errorf("facilities = %v, want [1 3]", boat.Facilities)
	}
	if boat.Location.Kind != BoatLocationDocked || boat.Location.Dock != "catherby" {
		t.Errorf("location = %+v, want docked at catherby", boat.Location)
	}
	// Cargo: only the valid slot survives; the rest are null.
	if len(boat.Cargo) != 5 {
		t.Fatalf("cargo len = %d, want 5", len(boat.Cargo))
	}
	if boat.Cargo[0] == nil || boat.Cargo[0].ID != 1 || boat.Cargo[0].Amount != 5 {
		t.Errorf("cargo[0] = %+v, want {1 5}", boat.Cargo[0])
	}
	for i := 1; i < 5; i++ {
		if boat.Cargo[i] != nil {
			t.Errorf("cargo[%d] = %+v, want nil", i, boat.Cargo[i])
		}
	}
	// Parts: keel 9 and sails -1 clamp to 0; trim 5 is a hull
	// tier's own trim (unset); brazier 2 is not exactly 1.
	if boat.Parts.Hull != 2 || boat.Parts.Keel != 0 || boat.Parts.Sails != 0 || boat.Parts.Helm != 1 {
		t.Errorf("parts = %+v, want hull 2 keel 0 sails 0 helm 1", boat.Parts)
	}
	if boat.Parts.Trim != nil {
		t.Errorf("trim = %v, want nil", boat.Parts.Trim)
	}
	if boat.Parts.Flag == nil || *boat.Parts.Flag != 2 {
		t.Errorf("flag = %v, want 2", boat.Parts.Flag)
	}
	if boat.Parts.Brazier != nil {
		t.Errorf("brazier = %v, want nil", boat.Parts.Brazier)
	}
	// At-sea boat: level defaults to 0, dock kept.
	sea := state.Boats[1]
	if sea.Location.Kind != BoatLocationAtSea || sea.Location.FineX != 100 || sea.Location.FineY != 200 ||
		sea.Location.Angle != 512 || sea.Location.Level != 0 || sea.Location.Dock != "ardougne" || !sea.Location.HasDock {
		t.Errorf("at-sea location = %+v", sea.Location)
	}
	if state.ActiveBoatSlot == nil || *state.ActiveBoatSlot != 2 {
		t.Errorf("activeBoatSlot = %v, want 2", state.ActiveBoatSlot)
	}
	if !reflect.DeepEqual(state.Tools, []int{1, 2}) {
		t.Errorf("tools = %v, want [1 2] deduped", state.Tools)
	}
	if !reflect.DeepEqual(state.MissingToolParts, []int{3}) {
		t.Errorf("missingToolParts = %v, want [3]", state.MissingToolParts)
	}
	if state.LastDock == nil || *state.LastDock != "catherby" {
		t.Errorf("lastDock = %v, want catherby", state.LastDock)
	}
	if state.LastStandardDock != nil {
		t.Errorf("lastStandardDock = %v, want nil (non-string dropped)", state.LastStandardDock)
	}
	if state.LastMooringPoint == nil || *state.LastMooringPoint != "island" {
		t.Errorf("lastMooringPoint = %v, want island", state.LastMooringPoint)
	}
	if !reflect.DeepEqual(state.MooredDocks, []string{"catherby", "ardougne"}) {
		t.Errorf("mooredDocks = %v, want [catherby ardougne]", state.MooredDocks)
	}
}

func TestNormalizeSailingStateLocations(t *testing.T) {
	cases := []struct {
		raw  string
		want BoatLocation
	}{
		{`{"kind": "docked", "dock": "x", "at": {"fineX": 1, "fineY": 2, "angle": 3}}`,
			BoatLocation{Kind: BoatLocationDocked, Dock: "x", HasDock: true, At: &BoatAt{FineX: 1, FineY: 2, Angle: 3}}},
		{`{"kind": "docked", "dock": 5}`, SunkBoatLocation()},
		{`{"kind": "docked"}`, SunkBoatLocation()},
		{`{"kind": "at_sea", "fineX": 1, "fineY": 2, "angle": 3, "level": 1}`,
			AtSeaBoatLocation(1, 2, 1, 3, "")},
		{`{"kind": "at_sea", "fineX": "x", "fineY": 2, "angle": 3}`, SunkBoatLocation()},
		{`{"kind": "sunk"}`, SunkBoatLocation()},
		{`{"kind": "bogus"}`, SunkBoatLocation()},
		{`null`, SunkBoatLocation()},
		{`42`, SunkBoatLocation()},
	}
	for _, c := range cases {
		state := parseState(t, `{"boats": [{"slot": 0, "type": "sloop", "location": `+c.raw+`}]}`)
		if len(state.Boats) != 1 {
			t.Fatalf("raw %s: boats = %d, want 1", c.raw, len(state.Boats))
		}
		if !reflect.DeepEqual(state.Boats[0].Location, c.want) {
			t.Errorf("raw %s: location = %+v, want %+v", c.raw, state.Boats[0].Location, c.want)
		}
	}
}

func TestNormalizeSailingStateHotspotData(t *testing.T) {
	// All-zero hotspot data is omitted; any positive value
	// keeps the array, with non-positive entries zeroed.
	state := parseState(t, `{"boats": [{"slot": 0, "type": "sloop", "hotspotData": [0, -1, "x"]}]}`)
	if state.Boats[0].HotspotData != nil {
		t.Errorf("hotspotData = %v, want nil", state.Boats[0].HotspotData)
	}
	state = parseState(t, `{"boats": [{"slot": 0, "type": "sloop", "hotspotData": [0, 5, -1]}]}`)
	if !reflect.DeepEqual(state.Boats[0].HotspotData, []int{0, 5, 0}) {
		t.Errorf("hotspotData = %v, want [0 5 0]", state.Boats[0].HotspotData)
	}
}

func TestNormalizeSailingStateActiveSlotMustExist(t *testing.T) {
	state := parseState(t, `{"boats": [{"slot": 0, "type": "sloop"}], "activeBoatSlot": 9}`)
	if state.ActiveBoatSlot != nil {
		t.Errorf("activeBoatSlot = %v, want nil (no boat in slot 9)", state.ActiveBoatSlot)
	}
}

func TestBoatLocationMarshalJSON(t *testing.T) {
	docked := DockedBoatLocation("catherby")
	data, err := json.Marshal(docked)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"kind":"docked","dock":"catherby"}` {
		t.Errorf("docked = %s", data)
	}
	at := BoatAt{FineX: 1, FineY: 2, Level: 0, Angle: 3}
	docked.At = &at
	data, _ = json.Marshal(docked)
	if string(data) != `{"kind":"docked","dock":"catherby","at":{"fineX":1,"fineY":2,"level":0,"angle":3}}` {
		t.Errorf("docked+at = %s", data)
	}
	data, _ = json.Marshal(AtSeaBoatLocation(1, 2, 0, 3, "ardougne"))
	if string(data) != `{"kind":"at_sea","fineX":1,"fineY":2,"level":0,"angle":3,"dock":"ardougne"}` {
		t.Errorf("at_sea = %s", data)
	}
	data, _ = json.Marshal(AtSeaBoatLocation(1, 2, 0, 3, ""))
	if string(data) != `{"kind":"at_sea","fineX":1,"fineY":2,"level":0,"angle":3}` {
		t.Errorf("at_sea without dock = %s", data)
	}
	data, _ = json.Marshal(SunkBoatLocation())
	if string(data) != `{"kind":"sunk"}` {
		t.Errorf("sunk = %s", data)
	}
}

func TestSailingStateRoundTrip(t *testing.T) {
	raw := `{"boats":[{"slot":0,"type":"sloop","name":[1,0,0],"hitpoints":50,"damage":5,"facilities":[7],"location":{"kind":"docked","dock":"catherby"},"cargo":[null,{"id":3,"amount":2}],"parts":{"hull":1,"keel":0,"sails":0,"helm":0}}],"activeBoatSlot":0,"returnPoint":{"x":1,"y":2,"z":0},"tools":[4],"lastDock":"catherby","lastStandardDock":"catherby"}`
	state := parseState(t, raw)
	normalized := state.Normalized()
	if !reflect.DeepEqual(state, normalized) {
		t.Errorf("round trip changed the state:\n%+v\n%+v", state, normalized)
	}
	data, err := MarshalSailingState(state)
	if err != nil {
		t.Fatal(err)
	}
	again := ParseSailingStateJSON(data)
	if !reflect.DeepEqual(state, again) {
		t.Errorf("marshal round trip changed the state:\n%+v\n%+v", state, again)
	}
}

func TestEmptySailingStateJSONShape(t *testing.T) {
	data, err := json.Marshal(EmptySailingState())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"boats":[],"activeBoatSlot":null,"returnPoint":null,"tools":[],"lastDock":null,"lastStandardDock":null}`
	if string(data) != want {
		t.Errorf("empty state = %s, want %s", data, want)
	}
}
