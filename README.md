# tsps-go

A server-only Go port of [TSPS](https://github.com/RSPSApp/tsps) — the
TypeScript Old School RuneScape private server (server component,
commit `bfd81e3`, cache target `osrs-242_2026-11-03` per `target.txt`).
The TypeScript client is kept as-is; only the game server is being
ported. See [NOTICE](NOTICE) for upstream credits and licensing, and
[PORT_STATUS.md](PORT_STATUS.md) for exactly what is ported, what is
verified, and what comes next.

This is a fan project, not affiliated with Jagex Ltd. Old School
RuneScape assets belong to their respective owners. Cache data is
never bundled: the server downloads the cache at runtime from the
OpenRS2 archive (https://archive.openrs2.org).

## Status

Foundation stage, runnable but not yet playable. The server boots,
loads configuration, ensures the cache, accepts WebSocket
connections, completes the login handshake through the
LOGIN_RESPONSE packet, queues the player into the world, sends
the full player-bootstrap packet sequence, and runs the 600 ms
tick loop. Definitions load at boot from the cache, and the
per-tick flush sends a rebuild, an initial sync, and then a sync
frame every 600 ms tick, so a client can render the world after
login (verified live against the real rev-242 cache — see
[PORT_STATUS.md](PORT_STATUS.md)). Walk clicks move the player
with real map collision (movement queue + route finder +
region manager, also verified live). Running drains run energy
and stops at zero, and the run toggle works. Player saves round-trip
on login, logout, autosave, and shutdown (JSON, SQLite, and
in-memory backends). JSON definition data loads at boot, NPC
spawns enter the world, music areas play their tracks as the
player moves, and ground items from the definition data are
visible in the world. Live inventory, equipment, bank, skills,
and appearance systems exist: items can be equipped, dropped,
picked up, and reordered in the inventory, with carried weight
and run-energy drain following the live contents (all verified
live). NPCs wander, aggress, and fight with hitsplat sync, and
players can attack, kill, and watch them respawn through hub
death hooks (all verified live). Every packet listener decodes its packets (acting only
where the target system exists). A plugin hub exists
(compile-time Go content registration with lifecycle hooks,
world.json switches, and config declarations), with one
ported content piece: the date_minutes varp sender. The
content migration plan (`CONTENT_MIGRATION_PLAN.md`) lays out
the engine-systems-first ordering for the rest. Do not expect
OSRS parity from this run.

Two facts about TSPS shape this port and are easy to get wrong:

- There is **no JS5/update server and no TCP game listener**. The
  server reads the cache from local disk only, and the game
  transport is WebSocket binary with a custom protocol (1-byte
  opcode + optional length, big-endian fields, latin1 strings, no
  encryption). This port implements that protocol, not classic
  RS/ISAAC login.
- The analyzed cache target is **rev 242** (`osrs-242_2026-11-03`),
  although some TS comments still say 241. Login rejects clients
  reporting any other revision.

## Prerequisites

- Go 1.25 or later.
- No other toolchain. The only external Go dependencies are
  `golang.org/x/crypto` (scrypt), `github.com/gorilla/websocket`
  (game transport), and `modernc.org/sqlite` (pure-Go SQLite
  save backend, no CGO).

## Building and testing

```sh
go build ./...
go vet ./...
go test ./...          # fixtures only: no network, no cache binaries
go test ./... -race    # also passes
```

## Running

The process runs with its working directory set to the data root
(the `--root` flag, default `.`), which must contain `target.txt`
and `data/definitions/world.json`, exactly like the TypeScript
server directory. This repository ships a minimal starter
`data/definitions/` (world name, spawn, no zones, plus empty
valid definition files for the loaders that require them, and
the real `music-areas.json`/`music-areas-classic.json` imported
verbatim from TSPS) and `target.txt`; for real content, replace
the definition files with the shipped TSPS ones. The cache lives under `caches/<target>/`.

```sh
# First start: downloads the cache from OpenRS2, then serves on
# WEBSOCKET_PORT (default 43594).
go run ./cmd/server

# Boot without downloading the cache (revision is read from
# target.txt; definition-backed features degrade — run-energy
# weight is sent as 0 and XTEA keys are zero).
go run ./cmd/server --no-cache

# Options: --root DIR --port N --tick MS --save-backend json|memory|sqlite
go run ./cmd/server --root /srv/tsps --port 43594
```

On first start with a cache download, the server fetches `disk.zip`
and `keys.json` from the OpenRS2 archive into `caches/<target>/`,
under a download lock, with atomic rename — the same approach as
the TypeScript `CachePipeline`. The live download path was
smoke-tested end-to-end in the Phase 3 run (see
[PORT_STATUS.md](PORT_STATUS.md)); note the rev-242 OpenRS2 entry
ships zero XTEA keys, so rebuild packets carry all-zero keys.
With `BROWSER_HOST=1` a missing cache is a fatal error, as in TS.

Environment variables are read from the process environment only
(no `.env` file layering — the TS `run-with-env.mjs` layering was
unverified). See `internal/config` (`LoadEnv`): `WEBSOCKET_PORT`
(default 43594), `PUBLIC_HOST` (default `127.0.0.1`),
`PLAYER_SAVE_DATABASE_PATH` (SQLite save database, default
`data/saves/players.sqlite`), `LEGACY_PLAYER_SAVE_DIRECTORY`
(the JSON save directory actually used), `TSPS_ACCOUNTS_FILE`, `TSPS_DISPLAY_NAMES_FILE`,
`DISABLE_PLAYER_BOTS`, `DEV_USERNAME`, `BROWSER_HOST`.

Shutdown is graceful on SIGINT/SIGTERM/SIGHUP: the HTTP server
stops, the tick engine stops, online players are saved, and
account state is flushed.

## Configuration

World settings live in `data/definitions/world.json`, with optional
per-deployment overrides in `data/definitions/world.local.json`
(top-level keys replace; `pluginConfig` and `plugins` merge
key-by-key). Validation messages match the TypeScript
`WorldDefinition` parser. The cache target is read from `target.txt`
in the server root (format `osrs-{revision}_{YYYY-MM-DD}`).

## Architecture map

```
tsps-go/
  cmd/server/             startup wiring: flags, boot order, HTTP/WS
                          server, graceful shutdown, login<->world
                          adapter (pending names, online set, synchronous
                          world entry over the queued tick loop)
  internal/config/        world.json parsing/validation, zones,
                          environment variables, game constants
  internal/cache/         local cache reading only: FileStore sector
                          chains, container decrypt/decompress, XTEA,
                          reference tables, archives, OpenRS2 download,
                          definition decoders + CacheDefinitions facade,
                          CacheMaps, Gamevals. There is NO JS5 server in
                          TSPS and none here.
  internal/protocol/      wire protocol: Reader transforms, packet
                          framing, verbatim opcode/length tables,
                          BitWriter, welcome/tick/login/rebuild encoders,
                          player/NPC sync + appearance + state/widget
                          encoders, play-song + ground-item encoders,
                          typed login/handshake/movement decoders
  internal/session/       transport + login: WebSocket/headless channels,
                          session queue/flush, connection lifecycle,
                          login check order + literal response codes,
                          player bootstrap, per-tick flush (rebuild +
                          player/NPC sync + music + ground items +
                          NPC effects), turn-packet bridge,
                          name functions, timeouts
  internal/music/         music area grids (RLE) + track selection +
                          track lengths from cache DbTable 44
  internal/game/          tick engine (600 ms, drift-corrected,
                          no-overlap), World phase skeleton, MobileList,
                          entities, tasks, visibility/update buckets and
                          local lists; movement queue + route finder +
                          region manager (collision from cache maps);
                          ground-item manager (reveal/despawn/respawn/
                          merge); hooks for unported phases
  internal/definition/    JSON definition loaders (items, NPCs,
                          objects, shops, spawns, interactions,
                          layouts) + WorldConfigStore, layered over
                          the cache facade; read from data/definitions/
  internal/listeners/     packet-listener dispatch: decodes every
                          turn packet and acts only where the target
                          system exists, else drops with a named reason
  internal/persist/       scrypt passwords, SHA-256 login tokens,
                          accounts, display names, bans, PlayerSave JSON
                          shape, JSON-file + SQLite + in-memory save
                          stores, save apply/from-player merge
  internal/plugins/       plugin hub: compile-time content registry,
                          dependency-ordered loading, config-declaration
                          validation, world.json switches, members-gating,
                          lifecycle + custom-event hooks, gate contracts,
                          scoped core surface, login extensions
  internal/content/       ported plugin content, one package per piece
                          (currently only dateminutes: the date_minutes
                          varp sender); each package registers with the
                          hub and is pulled into the server by a blank
                          import in cmd/server/pluginwiring.go
  data/definitions/       minimal starter files (world.json + empty
                          valid definition files, plus the real
                          music-areas files imported verbatim from
                          TSPS; replace with the real TSPS files
                          for full content)
  target.txt              cache target (osrs-242_2026-11-03)
```

Cross-package rules the code follows: `game` never imports
`session`/`protocol`/`persist`/`plugins` (it defines the `SessionPort`
interface the session layer satisfies, and the hub calls into `game`,
never the reverse); `session` never imports `game` in non-test code; the coordinator (`cmd/server`) owns all
wiring and adapters. World queues and the player/NPC lists are
safe for concurrent use from network goroutines; everything else
in `game` is tick-goroutine-only unless documented otherwise.

## License

BSD 2-Clause, matching upstream. See [LICENSE](LICENSE) and
[NOTICE](NOTICE).
