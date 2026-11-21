package game

import (
	"math"
)

// Poison and venom: the port of
// game/task/impl/CombatPoisonEffect.ts (526 lines) — the
// per-entity poison task — plus CombatFactory.poisonEntity
// (CombatFactory.ts:1241-1300) and the poison application
// call sites in CombatFactory.executeHit (:1086-1145).
//
// TICK MECHANISM (grounded, no invented scheduler): Go runs
// per-entity recurring tasks on the world's TaskManager
// (task.go; World.Process runs Tasks().Process first each
// cycle), and the special-attack restore (special_restore.go)
// is the precedent for a per-entity task submitted to it.
// The poison effect does NOT use that manager: its tick must
// run even for a fighter with no TaskManager installed
// (standalone tests, NoCache mode), and its pulse is already
// driven by the combat tick — Combat.ProcessHitQueue runs
// once per cycle for every player (Player.Process) and NPC
// (NPC.Process), before the owner acts, which is the same
// order TS has (the task-manager phase runs before the
// process_players / process_npcs phases). So the effect
// ticks from ProcessHitQueue (combat.go), guarded to once
// per cycle, with the running state on Combat (the Go form
// of the WeakMap<Mobile, CombatPoisonEffect> active map).
// Resume/Suspend keep the TS login/logout/combat-bind sites
// (Combat.ts:811 getPoisonClock, Player.ts:757/801):
// ResumePoisonEffect restarts the effect, SuspendPoisonEffect
// stops it before the logout save while retaining the saved
// affliction. A fresh login needs no explicit resume call to
// work — the tick re-evaluates the resume condition every
// cycle — but the coordinator should still call
// ResumePoisonEffect at the onLogin site and
// SuspendPoisonEffect at the onLogout site, as TS does.
//
// POISON MODEL (CombatPoisonEffect.ts):
//   - 30-tick pulse. Poison damage = floor((severity + 4) / 5)
//     (damageFromSeverity); severity decays -1 per hit.
//   - Venom: the stored value IS the damage, +2 per hit,
//     cap 20, no decay or expiry — only a cure ends it.
//   - Hits go through the hit queue as HitMaskGreen.
//   - On death (HP <= 0): poisonDamage 0, venomed false,
//     player clock stopped.
//   - Orb varp 102: venom = 1,000,000 + hits dealt
//     (round((poisonDamage - 6) / 2)), poison = severity,
//     immunity = -ceil(seconds / 18) while > 2s.
//   - Resistance lines (purple <col=7f007f>): counter 1
//     "about to wear off", 0 "has worn off", 39 "venom
//     resistance is about to wear off", 38 "venom resistance
//     has worn off, but you are still resistant to poison";
//     reduction lines on tier crossings.
//   - Immunity prevents new afflictions in poisonEntity; it
//     does not cure an existing one.
//
// PoisonType values (CombatPoisonEffect.ts PoisonType enum):
// VERY_WEAK 2, WEAK 3, MILD 4, EXTRA 5, SUPER 6, VENOM 12.
// The item table lives in poison_data_tables.go
// (script-extracted, 258 entries).

// PoisonType is one CombatPoisonData poison kind.
type PoisonType int

const (
	// PoisonTypeVeryWeak is VERY_WEAK (2).
	PoisonTypeVeryWeak PoisonType = 2
	// PoisonTypeWeak is WEAK (3).
	PoisonTypeWeak PoisonType = 3
	// PoisonTypeMild is MILD (4).
	PoisonTypeMild PoisonType = 4
	// PoisonTypeExtra is EXTRA (5).
	PoisonTypeExtra PoisonType = 5
	// PoisonTypeSuper is SUPER (6).
	PoisonTypeSuper PoisonType = 6
	// PoisonTypeVenom is VENOM (12).
	PoisonTypeVenom PoisonType = 12
)

// ToxicBlowpipeID is ItemIdentifiers.TOXIC_BLOWPIPE (12926):
// the blowpipe rolls its own venom (ToxicBlowpipe plugin,
// 1 in 4), so the ranged application site never poisons
// with it (CombatFactory.ts:1105-1108).
const ToxicBlowpipeID = 12926

// NoxiousHalberdID is ItemIdentifiers.NOXIOUS_HALBERD (29796).
const NoxiousHalberdID = 29796

// NoxiousHalberd4ID is ItemIdentifiers.NOXIOUS_HALBERD_4
// (33178): the second wieldable noxious halberd copy.
const NoxiousHalberd4ID = 33178

// PoisonOrbVarp is ORB_VARP (102): the hitpoints orb's
// poison/venom/immunity value.
const PoisonOrbVarp = 102

// poisonOrbVenomBase is ORB_VENOM_BASE (1,000,000).
const poisonOrbVenomBase = 1000000

// poisonOrbImmunityStepSeconds is ORB_IMMUNITY_STEP_SECONDS
// (18): antipoison immunity counts down in steps of 30 ticks.
const poisonOrbImmunityStepSeconds = 18

// poisonOrbImmunityMinSeconds is ORB_IMMUNITY_MIN_SECONDS
// (2): shorter immunity is a serpentine helm's refresh, not
// a dose: the orb leaves it out.
const poisonOrbImmunityMinSeconds = 2

// PoisonTypeForItemID returns the poison type of a poisoned
// item id (CombatPoisonData.types.get), or false when the id
// is not a poisoned item.
func PoisonTypeForItemID(id int) (PoisonType, bool) {
	typ, ok := poisonTypeByItemID[id]
	return typ, ok
}

// PoisonTypeForItem returns the poison type of an item
// (CombatPoisonData.getPoisonType): invalid items (id < 1
// or amount < 1) have none.
func PoisonTypeForItem(item Item) (PoisonType, bool) {
	if item.ID() < 1 || item.Amount() < 1 {
		return 0, false
	}
	return PoisonTypeForItemID(item.ID())
}

// GetMeleeSeverity is CombatPoisonData.getMeleeSeverity:
// EXTRA 25, SUPER 30, VENOM 6, default 20.
func GetMeleeSeverity(typ PoisonType) int {
	switch typ {
	case PoisonTypeExtra:
		return 25
	case PoisonTypeSuper:
		return 30
	case PoisonTypeVenom:
		return 6
	default:
		return 20
	}
}

// GetRangedSeverity is CombatPoisonData.getRangedSeverity:
// WEAK 11, MILD 16, VENOM 6, default 6.
func GetRangedSeverity(typ PoisonType) int {
	switch typ {
	case PoisonTypeWeak:
		return 11
	case PoisonTypeMild:
		return 16
	case PoisonTypeVenom:
		return 6
	default:
		return 6
	}
}

// GetMeleeRollRange is CombatPoisonData.getMeleeRollRange:
// the noxious halberd rolls 2 (1 in 3), anything else 3
// (1 in 4) — Misc.getRandom(range) === 0, inclusive on both
// ends (Misc.ts:221-223).
func GetMeleeRollRange(itemID int) int {
	if itemID == NoxiousHalberdID || itemID == NoxiousHalberd4ID {
		return 2
	}
	return 3
}

// DamageFromSeverity is CombatPoisonEffect.damageFromSeverity:
// floor((severity + 4) / 5). The only formula.
func DamageFromSeverity(severity int) int {
	// Go's / truncates toward zero; severity is always > 0
	// at the call sites, where truncation is the floor.
	return (severity + 4) / 5
}

// OrbValue is CombatPoisonEffect.orbValue: the hitpoints
// orb's poison state. Venom is 1,000,000 plus the hits it
// has dealt (dark green), poison its severity (green), and
// antipoison immunity minus the 18-second steps left (-5 for
// 90 seconds, -20 for 6 minutes); 0 is none. Players read
// the clock counter; NPCs read the poison-immunity timer.
func OrbValue(m *Mobile) int {
	if m == nil {
		return 0
	}
	if m.IsVenomed() {
		return poisonOrbVenomBase + max(0, int(math.Round(float64(m.PoisonDamage()-6)/2)))
	}
	if m.IsPoisoned() {
		return m.PoisonDamage()
	}
	combat := m.GetCombat()
	if combatantFor(m).IsPlayer() {
		counter := combat.PoisonClock().Counter()
		if counter > 0 {
			return -counter
		}
		return 0
	}
	immunity := combat.PoisonImmunitySecondsRemaining()
	if immunity > poisonOrbImmunityMinSeconds {
		return -int(math.Ceil(immunity / poisonOrbImmunityStepSeconds))
	}
	return 0
}

// StatusApplyPayload is the "combat:status-apply" custom
// event (CombatFactory.ts:1283-1287): content may make the
// player immune outright (a league relic) — the same
// question for every status the server applies. A pointer:
// handlers mutate Allow in place, as TS plugins mutate the
// event object.
type StatusApplyPayload struct {
	// Target is the player the status would apply to.
	Target *Player
	// Kind is "venom" or "poison".
	Kind string
	// Allow is true unless a handler vetoes the application.
	Allow bool
}

// NpcPoisonPayload is the "combat:npc-poison" custom event
// (CombatFactory.ts:1130-1140): a poisonous/venomous NPC's
// hit may poison its target. A pointer: handlers set
// Handled (and may change Severity) in place.
type NpcPoisonPayload struct {
	// Attacker is the poisonous NPC.
	Attacker *Mobile
	// Target is the fighter the NPC hit.
	Target *Mobile
	// Severity is the poison severity (venomous: 6, the
	// starting venom damage; else 30) — not the first hit.
	Severity int
	// Venomous reports whether the attacker is venomous.
	Venomous bool
	// Handled, when set by a handler, suppresses the
	// default poisonEntity application.
	Handled bool
}

// PoisonEntity is CombatFactory.poisonEntity
// (CombatFactory.ts:1241-1300): apply poison or venom to an
// entity. severity <= 0 returns; venom can't re-apply,
// restack, or downgrade once active. orbType 1 is poison,
// 2 is venom (the TS default is 1). NPC venom/poison
// immunity comes from the definition (venom-immune converts
// to poison, severity x 5); player immunity comes from the
// clock (venom blocked if isVenomImmune, poison blocked if
// the clock is not finished). The combat:status-apply custom
// event may veto a player application. An accepted venom
// clears the poison counter; the "You have been poisoned!"
// / "You have been poisoned by venom!" message fires only
// when starting from nothing. Venom sets the value outright;
// poison takes the max of current/new. The effect is
// submitted only if not already poisoned.
func PoisonEntity(entity *Mobile, severity int, orbType int) {
	if entity == nil || severity <= 0 {
		return
	}
	// Venom can't be re-applied, restacked, or downgraded
	// back to regular poison once active — it only ends
	// via a cure. Matches OSRS.
	if entity.IsVenomed() {
		return
	}
	isVenom := orbType == 2
	if !combatantFor(entity).IsPlayer() {
		if def := combatantFor(entity).NPCDef(); def != nil {
			if isVenom && def.IsVenomImmune() {
				isVenom = false
				// Venom stores damage; ordinary poison
				// stores five severity points per starting
				// damage. Keep the converted poison's hit.
				severity *= 5
			}
			if !isVenom && def.IsPoisonImmune() {
				return
			}
		}
	}
	alreadyPoisoned := entity.IsPoisoned()
	if player := combatantFor(entity).Player(); player != nil {
		protection := player.GetCombat().PoisonClock()
		if isVenom {
			if protection.IsVenomImmune() {
				return
			}
		} else if !protection.Finished() {
			return
		}
		// Content may make the player immune outright (a
		// league relic): the same question for every status
		// the server applies.
		kind := "poison"
		if isVenom {
			kind = "venom"
		}
		status := &StatusApplyPayload{Target: player, Kind: kind, Allow: true}
		emitPlayerEvent(player, "combat:status-apply", status)
		if !status.Allow {
			return
		}
		// An accepted venom replaces the ordinary negative
		// poison counter.
		if isVenom {
			protection.ClearCounter()
		}
		// Live names venom, only when it starts from nothing
		// (rsprox_transcripts/237/01087.txt line 30368; 275
		// recorded); a poison turned to venom, or venom
		// reapplied, says nothing.
		if !alreadyPoisoned {
			if isVenom {
				player.SendMessage("You have been poisoned by venom!")
			} else {
				player.SendMessage("You have been poisoned!")
			}
		}
	}
	entity.SetVenomed(isVenom)
	// Venom overrides any existing (weaker) poison outright
	// and starts its own escalating curve; regular poison
	// keeps the OSRS re-poison rule of taking the stronger
	// of the current and new severity.
	if isVenom {
		entity.SetPoisonDamage(severity)
	} else if severity > entity.PoisonDamage() {
		entity.SetPoisonDamage(severity)
	}
	if !alreadyPoisoned {
		ResumePoisonEffect(entity)
	}
	SyncPoisonOrb(entity)
}

// CurePoison cures an entity (the engine cure API; no cure
// items are ported — antipoison etc. are content): the
// affliction fields clear and the effect stops, resetting
// the clock phase per CombatPoisonEffect.stop. The clock
// itself is NOT stopped: an antipoison's immunity is a
// separate clock dose the content applies afterwards, and a
// plain cure leaves a running clock running.
func CurePoison(entity *Mobile) {
	if entity == nil {
		return
	}
	entity.SetPoisonDamage(0)
	entity.SetVenomed(false)
	StopPoisonEffect(entity)
	SyncPoisonOrb(entity)
}

// ResumePoisonEffect is CombatPoisonEffect.resume: resume a
// saved affliction without reapplying or changing its
// damage. It runs on login (Player.ts:801) and on combat
// bind (Combat.ts:811, through getPoisonClock's ensure
// callback). Unregistered or dead entities, and entities
// with nothing to resume, are no-ops; an already-running
// effect is left alone (its 30-tick countdown is not reset).
func ResumePoisonEffect(entity *Mobile) {
	if entity == nil || !entity.IsRegistered() {
		return
	}
	// Dead is HP <= 0 or the death sequence started: an
	// NPC's Hitpoints() reads its definition value when
	// the field is 0 ("unset", combatant.go), so a dead
	// NPC reads as alive through Hitpoints() alone — the
	// Dying flag (set by beginDeath) is the reliable read.
	if combatantFor(entity).Hitpoints() <= 0 || combatantFor(entity).Dying() {
		return
	}
	if !entity.IsPoisoned() {
		player := combatantFor(entity).Player()
		if player == nil {
			return
		}
		clock := player.GetCombat().PoisonClock()
		if !clock.HasCounter() && !clock.HasPendingReduction() {
			return
		}
	}
	combat := entity.GetCombat()
	if combat.poisonEffectRunning {
		return
	}
	combat.poisonSuspended = false
	combat.poisonEffectRunning = true
	combat.poisonTicksUntilHit = PoisonClockPeriodTicks
}

// SuspendPoisonEffect is CombatPoisonEffect.suspend: stop
// callbacks before saving/removing an actor, retaining the
// saved affliction (Player.ts:757, before the logout save).
// Cancelling a callback is not a cure: the fields and the
// clock survive, and the next ResumePoisonEffect (or the
// next tick's resume check, once the suspension clears)
// restarts the countdown from 30.
func SuspendPoisonEffect(entity *Mobile) {
	if entity == nil {
		return
	}
	combat := entity.GetCombat()
	combat.poisonEffectRunning = false
	combat.poisonSuspended = true
}

// StopPoisonEffect is CombatPoisonEffect.stop: stop the
// effect for an entity. Cancelling a callback (notably
// before the logout save) is not a cure. When a player is
// left with neither poison nor a clock counter, the clock
// phase resets. Worker C (player death) calls this for the
// death reset; CurePoison calls it for a cure.
func StopPoisonEffect(entity *Mobile) {
	if entity == nil {
		return
	}
	combat := entity.GetCombat()
	if !combat.poisonEffectRunning {
		return
	}
	combat.poisonEffectRunning = false
	if player := combatantFor(entity).Player(); player != nil &&
		!entity.IsPoisoned() && !player.GetCombat().PoisonClock().HasCounter() {
		player.GetCombat().PoisonClock().ResetPhase()
	}
}

// SyncPoisonOrb sends the orb's poison/venom/immunity value
// (varp 102) when it changes (Player.ts:465-473,
// syncPoisonOrb, called from Player.process every tick).
// The first call always sends. NPCs have no session, so
// nothing is sent for them. A player with no session drops
// the send (Player.SendVarp is a nil-session no-op).
func SyncPoisonOrb(m *Mobile) {
	player := combatantFor(m).Player()
	if player == nil {
		return
	}
	combat := player.GetCombat()
	value := OrbValue(m)
	if combat.poisonLastOrbSet && combat.poisonLastOrb == value {
		return
	}
	combat.poisonLastOrb = value
	combat.poisonLastOrbSet = true
	player.SendVarp(PoisonOrbVarp, value)
}

// processPoisonEffect ticks the poison effect for one cycle
// (CombatPoisonEffect.execute), driven from
// Combat.ProcessHitQueue — once per cycle, before the owner
// acts. The body follows the TS execute order exactly:
// registration/death gates, reduction + resistance lines,
// the clock pulse, the non-clock 30-tick countdown, then
// the venom escalation or the poison decay hit.
func (c *Combat) processPoisonEffect(cycle int64) {
	if c == nil || c.owner == nil {
		return
	}
	if c.poisonLastTickCycle == cycle {
		return
	}
	c.poisonLastTickCycle = cycle
	entity := c.owner
	if c.poisonSuspended {
		return
	}
	if !entity.IsRegistered() {
		c.poisonEffectRunning = false
		return
	}
	if combatantFor(entity).Hitpoints() <= 0 || combatantFor(entity).Dying() {
		entity.SetPoisonDamage(0)
		entity.SetVenomed(false)
		if player := combatantFor(entity).Player(); player != nil {
			player.GetCombat().PoisonClock().Stop()
		}
		c.poisonEffectRunning = false
		SyncPoisonOrb(entity)
		return
	}
	player := combatantFor(entity).Player()
	var clock *PoisonClock
	if player != nil {
		clock = player.GetCombat().PoisonClock()
	}
	afflicted := entity.IsPoisoned()
	// The resume condition, re-evaluated every tick: an
	// afflicted entity, or a player whose clock still holds
	// a counter or a pending reduction, runs the effect.
	// (TS submits the task only from resume/poisonEntity;
	// the tick re-check is the Go form of those call sites
	// for a fighter whose effect was never started — a
	// login restore, or a direct field set in tests.)
	if !c.poisonEffectRunning {
		if !afflicted && (clock == nil || (!clock.HasCounter() && !clock.HasPendingReduction())) {
			return
		}
		c.poisonEffectRunning = true
		c.poisonTicksUntilHit = PoisonClockPeriodTicks
	}
	modal := false
	if player != nil {
		modal = player.GetInterfaceId() > 0 || player.hasInterruptibleInterface()
	}
	if clock != nil && (!modal || afflicted) {
		if before, after, ok := clock.TakeReduction(); ok && !afflicted && player != nil {
			sendPoisonReductionLines(player, before, after)
		}
	}
	if !afflicted && (clock == nil || (!clock.HasCounter() && !clock.HasPendingReduction())) {
		entity.SetVenomed(false)
		StopPoisonEffect(entity)
		SyncPoisonOrb(entity)
		return
	}
	// Immunity prevents new afflictions in poisonEntity; it
	// does not cure an existing one (for example when
	// equipping a serpentine helm). Explicit cures clear the
	// affliction fields, handled by isPoisoned above.
	if clock != nil {
		before := clock.Counter()
		pulse := clock.TickPulse(modal)
		// Explicit duration reductions can put protection
		// transitions between status pulses.
		if !afflicted && player != nil && before > clock.Counter() {
			sendPoisonResistanceLine(player, clock.Counter())
		}
		if !afflicted && !clock.HasCounter() && !clock.HasPendingReduction() {
			StopPoisonEffect(entity)
		}
		if !pulse || !afflicted {
			SyncPoisonOrb(entity)
			return
		}
	}
	if clock == nil {
		c.poisonTicksUntilHit--
		if c.poisonTicksUntilHit > 0 {
			return
		}
	}
	currentValue := entity.PoisonDamage()
	if currentValue <= 0 {
		StopPoisonEffect(entity)
		SyncPoisonOrb(entity)
		return
	}
	if entity.IsVenomed() {
		// OSRS venom: the stored value IS the damage dealt
		// (no severity->damage formula), escalates by 2
		// each hit up to a cap of 20, and does not decay or
		// expire on its own — only a cure ends it.
		generation := entity.VenomGeneration()
		hit := NewHitDamage(currentValue, HitMaskGreen).OnApplied(func(damageTaken int) {
			if damageTaken > 0 && entity.IsRegistered() && entity.IsVenomed() &&
				entity.VenomGeneration() == generation {
				entity.SetLastVenomDamage(damageTaken)
			}
		})
		entity.GetCombat().HitQueue().AddPendingDamage([]*HitDamage{hit})
		next := currentValue + 2
		if next > 20 {
			next = 20
		}
		entity.SetPoisonDamage(next)
		c.poisonTicksUntilHit = PoisonClockPeriodTicks
		SyncPoisonOrb(entity)
		return
	}
	poisonDamage := DamageFromSeverity(currentValue)
	entity.GetCombat().HitQueue().AddPendingDamage([]*HitDamage{NewHitDamage(poisonDamage, HitMaskGreen)})
	entity.SetPoisonDamage(currentValue - 1)
	c.poisonTicksUntilHit = PoisonClockPeriodTicks
	if currentValue <= 1 {
		if clock == nil || !clock.HasCounter() {
			StopPoisonEffect(entity)
		}
	}
	SyncPoisonOrb(entity)
}

// sendPoisonResistanceLine sends one resistance line
// (CombatPoisonEffect.sendResistanceLine): the purple lines
// one pulse (30 ticks) before and at each end
// (rsprox_transcripts/223/00014.txt ticks 1748/1778 for
// poison, 224/01443.txt ticks 151/181 for venom).
func sendPoisonResistanceLine(player *Player, counter int) {
	var line string
	switch counter {
	case 1:
		line = "Your poison resistance is about to wear off."
	case 0:
		line = "Your poison resistance has worn off."
	case PoisonClockVenomCutoff + 1:
		line = "Your venom resistance is about to wear off."
	case PoisonClockVenomCutoff:
		line = "Your venom resistance has worn off, but you are still resistant to poison."
	default:
		return
	}
	player.SendMessage("<col=7f007f>" + line + "</col>")
}

// sendPoisonReductionLines sends the reduction lines
// (CombatPoisonEffect.sendReductionLines): an exhausted tier
// gets its end line even when subtraction skipped its
// counter; a warning only describes a tier that still
// exists at the warning boundary.
func sendPoisonReductionLines(player *Player, before, after int) {
	if before > PoisonClockVenomCutoff && after <= PoisonClockVenomCutoff {
		sendPoisonResistanceLine(player, PoisonClockVenomCutoff)
	} else if before > PoisonClockVenomCutoff+1 && after == PoisonClockVenomCutoff+1 {
		sendPoisonResistanceLine(player, after)
	}
	if before > 0 && after == 0 {
		sendPoisonResistanceLine(player, 0)
	} else if before > 1 && after == 1 {
		sendPoisonResistanceLine(player, 1)
	}
}

// hasInterruptibleInterface reports whether the player has
// an interruptible interface open (PacketSender.ts:976-979):
// the modal half of CombatPoisonEffect.execute. A nil
// session reads as nothing open.
func (p *Player) hasInterruptibleInterface() bool {
	sender := p.interfaceRuntimeSender()
	return sender != nil && sender.HasInterruptibleInterface()
}

// ---- Save / restore (PlayerSave.ts applyToPlayer / fromPlayer) ----

// PoisonSave is the live poison state a snapshot writes
// (PlayerSave.ts:765-770, fromPlayer): the affliction
// fields, the clock snapshot (present only while poisoned
// or protected), and the immunity seconds. It mirrors the
// persist save fields (poisonDamage, venomed,
// lastVenomDamage, poisonClock, poisonImmunityTimer)
// without importing persist (see doc.go layering); the
// save wiring converts field-for-field.
type PoisonSave struct {
	// Damage is the live poison severity / venom damage.
	Damage int
	// Venomed reports whether the affliction is venom.
	Venomed bool
	// LastVenomDamage is present only while venomed.
	LastVenomDamage *int
	// Clock is present only while poisoned or protected.
	Clock *PoisonClockSave
	// PoisonImmunitySeconds is the immunity timer's
	// remaining seconds (for a player, the clock's
	// secondsRemaining; for an NPC, its immunity timer).
	PoisonImmunitySeconds float64
}

// PoisonSaveState returns the live poison state for a
// snapshot (fromPlayer, PlayerSave.ts:765-770). The clock
// snapshot is included only when the entity is poisoned or
// the clock is not finished, exactly as TS writes it.
func (c *Combat) PoisonSaveState() PoisonSave {
	if c == nil || c.owner == nil {
		return PoisonSave{}
	}
	entity := c.owner
	save := PoisonSave{
		Damage:                entity.PoisonDamage(),
		Venomed:               entity.IsVenomed(),
		PoisonImmunitySeconds: c.PoisonImmunitySecondsRemaining(),
	}
	if entity.IsVenomed() {
		if last := entity.LastVenomDamage(); last != nil {
			value := *last
			save.LastVenomDamage = &value
		}
	}
	if player := combatantFor(entity).Player(); player != nil {
		clock := player.GetCombat().PoisonClock()
		if entity.IsPoisoned() || !clock.Finished() {
			snapshot := clock.Snapshot()
			save.Clock = &snapshot
		}
	}
	return save
}

// RestorePoisonFromSave restores the live poison state from
// a carried save (applyToPlayer, PlayerSave.ts:655-660, in
// TS order): the affliction fields, then the clock restore
// with the legacy immunity seconds, then the effect resume
// (onLogin, Player.ts:801, resumes it). A nil clock state
// restores nothing but still applies the legacy seconds.
// NPCs have no saves; the immunity seconds apply to the
// NPC immunity timer instead (see PoisonImmunitySeconds).
func (c *Combat) RestorePoisonFromSave(damage int, venomed bool, lastVenomDamage *int, clockState *PoisonClockSave, legacyImmunitySeconds float64) {
	if c == nil || c.owner == nil {
		return
	}
	entity := c.owner
	entity.SetPoisonDamage(damage)
	entity.SetVenomed(venomed)
	if lastVenomDamage != nil {
		entity.SetLastVenomDamage(*lastVenomDamage)
	}
	if player := combatantFor(entity).Player(); player != nil {
		player.GetCombat().PoisonClock().Restore(clockState, legacyImmunitySeconds)
		if entity.IsPoisoned() || player.GetCombat().PoisonClock().HasCounter() || player.GetCombat().PoisonClock().HasPendingReduction() {
			ResumePoisonEffect(entity)
		}
	} else if legacyImmunitySeconds > 0 {
		c.StartPoisonImmunity(int(legacyImmunitySeconds))
	}
}

// PoisonImmunitySecondsRemaining is the NPC poison-immunity
// timer's remaining seconds — and, for a player, the clock's
// secondsRemaining (Combat.ts:805-806: getPoisonImmunityTimer
// returns the clock for a player, a SecondsTimer otherwise).
// It feeds OrbValue and the save's poisonImmunityTimer.
func (c *Combat) PoisonImmunitySecondsRemaining() float64 {
	if c == nil {
		return 0
	}
	if player := combatantFor(c.owner).Player(); player != nil {
		return player.GetCombat().PoisonClock().SecondsRemaining()
	}
	if c.poisonImmunityTimer == nil {
		return 0
	}
	return float64(c.poisonImmunityTimer.SecondsRemaining())
}

// StartPoisonImmunity starts the NPC poison-immunity timer
// (the SecondsTimer getPoisonImmunityTimer returns for an
// NPC). Core never starts it — no core path grants an NPC
// antipoison immunity — but the timer exists for content and
// for the orb's NPC branch.
func (c *Combat) StartPoisonImmunity(seconds int) {
	if c == nil {
		return
	}
	if c.poisonImmunityTimer == nil {
		c.poisonImmunityTimer = newSecondsTimer()
	}
	c.poisonImmunityTimer.Start(seconds)
}

// ---- Hit-pipeline application (CombatFactory.executeHit) ----

// applyPoisonOnHit applies poison/venom from one landed hit
// (CombatFactory.ts:1086-1145, executeHit, after the resolved
// event and before recoil/vengeance). Only damaging hits
// (damage > 0) can poison. The melee roll and the ranged roll
// are different — the plan's "1-in-8 roll at 1096" is the
// RANGED roll (getRandom(7) === 0); melee rolls
// getMeleeRollRange (1 in 4, 1 in 3 for the noxious
// halberd). The NPC branch fires the combat:npc-poison
// custom event first.
func applyPoisonOnHit(attacker, target *Mobile, hit *PendingHit) {
	if attacker == nil || target == nil || hit == nil {
		return
	}
	if hit.TotalDamage() <= 0 {
		return
	}
	if combatantFor(attacker).IsPlayer() {
		player := combatantFor(attacker).Player()
		if player == nil {
			return
		}
		var poisonType PoisonType
		var ok bool
		var ranged bool
		var apply bool
		switch hit.CombatType() {
		case CombatTypeMelee:
			var weapon Item
			if equipment := player.Equipment(); equipment != nil {
				weapon = equipment.GetWeapon()
			}
			apply = randomInclusive(GetMeleeRollRange(weapon.ID())) == 0
			poisonType, ok = PoisonTypeForItem(weapon)
		case CombatTypeRanged:
			apply = randomInclusive(7) == 0
			ranged = true
			// A ranged shot poisons with what it was fired
			// with: its last arrow or knife is no longer
			// worn. Only a weapon that fires from the ammo
			// slot uses it (not a crystal bow or
			// chinchompas with (p) ammo left there).
			projectile := rangedPoisonProjectile(player, hit)
			if projectile != nil {
				// The toxic blowpipe rolls its own venom
				// (ToxicBlowpipe plugin, 1 in 4 on the
				// Wiki), so the core site never poisons
				// with it. TS checks the projectile id
				// (CombatFactory.ts:1105-1108): the
				// plugin resolves the blowpipe's
				// ammunition, which makes
				// firesFromAmmoSlot false and the
				// projectile the blowpipe itself
				// (PendingHit.ts:132-135). Go has no
				// plugin ammunition (ranged_data.go), so
				// the captured projectile is the dart —
				// the weapon id carries the exclusion
				// here instead. Either id matching
				// excludes the shot.
				weaponID := 0
				if fired := hit.FiredRangedShot(); fired != nil && fired.Weapon != nil {
					weaponID = fired.Weapon.ID()
				}
				if weaponID != ToxicBlowpipeID && projectile.ID() != ToxicBlowpipeID {
					poisonType, ok = PoisonTypeForItem(*projectile)
				}
			}
		default:
			return
		}
		if apply && ok {
			severity := GetMeleeSeverity(poisonType)
			if ranged {
				severity = GetRangedSeverity(poisonType)
			}
			PoisonEntity(target, severity, poisonOrbType(poisonType))
		}
		return
	}
	// NPC attacker: a poisonous or venomous definition
	// poisons on Misc.getRandom(10) <= 5 (6 in 11).
	def := combatantFor(attacker).NPCDef()
	if def == nil {
		return
	}
	venomous := def.IsVenomous()
	if !def.IsPoisonous() && !venomous {
		return
	}
	if randomInclusive(10) > 5 {
		return
	}
	// Venom stores its starting damage rather than a
	// severity, and escalates from there — see
	// getMeleeSeverity.
	severity := 30
	if venomous {
		severity = 6
	}
	payload := &NpcPoisonPayload{
		Attacker: attacker,
		Target:   target,
		Severity: severity,
		Venomous: venomous,
	}
	emitCustomEvent(attacker, target, "combat:npc-poison", payload)
	if !payload.Handled && payload.Severity > 0 {
		PoisonEntity(target, payload.Severity, poisonOrbTypeForVenom(payload.Venomous))
	}
}

// poisonOrbType returns the poisonEntity orb type for a
// poison kind: 2 for venom, 1 otherwise.
func poisonOrbType(typ PoisonType) int {
	if typ == PoisonTypeVenom {
		return 2
	}
	return 1
}

// poisonOrbTypeForVenom returns 2 for a venomous attacker,
// 1 otherwise.
func poisonOrbTypeForVenom(venomous bool) int {
	if venomous {
		return 2
	}
	return 1
}

// rangedPoisonProjectile returns the item a ranged hit was
// fired with (CombatFactory.ts:1099-1108): the fired shot's
// projectile when the hit captured one, else the stand-in
// branch — the weapon slot for a thrown weapon (DART,
// KNIFE, THROWNAXE, JAVELIN), the ammunition slot
// otherwise. Nil when the slot held nothing valid.
func rangedPoisonProjectile(player *Player, hit *PendingHit) *Item {
	if hit != nil {
		if fired := hit.FiredRangedShot(); fired != nil && fired.Projectile != nil {
			return fired.Projectile
		}
	}
	equipment := player.Equipment()
	if equipment == nil {
		return nil
	}
	iface := ""
	if hit != nil {
		if fired := hit.FiredRangedShot(); fired != nil {
			iface = fired.WeaponInterface
		}
	}
	if iface == "" {
		if src, ok := player.ItemDefinitions().(WeaponInfoSource); ok && src != nil {
			iface = src.WeaponInterfaceName(equipment.GetWeapon().ID())
		}
	}
	thrown := iface == "DART" || iface == "KNIFE" || iface == "THROWNAXE" || iface == "JAVELIN"
	slot := AmmunitionSlot
	if thrown {
		slot = WeaponSlot
	}
	item := equipment.ItemAt(slot)
	if !item.IsValid() {
		return nil
	}
	return &item
}
