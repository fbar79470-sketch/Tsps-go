package main

import (
	"testing"

	"tsps-go/internal/config"
	"tsps-go/internal/game"
)

func TestZoneBoundarySourceAdaptsConfigView(t *testing.T) {
	def := &config.World{Zones: []config.Zone{
		{MinX: 3000, MaxX: 3100, MinY: 3500, MaxY: 3600, Z: 0, Tags: []string{"pvp"}, Bounded: true},
		{MinX: 3000, MaxX: 3050, MinY: 3500, MaxY: 3550, Z: 0, Tags: []string{"multi-combat"}, Bounded: true},
		{Tags: []string{"f2p"}, Bounded: false},
	}}
	src := newZoneBoundarySource(def)
	pvp := src.ZoneBoundaries("pvp")
	if len(pvp) != 1 {
		t.Fatalf("pvp boundaries = %d, want 1", len(pvp))
	}
	if !pvp[0].Inside(game.Location{X: 3050, Y: 3550, Z: 0}) {
		t.Fatal("pvp boundary must contain (3050, 3550, 0)")
	}
	if pvp[0].Inside(game.Location{X: 3050, Y: 3550, Z: 1}) {
		t.Fatal("pvp boundary must not match another plane")
	}
	if got := src.ZoneBoundaries("f2p"); len(got) != 0 {
		t.Fatalf("f2p boundaries = %d, want 0 (excluded from the zone view)", len(got))
	}
	if got := src.ZoneBoundaries("nope"); len(got) != 0 {
		t.Fatalf("unknown tag boundaries = %d, want 0", len(got))
	}
	// A nil world yields the empty-world reading.
	empty := newZoneBoundarySource(nil)
	if got := empty.ZoneBoundaries("pvp"); len(got) != 0 {
		t.Fatalf("nil-world pvp boundaries = %d, want 0", len(got))
	}
}

func TestNewAreaManagerAndWilderness(t *testing.T) {
	def := &config.World{Zones: []config.Zone{
		{MinX: 3000, MaxX: 3100, MinY: 3500, MaxY: 3600, Z: 0, Tags: []string{"pvp", "multi-combat"}, Bounded: true},
	}}
	manager, wilderness := newAreaManagerAndWilderness(def, nil)
	if manager == nil || wilderness == nil {
		t.Fatal("construction must return a manager and a Wilderness")
	}
	// The InMulti fallback is wired: an area-less player on a
	// multi-combat tile reports multi through the manager.
	p := game.NewPlayer("alice", game.Location{X: 3050, Y: 3550, Z: 0})
	if !manager.InMulti(p) {
		t.Fatal("area-less player on a multi-combat tile must report multi")
	}
	if !wilderness.IsInLocation(p.Location()) {
		t.Fatal("pvp tile must read as Wilderness proper")
	}
}
