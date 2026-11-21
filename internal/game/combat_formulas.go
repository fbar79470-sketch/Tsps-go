package game

import "math"

// Damage and accuracy formulas: the port of
// game/content/combat/formula/DamageFormulas.ts (418 lines) and
// AccuracyFormulasDpsCalc.ts (741 lines). Every constant and
// term is grounded in those files; the per-cycle roll cache TS
// keeps (getRollCache) is NOT ported — Go recomputes every roll
// (documented deviation: identical results, more arithmetic;
// the cache exists in TS only to avoid recomputation within one
// cycle, and Go has no per-cycle cache invalidation point).
//
// EquipmentEffects (salve, slayer helm, void, Dharok, bolt
// effects, demonbane, charge spells) is IDENTITY throughout:
// every apply*Modifiers call in TS is a no-op in Go, because
// no equipment-effect system exists. The call sites are noted
// where TS applies them, so the effects land in exactly one
// place each when a later milestone ports them. The
// combat:magic-base-max-hit, combat:magic-defence-roll,
// combat:magic-attack-roll(-modify), and modifyRangedAttackRoll
// plugin events are likewise skipped (no hook fields); the
// combat:defence-roll, combat:accuracy-rolled,
// combat:magic-max-hit, and combat:magic-hit-bounds events are
// exposed through Hooks (see combat_hooks.go).

// ---- Prayer percents (exact if/else chains) ----

// meleeAttackPrayerPercent is meleeAttackPrayerPercent
// (AccuracyFormulasDpsCalc.ts): Clarity 105, Improved 110,
// Incredible 115, Chivalry 115, Piety 120, Zeal 125.
func meleeAttackPrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerClarityOfThought):
		return 105
	case PrayerActive(m, PrayerImprovedReflexes):
		return 110
	case PrayerActive(m, PrayerIncredibleReflexes):
		return 115
	case PrayerActive(m, PrayerChivalry):
		return 115
	case PrayerActive(m, PrayerPiety):
		return 120
	case PrayerActive(m, PrayerZeal):
		return 125
	default:
		return 100
	}
}

// meleeStrengthPrayerPercent is meleeStrengthPrayerPercent
// (DamageFormulas.ts): Burst 105, Superhuman 110, Ultimate 115,
// Chivalry 118, Piety 123, Zeal 128.
func meleeStrengthPrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerBurstOfStrength):
		return 105
	case PrayerActive(m, PrayerSuperhumanStrength):
		return 110
	case PrayerActive(m, PrayerUltimateStrength):
		return 115
	case PrayerActive(m, PrayerChivalry):
		return 118
	case PrayerActive(m, PrayerPiety):
		return 123
	case PrayerActive(m, PrayerZeal):
		return 128
	default:
		return 100
	}
}

// defencePrayerPercent is defencePrayerPercent
// (AccuracyFormulasDpsCalc.ts): the base skin-prayer percent
// plus 5 when Deadeye or Mystic Vigour is active (the only
// Defence boosts that stack).
func defencePrayerPercent(m *Mobile) int {
	upgrade := 0
	if IsDeadeye(m) || IsMysticVigour(m) {
		upgrade = 5
	}
	return baseDefencePrayerPercent(m) + upgrade
}

func baseDefencePrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerThickSkin):
		return 105
	case PrayerActive(m, PrayerRockSkin):
		return 110
	case PrayerActive(m, PrayerSteelSkin):
		return 115
	case PrayerActive(m, PrayerChivalry):
		return 120
	case PrayerActive(m, PrayerPiety), PrayerActive(m, PrayerZeal):
		return 125
	case PrayerActive(m, PrayerRigour):
		return 125
	case PrayerActive(m, PrayerAugury):
		return 125
	default:
		return 100
	}
}

// rangedAttackPrayerPercent is rangedAttackPrayerPercent:
// Sharp 105, Hawk 110, Eagle 115 (Deadeye 118), Rigour 120.
func rangedAttackPrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerSharpEye):
		return 105
	case PrayerActive(m, PrayerHawkEye):
		return 110
	case PrayerActive(m, PrayerEagleEye):
		if IsDeadeye(m) {
			return 118
		}
		return 115
	case PrayerActive(m, PrayerRigour):
		return 120
	default:
		return 100
	}
}

// rangedStrengthPrayerPercent is rangedStrengthPrayerPercent
// (DamageFormulas.ts): Sharp 105, Hawk 110, Eagle 115 (Deadeye
// 118), Rigour 123.
func rangedStrengthPrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerSharpEye):
		return 105
	case PrayerActive(m, PrayerHawkEye):
		return 110
	case PrayerActive(m, PrayerEagleEye):
		if IsDeadeye(m) {
			return 118
		}
		return 115
	case PrayerActive(m, PrayerRigour):
		return 123
	default:
		return 100
	}
}

// magicAttackPrayerPercent is magicAttackPrayerPercent:
// Mystic Will 105, Lore 110, Might 115 (Vigour 118), Augury 125.
func magicAttackPrayerPercent(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerMysticWill):
		return 105
	case PrayerActive(m, PrayerMysticLore):
		return 110
	case PrayerActive(m, PrayerMysticMight):
		if IsMysticVigour(m) {
			return 118
		}
		return 115
	case PrayerActive(m, PrayerAugury):
		return 125
	default:
		return 100
	}
}

// magicDamagePrayerPermille is magicDamagePrayerPermille
// (DamageFormulas.ts): Augury 40, Mystic Might 20 (Vigour 30),
// Mystic Lore 10.
func magicDamagePrayerPermille(m *Mobile) int {
	switch {
	case PrayerActive(m, PrayerAugury):
		return 40
	case PrayerActive(m, PrayerMysticMight):
		if IsMysticVigour(m) {
			return 30
		}
		return 20
	case PrayerActive(m, PrayerMysticLore):
		return 10
	default:
		return 0
	}
}

// ---- Effective levels ----

// fightStyleOf returns the fighter's current fight style: the
// player's Combat fight type, or ACCURATE for NPCs (whose
// branches never read a style).
func fightStyleOf(c Combatant) FightStyle {
	if c == nil || !c.IsPlayer() || c.Mobile() == nil {
		return FightStyleAccurate
	}
	return c.Mobile().GetCombat().FightType().Style
}

// fightBonusTypeOf returns the fighter's fight-type bonus
// index (players) or BonusAttackCrush for NPCs (unused: NPC
// branches read stats directly).
func fightBonusTypeOf(c Combatant) int {
	if c == nil || !c.IsPlayer() || c.Mobile() == nil {
		return BonusAttackCrush
	}
	return c.Mobile().GetCombat().FightType().BonusType
}

// defenceStanceBonus is defenceStanceBonus
// (AccuracyFormulasDpsCalc.ts): autocasting gives nothing;
// Defensive +3, Controlled +1.
func defenceStanceBonus(c Combatant) int {
	if c == nil || !c.IsPlayer() || c.Mobile() == nil {
		return 0
	}
	if c.Mobile().GetCombat().AutoCastSpell() != nil {
		return 0
	}
	switch fightStyleOf(c) {
	case FightStyleDefensive:
		return 3
	case FightStyleControlled:
		return 1
	default:
		return 0
	}
}

// effectiveAttackLevel is effectiveAttackLevel: NPCs read
// Attack + 9; players scale the visible Attack level by the
// attack-level trait multiplier, then by the melee attack
// prayer percent, then add the style bonus (Accurate +3,
// Controlled +1) and 8.
func effectiveAttackLevel(attacker Combatant) int {
	if !attacker.IsPlayer() {
		return attacker.CurrentLevel(SkillAttack) + 9
	}
	traits := ActiveTraitsFor(attacker)
	visible := attacker.CurrentLevel(SkillAttack)
	if traits != nil && traits.AttackLevelMultiplier != nil {
		visible = int(math.Floor(float64(visible) * *traits.AttackLevelMultiplier))
	}
	att := scalePercent(visible, meleeAttackPrayerPercent(attacker.Mobile()))
	switch fightStyleOf(attacker) {
	case FightStyleAccurate:
		att += 3
	case FightStyleControlled:
		att += 1
	}
	att += 8
	return att
}

// effectiveDefenseLevel is effectiveDefenseLevel: NPCs read
// Defence + 9; players scale Defence by the defence prayer
// percent, then add the stance bonus and 8.
func effectiveDefenseLevel(defender Combatant) int {
	if !defender.IsPlayer() {
		return defender.CurrentLevel(SkillDefence) + 9
	}
	def := scalePercent(defender.CurrentLevel(SkillDefence), defencePrayerPercent(defender.Mobile()))
	def += defenceStanceBonus(defender) + 8
	return def
}

// effectiveRangedAttack is effectiveRangedAttack: NPCs read
// Ranged + 9; players scale Ranged by the ranged attack prayer
// percent, add Accurate's +3, and 8.
func effectiveRangedAttack(attacker Combatant) int {
	if !attacker.IsPlayer() {
		return attacker.CurrentLevel(SkillRanged) + 9
	}
	rng := scalePercent(attacker.CurrentLevel(SkillRanged), rangedAttackPrayerPercent(attacker.Mobile()))
	if fightStyleOf(attacker) == FightStyleAccurate {
		rng += 3
	}
	rng += 8
	return rng
}

// effectiveMagicLevel is effectiveMagicLevel: NPCs read Magic
// + 9; players scale Magic by the magic attack prayer percent,
// add 9 (+8 base and +1 for every cast), and 2 more for a
// powered staff on Accurate (fight bonus type MAGIC, Accurate
// style).
func effectiveMagicLevel(attacker Combatant) int {
	if !attacker.IsPlayer() {
		return attacker.CurrentLevel(SkillMagic) + 9
	}
	mag := scalePercent(attacker.CurrentLevel(SkillMagic), magicAttackPrayerPercent(attacker.Mobile()))
	mag += 9
	if fightBonusTypeOf(attacker) == BonusAttackMagic && fightStyleOf(attacker) == FightStyleAccurate {
		mag += 2
	}
	return mag
}

// effectiveStrengthLevel is effectiveStrengthLevel
// (DamageFormulas.ts): the visible Strength level scaled by
// the strength-level trait multiplier, then by the melee
// strength prayer percent, plus the style bonus (Aggressive
// +3, Controlled +1) and 8.
func effectiveStrengthLevel(attacker Combatant, visibleMultiplier *float64) int {
	visible := attacker.CurrentLevel(SkillStrength)
	if visibleMultiplier != nil {
		visible = int(math.Floor(float64(visible) * *visibleMultiplier))
	}
	prayerAdjusted := scalePercent(visible, meleeStrengthPrayerPercent(attacker.Mobile()))
	styleBonus := 0
	switch fightStyleOf(attacker) {
	case FightStyleAggressive:
		styleBonus = 3
	case FightStyleControlled:
		styleBonus = 1
	}
	return prayerAdjusted + styleBonus + 8
}

// effectiveRangedStrength is effectiveRangedStrength
// (DamageFormulas.ts): the damage skill's visible level scaled
// by the strength-level trait multiplier, then by the ranged
// strength prayer percent, plus Accurate's +3 (when the style
// bonus applies) and 8. damageSkill is RANGED, or STRENGTH
// for uses-melee-strength weapons.
func effectiveRangedStrength(attacker Combatant, visibleMultiplier *float64, damageSkill int, styleBonusApplies bool) int {
	visible := attacker.CurrentLevel(damageSkill)
	if visibleMultiplier != nil {
		visible = int(math.Floor(float64(visible) * *visibleMultiplier))
	}
	prayerAdjusted := scalePercent(visible, rangedStrengthPrayerPercent(attacker.Mobile()))
	styleBonus := 0
	if styleBonusApplies && fightStyleOf(attacker) == FightStyleAccurate {
		styleBonus = 3
	}
	return prayerAdjusted + styleBonus + 8
}

// ---- Attack / defence rolls ----

// applyAccuracyTraits is applyAccuracyTraits
// (AccuracyFormulasDpsCalc.ts): a special's accuracy traits
// scale a finished attack roll. Players only, only when a
// special is activated; with no traits the special's method
// type must match the roll's combat type. Stages apply in
// order with floor rounding; otherwise the single multiplier
// (traits, else the legacy special multiplier, else 1) applies
// via scaleSpecial.
func applyAccuracyTraits(attRoll int, attacker Combatant, combatType CombatType, traits *WeaponSpecialTraits) int {
	if !attacker.IsPlayer() {
		return attRoll
	}
	combat := attacker.Mobile().GetCombat()
	special := combat.GetCombatSpecial()
	if special == nil || !combat.IsSpecialActivated() {
		return attRoll
	}
	if traits == nil && !specialMethodMatches(special, combatType) {
		return attRoll
	}
	if traits != nil && len(traits.AccuracyMultiplierStages) > 0 {
		for _, mult := range traits.AccuracyMultiplierStages {
			attRoll = int(math.Floor(float64(attRoll) * mult))
		}
		return attRoll
	}
	multiplier := 1.0
	if traits != nil && traits.AccuracyMultiplier != nil {
		multiplier = *traits.AccuracyMultiplier
	} else if special.AccuracyMultiplier != nil {
		multiplier = *special.AccuracyMultiplier
	}
	return scaleSpecial(attRoll, multiplier)
}

// specialMethodMatches reports whether a special's combat
// method is of the given type. A nil method matches any type:
// TS specials always carry a method, and a Go registration
// without one means "the weapon's ordinary method".
func specialMethodMatches(special *CombatSpecial, t CombatType) bool {
	if special == nil || special.Method == nil {
		return true
	}
	return special.Method.Type() == t
}

// attackMeleeRoll is attackMeleeRoll: the effective attack
// level times (bonus + 64), where the bonus is the fight
// style's (or trait's) stab/slash/crush bonus, or the max of
// the three for any other bonus type. NPCs use the single
// melee bonus (stats[5]).
func attackMeleeRoll(attacker Combatant) int {
	attRoll := effectiveAttackLevel(attacker)
	if !attacker.IsPlayer() {
		return int(math.Floor(float64(attRoll * (attacker.AttackBonus(BonusAttackStab) + 64))))
	}
	traits := ActiveTraitsFor(attacker)
	bonusType := fightBonusTypeOf(attacker)
	if traits != nil && traits.MeleeAttackBonusIndex != nil {
		bonusType = *traits.MeleeAttackBonusIndex
	}
	switch bonusType {
	case BonusAttackStab, BonusAttackSlash, BonusAttackCrush:
		attRoll *= attacker.AttackBonus(bonusType) + 64
	default:
		maxAtt := attacker.AttackBonus(BonusAttackStab)
		if v := attacker.AttackBonus(BonusAttackSlash); v > maxAtt {
			maxAtt = v
		}
		if v := attacker.AttackBonus(BonusAttackCrush); v > maxAtt {
			maxAtt = v
		}
		attRoll *= maxAtt + 64
	}
	// applyMeleeAttackAccuracyModifiers: identity (no
	// equipment effects). See the file header.
	return applyAccuracyTraits(int(math.Floor(float64(attRoll))), attacker, CombatTypeMelee, traits)
}

// defenseMeleeRoll is defenseMeleeRoll: the effective defence
// level times (bonus + 64), where the bonus is the stab/slash/
// crush defence bonus named by bonusType, or the max of the
// three for any other value (including 3, the NPC-attacker
// default).
func defenseMeleeRoll(defender Combatant, bonusType int) int {
	defLevel := effectiveDefenseLevel(defender)
	defStab := defender.DefenceBonus(BonusAttackStab)
	defSlash := defender.DefenceBonus(BonusAttackSlash)
	defCrush := defender.DefenceBonus(BonusAttackCrush)
	switch bonusType {
	case BonusAttackStab:
		defLevel *= defStab + 64
	case BonusAttackSlash:
		defLevel *= defSlash + 64
	case BonusAttackCrush:
		defLevel *= defCrush + 64
	default:
		maxDef := defStab
		if defSlash > maxDef {
			maxDef = defSlash
		}
		if defCrush > maxDef {
			maxDef = defCrush
		}
		defLevel *= maxDef + 64
	}
	return int(math.Floor(float64(defLevel)))
}

// defenseRangedRoll is defenseRangedRoll: the effective
// defence level (or the projected level, already effective,
// when given) times (ranged defence bonus + 64). kind selects
// the NPC ranged defence bonus kind ("light"/"standard"/
// "heavy"); "" reads as "standard".
func defenseRangedRoll(defender Combatant, kind string, projectedDefenceLevel *int) int {
	var defLevel int
	if projectedDefenceLevel != nil {
		defLevel = *projectedDefenceLevel
		if defLevel < 0 {
			defLevel = 0
		}
	} else {
		defLevel = effectiveDefenseLevel(defender)
	}
	var defRange int
	if !defender.IsPlayer() && kind != "" && kind != "standard" {
		if d := defender.NPCDef(); d != nil {
			defRange = d.RangedDefenceBonus(kind)
		} else {
			defRange = defender.DefenceBonus(BonusAttackRange)
		}
	} else {
		defRange = defender.DefenceBonus(BonusAttackRange)
	}
	// applyRangedDefenseModifiers: identity. See file header.
	defLevel *= defRange + 64
	return int(math.Floor(float64(defLevel)))
}

// attackRangedRoll is attackRangedRoll: the effective ranged
// level times (ranged attack bonus + 64).
func attackRangedRoll(attacker Combatant) int {
	attRoll := effectiveRangedAttack(attacker) * (attacker.AttackBonus(BonusAttackRange) + 64)
	// applyRangedAttackAccuracyModifiers: identity for
	// players. See file header.
	return applyAccuracyTraits(int(math.Floor(float64(attRoll))), attacker, CombatTypeRanged, ActiveTraitsFor(attacker))
}

// defenseMagicRoll is defenseMagicRoll: NPCs read Magic + 9;
// players blend 70% prayer-adjusted Magic and 30%
// prayer-adjusted Defence (each floored), plus the stance
// bonus and 8 — times (magic defence bonus + 64).
func defenseMagicRoll(defender Combatant) int {
	var defLevel int
	if !defender.IsPlayer() {
		defLevel = defender.CurrentLevel(SkillMagic) + 9
	} else {
		magicLevel := scalePercent(defender.CurrentLevel(SkillMagic), magicAttackPrayerPercent(defender.Mobile()))
		defenceLevel := scalePercent(defender.CurrentLevel(SkillDefence), defencePrayerPercent(defender.Mobile()))
		defLevel = scaleRatio(magicLevel, 7, 10) + scaleRatio(defenceLevel, 3, 10) + defenceStanceBonus(defender) + 8
		// applyMagicDefenseModifiers: identity. See header.
	}
	defLevel *= defender.DefenceBonus(BonusAttackMagic) + 64
	return int(math.Floor(float64(defLevel)))
}

// attackMagicRoll is attackMagicRoll: the effective magic level
// times (magic attack bonus + 64), then the accuracy traits.
// The magic accuracy percent modifiers, demonbane multiplier,
// and combat:magic-attack-roll events are skipped (see the
// file header).
func attackMagicRoll(attacker Combatant) int {
	attRoll := effectiveMagicLevel(attacker) * (attacker.AttackBonus(BonusAttackMagic) + 64)
	return applyAccuracyTraits(int(math.Floor(float64(attRoll))), attacker, CombatTypeMagic, ActiveTraitsFor(attacker))
}

// ---- Accuracy resolution ----

// scopedAccuracyScale is AccuracyFormulasDpsCalc
// .scopedAccuracyScale: the withAccuracyScale multiplier
// applied to every attack roll while set (halberd Sweep's
// reduced second hit). It is process-wide in TS too.
var scopedAccuracyScale = 1.0

// WithAccuracyScale runs build with every accuracy roll's
// attack roll scaled (AccuracyFormulasDpsCalc.withAccuracyScale),
// restoring the previous scale afterwards, and returns
// build's result.
func WithAccuracyScale[T any](scale float64, build func() T) T {
	previous := scopedAccuracyScale
	scopedAccuracyScale = scale
	defer func() { scopedAccuracyScale = previous }()
	return build()
}

// AccuracyRolls is the {attack, defence} pair specialRolls
// resolves.
type AccuracyRolls struct {
	Attack  int
	Defence int
}

// SpecialRolls is specialRolls: the attacker and target rolls
// for an attack, honouring special traits. defenceType is the
// per-method defence override (nil = the attack style);
// rangedDefenceType selects the NPC ranged defence kind
// ("" = standard); projectedDefenceLevel overrides the
// defender's effective defence level for ranged rolls. The
// NPC roll factors (getRollFactor/getDefenceRollFactor) are
// identity — no per-NPC factors exist in Go — and the
// magic-defence-roll event is skipped; the combat:defence-roll
// hook (Hooks.ModifyDefenceRoll) and the scoped accuracy scale
// apply as in TS.
func SpecialRolls(attacker, target Combatant, style CombatType, defenceType *CombatType, rangedDefenceType string, projectedDefenceLevel *int) AccuracyRolls {
	traits := ActiveTraitsFor(attacker)
	attackStyle := style
	if traits != nil && traits.RollAttackType != nil {
		attackStyle = *traits.RollAttackType
	}
	defenceStyle := attackStyle
	if traits != nil && traits.DefenceRollAttackType != nil {
		defenceStyle = *traits.DefenceRollAttackType
	} else if defenceType != nil {
		defenceStyle = *defenceType
	}

	var attRoll int
	switch attackStyle {
	case CombatTypeMelee:
		attRoll = attackMeleeRoll(attacker)
	case CombatTypeRanged:
		// PluginManager.modifyRangedAttackRoll: skipped (no
		// hook field). See the file header.
		attRoll = attackRangedRoll(attacker)
	default:
		attRoll = attackMagicRoll(attacker)
	}

	var defRoll int
	switch defenceStyle {
	case CombatTypeMelee:
		meleeIndex := 3 // NPC attackers: the default case
		if attacker.IsPlayer() {
			meleeIndex = fightBonusTypeOf(attacker)
		}
		if traits != nil && traits.MeleeAttackBonusIndex != nil {
			meleeIndex = *traits.MeleeAttackBonusIndex
		}
		defenceIndex := meleeIndex
		if traits != nil && traits.MeleeDefenceBonusIndex != nil {
			defenceIndex = *traits.MeleeDefenceBonusIndex
		}
		defRoll = defenseMeleeRoll(target, defenceIndex)
	case CombatTypeRanged:
		kind := rangedDefenceType
		if kind == "" {
			kind = "standard"
		}
		defRoll = defenseRangedRoll(target, kind, projectedDefenceLevel)
	default:
		defRoll = defenseMagicRoll(target)
	}

	if traits != nil && traits.DefenceRollMultiplier != nil {
		defRoll = int(math.Floor(float64(defRoll) * *traits.DefenceRollMultiplier))
	}
	// NPC roll factors: identity (see SpecialRolls comment).
	// magicDefenceRollEvent: skipped (see file header).
	if Hooks.ModifyDefenceRoll != nil {
		if roll := Hooks.ModifyDefenceRoll(attacker.Mobile(), target.Mobile(), defRoll); roll != nil && *roll >= 0 {
			defRoll = *roll
		}
	}
	scale := scopedAccuracyScale
	if Hooks.AccuracyScale != nil {
		scale *= Hooks.AccuracyScale(attacker.Mobile(), target.Mobile())
	}
	attRoll = int(math.Floor(float64(attRoll) * scale))
	return AccuracyRolls{Attack: attRoll, Defence: defRoll}
}

// RollAccuracy is rollAccuracy: two inclusive rolls, accurate
// when the attack roll beats the defence roll. The
// combat:accuracy-rolled hook may override the outcome.
func RollAccuracy(attacker, target Combatant, style CombatType, defenceType *CombatType, rangedDefenceType string, projectedDefenceLevel *int) bool {
	rolls := SpecialRolls(attacker, target, style, defenceType, rangedDefenceType, projectedDefenceLevel)
	attackRolled := randomInclusive(rolls.Attack)
	defenceRolled := randomInclusive(rolls.Defence)
	accurate := attackRolled > defenceRolled
	if Hooks.ModifyAccuracyOutcome != nil {
		if outcome := Hooks.ModifyAccuracyOutcome(attacker.Mobile(), target.Mobile(), accurate); outcome != nil {
			accurate = *outcome
		}
	}
	return accurate
}

// RollAccuracyCount is rollAccuracyCount: the number of
// successful independent accuracy rolls out of count.
func RollAccuracyCount(attacker, target Combatant, style CombatType, count int, defenceType *CombatType, projectedDefenceLevel *int) int {
	successful := 0
	for i := 0; i < count; i++ {
		if RollAccuracy(attacker, target, style, defenceType, "", projectedDefenceLevel) {
			successful++
		}
	}
	return successful
}

// RollFixedAccuracy is rollFixedAccuracy: the attack roll is
// scaled by multiplier (a decimal), then compared
// deterministically to the defence roll; when it falls short,
// the hit lands with probability (attack+1)/(defence+1).
func RollFixedAccuracy(attacker, target Combatant, style CombatType, multiplier float64, defenceType *CombatType, projectedDefenceLevel *int) bool {
	rolls := SpecialRolls(attacker, target, style, defenceType, "", projectedDefenceLevel)
	attack := int(math.Floor(float64(rolls.Attack) * multiplier))
	if attack < 0 {
		attack = 0
	}
	defence := rolls.Defence
	if defence < 0 {
		defence = 0
	}
	if attack >= defence {
		return true
	}
	return randomFloat() < float64(attack+1)/float64(defence+1)
}

// HitChance is hitChance: the exact hit probability for a pair
// of rolls (the Wiki DPS formula).
func HitChance(attRoll, defRoll int) float64 {
	if attRoll > defRoll {
		return 1 - float64(defRoll+2)/float64(2*(attRoll+1))
	}
	return float64(attRoll) / float64(2*(defRoll+1))
}

// ---- Max hits ----

// applyDamageMultiplierTraits is applyDamageMultiplierTraits
// (DamageFormulas.ts): a special's damage traits scale a base
// max hit. With no traits the legacy per-weapon multiplier
// applies via scaleSpecial; stages apply in order (a "ceil"
// rounding entry rounds that stage up, anything else uses
// scaleSpecial rounding); otherwise the single damage
// multiplier (default 1) applies via scaleSpecial.
func applyDamageMultiplierTraits(maxHit int, traits *WeaponSpecialTraits, legacyMultiplier float64) int {
	if traits == nil {
		return scaleSpecial(maxHit, legacyMultiplier)
	}
	if len(traits.DamageMultiplierStages) > 0 {
		value := maxHit
		for i, mult := range traits.DamageMultiplierStages {
			rounding := ""
			if i < len(traits.DamageMultiplierStageRounding) {
				rounding = traits.DamageMultiplierStageRounding[i]
			}
			if rounding == "ceil" {
				value = int(math.Ceil(float64(value) * math.Round(mult*1000) / 1000))
			} else {
				value = scaleSpecial(value, mult)
			}
		}
		return value
	}
	multiplier := 1.0
	if traits.DamageMultiplier != nil {
		multiplier = *traits.DamageMultiplier
	}
	return scaleSpecial(maxHit, multiplier)
}

// npcCurrentMaxHit is NPC.getCurrentMaxHit: magic always reads
// the definition max hit; melee/ranged at full level read it
// too, and below full level read
// min(maximum, floor(0.5 + (current+9)*(bonus+64)/640)) with
// the melee/ranged strength bonus (stats[6]/[8]). Go has no
// stat drains, so NPCs always read the definition value; the
// drained branch is kept for fidelity.
func npcCurrentMaxHit(attacker Combatant, t CombatType) int {
	d := attacker.NPCDef()
	maximum := 1 // DEFAULT definition maxHit
	if d != nil {
		maximum = d.MaxHit()
	}
	if t == CombatTypeMagic {
		return maximum
	}
	skill, statSlot, bonusSlot := SkillStrength, 1, 6
	if t == CombatTypeRanged {
		skill, statSlot, bonusSlot = SkillRanged, 3, 8
	}
	current := attacker.CurrentLevel(skill)
	if current == npcStat(d, statSlot) {
		return maximum
	}
	bonus := npcStat(d, bonusSlot)
	drained := int(math.Floor(0.5 + float64(current+9)*float64(bonus+64)/640.0))
	if drained < maximum {
		return drained
	}
	return maximum
}

// CalculateMaxMeleeHit is calculateMaxMeleeHit
// (DamageFormulas.ts): floor((effStr*(strBonus+64)+320)/640)
// for players, with the special-override path when traits are
// active or useSpecial names an activated melee special;
// NPCs read getCurrentMaxHit(MELEE). useSpecial is the
// resolved includeSpecial ?? isSpecialActivated value.
func CalculateMaxMeleeHit(attacker Combatant, useSpecial bool) int {
	if !attacker.IsPlayer() {
		return npcCurrentMaxHit(attacker, CombatTypeMelee)
	}
	traits := ActiveTraitsFor(attacker)
	strengthBonus := attacker.OtherBonus(OtherBonusStrength)
	var visibleMult *float64
	if traits != nil {
		visibleMult = traits.StrengthLevelMultiplier
	}
	maxHit := scaleRatio(effectiveStrengthLevel(attacker, visibleMult)*(strengthBonus+64)+320, 1, 640)
	combat := attacker.Mobile().GetCombat()
	special := combat.GetCombatSpecial()
	specialApplies := traits != nil || (useSpecial && special != nil && specialMethodMatches(special, CombatTypeMelee))
	if specialApplies {
		if traits != nil && traits.MaxHitOverride != nil {
			maxHit = int(math.Floor(float64(*traits.MaxHitOverride)))
		} else {
			legacy := 1.0
			if special != nil && special.StrengthMultiplier != nil {
				legacy = *special.StrengthMultiplier
			}
			maxHit = applyDamageMultiplierTraits(maxHit, traits, legacy)
		}
	}
	// applyMeleeHitModifiers: identity. See file header.
	return int(math.Floor(float64(maxHit)))
}

// CalculateMaxRangedHit is calculateMaxRangedHit: NPCs read
// getCurrentMaxHit(RANGED); players use the ranged-strength
// formula (Strength skill + melee strength bonus for
// uses-melee-strength weapons), with the same special path as
// melee.
func CalculateMaxRangedHit(attacker Combatant, useSpecial bool) int {
	if !attacker.IsPlayer() {
		return npcCurrentMaxHit(attacker, CombatTypeRanged)
	}
	traits := ActiveTraitsFor(attacker)
	player := attacker.Player()
	meleeStrength := rangedUsesMeleeStrength(player)
	damageSkill := SkillRanged
	strengthType := OtherBonusRangedStrength
	if meleeStrength {
		damageSkill = SkillStrength
		strengthType = OtherBonusStrength
	}
	strengthBonus := attacker.OtherBonus(strengthType)
	var visibleMult *float64
	if traits != nil {
		visibleMult = traits.StrengthLevelMultiplier
	}
	maxHit := scaleRatio(effectiveRangedStrength(attacker, visibleMult, damageSkill, true)*(strengthBonus+64)+320, 1, 640)
	combat := attacker.Mobile().GetCombat()
	special := combat.GetCombatSpecial()
	specialApplies := traits != nil || (useSpecial && special != nil && specialMethodMatches(special, CombatTypeRanged))
	if specialApplies {
		if traits != nil && traits.MaxHitOverride != nil {
			maxHit = int(math.Floor(float64(*traits.MaxHitOverride)))
		} else {
			legacy := 1.0
			if special != nil && special.StrengthMultiplier != nil {
				legacy = *special.StrengthMultiplier
			}
			maxHit = applyDamageMultiplierTraits(maxHit, traits, legacy)
		}
	}
	// applyRangedHitModifiers: identity. See file header.
	return int(math.Floor(float64(maxHit)))
}

// visibleMagicBaseMaxHit is visibleMagicBaseMaxHit: the
// visible-magic special base,
// min(configured, floor(configured*visibleMagic/99 + 1)).
func visibleMagicBaseMaxHit(attacker Combatant, baseMaxHit int) int {
	base := baseMaxHit
	if base < 0 {
		base = 0
	}
	if base <= 0 {
		return 0
	}
	visibleMagic := attacker.CurrentLevel(SkillMagic)
	scaled := int(math.Floor(float64(base*visibleMagic)/99 + 1))
	if scaled < base {
		return scaled
	}
	return base
}

// CalculateMaxMagicHit is calculateMaxMagicHit: the trait
// override paths (visible-magic base, max-hit override) for
// players, else getMagicMaxhit. The volatile nightmare staff
// branch is NOT ported (weapon-specific content: it needs the
// item id and the special method check; see the file header).
func CalculateMaxMagicHit(attacker Combatant, spell CombatSpell, useSpecial bool) int {
	_ = useSpecial // only the volatile-staff branch reads it; skipped
	if attacker.IsPlayer() {
		traits := ActiveTraitsFor(attacker)
		if traits != nil {
			if traits.MaximumHitSource != nil && *traits.MaximumHitSource == SpecialMaxHitVisibleMagic {
				visible := 0
				if traits.VisibleMagicMaximumHit != nil {
					visible = *traits.VisibleMagicMaximumHit
				}
				return ApplyMagicDamageBonus(attacker, visibleMagicBaseMaxHit(attacker, visible))
			}
			if traits.MaxHitOverride != nil {
				return int(math.Floor(float64(*traits.MaxHitOverride)))
			}
		}
	}
	return GetMagicMaxHit(attacker, spell, nil)
}

// GetMagicMaxHit is getMagicMaxhit: the maximum of the magic
// hit bounds.
func GetMagicMaxHit(attacker Combatant, spell CombatSpell, target Combatant) int {
	_, maximum := GetMagicHitBounds(attacker, spell, target)
	return maximum
}

// GetMagicHitBounds is getMagicHitBounds: the spell's declared
// maximum (0 stays harmless), else the NPC definition max hit,
// else 1 for a spell-less player; then the magic damage bonus,
// the combat:magic-max-hit hook, and the
// combat:magic-hit-bounds hook (minHit, default 0). The
// combat:magic-base-max-hit event, charge-spell max hits, and
// demonbane are skipped (see the file header).
func GetMagicHitBounds(attacker Combatant, spellOverride CombatSpell, targetOverride Combatant) (minimum, maximum int) {
	spell := spellOverride
	if spell == nil && attacker.Mobile() != nil {
		spell = attacker.Mobile().GetCombat().SelectedSpell()
	}
	maxHit := 0
	if spell != nil {
		declared := spell.MaximumHit()
		if declared == 0 {
			return 0, 0
		}
		if declared > 0 {
			maxHit = declared
		} else if !attacker.IsPlayer() {
			maxHit = npcCurrentMaxHit(attacker, CombatTypeMagic)
		} else {
			maxHit = 1
		}
	} else if !attacker.IsPlayer() {
		maxHit = npcCurrentMaxHit(attacker, CombatTypeMagic)
	} else {
		maxHit = 1
	}
	// combat:magic-base-max-hit, applyChargeMaxHit: skipped.
	maxHit = ApplyMagicDamageBonus(attacker, maxHit)
	// demonbane, applyMagicHitModifiers: skipped / identity.
	var target *Mobile
	if targetOverride != nil {
		target = targetOverride.Mobile()
	} else if attacker.Mobile() != nil {
		target = attacker.Mobile().GetCombat().Target()
	}
	if Hooks.ModifyMagicMaxHit != nil {
		if modified := Hooks.ModifyMagicMaxHit(attacker.Mobile(), target, maxHit); modified != nil && *modified >= 0 {
			maxHit = *modified
		}
	}
	minHit := 0
	if Hooks.ModifyMagicHitBounds != nil {
		if newMin, newMax, changed := Hooks.ModifyMagicHitBounds(attacker.Mobile(), target, 0, maxHit); changed {
			if newMax >= 0 {
				maxHit = newMax
			}
			if newMin >= 0 {
				minHit = newMin
				if minHit > maxHit {
					minHit = maxHit
				}
			}
		}
	}
	return minHit, maxHit
}

// ApplyMagicDamageBonus is applyMagicDamageBonus: players only;
// floor(maxHit * (1000 + permille) / 1000) where permille is
// round(magic-strength bonus * 10) + the prayer permille. The
// equipment-effect modifier is identity.
func ApplyMagicDamageBonus(attacker Combatant, maxHit int) int {
	if !attacker.IsPlayer() {
		return maxHit
	}
	equipmentPermille := int(math.Round(float64(attacker.OtherBonus(OtherBonusMagicStrength)) * 10))
	bonusPermille := equipmentPermille + magicDamagePrayerPermille(attacker.Mobile())
	return scaleRatio(maxHit, 1000+bonusPermille, 1000)
}

// SourceMaxHit is sourceMaxHit (DamageFormulas.ts): a special's
// maximumHitSource override wins (physical_melee / ranged /
// magic / visible_magic route to the matching calculator);
// otherwise the attack type decides (ranged, magic via
// getMagicMaxhit, else melee).
func SourceMaxHit(attacker Combatant, attackType CombatType) int {
	traits := ActiveTraitsFor(attacker)
	useSpecial := false
	if attacker.Mobile() != nil {
		useSpecial = attacker.Mobile().GetCombat().IsSpecialActivated()
	}
	if traits != nil && traits.MaximumHitSource != nil {
		switch *traits.MaximumHitSource {
		case SpecialMaxHitPhysicalMelee:
			return CalculateMaxMeleeHit(attacker, useSpecial)
		case SpecialMaxHitRanged:
			return CalculateMaxRangedHit(attacker, useSpecial)
		case SpecialMaxHitMagic, SpecialMaxHitVisibleMagic:
			return CalculateMaxMagicHit(attacker, nil, useSpecial)
		}
	}
	switch attackType {
	case CombatTypeRanged:
		return CalculateMaxRangedHit(attacker, useSpecial)
	case CombatTypeMagic:
		return GetMagicMaxHit(attacker, nil, nil)
	default:
		return CalculateMaxMeleeHit(attacker, useSpecial)
	}
}

// SourceHitBounds is sourceHitBounds: magic attacks (without a
// maximumHitSource override) resolve through the magic bounds,
// with a magic special's damage traits scaling the maximum;
// other attacks read {0, sourceMaxHit}.
func SourceHitBounds(attacker Combatant, attackType CombatType, target Combatant) (minimum, maximum int) {
	traits := ActiveTraitsFor(attacker)
	if attackType == CombatTypeMagic && (traits == nil || traits.MaximumHitSource == nil) {
		minimum, maximum := GetMagicHitBounds(attacker, nil, target)
		if traits == nil || (traits.DamageMultiplier == nil && len(traits.DamageMultiplierStages) == 0) {
			return minimum, maximum
		}
		maximum = applyDamageMultiplierTraits(maximum, traits, 1)
		if minimum > maximum {
			minimum = maximum
		}
		return minimum, maximum
	}
	return 0, SourceMaxHit(attacker, attackType)
}

// SpecialDamageRange is specialDamageRange
// (CombatFactory.ts:478-512): the damage range for one hit,
// from the max hit, an optional bounds override, the active
// special traits, and a minimum floor. With neither traits
// nor an override the range is {min(maxHit, max(0, floor)),
// maxHit}. Otherwise the multipliers (fractions of the max
// hit, override wins over traits, defaults 0 / 1) and bonuses
// shape the ends, the cap and reduction clamp them, an
// inverted range swaps, a maximum split takes this hit's
// share, and the floor clamps the minimum last.
func SpecialDamageRange(attacker Combatant, maxHit int, boundsOverride *SpecialDamageBounds, applyTraits bool, minimumFloor int) (minimum, maximum int) {
	var traits *WeaponSpecialTraits
	if applyTraits {
		traits = ActiveTraitsFor(attacker)
	}
	if traits == nil && boundsOverride == nil {
		floorVal := minimumFloor
		if floorVal < 0 {
			floorVal = 0
		}
		minimum = maxHit
		if floorVal < minimum {
			minimum = floorVal
		}
		return minimum, maxHit
	}
	minMult := 0.0
	if boundsOverride != nil && boundsOverride.MinimumMultiplier != nil {
		minMult = *boundsOverride.MinimumMultiplier
	} else if traits != nil && traits.MinimumDamageMultiplier != nil {
		minMult = *traits.MinimumDamageMultiplier
	}
	maxMult := 1.0
	if boundsOverride != nil && boundsOverride.MaximumMultiplier != nil {
		maxMult = *boundsOverride.MaximumMultiplier
	} else if traits != nil && traits.MaximumDamageMultiplier != nil {
		maxMult = *traits.MaximumDamageMultiplier
	}
	minimum = int(math.Floor(float64(maxHit) * minMult))
	maximum = int(math.Floor(float64(maxHit) * maxMult))
	minBonus := 0
	if boundsOverride != nil && boundsOverride.MinimumBonus != nil {
		minBonus = *boundsOverride.MinimumBonus
	} else if traits != nil && traits.MinimumDamageBonus != nil {
		minBonus = *traits.MinimumDamageBonus
	}
	maxBonus := 0
	if boundsOverride != nil && boundsOverride.MaximumBonus != nil {
		maxBonus = *boundsOverride.MaximumBonus
	} else if traits != nil && traits.MaximumDamageBonus != nil {
		maxBonus = *traits.MaximumDamageBonus
	}
	minimum += minBonus
	maximum += maxBonus
	var cap *int
	if boundsOverride != nil {
		cap = boundsOverride.Cap
	}
	if cap == nil && traits != nil {
		cap = traits.MaximumDamageCap
	}
	if cap != nil {
		if maximum > *cap {
			maximum = *cap
		}
		if minimum > maximum {
			minimum = maximum
		}
	}
	reduction := 0
	if boundsOverride != nil {
		reduction = boundsOverride.Reduction
	}
	if reduction > 0 {
		maximum -= reduction
		if maximum < 0 {
			maximum = 0
		}
		if minimum > maximum {
			minimum = maximum
		}
	}
	if maximum < minimum {
		minimum, maximum = maximum, minimum
	}
	if boundsOverride != nil && boundsOverride.MaximumSplit != nil {
		count := boundsOverride.MaximumSplit.Count
		if count < 1 {
			count = 1
		}
		index := boundsOverride.MaximumSplit.Index
		if index < 0 {
			index = 0
		}
		if index > count-1 {
			index = count - 1
		}
		maximum = maximum*(index+1)/count - maximum*index/count
		if minimum > maximum {
			minimum = maximum
		}
	}
	floorVal := minimumFloor
	if floorVal < 0 {
		floorVal = 0
	}
	floorClamp := floorVal
	if maximum < floorClamp {
		floorClamp = maximum
	}
	if minimum < floorClamp {
		minimum = floorClamp
	}
	if minimum < 0 {
		minimum = 0
	}
	if maximum < 0 {
		maximum = 0
	}
	return minimum, maximum
}
