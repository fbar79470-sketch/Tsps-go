# Phase 15 plan

Migration plan §2 milestone 5, dispatched 2026-11-15 under Tyler's
unattended-progression authorization (2026-11-14, overnight). No
separate per-phase approval was sought.

## Scope

1. **Timers** — port `TimerRepository`/`Timer`/`TimerKey`
   (`util/timers/`, Mobile.ts:73, `getTimers()` at 963) as core
   defines them: lazy per-Mobile repository, countdown
   semantics (tick decrements while > 0, never below 0, entry
   stays at 0), all repository methods (`has`, `register`,
   `registerTimerKey`, `left`, `willEndIn`, `getTicks`,
   `registers`, `extendOrRegister`, `addOrSet`, `cancel`,
   `process`, `hasActive`, `timers`), the 11 `TimerKey`
   singletons with `BOT_WAIT_FOR_PLAYERS`' default ticks,
   firing in `Player.process()` before movement
   (Player.ts:501) and in `NPC.process()` (NPC.ts:309),
   replacing the `TimerHooks` no-op stand-in. Session-only:
   no timer-repository state in `PlayerSave` (verify, don't
   assume). FREEZE/STUN mobility gates become implementable
   with timers — wire them iff TS grounds them in
   `MovementQueue`/`Mobility`, else document the gap.
2. **Areas/zones** — port the `Area`/`AreaManager` system
   (`game/model/areas/`, `Boundary`): abstract `Area`
   (boundaries, member maps, enter/leave/postEnter/postLeave,
   `process` no-op, `getMusicTrack`, rule hooks, `isMulti`),
   `AreaManager.process` (enter/leave/process/multi-icon
   algorithm with boundary hints + the bucketed search index,
   or a faithful equivalent), `get`/`inside`/`inMulti`,
   called from `Player.process()` after movement/walk-to/
   aggression (Player.ts:553) and `NPC.process()`
   (NPC.ts:378). Areas are code-registered via
   `api.registerArea` — no JSON data files, no built-in
   areas in core. `Wilderness` static helpers
   (`isInLocation`, `isPvpArea`, `levelAt`, `isMulti`,
   `intersectsArea`, `isInSafeBuilding`) over the world.json
   zone boundaries (`WORLD_ZONE_BOUNDARIES`), with the
   `setLevelSource` registry. Repair the checks that
   currently degenerate for lack of areas where TS grounds
   them (Wilderness drop check, multi-icon update, area
   `getMusicTrack` in the music block). Gameplay areas are a
   DIFFERENT system from the Phase 8 music areas — do not
   conflate them. `PrivateArea`/`TemplatedInstanceArea` are
   instancing (milestone 11) — ground and document as gaps.
3. **Zone events** — port `onZoneEnter`/`onZoneExit`
   (PluginTypes.ts:1016-1018, PluginManager.ts:891-951,
   3311-3321): rectangular `PluginZone` (levels omitted =
   all planes), separate enter/exit hook entries, per-player
   active sets keyed by hook index, driven by
   `updatePlayerZones` from `emitPlayerProcess` BEFORE the
   process hooks run, with warn-and-continue error
   isolation. Hub registration methods + production emit
   sites; `game` never imports `plugins`.
4. **Hub exposure** — timer + area surface as `PluginApi`
   defines it: `registerArea` (sets pluginName, pushes onto
   the registry; `callArea` error isolation), zone hooks,
   and any area-query/timer members content uses (ground
   whether timer access is a `Mobile` method only). Update
   the hub table counts.
5. **Save/state implications** — verify from TS whether any
   timer/area state persists; handle through the
   owned/preserved split, no owned fields without grounding.
   Document explicitly if nothing persists (expected:
   timers session-only, areas runtime-only).
6. **Doc cleanup** — `internal/game/doc.go` still lists stale
   NOT-ported entries for systems now ported
   (skills/containers/appearance/sync/ground-items/
   persistence). Verify each entry against PORT_STATUS.md
   and correct the stale ones. Touch nothing else in that
   file beyond what verification supports.

## Out of scope

NPC drivers + combat (milestone 6), shops/trading (7),
interface runtime (8), clans/social (9), skill handlers + XP
(10), sailing/template regions/sounds (11), content ports
beyond proof, the pre-existing far-WALK routing bug (use
staged clicks in the smoke test), embedded JS/sidecar,
scrypt check, MobileList.All perf work.

## Verification

`go build ./...`, `go vet ./...`, `gofmt -l .` clean,
`go test ./... -p 1 -count=1` + `-race`, CGO_ENABLED=0 build,
and a live smoke test: a timer registered through a hub hook
fires after the expected ticks (observed via a reply); walking
into a fixture zone/area fires enter, walking out fires exit;
a cancelled timer never fires. Claim "timers fire and zones
react" only if verified live end to end.

## Outcome

COMPLETED 2026-11-15. All six scope items landed:

1. Timers: `internal/game/timers.go` — full repository, 11
   singletons, TS tick-phase placement, `TimerHooks`
   removed; FREEZE/STUN gates wired (STUN refusal line
   sends, FROZEN_SPELL sends nothing); session-only
   verified (zero `PlayerSave` references).
2. Areas/zones: `internal/game/area.go`,
   `areamanager.go`, `boundary.go`, `wilderness.go` —
   code-registered areas, bucketed search, membership
   evaluation in the TS positions, multi-icon varbit
   follow with the stale-area subtlety preserved,
   Wilderness helpers, live drop check, `getMusicTrack`
   override; `PrivateArea`/instancing documented gaps.
3. Zone events: `internal/plugins/zones.go` —
   `OnZoneEnter`/`OnZoneExit`, `updatePlayerZones` first
   in `EmitPlayerProcess`, index-keyed active sets,
   disconnect cleanup, warn-and-continue isolation.
4. Hub exposure: `registerArea` + queue/flush +
   isolating `AreaMethodCaller`; audit found timer access
   is a `Mobile` method only (no `PluginApi` surface) and
   `getAreaManager` has zero TS callers (stays table-only).
   Counts: 23 of 108 hook families firable, 13 of 95
   non-hook members ported.
5. Save/state: nothing persists — timers session-only,
   areas/zones runtime-only, no save fields added.
6. Doc cleanup: `internal/game/doc.go` NOT-ported list
   corrected against PORT_STATUS.md.

Verification: `go build`/`go vet`/`gofmt` clean,
`CGO_ENABLED=0 go build` clean, `go test ./... -p 1
-count=1` all 12 packages pass (2,497 cases incl.
subtests, +141), `-race` passes, and the live smoke test
passed 3/3 runs (timer fired exactly once, zone/area
enter + exit/leave on walks in/out, cancelled timer
never fired). Probe plugin + client removed afterward;
no artifacts left in the repo. Gaps recorded in
PORT_STATUS.md: `syncWildernessStateForMovedPlayer`,
`PrivateArea`/instancing, `getAreaManager` query surface.
The far-WALK bug remains open and untouched.
