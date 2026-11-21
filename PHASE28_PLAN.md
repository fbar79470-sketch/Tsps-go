# Phase 28 plan — DRAFT: poison/venom + skulls + player death

Status: COMPLETE (2026-11-17, coordinator-verified; parent
verification + Drive archive still to come). Dispatched
2026-11-17 under Tyler's "keep going without confirming";
quests stay postponed, Doric's Quest stays
picked-but-deferred. Three workers + coordinator integration.

## Why this phase exists

Phase 26's next-slice brief (PHASE26_PLAN.md, "Proposed order after
Phase 26"): "Phase 28 — poison/venom + skulls + player death:
`PoisonClock` + `CombatPoisonEffect`, skull system through the
existing seam, `PlayerDeath.ts` item-loss/kept-items/grave flow
(441 lines, heavy interface dependencies — may split)." Phase 27
landed spells; this is the next slice. The three pieces are
coupled (death's kept-items count depends on skull status;
death's reset clears poison and skulls), so they land together,
each behind a contract the others can call.

## Grounded shape (all at bfd81e3; do not re-derive, verify)

### Poison / venom — `PoisonClock.ts` (159) + `task/impl/CombatPoisonEffect.ts` (526)

- `PoisonClock` (shared player status pulse; advancing it is
  owned by CombatPoisonEffect): `PERIOD_TICKS = 30`,
  `PERIOD_SECONDS = 18`, `VENOM_CUTOFF = 38`. `start(seconds)`:
  counter = ceil(seconds / 18), never moves the phase of an
  existing pulse. `protectBriefly(seconds, venom)` (equipment/
  food flags; neither replaces a dose nor cures).
  `reduceProtection(seconds)`: removes protection time without
  moving the pulse; records `pendingReduction {before, after}`.
  `tickPulse(modal)`: counts down `ticksUntilPulse`; a due
  pulse decrements the counter (protection-ticks path holds a
  due transition while modal). `isVenomImmune()`: counter > 38
  or brief-venom running. `snapshot()` / `restore(state,
  legacySeconds)` with strict validation (counter bounds,
  `ticksUntilPulse` 0..30, protection-ticks consistency,
  brief seconds); legacy seconds ≤ 2 protect briefly, else
  rebuild counter + phase. `secondsRemaining()` =
  max(dose ticks * 0.6, brief-poison seconds). Save shape
  already exists in Go (`persist/playersave.go:503-512`,
  `PoisonClockState`; `apply.go:395-409` hydrates it) — the
  live clock does not.
- `CombatPoisonEffect` (one task per entity, WeakMap-keyed):
  30-tick pulse; poison damage = `floor((severity + 4) / 5)`
  (`damageFromSeverity`), severity decays −1 per hit; venom:
  the stored value IS the damage, +2 per hit, cap 20, no decay
  or expiry — only a cure ends it. Hits go through the hit
  queue as `HitMask.GREEN`. On death (HP ≤ 0): poisonDamage 0,
  venomed false, player clock stopped. `resume(entity)` on
  login / combat bind (`Combat.ts:811`, `Player.ts:801`);
  `suspend(entity)` before logout save (`Player.ts:757`).
  `ORB_VARP = 102`: venom = 1,000,000 + hits dealt
  (`round((poisonDamage − 6) / 2)`, dark green), poison =
  severity, immunity = −ceil(seconds / 18) while > 2s.
  Resistance lines (purple `<col=7f007f>`): counter 1 "about
  to wear off", 0 "has worn off", 39 "venom resistance is
  about to wear off", 38 "venom resistance has worn off, but
  you are still resistant to poison"; reduction lines on
  tier crossings. `damageFromSeverity` is the only formula.
- `PoisonType` enum: VERY_WEAK 2, WEAK 3, MILD 4, EXTRA 5,
  SUPER 6, VENOM 12. `CombatPoisonData.types`: ~250 poisoned
  item ids → type (script-extract, Phase 26 ranged precedent;
  ids from `ItemIdentifiers.ts`, never by hand).
  `getMeleeSeverity`: EXTRA 25, SUPER 30, VENOM 6, default 20.
  `getRangedSeverity`: WEAK 11, MILD 16, VENOM 6, default 6.
  `getMeleeRollRange`: noxious halberd 2, else 3.
- `CombatFactory.poisonEntity(entity, severity, orbType=1)`
  (`CombatFactory.ts:1241-1300`): severity ≤ 0 returns; venom
  can't re-apply/restack/downgrade once active; NPC
  venom/poison immunity via definition (`isVenomImmune` /
  `isPoisonImmune` — venom-immune converts to poison,
  severity × 5); player immunity via the clock (venom blocked
  if `isVenomImmune()`, poison blocked if clock not
  `finished()`); `combat:status-apply` custom event
  (`{target, kind, allow}`) may veto; accepted venom clears
  the poison counter; message "You have been poisoned!" /
  "You have been poisoned by venom!" only when starting from
  nothing; `setVenomed` / `setPoisonDamage` (venom sets the
  value, poison takes max of current/new); submits the
  effect task only if not already poisoned.
- Application call sites (`CombatFactory.ts` executeHit):
  melee ~1086-1096 (`Misc.getRandom(getMeleeRollRange) === 0`,
  plus a 1-in-8 roll at 1096 — read the site, do not guess
  what it gates); ranged ~1100-1123 (thrown vs ammo slot,
  toxic blowpipe excluded — its venom is the
  ToxicBlowpipe plugin's, 1 in 4); NPC ~1126-1145
  (`isPoisonous() || venomous`, `Misc.getRandom(10) <= 5`,
  severity venomous ? 6 : 30, `combat:npc-poison` event with
  `{attacker, target, severity, venomous, handled}` first).
- Go state: NO live poison fields on Mobile/Player (only
  persist save fields `poisonDamage`, `venomed`,
  `lastVenomDamage`, `poisonImmunityTimer`, `poisonClock`).
  Go has `HitMaskGreen` (`combat_hitqueue.go:23`) and the hit
  queue. Cures are content (antipoison etc.): the engine
  provides the cure API (clear fields + stop effect); no
  cure items are ported here.

### Skulls — `CombatFactory.handleSkull/wouldSkull` + `Player.processSkullTimer`

- `SkullType`: WHITE icon 0, RED icon 1 (`model/SkullType.ts`).
  `isSkulled()` = skull timer > 0 (verify at
  `Player.ts:1377`). `getHeadSkullIcon()`: override-always,
  else skulled ? icon : −1. `setSkullIconOverride(icon, always)`
  flags APPEARANCE.
- `handleSkull` (`CombatFactory.ts:1388-1420`): wilderness-only
  (both fighters); `wouldSkull` first; target records the
  attack (retaliation without a skull from now on, before the
  hit lands); a skulled attacker extends the skull
  (30 minutes) unless retaliating; an unskulled attacker who
  would skull gains a WHITE 30-minute skull. `wouldSkull`:
  both in Wilderness, attacker unskulled, no damage-map
  entries either way, neither fighter the other's attacker,
  not attacked-by (100-tick window). `PVP_SKULL_SECONDS =
  1800`; `ITEM_SKULL_SECONDS = 1200` (amulet of avarice /
  cape of skulls — plugin content; ground whether core ever
  applies item skulls, and if not, the constant is recorded
  only).
- `processSkullTimer` (`Player.ts:1385-1401`, called from
  `Player.process` at :556, i.e. after area processing, before
  run-energy restore): skipped if not skulled or dying; a
  skull "cannot disappear during PvP combat" — its last tick
  waits while a player is this player's target or attacker,
  or a player hit is still incoming; otherwise timer−−; at 0
  flag APPEARANCE. Go's `Player.Process` documents this exact
  slot (area processing → skull timer → run-energy restore)
  but has no skull step — the step lands here.
- Red skull: `CombatFactory.skull` sends "@bla@You have
  received a @red@red skull@bla@! You can no longer use the
  Protect item prayer!" for RED, "You've been skulled!" for
  WHITE. Ground where (if anywhere) core applies a RED skull;
  if no core call site exists, RED stays a supported type
  that core never applies, documented — do not invent one.
  (PrayerHandler.ts:371 reads red-skull state for Protect
  Item; death's `getAmountToKeep` returns 0 for red skull.)
- Go state: decision logic already exists
  (`HandleSkull`/`WouldSkull`/`RecordAttackedBy`/
  `WasAttackedBy`, `combat_factory.go:1302-1390`) through the
  `ApplySkull`/`IsSkulled`/`ExtendSkull` seam
  (`combat_hooks.go:263-269`), which currently has NO live
  system behind it (wiring supplies nothing). Bootstrap
  already reads the save for the appearance skull icon
  (`session/bootstrap.go:331-340`: `SkullTimer > 0` →
  `SkullType.IconID`). The live system (timer in ticks,
  type, icon override, process step, wiring) is this phase.
  `getAmountToKeep` (death) reads skull state — Worker C
  calls Worker B's API, never the seam directly.

### Player death — `task/impl/PlayerDeath.ts` (441)

- `PlayerDeathTask` (Task, 1-tick): `ticks = 4` — respawn
  four ticks after animation 836 starts (rsprox 426, 1084,
  1085, 2508). Case 4: combat reset, `setUnderAttack(null)`,
  following/interaction/face/force-movement cleared, movement
  queue blocked + reset, untargetable, close all interfaces
  (`sendInterfaceRemoval`), death animation 836,
  `player:dying` event. Case 0 (respawn tick): death message
  + jingle 90 (message, jingle and map rebuild share one
  tick); `onShouldDropItemsOnDeath` first-answer gate
  (`loseItems`, default true); bones: `player:death-bones`
  event (`{player, killer, location, allow}`), then a BONES
  item to the ground (owner = killer, else player) if
  allowed; `dropItems` if losing; `resetAttributes()`;
  kept items returned; `emitPlayerDefeated(killer, player)`
  for ALL deaths; save "death"; `onPlayerDeath` event
  (`{player, killer, itemsLost, handled}` — first handler
  that sets `handled` stops the chain, e.g. a quest's own
  respawn); `player:died` custom event for every observer;
  if not handled: `player:respawn` custom event, else
  `moveTo(DEFAULT_LOCATION)` (bot resolver first);
  save "respawn". On error: `resetAttributes` + move home.
  Go's state machine (`combat_death.go`) already runs the
  4-tick shape and case-4 resets; `respawnPlayer` currently
  sends the message and the death event only — no bones, no
  item loss, no reset, no saves, no teleport, no
  `player:died`/`player:respawn` events, no jingle. The
  teleport target is `GameConstants.DEFAULT_LOCATION =
  WORLD_SPAWN` (`WorldDefinition.ts` — ground the coords;
  Go has no such constant yet).
- `dropItems(player, killer, position)`:
  `getItemsToKeep(player, getAmountToKeep(player),
  protectsCosmetics(player, killer))`; inventory +
  equipment items split (Wilderness PvP kill only:
  `deathComponents` from item-gameplay.json — NOT imported,
  decision pending with Tyler, so NO component splitting;
  document the degradation); per-item
  `onPlayerDeathItemDrop` event (`{player, killer, item,
  kept, location, shouldDropItems, dropEligible,
  suppressDefaultDrop, handled, extraDrops}` — handled skips
  the item, extraDrops are lost with it, suppressDefaultDrop
  drops nothing); `keepsEverything` (OWNER/DEVELOPER rights
  unless `::mortal` — Go has NO rights field, Phase 20
  documented partial, so keepsEverything is false in Go;
  document it); `isLostOnDeath` items skipped (kept out of
  drops); untradeables (non-Barrows) → `lostUntradeables`
  (kept unless a death-drops hook takes them — the grave);
  tradeables → `droppedItems`; `onPlayerDeathDrops` event
  (`{player, killer, location, items, untradeables, layout}`
  — emitted even with nothing lost, a death still changes
  an existing grave); dropped items to the ground for
  `lootOwner` (killer, unless `player-death:loot-owner`
  says otherwise; an Ironman killer takes no other player's
  loot — Go has no Ironman mode; document); kept items +
  lostUntradeables returned; killer message
  "`<name>` had no valuable items to be dropped." when
  nothing dropped and no plugin handled it; inventory +
  equipment reset + refresh. `returnKeptItems` adds clones
  to the inventory (overflow: ground? — read the add path;
  Go inventory add failure must not silently eat items).
  `loseItemsAsDeath` (raid hold) is a static helper — port
  only if it falls out of the same code, else document.
- `getAmountToKeep`: red skull → 0; skulled → 0; else 3 +
  Protect Item 1 (max Prayer level ≥ requirement AND current
  Prayer > 0 AND PROTECT_ITEM active — ground the
  requirement from `PrayerData`; Go prayer state exists from
  Phase 16). `protectsCosmetics`: false for a PvP kill or in
  the Wilderness; else the varbit 10987
  (`death_waivecosmeticsprotection`) setting (Go varbit store
  exists from Phase 18 — read it; default on when unset).
  `isCosmetic` via `player-death:cosmetic` event (OrnamentKits
  answers — content; no answerer in core, so no item is
  cosmetic in core).
- `keepValue(item)`: GE value if any, else high alch, else
  floor(store value × 0.6). Go has the accessors
  (`ItemHasGrandExchangeValue` / `ItemGrandExchangeValue`,
  `ItemHighAlchValue`, `Item.Value` — verify the exact names
  and the data behind them; without item-alch.json the alch
  path may read cache data only — ground it, do not assume).
  `getKeptItems`: skip null/empty/lost-on-death; per-item
  `onShouldKeepItemOnDeath` (false = never kept, true =
  always kept beyond the count); sort by keepValue desc;
  ranked = first N; cosmetic = the rest that are cosmetic
  when `cosmetics` is true. `slotLayout` (inventory +
  equipment ids, −1 empty) goes to the drops event.
- `resetAttributes` (`Player.ts`, the block around :270-290,
  called from the respawn tick): cancel FREEZE_IMMUNITY,
  clear cast spell / previous cast, stop prayer-block timer,
  poison 0, venomed false, wilderness level 0, recoil attr 0,
  skull timer 0 + type WHITE, weapon-interface re-assign,
  `BonusManager.update`, deactivate prayers, refresh
  equipment + inventory, every skill's current level back to
  its resting level, run energy 100 + send. Port the
  Go-applicable subset in TS order; each skipped line gets a
  one-line reason (no such system in Go yet).
- Graves are plugin content: a grave listens to
  `onPlayerDeathDrops` and takes the items. Core emits the
  hooks and, with no taker, drops to the floor. Do NOT build
  a grave here.
- Go seams already in place: `OnPlayerDeath` /
  `OnPlayerDeathItemDrop` / `OnPlayerDeathDrops` hub families
  exist in `internal/plugins/combat.go` (with tests) but have
  NO producers (`combat_death.go` only calls
  `Hooks.OnPlayerDeath`); `Hooks.OnPlayerDeath` is the only
  game-side death field — the item-drop/drops fields must be
  added (or the hub emit called directly from game via the
  wiring-installed fields; follow the Phase 27 spell-gate
  precedent: func fields on `CombatHooks`, wired in
  `cmd/server`). `Player.EmitCustomEvent` is the custom-event
  seam. Jingle: Phase 21 has `EncodePlayJingle` + a sender —
  verify the send path and use it for jingle 90. Animation
  836: no `performAnimation` (Phase 29) — record the id as
  data, do not send. Bones id: `ItemIdentifiers.BONES` —
  ground the numeric id from the cache/identifiers, never
  from memory.

## Scope

1. **Poison/venom (Worker A).** `PoisonClock` port (counter/
   phase/protection/brief timers, snapshot/restore with TS
   validation, secondsRemaining/elapsed); `CombatPoisonEffect`
   port (30-tick pulse, green hit-queue damage, severity
   decay, venom escalation cap 20, orb varp 102, resistance/
   reduction lines, resume/suspend on login/logout/combat
   bind, death clearing); `CombatPoisonData` script-extracted
   table + severities + roll ranges; `poisonEntity` +
   application call sites in the hit pipeline (melee/ranged/
   NPC, with the `combat:npc-poison` and
   `combat:status-apply` events); cure API (clear + stop);
   save round-trip through the existing persist fields.
2. **Skulls (Worker B).** Live skull state (timer ticks,
   type, icon override, `getHeadSkullIcon`), `ApplySkull`/
   `IsSkulled`/`ExtendSkull` wiring behind the existing seam
   (the seam's callers in `HandleSkull`/`WouldSkull` must keep
   working), `processSkullTimer` in the `Player.Process`
   slot (after area, before run-energy restore, PvP-combat
   hold), skull messages, death-reset clearing (via Worker
   C's reset call), login/bootstrap live-icon path, red-skull
   grounding (apply site or documented never-in-core).
3. **Player death (Worker C).** `respawnPlayer` rewrite to
   the case-0 order: message + jingle 90, should-drop gate,
   bones event + ground bones, `dropItems` (kept-items
   ranking by `keepValue`, protect-item, cosmetics, per-item
   + drops hooks, loot owner, floor fallback, killer
   message, inventory/equipment reset), kept-items return,
   `resetAttributes` subset in TS order, defeated event,
   death + respawn saves, `onPlayerDeath` handled chain,
   `player:died` / `player:respawn` events, teleport to
   `WORLD_SPAWN` (ground coords), error-path fallback.
   `getAmountToKeep` / `getItemsToKeep` / `keepValue` /
   `slotLayout` as pure functions with tests.
4. **Hub exposure** only as the port requires it, grounded
   in TS: `onShouldDropItemsOnDeath` /
   `onShouldKeepItemOnDeath` families if they are not already
   firable (check `internal/plugins/combat.go` first);
   custom events `player:death-bones`, `player:died`,
   `player:respawn`, `player-death:loot-owner`,
   `player-death:cosmetic`, `combat:status-apply`,
   `combat:npc-poison` via `Player.EmitCustomEvent`.
   Update hub counts honestly if they change.
5. **Docs:** this plan (outcome at the end), `PORT_STATUS.md`
   (header verification numbers, inventory, hub counts if
   changed, gaps, next-modules order),
   `CONTENT_MIGRATION_PLAN.md` (combat-completion progress
   + residual gaps). Workers do NOT edit the docs files —
   the coordinator integrates them from worker reports to
   avoid merge conflicts.

## Cross-worker contracts (do not deviate without telling the coordinator)

- **C1 — Poison (A owns, C calls):** `Combat.PoisonClock()`
  (or equivalent) + `Mobile.SetPoisonDamage` /
  `SetVenomed` / `IsPoisoned` / `IsVenomed` /
  `VenomGeneration` / `LastVenomDamage` accessors +
  `CurePoison(entity)` (clear fields, stop effect, reset
  clock phase per `CombatPoisonEffect.stop`). C calls only
  these for the death reset; A owns the effect lifecycle.
- **C2 — Skulls (B owns, C calls):** `Player.SkullTimer()`,
  `Player.SkullType()` (or `IsSkulled()` + type accessor),
  `Player.ClearSkull()` (timer 0 + type WHITE, the death
  reset's skull lines). The `CombatHooks` seam keeps its
  existing signatures; B wires the fields in `cmd/server`
  to the live system. C never touches the seam directly.
- **C3 — Death (C owns):** `combat_death.go` respawn path
  (`respawnPlayer`, `defaultRespawn`, `resetAttributes`
  subset) + `internal/game/playerdeath.go` (new file for
  the kept-items/drop functions) + `internal/persist`
  save-request call sites if a save API exists (ground it;
  if none, the death save is a documented gap, not an
  invented call). A and B do not touch `combat_death.go`.
- **C4 — Custom events (owner emits, via
  `Player.EmitCustomEvent`):** names verbatim —
  `combat:status-apply`, `combat:npc-poison` (A);
  `player:death-bones`, `player:died`, `player:respawn`,
  `player-death:loot-owner`, `player-death:cosmetic` (C).
  Payload types in `internal/game` (Phase 27 precedent).
- **C5 — Tick order (B owns the slot, A owns the effect):**
  skull timer runs inside `Player.Process` after area
  processing, before run-energy restore (the documented
  slot). The poison effect is a world-task-style per-entity
  tick driven from the combat bind / login resume — A
  chooses the mechanism that matches how Go runs per-entity
  tasks (ground `TaskManager` equivalents in Go first; do
  NOT invent a scheduler).
- **C6 — File ownership:** A: `internal/game/poison*.go`
  (+ `combat_factory.go` poison call sites only,
  `combat_hooks.go` poison fields only). B:
  `internal/game/skull*.go` (+ `player.go` process slot
  only, `session/bootstrap.go` live-icon read only,
  `cmd/server/*` skull wiring only). C:
  `internal/game/playerdeath.go`, `combat_death.go`
  (respawn path only), `internal/plugins/combat.go`
  (death gate families only), `cmd/server/*` death wiring
  only. Anything else: ask first.

## Out of scope (do not start, document as gaps)

Combat presentation — `NpcAnimationScanner`,
animation/graphic/projectile/sound producers (Phase 29);
Ancients/Lunar/Arceuus spellbooks; all quest work; graves
(plugin content — hooks only here); `deathComponents`
splitting (needs item-gameplay.json, decision pending —
degrade to no splitting); `keepsEverything` rights path
(Go has no rights field); Ironman loot-owner rule (no
Ironman mode in Go); cure items / antipoison content;
amulet-of-avarice / cape-of-skulls item skulls (plugin
content — core provides the timer/type only); the
far-WALK bug; the y=3521–3522 map wall; the real
production scrypt-hash check; perf work. If the scope
exceeds one phase, land a coherent verified subset
(poison + skulls + death teleport/item-loss first, grave/
interface remainder documented) and document the remainder
precisely as the next phase's brief.

## Verification (same pattern as phases 16–27)

`go build ./...`, `go vet ./...`, `gofmt -l .`,
`CGO_ENABLED=0 go build ./...`,
`go test ./... -p 1 -count=1` (3,959 tests before this phase —
must not drop), `go test -race`, plus a live smoke test
against the real cache (reuse `/home/hatch/smoke27/`,
seed cache at `/home/hatch/smoke27/seed-root/caches/`), 3
runs stable: a poisoned player ticking green damage and
being cured; a skull appearing after a Wilderness attack
between two players; a killed player teleporting home with
the correct kept/lost items (3 most valuable kept, rest on
the floor, bones present). Update this plan and
`PORT_STATUS.md`; no Drive upload (parent's job).

Status: COMPLETE (2026-11-17; verified by the coordinator;
parent verification + Drive archive still to come).

## Outcome

COMPLETE 2026-11-17. All three scope items landed as
coherent verified subsets; the grave/interface remainder
is documented in `PORT_STATUS.md` gap 15, not hidden.

### What landed

**1. Poison/venom** — `internal/game/poison_clock.go`
(494 lines), `internal/game/poison.go` (916),
`internal/game/poison_data_tables.go` (300 lines, 258
entries: 43 VERY_WEAK / 43 WEAK / 75 MILD / 32 EXTRA / 62
SUPER / 3 VENOM, script-extracted, zero unresolved/
duplicates), `poisonEntity` + hit-pipeline sites in
`combat_factory.go`, cure API (`CurePoison`), effect ticks
from `Combat.ProcessHitQueue` (cycle-guarded, no invented
scheduler), live Mobile poison fields +
`Combat.PoisonClock()` + `PoisonSaveState()`/
`RestorePoisonFromSave()`. 29 new top-level tests.
Coordinator integration: poison live state round-trips
through the save in `cmd/server/savewiring.go`
(`buildSnapshot` writes it, `applyEntryLiveState`
restores it via `RestorePoisonFromSave`, `PlayerSave.ts:656-660`
/ `:764-771` order; poison is NOT an OwnedState field —
skull precedent) + 2 coordinator tests
(`TestSnapshotWritesLivePoison`,
`TestSnapshotPoisonRoundTripsThroughRelogin`).

**2. Skulls** — `internal/game/skull.go` (318 lines): live
skull state (timer ticks, WHITE icon 0 / RED icon 1, icon
override, `HeadSkullIcon()`), `ApplySkull`/`IsSkulled`/
`ExtendSkull` wired in `InstallCombatHooks` (ApplySkull
always WHITE — the seam carries no type, `handleSkull`
never applies RED), `processSkullTimer` in the
`Player.Process` slot (after area, before run-energy
restore; all three PvP-combat hold terms have exact Go
forms), skull messages verbatim, RED deactivates Protect
Item. 15 tests in `skull_test.go` + 2 in
`cmd/server/savewiring_test.go`.

**3. Player death** — `respawnPlayer` rewritten to case-0
order in `internal/game/combat_death.go` (message +
jingle 90, `onShouldDropItemsOnDeath` gate,
`player:death-bones` + BONES 526 to ground, `dropItems`,
`resetAttributes`, kept items returned,
`emitPlayerDefeated` for ALL deaths, save "death",
`onPlayerDeath` handled chain, `player:died`,
`player:respawn` event else teleport to
`DefaultRespawnLocation` (3089, 3524, 0) from `world.json`,
save "respawn"; error path via defer/recover). New
`internal/game/playerdeath.go` (768 lines) + 622-line test
file (18 tests): `dropItems`/`getItemsToKeep`/
`getAmountToKeep`/`keepValue`/`slotLayout`/
`returnKeptItems`/`lootOwner` (red skull → 0, skulled → 0,
else 3 + Protect Item 1 with max Prayer ≥ 25 AND current >
0 AND PROTECT_ITEM active; `protectsCosmetics` false for
PvP kill or Wilderness, else varbit 10987 default on;
`keepValue` = GE, else high alch > 0, else floor(store ×
0.6)). Hub: new `OnShouldDropItemsOnDeath`,
`OnShouldKeepItemOnDeath`, `OnPlayerDefeated` + `Emit*`
methods; game-side `CombatHooks` fields
(`OnPlayerDefeated`, `ShouldDropItemsOnDeath`,
`ShouldKeepItemOnDeath`, `OnPlayerDeathItemDrop`,
`OnPlayerDeathDrops`, `RequestSave`); `OnPlayerDeath` now
returns `bool`; `CombatDeathEvent` gained `ItemsLost`/
`HasItemsLost`/`Handled`. Wiring: `InstallDeathWiring` in
`cmd/server/combatwiring.go`, called from `server.go` after
`installTradeHooks`. Coordinator integration: the death
attribute reset now clears poison
(`resetDeathAttributes` calls `PoisonClock().Stop()` +
`ResetPhase()` then `CurePoison`, `Player.ts:264-265`,
`274-275`) + poison staging in
`TestResetDeathAttributes`.

Hub: **53 of 108** hook families firable (48 → 53; the
five death families), 29 of 95 non-hook members, 12 of
191 Core — all unchanged except the five `On*`
registrations. Re-measured by script from
`PluginTypes.ts:999–1559` (TS 108 confirmed); Go has 53
`Api.On*` registrations (51 within the 108, plus 2 Go
extras `onItemOnObjectNamed`/`onObjectInteractionNamed`).

### Measured verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 647 `.go` files,
  17 packages.
- `go test ./... -p 1 -count=1`: **4,032 passing cases
  incl. subtests, 0 failures, all 17 packages** (3,959
  baseline + 73: workers' ~70 + coordinator's 2 new test
  functions).
- `go test ./... -p 1 -count=1 -race`: passes, all 17
  packages (no DATA RACE reported; the earlier
  intermittent no-race failures were not reproduced).
- Live smoke, 3 runs stable each, real rev-242 cache,
  temp roots outside the repo:
  - **Poison** (harness `/home/hatch/smoke27/`,
    coordinator-extended with a relogin leg,
    `run_poison.sh`): 5 checks each, all passing. A real
    cave crawler (406) poisoned alice ("You have been
    poisoned!" + orb varp 102 == 30); the disconnect save
    carried `poisonDamage` 30 with the clock snapshot
    (counter 0 — the counter is the protection dose, none
    taken — `ticksUntilPulse` 30); after relogin the orb
    showed again (> 0), proving the save round-trip. Run 3:
    alice died mid-run and was re-poisoned after respawn
    (orb 29/30, 0 at death, 30 again) — accidental live
    proof the death reset clears poison.
  - **Skull timer** (harness `/tmp/smoke28/`,
    `run_skull_timer.sh`): staged red skull 3000 → 2984
    over ~10s online (16 ticks, icon 1 survived); staged
    timer 5 → exactly 0, no underflow.
  - **Death** (harness `/home/hatch/smoke28/`): 17 checks
    each, all passing. "Oh dear, you are dead!" + one
    jingle (opcode 132), ground frames at (3094, 3524)
    `{526×1 bones, 200×1, 201×1, 995×1000}`, disconnect
    save at (3089, 3524, 0), HP 10/10, kept exactly
    `{1333 rune scimitar, 100 ranarr potion (unf), 101
    irit potion (unf)}`, kept ∪ dropped == the 6 seeded
    items.

### Honest limits — what the plan's verification list did
### NOT get live

- **"A poisoned player ... being cured"**: not observed
  live. No cure path exists in production (cure items are
  NOT STARTED); `CurePoison` is unit-tested only, and the
  death-reset clear is the only live cure-adjacent
  observation (run 3 above).
- **"A skull appearing after a Wilderness attack"**: not
  observed live — BLOCKED by a pre-existing content gap,
  not a phase-28 defect: `CanAttackByPolicy` denies PvP
  with no opinion, and the allow comes from the Wilderness
  plugin's `canAttack` (`Wilderness.plugin.js:469`),
  which is NOT ported. No live PvP attack can start at all
  until that content lands. The skull trigger is
  unit-covered; the timer leg is the live proof; the
  head-skull icon on the wire was not directly observed
  (needs player-update frame parsing).
- **Green damage ticks**: the orb decay 30 → 29 in run 3
  is live evidence a pulse landed; the tick damage values
  themselves are unit-proven, not asserted on the wire.

### Deviations and findings (all documented at the port
### sites or in `PORT_STATUS.md` gap 15)

- Blowpipe exclusion checks weapon id too (Go has no
  plugin ammunition). `Player.Process` orb sync via effect
  tick + apply/cure sends. `skullTicks` uses integer
  `seconds*5/3`. Skull mutations rebuild the cached
  appearance block via `onEquipmentChanged()` (Go caches
  the encoded block; flagging alone would leave a stale
  icon). `Damage as any` → `*HitDamage` type-assert,
  fail-closed; ammunition identity via item id, noted form
  33596 excluded. No deathComponents splitting (pending
  data-import decision). `keepsEverything` always false
  (no rights field). Barrows items treated as ordinary
  untradeables. No Ironman loot-owner rule. No grave
  (plugin content). Animation 836 recorded but never sent.
  `LastVenomDamage` is deliberately NOT cleared by the
  death reset (neither `CurePoison` nor TS clears it).
  Curing does NOT clear the immunity dose — TS `fromPlayer`
  writes the clock while `!finished()`, so a stored clock
  keeps counter 10 after cure; only a spent dose clears it
  (proven via `ReduceProtection(180)` in the coordinator's
  round-trip test). NPC `Hitpoints()` returns the
  definition value when the field is 0, so death/resume
  gates check `Hitpoints() <= 0 || Dying()`. NPC
  `poisonImmunityTimer` is never started by core.
- Harness findings: goblin Attack is option 2 (opcode 12);
  default goblin max_hit 1 deals ~0.05 HP/s, below HP
  regen — the death smoke stages a one-entry
  `monsters-complete.json` (Goblin max_hit 8, attack 99);
  goblin A (3092, 3520) is north of the y=3521 wall and
  unreachable (independent confirmation of the wall
  finding); engine clamps a seeded Hitpoints maxLevel of 1
  back to 10 on load. The coordinator's first poison smoke
  run failed its own clock-counter check (counter 0 with
  no antipoison taken) before the expectation was
  corrected to presence-of-snapshot.

### Docs updated

`PORT_STATUS.md` (header, verification numbers, live
smoke, inventory hub row, combat notes, gaps 1/7/9, new
gap 15, next-modules order) and
`CONTENT_MIGRATION_PLAN.md` (combat-completion progress
+ residual gaps). No Drive upload (parent's job).

### Next phase's brief (Phase 29 — combat presentation)

`NpcAnimationScanner`, animation/graphic producers
(incl. death animation 836), melee/magic projectile
producers, sound pickers — `StartCast` is a deliberate
no-op until then. After combat: content ports resume
piece by piece per `CONTENT_MIGRATION_PLAN.md` §3, with
Tyler's direction; the Wilderness `canAttack` content
gate is the blocker for any live PvP proof.
