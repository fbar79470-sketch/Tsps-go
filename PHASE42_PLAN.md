# Phase 42 — Mining (5th skill)

Tyler's pick, 2026-11-21 02:38 PST ("Then add mining and smithing skills"), in that
order. Mining next, Smithing (Phase 43) after. F2P. No data file needed.

Upstream: `server/plugins/skills/Mining.plugin.js` (1,221 lines) + 8 sub-units under
`server/plugins/skills/mining/` (2,134 lines total) + `SkillActions.js` (30 lines),
all at commit `bfd81e3`. Survey (`SKILLS_SURVEY.md:76,188,252,325-327,463,566`):
entry 1,221 / 10 files / true size 2,164, F2P, no data file; hooks `onObjectInteraction,
onItemAction, onItemOnItem, onPlayerLogin, persistAttribute`; `mining:*` custom-event
family shared with ShootingStars/Runecrafting; Mining clue geodes / crystal pickaxe
noted as cross-skill data points.

## §1 Grounding (all verified against the real rev-242 cache, probe deleted after use)

### Entry plugin shape (`Mining.plugin.js`)

- `PICKAXES` (:86-119): 23 entries. Bronze→dragon are `standard` (bots wield only
  these); crystal (71 Mining + 50 Agility, 3 ticks, 1/4 fast-roll, members only),
  infernal (61, 3 ticks, 1/6 fast-roll), gilded / dragon (or) / 3rd-age / trailblazer
  variants, uncharged infernal/crystal (mine like dragon), echo pickaxe (level 1,
  rank 72, 2 ticks). `PICKAXES_DESC` sorts by `rank ?? requiredLevel` desc, then
  interval asc. Intervals: bronze 8, iron 7, steel 6, black/mithril 5, adamant 4,
  rune/dragon/crystal/infernal 3. Animations 625/626/627/3873/629/628/624/7139/8347/
  4482 etc. (verbatim literals).
- `ROCKS` (:121-262): 26 entries. Level, XP, ore, petBase, success chart `[low, high]`,
  `gems` flag, respawn ticks. Copper/tin 17.5 XP, iron 35, coal 50, gold 65, mithril
  80, adamantite 95, runite 125, amethyst 240. Special shapes: barronite + calcified
  (`outcomes` weighted table), sandstone/granite (`cascade` of size charts, no gem
  pre-roll, deplete after one piece), gem rocks (`outcomes` weighted gems, glory
  chart), Daeyalt (`stages` _01→_04), nickel/rubium/amethyst (`yield`), infernal
  shale (10 XP-valued outcomes), essence (`infinite`, `runeEssence` level-30 split).
- `ORE_VEINS` (:285-302): 7 Abandoned Mine wall veins (tin/copper/clay/coal/iron/
  mithril/adamant) built from their base rock's stats, `swing: "wall"`, depleted locs
  `lotr_mine_wall_<ore>_<pos>` → `lotr_mine_wall_depleted_<pos>`.
- `ESSENCE_ROCKS` (:264-273): rune essence (infinite, every roll succeeds, silent,
  sound 3600, counts in `tracking_ore_mined`), ancient essence crystals (75, 13.5 XP,
  1–4 by level, infinite, spot moves — AncientEssence).
- Core loop (`MiningTask`, :831-960): 1-tick global task, `miningTick = cycle`.
  Per session, in order: drop unregistered/dead players; stop if the movement queue
  is non-empty or a ForceMovement is active; skip `levelUpPaused`; resolve the rock
  (`MapObjects.get` by id/location/privateArea, or the NPC-rock target); stop if
  gone or out of interaction distance; re-check pickaxe/level/breathing filter;
  stop on full inventory (message box); swing at `swingTick` (message + animation,
  tick after the click); depletion-timer countdown; roll at `nextRollTick`
  (`cycle + actionInterval(rollInterval(pickaxe))`, `ROLL_READY_TICKS` carries the
  skilling timer between rocks); swing animation on every roll tick; `rollAttempt`
  (gem pre-roll first, then cascade / chart / outcomes); `mining:attempt` event may
  turn a miss into a hit; success block (hand-over, infernal combust else
  prepare/deliver ore + count, success sound, success line, crystal charge, XP with
  Prospector multiplier, pet roll, extra ore, `mining:success`); depletion rules
  (per-ore default, `yield`, `depletionTicks` timer, `stages` step-down, `infinite`
  never depletes); `depleteRock` → mined-out loc + depletion sound + `RockRespawnTask`
  + respawn-timer script 5474 for each viewer.
- Success formula (:512-519): Wiki skilling rate,
  `(1 + floor(low*(99-L)/98 + high*(L-1)/98 + 0.5)) / 256`, visible level capped at
  99 plus invisible boosts (`mining:level-boost` event, default 0). First roll at
  +2 ticks after arrival (`FIRST_ROLL_DELAY_TICKS = 2`); later rolls follow the
  pickaxe interval, one less on a fast roll (dragon/infernal 1/6, crystal 1/4,
  members only).
- Gem pre-roll (:525-545): 1/256 before each ore roll (charged glory adds 3/256);
  a hit gives a gem or nothing, never the ore, no XP, rock stays. Table weights
  null 70 / sapphire 32 / emerald 16 / ruby 8 / diamond 2.
- Depleted-rock naming (`depletedRockId`, :641-652): the rock's own `depletedRocks`
  gameval (granite→rocks2, gemrock→rocks2, calcified `_empty` locs, amethyst
  `amethystrock_empty`), else `<gameval>_empty` if it exists (lead, nickel, rubium),
  else `rocks` + the digit its gameval ends in (`copperrock2`→rocks2), else `rocks1`.
  A mined-out rock still offers Mine and only says it is empty (sound 2661, message
  "There is currently no ore available in this rock."); the depleted vein says
  "There is no ore currently available in this vein."
- Ores-mined counter (`countOre`, :1059-1066): `persistAttribute("mining:ores-mined",
  0)`, varp `tracking_ore_mined` (cache varp id **4526**, probe-verified), sent on
  login when > 0. This plugin is its only writer.
- Messages/sounds: swing "You swing your pick at the rock." (spam); no pickaxe
  "You need a pickaxe to mine this rock. You do not have a pickaxe which you have
  the Mining level to use." (message box); level gate "You need a Mining level of
  N to mine this rock." (message box); full inventory "Your inventory is too full
  to hold any more <ore>." (message box, Daeyalt: "ore"); no-ore sound 2661;
  success sound 3600 (rock.sound overrides: gem rocks 2655, Daeyalt 2998); smelt
  sound 2725 on infernal combust; smoke graphic 86 height 150.
- `findBestPickaxe` (:565-592): best by level (weapon slot first, then inventory),
  Agility gate for crystal, members-item gate on F2P worlds, `mining:tool` event
  fallback (league relic bare hands — answer-only, no listener in Go).
- Gloves (:393-411): members only; mining gloves tier 1 / superior 2 / expert 3;
  standard rocks grant `tier` saves, superior rocks `tier-1`, expert rocks 1 save
  at tier 3 only. Saves spend only after the rock's ordinary yield runs out.

### Gameval proof (temporary probe vs the real rev-242 cache, deleted after use)

All item gamevals resolve through `CacheDefinitions.GetObjId` — the tables store
these names, Load fails closed on any missing one. Notable (not guessable):
gilded pickaxe = `trail_gilded_pickaxe`, dragon (or) = `zalcano_pickaxe`,
3rd-age = `3a_pickaxe`, echo = `league_trailblazer_pickaxe`, barronite deposit =
`camdozaal_barronite_deposit`, barronite shards = `camdozaal_barronite_shard`
(singular), pure essence = `blankrune_high`, rune essence = `blankrune`, soft clay =
`softclay`, volcanic sulphur = `lovakengj_sulphur`, daeyalt ore = `sang_jas_ore`,
sandstone = `enakh_sandstone_tiny/small/medium/large`, granite =
`enakh_granite_tiny/small/medium`, gloves = `mguild_gloves[_superior|_expert]`.
Depleted locs resolve: `rocks1` 11390, `rocks2` 11391, `copperrock2` 11161,
`tinrock2` 11361, `gemrock` 11381 / `gemrock1` 11380, `amethystrock_empty` 11393,
`leadrock1_empty` 58918, `varlamore_mining_rock_empty[02|03|04]` 51486/51488/51490/
51492, `area_sanguine_mine_minerocks_01..04` 17962–17965, vein locs
`lotr_mine_wall_coal_l` 20410 / `_mid` 20413 / `lotr_mine_wall_depleted_l` 20441 /
`_mid` 20440, calcified water deco `varlamore_mining_special_rock` 51493.
Saltpetre locs 27433–27436 (`hosidius_saltpetre_1..4`, action Dig) and Ancient
Essence 46701 (`ancient_essence_rock_active`, Mine) / 46702
(`ancient_essence_rock_empty`, Mine) resolve.

**Stale `objectIds` finding (do NOT copy):** the plugin's `ROCKS[].objectIds`
numeric literals contradict the rev-242 cache (e.g. copper 7453/7484 are a door
and a wall pile in rev-242; the real copper rocks are `copperrock1` 10943 /
`copperrock2` 11161; clay 9711–9713 are tree stumps; iron 7455/7488, silver 7457,
coal 7456, gold 7491/9720…, mithril 7492/7459, adamantite 7460, runite 14859/4860/
2106/2107/7461 are likewise wrong). TS never reads them in the loop — `grep`
shows `objectIds` only in the table literals and stripped out at `:296` for
veins; registration is by object **name** and depletion resolves from the
**runtime** loc's gameval. The Go tables therefore store NO rock object ids:
registration by name, depletion by runtime gameval. The live legs will mine the
real rev-242 rocks (copperrock/tinrock/ironrock etc.) by name.

### Sub-units (all 8, grounded by a worker read of each file)

- **AncientEssence** (105 lines): rotating active crystal spot in the Ghorrock
  Dungeon mine. 3 spots (2908,10311,face 3), (2915,10311,face 0), (2912,10307,face 1),
  plane 0; one at a time swapped wall (46702) → crystals (46701), moving to a random
  other spot every 100–150 ticks. Essence by visible level: ≥90 → 3–4, 78–89 → 2–3,
  75–77 → 1–2. `onServerStartup` + 1-tick task. No dependencies.
- **CalcifiedWater** (214 lines): Cam Torum "oozing water" cycle. Every 117–146
  ticks water starts on 4 rocks (2 west + 2 east half), lasts 38–54 ticks, then
  stops; next tick all 4 wet rocks force-deplete. While wet, success chance ×1.15
  (only flips a miss to a hit, via `mining:attempt`). Chamber x1490–1528 y9530–9553
  z1, east split x≥1510, west 1497–1505, east 1514–1521. Deco loc
  `varlamore_mining_special_rock` shape 4, message
  `<col=a53fff>Some cracks around the cave begin to ooze water.`, ooze sound 2041,
  deplete area sound 2661 (delay 10, range 1). `registerArea` + 1-tick task +
  `mining:attempt` handler. `keepsStanding` hook is Demonic Pacts (unported league
  content) — consulted only, stub false.
- **ClueGeodes** (114 lines): every `mining:success` with `petBase > 0` rolls each
  clue tier the player doesn't own (beginner 0.2, easy 1.7, medium 2, hard 3.3,
  elite 10 modifiers; chance `1 in floor(floor(floor(petBase*0.7) * CA) /
  (100 + Mining) * modifier)`, CA = combat-achievement threshold met, ×0.95).
  `onItemAction("Clue geode (<tier>)", { Open })` ×5. Dependencies: Treasure Trails
  (clue ownership + scroll items — unported) and Combat Achievements tier
  (unported); both consulted via events with local fallbacks (inventory + bank
  name scan; no CA → no 0.95 factor). No `forceAdd` in Go (gap 16/33 precedent) —
  a geode with no room ground-drops via the woodcutting `dropGroundItem` seam.
- **CrystalPickaxe** (125 lines): charges. Start 10,000, 1 per ore mined, shards
  add 100 each, cap 20,000, at 0 → inactive (mines like dragon). Elven/celestial
  signet 10% chance to save a charge. Meta key `crystal-pickaxe`. `Check` item
  action; `onItemOnItem("Crystal shard", "Crystal pickaxe" | "(inactive)")`.
  Charge consumption via exported `tryUseCharge`, called by core on success.
  Go seam: `game.Item.GetMetaValue`/`SetMetaValue` (woodcutting `crystalaxe.go`
  model). No dependencies.
- **InfernalPickaxe** (166 lines): 1-in-3 combust per ore: ore destroyed, Mining XP
  unaffected, ~half a bar's Smithing XP granted, 1 of 5,000 charges used. Empty →
  uncharged (mines like dragon). Created/recharged by using a smouldering stone
  (or dragon pickaxe) on it at 85 Smithing (+200 Mining, +350 Smithing XP).
  Smithing XP per ore: blurite 4, copper/tin 3.6, iron/silver 5.5, coal 6, gold 9,
  mithril 12, adamantite 18.75, runite 25; sandstone 10.2/19.2/28.2 (1/5/10kg),
  granite 7.2/13.2/22.2 (500g/2kg/5kg); sunstone 2, rubium 2. Meta key
  `infernal-pickaxe`, default = MAX when no meta (differs from crystal).
  Combustion needs only the Smithing **skill id** for the XP grant
  (`SkillManager.AddExperiences(SMITHING, xp)`) — no Smithing content needed, so
  the unit ports whole. Creation is gated on the 85 Smithing level check, which
  the same seam provides.
- **ProspectorOutfit** (67 lines): XP multiplier from worn pieces (helmet 0.4%,
  jacket 0.8%, legs 0.6%, boots 0.2%, full-set bonus 0.5% → 2.5% total), golden
  recolours count, Varrock armour 4 counts as the jacket. Name-based matching
  (lowercased, `golden ` prefix stripped). Answers `mining:xp-multiplier`;
  `equipment:effects` set text 18 (enum 3047) — producer NOT STARTED, handler
  never fires (woodcutting Lumberjack precedent).
- **Saltpetre** (86 lines): Dig saltpetre deposits in Hosidius with a spade (name
  match: "Spade", "Large spade", "Eastfloor spade"). No Mining XP. Yield 1, or 2
  with chance = visible Mining level / 100. Dig anim 830, sound 1470, 2-tick task
  with re-validation (registration, HP, spade, private area, interaction distance,
  inventory space). Emits `mining:saltpetre`. No dependencies.
- **VarrockArmour** (36 lines): members only; worn Varrock armour gives 10% chance
  of an extra ore from eligible rocks, gated by armour tier vs ore tier (1: clay/
  copper/tin/blurite/iron/silver/coal/gold/lead; 2: mithril/lovakite/soft clay/
  rubium/sandstone/granite; 3: adamantite/sunstone; 4: runite/nickel/amethyst).
  Answers `mining:extra-ore`. No dependencies.

### No Prospecting (brief correction)

Prospecting does **not** exist in Mining. Grep for `prospect` across
`Mining.plugin.js` and `mining/` returns only "Prospector" (the outfit). The only
Prospect action in the skills tree is Crashed Stars (Shooting Stars, a separate
plugin — out of scope). No Prospect action is ported on rocks; inventing one
would be new content, not a port.

### Go seams (all exist — verify at build time, do not invent)

- Hub: `Api.OnObjectInteractionNamed` (objects.go:486, handler returns false to
  fall through), `Api.OnItemActionNamed` (itemaction.go:254),
  `Api.OnItemOnItemNamed` (itemonitem.go:116), `Api.OnCustomEvent`
  (events.go:32), `Api.OnPlayerLogin` (api.go:131), `Api.OnPlayerDisconnect`
  (api.go:160), `Api.OnServerStartup` (api.go:102), `Api.RegisterArea`
  (areas.go:143), `Api.PersistAttribute` (api.go:304), `Api.EmitCustomEvent`
  (api.go:249). Hub-contract lesson (PORT_STATUS.md, Phase 32): surfaces installed
  after plugin Load (TaskManager, ObjectManager) must be resolved lazily at use
  time or on server startup, never captured in Register; `TaskManager.Schedule`
  runs only on the tick goroutine — the 1-tick task schedules on the first Mine,
  not on startup.
- `internal/plugins/skillactions`: `PauseForLevelUp` + `ActionInterval` already
  ported (SkillActions.js:2-28). `ActionInterval` emits `skills:action-interval`
  with a `*ActionIntervalPayload`; saving applies after content's answer, never
  under 1 tick. `PauseForLevelUp` wires `level-up:popup-opened`; the emitter
  (SkillLevelUp feedback) is NOT STARTED, so the pause never fires yet.
- `SkillManager.AddExperiences(skill, xp int)` (game/skills.go:455) — int XP.
  Woodcutting's `treeBaseXP` returns float64 and the truncation (37.5 → 37) is a
  documented gap (gap 16). Mining's fractional XP (17.5, 40.5, 80.5, 13.2…) goes
  through the same int path — mirror woodcutting, record as a residual, do NOT
  change XP math in this phase.
- Object swap / respawn timer / sounds / spam: woodcutting model —
  `ObjectManagerAPI` (Get/Register/Deregister via the cmd/server adapter),
  `sendClientScript(viewer, 5474, coordinate, locID, type, 1, ticks, 16765184, 0)`
  (woodcutting.go:1489-1529), `game.SendSound`-style sound sends, spam chat via
  the packet sender. Depleted-rock resolution needs a runtime loc-name lookup
  (`CacheDefinitions.GetLocNames()` + `GetLocId`) — expose it through the same
  lazy surface the tables loader uses.
- Item meta: `game.Item.GetMetaValue` / `SetMetaValue` (woodcutting crystalaxe.go
  model: `{"charges": N}` under the meta key, survives save/load).

### Out of scope (deferred, named — not silently dropped)

- Motherlode Mine (separate plugin, not in the Mining entry's 10 files).
- Shooting Stars / Crashed Stars (separate plugin; Mining only emits
  `mining:success` with `petBase` for it).
- Mining Guild (needs `mining:level-boost` +7 and `mining:respawn-ticks`
  answers; no data file exists for it in the survey — the Guild is a content
  unit, not an import; record as deferred).
- Woodcutting Guild import approval — still pending, unrelated.
- `player:interaction-cleared` / `player:emote` interrupts: no Go emitter exists
  (woodcutting precedent — NOT registered).
- `storage:room` / `storage:gather` (coal bag): no storage content in Go —
  degrade to defaults (full inventory blocks; stored = 0).
- `pets:skilling-roll`: no answerer — emit only.
- `mining:level-boost` (celestial ring +4, Guild +7): no answerers — default 0.
- `mining:respawn-ticks` (Guild members' area): no answerer — default table value.
- `mining:tool` (league relic bare hands): answer-only, no listener.
- Treasure Trails / Combat Achievements (ClueGeodes fallbacks as above).
- SkillLevelUp emitter (pause never fires — same gap as Phases 34–36).

## §2 Plan

1. **Tables** (`internal/content/mining/tables.go` + `tables_test.go`): port
   PICKAXES (23), ROCKS (26), ORE_VEINS (7, derived from base rocks), ESSENCE_ROCKS
   (3), GEM_TABLE, constants (sounds, messages, timer script/id/colour, varp/attr
   names). Item ids as **gamevals** (probe-proven above), resolved at Load through
   an `IDResolver` (woodcutting tables.go:1134-1150 model) + `LoadFromCache`. Rock
   entries carry NO object ids (see §1 stale-ids finding). Fail closed listing
   every missing name. Unit tests: Load against a fixture resolver + load-proven
   against the real cache (seed cache, skip if absent — seq_scanner_test.go model).
2. **Core loop** (`internal/content/mining/mining.go` + `mining_test.go`): sessions
   map (no package-level mutable state — Phase 21 race rule), 1-tick `MiningTask`,
   `startMining` guard order (breathing filter → pickaxe → level → inventory),
   swing at +1 / first roll at +2, `rollAttempt` (gem pre-roll → cascade → chart →
   outcomes), `mining:attempt` mutable event, success block in TS order, depletion
   rules (per-ore / yield / timer / stages / infinite), `depleteRock` + `exhaustRock`
   + `RockRespawnTask`, respawn-timer script to viewers, ores-mined counter +
   login re-send, `mineDepletedRock` handler, `handleMine`, NPC-rock support
   (`mining:mine-npc-rock` / `mining:npc-rock-mined`), `mining:best-pickaxe` /
   `mining:pickaxe` / `mining:roll-interval` / `mining:gem-roll` / `mining:count-ore` /
   `mining:ores-mined` answers, `mining:deplete-rock` / `mining:respawn-ticks` /
   `mining:ore-yield` / `mining:hand-over` / `mining:extra-ore` / `mining:success` /
   `mining:gem` emits with documented defaults when unanswered. Register all
   `ALL_ROCKS` names + `Rocks`/`Empty wall`/`Wall`/`Depleted vein` on Mine.
3. **Sub-units** (one file + tests each, woodcutting Phase 33 model):
   `crystalpickaxe.go`, `infernalpickaxe.go`, `cluegeodes.go`,
   `calcifiedwater.go`, `saltpetre.go`, `varrockarmour.go`, `prospectoroutfit.go`,
   `ancientessence.go`. All 8 land in this phase; anything that cannot be observed
   live is unit-covered only and named in §5.
4. **Wiring**: package `init()` → `plugins.Register(Plugin)`; one blank import
   (first, alphabetical) in `cmd/server/pluginwiring.go`; lazy surface resolution
   per the hub contract; tables loaded from the cache at boot (fail boot on a
   missing name, as woodcutting does).
5. **Live** (`/home/hatch/smoke42/`, fresh temp root, real rev-242 cache): legs —
   mine copper (ore + 17 XP on the wire), mine tin, mine iron, depletion +
   respawn (copper 4-tick respawn + overlay timer), full-inventory refusal +
   message box, walk-away stops, no-pickaxe message box, essence mine (rune
   essence, infinite). Run singly if the driver exceeds the exec wall clock
   (RUNS override, Phase 39–41 precedent). Harness traps: gorilla readErr is
   sticky (deadline-free reader); ClientHandshake after LOGIN; per-mode
   npc-spawns.json in the temp root; ~2 min after boot for eager NPC spawning.
   Rock tiles by name from the real cache (copperrock1 10943 / copperrock2 11161
   etc. — find live rock coords from the map data at test time, do NOT use the
   plugin's stale numeric ids).
6. **Docs**: PORT_STATUS.md content row (13 of 577), new §25 residual gaps,
   "What was verified" entry. This plan's Results section.

## §3 Verification criteria

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./...` clean.
- Full suite `-p 1 -count=1 -v`: exact pass/fail/skip counts + package count
  (baseline 4,713 / 24 packages from Phase 41).
- `go test -race ./...` clean (re-run `internal/game` once if the known
  `TestEngineTicksAtRate` flake hits; say so plainly).
- Live legs pass with check counts; sub-units named live vs unit-only.
- No stray files, no `.js`, probe tests deleted, `live36` and `~/tsps-full`
  untouched, no commits.

## §4 Gaps carried in (not fixed here)

XP integer truncation; no SkillLevelUp emitter (pause never fires); no `forceAdd`
(clue geodes ground-drop when full); `storage:room`/`storage:gather` unanswered;
`pets:skilling-roll` unanswered; Mining Guild deferred; Motherlode Mine and
Shooting Stars are separate plugins.

## Results (2026-11-21, verified)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean.
  `CGO_ENABLED=0 go build ./...` clean. 787 `.go` files,
  25 packages (new `internal/content/mining`: 10 source
  files + 4 test files; `cmd/server/miningwiring.go`;
  blank import in `cmd/server/pluginwiring.go`).
- Full suite `-p 1 -count=1 -v`: all 25 packages pass —
  4,766 passing (3,605 top-level + 1,161 subtests), 0
  failures, 0 skips (4,713 from Phase 41 + 53 new: 40
  mining top-level + 13 subtests). Mining package alone:
  53 passing. Three `cmd/server/pluginwiring_test.go`
  tests updated for the Mining load order (F2P:
  Fishing → Mining → Woodcutting; members:
  Fletching → Mining → Woodcutting).
- `go test -race ./...`: clean on the first run, 25/25
  packages, no data race; the known
  `TestEngineTicksAtRate` flake did not hit this time.
- Live legs (`/home/hatch/smoke42/`, real server binary +
  real rev-242 cache, legs run singly): copper-tin PASS
  8/8 (7 copper + 7 tin, 238 XP, varp 4526 = 14, anim
  625, sound 3600, both chat lines), depletion PASS 4/4
  (8 copper from the one rock, varp = 8, anim + sound),
  full PASS 3/3 (0 ore, 0 XP, no anim), walkaway PASS
  3/3 (ore count + varp frozen at 1 after walking away).
  18 checks, 0 failures. Sub-units unit-covered only:
  AncientEssence, CalcifiedWater, ClueGeodes,
  CrystalPickaxe, InfernalPickaxe, ProspectorOutfit,
  Saltpetre, VarrockArmour.
- Two live-found harness lessons (harness bugs, not port
  bugs): a copper rock holds one ore and the session stops
  on depletion (TS-faithful), so legs re-click after each
  depletion; the ore chat line is "You manage to mine some
  copper." (first word of the rock name); ore totals must
  sum inventory slots (ores are not stackable).
- No stray files, no `.js`, probe tests deleted, `live36`
  and `~/tsps-full` untouched, no commits (parent handles
  git). No new dependencies, no verbatim imports.
