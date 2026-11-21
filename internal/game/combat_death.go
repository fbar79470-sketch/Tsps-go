package game

// Death handling: the port of the death flows in
// Mobile.decrementHealth, Player.appendDeath /
// PlayerDeathTask (PlayerDeath.ts), and NPC.appendDeath /
// NPCDeathTask — as far as core goes. TS runs death as
// TaskManager tasks; Go advances a combat-local state
// machine from ProcessHitQueue with the same tick counts.
//
// DROPS FINDING (grounded): core does NOT roll drops. No
// npc-drops:roll exists in TS core; drops are rolled by the
// PLUGIN server/plugins/npcs/NpcDrops.plugin.js (wiki-dump
// tables) listening to onNpcDeath. Core only emits the
// death event. Go dispatches CombatDeathEvent through the
// hook seam and rolls nothing.
//
// Player item loss (kept-items rules, skull/protect item,
// untradeables, graves, bones) lives in playerdeath.go: the
// port of PlayerDeathTask's respawn-tick drop (441 lines).
// The death event carries killer + items-lost shape through
// the seam, with the handled chain read back; content owns
// the loss only through the onPlayerDeathItemDrop /
// onPlayerDeathDrops hooks (a grave takes the drops there).
// The default respawn restores hitpoints (through the skills
// reset in resetDeathAttributes), clears dying/untargetable/
// blocked movement, sends the death message + jingle 90, and
// teleports home to DefaultRespawnLocation. The
// OnPlayerRespawn hook replaces the default wholesale.
//
// Death state persists nowhere: dying is session-only, and
// no combat state is written to saves (see the save-split
// finding in the phase report) — except the "death" and
// "respawn" save requests on the respawn tick, which run
// through the CombatHooks.RequestSave seam when wired.

// PlayerDeathTicks is the PlayerDeathTask tick count (4):
// respawn four ticks after the death animation starts.
const PlayerDeathTicks = 4

// DefaultDeathMessage is PlayerDeathTask.DEATH_MESSAGE.
const DefaultDeathMessage = "Oh dear, you are dead!"

// DeathAnimationID is the death animation (836)
// (PlayerDeath.ts:163): played on the case-4 death tick,
// four ticks before the respawn tick.
const DeathAnimationID = 836

// PlayerDyingEventName is the "player:dying" custom event
// (PlayerDeath.ts case 4): emitted on the death tick, after
// the death animation plays.
const PlayerDyingEventName = "player:dying"

// PlayerDyingEvent is the player:dying payload: the dying
// player and the killer, or nil when there is none.
type PlayerDyingEvent struct {
	Player *Player
	Killer *Player
}

// DecrementHealth is Mobile.decrementHealth: apply one
// landed hit to the victim's hitpoints and return the hit
// (possibly zeroed). Already-dead victims take 0 damage;
// overkill clamps to the hitpoints remaining (min 1);
// health lost is recorded on the hit; at 0 hitpoints the
// death sequence begins. The combat:damage-apply /
// combat:damage-applied events are skipped (no hook
// fields); the protected-developer rule is skipped (no
// rights system). The hitpoints-locked NPC rule IS ported
// (below): a locked NPC shows the hit but keeps its HP.
func DecrementHealth(victim *Mobile, hit *HitDamage) *HitDamage {
	fighter := combatantFor(victim)
	if fighter.Dying() {
		hit.SetDamage(0)
		hit.SetHealthLost(0)
		return hit
	}
	hitpoints := fighter.Hitpoints()
	damage := hit.Damage()
	if damage < 0 {
		damage = 0
	}
	cap := hitpoints
	if cap < 1 {
		cap = 1
	}
	if damage > cap {
		damage = cap
	}
	hit.SetDamage(damage)
	// A boss whose HP is a timer shows every hit but keeps its
	// HP (Mobile.ts:732-735, decrementHealth): a hitpoints-
	// locked NPC takes no health loss. The damage was already
	// clamped non-negative above (the TS negative clamp), the
	// hit keeps its damage value for display, and death never
	// starts.
	if fighter.NPC() != nil && fighter.NPC().IsHitpointsLocked() {
		hit.SetHealthLost(0)
		return hit
	}
	remaining := hitpoints - damage
	if remaining < 0 {
		remaining = 0
	}
	fighter.SetHitpoints(remaining)
	hit.SetHealthLost(hitpoints - remaining)
	// Reflected damage that credits its source counts
	// toward the source's kill credit.
	if hit.IsReflected() && hit.CreditsItsSource() && hit.Source() != nil {
		victim.GetCombat().AddDamage(hit.Source(), hit.HealthLost())
	}
	if remaining <= 0 {
		victim.GetCombat().beginDeath()
	}
	return hit
}

// TickHit is one hit that landed this cycle, in the shape
// the sync adapter needs: damage, mask, source, the rolled
// maximum (max-hit splat variants), and the splat-type
// overrides. The adapter computes "mine" per viewer.
type TickHit struct {
	Damage        int
	Mask          HitMask
	Source        *Mobile
	RolledMaximum int
	SplatMine     int
	SplatOthers   int
}

// AddTickHit records a landed hit for this cycle's sync
// (Mobile.addTickHit) and flags HIT. The list clears when
// the next cycle's ProcessHitQueue runs.
func AddTickHit(mobile *Mobile, hit *HitDamage) {
	combat := mobile.GetCombat()
	combat.tickHits = append(combat.tickHits, hit)
	mobile.Flags = mobile.Flags.Set(FlagHit)
}

// TickHits snapshots this cycle's landed hits for the sync
// adapter. It does not clear: ProcessHitQueue clears at
// the next cycle.
func (c *Combat) TickHits() []TickHit {
	hits := make([]TickHit, 0, len(c.tickHits))
	for _, hit := range c.tickHits {
		hits = append(hits, TickHit{
			Damage:        hit.Damage(),
			Mask:          hit.Mask(),
			Source:        hit.Source(),
			RolledMaximum: hit.RolledMaximum(),
			SplatMine:     hit.SplatType(true),
			SplatOthers:   hit.SplatType(false),
		})
	}
	return hits
}

// HitpointsOf returns the fighter's current and maximum
// hitpoints for the sync adapter (health bar).
func HitpointsOf(mobile *Mobile) (current, max int) {
	fighter := combatantFor(mobile)
	return fighter.Hitpoints(), fighter.MaxHitpoints()
}

// beginDeath starts the death sequence (Player/NPC
// appendDeath): the before-death hook may prevent it;
// in-flight hits from the victim are dropped; the killer
// and damagers are captured; combat resets; movement
// blocks; the victim is marked dying (and, for players,
// untargetable).
func (c *Combat) beginDeath() {
	if c.deathStarted {
		return
	}
	fighter := combatantFor(c.owner)
	before := &CombatBeforeDeathEvent{Victim: c.owner, Announce: true}
	if fighter.IsPlayer() {
		if Hooks.OnPlayerBeforeDeath != nil {
			Hooks.OnPlayerBeforeDeath(before)
		}
	} else if fighter.NPC() != nil {
		if Hooks.OnNpcBeforeDeath != nil {
			Hooks.OnNpcBeforeDeath(before)
		}
	} else {
		return
	}
	if before.PreventDeath {
		return
	}
	// Hits still in the air belong to this life too.
	DropHitsFrom(c.owner)
	// Kill credit is per life: removing this fighter from
	// other players' damage maps needs the world player
	// list — skipped (no world reference from combat).
	c.deathDamagers = c.RecentDamagers()
	c.deathKiller = c.GetKiller(true)
	c.deathMessage = before.Message
	if c.deathMessage == "" {
		c.deathMessage = DefaultDeathMessage
	}
	c.deathAnnounce = before.Announce
	c.deathStarted = true
	c.deathDispatched = false
	c.deathStartCycle = combatCycle()
	c.deathAdvanceCycle = -1
	fighter.SetDying(true)
	if fighter.IsPlayer() {
		c.deathTicks = PlayerDeathTicks
	} else {
		c.deathTicks = 1
		if d := fighter.NPCDef(); d != nil && d.DeathTicks() > 1 {
			c.deathTicks = d.DeathTicks()
		}
	}
	// Reset combat; block movement; clear interaction.
	// (PlayerDeathTask case 4 / NPCDeathTask tick 1.)
	c.Reset()
	c.SetUnderAttack(nil)
	c.setMobileInteraction(nil)
	if queue := fighter.MovementQueue(); queue != nil {
		queue.SetBlockMovement(true)
		queue.Reset(true)
	}
	if fighter.IsPlayer() {
		c.SetUntargetable(true)
		// Close all open interfaces (PlayerDeath.ts:160,
		// sendInterfaceRemoval after setUntargetable). The
		// removal runs through the player's dialogue sender,
		// which carries SendInterfaceRemoval; a session-less
		// player has no interface to remove.
		// Death animation (PlayerDeath.ts:163): 836, then
		// the player:dying event (:164) — the case-4 order
		// is combat reset (above) → animation → event.
		// GetDeathAnim is NOT read here: the player
		// accessor returns -1 and the death animation is
		// the constant, as in TS.
		if player := fighter.Player(); player != nil {
			if sender := player.dialogueSender(); sender != nil {
				sender.SendInterfaceRemoval()
			}
			c.owner.PerformAnimation(DeathAnimationID, 0)
			if player.EmitCustomEvent != nil {
				player.EmitCustomEvent(PlayerDyingEventName, &PlayerDyingEvent{
					Player: player, Killer: c.deathKiller,
				})
			}
		}
	} else {
		// NPC death animation (NPCDeathTask.ts:44): the
		// definition's death anim, played on the death
		// tick. A definition with no death anim (-1)
		// plays nothing — TS plays the Animation
		// unconditionally, but Go has no -1 animation to
		// send; the default definition carries 836.
		if anim := c.owner.GetDeathAnim(); anim >= 0 {
			c.owner.PerformAnimation(anim, 0)
		}
		// The NPC death sound (NPCDeathTask.ts:45,
		// NpcCombatSounds.death): a private synth to every
		// player who fought the NPC — the damagers, else
		// the killer alone (Worker C's picker,
		// DeathSoundFor; NoSound when the definition
		// carries none).
		if npc := fighter.NPC(); npc != nil {
			if sound := DeathSoundFor(npc); sound.ID() > 0 {
				listeners := c.deathDamagers
				if len(listeners) == 0 && c.deathKiller != nil {
					listeners = []*Player{c.deathKiller}
				}
				for _, listener := range listeners {
					SendSound(listener, sound)
				}
			}
		}
	}
}

// advanceDeath progresses the death state machine one
// cycle (called from ProcessHitQueue, cycle-guarded).
// Players respawn after 4 ticks; NPCs dispatch the death
// event after their death ticks and respawn. A pending
// remains removal counts down here first, before the
// deathStarted guard: the dead NPC's Process still calls
// ProcessHitQueue each tick while it stays registered, so
// the countdown advances on the tick goroutine.
func (c *Combat) advanceDeath(cycle int64) {
	if c.remainsNPC != nil && c.remainsAdvanceCycle != cycle {
		c.remainsAdvanceCycle = cycle
		c.remainsTicksLeft--
		if c.remainsTicksLeft <= 0 {
			npc := c.remainsNPC
			c.remainsNPC = nil
			c.removeDeadNPC(npc)
		}
	}
	if !c.deathStarted || c.deathDispatched || c.deathAdvanceCycle == cycle {
		return
	}
	c.deathAdvanceCycle = cycle
	if cycle-c.deathStartCycle < int64(c.deathTicks) {
		return
	}
	c.deathDispatched = true
	fighter := combatantFor(c.owner)
	if fighter.IsPlayer() {
		c.respawnPlayer(fighter.Player())
		return
	}
	if npc := fighter.NPC(); npc != nil {
		c.dispatchNpcDeath(npc)
	}
}

// respawnPlayer runs the player respawn tick
// (PlayerDeathTask case 0), in TS order: the death message +
// jingle 90, the should-drop gate, the bones event + ground
// bones, item loss, resetAttributes, kept items returned,
// emitPlayerDefeated for ALL deaths, save "death", the
// onPlayerDeath handled chain, the player:died custom event,
// and — when no hook handled the death — the player:respawn
// custom event and the teleport home. The revival itself (the
// hook's, or the core default) runs last, after save
// "respawn". On error the fallback is resetAttributes + move
// home (PlayerDeath.ts catch); a panic in a hook must not
// leave the player dying forever, so deathStarted clears in
// the deferred fallback on both paths.
func (c *Combat) respawnPlayer(player *Player) {
	defer func() {
		if rec := recover(); rec != nil {
			// PlayerDeath.ts catch: resetAttributes + move
			// home. The panic value is not re-raised: the
			// death machine must finish the respawn (the
			// engine's tick recover, engine.go, is the
			// backstop for reporting).
			resetDeathAttributes(player)
			if player != nil {
				MovePlayerTo(player, DefaultRespawnLocation)
			}
		}
		c.deathStarted = false
	}()
	c.respawnPlayerInner(player)
}

// respawnPlayerInner is the respawn tick without the
// error-path fallback or the deathStarted clear.
func (c *Combat) respawnPlayerInner(player *Player) {
	if player == nil {
		return
	}
	deathPosition := c.owner.Location()
	killer := c.deathKiller
	if c.deathAnnounce {
		player.SendMessage(c.deathMessage)
		sendDeathJingle(player)
	}
	// Bots have no combat-interaction death handling in Go:
	// skipped (no bot interaction system).
	loseItems := true
	if Hooks.ShouldDropItemsOnDeath != nil {
		if answer := Hooks.ShouldDropItemsOnDeath(player, killer); answer != nil {
			loseItems = *answer
		}
	}
	// Bones: the player:death-bones event, then a BONES item
	// to the ground (owner = killer, else player) if allowed.
	// With no ground-item seam the bones are not placed (the
	// seam is wired in production; see InstallDeathWiring).
	bonesAllowed := true
	if player.EmitCustomEvent != nil {
		bones := &DeathBonesEvent{Player: player, Killer: killer, Location: deathPosition, Allow: true}
		player.EmitCustomEvent(DeathBonesEventName, bones)
		bonesAllowed = bones.Allow
	}
	if bonesAllowed && Hooks.DropGroundItem != nil {
		owner := killer
		if owner == nil {
			owner = player
		}
		Hooks.DropGroundItem(NewItem(DeathBonesItemID, 1), deathPosition, owner)
	}
	var itemsToKeep []Item
	if loseItems {
		itemsToKeep = DropDeathItems(player, killer, deathPosition)
	}
	// Restore the player's default attributes (such as stats).
	resetDeathAttributes(player)
	// If the player lost items, the ones a death keeps come back.
	if loseItems {
		ReturnKeptItems(player, itemsToKeep)
	}
	// Notify defeat hooks for all deaths so plugins can apply
	// post-death cleanup even when no killer is present.
	if Hooks.OnPlayerDefeated != nil {
		Hooks.OnPlayerDefeated(killer, player)
	}
	requestDeathSave(player, "death")
	handled := false
	if Hooks.OnPlayerDeath != nil {
		handled = Hooks.OnPlayerDeath(&CombatDeathEvent{
			Victim:       c.owner,
			Killer:       killer,
			Damagers:     c.deathDamagers,
			Location:     deathPosition,
			ItemsLost:    loseItems,
			HasItemsLost: true,
		})
	}
	// Every observer hears the death, whichever death hook took it.
	if player.EmitCustomEvent != nil {
		player.EmitCustomEvent(PlayerDiedEventName, &PlayerDiedEvent{
			Player: player, Killer: killer, ItemsLost: loseItems,
		})
	}
	if !handled {
		// Respawn policies run after every dangerous-death observer.
		respawnHandled := false
		if player.EmitCustomEvent != nil {
			respawn := &PlayerRespawnEvent{Player: player, Killer: killer, Handled: false}
			player.EmitCustomEvent(PlayerRespawnEventName, respawn)
			respawnHandled = respawn.Handled
		}
		if !respawnHandled {
			MovePlayerTo(player, DefaultRespawnLocation)
		}
	}
	requestDeathSave(player, "respawn")
	// The revival: the hook's, or the core default.
	if Hooks.OnPlayerRespawn != nil {
		Hooks.OnPlayerRespawn(player, c.deathMessage)
	} else {
		c.defaultRespawn(player)
	}
}

// defaultRespawn is the core respawn when no hook handles
// it: hitpoints restored, dying/untargetable/blocked
// movement cleared. The teleport home and the item loss
// already ran on the respawn tick (respawnPlayerInner), so
// this step only revives; the weapon-interface re-assign
// duplicates resetDeathAttributes' and is harmless.
func (c *Combat) defaultRespawn(player *Player) {
	fighter := combatantFor(c.owner)
	fighter.SetHitpoints(fighter.MaxHitpoints())
	fighter.SetDying(false)
	c.SetUntargetable(false)
	if queue := fighter.MovementQueue(); queue != nil {
		queue.SetBlockMovement(false)
	}
	// Teleport home already ran on the respawn tick
	// (respawnPlayerInner, unless a hook handled the death).
	// Combat-tab reset assign (Player.ts:280): re-assign the
	// weapon interface for the equipped weapon after respawn,
	// as the login assign does.
	AssignWeaponInterface(player)
}

// dispatchNpcDeath runs the NPC death dispatch
// (NPCDeathTask tick 0): the event fires only with a
// killer; the NPC always respawns (BeginRespawn), after
// the remains respawn delay when the event set one, and
// leaves the world (remove-queue push). With remains set,
// the removal waits: the NPC stays registered for
// RemainsTicks (NPCDeathTask.ts:82-88, NPCRemainsTask),
// counted down by advanceDeath, while the respawn is
// scheduled at dispatch (see the Combat.remainsNPC field
// comment for the respawn-timing divergence). Death Charge
// is skipped (content).
func (c *Combat) dispatchNpcDeath(npc *NPC) {
	event := &CombatDeathEvent{
		Victim:   c.owner,
		Killer:   c.deathKiller,
		Damagers: c.deathDamagers,
		Location: c.owner.Location(),
	}
	if c.deathKiller != nil && Hooks.OnNpcDeath != nil {
		Hooks.OnNpcDeath(event)
	}
	c.deathStarted = false
	// NPCDeathTask.remove (NPCDeathTask.ts:101-117), in TS
	// order: area leave, dying-flag clear, transformation
	// reset, respawn submit, then the remove-queue push. The
	// area leave has no Go form here (no area-leave call
	// exists for a removed NPC; the removal itself drops it
	// from area processing), so the port does the dying-flag
	// clear, the transformation reset, the respawn submit,
	// and the removal. The removal half was missing until Phase
	// 26: the respawn was scheduled but the corpse stayed
	// registered forever. With remains, only the removal
	// waits — the transformation reset runs at the actual
	// removal (removeDeadNPC), so a hook's transform set
	// after dispatch (an Ent trunk at +2 ticks) survives
	// until the remains expire.
	combatantFor(c.owner).SetDying(false)
	if event.RemainsTicks > 0 && event.RespawnTicks > 0 {
		npc.BeginRespawnWithTicks(event.RespawnTicks)
	} else {
		npc.BeginRespawn()
	}
	if event.RemainsTicks > 0 {
		c.remainsNPC = npc
		c.remainsTicksLeft = event.RemainsTicks
		return
	}
	c.removeDeadNPC(npc)
}

// removeDeadNPC is the NPCDeathTask.remove tail (NPCDeathTask
// .ts:101-110): clear the transformation (and the hitpoints
// lock, which a transform's trunk sets) and push the NPC to
// the remove queue. Called at dispatch, or at remains expiry.
func (c *Combat) removeDeadNPC(npc *NPC) {
	npc.SetNpcTransformationID(-1)
	npc.SetHitpointsLocked(false)
	if npcRemover != nil {
		npcRemover(npc)
	}
}
