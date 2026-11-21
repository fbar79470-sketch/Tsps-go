# Phase 7 plan (COMPLETED 2026-11-15 — approved 2026-11-14)

Movement completions: run-energy drain/restore, the destination-flag
encoder, the run-toggle listener, and the remaining mobility gates.
This was PORT_STATUS.md next-module #1. It did NOT add the full
SkillManager, containers, combat, dialogue, shops, trading, NPC
drivers, or any other listener implementation.

Status: completed and verified (see Outcome). All three workers
finished; the coordinator integrated, updated the docs, and
verified personally, including the live smoke test.

## 0. Coordinator groundwork (done, tree is green)

The game layer must never import the protocol package, so the two
packets this phase sends go through typed `SessionPort` methods
(`internal/game/player.go`), implemented by `*Session` in
`internal/session/runstatus.go` on top of the existing
`EncodeRunEnergy` and the new `EncodeDestination`
(`internal/protocol/destination.go`, byte-exact per
`ClientProtocol.ts:2214-2218`). `MovementOwner` gained
`IsPlayerBot()` (TS `isPlayerBot`/`isUnwatchedBot`) for the queue's
bot branches. Test doubles (`fakeSession`, `recordingSession`,
`fakeOwner`) were updated. Build/vet/full suite pass on this
contract alone; no behavior changed.

## 1. Run-energy drain/restore (Worker A)

Grounded formulas (all verified in the TS source):

- Units: `RUN_ENERGY_MAX_UNITS = 10_000` (`GameConstants.ts:30`);
  100 units = 1%. `getRunEnergy()` returns units/100;
  `setRunEnergy(percent)` sets units to
  `floor(percent*100 + 1e-6)`; `setRunEnergyUnits` clamps to
  0..10,000 and floors (`Player.ts:1257-1277`).
- Drain, one running tick (`MovementQueue.runEnergyDrainUnits`,
  `MovementQueue.ts:803-808`): weight clamped 0..64 kg, Agility =
  boosted current level; `base = floor((60 +
  floor(67*weight/64)) * (1 - agility/300))`, then drain
  modifiers, floored, clamped >= 0.
- Restore, one tick not spent running
  (`MovementQueue.runEnergyRestoreUnits`, `MovementQueue.ts:811-815`):
  `base = floor(agility/10) + 15`, then restore modifiers,
  floored, clamped >= 0.
- Drain site (`drainRunEnergy`, `MovementQueue.ts:817-833`): only
  for players, only when run-toggled and `stepsMoved >= 2`
  (a lone step is a walk); sets `ranThisCycle`; subtracts drain
  units; at <= 0 clamps to 0, clears both run overrides,
  `setRunning(false)`, sends run status; always sends run
  energy. Bots are pinned to max instead of draining.
- Restore site (`Player.ts:559-566`): every tick not spent
  running (`didRunThisCycle` false), not a bot, energy < max:
  add restore units (if > 0) and send run energy. Runs after
  movement in the TS turn order.
- Modifier hooks (`EquipmentEffects.ts:303-325`): the drain,
  restore, and status modifier lists are equipment/plugin
  populated (graceful set, endurance ring, stamina). With no
  containers/equipment/plugins in Go they are identity; the
  omission is documented, not silent.
- Minimal SkillManager surface: ONLY the Agility current-level
  read path. `getCurrentLevel` (`SkillManager.ts:425-433`) reads
  `skills.level[idx]` with sanitize (non-finite/negative -> write
  0, return 0). Agility array index is 16 (declaration order in
  `Skill.ts`, matching `SkillCount = 24` / `HitpointsIndex = 3`
  in Go). No XP, no leveling, no other skills, no boosts (no
  boost source exists; current level = save level, static).
- Weight input: TS `getWeightKg` is live from equipment +
  inventory (`Player.ts:1287-1315`); Go has no live containers,
  so the entry-time carried weight (Phase 3 `ItemWeights`
  plumbing, same value the bootstrap sends) is the weight for
  the session. Documented at the site.
- Save integration (required): `runEnergy` moves from
  preserved-verbatim to owned. TS: `applyToPlayer` calls
  `player.setRunEnergy(this.runEnergy)` (`PlayerSave.ts:654`);
  `fromPlayer` writes `playerSave.runEnergy =
  player.getRunEnergy()` (`PlayerSave.ts:762`). Update
  `OwnedState`/`WithOwned`, the entry application, and the
  PORT_STATUS.md owned-vs-preserved split; every other preserved
  field stays untouched.

## 2. Destination-flag encoder + sync (Worker B)

- Encoder exists (coordinator groundwork, §0): `EncodeDestination`
  + `SessionPort.SendDestination`. Worker B adds byte-exact tests.
- `syncDestinationFlagToRoute` (`MovementQueue.ts:446-461`):
  bots excluded; destination = last queued point or (0, 0);
  dedup against `lastSentDestX/Y` (sentinel `MIN_SAFE_INTEGER`,
  already staged in Go as `destUnsent`); sends on change only.
- Send sites (all verified): `PathFinder.ts:177,206` (route set),
  `MovementQueue.ts:656` (queue emptied in process),
  `MovementQueue.ts:884` (reset with clearDestination),
  `MovementQueue.ts:329` (setPursuitCheckpoint — pursuit-only,
  NOT in Go scope). Worker B wires the sites in files it owns
  and specifies the rest exactly; the coordinator wires
  `movement_queue.go` call sites during integration (file
  owned by Worker A in wave 1).
- Outcome: Worker B found the exported stub and all four
  in-scope call sites already existed in `movement_queue.go`
  from phase 4 (applyWalkRoute failure/success, Process
  queue-empty, Reset with clearDestination) and delivered the
  real sending method (`destination.go`) + byte-exact encoder
  tests + 10 game tests; the coordinator replaced the stub
  body with delegation. No wiring in `pathfinder.go`/
  `routefinding.go` was needed (the route-set path runs
  through `applyWalkRoute`). Deviation: Go excludes ALL bots;
  TS excludes only unwatched bots (no has-network-client
  distinction in Go).

## 3. Run-toggle listener (Worker C, wave 2 — depends on §1)

- TS has no listener file for this: it is the `varp_transmit`
  case in `dispatch()` (`NetworkBuilder.ts:1147-1168`; the 173
  branch is lines 1161-1165 — line numbers in the original
  brief drifted slightly and the code cites actual lines).
  Decode: `varpId: short, value: int` (`ClientProtocol.ts:905`,
  opcode 191, already `ClientVarpTransmit` in Go's tables).
  Only `varpId === 173` toggles running:
  `setRunning(value !== 0 && getRunEnergy() > 0)` then
  `sendRunStatus()` (the run-energy packet, sent
  unconditionally in the branch) plus the `player:run-toggled`
  plugin event (not emitted — no plugin system) and the
  `player:varp-transmit` `handled` short-circuit (omitted —
  nothing can handle it). Other varpIds in that case (43
  combat style, 172 auto-retaliate, audio settings for every
  id) decode-drop with a named reason, never invented. The
  packet flows to `listeners.Handle` via the default
  turn-dispatch case; Handle stays safe for session-less
  players (nil-player drop mirroring TS's `if (this.player)`
  guard). Outcome: implemented, first `Outcome.Implemented`
  use, `Statuses()` 19th row (wording pinned by test and
  recorded in PORT_STATUS.md).

## 4. Remaining mobility gates (Worker C, wave 2)

- TS `getMobility()` (`MovementQueue.ts:337-382`): FREEZE timer
  -> FROZEN_SPELL; else `getMobilityIgnoringFreeze`: STUN timer
  -> STUNNED; needs-placement / movement-blocked / force
  movement -> INVALID; combat animation stall ->
  ANIMATION_STALL; trading/duelling button delay -> BUSY; duel
  NO_MOVEMENT rule -> DUEL_MOVEMENT_DISABLED; else MOBILE.
  `Mobility.sendMessage` needs the chat encoder (NOT STARTED);
  FROZEN_SPELL sends nothing live.
- Implement ONLY gates whose underlying state exists in Go;
  gates depending on unported systems (timers, combat,
  trading, duelling, force movement, placement) are explicit
  documented gaps naming the missing system. `blockMovement`
  behavior stays intact.
- Per-gate outcome (Worker C audit, verified against
  `MovementQueue.ts:337-376` + `Mobility` at 1969-2024; also
  in PORT_STATUS.md): `blockMovement` -> INVALID implemented
  (pre-existing, intact); FREEZE -> FROZEN_SPELL gap (no
  `TimerRepository`); STUN -> STUNNED gap (no timers);
  needs-placement -> INVALID gap (field never set by
  production code); force movement -> INVALID gap (no
  `ForceMovement`); combat animation stall -> ANIMATION_STALL
  gap (no combat); trading/duel button delay -> BUSY gap (no
  trading/duelling); duel NO_MOVEMENT -> DUEL_MOVEMENT_DISABLED
  gap (no duelling); `Mobility.sendMessage` gap (chat encoder
  NOT STARTED). No `Mobility` type was ported.

## Explicitly out of scope

Full SkillManager/XP/leveling/all-skills; containers/appearance
(Inventory/Equipment/Bank); deferred flush blocks (music/ground
items/NPC effects); any other listener implementations; NPC
AI/drivers; combat/dialogue/shops/trading systems; save-system
extensions beyond the runEnergy ownership change; plugin-strategy
decision; real-production scrypt-hash check.

## Verification (coordinator, at completion)

`go build ./...`, `go vet ./...`, `gofmt -l .` (clean),
`CGO_ENABLED=0 go build ./...` (clean),
`go test ./... -p 1 -count=1` and `-race` (serialized: the known
`TestEngineTicksAtRate` parallel flake): all 9 packages pass,
1,210 test cases including subtests, 0 failures; race passes.
Live smoke test (throwaway in-module WebSocket test, since
removed; 10 ms ticks, memory saves): seeded 2.0% energy,
walked far with run on — RUN_ENERGY 200 -> 0 with running
clearing at depletion (auto-stop), then +15/tick restore;
DESTINATION target then (0, 0); varp_transmit 173 values 1/0
flipped the running byte. Stable across 3 runs. No cache
download for this phase. Diagnosis note: the first smoke-test
attempts "saw" a dead server — the cause was gorilla's sticky
`readErr` after client read-deadline timeouts in the test
harness, not the server; the working test used a deadline-free
reader goroutine (see PORT_STATUS.md).

## Outcome

Completed 2026-11-15. Running now tires you out, verified live
end-to-end: energy drains per the TS formula, auto-stops at
zero with the TS double send, restores on non-run ticks, the
destination flag tracks the route, and the run toggle works
over the wire. `runEnergy` is now an owned save field. All
mobility gates beyond `blockMovement` are documented gaps.
Remaining gaps: identity energy modifiers (no equipment/
plugins), static weight/Agility, stamina always 0, all-bots
destination exclusion, other varpIds, `Mobility.sendMessage`,
NPC drivers, dynamic occupancy. Next module: deferred flush
blocks (see PORT_STATUS.md).
