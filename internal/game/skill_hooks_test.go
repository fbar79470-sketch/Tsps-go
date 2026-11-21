package game

import (
	"math"
	"testing"
)

// resetSkillsSeam restores the package-level skills seam and world
// rate after the test, so gate/rate mutations cannot leak into
// other tests (combat tests drive AddExperience with the default
// inert seam and rate 1).
func resetSkillsSeam(t *testing.T) {
	t.Helper()
	oldHooks := SkillHooks
	oldRate := ExperienceRate()
	t.Cleanup(func() {
		SkillHooks = oldHooks
		SetExperienceRate(oldRate)
	})
	SkillHooks = SkillsHooks{}
	SetExperienceRate(1.0)
}

// expDropRecorder captures exp-drop hook invocations.
type expDropRecorder struct {
	calls [][2]int // [skill, exp]
}

func (r *expDropRecorder) hook(skill, exp int) {
	r.calls = append(r.calls, [2]int{skill, exp})
}

// customEventRecorder captures Player.EmitCustomEvent calls.
type customEventRecorder struct {
	names   []string
	payload []any
}

func (r *customEventRecorder) emit(name string, payload any) {
	r.names = append(r.names, name)
	r.payload = append(r.payload, payload)
}

func TestSetExperienceRate(t *testing.T) {
	resetSkillsSeam(t)
	if got := ExperienceRate(); got != 1.0 {
		t.Fatalf("default rate = %v, want 1.0", got)
	}
	SetExperienceRate(2.5)
	if got := ExperienceRate(); got != 2.5 {
		t.Errorf("rate after set = %v, want 2.5", got)
	}
	// GameConstants.setExperienceRate ignores non-finite and
	// non-positive values (GameConstants.ts:119-124).
	for _, bad := range []float64{0, -1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		SetExperienceRate(bad)
		if got := ExperienceRate(); got != 2.5 {
			t.Errorf("rate after SetExperienceRate(%v) = %v, want 2.5 (unchanged)", bad, got)
		}
	}
}

func TestAddExperienceGateDeny(t *testing.T) {
	resetSkillsSeam(t)
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	drops := &expDropRecorder{}
	sm.SetExpDropHook(drops.hook)
	var gateArgs [3]any
	SkillHooks.CanGainExperience = func(player *Player, skill, experience int) (*bool, float64) {
		gateArgs = [3]any{player, skill, experience}
		return boolPtr(false), 3
	}
	SkillHooks.EmitExperienceGained = func(*ExperienceGainedEvent) {
		t.Error("denied gain emitted experience-gained")
	}
	SkillHooks.EmitPlayerLevelUp = func(*PlayerLevelUpEvent) {
		t.Error("denied gain emitted level-up")
	}
	events := &customEventRecorder{}
	p.EmitCustomEvent = events.emit

	sm.AddExperience(SkillAttack, 1000)

	// Deny returns before any send (SkillManager.ts:136-139):
	// no XP, no update, no exp drop, no emits.
	if got := sm.Experience(SkillAttack); got != 0 {
		t.Errorf("denied XP = %d, want 0", got)
	}
	if len(rec.updates) != 0 {
		t.Errorf("denied hook calls = %d, want 0", len(rec.updates))
	}
	if len(drops.calls) != 0 {
		t.Errorf("denied exp drops = %v, want none", drops.calls)
	}
	if len(events.names) != 0 {
		t.Errorf("denied custom events = %v, want none", events.names)
	}
	// The gate sees the player, skill, and pre-rate experience.
	if gateArgs[0] != any(p) || gateArgs[1] != 0 || gateArgs[2] != 1000 {
		t.Errorf("gate args = %v, want [player 0 1000]", gateArgs)
	}
}

func TestAddExperienceGateMultiplierAndWorldRate(t *testing.T) {
	resetSkillsSeam(t)
	// World rate 2 * gate multiplier 3 = 6x.
	SetExperienceRate(2)
	SkillHooks.CanGainExperience = func(*Player, int, int) (*bool, float64) {
		return nil, 3
	}
	_, sm := newTestSkillManager()
	sm.AddExperience(SkillAttack, 10)
	if got := sm.Experience(SkillAttack); got != 60 {
		t.Errorf("XP = %d, want 60 (world 2 * gate 3)", got)
	}
}

func TestAddExperienceGateMultiplierSanitize(t *testing.T) {
	// PluginManager.ts:1673: the combined multiplier is
	// sanitized finite-and-positive else 1.
	for _, mult := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 0, -2} {
		t.Run("multiplier", func(t *testing.T) {
			resetSkillsSeam(t)
			SkillHooks.CanGainExperience = func(*Player, int, int) (*bool, float64) {
				return nil, mult
			}
			_, sm := newTestSkillManager()
			sm.AddExperience(SkillAttack, 10)
			if got := sm.Experience(SkillAttack); got != 10 {
				t.Errorf("multiplier %v: XP = %d, want 10 (sanitized to 1)", mult, got)
			}
		})
	}
}

func TestAddExperienceMultipliersFalseRateOne(t *testing.T) {
	resetSkillsSeam(t)
	SetExperienceRate(3)
	SkillHooks.CanGainExperience = func(*Player, int, int) (*bool, float64) {
		return nil, 5
	}
	p, sm := newTestSkillManager()
	events := &customEventRecorder{}
	p.EmitCustomEvent = events.emit
	sm.AddExperienceWithMultipliers(SkillAttack, 10, false)
	if got := sm.Experience(SkillAttack); got != 10 {
		t.Errorf("XP = %d, want 10 (multipliers=false => rate 1)", got)
	}
	if len(events.names) != 1 || events.names[0] != "skills:experience-gained" {
		t.Fatalf("custom events = %v, want one skills:experience-gained", events.names)
	}
	payload, ok := events.payload[0].(SkillsExperienceGainedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want SkillsExperienceGainedPayload value", events.payload[0])
	}
	if payload.Experience != 10 || payload.Multiplied || payload.Rate != 1 {
		t.Errorf("payload = %+v, want {Experience 10, Multiplied false, Rate 1}", payload)
	}
	// The gate is still consulted with multipliers=false: a deny
	// refuses the gain.
	SkillHooks.CanGainExperience = func(*Player, int, int) (*bool, float64) {
		return boolPtr(false), 1
	}
	sm.AddExperienceWithMultipliers(SkillCooking, 10, false)
	if got := sm.Experience(SkillCooking); got != 0 {
		t.Errorf("denied multipliers=false XP = %d, want 0", got)
	}
}

func TestAddExperienceTenthsRounding(t *testing.T) {
	resetSkillsSeam(t)
	// 3 XP at rate 1.5 = 4.5 tenths, truncated to int 4 (the
	// putInt truncation; integer storage cannot keep tenths).
	SetExperienceRate(1.5)
	_, sm := newTestSkillManager()
	sm.AddExperience(SkillAttack, 3)
	if got := sm.Experience(SkillAttack); got != 4 {
		t.Errorf("XP = %d, want 4 (3 * 1.5 = 4.5, truncated)", got)
	}
	// A gain that rounds to zero tenths adds nothing and sends
	// nothing (SkillManager.ts:146-148).
	SetExperienceRate(0.04)
	drops := &expDropRecorder{}
	sm.SetExpDropHook(drops.hook)
	sm.AddExperience(SkillCooking, 1)
	if got := sm.Experience(SkillCooking); got != 0 {
		t.Errorf("XP = %d, want 0", got)
	}
	if len(drops.calls) != 0 {
		t.Errorf("exp drops = %v, want none for a zero gain", drops.calls)
	}
}

func TestAddExperienceExpDropPosition(t *testing.T) {
	resetSkillsSeam(t)
	p, sm := newTestSkillManager()
	drops := &expDropRecorder{}
	sm.SetExpDropHook(drops.hook)
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)

	// A locked gain still sends the exp drop (TS position:
	// after the tenths step, before the lock check), with the
	// post-rate tenths value, but adds no XP and sends no
	// update.
	SetExperienceRate(2)
	p.SetAttribute(XPLockedAttribute, true)
	sm.AddExperience(SkillAttack, 10)
	if len(drops.calls) != 1 || drops.calls[0] != [2]int{SkillAttack, 20} {
		t.Errorf("locked exp drops = %v, want [[0 20]]", drops.calls)
	}
	if got := sm.Experience(SkillAttack); got != 0 {
		t.Errorf("locked XP = %d, want 0", got)
	}
	if len(rec.updates) != 0 {
		t.Errorf("locked hook calls = %d, want 0", len(rec.updates))
	}

	// A capped gain also sends the drop and adds nothing.
	p.SetAttribute(XPLockedAttribute, nil)
	SetExperienceRate(1)
	sm.SetExperiences(SkillCooking, MaxExperience, false)
	sm.AddExperience(SkillCooking, 50)
	if len(drops.calls) != 2 || drops.calls[1] != [2]int{SkillCooking, 50} {
		t.Errorf("exp drops = %v, want a second [7 50] for the capped gain", drops.calls)
	}
	if got := sm.Experience(SkillCooking); got != MaxExperience {
		t.Errorf("capped XP = %d, want %d", got, MaxExperience)
	}
}

func TestAddExperienceLockBeforeCapOrdering(t *testing.T) {
	// The lock return sits before the max-exp sanitize
	// write-back (SkillManager.ts:156 vs 159-163): a locked
	// gain leaves an over-cap stored value untouched, while an
	// unlocked gain sanitizes it to the cap.
	resetSkillsSeam(t)
	p, sm := newTestSkillManager()
	p.SetAttribute(XPLockedAttribute, true)
	sm.experience[SkillAttack] = MaxExperience + 1
	sm.AddExperience(SkillAttack, 10)
	if got := sm.experience[SkillAttack]; got != MaxExperience+1 {
		t.Errorf("locked stored XP = %d, want %d (untouched)", got, MaxExperience+1)
	}
	p.SetAttribute(XPLockedAttribute, nil)
	sm.AddExperience(SkillAttack, 10)
	if got := sm.experience[SkillAttack]; got != MaxExperience {
		t.Errorf("unlocked stored XP = %d, want %d (sanitized)", got, MaxExperience)
	}
}

func TestAddExperienceLevelUpEmit(t *testing.T) {
	resetSkillsSeam(t)
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	var levelUps []*PlayerLevelUpEvent
	SkillHooks.EmitPlayerLevelUp = func(e *PlayerLevelUpEvent) {
		levelUps = append(levelUps, e)
	}
	p.Flags = p.Flags.Reset()
	sm.AddExperience(SkillAttack, 83)
	if len(levelUps) != 1 {
		t.Fatalf("level-up emits = %d, want 1", len(levelUps))
	}
	e := levelUps[0]
	if e.Player != p || e.Skill != SkillAttack || e.OldLevel != 1 || e.NewLevel != 2 {
		t.Errorf("level-up event = %+v, want {player, skill 0, old 1, new 2}", e)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("level-up did not flag APPEARANCE")
	}
	// The new levels publish through the update hook before
	// feedback would run: the refresh from SetCurrentLevels.
	if len(rec.updates) == 0 {
		t.Fatal("no skill update published on level-up")
	}
	last := rec.updates[len(rec.updates)-1]
	if last.MaxLevel != 2 || last.CurrentLevel != 2 {
		t.Errorf("published update = %+v, want levels 2/2", last)
	}

	// A non-level-up gain sends an update and emits no level-up.
	levelUps = nil
	rec.updates = nil
	sm.AddExperience(SkillAttack, 10)
	if len(levelUps) != 0 {
		t.Errorf("non-level-up emits = %d, want 0", len(levelUps))
	}
	if len(rec.updates) != 1 {
		t.Errorf("non-level-up hook calls = %d, want 1", len(rec.updates))
	}
}

func TestAddExperienceGainedLandedVsCustomAmounts(t *testing.T) {
	resetSkillsSeam(t)
	p, sm := newTestSkillManager()
	// Stage at level 99 with 5 XP of headroom, so the gain
	// lands partially without levelling up.
	sm.SetMaxLevels(SkillAttack, 99, false)
	sm.SetExperiences(SkillAttack, MaxExperience-5, false)
	var gained []*ExperienceGainedEvent
	SkillHooks.EmitExperienceGained = func(e *ExperienceGainedEvent) {
		gained = append(gained, e)
	}
	events := &customEventRecorder{}
	p.EmitCustomEvent = events.emit

	sm.AddExperience(SkillAttack, 100)

	if got := sm.Experience(SkillAttack); got != MaxExperience {
		t.Fatalf("XP = %d, want %d", got, MaxExperience)
	}
	// The hook event carries the LANDED post-cap delta (5);
	// the custom event carries the post-rate tenths amount
	// (100). The two values differ by design.
	if len(gained) != 1 {
		t.Fatalf("experience-gained emits = %d, want 1", len(gained))
	}
	if gained[0].Experience != 5 || gained[0].Skill != SkillAttack || gained[0].Player != p {
		t.Errorf("gained event = %+v, want {player, skill 0, experience 5}", gained[0])
	}
	if len(events.names) != 1 || events.names[0] != "skills:experience-gained" {
		t.Fatalf("custom events = %v, want one skills:experience-gained", events.names)
	}
	payload, ok := events.payload[0].(SkillsExperienceGainedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want SkillsExperienceGainedPayload value", events.payload[0])
	}
	if payload.Player != p || payload.Skill != SkillAttack || payload.Experience != 100 || !payload.Multiplied || payload.Rate != 1 {
		t.Errorf("custom payload = %+v, want {player, 0, 100, true, 1}", payload)
	}
}

func TestAddExperienceNoEmitsForRefusedLockedCapped(t *testing.T) {
	resetSkillsSeam(t)
	// Refused: no emits at all (gate deny covered in
	// TestAddExperienceGateDeny for sends; here the emits).
	SkillHooks.CanGainExperience = func(*Player, int, int) (*bool, float64) {
		return boolPtr(false), 1
	}
	SkillHooks.EmitExperienceGained = func(*ExperienceGainedEvent) {
		t.Error("refused gain emitted experience-gained")
	}
	_, sm := newTestSkillManager()
	sm.AddExperience(SkillAttack, 100)

	// Locked and capped: the hook and custom emits stay silent
	// (the exp drop still fires; see TestAddExperienceExpDropPosition).
	SkillHooks.CanGainExperience = nil
	p, sm := newTestSkillManager()
	SkillHooks.EmitExperienceGained = func(*ExperienceGainedEvent) {
		t.Error("locked/capped gain emitted experience-gained")
	}
	events := &customEventRecorder{}
	p.EmitCustomEvent = events.emit
	p.SetAttribute(XPLockedAttribute, true)
	sm.AddExperience(SkillAttack, 100)
	p.SetAttribute(XPLockedAttribute, nil)
	sm.SetExperiences(SkillCooking, MaxExperience, false)
	sm.AddExperience(SkillCooking, 100)
	if len(events.names) != 0 {
		t.Errorf("locked/capped custom events = %v, want none", events.names)
	}
}

func TestSetCurrentLevelChangedEvent(t *testing.T) {
	p, sm := newTestSkillManager()
	var order []string
	p.EmitCustomEvent = func(name string, payload any) {
		order = append(order, "event:"+name)
	}
	sm.SetUpdateHook(func(SkillUpdate) { order = append(order, "update") })

	// A change emits BEFORE the refresh (SkillManager.ts:530-535).
	sm.SetCurrentLevel(SkillAttack, 5, true)
	if len(order) != 2 || order[0] != "event:skills:current-level-changed" || order[1] != "update" {
		t.Errorf("order = %v, want [event skills:current-level-changed, update]", order)
	}
	// An equal value emits nothing but still refreshes.
	order = nil
	sm.SetCurrentLevel(SkillAttack, 5, true)
	if len(order) != 1 || order[0] != "update" {
		t.Errorf("equal-value order = %v, want [update] only", order)
	}
	// Payload contents, and the negative clamp.
	events := &customEventRecorder{}
	p.EmitCustomEvent = events.emit
	sm.SetCurrentLevel(SkillAttack, -3, false)
	if len(events.names) != 1 || events.names[0] != "skills:current-level-changed" {
		t.Fatalf("events = %v, want one current-level-changed", events.names)
	}
	payload, ok := events.payload[0].(SkillsCurrentLevelChangedPayload)
	if !ok {
		t.Fatalf("payload type = %T, want SkillsCurrentLevelChangedPayload value", events.payload[0])
	}
	if payload.Player != p || payload.Skill != SkillAttack || payload.Previous != 5 || payload.Current != 0 {
		t.Errorf("payload = %+v, want {player, 0, previous 5, current 0}", payload)
	}
	// No EmitCustomEvent installed: no panic, level still set.
	p.EmitCustomEvent = nil
	sm.SetCurrentLevel(SkillAttack, 7, false)
	if got := sm.CurrentLevel(SkillAttack); got != 7 {
		t.Errorf("level = %d, want 7", got)
	}
}

func TestIncreaseCurrentLevelRestoreReadBack(t *testing.T) {
	// Handler changes the amount: the changed value is used.
	p, sm := newTestSkillManager()
	p.EmitCustomEvent = func(name string, payload any) {
		if name == "skills:restore" {
			payload.(*SkillsRestorePayload).Amount = 8
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 5, 10)
	if got := sm.CurrentLevel(SkillAttack); got != 9 {
		t.Errorf("changed-amount level = %d, want 9 (1 + 8)", got)
	}

	// Handler leaves it alone: the original amount is used.
	p, sm = newTestSkillManager()
	var seen *SkillsRestorePayload
	p.EmitCustomEvent = func(name string, payload any) {
		if name == "skills:restore" {
			seen = payload.(*SkillsRestorePayload)
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 5, 10)
	if got := sm.CurrentLevel(SkillAttack); got != 6 {
		t.Errorf("unchanged-amount level = %d, want 6", got)
	}
	if seen == nil || seen.Player != p || seen.Skill != SkillAttack || seen.Amount != 5 || seen.Max != 10 {
		t.Errorf("restore payload = %+v, want {player, 0, 5, 10}", seen)
	}

	// Handler sets a negative amount: ignored (TS read-back
	// rule requires >= 0).
	p, sm = newTestSkillManager()
	p.EmitCustomEvent = func(name string, payload any) {
		if name == "skills:restore" {
			payload.(*SkillsRestorePayload).Amount = -3
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 5, 10)
	if got := sm.CurrentLevel(SkillAttack); got != 6 {
		t.Errorf("negative-amount level = %d, want 6 (original amount kept)", got)
	}

	// Handler sets zero: changed and >= 0, so it is used — no
	// increase, but the refresh still runs through
	// SetCurrentLevels.
	p, sm = newTestSkillManager()
	p.EmitCustomEvent = func(name string, payload any) {
		if name == "skills:restore" {
			payload.(*SkillsRestorePayload).Amount = 0
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 5, 10)
	if got := sm.CurrentLevel(SkillAttack); got != 1 {
		t.Errorf("zero-amount level = %d, want 1", got)
	}

	// No restore emit when amount <= 0 (SkillManager.ts:588).
	// (A negative amount still lowers the level through
	// SetCurrentLevels, which may emit current-level-changed;
	// only the restore event is asserted here.)
	p, sm = newTestSkillManager()
	fired := false
	p.EmitCustomEvent = func(name string, _ any) {
		if name == "skills:restore" {
			fired = true
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 0, 10)
	sm.IncreaseCurrentLevel(SkillAttack, -2, 10)
	if fired {
		t.Error("restore event fired for amount <= 0")
	}

	// The emit runs before the cap check: a level already at
	// the cap still fires the event, and stays put.
	p, sm = newTestSkillManager()
	sm.SetCurrentLevel(SkillAttack, 10, false)
	fired = false
	p.EmitCustomEvent = func(name string, payload any) {
		if name == "skills:restore" {
			fired = true
		}
	}
	sm.IncreaseCurrentLevel(SkillAttack, 5, 10)
	if !fired {
		t.Error("restore event did not fire for a capped level")
	}
	if got := sm.CurrentLevel(SkillAttack); got != 10 {
		t.Errorf("capped level = %d, want 10", got)
	}
}

// recordingSkillable is a StartableSkill test double recording
// Cancel/HasRequirements/Start calls.
type recordingSkillable struct {
	requirements  bool
	cancelCalls   []*Player
	startCalls    []*Player
	requireChecks []*Player
}

func (f *recordingSkillable) Cancel(player *Player) { f.cancelCalls = append(f.cancelCalls, player) }
func (f *recordingSkillable) HasRequirements(player *Player) bool {
	f.requireChecks = append(f.requireChecks, player)
	return f.requirements
}
func (f *recordingSkillable) Start(player *Player) { f.startCalls = append(f.startCalls, player) }

// bareSkillable is a cancel-only skill action (the TS bare
// `{cancel}` objects content sets directly).
type bareSkillable struct{ cancelCalls int }

func (b *bareSkillable) Cancel(*Player) { b.cancelCalls++ }

func TestSkillSlotLifecycle(t *testing.T) {
	// Start with requirements: slot set, started, interfaces
	// removed (DialogueSender seam).
	p, s := newDialoguePlayer()
	sm := p.Skills()
	skill := &recordingSkillable{requirements: true}
	sm.StartSkillable(skill)
	if p.GetSkill() != Skillable(skill) {
		t.Error("slot not set after StartSkillable")
	}
	if len(skill.startCalls) != 1 || skill.startCalls[0] != p {
		t.Errorf("start calls = %d, want 1 for the player", len(skill.startCalls))
	}
	if s.removals != 1 {
		t.Errorf("interface removals = %d, want 1", s.removals)
	}

	// Start replaces: the previous action is cancelled.
	next := &recordingSkillable{requirements: true}
	sm.StartSkillable(next)
	if len(skill.cancelCalls) != 1 || skill.cancelCalls[0] != p {
		t.Errorf("previous cancel calls = %d, want 1 for the player", len(skill.cancelCalls))
	}
	if p.GetSkill() != Skillable(next) {
		t.Error("slot not replaced by the second start")
	}
	if len(next.startCalls) != 1 {
		t.Errorf("second start calls = %d, want 1", len(next.startCalls))
	}

	// Stop cancels and clears; a second stop is a no-op.
	sm.StopSkillable()
	if len(next.cancelCalls) != 1 {
		t.Errorf("stop cancel calls = %d, want 1", len(next.cancelCalls))
	}
	if p.GetSkill() != nil {
		t.Error("slot not cleared by StopSkillable")
	}
	sm.StopSkillable()
	if len(next.cancelCalls) != 1 {
		t.Errorf("second stop cancel calls = %d, want 1 (no repeat)", len(next.cancelCalls))
	}
}

func TestSkillSlotRequirementsRefusal(t *testing.T) {
	p, s := newDialoguePlayer()
	sm := p.Skills()
	previous := &recordingSkillable{requirements: true}
	sm.StartSkillable(previous)
	// A skill without requirements: the previous action is
	// still stopped and interfaces still removed (TS order),
	// but the slot stays empty and Start never runs.
	refused := &recordingSkillable{requirements: false}
	sm.StartSkillable(refused)
	if len(previous.cancelCalls) != 1 {
		t.Errorf("previous cancel calls = %d, want 1", len(previous.cancelCalls))
	}
	if s.removals != 2 {
		t.Errorf("interface removals = %d, want 2", s.removals)
	}
	if p.GetSkill() != nil {
		t.Error("refused skill set the slot")
	}
	if len(refused.startCalls) != 0 {
		t.Errorf("refused start calls = %d, want 0", len(refused.startCalls))
	}
}

func TestSkillSlotBareCancelAndNil(t *testing.T) {
	p, sm := newTestSkillManager()
	// A bare {cancel} object set directly is cancelled by
	// StopSkillable, as TS stopSkillable cancels whatever the
	// slot holds.
	bare := &bareSkillable{}
	p.SetSkill(bare)
	if p.GetSkill() != Skillable(bare) {
		t.Fatal("direct SetSkill did not set the slot")
	}
	sm.StopSkillable()
	if bare.cancelCalls != 1 {
		t.Errorf("bare cancel calls = %d, want 1", bare.cancelCalls)
	}
	if p.GetSkill() != nil {
		t.Error("slot not cleared after bare cancel")
	}

	// StartSkillable(nil) stops the previous action and
	// returns without panicking (TS would throw; Go must not
	// panic the tick goroutine).
	skill := &recordingSkillable{requirements: true}
	sm.StartSkillable(skill)
	sm.StartSkillable(nil)
	if len(skill.cancelCalls) != 1 {
		t.Errorf("nil-start cancel calls = %d, want 1", len(skill.cancelCalls))
	}
	if p.GetSkill() != nil {
		t.Error("slot not cleared by nil start")
	}

	// Nil-player slot access is safe.
	var nilPlayer *Player
	if nilPlayer.GetSkill() != nil {
		t.Error("nil player GetSkill != nil")
	}
	nilPlayer.SetSkill(bare) // must not panic
	// A manager with no player does nothing.
	NewSkillManager(nil).StopSkillable()
	NewSkillManager(nil).StartSkillable(skill)
}

func TestSkillSlotCancelCannotReinstall(t *testing.T) {
	// TS order: cancel runs before setSkill(null), so a Cancel
	// that installs a new action still has it cleared.
	p, sm := newTestSkillManager()
	other := &recordingSkillable{requirements: true}
	p.SetSkill(cancelReinstaller{next: other})
	sm.StopSkillable()
	if p.GetSkill() != nil {
		t.Error("slot not cleared after a reinstalling Cancel")
	}
}

// cancelReinstaller is a Skillable whose Cancel installs another
// action in the slot.
type cancelReinstaller struct{ next Skillable }

func (c cancelReinstaller) Cancel(player *Player) { player.SetSkill(c.next) }

func TestWalkToResetStopsSkill(t *testing.T) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	skill := &recordingSkillable{requirements: true}
	p.Skills().StartSkillable(skill)
	if p.GetSkill() == nil {
		t.Fatal("slot not set")
	}
	// A walk click runs walkToReset: the skill stops.
	p.MovementQueue().RequestWalk(Location{X: 3091, Y: 3524, Z: 0}, nil)
	if len(skill.cancelCalls) != 1 {
		t.Fatalf("cancel calls after RequestWalk = %d, want 1", len(skill.cancelCalls))
	}
	if p.GetSkill() != nil {
		t.Error("slot not cleared by walkToReset")
	}

	// WalkToTile also runs walkToReset.
	skill2 := &recordingSkillable{requirements: true}
	p.Skills().StartSkillable(skill2)
	p.MovementQueue().WalkToTile(Location{X: 3095, Y: 3524, Z: 0}, func() {}, nil, false, 0)
	if len(skill2.cancelCalls) != 1 {
		t.Errorf("cancel calls after WalkToTile = %d, want 1", len(skill2.cancelCalls))
	}

	// CancelWalkTo alone does NOT stop a skill: it is the
	// clearPendingAction shape, not walkToReset.
	skill3 := &recordingSkillable{requirements: true}
	p.Skills().StartSkillable(skill3)
	p.MovementQueue().CancelWalkTo()
	if len(skill3.cancelCalls) != 0 {
		t.Errorf("CancelWalkTo cancel calls = %d, want 0", len(skill3.cancelCalls))
	}
	if p.GetSkill() == nil {
		t.Error("CancelWalkTo cleared the slot")
	}
}

// namedItemDefs wraps an ItemDefinitionSource with item names,
// satisfying the optional ItemNameSource capability.
type namedItemDefs struct {
	ItemDefinitionSource
	names map[int]string
}

func (s namedItemDefs) ItemName(id int) string { return s.names[id] }

func TestSetLevelWildernessGuard(t *testing.T) {
	p, fs := combatTestPlayer(t, "wild", 3089, 3524)
	sm := p.Skills()
	am := NewAreaManager()
	am.SetWilderness(pvpWilderness())
	p.areaManager = am
	// (3089, 3524) is inside the PvP box (3000-3300, 3500-3700).
	sm.SetLevel(SkillAttack, 50)
	if got := sm.GetMaxLevels(SkillAttack); got != 1 {
		t.Errorf("wilderness max level = %d, want 1 (refused)", got)
	}
	if len(fs.messages) != 1 || fs.messages[0] != "You cannot do this in the Wilderness!" {
		t.Errorf("wilderness messages = %v, want the TS refusal", fs.messages)
	}
	// Outside the Wilderness the guard passes.
	p.SetLocation(Location{X: 100, Y: 100, Z: 0})
	sm.SetLevel(SkillAttack, 50)
	if got := sm.GetMaxLevels(SkillAttack); got != 50 {
		t.Errorf("outside-wilderness max level = %d, want 50", got)
	}
}

func TestSetLevelNoAreaManagerPasses(t *testing.T) {
	// A player with no AreaManager is never in the Wilderness.
	_, sm := newTestSkillManager()
	sm.SetLevel(SkillAttack, 50)
	if got := sm.GetMaxLevels(SkillAttack); got != 50 {
		t.Errorf("max level = %d, want 50", got)
	}
}

func TestSetLevelEquipmentGuard(t *testing.T) {
	defs := &testItemDefs{
		requirements: map[int][]int{
			100: {60, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	named := namedItemDefs{ItemDefinitionSource: defs, names: map[int]string{100: "Dragon sword"}}
	p, fs := combatTestPlayer(t, "equipped", 3089, 3524)
	p.SetItemDefinitions(named)
	p.Equipment().Set(WeaponSlot, NewItem(100, 1))
	sm := p.Skills()

	// Setting Attack below the worn requirement refuses.
	sm.SetLevel(SkillAttack, 50)
	if got := sm.GetMaxLevels(SkillAttack); got != 1 {
		t.Errorf("guarded max level = %d, want 1 (refused)", got)
	}
	want := "Please unequip your Dragon sword before doing that."
	if len(fs.messages) != 1 || fs.messages[0] != want {
		t.Errorf("messages = %v, want [%q]", fs.messages, want)
	}
	// Setting exactly the requirement passes (TS: requirement
	// > level refuses).
	sm.SetLevel(SkillAttack, 60)
	if got := sm.GetMaxLevels(SkillAttack); got != 60 {
		t.Errorf("max level = %d, want 60", got)
	}
	// Other skills are unaffected by the Attack requirement.
	sm.SetLevel(SkillDefence, 50)
	if got := sm.GetMaxLevels(SkillDefence); got != 50 {
		t.Errorf("defence max level = %d, want 50", got)
	}
}

func TestSetLevelEquipmentGuardNoNameSource(t *testing.T) {
	// Without the ItemNameSource capability the name reads ""
	// (the DEFAULT definition's empty name); the guard still
	// refuses, with the degraded text.
	defs := &testItemDefs{
		requirements: map[int][]int{100: {60}},
	}
	p, fs := combatTestPlayer(t, "noname", 3089, 3524)
	p.SetItemDefinitions(defs)
	p.Equipment().Set(WeaponSlot, NewItem(100, 1))
	p.Skills().SetLevel(SkillAttack, 50)
	if got := p.Skills().GetMaxLevels(SkillAttack); got != 1 {
		t.Errorf("max level = %d, want 1 (refused)", got)
	}
	want := "Please unequip your  before doing that."
	if len(fs.messages) != 1 || fs.messages[0] != want {
		t.Errorf("messages = %v, want [%q]", fs.messages, want)
	}
	// A short requirements array: a missing entry counts as 0,
	// so a skill past its length is unguarded.
	p.Skills().SetLevel(SkillDefence, 50)
	if got := p.Skills().GetMaxLevels(SkillDefence); got != 50 {
		t.Errorf("defence max level = %d, want 50", got)
	}
}

func TestSetLevelEquipmentGuardNoSource(t *testing.T) {
	// No definition source: no requirements, the guard passes.
	p, sm := newTestSkillManager()
	p.Equipment().Set(WeaponSlot, NewItem(100, 1))
	sm.SetLevel(SkillAttack, 50)
	if got := sm.GetMaxLevels(SkillAttack); got != 50 {
		t.Errorf("max level = %d, want 50", got)
	}
}

func TestSetLevelDeactivatesPrayersAndDoubleSend(t *testing.T) {
	p, _ := combatTestPlayer(t, "praying", 3089, 3524)
	sm := p.Skills()
	if !ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Fatal("prayer activation failed")
	}
	if !PrayerActive(&p.Mobile, PrayerProtectFromMelee) {
		t.Fatal("prayer not active before SetLevel")
	}
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.SetLevel(SkillAttack, 50)
	if PrayerActive(&p.Mobile, PrayerProtectFromMelee) {
		t.Error("prayer still active after SetLevel (deactivatePrayers)")
	}
	// The TS double send: setExperience refreshes, then
	// setLevel calls updateSkill again.
	if len(rec.updates) != 2 {
		t.Errorf("update hook calls = %d, want 2 (double send)", len(rec.updates))
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetLevel did not flag APPEARANCE")
	}
	// The Hitpoints floor still refuses, before assignment.
	p.Flags = p.Flags.Reset()
	sm.SetLevel(SkillHitpoints, 5)
	if got := sm.GetMaxLevels(SkillHitpoints); got != 10 {
		t.Errorf("hitpoints max level = %d, want 10 (floor refused)", got)
	}
}
