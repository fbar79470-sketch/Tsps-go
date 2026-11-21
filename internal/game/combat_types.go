package game

// CombatType is CombatType (game/content/combat/CombatType.ts): the
// kind of an attack. The numeric order (MELEE, RANGED, MAGIC) matches
// the TS enum declaration order.
type CombatType int

const (
	CombatTypeMelee CombatType = iota
	CombatTypeRanged
	CombatTypeMagic
)

// String returns the TS enum name.
func (t CombatType) String() string {
	switch t {
	case CombatTypeMelee:
		return "MELEE"
	case CombatTypeRanged:
		return "RANGED"
	case CombatTypeMagic:
		return "MAGIC"
	default:
		return "UNKNOWN"
	}
}

// Attack/defence bonus indices (FightType.ts: ATTACK_STAB..ATTACK_RANGE
// and DEFENCE_STAB..DEFENCE_RANGE; BonusManager.ts: ATTACK_STAB=0 ..
// ATTACK_RANGE=4, DEFENCE_STAB=0 .. DEFENCE_RANGE=4). Attack and
// defence share the index space: a stab attack rolls against stab
// defence at the same index.
const (
	BonusAttackStab = iota
	BonusAttackSlash
	BonusAttackCrush
	BonusAttackMagic
	BonusAttackRange
)

// Other-bonus indices (BonusManager.ts: STRENGTH=0, RANGED_STRENGTH=1,
// MAGIC_STRENGTH=2, PRAYER=3): the "other" bonus array, distinct from
// the attack/defence arrays.
const (
	OtherBonusStrength = iota
	OtherBonusRangedStrength
	OtherBonusMagicStrength
	OtherBonusPrayer
)

// FightStyle is FightStyle (game/content/combat/FightStyle.ts): the
// stance of a fight type. It decides which skills an attack trains
// and the invisible style bonuses in the accuracy/damage formulas.
type FightStyle int

const (
	FightStyleAccurate FightStyle = iota
	FightStyleAggressive
	FightStyleDefensive
	FightStyleControlled
)

// String returns the TS style name.
func (s FightStyle) String() string {
	switch s {
	case FightStyleAccurate:
		return "ACCURATE"
	case FightStyleAggressive:
		return "AGGRESSIVE"
	case FightStyleDefensive:
		return "DEFENSIVE"
	case FightStyleControlled:
		return "CONTROLLED"
	default:
		return "UNKNOWN"
	}
}

// Skills returns the skill indices this style trains for an attack of
// the given type (FightStyle.ts skill()): Accurate trains Attack (or
// Ranged), Aggressive trains Strength (or Ranged), Defensive trains
// Defence alone for melee but splits Ranged/Magic with Defence, and
// Controlled trains Attack, Strength and Defence together. The
// returned slice is freshly allocated.
func (s FightStyle) Skills(t CombatType) []int {
	switch s {
	case FightStyleAccurate:
		if t == CombatTypeRanged {
			return []int{SkillRanged}
		}
		return []int{SkillAttack}
	case FightStyleAggressive:
		if t == CombatTypeRanged {
			return []int{SkillRanged}
		}
		return []int{SkillStrength}
	case FightStyleDefensive:
		switch t {
		case CombatTypeRanged:
			return []int{SkillRanged, SkillDefence}
		case CombatTypeMagic:
			return []int{SkillMagic, SkillDefence}
		default:
			return []int{SkillDefence}
		}
	case FightStyleControlled:
		return []int{SkillAttack, SkillStrength, SkillDefence}
	default:
		return []int{SkillAttack}
	}
}

// FightType is one row of the fight-type registry (FightType.ts): the
// style slot a weapon offers. The registry is data-driven
// (item-combat-styles.json, loaded by CombatStyleDefinitions.ts);
// see combat_styles.go for the loader and the installed registry.
// The player's current fight type lives on Combat (see
// Combat.FightType/SetFightType).
type FightType struct {
	// Name is the registry key (e.g. "UNARMED_PUNCH").
	Name string
	// Animation is the attack animation id.
	Animation int
	// NPCAnimation is the separate animation against NPCs, or -1.
	NPCAnimation int
	// ChildID is the style slot's child config id (varp 43 children).
	ChildID int
	// ParentID is the style slot's parent config id: always
	// COMBAT_STYLE_PARENT_ID (43) in TS (FightType.ts).
	ParentID int
	// BonusType is the attack-bonus index the style rolls with
	// (BonusAttackStab..BonusAttackRange).
	BonusType int
	// Style is the stance.
	Style FightStyle
	// Sound is the attack sound (FightType.ts getAttackSound):
	// the JSON sound name resolved against the known sounds,
	// falling back to WEAPON for an unknown name, as TS does
	// ((Sound as any)[definition.sound] ?? Sound.WEAPON).
	Sound Sound
	// Rapid shortens the attack speed by one tick (Player.ts
	// getBaseAttackSpeed).
	Rapid bool
}

// AnimationAgainst returns the animation for an attack on an NPC
// (npcAnimation where the style has one) or a player
// (FightType.ts getAnimationAgainst).
func (f *FightType) AnimationAgainst(npc bool) int {
	if f == nil {
		return -1
	}
	if npc && f.NPCAnimation > 0 {
		return f.NPCAnimation
	}
	return f.Animation
}

// CorrespondingBonus returns the defence bonus index this style's
// attack rolls against (FightType.ts getCorrespondingBonus): the
// defence index matching the attack bonus type, defaulting to
// crush defence. Attack and defence share the index space, so
// this is the identity except for an out-of-range bonus type.
func (f *FightType) CorrespondingBonus() int {
	if f == nil {
		return BonusAttackCrush
	}
	switch f.BonusType {
	case BonusAttackStab, BonusAttackSlash, BonusAttackCrush, BonusAttackMagic, BonusAttackRange:
		return f.BonusType
	default:
		return BonusAttackCrush
	}
}

// UnarmedPunch is FightType.UNARMED_PUNCH, the default fight type
// (Player.ts:230, `private fightType = FightType.UNARMED_PUNCH`):
// Punch, animation 422, child 0, CRUSH, ACCURATE
// (data/definitions/item-combat-styles.json, UNARMED_PUNCH row).
// Every player starts on it until a weapon is equipped; it is
// also the registry fallback's first style (see
// UnarmedFallbackRegistry in combat_styles.go).
var UnarmedPunch = &FightType{
	Name:         "UNARMED_PUNCH",
	Animation:    422,
	NPCAnimation: -1,
	ChildID:      0,
	ParentID:     CombatStyleVarp,
	BonusType:    BonusAttackCrush,
	Style:        FightStyleAccurate,
	Sound:        SoundWeaponUnarmedPunch,
}
