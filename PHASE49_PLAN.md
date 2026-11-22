# Phase 49 Plan — Ladders

Tyler's order: "Add npc dialogs, working doors and latters" (2026-11-21 20:55 PST).
Dialogs (Phase 47) and doors (Phase 48) are done; ladders are next. Phase 49.

Baseline at dispatch: HEAD 576a947, 5,182 tests passing, 31 packages,
content 24 of 577.

## §1 Survey (coordinator, 2026-11-22, read upstream + Go seams directly)

### §1.1 What upstream does

**`server/plugins/objects/Ladders.plugin.js`** (241 lines, upstream HEAD
`bfd81e3`) is the whole generic climb system. It registers, by object
*name* (`api.onObjectInteraction(name, options)` — hook-by-name, AGENTS.md
rule 4), 38 climbable names:

> "Ladder", "ladder", "Staircase", "Stairs", "Steps", "Trapdoor",
> "Manhole", "Spiral staircase", "Vine ladder", "Ship's ladder",
> "Bamboo Ladder", "Metal ladder", "Iron ladder", "Rope ladder",
> "Stone ladder", "Stone Ladder", "Copper Ladder", "Tower ladder",
> "Boney ladder", "Kings' ladder", "Troll ladder", "Ladder top", "Rope",
> "Climbing rope", "Escape rope", "Anchor rope", "Rope anchor",
> "Old ruin entrance", "Exit", "Stairs up", "Stairs down",
> "Wooden Stair", "Stone Staircase", "Crystal Staircase",
> "Crypt staircase", "Broken stairs", "Cellar stairs", "Stairwell"
> (`Ladders.plugin.js:21-38`)

Options per direction, exactly as the cache spells them
(`Ladders.plugin.js:40-50`):

- Up: "Climb-up", "Climb up", "Climb Up", "Walk-up", "Walk-Up", "Ascend"
- Down: "Climb-down", "Climb down", "Climb Down", "Walk-down", "Walk-Down",
  "Descend"
- Top-floor: "Top-floor", "Top-Floor"; Bottom-floor: "Bottom-floor",
  "Bottom-Floor" (straight to the last floor, `toEnd`)
- Ambiguous "Climb": asks "Which way would you like to climb?" via
  `sendMultiChatboxPrompt`, then hands off to the normal named handlers
  (only if the player has not moved)
- "Gangplank" `{ Cross: crossOption }` — dock ↔ ship deck, no climb
  animation

**The climb itself** (`Ladders.plugin.js:101-131` `climb()`):

- Refuses when `movement.isMovementBlocked()`.
- Blocks movement, resets the queue, submits a 1-cycle `Task` (an op that
  moves the player waits a cycle after arriving).
- On execute, if the player is still on the start tile, has HP > 0, and is
  in the same private area: plays the animation first (tick 1), then
  `player.moveTo(target)` (tick 2); `onArrival` runs right after the move.
  A refused/cancelled climb never calls `onArrival`. `stop()` unblocks
  movement.
- Animations: `CLIMB_UP = 828`, `CLIMB_DOWN = 827`; `animate: false`
  (walked stairs) plays none.

**Direction resolution** (`climbUp`/`climbDown`, `Ladders.plugin.js:162-185`):

1. Explicit `event.destination` (from the `ladders:climbUp` /
   `ladders:climbDown` custom events) is trusted.
2. Else the map decides via `ClimbLinks.destination` (below).
3. Else the last resort: the player's own tile one plane up/down —
   but only if an object of the same name sits right above/below
   (`hasObjectAbove`/`hasObjectBelow`, 3×3 search on the destination
   plane). A climb never lands on the ladder's own (blocked) tile.
   Out of plane range (z < 0 or z > 3) → no climb.

**Claiming** (`claimedElsewhere`, `Ladders.plugin.js:201-207`): content
that owns a ladder (e.g. Castle Wars) claims the click first via the
`ladders:climb` custom event (`{player, object, objectId, clickType,
handled}`); if `handled`, the generic handlers never run.

**`server/plugins/objects/ClimbLinks.js`** (~200 lines) resolves where a
climb leads, from the map itself (issue #119):

- The other end must have a climb option in the opposite direction
  (`UP_OPTIONS` = climb-up/climb/top-floor/climb up/walk-up/ascend;
  `DOWN_OPTIONS` = mirror; case-insensitive).
- Search: same column on the next plane, radius `SEARCH_RADIUS = 4`
  (5 with the +1 in `sameColumn`), nearest by footprint distance.
- Underground: areas lie `UNDERGROUND_OFFSET = 6400` tiles north of the
  surface. Going down from plane 0 (or up from y ≥ 6400) searches ±6400
  in y on plane 0. Lumbridge's trapdoor at (3209,3216) opens over the
  cellar ladder at (3209,9616).
- Inert segments: a same-name, non-climbable object in the same column
  (e.g. Goblin Village's middle ladder on plane 1) unlocks skipping
  that plane; chains are followed, bounded by the planes.
- Declared links first: `server/plugins/objects/data/climb-links.json`
  (2,669 bytes, **7 pairs**) covers arrangements the search cannot
  derive; `landing` / `downLanding` override the computed landing tile.
  The 7 pairs: Lady Lumbridge's hold ladder (Port Sarim), Port Sarim Rat
  Pits manhole, Wizards' Tower basement ladder, Forthos Ruin spiral
  staircase, Grand Tree base staircase, Grand Tree cave ladder,
  Ardougne Castle main staircase.
- Landing tile: walkable tiles adjacent to the other end (distance 1
  from its footprint) on a side it can be used from (access mask +
  rotation, the same reach check object clicks use), nearest the tile
  the player climbed from (with the ±6400 shift applied to the hint
  when the ends are that far apart in y).
- Top-floor / Bottom-floor (`toEnd`): keep going while the next end
  climbs on in the same direction (max 3 more floors).
- Gangplank `crossDestination`: the other half is the gangplank of the
  same name on the other plane within `GANGPLANK_RADIUS = 2`; the
  landing carries on 3 tiles past it in the direction of travel.

**Consumers of `ladders:climbUp` / `ladders:climbDown`** (explicit
destinations — area/quest plugins, NOT in this phase's scope):
FossilIslandUnderwater, Karamja, Keldagrim, MiningGuild, Misthalin
(Edgeville dungeon, Varrock sewers, Wizards' Tower basement/surface),
SlayerTower, WerewolfAgility, StrongholdOfSecurity Travel,
CountDraynor, KingBlackDragon, Brimhaven Dispensers, CastleWars
Navigation, GamesRoom Access, DT2 Prison, Heart of Darkness Tower,
AnotherSliceOfHAM, CabinFever, ClockTower, and more (~20 files
reference `ladders:*` events; Worker B counts exactly). These plugins
do not exist in Go; the events just need to exist and be emittable,
with the generic handlers as the fallback.

### §1.2 What the Go side already has (coordinator, file:line cited)

- Object interaction by name: `Api.OnObjectInteractionNamed`
  (`internal/plugins/objects.go:473-486`) — the doors/woodcutting
  precedent; the Ladders plugin registers 38 names × option maps
  through it. No new hub family is expected.
- Multi-option prompt: `Api.SendMultiChatboxPrompt`
  (`internal/plugins/dialogue.go:413-425`) — the `promptClimb`
  "Which way?" prompt rides it (proven by Smithing/Crafting).
- Animation: `(*Player).PerformAnimation(id, delay)`
  (`internal/game/player.go:704`, via `Mobile.PerformAnimation`,
  `internal/game/mobile_animation.go:157`).
- Instant move: `(*Mobile).SetLocation`
  (`internal/game/entity.go:205`); sailing teleports with it
  (`internal/game/sailing.go:965`). Player-death respawn and the
  Wilderness Ditch ForceMovement (`internal/game/forcemovement.go`,
  Phase 40) are the nearest precedents for "player ends up elsewhere";
  Worker A grounds which move path the climb task must use
  (movement-queue reset + block, as upstream does).
- Object lookup by tile: `ObjectRegistry.ObjectsAtTile(loc)`
  (`internal/game/objects.go:1314`). Whether placed *map* objects on
  another plane are queryable at runtime (ClimbLinks' `objectsAround`
  needs it) is Worker A's first grounding question — see
  `internal/game/regions*.go` (map region decode) and how the doors
  startup catalog (`internal/content/doors/catalog.go`) derives its
  556 pairs from the cache at boot. The climb resolver may need a
  similar startup scan, or a runtime map-object query if one exists.
- Delayed tasks: Smithing/Crafting/EnchantJewellery schedule 1-tick
  `TaskManager` tasks from content (Phase 45's startup-order race fix
  in `cmd/server/server.go` applies — schedule from startup hooks only
  after the engine is up, or from gameplay handlers, never during
  `Start()` before the engine runs).
- Doors precedent (`internal/content/doors/`): startup cache-scan
  catalog + toggle engine + `crossingFor`, wired in `cmd/server`
  (see `PHASE48_PLAN.md` §3 for the wiring shape to copy).
- Custom events: `Api.OnCustomEvent` / `EmitCustomEvent` exist
  (used by doors' `door:toggle` etc.); `ladders:climb`,
  `ladders:climbUp`, `ladders:climbDown` are new event names on the
  existing mechanism.
- Clip/blocked checks: `internal/game/collision.go` and the
  `combatClipping` source (`internal/game/combatant.go:507-513`);
  Worker A grounds the exact `blocked(tile)` equivalent for the
  landing-tile search.

### §1.3 Cache counts (to be confirmed by Worker A's probe)

Worker A runs a temporary probe (a `cmd/cacheprobe`-style program in
the repo, deleted after use, as in Phases 45/48) against the real
rev-242 cache and reports exact numbers in §8:

- Distinct object definitions whose interactions include a climb
  option (up/down/top/bottom/climb, case-insensitive), and how many
  of the 38 upstream names actually occur in the cache.
- Placed instances of climbable objects (map placements), and how
  many of the 7 declared climb-link pairs resolve to real placements.
- The Lumbridge/Varrock live-slice candidates below, confirmed
  present with ids, names, options, tiles and planes.

### §1.4 Live slice (sized honestly)

The mechanism is the phase; the slice proves it on real ladders:

1. **Lumbridge castle staircases** — Climb-up / Climb-down plus
   Top-floor / Bottom-floor (multi-floor `toEnd` chaining).
2. **Lumbridge cellar trapdoor** (3209,3216) → cellar ladder
   (3209,9616) and back up — the underground-offset path; closes
   gap 31(d) ("Trapdoors that Climb-down NOT STARTED").
3. **Wizards' Tower basement ladder** (down 2147 at (3104,3162) ↔ up
   2148 at (3103,9576)) — a declared-link pair.
4. **Grand Tree ladder column** (2466,3495) if the probe confirms it
   — a same-column multi-plane climb (Top-Floor spelling variant).

Gangplank Cross: mechanism ported + unit-covered; live only if the
probe finds a reachable gangplank pair (Port Sarim / Lady Lumbridge
is the candidate). If none is reachable in the slice, it stays
unit-covered, stated plainly.

## §2 Scope (sized honestly)

**In scope:**

- The climb mechanism: `ClimbLinks` destination resolution
  (declared links + other-end search + inert-segment chains +
  underground offset + landing-tile search + `toEnd` chaining),
  the climb task (movement block, 1-cycle anim, plane move,
  `onArrival`, private-area/HP guards), `hasObjectAbove`/`hasObjectBelow`
  fallback, ambiguous-"Climb" prompt, `claimedElsewhere`
  (`ladders:climb` event), Gangplank Cross, and the
  `ladders:climbUp` / `ladders:climbDown` custom-event handlers
  (explicit destinations trusted).
- The Ladders plugin registrations: all 38 names, all up/down/
  top/bottom options, "Climb" prompt option, Gangplank Cross.
- The 7 declared climb-link pairs, **transcribed into a Go table
  with per-pair JSON line citations** (the Phase 33 transcription
  precedent — transcribing upstream table values into Go source with
  citations is normal porting methodology; the file itself is NOT
  copied as an asset).
- Live verification of the §1.4 slice.

**Out of scope (residuals, gap 32):**

- All `ladders:climbUp` / `ladders:climbDown` *emitters* (area/quest
  plugins: Karamja, Keldagrim, Misthalin, SlayerTower, Werewolf
  Agility, Stronghold, Castle Wars, quests, …) — the events exist
  and the generic handlers are the fallback; the area plugins
  themselves do not exist in Go.
- Quest-gated or instanced-destination ladders whose guard systems
  do not exist.
- Any ladder whose other end the map search cannot derive and which
  is not in the 7 declared pairs (reported as found, not forced).

## §3 Port plan

- **Worker A — ClimbLinks + climb task.** The destination resolver
  (declared-links table, `otherEnd`, `landingTile`, `destination`,
  `crossDestination`, `climbs`), the climb task (block, anim 828/827,
  delayed plane move, `onArrival`), the above/below fallback, and
  the cache probe (§1.3). Pure functions of narrow cache/map
  interfaces (doors' `CatalogDefs` precedent) so the resolver is
  unit-testable with fixture defs/objects. Unit tests for: declared
  pair both directions + landings, same-column search, underground
  ±6400, inert-segment chain skip, no-other-end → no climb,
  landing-tile nearest + blocked-tile exclusion, `toEnd` chaining,
  gangplank pair + landing, climb task tick order (anim tick 1,
  move tick 2), movement-blocked refusal, moved/died mid-climb
  cancellation, `onArrival` only on success.
- **Worker B — Ladders plugin + wiring.** `internal/content/ladders/`
  (plugin `Register`: 38 names × option maps, "Climb" prompt,
  Gangplank, `ladders:climb` claim, `ladders:climbUp`/`climbDown`
  custom-event handlers), wiring in `cmd/server` (blank import +
  `Load` order — after doors, before/after existing content as the
  hub order requires; ground it against `pluginwiring.go`), unit
  tests for option routing (up/down/top/bottom/prompt/claimed).
  Counts the `ladders:*` event references upstream exactly.
- **Coordinator integration:** merge, run §5, write the smoke
  harness (`/home/hatch/smoke49/`, reusing the smoke48 WS driver
  shape), run the §1.4 legs singly, fill §8, update PORT_STATUS.

## §4 Live verification (harness /home/hatch/smoke49/)

Real server binary + real rev-242 cache, fresh temp root per leg,
legs run singly. Each leg: log in near the ladder, click the option,
observe on the wire (animation id, plane change, landing tile) and
in the save (position after relogin):

- `stairs-up` — Lumbridge castle staircase Climb-up: plane +1,
  landing tile adjacent to the upper staircase.
- `stairs-down` — same staircase Climb-down back: plane −1.
- `top-floor` — Top-floor option: lands on the top floor.
- `trapdoor-down` — Lumbridge cellar trapdoor Climb-down:
  (3209,3216,0) → cellar (3209,9616,0) area.
- `trapdoor-up` — cellar ladder Climb-up back to the surface.
- `tower-down` / `tower-up` — Wizards' Tower basement ladder both
  ways (declared link).
- `prompt` — ambiguous "Climb" option shows the prompt; choosing
  up climbs up.
- `gangplank` — only if a reachable pair is confirmed by the probe.
- `no-end` — a climbable object with nothing at the other end
  does nothing (negative leg; find one in the probe).

## §5 Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...` — all clean.
- Full suite `-p 1 -count=1 -v` — report top-level + subtest counts,
  packages, failures, skips.
- `go test -race ./...` — the known pre-existing
  `TestEngineTicksAtRate` 5 ms timing flake in `internal/game`, the
  known §20 `cmd/server` race-load timing issues, and the known
  `TestReloginRoundTripOverWebSocket` disconnect-save timeout are
  NOT data races; if the race run fails only on one of those with no
  DATA RACE warning, re-run that package once before reporting. Any
  DATA RACE warning is a real finding.
- Probe program deleted after use; no `.js` files; no stray
  artifacts; `git status` shows only Phase 49 files.

## §6 Residuals (gap 32)

- (a) All `ladders:climbUp` / `ladders:climbDown` *emitters* NOT
  STARTED: 23 upstream files emit them (17 climbUp, 11 climbDown;
  areas Karamja, Keldagrim, Misthalin, SlayerTower, WerewolfAgility,
  StrongholdOfSecurity Travel, KingBlackDragon; quests DT2 Prison,
  ClockTower, CurseOfArrav, DefenderOfVarrock, DragonSlayer,
  GrandTree, PriestInPeril, QueenOfThieves, RecipeForDisaster,
  RumDeal, SeaSlug, SongOfTheElves, TheBloodMoonRises, TowerOfLife,
  TribalTotem — Worker B's exact count). The events exist and the
  generic handlers are the fallback; the area/quest plugins
  themselves do not exist in Go.
- (b) All 63 `ladders:climb` *listeners* NOT STARTED (same 23-file
  set plus quest claimants; Ladders itself is the only
  `ladders:climbUp`/`ladders:climbDown` listener). No area owns a
  ladder in Go yet, so every climb runs the generic path.
- (c) Quest-gated or instanced-destination ladders whose guard
  systems do not exist are untested and unported (same class as
  (a)/(b)).
- (d) Gangplank landing: the live leg landed 1 tile past the far
  gangplank, not the 3 tiles `crossDestination` describes —
  `landingTile` picks the nearest walkable/reachable tile to the
  3-past hint, and the deck there is only reachable adjacent to
  the far half. Behavior, not a bug (the leg's within-4 check
  passed); recorded so a future reader does not "fix" it.
- (e) TS task immediacy (`Ladders.plugin.js:116`,
  `!movement.didMovePreviousCycle()`): Go always uses a delay-1
  non-immediate task — the climb entry carries no cycle to evaluate
  against. Pinned order (anim tick 1, move tick 2) matches the
  plan and TS's common (didn't-move) case.
- (f) Grand Tree cave down end (climb-links.json:37-39): id 2446
  offers only "Open", so the resolver's `climbs()` gate refuses
  climbing *down* from it — exactly as upstream `destination()`
  does. Climbing *up* from 17385 returns the declared landing
  (2463,3498,0).
- (g) Unknown definitions: TS throws on a null def in
  `footprint()`; Go treats an unknown def as 1×1 with no options.
  `CrossDestination` returns nil when the def name is unknown
  (TS would throw). Neither was observed live.
- (h) Gap 31(d) (trapdoors that Climb-down) is CLOSED by this
  phase: the Lumbridge cellar trapdoor climbs down and back up
  live.

## §7 Data files needing Tyler's import approval

None. The 7 declared climb-link pairs are transcribed into Go with
citations (Phase 33 precedent); no file is copied. No new data file
is needed: the resolver reads the real rev-242 cache at runtime (or
a startup scan of it, per Worker A's grounding).

## §8 Results

### What was ported

- `internal/content/ladders/resolver.go` — `Resolver`, the Go port
  of `ClimbLinks.js`: `Climbs` (case-insensitive UP/DOWN sets),
  `OtherEnd` (declared links first, same-column search radius 4+1,
  inert same-column segment chains, underground ±6400),
  `LandingTile` (distance-1 ring, blocked check, reach check with
  unturned sizes/face/type/clipMask, nearest-to-hint stable sort),
  `Destination` (declared-landing precedence, `toEnd` chaining max
  3), `CrossDestination` (radius 2, landing 3 tiles past the far
  half), `HasObjectAbove`/`HasObjectBelow`/`HasObjectOnFloor` (3×3).
  Pure function of narrow interfaces (`Defs`, `MapObjects`,
  `BlockedFunc`, `ReachFunc`).
- `internal/content/ladders/links.go` — the 7 declared climb-link
  pairs transcribed from `climb-links.json` with per-pair line
  citations (lines 5-8, 11-15, 18-20, 24-27, 30-33, 37-39, 42-46).
  The file itself was NOT copied.
- `internal/content/ladders/climb.go` — `StartClimb`: movement-blocked
  refusal; block + queue reset; 1-cycle task; anim tick 1 (828 up /
  827 down, or none when `animate: false`), move + `onArrival`
  tick 2; cancel if moved/died/changed private area; unblock on
  stop and on every terminal path. The move uses
  `game.MovePlayerTo` (sailing's `Mobile.moveTo` form), not bare
  `SetLocation`.
- `internal/content/ladders/mechanism.go` — `ClimbMechanism`
  (implements Worker B's `Climber`): explicit destination trusted,
  else map, else above/below fallback on the player's own tile;
  plus the production adapters `RegistryObjects`,
  `RegionBlocked`, `RegionReached`.
- `internal/content/ladders/ladders.go` — the plugin `Register`:
  39 `OnObjectInteractionNamed` calls (all 38 climbable names from
  `Ladders.plugin.js:21-38`, exact strings, upstream order, plus
  `Gangplank`), 17 options per climbable name (`Climb` prompt +
  Top/Bottom-floor variants + 6 up + 6 down spellings), `Gangplank`
  gets only `Cross`, 2 `OnCustomEvent` (`ladders:climbUp`,
  `ladders:climbDown`), and the `ladders:climb` claim event emitted
  before every generic handler.
- `cmd/server/ladderswiring.go` + `ladderswiring_test.go`
  (coordinator) — `installLaddersMechanism`, called from
  `server.go` after the RegionManager install (the Blocked/Reach
  adapters capture that manager); `cacheLadderDefs` adapts the
  cache definitions to `ladders.Defs`. Nil-defs install degrades
  to fall-through, pinned by tests.
- `cmd/server/pluginwiring.go` (+ test) — blank import in
  alphabetical slot; load order F2P `Doors`, `Ladders`,
  `LumbridgeGuide` (members: `Doors`, `GracefulOutfit`,
  `Ladders`, `LumbridgeGuide`).

### Grounding answers found in-phase

- Runtime map-object query EXISTS in Go: `ObjectRegistry.
  ObjectsAtTile` syncs the tile's region on demand, so placed map
  objects on any plane are queryable at runtime — no startup cache
  scan was needed (the doors catalog precedent was not followed
  here, deliberately).
- Cache probe (real rev-242 cache, temp program deleted after
  use): 63,334 object defs; 2,950 regions; **1,511 defs with a
  climb option**; all **38/38** upstream names present; **5,003**
  placed climbable instances (largest: "Ladder" 2,074,
  "Staircase" 786, "Stairs" 680); **7/7 declared pairs resolve
  to real placements (14/14 endpoints)**; 97 gangplank
  placements. (First probe run wrongly counted `Cross` as a climb
  option — 1,719 defs / 5,889 placed — corrected before reporting.)
- Upstream `ladders:*` references (exact, Worker B): 162 lines in
  80 files; 17 comment-only, so **145 code references**. Emitters:
  23 files (climbUp 17, climbDown 11, climb 1). Listeners: 64
  files (63 listen to `ladders:climb`). 14 files both emit and
  listen.

### Live verification (harness `/home/hatch/smoke49/`, real server binary + real rev-242 cache, legs run singly, port 47119) — 10 of 10 PASS, 38 CHECK pass, 0 CHECK FAIL

| Leg | Anim | Before → after (x,y,plane) |
|---|---|---|
| stairs-up (56230 Climb-up) | 828 ×1 | (3206,3207,0) → (3206,3208,1) |
| stairs-down (16672 Climb-down) | 827 ×1 | (3206,3207,1) → (3206,3208,0) |
| top-floor (56230 Top-floor) | 828 ×1 | (3206,3207,0) → (3205,3209,2) |
| trapdoor-down (14880 Climb-down) | 827 ×1 | (3208,3216,0) → (3210,9616,0), y +6400 |
| trapdoor-up (17385 Climb-up) | 828 ×1 | (3208,9616,0) → (3210,3216,0), y −6400 |
| tower-down (2147 Climb-down, declared) | 827 ×1 | (3103,3162,0) → (3104,9576,0), y +6400 |
| tower-up (2148 Climb-up, declared) | 828 ×1 | (3103,9575,0) → (3105,3162,0), y −6400 |
| prompt (16672 Climb) | 828 ×1 after answering | prompt script 58 "Which way would you like to climb?", answered "Climb up" → (3205,3209,2) |
| gangplank (55332 Cross) | none | (1455,2968,0) → (1458,2968,1), on the deck |
| no-end (32401 Climb-up, plane 3) | none | (1482,9950,3) → (1483,9951,3), plane stays 3 |

No sounds and no chats on any leg (climbs are silent). Two
harness premises were corrected live, not the engine: (a) the
"object in login snapshot" check is invalid — the server
legitimately omits static locs from the login snapshot (the
client has the map from the cache; the smoke48 artifact); (b)
the no-end "player did not move" check was too strict — the
click routing walks the player up to the ladder before the
handler refuses, so the check is now "adjacent to the ladder,
no plane change, no anim". No port bugs found.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...` — all clean (888 `.go` files,
  32 packages; new `internal/content/ladders`).
- Full suite `-p 1 -count=1 -v`: **5,247 tests (4,003 top-level
  + 1,244 subtests), 0 fail, 0 skip, 32 packages** (5,182 from
  Phase 48 + 65: Worker A 28, Worker B 10 top-level + 25
  subtests, coordinator wiring 2).
- `go test -race ./...`: **clean on the first full run (exit 0,
  0 DATA RACE warnings, 32/32 packages ok)**. No known-flake
  re-runs needed this phase.
- Probe programs (`cmd/cacheprobe`, `cmd/probe49tmp`) deleted
  after use; no `.js` files; `git status` shows only Phase 49
  files; nothing touched in `/home/hatch/live36`, `~/tsps-full`,
  or upstream.

### Unit-covered only (not observed live)

- Inert-segment chain skips, `toEnd` chaining beyond the
  Lumbridge 0→2 case, gangplank landings other than the one
  probed, the `ladders:climb` claim path with a real claimant
  (no claimant exists in Go), `animate: false` walked stairs
  (no walked-stairs consumer in the slice), movement-blocked /
  died-mid-climb cancellation (task-level tests only).
