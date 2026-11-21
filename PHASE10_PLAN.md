# Phase 10 plan (COMPLETED 2026-11-15 — approved 2026-11-14)

The Go plugin hub + content migration plan, following Tyler's
decided Go-native plugin strategy (2026-11-15): content is
ported to Go packages behind a Go hook registry, piece by
piece. No embedded JS runtime, no Node sidecar — both ruled
out; do not build, prototype, or spike them. This is
PORT_STATUS.md next-module #1. It does NOT add engine systems
(combat, dialogue, shops, trading/duelling, chat/clan, NPC
drivers, object registry), any content port beyond the single
proof piece, the real-production scrypt-hash check,
MobileList.All perf work, or HTTP extras/WebRTC.

Upstream reference: RSPSApp/tsps commit `bfd81e3`, read-only
checkout at `~/tsps-bfd81e3` (verified, HEAD `bfd81e3`; do
not use /tmp paths). Hub shape: `plugins/PluginManager.ts`
(5,252 lines), `plugins/PluginTypes.ts` (1,786 lines),
`game/definition/WorldPluginConfig.ts`,
`game/definition/WorldSettingDeclarations.ts`.
ANALYSIS.md §8 summarizes the system; the source is the
authority. Do NOT copy any `*.plugin.js` file into the repo.

## Scope, in order

1. Go plugin hub (`internal/plugins`). Port the
   PluginManager/PluginApi SHAPE to idiomatic Go:
   compile-time content registration (no `*.plugin.js`
   discovery — deliberate deviation, documented with
   gains/losses); dependency ordering with TS
   `loadPluginCandidatesWithDependencies` semantics;
   config-declaration validation BEFORE any registration
   (bad/clashing declarations logged and dropped);
   world.json `plugins` switches (exported name, then file
   name), legacy `disabledPlugins` handling, members-only
   gating on F2P worlds, unknown-switch warnings;
   `plugins:loaded` emit + `active=/disabled=/members-only=`
   counts log; hook registry + emit for the lifecycle
   families the engine can fire, custom namespaced events
   (`domain:event`), per-hook error isolation (panicking
   hook logged, never fatal; login-extension hooks fail
   closed); synchronous gate contracts grounded per family;
   a scoped Go `Core` access surface (what exists in Go
   today only — NOT a 217-member dynamic mirror); boot
   wiring in `cmd/server` (construct, register compiled-in
   content, validate/order, emit startup; shutdown emits
   shutdown hooks).
2. Content migration plan: `CONTENT_MIGRATION_PLAN.md`
   (durable). Measured inventory per `server/plugins/`
   subdirectory; engine-systems-first ordering grounded in
   what each area's plugins actually call; recommended
   first content pieces with rationale; porting rules;
   explicit non-goals; honest statement of what will
   likely never be ported.
3. One proof content piece: the SMALLEST real TS plugin
   whose every dependency already exists in Go, ported end
   to end through the hub with an observable effect. If no
   real plugin qualifies, the survey evidence must say so
   and the hub is proven with a minimal test-only content
   package instead. This is the ONLY content port.

## Seam contracts (coordinator-dictated; workers must match)

- Package `internal/plugins`. `game` never imports it (the
  established seam direction: engine exposes hooks, the
  adapter calls the hub). `internal/plugins` MAY import
  `internal/game` for typed event payloads; it must NOT
  import `internal/session`, `internal/protocol`,
  `internal/persist`, or `cmd/server`. Content packages
  import `internal/plugins` (+ `internal/game` as needed).
- `Plugin` struct (loader worker defines, `plugin.go`):
  `Name string` (required, non-blank; the exported name),
  `ID string` (stable identifier; defaults to Name — the
  compile-time analogue of the TS filename key used in
  switch resolution), `Description string`,
  `Members bool`, `Enabled *bool` (nil = default on, per
  TS `enabled?: boolean`), `DependsOn []string`,
  `Config map[string]ConfigDeclaration`,
  `Register func(api *Api)`, `RegisterDeferred func(api *Api)`
  (optional; nil = absent). A plugin with nil `Register`
  is skipped with a warning, mirroring TS
  `collectPluginLoadCandidates` (missing `register(api)`).
- Package-level registry: `func Register(p Plugin)` appends
  to the default registry (content packages call it from
  `init` or from explicit wiring); `func DefaultPlugins()
  []Plugin` returns a copy. `Hub` is built from an explicit
  list: `NewHub(list []Plugin, opts HubOptions) *Hub`.
  Tests construct hubs directly; production uses the
  default registry snapshot.
- `HubOptions`: `Logger *log.Logger` (nil = `log.Default()`),
  `Switches map[string]bool` (normalized-name -> on/off,
  from `internal/config` `World.Plugins`, normalized with
  `NormalizeName`), `DefaultEnabled *bool` (legacy
  `disabledPlugins` default; nil when the modern `plugins`
  map is in use), `PluginConfig map[string]any` (from
  `internal/config` `World.PluginConfig`),
  `MembersWorld bool`, `DisablePlayerBots bool` (reserved;
  no bot content exists — accepted, documented, unused
  beyond the filter shape).
- `Hub.Load()` performs, in TS `loadFromDirectory`
  (`PluginManager.ts:681-793`) order: switch filter +
  unknown-switch warnings; members gating (counted
  separately); config-declaration check (before ANY
  registration); dependency-ordered registration (TS
  `loadPluginCandidatesWithDependencies`,
  `PluginManager.ts:2522-2568`: multi-pass, deferred
  `registerDeferred` candidates re-queued in path order,
  no-progress break, unresolved-dependency warnings);
  record accepted declarations (first loaded claimant
  owns a clashing key); warn about invalid config values;
  emit `plugins:loaded`; log
  `[plugins] active=N disabled=M` (+ `members-only=K`
  when > 0). Load is idempotent: a second call is a no-op
  (TS `initialized` guard). `Hub.Loaded() []string` returns
  loaded plugin names in load order.
- Name normalization: `NormalizeName` = lowercase, strip
  all non-`[a-z0-9]` (`WorldPluginConfig.ts:8-10`).
  `ResolveEnabled(switches, defaultEnabled, exportedName,
  fileName, pluginEnabled)` implements
  `resolvePluginEnabled` (`PluginManager.ts:159-171`)
  exactly: exported-name switch, then file-name switch,
  then defaultEnabled, then `pluginEnabled != false`.
  Dependency normalization mirrors
  `normalizePluginDependencies`
  (`PluginManager.ts:2476-2520`): nil -> empty, non-array
  impossible in Go (a nil slice is the only input shape),
  blank entries dropped with a warning, self-dependency
  dropped with a warning, duplicates dropped keeping first
  order. Dependency matching is by EXACT (unnormalized)
  name against loaded names, as TS does (`loadedByName`
  holds raw `pluginName` strings).
- Config declarations: `ConfigDeclaration` mirrors
  `ConfigOptionDeclaration`
  (`WorldSettingDeclarations.ts:5-21`): Type, Label, Help,
  Default (`any`, presence tracked), DefaultText,
  Values, Min/Max, Integer, ExclusiveMin, Members.
  `ValidateDeclaration(key, decl) []string` ports
  `validateDeclaration` (key must match
  `^[^:\s]+:[^:\s]+$`, type in the TS TYPES list, label/help
  required, exactly one of default/defaultText, unknown
  fields rejected, enum/number field rules). Clashing
  keys: core-owned keys win (the Go core declaration set
  is EMPTY unless the coordinator adds one — document
  that); between plugins, the first claimant is expected
  to own it, ownership goes to the first that loads, and
  the clash is logged (`checkConfigDeclarations`,
  `PluginManager.ts:2336-2384`). `Hub.GetPluginConfig(key,
  def)` mirrors `getPluginConfig`
  (`PluginManager.ts:2306-2315` + the per-plugin prefixing
  at 4374-4375): bare keys (no `:`) are prefixed
  `pluginConfigKey(pluginName, key)` = `<plugin>:<key>`
  with the legacy namespace mapping from
  `WorldPluginConfig.ts` (port the mapping; the Go config
  package already normalizes `PluginConfig` keys, so the
  hub reads the normalized map — verify against
  `internal/config/world.go` before duplicating).
  `Api.GetPluginConfig(key, def)` prefixes with the
  owning plugin's name.
- `Api` struct (hooks worker defines, `api.go`): one per
  plugin, created by the hub at registration time, carrying
  the plugin name + hub + core. Hook-registration methods
  mirror the `PluginApi` `on*` families (see below); each
  ignores a nil handler silently, mirroring the TS
  `typeof handler !== "function"` guards. `Api` also
  exposes `EmitCustomEvent(name, payload)`,
  `GetPluginConfig(key, def)`, `Core() *Core`, and
  `Name() string`. Registration after `Load()` completes
  is ignored with a warning log (TS registers only during
  the load passes).
- Hook storage: the hooks worker defines `hookRegistry`
  (unexported) in `hooks.go` with constructor
  `newHookRegistry() *hookRegistry`; `Hub` (loader worker,
  `hub.go`) holds it as field `hooks *hookRegistry`,
  created in `NewHub`. All hook lists live in the
  registry, guarded by a `sync.RWMutex` (registration
  happens on the boot goroutine; emits happen on tick /
  session goroutines). Hook execution goes through ONE
  helper, `executeHook`, mirroring `PluginManager.executeHook`
  (`PluginManager.ts:429-466`): run the handler, recover
  a panic, log `[plugins] <label> hook failed
  (<pluginName>)` + the recovered value, continue. No
  perf profiling (explicit gap).
- Lifecycle hook families with production emit call
  sites this phase (hooks worker defines the event types
  + `Hub.Emit*` methods; the COORDINATOR wires the call
  sites): `EmitServerStartup` (TS call site:
  `GameBuilder.ts:35` — after world build, before
  serving), `EmitServerShutdown` (`Server.ts:199` —
  graceful shutdown), `EmitPlayerLogin`
  (`NetworkBuilder.ts:1694` — enterWorld),
  `EmitPlayerLogout` (`Player.ts:752`),
  `EmitPlayerDisconnect` (`World.ts:1260` — queue
  removal, non-bot registered players only),
  `EmitPlayerProcess` (`World.ts:1372` — per-player
  process phase). Event payloads are typed Go structs
  carrying `*game.Player` where TS carries `player: any`,
  plus the TS event fields that exist in Go (username,
  source, timestamp, isNewAccount/profile where the
  adapter can supply them — fields with no Go source are
  omitted and documented, never faked). Every OTHER
  `PluginApi` hook family is registered-but-unfired: the
  hooks worker provides the `Api.On*` registration method
  ONLY for families whose event type can be honestly
  typed today; families needing unported systems get NO
  registration method and are listed, with the missing
  system named, in the `doc.go` unfired-family table and
  in PORT_STATUS.md. Never silently drop: a family with
  no registration method must appear in the table.
- Custom events: `Hub.EmitCustomEvent(name string, payload
  any)` and `Api.OnCustomEvent(name string, handler func(any))`.
  Name rule `^[^:\s]+:[^:\s]+$`
  (`isPluginCustomEventName`, `PluginManager.ts:174-176`):
  invalid names are silently ignored on BOTH subscribe and
  emit, as TS does. Handlers run in subscription order
  through `executeHook`. `plugins:loaded` payload is
  `PluginsLoadedEvent{Names []string}` (TS passes a Set;
  Go carries the load-ordered slice — documented).
- Gate contracts (hooks worker defines generic gate
  helpers in `gates.go`, each grounded in its TS emit;
  gates are registered-but-unfired until engine systems
  exist — the helpers + contracts are the deliverable,
  plus unit tests proving each contract):
  - allow-veto (`emitCanAttack`, 1360-1377): all hooks
    run; `allow` reset to undecided before each; a deny
    returns deny immediately; an allow sticks; result is
    allow/undecided (never a bare deny-from-silence).
  - first-answer (`emitCanTeleport`, 1379-1390): first
    non-undecided answer wins, immediate return.
  - reason-gate (`emitCanLogout`, ~1393): deny returns the
    reason (default "You can't log out right now."); allow
    returns immediately with no reason.
  - handled (`emitNpcInteraction` 1246-1261,
    `emitSocialPacket` 836-843): run until `handled`, return
    handled. Dialogue variant: first answer wins (ground
    the exact emit before implementing).
  Each helper documents its TS source lines and which
  families use it. Do NOT invent a uniform gate rule.
- Login extension (hooks worker, `login.go`):
  `Api.RegisterLoginExtension(factory)` stores the
  factory (last registration wins? — ground in TS:
  `loginExtensionFactory` is a single slot; verify
  overwrite vs first-wins from source). `Hub.CreateLoginExtension`
  calls the factory WITHOUT recover — panics propagate
  (fail closed, `PluginManager.ts:794-798`). No production
  call site exists this phase (session login has no
  extension seam): mechanism + tests only, gap documented.
- `Core` (hooks worker, `core.go`): a small struct, one
  shared instance per hub, exposing ONLY what exists in
  Go today: `Hub() *Hub` (emit surface), `Logger()
  *log.Logger`, `Config()` read access via the hub, and
  `Cycle() int64` through an optional seam installed by
  the adapter (nil = 0, documented). Extension points are
  documented in code comments naming the future engine
  system per accessor. Do NOT mirror the 217-member
  `PluginCoreApi`; do NOT invent engine systems to fill
  it. `PluginContext` (ambient current-plugin) is NOT
  ported: hook errors already label the plugin name
  (documented deviation — Go has no thread-locals).
- `registerDeferred` semantics: a plugin with
  `RegisterDeferred` is re-queued after its own `register`
  pass with cleared dependencies, in ID order (TS sorts
  by `pluginPath`; Go sorts by `ID` — documented
  analogue), and its deferred function runs in the NEXT
  pass; a plugin whose `Register` panics is NOT added to
  loaded names and its dependents stay unresolved (TS
  `registerPluginCandidate` returns null on throw,
  `PluginManager.ts:2570-2590`; the Go equivalent recovers
  the panic, logs `[plugins] failed to initialize <id>`,
  and continues). A failed `RegisterDeferred` logs the
  same way and does not un-load the plugin.
- Logging: all hub output through the injected
  `*log.Logger`, TS message shapes preserved
  (`[plugins] ...`). No `fmt.Println`, no `log.Default()`
  calls inside the package except the documented nil
  fallback in `NewHub`.
- File ownership: the loader worker owns ONLY
  `internal/plugins/{doc.go,hub.go,plugin.go,loader.go,config.go,switches.go}`
  + their tests. The hooks worker owns ONLY
  `internal/plugins/{api.go,hooks.go,events.go,gates.go,core.go,login.go}`
  + their tests, and must NOT edit the loader worker's
  files (same-package cross-references use the contracts
  above). The plan worker owns ONLY
  `CONTENT_MIGRATION_PLAN.md`. The coordinator owns
  `PORT_STATUS.md`, `README.md`, `cmd/server/*`,
  `internal/game/world.go`, `internal/game/player.go`,
  `internal/session/*`, and the proof content package.
  Workers do NOT edit `PORT_STATUS.md` or `cmd/server`.

## Verification

Build/vet/gofmt clean, CGO_ENABLED=0 build, `go test ./...
-p 1 -count=1` + `-race` (serialized for the known
`TestEngineTicksAtRate` flake; no tolerance change), plus
the live smoke test: hub loads with active/disabled counts
logged, proof piece observable, switch-disabled plugin
does not run, bad config declaration logged and dropped
without failing boot. Fixtures-only unit tests; live
smoke with `--no-cache` in a temp root outside the repo,
cleaned up afterward. Deadline-free reader goroutine
(phase 7 lesson).

## Outcome

COMPLETED 2026-11-15. All three scope items landed: the Go
plugin hub (`internal/plugins` — compile-time registry,
dependency-ordered loading, config-declaration validation,
world.json switches, members-gating, lifecycle + custom-event
hooks with per-hook error isolation, per-family gate
contracts, scoped core surface, login extensions) with boot
wiring (`cmd/server/pluginwiring.go`, all six lifecycle emit
sites); the content migration plan
(`CONTENT_MIGRATION_PLAN.md` — measured inventory of 577
entries / 2,907 JS files / 789,355 lines, engine-systems-
first ordering, first-piece recommendations, porting rules);
and one proof content piece (`internal/content/dateminutes`,
faithful port of `DateMinutes.plugin.js`). Verified:
build/vet/gofmt clean incl. CGO_ENABLED=0, 1,780 test cases
incl. subtests across 12 packages, race clean, and the live
smoke test (3 runs stable) proving login sends exactly one
VARP_LARGE frame for varp 3078 with the current epoch minute,
a switch-disabled plugin sends none, and a bad config
declaration is logged and dropped without failing boot — plus
a real binary boot from a temp root outside the repo. See
PORT_STATUS.md for the honest limits: 101 of 108 hook
families have no registration method yet (table-documented
with the missing system named), no gate has a production emit
site, login extensions are placeholders with no call site,
player attributes do not exist in Go, and the legacy
`disabledPlugins` list form is not honored.
