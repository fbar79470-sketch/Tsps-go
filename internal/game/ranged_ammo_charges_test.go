package game

import "testing"

// Ammo strength + crystal-bow charges (Phase 30, Worker C,
// item 10): the engine-grounded part only. Strengths are
// script-extracted from the Ammunition constructor's fourth
// argument (RangedData.ts); the crystal-bow stage data is
// from CrystalBow.ts. Plugin ammo resolution
// (PluginManager.resolveRangedAmmunition: blowpipe scales,
// Dizana's quiver) and charge degradation (the per-shot
// counting in CombatFactory.decrementAmmo's crystal-bow
// branch) stay NOT STARTED and are not asserted here.

func TestAmmunitionStrengths(t *testing.T) {
	// Spot checks against RangedData.ts, across the families:
	// arrows, bolts (plain/gem/dragon/ascension), darts,
	// knives, javelins, and the zero-strength self-ammo bows.
	cases := []struct {
		id   int
		want int
	}{
		{882, 7},     // BRONZE_ARROW
		{892, 50},    // RUNE_ARROW
		{21326, 55},  // AMETHYST_ARROW
		{11212, 65},  // DRAGON_ARROW
		{33595, 60},  // SEEKING_DRAGON_ARROW
		{877, 13},    // BRONZE_BOLT
		{9144, 115},  // RUNITE_BOLT
		{9341, 117},  // DRAGON_BOLT
		{9342, 120},  // ONYX_BOLT
		{21948, 122}, // ENCHANTED_DRAGONSTONE_DRAGON_BOLT
		{34684, 53},  // ASCENSION_BOLT
		{806, 1},     // BRONZE_DART
		{811, 26},    // RUNE_DART
		{11230, 35},  // DRAGON_DART
		{864, 3},     // BRONZE_KNIFE
		{22636, 145}, // MORRIGANS_JAVELIN
		{6522, 58},   // TOKTZ_XIL_UL
		{4214, 0},    // CRYSTAL_BOW (self-ammo bow)
		{23901, 0},   // GAUNTLET_BOW
		{27655, 0},   // WEBWEAVER_BOW
		{22550, 0},   // CRAWS_BOW
		{25865, 0},   // BOW_OF_FAERDHINEN
		{10033, 0},   // CHINCHOMPA
		{28922, 0},   // TONALZTICS_OF_RALOS
		{33801, 0},   // SILVTHRILL_JAVELIN
	}
	for _, tc := range cases {
		ammo, ok := rangedAmmunitionByItem[tc.id]
		if !ok || ammo == nil {
			t.Errorf("no ammunition for item %d", tc.id)
			continue
		}
		if ammo.Strength != tc.want {
			t.Errorf("ammunition %d strength = %d, want %d", tc.id, ammo.Strength, tc.want)
		}
	}
	// The resolver carries the strength through: a bronze
	// arrow in the ammo slot resolves with Strength 7.
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	p.Equipment().Set(WeaponSlot, NewItem(841, 1))
	p.Equipment().Set(AmmunitionSlot, NewItem(882, 10))
	_, ammo := ResolveRangedData(p)
	if ammo == nil || ammo.Strength != 7 {
		t.Fatalf("resolved bronze arrow strength = %v, want 7", ammo)
	}
	// A poisoned variant shares the canonical kind's pointer
	// (and strength): rune arrow (p+) 893 is rune arrow 50.
	if got := rangedAmmunitionByItem[893]; got == nil || got.Strength != 50 {
		t.Errorf("poisoned rune arrow strength = %v, want 50", got)
	}
}

func TestCrystalBowCharges(t *testing.T) {
	if CrystalBowShotsPerStage != 250 {
		t.Errorf("shots per stage = %d, want 250", CrystalBowShotsPerStage)
	}
	// Classification (CrystalBow.ts): staged, static, empty.
	for _, id := range []int{4214, 4215, 4223, 11749, 11758} {
		if !IsCrystalBow(id) || !IsChargedCrystalBow(id) || IsEmptyCrystalBow(id) {
			t.Errorf("crystal bow %d misclassified", id)
		}
	}
	for _, id := range []int{23983, 24123} {
		if !IsCrystalBow(id) || !IsChargedCrystalBow(id) || IsEmptyCrystalBow(id) {
			t.Errorf("static crystal bow %d misclassified", id)
		}
	}
	for _, id := range []int{4212, 11748, 23985} {
		if !IsCrystalBow(id) || IsChargedCrystalBow(id) || !IsEmptyCrystalBow(id) {
			t.Errorf("empty crystal bow %d misclassified", id)
		}
	}
	if IsCrystalBow(841) || IsChargedCrystalBow(841) || IsEmptyCrystalBow(841) {
		t.Error("shortbow must not be a crystal bow")
	}
	// Stage bonuses/strengths (CrystalBow.ts): full and 1/10.
	if bonus, ok := CrystalBowAttackBonus(4214); !ok || bonus != 100 {
		t.Errorf("full bow attack bonus = %d/%v, want 100/true", bonus, ok)
	}
	if str, ok := CrystalBowRangedStrength(4214); !ok || str != 78 {
		t.Errorf("full bow ranged strength = %d/%v, want 78/true", str, ok)
	}
	if bonus, ok := CrystalBowAttackBonus(4223); !ok || bonus != 68 {
		t.Errorf("1/10 bow attack bonus = %d/%v, want 68/true", bonus, ok)
	}
	if str, ok := CrystalBowRangedStrength(4223); !ok || str != 62 {
		t.Errorf("1/10 bow ranged strength = %d/%v, want 62/true", str, ok)
	}
	if _, ok := CrystalBowAttackBonus(23983); ok {
		t.Error("static crystal bow has no stage bonus")
	}
	// The stage chain (CrystalBow.ts getNextCrystalBowItemId):
	// full -> 9/10, 1/10 -> empty, imbued stays imbued.
	if next, ok := CrystalBowNextStageID(4214); !ok || next != 4215 {
		t.Errorf("next after full = %d/%v, want 4215/true", next, ok)
	}
	if next, ok := CrystalBowNextStageID(4223); !ok || next != 4212 {
		t.Errorf("next after 1/10 = %d/%v, want 4212/true", next, ok)
	}
	if next, ok := CrystalBowNextStageID(11749); !ok || next != 11750 {
		t.Errorf("next after full (i) = %d/%v, want 11750/true", next, ok)
	}
	if next, ok := CrystalBowNextStageID(11758); !ok || next != 11748 {
		t.Errorf("next after 1/10 (i) = %d/%v, want 11748/true", next, ok)
	}
	if _, ok := CrystalBowNextStageID(23983); ok {
		t.Error("static crystal bow has no next stage")
	}
	if _, ok := CrystalBowNextStageID(4212); ok {
		t.Error("empty crystal bow has no next stage")
	}
}

// TestDecrementAmmoSparesChargeBows is the charge-bow guard
// (CombatFactory.decrementAmmo, CombatFactory.ts:1799-1811):
// a charge/ether bow's shot spends no stackable ammunition,
// so the bow itself must survive the decrement. Before the
// guard, a crystal bow's UsesWeaponSlotAmmo decremented the
// bow to nothing on its first shot.
func TestDecrementAmmoSparesChargeBows(t *testing.T) {
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
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	// A plugin hook must still win over the guard (TS order:
	// decrementRangedAmmo runs before the bow branches).
	hookCalled := false
	Hooks.DecrementAmmo = func(player *Player, weapon, ammo Item, amount int) bool {
		hookCalled = true
		return true
	}
	for _, id := range []int{4214, 23901, 27655, 22550, 25865} {
		p.Equipment().Set(WeaponSlot, NewItem(id, 1))
		resolveRangedData(p)
		DecrementAmmo(p, Location{X: 3001, Y: 3000}, 1, 1)
		if got := p.Equipment().GetWeapon(); !got.IsValid() || got.ID() != id || got.Amount() != 1 {
			t.Errorf("charge bow %d after decrement = %v, want the bow intact", id, got)
		}
	}
	if !hookCalled {
		t.Error("plugin decrement hook was never consulted")
	}
	// Without the hook the guard still spares the bow.
	Hooks.DecrementAmmo = nil
	p.Equipment().Set(WeaponSlot, NewItem(4214, 1))
	resolveRangedData(p)
	DecrementAmmo(p, Location{X: 3001, Y: 3000}, 1, 1)
	if got := p.Equipment().GetWeapon(); !got.IsValid() || got.ID() != 4214 || got.Amount() != 1 {
		t.Errorf("crystal bow after unhooked decrement = %v, want the bow intact", got)
	}
	// Control: an ordinary bow still pays one arrow.
	p.Equipment().Set(WeaponSlot, NewItem(841, 1))
	p.Equipment().Set(AmmunitionSlot, NewItem(882, 10))
	resolveRangedData(p)
	setFixedRandom(t, func(max int) int { return 10 }, 0.5) // < 20: break
	DecrementAmmo(p, Location{X: 3001, Y: 3000}, 1, 1)
	if got := p.Equipment().ItemAt(AmmunitionSlot).Amount(); got != 9 {
		t.Errorf("shortbow ammo after decrement = %d, want 9", got)
	}
}
