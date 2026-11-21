# Phase 37 Plan — RSProx gateway endpoints (GatewayTarget)

## Decision
Tyler approved 2026-11-20 23:36 PST (widget tap): implement the gateway next, ahead of
the next skill pick (still unanswered) and the woodcutting-guild.json import (still pending).
This phase ports ONLY the two static gateway endpoints. It does not touch content, skills,
or any other HTTP surface.

## Scope
Port `GatewayTarget` (`server/src/main/typescript/elvarg/net/http/GatewayTarget.ts`, 130 lines)
into the Go server so the RSProx gateway process can discover this world:

- `GET /gateway/jav_config.ws` — `text/plain; charset=iso-8859-1`, latin1 body, the 30-line
  jav_config (see §Grounding).
- `GET /gateway/worldlist.ws` — `application/octet-stream`, one-world binary list.

Both are served on the game port (43594) by the same HTTP listener as the WebSocket,
exactly as TS does in `NetworkBuilder.initialize` (`NetworkBuilder.ts:347-434`).

## Grounding (parent-verified 2026-11-20)
- `GatewayTarget.ts` read in full. `JAV_CONFIG_PATH = "/gateway/jav_config.ws"`,
  `WORLD_LIST_PATH = "/gateway/worldlist.ws"`. `resolve(url, websocketPort)` splits the URL
  on `"?"` and matches exact paths only; anything else returns null.
- `javConfig(websocketPort)`: lines verbatim (parent has the file):
  `title=tsps`, `codebase=http://<host>/`, `cachedir=tsps`, `storebase=0`,
  `initial_jar=gamepack.jar`, `initial_class=client.class`, `viewerversion=124`,
  `win_sub_version=1`, `mac_sub_version=2`, `other_sub_version=2`,
  `window_preferredwidth=800`, `window_preferredheight=600`, `advert_height=96`,
  `applet_minwidth=765`, `applet_minheight=503`, `applet_maxwidth=5760`,
  `applet_maxheight=2160`, `param=2=` … `param=29=1`, trailing empty line (joined with
  `"\n"`, so the body ends with a newline). `param=12` is the world id (1).
  `param=17` is `http://<host>:<port>/gateway/worldlist.ws`. `param=25` is the active
  cache revision. Live 242 params 30/31 are NOT sent (TS comment).
- `host()`: `PUBLIC_HOST` env, trimmed; must match bare-host pattern
  `^[A-Za-z0-9.-]+$` (no scheme/port/path/spaces) or it is warned about once and ignored;
  default `127.0.0.1`.
- `revision()`: active cache revision when initialized, else `target.txt`'s revision.
- `worldList()`: `u32 length, u16 count=1`, then per world `u16 id, u32 flags, host\0,
  activity\0, u8 location, u16 population`. Flags = `(members ? 0x1 : 0) | type.flags`.
  Activity = world type's activity, else `"tsps"`. Location 0, population 0.
- `GameConstants.WORLD_TYPE` default (`GameConstants.ts:54`): `{ label: "REGULAR", flags: 0,
  activity: null }`. World type may be overridden from world.json
  (`GameConstants.ts:112-114`); verify what the Go worldconfig carries before adding any
  field — do NOT invent a type field. If Go has no world-type source, default flags 0 /
  activity `"tsps"` is the TS-faithful behaviour for this world.
- `isMembersWorld()` (`WorldDefinition.ts:307`): members flag from the world definition.
- `NetworkBuilder.ts:347-371`: the HTTP handler sets `Access-Control-Allow-Origin: *` on
  every response, tries `ContentApi.resolve` first, then `GatewayTarget.resolve`, then
  `/regions`. Gateway responses carry no other headers in TS.
- Go side (parent-verified): `internal/session/handler.go` — non-WS HTTP gets 404; its
  header comment lists GatewayTarget as NOT implemented (update it). `NewHandler` is wired
  at `cmd/server/server.go:568`; `httpServer.ListenAndServe` at `cmd/server/main.go:54`.
  `Server.Revision()` exists (`cmd/server/server.go:683`) and reads target.txt.
  `worldDef.MembersWorld` exists (`internal/definition/worldconfig.go:125,747,1019`) and is
  already consumed by cooking/fishing wiring.

## Requirements
1. New package (e.g. `internal/session/gateway.go` or a small `internal/gateway/` — follow
   the existing layout; the resolver is pure and takes host/port/revision/members/type as
   inputs, never globals).
2. `jav_config`: byte-exact vs TS for the default world (test against the 30 TS lines).
   Host resolution mirrors TS: env `PUBLIC_HOST`, bare-host validation, warn-once,
   `127.0.0.1` default. Revision = the server's active revision.
3. `worldlist`: byte-exact binary layout; decode-and-assert in tests (length, count, id,
   flags, host, activity, location, population). Members and non-members worlds both
   covered. World-type flags/activity from the Go world definition when present, else
   TS defaults (0 / `"tsps"`).
4. Routing in `handler.go`: exact-path match after stripping the query string; gateway
   paths served for plain HTTP GET (and any method? — verify against TS: the TS handler
   does not check method for gateway routes, only for ContentApi/browser-host. Mirror TS:
   no method check on gateway paths). All other non-WS HTTP still 404. WS upgrade
   behaviour unchanged. `Access-Control-Allow-Origin: *` on gateway responses, as TS sets
   it on every HTTP response.
5. The gateway must work with the server's configured port: jav_config's worldlist URL
   uses the actual listen port (TS passes `port` from `NetworkBuilder.initialize`).
6. Tests: byte-exact jav_config (default + PUBLIC_HOST + invalid-host cases), worldlist
   binary decode (members/non-members, default type), routing (query strings served,
   `/gateway/jav_config.ws/extra` 404s, unknown paths 404, WS upgrade unaffected).
7. Live verification (temp root outside the repo, real rev-242 cache, as in prior phases):
   start the Go server, `curl` both endpoints, compare byte-for-byte against the TS
   expectations (jav_config text; worldlist decoded fields). Do NOT run the external
   runelite-gateway process — it is out of scope (see below).
8. Docs: PORT_STATUS.md — gateway row to DONE; update the `handler.go` header comment
   (remove GatewayTarget from the NOT-implemented list). No other HTTP surface changes.

## Explicitly out of scope
- ContentApi (`/api/*`), `/regions` (+ `.pack`/`.terrain`), browser-host bridge
  (`BROWSER_HOST=1`), WebRTC registration — all stay NOT implemented.
- The runelite-gateway process itself (external repo/process): it reads jav_config +
  worldlist from the game server and serves the cache to the official client over JS5.
  This phase makes the Go server serve what that process needs; running RSProx end to
  end is a separate step, not this phase.
- Any change to the game protocol, login, or content.

## Environment notes
- Go 1.25.1 at `~/workspace/tools/go/bin/go`. No new dependencies.
- The background instrumented browser test (`/home/hatch/live36/pw-test3.cjs`, session
  `proc_db44c8a98d0f`) is still running against the live36 servers — do NOT touch
  `/home/hatch/live36`, the live36 servers, or `~/tsps-full`'s running client. Use a fresh
  temp root for this phase's live check, as prior phases did.
- Repo tree is clean at commit `93250d7` (Phase 36 state).

## Verification (coordinator, then parent re-verifies)
- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build ./...`
- Full suite `go test ./... -p 1 -count=1 -v` with the exact pass count; `go test -race ./...`
- Live curl check per §7, byte-for-byte.
- No stray artifacts, no `*.plugin.js`, no probe references left in code.
