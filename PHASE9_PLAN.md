# Phase 9 plan (COMPLETED 2026-11-15 — approved 2026-11-14)

Containers, appearance, and skills, plus the listener
implementations they unblock. This is PORT_STATUS.md
next-module #1. It does NOT add combat, dialogue, shops,
trading/duelling, skilling ACTION content (XP sources beyond
what a ported listener itself grants per TS), plugin events
of any kind, the plugin-strategy decision, the
real-production scrypt-hash check, or MobileList.All perf
work.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only
checkout at `~/tsps-bfd81e3` (verify it exists; do not use
/tmp paths).

## Scope, in order

1. ItemContainer core + Inventory/Equipment/Bank
   (`game/model/container/ItemContainer.ts` 886 lines,
   `impl/Inventory.ts`, `impl/Equipment.ts`,
   `impl/Bank.ts`, `StackType.ts`, `game/model/Item.ts`).
   Container semantics: slots, stacking, capacity,
   add/remove/swap/move, refresh behavior. Equipment slots
   and equip requirements as TS structures them (see
   `game/model/equipment/EquipmentRequirements.ts`);
   requirements depending on unported systems are explicit
   gaps. Bank: container mechanics + tabs/placeholders only
   as the Bank container code defines them
   (`TOTAL_BANK_TABS = 11`, `CONTAINER_START = 50300`,
   search tab = index 10, `rearrange`); bank
   interface/dialogue flows needing unported widget/dialogue
   systems stay explicit gaps.
2. Appearance (`game/model/Appearance.ts`, the appearance
   block in `PlayerSession.createPlayerView` /
   `ClientProtocol` encode, `Player.getAppearanceEquipmentIds`,
   `Player.onEquipmentChanged` at `Player.ts:662`): live
   appearance derived from identity + equipment, Flag.APPEARANCE
   updates on equipment change, animation/render overrides
   from equipment where the data exists in Go.
3. Full SkillManager
   (`game/content/skill/SkillManager.ts`, `game/model/Skill.ts`):
   replaces the phase 7 minimal Agility read. XP tables/level
   computation, XP gain + level-up behavior, current/boosted
   levels, drains/restores, skills sync encoder wiring (the
   phase 2 encoder exists — wire live levels through it). XP
   gain SOURCES are out of scope: the manager accepts XP and
   levels correctly, but no action grants XP unless a ported
   listener does so per TS.
4. Live carried weight: inventory + equipment per TS weight
   computation, replacing the phase 7/8 entry-time static
   weight in the run-energy drain seam.
5. Listener implementations unblocked by 1–4 ONLY: equip,
   drop, switch-slot, item actions, ground-item pickup/drop
   flows. Each exactly as its TS listener does. Any listener
   depending on a still-unported system stays a decode-drop
   with its reason updated. No other listener
   implementations.

## Save integration (required, phase 7 runEnergy precedent)

Containers + skills move from preserved-verbatim to owned:
load from the save at entry (`PlayerSave.applyToPlayer`:
`getInventory().setItems`, `getEquipment().setItems`,
sanitized appearance, `getSkillManager().setSkills`,
`PlayerSave.ts:665-680` region), write back live state on
save (`fromPlayer`, `PlayerSave.ts:737-862`; trade-escrow
fold stays a gap — no Trading system). Every other
preserved field untouched. The coordinator owns the
`internal/persist` merge edits and `cmd/server/savewiring.go`
changes; workers expose Snapshot/Load on the game types
using plain Go values and do NOT edit `internal/persist`.

## Seam contracts (coordinator-dictated; workers must match)

- `game.Player` carries `Inventory() *Inventory`,
  `Equipment() *Equipment`, `Bank(tab int) *Bank`
  (lazy-create, TS `getBank` shape; 11 tabs),
  `Skills() *SkillManager`, `Appearance() *Appearance`,
  constructed by `NewPlayer` via `NewInventory(p)`,
  `NewEquipment(p)`, `NewBank(p)`, `NewSkillManager(p)`,
  `NewAppearance(p)`. The coordinator owns `player.go`;
  workers define these types in their own new files.
- `game` never imports `session`/`protocol`/`definition`/
  `cache`/`persist`. Definition lookups (stackable, noted,
  weight, equipment slot, requirements, tradeable/dropable,
  actions) go through an `ItemDefinitionSource` interface
  (defined by the containers worker in
  `internal/game/itemdef.go`, nil-safe with documented
  no-definition defaults); the `cmd/server` adapter
  implements it, reusing the existing lookup code behind
  the ground-item seams — never duplicated. The coordinator
  owns `player.go`, including the `itemDefs` field and the
  `OnEquipmentChanged func(*Player)` hook field.
- Container refreshes and skill updates reach the client
  through coordinator-installed hooks (nil-safe no-ops
  without a coordinator), never through new `SessionPort`
  methods. The session package already encodes
  (`EncodeInventorySnapshot`, `EncodeBankSnapshot`,
  `EncodeSkillsDelta`, `EncodePlayerAppearance`); the
  `cmd/server` adapter sends through the existing
  `SessionPort.SendPacket`. Each worker implements its own
  adapter in a NEW `cmd/server/*wiring.go` file.
- Appearance-block rebuilds on equipment change go through
  the `OnEquipmentChanged` hook: the adapter rebuilds the
  block from live state (look + equipment + skills) via a
  session-package builder taking plain data, and stores it
  with the existing `SetSyncAppearance`. The bootstrap-time
  block construction stays as-is.
- The run-energy drain keeps reading Agility through the
  existing accessor; the coordinator rewires that accessor
  to the live SkillManager during integration. The
  containers worker owns the weight lines of
  `internal/game/runenergy.go` ONLY and must not touch the
  Agility read or the formulas. No other worker edits
  `runenergy.go`.
- File ownership: workers own ONLY their new files plus the
  files their brief explicitly names. `PORT_STATUS.md`,
  `internal/persist/*`, `internal/game/player.go`,
  `internal/session/flush.go`, `internal/session/bootstrap.go`,
  `cmd/server/savewiring.go`, `cmd/server/syncsource.go`,
  `cmd/server/server.go` are coordinator-only. The
  listeners worker (phase 2 of this phase, after containers
  + skills land) owns `internal/listeners/*` and may extend
  `internal/game/movement_queue.go` with walk-to-arrival
  machinery ONLY as grounded in `MovementQueue.walkTo*`.

## Per-listener status (final)

- IMPLEMENTED: EquipPacketListener (equip/unequip, 2H/shield
  rule, requirements, stackable merge), DropItemPacketListener
  (registers() ground-item registration; destroy-item widget
  flow stays a gap), SwitchItemSlotPacketListener (bank
  rearrange + inventory swap), ItemActionPacketListener
  (gates + equip fallback; plugin actions stay drops),
  PickupItemPacketListener (walk-to arrival + transfer;
  300ms throttle as a cycle stamp). VarpTransmitRunToggle
  stays implemented (phase 7).
- All other listeners: decode-drop, reasons updated where
  the missing system changed (see PORT_STATUS.md
  per-listener table and `listeners.Statuses()`).

## Verification

Build/vet/gofmt clean, CGO_ENABLED=0 build, `go test ./...
-p 1 -count=1` + `-race` (serialized for the known
`TestEngineTicksAtRate` flake; no tolerance change), plus
the live smoke test: equip changes appearance + weight, drop
creates a visible ground item, pickup removes it and adds
to inventory, switch-slot reorders inventory in sync,
save round-trip keeps containers/skills. Fixtures-only unit
tests; the live smoke test genuinely needed cache data
(equip/drop/pickup routing reads real definitions), so it
booted with the real rev-242 cache in a temp root outside
the repo, cleaned up afterward.

## Outcome

COMPLETED 2026-11-15. All five scope items landed:
containers (ItemContainer/Inventory/Equipment/Bank +
ItemDefinitionSource seam), live appearance (equipment
rebuilds via OnEquipmentChanged, weapon animation
overrides via the ItemAnimationSource seam), full
SkillManager (replacing the phase 7 minimal Agility read),
live carried weight (entry-time static weight removed),
and the five listener implementations (plus the
walk-to/arrival machinery in MovementQueue and the
ItemActionSource seam for routing). Save integration:
inventory/equipment/appearance/skills/banks moved from
preserved-verbatim to owned (nil-means-preserve merge,
entry load + snapshot write-back). Verified: build/vet/
gofmt clean incl. CGO_ENABLED=0, 1,590 test cases incl.
subtests across 10 packages, race clean, and the live
smoke test (3 runs stable) proving equip/drop/pickup/
switch-slot/save-round-trip end to end against the real
cache. See PORT_STATUS.md for the honest limits: no
combat/dialogue/shops/trading, no plugin events, no bank
interface flows, no destroy-item flow, no sounds, no
chat-message encoder (refusal/examine text dropped).
