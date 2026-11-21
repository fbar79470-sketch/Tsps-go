package game

import (
	"math"

	"tsps-go/internal/definition"
)

// CombatFactory: the port of game/content/combat/CombatFactory.ts
// (2000 lines) — the static combat pipeline: method resolution,
// damage rolls, defence application, permission checks, hit
// queueing and execution, XP, retaliation, recoil/vengeance,
// skulls, stun/freeze, and ammo. In Go the "statics" are
// package-level functions.
//
// NOT PORTED (documented gaps, each with its reason):
//   - Barrows full-set checks (fullVeracs etc.): equipment-set
//     content; no set system exists.
//   - Duel branches: no duelling system; the duel response
//     values exist but no core path returns them.
//   - Poison/venom application: ported (poison.go:
//     PoisonClock, CombatPoisonEffect, CombatPoisonData,
//     poisonEntity, and the executeHit call sites). Cure
//     items (antipoison etc.) are content, not ported.
//   - Ward of Arceuus, corruption, demonbane: content spell
//     systems.
//   - NPC roll factors, per-NPC combat-XP multiplier, defence
//     overrides, max-level boosts: per-NPC mutable state with
//     no Go fields; definition values read instead.
//   - Skull application: live since phase 28 (skull.go);
//     the attacked-by record runs in core and application
//     goes through the Hooks seam, wired to the live system
//     in cmd/server.
//   - Crystal bow / ether bows / plugin ammo: item-specific
//     content; the plugin hooks cover them.
//   - Sounds, graphics, animations, interface closes, effect
//     timers: the presenting systems are NOT STARTED.

// Combat constants (CombatConstants.ts).
const (
	// DamageCacheTimeoutTicks is DAMAGE_CACHE_TIMEOUT
	// (60000ms): a damager counts toward the kill for 100
	// ticks after their last damage.
	DamageCacheTimeoutTicks = 100
	// PrayerDamageReductionAgainstPlayers is
	// PRAYER_DAMAGE_REDUCTION_AGAINST_PLAYERS (0.6): a
	// protection prayer cuts player-dealt damage by 40%.
	PrayerDamageReductionAgainstPlayers = 0.6
	// PrayerDamageReductionAgainstNPCs is
	// PRAYER_DAMAGE_REDUCTION_AGAINST_NPCS (0): a protection
	// prayer nullifies NPC-dealt damage.
	PrayerDamageReductionAgainstNPCs = 0.0
	// ElysianDamageReduction is ELYSIAN_DAMAGE_REDUCTION
	// (0.75): the Elysian spirit shield cuts damage by 25%.
	ElysianDamageReduction = 0.75
	// ElysianActivationChance is ELYSIAN_ACTIVATION_CHANCE
	// (0.7).
	ElysianActivationChance = 0.7
	// PrayerAccuracyReductionAgainstPlayers is
	// PRAYER_ACCURACY_REDUCTION_AGAINST_PLAYERS (0.3). It is
	// carried for grounding; no core path applies it (the
	// accuracy-prayer interaction lives in content hooks).
	PrayerAccuracyReductionAgainstPlayers = 0.3
	// PrayerAccuracyReductionAgainstNPCs is
	// PRAYER_ACCURACY_REDUCTION_AGAINST_NPCS (0.2). See above.
	PrayerAccuracyReductionAgainstNPCs = 0.2
	// MaxAttackDistance is MAX_ATTACK_DISTANCE (10): the cap
	// on ranged attack distance.
	MaxAttackDistance = 10
)

// ElysianSpiritShieldID is ItemIdentifiers.ELYSIAN_SPIRIT_SHIELD.
const ElysianSpiritShieldID = 12817

// RingOfRecoilID is ItemIdentifiers.RING_OF_RECOIL.
const RingOfRecoilID = 2550

// RecoilRingCharges is RECOIL_RING_CHARGES (40).
const RecoilRingCharges = 40

// RecoilDamageAttribute is RECOIL_DAMAGE_ATTRIBUTE
// ("ring-of-recoil:damage"): expended recoil charges.
const RecoilDamageAttribute = "ring-of-recoil:damage"

// RecoilImmuneAttribute is RECOIL_IMMUNE_ATTRIBUTE
// ("combat:recoil-immune"): recoil/vengeance immunity.
const RecoilImmuneAttribute = "combat:recoil-immune"

// AttackedByAttribute is ATTACKED_BY_ATTRIBUTE
// ("combat:pvp-attacked-by"): the username -> cycle record.
const AttackedByAttribute = "combat:pvp-attacked-by"

// AttackedByTicks is ATTACKED_BY_TICKS: 100 ticks (the damage
// cache timeout).
const AttackedByTicks = 100

// PVPSkullSeconds is PVP_SKULL_SECONDS (30 minutes).
const PVPSkullSeconds = 30 * 60

// ItemSkullSeconds is ITEM_SKULL_SECONDS (20 minutes).
const ItemSkullSeconds = 20 * 60

// AmmoBreakChance is AMMO_BREAK_CHANCE (20): the percent of
// fired ammo that breaks on impact.
const AmmoBreakChance = 20

// hitRollDepth is CombatFactory.hitRollDepth: non-zero while a
// PendingHit is rolling, so its defences defer to impact.
var hitRollDepth int

// DuringHitRoll runs action with defences deferred to the
// impact tick (CombatFactory.duringHitRoll).
func DuringHitRoll[T any](action func() T) T {
	hitRollDepth++
	defer func() { hitRollDepth-- }()
	return action()
}

// ---- Method resolution ----

// GetMethod is CombatFactory.getMethod: the combat method for
// an attack. Players: refresh ranged data, then a cast spell
// owns the action (MAGIC), then plugin resolvers, then an
// activated special's method, then autocast (MAGIC), then a
// ranged weapon (RANGED), else MELEE. NPCs: providers in
// registration order (matching id), then the definition
// attack type.
func GetMethod(attacker *Mobile) CombatMethod {
	attackerCombatant := combatantFor(attacker)
	if attackerCombatant.IsPlayer() {
		player := attackerCombatant.Player()
		resolveRangedData(player)
		combat := attacker.GetCombat()
		if combat.CastSpell() != nil {
			return MagicMethod
		}
		if resolved := resolveCombatMethod(player); resolved != nil {
			return resolved
		}
		if combat.IsSpecialActivated() {
			if special := combat.GetCombatSpecial(); special != nil && special.Method != nil {
				return special.Method
			}
		}
		if combat.AutoCastSpell() != nil {
			return MagicMethod
		}
		if combat.RangedWeapon() != nil {
			return RangedMethod
		}
		return MeleeMethod
	}
	if npc := attackerCombatant.NPC(); npc != nil {
		if method := npcCombatMethodFromProviders(npc); method != nil {
			return method
		}
		if d := attackerCombatant.NPCDef(); d != nil {
			switch d.AttackType() {
			case definition.CombatTypeRanged:
				return RangedMethod
			case definition.CombatTypeMagic:
				return MagicMethod
			}
		}
		return MeleeMethod
	}
	return MeleeMethod
}

// ---- Damage rolls ----

// GetHitDamage is CombatFactory.getHitDamage: rolls a hit and,
// unless defences are deferred, applies the victim's
// defensive reactions immediately. applyDefences nil means
// "defer while inside DuringHitRoll"; false defers; true
// applies. A PendingHit passes false and reads the reactions
// at impact.
func GetHitDamage(attacker, victim *Mobile, t CombatType, bypassProtectionPrayer bool, boundsOverride *SpecialDamageBounds, applySpecialTraits bool, applyDefences *bool) *HitDamage {
	deferDefences := false
	if applyDefences == nil {
		deferDefences = hitRollDepth > 0
	} else {
		deferDefences = !*applyDefences
	}
	rolled := RollHitDamage(attacker, victim, t, bypassProtectionPrayer, boundsOverride, applySpecialTraits, !deferDefences)
	if deferDefences {
		return rolled
	}
	return ApplyIncomingHitDamage(attacker, victim, t, rolled)
}

// RollHitDamage is CombatFactory.rollHitDamage: damage only.
// Protection prayers are included when applyProtectionPrayer
// is true; a PendingHit passes false and reads the prayer
// again when the hit lands.
func RollHitDamage(attacker, victim *Mobile, t CombatType, bypassProtectionPrayer bool, boundsOverride *SpecialDamageBounds, applySpecialTraits bool, applyProtectionPrayer bool) *HitDamage {
	attackerCombatant := combatantFor(attacker)
	var specialTraits *WeaponSpecialTraits
	if applySpecialTraits {
		specialTraits = ActiveTraitsFor(attackerCombatant)
	}
	if specialTraits != nil && specialTraits.IgnoreProtectionPrayer {
		bypassProtectionPrayer = true
	}
	prayerType := t
	if specialTraits != nil {
		prayerType = specialTraits.ResolveDamageType(t)
	}
	maxHit := 0
	minimumFloor := 0
	switch t {
	case CombatTypeMelee:
		maxHit = SourceMaxHit(attackerCombatant, CombatTypeMelee)
	case CombatTypeRanged:
		maxHit = SourceMaxHit(attackerCombatant, CombatTypeRanged)
		if Hooks.ModifyRangedMaxHit != nil {
			if modified := Hooks.ModifyRangedMaxHit(attacker, victim, maxHit); modified != nil {
				maxHit = *modified
			}
		}
	case CombatTypeMagic:
		minimumFloor, maxHit = SourceHitBounds(attackerCombatant, CombatTypeMagic, combatantFor(victim))
	}
	minimum, maximum := SpecialDamageRange(attackerCombatant, maxHit, boundsOverride, applySpecialTraits, minimumFloor)
	damage := randomInclusiveRange(minimum, maximum)
	if Hooks.OnCombatHitRolled != nil {
		e := &CombatHitRolledEvent{
			Attacker:   attacker,
			Target:     victim,
			CombatType: t,
			Damage:     damage,
			Minimum:    minimum,
			Maximum:    maximum,
		}
		Hooks.OnCombatHitRolled(e)
		if e.Damage != damage {
			damage = e.Damage
			if damage < 0 {
				damage = 0
			}
		}
	}
	// OSRS Project Rebalance (29 May 2024): successful player
	// zeroes become ones before defences.
	if attackerCombatant.IsPlayer() && damage == 0 && maximum > 0 {
		damage = 1
	}
	// Ward of Arceuus: skipped (content spell system).
	mask := HitMaskRed
	if damage == 0 {
		mask = HitMaskBlue
	}
	hitDamage := NewHitDamage(damage, mask)
	if attackerCombatant.IsPlayer() {
		hitDamage.SetRolledMaximum(maximum)
	}
	if applyProtectionPrayer && !bypassProtectionPrayer {
		if PrayerActive(victim, ProtectingPrayer(prayerType)) {
			if attackerCombatant.IsPlayer() {
				hitDamage.MultiplyDamage(PrayerDamageReductionAgainstPlayers)
			} else {
				hitDamage.MultiplyDamage(PrayerDamageReductionAgainstNPCs)
			}
		}
	}
	return hitDamage
}

// ApplyIncomingHitDamage is CombatFactory.applyIncomingHitDamage:
// the Elysian spirit shield (70% chance, x0.75, shield slot,
// id 12817; the graphic is skipped) plus the plugin
// incoming-damage modifiers (players only, damage > 0).
func ApplyIncomingHitDamage(attacker, victim *Mobile, t CombatType, hitDamage *HitDamage) *HitDamage {
	victimCombatant := combatantFor(victim)
	if victimCombatant.IsPlayer() && randomFloat() < ElysianActivationChance {
		if shield := victimCombatant.Equipment().ItemAt(ShieldSlot); shield.IsValid() && shield.ID() == ElysianSpiritShieldID {
			hitDamage.MultiplyDamage(ElysianDamageReduction)
			// Graphic 321: skipped (no graphic system).
		}
	}
	if victimCombatant.IsPlayer() && hitDamage.Damage() > 0 && Hooks.ModifyIncomingDamage != nil {
		if modified := Hooks.ModifyIncomingDamage(victim, hitDamage, hitDamage.Damage()); modified != nil {
			damage := *modified
			if damage < 0 {
				damage = 0
			}
			hitDamage.SetDamage(damage)
		}
	}
	return hitDamage
}

// ApplyProtectionPrayer is CombatFactory.applyProtectionPrayer:
// reads the victim's protection prayer for the hit's
// protection type and scales every splat (NPC attackers x0,
// player attackers x0.6). Idempotent via the impact flag.
func ApplyProtectionPrayer(hit *PendingHit) {
	if hit == nil || hit.HasAppliedImpactProtection() {
		return
	}
	hits := hit.Hits()
	attacker := hit.Attacker()
	victim := hit.Target()
	if len(hits) == 0 || attacker == nil || victim == nil {
		return
	}
	if !hit.BypassesProtectionPrayer() {
		if PrayerActive(victim, ProtectingPrayer(hit.ProtectionType())) {
			reduction := PrayerDamageReductionAgainstPlayers
			if !combatantFor(attacker).IsPlayer() {
				reduction = PrayerDamageReductionAgainstNPCs
			}
			for _, damage := range hits {
				damage.MultiplyDamage(reduction)
			}
		}
	}
	hit.MarkImpactProtectionApplied()
	hit.UpdateTotalDamage()
}

// ApplyHitTimeDefences is CombatFactory.applyHitTimeDefences:
// protection plus Elysian/equipment at impact, unless the
// incoming half was already applied or is deferred.
func ApplyHitTimeDefences(hit *PendingHit) {
	if hit == nil || len(hit.Hits()) == 0 {
		return
	}
	if hit.Attacker() == nil || hit.Target() == nil {
		return
	}
	ApplyProtectionPrayer(hit)
	if !hit.HasAppliedImpactIncoming() && !hit.DefersIncomingDamage() {
		applyImpactIncomingDamage(hit)
		hit.MarkImpactIncomingApplied()
	}
	hit.UpdateTotalDamage()
}

// applyImpactIncomingDamage is
// CombatFactory.applyImpactIncomingDamage: the incoming
// reactions over every positive splat.
func applyImpactIncomingDamage(hit *PendingHit) {
	for _, damage := range hit.Hits() {
		if damage.Damage() <= 0 {
			continue
		}
		ApplyIncomingHitDamage(hit.Attacker(), hit.Target(), hit.CombatType(), damage)
	}
}

// ApplyDeferredImpactIncoming is
// CombatFactory.applyDeferredImpactIncoming: a scripted
// producer filled the hit in manipulateHit after the
// protection pass; resolve the deferred equipment reactions
// now.
func ApplyDeferredImpactIncoming(hit *PendingHit) {
	if hit == nil || len(hit.Hits()) == 0 {
		return
	}
	if !hit.DefersIncomingDamage() || hit.HasAppliedImpactIncoming() {
		return
	}
	if hit.Attacker() == nil || hit.Target() == nil {
		return
	}
	applyImpactIncomingDamage(hit)
	hit.MarkImpactIncomingApplied()
	hit.UpdateTotalDamage()
}

// ---- Target validity and permission ----

// ValidTarget is CombatFactory.validTarget: null/self,
// registration, hitpoints, untargetable, needs-placement,
// teleporting (always false in Go: no teleport system),
// dying, same floor (no CrossFloor), within 40 tiles, and
// the owner-only NPC rules (with the player message).
func ValidTarget(attacker, target *Mobile) bool {
	if attacker == nil || target == nil || attacker == target {
		return false
	}
	attackerCombatant := combatantFor(attacker)
	targetCombatant := combatantFor(target)
	if !target.IsRegistered() || !attacker.IsRegistered() ||
		attackerCombatant.Hitpoints() <= 0 || targetCombatant.Hitpoints() <= 0 ||
		isUntargetable(attacker) || isUntargetable(target) ||
		attacker.NeedsPlacement || target.NeedsPlacement ||
		attackerCombatant.Dying() || targetCombatant.Dying() {
		return false
	}
	attackerLoc := attacker.Location()
	targetLoc := target.Location()
	if attackerLoc.Z != targetLoc.Z {
		return false
	}
	if attackerLoc.Distance(targetLoc) >= 40 {
		return false
	}
	if npc := attackerCombatant.NPC(); npc != nil && targetCombatant.IsPlayer() {
		if owner := npc.Owner(); owner != nil && owner != targetCombatant.Player() {
			return false
		}
	} else if npc := targetCombatant.NPC(); npc != nil && attackerCombatant.IsPlayer() {
		if owner := npc.Owner(); owner != nil && owner != attackerCombatant.Player() {
			attackerCombatant.SendMessage("This npc was not spawned for you.")
			return false
		}
	}
	return true
}

// CanAttackResponse is CanAttackResponse
// (CombatFactory.ts:1970-1987), in TS declaration order.
type CanAttackResponse int

const (
	CanAttackInvalidTarget CanAttackResponse = iota
	CanAttackAlreadyUnderAttack
	CanAttackTargetInCombat
	CanAttackCantAttackInArea
	CanAttackMethodNotAllowed
	CanAttackLevelDifferenceTooGreat
	CanAttackNotEnoughSpecialEnergy
	CanAttackStunned
	CanAttackDuelNotStartedYet
	CanAttackDuelMeleeDisabled
	CanAttackDuelRangedDisabled
	CanAttackDuelMagicDisabled
	CanAttackDuelWrongOpponent
	CanAttackTargetIsImmune
	CanAttackOK
)

// String names the response for debugging.
func (r CanAttackResponse) String() string {
	switch r {
	case CanAttackInvalidTarget:
		return "INVALID_TARGET"
	case CanAttackAlreadyUnderAttack:
		return "ALREADY_UNDER_ATTACK"
	case CanAttackTargetInCombat:
		return "TARGET_IN_COMBAT"
	case CanAttackCantAttackInArea:
		return "CANT_ATTACK_IN_AREA"
	case CanAttackMethodNotAllowed:
		return "COMBAT_METHOD_NOT_ALLOWED"
	case CanAttackLevelDifferenceTooGreat:
		return "LEVEL_DIFFERENCE_TOO_GREAT"
	case CanAttackNotEnoughSpecialEnergy:
		return "NOT_ENOUGH_SPECIAL_ENERGY"
	case CanAttackStunned:
		return "STUNNED"
	case CanAttackDuelNotStartedYet:
		return "DUEL_NOT_STARTED_YET"
	case CanAttackDuelMeleeDisabled:
		return "DUEL_MELEE_DISABLED"
	case CanAttackDuelRangedDisabled:
		return "DUEL_RANGED_DISABLED"
	case CanAttackDuelMagicDisabled:
		return "DUEL_MAGIC_DISABLED"
	case CanAttackDuelWrongOpponent:
		return "DUEL_WRONG_OPPONENT"
	case CanAttackTargetIsImmune:
		return "TARGET_IS_IMMUNE"
	case CanAttackOK:
		return "CAN_ATTACK"
	default:
		return "UNKNOWN"
	}
}

// CanAttack is CombatFactory.canAttack: the permission check
// (unless skipPermissionValidation) plus the method's own
// (consuming) check. executeAttack calls it with both skips
// set: permission was already checked in the movement
// phases, so only the method check (ammo, runes) runs there.
func CanAttack(attacker, target *Mobile, method CombatMethod, skipTargetValidation, skipPermissionValidation bool) CanAttackResponse {
	if !skipPermissionValidation {
		if response := CanAttackPermission(attacker, target, skipTargetValidation, method); response != CanAttackOK {
			return response
		}
	}
	if method != nil && !method.CanAttack(attacker, target) {
		return CanAttackMethodNotAllowed
	}
	return CanAttackOK
}

// duelStateOf returns the Dueling for a player mobile, or nil
// for a non-player (attacker.isPlayer() ?
// attacker.getAsPlayer().getDueling() : null,
// CombatFactory.ts:718-719). Resolution goes through
// combatantFor, like the rest of this file: an unbound
// combat identity reads as a non-player here, as it does for
// the attacker-player block below.
func duelStateOf(m *Mobile) *Dueling {
	if p := combatantFor(m).Player(); p != nil {
		return p.Dueling()
	}
	return nil
}

// CanAttackPermission is CombatFactory.canAttackPermission:
// the read-only permission check, in TS order: target
// validation, duel checks, the single-combat busy checks,
// the policy step — in a duel the onCanAttack gate alone
// (a veto denies, anything else passes; the area/PvP policy
// never runs), outside a duel the area/plugin policy —
// the method's pursue check, special energy, stun, duel
// rules, and target attack immunity.
func CanAttackPermission(attacker, target *Mobile, skipTargetValidation bool, method CombatMethod) CanAttackResponse {
	if !skipTargetValidation && !ValidTarget(attacker, target) {
		return CanAttackInvalidTarget
	}
	// Duel checks (CombatFactory.ts:718-731): when either
	// side is in a duel, the opponent must be the agreed one
	// and both sides must be IN_DUEL.
	attackerDuel := duelStateOf(attacker)
	targetDuel := duelStateOf(target)
	duelActive := (attackerDuel != nil && attackerDuel.InDuel()) || (targetDuel != nil && targetDuel.InDuel())
	if duelActive {
		if attackerDuel == nil || targetDuel == nil ||
			attackerDuel.Interact() != combatantFor(target).Player() ||
			targetDuel.Interact() != combatantFor(attacker).Player() {
			return CanAttackDuelWrongOpponent
		}
		if attackerDuel.State() != DuelStateInDuel || targetDuel.State() != DuelStateInDuel {
			return CanAttackDuelNotStartedYet
		}
	}
	if !MultiCombatBetween(attacker, target) {
		npcBrawl := func(mobile, by *Mobile) bool {
			if by == nil {
				return false
			}
			byCombatant := combatantFor(by)
			return !byCombatant.IsPlayer() && byCombatant.NPC() != nil && byCombatant.NPC().Owner() == nil &&
				!combatantFor(mobile).IsPlayer() &&
				(combatantFor(attacker).IsPlayer() || combatantFor(target).IsPlayer())
		}
		lockedBy := func(mobile *Mobile) bool {
			by := mobile.GetCombat().Attacker()
			if by == nil {
				return false
			}
			byCombatant := combatantFor(by)
			if !byCombatant.IsPlayer() && byCombatant.NPC() != nil && byCombatant.NPC().IsMultiCombat() {
				return false
			}
			return !npcBrawl(mobile, by)
		}
		hitQueueFree := func(mobile, except *Mobile) bool {
			queue := mobile.GetCombat().HitQueue()
			if !combatantFor(mobile).IsPlayer() && combatantFor(except).IsPlayer() {
				return queue.IsEmptyButFreeNPCs(except)
			}
			return queue.IsEmpty(except)
		}
		attackerCombat := attacker.GetCombat()
		if (IsBeingAttacked(attacker) && attackerCombat.Attacker() != target &&
			combatantFor(attackerCombat.Attacker()).Hitpoints() > 0 && lockedBy(attacker)) ||
			!attackerCombat.HitQueue().IsEmpty(target) {
			return CanAttackAlreadyUnderAttack
		}
		targetCombat := target.GetCombat()
		if (IsBeingAttacked(target) && targetCombat.Attacker() != attacker && lockedBy(target)) ||
			!hitQueueFree(target, attacker) {
			return CanAttackTargetInCombat
		}
	}
	// Policy (CombatFactory.ts:779-789): in a duel the
	// onCanAttack gate decides alone — a veto denies, and
	// anything else (an allow, or no opinion) passes, so
	// agreed opponents can fight with no plugin installed.
	// Outside a duel the full area/plugin policy runs.
	if duelActive {
		if allow := emitCanAttack(attacker, target, method); allow != nil && !*allow {
			return CanAttackCantAttackInArea
		}
	} else if response := CanAttackByPolicy(attacker, target, method); response != CanAttackOK {
		return response
	}
	if method != nil && !method.CanPursue(attacker, target) {
		return CanAttackMethodNotAllowed
	}
	if attackerCombatant := combatantFor(attacker); attackerCombatant.IsPlayer() {
		player := attackerCombatant.Player()
		combat := attacker.GetCombat()
		if special := combat.GetCombatSpecial(); combat.IsSpecialActivated() && special != nil {
			traits := ActiveTraitsFor(attackerCombatant)
			queuedInFlight := traits != nil && traits.QueuedAttack && combat.IsSpecialAttackQueued()
			if !queuedInFlight {
				weaponID := -1
				if weapon := attackerCombatant.Equipment().GetWeapon(); weapon.IsValid() {
					weaponID = weapon.ID()
				}
				if combat.SpecialPercentage() < special.Cost(player, weaponID) {
					return CanAttackNotEnoughSpecialEnergy
				}
			}
		}
		if attacker.GetTimers().Has(TimerKeyStun) {
			return CanAttackStunned
		}
		// Duel rules (CombatFactory.ts:814-823): in a duel,
		// a method whose combat type the rules disable is
		// refused.
		if method != nil && player.Dueling().InDuel() {
			dueling := player.Dueling()
			if method.Type() == CombatTypeMelee && dueling.RuleSet(DuelRuleNoMelee) {
				return CanAttackDuelMeleeDisabled
			}
			if method.Type() == CombatTypeRanged && dueling.RuleSet(DuelRuleNoRanged) {
				return CanAttackDuelRangedDisabled
			}
			if method.Type() == CombatTypeMagic && dueling.RuleSet(DuelRuleNoMagic) {
				return CanAttackDuelMagicDisabled
			}
		}
	}
	if !combatantFor(target).IsPlayer() && target.GetTimers().Has(TimerKeyAttackImmunity) {
		return CanAttackTargetIsImmune
	}
	return CanAttackOK
}

// emitCanAttack consults the onCanAttack gate
// (PluginManager.emitCanAttack, PluginManager.ts:1360-1377):
// the method-carrying OnCanAttack seam, the only form TS has
// (there is no method-less emit). Nil means no opinion (TS
// null). The allow-veto contract itself (a deny
// short-circuits, an allow sticks, silence is undecided) lives
// in the hub emit the coordinator wires onto OnCanAttack
// (plugins.EmitCanAttack); this helper only routes to the seam.
func emitCanAttack(attacker, target *Mobile, method CombatMethod) *bool {
	if Hooks.OnCanAttack != nil {
		return Hooks.OnCanAttack(attacker, target, method)
	}
	return nil
}

// CanAttackByPolicy is CombatFactory.canAttackByPolicy: the
// area/plugin attack policy. Private-area equality always
// holds (no private areas); deck rules are skipped (no
// sailing); wilderness safe buildings deny PvP; each side's
// area canAttack hook may deny; the global hook may deny;
// with no opinion, PvP is denied and PvE allowed.
func CanAttackByPolicy(attacker, target *Mobile, method CombatMethod) CanAttackResponse {
	attackerCombatant := combatantFor(attacker)
	targetCombatant := combatantFor(target)
	if attackerCombatant.IsPlayer() && targetCombatant.IsPlayer() &&
		(combatInSafeBuilding(attacker.Location()) || combatInSafeBuilding(target.Location())) {
		return CanAttackCantAttackInArea
	}
	var attackerAreaCanAttack, targetAreaCanAttack *bool
	attackerActor, _ := areaActorFor(attackerCombatant)
	targetActor, _ := areaActorFor(targetCombatant)
	if area := attacker.Area(); area != nil && attackerActor != nil && targetActor != nil {
		attackerAreaCanAttack = area.CanAttack(attackerActor, targetActor, method)
		if attackerAreaCanAttack != nil && !*attackerAreaCanAttack {
			return CanAttackCantAttackInArea
		}
	}
	if area := target.Area(); area != nil && area != attacker.Area() && attackerActor != nil && targetActor != nil {
		targetAreaCanAttack = area.CanAttack(attackerActor, targetActor, method)
		if targetAreaCanAttack != nil && !*targetAreaCanAttack {
			return CanAttackCantAttackInArea
		}
	}
	areaCanAttack := attackerAreaCanAttack
	if areaCanAttack == nil {
		areaCanAttack = targetAreaCanAttack
	}
	var globalCanAttack *bool
	globalCanAttack = emitCanAttack(attacker, target, method)
	if globalCanAttack != nil && !*globalCanAttack {
		return CanAttackCantAttackInArea
	}
	pluginCanAttack := areaCanAttack
	if pluginCanAttack == nil {
		pluginCanAttack = globalCanAttack
	}
	if pluginCanAttack != nil {
		if *pluginCanAttack {
			return CanAttackOK
		}
		return CanAttackCantAttackInArea
	}
	if attackerCombatant.IsPlayer() && targetCombatant.IsPlayer() {
		return CanAttackCantAttackInArea
	}
	return CanAttackOK
}

// areaActorFor resolves a combatant to the AreaActor the area
// hooks take, or nil for an unbound fighter.
func areaActorFor(c Combatant) (AreaActor, bool) {
	if c == nil {
		return nil, false
	}
	if p := c.Player(); p != nil {
		return p, true
	}
	if n := c.NPC(); n != nil {
		return n, true
	}
	return nil, false
}

// CanAttackSecondaryTarget is
// CombatFactory.canAttackSecondaryTarget: the permission
// check for area-attack secondary targets (no resource
// consumption). spellRadius < 0 means the caller already
// shaped the area.
func CanAttackSecondaryTarget(attacker, primaryTarget, candidate *Mobile, spellRadius int) bool {
	if candidate == nil || candidate == attacker || candidate == primaryTarget {
		return false
	}
	if !ValidTarget(attacker, candidate) {
		return false
	}
	// Duel restriction (CombatFactory.ts:894-897): duel
	// damage is restricted to the agreed opponent, including
	// spell splashes — an attacker or a candidate in a duel
	// excludes the candidate.
	if (duelStateOf(attacker) != nil && duelStateOf(attacker).InDuel()) ||
		(duelStateOf(candidate) != nil && duelStateOf(candidate).InDuel()) {
		return false
	}
	if combatantFor(candidate).Hitpoints() <= 0 {
		return false
	}
	if spellRadius >= 0 && !candidate.Location().IsWithinDistance(primaryTarget.Location(), spellRadius) {
		return false
	}
	if !inMultiCombat(combatantFor(candidate)) {
		return false
	}
	if npc := combatantFor(candidate).NPC(); npc != nil {
		if d := npc.Definition(); d != nil {
			if attackable, ok := d.(interface{ IsAttackable() bool }); ok && !attackable.IsAttackable() {
				return false
			}
		}
		if candidate.GetTimers().Has(TimerKeyAttackImmunity) {
			return false
		}
	}
	return CanAttackByPolicy(attacker, candidate, nil) == CanAttackOK
}

// MultiCombatBetween is CombatFactory.multiCombatBetween: an
// always-multi NPC on either side, or both fighters in multi.
func MultiCombatBetween(attacker, target *Mobile) bool {
	alwaysMulti := func(m *Mobile) bool {
		c := combatantFor(m)
		return !c.IsPlayer() && c.NPC() != nil && c.NPC().IsMultiCombat()
	}
	return alwaysMulti(attacker) || alwaysMulti(target) ||
		(inMultiCombat(combatantFor(attacker)) && inMultiCombat(combatantFor(target)))
}

// AreaAttackSpreads is CombatFactory.areaAttackSpreads: the
// target's zone decides whether an area attack spreads (an
// always-multi NPC on either side counts as multi).
func AreaAttackSpreads(attacker, target *Mobile) bool {
	alwaysMulti := func(m *Mobile) bool {
		c := combatantFor(m)
		return !c.IsPlayer() && c.NPC() != nil && c.NPC().IsMultiCombat()
	}
	return alwaysMulti(attacker) || alwaysMulti(target) || inMultiCombat(combatantFor(target))
}

// IsBeingAttacked is CombatFactory.isBeingAttacked: the
// fighter has an attacker.
func IsBeingAttacked(character *Mobile) bool {
	return character != nil && character.GetCombat().Attacker() != nil
}

// IsAttacking is CombatFactory.isAttacking: the fighter has a
// target.
func IsAttacking(character *Mobile) bool {
	return character != nil && character.GetCombat().Target() != nil
}

// InCombat is CombatFactory.inCombat: attacking or attacked.
func InCombat(character *Mobile) bool {
	return IsAttacking(character) || IsBeingAttacked(character)
}

// ---- Hit queueing and execution ----

// AddPendingHit is CombatFactory.addPendingHit: queue a
// rolled hit on the target. Dead, untargetable, or
// needs-placement targets drop the hit. The protection
// prayer is read now unless the victim's turn is still to
// come (NPC -> player); the damage budget caps; player
// attackers earn XP and handle skulls; the hit lands at
// cycle + delay + the hit-processing delay.
func AddPendingHit(qHit *PendingHit) {
	if qHit == nil {
		return
	}
	attacker := qHit.Attacker()
	target := qHit.Target()
	if target == nil || combatantFor(target).Hitpoints() <= 0 {
		return
	}
	if isUntargetable(target) || target.NeedsPlacement {
		return
	}
	// emitPlayerDealtDamage: skipped (no hook field; the
	// hub's player-dealt-damage event is content-facing).
	if !ReadsProtectionInVictimTurn(qHit) {
		ApplyProtectionPrayer(qHit)
	}
	qHit.CapDamageToBudget()
	if attacker != nil && combatantFor(attacker).IsPlayer() {
		if qHit.RewardsExperience() {
			RewardExp(combatantFor(attacker).Player(), qHit)
		}
		if combatantFor(target).IsPlayer() {
			HandleSkull(combatantFor(attacker).Player(), combatantFor(target).Player())
		}
	}
	target.GetCombat().HitQueue().AddPendingHit(qHit, combatCycle()+int64(qHit.Delay())+int64(HitProcessingDelay(qHit)))
}

// ReadsProtectionInVictimTurn is
// CombatFactory.readsProtectionInVictimTurn: an NPC's hit on
// a player whose turn this tick is still to come reads the
// protection prayer in that turn (HitQueue.process), after
// the player's clicks.
func ReadsProtectionInVictimTurn(hit *PendingHit) bool {
	attacker := hit.Attacker()
	target := hit.Target()
	if attacker == nil || target == nil {
		return false
	}
	if combatantFor(attacker).IsPlayer() || !combatantFor(target).IsPlayer() {
		return false
	}
	return !target.GetCombat().HitQueue().HasProcessed(combatCycle())
}

// HitProcessingDelay is CombatFactory.hitProcessingDelay:
// NPC targets process before players, so a hit queued
// against an NPC lands one tick after the distance table;
// player targets read 0 (PID decides in PvP, via turn order).
func HitProcessingDelay(hit *PendingHit) int {
	if hit != nil && hit.Target() != nil && !combatantFor(hit.Target()).IsPlayer() {
		return 1
	}
	return 0
}

// ExecuteHit is CombatFactory.executeHit: land one queued
// hit. Dead, untargetable, needs-placement, and
// safe-building cases drop it. Hit-time defences apply,
// manipulateHit may replace or cancel it, deferred incoming
// resolves, block animations (skipped) and interface closes
// (skipped) and Smite run, magic splashes from player casts
// deal no damage, the resolved damage queues, after-hit
// effects run, the resolved event fires, poison (skipped),
// recoil/vengeance, under-attack marking, retaliation, and
// damage-map credit follow.
func ExecuteHit(qHit *PendingHit) {
	if qHit == nil {
		return
	}
	attacker := qHit.Attacker()
	target := qHit.Target()
	if attacker == nil || target == nil {
		return
	}
	if combatantFor(target).Hitpoints() <= 0 || combatantFor(attacker).Hitpoints() <= 0 {
		return
	}
	if isUntargetable(target) || target.NeedsPlacement {
		return
	}
	if combatantFor(attacker).IsPlayer() && combatantFor(target).IsPlayer() &&
		(combatInSafeBuilding(attacker.Location()) || combatInSafeBuilding(target.Location())) {
		return
	}
	ApplyHitTimeDefences(qHit)
	resolvedHit := manipulateHit(target, qHit)
	if resolvedHit == nil {
		return
	}
	if resolvedHit != qHit {
		ApplyHitTimeDefences(resolvedHit)
	}
	ApplyDeferredImpactIncoming(resolvedHit)
	method := resolvedHit.CombatMethod()
	combatType := resolvedHit.CombatType()
	damage := resolvedHit.TotalDamage()
	// Block animations and interface closes: the block half
	// is CombatFactory.ts:1036-1041 — melee blocks play when
	// the attack is launched (combat.go), projectiles block
	// on a non-fatal impact: the target's block animation,
	// unless the method opts out, when the target survives
	// the queued damage plus this hit. The interface-close
	// half below is still skipped (no interface system).
	if combatType != CombatTypeMelee && method != nil && method.PlaysBlockAnimation() {
		if blockAnim := target.GetBlockAnim(); blockAnim >= 0 {
			if combatantFor(target).Hitpoints() > target.GetCombat().HitQueue().QueuedDamage()+damage {
				target.PerformAnimation(blockAnim, 0)
			}
		}
	}
	if targetCombatant := combatantFor(target); targetCombatant.IsPlayer() {
		if damage > 0 && PrayerActive(attacker, PrayerSmite) {
			HandleSmite(attacker, targetCombatant.Player(), damage)
		}
	}
	magicSplash := combatType == CombatTypeMagic && !resolvedHit.IsAccurate()
	if !(magicSplash && combatantFor(attacker).IsPlayer()) {
		ApplyResolvedHitDamage(target, resolvedHit)
	}
	if resolvedHit.HandleAfterHitEffects() && method != nil {
		method.HandleAfterHitEffects(resolvedHit)
	}
	if Hooks.OnCombatHitResolved != nil {
		Hooks.OnCombatHitResolved(&CombatHitResolvedEvent{Attacker: attacker, Target: target, Hit: resolvedHit})
	}
	// NPC hit sounds: skipped. Poison/venom application
	// runs here (CombatFactory.ts:1086-1145), after the
	// resolved event and before recoil/vengeance — see
	// applyPoisonOnHit (poison.go).
	applyPoisonOnHit(attacker, target, resolvedHit)
	if damage > 0 {
		if targetCombatant := combatantFor(target); targetCombatant.IsPlayer() && WearingRecoilRing(targetCombatant.Player()) {
			HandleRecoil(targetCombatant.Player(), attacker, damage)
		}
		if target.GetCombat().HasVengeance() {
			HandleVengeance(target, attacker, damage)
		}
	}
	target.GetCombat().SetUnderAttack(attacker)
	HandleRetaliation(attacker, target)
	target.GetCombat().AddDamage(attacker, damage)
}

// manipulateHit is Mobile.manipulateHit through the hook seam:
// the default returns the hit unchanged; a hook may return a
// replacement (re-defended by the caller) or nil to cancel.
// The per-splat NPC/player hit-modify events fold into this
// seam: hooks mutate the PendingHit's splats in place.
func manipulateHit(target *Mobile, hit *PendingHit) *PendingHit {
	if Hooks.ManipulateHit != nil {
		return Hooks.ManipulateHit(target, hit)
	}
	return hit
}

// ApplyResolvedHitDamage is
// CombatFactory.applyResolvedHitDamage: source the splats,
// then queue them as pending damage — immediately, or spread
// across ticks when the hit carries per-splat delays (the
// delayed splats are combat-local: they fire from the
// target's ProcessHitQueue with the incoming-generation
// check, replacing the TS TaskManager tasks).
func ApplyResolvedHitDamage(target *Mobile, resolvedHit *PendingHit) {
	hits := resolvedHit.Hits()
	for _, hit := range hits {
		if hit.Source() == nil {
			hit.SetSource(resolvedHit.Attacker())
		}
	}
	delays := resolvedHit.HitDelays()
	if len(delays) <= 1 || len(delays) != len(hits) {
		target.GetCombat().HitQueue().AddPendingDamage(hits)
		return
	}
	base := delays[0]
	for _, d := range delays {
		if d < base {
			base = d
		}
	}
	queue := target.GetCombat().HitQueue()
	generation := queue.IncomingGeneration()
	for i, hit := range hits {
		extra := delays[i] - base
		if extra < 0 {
			extra = 0
		}
		if extra <= 0 {
			queue.AddPendingDamage([]*HitDamage{hit})
			continue
		}
		target.GetCombat().queueDelayedDamage(hit, extra, generation)
	}
}

// ---- Experience ----

// RewardExp is CombatFactory.rewardExp (lines 1181-1225):
// combat XP through SkillManager. OSRS pays 4 XP per damage
// (times the NPC combat-XP multiplier, always 1 in Go — no
// per-NPC multiplier field exists). Magic with a cast or
// previous spell pays (dmg*4)/2 Magic XP on an accurate,
// non-split hit; Hitpoints pays (dmg*4)/3; defensive magic
// splits pay (dmg*4)/3 Magic + dmg Defence; melee/ranged
// split (dmg*4) across the fight style's skills. Damage <= 0
// pays nothing beyond the magic branch. TS keeps tenths (1
// damage = 1.3 HP XP); Go SkillManager stores ints, so each
// gain is floored — the fractional remainder is lost (a
// pre-existing Go storage limitation, documented at the
// call site in skills.go).
func RewardExp(player *Player, hit *PendingHit) {
	if player == nil || hit == nil {
		return
	}
	hitDamage := hit.TotalDamage()
	hitSkills := hit.Skills()
	defensiveMagicSplit := hit.CombatType() == CombatTypeMagic &&
		len(hitSkills) > 1 && containsSkill(hitSkills, SkillMagic) && containsSkill(hitSkills, SkillDefence) &&
		(hit.CombatMethod() == nil || hit.CombatMethod().SplitsDefensiveMagicExperience())
	// OSRS_DAMAGE_XP = 4 * NPC multiplier (1 in Go).
	osrsDamageXP := 4.0
	combat := player.GetCombat()
	if hit.CombatType() == CombatTypeMagic {
		if combat.CastSpell() != nil || combat.PreviousCast() != nil {
			if hit.IsAccurate() && !defensiveMagicSplit {
				player.Skills().AddExperience(SkillMagic, xpGain(float64(hitDamage)*osrsDamageXP/2))
			}
		}
	}
	if hitDamage <= 0 {
		return
	}
	player.Skills().AddExperience(SkillHitpoints, xpGain(float64(hitDamage)*osrsDamageXP/3))
	if hit.CombatType() == CombatTypeMagic {
		if !defensiveMagicSplit {
			return
		}
		player.Skills().AddExperience(SkillMagic, xpGain(float64(hitDamage)*osrsDamageXP/3))
		player.Skills().AddExperience(SkillDefence, xpGain(float64(hitDamage)))
		return
	}
	for _, skill := range hitSkills {
		if skill < 0 || skill >= SkillCount {
			continue
		}
		player.Skills().AddExperience(skill, xpGain(float64(hitDamage)*osrsDamageXP/float64(len(hitSkills))))
	}
}

func containsSkill(skills []int, skill int) bool {
	for _, s := range skills {
		if s == skill {
			return true
		}
	}
	return false
}

// xpGain converts a float XP amount to the int SkillManager
// takes: floored (TS keeps tenths; Go storage truncates).
func xpGain(amount float64) int {
	gained := int(math.Floor(amount + 1e-9))
	if gained < 0 {
		return 0
	}
	return gained
}

// ---- Retaliation ----

// HandleRetaliation is CombatFactory.handleRetaliation:
// schedule the target's automatic counter-attack. The
// attackable check (NPC definition attackable, viewer
// resolution skipped), the different-target / multi-retarget
// / leaves-NPC-fight rules, the player auto-retaliate + busy
// + no-retaliate gates, the NPC HOME-state gate, the NPC
// queue reset, and the OSRS flinch (half an attack speed
// when not already mid-fight) all run now; the attack itself
// fires one tick later from combat-local pending state
// (replacing the TS TaskManager task), re-checking the
// attackable and player gates at fire time.
func HandleRetaliation(attacker, target *Mobile) {
	if attacker == nil || target == nil {
		return
	}
	attackable := func() bool {
		if !combatantFor(target).IsPlayer() || combatantFor(attacker).IsPlayer() {
			return true
		}
		npc := combatantFor(attacker).NPC()
		if npc == nil {
			return true
		}
		if d := npc.Definition(); d != nil {
			if attackable, ok := d.(interface{ IsAttackable() bool }); ok {
				return attackable.IsAttackable()
			}
		}
		return true
	}
	if !attackable() {
		return
	}
	targetCombat := target.GetCombat()
	currentTarget := targetCombat.Target()
	playerIsBusy := func() bool {
		if !combatantFor(target).IsPlayer() {
			return false
		}
		queue := combatantFor(target).MovementQueue()
		// MovementTask checks: skipped (no TaskManager access
		// from combat).
		return queue != nil && (queue.Size() > 0 || queue.IsMoving())
	}
	hasActiveDifferentTarget := currentTarget != nil && currentTarget != attacker &&
		combatantFor(currentTarget).Hitpoints() > 0 && currentTarget.IsRegistered()
	npcCanRetargetInMulti := !combatantFor(target).IsPlayer() && combatantFor(attacker).IsPlayer() &&
		inMultiCombat(combatantFor(attacker)) && inMultiCombat(combatantFor(target))
	npcLeavesNpcFight := false
	if !combatantFor(target).IsPlayer() && combatantFor(attacker).IsPlayer() && currentTarget != nil {
		if currentNPC := combatantFor(currentTarget).NPC(); currentNPC != nil && currentNPC.Owner() == nil {
			npcLeavesNpcFight = true
		}
	}
	if !hasActiveDifferentTarget || npcCanRetargetInMulti || npcLeavesNpcFight {
		autoRet := false
		if combatantFor(target).IsPlayer() {
			autoRet = targetCombat.AutoRetaliate() && !playerIsBusy() &&
				!HasCombatFlag(target, CombatFlagNoRetaliate)
		} else {
			autoRet = !HasCombatFlag(target, CombatFlagNoRetaliate) && retaliationHomeState(target)
		}
		if !autoRet {
			return
		}
		if npc := combatantFor(target).NPC(); npc != nil {
			if queue := npc.MovementQueue(); queue != nil {
				queue.Reset(true)
			}
		}
		if currentTarget == nil || combatantFor(currentTarget).Hitpoints() <= 0 || !currentTarget.IsRegistered() {
			attackSpeed := GetMethod(target).AttackSpeed(target)
			if attackSpeed < 1 {
				attackSpeed = 1
			}
			targetCombat.ExtendAttackDelay(attackSpeed / 2)
		}
		targetCombat.scheduleRetaliation(attacker, combatCycle()+1, attackable, playerIsBusy)
	}
}

// retaliationHomeState answers the NPC coordinator HOME check
// for retaliation: the hook wins; else the coordinator state
// (a missing coordinator reads as HOME).
func retaliationHomeState(target *Mobile) bool {
	npc := combatantFor(target).NPC()
	if npc == nil {
		return false
	}
	if Hooks.RetaliationHomeCheck != nil {
		return Hooks.RetaliationHomeCheck(npc)
	}
	if coordinator := npc.MovementCoordinator(); coordinator != nil {
		return coordinator.GetCoordinateState() == CoordinateStateHome
	}
	return true
}

// ---- Redemption / Smite / Recoil / Vengeance ----

// HandleRedemption is CombatFactory.handleRedemption: after
// every applied hit (including 0s), a living player with
// Redemption active below 10% hitpoints heals floor(max
// Prayer / 4), empties Prayer, and deactivates all prayers.
// It cannot save a lethal hit (hitpoints <= 0 returns).
// The graphic is skipped.
func HandleRedemption(victim *Mobile) {
	victimCombatant := combatantFor(victim)
	if !victimCombatant.IsPlayer() {
		return
	}
	player := victimCombatant.Player()
	hitpoints := victimCombatant.Hitpoints()
	if hitpoints <= 0 || !PrayerActive(victim, PrayerRedemption) {
		return
	}
	skills := player.Skills()
	if hitpoints*10 >= skills.MaxLevel(SkillHitpoints) {
		return
	}
	skills.SetCurrentLevel(SkillPrayer, 0, true)
	skills.SetCurrentLevel(SkillHitpoints, hitpoints+skills.MaxLevel(SkillPrayer)/4, true)
	player.SendMessage("You have run out of Prayer points!")
	DeactivatePrayers(victim)
}

// HandleSmite is CombatFactory.handleSmite: Smite drains
// floor(damage/4) Prayer, floored at 0.
func HandleSmite(attacker *Mobile, victim *Player, damage int) {
	drain := damage / 4
	if drain > 0 {
		DrainPrayer(victim, drain)
	}
}

// CanReceiveRecoil is CombatFactory.canReceiveRecoil: the
// combat:recoil-immune attribute denies; the
// combat:recoil-target event is skipped (no hook field).
func CanReceiveRecoil(target *Mobile) bool {
	if target == nil {
		return false
	}
	return target.GetAttribute(RecoilImmuneAttribute) != true
}

// RecoilDamage is CombatFactory.recoilDamage: floor(damage /
// 10) + 1 (0 for non-positive damage).
func RecoilDamage(damage int) int {
	if damage <= 0 {
		return 0
	}
	return damage/10 + 1
}

// WearingRecoilRing reports whether player wears a ring of
// recoil (ring slot, id 2550).
func WearingRecoilRing(player *Player) bool {
	if player == nil {
		return false
	}
	ring := player.Equipment().ItemAt(RingSlot)
	return ring.IsValid() && ring.ID() == RingOfRecoilID
}

// HandleRecoil is CombatFactory.handleRecoil: rebound recoil
// damage to the attacker as reflected pending damage, track
// the 40 charges in the ring-of-recoil:damage attribute, and
// shatter the ring (clearing the slot, with the message) when
// the charges run out. The last recoil deals only what is left.
func HandleRecoil(player *Player, attacker *Mobile, damage int) {
	if player == nil || attacker == nil || !CanReceiveRecoil(attacker) {
		return
	}
	returnDmg := RecoilDamage(damage)
	if returnDmg <= 0 {
		return
	}
	used := attributeInt(player.GetAttribute(RecoilDamageAttribute))
	if used < 0 {
		used = 0
	}
	remaining := RecoilRingCharges - used
	if remaining < 0 {
		remaining = 0
	}
	if returnDmg > remaining {
		returnDmg = remaining
	}
	if returnDmg <= 0 {
		return
	}
	hit := NewHitDamage(returnDmg, HitMaskRed)
	hit.MarkReflected().SetOrigin(HitDamageOriginRecoil).SetSource(&player.Mobile)
	attacker.GetCombat().HitQueue().AddPendingDamage([]*HitDamage{hit})
	if used+returnDmg >= RecoilRingCharges {
		player.Equipment().Set(RingSlot, NewItem(-1, 0))
		player.Equipment().RefreshItems()
		player.SendMessage("<col=7f007f>Your Ring of Recoil has shattered.</col>")
		player.SetAttribute(RecoilDamageAttribute, 0)
	} else {
		player.SetAttribute(RecoilDamageAttribute, used+returnDmg)
	}
}

// HandleVengeance is CombatFactory.handleVengeance: rebound
// 75% rounded down (min 1) when receivable; always
// force-chats "Taste vengeance!" and clears vengeance, even
// when the rebound is immune (the rebound is spent and deals
// nothing).
func HandleVengeance(character, attacker *Mobile, damage int) {
	if character == nil || attacker == nil || damage <= 0 {
		return
	}
	if CanReceiveRecoil(attacker) {
		returnDmg := int(math.Floor(float64(damage) * 0.75))
		if returnDmg < 1 {
			returnDmg = 1
		}
		hit := NewHitDamage(returnDmg, HitMaskRed)
		hit.MarkReflected().SetOrigin(HitDamageOriginVengeance).SetSource(character)
		attacker.GetCombat().HitQueue().AddPendingDamage([]*HitDamage{hit})
	}
	character.ForceChat("Taste vengeance!")
	character.GetCombat().SetVengeance(false)
}

// attributeInt reads an attribute as an int (int, int64, and
// float64 forms; anything else reads 0).
func attributeInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return 0
	}
}

// ---- Skulls ----

// IsSkulled answers whether player is skulled, through the
// skull seam (wired to the live system in cmd/server; the
// zero seam reads false, as before the system existed).
func IsSkulled(player *Player) bool {
	if Hooks.IsSkulled != nil {
		return Hooks.IsSkulled(player)
	}
	return false
}

// HandleSkull is CombatFactory.handleSkull: wilderness-only.
// The target records the attack (retaliation without a skull
// from now on, before the hit lands). A skulled attacker
// extends the skull unless retaliating; an unskulled attacker
// who would skull gains a white 30-minute skull (through the
// seam).
func HandleSkull(attacker, target *Player) {
	if attacker == nil || target == nil {
		return
	}
	if !combatInWilderness(attacker.Location()) || !combatInWilderness(target.Location()) {
		return
	}
	skulls := WouldSkull(attacker, target)
	RecordAttackedBy(target, attacker)
	if IsSkulled(attacker) {
		retaliating := WasAttackedBy(attacker, target) || attacker.GetCombat().Attacker() == &target.Mobile
		if !retaliating && Hooks.ExtendSkull != nil {
			Hooks.ExtendSkull(attacker, PVPSkullSeconds)
		}
		return
	}
	if !skulls {
		return
	}
	if Hooks.ApplySkull != nil {
		Hooks.ApplySkull(attacker, PVPSkullSeconds)
	}
}

// WouldSkull is CombatFactory.wouldSkull: wilderness,
// unskulled, no damage-map entries either way, neither
// fighter the other's attacker, and not attacked-by.
func WouldSkull(attacker, target *Player) bool {
	if attacker == nil || target == nil {
		return false
	}
	if !combatInWilderness(attacker.Location()) || !combatInWilderness(target.Location()) || IsSkulled(attacker) {
		return false
	}
	if target.GetCombat().DamageMapContains(&attacker.Mobile) || attacker.GetCombat().DamageMapContains(&target.Mobile) {
		return false
	}
	if target.GetCombat().Attacker() == &attacker.Mobile {
		return false
	}
	if attacker.GetCombat().Attacker() == &target.Mobile {
		return false
	}
	return !WasAttackedBy(attacker, target)
}

// RecordAttackedBy records that target was attacked by
// attacker this cycle (the combat:pvp-attacked-by attribute
// map, username -> cycle).
func RecordAttackedBy(target, attacker *Player) {
	if target == nil || attacker == nil {
		return
	}
	var record map[string]int64
	if existing, ok := target.GetAttribute(AttackedByAttribute).(map[string]int64); ok {
		record = existing
	} else {
		record = map[string]int64{}
		target.SetAttribute(AttackedByAttribute, record)
	}
	record[attacker.Username] = combatCycle()
}

// WasAttackedBy reports whether player was attacked by other
// within the last 100 ticks.
func WasAttackedBy(player, other *Player) bool {
	if player == nil || other == nil {
		return false
	}
	record, ok := player.GetAttribute(AttackedByAttribute).(map[string]int64)
	if !ok {
		return false
	}
	at, ok := record[other.Username]
	return ok && combatCycle()-at < AttackedByTicks
}

// ---- Stun / Freeze / Prayer block ----

// StunTicks is CombatFactory.stunTicks: the STUN_IMMUNITY
// grace always applies (force never bypasses it); without
// force, an already-stunned fighter is left alone. STUN and
// STUN_IMMUNITY (ticks+1) register, combat and the queue
// reset, and a player is told (message=false silences it).
// The graphic is skipped.
func StunTicks(character *Mobile, ticks int, force bool, message bool) {
	if character == nil {
		return
	}
	timers := character.GetTimers()
	if timers.Has(TimerKeyStunImmunity) {
		return
	}
	if !force && timers.Has(TimerKeyStun) {
		return
	}
	timers.Registers(TimerKeyStun, ticks)
	timers.Registers(TimerKeyStunImmunity, ticks+1)
	character.GetCombat().Reset()
	if queue := combatantFor(character).MovementQueue(); queue != nil {
		queue.Reset(true)
	}
	if message && combatantFor(character).IsPlayer() {
		combatantFor(character).SendMessage("You've been stunned!")
	}
}

// Freeze is CombatFactory.freeze: freeze for seconds with the
// default message.
func Freeze(character *Mobile, seconds float64, message string, source *Mobile) {
	FreezeTicks(character, int(math.Ceil(seconds*1000/600)), &message, source, "")
}

// FreezeTicks is CombatFactory.freezeTicks: the FREEZE /
// FREEZE_IMMUNITY guards, freeze resistance (100 = immune),
// the large-NPC rule (size > 2 needs the freezable flag or
// positive resistance), the freeze-duration event (skipped:
// no hook field), resistance shortening, FREEZE +
// FREEZE_IMMUNITY (ticks + 5, the 3s grace), the queue reset,
// and the player message. message nil means the default
// ("You have been frozen!"); a pointer to "" sends nothing.
// The effect timer and frozen event are skipped.
func FreezeTicks(character *Mobile, ticks int, message *string, source *Mobile, kind string) {
	_ = source
	_ = kind
	if character == nil {
		return
	}
	timers := character.GetTimers()
	if timers.Has(TimerKeyFreeze) || timers.Has(TimerKeyFreezeImmunity) {
		return
	}
	fighter := combatantFor(character)
	resistance := 0
	if d := fighter.NPCDef(); d != nil {
		resistance = d.FreezeResistance()
	}
	if resistance >= 100 {
		return
	}
	if fighter.Size() > 2 && !HasCombatFlag(character, CombatFlagFreezable) && resistance <= 0 {
		return
	}
	// combat:freeze-duration: skipped (resisted=false).
	freezeTicks := float64(ticks)
	if resistance > 0 {
		freezeTicks = freezeTicks * float64(100-resistance) / 100
	}
	ticks = int(math.Floor(freezeTicks))
	if ticks < 1 {
		ticks = 1
	}
	timers.Registers(TimerKeyFreeze, ticks)
	timers.Registers(TimerKeyFreezeImmunity, ticks+5)
	if queue := fighter.MovementQueue(); queue != nil {
		queue.Reset(true)
	}
	if fighter.IsPlayer() {
		text := "You have been frozen!"
		if message != nil {
			text = *message
		}
		if text != "" {
			fighter.SendMessage(text)
		}
	}
}

// DisableProtectionPrayers is
// CombatFactory.disableProtectionPrayers: an already-disabled
// player is left alone; otherwise the 5-second prayer block
// starts, protection prayers reset, and the player is told.
func DisableProtectionPrayers(player *Player) {
	if player == nil {
		return
	}
	combat := player.GetCombat()
	if !combat.PrayerBlockFinished() {
		return
	}
	combat.StartPrayerBlock(5)
	DeactivateProtectionPrayers(&player.Mobile)
	player.SendMessage("You have been disabled and can no longer use protection prayers.")
}

// ---- Ammo ----

// CheckAmmo is CombatFactory.checkAmmo: the ranged shot's
// ammo gate. No ranged weapon rejects silently; the plugin
// check wins; crystal bows are skipped (no charge data in
// Go — they fall through to the ammo checks, a documented
// gap); no ammo data rejects with the quiver line;
// weapon-slot ammo and ammo-slot ammo check the required
// amount; the wrong ammo rejects with the bow/crossbow line
// (always "bow": item names are absent from
// ItemDefinitionSource). Rejection resets combat unless
// skipReset.
func CheckAmmo(player *Player, amountRequired int, skipReset bool) bool {
	if player == nil {
		return false
	}
	combat := player.GetCombat()
	rangedWeapon := combat.RangedWeapon()
	ammoData := combat.Ammunition()
	reject := func(message string) bool {
		if message != "" {
			player.SendMessage(message)
		}
		if !skipReset {
			combat.Reset()
		}
		return false
	}
	if rangedWeapon == nil {
		return reject("")
	}
	weapon := player.Equipment().GetWeapon()
	ammo := player.Equipment().ItemAt(AmmunitionSlot)
	if Hooks.CheckAmmo != nil {
		if checked := Hooks.CheckAmmo(player, weapon, ammo); checked != nil {
			return *checked
		}
	}
	// Crystal bow charges: skipped (no charge data).
	const noAmmo = "There is no ammo left in your quiver."
	if ammoData == nil {
		return reject(noAmmo)
	}
	if rangedWeapon.UsesWeaponSlotAmmo {
		if !weapon.IsValid() || weapon.Amount() < amountRequired {
			return reject("You don't have the required amount of ammunition to fire that.")
		}
		return true
	}
	if !ammo.IsValid() || ammo.Amount() <= 0 {
		return reject(noAmmo)
	}
	if ammo.Amount() < amountRequired {
		return reject("You don't have the required amount of ammunition to fire that.")
	}
	properReq := false
	for _, allowed := range rangedWeapon.AmmoIDs {
		if allowed == ammo.ID() && ammunitionDataForItem(ammo.ID()) == ammoData {
			properReq = true
			break
		}
	}
	if !properReq {
		return reject("You can't use that ammo with your bow.")
	}
	return true
}

// DecrementAmmo is CombatFactory.decrementAmmo: pay for a
// fired shot. The ammo-saving and plugin-owned-ammo paths
// are covered by Hooks.DecrementAmmo; charge/ether bows
// (IsRangedChargeBow) spend nothing here — their shots are
// paid by charges or ether, never by eating the bow; otherwise
// each shot rolls break (20%), floor drop (80% minus recovery),
// or recovery; the slot pays on the firing tick; the last
// thrown weapon messages; the floor drop goes through the
// ground-item seam immediately (the flight delay is not
// honored: no delayed item-drop scheduling exists in combat).
func DecrementAmmo(player *Player, pos Location, amount int, delayTicks int) {
	_ = delayTicks
	if player == nil {
		return
	}
	combat := player.GetCombat()
	rangedWeapon := combat.RangedWeapon()
	weapon := player.Equipment().GetWeapon()
	ammo := player.Equipment().ItemAt(AmmunitionSlot)
	if Hooks.DecrementAmmo != nil && Hooks.DecrementAmmo(player, weapon, ammo, amount) {
		return
	}
	if rangedWeapon == nil {
		return
	}
	// Charge/ether bows spend no stackable ammunition here:
	// their shots are paid by charges or ether, never by
	// eating the bow from the weapon slot
	// (CombatFactory.decrementAmmo, CombatFactory.ts:1799-1811:
	// the plugin ammo handlers run first, then webweaver/craw
	// return early, then the crystal-bow branch degrades
	// stages instead of decrementing). Without this guard a
	// crystal bow's UsesWeaponSlotAmmo would decrement the bow
	// itself to nothing on its first shot. The charge
	// degradation itself is NOT STARTED (see ranged_data.go):
	// until it lands, these bows fire without consuming
	// anything.
	if IsRangedChargeBow(weapon.ID()) {
		return
	}
	slot := AmmunitionSlot
	if rangedWeapon.UsesWeaponSlotAmmo {
		slot = WeaponSlot
	}
	ammoItem := player.Equipment().ItemAt(slot)
	recovery := 0
	if Hooks.AmmoRecoveryPercent != nil {
		recovery = Hooks.AmmoRecoveryPercent(player)
	}
	dropChance := 80 - recovery
	lost, dropped := 0, 0
	for shot := 0; shot < amount; shot++ {
		roll := randomInclusive(99)
		if roll < AmmoBreakChance {
			lost++
		} else if roll < AmmoBreakChance+dropChance {
			dropped++
		}
	}
	ammoID := ammoItem.ID()
	paid := lost + dropped
	if paid > ammoItem.Amount() {
		paid = ammoItem.Amount()
	}
	landed := dropped
	if landed > paid {
		landed = paid
	}
	if paid > 0 {
		ammoItem.SetAmount(ammoItem.Amount() - paid)
		player.Equipment().Set(slot, ammoItem)
	}
	if ammoItem.Amount() <= 0 {
		if slot == WeaponSlot {
			player.SendMessage("You have run out of ammunition!")
		}
		player.Equipment().Set(slot, NewItem(-1, 0))
	}
	player.Equipment().RefreshItems()
	if landed > 0 && Hooks.DropGroundItem != nil {
		Hooks.DropGroundItem(NewItem(ammoID, landed), pos, player)
	}
}

// AmmoRecovery is CombatFactory.ammoRecovery: the percent of
// fired ammo recovered (Ava's devices), through the seam.
func AmmoRecovery(player *Player) int {
	if Hooks.AmmoRecoveryPercent != nil {
		return Hooks.AmmoRecoveryPercent(player)
	}
	return 0
}
