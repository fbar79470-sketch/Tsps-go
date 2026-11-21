# Phase 11 plan (COMPLETED 2026-11-15 — verified: build/vet/gofmt clean incl. CGO_ENABLED=0, 1,837 test cases pass incl. subtests across 12 packages, race passes; live smoke test 3 runs stable)

Player attributes, milestone 1 of `CONTENT_MIGRATION_PLAN.md` §2
(the largest single blocker: `get/setAttribute` in 1,002 of 2,907
plugin files, `api.persistAttribute` 2,018 occurrences). This is
PORT_STATUS.md next-module #1. It does NOT add chat + commands
(milestone 2), object registry, dialogue, timers + areas, NPC
drivers + combat, shops/trading, interface runtime, clans/social,
skill handlers + XP sources, sailing/template regions/sounds,
any content port beyond what proves attributes work, the
real-production scrypt-hash check, MobileList.All perf work, or
HTTP extras/WebRTC. No embedded JS runtime, no Node sidecar —
both ruled out by the decided strategy; do not build,
prototype, or spike them.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only
checkout at `~/tsps-bfd81e3` (verified; do not use /tmp paths).
Attribute mechanics: `game/entity/impl/Mobile.ts:102-110`
(lazy `Map<Object, Object>`, `getAttribute`/`setAttribute` — no
has/remove methods exist in TS core), `game/entity/impl/player/
persistence/PlayerSave.ts:26-101` (legacy-field map, persistent
key set, `attributeDefaults`, `persistAttribute`,
`applyAttributeDefaults`, `migrateAttributeKey`,
`cloneAttribute`), `:681-697` (applyToPlayer attribute load),
`:793-816` (fromPlayer attribute rebuild + default stripping),
`net/NetworkBuilder.ts:1664-1674` (login order: transient attrs,
then applyToPlayer or defaults, then login tokens),
`plugins/PluginTypes.ts:1349-1351` (`persistAttribute` doc),
`plugins/PluginManager.ts:4616` (createApi passthrough).
`server/AGENTS.md` rule 14 states the kebab-case namespaced
convention; it is convention plus the `persistAttribute`
declaration mechanism, not code enforcement beyond
`persistAttribute`'s own key validation (non-empty, no
surrounding whitespace). ScopedPluginSettings "no declared key"
warnings (`WorldConfigStore.ts:191`, `PluginManager.ts:2415`)
concern pluginConfig keys, not attribute keys — out of scope.
Do NOT copy any `*.plugin.js` file into the repo.

## Scope, in order

1. Attribute storage + accessors on `Mobile` (TS home of the
   map): lazy map, `GetAttribute` (nil when absent),
   `SetAttribute` (nil value deletes — the Go form of TS
   `setAttribute(key, undefined)`, which `fromPlayer` skips),
   `HasAttribute` (Go-only presence helper; TS distinguishes
   via `undefined`). Mutex-guarded: login emits run on the
   login goroutine while tick work runs on the tick goroutine
   (the dateminutes precedent). No enumeration method: TS
   never enumerates the map (`fromPlayer` iterates the
   persistent registry). No `RemoveAttribute`: TS has none.
2. Persisted vs session-only, exactly as TS draws it: a
   process-global persistent-key + defaults registry
   (`persist` package, mirroring the `PlayerSave` statics),
   fed by core keys (legacy-field values,
   `npc-aggression:tolerance`, `creation-menu:last-item`,
   `login:tokens`, `combat:style-by-category`, the fromPlayer
   bank registrations) plus runtime `persistAttribute`
   calls. Entry loads only persistent keys present in the
   save; defaults fill missing keys (both the save and the
   new-account path); snapshots rebuild attributes from live
   persistent keys minus default-equal values. Session-only
   attributes start empty every login and never reach a save.
   Unknown keys in a loaded save are dropped on the next
   snapshot, as TS `fromPlayer` rebuilds from scratch —
   verify and document this change from the current
   preserved-verbatim behavior.
3. Save integration, following the phase 7/9 owned-field
   precedent: attributes become owned state (`OwnedState`,
   nil-means-preserve). `MigrateForEntry` key migration and
   the legacy fold keep working against the live registry;
   `StripDefaultAttributes` consults live defaults; entry
   application and snapshots in `cmd/server/savewiring.go`
   move attributes between save and live player in TS order.
   Every other preserved field stays untouched.
4. Key rules: port `persistAttribute` validation only; do NOT
   invent kebab-case enforcement or a declaration registry.
   Record the convention honestly in code/docs.
5. Hub exposure: `Api.PersistAttribute` delegating to the
   persist registry (panic on invalid input, mirroring the TS
   throw; the loader recovers it and drops the plugin).
   Content reaches attributes through `*game.Player` methods
   on event payloads — no new Core accessor. Update the
   `hooks.go` non-hook member table. Keep the
   `game`-never-imports-`plugins` rule intact.
6. Audit of attribute-shaped engine state: migrate onto
   attributes ONLY where TS grounds it (appearance
   pronouns/skin unlocks, XP lock, bank insert-mode/
   placeholders/quantity modes are the candidates); leave
   everything else (Running, music state, ground-item
   bookkeeping, dateminutes dedup) with a grounded reason.
   No churn without grounding.

## Verification

Build/vet/gofmt clean, CGO_ENABLED=0 build clean,
`go test ./... -p 1 -count=1` + `-race` (serialized for the
known `TestEngineTicksAtRate` flake; no tolerance change),
fixtures-only unit tests, plus a live smoke test (built
binary, temp data root outside the repo, `--no-cache`,
deadline-free reader): a persisted attribute set in-session
is present in the written save and restored on relogin, a
session-only attribute is gone after relogin, and other save
fields are undisturbed. Claim each behavior only if verified
live.

## Outcome

COMPLETED 2026-11-15. All six scope items landed:

1. Attribute store on `Mobile` (`internal/game/attributes.go`):
   lazy mutex-guarded map, `Get/Set/HasAttribute`, nil-deletes
   (TS `setAttribute(key, undefined)`), no enumeration/remove
   (grep-verified absent in TS). No `Player` accessor surface
   (`IsPlaceholders` etc.) was added — the two live reads go
   direct to `GetAttribute` via the `bank.go` constants.
2. Persisted vs session-only per TS: live registry in
   `internal/persist/attributes.go` (26 core keys, 8 core
   defaults, insertion order, `cloneAttribute` rules);
   snapshots rebuild from scratch and drop unregistered keys
   (TS-faithful change from preserved-verbatim, documented
   in PORT_STATUS.md).
3. Save integration: `OwnedState.Attributes` (nil-means-
   preserve), entry application + defaults in TS order,
   transient mobile/native attributes, live login-tokens
   attribute, snapshot rebuild with per-player recovery on
   marshal failure.
4. Key rules: `persistAttribute` validation only; kebab-case
   is convention-only, documented honestly. Scoped-settings
   "no declared key" warnings confirmed out of scope (they
   concern pluginConfig keys).
5. Hub: `Api.PersistAttribute` (panic on invalid input,
   loader drops the plugin); member table 4→5 ported;
   `game`-never-imports-`plugins` intact.
6. Audit: appearance pronouns/skin unlocks, XP lock, bank
   placeholders/insert mode migrated onto live attributes;
   Running/music/ground-items/dateminutes/autosave/session
   state verified correctly placed with TS cites.
   Coordinator addition: engine-consumed keys
   (`appearance:pronouns`, skin unlocks, `client-layout-
   root`, `xp-drops:enabled`) registered at boot in
   `cmd/server/attributewiring.go`, mirroring the
   MakeOverMage/Settings registrations, so the first
   snapshot does not drop keys the engine itself reads.
   Gaps left: `client-layout317` + all other content keys
   unregistered until their ports declare them; no Go
   constants for the transient/xp-drops keys; `Player`
   attribute accessor methods not added; stale
   attribute-related comments in `listeners/equip.go`,
   `listeners/itemaction.go`, `game/visibility.go` left for
   later phases. Live smoke test (temporary hub hook +
   real binary, since removed, 3 runs stable): persisted
   attribute survived relogin, session-only expired,
   default-equal stripped and refilled, walked-to position
   undisturbed. Claim earned: plugins can now store state
   on players.
