package game

import "math"

// The hit pipeline: HitMask, HitDamage, PendingHit, and
// HitQueue — the port of game/content/combat/hit/HitMask.ts,
// HitDamage.ts (175 lines), PendingHit.ts (697 lines), and
// HitQueue.ts (245 lines). A PendingHit rolls its damage at
// construction (accuracy + damage, capped to the target's
// hitpoints at roll time); the HitQueue schedules its impact
// for a reveal cycle, applies protection prayers at impact,
// and lands the damage through decrementHealth. Defensive
// reactions (protection, Elysian, incoming modifiers) are
// deliberately NOT applied at roll time: they are read when
// the hit lands, so a last-tick prayer flick works.

// HitMask is HitMask (hit/HitMask.ts): the splat color family.
type HitMask int

const (
	HitMaskBlue HitMask = iota
	HitMaskRed
	HitMaskGreen
	HitMaskYellow
)

// HitDamageOrigin is HitDamageOrigin (HitDamage.ts): where a
// hit queued straight to the target came from, past the NPC
// hit-modify hooks. "" for every other hit.
type HitDamageOrigin string

const (
	HitDamageOriginThrall    HitDamageOrigin = "thrall"
	HitDamageOriginRecoil    HitDamageOrigin = "recoil"
	HitDamageOriginVengeance HitDamageOrigin = "vengeance"
)

// HitDamage is HitDamage (hit/HitDamage.ts): one rolled
// hitsplat. Damage is always whole (floored on every write);
// the mask follows the damage (a positive hit shows its start
// mask, BLUE start masks show RED; a zero hit is BLUE).
type HitDamage struct {
	damage        int
	mask          HitMask
	startMask     HitMask
	rolledMaximum int
	creditsSource bool
	reflected     bool
	origin        HitDamageOrigin
	source        *Mobile
	splatMine     int
	splatOthers   int
	hasSplatTypes bool
	healthLost    int
	observers     []func(damageTaken int)
}

// NewHitDamage returns a HitDamage with damage and mask,
// normalized through update (HitDamage constructor).
func NewHitDamage(damage int, mask HitMask) *HitDamage {
	h := &HitDamage{damage: damage, mask: mask, startMask: mask}
	h.update()
	return h
}

// Damage returns the damage.
func (h *HitDamage) Damage() int { return h.damage }

// SetDamage sets the damage, normalizing through update.
func (h *HitDamage) SetDamage(damage int) {
	h.damage = damage
	h.update()
}

// IncrementDamage adds damage, normalizing through update.
func (h *HitDamage) IncrementDamage(damage int) {
	h.damage += damage
	h.update()
}

// MultiplyDamage scales the damage, rounding down as OSRS
// does (a 17 into a protection prayer in PvP is 10).
func (h *HitDamage) MultiplyDamage(mod float64) {
	h.damage = int(math.Floor(float64(h.damage) * mod))
	h.update()
}

// update normalizes damage and mask: damage is floored (and
// non-finite reads 0 — impossible for Go ints, kept for
// shape); a positive hit shows its start mask (BLUE start
// masks show RED); a zero hit is BLUE with damage 0.
func (h *HitDamage) update() {
	if h.damage < 0 {
		h.damage = 0
	}
	if h.damage > 0 {
		if h.startMask == HitMaskBlue {
			h.mask = HitMaskRed
		} else {
			h.mask = h.startMask
		}
	} else {
		h.damage = 0
		h.mask = HitMaskBlue
	}
}

// Mask returns the current hitmask.
func (h *HitDamage) Mask() HitMask { return h.mask }

// SetMask sets the hitmask directly (no update).
func (h *HitDamage) SetMask(mask HitMask) { h.mask = mask }

// HealthLost returns the health actually lost when this hit
// landed, recorded before reactive healing or death-phase
// transitions.
func (h *HitDamage) HealthLost() int { return h.healthLost }

// SetHealthLost records health lost (floored at 0).
func (h *HitDamage) SetHealthLost(amount int) {
	if amount < 0 {
		amount = 0
	}
	h.healthLost = amount
}

// OnApplied observes health actually lost when this hit lands.
// Observers are consumed by NotifyApplied even if one panics
// (TS catches observer errors).
func (h *HitDamage) OnApplied(observer func(damageTaken int)) *HitDamage {
	if observer != nil {
		h.observers = append(h.observers, observer)
	}
	return h
}

// NotifyApplied runs the applied observers with damageTaken
// and consumes them, recovering from observer panics.
func (h *HitDamage) NotifyApplied(damageTaken int) {
	observers := h.observers
	h.observers = nil
	for _, observer := range observers {
		func() {
			defer func() { _ = recover() }()
			observer(damageTaken)
		}()
	}
}

// SetSplatTypes records cache hitsplat ids replacing the
// hitmask's: one for the target, one for everyone else.
func (h *HitDamage) SetSplatTypes(mine, others int) *HitDamage {
	h.splatMine, h.splatOthers, h.hasSplatTypes = mine, others, true
	return h
}

// SplatType returns the splat override for a viewer (mine =
// the target or the dealer), or -1 when none is set.
func (h *HitDamage) SplatType(mine bool) int {
	if !h.hasSplatTypes {
		return -1
	}
	if mine {
		return h.splatMine
	}
	return h.splatOthers
}

// SetRolledMaximum records the top of the roll this hit came
// from (not for fixed, reflected, poison, or thrall damage).
func (h *HitDamage) SetRolledMaximum(maximum int) *HitDamage {
	if maximum < 0 {
		maximum = 0
	}
	h.rolledMaximum = maximum
	return h
}

// RolledMaximum returns the recorded roll top.
func (h *HitDamage) RolledMaximum() int { return h.rolledMaximum }

// IsMaxHit reports whether the hit still equals its recorded
// roll top (and that top is positive).
func (h *HitDamage) IsMaxHit() bool {
	return h.rolledMaximum > 0 && h.damage == h.rolledMaximum
}

// MarkCreditsSource marks the hit as counting as its source's
// for the kill (League Thorns damage).
func (h *HitDamage) MarkCreditsSource() *HitDamage {
	h.creditsSource = true
	return h
}

// CreditsItsSource reports the credits-source mark.
func (h *HitDamage) CreditsItsSource() bool { return h.creditsSource }

// MarkReflected marks rebounded damage (recoil, vengeance,
// retribution); a death from it doesn't trigger Retribution.
func (h *HitDamage) MarkReflected() *HitDamage {
	h.reflected = true
	return h
}

// IsReflected reports the reflected mark.
func (h *HitDamage) IsReflected() bool { return h.reflected }

// SetOrigin records where a directly-queued hit came from.
func (h *HitDamage) SetOrigin(origin HitDamageOrigin) *HitDamage {
	h.origin = origin
	return h
}

// Origin returns the hit's origin ("" for ordinary hits).
func (h *HitDamage) Origin() HitDamageOrigin { return h.origin }

// SetSource records who dealt the hit (the dealer sees it as
// their own hitsplat).
func (h *HitDamage) SetSource(source *Mobile) *HitDamage {
	h.source = source
	return h
}

// Source returns who dealt the hit, or nil.
func (h *HitDamage) Source() *Mobile { return h.source }

// HitDamageCache is HitDamageCache (hit/HitDamageCache.ts):
// one damager's accumulated damage plus a stopwatch reset on
// every increment. The stopwatch is cycle-stamped (TS
// TickStopwatch.elapsed() = (currentCycle - tick) * 600ms).
type HitDamageCache struct {
	damage    int
	lastCycle int64
}

// NewHitDamageCache returns a cache with damage and the
// current cycle stamp.
func NewHitDamageCache(damage int, cycle int64) *HitDamageCache {
	return &HitDamageCache{damage: damage, lastCycle: cycle}
}

// Damage returns the accumulated damage.
func (c *HitDamageCache) Damage() int { return c.damage }

// Increment adds damage and resets the stopwatch.
func (c *HitDamageCache) Increment(damage int, cycle int64) {
	c.damage += damage
	c.lastCycle = cycle
}

// ElapsedTicks returns the ticks since the last increment.
func (c *HitDamageCache) ElapsedTicks(cycle int64) int64 {
	return cycle - c.lastCycle
}

// FiredRangedShot is the FiredRangedShot snapshot
// (PendingHit.ts): a player's ranged shot as it was fired —
// weapon/ammo copies, the weapon interface, and the
// projectile item (ammo when the shot fires from the ammo
// slot, else the weapon). Nil items mean the slot held
// nothing valid.
type FiredRangedShot struct {
	Weapon          *Item
	Ammo            *Item
	WeaponInterface string
	Projectile      *Item
}

// PendingHitConfig is PendingHitConfig (PendingHit.ts): the
// constructor options. Zero values mean the TS defaults:
// delay 0, after-hit effects on, 1 hit, accuracy rolled, XP
// rewarded, best-of 1, damage multiplier 1, not ordinary.
type PendingHitConfig struct {
	Delay                  int
	HandleAfterHitEffects  *bool
	HitAmount              int
	HitDelays              []int
	HitMaximumSplitCount   int
	RequireFirstAccurate   bool
	RollAccuracy           *bool
	Experience             *bool
	ExperienceSkills       []int
	Damage                 *int // fixed damage (never a max hit)
	StatusDamage           bool // fixed, no rolls, no XP, protection bypassed
	BypassProtectionPrayer bool
	DamageBudget           *int // shared remaining budget (capDamageToBudget)
	BestOf                 int
	DamageMultiplier       float64
	Ordinary               bool // rolled as if no special were armed
	SpellContext           any
	WithoutHitsplat        bool
}

// PendingHit is PendingHit (hit/PendingHit.ts): a rolled
// attack — one to four HitDamage splats, rolled at
// construction — plus the metadata the queue and the factory
// need to land it.
type PendingHit struct {
	attacker                  *Mobile
	target                    *Mobile
	method                    CombatMethod
	combatType                CombatType
	hits                      []*HitDamage
	totalDamage               int
	delay                     int
	accurate                  bool
	failedAccuracyChecks      int
	handleAfterHitEffects     bool
	experience                bool
	experienceSkills          []int
	damageBudget              *int
	bestOf                    int
	damageMultiplier          float64
	ordinary                  bool
	hitDelays                 []int
	hitMaximumSplitCount      int
	requireFirstAccurate      bool
	spell                     CombatSpell
	spellContext              any
	protectionBypassed        bool
	protectionType            CombatType
	impactProtectionApplied   bool
	impactIncomingApplied     bool
	incomingDamageDeferred    bool
	firstSuccessfulRangeIndex *int
	firedRangedShot           *FiredRangedShot
	withoutHitsplat           bool
}

// NewPendingHit rolls a hit with a plain tick delay
// (PendingHit constructor, number form).
func NewPendingHit(attacker, target *Mobile, method CombatMethod, delay int) *PendingHit {
	return NewPendingHitConfig(attacker, target, method, PendingHitConfig{Delay: delay})
}

// NewPendingHitConfig rolls a hit from a config
// (PendingHit constructor, object form). The rolls happen
// inside DuringHitRoll, so defensive reactions defer to
// impact. An ordinary hit lowers the attacker's armed
// special for the rolls and restores it after.
func NewPendingHitConfig(attacker, target *Mobile, method CombatMethod, config PendingHitConfig) *PendingHit {
	p := &PendingHit{
		attacker:              attacker,
		target:                target,
		method:                method,
		handleAfterHitEffects: true,
		experience:            true,
		bestOf:                1,
		damageMultiplier:      1,
		hitMaximumSplitCount:  1,
		spellContext:          config.SpellContext,
		withoutHitsplat:       config.WithoutHitsplat,
	}
	if method != nil {
		p.combatType = method.Type()
	}
	p.protectionType = p.combatType
	p.captureFiredRangedShot()

	resolvedDelay := config.Delay
	resolvedHandleAfterHitEffects := true
	resolvedHitAmount := 1
	resolvedRollAccuracy := true
	if config.HandleAfterHitEffects != nil {
		resolvedHandleAfterHitEffects = *config.HandleAfterHitEffects
	}
	if config.HitAmount > 0 {
		resolvedHitAmount = config.HitAmount
	}
	if config.RollAccuracy != nil {
		resolvedRollAccuracy = *config.RollAccuracy
	}
	if config.Experience != nil && !*config.Experience {
		p.experience = false
	}
	p.experienceSkills = append([]int(nil), config.ExperienceSkills...)
	p.damageBudget = config.DamageBudget
	if len(config.HitDelays) > 0 {
		p.hitDelays = make([]int, len(config.HitDelays))
		for i, tick := range config.HitDelays {
			if tick < 0 {
				tick = 0
			}
			p.hitDelays[i] = tick
		}
	}
	if config.HitMaximumSplitCount > 1 {
		p.hitMaximumSplitCount = config.HitMaximumSplitCount
	}
	p.requireFirstAccurate = config.RequireFirstAccurate
	if config.BestOf > 1 {
		p.bestOf = config.BestOf
	}
	if config.DamageMultiplier != 0 {
		p.damageMultiplier = config.DamageMultiplier
	}
	p.ordinary = config.Ordinary

	// An ordinary hit is rolled as if no special were armed:
	// lower the flag for the rolls, restore it after (TS
	// also clears the hit-roll cache; Go has no cache).
	var attackerCombat *Combat
	armed := false
	if p.ordinary && attacker != nil {
		attackerCombat = attacker.GetCombat()
		armed = attackerCombat.IsSpecialActivated()
		if armed {
			attackerCombat.setSpecialActivated(false)
		}
	}
	if config.StatusDamage {
		amount := 0
		if config.Damage != nil {
			amount = *config.Damage
		}
		hp := 0
		if target != nil {
			hp = combatantFor(target).Hitpoints()
		}
		if amount < 0 {
			amount = 0
		}
		if amount > hp {
			amount = hp
		}
		p.hits = []*HitDamage{NewHitDamage(amount, HitMaskRed)}
		p.accurate = true
		p.totalDamage = amount
		p.experience = false
		p.protectionBypassed = true
	} else {
		p.hits = DuringHitRoll(func() []*HitDamage {
			return p.prepareHits(resolvedHitAmount, resolvedRollAccuracy, config.Damage, config.BypassProtectionPrayer)
		})
	}
	if armed {
		attackerCombat.setSpecialActivated(true)
	}
	if resolvedDelay < 0 {
		resolvedDelay = 0
	}
	p.delay = resolvedDelay
	p.handleAfterHitEffects = resolvedHandleAfterHitEffects
	return p
}

// captureFiredRangedShot snapshots a player attacker's ranged
// shot at construction (PendingHit constructor): weapon/ammo
// copies, the weapon interface name, and the projectile item.
func (p *PendingHit) captureFiredRangedShot() {
	if p.combatType != CombatTypeRanged || p.attacker == nil {
		return
	}
	attacker := combatantFor(p.attacker)
	if !attacker.IsPlayer() || attacker.Equipment() == nil {
		return
	}
	copySlot := func(slot int) *Item {
		item := attacker.Equipment().ItemAt(slot)
		if !item.IsValid() {
			return nil
		}
		copied := item
		return &copied
	}
	weapon := copySlot(WeaponSlot)
	ammo := copySlot(AmmunitionSlot)
	weaponInterface := ""
	if src, ok := attacker.ItemDefinitions().(WeaponInfoSource); ok && src != nil && weapon != nil {
		weaponInterface = src.WeaponInterfaceName(weapon.ID())
	}
	projectile := weapon
	if rangedFiresFromAmmoSlot(attacker.Player()) {
		projectile = ammo
	}
	p.firedRangedShot = &FiredRangedShot{
		Weapon:          weapon,
		Ammo:            ammo,
		WeaponInterface: weaponInterface,
		Projectile:      projectile,
	}
}

// FiredRangedShot returns the captured shot, or nil.
func (p *PendingHit) FiredRangedShot() *FiredRangedShot { return p.firedRangedShot }

// Attacker returns the hit's attacker.
func (p *PendingHit) Attacker() *Mobile { return p.attacker }

// Target returns the hit's target.
func (p *PendingHit) Target() *Mobile { return p.target }

// CombatMethod returns the hit's method.
func (p *PendingHit) CombatMethod() CombatMethod { return p.method }

// Spell returns the spell selected when the hit was launched.
func (p *PendingHit) Spell() CombatSpell { return p.spell }

// SetSpell sets the hit's spell (the magic method calls it).
func (p *PendingHit) SetSpell(spell CombatSpell) *PendingHit {
	p.spell = spell
	return p
}

// SpellContext returns the cast context, if any.
func (p *PendingHit) SpellContext() any { return p.spellContext }

// Hits returns the rolled splats.
func (p *PendingHit) Hits() []*HitDamage { return p.hits }

// Delay returns the hit delay in ticks.
func (p *PendingHit) Delay() int { return p.delay }

// TotalDamage returns the total rolled damage (capped).
func (p *PendingHit) TotalDamage() int { return p.totalDamage }

// SetTotalDamage sets the total (used by hit-modify hooks).
func (p *PendingHit) SetTotalDamage(damage int) { p.totalDamage = damage }

// UpdateTotalDamage recomputes the total from the splats.
func (p *PendingHit) UpdateTotalDamage() {
	p.totalDamage = 0
	for _, hit := range p.hits {
		p.totalDamage += hit.Damage()
	}
}

// CapDamageToBudget caps the splats to the shared remaining
// budget, in order, decrementing it (PendingHit
// capDamageToBudget). No budget means no cap.
func (p *PendingHit) CapDamageToBudget() {
	if p.damageBudget == nil {
		return
	}
	for _, damage := range p.hits {
		value := damage.Damage()
		if value > *p.damageBudget {
			value = *p.damageBudget
		}
		if value < 0 {
			value = 0
		}
		damage.SetDamage(value)
		*p.damageBudget -= value
	}
	p.UpdateTotalDamage()
}

// RewardsExperience reports whether the attacker earns XP.
func (p *PendingHit) RewardsExperience() bool { return p.experience }

// FirstSuccessfulRangeIndex returns the
// first-successful-accuracy branch that landed (-1 when every
// roll missed), or nil when the branch table did not run.
func (p *PendingHit) FirstSuccessfulRangeIndex() *int { return p.firstSuccessfulRangeIndex }

// IsAccurate reports whether the hit landed (any splat, for
// multi-hits).
func (p *PendingHit) IsAccurate() bool { return p.accurate }

// FailedAccuracyChecks returns the count of failed own
// accuracy rolls.
func (p *PendingHit) FailedAccuracyChecks() int { return p.failedAccuracyChecks }

// BypassesProtectionPrayer reports the protection bypass.
func (p *PendingHit) BypassesProtectionPrayer() bool { return p.protectionBypassed }

// MarkProtectionBypassed sets the protection bypass.
func (p *PendingHit) MarkProtectionBypassed() { p.protectionBypassed = true }

// ProtectionType returns the attack kind whose protection
// prayer is checked when the hit lands.
func (p *PendingHit) ProtectionType() CombatType { return p.protectionType }

// HasAppliedImpactProtection reports the impact-protection
// flag (prevents double application on replay).
func (p *PendingHit) HasAppliedImpactProtection() bool { return p.impactProtectionApplied }

// MarkImpactProtectionApplied sets the impact-protection flag.
func (p *PendingHit) MarkImpactProtectionApplied() { p.impactProtectionApplied = true }

// HasAppliedImpactIncoming reports the impact-incoming flag.
func (p *PendingHit) HasAppliedImpactIncoming() bool { return p.impactIncomingApplied }

// MarkImpactIncomingApplied sets the impact-incoming flag.
func (p *PendingHit) MarkImpactIncomingApplied() { p.impactIncomingApplied = true }

// DeferIncomingDamageUntilImpact marks the hit as filled by a
// scripted producer in manipulateHit: Elysian/equipment run
// after it.
func (p *PendingHit) DeferIncomingDamageUntilImpact() { p.incomingDamageDeferred = true }

// DefersIncomingDamage reports the deferral mark.
func (p *PendingHit) DefersIncomingDamage() bool { return p.incomingDamageDeferred }

// CompleteDeferredIncomingDamage clears the deferral mark.
func (p *PendingHit) CompleteDeferredIncomingDamage() { p.incomingDamageDeferred = false }

// HandleAfterHitEffects reports whether after-hit effects run.
func (p *PendingHit) HandleAfterHitEffects() bool { return p.handleAfterHitEffects }

// SetHandleAfterHitEffects sets the after-hit-effects flag.
func (p *PendingHit) SetHandleAfterHitEffects(handle bool) *PendingHit {
	p.handleAfterHitEffects = handle
	return p
}

// HitDelays returns the per-splat extra reveal delays, if any.
func (p *PendingHit) HitDelays() []int { return p.hitDelays }

// WithoutHitsplat marks the hit as resolving without a splat
// (no hitsplat shown, damage still resolves).
func (p *PendingHit) WithoutHitsplat() *PendingHit {
	p.withoutHitsplat = true
	return p
}

// IsWithoutHitsplat reports the no-splat mark.
func (p *PendingHit) IsWithoutHitsplat() bool { return p.withoutHitsplat }

// Skills returns the skills the usual combat XP splits
// across: none for NPC attackers, the experience-skills
// override, else the fight style's skills for the combat
// type (PendingHit.getSkills).
func (p *PendingHit) Skills() []int {
	attacker := combatantFor(p.attacker)
	if !attacker.IsPlayer() {
		return []int{}
	}
	if p.experienceSkills != nil {
		return append([]int(nil), p.experienceSkills...)
	}
	return p.attacker.GetCombat().FightType().Style.Skills(p.combatType)
}

// CombatType returns the hit's combat type.
func (p *PendingHit) CombatType() CombatType { return p.combatType }

// prepareHits rolls the splats (PendingHit.prepareHits). A hit
// amount above 4 is clamped to 4 (TS throws; a tick-path panic
// is worse than a clamp, and no core caller exceeds 4).
// Damage is capped to the HP the target held when the blow
// was rolled (tick-eating). Protection, Elysian, and
// incoming-damage reactions are deliberately not applied here.
func (p *PendingHit) prepareHits(hitAmount int, rollAccuracy bool, fixedDamage *int, bypassProtectionPrayer bool) []*HitDamage {
	if hitAmount > 4 {
		hitAmount = 4
	}
	if hitAmount < 0 {
		hitAmount = 0
	}
	if p.attacker == nil || p.target == nil {
		return nil
	}
	var traits *WeaponSpecialTraits
	if fixedDamage == nil && !p.ordinary {
		traits = ActiveTraitsFor(combatantFor(p.attacker))
	}
	if traits != nil && traits.SkipAttack {
		p.totalDamage = 0
		return []*HitDamage{}
	}
	if hitAmount > 0 && Hooks.OnCombatHitPrepare != nil {
		Hooks.OnCombatHitPrepare(&CombatHitPrepareEvent{
			Attacker:   p.attacker,
			Target:     p.target,
			CombatType: p.combatType,
		})
	}
	if traits != nil && traits.HitCount != nil {
		if *traits.HitCount > hitAmount {
			hitAmount = *traits.HitCount
		}
		if hitAmount > 4 {
			hitAmount = 4
		}
	}
	if traits != nil && len(traits.HitDelayTicks) > 0 {
		p.hitDelays = make([]int, len(traits.HitDelayTicks))
		for i, tick := range traits.HitDelayTicks {
			if tick < 0 {
				tick = 0
			}
			p.hitDelays[i] = tick
		}
	}
	p.protectionType = p.combatType
	if traits != nil {
		p.protectionType = traits.ResolveDamageType(p.combatType)
	}
	if bypassProtectionPrayer || (traits != nil && traits.IgnoreProtectionPrayer) {
		p.protectionBypassed = true
	}
	p.totalDamage = 0
	remaining := combatantFor(p.target).Hitpoints()
	if remaining < 0 {
		remaining = 0
	}
	capToRemaining := func(damage *HitDamage) *HitDamage {
		if damage.Damage() > remaining {
			damage.SetDamage(remaining)
		}
		remaining -= damage.Damage()
		return damage
	}

	if hitAmount == 1 {
		roll := &CombatHitRollEvent{
			Attacker:               p.attacker,
			Target:                 p.target,
			CombatType:             p.combatType,
			BypassProtectionPrayer: bypassProtectionPrayer,
		}
		if Hooks.OnCombatHitRoll != nil {
			Hooks.OnCombatHitRoll(roll)
		}
		if roll.BypassProtectionPrayer {
			p.protectionBypassed = true
		}
		damage := NewHitDamage(0, HitMaskBlue)
		p.accurate = false
		attempts := p.bestOf
		if fixedDamage != nil {
			attempts = 1
		}
		for attempt := attempts; attempt > 0; attempt-- {
			resolved := p.rollSpecialTraits(traits, roll.ForceAccurate, rollAccuracy, 0, nil)
			resolved = withForcedMax(resolved, roll.ForceMaxHit)
			var rolled *HitDamage
			if resolved.accurate {
				var bounds *SpecialDamageBounds
				if fixedDamage == nil {
					bounds = resolved.bounds
				} else {
					zero := 0.0
					fixed := *fixedDamage
					bounds = &SpecialDamageBounds{
						MinimumMultiplier: &zero,
						MaximumMultiplier: &zero,
						MinimumBonus:      &fixed,
						MaximumBonus:      &fixed,
					}
				}
				applyDefences := false
				rolled = GetHitDamage(p.attacker, p.target, p.combatType, roll.BypassProtectionPrayer, bounds, fixedDamage == nil, &applyDefences)
			} else {
				rolled = NewHitDamage(0, HitMaskBlue)
			}
			if fixedDamage != nil {
				rolled.SetRolledMaximum(0)
			}
			if rolled.Damage() > damage.Damage() || (resolved.accurate && !p.accurate) {
				damage = rolled
				p.accurate = resolved.accurate
			}
		}
		// Arceuus corruption: skipped (content spell system).
		p.emitHitDamage(damage)
		if rollAccuracy && !p.accurate {
			p.failedAccuracyChecks = 1
		}
		capToRemaining(damage)
		if p.damageMultiplier != 1 {
			damage.SetDamage(int(math.Floor(float64(damage.Damage()) * p.damageMultiplier)))
			hpCap := combatantFor(p.target).Hitpoints()
			if hpCap < 0 {
				hpCap = 0
			}
			if damage.Damage() > hpCap {
				damage.SetDamage(hpCap)
			}
		}
		p.totalDamage = damage.Damage()
		return []*HitDamage{damage}
	}

	if traits != nil && len(traits.FirstSuccessfulAccuracyDamageRanges) > 0 {
		if branch := p.resolveFirstSuccessfulRanges(traits, capToRemaining, bypassProtectionPrayer); branch != nil {
			return branch
		}
	}

	hits := make([]*HitDamage, hitAmount)
	var firstAccurate *bool
	var firstBounds *SpecialDamageBounds
	priorHits := []*HitDamage{}
	anyAccurate := false
	for i := 0; i < hitAmount; i++ {
		roll := &CombatHitRollEvent{
			Attacker:               p.attacker,
			Target:                 p.target,
			CombatType:             p.combatType,
			BypassProtectionPrayer: bypassProtectionPrayer,
		}
		blockedByFirstMiss := i > 0 && p.requireFirstAccurate && (firstAccurate == nil || !*firstAccurate)
		if !blockedByFirstMiss && Hooks.OnCombatHitRoll != nil {
			Hooks.OnCombatHitRoll(roll)
		}
		if roll.BypassProtectionPrayer {
			p.protectionBypassed = true
		}
		var projectedDefence *int
		if !blockedByFirstMiss && i > 0 && p.method != nil {
			projectedDefence = p.method.ProjectedDefenceLevel(p.attacker, p.target, ProjectedDefenceContext{
				HitIndex:   i,
				PriorHits:  append([]*HitDamage(nil), priorHits...),
				CombatType: p.combatType,
			})
		}
		var resolved accuracyResolution
		switch {
		case blockedByFirstMiss:
			resolved = accuracyResolution{accurate: false}
		case i > 0 && traits != nil && traits.SharedAccuracyRollAcrossHits:
			acc := firstAccurate != nil && *firstAccurate
			resolved = accuracyResolution{accurate: acc, bounds: firstBounds}
		default:
			resolved = withForcedMax(p.rollSpecialTraits(traits, roll.ForceAccurate, rollAccuracy, i, projectedDefence), roll.ForceMaxHit)
			if i == 0 {
				acc := resolved.accurate
				firstAccurate = &acc
				firstBounds = resolved.bounds
			}
		}
		if p.hitMaximumSplitCount > 1 {
			bounds := &SpecialDamageBounds{}
			if resolved.bounds != nil {
				copied := *resolved.bounds
				bounds = &copied
			}
			bounds.MaximumSplit = &MaximumSplit{Index: i, Count: p.hitMaximumSplitCount}
			resolved.bounds = bounds
		}
		p.accurate = resolved.accurate
		var damage *HitDamage
		if p.accurate {
			applyDefences := false
			damage = GetHitDamage(p.attacker, p.target, p.combatType, roll.BypassProtectionPrayer, resolved.bounds, true, &applyDefences)
		} else {
			damage = NewHitDamage(0, HitMaskBlue)
		}
		p.emitHitDamage(damage)
		ownRoll := !blockedByFirstMiss && !(i > 0 && traits != nil && traits.SharedAccuracyRollAcrossHits)
		if ownRoll && rollAccuracy && !p.accurate {
			p.failedAccuracyChecks++
		}
		if i == 0 && p.requireFirstAccurate {
			acc := p.accurate
			firstAccurate = &acc
		}
		capToRemaining(damage)
		p.totalDamage += damage.Damage()
		hits[i] = damage
		priorHits = append(priorHits, damage)
		anyAccurate = anyAccurate || p.accurate
	}
	p.accurate = anyAccurate
	return hits
}

// accuracyResolution is rollSpecialTraits' result: the outcome
// plus the per-hit damage bounds.
type accuracyResolution struct {
	accurate bool
	bounds   *SpecialDamageBounds
}

// withForcedMax is PendingHit.withForcedMax: a forced max hit
// lands on the maximum (a minimum multiplier of 1).
func withForcedMax(resolved accuracyResolution, forceMaxHit bool) accuracyResolution {
	if !forceMaxHit || !resolved.accurate {
		return resolved
	}
	bounds := &SpecialDamageBounds{}
	if resolved.bounds != nil {
		copied := *resolved.bounds
		bounds = &copied
	}
	one := 1.0
	bounds.MinimumMultiplier = &one
	resolved.bounds = bounds
	return resolved
}

// rollSpecialTraits resolves accuracy for one splat using the
// active special's traits and derives the per-hit damage
// bounds (PendingHit.rollSpecialTraits).
func (p *PendingHit) rollSpecialTraits(traits *WeaponSpecialTraits, forceAccurate bool, rollAccuracyFlag bool, hitIndex int, projectedDefenceLevel *int) accuracyResolution {
	var defenceType *CombatType
	if p.method != nil {
		defenceType = p.method.AccuracyDefenceType(p.combatType)
	}
	attacker := combatantFor(p.attacker)
	target := combatantFor(p.target)
	if traits == nil {
		accurate := forceAccurate || !rollAccuracyFlag ||
			RollAccuracy(attacker, target, p.combatType, defenceType, "", projectedDefenceLevel)
		return accuracyResolution{accurate: accurate}
	}
	lastDamager := target.Mobile().GetCombat().LastDamager()
	unsuspecting := lastDamager == nil || &lastDamager.Mobile != p.attacker
	guaranteed := traits.GuaranteedHit || forceAccurate ||
		(hitIndex == 0 && traits.GuaranteedFirstAccuracyRoll) ||
		(traits.GuaranteedHitOnUnsuspectingTarget && unsuspecting)
	rollCount := 1
	if traits.AccuracyRollCount != nil && *traits.AccuracyRollCount > 1 {
		rollCount = *traits.AccuracyRollCount
	}
	var successfulRolls int
	if guaranteed || !rollAccuracyFlag {
		successfulRolls = rollCount
	} else if traits.FixedAccuracyRollMultiplierWhenTargetAtOrBelowMaximumDamage != nil &&
		target.Hitpoints() <= SourceMaxHit(attacker, p.combatType) {
		if RollFixedAccuracy(attacker, target, p.combatType, *traits.FixedAccuracyRollMultiplierWhenTargetAtOrBelowMaximumDamage, defenceType, projectedDefenceLevel) {
			successfulRolls = 1
		}
	} else {
		successfulRolls = RollAccuracyCount(attacker, target, p.combatType, rollCount, defenceType, projectedDefenceLevel)
	}
	var rng *AccuracyDamageRange
	if successfulRolls > 0 && successfulRolls-1 < len(traits.DamageRangeBySuccessfulAccuracyRolls) {
		r := traits.DamageRangeBySuccessfulAccuracyRolls[successfulRolls-1]
		rng = &r
	}
	bounds := &SpecialDamageBounds{}
	if rng != nil && rng.MinimumDamageMultiplier != nil {
		bounds.MinimumMultiplier = rng.MinimumDamageMultiplier
	} else {
		bounds.MinimumMultiplier = traits.MinimumDamageMultiplier
	}
	if rng != nil && rng.MaximumDamageMultiplier != nil {
		bounds.MaximumMultiplier = rng.MaximumDamageMultiplier
	} else {
		bounds.MaximumMultiplier = traits.MaximumDamageMultiplier
	}
	bounds.MinimumBonus = traits.MinimumDamageBonus
	bounds.MaximumBonus = traits.MaximumDamageBonus
	bounds.Cap = traits.MaximumDamageCap
	if successfulRolls == rollCount && traits.MaximumHitReductionOnFullAccuracyRolls != nil {
		bounds.Reduction = *traits.MaximumHitReductionOnFullAccuracyRolls
	}
	return accuracyResolution{accurate: successfulRolls > 0, bounds: bounds}
}

// resolveFirstSuccessfulRanges is the first-successful-accuracy
// branch table (PendingHit.resolveFirstSuccessfulRanges).
func (p *PendingHit) resolveFirstSuccessfulRanges(traits *WeaponSpecialTraits, capToRemaining func(*HitDamage) *HitDamage, bypassProtectionPrayer bool) []*HitDamage {
	ranges := traits.FirstSuccessfulAccuracyDamageRanges
	if len(ranges) == 0 {
		return nil
	}
	hitCount := 1
	if traits.HitCount != nil && *traits.HitCount > 1 {
		hitCount = *traits.HitCount
	}
	attacker := combatantFor(p.attacker)
	target := combatantFor(p.target)
	var defenceType *CombatType
	if p.method != nil {
		defenceType = p.method.AccuracyDefenceType(p.combatType)
	}
	for attempt, rng := range ranges {
		success := traits.GuaranteedHit ||
			RollAccuracy(attacker, target, p.combatType, defenceType, "", nil)
		if !success {
			failed := attempt + 1
			if failed > hitCount {
				failed = hitCount
			}
			p.failedAccuracyChecks = failed
			continue
		}
		bounds := &SpecialDamageBounds{
			MinimumMultiplier: rng.MinimumDamageMultiplier,
			MaximumMultiplier: rng.MaximumDamageMultiplier,
			Reduction:         rng.MaximumDamageReduction,
		}
		applyDefences := false
		base := GetHitDamage(p.attacker, p.target, p.combatType, bypassProtectionPrayer, bounds, true, &applyDefences).Damage()
		if bypassProtectionPrayer {
			p.protectionBypassed = true
		}
		var distributed []int
		if rng.DistributeDamage != nil {
			distributed = rng.DistributeDamage(base, hitCount)
		}
		hits := []*HitDamage{}
		total := 0
		for i := 0; i < hitCount; i++ {
			var raw float64
			if distributed != nil {
				if i < len(distributed) {
					raw = float64(distributed[i])
				}
			} else {
				mult := 0.0
				if i < len(rng.HitDamageMultipliers) {
					mult = rng.HitDamageMultipliers[i]
				}
				bonus := 0
				if i < len(rng.HitDamageBonuses) {
					bonus = rng.HitDamageBonuses[i]
				}
				raw = float64(base)*mult + float64(bonus)
			}
			value := int(math.Floor(raw))
			if value < 0 {
				value = 0
			}
			mask := HitMaskRed
			if value == 0 {
				mask = HitMaskBlue
			}
			hit := NewHitDamage(value, mask)
			capToRemaining(hit)
			total += hit.Damage()
			hits = append(hits, hit)
		}
		p.accurate = total > 0
		p.totalDamage = total
		index := attempt
		p.firstSuccessfulRangeIndex = &index
		return hits
	}
	negative := -1
	p.firstSuccessfulRangeIndex = &negative
	patterns := traits.AllMissDamagePatterns
	fallbackRoll := randomFloat()
	var pattern []int
	if len(patterns) > 0 {
		index := int(math.Floor(fallbackRoll * float64(len(patterns))))
		if index >= len(patterns) {
			index = len(patterns) - 1
		}
		pattern = patterns[index]
	} else {
		pattern = make([]int, hitCount)
		for i := 0; i < hitCount; i++ {
			if i == hitCount-1 {
				switch {
				case fallbackRoll < 0.2:
					pattern[i] = 0
				case fallbackRoll < 0.6:
					pattern[i] = 1
				default:
					pattern[i] = 2
				}
			}
		}
	}
	hits := []*HitDamage{}
	total := 0
	landed := false
	for i := 0; i < hitCount; i++ {
		value := 0
		if i < len(pattern) {
			value = pattern[i]
		}
		if value < 0 {
			value = 0
		}
		if value > 0 {
			landed = true
		}
		mask := HitMaskRed
		if value == 0 {
			mask = HitMaskBlue
		}
		hit := NewHitDamage(value, mask)
		capToRemaining(hit)
		total += hit.Damage()
		hits = append(hits, hit)
	}
	p.accurate = landed
	p.totalDamage = total
	return hits
}

// emitHitDamage fires the combat:hit-damage hook and writes
// the outcome back (PendingHit.emitHitDamage).
func (p *PendingHit) emitHitDamage(damage *HitDamage) {
	if Hooks.OnCombatHitDamage == nil {
		return
	}
	e := &CombatHitDamageEvent{
		Attacker:   p.attacker,
		Target:     p.target,
		CombatType: p.combatType,
		Hit:        p,
		Damage:     damage,
		Accurate:   p.accurate,
	}
	Hooks.OnCombatHitDamage(e)
	p.accurate = e.Accurate
}

// ---- HitQueue ----

// HitQueue is HitQueue (hit/HitQueue.ts): the owner's
// scheduled impacts. One queue per Combat; the active set
// (ProcessAllHitQueues) catches entities that skipped a turn.
type HitQueue struct {
	owner              *Mobile
	pendingHits        []scheduledHit
	pendingDamage      []*HitDamage
	lastProcessedCycle int64
	lastAppliedHit     *HitDamage
	poisonSplats       int
	incomingGeneration int
}

type scheduledHit struct {
	hit         *PendingHit
	revealCycle int64
}

var activeHitQueues = map[*HitQueue]bool{}

// NewHitQueue returns a HitQueue for owner.
func NewHitQueue(owner *Mobile) *HitQueue {
	return &HitQueue{owner: owner, lastProcessedCycle: -1}
}

// ProcessAllHitQueues drains every active queue for cycle
// (HitQueue.processAll): the catch-all for entities that did
// not take a turn. The per-queue cycle guard filters out
// entities that already drained. The coordinator wires it to
// World.Hooks.CombatHits.
func ProcessAllHitQueues(cycle int64) {
	for queue := range activeHitQueues {
		queue.Process(cycle)
	}
}

// DropHitsFrom drops the hits attacker still has on their way
// to anyone (HitQueue.dropHitsFrom): called when the attacker
// dies, so a hit launched before death cannot land (or
// credit) after a respawn.
func DropHitsFrom(attacker *Mobile) {
	for queue := range activeHitQueues {
		kept := queue.pendingHits[:0]
		for _, entry := range queue.pendingHits {
			if entry.hit.Attacker() != attacker {
				kept = append(kept, entry)
			}
		}
		queue.pendingHits = kept
		if !queue.HasPendingWork() {
			delete(activeHitQueues, queue)
		}
	}
}

// Process applies every impact due on or before cycle
// (HitQueue.process): called at the start of the owner's own
// turn, so a lethal hit lands before the entity acts. The
// cycle guard makes repeat calls in one cycle no-ops. An
// unregistered or dead owner clears the queue. Protection
// prayers are read for every pending hit (idempotent via the
// impact flag). A full animation stall holds incoming hits
// head-of-line (players only); self-damage still lands, and
// held entries are re-queued as a block unless clearIncoming
// ran mid-drain.
func (q *HitQueue) Process(cycle int64) {
	if q.lastProcessedCycle == cycle {
		return
	}
	q.lastProcessedCycle = cycle
	owner := combatantFor(q.owner)
	if q.owner == nil || !q.owner.IsRegistered() || owner.Hitpoints() <= 0 {
		q.pendingHits = nil
		q.pendingDamage = nil
		delete(activeHitQueues, q)
		return
	}
	for _, entry := range q.pendingHits {
		ApplyProtectionPrayer(entry.hit)
	}
	hold := int64(-1)
	if owner.IsPlayer() {
		hold = q.owner.GetCombat().GetIncomingHitCycle()
	}
	stalled := hold > cycle
	for !stalled && len(q.pendingHits) > 0 && q.pendingHits[0].revealCycle <= cycle {
		entry := q.pendingHits[0]
		q.pendingHits = q.pendingHits[1:]
		hit := entry.hit
		attacker := combatantFor(hit.Attacker())
		if hit.Target() == q.owner && hit.Attacker() != nil && hit.Attacker().IsRegistered() &&
			attacker.Hitpoints() > 0 && !isUntargetable(q.owner) {
			ExecuteHit(hit)
		}
	}
	var held []*HitDamage
	generation := q.incomingGeneration
	for len(q.pendingDamage) > 0 {
		damage := q.pendingDamage[0]
		q.pendingDamage = q.pendingDamage[1:]
		if stalled && incomingFromOther(damage, q.owner) {
			held = append(held, damage)
			continue
		}
		AddTickHit(q.owner, q.applyDamage(damage))
	}
	if len(held) > 0 && generation == q.incomingGeneration {
		q.pendingDamage = append(held, q.pendingDamage...)
	}
	if !q.HasPendingWork() {
		delete(activeHitQueues, q)
	}
}

// incomingFromOther reports whether damage came from another
// combatant (HitQueue.incomingFromOther): a non-nil source
// that is not the owner.
func incomingFromOther(damage *HitDamage, owner *Mobile) bool {
	source := damage.Source()
	return source != nil && source != owner
}

// HasProcessed reports whether the queue already drained this
// cycle (its turn has passed).
func (q *HitQueue) HasProcessed(cycle int64) bool {
	return q.lastProcessedCycle == cycle
}

// applyDamage takes a hit off the owner's hitpoints, then
// runs the effects that react to the result (HitQueue
// applyDamage): last-applied tracking, the poison-splat
// count, applied observers, the reflected-damage event
// (skipped: no hook field — reflections skip the resolved-hit
// hooks in TS too), and Redemption.
func (q *HitQueue) applyDamage(hit *HitDamage) *HitDamage {
	applied := DecrementHealth(q.owner, hit)
	if applied.Damage() > 0 || combatantFor(q.owner).Hitpoints() > 0 {
		q.lastAppliedHit = applied
	}
	if applied.Mask() == HitMaskGreen && applied.Damage() > 0 {
		q.poisonSplats++
	}
	hit.NotifyApplied(applied.HealthLost())
	// combat:reflected-damage: skipped (no hook field).
	HandleRedemption(q.owner)
	return applied
}

// PoisonSplatCount returns how many poison-type (green)
// splats with damage have landed; it only grows.
func (q *HitQueue) PoisonSplatCount() int { return q.poisonSplats }

// LastAppliedHit returns the last hit taken; after a death it
// is the killing blow.
func (q *HitQueue) LastAppliedHit() *HitDamage { return q.lastAppliedHit }

// AddPendingHit queues a hit for its reveal cycle, inserted
// in reveal order (stable: equal cycles keep queue order).
func (q *HitQueue) AddPendingHit(hit *PendingHit, revealCycle int64) {
	if revealCycle < 0 {
		revealCycle = 0
	}
	scheduled := scheduledHit{hit: hit, revealCycle: revealCycle}
	index := len(q.pendingHits)
	for index > 0 && q.pendingHits[index-1].revealCycle > scheduled.revealCycle {
		index--
	}
	q.pendingHits = append(q.pendingHits, scheduledHit{})
	copy(q.pendingHits[index+1:], q.pendingHits[index:])
	q.pendingHits[index] = scheduled
	activeHitQueues[q] = true
}

// AddPendingDamage queues direct damage (multi-hit later
// splats, recoil, poison). Nil entries are skipped.
func (q *HitQueue) AddPendingDamage(hits []*HitDamage) {
	added := false
	for _, hit := range hits {
		if hit != nil {
			q.pendingDamage = append(q.pendingDamage, hit)
			added = true
		}
	}
	if added {
		activeHitQueues[q] = true
	}
}

// Clear drops everything queued.
func (q *HitQueue) Clear() {
	q.pendingHits = nil
	q.pendingDamage = nil
	delete(activeHitQueues, q)
}

// ClearIncoming drops what is still on its way in from other
// combatants and keeps the owner's own pending damage. It
// bumps the incoming generation, so deferred hits know they
// were cancelled.
func (q *HitQueue) ClearIncoming() {
	q.incomingGeneration++
	q.pendingHits = nil
	kept := q.pendingDamage[:0]
	for _, damage := range q.pendingDamage {
		if !incomingFromOther(damage, q.owner) {
			kept = append(kept, damage)
		}
	}
	q.pendingDamage = kept
	if !q.HasPendingWork() {
		delete(activeHitQueues, q)
	}
}

// IncomingGeneration returns the clearIncoming generation.
func (q *HitQueue) IncomingGeneration() int { return q.incomingGeneration }

// HasPendingWork reports whether anything is queued.
func (q *HitQueue) HasPendingWork() bool {
	return len(q.pendingHits) > 0 || len(q.pendingDamage) > 0
}

// HasIncomingPlayerHit reports whether another player's hit is
// on its way to the owner.
func (q *HitQueue) HasIncomingPlayerHit() bool {
	for _, entry := range q.pendingHits {
		attacker := entry.hit.Attacker()
		if attacker != nil && attacker != q.owner && combatantFor(attacker).IsPlayer() {
			return true
		}
	}
	return false
}

// HasPendingHit reports whether this exact hit is queued.
func (q *HitQueue) HasPendingHit(hit *PendingHit) bool {
	for _, entry := range q.pendingHits {
		if entry.hit == hit {
			return true
		}
	}
	return false
}

// HasPendingHitFrom reports whether any queued hit comes from
// attacker.
func (q *HitQueue) HasPendingHitFrom(attacker *Mobile) bool {
	for _, entry := range q.pendingHits {
		if entry.hit.Attacker() == attacker {
			return true
		}
	}
	return false
}

// QueuedDamage returns the queued direct damage (pending
// damage only — pending hits are counted by
// AccumulatedDamage).
func (q *HitQueue) QueuedDamage() int {
	total := 0
	for _, hit := range q.pendingDamage {
		total += hit.Damage()
	}
	return total
}

// AccumulatedDamage returns the damage due within the next
// two cycles (pending hits) plus all pending damage.
func (q *HitQueue) AccumulatedDamage() int {
	damage := 0
	imminentCycle := combatCycle() + 2
	for _, entry := range q.pendingHits {
		if entry.revealCycle <= imminentCycle {
			damage += entry.hit.TotalDamage()
		}
	}
	for _, hit := range q.pendingDamage {
		damage += hit.Damage()
	}
	return damage
}

// IsEmpty reports whether every pending hit comes from
// exception (an empty queue answers true).
func (q *HitQueue) IsEmpty(exception *Mobile) bool {
	for _, entry := range q.pendingHits {
		if entry.hit.Attacker() != exception {
			return false
		}
	}
	return true
}

// IsEmptyButFreeNPCs is IsEmpty, except hits from NPCs nobody
// owns don't count either.
func (q *HitQueue) IsEmptyButFreeNPCs(exception *Mobile) bool {
	for _, entry := range q.pendingHits {
		attacker := combatantFor(entry.hit.Attacker())
		if entry.hit.Attacker() == exception {
			continue
		}
		if !attacker.IsPlayer() && attacker.NPC() != nil && attacker.NPC().Owner() == nil {
			continue
		}
		return false
	}
	return true
}

// randomInclusiveRange rolls a uniform integer in [min, max]
// inclusive (Misc.randomInclusive). A max below min answers
// min.
func randomInclusiveRange(min, max int) int {
	if max <= min {
		return min
	}
	return min + randomInclusive(max-min)
}
