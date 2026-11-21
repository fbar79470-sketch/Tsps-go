# Phase 40 Plan — Wilderness Ditch crossing

## Decision
Tyler asked 2026-11-21 02:18 PST: "Add wilderness ditch". This phase ports
ONLY `server/plugins/objects/WildernessDitch.plugin.js` (62 lines) into the
Go server: the "Cross" interaction on the Wilderness Ditch object that jumps
the player 3 tiles across, in and out of the Wilderness. The work is mostly
the ForceMovement execution path the ditch needs — no ForceMovement
field/system exists in Go today, only the sync mask bit and the wire encoder.

## Scope
Port `WildernessDitch.plugin.js` in full:

- `resolveJump(player, ditch, source, face)`: ditch is 1x2. Face 0/2 runs
  east-west (the long stretch at y 3521, crossed north/south): fromY <
  ditch.y+1 (default threshold 3522) → {dx: 0, dy: 3, direction: 0} (north),
  else {dx: 0, dy: -3, direction: 2} (south). Face 1/3 runs north-south
  (where it turns north at 2996,3530-3533, crossed east/west): fromX <
  ditch.x+1 → {dx: 3, dy: 0, direction: 1} (east), else {dx: -3, dy: 0,
  direction: 3} (west). Lands 3 tiles on, away from the side stood on.
- `tryCrossWildernessDitch`: no-op if a ForceMovement is already active on
  the player (`force_movement_active`); no-op unless the player's click
  delay has elapsed 250 ms (`click_delay`); otherwise submit
  `ForceMovementTask(player, 3, forceMovement)` (start = player location,
  delta (dx, dy), speed 0, reverseSpeed 70, direction, animation 6132),
  send the jump sound (synth 2462, delay 25), reset the click delay.
- Registration: `api.onObjectInteraction("Wilderness Ditch", { Cross:
  crossDitch })` — hook by object NAME, not id. Object id 23271 (see
  transports.json "Cross Wilderness Ditch" entries).
- Home: new package `internal/content/wildernessditch/`, porting the
  `WildernessDitch` plugin (name + description from the plugin file),
  registered via the established content pattern: package `init()` calls
  `plugins.Register(Plugin)`; one blank import line in
  `cmd/server/pluginwiring.go` (alphabetical position); `hub.Load()` runs
  its `Register` during plugin Load.

## Grounding (parent-verified 2026-11-21)
- Upstream (`WildernessDitch.plugin.js`, read in full): constants
  `JUMP_ANIMATION = 6132`, `JUMP_SOUND_DELAY = 25`; `resolveJump`,
  `tryCrossWildernessDitch`, `crossDitch` as in Scope. No Wilderness
  entry warning/dialogue exists anywhere in the ditch plugin or in
  `plugins/areas/Wilderness.plugin.js` (grep for warning/dialogue: no
  hits) — nothing to port on that front.
- TS `ForceMovement` (`ForceMovement.ts`): fields start, end, speed,
  reverseSpeed, direction, animation. The ditch passes the DELTA as
  `end` (`new Location(jump.dx, jump.dy)`); the task computes the landing
  tile as start + end.
- TS `ForceMovementTask` (`ForceMovementTask.ts`): constructor (player,
  delay 3, forceM) does `player.getCombat().reset()`,
  `player.getMovementQueue().reset()`, `player.setForceMovement(forceM)`.
  `execute()` (after 3 ticks): if `player.getForceMovement() !==
  this.forceMovement`, stop; else `player.moveTo(start + end, same z)`
  ("Use moveTo() so completion emits placement/reset semantics... Direct
  setLocation() caused client/server position drift after ditch
  crosses"), `setForceMovement(null)`, stop. `stop()` clears the force
  movement only if it is the owned instance.
- TS `Mobile.moveTo` (`Mobile.ts:194`): notifyTeleport, movementQueue
  reset, setLocation(clone), setNeedsPlacement(true),
  setResetMovementQueue(true), clear interaction, handleRegionChange for
  players. Go precedent for instant repositioning: `Mobile.SetLocation`
  (`internal/game/entity.go:205`) + `NeedsPlacement = true` (sailing.go
  :966); confirm the Go death-respawn path for what else a reposition
  needs (movement-queue reset lives in `MovementQueue.Reset`, used by
  `internal/listeners/objectaction.go`).
- TS sync view (`PlayerSession.ts:742-786`): `forceMovement =
  player.getForceMovement()`, dirty = `Flag.FORCED_MOVEMENT` flagged.
  If dirty and forceMovement and no animation already in updates and
  `animation > 0`: `updates.animation = {id: animation, delay: 0}` (the
  jump animation rides the normal animation block). `forcedMovement`
  view, only when dirty AND present: startDeltaX/Y = start − location;
  endDeltaX/Y = start + end − location; startCycleOffset = speed (0);
  endCycleOffset = reverseSpeed (70); direction =
  `[1024, 1536, 0, 512][direction] ?? direction`. `forcedMovementEnd`,
  whenever forceMovement is present (even when not dirty): {x: start.x +
  end.x, y: start.y + end.y, level: z}.
- TS `TickStopwatch` (`TickStopwatch.ts`): measured in game ticks;
  `elapsed() = (currentCycle − tick) * 600`; a never-started stopwatch
  reports Infinity (every delay elapsed). `elapsedTime(250)` therefore
  passes unless the stopwatch was reset on the CURRENT cycle. Click-delay
  resets in TS are rare (`Gambiling.ts:40`, `EffectSpells.ts:71`,
  `TeleportHandler.ts:89,159`, `MagicOnItemPacketListener.ts:195`) — NOT
  on object clicks. So in practice the ditch gate blocks only a re-Cross
  in the same tick as a previous Cross/teleport/gamble; the
  force-movement-active guard blocks the double-click case first
  anyway. Go has no click-delay state: add it as Player methods
  (`ClickDelayElapsed(ms)`, `ResetClickDelay()`) reading the world
  cycle through the existing `combatCycle()` source
  (`internal/game/combatant.go:494`, wired to `World.ProcessCycle` at
  boot) — do NOT add a second cycle source.
- TS movement gating (`MovementQueue.ts:350,408`): `getMobilityIgnoringFreeze`
  returns INVALID when `player.getForceMovement() != null` (INVALID
  sends nothing — the click is silently refused, per the Go
  `movement_queue.go` comment); `requestWalk` has a forced-walk chain
  exception via `area.handleForcedWalk`, which Go has no form of — do
  NOT port it. Go `getMobilityIgnoringFreeze`
  (`internal/game/movement_queue.go:509`) documents the force-movement
  INVALID gate as a gap (lines 575-576): close exactly that leg here,
  reading the new Player force-movement field through the existing
  `IsPlayer()` interface-assertion shape.
- TS `Player.busy()` (`Player.ts:833-846`): forceMovement != null → busy.
  Go has no `Busy()` — do not invent one; the movement gate above is
  the ditch's only needed consumer.
- Go side (verified in-repo):
  - Sound 2462 exists: `SoundWildernessDitchJump`
    (`internal/game/sounds.go:226,392`). Send path:
    `game.SendSound(player, game.NewSound(2462, 1, 25, 0))`
    (`sounds.go:604`, TS `new Sound(id, 1, JUMP_SOUND_DELAY, 0)`).
  - Registration seam: `Api.OnObjectInteractionNamed`
    (`internal/plugins/objects.go:486`); the hub event carries
    `Object` (with `Face() int` — `GameObject` interface,
    `objects.go:57-67`), `Location` (object tile), `SourceLocation`
    (player tile at arrival), `Option`, `ClickType`
    (`ObjectInteractionEvent`, `objects.go:145-156`); the listener
    builds it in `internal/listeners/objectaction.go:380` and the
    adapter in `cmd/server/objectlistenerwiring.go:168`.
  - Sync wire: `protocol.ForcedMovementView` + `writeForcedMovement`
    (`internal/protocol/sync.go:77-90, 761-772`) and the mask bit
    (`playerMaskForceMovement = 0x400`) already exist; NOTHING populates
    them. Add `ForcedMovement *protocol.ForcedMovementView` +
    `ForcedMovementEnd *protocol.Tile` to `session.SyncPlayerSnapshot`
    (`internal/session/flush.go:42`), populate in
    `cmd/server/syncsource.go playerSnapshot` (with the
    PlayerSession.ts view math + the animation fallback above), map in
    `flush.go createPlayerView` (~line 799). `animationView`
    (`syncsource.go:261`) is FlagAnimation-gated; the forced-movement
    animation fallback applies when it returns nil.
  - Flags: `game.FlagForcedMovement` exists (`internal/game/flags.go`);
    `Player.setForceMovement` must flag it when non-nil, mirroring
    `Player.setForceMovement` (`Player.ts:1207-1211`).
  - Tasks: `game.NewTask(delay, action)` + `TaskManager.Schedule`
    (`internal/game/task.go`); delay 3 executes on the 3rd tick after
    scheduling, matching TS `Task` countdown semantics. `Combat.Reset`
    exists (`internal/game/combat.go:885`). Content reaches the
    TaskManager lazily via `api.GetTaskManager()` — resolve lazily,
    never capture in Register (the Phase 32 hub-contract lesson).
  - `WildernessDitch` has no members gate in its plugin definition
    (name + description only), so it loads on every world.

## Requirements
1. `internal/game/forcemovement.go` (new): `ForceMovement` type
   (Start + Delta `Location`s, Speed, ReverseSpeed, Direction int
   0=N/1=E/2=S/3=W, Animation int), `Player.ForceMovement()` /
   `Player.SetForceMovement(fm)` (flags `FlagForcedMovement` when
   non-nil, mirroring TS), `ForceMovementTask` (constructor side
   effects: combat reset + movement-queue reset + set; `Execute`
   identity check + moveTo-equivalent + clear + stop; `Stop` clears
   only the owned instance), and the click-delay stopwatch on Player
   (`ClickDelayElapsed(ms int) bool`, `ResetClickDelay()`) using
   `combatCycle()` (600 ms/tick; never-reset reads as elapsed).
2. Movement gate: `getMobilityIgnoringFreeze` returns INVALID while a
   player's force movement is active (TS order: after STUN, with the
   other INVALID sources); update the gap comment at
   `movement_queue.go:562-576` (needsPlacement stays a gap).
3. Sync: snapshot fields + `playerSnapshot` population + `createPlayerView`
   mapping, exactly per the PlayerSession.ts math above (direction table
   `[1024, 1536, 0, 512]` verbatim; animation fallback only when no
   animation view exists and `Animation > 0`).
4. `internal/content/wildernessditch/wildernessditch.go`: package doc
   (grounding + any deviation), `Plugin` value (name
   `"WildernessDitch"`, upstream description), `init()` registering it,
   handler calling `api.OnObjectInteractionNamed("Wilderness Ditch",
   {"Cross": ...})`. Handler, in exact TS order: resolve jump from
   object face + object location + source location (default thresholds
   3522 / ditch.x+1 when a location is absent, as in TS); guard active
   ForceMovement; guard click delay 250 ms; build the ForceMovement
   (start = player location, delta, 0, 70, direction, 6132); submit the
   task (delay 3) via the lazily-resolved TaskManager; send sound 2462
   (delay 25); reset click delay. Return true (claim) on EVERY path,
   including the guard paths: TS `crossDitch` returns undefined, and
   under the Go `void | boolean` handler shape (objects.go:158-168) any
   result other than exactly false claims the event — a blocked Cross
   is consumed silently, exactly as in TS. (The one exception is a nil
   TaskManager, which cannot happen in production: return false there
   so the failure is visible instead of silently swallowed.)
   Confirm "Cross" is the option at the clicked slot for object 23271
   in the rev-242 cache (def interactions) — if the cache names the
   option differently, say so and match the cache, not the plugin.
5. Blank import in `cmd/server/pluginwiring.go` (alphabetical position).
6. Gap 11 test: probe (temporary test, deleted after, per prior-phase
   practice) against the real rev-242 cache
   (`/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`): object
   def 23271 (name, interactions, size) via `CacheDefinitions.GetObject`,
   and the map locs in the y 3521–3522 / x 3038..3347 band via the
   region decode path `RegionManager.LoadRegion` uses
   (`internal/game/regions.go`, `regions_decode.go`). If the clipped
   rows are 23271 ditch locs, correct gap 11 in PORT_STATUS.md (the
   wall IS the ditch, crossed only via this interaction — not a data
   bug) and update the gap-11 text at lines ~3300 and ~3855. If they
   are NOT the ditch, report the evidence and leave gap 11 as-is.
7. Tests (`forcemovement_test.go` in `internal/game`,
   `wildernessditch_test.go` in the content package, sync tests where
   the snapshot mapping lives):
   - `resolveJump` table: face 0/2 both sides of threshold 3522
     (incl. the default when the ditch location is absent), face 1/3
     both sides (incl. the 2996,3530 turn), exact dx/dy/direction.
   - Guards: active ForceMovement → no task, no sound, falls through;
     click delay reset this cycle → no cross; never-reset → crosses.
   - Task: executes on tick 3 (lands start+delta, placement set,
     force movement cleared); a replaced force movement aborts it;
     `Stop` before execute clears only the owned instance.
   - Mobility: walk request while force movement is active → INVALID
     (silent); after clear → MOBILE.
   - Sync: snapshot carries the forced-movement view + end tile +
     animation 6132 on the dirty tick; end tile only (no view) once
     the flag clears; direction table 0/1/2/3 → 1024/1536/0/512.
   - Registration: `OnObjectInteractionNamed("Wilderness Ditch")`
     present with the Cross action; a click on a different object name
     does not fire it.
   - Sound: `SendSound` with the ditch sound reaches a session double
     as effect 2462 delay 25 (or the unit-level equivalent the sounds
     tests use).
8. Live verification (fresh temp root `/home/hatch/smoke40/`, real
   rev-242 cache, as in prior phases; do NOT touch `/home/hatch/live36`
   or `~/tsps-full`): small dedicated driver (copy the login/session
   helpers from the smoke39 driver; object click = the existing
   object-action opcode path the woodcutting driver uses): walk to the
   ditch at the Edgeville crossing (south side, y≈3520), Cross, and
   confirm the player lands 3 tiles north inside the Wilderness with
   animation 6132 + sound 2462 on the wire; then Cross back south the
   same way; also verify the north-south turn section (2996,3530-3533)
   if reachable; and verify walking INTO the ditch without Cross still
   blocks (stops at the ditch edge — the Phase 4 behaviour). Run legs
   singly (the full-driver pass exceeds the background-exec wall clock,
   per the smoke36 lesson).
9. Docs: PORT_STATUS.md — the P3 content row 10 → **11 of 577** with the
   WildernessDitch entry; gap 11 corrected or confirmed with evidence;
   a new **§23 residual gaps** (whatever is left: e.g. other
   ForceMovement consumers still unported, the forced-walk chain
   exception NOT ported, `needsPlacement` INVALID gate still a gap).

## Deliberate deviations (document in code)
- None expected beyond the documented ones: the click delay reads the
  world cycle through `combatCycle()` (the established shared source),
  and the forced-walk chain exception (`area.handleForcedWalk`) is not
  ported — Go has no Area hook of that shape, and no ditch behaviour
  needs it.

## Explicitly out of scope
- The Shopkeepers plugin (Phase 41), Mining (Phase 42), Smithing
  (Phase 43), any other Wilderness content (obelisks, Diary, Demonic
  Ruins), the client, or the Android app.
- A general ObstacleRunner: port only the ForceMovement execution path
  the ditch needs.
- Any change to the registry, the chat listener, the container, the save
  format, or the client.
- No new dependencies.

## Results (2026-11-21, verified)

- Gap 11 RESOLVED by probe (temporary test, deleted after use):
  def 23271 "Wilderness Ditch" (actions ["Cross","","","",""],
  size 1x2, clipType 1), 273 locs in band y3521–3522, x3042..3327,
  faces 0 (81) and 2 (192), all type 10; clip(3090,3521) =
  clip(3090,3522) = 0x40100, clip(3090,3520) = clip(3090,3523) =
  0x0; turn section 4 locs at (2996,3530–3533), face 3. The
  zero-XTEA theory is dead; PORT_STATUS gap 11 corrected.
- Ported: `internal/game/forcemovement.go` (ForceMovement,
  ForceMovementTask with constructor side effects +
  identity-checked execute + OnStop clearIfOwned,
  ClickDelayElapsed/ResetClickDelay on the combatCycle
  source), `Task.OnStop` hook (fires once via Stop(), never
  on done), Player `forceMovement` + `clickDelayCycle`
  (-1 in NewPlayer), mobility INVALID gate in
  `getMobilityIgnoringFreeze` (needsPlacement stays a gap),
  sync snapshot fields + `forcedMovementViews`
  (PlayerSession.ts:742-789, direction table
  [1024,1536,0,512], animation fallback), content package
  `internal/content/wildernessditch` (resolveJump verbatim
  incl. nil defaults, guards in TS order, handler claims on
  every path except nil TaskManager → false).
- Full suite: 4,696 passing (3,548 top-level + 1,148
  subtests), 0 failures, 23 packages; `go test -race ./...`
  clean 23/23 (the phase-37-suspect combatant.go race still
  does not reproduce). Build/vet/gofmt/CGO0 clean.
- Live (smoke40, real rev-242 cache, legs run singly):
  ditch-south PASS 4/4 (landed (3090,3523), anim 6132,
  sound 2462 delay 25), ditch-north PASS 4/4 (landed
  (3090,3520)), ditch-turn PASS 4/4 (landed (2998,3530)),
  ditch-wall PASS (stayed (3089,3520)). Arrival-tile
  finding: a diagonal corner tile does not reach a 1x2
  solid (reachRectangle1), so the player steps to the
  side-adjacent tile before jumping; the turn leg's
  north-adjacent tile (2996,3531) is itself a ditch piece
  (walled), so its arrival is the west side (2995,3530).
  Two test fixes during the run (both test-side, none in
  the port): the click-delay tests stamp from the test's own
  cycle source (a prior test's leftover cycle broke the
  full-suite run), and the sound fake needed SendAreaSound
  to satisfy the SoundSender interface.
- Docs: PORT_STATUS content row 11 of 577, gap 11 corrected
  (3 passages), new §23 residual gaps, "What was verified"
  Phase 40 entry.
- Not committed (parent handles git). Live36 untouched.

## Environment notes
- Go 1.25.1 at `~/workspace/tools/go/bin/go`. No new dependencies.
- Phase 39 is committed (190743f); the tree is clean. The live36 servers
  (Go :43594, client :3000) are running for the parent's browser test —
  do NOT touch `/home/hatch/live36` or `~/tsps-full`'s running client.
  Use a fresh temp root for the live check, as prior phases did.
- Verification: `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `CGO_ENABLED=0 go build ./...`, full suite `go test ./... -p 1
  -count=1 -v` with exact pass counts, `go test -race ./...`.
