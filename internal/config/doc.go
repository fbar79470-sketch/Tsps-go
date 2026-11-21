// Package config loads world settings and environment configuration for
// the Go port of TSPS.
//
// World settings come from data/definitions/world.json with an optional
// data/definitions/world.local.json overlay, parsed and validated the
// way WorldDefinition.ts does. Environment variables follow server/
// .env.example and the process.env reads in the TypeScript sources; see
// LoadEnv for the default of each variable.
//
// Upstream sources (paths relative to server/ in RSPSApp/tsps):
//   - src/main/typescript/elvarg/game/definition/WorldDefinition.ts
//   - src/main/typescript/elvarg/game/definition/WorldPluginConfig.ts
//   - src/main/typescript/elvarg/game/GameConstants.ts
//   - src/main/typescript/elvarg/net/NetworkConstants.ts
//   - src/main/typescript/elvarg/game/World.ts (capacities)
package config
