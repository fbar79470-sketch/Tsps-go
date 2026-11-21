# Phase 43 — Smithing

## Scope

Port the Smithing skill (Tyler's 6th skill pick, 2026-11-21 02:38 PST, after Mining).
Upstream: `server/plugins/skills/Smithing.plugin.js` (70 lines) + 12 units under
`server/plugins/skills/smithing/` + `SkillActions.js` (already ported, Phase 20) =
14 true files, 2,120 lines, F2P. This is the first full read of Smithing
(SKILLS_SURVEY.md §11: the survey sweep-screened it only).

Units (all under `~/tsps-bfd81e3/server/plugins/skills/`):

| File | Lines | Role |
|---|---|---|
| `Smithing.plugin.js` | 70 | Entry registrar + `useItemOnObject` router + exported API |
| `smithing/Core.Smithing.js` | 17 | Shared `api.core`/`api`, `questComplete()` |
| `smithing/Data.Smithing.js` | 158 | Cache-enum anvil products + `smithing.json` resolution |
| `smithing/Sessions.Smithing.js` | 139 | One per-tick task driving all make-X loops + `timeline()` |
| `smithing/Smelting.Smithing.js` | 345 | Furnace smelting + Smelt menu |
| `smithing/Anvil.Smithing.js` | 339 | Anvil interface 312 + smith loop |
| `smithing/Cannonballs.Smithing.js` | 113 | Cannonballs at furnaces |
| `smithing/Gear.Smithing.js` | 108 | Ring of forging, Varrock armour |
| `smithing/Godswords.Smithing.js` | 211 | Godsword blades/hilts, Dismantle |
| `smithing/DragonShields.Smithing.js` | 229 | Sq shield, visage shields, dragon forge, wyvern shield |
| `smithing/Spears.Smithing.js` | 93 | Barbarian anvil spears/hastas |
| `smithing/Superheat.Smithing.js` | 162 | Superheat Item spell |
| `smithing/Common.Smithing.js` | 106 | messageBox/spam, hammer check, uniform, level boost, cache lookups |

Out of scope: **Blast Furnace** (`server/plugins/minigames/blastfurnace/`, 1,044 lines —
a separate minigame phase; it requires `Smithing.plugin.js` and shares
`SMELTING_RECIPES`, so the entry exports `loadData` for it) and Giants' Foundry
(separate; only shares the Smiths' uniform chance). Quests stay deferred
(Doric's Quest picked but postponed 2026-11-16).

## Grounding — upstream behavior

### Entry router (`Smithing.plugin.js:23-29`, register `:57-69`)

`useItemOnObject` by object name: Furnace / Small furnace / Lovakite furnace →
Cannonballs then Smelting; "Rusted anvil" → Anvil; anvil names → Godswords →
DragonShields → Anvil; shield-machine loc id → DragonShields. Registers
`onPlayerDisconnect` (stop sessions, forget anvil state),
`onCustomEvent("level-up:popup-opened", Sessions.pause)`,
`onItemOnObject(useItemOnObject, {noted:false})`, and calls each unit's
`attach(api)`. Exports for reuse: `SMELTING_RECIPES`, `FURNACE_OBJECT_IDS`,
`quantityForSmeltAction`, `hasIngredients`, `consumeIngredients`, `smeltingXp`,
`resolveCatalyst`, `loadData`, `startBotSmelting`, `isSmeltingActive`.

### Smelting (`Smelting.Smithing.js`)

- Smelt menu is a shared skillmulti `CreationMenu`, **mode 13**, title
  "What would you like to smelt?", op flags bits 1–5 (Make 1/5/10/X/All),
  cap 28 (`Smelting.Smithing.js:22-30`). Ore-on-furnace: a lone unambiguous ore
  smelts once; otherwise opens the menu.
- Timing: interval **5 ticks**, bar + XP on the **4th tick**, anim
  `human_furnace` (899) + sound 2725 (`:8-15`, `:27-28`). Iron fails 50%:
  "The ore is too impure and you fail to refine it."
- Extras: Smithing catalyst (half coal, double XP, one per bar); coal bag via
  `storage:amount` / `storage:take` events; goldsmith gauntlets (or
  Smithing/max cape via `skillcape:has-perk`) +33.7 XP on gold; "perfect" gold
  ore falls back to the Family Crest recipe; molten glass is a **Crafting**
  recipe (`skill:"CRAFTING"`, interval 2, firstDelay 2, finishDelay 0,
  byproduct BUCKET) smelted at the same furnace; Lovakite furnace smelts
  lovakite only.
- Gear hooks into `smithing:smelt-outcome`: ring of forging (140 charges,
  persist `smithing:ring-of-forging-charges`, iron never fails, melts at 0 and
  stops the session); Varrock armour 10% double-bar at the Edgeville furnace
  only (`varrock_diary_furnace`), tier-gated by metal.

### Anvil (`Anvil.Smithing.js`)

- Interface **312** ("smithing"): server only sets varbit
  `smithing_bar_type` + varp `makexcrafting` (remembered quantity, All
  default); client draws products via scripts 415→430. Quantity row
  components 3–7 (1/5/10/X/All), product components 9–38. Click sets varbits
  `smithing_bar_lasttype` / `smithing_item_lasttype`, closes interface, plays
  `human_smithing` (898) same tick; first item at **3 ticks**, then every
  **5** (4 on a Smiths' uniform roll: 20% per piece, 100% full set). Spam
  message "You hammer the \<metal\> and make …". Requires a hammer (incl.
  Imcando, either hand). No bar → message box "You should select an item from
  your inventory and use it on the anvil."; over-level product closes
  interface with a level message box. Rusted anvil (Lumbridge) is bronze-only.
  Bar-on-anvil opens the interface on that bar; blurite bar instead opens a
  `CreationMenu` (bolts/limbs, The Knight's Sword). Session stops if the
  player moves/teleports/dies (station snapshot: location + teleport serial +
  area).
- `smithing:anvil-access` emit gates anvil use (Dorics Quest listens; unported
  in Go).

### Other units (skimmed)

- **Cannonballs** (113): 4 balls/bar, double mould 2 bars → 8; first batch
  8 ticks, then 10; needs Dwarf Cannon; offered as Smelt-menu extras per
  carried bar. XP/levels in `smithing.json`.
- **Godswords** (211): 80 Smithing, 100 XP per forge; shard pieces → pairs →
  blade; blade + hilt = godsword (no skill); `onItemOnItem` join-by-hand only
  messages; `onItemAction` Dismantle ×5 godswords (`:203-208`). Timed via
  `Sessions.timeline` (anim at 0/3, result at 6 ticks).
- **DragonShields** (229): sq shield 60/75 XP; visage shields 90/**2,000 XP**
  (draconic/skeletal visage + anti-dragon shield); dragon forge
  (`ds2_ac_forge_anvil`, after Dragon Slayer II) kiteshield 75/1,000, platebody
  90/2,000; wyvern shield on shield machine (`fossil_lab_shield_maker`) 66
  Smithing **and** Magic, 2,000 XP each, drains Magic to 0. Only hook is
  `onItemOnItem(halvesByHand)` (`:226`); the rest is reached via the entry
  router.
- **Spears** (93): barbarian anvil only (Otto's Grotto, Barbarian Village),
  bar + matching log → spear/hasta, 4-tick interval, levels 5–90 / XP 25–150.
  Lock: `trained()` returns true when no "Barbarian Training" quest is
  registered, so it is **open until that quest exists** (fail-open, unlike
  most quest gates here).
- **Superheat** (162): 43 Magic, 4 fire + 1 nature rune, 53 Magic XP, cast
  every 3 ticks; iron ore + 2 coal → steel if 30 Smithing else iron (never
  fails); catalyst halves coal / doubles Smithing XP only; jogre bones →
  burnt jogre bones (Cooking 30, 25 Cooking XP). Anim 725, graphic 148
  (fail 85), sounds 190/191, tab-switch script 915. Uses `core.Spell`
  subclass + `actionInterval`.
- **Sessions** (139): single 1-tick `Task` (`onServerStartup`) advances every
  player's session; `begin` (message/anim) then `step` (make item)
  `finishDelay` later; stops on move/death/logout/level-up popup;
  `skills:action-interval` and `skills:actions-per-cycle` queries per cycle.

### Hooks used (all verified against the Go hub)

- Entry: `onPlayerDisconnect`, `onCustomEvent("level-up:popup-opened")`,
  `onItemOnObject` (firable, `internal/plugins/objects.go:643`).
- Smelting: `onServerStartup`, `onCustomEvent("smithing:bar-for-ore")`,
  `onObjectInteraction("Furnace" {Smelt,Use} / "Small furnace" /
  "Lovakite furnace")`.
- Anvil: `onServerStartup`, `onObjectInteraction("Anvil" Smith /
  "An experimental anvil" Use / "Rusted anvil" Smith)`,
  `onInterfaceActionButton` (firable, `internal/plugins/interfaces.go:170` —
  there is NO `onInterfaceAction` in TS; the survey flag is a substring match
  of `onInterfaceActionButton`). `OnInterfaceActionClick`
  (`interfaces.go:144`) is the other ported form; Smithing needs Button.
- Gear: `persistAttribute("smithing:ring-of-forging-charges", 140)`,
  `onCustomEvent("smithing:smelt-outcome")`,
  `onItemAction("Ring of forging" {Check, Break})`.
- Godswords: `onItemOnItem`, `onItemAction` ×5 (Dismantle).
- DragonShields: `onItemOnItem` only.
- Spears: `persistAttribute("barbarian-training:smithing", false)`,
  `onObjectInteraction("Barbarian anvil")`, `onItemOnObject`.
- Superheat: **`onSpellOnItem`** (only user of it in the skill).
- Cross-skill events: Smithing emits `skillcape:has-perk` (query, no Go
  answerer) and `smithing:anvil-access` (Dorics Quest listens);
  `Superheat.Smithing.js` requires `Smithing.plugin.js` back.

### Engine needs already present / missing

Present: CreationMenu, `OnItemOnObject`, `OnObjectInteraction`,
`OnInterfaceActionButton`, `OnItemAction`, `OnItemOnItem`, `onServerStartup`,
`persistAttribute`, custom events, SkillActions equivalents, `TaskManager`
per-tick task, message boxes, entered-amount prompt, varbit/varp sends.
**Missing — `onSpellOnItem` is NOT STARTED in Go**
(`internal/listeners/listeners.go:207`, `interfaceaction.go:59`,
`magicitem.go:289-297`: `castSpellOnItem` reads false, no `emitSpellOnItem`
hook family). Superheat cannot fire until that seam ports — this phase builds
it (listener dispatch + hub family + registration), the way Phase 35 built
`onNpcInteraction` and Phase 33 built `onItemOnItem`.
**Unanswered (record as gaps, do not invent):** `quest:is-complete` (quest
gates fail-closed: blurite, cannonballs, dragon forge locked; Spears
fail-open per above); `skillcape:has-perk` (gold XP boost never applies);
`level-up:popup-opened` never emitted in Go (session pause dead);
`storage:amount`/`storage:take` (coal bag); `skills:level-boost` /
`skills:actions-per-cycle` / `skills:produced`; interface 312 + varbits
`smithing_bar_type` / `smithing_bar_lasttype` / `smithing_item_lasttype` +
varp `makexcrafting` need cache verification (rev-242);
`player.getTeleportSerial` / `getArea` for station snapshots.

## Grounding — data files and cache (probe-verified 2026-11-21)

Both files imported verbatim 2026-11-21 on Tyler's approval (widget tap
"Import both Smithing files"), SHA-256 verified identical to upstream:

- `data/definitions/smithing.json` — 11,856 bytes, SHA-256
  `0ce29227df26dc849d5e6ea505cf3fded7c368ca291bb71e33044d200a2ad5af`
  (265 lines). Top-level keys: `smelting` (14 recipes), `cannonballs` (6),
  `blurite` (2), `spears` (6), `phrases` (35), `objPhrases` (2),
  `questNames` (7), `xpPerBar` (7 bars), `productXp` (`{}` — empty, so every
  anvil product's XP is `xpPerBar[bar] × barsUsed`). Recipe entry e.g. Bronze:
  `{"name":"Bronze bar","bar":"BRONZE_BAR","level":1,"xp":6.2,"ingredients":[["COPPER_ORE",1],["TIN_ORE",1]],...}`.
  Item keys are `ItemIdentifiers` UPPER_SNAKE names, resolved at load
  (`Data.Smithing.js:29-44` — a bad name throws on load). Spears' spear/hasta
  keys are lowercase obj gamevals (resolved via `getObjId`,
  `Data.Smithing.js:126-135`).
- `data/definitions/smithing-anvil-layout.json` — 28,300 bytes, SHA-256
  `ab2b2222589d6bf8bc65a0326d5441b54e38e68b328d48d0334f00d3c5c91524`
  (1,226 lines). `bars` (7): bronze 2349/1, iron 2351/2, steel 2353/3,
  mithril 2359/4, adamantite 2361/5, runite 2363/6, lovakite 13354/7; slots
  27/28/29/27/26/26/25 = **188 total**. Slot entry:
  `{"component":9,"item":1205,"obj":"bronze_dagger","label":"Dagger"}`.
  The layout holds NO levels, bar counts, amounts, or XP — those all come
  from cache enums at runtime.

Cache probe (temporary test vs real rev-242 cache, since deleted):

- `smithing.json` keys: **40 resolved / 4 missing** (lowercase-name →
  `GetObjId` gameval lookup, the Fletching/Cooking pattern). All 40 map to
  the correct item (display names verified, e.g. `ADAMANTITE_BAR`→2361,
  `CUPRONICKEL_BAR`→32892, `LEAD_BAR`→32889, `PERFECT_GOLD_BAR`→2365).
  All 12 spear/hasta keys resolve.
- The 4 unresolvable keys need `ItemIds`-style fallback constants (Phase 36
  precedent — transcribe from `ItemIdentifiers.ts`, verify display names
  against the rev-242 cache, never invent an id):
  - `BLURITE_BOLTS_UNF_` → **9376** (`xbows_crossbow_bolts_blurite_unfeathered`)
  - `BLURITE_LIMBS` → **9422** (`xbows_crossbow_limbs_blurite`)
  - `BUCKET_OF_SAND` → **1783** (`bucket_sand`)
  - `STEEL_CANNONBALL` → **no cache item at all** (no `steel_cannonball`
    gameval; no steel cannonball in 31906–31916: bronze 31906, iron 31908,
    mithril 31910, adamant 31912, rune 31914; 31916 dragon, 31922 steel
    chainshot, 31936 steel incendiary). **Loader decision:** check
    `ItemIdentifiers.ts` for a `STEEL_CANNONBALL` constant first — if TS has
    one, use it as the fallback (matching TS load behavior, which resolves
    via ItemIdentifiers, not cache gamevals); if no such constant exists,
    **drop the steel cannonball entry** and record it as a gap. Never fail
    the boot on it (TS boots fine with the entry present).
- Anvil layout: **188/188 slots resolve, 0 id mismatches** — every slot's
  `obj` gameval resolves to exactly the `item` id in the file.
- Enums 844/845/846 (config archive 8, `GetEnumValues`): each has **188
  entries**, keys exactly = the 188 layout slot item ids, 0 in either
  direction unmatched. Semantics spot-verified on bronze slots: **844 =
  amount made** (1 mostly; 15 nails/arrowheads, 10 dart tips/bolts, 5
  knives/javelin tips); **845 = bars consumed** (1 mostly; 2 scimitar/
  longsword/full helm/sq shield/claws, 3 2h/warhammer/battleaxe/chainbody/
  platelegs/plateskirt/kiteshield, 5 platebody and keel parts); **846 =
  level required** (dagger 0, axe 0, mace 2, med helm 3, bolts 3, … platebody
  18). **Quirk: enum 846 stores `0` for exactly 2 keys — 1205 (Bronze
  dagger) and 1351 (Bronze axe), the only level-1 anvil products. 0 must be
  read as level 1.** Enum 1101 (slotIndex, keyed by
  `(interfaceId<<16)|component` per `Data.Smithing.js:62-89`) still needs a
  cache check at implementation time; if it does not exist in rev-242,
  derive slotIndex from layout order and record it.
- Smelting XP floats: Bronze 6.2, Iron 12.5, Steel 17.5, Gold 22.5, Mithril
  30, Adamantite 37.5, Runite 50, Lovakite 20. `xpPerBar`: bronze 12.5 …
  runite 75, lovakite 60. All truncate to int at the Go XP grant (gap 26a).

## Grounding — Go seams (verified read-only)

- **CreationMenu:** `internal/game/creationmenu.go` — `NewCreationMenu(title,
  items, action, options)` (`:92`); `CreationMenuOptions` (`:67`) with
  `Labels`, `*MaxAmount`, `*LastAmount`, `Mode` (default 0 = Make).
  Constants: group 270, first item component 15, max quantity 28, max items
  18, last-item varp 2673. Send path `internal/session/creationmenu.go`:
  script 2046 via `SendInterfaceScriptWithArgs` → opcode **110**
  WIDGET_RUN_SCRIPT (NOT 170); mount uses primitives only — NOT
  `SendChatboxInterface`. Selection callback reaches content via
  `internal/listeners/npc.go:953-970` (group 270 + child ≥ 15 →
  `menu.Execute(itemID, amount)`). Content call sites: Fletching
  `internal/content/fletching/shared.go:1581-1623` (Mode from recipe's menu,
  MaxAmount = most batches any choice can make, capped 28); Cooking
  `internal/content/cooking/common.go:293-308` (modes 0 and 6).
- **Loader pattern (copy Fletching/Cooking, NOT `internal/definition`):**
  a `Tables` type + `Load(data, r IDResolver)`, `LoadFile`, `LoadFromCache`
  in the content package's `tables.go`. Fletching
  `internal/content/fletching/tables.go:294-335`, `IDResolver` `:199-203`,
  `cacheResolver` `:205-213` (UPPER_SNAKE → lowercase gameval →
  `defs.GetObjId`); Cooking `internal/content/cooking/tables.go:528-557`.
  Failure handling: a `resolver` accumulates every missing name and `Load`
  fails closed listing all of them; nil cache / nil resolver / missing or
  malformed file / empty top-level arrays all fail. **No id is ever
  invented.** Fallback tables for names with no (correct) gameval:
  Fletching `itemIDFallbacks` (`item_ids.go:35`, checked before the cache);
  Cooking `itemIDConstants` (`item_ids.go:3-47`). Wiring:
  `cmd/server/fletchingwiring.go:30` / `cookingwiring.go:36` /
  `miningwiring.go:29` (`install*Tables`, called from `newPluginHub` in
  `pluginwiring.go:54-78` in F2P order Fishing → Mining → Woodcutting /
  members Fletching → Mining → Woodcutting). Load failure **fails the boot**;
  nil defs (NoCache) → tables stay uninstalled and handlers degrade to
  no-ops. `SetTables` must run **before hub Load** (Register builds indexes
  from the tables).
- **Mining as the model** (`internal/content/mining/`, 13 files): package
  `var Plugin = shared.plugin()` (`mining.go:189-203`, `Name: "Mining"`,
  `Members: false`); `register` no-ops if `tables == nil`, then registers
  `OnPlayerDisconnect`, `OnCustomEvent` answers, `PersistAttribute`,
  `OnObjectInteractionNamed` per rock name. Shared setters `SetTables` /
  `SetMembersWorld` / `SetIsMembersItem` (`mining.go:163-180`); **lazy**
  TaskManager/ObjectManager resolution at use time, never captured in
  Register (hub contract).
- **SkillActions / XP:** `SkillManager.StartSkillable` exists
  (`internal/game/skills.go:1258-1313`) but **no skill content package
  actually calls it** — Mining/Fletching/Cooking run their own 1-tick task
  loops; Smithing does the same (Sessions 1-tick task). `ActionInterval` /
  `PauseForLevelUp` (`internal/plugins/skillactions/skillactions.go:134`,
  `:81`); Mining uses both (`mining.go:1040`, `:1881`). XP grant:
  `SkillManager.AddExperiences(skill, xp int)` (`skills.go:455`);
  `SkillSmithing` = 13 (`skills.go:182`); Mining truncates at the call
  site (`mining.go:692`). Hub count: 58/108 families firable, 62 `Api.On*`
  methods, 4 named overloads (PORT_STATUS.md:727).

## Plan

Work split (tables first — the content workers code against the Tables API):

1. **Tables + loader + wiring** (`internal/content/smithing/tables.go`,
   `item_ids.go`, `smithingwiring.go`, tests): `Tables` type with
   `Load`/`LoadFile`/`LoadFromCache` per the Fletching/Cooking pattern;
   `IDResolver` + `cacheResolver` (UPPER_SNAKE → lowercase gameval);
   fallback constants for `BLURITE_BOLTS_UNF_` (9376), `BLURITE_LIMBS`
   (9422), `BUCKET_OF_SAND` (1783); steel-cannonball decision per the
   grounding above (ItemIdentifiers.ts check first). Load both files;
   resolve anvil products from layout + enums 844/845/846 (and 1101 if
   present in rev-242, else layout order) with the enum-846-zero-means-1
   rule. `LoadFromCache` load-proven against the real rev-242 cache with a
   temporary test, then deleted. Wire `installSmithingTables` in F2P order
   after Mining (and members order alongside Fletching/Mining — match the
   existing wiring order conventions); `SetTables` before hub Load.
   **Tables API (frozen for the content workers):**
   - `type Tables struct` with `Smelting []SmeltRecipe`, `Cannonballs []CannonballRecipe`, `Blurite []BluriteProduct`, `Spears []SpearRecipe`, `Anvil []AnvilProduct`, `Phrases map[string]string`, `ObjPhrases map[string]string`, `QuestNames map[string]string`, `XPPerBar map[int]float64`
   - `type SmeltRecipe struct { Name string; BarID, Bar int; Level int; XP float64; Ingredients []Ingredient; BarPhrase, SmeltMessage, RetrieveMessage, MissingMessage string; Quest string; SuccessChance float64; HasSuccessChance bool; FailMessage string; Members bool; Furnace string; NotInMenu bool; Skill string; Byproducts []Ingredient; Interval, FirstDelay, FinishDelay int; HasTiming bool }`
   - `type Ingredient struct { ItemID, Amount int }`
   - `type CannonballRecipe struct { BallID, BarID, Level int; XPPerBar float64 }`
   - `type BluriteProduct struct { ItemID, Level int; XP float64; Amount int; Phrase string }`
   - `type SpearRecipe struct { BarID, LogID, Level int; XP float64; SpearID, HastaID int }`
   - `type AnvilProduct struct { BarID, BarType, Component, SlotIndex, ItemID, Level, Bars, Amount int; Metal, Obj, Label, Phrase string; XP float64; Members bool; Quest string }`
   - `func Load(data *SmithingData, r IDResolver) (*Tables, error)`,
     `func LoadFile(path string, r IDResolver) (*Tables, error)`,
     `func LoadFromCache(defs *cache.CacheDefinitions) (*Tables, error)`,
     `func (t *Tables) SmeltByBar(barID int) *SmeltRecipe`,
     `func (t *Tables) AnvilByBar(barID int) []AnvilProduct`,
     `func (t *Tables) AnvilByComponent(barType, component int) *AnvilProduct`
   - Package-level `var tables *Tables`, `func SetTables(t *Tables)`,
     `func TablesLoaded() bool`. Content handlers no-op when `tables == nil`.
2. **Sessions + Smelting + Anvil + Cannonballs + Spears + Gear + Common**
   (`sessions.go`, `smelting.go`, `anvil.go`, `cannonballs.go`, `spears.go`,
   `gear.go`, `common.go`, `smithing.go`): entry router (`useItemOnObject`
   by object name, same order as TS), Smelt CreationMenu mode 13, 5-tick
   smelt loop (bar + XP on 4th tick, anim 899, sound 2725, iron 50% fail),
   anvil interface 312 (varbit `smithing_bar_type` + varp `makexcrafting`,
   quantity components 3–7, product components 9–38 via
   `OnInterfaceActionButton`, first item 3 ticks then every 5, uniform
   20%/piece → 4-tick, hammer check incl. Imcando, Rusted anvil bronze-only,
   bar-on-anvil opens on that bar, blurite bar → CreationMenu bolts/limbs),
   cannonballs as Smelt-menu extras (4/bar, double mould 8, first batch 8
   ticks then 10, Dwarf Cannon gate fail-closed), spears on barbarian anvil
   (4-tick, fail-open lock), Gear (ring of forging 140 charges via
   persistAttribute + `smithing:smelt-outcome`, Varrock armour 10% double-bar
   at Edgeville furnace only), Common helpers (messageBox/spam, hammer check,
   uniform, level boost query, cache lookups). Sessions 1-tick task driving
   all make-X loops with `begin`/`step`/`finishDelay`, stops on
   move/teleport/death/logout; `timeline()` one-offs.
3. **Godswords + DragonShields + Superheat + `onSpellOnItem` seam**
   (`godswords.go`, `dragonshields.go`, `superheat.go`,
   `internal/plugins/spells.go` + `internal/listeners/magicitem.go`):
   Godswords (shard pairs → blade, blade + hilt, Dismantle ×5 via
   `onItemAction`, join-by-hand messages via `onItemOnItem`, 6-tick
   timeline); DragonShields (sq shield, visage shields, dragon forge,
   wyvern shield on shield machine, `onItemOnItem` halves only); Superheat
   (43 Magic, 4 fire + 1 nature, 53 Magic XP, every 3 ticks, iron → steel
   at 30 Smithing else iron, catalyst, jogre bones → burnt jogre bones).
   **The `onSpellOnItem` seam is new engine work:** hub family
   (`Api.OnSpellOnItem` + `Hub.EmitSpellOnItem`, mirroring the existing
   spell families in `internal/plugins/spells.go`), listener dispatch from
   `castSpellOnItem` in `internal/listeners/magicitem.go` (currently reads
   false — wire the real dispatch, keeping the existing spell-cast flow),
   hub-count update (59/108). Superheat is its first and only consumer in
   this phase.
4. **Smoke harness + live verification** (`/home/hatch/smoke43/`, fresh temp
   root, real rev-242 cache): legs — smelt bronze bar at a furnace (bar + XP
   on the wire), smith a product at an anvil via interface 312 button click,
   smelt-menu open (CreationMenu 2046 on the wire), full-inventory refusal,
   walk-away stops. Legs run singly (RUNS override). Sub-units unit-covered
   only must be stated as such, not claimed live.
5. **Docs:** PORT_STATUS.md content row (14 of 577), new §26 residual gaps,
   "What was verified" entry, and complete the data-import paragraph (loader
   now exists — update the "no loader exists yet" line). PHASE43_PLAN.md
   Results section.

## Verification criteria

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...` all clean. Use Go 1.25.1 at
  `~/workspace/tools/go/bin/go` (do NOT install another toolchain; a worker
  previously downloaded Go 1.25.0 to `~/go-toolchain` because `go` was not on
  PATH — use the repo toolchain path instead).
- Full suite `-p 1 -count=1 -v`: exact pass/fail/skip counts + package count
  (baseline Phase 42: 4,766 passing, 25 packages).
- `go test -race ./...`: clean; the known `TestEngineTicksAtRate` flake may
  hit `internal/game` — re-run that package once and say so plainly.
- Live legs in `/home/hatch/smoke43/`: all checks pass, 0 failures; report
  per-leg check counts.
- No stray files, no `.js`, probe tests deleted, `live36` and `~/tsps-full`
  untouched, no commits, no new dependencies, no hand-edited JSON.

## Gaps to record (§26, all documented at the port sites, none silent)

- (a) XP integer truncation (smelting 6.2 → 6, gold 22.5 → 22,
  `xpPerBar` 12.5 → 12; infernal Smithing 3.6 → 3 already noted in gap
  25(c)) — structural, same gap as 16–19/25.
- (b) No `SkillLevelUp` emitter; `level-up:popup-opened` never emitted in Go
  (session pause dead).
- (c) Quest gates fail-closed (`quest:is-complete` unanswered): blurite,
  cannonballs, dragon forge locked; Spears fail-open per TS.
- (d) `skillcape:has-perk` unanswered (goldsmith XP boost never applies);
  coal bag `storage:amount`/`storage:take` unanswered; `skills:level-boost`
  / `skills:actions-per-cycle` / `skills:produced` unanswered.
- (e) `smithing:anvil-access` unanswered (Dorics Quest unported).
- (f) Steel cannonball entry (see loader decision above).
- (g) Enum 1101 slotIndex derivation if rev-242 lacks it.
- (h) Sub-units unit-covered only, not observed live (list them at the end).
- (i) Blast Furnace NOT STARTED (separate minigame phase); Giants' Foundry
  NOT STARTED.

## Results (2026-11-21, verified)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
  `CGO_ENABLED=0 go build ./...` clean. 808 `.go` files,
  26 packages (new `internal/content/smithing`, 20 files:
  13 source + 7 test; new `onSpellOnItem` hub family —
  59/108 firable, 63 `Api.On*` methods).
- Full suite `-p 1 -count=1 -v`: all 26 packages pass —
  4,863 passing (3,681 top-level + 1,182 subtests), 0
  failures, 0 skips (4,766 from Phase 42 + 97 new: 10
  tables/loader, 28 core loops, 34 godswords/
  dragonshields/superheat, 3 onSpellOnItem seam, 1
  object-name-resolver wiring test, 3 wiring tests updated
  for the Smithing load order, plus subtests).
- `go test -race ./...`: clean, 26/26 packages, no data
  race; the known `TestEngineTicksAtRate` flake did not hit
  this run.
- Live legs (`/home/hatch/smoke43/`, real server binary +
  real rev-242 cache, legs run singly): smelt-bronze PASS
  7/7 (1 bronze bar, 6 XP, anim 899, sound 2725),
  smelt-menu PASS 4/4 (script 2046 on opcode 110),
  anvil-smith PASS 8/8 (interface 312, 5 bronze daggers,
  60 XP, anim 898), walkaway PASS 5/5 (exactly 1 bar, 6
  XP), ore-on-furnace PASS 3/3, bar-on-anvil PASS 2/2,
  full PASS 9/9 (full-inventory smelt still produces —
  TS-faithful; nails with 2 free slots produce 0). 38
  checks, 0 failures. Sub-units unit-covered only:
  Cannonballs, Gear, Godswords, DragonShields, Spears,
  Superheat (incl. the `onSpellOnItem` seam).
- One live-found wiring gap fixed in-phase:
  `SetObjectNameResolver` was never called in `cmd/server`,
  so the item-on-object router was a no-op — wired from
  the cache definitions (cooking's `SetObjectInfo`
  precedent) and re-verified live by the ore-on-furnace +
  bar-on-anvil legs.
- Loader decisions (probe-verified): steel cannonball
  KEPT via `STEEL_CANNONBALL → 2` (`ItemIdentifiers.ts:5`,
  cache item 2 "Steel cannonball"); fifth fallback
  `BUCKET → 1925` (molten-glass byproduct); enum 1101
  EXISTS (30 entries, slotIndex = component − 8); enum 846
  zero-means-1 for 1205/1351.
- No stray files, no `.js`, probe tests deleted, `live36`
  and `~/tsps-full` untouched, no commits (parent handles
  git). No new dependencies, no hand-edited JSON.

## Parent verification note (2026-11-21)
- Build/vet/gofmt/CGO0 clean; full suite re-run by the parent:
  4,863 PASS, 0 fail, 0 skip, 26 packages (matches the
  coordinator's 3,681 top-level + 1,182 subtests). Live logs
  re-checked by the parent: 38 checks, 0 failures across the
  seven legs. Docs confirmed: hub 59/108, content 14/577,
  §26 residuals.
- Race: the coordinator's clean run did NOT hold. The parent's
  first full-race run reproduced the phase-37-suspect race —
  two genuine DATA RACE warnings in `cmd/server`
  (`SetCombatCycleSource` + `SetAggressionCombatBridge`
  writes during a second `buildServer` vs a first test
  server's in-flight tick). Root cause: `Engine.Stop`
  waited for the scheduler loop but not for a tick already
  in flight. Fixed by the parent in this phase
  (`internal/game/engine.go`: `sync.WaitGroup` over
  in-flight ticks, `Stop` waits; `TestEngineTicksAtRate`'s
  post-Stop poll became an immediate assertion, the
  regression check). After the fix: full-race run failed
  only on the known `TestEngineTicksAtRate` flake (no DATA
  RACE), `cmd/server` race runs 2 and 3 clean,
  `internal/game` race re-run clean. PORT_STATUS §20 and
  the phase-43 race line corrected to record this.
