// Combat and death hooks: the Go form of the combat/NPC families of
// PluginApi (PluginTypes.ts) that the engine can fire since phase 16:
// onNpcDeath, onNpcAttackOption, onPlayerAttack, onPlayerDeath,
// onPlayerDeathItemDrop, onPlayerDeathDrops, onCombatHitPrepare,
// onCombatHitRoll, onCombatHitDamage, onCombatHitResolved,
// onCombatAttackDistance, and onCanAttack (phase 26), plus the
// cross-plugin emitNpcAttackOption member and the
// registerNpcCombatMethodProvider member.
//
// Every emit below ports its PluginManager.emit* body, including the
// entry guards, the registration wrappers, the hook ordering, and the
// short-circuit/run-all shape — the families differ from each other,
// and those differences are behavior content relies on:
//
//   - emitNpcDeath (PluginManager.ts:1322-1329): run ALL hooks; no emit
//     guards (the zero-hooks early return is unobservable). The
//     registration wrapper skips events without a killer or NPC
//     (PluginManager.ts:3528-3541).
//   - emitNpcAttackOption (PluginManager.ts:1277-1287): short-circuit
//     handled gate (runHandledGate, gates.go); entry guards (nil
//     event/player/NPC return false) but NO pre-handled early-false —
//     a pre-handled event returns true with zero hooks run, matching
//     the loop shape. No registration wrapper (PluginManager.ts:
//     3472-3476).
//   - emitPlayerAttack (PluginManager.ts:1501-1508): run ALL hooks;
//     no emit guards. The registration wrapper skips events without
//     a player or target (PluginManager.ts:3729-3742).
//   - emitPlayerDeath (PluginManager.ts:1724-1735): short-circuit
//     handled gate; entry guards (nil event/player, or a pre-handled
//     event, return FALSE — unlike emitNpcAttackOption). The
//     registration wrapper skips events without a player or already
//     handled (PluginManager.ts:3885-3898).
//   - emitPlayerDeathItemDrop (PluginManager.ts:1591-1616):
//     short-circuit handled gate; entry guards (nil event/player/
//     item, or pre-handled, return false). The registration wrapper
//     skips events without a player or item (PluginManager.ts:
//     3815-3828).
//   - emitPlayerDeathDrops (PluginManager.ts:1618-1625): run ALL
//     hooks; entry guards (nil event/player). No registration
//     wrapper (PluginManager.ts:3809-3813).
//   - emitShouldDropItemsOnDeath (PluginManager.ts:1548-1569):
//     the first-answer gate (runFirstAnswerGate, gates.go):
//     the first hook to set shouldDrop wins, later hooks never
//     run; with zero hooks TS returns null without building an
//     event. The game-side seam is CombatHooks
//     .ShouldDropItemsOnDeath (internal/game), wired by the
//     coordinator in cmd/server/combatwiring.go; the emit site
//     is the death task's respawn tick (default true).
//   - emitShouldKeepItemOnDeath (PluginManager.ts:1571-1589):
//     the first-answer gate: the first hook to set keep wins.
//     Unlike emitShouldDropItemsOnDeath, TS builds the event
//     even with zero hooks (no length early return); the
//     outcome is the same null. The game-side seam is
//     CombatHooks.ShouldKeepItemOnDeath, wired in
//     cmd/server/combatwiring.go; the emit site is the death
//     task's kept-items ranking, per item.
//   - emitPlayerDefeated (PluginManager.ts:1901-1905): run ALL
//     hooks; no emit guards (the registration wrapper skips
//     events without a victim, PluginManager.ts:4028-4041).
//     The game-side seam is CombatHooks.OnPlayerDefeated,
//     wired in cmd/server/combatwiring.go; the emit site is
//     the death task's respawn tick, for ALL deaths.
//   - emitCombatHitPrepare/Roll/Damage/Resolved (PluginManager.ts:
//     1789-1815): run ALL hooks; entry guards (attacker/target, plus
//     damage for HitDamage, plus hit for Resolved — HitDamage does
//     NOT guard hit). No registration wrappers (PluginManager.ts:
//     3946-3956, 3970-3973).
//   - emitCombatAttackDistance (PluginManager.ts:1817-1822): run ALL
//     hooks, then return max(1, trunc(distance)). NO entry guards in
//     TS; the Go nil-event guard returns the floor (1) and is
//     documented at the emit. No registration wrapper
//     (PluginManager.ts:3974-3977).
//   - emitCanAttack (PluginManager.ts:1360-1377): the allow-veto
//     gate (runAllowVetoGate, gates.go): allow is reset to
//     undecided before EACH hook, a deny returns deny
//     immediately, an allow sticks but a later deny still
//     vetoes, and silence is undecided (nil). The game-side
//     seam is CombatHooks.OnCanAttack (internal/game), wired
//     by the coordinator in cmd/server/combatwiring.go; the
//     emit sites are CanAttackPermission's duel branch (a veto
//     denies, anything else passes — CombatFactory.ts:779-789)
//     and canAttackByPolicy's global hook (after the area
//     hooks — CombatFactory.ts:860-871).
//
// EMIT-SITE SEAM (game never imports plugins): the combat and death
// pipelines live in internal/game (Worker 2) and cannot call these
// emits. They call the game-side CombatHooks seam
// (internal/game/combat_hooks.go), whose func fields the cmd/server
// adapter populates from these Hub emits (see
// cmd/server/combatwiring.go), following the phase 15 area
// queue/flush pattern. The two listener families fire from the
// listeners package instead: onNpcAttackOption gates the NPC attack
// click and onPlayerAttack observes the player attack click, both
// through Deps hook seams the same adapter wires. Death-task call
// sites (NPCDeathTask.ts:58-75, PlayerDeath.ts:110-117, 190-262)
// and hit-pipeline call sites (PendingHit.ts:383, 421, 479, 538,
// CombatFactory.ts:1074, CombatRange.ts:77) are Worker 2's; this
// file owns only the hub side.
//
// TYPING DECISIONS, each grounded in the TS call sites:
//
//   - Killer/damagers/targets of the death and player-attack events
//     are *game.Player: Combat.getKiller/getRecentDamagers return
//     Player types (Combat.ts:706-740), and PlayerDeath.dropItems
//     takes killer: Player | null (PlayerDeath.ts:172). A nil killer
//     is TS null (an environmental death).
//   - Attacker/target of the hit and attack-distance events are
//     *game.Mobile: PendingHit and CombatRange work in Mobiles of
//     either kind (PendingHit.ts:383, CombatRange.ts:74-85). LIMIT:
//     Mobile has no kind predicates in Go (IsPlayer/AsPlayer live
//     on Player/NPC only), so a handler cannot downcast a Mobile to
//     its concrete type; it reads the Mobile surface (location,
//     combat, attributes, timers). Kind accessors on Mobile belong
//     to the combat-core work, not to this file.
//   - Hit and Damage on the hit events are `any`: the pipeline's
//     PendingHit and HitDamage values, which TS types `any` in the
//     event interfaces (PluginTypes.ts:620-659) and which have no Go
//     types yet (Worker 2 owns them). The hub passes them opaquely;
//     handlers that need them assert to the concrete game types.
//     Inventing accessor interfaces here would impose a contract
//     Worker 2 never agreed to.
//   - CombatType is the hub event payload enum, mirroring the TS
//     CombatType enum order (MELEE, RANGED, MAGIC — CombatType.ts).
//     It is NOT the core.CombatType class mirror (core.go keeps
//     CombatType in its NOT-mirrored list): content reads the type
//     from event payloads, never from Core.
//   - ForceMaxHit on the roll event is a plain bool, not optional:
//     every PendingHit roll event is built with forceMaxHit: false
//     (PendingHit.ts:420, 476) and the consumer reads it by
//     truthiness, so "unset" and false are the same value.
//
// NOT PORTED — the combat families whose emit sites need Worker 2
// pipeline types or call sites outside the Phase 16 contracts (see
// the unfired-family table in hooks.go): onNpcBeforeDeath,
// onNpcHitModify, onPlayerHitModify, onPlayerBeforeDeath,
// onPlayerDealtDamage, onAttackTiming, onAnimationStall,
// onAttackSpeed, and onNpcAggressionTolerance.
// onCanAttack landed in phase 26: its allow-veto gate helper already existed (runAllowVetoGate,
// gates.go), and the phase added the hub family plus the
// CombatHooks.OnCanAttack seam and the canAttackPermission emit
// sites (the duel branch and canAttackByPolicy's global hook).
// onShouldDropItemsOnDeath, onShouldKeepItemOnDeath, and
// onPlayerDefeated landed in phase 28 with the death item-loss
// pipeline: the hub families plus the CombatHooks seams
// (ShouldDropItemsOnDeath, ShouldKeepItemOnDeath,
// OnPlayerDefeated) and the respawn-tick emit sites.
// onSpellDisabled and onSpellRuneBypass landed in phase 27 in
// spells.go, with the CombatHooks spell seams wired in
// cmd/server/combatwiring.go; registerSpellRuneSource is
// Api.RegisterSpellRuneSource in the same file.
// onPlayerOption and
// onItemOnNpc stay table-only with their listeners (walk-to-entity
// is NOT STARTED; see internal/listeners). registerNpcCombatOption
// (PluginTypes.ts:1540) stays table-only: it mutates the
// NpcDefinition combat-options set, which has no Go registry.

package plugins

import (
	"log"
	"sync"

	"tsps-go/internal/game"
)

// CombatType is the combat style of a hit or an attack-distance
// query: the TS CombatType enum (CombatType.ts: MELEE, RANGED,
// MAGIC) as a Go enum, in the same order. It is the event payload
// type only; see the file header.
type CombatType int

const (
	// CombatTypeMelee is CombatType.MELEE.
	CombatTypeMelee CombatType = iota
	// CombatTypeRanged is CombatType.RANGED.
	CombatTypeRanged
	// CombatTypeMagic is CombatType.MAGIC.
	CombatTypeMagic
)

// NpcDeathRemains is the `remains` a handler sets on an NpcDeathEvent
// to leave the NPC in the world after it dies
// (PluginNpcDeathEvent.remains, PluginTypes.ts:256-260): it stays
// `ticks` ticks before it is removed, then respawns `respawnTicks`
// later (default: its definition's respawn). A nil Remains on the
// event is TS null (no remains). RespawnTicks without
// HasRespawnTicks means "unset" (the definition default); the
// consumer (NPCDeathTask.ts:72, 82-88) reads remains.ticks > 0.
type NpcDeathRemains struct {
	Ticks int
	// RespawnTicks is the respawn delay, when HasRespawnTicks.
	RespawnTicks    int
	HasRespawnTicks bool
}

// NpcDeathEvent is the payload of the onNpcDeath hooks. It mirrors
// PluginNpcDeathEvent (PluginTypes.ts:248-262): the killer, every
// player who damaged the NPC recently (the killer among them), the
// NPC, the content NPC id, the death tile, and the optional
// remains. The death task emits it only when a killer exists
// (NPCDeathTask.ts:58); Killer is nevertheless a pointer, because
// the registration wrapper guards on its presence and a nil killer
// must be representable. Damagers is nil when the task recorded
// none (TS passes the array, possibly empty; nil and empty are
// the same "no damagers" reading).
type NpcDeathEvent struct {
	Killer   *game.Player
	Damagers []*game.Player
	NPC      *game.NPC
	NpcID    int
	Location game.Location
	Remains  *NpcDeathRemains
}

// NpcAttackOptionEvent is the payload of the onNpcAttackOption hooks.
// It mirrors PluginNpcAttackOptionEvent (PluginTypes.ts:190-196):
// the player, the NPC, the content NPC id, the click type, and the
// handled flag. A hook that sets Handled takes the click over: the
// listener does not attack (NPCOptionPacketListener.ts:61).
type NpcAttackOptionEvent struct {
	Player    *game.Player
	NPC       *game.NPC
	NpcID     int
	ClickType int
	Handled   bool
}

// PlayerAttackEvent is the payload of the onPlayerAttack hooks. It
// mirrors PluginPlayerAttackEvent (PluginTypes.ts:450-453): the
// attacking player and the target player. It fires AFTER the attack
// starts (PlayerOptionPacketListener.ts:27-28); hooks observe, they
// cannot veto.
type PlayerAttackEvent struct {
	Player *game.Player
	Target *game.Player
}

// PlayerDeathEvent is the payload of the onPlayerDeath hooks. It
// mirrors PluginPlayerDeathEvent (PluginTypes.ts:536-542): the
// player, the killer (nil for an environmental death), whether the
// player lost items, and the handled flag. A hook that sets Handled
// takes the death over (a quest's own respawn): the default respawn
// policy is skipped (PlayerDeath.ts:110-130). ItemsLost is always
// set by the death task (PlayerDeath.ts:113); HasItemsLost
// distinguishes "set" from a hand-built event that left it unset.
type PlayerDeathEvent struct {
	Player       *game.Player
	Killer       *game.Player
	ItemsLost    bool
	HasItemsLost bool
	Handled      bool
}

// PlayerDeathItemDropEvent is the payload of the
// onPlayerDeathItemDrop hooks. It mirrors
// PluginPlayerDeathItemDropEvent (PluginTypes.ts:489-505): the
// player, the killer, the item, whether the death keeps it
// (protected), the death tile, whether the death drops items at
// all, whether core would create a normal floor item for it, the
// suppress-default-drop flag, the handled flag, and the extra
// drops. Hooks add items to ExtraDrops (a ring's imbue paid out as
// coins); core loses them with the rest (PlayerDeath.ts:203-208).
// SuppressDefaultDrop prevents the normal floor-item spawn without
// stopping later death-drop hooks (PlayerDeath.ts:213-216); it is
// read even when the event ends unhandled. Item is a game.Item
// value: handlers read its id/amount/meta, never mutate the
// player's containers through it.
type PlayerDeathItemDropEvent struct {
	Player              *game.Player
	Killer              *game.Player
	Item                game.Item
	Kept                bool
	Location            game.Location
	ShouldDropItems     bool
	DropEligible        bool
	SuppressDefaultDrop bool
	Handled             bool
	ExtraDrops          []game.Item
}

// PlayerDeathLayout is the per-slot item-id layout before the death
// emptied the containers (-1 for an empty slot): the `layout` field
// of PluginPlayerDeathDropsEvent (PluginTypes.ts:521).
type PlayerDeathLayout struct {
	Inventory []int
	Equipment []int
}

// PlayerDeathDropsEvent is the payload of the onPlayerDeathDrops
// hooks. It mirrors PluginPlayerDeathDropsEvent
// (PluginTypes.ts:512-522): everything a death is about to lose,
// once, after the per-item hooks (also when that is nothing). A
// hook takes items by removing them from Items (headed for the
// floor) or Untradeables (otherwise kept) — in Go, by reassigning
// the slice; whatever is left falls to the floor or is kept, as
// without the hook (PlayerDeath.ts:232-244). The emit runs even
// when nothing is lost: a death still changes an existing grave.
type PlayerDeathDropsEvent struct {
	Player       *game.Player
	Killer       *game.Player
	Location     game.Location
	Items        []game.Item
	Untradeables []game.Item
	Layout       PlayerDeathLayout
}

// ShouldDropItemsOnDeathEvent is the payload of the
// onShouldDropItemsOnDeath hooks. It mirrors
// PluginShouldDropItemsOnDeathEvent (PluginTypes.ts:477-481):
// the player, the killer (nil for an environmental death),
// and the shouldDrop answer. ShouldDrop is a *bool: nil is TS
// null (undecided — the first-answer gate keeps asking later
// hooks), non-nil is the hook's answer. The death task reads
// the first non-nil answer and defaults to dropping items
// (PlayerDeath.ts:110-113, via emitShouldDropItemsOnDeath,
// PluginManager.ts:1548-1569).
type ShouldDropItemsOnDeathEvent struct {
	Player     *game.Player
	Killer     *game.Player
	ShouldDrop *bool
}

// ShouldKeepItemOnDeathEvent is the payload of the
// onShouldKeepItemOnDeath hooks. It mirrors
// PluginShouldKeepItemOnDeathEvent (PluginTypes.ts:483-487):
// the player, the item, and the keep answer. Keep is a *bool:
// nil is TS null (undecided), true keeps the item beyond the
// kept count, false never keeps it. The death task consults
// it per item while ranking the kept ones
// (PlayerDeath.ts:301-313, via emitShouldKeepItemOnDeath,
// PluginManager.ts:1571-1589).
type ShouldKeepItemOnDeathEvent struct {
	Player *game.Player
	Item   game.Item
	Keep   *bool
}

// PlayerDefeatedEvent is the payload of the onPlayerDefeated
// hooks. It mirrors PluginPlayerDefeatedEvent
// (PluginTypes.ts:686-689): the killer (nil for an
// environmental death) and the victim. Hooks run for ALL
// deaths (PlayerDeath.ts emitPlayerDefeated, PluginManager
// .ts:1901-1905): content applies post-death cleanup here
// even with no killer.
type PlayerDefeatedEvent struct {
	Killer *game.Player
	Victim *game.Player
}

// CombatHitPrepareEvent is the payload of the onCombatHitPrepare
// hooks. It mirrors PluginCombatHitPrepareEvent
// (PluginTypes.ts:614-618): a real hit is being prepared, before
// accuracy, damage, and the target HP cap. It fires once per hit
// batch, only when the batch is non-empty (PendingHit.ts:382-384).
type CombatHitPrepareEvent struct {
	Attacker   *game.Mobile
	Target     *game.Mobile
	CombatType CombatType
}

// CombatHitRollEvent is the payload of the onCombatHitRoll hooks.
// It mirrors PluginCombatHitRollEvent (PluginTypes.ts:620-628):
// one accuracy roll per hitsplat. Handlers may set ForceAccurate
// (the roll lands), ForceMaxHit (an accurate roll lands on the
// maximum hit), or BypassProtectionPrayer (the overhead is
// ignored); the pipeline reads all three back after the emit
// (PendingHit.ts:420-424, 476-479). All three start false, as TS
// builds them (see the file header).
type CombatHitRollEvent struct {
	Attacker               *game.Mobile
	Target                 *game.Mobile
	CombatType             CombatType
	ForceAccurate          bool
	ForceMaxHit            bool
	BypassProtectionPrayer bool
}

// CombatHitDamageEvent is the payload of the onCombatHitDamage
// hooks. It mirrors PluginCombatHitDamageEvent
// (PluginTypes.ts:647-655): one rolled hitsplat, after its
// accuracy and damage are rolled and before the damage is capped
// to the target's hitpoints. Handlers may change Damage (a
// HitDamage) and Accurate, which the hit then keeps
// (PendingHit.ts:531-540). Hit is the PendingHit being rolled
// (opaque `any`; see the file header) and Damage is its HitDamage
// (opaque `any`); Accurate is read back after the emit.
type CombatHitDamageEvent struct {
	Attacker   *game.Mobile
	Target     *game.Mobile
	CombatType CombatType
	Hit        any
	Damage     any
	Accurate   bool
}

// CombatHitResolvedEvent is the payload of the onCombatHitResolved
// hooks. It mirrors PluginCombatHitResolvedEvent
// (PluginTypes.ts:657-660): a hit has resolved (landed). Hit is
// the resolved hit (opaque `any`; see the file header). It fires
// from the hit-resolution path (CombatFactory.ts:1074).
type CombatHitResolvedEvent struct {
	Attacker *game.Mobile
	Target   *game.Mobile
	Hit      any
}

// CombatAttackDistanceEvent is the payload of the
// onCombatAttackDistance hooks. It mirrors
// PluginCombatAttackDistanceEvent (PluginTypes.ts:634-641):
// combat asks how far an attacker reaches. Handlers may lower or
// raise Distance; it never goes below 1. ManualCast is a player's
// single click-cast spell (CombatRange.ts:77-85). Distance is an
// int: TS truncates fractional hook values (PluginManager.ts:
// 1821), and fractional tiles have no Go form.
type CombatAttackDistanceEvent struct {
	Attacker   *game.Mobile
	Target     *game.Mobile
	CombatType CombatType
	ManualCast bool
	Distance   int
}

// CanAttackEvent is the payload of the onCanAttack hooks. It mirrors
// PluginCanAttackEvent (PluginTypes.ts:315-321): the attacker, the
// target, the attacking combat method (so handlers can allow/deny per
// style), and the allow answer. Method is a game.CombatMethod: nil
// when the caller has none (an area secondary target —
// canAttackByPolicy called without a method, CombatFactory.ts:913),
// which is TS undefined. Allow is a *bool: nil is TS null
// (undecided), non-nil is the hook's answer. A hook sets Allow
// through BoolPtr (gates.go); the emit resets it before each hook,
// so a hook only ever sees its own answer.
type CanAttackEvent struct {
	Attacker *game.Mobile
	Target   *game.Mobile
	Method   game.CombatMethod
	Allow    *bool
}

// PlayerOptionRouteEventName is the custom-event name for the
// player-option route intercept (PlayerOptionPacketListener.ts:
// 9-15, "player:option-route"): a plugin that owns the slot sets
// Handled and walks itself. Content subscribes through
// Api.OnCustomEvent; the player-option listener emits it (through
// the cmd/server adapter) for the attack option before attacking.
// It is a custom event, not a hook family: it does not appear in
// the 108-family table.
const PlayerOptionRouteEventName = "player:option-route"

// PlayerOptionRouteEvent is the payload of the "player:option-route"
// custom event: the player, the target, the option number, and the
// handled flag (PlayerOptionPacketListener.ts:11). Handlers receive
// a pointer (through the opaque `any` payload) so setting Handled
// is visible to the emitter, following the ObjectExamineEvent
// precedent (events.go). A handled route suppresses the attack and
// clears following (which degenerates: no following system exists
// in Go).
type PlayerOptionRouteEvent struct {
	Player  *game.Player
	Target  *game.Player
	Option  int
	Handled bool
}

// --- hookRegistry storage for the combat families (see hooks.go) ---

func (r *hookRegistry) addNpcDeath(e hookEntry[*NpcDeathEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcDeath = append(r.npcDeath, e)
}

func (r *hookRegistry) snapshotNpcDeath() []hookEntry[*NpcDeathEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*NpcDeathEvent](nil), r.npcDeath...)
}

func (r *hookRegistry) addNpcAttackOption(e hookEntry[*NpcAttackOptionEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcAttackOption = append(r.npcAttackOption, e)
}

func (r *hookRegistry) snapshotNpcAttackOption() []hookEntry[*NpcAttackOptionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*NpcAttackOptionEvent](nil), r.npcAttackOption...)
}

func (r *hookRegistry) addPlayerAttack(e hookEntry[*PlayerAttackEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerAttack = append(r.playerAttack, e)
}

func (r *hookRegistry) snapshotPlayerAttack() []hookEntry[*PlayerAttackEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerAttackEvent](nil), r.playerAttack...)
}

func (r *hookRegistry) addPlayerDeath(e hookEntry[*PlayerDeathEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerDeath = append(r.playerDeath, e)
}

func (r *hookRegistry) snapshotPlayerDeath() []hookEntry[*PlayerDeathEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerDeathEvent](nil), r.playerDeath...)
}

func (r *hookRegistry) addPlayerDeathItemDrop(e hookEntry[*PlayerDeathItemDropEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerDeathItemDrop = append(r.playerDeathItemDrop, e)
}

func (r *hookRegistry) snapshotPlayerDeathItemDrop() []hookEntry[*PlayerDeathItemDropEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerDeathItemDropEvent](nil), r.playerDeathItemDrop...)
}

func (r *hookRegistry) addPlayerDeathDrops(e hookEntry[*PlayerDeathDropsEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerDeathDrops = append(r.playerDeathDrops, e)
}

func (r *hookRegistry) snapshotPlayerDeathDrops() []hookEntry[*PlayerDeathDropsEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerDeathDropsEvent](nil), r.playerDeathDrops...)
}

func (r *hookRegistry) addShouldDropItemsOnDeath(e hookEntry[*ShouldDropItemsOnDeathEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldDropItemsOnDeath = append(r.shouldDropItemsOnDeath, e)
}

func (r *hookRegistry) snapshotShouldDropItemsOnDeath() []hookEntry[*ShouldDropItemsOnDeathEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ShouldDropItemsOnDeathEvent](nil), r.shouldDropItemsOnDeath...)
}

func (r *hookRegistry) addShouldKeepItemOnDeath(e hookEntry[*ShouldKeepItemOnDeathEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.shouldKeepItemOnDeath = append(r.shouldKeepItemOnDeath, e)
}

func (r *hookRegistry) snapshotShouldKeepItemOnDeath() []hookEntry[*ShouldKeepItemOnDeathEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ShouldKeepItemOnDeathEvent](nil), r.shouldKeepItemOnDeath...)
}

func (r *hookRegistry) addPlayerDefeated(e hookEntry[*PlayerDefeatedEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerDefeated = append(r.playerDefeated, e)
}

func (r *hookRegistry) snapshotPlayerDefeated() []hookEntry[*PlayerDefeatedEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerDefeatedEvent](nil), r.playerDefeated...)
}

func (r *hookRegistry) addCombatHitPrepare(e hookEntry[*CombatHitPrepareEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatHitPrepare = append(r.combatHitPrepare, e)
}

func (r *hookRegistry) snapshotCombatHitPrepare() []hookEntry[*CombatHitPrepareEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CombatHitPrepareEvent](nil), r.combatHitPrepare...)
}

func (r *hookRegistry) addCombatHitRoll(e hookEntry[*CombatHitRollEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatHitRoll = append(r.combatHitRoll, e)
}

func (r *hookRegistry) snapshotCombatHitRoll() []hookEntry[*CombatHitRollEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CombatHitRollEvent](nil), r.combatHitRoll...)
}

func (r *hookRegistry) addCombatHitDamage(e hookEntry[*CombatHitDamageEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatHitDamage = append(r.combatHitDamage, e)
}

func (r *hookRegistry) snapshotCombatHitDamage() []hookEntry[*CombatHitDamageEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CombatHitDamageEvent](nil), r.combatHitDamage...)
}

func (r *hookRegistry) addCombatHitResolved(e hookEntry[*CombatHitResolvedEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatHitResolved = append(r.combatHitResolved, e)
}

func (r *hookRegistry) snapshotCombatHitResolved() []hookEntry[*CombatHitResolvedEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CombatHitResolvedEvent](nil), r.combatHitResolved...)
}

func (r *hookRegistry) addCombatAttackDistance(e hookEntry[*CombatAttackDistanceEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatAttackDistance = append(r.combatAttackDistance, e)
}

func (r *hookRegistry) snapshotCombatAttackDistance() []hookEntry[*CombatAttackDistanceEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CombatAttackDistanceEvent](nil), r.combatAttackDistance...)
}

func (r *hookRegistry) addCanAttack(e hookEntry[*CanAttackEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canAttack = append(r.canAttack, e)
}

func (r *hookRegistry) snapshotCanAttack() []hookEntry[*CanAttackEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanAttackEvent](nil), r.canAttack...)
}

// --- registration methods ---

// OnNpcDeath registers an NPC-death hook, mirroring onNpcDeath
// (PluginTypes.ts:1105). Hooks run in registration order, all of
// them (EmitNpcDeath). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3528-3541). The
// stored handler wraps the plugin's with the TS guard: an event
// without a killer or NPC never reaches the plugin's handler.
func (a *Api) OnNpcDeath(handler func(*NpcDeathEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onNpcDeath") {
		return
	}
	a.hub.hooks.addNpcDeath(hookEntry[*NpcDeathEvent]{pluginName: a.name, handler: func(event *NpcDeathEvent) {
		if event == nil || event.Killer == nil || event.NPC == nil {
			return
		}
		handler(event)
	}})
}

// OnNpcAttackOption registers an NPC-attack-option hook, mirroring
// onNpcAttackOption (PluginTypes.ts:1064). Hooks run in
// registration order until one sets Handled (EmitNpcAttackOption).
// A nil handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:3472-3476). There is no registration
// wrapper: the emit's entry guards are the only checks.
func (a *Api) OnNpcAttackOption(handler func(*NpcAttackOptionEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onNpcAttackOption") {
		return
	}
	a.hub.hooks.addNpcAttackOption(hookEntry[*NpcAttackOptionEvent]{pluginName: a.name, handler: handler})
}

// OnPlayerAttack registers a player-attack hook, mirroring
// onPlayerAttack (PluginTypes.ts:1121). Hooks run in registration
// order, all of them (EmitPlayerAttack). A nil handler is ignored
// silently, mirroring the createApi guard (PluginManager.ts:
// 3729-3742). The stored handler wraps the plugin's with the TS
// guard: an event without a player or target never reaches the
// plugin's handler.
func (a *Api) OnPlayerAttack(handler func(*PlayerAttackEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerAttack") {
		return
	}
	a.hub.hooks.addPlayerAttack(hookEntry[*PlayerAttackEvent]{pluginName: a.name, handler: func(event *PlayerAttackEvent) {
		if event == nil || event.Player == nil || event.Target == nil {
			return
		}
		handler(event)
	}})
}

// OnPlayerDeath registers a player-death hook, mirroring
// onPlayerDeath (PluginTypes.ts:1144). Hooks run in registration
// order until one sets Handled (EmitPlayerDeath). A nil handler is
// ignored silently, mirroring the createApi guard
// (PluginManager.ts:3885-3898). The stored handler wraps the
// plugin's with the TS guard: a nil event, an event without a
// player, or an already-handled event never reaches the plugin's
// handler.
func (a *Api) OnPlayerDeath(handler func(*PlayerDeathEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerDeath") {
		return
	}
	a.hub.hooks.addPlayerDeath(hookEntry[*PlayerDeathEvent]{pluginName: a.name, handler: func(event *PlayerDeathEvent) {
		if event == nil || event.Player == nil || event.Handled {
			return
		}
		handler(event)
	}})
}

// OnPlayerDeathItemDrop registers a per-item death-drop hook,
// mirroring onPlayerDeathItemDrop (PluginTypes.ts:1131-1133).
// Hooks run in registration order until one sets Handled
// (EmitPlayerDeathItemDrop). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3815-3828). The
// stored handler wraps the plugin's with the TS guard: an event
// without a player or item never reaches the plugin's handler.
// (The guard's !item check has no Go form beyond the zero Item:
// an Item with id 0 is the empty stack, and the emit site never
// builds one for a real drop — the wrapper checks the event and
// player only, and the emit's entry guard likewise treats the
// Item value as always present. See EmitPlayerDeathItemDrop.)
func (a *Api) OnPlayerDeathItemDrop(handler func(*PlayerDeathItemDropEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerDeathItemDrop") {
		return
	}
	a.hub.hooks.addPlayerDeathItemDrop(hookEntry[*PlayerDeathItemDropEvent]{pluginName: a.name, handler: func(event *PlayerDeathItemDropEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnPlayerDeathDrops registers a death-drops hook, mirroring
// onPlayerDeathDrops (PluginTypes.ts:1134). Hooks run in
// registration order, all of them (EmitPlayerDeathDrops). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3809-3813). There is no registration wrapper:
// the emit's entry guards are the only checks.
func (a *Api) OnPlayerDeathDrops(handler func(*PlayerDeathDropsEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerDeathDrops") {
		return
	}
	a.hub.hooks.addPlayerDeathDrops(hookEntry[*PlayerDeathDropsEvent]{pluginName: a.name, handler: handler})
}

// OnShouldDropItemsOnDeath registers a should-drop-items-on-death
// hook, mirroring onShouldDropItemsOnDeath
// (PluginTypes.ts:1125-1127). Hooks run in registration order
// through the first-answer gate (EmitShouldDropItemsOnDeath):
// the first hook to set ShouldDrop wins and later hooks never
// run. A nil handler is ignored silently, mirroring the
// createApi guard (PluginManager.ts:3781-3794). The stored
// handler wraps the plugin's with the TS guard: an event
// without a player never reaches the plugin's handler. A nil
// killer is NOT guarded: TS passes killer null through for an
// environmental death.
func (a *Api) OnShouldDropItemsOnDeath(handler func(*ShouldDropItemsOnDeathEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onShouldDropItemsOnDeath") {
		return
	}
	a.hub.hooks.addShouldDropItemsOnDeath(hookEntry[*ShouldDropItemsOnDeathEvent]{pluginName: a.name, handler: func(event *ShouldDropItemsOnDeathEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnShouldKeepItemOnDeath registers a should-keep-item-on-death
// hook, mirroring onShouldKeepItemOnDeath
// (PluginTypes.ts:1128-1130). Hooks run in registration order
// through the first-answer gate (EmitShouldKeepItemOnDeath):
// the first hook to set Keep wins and later hooks never run.
// A nil handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:3795-3808). The stored handler wraps
// the plugin's with the TS guard: an event without a player or
// item never reaches the plugin's handler. (The guard's !item
// check has no Go form beyond the zero Item, as in
// OnPlayerDeathItemDrop: the emit site never builds a query
// for an empty stack.)
func (a *Api) OnShouldKeepItemOnDeath(handler func(*ShouldKeepItemOnDeathEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onShouldKeepItemOnDeath") {
		return
	}
	a.hub.hooks.addShouldKeepItemOnDeath(hookEntry[*ShouldKeepItemOnDeathEvent]{pluginName: a.name, handler: func(event *ShouldKeepItemOnDeathEvent) {
		if event == nil || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnPlayerDefeated registers a player-defeated hook, mirroring
// onPlayerDefeated (PluginTypes.ts:1166). Hooks run in
// registration order, all of them (EmitPlayerDefeated), for
// EVERY death. A nil handler is ignored silently, mirroring
// the createApi guard (PluginManager.ts:4028-4041). The stored
// handler wraps the plugin's with the TS guard: an event
// without a victim never reaches the plugin's handler. A nil
// killer is NOT guarded: TS passes killer null through for an
// environmental death.
func (a *Api) OnPlayerDefeated(handler func(*PlayerDefeatedEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onPlayerDefeated") {
		return
	}
	a.hub.hooks.addPlayerDefeated(hookEntry[*PlayerDefeatedEvent]{pluginName: a.name, handler: func(event *PlayerDefeatedEvent) {
		if event == nil || event.Victim == nil {
			return
		}
		handler(event)
	}})
}

// OnCombatHitPrepare registers a hit-prepare hook, mirroring
// onCombatHitPrepare (PluginTypes.ts:1150). Hooks run in
// registration order, all of them (EmitCombatHitPrepare). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3946-3948). There is no registration wrapper.
func (a *Api) OnCombatHitPrepare(handler func(*CombatHitPrepareEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCombatHitPrepare") {
		return
	}
	a.hub.hooks.addCombatHitPrepare(hookEntry[*CombatHitPrepareEvent]{pluginName: a.name, handler: handler})
}

// OnCombatHitRoll registers a hit-roll hook, mirroring
// onCombatHitRoll (PluginTypes.ts:1151). Hooks run in registration
// order, all of them (EmitCombatHitRoll). A nil handler is ignored
// silently, mirroring the createApi guard (PluginManager.ts:
// 3950-3952). There is no registration wrapper.
func (a *Api) OnCombatHitRoll(handler func(*CombatHitRollEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCombatHitRoll") {
		return
	}
	a.hub.hooks.addCombatHitRoll(hookEntry[*CombatHitRollEvent]{pluginName: a.name, handler: handler})
}

// OnCombatHitDamage registers a hit-damage hook, mirroring
// onCombatHitDamage (PluginTypes.ts:1152). Hooks run in
// registration order, all of them (EmitCombatHitDamage). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3954-3956). There is no registration wrapper.
func (a *Api) OnCombatHitDamage(handler func(*CombatHitDamageEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCombatHitDamage") {
		return
	}
	a.hub.hooks.addCombatHitDamage(hookEntry[*CombatHitDamageEvent]{pluginName: a.name, handler: handler})
}

// OnCombatHitResolved registers a hit-resolved hook, mirroring
// onCombatHitResolved (PluginTypes.ts:1159). Hooks run in
// registration order, all of them (EmitCombatHitResolved). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3970-3972). There is no registration wrapper.
func (a *Api) OnCombatHitResolved(handler func(*CombatHitResolvedEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCombatHitResolved") {
		return
	}
	a.hub.hooks.addCombatHitResolved(hookEntry[*CombatHitResolvedEvent]{pluginName: a.name, handler: handler})
}

// OnCombatAttackDistance registers an attack-distance hook,
// mirroring onCombatAttackDistance (PluginTypes.ts:1160). Hooks
// run in registration order, all of them
// (EmitCombatAttackDistance). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3974-3977).
// There is no registration wrapper.
func (a *Api) OnCombatAttackDistance(handler func(*CombatAttackDistanceEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCombatAttackDistance") {
		return
	}
	a.hub.hooks.addCombatAttackDistance(hookEntry[*CombatAttackDistanceEvent]{pluginName: a.name, handler: handler})
}

// OnCanAttack registers a can-attack hook, mirroring onCanAttack
// (PluginTypes.ts:1109). Hooks run in registration order through
// the allow-veto gate (EmitCanAttack): allow is reset before each
// hook, a deny short-circuits, an allow sticks but a later deny
// still vetoes. A nil handler is ignored silently, mirroring the
// createApi guard (PluginManager.ts:3581-3593). The stored handler
// wraps the plugin's with the TS guard: an event without an
// attacker or target never reaches the plugin's handler. A nil
// method is NOT guarded: TS passes method undefined through, and
// the wrapper checks the fighters only.
func (a *Api) OnCanAttack(handler func(*CanAttackEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanAttack") {
		return
	}
	a.hub.hooks.addCanAttack(hookEntry[*CanAttackEvent]{pluginName: a.name, handler: func(event *CanAttackEvent) {
		if event == nil || event.Attacker == nil || event.Target == nil {
			return
		}
		handler(event)
	}})
}

// --- Hub emits ---

// EmitNpcDeath runs the NPC-death hooks in registration order,
// mirroring PluginManager.emitNpcDeath (PluginManager.ts:1322-1329)
// with label "npc_death": every hook runs. A nil Hub, a Hub
// without a registry, or a nil event runs no hooks (the nil-event
// guard is Go-only; TS has no nil-event equivalent, and the
// registration wrappers would skip a nil event anyway, so the
// outcome is identical). Events without a killer or NPC likewise
// run no plugin handler, through the wrappers.
//
// The production call site is the NPC death task, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitNpcDeath(event *NpcDeathEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "npc_death", h.hooks.snapshotNpcDeath(), event)
}

// EmitNpcAttackOption runs an NPC's combat option through the
// registered hooks and reports whether one took it over,
// mirroring PluginManager.emitNpcAttackOption
// (PluginManager.ts:1277-1287) with label "npc_attack_option":
// the entry guards (nil event/player/NPC return false), then the
// short-circuit handled loop. A pre-handled event returns true
// with zero hooks run, matching the TS loop shape (unlike
// EmitPlayerDeath, whose entry guard returns false for a
// pre-handled event). A nil Hub or a Hub without a registry
// returns false.
//
// The production call site is the NPC-option listener's combat
// branch; Api.EmitNpcAttackOption exposes the same emit to
// content (PluginTypes.ts:1405).
func (h *Hub) EmitNpcAttackOption(event *NpcAttackOptionEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.NPC == nil {
		return false
	}
	return runHandledGate(h.Logger(), "npc_attack_option", h.hooks.snapshotNpcAttackOption(), event,
		func(e *NpcAttackOptionEvent) bool { return e.Handled })
}

// EmitPlayerAttack runs the player-attack hooks in registration
// order, mirroring PluginManager.emitPlayerAttack
// (PluginManager.ts:1501-1508) with label "player_attack": every
// hook runs. A nil Hub, a Hub without a registry, or a nil event
// runs no hooks (the nil-event guard is Go-only, as in
// EmitNpcDeath). Events without a player or target run no plugin
// handler, through the wrappers.
//
// The production call site is the player-option listener, after
// the attack starts.
func (h *Hub) EmitPlayerAttack(event *PlayerAttackEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "player_attack", h.hooks.snapshotPlayerAttack(), event)
}

// EmitPlayerDeath runs a player death through the registered hooks
// and reports whether one took it over, mirroring
// PluginManager.emitPlayerDeath (PluginManager.ts:1724-1735) with
// label "player_death": the entry guards (nil event/player, or a
// pre-handled event, all return false), then the short-circuit
// handled loop. A nil Hub or a Hub without a registry returns
// false.
//
// The production call site is the player death task, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitPlayerDeath(event *PlayerDeathEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Handled {
		return false
	}
	return runHandledGate(h.Logger(), "player_death", h.hooks.snapshotPlayerDeath(), event,
		func(e *PlayerDeathEvent) bool { return e.Handled })
}

// EmitPlayerDeathItemDrop runs one death-drop item through the
// registered hooks and reports whether one took it over,
// mirroring PluginManager.emitPlayerDeathItemDrop
// (PluginManager.ts:1591-1616) with label "player_death_item_drop":
// the entry guards (nil event/player, or a pre-handled event,
// return false), then the short-circuit handled loop. The TS
// !event.item guard degenerates: Item is a game.Item value, always
// present, and the emit site never builds a drop for an empty
// stack (see OnPlayerDeathItemDrop). A nil Hub or a Hub without a
// registry returns false. SuppressDefaultDrop and ExtraDrops are
// read back from the event by the caller after the emit, whether
// or not a hook handled it.
//
// The production call site is the player death task's drop loop,
// through the game-side CombatHooks seam (see the file header).
func (h *Hub) EmitPlayerDeathItemDrop(event *PlayerDeathItemDropEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Handled {
		return false
	}
	return runHandledGate(h.Logger(), "player_death_item_drop", h.hooks.snapshotPlayerDeathItemDrop(), event,
		func(e *PlayerDeathItemDropEvent) bool { return e.Handled })
}

// EmitPlayerDeathDrops runs a death's lost items through the
// registered hooks, mirroring PluginManager.emitPlayerDeathDrops
// (PluginManager.ts:1618-1625) with label "player_death_drops":
// every hook runs, each taking what it holds from Items and
// Untradeables. A nil Hub, a Hub without a registry, a nil event,
// or an event without a player runs no hooks.
//
// The production call site is the player death task, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitPlayerDeathDrops(event *PlayerDeathDropsEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil {
		return
	}
	emitHooks(h.Logger(), "player_death_drops", h.hooks.snapshotPlayerDeathDrops(), event)
}

// EmitShouldDropItemsOnDeath runs the should-drop-items-on-death
// hooks through the first-answer gate and reports the outcome,
// mirroring PluginManager.emitShouldDropItemsOnDeath
// (PluginManager.ts:1548-1569) with label
// "should_drop_items_on_death": with no hooks registered it
// returns nil (TS returns null without building an event);
// otherwise the first hook to set ShouldDrop wins and later
// hooks never run, and silence from every hook yields nil. A
// nil Hub or a Hub without a registry yields nil. There is no
// emit-level player guard: the registration wrappers skip
// player-less events, exactly as TS's do, so the outcome is
// identical (nil) — following the EmitCanAttack precedent.
//
// The production call site is the player death task's respawn
// tick, through the game-side CombatHooks.ShouldDropItemsOnDeath
// seam (see the file header): nil means the death drops items.
func (h *Hub) EmitShouldDropItemsOnDeath(player, killer *game.Player) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &ShouldDropItemsOnDeathEvent{Player: player, Killer: killer}
	return runFirstAnswerGate(h.Logger(), "should_drop_items_on_death", h.hooks.snapshotShouldDropItemsOnDeath(), event,
		func(e *ShouldDropItemsOnDeathEvent) *bool { return e.ShouldDrop })
}

// EmitShouldKeepItemOnDeath runs the should-keep-item-on-death
// hooks through the first-answer gate and reports the outcome,
// mirroring PluginManager.emitShouldKeepItemOnDeath
// (PluginManager.ts:1571-1589) with label
// "should_keep_item_on_death": the first hook to set Keep wins
// and later hooks never run, and silence from every hook yields
// nil. Unlike emitShouldDropItemsOnDeath, TS builds the event
// even with zero hooks registered (there is no length early
// return); the outcome is the same nil, so Go skips the gate
// the same way. A nil Hub or a Hub without a registry yields
// nil. There is no emit-level player/item guard: the
// registration wrappers skip player-less events, exactly as
// TS's do, so the outcome is identical (nil) — following the
// EmitCanAttack precedent.
//
// The production call site is the death task's kept-items
// ranking, through the game-side CombatHooks.ShouldKeepItemOnDeath
// seam (see the file header): true keeps the item beyond the
// count, false never keeps it, nil means no opinion.
func (h *Hub) EmitShouldKeepItemOnDeath(player *game.Player, item game.Item) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &ShouldKeepItemOnDeathEvent{Player: player, Item: item}
	return runFirstAnswerGate(h.Logger(), "should_keep_item_on_death", h.hooks.snapshotShouldKeepItemOnDeath(), event,
		func(e *ShouldKeepItemOnDeathEvent) *bool { return e.Keep })
}

// EmitPlayerDefeated runs the player-defeated hooks in
// registration order, mirroring PluginManager.emitPlayerDefeated
// (PluginManager.ts:1901-1905) with label "player_defeated":
// every hook runs, for every death, killer nil for an
// environmental death. A nil Hub, a Hub without a registry, or
// a nil event runs no hooks; an event without a victim runs no
// plugin handler, through the wrappers.
//
// The production call site is the player death task's respawn
// tick, through the game-side CombatHooks.OnPlayerDefeated seam
// (see the file header).
func (h *Hub) EmitPlayerDefeated(event *PlayerDefeatedEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "player_defeated", h.hooks.snapshotPlayerDefeated(), event)
}

// EmitCombatHitPrepare runs the hit-prepare hooks in registration
// order, mirroring PluginManager.emitCombatHitPrepare
// (PluginManager.ts:1789-1794) with label "combat_hit_prepare":
// every hook runs. A nil Hub, a Hub without a registry, a nil
// event, or an event without an attacker or target runs no hooks.
//
// The production call site is the hit pipeline, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitCombatHitPrepare(event *CombatHitPrepareEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Attacker == nil || event.Target == nil {
		return
	}
	emitHooks(h.Logger(), "combat_hit_prepare", h.hooks.snapshotCombatHitPrepare(), event)
}

// EmitCombatHitRoll runs the hit-roll hooks in registration order,
// mirroring PluginManager.emitCombatHitRoll
// (PluginManager.ts:1796-1801) with label "combat_hit_roll": every
// hook runs. The entry guards match EmitCombatHitPrepare. The
// pipeline reads ForceAccurate, ForceMaxHit, and
// BypassProtectionPrayer back from the event after the emit.
//
// The production call site is the hit pipeline, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitCombatHitRoll(event *CombatHitRollEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Attacker == nil || event.Target == nil {
		return
	}
	emitHooks(h.Logger(), "combat_hit_roll", h.hooks.snapshotCombatHitRoll(), event)
}

// EmitCombatHitDamage runs the hit-damage hooks in registration
// order, mirroring PluginManager.emitCombatHitDamage
// (PluginManager.ts:1803-1808) with label "combat_hit_damage":
// every hook runs. A nil Hub, a Hub without a registry, a nil
// event, an event without an attacker or target, or an event
// with a nil Damage runs no hooks; Hit is deliberately NOT
// guarded, matching TS (which checks damage only). The pipeline
// reads Accurate (and the mutated Damage) back after the emit.
//
// The production call site is the hit pipeline, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitCombatHitDamage(event *CombatHitDamageEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Attacker == nil || event.Target == nil || event.Damage == nil {
		return
	}
	emitHooks(h.Logger(), "combat_hit_damage", h.hooks.snapshotCombatHitDamage(), event)
}

// EmitCombatHitResolved runs the hit-resolved hooks in
// registration order, mirroring PluginManager.emitCombatHitResolved
// (PluginManager.ts:1810-1815) with label "combat_hit_resolved":
// every hook runs. A nil Hub, a Hub without a registry, a nil
// event, or an event without an attacker, target, or hit runs no
// hooks.
//
// The production call site is the hit-resolution path, through
// the game-side CombatHooks seam (see the file header).
func (h *Hub) EmitCombatHitResolved(event *CombatHitResolvedEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Attacker == nil || event.Target == nil || event.Hit == nil {
		return
	}
	emitHooks(h.Logger(), "combat_hit_resolved", h.hooks.snapshotCombatHitResolved(), event)
}

// EmitCombatAttackDistance runs the attack-distance hooks in
// registration order and returns the adjusted reach, mirroring
// PluginManager.emitCombatAttackDistance (PluginManager.ts:
// 1817-1822) with label "combat_attack_distance": every hook runs,
// then the distance is floored at 1. TS has no entry guards — a
// nil event would throw reading event.distance. Go guards a nil
// Hub, a Hub without a registry, or a nil event by returning the
// floor (1): the distance never goes below 1, so the floor is the
// only value the guard can honestly return, and the production
// call site always passes a real event. The TS trunc() is vacuous
// in Go: Distance is an int, so no fractional value can arrive.
//
// The production call site is the combat-range query, through the
// game-side CombatHooks seam (see the file header).
func (h *Hub) EmitCombatAttackDistance(event *CombatAttackDistanceEvent) int {
	if h == nil || h.hooks == nil || event == nil {
		return 1
	}
	emitHooks(h.Logger(), "combat_attack_distance", h.hooks.snapshotCombatAttackDistance(), event)
	if event.Distance < 1 {
		return 1
	}
	return event.Distance
}

// EmitCanAttack runs the can-attack hooks through the allow-veto
// gate and reports the outcome, mirroring
// PluginManager.emitCanAttack (PluginManager.ts:1360-1377) with
// label "can_attack": with no hooks registered it returns nil
// (TS returns null without building an event); otherwise every
// hook sees Allow reset to undecided, a deny returns false
// immediately (later hooks never run), an allow sticks but a
// later deny still vetoes, and silence from every hook yields
// nil. Method is the attacking combat method, or nil when the
// caller has none (TS undefined). A nil Hub or a Hub without a
// registry yields nil. There is no emit-level attacker/target
// guard: the registration wrappers skip fighter-less events,
// exactly as TS's do, so the outcome is identical (nil) —
// following the EmitCanShop precedent.
//
// The production call sites are CanAttackPermission's duel branch
// and canAttackByPolicy's global hook, through the game-side
// CombatHooks.OnCanAttack seam (see the file header): the duel
// branch denies only on false, and the policy treats false as a
// denial, true as an allow, and nil as no opinion.
func (h *Hub) EmitCanAttack(attacker, target *game.Mobile, method game.CombatMethod) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &CanAttackEvent{Attacker: attacker, Target: target, Method: method}
	return runAllowVetoGate(h.Logger(), "can_attack", h.hooks.snapshotCanAttack(), event,
		func(e *CanAttackEvent) { e.Allow = nil },
		func(e *CanAttackEvent) *bool { return e.Allow })
}

// EmitNpcAttackOption runs an NPC's combat option through the
// registered hooks and reports whether one took it over,
// mirroring the PluginApi emitNpcAttackOption member
// (PluginTypes.ts:1405) and its createApi passthrough
// (PluginManager.ts:4697). It delegates to
// Hub.EmitNpcAttackOption. A nil Api, a nil hub, or a nil event
// yields false. This is an emit, not a registration: it has no
// load-phase gate.
func (a *Api) EmitNpcAttackOption(event *NpcAttackOptionEvent) bool {
	if a == nil || a.hub == nil {
		return false
	}
	return a.hub.EmitNpcAttackOption(event)
}

// --- NPC combat method providers ---

// NpcCombatMethodConstructor is the Go form of the methodCtor
// parameter of registerNpcCombatMethodProvider
// (PluginTypes.ts:1550: `new (npc?: any) => any`): a constructor
// for the combat method, returning the method value (opaque `any`,
// as TS returns `any`) or nil for "no method". The singleton form
// calls it ONCE with a nil NPC (TS: `new methodCtor()`, with the
// optional npc argument omitted) and reuses the result; the
// per-attack form calls it for each attack with the attacking NPC
// (TS: `new methodCtor(npc)`), so a per-attack method can read
// that NPC's own fight state. A singleton constructor must
// therefore not dereference its argument. The returned value must
// be the combat method type the game registry accepts; the hub
// passes it through opaquely, and the cmd/server adapter asserts
// it (see combatwiring.go).
type NpcCombatMethodConstructor func(npc *game.NPC) any

// npcCombatMethodProvider is one registered provider: the owning
// plugin's name, the NPC ids it covers, and the provide closure
// built at registration time (the Go form of the provider object
// in the createApi closure, PluginManager.ts:4944-4961, plus the
// entry PluginManager.registerNpcCombatMethodProviderInternal
// pushes, PluginManager.ts:5241-5250).
type npcCombatMethodProvider struct {
	pluginName string
	npcIDs     []int
	provide    func(npc *game.NPC) any
}

// NpcCombatMethodProviderRegistry is the narrow provider-registry
// surface Api.RegisterNpcCombatMethodProvider pushes onto: the Go
// form of the npcCombatMethodProviders push in
// registerNpcCombatMethodProviderInternal (PluginManager.ts:
// 5241-5250). It follows the AreaRegistry precedent (areas.go):
// the interface is declared here, in the plugins package, so game
// never imports plugins; the coordinator's cmd/server adapter
// implements it over the concrete game registry (Worker 2's
// RegisterNPCCombatMethodProvider). The provide closure already
// applies the id-set check and the singleton/per-attack
// construction, so the adapter only forwards it; npcIDs is
// carried for the game registry's own id-set check (CombatFactory
// consults entry.npcIds before calling provide,
// CombatFactory.ts:174-184), exactly as TS stores both.
type NpcCombatMethodProviderRegistry interface {
	// RegisterProvider appends one provider to the registry, in
	// registration order (the factory consults providers in
	// order and takes the first non-nil method).
	RegisterProvider(pluginName string, npcIDs []int, provide func(npc *game.NPC) any)
}

// SetNpcCombatMethodProviderRegistry installs the registry
// Api.RegisterNpcCombatMethodProvider pushes onto, flushing
// every provider queued before installation, in registration
// order. The coordinator calls it once at boot, after the game
// registry exists; calling it with nil clears the installed
// registry but keeps the queue (a later non-nil install flushes
// it), mirroring SetAreaRegistry. It is a no-op on a nil Hub or a
// Hub without a registry. The flush runs outside the registry
// lock, as SetAreaRegistry's does.
func (h *Hub) SetNpcCombatMethodProviderRegistry(reg NpcCombatMethodProviderRegistry) {
	if h == nil || h.hooks == nil {
		return
	}
	queued := h.hooks.installCombatMethodProviderRegistry(reg)
	for _, entry := range queued {
		reg.RegisterProvider(entry.pluginName, entry.npcIDs, entry.provide)
	}
}

// NpcCombatMethodProviderRegistry returns the installed provider
// registry, or nil when none is installed (or the Hub is nil). It
// mirrors Hub.AreaRegistry: the read half of the install seam,
// for the coordinator and tests.
func (h *Hub) NpcCombatMethodProviderRegistry() NpcCombatMethodProviderRegistry {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedCombatMethodProviderRegistry()
}

// combatMethodProviderRegistrationTarget returns the installed
// provider registry for entry to be pushed onto, or queues entry
// (in registration order) and returns nil when no registry is
// installed yet. The caller pushes onto a returned registry
// outside the lock. It mirrors areaRegistrationTarget (hooks.go).
func (r *hookRegistry) combatMethodProviderRegistrationTarget(entry npcCombatMethodProvider) NpcCombatMethodProviderRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.combatMethodProviderRegistry != nil {
		return r.combatMethodProviderRegistry
	}
	r.queuedCombatMethodProviders = append(r.queuedCombatMethodProviders, entry)
	return nil
}

// installCombatMethodProviderRegistry installs reg and returns the
// providers queued before installation, in registration order,
// for the caller to push outside the lock. Installing nil clears
// the registry but keeps the queue. It mirrors
// installAreaRegistry (hooks.go).
func (r *hookRegistry) installCombatMethodProviderRegistry(reg NpcCombatMethodProviderRegistry) []npcCombatMethodProvider {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.combatMethodProviderRegistry = reg
	if reg == nil {
		return nil
	}
	queued := r.queuedCombatMethodProviders
	r.queuedCombatMethodProviders = nil
	return queued
}

func (r *hookRegistry) installedCombatMethodProviderRegistry() NpcCombatMethodProviderRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.combatMethodProviderRegistry
}

// RegisterNpcCombatMethodProvider registers a combat method
// provider for one or more NPC ids, mirroring the PluginApi
// registerNpcCombatMethodProvider member (PluginTypes.ts:1548-1552)
// and its createApi implementation (PluginManager.ts:4926-4965):
// the ids are normalized to a set, the constructor is validated,
// and a provider is registered that answers only NPCs whose id is
// in the set, constructing a singleton method once (the default)
// or a fresh method per attack (singleton false, given the
// attacking NPC).
//
// singleton is variadic to mirror the optional TS options object:
// zero arguments means singleton (the TS default,
// `options?.singleton ?? true`), one argument sets it, and more
// than one panics (a programmer error with no TS equivalent,
// mirroring PersistAttribute's arity rule in api.go).
//
// Validation mirrors the TS warnings exactly (PluginManager.ts:
// 4928-4943): an empty id list, or a nil constructor, is warned
// about through the hub logger and dropped — never an error, never
// a panic. The TS non-integer-id check has no Go form (Go ints
// are always integers); negative ids pass, as they pass the TS
// Number.isInteger check. The TS id check also covers the
// npcIdSet used by provide: an NPC matches when its id is in the
// set. TS additionally matches the NPC's real id
// (getRealId, behind transforms); Go has no transforms, so the
// check reads the NPC id only — a documented degeneration.
//
// RegisterNpcCombatMethodProvider has NO load-phase gate, like
// RegisterArea: the TS closure has no phase check, and both
// load-time registration (queued until the registry is installed)
// and later registration (direct) are legitimate. A nil Api, or
// an Api with a nil hub, is a no-op. A nil NPC passed to provide
// yields nil (TS: npc?.getId?.() ?? npc?.id reads undefined, which
// matches no id in the set).
func (a *Api) RegisterNpcCombatMethodProvider(npcIDs []int, ctor NpcCombatMethodConstructor, singleton ...bool) {
	if a == nil || a.hub == nil || a.hub.hooks == nil {
		return
	}
	if len(singleton) > 1 {
		panic("plugins: RegisterNpcCombatMethodProvider accepts at most one singleton value")
	}
	isSingleton := true
	if len(singleton) == 1 {
		isSingleton = singleton[0]
	}
	logger := a.hub.Logger()
	warn := func(format string, args ...any) {
		if logger == nil {
			logger = log.Default()
		}
		logger.Printf(format, args...)
	}
	if len(npcIDs) == 0 {
		warn("[plugins] %s provided invalid npc ids for combat method registration", a.name)
		return
	}
	if ctor == nil {
		warn("[plugins] %s provided invalid combat method constructor", a.name)
		return
	}
	ids := append([]int(nil), npcIDs...)
	idSet := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	var once sync.Once
	var instance any
	provide := func(npc *game.NPC) any {
		if npc == nil {
			return nil
		}
		if _, ok := idSet[npc.ID]; !ok {
			return nil
		}
		if isSingleton {
			once.Do(func() { instance = ctor(nil) })
			return instance
		}
		return ctor(npc)
	}
	entry := npcCombatMethodProvider{pluginName: a.name, npcIDs: ids, provide: provide}
	if reg := a.hub.hooks.combatMethodProviderRegistrationTarget(entry); reg != nil {
		reg.RegisterProvider(entry.pluginName, entry.npcIDs, entry.provide)
	}
}
