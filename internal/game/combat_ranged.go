package game

import "math"

// RangedCombatMethod is the port of
// method/impl/RangedCombatMethod.ts: distance-based hit
// delays, per-shot ammo payment after the hits roll, and the
// ammo gate on attack and renewal. Start plays the attack
// animation, the launch/start graphics and (for NPCs) the
// draw graphic, fires the projectiles (Phase 29), and plays
// the fire sound (Worker C's picker, combat_sounds.go); the
// weapon/ammo graphic ids await the ranged-data tables
// (see RangedWeaponData / AmmunitionData).
//
// Ranged weapon/ammo DATA (RangedData.ts: the Ammunition and
// RangedWeapon registries, the WeaponProfiles tables) lives in
// ranged_data.go (tables in ranged_data_tables.go) and is
// registered in production by cmd/server InstallRangedData:
// with no resolver registered, no ranged weapon is ever
// detected and every fighter attacks in melee.

// HitDelayProfile is one WeaponProfile.ts HitDelayProfile
// (WeaponProfile.ts:17): a shot's hit-delay formula, delay =
// base + floor((distanceOffset + edgeDistance) / divisor).
type HitDelayProfile struct {
	Base           int
	DistanceOffset int
	Divisor        int
}

// RangedWeaponData is the core-visible half of a ranged
// weapon (RangedWeapon.getFor + WeaponProfiles.ranged): the
// fields the attack pipeline reads.
type RangedWeaponData struct {
	// UsesWeaponSlotAmmo: the weapon is its own ammo (thrown
	// weapons); the weapon slot pays, not the ammo slot.
	UsesWeaponSlotAmmo bool
	// AmmoIDs: the ammunition item ids this weapon accepts.
	AmmoIDs []int
	// AttackDistance: the weapon's attack distance (0 =
	// default 6; Longrange +2 capped at 10 is resolved by
	// the data that builds this struct).
	AttackDistance int
	// AmmoRequired: arrows per shot (0 = 1; a dark bow's 2).
	AmmoRequired int
	// HitDelayBase/HitDelayOffset/HitDelayDivisor: the
	// weapon's first hit-delay formula, delay = base +
	// floor((offset + edgeDistance) / divisor). All zero
	// means STANDARD_HIT (1, 3, 6): bows.
	HitDelayBase    int
	HitDelayOffset  int
	HitDelayDivisor int
	// HitDelayProfiles is the weapon's hit-delay formulas,
	// one per shot (WeaponProfile.ts hitDelays,
	// WeaponProfile.ts:166-171, hitDelays maps the whole
	// list). Empty means the single triple above, the
	// one-shot case. A multi-shot weapon carries one entry
	// per shot: the dark bow's second entry is
	// {1, 2, 3} (WeaponProfile.ts:295), so its second hit
	// lands on 1 + floor((2 + distance) / 3), not on the
	// first shot's STANDARD delay.
	HitDelayProfiles []HitDelayProfile
	// UsesMeleeStrength: the weapon's damage rolls off
	// Strength (some thrown weapons).
	UsesMeleeStrength bool
	// LaunchGraphicID is the weapon's own launch spotanim
	// (WeaponProfile.ts launchGraphic, e.g. the venator
	// bow's 2289), played instead of the ammunition's
	// start graphic (RangedCombatMethod.ts:143-144). 0
	// means none. Populated by the ranged-data tables
	// (ranged_data.go: rangedWeaponLaunchGraphicOverrides).
	LaunchGraphicID int
	// SuppressStartGraphic is WeaponProfile.ts
	// startGraphic: false (the blowpipe): no start
	// graphic plays even when the ammunition has one
	// (RangedCombatMethod.ts:145). Populated by the
	// ranged-data tables (ranged_data.go, BLOWPIPE
	// weaponType).
	SuppressStartGraphic bool
	// DoubleStartGraphic is WeaponProfile.ts
	// doubleStartGraphic (the dark bow): a two-arrow shot
	// plays the ammunition's double launch graphic
	// (RangedCombatMethod.ts:145). Populated by the
	// ranged-data tables (ranged_data.go, DARK_BOW).
	DoubleStartGraphic bool
	// ProjectileID is the weapon's own travel spotanim,
	// fired instead of its ammunition's (WeaponProfile.ts
	// projectileId, e.g. the venator bow's 2291). 0 means
	// the ammunition's projectile id.
	ProjectileID int
	// Projectiles is the weapon's projectile timing, one
	// entry per shot (WeaponProfile.ts projectiles; the dark
	// bow fires two). Empty means the default player
	// projectile (RangedCombatMethod.ts
	// DEFAULT_PLAYER_PROJECTILE).
	Projectiles []RangedProjectileProfile
}

// RangedProjectileProfile is one WeaponProfile.ts
// ProjectileProfile: the shot's client-cycle timing
// (projectileEnd: delay + lengthAdjustment + stepMultiplier
// * edge distance) and heights in quarter-tiles as TS
// floats (WeaponProfile.ts:18, 60-65: 40.75, 36.5, ... —
// the session sender scales them x4, so a bow sends
// 163/146 exactly). Angle and Progress are 0 when unset
// (the packet defaults, 16 and 64, apply).
type RangedProjectileProfile struct {
	Delay            int
	LengthAdjustment int
	StepMultiplier   int
	StartHeight      float64
	EndHeight        float64
	Angle            int
	Progress         int
}

// AmmunitionData is the core-visible half of an ammunition
// entry (Ammunition.getFor). Identity matters: CheckAmmo
// compares the ammo data pointer with the weapon's list.
type AmmunitionData struct {
	// ItemID is the ammunition item id.
	ItemID int
	// ProjectileID is the ammunition's travel spotanim id
	// (Ammunition's projectileId, the constructor's third
	// argument in RangedData.ts): the projectile a shot of
	// this ammunition fires, unless the weapon overrides it.
	ProjectileID int
	// StartGraphicID is the ammunition's launch spotanim
	// (Ammunition's startGfx, RangedData.ts — the
	// <ammo>_arrow_launch graphic, e.g. bronze arrow 19),
	// played on the shot's tick (RangedCombatMethod.ts:
	// 145-146). 0 means none (bolts carry none).
	// Populated by the ranged-data tables
	// (ranged_data_tables.go, script-extracted from
	// RangedData.ts).
	StartGraphicID int
	// DoubleStartGraphicID is the ammunition's two-arrow
	// launch spotanim (Ammunition's doubleStartGfx, e.g.
	// bronze arrow 1104), played when a weapon fires two
	// arrows at once (RangedCombatMethod.ts:145). 0 means
	// none (the single start graphic plays instead).
	// Populated by the ranged-data tables
	// (ranged_data_tables.go, script-extracted from
	// RangedData.ts).
	DoubleStartGraphicID int
	// Strength is the ammunition's ranged strength
	// (Ammunition's strength, the constructor's fourth
	// argument; getStrength, RangedData.ts:380-382).
	// Script-extracted with the rest of the kind (see
	// ranged_data_tables.go). It has no engine consumer:
	// TS never calls getStrength() in the engine at bfd81e3
	// and the ranged max hit reads the worn equipment's
	// ranged-strength bonus instead (BonusManager.ts:57-71),
	// so no formula reads this field; it rides along for
	// shape completeness and future content.
	Strength int
}

// rangedDataResolver, when set, resolves a player's ranged
// weapon and ammunition (RangedWeapon.getFor /
// Ammunition.getFor). Nil means no ranged weapon.
var rangedDataResolver func(player *Player) (weapon *RangedWeaponData, ammo *AmmunitionData)

// RegisterRangedDataResolver installs the ranged-data
// resolver. The coordinator/content owns the weapon and
// ammo tables; core only reads through this seam.
func RegisterRangedDataResolver(resolver func(player *Player) (*RangedWeaponData, *AmmunitionData)) {
	rangedDataResolver = resolver
}

// resolveRangedData refreshes the fighter's ranged weapon
// and ammunition from the resolver (CombatFactory.getMethod
// / RangedCombatMethod.shouldRenew), setting them when they
// changed.
func resolveRangedData(player *Player) {
	if player == nil {
		return
	}
	combat := player.GetCombat()
	var weapon *RangedWeaponData
	var ammo *AmmunitionData
	if rangedDataResolver != nil {
		weapon, ammo = rangedDataResolver(player)
	}
	if combat.RangedWeapon() != weapon {
		combat.SetRangedWeapon(weapon)
	}
	if combat.Ammunition() != ammo {
		combat.SetAmmunition(ammo)
	}
}

// ammunitionDataForItem resolves the ammunition data for an
// item id, for the CheckAmmo compatibility test. It reads
// through the resolver's view: with no resolver there is no
// ammunition data. Content that registers a resolver should
// also register ammunition data via RegisterAmmunitionData.
var ammunitionDataByItem = map[int]*AmmunitionData{}

// RegisterAmmunitionData registers ammunition data by item id.
func RegisterAmmunitionData(data *AmmunitionData) {
	if data != nil {
		ammunitionDataByItem[data.ItemID] = data
	}
}

func ammunitionDataForItem(itemID int) *AmmunitionData {
	return ammunitionDataByItem[itemID]
}

// rangedFiresFromAmmoSlot is Ammunition.firesFromAmmoSlot:
// the shot draws ammo from the ammo slot (normal bows).
// Weapon-slot ammo (thrown weapons) and plugin-supplied
// ammo read false. The plugin-ammo half is not ported (no
// plugin ammo system); the weapon-slot half is exact.
func rangedFiresFromAmmoSlot(player *Player) bool {
	if player == nil {
		return false
	}
	weapon := player.GetCombat().RangedWeapon()
	return weapon != nil && !weapon.UsesWeaponSlotAmmo
}

// rangedUsesMeleeStrength reports whether the player's
// ranged weapon rolls damage off Strength
// (WeaponProfiles usesMeleeStrength).
func rangedUsesMeleeStrength(player *Player) bool {
	if player == nil {
		return false
	}
	weapon := player.GetCombat().RangedWeapon()
	return weapon != nil && weapon.UsesMeleeStrength
}

// projectileFlightDistance is Projectile.flightDistance: the
// distance from the attacker's centre tile to the target's
// centre box (the size offset (size-1)>>1), in tiles.
//
// TS (Projectile.ts:82-89, 112-120): arrivalCycles centres BOTH
// ends (locationOf = centreOf) and flightDistance measures the
// Chebyshev gap from the centred start to the target footprint
// reconstructed from the centre end: west = end.X - offset and
// the east edge is west + size - 1 (size clamped to >= 1). The
// east edge is size - 1, NOT offset * 2: for even sizes offset =
// (size-1)>>1 loses a tile — size 2: offset 0, east = west + 1,
// not west + 0; size 4: offset 1, east = west + 3, not west + 2.
// (The canonical FlightDistance in projectile.go already uses
// size - 1; this duplicate used offset * 2 and centred neither
// end, overcounting by one tile for even-sized targets east or
// north of the attacker and for size >= 3 attackers. Phase 26
// aligns it with TS and the canonical form; the two now agree
// on every size. The only remaining difference from the
// canonical helper is the input shape: Mobiles here, centred
// Locations there.)
func projectileFlightDistance(from, to *Mobile) int {
	if from == nil || to == nil {
		return 0
	}
	size := combatantFor(to).Size()
	if size < 1 {
		size = 1
	}
	offset := (size - 1) >> 1
	fromSize := combatantFor(from).Size()
	if fromSize < 1 {
		fromSize = 1
	}
	fromOffset := (fromSize - 1) >> 1
	fromLoc := from.Location()
	startX := fromLoc.X + fromOffset
	startY := fromLoc.Y + fromOffset
	toLoc := to.Location()
	west := toLoc.X - offset
	south := toLoc.Y - offset
	dx := 0
	if west-startX > dx {
		dx = west - startX
	}
	if startX-(west+size-1) > dx {
		dx = startX - (west + size - 1)
	}
	dy := 0
	if south-startY > dy {
		dy = south - startY
	}
	if startY-(south+size-1) > dy {
		dy = startY - (south + size - 1)
	}
	if dx > dy {
		return dx
	}
	return dy
}

// projectileArrivalTicks is Projectile.arrivalTicks: ticks
// until a projectile with end base and cycles-per-tile
// lands (ceil of end cycles / 30).
func projectileArrivalTicks(attacker, target *Mobile, endBase, cyclesPerTile int) int {
	endCycles := endBase + projectileFlightDistance(attacker, target)*cyclesPerTile
	return int(math.Ceil(float64(endCycles) / 30))
}

// RangedMethod is the shared ranged combat method singleton
// (CombatFactory.RANGED_COMBAT).
var RangedMethod CombatMethod = &rangedCombatMethod{}

type rangedCombatMethod struct{ BaseCombatMethod }

func (m *rangedCombatMethod) Type() CombatType { return CombatTypeRanged }

// playerHitDelays is RangedCombatMethod.playerHitDelays
// (same-floor): the weapon's formulas over the edge
// distance — one delay per profile entry
// (WeaponProfile.ts:166-171, hitDelays maps the whole
// list), so a dark bow's two shots land on their own
// delays. The cross-floor branch is skipped (no CrossFloor).
func playerHitDelays(attacker, target *Mobile) []int {
	weapon := attacker.GetCombat().RangedWeapon()
	distance := EdgeDistance(attacker, target)
	if weapon != nil && len(weapon.HitDelayProfiles) > 0 {
		delays := make([]int, 0, len(weapon.HitDelayProfiles))
		for _, profile := range weapon.HitDelayProfiles {
			base, offset, divisor := profile.Base, profile.DistanceOffset, profile.Divisor
			if divisor <= 0 {
				// A malformed entry carries no formula:
				// STANDARD_HIT, the single-triple
				// fallback's all-zero reading.
				base, offset, divisor = 1, 3, 6
			}
			delays = append(delays, base+(offset+distance)/divisor)
		}
		return delays
	}
	base, offset, divisor := 1, 3, 6
	if weapon != nil && weapon.HitDelayDivisor > 0 {
		base, offset, divisor = weapon.HitDelayBase, weapon.HitDelayOffset, weapon.HitDelayDivisor
	}
	return []int{base + (offset+distance)/divisor}
}

// npcHitDelay is RangedCombatMethod.npcHitDelay: an NPC's
// shot flies like a bow's arrow (end base 46, 5 cycles a
// tile) and hits when it lands.
func npcHitDelay(attacker, target *Mobile) int {
	return projectileArrivalTicks(attacker, target, 46, 5)
}

// arrowsPerShot is RangedCombatMethod.arrowsPerShot: the
// profile's count (a dark bow's two), capped by what is
// left in the ammo slot. The plugin-ammo branch reads the
// plugin check in TS; in Go it keeps the wanted count (no
// plugin ammo system).
func arrowsPerShot(player *Player) int {
	wanted := 1
	if weapon := player.GetCombat().RangedWeapon(); weapon != nil && weapon.AmmoRequired > 0 {
		wanted = weapon.AmmoRequired
	}
	if wanted <= 1 {
		return wanted
	}
	if !rangedFiresFromAmmoSlot(player) {
		return wanted
	}
	worn := 0
	if ammo := player.Equipment().ItemAt(AmmunitionSlot); ammo.IsValid() {
		worn = ammo.Amount()
	}
	shot := wanted
	if worn < shot {
		shot = worn
	}
	if shot < 1 {
		shot = 1
	}
	return shot
}

// shotLimit is RangedCombatMethod.shotLimit: how many of the
// profile's hits one attack keeps; -1 means all (the
// single-arrow case).
func shotLimit(player *Player) int {
	wanted := 1
	if weapon := player.GetCombat().RangedWeapon(); weapon != nil && weapon.AmmoRequired > 0 {
		wanted = weapon.AmmoRequired
	}
	if wanted > 1 {
		return arrowsPerShot(player)
	}
	return -1
}

// Hits is RangedCombatMethod.hits: one PendingHit per shot,
// each with its landing delay.
func (m *rangedCombatMethod) Hits(attacker, target *Mobile) []*PendingHit {
	var delays []int
	if combatantFor(attacker).IsPlayer() {
		delays = playerHitDelays(attacker, target)
		if limit := shotLimit(combatantFor(attacker).Player()); limit >= 0 && len(delays) > limit {
			delays = delays[:limit]
		}
	} else {
		delays = []int{npcHitDelay(attacker, target)}
	}
	hits := make([]*PendingHit, 0, len(delays))
	for _, delay := range delays {
		hits = append(hits, NewPendingHit(attacker, target, m, delay))
	}
	return hits
}

// CanAttack is RangedCombatMethod.canAttack: NPCs always;
// players must pass the ammo gate.
func (m *rangedCombatMethod) CanAttack(attacker, target *Mobile) bool {
	if !combatantFor(attacker).IsPlayer() {
		return true
	}
	return CheckAmmo(combatantFor(attacker).Player(), 1, false)
}

// ShouldRenew is RangedCombatMethod.shouldRenew: re-resolve
// the weapon/ammo, then the ammo gate without resetting.
func (m *rangedCombatMethod) ShouldRenew(attacker, target *Mobile) bool {
	if !combatantFor(attacker).IsPlayer() {
		return true
	}
	player := combatantFor(attacker).Player()
	resolveRangedData(player)
	if player.GetCombat().RangedWeapon() == nil {
		return false
	}
	return CheckAmmo(player, 1, true)
}

// pendingShot is one fired shot waiting to be paid
// (RangedCombatMethod.pendingShots): paid in Finished, on
// combat end, or at the next Start — after the swing's hits
// roll with the ammo still worn.
type pendingShot struct {
	pos         Location
	amount      int
	flightTicks int
}

var pendingShots = map[*Mobile]pendingShot{}

// npcLaunchGraphicDef is the NPC-definition surface
// RangedCombatMethod's draw graphic reads: an NPC's
// captured draw spotanim (NpcDefinition.ts
// getLaunchGraphicId). *definition.NpcDefinition carries
// it (LaunchGraphicID); the combat definition interface
// does not, so the read is an assertion — a definition
// without one draws nothing.
type npcLaunchGraphicDef interface {
	LaunchGraphicID() int
}

// Start pays any earlier unpaid shot, fires the shot's
// projectile(s), then records this shot for payment in
// Finished. The attack animation, the launch/start
// graphics and (for NPCs) the draw graphic play here
// (RangedCombatMethod.ts:133-158), and the fire sound
// (Worker C's picker, FireSoundFor).
//
// Projectiles (RangedCombatMethod.ts start()): an NPC fires
// its definition's configured projectile, or the generic
// arrow (DEFAULT_NPC_PROJECTILE 10) when none is configured,
// at delay 41 with end 46 + 5 cycles a tile and heights
// 43/31. A player fires one projectile per profile entry
// (sliced to the arrows fired, so a dark bow looses two),
// with the weapon's projectile id or the ammunition's, the
// profile's delay and end (projectileEnd: delay +
// lengthAdjustment + stepMultiplier * edge distance), from
// the attacker's centre to the target's centre, locked onto
// the target. No weapon or ammunition means no projectile.
func (m *rangedCombatMethod) Start(attacker, target *Mobile) {
	payPendingShot(attacker)
	// RangedCombatMethod.start (RangedCombatMethod.ts:136-138):
	// the attack animation, skipped when -1.
	if anim := attacker.GetAttackAnim(target); anim != -1 {
		attacker.PerformAnimation(anim, 0)
	}
	if !combatantFor(attacker).IsPlayer() {
		// An archer draws the arrow on its bow on the
		// shot's tick (RangedCombatMethod.ts:156-160):
		// only NPCs whose draw spotanim was captured carry
		// one (npc-combat-defs.json `launchGraphic`).
		if npc := combatantFor(attacker).NPC(); npc != nil {
			if def, ok := npc.Definition().(npcLaunchGraphicDef); ok {
				if id := def.LaunchGraphicID(); id >= 0 {
					attacker.PerformGraphic(id, graphicHeightArcherDraw, 0)
				}
			}
		}
		fireNPCProjectile(attacker, target)
		return
	}
	player := combatantFor(attacker).Player()
	arrows := arrowsPerShot(player)
	rangedStartGraphic(attacker, arrows)
	processingOrder := 0
	if !combatantFor(target).IsPlayer() {
		processingOrder = 1
	}
	flightTicks := 1
	for _, delay := range playerHitDelays(attacker, target) {
		if delay+processingOrder > flightTicks {
			flightTicks = delay + processingOrder
		}
	}
	pendingShots[attacker] = pendingShot{pos: target.Location(), amount: arrows, flightTicks: flightTicks}
	firePlayerProjectiles(attacker, target, arrows)
	// The fire sound (RangedCombatMethod.ts:175-176): the
	// weapon's singleFireSound when one arrow looses, else
	// its fireSound, else SHOOT_ARROW (Worker C's picker).
	// NPC shots play no fire sound here (the NPC branch
	// returns above). A special attack's fire sound
	// replaces the ordinary fire sound (phase 30, item 5).
	if !PlaySpecialFireSound(attacker) {
		sendCombatSound(attacker, FireSoundFor(attacker))
	}
}

// rangedStartGraphic plays the shot's launch graphic
// (RangedCombatMethod.ts:143-146): the weapon's own
// launchGraphic when the profile carries one and
// ammunition is worn, else the ammunition's start
// graphic — the double form when the weapon fires two
// arrows — unless the profile suppresses start graphics.
// A weapon or ammunition the tables have not populated
// plays nothing: the ids default to 0 (none), so the
// shot still fires its projectile.
func rangedStartGraphic(attacker *Mobile, arrows int) {
	combat := attacker.GetCombat()
	weapon := combat.RangedWeapon()
	ammo := combat.Ammunition()
	if ammo == nil {
		return
	}
	if weapon != nil && weapon.LaunchGraphicID > 0 {
		attacker.PerformGraphic(weapon.LaunchGraphicID, graphicHeightLow, 0)
		return
	}
	if weapon != nil && weapon.SuppressStartGraphic {
		return
	}
	id := ammo.StartGraphicID
	if weapon != nil && weapon.DoubleStartGraphic && arrows > 1 && ammo.DoubleStartGraphicID > 0 {
		id = ammo.DoubleStartGraphicID
	}
	if id > 0 {
		attacker.PerformGraphic(id, graphicHeightHigh, 0)
	}
}

// fireNPCProjectile fires one NPC ranged shot
// (RangedCombatMethod.ts start(), the isNpc branch). The
// launch (draw) graphic plays in Start, before this call.
func fireNPCProjectile(attacker, target *Mobile) {
	source := projectileMobile(attacker)
	victim := projectileMobile(target)
	if source == nil || victim == nil {
		return
	}
	projectileID := 10
	if def := combatantFor(attacker).NPCDef(); def != nil && def.ProjectileID() >= 0 {
		projectileID = def.ProjectileID()
	}
	end := 46 + projectileFlightDistance(attacker, target)*5
	CreateProjectile(source, victim, projectileID, 41, end, 43, 31).SendProjectile()
}

// firePlayerProjectiles fires one projectile per profile
// entry of the player's ranged weapon, sliced to the arrows
// fired (RangedCombatMethod.ts start(): projectiles sliced
// to the shot limit). A weapon or ammunition the resolver
// did not resolve fires nothing.
func firePlayerProjectiles(attacker, target *Mobile, arrows int) {
	player := combatantFor(attacker).Player()
	if player == nil {
		return
	}
	combat := player.GetCombat()
	weapon := combat.RangedWeapon()
	ammo := combat.Ammunition()
	if weapon == nil || ammo == nil {
		return
	}
	profiles := weapon.Projectiles
	if len(profiles) == 0 {
		profiles = []RangedProjectileProfile{rangedDefaultProjectile}
	}
	limit := len(profiles)
	if weapon.AmmoRequired > 1 && arrows < limit {
		limit = arrows
	}
	source := projectileMobile(attacker)
	victim := projectileMobile(target)
	if source == nil || victim == nil {
		return
	}
	projectileID := weapon.ProjectileID
	if projectileID == 0 {
		projectileID = ammo.ProjectileID
	}
	distance := EdgeDistance(attacker, target)
	if distance < 0 {
		distance = 0
	}
	for i := 0; i < limit; i++ {
		profile := profiles[i]
		end := profile.Delay + profile.LengthAdjustment + profile.StepMultiplier*distance
		shot := CreateProjectile(source, victim, projectileID, profile.Delay, end, profile.StartHeight, profile.EndHeight)
		if profile.Angle != 0 {
			shot.WithAngle(profile.Angle)
		}
		if profile.Progress != 0 {
			shot.WithProgress(profile.Progress)
		}
		shot.SendProjectile()
	}
}

// projectileMobile resolves a Mobile to the ProjectileMobile
// a projectile is fired from or locked onto: the bound
// player or NPC. An unbound Mobile fires nothing.
func projectileMobile(mobile *Mobile) ProjectileMobile {
	if mobile == nil {
		return nil
	}
	if player := combatantFor(mobile).Player(); player != nil {
		return player
	}
	if npc := combatantFor(mobile).NPC(); npc != nil {
		return npc
	}
	return nil
}

// Finished pays the pending shot (after the hits rolled).
func (m *rangedCombatMethod) Finished(attacker, target *Mobile) {
	payPendingShot(attacker)
}

// OnCombatEnded pays a shot stopped between Start and
// Finished (no hits): the shot still fired.
func (m *rangedCombatMethod) OnCombatEnded(attacker *Mobile) {
	payPendingShot(attacker)
}

func payPendingShot(attacker *Mobile) {
	shot, ok := pendingShots[attacker]
	if !ok {
		return
	}
	delete(pendingShots, attacker)
	if combatantFor(attacker).IsPlayer() {
		DecrementAmmo(combatantFor(attacker).Player(), shot.pos, shot.amount, shot.flightTicks)
	}
}

// AttackDistance is RangedCombatMethod.attackDistance: the
// weapon profile's distance (fallback 6), 6 for NPCs.
func (m *rangedCombatMethod) AttackDistance(attacker *Mobile) int {
	if combatantFor(attacker).IsPlayer() {
		if weapon := attacker.GetCombat().RangedWeapon(); weapon != nil && weapon.AttackDistance > 0 {
			return weapon.AttackDistance
		}
		return 6
	}
	return 6
}
