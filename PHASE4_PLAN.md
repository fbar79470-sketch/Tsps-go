# Phase 4 plan (COMPLETED 2026-11-14)

Outcome: COMPLETED and verified. All three pieces landed
(`RegionManager` + collision, `RsmodRouteFinding` pathfinder,
`MovementQueue` + wiring), plus the `Location.Transform`
z-preserving helper and the run-energy gap recorded in
`PORT_STATUS.md`. Final verification (2026-11-14/15): `go build` /
`go vet` / `gofmt` clean, 644 test cases passing (530 + 114 new),
`go test -race` clean. Live smoke tests against the real rev-242
cache: a WALK to (3100, 3538) arrived exactly; a WALK south
across the wilderness ditch stopped at the ditch edge
(3089, 3523) with the ditch tiles carrying flags `0x40100`;
no occupied tile was ever blocked and no step exceeded 2 tiles;
37 consecutive TICK packets each paired with PLAYER_SYNC +
NPC_SYNC, with sync payloads changing while moving and settling
after arrival. A built binary passed a separate boot + WALK
smoke check with clean SIGINT shutdown. Temp test files, the
temp data root, and all cache binaries were removed afterward.
Remaining gaps (explicit): run-energy drain/restore, the
destination-flag packet encoder, the run-toggle listener,
`blockMovement`-only mobility gate, no dynamic occupancy, no
NPC drivers — see `PORT_STATUS.md`.

Goal: walk clicks actually move the player. Phase 3 delivered a
renderable world (definitions at boot, per-tick sync); WALK /
MOVE_GAMECLICK input is decoded and dispatched, but
`Player.OnMovementRequest` is nil, so requests drop and no position
ever changes. Phase 4 ports the movement core — `MovementQueue` +
`PathFinder` / `RsmodRouteFinding` + `RegionManager` collision —
and wires it so position changes flow through the existing per-tick
sync machinery. It does NOT add NPC behavior, combat, skills,
dialogue, shops, sailing, template regions, saves, JSON loaders,
or plugin content.

Status: approved 2026-11-14 (Tyler: "Keep going with movement +
collision"). Baseline verified before work started: `go build` /
`go vet` / `gofmt` clean, 530 test cases passing, `go test -race`
clean.

## 1. RegionManager + collision from cache maps

- Port `RegionManager` (TS `game/collision/RegionManager.ts`, 1,033
  lines) + `Region` (clip grid, roof/water tiles) into
  `internal/game`, reading region data through narrow interfaces
  (`RegionDataSource`, `ObjectDefSource`) implemented by a
  `cmd/server` adapter over the existing `CacheMaps` /
  `CacheDefinitions` facades. No `internal/game` import of
  `internal/cache` (existing seam convention).
- Region loading: lazy `loadMapFiles` semantics (load on demand +
  surroundings), terrain decode (new format, revision >= 209) with
  blocked-tile clipping from the height map, object decode
  (smart-int streams) with clipping from Loc definitions
  (`clipType`/`isSolid`/`isClippedDecoration`/`isImpenetrable`),
  roof/water tile tracking, `getClipping` (0 for a missing region,
  as in TS), `canMove`/`canMovestart` + directional `blocked*`
  helpers, add/remove clipping + object clipping. Private areas
  do not exist in Go: no `privateArea` parameter anywhere.
- Out of scope: projectile methods (`canProjectile*`), `wallExists`
  (needs `MapObjects`), `MapRegionReplacementManager`, template
  regions, sailing.

## 2. PathFinder

- Port `RsmodRouteFinding` (TS
  `game/model/movement/path/RsmodRouteFinding.ts`, 1,556 lines)
  whole and faithfully — BFS `routeFindSize1` / `routeFindBig`,
  wall/rectangle strategies, closest-approach fallback, waypoint
  extraction with the 25-checkpoint cap — reading clipping through
  a `CollisionLookup` function. This is the algorithm
  `MovementQueue` actually calls (via `PathFinder.applyRsmodRoute`
  → `findRoute`); no guessed algorithm.
- Port the walk-route entry only (`calculateWalkRoute` shape:
  `locShape -1`, `moveNear` true). Combat/entity/object/ground-item
  route wrappers are NOT STARTED (their callers do not exist).

## 3. MovementQueue + wiring

- Port the queue/stepping core (TS `MovementQueue.ts`, 2,058 lines;
  the core subset): points deque (cap 100), `requestWalk`,
  `checkDestination` (same-`z` + bounds), `reset`, `beginCycle`,
  `process` (1 step walking / 2 running per tick, checkpoint
  consumption, walking/running directions + facing), `validatedStep`
  (diagonal-first, then axis steps), `canWalkTo` (static clipping;
  dynamic occupancy degenerates — no NPC/player occupancy systems
  exist yet), `isRunToggled` + run/click-run overrides,
  `handleRegionChange` (`LastKnownRegion` part only),
  destination-flag tracking (no packet: the encoder does not
  exist). Following, pursuit, forced movement, freeze/stun,
  combat/trade/duel mobility gates, and the plugin walk-route
  event are NOT STARTED and must be called out, not silently
  skipped.
- Wire `Player.OnMovementRequest` via the existing
  `cmd/server` TurnHandler / `MovementRequest` path: destination
  `z` is the player's current level (TS `walk()`), run override is
  the WALK force-run bit or `clickRunOverride` for gameclicks.
  `Player.Process` runs the queue after timers, in TS turn order.
  `World` owns the `RegionManager` (empty/open-ground manager when
  no cache is present, matching TS missing-region semantics).
- `Location` `z`: the recorded decision stands — preserve `z`
  (TS `move`/`translate` dropping `z` is an upstream bug;
  movement uses z-preserving `transform()`). Port `Transform`;
  reconcile the stale "must decide" comment in `location.go` with
  the recorded decision. If upstream evidence contradicts the
  resolution, stop and flag it instead of resolving silently.
- Run energy: drain is recorded as an explicit gap unless it falls
  out of ported state without inventing `SkillManager` (it needs
  Agility + live energy units + run-status packets, none of which
  exist). The decision must be written down in `PORT_STATUS.md`
  either way; running still steps twice per tick.

## 4. Verification

- Fixture-only unit tests (no network): collision-flag tests,
  pathfinder tests (open ground, wall blocking, diagonal/corner
  rules, unreachable destination), queue stepping tests (walk/run
  per-tick behavior), integration tests proving a
  `MovementRequest` moves a player over successive ticks.
- Live smoke test (authorized, Phase 3 pattern): boot against the
  REAL cache via `EnsureCache` in a TEMP data root on the main
  filesystem OUTSIDE the repo (`/tmp` is a 512 MB tmpfs and does
  not fit the cache), log in over real WebSocket, send a WALK
  packet, assert the position changes over ticks and sync frames
  reflect it. Collision-against-a-blocked-tile live only if a
  grounded case exists in real map data; otherwise fixture-level
  only, stated as such. Delete the temp root and all cache
  binaries afterward; no cache binaries under the repo tree.
- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `go test ./... -p 1 -count=1` and `-race` clean (serialized:
  known `TestEngineTicksAtRate` parallel flake; do not change its
  tolerance unless it blocks verification).

## Explicitly out of scope (next phases)

- NPC AI / wandering / following (shared queue/pathfinder/collision
  machinery may be Mobile-generic as TS structures it, but no NPC
  behavior drivers; NPCs get no attached queue this phase).
- Combat, skills, dialogue, shops/trading; sailing/boats; template
  regions and `MapRegionReplacementManager`; save round-trip; JSON
  loaders/spawns; packet listeners beyond the existing turn bridge;
  deferred flush blocks (music/ground items/NPC effects);
  plugin-strategy decision; real-production scrypt-hash check.

## Acceptance

A live client login + WALK packet moves the player: position
changes over successive ticks, sync frames reflect the movement,
and collision blocks movement into blocked tiles (fixture-proven
at minimum, live-proven if a grounded real-map case exists).
