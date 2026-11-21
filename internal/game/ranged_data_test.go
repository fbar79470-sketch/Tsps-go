package game

import (
	"testing"
)

// Ranged tables: counts and spot-checks against upstream
// RangedData.ts (script-extracted; see ranged_data_tables.go).
// Upstream holds 193 ammunition kinds covering 253 item ids
// (193 canonical + 60 poisoned-variant aliases) and 102
// ranged-weapon entries covering 229 weapon item ids.

func TestRangedTableCounts(t *testing.T) {
	if got := len(rangedAmmunitionKinds); got != 193 {
		t.Errorf("ammunition kinds = %d, want 193", got)
	}
	if got := len(rangedAmmunitionByItem); got != 253 {
		t.Errorf("ammunition item ids = %d, want 253", got)
	}
	if got := len(rangedWeaponEntries); got != 102 {
		t.Errorf("weapon entries = %d, want 102", got)
	}
	if got := len(rangedWeaponByItem); got != 229 {
		t.Errorf("weapon item ids = %d, want 229", got)
	}
	if got := len(rangedWeaponSpecs); got != 229 {
		t.Errorf("weapon specs = %d, want 229", got)
	}
}

func rangedWeapon(t *testing.T, id int) *RangedWeaponData {
	t.Helper()
	weapon, ok := rangedWeaponByItem[id]
	if !ok || weapon == nil {
		t.Fatalf("no ranged weapon for item %d", id)
	}
	return weapon
}

func containsID(ids []int, want int) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}

func TestRangedTableSpotRows(t *testing.T) {
	// Shortbow (841): bronze/iron arrows only, 7 tiles,
	// STANDARD hit delay, ammo-slot ammo.
	shortbow := rangedWeapon(t, 841)
	if shortbow.AttackDistance != 7 {
		t.Errorf("shortbow distance = %d, want 7", shortbow.AttackDistance)
	}
	if shortbow.UsesWeaponSlotAmmo {
		t.Errorf("shortbow should draw from the ammo slot")
	}
	for _, want := range []int{882, 883, 884, 885} {
		if !containsID(shortbow.AmmoIDs, want) {
			t.Errorf("shortbow AmmoIDs missing %d: %v", want, shortbow.AmmoIDs)
		}
	}
	if containsID(shortbow.AmmoIDs, 886) {
		t.Errorf("shortbow AmmoIDs should not accept steel arrows: %v", shortbow.AmmoIDs)
	}
	if shortbow.HitDelayBase != 1 || shortbow.HitDelayOffset != 3 || shortbow.HitDelayDivisor != 6 {
		t.Errorf("shortbow hit delay = %d/%d/%d, want 1/3/6", shortbow.HitDelayBase, shortbow.HitDelayOffset, shortbow.HitDelayDivisor)
	}
	// Longbow (839): same arrows, 10 tiles (interface row,
	// not the type default 9).
	if got := rangedWeapon(t, 839).AttackDistance; got != 10 {
		t.Errorf("longbow distance = %d, want 10", got)
	}
	// Magic shortbow (861): nine arrow kinds; 37 item ids
	// (5 each for bronze..rune, 1 ice, 1 broad, 5 amethyst).
	magic := rangedWeapon(t, 861)
	if got := len(magic.AmmoIDs); got != 37 {
		t.Errorf("magic shortbow AmmoIDs = %d, want 37", got)
	}
	for _, want := range []int{892, 893, 21326, 4160} {
		if !containsID(magic.AmmoIDs, want) {
			t.Errorf("magic shortbow AmmoIDs missing %d", want)
		}
	}
	// Dark bow (11235): two arrows a shot, dragon + seeking
	// dragon arrows accepted, 10 tiles, two projectiles.
	darkbow := rangedWeapon(t, 11235)
	if darkbow.AmmoRequired != 2 {
		t.Errorf("dark bow AmmoRequired = %d, want 2", darkbow.AmmoRequired)
	}
	if darkbow.AttackDistance != 10 {
		t.Errorf("dark bow distance = %d, want 10", darkbow.AttackDistance)
	}
	for _, want := range []int{11212, 33595} {
		if !containsID(darkbow.AmmoIDs, want) {
			t.Errorf("dark bow AmmoIDs missing %d", want)
		}
	}
	if got := len(darkbow.Projectiles); got != 2 {
		t.Errorf("dark bow projectiles = %d, want 2", got)
	}
	// The dark bow's second hit delay (WeaponProfile.ts:295):
	// hitDelays [STANDARD_HIT, { base: 1, distanceOffset: 2,
	// divisor: 3 }], carried whole in HitDelayProfiles.
	if got := len(darkbow.HitDelayProfiles); got != 2 {
		t.Fatalf("dark bow hit-delay profiles = %d, want 2", got)
	}
	if darkbow.HitDelayProfiles[0] != (HitDelayProfile{Base: 1, DistanceOffset: 3, Divisor: 6}) ||
		darkbow.HitDelayProfiles[1] != (HitDelayProfile{Base: 1, DistanceOffset: 2, Divisor: 3}) {
		t.Errorf("dark bow hit-delay profiles = %v, want [{1 3 6} {1 2 3}]", darkbow.HitDelayProfiles)
	}
	// Bronze dart (806): thrown, weapon-slot ammo, 3 tiles.
	dart := rangedWeapon(t, 806)
	if !dart.UsesWeaponSlotAmmo {
		t.Errorf("bronze dart should be weapon-slot ammo")
	}
	if len(dart.AmmoIDs) != 1 || dart.AmmoIDs[0] != 806 {
		t.Errorf("bronze dart AmmoIDs = %v, want [806]", dart.AmmoIDs)
	}
	if dart.AttackDistance != 3 {
		t.Errorf("bronze dart distance = %d, want 3", dart.AttackDistance)
	}
	// Rune knife (868): thrown, 4 tiles.
	if got := rangedWeapon(t, 868).AttackDistance; got != 4 {
		t.Errorf("rune knife distance = %d, want 4", got)
	}
	// Crystal bow (4214, full): self-ammo, 10 tiles; its
	// ammunition is the crystal arrow (projectile 249).
	crystal := rangedWeapon(t, 4214)
	if !crystal.UsesWeaponSlotAmmo {
		t.Errorf("crystal bow should be self-ammo")
	}
	if crystal.AttackDistance != 10 {
		t.Errorf("crystal bow distance = %d, want 10", crystal.AttackDistance)
	}
	if got := rangedAmmunitionByItem[4214]; got == nil || got.ProjectileID != 249 {
		t.Errorf("crystal bow ammo projectile = %+v, want 249", got)
	}
	// Bronze crossbow (9174): bronze bolts only (canonical +
	// poisoned variants), 7 tiles.
	crossbow := rangedWeapon(t, 9174)
	for _, want := range []int{877, 878, 6061, 6062} {
		if !containsID(crossbow.AmmoIDs, want) {
			t.Errorf("bronze crossbow AmmoIDs missing %d", want)
		}
	}
	if containsID(crossbow.AmmoIDs, 9140) {
		t.Errorf("bronze crossbow AmmoIDs should not accept iron bolts")
	}
	// Ballista (19478): 32 javelin ids, 9 tiles, the slow
	// ballista hit delay (2, 1, 6).
	ballista := rangedWeapon(t, 19478)
	if got := len(ballista.AmmoIDs); got != 32 {
		t.Errorf("ballista AmmoIDs = %d, want 32", got)
	}
	if ballista.AttackDistance != 9 {
		t.Errorf("ballista distance = %d, want 9", ballista.AttackDistance)
	}
	if ballista.HitDelayBase != 2 || ballista.HitDelayOffset != 1 || ballista.HitDelayDivisor != 6 {
		t.Errorf("ballista hit delay = %d/%d/%d, want 2/1/6", ballista.HitDelayBase, ballista.HitDelayOffset, ballista.HitDelayDivisor)
	}
	// Karil's crossbow (4734): the KARILS interface row, 8 tiles.
	if got := rangedWeapon(t, 4734).AttackDistance; got != 8 {
		t.Errorf("karils crossbow distance = %d, want 8", got)
	}
	// Venator bow (27610): 6 tiles, the (0, 23, 15) hit
	// delay, weapon projectile 2291.
	venator := rangedWeapon(t, 27610)
	if venator.AttackDistance != 6 {
		t.Errorf("venator distance = %d, want 6", venator.AttackDistance)
	}
	if venator.HitDelayBase != 0 || venator.HitDelayOffset != 23 || venator.HitDelayDivisor != 15 {
		t.Errorf("venator hit delay = %d/%d/%d, want 0/23/15", venator.HitDelayBase, venator.HitDelayOffset, venator.HitDelayDivisor)
	}
	if venator.ProjectileID != 2291 {
		t.Errorf("venator projectile = %d, want 2291", venator.ProjectileID)
	}
	// Seeking dragon arrows (33595): projectile 1120, the
	// dragon-arrow travel spotanim.
	if got := rangedAmmunitionByItem[33595]; got == nil || got.ItemID != 33595 || got.ProjectileID != 1120 {
		t.Errorf("seeking dragon arrow ammo = %+v, want item 33595 projectile 1120", got)
	}
	// Rune arrows (892): projectile 15; the poisoned form
	// (893) resolves to the same AmmunitionData.
	if got := rangedAmmunitionByItem[892]; got == nil || got.ProjectileID != 15 {
		t.Errorf("rune arrow ammo = %+v, want projectile 15", got)
	}
	if rangedAmmunitionByItem[893] != rangedAmmunitionByItem[892] {
		t.Errorf("poisoned rune arrow should share the rune arrow AmmunitionData")
	}
	// No weapon uses melee strength: no WeaponProfile.ts row
	// sets rangedDamageSource.
	for id, weapon := range rangedWeaponByItem {
		if weapon.UsesMeleeStrength {
			t.Errorf("weapon %d sets UsesMeleeStrength, want false for all", id)
		}
	}
}

func TestRangedTableLaunchGraphics(t *testing.T) {
	// Ammunition launch graphics (Ammunition's startGfx /
	// doubleStartGfx, RangedData.ts constructor args 2 and 5;
	// script-extracted, see ranged_data_tables.go): arrows
	// carry both, bolts none, thrown weapons only a start.
	ammo := func(itemID int) *AmmunitionData {
		t.Helper()
		data, ok := rangedAmmunitionByItem[itemID]
		if !ok || data == nil {
			t.Fatalf("no ammunition for item %d", itemID)
		}
		return data
	}
	for _, want := range []struct {
		itemID     int
		start, dbl int
	}{
		{882, 19, 1104},     // bronze arrow
		{884, 18, 1105},     // iron arrow
		{886, 20, 1106},     // steel arrow
		{892, 24, 1109},     // rune arrow
		{78, 25, 1110},      // ice arrow
		{4160, 325, 1112},   // broad arrow
		{21326, 1385, 0},    // amethyst arrow (no double)
		{9706, 806, 0},      // training arrows
		{11212, 1116, 1111}, // dragon arrow
		{33595, 1116, 1111}, // seeking dragon arrow
		{877, 0, 0},         // bronze bolt: none
		{9144, 0, 0},        // runite bolt: none
		{806, 232, 0},       // bronze dart
		{864, 219, 0},       // bronze knife
		{800, 43, 0},        // bronze thrownaxe
		{27655, 1692, 0},    // webweaver bow (self-ammo)
		{10158, 955, 0},     // kebbit bolt
		{25865, 1888, 0},    // bow of faerdhinen
		{25896, 1935, 0},    // faerdhinen (c) amlodd
		{825, 0, 0},         // bronze javelin: none
	} {
		got := ammo(want.itemID)
		if got.StartGraphicID != want.start || got.DoubleStartGraphicID != want.dbl {
			t.Errorf("ammo %d graphics = %d/%d, want %d/%d", want.itemID, got.StartGraphicID, got.DoubleStartGraphicID, want.start, want.dbl)
		}
	}
	// The poisoned form shares the plain form's graphics
	// (same AmmunitionData pointer, variants static block).
	if got := ammo(893); got.StartGraphicID != 24 || got.DoubleStartGraphicID != 1109 {
		t.Errorf("poisoned rune arrow graphics = %d/%d, want 24/1109", got.StartGraphicID, got.DoubleStartGraphicID)
	}
	// Table totals: 82 kinds carry a start graphic, 10 a
	// double (script-extracted; the extractor prints the
	// same numbers).
	starts, doubles := 0, 0
	for i := range rangedAmmunitionKinds {
		if rangedAmmunitionKinds[i].startGraphicID > 0 {
			starts++
		}
		if rangedAmmunitionKinds[i].doubleStartGraphicID > 0 {
			doubles++
		}
	}
	if starts != 82 {
		t.Errorf("kinds with a start graphic = %d, want 82", starts)
	}
	if doubles != 10 {
		t.Errorf("kinds with a double start graphic = %d, want 10", doubles)
	}
	// Weapon flags (WeaponProfile.ts): venator launch
	// graphic 2289, dark bow double, blowpipes suppress.
	if got := rangedWeapon(t, 27610).LaunchGraphicID; got != 2289 {
		t.Errorf("venator launch graphic = %d, want 2289", got)
	}
	if darkbow := rangedWeapon(t, 11235); !darkbow.DoubleStartGraphic {
		t.Errorf("dark bow should set DoubleStartGraphic")
	}
	for _, id := range []int{12926, 31583, 30374} {
		if got := rangedWeapon(t, id); !got.SuppressStartGraphic {
			t.Errorf("blowpipe %d should suppress the start graphic", id)
		}
	}
	// An ordinary bow sets none of the three flags.
	shortbow := rangedWeapon(t, 841)
	if shortbow.LaunchGraphicID != 0 || shortbow.SuppressStartGraphic || shortbow.DoubleStartGraphic {
		t.Errorf("shortbow flags = %d/%v/%v, want 0/false/false", shortbow.LaunchGraphicID, shortbow.SuppressStartGraphic, shortbow.DoubleStartGraphic)
	}
}

func TestResolveRangedData(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	RegisterRangedDataResolver(ResolveRangedData)
	t.Cleanup(func() { RegisterRangedDataResolver(nil) })
	RegisterRangedAmmunitionData()
	t.Cleanup(func() {
		for id := range rangedAmmunitionByItem {
			delete(ammunitionDataByItem, id)
		}
	})

	equip := func(p *Player, weaponID, ammoID, ammoAmount int) {
		p.Equipment().Set(WeaponSlot, NewItem(weaponID, 1))
		if ammoID >= 0 {
			p.Equipment().Set(AmmunitionSlot, NewItem(ammoID, ammoAmount))
		} else {
			p.Equipment().Set(AmmunitionSlot, NewItem(-1, 0))
		}
	}

	// Bow + arrows: ranged weapon, rune-arrow ammunition.
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	equip(p, 841, 892, 10)
	weapon, ammo := ResolveRangedData(p)
	if weapon == nil || ammo == nil {
		t.Fatalf("bow+arrows resolved weapon=%v ammo=%v, want both", weapon, ammo)
	}
	if ammo.ItemID != 892 {
		t.Errorf("ammo item = %d, want 892", ammo.ItemID)
	}
	if weapon.AttackDistance != 7 {
		t.Errorf("resolved shortbow distance = %d, want 7", weapon.AttackDistance)
	}
	// Poisoned rune arrows (893) resolve to the rune arrow data.
	equip(p, 841, 893, 10)
	if _, ammo := ResolveRangedData(p); ammo == nil || ammo.ItemID != 892 {
		t.Errorf("poisoned rune arrows resolved ammo=%v, want item 892", ammo)
	}
	// Thrown weapon: the weapon is its own ammunition.
	equip(p, 806, -1, 0)
	weapon, ammo = ResolveRangedData(p)
	if weapon == nil || !weapon.UsesWeaponSlotAmmo {
		t.Fatalf("bronze dart resolved weapon=%v, want weapon-slot ammo", weapon)
	}
	if ammo == nil || ammo.ItemID != 806 {
		t.Errorf("bronze dart ammo=%v, want item 806", ammo)
	}
	// Melee weapon: no ranged weapon (melee fallback).
	equip(p, 1321, 892, 10)
	if weapon, ammo := ResolveRangedData(p); weapon != nil || ammo != nil {
		t.Errorf("bronze scimitar resolved weapon=%v ammo=%v, want nil/nil", weapon, ammo)
	}
	// No weapon at all: nil/nil.
	p.Equipment().Set(WeaponSlot, NewItem(-1, 0))
	if weapon, ammo := ResolveRangedData(p); weapon != nil || ammo != nil {
		t.Errorf("empty weapon slot resolved weapon=%v ammo=%v, want nil/nil", weapon, ammo)
	}
	// Bow with no ammunition: weapon resolves, ammo is nil
	// (CheckAmmo rejects with the quiver line).
	equip(p, 841, -1, 0)
	weapon, ammo = ResolveRangedData(p)
	if weapon == nil || ammo != nil {
		t.Errorf("bow without ammo resolved weapon=%v ammo=%v, want weapon/non-nil ammo nil", weapon, ammo)
	}
	// Dark bow: two arrows a shot.
	equip(p, 11235, 892, 10)
	weapon, ammo = ResolveRangedData(p)
	if weapon == nil || weapon.AmmoRequired != 2 {
		t.Fatalf("dark bow resolved weapon=%v, want AmmoRequired 2", weapon)
	}
	if ammo == nil || ammo.ItemID != 892 {
		t.Errorf("dark bow ammo=%v, want item 892", ammo)
	}
	// Crystal bow: self-ammo, no ammo-slot item needed.
	equip(p, 4214, -1, 0)
	weapon, ammo = ResolveRangedData(p)
	if weapon == nil || !weapon.UsesWeaponSlotAmmo {
		t.Fatalf("crystal bow resolved weapon=%v, want self-ammo", weapon)
	}
	if ammo == nil || ammo.ItemID != 4214 || ammo.ProjectileID != 249 {
		t.Errorf("crystal bow ammo=%v, want item 4214 projectile 249", ammo)
	}
	// Nil player resolves nil/nil.
	if weapon, ammo := ResolveRangedData(nil); weapon != nil || ammo != nil {
		t.Errorf("nil player resolved weapon=%v ammo=%v, want nil/nil", weapon, ammo)
	}
}

func TestRangedAttackDistanceCacheRange(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	RegisterRangedDataResolver(ResolveRangedData)
	t.Cleanup(func() { RegisterRangedDataResolver(nil) })
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	p.Equipment().Set(WeaponSlot, NewItem(841, 1))
	p.Equipment().Set(AmmunitionSlot, NewItem(882, 10))
	// No item definitions: the interface distance (7).
	weapon, _ := ResolveRangedData(p)
	if weapon == nil || weapon.AttackDistance != 7 {
		t.Fatalf("no-cache shortbow distance = %v, want 7", weapon)
	}
	// The cache range wins over the static distance
	// (WeaponProfile.ts attackDistance order).
	defs := newCombatTestItemDefs()
	defs.attackRanges[841] = 9
	p.SetItemDefinitions(defs)
	weapon, _ = ResolveRangedData(p)
	if weapon == nil || weapon.AttackDistance != 9 {
		t.Fatalf("cache shortbow distance = %v, want 9", weapon)
	}
	// The 10-tile cap applies to a cache range above it.
	defs.attackRanges[841] = 12
	weapon, _ = ResolveRangedData(p)
	if weapon == nil || weapon.AttackDistance != 10 {
		t.Fatalf("capped shortbow distance = %v, want 10", weapon)
	}
}

func TestIsRangedChargeBow(t *testing.T) {
	// Crystal bow (full 4214, empty 23983), a Bow of
	// Faerdhinen (25865) and a Gauntlet bow (23901) are
	// charge/ether bows; thrown weapons and the blowpipe
	// are not. Webweaver / Craw's bows are checked from
	// the tables below, not hard-coded here.
	cases := []struct {
		id   int
		want bool
	}{
		{4214, true},   // crystal bow (full)
		{23983, true},  // crystal bow (empty)
		{25865, true},  // bow of faerdhinen
		{23901, true},  // gauntlet bow (basic)
		{806, false},   // bronze dart
		{12926, false}, // toxic blowpipe
		{841, false},   // shortbow
		{10033, false}, // chinchompa
	}
	for _, tc := range cases {
		if got := IsRangedChargeBow(tc.id); got != tc.want {
			t.Errorf("IsRangedChargeBow(%d) = %v, want %v", tc.id, got, tc.want)
		}
	}
	// Webweaver / Craw's bows resolve from the tables: find
	// their ids in the weapon entries rather than hard-coding
	// them here.
	for _, name := range []string{"WEBWEAVER_BOW", "CRAWS_BOW"} {
		found := false
		for _, entry := range rangedWeaponEntries {
			if entry.name != name {
				continue
			}
			found = true
			for _, id := range entry.weaponIDs {
				if !IsRangedChargeBow(id) {
					t.Errorf("%s id %d should be a charge bow", name, id)
				}
			}
		}
		if !found {
			t.Errorf("weapon entry %s missing from the tables", name)
		}
	}
}

// TestBowAttackResolvesRangedAndPaysAmmo is the integration
// test: a bow attack resolves RANGED (not melee) through the
// production resolver, and the fired shot pays one arrow.
func TestBowAttackResolvesRangedAndPaysAmmo(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	RegisterRangedDataResolver(ResolveRangedData)
	t.Cleanup(func() { RegisterRangedDataResolver(nil) })
	RegisterRangedAmmunitionData()
	t.Cleanup(func() {
		for id := range rangedAmmunitionByItem {
			delete(ammunitionDataByItem, id)
		}
	})
	// Max rolls: every accuracy roll succeeds (the
	// TestAttackCooldownBlocksSecondAttack precedent), and
	// the ammo roll drops the arrow to the floor — either
	// way the shot pays exactly one arrow.
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillRanged: 99, SkillHitpoints: 99})
	p.Equipment().Set(WeaponSlot, NewItem(841, 1))
	p.Equipment().Set(AmmunitionSlot, NewItem(882, 10))
	npc := combatTestNPC(t, 1, 3003, 3000, newCombatFakeDef())
	if got := GetMethod(&p.Mobile); got.Type() != CombatTypeRanged {
		t.Fatalf("bow method = %v, want RANGED", got.Type())
	}
	p.GetCombat().Attack(&npc.Mobile)
	if p.GetCombat().Target() != &npc.Mobile {
		t.Fatalf("bow attack did not set the target")
	}
	// Cycle 100: the swing fires (Start pays the shot in
	// Finished) and the hit queues.
	p.GetCombat().PreMovementProcess()
	p.GetCombat().PostMovementProcess()
	if got := p.Equipment().ItemAt(AmmunitionSlot).Amount(); got != 9 {
		t.Errorf("ammo after one bow shot = %d, want 9", got)
	}
	// The hit lands on its delay (2 at edge distance 3:
	// 1 + (3+3)/6): the NPC took damage. (AccumulatedDamage
	// stays 0 for a queued ranged hit — it only counts
	// applied damage — so the HP drop is the probe.)
	advance(103)
	npc.GetCombat().ProcessHitQueue(103)
	if npc.Hitpoints >= 10 {
		t.Errorf("goblin HP after a max-roll bow hit = %d, want < 10", npc.Hitpoints)
	}
}

func TestRangedMethodStartFiresPlayerProjectile(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, sess := rangedProjectileTestPlayer(t, "archer", 3000, 3000)
	target := combatTestNPC(t, 1, 3003, 3000, newCombatFakeDef())
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{attacker} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	attacker.Equipment().Set(WeaponSlot, NewItem(841, 1))
	attacker.Equipment().Set(AmmunitionSlot, NewItem(882, 10))
	attacker.GetCombat().SetRangedWeapon(rangedWeaponByItem[841])
	attacker.GetCombat().SetAmmunition(rangedAmmunitionByItem[882])
	t.Cleanup(func() { delete(pendingShots, &attacker.Mobile) })
	RangedMethod.Start(&attacker.Mobile, &target.Mobile)
	// The bronze arrow's start graphic (19, HIGH) plays on
	// the shot's tick (RangedCombatMethod.ts:145-146).
	if gfx := attacker.Mobile.Graphic(); gfx == nil || gfx.ID != 19 || gfx.Height != 100 || gfx.Delay != 0 {
		t.Fatalf("ranged start graphic = %+v, want 19 at HIGH, delay 0", gfx)
	}
	if len(sess.sends) != 1 {
		t.Fatalf("projectile sends = %d, want 1", len(sess.sends))
	}
	send := sess.sends[0]
	// Bronze arrow: travel spotanim 10, bow timing (delay 41,
	// end 41 + 5 + 5 * edge distance 3 = 61), from the
	// attacker's tile to the target's tile, locked onto the
	// NPC (index+1).
	if send.id != 10 {
		t.Errorf("projectile id = %d, want 10 (bronze arrow)", send.id)
	}
	if send.delay != 41 {
		t.Errorf("projectile delay = %d, want 41", send.delay)
	}
	if send.speed != 61 {
		t.Errorf("projectile end = %d, want 61", send.speed)
	}
	if send.start != (Location{X: 3000, Y: 3000}) || send.end != (Location{X: 3003, Y: 3000}) {
		t.Errorf("projectile %v -> %v, want (3000,3000) -> (3003,3000)", send.start, send.end)
	}
	if send.lockon != target.GetIndex()+1 {
		t.Errorf("projectile lockon = %d, want NPC index+1 %d", send.lockon, target.GetIndex()+1)
	}
	if send.startH != 40.75 || send.endH != 36.5 {
		t.Errorf("projectile heights = %v/%v, want 40.75/36.5", send.startH, send.endH)
	}
}

func TestRangedMethodStartDarkBowFiresTwo(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, sess := rangedProjectileTestPlayer(t, "archer", 3000, 3000)
	target := combatTestNPC(t, 1, 3003, 3000, newCombatFakeDef())
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{attacker} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	attacker.Equipment().Set(WeaponSlot, NewItem(11235, 1))
	attacker.Equipment().Set(AmmunitionSlot, NewItem(892, 10))
	attacker.GetCombat().SetRangedWeapon(rangedWeaponByItem[11235])
	attacker.GetCombat().SetAmmunition(rangedAmmunitionByItem[892])
	t.Cleanup(func() { delete(pendingShots, &attacker.Mobile) })
	RangedMethod.Start(&attacker.Mobile, &target.Mobile)
	if len(sess.sends) != 2 {
		t.Fatalf("dark bow projectile sends = %d, want 2", len(sess.sends))
	}
	// Rune arrow (15) twice: first end 41+5+5*3 = 61, second
	// 41+14+10*3 = 85.
	if sess.sends[0].id != 15 || sess.sends[1].id != 15 {
		t.Errorf("dark bow projectile ids = %d/%d, want 15/15", sess.sends[0].id, sess.sends[1].id)
	}
	if sess.sends[0].speed != 61 || sess.sends[1].speed != 85 {
		t.Errorf("dark bow projectile ends = %d/%d, want 61/85", sess.sends[0].speed, sess.sends[1].speed)
	}
}

func TestRangedMethodStartFiresNothingWithoutAmmo(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, sess := rangedProjectileTestPlayer(t, "archer", 3000, 3000)
	target := combatTestNPC(t, 1, 3003, 3000, newCombatFakeDef())
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{attacker} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	attacker.Equipment().Set(WeaponSlot, NewItem(841, 1))
	attacker.Equipment().Set(AmmunitionSlot, NewItem(-1, 0))
	attacker.GetCombat().SetRangedWeapon(rangedWeaponByItem[841])
	attacker.GetCombat().SetAmmunition(nil)
	t.Cleanup(func() { delete(pendingShots, &attacker.Mobile) })
	RangedMethod.Start(&attacker.Mobile, &target.Mobile)
	if len(sess.sends) != 0 {
		t.Errorf("projectile sends without ammo = %d, want 0", len(sess.sends))
	}
}

func TestRangedMethodStartFiresNPCProjectile(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	viewer, sess := rangedProjectileTestPlayer(t, "viewer", 3002, 3000)
	def := newCombatFakeDef()
	def.projectileID = 123
	npc := combatTestNPC(t, 1, 3003, 3000, def)
	target, _ := combatTestPlayer(t, "target", 3000, 3000)
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{viewer} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	RangedMethod.Start(&npc.Mobile, &target.Mobile)
	if len(sess.sends) != 1 {
		t.Fatalf("NPC projectile sends = %d, want 1", len(sess.sends))
	}
	send := sess.sends[0]
	if send.id != 123 {
		t.Errorf("NPC projectile id = %d, want 123 (definition)", send.id)
	}
	if send.delay != 41 {
		t.Errorf("NPC projectile delay = %d, want 41", send.delay)
	}
	// End 46 + 5 * edge distance 3 = 61, heights 43/31.
	if send.speed != 61 {
		t.Errorf("NPC projectile end = %d, want 61", send.speed)
	}
	if send.startH != 43 || send.endH != 31 {
		t.Errorf("NPC projectile heights = %v/%v, want 43/31", send.startH, send.endH)
	}
	// No configured projectile: the generic arrow (10).
	def.projectileID = -1
	npc2 := combatTestNPC(t, 1, 3003, 3000, def)
	RangedMethod.Start(&npc2.Mobile, &target.Mobile)
	if len(sess.sends) != 2 {
		t.Fatalf("unconfigured NPC projectile sends = %d, want 2", len(sess.sends))
	}
	if sess.sends[1].id != 10 {
		t.Errorf("unconfigured NPC projectile id = %d, want 10", sess.sends[1].id)
	}
}

// rangedProjectileTestPlayer is combatTestPlayer with a
// projectile-recording session, for Start() tests.
func rangedProjectileTestPlayer(t *testing.T, name string, x, y int) (*Player, *projectileFakeSession) {
	t.Helper()
	p := NewPlayer(name, Location{X: x, Y: y, Z: 0})
	p.SetRegistered(true)
	sess := newProjectileFakeSession()
	p.Session = sess
	p.GetCombat() // bind identity
	return p, sess
}
