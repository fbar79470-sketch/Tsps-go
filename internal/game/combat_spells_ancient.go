package game

// Ancient Magicks combat spells (Phase 45, Worker P3).
//
// The port of the 16 CombatAncientSpellExtend statics in
// upstream CombatSpells.ts:1720-2058 (commit bfd81e3), in
// declaration order: the 8 teleports in SpellTeleports.ts
// are NOT here (no teleport system exists in Go). The table
// below is the positional-arg constructor flattened
// (CombatSpells.ts:236-261: castAnim, startGraphic,
// effectOnHitCalc, radius, projectile, endGraphic, maxHit,
// baseXP, itemsRequired, levelRequired, spellId,
// impactSound — only Ice Barrage passes an impact sound,
// and the Go sound table already maps it,
// combat_sounds.go:335, so the table carries no sound).
//
// The CombatAncientSpell base (CombatAncientSpell.ts:1-47)
// is ANCIENT-book, never requires equipment, and its
// finishCast calls the no-op spellEffect; the real effect
// runs in spellEffectOnHitCalc, reached through onHitCalc
// (CombatSpell.ts:126-134) — once per hit, primary and
// secondary alike (MagicCombatMethod.ts:142,177), only on
// accurate hits, with the magic:hit-effects /
// magic:hit-effects-applied events around it and the
// EffectMultiplier scaling it. FinishCast here is the same
// no-op; OnHitCalc carries the effect.
//
// Effects (CombatSpells.ts:1720-2058): Smoke = 1-in-8 poison
// (Misc.getRandom(7) === 0), 10 (Rush/Burst) / 20
// (Blitz/Barrage); Shadow = Attack drain 10% (Rush/Burst) /
// 15% (Blitz/Barrage), never reapplying while already
// lowered; Blood = heal 25% of the damage dealt; Ice =
// freeze 8/16/24/32 ticks (Rush/Burst/Blitz/Barrage).
// Radius 0 Rush/Blitz, 1 Burst/Barrage. A landed ice freeze
// emits combat:frozen (CombatFactory.ts:1662) with the ticks
// actually applied, so the Freezes consumer runs its
// aftermath (red message, thaw warning, 10-tile release);
// the freeze's own message is suppressed (empty, not nil —
// Go's FreezeTicks sends its default on nil, while TS sends
// nothing on the null message the ice spells pass).
//
// Presentation: cast anim 1978 (Rush/Blitz) / 1979
// (Burst/Barrage); only Ice Blitz has a start graphic (366,
// HIGH); Rush/Blitz carry a projectile except Ice Blitz;
// Burst/Barrage have none. TS queues the impact visual only
// for the NORMAL book (CombatSpell.ts startCast), so Ancient
// casts never queue one — the end graphic plays at impact
// time through spellEndGraphic (all Ancient end graphics
// are bare `new Graphic(id)`, height LOW, Graphic.ts).
// Cast and impact sounds come from the existing tables by
// spell id (combat_sounds.go); StartCast plays no sound
// itself (the magic method sends CastSoundFor).
//
// XP is the TS int, verbatim; BaseExperience and
// SplashExperience pay the same (splashExperience defaults
// to baseExperience, CombatSpell.ts). The Go int-XP
// truncation gap applies as for standard spells (do NOT
// change XP math; see spellXP, combat_spells.go).
//
// GAPS (same known gaps as the god-spell drains):
//   - Shadow drain on NPC targets is NOT ported: Go NPC
//     levels are definition stats with no setter
//     (combatant.go: "no drains in Go"), so only player
//     targets drain (the godSpellDrain precedent).
//   - The blood-drain chat line ("You drain some of your
//     opponent's health.", MagicCombatMethod.ts:110-122,
//     once per cast, plural for 2+ accurate hits) is NOT
//     sent: it is a per-cast message over all hits, and
//     OnHitCalc runs per hit — it needs a Hits-level seam.
//   - The Ancient sceptre x1.1 effect multiplier arrives
//     through the magic:hit-effects event's EffectMultiplier
//     (default 1); no plugin sets it today.

// ancientEffectKind names the spell's effect family
// (CombatSpells.ts:1720-2058, the third positional arg).
type ancientEffectKind string

const (
	ancientEffectSmoke  ancientEffectKind = "smoke"
	ancientEffectShadow ancientEffectKind = "shadow"
	ancientEffectBlood  ancientEffectKind = "blood"
	ancientEffectIce    ancientEffectKind = "ice"
)

// ancientSpellDef is one CombatAncientSpellExtend static,
// flattened: the CombatAncientSpellExtend positional args
// plus the effect parameters. poisonSeverity is the Smoke
// poison (10/20); drainPercent is the Shadow Attack drain
// (0.10/0.15); freezeTicks is the Ice freeze (8/16/24/32);
// Blood heals a fixed 25% of the damage and carries no
// parameter. members is true for every Ancient spell
// (Spell.ts:78: "other books are all members").
type ancientSpellDef struct {
	name         string
	kind         ancientEffectKind
	id, level    int
	maxHit       int
	baseXP       float64
	runes        []spellRune
	castAnim     int
	startGraphic int
	// startGraphicHeight is the start graphic's height
	// (HIGH only for Ice Blitz's 366; -1 means no graphic).
	startGraphicHeight int
	projectile         int
	endGraphic         int
	radius             int
	poisonSeverity     int
	drainPercent       float64
	freezeTicks        int
}

// ancientSpellDefs is every Ancient combat static in
// upstream declaration order (CombatSpells.ts:1720-2058).
// Rune ids: 554 fire, 555 water, 556 air, 560 death,
// 562 chaos, 565 blood, 566 soul.
var ancientSpellDefs = []ancientSpellDef{
	{
		name: "SMOKE_RUSH", kind: ancientEffectSmoke, id: 12939, level: 50, maxHit: 13,
		baseXP: 30, runes: []spellRune{{556, 1}, {554, 1}, {562, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 384, endGraphic: 385,
		radius: 0, poisonSeverity: 10,
	},
	{
		name: "SHADOW_RUSH", kind: ancientEffectShadow, id: 12987, level: 52, maxHit: 14,
		baseXP: 31, runes: []spellRune{{556, 1}, {566, 1}, {562, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 378, endGraphic: 379,
		radius: 0, drainPercent: 0.10,
	},
	{
		name: "BLOOD_RUSH", kind: ancientEffectBlood, id: 12901, level: 56, maxHit: 15,
		baseXP: 33, runes: []spellRune{{565, 1}, {562, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 372, endGraphic: 373,
		radius: 0,
	},
	{
		name: "ICE_RUSH", kind: ancientEffectIce, id: 12861, level: 58, maxHit: 16,
		baseXP: 34, runes: []spellRune{{555, 2}, {562, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 360, endGraphic: 361,
		radius: 0, freezeTicks: 8,
	},
	{
		name: "SMOKE_BURST", kind: ancientEffectSmoke, id: 12963, level: 62, maxHit: 17,
		baseXP: 36, runes: []spellRune{{556, 2}, {554, 2}, {562, 4}, {560, 2}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 389,
		radius: 1, poisonSeverity: 10,
	},
	{
		name: "SHADOW_BURST", kind: ancientEffectShadow, id: 13011, level: 64, maxHit: 18,
		baseXP: 37, runes: []spellRune{{556, 1}, {566, 2}, {562, 4}, {560, 2}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 382,
		radius: 1, drainPercent: 0.10,
	},
	{
		name: "BLOOD_BURST", kind: ancientEffectBlood, id: 12919, level: 68, maxHit: 21,
		baseXP: 39, runes: []spellRune{{565, 2}, {562, 4}, {560, 2}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 376,
		radius: 1,
	},
	{
		name: "ICE_BURST", kind: ancientEffectIce, id: 12881, level: 70, maxHit: 22,
		baseXP: 40, runes: []spellRune{{555, 4}, {562, 4}, {560, 2}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 363,
		radius: 1, freezeTicks: 16,
	},
	{
		name: "SMOKE_BLITZ", kind: ancientEffectSmoke, id: 12951, level: 74, maxHit: 23,
		baseXP: 42, runes: []spellRune{{556, 2}, {554, 2}, {565, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 386, endGraphic: 387,
		radius: 0, poisonSeverity: 20,
	},
	{
		name: "SHADOW_BLITZ", kind: ancientEffectShadow, id: 12999, level: 76, maxHit: 24,
		baseXP: 43, runes: []spellRune{{556, 2}, {566, 2}, {565, 2}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 380, endGraphic: 381,
		radius: 0, drainPercent: 0.15,
	},
	{
		name: "BLOOD_BLITZ", kind: ancientEffectBlood, id: 12911, level: 80, maxHit: 25,
		baseXP: 45, runes: []spellRune{{565, 4}, {560, 2}},
		castAnim: 1978, startGraphic: -1, startGraphicHeight: -1, projectile: 374, endGraphic: 375,
		radius: 0,
	},
	{
		name: "ICE_BLITZ", kind: ancientEffectIce, id: 12871, level: 82, maxHit: 26,
		baseXP: 46, runes: []spellRune{{555, 3}, {565, 2}, {560, 2}},
		castAnim: 1978, startGraphic: 366, startGraphicHeight: graphicHeightHigh, projectile: -1, endGraphic: 367,
		radius: 0, freezeTicks: 24,
	},
	{
		name: "SMOKE_BARRAGE", kind: ancientEffectSmoke, id: 12975, level: 86, maxHit: 27,
		baseXP: 48, runes: []spellRune{{556, 4}, {554, 4}, {565, 2}, {560, 4}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 391,
		radius: 1, poisonSeverity: 20,
	},
	{
		name: "SHADOW_BARRAGE", kind: ancientEffectShadow, id: 13023, level: 88, maxHit: 28,
		baseXP: 49, runes: []spellRune{{556, 4}, {566, 3}, {565, 2}, {560, 4}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 383,
		radius: 1, drainPercent: 0.15,
	},
	{
		name: "BLOOD_BARRAGE", kind: ancientEffectBlood, id: 12929, level: 92, maxHit: 29,
		baseXP: 51, runes: []spellRune{{560, 4}, {566, 1}, {565, 4}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 377,
		radius: 1,
	},
	{
		name: "ICE_BARRAGE", kind: ancientEffectIce, id: 12891, level: 94, maxHit: 30,
		baseXP: 52, runes: []spellRune{{555, 6}, {565, 2}, {560, 4}},
		castAnim: 1979, startGraphic: -1, startGraphicHeight: -1, projectile: -1, endGraphic: 369,
		radius: 1, freezeTicks: 32,
	},
}

// ancientSpell is CombatAncientSpellExtend: an Ancient
// combat spell behind the CombatSpell interface.
type ancientSpell struct {
	base *spellBase
	def  *ancientSpellDef
}

func (s *ancientSpell) SpellID() int          { return s.base.id }
func (s *ancientSpell) MaximumHit() int       { return s.def.maxHit }
func (s *ancientSpell) BaseExperience() int   { return spellXP(s.def.baseXP) }
func (s *ancientSpell) SplashExperience() int { return spellXP(s.def.baseXP) }

// AttackSpeed is CombatSpell.getAttackSpeed
// (CombatSpell.ts:137-139): 5 for every combat spell.
func (s *ancientSpell) AttackSpeed() int { return 5 }

// HitDelay is CombatSpell.hitDelay (CombatSpell.ts:147-149):
// -1 (no fixed delay; the magic method computes it from
// distance).
func (s *ancientSpell) HitDelay() int { return -1 }

// CanCast is Spell.canCast through the shared base (the
// ANCIENT book, the level and the rune cost).
func (s *ancientSpell) CanCast(caster *Player, consume bool) bool {
	return canCastSpell(caster, s.base, s, consume)
}

// CanCastOnTarget is CombatSpell.canCastOnTarget
// (CombatSpell.ts:31-33): always true — no Ancient spell
// overrides it.
func (s *ancientSpell) CanCastOnTarget(caster *Player, target *Mobile) bool { return true }

// Spellbook is CombatAncientSpell.getSpellbook
// (CombatAncientSpell.ts:9-11): ANCIENT. spellbookOfSpell
// reads it (autocasting.go), which also keeps the queued
// impact visual off: TS queues one only for the NORMAL
// book (CombatSpell.ts startCast).
func (s *ancientSpell) Spellbook() Spellbook { return SpellbookAncient }

// StartCast is CombatSpell.startCast (CombatSpell.ts:50-101)
// for an Ancient spell: the cast animation (with its staff
// variant — 1978/1979 have none, so the base anim plays),
// the start graphic (only Ice Blitz has one), and the cast
// projectile (Rush/Blitz except Ice Blitz; Burst/Barrage
// have none). No impact visual is queued: TS queues one
// only for the NORMAL book, so the end graphic plays at
// impact time (magicImpactPresentation, combat_magic.go).
func (s *ancientSpell) StartCast(caster *Mobile, target *Mobile) {
	if caster == nil || target == nil {
		return
	}
	if anim := castAnimationFor(caster, s.def.castAnim); anim >= 0 {
		caster.PerformAnimation(anim, 0)
	}
	if s.def.startGraphic >= 0 && !npcSkipsStartGraphic(caster) {
		caster.PerformGraphic(s.def.startGraphic, s.def.startGraphicHeight, 0)
	}
	if s.def.projectile < 0 {
		return
	}
	source := projectileMobile(caster)
	victim := projectileMobile(target)
	if source == nil || victim == nil {
		return
	}
	CreateProjectile(source, victim, s.def.projectile, 0, 20, 43, 31).SendProjectile()
}

// FinishCast is CombatAncientSpell.finishCast
// (CombatAncientSpell.ts:13-22): nothing on a splash or a
// 0-damage hit, otherwise the no-op spellEffect — the real
// effect runs in OnHitCalc (spellEffectOnHitCalc).
func (s *ancientSpell) FinishCast(attacker, target *Mobile, accurate bool, damage int) {}

// OnHitCalc is CombatSpell.onHitCalc (CombatSpell.ts:126-134)
// with the Ancient spellEffectOnHitCalc in the middle: the
// magic:hit-effects event, then the spell's effect with the
// rolled damage and the event's multiplier, then
// magic:hit-effects-applied. A vetoed effect (Allowed false)
// or a splash runs nothing. Mirrors emitHitEffects
// (combat_spells.go), which has no middle call because no
// standard spell overrides spellEffectOnHitCalc.
func (s *ancientSpell) OnHitCalc(hit *PendingHit) {
	if hit == nil || !hit.IsAccurate() {
		return
	}
	effect := &MagicHitEffectsPayload{
		Attacker: hit.Attacker(), Target: hit.Target(), Spell: s,
		Hit: hit, Allowed: true, EffectMultiplier: 1,
	}
	emitCustomEvent(hit.Attacker(), hit.Target(), "magic:hit-effects", effect)
	if !effect.Allowed {
		return
	}
	s.spellEffectOnHitCalc(hit.Attacker(), hit.Target(), hit.TotalDamage(), effect.EffectMultiplier)
	emitCustomEvent(hit.Attacker(), hit.Target(), "magic:hit-effects-applied", effect)
}

// SpellRadius is CombatAncientSpell.spellRadius
// (CombatAncientSpell.ts:45-47): 1 for Burst/Barrage, 0 for
// Rush/Blitz.
func (s *ancientSpell) SpellRadius() int { return s.def.radius }

// spellEffectOnHitCalc is the CombatAncientSpellExtend
// effect (CombatSpells.ts:1720-2058, third positional arg),
// with the TS multiplier (the Ancient sceptre's x1.1
// arrives here; 1 by default). All amounts floor, as TS
// does (Math.floor on the poison severity, the drain and
// the freeze ticks; Go int conversion truncates toward
// zero, which is the same floor for non-negative values).
func (s *ancientSpell) spellEffectOnHitCalc(cast, target *Mobile, damage int, multiplier float64) {
	switch s.def.kind {
	case ancientEffectSmoke:
		// 1-in-8 (Misc.getRandom(7) === 0, Misc.ts:221-223:
		// floor(random * 8), so 0..7 inclusive).
		if randomInclusive(7) == 0 {
			PoisonEntity(target, int(float64(s.def.poisonSeverity)*multiplier), 1)
		}
	case ancientEffectShadow:
		shadowAttackDrain(target, s.def.drainPercent, multiplier)
	case ancientEffectBlood:
		healMobile(cast, int(float64(damage)*0.25*multiplier))
	case ancientEffectIce:
		// TS passes a null message (which sends nothing —
		// the red message comes from the Freezes consumer
		// on combat:frozen) and kind "ice"
		// (CombatFactory.ts:1620-1662). Go's FreezeTicks
		// sends its default "You have been frozen!" on a
		// nil message, so pass an empty message to suppress
		// it — exactly as the bind consumer does
		// (standardeffects/bindings.go) — and emit
		// combat:frozen on a landed freeze, with the
		// ticks actually applied (after resistance
		// shortening, as TS emits them). A freeze that
		// does not land (already frozen, immune,
		// resistant, too large) emits nothing.
		if target == nil {
			return
		}
		ticks := int(float64(s.def.freezeTicks) * multiplier)
		hadFreeze := target.GetTimers().Has(TimerKeyFreeze)
		empty := ""
		FreezeTicks(target, ticks, &empty, cast, "ice")
		if !hadFreeze && target.GetTimers().Has(TimerKeyFreeze) {
			emitFrozen(cast, target, target.GetTimers().Left(TimerKeyFreeze), "ice")
		}
	}
}

// emitFrozen emits the "combat:frozen" event for a freeze an
// Ancient ice spell landed (CombatFactory.ts:1662). TS emits
// it from freezeTicks itself; Go's FreezeTicks does not, so
// the applying consumer emits it — the bind consumer for
// binds (standardeffects/bindings.go), here for ice. The
// Freezes consumer owns the aftermath (the red message, the
// thaw warning, the 10-tile release).
func emitFrozen(cast, target *Mobile, ticks int, kind string) {
	emitCustomEvent(cast, target, CombatFrozenEventName, &CombatFrozenPayload{
		Target: target,
		Source: cast,
		Ticks:  ticks,
		Kind:   kind,
	})
}

// shadowAttackDrain is the Shadow-spell drain
// (CombatSpells.ts:351-367, shadowDrainNpc + the per-spell
// player branch): a hit lowers the target's Attack by 10%
// (Rush/Burst) or 15% (Blitz/Barrage) of its current level,
// never reapplying while already lowered (current < max
// returns). Player targets only: the NPC branch needs an
// NPC level setter Go does not have (the godSpellDrain
// precedent, combat_spells.go).
func shadowAttackDrain(target *Mobile, percent, multiplier float64) {
	victim := combatantFor(target).Player()
	if victim == nil || victim.Skills() == nil {
		return
	}
	skills := victim.Skills()
	current := skills.CurrentLevel(SkillAttack)
	if current < skills.MaxLevel(SkillAttack) {
		return
	}
	decrease := int(percent * multiplier * float64(current))
	skills.SetCurrentLevelCombat(SkillAttack, max(0, current-decrease))
}

// healMobile is Mobile.heal (Player.ts:378-382,
// NPC.ts:492-498): restore hitpoints, capped at the max —
// a boosted player keeps its boosted cap (the max of
// current and max level), an NPC caps at its definition
// maximum. A non-positive amount heals nothing.
func healMobile(caster *Mobile, amount int) {
	if caster == nil || amount <= 0 {
		return
	}
	c := combatantFor(caster)
	if c == nil {
		return
	}
	current := c.Hitpoints()
	cap := c.MaxHitpoints()
	if c.IsPlayer() && current > cap {
		cap = current
	}
	c.SetHitpoints(min(current+amount, cap))
}

// ---- Table -> spells ----

// ancientSpellDefOf returns the Ancient-book table row
// behind a spell, or nil for a spell that is not an
// ancientSpell (content spells carry no table row, so they
// carry no end graphic id). Mirrors standardSpellDefOf
// (combat_spells.go).
func ancientSpellDefOf(spell CombatSpell) *ancientSpellDef {
	if s, ok := spell.(*ancientSpell); ok {
		return s.def
	}
	return nil
}

// spellEndGraphic returns the impact-time end graphic (id,
// height) for a spell: the standard-book row, else the
// Ancient-book row (bare `new Graphic(id)`, height LOW,
// Graphic.ts). False when the spell has no end graphic.
// The two call sites in combat_magic.go (the queued and the
// impact-time paths) read it instead of the standard-only
// standardSpellDefOf, so Ancient end graphics play.
func spellEndGraphic(spell CombatSpell) (id, height int, ok bool) {
	if def := standardSpellDefOf(spell); def != nil && def.endGraphic >= 0 {
		return def.endGraphic, spellEndGraphicHeight(spell), true
	}
	if def := ancientSpellDefOf(spell); def != nil && def.endGraphic >= 0 {
		return def.endGraphic, graphicHeightLow, true
	}
	return 0, 0, false
}

// newAncientSpellFromDef builds the CombatSpell for one
// table entry (the CombatAncientSpellExtend positional
// args, CombatSpells.ts:236-261). The spellbook is ANCIENT
// for every Ancient spell (CombatAncientSpell.ts:9-11);
// Ancient spells never require equipment
// (CombatAncientSpell.ts:24-28).
func newAncientSpellFromDef(def *ancientSpellDef) CombatSpell {
	base := &spellBase{
		id:                def.id,
		level:             def.level,
		runes:             runesToItems(def.runes),
		equipmentRequired: nil,
		spellbook:         SpellbookAncient,
		members:           true,
		sharedDelay:       true,
		offensive:         true,
	}
	return &ancientSpell{base: base, def: def}
}

// ancientCombatSpells are the 16 Ancient combat spells,
// singletons built once from the table in declaration order.
// The registry holds these same pointers, so identity
// comparisons (autocasting.go: autocastSpellIndex) see the
// table's spells.
var ancientCombatSpells = func() []CombatSpell {
	out := make([]CombatSpell, 0, len(ancientSpellDefs))
	for i := range ancientSpellDefs {
		out = append(out, newAncientSpellFromDef(&ancientSpellDefs[i]))
	}
	return out
}()

// InstallAncientSpells registers the Ancient spellbook with
// the resolvers (spell_resolve.go:
// RegisterCombatSpellNamed). Call once at boot, before
// ticking starts — the cmd/server wiring calls it next to
// InstallStandardSpells. Re-registering the same singletons
// is a no-op in effect. Combat spells register under id and
// name (CombatSpells.getCombatSpell / getCombatSpellByName,
// CombatSpells.ts:2068-2116); registering lights up the
// autocast slots 31-46, which already map to these ids
// (autocasting.go).
func InstallAncientSpells() {
	for _, spell := range ancientCombatSpells {
		s, ok := spell.(*ancientSpell)
		if !ok || s.def == nil {
			continue
		}
		RegisterCombatSpellNamed(s.def.id, s.def.name, spell)
	}
}

// uninstallAncientSpells removes every Ancient registration
// InstallAncientSpells made. The registry is process-global,
// so a test that installs the table must unregister it on
// cleanup — otherwise a later test that expects an empty
// registry (TestCombatSpellRegistry) sees the real spells
// and fails.
func uninstallAncientSpells() {
	for _, def := range ancientSpellDefs {
		UnregisterCombatSpell(def.id)
	}
}
