# First-quest survey

Phase 25, 2026-11-16. Analysis only: nothing was ported, no Go code
changed, no quest was picked. The migration plan (§3 item 5) says the
first quest must not be picked until dialogue, objects, and combat
exist, and that selection needs its own survey preferring the shortest
`*.Quest.js` with no combat, no instancing, and no cross-quest
dependencies. All 11 engine milestones are DONE, so this survey ran.
It recommends one quest, with measured evidence. **The pick is Tyler's.**

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only checkout
at `~/tsps-bfd81e3`. Quest content lives at
`server/plugins/quests/`. All counts below were re-measured for this
survey; method is stated per table so every number is reproducible.

## 1. Measured inventory (reconciled with the plan)

Method: `find` + `wc -l` under `server/plugins/quests/`; the `QUESTS`
array parsed with node from `Quests.plugin.js`.

| Measure | Count |
|---|---|
| JS files under `quests/` (whole directory) | 273 files, 277,911 lines (matches the plan exactly) |
| Entries in the `QUESTS` array (`Quests.plugin.js:9`) | 193 |
| Top-level `*.Quest.js` files (`quests/*.Quest.js`) | 193 files, 261,277 lines |
| `*.Quest.js` files by `find` (incl. subdirectories) | 194 files, 261,405 lines |
| All JS under `quests/` (193 quest files + 5 unit files + 1 subdir quest file) | 199 files, 261,969 lines |
| Registered quests on a members world | 195 (193 + 2 Mage Arena miniquest-likes) |
| F2P quests (`F2P_QUESTS`, `Quests.plugin.js:73`) | 24 |
| Median quest file | 1,199 lines |
| Smallest / largest quest file | 181 (LearningTheRopes) / 9,118 (DesertTreasureII; 9,958 with its `dt2/` unit) |
| Shared engine `QuestRuntime.js` | 1,007 lines |

Reconciliation note: the plan (§1) says "193 `*.Quest.js` files
(261,969 lines)". The 261,969 figure is the line count of *all* JS
under `quests/` (199 files); the 193 quest files alone are 261,277
lines. The 194th `*.Quest.js` file,
`quests/ides-of-milk/CowbellAmulet.Quest.js` (128 lines), is not a
quest: it never calls `registerQuest`, it is required by
`IdesOfMilk.Quest.js:1080`, and it implements amulet item behaviour.
Every `QUESTS` entry has exactly one matching file, and every
top-level `*.Quest.js` file is in the list.

The 30 shortest quest files (`wc -l quests/*.Quest.js | sort -n`):

| Lines | Quest | Lines | Quest |
|---|---|---|---|
| 181 | LearningTheRopes | 461 | RestlessGhost |
| 188 | DoricsQuest | 476 | RovingElves |
| 204 | WhileGuthixSleeps * | 486 | ScorpionCatcher |
| 222 | DruidicRitual | 487 | SkippyAndTheMogres |
| 247 | CooksAssistant | 493 | TribalTotem |
| 249 | WitchsPotion | 500 | KnightsSword |
| 251 | ImpCatcher | 501 | FamilyCrest |
| 314 | CurseOfTheEmptyLord | 506 | RuneMysteries |
| 314 | RecruitmentDrive * | 510 | LostCity |
| 330 | BearYourSoul | 511 | SheepShearer |
| 330 | VampyreSlayer | 538 | ElementalWorkshopI |
| 359 | HisFaithfulServants | 554 | DwarfCannon |
| 365 | JunglePotion | 558 | RagAndBoneManII |
| 378 | ANightAtTheTheatre | 562 | HeroesQuest |
| 436 | MonksFriend | | |

\* File size is misleading: WhileGuthixSleeps requires all 9 `wgs/`
unit files (1,380 lines; true total 1,584) and RecruitmentDrive
requires all 8 `recruitmentdrive/` files (1,952 lines; true total
2,266). See §3. Every quest that requires from a unit subdirectory
requires *all* files in it (17 cases, verified by following the
`require` strings).

## 2. Whole-catalogue sweep

Method: lexical flag classification over all 194 `*.Quest.js` files,
computed twice (Python `re.search` and `grep -rEl`) with exact
agreement on every count. Flags are per-file only (unit subdirectories
excluded — see §3). Patterns:

| Group | Pattern |
|---|---|
| COMBAT | `getCombat\|onNpcDeath\|registerNpcCombatMethodProvider\|onCombatHit\|onNpcHit\|CombatMethod\|dealDamage\|isInCombat` |
| INSTANCING (strict) | `PrivateArea\|TemplatedInstanceArea\|TemplateRegion\|createInstance\|MapRegionReplacement\|replaceRegion` |
| CROSSQUEST | `quest:is-complete\|quest:get-stage\|quest:is-started\|requiredQuests\|requiresQuest` |
| DIALOGUE | `onNpcDialogueVariant\|onNpcDialogueCondition\|npc-dialogue:` |
| OBJECT | `onObjectInteraction\|onItemOnObject` |
| ITEM | `onItemAction\|onItemOnItem\|onItemOnNpc` |
| NPC | `onNpcInteraction\|spawnNpc\|onBeforeNpcInteraction` |
| INTERFACE | `onInterfaceActionButton\|onInterfaceAction\|openInterface\|sendInterface` |
| TIMER | `Timer\|scheduleTask\|TaskManager` |
| AREA | `registerArea\|onZoneEnter\|onZoneExit` |

Flag counts (of 194): COMBAT 129, INSTANCING 79 strict (25 under the
strong pattern `extends/new PrivateArea|TemplatedInstanceArea|…` —
the other 54 only mention `getPrivateArea()`), CROSSQUEST 127,
DIALOGUE 190, OBJECT 171, ITEM 156, NPC 167, INTERFACE 74, TIMER 112,
AREA 65. Combat+instancing overlap 64; combat-only 65;
instancing-only 15.

Shared-unit use: `QuestRuntime` is required by 193 files (all but
CowbellAmulet); `registerQuest` appears in 193; `startTranscript` in
173; `refreshQuestList` in 144; `startDialogue` in 70; `giveReward`
in 25; `openBook` in 13; `QuestScenes` in 35; `QuestPuzzleLocks` in
2; `QuestLockpicking` in 2. No quest file requires another quest's
file (the only `.Quest`-suffixed require is IdesOfMilk requiring its
own CowbellAmulet unit).

Clean list — NO combat AND NO instancing AND NO cross-quest flags
(strict patterns), all 25, LOC ascending. Flags: D=dialogue,
O=object, I=item, N=npc, F=interface, T=timer, A=area. F2P per
`F2P_QUESTS`. "Read?" marks the files read in full for this survey;
the rest are sweep-screened only and are NOT ranked.

| Lines | Flags | Quest | F2P? | Read? |
|---|---|---|---|---|
| 181 | ------- | LearningTheRopes | F2P | Yes — ruled out (§5) |
| 188 | DO----- | DoricsQuest | F2P | Yes — ranked 1 |
| 222 | DO----- | DruidicRitual | members | Yes — ranked 4 |
| 247 | D------ | CooksAssistant | F2P | Yes — ranked 2 |
| 251 | D---FT- | ImpCatcher | F2P | Yes — ruled out (§5) |
| 314 | D--N-T- | CurseOfTheEmptyLord | members | Yes — ruled out (§5) |
| 330 | DOIN-T- | BearYourSoul | members | Yes — ruled out (§5) |
| 436 | DO----A | MonksFriend | members | No |
| 506 | D------ | RuneMysteries | F2P | Yes — ranked 3 |
| 511 | D-IN-T- | SheepShearer | F2P | No |
| 576 | D-IN-TA | GertrudesCat | members | No |
| 595 | D-IN--- | AlfredGrimhandsBarcrawl | members | No |
| 611 | DOIN--- | MurderMystery | members | No |
| 623 | DOI--T- | RagAndBoneManI | members | No |
| 630 | D-INF-- | XMarksTheSpot | F2P | No |
| 679 | DOINF-- | GoblinDiplomacy | F2P | No |
| 721 | DO-N-T- | FishingContest | members | No |
| 743 | DOI-FTA | GardenOfDeath | members | No |
| 808 | DOIN--- | InSearchOfKnowledge | members | No |
| 954 | DO-N--- | MourningsEndPartII | members | No |
| 1022 | DOIN--- | EadgarsRuse | members | No |
| 1054 | -OINFT- | DaddysHome | members | No |
| 1204 | DOI---- | EnlightenedJourney | members | No |
| 1210 | D--N-T- | ChildrenOfTheSun | members | No |
| 1260 | DOI---- | TaiBwoWannaiTrio | members | No |

Caveat: flags are lexical, so comments/strings can match (notably
TIMER and the weak INSTANCING matches), and per-file flags understate
split quests (RecruitmentDrive's main file flags only CROSSQUEST —
its combat and Area live in subdir files). Treat counts as screening
signals; every ranked quest below was read in full, and Imp Catcher
is the proof that file-only flags mislead: it is sweep-clean but
builds a `PrivateArea` cutscene through the shared `QuestScenes`
factory. Under the strong instancing pattern, 5 more files would
qualify (TribalTotem 493, DeathPlateau 807, TheGolem 1028,
Pandemonium 1989, plus CowbellAmulet) — none was read in full.

## 3. True-size traps: subdirectory requires

| Quest file | Own | + Subdir | = True | Subdir |
|---|---|---|---|---|
| WhileGuthixSleeps | 204 | 1,380 | 1,584 (7.8×) | wgs (9 files) |
| RecruitmentDrive | 314 | 1,952 | 2,266 (7.2×) | recruitmentdrive (8) |
| SeaSlug | 632 | 419 | 1,051 | seaslug (1) |
| TheHeartOfDarkness | 1,069 | 939 | 2,008 | hod (2) |
| GhostsAhoy | 1,352 | 1,208 | 2,560 | ghostsahoy (5) |
| Wanted | 1,585 | 749 | 2,334 | wanted (1) |
| ObservatoryQuest | 924 | 345 | 1,269 | observatory (1) |
| SlugMenace | 1,547 | 962 | 2,509 | slugmenace (3) |
| ValeTotems | 585 | 295 | 880 † | valetotems (1) |
| LostTribe | 1,455 | 441 | 1,896 | losttribe (3) |
| GrandTree | 1,032 | 123 | 1,155 | grandtree (1) |
| Ratcatchers | 1,435 | 235 | 1,670 | ratcatchers (1) |
| ARuffSituation | 1,357 | 238 | 1,595 | ruff (1) |
| CrabQuest | 1,796 | 458 | 2,254 | crab (2) |
| MakingHistory | 1,541 | 165 | 1,706 | makinghistory (1) |
| DesertTreasureII | 9,118 | 840 | 9,958 | dt2 (1) |
| TheBloodMoonRises | 8,997 | 1,022 | 10,019 | bmr (7) |

† ValeTotems also requires two minigames units outside the quests
tree (not counted). `questhelper/` (9 files, 1,152 lines) is required
by no quest file — it is Quest Helper HTTP/runtime, not quest
content. Five sibling unit files sit directly under `quests/` (564
lines total), each required only by its own quest. None of the 25
clean-list files has a subdir/sibling require.

## 4. The shared engine a first quest needs

`QuestRuntime.js` (1,007 lines) is the shared engine; every quest
file requires it. What it provides, measured:

- `registerQuest(api, def)` (`QuestRuntime.js:772`): def fields
  across the 193 defs — `key`/`name`/`startedValue`/
  `completionValue`/`questPoints` in all 193; `varpId` in 184,
  `varbitId` in 120 (every def has at least one; when `varbitId`
  is set the stage mirrors to the varbit *instead* of the varp);
  `buildJournal` in 192; `onReward` in 175; `xpRewards` in 170
  (**display only** — the engine never grants XP from it);
  `rewardItemId` in 72 (actually granted via `giveReward`);
  `varpValue` in 28, `varbitValue` in 47; `completionJingleId`
  in 28 (default 238); `scrollItemId` in 87; `rewardItemLabel`
  in 112; `otherRewards` in 158. There is NO `requirements`
  field and NO `quest:started` event — do not invent them.
  Requirements are enforced by the cache quest table (QuestHelper
  eligibility only) and by per-quest dialogue gating.
- Handle methods: `getStage`/`setStage`/`isStarted`/`isComplete`
  (thresholds, not equality), `complete(player, options)` with an
  idempotency guard, running `onReward` *before* the jingle /
  congratulations message / completion scroll, then emitting
  `quest:completed`. Stage is a persisted player attribute
  `quest.<key>.stage` (default 0), mirrored to the varp/varbit.
  Quest points are attribute `quest.points`, mirrored to varp 101.
  `setStage` emits `quest:stage-changed` and refreshes the quest
  list; `sendQuestVarps` re-sends every stage on login.
- Query events, answered once by `registerQuestWidgets`:
  `quest:is-complete`, `quest:is-started`, `quest:get-stage`
  (mutable request objects, key-first then fuzzy name lookup,
  unknown quests left unanswered). Plus `quest:stage-changed`,
  `quest:completed`, `quest:list-refresh`, and the
  `npc-dialogue:shop` version picker.
- UI in `registerQuestWidgets`: quest list group 399 (native CS2
  rows; server sends op flags + header stats via varp 101 and
  varbits 1782/6347/11877/14162), journal overlay group 119,
  overview group 782, completion scroll group 153 (deferred via
  1-tick chatbox-free polling), books groups 392/680. Login
  coalescing holds per-quest refreshes and sends once.
- Other exports: `startTranscript` (190 files, 1,575 calls in
  `quests/` — the dominant dialogue mechanism; Go Phase 14 ported
  the playback engine, so a Go analogue is a thin shim),
  `startDialogue` (9 files, 33 calls — a hand-rolled step runner;
  do not conflate with `NpcDialogues.plugin.js`'s
  different-signature function), `loadTranscripts` (48 files),
  `giveReward` (28 files; force-add, overflow to the floor),
  `openBook`/`openIndexedBook` (book quests only),
  `refreshQuestList` (144 files), `sendQuestVarps`,
  `getRegisteredQuests`, `questNamed`. `QuestList.QuestRuntime.js`
  (50 lines) joins cache DB table 0 (`quest`) for list rows.
- Companions, none needed for quest 1: `Equipment.Quests` (83
  lines, equip gates), `Shops.Quests` (124 lines, shop gates),
  `SirTiffy.Quests` (53 lines, 4 specific members' quests),
  `QuestCape` (210 lines, needs the near-complete catalogue +
  diaries), `QuestCommand` (85 lines, `::quest` admin command —
  recommended as a *test aid* in the port phase, not gameplay),
  `QuestHelper` (1,152 lines + an external HTTP engine on
  127.0.0.1:16471 — do not port; Go is WS-only),
  `QuestPuzzleLocks` (2 users), `QuestLockpicking` (2 users),
  `QuestScenes` (358 lines, 35 users — cutscene/instancing
  machinery, out of scope for a minimal quest),
  `ChatheadAnimations` (27 lines + 101 KB data, cosmetic).

**Minimal engine subset for quest 1** (judgment, grounded in the
above; ~550–700 lines of Go + ~400–600 lines of tests, excluding
the quest content itself): registry + handle with the full def
struct; the three query events as typed request structs on the
hub's custom-event mechanism; `complete()` in TS order; login
re-send; journal (119) + completion scroll (153); list flags +
header stats (399) with the cache-table join; a `startTranscript`
analogue over the Phase 14 transcript engine; the `::quest`
command as a test aid. All interface/varp/jingle senders it needs
already exist in Go (verified: `SendString`, `SendSubInterface`,
`SendInterfaceScriptArgs`, `SendInterfaceFlagsRange`,
`SendItemOnInterfaces`, `SendInterfaceRemoval`,
`CloseSubInterface`, `SendJingle`, `SendVarp`/`SendVarbit`,
`GetDbTableRows`). Defer: overview 782 + start-text + Show-on-Map
+ wiki ops, books, ChatheadAnimations, Equipment/Shops/SirTiffy
gates, QuestCape, QuestHelper, locks, scenes.

## 5. Go hook-surface mapping

The Go hub has 45 of 108 hook families firable
(`internal/plugins/hooks.go`, unfired-family table). Families a
quest candidate needs, and their Go status:

Firable (usable today): `onPlayerLogin`, `onPlayerLogout`,
`onPlayerDisconnect`, `onPlayerProcess`, `onServerStartup`,
`onServerShutdown`, `onCustomEvent`, `onCommand`, `onSocialPacket`,
the 10 object families (`onObjectRoute`, `onObjectInteraction`,
`onBeforeObjectInteraction`, `onObjectClick` + First–FifthClick,
`onItemOnObject`), `onNpcDialogueVariant`,
`onNpcDialogueCondition`, `onZoneEnter`, `onZoneExit`, the 11
combat families (`onNpcDeath`, `onNpcAttackOption`,
`onPlayerAttack`, `onPlayerDeath`, `onPlayerDeathItemDrop`,
`onPlayerDeathDrops`, `onCombatHitPrepare/Roll/Damage/Resolved`,
`onCombatAttackDistance`), `onCanShop`, `onCanStockItem`,
`onCanTrade`, `onTradeRequest`, `onTradeCompleted`,
`onInterfaceActionClick`, `onInterfaceActionButton`,
`onPlayerLevelUp`, `onExperienceGained`, `onCanGainExperience`,
`onWorldEntityInteraction`. The 8 `npc-dialogue:*` custom events
ride the custom-event surface with rich mutable payloads
(`Before`/`After`/`Skip`/`End`/`Handled`/`Box`/`Text`/`Warning` —
`internal/game/transcript.go:557-760`, verified). The object
handler's return-`false`-falls-through contract is ported
(`internal/plugins/objects.go:158-168`).

Table-only (a quest needing one is blocked until its system
ports): `onNpcInteraction`, `onBeforeNpcInteraction`,
`onNpcClick` + First–FourthClick, `onNpcRoute`, `onItemOnNpc`,
`onItemAction`, `onItemFirstAction`, `onItemOnItem`,
`onItemOnPlayer`, `onItemOnGroundItem`, `onGroundItemClick`,
`onGroundItemPickup`, `onSpellOnNpc`, `onSpellOnObject`,
`onSpellOnItem`, `onSpellCast`, `onNpcBeforeDeath`,
`onNpcHitModify`, `onCanAttack`, `onPlayerHitModify`,
`onPlayerDealtDamage`, `onCanTeleport`, `onCanEquip`,
`onCanUnequip`, `onCanBank`, `onCanEat`, `onCanDrink`,
`onCanUseItem`, `onWalkRoute`, `onPlayerMapSquareChange`,
`onButtonClick`, `onButton`, `onCanLogout`, `onCanSpawnNpc`,
plus the death/policy/prayer/special gates. `CreationMenu` has no
Go type (Phase 20 gap). There is no game-side `performAnimation`
equivalent: the sync encoder carries an animation mask bit, but
no `Mobile` animation state feeds the flush — a quest using
`performAnimation` needs that small seam built, or the emote
stays a documented gap. There is no inventory force-add with
floor overflow (`giveReward` needs a small seam; the ground-item
manager exists in `internal/game` with `Register`, but no
content-facing spawn seam). The dialogue-data starters in the
repo are `{}`: every quest's transcript entries are a documented
data need, staged as fixtures for live verification (Phase 23
precedent) — nothing is copied from upstream JSON.

## 6. Ranked shortlist (all read in full)

Ranking criteria, in plan order: shortest, no combat, no
instancing, no cross-quest dependencies, uses only firable hook
families, F2P preferred. Estimated port size = quest file + the
§4 engine subset (~550–700 lines Go) + listed seams; the engine
subset is shared, so it is counted once across all candidates.

### 1. Doric's Quest — 188 lines — RECOMMENDED

- File: `quests/DoricsQuest.Quest.js`. True total 188 (QuestRuntime
  only; no sibling/data requires). F2P. 1 quest point, Mining
  1,300 XP, 180 coins, jingle 154, varp 31, stages 0/10/100.
- Hooks, all firable: `onNpcDialogueVariant`,
  `onNpcDialogueCondition`, `npc-dialogue:line/action` (×2)
  `/menu`, `onObjectInteraction("Anvil", {Smith})`,
  `onObjectInteraction("Doric's Whetstone", {Use})`, plus one
  `smithing:anvil-access` listen (no Go producer — Smithing is
  unported — so it never fires; the two object hooks cover the
  same gate for direct clicks, and the listen degrades
  gracefully).
- No combat, no instancing, standalone (no quest queries, no
  `requiredQuests`, no deferral). One NPC (Doric, id 3893), one
  start hook (a `before` on the "Clay is what I use…" line, with
  an inventory-full refusal), one hand-in (action step ids,
  exact-amounts condition branch), one menu warning (Mining < 15
  recommendation, not a gate). Uses `startTranscript` for the
  pre-quest anvil/whetstone attempts.
- Dialogue data needed: page "Doric's Quest" (variants
  `starting-off`,
  `starting-off-returning-with-all-the-materials`,
  `starting-off-talking-to-doric-without-all-of-the-materials`,
  `starting-off-attempting-to-use-his-anvils-before-the-quest`,
  `starting-off-attempting-to-use-his-whetstone-before-the-quest`)
  + page "Doric" (`after-doric-s-quest`). All six variants
  verified present in upstream `npc-dialogues.json`.
- Blockers/seams: (a) the §4 engine subset; (b) the
  `performAnimation` seam for the hand-in emotes (player 3071,
  Doric 3072, Doric found by an 8-tile NPC scan); (c) the
  `smithing:anvil-access` request struct needs a `Denied` field
  for future Smithing compatibility (`denied` is read only by
  the Smithing gate, `Anvil.Smithing.js:285-287` — it is not
  part of the object-interaction contract); (d) the object-hook
  claim/deny precedence must be exact: `handleSmithy` returns
  `false` for every non-Doric anvil (Go's fall-through contract
  covers this) and claims Doric's anvil/whetstone — with no
  Smithing plugin in Go, a non-Doric anvil click falls through
  to the honest "Nothing interesting happens" default, and the
  port must preserve return-`false` so future Smithing can claim
  it. Items: clay 434 ×6, copper ore 436 ×4, iron ore 440 ×2,
  bronze pickaxe 1265 (starter), steel pickaxe 1269 (scroll
  display). Objects: anvil 2031, whetstone 10641.
- Estimated port size: ~190 lines of quest content + the shared
  engine subset + the animation seam (~60 lines). The smallest
  faithful quest port available, and the most representative of
  the dialogue+object quest shape (171 of 194 quests use object
  hooks; 190 use dialogue).

### 2. Cook's Assistant — 247 lines

- File: `quests/CooksAssistant.Quest.js`. True total 247. F2P. 1
  quest point, Cooking 300 XP, jingle 152. Single NPC (Cook,
  4626). No combat, no instancing, standalone.
- Hooks, all firable: `onNpcDialogueVariant`,
  `onNpcDialogueCondition`, `npc-dialogue:line/hook/action`,
  `onPlayerLogin`, one `persistAttribute` (a 3-bit delivered
  bitmask, `cooks-assistant:delivered-ingredients`).
- Emits the `bottomless-milk:holds` / `bottomless-milk:spend`
  query pair, answered by the unported BottomlessMilkBucket
  plugin; unanswered, both stay false — graceful degradation.
  Two-phase start (hook + line `after`); the heaviest
  transcript surgery of the shortlist: per-line `skip`/`after`
  delivery, bulk delivery with an item box, dynamic `event.text`
  rewrite for the missing-ingredients list, partial hand-in
  persisting across conversations via the bitmask.
- Blockers/seams: the §4 engine subset only — no animation,
  object, item, or combat surface. But the port leans hardest
  on exact dialogue-engine fidelity (step-id fidelity, line
  skip/rewrite ordering, `after` execution timing). No new
  seams; highest dialogue-mutation risk.
- Estimated port size: ~250 lines of quest content + the shared
  engine subset. Ranked below Doric's on size and on
  transcript-surgery risk, despite needing no new seams.

### 3. Rune Mysteries — 506 lines

- File: `quests/RuneMysteries.Quest.js`. True total 506. F2P. 1
  quest point, Runecraft 500 XP, air talisman reward, jingle
  154, varp 63, 7 stages (0–6). Three NPCs (Duke Horacio,
  Archmage Sedridor in 3 id forms, Aubury in 3 id forms).
- Hooks, all firable: `onNpcDialogueVariant`,
  `onNpcDialogueCondition`, `npc-dialogue:hook/action/line/
  choice`, `onPlayerLogin`, five `persistAttribute` calls
  (talisman/doubted/package/notes/delivered flags).
- No combat, no instancing. Cross-quest coupling is graceful
  only: it reads Temple of the Eye's stage attribute directly
  to defer Sedridor at that quest's tower/portal stages
  (absent attribute → 0 → no deferral), and returns `null`
  for the shared Duke Horacio after the talisman handover so
  other quests' dialogue plays. Item hand-ins with recovery
  flows (lost talisman/package/notes reissued), a `startTranscript`
  `select` step-filter (the `tailAfter` resume logic + an
  injected message step), one `performAnimation` (829, tea),
  three `sendSoundEffect` calls (sender exists), one direct
  `sendVarbit` (talisman flag 13720, restored on login).
- Blockers/seams: the §4 engine subset + the `performAnimation`
  seam + a `startTranscript` analogue that supports the `select`
  step-filter callback (not just page/variant). Largest
  dialogue surface of the shortlist (~15 step ids, ~14
  variants, 3 NPCs, 7 stages, 5 attributes).
- Estimated port size: ~510 lines of quest content + the shared
  engine subset + the animation seam. Ranked third on size and
  surface width, despite being the most representative
  multi-NPC delivery-chain quest; a strong second quest after
  Doric's proves the engine.

### 4. Druidic Ritual — 222 lines

- File: `quests/DruidicRitual.Quest.js`. True total 222.
  MEMBERS-ONLY (absent from `F2P_QUESTS`). 4 quest points (the
  highest of the shortlist), Herblore 250 XP, default jingle
  238. Two NPCs (Kaqemeex 5045, Sanfew 5044).
- Hooks, all firable: `onNpcDialogueVariant`,
  `onNpcDialogueCondition`, `npc-dialogue:hook/line/action/
  condition`, `onItemOnObject` (raw→enchanted meat dipping in
  the Cauldron of Thunder 2142, stage-gated with a refusal
  message), `onPlayerLogin` (list refresh).
- No combat, no instancing. Cross-quest: shared-NPC deferral —
  it looks up the `one_small_favour` quest handle and yields
  Sanfew at that quest's stages 10/11/30 (graceful when OSF is
  absent/unstarted); Eadgar's Ruse also owns Sanfew. The
  hand-in is via a *condition* step (the transcript has no
  hand-in action), consuming the four meats when the positive
  branch is chosen — an unusual dialogue-engine semantic the
  port must reproduce (side effects inside a condition-step
  handler; the Go condition event has no mutable read-back
  fields, but side effects through the player pointer work).
  Post-quest it returns `null` for Sanfew, releasing the NPC.
- Blockers/seams: the §4 engine subset + `getRegisteredQuests`
  (part of the subset) for the deferral lookup. No animation
  or new seams. Members-only load gating must be honored.
- Estimated port size: ~225 lines of quest content + the shared
  engine subset. Ranked fourth: shorter than Cook's/Rune
  Mysteries, but members-only and carrying shared-NPC
  arbitration that only pays off when the other Sanfew quests
  land.

### 5. Witch's Potion — 249 lines

- File: `quests/WitchsPotion.Quest.js`. True total 249. F2P. 1
  quest point, Magic 325 XP, jingle 154. One NPC (Hetty 4619)
  + one object (Cauldron 2024).
- Hooks, all firable: `onNpcDialogueVariant`,
  `onNpcDialogueCondition`, `npc-dialogue:hook/action` (×2)
  `/line`, `onNpcDeath` (rat-tail drop), `onObjectFirstClick`.
  Uses BOTH `startTranscript` and `startDialogue` (the
  hand-rolled step runner — only 9 quest files use it).
- COMBAT (narrow): the `onNpcDeath` hook drops a rat's tail
  from killed rats while stages 1–2 and none carried, via
  ground-item registration at the corpse location, over a
  hand-maintained 23-id rat list. No attack hooks, no boss.
  No instancing, standalone. Also a set-equality condition
  prose parser and a cauldron click branching between a
  transcript and an inline dialogue.
- Blockers/seams: the §4 engine subset + a content-facing
  ground-item spawn seam (manager exists, no content seam) +
  the `startDialogue` step runner as a second dialogue
  mechanism (~90 lines) + killer attribution / no-duplicate
  gating through the death pipeline. Ranked last: the only
  shortlist quest needing combat-pipeline coupling and a
  second dialogue mechanism.
- Estimated port size: ~250 lines of quest content + the shared
  engine subset + ground-spawn seam + step runner.

## 7. Ruled out (read in full, with reasons)

| Quest | True size | Reason |
|---|---|---|
| LearningTheRopes | 181 + Tutorial Island (1,908) | No dialogue at all; a mirror of Tutorial Island progress (shared varp 281, tutorial step values as stages). Not portable alone — needs a tutorial-progress source first. |
| ImpCatcher | 251 + QuestScenes (358) | `PrivateArea` cutscene: 19-tick choreography, tick-scheduled tasks, fade/camera/graphic/sound sends, movement blocking, logout/crash recovery via a pending attribute. Sweep-clean only because the instancing lives in the shared factory. |
| WhileGuthixSleeps | 1,584 | 8 `PrivateArea` subclasses, scripted boss fights, 10 quest prerequisites + 180 QP + skill levels, 43 attributes, 56 object hooks, `onNpcInteraction`/`onItemOnNpc`/`onSpellOnObject` (all table-only). |
| RecruitmentDrive | 2,266 | Tick-timed Course choreography (countdown-task chains, fade sequencing, held dialogue), 7 room units, agility `ObstacleRunner` dependency, 2 quest prerequisites, `onNpcInteraction`/`onGroundItemPickup`/`onWalkRoute` (table-only). |
| CurseOfTheEmptyLord | 314 | Members miniquest, 0 QP. Needs `onNpcInteraction` (table-only), `CreationMenu` (no Go type), an entered-amount prompt, ring-of-visibility NPC visibility owned by another plugin, and randomised sequence routing across 16 spawns. |
| BearYourSoul | 330 + 290 JSON | Members miniquest, 0 QP. Needs `onNpcInteraction` + `onItemAction` (both table-only), `openBook`, and transcript-gap patching via fragile `event.text` rewrites + a hand-built chain. |
| VampyreSlayer | 330 | F2P, but needs `onNpcBeforeDeath` (table-only: `preventDeath` + heal + delayed hammer feedback) and the Count Draynor boss `PrivateArea` owned by another plugin. |

## 8. Recommendation

**Port Doric's Quest first**, with the §4 minimal engine subset in
the same phase (the subset has no observable effect without a quest,
and the quest has no stage storage without the subset). It is the
shortest standalone quest (188 lines), F2P, uses only firable hook
families, needs exactly one small new seam (player/NPC animation),
and exercises the two surfaces 90%+ of quests need (dialogue
variants/conditions + object interaction). Cook's Assistant is the
nearest alternative if Tyler prefers zero new seams and accepts
heavier transcript surgery; Rune Mysteries is the best second quest.

This is a recommendation, not a decision. If Tyler picks a different
quest, the §4 engine subset still applies unchanged; only the
candidate-specific seams in §6 differ.

## 9. What a Doric's Quest port phase needs first (brief-ready)

Engine seams, in dependency order:

1. **QuestRuntime minimal subset** (§4): registry + handle, stage
   attribute `quest.dorics_quest.stage` (persistent key + default
   0 registered at boot, Phase 11 registry), `quest.points` +
   varp 101, the three query events, `complete()` in TS order
   (guard → QP add → `onReward` → `rewardItemId` → jingle 154 →
   congratulations message → deferred scroll → `quest:completed`
   emit), login varp re-send, journal group 119, completion
   scroll group 153, list flags + header stats group 399 with
   the cache DB-table-0 join, `startTranscript` analogue
   (page/variant; no `select` filter needed — Doric's never
   passes one), `::quest` command as a test aid.
2. **Animation seam**: `Mobile` animation state + flush wiring
   into the existing sync animation mask bit, for
   `performAnimation` (player 3071, Doric 3072). Ground in TS
   `Mobile.performAnimation` / `Animation` before building.
3. **Inventory force-add seam** for `giveReward` (overflow to the
   floor) — check the Go container API; the ground-item manager
   exists but no content-facing spawn path does.
4. **Dialogue data need** (documented, not copied): page "Doric's
   Quest" + page "Doric" entries in `npc-dialogues.json` and the
   Doric index mappings, per the Phase 23 precedent — fixture
   data staged in the temp root for live verification, removed
   afterwards. NPC Doric id 3893, objects anvil 2031 / whetstone
   10641, items clay 434 / copper ore 436 / iron ore 440 / bronze
   pickaxe 1265 / steel pickaxe 1269 — all from the cache, to be
   re-verified at port time against `NpcIdentifiers` /
   `ItemIdentifiers` / `ObjectIdentifiers`.
5. **Hook wiring to verify at port time**: the object-hook
   return-`false` fallthrough for non-Doric anvils (Go contract
   exists); the `smithing:anvil-access` listen registered with
   a `Denied`-field request struct (no producer until Smithing
   ports); the `npc-dialogue:line` `before` start commit with
   the inventory-full refusal; the hand-in action step ids
   (`wEuzw0`, `M4gVPX`) and message step ids (`NpBbL9`, `n6fQmN`,
   `UgCEfK`) against the staged fixture data.

Live verification shape (following the Phase 23/24 precedent):
real WebSocket client, real rev-242 cache, fixture dialogue data
staged in a temp root, 3 runs stable: Talk-to Doric → start
(pickaxe granted, stage 10, varp 31) → gather materials → return
→ hand-in (materials consumed, stage 100, 180 coins, Mining XP,
jingle, scroll, quest points) → post-quest variant plays;
pre-quest anvil/whetstone clicks play the attempt transcripts;
a non-Doric anvil click falls through (no claim). Out of scope
for that phase: any other quest, Smithing content, the ranged
resolver gap, the far-WALK bug, the y=3521 wall, the scrypt
check, perf work.

## 10. Method appendix (reproducibility)

All commands run from `~/tsps-bfd81e3/server/plugins/quests/`
unless noted. Counts are `wc -l` (lines including blanks and
comments), matching the plan's method.

- Directory totals: `find . -name '*.js' | wc -l`;
  `find . -name '*.js' -exec cat {} + | wc -l`.
- Quest files: `find quests -name '*.Quest.js' | wc -l`;
  `wc -l quests/*.Quest.js | sort -n` (shortest table).
- QUESTS/F2P counts: node parse of `Quests.plugin.js`
  (`require("./Quests.plugin.js").QUESTS.length`, set diff
  against the `quests/` directory listing).
- Sweep flags: the §2 patterns, run both as Python `re.search`
  per file and as `grep -rEl --include='*.Quest.js' -E
  '<pattern>'`; both runs agreed exactly. Strong instancing
  pattern: `extends PrivateArea|extends TemplatedInstanceArea|
  new PrivateArea|new TemplatedInstanceArea|TemplateRegion|
  createInstance|MapRegionReplacement|replaceRegion`.
- Subdir true sizes: follow each quest file's `require(…)` strings
  to unit files, `wc -l` each, sum. Verified every subdir quest
  requires all files in its subdir.
- Engine field census: def-block regex over the 193 quest files
  + manual count for single-line defs; export usage via
  `grep -rl` / occurrence counts excluding `QuestRuntime.js`.
- Go-side checks: `internal/plugins/hooks.go` unfired-family
  table (firable 45 / table-only 63); `internal/game/
  transcript.go:557-760` (event payloads);
  `internal/plugins/objects.go:158-168` (fall-through contract);
  sender existence via `grep -rl "func.*Send*("`; no
  `performAnimation` / force-add equivalents found by grep over
  `internal/game` + `internal/session`.
- Dialogue-entry existence: `grep -c` for each variant string in
  `server/data/definitions/npc-dialogues.json` (Doric's six
  variants: 1 occurrence each, except the shared `starting-off`
  name at 68 across pages; page "Doric's Quest" present).
- Deep dives: 12 quest files read in full (LearningTheRopes,
  DoricsQuest, DruidicRitual, CooksAssistant, WitchsPotion,
  ImpCatcher, WhileGuthixSleeps + 9 `wgs/` units,
  RecruitmentDrive + 8 `recruitmentdrive/` units,
  CurseOfTheEmptyLord, BearYourSoul, VampyreSlayer,
  RuneMysteries), plus `QuestRuntime.js`, `Quests.plugin.js`,
  and all companion modules in full.
