package game

import "math"

// Projectile: the port of game/model/Projectile.ts (254 lines):
// createProjectile + sendProjectile broadcast to viewers, the
// centre/arrival/flight-distance helpers, the legacy-magic
// timing normalization, and the withSource/withAngle/
// withProgress/withEndDelay builders.
//
// The wire encoding lives in internal/protocol
// (EncodeProjectiles); delivery goes through the
// ProjectileSender session extension, following the
// SoundSender precedent — the game layer never imports
// protocol or session. The session sender owns the
// end-cycle fallback, the height x4 scaling, and the
// resolved-index-to-actor mapping (PacketSender
// .sendProjectile, PacketSender.ts:1236-1291).
//
// VIEWER RULE (grounded, not invented): sendProjectile
// iterates World.forEachNetworkPlayer (Projectile.ts:204),
// i.e. shouldRunNetworkUpdates (World.ts:921-931: bots
// without a network client skipped, session-connected
// check) — in Go, the installed network-player source
// (SetNetworkPlayerSource, sounds.go) plus the IsBot /
// SessionPort.IsOpen gates, the same degeneration the
// ground-item fan-out documents (no watched/unwatched or
// hasNetworkClient distinction in Go). Visibility is
// CrossFloor.projectileVisible (CrossFloor.ts:22-45): with
// no plane policy registered it degenerates to same-plane
// plus Chebyshev distance (the visibility.go precedent),
// at range max(15, viewer NpcViewDistance) — "seen as far
// as NPCs are" (Projectile.ts:217). Area matching compares
// Mobile.Area() pointers, the contract-2 equivalence for
// getPrivateArea comparison (see PlayAreaSound).
//
// BOAT DEGENERATIONS, DOCUMENTED. Three TS branches need
// sailing, which lands in this same phase (Worker C) but
// in files this file must not depend on:
//
//   - createProjectile's across-deck branch
//     (CombatPosition.acrossDeck, Projectile.ts:128-142):
//     across is always false here, so start/end are
//     always the centres of source/victim and the area is
//     always the source's. rootCentreOf has no Go
//     equivalent until BoatManager.rootLocation exists.
//   - sendProjectile's syncArea fallback
//     (Projectile.ts:210-213): a viewer in a different
//     area is skipped outright; there is no deck-as-main-
//     world second chance, and the viewer location is
//     always the player's own (no rootLocation).
//   - The privateArea constructor argument is the
//     source's *Area (see NewProjectile), not a
//     PrivateArea: PrivateArea does not exist in Go yet
//     (Worker B, this phase), and contract 1 makes the
//     Area() pointer the area identity.
//
// When sailing lands, these three sites — and only these —
// gain the boat branches; the timing, visibility, and
// broadcast shape below do not change.
//
// NO COMBAT PRODUCERS. TS producers are MagicCombatMethod,
// RangedCombatMethod, and CombatSpells; Go combat has no
// projectile call site (the Phase 16 gap). Nothing in this
// file is wired to combat; the seam is CreateProjectile +
// SendProjectile, which a combat method calls when one
// learns to fire. The PROJECTILE_DEBUG logging
// (Projectile.ts:238-246) is not ported: Go has no env-gated
// projectile debug, and inventing one is out of scope.

// ProjectileMobile is the mobile surface Projectile needs:
// a location, an area identity, a footprint size, a world
// index, and the player/NPC distinction. *Player and *NPC
// both satisfy it. Size is definition-driven for NPCs
// (NPC.Size) and 1 for players (Player.Size).
type ProjectileMobile interface {
	Location() Location
	Area() *Area
	Size() int
	GetIndex() int
	IsPlayer() bool
}

// Projectile is one fired projectile: frozen start/end
// tiles, a lock-on target, timing, heights, and the arc
// builders. Coordinates freeze at construction: Go
// Location is a value type, so later movement cannot
// mutate the trajectory (TS clones, Projectile.ts:37-40).
// Heights are TS floats in quarter-tiles (Projectile.ts:14-15,
// 26: startHeight/endHeight are numbers — 40.75, 36.5 —
// not integers); the session sender scales them x4, so a
// bow sends 163/146 exactly.
type Projectile struct {
	start             Location
	end               Location
	speed             int
	projectileID      int
	startHeight       float64
	endHeight         float64
	lockon            ProjectileMobile
	lockonTargetIndex int
	// sourceIndex is the mobile firing it, as a resolved
	// lock-on index (players negative, NPCs positive, 0
	// none): the client starts the projectile from where
	// that mobile is drawn. 0 unless WithSource names an
	// NPC (players never name their caster).
	sourceIndex int
	delay       int
	// area is the source's private-area identity as an
	// Area() pointer (nil = main world). See the file
	// header: PrivateArea does not exist in Go yet.
	area     *Area
	angle    int
	progress int
	endDelay int
}

// NewProjectile builds a Projectile: constructor (start,
// end, lockon, projectileId, delay, speed, startHeight,
// endHeight, privateArea) (Projectile.ts:30-46). area is
// the source's Area() pointer (nil for the main world);
// lockon may be nil (an untargeted shot). The lock-on
// target index resolves at construction, so a later
// retarget cannot change it.
func NewProjectile(start, end Location, lockon ProjectileMobile, projectileID, delay, speed int, startHeight, endHeight float64, area *Area) *Projectile {
	return &Projectile{
		start:             start,
		end:               end,
		lockon:            lockon,
		projectileID:      projectileID,
		delay:             delay,
		speed:             speed,
		startHeight:       startHeight,
		endHeight:         endHeight,
		area:              area,
		lockonTargetIndex: resolveLockonTargetIndex(lockon),
		angle:             16,
		progress:          64,
	}
}

// resolveLockonTargetIndex resolves a lock-on to the
// packet's signed index (resolveLockonTargetIndex,
// Projectile.ts:48-61): 0 for no target, -(index+1) for a
// player, index+1 for an NPC. A negative index resolves
// to 0 (TS: not an integer, or < 0); Go indices are
// always integers.
func resolveLockonTargetIndex(lockon ProjectileMobile) int {
	if lockon == nil {
		return 0
	}
	index := lockon.GetIndex()
	if index < 0 {
		return 0
	}
	if lockon.IsPlayer() {
		return -(index + 1)
	}
	return index + 1
}

// Start returns the frozen start tile.
func (p *Projectile) Start() Location { return p.start }

// End returns the frozen end tile.
func (p *Projectile) End() Location { return p.end }

// ProjectileID returns the projectile (spotanim) id.
func (p *Projectile) ProjectileID() int { return p.projectileID }

// LockonTargetIndex returns the resolved lock-on index
// (players negative, NPCs positive, 0 none).
func (p *Projectile) LockonTargetIndex() int { return p.lockonTargetIndex }

// SourceIndex returns the resolved source index (0 unless
// WithSource named an NPC).
func (p *Projectile) SourceIndex() int { return p.sourceIndex }

// Angle returns the arc slope at launch (16 unless set).
func (p *Projectile) Angle() int { return p.angle }

// Progress returns the start position within the source
// tile, in 1/128ths (64 unless set).
func (p *Projectile) Progress() int { return p.progress }

// CentreOf returns the centre tile of a mobile's footprint
// (centreOf, Projectile.ts:68-72): the south-west tile
// shifted by (size-1)>>1 in x and y. A 1x1 mobile's centre
// is its own tile.
func CentreOf(mobile ProjectileMobile) Location {
	loc := mobile.Location()
	size := mobile.Size()
	if size < 1 {
		size = 1
	}
	offset := (size - 1) >> 1
	if offset > 0 {
		return loc.Transform(offset, offset)
	}
	return loc
}

// locationOf resolves an arrival endpoint to a tile: a
// Location as-is, a mobile at its centre (locationOf,
// Projectile.ts:118-120).
func locationOf(at any) Location {
	switch v := at.(type) {
	case Location:
		return v
	case ProjectileMobile:
		return CentreOf(v)
	default:
		return Location{}
	}
}

// victimOf resolves an arrival endpoint to its victim
// mobile for flight-distance purposes, or nil for a bare
// tile (arrivalCycles passes the `to` endpoint as the
// victim only when it has a location, Projectile.ts:84-88).
func victimOf(at any) ProjectileMobile {
	if mobile, ok := at.(ProjectileMobile); ok {
		return mobile
	}
	return nil
}

// ArrivalCycles returns the cycles (20ms) between a
// projectile being sent and it landing (arrivalCycles,
// Projectile.ts:82-89): delay + flightDistance * perTile.
// TS defaults are delay 40, perTile 10; Go callers pass
// them explicitly. from/to are Locations or
// ProjectileMobiles.
func ArrivalCycles(from, to any, delay, perTile int) int {
	return delay + FlightDistance(locationOf(from), locationOf(to), victimOf(to))*perTile
}

// ArrivalTicks returns the same arrival in game ticks
// (arrivalTicks, Projectile.ts:95-97): ceil(cycles / 30).
// math.Ceil reproduces TS Math.ceil exactly, including
// for negative cycle counts.
func ArrivalTicks(from, to any, delay, perTile int) int {
	return int(math.Ceil(float64(ArrivalCycles(from, to, delay, perTile)) / 30))
}

// TicksToLand returns the ticks to queue a hit so its
// splat shows when a projectile ending at endCycle lands
// (ticksToLand, Projectile.ts:103-105): max(1,
// ceil(endCycle / 30)).
func TicksToLand(endCycle int) int {
	ticks := int(math.Ceil(float64(endCycle) / 30))
	if ticks < 1 {
		return 1
	}
	return ticks
}

// FlightDistance returns the horizontal tiles a projectile
// flies to the nearest tile of the target, ignoring floor
// delta (flightDistance, Projectile.ts:112-120). end is
// the target's CENTRE tile (callers centre it first); the
// footprint is reconstructed from the victim's size, and
// the distance is the Chebyshev gap to the footprint's
// nearest tile, or 0 when start is inside the footprint.
// A nil victim counts as size 1.
func FlightDistance(start, end Location, victim ProjectileMobile) int {
	size := 1
	if victim != nil && victim.Size() > 1 {
		size = victim.Size()
	}
	offset := (size - 1) >> 1
	west := end.X - offset
	south := end.Y - offset
	dx := 0
	if west-start.X > dx {
		dx = west - start.X
	}
	if start.X-(west+size-1) > dx {
		dx = start.X - (west + size - 1)
	}
	dy := 0
	if south-start.Y > dy {
		dy = south - start.Y
	}
	if start.Y-(south+size-1) > dy {
		dy = start.Y - (south + size - 1)
	}
	if dx > dy {
		return dx
	}
	return dy
}

// CreateProjectile builds a projectile fired by source at
// victim (createProjectile, Projectile.ts:122-142): from
// the source's centre to the victim's centre, locked onto
// the victim, in the source's area, naming the source.
// The across-deck branch degenerates (see the file
// header): across is always false without sailing.
func CreateProjectile(source, victim ProjectileMobile, projectileID, delay, speed int, startHeight, endHeight float64) *Projectile {
	return NewProjectile(
		CentreOf(source),
		CentreOf(victim),
		victim,
		projectileID,
		delay,
		speed,
		startHeight,
		endHeight,
		source.Area(),
	).WithSource(source)
}

// WithSource names the mobile firing the projectile
// (withSource, Projectile.ts:149-156). A player's
// projectile never names its caster (0 of ~450,000
// map_projanim sources in rsprox name one); NPCs keep
// theirs. It returns the projectile for chaining.
func (p *Projectile) WithSource(source ProjectileMobile) *Projectile {
	if source != nil && !source.IsPlayer() {
		p.sourceIndex = resolveLockonTargetIndex(source)
	} else {
		p.sourceIndex = 0
	}
	return p
}

// WithAngle sets the arc's slope at launch, clamped 0..255
// (withAngle, Projectile.ts:159-162).
func (p *Projectile) WithAngle(angle int) *Projectile {
	if angle < 0 {
		angle = 0
	}
	if angle > 255 {
		angle = 255
	}
	p.angle = angle
	return p
}

// WithProgress sets where along the source tile the shot
// starts, in 1/128ths, clamped 0..255 (withProgress,
// Projectile.ts:167-169).
func (p *Projectile) WithProgress(progress int) *Projectile {
	if progress < 0 {
		progress = 0
	}
	if progress > 255 {
		progress = 255
	}
	p.progress = progress
	return p
}

// WithEndDelay adds client cycles to where the shot lands,
// its start unchanged (withEndDelay, Projectile.ts:174-176).
// Negative values clamp to 0.
func (p *Projectile) WithEndDelay(cycles int) *Projectile {
	if cycles < 0 {
		cycles = 0
	}
	p.endDelay = cycles
	return p
}

// ProjectileTiming is the exact relative client-cycle
// deadlines sendProjectile uses (getTiming,
// Projectile.ts:179-189).
type ProjectileTiming struct {
	StartCycleOffset int
	EndCycleOffset   int
	LegacyMagic      bool
}

// GetTiming resolves the launch/landing cycle offsets
// (getTiming, Projectile.ts:179-189). The legacy magic
// shape (delay 0, speed 20, heights 43/31) normalizes to
// a launch at 51 and a landing at 46 + 10 per tile; any
// other shape launches at delay and lands at speed. The
// end delay is added to the end offset in both cases.
func (p *Projectile) GetTiming() ProjectileTiming {
	legacyMagic := p.delay == 0 && p.speed == 20 && p.startHeight == 43 && p.endHeight == 31
	start := p.delay
	end := p.speed
	if legacyMagic {
		start = 51
		end = LegacyMagicEndBase + FlightDistance(p.start, p.end, p.lockon)*10
	}
	return ProjectileTiming{
		StartCycleOffset: start,
		EndCycleOffset:   end + p.endDelay,
		LegacyMagic:      legacyMagic,
	}
}

// ProjectileSender is the optional SessionPort extension
// through which projectiles reach a player's client: the
// Go form of PacketSender.sendProjectile
// (PacketSender.ts:1236-1284), with the lock-on and
// source already resolved to signed indices (TS accepts
// pre-resolved integers for exactly this stability,
// resolveProjectileTargetIndex, :1293-1311). offset is
// accepted and ignored — TS declares it and never reads
// it. Heights are TS floats in quarter-tiles (the sender
// scales them x4); *session.Session implements it
// (internal/session/projectile_sender.go).
type ProjectileSender interface {
	SendProjectile(start, end Location, offset, speed, projectileID int, startHeight, endHeight float64, lockonIndex, delay, angle, distanceOffset, sourceIndex int)
}

// SendProjectile broadcasts the projectile to every viewer
// who can see it (sendProjectile, Projectile.ts:191-237).
// The gates, in TS order: network-player eligibility (bot
// + connected, see the file header), same area (with the
// boat syncArea fallback degenerated away), the start
// tile visible within max(15, NpcViewDistance), and —
// when locked on — the lock-on's current tile visible
// within the same range. Each recipient gets one
// sendProjectile call with the resolved timing (offset 0,
// the legacy-magic normalization applied, heights raw —
// the session sender scales them x4).
func (p *Projectile) SendProjectile() {
	timing := p.GetTiming()
	for _, player := range networkPlayers() {
		if player == nil || player.IsBot {
			continue
		}
		if player.Session == nil || !player.Session.IsOpen() {
			continue
		}
		if player.Area() != p.area {
			continue
		}
		viewer := player.Location()
		viewRange := player.NpcViewDistance()
		if viewRange < 15 {
			viewRange = 15
		}
		if !viewer.IsWithinDistance(p.start, viewRange) {
			continue
		}
		if p.lockon != nil && !viewer.IsWithinDistance(p.lockon.Location(), viewRange) {
			continue
		}
		p.sendTo(player, timing)
	}
}

// sendTo delivers one projectile copy through the player's
// session, dropping silently without a ProjectileSender
// session.
func (p *Projectile) sendTo(player *Player, timing ProjectileTiming) {
	if sender, ok := player.Session.(ProjectileSender); ok {
		sender.SendProjectile(
			p.start,
			p.end,
			0,
			timing.EndCycleOffset,
			p.projectileID,
			p.startHeight,
			p.endHeight,
			p.lockonTargetIndex,
			timing.StartCycleOffset,
			p.angle,
			p.progress,
			p.sourceIndex,
		)
	}
}
