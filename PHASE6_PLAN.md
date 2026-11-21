# Phase 6 plan (COMPLETED 2026-11-14 — approved 2026-11-14)

Goal: the world is no longer empty. JSON definition data loads at
boot, custom-definition overlays apply onto the cache facade,
spawned NPCs/objects enter the world, and every packet listener
has a decided, documented fate.

Outcome: COMPLETED and verified. All four scope items landed.
`go build ./...`, `go vet ./...`, `gofmt -l .` clean;
`CGO_ENABLED=0` build clean; `go test ./... -p 1` all 9 packages
pass (1,126 cases incl. subtests, 0 failures; 750 phase-5
baseline + 376 new); `-race` passes. Live smoke test: a spawned
NPC appeared in NPC_SYNC additions at exactly the right offset,
the far NPC stayed invisible, listener packets were absorbed
with no disconnect, and a subsequent WALK + disconnect save
held the walked-to tile. Real-file validation: the loaders
parsed the real 81 MB definitions directory read-only (30,827
NPC spawns, 1 object spawn, 1,952 ground items, 512 shops,
38 layouts). No data files, fixtures, or cache binaries left
under the repo. See PORT_STATUS.md for the full account.

## 1. JSON definition loaders (new `internal/definition` package)

DONE. All 8 core loaders + wrappers + `WorldConfigStore`, in
`GameBuilder.loadStartupData()` order, with TS-faithful
required/optional file behavior (7 files required, the rest
tolerated or optional-when-missing as documented). Dialogue
loading, `ShopManager`/`Sailing` init, and
`NpcAnimationScanner` NOT STARTED, as scoped. The shipped
`data/definitions/` carries minimal valid contents for the
required files so the starter root boots.

## 2. Custom-definition overlays

DONE. `registerCustomDefinitions` / `registerCustomItems` on
the Phase 2 archived registry, precedence custom items >
custom definitions > archived > native, verified in the TS
getters. Missing custom files mean zero rows.

## 3. Spawns

DONE. NPC spawns apply eagerly via `QueueAddNPC` before the
engine ticks (static: no drivers). Object spawns apply as
region clipping only — spawned objects are INVISIBLE (no
`GameObject` registry / loc encoders; explicit gap). Deferred
per-region activation NOT STARTED. Ground-item spawns load
but are not applied (no ground-item system).

## 4. Packet listeners (`net/packet/impl/*`, 18 files)

DONE as decode + explicit drop. All 18 files decoded through
the turn-packet bridge / `TurnHandler` (WALK/MOVE_GAMECLICK
behavior unchanged); ZERO listeners implemented, because no
target system exists in Go — no behavior invented, no walk
ever routed. Per-listener status (also in PORT_STATUS.md):

| Listener | Status | Reason / note |
|---|---|---|
| ChangeAppearancePacketListener (37) | decode-drop | Appearance system NOT STARTED |
| ChatPacketListener (190) | decode-drop | Chat broadcast + chat encoder NOT STARTED |
| CommandPacketListener (none; `::` sub-dispatch) | decode-drop | Plugin-owned command registry NOT STARTED |
| DropItemPacketListener (240 + widget) | decode-drop | Inventory + ItemOnGroundManager NOT STARTED |
| EquipPacketListener (240 + widget) | decode-drop | Containers, SkillManager, combat NOT STARTED |
| FollowPlayerPacketListener (46, 233) | decode-drop | Following machinery + World lookup NOT STARTED |
| InterfaceActionClickOpcode (251 + natives) | decode-drop | Plugin-owned interface actions NOT STARTED |
| ItemActionPacketListener (240 + widget) | decode-drop | Containers + plugin item actions NOT STARTED |
| MagicOnItemPacketListener (94, 90) | decode-drop | Skills, Inventory, magic, ground items NOT STARTED |
| MagicOnPlayerPacketListener (32) | decode-drop | Combat/magic system NOT STARTED |
| NPCOptionPacketListener (76, 12, 34, 70, 57, 75) | decode-drop | NPC interaction/combat/dialogue, walk-to-entity, plugins NOT STARTED |
| ObjectActionPacketListener (96, 28, 42, 38, 51, 231) | decode-drop | MapObjects, walk-to-object, plugin interactions NOT STARTED |
| PickupItemPacketListener (102, 43, 103, 56, 82, 232) | decode-drop | ItemOnGroundManager + Inventory NOT STARTED |
| PlayerOptionPacketListener (44, 45, 46, 73, 6, 48, 10, 21, 233) | decode-drop | Combat, Trading, following, plugins NOT STARTED |
| SecondGroundItemOptionPacketListener (ground opcodes) | decode-drop | ItemOnGroundManager + plugin interactions NOT STARTED |
| SwitchItemSlotPacketListener (242, 1, 253) | decode-drop | Inventory/Bank containers NOT STARTED |
| TradeRequestPacketListener (45, 233) | decode-drop | Trading, walk-to-entity, plugins NOT STARTED |
| UseItemPacketListener (65, 36, 86, 79, 2, 241) | decode-drop | Inventory, MapObjects, plugin item-use NOT STARTED |

## Explicitly out of scope (untouched)

Combat, dialogue, shops/trading systems, containers/appearance/
skills internals, movement completions (run-energy drain,
destination-flag encoder, run-toggle listener, mobility gates),
deferred flush blocks (music/ground items/NPC effects), NPC AI,
save-system extensions, plugin-strategy decision, real-production
scrypt-hash check.

## Acceptance (all met)

- `go build ./...`, `go vet ./...`, `gofmt -l .` clean;
  `go test ./... -p 1` and `-race` pass (1,126 cases).
- Live smoke test: spawned NPC in NPC_SYNC frames; listener
  packets absorbed; WALK + save intact afterwards.
- Real-file shape validation done (counts above).
- No data files, fixtures, or cache binaries left under the repo.
