package game

// Weapon special attacks: the core mechanism. TS splits specials
// into CombatSpecial.ts (the registry: definitions, energy, the
// bar) and WeaponSpecialTraits.ts (the roll overrides a special
// applies). Individual specials are CONTENT — registered by
// plugins against item ids — so core defines the mechanism and
// the trait computation, registers nothing, and treats an
// unregistered weapon as ordinary. The boundary: CombatSpecial
// definitions, trait values, and special combat methods are
// content; cost/drain/energy bookkeeping, trait application in
// the roll pipeline, and the targeting resolver seam are core.

// SpecialMaximumHitSource is SpecialAttackMaximumHitSource
// (WeaponSpecialTraits.ts): which formula a special's max hit
// is computed from, when maximumHitSource is set.
type SpecialMaximumHitSource string

const (
	SpecialMaxHitStandard      SpecialMaximumHitSource = "standard"
	SpecialMaxHitPhysicalMelee SpecialMaximumHitSource = "physical_melee"
	SpecialMaxHitRanged        SpecialMaximumHitSource = "ranged"
	SpecialMaxHitMagic         SpecialMaximumHitSource = "magic"
	SpecialMaxHitVisibleMagic  SpecialMaximumHitSource = "visible_magic"
)

// ResolveSpecialAttackType is resolveSpecialAttackType
// (WeaponSpecialTraits.ts): a trait's damage type may be named
// "melee"/"ranged"/"magic" or given as a CombatType. Unknown
// values answer CombatTypeMelee (the TS fallback is the
// attack's own type; callers pass that as the default — see
// WeaponSpecialTraits.ResolveDamageType).
func ResolveSpecialAttackType(value string) (CombatType, bool) {
	switch value {
	case "melee":
		return CombatTypeMelee, true
	case "ranged":
		return CombatTypeRanged, true
	case "magic":
		return CombatTypeMagic, true
	default:
		return CombatTypeMelee, false
	}
}

// WeaponSpecialTraits is WeaponSpecialTraits
// (WeaponSpecialTraits.ts): every field optional; a nil pointer
// means "not set" and the ordinary computation applies. Slice
// fields are nil when unset. The zero value is an inert trait
// set: no overrides.
type WeaponSpecialTraits struct {
	// HitCount overrides the number of hits (PendingHit
	// hitAmount). HitDelayTicks overrides the per-hit reveal
	// delays in ticks. AttackSpeedTicks overrides the attack
	// speed. BypassAttackDelay skips the cooldown for this
	// attack.
	HitCount          *int
	HitDelayTicks     []int
	AttackSpeedTicks  *int
	BypassAttackDelay bool

	// AccuracyMultiplier scales the attack roll (a decimal:
	// 1.5 = +50%). AccuracyMultiplierStages is the staged form:
	// each entry a decimal multiplier applied in order with
	// floor rounding (AccuracyFormulasDpsCalc.applyAccuracyTraits).
	AccuracyMultiplier       *float64
	AccuracyMultiplierStages []float64
	// RollAttackType overrides the attack-roll type
	// (melee/ranged/magic) used for accuracy.
	RollAttackType *CombatType
	// DefenceRollAttackType overrides the defence-roll type.
	DefenceRollAttackType *CombatType
	// DefenceRollMultiplier scales the defence roll (a
	// decimal, floored: AccuracyFormulasDpsCalc.specialRolls).
	DefenceRollMultiplier *float64
	// MeleeAttackBonusIndex / MeleeDefenceBonusIndex override
	// the bonus index (0 stab, 1 slash, 2 crush) for melee
	// rolls.
	MeleeAttackBonusIndex  *int
	MeleeDefenceBonusIndex *int
	// AccuracyRollCount rolls accuracy N times and takes the
	// best (accuracyRollCount). SharedAccuracyRollAcrossHits
	// shares one accuracy roll across a multi-hit.
	AccuracyRollCount            *int
	SharedAccuracyRollAcrossHits bool
	// GuaranteedHit forces accuracy. GuaranteedFirstAccuracyRoll
	// forces only the first roll accurate.
	// GuaranteedHitOnUnsuspectingTarget forces accuracy when
	// the target is not in combat with the attacker.
	GuaranteedHit                     bool
	GuaranteedFirstAccuracyRoll       bool
	GuaranteedHitOnUnsuspectingTarget bool
	// FixedAccuracyRollMultiplierWhenTargetAtOrBelowMaximumDamage
	// is the fixed-accuracy multiplier (a decimal fraction of
	// the attack roll) applied when the target's hitpoints are
	// at or below MaximumDamage (rollFixedAccuracy).
	FixedAccuracyRollMultiplierWhenTargetAtOrBelowMaximumDamage *float64

	// DamageMultiplier scales damage (a decimal: 1.3 = +30%).
	// AttackLevelMultiplier / StrengthLevelMultiplier scale
	// the visible attack/strength levels before the prayer
	// adjustment (decimals: floor(level * multiplier)).
	// DamageMultiplierStages is the staged form (decimals,
	// scaleSpecial rounding per stage unless the matching
	// DamageMultiplierStageRounding entry is "ceil").
	DamageMultiplier        *float64
	AttackLevelMultiplier   *float64
	StrengthLevelMultiplier *float64
	DamageMultiplierStages  []float64
	// DamageMultiplierStageRounding holds one entry per
	// stage: "ceil" rounds that stage up, anything else
	// (including a missing entry) uses scaleSpecial rounding.
	DamageMultiplierStageRounding []string
	// MaximumHitSource overrides the max-hit formula source.
	MaximumHitSource *SpecialMaximumHitSource
	// MaxHitOverride replaces the max hit outright.
	MaxHitOverride *int
	// VisibleMagicMaximumHit overrides the visible magic max
	// hit (the number the client-facing formula shows).
	VisibleMagicMaximumHit *int
	// MinimumDamageMultiplier / MaximumDamageMultiplier scale
	// the max hit into the damage range ends (decimals, as in
	// specialDamageRange). MinimumDamageBonus /
	// MaximumDamageBonus add flat damage to the range ends.
	// MaximumDamageCap caps the range maximum.
	// MaximumHitReductionOnFullAccuracyRolls reduces the max
	// hit per fully-accurate roll (dragon claws family) — a
	// flat amount, applied by PendingHit's multi-hit path.
	MinimumDamageMultiplier                *float64
	MaximumDamageMultiplier                *float64
	MinimumDamageBonus                     *int
	MaximumDamageBonus                     *int
	MaximumDamageCap                       *int
	MaximumHitReductionOnFullAccuracyRolls *int
	// DamageRangeBySuccessfulAccuracyRolls maps a count of
	// successful accuracy rolls to a damage range (index 0 is
	// the one-success range): the range's multipliers win
	// over the trait-level ones (PendingHit.rollSpecialTraits).
	DamageRangeBySuccessfulAccuracyRolls []AccuracyDamageRange
	// FirstSuccessfulAccuracyDamageRanges is the
	// first-successful-accuracy branch table (dragon claws
	// family): each entry is attempted in order until one
	// lands, then its damage splits across HitCount hits
	// (PendingHit.resolveFirstSuccessfulRanges).
	FirstSuccessfulAccuracyDamageRanges []FirstSuccessfulAccuracyRange
	// AllMissDamagePatterns lists the damage patterns applied
	// when every accuracy roll misses (one pattern per hit).
	AllMissDamagePatterns [][]int

	// DamageType overrides the damage/protection type
	// ("melee"/"ranged"/"magic").
	DamageType string
	// IgnoreProtectionPrayer bypasses protection-prayer
	// reduction for this attack.
	IgnoreProtectionPrayer bool
	// SkipAttack makes the special consume its activation
	// without attacking (a queued-attack no-op).
	SkipAttack bool
	// EnchantedBoltEffectChanceMultiplier scales the
	// enchanted-bolt effect chance (a decimal);
	// GuaranteedEnchantedBoltEffect forces it. Bolt effects
	// themselves are content; core only carries the fields.
	EnchantedBoltEffectChanceMultiplier *float64
	GuaranteedEnchantedBoltEffect       bool
	// Targeting names the special's targeting mode
	// ("single", "area", "footprint"); see
	// SpecialAttackTargeting.
	Targeting string
	// QueuedAttack marks a special that fires from the queued
	// (next-attack) slot rather than immediately.
	QueuedAttack bool
}

// ResolveDamageType returns the trait's damage type, or the
// attack's own type when the trait sets none
// (WeaponSpecialTraits resolveSpecialAttackType usage in
// PendingHit.protectionType / CombatFactory.rollHitDamage).
func (t *WeaponSpecialTraits) ResolveDamageType(fallback CombatType) CombatType {
	if t == nil || t.DamageType == "" {
		return fallback
	}
	if ct, ok := ResolveSpecialAttackType(t.DamageType); ok {
		return ct
	}
	return fallback
}

// AccuracyDamageRange is one entry of
// damageRangeBySuccessfulAccuracyRolls: the damage-range
// multipliers for a given count of successful accuracy rolls.
// Nil multipliers fall back to the trait-level ones.
type AccuracyDamageRange struct {
	MinimumDamageMultiplier *float64
	MaximumDamageMultiplier *float64
}

// FirstSuccessfulAccuracyRange is one entry of
// firstSuccessfulAccuracyDamageRanges: the damage range for
// the attempt that lands, how its damage splits across hits,
// and the reduction applied to the rolled base.
type FirstSuccessfulAccuracyRange struct {
	MinimumDamageMultiplier *float64
	MaximumDamageMultiplier *float64
	// MaximumDamageReduction subtracts flat damage from the
	// rolled base (the bounds reduction).
	MaximumDamageReduction int
	// DistributeDamage, when set, splits the rolled base
	// across hitCount hits; otherwise HitDamageMultipliers /
	// HitDamageBonuses shape each hit.
	DistributeDamage func(base, hitCount int) []int
	// HitDamageMultipliers / HitDamageBonuses shape hit i:
	// floor(base * multiplier[i]) + bonus[i] (missing
	// entries read 0).
	HitDamageMultipliers []float64
	HitDamageBonuses     []int
}

// SpecialDamageBounds is the bounds override a hit carries into
// CombatFactory.specialDamageRange (SpecialDamageBounds in
// CombatFactory.ts / PendingHit.ts): fractional multipliers of
// the max hit, flat bonuses, a cap, a flat reduction, and an
// optional maximum split across hits. All fields optional; nil
// / zero means "not set". It is NOT the resulting {min, max}
// range — specialDamageRange returns that pair.
type SpecialDamageBounds struct {
	// MinimumMultiplier / MaximumMultiplier scale the max hit
	// into the range ends (fractions: 1.0 = the max hit).
	MinimumMultiplier *float64
	MaximumMultiplier *float64
	// MinimumBonus / MaximumBonus add flat damage to the ends.
	MinimumBonus *int
	MaximumBonus *int
	// Cap caps the maximum (and the minimum with it).
	Cap *int
	// Reduction subtracts flat damage from the maximum.
	Reduction int
	// MaximumSplit splits the maximum across Count hits;
	// Index selects this hit's share (0-based).
	MaximumSplit *MaximumSplit
}

// MaximumSplit is the maximumSplit half of SpecialDamageBounds.
type MaximumSplit struct {
	Count int
	Index int
}

// SpecialAttackTargeting is the targeting mode of a special
// attack (SpecialAttackTargetingResolver.ts): single-target
// (the default), area (a radius around the primary target), or
// footprint (every tile of a large target). Core resolves the
// mode through ActiveTargetingFor and lists secondary targets
// through the Hooks.AreaAttackTargets seam; the resolver's
// tile math needs world lists core does not have.
type SpecialAttackTargeting struct {
	// Mode is "single", "area", or "footprint".
	Mode string
	// Radius is the area radius in tiles (area mode).
	Radius int
	// AccuracyScale scales secondary-target accuracy
	// (default 1, Combat.ts executeAttack). It is also the
	// large-target extra hit's accuracy scale.
	AccuracyScale float64
	// LargeTargetExtraHit: the special strikes a second hit
	// on a large primary target instead of secondaries
	// (SpecialAttackTargetingResolver largeTargetExtraHit).
	LargeTargetExtraHit bool
}

// CombatSpecial is one registered special attack definition
// (CombatSpecial.ts): the item ids that trigger it, its energy
// cost, its multipliers, its combat method, and its traits.
// Content registers definitions; core reads them through
// ActiveTraitsFor / ActiveTargetingFor.
type CombatSpecial struct {
	// ID is the registry id (e.g. "dragon_dagger").
	ID string
	// ItemIDs are the weapon item ids that carry this special.
	ItemIDs []int
	// DrainAmount is the energy cost in percent (0..100).
	DrainAmount int
	// DrainAmountByItemID overrides the cost per item id.
	DrainAmountByItemID map[int]int
	// StrengthMultiplier / AccuracyMultiplier are the legacy
	// multipliers (decimals: 1.5 = +50%), used when Traits
	// sets neither the staged nor the direct equivalents.
	StrengthMultiplier *float64
	AccuracyMultiplier *float64
	// Method is the special's combat method, or nil for the
	// weapon's ordinary method.
	Method CombatMethod
	// Traits are the roll overrides, or nil.
	Traits *WeaponSpecialTraits
	// FireSounds are the special's own fire sounds, played
	// instead of the weapon's ordinary attack/fire sound on a
	// special swing (SpecialFireSoundsFor, combat_sounds.go).
	// TS carries them on the special's combat method: a ranged
	// special overrides fireSound() (RangedCombatMethod.ts:
	// 38-40 base returns undefined; :182-183 plays
	// `this.fireSound(character) ?? profileSound ??
	// Sound.SHOOT_ARROW`, so the special's sound wins over
	// the profile's), and a melee special overrides start()
	// to play its sound instead of getAttackSound
	// (MeleeCombatMethod.ts:9-14). The plugin methods
	// themselves are not in the sparse checkout, so the
	// definition carries the sounds the method would own.
	// Most specials carry one sound (Sound.ts special
	// statics, e.g. DRAGON_DAGGER_SPECIAL 2537); multi-hit
	// specials carry the sequence — dragon claws 1-4,
	// burning claws 1-2, emberlight 1-3, voidwaker 1-2 —
	// each Sound already carrying its client-cycle delay
	// (Sound.ts:121-124, 140-141, 158-163), so sending them
	// all at swing start reproduces the TS timing. Empty
	// means the special has no own sound and the ordinary
	// sound plays, exactly as a TS method that does not
	// override fireSound falls through to the profile sound.
	FireSounds []Sound
	// TraitsFor, when set, computes per-player traits
	// (CombatSpecial.traitsFor).
	TraitsFor func(player *Player) *WeaponSpecialTraits
	// Targeting is the special's targeting mode, or nil for
	// single-target.
	Targeting *SpecialAttackTargeting
	// TargetingFor, when set, computes per-player targeting.
	TargetingFor func(player *Player) *SpecialAttackTargeting
	// Metadata carries content-defined values (CombatSpecial
	// metadata); core never reads it.
	Metadata map[string]any
}

var combatSpecialsByID = map[string]*CombatSpecial{}
var combatSpecialsByItem = map[int]*CombatSpecial{}

// RegisterCombatSpecial registers a special attack definition
// (CombatSpecial.ts register). Content (via the hub) registers
// definitions; registering the same id twice replaces it, and
// the last registration wins per item id, matching the TS
// Map.set behavior. Register before ticking starts.
func RegisterCombatSpecial(special *CombatSpecial) {
	if special == nil {
		return
	}
	combatSpecialsByID[special.ID] = special
	for _, id := range special.ItemIDs {
		combatSpecialsByItem[id] = special
	}
}

// CombatSpecialForItem returns the special registered for a
// weapon item id, or nil.
func CombatSpecialForItem(itemID int) *CombatSpecial {
	return combatSpecialsByItem[itemID]
}

// CombatSpecialByID returns the special registered under id,
// or nil.
func CombatSpecialByID(id string) *CombatSpecial {
	return combatSpecialsByID[id]
}

// Cost returns the energy cost of this special for player:
// the plugin cost event (Hooks.SpecialAttackCost) wins, then
// the per-item override, then the drain amount
// (CombatSpecial.cost).
func (s *CombatSpecial) Cost(player *Player, itemID int) int {
	if s == nil {
		return 0
	}
	if Hooks.SpecialAttackCost != nil {
		if cost := Hooks.SpecialAttackCost(player, s); cost != nil {
			return *cost
		}
	}
	if s.DrainAmountByItemID != nil {
		if cost, ok := s.DrainAmountByItemID[itemID]; ok {
			return cost
		}
	}
	return s.DrainAmount
}

// ActiveTraitsFor returns the active special traits for a
// fighter (CombatSpecial.activeTraitsFor): players only, only
// when a special is activated, from traitsFor(player) or
// traits. NPCs and inactive players answer nil.
func ActiveTraitsFor(c Combatant) *WeaponSpecialTraits {
	if c == nil || !c.IsPlayer() {
		return nil
	}
	combat := c.Mobile().GetCombat()
	special := combat.GetCombatSpecial()
	if special == nil {
		return nil
	}
	if special.TraitsFor != nil {
		if t := special.TraitsFor(c.Player()); t != nil {
			return t
		}
	}
	return special.Traits
}

// ActiveTargetingFor returns the active special targeting for
// a fighter (CombatSpecial.activeTargetingFor), with the same
// gating as ActiveTraitsFor.
func ActiveTargetingFor(c Combatant) *SpecialAttackTargeting {
	if c == nil || !c.IsPlayer() {
		return nil
	}
	combat := c.Mobile().GetCombat()
	special := combat.GetCombatSpecial()
	if special == nil {
		return nil
	}
	if special.TargetingFor != nil {
		if t := special.TargetingFor(c.Player()); t != nil {
			return t
		}
	}
	return special.Targeting
}
