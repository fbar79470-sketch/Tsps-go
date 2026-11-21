# Phase 32 plan — Woodcutting port (first skill)

Dispatched 2026-11-20 under Tyler's "keep going without confirming".
Tyler CONFIRMED Woodcutting as the first skill (2026-11-19, over the
Phase 31 survey's recommended BarbarianPyres; a later Fletching widget
tap was a conflicting signal he resolved in favor of Woodcutting —
Fletching is NOT confirmed as queued after it, do not assume it is
next). Quests postponed (Doric's Quest picked but deferred).

## Grounding (read before implementing)

- Survey: `SKILLS_SURVEY.md` §2 (hook flags), §3 (data map), §4
  (cross-deps), §6 (engine gaps), §11 (honest limits — Woodcutting
  was sweep-screened only, NOT read in full by the survey).
- Upstream: `~/tsps-bfd81e3/server/plugins/skills/Woodcutting.plugin.js`
  (1,594 lines, read in full by the coordinator + Worker 1) and the
  9 files under `plugins/skills/woodcutting/` (read in full by
  Worker 2). All line numbers below are `Woodcutting.plugin.js`
  unless noted.
- Go seams: audited by Worker 3 (see §"Seam audit" below).

## Scope — core chopping loop ONLY

Port the entry file's core loop and its in-path helpers. Everything
else in the plugin defers to a later phase (see "Deferred").

### 1. Tables (Worker A)

`internal/content/woodcutting/tables.go` (+ test). All values verbatim
from the plugin file; ids are SYMBOLIC in TS (`ItemIds.*` /
`ObjectIds.*` from `IdEnums`) and must be resolved through the Go
cache — `CacheDefinitions.idForName(kind, name)` (`internal/cache/
facade.go:721`) or the equivalent public accessor. **No invented
numbers.** Verify every id resolves against the real rev-242 seed
cache (temporary test, deleted afterwards, same pattern as the
Phase 27/28/31 import checks); permanent unit tests use fixtures.

- `AXES` (lines 193–227): 31 entries worst→best, `axe(id,
  requiredLevel, tier, animationId, extra)`; `AXE_SPEED_BY_TIER =
  [0.03, 0.05, 0.09, 0.11, 0.13, 0.16, 0.19, 0.25]` (line 182);
  `AXES_BEST_FIRST = reverse(AXES)`. Extra flags: `felling`,
  `standard`, `requiredAgility`, `requiredFiremaking`, `speed`
  override (infernal 0.3). No blessed axe (comment lines 202–203).
- `CUT_CHANCE` (234–251): 14 charts (normal, oak, willow, teak,
  maple, mahogany, yew, magic, redwood, hollow, blisterwood,
  camphor, ironwood, rosewood), `[low, high]` per tier bronze→dragon;
  blisterwood has nulls for tiers 1–6 (scale off normal chart).
- `TREES` (254–651): 21 entries. Per entry: name, objectNames,
  action(s), requiredLevel, xpReward, logId, petBase (or none),
  objectIds (symbolic list, where present), cutChance chart,
  respawnTicks (+ respawnTicksMax), stumpId (fallback only),
  multi, despawnTicks, depleteChance, depletes, logMessage,
  logLine, countsLogs, respawnTimer, diaryXp (maple only),
  birdNests, interruptMessage/Animation/Sound (blisterwood only).
  Trees with no `objectIds` (hollow, arctic pine, juniper, jatoba,
  blisterwood, burnt, camphor, ironwood, rosewood) are reachable
  only by object NAME — the port must look up by name, not id.
- Derived: `TREES_BY_NAME`, pest-control clone (656–665: normal
  tree, objectIds TREE_59/TREE_60, logLine/countsLogs/respawnTimer
  false — the ONLY entry in `TREES_BY_OBJECT_ID`), `FARMED_TREES`
  (667–676: camphor/ironwood/rosewood with respawnTicks 150;
  redwood with despawnTicks 0, depleteChance 11, respawnTicks 200 —
  the two farming constants are literals with provenance
  `farming/Redwood.Farming.js:5–6`: `DEPLETION_ROLL = 11`,
  `REGROW_TICKS = 200`; do NOT port Redwood/Core.Farming.js),
  `TREE_LOG_IDS`, `NORMAL_TREE_STUMP_BY_MODEL` (25–51, 25
  model→stump mappings), `DEFAULT_TREE_STUMP_ID`.
- Nest tables: `BIRD_NESTS` (102–112), `NEST_SEEDS` (150–170, 19
  seeds with weights), `NEST_RINGS` (173–179), `SEED_NEST_IDS`,
  `SEARCHABLE_NEST_IDS`.
- `LUMBERJACK_OUTFIT` (130–137) + `LUMBERJACK_SET_BONUS = 0.005`
  + set-text 17; `SEERS_VILLAGE` region (145); constants
  (55–74): action interval 4, animation interval 4, first-roll
  delay 2, `SWING_MESSAGE`, `NO_AXE_MESSAGE`, inventory-full
  sound 2277, respawn script 5474 / timer id 2 / colour 16765184,
  logs-chopped varp `"tracking_logs_chopped"`, attribute
  `"woodcutting:logs-chopped"`, multi depletion 15/2, nest chance
  1/256, nest fall sound 1516, cape multiplier 1.1, group bonus
  max 10.

### 2. Core loop (Worker B)

`internal/content/woodcutting/woodcutting.go` (+ test). State lives
in a `*Woodcutting` struct created in `Register` — NO package-level
mutable maps (TS module-level state becomes per-registration struct
state; cf. Phase 21 race fix). Port in TS order:

- Sessions: `map[*game.Player]*session` + a 1-tick global task
  (`WoodcuttingTask`, `super(1)`, lines 1462–1475) submitted at
  Register via `api.GetTaskManager()`; the task owns the tick
  counter (`woodcuttingTick`). Explicit cancellation: the tick loop
  itself stops sessions on every TS condition (below); plus
  `onPlayerDisconnect` stops the session. Go `TaskManager.
  CancelTasks` has zero production callers — do not assume the
  engine cancels anything.
- `startWoodcutting` (1243–1303) in exact order: `woodcutting:
  validate-tree` query (allow defaults true) → best usable axe or
  `NO_AXE_MESSAGE` → tree level check → inventory check
  (`inventoryBlocks`: full inventory + `storage:room` query, room
  defaults false) → tree still there (`treeAt`, incl. lazy
  template copies — port the shape that applies in Go; private-area
  template laziness is a later gap if Go lacks the seam, document
  it) → `stopSkillable` + stop old session + `reset` combat +
  face tree → store session (nextActionTick/nextAnimationTick =
  tick + 2) → spam `SWING_MESSAGE` + play axe animation.
- `findBestUsableAxe` (710–733): best-first over `AXES_BEST_FIRST`,
  equipped weapon slot OR inventory, `canUseAxe` (current WC level
  ≥ required, current Agility ≥ requiredAgility ?? 1, current
  Firemaking ≥ requiredFiremaking ?? 1), members-axe filter on
  free worlds (`choppingAxeEligible`, via the item definition's
  members flag), felling promotion (same tier + same level +
  rations available → felling axe), fallback `woodcutting:tool`
  query (axe defaults null).
- `processWoodcuttingTick` (1305–1460) in exact order: dead/
  unregistered → delete; force movement → skip (keep session);
  movement queue non-empty → stop; level-up paused → skip;
  tree gone → stop; validate-tree → stop; out of interaction
  distance → stop; re-find axe every tick (none / axe level lost
  / tree level lost → message + stop); inventory full → full
  message + stop WITHOUT animation reset; tick despawn timer;
  animation every 4 ticks; action gate (nextActionTick, via
  `skills:action-interval` query through `skillactions.
  ActionInterval`, 4 ticks, floor 1); `woodcutting:attempt`
  query around `rollLog`; miss → continue; success → nest
  pre-emption (see §3) else the success block (see §4); then
  depletion check (see §5).
- `calculateCutChance` (748–767): chart row by axe tier, null-row
  scaling off normal; successes `1 + floor(low*(99-L)/98) +
  floor(high*(L-1)/98)` (rounded variant for scrapey only);
  chance = successes/256 clamped [0,1]. Effective level =
  current WC + guild boost (stub 0 — guild deferred) + group
  bonus (see below).
- Group bonus (777–793): +1 per chopper on the same object id +
  location + private area (incl. self), cap 10; 0 for requested
  sessions, normal trees, or in-guild players (guild stub → never
  in guild, so the exclusion never fires). Answer the
  `woodcutting:group-bonus` query.
- `rollLog` (799–802): `rand < calculateCutChance(...)`.
- Felling (834–893, 1404–1408): `fellingChop` consumes one
  Forester's ration (inventory first, else `storage:take` query —
  unanswered → not consumed) → +10% XP, 20% log skip;
  `rollForesterRationEnergy`: 30% roll, +2000 run-energy units
  when < 9000, members only. Include — it is core-path and works
  with inventory rations; storage-container support degrades
  honestly (unanswered queries).
- Lumberjack XP multiplier (820–824): include (pure equipment
  check; Forestry pieces count). Seers maple diary bonus
  (805–818): include the `diary:is-complete` query emit —
  unanswered defaults to no bonus.
- Kandarin headgear (27 lines, pure function): INCLUDE —
  `logsPerChop` returns 2 only for regular logs on members worlds
  with headgear 1–4 worn, else 1; no extra XP.
- `woodcutting:chop` listener (`requestedChop`, 1489–1500):
  include — find tree by `logId` in `FARMED_TREES` then `TREES`;
  `removeOnly` → no-log/no-XP clone; mark session `requested`.
  `woodcutting:axe` listener (`requestedAxe`, 1502–1508):
  include. `woodcutting:logs-chopped` query + login re-send:
  include — persist attribute `"woodcutting:logs-chopped"` default
  0; varp id via `CacheDefinitions.GetVarpId
  ("tracking_logs_chopped")`.
- `level-up:popup-opened`: register `pauseWoodcutting` via
  `skillactions.PauseForLevelUp` (sessions + stop callback). No
  emitter exists in Go yet (SkillLevelUp feedback NOT STARTED) —
  registration is harmless and future-proof; document that pause
  never fires until SkillLevelUp lands, so level-up currently
  does NOT pause chopping (XP still lands, `OnPlayerLevelUp`
  still fires).
- `player:emote` / `player:interaction-cleared` listeners:
  register IF the Go hub has emitters for those events (check
  first); otherwise document as gap and skip registration.
- `onPlayerDisconnect`: stop session, no animation reset.
- `equipment:effects` Lumberjack set-text: include ONLY if the
  Go `equipment:effects` emit site + `setTexts` shape exists
  (GracefulOutfit precedent) — check first; else defer with a
  note.
- Tree-down sound: `SoundWoodcuttingTreeDown` exists
  (`internal/game/sounds.go:109`, id 2734) — send via the
  Phase 29 sound path on depletion.
- Respawn overlay: `SendClientScript(5474, …)` exists via the
  PacketSender interface (`internal/game/dialogue_entries.go:
  155–158`) — send to the chopper AND all local players, packed
  coordinate `(z<<28)|(x<<14)|y`, stump id/type, timer id 2,
  ticks, colour 16765184, 0. Skip when `tree.respawnTimer ==
  false`.

### 3. Bird nests (Worker B)

- Drop (1032–1061): members world only; chance 1/256 per
  successful roll × 1.1 if the Woodcutting cape perk answers
  (`skillcape:has-perk` query — no Go answerer, defaults false)
  × `woodcutting:nest-chance` multiplier if > 1 (no listener,
  defaults 1). Nest type: weighted 100 slots (seed 65 / 60 with
  strung rabbit foot, ring 32, red/green/blue egg 1 each).
  On drop: `woodcutting:nest-drop` query (handled defaults false)
  → ground via the ground-item manager, sound 1516, red message
  `"<col=ff0000>A bird's nest falls out of the tree.</col>"`.
  **A nest pre-empts the log entirely**: no log, no log line, no
  XP, no countLog, no felling/crystal/infernal processing — but
  clue-nest rolling and depletion still run (deferred units:
  clue-nest roll is NOT called; depletion still runs).
- Search (1072–1112): `searchBirdNest(player, nestId)` as an
  exported-in-package function. Searchable: 3 egg nests, both
  seed nests, ring nest. Seed nests roll `NEST_SEEDS` (needs a
  free slot + addable); ring nests roll `NEST_RINGS`; egg nests
  give the matching bird's egg. Consumes the nest, adds an empty
  nest; seed line is a spam message; rings/eggs arrive silently.
  Full inventory → statement dialogue with
  `NEST_FULL_MESSAGE`, nest kept. Seed search emits
  `woodcutting:nest-search`. Registration to the
  `onItemFirstAction` family is the COORDINATOR's integration
  step (Worker C builds the family; Worker B exposes the
  handler) — Worker B must NOT invent the family itself.

### 4. Success block (Worker B, TS order, lines 1400–1441)

Felling ration → ration energy → XP computed (base × lumberjack,
+10% if rationed; diary query inside base) → `logAmount` via
headgear → if not skipLog: burn check (STUB: `tryBurnLog`
returns false — InfernalAxe deferred) → `prepareLog` (`storage:
gather` query, stored defaults 0; remainder is the log item) →
`countLog` unless `countsLogs == false` → log line unless
skipLog or `logLine == false` → crystal charge (STUB: no-op —
CrystalAxe deferred; axes still chop as tier-7) → **XP granted
unconditionally on success** via `AddExperiences(SkillWoodcutting,
xp)` — including burnt/skipLog cases, excluding nest pre-emption
→ `pets:skilling-roll` query emit (no answerer, defaults
unhandled) → `deliverLog` (inventory add up to addable amount;
overflow to ground via the ground-item manager) → emit
`woodcutting:success` (fire-and-forget; Guild listens — deferred,
no listener) → clue-nest roll (NOT called — deferred).

### 5. Depletion / respawn (Worker B)

- `shouldDepleteTree` (976–993): false if `depletes == false`;
  true if `!multi`; else Forestry timer at 0 (despawnTicks
  trees) / `depleteChance` 1-in-N / fallback roll 0–15 < 2.
  Forestry timers: module map keyed `"<objectId>:<x>,<y>,<z>"`,
  drain 1/tick while chopped (once per tick regardless of
  chopper count), regen 1/tick while not chopped, deleted at
  full (947–975).
- Blisterwood interrupt (1443–1453): interrupt animation (seq
  id via `CacheDefinitions.getSeqId` equivalent — check Go
  accessor first), sound 3604, spam message, stop WITHOUT
  animation reset; the tree does NOT fall.
- `depleteTree` (1194–1210): `woodcutting:deplete-tree` query
  (handled defaults false → falls); stump via `resolveStumpId`
  (model map → treeId+1 option-less stump → treeId+1
  same-name-different-model → tree.stumpId → default stump);
  `ObjectManagerAPI.ReplaceInPlace` (existing seam) for the
  swap; respawn overlay unless `respawnTimer == false`;
  `TreeRespawnTask`: delayed one-shot Go `Task` (delay =
  `rollRespawnTicks`, clamped ≥ 1; respawnTicksMax range when
  present) that deregisters the stump if present and
  re-registers the original tree (always re-register so clients
  get an explicit spawn update); abort if the private area is
  destroyed. Then tree-down sound + stop session.

### 6. Engine seam (Worker C)

Build the `onItemAction` family in Go, grounded in TS
(`PluginTypes.ts:702–730, 831–860, 1213–1225`;
`PluginManager.ts:1654–1665 emitCanUseItem, 2016–2022
emitItemAction, 4253–4330 registration`):

- `PluginItemActionEvent { Player, InterfaceID, ItemID, Slot,
  ClickType, Option, SubOpID, SubOption, Handled }` (Go form of
  `PluginItemActionEvent`; item as id, not object — follow the
  Go `ObjectInteractionEvent` precedent).
- `Api.OnItemAction(handler)` (generic) + `Api.OnItemActionNamed
  (itemName, actions map, options?)` — mirror the
  `OnObjectInteraction` / `OnObjectInteractionNamed` pair in
  `internal/plugins/objects.go`. `Api.OnItemFirstAction(handler)`
  — first-action only (clickType 1, skip if already handled,
  runs before the generic fallbacks — TS `onItemFirstAction`
  pushes a filtered hook onto the main list).
- `Hub.EmitItemAction(event)` running main hooks then fallback
  hooks, returning handled; `EmitCanUseItem` gate — include
  ONLY if it fits the existing `onCanAttack` gate pattern
  cleanly, else document as a gap (the listener degenerates to
  proceed today).
- Emit sites in `internal/listeners/itemaction.go` at the two
  documented GAP sites (`handleAction` non-first click ~line 166,
  `handleFirstAction` ~line 205), in TS order (gate → plugin
  event → equip fallback for first actions; the equip fallback
  must still run when no plugin handles it). Adapter in
  `cmd/server/` following the object-listener wiring precedent.
- Update the unfired-family table in `internal/plugins/hooks.go`
  (remove `onItemAction`/`onItemFirstAction` from table-only;
  recount families honestly — these are TWO distinct families,
  so firable goes 51 → 53 if both land).
- Tests: unit tests proving a registered handler fires through
  the listener for both first and non-first clicks, that
  `handled` stops the equip fallback, and that unhandled clicks
  still equip. Do NOT touch the drop/equip behaviour otherwise.

### 7. Registration / wiring (Worker B)

`internal/content/woodcutting/woodcutting.go`: `var Plugin =
plugins.Plugin{Name: "Woodcutting", Description: <verbatim
from the plugin file>, Members: false, Register: …}` — note:
the TS plugin has NO `members: true` (members gating is
per-feature via `isMembersWorld`); Go `Members: false`.
`Register` is attach-only. `init() { plugins.Register(Plugin) }`.
One blank-import line in `cmd/server/pluginwiring.go`
(seekingarrows/lumbridgeguide precedent). Object registrations:
27 `onObjectInteraction` calls (name + action, generated loop
over TREES — normal tree 4 names × 2 actions, 19 others × 1;
scrapey 0). `handleChop` looks up by object id (pest-control)
then by definition name, returns silently if no tree, sets
`handled = true` ALWAYS (even on start failure — fail messages
are the plugin's own).

### 8. Deferred (document as gaps, do not build)

- Guild (+ GuildData + `woodcutting-guild.json` import — needs
  Tyler's approval; do NOT import it): gate, cave, vine, roots,
  redwood entry, diary tasks, +7 boost. Core-loop touchpoints
  stubbed: `isInGuild → false`, `invisibleBoost → 0`.
- EntTrunk (needs `onNpcDeath` + `onNpcClick` on the trunk +
  `woodcutting-guild.json` ents data; killer-only sessions).
- ThickVines (Tar Swamp chop/clear; separate interaction).
- Shrine (guild shrine; needs the JSON + `onItemOnObject`).
- ClueNests (clue-nest rolls + `onItemAction("Clue nest …",
  { Open })` — needs the family Worker C builds, but the unit
  itself defers; Treasure Trails answerers are unported).
- CrystalAxe (charge drain; item-meta state — deferred; axes
  chop undrained meanwhile). InfernalAxe (burn; needs
  Firemaking `firemaking:log-xp` answerer — unported).
- SkillLevelUp feedback content (the `level-up:popup-opened`
  emitter) — NOT STARTED; pause registration is harmless.
- `storage:room` / `storage:gather` / `storage:amount` /
  `storage:take` answerers (no storage content); `pets:
  skilling-roll` (no pets); `diary:is-complete` (no diaries);
  `skillcape:has-perk` (no SkillCapes answerer); league hooks
  (`woodcutting:tool/nest-chance/nest-drop/deplete-tree/
  attempt`, `skills:action-interval`) — all degrade to
  documented defaults.
- `npc-combat-sounds` game-level seam (data shipped; wiring is
  a separate small port — do NOT fold in silently).

## Verification (coordinator)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean;
  `CGO_ENABLED=0 go build ./...` clean.
- `go test ./... -p 1 -count=1` full count (baseline 4,115);
  `go test -race` 17/17 (18/17 if a new package lands — count
  packages honestly).
- Live smoke, 3 runs stable, real rev-242 cache, temp roots
  outside the repo: bronze axe on a normal tree (logs land in
  inventory, Woodcutting XP grants, tree depletes to a stump,
  stump respawns to a tree); a higher-level tree if the seed
  allows (oak at 15+); inventory-full stop; session stops on
  walk-away and on disconnect; nest search if a nest drops
  (1/256 — likely unobserved live; unit-covered, say so).
- Docs: this file (outcome), `PORT_STATUS.md` (header numbers,
  inventory, hub counts if changed, gaps, next-modules order),
  `CONTENT_MIGRATION_PLAN.md` (skills progress), `SKILLS_SURVEY.
  md` ONLY if a survey claim proves wrong (note the correction).

## Out of scope

Any other skill (incl. Fletching, BarbarianPyres), all quest
work, gap 14 spellbook residuals, the far-WALK bug, the
y=3521–3522 map wall, the scrypt check, perf work, any new
verbatim data import. No scope expansion without TS grounding.
If the scope exceeds one phase, land the core chopping loop
first and document the remainder precisely as the next phase's
brief — do not half-land a unit.

## Outcome (2026-11-21, coordinator-verified)

Woodcutting landed as the first skill: the core chopping loop
only, live-verified end to end. All three worker streams
completed and were integrated; the coordinator fixed two
live-found bugs and one race, then re-verified everything
against the final code.

### What landed

- **Tables/data** (`internal/content/woodcutting/tables.go`,
  1,455 lines; `tables_test.go`, 6 tests): all §1 tables
  verbatim from `Woodcutting.plugin.js` with TS `file:line`
  citations; gameval-name source tables resolved via
  `Load`/`LoadFromCache` (fail-closed; 393 distinct names
  resolved against the rev-242 cache, 0 failures). **Plan
  corrections:** the plugin has exactly **30** `axe()` calls
  (plan §1 said 31 — wrong) and **24** stump-model mappings
  (plan §1 said 25 — wrong). 14 CUT_CHANCE charts, 21 TREES,
  BirdNests/Seeds/Rings, Lumberjack, constants.
- **Core loop** (`internal/content/woodcutting/woodcutting.go`,
  2,062 lines; `woodcutting_test.go`, 1,242 lines, 32 test
  functions): 1-tick global task, 4-tick roll cadence (first
  roll +2 ticks), axe selection, cut chance, group bonus,
  felling/ration energy, lumberjack multiplier, Seers maple
  diary query, Kandarin headgear, bird nests (1/256,
  members-only, pre-empting the log), depletion/respawn
  (Forestry timers, stump swap, client script 5474 overlay),
  `level-up:popup-opened` pause,
  `persistAttribute("woodcutting:logs-chopped", 0)`, 27
  `OnObjectInteractionNamed` registrations, nest search on
  `OnItemFirstAction`. Deferred units stubbed explicitly.
- **onItemAction family seam**
  (`internal/plugins/itemaction.go`,
  `cmd/server/itemactionlistenerwiring.go`,
  `internal/listeners/itemaction.go`): `OnItemAction` /
  `OnItemActionNamed` / `OnItemFirstAction` / `OnCanUseItem`
  + `Hub.EmitItemAction` / `EmitCanUseItem`. Hub now **54 of
  108 families firable (57 registration methods)**; 54
  table-only. Deviations: `item:operated` custom emit NOT
  ported, `subOption` fill-in NOT ported, widget-action emit
  sites remain gaps, no `Api.EmitItemAction` members.
- **ObjectManagerAPI.Get seam** (`internal/plugins/objects.go`,
  `cmd/server/objectlistenerwiring.go`): `MapObjects.get`
  (`MapObjects.ts:94-122`) — the only way content sees
  untouched map objects; `ObjectsAt` sees runtime
  registrations only. A felled tree stops answering `Get`
  (stump swap removes the original from the tile map).
- **Boot wiring** (`cmd/server/server.go`,
  `cmd/server/woodcuttingwiring.go`): cache + definitions load
  BEFORE the hub; `installWoodcuttingTables` calls
  `LoadFromCache` → `SetTables` → `SetMembersWorld` →
  `SetIsMembersItem` (cache `IsMembers` flag); a table-load
  failure fails boot. `newPluginHub(worldDef, logger, defs)`
  signature change; test call sites updated.

### Live-found bugs fixed in-phase

1. **`treeAt` never found real map trees** (smoke run 1):
   it only consulted `ObjectsAt`; map originals never enter
   the world indexes, so every chop failed with "You can't
   reach that tree right now." Fixed with the `Get` seam
   above (TS `MapObjects.get ?? objectsAt.find` order,
   plugin.js:1239). Regression tests:
   `TestTreeAtFindsMapOriginal`, `TestTreeAtNilAfterStumpSwap`.
2. **Register-time hub-surface captures read nil** (smoke
   run 1 re-run): TaskManager/ObjectManager install on the
   hub after plugin Load, so `w.tasks`/`w.objects` were nil
   in production and the loop could never tick. Fixed by
   lazy resolution at use time (`taskManager()` /
   `objectManager()` accessors; directly-set fields are the
   unit-test override). **Hub-contract lesson:** surfaces
   installed after plugin Load must be resolved lazily, never
   captured in Register.
3. **Data race in the first form of fix 2** (caught by
   `go test -race`, `cmd/server`): an `OnServerStartup`
   schedule called `TaskManager.Schedule` off the tick
   goroutine while the engine was already ticking
   (`Schedule`-vs-`Process` race; the TaskManager is NOT
   safe for concurrent use — `task.go:155-158`). The startup
   hook was removed; the 1-tick task now schedules once on
   the first chop, which runs on the tick goroutine through
   the turn-packet drain. Race re-run clean.

### Verification (final code)

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 670 `.go` files,
  18 packages.
- `go test ./... -p 1 -count=1 -v`: **4,182 `=== RUN` lines
  (3,144 top-level `--- PASS`), 0 failures, 18/18
  packages** (baseline 4,115 → +67).
- `go test -race ./... -p 1 -count=1`: **clean, 18/18 ok,
  0 data races** (after the fix above; the pre-fix run
  failed — see bug 3).
- **Live smoke** (harness `/home/hatch/smoke32/`, copy of
  smoke30 + `woodcutting` mode; driver `run_smoke32.sh`,
  `RUNS` override; ports 45991–45993): **3 runs, 9 checks
  each, 0 failures**, all on the final code, each from a
  fresh temp root. Observed live in every run: "You swing
  your axe at the tree.", bronze chop animation 879 (+ 65535
  reset), stump 1342 arriving at the tree tile, respawn
  overlay client script 5474, tree respawning at the same
  tile, second "You get some logs.", save with 2 logs (1511,
  one per slot — logs do not stack), 50 Woodcutting XP
  (25/log, skill 8), `woodcutting:logs-chopped` == 2. Tree:
  normal tree id 1276 at (3086,3526), 3 tiles from spawn
  (tile found by a cache-map decode probe — the server never
  sends `LocAddChange` for untouched map trees). TS rule
  confirmed live: non-multi trees always deplete on the
  first successful log. Honest limits: nest drops unobserved
  live (normal tree has `birdNests: false`; 1/256 rate is
  unit-covered only); walk-away/disconnect stop not
  separately observed live (tree depletes after each log
  anyway; disconnect save landed normally); the plan's
  higher-level-tree (oak) and inventory-full live checks were
  NOT run — the smoke mode covers the normal tree only, and
  those paths are unit-covered only. Harness notes:
  tree click uses `NativeOpLoc1` (96); the harness's
  first-slot `invAmount` check was fixed to sum across slots.
- No probe files remain in the repo (all `zz_temp_*_test.go`
  deleted; grep clean). No data file imported. No stray
  artifacts.

### Deferred — next phase's brief

Woodcutting sub-units, each stubbed explicitly in
`woodcutting.go`: **Guild** (incl. `data/woodcutting-guild.json`
— NOT imported, needs Tyler's approval — +7 invisible boost,
level 60 gate, ObstacleRunner minimal use), **EntTrunk**
(NPCs 7234/6594, trunk 9474), **ThickVines**, **Shrine**,
**ClueNests**, **CrystalAxe** (10,000 start / 20,000 max
charges), **InfernalAxe** (1/3 burn, 5,000 charges, needs
Firemaking `firemaking:log-xp` answerer). Farming slice = 2
constants only (`DEPLETION_ROLL` 11, `REGROW_TICKS` 200) —
do not port Farming. Residual gaps (see PORT_STATUS.md
gap 16): stump model-map leg partial (stump-less trees →
default stump 1342; normal variants → generic treestump2),
no ForceMovement tick-skip, no face-tree step, XP integer
truncation (37.5 → 37), Chebyshev ≤2 interaction distance,
`treeAt` template path skips `World.getRemovedObjects()`,
`equipment:effects` producerless, no SkillLevelUp emitter,
`item:operated` / `subOption` not ported. Fletching is NOT
confirmed as queued after Woodcutting — do not assume it
is next. Doric's Quest remains picked but deferred; no quest
work.
