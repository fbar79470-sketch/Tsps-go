# PHASE35_PLAN.md — Fishing (third skill, F2P)

## Scope

Port the Fishing skill's **core fishing loop** plus the sub-units that plug
directly into it. Tyler picked Fishing (text 2026-11-20 20:10 PST; it was not
on the options widget; no conflicting signal). F2P, entry 1,134 lines, true
size 2,749 (survey §3 — Fishing was sweep-screened only, NOT read in full;
this plan is grounded in a full read of `Fishing.plugin.js` + all 10
`fishing/` files, done by the coordinator 2026-11-20).

**In scope (§2–§6):** tables + loader (§2); the `onNpcInteraction` hub seam
(§3, the one new family); the core loop — sessions, 1-tick task, rolls,
landing (§4); sub-units that plug into the loop — AnglerOutfit XP multiplier
(+ set text), CrystalHarpoon, InfernalHarpoon, KarambwanVessel, RadasBlessing,
SpiritFlakes, ClueBottles, Camdozaal tools, Guild invisible boost (§5).
**Deferred (§7, precise brief):** MinnowPlatform (rotation + Kylie + boats +
exchange), AnglerOutfit Gita exchange, Guild door. **Out of scope:** any other
skill, all quest work (Doric's Quest picked but deferred), the Woodcutting
Guild (import approval still pending — separate question, do not fold it in),
gap 14 spellbook residuals, the npc-combat-sounds game-level seam, the
far-WALK bug, the y=3521–3522 map wall, the real production scrypt-hash
check, perf work, any new verbatim data import. No data file: the survey
(§5) says Fishing has none, and the full read confirms it — every table is
inline in the plugin or its units. If a worker finds a data file the plan
missed, do NOT import it without Tyler's approval; transcribe with citations
or document as a gap.

## §1. Grounding (read first)

- Upstream: `~/tsps-bfd81e3/server/plugins/skills/Fishing.plugin.js`
  (1,134 lines) + `fishing/` (AnglerOutfit 10,602 B; Camdozaal 1,096 B;
  ClueBottles 3,007 B; CrystalHarpoon 4,579 B; Guild 3,122 B;
  InfernalHarpoon 6,093 B; Karambwan 1,927 B; MinnowPlatform 12,722 B;
  RadasBlessing 1,571 B; SpiritFlakes 847 B).
- Go precedents to follow exactly: Woodcutting core loop
  (`internal/content/woodcutting/woodcutting.go`, incl. its header comment
  on unanswered events, the 1-tick task pattern at lines ~616–660, and the
  `storage:` query types); Fletching (`internal/content/fletching/`, incl.
  `shared.go` loop + `tables.go` `Load`/`LoadFromCache` with gameval
  resolution + the 176-entry fallback table precedent); Phase 33
  `cluenests.go` for the `treasure-trails:can-receive` /
  `treasure-trails:clue-item` handling (box-true path NOT built — no Go
  `forceAdd`).
- Go hub today (verified 2026-11-20): 60 `Api.On*` methods, 4 named
  overloads, **56/108 families firable**. `OnNpcClick` firable (Phase 33);
  `OnNpcInteraction` / `OnItemOnNpc` / `OnNpcsInteraction` /
  `OnAnyNpcInteraction` / `OnBeforeNpcInteraction` / `OnNpcRoute`
  table-only. `OnPlayerLogin`, `OnPlayerDisconnect`, `OnNpcDialogueCondition`
  firable. `OnItemAction` / `OnItemOnItem` firable (Phases 32/33).
  `Hub.EmitNpcInteraction` exists but runs ONLY onNpcClick bindings
  (`internal/plugins/npcinteraction.go` header: "the generic families slot
  in after them when they land").
- Transcription precedent (parent judgment, Phase 33): transcribing upstream
  table values into Go source with line citations is normal porting
  methodology; copying a data FILE as an asset is a verbatim import needing
  Tyler's approval. Document which path each table took.

## §2. Tables (`internal/content/fishing/tables.go`)

Port verbatim with `Fishing.plugin.js:line` citations (same method as Phase
32's `tables.go`):

- Constants: `FISHING_ACTION_INTERVAL_TICKS = 5`,
  `NET_CAGE_HARPOON_INTERVAL_TICKS = 6`, `KARAMBWAN_INTERVAL_TICKS = 4`,
  `NET_CAGE_FIRST_ROLL_TICKS = 3`, `HARPOON_ROD_FIRST_ROLL_TICKS = 4`,
  `FISHING_ANIMATION_INTERVAL_TICKS = 5` (plugin.js:28–37); `ANIM`
  (HARPOON 618, CAGE 619, BIG_NET 620, NET 621, ROD 622, ROD_ON_SPOT 623,
  VESSEL 1193, BARBARIAN_ROD 9349, BARBARIAN_ROD_ON_SPOT 9350, barehand
  6703/6704/6706/6707/6711); messages (`CAST_NET`, `CAST_LINE`,
  `CAST_AND_ATTEMPT`, `START_HARPOON`, `START_BAREHAND`,
  `FULL_INVENTORY_STATEMENT` + sound 2277, bait refusals, level gates).
- `Fish` (~50 entries, plugin.js:~180–330): id (gameval name, resolved via
  `Load`/`LoadFromCache` like Woodcutting's 393 names; numeric constants
  only where the cache has no gameval name, each verified by display name
  against the real cache — the Phase 33 clue-nest precedent), level, rolls
  [low, high] (out of 256, at levels 1 and 99), xp, caught text, petBase
  (Heron base; null = no pet roll), requires/extraXp, amount fn, message,
  junk flag, catchAnimation, big [id, 1-in-n, name]. Includes the big-net
  junk (boots/seaweed/gloves/oyster/casket), park-pond finds (7.5 XP,
  `junk: true`), leaping fish (barbarian requires/extraXp), dark crab
  diaryRolls (wilderness elite [[13,55],[3,40]]), karambwanji amount
  (floor(level/5)+1), minnow (chart `[24, 148]` — see §7, fitted estimate).
- `FishingTool` (24 tools, plugin.js:~340–480): id, level, animation,
  loopAnimation, catchAnimation, fish (highest level first — the cascade
  order), variants (best first: echo/crystal/infernal/dragon/plain
  harpoons with bonuses 135/120/100; pearl rods = plain), bait, worn,
  quest (`"Swan Song"` monkfish, `"Barbarian Training"` barbarian rod),
  multi (big net), requires, interval (incl. `pikeInterval`: 6 one time in
  5, else 5), firstRoll, animateOnRoll, unboostable (minnow), spot
  (MinnowPlatform — deferred, nil here), vessel (Karambwan), stationary
  (park, karambwan vessel), start/follow/miss messages, noToolStatement /
  noBaitStatement / noToolMessage, levelStatement, table (park pond:
  rolls [[20,20]], preRoll jade 1/100 + red topaz 1/505, weighted
  glass 3 / boot 2 / net 2 / casket 2 / keys 1, tertiary loop-half
  1/1014), barehand, clueBase.
- `HARPOONS` variants (plugin.js:~230–250); rod variant sets
  (FISHING_RODS, FLY_FISHING_RODS, OILY_FISHING_RODS, BARBARIAN_RODS).
- Spot maps: `SPOT_TOOLS_BY_OPTIONS` (7 keys: "Small Net/Bait",
  "Lure/Bait", "Cage/Harpoon", "Big Net/Harpoon", "Net/Harpoon",
  "Use-rod", "Cage") and `SPOT_TOOLS_BY_NPC` (~30 NPC ids; numeric ids
  from `NpcIdentifiers.ts` — e.g. FISHING_SPOT_43 = 3317, FISHING_SPOT_30
  = 1530, FISHING_SPOT_55 = 4316, FISHING_SPOT_2/3/4/5 = 1497–1500,
  FISHING_SPOT_92 = 10653, FISHING_SPOT_40/41/42 = 2653–2655,
  FISHING_SPOT_63 = 4928, FISHING_SPOT_6 = 6784, FISHING_SPOT_130 =
  15384, FISHING_SPOT_68 = 6488, ROD_FISHING_SPOT_16 = 6825,
  ROD_FISHING_SPOT_20 = 7676, FISHING_SPOT_87–90 = 7730–7733,
  FISHING_SPOT_58 = 4710, FISHING_SPOT_60/61/62 = 4712–4714,
  FISHING_SPOT_101 = 13329; Camdozaal 10686/10687/10688). Verify every
  id against the real cache by display name ("Fishing spot" /
  "Rod Fishing spot") in the loader test.
- `FISHING_SPOT_NAMES = ["Fishing spot", "Rod Fishing spot"]`,
  `FISHING_SPOT_OPTIONS` (10 options), `CLUE_BOTTLE_FISH` set,
  `PARK_CLUE_BASE = 152120`.
- Loader: `Load` / `LoadFromCache(defs)` in the Fletching/Woodcutting
  shape; fail-closed on unresolvable names; a fallback table (like
  Fletching's 176-entry `item_ids.go`) only for names the cache
  mis-resolves, each pinned with a test. Load-prove against the real
  rev-242 cache via a temporary test, then delete it (prior-phase
  pattern).

## §3. Seam: `onNpcInteraction` (new hub family → 57/108)

TS grounding: `PluginManager.ts:3488-3499` (`onNpcInteraction(name,
actions)` → `registerNpcActions`), `:3160-3180` (name-based hook:
definition name must match, action looked up by
`definition.getActions()[clickType-1]`, `action(event) !== false` claims
it), `:1246-1263` (emit order: before → specific → any; stop when
handled). `PluginTypes.ts:167-176` (`PluginNpcInteractionEvent`: player,
npc, definition?, npcId, npcIndex, clickType, location, handled).

Go shape (follow `internal/plugins/npcinteraction.go` + the
`dialoguewiring.go:389-434` adapter):
- `Api.OnNpcInteraction(name string, actions map[string]NpcInteractionHandler)`
  where `NpcInteractionHandler = func(*NpcInteractionEvent) bool`
  (reuse the `NpcClickHandler` shape: false falls through). Validate like
  TS: empty name or no function-valued actions → warn + drop (mirror the
  `registerNpcActions` guards; read them at `:3155-3160` before writing).
- Registry: name-keyed hooks in registration order, run by
  `Hub.EmitNpcInteraction` AFTER the onNpcClick bindings (TS runs
  `NpcInteractionManager.handle` before `emitNpcInteraction`; the
  `npcinteraction.go` header already reserves this slot: "the generic
  families slot in after them when they land").
- The event needs the NPC's name and the clicked option: extend
  `plugins.NpcInteractionEvent` with `DefinitionName string` and
  `Action string` (option text for the click type), filled by the
  `dialoguewiring.go` adapter from `listeners.NpcInteractionEvent`
  (`.Definition.Name()` + `.Definition.Actions()[ClickType-1]` — the
  listener interface already has `Actions() []string`,
  `listeners/npc.go:220-226`). Do NOT add the whole Definition to the
  hub event (the `npcinteraction.go` header says the hub never needs
  it — the name + action string are what the match needs).
- `OnNpcsInteraction` / `OnAnyNpcInteraction` / `OnBeforeNpcInteraction` /
  `OnNpcRoute` stay table-only (Fishing does not use them); update the
  unfired-family table in `hooks.go` and the hub counts (families, not
  methods — 56 → 57 firable, 60 → 61 `Api.On*` methods).
- `onItemOnNpc` stays table-only (only the deferred Gita exchange uses
  it); do not build it in this phase.

## §4. Core loop (`internal/content/fishing/fishing.go`)

Port `Fishing.plugin.js` in TS order. Session + 1-tick `FishingTask`
(`plugin.js:1001-1085`), scheduled exactly like Woodcutting's 1-tick task
(`woodcutting.go:616-660`: lazy `TaskManager` resolution, `ensureTask` on
first start, tick goroutine only — the Phase 32 race lesson: never
`Schedule` off the tick goroutine, never capture hub surfaces in
`Register`).

- `startFishing` (plugin.js:~955): `hasToolRequirements` → `inventoryBlocks`
  → stop old session (no anim reset) → set session `{npcIndex, npcId,
  spotX, spotY, tool, nextAnimationTick: tick+5, nextCatchTick:
  tick+min(firstRoll ?? ∞, catchInterval), followMessageTick: +1 if any}`
  → `tool.spot?.onStart` (nil in this phase — MinnowPlatform deferred)
  → `tool.vessel?.onStart` (Karambwan load) → spam startMessage → perform
  start animation. Register via `api.onNpcInteraction` for both spot names,
  all 10 options, one shared handler (`startFishingInteraction`:
  `getSpotTool(npcId, definition, clickType, player)` → tool or false —
  false leaves non-fishing spots to others). `getSpotTool` order:
  `SPOT_TOOLS_BY_NPC` → `SPOT_TOOLS_BY_OPTIONS` (joined non-empty option
  list) → harpoon→barehand fallback (`BAREHAND_SHARK`/`BAREHAND_HARPOON`
  when the player has no harpoon and `fishing:barehand-unlocked` answers
  true; unanswered → locked).
- `hasToolRequirements` (plugin.js:~870, exact order): level
  (unboostable → max level, else current; `levelStatement` box or "You
  need a Fishing level of at least N to do this.") → `requires` skills →
  quest (`questAllows`: TS `plugin.js:~680` fails OPEN when the quest is
  not registered — Go has no QuestRuntime, so Swan Song / Barbarian
  Training gates always pass; document as TS-faithful) → `findTool`
  variant (carried or wielded, best first; members tools are no tool on
  F2P worlds — `WorldDefinition.isMembersWorld()`; crystal needs Agility
  ≥ variant.agility; `fishing:tool` event fallback, unanswered → null) →
  `worn` (ice gloves / smiths gloves (i): "You need to wear ice gloves
  to handle these eels.") → bait (`findBait`: inventory first, then
  `storage:amount` query — Woodcutting precedent: unanswered → inventory
  count) / vessel ready (`KarambwanVessel.ready`).
- `catchChance(level, fish, bonus, roll)` (plugin.js:~905): interpolate
  low→high over levels 1→99, bonus scales low/high floored, value =
  floor(low·(99−level)/98 + high·(level−1)/98 + 0.5), chance =
  min(1, (1+value)/256). `catchLevel` = min(99, current) +
  Guild.invisibleBoost (§5).
- `rollCatch` (plugin.js:~935): table tools roll the table; otherwise the
  cascade — fish highest-first, skip members fish on F2P, level/requires
  filter, diary rolls (`diary:is-complete` query, unanswered → standard
  rolls), `fishing:catch-chance` query per roll (unanswered → computed
  chance), `multi` → every roll stands, else first success returns.
  `rollTable` (plugin.js:~920): preRolls first, else one weighted pick,
  plus tertiaries.
- Task tick (plugin.js:1001–1085, exact order): stop if unregistered /
  dead → stop if moving / in combat / force movement → skip if
  `levelUpPaused` → NPC gone/wrong id → stop → spot moved: stop, and if
  no spot module and `fishing:continue` answers handled and same z and
  within 24 → `walkToEntity` (Go `MovementQueue.WalkToEntity`, Phase 17
  seam) and restart within 2 (validate again inside the callback) → stop
  if player >2 tiles from spot → `hasToolRequirements` re-check each tick
  (stop if fail) → `inventoryBlocks` → stop (no anim reset) +
  `inventoryFull` (reset anim, sound 2277, statement "You can't carry
  any more fish.") → follow message at +1 tick + re-cast anim → loop
  animation every 5 ticks when `!animateOnRoll` → catch tick: next roll
  at `catchInterval` (`fishing:interval` query, unanswered → tool
  interval; min 1), loop animation on roll when `animateOnRoll` →
  `rollCatch` → `fishing:attempt` query (unanswered → keep the miss) →
  vessel bait per attempt → miss message → catchAnimation → spot
  `takesCatch` (nil this phase) → `fishing:catch` event → `landCatch` →
  stop if `stopAfterCatch` / no tool requirements / random 1/90 spot
  move (non-stationary, no spot module, `fishing:continue` unanswered).
- `landCatch` (plugin.js:~845): angler multiplier (§5) on XP;
  `hasRoom` per fish (free slot / own stackable stack / `storage:room`
  query — Woodcutting precedent, unanswered → false); `bigOrFish`
  (1/1000 bass, 1/2500 swordfish, 1/5000 shark); `amount` (karambwanji
  floor(level/5)+1, minnows by level); flaked roll
  (`SpiritFlakes.consumeFlake`, not for PARK_NET); `giveCatch`
  (`storage:gather` query first, rest to inventory — Woodcutting
  precedent); catch message via `spam` (Go `ChatTypeSpam = 105`,
  `internal/protocol/chat.go:50`); junk → `fishing:junk` emit (no
  answerer); infernal cook attempt per fish (`tryCookFish` — §5, never
  fires: no `cooking:raw-xp` answerer); crystal charge per caught fish
  (§5); XP via `SkillManager.AddExperiences` (int — **XP integer
  truncation**: minnow 26.1 → 26, park 7.5 → 7; the structural gap
  already in gap 16/17, document it here too) + `extraXp` skills;
  `landExtra` per landed fish (Rada/flakes, §5); clue bottles once per
  batch (§5; skip for the tutorial spot id 3317); `fishing:success`
  emit per fish kind with `petBase` (no pet system in Go — emit honestly,
  no answerer, the Woodcutting `pets:skilling-roll` precedent); bait
  consumed only on catch, one per cast (`useBait`: inventory first, else
  `storage:take` query — unanswered → not consumed).
- `statement()`: Go `game.SendStatementDialogue(p, text, true)`
  (`internal/game/dialogue_entries.go:622`) — the "Click here to
  continue" box. Full-inventory statement uses `canContinue` per TS
  (`StatementDialogue(0, text)` + `EndDialogue(1)` — ground the exact
  flag against the TS `statement()` at plugin.js:~945 before writing).
- Stops: `onPlayerDisconnect` (stop session, no anim reset — Fletching
  precedent); `level-up:popup-opened` via `skillactions.PauseForLevelUp`
  (Phase 20 pattern; the emitter is NOT STARTED so the pause never fires
  yet — document, Woodcutting precedent). Do NOT register
  `player:interaction-cleared` / `player:emote`: no Go emitter exists
  (Woodcutting header comment — follow it, do not invent an emitter).
- Wiring: `cmd/server/fishingwiring.go` + blank import in
  `cmd/server/pluginwiring.go` (Fletching pattern); cache + definitions
  load BEFORE the hub; `SetTables` before hub Load; table-load failure
  fails boot. Fishing is F2P — no `members:true` gate on the plugin
  (members filtering is per-tool/per-fish, as TS does it).

## §5. Sub-units (plug into the loop)

- **AnglerOutfit** (`angleroutfit.go`): `xpMultiplier` — hat 0.4%, top
  0.8%, waders 0.6%, boots 0.2%, +0.5% full set (spirit pieces count the
  same); `wearsFullOutfit`; set text via `equipment:effects` (follow the
  Woodcutting Lumberjack precedent — the producer is NOT STARTED, so the
  handler never fires; document). Gita Prymes exchange DEFERRED (§7).
- **CrystalHarpoon** (`crystalharpoon.go`): START 10,000 / MAX 20,000 /
  SHARD 100, meta key `"crystal-harpoon"`, signet ids 23943/23944/23945/
  25545/25543 (10% save), drain per caught fish in `landCatch` order,
  `Check` via `onItemAction`, shard recharge via `onItemOnItem` (firable
  since Phase 33). Woodcutting CrystalAxe precedent (Phase 33).
- **InfernalHarpoon** (`infernalharpoon.go`): COOK_CHANCE 3 / MAX 5,000,
  meta key `"infernal-harpoon"`, `tryCookFish` (1/3, `cooking:raw-xp`
  query, half Cooking XP, charge) + `tryCookKept`, `Check` via
  `onItemAction`, make/recharge via `onItemOnItem`. **Honest
  degradation: `cooking:raw-xp` has no Go answerer (Cooking unported),
  so the infernal cook NEVER fires** — wire it anyway, pin with a test
  (the infernal-axe precedent, Phase 33). Creation needs Fishing 75 +
  Cooking 85 (max levels, not boostable).
- **KarambwanVessel** (`karambwan.go`): `ready` / `load` (onStart) /
  `useBait` (per attempt) — pure inventory ops, exact TS order.
- **RadasBlessing** (`radasblessing.go`): tiers 4→1 at 8/6/4/2% (ammo
  slot, members only), `rollExtraFish(flakeChance)` — flakes' 50% first,
  then blessing on top; message "Rada's blessing enabled you to catch an
  extra fish." (fish only, not junk).
- **SpiritFlakes** (`spiritflakes.go`): `EXTRA_FISH_CHANCE = 0.5`,
  `consumeFlake` (one carried flake per landed fish, always consumed —
  tackle-box flakes NOT used).
- **ClueBottles** (`cluebottles.go`): `rollClueBottles(player, base)` —
  tiers beginner 2 / easy 17 / medium 20 / hard 33 / elite 100 (tenths),
  `bottleDenominator(base, level, tenths, combatAchievement)` =
  floor(adjusted·tenths / ((100+level)·10)) with the 95% combat-achievement
  adjustment, wilderness imbued-ring ×2 (not beginner), `treasure-trails:
  vessel-chance` / `combat-achievements:tier` / `treasure-trails:
  can-receive` / `treasure-trails:missed-clue` / `treasure-trails:clue-item`
  queries — all unanswered in Go. **Follow `cluenests.go` exactly for the
  can-receive / box-true handling** (read `cluenests.go:270-390` first);
  the box-true path is NOT built (no `forceAdd`); document the resulting
  behaviour precisely (what lands, what never lands, and why).
- **Camdozaal** (`camdozaal.go`): factory producing the Small Net
  (guppy 7/8, cavefish 20/16, tetra 33/24 — rolls [[32,192]] estimate,
  per the file header) and Big Net (catfish 46/33, tetra; clueBase
  100000) tools for NPCs 10686–10688; stationary, interval 6, firstRoll
  3, start "You cast out your net...". Register under
  `SPOT_TOOLS_BY_NPC`.
- **Guild invisible boost** (`guild.go`): `isInGuild` (bounds x
  2560–2623, y 3394–3436, z 0) + `invisibleBoost` (+7) wired into
  `catchLevel` (plugin.js:~655; `Guild.Fishing.js:22-28`). Door DEFERRED
  (§7).

## §6. Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build
  ./...`: clean. Full suite `go test ./... -p 1 -count=1 -v` with the
  exact passing-case count incl. subtests (baseline 4,413). `go test -race
  ./... -p 1 -count=1`: clean, 19+ packages.
- Live smoke (`/home/hatch/smoke35/`, new `fishing` mode, real rev-242
  cache, temp roots outside the repo, `RUNS` override, legs singly):
  **3 runs × checks, 0 failures.** Legs: (1) small net at a real spot —
  find the spot by cache-map probe (Phase 32 pattern: the woodcutting
  tree at (3086,3526) came from a probe; target a Small Net/Bait spot
  e.g. Lumbridge Swamp 1530 or Draynor — verify the NPC id + tile from
  the cache, never guess); observe the cast line, net anim 621, a catch
  ("You catch some raw shrimps."), fish in inventory, Fishing XP. (2)
  bait rod: feathers/bait consumed per catch, level gate message.
  (3) spot behaviour: walk away stops fishing; full inventory stops with
  the statement + sound. Harpoon/crystal/infernal legs are unit-covered
  only unless the harness can reach them — state plainly what was and
  wasn't observed live.
- No `zz_*` probe files, no `.js` files, no probe references in the repo
  after the phase. No data file imported.

## §7. Deferred (next phase's brief — do not half-land)

- **MinnowPlatform** (`fishing/MinnowPlatform.Fishing.js`, 12,722 B):
  (a) minnow mechanics — `ATTEMPT_INTERVAL_TICKS = 2`, `CATCH_CHART =
  [24, 148]` (fitted estimate, not a measured curve — the file says so),
  `minnowsPerCatch` 10@82 … 14@99, spot rotation every 25 ticks around
  the two 8-tile pond loops (ponds at (2609,3443) and (2617,3443)),
  flying fish 1/10 on first click after a move, eats 16–26 minnows per
  catch until the spot moves; needs an NPC-relocation seam (Go NPCs have
  no `moveTo` — ground `NPC.MovementCoordinator` / drivers first, or
  build the seam); (b) Kylie Minnow access — varbit 5669, attribute
  `fishing:minnow-platform`, 82 Fishing (max) + Fishing Contest complete
  + full angler outfit, transcript page "Kylie Minnow" (not in the repo's
  `{}` dialogue starters — transcript data is a gap); (c) row boats
  (`onObjectInteraction "Row boat"`, landings (2614,3440,0) /
  (2600,3425,0)); (d) minnow exchange — 40 minnows → 1 noted raw shark
  via entered-amount prompt.
- **AnglerOutfit Gita exchange** (`AnglerOutfit.Fishing.js`): needs
  `onItemOnNpc` (table-only — build the family + listener routing when
  this unit is taken) + `npc-dialogue:condition`/`choice`/`action`
  (`OnNpcDialogueCondition` IS firable; the `npc-dialogue:*` custom
  events need a Go emitter — ground the dialogue engine first) +
  `npc-dialogue:start` + transcript data (Gita Prymes page, not in the
  repo). Spirit-flake upgrade: 1,200 flakes first time per piece,
  attribute `fishing:angler-spirit-paid`.
- **Guild door** (`Guild.Fishing.js:30-75`): `door:toggle` custom event +
  `ObstacleRunner.run` (one tile per tick through the doorway). Needs an
  ObstacleRunner / ForceMovement seam — no ForceMovement system exists in
  Go (gap 16). Door tile (2611,3394,0), level 68 (boosts count), object
  id 422 (`ObjectIdentifiers.DOOR_422` — verify against the cache).

## §8. Docs

- This file (plan + outcome). `PORT_STATUS.md`: header verification
  numbers, "What was verified", P3 hub row (57/108, 61 methods, 5 named
  overloads — recount, don't trust the plan), P3 content row (8 of 577),
  Phase 35 paragraph, new gap 18 (residuals), Next modules (Fishing
  landed; MinnowPlatform/Guild/Gita deferred; next skill is Tyler's
  pick). `CONTENT_MIGRATION_PLAN.md`: skills progress paragraph.
  `SKILLS_SURVEY.md` only if a survey claim proves wrong (note the
  correction — e.g. if the hook flags or the "no data file" row are wrong).

## Outcome (coordinator-verified)

Phase 35 landed 2026-11-21 (coordinator-verified, parent to confirm).
Fishing — the third skill, F2P, Tyler's pick 2026-11-20 — is ported:
core loop + 8 of the 10 sub-units (AnglerOutfit, CrystalHarpoon,
InfernalHarpoon, KarambwanVessel, RadasBlessing, SpiritFlakes,
ClueBottles, Camdozaal tools, Guild invisible boost). MinnowPlatform,
the AnglerOutfit Gita exchange, and the Guild door are deferred per
§7, unchanged. No data file was imported in this phase — Fishing has
none (survey §5 row, confirmed by the full read), and no file was
copied; the tables were transcribed into Go source with TS line
citations (Phase 33 transcription precedent).

What landed, per §2–§6 item (all in `internal/content/fishing/`,
25 Go files; `internal/plugins/` for the seam; `cmd/server/` for
wiring; harness in `/home/hatch/smoke35/`):

- §2 tables + loader (Worker A): `tables.go` (1,612 lines — 50 fish,
  24 tools, 7 spot options, 29 spot NPCs, 29 clue-bottle fish,
  84 distinct item ids, 84 gameval names, 0-entry fallback),
  `camdozaal.go`, `tables_test.go` (11 tests). Load-proved against
  the real rev-242 cache (temporary test, since deleted): 84 ids
  checked by numeric id + display name and 29 spot NPC ids by
  display name, 0 failures; every "missing" lowercase guess has a
  true Jagex gameval, so no `item_ids.go` fallback table was needed.
  Loader shape is Woodcutting's `Load(r)` / `LoadFromCache(defs)`
  (no data/path args — there is no file to load), not Fletching's.
  `DARK_CRAB_POT` relies on TS defaults; `PARK_NET` table chance
  `[[20,20]]` and the minnow chart `[24,148]` are upstream
  estimates, not measured curves.
- §3 onNpcInteraction seam (Worker B): `internal/plugins/
  npcinteraction.go` — `NpcInteractionEvent` extended with
  `DefinitionName`, `Action`, and `Actions []string` (the joined
  list is needed: "Bait" alone is ambiguous across spots),
  `NpcInteractionHandler` (true claims), named-hook registry,
  `Api.OnNpcInteraction(name, actions)`, `Hub.EmitNpcInteraction`
  running onNpcClick bindings FIRST, then name hooks — TS order
  (`NPCOptionPacketListener.ts:122` handle, `:132` emit;
  `PluginManager.ts:1246-1263`). `cmd/server/dialoguewiring.go`
  adapter fills the new fields from `.Definition`. Hub recount
  by grep: **61 `Api.On*` methods, 57/108 families firable**
  (was 60/56). Plan correction: §8 said "5 named overloads" —
  there are exactly 4 (`OnObjectInteractionNamed`,
  `OnItemOnObjectNamed`, `OnItemActionNamed`,
  `OnItemOnItemNamed`); 61 − 4 = 57. `OnNpcsInteraction` /
  `OnAnyNpcInteraction` / `OnBeforeNpcInteraction` / `OnNpcRoute`
  stay table-only.
- §4 core loop (Worker B): `fishing.go` (~58 KB) — struct with
  exactly the mandated fields (`api`, `tables`, `ids`) plus
  session/task/rand; Register attaches `OnPlayerDisconnect`,
  `level-up:popup-opened` via `skillactions.PauseForLevelUp`, and
  `OnNpcInteraction` for both spot names × all 10 options, plus
  the three sub-unit register calls in TS attach order; 1-tick
  task exactly per Woodcutting (lazy TaskManager, `ensureTask`
  on first start). Full loop in TS order: `startFishing`,
  `hasToolRequirements` (exact TS order), `catchChance` (single
  floor of the sum — one citation corrected by the coordinator:
  the function is at `plugin.js:725-732`, not `:902-910`, which
  is the session creation), `rollCatch`/`rollTable`, `landCatch`,
  `inventoryFull` (sound 2277 + statement), `statement` via
  `game.SendStatementDialogue`, spam via chat type 105. All 13
  Worker-C methods called with the specified signatures; XP
  truncation documented at the grant site (`int(fish.XP *
  xpMultiplier)` — 26.1→26, 7.5→7). Two documented deviations:
  the session holds the NPC pointer, not the world NPC index
  (`Api` exposes no NPC-by-index lookup — stricter than TS: a
  respawned spot never resumes an old session), and
  `variant.baitless` is unexpressible (Go `ToolVariant` has no
  Baitless field — every found variant needs bait). No
  ForceMovement check (no such system in Go, same as
  Woodcutting); `player:interaction-cleared` / `player:emote`
  not registered (no Go emitter).
- §5 sub-units (Worker C): `ids.go` (`FishingIDs` +
  `LoadIDs`/`LoadIDsFromCache`, fail-closed; all 39 gameval names
  probe-verified against the real cache, temporary test deleted),
  `angleroutfit.go` (XP multiplier hat .4/top .8/waders .6/boots
  .2% + .5% set, spirit counts same; set texts via
  `equipment:effects`), `crystalharpoon.go` (10,000/20,000/100,
  signet 10% save, drain, shard recharge; shared charge helpers),
  `infernalharpoon.go` (COOK_CHANCE 3 / MAX 5,000; make 75
  Fishing + 85 Cooking), `karambwan.go`, `radasblessing.go`
  (tiers 4→1 at 8/6/4/2%), `spiritflakes.go` (50%),
  `cluebottles.go` (tiers 2/17/20/33/100 tenths, 95% CA
  adjustment, Wilderness imbued-ring ×2), `guild.go`
  (`isInGuild` + invisible boost +7), `world.go` (shared
  Wilderness handle + `SetWilderness`). C reads only
  `f.tables`, `f.ids`, `f.emitCustomEvent`, plus
  `f.membersWorld` (its draft `SetMembersWorld` collided
  with B's and was removed — one source of truth).
- §6 wiring (Worker B + coordinator): `cmd/server/
  fishingwiring.go` (`installFishingTables` — members flag,
  `LoadFromCache`, `SetTables`, ids, `SetIDs`,
  `SetIsMembersItem`; load failure fails boot; nil defs →
  no-op degradation) + `pluginwiring.go` (blank import +
  call) + `pluginwiring_test.go` (Fishing loads before
  Woodcutting, Go init order). Coordinator addition:
  `installFishingWilderness` in `fishingwiring.go`, called
  from `buildServer` right after `newAreaManagerAndWilderness`
  (`server.go`) — the Wilderness does not exist until then
  (it is built after hub Load), and the handle is only read
  at catch time on the tick goroutine, so installing it
  before the engine starts is in time; the "before hub Load"
  instruction in `world.go` was corrected to match. The
  imbued-ring ×2 now has a production call site.
- Smoke harness (Worker B + coordinator fixes):
  `/home/hatch/smoke35/` — `main.go`, `protocol/`,
  `run_smoke35.sh` (RUNS override, MODE env),
  `seed_save35.py` (4 validated variants). Four modes:
  `fishing` (L1 net, spot 1530), `fishing-rod` (L5 rod),
  `fishing-fly` (L20 fly rod, spot 1527), `fishing-full`
  (28 distinct items). Three harness bugs found by the
  coordinator's first smoke runs and fixed before the
  passing runs: (1) the driver wrote no `npc-spawns.json`,
  so the world had zero NPCs and no spot existed — added
  per-mode spawns in the temp root only (smoke27 goblin
  precedent; coordinates are upstream's own:
  1530 at (3240,3146), 1527 at (3238,3252));
  (2) Bait was clicked with opcode 12 (option 2) — spot
  1530's actions are [Net, "", Bait], so Bait is action
  slot 3, opcode 34 (TS: `getActions()[clickType-1]`);
  the fly mode already had this right; fixed in both
  `fishing` and `fishing-rod`; (3) the fly check expected
  "You catch a raw trout." with a period — TS is explicit
  (`plugin.js:222`) that trout's message has NO period,
  and the Go table mirrors it; check fixed. (4) the
  `fishing-full` check expected 0 raw shrimp in the save
  but the seed itself holds one 317 among the 28 items —
  check now asserts the count stays at the seeded 1 and
  Fishing XP stays 0.

Measured verification (coordinator, final state after all
edits): `go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...` — all clean. `go test ./...
-p 1 -count=1 -v`: **4,534 passing test cases incl.
subtests, 0 failures, 20 packages** (baseline 4,424 + 110:
A 11, B 18 loop tests + 4 new NPC-interaction tests +
pluginwiring updates, C 38; fishing package 67 cases).
`go test -race ./... -p 1 -count=1`: clean, 20/20, 0 data
races. (The pre-existing `TestEngineTicksAtRate` timing
flake appeared once in a worker's run and passed
serialized; the final full run had 0 failures; no
tolerance change.) 735 `.go` files (was 709), 20 packages.
No probe references in the repo (`zz_`/`ZZLoadProve`/
`ZZProbe`/`seed-root` clean); no `.js` files; `~/tsps-bfd81e3`
untouched. Grounding spot-checks by the coordinator:
`catchChance` matches TS exactly (single floor);
`EmitNpcInteraction` order matches
`NPCOptionPacketListener.ts:122/:132`; the adapter's
`Actions()[ClickType-1]` matches TS `:1246-1263`.

Live smoke (coordinator, `/home/hatch/smoke35/`, real
server binary + real rev-242 cache, fresh temp roots,
legs singly): **4 modes × 3 runs, all PASS, 0 failures.**
- `fishing` 3/3 (9 checks each): spot 1530 found at
  (3240,3146); Bait at Fishing 1 refused with the level
  box "You need at least level 5 Fishing to bait these
  fish." and no rod anim; Net → "You cast out your
  net..." + anim 621 + "You catch some raw shrimps.",
  save 1 shrimp (317), 10 Fishing XP (skill 10);
  walk-away stops (no further shrimp in 30 s, save at
  (3252,3160,0)).
- `fishing-rod` 3/3: "You cast out your line..." + anims
  622/623 + "You catch a raw sardine.", save 1 sardine
  (327), bait 20→19, 20 XP.
- `fishing-fly` 3/3: spot 1527 at (3238,3252); Lure →
  "You catch a raw trout" (no period, TS-faithful) +
  anims 622/623, save 1–9 trout (catch-chance variance),
  feathers consumed, 50 XP per trout; Bait refused at
  level 20 with "You need at least level 25 Fishing to
  bait these fish." (pike gate).
- `fishing-full` 3/3: "You can't carry any more fish."
  + sound 2277 on the wire, net anim 621 never plays,
  save still holds only the seeded shrimp and 0 XP.
Honest limits: only these four legs were observed live;
the harpoon/crystal/infernal/karambwan/Rada's/spirit-flake/
clue-bottle/guild legs are unit-covered only (no harness
modes for them); spot depletion/movement (the 1/90 random
move) was not observed live; no SkillLevelUp feedback was
observed live (no emitter exists yet).

Deferred brief status (§7, unchanged): MinnowPlatform
(needs NPC-relocation seam + Kylie + boats + exchange),
AnglerOutfit Gita exchange (needs `onItemOnNpc` —
still table-only — + `npc-dialogue:*` emitters + Gita
transcript), Guild door (needs ObstacleRunner /
ForceMovement). The next skill after Fishing is Tyler's
pick; the Woodcutting Guild import question is still
pending with him (asked 2026-11-20, unanswered).

Open questions for the parent: none needing a decision.
C's order-dependence note (two of its tests failed in a
full-package run mid-session but pass in isolation) was
re-checked at integration: the final full suite (4,534)
and full race run are both clean, and the fishing package
is fully green — no action needed.
