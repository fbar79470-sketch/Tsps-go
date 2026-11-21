package plugins

import (
	"log"
	"sync"

	"tsps-go/internal/game"
)

// Core is the scoped core-access surface: what exists in Go today only. One
// shared instance per hub (see Hub.core), mirroring the single frozen
// PluginCoreApi object TS builds once in getCoreApi
// (PluginManager.ts:2606+) — but NOT its breadth. TS PluginCoreApi
// (PluginTypes.ts:1560-1771) is a 191-member dynamic mirror of core classes,
// singletons, and protocol encoders (counted from the interface at commit
// bfd81e3; the getCoreApi literal carries 196 properties, a small superset).
// Mirroring it would mean inventing Go equivalents for systems that do not
// exist. Instead Core exposes exactly eight things, each grounded in
// something real:
//
//   - Hub: the emit surface (Hub.Emit* methods).
//   - Logger: the hub's injected logger.
//   - Config: read access to the hub's plugin configuration.
//   - Cycle: the current process cycle, through an optional seam installed
//     by the adapter (nil = 0).
//   - ObjectManager: the installed object registry surface
//     (ObjectManagerAPI), through an optional seam installed by the
//     adapter (nil = not installed).
//   - ShopManager: the installed shop surface (ShopManagerAPI),
//     through an optional seam installed by the adapter (nil = not
//     installed).
//   - ShopDefinition: the installed shop-definition lookup surface
//     (ShopDefinitionRegistry), through an optional seam installed
//     by the adapter (nil = not installed).
//   - TradeState: one player's current trading state, through an
//     optional reader installed by the adapter (uninstalled = no
//     state to read).
//
// EXTENSION POINTS — a future accessor is added here only when its engine
// system exists in Go, naming the system:
//
//   - World access (TS getWorld / core.World): the internal/game World,
//     via the cmd/server adapter that owns it. Not a Core method until the
//     adapter can supply a *game.World.
//   - TaskManager (TS getTaskManager / core.TaskManager): game.TaskManager,
//     via the World's Tasks(). Same adapter condition.
//   - RegionManager / collision (TS getRegionManager / core.RegionManager,
//     core.PathFinder, core.RsmodRouteFinding): game.RegionManager, via the
//     World's RegionManager(). Same adapter condition.
//   - Definitions (core.CacheDefinitions / core.CacheMaps): internal/cache
//     facades, owned by cmd/server boot wiring. No accessor until the
//     adapter exposes them.
//   - Ground items (TS getItemOnGroundManager / core.ItemOnGroundManager):
//     game.GroundItemManager, owned by the cmd/server adapter. Same
//     condition.
//   - Objects (TS core.ObjectManager / core.MapObjects): ported as
//     Core.ObjectManager (see objects.go). core.MapObjects has no
//     separate accessor: its query surface (find, still-stands)
//     lives on ObjectManagerAPI, so one accessor covers both.
//   - Skills, containers, appearance (core.SkillManager / core.Inventory /
//     core.Equipment / core.Bank / core.Appearance / core.Flag /
//     core.Direction / core.Location / core.Item): already reachable from
//     event payloads (*game.Player methods); no Core accessor is needed.
//   - Music (core.Music): internal/music lengths/areas, owned by the
//     session adapter. No accessor until the adapter exposes them.
//   - Shops & trade (TS core.ShopManager / core.ShopDefinition /
//     core.TradeState): ported as Core.ShopManager,
//     Core.ShopDefinition, and Core.TradeState (see shops.go and
//     trades.go). No Trading class mirror is offered: the trade
//     flow is the engine system, reached through the player.
//   - Protocol encoders (TS core.encodeChatMessage and the five
//     core.encodeClan* encoders, PluginTypes.ts:1653-1658;
//     assessed Phase 19 against the landed Go encoders, NOT
//     ported): five of the six take protocol-defined snapshot
//     structs — EncodeClanSettingsFull takes
//     *protocol.ClanSettingsSnapshot, EncodeClanChannelFull
//     takes *protocol.ClanChannelSnapshot, the delta encoders
//     take []protocol.ClanChannelDeltaEvent /
//     []protocol.ClanSettingsDeltaUpdate, and
//     EncodeFriendsChatSnapshot takes protocol.FriendsChatSnapshot
//     (internal/protocol/social.go; the TS snapshots are
//     ClientProtocol.ts interfaces, ClientProtocol.ts:1261-1294)
//     — so Core methods would force a plugins-to-protocol import,
//     the edge the layering forbids (the game layer must never
//     import protocol, internal/game/message.go; protocol is
//     imported only by listeners and cmd/server), and leak wire
//     structs into content. Only EncodeClanVar has a plain
//     (int, int) signature, and exposing it alone would still
//     need the new import edge for a trivial encoder. There is
//     also no send path: the TS encoders
//     return a Buffer the caller sends with
//     session.sendClientPacket (FriendsChatManager.js:274),
//     while Go content has no raw-byte send on *game.Player and
//     sends chat through Player.SendMessage / SendTypedMessage
//     instead, so a Core encoder returning []byte would be dead
//     surface. No TS content calls the core.encodeClan*
//     encoders (only the getCoreApi wiring,
//     PluginManager.ts:2698-2702, references them), and the
//     snapshot encoder EncodeFriendsChatSnapshot is not a
//     PluginCoreApi member at all (absent from PluginTypes.ts;
//     TS content requires it from ClientProtocol directly),
//     so exposing it would invent a member. The encoders stay
//     engine-side (session senders) until content has both a
//     send path and a game-shaped snapshot surface; no wrapper
//     snapshot types are defined in this package to dodge the
//     layering question.
//   - Everything combat, dialogue, prayer, area, boat, or
//     sailing shaped: no accessor until that system is ported. See the
//     NOT-mirrored list below.
//
// NOT MIRRORED — every PluginCoreApi member with no Go equivalent this
// phase (187 members — ObjectManager, ShopManager, ShopDefinition,
// and TradeState are ported, see above — grouped;
// PluginPerf's nested snapshot/reset/setEnabled
// /isEnabled methods are likewise absent):
//
// World/config head: `CrossFloor`, `WorldConfigCatalog`, `WorldConfigStore`.
//
// Combat methods & formulas: `MeleeCombatMethod`, `RangedCombatMethod`,
// `MagicCombatMethod`, `TeleBlock`, `CombatMethod`, `CombatSpecial`,
// `CombatFactory`, `BonusManager`, `EquipmentRequirements`,
// `WeaponInterfaces`, `Barrows`, `applyPrayerDrainModifiers`, `hitTargetOf`,
// `CombatEquipment`, `CanAttackResponse`, `CombatType`, `SkullType`,
// `CombatConstants`, `DamageFormulas`, `AccuracyFormulasDpsCalc`,
// `PendingHit`, `HitDamage`, `HitMask`, `RangedWeapon`, `RangedWeaponType`,
// `Ammunition`, `WeaponProfiles`, `FightStyle`, `FightType`,
// `WeaponInterfaceManager`.
//
// Prayer & content helpers: `PrayerHandler`, `PrayerData`,
// `ItemActionPacketListener`, `castAlchemy`, `examineItem`, `ChatType`,
// `resolveChatHighlights`, `DuelRule`.
//
// Entities & model: `RegionManager`, `Animation`, `Graphic`,
// `GraphicHeight`, `Priority`, `Skill`, `SkillManager`,
// `Item`, `Flag`, `Appearance`, `Direction`, `Equipment`, `Bank`,
// `Inventory`, `PlayerStatus`, `Task`, `CountdownTask`, `ForceMovement`,
// `ForceMovementTask`, `TimedObjectReplacementTask`, `TaskManager`,
// `NPCRespawnTask`.
// (`Projectile` was in this group; it is ported as
// Core.Projectile in projectiles.go.)
//
// Identifiers & misc: `ItemIdentifiers`, `ItemIds`, `NpcIdentifiers`,
// `ObjectIdentifiers`, `ShopIdentifiers`, `Misc`, `ChatTextLayout`,
// `TimerKey`, `SecondsTimer`, `Location`, `Mobile`.
// (`Sound` and `Sounds` were in this group; they are ported as
// Core.Sound and Core.Sounds in sounds.go.)
//
// Protocol & characters: `encodeCharacterList`, `encodeCharacterResult`,
// `LOGIN_FLAG_CHARACTER_SELECT`, `MAIN_PROFILE`, `LEAGUE_PROFILE`,
// `HARDCORE_PROFILE`, `PLAYER_PROFILES`, `Skills`, `encodePluginMessage`,
// `encodeChatMessage`, `encodeClanSettingsFull`, `encodeClanChannelFull`,
// `encodeClanChannelDelta`, `encodeClanSettingsDelta`, `encodeClanVar`,
// `MOBILE_CLIENT_ATTRIBUTE`, `XPDROPS_ENABLED_ATTRIBUTE`,
// `encodeGameframeBootstrap`, `resolveGameframeRoot`, `orbsGroupFor`,
// `dataOrbsHidden`, `encodeFinePosition`.
//
// Areas, perf & world: `Boundary`, `PolygonalBoundary`, `Area`,
// `ServerPerf`, `PluginPerf`, `World`, `DisplayNames`, `Accounts`.
//
// Boats, objects & ground items: `BoatManager`,
// `BOAT_VIEW_DISTANCE`, `GameObject`, `PrivateArea`,
// `TemplatedInstanceArea`, `OperationType`, `LocModelType`,
// `MapObjects`, `ItemOnGroundManager`, `GroundItemOperationType`,
// `ItemOnGround`, `GroundItemState`, `ItemDefinition`, `EquipPacketListener`,
// `PlayerDeathTask`, `CombatPoisonData`, `CombatPoisonEffect`.
// (`ObjectManager` was in this group; it is ported as
// Core.ObjectManager in objects.go. `MapObjects` stays listed:
// it has no separate accessor — its query surface folds into
// ObjectManagerAPI. `Sailing` and `BoatMoveMode` were in this
// group; they are ported as Core.Sailing and Core.BoatMoveMode
// in sailing.go.)
//
// Cache & maps: `CacheDefinitions`, `encodeContentData`, `CacheMaps`,
// `Wilderness`, `PathFinder`, `RsmodRouteFinding`, `NpcDefinition`,
// `NpcSpawnDefinition`, `ObjectDefinition`.
// (`MapRegionReplacementManager` and `readTemplateRegion` were
// in this group; they are ported as
// Core.MapRegionReplacementManager and Core.ReadTemplateRegion
// in instancing.go.)
//
// Spells & NPCs: `MagicSpellbook`, `ItemAliases`, `Spell`,
// `CombatNormalSpell`, `CombatSpells`, `Autocasting`, `NPC`,
// `NpcSpawnDefinitionLoader`, `GameConstants`, `Music`, `WorldDefinition`,
// `TeleportHandler`, `TeleportType`.
//
// Dialogue: `DialogueChainBuilder`, `NpcDialogue`, `PlayerDialogue`,
// `OptionDialogue`, `StatementDialogue`, `ItemStatementDialogue`,
// `DoubleItemStatementDialogue`, `ActionDialogue`, `EndDialogue`,
// `CreationMenu`.
//
// Server, shops, trade & player control: `PlayerRights`, `Server`,
// `PluginManager`, `CustomInterfaceRegistry`,
// `MultiChatboxPrompt`, `ChatPacketListener`,
// `controlPlayer`, `releasePlayerControl`,
// `endPlayerControl`, `dispatchClientMessages`, `connectHeadlessClient`.
// (`ShopManager`, `ShopDefinition`, and `TradeState` were in this
// group; they are ported as Core.ShopManager, Core.ShopDefinition,
// and Core.TradeState in shops.go / trades.go.)
//
// Cache pipeline, template regions & spellbooks: `CachePipeline`,
// `CacheIndexDat2`, `encodeRebuildRegion`, `encodeLocAddChange`,
// `encodeLocDel`, `templateChunkKey`,
// `rotateChunkLoc`, `rotateChunkTile`, `PrivateChatStatus`,
// `SpellTeleports`, `LunarSpells`, `SpellbookLocks`, `EffectSpells`,
// `ArceuusSpells`, `ArceuusOfferings`.
// (`getTemplateRegion` was in this group; it is ported as
// Core.GetTemplateRegion in instancing.go. `templateChunkKey`,
// `rotateChunkLoc`, and `rotateChunkTile` stay listed: Go
// content calls game.PackTemplateChunk / game.RotateChunkTile /
// game.RotateChunkLoc directly, by the TimerKey precedent.)
type Core struct {
	hub *Hub

	mu sync.RWMutex
	// cycleSource answers the current process cycle, or is nil. It is
	// installed by the adapter via Hub.SetCycleSource; with no source,
	// Cycle returns 0.
	cycleSource func() int64
}

// Hub returns the hub this Core belongs to, or nil for a nil Core.
func (c *Core) Hub() *Hub {
	if c == nil {
		return nil
	}
	return c.hub
}

// Logger returns the hub's injected logger, falling back to log.Default()
// when the Core or hub is nil or the hub has no logger.
func (c *Core) Logger() *log.Logger {
	if c == nil || c.hub == nil {
		return log.Default()
	}
	if logger := c.hub.Logger(); logger != nil {
		return logger
	}
	return log.Default()
}

// Config returns read-only access to the hub's plugin configuration. Keys
// are used exactly as given (full "Namespace:key" keys); unlike
// Api.GetPluginConfig, there is no per-plugin prefixing, because Core is
// shared across plugins and has no owning plugin name.
func (c *Core) Config() CoreConfig {
	if c == nil {
		return CoreConfig{}
	}
	return CoreConfig{hub: c.hub}
}

// Cycle returns the current process cycle as reported by the adapter's
// cycle source, or 0 when no source is installed (or the Core is nil).
// PlayerProcess handlers use it to learn the tick: the
// PlayerProcessEvent itself carries no cycle, because TS carries none.
func (c *Core) Cycle() int64 {
	if c == nil {
		return 0
	}
	c.mu.RLock()
	fn := c.cycleSource
	c.mu.RUnlock()
	if fn == nil {
		return 0
	}
	return fn()
}

// ObjectManager returns the installed object registry surface
// (ObjectManagerAPI): the core.ObjectManager equivalent
// (PluginManager.ts:2738; PluginTypes.ts:1687). It is the same
// instance Api.GetObjectManager returns, installed by the
// coordinator through Hub.SetObjectManager. Nil when no
// surface is installed (or the Core is nil). core.MapObjects
// has no separate accessor: its query surface lives on
// ObjectManagerAPI (see objects.go).
func (c *Core) ObjectManager() ObjectManagerAPI {
	if c == nil || c.hub == nil {
		return nil
	}
	return c.hub.ObjectManager()
}

// ShopManager returns the installed shop surface
// (ShopManagerAPI): the core.ShopManager equivalent
// (PluginManager.ts:2790; PluginTypes.ts:1741), installed by the
// coordinator through Hub.SetShopManager. Nil when no surface
// is installed (or the Core is nil). See shops.go for the
// scoped surface and its exclusions.
func (c *Core) ShopManager() ShopManagerAPI {
	if c == nil || c.hub == nil {
		return nil
	}
	return c.hub.ShopManager()
}

// ShopDefinition returns the installed shop-definition lookup
// surface (ShopDefinitionRegistry): the core.ShopDefinition
// equivalent (PluginManager.ts:2791; PluginTypes.ts:1742),
// installed by the coordinator through
// Hub.SetShopDefinitionRegistry. Nil when no surface is
// installed (or the Core is nil). See shops.go for the scoped
// surface and its exclusions.
func (c *Core) ShopDefinition() ShopDefinitionRegistry {
	if c == nil || c.hub == nil {
		return nil
	}
	return c.hub.ShopDefinitionRegistry()
}

// TradeState returns player's current trading state through the
// installed TradeStateReader: the core.TradeState equivalent
// (PluginManager.ts:2794; PluginTypes.ts:1746) as a read, not a
// class mirror — TS content compares getTrading().getState()
// against the enum, and Go content compares this result
// against the TradeState constants (trades.go). It reports
// false when no reader is installed, the Core is nil, or the
// player has no trading state to read (the reader reports
// false); the returned state is then TradeStateNone and must
// not be read as a real state.
func (c *Core) TradeState(player *game.Player) (TradeState, bool) {
	if c == nil || c.hub == nil {
		return TradeStateNone, false
	}
	reader := c.hub.TradeStateReader()
	if reader == nil {
		return TradeStateNone, false
	}
	return reader(player)
}

// setCycleSource installs (or, with nil, clears) the cycle source.
func (c *Core) setCycleSource(fn func() int64) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.cycleSource = fn
	c.mu.Unlock()
}

// CoreConfig is the read-only plugin-configuration view returned by
// Core.Config. It reads through Hub.GetPluginConfig and never writes.
type CoreConfig struct {
	hub *Hub
}

// Get returns the configured value for the full key, or def when the key is
// unset or empty. Bare keys are NOT prefixed: pass the full
// "Namespace:key" key (per-plugin prefixing is Api.GetPluginConfig's job).
func (c CoreConfig) Get(key string, def any) any {
	if c.hub == nil || key == "" {
		return def
	}
	return c.hub.GetPluginConfig(key, def)
}

// core returns the hub's one shared Core instance, creating it on first use
// in the hook registry. A nil Hub, or a Hub without a registry (only
// possible for a Hub not built by NewHub), yields nil.
//
// Defined here (hooks worker) because the shared instance lives in the hook
// registry. Coordinator: if hub.go defines a field also named `core`,
// rename one side — a Hub field and a Hub method may not share the name.
func (h *Hub) core() *Core {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.sharedCore(h)
}

// SetCycleSource installs the function Core.Cycle reads the current process
// cycle from — typically the World's ProcessCycle. The adapter calls it once
// during boot wiring; calling it with nil clears the source (Cycle then
// returns 0). It is a no-op on a nil Hub or a Hub without a registry. The
// seam itself lives on the shared Core (Core.cycleSource); this method is
// the Hub-level installer the adapter calls, so content never touches Core
// internals.
func (h *Hub) SetCycleSource(fn func() int64) {
	if c := h.core(); c != nil {
		c.setCycleSource(fn)
	}
}
