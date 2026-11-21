# Port status

Server-only Go port of TSPS (`RSPSApp/tsps`, commit `bfd81e3`, cache
target `osrs-242_2026-11-03`). The TypeScript client is kept. The
full upstream analysis (module inventory, protocol/login/tick
findings) lives outside this repository; status values below mean:
**DONE** (implemented, tests pass), **PARTIAL** (some behavior
ported, gaps listed), **NOT STARTED** (absent — nothing here
pretends otherwise).

Last updated: 2026-11-21 (phase 37 run: RSProx gateway
endpoints — `GatewayTarget`, approved by Tyler
2026-11-20. Two static endpoints on the game port:
`GET /gateway/jav_config.ws` (30 lines, byte-exact vs
TS, param=25 = cache revision, host from `PUBLIC_HOST`
or 127.0.0.1) and `GET /gateway/worldlist.ws`
(one-world binary list, members flag from the world
definition) — both live-verified against the real
rev-242 cache, members and non-members, with and
without `PUBLIC_HOST`. The external runelite-gateway
process itself was NOT run (out of scope). Hub counts
unchanged (57 of 108 families firable, 61 registration
methods — 4 named overloads). 4,664 tests passing.
Quests are postponed per Tyler's 2026-11-16
instruction (Doric's Quest picked but deferred); Tyler
decided 2026-11-17: combat leftovers first, then adding
all skills — see "Next modules".)

## What was verified in this run

- Phase 40 (`go build ./...`, `go vet ./...`, `gofmt -l .`:
  clean. `CGO_ENABLED=0 go build ./...`: clean. 768 `.go`
  files, 23 packages (new `internal/content/wildernessditch`,
  `internal/game/forcemovement.go`).
- Phase 40 `go test ./... -p 1 -count=1 -v`: all 23 packages
  pass (4,696 passing test cases: 3,548 top-level + 1,148
  subtests, 0 failures, 0 skips — 4,678 from phase 39 + 18
  new: 8 game ForceMovement/click-delay/mobility/OnStop, 8
  wildernessditch resolveJump/guards/flow, 2 sync
  forced-movement views; the temporary gap-11 probe test
  was deleted after use).
- Phase 40 `go test -race ./...`: passes clean, 23/23
  packages, no data race — the phase-37-suspect
  `combatant.go` `SetCombatCycleSource` race still does not
  reproduce.
- Live phase-40 smoke tests (throwaway harness in
  `/home/hatch/smoke40/`, four modes driving a real
  WebSocket client against the real server binary + real
  rev-242 cache from a temp root outside the repo; legs run
  singly): ditch-south PASS 4/4 (Cross on (3090,3521) from
  (3089,3520) → landed (3090,3523), jump anim 6132 and sound
  2462 delay 25 on the wire), ditch-north PASS 4/4 (Cross
  from (3089,3523) → landed (3090,3520)), ditch-turn PASS
  4/4 (Cross on (2996,3530) face 3 from (2995,3531) →
  landed (2998,3530)), ditch-wall PASS (walked north without
  Cross, stayed at (3089,3520)). Arrival-tile finding: a
  diagonal corner tile does not reach a 1x2 solid
  (`reachRectangle1`), so the player steps to the
  side-adjacent tile before jumping and lands 3 tiles from
  the arrival tile, not the click-time tile. Gap 11's wall
  leg is RESOLVED by this phase: the clipped rows are the
  ditch itself (probe evidence in gap 11).
- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 761 `.go` files,
  21 packages (new `internal/session/gateway.go`).
- `go test ./... -p 1 -count=1 -v`: all 21 packages pass
  (4,664 passing test cases: 3,516 top-level + 1,148
  subtests, 0 failures, 0 skips — 4,648 from phase 36 + 16
  new, exactly the 11 gateway tests + 5 handler routing
  tests).
- `go test -race ./...` (full run) does NOT pass cleanly
  on this tree under load — and the same run on the clean
  phase-36 tree (commit `93250d7`, git worktree) fails
  the same way, so this is pre-existing, not a phase-37
  regression: (a) `internal/game` `TestEngineTicksAtRate`
  fails (the known 5 ms timing flake under race
  slowness; it passes serialized, tolerance untouched);
  (b) `cmd/server` timing-sensitive savewiring tests fail
  under race load — on the baseline tree three of them
  time out (`TestShutdownEngineDrivenSavesWalkedPosition`,
  `TestReloginResumesAtWalkedTile`,
  `TestReloginRoundTripOverWebSocket`: "timed out waiting
  for walk arrival / disconnect save"); on this tree
  `TestReloginResumesAtWalkedTile` failed with one genuine
  data-race report instead: package-level combat-cycle
  source written by `SetCombatCycleSource`
  (`internal/game/combatant.go:484`, via
  `InstallCombatSources` in `cmd/server/combatwiring.go:413`
  during a second `buildServer`) while a first server's
  tick goroutine reads it (`combatCycle()`,
  `combatant.go:495`). That race is pre-existing code the
  phase did not touch (the phase diff is only
  `gateway.go`, `handler.go`, `server.go` wiring,
  `main.go`, `doc.go`, and tests) and it does not
  reproduce in isolation: the single test passes 3/3
  with `-race` on both trees. It is a real race worth
  fixing, but it is not this phase's scope; the
  non-race suite above is green.
- Live phase-37 checks (throwaway roots outside the repo,
  real server binary + real rev-242 cache, roots kept at
  `/home/hatch/live37/` and `/home/hatch/live37-f2p/`):
  server on :49703 with the real cache — `GET
  /gateway/jav_config.ws` → 200, 649 bytes, byte-exact
  against an expectation derived independently from
  `GatewayTarget.ts` (41 parsed lines, host 127.0.0.1,
  port 49703, `param=25=242`); `GET
  /gateway/worldlist.ws` → 200, 30 bytes, decoded
  length=26, count=1, id=1, flags=0x1 (members), host
  127.0.0.1, activity `tsps`, location 0, population 0.
  Routing: query string → 200, POST → 200 (no method
  check, as in TS), `/gateway/jav_config.ws/extra` →
  404, unknown paths → 404, plain `/` → 404, both
  responses carry `Access-Control-Allow-Origin: *` and
  the correct Content-Type. Second server on :49704
  with `PUBLIC_HOST=play.example.com` and an F2P
  world.json (freetoplay plugin enabled + an `f2p`-tagged
  zone covering the spawn, temp root only): codebase
  `http://play.example.com/`,
  `param=17=http://play.example.com:49704/gateway/worldlist.ws`,
  worldlist flags=0x0, host `play.example.com`,
  activity `tsps`, 37 bytes. Both live servers stopped
  afterwards; the `/home/hatch/live36` servers were not
  touched.
  Honest limits: only the three smoke legs were observed
  live; Gnome, Churn, SacredEel, LeapingFish, doughs,
  karambwan, the other 167 recipes, and the infernal-harpoon
  cook are unit-covered only; no SkillLevelUp feedback was
  observed live (no emitter exists yet).
- Live phase-35 smoke tests (throwaway harness in
  `/home/hatch/smoke35/`, four modes driving a real
  WebSocket client against the real server binary + real
  rev-242 cache from temp data roots outside the repo;
  harness + roots kept at `/home/hatch/smoke35/`): 4 modes
  × 3 runs, 0 failures, each from a fresh temp root, runs
  executed singly via a `RUNS` override (a full pass
  exceeds the background-exec wall clock). `fishing`
  (Fishing 1, small net 303, spot 1530 at (3240,3146)):
  Bait at level 1 refused with the level box "You need at
  least level 5 Fishing to bait these fish." and no rod
  animation; Net → "You cast out your net..." + anim 621
  + "You catch some raw shrimps.", save 1 shrimp (317)
  and 10 Fishing XP (skill 10); walking away stops the
  loop (no further shrimp in 30 s, save at (3252,3160,0)).
  `fishing-rod` (Fishing 5, rod 307, bait 313 ×20, same
  spot): "You cast out your line..." + anims 622/623 +
  "You catch a raw sardine.", save 1 sardine (327), bait
  20→19, 20 XP. `fishing-fly` (Fishing 20, fly rod 309,
  feathers 314 ×20, spot 1527 at (3238,3252)): Lure →
  "You catch a raw trout" (no period — TS is explicit,
  `plugin.js:222`) + anims 622/623, save 1–9 trout
  (catch-chance variance), feathers consumed, 50 XP per
  trout; Bait refused at level 20 with "You need at
  least level 25 Fishing to bait these fish." (pike gate).
  `fishing-full` (28 distinct items, same spot):
  "You can't carry any more fish." + sound 2277 on the
  wire, net anim 621 never plays, save still holds only
  the seeded shrimp and 0 XP. Three harness bugs were
  found by the first smoke runs and fixed before the
  passing runs (all harness-side, none in the port):
  (1) the driver wrote no `npc-spawns.json`, so the world
  had zero NPCs — per-mode spawns added in the temp root
  only (smoke27 goblin precedent; coordinates are
  upstream's own); (2) Bait was clicked with opcode 12
  (option 2) — spot 1530's actions are [Net, "", Bait],
  so Bait is action slot 3, opcode 34 (TS
  `getActions()[clickType-1]`); (3) the fly check expected
  a period after "trout" that TS does not send, and the
  full check expected 0 shrimp though the seed itself
  holds one 317. Honest limits: only the four smoke legs
  were observed live; the harpoon/crystal/infernal/
  karambwan/Rada's/spirit-flake/clue-bottle/guild legs are
  unit-covered only; spot depletion/movement (the 1/90
  random move) was not observed live; no SkillLevelUp
  feedback was observed live (no emitter exists yet).
- Live phase-34 smoke tests (throwaway harness in
  `/home/hatch/smoke34/`, `fletching` mode, a copy of the
  phase-32 harness with a `ServerWidgetRunScript` (opcode
  110) recorder case plus `useOnItem` (INVENTORY_USE_ON 241
  kind 4) and `resumeMenu` (RESUME_PAUSEBUTTON 252)
  senders, driving a real WebSocket client against the real
  server binary + real rev-242 cache from temp data roots
  outside the repo, with a Fletching level 5 seed — knife
  946, logs 1511 ×10, bow string 1777, unstrung shortbow
  50, bow string spool 31052; harness + roots kept at
  `/home/hatch/smoke34/`): 3 runs, 11 checks each, 0
  failures, each from a fresh temp root, runs executed
  singly via a `RUNS` override (a full 3-run pass exceeds
  the background-exec wall clock). Knife on logs opened
  the CreationMenu (script 2046 on the wire, alongside
  2379); choosing arrow shafts Make-All produced 10
  "You carefully cut the wood into 15 arrow shafts."
  messages and fletching anim 1248 on the wire, and the
  save held 150 arrow shafts (52) and 50 Fletching XP
  (skill 9). Bow string on the unstrung shortbow strung
  it in one batch with NO second menu (2046 count stayed
  1), "You add a string to the bow." and anim 6678 on the
  wire, and the save held 1 shortbow (841) and 55 total
  Fletching XP. Bow string on the spool filled it —
  "You add 5 bow strings to the spool. It now holds 5."
  on the wire, and the save held
  `fletching:spool-charges` == 5. Honest limits: only
  the three smoke legs were observed live (shafts,
  stringing, spool fill); the other 167 recipes, the
  diary tasks, and the chisel/hammer paths are
  unit-covered only; no SkillLevelUp feedback was
  observed live (no emitter exists yet).
- Live phase-33 smoke tests (the phase-32 harness in
  `/home/hatch/smoke32/`, `woodcutting` mode, re-run as a
  regression on the final code): 3 runs, 9 checks each, 0
  failures, each from a fresh temp root. A real axe
  chopping a real tree: "You swing your axe at the tree."
  on the wire, bronze chop anim 879 (+ 65535 reset), "You
  get some logs." twice, the tree depleting to stump 1342
  at the tree tile, the respawn overlay client script 5474,
  the tree respawning at the same tile, and the save holding
  2 logs (1511, one per slot — logs do not stack), 50
  Woodcutting XP (25 per log, skill 8), and
  `woodcutting:logs-chopped` == 2. The stump 1342 matches
  the load-time memo cross-checked against the real cache
  (temporary probe, deleted): the normal tree 1276 is 2x2
  in the cache, and all 273 tree object ids carry a memo
  entry and a footprint. Honest limits: the six sub-units
  are unit-covered only live — no Ent kill/chop, no vine
  clearing, no clue-nest roll, no crystal/infernal Check
  or charge drain was observed over the wire (the harness
  has no modes for them); nest drops unobserved live (the
  normal tree has `birdNests: false`; the 1/256 rate is
  unit-covered only).
- Live phase-32 smoke tests (throwaway harness in
  `/home/hatch/smoke32/` — a copy of the phase-30 harness
  with a `woodcutting` mode driving a real WebSocket
  client against the real server binary + real rev-242
  cache from temp data roots outside the repo, with staged
  fixtures — a bronze axe (1351) in the inventory, the
  repo's shipped `item-alch.json` + `item-gameplay.json`
  for the weapon interface — nothing copied into the
  repo; harness + roots kept at `/home/hatch/smoke32/`):
  3 runs, 9 checks each, 0 failures, each from a fresh
  temp root. A real axe chopping a real tree: "You swing
  your axe at the tree." on the wire, bronze chop anim
  879 (+ 65535 reset), "You get some logs." twice, the
  tree depleting to stump 1342 at the tree tile, the
  respawn overlay client script 5474, the tree respawning
  at the same tile, and the save holding 2 logs (1511,
  one per slot — logs do not stack), 50 Woodcutting XP
  (25 per log, skill 8), and `woodcutting:logs-chopped`
  == 2. The tree is a normal tree (id 1276 at 3086,3526,
  3 tiles from spawn — the tile was found by a
  cache-map decode probe, since the server never sends
  `LocAddChange` for untouched map trees; the real client
  reads them from its own cache) and depletes on the
  first successful log, per the TS rule. Two bugs were
  found by the first smoke run and fixed before the
  passing runs: (1) `treeAt` never found real map trees
  — it only consulted `ObjectsAt` (runtime
  registrations), so every chop failed with "You can't
  reach that tree right now"; fixed with the
  `ObjectManagerAPI.Get` seam (`MapObjects.get`). (2)
  Register-time capture of the TaskManager/ObjectManager
  read nil (both install on the hub after plugin Load),
  so the loop could never tick; fixed by lazy resolution
  at use time — and the first form of that fix, an
  `OnServerStartup` schedule, raced the tick goroutine
  (`go test -race` caught it in `cmd/server`); the
  startup hook was removed and the 1-tick task now
  schedules on the first chop, which runs on the tick
  goroutine through the turn-packet drain. All three
  passing runs used the final code. Honest limits: nest
  drops unobserved live (the normal tree has
  `birdNests: false`; the 1/256 rate is unit-covered
  only); walk-away/disconnect stop not separately
  observed live (the tree depletes after each log
  anyway; the disconnect save landed normally).
- Live phase-30 smoke tests (throwaway harness in
  `/home/hatch/smoke30/` — an extension of the phase-29
  harness with varp-frame, projectile-height, and
  NPC-hit-timestamp recording plus four new modes —
  driving real WebSocket clients against the real server
  binary + real rev-242 cache from temp data roots
  outside the repo, with staged fixtures — two goblin
  spawns (id 3030) + one `monsters-complete.json` entry
  carrying only `hitpoints: 50` for the goblin (a 5-HP
  goblin dies on a dark bow's first hit, so the second
  hitsplat never lands; smoke28's max_hit 8 / attack 99
  staging was tried first and made the goblin lethal
  across the long fights — corrected to hitpoints-only),
  the repo's shipped `item-alch.json` +
  `item-gameplay.json` for the weapon interfaces —
  nothing copied into the repo; harness + roots kept at
  `/home/hatch/smoke30/`): 3 runs, 8 modes each, 34
  checks per run, 0 failures. Melee 3/3, ranged 3/3
  (bronze-arrow projectile heights 163/146 — 40.75/36.5
  ×4 — on the wire), magic 3/3, profile 3/3 (ascension
  crossbow attack anim 15114, the profile's
  npcAttackAnimation vs an NPC target, never the 4230
  style fallback; ascension bolt projectile 4437),
  darkbow 3/3 (dragon-arrow projectile 1120; the two
  hitsplats of one shot landed 597–601ms apart — exactly
  one tick — in every run), orb 3/3 (varp 300 == 1000
  and varp 301 == 0 at login), crystalbow 3/3
  (projectile 249; the bow stayed equipped, slot 4214,
  after firing), death 3/3 (anims [422 424 836], "Oh
  dear, you are dead!", respawn (3089, 3524, 0), HP
  99/99). Harness corrections during the phase: the
  profile check first expected 15113 (the vs-player
  anim) — the expectation was wrong, the code is
  TS-faithful; the darkbow check first failed on a 5-HP
  goblin (first-hit kill) — fixed by the 50-HP staging.
  A full 3-run driver pass exceeds the background-exec
  wall clock (~520s SIGTERM mid-run-2), so runs were
  executed singly via a `RUNS` override: run 1 8/8,
  run 2 8/8, run 3 8/8, each from a fresh temp root.
  The coordinator applied Worker C's two special-fire-
  sound call-site hunks (melee + ranged `Start`) before
  verification. Honest limits: deathComponents splitting
  is unit-covered only (live PvP still blocked — no PvP
  kill exists to observe it on); the NPC-with-spell
  branch is unit-covered only (no production path sets
  an NPC's cast spell); the queued-graphic losing case
  is unit-covered only.
- Live phase-29 smoke tests (throwaway harness in
  `/home/hatch/smoke29/` driving real WebSocket clients
  against the real server binary + real rev-242 cache from
  temp data roots outside the repo, with staged fixtures —
  two goblin spawns (id 3030), the repo's shipped
  `item-alch.json` + `item-gameplay.json` for the
  bow/scimitar/staff interfaces — nothing copied into the
  repo; harness + roots kept at `/home/hatch/smoke29/`):
  3 runs, 4 modes each. Melee 3/3, 4 checks each, all
  passing — player anims 390 (scimitar chop) + 388
  (scimitar block), goblin attack 6184 + death 836 on the
  wire. Ranged 3/3, 5 checks each, all passing — player
  anim 426, projectile 10 (bronze arrow), fire sound 2702
  (private synth), start graphic 19 at height 100 (the
  coordinator's new check, proving the populated launch
  tables fire), goblin death 836, ammo slot 499 → 497 per
  shot. Magic 3/3, 8 checks each, all passing — cast anim
  1162 (the staff variant of 711), start graphic 90 at
  height 100, projectile 91, impact graphic 92 at height
  100 delay 56, cast sound 220 + impact sound 221, goblin
  death 836, mind runes paid (100 → 98/99). Death 2/3 in
  the driver + 1/1 clean re-run by the coordinator, 4
  checks each — player anims [422 424 836], "Oh dear, you
  are dead!", respawn at (3089, 3524, 0), HP 99/99. The
  single driver failure was run 1's death leg: alice
  survived at HP 2/99 when the harness's 240s deadline
  hit (a timing boundary in the harness, not a defect —
  no code in this phase touches death timing or damage;
  runs 2, 3, and the re-run all died and respawned
  cleanly). The coordinator closed the one open item the
  workers left: ranged launch graphics are now populated
  (all 193 ammunition kinds carry script-extracted
  start/double-start graphics — 82 with a start, 10 with
  a double — plus the venator/dark-bow/blowpipe weapon
  flags), and fixed two stale "no producer sends them yet"
  comments.
- Live phase-28 smoke tests (throwaway harnesses in
  `/home/hatch/smoke27/` (poison, extended by the
  coordinator with a relogin leg) and `/home/hatch/smoke28/`
  (death) plus `/tmp/smoke28/` (skull timer), driving real
  WebSocket clients against the real server binary + real
  rev-242 cache from temp data roots outside the repo, with
  staged fixtures — two cave-crawler spawns (id 406) + one
  `monsters-complete.json` entry carrying the poisonous flag
  for poison; two goblin spawns (id 3030) + one
  `monsters-complete.json` entry (max_hit 8) + one minimal
  `item-alch.json` entry for death; staged skull timers for
  skulls — all in the temp roots, nothing copied into the
  repo; harnesses + roots kept at `/home/hatch/smoke27/`
  and `/home/hatch/smoke28/`): poison 3 runs stable, 5
  checks each, all passing — a real cave crawler poisoned
  alice ("You have been poisoned!" + orb varp 102 == 30 on
  the wire), the disconnect save carried `poisonDamage` 30
  with the poison-clock snapshot (`counter` 0 — the counter
  is the protection dose, and none was taken — with
  `ticksUntilPulse` 30), and after a relogin the orb varp
  showed again (> 0), proving the affliction resumed live
  from the save. In run 3 alice died mid-run and was
  re-poisoned after respawn (orb 29/30, then 0 at death,
  then 30 again) — an accidental live proof that the death
  reset clears poison. Death 3 runs stable, 17 checks each,
  all passing — "Oh dear, you are dead!" + one jingle
  (opcode 132), ground-item frames at the death tile
  (3094, 3524) carrying bones (526) + the three lost items
  (grimy guam 200, grimy marrentill 201, 1,000 coins), the
  disconnect save at the world spawn (3089, 3524, 0) with
  HP 10/10 and exactly the three kept items (rune scimitar
  1333, ranarr potion (unf) 100, irit potion (unf) 101),
  kept ∪ dropped == the 6 seeded items. Skull timer
  re-run by the coordinator against the integrated tree:
  staged red skull timer 3000 counted down to 2984 over
  ~10s online (16 ticks, icon 1 survived), and a staged
  timer 5 expired to exactly 0 with no underflow. The
  coordinator fixed one integration item the workers left
  open: poison live state now round-trips through the save
  (snapshot writes + entry restore, `PlayerSave.ts:656-660`
  / `:764-771` order), and the death attribute reset now
  clears poison (TS `resetAttributes`). The live PvP-attack
  leg is BLOCKED, honestly: `CanAttackByPolicy` denies PvP
  with no opinion, and the allow comes from the Wilderness
  plugin's `canAttack` (`Wilderness.plugin.js:469`), which
  is NOT ported — no live PvP attack can start at all until
  that content lands; the skull trigger is unit-covered
  instead, and the head-skull icon on the wire was not
  directly observed (it needs player-update frame parsing).
- Live phase-27 smoke tests (throwaway harness in
  `/home/hatch/smoke27/` driving a real WebSocket client
  against the real server binary + real rev-242 cache from
  temp data roots outside the repo, with staged fixtures —
  two goblin spawns (id 3030), one ground-item coins stack,
  one `item-alch.json` entry, one `item-gameplay.json`
  entry — all in the temp roots, nothing copied into the
  repo; harness + roots kept at `/home/hatch/smoke27/`):
  3 runs stable, 9 checks each, all passing. Session 1 (one
  login): a real High Alchemy on a rune scimitar paid 15,360
  coins (16,360 with the telegrab coins), consumed 5 fire +
  1 nature, and the scimitar left the inventory; a real
  Telekinetic Grab returned the staged 1,000 coins and
  consumed 1 law rune (its air rune covered by the
  listener's infinite-air staff set); a real Wind Strike on
  goblin A consumed 1 air + 1 mind and granted XP (Magic
  210,000 → 210,116/210,118 = wind strike + 65 alchemy + 43
  telegrab); the autocast pick (combat interface 593 child
  23, then autocast interface 201 slot 1) persisted as
  `autocastSpellId` 1152 in the logout save. Session 2
  (relogin): a plain Attack click on goblin B fired the
  restored autocast spell — 3–4 Wind Strikes in 10s, air +
  mind consumed one pair per cast (99 → 96/95), Magic XP
  +28/+30, autocast still 1152. No projectile frame is
  asserted: `StartCast` is a deliberate no-op this phase
  (Phase 29 owns the producers), so a cast sends no
  opcode-84 frame; casts are proven by runes + XP in the
  save. The coordinator fixed two integration bugs the
  smoke found: `autocastSpellId` was a preserved save field
  and never written from live state (now owned, per
  `PlayerSave.ts:756`), and the listener resolution needed
  the real client's widget-child/item values (High Alchemy
  child 47 / item 3303, Telekinetic Grab child 30 / item
  3290, Wind Strike child 11 / item 3273 — the cache
  spell-name fallback), not spell ids. The repo holds no
  cache directory: the rev-242 cache was downloaded once
  into the smoke seed root (242 MB, OpenRS2) and symlinked
  per run.
- Live SeekingArrows smoke test (phase 24, throwaway test in
  `cmd/server/` driving real WebSocket clients against the
  real rev-242 cache from a temp data root outside the repo,
  with a goblin spawn (id 3030, adjacent to spawn, wander
  radius 0) and a test-only ranged-data resolver registered
  in the test process — production registers none; 3 runs
  stable, 2 invocations; test + temp root since removed):
  the client parsed NPC_INFO frames with a throwaway
  decoder, sent real NPC-option-2 Attack clicks (the
  goblin's cache actions are `["", "Attack", ...]`), and
  observed hitsplat damages on the wire grouped into goblin
  lives bounded by respawn additions. Seeking-arrow lives
  ran `[3 2]` in all three runs of both invocations: every
  non-final hit landed exactly at the floor of 3, the final
  2 cap-bound at remaining HP (each life summing to the
  goblin's 5 HP). Rune-arrow control lives ran
  `[1 1 1 2]`/`[1 2 2]`/`[1 2 1 1]` (and
  `[1 1 2 1]`/`[1 2 1 1]`/`[1 1 1 1 1]` on the second
  invocation), proving sub-floor hits occur without the
  floor. The killing frame shows full health (death resets
  HP before the flush snapshot), so lives — not the health
  entry — classify killing blows; that timing is a
  phase-16 combat observation, not a phase-24 change.
- Earlier live checks stand: phase 23 verified
  LumbridgeGuide live (a real Talk-to click on the spawned
  guide played the NORMAL fixture line with the
  amulet-status query unanswered — proving the hook
  overrode the loader's default — and the LOST fixture line
  with a temporary probe answerer, 3 runs stable, 2
  invocations); phase 22 verified
  GracefulOutfit + WornWeights live (a full graceful set
  restored +19/tick at −25kg, five pieces +17/tick at
  −21kg, boots of lightness weighed −4kg, and six real
  "Wear" packets moved a character from +15/tick to
  +19/tick mid-session, 3 runs stable); phase 21 verified sailing/
  template regions/sounds live (sound + jingle +
  projectile frames on the wire, boat boarding with
  world-entity scene/info frames and a deck-scene
  rebuild, a teleport snap update, and a templated
  instance rebuild carrying the probe chunk, 3 runs
  stable); phase 20 verified skills/XP
  live (a stub skilling action granted XP through the
  framework over real WebSocket, the gate doubled one gain
  and refused another, the level-up and experience-gained
  hooks fired, the action-interval hook paced the grants,
  and a simulated level-up pop-up paused and resumed the
  session, 6 runs stable); phase 19 verified clans/
  social live (three clients: friend/ignore flows with
  TS-verbatim refusals, the ignore gate, PM delivery
  through the packed-bytes path, all social dispatch
  kinds reaching the hub, and a logout/relogin round-trip
  preserving the friends list, over real WebSocket, 3
  runs stable); phase 18 verified the
  interface runtime live (a login hook opened interface
  500, a widget click dispatched to the hub hook and armed
  an amount prompt, answering the prompt reached the hook,
  a varp set through the store reached the client, and
  walking away closed the interface, over real WebSocket, 3
  runs stable); phase 17 verified shops +
  trading live (a hook-opened shop and a dialogue-opened
  shop both showed stock, a buy deducted coins and a sell
  returned them, a two-player trade completed with the
  offered items transferred, and a decline left both
  inventories unchanged, over real WebSocket, 3 runs
  stable); phase 16 verified NPC drivers +
  combat live (wander, aggression, kill, respawn, death
  hooks) against the real rev-242 cache + the real upstream
  definitions directory read-only (30,827 NPC spawns, 1
  object spawn, 1,952 ground-item spawns, 512 shops, 38
  interface layouts, 0 interactions; nothing copied); phase
  5 saves round-tripped live on all three backends; phase 3
  verified the live OpenRS2 download and boot-time definition
  loading against the real rev-242 cache.

- All unit tests use hand-built fixtures only: no live Jagex
  assets, no network, no cache binaries under this repository.
  The exceptions are the live checks above (cache download +
  real music-areas data in temp roots, all cleaned up). The
  shipped `data/definitions/music-areas.json`
  and `music-areas-classic.json` are verbatim imports from the
  upstream TSPS data (SHA-256 verified identical), generated
  by `scripts/import-music-areas.mjs` — never hand-edited.
  The same applies to `data/definitions/item-alch.json`
  (16,750 entries) and `data/definitions/item-gameplay.json`
  (27,407 entries), imported verbatim 2026-11-17 on Tyler's
  approval (SHA-256 verified identical to upstream; a temporary
  load test against the real rev-242 cache proved rune scimitar
  high-alch 15360 and staff of air interface STAFF resolve,
  then the test was deleted) — without them a repo-root run
  refuses all alchemy and never recognises a staff. The same
  applies to `data/definitions/npc-combat-sounds.json`
  (460 NPC entries), imported verbatim 2026-11-19 on Tyler's
  approval (SHA-256 verified identical; a temporary load test
  proved NPC 2 death 274 / hit 275 resolve, then the test was
  deleted) — the definition Store loads it, but the game-level
  seam that would let DeathSoundFor use the captured fallback
  is still NOT STARTED. The same applies to
  `data/definitions/fletching.json` (170 recipes), imported
  verbatim 2026-11-20 on Tyler's approval (SHA-256 verified
  identical, `c9f64bea...`; valid JSON with 170 recipes under
  `recipes` plus `tools`/`alternatives`/`menus`/`groups`/
  `diaryTasks`) — Phase 34 built its Go loader
  (load-proven: 265 distinct names, 278 resolved). The same
  applies to `data/definitions/cooking.json` (122 cookables,
  47 recipes, 2 wines, 3 doughs, churn), imported verbatim
  2026-11-20 on Tyler's approval (SHA-256 verified identical,
  `5760fe60...`) — Phase 36 built its Go loader
  (`internal/content/cooking/tables.go`, `Load` /
  `LoadFromCache` via the definition Store), load-proven
  against the real rev-242 cache by a temporary test, then
  the test was deleted: 122 cookables, 47 recipe rows (40
  distinct products), 2 wines, 4 dough products, churn
  inputs 4 / products 7, 369 item names resolved with zero
  missing gamevals; the loader's knife fallbacks
  (KNIFE_2..5) and the four `ItemIds` constants
  (POISON_KARAMBWAN 3146, COOKING_GAUNTLETS 775, SACRED_EEL
  13339, ZULRAHS_SCALES 12934) are transcriptions from
  `ItemIdentifiers.ts`, not gamevals — the cache has no such
  gamevals (probe 2026-11-21).
  (`internal/content/fletching/tables.go`, `LoadFletching`
  via the definition Store), load-proven against the real
  rev-242 cache by a temporary test, then the test was
  deleted: 170 recipes, 13 groups, 8 menus, 4 diary tasks,
  265 distinct item names, 278 resolved ids (102 via
  lowercase gameval, 176 via the fallback table — 3 of
  those pin gameval mis-resolutions: opal/pearl/barbed
  bolts), 68 sequence names + 3 sounds + 3 varbits all
  resolving. The same applies to
  `data/definitions/npc-spawns.json` (30,828 entries),
  `npc-combat-defs.json` (963 NPC entries + 179 refs rows +
  humanoid defaults), `npc-animations.json` (3,383 id →
  sequence-array entries) and `shops.json` (487 shops,
  5,471 originalStock lines), imported verbatim 2026-11-21
  on Tyler's approval — npc-spawns.json on his "Add npc
  spawns" request, the other three on his "Also add
  npc-combat-defs npc-animations and shops" — SHA-256
  verified identical to upstream (`aa6e04af...` spawns,
  `429e4b06...` combat-defs, `be12ec7d...` animations,
  `60865b1f...` shops); `npc-animations.LICENSE` (BSD
  2-Clause, Adam/sigterm + Null, `5dac153c...`) copied
  alongside so the attribution travels with the data. No
  loader work was needed — all four loaders already
  existed (spawns, `loadNpcDefinitions`, `loadShops`); the
  repo had held `{}` starters for the three animation/shop
  files and no spawn file at all. Load-proven against the
  real rev-242 cache by a temporary test, then the test was
  deleted: spawns accepted 30,827 (candidates 30,828,
  invalid 1 — Elena, see §22, unsupported 0, failures 0);
  combat-defs spot-resolve Rat 2854 attack 2705 / block
  2706 / death 2707, Goblin 3036 6184/6183/6182, Chicken
  1174 5387/5388/5389, Imp 5007 169/170/172, Man 3108
  422/425/836; inferred-from-animations Guard 3254
  386/388/836, Dark wizard 512 711/425/836; Forester
  12119 (in neither file) the TS DEFAULT shape 422/-1/836;
  shops 487 with 5,471 stock lines, shop 0 "General Store"
  stock exactly 590/1351/1265/946/2347/1755 × 2,000,000,000.
  Live-verified in smoke39 (see §22 for the legs).

## Module inventory

| Priority | Module (upstream source) | Go location | Status |
|---|---|---|---|
| P0 | Scaffold: go.mod, LICENSE, NOTICE | `go.mod`, `go.sum`, `LICENSE`, `NOTICE` | DONE |
| P0 | Config: `WorldDefinition`, `WorldPluginConfig`, `GameConstants` (world.json, zones, env, constants) | `internal/config` | DONE |
| P0 | Cache download/validate: `CachePipeline`, `scripts/ensure-cache.ts` (OpenRS2 fetch, lock, validation) | `internal/cache` (`EnsureCache`, `ValidateCacheDir`) | DONE (live download verified 2026-11-14 — see notes; entry ships 0 XTEA keys) |
| P0 | Cache codec: `FileStore`, `Sector*`, `Container`, `ReferenceTable`, `Archive*`, `ByteBuffer`, `Xtea`, `Crc32`, compression | `internal/cache` | DONE (see notes) |
| P0 | Definition decoders: `Loc/Npc/Obj/Seq/SpotAnim/VarBit/Idk/Bas/DbRow` loaders + `CacheDefinitions`, `CacheMaps`, `Gamevals` | `internal/cache` (+ boot wiring in `cmd/server`) | DONE (decoders + facade + maps + gamevals + custom-definition/item overlays; loaded at boot and verified against the real rev-242 cache) |
| P0 | Framing: `ClientProtocol` Reader + decode/encode + opcode/length tables + `BitWriter` | `internal/protocol` | DONE (framing/tables/Reader/BitWriter; sync encoders done — see sync row) |
| P0 | Transport: `BinaryChannel` (WS), `NetworkBuilder` HTTP+WS + `ClientConnection` | `internal/session` | PARTIAL (WS + lifecycle done; WebRTC/TCP/browser-host/HTTP routes not started) |
| P0 | Login/session: login + enterWorld + bootstrap, `LoginTokens`, `PlayerSession` queue/flush, `Misc` names, `PasswordUtil` | `internal/session`, `internal/persist` | PARTIAL (login + enterWorld + bootstrap + per-tick flushClient done, including the music, ground-item, NPC-effect, object, and world-entity blocks and the templated-instance rebuild decision; plugin-owned pieces of those blocks are gaps — see notes) |
| P0 | Tick: `GameEngine`, `World.process` skeleton, `TaskManager`/`Task` | `internal/game` | DONE (skeleton + hooks; phase bodies per inventory below) |
| P0 | Startup wiring: `Server.main` order, `GameBuilder` | `cmd/server` | PARTIAL (boot + serve + shutdown + bootstrap/bridge/logout/flush-source wiring + definition loading + save wiring + JSON definition loading + overlays + spawns + plugin hub loading and lifecycle emits + chat/command/examine wiring (delivery, examine source, command registry, mute + social-packet seams) + object wiring (registry seams, sync source + delta feed, listener + hub adapters, shutdown `removePluginObjects`) + timer/area wiring (AreaManager + Wilderness install, `registerArea` adapter, isolating `callArea` caller, area music source, Wilderness drop seam) + interface wiring (session mount tracking, layout open dispatch, interface-action/prompt/close listener install, varbit resolver, audio-settings replay) + social wiring (relations world/shown-name seams, social-packet hub-emit install, long-username + relations entry load, relations snapshot fields, logout presence-offline call) + skills wiring (game skills seam from the hub, engine TaskManager handle on the hub, world XP rate from world.json, exp-drop hook install at entry) + sailing/instancing/sound wiring (world-scoped boat manager + Sailing runtime, world-entity listener deps, actor-aware collision closures, steering inside the movement request, login/logout/disconnect Sailing calls, region-replacement manager + wrapped region source + per-session replacement sender, instancing install, sound player source) done; no engine systems for content beyond the lifecycle, chat, command, object, zone/area, interface, social, skills, and sailing/instancing hooks) |
| P1 | Entities core: `Entity`, `Mobile`, `MobileList`, `Player`, `NPC`, `Location`, `UpdateFlag` | `internal/game` | PARTIAL (core fields + process skeleton + movement queue + local lists + view distance + static spawn application + live containers/skills/appearance + player attributes (lazy map on `Mobile`, `Get/Set/HasAttribute`, mutex-guarded) + forced chat + chat icons + the command registry/dispatch core + the object registry (`GameObject`/`MapObjects`/`ObjectManager`, region-load population, JSON-spawn registration) + timers (`TimerRepository`/`TimerKey`, session-only, tick-phase placement) + gameplay areas (`Area`/`AreaManager`/`Boundary`, membership + enter/exit evaluation, multi-icon follow) + `Wilderness` helpers + NPC drivers (movement coordinator, aggression/tolerance, following/facing, respawn) + combat state/targeting on `Mobile` (see the combat-core row) + shops/trading state (`ShopManager`, `Trading`, `Duelling`, walk-to-entity — see the shop/trading rows) + interface state (interface id + close gate, entered amount/syntax actions — see the interface-runtime row) + relations state (`PlayerRelations`: friend/ignore lists, ranks, chat modes, presence/PM gates — see the clans/social row) + sailing state (`SailingState`, owned save state) + private-area membership (`Mobile.PrivateArea`, clip stores, destroy-on-empty — see the sailing/instancing notes); no content systems) |
| P1 | Movement/collision: `MovementQueue`, `PathFinder`, `RsmodRouteFinding`, `RegionManager` | `internal/game` + `cmd/server` (regions adapter) | DONE (queue + pathfinder + regions + wiring + run-energy drain/restore + the `EquipmentEffects` run-energy modifier registry (phase 22) + destination-flag encoder/sync + run-toggle listener + walk-to/arrival machinery + FREEZE/STUN/ANIMATION_STALL mobility gates + NPC drivers (wander/retreat/follow) + lazy RouteFinder allocation (phase 16 OOM fix) + actor-aware area-clip threading + boat-deck projected-NPC routing + helm steering inside the movement request (phase 21), live-verified; mobility gates beyond `blockMovement`/freeze/stun/stall are documented gaps — see notes) |
| P1 | Containers/appearance/skills: `ItemContainer`/`Inventory`/`Equipment`/`Bank`, `Appearance`, `SkillManager` | `internal/game` (systems), `internal/session` (skill/appearance senders), `cmd/server` (adapters) | DONE (container core + Inventory/Equipment/Bank + live appearance + full SkillManager + live carried weight with the `equipment:weights` emit site (phase 22), save-integrated and live-verified; XP gate + world rate + level-up/experience-gained emits + `skills:*` custom events + skill slot + `setLevel` guards + skill metadata done since phase 20 (see notes); grounded refusal/overflow text wired through `sendMessage` since phase 12, ungroundable text sites stay silent gaps, SkillLevelUp feedback content, CreationMenu, bank interface flows (plugin content unlocked by the phase 18 runtime, not ported), and the equipment/NPC-combat sound picker call sites (the sound system exists since phase 21; the pickers have no call sites yet) are gaps — see notes) |
| P1 | Sync/visibility: `encodePlayerSync`, `encodeNpcSync`, `encodeInitialPlayerSync`, rebuild variants, `PacketSender` encode paths, `PlayerSession.flushClient`, visibility/update buckets | `internal/protocol` (encoders), `internal/session` (FlushTick), `internal/game` (buckets/local lists), `cmd/server` (flush source) | PARTIAL (encoders + flushClient + buckets + local lists + scene/replay state done and live-verified, including the music, ground-item, and NPC-effect blocks and forced-chat view population; chat-message encoder done (phase 12); loc encoders + per-session object sync done (phase 13: scene snapshots + per-tick deltas, live-verified; merge/animation encoders exist with no producers); multi-icon send via varbit 4605 done (phase 15); combat flush mapping done (phase 16: hitsplat + health-bar snapshots, interaction indices, NPC face tiles, live-verified via update-block frames); shop/trade encoders done (phase 17: shop open/close, trade request/open/update/close, live-verified); interface open/close + run-client-script encoders wired through the interface runtime (phase 18, live-verified); friends-chat-snapshot + clan encoders exist with no producers (phase 19: content sends them); sound/jingle/projectile/camera/world-entity/graphic encoders + senders done (phase 21, live-verified for sound/jingle/projectile/world-entity; camera senders stateless, no state restore); region-replacement framing done (phase 21: manager-built payloads, protocol framing at the sink)) |
| P1 | Music: area tracking + song sending (`Music.ts`, `PlayerSession` music block) | `internal/music` (areas/lengths), `internal/session` (state machine + flush block), `internal/protocol` (play-song encoder), `cmd/server` (data wiring) | DONE (area tracking, mode/varp behavior, keep-track rule, play-song encoder, DbTable 44 lengths verified live; area `getMusicTrack` override wired (phase 15); jingle sending + the templated-instance source tile wired (phase 21, jingles live-verified); shuffle/unlock plugin events, `noteSongSent` NOT STARTED — see notes) |
| P1 | Ground items: `ItemOnGroundManager` + `ItemOnGround` + ground-item sync | `internal/game` (manager), `internal/session` (per-session sync), `internal/protocol` (encoders), `cmd/server` (wiring) | DONE (lifecycle: owner reveal after 100 ticks, despawn after 300, respawn timers, stack merging; region-change snapshots + deltas; static spawns applied at boot; pickup/drop flows implemented and live-verified — see notes) |
| P1 | Persistence: `PlayerPersistence`, `PlayerSave`, `Accounts`, `DisplayNames`, `PlayerPunishment` | `internal/persist` | PARTIAL (shape + JSON/memory/SQLite stores + apply/from-player merge + entry/disconnect/autosave/shutdown wiring done, with containers/skills/appearance/banks owned since phase 9, attributes owned since phase 11 (live persistent-key/defaults registry, TS-order entry application, from-scratch snapshot rebuild), and `specPercentage` owned since phase 16, and friends/ignores/friend-ranks/friends-chat channel state owned since phase 19 (`fightType` owned since phase 30, `autocastSpellId` owned since phase 27), and sailing state owned since phase 21 (entry normalizes + loads, snapshots rebuild from live state, nil-means-preserve); nothing interface/varp/chat-mode persists (verified in phase 18: `PlayerSave` has no varp/varc/interface/prompt fields — varps, interface id, and prompt actions are session-only; verified in phase 19: no chat-mode/private-message-id fields either — modes are session-only, persisting in TS only through the FriendsList plugin's `chat.filters` attribute); `requestSave` coalescing, offline admin ops, snapshot list/restore, legacy JSON import, pid-file guard NOT STARTED) |
| P1 | Definitions JSON loaders + `Item/Npc/Object/ShopDefinition`, `WorldConfigStore` | `internal/definition` | DONE (all 8 core loaders + wrappers + WorldConfigStore, TS-faithful required/optional file behavior; dialogue data loading DONE (phase 14: `npc-dialogues.json`, `npc-dialogue-index.json`, `event-dialogues.json`, validated read-only against the real dir; `dialogue-types.json` has no loader — grounded gap, pet content only — and retired-NPC successors stay empty, plugin-private data); ShopManager init DONE (phase 17); interface-layout open dispatch DONE (phase 18: `OpenLayout`, all 7 surfaces, TS errors, validated read-only against the real dir — all 38 layouts open); Sailing init DONE (phase 21: types/docks registries, `initialize()` idempotence); NpcAnimationScanner DONE (phase 29: seq decoder + stall + boot preload — see notes) |
| P1 | Packet listeners: `net/packet/impl/*` (18 files) + turn-packet bridge (connection -> player turn queue) | `internal/listeners`, `internal/session` (bridge), `cmd/server` (dispatch), `internal/protocol` (decoders) | PARTIAL (bridge + WALK/MOVE_GAMECLICK decode/dispatch + movement execution + 11 listeners implemented (equip, drop, switch-slot, item actions, pickup, chat, command, object action, item-on-object paths of UseItem, NPC option incl. Talk-to dialogue dispatch + the combat attack option, interface-action dispatch incl. the shop widget branch) + 1 partial (PlayerOption: attack option 1 + trade option 2 implemented, options 3-8 decode-drop) + the dispatch-inline `varp_transmit` run toggle + the dispatch-inline examine opcodes (9/85) and inventory/ground/worn examine routes + the dispatch-inline dialogue_continue resume (prompt check before manager) + the dispatch-inline entered-amount/syntax prompt answers + the dispatch-inline interface-close case + the dispatch-inline social-packet cases (phase 19: opcodes 181/196/197/198/199 + the CHAT clan/friends-chat kinds, decode + hub emit, no core default action — exactly as TS dispatches them); all other listeners decoded and explicitly dropped — see per-listener table) |
| P2 | Combat core: `game/content/combat/*` | `internal/game` (combat/*, drivers, prayer, specials), `internal/session` (hits snapshots), `internal/listeners` (attack options), `internal/plugins` (combat families + provider registry), `cmd/server` (wiring) | DONE (phase 16, live-verified: NPC drivers + attack cycles + melee/ranged/magic methods + exact damage formulas + hit queue/hitsplat sync + death/respawn + prayer state/drain + special-energy restore + combat XP + combat-method providers + death/hit hook families; session-only except owned `specPercentage`; no drops in core (content's job), no animations, no equipment effects/poison/skulls, spell system NOT STARTED — see notes) + phase 26 first finish-combat slice, live-verified: ranged weapon/ammunition data ported (193 kinds / 253 ids, 102 weapons / 229 ids, resolver registered in production — Seeking Arrows fires in production), combat styles registry + weapon interfaces + BonusManager + varp 43/172/varbit 357 (login + death assign), onCanAttack gate firable, spec-orb varps 300/301 on restore, ranged projectile producers, projectile-flight divergence resolved, dead NPCs now leave the world at the remove point (regression-tested) + phase 27 second finish-combat slice, live-verified: the 36-spell standard book + MagicOnNpc/Player/Item listeners + autocast (58-slot map, varp 664, `autocastSpellId` owned save state) + onSpellDisabled/onSpellRuneBypass firable (48/108) + phase 28 third slice, live-verified: poison/venom (PoisonClock + effect, 258-entry table, orb varp 102, save round-trip) + skulls (WHITE/RED icons, timer, Protect Item interaction) + player-death loss/teleport (kept-items rules, bones 526, respawn at 3089,3524,0) + five death hub families firable (51/108) + phase 29 fourth and last slice, live-verified: NpcAnimationScanner ported + attack/block/death animations + spot animations wired into the combat core and spell casting (`StartCast` live; melee projectiles verified absent in TS, none invented) + ranged launch graphics populated (82 kinds with a start graphic, 10 with a double; venator 2289, dark bow double, blowpipes suppress) + sound pickers at the call sites + `player:dying` custom event (not a hook family — hub counts unchanged at 51/108, 29/95, 12/191). Combat is finished as a planned engine slice; residual gaps — see notes) |
| P2 | Dialogue framework + data loading + transcript playback + MultiChatboxPrompt | `internal/game` (manager/entries/playback/prompts), `internal/definition` (loaders), `internal/protocol` (chatbox encoders), `internal/session` (sender), `cmd/server` (wiring) | DONE (phase 14, live-verified; session-only state, no save fields; shop steps DONE in phase 17 via the ShopManager seams, menus presenter is the prompt registry — see notes) |
| P2 | Timers + gameplay areas/zones: `TimerRepository`/`TimerKey`, `Area`/`AreaManager`/`Boundary`, `Wilderness`, zone hooks | `internal/game` (timers/areas/wilderness), `internal/config` (zone boundaries), `internal/protocol` (multi-icon), `internal/session` (music override), `internal/plugins` (zone families + `registerArea`), `cmd/server` (wiring) | DONE (phase 15, live-verified; session-only state, no save fields, no data files — areas/zones are code-registered; `PrivateArea`/`TemplatedInstanceArea` instancing + the `getAreaManager` query surface DONE in phase 21 (see the sailing/instancing notes); `syncWildernessStateForMovedPlayer` is a gap — see notes) |
| P2 | Skill frameworks, `Trading`, `PrayerHandler` | `internal/game` (prayer, trading) | PARTIAL (combat-facing prayer surface DONE since phase 16: all 30 prayers, conflict groups, protection-prayer mapping in the damage pipeline, drain state/resistance; level requirements, varbit glow, and sounds NOT STARTED — interface-layer concerns; `Trading` DONE since phase 17 (requests, offer/confirm stages, transfer, gates, live-verified) + `Duelling` state + the duel-order shuffle exception (Duel Arena gameplay NOT STARTED); skill frameworks NOT STARTED) |
| P2 | Shop system: `ShopManager`, shop stock/pricing/currencies | `internal/game` (shop_*), `internal/protocol` (shop encoders), `internal/session` (sender), `cmd/server` (shop wiring) | DONE (phase 17, live-verified: definitions in TS load order with the `onCanStockItem` veto, stock/restock, rated + legacy pricing, currencies incl. content-registered ones, access rules, NPC Trade dispatch + dialogue shop steps, widget buy/sell with TS-exact refusal text, close behavior; runtime-only stock, no save fields beyond `shop:points`/`shop:quantity-mode` attributes — see notes; Grand Exchange marketplace NOT STARTED, plugin content) |
| P2 | Collision extras: `MapRegionReplacementManager`, template regions, sailing | `internal/game` (private areas, template instances, replacements, sailing, boats), `internal/session` (flush rebuild decision + replacement sender), `internal/plugins` (instancing/sailing members), `cmd/server` (wiring) | DONE (phase 21, live-verified: private areas + templated instances + template-region decoding + region replacements + sailing engine + boat sync; the sailing SKILL and Construction POH content are NOT STARTED — plugin content; residual gaps: `viewedArea` object sync, the `sceneTile` root-location term, deck-object lookup, camera state, destroy-hook splicing — see notes) |
| P2 | HTTP extras: `ContentApi`, `GatewayTarget`, `/regions` routes | `internal/session` (`gateway.go`), `cmd/server` (wiring) | PARTIAL (phase 37: `GatewayTarget` DONE — `GET /gateway/jav_config.ws` byte-exact vs TS (30 lines, `param=25` = cache revision, host from `PUBLIC_HOST` or 127.0.0.1, warn-once on invalid host) + `GET /gateway/worldlist.ws` one-world binary (members flag from the world definition, type flags 0 / activity `tsps` — the Go world definition carries no world-type source, which is the TS default), both live-verified against the real rev-242 cache incl. members/non-members and `PUBLIC_HOST`; `ContentApi` (`/api/*`) and the `/regions` routes still NOT STARTED — handler 404s them) |
| P2 | Interface runtime: `Player` interface id, `PacketSender` interface mounts, `InterfaceLayoutRegistry`, `InterfaceActionClickOpcode`, entered-amount/syntax prompts, the live varp/varbit store | `internal/game` (interface id + close gate, entered actions), `internal/session` (mount tracking + sender surface, varp store), `internal/definition` (layout open dispatch), `internal/listeners` (action dispatch, prompt answers, close case), `internal/plugins` (interface families), `cmd/server` (wiring) | DONE (phase 18, live-verified: interface open/close/layering + close-on-movement/logout/death + layout open dispatch on all 7 surfaces + action dispatch to hub hooks with prompt-first order + entered-amount/syntax prompts + varp/varbit store with change detection, varbit packing, and replay; session-only, nothing persists; `VarManager` transform plumbing, `registerCustomInterface`, bank interface flows, and the per-player vars seam are gaps — see notes) |
| P2 | Clans/social: `PlayerRelations`, social-packet dispatch, friends-chat-snapshot + clan encoders | `internal/game` (relations, names, chat gate), `internal/protocol` (social decoders/encoders), `internal/session` (social senders), `internal/listeners` (social dispatch), `internal/persist` (relations ownership), `internal/plugins` (social-packet variants), `cmd/server` (wiring) | DONE (phase 19, live-verified: whole `PlayerRelations` port + live chat gate + all 8 social-packet kinds emitted to the hub + snapshot/clan encoders + relations owned save state + logout presence-offline; friends-chat channels + the clan system are plugin content (FriendsList/Clans, NOT ported — the engine unlocks them); the legacy friend/ignore presence sends are no-ops, exactly as in TS (`PlayerSession.write` is a no-op upstream) — live presence comes from the snapshot, which content sends; chat modes session-only — see notes) |
| P3 | Plugin hub: `PluginManager`, `PluginTypes` | `internal/plugins` + `cmd/server` (`pluginwiring.go`) | DONE (compile-time registry, dependency-ordered loading, config-declaration validation, world.json switches, members-gating, lifecycle + custom-event + command/social-packet + object-interaction + dialogue + zone + combat + shop/trade + interface + skills + world-entity-interaction hooks with per-hook error isolation, gate contracts, scoped core surface, login extensions; the social-packet family fires all 8 kinds since phase 19 (same family, not a new one — the count is unchanged); onCanAttack became firable in phase 26; onSpellDisabled + onSpellRuneBypass became firable in phase 27; onShouldDropItemsOnDeath + onShouldKeepItemOnDeath + onPlayerDefeated + onPlayerDeathItemDrop + onPlayerDeathDrops became firable in phase 28 (51 of 108); the onItemAction family became firable in phase 32 (onItemAction/onItemActionNamed/onItemFirstAction/onCanUseItem, 54 of 108); the onNpcClick family became firable in phase 33 (onNpcClick, with the NPC remains-delay + transform seams in the death flow) and the onItemOnItem family became firable in phase 33 (onItemOnItem/onItemOnItemNamed, riding INVENTORY_USE_ON kind 4 — there is no separate item-on-item opcode), and the onNpcInteraction family became firable in phase 35 (onNpcInteraction, name hooks after the onNpcClick bindings — TS order, `NPCOptionPacketListener.ts:122/:132` — with the `DefinitionName`/`Action`/`Actions` fields on the hub event; `OnNpcsInteraction`/`OnAnyNpcInteraction`/`OnBeforeNpcInteraction`/`OnNpcRoute` stay table-only), 57 of 108; 51 of 108 hook families have no registration method yet — documented in the unfired-family table with the missing system named — and 29 of 95 non-hook members are ported, and 12 of 191 Core members — see notes; hub-count correction in phase 31: earlier versions of this document said 53/108, counting registration *methods* as *families* — Go has 61 `Api.On*` methods but four are named overloads of already-counted families (`OnObjectInteractionNamed`, `OnItemOnObjectNamed`, `OnItemActionNamed`, `OnItemOnItemNamed`), so 57 distinct families are firable (61 methods) and 51 are table-only; `hooks.go`'s header says fifty-seven (updated in phase 35; the phase-35 plan's "5 named overloads" was wrong — recount: exactly 4 `*Named` methods) |
| P3 | All `plugins/` content (2,907 JS files, 789,355 lines) | Go packages under `internal/content/` | PARTIAL (11 of 577 entries ported: DateMinutes (phase 10), GracefulOutfit + WornWeights (phase 22), LumbridgeGuide (phase 23), SeekingArrows (phase 24), Woodcutting (phases 32–33 — core loop + all sub-units except the Guild, which needs Tyler's `woodcutting-guild.json` import approval), Fletching (phase 34 — full skill: Shared loop, Recipes, Spool, Diaries, CreationMenu seam, all live-verified), Fishing (phase 35 — core loop + 8 of 10 sub-units, MinnowPlatform / Gita exchange / Guild door deferred, live-verified in 4 modes), Cooking (phase 36 — full skill: Cookers, Recipes, Wine, Churn, SacredEel, LeapingFish, Gnome, live-verified on fire/range/wine legs), AdminCommands (phase 38 — `::item` only, the first registered command, live-verified: admin spawn + denial path), WildernessDitch (phase 40 — `Cross` on object 23271 with the ForceMovement glide + click delay, live-verified: south/north/turn crossings + the wall leg), all live-verified; the rest per `CONTENT_MIGRATION_PLAN.md`, piece by piece with Tyler's direction; no wholesale port) |
| P3 | Bots: `plugins/bots/` + bot runtime | undecided | NOT STARTED |
| P3 | WebRTC transport | undecided | NOT STARTED |
| P3 | Browser host: `BrowserHostHttpBridge`, IndexedDB persistence | undecided | NOT STARTED |
| Never | Dead legacy: `PacketBuilder`/`Packet`/`PacketEncoder`, `IsaacRandom`, RSA constants, `util/flood/*`, generated `*Identifiers.ts` | — | NOT STARTED (do not port) |

## Notes per area

### Config (`internal/config`) — DONE

`LoadWorld` (world.json + world.local.json layering, strict
validation with TS-identical messages), zone parsing + per-64x64
f2p index, `WorldPluginConfig` normalization, `LoadEnv` (process
env only; `.env` layering unverified upstream), all game
constants. Verified against the real shipped `world.json` (308
zones). Deliberate deviations, all commented at the site:
`ParseTarget`-style strictness is in `internal/cache`;
non-boolean scalar plugin switches are dropped from `Plugins`
(TS validates them at PluginManager read time); identically
normalizing plugin names are nondeterministic in Go maps (TS: last
in file order wins).

### Cache (`internal/cache`) — DONE (codec + definitions library)

Implemented: `Target`/`ParseTarget`, `ValidateCacheDir`,
`Store`/`OpenStore`/`Read` (sector chains incl. extended archive
ids and the meta-index rule), `XTEAValidKey`/`XTEADecryptRange`,
`DecodeContainer` (None/Bzip2/Gzip, optional XTEA),
`ReferenceTable`/`DecodeReferenceTable` (protocols 5-7),
`Archive`/`DecodeArchive`, `Keys`/`LoadKeys`,
`ActiveCache`/`EnsureCache` (OpenRS2 download with lock + atomic
rename), `ByteBuffer`, CRC32, DJB2/old hashes, index/config
constants. The live OpenRS2 download end-to-end was VERIFIED in
phase 3 (temp root, since removed): selection/download/extraction/
lock/manifest all worked, and boot logged real definition counts.
One archive fact discovered there: the rev-242 entry (id 2757)
ships ZERO XTEA keys (`info.json` `valid_keys: 0`, `keys.json` is
`{}`), so `LoadKeys` yields an empty map and rebuild packets carry
all-zero keys — the same result the TS server gets from the same
endpoint. Map data still decodes with zero keys (spawn region
verified: 48,345 terrain + 3,511 object bytes). Real-key handling
is unit-tested with synthetic keys only; a future cache with keys
flows through the same code.

Definitions (phase 2): binary decoders for `Obj`/`Npc`/`Loc`/
`Seq`/`SpotAnim`/`VarBit`/`Idk`/`Bas`/`DbRow` (+ `VarcInt`, inline
struct/enum decoders), generic `TypeLoader` machinery
(archive/index/dat/indexed-dat/dummy loaders, LRU),
`CacheDefinitions` facade (lazy loading over index 2,
`ArchivedDefinitionRegistry` overlay + `PostProcessedObjTypeLoader`,
name/DB/struct/enum/spell lookups), `CacheMaps` (region archives
`mX_Y`/`lX_Y`, XTEA keys), `Gamevals` (index 24), and a
`CacheIndex` facade over the real `Store`
(`NewCacheDefinitionsFromStore`, `NewCacheMapsFromStore`,
`NewGamevalsFromStore`). Verified against hand-built fixtures, a
synthetic store, AND the real rev-242 cache (phase 3 live check:
counts, Coins/stackability, noted items, named NPCs, gamevals
names, spawn-region map data — see "What was verified").
Boot-time loading (phase 3, `cmd/server/definitions.go`): with a
cache present, `buildServer` opens the store, reloads XTEA keys,
constructs all three facades before anything reads definitions,
and logs the counts; any failure fails the boot (a present but
unreadable cache must not silently serve default definitions).
In NoCache mode the facades stay nil and the server still boots
and answers logins (weight 0, zero XTEA keys).
Custom-definition overlays (phase 6, `custom.go`):
`RegisterCustomDefinitions`/`RegisterCustomItems` port
`CacheDefinitions.registerCustomDefinitions`/
`registerCustomItems` (`CacheDefinitions.ts:597,703`), reading
`custom-definitions.json` (`{version, rows}`) and
`custom-items.json` (array of `{id, baseItemId, objType}`) from
the definitions directory; missing files mean zero rows (the
editor's `Document.read` / plugin `existsSync` behavior).
Precedence, verified in the TS getters: custom items > custom
definitions > archived > native. Identity/validation rules
(id ranges, forbidden keys, model-id uniqueness, 5-entry
actions, `specialAttack` shape) are ported; field application
is stricter than TS `Object.assign` (unknown/mistyped fields
are registration errors, matching the editor's save-time
audit). Only custom-definition ids extend loader counts (TS
`getCounts`); custom items do not. Documented deviations:
`GetBaseCounts` shares loader counts (so it reflects customs,
unlike TS), `HasDefinition`/`HasItem` do not consult customs,
custom models are validated but never read/sent (no model
pipeline), and there is no `encodeContentData`/transport
budget (approximated by a JSON-encodability check).
DONE (phase 29): `NpcAnimationScanner` — the seq decoder
(`SequenceScanner`: priority / forced priority / stall
ticks = ceil(clientCycles/30), boot preload via
`InstallSequenceAnimations`), verified against the real
rev-242 cache (last seq 15173; 422 punch forced 6 /
stall 2; 424 block forced 5 / stall 3; 836 death forced
10 / stall 669, capped to 5 by `OnAnimationStarted`;
1658 whip forced 6 / stall 2). `findNpcRigAnimations` is
NOT ported as runtime (tooling-only; its only caller is
the AdminCommands rig-scan command). The animation
producers it feeds (attack/block/death anims + spot
animations at the combat call sites) are DONE in phase 29
too — see the combat row.
One latent TS bug was corrected:
`ArchiveFileReference.ArchiveID` is the archive's id (TS passes
the file id). Hardening where TS silently misbehaves (truncated
inputs, negative sizes, zip-slip) returns errors instead of
garbage; see code comments.

### Definitions JSON (`internal/definition`) — DONE (loaders + wrappers)

All 8 core loaders from `game/definition/loader/` are ported,
in `GameBuilder.loadStartupData()` order: NpcInteraction,
InterfaceLayout, ObjectSpawn, GroundItemSpawn, NpcDefinition,
NpcSpawn, Shop, over the `DefinitionLoader` source-registry
semantics (owner/name/priority, priority-then-name ordering,
per-source failure counting). Wrappers (`ItemDefinition`,
`NpcDefinition`, `ObjectDefinition`, `ShopDefinition`, spawn/
interaction/layout definitions, equipment/NPC-combat sounds)
layer JSON/gameplay data over cache types, nil-safe: with no
cache the JSON still loads, cache-backed filters are skipped,
and wrappers return TS DEFAULT shapes (every degradation site
is commented — TS has no no-cache path). `WorldConfigStore`
(planning, merging, validation, plugin-switch resolution) is
ported as TS structures it. Required-vs-optional file behavior
is TS-faithful, verified per loader: REQUIRED (missing or
malformed fails boot, direct `readFileSync` in TS) —
`interface-layouts.json`, `object-spawns.json`,
`ground-items.json`, `npc-combat-defs.json`,
`npc-animations.json`, `shops.json`, `custom-shops.json`;
TOLERATED when missing — `npc-spawns.json` (a plugin-registered
source in TS, loaded through failure-counting `loadSources`;
the Go port registers it as a built-in source),
`monsters-complete.json`, `npc-status-immunities.json`,
`npc-captured-animations.json` (TS `readOptional`, warn +
empty); item/examine/sound files are optional-when-missing in
Go as a documented deviation (TS requires `item-gameplay.json`
and reads examines/sounds lazily, throwing if missing) so
fixture boots work — malformed-when-present still fails. The
shipped `data/definitions/` carries minimal valid contents for
the required files. `NpcSpawnDefinition` merge semantics
(priority-order sources, one-for-one replacement, remove
markers with freed-slot credits), direction tables
(client/spawn numbering), and the default wander radius (5)
are ported; `key`-based spawns are skipped with a warning
because the generated `NpcIdentifiers` table was never
ported — that is a real 1-entry gap, not TS behavior: TS
resolves the file's only keyed entry (Elena, `key: "ELENA"`
→ id 1102) and spawns her, Go drops her (see §22).
Dialogue data loading is DONE (phase
14, `dialogue.go`): `npc-dialogues.json` is REQUIRED
(missing/malformed fails boot, as the TS loader's throw
fails the NpcDialogues registration), `event-dialogues.json`
missing reads as `{}`, and any index failure yields an empty
index — with order-preserving decode, `PickVariant`
(explicit-activation / flat-steps / named-default /
`standard*` / first-non-`overhead*`), aliases, and the
speaker-id index, validated read-only against the real
definitions dir (3,374 records, 250 aliases, 7,438 index
ids / 12,450 entries, 3,293 speaker ids, 20 event names,
18,315 variant keys, 74,970 top-level steps, zero parse
failures; nothing copied). The repo holds only tiny
hand-written starter files. `dialogue-types.json` has NO
loader — a grounded gap: its only consumer is pet content.
The retired-NPC successor map stays empty: `retired-npcs.json`
is plugin-private data (`plugins/npcs/data/`), never copied,
and no approved read path exists (12 successors upstream).
NOT STARTED:
plugin vetoes (never veto);
`applyCustomCombat` (no-op); `ScopedPluginSettings`; runtime
world state (`WORLD_SPAWN`, zone boundaries, `isMembersArea`).
(`NpcAnimationScanner` / `preloadSequences` /
`preloadEnums` are DONE since phase 29, and
`CombatPoisonData` since phase 28.)
Deviations, all commented at the site: registry tie-break uses
Go string `<` (TS `localeCompare`, differs only for non-ASCII
names); `WorldConfigStore` writes sort JSON keys (Go encoder)
vs TS insertion order; invalid equipment-sound regexes warn
at load and never match (TS throws on first use); weapon
interface names are unvalidated strings (combat style tables
not ported); Barrows variants out of cache range are skipped
(TS would mutate DEFAULT).

### Protocol (`internal/protocol`) — framing + sync encoders DONE

`Reader` (all 15 transform methods, hand-vector tested),
`DecodeFrame`/`EncodePacket` (exact-length assertion,
unknown-opcode/truncation errors), full opcode + length tables
(44 high-level / 63 native / 106 server, verified against the TS
table files), `BitWriter` (MSB-first), `EncodeWelcome`,
`EncodeTick`, `EncodeLoginResponse` (all three strings always
present, as in TS), `EncodeRebuildNormal`/`EncodeRebuildRegion`
(byte-exact, including the inverted reload byte and the region
palette layout), `DecodeLogin`/`DecodeHandshake` (appearance-block
skip grounded)/`DecodePing`/`DecodeWalk`. Findings: `NativeClientPackets`
says APPEARANCE_SET is "len=13" in a comment but the table and
decode branch use 20 — 20 is implemented. `DecodeWalk` drops the
flags byte per its contract signature; `DecodeWalkFull` and
`DecodeMoveGameclick` (phase 2) return the full fields.
Listener decoders (phase 6, `decode_listeners.go`): 25 decoders
covering all 58 recognized listener opcodes, mirroring every
`decodeClientPacket` branch's Reader calls in order (transforms,
read orders, `unsignedItemId`, `readChatStyle` quirks,
`IF_TRIGGEROPLOCAL` block-length validation). Phase 7 added
`DecodeVarpTransmit` (opcode 191, `varpId: short` + `value: int`,
length 6) for the dispatch-inline run-toggle case, which is not
a listener file. Phase 12 added `DecodeExamineNpc` (opcode 9,
shortAdd) and `DecodeExamineLoc` (opcode 85, shortAddLE) for
the dispatch-inline examine cases, bringing the recognized
listener opcodes to 60. No opcode constants were added — every needed
constant already existed in the tables. Traps preserved, not "fixed": opcode 57 decodes as
NPC option 5 and 76 as option 1 (the `ClientPacket` enum is
what the TS switch uses); opcode 50 has a length-table entry
but NO decode branch in TS, so it is deliberately unrecognized;
opcode 94 decodes a ground-item target (Telekinetic Grab), not
a loc.

Sync encoders (phase 2, byte-exact tested incl. bit-level sync
vectors): `EncodePlayerSync`, `EncodeNpcSync` (the real TS name
is `encodeNpcSync`, `ClientProtocol.ts:3611` — earlier status
text saying `encodeNpcInfo` was wrong; `NPC_INFO` is only the
opcode name), `EncodeInitialPlayerSync`, `EncodePlayerAppearance`
(raw block, not a framed packet), `EncodeHandshake`,
`EncodeDefaultAnimations`, `EncodeGameframeBootstrap`,
`EncodeLogoutResponse`, varp/varbit, inventory/bank, skills,
run-energy, and the full widget encoder family. `EncodeVarpBatch`
does not exist: no such encoder exists in TS. Phase 3 added
`EncodeHideNpcOps`/`EncodeHideObjOps` (opcodes 148/149, 1 byte, no
length prefix) for the initial-sync packets flushClient sends.
Phase 7 added `EncodeDestination` (opcode 87, 4-byte payload,
x/y UInt16BE masked to 16 bits, byte-exact per
`ClientProtocol.ts:2214-2218`), sent through the typed
`SessionPort.SendDestination` (the game layer never imports
protocol; `*Session` implements it in `runstatus.go`).
Phase 8 added `EncodePlaySong` (opcode 133, 10-byte payload:
track u16, duration int32, `SongFades` bytes, byte-exact per
`ClientProtocol.ts:2802-2810`) and `EncodeGroundItems`/
`EncodeGroundItemsDelta` (opcodes 54/55, 34-byte stacks:
id int32, itemId u16, quantity int32, x/y u16, level byte,
byte-exact per `ClientProtocol.ts:1490-1530`, over the
`GroundItemView` shape at `ClientProtocol.ts:260-275`).
Phase 12 added `EncodeChatMessage` (opcode 120, var-short,
byte-exact per `ClientProtocol.ts:1185-1217`): 8 message
kinds with default type bytes, 12 `ChatType` channels,
the legacy `@xxx@` colour rewrite for game/server kinds
only, and caller-side highlight resolution. Phase 21 added `EncodeSound` (opcode 131, 8/13 bytes),
`EncodePlayJingle` (opcode 132, 5 bytes, the delay in
shuffled 3-byte IME order), `EncodeProjectiles` (opcode 84,
29 bytes/entry), all 11 camera encoders (opcode 137, leading
discriminator), `EncodeRebuildWorldEntity` (opcode 142) +
`EncodeWorldEntityInfo` (opcode 143), `EncodeTileSpotAnim`
(opcode 83) and `EncodeNpcSpotAnimationSpecific` (opcode
25), all byte-exact per `ClientProtocol.ts`, plus the
world-entity decoders (opcodes 220–225 + 214, dispatch-
inline, emit-only — TS has no listener file for them).
Region-replacement framing stays split by the
game-never-imports-protocol rule: the manager builds
payloads, the session sink frames them (opcodes 144/147).
NOT STARTED (absent, not stubbed): interaction-mode
encoders, the `Graphic`-class encoder family (the two
spot-animation encoders above are ported; the class itself
is not). The `NpcAnimationScanner`-driven animation
producers are DONE (phase 29: attack/block/death
animations + spot animations wired into the sync flush).

### Session/login (`internal/session` + `internal/persist`) — PARTIAL

Ported and tested: `BinaryChannel` interface, WebSocket channel
(no compression, 64 KiB read limit, loopback-only `x-real-ip`) and
headless channel; session queue/flush (64 KiB split, single-frame
concat); connection lifecycle (serialized input, immediate vs
turn packets, 240-packet cap -> 1008, malformed -> 1002, idle 30 s
after first ping, pending-login 5 min -> 1001); login check order
with literal response codes (6/3/4/27/2/5/8, verified verbatim —
`LoginResponses.ts` constants are legacy and deliberately not
used); pending-name reservation; credential fallback
(main -> profile -> other profiles); scrypt passwords (Node
golden vectors verified: N=16384, r=8, p=1, salt passed as hex
string — not verified against a real production hash); SHA-256
token rotation/revocation (30-day, max 5); ban re-check on
enterWorld; `Misc` name functions. Verified against TS source
that `login()` only reads the accounts registry (never creates
entries), matching this port. Per-tick flush (phase 3,
`flush.go`): `FlushTick` ports `flushClient`
(`PlayerSession.ts:355-660`) reduced to ported snapshot state —
hide-ops packets, scene-base init/recentering, scene-change
detection + normal rebuilds with XTEA keys, initial player sync,
player/NPC sync views, tick/player/NPC frame order, scene-replay
bookkeeping, and the appearance cache — reading each tick's view
through the `FlushSource` interface the `cmd/server` adapter
implements. Phase 8 ported the three blocks that were DEFERRED
in phase 3 — music, ground-item region handling, and
NPC-specific effects — described in the paragraphs below and
in the Game notes. Phase 13 ported the object region-change
handling (scene snapshots at both `onRegionChange` sites +
per-tick delta drain, described in the Objects paragraph of
the Game notes). Phase 21 ported the world-entity block (per-viewer sync
state, max 8 boats / 4 new per tick, between the tick frame
and player sync), the templated-instance rebuild decision
(templated -> region rebuild with the scene palette + XTEA
keys, forceReload; plain private area -> no rebuild; the
replay bookkeeping distinguishes them), and the region-
replacement sends (visible replacements on window change).
Also omitted with callouts: the plugin sync events, the
`viewedArea` term in object sync (area objects sync as
main-world objects; template objects are not synced — see
the Game notes), and the `sceneTile` root-location term
(the music tile IS root-location aware since phase 21; the
scene tile is not). One TS-faithful
subtlety, pinned by a test: a pure plane change replays scene
bookkeeping but sends NO rebuild (`sceneRebuildFor` receives the
change flag without the level term). With no `FlushSource`
attached, `FlushTick` degrades to the queue-only flush. NOT
STARTED: character select/create/delete and `login_forget` are
shape-validated then dropped (accounts-plugin owned);
plugin-message/movement-input packets are dropped; bot control,
stalled input, WebRTC, TCP listener, browser-host bridge,
ContentApi/`/regions` are absent (`GatewayTarget` DONE in
phase 37 — see the HTTP extras row). `shownByAnother`
(live-player display-name check) is not implemented. The TS
microtask auto-flush and WS backpressure guards are not
reproduced; `WebSocketChannel.Send` uses a 10 s write deadline
(operational Go necessity, flagged in code).

Music (phase 8, `internal/music` + `session/music.go`): the
`region_updates` music logic (`PlayerSession.ts:375-452`) runs
every flush: the tile's area comes from the RLE area grids
(`tracksAt`/`AreaAt`, 64x64 tiles per region, plane 0 — the
grids are per-plane maps and the port reads the plane-0 map,
as TS does for the area lookup), the mode from varp 18
(0 Area, 1 Shuffle, 2 Single) and the settings varp 19
(loop bit 0, area-type shift 10 for classic/modern), with
the missing-varp defaults from `PlayerSession.ts:391-392`;
a track already playing that belongs to the new area keeps
playing when both the old and new modes are Area
(keep-track rule); otherwise `pickAreaTrack` picks a random
track other than the ended one (in Area mode), falling back
to replaying it when loop is on and it is the only choice.
A finished track sends the silence track (147) and varp
3883 = -1, with the next track 2 ticks later.
Song lengths come from cache DbTable 44 (`SetLengths`: id in
column 4, name in column 1 or 0, length in ticks in column
3, per `Music.cachedTracks`), verified live against the real
cache; with no lengths (NoCache) a started track never ends,
which is the TS behavior when the table is absent. When a
track ends, the TS fallback runs: replay the ended track if
loop is on, else nothing — shuffle depends on the plugin
event `music:shuffle-track` and unlock on
`music:unlock-track`, and no ported content emits either
event yet, so shuffle and unlock are explicit gaps (the
events are never emitted, the queue never advances past
the fallback). Jingles ARE wired (phase 21: `sendJingle`
sends the current track with zero fades first, then the
jingle; no track-first frame when no track is playing);
`noteSongSent` and the
`music:shuffle-track`/`music:unlock-track` handlers are NOT
STARTED. The templated-instance source tile IS wired
(phase 21): in a templated instance the music block asks
the instance for the source tile (chunk-rotation undo),
falling back to the current tile, which keeps the previous
area exactly as the TS NO_TRACKS branch does; outside
instances the music tile is the boat root location (the
entity's own tile when not aboard). The area `getMusicTrack` override IS wired (phase
15): every flush, the music block asks the player's current
gameplay area for a track through the coordinator-installed
source (`CallAreaMusicTrack`, via the hub's isolating caller
in production), and a valid track (integer, 0 <= t < 0xffff)
replaces the music-area track list; an area with no opinion,
an invalid track, or a throwing `getMusicTrack` falls back
to the music-area behavior. Player audio settings (mode, volume, loop, shuffle)
are read from the save (`PlayerSave.ts:650/758`) and carried
through the merge untouched; the `varp_transmit` audio-setting
cases still decode-drop. Verified live: area-30 track at
spawn, track 98 on entering area 188, track 141 on entering
area 55 (see "What was verified").

NPC-specific effects (phase 8, `session/effects.go`): the
effect queue (`NpcSpecificEffects.ts`, ~15 lines) plus the
`FlushTick` take/send checks — effects are taken at the top
of the flush and sent only for NPCs visible and encoded this
tick (visible in the local list AND present in this tick's
`npcSnapshot`). The TS `isRegistered()` check degenerates in
Go: a deregistered NPC cannot appear in local lists or the
snapshot, so no snapshot field was needed (documented at the
site; the only residual gap is a mid-pass panic in
`UpdateLocalNPCs`, which the phase-13 failure isolation turns
into an error + logout). Honest limit: the only TS producers
are `sendNpcHeadIconSpecific` /
`sendNpcAnimationSpecific` / `sendNpcSpotAnimationSpecific`,
all content/plugin-driven — this is plumbing with no visible
effect until a producer exists, and none was invented.
Verified through real `FlushTick` at the session level
(`TestFlushTickPhase8Blocks`): effect delivered for a
visible+encoded NPC, suppressed for a non-visible or
non-encoded one, never repeated.

Bootstrap (phase 2): `Bootstrap()` sends the full
`sendPlayerBootstrap` sequence at the end of `enterWorld`
(handshake, default animations, gameframe bootstrap, spell-unlock
varbits/varps + varbit 4070 + tab-6 mount, interface script 828,
inventory 3214, equipment 94 for native clients only, skills
snapshot, run energy, quick-prayer varbits), deriving every value
from the `PlayerSave` when present and TS new-character defaults
otherwise (all defaults cited in `bootstrap.go`). The bootstrap
also stores the session's own encoded appearance block
(`SetSyncAppearance`, from the save's look/skills with default
animations) so other players' sync views can read it; the
adapter marks snapshots appearance-dirty while no block is
stored, covering the tick that can interleave between world entry
and bootstrap completion. Run-energy weight is now REAL (phase 3):
`carriedWeightKg` sums per-slot definition weights over the
save's equipment + inventory via the optional
`LoginDeps.ItemWeights` source (stackable/noted items weigh 0,
each occupied slot counts once regardless of amount, kg =
trunc(grams/1000) per `Player.getCarriedWeightGrams`/
`getWeightKg`); without definitions (NoCache) it sends 0, and the
`equipment:weights` plugin overrides are omitted (no ported
content emits the event yet). Remaining bootstrap
approximations, all documented at the site: data orbs are
always shown
(the off-setting is restored by the Settings plugin in TS); the
appearance kit sanitize uses the TS no-cache fallback (clamp,
no ident-kit cache check); the `player:bootstrap-complete`
plugin event is not emitted (no emit site: the bootstrap is
sent by the session layer, which has no hub reference); the logout
walk-interaction-mode reset has no encoder and is omitted.
The turn-packet bridge is implemented: `WorldPort`
`AddPlayer` returns a `TurnSink` bound to the admitted player and
`Dispatch` hands entered non-immediate packets to it
(`Player.QueueTurnPacket`, drained by `Player.Process` on the
tick); with no sink, `TurnPackets()` draining still works and
packets are never double-delivered. The wired `TurnHandler`
(phase 6) dispatches WALK/MOVE_GAMECLICK to movement and every
other turn opcode to `listeners.Handle`, which decodes and
either acts or drops with a named reason (see the per-listener
table below); the movement-hook nil guard covers only the
movement cases, so listener dispatch runs for session-less
players too.

### Game (`internal/game`) — skeleton + movement + containers/skills/appearance + NPC drivers + combat + shops/trading DONE, content NOT STARTED

Ported and tested: `Engine` (monotonic drift-corrected schedule,
no-overlap skip, panic -> `onTickError`); `World.Process` with
all 15 phases in TS order, each failure-isolated, with hooks for
unported phases; queue drains (cap 150, backwards remove with
combat-hold semantics); `MobileList` (lowest-free-first, slots
1..capacity); `Task`/`TaskManager` (delay semantics verified: 0/1
= every tick, 2 = every other tick; no Repeat field in TS);
`Player`/`NPC`/`Mobile`/`Entity` cores, `Location`, `Direction`,
`Flags`. Thread-safety (coordinator addition): world queues and
the entity lists are safe for concurrent use from network
goroutines; `World.cycle` is atomic; everything else is
tick-goroutine-only. Corrections grounded in source:
`Location.RegionX()` is `(x>>3)-6`, not `x>>6`; TS `Location`
move/translate drop `z` and were not
ported. Resolved 2026-11-13 (checked against upstream
`Location.ts` at commit `bfd81e3`): it is an upstream bug, not
intended behavior — `move` computes `z` then discards it,
`translate` ignores its `z` parameter entirely, and `getTranslate`
passes 0. No caller in `MovementQueue`, `RegionManager`,
`PathFinder`, or `Mobile` uses translate/getTranslate or relies
on a level reset; movement/collision instead use the
z-preserving `transform()` and reject destinations whose `z`
differs. Decision for the movement phase: preserve `z` in the
Go port (do not reproduce the drop). Caveat: a repo-wide caller
search was not possible (GitHub code search needs sign-in), so
an unchecked caller under `server/plugins/` cannot be ruled
out.

Attributes (phase 11, `attributes.go`): the lazy attribute
map lives on `Mobile`, as in TS (`Mobile.ts:102-110`),
behind a pointer to a mutex-guarded store (login emits run
on the login goroutine, tick work on the tick goroutine).
`GetAttribute` returns nil when absent, `SetAttribute`
stores by reference with no cloning (cloning happens only
at save/load boundaries, in `internal/persist`), a nil
value deletes the key (the Go form of TS
`setAttribute(key, undefined)`, which `fromPlayer` skips),
and `HasAttribute` is a Go-only presence helper (TS
distinguishes via `undefined`). No enumeration and no
remove method exist, matching TS (verified by grep: zero
enumeration/remove sites in TS core or plugins; 346
explicit-null stores exist in TS/plugins but nothing relies
on a stored null being present — loose `== null` checks
treat deleted exactly like null-valued). Appearance
pronouns/skin unlocks, the SkillManager XP lock, bank
placeholders, and switch-slot insert mode read the live
store with TS-identical defaults; `Running` stays a field
and music/ground-item/dateminutes state stays
session/manager/content-local, each verified against TS.
`Player.resetAttributes` does not clear the map (it resets
combat state and sets one attribute) and nothing is wired
to it.

Visibility (phase 3, `visibility.go`): update buckets
(key `z:(x>>3):(y>>3)`, nearest-first ring collection),
`NearbyPlayersForUpdate`/`NearbyNPCsForUpdate`,
`UpdateLocalPlayers`/`UpdateLocalNPCs` (compaction/splice
semantics, 255 caps, 25 new players/cycle), per-player view
distance with shrink/regrow (`ResizeViewDistance`), and the
phase-13 wiring with per-player failure isolation (error +
logout, mirroring the TS try/catch); the `VisibilityBuckets`/
`UpdatePlayersNPCs` hooks remain as overrides. Degenerations,
all commented at the site: CrossFloor has no policy (same-plane
+ Chebyshev only), boats/areas degenerate (root location = own
tile, all areas nil), region gating is absent (every registered
visible NPC is bucketed), `staysInViewWhenPlaced` is always
false, and the pet special-case / owner-only checks are omitted
(no pets/ownership). Local lists are tick-goroutine-only and
must be re-fetched every tick.

Movement/collision (phase 4): `RegionManager` + `Region`
(`regions.go`, `regions_decode.go`) load regions around players
from the cache maps through the `RegionDataSource` seam
(`collision.go`), decode terrain (pre/post-rev-209 formats) and
object placements, and build per-plane clipping grids with
object clipping from Loc definitions. `CanMove`/
`canStepDiagonally` and the directional `Blocked*` helpers port
the TS clipping semantics (flag constants cited to the TS
collision code); missing regions answer 0 (open ground —
upstream never writes `UNLOADED_TILE` into a clip grid,
verified by grep). `RsmodRouteFinding` (`routefinding.go`) is a
full port of the TS route finder (128x128 search map, BFS with
direction bitsets, entity size, moveNear closest-approach,
alternative routes); `PathFinder.WalkRoute` wraps it for
walking. `MovementQueue` (`movement_queue.go`) ports the
queue/stepping core: enqueue a route (with the route-result
handling), step per tick in `Player.Process` (after timers, in
the TS turn order), walk/run stepping, queue clearing,
teleport-reset, facing updates as the queue produces them, and
the `blockMovement` mobility gate. `Player.OnMovementRequest`
is wired (via the existing TurnHandler) so a decoded
WALK/MOVE_GAMECLICK request pathfinds and enqueues; position
changes flow through the per-tick sync machinery, so a client
sees the player move. `Location.Transform` is the z-preserving
helper movement uses; destinations whose `z` differs are
rejected (see the resolved `z` decision above). Verified live
(see "What was verified"): exact arrival, ditch refusal, no
blocked tile occupied, <=2 tiles/tick.

Run energy (phase 7, `runenergy.go`, the queue's
`drainRunEnergy`, the restore branch in `Player.Process`): units
0..10,000, 100 = 1% (`GameConstants.ts:30`); drain per running
tick `floor((60 + floor(67*weight/64)) * (1 - agility/300))`
(weight clamped 0..64, `MovementQueue.ts:803-808`); restore per
tick not spent running `floor(agility/10) + 15`
(`MovementQueue.ts:811-815`); drain only when run-toggled and
`stepsMoved >= 2`, setting `ranThisCycle`; at <= 0 the queue
clamps to 0, clears both run overrides, `setRunning(false)`,
and sends run status AND run energy (the TS double send,
`MovementQueue.ts:817-833`); restore runs after movement
(`Player.ts:559-566`); bots are pinned to max. Since phase 9
both inputs are LIVE: Agility reads the live SkillManager
(current level, sanitized per `SkillManager.ts:425-433`) and
weight reads the live containers (see the containers paragraph
below) — the phase 7/8 entry-time static weight and static
Agility accessors are gone. Since phase 22 the
`EquipmentEffects` drain/restore modifier registries exist
(world-scoped, `equipment_effects.go`, installed on players
at add; the apply step keeps the previous value on a
non-finite/negative modifier result, per
`EquipmentEffects.ts:303-308`): the Phase 7 formulas are
unchanged, and hub-registered modifiers run through them —
`GracefulOutfit` registers the restore modifier; the drain
registry has no registrants yet, and the other
`EquipmentEffects` modifier families (hit/prayer/special/
status) are NOT STARTED. Stamina is always 0 ticks.
Energy accessors follow `Player.ts:1257-1277` exactly,
including the `+1e-6` hair in `setRunEnergy`. Verified live:
depletion, auto-stop, restore (see "What was verified").

Destination flag (phase 7, `destination.go`): `EncodeDestination`
wired through `syncDestinationFlagToRoute`
(`MovementQueue.ts:446-461`) at the route-set, queue-emptied,
and reset-with-clearDestination sites: last queued point or
(0, 0), deduped against the last sent destination, non-players
and bots excluded. Documented deviation: TS excludes only
*unwatched* bots (`Player.ts:1538-1540`); Go excludes all bots
(no has-network-client distinction exists). Verified live: the
walk target, then (0, 0) on arrival.

Containers, appearance, skills (phase 9, `item.go`,
`container.go`, `inventory.go`, `equipment.go`, `bank.go`,
`weight.go`, `skills.go`, `appearance.go`, `itemdef.go`):
`Item` (id/amount/meta, `IsValid` = id>=0 && amount>0, meta
kept byte-for-byte as `json.RawMessage` maps with the 6-key
migration, canonical-JSON equality) and the unexported
`itemContainer` base (full TS method set: add/remove/swap/
move/stack/capacity, value semantics — `Items()` returns deep
copies, read-modify-write via `ItemAt`/`SetItem`),
embedded by distinct `Inventory` (28 slots, interface 3214),
`Equipment` (14 slots, gaps at 6/8/11, refresh flags
appearance), and `Bank` (352 slots growing to 1410, 11 tabs,
search tab 10, placeholders, `Rearrange`). `ShiftSwap` is
ported exactly as the upstream stub (validates, never swaps).
Definition lookups go through the `ItemDefinitionSource`
interface (stackable/noted/weight/equipment slot/requirements/
tradeable/sellable/dropable/un-note/placeholder), nil-safe
with documented no-definition defaults — a nil source answers
false for tradeable/sellable/dropable, matching the TS
DEFAULT definition, so in NoCache mode drops refuse as
undroppable. `EquipmentRequirements` ports the TS structure
(`RequirementLevels` = `MaxLevel` + `CombatLevel`, 24 hardcoded
skill names); requirements depending on unported systems are
gaps. `Appearance` ports the 13-slot look, male/female
defaults, skin unlocks, pronouns with gender fallback, and
head hints; `SyncAnimations` returns the weapon's
stand/walk/run overrides through the optional
`ItemAnimationSource` seam (defaults 808/819/824 when absent).
`SkillManager` ports the EXP_ARRAY verbatim (level 99 =
13,034,431, level 100 = 14,391,160, 200M cap, the
negative-XP->99 fall-through quirk), combat/total level
formulas, drains/restores/boosts, level-up behavior (current
raised by levels gained, `Flag.APPEARANCE`, hook fires once),
and the `CurrentLevel` negative->write-0-return-0 sanitize;
`SetSkills` normalization mirrors TS exactly, including the
HP 1154 floor vs the constructor 1184. Carried weight
(`weight.go`) ports `Player.getCarriedWeightGrams`/
`getWeightKg` exactly (equipment then inventory,
stackable/noted weigh 0, one count per occupied slot, kg
truncates toward zero), including — since phase 22 — the
`equipment:weights` emit before computing: a finite
override for an occupied slot replaces that slot's
definition weight with `int(override * 1000)` grams, even
for stackable/noted items (`Player.ts:1284-1311`).
GracefulOutfit and WornWeights answer the emit; the login
bootstrap frame still computes weight from the save
without the emit (see gap 12). Refreshes and skill updates reach the
client through coordinator-installed hooks (nil-safe no-ops
without a coordinator), never new `SessionPort` methods;
`OnEquipmentChanged` rebuilds the appearance block via the
session builder and stores it with `SetSyncAppearance`, so
equipment changes appear in the next sync frame. Walk-to
machinery (phase 9, `movement_queue.go`, additive only) ports
the minimal faithful subset of `MovementQueue.walkToTile`/
`walkToGroundItem`/`reachedGroundItem` + the `MovementTask`
closure: re-route throttle, fail-when-unreachable, resolution
on the cycle after arrival, exact-tile immediate action,
`ReachedObj` orthogonal adjacency only, and `RequestWalk`
cancelling a pending walk-to. Gaps, all commented at the
site: ungroundable chat messages on full/overflow/refusal
(the 12 TS-grounded text sites were wired through
`sendMessage` in phase 12; the rest stay silent with
comments because TS grounds no exact text or call site),
`forceAdd`, `getTotalValue`, bank interface flows /
`switchsItem` / `bank:*` events, `equipment:changed`,
`BonusManager.update`, weapon interface updates,
`equipment:weights` overrides, level-up messages/graphics
(SkillLevelUp plugin content, NOT STARTED),
skill/render/transformation animation overrides, displayed
equipment, skull overrides, `walkToEntity` (walk-to-object
was ported in phase 13, with the route event + reach
rectangle + repath rules; `walkToEntity` still NOT
STARTED), and the destroy-item widget flow. Bank loading has no
`withoutLimit`: a save holding more than the player's space
loads only what fits (TS loads whole).

Skills shared handlers + XP sources (phase 20, `skill_hooks.go`,
`skillmeta.go`, `skills.go`, `internal/plugins/skills.go`,
`internal/plugins/skillactions`, `cmd/server/skillswiring.go`):
`AddExperience` consults the `onCanGainExperience` gate first
(deny returns before any send, per TS order) and applies
`rate = worldRate * gateMultiplier` when multipliers apply,
with the world rate set once at boot from world.json
`experienceMultiplier` (default 1, invalid values ignored);
the level-up and experience-gained emits fire through the
game-side `SkillsHooks` seam with the two different
experience values TS sends (landed post-cap delta vs
post-rate tenths amount); the `skills:experience-gained`,
`skills:current-level-changed`, and `skills:restore` custom
events fire through `Player.EmitCustomEvent`, with the
restore read-back rule applied. Payload convention, mixed
by necessity: the experience-gained and current-level-
changed payloads are emitted as VALUES (the
interface-state precedent), while the restore payload is
emitted as a POINTER (`*SkillsRestorePayload`) so handler
mutation is visible to the read-back — handlers for
`skills:restore` must type-assert the pointer form. The
skill slot (`GetSkill`/`SetSkill`) plus `StartSkillable`/
`StopSkillable` are ported; `walkToReset` and the Drop/Equip
listeners call `StopSkillable` and are live now that the
slot exists (their comments were updated at phase 20
integration);
`setLevel`'s Wilderness + equipment-requirements guards run
with TS refusal text plus `DeactivatePrayers`, but the
rights bypasses are not applied (no live rights field) and
vengeance / `BonusManager` / `WeaponInterfaceManager` stay
gaps; skill metadata (names, tab buttons, chatbox
interfaces, client ids) is ported for future content.
`pressedSkill` is DEAD CODE in TS (zero callers in the
whole tree) and is deliberately NOT ported. The
`SkillActions` helper ports `SkillActions.js` exactly
(`pauseForLevelUp` with identity-checked resume/cancel,
`actionInterval` with the post-content tool saving and the
floor of 1). The exp-drop is a grounded TS NO-OP at
`bfd81e3`: `sendExpDrop` hands a `PacketBuilder(116)` to the
empty `PlayerSession.write()`, no native encoder exists in
`ClientProtocol.ts`, and legacy opcode 116 collides with
`WIDGET_SET_ROTATE_SPEED` (fixed length 8) — so Go mirrors
it with a layout-only `EncodeExpDrop` (never framed/sent)
and a `Session.SendExpDrop` that builds the layout and
discards it; the hook seam still fires at the TS position,
and live XP display travels on the skills delta packets.
Remaining gaps: SkillLevelUp feedback content, the
`CreationMenu` clear in `stopSkillable` (no `CreationMenu`
type), the core `skills:action-interval` emit site (it rides
the decode-drop MagicOnItem listener, so the event fires
only when content calls the helper), `setLevel`'s
vengeance/`BonusManager`/weapon-interface calls, and the
Emotes/TeleportHandler/MagicOnItem/SecondGroundItem
`stopSkillable` call sites (no Go equivalent — do not
invent them). No skilling content grants XP yet: combat
gains drive the gate/rate/emits in production until skill
ports land, piece by piece.

Mobility gates (phase 7 audit, extended phase 15, verified
against `MovementQueue.ts:337-376` and the `Mobility` class
at 1969-2024): `blockMovement` -> INVALID is implemented
(pre-existing, intact, pinned by `TestMobilityGate`); the
FREEZE timer -> FROZEN_SPELL and STUN timer -> STUNNED gates
are implemented since phase 15 (the `TimerRepository` now
exists; the STUN refusal line "You're stunned!" sends
through `sendTypedMessage`, the FROZEN_SPELL refusal sends
nothing, exactly as TS does), including the frozen-object
exception. Every other gate is a gap, because its state
does not exist in Go: needs-placement -> INVALID (the field
exists but no production code ever sets it — TS setters are
unported teleport/login paths — so consulting it would be
dead behavior); force movement -> INVALID (no `ForceMovement`
system); trading/duel button delay -> BUSY and duel
NO_MOVEMENT -> DUEL_MOVEMENT_DISABLED (no trading/duelling).
The combat animation stall -> ANIMATION_STALL gate IS ported
(phase 16: `Combat.IsAnimationStalled`, full stalls only, via
the movement queue's combat-owner assertion).
`Mobility` itself is not ported (with the implemented gates
it could only answer MOBILE-or-blocked-or-frozen-or-stunned,
which `canMove` already does), and the busy/duel refusal
lines stay unwired: no gate state exists to trigger them.

Gaps, all documented at the site: dynamic occupancy degenerates
to false (no occupancy systems). Template
regions, `MapRegionReplacementManager`, and boats/sailing
are DONE since phase 21 (see the sailing/instancing notes
below). NOT STARTED: NPC
subclasses, persistence hooks, ServerPerf/diagnostics. `Player.Process`
drains input, lands the hit queue, ticks timers, then runs the
movement queue with the combat pre/post-movement phases, the
aggression pass, and area processing, in TS order;
`NPC.Process` runs the TS phase order (hit queue, timers,
coordinator, combat pre/post-movement gated on pending work,
following, interaction clearing, areas).
`TurnHandler`
is installed by the wired server (phase 2): it decodes
WALK/MOVE_GAMECLICK into `MovementRequest` and invokes
`Player.OnMovementRequest`, which pathfinds and enqueues
(phase 4); all other turn opcodes go to `listeners.Handle`
(phase 6), which since phase 16 implements the equip/drop/
switch-slot/item-action/pickup/chat/command/object-action/
item-on-object/NPC-option listeners (incl. the combat attack
option) plus player attack option 1, and drops the rest with
named reasons (see the per-listener table).

NPC drivers + combat (phase 16): `NPCMovementCoordinator`
(HOME/AWAY/RETREATING, wander roll 0.26, Chebyshev radius,
radius-0 stationary rule, walk-home routing through a lazily
allocated `RouteFinder`); `NpcAggression` + `AggressionTolerance`
(two 10-tile centres, 1000-tick cap, combat-level rule,
single/multi engagement, melee/ranged/magic attack
distances); the respawn scheduler (definition delay, default
25, 0 disables, clone lineage, unique claims); following/
facing/interaction with dialogue-hold and scripted-movement
guards; `NPC.Process` in TS phase order. Combat:
`Combat` (target/attacker/hit queue/attack cycles,
pre/post-movement phases, cycle guards), `CombatFactory`
(exact accuracy/max-hit formulas incl. prayer chains and
protection terms, hit resolution with combat XP per
`CombatFactory.ts:1181-1225` — `OSRS_DAMAGE_XP` 4, HP share
(dmg*4)/3), melee/ranged/magic methods, the hit queue (rolls
at construction, reveal at cycle+delay, NPC +1/player +0),
death state machines (players respawn after 4 ticks with
"Oh dear, you are dead!", HP restored, no teleport, no item
loss; NPCs dispatch after death ticks then respawn and leave
the world at the remove point (phase 26: the corpse-removal
half was missing until then — the respawn was scheduled but
the dead NPC stayed registered; `SetNpcRemover` seam,
installed by `InstallNpcRespawnWiring`, regression-tested);
the NPC death event fires only with a killer), `PrayerHandler`
state/drain with the combat-facing protection surface,
specials registry (core never drains; restore +10/50 ticks),
and the combat-method provider registry content consults.
Drops are NOT rolled by core: no `npc-drops:roll` exists in
TS core — drops are plugin content listening to `onNpcDeath`,
and Go dispatches `CombatDeathEvent` through the hook seam.
Save split: `specPercentage` is owned (restore + snapshot);
`fightType` is owned since phase 30 (login restore via
`applyEntryFightType`, snapshot writes the live fight type,
`persist.OwnedState`); `autocastSpellId` is owned since phase
27. Deliberate deviations, all commented at the
site: provider matching by `npc.ID` only (no `RealID` on Go
NPCs); barrage detection is `SpellRadius() > 0` (TS also
requires the Ancient spellbook); autocast-active is
`AutoCastSpell() != nil` (TS also checks weapon capability);
ranged data is ported since phase 26 (resolver registered in
production; see the ranged notes below); no animations; combat
projectile producers exist for ranged shots since phase 26
(melee/magic producers still NOT STARTED — the phase-21
senders exist; combat calls them only from the ranged method);
no EquipmentEffects, poison/venom, or skulls; ammo floor drops
fire immediately via hook (TS delays by flight time); same-Z only
(cross-floor degenerate; sailing/private-area terms are
live since phase 21 for visibility and movement).

Timers (phase 15, `timers.go`): `TimerRepository` + `Timer`
+ `TimerKey` port `util/timers/` exactly — a per-`Mobile`
lazy map of countdowns, all 13 repository methods
(`Register`/`RegisterTimerKey`/`Left`/`WillEndIn`/
`GetTicks`/`Registers`/`ExtendOrRegister`/`AddOrSet`/
`Cancel`/`Process`/`Has`/`HasActive`/`Timers`), the 11
`TimerKey` singletons with the TS default-ticks shape (10
with undefined ticks, `BOT_WAIT_FOR_PLAYERS` with 300), and
the tick rule (decrement while > 0, never below 0, the entry
stays at 0). `ProcessTimers` runs in the TS phase position:
`Player.Process` right after the combat-hits phase and
before movement, `NPC.Process` at its TS position. There is
no callback and no `PluginApi` surface: content polls the
repository through `Mobile.GetTimers()` on event payloads,
as TS content does. Timers are SESSION-ONLY: `PlayerSave`
holds no timer state (verified by grep — the five scalar
save fields like `skullTimer` belong to separate systems),
so nothing persists and no save fields were added. The one
documented deviation: `Timers()` returns a snapshot copy,
not the live map (the TS `getTimers` map is mutable by
reference; the copy is the concurrency-safe reading).

Areas (phase 15, `area.go` + `areamanager.go` +
`boundary.go` + `wilderness.go`): gameplay areas are a
DIFFERENT system from the phase-8 music areas (RLE grids in
JSON) and share nothing with them except the music block's
`getMusicTrack` question. Areas are CODE-REGISTERED via
`registerArea` — no data files, no built-in areas in core,
no loader. `Area` is a concrete struct with hook fields
(`PostEnterHook`/`PostLeaveHook`/`ProcessHook`/
`MusicTrackHook`), not an interface + embedded base, because
TS membership tests depend on object identity. `Boundary`
ports the inclusive rectangle + loose height equality (a
height-less boundary matches nothing — callers must pass
the plane). `AreaManager` ports the bucketed search index
(64-tile buckets, max 1024, registration-order fallback
merge), the per-actor hints, and the `Process` membership
evaluation: it runs from `Player.Process` after movement
(with the TS bot downsampling — bots process every 3rd tick
while idle) and from `NPC.Process` at its TS position, and
the multi-icon update inside it preserves the TS stale-area
subtlety (the icon clears one tick after leaving, because
`setArea` runs after the multi-icon block). The multi-icon
itself is `sendVarbit(4605, 0|1)` (opcode 42), change-
detected, with the unset sentinel sending on the first
tick. On removal, players leave their area with
`logout=true` through the isolating caller (a throwing
leave is logged, never fatal); NPCs never leave on removal,
as in TS; hints are forgotten explicitly (Go has no
`WeakMap`). `Wilderness` ports the static helpers over the
world zone boundaries (`LEVELLED_AREAS` strips, `levelAt`,
`isPvpArea`, `isInLocation`, `isIn`, `isMulti`,
`intersectsArea`, `isInSafeBuilding` with the real
`isUnderRoof` check, named `setLevelSource`); `f2p` zones
are excluded from the boundary source, as in TS.
`PrivateArea`/`TemplatedInstanceArea` instancing and the
`getAreaManager` query surface are DONE since phase 21
(see below). Gap remaining:
`MovementQueue.syncWildernessStateForMovedPlayer`
(wilderness level + "Attack" option on movement — unported,
flagged by the phase-15 area work). The walk-to sweep runs
after `process_players`, so an arrival resolving in the
sweep sees its area update on the next tick (a structural
consequence of the existing phase order, documented at the
site).

Sailing + private areas + template regions (phase 21,
`private_area.go` + `templated_instance_area.go` +
`template_region.go` + `map_region_replacement.go` +
`sailing*.go` + `boat*.go` + `world_entity_sync.go`):
`PrivateArea` embeds `*Area` with an entity list, a clip
store, destroy-on-empty, and the `private-area:entered`
custom event for players; `TemplatedInstanceArea` ports the
chunk grid (420 slots, 6x70, stride 28), chunk packing/
rotation, the scene palette, clip-grid build with the +1
ring, template-object lookup, and the instance source tile;
`TemplateRegion` ports the decode-once region decoding over
the cache maps; `MapRegionReplacementManager` ports replace/
get/remove, packet building (uncompressed + zlib fallback),
`.pack` parsing, and the visible-replacement sends — it is
LIVE upstream (`sendClientPacket`), not a no-op mirror.
`Sailing` ports `initialize()` idempotence, the type/dock
registries, `giveBoat`, boarding/disembark/guest rules, the
`LOST_AT_SEA` text, the `sailing:*` custom events, and the
login/logout calls in TS order; `BoatManager` ports spawn/
dispose, deck-loc CRUD, root location, deck-tile projection,
the sync-area rule, steering, `isSailable`, teleport, and
the per-tick boat process; `BoatDeckArea` is a 104x104
private area with walkable/solid sets; `WorldEntitySync`
ports the per-viewer flush (view distance 32, max 8 boats /
4 new per tick, no respawn on the despawn tick). Sailing
state is OWNED save state (normalized on entry, rebuilt on
snapshot); private areas, template instances, and boats are
session-only (TS `PlayerSave` holds no such fields). Clip
reads/writes thread the actor's private area at the same
layer TS does (explicit `privateArea` argument everywhere);
visibility buckets, NPC sorting, and movement routing use
root locations and the sync-area filter; the 6th
`walkToReset` site (boat-deck projected NPCs) is live, and
helm steering runs first inside the movement request, in TS
order. Scoping rule, fixed after the race detector caught
it: the boat manager and the Sailing runtime are scoped to
the world/hub that owns them — NO process-wide globals,
because engine ticks run in goroutines that outlive
`Engine.Stop` and every-tick global reads race the next
server's initialization. Gaps: the `viewedArea` term in
object sync (area objects sync as main-world objects,
template objects not at all); the `sceneTile` root-location
term (only the music tile is root-aware); deck-object
interaction lookup; destroy-hook splicing (pending NPCs
cannot be removed from the world's add queue, instance
ground items have no area field); `MovePlayerTo` does not
clear following/interaction (half of TS `notifyTeleport`);
the sailing SKILL and Construction POH content are plugin
content, NOT STARTED.

Sounds + projectiles + graphics (phase 21, `sounds.go` +
`projectile.go` + the session senders): `Sound` ports the
constructor argument order and all 161 named statics
(153 + 8 aliases, declaration-order first-wins id
resolution) with `resolveKnownSound`/`knownSoundNames`;
`Sounds` ports `sendSound`, `sendSoundEffect`,
`playAreaSoundAt`, and `playAreaSound` (radius clamped
0..31, broadcast radius `max(15, radius+1)`, same-plane
check, area-pointer matching, bot guard, `exclude`);
`Projectile` ports `createProjectile` + `sendProjectile`
(centre/flight-distance/arrival math, broadcast to viewers
with the area + plane + range gates). The senders port
`PacketSender` exactly, including the dropped volume
argument in `sendSoundEffect`, the forced loop count in
`sendSound`, the track-first jingle send, the projectile
end-cycle fallback + actor lock-on mapping, and all 11
camera senders (stateless — camera state + restore are NOT
STARTED); `sendNpcSpotAnimationSpecific` is the FIRST
producer of the phase-8 NPC-effects queue. Every encoder is
live upstream (all reached via `sendClientPacket`); no
no-op mirrors in this phase. Gaps: combat projectile
producers (combat never calls the senders); the
equipment/NPC-combat sound picker call sites (zero call
sites exist); the `Graphic` class + its Core surface;
camera state (`cameraState`, `getCameraState`,
`restoreCameraState`); `setCurrentMusicTrack`/
`noteSongSent`; the pre-existing `combat_ranged.go`
projectile-flight divergence (unexported duplicates of the
`Projectile` methods with a subtle even-size difference)
is untouched, pending grounding.

Spawns (phase 6, `spawns.go`): `SpawnNPCs` applies loaded
`NpcSpawn` records eagerly via `QueueAddNPC` (picked up on the
first tick), each with `Visible = true` (the TS field default,
`NPC.ts:167`), hitpoints from the full TS chain (JSON
monsters-dump override > cache `NpcType.hitpoints` if > 0 > 10,
clamped to >= 10), and facing from the spawn record; no
movement queue, drivers, or hooks are attached — spawned NPCs
are static. `SpawnObjects`/`SpawnObjectRemovals` register/
deregister through the object registry (phase 13, see the
Objects paragraph below): spawned objects block movement
AND are visible to clients through the loc encoders. TS
defers most spawns per-region near players
(`pendingByRegion`, lineage); the Go port spawns eagerly
and deferred activation is NOT STARTED. Duplicates are
kept, as TS stacks spawns. `WanderRadius` is stored but unused
(no drivers).

Objects (phase 13, `objects.go` + `session/objects.go` +
`protocol/loc.go` + `cmd/server` object wiring): the
`GameObject`/`MapObjects`/`ObjectManager` two-register model
— the tile map (shared ownerless non-instanced objects incl.
map originals) plus the world list with by-tile/by-region
indexes (runtime objects only; map originals are skipped
unless instanced) plus the explicit player-keyed owner map
(TS uses a `WeakMap`; Go cleans up via `DiscardPlayer` on
player removal). `Register`/`Deregister` port the TS
replacement/removal/clipping interplay (same-type predecessor
replacement, removed-list splice, `MarkBaseMap`); `Find` ports
the `getPrivateArea` lookup (owner-first variant-resolved
branch, raw `get`, shared-tile variant scan, bridge-tile z-1
fallback, final forPlayer-null guard, developer notice via
the carried-save rights seam); variant resolution reuses the
Phase 12 `forPlayer` seam with the nil-var degradation
(default transform; the per-player vars seam is unwired —
a phase 18 gap, see the interface notes). Region-load
population imports cache-map placements lazily on lookup as
map-original `GameObject`s (tile map only, no clipping
re-applied, no deltas; reloads drop + re-import; the
replay removed-skip is ported). Deltas are collected per
tick (TS sends synchronously inside `perform()`; Go batches
for the flush layer, ground-items precedent — a spawn+
despawn in one tick reaches the client as two ordered
frames). The per-session sync ports `onRegionChange`
ordering (missed removals, removed-objects removals before
additions, hidden removals, region scan, raw owner sends),
shown/missed-removal/hidden tracking, and the delta fan-out
gated by `IsTileInScene`; the loc encoders (add/change 134,
del 135, anim 136, merge 138) carry absolute world tiles.
No object state persists (runtime-only, re-derived from
`object-spawns.json` at boot; shutdown runs
`removePluginObjects` after the saves). Gaps, all commented
at the site: `ReplaceInPlace` emits no removal delta for the
outgoing loc (no missed-removal for away players until the
session shown-state feeds it back); `spawnedBy` is stamped
by the caller, not by a plugin context (content must call
`SetSpawnedBy` or shutdown cleanup will not recognize its
objects); merge/animation encoders have no producers
(`Player.mergeObject` is agility content,
`GameObject.performAnimation` needs the fan-out seam);
`getDefinition`/`performAnimation`/`performGraphic` not
ported; private-area branches degenerate (no areas);
`wasRemovedOnLoad`/`loadRemovals` bookkeeping NOT STARTED;
region-reload runtime-clipping restore omitted
(pre-existing `ReloadRegion` behavior); owner objects are
excluded from `InScene` (no double-send; TS double-sends,
no client-visible difference).

Dialogue (phase 14, `dialogue_manager.go` /
`dialogue_entries.go` / `dialogue_builders.go` /
`dialogue_expression.go` / `transcript.go` /
`transcript_steps.go` / `multi_chatbox_prompt.go` +
`protocol/dialogue_encode.go` + `session/dialogue_sender.go`
+ `cmd/server` dialogue wiring): the `DialogueManager`
state machine (chains, owner scoping, `handleResume` /
`handleOption` with the 1-based child-index quirk, `reset`
without interface removal) with all 8 entry types (NPC,
player, statement, item/double-item statement, option,
action, multi-NPC) and the 30-value expression set, sending
through the `DialogueSender` seam (the session implements
it: chatbox mount/close frames, chat-text frames, mesbox
lines, head/animation/string/script/flags sends, chatbox
owner scoping). The transcript playback engine ports the
NpcDialogues plugin's `startDialogue`/`run`/`presentMenu`
as an engine-adjacent built-in system (the Phase 6
NpcSpawns precedent): page/variant resolution with
predecessor + name/alias fallback, `[player name]` and
gendered `[.../...]` substitutions, conditions, jumps
(MAX_JUMPS 100), random alternatives, overhead shouts
(2-tick continuation), menus with 5-option pagination,
the placeholder line, and the `npc-dialogue:line/action/
choice/hook/menu/condition` custom events. Menus present
through the `MultiChatboxPrompt` registry (the port of
`sendMultiChatboxPrompt`: 10-minute TTL, ownership checks
against manager activity + the mounted chatbox group,
Cancel padding, callback error isolation), and the
dialogue_continue dispatch checks the registry before the
manager, as TS `emitInterfaceActionClick` does. Dialogue
state is session-only (the manager lives on the player
field initializer; zero references in `PlayerSave`) — no
owned save fields. Gaps, all commented at the site:
`StatementLineHeight` uses the no-cache branch only (full
ChatTextLayout NOT STARTED); no FrameUpdater (every
SendString sends); no `closeChatboxFor` equivalent
(cancel checks ownership itself, then removes unscoped);
`presentOwned` degenerates to unscoped presentation;
NPC transforms unported (base definitions only); the
static-binding registry (`registerPluginInteraction`) has
no Go equivalent; shop steps DONE since phase 17
(`FindShops`/`OpenShop` wired to the ShopManager);
`dialogue-types.json`
unloaded (pet content); retired-NPC successors empty
(plugin-private data, no approved read path).

Shops + trading (phase 17, `shop_*.go`, `trading.go`,
`duelling.go`, `secondstimer.go`, `examine_price.go` +
`protocol/shop.go` + `protocol/trade.go` + the
`cmd/server` shop wiring): `ShopManager` ports the TS shop
system — definitions converted field-for-field from the
loaded `ShopDefinition`s in TS load order, with the
`onCanStockItem` veto firing per stock entry at load time
(before `Load`, since the hub loads first); stock with
initial amounts, the TS restock pass timing (once per tick
while the pending flag is set, stopping on the pass after
balance), and player-sold stock handling; pricing in
float64 as TS (rated buy/sell, legacy sell at
`floor(price x 0.85)`, price <= 1 pays in full, specialist
buyback, per-item price overrides winning for buying
only); currencies (coins/blood-money fallbacks, points via
the `shop:points` attribute, content-registered currencies
through the hub registry); and access rules (members
gating via the definition, the `onCanShop` gate). Shop
flows: NPC Trade-option dispatch (the static shop step
first, then the interaction emit), dialogue `open_shop`
steps, the shop widget frames (groups 300/301: value/buy/
sell/x actions, quantity modes), buy/sell transactions
with inventory/coins checks and TS-exact refusal text,
and close behavior (explicit close on interface loss and
departure; auto-close on movement/logout/death through the
phase 18 runtime). `Trading` ports the TS trade system: the six-state machine
(requested/trade/accepted/confirm/accepted-confirm),
request/acceptance with the 2s request delay and mutual
initiation, the 28-slot offer container, offer/remove with
the `trade:item-eligibility` event, both accept stages
with the accept-delay rule (varbit 13130, live through the
phase 18 varp store), decline/close with offered-item return
or ground fallback, completion transfer with the
`ReceiverCanHold` re-check, the mirrored
`trade-completed` emit, and the post-trade save. Trade
gates: movement/combat/prayer gates read the
Trading-derived status, and the WORLD_INTERACTIONS
TRADING-status preamble drop degenerates to the busy gate
(same observable outcome). `Duelling` ports the shared
state only (8 states, 23 rules, live 28-length rules
slice); Duel Arena gameplay/staking is plugin content, NOT
STARTED. The duel-order shuffle exception in
`shufflePlayerProcessOrder` is implemented (duelling
players keep their slots). Save/state: shop stock is
runtime-only (rebuilt every boot); the only shop save
state is the `shop:points` and `shop:quantity-mode`
attributes, and trades persist only through the escrow
fold (offered items folded back into the inventory at
save time) plus the post-trade save. Gaps, all commented
at the site: no sound system; `canTrade` calls
`area.CanTrade` directly
(panics propagate; TS catches); no `FrameUpdater` clear;
walk-to-entity (ported for the trade request) lacks
facing, unreachable-NPC, pre-step pursuit, and
projected-NPC/boat handling; the Grand Exchange
marketplace is NOT STARTED (plugin content needing
milestone 8; the GE-value accessors ARE ported and feed
shop pricing, trade value sums, and the examine price
line). The phase 17 gaps that the phase 18 runtime closed:
the examine price line is live (the settings gate reads
the varp store), the trade accept-delay triggers (varbit
13130), and buy/sell-X prompts round-trip through the
entered-amount listener.

Interface runtime (phase 18, `game/interface_state.go` +
`enteredaction.go`, `session/interface_state.go` +
`interface_sender.go` + `varps.go`, `definition/layouts_open.go`,
`listeners/interfaceaction.go` + `prompts.go`, `plugins/
interfaces.go`, `cmd/server` wiring): the TS interface system
as core defines it. Per-player state: `Player.interfaceId`
(-1 default) with `SetInterfaceId` emitting
`interface:closed`/`interface:changed` custom events, and the
close gate (`closeInterruptibleInterfaces` semantics: an
interface closes only while the player can move). The sender
side tracks mounted sub-interfaces, chatbox group/owner, and
the gameframe root, with the full `PacketSender` surface
(`sendSubInterface`/`closeSubInterface`, `sendInterface`,
`sendChatboxInterface`, `sendTabInterface`,
`sendSidebarInterface`, `sendInterfaceSet`,
`sendWalkableInterface`, `sendInterfaceRemoval`,
`closeInterface`, and the TS-order `resetInterfaceState`,
which also resets the dialogue manager). Layouts open through
`OpenLayout` on all 7 surfaces with the TS errors, on the
phase 6 layouts loader; validated read-only against the real
definitions dir (all 38 layouts open). Close triggers:
movement (the WORLD_INTERACTIONS move preamble, gated on
`CanMove`), logout (removal before the logout hooks, TS
`Player.onLogout` order), and death. Action dispatch: the
opcode handle with the dead/busy/teleport gates and the
BUSY_MESSAGE refusal, the MultiChatboxPrompt check first
(slot-preferred-over-action, closing the phase 14 slot-form
gap), then the hub emit; the shop widget branch (groups
300/301) is unchanged. Entered-amount/syntax prompts: the
holder actions with accept/cancel, the opcode 192/193/194
answers, the abort frame (opcode 123), and the run-client-
script prompts (scripts 108/110); shop buy/sell-X arms the
amount action. The live varp/varbit store is the
`PacketSender` form: per-player map with change detection,
insertion-order replay, varbit packing over cache
`VarBitType` defs (int32 semantics, endBit inclusive), and
`sendConfig`/`sendVarbit`/`sendToggle`/`sendVarpLong`;
`getVarbit` on an unknown id returns 0 and `sendVarbit`
sends the frame without recording, mirroring TS. The store
unblocks the phase 12/17 gaps: the examine price line reads
the settings gate, the trade accept-delay reads varbit 13130,
and the multi-icon send routes through `SendVarbit(4605)`.
Save/state: nothing persists — `PlayerSave` has no
varp/varc/interface/prompt fields (grep-verified), so varps,
interface id, and prompt actions are session-only and no
owned fields were added. Gaps, all commented at the site:
`VarManager` transform plumbing is NOT STARTED (zero core
instantiations in TS — it is not the live store);
`registerCustomInterface` is NOT STARTED (needs the HTTP
ContentApi endpoint; the Go transport is WS-only); bank
interface flows are plugin content (no core bank listener
exists in TS; `endBankSearch` is a no-op — the Go Bank holds
no search state); the per-player vars seam (`forPlayer`
var resolution) is unwired; bootstrap/music/dialogue-unclamp
varp sends bypass the store; `emitButtonClick` fallback,
animation-stall hold, bank/prayer/combat/spell widget
branches, and spell-on-item routes are gaps; the IF_CLOSE
nothing-open branch is a TS TODO; the teleport gate
degenerates to pass (no teleporting state); audio-setting
application from `varp_transmit` is NOT STARTED (the login
replay of the carried audio settings IS wired).

Clans/social (phase 19, `game/relations.go` + `names.go` +
`socialpacket.go`, `protocol/social.go` + social decoders,
`session/social_sender.go`, `listeners/social.go`,
`plugins/hooks.go` social variants, `cmd/server`
social wiring): the TS social system as core defines it —
and core defines less than the name suggests. `PlayerRelations`
is ported whole: friend/ignore lists as int64 name-hashes
(caps 200/100), friend ranks, friends-chat channel name /
last owner / entry-talk-kick ranks, private-chat status +
public/trade modes with TS clamping, the presence/PM/public-
chat gates, `updateLists`, add/delete with TS-verbatim
refusal text, `message()` PM delivery, `onLogin` sends, and
the private-message id counter. The `Misc` name functions
relations needs (`longToString`, `formatName`/`fixName`) are
ported in the game layer; `stringToLongBigInt` already
existed as `session.NameToLong`. Dispatch decodes the five
social opcodes (181/196/197/198/199) plus the CHAT clan and
friends-chat kinds and emits all 8 `PluginSocialPacketEvent`
variants to the hub — and takes no further action, exactly
as TS dispatches them (`NetworkBuilder.ts:894-911`): there
is no core default social handling, so production
add-friend/PM/filter behavior awaits the FriendsList content
port (the smoke probe played that role in miniature). The
chat gate is live: `CanReceivePublicChatFrom` reads lists +
modes, closing the phase 12 degeneration. Save/state:
friends/ignores/ranks/channel fields are owned (see the
persistence notes); chat modes + the private-message id are
session-only (`PlayerSave` has no such fields — modes persist
in TS only through the plugin's `chat.filters` attribute).
Logout calls `updateLists(false)` in TS `onLogout` order
(after the logout hooks, before the save); the login
`onLogin` sends are exposed but NOT wired by core (TS: the
FriendsList plugin calls them). Two findings, both verified
against TS: (1) the legacy friend/ignore presence sends
(opcodes 50/51/214/215/221) are no-ops in Go because they are
no-ops in TS — `PlayerSession.write` is an empty method
upstream, and live presence travels in the friends-chat
snapshot (opcode 121), which content sends; the snapshot +
clan encoders (121/124-128) are ported with no producers.
(2) TS `Misc.textUnpack` is broken at this commit
(`parseInt` of a letter is NaN, so it returns NULs); Go
ports the intended nibble/table algorithm `textPack`
inverts, a documented deviation with no live TS caller to
diverge from (`message()` has zero callers upstream).
`onFriendAdd`/`onFriendRemove` stay table-only: zero emit
call sites exist in TS core or plugins. Gaps: friends-chat
channels + the clan system (plugin content, NOT ported),
snapshot sending (content's job — no raw-byte content send
path exists on `*game.Player`), the sender-side PM echo
(`private_out`), `chat.filters` attribute persistence
(content's `persistAttribute`), and Core exposure of the
clan encoders (table-only: the signatures would force a
plugins→protocol edge and leak wire structs into content,
and no TS content calls them through `core`).

Ground items (phase 8, `grounditems.go` + `session/grounditems.go`
+ `protocol/grounditems.go`): `GroundItemManager` ports
`ItemOnGroundManager`/`ItemOnGround` — every item is a stack
with an owner, a reveal cycle, and a despawn cycle. Timings,
all verified against the TS source: owner-only for the first
100 ticks (`PUBLIC_REVEAL_DELAY`), then visible to everyone;
despawn 300 ticks after the drop (`DESPAWN_DELAY`); respawn
timers re-create static spawns through one-shot `TaskManager`
tasks; stack merging combines same-item stacks at the same
tile up to `MAX_STACK_AMOUNT` (0x7fffffff) and keeps the
earlier reveal/despawn cycles. The manager runs on the tick
through the `World.Hooks.GroundItems` hook (the phase 6 stub
is now wired); its seams (`GroundItemManagerConfig`: Players,
Sender, Scheduler, Cycle, Stackable, TradeableDropable) keep
`game` free of `session`/`protocol` imports, and the
`cmd/server` adapter supplies them. Per-session sync
(`GroundItemSync`) sends a full snapshot on region/level
change (chunks of 1927 stacks per packet, serial-tracked),
view-gated to 64 tiles (Chebyshev, same plane), plus deltas
for create/alter/remove events collected during the tick.
Static spawns from the phase 6 ground-items loader are
applied at boot through `applyGroundItemSpawns` (respawn 0 =
static, never despawns; `respawn` counts seconds in the JSON
and is converted to ticks). Pickup and drop flows were
implemented in phase 9 (see the per-listener table): dropping
registers a stack through the manager with owner/reveal/
despawn cycles, picking up walks to the stack and transfers
it to the inventory with capacity/merge semantics; both are
live-verified. Verified live: near/far visibility, region-
change resends, and the 64-tile exclusion (see "What was
verified").

### Persist (`internal/persist`) — PARTIAL

Ported and tested: `HashPassword`/`CheckPassword` (scrypt +
legacy plaintext fallback + timing-safe compare),
`TokenEntry`/`HashToken`/`NewToken`/`VerifyAndRotate` (stored
shape `{hash, previousHash?, lastUsed}` exactly as TS),
`Accounts` (full method set, tombstones, whole-file writes),
`DisplayNames` (ownership/hold rules), `Punishment` (four txt
lists, verbatim parsing), `Profile`/`StorageKey`,
`PlayerSave` (every TS field represented with identical JSON
names; `fightType`/`sailing`/attribute values verbatim via
`json.RawMessage`; unknown keys preserved), `SaveStore` +
`MemoryStore` + `JSONFileStore` (atomic write + fsync, validation
mirroring `validateSerializedSave`). A save written by
`JSONFileStore` was verified to parse under the TS reviver in
Node. Phase 5 added the apply/from-player merge (`apply.go`):
`OwnedState`/`WithOwned` (deep-clone merge onto the loaded save),
`SanitizeAppearance`, legacy-field/attribute migration
(`MigrateForEntry`), `Unmake`, `NewCharacterSave`, and the
load-time legacy button-indexed skill unmapping in `store.go`.
Owned-vs-preserved split (the Go player owns only what it can
hold): snapshots overwrite position, running, `runEnergy`
(phase 7: the live units/100, per `PlayerSave.ts:762`
`fromPlayer`; entry loads it per `PlayerSave.ts:654`
`applyToPlayer` via `setRunEnergy` semantics,
`floor(percent*100 + 1e-6)` clamped to 0..10,000, held in
`OwnedState.RunEnergyUnits`), and since phase 9 the live
inventory, equipment, appearance (sanitized on write, per
`fromPlayer`), skills (cloned arrays), and banks (tabs 0–9
via `getValidItems()`, search tab skipped, per
`fromPlayer`; entry loads them per `applyToPlayer`:
`setItems` for inventory/equipment, sanitized appearance,
`setSkills`, banks through `add` for tabs present in the
map), and since phase 11 the live attributes (see below) —
with nil-means-preserve semantics in `WithOwned`, so
a nil slice/map (or all-nil skill arrays, or a nil attribute
map) leaves the carried value untouched. Also overwritten:
`teleblockTimer` (always 0, as TS `fromPlayer` does); the
password hash is overwritten at entry from the pending
login; everything else — displayName, rights, spellbook,
fightType, autocast, timers, quick prayers,
flags, `Extra` — is preserved
verbatim, including unknown keys. Since phase 21 the
sailing state is owned too (entry normalizes + loads it
into live state in `applyToPlayer` order; snapshots rebuild
it from live state in `fromPlayer` order; a nil live state
preserves the carried value). Since phase 19 the
friends/ignores/friend-ranks/friends-chat channel fields
are owned too (entry loads them into live relations in
`applyToPlayer` order; snapshots rebuild them from live
relations in `fromPlayer` order, normalized to positive
unique stringified hashes capped at 200/100; a save that
omits the channel rank fields loads the TS getter defaults
−1/−1/2). The trade-escrow
fold in `fromPlayer` is ported since phase 17
(`persist.FoldTradeEscrow`: offered items fold back into
the inventory at save time; overflow skips the save).
Phase 11 added the persistent-attribute registry
(`attributes.go`): the process-global persistent-key set +
default map mirroring the `PlayerSave` statics
(`PlayerSave.ts:52-101`), seeded with the 26 core keys
(22 legacy-field values + `npc-aggression:tolerance`,
`creation-menu:last-item`, `login:tokens`,
`combat:style-by-category`) and 8 core defaults, extended
at boot by `persistAttribute` registrations
(`PersistAttribute`: TS key validation, `cloneAttribute`
JSON-compatibility rules, defaults stored as canonical
JSON, insertion order kept). Entry loads only persistent
keys present in the (migrated) save, then fills missing
defaults (`applyToPlayer` order; new accounts get defaults
only); snapshots rebuild attributes from scratch from the
live persistent keys minus default-equal values
(`fromPlayer` order) — so a snapshot DROPS attribute keys
the live player does not hold under a registered key,
including unregistered keys the carried save held. This
replaces the pre-phase-11 preserved-verbatim behavior and
is TS-faithful, not a deviation. The kebab-case namespaced
key convention is convention-only: no enforcement exists
beyond `persistAttribute`'s own key validation, and the
scoped-settings "no declared key" warnings concern
pluginConfig keys, not attribute keys, and stay a gap.
Engine-consumed keys the TS content plugins register
(`appearance:pronouns` default -1, the three skin-unlock
keys default false, `client-layout-root` default 161,
`xp-drops:enabled` with no default) are registered at boot
in `cmd/server/attributewiring.go`, mirroring the
MakeOverMage/Settings registrations, so the engine's own
reads keep round-tripping; `client-layout317` and every
other content key stay unregistered until their content
port declares them.
Appearance is sanitized once at entry; `isSelectableKit` is
a clamp-only fallback (the kit table lives in the cache
layer this package cannot import — the same fallback TS uses
with no cache loaded). One known limit: a zero at a legacy
button index reads as absent, so hitpoints experience
exactly 0 loads as the default (1184). The SQLite
backend (`sqlite.go`, `modernc.org/sqlite` v1.57.0, pure Go, no
CGO — the newest release compatible with Go 1.25.1) implements
the TS `player_saves` + `player_save_history` schema with the
exact TS upsert, SHA-1 dedupe, gzip snapshots, and retention
pruning, reusing the JSON encoding and validation (byte-identical
JSON to `JSONFileStore`); writes are synchronous, as with
`JSONFileStore`. NOT STARTED: snapshot listing/restore, pending
restores, legacy JSON import, pid-file single-writer guard,
`requestSave` coalescing, offline admin ops
(`updateOffline`/`copyProfile`/`delete`/`changePassword`);
background 30 s last-played flush timer (callers must call
`Flush()`).

### Wiring (`cmd/server`) — PARTIAL

Boot order (config -> cache -> accounts/saves -> world/engine ->
HTTP+WS), flags (`--root`, `--port`, `--no-cache`, `--tick`,
`--save-backend`), graceful shutdown, and the login<->world
adapter are implemented and integration-tested (including a real
WebSocket round trip). The adapter installs the `TurnHandler`
(WALK/MOVE_GAMECLICK decode + movement-request hook, all other
turn opcodes to `listeners.Handle`), returns the turn-packet sink from `AddPlayer`, sends the LOGOUT_RESPONSE
packet before closing on accepted logout, and wires the
bootstrap's world-level facts (members flag, world-type label,
dev usernames) plus the process cycle. Boot-time definition
loading (phase 3): `buildServer` constructs the
`CacheDefinitions`/`CacheMaps`/`Gamevals` facades from the
ensured cache (see Cache notes), wires the XTEA keys into the
adapter (`XteaKey`, nil-safe) and the item-weight source into
`LoginDeps`, and closes the store on shutdown. Movement wiring (phase 4):
`buildServer` constructs the world's `RegionManager` from the
cache adapter (`regions_adapter.go`), selecting the new terrain
format at revision >= 209; with no cache the manager still
exists but loads nothing (all ground open). The per-tick
flush glue (phase 3, `syncsource.go`): `AddPlayer` attaches a
`FlushSource` snapshotter + the XTEA provider to each entering
session before queueing, so the first `FlushTick` already has a
view; snapshots carry the stored appearance block (or nil for
the session's default fallback) with the bootstrap-race dirty
guard. Save wiring (phase 5, `savewiring.go`): entering players
apply their save (position/running, migrated in place before
the bootstrap reads it; phase 9 also applies the live state
— run-energy units, inventory/equipment contents, sanitized
appearance, skills, pronouns/skin-unlock attributes, and
banks for tabs present in the map — through the
`ItemDefinitionSource` installed first thing on the player,
so stacking consults real definitions; phase 11 applies
the save's persistent attributes onto the live player in TS
login order — transient mobile/native attributes from the
handshake facts first, then the migrated save attributes
present under persistent keys, then registered defaults
for missing keys, then the live `login:tokens` attribute
from the folded pending tokens — with new accounts getting
defaults only; the phase 7/8
entry-time static weight and static Agility accessors are
gone, replaced by the live systems) or start from
a new-character save
(DEVELOPER rights for configured dev usernames, remembered-login
tokens folded for the primary only); the carried save is
registered on entry, activated only when the add-wait succeeds,
and written on disconnect (reason `"logout"`), by the Autosave
task (reason `"autosave"`, name-hash spread, max 10/tick), on
shutdown (reason `"shutdown"`, snapshotted on the tick goroutine
and written after the engine stops — failures log and shutdown
proceeds, a documented deviation from TS, which cancels shutdown
on save failure; at most one tick of movement can be lost
because the snapshot is taken in the first phase of the tick),
and on tick errors (reason `"tick-error"`); snapshots rebuild
the attribute map from live state (phase 11: persistent keys
held live, default-equal values stripped; a non-JSON-compatible
live value fails that player's save with per-player recovery,
as the TS `cloneAttribute` throw fails `fromPlayer`); writes are
synchronous on the tick goroutine, with per-player recovery,
and bots are never saved. The `--save-backend sqlite` case is
wired (`PLAYER_SAVE_DATABASE_PATH`, default
`data/saves/players.sqlite`); `json` remains the default.
Definition/spawn wiring (phase 6, `definitionwiring.go`):
`buildServer` runs overlays, then `definition.Load`, then
spawning — after the `RegionManager` is constructed (object
clipping needs it) and before the engine starts — following
TS boot order (plugin registration before `GameBuilder`, so
spawn-id validation sees extended counts). Load errors are
boot-fatal and returned wrapped, never downgraded; a nil
`Store` is guarded, never a crash. Counts are logged
Phase-3-style. Music/ground-item wiring (phase 8,
`musicwiring.go`/`grounditemswiring.go`): `buildServer`
constructs the `GroundItemManager` with the adapter seams,
applies the loaded ground-item spawns, installs it on
`World.Hooks.GroundItems`, and loads the music data (area
grids from `music-areas.json`/`music-areas-classic.json` plus
track lengths from DbTable 44 when a cache is present) —
music-data failures log and continue, never fail the boot,
because TS degrades to no music when the data is absent;
`AddPlayer` attaches the music data and the ground-item
source to each entering session. The save carry also applies
the player's audio settings (`SetAudioSettings`) so the
flush reads the saved mode/volume/loop/shuffle.
Container/skill/appearance wiring (phase 9,
`containerswiring.go`/`appearancewiring.go`): `buildServer`
constructs the `ItemDefinitionSource` adapter (definition
lookups delegate to the loaded `Store`; inventory/ground
actions and the shift-click index come from the cache
`ObjType` through the optional `ItemActionSource` seam, with
ObjType defaults when no cache is present) and installs it
on every entering player before the save is applied;
`AddPlayer` installs the container refresh hooks (inventory
-> full snapshot, equipment -> worn-inventory run-script for
native clients, bank -> tab-count varbits + bank snapshot +
inventory snapshot) and the skills/appearance wiring (skill
hook -> `EncodeSkillsDelta`, `OnEquipmentChanged` ->
appearance-block rebuild -> `SetSyncAppearance`), all before
queueing, and binds the turn-packet `Deps` (the ground-item
manager, only when present — a nil manager degrades drop/
pickup to a named drop instead of panicking). In NoCache
mode the overlay step is a no-op (files unread) and spawns
still load (nil-cache degradation). Plugin wiring (phase 10,
`pluginwiring.go`): `buildServer` builds the hub from the
compiled-in content registry and the world definition
(switches, plugin config, members flag) and loads it before
the world is built, as TS loads plugins before `GameBuilder`;
the cycle source and the per-tick process emit are installed
on the world once it exists, and the adapter carries the hub
for the login/logout/disconnect emits (login after world
entry, before the bootstrap; logout before the logout
response; disconnect before the disconnect save; shutdown
after the engine stops, before the shutdown saves are
written — matching the TS order in each case; see the Plugin
hub notes). Content packages join the server with one blank
import each in `pluginwiring.go`. Known
wiring limitations, all deliberate and documented in code:
`AddPlayer` waits up to 3 s for the tick loop to assign
an index (TS is synchronous on one event loop; a timeout fails
the login and a late add is immediately removed); the legacy
whole-list `disabledPlugins` form is not honored (the config
package does not carry it); the shipped `data/definitions/` is
a minimal starter (no zones, empty definition files — replace
with the real TSPS files for full content), except the two
music-areas files, which are verbatim imports of the real
TSPS data (see "What was verified").

### Packet listeners (`internal/listeners`) — 11 implemented + 1 partial, rest decode + explicit drop

TS has no listener registry: `decodeClientPackets` produces
typed messages, non-immediate ones queue as turn packets, and
the typed `switch` in `dispatch()` (`NetworkBuilder.ts:719`)
calls listener methods directly. The Go port mirrors that
shape: `listeners.Handle` decodes each turn packet exactly as
the TS branch does and, for the eleven implemented listeners,
acts exactly as the TS listener does (see the table); for
every other listener file it drops with a named reason,
because the target system does not exist in Go. The
`varp_transmit` run toggle is a dispatch-inline case in TS
(`NetworkBuilder.ts:1147-1168`) and is implemented in `Handle`
for varpId 173 (see the last table row). Dialogue_continue
(opcodes 62/252) is a dispatch-inline case in TS
(`NetworkBuilder.ts:913-951`) and is implemented in `Handle`
through the `DialogueAccess` seam: the pending
MultiChatboxPrompt check runs before the manager's
`handleResume`, as `emitInterfaceActionClick` does in TS.
Nothing else is invented: no plugin event is emitted for the
dropped listeners. Chat broadcast and examine answers are
implemented since phase 12 (see the table); examine lives in
TS `dispatch()` itself and is implemented as dispatch-inline
cases in `Handle`.
Malformed payloads yield a named malformed-payload drop,
never a disconnect or panic (TS closes with 1002 at
frame-decode time, but Go framing already enforces table
lengths before `Handle` runs). `InterfaceActionClickOpcode`
is a helper, not a listener (no opcode of its own; reached
from widget/local-trigger cases); Command, FollowPlayer, and
TradeRequest have no opcodes of their own (sub-dispatches of
chat / player-option). Trade-action (180) is implemented
since phase 17 (offer/remove/accept/decline/confirm).
Social packets (181/196-199 + the CHAT clan/friends-chat
kinds) are dispatch-inline cases since phase 19 (decode +
hub emit, no core default action). Opcodes
with no listener mapping in
TS (50, 245, 220-225) return
unrecognized.

| Listener (TS file) | Opcodes | Status |
|---|---|---|
| ChangeAppearancePacketListener | 37 | decode-drop: appearance-change flow NOT STARTED (interface tracking + removal now exist since phase 18; the flow itself does not) |
| ChatPacketListener | 190 | implemented: normalize (strip `<>`, trim, 80 chars), `::` commands dispatched before any gate, `allowChat` (empty silent; muted + blocked-word refusals with TS text), social-packet emit point, `forceChat` with icon prefix, broadcast to speaker + local players + nearby-supplement at the 255 cap with dedup and the live relations gate (lists + modes, since phase 19); clan/friends kinds emit to the hub via the social dispatch (see the SocialPacketDispatch row), game kind no-action; overhead text via the forced-chat sync update blocks; no rate limit / hitpoints gate / sender chat-mode check (none in TS core); chat icons render only when content sets them |
| CommandPacketListener | none (sub-dispatch of 190 on `::`) | implemented: parse (NULs->spaces, trim, JS-`\s+` split, strip leading `[:/]+`, leading `[a-z0-9_]+` lowercased, no quoting), any-command hooks -> base lookup -> rights gate (TS denial text) -> handlers in order; rights floor from world.json `commands:permissions` > override > registration, read from the carried save (no live `Player` rights field); unknown command warns + TS "This command does not exist. Try ::players." reply; hitpoints gate via live skills; first shipped command: `::item` (phase 38, `internal/content/admincommands`, ADMINISTRATOR, `AdminCommands.plugin.js:1623-1635`, live-verified: `::item 882 5` / `::item 995 1000` spawn + messages on the wire, usage on no args, non-admin denial on the wire; `::give`, `::items` and all other admin commands NOT STARTED — see gap 21) |
| DropItemPacketListener | 240 + widget inventory path | implemented: hitpoints/interface/slot/id/amount gates, `stopSkillable`, `isDropable` -> ground-item registration (owner, location, goesGlobal, respawn -1), slot clear + refresh; undroppable -> destroy-widget flow gap (neither dropped nor destroyed); Wilderness check is live (phase 15: `Wilderness.isInLocation` through the `WildernessIn` seam — PvP area minus safe areas and safe buildings — selecting the global registration); the DEVELOPER refusal message is a gap (no rights system), sounds are gaps |
| EquipPacketListener | 240 + widget paths | implemented: hitpoints gate, slot bounds + slot/id check, `stopSkillable`, interface-3214 check, requirements via `GetUnmetRequirement` (refusal text wired through `sendMessage` since phase 12), equipment-slot -1 refusal (TS-exact silence: server-side log only), unequip no-space refusal text wired, stackable-same-stack merge, 2H/shield rule incl. `slotsRequired` and empty-hands case, shield-vs-2H branch, ordinary swap/displace, weapon-slot handling, appearance refresh via `OnEquipmentChanged`; `resetWeapon` (combat), `BonusManager.update`, weapon-interface update, following clear, plugin can-equip/unequip gates (no-hooks default = proceed), sounds are gaps |
| FollowPlayerPacketListener | 46 + 233 (byte 0), via player_option 3 | decode-drop: following/pursuit machinery + World player lookup NOT STARTED |
| InterfaceActionClickOpcode | 251 + native 13, 89, 91, 23, 25, 31, 63, 69, 11, 14, 19, 20, 84 + 90 + 30 | IMPLEMENTED (phase 18): dead/busy/teleport gates + BUSY_MESSAGE, slot-preferred-over-action prompt check first, hub dispatch (generic + per-button + named overloads, stop once handled), no-handler falls through to the dialogue tail; shop widget actions (groups 300/301: value/buy/sell/x + quantity-mode buttons, routed to the ShopManager) implemented since phase 17 and untouched; `emitButtonClick` fallback, animation-stall hold, bank/prayer/combat/spell widget branches, and spell-on-item routes are gaps |
| ItemActionPacketListener | 240 + widget inventory path | implemented: all gates, walk-to cancellation, equip fallback reusing `equip()` (interface 3214 only), widget-op resolution (op 10 Examine, op 1 shift-click, ops 2/3/4/6/7 -> actions 1-5), drop-option detection; plugin actions stay drops (plugin-only); hard-coded special cases 9520/2545/12873/12875/12879/12881/12883/12877 unhandled with gap comments; 9520/2545 messages wired through `sendMessage` since phase 12, interface removal exists since phase 18 but is not wired at these sites |
| MagicOnItemPacketListener | 94 + 90 | decode-drop: spell system (spell-id resolution, CombatSpells table, validation) NOT STARTED; telegrab NOT STARTED |
| MagicOnPlayerPacketListener | 32 | decode-drop: spell system NOT STARTED (no spell-id resolution, CombatSpells table, or validation exists; nothing invented) |
| NPCOptionPacketListener | 76, 12, 34, 70, 57 + 75 | implemented (phase 14, combat option phase 16): all 5 click types walk + dispatch; gates: hitpoints > 0, slot holds the NPC, ≤ 24 tiles same plane; walk reuses `WalkToTile` + `ReachedObj` (documented deviations: 1×1 reach, no pursuit, no projectile-clip term); arrival re-validates, then the combat branch (the "attack" option on an attackable NPC: `emitNpcAttackOption` gate, autocast clear, `Combat.attack` — the combat process routes the attacker in, so the combat branch never walks) -> the dialogue seam (Talk-to only) -> interaction emit -> bank gap -> "Nothing interesting happens." fallback; claimed clicks emit `npc:operated`; spell-on-NPC (75) decode-drop (spell system NOT STARTED); item-on-NPC (36) decode-drop (no item-on-NPC dispatch; walk-to-entity exists since phase 17 for the trade path) |
| ObjectActionPacketListener | 96, 28, 42, 38, 51 + 231 | implemented: hitpoints/busy gates (`busy()` degenerates to the hitpoints term), packet x/y + player's plane, registry lookup (miss = silent return; the DEVELOPER notice fires through the registry's rights seam), definition via `forPlayer` on the object's id, exact click-type/action normalization (valid 1-5 ignores action text; otherwise trim+lowercase match against interactions), route event before mobility checks, walk-to-object with the TS reach rectangle + repath rules + "I can't reach that!" failure, arrival still-stands re-check by packet id + click tile, dispatch order before-hooks (claim skips mains) -> merged generic + `id:clickType` + name hooks in registration order, stop once handled -> `object:operated` emit, no-handler fallback is TS-exact silence; the deck-object interaction lookup (sailing), the `bank` option (Bank.open/status, not the runtime — the phase 18 runtime exists), facing, and the mounted-decoration reach term are gaps |
| PickupItemPacketListener | 102, 43, 103, 56, 82 + 232 | implemented: `takeOperation`/`getOperableGroundItem` (stack-id path + own -> unowned -> any preference, registration order), `canOperate`/`canInteract`, walk-to arrival via `WalkToGroundItem` + `ReachedObj`, `transferToInventory` (capacity via `GetAddableAmount`, before/after matching-amount credit, full -> deregister, partial -> `OldAmount` + decrement + ALTER, pickup-replacement credit, meta preserved), 300ms throttle as a cycle stamp; `busy()` degenerates to the hitpoints term, private-area checks degenerate to equal, plugin pickup/take/reward/taken events -> no-hooks defaults, "I can't reach that!" text wired through `sendMessage` since phase 12; reach animation + sounds are gaps |
| Examine (dispatch-inline cases, not a listener file) | 9 (NPC), 85 (object), + inventory/ground/worn examine routes | implemented: NPC examine (typed, type 28) + object examine (typed, type 29, full multi-loc rule with the `world:object-examine` plugin intercept) + item examine (examine-or-name text) + ground/worn routes + "Nothing interesting happens." + the warn-once backpack message; the item-examine price line is live since phase 18 (value/GE/alch accessors ported in phase 17; the settings gate reads the varp store); unknown ids answer "It's nothing special." |
| DialoguePromptAnswers (dispatch-inline cases, not a listener file) | 192 (amount), 193/194 (input) | IMPLEMENTED (phase 18): exact decoders (amount = single int), prompt-first routing to the armed entered action (amount/syntax), cancel/close abort paths with the trigger frame; Bank/SkillManager prompt consumers unwired (no call sites yet) |
| InterfaceCloseDispatch (dispatch-inline case, not a listener file) | 55 (IF_CLOSE) | IMPLEMENTED (phase 18): strict-empty decoder, closeTrackedInterfaces + interface:closed emit when an interface is open; the nothing-open branch is a TS TODO (no-op) |
| SocialPacketDispatch (dispatch-inline cases, not a listener file) | 181, 196, 197, 198, 199 + CHAT kinds 2/4 | IMPLEMENTED (phase 19): exact decoders (signed rank byte, unknown actions malformed-dropped), nil-player drop after decode, hub `onSocialPacket` emit for all 7 kinds (+ public_chat from the chat listener = all 8 union variants), no further core action regardless of handled — exactly as TS dispatches them; production handling is plugin content (FriendsList/Clans, NOT ported) |
| PlayerOptionPacketListener | 44, 45, 46, 73, 6, 48, 10, 21 + 233 | PARTIAL (phase 16 attack, phase 17 trade): attack option 1 implemented — deliberately NO distance/plane/Wilderness/skull/level checks in the listener (all in `CombatFactory`, per the TS listener); trade option 2 implemented — the shared gates, plane + 20-tile Chebyshev checks, hitpoints/registration gates, `onTradeRequest` emit, walk-to-entity delivery, busy refusals (see TradeRequest row); options 3-8 decode-drop: following, plugin-owned player options NOT STARTED |
| SecondGroundItemOptionPacketListener | same ground opcodes | decode-drop: plugin-only |
| SwitchItemSlotPacketListener | 242 + 1, 253 | implemented: hitpoints gate, bank path `[50300, 50311)` with tab from interface id + `Rearrange(from, to, insertMode)`, inventory swap + refresh for 3214/5064/1688; searching-bank always false, insert mode from the live `bank:insert-mode` attribute (phase 11; default false); `reconfigureTabs` gap |
| TradeRequestPacketListener | 45 + 233 (byte != 0), via player_option 2 | implemented (phase 17): index bounds, 20-tile Chebyshev same-plane check, hitpoints/registration gates, emit-handled short-circuit, `walkToEntity` delivery to the request; trade-action packets (180: offer/remove/accept/decline/confirm_accept/confirm_decline) implemented in the same file |
| UseItemPacketListener | 65, 36, 86, 79 + 2 + 241 | PARTIAL: item-on-object paths implemented (no hitpoints/busy gate, slot bounds + item-id match, `canUse` degenerates to allow, walk-to-object clickType 0, arrival still-stands re-check, hub `onItemOnObject` dispatch, unhandled -> "Nothing interesting happens.", no inventory re-verification of the captured item, no operated emit); item-on-NPC/player (no item-on-NPC/player dispatch; walk-to-entity exists since phase 17 for the trade path), item-on-item / item-on-ground-item (unfired emits), and spell-on-object (spell system NOT STARTED) stay decode-drop with `ReasonUseItem` |
| VarpTransmit run toggle (dispatch-inline case, not a listener file) | 191 (VARP_TRANSMIT): varpId 173 toggles running; other varpIds decode-drop (43 combat style, 172 auto-retaliate, audio settings for every id) | implemented: setRunning(value != 0 && getRunEnergy() > 0), then send run status; player:run-toggled plugin event and audio-setting application NOT STARTED; combat-style varp 43 stays decode-drop (needs weapon→FightType resolution, per-category style memory, autocast config send, `BonusManager.update`) |
| World-entity interaction (dispatch-inline, no TS listener file — `NetworkBuilder.ts:747-762` emits only) | 220–225 (NATIVE_OP_WORLD_ENTITY_1..6) + 214 (SET_HEADING) | implemented (phase 21): byte-exact decoders (all Reader transform forms, the SET_HEADING `& 15` mask), preamble + gate, then a pure hub emit (`onWorldEntityInteraction`, first-handled-wins) — no walk-to, no before→main→operated dispatch, exactly as TS; SET_HEADING routes to the helm-heading seam |

Ground/widget/inventory dispatches ARE split per listener
in `Handle` (phase 9): `routeInventoryAction`,
`routeGroundItemAction`, `routePickup`, and `routeWidgetAction`
mirror the TS `NetworkBuilder` routing branches, resolving by
option text/index, widget id, and container contents through
the `ItemActionSource` seam (ObjType action lists, with
defaults when no cache is present). `Statuses()` reports the
per-listener status per file above.

### Plugin hub (`internal/plugins`) + content (`internal/content`) — DONE (hub + wiring + 5 pieces)

Phase 10 ported the `PluginManager`/`PluginApi` SHAPE to
idiomatic Go, grounded in `PluginManager.ts`/`PluginTypes.ts`
with file:line cites; it did not mirror the 203-member
`PluginApi` / 191-member `PluginCoreApi` surface. The hub
(`internal/plugins`, which the `game` package never imports)
provides:

- Compile-time registration: content packages register a
  `Plugin` value (name, description, members flag,
  dependencies, config declarations, `Register` /
  `RegisterDeferred`) with the package-level registry; the
  server builds its hub from `DefaultPlugins()`. This is the
  deliberate deviation from TS `*.plugin.js` discovery: Go
  has no dynamic loading. Gained: type safety, dead-code
  elimination, no filesystem dependency at boot. Lost:
  drop-in content without a rebuild, and per-file identity
  (the `ID` field is the compile-time analogue of the TS
  filename key; the `--disablePlayerBots` file filter has no
  analogue and is a reserved no-op).
- Dependency-ordered loading with TS
  `loadPluginCandidatesWithDependencies` semantics:
  multi-pass, same-pass loads count, failed `register`
  never marks the name loaded, `registerDeferred` re-queued
  with cleared deps, no-progress break with unresolved-
  dependency warnings, exact unnormalized name matching.
- Config declarations validated BEFORE any plugin registers;
  bad/clashing declarations are logged and dropped, never
  fatal; core-owned keys win (the Go core declaration set is
  empty: the core reads no pluginConfig options today).
  World.json switches resolve by exported name, then file
  name (ID), then the plugin default; members-only plugins
  are skipped on F2P worlds. The scoped-settings "no
  declared key" warnings are NOT ported (the config package
  does not surface `ScopedPluginSettings`), and the legacy
  whole-list `disabledPlugins` form is not honored (the
  config package does not carry it; the hub supports it via
  `ParseDisabledPlugins`).
- Hook registry + emit: typed registrations for the 45
  families the Go engine can fire (server startup/shutdown,
  player login/logout/disconnect/process, command,
  social-packet, custom events, the object families:
  route, interaction + named overload, before-interaction,
  click + first..fifth click, item-on-object, the dialogue
  families: variant, condition, the zone families:
  zone-enter, zone-exit, the combat families:
  npc-death, npc-attack-option, player-attack, player-death,
  player-death-item-drop, player-death-drops,
  combat-hit-prepare/roll/damage/resolved,
  combat-attack-distance, the shop families: can-shop,
  can-stock-item, the trade families: can-trade,
  trade-request, trade-completed, the interface
  families: interface-action-click, interface-action-button,
  the skills families: player-level-up,
  experience-gained, can-gain-experience, and the
  world-entity family: world-entity-interaction)
  with per-hook error isolation (a panicking hook is logged
  with the TS message shape and isolated, never fatal).
  Lifecycle emits carry the TS emit guards (login/logout/
  disconnect require a non-nil player + non-empty username).
  The other 63 `on*` families are enumerated in the
  `hooks.go` table with the missing engine system named for
  each — no registration method exists for them yet, and
  none is silently dropped — along with the 95 non-hook
  `PluginApi` members (24 ported: `persistAttribute` as
  `Api.PersistAttribute` since phase 11, plus
  `registerCommand`, `getRegisteredCommands`, and
  `setCommandRights` since phase 12, plus
  `getObjectManager`, `emitObjectInteraction`, and
  `emitItemOnObject` since phase 13, plus
  `sendMultiChatboxPrompt` since phase 14, plus
  `registerArea` since phase 15, plus
  `emitNpcAttackOption` and `registerNpcCombatMethodProvider`
  since phase 16, plus `registerShopCurrency` and
  `registerItemShopCurrency` since phase 17, plus
  `refuseWhileBusy` since phase 18, plus
  `getTaskManager` since phase 20, plus `getAreaManager`,
  `replaceMapRegion`, `replaceMapRegionData`,
  `restoreMapRegion`, and `emitWorldEntityInteraction`
  since phase 21).
  Custom namespaced events (`domain:event`) subscribe/emit
  with the TS name rule enforced silently on both paths.
  `plugins:loaded` fires at the end of `Load()` with the
  load-ordered names; post-Load registration is refused with
  a warning (no TS equivalent; TS registers only during load
  passes by construction).
- Player attributes (phase 11): content declares persistent
  attributes through `Api.PersistAttribute` (a direct
  passthrough to the `persist` registry, panicking on invalid
  input exactly as the TS throw fails the plugin's register)
  and reads/writes them through the `*game.Player` methods
  (`Get/Set/HasAttribute`) on event payloads — no new `Core`
  accessor, keeping the `game`-never-imports-`plugins` rule
  intact. Attribute-key constants for the transient
  mobile/native client facts and `xp-drops:enabled` do not
  exist yet as Go constants (the keys are string literals at
  their single use sites); defining them belongs with the
  Settings content work that first writes them.
- Chat + commands (phase 12): content sends messages through
  `Player.SendMessage`/`SendTypedMessage` on event payloads
  (the session side is an optional `ChatMessageSender`
  extension interface, asserted — never a new `SessionPort`
  method — keeping `game`-never-imports-`plugins` intact)
  and registers commands through `Api.RegisterCommand` /
  `GetRegisteredCommands` / `SetCommandRights`, with
  `OnCommand` (any-command hooks) and `OnSocialPacket`
  (handled short-circuit) registration methods and
  production emit sites in the chat listener. Command rights
  are read from the carried save (no live `Player` rights
  field exists); the object-examine intercept is a custom
  event (`world:object-examine`), not a hook family. No chat
  or command state persists: registrations and overrides are
  process state, permissions are world.json config, mutes
  are the already-ported `PlayerPunishment` txt lists, and
  `Rights` is preserved verbatim in the save.
- Gate contracts, one helper per TS emit shape (no uniform
  rule): allow-veto (`onCanAttack`), first-answer
  (`onCanTeleport` + 15 inline equivalents), reason-gate
  (`onCanLogout`, default "You can't log out right now."),
  handled short-circuit (NPC interaction, social packets),
  dialogue first-answer, and veto-plus-accumulator (the
  `onCanGainExperience` XP gate since phase 20: every hook
  runs against one shared event, a deny returns immediately
  with the multiplier reset to 1, the final multiplier is
  sanitized finite-and-positive else 1). Run-all-then-read
  and two-phase emit shapes are documented as uncovered.
  The social-packet handled short-circuit has a
  production emit site (phase 12, the chat listener's
  social intercept), as does the XP gate (phase 20,
  `AddExperience` through the skills seam); the other
  gates have no production emit site, and the
  `World.LogoutRefusal` seam has no supplier (nothing in
  `cmd/server` assigns it); the coordinator supplies a
  logout gate through it when one is needed.
- Objects (phase 13): content registers object hooks
  through `OnObjectRoute` (stop-on-destination),
  `OnObjectInteraction` (plain + exact name/actions-map
  overload, false falls through), `OnBeforeObjectInteraction`
  (claim-by-`Handled`, skips mains), `OnObjectClick` +
  `OnObjectFirstClick`..`OnObjectFifthClick` (id validation/
  dedup, `!== false` claims), and `OnItemOnObject` (plain +
  named, noted/fallback filters), with production emit sites
  in the object-action / item-on-object listeners and the
  walk-to path; `emitObjectInteraction` / `emitItemOnObject`
  carry the TS emit contracts (definition/option fill-ins,
  no-hooks early return). The `getObjectManager` /
  `core.ObjectManager` / `core.MapObjects` surface is the
  `ObjectManagerAPI` interface (register/deregister/
  replace/queries/still-stands/shutdown cleanup) behind
  `Api.GetObjectManager` / `Core.ObjectManager()`, wired at
  boot over the world registry; `onSpellOnObject`,
  `setDeckReach`/`setUsableFromDeck` stay table-only (magic
  / sailing). No object state persists: the registry is
  runtime-only and spawns re-derive from JSON at boot.
- Dialogue (phase 14): content registers variant/condition
  handlers through `OnNpcDialogueVariant` /
  `OnNpcDialogueCondition` (first-answer wins,
  throw-and-continue, the TS emit contracts verbatim) with
  production emit sites in the transcript playback; the 8
  `npc-dialogue:*` custom-event names ride the existing
  custom-event surface (line/action/choice/hook/menu/
  condition/start + `npc:operated`, emitted by the NPC-click
  dispatch); `Api.SendMultiChatboxPrompt` ports the
  `sendMultiChatboxPrompt` member over the coordinator-
  installed prompt registry. Verified: `PluginApi` has NO
  dialogue members — `getDialogueManager` is a `Player`
  method, already ported as `Player.GetDialogueManager` —
  so no further member ports belong to this phase. The
  `npc-dialogue:shop` opening was RESOLVED in phase 17
  (the dialogue seams now open shops through the
  ShopManager); the static-binding registry
  (`registerPluginInteraction`) has no Go equivalent.
  Dialogue state does not persist (session-only).
- Timers + areas/zones (phase 15): content registers zone
  hooks through `OnZoneEnter`/`OnZoneExit` (one list, one
  entry per call carrying only enter or only exit, per-player
  active sets keyed by hook index, nil zone/handler silently
  ignored, warn-and-continue on throw), driven by
  `updatePlayerZones` FIRST inside `EmitPlayerProcess`,
  before the process hooks, even with zero process hooks;
  `isInsideZone` is the inclusive rectangle plus the levels
  test (nil levels = all planes, an empty non-nil slice =
  no plane, exactly as TS). Zone state is discarded on
  disconnect (the `WeakMap` analogue — no exit hooks fire).
  Content registers areas through `Api.RegisterArea` over
  the `AreaRegistry` interface (the `ObjectManagerAPI`
  precedent); areas registered before the world exists
  queue and flush in registration order when the boot
  wiring installs the registry, and a nil area panics (the
  TS throw analogue). The isolating `AreaMethodCaller`
  (per-call recover, TS `callArea` log shape, zero values
  on failure) is installed at boot; without it, area
  methods are called directly. Timers have NO `PluginApi`
  surface (verified by audit: `getTimers` is a `Mobile`
  method only) — content reaches them through
  `*game.Player` on event payloads, like attributes.
- Combat (phase 16): content registers death/hit hooks
  through `OnNpcDeath`/`OnPlayerDeath`/`OnPlayerDeathItemDrop`/
  `OnPlayerDeathDrops`/`OnCombatHitPrepare`/`OnCombatHitRoll`/
  `OnCombatHitDamage`/`OnCombatHitResolved`/
  `OnCombatAttackDistance`/`OnNpcAttackOption`/`OnPlayerAttack`
  with production emit sites in the combat core (the death
  and hit pipelines) and the attack listeners; the game-side
  `CombatHooks` seam carries them across the
  `game`-never-imports-`plugins` boundary, populated by
  `InstallCombatHooks`. `Api.EmitNpcAttackOption` ports the
  attack-option gate member, and
  `Api.RegisterNpcCombatMethodProvider` ports the provider
  member over a registry the combat factory consults, with a
  queue-until-installed seam for providers registered before
  the world exists. `onPlayerDeathItemDrop`/
  `onPlayerDeathDrops` gained producers in phase 28 (the
  death item-loss pipeline: `ItemsLost` is populated from
  `dropItems`, and a handled player-death claim suppresses
  the core respawn through the `player:respawn` event);
  `onShouldDropItemsOnDeath`, `onShouldKeepItemOnDeath`,
  and `onPlayerDefeated` are firable since phase 28 (hub
  emit + game seam + `InstallDeathWiring` in
  `cmd/server/combatwiring.go`; `emitPlayerDefeated` fires
  for ALL deaths). `onCanAttack` is firable since phase 26 (game seam +
  hub emit + production wiring; the legacy method-less
  `CanAttack` field is kept as a fallback until its test
  assignments migrate); `onNpcBeforeDeath`,
  `onPlayerBeforeDeath`, `onAttackTiming`, `onAttackSpeed`,
  `onNpcHitModify`, `onPlayerHitModify`,
  `onPlayerDealtDamage`,
  `onNpcAggressionTolerance`, `onPlayerOption`,
  `onItemOnNpc`, and `registerNpcCombatOption` stay
  table-only. (`onSpellDisabled`/`onSpellRuneBypass` became
  firable in phase 27; the list above was not updated then.)
  `getAreaManager` stays table-only (no narrow surface
  designed; zero TS callers outside the hub itself). No
  timer/zone/area state persists.
- Poison/venom + skulls + player death (phase 28): the
  `PoisonClock` (protection-dose counter, 30-tick pulse
  phase, 15/25-tick immunity windows, venom damage
  progression, reduce/spend semantics) and the
  `CombatPoisonEffect` (submit/resume/advance/finish, the
  6-in-11 NPC application roll, 25/50/75 venom tiers, the
  poison orb varp 102, `PoisonSaveState` with the
  258-entry script-extracted weapon/ammunition table) live
  in `internal/game/poison*.go`, with the effect ticking
  from `Combat.ProcessHitQueue` and the live poison fields
  round-tripping through the save (`cmd/server/savewiring.go`,
  `PlayerSave.ts:656-660` / `:764-771` order); the skull
  system (`internal/game/skull.go`: WHITE icon 0 / RED icon
  1, 30-minute timer ticking in the `Player.Process` slot,
  `ApplySkull`/`IsSkulled`/`ExtendSkull`, RED deactivates
  Protect Item) is wired through `InstallCombatHooks`; the
  player-death flow (`internal/game/playerdeath.go` +
  `respawnPlayer` in `combat_death.go`: death message +
  jingle 90, `onShouldDropItemsOnDeath` gate, `player:death-bones`
  + BONES 526 to the ground, `dropItems` with the kept-items
  rules (3 + Protect Item 1, red skull 0, skulled 0,
  `keepValue` = GE else high alch else floor(store × 0.6)),
  `resetAttributes` incl. the poison clear, kept items
  returned, `emitPlayerDefeated` for ALL deaths, save
  "death", the `onPlayerDeath` handled chain, `player:died`,
  `player:respawn` else teleport to the world spawn, save
  "respawn") is wired through `InstallDeathWiring` in
  `cmd/server/combatwiring.go`. The five death hub families
  are firable (51 of 108); poison and skulls add no hub
  surface (poison is reached through `Mobile` methods like
  timers; the skull trigger is the combat core's
  `handleSkull` call, which always applies WHITE — RED has
  no core call site).
- Shops + trading (phase 17): content registers shop/trade
  hooks through `OnCanShop`/`OnCanStockItem`/`OnCanTrade`/
  `OnTradeRequest`/`OnTradeCompleted` (TS gate contracts:
  `*bool` allows, trade-request short-circuits on handled,
  trade-completed runs all, fired twice per completed trade,
  once per player, before either player is saved) with
  production emit sites in the shop open path, the
  shop-definition load, the trade gates, the trade-request
  listener, and the trade completion path, carried across
  the `game`-never-imports-`plugins` boundary by the
  game-side shop/trade seams; the ten `shop:*` custom-event
  names ride the existing custom-event surface, and
  `trade:item-eligibility` rides it too (no hub constant —
  no such TS member). `Api.RegisterShopCurrency` /
  `Api.RegisterItemShopCurrency` port the currency members
  over a queue-until-installed registry (eager panics with
  the TS messages). The scoped core surface gains
  `Core.ShopManager()` (open/close/query/stock over the live
  manager), `Core.ShopDefinition()` (the definition
  registry), and `Core.TradeState(player)` (the read-only
  trade-state view); there is no `Api.GetShopManager` (no
  such TS member). No shop/trade state persists beyond the
  trade-escrow save fold and the `shop:points` /
  `shop:quantity-mode` attributes: shop stock rebuilds every
  boot, and the Grand Exchange marketplace + Duel Arena
  gameplay stay NOT STARTED (plugin content needing
  milestone 8 / duel gameplay respectively).
- Interface runtime (phase 18): content registers
  interface hooks through `OnInterfaceActionClick` (generic
  + per-button + named overloads) and
  `OnInterfaceActionButton` (per-button, with the
  `refuseWhileBusy` member ported as the TS
  `refuseWhileBusy` gate: buttons registered through it are
  refused while the player is busy), with the production
  emit site in the interface-action dispatch (prompt-first
  order, stop once handled, no-handler falls through to the
  dialogue tail). Content opens/closes interfaces through
  `Player.SendInterface`/`CloseInterface` on event payloads
  (the session side is an optional `InterfaceSender`
  extension interface, asserted — never a new `SessionPort`
  method — keeping `game`-never-imports-`plugins` intact),
  and `interface:closed`/`interface:changed` ride the
  custom-event surface. `registerCustomInterface` stays
  table-only (needs the HTTP ContentApi endpoint). No
  interface/varp state persists: varps, interface id, and
  prompt actions are session-only.
- Skills + experience (phase 20): content registers
  skills hooks through `OnPlayerLevelUp`/
  `OnExperienceGained` (run-all, error-isolated) and
  `OnCanGainExperience` (the veto-plus-accumulator gate
  above, `*bool` allow + stacking multiplier) with
  production emit sites in `SkillManager.addExperience`,
  carried across the `game`-never-imports-`plugins`
  boundary by the game-side `SkillsHooks` seam, populated
  by `InstallSkillHooks`; the `skills:experience-gained`,
  `skills:current-level-changed`, and `skills:restore`
  custom events ride the existing custom-event surface
  (value payloads, except `skills:restore`, whose pointer
  payload carries the handler read-back — see the skills
  notes). `Api.GetTaskManager` ports the `getTaskManager`
  member over the world's single `TaskManager`, installed
  at boot (nil until installed); `Schedule` is TS
  `submit`, so content runs session-tick tasks through it.
  The `SkillActions` helper (`internal/plugins/
  skillactions`, no hub import) ports `SkillActions.js`
  for content session loops. `core.Skill` /
  `core.SkillManager` and `getSkillManager` stay
  table-only (no `Skill` type in Go; the manager is
  per-player state reached through `Player.Skills()`).
  SkillLevelUp feedback, `skills:produced` /
  `skills:actions-per-cycle` / `skills:level-boost` /
  `skills:xp-locked` / `skills:combat-xp-locked` producers,
  and all individual skill content stay NOT STARTED. No
  skills state beyond the save's skill arrays persists:
  the skill slot and hook registrations are
  session/process state.
- Sailing + instancing + sounds (phase 21): content
  registers world-entity interaction through
  `OnWorldEntityInteraction` (no guard wrapper, first-
  handled-wins, with the production emit site in the
  world-entity dispatch) and `Api.EmitWorldEntityInteraction`
  ports the emit member; `Api.GetAreaManager` ports the
  query member as a narrow `AreaManagerQuery` interface
  (`Get`/`Inside`/`InMulti` — the engine-internal `Process`
  is excluded); `Api.ReplaceMapRegion` (incl. the pair
  form `ReplaceMapRegionFromFiles`), `ReplaceMapRegionData`,
  and `RestoreMapRegion` port the region-replacement
  members with the TS validation; the scoped core surface
  gains `Core.Sound()`/`Core.Sounds()` (named statics are
  NOT duplicated — content uses the `game.Sound*` vars),
  `Core.Projectile()`, `Core.MapRegionReplacementManager()`,
  `Core.ReadTemplateRegion()`/`Core.GetTemplateRegion()`,
  and `Core.Sailing()`/`Core.BoatMoveMode()` (the Sailing
  runtime is hub-installed, nil-safe, following the
  `TaskManager` precedent — no process-wide global).
  Sailing/private-area signalling rides the custom-event
  surface (`sailing:boarded`, `sailing:left`,
  `sailing:guest-boarded`, `sailing:guest-left`,
  `private-area:entered`); no `on*` families exist for
  sound/projectile/camera/private-area in TS, and none
  were invented. `templateChunkKey`/`rotateChunk*` stay
  table-only (no Core mirror, TimerKey precedent), as do
  `Graphic`/`GraphicHeight`/`Priority`.
- The `api.core` equivalent is scoped to what exists in Go:
  `Hub()`, `Logger()`, `Config()` (full keys verbatim),
  `Cycle()` (via the installed cycle source) — hub
  conveniences, not `PluginCoreApi` members — plus the 12
  ported `PluginCoreApi` members: `ObjectManager()`,
  `ShopManager()`, `ShopDefinition()`, `TradeState()`,
  `Sound()`, `Sounds()`, `Projectile()`,
  `MapRegionReplacementManager()`, `ReadTemplateRegion()`,
  `GetTemplateRegion()`, `Sailing()`, `BoatMoveMode()`.
  All 179 remaining `PluginCoreApi` members are listed by name in `core.go`,
  grouped, with the future engine system named per accessor.
  `PluginContext` is not ported (no thread-locals; errors
  label the plugin name). No perf profiling: hook entries
  omit `profileName`/`handlerName`.
- Login extensions: first-wins registration (a second
  registration panics, as TS throws), fail-closed creation
  (no recover on the factory path). `LoginExtension` is a
  placeholder with no callbacks — the TS callbacks take
  session/persist types this package must not import — and
  no production call site exists.

Boot wiring (`cmd/server/pluginwiring.go`, see the Wiring
notes) emits startup after the engine starts, shutdown after
the engine stops and before the shutdown saves are written,
login after world entry and before the bootstrap, logout
before the logout response, disconnect before the disconnect
save, and process once per non-bot registered player per
tick after `Player.Process` (behind the new
`WorldHooks.PlayerProcess` seam). Shutdown hooks that mutate
live player state are NOT reflected in the shutdown saves:
the snapshots were already taken before the engine stopped.

The proof content piece, `internal/content/dateminutes`, is
a faithful port of `DateMinutes.plugin.js` (varp 3078, login
+ process hooks, per-player dedup), live-verified (see "What
was verified"). Its two deviations are documented at the
site: a mutex-guarded map keyed by `*game.Player` with
disconnect cleanup stands in for the TS `WeakMap`, and the
send goes through the new `SessionPort.SendVarp` seam
(`Player.SendVarp` -> `Session.SendVarp` ->
`protocol.EncodeVarp`, porting `PacketSender.sendConfig`;
the PacketSender-side varp cache is not ported). One Go
package per content piece under `internal/content/<name>`;
see `CONTENT_MIGRATION_PLAN.md` for the measured inventory
(577 entries, 2,907 JS files, 789,355 lines),
engine-systems-first ordering, and porting rules.

Phase 22 ported the first two §3 content pieces (the
engine-systems plan was exhausted in phase 21).
`internal/content/gracefuloutfit` is a faithful port of
`GracefulOutfit.plugin.js`: the PIECES table (hood 3% −3kg,
top 4% −5kg, legs 4% −6kg, gloves 3% −3kg, boots 3% −4kg,
cape 3% −4kg), the 10% full-set bonus, the pure bonus
calculation, the signature-keyed bonus cache (a
mutex-guarded map keyed by `*game.Player` with disconnect
cleanup, standing in for the TS `WeakMap`), the
`skillcape:has-perk` query, and the `equipment:effects` /
`equipment:weights` listeners — members: true, attach-only
register. `internal/content/wornweights` is a faithful port
of `WornWeights.plugin.js`: the worn-weight id map (88→89,
10553→10554, 10069→10073, 10071→10074), the
iterate-all-slots loop, and the first-writer-wins finite
skip — no members flag. Both read names/weights through the
player's installed definition source (`Player.ItemName` /
`ItemDefinitions().WeightGrams`) rather than a Core
`ItemDefinition` surface, which stays table-only. Their
event inventory is documented in each package comment:
`equipment:weights` has a Go producer (the engine weight
calculation) and both pieces answer it;
`equipment:effects` has NO Go producer (the EquipmentStats
plugin is not ported), so GracefulOutfit's effects handler
is registered but never fires; `skillcape:has-perk` has NO
Go answerer (the SkillCapes plugin is not ported), so the
Agility-cape substitute path never triggers in production —
the honest no-hooks default. The hub gained the two
grounded register members
(`RegisterRunEnergyRestoreModifier`,
`RegisterRunEnergyDrainModifier`), 26 of 95 non-hook
members; the hook-family count is unchanged (45 of 108
firable).

Phase 23 ported the third §3 content piece.
`internal/content/lumbridgeguide` is a faithful port of
`LumbridgeGuide.plugin.js` + `Dialogue.LumbridgeGuide.js`:
the page/variant constants verbatim, one
`onNpcDialogueVariant` registration whose selector returns
nil when no page entry matches "Lumbridge Guide" and
otherwise emits the `lumbridge-guide:amulet-status` query
and picks the LOST variant when the mutable request comes
back with `reclaim` set, else NORMAL — no members flag,
attach-only register. The shared request type
(`LumbridgeGuideAmuletStatusRequest`) lives in
`internal/game` next to the `SkillcapeHasPerk` precedent,
with the unported GOTR answerer grounded at the site; the
JS module-level `pluginApi` global becomes a struct field
holding the emit function, the only deviation, documented
at the site. Its event inventory is documented in the
package comment: the piece emits exactly one custom event,
`lumbridge-guide:amulet-status`, which has NO Go answerer
(the Guardians of the Rift Amulet unit is not ported), so
in production the query goes unanswered and the NORMAL
variant always plays — the designed default, per the
upstream unit's own header comment. No new hub surface was
needed: the hook-family count (45 of 108 firable), the
non-hook member count (26 of 95), and the Core count (12
of 191) are all unchanged. The guide's dialogue entries
are NOT in the repo's `{}` starters: with starter data
Talk-to yields no pages (existing Phase 14 behavior); the
entries needed are documented in PHASE23_PLAN.md and
nothing was copied.

Phase 24 ported the fourth §3 content piece.
`internal/content/seekingarrows` is a faithful port of
`SeekingArrows.plugin.js`: one `onCombatHitDamage`
registration whose handler follows the TS guard order
(accurate, player attacker, RANGED combat type, no active
special, ammunition identity) and raises the damage to at
least 3 via the shared `*game.HitDamage` — members: true,
attach-only register. No weapon/bow check exists upstream
and none was added. The port needed one minimal grounded
seam, `Mobile.IsPlayer()`/`AsPlayer()` in `internal/game`
(backed by the existing `combatantFor` identity mechanism,
mirroring TS `Mobile.isPlayer()`/`getAsPlayer()`,
fail-closed for unbound/nil mobiles); `isNpc`/`getAsNpc`
stay unported. Its event inventory is documented in the
package comment: the piece listens to `onCombatHitDamage`
only and emits nothing. Two deviations forced by the
language change are documented at the site: `Damage`
arrives as `any` and is type-asserted to `*game.HitDamage`
(fail-closed otherwise), and the ammunition identity check
compares `ItemID` against 33595 (the TS singleton has no Go
form; the noted form 33596 does not match). No new hub
surface was needed: the hook-family count (45 of 108
firable), the non-hook member count (26 of 95), and the
Core count (12 of 191) are all unchanged. The handler could not fire in production
until ranged weapon/ammunition data was ported: no
ranged-data resolver was registered (the phase-16 seam left
unset — see the combat notes), so every fighter attacked in
melee; phase 26 ported the tables and registered the
resolver, so the handler fires in production (live-verified:
seeking-arrow hits floored at 3 with a production resolver).
The live runs proved the pipeline plus handler end to end
with a test-only resolver in the throwaway harness, removed
afterwards.

Phase 32 ported the first skill, Woodcutting (Tyler's
pick, 2026-11-19, over the survey's recommended
BarbarianPyres), as the core chopping loop only.
`internal/content/woodcutting` is a faithful port of
`Woodcutting.plugin.js` (1,594 lines): the §1 tables
verbatim with TS `file:line` citations (30 AXES — the
plan said 31, wrong, the plugin has exactly 30 `axe()`
calls; 14 CUT_CHANCE charts; 21 TREES; 24 stump-model
mappings — the plan said 25, wrong; BirdNests/Seeds/
Rings; Lumberjack; constants), gameval-name (string)
source tables resolved through `Load`/`LoadFromCache`
(fail-closed, every unresolved name listed; 393 distinct
names resolved against the rev-242 cache, 0 failures),
the 1-tick global WoodcuttingTask driving per-player
sessions, the 4-tick roll cadence with the first roll at
+2 ticks, axe selection, cut chance, group bonus,
felling/ration energy, the lumberjack multiplier, the
Seers maple diary query, Kandarin headgear log-doubling,
bird nests (1/256, members-only, pre-empting the log),
depletion/respawn (Forestry timers, stump swap, client
script 5474 overlay), the `level-up:popup-opened` pause,
`persistAttribute("woodcutting:logs-chopped", 0)` with
the logs-chopped varp 4525, and 27
`OnObjectInteractionNamed` registrations plus the nest
search on `OnItemFirstAction` — members: true,
attach-only register, one blank import in
`cmd/server/pluginwiring.go`. The port needed two
grounded seams: the `onItemAction` hub family
(`OnItemAction`/`OnItemActionNamed`/`OnItemFirstAction`/
`OnCanUseItem` in `internal/plugins/itemaction.go`, emit
sites in `internal/listeners/itemaction.go`, 54 of 108
families firable) and `ObjectManagerAPI.Get`
(`MapObjects.get`, `MapObjects.ts:94-122`) so content can
see untouched map objects — `ObjectsAt` sees runtime
registrations only. Boot wiring
(`cmd/server/woodcuttingwiring.go`): the cache and
definitions load BEFORE the hub, `SetTables` runs before
hub Load, members gating reads the cache `IsMembers`
flag, and a table-load failure fails boot. Two live bugs
found by the first smoke run and fixed in-phase (see
"What was verified"): `treeAt` never found map trees
(the Get seam), and Register-time hub-surface captures
read nil. The lasting lesson is a hub contract: surfaces
installed after plugin Load (TaskManager, ObjectManager)
must be resolved lazily at use time or on server startup,
never captured in Register — and the TaskManager
additionally allows `Schedule` only from the tick
goroutine, so the 1-tick task schedules on the first
chop, not on startup. The deferred sub-units landed in
phase 33 (Guild excepted — see the Phase 33 paragraph
below); `woodcutting-guild.json` was NOT imported (needs
Tyler's approval). Documented deviations at the time:
XP integer truncation (37.5 → 37), interaction distance
Chebyshev ≤2 vs the TS object footprint, stump-less trees
falling to default stump 1342, no ForceMovement tick-skip,
no face-tree step, `equipment:effects` producerless (the
Lumberjack set-text handler is registered but never
fires), and no SkillLevelUp emitter yet (the level-up
pause never fires). The distance, stump, and face-tree
deviations were closed in phase 33; the rest remain —
see gap 16. No data file was imported; the non-hook member
count (29 of 95) and the Core count (12 of 191) are
unchanged.

Phase 33 finished Woodcutting (all sub-units except the
Guild). `internal/content/woodcutting` gained `guilddata.go`
(the JSON `ents` + `shrine` sections transcribed into Go
tables with JSON line citations — the file itself was NOT
copied into the repo), `enttrunk.go` (Ent death → remains
{103 ticks, respawn 50 guild / 15 wilderness} → transform
at +2 ticks to trunk 9474, 1 HP locked → killer-only
chopping, 3-tick rolls, 25 XP, 1 noted log guild / 2
wilderness, bird nests, `woodcutting:ent-logs` emit),
`shrine.go` (bird's egg on Shrine 29088 → seed nest 22798
or an evil-chicken piece at 1/1200 each, +100 Prayer XP,
`woodcutting-guild:shrine-offerings` attribute, anim 3705,
per-egg projectile + sound), `thickvines.go` (Tar Swamp
chop/clear: any axe, no level gate; rake clear gives
Farming XP 2; 2 XP either way; 49-tick regrow; pending map
keyed by loc), `cluenests.go` (clue rolls per tier off
`petBase`, Wilderness ring doubling, `ownsClue` inventory +
bank scan, open-vessel default path; the
`treasure-trails:*` emits go unanswered and degrade to
documented defaults; the box-true `forceAdd` path has no Go
form and is a gap), `crystalaxe.go` (10,000 start / 20,000
max charges in item meta, shard recharge 100 each, 10%
signet save, empty → inactive, Check), and `infernalaxe.go`
(1/3 burn, 5,000 charges, smokepuff 86 @ 150 + fire sound
2596, Check; the burn never fires until Firemaking lands —
`firemaking:log-xp` has no answerer yet, wired anyway).
The burn and crystal drains replaced the Phase 32 stubs in
the core success block, and the clue-nest roll was wired in
after the `woodcutting:success` emit. Three seams: the
`onNpcClick` family (`internal/plugins/npcinteraction.go`,
first-claim-wins at the head of the emit), the
`onItemOnItem` family (`internal/plugins/itemonitem.go` —
grounded finding: there is NO separate item-on-item opcode,
it rides INVENTORY_USE_ON 241 kind 4, and the Go decode
already handled kind 4, so only the route + family + adapter
were built; no `Api.EmitItemOnItem` was invented — no such
member exists in `PluginTypes.ts`), and the NPC
remains-delay (death hooks setting Remains defer removal
by RemainsTicks while the NPC stays registered) + transform
seam (`ContentID`, `SetNpcTransformationID`, HP lock).
Core-loop residuals closed by the coordinator: the stump
model-map + treeId+1 legs (memo computed at load from the
cache definitions; cross-checked against the real cache —
stump 1342 for tree 1276, all 273 tree object ids memoised),
the footprint interaction distance
(`GameObject.ts:151-155`; the normal tree is 2x2 in the
cache, so the old Chebyshev check was one tile stricter at
the far edge), the face-tree step (new `Player.
ForcePositionToFace` seam; the sync encoder does not yet
send player face updates), and the `treeAt` removed-objects
leg (`RemovedObjects()` on `ObjectManagerAPI`,
`World.ts:1099-1101`). The Guild stays out pending Tyler's
data-import approval. No data file was imported. Hub: 56 of
108 families firable (60 methods, 4 named overloads), 52
table-only; non-hook members 29 of 95, Core 12 of 191,
unchanged. The six sub-units are unit-covered only live —
no Ent kill/chop, vine clearing, clue roll, or crystal/
infernal Check was observed over the wire.

Phase 36 ported Cooking (the fourth skill, F2P, Tyler's
pick 2026-11-20). `internal/content/cooking` holds
`tables.go` (the `cooking.json` loader — `Load` /
`LoadFromCache` via the definition Store — plus the
transcribed knife fallbacks and `itemIDConstants` with TS
line citations; 122 cookables, 47 recipe rows, 2 wines, 4
dough products, churn inputs 4 / products 7, 369 item names
resolved with zero missing gamevals, load-proven against
the real cache, temporary test deleted), `common.go`
(`successChance`, `cookingLevel`, `actionsPerCycle`,
`wearsCookingCape`, `knives()`, `addXP`, `run`, `spam`,
`mesbox`, `sendVarbit`, `sendCreationMenu`, and the
`cooking:raw-xp` / `cooking:cooked` answerers),
`recipes.go` (`makeOne`, `startRecipe`, `offer`,
`offerDough`, `doughRecipe`, `waterFor`, `useItemOnItem`,
`prepareGnomebowl`), `cookers.go` (the core loop:
`sessions`, the ONE global 1-tick task, `cookTick` /
`cookStep` / `cookOne` in exact TS order, CreationMenu mode
6, gates in exact TS order, Hosidius +12, gauntlets, cape,
deliberate burn, karambwan, `countCook` + `cooking:count`),
`wine.go` (make via `Recipes.offer`, `pendingWine`,
`fermentWine` carried + banked, `queueFerment` 20 ticks,
`resumeFermenting`, `forget`), `churn.go`, `sacred_eel.go`
(MIN_LEVEL 72, band `min(4, (level-72)/8)`, scales
`3+band+rand(0..2)`, XP `100+3×scales`), `leaping_fish.go`
(tick source `Api.Core().Cycle()`), `gnome.go` (COCKTAILS /
BATTA / CRUNCHY menus, FINISH, FRUIT, `plan` / `apply`,
`interfaceAction` on `OnInterfaceActionClick`), and
`types.go` (the shared request/event types). Wiring:
`cmd/server/cookingwiring.go` (`installCookingTables` —
members-world flag, cooker ids, members lookup, object
info, gameval resolver; fails the boot on table/load
failure) + `pluginwiring.go`. Hub unchanged: 57 of 108
families firable (61 methods, 4 named overloads), 51
table-only; non-hook members 29 of 95, Core 12 of 191.
No data file was imported in this phase (`cooking.json`
was imported by the parent before the phase began); the
phase built its loader. Coordinator fix: the load-prove
caught a boot-blocking bug — `LoadCookerIDsFromCache`
resolved `poison_karambwan` / `cooking_gauntlets` as cache
gamevals and SacredEel resolved `sacred_eel` /
`zulrahs_scales` the same way, but the rev-242 cache has
no such gamevals (TS reads all four through `core.ItemIds`);
they now come from `itemIDConstants`, transcribed from
`ItemIdentifiers.ts`. Known degradations, all documented
at the port sites: XP integer truncation (structural,
shared with Woodcutting/Fletching/Fishing); unanswered
queries all fail-closed (`skills:level-boost` → 0,
`skills:actions-per-cycle` → 1, `diary:is-complete` →
false, `quest:is-complete` / `quest:is-started` → false,
`hunter:rumours-completed` → 0, `skillcape:has-perk` →
false); the Lumbridge Castle range (object 114) is refused
without Cook's Assistant (`quest:is-complete` has no Go
answerer); `duringQuest` / `rumours` foods never cook;
Hosidius bonus 12, never 25; the infernal cook is
unit-covered only (`TestTryCookFishFiresAnswered` +
`TestAnswerRawXP` — never observed live); no SkillLevelUp
emitter exists (same gap as Woodcutting/Fletching/Fishing),
so `level-up:popup-opened` never fires; no ForceMovement
check (no such system in Go); LeapingFish/Churn offcuts
ground-drop via `DropGroundItem` (no `forceAdd` — the Wiki
outcome, unconfirmed in TS); wine banked leftovers go to
the first tab with a free slot (no Go `GetTabForItem`);
`cooking:cooked` has no Go emitter. Live: 3 modes × 3 runs,
0 failures (see "What was verified").

Phase 35 ported Fishing (the third skill, F2P, Tyler's
pick 2026-11-20). `internal/content/fishing` holds
`tables.go` (the transcribed fish/tool/spot tables with
TS line citations — 50 fish, 24 tools, 7 spot options,
29 spot NPCs, 29 clue-bottle fish, 84 distinct item ids,
84 gameval names, 0-entry fallback: every "missing"
lowercase guess has a true Jagex gameval, so no
`item_ids.go` fallback table was needed; all 84 ids +
29 spot NPC ids load-proven against the real cache,
temporary test deleted), `camdozaal.go`, `fishing.go`
(the core loop: `startFishing`, `hasToolRequirements`
in exact TS order, `catchChance` with the single floor
of the sum, `rollCatch`/`rollTable`, `landCatch`,
`inventoryFull` sound 2277 + statement; the 1-tick task
exactly per Woodcutting; Register attaches
`OnPlayerDisconnect`, `level-up:popup-opened` via
`skillactions.PauseForLevelUp`, and `OnNpcInteraction`
for both spot names × all 10 options), `ids.go` (the
39 gameval names for the sub-units, all probe-verified
against the real cache), `angleroutfit.go` (XP multiplier
hat .4/top .8/waders .6/boots .2% + .5% set), `crystalharpoon.go`
(10,000/20,000/100, signet 10% save, drain, shard recharge),
`infernalharpoon.go` (COOK_CHANCE 3 / MAX 5,000), `karambwan.go`,
`radasblessing.go` (tiers 4→1 at 8/6/4/2%), `spiritflakes.go`
(50%), `cluebottles.go` (tiers 2/17/20/33/100 tenths, 95% CA
adjustment, Wilderness imbued-ring ×2), `guild.go` (`isInGuild`
+ invisible boost +7), and `world.go` (shared Wilderness
handle). The onNpcInteraction engine gap is closed by a new
seam: `internal/plugins/npcinteraction.go` (`Api.OnNpcInteraction`
+ named-hook registry + `Hub.EmitNpcInteraction` running
onNpcClick bindings first, then name hooks) and the
`cmd/server/dialoguewiring.go` adapter (fills
`DefinitionName`/`Action`/`Actions`). Wiring:
`cmd/server/fishingwiring.go` (`installFishingTables` +
`installFishingWilderness`, the latter called from
`buildServer` right after `newAreaManagerAndWilderness` —
the Wilderness does not exist until then and the handle is
only read at catch time, so installing it before the engine
starts is in time) + `pluginwiring.go`. Hub: 57 of 108
families firable (61 methods, 4 named overloads), 51
table-only; non-hook members 29 of 95, Core 12 of 191,
unchanged. No data file was imported in this phase;
Fishing has none (survey §5 row, confirmed by the full
read) and the tables were transcribed, not copied.
Known degradations, all documented at the port sites:
XP integer truncation (structural, shared with
Woodcutting/Fletching); quest gates (Swan Song /
Barbarian Training) always pass — no QuestRuntime exists
yet, and TS's `questAllows` fails OPEN for unregistered
quests, so this is TS-faithful; `cooking:raw-xp` has no
answerer, so the infernal cook never fires (pinned by
test: 200 attempts → all false, 0 charges, 0 Cooking XP;
it fires the moment Cooking lands); clue bottles fall to
the ground, never into the inventory — Go has no
`forceAdd`, so the box-true path is not built (Woodcutting
clue-nest precedent); the Angler set text never fires
(`equipment:effects` has no producer); `level-up:
popup-opened` is never emitted (no SkillLevelUp emitter);
`diary:is-complete`, `fishing:continue`, `fishing:tool`,
`fishing:interval`, `fishing:catch-chance`, `fishing:attempt`,
`fishing:catch`, `fishing:success`, `fishing:junk`, and the
`storage:` queries have no answerer yet (documented
defaults); the session holds the NPC pointer, not the
world NPC index — stricter than TS (a respawned spot
never resumes an old session); `variant.baitless` is
unexpressible (no Baitless field on `ToolVariant`); no
ForceMovement check (no such system in Go). Deferred to
the next phase's brief (plan §7, unchanged):
MinnowPlatform (needs an NPC-relocation seam), the
AnglerOutfit Gita exchange (needs `onItemOnNpc`,
still table-only), and the Guild door (needs
ObstacleRunner / ForceMovement). Live: 4 modes × 3 runs,
0 failures (see "What was verified").

Phase 34 ported Fletching (the second skill, members-only,
Tyler's pick 2026-11-20). `internal/content/fletching`
holds `tables.go` (the `fletching.json` loader —
`LoadFletching` via the definition Store — plus the
transcribed recipe/tool/alternative/menu/group/diary tables
with TS line citations; 170 recipes, 13 groups, 8 menus,
4 diary tasks, 265 distinct item names, 278 resolved ids:
102 via lowercase gameval, 176 via the fallback table,
3 of those pinning gameval mis-resolutions — opal/pearl/
barbed bolts resolve to the unenchanted ids 879/880/881,
not the enchanted 3818/3822/3826 — and bow-string spool
31052, which has no working cache gameval; all 68 sequence
names, 3 sounds (CUTTING 2605, CROSSBOW_LIMBS 2922,
GEM_CUTTING 2586) and 3 varbits resolve against the real
cache), `item_ids.go` (the 176-entry fallback table),
`diaries.go` (4 diary tasks on `fletching:made`), `shared.go`
(the full Shared loop: `handlePair` recipe resolution with
the TS exact priority — `findBestRecipe` then
`bestRecipeByItemIds`, name-only exact for tools, the
level-gate + members check + `onCanFletch` gate, the
5-tick batch interval, the 3-tick ticks-per-item for
non-feather recipes, item-level gate checks on every
batch, the 1-tick `lastItem` varp 2673 update, the
`fletching:made` emit after every batch, the
`crafting:spun` emit, and the CreationMenu open/close
flow), `recipes.go` (the generic item-on-item handler
plus the Celastrus-bark Fletch action), `spool.go` (fill/
check/empty + `fletching:spool-charges` attribute), and
`fletching.go` (unit registration, `members: true`).
The CreationMenu engine gap is closed by a new seam:
`internal/game/creationmenu.go` (menu construction from
`CreationMenu.ts:44-97` + `PacketSender.ts:1370-1418`) and
`internal/session/creationmenu.go` (the sender primitives),
the `routeDialogueResume` creation-menu pre-branch
(`internal/listeners/npc.go`), and `StopSkillable` clearing
(`internal/game/skills.go`). Two plan corrections landed
during the phase, both verified against the TS encoders:
script 2046 travels on opcode 110 (`WIDGET_RUN_SCRIPT`,
4-byte id — `encodeWidgetRunScript`, `ClientProtocol.ts`),
NOT opcode 170 (`RUN_CLIENT_SCRIPT`, 2-byte id); and the
menu mount must NOT route through `SendChatboxInterface`
(TS `sendCreationMenu` never calls it — it uses
`setChatboxUnclamped` + display-state + sub-interface
directly — routing through it would wrongly set
`chatboxGroupId = 270` and emit `interface:closed`; a test
pins `IsChatboxInterface(270) == false` after send). The
frame-byte test compares all 8 queued frames against
hand-computed bytes (2379, varbit 10670=1, modal unhide,
open-sub 270, two flags ranges, varp 2673=1, the full
165-byte 2046 frame). Hub: 56 of 108 families firable
(60 methods, 4 named overloads), 52 table-only; non-hook
members 29 of 95, Core 12 of 191, unchanged — Fletching
rides the Phase 32/33 `onItemAction` + `onItemOnItem`
families and needed no new hook. No data file was
imported in this phase; `fletching.json` was imported by
the parent 2026-11-20 with Tyler's approval and this phase
built its loader (load-proven, temporary test deleted).
Known degradations, all documented at the port sites:
XP integer truncation (structural, shared with
Woodcutting); quest/Slayer-gated recipes locked
fail-closed; `crafting:spun`, `level-up:popup-opened`,
`fletching:shafts`, and `diary:task` have no producer or
consumer yet; no SkillLevelUp emitter; the force-movement
task stop is unported (no Go seam — the same Woodcutting
gap); spool messages are TS's own (no rsprox capture).
Live: 3 runs × 11 checks, 0 failures (see "What was
verified").

Phase 25 ran the first-quest survey (migration plan §3 item 5
prerequisite) as analysis only: no Go code changed, no quest
ported, no quest picked. `QUEST_SURVEY.md` (repo root) holds the
re-measured inventory (193 quests, 261,277 lines of quest files;
195 registered on a members world), the whole-catalogue sweep (25
of 194 files clean of combat, instancing, and cross-quest flags),
the QuestRuntime engine analysis with the minimal subset a first
quest needs, the Go hook-surface mapping, a ranked shortlist of 5
quests read in full, and the recommendation (Doric's Quest) — the
pick is Tyler's. Verification counts are unchanged from phase 24
(3,799 cases, 17 packages, race clean).

## Known gaps (explicit, not silent)

1. A client can render the world around the spawn after login
   (rebuild + initial sync + a sync frame every tick, verified
   live), walk around it with real collision, SEE spawned NPCs
   standing in the world (verified live), SEE spawned objects
   standing in the world and click them through the hub's
   object-interaction hooks (verified live, phase 13), hear
   area music as it moves (verified live), see ground items
   (verified live),
   equip, drop, pick up, and reorder items with live
   containers/skills/appearance/weight (verified live), chat
   with nearby players and run hub-registered commands
   (verified live, phase 12), examine NPCs/objects/items
   (verified live), TALK to NPCs through the transcript
   dialogue system (verified live, phase 14), and react to
   timers, zones, and gameplay areas through hub hooks
   (verified live, phase 15), watch NPCs wander, aggress,
   and fight with hitsplat sync, kill them, and see them
   respawn through hub death hooks (verified live, phase 16),
   and buy from shops, sell to shops, and trade with other
   players (verified live, phase 17), and open interfaces,
   click widget buttons that dispatch to hub hooks, answer
   entered-amount prompts, and watch interfaces close on
   movement (verified live, phase 18), and hear sounds and
   jingles, see projectiles, board boats with world-entity
   sync, and enter templated instances (verified live,
   phase 21), and be poisoned by poisonous NPCs with the
   orb and damage ticks (verified live, phase 28), carry a
   skull timer that counts down (verified live, phase 28),
   and die with item loss and kept items, respawn at home,
   and leave bones on the floor (verified live, phase 28) — but the only content
   is the date_minutes varp sender (phase 10), the graceful
   outfit + worn-weights effects (phase 22), the Lumbridge
   Guide dialogue variant selector (phase 23), and the engine
   systems' built-in behaviors.
2. RESOLVED (phase 5): saves round-trip on all three backends
   (JSON, SQLite, memory) — a player who walks away from spawn
   and logs out resumes at the walked-to tile, with
   shutdown/autosave/disconnect saves writing. `requestSave`
   coalescing and offline admin ops are NOT STARTED.
3. RESOLVED (phase 7): run-energy drain/restore, the
   destination-flag packet, and the run-toggle listener are
   ported and live-verified — running now tires you out.
   RESOLVED in part (phase 15): the FREEZE/STUN mobility
   gates are ported (timers now exist). RESOLVED in part
   (phase 16): the combat animation-stall gate is ported
   (combat now exists) and NPC drivers exist (wandering,
   aggression, following, retreat, respawn). RESOLVED in part
   (phase 17): the trade gates are ported (movement/combat/
   prayer gates read the Trading-derived status) and the
   duel-order shuffle exception is ported (duelling players
   keep their process-order slots). Movement gaps
   remaining: placement/force-movement gates (need
   teleport/login paths, force movement — see the Game
   notes), no
   dynamic occupancy, and
   `syncWildernessStateForMovedPlayer` (wilderness level +
   "Attack" option on movement) is NOT STARTED. No
   skills content or content beyond the date_minutes varp sender.
4. RESOLVED (phase 8): the deferred flush blocks are ported —
   music area tracking + song sending, the ground-item manager
   + sync, and the NPC-specific-effects queue — all verified
   (music and ground items live over the wire, effects through
   real `FlushTick`). Residual gaps inside those blocks:
   shuffle/unlock depend on plugin events that do not exist
   (the TS fallback runs instead). Jingles were RESOLVED in
   phase 21 (wired + live-verified), and the NPC-effects queue
   gained its first producer in phase 21
   (`sendNpcSpotAnimationSpecific`). The
   area `getMusicTrack` override was RESOLVED in phase 15
   (wired through the area system). Ground-item pickup/drop
   was RESOLVED in phase 9 (see gap 10).
5. Scrypt parameters are verified against Node's `crypto.scrypt`
   defaults via golden vectors, not against a real production
   TSPS password hash.
6. RESOLVED (phase 6): custom-definition overlays
   (`registerCustomDefinitions`/`registerCustomItems`) are
   ported with TS precedence. RESOLVED (phase 14): dialogue
   data loading is ported (`npc-dialogues.json`,
   `npc-dialogue-index.json`, `event-dialogues.json`),
   validated read-only against the real dir;
   `dialogue-types.json` has no loader (grounded gap, pet
   content only) and retired-NPC successors stay empty
   (plugin-private data). RESOLVED (phase 17):
   `ShopManager.initialize()` is ported (shop definitions in
   TS load order with the `onCanStockItem` veto). RESOLVED
   (phase 21): `Sailing.initialize()` is ported (type/dock
   registries, idempotent init, boarding rules, boat
   manager, sailing state).
   `NpcAnimationScanner` is NOT
   STARTED; spawn activation is eager (TS defers most spawns
   per-region near players); `key`-based spawns are skipped
   (generated `NpcIdentifiers` never ported).
7. RESOLVED in part (phase 19): 12 of the 18 packet listener
   files are implemented (equip, drop, switch-slot, item
   actions, pickup, chat, command, object action, the
   item-on-object paths of UseItem, NPC option incl.
   Talk-to dialogue dispatch + the combat attack option,
   trade request incl. trade actions, and interface-action
   dispatch incl. the shop widget branch)
   plus 1 partial (PlayerOption: attack option 1 + trade
   option 2) plus the dispatch-inline `varp_transmit` run
   toggle, the dispatch-inline examine cases, the
   dispatch-inline dialogue_continue resume, the
   dispatch-inline entered-amount/syntax prompt answers,
   the dispatch-inline interface-close case, and the
   dispatch-inline social-packet cases (phase 19), and the
   dispatch-inline world-entity interaction cases (phase
   21: opcodes 220–225 + 214, emit-only, no TS listener
   file); the
   remaining 5 decode and drop
   with named reasons (see the per-listener table), because
   every target system (following, spellcasting,
   appearance-change) is NOT STARTED
   and the hook families those listeners need have no emit
   sites (the hub itself is DONE since phase 10, and 53 of
   108 families fire since phase 28, which added the five
   death families).
   Listener packets that need a walk now route it through the
   walk-to/arrival machinery (pickup, object action,
   item-on-object, NPC option, and trade request via
   walk-to-entity).
8. The rev-242 OpenRS2 entry ships zero XTEA keys, so rebuild
   packets carry all-zero keys (verified: map data still decodes).
   Real-key flow is unit-tested only.
9. DECIDED 2026-11-15 (Tyler): plugin content is ported Go-native,
   piece by piece, behind a Go hook registry — no embedded JS
   runtime, no Node sidecar. The full 790k LOC upstream content
   library is therefore out of scope as a wholesale port; content
   is selected and ported incrementally, with the engine systems
   each piece needs built first. RESOLVED (phase 10): the plugin
   hub and content migration plan are DONE (see the Plugin hub
   notes and `CONTENT_MIGRATION_PLAN.md`); 1 of 577 content
   entries is ported (7 of 577 after phase 34: DateMinutes,
   GracefulOutfit, WornWeights, LumbridgeGuide, SeekingArrows,
   Woodcutting, Fletching). RESOLVED (phase 11): player attributes
   exist in Go (live store on `Mobile`, persistent registry,
   owned save state, hub `persistAttribute`), live-verified
   end to end — plugins can now store state on players.
   RESOLVED (phase 12): chat + commands exist in Go (chat
   encoder, `sendMessage`, public-chat broadcast, command
   registry + hub `RegisterCommand`, examine answers),
   live-verified end to end — players can now chat and run
   commands. RESOLVED (phase 13): the object registry +
   object interaction exist in Go (registry, loc encoders +
   object sync, object-action/item-on-object listeners with
   walk-to-object, hub object families + `getObjectManager`),
   live-verified end to end — objects are now visible and
   interactable. RESOLVED (phase 14): dialogue exists in Go
   (DialogueManager + data loading + transcript playback +
   MultiChatboxPrompt + NPC-click dispatch + hub dialogue
   families), live-verified end to end — NPCs can now talk.
   RESOLVED (phase 15): timers + areas/zones exist in Go
   (timer repository + tick-phase firing, code-registered
   areas + membership evaluation, Wilderness helpers, hub
   zone families + `registerArea`), live-verified end to
   end — content can now schedule timers and react to zone
   and area enter/exit. RESOLVED (phase 16): NPC drivers +
   combat exist in Go (movement coordinator, aggression +
   tolerance, following, respawn, attack cycles, damage
   formulas, hit queue + hitsplat sync, death/respawn,
   prayer, specials, combat XP, attack listeners, hub
   combat families + combat-method providers),
   live-verified end to end — NPCs now move and fight, and
   content can hook deaths, hits, and combat methods.
   RESOLVED (phase 17): shops + trading exist in Go
   (ShopManager + shop flows + Trading + Duelling state +
   trade gates + hub shop/trade families + currency
   registration + trade-escrow save fold), live-verified end
   to end — players can now buy, sell, and trade, and
   content can gate shops, veto stock, register currencies,
   and hook trades. RESOLVED (phase 18): the interface
   runtime exists in Go (interface state + close gates,
   mount/sub-interface/chatbox/tab layering, layout open
   dispatch, action dispatch to hub hooks, entered-amount/
   syntax prompts, the live varp/varbit store), live-
   verified end to end — content can now open interfaces,
   receive widget clicks, prompt for amounts, and read/write
   varps. RESOLVED (phase 19): clans/social exists in Go
   (whole `PlayerRelations` port + live chat gate + social-
   packet decode/dispatch to the hub in all 8 variants +
   friends-chat-snapshot/clan encoders + relations owned
   save state + logout presence-offline), live-verified end
   to end — content can now manage friends/ignores, gate
   chat on relations, and receive every social packet; the
   friends-chat channels + clan system themselves are
   plugin content, not engine. RESOLVED (phase 20): skills
   shared handlers + XP sources exist in Go (the XP gate +
   world rate, the level-up / experience-gained hub
   families, the `skills:*` custom events, the skill slot,
   the `setLevel` guards, skill metadata, the
   `SkillActions` helper, and `getTaskManager`),
   live-verified end to end — content can now gate and
   multiply XP, react to level-ups and gains, hold
   sessions through level-up pop-ups, and pace actions
   through the interval helper; individual skill content
   still ports separately, piece by piece. RESOLVED (phase
   21): sailing/template regions/sounds exist in Go (the
   sailing engine + boat manager + deck areas + world-
   entity sync, private areas + templated instances +
   template-region decoding + region replacements, the
   sound/jingle/projectile/camera/world-entity/graphic
   encoders + senders, the world-entity-interaction hub
   family + the instancing members + the scoped Core
   accessors), live-verified end to end — content can now
   register boat types and docks, board players, build
   instanced scenes, replace map regions, and send sounds,
   jingles, projectiles, and camera moves; the sailing
   SKILL and Construction POH content themselves are plugin
   content and stay NOT STARTED. The engine-systems plan
   in §2 is now EXHAUSTED: all 11 milestones are DONE. What
   remains is content ports (per the plan's §3
   recommendations, piece by piece, with Tyler's direction),
   the surviving pre-existing map/routing findings (gap 11
   — the y=3521–3522 wall leg is RESOLVED in phase 40: it
   is the Wilderness Ditch), the residual gaps named in
   this document, and the HTTP/WebRTC extras. RESOLVED
   (phase 28): poison/venom + skulls + player death exist in Go (the poison clock + effect with
   the 258-entry data table, the orb varp, and save
   round-trip; the skull system with timer + icons + Protect
   Item interaction; the death flow with kept-items rules,
   bones, item loss to the floor, and respawn at home, plus
   the five death hub families), live-verified end to end —
   a cave crawler poisoned a player and the affliction
   survived a logout/relogin round-trip, a skull timer
   counted down live and expired to zero, and a killed
   player respawned with exactly the right items kept. The
   finish-combat slices now stand at four of four: ranged
   (phase 26), spells (phase 27), poison/skulls/death
   (phase 28), presentation (phase 29) — combat is
   finished as a planned engine slice. What remains is
   the residual gaps in gap 15 plus the new phase-29 gaps
   (WeaponProfiles profile layers, `npc-combat-sounds.json`
   death sounds, NPC-with-spell casts, special-attack
   fire sounds).
10. RESOLVED (phase 9): containers/appearance/skills are
    ported, save-integrated, and live-verified (equip/drop/
    pickup/switch-slot/save-round-trip). RESOLVED in part
    (phase 12): the chat-message encoder exists and the 12
    TS-grounded refusal/examine/overflow text sites are wired
    through `sendMessage` (live-verified for the probe hook
    message, the mute refusal, and the examine text); the
    ungroundable sites stay silent with comments, and
    level-up feedback belongs to SkillLevelUp plugin content
    (no `sendMessage` in the TS `SkillManager` level-up path).
    Residual gaps inside those systems: no container/skill
    plugin events beyond the skills families that fire
    since phase 20 (the lifecycle, command, social-packet,
    and skills hook families fire, but `onSkillLevelUp`,
    `onItemEquipped`, and the other content families have
    no emit sites), no bank interface flows
    (plugin content unlocked by the phase 18 runtime, not
    ported), no
    destroy-item widget flow (undroppable items are neither
    dropped nor destroyed), no sound picker call sites or
    combat sound/projectile producers (the sound/projectile
    systems exist since phase 21, but no equipment/NPC-
    combat picker call site and no combat producer calls
    them), no SKILLING content
    (combat XP is granted since phase 16 and the XP gate,
    rate, emits, skill slot, and shared helpers exist since
    phase 20 — but no Woodcutting/Fishing/Mining-style
    skill port grants XP yet), no
    `BonusManager`/weapon-interface updates, bank loading without `withoutLimit` (over-
    capacity saves load only what fits). The `equipment:weights`
    emit site was RESOLVED in phase 22 (live, with
    GracefulOutfit + WornWeights answering it). The item-examine
    price line was RESOLVED in phase 18 (live through the
    varp store). The Wilderness drop check was
    RESOLVED in phase 15 (live, through the area system).
11. Pre-existing map/routing findings, NOT fixed (the
    survivors need TS/cache grounding before any fix; both
    are out of scope for phase 23): (a) the far-WALK bug
    recorded in PHASE12_PLAN.md — a single WALK click over
    64 tiles routes nowhere (the Phase 4 route finder
    searches a 128×128 map centered on the source and the
    closest-approach fallback fails for far-outside
    destinations); staged clicks work. (b) RESOLVED in
    phase 40 — the "gateless wall" found by the phase 16
    smoke test (two solid clipped rows at y=3521–3522
    spanning x=3038..3347) IS the Wilderness Ditch: a probe
    against the real rev-242 cache (temporary test, since
    deleted) showed def 23271 "Wilderness Ditch" (actions
    ["Cross","","","",""], size 1x2, clipType 1) with 273
    locs in the band y3521–3522, x3042..3327, faces 0 (81)
    and 2 (192), all type 10; clip(3090,3521) =
    clip(3090,3522) = 0x40100 (the exact flags phase 4
    found), clip(3090,3520) = clip(3090,3523) = 0x0; the
    turn section is 4 locs at (2996,3530–3533), face 3.
    The zero-XTEA theory is dead: the rows are real ditch
    objects, crossed only via the Cross interaction (phase
    40 ported it — live-verified south/north/turn, and the
    wall leg proves walking in without Cross still blocks).
    Do not "fix" the far-WALK bug by widening the search
    map or clearing clipping without grounding in TS
    behavior. (c) Found by the phase 27 smoke test, same
    family: the projectile line from the spawn (3089,3524)
    to a goblin at (3086,3528) is blocked — a magic attack
    selects, `CanReach` fails, the combat routes east
    around the obstacle, the route ends with no clear line,
    and the post-movement check cancels with the
    TS-faithful "I can't reach that!". The NE line to
    (3092,3520) in the same map region is clear (the manual
    Wind Strike landed there), so the region is loaded and
    the SW line hits a real map obstacle, not unloaded
    data. No port bug found; the obstacle's identity is map
    trivia, not investigated further. The smoke moved the
    second goblin due east (3095,3524) and the autocast
    attack fired there.
12. Phase 22 residual gaps (all documented at the port sites,
    none silent): (a) `equipment:effects` has no Go producer
    — the EquipmentStats plugin (`describeEffects`) is not
    ported, so GracefulOutfit's set-effects line never
    reaches the equipment panel. (b) `skillcape:has-perk`
    has no Go answerer — the SkillCapes plugin
    (`answerPerk`) is not ported, so an Agility/max cape
    never counts as the graceful cape. (c) The login
    bootstrap run-energy frame computes weight from the
    save (`carriedWeightKg`, `bootstrap.go`), bypassing
    the `equipment:weights` overrides; the first tick frame
    corrects it. (d) The other `EquipmentEffects` modifier
    families (hit/prayer/special/status) are NOT STARTED,
    and the Core `ItemDefinition` surface stays table-only
    (content reads names/weights through the player's
    installed definition source).
13. Phase 23 residual gaps (all documented at the port sites,
    none silent): (a) `lumbridge-guide:amulet-status` has no
    Go answerer — the Guardians of the Rift Amulet unit
    (`status`, with its `gotr_amulet_of_the_eye_obtained`
    attribute read, `quest:is-complete` query, and four-
    variant ownership check) is not ported, so the NORMAL
    variant always plays in production; the LOST branch is
    unit-covered and was observed live only through the
    removed probe. (b) The repo's `{}` dialogue starters
    lack the guide's entries — the `npc-dialogue-index.json`
    mappings for NPC ids 306, 1179, 1181, 3393 and the
    `npc-dialogues.json` "Lumbridge Guide" record (7
    variants, 14 branches) — so with starter data Talk-to
    yields no pages; the need is documented in
    PHASE23_PLAN.md and nothing was copied from upstream.
    (c) The remaining 5 upstream variants (astral-contacts,
    treasure-trails, tarnished-locket) have no selectors in
    Go; they are other content pieces' concern.
14. Phase 27 residual gaps (all documented at the port sites,
    none silent): (a) Ancients/Lunar/Arceuus spellbooks are
    NOT STARTED (tables, handlers, Lunar routing beyond a
    documented drop). (b) Bind/curse/freeze application is
    plugin content — the phase emits `magic:utility-effect`
    and nothing applies a freeze until its plugin ports.
    (c) Vengeance and other Lunar utility spells are NOT
    STARTED. (d) Spell-on-object (opcode 2) is NOT STARTED
    (needs a hub family). (e) `ApplyChargeMaxHit` has no call
    site — it needs the `GetMagicHitBounds` seam; do NOT wire
    it via `ModifyMagicMaxHit` (wrong order). (f) NPC-target
    god drains have no seam (no NPC level setter); TeleBlock
    sends no effect-timer frame (no sender seam). (g) Magic
    Dart Slayer's-staff rules, the ElementalStaves plugin
    (staff rune coverage — without it a staff of air does not
    cover the air rune on combat spells, TS-faithful), and
    the Barrows Ahrim's autocast exception are NOT STARTED.
    (h) Animation/graphic/projectile/sound production is
    Phase 29: `StartCast` is a deliberate no-op, so a cast
    sends no projectile frame. (i) `ItemAliases`, rune-pouch
    sources (no-source default), the alchemy confirm dialogue
    (fail-closed), the click-delay system, and `Player.SendTab`
    are NOT STARTED. (j) Production data gaps, found by the
    smoke test and RESOLVED 2026-11-17 (Tyler-approved
    verbatim import, SHA-256 verified): the repo now ships
    `item-alch.json` (16,750 entries) and
    `item-gameplay.json` (27,407 entries) — without them
    every alchemy cast was refused and no weapon had a
    weapon-interface name. The import also unblocked
    deathComponents splitting (phase 30, gap 15b).
    (k) The alchemy XP
    table values (Low 4000 / High 20000) are verbatim TS
    dead data — TS pays its own listener constants 31/65,
    and Go mirrors that exactly.
15. Phase 28 residual gaps (all documented at the port sites,
    none silent): (a) No live PvP attack can start at all —
    `CanAttackByPolicy` denies PvP with no opinion, and the
    allow comes from the Wilderness plugin's `canAttack`
    (`Wilderness.plugin.js:469`), which is NOT ported; the
    skull trigger is unit-covered only, and the head-skull
    icon on the wire was not directly observed (it needs
    player-update frame parsing). (b) deathComponents
    splitting is DONE since phase 30
    (`splitIntoComponents`, `PlayerDeath.ts:353-360`;
    the 10 gameplay entries — Nightmare staves
    24423–24425, Avernic treads variants 31091–31097 —
    split in Wilderness PvP deaths; unit-covered only,
    since live PvP is still blocked — see (a)). (c) No grave — the `onPlayerDeathDrops` hook
    exists and fires, but no grave content answers it, so
    drops go to the floor; TS `PlayerDeath.ts` itself opens
    no death interface (only `sendInterfaceRemoval`, which
    Go ports), so death presentation is the message + jingle
    90 + the death animation 836 (phase 29). (d) Poison residuals: spell-effect poison has
    no call site, cure items are NOT STARTED (no
    antipoison/serum consumption path), and an NPC's
    `poisonImmunityTimer` is never started by core (nothing
    grants NPCs immunity). (e) RED skull has no core call
    site — `handleSkull` always applies WHITE; item skulls
    (amulet of avarice, cape of skulls) are plugin content.
    (f) Death-rule residuals: `keepsEverything` is always
    false (no rights field), Barrows items are treated as
    ordinary untradeables, and there is no Ironman
    loot-owner rule. (g) Death animation 836 is recorded
    and sent since phase 29 (player death anim + NPC
    definition death anims at the combat call sites). (h) The
    poison clock's counter is the protection dose only —
    a fresh poison has counter 0, which is TS-faithful, not
    a bug; the coordinator's first smoke run failed its own
    check on this before the expectation was corrected.
16. Phase 32 residual gaps, as closed or remaining after
    phase 33 (all documented at the port sites, none
    silent): (a) The Woodcutting sub-units EXCEPT the Guild
    are DONE (EntTrunk, ThickVines, Shrine, ClueNests,
    CrystalAxe, InfernalAxe — phase 33). The Guild is NOT
    STARTED (incl. `data/woodcutting-guild.json`, NOT
    imported: the import needs Tyler's approval; `isInGuild`
    stays false and `invisibleBoost` stays 0 in the core
    loop). The Farming slice Woodcutting pulls
    (`Redwood.Farming.js` → `Core.Farming.js`) is 2 constants
    only (`DEPLETION_ROLL` 11, `REGROW_TICKS` 200) — Farming
    itself is NOT ported. (b) CLOSED in phase 33: the stump
    model-map + treeId+1 legs now run from a load-time memo
    (`StumpByTreeID`, cross-checked against the real cache).
    (c) CLOSED in part in phase 33: the face-tree step now
    runs (`Player.ForcePositionToFace`); the ForceMovement
    tick-skip still cannot run — no ForceMovement field or
    system exists in Go — and the player sync encoder does
    not yet send face updates (state + flag are set; clients
    do not see the turn). (d) XP is truncated to an integer
    (37.5 → 37): structural, the Go SkillManager stores
    `[]int` while TS stores tenths as floats — a fix would
    retype the XP pipeline, out of scope. (e) CLOSED in
    phase 33: interaction distance is the TS footprint rule
    (`GameObject.ts:151-155`, sizes from the cache
    definitions at load). (f) CLOSED in phase 33:
    `treeAt`'s template path now checks
    `World.getRemovedObjects()` (new `RemovedObjects()` on
    `ObjectManagerAPI`). (g) `equipment:effects` is
    producerless, so the Lumberjack set-text handler never
    fires. (h) No SkillLevelUp emitter exists yet, so the
    level-up pause never fires. (i) The `onItemAction` seam
    does not port the `item:operated` custom emit or the
    `subOption` fill-in, and the widget-action emit sites
    remain gaps. (j) Nest drops are unit-covered only (1/256,
    members-only; unobserved live) — as are all six phase-33
    sub-units live (no Ent kill/chop, vine clearing, clue
    roll, or crystal/infernal Check observed over the wire).
    The Infernal burn never fires until Firemaking lands
    (`firemaking:log-xp` unanswered). (k) The hub-contract
    lesson from phase 32 is recorded in the Plugin hub notes:
    hub surfaces installed after plugin Load must be resolved
    lazily, and `TaskManager.Schedule` runs only on the tick
    goroutine — content registering a startup hook that
    schedules tasks will race; the race detector caught exactly
    that in-phase. (l) The clue-nest box-true path needs
    `forceAdd`, which Go has no form of, and no answerer can
    set it — rolls always ground-drop. (m) The vine tool-use
    handlers are registered by name (one `OnItemOnObjectNamed`
    per axe display name × chop name + Rake × Vines) because
    `ItemOnObjectEvent` carries no object definition; the
    hub's name match is the group filter. (n) The clue-nest
    Wilderness ring check reads `*game.Wilderness` through a
    type assertion on the Api's AreaManager; if the manager
    is ever wrapped, the ring silently stops doubling — a
    `SetWilderness`-style installer would remove the
    fragility. (o) Respawn is scheduled at death dispatch in
    Go, vs at remove() time in TS — a documented divergence
    in `combat_death.go`.

17. Phase 34 residual gaps (Fletching; all documented at
    the port sites, none silent): (a) XP integer truncation
    is structural (shared with Woodcutting — the Go XP
    grant takes an integer where TS takes a float). (b)
    Quest/Slayer-gated recipes are locked fail-closed
    (no quest/Slayer state exists to check against yet).
    (c) `crafting:spun` has no producer, `level-up:
    popup-opened` is never emitted, `fletching:shafts`
    is answered but has no consumer yet, and `diary:task`
    has no consumer — all four are honest dead ends until
    their counterpart systems land. (d) No SkillLevelUp
    emitter exists (same gap as Woodcutting). (e) The
    force-movement task stop is unported (no Go seam —
    the same Woodcutting gap 16 item). (f) Spool messages
    are TS's own text; no rsprox capture exists to confirm
    the exact live strings. (g) Only the three smoke legs
    (shafts, stringing, spool fill) were observed live;
    the other 167 recipes, the diary tasks, and the
    chisel/hammer paths are unit-covered only. (h) The
    CreationMenu seam sends the menu through the session
    primitives directly (per TS), not through
    `SendChatboxInterface` — see the Phase 34 paragraph
    for the grounding; the `routeDialogueResume`
    pre-branch and `StopSkillable` clearing are the only
    listener/session touch points. (i) The duplicate
    event-name constants the workers produced
    (`FletchingMadeEventName` vs `MadeEventName`, same
    value) were unified by the coordinator — the emit site
    uses `MadeEventName` only.

18. Phase 35 residual gaps (Fishing; all documented at
    the port sites, none silent): (a) XP integer truncation
    is structural (shared with Woodcutting/Fletching — the
    Go XP grant takes an integer where TS takes a float).
    (b) MinnowPlatform is NOT STARTED (needs an
    NPC-relocation seam — Go NPCs have no `moveTo` — plus
    Kylie access, row boats, and the minnow exchange).
    (c) The AnglerOutfit Gita exchange is NOT STARTED
    (needs `onItemOnNpc`, still table-only, plus
    `npc-dialogue:*` emitters and the Gita transcript).
    (d) The Guild door is NOT STARTED (needs
    `door:toggle` + ObstacleRunner / ForceMovement); the
    Guild invisible boost (+7) IS ported. (e) The infernal
    cook never fires (`cooking:raw-xp` has no answerer
    until Cooking lands). (f) Clue bottles fall to the
    ground, never into the inventory (no Go `forceAdd`).
    (g) The Angler set text never fires (`equipment:effects`
    has no producer). (h) Quest gates always pass (no
    QuestRuntime; TS-faithful OPEN). (i) No SkillLevelUp
    emitter exists (same gap as Woodcutting/Fletching), so
    `level-up:popup-opened` never fires. (j) `diary:
    is-complete`, `fishing:continue`, `fishing:tool`,
    `fishing:interval`, `fishing:catch-chance`,
    `fishing:attempt`, `fishing:catch`, `fishing:success`,
    `fishing:junk`, and the `storage:` queries have no
    answerer (documented defaults). (k) The session holds
    the NPC pointer, not the world NPC index — stricter
    than TS. (l) `variant.baitless` is unexpressible (no
    Baitless field on `ToolVariant`). (m) No ForceMovement
    check in the loop (no such system in Go). (n) Only the
    four smoke legs (net, bait rod, fly rod, full
    inventory) were observed live; the harpoon/crystal/
    infernal/karambwan/Rada's/spirit-flake/clue-bottle/
    guild legs are unit-covered only, and spot
    depletion/movement (the 1/90 random move) was not
    observed live. (o) `PARK_NET` table chance `[[20,20]]`
    and the minnow chart `[24,148]` are upstream
    estimates, not measured curves; `DARK_CRAB_POT` relies
    on TS defaults.

19. Phase 36 residual gaps (Cooking; all documented at
    the port sites, none silent): (a) XP integer truncation
    is structural (shared with Woodcutting/Fletching/Fishing
    — the Go XP grant takes an integer where TS takes a
    float). (b) Unanswered queries all fail-closed:
    `skills:level-boost` → 0, `skills:actions-per-cycle` →
    1, `diary:is-complete` → false, `quest:is-complete` /
    `quest:is-started` → false, `hunter:rumours-completed`
    → 0, `skillcape:has-perk` → false (documented
    defaults). (c) The Lumbridge Castle range (object 114)
    is refused without Cook's Assistant (no Go answerer for
    `quest:is-complete`). (d) `duringQuest` foods
    (`tbwt_raw_karambwanji`) and `rumours` foods (6 hunter
    meats) never cook. (e) Hosidius Kitchen bonus is 12,
    never 25 (no `diary:is-complete` answerer). (f) The
    infernal-harpoon cook is unit-covered only
    (`TestTryCookFishFiresAnswered` + `TestAnswerRawXP`);
    never observed live. (g) No SkillLevelUp emitter exists
    (same gap as Woodcutting/Fletching/Fishing), so
    `level-up:popup-opened` never fires. (h) No ForceMovement
    check in the loop (no such system in Go). (i)
    LeapingFish/Churn offcuts ground-drop via
    `DropGroundItem` (no Go `forceAdd` — the Wiki outcome,
    unconfirmed in TS). (j) Wine banked leftovers go to the
    first tab with a free slot (no Go `GetTabForItem`).
    (k) `cooking:cooked` has no Go emitter (the cooking
    package defines the request type; nothing emits it).
    (l) Gnome reads the used item from the inventory slot at
    handler time (`PluginItemActionEvent` carries no Item
    reference). (m) Only the three smoke legs (fire, range,
    wine) were observed live; Gnome, Churn, SacredEel,
    LeapingFish, doughs, karambwan, and the other 167 recipes
    are unit-covered only. (n) The Phase 35 gap 18(e) is now
    closed at the unit level: `cooking:raw-xp` HAS an
    answerer (this phase), so the infernal cook can fire;
    it still awaits a live observation.

20. Phase 37 residual gaps (gateway; all documented at
    the port sites, none silent): (a) The external
    runelite-gateway process was NOT run — this phase
    makes the Go server serve what that process reads
    (jav_config + worldlist); running RSProx end to end
    is a separate step. (b) `s.Port(opts.Port)` is computed
    in `buildServer` for the gateway and again in `main.go`
    for `ListenAndServe` — the computation is identical
    and deterministic, but on an invalid port value the
    fallback warning logs twice (invalid-port path only).
    (c) The full `go test -race ./...` run does not pass
    cleanly on this tree under load (see "What was verified
    in this run"): the same run fails on the clean phase-36
    tree, so it is pre-existing — one genuine data race in
    the combat-cycle source (`SetCombatCycleSource` vs
    `combatCycle()`, `internal/game/combatant.go:484-495`)
    plus timing-sensitive savewiring tests and the known
    `TestEngineTicksAtRate` flake. None of it is phase-37
    code. (d) The gateway advertises type flags 0 /
    activity `tsps` (the TS default) because the Go world
    definition carries no world-type source; a game mode
    that claims a world type would need a `setWorldType`
    equivalent wired into the gateway target first.
    (NOTE: the phase-38 race run passed 22/22 clean, so
    the (c) failure did not reproduce there either — the
    combatant race remains unverified, worth a look, not
    this phase's scope.)

21. Phase 38 residual gaps (`::item`; all documented at
    the port sites, none silent): (a) For ids / amounts
    >= 2^63 the Go parser saturates at math.MaxInt64 while
    JS keeps a float, so the success message prints
    `9223372036854775807` where TS prints
    `100000000000000000000` (and ids >= 2^63 do not exist
    in the cache anyway); the inventory behaviour is
    identical — the divergence is the printed number only.
    (b) `::give`, `::items` (search UI) and every other
    admin command in `AdminCommands.plugin.js` are NOT
    STARTED — `::item` is the only command registered.

22. Phase 39 residual gaps (NPC spawns + npc-combat-defs /
    npc-animations / shops; all documented at the port
    sites, none silent): (a) Elena: the spawn file's only
    keyed entry (`{"level":0,"name":"Elena","x":2592,
    "y":3336,"key":"ELENA"}`) is dropped — Go has no
    `NpcIdentifiers` table, so the loader warns `Unknown
    NpcIdentifiers key "ELENA"` and skips her. TS resolves
    ELENA → 1102 and spawns her. The file stays verbatim;
    do not hand-edit an id into it. (b) Tool Leprechaun:
    45 entries carry `"id": 0`. Both TS and Go keep id 0
    (0 < cache NPC count 16,876), so both spawn NPC 0 at
    those tiles — parity, but the upstream data is suspect;
    do not "fix" the ids. (c) Eager spawning: TS defers
    most spawns in `pendingByRegion` and activates them
    near players; Go spawns EAGERLY — every accepted record
    becomes a live NPC via `QueueAddNPC`
    (`internal/game/spawns.go` header). The
    deferred-activation machinery is NOT STARTED.
    Consequence, measured: the add queue drains at
    `QueueDrainCap` 150/tick (600 ms/tick), so 30,827
    spawns take ~206 ticks ≈ 124 s to fully drain, in file
    order (not geographic); Lumbridge-area entries span
    file idx 0–20124 (goblins ~tick 17 / ~10 s, chickens
    ~tick 55 / ~33 s, last Lumbridge entry ~tick 134 /
    ~80 s). Boot cost itself is negligible: 2.85 s → 3.09 s
    to the started line (+0.24 s). (d) No shop opens by NPC
    click: the Shopkeepers plugin (231 lines,
    `plugins/npcs/Shopkeepers.plugin.js`) is NOT STARTED —
    it is the TS piece that reads `shops.json`
    `npcInteractions` (+ `custom-shops.json` bindings) and
    registers `NpcInteractionDefinition`s with
    `core.shops.open` actions. The Go loader ignores
    `npcInteractions`, `loadInteractions` has zero
    contributed sources (`Store.Interaction()` is always
    empty), and no shipped dialogue opens a shop (the
    `open_shop` transcript step exists but zero dialogue
    rows use it). The ShopManager holds all 487 shops with
    stock (load-proven), but there is no client-reachable
    trigger to open one — including the General Store (shop
    0), whose own `npcInteractions` is empty in the file.
    Porting Shopkeepers is the fix; it is content work,
    not a data import. (e) Forester cache name: NPC
    12119's rev-242 cache name is literally `"null"`; the
    spawn file carries "Friendly Forester" and the spawn
    uses the file name. TS reads the same cache, so parity
    holds; noted, not a defect. (f) Live legs (smoke39,
    real rev-242 cache, run singly): presence 7/7
    (Edgeville set in sync; the first attempt failed 2
    checks — Imp 5007 ~30 tiles and Banker 1613 ~35 tiles
    away, beyond the ~15-tile NPC view distance — so the
    presence set was corrected to NPCs within ~13 tiles),
    lumbridge 6/6 (Forester 12119 radius 0 at (3096,3238)
    first-seen 1 s; Dark wizard 512, Banker 1613, Man 9392,
    Fishing spot 1525 all 6 s), chickens PASS (7 chickens
    1174 in view), goblins PASS (5 goblins near
    (3002,3206)), wander PASS (Shop keeper 2821 moved
    (3080,3511) → (3079,3512) within radius 3; Forester
    12119 unmoved in 60 s), respawn PASS (Goblin 3036 died,
    death anim 6182 on the wire, count in the circle 1 → 0
    → 1, i.e. the respawn clone fired for a file-loaded
    spawn), combatanims PASS (Goblin 3036 attack 6184 /
    block 6183 / death 6182 all on the wire — the
    npc-combat-defs.json values, not the cache scanner's
    guesses). The two legs were retargeted from the
    Edgeville rats to the Goblin House goblins because the
    rats live inside a walled pen (cache clip grid + walk
    probe: the player stops at (3117,3485) and combat
    cancels with "I can't reach that!") — correct game
    behavior for attacking through a wall, not a port bug;
    see PHASE39_PLAN.md Results. The `walkprobe` mode stays
    in the smoke39 driver as the pen-access diagnostic (it
    FAILs by design).

23. Phase 40 residual gaps (Wilderness Ditch; all
    documented at the port sites, none silent): (a) The
    ForceMovement execution path exists now
    (`internal/game/forcemovement.go`: type, task, click
    delay, mobility INVALID gate, sync view), but the ditch
    is its ONLY consumer — no other ForceMovement
    producers are ported (agility obstacles, knockbacks),
    and there is no general ObstacleRunner. The forced-walk
    area fallback (`requestWalk` 406-413,
    `area.handleForcedWalk`) is still absent, and
    `Player.Busy()` still does not exist (Go has no Busy();
    the object-action listener's `busy()` still
    degenerates to the hitpoints term — the ditch does not
    need it). (b) The needsPlacement INVALID gate
    (`MovementQueue.ts:350-351`) is still not consulted in
    `getMobilityIgnoringFreeze` — deliberate, needs its own
    verification; `MovePlayerTo` now sets NeedsPlacement,
    so the field is live, but the gate stays a gap. (c) No
    Wilderness entry warning or dialogue exists — checked:
    neither `WildernessDitch.plugin.js` nor
    `Wilderness.plugin.js` has one, so first entry is
    silent in TS too; nothing to port. (d) The click-delay
    gate reads the world cycle through the existing
    `combatCycle()` source — no second cycle source was
    added (see `forcemovement.go`); a second Cross within
    250 ms is consumed silently (unit-covered; the live
    legs cross once each). (e) Live legs (smoke40, real
    rev-242 cache, run singly): ditch-south PASS 4/4
    (landed (3090,3523), anim 6132, sound 2462 delay 25),
    ditch-north PASS 4/4 (landed (3090,3520)),
    ditch-turn PASS 4/4 (landed (2998,3530)), ditch-wall
    PASS (walked north without Cross, stayed
    (3089,3520)). Arrival-tile finding, verified live: a
    diagonal corner tile does NOT reach a 1x2 solid
    (`reachRectangle1` — only side-adjacent tiles reach),
    so the player steps to the side-adjacent arrival tile
    before jumping, and lands 3 tiles from the ARRIVAL
    tile, not from the click-time tile; the turn leg's
    north-adjacent tile (2996,3531) is itself a ditch piece
    (walled), so its arrival is the west side (2995,3530).
    (f) `Task.OnStop` is a new general mechanism (fires
    once, only via `Stop()`, never when a task finishes by
    returning done) — only ForceMovementTask uses it so
    far.

## Next modules, in priority order

1. The engine-systems plan in `CONTENT_MIGRATION_PLAN.md`
   §2 is EXHAUSTED: all 11 milestones are DONE (player
   attributes, chat + commands, object registry +
   interaction, dialogue, timers + areas, NPC drivers +
   combat, shops/trading, interface runtime, clans/social,
   skill handlers + XP, sailing/template regions/sounds).
   Content ports have begun: §3 item 1 (DateMinutes,
   phase 10), item 2 (GracefulOutfit + WornWeights,
   phase 22), item 3 (LumbridgeGuide, phase 23), and item 4
   (SeekingArrows, phase 24) are DONE, and the first skill
   is ported (Woodcutting, phases 32–33 — see
   `PHASE32_PLAN.md` and `PHASE33_PLAN.md`; core loop plus
   all sub-units except the Guild, which needs Tyler's
   `woodcutting-guild.json` import approval), and the
   second skill is ported (Fletching, phase 34 — see
   `PHASE34_PLAN.md`; full skill including the CreationMenu
   seam, live-verified), and the third skill is ported
   (Fishing, phase 35 — see `PHASE35_PLAN.md`; core loop +
   8 of 10 sub-units with the onNpcInteraction seam,
   live-verified in 4 modes; MinnowPlatform, the Gita
   exchange, and the Guild door deferred), and the fourth
   skill is ported (Cooking, phase 36 — see
   `PHASE36_PLAN.md`; full skill — Cookers, Recipes, Wine,
   Churn, SacredEel, LeapingFish, Gnome — live-verified on
   fire/range/wine legs; the `cooking:raw-xp` answerer
   unblocks Phase 35's infernal-harpoon cook at the unit
   level). The next skill is Tyler's pick.
   The first-quest survey
   (phase 25, `QUEST_SURVEY.md`) is also DONE: 5 quests ranked
   with measured evidence, Doric's Quest recommended — Tyler
   picked it, then minutes later instructed to postpone
   quests and finish combat first, so Doric's Quest stays
   picked-but-deferred. Phase 26 landed the first
   finish-combat slice (ranged data, combat styles, onCanAttack,
   spec orb, ranged projectiles, flight divergence, NPC corpse
   removal). Phase 27 landed the second slice (combat spells:
   standard spellbook, MagicOnNpc/MagicOnPlayer/MagicOnItem
   listeners, autocast + spellbook + the spell hub families,
   and the autocast save-split fix). Phase 28 landed the
   third slice (poison/venom + skulls + player-death
   loss/teleport: the poison clock + effect with the
   258-entry data table, the skull system, the death
   kept-items/loss pipeline + five death hub families, all
   live-verified). Phase 29 landed the fourth and last
   finish-combat slice (combat presentation — see
   `PHASE29_PLAN.md`): the NpcAnimationScanner port,
   attack/block/death animations + spot animations at the
   combat call sites, `StartCast` live, ranged launch
   graphics populated, and sound pickers at the call
   sites — all live-verified. Combat is finished as a
   planned engine slice. Phase 30 landed the first
   combat-leftovers slice (see `PHASE30_PLAN.md`):
   WeaponProfiles profile layers, dark-bow second-hit
   delay, exact projectile heights, NPC-with-spell casts
   (engine path open; no producer sets an NPC's cast
   spell), special-attack fire sounds, queued-graphic
   priority, spec-orb sends, deathComponents splitting,
   fightType wired live, the onCanAttack legacy fallback
   removed, and the plugin ammo / crystal-bow charges
   engine part — all live-verified where observable.
   What remains: the gap-14 spellbook residuals
   (Ancients/Lunar/Arceuus, bind/curse/freeze, Vengeance,
   spell-on-object, `ApplyChargeMaxHit`, NPC god drains,
   TeleBlock timer frame, Magic Dart Slayer's-staff rules,
   ElementalStaves, rune pouch, alchemy confirm dialogue);
   `npc-combat-sounds.json` death sounds (import needs
   Tyler's approval); NPC spell selection from NPC combat
   defs; the crystal-bow degradation loop + charge system;
   plugin ammo resolution; `ItemAliases`; the javelin
   weapon-interface seam; and the gap-15 residuals (no
   grave, no live PvP, cure items, RED skull call site,
   keepsEverything, Barrows items, Ironman loot-owner
   rule). Tyler decided 2026-11-17: combat leftovers
   first, then adding all skills.
   After combat, content ports resume piece by piece
   behind the hub per the plan's §3 recommendations and
   only with Tyler's direction. Each phase names which
   piece it ports and what stays unported. The skills
   survey (phase 31, `SKILLS_SURVEY.md`) is DONE:
   29 skill entries measured (393 files, 77,898 lines),
   per-skill require-closure true totals, a lexical
   dependency sweep, a data-file map, a cross-dependency
   map, and the Go skill-engine gaps (NPC-interaction
   family, item-action/item-on-item families,
   spell-on-X/`onSpellCast`, consumable/equipment gates,
   `CreationMenu`, SkillLevelUp feedback, ground-item
   seam + `forceAdd`, `spawnNpc`/`removeNpc`). Ranked
   shortlist of 5, all read in full: 1. BarbarianPyres
   (266 lines, RECOMMENDED — core loop needs no new
   engine system and no data import), 2. BoneAltars
   (269), 3. Ectofuntus (747), 4. Herblore (761),
   5. Fletching (965). Tyler picked Woodcutting as the first
   skill on 2026-11-19 (over the survey's recommended
   BarbarianPyres) and re-confirmed it the same day after a
   conflicting Fletching widget tap — Fletching is NOT
   confirmed as queued next. Phase 32 (2026-11-21,
   `PHASE32_PLAN.md`) ported the Woodcutting core chopping
   loop live-verified, and Phase 33 (2026-11-21,
   `PHASE33_PLAN.md`) finished the skill — all sub-units
   except the Guild (EntTrunk, ThickVines, Shrine,
   ClueNests, CrystalAxe, InfernalAxe), with the
   `onNpcClick` + `onItemOnItem` hub families and the NPC
   remains-delay + transform seams built as part of it.
   What remains of Woodcutting is the Guild, which needs
   Tyler's approval to import `woodcutting-guild.json`
   (asked 2026-11-20, awaiting his answer); the next skill
   after that is Tyler's pick — Fletching is NOT confirmed
   as queued. The `item:operated` emit and `subOption`
   fill-in and the SkillLevelUp emitter are still NOT
   STARTED.
   Residual engine gaps named in this document (the
   `viewedArea` object-sync term, the `sceneTile`
   root-location term, camera state, `VarManager`,
   destroy-hook splicing) are fixed only when a content
   piece needs them, grounded in TS first.
2. The surviving pre-existing map/routing findings (gap 11:
   far-WALK, and the phase-27 projectile-line obstacle) stay
   out of scope until TS/cache grounding exists for a fix.
   The y=3521–3522 wall leg is RESOLVED in phase 40 (it is
   the Wilderness Ditch).
3. HTTP extras (`ContentApi`, `/regions` — `GatewayTarget`
   DONE in phase 37), then WebRTC (needs a Go stack + relay port) if required.
