package config

import "testing"

func TestZoneBoundariesSeededTags(t *testing.T) {
	w := &World{}
	got := w.ZoneBoundaries()
	for _, tag := range []string{"duel", "pvp", "safe", "multi-combat", "all-buildings-safe"} {
		list, ok := got[tag]
		if !ok {
			t.Errorf("tag %q missing, want seeded", tag)
			continue
		}
		if len(list) != 0 {
			t.Errorf("tag %q = %v, want empty", tag, list)
		}
	}
	if _, ok := got["f2p"]; ok {
		t.Error("tag \"f2p\" present, want absent (f2p zones are excluded)")
	}
}

func TestZoneBoundariesBoundedZone(t *testing.T) {
	w := &World{Zones: []Zone{
		{MinX: 3000, MaxX: 3300, MinY: 3500, MaxY: 3700, Z: 1, Tags: []string{"pvp"}, Bounded: true},
	}}
	got := w.ZoneBoundaries()["pvp"]
	want := []ZoneBoundary{{MinX: 3000, MaxX: 3300, MinY: 3500, MaxY: 3700, Z: 1}}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("pvp = %v, want %v", got, want)
	}
}

// A zone without bounds expands to four full-map boundaries,
// one per plane, in plane order (WorldDefinition.ts:237-242).
func TestZoneBoundariesUnboundedZone(t *testing.T) {
	w := &World{Zones: []Zone{
		{Tags: []string{"pvp", "multi-combat"}},
	}}
	for _, tag := range []string{"pvp", "multi-combat"} {
		got := w.ZoneBoundaries()[tag]
		if len(got) != 4 {
			t.Fatalf("%s len = %d, want 4", tag, len(got))
		}
		for z := 0; z < 4; z++ {
			want := ZoneBoundary{MinX: 0, MaxX: 0x3fff, MinY: 0, MaxY: 0x3fff, Z: z}
			if got[z] != want {
				t.Errorf("%s[%d] = %v, want %v", tag, z, got[z], want)
			}
		}
	}
}

// The f2p exclusion is PER TAG (WorldDefinition.ts:279-282:
// the loop skips the tag "f2p" itself): a zone tagged only
// "f2p" contributes nothing anywhere, but a zone tagged
// ["f2p", "pvp"] DOES contribute its boundaries to "pvp".
// Either way no "f2p" key ever appears.
func TestZoneBoundariesF2PExclusion(t *testing.T) {
	w := &World{Zones: []Zone{
		{MinX: 0, MaxX: 100, MinY: 0, MaxY: 100, Z: 0, Tags: []string{"f2p"}, Bounded: true},
		{MinX: 200, MaxX: 300, MinY: 200, MaxY: 300, Z: 0, Tags: []string{"f2p", "pvp"}, Bounded: true},
	}}
	got := w.ZoneBoundaries()
	if _, ok := got["f2p"]; ok {
		t.Error("tag \"f2p\" present, want absent")
	}
	pvp := got["pvp"]
	if len(pvp) != 1 {
		t.Fatalf("pvp len = %d, want 1 (only the f2p+pvp zone)", len(pvp))
	}
	want := ZoneBoundary{MinX: 200, MaxX: 300, MinY: 200, MaxY: 300, Z: 0}
	if pvp[0] != want {
		t.Errorf("pvp[0] = %v, want %v", pvp[0], want)
	}
	// An unbounded f2p-only zone likewise contributes
	// nothing anywhere.
	w2 := &World{Zones: []Zone{{Tags: []string{"f2p"}}}}
	got2 := w2.ZoneBoundaries()
	if _, ok := got2["f2p"]; ok {
		t.Error("unbounded f2p: tag \"f2p\" present")
	}
	for tag, list := range got2 {
		if len(list) != 0 {
			t.Errorf("unbounded f2p: tag %q = %v, want empty", tag, list)
		}
	}
}

// Unknown tags gain entries (the `??=` at
// WorldDefinition.ts:282), and each tag's list preserves
// zone order.
func TestZoneBoundariesUnknownTagsAndOrder(t *testing.T) {
	w := &World{Zones: []Zone{
		{MinX: 0, MaxX: 10, MinY: 0, MaxY: 10, Z: 0, Tags: []string{"custom-tag"}, Bounded: true},
		{MinX: 20, MaxX: 30, MinY: 20, MaxY: 30, Z: 2, Tags: []string{"custom-tag"}, Bounded: true},
		{MinX: 40, MaxX: 50, MinY: 40, MaxY: 50, Z: 0, Tags: []string{"pvp"}, Bounded: true},
	}}
	got := w.ZoneBoundaries()
	custom := got["custom-tag"]
	if len(custom) != 2 {
		t.Fatalf("custom-tag len = %d, want 2", len(custom))
	}
	if custom[0].MinX != 0 || custom[1].MinX != 20 {
		t.Errorf("custom-tag order = %v, want zone order", custom)
	}
	if custom[1].Z != 2 {
		t.Errorf("custom-tag[1].Z = %d, want 2", custom[1].Z)
	}
}

// A zone carrying several non-f2p tags contributes its
// boundaries to each of them.
func TestZoneBoundariesMultipleTags(t *testing.T) {
	w := &World{Zones: []Zone{
		{MinX: 1, MaxX: 2, MinY: 3, MaxY: 4, Z: 3, Tags: []string{"pvp", "safe", "duel"}, Bounded: true},
	}}
	got := w.ZoneBoundaries()
	want := ZoneBoundary{MinX: 1, MaxX: 2, MinY: 3, MaxY: 4, Z: 3}
	for _, tag := range []string{"pvp", "safe", "duel"} {
		if len(got[tag]) != 1 || got[tag][0] != want {
			t.Errorf("%s = %v, want [%v]", tag, got[tag], want)
		}
	}
}

// The result is freshly built: mutating it must not affect
// later calls.
func TestZoneBoundariesFreshEachCall(t *testing.T) {
	w := &World{Zones: []Zone{
		{MinX: 0, MaxX: 10, MinY: 0, MaxY: 10, Z: 0, Tags: []string{"pvp"}, Bounded: true},
	}}
	first := w.ZoneBoundaries()
	first["pvp"][0].MinX = 999
	second := w.ZoneBoundaries()
	if second["pvp"][0].MinX != 0 {
		t.Errorf("second call MinX = %d, want 0 (mutation leaked)", second["pvp"][0].MinX)
	}
}
