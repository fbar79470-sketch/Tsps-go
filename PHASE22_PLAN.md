# Phase 22 plan — Graceful Outfit + Worn Weights (first content port)

Status: COMPLETED (coordinator-verified, 2026-11-16; live smoke test 3 runs stable).

## Scope

Migration plan §3 item 2, the first content port now that all 11 engine
milestones are DONE: port `server/plugins/items/GracefulOutfit.plugin.js`
(137 lines) and `server/plugins/items/WornWeights.plugin.js` (38 lines) to
Go content packages, building the ONE engine seam they need (plan §3 #2):
the `EquipmentEffects` run-energy modifier registry plus an
`equipment:weights` emit site in the weight calculation. Porting rules are
migration plan §4 (read before implementing): faithful port with file:line
citations, fixtures-only unit tests with pure logic factored out,
live verification before claiming behaviour, honest gaps in
PORT_STATUS.md, no copied `*.plugin.js` files, hub registration via
`internal/plugins`, one Go package per piece under `internal/content/`
with one blank import in `cmd/server/pluginwiring.go`.

Out of scope: every other content port (Lumbridge Guide, Seeking Arrows,
quests, skills), skillcape content, the far-WALK bug, the y=3521–3522 map
wall, the real production scrypt-hash check, perf work. No scope expansion
without TS grounding.

## Upstream grounding (verified by the coordinator, not trusted from summaries)

### GracefulOutfit.plugin.js (all line numbers are that file)

- `PIECES` (lines 9–16): six slots with name prefixes, restore percents,
  and Wiki weights in kg: slot 0 "Graceful hood" 3% −3 kg, slot 4
  "Graceful top" 4% −5 kg, slot 7 "Graceful legs" 4% −6 kg, slot 9
  "Graceful gloves" 3% −3 kg, slot 10 "Graceful boots" 3% −4 kg, slot 1
  "Graceful cape" 3% −4 kg. `SET_BONUS = 0.10` (line 17).
- `capeIsSubstitute` (lines 21–25): emits `skillcape:has-perk` with
  `{player, skill: "AGILITY", result: false}` and returns
  `request.result === true`.
- `itemName` (lines 27–33): id ≤ 0 → null, else
  `coreApi.ItemDefinition.forId(id)?.getName?.() ?? null`.
- `gracefulRestoreBonusFromSlots` (lines 36–58): pure bonus calculation.
  Per piece whose slot name starts with the prefix: += percent, pieces++.
  If the cape substitutes AND the cape slot name does not already start
  with "Graceful cape": += cape percent (3%), pieces++. 0 pieces → 0.
  All 6 pieces → += SET_BONUS. Returns the bonus fraction.
- `energyRestoreBonus` (lines 60–67): builds nameBySlot for the six
  slots from `player.getEquipment().getItems()` and calls the pure
  function with `capeIsSubstitute(player)`.
- `applyGracefulRunEnergy` (lines 74–83): caches the bonus in a WeakMap
  keyed by player until the six worn ids (joined signature) change;
  returns `bonus > 0 ? units * (1 + bonus) : units`.
- `addGracefulWeights` (lines 91–104): for each piece whose slot name
  matches, sets `request.slotWeights[slot] = weightKg` (kg, NOT grams)
  unconditionally; returns the worn count. No-op when no player or no
  slotWeights.
- `addGracefulEffects` (lines 109–117): calls `addGracefulWeights`
  first; when worn == 6, or worn == 5 with a substitute cape, pushes
  `` `Graceful set: +${Math.round(bonus * 100)}% run energy restore` ``
  (bonus recomputed via `energyRestoreBonus`) onto `request.lines` and
  pushes `SET_TEXT = 8` onto `request.setTexts` (enum 3047 id).
- Entry (lines 120–137): name "GracefulOutfit", description verbatim in
  the file, `members: true`, `_test` exports, `register` is attach-only:
  `api.registerRunEnergyRestoreModifier(applyGracefulRunEnergy)`,
  `api.onCustomEvent("equipment:effects", addGracefulEffects)`,
  `api.onCustomEvent("equipment:weights", addGracefulWeights)`.

### WornWeights.plugin.js

- `WORN_WEIGHT_ID` (lines 12–17): 88→89 boots of lightness, 10553→10554
  penance gloves, 10069→10073 spotted cape, 10071→10074 spottier cape.
- `addWornWeights` (lines 21–29): iterates ALL equipment slots; when the
  worn id is in the map AND `request.slotWeights[slot]` is not already
  finite (first writer wins), sets it to
  `core.ItemDefinition.forId(wornId).getWeight() / 1000` (kg). No-op
  when no player or no slotWeights.
- Entry (lines 31–38): name "WornWeights", description verbatim, NO
  members flag (loads on both world types), `register` attach-only:
  `api.onCustomEvent("equipment:weights", addWornWeights)`.

### TS engine grounding

- `EquipmentEffects.ts`: module-level registries. `RunEnergyRestoreModifier
  = (entity, units) => number` (line 21), `RunEnergyDrainModifier =
  (entity, amount) => number` (line 22). `registerRunEnergyRestoreModifier`
  / `registerRunEnergyDrainModifier` ignore non-functions (lines
  199–209). `applyRunEnergyRestoreModifiers` / `applyRunEnergyDrainModifiers`
  (lines 303–308, 321–325): reduce, keeping the previous value when a
  modifier returns a non-finite or negative result. All other modifier
  families in the file (melee/ranged/magic hit, prayer, special energy,
  status) are OUT OF SCOPE for this phase.
- `PluginTypes.ts:1491-1498` + `PluginManager.ts:4761-4768`: `PluginApi`
  exposes `registerRunEnergyRestoreModifier` and
  `registerRunEnergyDrainModifier`, delegating to the EquipmentEffects
  module functions. Both are non-hook members (currently table-only in
  `internal/plugins/hooks.go`).
- `Player.getCarriedWeightGrams` (`Player.ts:1284-1311`): emits
  `equipment:weights` with `{player, slotWeights: {}}` FIRST, then per
  equipment slot uses `override * 1000` when the override is finite AND
  the slot holds an item with id ≥ 0 and amount > 0, else the definition
  weight (0 for stackable/noted/empty). Inventory slots use definition
  weight only. `getWeightKg` truncates grams/1000 toward zero
  (`Player.ts:1314-1316`).
- `MovementQueue.runEnergyDrainUnits` / `runEnergyRestoreUnits`
  (`MovementQueue.ts:803-815`): drain base
  `floor((60 + floor(67*weight/64)) * (1 - agility/300))`, weight clamped
  0..64; restore base `floor(agility/10) + 15`; each returns
  `max(0, floor(apply*Modifiers(p, base)))`. Formulas are already ported
  (phase 7) and MUST NOT change — only the identity apply hooks get
  wired to the registry.
- `equipment:effects` producer is plugin content:
  `EquipmentStats.plugin.js:126-133` (`describeEffects`) emits
  `{player, lines: [], slotWeights: {}, setTexts: []}`. EquipmentStats
  is NOT ported: in Go this event has NO producer (documented gap).
- `skillcape:has-perk` producer is plugin content:
  `skillcapes/Perks.SkillCapes.js:10` + `Common.SkillCapes.js:124-130`
  (`answerPerk` sets `request.result = true` when the worn/carried cape
  gives the skill's perk; `skill` is a Skill or its key string).
  SkillCapes is NOT ported: in Go the query has NO handlers, so
  `result` stays false — the honest no-hooks default. Do NOT invent
  skillcape content.

## Go current state (verified)

- `internal/game/runenergy.go`: `RunEnergyDrainUnits(weightKg, agility)`
  / `RunEnergyRestoreUnits(agility)` are pure; the apply hooks
  (`applyRunEnergyDrainModifiers` / `applyRunEnergyRestoreModifiers`)
  are IDENTITY with a documented NOT STARTED comment. Call sites:
  restore in `player.go:938` (Player.Process, has `p`); drain in
  `movement_queue.go:1235` (has `q.owner`, a `MovementOwner`; `AsPlayer`
  exists on Player/NPC — worker A verifies how the queue resolves the
  owner and whether NPC queues can drain, grounding the choice in
  `MovementQueue.ts`).
- `internal/game/weight.go`: `CarriedWeightGrams` has NO
  `equipment:weights` emit (documented omission in the file header).
- `ItemDefinitionSource` (`itemdef.go`) has `WeightGrams` but NO name
  method (deliberately absent; the `ItemAnimationSource` optional
  interface in the same file is the precedent for optional
  capabilities). The cmd/server adapter (`storeItemDefinitions`,
  `containerswiring.go:96`) already implements `ItemName`. `Player`
  exposes `ItemDefinitions()` and `Equipment()`; `Item.ID()` /
  `Amount()` / `IsValid()` and `ItemContainer.Items()` / `ItemAt(slot)`
  exist; slot constants (`HeadSlot` 0, `CapeSlot` 1, `BodySlot` 4,
  `LegSlot` 7, `HandsSlot` 9, `FeetSlot` 10) match the TS slots.
- Game→hub emit seam: `Player.EmitCustomEvent func(name string, payload
  any)` (`player.go:369-377`), nil-safe, installed by wiring. Hub
  custom events run handlers in subscription order with per-hook error
  isolation (`internal/plugins/events.go`); payloads are `any`, mutated
  through pointers (precedents: `ObjectExamineEvent`, Phase 20
  `skills:restore` pointer payload).
- Install precedents: `Hub.SetTaskManager` / `Hub.SetAreaManager`
  (engine→hub); `Api.RegisterNpcCombatMethodProvider` in
  `internal/plugins/combat.go` (hub→engine with queue-until-installed
  semantics — follow this for the modifier registry). World-scoped
  registries are the rule (`AreaManager`, Phase 21 `BoatManager`):
  NO process-wide globals (Phase 21 race-fix precedent).
- Content precedent: `internal/content/dateminutes` (mutex-guarded
  per-player map + `OnPlayerDisconnect` cleanup standing in for the TS
  WeakMap; `Plugin` var + `init()` + `plugins.Register`; blank import
  in `cmd/server/pluginwiring.go`).

## Seam API contract (FIXED — worker B codes against these exact names)

Game package (`internal/game/equipment_effects.go`, new file unless
worker A justifies otherwise):

```go
type RunEnergyRestoreModifier func(*Player, float64) float64
type RunEnergyDrainModifier func(*Player, float64) float64
```

Modifiers take `*Player` (TS takes `Mobile`; only players have run
energy in Go — document the narrowing). Registry is world-scoped with
queue-until-installed hub registration; apply semantics are the TS
reduce with the finite-and-≥0 guard; nil registrations are ignored
(TS ignores non-functions). When nothing is registered or installed,
apply is identity and every existing test passes unchanged in
behaviour. The two formula call sites apply modifiers at the TS
positions; the formula arithmetic does not change. Worker A decides
the exact registry shape and whether the pure formula functions gain
a player parameter, grounding the choice in the call sites; the
contract above is what worker B and the hub surface depend on.

Payload types + event names (game package, next to the payloads):

```go
const EquipmentWeightsEventName = "equipment:weights"
type EquipmentWeightsRequest struct {
    Player      *Player
    SlotWeights map[int]float64 // kg per equipment slot, as TS fills slotWeights
}
const EquipmentEffectsEventName = "equipment:effects"
type EquipmentEffectsRequest struct {
    Player      *Player
    Lines       []string
    SlotWeights map[int]float64
    SetTexts    []int
}
const SkillcapeHasPerkEventName = "skillcape:has-perk"
type SkillcapePerkRequest struct {
    Player    *Player
    Skill     string // e.g. "AGILITY", as GracefulOutfit passes it
    Inventory bool
    Result    bool
}
```

Weight emit site: `CarriedWeightGrams` emits `equipment:weights` via
`p.EmitCustomEvent` (nil-safe) with a `*EquipmentWeightsRequest` BEFORE
computing, then applies the TS override rule per equipment slot
(finite override + valid item → `int(override * 1000)` grams; else
definition weight). Document the float round-trip: overrides are kg
floats (graceful whole kg; worn weights `WeightGrams/1000`); the
conversion must reproduce the TS observable kg totals — pin the exact
WornWeights values in fixture tests.

Name seam: add an optional `ItemNameSource` interface
(`ItemName(id int) string`) in `itemdef.go` following the
`ItemAnimationSource` precedent, plus `func (p *Player) ItemName(id int)
string` returning "" when id ≤ 0, no source is installed, or the source
does not implement the interface (TS `?.getName?.() ?? null` → no
prefix match). Update the stale "Name deliberately absent" rationale
in `itemdef.go` minimally; `wearingHalberd` / `hasGodsword` /
`getItemCount` stay NOT STARTED (out of scope — touch only the
rationale comment, do not port them). Document the deviation: TS
content reads names via `core.ItemDefinition.forId`; Go content reads
them via the player's installed definition source (same Store); a Core
`ItemDefinition` surface stays table-only.

Hub surface (`internal/plugins/`, new file or fitting existing file):

```go
func (a *Api) RegisterRunEnergyRestoreModifier(modifier game.RunEnergyRestoreModifier)
func (a *Api) RegisterRunEnergyDrainModifier(modifier game.RunEnergyDrainModifier)
```

Both grounded in `PluginTypes.ts:1491-1498`; registration-gated like
other Api methods; queue-until-installed into the world's registry
(`RegisterNpcCombatMethodProvider` precedent). Move both names out of
the NOT STARTED table in `hooks.go` and update the ported-member count
honestly (24 → 26 of 95 if both land; verify by recounting the table).
No other hub surface: `equipment:weights` / `equipment:effects` /
`skillcape:has-perk` ride the existing generic custom-event bus
(`OnCustomEvent` / `EmitCustomEvent`); `getEquipment` is `Player.Equipment()`.

Wiring (`cmd/server/equipmentwiring.go`, new): install the queued
modifiers into the built world and the `EmitCustomEvent` seam already
covers the weight emit (verify the seam is installed on players at add;
if the seam install lives elsewhere, extend it — do not add a second
seam). Follow the existing `*wiring.go` + `server.go` hookup pattern.

## Content ports (worker B)

- `internal/content/gracefuloutfit`: package `gracefuloutfit`, plugin
  name "GracefulOutfit", description verbatim from the entry, `Members:
  true`. `register` attach-only, one hook per line, named functions:
  `RegisterRunEnergyRestoreModifier`, `OnCustomEvent("equipment:effects",
  ...)`, `OnCustomEvent("equipment:weights", ...)`. Port
  `gracefulRestoreBonusFromSlots` as a pure package-level function
  (name-by-slot map + substitute bool) tested fixture-only without a
  player; port the WeakMap cache as a mutex-guarded map keyed by
  `*game.Player` with `OnPlayerDisconnect` cleanup (dateminutes
  precedent — verify the disconnect emit's goroutine before deciding
  the mutex is needed, and keep it if in doubt). The skillcape query
  emits `skillcape:has-perk` through an emit func captured from the
  `Api` at Register, with a `*game.SkillcapePerkRequest` payload, and
  reads `Result` after the emit returns. `Math.round` → `math.Round`
  (identical for the positive values here); `String.startsWith` →
  `strings.HasPrefix`.
- `internal/content/wornweights`: package `wornweights`, plugin name
  "WornWeights", description verbatim, NO members flag. `register`
  attach-only: `OnCustomEvent("equipment:weights", ...)` only. Port the
  id map as a package-level map; the finite check is
  `math.IsNaN`/`math.IsInf`-aware; the worn weight is
  `float64(WeightGrams(wornId)) / 1000` via the player's definition
  source.
- `cmd/server/pluginwiring.go`: add the two blank imports,
  gracefuloutfit before wornweights (alphabetical; handler order is
  unobservable — no item matches both pieces' rules — document that).
- Each package documents its event inventory (emits/listens + which
  producers/consumers exist in Go) in its package comment.

## Event inventory (document in code + PORT_STATUS.md)

| Event | Producer in Go | Consumers in Go |
|---|---|---|
| `equipment:weights` | `Player.CarriedWeightGrams` (engine) | GracefulOutfit, WornWeights |
| `equipment:effects` | NONE — EquipmentStats plugin not ported (gap) | GracefulOutfit (registered; never fires) |
| `skillcape:has-perk` | GracefulOutfit (query) | NONE — SkillCapes plugin not ported; `Result` stays false (honest degradation) |

## Verification (coordinator-measured)

- `go build ./...`, `go vet ./...`, `gofmt -l .`, `CGO_ENABLED=0 go
  build ./...`: clean. `go test ./... -p 1 -count=1` (serialized per
  the `TestEngineTicksAtRate` flake; no tolerance change) with the exact
  passing-case count incl. subtests; `go test -race`. Zero JS files;
  game-never-imports-plugins intact; zero probe references remain.
- Live smoke test, 3 runs stable, real rev-242 cache, temp root outside
  the repo, deadline-free reader, ClientHandshake after LOGIN: equip
  graceful pieces and observe the restore-rate change (run-energy
  packet values over ticks) and the carried-weight change on the wire;
  equip boots of lightness / a cape and observe the worn-weight change;
  confirm no behaviour change with no graceful/worn items equipped.
  The Agility-cape substitute path cannot be observed live (no
  skillcapes plugin) — unit-cover it and say so.
- Docs: this file (Outcome), PORT_STATUS.md (header numbers, content
  rows, hub counts, gaps, next-modules order), CONTENT_MIGRATION_PLAN.md
  (§3 item 2 DONE + boundary + residual gaps).

## Outcome

COMPLETED 2026-11-16 (coordinator-verified; parent verification
pending). All scope items landed:

1. GracefulOutfit ported (`internal/content/gracefuloutfit/`):
   faithful port of `GracefulOutfit.plugin.js` — PIECES table,
   SET_BONUS, pure bonus function, signature-keyed cache
   (mutex-guarded map + disconnect cleanup, the WeakMap
   analogue), `skillcape:has-perk` query (no answerer in Go,
   honest false), `equipment:effects` + `equipment:weights`
   listeners, members: true, attach-only register.
2. WornWeights ported (`internal/content/wornweights/`): faithful
   port of `WornWeights.plugin.js` — the 88/10553/10069/10071
   id map, iterate-all-slots, first-writer-wins finite skip,
   no members flag.
3. The one engine seam built: world-scoped `EquipmentEffects`
   registry (`internal/game/equipment_effects.go`) wired
   through the unchanged Phase 7 drain/restore formulas;
   `equipment:weights` emit site in `CarriedWeightGrams`
   (`weight.go`) with the TS override rule; hub
   `RegisterRunEnergyRestoreModifier` /
   `RegisterRunEnergyDrainModifier` (24→26 of 95 non-hook
   members; hook families unchanged at 45/108 firable).
4. Event inventory named in both package comments:
   `equipment:weights` HAS a Go producer (the engine) and two
   consumers; `equipment:effects` has NO Go producer
   (EquipmentStats not ported — registered handler never
   fires); `skillcape:has-perk` has NO Go answerer
   (SkillCapes not ported — substitute path never triggers).
5. Hub exposure only as required (the two register members,
   grounded in `PluginTypes.ts`/`PluginManager.ts`).
6. Docs updated: this file, PORT_STATUS.md, and
   CONTENT_MIGRATION_PLAN.md (§3 item 2 DONE).

Verification (coordinator-measured): `go build`, `go vet`,
`gofmt -l`, `CGO_ENABLED=0 go build` clean; 598 `.go` files,
15 packages, 0 JS files; `go test ./... -p 1 -count=1`:
3,773 passing incl. subtests, 0 failures (3,696 + 77 new);
`go test -race`: clean, 15/15. Live smoke test, 3 runs
stable, real rev-242 cache, seeded saves, headless session
path, opcode-81 frames: plain +15/tick weight 0; full set
+19/tick weight −25kg; five pieces +17/tick weight −21kg;
boots of lightness +15/tick weight −4kg; six real "Wear"
packets mid-session moved a character from +15/3kg to
+19/−25kg. The Agility-cape substitute path was NOT observed
live (no SkillCapes plugin) — unit-covered only, as planned.
New finding (documented gap, not fixed): the login bootstrap
run-energy frame computes weight from the save
(`carriedWeightKg`, no emit), so frame 0 shows the natural
weight; the first tick frame corrects it. The far-WALK bug,
the y=3521–3522 wall, and the scrypt check stay untouched.
