// Package definition loads the JSON definition data TSPS reads from
// data/definitions/ at startup and layers it over the cache
// definitions, following GameBuilder.loadStartupData()
// (game/GameBuilder.ts:38-55): NpcInteraction, InterfaceLayout,
// ObjectSpawn, GroundItemSpawn, NpcDefinition, NpcSpawn, Shop.
//
// Upstream sources (paths relative to server/ in RSPSApp/tsps, under
// src/main/typescript/elvarg/game/definition unless noted):
//   - loader/DefinitionLoader.ts (source registry semantics)
//   - loader/impl/NpcInteractionDefinitionLoader.ts
//   - loader/impl/InterfaceLayoutDefinitionLoader.ts
//   - loader/impl/ObjectSpawnDefinitionLoader.ts
//   - loader/impl/GroundItemSpawnDefinitionLoader.ts
//   - loader/impl/NpcDefinitionLoader.ts
//   - loader/impl/NpcSpawnDefinitionLoader.ts
//   - loader/impl/ShopDefinitionLoader.ts
//   - ItemDefinition.ts, NpcDefinition.ts, ObjectDefinition.ts,
//     ShopDefinition.ts, NpcSpawnDefinition.ts,
//     ObjectSpawnDefinition.ts, DefaultSpawnDefinition.ts,
//     NpcInteractionDefinition.ts, InterfaceLayoutDefinition.ts,
//     EquipmentSounds.ts, NpcCombatSounds.ts, WorldConfigStore.ts
//   - plugins/items/ItemDefinitionLoader.plugin.js (item-gameplay /
//     prices / alch / overrides application onto ItemDefinition;
//     the custom-items portion is NOT ported here — see below)
//   - plugins/npcs/NpcSpawns.plugin.js (core npc-spawns registration)
//   - plugins/npcs/NpcDrops.plugin.js, SeedCategories.NpcDrops.js
//     (NPC drop tables, shared subtables, overrides, Slayer-key
//     strip — the loader in npcdrops.go; the roller and death
//     integration live in internal/content/npcdrops/)
//
// Load reads from a definitions directory at runtime (like
// world.json); no definition data is bundled, and tests use small
// hand-built fixtures only.
//
// Required vs optional files. Each loader reproduces the
// TypeScript behavior exactly as the TS code does it — a missing or
// malformed REQUIRED file fails Load, a missing OPTIONAL file warns
// and continues, and a malformed optional file that exists fails
// Load (JSON.parse throws in TS too):
//
//   - interface-layouts.json: REQUIRED (InterfaceLayout loader
//     readFileSync/JSON.parse throw).
//   - object-spawns.json: REQUIRED (ObjectSpawn loader throws;
//     entries are validated and the first invalid entry fails).
//   - ground-items.json: REQUIRED (GroundItemSpawn loader
//     readCoreDefinitions throws; invalid entries are skipped).
//   - monsters-complete.json: OPTIONAL when missing (warn, empty);
//     malformed when present fails. (NpcDefinition readOptional.)
//   - npc-status-immunities.json: OPTIONAL when missing; malformed
//     fails; schema_version != 1 fails.
//   - npc-combat-defs.json: REQUIRED. npc-animations.json: REQUIRED.
//   - npc-captured-animations.json: OPTIONAL when missing;
//     malformed when present fails.
//   - npc-spawns.json: loaded through the source registry (see
//     below); a missing or malformed file is a counted source
//     failure, not a Load error — zero NPC spawns, boot continues.
//   - shops.json and custom-shops.json: BOTH REQUIRED (Shop loader
//     readCoreDefinitions reads both with readFileSync).
//   - npc-drops.json: REQUIRED (NpcDrops loader readJSONValue
//     throws; must hold an object with tables and npcs sections).
//     npc-drop-subtables.json and npc-drop-overrides.json:
//     OPTIONAL when missing (TS readJson returns null, treated as
//     {}); malformed when present fails.
//   - item-gameplay.json, item-prices.json, item-alch.json,
//     item-overrides.json: OPTIONAL when missing (a deliberate
//     deviation for item-gameplay.json, which the TS plugin
//     requires — see item.go); malformed when present fails, except
//     item-overrides.json whose malformed JSON also fails.
//   - npc-examines.json, object-examines.json,
//     object-examine-overrides.json, equipment-sounds.json,
//     npc-combat-sounds.json: OPTIONAL when missing (TS reads them
//     lazily on first use and throws then; this package reads them
//     eagerly at Load and falls back to the generic text/defaults
//     when absent — see the per-file comments).
//
// Source registry. TypeScript keeps a global static registry
// (DefinitionLoader.sourcesByDefinition) fed by plugins. This
// package uses an explicit Registry value owned by the Store
// (LoadOptions.Sources, or a fresh one): same register/ordering/
// failure-counting semantics, no global mutable state. Load itself
// registers the core npc-spawns file source, because in TS that
// registration is done by the NpcSpawns plugin
// (plugins/npcs/NpcSpawns.plugin.js:16-21), and there is no plugin
// system here to do it. The core source's owner/name is "NpcSpawns"
// exactly as in TS, so loaded spawns carry Source "NpcSpawns" —
// "core" is only the NpcSpawnDefinition constructor default, which
// the TS loader never produces because it always passes the source
// name.
//
// No-cache degradation. TypeScript has NO no-cache path:
// CacheDefinitions.getState() throws when the cache pipeline never
// initialized (CachePipeline.getStore() throws). The Go --no-cache
// boot is a Go-only mode, so with a nil Cache this package degrades
// gracefully instead of throwing: JSON loading still runs, spawn
// entries are kept (the cache-backed "unsupported" filters are
// skipped — otherwise a --no-cache boot could never show spawns),
// name-based lookups resolve nothing, and the wrappers return
// their TS DEFAULT shapes. Every nil-cache behavior is commented
// at its site.
//
// Explicit gaps (NOT STARTED, never silent):
//   - Dialogue data loading (npc-dialogues.json,
//     npc-dialogue-index.json, event-dialogues.json,
//     dialogue-types.json): no ported loader reads them. The NPC
//     interaction loader still produces core.dialogues.start
//     actions for dialogueId shorthand, as TS does; the dialogue
//     data itself is not loaded.
//   - ShopManager.initialize() and Sailing.initialize() (called
//     after the loaders in loadStartupData): not ported; the shop
//     and sailing systems do not exist in Go.
//   - NpcAnimationScanner.preloadSequences,
//     CacheDefinitions.preloadEnums, CombatPoisonData.init
//     (also in loadStartupData): not ported here. preloadEnums
//     exists on the cache facade; the other two have no Go
//     counterpart.
//   - Custom-definition overlays (custom-definitions.json,
//     custom-items.json, ItemDefinition.registerCustom,
//     NpcDefinitionLoader.applyCustomCombat): owned by the overlay
//     work; this package reads neither file and applies no custom
//     content. Shop custom-shops.json IS read, because the TS Shop
//     loader reads it as a core file (it is shop data, not a cache
//     overlay).
//   - Plugin vetoes: emitCanStockItem fires through the
//     CanStockItemHook seam (set by the coordinator from the
//     plugin hub before Load; with none installed, no stock is
//     vetoed). emitCanSpawnNpc has no seam yet, so nothing is
//     spawn-vetoed. Plugin-registered definition sources beyond
//     what the caller registers through the Registry: none.
//   - NpcIdentifiers `key` spawn resolution: the generated
//     identifier tables are not ported, so a spawn entry naming a
//     `key` (one entry in the shipped npc-spawns.json does) cannot
//     resolve and is skipped with a warning, as TS skips an
//     unknown key.
//   - WeaponInterfaces validation for item-gameplay.json
//     weaponInterface names (CombatStyleDefinitions): names are
//     validated against the ported combat style tables
//     (data/definitions/item-combat-styles.json, loaded by
//     game.LoadCombatStyles; unknown fight-type refs error at
//     load, as TS throws).
//   - Files no ported loader or wrapper reads: npc-rivalries.json, npc-spawn-sync.json, dormant-npcs.json,
//     weapon-stats.json, music-areas.json,
//     music-areas-classic.json, bot-*.json, clue-caskets.json,
//     reward-caskets.json, pvp-bot-loadouts.json,
//     sequence-names.json. They are read by plugins, scripts, or
//     unported systems (Music), not by the loaders ported here.
package definition
