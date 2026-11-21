package game

import (
	"encoding/json"
	"testing"

	"tsps-go/internal/definition"
)

// fakeAggressionCombat records engagement decisions for
// runAggression tests. inCombat maps a Mobile pointer to its
// in-combat answer; canAttack answers every CanAttack call.
type fakeAggressionCombat struct {
	inCombat  map[*Mobile]bool
	canAttack bool
	attacks   [][2]any // [npc, player] pairs, in order
}

func (f *fakeAggressionCombat) InCombat(m *Mobile) bool { return f.inCombat[m] }
func (f *fakeAggressionCombat) CanAttack(npc *NPC, target *Player) bool {
	return f.canAttack
}
func (f *fakeAggressionCombat) Attack(npc *NPC, target *Player) {
	f.attacks = append(f.attacks, [2]any{npc, target})
}

func (f *fakeAggressionCombat) attacked(npc *NPC) bool {
	for _, pair := range f.attacks {
		if pair[0] == npc {
			return true
		}
	}
	return false
}

// installFakeBridge installs f as the aggression bridge for the
// test, restoring the default afterwards.
func installFakeBridge(t *testing.T, f AggressionCombat) {
	t.Helper()
	SetAggressionCombatBridge(f)
	t.Cleanup(func() { SetAggressionCombatBridge(nil) })
}

// hunterNPC returns an aggressive NPC at loc with spawn loc,
// registered, using the aggressive fake definition.
func hunterNPC(id int, loc Location) *NPC {
	n := NewNPC(id, loc)
	n.SetDefinition(aggressiveFakeDef())
	n.SetRegistered(true)
	return n
}

// aggressionPlayer returns a registered player at loc with no
// tolerance state.
func aggressionPlayer(loc Location) *Player {
	p := NewPlayer("alice", loc)
	p.SetRegistered(true)
	return p
}

func TestToleranceLifecycle(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3000, Y: 3000, Z: 0})
	tol := NewAggressionTolerance(p)
	if _, ok := tol.State(); ok {
		t.Fatal("fresh player must have no tolerance state")
	}
	if tol.Finished() {
		t.Fatal("fresh player must not be finished")
	}
	// Seeding: both centres on the tile, 0 ticks.
	tol.Update(p.Location())
	state, ok := tol.State()
	if !ok {
		t.Fatal("update must seed state")
	}
	if state.Ticks != 0 || state.Older != (ToleranceCentre{X: 3000, Y: 3000, Z: 0}) || state.Newer != state.Older {
		t.Errorf("seeded state = %+v", state)
	}
	// Counting inside the regions.
	for i := 0; i < 5; i++ {
		tol.Update(Location{X: 3005, Y: 3005, Z: 0})
	}
	state, _ = tol.State()
	if state.Ticks != 5 {
		t.Errorf("ticks = %d, want 5", state.Ticks)
	}
	// Moving within 10 of the newer centre keeps counting.
	tol.Update(Location{X: 3010, Y: 3000, Z: 0})
	state, _ = tol.State()
	if state.Ticks != 6 {
		t.Errorf("ticks = %d, want 6 (within the newer region)", state.Ticks)
	}
	// Moving outside both regions restarts: older <- newer,
	// newer <- the tile left on, ticks 0.
	tol.Update(Location{X: 3025, Y: 3000, Z: 0})
	state, _ = tol.State()
	if state.Ticks != 0 || state.Newer != (ToleranceCentre{X: 3025, Y: 3000, Z: 0}) {
		t.Errorf("restarted state = %+v", state)
	}
	if state.Older != (ToleranceCentre{X: 3000, Y: 3000, Z: 0}) {
		t.Errorf("older centre = %+v, want the previous newer centre", state.Older)
	}
	// A plane change is leaving, even on the same x/y.
	tol.Update(Location{X: 3025, Y: 3000, Z: 1})
	state, _ = tol.State()
	if state.Ticks != 0 || state.Newer.Z != 1 {
		t.Errorf("plane-change state = %+v", state)
	}
}

func TestToleranceFinishedBoundary(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3000, Y: 3000, Z: 0})
	tol := NewAggressionTolerance(p)
	centre := ToleranceCentre{X: 3000, Y: 3000, Z: 0}
	p.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(centre, centre, ToleranceTicks-1))
	if tol.Finished() {
		t.Error("999 ticks must not be finished")
	}
	tol.Update(p.Location())
	if !tol.Finished() {
		t.Error("1000 ticks must be finished")
	}
	// Counting stops at the cap.
	tol.Update(p.Location())
	state, _ := tol.State()
	if state.Ticks != ToleranceTicks {
		t.Errorf("ticks = %d, want capped at %d", state.Ticks, ToleranceTicks)
	}
}

func TestToleranceInvalidStateReseeds(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3000, Y: 3000, Z: 0})
	tol := NewAggressionTolerance(p)
	p.SetAttribute(AggressionToleranceAttribute, "not a state")
	tol.Update(p.Location())
	state, ok := tol.State()
	if !ok || state.Ticks != 0 {
		t.Errorf("invalid state must reseed, got %+v, %v", state, ok)
	}
	// A map missing a centre is invalid too.
	p.SetAttribute(AggressionToleranceAttribute, map[string]any{"ticks": 500})
	tol.Update(p.Location())
	state, ok = tol.State()
	if !ok || state.Ticks != 0 {
		t.Errorf("centre-less state must reseed, got %+v, %v", state, ok)
	}
}

func TestToleranceSurvivesJSONRoundTrip(t *testing.T) {
	// The save layer JSON-clones attribute values: the stored
	// map round-trips through JSON, and numbers come back as
	// float64. The state must still parse.
	centre := ToleranceCentre{X: 3000, Y: 3000, Z: 0}
	raw, err := json.Marshal(toleranceStateMap(centre, centre, 42))
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	p := NewPlayer("alice", Location{X: 3000, Y: 3000, Z: 0})
	p.SetAttribute(AggressionToleranceAttribute, decoded)
	state, ok := NewAggressionTolerance(p).State()
	if !ok {
		t.Fatal("JSON-round-tripped state did not parse")
	}
	if state.Ticks != 42 || state.Older != centre || state.Newer != centre {
		t.Errorf("round-tripped state = %+v", state)
	}
	// A struct value stored directly parses too.
	p2 := NewPlayer("bob", Location{X: 1, Y: 1, Z: 0})
	p2.SetAttribute(AggressionToleranceAttribute, ToleranceState{Older: centre, Newer: centre, Ticks: 7})
	if state, ok := NewAggressionTolerance(p2).State(); !ok || state.Ticks != 7 {
		t.Errorf("struct state = %+v, %v", state, ok)
	}
}

func TestFootprintDistance(t *testing.T) {
	cases := []struct {
		origin Location
		size   int
		point  Location
		want   int
	}{
		{Location{X: 10, Y: 10, Z: 0}, 1, Location{X: 10, Y: 10, Z: 0}, 0},
		{Location{X: 10, Y: 10, Z: 0}, 1, Location{X: 11, Y: 10, Z: 0}, 1},
		{Location{X: 10, Y: 10, Z: 0}, 1, Location{X: 13, Y: 12, Z: 0}, 3},
		{Location{X: 10, Y: 10, Z: 0}, 1, Location{X: 7, Y: 10, Z: 0}, 3},
		// A size-3 footprint covers 10..12 on both axes: the
		// size extends north and east only, never south/west.
		{Location{X: 10, Y: 10, Z: 0}, 3, Location{X: 12, Y: 12, Z: 0}, 0},
		{Location{X: 10, Y: 10, Z: 0}, 3, Location{X: 13, Y: 12, Z: 0}, 1},
		{Location{X: 10, Y: 10, Z: 0}, 3, Location{X: 9, Y: 10, Z: 0}, 1},
		{Location{X: 10, Y: 10, Z: 0}, 3, Location{X: 9, Y: 9, Z: 0}, 1},
		{Location{X: 10, Y: 10, Z: 0}, 3, Location{X: 15, Y: 15, Z: 0}, 3},
		{Location{X: 10, Y: 10, Z: 0}, 1, Location{X: 10, Y: 10, Z: 1}, unreachableDistance},
	}
	for _, tc := range cases {
		if got := footprintDistance(tc.origin, tc.size, tc.point); got != tc.want {
			t.Errorf("footprintDistance(%+v, %d, %+v) = %d, want %d", tc.origin, tc.size, tc.point, got, tc.want)
		}
	}
}

func TestIsAggressiveTo(t *testing.T) {
	p := aggressionPlayer(Location{X: 100, Y: 100, Z: 0})
	if got := p.Skills().CombatLevel(); got != 3 {
		t.Fatalf("default player combat level = %d, want 3", got)
	}
	weak := NewNPC(1, Location{X: 100, Y: 100, Z: 0})
	d := defaultFakeDef()
	d.combatLevel = 1 // threshold 2: the level-3 player is safe
	weak.SetDefinition(d)
	if weak.IsAggressiveTo(p, nil) {
		t.Error("level-1 NPC must not be aggressive to a level-3 player without wilderness")
	}
	strong := NewNPC(2, Location{X: 100, Y: 100, Z: 0})
	d2 := defaultFakeDef()
	d2.combatLevel = 2 // threshold 4: the level-3 player is prey
	strong.SetDefinition(d2)
	if !strong.IsAggressiveTo(p, nil) {
		t.Error("level-2 NPC must be aggressive to a level-3 player")
	}
	// Wilderness above level 0 overrides the level rule.
	wild := &fakeWilderness{in: true, level: 1}
	if !weak.IsAggressiveTo(p, wild) {
		t.Error("wilderness level 1 must make the NPC aggressive despite levels")
	}
	wild.level = 0
	if weak.IsAggressiveTo(p, wild) {
		t.Error("wilderness level 0 must not override the level rule")
	}
	wild.in = false
	wild.level = 5
	if weak.IsAggressiveTo(p, wild) {
		t.Error("not being in the wilderness must not override the level rule")
	}
}

type fakeWilderness struct {
	in    bool
	level int
}

func (w *fakeWilderness) IsIn(m Located) bool  { return w.in }
func (w *fakeWilderness) LevelAt(x, y int) int { return w.level }

func TestRunAggressionEngagesInRange(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
	npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
	if engaged := runAggression(p, []*NPC{npc}, AggressionEnv{}, false); !engaged {
		t.Error("an ordinary engagement must mark single-combat engaged")
	}
	if !fake.attacked(npc) {
		t.Error("in-range aggressive NPC did not attack")
	}
}

func TestRunAggressionRanges(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	t.Run("outside hunt range", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 35, Y: 30, Z: 0}) // 5 from the NPC
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})   // hunt max(3,1) = 3
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("NPC attacked outside its hunt range")
		}
	})
	t.Run("outside spawn range despite hunt range", func(t *testing.T) {
		fake.attacks = nil
		// The NPC wandered 10 tiles from its spawn; the player
		// stands next to it, but 11 from the spawn footprint:
		// beyond aggression range 8.
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		npc.SetLocation(Location{X: 40, Y: 30, Z: 0})
		p := aggressionPlayer(Location{X: 41, Y: 30, Z: 0})
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("NPC attacked outside its spawn aggression range")
		}
	})
	t.Run("aggression range edge", func(t *testing.T) {
		fake.attacks = nil
		// Hunt 10, follow 7, melee reach 1: aggression range 8.
		// A player 8 from the spawn engages; 9 does not.
		for _, dist := range []int{8, 9} {
			fake.attacks = nil
			npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
			d := aggressiveFakeDef()
			d.hunt = 10
			npc.SetDefinition(d)
			p := aggressionPlayer(Location{X: 30 + dist, Y: 30, Z: 0})
			runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
			if dist == 8 && !fake.attacked(npc) {
				t.Error("NPC did not attack at aggression range 8")
			}
			if dist == 9 && fake.attacked(npc) {
				t.Error("NPC attacked at spawn distance 9, beyond range 8")
			}
		}
	})
	t.Run("ranged reach extends hunt and spawn range", func(t *testing.T) {
		fake.attacks = nil
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		d := aggressiveFakeDef()
		d.attackType = definition.CombatTypeRanged // reach 6, hunt max(3,6) = 6
		npc.SetDefinition(d)
		p := aggressionPlayer(Location{X: 35, Y: 30, Z: 0}) // 5 away
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("ranged NPC did not attack at distance 5 (reach 6)")
		}
	})
	t.Run("cross-plane player never hunted", func(t *testing.T) {
		fake.attacks = nil
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 1})
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("NPC attacked a cross-plane player")
		}
	})
}

func TestRunAggressionTolerance(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	finishTolerance := func(p *Player) {
		centre := ToleranceCentre{X: p.Location().X, Y: p.Location().Y, Z: p.Location().Z}
		p.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(centre, centre, ToleranceTicks))
	}
	npcAt := func() *NPC { return hunterNPC(1, Location{X: 30, Y: 30, Z: 0}) }
	playerAt := func() *Player { return aggressionPlayer(Location{X: 31, Y: 30, Z: 0}) }

	t.Run("finished tolerance blocks", func(t *testing.T) {
		fake.attacks = nil
		p := playerAt()
		finishTolerance(p)
		npc := npcAt()
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("tolerant player was attacked")
		}
	})
	t.Run("unfinished tolerance does not block", func(t *testing.T) {
		fake.attacks = nil
		p := playerAt()
		centre := ToleranceCentre{X: 31, Y: 30, Z: 0}
		p.SetAttribute(AggressionToleranceAttribute, toleranceStateMap(centre, centre, 999))
		npc := npcAt()
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("player at 999 ticks must still be attacked")
		}
	})
	t.Run("non-building NPC ignores tolerance", func(t *testing.T) {
		fake.attacks = nil
		p := playerAt()
		finishTolerance(p)
		npc := npcAt()
		d := aggressiveFakeDef()
		d.tolerance = false
		npc.SetDefinition(d)
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("NPC that does not build tolerance must attack a tolerant player")
		}
	})
	t.Run("area tolerance override keeps aggression", func(t *testing.T) {
		fake.attacks = nil
		p := playerAt()
		finishTolerance(p)
		area := NewArea("test", []Boundary{NewBoundary(0, 100, 0, 100, 0)})
		yes := true
		area.NpcAggressionToleranceHook = func(*Player, *NPC) *bool { return &yes }
		p.SetArea(area)
		npc := npcAt()
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("area tolerance override must keep the NPC aggressive")
		}
	})
	t.Run("emit override keeps aggression, area wins over emit", func(t *testing.T) {
		fake.attacks = nil
		p := playerAt()
		finishTolerance(p)
		yes := true
		env := AggressionEnv{EmitNpcAggressionTolerance: func(*Player, *NPC) *bool { return &yes }}
		npc := npcAt()
		runAggression(p, []*NPC{npc}, env, false)
		if !fake.attacked(npc) {
			t.Error("emit override must keep the NPC aggressive")
		}
		// An area answer of false beats the emit's true (?? chain).
		fake.attacks = nil
		area := NewArea("test", []Boundary{NewBoundary(0, 100, 0, 100, 0)})
		no := false
		area.NpcAggressionToleranceHook = func(*Player, *NPC) *bool { return &no }
		p.SetArea(area)
		runAggression(p, []*NPC{npc}, env, false)
		if fake.attacked(npc) {
			t.Error("area tolerance false must beat the emit override")
		}
	})
}

func TestRunAggressionAreaRule(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	areaWithRule := func(rule *bool) *Area {
		area := NewArea("test", []Boundary{NewBoundary(0, 100, 0, 100, 0)})
		area.NpcAggressionHook = func(*Player, *NPC) *bool { return rule }
		return area
	}
	t.Run("rule false forbids", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		no := false
		p.SetArea(areaWithRule(&no))
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("area rule false must forbid aggression")
		}
	})
	t.Run("rule true forces a non-aggressive NPC", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		yes := true
		p.SetArea(areaWithRule(&yes))
		npc := NewNPC(1, Location{X: 30, Y: 30, Z: 0})
		npc.SetDefinition(defaultFakeDef()) // not aggressive
		npc.SetRegistered(true)
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("area rule true must force aggression from a non-aggressive NPC")
		}
	})
	t.Run("rule true overrides spawn range", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 41, Y: 30, Z: 0})
		yes := true
		p.SetArea(areaWithRule(&yes))
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		npc.SetLocation(Location{X: 40, Y: 30, Z: 0}) // 11 from spawn
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("area rule true must override the spawn-range check")
		}
	})
	t.Run("rule true does not override hunt range", func(t *testing.T) {
		// The hunt-range check has no rule override in TS: a
		// forced NPC still only hunts within its hunt range.
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 36, Y: 30, Z: 0}) // 6 away, hunt 3
		yes := true
		p.SetArea(areaWithRule(&yes))
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("area rule true must not override the hunt-range check")
		}
	})
}

func TestRunAggressionCombatAndScope(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	t.Run("in-combat NPC never re-hunts", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		fake.inCombat[&npc.Mobile] = true
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("in-combat NPC re-hunted")
		}
	})
	t.Run("canAttack refusal blocks", func(t *testing.T) {
		fake.attacks = nil
		fake.canAttack = false
		defer func() { fake.canAttack = true }()
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("NPC attacked despite canAttack refusal")
		}
	})
	t.Run("dying NPC skipped, unset hitpoints alive", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		dying := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		dying.Dying = true
		runAggression(p, []*NPC{dying}, AggressionEnv{}, false)
		if fake.attacked(dying) {
			t.Error("dying NPC attacked")
		}
		// Hitpoints 0 means unset, not dead: a fresh hunter
		// with unset hitpoints still aggresses.
		fake.attacks = nil
		fresh := hunterNPC(2, Location{X: 30, Y: 30, Z: 0})
		if fresh.Hitpoints != 0 {
			t.Fatalf("fixture hitpoints = %d, want unset 0", fresh.Hitpoints)
		}
		runAggression(p, []*NPC{fresh}, AggressionEnv{}, false)
		if !fake.attacked(fresh) {
			t.Error("NPC with unset hitpoints must still aggress")
		}
	})
	t.Run("owner-only NPC skips strangers", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		npc.SetOwnerOnly(true)
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("owner-only NPC with no owner attacked a stranger")
		}
		npc.SetOwner(p)
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("owner-only NPC must attack its owner")
		}
	})
	t.Run("non-aggressive definition skipped", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := NewNPC(1, Location{X: 30, Y: 30, Z: 0})
		npc.SetDefinition(defaultFakeDef())
		npc.SetRegistered(true)
		runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
		if fake.attacked(npc) {
			t.Error("non-aggressive NPC attacked")
		}
	})
	t.Run("nil entries skipped", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		runAggression(p, []*NPC{nil, npc, nil}, AggressionEnv{}, false)
		if !fake.attacked(npc) {
			t.Error("nil entries must not stop the scan")
		}
	})
}

func TestRunAggressionSingleVsMulti(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	twoHunters := func() (*Player, *NPC, *NPC) {
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		a := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		b := hunterNPC(2, Location{X: 30, Y: 31, Z: 0})
		return p, a, b
	}
	t.Run("single combat: only the first hunter engages", func(t *testing.T) {
		fake.attacks = nil
		p, a, b := twoHunters()
		runAggression(p, []*NPC{a, b}, AggressionEnv{}, false)
		if !fake.attacked(a) || fake.attacked(b) {
			t.Errorf("attacks = %d, want exactly the first hunter", len(fake.attacks))
		}
	})
	t.Run("an earlier pass counts as engaged", func(t *testing.T) {
		fake.attacks = nil
		p, a, _ := twoHunters()
		runAggression(p, []*NPC{a}, AggressionEnv{}, true)
		if fake.attacked(a) {
			t.Error("a hunter must not engage when an earlier pass already took the player")
		}
	})
	t.Run("multi combat: every hunter engages", func(t *testing.T) {
		fake.attacks = nil
		p, a, b := twoHunters()
		env := AggressionEnv{InMulti: func(AreaActor) bool { return true }}
		runAggression(p, []*NPC{a, b}, env, false)
		if !fake.attacked(a) || !fake.attacked(b) {
			t.Errorf("attacks = %d, want both hunters in multi", len(fake.attacks))
		}
	})
	t.Run("always-multi NPC joins despite engagement", func(t *testing.T) {
		fake.attacks = nil
		p, a, b := twoHunters()
		b.SetMultiCombat(true)
		runAggression(p, []*NPC{a, b}, AggressionEnv{}, false)
		if !fake.attacked(a) || !fake.attacked(b) {
			t.Errorf("attacks = %d, want both (second is always-multi)", len(fake.attacks))
		}
	})
	t.Run("always-multi first does not count as engaged", func(t *testing.T) {
		fake.attacks = nil
		p, a, b := twoHunters()
		a.SetMultiCombat(true)
		engaged := runAggression(p, []*NPC{a}, AggressionEnv{}, false)
		if engaged {
			t.Error("an always-multi engagement must not mark single-combat engaged")
		}
		runAggression(p, []*NPC{b}, AggressionEnv{}, engaged)
		if !fake.attacked(b) {
			t.Error("second hunter must still engage after an always-multi first")
		}
	})
}

func TestProcessNpcAggressionPlayerInCombat(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
	npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
	p.localNPCs = []*NPC{npc}
	fake.inCombat[&p.Mobile] = true
	// Single-combat: an in-combat player takes no new hunters.
	ProcessNpcAggression(p, AggressionEnv{})
	if fake.attacked(npc) {
		t.Error("in-combat player in singles took a new hunter")
	}
	// Multi-combat: the gate opens.
	fake.attacks = nil
	env := AggressionEnv{InMulti: func(AreaActor) bool { return true }}
	ProcessNpcAggression(p, env)
	if !fake.attacked(npc) {
		t.Error("in-combat player in multi must still take hunters")
	}
}

func TestProcessPlayerAggression(t *testing.T) {
	fake := &fakeAggressionCombat{inCombat: map[*Mobile]bool{}, canAttack: true}
	installFakeBridge(t, fake)
	t.Run("updates tolerance and aggresses", func(t *testing.T) {
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		p.localNPCs = []*NPC{npc}
		ProcessPlayerAggression(p, AggressionEnv{})
		if _, ok := NewAggressionTolerance(p).State(); !ok {
			t.Error("tolerance state must be seeded by the player aggression block")
		}
		if !fake.attacked(npc) {
			t.Error("NPC did not aggress through the player block")
		}
	})
	t.Run("bots skip entirely", func(t *testing.T) {
		fake.attacks = nil
		p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
		p.IsBot = true
		npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
		p.localNPCs = []*NPC{npc}
		ProcessPlayerAggression(p, AggressionEnv{})
		if _, ok := NewAggressionTolerance(p).State(); ok {
			t.Error("bots must not update tolerance")
		}
		if fake.attacked(npc) {
			t.Error("bots must not draw aggression")
		}
	})
	t.Run("nil player is safe", func(t *testing.T) {
		ProcessPlayerAggression(nil, AggressionEnv{})
		ProcessNpcAggression(nil, AggressionEnv{})
	})
}

func TestDefaultBridgeNeverEngages(t *testing.T) {
	// With no bridge installed, decisions run but CanAttack
	// refuses: nothing engages until the combat core installs
	// the bridge.
	SetAggressionCombatBridge(nil)
	p := aggressionPlayer(Location{X: 31, Y: 30, Z: 0})
	npc := hunterNPC(1, Location{X: 30, Y: 30, Z: 0})
	engaged := runAggression(p, []*NPC{npc}, AggressionEnv{}, false)
	if engaged {
		t.Error("default bridge must not report an engagement")
	}
	if d := (defaultAggressionCombat{}); d.CanAttack(npc, p) {
		t.Error("default bridge CanAttack must be false")
	}
}
