package game

import (
	"math"
	"math/rand/v2"
)

// This file is the combat core's view of a fighter. TS Combat works
// directly on Mobile, which carries hitpoints, size, skills and
// equipment accessors (Mobile.ts, Player.ts, NPC.ts). Go's Mobile
// carries none of those — the typed state lives on *Player and *NPC,
// and entity.go / player.go / npc.go belong to other work — so
// combat reads fighters through the Combatant interface, bound onto
// each Combat by the shadowing GetCombat accessors in combat.go
// (the same shadowing precedent as ResetUpdating on both types).
// A Combat whose identity was never bound (only reachable through
// a bare *Mobile) reads as an unknown combatant: hitpoints 0, so
// validTarget fails closed and no damage is ever applied to it.
//
// NPC definition data reaches combat through NPCCombatDefinition:
// *definition.NpcDefinition satisfies it directly (like the driver
// NPCDefinition interface in npc.go), and an NPC with no installed
// definition reads the DEFAULT shape. The game package already
// imports internal/definition (npc.go), so no new layering is
// introduced.

// NPCCombatDefinition is the NPC-definition surface combat reads:
// the driver interface (npc.go NPCDefinition) plus the combat
// fields. *definition.NpcDefinition satisfies it directly. The
// per-NPC mutable overrides TS carries on the NPC itself
// (defence-bonus overrides, max-level boosts, stat drains,
// combat-XP multiplier, hitpoints override) are NOT ported: no
// such fields exist on Go's NPC, and the drain/restore system
// belongs to a later milestone. Combat reads definition values
// only; see the NPCCombatStats comment in the formula file.
type NPCCombatDefinition interface {
	NPCDefinition
	IsAttackable() bool
	DoesRetreat() bool
	DoesFightBack() bool
	IsPoisonous() bool
	IsVenomous() bool
	IsPoisonImmune() bool
	IsVenomImmune() bool
	FreezeResistance() int
	IsDemon() bool
	MaxHit() int
	AttackSpeed() int
	AttackAnim() int
	DefenceAnim() int
	DeathAnim() int
	DeathTicks() int
	Stats() []int
	ProjectileID() int
	RangedDefenceBonus(kind string) int
}

// npcCombatDefinition returns the NPC's definition as the combat
// surface, or nil when the installed definition does not carry the
// combat getters (the DEFAULT shape does not). Callers fall back
// to defaultNPCCombatStats.
func npcCombatDefinition(n *NPC) NPCCombatDefinition {
	if n == nil {
		return nil
	}
	if d, ok := n.Definition().(NPCCombatDefinition); ok {
		return d
	}
	return nil
}

// defaultNPCStats is NpcDefinition.DEFAULT_STATS
// (NpcDefinition.ts: the 18-slot default): levels 1 in the five
// combat skills, every bonus 0. It is what an NPC with no combat
// definition reads, exactly as TS forId yields DEFAULT for an
// out-of-range id.
var defaultNPCStats = [18]int{1, 1, 1, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}

// npcStat reads one slot of an NPC's stats array, defaulting to
// defaultNPCStats when the definition is absent or the array is
// short. Stats slots (NPC.ts getBaseAttack comment): 0 attack,
// 1 strength, 2 defence, 3 ranged, 4 magic, 5 melee attack bonus,
// 6 melee strength bonus, 7 magic attack bonus, 8 ranged strength
// bonus, 9 ranged attack bonus, 10..14 defence bonuses
// (stab/slash/crush/magic/range).
func npcStat(d NPCCombatDefinition, slot int) int {
	if d != nil {
		if s := d.Stats(); slot >= 0 && slot < len(s) {
			return s[slot]
		}
	}
	if slot >= 0 && slot < len(defaultNPCStats) {
		return defaultNPCStats[slot]
	}
	return 0
}

// Combatant is everything combat needs to know about a fighter.
// playerCombatant and npcCombatant adapt *Player and *NPC; the
// unknownCombatant fail-closed adapter covers unbound Mobiles.
type Combatant interface {
	// Mobile is the fighter's Mobile: location, timers, flags,
	// attributes, registration. Never nil.
	Mobile() *Mobile
	// IsPlayer distinguishes players from NPCs (Mobile.ts
	// isPlayer). NPCs answer false.
	IsPlayer() bool
	// Name is the display name (player) or definition name
	// (NPC, "null" when unknown), for combat messages.
	Name() string
	// Size is the footprint in tiles (Mobile.ts getSize): 1 for
	// players, the definition size for NPCs.
	Size() int
	// Hitpoints is the current hitpoints. For an NPC whose
	// Hitpoints field is unset (0), the definition hitpoints
	// (NPC.ts:195-199 constructor assignment).
	Hitpoints() int
	// SetHitpoints writes current hitpoints. Player writes go
	// through SkillManager (Player.setHitpoints); NPC writes go
	// to the Hitpoints field. Neither write triggers death here:
	// death is driven by DecrementHealth (combat_death.go),
	// mirroring the TS setHitpoints -> appendDeath call chain
	// without editing player.go/npc.go.
	SetHitpoints(hp int)
	// MaxHitpoints is the hitpoints cap: the Hitpoints skill max
	// level for players, the definition hitpoints for NPCs
	// (NPC.ts getMaxHitpoints, no override in Go).
	MaxHitpoints() int
	// Dying reports whether the death sequence has started
	// (Mobile.ts isDying). Players track it on Combat (no
	// field exists on Player); NPCs read the Dying field.
	Dying() bool
	// SetDying records the death-sequence state.
	SetDying(dying bool)
	// CurrentLevel is the current level of a combat skill:
	// SkillManager current levels for players; definition
	// stats for NPCs (NPC.ts getMaxLevel, no drains in Go).
	// Unsupported skills answer 0 (TS throws; nothing in
	// core asks an NPC for a non-combat skill).
	CurrentLevel(skill int) int
	// AttackBonus is the attack bonus at index (BonusAttackStab
	// ..BonusAttackRange): summed equipment bonuses for
	// players, definition stats for NPCs.
	AttackBonus(index int) int
	// DefenceBonus is the defence bonus at index: summed
	// equipment bonuses for players, definition stats[10+index]
	// for NPCs (index 4, ranged, reads the standard ranged
	// defence bonus — see NPC.ts getDefenceBonus).
	DefenceBonus(index int) int
	// OtherBonus is the "other" bonus at index
	// (OtherBonusStrength..OtherBonusPrayer): summed equipment
	// bonuses for players, definition stats for NPCs (melee
	// strength stats[6], ranged strength stats[8], magic
	// damage and prayer 0 — NPCs have neither in TS).
	OtherBonus(index int) int
	// MovementQueue is the fighter's walk queue, or nil for a
	// zero-value fighter.
	MovementQueue() *MovementQueue
	// Skills is the player's SkillManager, or nil for NPCs.
	Skills() *SkillManager
	// Equipment is the player's equipment, or nil for NPCs.
	Equipment() *Equipment
	// ItemDefinitions is the player's item-definition source,
	// or nil for NPCs.
	ItemDefinitions() ItemDefinitionSource
	// SendMessage / SendTypedMessage forward chat to the
	// fighter's session; NPCs drop messages silently.
	SendMessage(text string)
	SendTypedMessage(chatType int, message, from string)
	// IsPlayerBot reports whether the fighter is a player bot
	// (some combat messages are suppressed for bots).
	IsPlayerBot() bool
	// Username is the player's username, or "" for NPCs. It
	// keys the PvP attacked-by record.
	Username() string
	// Player returns the fighter as a *Player, or nil.
	Player() *Player
	// NPC returns the fighter as an *NPC, or nil.
	NPC() *NPC
	// NPCDef returns the NPC's combat definition, or nil for
	// players and for NPCs without a combat definition.
	NPCDef() NPCCombatDefinition
}

// playerCombatant adapts *Player to Combatant.
type playerCombatant struct{ p *Player }

func (c playerCombatant) Mobile() *Mobile               { return &c.p.Mobile }
func (c playerCombatant) IsPlayer() bool                { return true }
func (c playerCombatant) Name() string                  { return c.p.GetDisplayName() }
func (c playerCombatant) Size() int                     { return 1 }
func (c playerCombatant) Hitpoints() int                { return c.p.Skills().CurrentLevel(SkillHitpoints) }
func (c playerCombatant) SetHitpoints(hp int)           { c.p.Skills().SetCurrentLevel(SkillHitpoints, hp, true) }
func (c playerCombatant) MaxHitpoints() int             { return c.p.Skills().MaxLevel(SkillHitpoints) }
func (c playerCombatant) Dying() bool                   { return c.p.GetCombat().dying }
func (c playerCombatant) SetDying(dying bool)           { c.p.GetCombat().dying = dying }
func (c playerCombatant) CurrentLevel(skill int) int    { return c.p.Skills().CurrentLevel(skill) }
func (c playerCombatant) MovementQueue() *MovementQueue { return c.p.MovementQueue() }
func (c playerCombatant) Skills() *SkillManager         { return c.p.Skills() }
func (c playerCombatant) Equipment() *Equipment         { return c.p.Equipment() }
func (c playerCombatant) ItemDefinitions() ItemDefinitionSource {
	return c.p.ItemDefinitions()
}
func (c playerCombatant) SendMessage(text string) { c.p.SendMessage(text) }
func (c playerCombatant) SendTypedMessage(chatType int, message, from string) {
	c.p.SendTypedMessage(chatType, message, from)
}
func (c playerCombatant) IsPlayerBot() bool { return c.p.IsPlayerBot() }
func (c playerCombatant) Username() string  { return c.p.Username }
func (c playerCombatant) Player() *Player   { return c.p }
func (c playerCombatant) NPC() *NPC         { return nil }
func (c playerCombatant) NPCDef() NPCCombatDefinition {
	return nil
}

// AttackBonus sums the equipment attack bonuses at index
// (BonusManager.ts: the attack bonus array, indices 0..4). A
// nil item source or an item without bonus data contributes 0,
// so a bare test player reads as unarmed and naked.
func (c playerCombatant) AttackBonus(index int) int {
	return c.equipmentBonusSum(index)
}

// DefenceBonus sums the equipment defence bonuses at index
// (BonusManager.ts: the defence bonus array, stored after the
// five attack bonuses in the item bonus array).
func (c playerCombatant) DefenceBonus(index int) int {
	return c.equipmentBonusSum(5 + index)
}

// OtherBonus sums the equipment "other" bonuses at index
// (BonusManager.ts: strength, ranged strength, magic damage,
// prayer — stored after the ten attack/defence bonuses).
func (c playerCombatant) OtherBonus(index int) int {
	return c.equipmentBonusSum(10 + index)
}

func (c playerCombatant) equipmentBonusSum(bonusIndex int) int {
	// The sum is BonusManager.ComputeBonuses (BonusManager.ts):
	// the 14-entry equipment sum, minus the ammo slot's ranged
	// strength when the weapon does not fire from the ammo
	// slot. The formulas read every bonus through this one
	// seam — attackMeleeRoll, defenseMeleeRoll,
	// defenseRangedRoll, attackRangedRoll, defenseMagicRoll,
	// attackMagicRoll, CalculateMaxMeleeHit,
	// CalculateMaxRangedHit, ApplyMagicDamageBonus, and the
	// prayer-resistance read in combat_prayer.go — so the
	// ammo rule and any future veto apply to all of them at
	// once. A nil item source or an item without bonus data
	// contributes 0, so a bare test player reads as unarmed
	// and naked.
	if bonusIndex < 0 || bonusIndex >= 14 {
		return 0
	}
	return ComputeBonuses(c.p, nil).Bonus()[bonusIndex]
}

// npcCombatant adapts *NPC to Combatant.
type npcCombatant struct{ n *NPC }

func (c npcCombatant) Mobile() *Mobile { return &c.n.Mobile }
func (c npcCombatant) IsPlayer() bool  { return false }
func (c npcCombatant) Name() string {
	// The driver NPCDefinition interface carries no name; the
	// full definition does (NpcDefinition.Name). An NPC with
	// only the DEFAULT shape reads "null", the TS default name.
	if named, ok := c.n.Definition().(interface{ Name() string }); ok {
		return named.Name()
	}
	return "null"
}
func (c npcCombatant) Size() int       { return c.n.Size() }
func (c npcCombatant) NPC() *NPC       { return c.n }
func (c npcCombatant) Player() *Player { return nil }
func (c npcCombatant) NPCDef() NPCCombatDefinition {
	return npcCombatDefinition(c.n)
}
func (c npcCombatant) Hitpoints() int {
	if c.n.Hitpoints != 0 {
		return c.n.Hitpoints
	}
	// Zero means unset (npc.go): the TS constructor value.
	return c.n.Definition().Hitpoints()
}
func (c npcCombatant) SetHitpoints(hp int) { c.n.Hitpoints = hp }
func (c npcCombatant) MaxHitpoints() int   { return c.n.Definition().Hitpoints() }
func (c npcCombatant) Dying() bool         { return c.n.Dying }
func (c npcCombatant) SetDying(dying bool) { c.n.Dying = dying }

// CurrentLevel maps the five combat skills to definition stat
// slots (NPC.ts getMaxLevel: ATTACK/STRENGTH/DEFENCE/RANGED/
// MAGIC -> slots 0..4). Stat drains are NOT ported, so the
// current level is the base level.
func (c npcCombatant) CurrentLevel(skill int) int {
	d := c.NPCDef()
	switch skill {
	case SkillAttack:
		return npcStat(d, 0)
	case SkillStrength:
		return npcStat(d, 1)
	case SkillDefence:
		return npcStat(d, 2)
	case SkillRanged:
		return npcStat(d, 3)
	case SkillMagic:
		return npcStat(d, 4)
	default:
		return 0
	}
}

// AttackBonus reads the definition attack bonus for the index:
// melee indices share stats[5], magic reads stats[7], ranged
// reads stats[9] (AccuracyFormulasDpsCalc.ts NPC branches).
func (c npcCombatant) AttackBonus(index int) int {
	d := c.NPCDef()
	switch index {
	case BonusAttackMagic:
		return npcStat(d, 7)
	case BonusAttackRange:
		return npcStat(d, 9)
	default:
		return npcStat(d, 5)
	}
}

// DefenceBonus reads stats[10+index], except ranged (index 4),
// which reads the standard ranged defence bonus (NPC.ts
// getDefenceBonus, default rangedType "standard"). The
// light/heavy kinds are chosen by attacker weapon data core
// does not have; always standard, documented in the formulas.
func (c npcCombatant) DefenceBonus(index int) int {
	d := c.NPCDef()
	if index == BonusAttackRange && d != nil {
		return d.RangedDefenceBonus("standard")
	}
	return npcStat(d, 10+index)
}

// OtherBonus reads melee strength (stats[6]) and ranged
// strength (stats[8]); magic damage and prayer are 0 for NPCs.
func (c npcCombatant) OtherBonus(index int) int {
	d := c.NPCDef()
	switch index {
	case OtherBonusStrength:
		return npcStat(d, 6)
	case OtherBonusRangedStrength:
		return npcStat(d, 8)
	default:
		return 0
	}
}
func (c npcCombatant) MovementQueue() *MovementQueue { return c.n.MovementQueue() }
func (c npcCombatant) Skills() *SkillManager         { return nil }
func (c npcCombatant) Equipment() *Equipment         { return nil }
func (c npcCombatant) ItemDefinitions() ItemDefinitionSource {
	return nil
}
func (c npcCombatant) SendMessage(text string)                             {}
func (c npcCombatant) SendTypedMessage(chatType int, message, from string) {}
func (c npcCombatant) IsPlayerBot() bool                                   { return false }
func (c npcCombatant) Username() string                                    { return "" }

// unknownCombatant is the fail-closed adapter for a Mobile whose
// Combat was never bound through a typed GetCombat: hitpoints 0
// (so validTarget rejects it), size 1, no queue, no skills, no
// bonuses. It exists so *Mobile-typed call sites (damage maps,
// aggression checks) never nil-panic on an unbound fighter.
type unknownCombatant struct{ m *Mobile }

func (c unknownCombatant) Mobile() *Mobile               { return c.m }
func (c unknownCombatant) IsPlayer() bool                { return false }
func (c unknownCombatant) Name() string                  { return "null" }
func (c unknownCombatant) Size() int                     { return 1 }
func (c unknownCombatant) Hitpoints() int                { return 0 }
func (c unknownCombatant) SetHitpoints(hp int)           {}
func (c unknownCombatant) MaxHitpoints() int             { return 0 }
func (c unknownCombatant) Dying() bool                   { return true }
func (c unknownCombatant) SetDying(dying bool)           {}
func (c unknownCombatant) CurrentLevel(skill int) int    { return 0 }
func (c unknownCombatant) AttackBonus(index int) int     { return 0 }
func (c unknownCombatant) DefenceBonus(index int) int    { return 0 }
func (c unknownCombatant) OtherBonus(index int) int      { return 0 }
func (c unknownCombatant) MovementQueue() *MovementQueue { return nil }
func (c unknownCombatant) Skills() *SkillManager         { return nil }
func (c unknownCombatant) Equipment() *Equipment         { return nil }
func (c unknownCombatant) ItemDefinitions() ItemDefinitionSource {
	return nil
}
func (c unknownCombatant) SendMessage(text string)                             {}
func (c unknownCombatant) SendTypedMessage(chatType int, message, from string) {}
func (c unknownCombatant) IsPlayerBot() bool                                   { return false }
func (c unknownCombatant) Username() string                                    { return "" }
func (c unknownCombatant) Player() *Player                                     { return nil }
func (c unknownCombatant) NPC() *NPC                                           { return nil }
func (c unknownCombatant) NPCDef() NPCCombatDefinition {
	return nil
}

// combatantFor resolves a Mobile to its bound Combatant, or the
// unknown adapter when the Mobile was never bound or is nil.
// The binding lives on the Mobile's own Combat (see combat.go),
// so any fighter that has ticked through a typed Process is
// bound; a nil Mobile yields an unknown adapter over nil, and
// callers must nil-check the Mobile before using it.
func combatantFor(m *Mobile) Combatant {
	if m == nil {
		return unknownCombatant{m: nil}
	}
	if c := m.GetCombat(); c != nil && c.identity != nil {
		return c.identity
	}
	return unknownCombatant{m: m}
}

// IsPlayer reports whether this Mobile is a player (Mobile.ts:967,
// isPlayer: instanceof Player). It reads the bound combatant
// identity (see combatantFor): a Mobile whose typed owner never
// bound an identity — or a nil Mobile — answers false, the
// fail-closed unknownCombatant answer. Player and NPC define
// their own IsPlayer methods, which shadow this one for typed
// receivers; this method exists for code that holds only the
// Mobile, such as content handling hub combat events (the
// SeekingArrows port's attacker check).
func (m *Mobile) IsPlayer() bool { return combatantFor(m).IsPlayer() }

// AsPlayer returns this Mobile as a *Player, or nil when it is
// not a player (Mobile.ts:981, getAsPlayer). Nil-safe: a nil
// Mobile yields nil. Like IsPlayer, it reads the bound
// combatant identity; an unbound Mobile yields nil even when a
// Player value embeds it but never bound its combat state
// through the typed GetCombat.
func (m *Mobile) AsPlayer() *Player { return combatantFor(m).Player() }

// ItemBonusSource is the optional bonus read on an
// ItemDefinitionSource (the ItemAnimationSource precedent in
// appearance.go): the 14-entry bonus array for an item id
// (definition.ItemDefinition.Bonuses: 0..4 attack, 5..9
// defence, 10 melee strength, 11 ranged strength, 12 magic
// damage percent, 13 prayer). The cmd/server adapter supplies
// it; without it every fighter reads as naked.
type ItemBonusSource interface {
	ItemBonuses(id int) []float64
}

// WeaponInfoSource is the optional weapon read on an
// ItemDefinitionSource: attack speed (cache ticks), attack
// range, and weapon interface name, for the base-attack-speed
// and ranged-distance defaults. Without it, defaults apply
// (speed 4, range 6).
type WeaponInfoSource interface {
	WeaponAttackSpeed(id int) int
	WeaponAttackRange(id int) int
	WeaponInterfaceName(id int) string
	// WeaponAttackAnim returns this weapon's own animation
	// for an attack of attackType ("stab", "slash",
	// "crush", "magic", "range"), or -1 for its type's
	// (ItemDefinition.getAttackAnim, ItemDefinition.ts:
	// 308-311). Read by GetAttackAnim's item-def layer
	// (WeaponProfiles.attackAnimation, WeaponProfile.ts:
	// 111-119).
	WeaponAttackAnim(id int, attackType string) int
	// WeaponBlockAnim returns the block animation
	// (ItemDefinition.getBlockAnim, ItemDefinition.ts:
	// 253-255, default 424). Read by GetBlockAnim
	// (Player.ts:340-346).
	WeaponBlockAnim(id int) int
}

// ---- World seams: cycle, clipping, multi-combat, wilderness ----
//
// Combat reads the world tick cycle, clipping, and area state
// through package-level func vars the coordinator wires at boot
// (World.ProcessCycle, RegionManager.GetClipping, AreaManager /
// Wilderness). The defaults keep unit tests and standalone
// fighters working: cycle from the last hit-queue process, open
// ground, and the TS fallbacks for multi-combat.

var combatCycleSource func() int64

// SetCombatCycleSource installs the world cycle source. The
// coordinator wires World.ProcessCycle at boot; until then the
// last cycle ProcessHitQueue saw is used (see combatCycle).
func SetCombatCycleSource(source func() int64) { combatCycleSource = source }

// lastHitQueueCycle is the fallback cycle: the most recent cycle
// any hit queue processed. ProcessHitQueue stamps it.
var lastHitQueueCycle int64

// combatCycle is World.getProcessCycle() (Combat.ts reads it
// throughout): the installed source, else the last hit-queue
// cycle. Pre/PostMovementProcess take no cycle argument (fixed
// stub signatures), so the cycle must come from here.
func combatCycle() int64 {
	if combatCycleSource != nil {
		return combatCycleSource()
	}
	return lastHitQueueCycle
}

var combatClippingSource func(x, y, z int) int32

// SetCombatClippingSource installs the clipping lookup. The
// coordinator wires RegionManager.GetClipping at boot; until
// then every tile reads as open ground (0), matching Go
// GetClipping for missing regions.
func SetCombatClippingSource(source func(x, y, z int) int32) {
	combatClippingSource = source
}

// combatClipping reads one tile's clipping mask through the
// installed source, or 0 (open) when none is installed.
func combatClipping(x, y, z int) int32 {
	if combatClippingSource != nil {
		return combatClippingSource(x, y, z)
	}
	return 0
}

// inMultiOverride, when set, answers multi-combat for a
// fighter (AreaManager.InMulti). The default reads the
// fighter's area IsMulti (Area.ts isMulti) — nil area reads
// single-combat — which is the TS fallback shape minus the
// wilderness multi-zone check the coordinator's override
// supplies.
var inMultiOverride func(c Combatant) bool

// SetCombatMultiSource installs the multi-combat source.
func SetCombatMultiSource(source func(c Combatant) bool) {
	inMultiOverride = source
}

// inMultiCombat answers whether fighter is in multi-combat:
// the fighter's area multi flag (or the installed override,
// which additionally answers the wilderness multi-zone
// fallback). TS AreaManager.inMulti is area-based for every
// fighter; the always-multi-NPC rule is NOT here — it lives
// in MultiCombatBetween / AreaAttackSpreads at the pair
// level (CombatFactory.multiCombatBetween).
func inMultiCombat(c Combatant) bool {
	if c == nil || c.Mobile() == nil {
		return false
	}
	if inMultiOverride != nil {
		return inMultiOverride(c)
	}
	if area := c.Mobile().Area(); area != nil {
		return area.IsMulti()
	}
	return false
}

// wildernessOverride, when set, answers wilderness membership
// and safe-building status for a location (Wilderness.ts).
// Defaults: nowhere is wilderness, no safe buildings.
var wildernessOverride func(loc Location) (inWilderness, safeBuilding bool)

// SetCombatWildernessSource installs the wilderness source.
func SetCombatWildernessSource(source func(loc Location) (bool, bool)) {
	wildernessOverride = source
}

func combatInWilderness(loc Location) bool {
	if wildernessOverride != nil {
		in, _ := wildernessOverride(loc)
		return in
	}
	return false
}

func combatInSafeBuilding(loc Location) bool {
	if wildernessOverride != nil {
		_, safe := wildernessOverride(loc)
		return safe
	}
	return false
}

// ---- Combat flags ----
//
// TS Entity carries a string-flag set (Mobile.ts flags); Go has
// no such set. The combat-only flags TS reads in the core
// pipeline live in this registry, keyed by *Mobile, set by
// content/hub code through SetCombatFlag. Only the flags the
// ported core reads are defined here; content-defined flags
// (equipment effects, boss mechanics) are out of scope.
var combatFlags = map[*Mobile]map[string]bool{}

// Combat flag keys (TS string literals, Combat.ts /
// CombatFactory.ts / CombatRange.ts).
const (
	// CombatFlagKeepFacing: the NPC keeps facing its target
	// instead of turning (Combat.ts face handling).
	CombatFlagKeepFacing = "combat:keep-facing"
	// CombatFlagNoRetaliate: the fighter never retaliates
	// (CombatFactory.handleRetaliation).
	CombatFlagNoRetaliate = "combat:no-retaliate"
	// CombatFlagCloseIn: the NPC keeps its movement queue when
	// in reach instead of resetting it (Combat.ts preMovement).
	CombatFlagCloseIn = "combat:close-in"
	// CombatFlagKeepTarget: the NPC never gives up an
	// unreachable target (Combat.ts isSafespotted).
	CombatFlagKeepTarget = "combat:keep-target"
	// CombatFlagBuiltIntoScenery: the NPC is part of the
	// scenery; range checks use its footprint specially
	// (CombatRange.ts).
	CombatFlagBuiltIntoScenery = "combat:built-into-scenery"
	// CombatFlagNoRetreat: the NPC never retreats
	// (Combat.ts shouldRetreat).
	CombatFlagNoRetreat = "combat:no-retreat"
	// CombatFlagFreezable: a large NPC may be frozen despite
	// its size (CombatFactory.freezeTicks).
	CombatFlagFreezable = "combat:freezable"
	// NOTE: "combat:recoil-immune" is deliberately NOT a flag
	// here: TS reads it as an ATTRIBUTE (getAttribute(...) ===
	// true) in CombatFactory.canReceiveRecoil. See
	// RecoilImmuneAttribute in combat_factory.go.
)

// SetCombatFlag sets or clears a combat flag on m. A nil Mobile
// is a no-op. Tick-goroutine-only, like the rest of combat
// state; no synchronization.
func SetCombatFlag(m *Mobile, flag string, set bool) {
	if m == nil {
		return
	}
	if set {
		flags := combatFlags[m]
		if flags == nil {
			flags = map[string]bool{}
			combatFlags[m] = flags
		}
		flags[flag] = true
		return
	}
	if flags := combatFlags[m]; flags != nil {
		delete(flags, flag)
		if len(flags) == 0 {
			delete(combatFlags, m)
		}
	}
}

// HasCombatFlag reports whether m carries a combat flag.
func HasCombatFlag(m *Mobile, flag string) bool {
	if m == nil {
		return false
	}
	return combatFlags[m][flag]
}

// ---- Randomness ----
//
// TS rolls through Misc.randomInclusive / Math.random. The func
// vars let tests install deterministic sequences; the defaults
// use math/rand/v2 (the transcript.go precedent).

// randomInclusive rolls a uniform integer in [0, max] inclusive
// (Misc.randomInclusive). A max below 0 answers 0.
var randomInclusive = func(max int) int {
	if max <= 0 {
		return 0
	}
	return rand.IntN(max + 1)
}

// randomFloat rolls a uniform float in [0, 1) (Math.random).
var randomFloat = func() float64 { return rand.Float64() }

// scaleRatio is DamageFormulas.scaleRatio: floor(value * num /
// den) with integer arithmetic, the shared integer-scaling
// helper for every formula term that scales a level or bonus.
func scaleRatio(value, num, den int) int {
	if den == 0 {
		return 0
	}
	return value * num / den
}

// scalePercent is DamageFormulas.scalePercent: floor(value *
// percent / 100).
func scalePercent(value, percent int) int {
	return value * percent / 100
}

// scaleSpecial is DamageFormulas.scaleSpecial: value scaled by a
// decimal multiplier (1.3 = +30%), converted once to permille
// with rounding and kept integral thereafter:
// floor(value * round(multiplier * 1000) / 1000).
func scaleSpecial(value int, multiplier float64) int {
	return scaleRatio(value, int(math.Round(multiplier*1000)), 1000)
}
