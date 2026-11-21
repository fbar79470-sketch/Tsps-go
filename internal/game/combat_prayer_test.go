package game

import "testing"

func TestPrayerActivationConflicts(t *testing.T) {
	p, _ := combatTestPlayer(t, "priest", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	if !ActivatePrayer(p, PrayerThickSkin) {
		t.Fatal("Thick Skin activation failed")
	}
	// Same defence group: Rock Skin replaces Thick Skin.
	if !ActivatePrayer(p, PrayerRockSkin) {
		t.Fatal("Rock Skin activation failed")
	}
	if PrayerActive(&p.Mobile, PrayerThickSkin) {
		t.Errorf("Thick Skin should deactivate when Rock Skin activates")
	}
	if !PrayerActive(&p.Mobile, PrayerRockSkin) {
		t.Errorf("Rock Skin should be active")
	}
	// Overhead group: Smite replaces Protect from Melee.
	if !ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Fatal("Protect from Melee activation failed")
	}
	if !ActivatePrayer(p, PrayerSmite) {
		t.Fatal("Smite activation failed")
	}
	if PrayerActive(&p.Mobile, PrayerProtectFromMelee) {
		t.Errorf("Protect from Melee should deactivate when Smite activates")
	}
	// Toggling an active prayer off (PrayerHandler.ts:31-36).
	if !TogglePrayer(p, PrayerSmite) {
		t.Fatal("Smite toggle-off failed")
	}
	if PrayerActive(&p.Mobile, PrayerSmite) {
		t.Errorf("Smite should toggle off")
	}
	// Toggling an inactive prayer on.
	if !TogglePrayer(p, PrayerSmite) {
		t.Fatal("Smite toggle-on failed")
	}
	if !PrayerActive(&p.Mobile, PrayerSmite) {
		t.Errorf("Smite should toggle on")
	}
}

func TestPrayerRequiresPoints(t *testing.T) {
	p, fs := combatTestPlayer(t, "empty", 3000, 3000)
	p.Skills().SetCurrentLevel(SkillPrayer, 0, false)
	if ActivatePrayer(p, PrayerThickSkin) {
		t.Errorf("activation with 0 Prayer points should fail")
	}
	if len(fs.messages) == 0 || fs.messages[0] != "You do not have enough Prayer points." {
		t.Errorf("messages = %v", fs.messages)
	}
}

func TestProtectingPrayerMapping(t *testing.T) {
	if got := ProtectingPrayer(CombatTypeMelee); got != PrayerProtectFromMelee {
		t.Errorf("melee protection = %d, want 18", got)
	}
	if got := ProtectingPrayer(CombatTypeRanged); got != PrayerProtectFromMissiles {
		t.Errorf("ranged protection = %d, want 17", got)
	}
	if got := ProtectingPrayer(CombatTypeMagic); got != PrayerProtectFromMagic {
		t.Errorf("magic protection = %d, want 16", got)
	}
}

func TestPrayerDrainMath(t *testing.T) {
	p, _ := combatTestPlayer(t, "drainer", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	if !ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Fatal("activation failed")
	}
	// The activation tick is free.
	ProcessDrain(p)
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 99 {
		t.Errorf("activation tick drained: Prayer = %d", got)
	}
	// drainEffect 12, prayer bonus 0 -> resistance 60 ->
	// one point every 5 ticks.
	for i := 0; i < 4; i++ {
		ProcessDrain(p)
	}
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 99 {
		t.Errorf("Prayer after 4 drain ticks = %d, want 99", got)
	}
	ProcessDrain(p)
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 98 {
		t.Errorf("Prayer after 5 drain ticks = %d, want 98", got)
	}
	// Prayer bonus 30 -> resistance 120 -> one point per 10 ticks.
	defs := newCombatTestItemDefs()
	amulet := 300
	bonuses := make([]float64, 14)
	bonuses[13] = 30
	defs.bonuses[amulet] = bonuses
	p.SetItemDefinitions(defs)
	p.Equipment().Set(AmuletSlot, NewItem(amulet, 1))
	for i := 0; i < 9; i++ {
		ProcessDrain(p)
	}
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 98 {
		t.Errorf("Prayer after 9 bonus ticks = %d, want 98", got)
	}
	ProcessDrain(p)
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 97 {
		t.Errorf("Prayer after 10 bonus ticks = %d, want 97", got)
	}
}

func TestPrayerDrainExhaustion(t *testing.T) {
	p, fs := combatTestPlayer(t, "exhausted", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 1})
	if !ActivatePrayer(p, PrayerPiety) {
		t.Fatal("activation failed")
	}
	// Drain effect 24, resistance 60: the first ProcessDrain
	// consumes the free activation tick, then 3 draining
	// ticks accumulate 72 >= 60 and take the last point.
	for i := 0; i < 4; i++ {
		ProcessDrain(p)
	}
	if got := p.Skills().CurrentLevel(SkillPrayer); got != 0 {
		t.Fatalf("Prayer = %d, want 0", got)
	}
	ProcessDrain(p)
	if PrayerActive(&p.Mobile, PrayerPiety) {
		t.Errorf("Piety should deactivate when Prayer hits 0")
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != PrayerRunOutMessage {
		t.Errorf("run-out messages = %v", fs.messages)
	}
}

func TestPrayerHeadHint(t *testing.T) {
	p, _ := combatTestPlayer(t, "hinted", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	if !ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Fatal("activation failed")
	}
	if got := p.Appearance().HeadHint(); got != 0 {
		t.Errorf("melee head hint = %d, want 0", got)
	}
	DeactivatePrayers(&p.Mobile)
	if !ActivatePrayer(p, PrayerSmite) {
		t.Fatal("activation failed")
	}
	if got := p.Appearance().HeadHint(); got != 4 {
		t.Errorf("smite head hint = %d, want 4", got)
	}
	DeactivatePrayers(&p.Mobile)
	if got := p.Appearance().HeadHint(); got != -1 {
		t.Errorf("head hint after deactivate = %d, want -1", got)
	}
}

func TestDisableProtectionPrayers(t *testing.T) {
	setCombatCycle(t, 100)
	p, fs := combatTestPlayer(t, "disabled", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	if !ActivatePrayer(p, PrayerProtectFromMelee) {
		t.Fatal("activation failed")
	}
	DisableProtectionPrayers(p)
	if PrayerActive(&p.Mobile, PrayerProtectFromMelee) {
		t.Errorf("protection prayers should reset")
	}
	if p.GetCombat().PrayerBlockFinished() {
		t.Errorf("disable should start the prayer-block timer")
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != "You have been disabled and can no longer use protection prayers." {
		t.Errorf("messages = %v", fs.messages)
	}
	// A second disable during the block is a no-op.
	DisableProtectionPrayers(p)
	if len(fs.messages) != 1 {
		t.Errorf("second disable sent messages: %v", fs.messages)
	}
}

func TestDeadeyeAndMysticVigour(t *testing.T) {
	p, _ := combatTestPlayer(t, "upgraded", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillPrayer: 99})
	if !ActivatePrayer(p, PrayerEagleEye) {
		t.Fatal("activation failed")
	}
	if IsDeadeye(&p.Mobile) {
		t.Errorf("Eagle Eye without the unlock is not Deadeye")
	}
	p.SetAttribute("prayer:deadeye-unlocked", true)
	if !IsDeadeye(&p.Mobile) {
		t.Errorf("Eagle Eye + unlock should be Deadeye")
	}
	DeactivatePrayers(&p.Mobile)
	if !ActivatePrayer(p, PrayerMysticMight) {
		t.Fatal("activation failed")
	}
	p.SetAttribute("prayer:mystic-vigour-unlocked", true)
	if !IsMysticVigour(&p.Mobile) {
		t.Errorf("Mystic Might + unlock should be Mystic Vigour")
	}
}
