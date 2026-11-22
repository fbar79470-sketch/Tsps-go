package main

import (
	"log"

	"tsps-go/internal/cache"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// Combat wiring: the cmd/server adapter between the combat
// listeners (internal/listeners), the combat event seam
// (internal/game CombatHooks), and the plugin hub
// (internal/plugins). It follows the Phase 13/14 precedents
// exactly: narrow func fields and lookup interfaces on the
// caller side, translation adapters here, where all sides are
// importable. Game never imports plugins; listeners never import
// plugins.
//
// Five entry points, called by the coordinator's server.go
// hookup (this file only defines the adapters; the hookup lines
// are in server.go):
//
//   - InstallCombatHooks, once in buildServer, before ticking
//     starts: populates the game.CombatHooks seam fields the
//     hub families cover, and installs the NPC combat-method
//     provider registry on the hub (flushing providers queued
//     during plugin load, in registration order).
//   - InstallDeathWiring, once in buildServer, before ticking
//     starts: the death task's world-backed seams — the
//     "death"/"respawn" save requests, the ground-item spawn
//     for bones and dropped items, and the respawn teleport
//     target from the world's live spawn.
//   - InstallCombatSources, once in buildServer, before ticking
//     starts: the world-backed sources the combat core reads
//     through its seams (cycle, clipping, multi-combat,
//     Wilderness), the combat_hits world phase, the NPC
//     respawn + special-restore wiring, and the aggression
//     combat bridge.
//   - InstallSequenceAnimations, once in buildServer, before
//     ticking starts: the cache-backed sequence scanner behind
//     the stall / forced-priority / standing-priority seams
//     and the delayed-animation task submitter.
//   - InstallRangedData, once in buildServer, before ticking
//     starts: the production ranged weapon/ammunition resolver
//     and ammunition data, plus the charge-bow decrement skip
//     on the DecrementAmmo seam.
//   - InstallCombatListeners, once per entering player in
//     AddPlayer's Deps block, AFTER InstallDialogueListeners:
//     the player lookup, the player-option hook adapters, the
//     NPC attack-option gate adapter, and the combat seam. The
//     order matters: InstallDialogueListeners replaces
//     deps.NpcHooks wholesale, so InstallCombatListeners must
//     run after it — it sets only the EmitNpcAttackOption field
//     on the struct InstallDialogueListeners installed, and
//     never clobbers StartDialogue or EmitOperated.
//
// Seam coverage. Twenty of the CombatHooks fields are populated —
// the ones whose hub families Phase 16 ports: OnNpcDeath,
// OnPlayerDeath, OnCombatHitPrepare, OnCombatHitRoll,
// OnCombatHitDamage, OnCombatHitResolved,
// OnCombatAttackDistance, and (Phase 26) OnCanAttack, plus
// (Phase 27) the four spell fields SpellDisabled,
// SpellRuneBypass, CheckSpellRuneSource, and
// ConsumeSpellRuneSource, plus (Phase 28) the five death fields
// OnPlayerDefeated, ShouldDropItemsOnDeath,
// ShouldKeepItemOnDeath, OnPlayerDeathItemDrop, and
// OnPlayerDeathDrops, plus (Phase 28) the skull trio
// ApplySkull/IsSkulled/ExtendSkull, wired to the live skull
// system (internal/game skull.go: ApplySkull always applies
// WHITE — HandleSkull never applies RED and the seam carries
// no type; IsSkulled reads the live timer; ExtendSkull keeps
// the timer and colour). InstallDeathWiring populates two more
// world-backed seams: DropGroundItem (the bones + drops spawn,
// shared with the ammo-drop seam) and RequestSave (the
// "death"/"respawn" save requests). InstallCombatSources
// populates one more: SpellAreaTargets (the barrage
// secondary-target list, Phase 45). The remaining fields stay
// unset (the zero seam is inert; combat code checks every field
// for nil) because their hub families stay table-only this
// phase — OnNpcBeforeDeath, OnPlayerBeforeDeath,
// OnCombatHitRolled, OnAttackTiming, OnAttackSpeed,
// ManipulateHit, CanAttack, ModifyIncomingDamage, CheckAmmo,
// DecrementAmmo, AmmoRecoveryPercent, SpecialAttackCost,
// PrayerDisabled, ModifyMagicMaxHit, ModifyMagicHitBounds,
// ModifyRangedMaxHit, ModifyDefenceRoll, ModifyAccuracyOutcome,
// AccuracyScale, OnPlayerRespawn, QueueDelayedDamage,
// AreaAttackTargets, and RetaliationHomeCheck. (OnCombatHitRolled is a custom event —
// combat:hit-rolled — not one of the 108 hook families, so it
// has no hub registration surface at all.)
//
// Read-back limits, documented at each adapter: the hit-damage
// adapter writes back only a *HitDamage Damage replacement (the
// game event's Damage form; mutations to the shared Hit/Damage
// pointers need no read-back).

// playerListLookup adapts the world's player MobileList to the
// listeners.PlayerLookup interface, mirroring npcListLookup.
type playerListLookup struct {
	list *game.MobileList[*game.Player]
}

var _ listeners.PlayerLookup = playerListLookup{}

// Find is MobileList.Get: the slot's player, or nil on a miss.
func (l playerListLookup) Find(index int) *game.Player {
	if l.list == nil {
		return nil
	}
	player, ok := l.list.Get(index)
	if !ok {
		return nil
	}
	return player
}

// InstallCombatListeners fills the combat seams on deps: the
// player lookup, the player-option hub adapters, the NPC
// attack-option gate adapter, and the combat-attack seam. A nil
// deps is a no-op; a nil list leaves deps.Players unset (the
// unwired drop); a nil hub leaves the hook adapters unset (the
// no-hooks defaults) but still wires the combat seam, which
// needs no hub. Must run after InstallDialogueListeners (see
// the file header).
func InstallCombatListeners(deps *listeners.Deps, hub *plugins.Hub, players *game.MobileList[*game.Player]) {
	if deps == nil {
		return
	}
	if players != nil {
		deps.Players = playerListLookup{list: players}
	}
	if hub != nil {
		deps.PlayerHooks = listeners.PlayerHooks{
			EmitOptionRoute:  playerOptionRouteAdapter(hub),
			EmitPlayerAttack: playerAttackAdapter(hub),
		}
		// Set the field, never the struct: StartDialogue and
		// EmitOperated were installed by
		// InstallDialogueListeners and must survive.
		deps.NpcHooks.EmitNpcAttackOption = npcAttackOptionAdapter(hub)
	}
	deps.CombatAttack = combatAttack
}

// combatAttack is the Deps.CombatAttack seam: the Go form of
// player.getCombat().attack(target)
// (NPCOptionPacketListener.ts:63,
// PlayerOptionPacketListener.ts:26), through the Phase 16
// combat contract (Worker 2's Combat.Attack). Before attacking
// it performs the listeners' autocast-conditional clear, which
// both TS listeners run immediately before attack()
// (NPCOptionPacketListener.ts:62,
// PlayerOptionPacketListener.ts:25): when an autocast spell is
// set, the single-cast spell is cleared, so the click attacks
// with the autocast running. The clear lives here, in the seam
// both listeners share, rather than duplicated in each listener
// — the listeners package never names Combat methods. GetCombat
// never returns nil for a non-nil Mobile; a nil attacker is a
// defensive no-op (the listeners never call it with one).
func combatAttack(attacker *game.Player, target *game.Mobile) {
	if attacker == nil {
		return
	}
	combat := attacker.GetCombat()
	if combat.AutoCastSpell() != nil {
		combat.SetCastSpell(nil)
	}
	combat.Attack(target)
}

// playerOptionRouteAdapter fires the "player:option-route"
// custom event for the attack option and reports whether a
// plugin claimed the slot (PlayerOptionPacketListener.ts:10-15).
// The event is passed by pointer through the opaque payload, so
// a handler setting Handled is visible to the emitter,
// following the ObjectExamineEvent precedent. A nil hub runs
// no handlers and claims nothing.
func playerOptionRouteAdapter(hub *plugins.Hub) func(*listeners.PlayerOptionRouteEvent) bool {
	return func(route *listeners.PlayerOptionRouteEvent) bool {
		if hub == nil || route == nil {
			return false
		}
		ev := &plugins.PlayerOptionRouteEvent{
			Player: route.Player,
			Target: route.Target,
			Option: route.Option,
		}
		hub.EmitCustomEvent(plugins.PlayerOptionRouteEventName, ev)
		if ev.Handled {
			route.Handled = true
		}
		return ev.Handled
	}
}

// playerAttackAdapter delivers the onPlayerAttack event for a
// started attack (PlayerOptionPacketListener.ts:27-28). A nil
// hub or a nil event is a no-op (the hub emit guards both).
func playerAttackAdapter(hub *plugins.Hub) func(*listeners.PlayerAttackEvent) {
	return func(attack *listeners.PlayerAttackEvent) {
		if attack == nil {
			return
		}
		hub.EmitPlayerAttack(&plugins.PlayerAttackEvent{
			Player: attack.Player,
			Target: attack.Target,
		})
	}
}

// npcAttackOptionAdapter runs the onNpcAttackOption gate for an
// NPC combat click (NPCOptionPacketListener.ts:61). A nil hub
// claims nothing (the hub emit guards it); a nil event claims
// nothing. A claim is mirrored onto the listener event's
// Handled flag as well as the return value, matching the
// dual-claim pattern.
func npcAttackOptionAdapter(hub *plugins.Hub) func(*listeners.NpcAttackOptionEvent) bool {
	return func(option *listeners.NpcAttackOptionEvent) bool {
		if hub == nil || option == nil {
			return false
		}
		ev := &plugins.NpcAttackOptionEvent{
			Player:    option.Player,
			NPC:       option.NPC,
			NpcID:     option.NpcID,
			ClickType: option.ClickType,
		}
		claimed := hub.EmitNpcAttackOption(ev)
		if ev.Handled {
			option.Handled = true
		}
		return claimed
	}
}

// InstallCombatHooks populates the game-side combat event seam
// from the hub and installs the NPC combat-method provider
// registry. A nil hub is a no-op: the seam stays zero (inert)
// and no registry is installed. A nil world still installs the
// hit/distance adapters; the death adapters skip their emits
// when they cannot resolve the victim (see findCombatNPC /
// findCombatPlayer). Call once at boot, before ticking starts:
// the game provider registry is not synchronized, and the seam
// is read every tick once combat runs.
func InstallCombatHooks(hub *plugins.Hub, world *game.World) {
	if hub == nil {
		return
	}
	game.Hooks.OnNpcDeath = npcDeathAdapter(hub, world)
	game.Hooks.OnPlayerDeath = playerDeathAdapter(hub, world)
	game.Hooks.OnPlayerDefeated = playerDefeatedAdapter(hub)
	game.Hooks.ShouldDropItemsOnDeath = shouldDropItemsOnDeathAdapter(hub)
	game.Hooks.ShouldKeepItemOnDeath = shouldKeepItemOnDeathAdapter(hub)
	game.Hooks.OnPlayerDeathItemDrop = playerDeathItemDropAdapter(hub)
	game.Hooks.OnPlayerDeathDrops = playerDeathDropsAdapter(hub)
	game.Hooks.OnCombatHitPrepare = combatHitPrepareAdapter(hub)
	game.Hooks.OnCombatHitRoll = combatHitRollAdapter(hub)
	game.Hooks.OnCombatHitDamage = combatHitDamageAdapter(hub)
	game.Hooks.OnCombatHitResolved = combatHitResolvedAdapter(hub)
	game.Hooks.OnCombatAttackDistance = combatAttackDistanceAdapter(hub)
	game.Hooks.OnCanAttack = canAttackAdapter(hub)
	game.Hooks.SpellDisabled = spellDisabledAdapter(hub)
	game.Hooks.SpellRuneBypass = spellRuneBypassAdapter(hub)
	game.Hooks.CheckSpellRuneSource = spellRuneSourceCheckAdapter(hub)
	game.Hooks.ConsumeSpellRuneSource = spellRuneSourceConsumeAdapter(hub)
	// The skull trio (Phase 28, internal/game skull.go):
	// HandleSkull/WouldSkull apply WHITE only, and the seam
	// carries no type, so ApplySkull supplies WHITE; RED is a
	// supported type core never applies (see skull.go).
	game.Hooks.ApplySkull = func(player *game.Player, seconds int) {
		player.ApplySkull(game.SkullTypeWhite, seconds)
	}
	game.Hooks.IsSkulled = func(player *game.Player) bool {
		return player.IsSkulled()
	}
	game.Hooks.ExtendSkull = func(player *game.Player, seconds int) {
		player.ExtendSkull(seconds)
	}
	hub.SetNpcCombatMethodProviderRegistry(combatMethodProviderRegistry{})
}

// InstallDeathWiring installs the death task's world-backed
// seams: the "death"/"respawn" save requests, the ground-item
// spawn for bones and dropped items, and the respawn teleport
// target from the world's live spawn. Call once in buildServer,
// after the ground-item manager and save coordinator exist and
// before the engine starts (next to installTradeHooks, which
// takes the same arguments). A nil saves leaves RequestSave
// unset (the death save is then a documented gap); a nil
// groundItems leaves DropGroundItem unset (bones and drops are
// not placed); a nil world leaves the default respawn location
// (world.json spawn) in place. Wiring DropGroundItem here also
// serves the combat ammo-drop seam, which shares the field.
func InstallDeathWiring(saves *saveCoordinator, groundItems *game.GroundItemManager, world *game.World, logger *log.Logger) {
	if world != nil {
		game.SetDefaultRespawnLocation(world.Spawn())
	}
	if groundItems != nil {
		// ItemOnGroundManager.registerLocation (PlayerDeath.ts
		// bones + drops): the stack registers at the tile,
		// owner-only until the reveal delay, then global —
		// the same construction the trade seam and the drop
		// listener use.
		game.Hooks.DropGroundItem = func(item game.Item, loc game.Location, owner *game.Player) {
			if owner == nil {
				return
			}
			stack := game.NewItemOnGround(
				game.GroundItemSeenByPlayer,
				owner.Username,
				loc,
				item.ID(),
				item.Amount(),
				tradeItemMetaToGround(item.Meta()),
				true, -1, false, nil,
			)
			groundItems.Register(stack)
		}
	}
	if saves != nil {
		// PLAYER_PERSISTENCE.requestSave (PlayerDeath.ts
		// case 0: "death", "respawn"), following the
		// Trading.save precedent: log failures with the TS
		// message. Bots and players without a carried save
		// are refused by snapshot itself.
		game.Hooks.RequestSave = func(player *game.Player, reason string) {
			if player == nil {
				return
			}
			if err := saves.snapshot(player, reason); err != nil {
				logger.Printf("[death] Failed to save %s after %s: %v", player.Username, reason, err)
			}
		}
	}
}

// InstallRangedData installs the production ranged weapon /
// ammunition data (internal/game ranged_data.go): the resolver
// behind GetMethod's RANGED detection and the ammunition data
// CheckAmmo's identity comparison reads. Call once in
// buildServer, before ticking starts: the resolver is a
// package-level var, and the ammunition map is written once
// here (both installs are idempotent, so a second call is a
// no-op in effect). It also chains the charge-bow decrement
// skip onto the DecrementAmmo seam: a charge/ether bow spends
// no stackable ammunition (see game.IsRangedChargeBow), and
// the core decrement would otherwise eat the bow from the
// weapon slot. A hook already installed on the seam runs
// first and wins when it claims the shot.
func InstallRangedData() {
	game.RegisterRangedDataResolver(game.ResolveRangedData)
	game.RegisterRangedAmmunitionData()
	prev := game.Hooks.DecrementAmmo
	game.Hooks.DecrementAmmo = func(player *game.Player, weapon, ammo game.Item, amount int) bool {
		if prev != nil && prev(player, weapon, ammo, amount) {
			return true
		}
		return game.IsRangedChargeBow(weapon.ID())
	}
}

// InstallSequenceAnimations installs the sequence
// (animation) scanner behind the combat core's sequence
// seams (internal/cache seq_scanner.go, the port of
// NpcAnimationScanner.ts): the stall source behind
// Combat.OnAnimationStarted / ExpectAttackAnimation
// (getSequenceStall), the forced-priority source behind
// Mobile.performAnimation's equal-priority rule
// (getSequenceForcedPriority), and the standing-priority
// source behind Player.performAnimation's arrival hold
// (getSequencePriority), plus the delayed-animation task
// submitter behind Mobile.DelayedAnimation /
// DelayedGraphic (the world's TaskManager). Call once in
// buildServer, before ticking starts: the scanner preloads
// the seq archive (preloadSequences, GameBuilder.ts:40).
// A nil facade degrades to the no-cache scanner (every id
// reads the defaults); a nil world leaves the delayed-task
// submitter unset (delayed animations are dropped).
func InstallSequenceAnimations(defs *cache.CacheDefinitions, world *game.World) {
	scanner := cache.SequenceScannerForCache(defs)
	scanner.Preload()
	game.SetSequenceStallSource(scanner.StallTicks)
	game.SetSequenceForcedPrioritySource(scanner.ForcedPriority)
	game.SetSequencePrioritySource(scanner.Priority)
	if world != nil {
		tasks := world.Tasks()
		game.SetDelayedAnimationSubmit(func(delay int, action func()) {
			tasks.Schedule(game.NewTask(delay, func(cycle int64) bool {
				action()
				return true
			}))
		})
	}
}

// InstallCombatSources installs the world-backed sources the
// combat core reads through its seams, plus the combat-owned
// world wiring: the combat_hits phase, the NPC respawn and
// special-restore schedulers, and the aggression combat
// bridge. Call once in buildServer, after the AreaManager and
// RegionManager are installed and before the engine starts.
// A nil world installs nothing; a nil RegionManager leaves the
// clipping source unset (combat routes on open ground, the
// documented no-regions degeneration); a nil Wilderness leaves
// the Wilderness source unset (no PvP ground).
func InstallCombatSources(world *game.World, wilderness *game.Wilderness) {
	if world == nil {
		return
	}
	// Cycle source: the world's process-cycle counter, which
	// the combat timing (attack cooldowns, attacker expiry,
	// retaliation delays) reads.
	game.SetCombatCycleSource(world.ProcessCycle)
	// Clipping source: the RegionManager's tile clipping, which
	// combat routing and projectile-line checks read.
	if rm := world.RegionManager(); rm != nil {
		game.SetCombatClippingSource(rm.GetClipping)
	}
	// Multi-combat source: the AreaManager's InMulti (areas +
	// Wilderness zone fallback), resolved through the
	// fighter's typed identity. An unresolvable fighter reads
	// as single-combat.
	if areas := world.Areas(); areas != nil {
		game.SetCombatMultiSource(func(c game.Combatant) bool {
			if p := c.Player(); p != nil {
				return areas.InMulti(p)
			}
			if n := c.NPC(); n != nil {
				return areas.InMulti(n)
			}
			return false
		})
	}
	// Wilderness source: PvP ground + safe buildings, for the
	// combat permission policy.
	if wilderness != nil {
		game.SetCombatWildernessSource(func(loc game.Location) (bool, bool) {
			return wilderness.IsInLocation(loc), wilderness.IsInSafeBuilding(loc)
		})
	}
	// Barrage secondaries (Phase 45): the world-backed
	// area-target list behind Hooks.SpellAreaTargets
	// (MagicCombatMethod.hits). TS draws from the caster's
	// local players/NPCs (player caster) or the world near
	// the target (NPC caster, World.getNpcsNear); Go has no
	// local lists, so the adapter scans the world's players
	// and NPCs and keeps only the mobiles within radius of
	// the primary target. CanAttackSecondaryTarget re-checks
	// radius, multi-combat and policy, so this is a
	// candidate list, not the verdict — the scan is the
	// same shape as the findCombatNPC/findCombatPlayer
	// fallback scans above.
	game.Hooks.SpellAreaTargets = func(attacker, target *game.Mobile, radius int) []*game.Mobile {
		if target == nil {
			return nil
		}
		var out []*game.Mobile
		for _, p := range world.Players().All() {
			if p != nil && p.Location().IsWithinDistance(target.Location(), radius) {
				out = append(out, &p.Mobile)
			}
		}
		for _, n := range world.NPCs().All() {
			if n != nil && n.Location().IsWithinDistance(target.Location(), radius) {
				out = append(out, &n.Mobile)
			}
		}
		return out
	}
	// The combat_hits world phase sweeps hit queues for
	// entities that did not process their own this tick; the
	// per-cycle guards make the double call a no-op for those
	// that did.
	world.Hooks.CombatHits = game.ProcessAllHitQueues
	// NPC respawn scheduling + unique-holder liveness over
	// this world's queues.
	world.InstallNpcRespawnWiring()
	// Special-attack restore tasks run on this world's
	// TaskManager.
	world.InstallSpecialRestoreWiring()
	// The aggression loop engages through the combat core.
	game.InstallAggressionCombatBridge()
}

// toPluginCombatType converts a game combat type to the hub
// event payload type. The orders match exactly — both ground
// the TS CombatType enum (MELEE, RANGED, MAGIC, CombatType.ts)
// as iota from 0 — so the conversion is a direct cast.
func toPluginCombatType(t game.CombatType) plugins.CombatType {
	return plugins.CombatType(t)
}

// findCombatNPC resolves the NPC behind a death event's Victim
// Mobile. Mobile carries a player predicate (IsPlayer/AsPlayer,
// backed by the combat identity) but no NPC back-pointer —
// isNpc/getAsNpc stay unported — so the adapter reads the
// victim's world index and verifies the slot still holds that
// same Mobile (a reused slot fails the check), falling back to
// a scan of the NPC list. Deaths are rare; the scan is
// acceptable. A nil world or victim, or an unresolvable victim,
// yields nil and the emit is skipped — the hub event requires
// the NPC, and a death for a victim that left the world has no
// plugin audience.
func findCombatNPC(world *game.World, victim *game.Mobile) *game.NPC {
	if world == nil || victim == nil {
		return nil
	}
	if npc, ok := world.NPCs().Get(victim.GetIndex()); ok && &npc.Mobile == victim {
		return npc
	}
	for _, npc := range world.NPCs().All() {
		if npc != nil && &npc.Mobile == victim {
			return npc
		}
	}
	return nil
}

// findCombatPlayer resolves the player behind a death event's
// Victim Mobile, mirroring findCombatNPC over the player list.
func findCombatPlayer(world *game.World, victim *game.Mobile) *game.Player {
	if world == nil || victim == nil {
		return nil
	}
	if player, ok := world.Players().Get(victim.GetIndex()); ok && &player.Mobile == victim {
		return player
	}
	for _, player := range world.Players().All() {
		if player != nil && &player.Mobile == victim {
			return player
		}
	}
	return nil
}

// npcDeathAdapter translates a game NPC-death event to the hub
// NpcDeathEvent (NPCDeathTask.ts:58-75). The remains read-back
// applies the hook-set remains to the game event: RemainsTicks
// takes the remains ticks (only when positive — the consumer
// reads remains.ticks > 0), RespawnTicks takes the respawn
// delay when the hook set one. An unresolvable victim skips
// the emit.
func npcDeathAdapter(hub *plugins.Hub, world *game.World) func(*game.CombatDeathEvent) {
	return func(e *game.CombatDeathEvent) {
		if e == nil {
			return
		}
		npc := findCombatNPC(world, e.Victim)
		if npc == nil {
			return
		}
		ev := &plugins.NpcDeathEvent{
			Killer:   e.Killer,
			Damagers: e.Damagers,
			NPC:      npc,
			NpcID:    npc.ContentID(),
			Location: e.Location,
		}
		hub.EmitNpcDeath(ev)
		if ev.Remains != nil && ev.Remains.Ticks > 0 {
			e.RemainsTicks = ev.Remains.Ticks
			if ev.Remains.HasRespawnTicks {
				e.RespawnTicks = ev.Remains.RespawnTicks
			}
		}
	}
}

// playerDeathAdapter translates a game player-death event to
// the hub PlayerDeathEvent (PlayerDeath.ts:110-130). ItemsLost
// is never populated: the death item-loss pipeline is not
// ported (combat_death.go), so HasItemsLost stays false —
// hooks read "unset", never a false claim that nothing was
// lost. The handled claim has NO read-back channel: the game
// death event carries no Handled field, so a hook that takes
// the death over cannot suppress the core respawn default;
// that needs a seam field the combat core does not define yet.
// An unresolvable victim skips the emit.
func playerDeathAdapter(hub *plugins.Hub, world *game.World) func(*game.CombatDeathEvent) bool {
	return func(e *game.CombatDeathEvent) bool {
		if e == nil {
			return false
		}
		player := findCombatPlayer(world, e.Victim)
		if player == nil {
			return false
		}
		handled := hub.EmitPlayerDeath(&plugins.PlayerDeathEvent{
			Player:       player,
			Killer:       e.Killer,
			ItemsLost:    e.ItemsLost,
			HasItemsLost: e.HasItemsLost,
		})
		e.Handled = handled
		return handled
	}
}

// playerDefeatedAdapter translates a game player-defeated event
// to the hub event (PlayerDeath.ts emitPlayerDefeated): run-all
// hooks, for every death, killer nil for an environmental death.
// No read-back.
func playerDefeatedAdapter(hub *plugins.Hub) func(killer, victim *game.Player) {
	return func(killer, victim *game.Player) {
		hub.EmitPlayerDefeated(&plugins.PlayerDefeatedEvent{Killer: killer, Victim: victim})
	}
}

// shouldDropItemsOnDeathAdapter translates the death task's
// should-drop gate to the hub first-answer gate
// (PlayerDeath.ts:110-113): the first non-nil answer wins; nil
// means the death drops items.
func shouldDropItemsOnDeathAdapter(hub *plugins.Hub) func(*game.Player, *game.Player) *bool {
	return func(player, killer *game.Player) *bool {
		return hub.EmitShouldDropItemsOnDeath(player, killer)
	}
}

// shouldKeepItemOnDeathAdapter translates the kept-items
// ranking's per-item query to the hub first-answer gate
// (PlayerDeath.ts:301-313): true keeps the item beyond the
// count, false never keeps it, nil means no opinion.
func shouldKeepItemOnDeathAdapter(hub *plugins.Hub) func(*game.Player, game.Item) *bool {
	return func(player *game.Player, item game.Item) *bool {
		return hub.EmitShouldKeepItemOnDeath(player, item)
	}
}

// playerDeathItemDropAdapter translates a game per-item death
// drop to the hub event (PlayerDeath.ts dropItems) and reads the
// handled flag, SuppressDefaultDrop, and ExtraDrops back: the
// death task reads all three whether or not a hook handled it.
func playerDeathItemDropAdapter(hub *plugins.Hub) func(*game.DeathItemDropEvent) bool {
	return func(e *game.DeathItemDropEvent) bool {
		if e == nil {
			return false
		}
		ev := &plugins.PlayerDeathItemDropEvent{
			Player:          e.Player,
			Killer:          e.Killer,
			Item:            e.Item,
			Kept:            e.Kept,
			Location:        e.Location,
			ShouldDropItems: e.ShouldDropItems,
			DropEligible:    e.DropEligible,
		}
		handled := hub.EmitPlayerDeathItemDrop(ev)
		e.Handled = ev.Handled
		e.SuppressDefaultDrop = ev.SuppressDefaultDrop
		e.ExtraDrops = ev.ExtraDrops
		return handled
	}
}

// playerDeathDropsAdapter translates a game death-drops event to
// the hub event (PlayerDeath.ts dropItems) and reads Items and
// Untradeables back: a hook (a grave) takes items by removing
// them, and whatever is left falls to the floor or is kept.
func playerDeathDropsAdapter(hub *plugins.Hub) func(*game.DeathDropsEvent) {
	return func(e *game.DeathDropsEvent) {
		if e == nil {
			return
		}
		ev := &plugins.PlayerDeathDropsEvent{
			Player:       e.Player,
			Killer:       e.Killer,
			Location:     e.Location,
			Items:        e.Items,
			Untradeables: e.Untradeables,
			Layout: plugins.PlayerDeathLayout{
				Inventory: e.Layout.Inventory,
				Equipment: e.Layout.Equipment,
			},
		}
		hub.EmitPlayerDeathDrops(ev)
		e.Items = ev.Items
		e.Untradeables = ev.Untradeables
	}
}

// combatHitPrepareAdapter translates a game hit-prepare event
// to the hub event (PendingHit.ts:382-384). Hooks observe; no
// read-back.
func combatHitPrepareAdapter(hub *plugins.Hub) func(*game.CombatHitPrepareEvent) {
	return func(e *game.CombatHitPrepareEvent) {
		if e == nil {
			return
		}
		hub.EmitCombatHitPrepare(&plugins.CombatHitPrepareEvent{
			Attacker:   e.Attacker,
			Target:     e.Target,
			CombatType: toPluginCombatType(e.CombatType),
		})
	}
}

// combatHitRollAdapter translates a game hit-roll event to the
// hub event (PendingHit.ts:420-424, 476-479) and reads the
// three roll flags back: the pipeline applies them to the roll.
func combatHitRollAdapter(hub *plugins.Hub) func(*game.CombatHitRollEvent) {
	return func(e *game.CombatHitRollEvent) {
		if e == nil {
			return
		}
		ev := &plugins.CombatHitRollEvent{
			Attacker:   e.Attacker,
			Target:     e.Target,
			CombatType: toPluginCombatType(e.CombatType),
		}
		hub.EmitCombatHitRoll(ev)
		e.ForceAccurate = ev.ForceAccurate
		e.ForceMaxHit = ev.ForceMaxHit
		e.BypassProtectionPrayer = ev.BypassProtectionPrayer
	}
}

// combatHitDamageAdapter translates a game hit-damage event to
// the hub event (PendingHit.ts:531-540) and reads Accurate back,
// plus a replaced Damage. Hit (the PendingHit) and Damage (the
// HitDamage) pass through opaquely as shared pointers (nil stays
// nil, so the emit's nil-Damage guard applies): a hook mutating
// either mutates the pipeline's values directly, and a hook that
// replaces Damage with a different *HitDamage has the
// replacement written back — a non-*HitDamage replacement leaves
// the rolled damage unchanged, because the game event has no
// other Damage form.
func combatHitDamageAdapter(hub *plugins.Hub) func(*game.CombatHitDamageEvent) {
	return func(e *game.CombatHitDamageEvent) {
		if e == nil {
			return
		}
		ev := &plugins.CombatHitDamageEvent{
			Attacker:   e.Attacker,
			Target:     e.Target,
			CombatType: toPluginCombatType(e.CombatType),
			Accurate:   e.Accurate,
		}
		if e.Hit != nil {
			ev.Hit = e.Hit
		}
		if e.Damage != nil {
			ev.Damage = e.Damage
		}
		hub.EmitCombatHitDamage(ev)
		if d, ok := ev.Damage.(*game.HitDamage); ok {
			e.Damage = d
		}
		e.Accurate = ev.Accurate
	}
}

// combatHitResolvedAdapter translates a game hit-resolved event
// to the hub event (CombatFactory.ts:1074). Hit passes through
// opaquely as the shared *PendingHit pointer (nil stays nil, so
// the emit's hit guard applies). Hooks observe; no read-back.
func combatHitResolvedAdapter(hub *plugins.Hub) func(*game.CombatHitResolvedEvent) {
	return func(e *game.CombatHitResolvedEvent) {
		if e == nil {
			return
		}
		ev := &plugins.CombatHitResolvedEvent{
			Attacker: e.Attacker,
			Target:   e.Target,
		}
		if e.Hit != nil {
			ev.Hit = e.Hit
		}
		hub.EmitCombatHitResolved(ev)
	}
}

// combatAttackDistanceAdapter translates a game
// attack-distance event to the hub event (CombatRange.ts:74-85)
// and writes the emit's floored distance back: the hub emit
// floors at 1, matching the TS trunc + floor.
func combatAttackDistanceAdapter(hub *plugins.Hub) func(*game.CombatAttackDistanceEvent) {
	return func(e *game.CombatAttackDistanceEvent) {
		if e == nil {
			return
		}
		e.Distance = hub.EmitCombatAttackDistance(&plugins.CombatAttackDistanceEvent{
			Attacker:   e.Attacker,
			Target:     e.Target,
			CombatType: toPluginCombatType(e.CombatType),
			ManualCast: e.ManualCast,
			Distance:   e.Distance,
		})
	}
}

// canAttackAdapter translates a game can-attack gate query to the hub
// onCanAttack emit (PluginManager.emitCanAttack, PluginManager.ts:1360-1377)
// and returns its tri-state answer: false denies, true allows, nil is no
// opinion. The method passes through (nil when the caller has none).
func canAttackAdapter(hub *plugins.Hub) func(attacker, target *game.Mobile, method game.CombatMethod) *bool {
	return func(attacker, target *game.Mobile, method game.CombatMethod) *bool {
		if hub == nil {
			return nil
		}
		return hub.EmitCanAttack(attacker, target, method)
	}
}

// spellDisabledAdapter translates a game spell-disabled gate query
// to the hub onSpellDisabled emit (PluginManager.emitSpellDisabled,
// PluginManager.ts:1824-1849) and returns its collapsed answer:
// true means a hook disabled the spell, false means no hook did
// (TS null collapses to false — every caller compares `===
// true`). The spell passes through (nil when the caller has none).
func spellDisabledAdapter(hub *plugins.Hub) func(*game.Player, game.Spellbook, int, game.CombatSpell) bool {
	return func(player *game.Player, spellbook game.Spellbook, spellID int, spell game.CombatSpell) bool {
		if hub == nil {
			return false
		}
		return hub.EmitSpellDisabled(player, spellbook, spellID, spell)
	}
}

// spellRuneBypassAdapter translates a game spell-rune-bypass query
// to the hub onSpellRuneBypass emit
// (PluginManager.emitSpellRuneBypass, PluginManager.ts:1851-1876)
// and returns its collapsed answer: true means a hook bypasses
// the rune cost (or the single rune), false means no hook did.
// RuneID is the rune being queried, or 0 for the whole-cost
// query (TS passes runeId undefined there).
func spellRuneBypassAdapter(hub *plugins.Hub) func(*game.Player, game.Spellbook, int, int) bool {
	return func(player *game.Player, spellbook game.Spellbook, spellID, runeID int) bool {
		if hub == nil {
			return false
		}
		return hub.EmitSpellRuneBypass(player, spellbook, spellID, runeID)
	}
}

// spellRuneSourceCheckAdapter translates a game rune-source check
// to Hub.CheckSpellRuneSource (PluginManager.checkSpellRuneSource,
// PluginManager.ts:5056-5074): true when a registered source can
// supply every missing item. With no source registered it answers
// false (the no-source default — no rune pouch exists in Go).
func spellRuneSourceCheckAdapter(hub *plugins.Hub) func(*game.Player, []game.Item) bool {
	return func(player *game.Player, items []game.Item) bool {
		if hub == nil {
			return false
		}
		return hub.CheckSpellRuneSource(player, items)
	}
}

// spellRuneSourceConsumeAdapter translates a game rune-source
// consumption to Hub.ConsumeSpellRuneSource
// (PluginManager.consumeSpellRuneSource, PluginManager.ts:5076-5092):
// the shortfall is deducted from the first source that can supply
// it. With no source registered it is a no-op.
func spellRuneSourceConsumeAdapter(hub *plugins.Hub) func(*game.Player, []game.Item) {
	return func(player *game.Player, items []game.Item) {
		if hub == nil {
			return
		}
		hub.ConsumeSpellRuneSource(player, items)
	}
}

// combatMethodProviderRegistry adapts the hub's provider
// registration surface to Worker 2's game registry
// (RegisterNPCCombatMethodProvider). The provide closure
// already applies the id-set check and the singleton/per-attack
// construction (see Api.RegisterNpcCombatMethodProvider); the
// game registry applies its own id-set check as well, exactly
// as TS stores both. The closure's `any` result is asserted to
// game.CombatMethod: a constructor returning any other type
// yields nil ("no method"), and the factory falls back to the
// NPC's definition method — the adapter never panics the tick
// on a content type error. Constructors must return untyped
// nil for "no method": a typed-nil method value would assert
// successfully and panic at use.
type combatMethodProviderRegistry struct{}

var _ plugins.NpcCombatMethodProviderRegistry = combatMethodProviderRegistry{}

// RegisterProvider forwards one provider to the game registry,
// in registration order.
func (combatMethodProviderRegistry) RegisterProvider(pluginName string, npcIDs []int, provide func(npc *game.NPC) any) {
	game.RegisterNPCCombatMethodProvider(pluginName, npcIDs, game.NPCCombatMethodProviderFunc(func(npc *game.NPC) game.CombatMethod {
		if provide == nil {
			return nil
		}
		method, _ := provide(npc).(game.CombatMethod)
		return method
	}))
}
