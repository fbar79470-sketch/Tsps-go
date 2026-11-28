# Phase 50 — NPC Drops

**Status:** IN PROGRESS (survey complete, plan written 2026-11-27; port workers dispatched)
**Tyler's order:** "Add npc drops" (2026-11-21 20:59 PST), queued after ladders (Phase 49 done).
**Baseline:** 5,247 tests, 32 packages (Phase 49, commit `e535eaa`).

## §1 Survey

### 1.1 Upstream sources read

- `~/tsps-bfd81e3/server/plugins/npcs/NpcDrops.plugin.js` (1,320 lines) — the whole drop system.
- `~/tsps-bfd81e3/server/docs/npc-drops-ordered.md` — ordered-reward contracts (Forgotten Four).
- `~/tsps-bfd81e3/server/plugins/npcs/data/drop-locations.json` — place boxes (NOT imported; see §1.5).
- `~/tsps-bfd81e3/server/plugins/skills/slayer/Keys.Slayer.js` — Brimstone/Larran's keys are Slayer's job.
- Both upstream AGENTS.md files (rules: plugin data lives with the plugin, world data in
  `data/definitions/`; rule 16: never hand-edit generated JSON — fix the dumper and regenerate).

### 1.2 Data shape (computed by Python over the imported files; hashes re-verified identical to upstream)

All three files live verbatim in `~/workspace/tsps-go/data/definitions/` (commit `821cd86`, pushed):

| File | Bytes | SHA-256 (prefix) | Shape |
|---|---|---|---|
| `npc-drops.json` | 5,478,191 | `18406d0b…` | `{meta, tables, npcs}` |
| `npc-drop-subtables.json` | 27,533 | `719e91ad…` | 13 shared tables |
| `npc-drop-overrides.json` | 15,665 | `b332e0c8…` | 6 keys: `$comment`, `removeNpcs`, `addNpcs`, `bossNpcs`, `tables`, `subtables` |

**`npc-drops.json` counts (computed, not guessed):**
- **1,106 tables**, keyed by id; `table_id` == key on all 1,106. **3,086 NPCs**, keys are numeric
  strings 1–16344, `npc_id` == key. Meta counts check out (`npc_count` 3086, `table_count` 1106,
  `table_refs` 3106 = 3060×1 + 23×2).
- **14,780 main entries + 1,594 tertiary entries = 16,374 total.** Main entry: `{item_id, name,
  quantity: [min,max], weight, out_of, rarity, section}` plus flags. 1,522 distinct item ids
  (main), 1,759 incl. tertiary.
- **1,278 always entries** (`always: true`, no weight/out_of/rarity; identified by the flag, not
  by rarity 1 — no main entry has numeric rarity 1.0). 949 tables have ≥1 always entry; 2,731 NPCs
  have ≥1. Bones 526 as an always drop: 481 tables, 1,558 NPC ids.
- **Tertiary:** 693 tables non-empty (0–10 entries), 1,594 entries, same shape as main, always
  `section: "Tertiary"` (mostly). 151 have numeric `rarity: 1.0` — conditional 100% (clue keys,
  quest items), gated by `notes`, not by a flag. 474 have `clue_scroll_box: true`.
- **`main_max_roll`** (658 tables) is the denominator of the main weighted roll, not the weight
  sum (e.g. Goblin table 1: main_max_roll 128, non-always weights sum to exactly 128;
  `a_corpse`: main_max_roll 4, one entry 1/4 — remainder is implicit nothing). Absent on 448
  tables (409 all-always + 39 empty). `rolls` (47 tables): 3 rolls on 39, 2 on 6, 9 on
  `bloodthirst_rockslug`, 25 on `ent`.
- **Special entries (1,106, no `item_id`):** 166 `{"nothing": true, ...}` (explicit empty outcome
  consuming weight) + 940 `{"shared_table": "<name>", ...}` (roll into a subtable).
- **Shared-table refs from main tables:** gem 377, herb 312, rareDrop 72, rareSeed 57,
  uncommonSeed 55, seed 44, usefulHerb 8, combatHerb 7, gwdRareDrop 4, gwdGem 4. Never referenced
  directly (nested only): `megaRare`, `talisman`, `gwdMegaRare`.
- **39 empty tables** (0 main entries, incl. `rat`, `ghost`, Barrows brothers, `reanimated_*` ×6,
  `tz_tok_jad`) — most still have tertiary entries; the port must not crash on empty `entries`.
- **3 orphan tables** (0 NPC refs): `alomone`, `maggot_king_2`, `zombie_pirate` — the latter two
  are reached via overrides `addNpcs` (15741 Maggot King, zombie pirates 13489–13493).
- **Tables per NPC:** 0 tables: 3 (8058/8060 Vorkath, 13686 Amoxliatl); 1 table: 3,060; 2 tables: 23
  (Flesh Crawler ×3, Ankou ×8, Farmer ×6, Scurrius, Nechryarch, Obor, Bryophyta, The Mimic).
  Most-referenced table: `undead_lumberjack` (72 NPCs). Referenced-but-undefined tables: 0.
- **1 unresolved item:** "Brimstone key" — appears as `unresolved_items: ["Brimstone key"]` in
  exactly 2 tables (`reanimated_dagannoth`, `wyrmling`); the name has no item_id anywhere in the
  dump. (Item 23083 "Brimstone key" exists as a resolved always-drop elsewhere.) The loader must
  skip it, not crash, and count it.
- **Rarity consistency:** `rarity == weight/out_of` holds except the 3 Corporeal Beast sigils
  (conditional-group published rarities via `group_access_weight/out_of`, `roll_group: "Sigils"`,
  `weight_basis: "conditional_group"`). The roller uses `weight`/`out_of` for the draw; `rarity`
  is informational.
- **Other flags:** `noted: true` 1,254 entries; `separate_roll: true` 1,358; `pre_roll: true` 156;
  `once_per_kill: true` 26; `bonus_drops` 10; `roll_count_weights` 29; `quantity_alternatives` 116
  (`quantity_selection: "conditional"`); `notes` on 526 entries; `assumed_rarity` 95;
  `published_rarity` 181; `clue_scroll_box` 1 main + 474 tertiary.
- **Case trap:** one always entry uses lowercase `rarity_raw: "always"` (Vanguard, Xeric's aid
  (+)(4) 20984) vs `"Always"` elsewhere — compare case-insensitively.
- **`reward_chain` (ordered path):** 4 tables only — the Forgotten Four (`duke_sucellus`,
  `the_leviathan`, `the_whisperer`, `vardorvis`): `type: "ordered_exclusive"`,
  `state_domain: "forgotten_four"`, 5 stages each + one `tertiary_groups` entry
  (`type: "exclusive"`, `source_access: {weight: 1, out_of: 40}`).

**`npc-drop-subtables.json` — 13 subtables** (`rareDrop`, `gem`, `megaRare`, `herb`, `usefulHerb`,
`combatHerb`, `seed`, `rareSeed`, `uncommonSeed`, `talisman`, `gwdRareDrop`, `gwdGem`,
`gwdMegaRare`): 133 entries in `entries` arrays + 44 in `seed.categories` = 177; 73 distinct
item ids. Entries are **raw strings, not parsed numbers**: `quantity_raw` (`"67"`, `"19500-20000"`,
`"100 (noted)"` → noted flag), `rarity_raw` always a fraction string `"N/D"`. `seed` is special:
no `entries`, instead `combat_level_multiplier: 10` + 6 level-banded categories (rarities out of
1008/1000). Nested refs: rareDrop→gem (20/128) + megaRare (15/128); gem→talisman (3/128) +
megaRare (1/128); gwdRareDrop→gwdGem + gwdMegaRare.

**`npc-drop-overrides.json` — hand edits applied by the plugin on top of the dump:**
- `removeNpcs`: 5 ids (6594, 7234, 9474 Ent — loot comes from the Ent trunk; 6406/6407 Tai Bwo
  Wannai tribesmen). Only **7234** is actually in the dump; the other four removals are no-ops.
- `addNpcs`: 113 NPCs (108 new ids + 5 zombie pirates 13489–13493 that already exist — override
  appends the second table, dump order first). All 113 override tables exist in the dump.
- `bossNpcs`: 3 ids (124 Yama, 13668 Araxxor, 14860 Shellbane gryphon) — cache lacks param 1361.
- `tables`: 2 edits — `abyssal_sire` `addTertiary` (Abyssal orphan pet 13262, 1/2560);
  `yama` `removeSections: ["Contract"]`.
- `subtables`: `{}` (empty).

### 1.3 TS roll algorithm (`NpcDrops.plugin.js`, line numbers verified)

**Load** (`loadDrops`, :440–552): read dump + subtables + overrides (+ world's
`npc-drops-custom.json`, which Go does not have — custom file support is a residual). Order:
`applyCustomDrops` first, then `applyOverrides` (:168–209: removeNpcs → addNpcs (existing NPC:
only tables it lacks are appended) → tables (addEntries/addTertiary dedup by item_id,
removeSections filters by section) → subtables (dump wins, override only fills gaps) →
bossNpcs collected), then `applyWikiCorrections` (:329–369: strip Slayer keys 23083/23490 from
all tables; animated bronze armour → separate 9/10 rolls; Zulrah + Vorkath `rolls = 2`; Mad
Angel / Skotizo / Amoxliatl / Alchemical Hydra bespoke fixes; Kalphite Queen items noted +
Consumables `roll_group`). `tablesByNpc` built; boss NPCs' tables get `table.boss = true`.

**Per kill** (`rollTable`, :1001–1088; `dropFor`, :1131–1155):
1. Location filter `withoutOtherLocations` (:320–328) — drops rows limited by a place note, using
   boxes in `plugins/npcs/data/drop-locations.json`.
2. Chance rules (`npc-drops:chance`, :693–699) + quest verdicts (`npc-drops:quest-item`, :974–999)
   fetched once per kill.
3. **Ordered chain first** (`OrderedRewards.select`, `OrderedRewards.NpcDrops.js:31–92`): if it
   returns an entry, that entry resolves and the **entire flat main roll is skipped**. Only
   tables with `reward_chain` trigger this (4 DT2 bosses). Throws (fail closed) if no
   `npc-drops:reward-state` provider answers.
4. **Always drops**: every `always: true` entry resolves unconditionally, once per kill (not
   repeated by `rolls`).
5. **Main rolls**: `rolls = table.rolls` if integer > 1 else 1; `rollOnce` runs that many times.
   Guaranteed, tertiary and `once_per_kill` drops are not repeated.
6. **Tertiary groups**: each `tertiary_groups` entry of `type: "exclusive"` picks at most one
   outcome (`OrderedRewards.group`).
7. **Tertiary rolls**: each `tertiary[]` entry rolled independently (`hits`, :616–624:
   chance = weight/out_of, scaled by multiplier, capped at 1). A tertiary with `out_of <= 1`
   (Wiki "Always" that is really conditional — clue keys, quest items) is **skipped unless a
   quest plugin returned `allowed: true`**. Looting-bag rows only drop in the Wilderness and
   never from Aviansie / Abyssal demon / Black chinchompa (:55–56).
8. **Guarantee rules**: tertiary rules with `guarantee: true` (league relics' clue scrolls) add
   the rarest matching entry if none naturally hit.
9. `selected.commit?.()` persists ordered-chain state after the full outcome resolves.
10. **Post-filter**: drops whose quest verdict is `false` are removed (filtering after the roll
    keeps other rows' published weights intact). Drops whose id is in `context.handledItemIds`
    are removed.

**`rollOnce`** (:867–955) — one main-table roll. Entries classified per roll:
- `always` → skipped here (handled in `rollTable`).
- `pre_roll` → independent `hits()` chance; **first hit wins and replaces the main roll**.
  Pre-roll *groups* (`roll_group`): one access roll (`group_access_weight /
  group_access_out_of`), then exactly one weighted item from the group (Corporeal Beast sigils:
  "1/585 chance of hitting the sigil drop table").
- `separate_roll` → independent `hits()` chance each; groups resolve to exactly one weighted
  item via `rollWeighted` (Skotizo ancient shards 75/15/5/4/1 in 100; Amoxliatl frozen tears one
  roll of 10 per kill, `once_per_kill`).
- Everything else with finite `weight > 0` → main pool, split into `main` vs `boostedMain`
  (boss-boosted rares, see below).

Then, if no pre-roll hit: boosted-main entries are each rolled **independently** via `hits()`
(so a boosted rare can drop *alongside* the main roll, and common rows keep OSRS rates), then
**one** weighted main roll via `rollWeighted(main, table.main_max_roll, rules)`.

**`rollWeighted`** (:847–865): `total = main_max_roll` (or sum of weights if `out_of` invalid).
Chance-rule multipliers grow the pool (`weight = entry.weight * rulesMultiplier`, `total +=
weight - entry.weight`). One uniform draw `roll = Math.random() * total`, walk cumulative
weights, first entry where `roll < 0` wins. **A roll past the last slot is a miss (empty slot)**
— returns `null`, no drop. An entry's own `out_of` is *not* the main-roll denominator; it is
used by the independent `hits()` rolls.

**Quantities** (`rollQuantity`, :600–614): uniform integer in `[min, max]`;
`quantity_alternatives` pick the first alternative whose condition holds (`perfect_kill`,
`bounty_task`, `preferred_method` via `npc-drops:context`, :565–598);
`quantity_selection: "unknown"` rows roll uniformly among listed quantities (an assumed
distribution, counted and logged at load).

**Subtable resolution** (`resolveEntry`, :816–845; `rollSharedTable`, :772–814):
- A main-table entry with `shared_table: "<name>"` rolls that shared table.
  `roll_count_weights` (e.g. aberrant spectre herb roll 1/2/3 herbs at 11/26, 11/26, 4/26)
  repeats the roll that many times (`RollCounts.NpcDrops.js:14–22`: one weighted draw over
  `[{rolls, weight}]`; a row without valid weights rolls once).
- Shared tables parsed at load (`sharedRows`): `rarity_raw: "n/outOf"` → `{weight, outOf}`
  (`parseSharedRarity`, :130–141); text rarities ("Common") skipped, counted as
  `unusableSubtableRows`; `quantity_raw` → `[min,max]` (`parseQuantityRaw`, :143–157);
  `"(noted)"` → `noted: true` (`isNotedQuantity`, :159–161).
- **RDT is a nested shared-table reference, not special-cased.** Resolution is recursive with
  a **depth cap of 4** (:774–776). Roll: **one** draw over the pool's cumulative weights;
  `outOf = entries[0].outOf` for the full pool; if the pool was filtered, `outOf` becomes the
  sum of remaining weights. Roll past the last slot = miss. `nothing` hit = no drop. A `ref`
  entry recurses.
- **Gem→mega-rare Legends' Quest gate** (:743–770, :805–808): if the player has *not* completed
  Legends' Quest, the mega-rare slot from the gem table is replaced by a talisman — nature
  talisman above ground, chaos talisman underground (map y ≥ 6400, with instanced-template
  chunk mapping via `mapY`). The rare table's own mega-rare slot needs no quest. Quest
  completion asked via `quest:is-complete`.
- **Ring of Wealth** (`RING_OF_WEALTH_TABLES = {"rareDrop","gem","megaRare"}`, :43): when worn,
  `nothing` rows are filtered out of those three tables before rolling (:792–794).
- **Seed table** (`SeedCategories.NpcDrops.js:31–38`): combat level × 10
  (`combat_level_multiplier`) is rolled as `randomInt(combatLevel * 10)` and the first category
  whose `maximum_exclusive` exceeds the roll is used; an NPC with no/unknown combat level rolls
  the first (lowest) category. `validCategories` requires categories to start at 0, be
  contiguous, and the last to have no `maximum_exclusive`.

**Boss rarity multiplier** (:84–94, :626–677): `isBoss` = id in `overrides.bossNpcs` **or** cache
NPC param **1361** is an integer. World config `NpcDrops:bossRareMultiplier` (default 1 = OSRS;
values ≤ 1 change nothing). Applies **only to "unique" entries** (`isUniqueEntry`, :655–670):
chance (weight/out_of) must be > 0 and **< 1/10** (`SPECIAL_MAX_CHANCE`, :88), and the entry
must be in a section matching `/unique/i` **or** be a Collection-log boss-tab item (struct 471,
params 683/690). An ordered pool counts only if *every* entry is a unique item. Boosted
main-table entries roll independently alongside the weighted main roll. Per
`docs/npc-drops-ordered.md`, ordered access rates use the same rules but only boss tables and
effective access chances rarer than 1/100 are boosted — **the 1/100 boundary (ordered) vs the
< 1/10 threshold (flat) is a real discrepancy to resolve in the follow-up that ports the
multiplier; this phase does not port the multiplier at all** (see §2).

**Death-hook integration** (`NPCDeathTask.ts:55–66`, `PluginManager.ts:1322–1329,3528–3544`):
after the death animation ticks elapse, **if `killer != null`** the event fires
(`killer = npc.getCombat().getKiller(true)`; `damagers = getRecentDamagers()` captured before
combat reset). **If there is no killer, no `onNpcDeath` fires at all — drops only happen for
player kills.** Event: `{ killer, damagers?, npc, npcId (from npc.getContentId(killer)),
location: {x,y,z}, remains? }`.
`NpcDrops` handler (:1277–1300): returns early if no killer/npc or non-integer id; emits
`npc-drops:claim` first (shared-loot bosses take over, `handled: true` skips the normal path);
otherwise lazy-loads drops on first death (`ensureDrops`, :1240–1244 — the ~10 MiB tables are
parsed on first NPC death, not at boot) and calls `dropFor(killer, npc, id,
npc.getLocation(), damagers ?? [])`, wrapped in try/catch that logs and swallows errors.
`dropFor` (:1131–1155): `tableForNpc` (first table by default; multiple tables → emits
`npc-drops:table` so a plugin can pick another, e.g. zombie pirates' Medium Wilderness Diary
table; if `itemOnGroundManager` is null, returns 0 — **a kill with no base table can still
receive contextual drops from plugins**); `rollTable` → drops; `npc-drops:add` (Catacombs totem
pieces/shards); `npc-drops:roll` (Warriors' Guild defenders; **diary noted-drop marking happens
here**); `npc-drops:generated` (if a listener sets `handled = true`, **nothing is placed**);
`npc-drops:location` (may move the drop location — Zulrah's body is out of reach → under the
killer — or set `public: true`, Chambers of Xeric); `placeDrops` → ground items; `reportLoot`
(:1170–1179) sends client script **7192** (`loottracker_add_loot`) once per item with the NPC
id, a per-kill key (`lootKey`, increments per kill, :40–42), item id and amount. Side channels:
`npc-drops:extra-rolls`, `npc-drops:table-rolls`, `npc-drops:roll-shared`,
`npc-drops:roll-table`, `npc-drops:place`. Owner command `reloaddrops` (:1301–1315).

**Ground items** (`ItemOnGroundManager.ts:371–380`, `ItemOnGround.ts:55–100`): drops placed via
`registerLocation` with the **killer as owner**, the **NPC's death location**, `{public: true}`
only when the `npc-drops:location` event said so. Default state `SEEN_BY_PLAYER`, owner =
killer's username, `goesGlobal` defaults to `true`. **Goes public after 100 ticks**
(`PUBLIC_REVEAL_DELAY = 100`); **despawns after 300 ticks** (`DESPAWN_DELAY = 300`). Stackables
register as one stack; unstackables one item per unit (`placeDrops`, :1209–1227). Invalid drops
(non-integer/negative id, non-positive amount) are skipped (`validDrop`, :1161–1168).
**Inventory-full is a non-issue: drops always go to the ground.** The sole exception is Ring
of Wealth coin auto-collect: currency ids 995 (coins), 6529 (tokichat), 21555 go straight into
the inventory up to the stack cap when the killer wears a ring of wealth and has auto-collect
on (default on; `RingOfWealth.plugin.js:74–76`); overflow drops to the ground as usual.

**Noted drops** (`droppedItemId`, :1181–1189): a drop arrives noted when its resolved `noted`
flag is true; `placeDrops` swaps the item id for its note id (only if a note exists and is
noted). Flag sources, all applied before placement: (1) table data: `"(noted)"` in a subtable
row's quantity string, or hardcoded `noted: true` in runtime corrections (Kalphite Queen's
listed items, Mad Angel's items); (2) **diary perks — drop-time behavior, not a note-exchange**:
`NotedDrops.AchievementDiaries.js` listens on `npc-drops:roll` and flips `drop.noted = true`
when a completed diary tier applies (Desert easy — goat horns; Falador hard — Giant Mole
skin/claw; Karamja elite — red dragonhide + bronze/iron/steel bars in the Brimhaven Dungeon
box; Wilderness elite — dragon bones from dragons in the Wilderness except KBD);
`NotedDrops.FremennikDiary.js` (Dagannoth bones, aviansie adamantite bars). The Pirate Jackie /
Lesser Fanatic toggles are **not** implemented in TS either (no capture of their dialogue
lines), so these rewards are always on. (3) `NoteExchange.plugin.js` is a separate merchant
service, unrelated to drops.

**RNG: `Math.random` everywhere — no seeded RNG, no per-NPC RNG.** `randomInt(bound) =
Math.floor(Math.random() * bound)` (:554–556); `hits`, `rollWeighted`, `rollSharedTable`,
`rollQuantity` and `OrderedRewards.select` (default `Math.random`) all use it. The only counter
is `lootKey` (loot-tracker key, increments per kill — not RNG).

**Kill context** (`killContext`, :962–970): emits `npc-drops:context`; plugins fill
`perfectKill`, `bountyTask`, `preferredMethodLocation`, `handledItemIds`, and `combatLevel`
(from the NPC definition, :957–960).

### 1.4 Go-side integration points (surveyed, read-only)

- **Death flow — a hook already exists; no new call site needed.**
  `internal/game/combat_death.go`: `DecrementHealth` (:108) → `beginDeath()` (:189, fires
  `Hooks.OnNpcBeforeDeath`, captures `deathDamagers`/`deathKiller`, `deathTicks = 1` or the
  definition's `DeathTicks()` if > 1 — default 2, `internal/definition/npc.go:295`) →
  `advanceDeath` (:309, from `ProcessHitQueue`, `internal/game/combat.go:506`) →
  `dispatchNpcDeath` (:469–518): builds the event, fires **only if a killer exists**, schedules
  respawn, removes the corpse via `removeDeadNPC` (:510) → `npcRemover` seam
  (`npc_respawn.go:303–308`).
- **Hub side:** `plugins.Api.OnNpcDeath` (`internal/plugins/combat.go:649–670`);
  `plugins.Hub.EmitNpcDeath` (:954–970); `plugins.NpcDeathEvent` (:206–219: `Killer`,
  `Damagers`, `NPC`, `NpcID`, `Location`, `Remains`). Wiring:
  `cmd/server/combatwiring.go:250` (`InstallCombatHooks`), adapter :538–574 resolves the victim
  to `*game.NPC` via `findCombatNPC` (:517–536), sets `NpcID: npc.ContentID()` (transformation
  id if set, else spawned id). **Firable today** — live consumer exists: Ent trunks
  (`internal/content/woodcutting/enttrunk.go:118–121`, `api.OnNpcDeath(e.entDied)`).
  Consequence: **NPCs that die with no killer never fire `onNpcDeath`** — TS-faithful; no
  drops for those kills (same as TS).
- **Ground-item manager** (Phase 8): `internal/game/grounditems.go` (1,186 lines).
  `GroundItemManager.Register` (:684); `NewItemOnGround` (:238). `GroundItemSeenByPlayer` (0,
  owner-only) vs `GroundItemSeenByEveryone` (1); `PublicRevealDelay = 100` ticks (:60),
  `DespawnDelay = 300` ticks (:65), view range 64. Engine seam
  `game.CombatHooks.DropGroundItem func(item Item, loc Location, owner *Player)`
  (`combat_hooks.go:327–331`) — installed by `InstallDeathWiring`
  (`cmd/server/combatwiring.go:295–323`): builds an owner-only stack (`GroundItemSeenByPlayer`,
  goes-global after 100 ticks, despawns after 300). Already used by woodcutting
  (`woodcutting.go:750–759`), fishing clue bottles (`cluebottles.go:272–275`), cooking
  (`churn.go:186–193`). **Limits:** drops silently when owner is nil (:300–303); always
  `goesGlobal=true, respawnTimer=-1`, no per-drop despawn override, no public-from-birth
  option; non-stackables need one call per single item. Content cannot reach the manager
  directly today (it lives in `cmd/server`).
- **NPC definition at death:** `internal/game/npc.go:72` (`ID`, `Spawn`, embedded `Mobile`);
  `ContentID()` (:336) is what the hub event carries. `func (s *Store) NPC(id int)
  *NpcDefinition` (`internal/definition/npc.go:180`) never returns nil (DEFAULT shape).
  `game.NPCDefinition` (`npc.go:241–253`) has **no `Name()`** — a drop plugin needing the NPC
  name must resolve it via the `*definition.Store` (wired in `cmd/server`, e.g.
  `adapter.store`, `server.go:353`) or via the drop file's own `npcs` map (carries
  `name`/`cache_name` per id).
- **Loader pattern to copy:** `loadGroundItems` (`internal/definition/grounditems.go:66–106`) —
  core file REQUIRED, `readJSONValue` (`loader.go:532`, missing/malformed = error), per-entry
  validation in a `toX(raw any) (X, bool)` converter, invalid entries skipped + counted,
  members-only skipped on F2P, cache-unsupported ids skipped via `s.hasItem(id)`, counts logged
  via `s.logf`. Called in fixed order from `Load` (`loader.go:354`), called once at boot from
  `loadGameDefinitions` (`cmd/server/definitionwiring.go:66`, error is boot-fatal). `loadShops`
  (`internal/definition/shop.go:159–226`) is the variant where two core files are both required.
  **No loader currently reads the three drop files** — `internal/definition/doc.go:131–137`
  lists them under "Files no ported loader or wrapper reads". Note the drop files are a JSON
  **object** (`{meta, tables, npcs}`), not an array — validate object shape instead.
- **Drop hub families:** none exist. No `onNpcDrop`, `npc-drops:roll`, or drop-table family.
  The generic mechanism exists: `Api.OnCustomEvent(name, handler)` /
  `Hub.EmitCustomEvent(name, payload)` (`internal/plugins/events.go:32,47`), names must be
  namespaced (`isCustomEventName`, :21). Player-death families (`onShouldDropItemsOnDeath`
  etc.) are unrelated.
- **PORT_STATUS current drops text** (`PORT_STATUS.md:2156–2158`, combat row): *"Drops are NOT
  rolled by core: no `npc-drops:roll` exists in TS core — drops are plugin content listening to
  `onNpcDeath`, and Go dispatches `CombatDeathEvent` through the hook seam."* Also
  `PORT_STATUS.md:1360` (*"…no drops in core (content's job)…"*) and `:1369` (onNpcDeath
  firable since Phase 16/33). Hub is 63/108 families firable.

### 1.5 Live slice — full tables (from the files; ids verified)

**Goblin — 43 variants named "Goblin".** 3036 → `goblin_drop_table1` (30 NPCs); 3045–3046 etc.
→ `goblin_drop_table2` (13 NPCs). **Neither goblin table references the rare drop table or the
gem subtable** — table 1 has no shared-table entries at all; table 2 has one shared entry,
`herb` 2/128.

*goblin_drop_table1* (`label: "Drop table 1"`, `main_max_roll: 128`, 20 entries + 4 tertiary;
non-always weights sum to exactly 128):
- **ALWAYS: Bones 526 ×1**
- Bronze sq shield 1173 ×1, 3/128; Bronze spear 1237 ×1, 4/128; Water rune 555 ×6, 6/128; Body
  rune 559 ×7, 5/128; Earth rune 557 ×4, 3/128; Bronze bolts 877 ×8, 3/128; Coins 995 ×15,
  28/128; Coins ×20, 2/128; Coins ×1, 1/128; **Nothing, 38/128**; Hammer 2347 ×1, 15/128;
  Goblin mail 288 ×1, 5/128; Chef's hat 1949 ×1, 3/128; Energy potion(3) 3010 ×1, 3/128; Energy
  potion(4) 3008 ×1, 3/128; Goblin book 10999 ×1, 2/128; Beer 1917 ×1, 2/128; Brass necklace
  1009 ×1, 1/128; Air talisman 1438 ×1, 1/128.
- Tertiary: Goblin skull 7812 ×1, 1/1 (quest-gated: Rag and Bone Man I only); Ensouled goblin
  head 13448 ×1, 1/35; Goblin champion scroll 6801 ×1, 1/5000; Clue scroll (beginner) 23182 ×1,
  1/64, `clue_scroll_box: true`.

*goblin_drop_table2* (`label: "Drop table 2"`, `main_max_roll: 128`, 24 entries + 4 tertiary):
- **ALWAYS: Bones 526 ×1**
- Bronze axe 1351 ×1, 3/128; Bronze scimitar 1321 ×1, 1/128; Bronze spear 1237 ×1, 9/128;
  Bronze arrow 882 ×7, 3/128; Mind rune 558 ×2, 3/128; Earth rune 557 ×4, 3/128; Body rune
  559 ×2, 3/128; Bronze javelin 825 ×5, 2/128; Chaos rune 562 ×1, 1/128; Nature rune 561 ×1,
  1/128; Coins ×8, 34/128; Coins ×5, 8/128; Coins ×16, 7/128; Coins ×24, 3/128; Energy
  potion(2) 3012 ×1, 13/128; Goblin mail 288 ×1, 10/128; Hammer 2347 ×1, 9/128; **Nothing,
  8/128**; Goblin book 10999 ×1, 2/128; Grapes 1987 ×1, 1/128; Red cape 1007 ×1, 1/128; Tin
  ore 438 ×1, 1/128; **shared `herb`, 2/128** (section Herbs).
- Tertiary: same 4 as table 1 except Ensouled goblin head 1/30 and Clue scroll (beginner) 1/80.

**Chicken — 12 variants** (1173, 1174, 2804–2806, 3661–3662, 9488, 10494–10497), all →
`chicken`. Table `chicken` (main_max_roll 128, 5 entries + 2 tertiary):
- **ALWAYS: Bones 526 ×1; Raw chicken 2138 ×1**
- Feather 314 ×5, 64/128 (0.5); Feather ×15, 32/128 (0.25); **Nothing, 32/128**.
- Tertiary: Key (medium) 2832 ×1, 1/1 (only when completing a medium clue asking for a Chicken
  kill); Clue scroll (beginner) 23182 ×1, 1/300, `clue_scroll_box: true`.

**Cow — 5 variants** (2790, 2791, 2793, 2795, 5842), all → `cow`. Table `cow` (no
`main_max_roll` — all-always table, 3 entries + 1 tertiary):
- **ALWAYS: Cowhide 1739 ×1; Raw beef 2132 ×1; Bones 526 ×1** (no weighted roll at all).
- Tertiary: Clue scroll (beginner) 23182 ×1, 1/128, `clue_scroll_box: true`.

**Man — 18 variants** (3014, 3106–3110, 3261, 3264–3265, 3652, 6815, 6818, 6987–6989,
11057–11058, 14920), all → `man`. Table `man` (main_max_roll 128, 20 entries + 3 tertiary;
main weights incl. herb sum to 128):
- **ALWAYS: Bones 526 ×1**
- Bronze med helm 1139 ×1, 2/128; Iron dagger 1203 ×1, 1/128; Bronze bolts 877 ×2–12,
  22/128; Bronze arrow 882 ×7, 3/128; Earth rune 557 ×4, 2/128; Fire rune 554 ×6, 2/128;
  Mind rune 558 ×9, 2/128; Chaos rune 562 ×2, 1/128; Coins ×3, 38/128; Coins ×5, 9/128;
  Coins ×15, 4/128; Coins ×25, 1/128; **Nothing, 8/128**; Fishing bait 313 ×1, 5/128; Copper
  ore 436 ×1, 2/128; Earth talisman 1440 ×1, 2/128; Cabbage 1965 ×1, 1/128; **shared `herb`,
  23/128**; Energy potion(3) 3010 ×1, **1/8, `separate_roll: true`, `assumed_rarity: true`**
  (its own denominator, excluded from the 128-sum).
- Tertiary: Key (medium) 2832 ×1, 1/1 (clue-gated); Looting bag 11941 ×1, 1/15 (Wilderness
  only); Clue scroll (beginner) 23182 ×1, 1/90, `clue_scroll_box: true`.

**Rat — 4 variants named "Rat":** 2854, 2855 → `rat`; 2492, 2513 → `rat_2`.
- Table `rat` (no `main_max_roll`, **0 main entries** — one of the 39 empty tables; 3 tertiary
  only, **no Bones always drop**): Rat's tail 300 ×1, 1/1 (Witch's Potion only); Rat bone
  7836 ×1, 1/4 (Rag and Bone Man II only); Looting bag 11941 ×1, 1/15 (Wilderness only).
- Table `rat_2` (1 entry + 2 tertiary): **ALWAYS: Bones 526 ×1**; tertiary Rat's tail 1/1
  (Witch's Potion) and Rat bone 1/4 (Rag and Bone Man II) — no Looting bag.

**Rare-drop subtable referrers:** 262 NPC ids (70 distinct names) reference `rareDrop` — all
mid/high-level monsters (Nechryael, King Black Dragon, dragons, Abyssal demon/Sire, Skeletal
Wyvern, Kalphite Queen, Zulrah, Chaos Elemental, Fire giant, Dagannoth Kings, Cerberus,
Vorkath, Alchemical Hydra, Lizardman shaman, Demonic/Tortured gorilla, Phantom Muspah, Araxxor,
Mad Angel, Vyrewatch, Giant Mole, Cyclops, archaeologists, Chaos Fanatic, Elder Chaos druid,
Mammoth, Lava/Frost dragon, etc.). **Goblins, chickens, cows, men and rats do NOT reference it.**

### 1.6 Data hygiene (all computed)

- No duplicate table ids possible (JSON object keys; `table_id` == key on all 1,106).
- Zero entries with `item_id` 0/negative/missing-except-by-design (the only `item_id`-less
  entries are the 1,106 intentional `shared_table`/`nothing` shapes).
- 533 rows match "Only dropped…" in `notes` — but most are quest phrasing ("Only dropped
  during Rag and Bone Man II", "only dropped after completion of Monkey Madness II"), not
  place gating; the true place-gated subset is smaller and **uncounted**. Location filtering
  needs `drop-locations.json`, which was NOT imported — see §7 residual.
- 224 rows carry `WikiQuestDropRequirement` notes. With no quest plugins in Go, quest
  verdicts are undefined → rows roll normally (TS behaves the same with no answerer: only a
  verdict of `false` removes a drop). The 1/1 conditional-tertiary rule (§1.3 step 7) is the
  separate mechanism that keeps clue keys / quest items from dropping unconditionally.
- `brimstone_key_roll: true` on 151 tables, `brimstone_konar_bonus: true` on 66 — these mark
  tables where the **Slayer** plugin (`Keys.Slayer.js`) rolls Brimstone/Larran's keys on Konar
  tasks. Not NpcDrops' job; keys never drop from the drop system in this phase (Slayer
  residual).

## §2 Scope decision

**The phase is the drop-table SYSTEM (loader + roller + NPC-death integration) for ALL 1,106
tables, plus a live-verified slice.** The system is generic — every table loads and rolls the
same way — so the "slice" is about what gets **live-verified**, not what gets loaded. All
3,086 NPCs with tables get drops from this phase; nothing is gated behind a per-NPC port.

**In scope:**
1. **Loader** (`internal/definition/`): parse all three files at boot (eager, Go pattern — not
   TS's lazy load on first death); apply overrides in TS order (removeNpcs → addNpcs → tables
   → subtables → bossNpcs); apply the Slayer-key strip (remove entries with item_id 23083 /
   23490 from all tables, per `applyWikiCorrections`); parse subtables (string rarities,
   quantity ranges, `"(noted)"`); parse seed categories; expose `Store.NpcDropTable(npcID)`
   (+ stats for tests). Boot-fatal on missing/malformed core file, per the loader pattern.
   Skip (counted, logged) the 1 unresolved "Brimstone key" entry in `reanimated_dagannoth` /
   `wyrmling`. Load counts asserted in tests against the real files (1,106 tables, 3,086 NPCs
   after overrides, 13 subtables).
2. **Roller** (`internal/content/npcdrops/` or as the survey dictates): the full flat path —
   always drops, `rolls` repetition, `rollOnce` (pre_roll first-hit-wins incl. groups,
   separate_roll incl. groups, one weighted main roll over `main_max_roll` with empty-slot
   misses), tertiary independent rolls incl. the **1/1 conditional rule** (skip unless a quest
   plugin answers `allowed: true`; with no quest plugins in Go, conditional tertiaries never
   drop — TS-faithful), tertiary groups (only the 4 DT2 tables carry them — see ordered-path
   rule below), `roll_count_weights` (RollCounts), seed-table category banding by NPC combat
   level, shared-table recursion with depth cap 4, `quantity_alternatives` evaluated against an
   empty context (base quantity — no context provider exists), `bonus_drops`, `once_per_kill`,
   `clue_scroll_box` rows roll as ordinary tertiary (no clue system consumes them yet).
   RNG: `math/rand` in production (unseeded, like TS `Math.random`); injectable source for
   tests so distributions are verified exactly over seeded runs.
3. **Ordered path — fail closed:** if a table carries `reward_chain` (4 DT2 boss tables only),
   the main roll is skipped with a log line (TS throws when no `npc-drops:reward-state`
   provider answers); **always drops and tertiary still resolve** (per
   `docs/npc-drops-ordered.md`). No state provider is built in this phase.
4. **Integration:** a content plugin registering `api.OnNpcDeath`, first table by default
   (multi-table NPCs: emit `npc-drops:table`, no picker exists yet → first table), placement
   via the existing `game.Hooks.DropGroundItem` seam (owner-only stack, goes global at 100
   ticks, despawns at 300), stackables as one stack, non-stackables one call per unit,
   noted drops swapped to their note id (ground the note-id lookup from the cache/item
   definitions — if no Go lookup exists, add one from the cache the way the loader resolves
   item names; do not invent note ids), `validDrop` filtering (integer id ≥ 0, amount > 0).
   Emit the custom events at the same points TS does (`npc-drops:claim` first,
   `npc-drops:table`, `npc-drops:add`, `npc-drops:roll`, `npc-drops:generated` (handled
   short-circuits placement), `npc-drops:location`) — with no consumers in Go yet, default
   behavior is the normal path (TS-faithful). Loot-tracker script **7192** sent once per
   dropped item (npc id, per-kill key, item id, amount). `reloaddrops` OWNER command (reload
   the three files).
5. **Wilderness looting-bag gate:** implement the TS rule (tertiary "Looting bag" rows drop
   only if the NPC died in the Wilderness and its name is not Aviansie / Abyssal demon /
   Black chinchompa) using the existing Wilderness zone helpers (Phase 15).
6. **Legends' Quest gem→mega-rare gate:** implement the substitution (nature talisman above
   ground, chaos talisman underground, map y ≥ 6400) with the quest-complete check defaulting
   to **false** (no quest system in Go yet) — documented stub, to be wired to the quest
   system when it lands.
7. **Boss multiplier: NOT implemented** (default config 1 = OSRS rates). At multiplier 1, TS
   classifies every entry into the main pool, so a roller with no boost logic is exact at
   default config. The config plumbing + unique-entry classification + the 1/100-vs-1/10
   threshold discrepancy are a follow-up.
8. **Ring of Wealth: NOT implemented** (no RoW plugin in Go). `nothing` rows stay in the
   RDT/gem/megaRare pools (OSRS no-RoW behavior); coins drop on the ground normally (no
   auto-collect).

**Live-verified slice (coordinator runs the legs in `/home/hatch/smoke50/`):**
Goblin 3036 (table 1), Chicken 1174, Cow 2790, Man 3106 (herb subtable + separate-roll
energy potion), Rat 2854 (empty main table — proves no crash, no main drops), plus one RDT
referrer if a kill is feasible in the harness (Fire giant — check spawn availability; if not
feasible, RDT is unit-covered with seeded RNG and documented as live-open).

**Explicitly OUT of this phase (residuals, §7):** ordered-path state provider (Forgotten Four
persistence); boss rare multiplier; Ring of Wealth (nothing-filter + coin auto-collect);
diary noted-drop marking; location filtering (`drop-locations.json` not imported);
`applyWikiCorrections` boss fixes (Zulrah/Vorkath `rolls = 2`, Mad Angel / Skotizo /
Amoxliatl / Alchemical Hydra, animated bronze armour, Kalphite Queen noted);
`npc-drops:extra-rolls` / `:table-rolls` / `:roll-shared` / `:roll-table` / `:place` side
channels (no consumers exist in Go); `npc-drops-custom.json` world overrides; Slayer keys
(Slayer's job).

**Decisions recorded:** killer-less deaths drop nothing (TS-faithful — `onNpcDeath` never fires
without a killer, both engines). NPCs with 0 tables (Vorkath ×2, Amoxliatl) drop nothing from
the base table (contextual drops only — none exist). 1/1 conditional tertiaries never drop
until a quest plugin answers (fail-closed, TS-faithful). Location-noted rows roll normally in
this phase (over-drop for the place-gated subset — see §7). Quest-noted rows roll normally
(TS-faithful with no quest plugins). The loader is eager at boot (Go pattern), not lazy on
first death (TS pattern) — a 5.5 MB parse at boot, not per kill.

## §3 Work streams (parallel workers)

- **Worker A — loader** (`internal/definition/npcdrops.go` + tests): the three files, override
  application in TS order, Slayer-key strip, subtable parsing, seed categories, `Store`
  accessors, load-count tests vs the real files. Ground every field mapping in the files
  themselves; cite `NpcDrops.plugin.js` line numbers for the transform order. Update
  `internal/definition/doc.go` (drop files move out of "no ported loader reads").
- **Worker B — roller** (`internal/content/npcdrops/roller.go` + tests): flat path + ordered
  fail-closed + tertiary + shared tables + seed banding + RollCounts + Legends gate stub.
  Injectable RNG; seeded-distribution tests (always-drops exact; main-roll membership and
  amounts in range over N seeded kills; empty-table no-crash; depth-cap; 1/1 tertiary skip).
  No placement, no hub registration — pure roll function over loaded tables.
- **Worker C — integration + harness** (`internal/content/npcdrops/` plugin: `OnNpcDeath`
  handler, `dropFor`/`placeDrops`, custom-event emits, script 7192, `reloaddrops` command;
  wiring in `cmd/server` + one blank import in `pluginwiring.go`; note-id lookup grounding):
  plus the smoke harness under `/home/hatch/smoke50/` (reuse the smoke42/49 WS driver pattern:
  real server binary + real rev-242 cache, kill goblins/chickens/cows/men/rats, observe ground
  items on the wire / in the save). Legs may run singly if a full pass exceeds the
  background-exec wall clock.

**Coordinator integrates** (merge the three streams, resolve conflicts, run full verification,
run the live legs, fill §8, update PORT_STATUS, report to the parent). No commits by workers
or coordinator. No `.js` files. No stray artifacts in the repo. Do not touch `/home/hatch/live36`
or `~/tsps-full`. No broad pkill. Do not write to `~/tsps-bfd81e3`.

## §4 Verification (coordinator)

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./...`.
- Full suite with `-p 1 -count=1 -v` (report top-level + subtest counts, packages, failures,
  skips). Baseline 5,247 tests / 32 packages.
- `go test -race ./...` — the known pre-existing `TestEngineTicksAtRate` 5 ms timing flake in
  `internal/game`, the known §20 `cmd/server` race-load timing issues, and the known
  `TestReloginRoundTripOverWebSocket` disconnect-save timeout are NOT data races: if the race
  run fails only on one of those with no DATA RACE warning, re-run that package once before
  reporting. Any DATA RACE warning is a real finding.
- **Live legs** (fresh temp root `/home/hatch/smoke50/`, real server binary + real rev-242
  cache at `/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`): kill real NPCs and
  observe the loot actually dropping on the ground (item ids + amounts vs the table).
  Deterministic checks: always-drops present on **every** kill; every dropped id is a member
  of the NPC's table (incl. subtables); amounts within `[min,max]`; no crash on the empty Rat
  table; nothing-slot misses observed (a kill with only Bones = main roll hit Nothing).
  Random rolls cannot be verified exactly — verify the deterministic parts and say so.
  Record exact observed drops in §8, with what was NOT observed live stated plainly.

## §5 Docs

- This plan (§8 Results filled in).
- `PORT_STATUS.md`: header, What-was-verified entry, content row count — **COUNT it, do not
  guess** (NpcDrops.plugin.js is one entry in `CONTENT_MIGRATION_PLAN.md`; verify the row
  number at integration time) — new gaps section for residuals.
- PORT_STATUS honesty rules: DONE / PARTIAL / NOT STARTED, no parity claims.

## §6 Risks / open grounding points

- **Note-id lookup:** TS swaps noted drops to their note id via the cache. The integration
  worker must ground whether Go has a note-id lookup (`internal/definition/item.go` or the
  cache); if missing, add one from the cache — do not invent note ids.
- **Loot-tracker script 7192:** Go sends client scripts by number elsewhere (e.g. 917 bank,
  2046 creation menu) — the worker must confirm the send path, not assume the client
  renders it.
- **Looting-bag Wilderness check:** verify the Phase 15 Wilderness helpers resolve a death
  location to "in the Wilderness" before relying on them.
- **Seed table combat level:** the NPC definition must expose a combat level for the banding
  roll; if the Go definition lacks it, the first (lowest) category is the documented fallback
  (same as TS for unknown level).
- **Ground-item observation in the harness:** the driver must read ground-item CREATE frames
  (or the save after pickup) — reuse the smoke42/49 driver's frame parsing; do not invent a
  new observation channel.
- **`getKiller(true)` semantics** (top damager vs last hitter) were not traced upstream — the
  Go port inherits whatever `GetKiller` already does; not a drops-phase question.
- **Tick-to-second conversion** for the 100-tick reveal / 300-tick despawn was not verified in
  the survey — state ticks only.

## §7 Residuals (NOT STARTED — follow-ups)

1. Ordered-path state provider (Forgotten Four persistence domain, `npc-drops:reward-state` /
   `:reward-state-commit`, cyclic progression, quartz/tablet/orb scaling curves unresolved
   upstream) + `tertiary_groups` exclusive picks. 4 boss tables only.
2. Boss rare multiplier (world config `NpcDrops:bossRareMultiplier`, unique-entry
   classification, 1/100 ordered vs <1/10 flat threshold discrepancy).
3. Ring of Wealth (nothing-filter on RDT/gem/megaRare + coin auto-collect 995/6529/21555).
4. Diary noted-drop marking (`npc-drops:roll` listeners; no diary system in Go).
5. Location filtering — import `drop-locations.json` + `withoutOtherLocations`; place-gated
   rows currently roll anywhere (over-drop; place-gated subset uncounted, inside the 533
   "Only dropped…" rows, most of which are quest phrasing).
6. `applyWikiCorrections` boss fixes: Zulrah/Vorkath `rolls = 2` (currently roll once),
   Mad Angel / Skotizo / Amoxliatl / Alchemical Hydra bespoke fixes, animated bronze armour
   separate rolls, Kalphite Queen noted items.
7. Side channels with no consumers: `npc-drops:extra-rolls`, `:table-rolls`, `:roll-shared`,
   `:roll-table`, `:place` (for CoX, chests, multi-player boss rewards).
8. `npc-drops-custom.json` world overrides + `npc-drops:claim` shared-loot bosses
   (Wilderness bosses) + `npc-drops:location` movers (Zulrah/CoX) — events are emitted,
   no listeners exist.
9. Slayer keys (23083 Brimstone / 23490 Larran's) — stripped from drop tables at load;
   rolled by the Slayer plugin when it lands.
10. Clue-scroll tertiary rows roll but no clue system consumes them; ensouled heads /
    champion scrolls roll but no systems consume them.
11. RDT live observation — if the Fire-giant leg is not feasible in the harness, RDT stays
    unit-covered with seeded RNG.

## §8 Results

**Status: DONE, coordinator-verified 2026-11-28.** The drop-table system is
ported (loader + roller + `onNpcDeath` integration), unit/race-verified, and
live-verified on six legs. No commits made by the coordinator (parent handles
git). Harness: `/home/hatch/smoke50/` (not in the repo); logs
`smoke50-<mode>-run1.log` per leg (each run overwrites `run1/` + the log).

### What was ported

- **Loader** — `internal/definition/npcdrops.go` (+ tests): reads the three
  already-imported files, applies overrides in TS order (removeNpcs →
  addNpcs → tables → subtables → bossNpcs), then the Slayer-key strip of
  `applyWikiCorrections` only. Load line from the smoke server log:
  `[npc-drops] Loaded 1106 tables for 3193 NPCs (3190 with tables), 13
  shared subtables (1 seed), 14737 entries + 1592 tertiary (skipped=0
  entries, 0 tables, 0 NPCs; unusableSubtableRows=0, unresolvedItems=2,
  slayerKeysStripped=27, assumedQuantityRows=12)`. 3,193 NPCs is 3,086 dump
  − Ent 7234 removed + 108 new `addNpcs` ids (the plan's §2.1 figure of
  3,086 "after overrides" was wrong — the loader test asserts 3,193).
  14,737 main entries is the dump's 14,780 minus 24 Slayer keys stripped
  minus 19 Yama Contract-section entries removed (tertiary: 1,594 minus
  3 Slayer keys plus 1 Abyssal orphan added = 1,592 — both close exactly). `Store.ReloadNpcDrops()`
  rebuilds every map from scratch and only assigns at the end, so a failed
  reload leaves the old tables in place.
- **Roller** — `internal/content/npcdrops/types.go` + `roller.go` (+ 22
  tests): pure roll function, injectable RNG. Flat path fully ported:
  always → `rolls`×`rollOnce` (pre-roll first-hit-wins incl. groups,
  separate rolls, one weighted main roll over `main_max_roll` with
  empty-slot misses) → exclusive tertiary groups → independent tertiary
  rolls (1/1 conditional rule, Looting-bag Wilderness gate) → quest-verdict
  + handled-ids post-filter. Shared subtables resolve recursively (depth
  cap 4; RDT is a nested reference, not special-cased), seed table bands
  by combat level ×10, gem→mega-rare Legends' Quest gate (quest-complete
  defaults to false — no quest system yet), Ring of Wealth NOT implemented
  (nothing rows stay — OSRS no-RoW behavior), boss multiplier NOT
  implemented (config default 1 = OSRS rates), ordered path fails closed
  (4 DT2 tables: main roll skipped with a log line, always + tertiary
  still resolve). Quantities roll uniformly in `[min, max]`;
  `quantity_selection: "unknown"` rolls uniformly among listed quantities
  (assumed distribution, as in TS).
- **Integration** — `internal/content/npcdrops/npcdrops.go` (+ 17 tests):
  `NpcDrops` plugin on `api.OnNpcDeath`, TS `dropFor` order exactly
  (`npc-drops:claim` → roll → `:add` → `:roll` → `:generated` → `:location`
  → `placeDrops` → `reportLoot`), loot-tracker script 7192 sent once per
  dropped item as `(npcID, lootKey, itemID, amount)` with `lootKey`
  starting at 1 and incrementing per kill, `::reloaddrops` OWNER command.
  Placement goes through the existing `DropGroundItem` engine seam
  (owner-only stack, goes global at 100 ticks, despawns at 300) — so
  `npc-drops:location` `public:true` is accepted but cannot be honoured on
  this seam, and noted drops swap to the note id via `NoteID()`.
- **Wiring** — `cmd/server/npcdropswiring.go` (+ converter tests):
  `definition.NpcDropTable → npcdrops.Table` converter (quest key/mode
  parsed from Notes with the TS regex; quantity-alternative conditions
  preserved — the loader was fixed in-integration to keep them, since the
  dump carries `preferred_method`/`bounty_task`/`perfect_kill`
  conditions), shared-table map builder (flat subtables + seed categories),
  `npcDropTableSource`, `SetTableSource`/`SetRoller`/`SetReloader`,
  Wilderness checker. Installed in `buildServer` right after
  `loadGameDefinitions`. Three `pluginwiring_test.go` expected lists
  updated (NpcDrops loads between LumbridgeGuide and Shopkeepers / between
  LumbridgeGuide and SeekingArrows).

### Live-found bug #1 (FIXED): lazy-resolution failure

The first post-integration goblin leg (8 kills) landed all kills with empty
ground and 0 script-7192 frames — the plugin was inert. Root cause:
`hub.Load()` runs Register at `cmd/server/server.go:185`, but
`installNpcDropsWiring` runs at :298 (after the Store loads), and
`newNpcDrops()` **snapshotted** the (nil) installed source/roller into
struct fields. This is the Phase 32 hub-contract violation (surfaces
installed after plugin Load must be resolved lazily, never captured in
Register — same class as Phase 32's Register-time captures and Phase 48's
nil-manager capture). Fix (`internal/content/npcdrops/npcdrops.go`): seam
accessors `tableSource()` / `tableRoller()` / `dropReloader()` /
`wildernessCheck()` — field first (test injection keeps working), else the
currently installed seam read at call time; all use sites call the
accessors. Regression test `TestPluginFiresWhenSeamsInstalledAfterLoad`
(hub.Load() first with nil seams, install after, then emit the death —
the kill must roll). The existing test installed seams BEFORE Load, so it
never caught this.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 33 packages (new
  `internal/content/npcdrops`; hub 63/108 firable, unchanged — no new hub
  family, `onNpcDeath` already firable; the `npc-drops:*` custom events
  ride the generic `OnCustomEvent` mechanism).
- `go test ./... -p 1 -count=1 -v`: **5,300 passing test cases
  (4,054 top-level + 1,246 subtests), 33 packages, 0 failures, 0 skips**
  — baseline 5,247 / 32 (Phase 49), so +53 tests, +1 package (51 new
  top-level: loader 8, roller 22, integration 18, wiring/converter 3;
  +2 subtests from the extended loader table cases). (First run
  failed only on the three `pluginwiring_test.go` expected-list
  mismatches; fixed, re-run clean. A final confirmation run after all
  live legs also passed 33/33, exit 0.)
- `go test -race ./...`: **exit 0, 33/33 packages ok, 0 DATA RACE
  warnings** — fully clean, no flakes this time (the known
  `TestEngineTicksAtRate` flake, the known §20 `cmd/server` race-load
  timing issues, and the known `TestReloginRoundTripOverWebSocket`
  disconnect-save timeout did not trigger).

### Live legs (all six PASS on the final harness)

Harness `/home/hatch/smoke50/`, `./run_smoke50.sh <mode>`, one leg per
invocation, fresh temp root + fresh server binary each run, real rev-242
cache. The driver observes GROUND_ITEMS_DELTA opcode 55 upserts + loot-
tracker script 7192 frames (opcode 170); script args are the exact
per-kill record (item ids + amounts), ground upserts prove placement.
Stackable drops merge into earlier kills' stacks on the same tile, so
ground quantities can exceed the kill's own amount (e.g. bolts 877 x16 =
8 + 8) — script args give the exact per-kill amount. Random rolls cannot
be verified exactly; the legs verify the deterministic parts (always
drops on every kill, table membership of every observed drop, amounts
within range) plus the live tertiary hits below.

- **Goblin 3036, `goblin_drop_table1`, 8 kills — PASS.** Always Bones 526
  ×1 on all 8. Main roll: Coins 995 ×15 (kills 1, 2, 4), Hammer 2347
  (kills 3, 8), Bronze spear 1237 (kill 5), Bronze bolts 877 ×8
  (kill 6), Goblin mail 288 (kill 7). One find retry (attempt 3, no live
  goblin in range). Earlier runs also observed Water rune 555 ×6, Chef's
  hat 1949, Goblin book 10999, and the tertiary **Ensouled goblin head
  13448 (1/35)** live.
- **Chicken 1174, `chicken`, 8 kills — PASS.** Always Bones 526 ×1 +
  Raw chicken 2138 ×1 on all 8. Main roll: Feather 314 ×5 (kills 2–5, 8),
  ×15 (kill 7), Nothing (kills 1, 6). Two find retries (attempts 3–4).
- **Cow 2790, `cow`, 6 kills — PASS.** Always Cowhide 1739 ×1 + Raw beef
  2132 ×1 + Bones 526 ×1 on all 6 (all-always table, no weighted roll).
  Tertiary **Clue scroll (beginner) 23182 (1/128)** hit on kills 3 and 5.
- **Man 3106, `man`, 12 kills — PASS.** Always Bones 526 ×1 on all 12.
  Main roll: Coins 995 ×3 (kills 1, 3, 4, 8, 10, 12) / ×5 (kills 6, 11),
  Bronze bolts 877 ×6 (kill 2) / ×10 (kills 5, 7 — range [2,12]),
  Earth talisman 1440 (kill 9). Separate-roll **Energy potion(3) 3010
  (1/8)** hit on kills 1 and 8 — the independent separate roll fires
  alongside the main roll, as in TS. No herb-subtable hit in 12 kills
  (23/128; P(0) ≈ 0.09 — unlucky, not a defect). Two find retries.
- **Rat 2854, `rat`, 6 kills — PASS.** The `rat` table has 0 main
  entries and 3 conditional tertiaries. Kill 1: **Rat bone 7836 (1/4)**.
  Kills 2–6: no drops at all (0 script frames) — the empty table rolls
  nothing, and the 1/1 Rat's tail (Witch's Potion only) never dropped
  (correct: no quest plugin answers `allowed: true`), nor the Looting
  bag (not Wilderness). Two find retries.
- **Fire giant 2079, `fire_giant`, 3 kills — PASS** (final run; 9 landed
  kills across three runs, all correct). Always Big bones 532 ×1 on all
  3. Final run: Fire rune 554 ×150 (kill 1), Coins 995 ×60 (kill 2),
  **Uncut sapphire 1623 (kill 3)** — the shared `gem` subtable (11/128)
  resolved live. Earlier runs also observed Fire rune 554 ×37, Chaos
  rune 562 ×5, and tertiary **Fire giant bone 7872 (1/4)** on 3 of 6
  kills. The Fire giant bone carries a Rag and Bone Man II quest note —
  it dropped because no quest system exists to gate it (planned
  over-drop, §7 residual 5 / gap 33). The **rare drop table (1/128) was
  not hit in 9 fire-giant kills** (P(no hit) ≈ 0.93) — RDT stays
  unit-covered only (roller tests cover the nested
  rareDrop→gem→megaRare chain with scripted RNG). The herb subtable
  (19/128) was likewise not hit live — unit-covered only.

### Harness fixes made during live verification (harness only, no repo code)

The first goblin runs failed on observation, not on drops — script 7192
proved the drops were placed while the driver saw empty ground. Three
successive harness fixes, each diagnosed from the raw ground-upsert log:

1. The driver filtered ground items to the NPC's tile **at attack time**;
   goblins wander while fighting, so drops landed elsewhere. Fixed to
   the death tile (`npcTileOf`, the last known tile after removal).
2. The recorded death tile still lagged the server death tile by up to 4
   tiles (goblins) and 11 tiles (cows) — client sync lag and/or freed
   runtime-index reuse by newly spawning NPCs. Fixed to a **tile-free**
   collection: in this single-player harness the only first-seen stacks
   after the per-kill mark are this kill's drops (static spawns predate
   the mark; re-sends/reveals are deduped).
3. The server re-sends visible stacks in GROUND_ITEMS (54) snapshots
   (e.g. when the player moves between kills), so an earlier kill's stack
   could be counted again (chicken kill 3 counted 526 ×2). Fixed by
   deduping on **(stack id, qty)**: a re-send carries the same id AND qty
   and is skipped, while a stackable merge reuses the id with a larger
   qty and is counted once with the merged qty.
4. The fire-giant leg retried the same unreachable giant (idx 449)
   three times (90 s timeouts behind dungeon walls). Fixed by
   blacklisting timed-out runtime indices so the finder picks a different
   NPC; a blacklisted index is an abandoned attempt, not a drops failure
   (the attempt budget still bounds the leg). The final fire-giant run
   needed no blacklist.

Two unexplained single occurrences, neither reproduced on re-run, recorded
honestly: (a) the first cow run's kill 6 produced 0 script-7192 frames
(the NPC left the client's sync before dying server-side, or the attack
never engaged — a harness false-positive of the same class as the
fire-giant timeouts); (b) the first fire-giant runs' 90 s timeouts (see
4 above). Neither is a drops defect: every landed kill in every run
dropped correctly.

### NOT observed live (unit-covered only)

- Rare drop table roll (1/128 on fire giant; 0 hits in 9 kills) and the
  herb subtable roll — both covered by roller tests with scripted RNG.
- Noted drops: no noted drop row was hit live (the note-id swap itself is
  unit-tested both ways).
- `::reloaddrops`: unit-tested (reload refills the shared map in place);
  not run live against a running server.
- `npc-drops:claim` / `:table` / `:add` / `:roll` / `:generated` /
  `:location` listeners: the events are emitted; no consumers exist yet.
- Killer-less NPC deaths drop nothing (TS-faithful: `onNpcDeath` never
  fires without a killer) — observed only as the absence of drops on
  non-player deaths in unit tests, not as a live leg.
- The 100-tick public reveal / 300-tick despawn of dropped stacks was not
  timed live.

### Files changed (no commits)

- New: `internal/definition/npcdrops.go`,
  `internal/definition/npcdrops_test.go`,
  `internal/content/npcdrops/types.go`,
  `internal/content/npcdrops/roller.go`,
  `internal/content/npcdrops/roller_test.go`,
  `internal/content/npcdrops/npcdrops.go`,
  `internal/content/npcdrops/npcdrops_test.go`,
  `cmd/server/npcdropswiring.go`, `cmd/server/npcdropswiring_test.go`,
  `PHASE50_PLAN.md`.
- Modified: `internal/definition/loader.go` (load call + Store fields),
  `internal/definition/doc.go`, `internal/definition/fixtures_test.go`,
  `internal/definition/loader_test.go`, `cmd/server/server.go` (wiring
  call site), `cmd/server/pluginwiring.go` (blank import),
  `cmd/server/pluginwiring_test.go` (expected lists),
  `cmd/server/definitionwiring_test.go`, `cmd/server/server_test.go`,
  `cmd/server/shopwiring_test.go`, `cmd/server/chatwiring_test.go`
  (empty drop file in hand-built definition dirs), `PORT_STATUS.md`.

