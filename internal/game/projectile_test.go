package game

import (
	"testing"
)

// projectileFakeMobile is a controllable ProjectileMobile.
type projectileFakeMobile struct {
	loc    Location
	area   *Area
	size   int
	index  int
	player bool
}

func (m *projectileFakeMobile) Location() Location { return m.loc }
func (m *projectileFakeMobile) Area() *Area        { return m.area }
func (m *projectileFakeMobile) Size() int          { return m.size }
func (m *projectileFakeMobile) GetIndex() int      { return m.index }
func (m *projectileFakeMobile) IsPlayer() bool     { return m.player }

// projectileFakeSession records projectile sends through
// the ProjectileSender extension.
type projectileFakeSession struct {
	*fakeSession
	sends []projectileSendCall
}

type projectileSendCall struct {
	start, end                             Location
	offset, speed, id                      int
	startH, endH                           float64
	lockon, delay, angle, progress, source int
}

func newProjectileFakeSession() *projectileFakeSession {
	return &projectileFakeSession{fakeSession: newFakeSession()}
}

func (f *projectileFakeSession) SendProjectile(start, end Location, offset, speed, id int, startH, endH float64, lockon, delay, angle, progress, source int) {
	f.sends = append(f.sends, projectileSendCall{start, end, offset, speed, id, startH, endH, lockon, delay, angle, progress, source})
}

func projectileTestPlayer(name string, loc Location) (*Player, *projectileFakeSession) {
	p := NewPlayer(name, loc)
	sess := newProjectileFakeSession()
	p.Session = sess
	return p, sess
}

func TestResolveLockonTargetIndex(t *testing.T) {
	origin := Location{X: 3200, Y: 3200}
	if p := NewProjectile(origin, origin, nil, 1, 0, 0, 0, 0, nil); p.LockonTargetIndex() != 0 {
		t.Errorf("nil lockon index = %d, want 0", p.LockonTargetIndex())
	}
	player := &projectileFakeMobile{loc: origin, size: 1, index: 5, player: true}
	if p := NewProjectile(origin, origin, player, 1, 0, 0, 0, 0, nil); p.LockonTargetIndex() != -6 {
		t.Errorf("player index 5 = %d, want -6", p.LockonTargetIndex())
	}
	npc := &projectileFakeMobile{loc: origin, size: 1, index: 5}
	if p := NewProjectile(origin, origin, npc, 1, 0, 0, 0, 0, nil); p.LockonTargetIndex() != 6 {
		t.Errorf("npc index 5 = %d, want 6", p.LockonTargetIndex())
	}
	neg := &projectileFakeMobile{loc: origin, size: 1, index: -1}
	if p := NewProjectile(origin, origin, neg, 1, 0, 0, 0, 0, nil); p.LockonTargetIndex() != 0 {
		t.Errorf("negative index = %d, want 0", p.LockonTargetIndex())
	}
}

func TestCentreOf(t *testing.T) {
	loc := Location{X: 100, Y: 100, Z: 1}
	cases := []struct {
		size int
		want Location
	}{
		{0, loc},
		{1, loc},
		{2, loc},
		{3, Location{X: 101, Y: 101, Z: 1}},
		{5, Location{X: 102, Y: 102, Z: 1}},
	}
	for _, tc := range cases {
		m := &projectileFakeMobile{loc: loc, size: tc.size}
		if got := CentreOf(m); got != tc.want {
			t.Errorf("CentreOf size %d = %v, want %v", tc.size, got, tc.want)
		}
	}
}

func TestFlightDistance(t *testing.T) {
	start := Location{X: 0, Y: 0}
	if got := FlightDistance(start, Location{X: 10, Y: 0}, nil); got != 10 {
		t.Errorf("size-1 distance = %d, want 10", got)
	}
	// Size-3 victim centred at (10,0): footprint x 9..11,
	// nearest tile 9 away.
	victim := &projectileFakeMobile{loc: Location{X: 9, Y: 0}, size: 3}
	if got := FlightDistance(start, Location{X: 10, Y: 0}, victim); got != 9 {
		t.Errorf("size-3 distance = %d, want 9", got)
	}
	// Start inside the footprint: 0.
	if got := FlightDistance(Location{X: 10, Y: 0}, Location{X: 10, Y: 0}, victim); got != 0 {
		t.Errorf("inside distance = %d, want 0", got)
	}
	// Chebyshev: diagonal 5/8 -> 8.
	if got := FlightDistance(start, Location{X: 5, Y: 8}, nil); got != 8 {
		t.Errorf("diagonal distance = %d, want 8", got)
	}
	// Floor delta ignored.
	if got := FlightDistance(start, Location{X: 10, Y: 0, Z: 2}, nil); got != 10 {
		t.Errorf("cross-floor distance = %d, want 10", got)
	}
}

func TestArrivalHelpers(t *testing.T) {
	from := Location{X: 0, Y: 0}
	to := Location{X: 10, Y: 0}
	if got := ArrivalCycles(from, to, 40, 10); got != 140 {
		t.Errorf("ArrivalCycles = %d, want 140", got)
	}
	if got := ArrivalTicks(from, to, 40, 10); got != 5 {
		t.Errorf("ArrivalTicks = %d, want 5 (ceil 140/30)", got)
	}
	// Mobile endpoints centre first: a size-3 victim at
	// (9,0) centres to (10,0) and flies 9 tiles.
	victim := &projectileFakeMobile{loc: Location{X: 9, Y: 0}, size: 3}
	if got := ArrivalCycles(from, victim, 40, 10); got != 130 {
		t.Errorf("ArrivalCycles to mobile = %d, want 130", got)
	}
	if got := TicksToLand(0); got != 1 {
		t.Errorf("TicksToLand 0 = %d, want 1", got)
	}
	if got := TicksToLand(30); got != 1 {
		t.Errorf("TicksToLand 30 = %d, want 1", got)
	}
	if got := TicksToLand(31); got != 2 {
		t.Errorf("TicksToLand 31 = %d, want 2", got)
	}
	if got := TicksToLand(90); got != 3 {
		t.Errorf("TicksToLand 90 = %d, want 3", got)
	}
}

func TestProjectileTiming(t *testing.T) {
	// Legacy magic shape normalizes.
	p := NewProjectile(Location{X: 0, Y: 0}, Location{X: 10, Y: 0}, nil, 1, 0, 20, 43, 31, nil)
	timing := p.GetTiming()
	if !timing.LegacyMagic || timing.StartCycleOffset != 51 || timing.EndCycleOffset != 146 {
		t.Errorf("legacy timing = %+v, want {51 146 true}", timing)
	}
	// Any other shape passes delay/speed through.
	p = NewProjectile(Location{X: 0, Y: 0}, Location{X: 10, Y: 0}, nil, 1, 5, 60, 10, 20, nil)
	timing = p.GetTiming()
	if timing.LegacyMagic || timing.StartCycleOffset != 5 || timing.EndCycleOffset != 60 {
		t.Errorf("plain timing = %+v, want {5 60 false}", timing)
	}
	// End delay adds to the end offset only.
	p.WithEndDelay(7)
	if timing = p.GetTiming(); timing.StartCycleOffset != 5 || timing.EndCycleOffset != 67 {
		t.Errorf("end-delay timing = %+v, want {5 67 false}", timing)
	}
	// Defaults: angle 16, progress 64.
	if p.Angle() != 16 || p.Progress() != 64 {
		t.Errorf("defaults angle/progress = %d/%d, want 16/64", p.Angle(), p.Progress())
	}
}

func TestProjectileBuilders(t *testing.T) {
	origin := Location{X: 3200, Y: 3200}
	p := NewProjectile(origin, origin, nil, 1, 0, 0, 0, 0, nil)
	player := &projectileFakeMobile{loc: origin, size: 1, index: 4, player: true}
	if p.WithSource(player).SourceIndex() != 0 {
		t.Errorf("player source index = %d, want 0", p.SourceIndex())
	}
	npc := &projectileFakeMobile{loc: origin, size: 1, index: 4}
	if p.WithSource(npc).SourceIndex() != 5 {
		t.Errorf("npc source index = %d, want 5", p.SourceIndex())
	}
	if p.WithSource(nil).SourceIndex() != 0 {
		t.Errorf("nil source index = %d, want 0", p.SourceIndex())
	}
	if p.WithAngle(300).Angle() != 255 || p.WithAngle(-5).Angle() != 0 {
		t.Errorf("angle clamps = %d, want 255 then 0", p.Angle())
	}
	if p.WithProgress(300).Progress() != 255 || p.WithProgress(-1).Progress() != 0 {
		t.Errorf("progress clamps = %d", p.Progress())
	}
	p.WithEndDelay(-3)
	if p.GetTiming().EndCycleOffset != 0 {
		t.Errorf("negative end delay leaked: %+v", p.GetTiming())
	}
}

func TestCreateProjectile(t *testing.T) {
	area := NewArea("a", nil)
	source := &projectileFakeMobile{loc: Location{X: 100, Y: 100}, area: area, size: 1, index: 3, player: true}
	victim := &projectileFakeMobile{loc: Location{X: 110, Y: 100}, size: 3, index: 7}
	p := CreateProjectile(source, victim, 42, 0, 20, 43, 31)
	if p.Start() != (Location{X: 100, Y: 100}) {
		t.Errorf("start = %v, want source centre", p.Start())
	}
	if p.End() != (Location{X: 111, Y: 101}) {
		t.Errorf("end = %v, want victim centre (111,101)", p.End())
	}
	if p.LockonTargetIndex() != 8 {
		t.Errorf("lockon = %d, want 8", p.LockonTargetIndex())
	}
	if p.SourceIndex() != 0 {
		t.Errorf("player source index = %d, want 0", p.SourceIndex())
	}
	if p.ProjectileID() != 42 {
		t.Errorf("id = %d, want 42", p.ProjectileID())
	}
	// NPC source names itself.
	npcSource := &projectileFakeMobile{loc: Location{X: 100, Y: 100}, size: 1, index: 2}
	p = CreateProjectile(npcSource, victim, 42, 0, 20, 43, 31)
	if p.SourceIndex() != 3 {
		t.Errorf("npc source index = %d, want 3", p.SourceIndex())
	}
}

func TestSendProjectileGates(t *testing.T) {
	origin := Location{X: 3200, Y: 3200}
	target := &projectileFakeMobile{loc: Location{X: 3205, Y: 3200}, size: 1, index: 9}
	near, nearSess := projectileTestPlayer("near", Location{X: 3202, Y: 3200})
	farStart, farStartSess := projectileTestPlayer("farStart", Location{X: 3230, Y: 3200})
	// Sees the start (within 15) but not the lock-on tile:
	// start at 3200, lock-on at 3205; viewer at 3188 is 12
	// from the start and 17 from the lock-on.
	farTarget, farTargetSess := projectileTestPlayer("farTarget", Location{X: 3188, Y: 3200})
	otherLevel, otherSess := projectileTestPlayer("up", Location{X: 3202, Y: 3200, Z: 1})
	bot, botSess := projectileTestPlayer("bot", Location{X: 3202, Y: 3200})
	bot.IsBot = true
	closed, closedSess := projectileTestPlayer("closed", Location{X: 3202, Y: 3200})
	closedSess.open = false
	otherArea, otherAreaSess := projectileTestPlayer("otherArea", Location{X: 3202, Y: 3200})
	otherArea.SetArea(NewArea("b", nil))
	noSender, _ := projectileTestPlayer("noSender", Location{X: 3202, Y: 3200})
	noSender.Session = newFakeSession() // no ProjectileSender extension
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player {
		return []*Player{near, farStart, farTarget, otherLevel, bot, closed, otherArea, noSender}
	})
	defer SetNetworkPlayerSource(old)

	p := NewProjectile(origin, Location{X: 3205, Y: 3200}, target, 42, 10, 80, 43, 31, nil)
	p.SendProjectile()
	if len(nearSess.sends) != 1 {
		t.Fatalf("near sends = %d, want 1", len(nearSess.sends))
	}
	want := projectileSendCall{
		start: origin, end: Location{X: 3205, Y: 3200},
		offset: 0, speed: 80, id: 42, startH: 43, endH: 31,
		lockon: 10, delay: 10, angle: 16, progress: 64, source: 0,
	}
	if nearSess.sends[0] != want {
		t.Errorf("near send = %+v, want %+v", nearSess.sends[0], want)
	}
	for name, sess := range map[string]*projectileFakeSession{
		"farStart": farStartSess, "farTarget": farTargetSess, "otherLevel": otherSess,
		"bot": botSess, "closed": closedSess, "otherArea": otherAreaSess,
	} {
		if len(sess.sends) != 0 {
			t.Errorf("%s sends = %d, want 0", name, len(sess.sends))
		}
	}
}

func TestSendProjectileWideViewDistance(t *testing.T) {
	viewer, sess := projectileTestPlayer("viewer", Location{X: 3220, Y: 3200})
	wide := 30
	viewer.SetNpcViewDistance(&wide)
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{viewer} })
	defer SetNetworkPlayerSource(old)
	// Start 20 away: invisible at 15, visible at 30.
	p := NewProjectile(Location{X: 3200, Y: 3200}, Location{X: 3200, Y: 3200}, nil, 1, 0, 50, 10, 10, nil)
	p.SendProjectile()
	if len(sess.sends) != 1 {
		t.Errorf("wide-view sends = %d, want 1", len(sess.sends))
	}
}

func TestSendProjectileUntargetedAndUnwired(t *testing.T) {
	viewer, sess := projectileTestPlayer("viewer", Location{X: 3202, Y: 3200})
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{viewer} })
	defer SetNetworkPlayerSource(old)
	// No lock-on: only the start check applies.
	p := NewProjectile(Location{X: 3200, Y: 3200}, Location{X: 3300, Y: 3300}, nil, 1, 0, 50, 10, 10, nil)
	p.SendProjectile()
	if len(sess.sends) != 1 {
		t.Errorf("untargeted sends = %d, want 1", len(sess.sends))
	}
	// No source installed: nothing sends, no panic.
	SetNetworkPlayerSource(nil)
	p.SendProjectile()
	if len(sess.sends) != 1 {
		t.Errorf("unwired sends leaked: %d", len(sess.sends))
	}
}

func TestSendProjectileLegacyTimingFlowsThrough(t *testing.T) {
	viewer, sess := projectileTestPlayer("viewer", Location{X: 3202, Y: 3200})
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{viewer} })
	defer SetNetworkPlayerSource(old)
	p := NewProjectile(Location{X: 3200, Y: 3200}, Location{X: 3210, Y: 3200}, nil, 1, 0, 20, 43, 31, nil)
	p.SendProjectile()
	if len(sess.sends) != 1 {
		t.Fatalf("sends = %d, want 1", len(sess.sends))
	}
	// Legacy: launch 51, land 46 + 10*10 = 146.
	if sess.sends[0].delay != 51 || sess.sends[0].speed != 146 {
		t.Errorf("legacy send delay/speed = %d/%d, want 51/146", sess.sends[0].delay, sess.sends[0].speed)
	}
}
