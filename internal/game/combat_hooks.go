package game

// CombatHooks is the game-side combat event seam: the func fields
// combat code calls for the events TS emits through PluginManager,
// and the policy hooks TS consults before acting. Worker 3's
// cmd/server wiring populates the package-level Hooks var from the
// plugin hub; every call site checks for nil, so an unpopulated
// seam is inert and core combat runs standalone. The game package
// must never import plugins — the seam is func fields only.
//
// Event shapes are grounded in PluginTypes.ts (the event payload
// types) and the emit sites in Combat.ts / CombatFactory.ts /
// HitQueue.ts / PendingHit.ts. Where TS payloads carry plugin-only
// values (spell objects, item definitions), the Go shape carries
// the core-readable subset and documents the omission.

// CombatDeathEvent is the core subset of PluginNpcDeathEvent
// (PluginTypes.ts:489-496) and PluginPlayerDeathEvent
// (PluginTypes.ts:614-621): who died, who killed them, where.
// NPC death: Damagers lists the players who dealt damage
// (NPCDeathTask.ts:46-47, captured before the damage map is
// cleared); Remains, when set by a hook, replaces the default
// respawn with a remains object (NPCDeathTask.ts:71-90) — core
// honors it only through the respawn seam (see combat_death.go).
// Player death: ItemsLost is NOT populated by core — the death
// item-loss pipeline (PlayerDeathTask.ts) is not ported; see
// combat_death.go.
type CombatDeathEvent struct {
	// Victim is the Mobile that died.
	Victim *Mobile
	// Killer is the player who dealt the most damage, or nil.
	Killer *Player
	// Damagers lists the players who damaged the victim (NPC
	// death only; empty for players).
	Damagers []*Player
	// Location is the victim's tile at death.
	Location Location
	// Remains, when set by a hook, names a remains object:
	// RemainsTicks how long it stands, RespawnTicks the delay
	// before the NPC respawns. Zero values mean "no remains".
	RemainsTicks int
	RespawnTicks int
	// ItemsLost is set by the player death task
	// (PlayerDeath.ts:113): whether the death dropped items.
	// HasItemsLost distinguishes "set" from a hand-built
	// event that left it unset (the plugins.PlayerDeathEvent
	// shape).
	ItemsLost    bool
	HasItemsLost bool
	// Handled is set by the adapter when a hook takes the
	// death over (PlayerDeath.ts:110-130): the default
	// respawn policy (player:respawn event + teleport home)
	// is skipped. The game event carries it so the death
	// task can read the handled chain back through the seam.
	Handled bool
}

// CombatBeforeDeathEvent is PluginNpcBeforeDeathEvent /
// PluginPlayerBeforeDeathEvent (PluginTypes.ts:248,
// PluginNpcBeforeDeathEvent; PlayerDeathTask.ts emitPlayerBeforeDeath):
// fired before the death sequence starts. Setting PreventDeath
// cancels the death entirely (the victim survives at its current
// hitpoints — TS returns from appendDeath before isDying is set).
type CombatBeforeDeathEvent struct {
	Victim       *Mobile
	PreventDeath bool
	// Announce and Message are the player-death task's announce
	// flag and respawn message (PlayerDeathTask.ts); NPC hooks
	// ignore them.
	Announce bool
	Message  string
}

// CombatHitPrepareEvent is PluginCombatHitPrepareEvent
// (PluginTypes.ts:489-496 area, combat:hit-prepare): fired when
// a PendingHit is constructed with hits to roll (PendingHit.ts
// emitCombatHitPrepare). Hooks observe; they do not change the
// roll here.
type CombatHitPrepareEvent struct {
	Attacker   *Mobile
	Target     *Mobile
	CombatType CombatType
}

// CombatHitRollEvent is PluginCombatHitRollEvent
// (PluginTypes.ts:489-536, combat:hit-rolled): fired per hit
// roll (PendingHit.ts rollHit, CombatFactory.rollHitDamage).
// Hooks may force accuracy or a max hit, or bypass protection
// prayer for this roll.
type CombatHitRollEvent struct {
	Attacker               *Mobile
	Target                 *Mobile
	CombatType             CombatType
	ForceAccurate          bool
	ForceMaxHit            bool
	BypassProtectionPrayer bool
}

// CombatHitDamageEvent is PluginCombatHitDamageEvent
// (PluginTypes.ts:489-536, combat:hit-damage): fired after a
// hit's damage is rolled (PendingHit.emitHitDamage). Hit is
// the PendingHit being built; Damage is the rolled splat
// (shared: hooks mutate it through SetDamage); Accurate is
// written back into the PendingHit, so hooks may flip the
// outcome.
type CombatHitDamageEvent struct {
	Attacker   *Mobile
	Target     *Mobile
	CombatType CombatType
	Hit        *PendingHit
	Damage     *HitDamage
	Accurate   bool
}

// CombatHitRolledEvent is the combat:hit-rolled custom event
// (CombatFactory.rollHitDamage): fired for the damage roll
// itself, before defences. Damage is mutable (floored at 0);
// Minimum/Maximum are the roll's range, for hooks to judge
// against.
type CombatHitRolledEvent struct {
	Attacker   *Mobile
	Target     *Mobile
	CombatType CombatType
	Damage     int
	Minimum    int
	Maximum    int
}

// CombatHitResolvedEvent is PluginCombatHitResolvedEvent
// (PluginTypes.ts:489-536, combat:hit-resolved): fired after a
// hit's damage is applied (CombatFactory.executeHit). Hit is
// the resolved PendingHit.
type CombatHitResolvedEvent struct {
	Attacker *Mobile
	Target   *Mobile
	Hit      *PendingHit
}

// CombatAttackDistanceEvent is PluginCombatAttackDistanceEvent
// (PluginTypes.ts:614-657, combat:attack-distance): fired when
// CombatRange resolves an attack distance. Distance is mutable
// and floors at 1 (CombatRange.ts attackDistance).
type CombatAttackDistanceEvent struct {
	Attacker   *Mobile
	Target     *Mobile
	CombatType CombatType
	ManualCast bool
	Distance   int
}

// AttackTimingResult is the PluginAttackTimingEvent outcome
// (Combat.ts executeAttack reads it): whether the attack
// ignores, keeps, or extends the cooldown. The event's
// newTarget input (target !== lastAttackTarget) is passed
// to the hook as an argument, not carried here.
type AttackTimingResult struct {
	IgnoreDelay  bool
	KeepDelay    bool
	MinimumDelay int
}

// AttackSpeedEvent is PluginAttackSpeedEvent (Combat.ts
// emitAttackSpeed): the attack speed a swing will use. Speed
// is mutable and floors at 1.
type AttackSpeedEvent struct {
	Attacker   *Mobile
	Target     *Mobile
	CombatType CombatType
	Speed      int
}

// CombatHooks holds every combat hook field. All fields are
// optional; combat code calls them nil-safely. The wiring
// populates Hooks once at boot before ticking starts.
type CombatHooks struct {
	// OnNpcDeath fires the npc-death event (NPCDeathTask.ts:63).
	// Drops are rolled by content listening to this event: core
	// rolls no drops (see combat_death.go).
	OnNpcDeath func(e *CombatDeathEvent)
	// OnNpcBeforeDeath fires before the NPC death sequence
	// (NPC.ts appendDeath). PreventDeath cancels the death.
	OnNpcBeforeDeath func(e *CombatBeforeDeathEvent)
	// OnPlayerDeath fires the player-death event
	// (PlayerDeathTask.ts). It returns whether a hook took the
	// death over (the handled chain): the death task reads the
	// return back through the event's Handled field, and
	// skips the respawn event + teleport home when handled.
	// The adapter populates ItemsLost/HasItemsLost from the
	// task before emitting.
	OnPlayerDeath func(e *CombatDeathEvent) bool
	// OnPlayerBeforeDeath fires before the player death
	// sequence (Player.ts appendDeath). PreventDeath cancels it.
	OnPlayerBeforeDeath func(e *CombatBeforeDeathEvent)

	// OnPlayerDefeated fires the player-defeated event for
	// EVERY player death (PlayerDeathTask.ts:
	// emitPlayerDefeated, PluginManager.ts:1901-1905): run-all
	// hooks, killer nil for an environmental death. Content
	// applies post-death cleanup here even with no killer.
	OnPlayerDefeated func(killer, victim *Player)
	// ShouldDropItemsOnDeath is the onShouldDropItemsOnDeath
	// first-answer gate (PluginManager.emitShouldDropItemsOnDeath,
	// PluginManager.ts:1548-1569): the first non-nil answer
	// wins; nil means no opinion and the death defaults to
	// dropping items.
	ShouldDropItemsOnDeath func(player, killer *Player) *bool
	// ShouldKeepItemOnDeath is the onShouldKeepItemOnDeath
	// first-answer gate (PluginManager.emitShouldKeepItemOnDeath,
	// PluginManager.ts:1571-1589), consulted per item while
	// ranking the kept items: true keeps the item beyond the
	// count, false never keeps it, nil means no opinion.
	ShouldKeepItemOnDeath func(player *Player, item Item) *bool
	// OnPlayerDeathItemDrop fires the per-item death-drop
	// event (PlayerDeath.ts dropItems) and reports whether a
	// hook took the item over (handled skips it; extra drops
	// and SuppressDefaultDrop are read back from the event
	// whether or not a hook handled it).
	OnPlayerDeathItemDrop func(e *DeathItemDropEvent) bool
	// OnPlayerDeathDrops fires the death-drops event once per
	// death, after the per-item hooks — even with nothing
	// lost. Hooks take items by reassigning Items or
	// Untradeables on the event; the caller reads them back.
	OnPlayerDeathDrops func(e *DeathDropsEvent)
	// RequestSave requests a player save with the given
	// reason ("death", "respawn" — the death task's
	// PLAYER_PERSISTENCE.requestSave calls). The coordinator
	// wires it to the save coordinator (the
	// TradeHookSeam.SavePlayer precedent); nil means the save
	// is skipped, a documented gap, not an invented call.
	RequestSave func(player *Player, reason string)

	// OnCombatHitPrepare / OnCombatHitRoll / OnCombatHitDamage
	// / OnCombatHitResolved fire the hit pipeline events.
	// OnCombatHitRolled fires the damage-roll event.
	OnCombatHitPrepare  func(e *CombatHitPrepareEvent)
	OnCombatHitRoll     func(e *CombatHitRollEvent)
	OnCombatHitDamage   func(e *CombatHitDamageEvent)
	OnCombatHitRolled   func(e *CombatHitRolledEvent)
	OnCombatHitResolved func(e *CombatHitResolvedEvent)

	// OnCombatAttackDistance fires the attack-distance event.
	OnCombatAttackDistance func(e *CombatAttackDistanceEvent)
	// OnAttackTiming fires the attack-timing event; newTarget
	// reports whether the target differs from the last
	// attack's target. A nil return means "no opinion" (all
	// zero values).
	OnAttackTiming func(attacker, target *Mobile, method CombatMethod, newTarget bool) *AttackTimingResult
	// OnAttackSpeed fires the attack-speed event.
	OnAttackSpeed func(e *AttackSpeedEvent)
	// OnCombatAttackStarted / OnCombatAttackFinished fire the
	// combat:attack-started / combat:attack-finished custom
	// events around a swing (Combat.ts executeAttack).
	OnCombatAttackStarted  func(attacker, target *Mobile, method CombatMethod)
	OnCombatAttackFinished func(attacker, target *Mobile, method CombatMethod)

	// ManipulateHit is the Mobile.manipulateHit seam
	// (CombatFactory.executeHit): a hook may replace the
	// PendingHit about to land, or return nil to cancel it
	// (TS returns null to cancel). A replacement is
	// re-defended by the caller, as in TS. The per-splat
	// NPC/player hit-modify events fold into this seam:
	// hooks mutate the PendingHit's splats in place.
	ManipulateHit func(target *Mobile, hit *PendingHit) *PendingHit

	// OnCanAttack is the emitCanAttack policy gate
	// (PluginManager.emitCanAttack, PluginManager.ts:1360-1377)
	// in its TS-exact form: attacker, target, and the attacking
	// combat method (nil when the caller has none, e.g. an area
	// secondary target — TS undefined). True = allow, false =
	// deny, nil = no opinion. The hub emit behind it runs the
	// allow-veto contract (plugins.EmitCanAttack): allow is
	// reset before each hook, a deny returns immediately, an
	// allow sticks but a later deny still vetoes, and silence
	// is undecided. The coordinator wires it in
	// cmd/server/combatwiring.go (InstallCombatHooks) from the
	// hub; until then the field stays nil and the gate is
	// inert. Call sites: CanAttackPermission's duel branch
	// (a veto denies, anything else passes) and
	// CanAttackByPolicy (the global hook, after the area
	// hooks).
	//
	// There is no method-less form: TS has only this
	// method-carrying emit (PluginManager.ts:1360-1377, called
	// with the method at CombatFactory.ts:780, 844, 860), so
	// the Phase 16 method-less CanAttack field was removed in
	// phase 30 and its test assignments migrated to this
	// field; production never assigned it (only
	// cmd/server/combatwiring.go ever wired the gate, onto
	// OnCanAttack).
	OnCanAttack func(attacker, target *Mobile, method CombatMethod) *bool

	// ModifyIncomingDamage is the plugin incoming-damage
	// modifier (CombatFactory.applyIncomingHitDamage, players
	// only): it adjusts damage after Elysian reduction.
	// Returning nil keeps the damage.
	ModifyIncomingDamage func(target *Mobile, hit *HitDamage, damage int) *int

	// CheckAmmo is the plugin ammo check
	// (CombatFactory.checkAmmo): true = the shot may fire.
	// Nil means "no opinion" (core checks apply).
	CheckAmmo func(player *Player, weapon, ammo Item) *bool
	// DecrementAmmo is the plugin ammo decrement
	// (CombatFactory.decrementAmmo): returning true means the
	// plugin consumed the ammo and core must not.
	DecrementAmmo func(player *Player, weapon, ammo Item, amount int) bool
	// AmmoRecoveryPercent is the ammo-recovery hook (Ava's
	// devices, CombatFactory.decrementAmmo): the percent of
	// fired ammo recovered, 0 when unset.
	AmmoRecoveryPercent func(player *Player) int
	// DropGroundItem spawns a ground item (ammo drops,
	// CombatFactory.decrementAmmo). Core has no World or
	// ground-item manager reference; the wiring supplies it.
	// Nil drops the item silently (documented gap).
	DropGroundItem func(item Item, loc Location, owner *Player)

	// ApplySkull / IsSkulled / ExtendSkull are the skull-system
	// seam (CombatFactory.handleSkull/wouldSkull): the live
	// system behind it is Player.ApplySkull / IsSkulled /
	// ExtendSkull (skull.go), wired in cmd/server
	// (InstallCombatHooks). The zero seam is inert — the
	// attacked-by recording in HandleSkull still runs — and
	// IsSkulled reads false with no wiring, as before the
	// system existed.
	ApplySkull  func(player *Player, seconds int)
	IsSkulled   func(player *Player) bool
	ExtendSkull func(player *Player, seconds int)

	// SpecialAttackCost is the plugin special-cost event
	// (CombatSpecial.cost): the energy cost override for a
	// special. Nil means the definition drain amount.
	SpecialAttackCost func(player *Player, special *CombatSpecial) *int

	// SpellDisabled is the onSpellDisabled first-answer gate
	// (PluginManager.emitSpellDisabled,
	// PluginManager.ts:1824-1849; the hub side is
	// Hub.EmitSpellDisabled, internal/plugins/spells.go).
	// True = the spell is disabled for this player (the cast
	// is refused). Nil = not disabled. It is consulted by
	// Spell.canCast before anything else is spent.
	SpellDisabled func(player *Player, spellbook Spellbook, spellID int, spell CombatSpell) bool
	// SpellRuneBypass is the onSpellRuneBypass first-answer
	// gate (PluginManager.emitSpellRuneBypass,
	// PluginManager.ts:1851-1876; the hub side is
	// Hub.EmitSpellRuneBypass). True = the cost is bypassed.
	// RuneID is the rune being queried, or 0 for the
	// whole-cost query (TS passes runeId undefined there;
	// the hub uses 0 for it — plugins/spells.go,
	// SpellRuneBypassEvent). Nil = no bypass.
	SpellRuneBypass func(player *Player, spellbook Spellbook, spellID, runeID int) bool
	// CheckSpellRuneSource is the rune-source check
	// (PluginManager.checkSpellRuneSource,
	// PluginManager.ts:5056-5074; the hub side is
	// Hub.CheckSpellRuneSource). True = a registered source
	// (e.g. a rune pouch) can supply every item. Nil = no
	// source (no rune pouch exists in Go).
	CheckSpellRuneSource func(player *Player, items []Item) bool
	// ConsumeSpellRuneSource is the rune-source consumption
	// (PluginManager.consumeSpellRuneSource,
	// PluginManager.ts:5076-5092; the hub side is
	// Hub.ConsumeSpellRuneSource). Nil is a no-op.
	ConsumeSpellRuneSource func(player *Player, items []Item)

	// PrayerDisabled answers whether a prayer is disabled for
	// a player by content (PrayerHandler.canUse plugin gate).
	// Nil means no prayer is disabled.
	PrayerDisabled func(player *Player, prayer int) bool

	// ModifyMagicMaxHit is the combat:magic-max-hit event
	// (DamageFormulas.sourceMaxHit): adjust a magic max hit.
	// Nil keeps the computed value.
	ModifyMagicMaxHit func(attacker, target *Mobile, maxHit int) *int
	// ModifyMagicHitBounds is the combat:magic-hit-bounds
	// event: adjust a magic hit's {min, max} bounds. Nil keeps
	// the computed bounds.
	ModifyMagicHitBounds func(attacker, target *Mobile, min, max int) (int, int, bool)
	// ModifyRangedMaxHit is the modifyRangedMaxHit hook
	// (DamageFormulas.sourceMaxHit): adjust a ranged max hit.
	ModifyRangedMaxHit func(attacker, target *Mobile, maxHit int) *int
	// ModifyDefenceRoll is the combat:defence-roll event
	// (AccuracyFormulasDpsCalc.specialRolls): adjust a defence
	// roll. Nil keeps the computed roll.
	ModifyDefenceRoll func(attacker, target *Mobile, roll int) *int
	// ModifyAccuracyOutcome is the combat:accuracy-rolled
	// event (AccuracyFormulasDpsCalc.rollAccuracy): override
	// the rolled outcome. Nil keeps the rolled outcome.
	ModifyAccuracyOutcome func(attacker, target *Mobile, accurate bool) *bool
	// AccuracyScale is the scoped accuracy scale
	// (AccuracyFormulasDpsCalc.withAccuracyScale): a
	// multiplier applied to the attack roll, 1.0 when unset.
	AccuracyScale func(attacker, target *Mobile) float64

	// OnPlayerRespawn runs the player respawn after the death
	// sequence (PlayerDeathTask respawn tick). Nil runs the
	// core default: restore hitpoints, send the death message,
	// teleport home (see combat_death.go). Item loss already
	// ran on the respawn tick, before this hook.
	OnPlayerRespawn func(player *Player, message string)
	// AreaAttackTargets lists the secondary targets of an
	// area special attack (SpecialAttackTargetingResolver,
	// Combat.ts executeAttack): the mobiles in range of the
	// primary target. Nil means no secondary targets.
	AreaAttackTargets func(attacker, target *Mobile, radius int) []*Mobile
	// SpellAreaTargets lists the secondary targets of a
	// barrage spell (MagicCombatMethod.hits). Nil means none.
	SpellAreaTargets func(attacker, target *Mobile, radius int) []*Mobile
	// RetaliationHomeCheck answers whether an NPC is in its
	// coordinator HOME state for retaliation
	// (CombatFactory.handleRetaliation). Nil falls back to
	// the coordinator state read in combat code.
	RetaliationHomeCheck func(npc *NPC) bool
}

// Hooks is the populated combat hook seam. The coordinator /
// Worker 3 wiring assigns its fields at boot; combat code
// reads it nil-safely field by field. It is a var (not a
// pointer) so a zero Hooks is a valid inert seam.
var Hooks CombatHooks

// ---- NPC combat-method providers ----
//
// NPCCombatMethodProvider is PluginNpcCombatMethodProvider
// (PluginTypes.ts:932-942): content registers a provider for a
// set of NPC ids, and CombatFactory.getMethod consults the
// providers in registration order before falling back to the
// NPC's definition attack type (CombatFactory.ts:174). The
// provider returns the method for this NPC, or nil to pass.

// NPCCombatMethodProvider supplies a combat method for an NPC.
// A nil return means "no method from this provider".
type NPCCombatMethodProvider interface {
	Provide(npc *NPC) CombatMethod
}

// NPCCombatMethodProviderFunc adapts a func to the provider
// interface.
type NPCCombatMethodProviderFunc func(npc *NPC) CombatMethod

// Provide calls the func.
func (f NPCCombatMethodProviderFunc) Provide(npc *NPC) CombatMethod {
	if f == nil {
		return nil
	}
	return f(npc)
}

type npcCombatMethodProviderEntry struct {
	pluginName string
	provider   NPCCombatMethodProvider
	npcIDs     map[int]bool
}

var npcCombatMethodProviders []npcCombatMethodProviderEntry

// RegisterNPCCombatMethodProvider registers a combat-method
// provider for npcIDs (PluginTypes.ts:1548,
// registerNpcCombatMethodProvider; the options form with a
// constructor is a plugin-side shape — Go registers the
// provider directly). Providers are consulted in registration
// order; the first whose id set contains the NPC's id (or real
// id — transforms are not ported, so id only) and whose
// Provide returns non-nil wins (CombatFactory.ts:174-178).
// Worker 3's hub code delegates to this function. Register
// before ticking starts; the registry is not synchronized.
func RegisterNPCCombatMethodProvider(pluginName string, npcIDs []int, provider NPCCombatMethodProvider) {
	ids := make(map[int]bool, len(npcIDs))
	for _, id := range npcIDs {
		ids[id] = true
	}
	npcCombatMethodProviders = append(npcCombatMethodProviders, npcCombatMethodProviderEntry{
		pluginName: pluginName,
		provider:   provider,
		npcIDs:     ids,
	})
}

// npcCombatMethodFromProviders runs the provider registry for
// npc, in order. Nil when no provider supplies a method.
func npcCombatMethodFromProviders(npc *NPC) CombatMethod {
	if npc == nil {
		return nil
	}
	for _, e := range npcCombatMethodProviders {
		if e.provider == nil || !e.npcIDs[npc.ID] {
			continue
		}
		if m := e.provider.Provide(npc); m != nil {
			return m
		}
	}
	return nil
}

// ---- Combat-method resolvers (players) ----
//
// CombatFactory.getMethod consults plugin combat-method
// resolvers for players before the weapon-based fallback
// (CombatFactory.ts getMethod). A resolver returns a method
// for the attacker, or nil to pass. Registration order decides.

var combatMethodResolvers []func(attacker *Player) CombatMethod

// RegisterCombatMethodResolver registers a player
// combat-method resolver (the getMethod plugin hook). Worker
// 3's hub code delegates to this function.
func RegisterCombatMethodResolver(resolver func(attacker *Player) CombatMethod) {
	if resolver != nil {
		combatMethodResolvers = append(combatMethodResolvers, resolver)
	}
}

// resolveCombatMethod runs the player resolvers in order.
func resolveCombatMethod(attacker *Player) CombatMethod {
	for _, r := range combatMethodResolvers {
		if m := r(attacker); m != nil {
			return m
		}
	}
	return nil
}
