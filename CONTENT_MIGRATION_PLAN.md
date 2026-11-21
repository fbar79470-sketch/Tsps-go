# Content migration plan

Go-native content migration for the TSPS server-only Go port, per the
decided strategy (2026-11-15): content is ported to Go packages behind
the Go hook registry (`internal/plugins`), piece by piece. No embedded
JS runtime, no Node sidecar. This plan is durable: future phases select
from it, they do not rewrite it without recording why.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only checkout
at `~/tsps-bfd81e3`. Content lives at `server/plugins/`. Hub surface:
`server/src/main/typescript/elvarg/plugins/PluginTypes.ts`
(`PluginApi` lines 999–1553, `PluginCoreApi` lines 1560–1770).
All numbers below were measured 2026-11-14 by the plan author; method
is stated per table. Do not copy numbers from the earlier analysis
without re-measuring.

## 1. Measured inventory

Method: per top-level directory under `server/plugins/` —
`find <dir> -name '*.plugin.js' | wc -l` (entries),
`find <dir> -name '*.js' | wc -l` (all JS files),
`find <dir> -name '*.js' -exec cat {} + | wc -l` (lines, including
blanks and comments). No other top-level directories exist; no `.js`
files live at `plugins/` root.

| Directory | Entries | JS files | Lines |
|---|---|---|---|
| accounts | 1 | 4 | 451 |
| achievements | 4 | 92 | 11,245 |
| agent | 1 | 26 | 5,958 |
| areas | 54 | 114 | 18,661 |
| bosses | 48 | 375 | 67,043 |
| bots | 2 | 108 | 24,467 |
| combat | 24 | 144 | 10,893 |
| commands | 9 | 10 | 3,190 |
| events | 1 | 1 | 244 |
| interface | 58 | 94 | 19,239 |
| items | 142 | 231 | 41,584 |
| minigames | 55 | 563 | 142,997 |
| modes | 5 | 161 | 22,182 |
| npcs | 60 | 115 | 22,497 |
| objects | 21 | 27 | 6,031 |
| persistence | 5 | 9 | 1,955 |
| quests | 4 | 273 | 277,911 |
| skills | 29 | 393 | 77,898 |
| tools | 3 | 10 | 1,965 |
| treasuretrails | 5 | 26 | 3,836 |
| world | 46 | 131 | 29,108 |
| **Total** | **577** | **2,907** | **789,355** |

Non-JS files under `plugins/` (310): 291 JSON data files, 7 Markdown,
3 woff2, 3 txt, 2 cjs, 2 LICENSE, 1 HTML, 1 CSS.

Largest areas, called out:

- **Quests (277,911 lines, 35% of all content):** 4 entries only.
  `quests/quests/` holds 193 `*.Quest.js` files (261,969 lines) plus
  7 more files; one shared engine, `QuestRuntime.js` (1,007 lines),
  plus `QuestRuntime`-suffixed units (`QuestList.QuestRuntime.js`,
  `ChatheadAnimations.QuestRuntime.js`). The 4 entries are
  `Quests`, `QuestCape`, `QuestCommand`, `QuestHelper`.
- **Minigames (142,997 lines):** 55 entries at the directory root,
  each with a unit subdirectory. Largest: `toa` (27 files, 17,196),
  `cox` (32 files, 12,443), `tob` (23 files, 9,828), `tfa` (21
  files, 9,525), `castlewars` (22 files, 7,170).
- **Skills (77,898 lines):** 29 entries. Largest units: `sailing`
  (108 files, 20,617), `construction` (54 files, 12,293), `agility`
  (38 files, 5,434), `farming` (16 files, 5,446).
- **Bosses (67,043 lines):** 48 entries at the root, each with a unit
  subdirectory (largest: `nex` 3,776, `doom` 3,764, `moonsofperil`
  3,555, `nightmare` 3,279).

Hub surface, measured the same day and re-verified 2026-11-15 by
script diff against the source during hub construction (distinct
member names, overloads collapsed): `PluginApi` has 203 distinct
member names (lines 999–1559), of which 108 are distinct `on*` hook
registrations; `PluginCoreApi` has 191 distinct member names (lines
1560–1771); `PluginManager` defines 87
`public static emit*` methods. The Go hub deliberately does NOT mirror
this surface; it exposes only what exists in Go (see `PHASE10_PLAN.md`).

Naming conventions observed (upstream `server/AGENTS.md` rules 3, 4,
12, 14, 15, confirmed by inspection):

- Entry files are `<Name>.plugin.js`, CommonJS, `register(api)` is
  attach-only: one line per hook, named module-level functions.
  285 of 577 entries are self-contained (no local `require`); 292 are
  thin shims delegating to unit files (e.g. 4-line
  `bosses/Whisperer.plugin.js` delegates to three `*.Whisperer.js`
  units). Entry line count says nothing about plugin size — always
  follow the `require` chain.
- Unit files end with the plugin name: `<Name>.<Plugin>.js`, e.g.
  `Courses.Agility.js` under `skills/agility/courses/`,
  `Gravedigger.RandomEvents.js` under `plugins/npcs/random-events/`.
- Quest units are `<QuestName>.Quest.js`; the shared engine is
  `QuestRuntime.js`, required directly by other areas
  (e.g. `areas/MorUiRek.plugin.js` requires `../quests/QuestRuntime`).
- Hooks match entities by name, not id lists
  (`onObjectInteraction("Ladder", ...)`); attribute keys are
  kebab-case and namespaced (`warriors-guild:basement-unlocked`),
  declared once in a `*_ATTRIBUTE` constant with
  `api.persistAttribute(KEY, default)`; plugin-private data lives in
  a `data/` directory next to the plugin.
- The conventions are aspirational in places: 896 relative
  `require("../../src/...")` calls bypass `api.core` (rule 2), and
  149 `require`s load another plugin's `*.plugin.js` file directly.
  A port must ground each such require in core source; the `api`
  surface alone does not cover it.

## 2. Engine-systems-first ordering

Evidence method: `grep -rhoE 'api\.[a-zA-Z]+'` /
`player\.[a-zA-Z]+` per directory, counted. Top calls per area
(counts are occurrences across all `*.js` in the directory):

| Area | Top api.* calls (count) | Top player.* calls (count) | Systems needed |
|---|---|---|---|
| quests | onCustomEvent 1214, persistAttribute 1097, onObjectInteraction 566, onNpcInteraction 505, emitCustomEvent 450, spawnNpc 297, onItemOnObject 243, onNpcDialogueCondition 200, onNpcDialogueVariant 164 | sendMessage 4064, getInventory 3483, get/setAttribute 3547, getPacketSender 1686, moveTo 891, getDialogueManager 235 | Dialogue + dialogue data, objects, NPC interaction/spawning, attributes, chat |
| minigames | onCustomEvent 309, onObjectInteraction 285, persistAttribute 241, onInterfaceActionButton 95, onNpcInteraction 91, registerNpcCombatMethodProvider 58, registerCommand 52 | sendMessage 1635, getPacketSender 975, getInventory 951, getLocation 748 | Combat + NPC drivers, objects, interfaces, areas/zones, chat, attributes |
| skills | onCustomEvent 267, emitCustomEvent 251, onObjectInteraction 180, onItemAction 82, onItemOnItem 51 | sendMessage 1109, getInventory 817, getPacketSender 704, getSkillManager 287, getDialogueManager 135 | Objects, item actions, XP sources, shared skill handlers, chat, attributes |
| bosses | emitCustomEvent 272, persistAttribute 120, onCustomEvent 116, onObjectInteraction 85, registerNpcCombatMethodProvider 63, onNpcDeath 36, onNpcHitModify 31 | getPacketSender 471, sendMessage 453, getLocation 294, getCombat 87 | Combat core + NPC drivers/combat methods, drops, chat |
| items | onCustomEvent 213, onItemAction 142, onItemOnItem 111, persistAttribute 84 | sendMessage 766, getInventory 537 | Item-action dispatch (exists, decode-level), attributes, chat, combat modifiers |
| npcs | onCustomEvent 116, onNpcInteraction 44, onNpcDialogueCondition 16 | getPacketSender 207, sendMessage 162 | NPC interaction, dialogue, chat |
| objects | onObjectInteraction 37, onItemOnObject 11 | sendMessage 57 | Object registry + walk-to-object, chat |
| areas | onObjectInteraction 71, onCustomEvent 70, persistAttribute 45 | getPacketSender 225, sendMessage 197 | Areas/zones, objects, dialogue, chat |
| interface | onCustomEvent 130, onInterfaceActionButton 70, persistAttribute 50 | getPacketSender 328, sendMessage 164 | Interface-action dispatch, widget open/close, chat |
| combat | registerCombatSpecial 89, onCombatHitDamage 11, registerMeleeHitModifier 9 | getAttribute 38, getEquipment 36 | Combat core (all of it) |
| commands | registerCommand 103 | sendMessage 169 | Chat + command registry |
| achievements | onCustomEvent 326, onNpcDeath 12, onZoneEnter 9 | get/setAttribute 150 | Custom-event producers everywhere else, zones, attributes |
| modes | onCustomEvent 461, emitCustomEvent 158, onInterfaceActionButton 55 | sendMessage 181 | Everything its modes modify (combat, interfaces, XP) |
| world | onCustomEvent 151, emitCustomEvent 119, persistAttribute 41 | getPacketSender 247, sendMessage 203 | Mixed; per-plugin |
| treasuretrails | onCustomEvent 30, onItemAction 15, onBeforeNpcInteraction 7 | getInventory 79, sendMessage 49 | Item actions, NPC interaction, dialogue, chat |
| bots/agent/tools/persistence | (see §5) | — | Bot runtime / MCP / ContentApi / already-ported persistence |

Pervasiveness, whole tree: `player.sendMessage` in 1,416 of 2,907
files; `getPacketSender` in 1,303; `get/setAttribute` in 1,002;
`getCombat` in 558; `getDialogueManager` in 465;
`api.persistAttribute` 2,018 occurrences. There is no content area
that avoids chat and attributes; those two systems gate everything.

Dependency order — engine milestones, each unlocking content.
Order matters: a milestone's content is portable only when every
system in its row exists in Go.

1. **Player attributes** (`get/setAttribute`, `persistAttribute`
   with save merge). Unlocks: state-keeping in every area; required
   before any quest, skill, item, or achievement port is faithful.
   DONE (phase 11, 2026-11-15): lazy attribute map on `Mobile`,
   live persistent-key/defaults registry, attributes as owned
   save state with TS-order entry application and from-scratch
   snapshot rebuilds, `Api.PersistAttribute` on the hub, and
   engine-consumed key registrations — live-verified end to end
   (persisted survives relogin, session-only expires,
   default-equal stripped and refilled). Plugins can now store
   state on players.
2. **Chat/message encoders + chat broadcast + command registry.**
   Unlocks: `commands/` (103 `registerCommand` calls), and every
   refusal/feedback line currently dropped silently (1,416 files).
   DONE (phase 12, 2026-11-15): chat-message encoder,
   `sendMessage`/`SendTypedMessage`, public-chat listener with
   broadcast + overhead sync text + mute/blocked-word gates,
   command registry + dispatch + hub `RegisterCommand` with
   rights gating, and examine answers — live-verified end to
   end (four clients: hook message delivery, near/far chat
   visibility, command dispatch + replies, NPC examine, mute
   refusal + command bypass). Players can now chat and run
   commands; no core commands exist (all commands are content).
3. **Object registry + object interaction** (MapObjects, walk-to-
   object, loc encoders; spawned objects are currently clipping-only
   and invisible). Unlocks: `objects/`, most of `skills/`,
   `areas/`, and the object half of `quests/`.
   DONE (phase 13, 2026-11-15): `GameObject`/`MapObjects`/
   `ObjectManager` registry with region-load population and
   JSON-spawn registration, loc encoders + per-session object
   sync (scene snapshots + per-tick deltas), object-action +
   item-on-object listeners with walk-to-object, and the hub's
   object-interaction hook families + `getObjectManager`
   surface — live-verified end to end (spawn visibility,
   runtime spawn/remove deltas with distance gating,
   click-to-hook). Objects are now visible and interactable.
4. **Dialogue framework + dialogue data loading** (`DialogueManager`,
   `npc-dialogues.json` / `event-dialogues.json` / index loading,
   `npc-dialogue:*` events). Unlocks: `npcs/`, the dialogue half of
   `quests/`, and every `npc-dialogue:line/action/choice/hook`
   listener (quests alone: 205 + 183 + 152 + 126 listens).
   DONE (phase 14, 2026-11-15): `DialogueManager` state machine
   with all 8 entry types + chatbox encoders, dialogue data
   loaders (validated read-only against the real dir), the
   NpcDialogues transcript playback engine (conditions,
   variants, jumps, menus), the `MultiChatboxPrompt` registry
   behind `sendMultiChatboxPrompt`, the NPC-click-to-dialogue
   dispatch with walk-to-NPC, and the hub's dialogue hook
   families + `npc-dialogue:*` events — live-verified end to
   end (click → lines advance → choice branches → dialogue
   ends; hub-hook-started dialogue behaves the same). NPCs can
   now talk. Residual gaps: `dialogue-types.json` (pet
   content), retired-NPC successors (plugin-private data),
   static-binding registry, NPC transforms. (Shop steps were
   a residual gap until phase 17, which wired them to the
   ShopManager.)
5. **Timers + areas/zones** (per-player timers, `Area` registry,
   `onZoneEnter/Exit`, Wilderness/area checks that currently
   degenerate). Unlocks: `areas/`, minigame arenas, drop-check and
   aggression behaviour.
   DONE (phase 15, 2026-11-15): `TimerRepository`/`TimerKey`
   countdowns with TS tick-phase placement and FREEZE/STUN
   mobility gates, code-registered `Area`/`AreaManager`/
   `Boundary` with membership evaluation + enter/exit/leave
   hooks + multi-icon follow, `Wilderness` PvP/safe-area
   helpers with the live drop check, the area `getMusicTrack`
   music override, and the hub's zone families +
   `registerArea` with the isolating `callArea` caller —
   live-verified end to end (10-tick timer fired exactly
   once, zone/area enter + exit/leave on walks in and out,
   cancelled timer never fired). Content can now schedule
   timers and react to zone/area enter/exit. Residual gaps:
   `PrivateArea`/instancing (milestone 11 — resolved in
   phase 21) and the
   `getAreaManager` query surface (zero TS callers —
   resolved in phase 21 as a narrow query interface);
   `syncWildernessStateForMovedPlayer` (wilderness level +
   "Attack" option on movement) remains a gap.
6. **NPC drivers + combat core** (wandering/following/AI drivers,
   combat methods, hit/damage pipeline, drops, prayer). Unlocks:
   `bosses/`, `combat/`, most of `minigames/`, Slayer, and every
   `registerNpcCombatMethodProvider` / `onNpcDeath` consumer.
   DONE (phase 16, 2026-11-15): the movement coordinator
   (wandering, walking home, retreat, radius-0 rule), NPC
   aggression + tolerance, following/facing/interaction, the
   respawn scheduler, the combat core (attack cycles,
   melee/ranged/magic methods, exact accuracy/max-hit
   formulas, hit queue + hitsplat sync, player/NPC death +
   respawn, protection prayers + drain, special-energy
   restore, combat XP through the SkillManager), the NPC +
   player attack listeners, and the hub's combat families +
   `registerNpcCombatMethodProvider` — live-verified end to
   end (an NPC wandered within its radius, an aggressive
   spider engaged and fought, the player killed it, the
   death hook fired, it respawned and died again). Drops
   stay content's job (TS core rolls none); spellcasting is
   DONE since phase 27 (see below) and item-on-NPC stays
   decode-drop (no item-on-NPC dispatch yet). Combat-style selection is
   DONE since phase 26 (styles registry + weapon interfaces
   + BonusManager + varp 43/172, login + death assign);
   `onCanAttack` is firable since phase 26; ranged
   weapon/ammunition data is ported since phase 26 (the
   content-owned resolver is registered in production);
   ranged projectile producers exist since phase 26; dead
   NPCs leave the world at the remove point since phase 26
   (the corpse-removal half was missing until then).
   Combat spells are DONE since phase 27 (2026-11-17):
   the 36-spell standard book (script-extracted verbatim)
   with `canCastSpell` in TS order, the MagicOnNpc (75) /
   MagicOnPlayer (32) / MagicOnItem (90/94) listeners out of
   decode-drop, spell-id resolution through the cache
   spell-name fallback, autocast (selector, indicators,
   login restore, `autocastSpellId` now owned save state —
   the phase-27 save-split fix) + spellbooks, and the
   `onSpellDisabled` / `onSpellRuneBypass` hub families
   (48 of 108 firable, 29 of 95 members) — live-verified
   end to end (Wind Strike, High Alchemy, Telekinetic Grab
   all landed in production; the autocast pick survived a
   logout/relogin round-trip and fired as magic on the next
   login).
   Poison/venom + skulls + player death are DONE since
   phase 28 (2026-11-17): the `PoisonClock` + effect
   (6-in-11 NPC application, 25/50/75 venom tiers, orb
   varp 102, 258-entry script-extracted data table, save
   round-trip through the coordinator's wiring) + the
   skull system (WHITE/RED icons, 30-minute timer, Protect
   Item interaction) + the death flow (kept-items rules,
   bones + item loss to the floor, respawn at the world
   spawn, the `onShouldDropItemsOnDeath` /
   `onShouldKeepItemOnDeath` / `onPlayerDefeated` /
   `onPlayerDeathItemDrop` / `onPlayerDeathDrops` hub
   families — 51 of 108 firable, 29 of 95 members) —
   live-verified end to end (a cave crawler poisoned a
   player and the affliction survived a logout/relogin
   round-trip and resumed live; a skull timer counted down
   live and expired to zero; a killed player respawned at
   home with exactly the right items kept and the rest on
   the floor).
   Combat presentation is DONE since phase 29 (2026-11-17):
   the `NpcAnimationScanner` port (seq decoder + stall +
   boot preload), attack/block/death animations + spot
   animations at the combat call sites, `StartCast` live
   (staff-variant anims, start graphic, cast projectile,
   queued end-graphic, impact graphic + sound), melee
   projectiles verified absent in TS (none invented),
   ranged launch graphics populated (82 kinds with a start
   graphic, 10 with a double; venator 2289, dark bow
   double, blowpipes suppress), sound pickers at the call
   sites, and the `player:dying` custom event — hub counts
   unchanged (51 of 108 firable, 29 of 95 members) —
   live-verified end to end (melee chop/block, ranged
   launch graphic + projectile + fire sound, Wind Strike
   cast + impact presentation, death animation 836 with
   respawn). Combat is finished as a planned engine slice.
   Residual gaps: hit-modify/death-gate families,
   animations/graphics/sounds (WeaponProfiles profile
   layers, `npc-combat-sounds.json` death sounds,
   NPC-with-spell casts, special-attack fire sounds),
   equipment effects, no live PvP attack (the
   Wilderness `canAttack` content gate is unported),
   no grave, no deathComponents splitting, no cure items,
   no RED-skull core call site, Ancients/Lunar/Arceuus
   spellbooks, bind/curse/freeze application (events only),
   spell-on-object, ElementalStaves rune coverage, and the
   production data gaps (`item-alch.json` and
   `item-gameplay.json` shipped since phase 27, Tyler-
   approved verbatim import).
   Combat leftovers, first slice, are DONE since phase 30
   (2026-11-18): WeaponProfiles profile layers (4-layer
   attack-animation order, all 18 entries), dark-bow
   second-hit delay ([3,4] at distance 9), exact
   projectile heights (float64; bow 40.75/36.5),
   NPC-with-spell casts (engine path open; no producer
   sets an NPC's cast spell), special-attack fire sounds,
   queued-graphic priority (TS identity check), spec-orb
   sends (drain, login, death reset; the weapon-meter
   tail is dead in TS itself — `PlayerSession.write`
   is a no-op at bfd81e3), deathComponents splitting
   (Nightmare staves, Avernic treads), fightType wired
   live at login, the onCanAttack legacy fallback
   removed, and the plugin ammo / crystal-bow charges
   engine part (all 193 ammo kinds carry strength; a
   real bug fixed — `DecrementAmmo` ate a crystal bow
   on its first shot). Hub counts unchanged (51 of 108
   firable, 29 of 95 members — corrected from 53 in phase 31;
   see the hub row) — live-verified end to
   end (profile anim 15114, dark-bow hits one tick
   apart, orb varps at login, crystal bow surviving
   fire). Residual gaps: the gap-14 spellbook residuals,
   `npc-combat-sounds.json` (import needs Tyler's
   approval), NPC spell selection from NPC combat defs,
   the crystal-bow degradation loop + charge system,
   plugin ammo resolution, `ItemAliases`, the javelin
   weapon-interface seam, no grave, no live PvP, no cure
   items, no RED-skull core call site. Tyler decided
   2026-11-17: combat leftovers first, then adding all
   skills. Skills survey DONE since phase 31 (2026-11-19,
   `SKILLS_SURVEY.md`, docs only — no Go files touched, no
   skill picked): the whole `plugins/skills/` tree measured
   (29 entries, 393 files, 77,898 lines; per-skill
   require-closure true totals from 63 for Reanimation to
   20,617 for Sailing), a lexical dependency sweep, a
   data-file map (all 32 `data/*.json` files referenced),
   a cross-dependency map (388 custom events, 33
   cross-skill pairs), and the Go skill-engine gaps
   (NPC-interaction family, item-action/item-on-item
   families, spell-on-X/`onSpellCast`, consumable/
   equipment gates, `CreationMenu`, SkillLevelUp feedback,
   ground-item seam + `forceAdd`, `spawnNpc`/`removeNpc`).
   Ranked shortlist of 5, all read in full: 1. BarbarianPyres
   (266 lines, RECOMMENDED — core loop needs no new engine
   system and no data import), 2. BoneAltars (269),
   3. Ectofuntus (747), 4. Herblore (761), 5. Fletching
   (965). Tyler picked Woodcutting as the first skill on
   2026-11-19 (over the survey's recommended BarbarianPyres)
   and re-confirmed it the same day after a conflicting
   Fletching widget tap. Tyler then picked Fletching as the
   second skill on 2026-11-20, answering the parent's
   explicit "which skill next?" question — no conflicting
   signal that time. Phase 32 (2026-11-21, `PHASE32_PLAN.md`)
   ported the Woodcutting core chopping loop, live-verified
   end to end (bronze axe, real tree, 2 logs, 50 XP, tree
   depleting and respawning, `woodcutting:logs-chopped` ==
   2): the §1 tables verbatim (30 axes, 14 cut-chance
   charts, 21 trees, 24 stump-model mappings), the 1-tick
   global task with the 4-tick roll cadence, depletion/
   respawn via Forestry timers + stump swap + client script
   5474, bird nests, and the `onItemAction` hub family
   built as a seam (54 of 108 families firable) plus the
   `ObjectManagerAPI.Get` seam for map originals. No data
   file was imported in phase 32. Phase 33 (2026-11-21,
   `PHASE33_PLAN.md`) finished Woodcutting: all six
   sub-units except the Guild — EntTrunk, ThickVines,
   Shrine, ClueNests, CrystalAxe, InfernalAxe — with the
   `onNpcClick` + `onItemOnItem` hub families and the NPC
   remains-delay + transform seams built as part of it, and
   the core-loop residuals closed (stump model-map + treeId+1
   legs, footprint interaction distance, face-tree step,
   `treeAt` removed-objects leg); hub 56 of 108 families
   firable (60 methods). No data file was imported in either
   phase — the `ents` + `shrine` values were transcribed into
   Go tables with JSON line citations, and
   `woodcutting-guild.json` itself was NOT copied into the
   repo. NOT STARTED: the Woodcutting Guild (needs Tyler's
   approval to import `woodcutting-guild.json`, asked
   2026-11-20, awaiting his answer); the `item:operated`
   emit and `subOption` fill-in; the SkillLevelUp emitter.
   Phase 34 (2026-11-21, `PHASE34_PLAN.md`) ported Fletching,
   the second skill, members-only, live-verified end to end
   (knife on logs → CreationMenu script 2046 → 150 arrow
   shafts, 50 XP; bow string on unstrung shortbow →
   shortbow, 5 XP, no second menu; spool fill → 5 charges;
   3 runs × 11 checks, 0 failures): the full Shared loop,
   Recipes, Spool, and Diaries, plus the CreationMenu engine
   gap closed as a `game` + `session` seam (frame-byte
   verified; script 2046 travels on opcode 110
   `WIDGET_RUN_SCRIPT`, not 170, and the menu mount does not
   route through `SendChatboxInterface` — both plan
   corrections verified against the TS encoders). The
   `fletching.json` loader reads the verbatim import Tyler
   approved 2026-11-20 (SHA-256 identical, 170 recipes,
   load-proven against the real cache, temporary test
   deleted); no data file was imported in the phase itself.
   Hub unchanged (56 of 108 families firable). Phase 35
   (2026-11-21, `PHASE35_PLAN.md`) ported Fishing, the
   third skill, F2P, live-verified end to end in 4 modes
   (net → shrimp, 10 XP; bait rod → sardine, bait 20→19,
   20 XP; fly rod → trout, feathers consumed, 50 XP; full
   inventory → statement + sound 2277, no catch; 4 modes ×
   3 runs, 0 failures): the core loop (tool + bait checks,
   1-tick catch ticks, exact catch/XP/depletion rules) plus
   8 of the 10 sub-units — AnglerOutfit, CrystalHarpoon,
   InfernalHarpoon, KarambwanVessel, RadasBlessing,
   SpiritFlakes, ClueBottles, Camdozaal tools, Guild
   invisible boost — with the `onNpcInteraction` engine gap
   closed as a `plugins` seam (name hooks after the
   onNpcClick bindings, TS order; hub 57 of 108 families
   firable, 61 methods). No data file was imported in the
   phase (Fishing has none); the tables were transcribed
   into Go source with TS line citations, load-proven
   against the real cache. Deferred: MinnowPlatform (needs
   an NPC-relocation seam), the AnglerOutfit Gita exchange
   (needs `onItemOnNpc`, still table-only), and the Guild
   door (needs ObstacleRunner / ForceMovement). Phase 36
   (2026-11-21, `PHASE36_PLAN.md`) ported Cooking, the
   fourth skill, F2P, live-verified end to end in 3 modes
   (fire → shrimps, 30 XP, burns observed; range → shrimps,
   sardines untouched; wine → jug of wine, 200 XP per good
   jug; 3 modes × 3 runs, 0 failures): the whole skill —
   Cookers core loop (timing 2/3/4, gates in TS order),
   Recipes + doughs, Wine (20-tick ferment), Churn,
   SacredEel, LeapingFish, Gnome — behind the Go loader for
   the verbatim `cooking.json` Tyler approved 2026-11-20
   (122 cookables, 47 recipes, load-proven against the real
   cache, temporary test deleted); the `cooking:raw-xp`
   answerer unblocks Phase 35's infernal-harpoon cook at the
   unit level (live observation still open). No data file was
   imported in the phase itself. The next skill is Tyler's
   pick.
7. **Shops + trading/duelling + Grand Exchange.** Unlocks: shop
   content (`shop:*` events, e.g. `npcs/Apothecary.plugin.js`),
   `interface/` trading panels, Duel Arena staking.
   DONE (phase 17, 2026-11-15): `ShopManager` (definitions in
   TS load order with the `onCanStockItem` veto, stock/
   restock, rated + legacy pricing, currencies incl.
   content-registered ones, access rules), shop flows (NPC
   Trade dispatch, dialogue `open_shop` steps, widget
   buy/sell with TS-exact refusal text, close behavior),
   `Trading` (requests, offer/confirm stages, transfer,
   mobility/combat gates, trade-escrow save fold),
   `Duelling` state + the duel-order shuffle exception, the
   hub's shop/trade families + currency registration + the
   ten `shop:*` custom events, and the GE-value accessors
   feeding shop pricing, trade values, and the examine price
   line — live-verified end to end (hook-opened and
   dialogue-opened shops showed stock, buy deducted coins,
   sell returned them, a two-player trade completed with the
   offers transferred, a decline left both inventories
   unchanged). Players can now buy, sell, and trade.
   Boundary (grounded in TS): `Duelling.ts` is shared state
   only — Duel Arena gameplay/staking is plugin content and
   stays NOT STARTED; the Grand Exchange marketplace is
   plugin content needing milestone 8 and stays NOT STARTED.
   Residual gaps: no sounds, walk-to-entity lacks
   facing/pursuit/boat handling. (The phase 17 residual gaps
   naming the varp/varbit store, the entered-amount
   listener, and interface state were closed by phase 18:
   the examine price line is live, the trade accept-delay
   triggers, buy/sell-X prompts round-trip, and interfaces
   auto-close on movement/logout/death.)
8. **Interface runtime** (interface-action dispatch, open/close,
   widget state). Unlocks: `interface/` (58 plugins), bank/shop
   interface flows, minigame panels.
   DONE (phase 18, 2026-11-15): per-player interface state
   (`interfaceId` + close gate, `interface:closed`/
   `interface:changed` events), the `PacketSender` mount/
   sub-interface/chatbox/tab layering, interface-layout open
   dispatch on all 7 surfaces, interface-action dispatch to
   the hub families (prompt-first, stop once handled), the
   entered-amount/syntax prompt listeners, and the live
   varp/varbit store (change detection, varbit packing,
   replay) — live-verified end to end (login hook opened
   interface 500, a widget click dispatched to the hub hook
   and armed an amount prompt, answering the prompt reached
   the hook, a varp set through the store reached the
   client, walking away closed the interface). Content can
   now open interfaces, receive widget clicks, prompt for
   amounts, and read/write varps.
   Boundary (grounded in TS): the live store is the
   `PacketSender` varps map — `VarManager` is transform
   plumbing with zero core instantiations and stays NOT
   STARTED; bank interface flows are plugin-driven in TS
   (no core bank listener) and stay content work;
   `registerCustomInterface` stays NOT STARTED (needs the
   HTTP ContentApi endpoint; the Go transport is WS-only).
   Residual gaps: the per-player vars seam unwired,
   bootstrap/music/dialogue-unclamp varp sends bypass the
   store, `emitButtonClick` fallback, bank/prayer/combat/
   spell widget branches, spell-on-item routes, audio-
   setting application from `varp_transmit`.
9. **Clans/social + friends/ignores.** Unlocks: clan content,
   social packets, ignore-list behaviour.
   DONE (phase 19, 2026-11-15): the whole `PlayerRelations`
   port (friend/ignore lists, ranks, chat modes, presence/
   PM/public-chat gates, `updateLists`, TS-verbatim refusal
   text), the live relations chat gate (closing the phase 12
   degeneration), social-packet decoders + dispatch emitting
   all 8 `onSocialPacket` variants to the hub with no core
   default action (exactly as TS dispatches them), the
   friends-chat-snapshot + clan encoders, relations as owned
   save state (modes session-only), and the logout
   presence-offline call — live-verified end to end (three
   clients: friend/ignore flows, ignore gate, PM delivery,
   all dispatch kinds, logout/relogin round-trip).
   Boundary (grounded in TS): friends-chat channels
   (`FriendsChatManager.js`) + the clan system (`clans/`,
   ~5,082 lines) are plugin content and stay unported; the
   legacy presence sends are no-ops upstream
   (`PlayerSession.write` is empty) — live presence comes
   from the snapshot, which content sends;
   `onFriendAdd`/`onFriendRemove` have zero emit call sites
   in TS and stay table-only. Residual gaps: snapshot
   sending (no content raw-send path), the sender-side PM
   echo, `chat.filters` attribute persistence, Core
   exposure of the clan encoders.
10. **Skills shared handlers + XP sources** (the `SkillActions.js`
    pattern, skilling frameworks, XP grants). Unlocks: `skills/`
    piece by piece; each skill still ports separately.
    DONE (phase 20, 2026-11-15): the XP gate + world rate
    (`onCanGainExperience` veto-plus-accumulator, deny before
    any send, rate from world.json `experienceMultiplier`),
    the `onPlayerLevelUp` / `onExperienceGained` hub families
    firing from `addExperience` with the two different
    experience values TS sends, the `skills:experience-gained`
    / `skills:current-level-changed` / `skills:restore`
    custom events (value payloads except the pointer
    `skills:restore` read-back), the player skill slot +
    `startSkillable` / `stopSkillable` (live from
    `walkToReset` and the Drop/Equip listeners), the
    `setLevel` Wilderness + equipment-requirements guards,
    skill metadata (names, buttons, chatbox interfaces,
    client ids), the `SkillActions` helper
    (`pauseForLevelUp` / `actionInterval`, exact port), and
    the `getTaskManager` hub member over the engine
    `TaskManager` — live-verified end to end (a stub
    skilling action: gate doubled one gain and refused
    another, level-up + experience-gained hooks fired,
    the action-interval hook paced the grants, a simulated
    level-up pop-up paused and resumed the session).
    Boundary (grounded in TS): SkillLevelUp feedback
    (message, graphic, pop-up) is plugin content and stays
    NOT STARTED; `pressedSkill` is dead code upstream (zero
    callers) and is deliberately not ported; the exp-drop
    send is a no-op in TS at `bfd81e3` (empty
    `PlayerSession.write()`, legacy opcode 116 colliding
    with `WIDGET_SET_ROTATE_SPEED`, no native encoder) —
    Go mirrors it with a layout-only encoder + discarding
    sender, and the wire stays clean of opcode 116.
    Residual gaps: SkillLevelUp content, `CreationMenu`
    (no Go type; the `stopSkillable` menu clear stays a
    gap), the core `skills:action-interval` emit site
    (rides the decode-drop MagicOnItem listener),
    `setLevel`'s vengeance / `BonusManager` /
    weapon-interface calls + rights bypasses (no live
    rights field), the Emotes/TeleportHandler/MagicOnItem/
    SecondGroundItem `stopSkillable` call sites (no Go
    equivalent), and all individual skill content.
11. **Sailing/boats + template regions + sounds/projectiles.**
    Unlocks: `skills/sailing`, Construction player-owned houses,
    instanced content, and the remaining encoders.
    DONE (phase 21, 2026-11-16): the sailing engine
    (`Sailing.initialize()` idempotence, type/dock
    registries, `giveBoat`, boarding/disembark/guest rules,
    the `sailing:*` custom events, login/logout calls in TS
    order, sailing state as owned save state), the boat
    manager (spawn/dispose, deck-loc CRUD, root location,
    deck-tile projection, sync-area rule, steering,
    `isSailable`, teleport, per-tick process), boat-deck
    areas + world-entity sync (view distance 32, max 8/4),
    private areas + templated instances (chunk grid,
    packing/rotation, scene palettes, clip grids, template-
    object lookup, instance source tile), template-region
    decoding, map-region replacements (packets, `.pack`
    parsing, visible-replacement sends), the sound/jingle/
    projectile/camera/world-entity/graphic encoders +
    senders (all live upstream via `sendClientPacket` — no
    no-op mirrors), the `onWorldEntityInteraction` hub
    family + the instancing members + the scoped Core
    accessors — live-verified end to end (sound both
    forms, jingle, projectile, boat boarding with
    world-entity frames + deck-scene rebuild, teleport snap
    update, templated rebuild with the probe chunk).
    Boundary (grounded in TS): the sailing SKILL
    (`skills/sailing`, ~20,617 lines) and Construction POH
    content (~12,293 lines) are plugin content and stay NOT
    STARTED; no `on*` families exist for sound/projectile/
    camera/private-area in TS and none were invented.
    Combat presentation landed in phase 29 (the
    `NpcAnimationScanner` port, animation/graphic
    producers, melee/magic projectile producers — melee
    verified absent in TS — and sound pickers). Residual
    gaps: the
    `viewedArea` object-sync term, the `sceneTile`
    root-location term, deck-object interaction lookup,
    camera state + restore, destroy-hook splicing,
    `MovePlayerTo` following/interaction clearing. The
    engine-systems plan is now EXHAUSTED: all 11
    milestones are DONE; what remains is content ports.

Blocked on multiple systems (do not schedule early): quests
(milestones 1–4 + 6 for combat quests), minigames (1–6 + 8),
bosses (1, 2, 5, 6), modes (1, 2, 6, 8, 10 — modes modify other
systems, so they port last), achievements (needs the custom-event
producers of every area it observes).

Cross-plugin coupling, measured: `emitCustomEvent` 2,876
occurrences / 861 distinct event names; `onCustomEvent` 3,678 /
1,014 distinct names. For the top 3 areas:

- Quests emit `quest:is-complete` (127), `quest:get-stage` (24),
  `quest:is-started` (17); they listen to `npc-dialogue:*` (666
  combined) and to other areas' events (`ladders:climb` 57,
  `door:toggle` 55, `ferries:travel` emits 9). `quest:completed`
  is listened to from 8 areas (quests 4 files, plus interface,
  minigames, modes, npcs, persistence, skills, world).
- Minigames emit `collection-log:obtain` (29), `quest:is-complete`
  queries (19), `combat-achievements:*` (22 combined); they listen
  to `npc-dialogue:*` (58), `npc-drops:roll` (8),
  `omnishop:currency` (7).
- Skills emit `quest:is-complete` queries (22), `diary:task` /
  `diary:is-complete` (30 combined), `skills:produced` (14); they
  listen to `sailing:*` (20), `equipment:effects` (10).

`dependsOn` is declared in only 18 of 577 plugin entries (e.g.
`LootingBag` on `Wilderness`, `Food`, `Potions`, `LootKeys`;
`Chests` on `Quests`, `Clans`, `RandomEvents`, `GiantLairs`;
`CastleWars` on `Food`, `Mining`, `Ladders`, `Potions`,
`Omnishop`). The real coupling is the custom-event graph above
plus 149 direct cross-plugin `require`s — `dependsOn` understates
it badly. A ported piece must name the events it emits/listens to
and which producers/consumers exist in Go; an event with no Go
producer is a gap, not a silent no-op.

## 3. Recommended first content pieces

Ordered. Each names exact upstream files and what must exist first.

1. **`world/DateMinutes.plugin.js`** (28 lines, self-contained) —
   portable with ZERO new engine systems, modulo one send-path
   decision documented in §6. Sends varp 3078 (minutes since epoch)
   on login and each new minute. Observable without chat; exercises
   login + per-tick process hooks. Recommended as the Phase 10
   proof piece; see §6 for the full survey.
2. **`items/GracefulOutfit.plugin.js`** (137 lines, self-contained)
   — needs ONE engine seam: the `EquipmentEffects` run-energy
   modifier registry (Go's drain/restore modifier hooks are
   identity today) plus an `equipment:weights` emit site in the
   weight calculation. The `skillcape:has-perk` query degrades
   honestly (no skillcapes plugin in Go → false, the no-hooks
   default). Observable: restore rate and carried weight change
   with the outfit. Representative of `items/` equipment-effect
   content; pair it with **`items/WornWeights.plugin.js`**
   (38 lines, same `equipment:weights` seam) in the same phase.
   DONE (phase 22, 2026-11-16): both ported to
   `internal/content/gracefuloutfit` + `internal/content/wornweights`
   with the seam (world-scoped `EquipmentEffects` registry,
   `equipment:weights` emit in `CarriedWeightGrams`, hub
   register members), live-verified 3 runs stable (full set
   +19/tick at −25kg, five pieces +17/tick at −21kg, boots
   of lightness −4kg, mid-session equips). Boundary: the
   ports + the seam only — no other content, no skillcape
   content, no EquipmentStats port. Residual gaps:
   `equipment:effects` has no Go producer (EquipmentStats
   not ported), `skillcape:has-perk` has no Go answerer
   (SkillCapes not ported; substitute path unit-covered
   only), the login bootstrap run-energy frame bypasses the
   weight overrides (first tick frame corrects it), other
   `EquipmentEffects` modifier families NOT STARTED.
3. **`npcs/LumbridgeGuide.plugin.js`** (7 lines) +
   **`npcs/lumbridgeguide/Dialogue.LumbridgeGuide.js`** (18 lines)
   — the smallest dialogue content in the tree: one
   `onNpcDialogueVariant` registration. Needs milestone 4
   (dialogue framework + data loading) and milestone 1
   (attributes, for the lost-amulet check). It is the right first
   proof that dialogue works end to end, before any quest port.
   DONE (phase 23, 2026-11-16): ported to
   `internal/content/lumbridgeguide` (page/variant constants
   verbatim, the `lumbridge-guide:amulet-status` query emitted
   through the hub's synchronous custom-event path, shared
   request type in `internal/game`), no new hub surface needed,
   live-verified 3 runs stable (a real Talk-to click played the
   NORMAL fixture line unanswered and the LOST fixture line
   with a temporary probe answerer; the NORMAL observation
   proves the hook overrode the loader's default). Boundary:
   the port only — the Guardians of the Rift answerer was NOT
   ported, per the upstream unit's own header comment (the
   normal F2P conversation must remain available when the
   members' recovery content is absent); the attribute read
   lives in the unported answerer, not in this piece. Residual
   gaps: the query has no Go answerer (NORMAL always plays in
   production until GOTR ports), the repo's `{}` starters lack
   the guide's index + record entries (documented need, nothing
   copied), the LOST branch was observed live only via the
   removed probe.
4. **`items/SeekingArrows.plugin.js`** (21 lines, self-contained) —
   one `onCombatHitDamage` handler: seeking dragon arrows raise
   successful bow hits to at least 3 damage. Needs milestone 6
   (combat hit pipeline). Smallest real combat-modifier content;
   proves the combat hook surface before any boss or special
   attack is attempted. DONE (phase 24, 2026-11-16): ported to
   `internal/content/seekingarrows` (guard order verbatim:
   accurate, player attacker, RANGED type, no active special,
   ammunition item id 33595; the noted form 33596 does not
   match; no weapon check exists upstream), no new hub surface
   needed, plus the minimal grounded seam
   `Mobile.IsPlayer()`/`AsPlayer()` in `internal/game`
   (backed by the `combatantFor` identity mechanism, mirroring
   TS `Mobile.isPlayer()`/`getAsPlayer()`). Live-verified 3
   runs stable over 2 invocations (a real option-2 Attack
   click on a spawned goblin: seeking-arrow lives ran
   `[3 2]` — every non-final hit at the floor of 3, the
   final 2 cap-bound at remaining HP; rune-arrow control
   lives ran `[1 1 1 2]`/`[1 2 2]`/`[1 2 1 1]`, each life
   summing to the goblin's 5 HP). Boundary: the port + the
   identity seam only — no ranged-data tables, no other
   content. Residual gaps: no ranged-data resolver is
   registered in production, so the handler cannot fire in
   production until ranged weapon/ammunition data is ported
   (the live runs used a test-only resolver in the throwaway
   harness); `isNpc`/`getAsNpc` stay unported.
5. **First quest, only after milestones 1–4 + 6:** do NOT pick yet.
   Quest selection needs its own survey once dialogue, objects,
   and combat exist: prefer the shortest `*.Quest.js` with no
   combat, no instancing, and no cross-quest dependencies. The
   193 quest files range widely in size; picking one now, before
   the systems exist, would be guessing.
   SURVEY DONE (phase 25, 2026-11-16): see `QUEST_SURVEY.md` —
   ranked shortlist of 5 with measured evidence, Doric's Quest
   recommended. The pick is still Tyler's; no quest was ported
   and none is decided.

## 4. Porting rules

For every future content phase:

- **Faithful port from TS/JS source.** Cite file and line for each
  behaviour (e.g. `quests/quests/CooksAssistant.Quest.js:42`).
  Follow the `require` chain into unit files; never port an entry
  shim alone. Where upstream violates its own conventions (relative
  core requires, direct cross-plugin requires), ground the behaviour
  in the core source it reaches and record the deviation.
- **Fixtures-only unit tests.** No live cache, no network, no real
  definition data in tests. Pure logic (bonus calculations, name
  derivations, table picks) must be factored so it tests without a
  player, as upstream does with `_test` exports.
- **Live verification before claiming behaviour.** A ported piece
  is claimed working only after a live smoke test against the real
  rev-242 cache shows its observable effect (packet capture, sync
  frame, or save round-trip). Unit tests alone never close a phase.
- **Per-phase approval.** Tyler approves each phase's scope before
  work starts, as with phases 1–10. A phase names its engine
  milestone(s) from §2 and its content piece(s) from §3 or a new
  survey; scope creep into unapproved systems is out.
- **Honest gaps in PORT_STATUS.md.** Status values DONE / PARTIAL /
  NOT STARTED, with the gap named at the site. Never a silent stub:
  a dropped packet, an unemitted event, or a degenerated check gets
  a named reason in code and a line in PORT_STATUS.md.
- **No copied `*.plugin.js` files.** Go ports are written in Go.
  Short illustrative snippets (a few lines, e.g. a `register()`
  shape) in docs are fine; reproducing plugin bodies is not.
- **Hub registration via `internal/plugins`.** Content registers
  through the compile-time registry with its exported name, members
  flag, `dependsOn`, and config declarations, mirroring the TS
  entry. No file discovery, no dynamic loading. One Go package per
  content piece under `internal/content/<name>` (established by the
  Phase 10 proof piece, `internal/content/dateminutes`); the server
  pulls a piece in with one blank import in
  `cmd/server/pluginwiring.go`.
- **Unported system needed → gap it, don't invent it.** If a plugin
  needs a system that does not exist in Go, the port stops at the
  seam: record the missing milestone, port nothing around it, and
  do not approximate the behaviour with invented logic. Partial
  ports name exactly which hooks are wired and which are waiting.
- **Upstream conventions, translated.** `register` stays attach-only
  (one hook per line, named functions); hook by entity name, not id
  lists; content talks through namespaced custom events
  (`domain:event`); attribute keys stay kebab-case and namespaced;
  members content sets the members flag. Unit-file naming follows
  the Go package layout, with the upstream file cited at the top.
- **Scale honesty.** No wholesale port (see §5). Selection criteria
  for what gets ported, in order: (a) its engine milestones exist;
  (b) it is small and self-contained (few cross-plugin events);
  (c) it is observable and verifiable live; (d) it is representative
  of its area, so the next port in that area is cheaper. Anything
  failing (a) waits, however small.

## 5. Non-goals + honest scale statement

Non-goals, permanently: a wholesale port of the 789,355-line
content library; an embedded JS runtime; a Node sidecar; running
unported `*.plugin.js` files in any form; the bot runtime; the
agent/MCP integrations; the cache/map editor tooling.

What will likely NEVER be ported, with reasoning:

- **The long tail of quests.** 193 quest files, 261,969 lines in
  `quests/quests/` alone, each needing dialogue, objects, combat,
  and quest-specific state. A handful of short quests may port
  over time; the full catalogue will not. The QuestRuntime engine
  (1,007 lines) is portable; the 193 quests on top of it are a
  multi-year tail.
- **The long tail of minigames.** Raids (ToA 17,196 lines, CoX
  12,443, ToB 9,828), the Inferno/Colosseum, Castle Wars, and the
  rest each need combat, instancing, interfaces, and areas. One or
  two small minigames may port; the catalogue will not.
- **Modes (22,182 lines, 5 entries).** Leagues, Hardcore, and
  Deadman modify combat, XP, interfaces, and death handling across
  the whole server; they port last or never, because every system
  they touch must exist first and their event surface (461
  `onCustomEvent` calls) is the widest in the tree.
- **Agent (5,958 lines) and tools (1,965 lines).** MCP servers,
  AI-agent integrations, and the cache editor are development and
  automation tooling, not game content. They depend on Node
  libraries and the ContentApi; there is no Go case for them.
- **Bots (24,467 lines, 2 entries + runtime).** Player bots need a
  bot runtime, AI drivers, and combat; the Go port has none, and
  bots exist to populate a development world, not to serve players.
- **Sailing (20,617 lines) and Construction (12,293 lines).**
  Sailing needs boats, charting, and template regions; Construction
  needs player-owned houses and template regions. The engine
  mechanisms behind both landed in milestone 11 (phase 21,
  the last engine milestone — DONE); the content pieces
  themselves are large enough to be projects of their own
  and port only piece by piece, with direction.
- **Persistence plugins (1,955 lines).** Already ported: the Go
  `internal/persist` package covers JSON/SQLite/memory saves. The
  IndexedDB/browser-host plugin is irrelevant to a server-only port.

The honest picture: the Go port has acquired all 11 engine
milestones in §2 (phases 11–21) and will, over many phases,
acquire a selected slice of content — on the
order of tens of plugins, not 577. Every phase must be able to say
which piece it ports and what stays unported. A phase that cannot say the second thing is overscoped.

## 6. Proof-piece survey

Question: is there a real TS plugin whose EVERY dependency already
exists in Go (movement/collision, containers/appearance/skills,
music, ground items, saves, loaders, the 5 implemented listeners,
lifecycle hooks via the new hub), with an effect observable WITHOUT
chat, dialogue, combat, shops, trading, NPC drivers, or objects?
Commands are out (need chat). Player attributes were assumed
available in the survey brief; they are NOT: the Go `Player` has no
`get/setAttribute` (only save-level attributes preserved verbatim),
so any plugin touching attributes needs milestone 1.

Method: all 577 entries classified (285 self-contained, 292 shims).
Self-contained entries screened by token filters for calls with no Go
counterpart (`sendMessage`, `getPacketSender`, `getDialogueManager`,
`getCombat`, `registerCommand`, `onNpc*`, `onObject*`, `onItem*`,
`onCombat*`, `onInterface*`, `getSession`, `performAnimation`,
`get/setAttribute`, `persistAttribute`, `TaskManager`,
`getMovementQueue`, `moveTo`, shop/trade/area/timer calls): 8 strict
survivors and 19 relaxed survivors, every one read in full. The 13
smallest shims read in full (all delegate to units needing missing
systems). 9 smallest unit directories scanned. 4 startup-only
entries read. Whole-tree sweeps for login handlers that only add
items/grant XP, and for lifecycle-only files with no player calls
at all (6 files, all ruled out). ~40 files read in full.

Candidates examined (top 5 + instructive rejections):

| Plugin (lines) | register() body | Calls made | Verdict |
|---|---|---|---|
| `world/DateMinutes.plugin.js` (28) | `onPlayerLogin` + `onPlayerProcess` → `sendDateMinutes` | `player.getPacketSender().sendConfig(3078, minute)`; `dateMinutes()` pure helper from `combat/HomeTeleportSequence.js:194` | **RECOMMENDED, with one caveat (below)** |
| `items/GracefulOutfit.plugin.js` (137) | `registerRunEnergyRestoreModifier` + `equipment:effects` + `equipment:weights` listeners | `getEquipment`, `ItemDefinition.forId().getName()`, `skillcape:has-perk` query | PORTED (phase 22): seam built, `equipment:effects` has no Go producer and `skillcape:has-perk` no Go answerer — documented gaps |
| `items/WornWeights.plugin.js` (38) | `equipment:weights` listener | `getEquipment`, `ItemDefinition.forId().getWeight()` | PORTED (phase 22): same seam |
| `interface/IgnoreList.plugin.js` (12) | Empty `register(){}` | None | Out: no observable effect; proves nothing |
| `npcs/NpcSpawns.plugin.js` (23) | Two `registerDefinitionSource` calls | File read of `npc-spawns-custom.json` | Out: Go already loads `npc-spawns.json` as a built-in source; no distinct effect, and the hub has no definition-source API |
| `interface/BossKillCount.plugin.js` (32) | `onServerStartup` storing `ChatType.SPAM` | Helper `sendBossKillCount` uses `sendMessage` | Out: a shared helper for other plugins, not standalone content |
| `npcs/ZombiePirates.plugin.js` (49) | `npc-drops:table` listener | Emits `diary:is-complete` query | Out: needs the drops system + achievements plugin |
| `persistence/SaveEvents.plugin.js` (53) | Five custom-event listeners | `PLAYER_PERSISTENCE.requestSave` (coalescing NOT STARTED) | Out: needs unported save coalescing + quest/shop/drop producers |
| `areas/MorUiRek.plugin.js` (230) + `Names.MorUiRek.js` (36) | Dialogue conditions, `npc-dialogue:*`, `onItemOnNpc`, object interactions | Attributes, `getDialogueManager` via QuestRuntime, `TaskManager`, `moveTo` | Out: needs milestones 1, 3, 4, 5. The `Names` unit alone is portable logic, but it is not a plugin |
| `items/skillcapes/MetalAttraction.SkillCapes.js` | `onPlayerProcess` + login/logout state cleanup | `getAttribute`, `getInventory().adds`, shared `Common.SkillCapes` | Out: attributes + skillcape framework; the only login/XP/item-adding file in the tree without chat calls |

**Recommendation: port `world/DateMinutes.plugin.js`.** Its full
behaviour: on player login and on every player-process tick, compute
`floor(Date.now() / 60000)`; if it differs from the last minute sent
to that player, send varp 3078 with the new minute. Every call maps
to something in Go: the two lifecycle hooks are Phase 10 hub emits;
the minute computation is `time.Now().Unix() / 60`; the per-player
last-sent map is a plain Go map keyed by player (TS uses a `WeakMap`);
the send is `EncodeVarp` (`internal/protocol/state.go`, opcodes
40/41), already ported and byte-exact.

**The one caveat, stated plainly:** TS sends through
`player.getPacketSender().sendConfig`, and Go has no packet-sender
object and no runtime varp-sender method — `EncodeVarp` is used
today only at bootstrap and in the music flush block. The proof
port needs one of: (a) the proof package imports `internal/protocol`
and calls `player.Session.SendPacket(protocol.EncodeVarp(3078, m))`
(`SessionPort.SendPacket` exists; `Player.Session` is set at entry
in `cmd/server/server.go:562`) — a seam-direction exception the
coordinator must approve; or (b) a one-method `SendVarp` addition to
`SessionPort` + `Session`, reusing the existing encoder — new code,
but a sender method, not an engine system. If the coordinator
rejects both, NO real plugin qualifies strictly, and the hub must be
proven with a minimal test-only content package instead (per
`PHASE10_PLAN.md` §3). Do not force a fit by inventing a packet path.

Observable effect for the live smoke test: a VARP_LARGE frame
(opcode 41, id 3078, value = current epoch minute) on the wire at
login, and a fresh one when the minute rolls over; no second frame
within the same minute. Client-visible consequence: home/minigame
teleport cooldown timers display (cache scripts compare varp 3078
with varps 892/888).
