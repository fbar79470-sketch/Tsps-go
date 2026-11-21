# Phase 3 plan (COMPLETED 2026-11-14)

Goal: the first renderable world after login. Phase 2 delivered the
pieces (definition decoders, sync encoders, bootstrap); nothing calls
them per tick yet, and nothing loads definitions at boot. Phase 3
wires them together. It does NOT add movement, combat, skills,
dialogue, shops, or plugin content.

Status: completed 2026-11-14. Tyler approved Phase 3 at 2026-11-14
15:11 PST. All three plan items were implemented and verified:
boot-time definition loading with real run-energy weight; the
`flushClient` port into `Session.FlushTick` with visibility/update
buckets and local lists; and both live smoke tests (OpenRS2
download + definitions against the real rev-242 cache; binary
smoke test asserting a rebuild, initial sync, and a sync frame
every tick). Verification: `go build`/`go vet`/`gofmt` clean,
530 test cases passing (0 failures), `go test -race` clean, live
smoke tests passed (see `PORT_STATUS.md` for exact results,
including the finding that the rev-242 OpenRS2 entry ships zero
XTEA keys). The music/ground-item/NPC-effects blocks inside
`flushClient` were DEFERRED as the plan's default, with callouts
at the code sites and in `PORT_STATUS.md`. Out-of-scope items
(movement, save round-trip, JSON loaders, plugin strategy,
scrypt-hash check) remain untouched.

## 1. Boot-time definition loading

- Construct `CacheDefinitions` / `CacheMaps` / `Gamevals` from the
  ensured cache at startup in `cmd/server` (facades already exist:
  `NewCacheDefinitionsFromStore`, `NewCacheMapsFromStore`,
  `NewGamevalsFromStore` in `internal/cache`).
- This also resolves the Phase 2 bootstrap approximation "run-energy
  weight is sent as 0 (needs item definitions)" — weight can then be
  derived from real item definitions.
- Out of scope: custom-definition overlays
  (`registerCustomDefinitions` / `registerCustomItems`, need the JSON
  definition pipelines) and `NpcAnimationScanner`.

## 2. Per-tick sync / visibility wiring

Port `PlayerSession.flushClient` (TS `PlayerSession.ts:355-660`) into
`Session.FlushTick` (`internal/session/session.go:121`, currently
flushes the pending queue only), in this order:

1. Region-change detection + rebuild packets — encoders already
   exist (`EncodeRebuildNormal` / `EncodeRebuildRegion`,
   byte-exact, incl. XTEA keys for map decryption).
2. World visibility: local player/NPC lists and update buckets —
   TS `World.process` phases 13–15 (`rebuild_update_buckets`,
   `update_players_npcs`, `flush_network_players`,
   `reset_player_updates` / `reset_npcs`; see
   `~/workspace/tsps-go-analysis/ANALYSIS.md` §7). Per visibility
   player: collect nearby NPCs, `updateLocalPlayers`,
   `updateLocalNpcs`.
3. Sync-state ownership: `PlayerSyncState` / `NpcSyncState`, scene
   base coordinates, appearance cache, scene replay bookkeeping.
4. Per-tick frame: `EncodeTick` + `EncodePlayerSync` /
   `EncodeNpcSync` (encoders exist, byte-exact tested; no
   production caller yet) sent through the existing flush path.

Deferred within flushClient (call out explicitly, don't silently
skip): music area tracking / song-jingle sending and ground-item
region handling — port or defer as a named decision at the start of
the run. NPC-specific effects likewise.

## 3. Verification (settles two open items from Phase 2)

- Live OpenRS2 download smoke test: run `EnsureCache` against the
  live archive for target `osrs-242_2026-11-03`, then load
  definitions from the real cache — this is the first time the
  decoders run against a real rev-242 cache (all Phase 2 tests used
  hand-built fixtures / a synthetic store).
- Live smoke test of the built binary: boot, login, and assert a
  TICK / rebuild / sync frame arrives every tick after login
  (Phase 2's smoke test explicitly found none arrive).
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `go test ./...`
  and `-race` clean, as in Phases 1–2.

## Explicitly out of scope (next phases)

- Movement execution (`MovementQueue` + `PathFinder` +
  `RegionManager` collision; the `Location` translate `z` question
  belongs there) — walk/gameclick input is decoded and dispatched,
  requests still drop on the nil hook.
- Save round-trip (`applyToPlayer` / `fromPlayer`, autosave,
  SQLite), JSON definition loaders, packet listeners beyond the
  turn bridge, plugin-strategy decision (still the largest open
  architectural decision; blocks all content).
- Real-production scrypt-hash check (still open; needs a real TSPS
  password hash — separate from this phase unless Tyler supplies
  one).

## Acceptance

A client can log in and render the world: region rebuild + initial
player sync on login, then a sync frame every 600 ms tick, with
definitions resolving real item/NPC/object ids from the live cache.
