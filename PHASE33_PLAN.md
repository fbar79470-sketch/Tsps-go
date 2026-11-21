# Phase 33 plan — Woodcutting sub-units (finish the skill)

Dispatched 2026-11-20 under Tyler's "keep going without confirming".
Phase 32 (parent-verified) landed the core chopping loop: 670 .go files,
18 packages, 4,182 tests, race clean, hub 54/108 firable (57 methods).
This phase ports the six deferred sub-units. **Guild is OUT** — it needs
`data/woodcutting-guild.json` imported, which needs Tyler's approval;
the parent has asked him. If he approves, the parent sends a follow-up
to fold Guild in — do not import it on your own, do not block on it.

## Grounding (read before implementing)

- Phase 32: `PHASE32_PLAN.md` (outcome + deferred brief), `PORT_STATUS.md`
  gap 16, `CONTENT_MIGRATION_PLAN.md` skills paragraph.
- Upstream sub-units, read in full by the coordinator (all line numbers
  below are the unit file unless noted):
  - `plugins/skills/woodcutting/EntTrunk.Woodcutting.js` (162 lines)
  - `plugins/skills/woodcutting/ThickVines.Woodcutting.js` (278 lines)
  - `plugins/skills/woodcutting/Shrine.Woodcutting.js` (53 lines)
  - `plugins/skills/woodcutting/ClueNests.Woodcutting.js` (113 lines)
  - `plugins/skills/woodcutting/CrystalAxe.Woodcutting.js` (132 lines)
  - `plugins/skills/woodcutting/InfernalAxe.Woodcutting.js` (87 lines)
  - `plugins/skills/woodcutting/GuildData.Woodcutting.js` (14 lines:
    loads `plugins/skills/data/woodcutting-guild.json`)
- Core loop touchpoints in `internal/content/woodcutting/woodcutting.go`:
  stubs at `tryBurnLog` (~2144), `tryUseCrystalCharge` (~2154),
  call sites ~1990 (burn) and ~2006-2010 (crystal), clue-nest roll
  site ~1969-1970 (NOT called), `isInGuild`/`invisibleBoost` (Guild
  stubs — do NOT touch), helpers `findBestUsableAxe` (~878),
  `calculateCutChance` (~751), `lumberjackXpMultiplier`,
  `maybeDropBirdNest` (~1191), `dropGroundItem` (~713, via
  `game.Hooks.DropGroundItem`).

## Guild-data decision (read this)

EntTrunk and Shrine load their data from `woodcutting-guild.json`
via `GuildData.load` — the `ents` section (~30 values: npcs
7234/6594, trunk 9474, transformTicks 4, trunkTicks 101, chopTicks 3,
xp 25, cutChance 8 rows, 6 log weights, 4 messages) and the `shrine`
section (~20 values: id 29088, anim 3705, xp 100, nest 22798,
projectile, 3 eggs, evilChicken 1/1200 × 4 pieces, 3 messages).
**These values are transcribed into Go tables with JSON line
citations, exactly as Phase 32 transcribed the plugin.js tables into
Go — the JSON file itself is NEVER copied into the repo.** That is
the line the project has drawn (verbatim file imports need Tyler's
approval; ported values in Go source with citations are the normal
porting method). If the parent judges this crosses the line, EntTrunk
+ Shrine revert cleanly: they live in their own files plus one
`guilddata.go` tables section. Flag the decision in the final report.

## Seam audit (coordinator-verified 2026-11-20)

Exists in Go — reuse, do not rebuild:
- `onNpcDeath` firable (`Api.OnNpcDeath`, `internal/plugins/combat.go:655`);
  `NpcDeathEvent` has Killer/NPC/NpcID/Location/Remains
  (`*NpcDeathRemains{Ticks, RespawnTicks, HasRespawnTicks}`).
- `onObjectInteraction` / `OnObjectInteractionNamed` firable (Phase 32,
  27 registrations); `onItemOnObject` / `OnItemOnObjectNamed` firable
  (`internal/plugins/objects.go:636,665`); `onItemAction` /
  `OnItemActionNamed` / `OnItemFirstAction` firable (Phase 32).
- `onPlayerDisconnect` firable; `SetCombatFlag(m, "combat:no-retaliate")`
  exists (`internal/game/combatant.go:623,597`).
- Item meta: `Item.GetMetaValue` / `SetMetaValue(key, json.RawMessage)`
  (`internal/game/item.go:99,124`); meta survives save/load
  (`internal/persist/apply.go:507`). Charge state is
  `{"charges": N}` under keys `"crystal-axe"` / `"infernal-axe"`.
- `ItemDefinition.NoteID()` exists (`internal/definition/item.go:304`).
- `ObjectManagerAPI.Register/Deregister/ReplaceInPlace` exist
  (`internal/plugins/objects.go:271-290`).
- `Player.Bank(tab)`, `TotalBankTabs = 11`; `Inventory().Items()`;
  `container.Contains(id)` (Equipment embeds `*itemContainer`, so
  `Equipment.Contains` works).
- Wilderness `IsInLocation` / `LevelAt` (`internal/game/wilderness.go:162,219`).
- `Mobile.PerformGraphic(id, height, delay)`; `SendSoundEffect`
  (`internal/game/sounds.go:669`); `SoundFireSuccessful` = 2596;
  `Projectile.WithAngle` (`internal/game/projectile.go:330`).
- `SkillManager.AddExperiences(skill, xp int)` — **int XP throughout**
  (TS stores tenths as floats; Go stores `[]int`). The Phase 32
  truncation residual (37.5 → 37) is structural and stays a gap.
- `CacheDefinitions.GetItem(id)` + `GetCounts().Items` — cache scans
  (e.g. rake lookup) are feasible.

Must be built this phase:
- **`onNpcClick` family** (Worker A): `onNpcInteraction` /
  `onNpcClick` are table-only in Go. The listener emit site exists
  (`internal/listeners/npc.go:735`, `deps.NpcHooks.EmitNpcInteraction`)
  but no hub adapter sets it. Ground the routing in TS
  (`PluginManager.emitNpcInteraction`, `NpcInteractionManager`) and
  build the minimal family EntTrunk needs (click by NPC id +
  click type / identifier + option 1). Update the unfired-family
  table + hub counts honestly (families, not methods).
- **NPC remains delay** (Worker A): Go's `dispatchNpcDeath`
  (`internal/game/combat_death.go`) removes the NPC immediately at
  dispatch (documented deviation: "Go has no remains system").
  TS keeps the Ent's remains for `RemainsTicks` (103) and respawns
  after `RespawnTicks` (50/15). Ground in TS
  (`elvarg/game/task/impl/NPCDeathTask.ts`) and build the minimal
  remains delay: when a death hook sets Remains, defer the removal
  by RemainsTicks while the NPC stays registered. The respawn is
  already scheduled from dispatch via `BeginRespawnWithTicks`.
- **NPC transform seam** (Worker A): TS `setNpcTransformationId(9474)`
  + 1 HP locked. Go has NO transform support (Phase 14 gap). Ground
  in TS (`NPCDeathTask.ts`, `Npc.setNpcTransformationId`) and build
  the minimal seam (e.g. a transform id on the NPC consulted by
  sync). `SetHitpoints` on the NPC combatant exists; HP-lock is
  part of the seam (the trunk must not be re-killed mid-window).
- **`onItemOnItem` family** (Worker C): table-only; the listener
  comment says "itemOnItem needs emitItemOnItem (family unfired)".
  There is NO item-on-item decode in Go (`internal/protocol/` has
  ItemOnObject 86 + InventoryUseOn 241 only). Ground the packet in
  TS (`elvarg/net/packet/impl/UseItemPacketListener.ts` +
  ClientProtocol/NetworkBuilder item-on-item path: opcode + layout)
  and build decode + listener route + hub `OnItemOnItem` /
  `OnItemOnItemNamed` + `EmitItemOnItem` + `cmd/server` adapter.
  Update the unfired-family table + hub counts honestly.
- **Player facing seam** (Worker B builds it; coordinator reuses it
  for the face-tree residual): ThickVines calls
  `forcePositionToFace` else `setPositionToFace`. `NPC.SetPositionToFace`
  exists; no player equivalent (Phase 32 gap 16c). Build the minimal
  player seam (check `Mobile.Facing` / entity.go first).

## Scope — the six units

### Worker A — EntTrunk + Shrine

- `internal/content/woodcutting/guilddata.go` (+ test): transcribe
  the JSON `ents` + `shrine` sections into Go tables with JSON
  line citations (see decision above). Fail-closed resolution
  where ids are symbolic (log ids are numeric in the JSON — use
  them verbatim; verify each resolves in the rev-242 cache).
- `enttrunk.go` (+ test): own 1-tick task; `entDied` in exact TS
  order (NPCs 7234 guild / 6594 wilderness only; killer must be a
  player; `remains = {ticks: transformTicks-2 + trunkTicks,
  respawnTicks}`; at +2 ticks: HP 1, HP locked, transform to
  9474, trunk registered `{killer, ent, until: now+101}`); `chopTrunk`
  (killer-only, else return true unhandled; best axe or noAxe;
  full inventory or full message; swing message + axe anim;
  session `{npc, axe, next: now+3, from: player loc}`; no-retaliate
  flag set); `tick` (trunk expiry + deregistration; sessions:
  trunk gone / player unregistered / moved → end; every 3 ticks
  axe anim + cut chance vs `ENTS.cutChance` + `giveLogs`);
  `giveLogs` (rollLog by base level + axe tier weights; noted log
  via NoteID; logsPerSuccess 1 guild / 2 wilderness; full-inventory
  rule incl. `contains(itemId)`; 25 XP × lumberjack multiplier;
  yield message; bird nest via `maybeDropBirdNest`; emit
  `woodcutting:ent-logs`); `forget` on disconnect (flag cleared).
  Registration: `onNpcDeath`, `onNpcClick` (trunk identifier,
  option 1), `onPlayerDisconnect`. Uses helpers `findBestUsableAxe`,
  `calculateCutChance`, `lumberjackXpMultiplier`, `maybeDropBirdNest`.
- `shrine.go` (+ test): `offerEgg` in exact TS order (egg by item
  id in SHRINE.eggs, object id 29088; slot identity check;
  replace slot item with reward — evil chicken piece 1/1200 each
  else nest 22798; refresh; +100 Prayer XP; offerings attribute
  `woodcutting-guild:shrine-offerings` persisted, count messages;
  anim 3705; projectile from/to + angle + heights; per-egg sound).
  Registration: `onItemOnObjectNamed("Bird's egg", "Shrine")` +
  `persistAttribute`. Praying at the shrine is the Altars plugin's —
  do NOT build it.
- Live smoke: decide what is feasible (killing an Ent live needs
  guild access — level 60 gate is a Guild feature, NOT this phase;
  a spawned Ent trunk chop may be testable via a seeded NPC).
  Document honest limits; unit-cover the rest.

### Worker B — ThickVines + ClueNests

- `thickvines.go` (+ test): constants verbatim (CHOP_NAMES,
  CLEAR_NAMES, CLEAR_XP 2, RESPAWN_TICKS 49, CLEAR_DELAY_TICKS 2,
  RAKE_ANIMATION 2273, messages). `groupFor` by cache name + own
  option (Chop vs Clear — Brimhaven "Vines" must NOT clear).
  `playerAxe`: best axe held anywhere, IGNORING wield levels
  (any axe cuts). `resolveRakeId`: scan cache items for unnoted
  "rake". `useTool` in TS order (pending keyed by loc — double
  click pays once; face object via the new player seam; start
  message + anim; +2 ticks: player registered + alive, plant
  still live → snapshot → Deregister → XP (Woodcutting ×
  lumberjack multiplier for chop, Farming for clear) → done
  message → regrow after 49 ticks, re-Register only if the tile
  is empty). Registration: `onObjectInteraction` Chop/Clear per
  name + `onItemOnObject` tool use (axe by id / rake by id,
  already-chopped/cleared messages, `handled = true`).
- `cluenests.go` (+ test): `buildTiers` (5 tiers, modifiers
  0.2/1.7/2/3.3/10, nest + scroll ids resolved via cache names);
  `rollClueNests(player, tree)` in TS order (skip when no
  `tree.petBase`; current WC level; Wilderness ring check —
  Ring of Wealth (i) ids resolved via cache, level > 0;
  per tier: `treasure-trails:vessel-chance` emit (unanswered →
  multiplier 1), `combat-achievements:tier` emit (unanswered),
  base = floor(floor(petBase × 0.95 if CA met else petBase) /
  (100 + level) × modifier), chance = max(1, floor(base /
  multiplier)), ring ×2 non-beginner in Wilderness; roll;
  `ownsClue` — `treasure-trails:can-receive` emit (unanswered →
  inventory + all 11 bank tabs name scan for the 5 clue forms);
  `treasure-trails:missed-clue` emit on owned; `treasure-trails:
  clue-item` emit (unanswered → box false → ground drop via
  `dropGroundItem`; box true path uses forceAdd — NO Go
  equivalent and no answerer can set it, so document as gap,
  do not build it); red nest message). `openClueNest` via
  `treasure-trails:open-vessel` emit (unanswered → default path:
  slot identity, free-slot gate, delete nest, add empty nest
  6 + scroll, message). Registration: `onItemAction("Clue nest
  (tier)", {Open: ...})` per tier. **Wire `rollClueNests` into
  the core success block** at the documented site (~1969-1970,
  after the `woodcutting:success` emit, in TS order) — this is
  Worker B's only edit to `woodcutting.go`; do not touch the
  burn/crystal stubs (Worker C owns those).
- Live smoke: Tar Swamp vines need travel; clue rolls are
  1-in-large. Unit-cover thoroughly; attempt live only if the
  harness reaches the swamp cheaply. Document honest limits.

### Worker C — CrystalAxe + InfernalAxe + onItemOnItem family

- `onItemOnItem` family first (blocks the shard recharge):
  decode + listener route + `Api.OnItemOnItem` /
  `OnItemOnItemNamed` + `Hub.EmitItemOnItem` + `cmd/server`
  adapter, grounded in TS (see seam audit). Update the
  unfired-family table + hub counts honestly. Tests: a
  registered handler fires through the listener; unhandled
  uses fall through silently (TS behaviour — check the
  listener file for the exact fallthrough).
- `crystalaxe.go` (+ test): constants (START 10,000 / MAX 20,000
  / SHARD 100 / key `"crystal-axe"`); `charges(item)` (meta
  `{"charges": N}` clamped [0, 20000]; no meta → START if charged
  id else 0); `findCharged` (wielded first, then inventory);
  `tryUseCharge` (signet 10% save — signet ids resolved via
  cache names; else `useCharge`); `useCharge` (decrement meta;
  at 0 → set id to uncharged form, clear meta, refresh, message);
  `checkCharges` ("Your crystal axe has X charge(s) left.");
  `addShardCharges` (shard on axe either direction; full → message
  + handled; uncharged → charged form; capped at MAX). Maps
  verbatim from the unit file (3 charged→uncharged, 3
  uncharged→charged). Registration: `onItemAction` Check per
  axe name + `onItemOnItem` shard pairs. **Wire the drain into
  the core loop**: replace the `tryUseCrystalCharge` stub and
  its call site (~2006-2010) with the real call (TS order:
  after the log line, before XP — verify against the success
  block in `Woodcutting.plugin.js:1400-1441`).
- `infernalaxe.go` (+ test): constants (BURN_CHANCE 3 /
  MAX 5,000 / key `"infernal-axe"` / graphic 86 height 150);
  `tryBurnLog` in TS order (charged axe id? else false; 1/3 roll;
  find charged axe; `firemaking:log-xp` emit — NO answerer yet,
  so `request.xp` stays null and the burn NEVER fires: honest
  degradation, wire it anyway; `firemaking:xp-multiplier` emit
  defaults 1; half Firemaking XP; smokepuff graphic; fire sound;
  `useCharge`; at 0 → uncharged form + message). `check`
  ("Infernal axe: X.X% remaining."). Registration: `onItemAction`
  Check per name. **Wire the burn into the core loop**: replace
  the `tryBurnLog` stub (~2144) and verify the call site (~1990)
  matches TS order (burn check before `prepareLog`).
- Live smoke: crystal/infernal axes need seeded items; attempt
  live Check + charge drain only if the harness seeds them
  cheaply. Document honest limits.

### Coordinator (integration, after workers)

- Core-loop residuals from gap 16, small ones only: face-tree
  step (reuse Worker B's player-facing seam), stump model-map leg
  (`treeId+1` legs), `treeAt` template path
  (`World.getRemovedObjects()` — check the seam exists first),
  Chebyshev ≤2 interaction distance (ground in TS object
  footprint), ForceMovement tick-skip. XP truncation stays
  (structural int storage — document). SkillLevelUp emitter,
  `equipment:effects` producer, `item:operated`/`subOption`
  stay gaps unless trivial.
- Integrate, resolve conflicts, full verification (below), docs.

## Verification (coordinator)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean;
  `CGO_ENABLED=0 go build ./...` clean.
- `go test ./... -p 1 -count=1` full count (baseline 4,182);
  `go test -race` 18/18.
- Live smoke, 3 runs stable, real rev-242 cache, temp roots
  outside the repo: at minimum re-run the Phase 32 normal-tree
  chop (regression), plus whatever each unit's smoke covers.
  Describe exactly what was observed live per unit; say plainly
  what was unit-covered only.
- Docs: this file (outcome), `PORT_STATUS.md` (header numbers,
  inventory, hub counts if changed, gaps, next-modules order),
  `CONTENT_MIGRATION_PLAN.md` (skills progress), `SKILLS_SURVEY.md`
  ONLY if a survey claim proves wrong (note the correction).

## Out of scope

Guild (needs Tyler's data-import approval), any other skill
(Fletching NOT confirmed as next), all quest work, gap 14
spellbook residuals, the far-WALK bug, the y=3521–3522 map wall,
the scrypt check, perf work, any new verbatim data import. No
scope expansion without TS grounding. If a unit cannot land
whole, document the remainder precisely as the next phase's
brief — do not half-land a unit.

## Outcome

(appended by the coordinator at completion, 2026-11-21)

All six in-scope sub-units landed whole; the Guild stayed out
(pending Tyler's `woodcutting-guild.json` import decision).
4,317 tests passing (baseline 4,182 + 135), 18 packages, 692
Go files. Build/vet/gofmt/CGO clean. Race clean (18/18, 0 data
races). Live smoke 3 runs × 9 checks, 0 failures (Phase 32
tree-chop regression).

### Per unit

- **EntTrunk** — `internal/content/woodcutting/enttrunk.go`
  (437 lines) + `guilddata.go` (233 lines, `ents` + `shrine`
  transcribed from `woodcutting-guild.json` with JSON line
  citations — the file itself was NOT copied into the repo)
  + tests (33 checks incl. subtests). Ent death → remains
  {103 ticks, respawn 50 guild / 15 wilderness} → transform
  at +2 ticks to trunk 9474, 1 HP locked → killer-only
  chopping, 3-tick rolls, 25 XP, 1 noted log guild / 2
  wilderness, bird nests, `woodcutting:ent-logs` emit. New
  seams: `onNpcClick` family (`internal/plugins/
  npcinteraction.go`, `Api.OnNpcClick`, first-claim-wins),
  NPC remains-delay (`internal/game/combat_death.go`: death
  hooks setting Remains defer removal by RemainsTicks while
  the NPC stays registered), NPC transform seam
  (`internal/game/npc.go`: `ContentID()`,
  `SetNpcTransformationID`, HP lock in `DecrementHealth`).
  Cache probe (deleted): NPCs 7234/6594 "Ent", 9474
  `"<col=00ffff>Ent trunk</col>"` with Chop at option 1;
  logs with note ids 1511→1512, 1521→1522, 1519→1520,
  1517→1518, 1515→1516, 1513→1514; eggs 5076–5078; nest
  22798; evil pieces 20433/20436/20439/20442; seq 3705,
  spotanims 1307–1309. Unit-covered only live (no Ent
  kill/chop observed over the wire). Documented divergence:
  respawn scheduled at death dispatch vs TS at remove()
  time (`combat_death.go`).
- **ThickVines** — `internal/content/woodcutting/thickvines.go`
  (554 lines) + tests (553 lines), 27/27 tests passing.
  Tar Swamp chop/clear: any axe, no level gate; rake clear
  gives Farming XP 2; 2 XP either way; 49-tick regrow;
  pending map keyed by loc; vine tool use registered by name
  (one `OnItemOnObjectNamed` per axe display name × chop name
  + Rake × Vines) because `ItemOnObjectEvent` carries no
  object definition. New seam: player-facing
  (`internal/game/player_facing.go`: `SetPositionToFace` /
  `ForcePositionToFace` / `PositionToFace()` /
  `SetDirection`, TS `Mobile.ts:587-654,689-691`). Unit-covered
  only live. Open: the player sync encoder does not yet send
  face updates (state set, not flushed).
- **Shrine** — `internal/content/woodcutting/shrine.go`
  (135 lines). Bird's egg on Shrine 29088 → seed nest 22798
  or an evil-chicken piece at 1/1200 each, +100 Prayer XP,
  `woodcutting-guild:shrine-offerings` attribute, anim 3705,
  per-egg projectile + sound. Unit-covered only live.
- **ClueNests** — `internal/content/woodcutting/cluenests.go`
  (447 lines) + tests (521 lines), 3/3 player-facing tests
  passing. `buildClueTiers` (modifiers 0.2/1.7/2/3.3/10),
  `rollClueNests` wired into the core success block after the
  `woodcutting:success` emit, `ownsClue` via inventory + all
  11 bank tabs, `openClueNest` via `onItemAction`. The rev-242
  cache has no gameval names for clue nests/scrolls nor the
  three `__2` ring variants, so all 10 tier ids + 8 ring ids
  are numeric constants from `ItemIdentifiers.ts`, each
  verified by display name against the real cache. The
  `treasure-trails:clue-item` box-true path needs `forceAdd`
  (no Go form) — documented gap; rolls always ground-drop.
  The Wilderness ring check reads `*game.Wilderness` via a
  lazy type assertion on the Api's AreaManager (fails closed;
  a `SetWilderness` installer was considered and rejected —
  threading the World through `installWoodcuttingTables`
  was not warranted). Unit-covered only live (1-in-hundreds
  rolls, no harness mode).
- **CrystalAxe** — `internal/content/woodcutting/crystalaxe.go`
  (429 lines). START 10,000 / MAX 20,000 / SHARD 100, meta key
  `"crystal-axe"`, signet ids 23943/23944/23945/25545/25543
  (10% save), drain wired into `chopSuccess` after the log
  line, before XP (`Woodcutting.plugin.js:1416-1420`); old
  `tryUseCrystalCharge` stub deleted. Unit-covered only live.
  Cache note: crystal axe id 23862 has no Check action in
  the cache, so name-based Check only fires for 23673/28220
  — matches TS.
- **InfernalAxe** — `internal/content/woodcutting/infernalaxe.go`
  (235 lines). BURN_CHANCE 3 / MAX 5,000, meta key
  `"infernal-axe"`, burn wired before `prepareLog`
  (`Woodcutting.plugin.js:1410-1413`); old `tryBurnLog` stub
  deleted. **Honest degradation: `firemaking:log-xp` has no Go
  answerer yet, so the infernal burn NEVER fires** — wired
  anyway, pinned by test; it unblocks itself when Firemaking
  is ported. Unit-covered only live.
- **onItemOnItem family** — `internal/plugins/itemonitem.go`
  (168 lines). Grounded finding: there is NO separate
  item-on-item opcode — it rides INVENTORY_USE_ON (241);
  `ClientProtocol.ts:912-925` decodes kind 4,
  `NetworkBuilder.ts:839-844` dispatches to
  `UseItemPacketListener.itemOnItem`
  (`UseItemPacketListener.ts:15-37`); Go decode already
  handled kind 4, so no protocol change was needed — only
  the route (`internal/listeners/useitem.go`: slot-bounds
  checks, no id verification, no walk-to, no click-run
  override, unhandled uses fall through silently), hub family
  (`Api.OnItemOnItem` / `OnItemOnItemNamed`, either-order
  name match, noted filter on both ids per
  `PluginManager.ts:4132`, named claim on `handler() !==
  false` per `:4139`; `Hub.EmitItemOnItem` per
  `PluginManager.ts:1925-1930`), and adapter on existing
  `ObjectHooks` in `cmd/server/objectlistenerwiring.go`. No
  `Api.EmitItemOnItem` was invented — no such member exists
  in `PluginTypes.ts`.

### Hub counts after this phase

56 of 108 hook families firable (60 `Api.On*` methods — four
named overloads: `OnObjectInteractionNamed`,
`OnItemOnObjectNamed`, `OnItemActionNamed`,
`OnItemOnItemNamed`), 52 table-only; 29 of 95 non-hook
members; 12 of 191 Core members. (Correction during
integration: `hooks.go`'s header listed five overload
families including `onNpcInteraction` — there is no
`OnNpcInteractionNamed`; the four real overloads give
60 − 4 = 56 families. Fixed in `hooks.go`.)

### Core-loop residuals (gap 16)

Closed: stump model-map + treeId+1 legs (memo computed at
load from the cache definitions; cross-checked against the
real cache — stump 1342 for tree 1276, all 273 tree object
ids memoised; finding: normal tree 1276 is 2x2 in the cache,
so the footprint fix matters), footprint interaction
distance (`GameObject.ts:151-155`, sizes from
`Tables.ObjectSize`, 1x1 fallback = old Chebyshev behavior),
face-tree step (`startWoodcutting` calls
`player.ForcePositionToFace` after combat reset,
`Woodcutting.plugin.js:1279-1285`), `treeAt` removed-objects
leg (`RemovedObjects()` on `ObjectManagerAPI`,
`World.ts:1099-1101`; fake-level test only — a
template-backed `PrivateArea` cannot be constructed from
the content package's tests). Remaining: ForceMovement
tick-skip (no ForceMovement system exists in Go), XP integer
truncation (structural), no SkillLevelUp emitter,
`equipment:effects` producerless, `item:operated`/`subOption`
not ported, nest drops unit-covered only, player face-sync
encoder missing, `resolveWilderness` type assert,
`treasure-trails:clue-item` box-true path needs `forceAdd`,
Ent respawn scheduled at death dispatch vs TS at remove()
time.

### Verification (measured)

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 692 `.go` files,
  18 packages.
- `go test ./... -p 1 -count=1`: all 18 packages pass,
  4,317 passing test cases including subtests, 0 failures,
  0 skips (baseline 4,182 + 135).
- `go test ./... -p 1 -count=1 -race`: passes, 18/18,
  0 data races. (`TestEngineTicksAtRate` passed serialized;
  no tolerance change.)
- Live smoke (`/home/hatch/smoke32/`, `woodcutting` mode,
  real rev-242 cache, temp roots outside the repo): 3 runs,
  9 checks each, 0 failures. Observed on the wire each run:
  "You swing your axe at the tree.", bronze chop anim 879
  (+ 65535 reset), "You get some logs." twice, tree
  depleting to stump 1342, respawn overlay client script
  5474, tree respawning at the same tile, save holding 2
  logs (1511), 50 Woodcutting XP, `woodcutting:logs-chopped`
  == 2. The six sub-units are unit-covered only live — no
  Ent kill/chop, vine clearing, clue-nest roll, or
  crystal/infernal Check or charge drain was observed over
  the wire (the harness has no modes for them).
- No `zz_*`/probe files, no `.js` files in the repo. No data
  file imported; no Drive upload (parent's step).

### Open questions for the parent

1. **Guild-data transcription:** EntTrunk/Shrine values were
   transcribed from `woodcutting-guild.json` into Go tables
   with JSON line citations; the JSON file itself was NOT
   copied into the repo. Does transcription cross the
   verbatim-import line? EntTrunk+Shrine revert cleanly if
   it does. (Flagged here and in this file's plan section.)
2. **Guild import:** still awaiting Tyler's answer on the
   `woodcutting-guild.json` import (asked 2026-11-20). If he
   approves, a follow-up folds Guild in; nothing in this
   phase blocks on it.
3. **Next skill:** Fletching is NOT confirmed as queued —
   the next skill is Tyler's pick.
