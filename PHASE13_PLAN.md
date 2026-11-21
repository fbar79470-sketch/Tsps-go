# Phase 13 plan (COMPLETED 2026-11-15 — coordinator: see Outcome at the end)

Object registry + object interaction, milestone 3 of
`CONTENT_MIGRATION_PLAN.md` §2 ("Object registry + object
interaction (MapObjects, walk-to-object, loc encoders;
spawned objects are currently clipping-only and invisible).
Unlocks: `objects/`, most of `skills/`, `areas/`, and the
object half of `quests/`"). This is PORT_STATUS.md
next-module #1. Its central deliverable: the Phase 6
clipping-only object spawns become VISIBLE to clients, and
object clicks route through the hub. It does NOT add
dialogue (milestone 4), timers + areas (5), NPC drivers +
combat (6), shops/trading (7), interface runtime (8),
clans/social (9), skill handlers + XP sources (10),
sailing/template regions/sounds (11), any content port
beyond what proves objects work, the pre-existing far-WALK
routing bug found in Phase 12 smoke testing (single WALK
clicks over 64 tiles route nowhere; staged clicks work;
recorded below in the Phase 12 outcome — needs TS grounding
before any fix and is NOT this phase's work), the
real-production scrypt-hash check, MobileList.All perf
work, or HTTP extras/WebRTC. No embedded JS runtime, no
Node sidecar — both ruled out by the decided strategy; do
not build, prototype, or spike them. Do NOT copy any
`*.plugin.js` file into the repo.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only
checkout at `~/tsps-bfd81e3` (verified; TS root
`server/src/main/typescript/elvarg/`). Grounding entry
points (workers: cite file:line for every behavior):
`game/entity/impl/object/GameObject.ts` (174 lines),
`MapObjects.ts` (278), `ObjectManager.ts` (434),
`net/packet/impl/ObjectActionPacketListener.ts` (234),
`net/packet/impl/UseItemPacketListener.ts` (239),
`net/packet/PacketSender.ts` `sendObject`/`sendObjectRemoval`/
`sendObjectMerge`/`sendObjectAnimation` (1313-1360),
the `ClientProtocol.ts` loc encoders, `RegionManager.ts`
map-object loading (how cache-map objects populate
`MapObjects`), `MovementQueue.ts` `walkToObject` + reach
rules, `ObjectDefinition.forPlayer` + the multi-loc rule
(the examine seam in `internal/listeners/examine.go`
already ports part of this — reuse it), and the
`PluginTypes.ts` object surface (`onObjectRoute`,
`onObjectInteraction` + name/actions overload,
`onBeforeObjectInteraction`, `onObjectClick`,
`onObjectFirstClick`..`onObjectFifthClick`, `onItemOnObject`,
`onSpellOnObject`, `emitObjectInteraction`,
`getObjectManager`, `core.ObjectManager`/`core.MapObjects`).
Private areas (`PrivateArea`) and boat-deck reach
(`DECK_REACH_TILES`, `setDeckReach`, `setUsableFromDeck`)
belong to sailing/templates (milestone 11) and stay gaps
with grounding; spell-on-object needs magic (milestone 6)
and stays a gap.

## Scope, in order

1. Object registry: port the TS object model — `GameObject`
   (id/type/face/location, baseMap/mapOriginal/spawnedBy),
   `MapObjects` (tile-hash registry, owner objects, `find`
   semantics incl. the bridge-tile z-1 fallback and
   `forPlayer` variant resolution), `ObjectManager`
   (register/deregister indexes, scene queries, shutdown
   `removePluginObjects`), region-load population of the
   registry from cache-map objects, and JSON-spawn
   registration replacing the clipping-only path (clipping
   behavior kept, registry + visibility added).
2. Loc encoders + sync: port the loc encoders and wire
   them into the rebuild/flush path — scene snapshots on
   region change (TS `onRegionChange` ordering, incl.
   removals-before-additions and missed removals) plus
   per-tick deltas for runtime spawn/replace/remove.
   Objects appear at the right tile with the right
   type/rotation and disappear when removed.
3. Object interaction: implement `ObjectActionPacketListener`
   (opcodes 96, 28, 42, 38, 51 + 231) per TS — gates,
   lookup, click-type/action normalization, walk-to-object
   (reuse the Phase 9 walk-to machinery; ground the reach
   rules incl. multi-tile objects), and dispatch to the
   hub's object-interaction hook families with the
   TS-exact no-handler fallback (ground it; invent no
   default message). Object examine (Phase 12) is reused,
   not redone.
4. Item-on-object: port it through the same dispatch if TS
   core routes it there (`UseItemPacketListener` item-on-
   object paths + `onItemOnObject`); otherwise leave it
   decode-drop and document the boundary with grounding.
   Spell-on-object stays decode-drop (magic NOT STARTED).
5. Hub exposure: object-interaction hook families with
   registration methods + production emit sites, and the
   object-query/spawn surface content uses
   (`getObjectManager` / `core.ObjectManager` /
   `core.MapObjects` equivalents), with counts updated in
   the hub tables. Keep `game`-never-imports-`plugins`
   intact.
6. Save/state implications: determine from TS whether any
   object state persists across restarts; handle through
   the existing owned/preserved split, no new owned fields
   without TS grounding. If object state is purely
   runtime/config, document that explicitly.

## Verification

Build/vet/gofmt clean, CGO_ENABLED=0 build clean,
`go test ./... -p 1 -count=1` + `-race` (serialized for the
known `TestEngineTicksAtRate` flake; no tolerance change),
fixtures-only unit tests, plus a live smoke test (built
binary, temp data root outside the repo, `--no-cache`,
deadline-free reader): a JSON-spawned object appears in
rebuild frames at the correct tile; a runtime spawn through
a hub hook appears in a delta frame and disappears on
removal; clicking an object walks the player to it and
fires the hub hook (observed via a test hook's `sendMessage`
reply); distance gating verified as TS defines it. Claim
each behavior only if verified live. Long walks in the
smoke test use staged clicks (far-WALK bug, out of scope).

## Outcome

COMPLETED 2026-11-15. All six scope items landed, verified
(build/vet/gofmt clean incl. CGO_ENABLED=0, 2,137 test cases
incl. subtests across 12 packages, race clean, live smoke
test 3 runs stable — see PORT_STATUS.md):

1. Object registry: `GameObject` + the `MapObjects` tile/owner
   maps + the `ObjectManager` world list/indexes, register/
   deregister with the TS replacement/removal/clipping
   interplay, `Find` with the bridge-tile z-1 fallback and
   `forPlayer` variant resolution (nil-var degradation),
   region-load population of cache-map objects as map
   originals, JSON-spawn registration replacing the
   clipping-only path, per-tick deltas, shutdown
   `removePluginObjects` wired after the saves.
2. Loc encoders + sync: add/change (134), del (135), anim
   (136), merge (138) with absolute world tiles; per-session
   sync ports `onRegionChange` ordering (missed removals,
   removals before additions, hidden removals, region scan,
   raw owner sends) + shown/missed/hidden tracking + delta
   fan-out gated by scene; wired into `FlushTick` at both
   region-change sites and the delta drain. Merge/animation
   encoders have no producers.
3. Object interaction: `ObjectActionPacketListener`
   implemented per TS (gates, lookup, click-type/action
   normalization, walk-to-object with the reach rectangle +
   repath rules, arrival still-stands re-check, before/main/
   operated dispatch order, TS-exact silent no-handler
   fallback). Boat-deck branches, the `bank` option, and
   facing are gaps.
4. Item-on-object: implemented through the same dispatch
   (no HP/busy gate, clickType 0, "Nothing interesting
   happens." fallback). Item-on-NPC/player/item/ground-item
   and spell-on-object stay decode-drop.
5. Hub exposure: 10 more families firable (19 of 108:
   route, interaction + named overload, before-interaction,
   click + first..fifth click, item-on-object) and 3 more
   non-hook members ported (11 of 95: `getObjectManager`,
   `emitObjectInteraction`, `emitItemOnObject`), plus
   `Core.ObjectManager()`. `game` never imports `plugins`.
6. Save/state: nothing about objects persists (runtime-only,
   re-derived from JSON at boot). No new owned fields.

Live-verified end to end: JSON-spawn visibility, runtime
spawn/remove deltas with distance gating, click-to-hook.
Gaps carried forward: `ReplaceInPlace` emits no removal
delta for the outgoing loc; `spawnedBy` stamping is the
caller's job (no plugin context); merge/animation producers;
private-area branches; `wasRemovedOnLoad` bookkeeping;
region-reload runtime-clipping restore; far-WALK bug
(pre-existing, still out of scope); scrypt check;
MobileList.All perf note.
