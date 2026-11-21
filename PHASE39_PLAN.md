# Phase 39 Plan — NPC spawns (import `npc-spawns.json`)

## Decision
Tyler asked 2026-11-21 02:03 PST: "Add npc spawns" (typed "nice spawns"
first — autocorrect; "npc" is the word). This phase imports the upstream
`npc-spawns.json` verbatim into `data/definitions/` and proves the world
is populated. Tyler's request IS the approval for this verbatim import
(same precedent as item-alch.json, item-gameplay.json,
npc-combat-sounds.json, fletching.json, cooking.json).

## Scope
- Copy `~/tsps-bfd81e3/server/data/definitions/npc-spawns.json`
  (2,121,046 bytes, 30,828 entries) verbatim to
  `data/definitions/npc-spawns.json`; SHA-256 verify identical to
  upstream; record the hash in PORT_STATUS.md's data-import paragraph.
- **SCOPE EXPANSION (Tyler, 2026-11-21 02:23 PST: "Also add
  npc-combat-defs npc-animations and shops"):** the same verbatim
  treatment for three more files, all in
  `~/tsps-bfd81e3/server/data/definitions/`:
  - `npc-combat-defs.json` (101,970 bytes): top shape
    `{"$comment", "npcs", "refs", "defaults"}` — `npcs` is 963
    id-keyed entries `{name, anims:{attack,block,death,...},
    sounds, respawn, projectile, deathTicks, huntRange,
    launchGraphic}`; `refs.npcs` is rows of `[id, attack, block,
    death]`; `defaults.humanoid` is `{attack: 422, block: 424,
    death: 836, deathSound: 512}`. (NOT a flat id→array map —
    the parent's first grounding said that; the file itself
    corrects it. The flat id→array shape is npc-animations.json.)
  - `npc-animations.json` (84,032 bytes): flat id → array of
    sequence ids, 3,383 entries, rows of 1–28 numbers.
  - `npc-animations.LICENSE` (1,383 bytes, BSD 2-Clause,
    Adam/sigterm + Null): copied alongside so the attribution
    travels with the data; the repo is public.
  - `shops.json` (325,748 bytes): array of 487 shops, 5,471
    total originalStock lines, ids 0–1499 (unique), shop 0 is
    "General Store".
  Go loaders already exist for all three
  (`internal/definition/npc.go` `loadNpcDefinitions`, which reads
  both animation files; `internal/definition/shop.go`
  `loadShops`, called from `loader.go:431`); the repo holds
  3-byte `{}` starters today, so the loaders run on empty data.
  Verify by loading: counts + spot resolutions. If any file's
  shape fails the existing loader, do NOT hand-edit the file
  (upstream rule 16: generated data is fixed at the source) —
  report it as a loader gap and leave that file's import un-done
  rather than patching data.
- Ground the loader against the real file; live-verify NPCs are
  present, wander, and respawn. No wiring change is expected (see
  Grounding) — if one turns out to be needed, say so plainly.
- Content row count does NOT change: this is definition data, not a
  plugin entry.

## Grounding (parent-verified 2026-11-21)
- Upstream file, measured in full (30,828 entries, all objects):
  fields used are `level` (all; values 0/1/2/3 only), `name` (30,782;
  46 entries have none), `x`/`y` (all), `id` (30,827 — one entry,
  Elena, has a `key` instead), `wanderRadius` (4,506 entries, values
  0–12, max 12; 26,322 entries have none → default 5), `direction`
  (599 entries, values 0–7), `source` (6,269 entries: provenance
  metadata — `wiki`, `rsprox`, `rsprox:<id>`, `capture-1693`, etc. —
  NOT read by the TS loader's `toDefinition`, which takes only
  id/level/x/y/direction/wanderRadius/name; the Go loader likewise
  ignores it), `key` (1 entry). No `remove` entries. 36 duplicate
  (id,x,y,level) keys (kept in both TS and Go — duplicates inside one
  source are all kept). x range 1042–4286, y range 1250–12581.
- The Go loader already handles every field the file uses
  (`internal/definition/spawns.go`, `loadNpcSpawns` porting
  `NpcSpawnDefinitionLoader.load()`, `toNpcSpawn` porting
  `toDefinition`): explicit `direction` wins else cache
  `spawnDirection` else south; `wanderRadius` default 5, clamped
  >= 0; `level` → Z; missing `name` → `""`; ids >= the cache NPC
  count are dropped as unsupported (TS line 120, same rule); a
  missing file is TOLERATED (failure-counted in `loadSources`, never
  boot-fatal — that is how the server boots today with no file).
  Boot wiring (`cmd/server/definitionwiring.go`,
  `loadGameDefinitions`) already loads the Store from the run root's
  `data/definitions/` and applies `store.NPCSpawns()` via
  `game.SpawnNPCsWithHitpointsAndDefinitions` after the
  RegionManager is installed; the loader registers the core file
  itself (`internal/definition/loader.go:390`,
  `RegisterFile(npcSpawnsDefinitionType, "NpcSpawns", <dir>/
  npc-spawns.json)`). No wiring change is needed to pick up the
  shipped file.
- Known divergences to document, not fix silently:
  - **Elena (1 entry):** `{"level":0,"name":"Elena","x":2592,
    "y":3336,"key":"ELENA"}`. TS resolves the key through
    `NpcIdentifiers` (`ELENA = 1102`,
    `NpcIdentifiers.ts:1053`) and spawns her. Go has no
    `NpcIdentifiers` table ported, so the loader drops the entry
    with a warning (`toNpcSpawn`, "Unknown NpcIdentifiers key").
    PORT_STATUS.md's loader row currently says keyed spawns are
    "skipped with a warning, matching TS unknown-key behavior" —
    that framing is wrong for this entry: the key IS known in TS.
    Correct the row and record the 1-entry gap. The file itself
    must stay verbatim; do not hand-edit Elena an id.
  - **Tool Leprechaun (45 entries):** all carry `"id": 0`.
    Both TS and Go keep id 0 (0 < cache NPC count), so both spawn
    NPC 0 at those tiles — parity, but the upstream data is
    suspect (the real Tool Leprechaun id is not 0). Document it;
    do not "fix" the ids.
  - **Eager vs deferred spawning:** TS defers most spawns in
    `pendingByRegion` and activates them near players; Go spawns
    EAGERLY — every record becomes a live NPC via `QueueAddNPC`
    (`internal/game/spawns.go` header, honest static-world
    approximation). The deferred-activation machinery is NOT
    STARTED and stays that way this phase.
- **Drain math (matters for the live test):** the add queue drains
  at `QueueDrainCap` = 150/tick (`internal/game/constants.go:15`,
  `world.go` `queue_add_npcs` phase), 600 ms/tick. 30,827 accepted
  spawns take ~206 ticks ≈ 124 s to fully drain, in file order.
  File order is NOT geographic: Lumbridge-area entries span file
  idx 0–20124 (goblins from idx 2563 → tick ~17, ~10 s; chickens
  from idx 8210 → tick ~55, ~33 s; last Lumbridge entry idx 20124
  → tick ~134, ~80 s). The live driver must wait ~90 s after
  engine start before asserting the full Lumbridge set (or poll
  until the expected NPCs appear), and must not mistake drain lag
  for missing spawns.
- **Capacity:** `MaxNPCs` = 65,534 (`internal/game/constants.go`),
  so 30,827 spawns + respawn clones fit. Hitpoints come from the
  Store-backed source (`Store.NPC`, full chain incl. the JSON
  monsters dump when present, else cache, else 10).
- Existing tests: `internal/definition/spawns_test.go`
  (`TestNpcSpawnRealShapeNilCache`, `TestNpcSpawnFacing`,
  `TestNpcSpawnWanderRadius`, `TestNpcSpawnInvalidEntriesSkipped`,
  `TestNpcSpawnKeyedEntryDropped`, `TestNpcSpawnCacheDirection`,
  `TestNpcSpawnMergeReplaceAndRemove`). The doc.go `key` note
  already anticipates the shipped file.

## Requirements
1. Import the file verbatim; `sha256sum` both sides, identical.
   Record the hash in PORT_STATUS.md's data-import paragraph
   (same sentence shape as the fletching/cooking entries).
2. Load-prove with a TEMPORARY test against the real rev-242 cache
   (then delete it, as prior phases did): candidates, accepted,
   invalid, unsupported (ids >= cache NPC count — report the cache
   NPC count and how many entries that drops, if any), failures;
   confirm Elena is the only invalid entry; confirm the 45 id-0
   Tool Leprechauns are kept; spot-check Friendly Forester
   (12119 at 3096,3238, radius 0), a goblin (3036 at 3002,3206),
   a chicken (1174 at 3225,3300). Report the boot `[npc-spawns]`
   log line verbatim.
2b. Load-prove the three extra imports the same way (same
   temporary test, then deleted): npc-combat-defs.json —
   `npcs` entry count (963), refs.npcs row count, defaults
   present; npc-animations.json — entry count (3,383);
   shops.json — shop count (487) + total originalStock lines
   (5,471). Spot-resolve through the Store: `Store.NPC(id)` for
   a few ids (rat 2854, goblin 3036, guard 3254 — report each
   id's AttackAnim/DefenceAnim/DeathAnim as the loader resolves
   them, noting which come from the defs vs the 422/424/836
   defaults); `Store.Shop(0)` — name "General Store", its
   originalStock rows. If the loader rejects any file, stop:
   report the loader gap, do NOT edit the file.
3. Boot-cost measurement, before vs after, same temp-root shape:
   (a) time from process start to the `[server] ... started`
   line with the file ABSENT (copy `data/` minus
   `npc-spawns.json`) vs PRESENT; (b) time until the add queue
   fully drains (≈206 ticks by the cap — verify by watching the
   Lumbridge set complete in the live driver, not by asserting
   an internal counter); (c) watch the server log for tick
   overrun/freeze lines with 30k NPCs live. Say plainly whether
   the cost matters.
4. Tests (permanent, in `internal/definition/spawns_test.go` or a
   new test file there):
   - Field handling on small fixtures: `wanderRadius` absent →
     5; explicit `direction` (incl. `& 7` masking); `level` → Z;
     missing `name` → `""`; `source` field ignored; duplicate
     (id,x,y,level) keys all kept.
   - Shipped-file test (nil cache, so the count is
     cache-independent): load the Store from the repo's
     `data/definitions/` and assert the accepted count is 30,827
     (30,828 − Elena), Elena is dropped, and the three
     spot-check spawns above resolve. (Path is relative to the
     package dir: `../../data/definitions`.)
5. Live verification (fresh temp root `/home/hatch/smoke39/`,
   real rev-242 cache, as in prior phases; do NOT touch
   `/home/hatch/live36` or `~/tsps-full`): new driver + runner
   script there. Reuse the smoke38 login/session helpers
   (`/home/hatch/smoke38/main.go`) and the smoke36 NPC sync
   parsing (`/home/hatch/smoke36/main.go`: `recordNpcInfo`,
   `parseNpcAdditions`, NPC_INFO opcode 21); attack via the
   smoke27 pattern (goblin Attack is option 2, opcode 12;
   `/home/hatch/smoke27/main.go:674`). Legs, run singly (the
   smoke36 lesson — a full-driver pass exceeds the
   background-exec wall clock):
   - `presence`: log in at the default spawn (3089,3524,0),
     wait for the drain (poll NPC_INFO until the Lumbridge set
     is complete, up to ~90 s), assert NPCs are in the client
     sync at their file coordinates — goblins (e.g. 3036 near
     3002,3206), chickens (1174 near 3225,3300), cows, and
     Lumbridge locals (e.g. Friendly Forester 12119 at
     3096,3238). At least one NPC of each named kind, with the
     right id at the right tile.
   - `wander`: snapshot one wandering NPC's (radius > 0)
     position twice ~60 s apart; assert it moved AND stayed
     within its radius of its spawn tile. (A radius-0 NPC, e.g.
     the Forester, must NOT move.)
   - `respawn`: attack a goblin until it dies (observe its
     removal from NPC_INFO), wait, and assert a goblin of the
     same id reappears at/near the same spawn tile (respawn
     machinery from Phase 16/26 firing for file-loaded spawns).
     If the kill leg cannot complete in the time available,
     say so honestly and leave it unit-covered — do not fake it.
5b. Live verification for the extra imports (same temp root,
   new driver modes):
   - `combatanims`: fight an NPC whose attack/block/death
     anims resolve from npc-combat-defs.json / npc-animations.json
     (pick the NPC from the load-prove's resolved values — it
     must differ from the 422/424/836 defaults, so the wire
     proves the file is feeding the game and not the cache
     scanner's guesses); assert the attack, block and death
     anims arrive on the wire with exactly the file's ids.
   - `shop`: open the General Store (shop id 0) and confirm
     its stock shows on the wire (interface + stock rows
     matching the file). Ground the open path first: how a
     player opens a shop in the Go server (ShopManager, Phase
     17) and which NPC/action opens shop 0 — the file's
     `npcInteractions` for shop 0 is empty, so the binding may
     live elsewhere; if no open path exists for shop 0, say so
     plainly instead of forcing one.
6. Docs: PORT_STATUS.md — the spawn-loader row (file now shipped;
   correct the `key` framing per the Elena note above; note the
   eager-vs-deferred divergence and the 150/tick drain); the
   data-import paragraph (npc-spawns.json + SHA-256); a new §22
   residual gap (Elena dropped; id-0 Tool Leprechauns; eager
   spawning; deferred activation NOT STARTED). Content row count
   unchanged — say so in the report.

## Deliberate non-changes (document, do not do)
- Do not hand-edit any entry in the imported file (Elena, the
  id-0 leprechauns, duplicates).
- Do not port `NpcIdentifiers` for one entry, do not build the
  deferred-activation machinery, do not raise `QueueDrainCap`.

## Explicitly out of scope
- `object-spawns.json`, `ground-items.json`,
  `dormant-npcs.json`, `monsters-complete.json`, the `::npc` /
  `::npcperm` spawn commands, NPC dialogue/combat content for the
  spawned NPCs, the client, the Android app. (`shops.json` was
  in this list in the original brief; Tyler's 2026-11-21 02:23
  PST scope expansion superseded that line — shops.json is IN
  scope, see Scope above.)
- No new dependencies.

## Results (2026-11-21, in progress)

- Imports done, all verbatim, SHA-256 identical to upstream:
  npc-spawns.json `aa6e04af...` (30,828 entries), npc-combat-defs.json
  `429e4b06...` (963 npcs + 179 refs rows + defaults), npc-animations.json
  `be12ec7d...` (3,383 entries), npc-animations.LICENSE `5dac153c...`,
  shops.json `60865b1f...` (487 shops, 5,471 stock lines). No wiring
  change was needed: the spawn loader and all three extra loaders
  already existed and read the repo's `data/definitions/` at boot.
- Extended load-prove (temporary test, deleted after the run):
  spawns accepted 30,827 (candidates 30,828, invalid 1 = Elena,
  unsupported 0, failures 0), cache NPC count 16,876; Rat 2854
  2705/2706/2707 (respawn 25, hp 2), Goblin 3036/3035 6184/6183/6182,
  Chicken 1174 5387/5388/5389, Imp 5007 169/170/172, Man 3108
  422/425/836; inferred Guard 3254 386/388/836, Dark wizard 512
  711/425/836; Forester 12119 the TS DEFAULT shape 422/-1/836
  (correction to req. 2b: the default block anim is -1, not 424 —
  424 is the player's default, not the NPC DEFAULT shape's;
  the forester's cache name is literally "null", the spawn file
  carries "Friendly Forester"); shops 487 / 5,471 stock lines,
  shop 0 "General Store" stock exactly 590/1351/1265/946/2347/1755
  × 2,000,000,000; `Store.Interaction()` empty (0 definitions).
  Loader log: `inferred=6439 (by name=2808), guessed=1225,
  by family=2476`.
- Boot cost (`run_smoke39.sh bootcost`): WITHOUT the spawn file
  2.85 s to started, WITH it 3.09 s (+0.24 s — negligible).
- Live legs so far (smoke39, run singly, real rev-242 cache):
  presence 7/7, lumbridge 6/6, chickens PASS (7 in view),
  goblins PASS (5 near (3002,3206)), wander PASS (Shop keeper
  2821 (3080,3511) → (3079,3512) within radius 3; Forester
  12119 unmoved 60 s). Presence lesson: Imp 5007 (~30 tiles)
  and Banker 1613 (~35 tiles) never enter sync — NPC view
  distance is ~15 tiles, so presence sets must use NPCs within
  ~13 tiles of the login tile.
- Shop leg finding (req. 5b, `shop`): NO client-reachable open
  path exists for any shop in the Go server, so the leg is
  recorded as a gap, not forced. The binding lives in the
  Shopkeepers plugin (231 lines, `plugins/npcs/Shopkeepers.plugin.js`
  :88/:196 — reads `shops.json` `npcInteractions` + custom-shops
  bindings, registers `core.shops.open` NPC interactions), which
  is NOT ported; the Go loader ignores `npcInteractions`,
  `loadInteractions` has zero contributed sources, and no shipped
  dialogue opens a shop (the `open_shop` transcript step exists
  but zero dialogue rows use it). Shop 0's own `npcInteractions`
  is empty in the file. The ShopManager holds all 487 shops with
  stock (load-proven above). Porting Shopkeepers is content work
  for a later phase; see PORT_STATUS §22(d).
- Rat-pen finding (respawn/combatanims legs, 2026-11-21): the
  Edgeville rats at (3119,3485) live inside a WALLED pen and
  cannot be killed from outside — this is correct game behavior,
  not a port bug. Evidence: unarmed AND armed (rune scimitar)
  attacks both ended with the server sending "I can't reach
  that!" and the target cancelled; playerAnims empty for 240 s;
  a walk probe (walk straight at (3119,3485)) stops at
  (3117,3485); a temporary RegionManager clip probe against the
  real rev-242 cache shows continuous wall lines around the pen
  (west x=3117/3118, north y=3488, east x=3125/3126, south
  y=3480/3481 — wall bits 0x8/0x80/0x20/0x2 plus projectile flags,
  wall objects 184/185 type 22 placed along the perimeter, a
  coherent rectangle, not random garbage). The pen is real cache
  data that TS reads too; the combat cycle's cancel is the
  faithful response to attacking through a wall. Both legs were
  therefore retargeted at Goblin 3036 at the Goblin House
  (3002,3206, open ground, Phase 29 precedent), armed with a
  rune scimitar: respawn PASS (goblin died, death anim 6182,
  count 1→0→1), combatanims PASS (attack 6184 / block 6183 /
  death 6182 all on the wire). The walkprobe mode stays in the
  smoke39 driver as the pen-access diagnostic.
- Live legs final (smoke39, run singly, real rev-242 cache):
  presence 7/7, lumbridge 6/6, chickens PASS (7 in view),
  goblins PASS (5 near (3002,3206)), wander PASS (Shop keeper
  2821 (3080,3511) → (3079,3512) within radius 3; Forester
  12119 unmoved 60 s), respawn PASS, combatanims PASS.
  (walkprobe FAILs by design — it proves the pen is walled.)

## Environment notes
- Go 1.25.1 at `~/workspace/tools/go/bin/go`. No new dependencies.
- The live36 servers (Go :43594, client :3000) are running for the
  parent's browser test — do NOT touch `/home/hatch/live36` or
  `~/tsps-full`'s running client. Use a fresh temp root for the
  live check, as prior phases did.
- Verification: `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`, full suite `go test ./... -p 1
  -count=1 -v` with exact pass counts, `go test -race ./...`.
