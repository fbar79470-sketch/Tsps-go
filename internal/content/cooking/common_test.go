package cooking

import (
	"math"
	"testing"

	"tsps-go/internal/content/fishing"
	"tsps-go/internal/game"
)

func TestSuccessChanceEndpoints(t *testing.T) {
	// Level 1 reads low+1 over 256; level 99 reads high+1 over
	// 256 (Common.Cooking.js:115-118 interp, floor + 0.5 + 1).
	if got := successChance(128, 512, 1); got != 129.0/256 {
		t.Errorf("successChance(128,512,1) = %v, want %v", got, 129.0/256)
	}
	if got := successChance(128, 512, 99); math.Abs(got-1) > 1e-9 {
		t.Errorf("successChance(128,512,99) = %v, want 1 (513/256 clamped)", got)
	}
	if got := successChance(48, 352, 99); got != 1 {
		t.Errorf("successChance(48,352,99) = %v, want 1", got)
	}
	// Midpoint: level 50 of [48, 352] is near the middle of
	// the line, well inside [0, 1].
	mid := successChance(48, 352, 50)
	if mid <= 0 || mid >= 1 {
		t.Errorf("successChance(48,352,50) = %v, want strictly inside (0,1)", mid)
	}
	// Uncapped past 99: a food that still burns at 99 keeps
	// improving (the TS NOTE in Common.Cooking.js:115-118).
	low99 := successChance(1, 200, 99)
	low150 := successChance(1, 200, 150)
	if !(low150 > low99) {
		t.Errorf("successChance at 150 (%v) <= at 99 (%v), want it to keep rising", low150, low99)
	}
	// A degenerate chart clamps to [0, 1].
	if got := successChance(-1000, -500, 1); got != 0 {
		t.Errorf("successChance(-1000,-500,1) = %v, want 0 (clamped)", got)
	}
}

func TestWithArticle(t *testing.T) {
	cases := []struct{ name, article, want string }{
		{"shark", "", "a shark"},
		{"anglerfish", "", "an anglerfish"},
		{"shrimps", "some", "some shrimps"},
		{"giant krill", "the", "the giant krill"},
		{"eel", "", "an eel"},
	}
	for _, tc := range cases {
		if got := withArticle(tc.name, tc.article); got != tc.want {
			t.Errorf("withArticle(%q, %q) = %q, want %q", tc.name, tc.article, got, tc.want)
		}
	}
}

func TestCookingLevelDefaultAndBoost(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	setCooking(p, 40)
	if got := c.cookingLevel(p); got != 40 {
		t.Errorf("cookingLevel = %d, want 40 (no answerer, boost 0)", got)
	}
	// An answerer raising the boost is honored, clamped >= 0.
	c.emit = func(name string, payload any) {
		if req, ok := payload.(*LevelBoostRequest); ok {
			req.Boost = 12
		}
	}
	if got := c.cookingLevel(p); got != 52 {
		t.Errorf("cookingLevel with +12 boost = %d, want 52", got)
	}
	c.emit = func(name string, payload any) {
		if req, ok := payload.(*LevelBoostRequest); ok {
			req.Boost = -5
		}
	}
	if got := c.cookingLevel(p); got != 40 {
		t.Errorf("cookingLevel with -5 boost = %d, want 40 (clamped >= 0)", got)
	}
	if got := c.cookingLevel(nil); got != 0 {
		t.Errorf("cookingLevel(nil) = %d, want 0", got)
	}
}

func TestActionsPerCycleDefaultAndAnswered(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	if got := c.actionsPerCycle(p, "make"); got != 1 {
		t.Errorf("actionsPerCycle = %d, want 1 (no answerer)", got)
	}
	c.emit = func(name string, payload any) {
		if req, ok := payload.(*ActionsPerCycleRequest); ok {
			req.Actions = 3
		}
	}
	if got := c.actionsPerCycle(p, "make"); got != 3 {
		t.Errorf("actionsPerCycle answered 3 = %d, want 3", got)
	}
	c.emit = func(name string, payload any) {
		if req, ok := payload.(*ActionsPerCycleRequest); ok {
			req.Actions = 0
		}
	}
	if got := c.actionsPerCycle(p, "make"); got != 1 {
		t.Errorf("actionsPerCycle answered 0 = %d, want 1 (clamped >= 1)", got)
	}
}

func TestWearsCookingCapeDefaultFalse(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	// No Go answerer exists for skillcape:has-perk, so Result
	// stays false (the honest no-hooks default).
	if c.wearsCookingCape(p) {
		t.Error("wearsCookingCape = true, want false (no answerer)")
	}
}

func TestAddXPTruncates(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	setCooking(p, 1)
	c.addXP(p, 82.5)
	if got := p.Skills().Experience(game.SkillCooking); got != 82 {
		t.Errorf("Experience after 82.5 XP = %d, want 82 (truncated)", got)
	}
	c.addXP(p, 0)
	c.addXP(p, -5)
	if got := p.Skills().Experience(game.SkillCooking); got != 82 {
		t.Errorf("Experience after 0/-5 XP = %d, want 82 (no-op)", got)
	}
}

func TestKnives(t *testing.T) {
	c, _, _ := newTestCooking()
	if got := c.knives(); len(got) != 1 || got[0] != fxKnife {
		t.Errorf("knives = %v, want [%d]", got, fxKnife)
	}
	empty := newCooking(nil)
	if got := empty.knives(); len(got) != 0 {
		t.Errorf("knives with no tables = %v, want none", got)
	}
}

func TestSendVarbit(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	session := &fakeSession{}
	p.Session = session
	c.sendVarbit(p, "busy", 1)
	if got := session.varbits[7002]; got != 1 {
		t.Errorf("busy varbit = %d, want 1", got)
	}
	// An unresolvable name sends nothing (Common.Cooking.js
	// sendVarbit skips it).
	c.sendVarbit(p, "no_such_varbit", 1)
	if len(session.varbits) != 1 {
		t.Errorf("varbits after unknown name = %v, want only busy", session.varbits)
	}
	// A session-less player sends nowhere.
	p2 := newTestPlayer("bob")
	c.sendVarbit(p2, "busy", 1)
}

func TestMesboxAndSpam(t *testing.T) {
	c, _, _ := newTestCooking()
	p := newTestPlayer("alice")
	session := &fakeSession{}
	p.Session = session
	c.spam(p, "You mix the water and flour.")
	if len(session.spam) != 1 || session.spam[0] != "You mix the water and flour." {
		t.Errorf("spam = %v, want the mix line", session.spam)
	}
	// mesbox opens a statement dialogue without panicking on a
	// session-less dialogue manager path.
	c.mesbox(p, "You need a Cooking level of 99 to make a test.")
	c.mesbox(nil, "nothing")
}

func TestRunStopsOnDeathAndUnregister(t *testing.T) {
	// A running step ticks once per cycle.
	c, _, tm := newTestCooking()
	p := newTestPlayer("alice")
	p.SetIndex(7)
	calls := 0
	c.run(p, func(tick int64) bool {
		calls++
		return true
	})
	driveTicks(tm, 3)
	if calls != 3 {
		t.Errorf("run calls = %d, want 3", calls)
	}

	// Death stops the task before the first step.
	c, _, tm = newTestCooking()
	p = newTestPlayer("alice")
	p.SetIndex(8)
	setTestLevel(p, game.SkillHitpoints, 0)
	calls = 0
	c.run(p, func(tick int64) bool {
		calls++
		return true
	})
	driveTicks(tm, 3)
	if calls != 0 {
		t.Errorf("run calls after death = %d, want 0 (stopped)", calls)
	}

	// Unregistered stops the task before the first step.
	c, _, tm = newTestCooking()
	p = newTestPlayer("alice")
	p.SetIndex(9)
	p.SetRegistered(false)
	calls = 0
	c.run(p, func(tick int64) bool {
		calls++
		return true
	})
	driveTicks(tm, 3)
	if calls != 0 {
		t.Errorf("run calls unregistered = %d, want 0 (stopped)", calls)
	}

	// A step returning false stops after one tick.
	c, _, tm = newTestCooking()
	p = newTestPlayer("alice")
	p.SetIndex(10)
	calls = 0
	c.run(p, func(tick int64) bool {
		calls++
		return false
	})
	driveTicks(tm, 3)
	if calls != 1 {
		t.Errorf("run calls after false step = %d, want 1 (one tick, then stopped)", calls)
	}

	// No TaskManager schedules nothing.
	none := newCooking(fixtureTables())
	calls = 0
	none.run(p, func(tick int64) bool {
		calls++
		return true
	})
	if calls != 0 {
		t.Errorf("run calls with no TaskManager = %d, want 0", calls)
	}
}

func TestAnswerRawXP(t *testing.T) {
	c, _, _ := newTestCooking()
	// Hit: raw shrimps answer 30 XP.
	req := &fishing.CookingRawXPRequest{Player: newTestPlayer("alice"), RawID: fxRawShrimp}
	c.answerRawXP(req)
	if !req.HasXP || req.XP != 30 {
		t.Errorf("raw-xp shrimps = %+v, want HasXP + XP 30", req)
	}
	// Facility foods never answer (Cookers must NOT cook them).
	req = &fishing.CookingRawXPRequest{Player: newTestPlayer("alice"), RawID: fxRawGuppy}
	c.answerRawXP(req)
	if req.HasXP || req.XP != 0 {
		t.Errorf("raw-xp guppy = %+v, want unanswered (facility)", req)
	}
	// Unknown raw never answers.
	req = &fishing.CookingRawXPRequest{Player: newTestPlayer("alice"), RawID: 99999}
	c.answerRawXP(req)
	if req.HasXP {
		t.Errorf("raw-xp unknown = %+v, want unanswered", req)
	}
	// Wrong payload type and nil are ignored.
	c.answerRawXP("not a request")
	c.answerRawXP(nil)
	c.answerRawXP((*fishing.CookingRawXPRequest)(nil))
}

func TestAnswerCooked(t *testing.T) {
	c, _, _ := newTestCooking()
	req := &CookedRequest{Player: newTestPlayer("alice"), RawID: fxRawShrimp}
	c.answerCooked(req)
	if req.CookedID != fxShrimp || req.XP != 30 {
		t.Errorf("cooked shrimps = %+v, want CookedID %d XP 30", req, fxShrimp)
	}
	// Facility foods never answer.
	req = &CookedRequest{Player: newTestPlayer("alice"), RawID: fxRawGuppy}
	c.answerCooked(req)
	if req.CookedID != 0 || req.XP != 0 {
		t.Errorf("cooked guppy = %+v, want unanswered (facility)", req)
	}
	// Unknown raw never answers.
	req = &CookedRequest{Player: newTestPlayer("alice"), RawID: 99999}
	c.answerCooked(req)
	if req.CookedID != 0 {
		t.Errorf("cooked unknown = %+v, want unanswered", req)
	}
	c.answerCooked("not a request")
	c.answerCooked(nil)
}
