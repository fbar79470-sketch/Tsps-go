package session

import "testing"

// Hitsplat type ids from PlayerSession.ts hitView /
// hitsplatType: BLUE 12/13, RED 16/17, YELLOW 22/23, GREEN
// 65/66 (mine/other), max-hit RED 43, max-hit YELLOW 46.

func TestHitsplatTypeTable(t *testing.T) {
	cases := []struct {
		mask  int
		own   bool
		want  int
		label string
	}{
		{hitsplatMaskBlue, true, 12, "blue mine"}, {hitsplatMaskBlue, false, 13, "blue other"},
		{hitsplatMaskRed, true, 16, "red mine"}, {hitsplatMaskRed, false, 17, "red other"},
		{hitsplatMaskYellow, true, 22, "yellow mine"}, {hitsplatMaskYellow, false, 23, "yellow other"},
		{hitsplatMaskGreen, true, 65, "green mine"}, {hitsplatMaskGreen, false, 66, "green other"},
	}
	for _, tc := range cases {
		hit := HitsplatSnapshot{Mask: tc.mask, Damage: 5, OnMe: tc.own}
		if got := hitsplatTypeFor(hit); got != tc.want {
			t.Errorf("%s: type = %d, want %d", tc.label, got, tc.want)
		}
	}
	// An unknown mask falls back to the RED pair.
	if got := hitsplatTypeFor(HitsplatSnapshot{Mask: 99, Damage: 1, OnMe: true}); got != 16 {
		t.Errorf("unknown mask mine = %d, want 16", got)
	}
}

func TestHitsplatMaxHitVariants(t *testing.T) {
	// RED max hit from the viewer, over the threshold: 43.
	hit := HitsplatSnapshot{Mask: hitsplatMaskRed, Damage: 30, SourceIsViewer: true, IsMaxHit: true, MaxHitThreshold: 20}
	if got := hitsplatTypeFor(hit); got != 43 {
		t.Errorf("max-hit RED = %d, want 43", got)
	}
	// YELLOW max hit: 46.
	hit.Mask = hitsplatMaskYellow
	if got := hitsplatTypeFor(hit); got != 46 {
		t.Errorf("max-hit YELLOW = %d, want 46", got)
	}
	// OnMe suppresses the max-hit variant.
	hit.Mask = hitsplatMaskRed
	hit.OnMe = true
	if got := hitsplatTypeFor(hit); got != 16 {
		t.Errorf("max-hit onMe = %d, want 16", got)
	}
	// Below the threshold: ordinary (mine, since the viewer dealt it).
	hit.OnMe = false
	hit.Damage = 10
	if got := hitsplatTypeFor(hit); got != 16 {
		t.Errorf("below-threshold max = %d, want 16", got)
	}
	// Source is not the viewer: ordinary other.
	hit.Damage = 30
	hit.SourceIsViewer = false
	if got := hitsplatTypeFor(hit); got != 17 {
		t.Errorf("other-source max = %d, want 17", got)
	}
	// BLUE max hits never use the variant (mine, viewer dealt it).
	hit.Mask = hitsplatMaskBlue
	hit.SourceIsViewer = true
	if got := hitsplatTypeFor(hit); got != 12 {
		t.Errorf("blue max = %d, want 12", got)
	}
}

func TestHitsplatOverridePrecedence(t *testing.T) {
	// A splat override beats the max-hit variant and the mask table.
	hit := HitsplatSnapshot{Mask: hitsplatMaskRed, Damage: 30, SourceIsViewer: true, IsMaxHit: true, MaxHitThreshold: 20, SplatMine: 99, SplatOthers: 98}
	hit.OnMe = true
	if got := hitsplatTypeFor(hit); got != 99 {
		t.Errorf("mine override = %d, want 99", got)
	}
	// Others: neither on the viewer nor dealt by the viewer.
	hit.OnMe = false
	hit.SourceIsViewer = false
	if got := hitsplatTypeFor(hit); got != 98 {
		t.Errorf("others override = %d, want 98", got)
	}
	// A zero override (unset) falls through to the mask table.
	hit.SplatMine, hit.SplatOthers = 0, 0
	if got := hitsplatTypeFor(hit); got != 17 {
		t.Errorf("unset override = %d, want 17", got)
	}
}

func TestHitsplatViewsMapping(t *testing.T) {
	if got := hitsplatViews(nil); got != nil {
		t.Errorf("nil hits -> %v, want nil", got)
	}
	views := hitsplatViews([]HitsplatSnapshot{
		{Mask: hitsplatMaskRed, Damage: 7, OnMe: true},
		{Mask: hitsplatMaskBlue, Damage: 0, OnMe: false},
	})
	if len(views) != 2 {
		t.Fatalf("views = %d, want 2", len(views))
	}
	if views[0].Type != 16 || views[0].Damage != 7 || views[0].Delay != 0 {
		t.Errorf("view[0] = %+v, want {16 7 0}", views[0])
	}
	if views[1].Type != 13 || views[1].Damage != 0 {
		t.Errorf("view[1] = %+v, want {13 0 0}", views[1])
	}
}

func TestHealthViewMapping(t *testing.T) {
	if got := healthView(nil); got != nil {
		t.Errorf("nil health -> %v, want nil", got)
	}
	v := healthView(&HealthSnapshot{Current: 42, Max: 99})
	if v.Current != 42 || v.Max != 99 || v.Bar != nil {
		t.Errorf("health view = %+v, want {42 99 nil}", v)
	}
}

func TestCreateViewsHitMapping(t *testing.T) {
	s := NewSession(newFakeChannel())
	pv := s.createPlayerView(SyncPlayerSnapshot{
		Hits:   []HitsplatSnapshot{{Mask: hitsplatMaskRed, Damage: 5, OnMe: true}},
		Health: &HealthSnapshot{Current: 90, Max: 99},
	}, false)
	if len(pv.Hits) != 1 || pv.Hits[0].Type != 16 || pv.Hits[0].Damage != 5 {
		t.Errorf("player view hits = %+v, want one {16 5}", pv.Hits)
	}
	if pv.Health == nil || pv.Health.Current != 90 || pv.Health.Max != 99 {
		t.Errorf("player view health = %+v, want {90 99}", pv.Health)
	}
	// No hits: both stay nil (no HIT mask bit).
	pv = s.createPlayerView(SyncPlayerSnapshot{}, false)
	if pv.Hits != nil || pv.Health != nil {
		t.Errorf("empty player view hits/health = %v/%v, want nil/nil", pv.Hits, pv.Health)
	}
	nv := createNpcView(SyncNpcSnapshot{
		Hits:   []HitsplatSnapshot{{Mask: hitsplatMaskYellow, Damage: 3}},
		Health: &HealthSnapshot{Current: 4, Max: 10},
	})
	if len(nv.Hits) != 1 || nv.Hits[0].Type != 23 || nv.Hits[0].Damage != 3 {
		t.Errorf("NPC view hits = %+v, want one {23 3}", nv.Hits)
	}
	if nv.Health == nil || nv.Health.Current != 4 {
		t.Errorf("NPC view health = %+v, want {4 10}", nv.Health)
	}
	nv = createNpcView(SyncNpcSnapshot{})
	if nv.Hits != nil || nv.Health != nil {
		t.Errorf("empty NPC view hits/health = %v/%v, want nil/nil", nv.Hits, nv.Health)
	}
}
