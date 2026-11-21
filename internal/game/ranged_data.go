package game

import "strings"

// Ranged weapon and ammunition data: the port of
// RangedData.ts (RangedWeapon.getFor / Ammunition.getFor, the
// RangedWeapon / Ammunition registries) and the WeaponProfile.ts
// ranged rows (WeaponProfiles.ranged / attackDistance /
// hitDelays) behind the resolver seam in combat_ranged.go.
// The tables themselves live in ranged_data_tables.go
// (script-extracted from the TS literals); this file builds the
// lookup maps, resolves a player's weapon + ammunition from
// equipment, and computes the attack distance in the TS order.
//
// RESOLUTION (RangedData.ts, Ammunition.getFor /
// RangedWeapon.getFor): the weapon slot's item decides the
// weapon; a self-ammo weapon (one ammunition entry whose item
// id is one of the weapon ids — CombatFactory.usesWeaponSlotAmmo,
// CombatFactory.ts:1952-1965, and the selfAmmoByWeapon rule in
// the RangedWeapon constructor) is its own ammunition; every
// other weapon draws its ammunition from the ammo slot, looked
// up in the same ammunition map (so a poisoned arrow resolves
// to its plain form's Ammunition object, as the variants static
// block maps it). Plugin-supplied ammunition
// (PluginManager.resolveRangedAmmunition) has no Go form: the
// blowpipe's scales and Dizana's quiver stay unported, so the
// resolver reads equipment only. A weapon with no table entry
// resolves to nil (melee fallback preserved, exactly as a
// missing RangedWeapon entry falls back to melee in TS).
//
// ATTACK DISTANCE (WeaponProfile.ts attackDistance, :134-154):
// a per-item profile distance, then the cache's attack range
// (ItemDefinition.ATTACK_RANGE_PARAM), then the weapon
// interface's profile distance, then the 6-tile fallback; the
// Longrange style adds 2 tiles, capped at 10. The two per-item
// registrations (crystal bow 10, ascension crossbow 6,
// WeaponProfile.ts:303-311, 365-381) equal the static values
// below, so no per-item map is kept: the resolver reads the
// cache range when a WeaponInfoSource is installed (the
// production Store; combat_method.go:133 is the precedent) and
// falls back to the static interface distance otherwise (unit
// tests, NoCache mode). Longrange is resolved from the
// fighter's current FightType name against the weapon type's
// long-range style; Go has only UNARMED_PUNCH today (the
// style system is Worker B's slice), so it never adds tiles
// yet — the check is written so it starts working when fight
// types land.
//
// HIT DELAYS (WeaponProfile.ts hitDelays, :164-171): the
// profile's formulas, base + floor((offset + distance) /
// divisor), over the edge distance — one per shot. The dark
// bow carries two (STANDARD_HIT, then (1, 2, 3),
// WeaponProfile.ts:290-301), and RangedWeaponData carries
// the list in HitDelayProfiles so both of its hits land on
// their own delay.
//
// PROJECTILES (WeaponProfile.ts projectile profiles, :60-65,
// and RangedCombatMethod.ts start(), DEFAULT_PLAYER_PROJECTILE
// :25): the weapon type's projectile timing. Heights are
// quarter-tiles in TS as floats (40.75, 36.5 —
// WeaponProfile.ts:58-59: PacketSender sends them times
// four, so arrows send 163 -> 146); Go carries the same
// floats, so the sent heights are exact, not rounded to
// the nearest quarter-tile. UsesMeleeStrength is false for
// every weapon: no profile in WeaponProfile.ts sets
// rangedDamageSource (only the field declaration :53 and
// the check :101-104 exist), so the RangedWeaponData flag
// is never set by this table.
//
// AMMUNITION STRENGTH is carried on AmmunitionData (Strength,
// script-extracted from the Ammunition constructor's fourth
// argument; see ranged_data_tables.go): it has no engine
// consumer — TS's getStrength() is never called by the engine
// at bfd81e3, and the ranged max hit reads the worn
// equipment's ranged-strength bonus instead — so no formula
// reads it.

// rangedTypeProfile is one RangedWeaponType's WeaponProfile.ts
// row: the interface attack distance, the hit-delay formula,
// the projectile timing, and the Longrange fight-type name
// ("" when the type has none).
type rangedTypeProfile struct {
	attackDistance     int
	hitDelayBase       int
	hitDelayOffset     int
	hitDelayDivisor    int
	projectiles        []RangedProjectileProfile
	longRangeFightType string
}

// rangedDefaultProjectile is RangedCombatMethod's
// DEFAULT_PLAYER_PROJECTILE (RangedCombatMethod.ts:25): delay
// 41, length adjustment 5, 5 cycles a tile, heights 43/31.
var rangedDefaultProjectile = RangedProjectileProfile{Delay: 41, LengthAdjustment: 5, StepMultiplier: 5, StartHeight: 43, EndHeight: 31}

// rangedBowProjectile is WeaponProfile.ts DEFAULT_PROJECTILE
// (:60): delay 41, length adjustment 5, 5 cycles a tile,
// heights 40.75/36.5 (sent 163/146).
var rangedBowProjectile = RangedProjectileProfile{Delay: 41, LengthAdjustment: 5, StepMultiplier: 5, StartHeight: 40.75, EndHeight: 36.5}

// rangedCrossbowProjectile is WeaponProfile.ts
// CROSSBOW_PROJECTILE (:65): as the bow's, heights 38.75/36.5
// (sent 155/146), angle 5, progress 11.
var rangedCrossbowProjectile = RangedProjectileProfile{Delay: 41, LengthAdjustment: 5, StepMultiplier: 5, StartHeight: 38.75, EndHeight: 36.5, Angle: 5, Progress: 11}

// rangedThrownProjectile is WeaponProfile.ts THROWN_PROJECTILE
// (:61): delay 32, no length adjustment, 5 cycles a tile,
// heights 40.75/36.5 (sent 163/146).
var rangedThrownProjectile = RangedProjectileProfile{Delay: 32, LengthAdjustment: 0, StepMultiplier: 5, StartHeight: 40.75, EndHeight: 36.5}

// rangedDartProjectile is WeaponProfile.ts DART_PROJECTILE
// (:62): as the thrown one, heights 35.5/36.5 (sent 142/146).
var rangedDartProjectile = RangedProjectileProfile{Delay: 32, LengthAdjustment: 0, StepMultiplier: 5, StartHeight: 35.5, EndHeight: 36.5}

// rangedBallistaProjectile is the WeaponProfile.ts BALLISTA
// row (:278-289): delay 42, length adjustment 2, 3 cycles a
// tile, heights 38.75/44.25 (sent 155/177), angle 14,
// progress 120.
var rangedBallistaProjectile = RangedProjectileProfile{Delay: 42, LengthAdjustment: 2, StepMultiplier: 3, StartHeight: 38.75, EndHeight: 44.25, Angle: 14, Progress: 120}

// rangedVenatorProjectile is the WeaponProfile.ts VENATOR_BOW
// register (:333-342): delay 31, length adjustment 15, 2
// cycles a tile, heights 39/36.5 (sent 156/146), angle 255,
// progress 11.
var rangedVenatorProjectile = RangedProjectileProfile{Delay: 31, LengthAdjustment: 15, StepMultiplier: 2, StartHeight: 39, EndHeight: 36.5, Angle: 255, Progress: 11}

// rangedTypeProfiles maps each RangedWeaponType name to its
// WeaponProfile.ts row. Distances are the weapon-interface
// profile's attackDistance (WeaponProfile.ts:182-301), not the
// RangedWeaponType defaults — getDefaultDistance has no
// callers in TS, and attackDistance reads the interface
// profile (WeaponProfile.ts:134-154). Where the interface
// profile carries no distance, the value is the cache range
// the WeaponProfile.ts comments state (venator 6, scorching
// 10, chinchompa 9, gauntlet/twisted/faerdhinen 10) or the
// type default (glaive 5, morrigans javelin 5); the resolver's
// cache lookup overrides all of these in production. Hit
// delays are the interface rows: STANDARD_HIT (1, 3, 6) for
// bows/crossbows (WeaponProfile.ts:69, 182-218), (1, 0, 6) for
// thrown weapons (BLOWPIPE/KNIFE/DART/JAVELIN/OBBY/THROWNAXE/
// CHINCHOMPA, :220-275, 411-424), (2, 1, 6) for ballistae
// (:278-289), (0, 23, 15) for the venator (:333-342).
var rangedTypeProfiles = map[string]rangedTypeProfile{
	// KNIFE (WeaponProfile.ts:231-240): 4 tiles.
	"KNIFE": {attackDistance: 4, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedThrownProjectile}, longRangeFightType: "KNIFE_LONGRANGE"},
	// THROWNAXE (WeaponProfile.ts:269-275): 4 tiles.
	"THROWNAXE": {attackDistance: 4, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedThrownProjectile}, longRangeFightType: "THROWNAXE_LONGRANGE"},
	// GLAIVE: no WeaponInterfaces row, so get() is nil and
	// WeaponProfiles.ranged falls back to DEFAULT_RANGED
	// (WeaponProfile.ts:70-76, 97): STANDARD_HIT, DEFAULT.
	// The type default is 5 (RangedData.ts, GLAIVE).
	"GLAIVE": {attackDistance: 5, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}},
	// DART (WeaponProfile.ts:241-250): 3 tiles.
	"DART": {attackDistance: 3, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedDartProjectile}, longRangeFightType: "DART_LONGRANGE"},
	// TOKTZ_XIL_UL: the OBBY_RINGS interface row
	// (WeaponProfile.ts:261-268) reaches 7 tiles, not the
	// type default 5.
	"TOKTZ_XIL_UL": {attackDistance: 7, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedThrownProjectile}, longRangeFightType: "OBBY_RING_LONGRANGE"},
	// MORRIGANS_JAVELIN: the JAVELIN interface row
	// (WeaponProfile.ts:251-260): 5 tiles.
	"MORRIGANS_JAVELIN": {attackDistance: 5, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedThrownProjectile}, longRangeFightType: "JAVELIN_LONGRANGE"},
	// CHINCHOMPA: the item registers (WeaponProfile.ts:411-424)
	// carry no attackDistance; the comment states the cache's
	// 9-tile range.
	"CHINCHOMPA": {attackDistance: 9, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedThrownProjectile}, longRangeFightType: "CHINCHOMPA_LONG_FUSE"},
	// SALAMANDER: the interface row (WeaponProfile.ts:426)
	// carries only attackDistance 1, so hitDelays falls back
	// to STANDARD_HIT (WeaponProfile.ts:164-171) and
	// projectiles to DEFAULT_PLAYER_PROJECTILE
	// (RangedCombatMethod.ts:25, 168-171).
	"SALAMANDER": {attackDistance: 1, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedDefaultProjectile}},
	// LONGBOW (WeaponProfile.ts:192-198): the interface row
	// reaches 10 tiles, not the type default 9.
	"LONGBOW": {attackDistance: 10, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "LONGBOW_LONGRANGE"},
	// TWISTED_BOW: the item register (WeaponProfile.ts:320-327)
	// carries no attackDistance; the type default is 10.
	"TWISTED_BOW": {attackDistance: 10, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "LONGBOW_LONGRANGE"},
	// VENATOR_BOW: the item register (WeaponProfile.ts:333-342)
	// carries no attackDistance; the comment states the cache's
	// 6-tile range (8 on Longrange).
	"VENATOR_BOW": {attackDistance: 6, hitDelayBase: 0, hitDelayOffset: 23, hitDelayDivisor: 15, projectiles: []RangedProjectileProfile{rangedVenatorProjectile}, longRangeFightType: "SHORTBOW_LONGRANGE"},
	// SCORCHING_BOW: the item register (WeaponProfile.ts:343-350)
	// carries no attackDistance; the comment states the cache's
	// 10-tile range.
	"SCORCHING_BOW": {attackDistance: 10, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "LONGBOW_LONGRANGE"},
	// BLOWPIPE (WeaponProfile.ts:220-230): 5 tiles.
	"BLOWPIPE": {attackDistance: 5, hitDelayBase: 1, hitDelayOffset: 0, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedDartProjectile}, longRangeFightType: "BLOWPIPE_LONGRANGE"},
	// SHORTBOW (WeaponProfile.ts:182-191): 7 tiles.
	"SHORTBOW": {attackDistance: 7, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "SHORTBOW_LONGRANGE"},
	// CRYSTAL_BOW: the item register (WeaponProfile.ts:303-311)
	// sets attackDistance 10.
	"CRYSTAL_BOW": {attackDistance: 10, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "SHORTBOW_LONGRANGE"},
	// GAUNTLET_BOW: no item register; the type default is 10.
	"GAUNTLET_BOW": {attackDistance: 10, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBowProjectile}, longRangeFightType: "LONGBOW_LONGRANGE"},
	// CROSSBOW (WeaponProfile.ts:199-210): 7 tiles.
	"CROSSBOW": {attackDistance: 7, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedCrossbowProjectile}, longRangeFightType: "CROSSBOW_LONGRANGE"},
	// ASCENSION_CROSSBOWS: the item register
	// (WeaponProfile.ts:365-381) sets attackDistance 6.
	"ASCENSION_CROSSBOWS": {attackDistance: 6, hitDelayBase: 1, hitDelayOffset: 3, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedCrossbowProjectile}, longRangeFightType: "CROSSBOW_LONGRANGE"},
	// BALLISTA (WeaponProfile.ts:278-289): the interface row
	// reaches 9 tiles, not the type default 7.
	"BALLISTA": {attackDistance: 9, hitDelayBase: 2, hitDelayOffset: 1, hitDelayDivisor: 6, projectiles: []RangedProjectileProfile{rangedBallistaProjectile}, longRangeFightType: "BALLISTA_LONGRANGE"},
}

// rangedWeaponDistanceOverrides is the per-entry attack
// distance where the weapon's interface row differs from its
// RangedWeaponType row (WeaponProfile.ts:134-154 reads the
// interface row first): Karil's crossbow is the KARILS
// interface (8, :211-219), not CROSSBOW (7); the comp ogre
// bow's cache range is 5 and the composite bows' 12, cut to
// 10 by MAX_ATTACK_RANGE (comment at :393-402); the 3rd Age
// bow's cache range is 9 (:382-392); blisterwood stakes'
// cache range is 6 (:429-438). Everything else follows its
// type row above.
var rangedWeaponDistanceOverrides = map[string]int{
	"KARILS_CROSSBOW":   8,
	"WILLOW_COMP_BOW":   10,
	"YEW_COMP_BOW":      10,
	"MAGIC_COMP_BOW":    10,
	"COMP_OGRE_BOW":     5,
	"THIRD_AGE_BOW":     9,
	"BLISTERWOOD_STAKE": 6,
}

// rangedWeaponProjectileOverrides is the per-entry weapon
// projectile id (WeaponProfile.ts projectileId, read as
// profile?.projectileId ?? ammo.getProjectileId() in
// RangedCombatMethod.ts start()): only the venator bow sets
// one (2291, WeaponProfile.ts:333-342).
var rangedWeaponProjectileOverrides = map[string]int{
	"VENATOR_BOW": 2291,
}

// rangedWeaponLaunchGraphicOverrides is the per-entry weapon
// launch spotanim (WeaponProfile.ts launchGraphic, read as
// profile?.launchGraphic in RangedCombatMethod.ts start()
// :143-144): only the venator bow sets one (2289,
// WeaponProfile.ts:328-342, arrow_venator01_launch01).
var rangedWeaponLaunchGraphicOverrides = map[string]int{
	"VENATOR_BOW": 2289,
}

// rangedWeaponSpec is one built weapon entry: the shared
// RangedWeaponData template plus the per-entry Longrange
// style name ("" when the type has none).
type rangedWeaponSpec struct {
	data               *RangedWeaponData
	longRangeFightType string
}

// rangedAmmunitionByItem maps every ammunition item id —
// canonical and poisoned-variant alias alike — to its shared
// AmmunitionData (one pointer per kind, so CheckAmmo's
// pointer comparison sees a poisoned arrow as its plain form).
var rangedAmmunitionByItem = buildRangedAmmunitionByItem()

// rangedWeaponByItem maps every weapon item id to its shared
// RangedWeaponData template.
var rangedWeaponByItem = buildRangedWeaponByItem()

// rangedWeaponSpecs maps every weapon item id to its spec
// (template + Longrange style).
var rangedWeaponSpecs = buildRangedWeaponSpecs()

// rangedAmmunitionKindByName indexes the kinds for the weapon
// build.
var rangedAmmunitionKindByName = buildRangedAmmunitionKindByName()

func buildRangedAmmunitionKindByName() map[string]*AmmunitionData {
	byName := make(map[string]*AmmunitionData, len(rangedAmmunitionKinds))
	for i := range rangedAmmunitionKinds {
		kind := &rangedAmmunitionKinds[i]
		byName[kind.name] = &AmmunitionData{ItemID: kind.itemID, ProjectileID: kind.projectileID, StartGraphicID: kind.startGraphicID, DoubleStartGraphicID: kind.doubleStartGraphicID, Strength: kind.strength}
	}
	return byName
}

func buildRangedAmmunitionByItem() map[int]*AmmunitionData {
	byItem := make(map[int]*AmmunitionData)
	for i := range rangedAmmunitionKinds {
		kind := &rangedAmmunitionKinds[i]
		data := rangedAmmunitionKindByName[kind.name]
		byItem[kind.itemID] = data
		for _, alias := range kind.aliasIDs {
			byItem[alias] = data
		}
	}
	return byItem
}

func buildRangedWeaponSpecs() map[int]*rangedWeaponSpec {
	specs := make(map[int]*rangedWeaponSpec)
	for _, entry := range rangedWeaponEntries {
		profile, ok := rangedTypeProfiles[entry.weaponType]
		if !ok {
			continue
		}
		distance := profile.attackDistance
		if override, ok := rangedWeaponDistanceOverrides[entry.name]; ok {
			distance = override
		}
		ammoIDs := make([]int, 0, len(entry.ammoKinds))
		for _, kindName := range entry.ammoKinds {
			// AmmoIDs carries every item id of each accepted
			// kind (canonical + aliases): CheckAmmo compares
			// the worn item id against this list
			// (combat_factory.go), so a poisoned arrow must
			// list here or it fails the gate even though its
			// AmmunitionData resolves. A kind name the tables
			// do not hold is skipped, never fatal at boot.
			kind := rangedAmmunitionKindByName[kindName]
			if kind == nil {
				continue
			}
			ammoIDs = append(ammoIDs, kind.ItemID)
			if idx := kindIndex(kindName); idx >= 0 {
				ammoIDs = append(ammoIDs, rangedAmmunitionKinds[idx].aliasIDs...)
			}
		}
		data := &RangedWeaponData{
			AmmoIDs:         ammoIDs,
			AttackDistance:  distance,
			HitDelayBase:    profile.hitDelayBase,
			HitDelayOffset:  profile.hitDelayOffset,
			HitDelayDivisor: profile.hitDelayDivisor,
			Projectiles:     profile.projectiles,
			ProjectileID:    rangedWeaponProjectileOverrides[entry.name],
			LaunchGraphicID: rangedWeaponLaunchGraphicOverrides[entry.name],
		}
		// The blowpipe suppresses the start graphic
		// (WeaponProfile.ts BLOWPIPE row, startGraphic: false,
		// :220-228): its darts carry launch spotanims, but no
		// launch graphic plays (RangedCombatMethod.ts:145).
		// The row is per-interface, so every BLOWPIPE entry
		// suppresses, not just the toxic blowpipe.
		if entry.weaponType == "BLOWPIPE" {
			data.SuppressStartGraphic = true
		}
		// UsesWeaponSlotAmmo is CombatFactory.usesWeaponSlotAmmo
		// (CombatFactory.ts:1952-1965): one ammunition entry
		// whose item id is one of the weapon ids — thrown
		// weapons and self-ammo bows.
		if len(entry.ammoKinds) == 1 {
			if ammo := rangedAmmunitionKindByName[entry.ammoKinds[0]]; ammo != nil {
				for _, weaponID := range entry.weaponIDs {
					if weaponID == ammo.ItemID {
						data.UsesWeaponSlotAmmo = true
						break
					}
				}
			}
		}
		// The dark bow fires two arrows (WeaponProfile.ts
		// DARK_BOW, ammoRequired 2, :290-301); every other
		// weapon fires one (AmmoRequired 0 reads as 1 in
		// combat_ranged.go). Its doubleStartGraphic flag
		// (same row) plays the ammunition's double launch
		// graphic on a two-arrow shot
		// (RangedCombatMethod.ts:145).
		if entry.name == "DARK_BOW" {
			data.AmmoRequired = 2
			data.DoubleStartGraphic = true
			// The dark bow's second hit delay
			// (WeaponProfile.ts:295, hitDelays
			// [STANDARD_HIT, { base: 1,
			// distanceOffset: 2, divisor: 3 }]):
			// the first entry repeats the single
			// triple above so the list maps whole.
			data.HitDelayProfiles = []HitDelayProfile{
				{Base: 1, DistanceOffset: 3, Divisor: 6},
				{Base: 1, DistanceOffset: 2, Divisor: 3},
			}
			data.Projectiles = []RangedProjectileProfile{
				rangedBowProjectile,
				{Delay: 41, LengthAdjustment: 14, StepMultiplier: 10, StartHeight: 48, EndHeight: 31},
			}
		}
		spec := &rangedWeaponSpec{data: data, longRangeFightType: profile.longRangeFightType}
		for _, weaponID := range entry.weaponIDs {
			specs[weaponID] = spec
		}
	}
	return specs
}

func kindIndex(name string) int {
	for i := range rangedAmmunitionKinds {
		if rangedAmmunitionKinds[i].name == name {
			return i
		}
	}
	return -1
}

func buildRangedWeaponByItem() map[int]*RangedWeaponData {
	byItem := make(map[int]*RangedWeaponData)
	for weaponID, spec := range rangedWeaponSpecs {
		byItem[weaponID] = spec.data
	}
	return byItem
}

// ResolveRangedData is the production ranged-data resolver
// (RangedWeapon.getFor / Ammunition.getFor, RangedData.ts):
// the equipped weapon from the weapon slot, the ammunition
// from the weapon itself when it is self-ammo or from the ammo
// slot otherwise. It returns (nil, nil) when no ranged weapon
// is equipped, preserving the melee fallback. The returned
// weapon is a per-call copy of the shared template with the
// attack distance resolved for this player (see
// rangedAttackDistance): the shared template must never be
// mutated, because two players with different fight types
// resolve the same weapon. The ammunition pointer is the
// shared kind pointer, so CheckAmmo's identity comparison
// holds.
func ResolveRangedData(player *Player) (*RangedWeaponData, *AmmunitionData) {
	if player == nil {
		return nil, nil
	}
	weaponItem := player.Equipment().GetWeapon()
	if !weaponItem.IsValid() {
		return nil, nil
	}
	spec, ok := rangedWeaponSpecs[weaponItem.ID()]
	if !ok {
		return nil, nil
	}
	weapon := *spec.data
	weapon.AttackDistance = rangedAttackDistance(player, weaponItem.ID(), spec)
	var ammo *AmmunitionData
	if spec.data.UsesWeaponSlotAmmo {
		ammo = rangedAmmunitionByItem[weaponItem.ID()]
	} else {
		ammoItem := player.Equipment().ItemAt(AmmunitionSlot)
		if ammoItem.IsValid() {
			ammo = rangedAmmunitionByItem[ammoItem.ID()]
		}
	}
	return &weapon, ammo
}

// rangedAttackDistance resolves one player's attack distance
// for a weapon (WeaponProfile.ts attackDistance, :134-154):
// the cache range when a WeaponInfoSource is installed and
// knows the item, else the static interface distance; the
// Longrange style adds 2 tiles, capped at 10.
func rangedAttackDistance(player *Player, weaponID int, spec *rangedWeaponSpec) int {
	distance := spec.data.AttackDistance
	if src, ok := player.ItemDefinitions().(WeaponInfoSource); ok && src != nil {
		if cacheRange := src.WeaponAttackRange(weaponID); cacheRange > 0 {
			distance = cacheRange
		}
	}
	if spec.longRangeFightType != "" && player.GetCombat().FightType().Name == spec.longRangeFightType {
		distance += 2
	}
	if distance > 10 {
		distance = 10
	}
	if distance < 1 {
		distance = 1
	}
	return distance
}

// RegisterRangedAmmunitionData registers every ammunition
// item id in the tables through the RegisterAmmunitionData
// seam (combat_ranged.go), so CheckAmmo's
// ammunitionDataForItem lookup answers for canonical ids and
// poisoned-variant aliases alike. Called once at boot by the
// server wiring; unit tests register what they need directly.
func RegisterRangedAmmunitionData() {
	for itemID, data := range rangedAmmunitionByItem {
		ammunitionDataByItem[itemID] = data
	}
}

// rangedChargeBowIDs is the set of weapon ids whose shots
// spend charges or ether instead of a stackable ammunition
// item (CombatFactory.decrementAmmo, CombatFactory.ts:
// 1807-1859): the crystal bows (charge degradation via the
// CrystalBow plugin), the Gauntlet's bows ("Plugin-owned
// ammunition ... the Gauntlet's bows", :1796-1800), the Bows
// of Faerdhinen (crystal arrows of their own, same family),
// and Webweaver/Craw's bows ("The bows spend their revenant
// ether through the plugin ammo handlers", :1807-1812). The
// core decrement would otherwise eat the bow itself from the
// weapon slot (UsesWeaponSlotAmmo). DecrementAmmo spares
// them (the charge-bow guard there, after the plugin hook):
// they fire without consuming anything until the charge
// systems land. Charge degradation itself is NOT STARTED:
// the stage data and predicates below are the grounded half;
// the per-shot counting, the "combat:consume-charge"
// question, and the stage swap are still to come. Thrown
// weapons, chinchompas, tonalztics and morrigans weapons are
// NOT in this set: core TS decrements them from the weapon
// slot. Whether a plugin ranged-ammo handler spares
// tonalztics or morrigans weapons could not be grounded here
// — the sparse checkout carries no plugin content — so they
// follow core TS literally.
var rangedChargeBowIDs = buildRangedChargeBowIDs()

func buildRangedChargeBowIDs() map[int]bool {
	ids := make(map[int]bool)
	for _, entry := range rangedWeaponEntries {
		switch {
		case entry.name == "CRYSTAL_BOW",
			entry.name == "GAUNTLET_BOW",
			entry.name == "WEBWEAVER_BOW",
			entry.name == "CRAWS_BOW":
			for _, weaponID := range entry.weaponIDs {
				ids[weaponID] = true
			}
		case strings.HasPrefix(entry.name, "FAERDHINEN_"):
			for _, weaponID := range entry.weaponIDs {
				ids[weaponID] = true
			}
		}
	}
	return ids
}

// IsRangedChargeBow reports whether a weapon id is a
// charge/ether bow (see rangedChargeBowIDs): its shots spend
// no stackable ammunition.
func IsRangedChargeBow(itemID int) bool {
	return rangedChargeBowIDs[itemID]
}

// Crystal-bow charges: the engine data from CrystalBow.ts
// (ranged/CrystalBow.ts). A charged crystal bow degrades one
// stage every CrystalBowShotsPerStage shots; the per-shot
// counting, the "combat:consume-charge" question, and the
// stage swap live in CombatFactory.decrementAmmo's crystal-bow
// branch (CombatFactory.ts:1812-1861) and are NOT STARTED in
// Go — what lands here is the grounded data that branch
// needs, plus the predicates, so the charge system can be
// built without re-deriving ids. DecrementAmmo already
// spares these bows (see the charge-bow guard there): they
// fire without consuming anything until the charge system
// lands (the rangedChargeBowIDs comment).

// CrystalBowShotsPerStage is CRYSTAL_BOW_SHOTS_PER_STAGE
// (CrystalBow.ts): shots before a charged bow drops a stage.
const CrystalBowShotsPerStage = 250

// crystalBowStageOrder is CRYSTAL_BOW_STAGE_ORDER
// (CrystalBow.ts): the ten degradation stages, full to 1/10,
// each [plain, imbued] by item id.
var crystalBowStageOrder = [][2]int{
	{4214, 11749}, {4215, 11750}, {4216, 11751}, {4217, 11752}, {4218, 11753},
	{4219, 11754}, {4220, 11755}, {4221, 11756}, {4222, 11757}, {4223, 11758},
}

// crystalBowAttackBonuses is CRYSTAL_BOW_ATTACK_BONUSES
// (CrystalBow.ts): the ranged attack bonus by stage index.
var crystalBowAttackBonuses = []int{100, 100, 96, 92, 88, 84, 80, 76, 72, 68}

// crystalBowRangedStrengths is CRYSTAL_BOW_RANGED_STRENGTHS
// (CrystalBow.ts): the ranged strength by stage index.
var crystalBowRangedStrengths = []int{78, 78, 76, 74, 72, 70, 68, 66, 64, 62}

// crystalBowStaticIDs is STATIC_CRYSTAL_BOW_IDS
// (CrystalBow.ts): charged bows with no stage (the plain
// crystal bow 23983 and crystal bow 3, 24123).
var crystalBowStaticIDs = map[int]bool{23983: true, 24123: true}

// crystalBowEmptyIDs is EMPTY_CRYSTAL_BOW_IDS
// (CrystalBow.ts): uncharged bows (new 4212, new (i) 11748,
// inactive 23985).
var crystalBowEmptyIDs = map[int]bool{4212: true, 11748: true, 23985: true}

// crystalBowStageIndexByID indexes staged bows by item id.
var crystalBowStageIndexByID = buildCrystalBowStageIndex()

func buildCrystalBowStageIndex() map[int]int {
	byID := make(map[int]int, 20)
	for stage, pair := range crystalBowStageOrder {
		for _, id := range pair {
			byID[id] = stage
		}
	}
	return byID
}

// IsCrystalBow reports whether id is any crystal bow: staged,
// static, or empty (isCrystalBow, CrystalBow.ts).
func IsCrystalBow(itemID int) bool {
	_, staged := crystalBowStageIndexByID[itemID]
	return staged || crystalBowStaticIDs[itemID] || crystalBowEmptyIDs[itemID]
}

// IsChargedCrystalBow reports whether id is a charged crystal
// bow: staged or static, not empty (isChargedCrystalBow,
// CrystalBow.ts). Only a charged bow degrades.
func IsChargedCrystalBow(itemID int) bool {
	_, staged := crystalBowStageIndexByID[itemID]
	return staged || crystalBowStaticIDs[itemID]
}

// IsEmptyCrystalBow reports whether id is an uncharged crystal
// bow (isEmptyCrystalBow, CrystalBow.ts).
func IsEmptyCrystalBow(itemID int) bool {
	return crystalBowEmptyIDs[itemID]
}

// CrystalBowAttackBonus returns the ranged attack bonus for a
// staged crystal bow (getCrystalBowAttackBonus, CrystalBow.ts),
// or false when the id has no stage (static and empty bows
// read their definition bonuses instead, as BonusManager does).
func CrystalBowAttackBonus(itemID int) (int, bool) {
	stage, ok := crystalBowStageIndexByID[itemID]
	if !ok {
		return 0, false
	}
	return crystalBowAttackBonuses[stage], true
}

// CrystalBowRangedStrength returns the ranged strength for a
// staged crystal bow (getCrystalBowRangedStrength,
// CrystalBow.ts), or false when the id has no stage.
func CrystalBowRangedStrength(itemID int) (int, bool) {
	stage, ok := crystalBowStageIndexByID[itemID]
	if !ok {
		return 0, false
	}
	return crystalBowRangedStrengths[stage], true
}

// CrystalBowNextStageID returns the item id a staged crystal
// bow degrades into after CrystalBowShotsPerStage shots
// (getNextCrystalBowItemId, CrystalBow.ts): the next stage,
// same imbued-ness, or the empty bow (new 4212 / new (i)
// 11748) past the last stage. It returns false for ids with
// no stage: static bows never degrade and empty bows are
// already spent.
func CrystalBowNextStageID(itemID int) (int, bool) {
	stage, ok := crystalBowStageIndexByID[itemID]
	if !ok {
		return 0, false
	}
	// Imbued is the second id of the stage pair (TS tests
	// itemId >= CRYSTAL_BOW_FULL_I_, 11749).
	imbued := crystalBowStageOrder[stage][1] == itemID
	if stage >= len(crystalBowStageOrder)-1 {
		if imbued {
			return 11748, true
		}
		return 4212, true
	}
	next := crystalBowStageOrder[stage+1][0]
	if imbued {
		next = crystalBowStageOrder[stage+1][1]
	}
	return next, true
}
