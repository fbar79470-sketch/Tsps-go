// Combat state for a Mobile: the port of
// game/content/combat/Combat.ts (1217 lines) — target and
// attacker state, attack-cycle timing, the pre/post
// movement phases, attack initiation and execution, the
// damage map, retaliation scheduling, animation stalls, and
// the special/spell/ranged slots.
//
// Identity: TS reads kind, hitpoints, size, skills, and
// equipment off Mobile itself. Go's Mobile carries none of
// those, and entity.go / player.go / npc.go are owned by
// other work, so the fighter's identity lives on Combat
// (the Combatant adapter, combatant.go), bound by the
// typed GetCombat shadows below. Every combat entry point
// that receives a *Player or *NPC must call the TYPED
// GetCombat (Player.GetCombat / NPC.GetCombat) at least
// once — spawn/login wiring, listeners, drivers — or the
// fighter reads as the fail-closed unknown adapter (0 HP,
// dying) and ValidTarget rejects it. Bind is the escape
// hatch for code that builds a Combatant directly.
//
// Cycle source: the process phases take no cycle argument
// (the fixed stub signatures), so combat reads the cycle
// through combatCycle (combatant.go): the coordinator wires
// SetCombatCycleSource to World.ProcessCycle at boot, and
// the last cycle seen by ProcessHitQueue is the fallback.
//
// Deviations from TS, each commented at its site:
//   - TaskManager.cancelTasks on attack, MovementTask busy
//     checks, and the 1-tick retaliation task: no
//     TaskManager access from combat; retaliation uses
//     combat-local pending state with the same timing.
//   - setFollowing / faceMobileBriefly / markNpcCombatActive
//     / sendAttackTimer / animations: the receiving systems
//     do not exist in Go (following exists for NPCs only
//     and is called); the calls are no-ops.
//   - Boat-deck, CrossFloor, private-area, duel, and
//     trading branches: degenerate (same floor, no decks,
//     no private areas, no duels).
//   - Poison/fire/teleblock timers: the poison system is
//     ported (poison.go: PoisonClock, CombatPoisonEffect,
//     poisonEntity, orb varp 102); the fire and teleblock
//     timers are still cycle-stamped / not ported, and the
//     prayer-block timer is cycle-stamped here.
//   - setPursuitCheckpoint / ignoreOccupantThisCycle: no Go
//     equivalents; pursuit uses AddCheckpoint, and the
//     step-off path cannot ignore the target's occupancy.
package game

// CombatTargetPlayerVarp is COMBAT_TARGET_PLAYER_VARP
// (1075): the target's player index, or -1.
const CombatTargetPlayerVarp = 1075

// UnreachableGiveUpCycles is UNREACHABLE_GIVE_UP_CYCLES
// (20): cycles of no progress toward an unreachable target
// before the engagement ends.
const UnreachableGiveUpCycles = 20

// MaxAnimationStallTicks is MAX_ANIMATION_STALL_TICKS (5):
// the cap on a measured swing-animation stall.
const MaxAnimationStallTicks = 5

// MaxContentAnimationStallTicks is
// MAX_CONTENT_ANIMATION_STALL_TICKS (100): the cap on a
// content-declared stall.
const MaxContentAnimationStallTicks = 100

// AttackerExpiryTicks is the attacker window:
// lastAttack.elapsedTime(6000) — 10 ticks.
const AttackerExpiryTicks = 10

// Combat holds one Mobile's combat state. TS Combat.ts:60.
type Combat struct {
	owner    *Mobile
	identity Combatant

	hitQueue    *HitQueue
	damageMap   map[*Player]*HitDamageCache
	lastDamager *Player
	// lastAttackCycle stamps the last applyAttacker call
	// (TickStopwatch.reset); the attacker expires 10 ticks
	// after it.
	lastAttackCycle int64

	nextAttackCycle int64
	target          *Mobile
	autoRetaliating bool
	attacker        *Mobile
	// lastAttackedCycle / lastAttackedByPlayerCycle: -1
	// when cleared / never.
	lastAttackedCycle         int64
	lastAttackedByPlayerCycle int64

	specialAttackQueued bool
	method              CombatMethod
	castSpell           CombatSpell
	autoCastSpell       CombatSpell
	previousCast        CombatSpell

	generation                int
	deniedNoticeGeneration    int
	foughtNpcNoticeGeneration int
	foughtNpcNoticeCycle      int64

	routedTargetX int
	routedTargetY int
	routedTargetZ int

	lastPreMovementCycle  int64
	lastPostMovementCycle int64
	// manualMovementUntilCycle: content-owned movement
	// window; -1 when none.
	manualMovementUntilCycle int64

	unreachableCycles   int
	unreachableDistance int

	// Animation stall: untilCycle + kind + holdsHits. A
	// partial stall only withholds steps client-side; a
	// full stall holds clicks, movement, and (when
	// holdsHits) incoming hits.
	stallUntilCycle int64
	stallFull       bool
	stallHoldsHits  bool
	stallSet        bool
	// measureAnimationCycle: the cycle a swing was armed
	// on, or -1.
	measureAnimationCycle int64

	cycleState *CombatCycleState

	rangedWeapon *RangedWeaponData
	ammunition   *AmmunitionData

	fightType *FightType
	prayer    *PrayerState

	// dying: the player death flag (NPCs use NPC.Dying).
	// Read through Combatant.Dying.
	dying bool
	// untargetable: the Mobile.untargetable flag, stored
	// here (no such field exists on Go's Mobile).
	untargetable bool
	// interacting: the setMobileInteraction target (the
	// sync adapter reads it through Interacting).
	interacting *Mobile

	vengeance        bool
	autoRetaliate    bool
	specialActivated bool
	// specialPercentage: special attack energy, 0..100.
	// TS stores it on Mobile; Go stores it here (no
	// editable field exists on Player).
	specialPercentage int

	// prayerBlockUntilCycle: the prayer-block timer
	// (SecondsTimer in TS), cycle-stamped. 0 = finished.
	prayerBlockUntilCycle int64

	// poisonClock is the player's shared status pulse
	// (PoisonClock, Combat.ts:808-812 getPoisonClock,
	// lazily created by PoisonClock()). The effect reads
	// the clock only for players; NPC immunity uses
	// poisonImmunityTimer below.
	poisonClock *PoisonClock
	// poisonImmunityTimer is the NPC poison-immunity timer
	// (Combat.ts:805-806: getPoisonImmunityTimer returns a
	// SecondsTimer for NPCs, the clock for players).
	poisonImmunityTimer *SecondsTimer
	// poisonEffectRunning / poisonTicksUntilHit /
	// poisonLastTickCycle / poisonSuspended are the
	// CombatPoisonEffect runtime (poison.go): the effect
	// ticks from ProcessHitQueue once per cycle, with the
	// running state here as the Go form of the TS
	// WeakMap<Mobile, CombatPoisonEffect> active map.
	poisonEffectRunning bool
	poisonTicksUntilHit int
	poisonLastTickCycle int64
	poisonSuspended     bool
	// poisonLastOrb / poisonLastOrbSet are the last orb
	// varp 102 value sent (Player.ts:465-473
	// syncPoisonOrb); the first tick always sends.
	poisonLastOrb    int
	poisonLastOrbSet bool

	lastAttackTarget *Mobile
	sentAttackCycle  int64

	pendingRetaliation *pendingRetaliation
	delayedDamage      []delayedDamageEntry

	// Death state machine (combat_death.go): TS runs death
	// tasks; Go advances this state from ProcessHitQueue.
	deathStarted      bool
	deathStartCycle   int64
	deathTicks        int
	deathDispatched   bool
	deathAdvanceCycle int64
	deathMessage      string
	deathAnnounce     bool
	// deathKiller / deathDamagers: captured at death start
	// (the death tasks capture before getKiller clears the
	// record).
	deathKiller   *Player
	deathDamagers []*Player

	// remainsNPC / remainsTicksLeft / remainsAdvanceCycle: the
	// deferred removal for a death whose hook set Remains
	// (NPCDeathTask.ts:82-88, NPCRemainsTask: the NPC stays
	// registered for remains.ticks, then remove() runs).
	// dispatchNpcDeath fills them instead of removing at
	// dispatch; advanceDeath counts them down from
	// ProcessHitQueue and removes at zero. The respawn is
	// still scheduled at dispatch (BeginRespawnWithTicks),
	// per the Phase 33 plan — TS schedules it at remove()
	// time, a divergence the plan accepts so the existing
	// respawn timing is unchanged.
	remainsNPC          *NPC
	remainsTicksLeft    int
	remainsAdvanceCycle int64

	// Tick hits (combat_death.go): the hits that landed
	// this cycle, for the sync adapter. Cleared when a new
	// cycle's ProcessHitQueue runs.
	tickHitCycle int64
	tickHits     []*HitDamage

	// baseExperienceOwed: a cast whose base XP hits() has
	// not paid yet (MagicCombatMethod.baseExperienceOwed).
	baseExperienceOwed bool
}

// CombatCycleState is CombatCycleState (Combat.ts): the
// pre-movement snapshot postMovementProcess validates
// against.
type CombatCycleState struct {
	cycle           int64
	generation      int
	target          *Mobile
	method          CombatMethod
	targetX         int
	targetY         int
	targetZ         int
	routeRequested  bool
	attackBlocked   bool
	mobilityBlocked bool
	skipPost        bool
}

// pendingRetaliation is one scheduled counter-attack: the
// Go form of the 1-tick TaskManager task in
// handleRetaliation. The checks re-run at fire time.
type pendingRetaliation struct {
	target     *Mobile
	dueCycle   int64
	attackable func() bool
	playerBusy func() bool
}

// delayedDamageEntry is one per-splat delayed damage
// (applyResolvedHitDamage): the Go form of the TS
// TaskManager task, fired from ProcessHitQueue when the
// incoming generation still matches.
type delayedDamageEntry struct {
	hit        *HitDamage
	dueCycle   int64
	generation int
}

// NewCombat returns the combat state for owner. TS constructs
// it in the Mobile constructor (Mobile.ts:146); in Go it is
// allocated lazily by GetCombat.
func NewCombat(owner *Mobile) *Combat {
	c := &Combat{
		owner:                     owner,
		lastAttackedCycle:         -1,
		lastAttackedByPlayerCycle: -1,
		deniedNoticeGeneration:    -1,
		foughtNpcNoticeGeneration: -1,
		foughtNpcNoticeCycle:      -1,
		routedTargetX:             -1 << 30,
		routedTargetY:             -1 << 30,
		routedTargetZ:             -1 << 30,
		lastPreMovementCycle:      -1,
		lastPostMovementCycle:     -1,
		manualMovementUntilCycle:  -1,
		unreachableDistance:       -1,
		measureAnimationCycle:     -1,
		fightType:                 UnarmedPunch,
		autoRetaliate:             true,
		specialPercentage:         100,
		tickHitCycle:              -1,
		deathAdvanceCycle:         -1,
		remainsAdvanceCycle:       -1,
		poisonTicksUntilHit:       PoisonClockPeriodTicks,
		poisonLastTickCycle:       -1,
	}
	c.hitQueue = NewHitQueue(owner)
	return c
}

// GetCombat returns the Mobile's combat state, allocating it
// on first use. Mobile.ts:681 (getCombat). Never returns nil
// for a non-nil Mobile, and always returns the same Combat.
func (m *Mobile) GetCombat() *Combat {
	if m.combat == nil {
		m.combat = NewCombat(m)
	}
	return m.combat
}

// GetCombat returns the Player's combat state, binding the
// player identity on first use (see the file header). It
// shadows Mobile.GetCombat for *Player receivers.
func (p *Player) GetCombat() *Combat {
	c := (&p.Mobile).GetCombat()
	if c.identity == nil {
		c.identity = playerCombatant{p: p}
	}
	return c
}

// GetCombat returns the NPC's combat state, binding the NPC
// identity on first use and installing the movement
// coordinator's combat seams (inCombatCheck, retreatReset)
// when unset. It shadows Mobile.GetCombat for *NPC
// receivers.
func (n *NPC) GetCombat() *Combat {
	c := (&n.Mobile).GetCombat()
	if c.identity == nil {
		c.identity = npcCombatant{n: n}
		if coordinator := n.MovementCoordinator(); coordinator != nil {
			if coordinator.inCombatCheck == nil {
				coordinator.inCombatCheck = func(npc *NPC) bool { return InCombat(&npc.Mobile) }
			}
			if coordinator.retreatReset == nil {
				coordinator.retreatReset = func(npc *NPC) { npc.GetCombat().Reset() }
			}
		}
	}
	return c
}

// Bind installs an identity directly, for code that builds
// a Combatant without going through a typed GetCombat.
func (c *Combat) Bind(identity Combatant) {
	if c != nil {
		c.identity = identity
	}
}

// Owner returns the Mobile this Combat belongs to.
func (c *Combat) Owner() *Mobile { return c.owner }

// HitQueue returns the owner's hit queue.
func (c *Combat) HitQueue() *HitQueue { return c.hitQueue }

// PoisonClock returns the owner's shared status pulse
// (Combat.ts:808-812, getPoisonClock), creating it lazily.
// The ensure callback resumes the poison effect
// (CombatPoisonEffect.resume, Combat.ts:811), mirroring the
// TS lazy require inside getPoisonClock.
func (c *Combat) PoisonClock() *PoisonClock {
	if c.poisonClock == nil {
		owner := c.owner
		c.poisonClock = NewPoisonClock(func() { ResumePoisonEffect(owner) })
	}
	return c.poisonClock
}

// ---- Attack initiation ----

// Attack starts an attack on target (Combat.attack): the
// target is set, the method and cycle state clear, the
// movement queue resets, the fighter faces the target, and
// a player client learns the target index (varp 1075). An
// NPC that does not fight back never attacks. Retaliation
// uses the same path with autoRetaliation set (internal).
func (c *Combat) Attack(target *Mobile) {
	c.attack(target, false)
}

func (c *Combat) attack(target *Mobile, autoRetaliation bool) {
	if target == nil {
		return
	}
	fighter := combatantFor(c.owner)
	if npc := fighter.NPC(); npc != nil {
		if d := fighter.NPCDef(); d != nil && !d.DoesFightBack() {
			return
		}
	}
	previousTarget := c.target
	if previousTarget != target {
		c.manualMovementUntilCycle = -1
		c.resetUnreachableTracking()
	}
	if previousTarget != nil && previousTarget != target && c.method != nil {
		c.method.OnCombatEnded(c.owner)
	}
	c.generation++
	c.target = target
	c.autoRetaliating = autoRetaliation
	c.method = nil
	c.cycleState = nil
	// TaskManager.cancelTasks: skipped (no TaskManager
	// access from combat).
	if queue := fighter.MovementQueue(); queue != nil {
		queue.Reset(true)
		if autoRetaliation {
			queue.EndClickRunOverride()
		}
	}
	// setFollowing(null): NPCs have following; players do
	// not (no following system), so only NPCs clear it.
	if npc := fighter.NPC(); npc != nil {
		npc.SetFollowing(nil)
	}
	c.faceTarget(target, true)
	if player := fighter.Player(); player != nil {
		index := -1
		if combatantFor(target).IsPlayer() {
			index = target.GetIndex()
		}
		player.SendVarp(CombatTargetPlayerVarp, index)
	}
	// World.markNpcCombatActive: skipped (no such registry
	// in Go; NPC.Process derives combat activity from
	// HasPendingWork).
}

// CastSpellOn sets the cast spell and attacks (Combat.castSpellOn).
func (c *Combat) CastSpellOn(target *Mobile, spell CombatSpell) {
	if target == nil || spell == nil {
		return
	}
	c.SetCastSpell(spell)
	c.Attack(target)
}

// Target returns the Mobile this Combat is currently
// attacking, or nil.
func (c *Combat) Target() *Mobile { return c.target }

// SetTarget attacks target, or resets when target is nil
// (Combat.setTarget).
func (c *Combat) SetTarget(target *Mobile) {
	if target == nil {
		c.Reset()
	} else {
		c.Attack(target)
	}
}

// IsAutoRetaliating reports whether the current fight began
// as a reply to an attack.
func (c *Combat) IsAutoRetaliating() bool { return c.autoRetaliating }

// StopAutoRetaliation ends a retaliation fight
// (Combat.stopAutoRetaliation).
func (c *Combat) StopAutoRetaliation() {
	if c.autoRetaliating || c.target == c.attacker {
		c.Reset()
	}
}

// PreserveMovementForTicks keeps content-owned movement for
// ticks (default 1): the fighter faces where it walks until
// the window ends (Combat.preserveMovementForTicks).
func (c *Combat) PreserveMovementForTicks(ticks ...int) {
	n := 1
	if len(ticks) > 0 {
		n = ticks[0]
	}
	if n < 0 {
		n = 0
	}
	c.manualMovementUntilCycle = combatCycle() + int64(n) - 1
}

// ---- Tick phases ----

// HasPendingWork reports whether this Combat has anything
// to do this tick: a target, an attacker, or queued hits.
// Combat.ts:567-568.
func (c *Combat) HasPendingWork() bool {
	return c.target != nil || c.attacker != nil || c.hitQueue.HasPendingWork()
}

// ProcessHitQueue lands queued impacts for this cycle,
// before the owner acts (NPC.ts:308 calls it first in
// process). It also stamps the fallback cycle, advances
// the death state machine, fires due delayed damage, and
// clears last cycle's tick hits. The poison effect ticks
// FIRST: TS runs it in the task-manager phase, before the
// process_players / process_npcs phases, so poison damage
// queued this tick lands this tick (see poison.go).
func (c *Combat) ProcessHitQueue(cycle int64) {
	lastHitQueueCycle = cycle
	if c.tickHitCycle != cycle {
		c.tickHitCycle = cycle
		c.tickHits = nil
	}
	c.processPoisonEffect(cycle)
	c.hitQueue.Process(cycle)
	c.processDelayedDamage(cycle)
	c.advanceDeath(cycle)
}

// PreMovementProcess runs the combat step before movement:
// target validation, retreat, permission, facing, reach,
// pursuit routing. Combat.ts preMovementProcess.
func (c *Combat) PreMovementProcess() {
	cycle := combatCycle()
	if c.lastPreMovementCycle == cycle {
		return
	}
	c.lastPreMovementCycle = cycle

	// Only the attacker expires here: the logout timer runs
	// on to its own 16 ticks. Before the scripted-walk
	// return, so a long plugin walk still lets it expire.
	if c.attacker != nil && cycle-c.lastAttackCycle >= AttackerExpiryTicks {
		c.clearAttacker()
	}

	c.checkPendingRetaliation(cycle)

	// A plugin scripted this NPC's walk; it owns the
	// movement queue and the attack pauses until the walk
	// finishes.
	if npc := combatantFor(c.owner).NPC(); npc != nil && npc.IsScriptedMovement() {
		c.cycleState = nil
		return
	}

	target := c.target
	if target == nil {
		c.cycleState = nil
		return
	}
	generation := c.generation
	location := target.Location()
	method := GetMethod(c.owner)
	state := &CombatCycleState{
		cycle:      cycle,
		generation: generation,
		target:     target,
		method:     method,
		targetX:    location.X,
		targetY:    location.Y,
		targetZ:    location.Z,
	}
	c.cycleState = state
	c.method = method

	if !ValidTarget(c.owner, target) {
		c.cancelIfCurrent(target, generation)
		return
	}
	if c.shouldRetreat(method) {
		c.cancelIfCurrent(target, generation)
		return
	}

	permission := CanAttackPermission(c.owner, target, true, method)
	if permission != CanAttackOK && c.walksInOnFoughtNpc(permission, target, method, false) {
		state.attackBlocked = true
	} else if permission != CanAttackOK {
		c.handleAttackDenied(permission, target, generation)
		if !c.isTransientDenial(permission) || c.target != target || c.generation != generation {
			return
		}
		// Keep walking in; postMovementProcess re-checks
		// before landing a hit.
		state.attackBlocked = true
	}

	if c.manualMovementUntilCycle >= cycle {
		// Walking where content sent them: they face where
		// they walk, and turn back to the target once it
		// ends.
		if !HasCombatFlag(c.owner, CombatFlagKeepFacing) {
			c.setMobileInteraction(nil)
		}
		return
	}

	c.faceTarget(target, false)

	fighter := combatantFor(c.owner)
	targetFighter := combatantFor(target)
	reachable := CanReach(fighter, method, targetFighter)
	// Boat-deck branch: skipped (no sailing / deck logic).
	if reachable {
		c.resetUnreachableTracking()
	} else if c.giveUpUnreachableTarget(target, generation) {
		return
	}

	// An NPC over its target can't reach it: it steps off
	// below.
	if fighter.NPC() != nil && OverlapsEntities(fighter, targetFighter) && !reachable {
		c.processNpcUnderTarget(state)
		return
	}

	if reachable {
		// An NPC flagged combat:close-in keeps walking in
		// while it attacks from range, until it stands
		// beside its target.
		closesIn := fighter.NPC() != nil && HasCombatFlag(c.owner, CombatFlagCloseIn) &&
			!Beside(fighter, targetFighter)
		if !closesIn {
			if queue := fighter.MovementQueue(); queue != nil {
				queue.Reset(true)
			}
			return
		}
	}

	movement := fighter.MovementQueue()
	if movement == nil || !movement.getMobility().canMove() {
		state.mobilityBlocked = true
		// A frozen player's attack on a target out of reach
		// ends there, still turned to the target for that
		// tick.
		if fighter.IsPlayer() && c.owner.GetTimers().Has(TimerKeyFreeze) {
			if !fighter.IsPlayerBot() {
				fighter.SendMessage("I can't reach that!")
			}
			current := c.target == target && c.generation == state.generation
			c.cancelIfCurrent(target, state.generation)
			if current {
				// faceMobileBriefly: skipped (no brief-face
				// system); the interaction set above by
				// faceTarget stands for this tick.
			}
		}
		return
	}

	if fighter.NPC() != nil {
		movement.AddCheckpoint(naiveEntityDestination(c.owner, target))
		state.routeRequested = true
	} else if c.shouldRouteToward(movement, location, target) {
		state.routeRequested = true
		c.routedTargetX = location.X
		c.routedTargetY = location.Y
		c.routedTargetZ = location.Z
		CombatRoute(fighter, targetFighter, method)
	}
}

// shouldRouteToward is Combat.shouldRouteToward: a path is
// only recalculated near its final checkpoint. An empty
// queue always routes; an invalidated route always routes;
// a queue longer than one step never refreshes; otherwise
// a moved target triggers a refresh. The cross-floor rule
// degenerates (same floor always).
func (c *Combat) shouldRouteToward(movement *MovementQueue, targetLocation Location, target *Mobile) bool {
	if movement.Size() == 0 || movement.WasRouteInvalidated() {
		return true
	}
	if movement.Size() > 1 {
		return false
	}
	return c.routedTargetX != targetLocation.X ||
		c.routedTargetY != targetLocation.Y ||
		c.routedTargetZ != targetLocation.Z
}

// PostMovementProcess runs the combat step after movement:
// landing hits in range. Combat.ts postMovementProcess.
func (c *Combat) PostMovementProcess() {
	cycle := combatCycle()
	if c.lastPostMovementCycle == cycle {
		return
	}
	c.lastPostMovementCycle = cycle

	state := c.cycleState
	if state == nil || state.cycle != cycle || state.skipPost ||
		c.target != state.target || c.generation != state.generation {
		return
	}
	target, method, generation := state.target, state.method, state.generation
	if !ValidTarget(c.owner, target) {
		c.cancelIfCurrent(target, generation)
		return
	}

	permission := CanAttackPermission(c.owner, target, true, method)
	if permission != CanAttackOK {
		// Walking in on an NPC someone else is fighting: a
		// step that arrived this cycle is answered on the
		// next.
		stepped := false
		if queue := combatantFor(c.owner).MovementQueue(); queue != nil {
			stepped = queue.DidMoveThisCycle()
		}
		if !c.walksInOnFoughtNpc(permission, target, method, stepped) {
			c.handleAttackDenied(permission, target, generation)
			return
		}
		if CanReach(combatantFor(c.owner), method, combatantFor(target)) {
			return
		}
	}
	if state.attackBlocked && CanReach(combatantFor(c.owner), method, combatantFor(target)) {
		// Denied before movement but in range now; the next
		// cycle re-evaluates.
		return
	}

	if !CanReach(combatantFor(c.owner), method, combatantFor(target)) {
		location := target.Location()
		targetMoved := state.targetX != location.X ||
			state.targetY != location.Y ||
			state.targetZ != location.Z
		movement := combatantFor(c.owner).MovementQueue()
		movedThisCycle := movement != nil && movement.DidMoveThisCycle()
		if targetMoved || state.mobilityBlocked || movedThisCycle {
			return
		}
		if combatantFor(c.owner).NPC() != nil && state.routeRequested &&
			movement != nil && !movement.WasBlockedByDynamicOccupancy() {
			if !c.isSafespotted(target) {
				c.cancelIfCurrent(target, generation)
			}
			return
		}
		if movement != nil && movement.Size() > 0 {
			return
		}
		if combatantFor(c.owner).IsPlayer() && movement != nil &&
			(state.routeRequested || movement.WasRouteEvaluated()) {
			if !combatantFor(c.owner).IsPlayerBot() {
				combatantFor(c.owner).SendMessage("I can't reach that!")
			}
			c.cancelIfCurrent(target, generation)
		}
		return
	}

	if queue := combatantFor(c.owner).MovementQueue(); queue != nil {
		queue.Reset(true)
	}
	// A bypassing special must reach executeAttack while
	// the old cooldown is still active.
	special := c.GetCombatSpecial()
	traits := ActiveTraitsFor(combatantFor(c.owner))
	bypass := special != nil && special.Method == method &&
		traits != nil && traits.BypassAttackDelay
	if cycle < c.nextAttackCycle && !bypass && !c.attackTiming(method, target).IgnoreDelay {
		c.renewInteraction(target, generation)
		return
	}
	c.executeAttack(method, target, false, generation, true)
}

// PerformNewAttack runs one attack immediately (a queued
// special pressed mid-cooldown): resolve, validate,
// permission, reach, then execute. Combat.performNewAttack.
func (c *Combat) PerformNewAttack(instant bool) bool {
	target := c.target
	if target == nil {
		return false
	}
	generation := c.generation
	method := GetMethod(c.owner)
	if !ValidTarget(c.owner, target) {
		c.cancelIfCurrent(target, generation)
		return false
	}
	permission := CanAttackPermission(c.owner, target, true, method)
	if permission != CanAttackOK {
		c.handleAttackDenied(permission, target, generation)
		return false
	}
	if !CanReach(combatantFor(c.owner), method, combatantFor(target)) {
		return false
	}
	c.method = method
	return c.executeAttack(method, target, instant, generation, false)
}

// ---- Attack timing ----

// attackTiming asks the hook what it says about this
// attack's cooldown (PluginAttackTimingEvent). Nil hook
// means all zero values.
func (c *Combat) attackTiming(method CombatMethod, target *Mobile) AttackTimingResult {
	if Hooks.OnAttackTiming != nil {
		if result := Hooks.OnAttackTiming(c.owner, target, method, target != c.lastAttackTarget); result != nil {
			return *result
		}
	}
	return AttackTimingResult{}
}

// emitAttackSpeed asks the hook for the attack speed of a
// swing (PluginAttackSpeedEvent). The caller floors at 1
// where TS does.
func (c *Combat) emitAttackSpeed(target *Mobile, t CombatType, speed int) int {
	if Hooks.OnAttackSpeed != nil {
		e := &AttackSpeedEvent{Attacker: c.owner, Target: target, CombatType: t, Speed: speed}
		Hooks.OnAttackSpeed(e)
		return e.Speed
	}
	return speed
}

// SetAttackDelay sets the next attack to ticks from now.
func (c *Combat) SetAttackDelay(ticks int) {
	if ticks < 0 {
		ticks = 0
	}
	c.nextAttackCycle = combatCycle() + int64(ticks)
	c.sendAttackTimer()
}

// ExtendAttackDelay pushes the next attack out to at least
// ticks from now (never earlier).
func (c *Combat) ExtendAttackDelay(ticks int) {
	if ticks < 0 {
		ticks = 0
	}
	if deadline := combatCycle() + int64(ticks); deadline > c.nextAttackCycle {
		c.nextAttackCycle = deadline
	}
	c.sendAttackTimer()
}

// DelayAttack adds ticks to the attack timer as OSRS does
// when eating (Combat.delayAttack).
func (c *Combat) DelayAttack(ticks int) {
	if ticks < 0 {
		ticks = 0
	}
	c.nextAttackCycle += int64(ticks)
	c.sendAttackTimer()
}

// GetAttackDelay returns the ticks until the next attack
// may land (floored at 0).
func (c *Combat) GetAttackDelay() int {
	delay := c.nextAttackCycle - combatCycle()
	if delay < 0 {
		return 0
	}
	return int(delay)
}

// WillAttackBeReadyIn reports whether the attack will be
// ready within ticks.
func (c *Combat) WillAttackBeReadyIn(ticks int) bool {
	if ticks < 0 {
		ticks = 0
	}
	return c.GetAttackDelay() <= ticks
}

// sendAttackTimer tells the player's client how many ticks
// remain until the next attack. No-op in Go: SessionPort
// has no attack-timer send. The sent-cycle bookkeeping is
// kept so the wiring can add the send without touching
// combat logic.
func (c *Combat) sendAttackTimer() {
	if !combatantFor(c.owner).IsPlayer() || c.nextAttackCycle == c.sentAttackCycle {
		return
	}
	remaining := c.GetAttackDelay()
	if remaining <= 0 && c.sentAttackCycle <= combatCycle() {
		return
	}
	c.sentAttackCycle = c.nextAttackCycle
	// sendAttackTimer(remaining): no SessionPort method.
}

// ---- Reset / interaction ----

// Reset clears the interaction and the movement queue.
// resetAnimation=false keeps this tick's animation (a
// manual spell's cast after it resolves). Combat.reset.
func (c *Combat) Reset(resetAnimation ...bool) {
	keep := true
	if len(resetAnimation) > 0 {
		keep = resetAnimation[0]
	}
	c.ClearInteraction(keep)
	if queue := combatantFor(c.owner).MovementQueue(); queue != nil {
		queue.Reset(true)
	}
}

// ClearInteraction clears the current interaction but
// leaves the walk queue intact (Combat.clearInteraction).
func (c *Combat) ClearInteraction(resetAnimation ...bool) {
	keep := true
	if len(resetAnimation) > 0 {
		keep = resetAnimation[0]
	}
	c.manualMovementUntilCycle = -1
	c.resetUnreachableTracking()
	previousTarget := c.target
	c.generation++
	c.target = nil
	c.autoRetaliating = false
	c.cycleState = nil
	if previousTarget != nil && c.method != nil {
		c.method.OnCombatEnded(c.owner)
	}
	c.method = nil
	c.setMobileInteraction(nil)
	c.clearFacePosition()
	if fighter := combatantFor(c.owner); fighter.IsPlayer() {
		// The reset animation: skipped (no animation
		// system); the stall check that guards it is kept
		// in animationStillPlaying for the wiring.
		_ = keep
		fighter.Player().SendVarp(CombatTargetPlayerVarp, -1)
	}
	c.specialAttackQueued = false
	// World.markNpcCombatActive: skipped (see Attack).
}

// clearFacePosition clears the face-position target (NPCs
// only; players carry no face-position state in Go).
func (c *Combat) clearFacePosition() {
	if npc := combatantFor(c.owner).NPC(); npc != nil {
		npc.SetPositionToFace(nil)
	}
}

// setMobileInteraction sets the interaction target: NPCs
// through their interaction API (which faces and flags),
// players through the combat-local record + the
// ENTITY_INTERACTION flag (the sync adapter reads
// Interacting for the index).
func (c *Combat) setMobileInteraction(target *Mobile) {
	c.interacting = target
	if npc := combatantFor(c.owner).NPC(); npc != nil {
		npc.SetMobileInteraction(target)
		return
	}
	c.owner.Flags = c.owner.Flags.Set(FlagEntityInteraction)
}

// Interacting returns the setMobileInteraction target, or
// nil. The sync adapter reads it for the interaction index.
func (c *Combat) Interacting() *Mobile { return c.interacting }

// faceTarget turns the fighter to its target, unless
// content holds its facing elsewhere with
// combat:keep-facing.
func (c *Combat) faceTarget(target *Mobile, clearFacePosition bool) {
	if HasCombatFlag(c.owner, CombatFlagKeepFacing) {
		return
	}
	if clearFacePosition {
		c.clearFacePosition()
	}
	c.setMobileInteraction(target)
}

func (c *Combat) cancelIfCurrent(target *Mobile, generation int) {
	if c.target == target && c.generation == generation {
		c.Reset()
	}
}

func (c *Combat) renewInteraction(target *Mobile, generation int) {
	if c.target != target || c.generation != generation {
		return
	}
	c.generation++
	c.cycleState = nil
	c.faceTarget(target, false)
}

// ---- Damage map ----

// AddDamage records amount of damage entity dealt to this
// fighter (Combat.addDamage): non-positive amounts and NPC
// attackers are ignored.
func (c *Combat) AddDamage(entity *Mobile, amount int) {
	if amount <= 0 || entity == nil {
		return
	}
	fighter := combatantFor(entity)
	if !fighter.IsPlayer() {
		return
	}
	player := fighter.Player()
	c.lastDamager = player
	if c.damageMap == nil {
		c.damageMap = map[*Player]*HitDamageCache{}
	}
	if cached, ok := c.damageMap[player]; ok {
		cached.Increment(amount, combatCycle())
	} else {
		c.damageMap[player] = NewHitDamageCache(amount, combatCycle())
	}
}

// LastDamager returns the player who damaged this fighter
// most recently, while that damage still counts.
func (c *Combat) LastDamager() *Player {
	player := c.lastDamager
	if player != nil && player.IsRegistered() && c.DamageMapContains(&player.Mobile) {
		return player
	}
	return nil
}

// GetKiller returns the player who dealt the most damage
// still within the cache timeout, optionally clearing the
// map (Combat.getKiller).
func (c *Combat) GetKiller(clearMap bool) *Player {
	damage := 0
	var killer *Player
	for player, cached := range c.damageMap {
		if !player.IsRegistered() || cached.ElapsedTicks(combatCycle()) >= DamageCacheTimeoutTicks {
			continue
		}
		if cached.Damage() > damage {
			damage = cached.Damage()
			killer = player
		}
	}
	if clearMap {
		c.damageMap = nil
	}
	return killer
}

// ClearDamageMap forgets recorded player damage.
func (c *Combat) ClearDamageMap() { c.damageMap = nil }

// RemoveDamager forgets one player's damage.
func (c *Combat) RemoveDamager(player *Player) {
	delete(c.damageMap, player)
}

// DamageMapContains reports whether player's damage still
// counts toward the kill.
func (c *Combat) DamageMapContains(mobile *Mobile) bool {
	if mobile == nil {
		return false
	}
	fighter := combatantFor(mobile)
	if !fighter.IsPlayer() {
		return false
	}
	cached, ok := c.damageMap[fighter.Player()]
	return ok && cached.ElapsedTicks(combatCycle()) < DamageCacheTimeoutTicks
}

// RecentDamagers lists the registered players with counted
// damage.
func (c *Combat) RecentDamagers() []*Player {
	var players []*Player
	for player, cached := range c.damageMap {
		if player.IsRegistered() && cached.Damage() > 0 &&
			cached.ElapsedTicks(combatCycle()) < DamageCacheTimeoutTicks {
			players = append(players, player)
		}
	}
	return players
}

// DamagerEntry is one recent-damager record (player +
// damage).
type DamagerEntry struct {
	Player *Player
	Damage int
}

// RecentDamagerEntries lists the recent damagers with
// their damage.
func (c *Combat) RecentDamagerEntries() []DamagerEntry {
	var entries []DamagerEntry
	for player, cached := range c.damageMap {
		if player.IsRegistered() && cached.Damage() > 0 &&
			cached.ElapsedTicks(combatCycle()) < DamageCacheTimeoutTicks {
			entries = append(entries, DamagerEntry{Player: player, Damage: cached.Damage()})
		}
	}
	return entries
}

// ---- Attacker ----

// Attacker returns the Mobile attacking this fighter, or nil.
func (c *Combat) Attacker() *Mobile { return c.attacker }

// SetUnderAttack marks attacker as this fighter's attacker
// (Combat.setUnderAttack): stamps the attacked cycles and
// resets the attacker window.
func (c *Combat) SetUnderAttack(attacker *Mobile) {
	if attacker != nil {
		c.lastAttackedCycle = combatCycle()
		if combatantFor(attacker).IsPlayer() {
			c.lastAttackedByPlayerCycle = combatCycle()
		}
	} else {
		c.lastAttackedCycle = -1
	}
	c.applyAttacker(attacker)
}

// TicksSinceAttackedByPlayer returns the ticks since a
// player last hit this fighter, or -1 when none has (TS
// Infinity).
func (c *Combat) TicksSinceAttackedByPlayer() int64 {
	if c.lastAttackedByPlayerCycle < 0 {
		return -1
	}
	return combatCycle() - c.lastAttackedByPlayerCycle
}

// TicksSinceAttacked returns the ticks since an attacker
// last hit this fighter, or -1 when never hit or cleared
// (TS Infinity).
func (c *Combat) TicksSinceAttacked() int64 {
	if c.lastAttackedCycle < 0 {
		return -1
	}
	return combatCycle() - c.lastAttackedCycle
}

func (c *Combat) clearAttacker() { c.applyAttacker(nil) }

func (c *Combat) applyAttacker(attacker *Mobile) {
	c.attacker = attacker
	c.lastAttackCycle = combatCycle()
	// World.markNpcCombatActive: skipped (see Attack).
}

// ---- Spells / ranged / fight type ----

// CastSpell returns the current cast spell.
func (c *Combat) CastSpell() CombatSpell { return c.castSpell }

// SetCastSpell sets the current cast spell.
func (c *Combat) SetCastSpell(spell CombatSpell) { c.castSpell = spell }

// AutoCastSpell returns the autocast spell.
func (c *Combat) AutoCastSpell() CombatSpell { return c.autoCastSpell }

// SetAutoCastSpell sets the autocast spell.
func (c *Combat) SetAutoCastSpell(spell CombatSpell) { c.autoCastSpell = spell }

// SelectedSpell returns the cast spell, else the autocast
// spell (Combat.getSelectedSpell).
func (c *Combat) SelectedSpell() CombatSpell {
	if c.castSpell != nil {
		return c.castSpell
	}
	return c.autoCastSpell
}

// PreviousCast returns the previous cast spell.
func (c *Combat) PreviousCast() CombatSpell { return c.previousCast }

// SetPreviousCast sets the previous cast spell.
func (c *Combat) SetPreviousCast(spell CombatSpell) { c.previousCast = spell }

// RangedWeapon returns the resolved ranged weapon data.
func (c *Combat) RangedWeapon() *RangedWeaponData { return c.rangedWeapon }

// SetRangedWeapon sets the resolved ranged weapon data.
func (c *Combat) SetRangedWeapon(weapon *RangedWeaponData) { c.rangedWeapon = weapon }

// Ammunition returns the resolved ammunition data.
func (c *Combat) Ammunition() *AmmunitionData { return c.ammunition }

// SetAmmunition sets the resolved ammunition data.
func (c *Combat) SetAmmunition(ammo *AmmunitionData) { c.ammunition = ammo }

// FightType returns the fighter's current fight type
// (default UNARMED_PUNCH). No fight-type field exists on
// Go's Player, so it lives here; a style-button listener
// sets it through SetFightType.
func (c *Combat) FightType() *FightType {
	if c.fightType == nil {
		return UnarmedPunch
	}
	return c.fightType
}

// SetFightType sets the fighter's fight type (nil restores
// the default).
func (c *Combat) SetFightType(fightType *FightType) {
	if fightType == nil {
		fightType = UnarmedPunch
	}
	c.fightType = fightType
}

// ---- Special attacks ----

// GetCombatSpecial resolves the equipped weapon's special
// attack (Player.getCombatSpecial), or nil.
func (c *Combat) GetCombatSpecial() *CombatSpecial {
	fighter := combatantFor(c.owner)
	if !fighter.IsPlayer() {
		return nil
	}
	weapon := fighter.Equipment().GetWeapon()
	if !weapon.IsValid() {
		return nil
	}
	return CombatSpecialForItem(weapon.ID())
}

// IsSpecialActivated reports whether the special attack is
// armed (Mobile.isSpecialActivated, stored here).
func (c *Combat) IsSpecialActivated() bool { return c.specialActivated }

// SetSpecialActivated arms or disarms the special attack.
func (c *Combat) SetSpecialActivated(activated bool) { c.specialActivated = activated }

// setSpecialActivated is the unexported alias the hit queue
// uses for the ordinary-hit lowering.
func (c *Combat) setSpecialActivated(activated bool) { c.specialActivated = activated }

// SpecialPercentage returns the special attack energy
// (0..100, default 100).
func (c *Combat) SpecialPercentage() int { return c.specialPercentage }

// SetSpecialPercentage sets the special attack energy,
// clamped to 0..100.
func (c *Combat) SetSpecialPercentage(percentage int) {
	if percentage < 0 {
		percentage = 0
	}
	if percentage > 100 {
		percentage = 100
	}
	c.specialPercentage = percentage
}

// DrainSpecial drains special energy (CombatSpecial.drain).
// Core NEVER calls it: the drain site is the individual
// special-attack content method's Start (grep: no core
// caller of CombatSpecial.drain exists in TS; every caller
// is a plugin special). With a special armed, the cost
// hook may lower the cost; the drain takes what there is
// (floored at 0). The item-level drain override
// (setItemSpecial) is not modeled: Cost resolves the
// special's own drain amounts.
func (c *Combat) DrainSpecial(amount int) {
	fighter := combatantFor(c.owner)
	cost := amount
	if fighter.IsPlayer() {
		if special := c.GetCombatSpecial(); special != nil {
			weaponID := -1
			if weapon := fighter.Equipment().GetWeapon(); weapon.IsValid() {
				weaponID = weapon.ID()
			}
			cost = special.Cost(fighter.Player(), weaponID)
		}
	}
	c.specialPercentage -= cost
	if c.specialPercentage < 0 {
		c.specialPercentage = 0
	}
	// Draining starts the restore task (CombatSpecial.ts:178,
	// drain calls ensureRestoreTask); the ensure no-ops when
	// the energy is still full or a restore already runs.
	EnsureSpecialRestore(c.owner)
	// Draining re-sends the orb (CombatSpecial.ts:182,
	// drain calls updateBar for a player, after the decrement
	// and the restore-task ensure): varp 300 then varp 301,
	// in TS order. NPCs get nothing (updateSpecialAttackOrb
	// is player-only).
	updateSpecialAttackOrb(c.owner)
}

// IsSpecialAttackQueued reports whether a special is
// queued for the next attack.
func (c *Combat) IsSpecialAttackQueued() bool { return c.specialAttackQueued }

// SetSpecialAttackQueued queues or unqueues a special.
func (c *Combat) SetSpecialAttackQueued(queued bool) { c.specialAttackQueued = queued }

// ---- Animation stalls ----

// ExpectAttackAnimation arms the swing measurement for this
// cycle (Combat.expectAttackAnimation).
func (c *Combat) ExpectAttackAnimation() {
	c.measureAnimationCycle = combatCycle()
}

// sequenceStallTicks, when set, answers the stall length of
// a sequence id (getSequenceStall). Nil means no sequence
// carries a stall: OnAnimationStarted is a no-op until the
// coordinator wires sequence data.
var sequenceStallTicks func(animationID int) int

// SetSequenceStallSource installs the sequence-stall lookup.
func SetSequenceStallSource(source func(animationID int) int) {
	sequenceStallTicks = source
}

// OnAnimationStarted measures an animation the fighter just
// started (Combat.onAnimationStarted): only an armed
// swing's animation, only for players, capped at 5 ticks,
// a partial stall holding hits. The emitAnimationStall
// hook (which lets content change kind/ticks) is NOT
// ported: no hook field exists for it. Content-declared
// full stalls use SetAnimationStall.
func (c *Combat) OnAnimationStarted(animationID int) {
	if c.measureAnimationCycle != combatCycle() {
		return
	}
	if !combatantFor(c.owner).IsPlayer() {
		return
	}
	if sequenceStallTicks == nil {
		return
	}
	ticks := sequenceStallTicks(animationID)
	if ticks < 0 {
		ticks = 0
	}
	if ticks > MaxAnimationStallTicks {
		ticks = MaxAnimationStallTicks
	}
	if ticks <= 0 {
		c.stallSet = false
		return
	}
	c.stallUntilCycle = combatCycle() + int64(ticks)
	c.stallFull = false
	c.stallHoldsHits = true
	c.stallSet = true
}

// SetAnimationStall declares a content full stall
// (Combat.setAnimationStall): at most 100 ticks, never
// shortening an active full stall.
func (c *Combat) SetAnimationStall(ticks int) {
	if ticks <= 0 {
		return
	}
	now := combatCycle()
	if ticks > MaxContentAnimationStallTicks {
		ticks = MaxContentAnimationStallTicks
	}
	activeFull := c.stallSet && c.stallFull && c.stallUntilCycle > now
	until := now + int64(ticks)
	if activeFull && c.stallUntilCycle > until {
		until = c.stallUntilCycle
	}
	c.stallUntilCycle = until
	c.stallFull = true
	// A full stall holds clicks, movement, and incoming hits
	// (Combat.ts: getIncomingHitCycle returns the stall end
	// when the stall holds hits).
	c.stallHoldsHits = true
	c.stallSet = true
}

// GetAnimationStallTicks returns the ticks before the
// fighter may act again; only a full stall holds them.
func (c *Combat) GetAnimationStallTicks() int {
	if !c.stallSet || !c.stallFull || c.stallUntilCycle <= combatCycle() {
		return 0
	}
	return int(c.stallUntilCycle - combatCycle())
}

// IsAnimationStalled reports whether a full stall holds the
// fighter (the movement gate reads this).
func (c *Combat) IsAnimationStalled() bool { return c.GetAnimationStallTicks() > 0 }

// GetIncomingHitCycle returns the last cycle a hit on this
// fighter is held to, or -1 when nothing holds it.
func (c *Combat) GetIncomingHitCycle() int64 {
	if !c.stallSet || !c.stallHoldsHits || c.stallUntilCycle <= combatCycle() {
		return -1
	}
	return c.stallUntilCycle
}

// animationStillPlaying reports whether the swing's
// animation is still meant to be on screen.
func (c *Combat) animationStillPlaying() bool {
	return c.stallSet && combatCycle() < c.stallUntilCycle
}

// ---- Prayer block / vengeance / retaliation flags ----

// PrayerBlockFinished reports whether the prayer block has
// expired (the prayerBlockTimer SecondsTimer, cycle-stamped).
func (c *Combat) PrayerBlockFinished() bool {
	return combatCycle() >= c.prayerBlockUntilCycle
}

// StartPrayerBlock starts the prayer block for seconds
// (DisableProtectionPrayers uses 5).
func (c *Combat) StartPrayerBlock(seconds int) {
	ticks := (int64(seconds)*1000 + 599) / 600
	c.prayerBlockUntilCycle = combatCycle() + ticks
}

// HasVengeance reports whether vengeance is armed.
func (c *Combat) HasVengeance() bool { return c.vengeance }

// SetVengeance arms or clears vengeance.
func (c *Combat) SetVengeance(vengeance bool) { c.vengeance = vengeance }

// AutoRetaliate reports the auto-retaliate setting (default
// true; TS reads the player setting — no such field exists
// in Go, so it lives here).
func (c *Combat) AutoRetaliate() bool { return c.autoRetaliate }

// SetAutoRetaliate sets the auto-retaliate setting.
func (c *Combat) SetAutoRetaliate(autoRetaliate bool) { c.autoRetaliate = autoRetaliate }

// SetUntargetable sets the untargetable flag (content sets
// Mobile.untargetable in TS; Go stores it here).
func (c *Combat) SetUntargetable(untargetable bool) { c.untargetable = untargetable }

// isUntargetable reports whether mobile is untargetable.
func isUntargetable(mobile *Mobile) bool {
	return mobile != nil && mobile.GetCombat().untargetable
}

// ---- Retaliation scheduling ----

// scheduleRetaliation records a counter-attack due on
// dueCycle, re-validating at fire time (the Go form of the
// 1-tick TaskManager task in handleRetaliation).
func (c *Combat) scheduleRetaliation(target *Mobile, dueCycle int64, attackable, playerBusy func() bool) {
	c.pendingRetaliation = &pendingRetaliation{
		target:     target,
		dueCycle:   dueCycle,
		attackable: attackable,
		playerBusy: playerBusy,
	}
}

func (c *Combat) checkPendingRetaliation(cycle int64) {
	pending := c.pendingRetaliation
	if pending == nil || cycle < pending.dueCycle {
		return
	}
	c.pendingRetaliation = nil
	if pending.attackable != nil && !pending.attackable() {
		return
	}
	if combatantFor(c.owner).IsPlayer() {
		if !c.autoRetaliate || HasCombatFlag(c.owner, CombatFlagNoRetaliate) {
			return
		}
		if pending.playerBusy != nil && pending.playerBusy() {
			return
		}
	}
	c.attack(pending.target, true)
}

// ---- Delayed damage ----

// queueDelayedDamage records one splat to land after
// extraTicks, if the incoming generation still matches
// (applyResolvedHitDamage's per-splat delays).
func (c *Combat) queueDelayedDamage(hit *HitDamage, extraTicks int, generation int) {
	c.delayedDamage = append(c.delayedDamage, delayedDamageEntry{
		hit:        hit,
		dueCycle:   combatCycle() + int64(extraTicks),
		generation: generation,
	})
}

func (c *Combat) processDelayedDamage(cycle int64) {
	if len(c.delayedDamage) == 0 {
		return
	}
	remaining := c.delayedDamage[:0]
	for _, entry := range c.delayedDamage {
		if entry.dueCycle > cycle {
			remaining = append(remaining, entry)
			continue
		}
		if entry.generation != c.hitQueue.IncomingGeneration() {
			continue
		}
		if !c.owner.IsRegistered() || combatantFor(c.owner).Hitpoints() <= 0 {
			continue
		}
		c.hitQueue.AddPendingDamage([]*HitDamage{entry.hit})
	}
	c.delayedDamage = remaining
}

// ---- Magic base XP ----

func (c *Combat) setBaseExperienceOwed() { c.baseExperienceOwed = true }

// takeBaseExperienceOwed consumes the owed flag, reporting
// whether XP was owed.
func (c *Combat) takeBaseExperienceOwed() bool {
	if !c.baseExperienceOwed {
		return false
	}
	c.baseExperienceOwed = false
	return true
}

// ---- Retreat / unreachable / safespot ----

// shouldRetreat is Combat.shouldRetreat: an NPC that has
// been dragged out of range, or whose low-health retreat
// rule fires, gives up and walks home (the coordinator
// RETREATING state). Private-area exemption: skipped (no
// private areas). Cross-floor ranged exception: skipped
// (no CrossFloor).
func (c *Combat) shouldRetreat(method CombatMethod) bool {
	fighter := combatantFor(c.owner)
	npc := fighter.NPC()
	if npc == nil {
		return false
	}
	if attributeInt(npc.GetAttribute("arceuus:dark-lure-until")) > int(combatCycle()) {
		return false
	}
	if HasCombatFlag(c.owner, CombatFlagNoRetreat) {
		return false
	}
	coordinator := npc.MovementCoordinator()
	if coordinator != nil && coordinator.GetCoordinateState() == CoordinateStateRetreating {
		return true
	}
	definition := npc.Definition()
	spawn := npc.Spawn
	spawnDistance := npc.Location().Distance(spawn)
	doesRetreat := false
	if d := fighter.NPCDef(); d != nil {
		doesRetreat = d.DoesRetreat()
	}
	if doesRetreat && spawnDistance >= definition.CombatFollowDistance() {
		if coordinator != nil {
			coordinator.SetCoordinateState(CoordinateStateRetreating)
		}
		return true
	}
	target := c.target
	if target == nil || c.isSafespotted(target) {
		return false
	}
	aggressionRange := definition.AggressionRange(float64(method.AttackDistance(c.owner)))
	targetDistance := FootprintDistance(spawn, definition.Size(), target.Location(), false)
	if spawnDistance > definition.CombatFollowDistance() || targetDistance > aggressionRange {
		if coordinator != nil {
			coordinator.SetCoordinateState(CoordinateStateRetreating)
		}
		return true
	}
	return false
}

// processNpcUnderTarget steps an NPC off its target
// (Combat.processNpcUnderTarget): a size-1 NPC steps a
// random cardinal tile; a large NPC walks off the shortest
// way, one tile a tick. The target-busy check degenerates:
// Go players have no following/MovementTask, so the target
// never counts as busy. The occupant-ignore half is
// unavailable (no ignoreOccupantThisCycle in Go).
func (c *Combat) processNpcUnderTarget(state *CombatCycleState) {
	state.skipPost = true
	movement := combatantFor(c.owner).MovementQueue()
	if movement == nil {
		return
	}
	movement.Reset(true)
	size := combatantFor(c.owner).Size()
	if size < 1 {
		size = 1
	}
	if size <= 1 {
		offsets := [][2]int{{-1, 0}, {1, 0}, {0, -1}, {0, 1}}
		offset := offsets[randomInclusive(3)]
		movement.AddCheckpoint(c.owner.Location().Transform(offset[0], offset[1]))
		return
	}
	if !movement.getMobility().canMove() || movement.IsMovementBlocked() {
		return
	}
	if npc := combatantFor(c.owner).NPC(); npc != nil && npc.IsScriptedMovement() {
		return
	}
	if HasCombatFlag(c.owner, CombatFlagBuiltIntoScenery) {
		return
	}
	step := c.stepOffTarget(state.target, size)
	if step == nil {
		return
	}
	movement.AddCheckpoint(*step)
}

// stepOffTarget is Combat.stepOffTarget: the first tile of
// the shortest cardinal way out from over target; nil when
// boxed in.
func (c *Combat) stepOffTarget(target *Mobile, size int) *Location {
	at := c.owner.Location()
	them := target.Location()
	movement := combatantFor(c.owner).MovementQueue()
	if movement == nil {
		return nil
	}
	type option struct {
		dx, dy, cost int
	}
	options := []option{
		{-1, 0, at.X + size - them.X},
		{1, 0, them.X - at.X + 1},
		{0, -1, at.Y + size - them.Y},
		{0, 1, them.Y - at.Y + 1},
	}
	var best *option
	for i := range options {
		candidate := at.Transform(options[i].dx, options[i].dy)
		if !movement.CanWalkTo(candidate, at) {
			continue
		}
		if best == nil || options[i].cost < best.cost {
			best = &options[i]
		}
	}
	if best == nil {
		return nil
	}
	step := at.Transform(best.dx, best.dy)
	return &step
}

// giveUpUnreachableTarget ends combat with a target the
// fighter has stopped making progress toward
// (Combat.giveUpUnreachableTarget). Progress is the gap
// closing or a step taken last cycle. Returns true when
// the interaction was dropped.
func (c *Combat) giveUpUnreachableTarget(target *Mobile, generation int) bool {
	if c.isSafespotted(target) {
		c.resetUnreachableTracking()
		return false
	}
	distance := c.owner.Location().Distance(target.Location())
	closing := c.unreachableDistance >= 0 && distance < c.unreachableDistance
	stepping := false
	if queue := combatantFor(c.owner).MovementQueue(); queue != nil {
		stepping = queue.DidMovePreviousCycle(combatCycle())
	}
	c.unreachableDistance = distance
	if closing || stepping {
		c.unreachableCycles = 0
	} else {
		c.unreachableCycles++
	}
	if c.unreachableCycles < UnreachableGiveUpCycles {
		return false
	}
	if fighter := combatantFor(c.owner); fighter.IsPlayer() && !fighter.IsPlayerBot() {
		fighter.SendMessage("I can't reach that!")
	}
	c.resetUnreachableTracking()
	c.cancelIfCurrent(target, generation)
	return true
}

// isSafespotted is Combat.isSafespotted: an NPC whose
// target is fighting it from where it cannot be reached
// keeps the target.
func (c *Combat) isSafespotted(target *Mobile) bool {
	if combatantFor(c.owner).NPC() == nil {
		return false
	}
	return HasCombatFlag(c.owner, CombatFlagKeepTarget) ||
		target.GetCombat().Target() == c.owner || c.attacker == target
}

func (c *Combat) resetUnreachableTracking() {
	c.unreachableCycles = 0
	c.unreachableDistance = -1
}

// naiveEntityDestination is
// PathFinder.naiveEntityDestination, reduced: the tile in
// the ring around the target's footprint nearest the
// attacker. The TS form is a naive (unpathed) destination;
// the ring-nearest tile is the same shape.
func naiveEntityDestination(attacker, target *Mobile) Location {
	targetLoc := target.Location()
	size := combatantFor(target).Size()
	if size < 1 {
		size = 1
	}
	attackerLoc := attacker.Location()
	best := targetLoc
	bestDistance := int64(1) << 60
	for x := targetLoc.X - 1; x <= targetLoc.X+size; x++ {
		for y := targetLoc.Y - 1; y <= targetLoc.Y+size; y++ {
			if x >= targetLoc.X && x < targetLoc.X+size && y >= targetLoc.Y && y < targetLoc.Y+size {
				continue
			}
			candidate := Location{X: x, Y: y, Z: targetLoc.Z}
			dx := int64(candidate.X - attackerLoc.X)
			dy := int64(candidate.Y - attackerLoc.Y)
			if dx < 0 {
				dx = -dx
			}
			if dy < 0 {
				dy = -dy
			}
			distance := dx
			if dy > distance {
				distance = dy
			}
			if distance < bestDistance {
				bestDistance = distance
				best = candidate
			}
		}
	}
	return best
}

// ---- Attack execution ----

// executeAttack is Combat.executeAttack: the cooldown and
// timing checks, the method check, onCombatBegan, the
// delay, the swing (start, hits, addPendingHit, special
// targeting, finished), and renewal or cancellation.
func (c *Combat) executeAttack(method CombatMethod, target *Mobile, bypassDelay bool, generation int, renew bool) bool {
	cycle := combatCycle()
	fighter := combatantFor(c.owner)
	specialTraits := ActiveTraitsFor(fighter)
	var specialMethod CombatMethod
	if special := c.GetCombatSpecial(); special != nil {
		specialMethod = special.Method
	}
	bypass := bypassDelay || (specialMethod == method && specialTraits != nil && specialTraits.BypassAttackDelay)
	timing := c.attackTiming(method, target)
	if !bypass && !timing.IgnoreDelay && cycle < c.nextAttackCycle {
		if renew {
			c.renewInteraction(target, generation)
		}
		return false
	}

	// Both validations skipped: permission was checked in
	// the movement phases; only the method check runs.
	response := CanAttack(c.owner, target, method, true, true)
	if response != CanAttackOK {
		c.handleAttackDenied(response, target, generation)
		return false
	}

	if c.attacker == nil {
		method.OnCombatBegan(c.owner)
	}
	if target.GetCombat().Attacker() == nil {
		GetMethod(target).OnCombatBegan(target)
	}
	if !bypass && !timing.KeepDelay {
		speed := method.AttackSpeed(c.owner)
		if specialTraits != nil && specialTraits.AttackSpeedTicks != nil {
			speed = *specialTraits.AttackSpeedTicks
		}
		speed = c.emitAttackSpeed(target, method.Type(), speed)
		if speed < 1 {
			speed = 1
		}
		c.nextAttackCycle = cycle + int64(speed)
		c.sendAttackTimer()
	} else if !bypass && timing.MinimumDelay > 0 {
		if deadline := cycle + int64(timing.MinimumDelay); deadline > c.nextAttackCycle {
			c.nextAttackCycle = deadline
		}
		c.sendAttackTimer()
	} else if specialMethod == method && specialTraits != nil && specialTraits.BypassAttackDelay {
		if specialTraits.AttackSpeedTicks != nil {
			// Quick Smash keeps a cooldown already running
			// and leaves its own minimum delay.
			c.ExtendAttackDelay(c.emitAttackSpeed(target, method.Type(), *specialTraits.AttackSpeedTicks))
		} else {
			// Momentum Throw bypasses the old deadline but
			// starts a full weapon cooldown.
			speed := c.emitAttackSpeed(target, method.Type(), method.AttackSpeed(c.owner))
			if speed < 1 {
				speed = 1
			}
			c.nextAttackCycle = cycle + int64(speed)
			c.sendAttackTimer()
		}
	}
	c.lastAttackTarget = target

	c.ExpectAttackAnimation()
	if Hooks.OnCombatAttackStarted != nil {
		Hooks.OnCombatAttackStarted(c.owner, target, method)
	}
	method.Start(c.owner, target)
	// NPC attack sounds: skipped (no sound system).
	c.measureAnimationCycle = -1
	hits := method.Hits(c.owner, target)
	if hits == nil {
		if Hooks.OnCombatAttackFinished != nil {
			Hooks.OnCombatAttackFinished(c.owner, target, method)
		}
		return false
	}
	// Block animations (Combat.ts:1054-1055): a melee swing
	// plays the target's block animation at launch — a
	// block animation of -1 means the target doesn't block
	// (e.g. the Gemstone Crab) — unless the method opts out
	// (playsBlockAnimation() !== false).
	if len(hits) > 0 && method.Type() == CombatTypeMelee && method.PlaysBlockAnimation() {
		if blockAnim := target.GetBlockAnim(); blockAnim >= 0 {
			target.PerformAnimation(blockAnim, 0)
		}
	}
	for _, hit := range hits {
		AddPendingHit(hit)
	}

	// Area/footprint specials strike additional targets
	// with the same method.
	if targeting := ActiveTargetingFor(fighter); targeting != nil {
		if targeting.LargeTargetExtraHit {
			scale := targeting.AccuracyScale
			if scale == 0 {
				scale = 1
			}
			extra := WithAccuracyScale(scale, func() []*PendingHit {
				return method.Hits(c.owner, target)
			})
			for _, hit := range extra {
				AddPendingHit(hit)
			}
		} else if Hooks.AreaAttackTargets != nil {
			for _, secondary := range Hooks.AreaAttackTargets(c.owner, target, targeting.Radius) {
				secondaryHits := method.Hits(c.owner, secondary)
				for _, hit := range secondaryHits {
					AddPendingHit(hit)
				}
			}
		}
	}

	method.Finished(c.owner, target)
	if Hooks.OnCombatAttackFinished != nil {
		Hooks.OnCombatAttackFinished(c.owner, target, method)
	}

	c.specialAttackQueued = false
	if c.specialActivated {
		c.specialActivated = false
		// CombatSpecial.updateBar: skipped (no interface
		// runtime).
	}
	if renew && c.target == target && c.generation == generation {
		if method.ShouldRenew(c.owner, target) {
			c.renewInteraction(target, generation)
		} else {
			c.cancelIfCurrent(target, generation)
		}
	}
	return true
}

// isTransientDenial is Combat.isTransientDenial: denials
// that clear on their own while pursuit continues.
func (c *Combat) isTransientDenial(response CanAttackResponse) bool {
	return response == CanAttackAlreadyUnderAttack ||
		response == CanAttackMethodNotAllowed ||
		(response == CanAttackTargetInCombat && !combatantFor(c.owner).IsPlayer())
}

// walksInOnFoughtNpc is Combat.walksInOnFoughtNpc: single
// combat for a player attacking in melee an NPC someone
// else is fighting — the player still walks in, and the
// refusal is said on the click tick and again the tick
// after arrival. Ranged and magic are refused on the spot.
func (c *Combat) walksInOnFoughtNpc(permission CanAttackResponse, target *Mobile, method CombatMethod, steppedThisCycle bool) bool {
	if permission != CanAttackTargetInCombat || !combatantFor(c.owner).IsPlayer() ||
		combatantFor(target).NPC() == nil || method.Type() != CombatTypeMelee {
		return false
	}
	if !steppedThisCycle && CanReach(combatantFor(c.owner), method, combatantFor(target)) {
		return false
	}
	cycle := combatCycle()
	if c.foughtNpcNoticeGeneration != c.generation || c.foughtNpcNoticeCycle == cycle {
		c.foughtNpcNoticeGeneration = c.generation
		c.foughtNpcNoticeCycle = cycle
		combatantFor(c.owner).SendMessage("Someone else is fighting that.")
	}
	return true
}

// handleAttackDenied is Combat.handleAttackDenied: the
// refusal messages, then cancellation — except
// COMBAT_METHOD_NOT_ALLOWED (keeps the target) and
// ALREADY_UNDER_ATTACK (once per interaction, no cancel).
// Duel cases use plain messages: the Dueling port (phase 17)
// produces these responses through CanAttackPermission, and
// the StatementDialogue form is not ported.
func (c *Combat) handleAttackDenied(response CanAttackResponse, target *Mobile, generation int) {
	if c.target != target || c.generation != generation {
		return
	}
	fighter := combatantFor(c.owner)
	switch response {
	case CanAttackMethodNotAllowed:
		// The weapon/spell is momentarily unusable. Keep
		// the target so re-equipping resumes; the method
		// itself owns any message and any hard reset.
		return
	case CanAttackAlreadyUnderAttack:
		if c.deniedNoticeGeneration != generation {
			c.deniedNoticeGeneration = generation
			fighter.SendTypedMessage(chatTypeSpam, "I'm already under attack.", "")
		}
		return
	case CanAttackTargetInCombat:
		if !fighter.IsPlayer() {
			return
		}
		if combatantFor(target).IsPlayer() {
			fighter.SendMessage("Someone else is already fighting " + combatantFor(target).Name() + ".")
		} else {
			fighter.SendMessage("Someone else is fighting that.")
		}
	case CanAttackLevelDifferenceTooGreat:
		fighter.SendMessage("Your level difference is too great!<br>You need to move deeper into the Wilderness.")
	case CanAttackNotEnoughSpecialEnergy:
		if fighter.IsPlayer() {
			fighter.SendMessage("You don't have enough power left.")
			c.specialActivated = false
		}
	case CanAttackStunned:
		fighter.SendMessage("You're currently stunned and cannot attack.")
	case CanAttackDuelNotStartedYet:
		fighter.SendMessage("The duel has not started yet!")
	case CanAttackDuelWrongOpponent:
		fighter.SendMessage("This is not your opponent!")
	case CanAttackDuelMeleeDisabled:
		fighter.SendMessage("Melee has been disabled in this duel!")
	case CanAttackDuelRangedDisabled:
		fighter.SendMessage("Ranged has been disabled in this duel!")
	case CanAttackDuelMagicDisabled:
		fighter.SendMessage("Magic has been disabled in this duel!")
	case CanAttackTargetIsImmune:
		fighter.SendMessage("This npc is currently immune to attacks.")
	}
	c.cancelIfCurrent(target, generation)
}
