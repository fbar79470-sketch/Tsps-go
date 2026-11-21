package game

import (
	"testing"
)

// hookRecorder captures SkillUpdate hook invocations for assertions.
type hookRecorder struct {
	updates []SkillUpdate
}

func (r *hookRecorder) hook(update SkillUpdate) {
	r.updates = append(r.updates, update)
}

func newTestSkillManager() (*Player, *SkillManager) {
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	return p, p.Skills()
}

func TestSkillIndices(t *testing.T) {
	// Declaration order (Skill.ts:7-31). SkillHitpoints must equal
	// persist.HitpointsIndex (3) and SkillAgility must equal
	// persist.AgilityIndex (16); the game package cannot import
	// persist, so the values are pinned here with the citation to
	// internal/persist/playersave.go.
	want := []int{
		SkillAttack, SkillDefence, SkillStrength, SkillHitpoints,
		SkillRanged, SkillPrayer, SkillMagic, SkillCooking,
		SkillWoodcutting, SkillFletching, SkillFishing, SkillFiremaking,
		SkillCrafting, SkillSmithing, SkillMining, SkillHerblore,
		SkillAgility, SkillThieving, SkillSlayer, SkillFarming,
		SkillRunecrafting, SkillConstruction, SkillHunter, SkillSailing,
	}
	if len(want) != SkillCount {
		t.Fatalf("skill constants = %d, want SkillCount %d", len(want), SkillCount)
	}
	for i, got := range want {
		if got != i {
			t.Errorf("skill constant at position %d = %d, want %d", i, got, i)
		}
	}
	if SkillHitpoints != 3 {
		t.Errorf("SkillHitpoints = %d, want 3 (persist.HitpointsIndex)", SkillHitpoints)
	}
	if SkillAgility != 16 {
		t.Errorf("SkillAgility = %d, want 16 (persist.AgilityIndex)", SkillAgility)
	}
	if SkillCount != 24 {
		t.Errorf("SkillCount = %d, want 24", SkillCount)
	}
}

func TestNewSkillManagerDefaults(t *testing.T) {
	_, sm := newTestSkillManager()
	for i := 0; i < SkillCount; i++ {
		if i == SkillHitpoints {
			continue
		}
		if got := sm.CurrentLevel(i); got != 1 {
			t.Errorf("skill %d current level = %d, want 1", i, got)
		}
		if got := sm.MaxLevel(i); got != 1 {
			t.Errorf("skill %d max level = %d, want 1", i, got)
		}
		if got := sm.Experience(i); got != 0 {
			t.Errorf("skill %d experience = %d, want 0", i, got)
		}
	}
	// The constructor's Hitpoints override: 10/10 with 1184 XP
	// (SkillManager.ts:53-63) — not the 1154 setSkills floor.
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("Hitpoints current level = %d, want 10", got)
	}
	if got := sm.MaxLevel(SkillHitpoints); got != 10 {
		t.Errorf("Hitpoints max level = %d, want 10", got)
	}
	if got := sm.Experience(SkillHitpoints); got != 1184 {
		t.Errorf("Hitpoints experience = %d, want 1184", got)
	}
	if got := sm.TotalLevel(); got != 33 {
		t.Errorf("TotalLevel = %d, want 33", got)
	}
	if got := sm.TotalExperience(); got != 1184 {
		t.Errorf("TotalExperience = %d, want 1184", got)
	}
	if got := sm.CombatLevel(); got != 3 {
		t.Errorf("CombatLevel = %d, want 3", got)
	}
}

func TestExpArrayShape(t *testing.T) {
	if len(ExpArray) != 99 {
		t.Fatalf("ExpArray length = %d, want 99", len(ExpArray))
	}
	if ExpArray[0] != 0 || ExpArray[9] != 1154 || ExpArray[98] != ExperienceFor99 {
		t.Errorf("ExpArray endpoints = %d/%d/%d, want 0/1154/13034431",
			ExpArray[0], ExpArray[9], ExpArray[98])
	}
	for i := 1; i < len(ExpArray); i++ {
		if ExpArray[i] <= ExpArray[i-1] {
			t.Errorf("ExpArray not strictly increasing at %d: %d <= %d", i, ExpArray[i], ExpArray[i-1])
		}
	}
	if ExperienceFor99 != 13034431 {
		t.Errorf("ExperienceFor99 = %d, want 13034431", ExperienceFor99)
	}
	if MaxExperience != 200000000 {
		t.Errorf("MaxExperience = %d, want 200000000", MaxExperience)
	}
}

func TestExperienceForLevelVectors(t *testing.T) {
	cases := []struct {
		level, want int
	}{
		{1, 0}, {2, 83}, {3, 174}, {10, 1154}, {11, 1358},
		{50, 101333}, {98, 11805606}, {99, 13034431},
		// Above 99 the TS summation loop applies the standard
		// formula (terms 1..level-1): level 100 is 14,391,160.
		{100, 14391160},
		// Levels <= 0: TS returns undefined; Go returns 0 (the
		// setExperience sanitize equivalent).
		{0, 0}, {-5, 0},
	}
	for _, tc := range cases {
		if got := ExperienceForLevel(tc.level); got != tc.want {
			t.Errorf("ExperienceForLevel(%d) = %d, want %d", tc.level, got, tc.want)
		}
	}
}

func TestLevelForExperienceVectors(t *testing.T) {
	cases := []struct {
		xp, want int
	}{
		{0, 1}, {82, 1}, {83, 2}, {174, 3}, {1153, 9}, {1154, 10},
		{1358, 11}, {101333, 50}, {13034430, 98}, {13034431, 99},
		{13034432, 99}, {200000000, 99},
		// TS-faithful fall-through quirk: negative XP reaches the
		// final `return 99` (SkillManager.ts:82-100). Unreachable
		// through sanitized callers; pinned so the port stays
		// identical to TS and to bootstrap.go's copy.
		{-1, 99},
	}
	for _, tc := range cases {
		if got := LevelForExperience(tc.xp); got != tc.want {
			t.Errorf("LevelForExperience(%d) = %d, want %d", tc.xp, got, tc.want)
		}
	}
}

func TestLevelExperienceRoundTrip(t *testing.T) {
	for level := 1; level <= 99; level++ {
		xp := ExperienceForLevel(level)
		if got := LevelForExperience(xp); got != level {
			t.Errorf("LevelForExperience(ExperienceForLevel(%d)=%d) = %d", level, xp, got)
		}
		if got := LevelForExperience(ExpArray[level-1]); got != level {
			t.Errorf("LevelForExperience(ExpArray[%d]) = %d, want %d", level-1, got, level)
		}
	}
}

func TestCombatLevelVectors(t *testing.T) {
	fresh := make([]int, SkillCount)
	for i := range fresh {
		fresh[i] = 1
	}
	fresh[SkillHitpoints] = 10

	cases := []struct {
		name   string
		levels []int
		want   int
	}{
		{"fresh character", fresh, 3},
		{"maxed", func() []int {
			l := make([]int, SkillCount)
			for i := range l {
				l[i] = 99
			}
			return l
		}(), 126},
		{"ranged pure", func() []int {
			l := append([]int(nil), fresh...)
			l[SkillRanged] = 99
			return l
		}(), 50},
		{"magic pure", func() []int {
			l := append([]int(nil), fresh...)
			l[SkillMagic] = 99
			return l
		}(), 50},
		{"melee mid", func() []int {
			l := append([]int(nil), fresh...)
			l[SkillAttack], l[SkillStrength], l[SkillDefence], l[SkillHitpoints] = 60, 60, 60, 60
			return l
		}(), 69},
		// Prayer halving: floor(43/2) = 21 in the base term.
		{"prayer 43", func() []int {
			l := append([]int(nil), fresh...)
			l[SkillDefence], l[SkillHitpoints], l[SkillPrayer] = 40, 50, 43
			return l
		}(), 28},
		// The clamp floor: all 1s (even Hitpoints 1) still yields 3.
		{"all ones", func() []int {
			l := make([]int, SkillCount)
			for i := range l {
				l[i] = 1
			}
			return l
		}(), 3},
	}
	for _, tc := range cases {
		if got := CalculateCombatLevel(tc.levels); got != tc.want {
			t.Errorf("%s: CalculateCombatLevel = %d, want %d", tc.name, got, tc.want)
		}
	}
	// CombatLevel() reads the RAW maxLevel array: a skill whose XP
	// implies 99 but whose raw base is 1 does not raise combat.
	_, sm := newTestSkillManager()
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	if got := sm.CombatLevel(); got != 3 {
		t.Errorf("CombatLevel with XP-only Attack 99 = %d, want 3 (raw array)", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 99 {
		t.Errorf("MaxLevel with XP for 99 = %d, want 99 (XP floor)", got)
	}
}

func TestAddExperienceNoLevelUp(t *testing.T) {
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	p.Flags = p.Flags.Reset()
	sm.AddExperience(SkillAttack, 82)
	if got := sm.Experience(SkillAttack); got != 82 {
		t.Errorf("XP = %d, want 82", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 1 {
		t.Errorf("max level = %d, want 1", got)
	}
	if len(rec.updates) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(rec.updates))
	}
	u := rec.updates[0]
	if u.Skill != SkillAttack || u.CurrentLevel != 1 || u.MaxLevel != 1 || u.Experience != 82 ||
		u.TotalLevel != 33 || u.CombatLevel != 3 {
		t.Errorf("hook update = %+v, want skill 0, 1/1, 82 XP, totals 33/3", u)
	}
	if p.Flags.Has(FlagAppearance) {
		t.Error("non-level-up gain flagged APPEARANCE")
	}
}

func TestAddExperienceLevelsUp(t *testing.T) {
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	p.Flags = p.Flags.Reset()
	sm.AddExperience(SkillAttack, 83)
	if got := sm.Experience(SkillAttack); got != 83 {
		t.Errorf("XP = %d, want 83", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 2 {
		t.Errorf("max level = %d, want 2", got)
	}
	if got := sm.CurrentLevel(SkillAttack); got != 2 {
		t.Errorf("current level = %d, want 2", got)
	}
	if len(rec.updates) != 1 {
		t.Fatalf("hook calls = %d, want 1 (via setCurrentLevels refresh)", len(rec.updates))
	}
	if u := rec.updates[0]; u.CurrentLevel != 2 || u.MaxLevel != 2 || u.TotalLevel != 34 {
		t.Errorf("level-up hook update = %+v, want current/max 2, total 34", u)
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("level-up did not flag APPEARANCE")
	}
}

func TestAddExperienceKeepsBoostAndDrain(t *testing.T) {
	_, sm := newTestSkillManager()
	// Boosted: current 5 over base 1 rises by the levels gained.
	sm.SetCurrentLevel(SkillAttack, 5, false)
	sm.AddExperience(SkillAttack, 83)
	if got := sm.CurrentLevel(SkillAttack); got != 6 {
		t.Errorf("boosted current = %d, want 6", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 2 {
		t.Errorf("max = %d, want 2", got)
	}
	// Drained: current 0 rises by the levels gained, restoring
	// nothing else.
	_, sm2 := newTestSkillManager()
	sm2.SetCurrentLevel(SkillAttack, 0, false)
	sm2.AddExperience(SkillAttack, 83)
	if got := sm2.CurrentLevel(SkillAttack); got != 1 {
		t.Errorf("drained current = %d, want 1", got)
	}
}

func TestAddExperienceMultiLevelJump(t *testing.T) {
	_, sm := newTestSkillManager()
	sm.AddExperience(SkillAttack, ExperienceFor99)
	if got := sm.MaxLevel(SkillAttack); got != 99 {
		t.Errorf("max = %d, want 99", got)
	}
	if got := sm.CurrentLevel(SkillAttack); got != 99 {
		t.Errorf("current = %d, want 99 (1 + 98 gained)", got)
	}
}

func TestAddExperienceCapsAtMax(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.SetExperiences(SkillAttack, MaxExperience-10, false)
	sm.AddExperience(SkillAttack, 100)
	if got := sm.Experience(SkillAttack); got != MaxExperience {
		t.Errorf("XP = %d, want %d", got, MaxExperience)
	}
	calls := len(rec.updates)
	// At max XP, further gains return before any send.
	sm.AddExperience(SkillAttack, 100)
	if len(rec.updates) != calls {
		t.Errorf("hook calls grew at max XP: %d -> %d", calls, len(rec.updates))
	}
	if got := sm.Experience(SkillAttack); got != MaxExperience {
		t.Errorf("XP after capped gain = %d, want %d", got, MaxExperience)
	}
}

func TestAddExperienceIgnoresInvalid(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.AddExperience(SkillAttack, 0)
	sm.AddExperience(SkillAttack, -50)
	sm.AddExperience(-1, 100)
	sm.AddExperience(SkillCount, 100)
	if got := sm.Experience(SkillAttack); got != 0 {
		t.Errorf("XP = %d, want 0", got)
	}
	if len(rec.updates) != 0 {
		t.Errorf("hook calls = %d, want 0", len(rec.updates))
	}
}

func TestAddExperienceXPLocked(t *testing.T) {
	// The XP lock (SkillManager.ts:156): a gain is dropped when
	// the player's skills:xp-locked attribute is strictly true.
	// The locked return sits before the max-exp sanitize and
	// before any send, so XP, levels, and hook calls are all
	// untouched.
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	p.Flags = p.Flags.Reset()
	p.SetAttribute(XPLockedAttribute, true)
	sm.AddExperience(SkillAttack, ExperienceFor99)
	if got := sm.Experience(SkillAttack); got != 0 {
		t.Errorf("locked XP = %d, want 0", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 1 {
		t.Errorf("locked max level = %d, want 1", got)
	}
	if len(rec.updates) != 0 {
		t.Errorf("locked hook calls = %d, want 0", len(rec.updates))
	}
	if p.Flags.Has(FlagAppearance) {
		t.Error("locked gain flagged APPEARANCE")
	}
	// Unlocking (deleting the key) resumes gains.
	p.SetAttribute(XPLockedAttribute, nil)
	sm.AddExperience(SkillAttack, 83)
	if got := sm.Experience(SkillAttack); got != 83 {
		t.Errorf("unlocked XP = %d, want 83", got)
	}
	if got := sm.MaxLevel(SkillAttack); got != 2 {
		t.Errorf("unlocked max level = %d, want 2", got)
	}
	// The lock also blocks gains that would not level up, and
	// applies through AddExperienceWithMultipliers with
	// multipliers=false (TS checks the attribute after the rate
	// computation, on every path).
	p.SetAttribute(XPLockedAttribute, true)
	sm.AddExperienceWithMultipliers(SkillCooking, 100, false)
	if got := sm.Experience(SkillCooking); got != 0 {
		t.Errorf("locked multipliers=false XP = %d, want 0", got)
	}
}

func TestAddExperienceXPLockStrictTrue(t *testing.T) {
	// TS checks `=== true`: only the boolean true locks. Other
	// truthy values (1, "true") do not, and false does not.
	for _, value := range []any{1, "true", false, 0} {
		p, sm := newTestSkillManager()
		p.SetAttribute(XPLockedAttribute, value)
		sm.AddExperience(SkillAttack, 82)
		if got := sm.Experience(SkillAttack); got != 82 {
			t.Errorf("attribute %#v: XP = %d, want 82 (not locked)", value, got)
		}
	}
	// A bare manager (nil player) is never locked.
	sm := NewSkillManager(nil)
	sm.AddExperience(SkillAttack, 82)
	if got := sm.Experience(SkillAttack); got != 82 {
		t.Errorf("nil-player XP = %d, want 82", got)
	}
}

func TestAddExperienceMultipliersIdentity(t *testing.T) {
	// With the default inert seam (no gate hooks) and the
	// default world rate 1, both multiplier branches land the
	// full amount; the rate and gate wiring is covered by the
	// skill_hooks tests.
	_, sm := newTestSkillManager()
	sm.AddExperienceWithMultipliers(SkillAttack, 100, false)
	if got := sm.Experience(SkillAttack); got != 100 {
		t.Errorf("multipliers=false XP = %d, want 100", got)
	}
	_, sm2 := newTestSkillManager()
	sm2.AddExperiences(SkillAttack, 100)
	if got := sm2.Experience(SkillAttack); got != 100 {
		t.Errorf("AddExperiences XP = %d, want 100", got)
	}
}

func TestAddExperienceSanitizesStoredXP(t *testing.T) {
	_, sm := newTestSkillManager()
	// A negative stored value (reachable only by corrupting the
	// array directly) is sanitized to 0 before the gain applies.
	sm.experience[SkillAttack] = -500
	sm.AddExperience(SkillAttack, 100)
	if got := sm.Experience(SkillAttack); got != 100 {
		t.Errorf("XP = %d, want 100", got)
	}
}

func TestSetSkillsNormalization(t *testing.T) {
	_, sm := newTestSkillManager()
	// All-zero arrays: Hitpoints floors apply; other skills keep
	// current 0 (max(0, raw)) with base 1.
	zeros := make([]int, SkillCount)
	sm.SetSkills(zeros, zeros, zeros)
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("HP current = %d, want 10", got)
	}
	if got := sm.MaxLevel(SkillHitpoints); got != 10 {
		t.Errorf("HP max = %d, want 10", got)
	}
	if got := sm.Experience(SkillHitpoints); got != 1154 {
		t.Errorf("HP XP = %d, want 1154 (setSkills floor, not 1184)", got)
	}
	if got := sm.CurrentLevel(SkillAttack); got != 0 {
		t.Errorf("Attack current = %d, want 0", got)
	}
	// Nil slices: every entry missing -> fresh-Skills shape with
	// the 1154 HP floor; current rests at base.
	sm.SetSkills(nil, nil, nil)
	if got := sm.CurrentLevel(SkillAttack); got != 1 {
		t.Errorf("nil-slice Attack current = %d, want 1", got)
	}
	if got := sm.Experience(SkillHitpoints); got != 1154 {
		t.Errorf("nil-slice HP XP = %d, want 1154", got)
	}
	// XP-implied floor: base is at least the XP-implied level.
	_, sm2 := newTestSkillManager()
	xp := make([]int, SkillCount)
	max := make([]int, SkillCount)
	lvl := make([]int, SkillCount)
	for i := range xp {
		lvl[i], max[i] = 1, 1
	}
	xp[SkillAttack] = ExperienceFor99
	sm2.SetSkills(lvl, max, xp)
	if got := sm2.MaxLevel(SkillAttack); got != 99 {
		t.Errorf("XP-floored max = %d, want 99", got)
	}
	// SetSkills STORES the floor: normalization writes
	// max(maxFromExp, rawMax) into the array (unlike MaxLevel's
	// computed floor after a raw SetMaxLevels write, pinned in
	// TestMaxLevelXPFloorNotWrittenBack).
	if got := sm2.GetMaxLevels(SkillAttack); got != 99 {
		t.Errorf("raw max after SetSkills = %d, want 99 (floor stored)", got)
	}
	// Negative current clamps to 0 (non-HP); HP current floors at 10.
	lvl[SkillAttack] = -5
	lvl[SkillHitpoints] = 5
	max[SkillHitpoints] = 5
	xp[SkillHitpoints] = 100
	sm2.SetSkills(lvl, max, xp)
	if got := sm2.CurrentLevel(SkillAttack); got != 0 {
		t.Errorf("negative current = %d, want 0", got)
	}
	if got := sm2.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("HP current floor = %d, want 10", got)
	}
	if got := sm2.Experience(SkillHitpoints); got != 1154 {
		t.Errorf("HP XP floor = %d, want 1154", got)
	}
	// HP XP above the floor is kept verbatim.
	xp[SkillHitpoints] = 1184
	sm2.SetSkills(lvl, max, xp)
	if got := sm2.Experience(SkillHitpoints); got != 1184 {
		t.Errorf("HP XP = %d, want 1184", got)
	}
	// Over-long arrays: entries past SkillCount are ignored.
	long := make([]int, SkillCount+6)
	for i := range long {
		long[i] = 50
	}
	sm2.SetSkills(long, long, long)
	if got := sm2.CurrentLevel(SkillSailing); got != 50 {
		t.Errorf("Sailing current = %d, want 50", got)
	}
}

func TestSetSkillsSendsNothing(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.SetSkills(nil, nil, nil)
	if len(rec.updates) != 0 {
		t.Errorf("SetSkills sent %d updates, want 0", len(rec.updates))
	}
}

func TestMaxLevelXPFloorNotWrittenBack(t *testing.T) {
	_, sm := newTestSkillManager()
	sm.SetMaxLevels(SkillAttack, 1, false)
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	if got := sm.MaxLevel(SkillAttack); got != 99 {
		t.Errorf("MaxLevel = %d, want 99", got)
	}
	// No write-back: the raw array still holds 1 after MaxLevel ran.
	if got := sm.GetMaxLevels(SkillAttack); got != 1 {
		t.Errorf("raw max after MaxLevel = %d, want 1", got)
	}
}

func TestCurrentLevelNegativeSanitize(t *testing.T) {
	// SkillManager.ts:425-433: a negative level is written back as
	// 0 and 0 is returned. The run-energy drain depends on this
	// exact behavior; the white-box write simulates a corrupt
	// stored value, since every setter clamps negatives on write.
	_, sm := newTestSkillManager()
	sm.level[SkillAgility] = -3
	if got := sm.CurrentLevel(SkillAgility); got != 0 {
		t.Errorf("CurrentLevel = %d, want 0", got)
	}
	if sm.level[SkillAgility] != 0 {
		t.Errorf("stored level = %d, want 0 (write-back)", sm.level[SkillAgility])
	}
}

func TestExperienceSanitizeWriteBack(t *testing.T) {
	_, sm := newTestSkillManager()
	sm.experience[SkillAttack] = MaxExperience + 50000000
	if got := sm.Experience(SkillAttack); got != MaxExperience {
		t.Errorf("Experience = %d, want %d", got, MaxExperience)
	}
	if sm.experience[SkillAttack] != MaxExperience {
		t.Errorf("stored XP = %d, want %d (write-back)", sm.experience[SkillAttack], MaxExperience)
	}
}

func TestTotalLevelUsesRawMax(t *testing.T) {
	_, sm := newTestSkillManager()
	// Raw base 1 with XP for 99: TotalLevel counts 1 (raw), while
	// MaxLevel computes 99. Fresh total is 33; the XP-only skill
	// must not change it.
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	if got := sm.TotalLevel(); got != 33 {
		t.Errorf("TotalLevel = %d, want 33 (raw sum)", got)
	}
	sm.SetMaxLevels(SkillAttack, 50, false)
	if got := sm.TotalLevel(); got != 82 {
		t.Errorf("TotalLevel = %d, want 82", got)
	}
}

func TestDrainsRestoresBoosts(t *testing.T) {
	_, sm := newTestSkillManager()
	// HP base 10. An increase never lowers a level at/past the cap.
	sm.IncreaseCurrentLevel(SkillHitpoints, 5, 10)
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("increase at cap = %d, want 10", got)
	}
	sm.SetCurrentLevel(SkillHitpoints, 5, false)
	sm.IncreaseCurrentLevel(SkillHitpoints, 3, 10)
	if got := sm.CurrentLevel(SkillHitpoints); got != 8 {
		t.Errorf("restore = %d, want 8", got)
	}
	sm.IncreaseCurrentLevel(SkillHitpoints, 5, 10)
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("restore capped = %d, want 10", got)
	}
	// IncreaseCurrentLevelMax caps at base + amount.
	sm.SetCurrentLevel(SkillHitpoints, 5, false)
	sm.IncreaseCurrentLevelMax(SkillHitpoints, 3)
	if got := sm.CurrentLevel(SkillHitpoints); got != 8 {
		t.Errorf("increase-max = %d, want 8", got)
	}
	// Decrease floors at the minimum.
	sm.DecreaseCurrentLevel(SkillHitpoints, 20, 0)
	if got := sm.CurrentLevel(SkillHitpoints); got != 0 {
		t.Errorf("decrease floor = %d, want 0", got)
	}
	sm.SetCurrentLevel(SkillHitpoints, 10, false)
	sm.DecreaseCurrentLevel(SkillHitpoints, 3, 0)
	if got := sm.CurrentLevel(SkillHitpoints); got != 7 {
		t.Errorf("decrease = %d, want 7", got)
	}
	// DecreaseLevelMax floors at base - amount.
	sm.SetCurrentLevel(SkillHitpoints, 10, false)
	sm.DecreaseLevelMax(SkillHitpoints, 2)
	if got := sm.CurrentLevel(SkillHitpoints); got != 8 {
		t.Errorf("decrease-max = %d, want 8", got)
	}
	// Boosted means current above base.
	sm.SetCurrentLevel(SkillHitpoints, 11, false)
	if !sm.IsBoosted(SkillHitpoints) {
		t.Error("IsBoosted = false at 11/10, want true")
	}
	sm.SetCurrentLevel(SkillHitpoints, 10, false)
	if sm.IsBoosted(SkillHitpoints) {
		t.Error("IsBoosted = true at 10/10, want false")
	}
}

func TestRestingLevels(t *testing.T) {
	_, sm := newTestSkillManager()
	if got := sm.RestingLevel(SkillAttack); got != sm.MaxLevel(SkillAttack) {
		t.Errorf("RestingLevel = %d, want base %d (no modifiers)", got, sm.MaxLevel(SkillAttack))
	}
	// A registered modifier raises the resting level; one that
	// lowers it is clamped to the base. Restore the registry so
	// other tests see zero modifiers.
	saved := restingLevelModifiers
	defer func() { restingLevelModifiers = saved }()
	RegisterRestingLevelModifier(func(player *Player, skill, level int) int { return level + 5 })
	if got := sm.RestingLevel(SkillAttack); got != 6 {
		t.Errorf("RestingLevel with +5 modifier = %d, want 6", got)
	}
	RegisterRestingLevelModifier(func(player *Player, skill, level int) int { return 0 })
	if got := sm.RestingLevel(SkillAttack); got != 1 {
		t.Errorf("RestingLevel with lowering modifier = %d, want base 1", got)
	}
	RegisterRestingLevelModifier(nil)
	if got := len(restingLevelModifiers); got != len(saved)+2 {
		t.Errorf("nil modifier registered: %d modifiers", got)
	}
}

func TestLevelCaps(t *testing.T) {
	_, sm := newTestSkillManager()
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	cap50 := 50
	sm.SetMaxLevelCap(SkillAttack, &cap50)
	if got := sm.MaxLevel(SkillAttack); got != 50 {
		t.Errorf("capped MaxLevel = %d, want 50", got)
	}
	if got := sm.GetMaxLevels(SkillAttack); got != 1 {
		t.Errorf("raw max under cap = %d, want 1", got)
	}
	// Capping below the current level lowers the current level.
	sm.SetCurrentLevel(SkillAttack, 80, false)
	sm.SetMaxLevelCap(SkillAttack, &cap50)
	if got := sm.CurrentLevel(SkillAttack); got != 50 {
		t.Errorf("current after cap = %d, want 50", got)
	}
	// Clearing the cap restores the computed level.
	sm.SetMaxLevelCap(SkillAttack, nil)
	if got := sm.MaxLevel(SkillAttack); got != 99 {
		t.Errorf("MaxLevel after clear = %d, want 99", got)
	}
	// A cap below 1 floors at 1.
	cap0 := 0
	sm.SetMaxLevelCap(SkillAttack, &cap0)
	if got := sm.MaxLevel(SkillAttack); got != 1 {
		t.Errorf("cap-0 MaxLevel = %d, want 1", got)
	}
}

func TestSetLevel(t *testing.T) {
	p, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	p.Flags = p.Flags.Reset()
	sm.SetLevel(SkillAttack, 50)
	if got := sm.CurrentLevel(SkillAttack); got != 50 {
		t.Errorf("current = %d, want 50", got)
	}
	if got := sm.GetMaxLevels(SkillAttack); got != 50 {
		t.Errorf("raw max = %d, want 50", got)
	}
	if got := sm.Experience(SkillAttack); got != ExperienceForLevel(50) {
		t.Errorf("XP = %d, want %d", got, ExperienceForLevel(50))
	}
	// TS sends twice: setExperience refreshes, then setLevel calls
	// updateSkill again (SkillManager.ts:236-282).
	if len(rec.updates) != 2 {
		t.Errorf("hook calls = %d, want 2 (TS double send)", len(rec.updates))
	}
	if !p.Flags.Has(FlagAppearance) {
		t.Error("SetLevel did not flag APPEARANCE")
	}
	// Hitpoints below 10 is refused (the refusal message is
	// pinned by TestSetLevelHitpointsFloorSendsMessage; the
	// return is ported here).
	rec.updates = nil
	sm.SetLevel(SkillHitpoints, 5)
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("HP current = %d, want 10 (refused)", got)
	}
	if len(rec.updates) != 0 {
		t.Errorf("refused SetLevel sent %d updates", len(rec.updates))
	}
}

func TestSettersRefresh(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.SetCurrentLevel(SkillAttack, 5, true)
	sm.SetCurrentLevel(SkillAttack, 6, false)
	sm.SetMaxLevels(SkillAttack, 7, true)
	sm.SetMaxLevels(SkillAttack, 8, false)
	sm.SetExperiences(SkillAttack, 100, true)
	sm.SetExperiences(SkillAttack, 200, false)
	sm.SetCurrentLevelCombat(SkillAttack, 9)
	sm.SetMaxLevel(SkillAttack, 10)
	sm.SetExperience(SkillAttack, 300)
	sm.SetCurrentLevels(SkillAttack, 11)
	if len(rec.updates) != 7 {
		t.Fatalf("hook calls = %d, want 7 (refresh=true variants only)", len(rec.updates))
	}
	if got := sm.CurrentLevel(SkillAttack); got != 11 {
		t.Errorf("current = %d, want 11", got)
	}
	if got := sm.GetMaxLevels(SkillAttack); got != 10 {
		t.Errorf("raw max = %d, want 10", got)
	}
	if got := sm.Experience(SkillAttack); got != 300 {
		t.Errorf("XP = %d, want 300", got)
	}
	// SetExperiences sanitizes on write.
	sm.SetExperiences(SkillAttack, -5, false)
	if got := sm.Experience(SkillAttack); got != 0 {
		t.Errorf("negative XP stored = %d, want 0", got)
	}
	sm.SetExperiences(SkillAttack, MaxExperience+1, false)
	if got := sm.Experience(SkillAttack); got != MaxExperience {
		t.Errorf("over-cap XP stored = %d, want %d", got, MaxExperience)
	}
}

func TestUpdateSkillHookValues(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	sm.SetCurrentLevel(SkillAttack, 50, false)
	sm.UpdateSkill(SkillAttack)
	if len(rec.updates) != 1 {
		t.Fatalf("hook calls = %d, want 1", len(rec.updates))
	}
	u := rec.updates[0]
	// Computed MaxLevel (XP floor), floored XP, RAW total (33:
	// the raw base array is unchanged), computed combat (3).
	if u.Skill != SkillAttack || u.CurrentLevel != 50 || u.MaxLevel != 99 ||
		u.Experience != ExperienceFor99 || u.TotalLevel != 33 || u.CombatLevel != 3 {
		t.Errorf("hook update = %+v, want 0/50/99/%d/33/3", u, ExperienceFor99)
	}
	// No hook installed: no panic, no send.
	_, sm2 := newTestSkillManager()
	sm2.UpdateSkill(SkillAttack)
}

func TestSnapshotDefensiveCopies(t *testing.T) {
	_, sm := newTestSkillManager()
	sm.SetExperiences(SkillAttack, ExperienceFor99, false)
	level, maxLevel, xp := sm.Snapshot()
	if len(level) != SkillCount || len(maxLevel) != SkillCount || len(xp) != SkillCount {
		t.Fatalf("snapshot lengths = %d/%d/%d, want %d", len(level), len(maxLevel), len(xp), SkillCount)
	}
	// Raw values, as fromPlayer clones them: raw max 1, XP for 99.
	if maxLevel[SkillAttack] != 1 || xp[SkillAttack] != ExperienceFor99 {
		t.Errorf("snapshot Attack = max %d xp %d, want 1/%d", maxLevel[SkillAttack], xp[SkillAttack], ExperienceFor99)
	}
	level[SkillAttack] = 99
	maxLevel[SkillAttack] = 99
	xp[SkillAttack] = 0
	if got := sm.CurrentLevel(SkillAttack); got != 1 {
		t.Errorf("mutating snapshot changed live current to %d", got)
	}
	if got := sm.Experience(SkillAttack); got != ExperienceFor99 {
		t.Errorf("mutating snapshot changed live XP to %d", got)
	}
}

func TestEnsureCombatBaseline(t *testing.T) {
	_, sm := newTestSkillManager()
	zeros := make([]int, SkillCount)
	sm.SetSkills(zeros, zeros, zeros)
	sm.EnsureCombatBaseline()
	// Hitpoints at least 10 with at least 1184 XP (the baseline
	// uses 1184, not the 1154 setSkills floor).
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Errorf("HP current = %d, want 10", got)
	}
	if got := sm.Experience(SkillHitpoints); got != 1184 {
		t.Errorf("HP XP = %d, want 1184", got)
	}
	// Base combat skills raised to at least 1; non-combat skills
	// keep their normalized current level (0 from the zero arrays).
	if got := sm.CurrentLevel(SkillAttack); got != 1 {
		t.Errorf("Attack current = %d, want 1", got)
	}
	if got := sm.CurrentLevel(SkillCooking); got != 0 {
		t.Errorf("Cooking current = %d, want 0", got)
	}
	// White-box corruption is normalized: negative XP -> 0, base
	// below the XP-implied level -> the implied level.
	sm.experience[SkillCooking] = -50
	sm.maxLevel[SkillCooking] = 0
	sm.EnsureCombatBaseline()
	if got := sm.Experience(SkillCooking); got != 0 {
		t.Errorf("Cooking XP = %d, want 0", got)
	}
	if got := sm.GetMaxLevels(SkillCooking); got != 1 {
		t.Errorf("Cooking raw max = %d, want 1", got)
	}
}

func TestStartStopSkillable(t *testing.T) {
	_, sm := newTestSkillManager()
	if sm.StartSkillables() {
		t.Error("StartSkillables = true, want false (TS always returns false)")
	}
	// Empty slot: state unchanged, no panic. The live slot
	// lifecycle is covered by the skill_hooks tests.
	sm.StopSkillable()
	if got := sm.TotalLevel(); got != 33 {
		t.Errorf("TotalLevel after StopSkillable = %d, want 33", got)
	}
}

func TestInvalidSkillIndices(t *testing.T) {
	_, sm := newTestSkillManager()
	rec := &hookRecorder{}
	sm.SetUpdateHook(rec.hook)
	if got := sm.CurrentLevel(-1); got != 0 {
		t.Errorf("CurrentLevel(-1) = %d, want 0", got)
	}
	if got := sm.MaxLevel(SkillCount); got != 0 {
		t.Errorf("MaxLevel(24) = %d, want 0", got)
	}
	if got := sm.Experience(99); got != 0 {
		t.Errorf("Experience(99) = %d, want 0", got)
	}
	if got := sm.RestingLevel(-1); got != 0 {
		t.Errorf("RestingLevel(-1) = %d, want 0", got)
	}
	if sm.IsBoosted(SkillCount) {
		t.Error("IsBoosted(24) = true, want false")
	}
	sm.UpdateSkill(-1)
	sm.SetLevel(99, 50)
	sm.SetCurrentLevel(-1, 50, true)
	sm.IncreaseCurrentLevel(SkillCount, 5, 10)
	sm.DecreaseCurrentLevel(-1, 5, 0)
	if len(rec.updates) != 0 {
		t.Errorf("invalid indices sent %d updates", len(rec.updates))
	}
	if got := sm.TotalLevel(); got != 33 {
		t.Errorf("TotalLevel = %d, want 33", got)
	}
}
