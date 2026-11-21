// Package persist ports the TSPS persistence layer: password hashing,
// remembered-login tokens, accounts/display-names registries, punishments,
// and the JSON-serializable player save snapshot with its file and
// in-memory backends.
//
// Sources (server/ relative, commit bfd81e3):
//   - src/main/typescript/elvarg/util/PasswordUtil.ts
//   - src/main/typescript/elvarg/net/login/LoginTokens.ts
//   - src/main/typescript/elvarg/game/entity/impl/player/Accounts.ts
//   - src/main/typescript/elvarg/game/entity/impl/player/DisplayNames.ts
//   - src/main/typescript/elvarg/util/PlayerPunishment.ts
//   - src/main/typescript/elvarg/game/entity/impl/player/persistence/PlayerSave.ts,
//     PlayerProfile.ts, PlayerPersistence.ts, NullPlayerPersistence.ts
//   - plugins/persistence/JsonPlayerPersistence.plugin.js
//   - plugins/persistence/SqlitePlayerPersistence.plugin.js (schema/shape only)
//
// Every behavior is cited to its TS source file in comments. Behavior that
// could not be grounded in TS is absent or returns an explicit error; see
// the per-file "UNGROUNDED"/"DIVERGENCE" notes.
//
// SQLite backend: implemented in sqlite.go (phase 5) with the pure-Go
// modernc.org/sqlite driver (no CGO). The TS default backend
// (SqlitePlayerPersistence.plugin.js) stores one row per storage key in
// table player_saves(username TEXT PRIMARY KEY, save_json, updated_at),
// where username is the profile storage key (see StorageKey) and save_json
// is the same JSON shape PlayerSave marshals to; the Go store reuses
// PlayerSave's JSON encoding and the validation in store.go, and adds
// the TS snapshot-history table (player_save_history) with SHA-1
// dedupe and retention pruning. Snapshot listing/restore, legacy JSON
// import, and the pid-file single-writer guard are NOT STARTED.
//
// Scrypt parameters: Node's crypto.scrypt defaults (N=16384, r=8, p=1) are
// used; they were verified empirically against Node v24.22.1's
// crypto.scrypt (golden vectors in password_test.go), but NOT against a
// real production TSPS hash. See password.go.
package persist
