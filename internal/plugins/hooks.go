// Hook storage, hook execution, and the lifecycle event types for the Go
// plugin hub. The hook lists live in hookRegistry (one per Hub, created by
// NewHub); every hook invocation goes through the single executeHook helper,
// mirroring PluginManager.executeHook (PluginManager.ts:429-466).
//
// UNFIRED-FAMILY TABLE (required; this file is its home)
//
// The PluginApi interface (PluginTypes.ts:999-1559) declares 108 distinct
// hook families (on* methods; four have two overloads each and count once:
// onObjectInteraction, onItemOnObject, onItemOnItem,
// onItemAction). Fifty-nine get a registration method: the six lifecycle
// families below (Hub.Emit* methods in this file; the coordinator wires
// the production call sites) plus onCustomEvent (events.go), onCommand
// (api.go; fired by the command registry through Hub.EmitCommandHooks,
// commands.go), onSocialPacket (api.go; fired for all eight
// packet kinds — public_chat by the chat listener, the other
// seven by the social dispatch through the cmd/server adapter,
// see EmitSocialPacket), the ten object families in objects.go
// (onObjectRoute, onObjectInteraction, onBeforeObjectInteraction,
// onObjectClick, onObjectFirstClick, onObjectSecondClick,
// onObjectThirdClick, onObjectFourthClick, onObjectFifthClick,
// onItemOnObject; fired by the object-action and item-on-object
// listeners and the walk-to-object path), the onItemOnItem family
// in itemonitem.go (onItemOnItem and OnItemOnItemNamed, fired by
// the inventory-use-on listener through the cmd/server adapter),
// the onNpcClick family in npcinteraction.go (onNpcClick, fired
// by the NPC-option listener through the cmd/server adapter),
// plus the onNpcInteraction family in the same file (fired by
// the same emit, after the click bindings — see
// npcinteraction.go), plus the onAnyNpcInteraction family in
// the same file (phase 41: fired by the same emit, last, after
// the name hooks — see npcinteraction.go),
// the two dialogue
// families in dialogue.go (onNpcDialogueVariant,
// onNpcDialogueCondition; fired by the transcript playback
// through the cmd/server adapter — see dialogue.go), the two zone
// families in zones.go (onZoneEnter, onZoneExit; driven by
// updatePlayerZones from EmitPlayerProcess, before the process hooks),
// the eleven combat families in combat.go (onNpcDeath,
// onNpcAttackOption, onPlayerAttack, onPlayerDeath,
// onPlayerDeathItemDrop, onPlayerDeathDrops, onCombatHitPrepare,
// onCombatHitRoll, onCombatHitDamage, onCombatHitResolved,
// onCombatAttackDistance; the death and hit families fire from the
// combat/death pipelines through the game-side CombatHooks seam,
// the attack-option family gates the NPC attack click, and the
// player-attack family fires from the player attack click — all
// through cmd/server wiring, see combat.go), plus onCanAttack in
// combat.go (phase 26: fired from CanAttackPermission's duel branch
// and canAttackByPolicy's global hook through the game-side
// CombatHooks.OnCanAttack seam, also via cmd/server wiring), plus
// onShouldDropItemsOnDeath, onShouldKeepItemOnDeath, and
// onPlayerDefeated in combat.go (phase 28: fired from the death
// task's respawn tick and kept-items ranking through the
// game-side CombatHooks seams, also via cmd/server wiring), the two spell families
// in spells.go (onSpellDisabled, fired from Spell.canCast through the
// game-side CombatHooks.SpellDisabled seam, and onSpellRuneBypass, fired from
// Spell.itemsToConsume and the MagicOnItem listener through the
// CombatHooks.SpellRuneBypass seam — both phase 27, via cmd/server wiring), plus
// onSpellOnItem in the same file (phase 43: fired from castSpellOnItem in the
// MagicOnItem listener through the listeners' SpellHooks.EmitSpellOnItem seam,
// wired in cmd/server/spellwiring.go; Superheat Item is its first consumer), the two shop families
// in shops.go (onCanShop, fired from ShopManager.open through the
// game-side shop seam; onCanStockItem, fired per stock entry at
// shop-definition load, after Hub.Load — see shops.go), the three
// trade families in trades.go (onCanTrade, fired from
// Trading.canTrade at request and accept time; onTradeRequest,
// fired from the trade-request listener before the walk;
// onTradeCompleted, fired twice per completed trade, once per
// player, before either player is saved — all through the
// game-side trade seam and cmd/server wiring, see trades.go),
// and the two interface-action families in interfaces.go
// (onInterfaceActionClick, onInterfaceActionButton; fired by
// Hub.EmitInterfaceActionClick from the interface-action
// dispatch paths in the listeners package through the cmd/server
// adapter, after the MultiChatboxPrompt check — see
// interfaces.go), the three skills families in skills.go
// (onPlayerLevelUp, onExperienceGained, onCanGainExperience;
// fired from SkillManager.addExperience through the game-side
// SkillsHooks seam and cmd/server wiring, see skills.go),
// and the world-entity family in sailing.go
// (onWorldEntityInteraction; fired by the world-entity
// listener through Hub.EmitWorldEntityInteraction's
// first-handled-wins gate — see sailing.go), and the three
// item-action families in itemaction.go (onItemAction,
// onItemFirstAction, onCanUseItem; fired by the item-action
// listener through the cmd/server adapter — see itemaction.go).
// The other 50 get NO registration method this phase.
// Each is listed below with the missing engine system named. A family
// gets a method only when its event type can be honestly typed AND an
// emit site exists or is imminent; no family is silently dropped — if
// it is in PluginApi and has no method, it is in this table.
//
// Chat, clan & relations — (`onSocialPacket` is ported: it
// fires for all eight packet kinds of the TS union
// (PluginTypes.ts:67-79) — the chat listener emits public_chat
// and the social dispatch emits the other seven; the relations
// engine itself lives in the game package, phase 19. The eight
// kinds are variants of the one onSocialPacket family, already
// counted firable since Phase 12, so they add no family and
// the firable count does not increase.) `onFriendAdd` and
// `onFriendRemove` stay table-only: TS defines the emits
// (emitFriendAdd/emitFriendRemove, PluginManager.ts:863-881)
// and the registration guards (PluginManager.ts:3269-3296),
// but NOTHING calls them — a grep over server/src and
// server/plugins at commit bfd81e3 finds zero emit call sites
// and zero .onFriendAdd/.onFriendRemove registrations, so the
// events can never fire upstream. Adding registration methods
// or emit sites would invent behavior TS does not have; the
// families get methods only if a genuine emit site is found.
//
// Map-square tracking — no per-player map-square tracking (TS
// emitPlayerMapSquareChange, PluginManager.ts:812):
// `onPlayerMapSquareChange`. (`onZoneEnter` and
// `onZoneExit` are ported in zones.go, driven by updatePlayerZones from
// EmitPlayerProcess.)
//
// Skills & experience — (onPlayerLevelUp, onExperienceGained,
// and onCanGainExperience are ported in skills.go, fired from
// SkillManager.addExperience through the game-side SkillsHooks
// seam and cmd/server wiring; the gate is the
// veto-plus-accumulator contract of PluginManager.ts:1665-1674.
// No skilling XP sources exist yet beyond combat — the gate and
// the emits are the engine side, and individual skills port
// separately.)
//
// Regions & active-region index — RegionManager loads regions with no hub
// seam; no active-region index: `onRegionLoaded`, `onActiveRegionsUpdated`.
//
// Pathfinding events — PathFinder exists but emits nothing when a path is
// blocked: `onPathBlocked`, `onPlayerPathBlocked`.
//
// Object interaction — the spell system is NOT STARTED, so the
// spell-on-object family has no emit site: `onSpellOnObject`.
// (The other ten object families are ported in objects.go:
// onObjectRoute, onObjectInteraction, onBeforeObjectInteraction,
// the onObjectClick family, and onItemOnObject.)
//
// NPC interaction & routing — spawned NPCs have drivers but no NPC click
// routing to the hub beyond the attack option and the click
// family: `onNpcRoute`,
// `onBeforeNpcInteraction`, `onNpcsInteraction`,
// `onNpcFirstClick`,
// `onNpcSecondClick`, `onNpcThirdClick`, `onNpcFourthClick`,
// `onItemOnNpc`, `onSpellOnNpc`. (`onNpcAttackOption` is ported in
// combat.go: it gates the NPC attack click in the NPC-option
// listener. `onNpcClick` is ported in npcinteraction.go: it runs
// the registered click hooks from Hub.EmitNpcInteraction, which
// the NPC-option listener calls through the cmd/server adapter.
// `onNpcInteraction` is ported in npcinteraction.go since Phase
// 35: the same emit runs the name-based hooks after the click
// bindings. `onAnyNpcInteraction` is ported in npcinteraction.go
// since Phase 41: the same emit runs the generic list last, after
// the name hooks — TS order, `PluginManager.ts:1246-1263`.)
//
// World entities & sailing — (onWorldEntityInteraction is
// ported in sailing.go, fired by the world-entity listener
// through Hub.EmitWorldEntityInteraction's first-handled-wins
// gate; the Sailing runtime and BoatManager live in the game
// package, Phase 21. No other sailing/world-entity family
// exists in PluginApi: sailing and private-area signalling
// ride the custom-event surface.)
//
// Dialogue — (onNpcDialogueVariant and onNpcDialogueCondition are
// ported in dialogue.go, fired by the transcript playback through
// the cmd/server adapter; the npc-dialogue:* custom events ride
// the existing custom-event surface. No PluginApi/PluginCoreApi
// dialogue member exists to port: getDialogueManager is a Player
// method, and the core dialogue classes await Worker B's game
// types — see dialogue.go.)
//
// NPC lifecycle & hits — the death task fires onNpcDeath, but the
// before-death gate and the hit-modify families need the death-task
// and hit-pipeline call sites plus the Hit types (Worker 2
// follow-up, outside the Phase 16 contracts): `onNpcBeforeDeath`,
// `onNpcHitModify`. (`onNpcDeath` is ported in combat.go.)
//
// Combat gates & hits — the hit-pipeline families below are ported,
// but the gates and timing families need emit sites inside
// canAttackPermission, the hit-resolution path, and the attack
// cycle, plus the CombatMethod/Hit types (Worker 2 follow-up,
// outside the Phase 16 contracts):
// `onPlayerHitModify`, `onPlayerDealtDamage`, `onAttackTiming`,
// `onAnimationStall`, `onAttackSpeed`,
// `onNpcAggressionTolerance`.
// (`onCombatHitPrepare`, `onCombatHitRoll`, `onCombatHitDamage`,
// `onCombatHitResolved`, and `onCombatAttackDistance` are ported
// in combat.go, fired through the CombatHooks seam, as is
// `onCanAttack` since phase 26; `onSpellDisabled` and
// `onSpellRuneBypass` are ported in spells.go, fired through
// the CombatHooks spell seams since phase 27.)
//
// Death — the death tasks fire the ported families below, but the
// before-death gate needs the death-task call site (Worker 2
// follow-up, outside the Phase 16 contracts): `onPlayerBeforeDeath`.
// (`onPlayerDeathItemDrop`, `onPlayerDeathDrops`, and
// `onPlayerDeath` are ported in combat.go, fired through the
// CombatHooks seam; `onShouldDropItemsOnDeath`,
// `onShouldKeepItemOnDeath`, and `onPlayerDefeated` are ported
// in combat.go since phase 28, fired from the respawn tick and
// the kept-items ranking through the CombatHooks seams.)
//
// Teleport — TeleportHandler NOT STARTED: `onCanTeleport`.
//
// Logout gate — the gate helper exists (gates.go) but there is no Hub emit
// or coordinator call site at the logout-request seam yet (the
// World.LogoutRefusal hook exists; wiring is a later phase): `onCanLogout`.
//
// Trading — (onCanTrade, onTradeRequest, and onTradeCompleted are
// ported in trades.go, fired from the trading system and the
// trade-request listener through the game-side trade seam.)
//
// Player follow/options/PvP — follow and player-option routing NOT STARTED
// (walk-to-entity is NOT STARTED): `onPlayerFollow`, `onPlayerOption`.
// (`onPlayerAttack` is ported in combat.go: it fires from the
// player-option listener after the attack option starts an attack.)
//
// Bank & shops — the Bank container exists but there is no bank-open
// flow: `onCanBank`, `onCanBankItem`. (`onCanShop` and
// `onCanStockItem` are ported in shops.go: the shop-open gate and
// the load-time stock veto.)
//
// Prayer & special attack — prayer and special-attack systems NOT STARTED:
// `onPrayerDisabled`, `onSpecialAttackDisabled`.
//
// Consumables & equipment gates — no eat/drink content and no gate call
// sites at the equip/unequip paths (the equip listener exists but calls no
// hub gate): `onCanEat`, `onCanDrink`, `onFiremakingBlocked`,
// `onCanEquip`, `onCanUnequip`. (`onCanUseItem` is ported in
// itemaction.go: it fires from the item-action listener through
// the cmd/server adapter.)
//
// NPC spawn gate — spawns apply eagerly with no gate call site:
// `onCanSpawnNpc`.
//
// Item-on-X, spells & item actions — the spell-on-object
// family has no emit site and the drop listener calls no hub
// emit: `onItemOnPlayer`, `onItemOnGroundItem`, `onSpellCast`,
// `onItemDropPolicy`. (`onSpellOnItem` is ported in spells.go,
// fired from castSpellOnItem through the listeners'
// SpellHooks.EmitSpellOnItem seam; `onItemAction` and
// `onItemFirstAction` are ported in itemaction.go, fired by
// the item-action listener through the cmd/server adapter;
// `onItemOnItem` is ported in itemonitem.go, fired by the
// inventory-use-on listener through the cmd/server adapter.)
//
// Ground-item clicks — GroundItemManager and the pickup listener exist but
// call no hub emit: `onGroundItemClick`, `onGroundItemPickup`.
//
// Walk routing — MovementQueue exists but emits nothing when a route is
// requested: `onWalkRoute`.
//
// Buttons & interfaces — the simple-click button families have no
// emit: `onButtonClick`, `onButton`. (`onInterfaceActionClick`
// and `onInterfaceActionButton` are ported in interfaces.go,
// fired by Hub.EmitInterfaceActionClick from the listeners'
// interface-action dispatch; `onCommand` is ported: the command
// registry fires it through Hub.EmitCommandHooks. The
// simple-click emitButtonClick fallback in the widget-action
// fallthrough has no Go emit: with no button hooks registered
// it can never claim a click.)
//
// NON-HOOK PluginApi MEMBERS (95 total; 30 ported —
// registerLoginExtension in login.go, getPluginConfig, core, and
// emitCustomEvent on Api, persistAttribute as Api.PersistAttribute
// (api.go, Phase 11), registerCommand, getRegisteredCommands,
// and setCommandRights as Api methods delegating to the hub's
// command registry (api.go, Phase 12), getObjectManager,
// emitObjectInteraction, and emitItemOnObject as Api methods in
// objects.go (Phase 13), sendMultiChatboxPrompt as an Api method
// in dialogue.go (Phase 14), registerArea as Api.RegisterArea
// in areas.go (Phase 15), emitNpcAttackOption and
// registerNpcCombatMethodProvider as Api methods in combat.go
// (Phase 16), registerShopCurrency and
// registerItemShopCurrency as Api methods in shops.go (Phase 17,
// delegating to the installed ShopCurrencyRegistry with
// queue-until-installed semantics), refuseWhileBusy as
// Api.RefuseWhileBusy in interfaces.go (Phase 18, recording
// button ids in the hub's busy-refusal set, read by
// Hub.RefusesWhileBusy), getTaskManager as Api.GetTaskManager
// in skills.go (Phase 20, returning the TaskManager installed
// through Hub.SetTaskManager), getAreaManager,
// replaceMapRegion, replaceMapRegionData, and restoreMapRegion
// as Api methods in instancing.go (Phase 21, delegating to the
// installed area manager and region-replacement wiring),
// emitWorldEntityInteraction as Api.EmitWorldEntityInteraction
// in sailing.go (Phase 21, delegating to
// Hub.EmitWorldEntityInteraction), and
// registerRunEnergyRestoreModifier and
// registerRunEnergyDrainModifier as Api methods in equipment.go
// (Phase 22, delegating to the installed
// RunEnergyModifierRegistry with queue-until-installed
// semantics), and registerSpellRuneSource as
// Api.RegisterSpellRuneSource in spells.go (Phase 27, pushing
// onto the hub's rune-source list with no installed registry to
// wait for — the hub itself is the registry), and
// emitNpcInteraction as Api.EmitNpcInteraction in
// npcinteraction.go (Phase 33, delegating to
// Hub.EmitNpcInteraction), and onAnyNpcInteraction as
// Api.OnAnyNpcInteraction in npcinteraction.go (Phase 41, the
// generic list Hub.EmitNpcInteraction runs last) — the other
// 64 have no Go equivalent yet):
//
// Engine accessors & logging — no Core surface beyond the scoped Core in
// core.go; the systems are missing or adapter-owned: `getWorld`,
// `getRegionManager`, `getCombatFactory`,
// `getPrayerHandler`, `getBonusManager`, `getItemOnGroundManager`,
// `getServerPerf`, `getSkillManager`,
// `getPlayerPunishment`, `getActiveRegionSnapshot`, `log`.
// (`getObjectManager` was in this group; it is ported as
// Api.GetObjectManager in objects.go, delegating to the hub's
// installed ObjectManagerAPI, with Core.ObjectManager as the
// core.ObjectManager equivalent. `getTaskManager` was in this
// group; it is ported as Api.GetTaskManager in skills.go,
// returning the TaskManager installed through
// Hub.SetTaskManager, with no Core.TaskManager accessor.
// `getAreaManager` was in this group; it is ported as
// Api.GetAreaManager in instancing.go, returning the narrow
// AreaManagerQuery installed through Hub.SetAreaManager.)
//
// Spawning, areas & definitions — NPC drivers and the
// definition-loader source registry have no plugin seam: `spawnNpc`,
// `removeNpc`, `registerDefinitionSource`,
// `registerNpcInteraction`, `setDeckReach`, `setUsableFromDeck`.
// (`registerArea` was in this group; it is ported as
// Api.RegisterArea in areas.go, pushing onto the installed
// AreaRegistry with queue-until-installed semantics.)
//
// Commands — (registerCommand, getRegisteredCommands, and
// setCommandRights were in this group; they are ported as Api
// methods in api.go, delegating to the hub's command registry.)
//
// Content endpoints & prompts — no HTTP content API: the
// custom-interface definition is served at
// /api/interfaces/<groupId> over the HTTP ContentApi
// (net/http/ContentApi.ts; PluginTypes.ts:1285-1288) and the
// transport is WS-only: `registerContentEndpoint`,
// `registerCustomInterface`. (`sendMultiChatboxPrompt` was in
// this group; it is ported as an Api method in dialogue.go.)
//
// Map regions — (replaceMapRegion, replaceMapRegionData, and
// restoreMapRegion were in this group; they are ported as Api
// methods in instancing.go, delegating to the installed
// region-replacement wiring.)
//
// Shop currency — (registerShopCurrency and
// registerItemShopCurrency were in this group; they are ported as
// Api methods in shops.go, delegating to the installed
// ShopCurrencyRegistry with queue-until-installed semantics.)
//
// Persistence, profile & world — persistence override, world profile/type,
// and experience-rate seams are adapter-owned with no plugin surface:
// `setPlayerPersistence`, `setExperienceRate`, `setWorldProfile`,
// `setWorldType`. (`persistAttribute` was in this group; it is ported as
// Api.PersistAttribute in api.go, delegating to the persist registry.)
//
// Cross-plugin emits — the underlying gate/interaction emits do not exist
// yet (see the family table above): `emitCanEat`, `emitCanDrink`,
// `emitCanBank`, `emitCanBankItem`, `emitShouldKeepItemOnDeath`,
// `emitFiremakingBlocked`,
// `emitPlayerLogin`. (`emitObjectInteraction` and `emitItemOnObject`
// were in this group; they are ported as Api methods in objects.go,
// delegating to Hub.EmitObjectInteraction / Hub.EmitItemOnObject.
// `emitNpcAttackOption` was in this group; it is ported as
// Api.EmitNpcAttackOption in combat.go, delegating to
// Hub.EmitNpcAttackOption. `emitNpcInteraction` was in this
// group; it is ported as Api.EmitNpcInteraction in
// npcinteraction.go, delegating to Hub.EmitNpcInteraction.
// `emitWorldEntityInteraction` was in
// this group; it is ported as Api.EmitWorldEntityInteraction
// in sailing.go, delegating to Hub.EmitWorldEntityInteraction.)
//
// Performance & profiling — no perf profiling (explicit gap):
// `withProfileLabel`, `getPluginPerformanceSnapshot`,
// `resetPluginPerformanceStats`, `setPluginPerformanceProfilingEnabled`,
// `isPluginPerformanceProfilingEnabled`.
//
// Busy refusal — (`refuseWhileBusy` was in this group; it is
// ported as Api.RefuseWhileBusy in interfaces.go, recording
// button ids in the hub's busy-refusal set, read by
// Hub.RefusesWhileBusy for the opcode's locked-player gate.)
//
// Combat modifiers & providers — combat modifier registries and the
// combat-engine/provider surface beyond the NPC method provider are
// NOT STARTED:
// `registerCombatEffectiveLevelModifier`,
// `registerMeleeStrengthPercentModifier`, `registerMeleeStrengthBonusModifier`,
// `registerRestingLevelModifier`, `registerMeleeHitModifier`,
// `registerRangedHitModifier`, `registerMagicHitModifier`,
// `registerMagicDamageBonusModifier`, `registerMagicAccuracyPercentModifier`,
// `registerMeleeAttackAccuracyModifier`, `registerMeleeDefenseModifier`,
// `registerRangedDefenseModifier`, `registerRangedAttackAccuracyModifier`,
// `registerMagicAttackAccuracyModifier`, `registerMagicDefenseModifier`,
// `registerRunEnergyStatusModifier`, `registerSpecialEnergyRestoreModifier`,
// `registerSpecialEnergyCostModifier`, `registerIncomingDamageModifier`,
// `registerPrayerDrainModifier`, `registerPrayerDrainRateModifier`,
// `setCombatEngine`, `setCombatDamageProvider`, `registerBonusProvider`,
// `registerRangedAmmoResolver`, `registerRangedAmmoHandler`,
// `registerRangedAmmoRecovery`,
// `registerRangedCombatModifier`, `registerWeaponProfile`,
// `registerCombatSpecial`, `registerCombatMethodResolver`,
// `registerNpcCombatOption`. (`registerNpcCombatMethodProvider` was in
// this group; it is ported as Api.RegisterNpcCombatMethodProvider in
// combat.go, delegating to the installed provider registry with
// queue-until-installed semantics.
// `registerRunEnergyRestoreModifier` and
// `registerRunEnergyDrainModifier` were in this group; they are
// ported as Api.RegisterRunEnergyRestoreModifier and
// Api.RegisterRunEnergyDrainModifier in equipment.go, delegating
// to the installed modifier registry with queue-until-installed
// semantics.)

package plugins

import (
	"log"
	"sync"

	"tsps-go/internal/game"
)

// hookEntry is one registered hook: the owning plugin's name (for error
// labelling) plus the handler. It mirrors PluginHook<T>
// (PluginManager.ts:141-146) minus the profiling fields (profileName,
// handlerName): there is no perf profiling in Go (explicit gap), and minus
// PluginContext: Go has no thread-locals, so there is no ambient
// current-plugin — hook errors already label the plugin name (documented
// deviation).
type hookEntry[T any] struct {
	pluginName string
	handler    func(T)
}

// hookRegistry holds every hook list for one Hub, guarded by a sync.RWMutex:
// registration happens on the boot goroutine (during Load) while emits
// happen on tick/session goroutines. Emits snapshot the list under a read
// lock and run handlers without holding the lock, so a handler that
// registers during an emit (only possible during the plugins:loaded emit,
// before the registry seals) cannot deadlock.
//
// The registry also holds the hub's hooks-owned shared state that cannot
// live on the loader-owned Hub struct from this file's side of the package:
// the load seal, the shared Core instance, and the single login-extension
// slot. All three are created here so the hooks worker never depends on
// Hub field names beyond the contracted `hooks` field.
type hookRegistry struct {
	mu sync.RWMutex

	serverStartup    []hookEntry[*ServerLifecycleEvent]
	serverShutdown   []hookEntry[*ServerLifecycleEvent]
	playerLogin      []hookEntry[*PlayerLoginEvent]
	playerLogout     []hookEntry[*PlayerLogoutEvent]
	playerDisconnect []hookEntry[*PlayerDisconnectEvent]
	playerProcess    []hookEntry[*PlayerProcessEvent]
	commandHooks     []hookEntry[*game.CommandEvent]
	socialPacket     []hookEntry[*SocialPacketEvent]
	custom           map[string][]hookEntry[any]

	// Object-interaction hooks (see objects.go): the route hooks,
	// the before-interaction hooks, and the three ordered
	// interaction lists TS merges at emit time — generic hooks,
	// hooks keyed by "id:clickType", and hooks keyed by object
	// name — sharing one registration-order counter
	// (PluginManager.ts:294-300). Item-on-object hooks live in
	// two lists, main and fallback (PluginManager.ts:374-376).
	objectRoute     []hookEntry[*ObjectRouteEvent]
	beforeObject    []hookEntry[*ObjectInteractionEvent]
	objectGeneric   []objectInteractionHook
	objectByID      map[string][]objectInteractionHook
	objectByName    map[string][]objectInteractionHook
	nextObjectOrder int
	// Item-action hooks (see itemaction.go): the main and
	// fallback lists (PluginManager.ts:374-376), plus the
	// can-use-item gate list and the installed item-actions
	// resolver. One list per family, in registration order.
	itemAction       []hookEntry[*PluginItemActionEvent]
	itemActionFallbk []hookEntry[*PluginItemActionEvent]
	canUseItem       []hookEntry[*CanUseItemEvent]
	itemActionsFn    ItemActionsResolver

	itemOnObject       []hookEntry[*ItemOnObjectEvent]
	itemOnObjectFallbk []hookEntry[*ItemOnObjectEvent]

	// Item-on-item hooks (see itemonitem.go): one list, in
	// registration order; TS has no fallback list for this
	// family (PluginManager.ts:377).
	itemOnItem []hookEntry[*PluginItemOnItemEvent]

	// Dialogue hooks (see dialogue.go): variant and condition
	// handlers RETURN answers instead of mutating the event,
	// so they use dialogueHook (gates.go), not hookEntry.
	npcDialogueVariant   []dialogueHook[*NpcDialogueContext, *NpcDialogueVariantChoice]
	npcDialogueCondition []dialogueHook[*NpcDialogueConditionEvent, *bool]

	// Zone hooks (see zones.go): ONE list, each entry carrying
	// only onEnter or only onExit (PluginManager.ts:326-331),
	// plus the per-player active sets keyed by hook index —
	// the Go form of the TS WeakMap (PluginManager.ts:332),
	// discarded explicitly via forgetZoneState because Go
	// has no weak map.
	zoneHooks   []zoneHook
	activeZones map[*game.Player]map[int]struct{}

	// Combat hooks (see combat.go): the death, attack-option,
	// and hit-pipeline lists. One list per family, in
	// registration order; the emits snapshot and run them
	// through executeHook / runHandledGate.
	npcDeath        []hookEntry[*NpcDeathEvent]
	npcAttackOption []hookEntry[*NpcAttackOptionEvent]

	// NPC-click hooks (see npcinteraction.go): one list, in
	// registration order; each entry carries the NPC ids and
	// click type it covers, and the emit snapshots the
	// matching entries only.
	npcClick []npcClickHook
	// NPC-interaction hooks (see npcinteraction.go): one
	// list, in registration order; each entry carries the NPC
	// definition name and its option-text-keyed actions, and
	// the emit snapshots the matching names only.
	npcInteraction []npcInteractionHook
	// Any-NPC-interaction hooks (see npcinteraction.go): one
	// list, in registration order; each entry carries its
	// option-text-keyed actions, and the emit runs every entry
	// whose map holds the clicked option text — the generic
	// list TS runs last in emitNpcInteraction
	// (PluginManager.ts:1246-1263).
	anyNpcInteraction      []anyNpcInteractionHook
	playerAttack           []hookEntry[*PlayerAttackEvent]
	playerDeath            []hookEntry[*PlayerDeathEvent]
	playerDeathItemDrop    []hookEntry[*PlayerDeathItemDropEvent]
	playerDeathDrops       []hookEntry[*PlayerDeathDropsEvent]
	shouldDropItemsOnDeath []hookEntry[*ShouldDropItemsOnDeathEvent]
	shouldKeepItemOnDeath  []hookEntry[*ShouldKeepItemOnDeathEvent]
	playerDefeated         []hookEntry[*PlayerDefeatedEvent]
	combatHitPrepare       []hookEntry[*CombatHitPrepareEvent]
	combatHitRoll          []hookEntry[*CombatHitRollEvent]
	combatHitDamage        []hookEntry[*CombatHitDamageEvent]
	combatHitResolved      []hookEntry[*CombatHitResolvedEvent]
	combatAttackDistance   []hookEntry[*CombatAttackDistanceEvent]
	canAttack              []hookEntry[*CanAttackEvent]

	// Spell hooks (see spells.go): the spell-disabled and
	// spell-rune-bypass first-answer gates, plus the registered
	// rune sources (e.g. a rune pouch). One list per family, in
	// registration order; sources are consulted in registration
	// order by Check/ConsumeSpellRuneSource.
	spellDisabled    []hookEntry[*SpellDisabledEvent]
	spellRuneBypass  []hookEntry[*SpellRuneBypassEvent]
	spellRuneSources []spellRuneSourceEntry
	spellOnItem      []hookEntry[*SpellOnItemEvent]

	// Shop and trade hooks (see shops.go, trades.go): the
	// can-shop / can-stock-item gates and the can-trade /
	// trade-request / trade-completed families. One list per
	// family, in registration order.
	canShop        []hookEntry[*CanShopEvent]
	canStockItem   []hookEntry[*CanStockItemEvent]
	canTrade       []hookEntry[*CanTradeEvent]
	tradeRequest   []hookEntry[*TradeRequestEvent]
	tradeCompleted []hookEntry[*TradeCompletedEvent]

	// Skills hooks (see skills.go): the level-up, experience-gained,
	// and can-gain-experience lists, plus the installed engine
	// TaskManager (nil until the adapter installs it). One list
	// per family, in registration order.
	playerLevelUp     []hookEntry[*PlayerLevelUpEvent]
	experienceGained  []hookEntry[*ExperienceGainedEvent]
	canGainExperience []hookEntry[*CanGainExperienceEvent]
	taskManager       *game.TaskManager

	// Interface-action hooks (see interfaces.go): ONE list
	// shared by the plain and button-filtered registrations,
	// in registration order (PluginManager.ts:394-396), plus
	// the refuse-while-busy button set
	// (PluginManager.ts:397).
	interfaceActionClick []hookEntry[*InterfaceActionClickEvent]
	busyRefusalButtons   map[int]struct{}

	// Shop currency registry (see shops.go): the installed
	// ShopCurrencyRegistry, plus registrations made before
	// installation (queued in registration order, flushed by
	// SetShopCurrencyRegistry). Nil registry means uninstalled.
	shopCurrencyRegistry ShopCurrencyRegistry
	queuedShopCurrencies []shopCurrencyRegistration

	// Shop core surface (see shops.go): the installed
	// ShopManagerAPI and ShopDefinitionRegistry. Nil means
	// uninstalled.
	shopManagerAPI         ShopManagerAPI
	shopDefinitionRegistry ShopDefinitionRegistry

	// Trade core surface (see trades.go): the installed
	// TradeStateReader. Nil means uninstalled.
	tradeStateReader TradeStateReader

	// NPC combat method providers (see combat.go): the
	// installed provider registry, plus providers registered
	// before installation (queued in registration order,
	// flushed by SetNpcCombatMethodProviderRegistry). Nil
	// registry means uninstalled.
	combatMethodProviderRegistry NpcCombatMethodProviderRegistry
	queuedCombatMethodProviders  []npcCombatMethodProvider

	// Area registry (see areas.go): the installed
	// AreaRegistry, plus areas registered before installation
	// (queued in registration order, flushed by
	// SetAreaRegistry). Nil registry means uninstalled.
	areaRegistry AreaRegistry
	queuedAreas  []*game.Area

	// Run-energy modifier registry (see equipment.go): the
	// installed RunEnergyModifierRegistry, plus modifiers
	// registered before installation (queued in registration
	// order within each family, flushed by
	// SetRunEnergyModifierRegistry). Nil registry means
	// uninstalled.
	runEnergyModifierRegistry       RunEnergyModifierRegistry
	queuedRunEnergyRestoreModifiers []game.RunEnergyRestoreModifier
	queuedRunEnergyDrainModifiers   []game.RunEnergyDrainModifier

	// Object definition / item resolvers and the installed
	// ObjectManagerAPI (see objects.go): installed by the
	// coordinator at boot, read by the object emits. Nil means
	// uninstalled.
	objectDefResolver ObjectDefinitionResolver
	itemResolverFn    ItemInfoResolver
	objectManagerAPI  ObjectManagerAPI

	// prompts is the installed MultiChatboxPrompt registry (see
	// dialogue.go): installed by the coordinator at boot, read
	// by Api.SendMultiChatboxPrompt. Nil means uninstalled.
	prompts *game.MultiChatboxPrompt

	// World-entity hooks (see sailing.go): the
	// world-entity-interaction list, in registration order,
	// plus the installed Sailing runtime Core.Sailing reads
	// (nil until the coordinator installs it).
	worldEntityInteraction []hookEntry[*WorldEntityInteractionEvent]
	sailing                *game.Sailing

	// sealed reports whether Load() has completed (set at the end of
	// emitPluginsLoaded, which Load() calls). Once sealed, further
	// registrations are ignored with a warning (see Api.canRegister).
	sealed bool

	// core is the hub's one shared Core instance, created lazily.
	core *Core

	// commands is the hub's one shared command registry, created
	// lazily (see sharedCommands): the Go form of the
	// PluginManager command statics (PluginManager.ts:404-408).
	// Api.RegisterCommand writes into it; the coordinator reads
	// it out through Hub.Commands to install the dispatch seams
	// and hand it to the chat listener path.
	commands *game.CommandRegistry

	// loginExtension is the single login-extension slot (first-wins;
	// see login.go).
	loginExt *loginExtensionEntry
}

// newHookRegistry returns an empty registry. NewHub (loader-owned) calls it.
func newHookRegistry() *hookRegistry {
	return &hookRegistry{
		custom:             make(map[string][]hookEntry[any]),
		objectByID:         make(map[string][]objectInteractionHook),
		objectByName:       make(map[string][]objectInteractionHook),
		activeZones:        make(map[*game.Player]map[int]struct{}),
		busyRefusalButtons: make(map[int]struct{}),
	}
}

// objectInteractionHook is one object-interaction hook plus its
// registration order: the Go form of ObjectInteractionHook
// (PluginManager.ts:204). Generic, id, and name hooks share one
// order counter so the emit can merge them in original
// registration priority (PluginManager.ts:1201-1202).
type objectInteractionHook struct {
	entry hookEntry[*ObjectInteractionEvent]
	order int
}

func (r *hookRegistry) addServerStartup(e hookEntry[*ServerLifecycleEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serverStartup = append(r.serverStartup, e)
}

func (r *hookRegistry) snapshotServerStartup() []hookEntry[*ServerLifecycleEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ServerLifecycleEvent](nil), r.serverStartup...)
}

func (r *hookRegistry) addServerShutdown(e hookEntry[*ServerLifecycleEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.serverShutdown = append(r.serverShutdown, e)
}

func (r *hookRegistry) snapshotServerShutdown() []hookEntry[*ServerLifecycleEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ServerLifecycleEvent](nil), r.serverShutdown...)
}

func (r *hookRegistry) addPlayerLogin(e hookEntry[*PlayerLoginEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerLogin = append(r.playerLogin, e)
}

func (r *hookRegistry) snapshotPlayerLogin() []hookEntry[*PlayerLoginEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerLoginEvent](nil), r.playerLogin...)
}

func (r *hookRegistry) addPlayerLogout(e hookEntry[*PlayerLogoutEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerLogout = append(r.playerLogout, e)
}

func (r *hookRegistry) snapshotPlayerLogout() []hookEntry[*PlayerLogoutEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerLogoutEvent](nil), r.playerLogout...)
}

func (r *hookRegistry) addPlayerDisconnect(e hookEntry[*PlayerDisconnectEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerDisconnect = append(r.playerDisconnect, e)
}

func (r *hookRegistry) snapshotPlayerDisconnect() []hookEntry[*PlayerDisconnectEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerDisconnectEvent](nil), r.playerDisconnect...)
}

func (r *hookRegistry) addPlayerProcess(e hookEntry[*PlayerProcessEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.playerProcess = append(r.playerProcess, e)
}

func (r *hookRegistry) snapshotPlayerProcess() []hookEntry[*PlayerProcessEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PlayerProcessEvent](nil), r.playerProcess...)
}

func (r *hookRegistry) addCommandHook(e hookEntry[*game.CommandEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commandHooks = append(r.commandHooks, e)
}

func (r *hookRegistry) snapshotCommandHooks() []hookEntry[*game.CommandEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*game.CommandEvent](nil), r.commandHooks...)
}

func (r *hookRegistry) addSocialPacket(e hookEntry[*SocialPacketEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.socialPacket = append(r.socialPacket, e)
}

func (r *hookRegistry) snapshotSocialPacket() []hookEntry[*SocialPacketEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*SocialPacketEvent](nil), r.socialPacket...)
}

func (r *hookRegistry) addCustomEvent(name string, e hookEntry[any]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.custom[name] = append(r.custom[name], e)
}

func (r *hookRegistry) addObjectRoute(e hookEntry[*ObjectRouteEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objectRoute = append(r.objectRoute, e)
}

func (r *hookRegistry) snapshotObjectRoute() []hookEntry[*ObjectRouteEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ObjectRouteEvent](nil), r.objectRoute...)
}

func (r *hookRegistry) addBeforeObject(e hookEntry[*ObjectInteractionEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.beforeObject = append(r.beforeObject, e)
}

func (r *hookRegistry) snapshotBeforeObject() []hookEntry[*ObjectInteractionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ObjectInteractionEvent](nil), r.beforeObject...)
}

// takeObjectOrder returns the next object-interaction
// registration order and advances the counter
// (PluginManager.nextObjectHookOrder, PluginManager.ts:297).
// Every generic, id, and name registration takes exactly one.
func (r *hookRegistry) takeObjectOrder() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	order := r.nextObjectOrder
	r.nextObjectOrder++
	return order
}

// objectHookCount returns how many object-interaction
// registrations have been taken: the Go form of the
// nextObjectHookOrder === 0 check in emitObjectInteraction
// (PluginManager.ts:1176).
func (r *hookRegistry) objectHookCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.nextObjectOrder
}

func (r *hookRegistry) addObjectGeneric(e objectInteractionHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objectGeneric = append(r.objectGeneric, e)
}

func (r *hookRegistry) addObjectByID(key string, e objectInteractionHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.objectByID == nil {
		r.objectByID = make(map[string][]objectInteractionHook)
	}
	r.objectByID[key] = append(r.objectByID[key], e)
}

func (r *hookRegistry) addObjectByName(name string, e objectInteractionHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.objectByName == nil {
		r.objectByName = make(map[string][]objectInteractionHook)
	}
	r.objectByName[name] = append(r.objectByName[name], e)
}

// snapshotObjectInteraction returns the merged generic + id +
// name hooks for one interaction, in registration order
// (PluginManager.ts:1194-1202). The name list is consulted
// only when hasName is true: with no definition, TS looks up
// objectHooksByName with undefined and matches nothing, and
// Go must not match hooks registered under the empty name.
func (r *hookRegistry) snapshotObjectInteraction(objectID, clickType int, name string, hasName bool) []objectInteractionHook {
	r.mu.RLock()
	defer r.mu.RUnlock()
	merged := append([]objectInteractionHook(nil), r.objectGeneric...)
	merged = append(merged, r.objectByID[objectHookKey(objectID, clickType)]...)
	if hasName {
		merged = append(merged, r.objectByName[name]...)
	}
	sortObjectHooks(merged)
	return merged
}

func (r *hookRegistry) addItemAction(e hookEntry[*PluginItemActionEvent], fallback bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if fallback {
		r.itemActionFallbk = append(r.itemActionFallbk, e)
		return
	}
	r.itemAction = append(r.itemAction, e)
}

func (r *hookRegistry) snapshotItemAction(fallback bool) []hookEntry[*PluginItemActionEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if fallback {
		return append([]hookEntry[*PluginItemActionEvent](nil), r.itemActionFallbk...)
	}
	return append([]hookEntry[*PluginItemActionEvent](nil), r.itemAction...)
}

func (r *hookRegistry) addCanUseItem(e hookEntry[*CanUseItemEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canUseItem = append(r.canUseItem, e)
}

func (r *hookRegistry) snapshotCanUseItem() []hookEntry[*CanUseItemEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanUseItemEvent](nil), r.canUseItem...)
}

func (r *hookRegistry) setItemActionsResolver(fn ItemActionsResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.itemActionsFn = fn
}

func (r *hookRegistry) itemActionsResolver() ItemActionsResolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.itemActionsFn
}

func (r *hookRegistry) addItemOnObject(e hookEntry[*ItemOnObjectEvent], fallback bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if fallback {
		r.itemOnObjectFallbk = append(r.itemOnObjectFallbk, e)
		return
	}
	r.itemOnObject = append(r.itemOnObject, e)
}

func (r *hookRegistry) snapshotItemOnObject(fallback bool) []hookEntry[*ItemOnObjectEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if fallback {
		return append([]hookEntry[*ItemOnObjectEvent](nil), r.itemOnObjectFallbk...)
	}
	return append([]hookEntry[*ItemOnObjectEvent](nil), r.itemOnObject...)
}

func (r *hookRegistry) addItemOnItem(e hookEntry[*PluginItemOnItemEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.itemOnItem = append(r.itemOnItem, e)
}

func (r *hookRegistry) snapshotItemOnItem() []hookEntry[*PluginItemOnItemEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*PluginItemOnItemEvent](nil), r.itemOnItem...)
}

func (r *hookRegistry) addNpcDialogueVariant(e dialogueHook[*NpcDialogueContext, *NpcDialogueVariantChoice]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcDialogueVariant = append(r.npcDialogueVariant, e)
}

func (r *hookRegistry) snapshotNpcDialogueVariant() []dialogueHook[*NpcDialogueContext, *NpcDialogueVariantChoice] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]dialogueHook[*NpcDialogueContext, *NpcDialogueVariantChoice](nil), r.npcDialogueVariant...)
}

func (r *hookRegistry) addNpcDialogueCondition(e dialogueHook[*NpcDialogueConditionEvent, *bool]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.npcDialogueCondition = append(r.npcDialogueCondition, e)
}

func (r *hookRegistry) snapshotNpcDialogueCondition() []dialogueHook[*NpcDialogueConditionEvent, *bool] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]dialogueHook[*NpcDialogueConditionEvent, *bool](nil), r.npcDialogueCondition...)
}

func (r *hookRegistry) addZoneHook(e zoneHook) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.zoneHooks = append(r.zoneHooks, e)
}

// forgetZoneState discards one player's zone active set: the explicit
// cleanup Go needs in place of the TS WeakMap's automatic collection
// (see zones.go). Deleting a missing entry is a no-op.
func (r *hookRegistry) forgetZoneState(player *game.Player) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.activeZones, player)
}

// areaRegistrationTarget returns the installed AreaRegistry for area
// to be pushed onto, or queues area (in registration order) and
// returns nil when no registry is installed yet. The caller pushes
// onto a returned registry outside the lock.
func (r *hookRegistry) areaRegistrationTarget(area *game.Area) AreaRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.areaRegistry != nil {
		return r.areaRegistry
	}
	r.queuedAreas = append(r.queuedAreas, area)
	return nil
}

// installAreaRegistry installs reg and returns the areas queued
// before installation, in registration order, for the caller to
// push outside the lock. Installing nil clears the registry but
// keeps the queue: a later non-nil install still flushes it, so
// no area is lost to an install/clear cycle.
func (r *hookRegistry) installAreaRegistry(reg AreaRegistry) []*game.Area {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.areaRegistry = reg
	if reg == nil {
		return nil
	}
	queued := r.queuedAreas
	r.queuedAreas = nil
	return queued
}

func (r *hookRegistry) installedAreaRegistry() AreaRegistry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.areaRegistry
}

func (r *hookRegistry) setObjectDefinitionResolver(fn ObjectDefinitionResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objectDefResolver = fn
}

func (r *hookRegistry) objectDefinitionResolver() ObjectDefinitionResolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.objectDefResolver
}

func (r *hookRegistry) setItemResolver(fn ItemInfoResolver) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.itemResolverFn = fn
}

func (r *hookRegistry) itemResolver() ItemInfoResolver {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.itemResolverFn
}

func (r *hookRegistry) setObjectManager(api ObjectManagerAPI) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.objectManagerAPI = api
}

func (r *hookRegistry) objectManager() ObjectManagerAPI {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.objectManagerAPI
}

func (r *hookRegistry) setMultiChatboxPrompt(registry *game.MultiChatboxPrompt) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prompts = registry
}

func (r *hookRegistry) multiChatboxPrompt() *game.MultiChatboxPrompt {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.prompts
}

func (r *hookRegistry) snapshotCustomEvent(name string) []hookEntry[any] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[any](nil), r.custom[name]...)
}

func (r *hookRegistry) addInterfaceActionClick(e hookEntry[*InterfaceActionClickEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.interfaceActionClick = append(r.interfaceActionClick, e)
}

func (r *hookRegistry) snapshotInterfaceActionClick() []hookEntry[*InterfaceActionClickEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*InterfaceActionClickEvent](nil), r.interfaceActionClick...)
}

func (r *hookRegistry) addBusyRefusal(buttonID int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.busyRefusalButtons == nil {
		r.busyRefusalButtons = make(map[int]struct{})
	}
	r.busyRefusalButtons[buttonID] = struct{}{}
}

func (r *hookRegistry) refusesWhileBusy(buttonID int) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.busyRefusalButtons[buttonID]
	return ok
}

// isSealed reports whether the registry sealed at the end of Load().
func (r *hookRegistry) isSealed() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.sealed
}

// seal marks Load() complete. It is called once, at the end of
// emitPluginsLoaded, after the plugins:loaded handlers have run — so a
// plugins:loaded handler can still register, matching TS where registration
// during the load passes is allowed.
func (r *hookRegistry) seal() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sealed = true
}

// sharedCore returns the hub's one shared Core instance, creating it on
// first use.
func (r *hookRegistry) sharedCore(h *Hub) *Core {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.core == nil {
		r.core = &Core{hub: h}
	}
	return r.core
}

// sharedCommands returns the hub's one shared command registry,
// creating it on first use. Registrations during Load and reads
// after Load all share this instance.
func (r *hookRegistry) sharedCommands() *game.CommandRegistry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.commands == nil {
		r.commands = game.NewCommandRegistry()
	}
	return r.commands
}

// executeHook runs one handler with error isolation, mirroring
// PluginManager.executeHook (PluginManager.ts:429-466) without the perf
// branch: a panicking handler is recovered, logged as
// "[plugins] <label> hook failed (<pluginName>)" plus the recovered value,
// and the emit continues with the next hook. A nil logger falls back to
// log.Default(). PluginContext enter/exit has no Go equivalent (no
// thread-locals); the plugin name in the log line carries the same
// attribution.
func executeHook[T any](logger *log.Logger, pluginName string, label string, handler func(T), event T) {
	defer func() {
		if r := recover(); r != nil {
			if logger == nil {
				logger = log.Default()
			}
			logger.Printf("[plugins] %s hook failed (%s): %v", label, pluginName, r)
		}
	}()
	handler(event)
}

// emitHooks runs hooks in registration order through executeHook.
func emitHooks[T any](logger *log.Logger, label string, hooks []hookEntry[T], event T) {
	for _, hook := range hooks {
		executeHook(logger, hook.pluginName, label, hook.handler, event)
	}
}

// ServerLifecycleEvent is the payload of the server startup/shutdown hooks.
// It mirrors PluginServerLifecycleEvent (PluginTypes.ts:81-83): a single
// timestamp. Timestamp is milliseconds since the Unix epoch, as TS
// Date.now() produces at the call sites (GameBuilder.ts:35, Server.ts:199);
// the adapter supplies time.Now().UnixMilli().
type ServerLifecycleEvent struct {
	Timestamp int64
}

// PlayerLoginEvent is the payload of the player-login hooks. It mirrors
// PluginPlayerLoginEvent (PluginTypes.ts:47-55): the player, the username,
// and the two optional fields, each with a Go source at the enterWorld call
// site (NetworkBuilder.ts:1694-1700). IsNewAccount is true only when login
// created a player because no saved account exists for the profile
// (pending.save == null in TS). Profile is the character profile
// ("main", "league", or "hardcore"); "" means unset, matching Player.Profile.
type PlayerLoginEvent struct {
	Player *game.Player
	// Username is the player's username (player.getUsername() in TS). It
	// duplicates Player.Username, as in TS; the adapter must supply it —
	// EmitPlayerLogin drops events with an empty Username, mirroring the
	// onPlayerLogin registration guard (PluginManager.ts:3201-3212).
	Username string
	// IsNewAccount is true only when no save existed for this profile.
	IsNewAccount bool
	// Profile is the character profile chosen at login.
	Profile string
}

// PlayerLogoutEvent is the payload of the player-logout hooks. It mirrors
// PluginPlayerLogoutEvent (PluginTypes.ts:62-65). The TS call site is the
// player logout path (Player.ts:752-755), which runs before the logout
// save; the Go adapter must supply Player and Username (see PlayerLoginEvent
// for the Username rule — the onPlayerLogout guard is at
// PluginManager.ts:3229-3240).
type PlayerLogoutEvent struct {
	Player   *game.Player
	Username string
}

// PlayerDisconnectEvent is the payload of the player-disconnect hooks. It
// mirrors PluginPlayerDisconnectEvent (PluginTypes.ts:56-60). The TS call
// site is the queue_remove_players phase (World.ts:1260-1264), which fires
// only for non-bot registered players with source
// getDisconnectSource() ?? "removed"; the Go World.Hooks.OnPlayerDisconnect
// hook already applies the same gate and default, so the coordinator wires
// EmitPlayerDisconnect behind it. Source carries the disconnect reason
// ("logout", "removed", ...); the onPlayerDisconnect guard
// (PluginManager.ts:3215-3226) requires only player and username.
type PlayerDisconnectEvent struct {
	Player   *game.Player
	Username string
	Source   string
}

// PlayerProcessEvent is the payload of the per-player process hooks. It
// mirrors PluginPlayerProcessEvent (PluginTypes.ts:90-92): just the player.
// There is deliberately no cycle field — TS carries none; a handler that
// needs the tick reads Core.Cycle(). The TS call site runs after
// player.process() (and ShopManager.processPlayer, which does not exist in
// Go) for non-bot players only (World.ts:1365-1376). EmitPlayerProcess
// drives the zone hooks first (updatePlayerZones, PluginManager.ts:882),
// before the process hooks run — see zones.go.
type PlayerProcessEvent struct {
	Player *game.Player
}

// EmitServerStartup runs the server-startup hooks in registration order.
// The coordinator calls it after the world is built and the engine starts,
// mirroring GameBuilder.ts:35. A nil event runs no hooks (a Go-only guard;
// TS has no nil-event equivalent, and inventing a zero timestamp would fake
// a field). A nil Hub or a Hub without a registry is a no-op.
func (h *Hub) EmitServerStartup(event *ServerLifecycleEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "server_startup", h.hooks.snapshotServerStartup(), event)
}

// EmitServerShutdown runs the server-shutdown hooks in registration order.
// The coordinator calls it on graceful shutdown, mirroring Server.ts:199.
// Nil-event and nil-Hub handling match EmitServerStartup.
func (h *Hub) EmitServerShutdown(event *ServerLifecycleEvent) {
	if h == nil || h.hooks == nil || event == nil {
		return
	}
	emitHooks(h.Logger(), "server_shutdown", h.hooks.snapshotServerShutdown(), event)
}

// EmitPlayerLogin runs the player-login hooks in registration order,
// mirroring emitPlayerLogin (PluginManager.ts:800-810) with label "login".
// Events with a nil Player or an empty Username run no hooks, mirroring the
// onPlayerLogin registration guard (PluginManager.ts:3201-3212) at emit
// level — the outcome is identical (no handler runs) and the check is made
// once instead of per hook. The adapter must supply Username,
// IsNewAccount, and Profile; see PlayerLoginEvent.
func (h *Hub) EmitPlayerLogin(event *PlayerLoginEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Username == "" {
		return
	}
	emitHooks(h.Logger(), "login", h.hooks.snapshotPlayerLogin(), event)
}

// EmitPlayerLogout runs the player-logout hooks in registration order,
// mirroring emitPlayerLogout (PluginManager.ts:827-835) with label "logout".
// The nil-Player/empty-Username guard mirrors the onPlayerLogout
// registration guard (PluginManager.ts:3229-3240); see EmitPlayerLogin.
func (h *Hub) EmitPlayerLogout(event *PlayerLogoutEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Username == "" {
		return
	}
	emitHooks(h.Logger(), "logout", h.hooks.snapshotPlayerLogout(), event)
}

// EmitPlayerDisconnect runs the player-disconnect hooks in registration
// order, mirroring emitPlayerDisconnect (PluginManager.ts:818-826) with
// label "disconnect". The nil-Player/empty-Username guard mirrors the
// onPlayerDisconnect registration guard (PluginManager.ts:3215-3226); see
// EmitPlayerLogin. Source is not guarded: TS requires the field but never
// checks its value. The player's zone active set is discarded first (see
// ForgetPlayerZones): the removal phase is the moment the player object
// leaves the world, and no exit hooks fire on disconnect — TS lets the
// WeakMap entry die silently, and so does this.
func (h *Hub) EmitPlayerDisconnect(event *PlayerDisconnectEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Username == "" {
		return
	}
	h.ForgetPlayerZones(event.Player)
	emitHooks(h.Logger(), "disconnect", h.hooks.snapshotPlayerDisconnect(), event)
}

// EmitPlayerProcess runs the per-player process hooks in registration
// order, mirroring emitPlayerProcess (PluginManager.ts:881-889) with label
// "player_process". updatePlayerZones runs FIRST, before the process hooks
// — and even when there are zero process hooks — exactly as TS calls it at
// the top of emitPlayerProcess (PluginManager.ts:882). Events with a nil
// Player run no hooks and no zone update, mirroring the onPlayerProcess
// registration guard (PluginManager.ts:3297-3310) at emit level. The
// coordinator calls it once per non-bot registered player per tick, after
// Player.Process.
func (h *Hub) EmitPlayerProcess(event *PlayerProcessEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil {
		return
	}
	h.updatePlayerZones(event.Player)
	emitHooks(h.Logger(), "player_process", h.hooks.snapshotPlayerProcess(), event)
}

// Social packet kinds: the `type` discriminant of the TS
// PluginSocialPacketEvent packet union (PluginTypes.ts:67-79).
// The kind type and constants live in the game package with the
// packet struct (game.SocialPacketKind, contract C1 of the
// Phase 19 plan); this package keeps only these aliases, so
// existing content and adapter code keeps compiling against the
// plugins names.
const (
	// SocialPacketPublicChat is the public_chat packet kind,
	// emitted by the chat listener (ChatPacketListener.ts:38-43).
	SocialPacketPublicChat = game.SocialPacketPublicChat
	// SocialPacketClanChat is the clan_chat packet kind,
	// emitted by the social dispatch (NetworkBuilder.ts:905-911).
	SocialPacketClanChat = game.SocialPacketClanChat
	// SocialPacketClanChannelAction is the clan_channel_action
	// packet kind, emitted by the social dispatch
	// (NetworkBuilder.ts:905-911).
	SocialPacketClanChannelAction = game.SocialPacketClanChannelAction
	// SocialPacketFriendsChatAction is the friends_chat_action
	// packet kind, emitted by the social dispatch
	// (NetworkBuilder.ts:905-911).
	SocialPacketFriendsChatAction = game.SocialPacketFriendsChatAction
	// SocialPacketPrivateMessage is the private_message packet
	// kind, emitted by the social dispatch
	// (NetworkBuilder.ts:905-911).
	SocialPacketPrivateMessage = game.SocialPacketPrivateMessage
	// SocialPacketChatFilter is the chat_filter packet kind,
	// emitted by the social dispatch (NetworkBuilder.ts:905-911).
	SocialPacketChatFilter = game.SocialPacketChatFilter
	// SocialPacketReportAbuse is the report_abuse packet kind,
	// emitted by the social dispatch (NetworkBuilder.ts:905-911).
	SocialPacketReportAbuse = game.SocialPacketReportAbuse
	// SocialPacketChat is the chat packet kind: only the
	// messageType "friends_chat" variant reaches the hub
	// (NetworkBuilder.ts:894-903); public chat arrives as
	// public_chat, and game chat takes no action.
	SocialPacketChat = game.SocialPacketChat
)

// SocialPacketEvent is the payload of the social-packet hooks. It
// mirrors PluginSocialPacketEvent (PluginTypes.ts:67-79): the
// player, the handled flag, and the packet. A hook that marks the
// event handled suppresses the engine's default handling of the
// packet (for public_chat, the broadcast; for the dispatch kinds
// the emit IS the handling — the dispatch takes no further action
// and discards the return value, NetworkBuilder.ts:894-911 — so
// handled stops only later hooks). Packet is a game.SocialPacket
// value, not a pointer: TS requires the field, and the
// registration guard's !packet check degenerates to
// always-present in Go (documented at Api.OnSocialPacket). The
// packet is the flattened single struct of contract C1: one
// shape for all eight union variants, with Kind selecting which
// fields carry meaning:
//
//   - public_chat: Text, AutoChat.
//   - clan_chat: ClanType, Text.
//   - clan_channel_action: Action ("kick", "ban", or "mute"),
//     Name, Value.
//   - friends_chat_action: Action (the FriendsChatAction
//     discriminant — join, leave, kick, add_friend,
//     remove_friend, set_friend_rank, add_ignore,
//     remove_ignore; ClientProtocol.ts:276-284), Name, and
//     Rank for set_friend_rank.
//   - private_message: Recipient, Text.
//   - chat_filter: PublicMode, PrivateMode, TradeMode.
//   - report_abuse: Name, RuleID, Mute.
//   - chat: Text, MessageType ("friends_chat").
//
// Fields a kind does not use stay at their zero values; the hub
// never validates the kind or the field combination, because
// the TS emit does not (PluginManager.ts:836-843).
type SocialPacketEvent struct {
	Player  *game.Player
	Packet  game.SocialPacket
	Handled bool
}

// EmitSocialPacket runs the social-packet hooks in registration
// order until one marks the event handled, mirroring
// emitSocialPacket (PluginManager.ts:836-843) with label
// "social_packet", through the runHandledGate helper (gates.go).
// It returns whether the event ended handled. The entry guards
// mirror the TS early return: a nil event, a nil player, or an
// already-handled event runs no hooks and returns FALSE —
// including the pre-handled case, which differs from the bare
// gate loop and is reproduced here, not in the helper (see
// gates.go). A nil Hub or a Hub without a registry returns
// false.
//
// The production call sites are the chat listener's (public
// chat emits {type: "public_chat"} before broadcasting, and an
// intercepted event suppresses the broadcast,
// ChatPacketListener.ts:38-43) and the social dispatch: the
// listeners route every other social packet kind — clan_chat,
// clan_channel_action, friends_chat_action, private_message,
// chat_filter, report_abuse, and chat with messageType
// friends_chat — to this emit and take no further action
// (NetworkBuilder.ts:894-911), through the cmd/server adapter.
// All eight kinds have dispatch producers; the hub itself
// validates nothing about the kind.
func (h *Hub) EmitSocialPacket(event *SocialPacketEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Handled {
		return false
	}
	return runHandledGate(h.Logger(), "social_packet", h.hooks.snapshotSocialPacket(), event,
		func(e *SocialPacketEvent) bool { return e.Handled })
}
