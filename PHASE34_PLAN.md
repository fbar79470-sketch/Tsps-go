# Phase 34 Plan — Fletching (second skill)

## §0. Baseline and decision

- Tyler PICKED Fletching as the next skill (widget tap 2026-11-20 18:59 PST,
  answering the parent's explicit "which skill next?" — no conflicting signal
  this time; Fletching is confirmed).
- Phase 33 state (parent-verified): 692 `.go` files, 18 packages, 4,317 test
  cases passing incl. subtests, race clean, hub 56 of 108 families firable
  (60 `Api.On*` methods — four named overloads), 29 of 95 non-hook members,
  12 of 191 Core members. Content: 6 of 577 entries ported (Woodcutting done
  except the Guild).
- **Data file — SETTLED, approved.** Tyler approved the `fletching.json`
  import (widget tap 2026-11-20 19:00 PST). The parent copied it verbatim to
  `data/definitions/fletching.json` — SHA-256 identical both sides,
  `c9f64bea417ecd909c04dd3d9633e052f4c295ad91cd9fdc6cbf0fbe6baeaea9`;
  valid JSON, 170 recipes. Full suite still 4,317 after the import.
  **No Go loader exists yet — Phase 34 builds it.** Use the file verbatim;
  do not re-import it, do not copy it anywhere else.
- The Woodcutting Guild import (`woodcutting-guild.json`) is still unanswered
  — do NOT import it, do NOT fold the Guild into this phase.
- Transcription precedent (parent judgment, Phase 33): transcribing upstream
  table values into Go source with line citations is normal porting
  methodology (as Phase 32 did for plugin.js tables); copying a data FILE as
  an asset into the repo is a verbatim import needing Tyler's approval. The
  two are different. This phase uses the imported file verbatim (approved),
  so no transcription question arises for the recipe data; document which
  path every other table took.

## §1. What the skill is (grounded, re-verified 2026-11-20)

- Entry `skills/Fletching.plugin.js` (36 lines), true size 965, members-only.
  Units: `fletching/Shared.Fletching.js` (644), `Spool.Fletching.js` (186),
  `Recipes.Fletching.js` (34), `Diaries.Fletching.js` (35).
- `data/fletching.json` (235 lines, 37,615 B): **170 recipes** (counted with
  Python, matches the survey), 13 groups (`logs`, `shields`, `stringing`,
  `crossbows`, `arrows`, `darts`, `bolts`, `tipped`, `javelins`, `chisel`,
  `boltTips`, `ogre`, `parts`), 8 menus (`make`, `string`, `feather`,
  `complete`, `javelins`, `boltTips`, `tip`, `sets`), 4 diary tasks,
  265 distinct item names (counted). Tools: `knife` (KNIFE,
  FLETCHING_KNIFE), `plainKnife` (KNIFE), `chisel` (CHISEL,
  JEWELLERS_CHISEL), `hammer` (HAMMER). Alternatives: `@feathers` (6 feather
  kinds + GRYPHON_FEATHER `usedOnly` + STYMPHIKE_FEATHER).
- Core loop (`Shared.Fletching.js`): item-on-item pair → the recipes that
  pair starts → single-recipe fast paths (instant darts/bolts, 1-batch) or
  a make-many `CreationMenu` (skillmulti) → a hand-rolled `FletchingTask
  extends core.Task`, 1-tick, first batch on the recipe's `firstTick`,
  per-recipe tick pacing (`actionInterval` + fletching-knife −1 tick after
  the first action; jeweller's-chisel pace for bolt tips), partial-set
  handling, random yields (`amount`..`amountMax`), per-item XP (`xpEach`).
- Spool (`Spool.Fletching.js`): bow strings stored on the account, not the
  item; capacity tiers 250/500/2000/5000/10000/100000 by spare spools;
  persist attributes `fletching:spool-charges` /
  `fletching:spool-spares`; varbits `bowstring_spool_charges` /
  `bowstring_spool_size`; a carried spool is a virtual bow-string source
  and a trigger alias for bow string; Fill/Check/Empty item actions;
  `crafting:spun` listener (no producer until Crafting — wire, harmless).
- Diaries (`Diaries.Fletching.js`): `fletching:made` listener, 4 tasks
  (oak shortbow in Gnome Stronghold, magic longbow in Tirannwn, dragon
  darts at Bedabin Camp, maple shortbow in Seers' bank), emits `diary:task`
  (no consumer — harmless).
- Plugin (`Fletching.plugin.js`): `init` all units then `attach`; answers
  `fletching:shafts` (the log → arrow-shaft recipe for content that
  fletches as it chops; no answer when no recipe matches).
- Survey corrections to verify at port time (§7.5 was written before
  Phases 32–33): it calls `onItemOnItem` ×3 and `onItemAction` ×2
  "TABLE-ONLY" — both families are firable in Go now (Phases 32/33). If
  the port confirms, correct those two labels in `SKILLS_SURVEY.md`.

## §2. Recipe data: loader (Worker A)

- Follow the definition-Store precedent (`internal/definition/sounds.go`:
  `loadExaminesAndSounds` / `loadNpcCombatSounds` — eager at Load, present
  but malformed fails Load) for the JSON read, and the woodcutting
  `LoadFromCache` precedent for name resolution: parse the JSON into raw
  structs, resolve every gameval name through the cache
  (`GetObjId`/`GetSeqId`/`GetVarbitId` + sound-name lookup), fail closed
  listing every unresolvable name — no id is invented.
- Names to resolve: 265 item names, the `animation` seq names
  (`human_fletching`, `stringing_*`, `xbows_*`, `human_*cutting`, …),
  varbit `option_fletch_dart_or_bolt_makex` + `bowstring_spool_charges` /
  `bowstring_spool_size`, sounds `CROSSBOW_LIMBS` / `CUTTING` /
  `GEM_CUTTING` (Go `internal/game/sounds.go` already has a name table —
  verify all three resolve).
- **Load-prove** against the real rev-242 cache the way earlier imports were
  proven: a temporary test that loads the file + resolves every name and
  asserts the counts (170 recipes, 13 groups, 8 menus, 4 diary tasks, 265
  names, 0 missing), then **delete the test**. Record the proof in the
  outcome section and extend the `PORT_STATUS.md` data-import paragraph
  (the parent already added the import entry — extend it with the loader).
- XP is float in the JSON (`16.5`, `33.3`, `187.5`…); Go
  `SkillManager.AddExperiences(skill, xp int)` takes int. **XP integer
  truncation is a known structural gap** (same as woodcutting gap 16: 37.5
  → 37) — truncate in the same place woodcutting does, document it, do not
  invent float XP storage.

## §3. CreationMenu engine seam (Worker B)

TS grounding: `CreationMenu.ts` (97 lines: title + items + action +
options `{labels, maxAmount, lastAmount, mode}`), `Player.ts:1602`
`setCreationMenu`, `PacketSender.ts:1370–1418` `sendCreationMenu` /
`closeCreationMenu`, `SkillManager.ts:648` (clearing in `StopSkillable`),
`NetworkBuilder.ts:908–951` (the `dialogue_continue` pre-branch).

- Constants (`PacketSender.ts:159–168`): group **270**, first item component
  **15**, max quantity **28**, last-item varp **2673**, attribute
  `creation-menu:last-item` (already in the Go persist registry —
  `internal/persist/attributes.go:40`), op flags
  `(1<<1)|(1<<2)|(1<<3)|(1<<4)|(1<<5)` = 62.
- Send path (`PacketSender.ts:1397–1414`): `sendInterfaceScript(2379)` →
  chatbox unclamped → display state (modal target, shown) → sub-interface
  (group 270) → `sendInterfaceFlagsRange` per item button → varp 2673 when
  a last item is remembered → `sendInterfaceScript(2046, [mode ?? 0,
  "title|name|name…", maxAmount, ...18 padded ids, lastAmount])`.
  Go already has every sender this needs: `SendInterfaceScript`,
  `SendChatboxInterface(groupID, unclamp)`, `SendInterfaceDisplayState`,
  `SendSubInterface`, `SendInterfaceFlagsRange`, `SendVarp`, and
  `SendClientScript(scriptID, ...ClientScriptArg)` — TS `sendInterfaceScript`
  writes `encodeWidgetRunScript`, which is Go `EncodeRunClientScript`, the
  same frame; `ClientScriptArg` carries strings and ints. Item names come
  from the cache (`HasItem`/`GetItem().name`, `"null"` fallback). Items
  slice to 18; empty item list sends nothing (TS early return).
- Receive path (`NetworkBuilder.ts:918–937`): on `dialogue_continue`, when
  a creation menu is open and the widget group is 270 with child ≥ 15, the
  item is `menu.items[child-15]`, the amount is `childIndex` clamped to the
  menu's `maxAmount` (or 28), default 1; the player attribute is set to
  `child-15`; the menu closes; `menu.execute(itemId, amount)` runs. This
  branch runs BEFORE the interface-action-click emit and `handleResume`.
- Go work: `CreationMenu` type in `internal/game` (title/items/action/
  options + `execute` with the includes-check); `Player` get/set creation
  menu; `Session.SendCreationMenu` / `CloseCreationMenu` (close = close
  sub-interface + display state shown — verify the Go close calls against
  TS `closeCreationMenu`); the pre-branch in
  `internal/listeners/npc.go:routeDialogueResume` (currently NOT STARTED —
  `npc.go:906`); clearing from `StopSkillable` (Go `internal/game/skills.go`
  has GAP comments at :1272/:1283 naming exactly this).
- Verify the 2046 script args + interface frames byte-for-byte against TS
  in unit tests where the frame encoder allows it; live-prove the menu
  opens in the smoke test (script 2046 on the wire).

## §4. Shared loop port (Worker C)

Port `Shared.Fletching.js` in TS order, with `file:line` citations:

- `resolveRecipe`: group defaults filled in, then entry overrides; menu
  lookup (`DEFAULT_MENU = "make"`); `ticks ?? 2`, `firstTick ?? 1`,
  `startTick` (undefined → 0, null stays null = no opening), `startSound`,
  `jewellersTicks`, `knifeBonus`, `tool`/`held`, materials via
  `resolveMaterial` (alternatives incl. `usedOnly`), `animation`
  (`false` → none, else name or `human_fletching`), `sound`, `message`,
  `missingTool`, `instantVarbit`, `maxSets ?? 28`
  (`CREATION_ALL_SENTINEL`), `quest`/`unlock`, menu.
- Pair index: `triggerPairs` (tool × materials[0], else materials[0] ×
  materials[1]), `recipesFor` (filters `usedOnlyIds` containing the
  *target* id `b`), `addTriggerAlias` (spool ↔ bow string), virtual
  materials registry.
- Level: `fletchingLevel` emits `skills:level-boost` (default boost 0 —
  unanswered is fine); `actionsPerCycle` emits `skills:actions-per-cycle`
  (default 1). Quest/Slayer gates: `questState` / `hasSlayerUnlock` require
  `request.complete === true` / `request.unlocked === true` — **fail
  closed when unanswered** (quests/Slayer unported), so gated recipes are
  silently unavailable; ungated recipes unaffected.
- Materials: `restricted` (preset / untradeable / unbankable — Go has
  `Item.IsPresetItem` + `ItemUntradeableMeta`/`ItemUnbankableMeta`);
  `plainAmount` (unrestricted copies only, slot order for taking);
  `heldAmount` (inventory + virtual source); `fullBatches` /
  `partialSize` (one-for-one recipes only: `amount > 1`, `amountMax ==
  amount`, every material amount == recipe amount) / `scaledRecipe`;
  `hasTools` (tool + held, inventory or worn — Go `carries` = inventory
  or equipment contains); `takeOrder` (clicked id first, then data order);
  `materialPlan` (virtual-first when `via` is an alias, else
  inventory-first, then virtual — exact order matters);
  `canGrantAfterConsumption` (preview inventory must fit `amountMax` —
  verify the Go container copy/preview API at port time); `takePlain`
  (slot order, skip restricted); `consumePlan`.
- `makeBatch`: level gate message first; tools + availability; partial
  scaling when no full batch; plan must be complete; random amount roll
  (`amount`..`amountMax`); space check at `amountMax` or refuse with "You
  don't have enough inventory space."; consume; add output; animate;
  sound; spam message (`{amount}` filled); XP (int-truncated, §2);
  emits `fletching:made` + `skills:produced` with the action payload
  (inputItemIds/inputs/outputItemId/stationObjectId null/location/
  startLocation).
- `interval` / `toolInterval` (jeweller's chisel → `jewellersTicks`) /
  `knifeSaving` (carried or worn fletching knife, −1 tick, only when
  `knifeBonus`); `playOpening` (anim + sound when `startSound`).
- `FletchingTask`: 1-tick task on the lazily-resolved TaskManager —
  follow woodcutting's `ensureTask` pattern exactly (lazy resolution,
  idempotent, `Schedule` only on the tick goroutine; the Phase 32
  hub-contract lesson). `ticks` counter, `nextTick = firstTick`,
  `startTick` opening, force-movement stop (with animation reset),
  `levelUpPaused` (via `pauseForLevelUp`, `skillactions.go:81`),
  first-two-arrow-shaft-logs knife-saving delay, `remaining` batches,
  stop when a batch fails (reset animation only when `done === 0`) or
  when no batches remain.
- `startFletching`: level gate, availability, `min(requested, available,
  maxSets)`; `ticks === 0` → immediate single `makeBatch`, no task;
  otherwise submit the task; opening plays on the click's own tick when
  `startTick === 0`.
- `openMenu`: stop any running task; `maxAmount = max(min(batches,
  maxSets), 1)` over the choices, capped at 28; title filled with
  `choices[0].amount`; mode passed through when set.
- `isInstant`: `instantVarbit` set and `Session.GetVarbit` == 0
  (darts/bolts default one set per click).
- `handlePair` in exact TS order: no recipes for the pair → "Nothing
  interesting happens." only if the pair is known, else return false;
  unlocked filter → none → "Nothing interesting happens."; missing-tool
  message only when ALL choices lack tools; usable filter (tools +
  batches > 0); empty → preset-only check → `PRESET_TEXT` ("Preset items
  cannot be used for this.") or silent (empty spool); single usable
  recipe → instant path (as many as `actionsPerCycle` in one click) or
  1-batch fast path; otherwise the menu. Returns true when Fletching took
  the click.

## §5. Recipes unit (Worker C)

- `Recipes.Fletching.js`: generic `onItemOnItem(itemOnItem, {noted:
  false})` → `Shared.handlePair(usedId, usedWithId)`, `handled = true`
  (Go `OnItemOnItem` + noted filter exist since Phase 33 — verify the
  event field names at port time); `onItemAction("Celastrus bark",
  {Fletch})` → `handlePair(knife, bark)` (Go `OnItemActionNamed` exists
  since Phase 32); `onPlayerDisconnect` → `stopFletching` (firable —
  `Hub.EmitPlayerDisconnect`, `hooks.go:1264`); `onCustomEvent
  ("level-up:popup-opened", pauseFletching)` — the emit is dead in Go
  (never emitted), wire it anyway like woodcutting did.

## §6. Spool unit (Worker C)

- `Spool.Fletching.js`: persist attributes `fletching:spool-charges` /
  `fletching:spool-spares` (owned save state, `Api.PersistAttribute`,
  like `woodcutting:logs-chopped`); capacity tiers
  `[250, 500, 2000, 5000, 10000, 100000]` by spares (clamped index);
  varbits `bowstring_spool_charges` / `bowstring_spool_size` sent on
  login and on every change (`sendVarbits`); virtual-material `SOURCE`
  (`count` only while a spool is carried, `take` capped at charges);
  trigger alias spool → bow string.
- Fill/Check/Empty via `onItemAction("Bow string spool", …)`; bow string
  or its note used on the spool → fill (`handled = true`); spool on spool
  → combine (needs 2 spools in inventory; spares cap 5; message on cap);
  `crafting:spun` listener (no producer — harmless).
- Noted bow string id: TS reads it from the item definition
  (`getNoteId`). **Verify at port time** how Go resolves a note id
  (`container.unNote` goes the other way; check the definition store for
  a note-id accessor — do not invent one).
- Empty → `OptionDialogue` ("Take them as bow strings." / "Take them as
  notes." / "Cancel.", title "Empty the spool?") via
  `DialogueChainBuilder` (both exist in Go — verify the execute-option
  flow + `sendInterfaceRemoval` on choose); withdraw honours free slots
  (plain) or addable amount (noted) and refuses with "You don't have
  enough inventory space." when 0.

## §7. Diaries unit (Worker A)

- `Diaries.Fletching.js`: 4 tasks from the JSON `diaryTasks` (items
  resolved to ids, rects as-is); `fletching:made` listener; `diary:task`
  emit (no consumer — harmless). Note the survey's diary-key mapping
  (`western`/`desert`/`kandarin`) is as the JSON has it.

## §8. Plugin wiring + members gating (Worker C)

- `Fletching.plugin.js` register: `init` every unit, then `attach` every
  unit, then `api.onCustomEvent("fletching:shafts", answerShafts)`.
  `answerShafts`: find the recipe whose output is `ARROW_SHAFT` and whose
  materials include `request.logId`; set `itemId`/`amount`/`xp` on the
  request; answer nothing when no recipe matches. Verify Go
  `EmitCustomEvent` passes the same request pointer so the mutation is
  visible to the emitter (woodcutting's query precedent).
- **Members gating**: `members: true`. TS skips members plugins on
  free worlds (`PluginManager.ts:751,767`). Mirror it in Go the way the
  hub's members-gating already works (`cmd/server/pluginwiring.go:28,52`
  — verify at port time that a members content package does not register
  on a F2P world; woodcutting's `SetMembersWorld`/`SetIsMembersItem`
  pattern is the precedent for item-level gating).
- Boot wiring in `cmd/server` (follow `woodcuttingwiring.go`): definitions
  load BEFORE the hub; the fletching loader runs before hub Load; a load
  failure fails boot. Blank import in `pluginwiring.go`.

## §9. Verification (coordinator)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean;
  `CGO_ENABLED=0 go build ./...` clean.
- `go test ./... -p 1 -count=1 -v`: exact passing-case count incl.
  subtests, 0 failures, 0 skips, 18/18 packages (baseline 4,317).
- `go test -race ./... -p 1 -count=1`: clean, 18/18, 0 data races.
- **Live smoke** (new `smoke34/` harness reusing the `smoke32` pattern,
  real rev-242 cache, temp roots outside the repo, `RUNS` override for
  single runs): 3 runs stable. Observe on the wire every run: knife on
  logs → CreationMenu opens (script 2046 + group 270) → choose Make-All →
  arrow shafts land in inventory + Fletching XP in the save; bow string
  on unstrung bow → single-batch stringing with no menu; spool Fill then
  Check shows the stored count. State plainly anything unit-covered only
  (e.g. diary-task areas, quest-gated recipes) — do not claim it live.
- No `zz_*`/probe files, no `.js` files in the repo. No data file
  imported (the import is already done — do not re-import).

## §10. Out of scope / known gaps

- Any other skill; all quest work; the Woodcutting Guild; gap 14
  spellbook residuals; the npc-combat-sounds game-level seam; far-WALK
  bug; y=3521–3522 map wall; real production scrypt-hash check; perf work;
  any new verbatim data import.
- Known degradations to document, not fix here: XP integer truncation
  (§2); quest/Slayer-gated recipes locked until those systems exist
  (fail-closed, TS-faithful when unanswered); `crafting:spun` has no
  producer; `level-up:popup-opened` never emitted; `fletching:shafts`
  answered but no consumer yet (Woodcutting league relic unported);
  `diary:task` has no consumer; no SkillLevelUp emitter.

## §11. Work split (3 workers)

- **Worker A — tables + loader + Diaries:** §2 (JSON structs, cache name
  resolution, fail-closed load, temporary load-prove test then delete) +
  §7. Owns `internal/content/fletching/tables.go` (+ tests) and
  `diaries.go`.
- **Worker B — CreationMenu seam:** §3. Owns the `CreationMenu` type,
  Player field, session senders, the `routeDialogueResume` pre-branch,
  `StopSkillable` clearing, frame tests. Must not break the existing
  dialogue-resume behaviour (interface-action-click emit still runs when
  no menu is open).
- **Worker C — Shared loop + Recipes + Spool + wiring:** §4–§6 + §8.
  Owns `shared.go`, `recipes.go`, `spool.go`, `fletching.go` (plugin
  register), `cmd/server/fletchingwiring.go` + blank import.
- The coordinator integrates, resolves conflicts, runs the full
  verification + live smoke, and updates the docs (§12).

## §12. Docs

- `PHASE34_PLAN.md` (this file): outcome section appended.
- `PORT_STATUS.md`: header (phase-34 summary), "What was verified",
  hub row if changed, content row (Fletching), new Phase 34 paragraph,
  gap 17 (phase-34 residuals), Next modules, data-import paragraph
  extension (§2).
- `CONTENT_MIGRATION_PLAN.md`: skills progress paragraph.
- `SKILLS_SURVEY.md`: only if a survey claim proves wrong — the §7.5
  `onItemOnItem`/`onItemAction` "TABLE-ONLY" labels are already stale
  (see §1); correct them if the port confirms, and note the correction.

## Outcome (coordinator, 2026-11-21)

Phase 34 is complete and verified. All §2–§8 scope items landed;
nothing was stubbed or half-landed.

- **§2 loader (Worker A):** `internal/content/fletching/tables.go`
  (797 lines) + `item_ids.go` (212 lines, 176-entry fallback table)
  + tests. Load-proven against the real rev-242 cache by a temporary
  test, then the test was deleted: `recipes=170 groups=13 menus=8
  diaryTasks=4 distinctItemNames=265 expanded=272 resolved=278
  fallback=176 gameval=102 seqs=68 sounds=3 varbits=3`. The 3
  gameval mis-resolutions (opal/pearl/barbed bolts) are pinned in
  the fallback table; BOW_STRING_SPOOL (31052) has no working cache
  gameval. A also fixed the `pluginwiring_test.go` members-world
  expectation to include Fletching.
- **§3 CreationMenu seam (Worker B):** `internal/game/creationmenu.go`
  (234 lines), `internal/session/creationmenu.go` (179 lines), Player
  field, `StopSkillable` clearing, `routeDialogueResume` pre-branch,
  `cmd/server/interfacewiring.go` dep. Two plan corrections, both
  verified against the TS encoders by the coordinator:
  (1) script 2046 travels on opcode 110 (`WIDGET_RUN_SCRIPT`,
  4-byte id — `encodeWidgetRunScript`, `ClientProtocol.ts`), NOT
  opcode 170 (`RUN_CLIENT_SCRIPT`, 2-byte id); the plan's claim
  that `SendClientScript` is the same frame was wrong — B added
  `SendInterfaceScriptWithArgs` and pinned opcode 110 in tests.
  (2) The menu mount must NOT use `SendChatboxInterface` — TS
  `sendCreationMenu` never calls it (`PacketSender.ts:1397-1400`
  uses `setChatboxUnclamped` + display-state + sub-interface
  directly); routing through it would wrongly set
  `chatboxGroupId = 270` and emit `interface:closed` — B used the
  primitives, and a test pins `IsChatboxInterface(270) == false`
  after send. Frame-byte test compares all 8 queued frames against
  hand-computed bytes (2379, varbit 10670=1, modal unhide,
  open-sub 270, two flags ranges, varp 2673=1, full 165-byte 2046
  frame). 24 new passing cases (6 game + 7 session + 10 listener
  subtests); race clean on the three touched packages.
- **§4–§6 + §8 (Worker C):** `internal/content/fletching/shared.go`
  (1,770 lines), `recipes.go` (58), `spool.go` (429),
  `fletching.go` (102), `cmd/server/fletchingwiring.go` (41), tests
  (shared 1,164, recipes 170, spool 407 lines). All §4–§6 + §8
  items landed; `members:true` verified by test (skipped on F2P
  world, loaded on members world); bow-string id resolved via
  `ItemNoteIDSource` (EntTrunk precedent, no invented accessor).
  The duplicate event-name constants C and A produced
  (`FletchingMadeEventName` vs `MadeEventName`, same value) were
  unified by the coordinator — the emit site uses `MadeEventName`
  only.
- **Verification (coordinator):** `go build`, `go vet`, `gofmt -l`,
  `CGO_ENABLED=0 go build` all clean; 709 `.go` files, 19 packages
  (new `internal/content/fletching`); no `zz_*` probe files, no `.js`
  files. Full suite `go test ./... -p 1 -count=1 -v`: **4,413
  `=== RUN` lines (3,315 top-level `--- PASS`, 1,098 subtest PASS),
  0 failures, 0 skips, 19/19 packages** — baseline 4,317 → +96.
  Race: **exit 0, 19/19 ok, 0 data races**. One flake dismissed
  with evidence: `TestEngineTicksAtRate` failed once under load,
  passes 3/3 isolated, tolerance untouched. Live smoke
  (`/home/hatch/smoke34/`, `fletching` mode, real rev-242 cache,
  temp roots outside the repo): **3 runs × 11 checks, 0 failures**,
  runs executed singly via a `RUNS` override — knife on logs →
  CreationMenu script 2046 → 10 shaft messages + anim 1248,
  save 150 shafts + 50 XP; bow string on unstrung bow → shortbow
  + 5 XP with no second menu (2046 count stayed 1), anim 6678;
  spool fill → 5 charges in save.
- **Docs (§12):** `PORT_STATUS.md` (header, "What was verified",
  content row 7 of 577, Phase 34 paragraph, gap 17, Next modules,
  data-import paragraph extended for the loader); hub row unchanged
  (56/108 families, 60 methods — Fletching needed no new hook);
  `CONTENT_MIGRATION_PLAN.md` (skills progress paragraph; the stale
  "Fletching is NOT confirmed as queued next" line corrected);
  `SKILLS_SURVEY.md` §7.5 `onItemOnItem`/`onItemAction` TABLE-ONLY
  labels corrected to FIRABLE (Fletching + Herblore + Ectofuntus
  sites).
- **Remaining gaps (gap 17 in PORT_STATUS.md):** XP integer
  truncation (structural); quest/Slayer-gated recipes locked
  fail-closed; `crafting:spun`, `level-up:popup-opened`,
  `fletching:shafts`, `diary:task` have no producer/consumer yet;
  no SkillLevelUp emitter; force-movement task stop unported (no
  Go seam); spool messages are TS's own (no rsprox capture); only
  the three smoke legs observed live (167 recipes, diary tasks,
  chisel/hammer paths unit-covered only).
- **Out of scope, untouched:** quests, Woodcutting Guild
  (import still pending with Tyler), gap 14, npc-combat-sounds
  game-level seam, far-WALK bug, y=3521 wall, scrypt check, perf
  work, any new verbatim data import. No data file was imported in
  this phase — `fletching.json` was imported by the parent
  2026-11-20 with Tyler's approval; this phase built its loader.
- **Next:** the next skill is Tyler's pick.
