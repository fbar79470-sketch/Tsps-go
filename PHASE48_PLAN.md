# Phase 48 Plan — Working doors

Tyler's order: "Add npc dialogs, working doors and latters" (2026-11-21 20:55 PST),
queued after Bank (Phase 46, done), Magic (Phase 45, done), NPC dialogs (Phase 47,
done). Doors are next; ladders follow as Phase 49. This is Phase 48.

## §1 Survey (done 2026-11-22, two workers + coordinator read-through + live-cache probe)

### §1.1 What upstream does

`plugins/objects/Doors.plugin.js` (2,144 lines) is the whole door mechanism. One
entry in the migration plan's 577 (`objects/` has 21 entries). It registers one
`onObjectInteraction` per name in `DOOR_NAMES` (`:2134-2136`, `{Open, Close,
Release} → toggleDoor` — Release covers gates like 60760/60763), plus
`onRegionLoaded` (re-apply open doors, `:2139`), `onServerStartup`
(route registration, `:2138`), and two custom events (`door:open` `:2140`,
`door:state` `:2141`). `WallDoor.Doors.js` (14 lines) is just `crossingFor`:
the approach/destination tiles across a straight wall door by face.

**The mechanism, in one paragraph.** Doors are identified from the cache, not
from a hand list: at startup `buildDoorCatalog` (`:1056-1180`) scans every loc
definition (~63k) and pairs each closed door (name in `DOOR_NAMES`, offers
"Open") with its open leaf — classic `closedId+1` with the same name and a
"Close"-only action, or a gameval-named open leaf (`<name>_open`, or
`closed`→`open`, same models), or the `id+2` skipped-actionless and `id+3`
interleaved rules, plus explicit override tables for the pairs no heuristic
finds. Opening a door swaps locs through the object manager (deregister the
closed leaf, register the open leaf on the adjacent tile, turned a quarter);
closing swaps back. Open state is tracked server-side in `OPEN_OBJECT_STATES`
(anchor key → closed/current snapshots) and reverts on its own after
**500 ticks** (`DOOR_AUTO_CLOSE_TICKS`, `:9-11` — 300 s; cross-checked against
rsmod/OpenRune-Server and a from-scratch OSRS-parity server). Sounds are the
cache sounds: door open 62 / close 60, gate open 67 / close 66
(`doorSound`, `:1233`). Clipping follows the swap: the closed loc's clip is
lifted when it is deregistered and the open leaf's clip is laid when it is
registered — there is no separate clip layer. `toggleDoor` (`:2033-2044`)
emits `door:toggle` FIRST; if any listener sets `request.handled = true` the
generic swing never runs, and content that passed its own check re-opens the
door by emitting `door:open` (`requestDoorOpen`, `:2046-2075`).

**The variants** (all in the same file):

- **Single doors**: closed→open pair; open leaf steps one tile along the
  closed face (`COORD_OFFSETS`, `:907`) and turns +1 (close turns −1 back);
  in-place doors (`IN_PLACE_DOOR_IDS`, 3: curtains, two trapdoors) replace
  the model on the same tile; own-tile doors (`OWN_TILE_DOOR_IDS`, 7 + every
  `*_reverse` name) turn on the closed tile; self-opening doors
  (`SELF_OPENING_DOOR_IDS`, 37 entries / 36 unique — `O.DOOR_67` listed
  twice) keep their id and swing in place, tracked by state only.
- **Double doors**: two leaves swing together (left leaf turns to
  `LEFT_LEAF_OPEN_FACES`, right to `RIGHT_LEAF_OPEN_FACES`, `:923-924`).
  Found three ways: `DOUBLE_DOOR_ID_FAMILIES` (26 families, `:277-332`),
  `SPECIAL_DOUBLE_DOOR_PAIRS` (36 entries / 35 unique — `[GATE_232,
  GATE_233]` twice, `:333-375`) with `SPECIAL_DOUBLE_DOOR_OPEN_IDS_BY_CLOSED_ID`,
  and same-model consecutive "Open"-only leaves detected by the scan
  (`addSameModelDoubleDoor`, mirrored leaf names via `isMirroredLeafName`).
  A leaf only acts as a double door where both leaves stand side by side;
  a lone one stays a single door.
- **Wooden gates** (`WOODEN_GATES`, 14 `woodenGate(` calls, `:247-266`): two
  locs (hinge + extension panels) pivot together 90° around the hinge post
  (`gateHingeTransform`, `:1318-1338`, ported 1:1 from the original elvarg
  gate state manager); opening/closing moves BOTH pieces.
- **Composite openings** (`COMPOSITE_DOOR_OPENINGS`, 3): one loc opens into
  two leaves (Tree Gnome Stronghold main gate: one 5×2 loc → two nameless
  2×2 leaves).
- **Pass-through gates** (`PASS_THROUGH_GATES`, 29 entries, `:427-860`;
  `PASS_THROUGH_FAMILIES`, 15 entries, `:871-889`): gates that let the
  player through and shut behind them, never staying open. Open swaps the
  closed leaves for an inviswall (38848) on the doorway + open leaves on
  the tiles past the edge, steps the player across (walkOver: one step
  ignoring clipping, flag cleared within 2 ticks; carry: forced movement;
  diagonal: two-tile forced move; thick gates: carried across the span),
  and shuts 2 ticks later (5 for the West Ardougne wall gate). Sounds are
  per-gate (`openSoundId`, mostly 62/67/69/71/81).
- **Route**: `routeWallDoor` (`onObjectRoute`, `:2097-2116`) — a closed
  straight wall door is used across its face, never from either end of the
  wall (`blockAccessFlags |= 0x5`); diagonal (type 9) doors record the
  approach side (`DIAGONAL_DOOR_APPROACHES`) so a swinging leaf never lands
  on the opener.
- **Dorgesh-Kaan doors** (`DORGESH_CITY_DOOR_IDS`, 2): explicitly NOT swung
  here — they belong to LocTeleports (`toggleDoor` returns false, `:2033`).

**Table counts** (grep/Python, worker-verified, not estimates):
`DOOR_NAMES` 36; `SELF_OPENING_DOOR_IDS` 37 entries / 36 unique;
`SINGLE_DOOR_OPEN_IDS` 21; `WOODEN_GATES` 14; `DOUBLE_DOOR_ID_FAMILIES` 26;
`SPECIAL_DOUBLE_DOOR_PAIRS` 36 / 35 unique; `PASS_THROUGH_GATES` 29;
`PASS_THROUGH_FAMILIES` 15; `IN_PLACE_DOOR_IDS` 3; `OWN_TILE_DOOR_IDS` 7;
`COMPOSITE_DOOR_OPENINGS` 3; `DORGESH_CITY_DOOR_IDS` 2.

### §1.2 The consumers: every lock lives outside Doors

74 files reference `door:toggle` / `door:open` — **52 quest files, 11
area/guild files, 10 minigame/other files** (77 `onCustomEvent("door:toggle")`
lines in 71 files; 18 `door:open` emits in 13 files). **Doors.plugin.js
itself implements no lock, pick-lock, or Unlock logic.** All gating is
consumer-side, in 5 mechanisms: (1) generic swing with an external
`door:toggle` guard (the common case — quest stage, key held, skill level);
(2) key/item used ON the door → emit `door:open` (Taverley keys, Ghosts Ahoy
bone key); (3) "Pick-lock" — 8 consumer implementations, each with its own
level/item/chance (Ardougne Castle 61 Thieving, Yanille 82 + lockpick,
Forthos 57, Shantay Pass lockpick chance, Pyramid Plunder, Construction
dungeon, Dorgeshuun storeroom 23 Thieving, Monkey Madness state-based);
(4) "Unlock" option — 4 door-relevant registrations (CastleWars side doors,
Sins of the Father mausoleum, Troll Stronghold cell/prison, Grotesque
Guardians roof); (5) toll/payment (Al-Kharid 10 gp — a separate plugin that
intercepts `door:toggle` and always handles it; Tai Bwo Wannai grove
Quick-Pay 100).

Guild/area gates (all consumer-side, none in this phase): Champions Guild 32
Quest Points; Mining Guild 60 Mining (+ members for inner doors); Ranging
Guild 40 Ranged; Farming Guild 45 Farming (from the south only); Fishing
Guild 68 Fishing (already a deferred stub in Go, `internal/content/fishing/guild.go`
— door id 422 at (2611,3394,0)); Woodcutting Guild 60 Woodcutting; Grand
Tree door has NO requirement (swings in place, forced walk across, shuts
after 5 ticks); McGrubors Wood main gate is ALWAYS locked; Tutorial Island
doors are tutorial-step gated.

Doors NOT covered by Doors.plugin.js (own `Open` elsewhere, tile/id-scoped):
BrassKeyDoor (Edgeville Dungeon — brass key or keyring held, key kept, custom
inviswall + open leaf 1539, shuts 2 ticks later); Stronghold of Security
per-floor doors (pairs with a gap; security-question doors); Slayer Tower,
Warriors' Guild, Al-Kharid gate, CastleWars, Construction dungeon doors,
trapdoors that Climb-down (LocTeleports `loc-swaps.json` territory), and
portal/teleport-style doors. None of these are in this phase.

### §1.3 What the Go side already has (survey worker, file:line cited)

Every primitive the mechanism needs is ported and live; the door mechanism
itself is MISSING. No new hub family is needed.

- **Swap + sync: AVAILABLE.** `ObjectRegistry.Register/Deregister(obj,
  playerUpdate bool)` (`internal/game/objects.go:1000/:1093`); with
  `playerUpdate=true` they call `perform()` → `mapAdd`/`mapRemove`, the
  only paths touching clipping. Deltas drain once per tick into
  `LOC_ADD_CHANGE` (opcode 134) / `LOC_DEL` (135)
  (`internal/session/objects.go`), so a runtime swap is visible to nearby
  players on the next tick flush. `ReplaceInPlace` (`:1137`) is same
  tile + same shape slot only, emits one spawn delta and no removal delta,
  with an explicit missed-removal gap for away players — a door that swings
  to a different tile/face must use Deregister + Register, not it.
  Removed-map-object re-apply on region load is live
  (`importRegion`, `OnRegionChange` sends removals before additions).
- **Clipping: AVAILABLE.** Single clip layer on the `RegionManager` grid;
  `AddObjectClipping`/`RemoveObjectClipping` derive clip from the
  definition at add/remove time (`internal/game/collision.go:40`,
  `regions.go:1159/:1189`). There is no separate clip layer to update —
  swapping an object automatically lifts the old clip and lays the new
  one. Trap: `addObjectClipping` returns early when no definition exists
  for the id, so an unknown open-door id would silently clip nothing.
- **Route + click: AVAILABLE.** `OnObjectInteractionNamed(name, actions)`
  / `OnObjectClick(ids, clickType, …)` (`internal/plugins/objects.go`);
  `OnObjectRoute` with settable `Destination` and `BlockAccessFlags`,
  honoured by the route finder (`movement_queue.go:1904`,
  `routefinding.go:139,222,273`). Click flow: opcode → `routeObjectAction`
  → `WalkToObject` → still-stands re-check on arrival → emit
  (`internal/listeners/objectaction.go`). Known gap: arrival does not call
  `Player.SetPositionToFace`.
- **Sound: AVAILABLE.** `SoundDoorOpen = (62,1,0,0)`, `SoundDoorClose =
  (60,1,0,0)`, `SoundGateOpen = (67,1,0,0)`, `SoundGateClose =
  (66,1,0,0)` (`internal/game/sounds.go:209-210`); player-only
  `SendSound` and area `PlayAreaSoundAt` both live.
- **Timer: AVAILABLE.** `TaskManager.Schedule(NewTask(delay, …))`
  (`internal/game/task.go:103,216`); tick-goroutine only; a 500-tick
  auto-close is one `NewTask(500, …)` with a comparable key for
  `CancelTasks`/`HasActiveTask`. (Phase 45 fixed the startup-hook race in
  this area; scheduling from content hooks is the established pattern.)
- **Catalog scan: PARTIAL.** `CacheDefinitions.GetObject(id) *LocType`
  (`internal/cache/facade.go:391`), `GetBaseCounts().Objects`
  (`:423`), `GetLocId(name)` (`:761`), `GetLocNames()` (`:820`);
  `LocType` has every field the scan needs (`Name`, `Actions`, `Models`,
  `Types`, `SizeX/SizeY`, `ClipType`, `ClipMask`, `BlocksProjectile`,
  `Transforms`). No bulk iterator (loop `0..count-1` calling `GetObject`);
  the type cache is bounded at 4,096 entries, so a ~63k scan is one full
  decode pass at startup (acceptable once; never per click). No pairing
  code exists.
- **Walk-through step: MISSING.** No `ObstacleRunner`; the ditch's
  `ForceMovementTask` pattern (Phase 40) is the borrowable precedent.
  `door:toggle` / `door:open` custom events do not exist; the generic
  `Hub.EmitCustomEvent(name, payload)` exists (`internal/plugins/events.go:47`).
  Only existing door mention in content: the Fishing Guild stub comment.

### §1.4 Live-cache probe (coordinator, real rev-242 cache, temp probe — deleted after use)

Probe `cmd/doorcatalogprobe` (temporary, removed after the run) scanned all
object defs with the TS pairing rules in simplified form (classic id+1,
gameval-named leaf, id+2 skipped-actionless, same-model double-door leaves,
self-opening set, wooden-gate skip, 21 single-door overrides):

- Total object defs: **63,334**. Door-named with an "Open" action: **873**.
- Paired by the simplified heuristics: **575** closed→open entries —
  100 classic id+1 Close-only pairs, 173 gameval-named open leaves, 130
  same-model double-door leaf pairs (upper bound: the probe dropped TS's
  mirrored-name + named-open-leaf requirements), 36 self-opening,
  12 newly paired by the single-door overrides; 26 wooden-gate panels
  skipped from generic pairing.
- **408** door-named Open locs unpaired by the simplified heuristics: 191
  have a same-name Open neighbour (double-door leaves — TS's family
  tables + mirrored-leaf rule handle these); **217** need the full
  `DOUBLE_DOOR_ID_FAMILIES` (26) + mirrored-leaf detection + id+3
  interleave rule. The Go catalog transcribes those tables; its unit test
  asserts the final counts (the probe numbers above are the approximate
  baseline, not the acceptance values).

Placed doors (decoded region placements, same probe) — the live-test
targets, all real map locs:

- **Lumbridge** (region 12850): single `1535 "Door" → 1536` (many; e.g.
  (3225,3240,0) face 2, (3226,3214,0)); `1543 → 1544` (e.g. (3207,3214,1));
  `1540 → 1541` (3238,3210,0); kitchen door `12348 → 1544` at
  (3207,3217,0) face 1 (the TS-cited `DOOR_302` case); double
  `1521 + 1524 "Large door"` at (3244,3216,0) + (3243,3216,0) face 3,
  opening to 1522/1525; self-opening double `12349 + 12350` at
  (3213,3221,0) + (3213,3222,0) face 0; wooden gate `12986 + 12987
  "Gate"` at (3213,3261,0) + (3213,3262,0) face 0; `56376 → 56377` at
  (3230,3235,0) face 1.
- **Varrock** (region 12853): `11775 "Door" → 11774` (inverted pair, many;
  e.g. (3205,3432,0)); `11780 → 11778` (e.g. (3235,3404,0)); unpaired
  `50048` at (3259,3400,0) and gate `24536` at (3261,3446,0).
- **Edgeville** (region 12342): double `1521 + 1524` at (3101,3509,0) +
  (3101,3510,0) face 0; wooden gate `1558 + 1560` at (3080,3501,0) +
  (3079,3501,0) face 3; trapdoor `1579 → 1581` type 22 at (3097,3468,0).
- **Falador** (region 11828): `24057 → 24058` (many); `24059 "Castle
  door"` unpaired (3 placements) vs paired `24060 → 24061`, `24062 →
  24063`, `24064 → 24065`; `46253 → 46254` at (2982,3371,0).

## §2 Scope (sized honestly)

**This phase is the door MECHANISM plus a verified slice.** The full upstream
surface is 2,144 lines plus 74 consumer files; the consumers are quests,
guilds, and minigames that do not exist in Go yet, so they cannot be ported
now. What lands:

1. **Catalog** (`internal/content/doors/catalog.go`): startup scan of all
   object defs with the TS pairing rules + transcribed exception tables
   (`DOOR_NAMES`, self-opening, single-door overrides, wooden gates,
   double-door families + special pairs, pass-through gates + families,
   in-place / own-tile / composite / Dorgesh sets). TS `O.X` constants
   resolve to numeric ids by parsing `ObjectIdentifiers.ts` (read-only)
   at authoring time and transcribing the values into Go with file:line
   citations — the Phase 33 transcription precedent; the file itself is
   NOT copied. Unit test asserts the catalog counts against the real
   cache (fixture-backed where the scan is expensive).
2. **Toggle engine** (`internal/content/doors/doors.go`): `Open`/`Close`/
   `Release` via `OnObjectInteractionNamed` for all 36 `DOOR_NAMES`;
   single, double, wooden-gate, composite, self-opening, in-place, and
   own-tile swings; open-state tracking (anchor → closed/current
   snapshots) with the 500-tick auto-close; door/gate sounds; wall-door
   route handling via `OnObjectRoute` (`blockAccessFlags |= 0x5` for closed
   straight wall doors; diagonal approach-side recording for type 9);
   `door:toggle` emitted first (a `door:open` listener re-opens after a
   guard passes) and `door:open` / `door:state` custom events. Walk-through:
   after an ordinary door opens, the player walks through with ordinary
   movement (the closed clip is gone); pass-through gates use the
   walkOver / carry / diagonal step-through from the TS code, borrowing
   the Phase 40 `ForceMovementTask` pattern where forced movement is
   required.
3. **Pass-through gates**: mechanism ported with the full 29-entry +
   15-family tables transcribed — BUT with no guards: the `door:toggle`
   consumers that lock quest/area pass-through gates do not exist in Go,
   so those gates swing for anyone until their guard plugins land. This
   is a documented residual (gap 31), not a silent parity claim.
4. **WallDoor `crossingFor`** ported as a small pure function with unit
   tests (the wall-door approach/destination geometry).

**Live-verified slice** (Lumbridge, all real map locs from §1.4): single
door `1535 → 1536` at (3225,3240,0); double door `1521 + 1524` at
(3244,3216,0)/(3243,3216,0); wooden gate `12986 + 12987` at
(3213,3261,0)/(3213,3262,0); close-and-reblock on the single door;
walk-through to the other side on each. The 500-tick auto-close is
unit-covered only (5 minutes is too long for a live leg); pass-through
gates are unit-covered only (their placements are quest-gated in TS and
the guards are not ported — a live leg would prove the wrong thing).

**NOT in this phase (residuals, gap 31):** all 74 `door:toggle` consumers
(52 quest gates, 11 guild/area gates, minigames); pick-lock (8
implementations) and Unlock (4 registrations); BrassKeyDoor; Stronghold
of Security doors; Al-Kharid gate; Construction dungeon doors; trapdoors
that Climb-down (LocTeleports territory — ladders are Phase 49);
diagonal-door live observation; the Fishing Guild door (still deferred,
needs the guild + `door:toggle` + walk-through — the mechanism now
exists, the guild gate does not).

## §3 Port plan

Worker split (file ownership is disjoint — do not cross it):

- **Worker A — catalog** (`internal/content/doors/catalog.go` +
  `catalog_test.go`): the scan + pairing + transcribed exception tables.
  Resolves TS `O.X` constants via a script over the read-only
  `ObjectIdentifiers.ts` (do not copy the file; transcribe the values
  the tables need, each with its TS file:line citation). The catalog is a
  pure function of the cache facades (`GetObject`, `GetBaseCounts`,
  `GetLocNames`, `GetLocId`) so it is unit-testable with fixture defs;
  one test runs it against the real rev-242 cache and asserts the counts
  (§1.4 baseline: 873 door-named Open, 575 paired by simplified rules —
  final numbers will differ once the family tables land; record the real
  numbers in §8).
- **Worker B — toggle engine** (`internal/content/doors/doors.go` +
  `doors_test.go`, `passthrough.go` + `passthrough_test.go`,
  `walldoor.go` + `walldoor_test.go`): the `Plugin` value + `init()` /
  `plugins.Register` (pattern: `internal/content/wildernessditch`), the
  open/close state machine, double/wooden/composite/self-opening/in-place
  handling, auto-close via the world's `TaskManager`, sounds, route
  handling, `door:toggle` / `door:open` / `door:state` custom events,
  the pass-through step-through (walkOver / carry / diagonal), and
  `crossingFor`. Depends on A's catalog API — the seam is fixed below;
  B builds against a stub if A has not landed, and the coordinator
  integrates.
- **Worker C — live harness** (`/home/hatch/smoke48/`, copy the smoke47
  structure): legs single / double / wooden-gate / close-reblock /
  walk-through. Runs after A+B integrate. Legs run singly if a full pass
  exceeds the background-exec wall clock.

Shared contracts (seam between A and B — do not change without telling the
coordinator):

```go
// catalog.go
type Catalog struct {
    ClosedToOpen map[int]int // closed id -> open id (self-opening: id -> id)
    OpenToClosed map[int]int // open id -> closed id (shared leaves: first wins)
}
func BuildCatalog(defs CatalogDefs) (*Catalog, error) // CatalogDefs: GetObject(id), ObjectCount(), LocName(id), LocID(name)
func (c *Catalog) OpenID(closedID int) (int, bool)
func (c *Catalog) ClosedID(openID int) (int, bool)
func (c *Catalog) IsSelfOpening(id int) bool
func (c *Catalog) WoodenGate(id int) (*WoodenGate, bool)   // hinge/extension, closed/opened ids
func (c *Catalog) DoubleDoorFamily(id int) ([]int, bool)   // family ids containing id
func (c *Catalog) PassThroughGate(id, x, y, z int) (*PassThroughGate, bool)
func (c *Catalog) PassThroughFamily(id int) (*PassThroughFamily, bool)
```

Wiring: `cmd/server/doorswiring.go` installs the catalog at boot (from the
loaded cache defs; NoCache mode → empty catalog, doors stay shut, boot
still succeeds) and the blank import in `cmd/server/pluginwiring.go`
(alphabetical, first position among content imports — `doors` sorts before
`fishing`). No new hub family: `OnObjectInteractionNamed`,
`OnObjectRoute`, and the generic custom-event surface cover everything.

Data-file decision: **none needed.** The catalog is derived from the cache
at startup, exactly as upstream; no JSON import, no Tyler approval needed.

## §4 Live verification (harness /home/hatch/smoke48/)

Real server binary + real rev-242 cache (seed cache at
`/home/hatch/smoke27/seed-root/caches/osrs-242_2026-11-03`, symlinked as
in prior phases). Spawn at Lumbridge (3222,3218,0). Legs (each: click the
door, observe the `LOC_ADD_CHANGE`/`LOC_DEL` frames + the player's tile
after walking, then the close):

- **single** — Open `1535` at (3225,3240,0) face 2 → leaf `1536` on the
  wire; walk through to the far side; Close `1536` → `1535` back; walk
  into the doorway from the far side is refused (clip restored).
- **double** — Open `1521` at (3244,3216,0) → both leaves swap
  (`1522` + `1525`); walk through; Close → both back.
- **wooden-gate** — Open `12986` at (3213,3261,0) → both panels swing
  (`12987` partner moves too); walk through; Close → both back.
- **kitchen** — Open `12348` at (3207,3217,0) → `1544` (the
  single-door-override case); walk through.
- **close-reblock** is folded into single (the refused walk after Close).
- Checks per leg: open leaf id(s) on the wire, player tile after the
  walk-through, closed leaf id(s) after Close, refusal chat/position
  after Close. Record exact values in §8.

## §5 Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go build
  ./...` — all clean.
- Full suite `-p 1 -count=1 -v` — report top-level + subtest counts,
  packages, failures, skips (baseline: 5,134 tests, 30 packages from
  Phase 47).
- `go test -race ./...` — the known pre-existing `TestEngineTicksAtRate`
  5 ms timing flake in `internal/game`, the known §20 `cmd/server`
  race-load timing issues, and the known
  `TestReloginRoundTripOverWebSocket` disconnect-save timeout are NOT
  data races; if the race run fails only on one of those with no DATA
  RACE warning, re-run that package once before reporting. Any DATA RACE
  warning is a real finding.
- Live legs per §4, in the fresh temp root `/home/hatch/smoke48/`.
- Docs: this plan (§8 filled in), PORT_STATUS.md (header, "What was
  verified" entry, content row — COUNT it, currently 23 of 577 — new gap
  31 residuals).

## §6 Residuals (gap 31)

- All 74 `door:toggle` consumers NOT STARTED (52 quest gates, 11
  guild/area gates, 10 minigame/other). Consequence: quest-locked
  pass-through gates swing for anyone until their guards land.
- Pick-lock (8 implementations) + Unlock (4 registrations) NOT STARTED.
- BrassKeyDoor, Stronghold of Security doors, Al-Kharid gate,
  Construction dungeon doors NOT STARTED (own `Open` handlers elsewhere).
- Trapdoors that Climb-down NOT STARTED (LocTeleports; ladders Phase 49).
- Diagonal (type 9) doors: geometry ported + unit-tested, live
  observation open.
- 500-tick auto-close: unit-covered only, not observed live.
- Pass-through gates: mechanism + tables ported, live observation open
  (placements are quest-gated in TS; a live leg would prove the wrong thing).
- Fishing Guild door still deferred (needs the guild gate; mechanism now
  exists).
- LIVE-FOUND ENGINE DEFECT (not doors-specific): a Close clicked in the
  first tick after login silently loses the open leaf's LOC_DEL
  (reproduced deterministically live; `FlushTick` drains deltas before
  the first tick's snapshot and before `hasReplayedScene` is set — see
  §8). Server-side state stays correct; real-client impact assessed as
  likely nil but unproven on the real client. Needs a real-client check
  or a flush-ordering fix.
- No `onRegionLoaded` re-apply (no region-loaded hook family in Go);
  wall-door route registered directly in `Register`; auto-close sound
  goes out as an area sound at the closed tile.
- 217 door-named Open locs unpaired by the simplified probe heuristics:
  resolved by the transcribed family tables + interleave rule in the
  catalog; final catalog counts asserted by test (see §8).

## §7 Data files needing Tyler's import approval

None. The catalog is derived from the cache at startup; no JSON import.

## §8 Results

Done 2026-11-22. The door mechanism is ported and live-verified on
four Lumbridge legs; one live-found wiring bug was fixed in-phase, and
one genuine engine defect was found live and carried forward as a
residual (gap 31).

### What was ported

- `internal/content/doors/` (new package, 31st): `catalog.go` +
  `tables.go` (startup cache scan + pairing; 63,334 object defs,
  873 door-named Open, 556 closed→open pairs, 259 open→closed
  reverse, 280 self-opening pairs, 317 unpaired door-named Open;
  all counts pinned by `TestRealCacheCatalog`), `doors.go` (toggle
  engine: Dorgesh refusal → `door:toggle` emit → pass-through →
  composite → wooden gate → double door → single mapped door;
  in-memory open state + 500-tick auto-close; sounds door 62/60,
  gate 67/66), `passthrough.go` (pass-through gates: swap leaves,
  diagonal/thick/carry/walkOver step-through, 2-tick close, 5 for
  West Ardougne), `walldoor.go` (`crossingFor` pure function +
  `routeWallDoor` route flags `0x5`).
- `cmd/server/doorswiring.go` + blank import in
  `cmd/server/pluginwiring.go` (catalog installed before hub Load;
  `pluginwiring_test.go` pins the loaded-plugin list — "Doors" loads
  4th, 19 entries). No new hub family: the existing
  `OnObjectInteractionNamed` / `OnObjectRoute` families cover it
  (hub stays 63/108 firable, 67 `Api.On*` methods).
- The `door:toggle` / `door:open` / `door:state` custom events exist
  (`DoorToggleEvent`, `DoorOpenEvent`, `DoorStateEvent`); no
  consumer is ported yet (gap 31).

### Live-found bug fixed in-phase (run 1 → run 2)

Run 1 of the live legs (harness `/home/hatch/smoke48/`, real server
binary + real rev-242 cache): all four legs failed — clicks played
the open sound (62) but no leaf ever swapped and no player ever
walked through. Root cause, verified in code: `engineFromAPI`
captured `api.GetObjectManager()` and `api.GetTaskManager()` at
Register time, but `Register` runs inside `newPluginHub`
(`cmd/server/server.go:185`) while `hub.SetObjectManager` runs at
:411 and the TaskManager arrives with `InstallSkillHooks` at :469 —
both were nil live, so every swap nil-guarded away while the sound
still played (the sounds service comes from `api.Core()`, available
earlier). The gate leg was silent throughout: the partner lookup
bailed on nil objects before the sound. Fix (the Phase 32 hub
contract — surfaces installed after Load must be resolved lazily,
never captured in Register): the engine now holds
`resolveObjects`/`resolveTasks` closures and resolves both lazily
per use (`objectManager()`/`taskManager()` in `doors.go`), caching
only non-nil results; all 20 direct `e.objects`/`e.tasks` uses in
`doors.go` + `passthrough.go` converted. Regression test
`TestLazySurfaceResolution` (resolvers answering nil until a
simulated install, then the real surfaces — swap + auto-close both
fire). Reordering boot was not an option: the object manager needs
`world.GameObjects()`, which does not exist until after hub load.

Run 1 also reported a "snapshot gap" (zero LOC frames in the login
snapshot); that was a harness-premise misdiagnosis, corrected before
run 2: upstream `ObjectManager.onRegionChange` SKIPS base-map objects
(`ObjectManager.ts:230-232`, `matchesBaseMapObject`) — the client
knows static map locs from its own cache, so the snapshot legitimately
carries none. Swaps are verified via post-click LOC_ADD_CHANGE (134)
/ LOC_DEL (135) deltas only.

### Live verification (run 2, harness `/home/hatch/smoke48/`, fresh
### server binary + fresh temp root per leg, port 47109)

All four legs PASS on the stated criteria (open leaf on the wire,
walk-through tile, closed leaf after Close, refusal):

- single — Open 1535 at (3225,3240,0) face 2: ADD 1536 at
  (3226,3240,0) rot 3 + DEL at (3225,3240) rot 2, sound 62; walk to
  (3226,3240); Close 1536: ADD 1535 at (3225,3240) rot 2, sound 60;
  walk into the doorway from the far side refused (player stayed
  (3226,3240)). S0 control passed: the closed door refused the
  crossing from the far side.
- double — Open 1521 at (3244,3216,0): ADD 1522 at (3244,3215) rot 2
  + ADD 1525 at (3243,3215) rot 0, DELs at (3244,3216) + (3243,3216)
  rot 3, sound 62; walk to (3244,3215); Close: ADD 1521 at
  (3244,3216) rot 3 + ADD 1524 at (3243,3216) rot 3, sound 60.
- wooden-gate — Open 12986 at (3213,3261,0): ADD 12988 at (3212,3261)
  rot 3 + ADD 12989 at (3211,3261) rot 3, DELs at (3213,3261) +
  (3213,3262) rot 0, sound 67 (gate-open); walk to (3212,3261);
  Close: ADD 12986 at (3213,3261) rot 0 + ADD 12987 at (3213,3262)
  rot 0, sound 66 (gate-close).
- kitchen — Open 12348 at (3207,3217,0) face 1: ADD **1544** at
  (3207,3218) rot 2 (the override case — not id+1) + DEL at
  (3207,3217) rot 1, sound 62; walk to (3207,3217). Open-only leg.
- doubleclick — click 1 on 1535: sound 62, ADD 1536 + DEL 1535. Click
  2 on 1535 at (3225,3240): silence, no frames — correct: after a
  real open, 1535 no longer exists there, so the click targets a
  non-existent loc and is ignored. (The run-1 prediction of "deltas
  back + sound 60" only held because the nil-manager bug left 1535
  in place; the harness check asserting it is stale, the behavior
  is right. The real close-click is on the open leaf 1536, proven
  by the single leg.)

### Genuine defect found live, carried forward (gap 31)

A Close clicked in the first tick after login silently loses the
open leaf's LOC_DEL (reproduced deterministically: single S3 twice,
double S3 once — ADD 1535 arrives, sound 60, dels = 0). Two
diagnostics prove the trigger is the immediate post-login click:
`closewatch` (close right after open, same session) and
`closewatch2` (fresh login, 3 s settle before Close) both deliver
DEL + ADD with sound 60. Mechanism, verified in code:
`FlushTick` drains object deltas (`DrainDeltas`,
`internal/session/flush.go:450`) BEFORE the first tick's
`OnRegionChange` snapshot and before `hasReplayedScene` is set
(`flush.go:606, 691-697`); on that first tick `IsTileInScene` is
false for every tile (`internal/session/objects.go:295`), so the
removal delta is dropped (`objects.go:492-496`) and
`recordMissedRemoval` no-ops on the fresh session's empty shown
map — while the same tick's snapshot re-sends the closed leaf ADD
(the restored copy enters the world list via `addToWorld`,
`internal/game/objects.go:1069-1072`). Server-side state is correct
throughout (collision/refusal checks pass). Real-client impact is
assessed as likely nil — the real client rebuilds its scene from its
own cache on login, which holds the closed leaf, not the open one —
but that is an assessment, not a live observation; the ghost is
visible to a frame-tracking protocol client that carried the open
leaf across sessions. It is a pre-existing engine ordering issue
(any first-tick removal delta would drop, not doors-specific), so it
is a residual, not a Phase 48 fix; it needs either a real-client
check or a flush-ordering fix.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`: clean.
  `CGO_ENABLED=0 go build ./...`: clean. 877 `.go` files, 31
  packages (new `internal/content/doors`; no new hub family —
  63/108 firable, unchanged).
- `go test ./... -p 1 -count=1 -v`: all 31 packages pass (5,182
  passing test cases: 3,963 top-level + 1,219 subtests, 0 failures,
  0 skips — 5,134 from phase 47 + 48 new, all in
  `internal/content/doors`: 16 catalog, 22 engine (including the
  lazy-resolution regression), 9 pass-through, 1 walldoor
  `TestCrossingFor`).
- `go test -race ./...`: clean on the first full run (exit 0, 0
  DATA RACE warnings, 31/31 packages ok). No known-flake re-runs
  needed this phase.

### Unit-covered only (not observed live)

500-tick auto-close (5 minutes too long for a live leg);
pass-through gates (placements are quest-gated in TS — a live leg
would prove the wrong thing); diagonal (type 9) doors; the
`door:toggle`/`door:open`/`door:state` consumer surface (no
consumers exist yet). Logs:
`/home/hatch/smoke48/smoke48-{single,double,gate,kitchen,doubleclick,closewatch,closewatch2}-run2.log`
(+ `smoke48-single-run2b.log`, the single-leg repeat) with matching
`-server.log` files; run-1 logs in the same directory.
