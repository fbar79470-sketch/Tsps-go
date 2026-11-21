package game

import "testing"

// Formula vectors: expected values computed by hand from the TS
// formulas (MovementQueue.ts:803-815) and cross-checked against
// Node evaluating the TS expressions verbatim (float64 op-for-op),
// so float-boundary floors (e.g. weight 0 / agility 10 = 58) match
// JS exactly.

func TestRunEnergyDrainUnits(t *testing.T) {
	// base = floor((60 + floor(67*weight/64)) * (1 - agility/300)),
	// weight clamped 0..64, result clamped >= 0. Identity modifiers.
	cases := []struct {
		weight, agility, want int
	}{
		// Weight 0: 60 * factor.
		{0, 0, 60}, {0, 1, 59}, {0, 10, 58}, {0, 50, 50}, {0, 99, 40},
		// Weight 1: floor(67/64) = 1 -> 61 * factor.
		{1, 0, 61}, {1, 1, 60}, {1, 10, 58}, {1, 50, 50}, {1, 99, 40},
		// Weight 32: floor(2144/64) = 33 -> 93 * factor.
		{32, 0, 93}, {32, 1, 92}, {32, 10, 89}, {32, 50, 77}, {32, 99, 62},
		// Weight 64: floor(4288/64) = 67 -> 127 * factor.
		{64, 0, 127}, {64, 1, 126}, {64, 10, 122}, {64, 50, 105}, {64, 99, 85},
		// Weight above the cap clamps to 64.
		{65, 0, 127}, {65, 1, 126}, {65, 10, 122}, {65, 50, 105}, {65, 99, 85},
		{1000, 50, 105},
		// Negative weight clamps to 0.
		{-1, 0, 60}, {-1, 1, 59}, {-1, 10, 58}, {-1, 50, 50}, {-1, 99, 40},
		{-64, 99, 40},
		// Agility at/above 300 zeroes the factor; the result
		// clamps to >= 0 rather than going negative.
		{64, 300, 0}, {64, 301, 0}, {0, 500, 0},
	}
	for _, c := range cases {
		if got := RunEnergyDrainUnits(c.weight, c.agility); got != c.want {
			t.Errorf("RunEnergyDrainUnits(%d, %d) = %d, want %d", c.weight, c.agility, got, c.want)
		}
	}
}

func TestRunEnergyRestoreUnits(t *testing.T) {
	// base = floor(agility/10) + 15, floored, clamped >= 0.
	// Identity modifiers (no graceful set).
	cases := []struct {
		agility, want int
	}{
		{0, 15}, {1, 15}, {9, 15}, {10, 16}, {19, 16}, {20, 17},
		{50, 20}, {99, 24}, {100, 25}, {126, 27},
	}
	for _, c := range cases {
		if got := RunEnergyRestoreUnits(c.agility); got != c.want {
			t.Errorf("RunEnergyRestoreUnits(%d) = %d, want %d", c.agility, got, c.want)
		}
	}
}

func TestRunEnergyAccessors(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	// Field defaults: max units (Player.ts:185), Agility 1 (fresh
	// skills), weight 0.
	if got := p.GetRunEnergyUnits(); got != RunEnergyMaxUnits {
		t.Fatalf("default units = %d, want %d", got, RunEnergyMaxUnits)
	}
	if got := p.GetRunEnergy(); got != 100 {
		t.Fatalf("default percent = %v, want 100", got)
	}
	if got := p.GetCurrentAgilityLevel(); got != 1 {
		t.Fatalf("default agility = %d, want 1", got)
	}
	if got := p.GetWeightKg(); got != 0 {
		t.Fatalf("default weight = %d, want 0", got)
	}

	// getRunEnergy = units/100 (Player.ts:1258).
	p.SetRunEnergyUnits(7750)
	if got := p.GetRunEnergy(); got != 77.5 {
		t.Fatalf("GetRunEnergy() = %v, want 77.5", got)
	}
	// setRunEnergy: floor(percent*100 + 1e-6) (Player.ts:1263).
	// The 0.29 case is the regression the +1e-6 hair exists for:
	// 0.29*100 is 28.999... in floating point.
	p.SetRunEnergy(0.29)
	if got := p.GetRunEnergyUnits(); got != 29 {
		t.Fatalf("SetRunEnergy(0.29) = %d units, want 29", got)
	}
	if got := p.GetRunEnergy(); got != 0.29 {
		t.Fatalf("round-trip percent = %v, want 0.29", got)
	}
	p.SetRunEnergy(77.5)
	if got := p.GetRunEnergyUnits(); got != 7750 {
		t.Fatalf("SetRunEnergy(77.5) = %d, want 7750", got)
	}
	// setRunEnergyUnits clamps 0..10000 (Player.ts:1272).
	p.SetRunEnergy(100.5)
	if got := p.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("SetRunEnergy(100.5) = %d, want 10000 (clamped)", got)
	}
	p.SetRunEnergy(-5)
	if got := p.GetRunEnergyUnits(); got != 0 {
		t.Fatalf("SetRunEnergy(-5) = %d, want 0 (clamped)", got)
	}
	p.SetRunEnergyUnits(10001)
	if got := p.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("SetRunEnergyUnits(10001) = %d, want 10000", got)
	}
	p.SetRunEnergyUnits(-1)
	if got := p.GetRunEnergyUnits(); got != 0 {
		t.Fatalf("SetRunEnergyUnits(-1) = %d, want 0", got)
	}
	// addRunEnergyUnits adds then clamps (Player.ts:1276).
	p.SetRunEnergyUnits(9990)
	p.AddRunEnergyUnits(20)
	if got := p.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("add past max = %d, want 10000", got)
	}
	p.SetRunEnergyUnits(10)
	p.AddRunEnergyUnits(-20)
	if got := p.GetRunEnergyUnits(); got != 0 {
		t.Fatalf("add past zero = %d, want 0", got)
	}
	p.SetRunEnergyUnits(5000)
	p.AddRunEnergyUnits(123)
	if got := p.GetRunEnergyUnits(); got != 5123 {
		t.Fatalf("add = %d, want 5123", got)
	}
}

func TestAgilityLevelSanitize(t *testing.T) {
	// getCurrentLevel (SkillManager.ts:425-433): negative -> write
	// 0, return 0. The level lives on the SkillManager; the
	// player read delegates to it.
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	level, maxLevel, xp := p.Skills().Snapshot()
	level[SkillAgility] = 99
	p.Skills().SetSkills(level, maxLevel, xp)
	if got := p.GetCurrentAgilityLevel(); got != 99 {
		t.Fatalf("agility = %d, want 99", got)
	}
	// SetSkills clamps negatives at write time, so the read-path
	// sanitize is exercised by direct field write (same package),
	// as a stale state could: the write-back sticks.
	p.skills.level[SkillAgility] = -7
	if got := p.GetCurrentAgilityLevel(); got != 0 || p.skills.level[SkillAgility] != 0 {
		t.Fatalf("read sanitize: got %d field %d, want 0/0", got, p.skills.level[SkillAgility])
	}
}

// drainTick runs one queue tick over a two-tile route and returns
// the owner, for drain assertions.
func drainTick(t *testing.T, owner *fakeOwner, dest Location) *MovementQueue {
	t.Helper()
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(dest, nil)
	tickQueue(q, 1)
	return q
}

func TestDrainOnTwoStepRunTick(t *testing.T) {
	// Weight 0, Agility 1: drain 59. One run-energy packet with
	// live args (units after drain, running, weight, stamina 0).
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := drainTick(t, owner, Location{X: 102, Y: 100, Z: 0})
	if owner.loc != (Location{X: 102, Y: 100, Z: 0}) {
		t.Fatalf("owner at %+v, want two steps east", owner.loc)
	}
	if !q.DidRunThisCycle() {
		t.Fatal("two-step run tick must set ranThisCycle")
	}
	if got := owner.GetRunEnergyUnits(); got != 10000-59 {
		t.Fatalf("units = %d, want %d", got, 10000-59)
	}
	want := [][4]any{{9941, true, 0, 0}}
	if len(owner.runEnergy) != 1 || owner.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", owner.runEnergy, want)
	}
}

func TestDrainUsesWeightAndAgility(t *testing.T) {
	// Weight 32, Agility 50: drain 77. Packet carries the live
	// weight.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.weightKg = 32
	owner.agility = 50
	owner.energyUnits = 5000
	drainTick(t, owner, Location{X: 102, Y: 100, Z: 0})
	if got := owner.GetRunEnergyUnits(); got != 5000-77 {
		t.Fatalf("units = %d, want %d", got, 5000-77)
	}
	want := [][4]any{{4923, true, 32, 0}}
	if len(owner.runEnergy) != 1 || owner.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", owner.runEnergy, want)
	}
}

func TestNoDrainOnOneStepTick(t *testing.T) {
	// A lone final step is a walk (MovementQueue.ts:646 comment):
	// one tile left, one step taken, no drain, no send, no run
	// marker. BeginCycle resets ranThisCycle between ticks.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 103, Y: 100, Z: 0}, nil)
	tickQueue(q, 1) // two steps, drains 59
	if owner.GetRunEnergyUnits() != 9941 {
		t.Fatalf("setup: units = %d, want 9941", owner.GetRunEnergyUnits())
	}
	tickQueue(q, 2) // one step left
	if owner.loc != (Location{X: 103, Y: 100, Z: 0}) {
		t.Fatalf("owner at %+v", owner.loc)
	}
	if q.DidRunThisCycle() {
		t.Fatal("a one-step tick must not count as running")
	}
	if got := owner.GetRunEnergyUnits(); got != 9941 {
		t.Fatalf("units = %d, want 9941 (no drain on a one-step tick)", got)
	}
	if len(owner.runEnergy) != 1 {
		t.Fatalf("runEnergy sends = %d, want 1 (the first tick only)", len(owner.runEnergy))
	}
}

func TestNoDrainOnWalkTick(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.running = false
	q := drainTick(t, owner, Location{X: 102, Y: 100, Z: 0})
	if owner.loc.X != 101 {
		t.Fatalf("walking tick moved to %+v, want one step", owner.loc)
	}
	if q.DidRunThisCycle() {
		t.Fatal("a walking tick must not count as running")
	}
	if got := owner.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("units = %d, want 10000 (no drain while walking)", got)
	}
	if len(owner.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none", owner.runEnergy)
	}
}

func TestNoDrainWhenNotRunToggled(t *testing.T) {
	// Running setting on, but a path runOverride of false: the
	// queue walks, so no drain.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 102, Y: 100, Z: 0}, boolPtr(false))
	tickQueue(q, 1)
	if owner.loc.X != 101 {
		t.Fatalf("override-walk tick moved to %+v, want one step", owner.loc)
	}
	if got := owner.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("units = %d, want 10000", got)
	}
	if len(owner.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none", owner.runEnergy)
	}
}

func TestNoDrainWithoutMovement(t *testing.T) {
	// No checkpoints, no movement: Process returns early and the
	// drain never runs, even run-toggled.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	tickQueue(q, 1)
	if got := owner.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("units = %d, want 10000", got)
	}
	if len(owner.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none", owner.runEnergy)
	}
}

func TestAutoStopAtZeroEnergy(t *testing.T) {
	// Energy 30, drain 59: clamps to 0, clears BOTH overrides,
	// setRunning(false), and sends run status AND run energy —
	// two identical packets with (0, false, weight, 0).
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.energyUnits = 30
	owner.weightKg = 5
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 105, Y: 100, Z: 0}, boolPtr(true))
	// Stage a click override too: RequestWalk clears it, so set
	// both fields directly (same package) as an interaction click
	// followed by a re-path would leave them.
	q.clickRunOverride = boolPtr(true)
	if !q.IsRunToggled() {
		t.Fatal("setup: queue must be run-toggled")
	}
	tickQueue(q, 1)
	if got := owner.GetRunEnergyUnits(); got != 0 {
		t.Fatalf("units = %d, want 0", got)
	}
	if owner.IsRunning() {
		t.Fatal("running must be false after auto-stop")
	}
	if q.runOverride != nil || q.clickRunOverride != nil {
		t.Fatal("both run overrides must be cleared at zero energy")
	}
	if q.IsRunToggled() {
		t.Fatal("queue must read as walking after auto-stop")
	}
	if !q.DidRunThisCycle() {
		t.Fatal("the auto-stop tick still counts as a run tick")
	}
	want := [][4]any{{0, false, 5, 0}, {0, false, 5, 0}}
	if len(owner.runEnergy) != 2 || owner.runEnergy[0] != want[0] || owner.runEnergy[1] != want[1] {
		t.Fatalf("runEnergy sends = %v, want %v (status + energy)", owner.runEnergy, want)
	}
	// The next tick walks (one step): no further drain, no sends.
	before := owner.loc
	tickQueue(q, 2)
	if owner.loc.X != before.X+1 {
		t.Fatalf("post-stop tick moved to %+v, want one walking step", owner.loc)
	}
	if len(owner.runEnergy) != 2 {
		t.Fatalf("runEnergy sends = %d, want 2 (no send on a walk tick)", len(owner.runEnergy))
	}
}

func TestBotDrainPinning(t *testing.T) {
	// Bots are pinned to max instead of draining (MovementQueue.ts:
	// 819-824): no drain, no ranThisCycle, no sends.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.bot = true
	owner.energyUnits = 5000
	q := drainTick(t, owner, Location{X: 102, Y: 100, Z: 0})
	if owner.loc.X != 102 {
		t.Fatalf("bot moved to %+v, want two steps (bots still move)", owner.loc)
	}
	if got := owner.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("bot units = %d, want 10000 (pinned to max)", got)
	}
	if q.DidRunThisCycle() {
		t.Fatal("a bot tick must not set ranThisCycle")
	}
	if len(owner.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none for a bot", owner.runEnergy)
	}
}

func TestBotAtMaxStaysPinnedWithoutSend(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.bot = true
	drainTick(t, owner, Location{X: 102, Y: 100, Z: 0})
	if got := owner.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("bot units = %d, want 10000", got)
	}
	if len(owner.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none", owner.runEnergy)
	}
}

func TestBotNotPinnedWithoutMovement(t *testing.T) {
	// The pin lives in drainRunEnergy, which Process calls only
	// when the player moved: an idle bot keeps its energy.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	owner.bot = true
	owner.energyUnits = 5000
	q := NewMovementQueue(owner, nil, nil)
	tickQueue(q, 1)
	if got := owner.GetRunEnergyUnits(); got != 5000 {
		t.Fatalf("idle bot units = %d, want 5000 (no pin without movement)", got)
	}
}

func TestBeginCycleResetsRanCycle(t *testing.T) {
	// BeginCycle clears ranThisCycle (MovementQueue.ts:475), so a
	// run tick does not suppress the NEXT tick's restore.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.RequestWalk(Location{X: 102, Y: 100, Z: 0}, nil)
	tickQueue(q, 1)
	if !q.DidRunThisCycle() {
		t.Fatal("setup: run tick must set ranThisCycle")
	}
	q.BeginCycle()
	if q.DidRunThisCycle() {
		t.Fatal("BeginCycle must clear ranThisCycle")
	}
}

// --- Restore branch (Player.Process, Player.ts:559-566) ---

func restorePlayer(t *testing.T, energy, agility, weight int) (*Player, *fakeSession) {
	t.Helper()
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.SetRunEnergyUnits(energy)
	// Agility is the live SkillManager current level.
	level, maxLevel, xp := p.Skills().Snapshot()
	level[SkillAgility] = agility
	p.Skills().SetSkills(level, maxLevel, xp)
	// Weight is the live carried weight: one inventory item
	// weighing weight kilograms.
	p.SetItemDefinitions(&testItemDefs{weights: map[int]int{1: weight * 1000}})
	if weight != 0 {
		p.Inventory().SetItem(0, NewItem(1, 1))
	}
	session := newFakeSession()
	p.Session = session
	return p, session
}

func TestRestoreOnIdleTick(t *testing.T) {
	// Agility 50 restores 20/tick; the send carries live args.
	p, session := restorePlayer(t, 9900, 50, 12)
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 9920 {
		t.Fatalf("units = %d, want 9920", got)
	}
	want := [][4]any{{9920, true, 12, 0}}
	if len(session.runEnergy) != 1 || session.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", session.runEnergy, want)
	}
}

func TestRestoreClampsAtMax(t *testing.T) {
	// Below max, so the branch runs; the add clamps at max.
	p, session := restorePlayer(t, 9990, 50, 0)
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("units = %d, want 10000", got)
	}
	if len(session.runEnergy) != 1 {
		t.Fatalf("runEnergy sends = %d, want 1", len(session.runEnergy))
	}
}

func TestRestoreSkippedAtMax(t *testing.T) {
	p, session := restorePlayer(t, 10000, 50, 0)
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 10000 {
		t.Fatalf("units = %d, want 10000", got)
	}
	if len(session.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none at max energy", session.runEnergy)
	}
}

func TestRestoreSkippedForBots(t *testing.T) {
	p, session := restorePlayer(t, 5000, 50, 0)
	p.IsBot = true
	p.Process(1)
	if got := p.GetRunEnergyUnits(); got != 5000 {
		t.Fatalf("bot units = %d, want 5000 (no restore for bots)", got)
	}
	if len(session.runEnergy) != 0 {
		t.Fatalf("runEnergy sends = %v, want none for a bot", session.runEnergy)
	}
}

func TestRestoreSkippedOnRunTick(t *testing.T) {
	// A two-step run tick drains and suppresses the restore: only
	// the drain send goes out, with drained units.
	p, session := restorePlayer(t, 9000, 1, 0)
	p.MovementQueue().RequestWalk(Location{X: 3091, Y: 3524, Z: 0}, nil)
	p.Process(1)
	// Drain 59 (weight 0, agility 1); no +15 restore.
	if got := p.GetRunEnergyUnits(); got != 9000-59 {
		t.Fatalf("units = %d, want %d (drain only, no restore)", got, 9000-59)
	}
	want := [][4]any{{8941, true, 0, 0}}
	if len(session.runEnergy) != 1 || session.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", session.runEnergy, want)
	}
}

func TestRestoreRunsOnWalkTick(t *testing.T) {
	// A walking tick is not a run tick, so the restore runs after
	// movement: position advances one tile AND energy restores.
	p, session := restorePlayer(t, 9000, 50, 0)
	p.SetRunning(false)
	p.MovementQueue().RequestWalk(Location{X: 3092, Y: 3524, Z: 0}, nil)
	p.Process(1)
	if p.Loc.X != 3090 {
		t.Fatalf("player at %+v, want one walking step", p.Loc)
	}
	if got := p.GetRunEnergyUnits(); got != 9020 {
		t.Fatalf("units = %d, want 9020 (restore 20 on a walk tick)", got)
	}
	if len(session.runEnergy) != 1 {
		t.Fatalf("runEnergy sends = %d, want 1 (restore only)", len(session.runEnergy))
	}
}

func TestRestoreRunsAfterRunTickEnds(t *testing.T) {
	// After the route drains the queue, the next idle tick
	// restores: BeginCycle cleared ranThisCycle.
	p, session := restorePlayer(t, 9000, 50, 0)
	p.MovementQueue().RequestWalk(Location{X: 3091, Y: 3524, Z: 0}, nil)
	p.Process(1) // run tick: drain 50 (weight 0, agility 50)
	if got := p.GetRunEnergyUnits(); got != 8950 {
		t.Fatalf("after run tick: units = %d, want 8950", got)
	}
	p.Process(2) // idle: restore 20
	if got := p.GetRunEnergyUnits(); got != 8970 {
		t.Fatalf("after idle tick: units = %d, want 8970", got)
	}
	if len(session.runEnergy) != 2 {
		t.Fatalf("runEnergy sends = %d, want 2 (drain + restore)", len(session.runEnergy))
	}
}

func TestRestoreWithNilSessionDoesNotPanic(t *testing.T) {
	// Session-less players restore silently: SendRunEnergy drops
	// nil-safely.
	p := NewPlayer("alice", Location{X: 3089, Y: 3524, Z: 0})
	p.SetRunEnergyUnits(9000)
	level, maxLevel, xp := p.Skills().Snapshot()
	level[SkillAgility] = 50
	p.Skills().SetSkills(level, maxLevel, xp)
	p.Process(1) // must not panic
	if got := p.GetRunEnergyUnits(); got != 9020 {
		t.Fatalf("units = %d, want 9020", got)
	}
}

func TestDrainThroughPlayerSession(t *testing.T) {
	// End-to-end through Player.Process: the queue's drain send
	// reaches the session with live args, including the carried
	// weight and stamina 0.
	p, session := restorePlayer(t, 10000, 10, 32)
	p.MovementQueue().RequestWalk(Location{X: 3091, Y: 3524, Z: 0}, nil)
	p.Process(1)
	// Drain 89 (weight 32, agility 10).
	if got := p.GetRunEnergyUnits(); got != 9911 {
		t.Fatalf("units = %d, want 9911", got)
	}
	want := [][4]any{{9911, true, 32, 0}}
	if len(session.runEnergy) != 1 || session.runEnergy[0] != want[0] {
		t.Fatalf("runEnergy sends = %v, want %v", session.runEnergy, want)
	}
}
