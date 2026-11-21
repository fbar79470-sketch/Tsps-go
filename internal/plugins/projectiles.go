package plugins

import "tsps-go/internal/game"

// Projectile Core member: the typed Go form of
// core.Projectile (PluginTypes.ts:1611;
// PluginManager.ts:2665, getCoreApi). In TS it is `any` —
// the Projectile class. In Go it is ProjectileClass,
// delegating to internal/game (projectile.go), where the
// model lives. Content builds projectiles through
// Create (or New for the raw constructor), tunes them
// with the With* builders on *game.Projectile, and fires
// them with SendProjectile.
//
// NO HOOK FAMILIES. No on*/emit* family exists for
// projectiles in TS: PluginTypes.ts and PluginManager.ts
// contain no onProjectile/emitProjectile family (verified
// by grep at bfd81e3). This file therefore adds NO
// hookRegistry methods and NO Api members: the
// firable-family count (44/108) and the PluginApi member
// count (19/95) are unchanged. The Core count moves 4 ->
// 7 with Sound, Sounds (sounds.go), and Projectile; the
// coordinator records the counts in core.go/hooks.go/
// doc.go (see sounds.go).

// ProjectileClass mirrors core.Projectile: the Projectile
// class (game/model/Projectile.ts). It is stateless; the
// zero value is ready.
type ProjectileClass struct{}

// New builds a Projectile from raw parts: the constructor
// (start, end, lockon, projectileId, delay, speed,
// startHeight, endHeight, privateArea)
// (Projectile.ts:30-46). area is the source's Area()
// pointer (nil for the main world); see game.NewProjectile
// for why the area is an *Area and not a PrivateArea.
func (ProjectileClass) New(start, end game.Location, lockon game.ProjectileMobile, projectileID, delay, speed int, startHeight, endHeight float64, area *game.Area) *game.Projectile {
	return game.NewProjectile(start, end, lockon, projectileID, delay, speed, startHeight, endHeight, area)
}

// Create builds a projectile fired by source at victim
// (Projectile.createProjectile, Projectile.ts:122-142).
// See game.CreateProjectile.
func (ProjectileClass) Create(source, victim game.ProjectileMobile, projectileID, delay, speed int, startHeight, endHeight float64) *game.Projectile {
	return game.CreateProjectile(source, victim, projectileID, delay, speed, startHeight, endHeight)
}

// CentreOf returns the centre tile of a mobile's footprint
// (Projectile.centreOf, Projectile.ts:68-72).
func (ProjectileClass) CentreOf(mobile game.ProjectileMobile) game.Location {
	return game.CentreOf(mobile)
}

// ArrivalCycles returns the cycles between a projectile
// being sent and it landing (Projectile.arrivalCycles,
// Projectile.ts:82-89). TS defaults are delay 40, perTile
// 10; callers pass them explicitly.
func (ProjectileClass) ArrivalCycles(from, to any, delay, perTile int) int {
	return game.ArrivalCycles(from, to, delay, perTile)
}

// ArrivalTicks returns the same arrival in game ticks
// (Projectile.arrivalTicks, Projectile.ts:95-97).
func (ProjectileClass) ArrivalTicks(from, to any, delay, perTile int) int {
	return game.ArrivalTicks(from, to, delay, perTile)
}

// TicksToLand returns the ticks to queue a hit so its
// splat shows when a projectile ending at endCycle lands
// (Projectile.ticksToLand, Projectile.ts:103-105).
func (ProjectileClass) TicksToLand(endCycle int) int {
	return game.TicksToLand(endCycle)
}

// FlightDistance returns the horizontal tiles a projectile
// flies to the nearest tile of the target
// (Projectile.flightDistance, Projectile.ts:112-120).
func (ProjectileClass) FlightDistance(start, end game.Location, victim game.ProjectileMobile) int {
	return game.FlightDistance(start, end, victim)
}

// LegacyMagicEndBase is Projectile.LEGACY_MAGIC_END_BASE
// (Projectile.ts:104-106): 46, the end-cycle base the
// legacy magic shape normalizes to. It is game
// .LegacyMagicEndBase (combat_magic.go), exposed here so
// the class mirror is complete.
func (ProjectileClass) LegacyMagicEndBase() int {
	return game.LegacyMagicEndBase
}

// Projectile returns the Projectile class surface: the
// core.Projectile member (PluginTypes.ts:1611). Safe on a
// nil Core: the surface is stateless.
func (c *Core) Projectile() ProjectileClass {
	return ProjectileClass{}
}
