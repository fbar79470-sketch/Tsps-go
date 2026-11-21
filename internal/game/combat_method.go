package game

// CombatMethod is method/CombatMethod.ts (88 lines): one attack
// style's behavior — how an attack starts, what hits it rolls,
// whether it may attack or pursue, how fast and how far it
// reaches. The three implementations live in combat_melee.go,
// combat_ranged.go, and combat_magic.go; content registers
// further methods through the provider/resolver registries
// (combat_hooks.go) or as special methods.
//
// TS CombatMethod is a class with default implementations; Go
// splits it into the CombatMethod interface and the
// BaseCombatMethod struct, which implementations embed and
// override selectively.
type CombatMethod interface {
	// Type is the combat type of this method's attacks.
	Type() CombatType
	// Start runs the attack's opening: animation, sound,
	// projectile, ammo payment setup (method start). No-ops
	// for animation/sound/projectile in Go (systems NOT
	// STARTED); ranged ammo payment and magic cast setup are
	// real.
	Start(attacker, target *Mobile)
	// Finished runs the attack's teardown (magic cast
	// teardown; a no-op for melee/ranged).
	Finished(attacker, target *Mobile)
	// Hits rolls the attack's hits, or returns nil when the
	// attack produces no hit (Combat.ts executeAttack treats
	// nil as "no attack happened"). A nil return is distinct
	// from an empty slice.
	Hits(attacker, target *Mobile) []*PendingHit
	// OnCombatBegan runs when combat begins for this fighter
	// (method onCombatBegan). A no-op by default.
	OnCombatBegan(fighter *Mobile)
	// OnCombatEnded runs when combat ends for this fighter
	// (method onCombatEnded). A no-op by default.
	OnCombatEnded(fighter *Mobile)
	// HandleAfterHitEffects runs the method's post-hit
	// effects (spell impacts; a no-op for melee/ranged).
	HandleAfterHitEffects(hit *PendingHit)
	// PlaysBlockAnimation reports whether the target plays
	// its block animation on a hit (default true).
	PlaysBlockAnimation() bool
	// SplitsDefensiveMagicExperience reports whether
	// defensive magic XP splits between Magic and Defence
	// (default true; CombatFactory.rewardExp).
	SplitsDefensiveMagicExperience() bool
	// CanAttack reports whether the method itself permits an
	// attack right now. It MAY consume state (magic casts);
	// callers must not call it speculatively — CanPursue is
	// the read-only form.
	CanAttack(attacker, target *Mobile) bool
	// CanPursue reports whether the method permits pursuing
	// the target (the read-only form of CanAttack; it must
	// NOT delegate to CanAttack, which consumes).
	CanPursue(attacker, target *Mobile) bool
	// ShouldRenew reports whether combat continues after this
	// attack (default true). Ranged re-checks ammo; a false
	// answer cancels combat after the attack lands.
	ShouldRenew(attacker, target *Mobile) bool
	// AttackSpeed is the ticks between attacks for this
	// method (default: the fighter's base attack speed).
	AttackSpeed(attacker *Mobile) int
	// AttackDistance is the method's reach in tiles (default
	// 1; ranged/magic override).
	AttackDistance(attacker *Mobile) int
	// AccuracyDefenceType is the defence-roll override for
	// attacks that roll one style's accuracy against
	// another's defence (nil = the attack's own type).
	AccuracyDefenceType(combatType CombatType) *CombatType
	// ProjectedDefenceLevel is the defender's projected
	// defence level for this method's rolls (nil = the live
	// level). Used by pending-drain predictions; ctx carries
	// the hit index, prior hits, and combat type.
	ProjectedDefenceLevel(attacker, target *Mobile, ctx ProjectedDefenceContext) *int
}

// ProjectedDefenceContext is the argument object TS passes to
// projectedDefenceLevel: the hit index within a multi-hit,
// the hits already rolled, and the combat type.
type ProjectedDefenceContext struct {
	HitIndex   int
	PriorHits  []*HitDamage
	CombatType CombatType
}

// BaseCombatMethod is the CombatMethod.ts default
// implementation: no-op lifecycle, permissive gates, renewing
// attacks, base speed, distance 1, identity defence type, no
// projection. Implementations embed it and override.
type BaseCombatMethod struct{}

func (BaseCombatMethod) Start(attacker, target *Mobile)            {}
func (BaseCombatMethod) Finished(attacker, target *Mobile)         {}
func (BaseCombatMethod) OnCombatBegan(fighter *Mobile)             {}
func (BaseCombatMethod) OnCombatEnded(fighter *Mobile)             {}
func (BaseCombatMethod) HandleAfterHitEffects(hit *PendingHit)     {}
func (BaseCombatMethod) PlaysBlockAnimation() bool                 { return true }
func (BaseCombatMethod) SplitsDefensiveMagicExperience() bool      { return true }
func (BaseCombatMethod) CanAttack(attacker, target *Mobile) bool   { return true }
func (BaseCombatMethod) CanPursue(attacker, target *Mobile) bool   { return true }
func (BaseCombatMethod) ShouldRenew(attacker, target *Mobile) bool { return true }
func (BaseCombatMethod) AttackSpeed(attacker *Mobile) int {
	return BaseAttackSpeed(combatantFor(attacker))
}
func (BaseCombatMethod) AttackDistance(attacker *Mobile) int { return 1 }
func (BaseCombatMethod) AccuracyDefenceType(combatType CombatType) *CombatType {
	return nil
}
func (BaseCombatMethod) ProjectedDefenceLevel(attacker, target *Mobile, ctx ProjectedDefenceContext) *int {
	return nil
}

// BaseAttackSpeed is the fighter's base attack speed in ticks
// (Player.ts getBaseAttackSpeed / NPC.ts getBaseAttackSpeed):
// NPCs read the definition attack speed (default 4); players
// read the weapon speed (cache attack speed, default 4),
// minus 1 for a rapid fight style, floored at 1. Weapon
// profiles (WeaponProfiles.attackSpeed) may override the
// cache speed for specific weapons; no profile table exists
// in Go, so the cache speed is the only source.
func BaseAttackSpeed(attacker Combatant) int {
	if attacker == nil || attacker.Mobile() == nil {
		return 4
	}
	if !attacker.IsPlayer() {
		if d := attacker.NPCDef(); d != nil {
			return d.AttackSpeed()
		}
		return 4
	}
	speed := 4
	if src, ok := attacker.ItemDefinitions().(WeaponInfoSource); ok && src != nil {
		if weapon := attacker.Equipment().GetWeapon(); weapon.IsValid() {
			if s := src.WeaponAttackSpeed(weapon.ID()); s > 0 {
				speed = s
			}
		}
	}
	if attacker.Mobile().GetCombat().FightType().Rapid {
		speed--
	}
	if speed < 1 {
		speed = 1
	}
	return speed
}

// CombatSpell is the core-read subset of CombatSpell
// (game/content/combat/magic/CombatSpell.ts): what the magic
// method, the magic formulas, and the XP grant read from a
// spell. Spell instances are CONTENT (registered by plugins);
// core defines the interface and the nil-spell behavior (NPCs
// cast without spell objects; players without a cast spell
// cannot use the magic method). Graphics, sounds, runes,
// requirements beyond CanCast, and area-effect details live
// on the content implementations.
type CombatSpell interface {
	// SpellID is the spell's client id (spell.spellId,
	// CombatSpells.ts): the widget-ish id the listeners
	// resolve (1152 Wind Strike, 1572 Bind, 12445 Tele
	// Block). Added in Phase 27 (contract C1): the
	// resolver and the autocast table read the id off the
	// spell instead of carrying a side map.
	SpellID() int
	// MaximumHit is the spell's declared maximum damage
	// (spell.maximumHit): 0 means harmless (stays 0 through
	// every bonus), > 0 is the base max hit.
	MaximumHit() int
	// BaseExperience is the XP owed on an accurate cast
	// (paid once per cast by MagicCombatMethod).
	BaseExperience() int
	// SplashExperience is the XP owed on a splashed cast.
	SplashExperience() int
	// AttackSpeed is the spell's attack speed in ticks (0 =
	// use the fighter's base attack speed).
	AttackSpeed() int
	// HitDelay is the spell's fixed hit delay in ticks (-1 =
	// compute from distance, MagicCombatMethod).
	HitDelay() int
	// CanCast reports whether the caster may cast (runes,
	// level, cooldowns); consume=true spends the cast cost.
	// The read-only form passes consume=false. Players only:
	// TS never consults a spell's CanCast for an NPC (the
	// magic method's canAttack returns true for NPCs before
	// any spell check, MagicCombatMethod.ts:230-232), and
	// NPCs pay no runes.
	CanCast(caster *Player, consume bool) bool
	// CanCastOnTarget reports whether the spell may be cast
	// on this target (immunities, target restrictions).
	// Players only, as CanCast: an NPC's target rule is the
	// combat method's, not the spell's.
	CanCastOnTarget(caster *Player, target *Mobile) bool
	// StartCast runs the cast's opening (animation, start
	// graphic, cast projectile; rune consumption is part of
	// CanCast(consume=true) in TS; StartCast handles the
	// cast-side effects). The caster is a *Mobile, as in TS
	// (CombatSpell.startCast, CombatSpell.ts:50-101): an
	// NPC holding a spell object casts through this same
	// path — its cast animation, its start graphic (except
	// NPC ids 2000/109/3580/2007), and its projectile —
	// while the queued impact visual stays player-only
	// (CombatSpell.ts:89-102).
	StartCast(caster *Mobile, target *Mobile)
	// FinishCast runs the spell's impact/splash effects
	// after the hit resolves (CombatSpell.finishCast):
	// accurate names the outcome, damage the total dealt.
	FinishCast(attacker, target *Mobile, accurate bool, damage int)
	// SpellRadius is the barrage radius in tiles (0 = a
	// single-target spell).
	SpellRadius() int
}
