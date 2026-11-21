package definition

import (
	"fmt"
	"math"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"tsps-go/internal/cache"
)

// CombatType is CombatType (content/combat/CombatType.ts): the
// default combat style an NPC fights with.
type CombatType int

const (
	// CombatTypeMelee is CombatType.MELEE.
	CombatTypeMelee CombatType = iota
	// CombatTypeRanged is CombatType.RANGED.
	CombatTypeRanged
	// CombatTypeMagic is CombatType.MAGIC.
	CombatTypeMagic
)

// Ranged defence bonus kinds for RangedDefenceBonus
// (NpcDefinition.getRangedDefenceBonus).
const (
	RangedDefenceStandard = "standard"
	RangedDefenceLight    = "light"
	RangedDefenceHeavy    = "heavy"
)

// ElementalWeakness is the {type, percent} pair
// getElementalWeakness returns.
type ElementalWeakness struct {
	Type    int
	Percent int
}

// npcDefaultStats is DEFAULT_STATS (NpcDefinition.ts:25-31):
// slots 0-4 combat levels, 5/7/9 accuracy bonuses, 10-14 defence
// bonuses; slots 6, 8, 15-17 unused.
var npcDefaultStats = [18]int{1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// NpcDefinition is the JSON/gameplay layer over a cache NPC type
// (NpcDefinition.ts). Instances are built by Store.NPC, which
// hydrates from the cache and applies the loaded overlays; each
// call returns a fresh value. The TS DEFAULT shape (forId for an
// out-of-range id, or with no cache here) has ID -1, name "null",
// and the field defaults below.
type NpcDefinition struct {
	id                   int
	name                 string
	examine              string
	wikiExamine          string
	size                 int
	walkRadius           int
	attackable           bool
	retreats             bool
	aggressive           bool
	aggressiveTolerance  bool
	poisonous            bool
	venomous             bool
	immunePoison         bool
	immuneVenom          bool
	freezeResistance     int
	demon                bool
	attributes           []string
	members              bool
	attackType           CombatType
	projectileID         int
	launchGraphicID      int
	fightsBack           bool
	respawn              int
	maxHit               int
	hitpoints            int
	attackSpeed          int
	attackAnim           int
	defenceAnim          int
	deathAnim            int
	spawnAnim            *int
	deathSound           int
	deathTicks           int
	combatLevel          int
	stats                [18]int
	rangedLight          *int
	rangedHeavy          *int
	weaknessType         int
	weaknessPercent      int
	slayerLevel          int
	attackStyle          string
	combatFollowDistance int
	huntRange            int
	actions              []string
}

func newDefaultNpcDefinition() *NpcDefinition {
	return &NpcDefinition{
		id:                   -1,
		name:                 "null",
		size:                 1,
		aggressiveTolerance:  true,
		attackType:           CombatTypeMelee,
		projectileID:         -1,
		launchGraphicID:      -1,
		fightsBack:           true,
		respawn:              25,
		maxHit:               1,
		hitpoints:            10,
		attackSpeed:          4,
		attackAnim:           422,
		defenceAnim:          -1,
		deathAnim:            836,
		deathSound:           -1,
		deathTicks:           2,
		stats:                npcDefaultStats,
		combatFollowDistance: 7,
		huntRange:            3,
		attributes:           []string{},
		actions:              []string{},
	}
}

// hydrateFromCache fills the definition from a cache NPC type
// (NpcDefinition.hydrateFromCache, NpcDefinition.ts:102-129).
func (d *NpcDefinition) hydrateFromCache(id int, cached *cache.NpcType, store *Store) {
	d.id = id
	d.name = cached.Name
	if cached.Desc != nil {
		d.examine = *cached.Desc
	}
	d.size = cached.Size
	d.attackable = false
	for _, action := range cached.Actions {
		if strings.ToLower(action) == "attack" {
			d.attackable = true
			break
		}
	}
	d.actions = append([]string(nil), cached.Actions...)
	// Only human rigs inherit the legacy human block; other
	// creatures need a resolved role.
	if seqName := store.seqName(cached.IdleSeqId); seqName != "" {
		for _, word := range strings.Split(seqName, "_") {
			if word == "human" {
				d.defenceAnim = store.seqID("human_unarmedblock")
				break
			}
		}
	}
	if cached.CombatLevel >= 0 {
		d.combatLevel = cached.CombatLevel
	}
	if cached.Hitpoints > 0 {
		d.hitpoints = cached.Hitpoints
	}
	levels := [][2]int{
		{0, cached.AttackLevel},
		{1, cached.StrengthLevel},
		{2, cached.DefenceLevel},
		{3, cached.RangedLevel},
		{4, cached.MagicLevel},
	}
	for _, pair := range levels {
		if pair[1] >= 0 {
			d.stats[pair[0]] = pair[1]
		}
	}
}

// NPC returns the NPC definition for an id, hydrated from the
// cache with the loaded JSON overlays applied (NpcDefinition.
// forId, NpcDefinition.ts:95-101). Out-of-range ids, and every id
// with no cache, return the DEFAULT shape. Custom-registered NPCs
// (the definitions map's out-of-range entries) are not ported —
// see the package comment.
func (s *Store) NPC(id int) *NpcDefinition {
	def := newDefaultNpcDefinition()
	if s.cache != nil && id >= 0 && id < s.npcCount() {
		def.hydrateFromCache(id, s.cache.GetNpc(id), s)
	}
	def.wikiExamine = s.npcExamines[id]
	if overlay, ok := s.npcOverlays[id]; ok {
		overlay.apply(def)
	}
	return def
}

// ID returns the NPC id (-1 for the DEFAULT shape).
func (d *NpcDefinition) ID() int { return d.id }

// Name returns the NPC name.
func (d *NpcDefinition) Name() string { return d.name }

// Examine returns the examine text: the definition's own examine
// (cache desc, or the monster dump's), else the Wiki examine from
// npc-examines.json, else a generic line (NpcDefinition.
// getExamine, NpcDefinition.ts:152-160). TypeScript reads the
// examines file lazily on first use; this package reads it eagerly
// at Load — the same text, without a throw when the file is
// missing.
func (d *NpcDefinition) Examine() string {
	if d.examine != "" {
		return d.examine
	}
	if d.wikiExamine != "" {
		return d.wikiExamine
	}
	if d.name == "" || d.name == "null" {
		return "It's nothing special."
	}
	return "It's " + anOrA(d.name) + " " + d.name + "."
}

// Size returns the NPC footprint size in tiles.
func (d *NpcDefinition) Size() int { return d.size }

// WalkRadius returns the walk radius.
func (d *NpcDefinition) WalkRadius() int { return d.walkRadius }

// IsAttackable reports whether the NPC can be attacked: the cache
// Attack action, or a plugin-added combat option
// (NpcDefinition.ts:172-174). addCombatOption is not ported — no
// plugin system — so only "attack" counts; see the package
// comment.
func (d *NpcDefinition) IsAttackable() bool {
	if d.attackable {
		return true
	}
	for _, action := range d.actions {
		if strings.ToLower(action) == "attack" {
			return true
		}
	}
	return false
}

// Actions returns the NPC's menu actions.
func (d *NpcDefinition) Actions() []string { return append([]string(nil), d.actions...) }

// DoesRetreat reports whether the NPC retreats.
func (d *NpcDefinition) DoesRetreat() bool { return d.retreats }

// IsAggressive reports whether the NPC is aggressive.
func (d *NpcDefinition) IsAggressive() bool { return d.aggressive }

// BuildsAggressionTolerance reports whether the NPC builds
// aggression tolerance.
func (d *NpcDefinition) BuildsAggressionTolerance() bool { return d.aggressiveTolerance }

// IsPoisonous reports whether the NPC is poisonous.
func (d *NpcDefinition) IsPoisonous() bool { return d.poisonous }

// DoesFightBack reports whether the NPC fights back.
func (d *NpcDefinition) DoesFightBack() bool { return d.fightsBack }

// Respawn returns the ticks from removal to respawn (0 disables).
func (d *NpcDefinition) Respawn() int { return d.respawn }

// MaxHit returns the maximum hit.
func (d *NpcDefinition) MaxHit() int { return d.maxHit }

// Hitpoints returns the hitpoints.
func (d *NpcDefinition) Hitpoints() int { return d.hitpoints }

// AttackSpeed returns the attack speed in ticks.
func (d *NpcDefinition) AttackSpeed() int { return d.attackSpeed }

// AttackAnim returns the attack animation.
func (d *NpcDefinition) AttackAnim() int { return d.attackAnim }

// DefenceAnim returns the defence (block) animation, or -1.
func (d *NpcDefinition) DefenceAnim() int { return d.defenceAnim }

// DeathAnim returns the death animation.
func (d *NpcDefinition) DeathAnim() int { return d.deathAnim }

// SpawnAnim returns the spawn animation, or false when the NPC
// has none (TS null).
func (d *NpcDefinition) SpawnAnim() (int, bool) {
	if d.spawnAnim == nil {
		return 0, false
	}
	return *d.spawnAnim, true
}

// DeathSound returns the death sound, or -1.
func (d *NpcDefinition) DeathSound() int { return d.deathSound }

// DeathTicks returns the ticks from the death animation to the
// death itself.
func (d *NpcDefinition) DeathTicks() int { return d.deathTicks }

// CombatLevel returns the combat level.
func (d *NpcDefinition) CombatLevel() int { return d.combatLevel }

// Stats returns the 18 combat stats (see npcDefaultStats for the
// slot layout).
func (d *NpcDefinition) Stats() []int {
	out := make([]int, 18)
	copy(out, d.stats[:])
	return out
}

// IsVenomous reports whether the NPC is venomous.
func (d *NpcDefinition) IsVenomous() bool { return d.venomous }

// IsPoisonImmune reports whether the NPC is immune to poison.
func (d *NpcDefinition) IsPoisonImmune() bool { return d.immunePoison }

// IsVenomImmune reports whether the NPC is immune to venom.
func (d *NpcDefinition) IsVenomImmune() bool { return d.immuneVenom }

// FreezeResistance returns the percentage a freeze is shortened
// by (100 is never frozen).
func (d *NpcDefinition) FreezeResistance() int { return d.freezeResistance }

// IsDemon reports whether the NPC is a demon.
func (d *NpcDefinition) IsDemon() bool { return d.demon }

// IsUndead reports whether the NPC has the undead attribute.
func (d *NpcDefinition) IsUndead() bool { return d.HasAttribute("undead") }

// Attributes returns the Wiki monster attributes.
func (d *NpcDefinition) Attributes() []string { return append([]string(nil), d.attributes...) }

// HasAttribute reports whether the NPC has an attribute.
func (d *NpcDefinition) HasAttribute(attribute string) bool {
	for _, a := range d.attributes {
		if a == attribute {
			return true
		}
	}
	return false
}

// IsMembers reports the Wiki members flag.
func (d *NpcDefinition) IsMembers() bool { return d.members }

// AttackStyle returns the dump's first attack style ("stab",
// "crush", "ranged", ...); "" when unknown.
func (d *NpcDefinition) AttackStyle() string { return d.attackStyle }

// AttackType returns the default combat style.
func (d *NpcDefinition) AttackType() CombatType { return d.attackType }

// ProjectileID returns the projectile graphic id, or -1.
func (d *NpcDefinition) ProjectileID() int { return d.projectileID }

// LaunchGraphicID returns the launch spotanim id, or -1.
func (d *NpcDefinition) LaunchGraphicID() int { return d.launchGraphicID }

// SlayerLevel returns the slayer level required, or 0.
func (d *NpcDefinition) SlayerLevel() int { return d.slayerLevel }

// CombatFollowDistance returns the retreat/leash max range.
func (d *NpcDefinition) CombatFollowDistance() int { return d.combatFollowDistance }

// AggressionRange returns the Wiki's aggression range: the NPC's
// max range plus the combat method's own reach, measured from its
// static spawn (NpcDefinition.ts:326-332). A non-finite attack
// distance counts as 1.
func (d *NpcDefinition) AggressionRange(attackDistance float64) int {
	reach := 1
	if !math.IsNaN(attackDistance) && !math.IsInf(attackDistance, 0) {
		reach = jsTrunc(attackDistance)
		if reach < 1 {
			reach = 1
		}
	}
	follow := d.combatFollowDistance
	if follow < 0 {
		follow = 0
	}
	return follow + reach
}

// HuntRange returns the tiles an aggressive NPC searches for a
// target from its current south-west tile.
func (d *NpcDefinition) HuntRange() int { return d.huntRange }

// ElementalWeakness returns the elemental weakness, or false when
// the NPC has none (type outside 1-4, or a zero percent).
func (d *NpcDefinition) ElementalWeakness() (ElementalWeakness, bool) {
	if d.weaknessType >= 1 && d.weaknessType <= 4 && d.weaknessPercent > 0 {
		return ElementalWeakness{Type: d.weaknessType, Percent: d.weaknessPercent}, true
	}
	return ElementalWeakness{}, false
}

// RangedDefenceBonus returns the ranged defence bonus for a kind
// ("light", "standard", "heavy"): the kind-specific bonus when the
// dump has one, else the standard ranged defence (stats slot 14).
// Any other kind behaves as "standard", as the TS index does.
func (d *NpcDefinition) RangedDefenceBonus(kind string) int {
	switch kind {
	case RangedDefenceLight:
		if d.rangedLight != nil {
			return *d.rangedLight
		}
	case RangedDefenceHeavy:
		if d.rangedHeavy != nil {
			return *d.rangedHeavy
		}
	}
	return d.stats[14]
}

// anOrA is Misc.anOrA (Misc.ts:458-482).
func anOrA(name string) string {
	s := strings.ToLower(name)
	switch s {
	case "anchovies", "soft clay", "cheese", "ball of wool", "spice", "steel nails", "snape grass", "coal":
		return "some"
	}
	if strings.HasPrefix(s, "a") || strings.HasPrefix(s, "e") || strings.HasPrefix(s, "i") || strings.HasPrefix(s, "o") || strings.HasPrefix(s, "u") {
		return "an"
	}
	return "a"
}

// npcOverlay is the accumulated NpcDefinitionLoader application
// for one NPC id. Pointer fields are nil when no stage set them;
// the dump fields (hasStats gate) are set together when the
// name-guarded monster dump applies.
type npcOverlay struct {
	hasStats         bool
	stats            [18]int
	rangedLight      *int
	rangedHeavy      *int
	weaknessType     int
	weaknessPercent  int
	examine          *string
	hitpoints        *int
	attackSpeed      *int
	maxHit           *int
	aggressive       bool
	poisonous        bool
	venomous         bool
	demon            bool
	members          bool
	attributes       []string
	hasAttributes    bool
	attackType       CombatType
	attackStyle      *string
	slayerLevel      *int
	immunePoison     *bool
	immuneVenom      *bool
	freezeResistance *int
	projectileID     *int
	launchGraphicID  *int
	respawn          *int
	huntRange        *int
	attackAnim       *int
	defenceAnim      *int
	deathAnim        *int
	spawnAnim        *int
	deathSound       *int
	deathTicks       *int
}

func (o *npcOverlay) apply(d *NpcDefinition) {
	if o.hasStats {
		d.stats = o.stats
		d.rangedLight = o.rangedLight
		d.rangedHeavy = o.rangedHeavy
		d.weaknessType = o.weaknessType
		d.weaknessPercent = o.weaknessPercent
		d.aggressive = o.aggressive
		d.poisonous = o.poisonous
		d.venomous = o.venomous
		d.demon = o.demon
		d.members = o.members
		d.attackType = o.attackType
	}
	if o.examine != nil {
		d.examine = *o.examine
	}
	if o.hitpoints != nil {
		d.hitpoints = *o.hitpoints
	}
	if o.attackSpeed != nil {
		d.attackSpeed = *o.attackSpeed
	}
	if o.maxHit != nil {
		d.maxHit = *o.maxHit
	}
	if o.hasAttributes {
		d.attributes = append([]string(nil), o.attributes...)
	}
	if o.attackStyle != nil {
		d.attackStyle = *o.attackStyle
	}
	if o.slayerLevel != nil {
		d.slayerLevel = *o.slayerLevel
	}
	if o.immunePoison != nil {
		d.immunePoison = *o.immunePoison
	}
	if o.immuneVenom != nil {
		d.immuneVenom = *o.immuneVenom
	}
	if o.freezeResistance != nil {
		d.freezeResistance = *o.freezeResistance
	}
	if o.projectileID != nil {
		d.projectileID = *o.projectileID
	}
	if o.launchGraphicID != nil {
		d.launchGraphicID = *o.launchGraphicID
	}
	if o.respawn != nil {
		d.respawn = *o.respawn
	}
	if o.huntRange != nil {
		d.huntRange = *o.huntRange
	}
	if o.attackAnim != nil {
		d.attackAnim = *o.attackAnim
	}
	if o.defenceAnim != nil {
		d.defenceAnim = *o.defenceAnim
	}
	if o.deathAnim != nil {
		d.deathAnim = *o.deathAnim
	}
	if o.spawnAnim != nil {
		v := *o.spawnAnim
		d.spawnAnim = &v
	}
	if o.deathSound != nil {
		d.deathSound = *o.deathSound
	}
	if o.deathTicks != nil {
		d.deathTicks = *o.deathTicks
	}
}

// --- Monster dump translation ------------------------------------

// combatStats is CombatStats (NpcDefinitionLoader.ts:12-46): one
// monsters-complete.json entry translated. Pointer fields are nil
// where the dump has no usable value, so the ?? fallbacks in the
// application loop leave the definition's own value alone.
type combatStats struct {
	name         any
	examine      *string
	hitpoints    *int
	attackLevel  *int
	strengthLvl  *int
	defenceLevel *int
	rangedLevel  *int
	magicLevel   *int
	attackSpeed  *int
	maxHit       *int
	aggressive   bool
	poisonous    bool
	venomous     bool
	demon        bool
	attributes   []string
	hasAttrs     bool
	members      bool
	weaknessType int
	weaknessPct  int
	attackType   CombatType
	attackStyle  *string
	slayerLevel  *int
	attackMelee  *int
	attackMagic  *int
	attackRanged *int
	defStab      *int
	defSlash     *int
	defCrush     *int
	defMagic     *int
	defRanged    *int
	defLight     *int
	defHeavy     *int
}

// finiteInt returns the truncated int for a finite JSON number, or
// false (Number.isFinite + Math.trunc, no coercion).
func finiteInt(v any) (int, bool) {
	n, ok := numberValue(v)
	if !ok {
		return 0, false
	}
	return jsTrunc(n), true
}

// fromMonsterDump is NpcDefinitionLoader.fromMonsterDump
// (NpcDefinitionLoader.ts:208-259).
func fromMonsterDump(monster any) *combatStats {
	stat := &combatStats{}
	stat.name, _ = field(monster, "name")
	if text, ok := mustNum(monster, "examine").(string); ok {
		stat.examine = &text
	}
	if v, ok := finiteInt(mustNum(monster, "hitpoints")); ok {
		stat.hitpoints = &v
	}
	if v, ok := finiteInt(mustNum(monster, "attack_level")); ok {
		stat.attackLevel = &v
	}
	if v, ok := finiteInt(mustNum(monster, "strength_level")); ok {
		stat.strengthLvl = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_level")); ok {
		stat.defenceLevel = &v
	}
	if v, ok := finiteInt(mustNum(monster, "ranged_level")); ok {
		stat.rangedLevel = &v
	}
	if v, ok := finiteInt(mustNum(monster, "magic_level")); ok {
		stat.magicLevel = &v
	}
	if v, ok := finiteInt(mustNum(monster, "attack_speed")); ok {
		stat.attackSpeed = &v
	}
	// null on ~141 entries of the dump.
	if n, ok := numberValue(mustNum(monster, "max_hit")); ok {
		v := jsTrunc(n)
		stat.maxHit = &v
	}
	stat.aggressive = boolField(monster, "aggressive")
	stat.poisonous = boolField(monster, "poisonous")
	stat.venomous = boolField(monster, "venomous")
	stat.members = boolField(monster, "members")
	if attrs, ok := mustNum(monster, "attributes").([]any); ok {
		stat.hasAttrs = true
		for _, attr := range attrs {
			stat.attributes = append(stat.attributes, jsString(attr, true))
		}
		// attributes.includes("demon") on the raw array: only an
		// exact string element counts.
		for _, attr := range attrs {
			if s, ok := attr.(string); ok && s == "demon" {
				stat.demon = true
				break
			}
		}
	}
	weaknessType := 0
	if n, ok := numberValue(mustNum(monster, "elemental_weakness_type")); ok && isJSInteger(n) && n >= 1 && n <= 4 {
		weaknessType = jsTrunc(n)
	}
	weaknessPercent := 0
	if n, ok := numberValue(mustNum(monster, "elemental_weakness_percent")); ok && n >= 0 {
		weaknessPercent = int(math.Floor(n))
	}
	stat.weaknessType = weaknessType
	if weaknessType != 0 {
		stat.weaknessPct = weaknessPercent
	}
	attackTypes, _ := mustNum(monster, "attack_type").([]any)
	stat.attackType = ResolveAttackType(attackTypes)
	if len(attackTypes) > 0 {
		style := strings.ToLower(jsString(attackTypes[0], true))
		stat.attackStyle = &style
	}
	if boolField(monster, "slayer_monster") {
		if level, present := field(monster, "slayer_level"); present && jsNumber(level, true) > 0 {
			v := jsTrunc(jsNumber(level, true))
			stat.slayerLevel = &v
		}
	}
	// Accuracy only: damage comes from maxHit, so the dump's
	// strength/magic/ranged damage bonuses are deliberately unread.
	if v, ok := finiteInt(mustNum(monster, "attack_bonus")); ok {
		stat.attackMelee = &v
	}
	if v, ok := finiteInt(mustNum(monster, "attack_magic")); ok {
		stat.attackMagic = &v
	}
	if v, ok := finiteInt(mustNum(monster, "attack_ranged")); ok {
		stat.attackRanged = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_stab")); ok {
		stat.defStab = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_slash")); ok {
		stat.defSlash = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_crush")); ok {
		stat.defCrush = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_magic")); ok {
		stat.defMagic = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_ranged_standard")); ok {
		stat.defRanged = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_ranged_light")); ok {
		stat.defLight = &v
	}
	if v, ok := finiteInt(mustNum(monster, "defence_ranged_heavy")); ok {
		stat.defHeavy = &v
	}
	return stat
}

var meleeAttackTypes = map[string]bool{"stab": true, "slash": true, "crush": true, "melee": true}

// ResolveAttackType is NpcDefinitionLoader.resolveAttackType
// (NpcDefinitionLoader.ts:267-276): anything that can melee keeps
// meleeing; only pure ranged/magic monsters switch. `dragonfire`
// and `typeless` fall through to melee. A non-array value yields
// melee.
func ResolveAttackType(attackTypes any) CombatType {
	arr, ok := attackTypes.([]any)
	if !ok {
		return CombatTypeMelee
	}
	types := make([]string, len(arr))
	for i, t := range arr {
		types[i] = strings.ToLower(jsString(t, true))
	}
	for _, t := range types {
		if meleeAttackTypes[t] {
			return CombatTypeMelee
		}
	}
	for _, t := range types {
		if t == "ranged" {
			return CombatTypeRanged
		}
	}
	for _, t := range types {
		if t == "magic" {
			return CombatTypeMagic
		}
	}
	return CombatTypeMelee
}

// jsTruthy is JavaScript truthiness for decoded JSON values.
func jsTruthy(v any) bool {
	switch t := v.(type) {
	case nil:
		return false
	case bool:
		return t
	case float64:
		return t != 0 && !math.IsNaN(t)
	case string:
		return t != ""
	default:
		return true
	}
}

// --- Animation inference -----------------------------------------

type combatAnimationRole string

const (
	roleAttack combatAnimationRole = "attack"
	roleBlock  combatAnimationRole = "block"
	roleDeath  combatAnimationRole = "death"
)

var deathWords = map[string]bool{"death": true, "die": true, "dies": true, "dying": true, "dead": true}
var blockWords = map[string]bool{"block": true, "blocking": true, "defend": true, "defending": true, "defence": true, "defense": true, "def": true, "parry": true}
var attackWords = map[string]bool{
	"attack": true, "att": true, "slash": true, "stab": true, "crush": true, "lunge": true,
	"pound": true, "swipe": true, "smash": true, "slam": true, "bite": true, "claw": true,
	"sting": true, "peck": true, "scratch": true, "whip": true, "headbutt": true, "shoot": true,
	"throw": true, "spit": true, "spell": true,
}
var styleWords = map[string]bool{"melee": true, "ranged": true, "range": true, "magic": true, "mage": true}
var motionWords = map[string]bool{"idle": true, "ready": true, "walk": true, "run": true, "stand": true, "standing": true}
var variantWords = map[string]bool{"special": true, "spec": true, "cutscene": true, "loop": true, "reverse": true, "fake": true, "quick": true, "fast": true, "transition": true}

var blockSuffixPattern = regexp.MustCompile(`(block|parry|defend)$`)
var attackSubstringPattern = regexp.MustCompile(`attack|punch|kick|strike`)

// animationWords splits a gameval name on "_" and strips trailing
// digits from each word (NpcDefinitionLoader.ts:70).
func animationWords(name string) []string {
	var words []string
	for _, word := range strings.Split(name, "_") {
		word = strings.TrimRight(word, "0123456789")
		if word != "" {
			words = append(words, word)
		}
	}
	return words
}

// animationRoleByName is animationRoleByName
// (NpcDefinitionLoader.ts:78-88): the combat role an animation's
// gameval name gives it, or "" when the name does not say. Death
// and block words win over attack words.
func animationRoleByName(name string) combatAnimationRole {
	if name == "" {
		return ""
	}
	words := animationWords(name)
	for _, word := range words {
		if deathWords[word] || strings.HasPrefix(word, "death") || strings.HasSuffix(word, "death") {
			return roleDeath
		}
	}
	for _, word := range words {
		if blockWords[word] || blockSuffixPattern.MatchString(word) {
			return roleBlock
		}
	}
	for _, word := range words {
		if attackWords[word] || attackSubstringPattern.MatchString(word) || strings.HasPrefix(word, "cast") {
			return roleAttack
		}
	}
	hasStyle, hasMotion := false, false
	for _, word := range words {
		if styleWords[word] {
			hasStyle = true
		}
		if motionWords[word] {
			hasMotion = true
		}
	}
	if hasStyle && !hasMotion {
		return roleAttack
	}
	return ""
}

// combatAnims is the anims object of a CombatAnimation: nil fields
// mean the role is unset (TS undefined). Explicit undefined/null
// values in the source collapse to unset here; the one observable
// difference from TS spread semantics — an explicit undefined
// blocking a captured role in a merge — cannot occur in the
// shipped files, whose rows are complete.
type combatAnims struct {
	spawn  *int
	attack *int
	block  *int
	death  *int
}

func (a combatAnims) role(role combatAnimationRole) *int {
	switch role {
	case roleAttack:
		return a.attack
	case roleBlock:
		return a.block
	case roleDeath:
		return a.death
	}
	return nil
}

func (a *combatAnims) setRole(role combatAnimationRole, v *int) {
	switch role {
	case roleAttack:
		a.attack = v
	case roleBlock:
		a.block = v
	case roleDeath:
		a.death = v
	}
}

// countRoles counts the set combat roles (spawn excluded: the TS
// log counts use Object.keys of the anims object, and spawn is
// never set by inference).
func (a combatAnims) countRoles() int {
	n := 0
	for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
		if a.role(role) != nil {
			n++
		}
	}
	return n
}

// combatAnimationEntry is one merged CombatAnimation for an NPC id.
type combatAnimationEntry struct {
	anims       combatAnims
	soundsDeath *int
	deathTicks  *int
}

// parseCombatAnims extracts the anims object of a CombatAnimation
// entry. Role values are finite numbers, truncated (the apply loop
// assigns them with ?? and no integer check).
func parseCombatAnims(animsValue any) combatAnims {
	var anims combatAnims
	animsObj, ok := animsValue.(map[string]any)
	if !ok {
		return anims
	}
	if v, ok := finiteInt(mustNum(animsObj, "spawn")); ok {
		anims.spawn = &v
	}
	if v, ok := finiteInt(mustNum(animsObj, "attack")); ok {
		anims.attack = &v
	}
	if v, ok := finiteInt(mustNum(animsObj, "block")); ok {
		anims.block = &v
	}
	if v, ok := finiteInt(mustNum(animsObj, "death")); ok {
		anims.death = &v
	}
	return anims
}

// familyRoles is familyRoles (NpcDefinitionLoader.ts:105-132):
// each role's animation from the family the stand animation names.
// The family index is built eagerly from the Store's seq names.
func (s *Store) familyRoles(standAnimation int, families map[string][]familyMember) combatAnims {
	var roles combatAnims
	if standAnimation < 0 {
		return roles
	}
	standName := s.seqName(standAnimation)
	if standName == "" {
		return roles
	}
	var family []string
	for _, word := range animationWords(standName) {
		if motionWords[word] || strings.HasPrefix(word, "ready") || strings.HasPrefix(word, "idle") {
			continue
		}
		family = append(family, word)
	}
	// Family names too broad to pick from: every human NPC shares
	// `human_ready`, and the defaults are human.
	if len(family) == 0 || (len(family) == 1 && family[0] == "human") {
		return roles
	}
	type bestEntry struct {
		id    int
		extra int
	}
	best := map[combatAnimationRole]bestEntry{}
	for _, member := range families[family[0]] {
		matches := true
		for _, word := range family {
			found := false
			for _, w := range member.words {
				if w == word {
					found = true
					break
				}
			}
			if !found {
				matches = false
				break
			}
		}
		if !matches {
			continue
		}
		extra := len(member.words) - len(family)
		for _, word := range member.words {
			if variantWords[word] {
				extra += 3
				break
			}
		}
		current, exists := best[member.role]
		if !exists || extra < current.extra || (extra == current.extra && member.id < current.id) {
			best[member.role] = bestEntry{id: member.id, extra: extra}
		}
	}
	for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
		if entry, ok := best[role]; ok && entry.extra <= 2 {
			id := entry.id
			roles.setRole(role, &id)
		}
	}
	return roles
}

type familyMember struct {
	id    int
	words []string
	role  combatAnimationRole
}

// buildAnimationFamilies is the lazy animationFamilies index
// (NpcDefinitionLoader.ts:116-125), built eagerly here: role-bearing
// animations keyed by their first word, in ascending id order.
func (s *Store) buildAnimationFamilies() map[string][]familyMember {
	ids := make([]int, 0, len(s.seqNames))
	for id := range s.seqNames {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	families := make(map[string][]familyMember)
	for _, id := range ids {
		name := s.seqNames[id]
		role := animationRoleByName(name)
		if role == "" {
			continue
		}
		words := animationWords(name)
		if len(words) == 0 {
			continue
		}
		families[words[0]] = append(families[words[0]], familyMember{id: id, words: words, role: role})
	}
	return families
}

// capturedNoBlockHits is CAPTURED_NO_BLOCK_HITS
// (NpcDefinitionLoader.ts:143).
const capturedNoBlockHits = 10

// capturedRoles is capturedRoles (NpcDefinitionLoader.ts:153-170).
// A malformed seen value (not an array of counts) is skipped; TS
// would throw destructuring it, but the shipped captures file is
// well-formed and a fixture typo should not fail the boot.
func (s *Store) capturedRoles(entry any) combatAnims {
	var roles combatAnims
	seen := objField(entry, "seen")
	type bestEntry struct {
		id    int
		count int
	}
	best := map[combatAnimationRole]bestEntry{}
	for _, key := range sortedNumericKeys(seen) {
		id := jsTrunc(jsParseNumber(key))
		counts, ok := seen[key].([]any)
		if !ok {
			continue
		}
		var death, block, attack float64 = math.NaN(), math.NaN(), math.NaN()
		if len(counts) > 0 {
			if n, ok := numberValue(counts[0]); ok {
				death = n
			}
		}
		if len(counts) > 1 {
			if n, ok := numberValue(counts[1]); ok {
				block = n
			}
		}
		if len(counts) > 2 {
			if n, ok := numberValue(counts[2]); ok {
				attack = n
			}
		}
		// A missing count is undefined in TS: comparisons against
		// it are false, and a count sum involving it is NaN,
		// which never beats the -1 seed. NaN here propagates
		// exactly the same way.
		role := animationRoleByName(s.seqName(id))
		if role == "" {
			if death >= 2 && death >= block+attack {
				role = roleDeath
			} else if block >= 5 && block >= 9*(attack+death) {
				role = roleBlock
			}
		}
		if role == "" {
			continue
		}
		var count float64
		if role == roleDeath {
			count = death
		} else {
			count = block + attack
		}
		if math.IsNaN(count) {
			continue
		}
		current, exists := best[role]
		if !exists || float64(current.count) < count {
			best[role] = bestEntry{id: id, count: int(count)}
		}
	}
	for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
		if entry, ok := best[role]; ok {
			id := entry.id
			roles.setRole(role, &id)
		}
	}
	// An NPC hit often that mostly does not animate has no block.
	if roles.block == nil {
		if hits, ok := mustNum(entry, "hits").([]any); ok && len(hits) >= 2 {
			total, okTotal := numberValue(hits[0])
			unanswered, okUnanswered := numberValue(hits[1])
			if okTotal && okUnanswered && total >= capturedNoBlockHits && unanswered >= total/2 {
				neg := -1
				roles.block = &neg
			}
		}
	}
	return roles
}

// --- Loader ------------------------------------------------------

// loadNpcDefinitions ports NpcDefinitionLoader.load()
// (NpcDefinitionLoader.ts:301-493). monsters-complete.json,
// npc-status-immunities.json and npc-captured-animations.json are
// optional when missing (warn, empty); a present-but-malformed
// file fails Load, and a status file whose schema_version is not
// exactly 1 fails Load. npc-combat-defs.json and
// npc-animations.json are REQUIRED. With no cache the files are
// still read and validated, but nothing is computed: the
// name-guarded application and the animation inference both need
// cache NPCs and sequence names, and TS has no cache-less path at
// all (CacheDefinitions.getState throws) — the Go degradation is
// to keep the wrappers at their DEFAULT shapes.
//
// applyCustomCombat (NpcDefinitionLoader.ts:184-195) is a no-op
// here: it applies only to cloned custom NPC ids, and there are no
// custom definitions in this package.
func (s *Store) loadNpcDefinitions() error {
	directory := s.dir
	monstersValue, err := readJSONValueOptional(filepath.Join(directory, "monsters-complete.json"))
	if err != nil {
		return err
	}
	if monstersValue == nil {
		s.logf("[npc-definitions] monsters-complete.json missing - NPC combat stats will fall back to defaults")
	}
	monsters := asKeyedObject(monstersValue)

	statusValue, err := readJSONValueOptional(filepath.Join(directory, "npc-status-immunities.json"))
	if err != nil {
		return err
	}
	if statusValue == nil {
		s.logf("[npc-definitions] npc-status-immunities.json missing - NPC combat stats will fall back to defaults")
	}
	var statusImmunities map[string]any
	if statusValue != nil {
		if version, ok := field(statusValue, "schema_version"); !ok || version != float64(1) {
			return fmt.Errorf("Unsupported NPC status immunity schema")
		}
		statusImmunities = asKeyedObject(mustNum(statusValue, "npcs"))
	} else {
		statusImmunities = make(map[string]any)
	}

	animationFile, err := readJSONValue(filepath.Join(directory, "npc-combat-defs.json"))
	if err != nil {
		return err
	}
	observedValue, err := readJSONValue(filepath.Join(directory, "npc-animations.json"))
	if err != nil {
		return err
	}
	observedAnimations := asKeyedObject(observedValue)

	capturedValue, err := readJSONValueOptional(filepath.Join(directory, "npc-captured-animations.json"))
	if err != nil {
		return err
	}
	if capturedValue == nil {
		s.logf("[npc-definitions] npc-captured-animations.json missing - NPC combat stats will fall back to defaults")
	}
	captured := asKeyedObject(mustNum(capturedValue, "npcs"))

	if s.cache == nil {
		return nil
	}

	animations := make(map[string]*combatAnimationEntry)
	// refs.npcs rows are [id, attack, block, death].
	refs := objField(animationFile, "refs")
	if refs != nil {
		if rowsValue, present := field(refs, "npcs"); present && rowsValue != nil {
			rows, ok := rowsValue.([]any)
			if !ok {
				return fmt.Errorf("npc-combat-defs.json refs.npcs must contain an array")
			}
			for _, row := range rows {
				cells, ok := row.([]any)
				if !ok {
					return fmt.Errorf("npc-combat-defs.json refs.npcs rows must be arrays")
				}
				var anims combatAnims
				if len(cells) > 1 {
					if v, ok := finiteInt(cells[1]); ok {
						anims.attack = &v
					}
				}
				if len(cells) > 2 {
					if v, ok := finiteInt(cells[2]); ok {
						anims.block = &v
					}
				}
				if len(cells) > 3 {
					if v, ok := finiteInt(cells[3]); ok {
						anims.death = &v
					}
				}
				// String(id): the row id stringifies as JS does.
				animations[jsString(cells[0], len(cells) > 0)] = &combatAnimationEntry{anims: anims}
			}
		}
	}
	npcsFile := asKeyedObject(mustNum(animationFile, "npcs"))
	projectiles := make(map[string]int)
	launchGraphics := make(map[string]int)
	for _, key := range sortedNumericKeys(npcsFile) {
		entry := npcsFile[key]
		if animsValue, present := field(entry, "anims"); present && jsTruthy(animsValue) {
			parsed := &combatAnimationEntry{anims: parseCombatAnims(animsValue)}
			if sounds := objField(entry, "sounds"); sounds != nil {
				if v, ok := finiteInt(mustNum(sounds, "death")); ok {
					parsed.soundsDeath = &v
				}
			}
			if v, ok := intField(entry, "deathTicks"); ok {
				parsed.deathTicks = &v
			}
			animations[key] = parsed
		}
		if v, ok := intField(entry, "projectile"); ok {
			projectiles[key] = v
		}
		if v, ok := intField(entry, "launchGraphic"); ok {
			launchGraphics[key] = v
		}
	}
	// Only hand-made entries stop the guess below; captures leave
	// a role open when they do not settle it.
	configuredAnimationIDs := make(map[string]bool, len(animations))
	for key := range animations {
		configuredAnimationIDs[key] = true
	}
	// Animations seen in live captures fill what a hand-made entry
	// leaves out.
	capturedAnimations := 0
	for _, key := range sortedNumericKeys(captured) {
		roles := s.capturedRoles(captured[key])
		capturedAnimations += roles.countRoles()
		if roles.countRoles() == 0 {
			continue
		}
		merged := combatAnims{spawn: nil, attack: roles.attack, block: roles.block, death: roles.death}
		existing := animations[key]
		if existing != nil {
			if existing.anims.attack != nil {
				merged.attack = existing.anims.attack
			}
			if existing.anims.block != nil {
				merged.block = existing.anims.block
			}
			if existing.anims.death != nil {
				merged.death = existing.anims.death
			}
		}
		entry := &combatAnimationEntry{anims: merged}
		if existing != nil {
			entry.soundsDeath = existing.soundsDeath
			entry.deathTicks = existing.deathTicks
		}
		animations[key] = entry
	}

	roleByAnimation := make(map[int]combatAnimationRole)
	for _, key := range sortedNumericKeys(animations) {
		for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
			if v := animations[key].anims.role(role); v != nil {
				roleByAnimation[*v] = role
			}
		}
	}
	inferredAnimations, namedAnimations, guessedAnimations := 0, 0, 0
	for _, npcID := range sortedNumericKeys(observedAnimations) {
		observed, ok := observedAnimations[npcID].([]any)
		if !ok || len(observed) == 0 {
			continue
		}
		// An observed list has no order to trust: a role comes
		// from the same animation's role in a configured or
		// captured entry, then from the animation's cache name,
		// and only then by guess.
		var inferred combatAnims
		for _, raw := range observed {
			id, ok := animationID(raw)
			if !ok {
				continue
			}
			if role, ok := roleByAnimation[id]; ok && inferred.role(role) == nil {
				v := id
				inferred.setRole(role, &v)
			}
		}
		type namedRole struct {
			id   int
			role combatAnimationRole
		}
		var named []namedRole
		namedSet := make(map[int]bool)
		for _, raw := range observed {
			id, ok := animationID(raw)
			if !ok || namedSet[id] {
				continue
			}
			if role := animationRoleByName(s.seqName(id)); role != "" {
				named = append(named, namedRole{id: id, role: role})
				namedSet[id] = true
			}
		}
		inferredUsed := make(map[int]bool)
		for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
			if v := inferred.role(role); v != nil {
				inferredUsed[*v] = true
			}
		}
		for _, nr := range named {
			if inferred.role(nr.role) != nil || inferredUsed[nr.id] {
				continue
			}
			v := nr.id
			inferred.setRole(nr.role, &v)
			inferredUsed[nr.id] = true
			namedAnimations++
		}
		resolved := inferred
		if explicit := animations[npcID]; explicit != nil {
			if explicit.anims.attack != nil {
				resolved.attack = explicit.anims.attack
			}
			if explicit.anims.block != nil {
				resolved.block = explicit.anims.block
			}
			if explicit.anims.death != nil {
				resolved.death = explicit.anims.death
			}
			if explicit.anims.spawn != nil {
				resolved.spawn = explicit.anims.spawn
			}
		}
		inferredAnimations += inferred.countRoles()
		if !configuredAnimationIDs[npcID] {
			// A named animation never fills another role: a
			// death is not guessed as an attack.
			var ids []int
			seenIDs := make(map[int]bool)
			for _, raw := range observed {
				id, ok := animationID(raw)
				if !ok || id < 0 || namedSet[id] || seenIDs[id] {
					continue
				}
				seenIDs[id] = true
				ids = append(ids, id)
			}
			indexes := make([]int, len(ids))
			for i := range ids {
				indexes[i] = i
			}
			preferences := map[combatAnimationRole][]int{
				roleBlock: indexes,
			}
			if len(ids) <= 2 {
				preferences[roleAttack] = indexes
			} else {
				attackPrefs := []int{1}
				for _, index := range indexes {
					if index != 1 {
						attackPrefs = append(attackPrefs, index)
					}
				}
				preferences[roleAttack] = attackPrefs
			}
			if len(ids) >= 3 {
				deathPrefs := make([]int, len(indexes))
				for i, index := range indexes {
					deathPrefs[len(indexes)-1-i] = index
				}
				preferences[roleDeath] = deathPrefs
			} else {
				preferences[roleDeath] = nil
			}
			used := make(map[int]bool)
			for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
				if v := resolved.role(role); v != nil {
					used[*v] = true
				}
			}
			for _, role := range []combatAnimationRole{roleAttack, roleBlock, roleDeath} {
				if resolved.role(role) != nil {
					continue
				}
				for _, index := range preferences[role] {
					id := ids[index]
					if used[id] {
						continue
					}
					v := id
					resolved.setRole(role, &v)
					used[id] = true
					guessedAnimations++
					break
				}
			}
		}
		if resolved.countRoles() > 0 || resolved.spawn != nil {
			entry := &combatAnimationEntry{anims: resolved}
			if existing := animations[npcID]; existing != nil {
				entry.soundsDeath = existing.soundsDeath
				entry.deathTicks = existing.deathTicks
			}
			animations[npcID] = entry
		}
	}

	// Last, an attackable NPC's roles that nothing above covers
	// come from its stand animation's family.
	familyAnimations := 0
	families := s.buildAnimationFamilies()
	npcCount := s.npcCount()
	for id := 0; id < npcCount; id++ {
		cached := s.cache.GetNpc(id)
		attackable := false
		for _, action := range cached.Actions {
			if action == "Attack" {
				attackable = true
				break
			}
		}
		if !attackable {
			continue
		}
		var current combatAnims
		if entry := animations[strconv.Itoa(id)]; entry != nil {
			current = entry.anims
		}
		if current.attack != nil && current.block != nil && current.death != nil {
			continue
		}
		family := s.familyRoles(cached.IdleSeqId, families)
		filled := family
		if current.attack != nil {
			filled.attack = current.attack
		}
		if current.block != nil {
			filled.block = current.block
		}
		if current.death != nil {
			filled.death = current.death
		}
		if current.spawn != nil {
			filled.spawn = current.spawn
		}
		gained := filled.countRoles() - current.countRoles()
		familyAnimations += gained
		if gained > 0 {
			entry := &combatAnimationEntry{anims: filled}
			if existing := animations[strconv.Itoa(id)]; existing != nil {
				entry.soundsDeath = existing.soundsDeath
				entry.deathTicks = existing.deathTicks
			}
			animations[strconv.Itoa(id)] = entry
		}
	}

	idSet := make(map[string]bool)
	for _, keys := range []map[string]any{monsters, statusImmunities, npcsFile, observedAnimations} {
		for key := range keys {
			idSet[key] = true
		}
	}
	for key := range animations {
		idSet[key] = true
	}
	for key := range projectiles {
		idSet[key] = true
	}
	for key := range launchGraphics {
		idSet[key] = true
	}
	keys := make([]string, 0, len(idSet))
	for key := range idSet {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		ni, nj := jsParseNumber(keys[i]), jsParseNumber(keys[j])
		if ni != nj {
			return ni < nj
		}
		return keys[i] < keys[j]
	})

	applied, mismatched := 0, 0
	for _, key := range keys {
		idNum := jsParseNumber(key)
		if !isJSInteger(idNum) {
			continue
		}
		id := jsTrunc(idNum)
		if id < 0 || id >= npcCount {
			continue
		}
		cached := s.cache.GetNpc(id)
		overlay := &npcOverlay{}
		hasOverlay := false
		monsterValue, hasMonster := monsters[key]
		var stat *combatStats
		if hasMonster && jsTruthy(monsterValue) {
			stat = fromMonsterDump(monsterValue)
		}
		// Name-guarded: a dump keyed to a different cache
		// revision is dropped, not applied to whatever monster
		// happens to sit at that id here.
		if stat != nil && normalizeName(stat.name) == normalizeName(cached.Name) {
			hydrated := newDefaultNpcDefinition()
			hydrated.hydrateFromCache(id, cached, s)
			levels := hydrated.stats
			for _, pair := range []struct {
				index int
				value *int
			}{
				{0, stat.attackLevel}, {1, stat.strengthLvl}, {2, stat.defenceLevel},
				{3, stat.rangedLevel}, {4, stat.magicLevel},
				{5, stat.attackMelee}, {7, stat.attackMagic}, {9, stat.attackRanged},
				{10, stat.defStab}, {11, stat.defSlash}, {12, stat.defCrush},
				{13, stat.defMagic}, {14, stat.defRanged},
			} {
				if pair.value != nil {
					levels[pair.index] = *pair.value
				}
			}
			overlay.hasStats = true
			overlay.stats = levels
			overlay.rangedLight = stat.defLight
			overlay.rangedHeavy = stat.defHeavy
			overlay.weaknessType = stat.weaknessType
			overlay.weaknessPercent = stat.weaknessPct
			overlay.examine = stat.examine
			overlay.hitpoints = stat.hitpoints
			overlay.attackSpeed = stat.attackSpeed
			overlay.maxHit = stat.maxHit
			overlay.aggressive = stat.aggressive
			overlay.poisonous = stat.poisonous
			overlay.venomous = stat.venomous
			overlay.demon = stat.demon
			overlay.members = stat.members
			overlay.attackType = stat.attackType
			overlay.attackStyle = stat.attackStyle
			overlay.slayerLevel = stat.slayerLevel
			if stat.hasAttrs {
				overlay.hasAttributes = true
				overlay.attributes = append([]string(nil), stat.attributes...)
			}
			hasOverlay = true
			applied++
		} else if stat != nil {
			mismatched++
		}

		// Status rules have one canonical source, independent of
		// combat-stat snapshots (applyStatusImmunities,
		// NpcDefinitionLoader.ts:278-286).
		if source, ok := statusImmunities[key]; ok && source != nil {
			if name, ok := mustNum(source, "name").(string); ok && normalizeName(name) == normalizeName(cached.Name) {
				immunePoison := boolField(source, "immune_poison")
				immuneVenom := boolField(source, "immune_venom")
				overlay.immunePoison = &immunePoison
				overlay.immuneVenom = &immuneVenom
				freeze := 0
				if n, ok := numberValue(mustNum(source, "freeze_resistance")); ok && isJSInteger(n) {
					freeze = jsTrunc(n)
					if freeze < 0 {
						freeze = 0
					}
					if freeze > 100 {
						freeze = 100
					}
				}
				overlay.freezeResistance = &freeze
				hasOverlay = true
			}
		}

		if projectile, ok := projectiles[key]; ok {
			v := projectile
			overlay.projectileID = &v
			hasOverlay = true
		}
		if graphic, ok := launchGraphics[key]; ok {
			v := graphic
			overlay.launchGraphicID = &v
			hasOverlay = true
		}
		if respawn, ok := intField(npcsFile[key], "respawn"); ok && respawn >= 0 {
			v := respawn
			if v < 0 {
				v = 0
			}
			overlay.respawn = &v
			hasOverlay = true
		}
		if huntRange, ok := intField(npcsFile[key], "huntRange"); ok && huntRange >= 1 {
			v := huntRange
			if v < 1 {
				v = 1
			}
			overlay.huntRange = &v
			hasOverlay = true
		}
		if animation := animations[key]; animation != nil {
			if animation.anims.attack != nil {
				overlay.attackAnim = animation.anims.attack
				hasOverlay = true
			}
			if animation.anims.block != nil {
				overlay.defenceAnim = animation.anims.block
				hasOverlay = true
			}
			if animation.anims.death != nil {
				overlay.deathAnim = animation.anims.death
				hasOverlay = true
			}
			if animation.anims.spawn != nil {
				overlay.spawnAnim = animation.anims.spawn
				hasOverlay = true
			}
			if animation.soundsDeath != nil {
				overlay.deathSound = animation.soundsDeath
				hasOverlay = true
			}
			if animation.deathTicks != nil {
				overlay.deathTicks = animation.deathTicks
				hasOverlay = true
			}
		}
		if hasOverlay {
			s.npcOverlays[id] = overlay
		}
	}
	s.logf("[npc-definitions] cache-backed; configured stats applied=%d, mismatched=%d, service animations captured=%d, inferred=%d (by name=%d), guessed=%d, by family=%d",
		applied, mismatched, capturedAnimations, inferredAnimations, namedAnimations, guessedAnimations, familyAnimations)
	return nil
}

// animationID coerces an observed-animation list element to an id:
// only integer-valued numbers participate (the role maps are keyed
// by number, and the guessing filter requires Number.isInteger).
func animationID(raw any) (int, bool) {
	n, ok := numberValue(raw)
	if !ok || !isJSInteger(n) {
		return 0, false
	}
	return jsTrunc(n), true
}
