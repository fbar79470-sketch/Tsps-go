# Phase 21 plan (sailing + template regions + sounds/projectiles — migration plan milestone 11)

Status: COMPLETED (coordinator-verified, 2026-11-16). Tyler
approved starting milestone 11 at 2026-11-16T02:14:30Z (UTC) in
reply to the Phase 20 summary. This plan REPLACES the proactivity
draft: every grounding claim below was re-verified by the
coordinator by grep/read against the read-only TS checkout at
`~/tsps-bfd81e3` and the Go repo at Phase 20 state (516 `.go`
files, 13 packages, 3,406 tests, hub 44/108 firable, 19/95
members, 4/191 Core). Line numbers are TS `bfd81e3` unless stated.

## Scope (migration plan §2 item 11, verbatim)

"Sailing/boats + template regions + sounds/projectiles. Unlocks:
`skills/sailing`, Construction player-owned houses, instanced
content, and the remaining encoders."

This is the LAST engine milestone. Sailing skill content (20,617
lines) and Construction (12,293 lines) are projects of their own:
this phase builds the ENGINE systems they need, not the skill or
the houses. After this milestone the engine-systems plan is
exhausted; what remains is content ports, named honestly in
PORT_STATUS.md.

## Grounding: what TS provides (all paths under `server/src/main/typescript/elvarg/`)

### A. Remaining encoders — all NOT STARTED in Go, all LIVE upstream

Go has the opcodes but no encoders (`internal/protocol/opcodes.go`:
`ServerSound = 131`, `ServerPlayJingle = 132`,
`ServerProjectiles = 84`, `ServerCameraControl = 137`,
`ServerRebuildWorldEntity = 142`, `ServerWorldEntityInfo = 143`).
Verified by grep: no `EncodeSound` / `EncodePlayJingle` /
`EncodeProjectiles` / `EncodeCamera*` / `EncodeRebuildWorldEntity`
/ `EncodeWorldEntityInfo` / `EncodeTileSpotAnim` exists in
`internal/protocol`.

Liveness rule (Phases 19/20 precedent): `PlayerSession.write()`
is an empty no-op (`net/PlayerSession.ts:228-230`), but
`sendClientPacket` (`:287`) is the LIVE queued flush path. Every
encoder below is reached via `sendClientPacket` from
`PacketSender`, so every one is live — no no-op mirrors in this
phase. Worker A must still verify each sender's path before
framing anything.

1. **Sound** — `encodeSound` (`net/protocol/ClientProtocol.ts:2753-2782`,
   opcode 131): soundId u16, positioned flag, optional x/u16 +
   y/u16 + level/u8, loops (min 1), delay u16, radius +
   attenuation clamped 0..31. Senders: `sendSoundEffect`
   (`PacketSender.ts:252-261`, unpositioned),
   `sendSound` (`:264-267`, wrapper — note its stale 174/175
   comment; the encoder's real opcode governs),
   `sendAreaSound` (`:293-306`, positioned).
2. **Jingle** — `encodePlayJingle` (`ClientProtocol.ts:2785-2790`,
   opcode 132, 5 bytes): jingleId u16 + delay as 3-byte IME
   (bytes 2..4 are `value>>16`, `value`, `value>>8` — note the
   shuffled order; copy it exactly, do not "fix" it). Sender:
   `sendJingle` (`PacketSender.ts:285-291`): sends the current
   track with zero fades first, then the jingle. Needs the
   current-track seam: Go `internal/session/music.go` records
   `getCurrentMusicTrack`/`noteSongSent` as NOT STARTED — Worker
   A reads the music state machine and exposes the getter if the
   state already holds the track; if not, the track-first step
   is a documented gap, not an invention.
3. **Projectiles** — `encodeProjectiles`
   (`ClientProtocol.ts:3749-3771`, opcode 84, −2): u16 count,
   then 29 bytes per projectile. `ProjectileView` type at
   `:233` — read it, do not re-derive the field order. Model:
   `game/model/Projectile.ts` (254 lines): `createProjectile`
   + `sendProjectile` broadcast to viewers. Sender:
   `PacketSender.sendProjectile` (`:1236-1289`, incl. the
   end-cycle fallback and `projectileActor` lock-on mapping).
   Live producers: `MagicCombatMethod`, `RangedCombatMethod`,
   `CombatSpells` — Go combat has no projectile producers
   (Phase 16 gap); port model + senders, wire a combat
   producer ONLY if a clean Go call site exists, else name the
   seam as a gap.
4. **Camera** — 11 encoders (`ClientProtocol.ts:1602-1816`,
   opcode 137, leading discriminator byte; `CAMERA_EASINGS` at
   `:1613-1614`): shake, moveTo, lookAt, rotateToCoordinate,
   rotateBy, moveArc, unlock, skybox, translate, target, reset.
   Senders in `PacketSender.ts:1111+`. Port ALL 11 (decision D1).
5. **World entities (boats)** — `encodeRebuildWorldEntity`
   (`ClientProtocol.ts:1942-1985`) and `encodeWorldEntityInfo`
   (`:1986-1997`; `WorldEntityPosition` / `WorldEntityUpdate` /
   `WorldEntitySpawn` at `:1870-1895`). Driven by
   `WorldEntitySync.flush`, called from `flushClient`
   (`PlayerSession.ts:616`) — live. Go's
   `internal/session/flush.go:429` records the omission; this
   phase lands it (Worker C; encoders owned by Worker A).
6. **Graphics** — `encodeTileSpotAnim`
   (`ClientProtocol.ts:1586`) + `encodeNpcSpotAnimationSpecific`
   (`:156`); senders `sendGraphic` / `sendGraphicToArea`
   (`PacketSender.ts:1217-1234`) and
   `sendNpcSpotAnimationSpecific` (`:215-218`), all via
   `sendClientPacket` — live. (Phase 8 NPC-effect queue
   plumbing exists; Worker A checks how it relates and wires
   or documents.) The Phase 13 loc anim/merge encoders exist
   with no producers — producers stay a gap unless a core
   caller exists.

### B. Sounds — the `Sound` / `Sounds` system

- `game/Sound.ts` (280 lines): `Sound` class, constructor
  `(id, volume, delay, loopType)` — note the argument ORDER;
  153 named statics (`new Sound(2605, 1, 0, 0)` = id, volume 1,
  delay 0, loopType 0). Getters at `:262-278`. Port the table
  exactly; do not reorder or "fix" values.
- `game/Sounds.ts` (162 lines): `sendSound(entity, sound)`
  resolves a player (Mobile-like or duck-typed) or a location
  for area sounds; `sendSoundEffect` direct; `playAreaSoundAt`
  / `playAreaSound` broadcast (radius clamped 0..31,
  broadcast radius `max(15, radius+1)`, same-plane check,
  private-area matching via `getPrivateArea`, bot guard,
  `exclude` support); `resolveKnownSound` /
  `knownSoundNames` registries.
- Go already has: `internal/definition/sounds.go`
  (equipment/NPC-combat-sound loaders + pickers — data, not
  the send path) and `internal/game/audio.go` (audio SETTINGS
  varps only). Neither sends a sound frame. The send path is
  this phase's work; wire the pickers to real sends ONLY where
  a Go call site exists, and list the missing call sites
  (combat/pickup/equip/drop sounds) as gaps.

### C. Template regions / instancing — NOT STARTED in Go

- `game/model/areas/impl/PrivateArea.ts` (156 lines, abstract,
  extends `Area`): entity list, clip map, destroy-on-empty,
  add/remove/detach, `postEnter`/`postLeave`, emits
  `private-area:entered` custom event for players.
- `game/model/areas/impl/TemplatedInstanceArea.ts` (409 lines,
  extends `PrivateArea`): `TemplateChunk`, `MAX_INSTANCE_CHUNKS
  = 24`, `packTemplateChunk` / `rotateChunkTile` /
  `rotateChunkLoc` helpers, chunk grid, clipGrid, locIndex,
  sceneVersion, `copySquare`, `buildScenePalette`,
  `encodeScene`, `hasClip`/`getClip`/`setClip`/`removeClip`,
  `getTemplateObject(s)`, `resolveObject`, destroy/invalidate.
- `game/collision/TemplateRegionData.ts` (93 lines):
  `TemplateRegion` (settings + chunked locs),
  `getTemplateRegion` / `readTemplateRegion` (decode-once
  cache over `CacheMaps.getRegion`). Go must read the same
  data through the existing cache API — no new cache copies.
- `game/collision/MapRegionReplacementManager.ts` (445 lines):
  region replacement packets, plugin-exposed via Core
  `replaceMapRegion` / `replaceMapRegionData`
  (`plugins/PluginManager.ts:4510-4543`), consumed by
  `NetworkBuilder` / `PlayerSession`. Worker B grounds the
  packet's encoder + liveness (sendClientPacket vs write)
  before porting; the HTTP/ContentApi side stays out.
- Go already has: `EncodeRebuildRegion` (opcode 140,
  `templateChunks [4][13][13]int32`, xtea keys —
  `internal/protocol/encode.go:80`, from
  `ClientProtocol.ts:1830-1863`); `Area`/`AreaManager`
  (`internal/game/area.go`, `areamanager.go`, Phase 15).
  KEY DESIGN QUESTION for Worker B (ground in TS first):
  how does clipping resolve for private areas — where does
  TS consult area clips (route finder? movement? RegionManager
  itself?), and how does area evaluation interplay with
  private-area membership (does the public AreaManager skip
  private-area members, or does `setArea` win)? Port the
  consultation at the SAME call sites TS uses. Do not invent
  a parallel clip path.
- `Music.instanceSourceTile` (templated-instance tile mapping
  for music areas, noted NOT STARTED in
  `internal/session/music.go`) — Worker B assesses whether it
  falls out of the templated-area work; port if small and
  grounded, else gap.

### D. Sailing engine — NOT STARTED in Go

`game/content/sailing/` (1,773 lines total):

| File | Lines | Role |
| --- | --- | --- |
| `Sailing.ts` | 519 | `initialize()` (`:76`, idempotent guard), types/docks registries, `giveBoat`, boarding/disembark/guest rules (`LeaveReason` / `GuestLeaveReason` / `GuestRefusal` at `:14-40`), `recordPositions`, teleport/death handling, `LOST_AT_SEA` text, `sailing:boarded` / `sailing:left` / `sailing:guest-boarded` / `sailing:guest-left` custom events (`:46-52`) |
| `BoatManager.ts` | 334 | `spawn` / `spawnStatic`, `dispose`, deck-loc CRUD + `deckLocChanges`, `getBoat` / `getSpec` / `getDeck` / `all`, `getBoatAboard`, `rootLocation` (`:179`), `deckTileToward` (`:192`), `syncArea` (`:223`), `canSeeBoat` / `canSeeAboard` (`:229-238`), `onHeadingSet`, `steerToward` / `steers` / `setHelmHeading`, `isSailable`, `teleport`, `onAfterTick`, `tick` (`:305`) |
| `BoatMovement.ts` | 118 | movement/heading |
| `SailingState.ts` | 195 | per-player boat state; PERSISTS via `PlayerSave.sailing` (`PlayerSave.ts:185,680,810`, `normalizeSailingState`) — owned save state, port the normalize rules |
| `Boat.ts` | 215 | boat entity: `entityIndex`, `fineX`/`fineY`, `angle`, `heading`, helm/crew, scene/deck bases, `BoatMoveMode` |
| `WorldEntitySync.ts` | 185 | per-session boat sync: `BOAT_VIEW_DISTANCE = 32`, max 8 boats / 4 new per tick, viewer state (order/boats/positions/locChanges/teleports), `flush` before player sync |
| `BoatDeckArea.ts` | 91 | deck area (extends `PrivateArea`): 104×104 scene, walkable/solid sets, `setLoc` |
| `BoatCollision.ts` | 64 | deck collision |
| `BoatSpec.ts` | 60 | specs incl. `BoatDeckLoc` |
| `HeadingUtils.ts` | 52 | headings |

Go seams already waiting (wire into them, do not re-invent):

- `internal/game/world.go:54-56,521`: `Boats func(cycle int64)`
  hook runs `BoatManager.tick + region refresh under boats`.
- `internal/game/visibility.go`: boat degeneracies documented
  throughout — `rootLocation`, `syncArea` nil, `canSeeAboard`
  true. Each degeneracy site must be revisited and go live.
- `internal/protocol/sync.go:1316-1431`: `EncodeNpcSync`
  already takes `deckLocal` + `deckBase` + `crossFloor`.
- `internal/protocol/loc.go:53,70`: loc add/change passes
  opFlags per `WorldEntitySync.ts:183`; sailing NOT wired.
- `internal/game/movement_queue.go`: TS `MovementQueue.ts`
  boat branches at `:1504-1773` (`deckTileToward`, main-world
  NPC from deck, walk-to-reset sites at `:421,1289,1519,1635,
  1773`) — the 6th `walkToReset` site from Phase 20 lives here.
- `internal/session/flush.go:429`: `WorldEntitySync.flush`
  omission site.
- Hub: `onWorldEntityInteraction` /
  `emitWorldEntityInteraction` are TABLE-ONLY today
  (`internal/plugins/hooks.go` unfired table). This phase
  makes the family firable with a production emit site.
- Listeners: `NativeOpWorldEntity1..6` (220–225) decoded;
  the interaction path is missing. Follow the Deps-adapter
  emit pattern (`cmd/server/*wiring.go` installs emit funcs
  into listener Deps; listeners never import `internal/plugins`).

### E. Hub exposure

`plugins/PluginTypes.ts` types the surfaces: `Projectile`
(`:1611`), `Sound` / `Sounds` (`:1639-1640`), `Sailing` /
`BoatMoveMode` (`:1681-1682`) — all `any` Core members in TS;
port as TYPED Go Core members where Go types exist, and say
which stay table-only and why. The ONLY `on*` family in this
milestone is `onWorldEntityInteraction`
(`PluginTypes.ts:1066`, `PluginManager.ts:1265,3477,4696`).
Sailing and private-area signalling is via CUSTOM events
(`sailing:*`, `private-area:entered`) on the existing
custom-event surface; `Player.EmitCustomEvent` is the
game→hub seam (cf. `internal/game/skill_hooks.go`,
`cmd/server/dialoguewiring.go:144`) — game code must never
import `internal/plugins`. Count firable families/members/Core
before/after, as every phase has.

## Coordinator decisions (D1–D4)

- **D1 — Camera:** port ALL 11 variants. One small family, one
  opcode, all with PacketSender senders; a partial port would
  leave arbitrary gaps.
- **D2 — `NpcAnimationScanner`:** NOT this milestone. It lives
  in combat/animation (`GameBuilder`, `Combat`, `Mobile`,
  `Player`), unrelated to sailing/templates/sounds. It stays a
  documented gap.
- **D3 — Proof:** BOTH, cheaply, via a temporary probe owned
  by the coordinator: (a) sound + jingle + projectile frames
  on the wire; (b) a templated instance entered + a boat
  spawned/moved with world-entity frames. No skill or POH
  content. Probe removed afterward; zero probe references remain.
- **D4 — Sailing skill XP:** engine seam only (state +
  boarding + custom events). No stub skill action, no XP
  grants — Phase 20's framework is the seam content will use.

## Frozen seam contracts (workers: implement/consume EXACTLY these)

1. **PrivateArea API** (Worker B implements in
   `internal/game/private_area.go`; Worker C consumes for
   `BoatDeckArea`):
   `type PrivateArea struct { *Area; ... }`,
   `NewPrivateArea(boundaries []Boundary) *PrivateArea`,
   `Add` / `Remove` / `Detach` (taking `AreaActor`),
   `Destroy()` / `Destroyed() bool`, `Entities() []AreaActor`,
   `HasClip` / `GetClip` / `SetClip` / `RemoveClip` (taking
   `Location`; masks `int32`), `UsesTemplateMap() bool`.
   `PrivateArea.Add` MUST set the actor's `Area` to the
   embedded `*Area` (TS `entity.setArea(this)`), so
   `Mobile.Area()` pointer comparison implements the
   `Sounds.playAreaSound` area-matching rule (Worker A relies
   on this; see contract 2). `Mobile.PrivateArea() *PrivateArea`
   (nil when not in one) is owned by Worker B.
   `BoatDeckArea` embeds `*PrivateArea` and uses ONLY the
   methods above plus `Area` boundary/Enter/Leave methods and
   its own walkable/solid/deck-loc state. Template-object
   lookup (`GetTemplateObjects`/`resolveObject`) is Worker B's
   surface; Worker C must NOT call it directly — deck locs
   sync through `WorldEntitySync` from `BoatSpec`/deck-loc
   state, and any deck-object interaction seam is reported to
   the coordinator for reconciliation.
2. **Sounds area rule** (Worker A): `playAreaSound` matches
   source/player areas by comparing `Mobile.Area()` pointers
   (nil == nil plays; different private areas skip). This is
   behaviorally equivalent to TS `getPrivateArea()` comparison
   GIVEN contract 1's `setArea` rule; the comment at site must
   say so. Worker B MUST verify area evaluation does not clear
   a private-area member's `Area()` (ground in TS: does the
   public AreaManager skip private-area members?).
3. **Custom events:** `private-area:entered` and `sailing:*`
   fire through `Player.EmitCustomEvent` (installed by wiring;
   nil-safe when uninstalled, matching existing seams).
4. **Hub registry:** new `hookRegistry` add/snapshot methods
   live in each family's OWN plugins file (Phase 20 `skills.go`
   precedent). ONLY Worker C edits `internal/plugins/hooks.go`,
   and ONLY to add the `worldEntityInteraction` struct field(s).
   The unfired-family table comment and all counts are
   coordinator-owned.
5. **Wiring:** each worker creates `cmd/server/<area>wiring.go`
   (+ `_test.go`) exposing an `Install*` function, and reports
   the exact install call line. The COORDINATOR adds install
   calls to `server.go`/`main.go`. Workers must NOT edit
   `cmd/server/server.go`, `cmd/server/main.go`,
   `cmd/server/pluginwiring.go`, `PORT_STATUS.md`,
   `CONTENT_MIGRATION_PLAN.md`, this file, or `README.md`.
6. **Shared-file ownership** (re-read the file immediately
   before each edit; report every shared-file edit):
   - `internal/session/flush.go`: Worker B ONLY the rebuild
     path (`sendRebuildNormal` + call site); Worker C ONLY the
     `WorldEntitySync` omission site (~:429). Worker A: none.
   - `internal/game/regions.go` (+ collision): Worker B only.
   - `internal/game/movement_queue.go`, `visibility.go`:
     Worker C only.
   - `internal/session/music.go`: Worker A only (getter seam).
   - `internal/listeners/listeners.go` dispatch: Worker C only
     (world-entity cases; handlers in new `worldentity.go`).
   - `internal/plugins/hooks.go`: Worker C only (struct fields).
   - Everything else: new files only, unless the worker's
     brief names the file.

## Work split

1. **Worker A — encoders + sounds + projectiles + graphics.**
   Protocol encoders (§A.1–A.6 except world-entity rebuild/info
   frame assembly, which is C's flush — A owns the two
   world-entity ENCODERS), `Sound` table + `Sounds` send path,
   session senders, `Projectile` model + broadcast, camera
   senders, graphic senders, current-track seam, hub Core
   members (`Sound`/`Sounds`/`Projectile`, + camera surface if
   `PluginTypes` has one — report if not), picker wiring only
   where call sites exist.
2. **Worker B — template regions + instancing.** `PrivateArea`,
   `TemplatedInstanceArea`, `TemplateRegionData`,
   `MapRegionReplacementManager` (grounded liveness),
   rebuild-with-template integration, clip consultation at TS
   call sites, AreaManager interplay, `private-area:entered`,
   `instanceSourceTile` assessment, hub Core/query members
   grounded in `PluginTypes`.
3. **Worker C — sailing engine.** `SailingState` + normalize +
   owned save state, `Sailing`, `BoatManager`, `Boat`,
   `BoatMovement`, `BoatSpec`, `HeadingUtils`, `BoatDeckArea`,
   `BoatCollision`, `WorldEntitySync` flush + `FlushTick`
   wiring, `Boats` hook wiring, visibility degeneracies live,
   MovementQueue boat-deck paths, world-entity interaction
   listener + `onWorldEntityInteraction` firable (+ `emit*`
   Api method per the objects.go precedent), `sailing:*`
   custom events, hub Core members (`Sailing`/`BoatMoveMode`).

## Out of scope (do not expand without grounding)

- Sailing skill content, Construction / POH content, quests,
  minigames, bosses, modes, achievements, individual skill ports.
- `NpcAnimationScanner` (D2). HTTP extras (`ContentApi`,
  `/regions`), WebRTC. `registerCustomInterface` (unchanged).
- The far-WALK bug and the y=3521 map wall — untouched unless
  this phase's grounding proves a shared cause; do not fix on
  speculation.
- Real production scrypt-hash check — still open, needs a
  production hash from Tyler. Perf work (`MobileList.All`).

## Verification (same bar as phases 8–20)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean,
  `CGO_ENABLED=0 go build ./...` clean, full test suite green
  (`-p 1`, serialized; do NOT touch `TestEngineTicksAtRate`
  tolerance), race detector clean.
- Live smoke, 3 runs stable (coordinator-owned probe, temp
  root outside the repo, deadline-free readers, handshake
  after login, cleaned up): (a) sound frame (131) + jingle
  frame (132) with TS-exact payloads; (b) projectile frame
  (84) with a 29-byte entry; (c) boat moving with a player
  aboard — world-entity info frames + deck-relative sync;
  (d) templated instance entered via a rebuild with template
  chunks. Record which of (a)–(d) could not be driven live
  and why, instead of claiming them.
- Docs (coordinator): `PORT_STATUS.md`, migration-plan
  milestone 11 DONE + boundary, this file's Outcome section.

## Outcome

COMPLETED 2026-11-16 (coordinator-measured). Three worker
streams (A: encoders/sounds/projectiles/graphics; B: template
regions/instancing; C: sailing engine), integrated and verified
by the coordinator.

Verification: `go build ./...`, `go vet ./...`, `gofmt -l .`
clean; `CGO_ENABLED=0 go build ./...` clean; 588 `.go` files,
13 packages; `go test ./... -p 1 -count=1`: 3,696 passing
incl. subtests, 0 failures (3,406 + 290 new); `go test -race`
clean (0 data races, all packages). Zero probe references
remain; zero JS files; `game` never imports `plugins`.

Hub: 45/108 hook families firable (+`onWorldEntityInteraction`),
24/95 non-hook members (+`getAreaManager`,
`replaceMapRegion`, `replaceMapRegionData`, `restoreMapRegion`,
`emitWorldEntityInteraction`), 12/191 Core (+`Sound`, `Sounds`,
`Projectile`, `MapRegionReplacementManager`,
`ReadTemplateRegion`, `GetTemplateRegion`, `Sailing`,
`BoatMoveMode`).

Live smoke (coordinator-owned temporary probe, removed
afterward; 3 runs stable): (a) sound frames — opcode 131,
8-byte unpositioned + 13-byte positioned, id 3220 — and a
jingle frame — opcode 132, 5 bytes, id 1234 — observed;
(b) projectile frame — opcode 84, count 1, one 29-byte
entry, id 100 — observed; (c) boat boarded with
world-entity scene (opcode 142, entity 3000, config 7,
8x16) + info spawn (opcode 143) + deck-scene rebuild
(opcode 141, zone 1200,1201), and a test-driven engine
teleport produced a type-3 snap update — observed; wind-
driven sailing motion was NOT observed live (no cache, no
water, `TickBoat` cannot move a hull — motion math is
unit-covered against a stub water source; helm steering
unit-covered only); (d) templated instance entered —
opcode 140, forceReload=1, probe chunk in the bit palette
— observed.

Coordinator corrections during integration: steering placed
inside `OnMovementRequest` (TS order: preamble → `walk()` →
steer first), not before the preamble; the `logger.Printf`
install argument wrapped to the `func(string)` shape;
hub counts reconciled from code (the pair form of
`replaceMapRegion` is one member, not two); the first race
run caught 2 data races from a process-wide boat-manager
global read on every tick — fixed by scoping the manager
and the Sailing runtime to the owning world/hub and
deleting both globals (the race detector is clean after
the fix). The pre-existing `combat_ranged.go` projectile-
flight divergence was left untouched, pending grounding.

Residual gaps (documented in PORT_STATUS.md): `viewedArea`
object sync unwired; `sceneTile` root-location term still
degenerate (music tile wired); deck-object interaction
lookup; camera state/restore; sound picker call sites;
combat projectile producers; destroy-hook splicing;
`MovePlayerTo` following/interaction clearing;
`NpcAnimationScanner` NOT STARTED; sailing skill and
Construction POH content NOT STARTED (plugin content).
Far-WALK bug + y=3521–3522 wall untouched; scrypt check
still open. The engine-systems plan is now exhausted:
all 11 milestones DONE. Next steps need Tyler's direction.
