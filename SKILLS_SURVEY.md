# Skills survey

Phase 31, 2026-11-19. Analysis only: nothing was ported, no Go code
changed, no skill was picked. Tyler decided 2026-11-17: combat
leftovers first, then adding all skills. Phase 30 finished the first
combat-leftovers slice, so this survey ran. It recommends one skill,
with measured evidence. **The pick is Tyler's** — same as the Phase 25
quest survey, where Tyler picked Doric's Quest from a ranked
shortlist.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only checkout
at `~/tsps-bfd81e3`. Skill content lives at
`server/plugins/skills/`. All counts below were re-measured for this
survey; method is stated per table so every number is reproducible.

**Update, phase 35 (2026-11-21):** the §6 NPC-interaction engine gap
is partly closed — `onNpcInteraction` is now firable (57 of 108
families; `Api.OnNpcInteraction` + `Hub.EmitNpcInteraction`, name
hooks after the onNpcClick bindings), which is what Fishing needed.
`onBeforeNpcInteraction` / `onNpcsInteraction` /
`onAnyNpcInteraction` / `onNpcRoute` remain table-only, as does
`onItemOnNpc` (it blocks the AnglerOutfit Gita exchange). The §5
"Fishing — none" data-file row was confirmed correct by the full
Phase 35 read: Fishing has no data file and none was imported.

## 1. Measured inventory (reconciled with the plan)

Method: `find` + `wc -l` under `server/plugins/skills/`; true totals
from the `require()` transitive closure (a skill's real size is its
entry plugin + everything reachable via `require()`, since only
`*.plugin.js` files are auto-loaded — `PluginManager.
discoverPluginFiles`, `MAX_PLUGIN_DEPTH = 2` — and subdir units are
not). JSON counted separately.

| Measure | Count |
|---|---|
| JS files under `skills/` (whole directory) | 393 files, 77,898 lines (matches the plan exactly) |
| Entry plugins (`*.plugin.js`, incl. 2 in subdirs) | 29 |
| Top-level `*.plugin.js` files | 27 |
| Subdir entry plugins | 2 (`sailing/Sailing.plugin.js` 36 lines, `firemaking/BarbarianPyres.plugin.js` 266 lines) |
| Non-plugin units | `SkillActions.js` (30), `Spells.EnchantJewellery.js` (285) |
| Docs | `Farming.md`, `agility/README.md`, `hunter/README.md` |
| Data files (`data/*.json` + per-skill `data/` dirs) | 32 in `data/` (547,544 B) + 15 in `construction/data/` (148,238 B) + 16 in `sailing/data/` (387,986 B) |
| Median true total (of 29 entries) | 1,329 lines (ShootingStars) |
| Smallest / largest true total | 63 (Reanimation) / 20,617 (Sailing) |
| Sum of true lines across all 29 entries | 85,187 — above the 77,898 tree total because shared files (notably `SkillActions.js`, `ObstacleRunner`, the farming tree) are counted once per entry that requires them |

### Reconciled per-skill inventory

"True files/lines" = entry + transitively required JS under
`plugins/skills/` (JSON counted separately). "Members" = the entry
declares `members: true` (no flag = loads on F2P worlds;
`PluginManager` gates only on `=== true`).

| Entry plugin | Entry | True files | True lines | Members? |
|---|---|---|---|---|
| Reanimation | 63 | 1 | **63** | members |
| BoltEnchanting | 13 | 2 | **185** | members |
| BarbarianPyres | 266 | 1 | **266** | members |
| BoneAltars | 239 | 2 | **269** | members |
| AgilityDispenser | 304 | 1 | **304** | members |
| PrayerUnlocks | 314 | 1 | **314** | members |
| EnchantJewellery | 7 | 3 | **322** | F2P |
| CraftingSpells | 14 | 4 | **346** | members |
| Prayer | 355 | 2 | **385** | F2P |
| RalosRise | 19 | 6 | **607** | members |
| Ectofuntus | 25 | 7 | **747** | members |
| Herblore | 19 | 8 | **761** | members |
| Fletching | 36 | 6 | **965** | members |
| Firemaking | 10 | 3 | **1,236** | F2P |
| ShootingStars | 37 | 10 | **1,329** | F2P |
| Runecrafting | 1,165 | 4 | **1,918** | F2P |
| Cooking | 39 | 10 | **2,045** | F2P |
| Crafting | 31 | 15 | **2,088** | F2P |
| Smithing | 70 | 14 | **2,120** | F2P |
| Mining | 1,221 | 10 | **2,164** | F2P |
| Thieving | 22 | 10 | **2,591** | members |
| Fishing | 1,134 | 13 | **2,749** | F2P |
| Woodcutting | 1,594 | 14 | **3,220** | F2P |
| Slayer | 34 | 19 | **3,604** | members |
| Farming | 13 | 18 | **5,489** | members |
| Agility | 852 | 39 | **6,286** | members |
| Hunter | 127 | 41 | **9,927** | members |
| Construction | 13 | 54 | **12,270** | members |
| Sailing | 36 | 108 | **20,617** | members |

Convention note: true totals include `SkillActions.js` (30 lines)
where required. It is already ported in Go (Phase 20), so it adds
~0 lines to a Go port; the per-candidate Go estimates in §8 exclude
it. The 8 smallest entries (through CraftingSpells) have zero
external requires and zero TS-engine imports.

### True-size traps: subdirectory requires

| Entry | Entry | True | Trap |
|---|---|---|---|
| Hunter | 127 | 9,927 (78×) | `hunter/Herbiboar.Hunter.js` requires `farming/Guild.Farming.js`, which fans out to the **entire 16-file farming tree (5,446 lines)**. Hunter is really "Hunter + all of Farming". |
| Construction | 13 | 12,270 | Entry requires `Core.Construction` + `House.Construction`, pulling 53 of 54 construction files. One orphan: `construction/Retrieval.Construction.js` (36 lines) is required by **nothing anywhere in the whole server tree** — dead code as far as measurement can tell. |
| Thieving | 22 | 2,591 (118×) | Pulls `agility/ObstacleRunner.Agility.js` (474) via its Guild/SoulChest units. |
| Fishing | 1,134 | 2,749 | Pulls the same ObstacleRunner via `fishing/Guild.Fishing.js`. |
| Woodcutting | 1,594 | 3,220 | Pulls ObstacleRunner plus `farming/Redwood.Farming.js` (58) → `farming/Core.Farming.js` (9). |
| Sailing | 36 | 20,617 | Entry requires all 108 sailing files. (Sailing *engine* mechanisms were ported in Go Phase 21; this is the sailing *skill content*, still unported.) |
| Cooking / Crafting / Farming / Mining / Runecrafting / Smithing | 39 / 31 / 13 / 1,221 / 1,165 / 70 | 2,045 / 2,088 / 5,489 / 2,164 / 1,918 / 2,120 | Each entry is a thin registrar over its subdir, plus `SkillActions.js` in most cases. |

### Logic living outside `plugins/skills/`

Other-plugin requires (JS): Agility → `plugins/items/LightSources.
plugin.js`; Construction → `plugins/items/PoweredStaff.js`,
`plugins/items/Teleports.Jewellery.js`; Cooking →
`plugins/modes/FreeToPlay.plugin.js`; Crafting → `PoweredStaff.js`;
Fishing → `plugins/npcs/NpcDialogues.plugin.js`,
`plugins/quests/QuestRuntime.js`; Hunter →
`plugins/interface/ValueWarning.js`,
`plugins/items/ConsumableEffects.js`; Slayer →
`plugins/areas/SlayerTower.plugin.js` +
`plugins/bosses/data/grotesque-guardians.json`; Woodcutting →
`plugins/objects/ClimbLinks.js`; Sailing → `PoweredStaff.js`,
`plugins/npcs/random-events/Teleports.RandomEvents.js`,
`plugins/quests/QuestRuntime.js`; Smithing →
`plugins/quests/QuestRuntime.js` (via `smithing/Spears`).

TS-engine requires (`server/src/...`, direct `.ts` imports — not
plugin requires): the four thick entries import engine classes
directly — Fishing, Mining, Woodcutting (each: `CacheDefinitions`,
`WorldDefinition`, `Animation`, `Item`, `Skill`, `Task`, `IdEnums`,
plus `GameObject`/`MapObjects`, `Equipment`, `Sound`/`Sounds`);
Agility courses (14 files require `IdEnums`); Sailing is the
heaviest (dozens: `Sailing`/`Boat`/`BoatManager`/`BoatMovement`/
`HeadingUtils`, `Task`/`TaskManager`, `DialogueChainBuilder` +
dialogue-entry classes, `CacheDefinitions`, `ItemDefinition`,
`Mobile`, `World`, `ClientProtocol`, `PluginManager` itself).
Construction/Farming/Hunter touch `ExaminePrice.ts`. No other skill
imports TS directly.

## 2. Whole-tree lexical sweep

Method: lexical flag classification over all 29 entries' true file
sets, computed twice (Python `re.search` and `grep -rEl`) with exact
agreement on every count (0 mismatches over all 393 files, at both
file and skill level). Patterns:

| Group | Pattern |
|---|---|
| Hooks | `onNpcInteraction\|onItemAction\|onItemOnItem\|onItemOnNpc\|onObjectInteraction\|onItemOnObject\|onGroundItemPickup\|onNpcDeath\|onInterfaceActionButton\|onInterfaceAction\|onSpellOnItem\|onSpellOnNpc\|onSpellOnObject\|onCanAttack\|onCanEquip\|onPlayerLevelUp\|onExperienceGained\|onPlayerLogin` |
| SkillActions | `SkillActions\|StartSkillable\|actionInterval` |
| Dialogue | `onNpcDialogueVariant\|onNpcDialogueCondition\|npc-dialogue:` |
| Interface | `openInterface\|sendInterface` |
| Timer | `scheduleTask\|TaskManager\|\bTask\b` |
| Area | `registerArea\|onZoneEnter\|onZoneExit` |
| CreationMenu | `CreationMenu` |
| persistAttribute | `persistAttribute` |
| CustomEvent | `emitCustomEvent\|onCustomEvent` |

Flag counts (of 29 entries): CustomEvent 28, Timer 27,
persistAttribute 21, onObjectInteraction 19, onPlayerLogin 18,
onItemAction 18, onItemOnObject 16, onItemOnItem 16, SkillActions
14, Interface 13, onNpcInteraction 9, CreationMenu 8,
onInterfaceAction 8, Dialogue 8, onNpcDeath 7, onItemOnNpc 5,
onInterfaceActionButton 4, Area 3, onSpellOnObject 3,
onGroundItemPickup 3, onSpellOnItem 2, onCanEquip 2, onCanAttack 2.
**Zero use anywhere: `onPlayerLevelUp`, `onExperienceGained`,
`onSpellOnNpc` (0/29 each).** Note: the `onInterfaceAction`
pattern also matches `onInterfaceActionClick` /
`onInterfaceActionButton` as substrings, so its count of 8
overlaps the button count of 4; and `onInterfaceAction` itself
does not exist in TS (see §6).

Per-skill flags (true file set):

| Skill | Flags |
|---|---|
| Agility | Area, CustomEvent, Timer, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |
| AgilityDispenser | CustomEvent, Timer, onObjectInteraction, onPlayerLogin, persistAttribute |
| BoltEnchanting | CreationMenu, Timer |
| BoneAltars | CustomEvent, SkillActions, Timer, onItemOnObject |
| Construction | Area, CustomEvent, Interface, Timer, onInterfaceAction, onInterfaceActionButton, onItemAction, onItemOnItem, onItemOnNpc, onItemOnObject, onNpcDeath, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |
| Cooking | CreationMenu, CustomEvent, Interface, SkillActions, Timer, onInterfaceAction, onItemAction, onItemOnItem, onItemOnObject, onObjectInteraction, onPlayerLogin, persistAttribute |
| Crafting | CreationMenu, CustomEvent, SkillActions, Timer, onInterfaceAction, onItemAction, onItemOnItem, onItemOnObject, onObjectInteraction, persistAttribute |
| CraftingSpells | CustomEvent, Timer, onSpellOnObject |
| Ectofuntus | CustomEvent, Dialogue, Timer, onItemAction, onItemOnObject, onObjectInteraction, onPlayerLogin, persistAttribute |
| EnchantJewellery | CustomEvent, SkillActions, Timer, onSpellOnItem |
| Farming | CustomEvent, Dialogue, Interface, SkillActions, Timer, onGroundItemPickup, onInterfaceAction, onItemAction, onItemOnItem, onItemOnNpc, onItemOnObject, onNpcDeath, onNpcInteraction, onObjectInteraction, onPlayerLogin, onSpellOnObject, persistAttribute |
| Firemaking | CreationMenu, CustomEvent, Interface, SkillActions, Timer, onItemOnItem, onItemOnObject, onObjectInteraction |
| Fishing | CustomEvent, Dialogue, Interface, SkillActions, Timer, onItemAction, onItemOnItem, onItemOnNpc, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |
| Fletching | CreationMenu, CustomEvent, Interface, SkillActions, Timer, onItemAction, onItemOnItem, onPlayerLogin, persistAttribute |
| Herblore | CreationMenu, CustomEvent, Timer, onItemOnItem, onItemOnObject |
| Hunter | CustomEvent, Dialogue, Interface, SkillActions, Timer, onCanEquip, onGroundItemPickup, onInterfaceAction, onInterfaceActionButton, onItemAction, onItemOnItem, onItemOnNpc, onItemOnObject, onNpcDeath, onNpcInteraction, onObjectInteraction, onPlayerLogin, onSpellOnObject, persistAttribute |
| Mining | Area, CustomEvent, SkillActions, Timer, onItemAction, onItemOnItem, onObjectInteraction, onPlayerLogin, persistAttribute |
| Prayer | CustomEvent, Interface, SkillActions, Timer, onGroundItemPickup, persistAttribute |
| PrayerUnlocks | CustomEvent, onItemAction, onPlayerLogin, persistAttribute |
| RalosRise | CustomEvent, SkillActions, Timer, onItemAction, onItemOnItem, onItemOnObject, onObjectInteraction, onPlayerLogin, persistAttribute |
| Reanimation | CustomEvent, onNpcDeath |
| Runecrafting | CustomEvent, Dialogue, Interface, Timer, onItemAction, onItemOnItem, onItemOnObject, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |
| ShootingStars | CustomEvent, Dialogue, Interface, Timer, onItemAction, onItemOnItem, onItemOnObject, onObjectInteraction, onPlayerLogin, persistAttribute |
| Slayer | CustomEvent, Dialogue, Interface, Timer, onCanAttack, onInterfaceAction, onInterfaceActionButton, onItemAction, onItemOnItem, onItemOnNpc, onNpcDeath, onNpcInteraction, onPlayerLogin, persistAttribute |
| Smithing | CreationMenu, CustomEvent, Interface, SkillActions, Timer, onInterfaceAction, onInterfaceActionButton, onItemAction, onItemOnItem, onItemOnObject, onSpellOnItem, persistAttribute |
| Thieving | CustomEvent, Timer, onItemAction, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |
| Woodcutting | CustomEvent, SkillActions, Timer, onItemAction, onItemOnItem, onItemOnObject, onNpcDeath, onObjectInteraction, onPlayerLogin, persistAttribute |
| BarbarianPyres | CustomEvent, Timer, onItemOnObject, onObjectInteraction |
| Sailing | CreationMenu, CustomEvent, Dialogue, Interface, Timer, onCanAttack, onCanEquip, onInterfaceAction, onItemAction, onItemOnObject, onNpcDeath, onNpcInteraction, onObjectInteraction, onPlayerLogin, persistAttribute |

Caveat: flags are lexical, so comments/strings can match (notably
Timer, which matches the bare `Task` class imported by the thick
entries), and per-skill flags understate borrowed units (Hunter's
`onGroundItemPickup` / `onNpcDeath` / `onSpellOnObject` come only
from borrowed `farming/Patches.Farming.js`). Treat counts as
screening signals; every ranked skill below was read in full.

**SkillActions finding:** `SkillActions.js` (30 lines) exports only
`pauseForLevelUp` + `actionInterval` — **`StartSkillable` does not
exist anywhere in the skills tree (0 hits)**. 15 files in 13
entries require it (Hunter inherits it via borrowed farming files,
making 14 flagged). **Agility never uses it** (0 hits; it runs its
own `ObstacleRunner`). The shared `skills:action-interval` request
event is emitted only by `SkillActions.js` itself; outside skills
it is listened to by `modes/leaguerelics/FlowState` and emitted by
`modes/leaguerelics/Transmute`. Consequence: no candidate uses the
Phase 20 `StartSkillable` pattern — TS skills predate it and all
hand-roll per-player `Task` loops. The Go `skillactions` package
already covers what they actually use (`actionInterval`,
`pauseForLevelUp`).

## 3. Data-file map

All 32 `data/*.json` files are referenced by at least one skill
(a require-only scan initially missed 5 — `crafting.json`,
`crafting-spells.json`, `farming-data.json`, `pickpocketing.json`,
`woodcutting-guild.json` — because they are loaded via
`fs.readFileSync(path.join(__dirname, "..", "data", <name>))`, not
`require()`). Load mechanisms: `require()` for most; `readFileSync`
for the five above; sailing uses a `read()` helper loading from
`sailing/data/`.

| Skill | Data files read (sizes) |
|---|---|
| Agility | `data/agility-shortcuts.json` (142,580 B) |
| AgilityDispenser | `data/agility-dispenser.json` (4,479) |
| BoltEnchanting | `data/bolt-enchanting.json` (2,619) |
| BoneAltars | `data/bone-altars.json` (1,202) |
| Construction | 15× `construction/data/*.json` (148,238 B total) |
| Cooking | `data/cooking.json` (29,762) |
| Crafting | `data/crafting.json` (38,296) |
| CraftingSpells | `data/crafting-spells.json` (5,382, via `readFileSync`) |
| Ectofuntus | `data/bones.json` (7,426) |
| EnchantJewellery | none |
| Farming | `data/farming-data.json` (34,886) + `farming/data/geomancy-panels.json` (4,468) |
| Firemaking | none |
| Fishing | none |
| Fletching | `data/fletching.json` (37,615) |
| Herblore | `data/herblore.json` (49,732) |
| Hunter | `data/herbiboar-data.json` (1,383), `impling-loot.json` (6,255), `impling-spawns.json` (658), `lucky-loot.json` (52,315), `rumour-data.json` (4,627), `sack-loot.json` (4,032), `tracking-data.json` (8,797) + `data/farming-data.json` + `farming/data/geomancy-panels.json` (via borrowed farming tree) |
| Mining | none |
| Prayer | `data/ashes.json` (625) + `data/bones.json` (7,426) |
| PrayerUnlocks | none |
| RalosRise | `data/bone-altars.json` (1,202) + `data/bones.json` (7,426) + `data/ralos-rise.json` (4,927) |
| Reanimation | `data/bone-altars.json` (1,202) |
| Runecrafting | `data/ourania-altar.json` (1,993) + `data/runecrafting-ruins.json` (1,404) |
| ShootingStars | `data/shooting-stars.json` (10,428) |
| Slayer | `data/slayer-areas.json` (4,956), `slayer-finishers.json` (2,730), `slayer-gear.json` (4,204), `slayer-masters.json` (8,409), `slayer-superiors.json` (2,443) + `plugins/bosses/data/grotesque-guardians.json` (outside skills/) |
| Smithing | `data/smithing.json` (11,856) + `data/smithing-anvil-layout.json` (28,300) |
| Thieving | `data/pickpocketing.json` (29,817) |
| Woodcutting | `data/woodcutting-guild.json` (3,406 bytes, 72 lines) |
| BarbarianPyres | none (inline `PYRE_LOGS` table) |
| Sailing | all 16× `sailing/data/*.json` (387,986 B total) |

Reads from outside `plugins/skills/`: exactly one in code —
`sailing/crew/Lines.Crew.js:55` reads
`server/data/definitions/npc-dialogues.json` via
`GameConstants.DEFINITIONS_DIRECTORY` (crew transcript lines). All
other `definitions/` mentions in skills are comments.

Data-import note for port phases: the small tables
(`bones.json`, `bone-altars.json`, `bolt-enchanting.json`,
`herblore.json`, `fletching.json`, `cooking.json`,
`crafting.json`, `agility-dispenser.json`) are verbatim-import
candidates under the same SHA-256-checked precedent as the Phase
27/28 imports — each needs Tyler's approval before it is copied,
per the standing rule. No data file was imported by this survey.

## 4. Cross-dependency map

Shared units required across skills: `SkillActions.js` (15 files /
13 entries); `agility/ObstacleRunner.Agility.js` (474) — required
by `fishing/Guild.Fishing.js`, `thieving/SoulChest.Thieving.js`,
`woodcutting/Guild.Woodcutting.js`; `farming/Guild.Farming.js`
(512) → whole farming tree — required by
`hunter/Herbiboar.Hunter.js`; `farming/Redwood.Farming.js` (58) →
`Core.Farming.js` (9) — required by `Woodcutting.plugin.js`;
`smithing/Superheat.Smithing.js` requires `Smithing.plugin.js`
back (entry re-require, presumably shared constants).

Custom events (literal `emitCustomEvent`/`onCustomEvent` strings):
388 distinct names; 531 emit call sites, 305 listen call sites
(the only event mechanism in the tree — no other `.emit(` form).
**33 cross-skill emit→listen pairs** (emitter skill ≠ listener
skill); 102 events listened-to in skills with no skills producer;
201 emitted in skills with no skills listener (most answered by
engine/other plugins or nobody).

Top cross-skill events (producer → consumer):

- `skillcape:has-perk` — emitted by 9 skills (Cooking, Farming,
  Herblore, Hunter, Runecrafting, Slayer, Smithing, Thieving,
  Woodcutting); **no listener in skills**; answered externally by
  `items/skillcapes/Perks.SkillCapes.js`. (Go Phase 22 already
  noted it has no Go answerer.)
- `smithing:anvil-access` — emitted by Smithing; listened
  externally by `quests/DoricsQuest.Quest.js` (also emitted there
  by LegendsQuest).
- `diary:task` / `diary:is-complete` — emitted by 8 / 14 skills;
  answered externally by `achievements/AchievementDiaries.plugin.js`.
- `quest:is-complete` / `quest:get-stage` / `quest:stage-changed`
  — emitted by Agility, PrayerUnlocks, Runecrafting; answered
  externally by `quests/QuestRuntime.js` (unported in Go).
- `agility:lap`, `agility:obstacle-start` — Agility →
  AgilityDispenser.
- `bone-altars:offer` — BoneAltars + Construction → BoneAltars
  (also `prayer:bone`/`prayer:bury` internal to Prayer +
  BoneAltars; `prayer:bone` also emitted by a league mode
  externally).
- `cooking:raw-xp` — Fishing → Cooking. `crafting:spun` —
  Crafting → Fletching. `firemaking:log-xp`,
  `firemaking:xp-multiplier` — Woodcutting → Firemaking.
  `woodcutting:axe`/`chop`/`deplete-tree`/`validate-tree` —
  Farming/Hunter/Runecrafting ↔ Woodcutting. `mining:*` family
  (best-pickaxe, count-ore, extra-ore, level-boost, roll-interval,
  success, xp-multiplier) — Mining ↔ ShootingStars/Runecrafting.
  `farming:use-water` — Construction → Farming + Hunter.
  `seed-box:gather` — Thieving → Farming + Hunter.
  `hunter:rumours-completed` — Cooking → Hunter.
  `slayer:current-task` — Agility → Slayer; `slayer:has-unlock` —
  Fletching → Slayer. `shooting-stars:forecast` — Construction →
  ShootingStars. `npc-drops:add`/`generated` — Sailing/Hunter →
  Slayer/Farming/Hunter. `ground-items:take`/`taken` —
  Farming/Hunter → Prayer/Agility. `npc-dialogue:start` — emitted
  by 6 skills, answered by `npcs/NpcDialogues.plugin.js`.

Orphaned listeners (no producer found by literal search anywhere
under `server/`): `equipment:effects` — listened by 10 skills
(BoneAltars, Construction, Farming, Firemaking, Fishing, Hunter,
Mining, Runecrafting, Thieving, Woodcutting) plus combat/items
plugins; **no `emitCustomEvent("equipment:effects")` call exists
in any `.js`/`.ts` under `server/`** (the TS engine's
`Equipment.ts` emits only `equipment:changed`).
`level-up:popup-opened` — listened by 13 skills, no producer in
skills (engine `interface/SkillLevelUp` presumably).
`door:toggle` — emitted by `objects/Doors.plugin.js`, listened by
5 skills. `interface:closed` / `player:emote` /
`player:interaction-cleared` — engine/interface producers outside
skills.

Event producer/consumer mapping is by literal
`emitCustomEvent("name")` / `onCustomEvent("name")` strings;
dynamically constructed event names would be missed (none
observed; the one case where no literal producer exists,
`equipment:effects`, is flagged as such rather than assumed).

## 5. Go hook-surface mapping

**Hub-count correction (found by this survey, verified by the
coordinator):** `PORT_STATUS.md` and `MEMORY.md` said "53 of 108
hook families firable". That counts registration *methods* as
*families*. Go has 53 `Api.On*` methods, but two are the named
overloads of already-counted families —
`OnObjectInteractionNamed` (`internal/plugins/objects.go:469`)
and `OnItemOnObjectNamed` (`internal/plugins/objects.go:655`).
Distinct firable families = **51, not 53**. `hooks.go`'s own
header agrees ("Fifty-one get a registration method… The other
57 get NO registration method"; 51 + 57 = 108). Correct figures:
**51/108 families firable (53 methods), 57/108 table-only**.
Non-hook counts (29/95, 12/191) were not recounted in this phase.
`PORT_STATUS.md` is corrected in this phase; `MEMORY.md` is the
parent's record to correct.

The Go hub has 51 of 108 hook families firable
(`internal/plugins/hooks.go`, unfired-family table). Families the
skills tree needs, and their Go status (recounted for this
survey; TS line numbers are `PluginTypes.ts`):

Firable (usable today): `onObjectInteraction` (+ First–FifthClick,
`onObjectRoute`, `onBeforeObjectInteraction`, `onObjectClick`),
`onItemOnObject`, `onInterfaceActionClick`,
`onInterfaceActionButton`, `onPlayerLevelUp`, `onExperienceGained`,
`onCanGainExperience`, `onPlayerLogin`, `onPlayerLogout`,
`onPlayerDisconnect`, `onPlayerProcess`, `onCustomEvent`,
`onCommand`, `onNpcDeath`, `onPlayerDeath`, the combat hit
families, `onNpcDialogueVariant`, `onNpcDialogueCondition`,
`onZoneEnter`, `onZoneExit`, `onCanAttack`, `onSpellDisabled`,
`onSpellRuneBypass`, `onCanShop`, `onCanStockItem`, `onCanTrade`,
`onTradeRequest`, `onTradeCompleted`,
`onWorldEntityInteraction`.

Table-only (a skill needing one is blocked until its system
ports): `onNpcInteraction`, `onBeforeNpcInteraction`,
`onNpcClick` + First–FourthClick, `onNpcRoute`,
`onNpcsInteraction`, `onAnyNpcInteraction`, `onItemOnNpc`,
`onItemAction`, `onItemFirstAction`, `onItemOnItem`,
`onItemOnPlayer`, `onItemOnGroundItem`, `onGroundItemClick`,
`onGroundItemPickup`, `onSpellOnNpc`, `onSpellOnObject`,
`onSpellOnItem`, `onSpellCast`, `onCanTeleport`,
`onCanEquip`, `onCanUnequip`, `onCanBank`, `onCanBankItem`,
`onCanEat`, `onCanDrink`, `onCanUseItem`, `onFiremakingBlocked`,
`onPrayerDisabled`, `onSpecialAttackDisabled`,
`onPlayerBeforeDeath`, `onNpcBeforeDeath`, `onNpcHitModify`,
`onPlayerHitModify`, `onPlayerDealtDamage`, `onCanLogout`,
`onCanSpawnNpc`, `onWalkRoute`, `onPlayerMapSquareChange`,
`onButtonClick`, `onButton`, plus the region/pathfinding
families. `onInterfaceAction` does not exist in TS — only
`onInterfaceActionClick` and `onInterfaceActionButton` do.
`CreationMenu` has no Go type (Phase 20 gap).

Usage counts inside `plugins/skills/` (occurrence counts, not
per-skill flags): `onCustomEvent` 208, `onObjectInteraction` 170,
`onItemAction` 70, `onItemOnItem` 51, `onItemOnObject` 42,
`onPlayerLogin` 32, `onInterfaceActionClick` 27,
`onNpcInteraction` 26, `onSpellCast` 4, `onItemOnNpc` 4,
`onItemFirstAction` 4, `onGroundItemPickup` 3, `onGroundItemClick`
3, `onSpellOnObject` 2, `onSpellOnItem` 2, `onNpcRoute` 2,
`onNpcsInteraction` 7, `onAnyNpcInteraction` 2, `onItemOnPlayer`
1, `onItemOnGroundItem` 1, `onCanUseItem`/`onCanUnequip`/
`onCanTeleport`/`onCanEquip` 1 each, `onNpcFirstClick` 1,
`onButtonClick` 1, `onPlayerMapSquareChange` 2. **Zero use
anywhere: `onPlayerLevelUp`, `onExperienceGained`,
`onSpellOnNpc`.** No skill registers for its own level-ups — the
level-up feedback is `SkillLevelUp` content (§6), not a hook the
skills register.

## 6. The shared engine a first skill needs

`SkillManager` is live (full, Phase 9 + Phase 20 XP-order port);
the `SkillActions.js` pattern (`PauseForLevelUp`,
`ActionInterval`) is ported exactly; the skill slot +
`StartSkillable`/`StopSkillable` exist; world XP rate, XP lock,
`skills:current-level-changed` / `skills:restore` producers, and
the three skills hub families exist. What is still missing, each
grounded in TS and sized:

- **NPC interaction hook family.** `onNpcInteraction` /
  `onBeforeNpcInteraction` / `onNpcClick` + First–FourthClick /
  `onNpcRoute` / `onNpcsInteraction` / `onAnyNpcInteraction` —
  registration, emit, and NPC-click routing to the hub.
  Grounded in `PluginTypes.ts:1060–1091,1167–1184`,
  `PluginManager.ts:1235` (`emitNpcRoute`), `:1246`
  (`emitNpcInteraction`), `:2944/:4042–4055` (click-hook
  registration — `onNpcClick` has no dedicated `emitNpcClick`;
  the port must carry the registration machinery, not just an
  emit). Needed by 26 `onNpcInteraction` uses in the skills tree
  (Agility, Fishing, Hunter, Runecrafting, Farming patches,
  Construction estate agent/servants, Sailing shipwright/crew/
  charting). Size: roughly 150 lines of TS PluginManager
  registration/emit plus the NPC-option listener routing (Go
  `listeners/npc.go` today routes only the attack option to
  `onNpcAttackOption`); a full engine slice, since the walk-to-NPC
  path must also route.
- **Item-action family + `onCanUseItem` gate.**
  `onItemAction` / `onItemFirstAction` emits plus the
  `emitCanUseItem` gate consulted before an item action runs.
  Grounded in `PluginTypes.ts:1136,1213–1221`,
  `PluginManager.ts:1654` (`emitCanUseItem`), `:2016`
  (`emitItemAction`); Go `listeners/itemaction.go` exists but
  drops every plugin action with `ReasonItemAction` and calls no
  hub emit. Needed by 70 `onItemAction` uses (Herblore cleaning,
  Fletching recipes, Mining clue geodes/crystal pickaxe, Prayer
  unlocks, Hunter, Cooking recipes). Size: ~80 lines of TS
  emit/registration plus listener wiring; the equip fallback for
  first-action wieldables already exists in Go.
- **Item-on-item / item-on-player / item-on-ground-item.**
  `onItemOnItem` (two overloads), `onItemOnPlayer`,
  `onItemOnGroundItem` families with their emits and the
  item-on-item listener path. Grounded in
  `PluginTypes.ts:1197–1207`, `PluginManager.ts:1925`
  (`emitItemOnItem`), `:1939`, `:1946`; Go `listeners/useitem.go`
  handles only item-on-object today. Needed by 51 `onItemOnItem`
  uses (Cooking recipes/leaping fish/wine, Crafting jewellery/
  gems/glassblowing/battlestaves, Hunter) plus one use each of the
  player/ground-item variants. Size: ~100 lines of TS
  emit/registration plus a new listener branch mirroring the
  existing item-on-object path.
- **Item-on-NPC.** `onItemOnNpc` family, emit, and the
  use-item-on-NPC listener routing. Grounded in
  `PluginTypes.ts:1206`, `PluginManager.ts:1932`. Needed by 4
  skills-tree uses: Hunter, Farming patches, Slayer finishers,
  Fishing angler outfit, Construction servants (5 files). Size:
  small — one emit (~10 lines TS) plus listener routing comparable
  to item-on-object; the family is the blocker, not the volume.
- **Ground-item click/pickup.** `onGroundItemClick` /
  `onGroundItemPickup` families with emits from the pickup
  listener. Grounded in `PluginTypes.ts:1188,1193`,
  `PluginManager.ts:1908` (`emitGroundItemPickup`); Go
  `listeners/pickup.go` and `GroundItemManager` exist but call no
  hub emit. Needed by 3+3 uses (Hunter, Prayer, Farming patches,
  Firemaking logs). Size: small — one emit plus wiring the existing
  pickup path into the hub.
- **Spell-on-X + `onSpellCast`.** `onSpellOnObject` /
  `onSpellOnNpc` / `onSpellOnItem` / `onSpellCast` families, fired
  from the MagicOn* listeners and the cast path. Grounded in
  `PluginTypes.ts:1208–1212`, `PluginManager.ts:1960–1987`.
  Needed by 4 `onSpellCast` uses (BoltEnchanting, Lunar crafting
  spells, Sailing boat teleport/summon) and 2+2 spell-on-object/
  item uses. Size: ~60 lines of TS emits plus wiring into the
  Phase 27 spell listeners (standard book exists; the hub emits do
  not).
- **Consumable & equipment gates.** `onCanEat` / `onCanDrink` /
  `onCanUseItem` / `onCanEquip` / `onCanUnequip` gate families,
  consulted at the eat/drink/use/equip/unequip call sites.
  Grounded in `PluginTypes.ts:1112,1116,1135–1136,1143`,
  `PluginManager.ts:1404,1442,1627,1654,1706`; Go equip listener
  exists but calls no hub gate, and no eat/drink content exists.
  Needed by one skills-tree registration each (Agility, Hunter,
  Prayer, Sailing cargo tasks) but by nearly every future skill
  that gates equipment or consumables. Size: five small gate emits
  (~15 lines TS each) plus one call site per gate — a small engine
  slice with wide future use.
- **`onCanTeleport` (needs TeleportHandler first).** The teleport
  gate family; grounded in `PluginTypes.ts:1110`,
  `PluginManager.ts:1379`. Needed by one skills-tree registration
  (Sailing boat teleport path) and by any skill that teleports.
  Size: the gate itself is ~15 lines of TS, but `hooks.go`
  records TeleportHandler as NOT STARTED, so the real cost is the
  teleport engine the gate hangs off — this cannot be a standalone
  hook port.
- **`onCanBank` / `onCanBankItem`.** Bank-open and bank-item gates.
  Grounded in `PluginTypes.ts:1122–1123`,
  `PluginManager.ts:1510,1524`; Go has a Bank container but no
  bank-open flow. No confirmed skills-tree registration for
  `onCanBank` in this survey (the single combined
  `onCanEat`/`onCanDrink`/`onCanBank` hit in the tree is the
  Sailing cargo task, an eat/drink gate). Size: two small gate
  emits plus the bank-open flow they gate — blocked on the bank
  interface flow, not on the hooks themselves.
- **CreationMenu.** The `CreationMenu` type (title + item list +
  action + options: labels, max/last amount, mode/clientscript
  arg), `Player.setCreationMenu`, and the `sendCreationMenu` /
  `closeCreationMenu` display path, plus clearing it from
  `StopSkillable`. Grounded in `CreationMenu.ts` (97 lines),
  `Player.ts:1602`, `PacketSender.ts:1370–1418`, `SkillManager.ts:
  648`. Needed by 12 skill files: Cooking (recipes, churn,
  cookers), Crafting shared, Fletching shared, Herblore mixing,
  Smithing (anvil, smelting, spears), Firemaking logs,
  BoltEnchanting, Sailing shipwright workbench. Size: ~97 lines
  for the type plus the PacketSender display path and the
  entered-amount listener that executes the chosen action — a
  compact, well-bounded engine piece.
- **SkillLevelUp feedback (content, not engine).** The level-up
  chat message, graphic (anim 199 / 1388 at 99), two area sounds,
  jingle selection per skill/level, group-233 pop-up with Continue,
  unlock-box dialogues per level, total-level and combat-level
  follow-ups, and the `level-up:popup-opened` emit that pauses
  skilling. Grounded in `SkillLevelUp.plugin.js` (444 lines) plus
  `data/level-up-messages.json` and `data/level-up-pictures.json`.
  Needed by every skill (it listens to the already-firable
  `onPlayerLevelUp`); its own blockers are `onCanUseItem` (above)
  and the `quest:is-complete` event (quests deferred). Size: 444
  lines + 2 data files — the largest single missing skill piece,
  but pure content once the gate exists. Not verified in this
  survey: whether Go produces the `player:world-input` /
  `interface:closed` custom events SkillLevelUp listens to —
  check before sizing it as "only needs `onCanUseItem`".
- **Ground-item manager content seam + `forceAdd` floor overflow.**
  The `getItemOnGroundManager` PluginApi member (and Core
  accessor) so content can spawn ground items, plus
  `ItemContainer.forceAdd` dropping inventory overflow at the
  player's feet. Grounded in `PluginTypes.ts` (member;
  `core.go:53` records it table-only), `ItemContainer.ts:840–862`,
  `Agility.plugin.js:427–507` (course-mark rewards),
  `Mining.plugin.js:1027` (ore overflow via `registers`). Go has
  the manager (`grounditems.go:603`) and plain `Add` (which
  silently loses overflow, `container.go:1007+`) but no seam
  between them (`container.go` header). Size: small — one hub
  member plus the `forceAdd` path (~25 lines TS) wired to the
  existing manager.
- **NPC spawn seam (`spawnNpc` / `removeNpc`).** Content-facing
  spawn/remove members for one-off plugin NPCs. Grounded in
  `PluginTypes.ts` (members; `hooks.go` records them table-only);
  Go spawns NPCs only from definitions/spawns, never from
  content. Needed by BarbarianPyres (ferocious spirit, mangled
  bones only — deferrable) and by any future content that spawns
  NPCs. Size: small — two members over the existing NPC spawn
  path; the identifier resolves by gameval name
  (`CacheDefinitions.GetNpcId`, `internal/cache/facade.go:766`).
- **`skills:produced` / `skills:level-boost` /
  `skills:actions-per-cycle` producer pattern (content pattern,
  not engine).** The per-action request objects each skill emits
  after producing something (`{player, skill, itemId, amount, xp,
  kind}`), after computing a boost, and per cycle — so diaries,
  league tasks, and league relics can react. Grounded in
  `cooking/Common.Cooking.js:123–130`,
  `cooking/Churn.Cooking.js:88`, `cooking/Cookers.Cooking.js:589`;
  27/13/24 uses across the tree; consumers are
  `FragmentProduction.LeagueMode.js:215–216`,
  `ProductionProdigy.LeagueRelics.js:43–45`, achievement diaries,
  and league tasks. Size: ~10 lines per skill port (emit calls
  inside each skill's `Common` module); no engine work — the
  custom-event surface already exists — but every skill port must
  replicate the pattern or diary/league content stays blind.

Shared gaps worth fixing once for all skills: emitting
`level-up:popup-opened` on level-up (dead in 4 candidates),
emitting `player:interaction-cleared` (dead in 2 — Go never emits
it; `player:interaction-cleared` listeners in BoneAltars and
Firemaking are dead until then), and the `CreationMenu` type
(blocks 4 candidates). The `onItemOnItem` / `onItemFirstAction` /
`onItemAction` listener emit sites would unblock Herblore,
Fletching, and Firemaking's core loops together — one engine
slice, three skills.

## 7. Ranked shortlist (all read in full)

Ranking criteria, in plan order: smallest, no new engine systems,
no cross dependencies, uses only firable hook families, F2P
preferred. Estimated port size = skill content only, excluding the
already-ported `SkillActions.js` (30 lines) and any shared engine
subset. **Only #1 fully meets the bar; #2 meets it with one
data-sized seam.** #3–#5 are ordered by what their shared blocker
unlocks, and are labelled as blocked — they are "next in line",
not "ready now".

### 1. BarbarianPyres — 266 lines — RECOMMENDED

- Files: `skills/firemaking/BarbarianPyres.plugin.js` (266, sole
  file; no requires at all). No data file — the 14-row
  `PYRE_LOGS` table is inline. Members-only. A sub-plugin of
  Firemaking (own `module.exports`, `name: "BarbarianPyres"`),
  not required by `Firemaking.plugin.js`.
- What it does: Barbarian funeral pyres by Otto's Grotto — an axe
  and logs carve a pyre site (25286) into a pyre boat (25265),
  chewed/mangled bones go on it (25266), a tinderbox or bow
  lights it (25267), it floats (25293) and resets; Crafting XP
  for carving, Firemaking XP for lighting (magic logs need 85
  Crafting and 85 Firemaking). Mangled bones spawn a ferocious
  barbarian spirit that expires after ~3 minutes; chewed bones
  on a magic-log pyre emit the diary-task event.
- Hooks, all firable: `onObjectInteraction("Pyre site",
  {Construct})`, `onItemOnObject` ×2 (logs on site, bones/
  tinderbox on boat), `onPlayerDisconnect` (resets the
  player's pyres). No custom-event listeners at all; one
  emit-only event (`barbarian-pyre:built`, diary consumer
  unported — harmless).
- No CreationMenu, no dialogue, no interface, no timer beyond
  the `TaskManager` tasks (exists in Go), no areas, no
  cross-skill dependencies, no external requires, no TS-engine
  imports.
- Blockers/seams: (a) **none for the core loop** — the carve →
  load → light → float → reset loop runs entirely on firable
  hooks plus existing Go systems (`ObjectManagerAPI.Register`,
  `internal/plugins/objects.go:271`, TaskManager, SkillManager,
  `Mobile.IsRegistered`, `GameObject.
  IsWithinInteractionDistance`); (b) the ferocious-spirit spawn
  needs the `spawnNpc`/`removeNpc` content seam (table-only
  members — §6), used only for the mangled-bones branch, so it
  can be deferred and the spirit left as a documented gap; the
  spirit's id resolves by gameval name via
  `CacheDefinitions.GetNpcId`.
- Estimated port size: ~220–260 lines of Go content (one file).
  The smallest faithful skill port available, and the only
  candidate whose core loop needs no new engine system and no
  data import.

### 2. BoneAltars — 269 lines

- Files: `skills/BoneAltars.plugin.js` (239) + `SkillActions.js`
  (30, already ported). Data: `data/bone-altars.json` (23 lines,
  1,202 B; 1 altar, zealot 4 pieces) + `data/bones.json` (76
  lines, 7,426 B; 37 bones, 31 bonemeal) — shared with
  Ectofuntus, Prayer, RalosRise. Members-only.
- What it does: Offering bones on altars — bone-on-altar starts
  an immediate offering then repeats every 4 ticks while that
  bone lasts; the Chaos Temple altar (level 38 Wilderness,
  2947,3820,0) pays 3.5× burying XP and spares the bone 50% of
  the time; Zealot's robes give a second spare roll (1.25%/
  piece); any prayable altar also works for content-allowed
  non-bone offerings (`bone-altars:offering`); other plugins'
  altars (Construction house altars) can offer through the
  `bone-altars:offer` event.
- Hooks: `onItemOnObject` (`BoneAltars.plugin.js:233`) —
  **FIRABLE**; `onCustomEvent` ×4 — `level-up:popup-opened`
  (`:234`, dead in Go — never emitted), `bone-altars:offer`
  (`:235`, producer unported Construction — graceful),
  `player:interaction-cleared` (`:236`, **never emitted in Go**),
  `equipment:effects` (`:237`, **FIRABLE** — Go emits it; adds
  Zealot's set text 27).
- SkillActions: `pauseForLevelUp` + `actionInterval` — Go
  equivalents exist; loop is a hand-rolled `Task`
  (`keepOffering`), not `StartSkillable`.
- Dialogue/interface/timer/area: none; no CreationMenu. Uses
  `performAnimation`/`performGraphic` (Go: `Mobile.
  PerformAnimation`/`PerformGraphic`, Phase 29), `sendGlobalGraphic`
  (Go `internal/session/graphic_sender.go:49`), `sendSound`,
  `setPositionToFace` (only `NPC.SetPositionToFace` was confirmed
  in Go — **verify the Player form at port time**), `sendVarbit`
  n/a.
- Cross-deps: **Prayer plugin's `prayer:bone` query** — `boneFor()`
  emits it and returns null unanswered; the Prayer plugin answers
  it from `bones.json` (`Prayer.plugin.js:112,348`). No Go
  answerer exists. The port must carry a small `prayer:bone`
  answerer over `bones.json` (37 rows) — data, not an engine
  system — or the offering loop never starts. Construction house
  altars and league relics are graceful-absent.
- Blockers/seams: (a) `prayer:bone` answerer (~40 lines) over
  `bones.json`, importable verbatim under the Phase 27/28
  SHA-256-checked precedent — **the import needs Tyler's
  approval**; the same file then serves Ectofuntus (#3) and
  Prayer next; (b) interaction-clear stop — Go never emits
  `player:interaction-cleared`, so the offering loop needs a
  Go-native cancellation (walk/click stop; without it the loop
  runs until bones exhaust even after walking away); (c) verify
  `Player.setPositionToFace` at port time.
- Estimated port size: ~200–230 lines (content only) + ~40 lines
  for the `prayer:bone` answerer.

### 3. Ectofuntus — 747 lines (584 without the ectophial)

- Files: `skills/Ectofuntus.plugin.js` (25) + `ectofuntus/` (6
  files): `Common` 144, `Grinder` 205, `Ectophial` 138,
  `Disciples` 102, `Worship` 91, `Slime` 42. Data:
  `data/bones.json` (76 lines; shared with BoneAltars/Prayer/
  RalosRise). Members-only.
- What it does: Prayer training at the Ectofuntus in Port
  Phasmatys — fill buckets with slime from the pool, grind bones
  to bonemeal in the 3-stage loader/handle/bin choreography
  (persistent per-player hopper/bin state via varbits), worship
  slime + bonemeal for 4× burying XP + 5 ecto-tokens, claim
  tokens / earn the bonecrusher from ghost disciples via
  dialogue, and the ectophial teleport (pour → 3-tick teleport
  → kneel → refill) with refill damage immunity.
- Hooks: `onItemOnObject` ×5 — **FIRABLE**; `onObjectInteraction`
  ×3 — **FIRABLE**; `onNpcDialogueVariant` +
  `onNpcDialogueCondition` + `npc-dialogue:line` — **FIRABLE**;
  `onPlayerLogin` — **FIRABLE**; `persistAttribute` ×3
  (`ectofuntus:tokens`, `grinder-status`, `grinder-bone`,
  default 0) — **FIRABLE**; `onItemAction` (`Ectophial.
  Ectofuntus.js:134`, "Ectophial" Empty) — **FIRABLE** since
  phase 32 (the `onItemAction` family became firable then;
  the survey's TABLE-ONLY label is stale);
  `onPlayerHitModify` (`:136`, refill immunity) —
  **TABLE-ONLY**; `onCustomEvent("combat:damage-apply")` (`:137`)
  — registration firable but **no Go producer**.
- SkillActions: `Common.run` hand-rolled `Task`; no
  `pauseForLevelUp`/`actionInterval` import.
- Dialogue/interface/timer/area: dialogue variants + conditions
  for Ghost disciple (data need: variants
  `without-a-ghostspeak-amulet`, `claiming-tokens`,
  `after-completing-the-hard-morytania-diary` — staged as
  fixtures per the Phase 23 precedent, not copied); `mesbox` in
  Worship; varbit sends (Go `SendVarbit` exists); no
  CreationMenu; no areas; tick choreography is all inside
  `Common.run` tasks + the ectophial's own `Task` (movement block
  via `MovementQueue.SetBlockMovement`, which exists in Go).
- Cross-deps: `diary:is-complete` (hard Morytania for bonecrusher
  — no producer; graceful, bonecrusher variant never plays);
  bank check in `ownsBonecrusher` (`player.getBanks?.()` — Go has
  a `Bank` type but no confirmed Player `getBanks` accessor;
  verify at port time — the `?.` means it degrades to "not
  owned"); `ectofuntus:worshipped` emit-only.
- Blockers/seams: (a) **Ectophial teleport needs a TeleportHandler
  seam** — `TeleportHandler.checkReqs/confirm` with the level-20
  Wilderness rule; no Go equivalent; (b) refill immunity needs
  `onPlayerHitModify` (table-only) and a `combat:damage-apply`
  producer; (c) dialogue data need (Ghost disciple page). The
  ectophial unit (138 lines) can be deferred; the other four
  units + Common = 584 TS lines.
- Estimated port size: ~600–700 lines full; ~500 lines with the
  ectophial deferred.

### 4. Herblore — 761 lines

- Files: `skills/Herblore.plugin.js` (19) + `herblore/` (7 files):
  `Common` 252, `Mixing` 218, `Grinding` 115, `Coconut` 61,
  `GuthixBalance` 41, `Cleaning` 39, `Events` 16. Data:
  `data/herblore.json` (234 lines, 49,732 B; 15 herbs, 200
  recipes, 11 grinding rows). Members-only.
- What it does: The full Herblore skill — cleaning grimy herbs
  (click, instant, random clean sound), mixing potions via
  item-on-item with a make-many `CreationMenu` (unfinished →
  secondary → upgrades, 1–3 tick batches, cape-cleaning path,
  barbarian mixes), grinding secondaries with pestle & mortar
  (0/2/3-tick loop, crystal-shard quantity prompt), coconut
  splitting/milking, and silver-dust grinding on the Ectofuntus
  grinder. Druidic Ritual gates all use.
- Hooks: `onItemFirstAction` (`Cleaning.Herblore.js:38`) —
  **FIRABLE** since phase 32 (part of the `onItemAction`
  family); `onItemOnItem` ×3 (`Mixing.Herblore.js:217`,
  `Grinding.Herblore.js:114`, `Coconut.Herblore.js:60`) —
  **FIRABLE** since phase 33; `onItemOnObject` ×1 (`GuthixBalance.
  Herblore.js:38`) — **FIRABLE**; `onCustomEvent` ×3 answerers
  (`Events.Herblore.js:13-15`: `herblore:clean/unfinished/herbs`)
  — **FIRABLE**.
- SkillActions: `Common.run` is a hand-rolled per-click `Task`;
  no `StartSkillable`. No `pauseForLevelUp`/`actionInterval`
  import at all — Herblore is the only candidate using neither
  (its loops are short and click-driven). `Common` does use
  `skills:level-boost` / `skills:actions-per-cycle` custom-event
  queries (no Go producers; defaults 0/1, graceful).
- Dialogue/interface/timer/area: `CreationMenu` in Mixing
  (`Mixing.Herblore.js:188,192`) — **no Go type**; entered-amount
  prompt in Grinding (sender exists in Go); `mesbox`
  (DialogueChainBuilder + StatementDialogue — Go has the builder,
  but no `StatementDialogue` type was found; verify at port
  time); no timers/areas beyond the `Task` loops.
- Cross-deps: Druidic Ritual (`quest:is-complete` — graceful:
  `Common.questDone` treats unanswered as allowed, so Herblore
  works without the quest system); Barbarian Training
  (`herblore:unlocked` — unanswered = allowed); `diary:is-complete`
  (grinding bonus — unanswered = no bonus); `skillcape:has-perk`
  (no Go answerer, false default — Phase 22 precedent); Zahur
  (consumer of `herblore:clean/unfinished`, unported, harmless);
  league relic hooks (`herblore:mix/made`, `skills:produced` —
  emit-only).
- Blockers/seams: (a) `onItemOnItem` + `onItemFirstAction`
  families — new engine systems (item-use listener emit sites);
  (b) `CreationMenu`. Portable today without those: only
  GuthixBalance (one `onItemOnObject` handler) + the three Events
  answerers.
- Estimated port size: ~650–750 lines (content only); ~60 lines
  for the portable GuthixBalance + Events slice alone.

### 5. Fletching — 965 lines

- Files: `skills/Fletching.plugin.js` (36) + `fletching/` (4
  files): `Shared` 644, `Spool` 186, `Recipes` 34, `Diaries` 35.
  Data: `data/fletching.json` (235 lines, 37,615 B; 170 recipes,
  13 groups, 4 diary tasks). Members-only.
- What it does: The full Fletching skill — every Wiki recipe
  (bows, arrows, bolts, darts, shields, crossbows, tipped/
  javelin/chisel work) started by item-on-item pairs (tool-on-
  material or material-on-material), with make-many
  `CreationMenu`s, per-recipe tick pacing (incl. fletching-knife
  −1 tick, jeweller's-chisel pace), partial-set handling, random
  yields, plus the bow-string spool (account-stored strings,
  capacity tiers 250→100,000, two persist attributes + varbits)
  and diary-task credits for fletching in specific places.
- Hooks: `onItemOnItem` ×3 — **FIRABLE** since phase 33
  (`Recipes.
  Fletching.js:28`; `Spool.Fletching.js:181-182`); `onItemAction`
  ×2 — **FIRABLE** since phase 32 (`Recipes.Fletching.js:29` "Celastrus
  bark" Fletch; `Spool.Fletching.js:180` spool Fill/Check/Empty);
  `onPlayerDisconnect` (`Recipes.Fletching.js:30`) —
  **FIRABLE**; `onPlayerLogin` (`Spool.Fletching.js:179`) —
  **FIRABLE**; `persistAttribute` ×2 (`Spool.Fletching.js:
  177-178`: `fletching:spool-charges`, `fletching:spool-spares`)
  — **FIRABLE**; `onCustomEvent` `level-up:popup-opened`
  (`Recipes.Fletching.js:31`) — **dead in Go** (never emitted);
  `fletching:shafts` answerer (`Fletching.plugin.js:34`) and
  `fletching:made` listener (`Diaries.Fletching.js:32`) —
  **FIRABLE**; `crafting:spun` listener (`Spool.Fletching.js:183`)
  — no producer (Crafting unported; harmless).
- SkillActions: `actionInterval` (`Shared.Fletching.js:441`) +
  `pauseForLevelUp` (`Shared.Fletching.js:469`) — Go equivalents
  exist; the loop itself is a hand-rolled `FletchingTask extends
  core.Task` with resume-animation support, not `StartSkillable`.
- Dialogue/interface/timer/area: `CreationMenu` is the core loop
  UI (`Shared.Fletching.js:547`) — **no Go type**; spool Empty
  uses an `OptionDialogue` (Go dialogue builders exist; verify
  `OptionDialogue` at port time); varbit reads (Go varbit store
  exists); no areas.
- Cross-deps (note the asymmetry with Herblore): quest/slayer
  gating **defaults to LOCKED when unanswered** — `questState`
  returns `request.complete === true` and `hasSlayerUnlock`
  requires `request.unlocked === true` (`Shared.Fletching.js`),
  so quest-/Slayer-gated recipes are silently unavailable until
  the quest/Slayer systems exist. Ungated recipes are unaffected.
  `diary:task` emit has no consumer (diary system unported;
  harmless). Woodcutting league relic consumes `fletching:shafts`
  (unported; harmless).
- Blockers/seams: (a) `onItemOnItem` + `onItemAction` families —
  new engine systems; (b) `CreationMenu`. Portable today: Spool
  login/varbit restore + persist attributes, and the Diaries
  `fletching:made` listener (which only fires once Shared's loop
  exists).
- Estimated port size: ~750–850 lines (content only); Shared alone
  is ~550 of it.

## 8. Ruled out (read in full, with reasons)

| Skill | True size | Reason |
|---|---|---|
| BoltEnchanting | 185 | Smallest, but hard-blocked: its only entry point is `onSpellCast` (table-only, `Cast.BoltEnchanting.js:169`) and it needs `CreationMenu` (`:138`). The spellbook entry can never fire. |
| EnchantJewellery | 322 | Only entry point is `onSpellOnItem` (table-only, `Spells.EnchantJewellery.js:283`); spell-on-item dispatch is NOT STARTED (`internal/listeners/interfaceaction.go:59`). Mixed F2P/members (Lvl-1..5 F2P, Lvl-6/7 members). |
| Reanimation | 63 | Uses only firable hooks (`onNpcDeath` + `reanimation:head-consumed` listener), but has zero behaviour without unported ArceuusItemSpells — the only spawner of reanimated NPCs and the only emitter of `reanimation:head-consumed`. The port would be dead code until then (gap 14: Ancients/Lunar/Arceuus books NOT STARTED). |
| Prayer | 385 | F2P and small, but its core loop is blocked on `onItemFirstAction` (`Prayer.plugin.js`, buryBone + scatterAshes), `onGroundItemPickup`/`onGroundItemClick` (ground-only bones), and `onCanUseItem` (burial interrupt) — all table-only. Portable today: only the `prayer:bury` / `prayer:bone` custom-event answerers — which is exactly the seam BoneAltars (#2) needs, so they ride with that port instead. Also needs `player.getClickDelay()` (no Go equivalent found) and a `CountdownTask` equivalent (no Go type; a plain `Task` adapts). |
| AgilityDispenser | 304 | Core loop cannot function without Agility: `agility:lap` / `agility:obstacle-start` have no Go producers (Agility unported, 6,286 lines), so no ticket is ever earned. Also needs the ground-item overflow seam (`ItemOnGroundManager.registerNonGlobals` — `getItemOnGroundManager` table-only), a `StatementDialogue` type (none found in Go), and `player.getBanks` (no Go accessor found). |
| PrayerUnlocks | 314 | Blocked on `onItemAction` (scroll reading, table-only) and `onPrayerDisabled` (table-only; prayer systems NOT STARTED), plus `quest:is-complete` / `quest:stage-changed` (no Go producers). The varbit/attribute sync half is portable, but no scroll can ever be read. |
| CraftingSpells | 346 | Blocked on `onSpellCast` (Lunar, `Lunar.CraftingSpells.js`) and `onSpellOnObject` (ChargeOrbs, table-only), plus `inventory.forceAdd` (superglass glass overflow — no Go equivalent) and `diary:is-complete` (Tan Leather gate — no producer). |
| Firemaking | 1,236 | Largest and most blocked — three table-only families (`onItemOnItem`, `onItemOnGroundItem`, `onGroundItemClick`) are the entire lighting entry path, plus `CreationMenu` (tend menu) and a ground-item content seam (register/get/deregister for content). Only campfire tend/check is portable today. Its sub-plugin BarbarianPyres (#1) is the portable slice. |

## 9. Recommendation

**Port BarbarianPyres first.** It is the smallest viable skill
content (266 lines), uses only firable hook families
(`onObjectInteraction`, `onItemOnObject`, `onPlayerDisconnect`),
needs no `CreationMenu` and no new engine system for its core
loop, needs no data import (inline table), has no cross-skill
dependencies, and exercises the machinery every later skill needs
(object interaction + item-on-object + object swapping +
`TaskManager` + XP grants through the live SkillManager). Its one
deferrable branch — the ferocious-spirit spawn on mangled bones —
needs the `spawnNpc`/`removeNpc` content seam (§6), which is small
engine work with wide future use and can land in the same phase or
wait. BoneAltars is the natural second port: it reuses no engine
work from BarbarianPyres but its `prayer:bone` answerer + the
`bones.json` import then serve Ectofuntus (#3) and Prayer next.
Herblore (#4) and Fletching (#5) wait on one shared engine slice
— the item-action/item-on-item listener emit sites plus
`CreationMenu` — which unblocks both together with Firemaking's
core loop.

This is a recommendation, not a decision. If Tyler picks a
different skill, the §6 engine gaps still apply unchanged; only
the candidate-specific seams in §7 differ.

## 10. What a BarbarianPyres port phase needs first (brief-ready)

1. **Content port** (`internal/content/barbarianpyres` or the
   skills-content home the phase chooses): the 14-row `PYRE_LOGS`
   table verbatim (inline, no data import), the pyre-state map
   keyed by tile, `construct` / `itemOnSite` / `itemOnPyre`
   handlers in TS order (near check → already-a-pyre → axe →
   logs → Crafting level → consume + Crafting XP → swap to
   `PYRE_BOAT`; bones → Firemaking level → swap to
   `PYRE_BOAT_BONES`; tinderbox/bow → Firemaking level →
   Firemaking XP → swap to `PYRE_LIT` → 6-tick float task →
   reset), the disconnect reset, the `barbarian-pyre:built`
   emit for the magic-log + chewed-bones diary case. Axe/bow
   detection is by item name (ends with "axe"/"bow" minus the
   exclusion sets) via the cache definitions.
2. **Seam decision (defer or build):** the `spawnNpc`/`removeNpc`
   content members for the ferocious spirit (mangled bones only,
   ~3-minute lifetime task). If deferred, the spirit is a
   documented gap and mangled bones still light the pyre.
3. **Hook wiring to verify at port time:** the object-hook
   return-`false` fallthrough for non-pyre-site objects (Go
   contract exists); the `onItemOnObject` handlers return
   `false` for non-pyre objects so other content can claim them;
   `ObjectManagerAPI.Register` with `playerUpdate=true` sends the
   swap to players who see the tile; `CacheDefinitions.GetNpcId`
   resolves the spirit id by gameval name if the spawn seam
   lands.
4. **Live verification shape** (following the Phase 23/24
   precedent): real WebSocket client, real rev-242 cache, 3 runs
   stable — carve a pyre with logs + axe (Crafting XP, boat
   appears) → load chewed bones (bones swap) → light with
   tinderbox (Firemaking XP, lit → floating → site resets) →
   a second player's pyre on the same tile refuses ("There is
   already a pyre here.") → disconnect mid-pyre resets the site.
   Out of scope for that phase: Firemaking proper, any other
   skill, quests, gap 14, far-WALK bug, y=3521 wall, scrypt
   check, perf work.

## 11. Honest limits

- 13 of 29 entries read in full (BarbarianPyres, BoneAltars,
  Ectofuntus, Herblore, Fletching, Firemaking, BoltEnchanting,
  EnchantJewellery, Reanimation, AgilityDispenser,
  PrayerUnlocks, CraftingSpells, Prayer); the other 16 are
  sweep-screened only (lexical flags + require closure + data
  map) and are NOT ranked. In particular, RalosRise (607),
  ShootingStars (1,329), Runecrafting (1,918), Cooking (2,045),
  Crafting (2,088), Smithing (2,120), Mining (2,164), Thieving
  (2,591), Fishing (2,749), Woodcutting (3,220), Slayer (3,604),
  Farming (5,489), Agility (6,286), Hunter (9,927),
  Construction (12,270), and Sailing (20,617) were not read in
  full — a later survey may find a smaller portable slice inside
  one of them (e.g. a single RalosRise or ShootingStars unit).
- Lexical flags understate split skills and can match comments/
  strings (notably Timer); every ranked skill was read in full,
  so the shortlist does not rest on flags alone.
- Go-side checks were by grep over the repo (hook registrations,
  type existence); a type's existence does not prove its
  behaviour matches TS at the call site — port phases must
  re-verify each seam (notably `Player.setPositionToFace`,
  `StatementDialogue`, `OptionDialogue`, `player.getBanks`,
  `player.getClickDelay`, `CountdownTask`).
- The `player:world-input` / `interface:closed` events
  SkillLevelUp listens to were not checked for Go producers.
- Port-size estimates are Go content lines only, excluding the
  already-ported `SkillActions.js`; they are estimates, not
  measurements.

## 12. Method appendix (reproducibility)

All commands run from `~/tsps-bfd81e3/server/plugins/skills/`
unless noted. Counts are `wc -l` (lines including blanks and
comments), matching the plan's method.

- Directory totals: `find . -name '*.js' | wc -l`;
  `find . -name '*.js' -exec cat {} + | wc -l`.
- Entry plugins: `ls *.plugin.js` (27) + `find . -name
  '*.plugin.js'` (29, incl. subdirs).
- True totals: `require()` transitive closure per entry
  (scratch script; per-skill file lists reproducible from the
  require strings in each entry + unit file). Zero unresolved
  requires: every relative require resolves to a `.js`, `.json`,
  or `.ts` file.
- Sweep flags: the §2 patterns, run both as Python `re.search`
  per file and as `grep -rEl`; both runs agreed exactly (0
  mismatches over all 393 files).
- Data-file map: `require()` + `readFileSync` + sailing `read()`
  scans; every `data/*.json` file is referenced by at least one
  skill.
- Event map: literal `emitCustomEvent("name")` /
  `onCustomEvent("name")` strings; 388 distinct names, 531 emit
  / 305 listen call sites.
- Go-side checks: `internal/plugins/hooks.go` unfired-family
  table (firable 51 / table-only 57 — corrected from 53 this
  phase; see §5); `grep` over `internal/` + `cmd/` for
  `CreationMenu`, `StatementDialogue`, `ClickDelay`,
  `CountdownTask`, `getBanks`, `SetPositionToFace`, `spawnNpc`,
  `forceAdd`, `getItemOnGroundManager`.
- Deep dives: 13 entries read in full (listed in §11), plus
  `SkillActions.js`, `Quests` survey (`QUEST_SURVEY.md`) for
  format, and the Go hub/skills files for §5–§6.
