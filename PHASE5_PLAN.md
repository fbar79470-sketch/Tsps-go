# Phase 5 plan (COMPLETED 2026-11-14 — approved 2026-11-14)

Goal: progress persists across logins. Phase 4 delivered movement
with real collision, but no save is ever written for a live player
and every login starts at the world spawn with default state: the
save is only read by the bootstrap for snapshot values. Phase 5
ports the save round-trip — `applyToPlayer`/`fromPlayer` semantics,
disconnect/shutdown/autosave wiring, and the SQLite backend — so a
player who walks away from spawn and logs out resumes at the
walked-to tile on the next login. It does NOT add JSON loaders,
spawns, packet listeners, movement completions, deferred flush
blocks, combat/skills/dialogue/shops, or plugin content.

Status: approved 2026-11-14 (Tyler: "Keep going with the save
round-trip"). Baseline verified before work started: `go build` /
`go vet` / `gofmt` clean, 644 test cases passing, `go test -race`
clean, 155 `.go` files.

## 0. Design contract (pinned; all workers code against it)

Layering stays as it is: `internal/game` imports no other
`tsps-go` package and `internal/persist` imports none either, so
apply/from logic splits into pure save-shape operations in
`internal/persist` (Worker A) and thin adapter wiring in
`cmd/server` (Worker B) using existing `game.Player` accessors
(`SetLocation`, `SetRunning`, `SetLastKnownRegion`, `Location()`,
`IsRunning()`). No new imports in `internal/game`.

Pinned `internal/persist` API (new file `apply.go`):

```go
// OwnedState is the live-player-owned subset of a save: the fields
// the Go Player demonstrably owns and snapshots overwrite.
type OwnedState struct {
    Position Location
    Running  bool
}
func (s *PlayerSave) OwnedState() OwnedState // non-nil receiver
func (s *PlayerSave) WithOwned(state OwnedState) *PlayerSave // deep-clone merge, receiver untouched
func SanitizeAppearance(appearance []int) []int
func (s *PlayerSave) MigrateForEntry() // sanitize + attribute-key migration + legacy Extra fold, in place
func (s *PlayerSave) Unmake()
type NewCharacterParams struct {
    Username string
    PasswordHash string
    Profile Profile
    Spawn Location
    Developer bool
    Now time.Time
}
func NewCharacterSave(p NewCharacterParams) *PlayerSave
```

## 1. Owned-vs-preserved field split (to be confirmed by Worker A)

OWNED (snapshot overwrites from live state): `position`,
`running`. Plus two fromPlayer constants: `teleblockTimer` is
always written as 0 (`PlayerSave.ts:774`, logging out ends Tele
Block), and default-valued attributes are stripped
(`StripDefaultAttributes`, as `fromPlayer` does). The password
hash is overwritten at ENTRY from the pending login
(`enterWorld` sets it after apply, `NetworkBuilder.ts:1670`), and
remembered-login tokens are folded into the carried save's
attributes at entry when the entering character is the primary.

PRESERVED (carried verbatim from the loaded base save, including
unknown keys and `json.RawMessage` fields): everything else —
`displayName`, `creationDate`, rights, spellbook, fightType,
autocast, combat/poison/vengeance/skull timers, `runEnergy`
(no live energy state exists), inventory, equipment, appearance
(sanitized once at entry, as `applyToPlayer` does), skills,
quick prayers, friends/ignores, banks, flags, sailing,
attributes, and `Extra`. A snapshot must never clobber these.

APPLIED at entry (from the save onto the live player): position
and running only — the Go `Player` holds nothing else. Entry also
sets `LastKnownRegion` to the applied location
(`NetworkBuilder.ts:1676`). Unrepresentable save fields must
never fail the login; they ride in the carried save.

## 2. Apply/from-player (Worker A: `internal/persist/apply.go`)

Port `applyToPlayer`/`fromPlayer` semantics
(`PlayerSave.ts:633-862`) in the merge form above, grounded with
TS file:line cites: `sanitizeAppearance` (ranges + colours,
`PlayerSave.ts:581-631`); the legacy-field-to-attribute migration
(`LEGACY_FIELD_ATTRIBUTES`, `PlayerSave.ts:27-50`, applied from
`Extra` on entry, migrated keys dropped from `Extra` as TS
`fromPlayer` drops them by building a fresh save);
`migrateAttributeKey` (camelCase to kebab-case,
`PlayerSave.ts:84-89`); `unmake()` (`PlayerSave.ts:204-213`);
new-character defaults for every field of `NewCharacterSave`,
grounded in the TS `Player`/container/spellbook/fightType
defaults. Load-time legacy button-indexed skill unmapping is
assessed and either ported (if the table is static TS data) or
recorded as an explicit gap. Round-trip tests must prove
unported fields and unknown keys survive byte-for-byte and owned
fields (especially position after movement) survive.

## 3. Wiring (Worker B: `cmd/server/savewiring.go` + small edits)

- Entry: `AddPlayer` applies the carried save (or builds a
  new-character save) instead of always spawning with defaults;
  dev-usernames grant DEVELOPER rights on new-character saves
  (`NetworkBuilder.ts:1670`); `CredentialName` is added to
  `EnteringPlayer` so token folding can apply the primary-only
  rule (`NetworkBuilder.ts:1672-1674`).
- Disconnect: save on removal via the existing
  `World.Hooks.OnPlayerDisconnect` seam (tick goroutine), reason
  grounded from TS (`Player.ts` logout path); bots
  (`IsBot`, the `bot-skip-persistence` proxy) are skipped.
- Shutdown: save-all with reason `"shutdown"`
  (`World.savePlayers`, `World.ts:207`; `Server.ts:200`), snapshotted
  on the tick goroutine before the engine stops (a tick in flight
  past `Engine.Stop` is NOT waited for — snapshot via a
  `TaskManager` one-shot, never by racing the tick loop). Tick-error
  save-all (`"tick-error"`, `GameEngine.ts:93`) uses the same path.
  A failed save is logged per player and never crashes the tick
  loop; shutdown continues (documented deviation — a SIGINT
  shutdown cannot be cancelled the way TS cancels its own).
- Autosave: port `plugins/persistence/Autosave.plugin.js` — a
  `TaskManager` task every tick, per-player due ticks spread by
  the name hash (`offsetOf`), 500–1000 ticks (env
  `AUTOSAVE_INTERVAL_TICKS`), max 10 saves per tick, reason
  `"autosave"`, scheduled on login and forgotten on logout.
  Synchronous writes on the tick goroutine (bounded by the
  per-tick budget; consistent with the existing `JSONFileStore`
  synchronous-write divergence).

## 4. SQLite backend (Worker C: `internal/persist/sqlite.go`)

Implement `SaveStore` per the schema in `internal/persist/doc.go`
and `plugins/persistence/SqlitePlayerPersistence.plugin.js`:
`player_saves(username TEXT PRIMARY KEY, save_json TEXT NOT NULL,
updated_at TEXT NOT NULL)`, WAL pragmas, default path
`data/saves/players.sqlite` (env `PLAYER_SAVE_DATABASE_PATH`),
same `PlayerSave` JSON shape and `validateSaveForWrite`
validation as `JSONFileStore`, plus the save-history table with
copy insertion (gzip, hash dedupe, reason) and retention pruning.
Driver decision: prefer a pure-Go driver (no CGO) that builds and
tests cleanly here; if none is acceptable, document exactly why
SQLite is deferred and keep `json` the default — no fake backend,
and the `--save-backend` flag keeps working. History
list/restore, legacy JSON import, and the pid-file guard are
explicit gaps unless implemented. The coordinator wires the
`"sqlite"` backend case in `cmd/server` from Worker's C report.

## 5. Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`,
  `go test ./... -p 1 -count=1` and `-race` clean (serialized for
  the known `TestEngineTicksAtRate` parallel flake).
- Round-trip, disconnect/shutdown/autosave, and SQLite tests as
  above; fixtures only, no network.
- Live smoke test (coordinator-run, `--no-cache`, temp root
  outside the repo, cleaned up after): log in, walk to a
  different tile, disconnect, re-login, resume at the walked-to
  tile (not spawn), save file on disk reflects it; shutdown
  save-all writes saves for connected players. No live cache
  download this phase.

## Explicitly out of scope

JSON definition loaders, custom-definition overlays, spawns,
packet listeners beyond the turn bridge, movement completions
(run-energy drain, destination-flag encoder, run-toggle listener,
mobility gates), deferred flush blocks (music/ground items/NPC
effects), combat/skills/dialogue/shops, plugin-strategy decision,
real-production scrypt-hash check, offline admin ops
(`updateOffline`/`copyProfile`/`delete`/`changePassword`),
`requestSave` coalescing (no content events exist to request
saves). Anything hit as a dependency is an explicit documented
gap, never a silent stub.

## Acceptance

A client can log in, walk somewhere, log out, and log back in at
the walked-to tile with its save intact; shutdown and autosave
write saves; the SQLite backend (or its documented deferral)
round-trips the same save shape.

## Outcome (completed 2026-11-14, verified by the coordinator)

All scope items landed. `internal/persist/apply.go` ports the
apply/from-player merge (owned: position, running,
`teleblockTimer`=0, default-attribute stripping; everything else
preserved verbatim), `sanitizeAppearance`, migration, `unmake`,
`NewCharacterSave`, and the legacy button-indexed skill
unmapping. `cmd/server/savewiring.go` wires entry application,
disconnect (`"logout"`), Autosave (`"autosave"`), shutdown
(`"shutdown"`), and tick-error (`"tick-error"`) saves, with the
activation rule (a login that never enters never writes).
`internal/persist/sqlite.go` implements the SQLite backend with
the pure-Go `modernc.org/sqlite` v1.57.0 driver (no CGO; v1.60+/
v1.62 require Go 1.26), wired to `--save-backend sqlite` with
`json` still the default. Verification: build/vet/gofmt clean,
`CGO_ENABLED=0` build clean, 750 test cases passing (0
failures), race clean, all `-p 1`. Live smoke tests against the
built binary (`--no-cache`, temp roots outside the repo, since
removed) proved relogin persistence on both the JSON backend
(save file + rebuild region) and the SQLite backend (`player_saves`
row + gzipped history snapshot), and shutdown save-all for a
connected player holding its socket open. Documented gaps:
`requestSave` coalescing, offline admin ops, snapshot
list/restore, legacy JSON import, pid-file guard, and the
one-tick shutdown-snapshot window; run-energy drain/restore is
still NOT STARTED, so `runEnergy` is preserved but never
changes. See PORT_STATUS.md.
