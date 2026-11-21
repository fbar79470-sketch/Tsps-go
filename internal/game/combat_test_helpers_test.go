package game

import (
	"testing"
)

// Hand-built combat fixtures. No cache, no network, no JSON data.

// combatTestItemDefs is a hand-built ItemDefinitionSource with
// bonus and weapon-info reads for combat tests.
type combatTestItemDefs struct {
	bonuses      map[int][]float64
	attackSpeeds map[int]int
	attackRanges map[int]int
	interfaces   map[int]string
	weights      map[int]int
	slots        map[int]int
	attackAnims  map[int]map[string]int
	blockAnims   map[int]int
}

func newCombatTestItemDefs() *combatTestItemDefs {
	return &combatTestItemDefs{
		bonuses:      map[int][]float64{},
		attackSpeeds: map[int]int{},
		attackRanges: map[int]int{},
		interfaces:   map[int]string{},
		weights:      map[int]int{},
		slots:        map[int]int{},
		attackAnims:  map[int]map[string]int{},
		blockAnims:   map[int]int{},
	}
}

func (s *combatTestItemDefs) IsStackable(id int) bool      { return false }
func (s *combatTestItemDefs) IsNoted(id int) bool          { return false }
func (s *combatTestItemDefs) WeightGrams(id int) int       { return s.weights[id] }
func (s *combatTestItemDefs) EquipmentSlot(id int) int     { return s.slots[id] }
func (s *combatTestItemDefs) IsDoubleHanded(id int) bool   { return false }
func (s *combatTestItemDefs) Requirements(id int) []int    { return nil }
func (s *combatTestItemDefs) CombatRequirement(id int) int { return 0 }
func (s *combatTestItemDefs) IsTradeable(id int) bool      { return true }
func (s *combatTestItemDefs) IsSellable(id int) bool       { return true }
func (s *combatTestItemDefs) IsDropable(id int) bool       { return true }
func (s *combatTestItemDefs) UnNote(id int) int            { return id }
func (s *combatTestItemDefs) PlaceholderID(id int) int     { return -1 }
func (s *combatTestItemDefs) ItemBonuses(id int) []float64 {
	if b, ok := s.bonuses[id]; ok {
		return b
	}
	return make([]float64, 14)
}
func (s *combatTestItemDefs) WeaponAttackSpeed(id int) int {
	if v, ok := s.attackSpeeds[id]; ok {
		return v
	}
	return 0
}
func (s *combatTestItemDefs) WeaponAttackRange(id int) int {
	if v, ok := s.attackRanges[id]; ok {
		return v
	}
	return 0
}
func (s *combatTestItemDefs) WeaponAttackAnim(id int, attackType string) int {
	if byType, ok := s.attackAnims[id]; ok {
		if v, ok := byType[attackType]; ok {
			return v
		}
	}
	return -1
}
func (s *combatTestItemDefs) WeaponBlockAnim(id int) int {
	if v, ok := s.blockAnims[id]; ok {
		return v
	}
	return 424
}
func (s *combatTestItemDefs) WeaponInterfaceName(id int) string { return s.interfaces[id] }

// combatFakeDef is a hand-built NPCCombatDefinition: the driver
// fake plus the combat getters. Defaults mirror the DEFAULT
// shape (level-1 stats, max hit 1, attack speed 4, death
// ticks 2, fights back, attackable).
type combatFakeDef struct {
	*fakeNPCDefinition
	stats          [18]int
	maxHit         int
	attackSpeed    int
	attackAnim     int
	defenceAnim    int
	deathAnim      int
	deathTicks     int
	projectileID   int
	fightBack      bool
	attackable     bool
	retreat        bool
	poisonous      bool
	venomous       bool
	poisonImmune   bool
	venomImmune    bool
	freezeResist   int
	demon          bool
	rangedDefBonus int
}

func newCombatFakeDef() *combatFakeDef {
	return &combatFakeDef{
		fakeNPCDefinition: defaultFakeDef(),
		stats:             [18]int{1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		maxHit:            1,
		attackSpeed:       4,
		attackAnim:        422,
		defenceAnim:       -1,
		deathAnim:         836,
		deathTicks:        2,
		projectileID:      -1,
		fightBack:         true,
		attackable:        true,
	}
}

func (d *combatFakeDef) IsAttackable() bool    { return d.attackable }
func (d *combatFakeDef) DoesRetreat() bool     { return d.retreat }
func (d *combatFakeDef) DoesFightBack() bool   { return d.fightBack }
func (d *combatFakeDef) IsPoisonous() bool     { return d.poisonous }
func (d *combatFakeDef) IsVenomous() bool      { return d.venomous }
func (d *combatFakeDef) IsPoisonImmune() bool  { return d.poisonImmune }
func (d *combatFakeDef) IsVenomImmune() bool   { return d.venomImmune }
func (d *combatFakeDef) FreezeResistance() int { return d.freezeResist }
func (d *combatFakeDef) IsDemon() bool         { return d.demon }
func (d *combatFakeDef) MaxHit() int           { return d.maxHit }
func (d *combatFakeDef) AttackSpeed() int      { return d.attackSpeed }
func (d *combatFakeDef) AttackAnim() int       { return d.attackAnim }
func (d *combatFakeDef) DefenceAnim() int      { return d.defenceAnim }
func (d *combatFakeDef) DeathAnim() int        { return d.deathAnim }
func (d *combatFakeDef) DeathTicks() int       { return d.deathTicks }
func (d *combatFakeDef) Stats() []int          { return d.stats[:] }
func (d *combatFakeDef) ProjectileID() int     { return d.projectileID }
func (d *combatFakeDef) RangedDefenceBonus(kind string) int {
	if d.rangedDefBonus != 0 {
		return d.rangedDefBonus
	}
	return d.stats[14]
}

var _ NPCCombatDefinition = (*combatFakeDef)(nil)

// combatTestPlayer builds a registered, bound player with a
// recording fake session at (x, y). Levels default to 1
// (10 hitpoints); tests set what they need.
func combatTestPlayer(t *testing.T, name string, x, y int) (*Player, *fakeSession) {
	t.Helper()
	p := NewPlayer(name, Location{X: x, Y: y, Z: 0})
	p.SetRegistered(true)
	fs := newFakeSession()
	p.Session = fs
	p.GetCombat() // bind identity
	return p, fs
}

// combatTestNPC builds a registered, bound NPC with the given
// definition at (x, y), at full hitpoints.
func combatTestNPC(t *testing.T, id, x, y int, def *combatFakeDef) *NPC {
	t.Helper()
	n := NewNPC(id, Location{X: x, Y: y, Z: 0})
	n.SetRegistered(true)
	if def == nil {
		def = newCombatFakeDef()
	}
	n.SetDefinition(def)
	n.Hitpoints = def.Hitpoints()
	n.GetCombat() // bind identity + coordinator seams
	return n
}

// setCombatCycle installs a fixed cycle source for the test,
// returning a function that advances it. The source is
// restored on cleanup.
func setCombatCycle(t *testing.T, start int64) func(int64) {
	t.Helper()
	cycle := start
	SetCombatCycleSource(func() int64 { return cycle })
	t.Cleanup(func() {
		SetCombatCycleSource(nil)
		lastHitQueueCycle = 0
	})
	return func(next int64) { cycle = next }
}

// setFixedRandom overrides the combat RNG with a fixed
// inclusive roll (0 = minimum rolls, max = maximum rolls)
// and a fixed float, restoring both on cleanup.
func setFixedRandom(t *testing.T, roll func(max int) int, floatVal float64) {
	t.Helper()
	prevRoll, prevFloat := randomInclusive, randomFloat
	randomInclusive = roll
	randomFloat = func() float64 { return floatVal }
	t.Cleanup(func() {
		randomInclusive, randomFloat = prevRoll, prevFloat
	})
}

// resetCombatHooks clears the hook table for the test,
// restoring the previous table on cleanup.
func resetCombatHooks(t *testing.T) {
	t.Helper()
	prev := Hooks
	Hooks = CombatHooks{}
	t.Cleanup(func() { Hooks = prev })
}

// setPlayerLevels sets current and max levels for the given
// skills (map skill -> level).
func setPlayerLevels(p *Player, levels map[int]int) {
	for skill, level := range levels {
		p.Skills().SetMaxLevel(skill, level)
		p.Skills().SetCurrentLevel(skill, level, false)
	}
}

// fakeSpell is a hand-built CombatSpell for method/formula tests.
type fakeSpell struct {
	maximumHit    int
	baseXP        int
	splashXP      int
	attackSpeed   int
	hitDelay      int
	radius        int
	canCastResult bool
	canCastTarget bool
	startCasts    int
	finishCasts   int
	lastFinish    *fakeFinishArgs
}

type fakeFinishArgs struct {
	accurate bool
	damage   int
}

func newFakeSpell() *fakeSpell {
	return &fakeSpell{
		maximumHit:    10,
		baseXP:        20,
		splashXP:      5,
		attackSpeed:   5,
		hitDelay:      -1,
		canCastResult: true,
		canCastTarget: true,
	}
}

func (s *fakeSpell) SpellID() int                              { return 1152 }
func (s *fakeSpell) MaximumHit() int                           { return s.maximumHit }
func (s *fakeSpell) BaseExperience() int                       { return s.baseXP }
func (s *fakeSpell) SplashExperience() int                     { return s.splashXP }
func (s *fakeSpell) AttackSpeed() int                          { return s.attackSpeed }
func (s *fakeSpell) HitDelay() int                             { return s.hitDelay }
func (s *fakeSpell) CanCast(caster *Player, consume bool) bool { return s.canCastResult }
func (s *fakeSpell) CanCastOnTarget(caster *Player, target *Mobile) bool {
	return s.canCastTarget
}
func (s *fakeSpell) StartCast(caster *Mobile, target *Mobile) { s.startCasts++ }
func (s *fakeSpell) FinishCast(attacker, target *Mobile, accurate bool, damage int) {
	s.finishCasts++
	s.lastFinish = &fakeFinishArgs{accurate: accurate, damage: damage}
}
func (s *fakeSpell) SpellRadius() int { return s.radius }
