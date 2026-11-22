package game

// MagicCombatMethod is the port of
// method/impl/MagicCombatMethod.ts: spell casts, distance
// hit delays, once-per-cast base XP, barrage multi-target
// hits, and the cast teardown in Finished. Cast and impact
// animations, graphics and projectiles went live in Phase 29
// (Start, the queued impact graphic, HandleAfterHitEffects),
// with the cast/impact sounds (Worker C's pickers,
// CastSoundFor / ImpactSoundFor); the blood-drain line is
// skipped (blood-spell content).
//
// Spells themselves are content (CombatSpell implementations
// registered by content/hub code). NPCs cast without a
// Spell object: definition max hit + projectile delay.

// MagicMethod is the shared magic combat method singleton
// (CombatFactory.MAGIC_COMBAT).
var MagicMethod CombatMethod = &magicCombatMethod{}

type magicCombatMethod struct{ BaseCombatMethod }

func (m *magicCombatMethod) Type() CombatType { return CombatTypeMagic }

// LegacyMagicEndBase is Projectile.LEGACY_MAGIC_END_BASE
// (46): the NPC magic projectile's end offset, 10 cycles a
// tile.
const LegacyMagicEndBase = 46

// DefaultNPCMagicProjectile is MagicCombatMethod's default
// NPC projectile (91, wind strike). Carried for grounding;
// projectiles are not sent.
const DefaultNPCMagicProjectile = 91

// splashGraphicID is MagicCombatMethod.SPLASH_GRAPHIC's id
// (85): the graphic a splashed spell plays on its target
// (MagicCombatMethod.ts:26). Its height is MIDDLE (50).
const splashGraphicID = 85

// The impact/splash area-sound radius is Worker C's
// MagicImpactSoundRadius (MagicCombatMethod.ts:53,
// IMPACT_SOUND_RADIUS 10), used by
// magicImpactPresentation and queueMagicImpactGraphic.

// magicHitDelay is MagicCombatMethod.hitDelay: a spell with
// a fixed delay keeps it; an NPC's cast lands when its
// projectile does (46 + 10 cycles a tile); a player's magic
// lands 1 + floor((1 + distance) / 3) ticks after the cast,
// the distance edge to edge — except barrage spells, which
// measure to the target's south-west tile (Chebyshev). The
// cross-floor branch is skipped (no CrossFloor).
func magicHitDelay(attacker, target *Mobile, spell CombatSpell) int {
	if spell != nil && spell.HitDelay() >= 0 {
		return spell.HitDelay()
	}
	if !combatantFor(attacker).IsPlayer() {
		return projectileArrivalTicks(attacker, target, LegacyMagicEndBase, 10)
	}
	distance := 0
	if spell != nil && spell.SpellRadius() > 0 {
		// Barrage: distance to the south-west tile. TS also
		// requires level 86+; the CombatSpell interface
		// carries no level, so any radius spell measures
		// this way (content declares a radius only for
		// barrages).
		distance = attacker.Location().Distance(target.Location())
		if distance < 0 {
			distance = 0
		}
	} else {
		distance = EdgeDistance(attacker, target)
	}
	return 1 + (1+distance)/3
}

// Hits is MagicCombatMethod.hits: one ordinary PendingHit
// carrying the selected spell, plus barrage secondaries in
// multi-combat, then the once-per-cast base XP. A spell
// cast is never a special attack (ordinary: true), so a
// special armed meanwhile keeps its energy and lends the
// spell nothing.
func (m *magicCombatMethod) Hits(attacker, target *Mobile) []*PendingHit {
	combat := attacker.GetCombat()
	spell := combat.SelectedSpell()
	hit := NewPendingHitConfig(attacker, target, m, PendingHitConfig{
		Delay:    magicHitDelay(attacker, target, spell),
		Ordinary: true,
	})
	hit.SetSpell(spell)
	hits := []*PendingHit{hit}
	if spell == nil {
		return hits
	}
	// spell.onHitCalc: a content hook on the spell; spells
	// that implement it are called through the optional
	// interface below.
	if calc, ok := spell.(interface{ OnHitCalc(hit *PendingHit) }); ok {
		calc.OnHitCalc(hit)
	}
	queueMagicImpactGraphic(hit)
	var multiHits []*PendingHit
	spellRadius := spell.SpellRadius()
	// Ancient-spellbook check: TS requires the ANCIENT
	// spellbook; the CombatSpell interface carries no
	// spellbook, so a positive radius is the area-spell
	// marker (content declares a radius only for barrages).
	if hit.IsAccurate() && spellRadius > 0 && AreaAttackSpreads(attacker, target) {
		if Hooks.SpellAreaTargets != nil {
			for _, next := range Hooks.SpellAreaTargets(attacker, target, spellRadius) {
				if !CanAttackSecondaryTarget(attacker, target, next, spellRadius) {
					continue
				}
				noAfterEffects := false
				secondary := NewPendingHitConfig(attacker, next, m, PendingHitConfig{
					Delay:                 magicHitDelay(attacker, next, spell),
					HandleAfterHitEffects: &noAfterEffects,
					Ordinary:              true,
				})
				secondary.SetSpell(spell)
				if calc, ok := spell.(interface{ OnHitCalc(hit *PendingHit) }); ok {
					calc.OnHitCalc(secondary)
				}
				multiHits = append(multiHits, secondary)
			}
		}
	}
	payBaseExperience(attacker, spell, hit.IsAccurate())
	// Blood-drain line: skipped (blood-spell content).
	return append(hits, multiHits...)
}

// payBaseExperience is MagicCombatMethod.payBaseExperience:
// OSRS pays a spell's base XP once per cast, however many
// targets it hits — the hit value, or the splash value
// when the main target splashes. rewardExp adds the
// per-damage XP.
func payBaseExperience(attacker *Mobile, spell CombatSpell, accurate bool) {
	combat := attacker.GetCombat()
	if !combat.takeBaseExperienceOwed() {
		return
	}
	experience := spell.SplashExperience()
	if accurate {
		experience = spell.BaseExperience()
	}
	if experience > 0 && combatantFor(attacker).IsPlayer() {
		combatantFor(attacker).Player().Skills().AddExperienceWithMultipliers(SkillMagic, experience, true)
	}
}

// CanAttack is MagicCombatMethod.canAttack: NPCs always;
// players promote the autocast spell into the cast slot,
// fail with no spell, fail (clearing the cast + resetting)
// when the spell refuses the target, else the spell's
// consuming cast check.
func (m *magicCombatMethod) CanAttack(attacker, target *Mobile) bool {
	if !combatantFor(attacker).IsPlayer() {
		return true
	}
	player := combatantFor(attacker).Player()
	combat := attacker.GetCombat()
	if combat.CastSpell() == nil && autocastingActive(player) {
		combat.SetCastSpell(combat.AutoCastSpell())
	}
	if combat.CastSpell() == nil {
		return false
	}
	if !combat.CastSpell().CanCastOnTarget(player, target) {
		combat.SetCastSpell(nil)
		combat.Reset()
		return false
	}
	return combat.CastSpell().CanCast(player, true)
}

// CanPursue is MagicCombatMethod.canPursue: read-only —
// canAttack owns promoting the autocast spell into the cast
// slot.
func (m *magicCombatMethod) CanPursue(attacker, target *Mobile) bool {
	if !combatantFor(attacker).IsPlayer() {
		return true
	}
	player := combatantFor(attacker).Player()
	combat := attacker.GetCombat()
	spell := combat.CastSpell()
	if spell == nil {
		spell = combat.AutoCastSpell()
	}
	return spell != nil && spell.CanCast(player, false)
}

// autocastingActive is Autocasting.isActive, routed through
// IsActive (autocasting.go): an autocast spell is set AND a
// staff is equipped. This closes the Phase 16 deviation, where
// the check read the spell only.
func autocastingActive(player *Player) bool {
	return IsActive(player)
}

// queueMagicImpactGraphic is MagicCombatMethod.
// queueProjectileGraphic (MagicCombatMethod.ts:196-210): at
// cast time — hits() runs on the cast tick — the impact or
// splash graphic, and its area sound, are queued on the
// target with the projectile's end-cycle delay, so they land
// when the projectile does. The visual was recorded on the
// paid cast's context by StartCast; a hit with no queued
// visual (an NPC cast, or a non-legacy spell) keeps the
// impact-time path in HandleAfterHitEffects.
func queueMagicImpactGraphic(hit *PendingHit) {
	spell := hit.Spell()
	if spell == nil || hit.Target() == nil {
		return
	}
	var ctx *SpellCastContext
	if player := combatantFor(hit.Attacker()).Player(); player != nil {
		ctx = spellCastContext(player, spell)
	}
	visual := getProjectileVisual(spell, ctx, hit.Target())
	if visual == nil || visual.queued {
		return
	}
	// Live sends the impact (or splash) area sound at the
	// cast, delayed to the projectile's end cycle
	// (MagicCombatMethod.ts:194-205): the impact sound on
	// an accurate hit, else SPELL_FAIL_SPLASH (227).
	// PlayAreaSound carries the delay; the sound plays
	// when the projectile lands. This runs before the
	// graphic's null check, as in TS.
	if !visual.soundQueued {
		var sound Sound
		if hit.IsAccurate() {
			sound = ImpactSoundFor(spell)
		} else {
			sound = SoundSpellFailSplash
		}
		if sound.ID() > 0 {
			loc := hit.Target().Location()
			PlayAreaSound(PlayAreaSoundOptions{
				SoundID: sound.ID(),
				Source:  hit.Target(),
				X:       loc.X, Y: loc.Y, Level: loc.Z,
				Loops:  sound.LoopType(),
				Delay:  visual.endCycleOffset + sound.Delay(),
				Radius: MagicImpactSoundRadius,
			})
			visual.soundQueued = true
		}
	}
	id, height, ok := spellEndGraphic(spell)
	if !ok {
		return
	}
	// The graphic counts as queued only when the delayed
	// graphic actually wins the priority check
	// (MagicCombatMethod.ts:211: visual.queued =
	// target.getGraphic() === delayed): PerformGraphic
	// reports whether it was accepted, which is the same
	// test — a fresh graphic object is the target's
	// graphic exactly when it was accepted. A graphic
	// that lost priority leaves queued false, so the
	// impact-time path replays it in HandleAfterHitEffects.
	// A spell with no end graphic plays nothing and stays
	// unqueued, as in TS.
	if hit.IsAccurate() {
		visual.queued = hit.Target().PerformGraphic(id, height, visual.endCycleOffset)
	} else {
		visual.queued = hit.Target().PerformGraphic(splashGraphicID, graphicHeightMiddle, visual.endCycleOffset)
	}
}

// Start is MagicCombatMethod.start
// (MagicCombatMethod.ts:261-300): NPCs without a spell cast
// with the configured projectile; a spell's StartCast runs
// for either caster — the cast sound plays for the spell
// branch (MagicCombatMethod.ts:280-285) — and the base XP
// is marked owed for players only (TS adds to
// baseExperienceOwed only when !isNpc, :296-298).
func (m *magicCombatMethod) Start(attacker, target *Mobile) {
	combat := attacker.GetCombat()
	spell := combat.SelectedSpell()
	if spell == nil {
		if combatantFor(attacker).IsPlayer() {
			return
		}
		// NPCs cast without a Spell object
		// (MagicCombatMethod.ts:268-282): the attack
		// animation, then the configured projectile — or
		// the default wind strike — at delay 0, speed 20,
		// heights 43/31. No cast sound on this branch.
		if anim := attacker.GetAttackAnim(target); anim != -1 {
			attacker.PerformAnimation(anim, 0)
		}
		projectileID := DefaultNPCMagicProjectile
		if def := combatantFor(attacker).NPCDef(); def != nil && def.ProjectileID() >= 0 {
			projectileID = def.ProjectileID()
		}
		source := projectileMobile(attacker)
		victim := projectileMobile(target)
		if source != nil && victim != nil {
			CreateProjectile(source, victim, projectileID, 0, 20, 43, 31).SendProjectile()
		}
		return
	}
	// The cast sound (MagicCombatMethod.ts:280-285): the
	// spell's own cast sound, else the table by spell id
	// (Worker C's picker), sent before the cast opens. A
	// zero Sound means no captured cast sound (TS null).
	// It plays for an NPC caster too (TS sends it for any
	// character with a spell).
	sendCombatSound(attacker, CastSoundFor(spell))
	spell.StartCast(attacker, target)
	if combatantFor(attacker).IsPlayer() && (spell.BaseExperience() > 0 || spell.SplashExperience() > 0) {
		combat.setBaseExperienceOwed()
	}
}

// AttackSpeed is MagicCombatMethod.attackSpeed: the previous
// cast's speed, else the fighter's base attack speed.
func (m *magicCombatMethod) AttackSpeed(attacker *Mobile) int {
	if previous := attacker.GetCombat().PreviousCast(); previous != nil {
		if speed := previous.AttackSpeed(); speed > 0 {
			return speed
		}
	}
	return m.BaseCombatMethod.AttackSpeed(attacker)
}

// AttackDistance is MagicCombatMethod.attackDistance: 10.
func (m *magicCombatMethod) AttackDistance(attacker *Mobile) int {
	return 10
}

// Finished is MagicCombatMethod.finished: NPCs return
// immediately (running the teardown reset their combat
// after every swing). Players move the cast spell to
// previous; a non-autocast hand cast resets combat (keeping
// the animation) and keeps facing the target.
func (m *magicCombatMethod) Finished(attacker, target *Mobile) {
	if !combatantFor(attacker).IsPlayer() {
		return
	}
	player := combatantFor(attacker).Player()
	combat := attacker.GetCombat()
	current := combat.CastSpell()
	combat.SetCastSpell(nil)
	if !autocastingActive(player) {
		combat.Reset(false)
		combat.setMobileInteraction(target)
	}
	combat.SetPreviousCast(current)
}

// HandleAfterHitEffects is
// MagicCombatMethod.handleAfterHitEffects: dead attacker or
// target returns; otherwise the spell's FinishCast runs
// with the outcome. The impact presentation runs first:
// the spell's end graphic on an accurate hit, else the
// splash graphic, and the impact or splash area sound —
// unless the graphic (or sound) was already queued for the
// projectile's landing (MagicCombatMethod.ts:358-372).
func (m *magicCombatMethod) HandleAfterHitEffects(hit *PendingHit) {
	attacker := hit.Attacker()
	target := hit.Target()
	if attacker == nil || target == nil {
		return
	}
	if combatantFor(attacker).Hitpoints() <= 0 || combatantFor(target).Hitpoints() <= 0 {
		return
	}
	if spell := hit.Spell(); spell != nil {
		magicImpactPresentation(hit, spell)
		spell.FinishCast(attacker, target, hit.IsAccurate(), hit.TotalDamage())
	}
}

// magicImpactPresentation is the impact half of
// MagicCombatMethod.handleAfterHitEffects
// (MagicCombatMethod.ts:358-372): the end graphic on an
// accurate hit, else the splash graphic (85, MIDDLE), and
// the impact or splash area sound at the target's tile
// (radius 10) — each skipped when the queued path already
// played it (queueMagicImpactGraphic). A spell with no
// impact graphic sends none.
func magicImpactPresentation(hit *PendingHit, spell CombatSpell) {
	target := hit.Target()
	if target == nil {
		return
	}
	var ctx *SpellCastContext
	if player := combatantFor(hit.Attacker()).Player(); player != nil {
		ctx = spellCastContext(player, spell)
	}
	visual := getProjectileVisual(spell, ctx, target)
	queuedGraphic := visual != nil && visual.queued
	queuedSound := visual != nil && visual.soundQueued
	if hit.IsAccurate() {
		if id, height, ok := spellEndGraphic(spell); ok && !queuedGraphic {
			target.PerformGraphic(id, height, 0)
		}
		if !queuedSound {
			// The impact sound (MagicCombatMethod.ts:366-369):
			// the spell's own impact sound, else the table
			// by spell id (Worker C's picker), as an area
			// sound at the target (radius
			// MagicImpactSoundRadius). A zero Sound means no
			// captured impact sound (TS null).
			if sound := ImpactSoundFor(spell); sound.ID() > 0 {
				PlayAreaSoundAt(target, sound, MagicImpactSoundRadius)
			}
		}
	} else {
		if !queuedGraphic {
			target.PerformGraphic(splashGraphicID, graphicHeightMiddle, 0)
		}
		if !queuedSound {
			// The splash sound needs no picker: it is the
			// SPELL_FAIL_SPLASH static (227), an area sound
			// at the target (MagicCombatMethod.ts:370-372).
			PlayAreaSoundAt(target, SoundSpellFailSplash, MagicImpactSoundRadius)
		}
	}
}
