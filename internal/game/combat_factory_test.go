package game

import "testing"

func fixedDamageHit(t *testing.T, attacker, target *Mobile, damage int) *PendingHit {
	t.Helper()
	return fixedDamageHitDelayed(t, attacker, target, damage, 0)
}

func fixedDamageHitDelayed(t *testing.T, attacker, target *Mobile, damage, delay int) *PendingHit {
	t.Helper()
	d := damage
	noAccuracy := false
	return NewPendingHitConfig(attacker, target, MeleeMethod, PendingHitConfig{Damage: &d, Delay: delay, RollAccuracy: &noAccuracy})
}

func TestProtectionPrayerReduction(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	// Player attacker: 17 -> floor(17*0.6) = 10.
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99, SkillPrayer: 99})
	if !ActivatePrayer(victim, PrayerProtectFromMelee) {
		t.Fatal("ActivatePrayer failed")
	}
	hit := fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 17)
	ApplyProtectionPrayer(hit)
	if got := hit.TotalDamage(); got != 10 {
		t.Errorf("player->player protection: total = %d, want 10", got)
	}
	// NPC attacker: protection nullifies.
	npc := combatTestNPC(t, 1, 3000, 3000, nil)
	hit2 := fixedDamageHit(t, &npc.Mobile, &victim.Mobile, 17)
	ApplyProtectionPrayer(hit2)
	if got := hit2.TotalDamage(); got != 0 {
		t.Errorf("NPC->player protection: total = %d, want 0", got)
	}
	// Bypassed hits ignore the prayer.
	d := 17
	noAccuracy := false
	hit3 := NewPendingHitConfig(&attacker.Mobile, &victim.Mobile, MeleeMethod,
		PendingHitConfig{Damage: &d, BypassProtectionPrayer: true, RollAccuracy: &noAccuracy})
	ApplyProtectionPrayer(hit3)
	if got := hit3.TotalDamage(); got != 17 {
		t.Errorf("bypassed protection: total = %d, want 17", got)
	}
}

func TestValidTargetCases(t *testing.T) {
	setCombatCycle(t, 100)
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	b, _ := combatTestPlayer(t, "b", 3001, 3000)
	if !ValidTarget(&a.Mobile, &b.Mobile) {
		t.Errorf("adjacent registered players should be valid targets")
	}
	if ValidTarget(&a.Mobile, &a.Mobile) {
		t.Errorf("self is never a valid target")
	}
	if ValidTarget(&a.Mobile, nil) {
		t.Errorf("nil is never a valid target")
	}
	c, _ := combatTestPlayer(t, "c", 3001, 3000)
	c.SetRegistered(false)
	if ValidTarget(&a.Mobile, &c.Mobile) {
		t.Errorf("unregistered target is invalid")
	}
	d, _ := combatTestPlayer(t, "d", 3001, 3000)
	d.Skills().SetCurrentLevel(SkillHitpoints, 0, false)
	if ValidTarget(&a.Mobile, &d.Mobile) {
		t.Errorf("0-HP target is invalid")
	}
	e, _ := combatTestPlayer(t, "e", 3001, 3000)
	e.SetLocation(Location{X: 3001, Y: 3000, Z: 1})
	if ValidTarget(&a.Mobile, &e.Mobile) {
		t.Errorf("cross-floor target is invalid (no CrossFloor)")
	}
	f, _ := combatTestPlayer(t, "f", 3040, 3000)
	if ValidTarget(&a.Mobile, &f.Mobile) {
		t.Errorf("target 40+ tiles away is invalid")
	}
	// Owner-only NPC: another player's NPC refuses with the message.
	owner, _ := combatTestPlayer(t, "owner", 3000, 3000)
	npc := combatTestNPC(t, 5, 3001, 3000, nil)
	npc.SetOwner(owner)
	stranger, sfs := combatTestPlayer(t, "stranger", 3002, 3000)
	if ValidTarget(&stranger.Mobile, &npc.Mobile) {
		t.Errorf("another player's owner-only NPC is invalid")
	}
	if len(sfs.messages) != 1 || sfs.messages[0] != "This npc was not spawned for you." {
		t.Errorf("owner-only refusal messages = %v", sfs.messages)
	}
	if !ValidTarget(&owner.Mobile, &npc.Mobile) {
		t.Errorf("the owner's own NPC is a valid target")
	}
}

func TestMultiCombatBetween(t *testing.T) {
	setCombatCycle(t, 100)
	a, _ := combatTestPlayer(t, "a", 3000, 3000)
	b, _ := combatTestPlayer(t, "b", 3001, 3000)
	if MultiCombatBetween(&a.Mobile, &b.Mobile) {
		t.Errorf("two players, no multi: expected single-combat")
	}
	// Always-multi NPC on either side.
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	npc.SetMultiCombat(true)
	if !MultiCombatBetween(&a.Mobile, &npc.Mobile) {
		t.Errorf("always-multi NPC should make the pair multi")
	}
	// Both in multi via the override.
	SetCombatMultiSource(func(c Combatant) bool { return true })
	t.Cleanup(func() { SetCombatMultiSource(nil) })
	if !MultiCombatBetween(&a.Mobile, &b.Mobile) {
		t.Errorf("both in multi: expected multi-combat")
	}
}

func TestCanAttackPermissionBusy(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	other, _ := combatTestPlayer(t, "other", 3002, 3000)
	// Attacker busy: attacked by someone else.
	attacker.GetCombat().SetUnderAttack(&other.Mobile)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackAlreadyUnderAttack {
		t.Errorf("busy attacker = %v, want ALREADY_UNDER_ATTACK", got)
	}
	attacker.GetCombat().SetUnderAttack(nil)
	// Target busy: attacked by someone else.
	target.GetCombat().SetUnderAttack(&other.Mobile)
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackTargetInCombat {
		t.Errorf("busy target = %v, want TARGET_IN_COMBAT", got)
	}
	target.GetCombat().SetUnderAttack(nil)
	// Free pair, policy allowed by the hook: CAN_ATTACK.
	allow := true
	Hooks.OnCanAttack = func(attacker, target *Mobile, method CombatMethod) *bool { return &allow }
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackOK {
		t.Errorf("free pair = %v, want CAN_ATTACK", got)
	}
	// PvP default policy denies without a hook opinion.
	Hooks.OnCanAttack = nil
	if got := CanAttackPermission(&attacker.Mobile, &target.Mobile, true, MeleeMethod); got != CanAttackCantAttackInArea {
		t.Errorf("PvP default = %v, want CANT_ATTACK_IN_AREA", got)
	}
}

func TestCanAttackPermissionStunAndImmunity(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	npc := combatTestNPC(t, 1, 3001, 3000, nil)
	attacker.GetTimers().Registers(TimerKeyStun, 5)
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackStunned {
		t.Errorf("stunned attacker = %v, want STUNNED", got)
	}
	attacker.GetTimers().Cancel(TimerKeyStun)
	npc.GetTimers().Registers(TimerKeyAttackImmunity, 5)
	if got := CanAttackPermission(&attacker.Mobile, &npc.Mobile, true, MeleeMethod); got != CanAttackTargetIsImmune {
		t.Errorf("immune target = %v, want TARGET_IS_IMMUNE", got)
	}
}

func TestRewardExpVectors(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	newHit := func(attacker *Player, combatType CombatType, skills []int, damage int, accurate bool) *PendingHit {
		hit := NewPendingHit(&attacker.Mobile, &attacker.Mobile, MeleeMethod, 0)
		hit.combatType = combatType
		hit.experienceSkills = skills
		hit.totalDamage = damage
		hit.accurate = accurate
		hit.experience = true
		return hit
	}
	// Melee Accurate, 10 damage: Attack 40, Hitpoints 13.
	p, _ := combatTestPlayer(t, "melee", 3000, 3000)
	RewardExp(p, newHit(p, CombatTypeMelee, nil, 10, true))
	if got := p.Skills().Experience(SkillAttack); got != 40 {
		t.Errorf("melee Attack XP = %d, want 40", got)
	}
	if got := p.Skills().Experience(SkillHitpoints); got != 13+1184 {
		t.Errorf("melee Hitpoints XP = %d, want %d", got, 13+1184)
	}
	// Magic accurate with a cast spell: Magic 20 + HP 13.
	m, _ := combatTestPlayer(t, "mage", 3000, 3000)
	spell := newFakeSpell()
	m.GetCombat().SetCastSpell(spell)
	magicHit := newHit(m, CombatTypeMagic, []int{SkillMagic}, 10, true)
	magicHit.method = MagicMethod
	RewardExp(m, magicHit)
	if got := m.Skills().Experience(SkillMagic); got != 20 {
		t.Errorf("magic Magic XP = %d, want 20", got)
	}
	// Defensive magic split: Magic 13 + Defence 10 + HP 13.
	d, _ := combatTestPlayer(t, "defmage", 3000, 3000)
	d.GetCombat().SetCastSpell(spell)
	splitHit := newHit(d, CombatTypeMagic, []int{SkillMagic, SkillDefence}, 10, true)
	splitHit.method = MagicMethod
	RewardExp(d, splitHit)
	if got := d.Skills().Experience(SkillMagic); got != 13 {
		t.Errorf("split Magic XP = %d, want 13", got)
	}
	if got := d.Skills().Experience(SkillDefence); got != 10 {
		t.Errorf("split Defence XP = %d, want 10", got)
	}
	// Zero damage pays nothing.
	z, _ := combatTestPlayer(t, "zero", 3000, 3000)
	RewardExp(z, newHit(z, CombatTypeMelee, nil, 0, false))
	if got := z.Skills().Experience(SkillAttack); got != 0 {
		t.Errorf("zero-damage Attack XP = %d, want 0", got)
	}
}

func TestRecoil(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	wearer, _ := combatTestPlayer(t, "wearer", 3000, 3000)
	attacker, _ := combatTestPlayer(t, "attacker", 3001, 3000)
	wearer.Equipment().Set(RingSlot, NewItem(RingOfRecoilID, 1))
	if !WearingRecoilRing(wearer) {
		t.Fatal("expected to wear a recoil ring")
	}
	if got := RecoilDamage(50); got != 6 {
		t.Errorf("RecoilDamage(50) = %d, want 6", got)
	}
	if got := RecoilDamage(0); got != 0 {
		t.Errorf("RecoilDamage(0) = %d, want 0", got)
	}
	HandleRecoil(wearer, &attacker.Mobile, 50)
	if got := attacker.GetCombat().HitQueue().QueuedDamage(); got != 6 {
		t.Errorf("queued recoil = %d, want 6", got)
	}
	if got := attributeInt(wearer.GetAttribute(RecoilDamageAttribute)); got != 6 {
		t.Errorf("recoil charges used = %d, want 6", got)
	}
	// Shatter: 39 used, 1 left; the last recoil deals 1.
	wearer.SetAttribute(RecoilDamageAttribute, 39)
	HandleRecoil(wearer, &attacker.Mobile, 50)
	ring := wearer.Equipment().ItemAt(RingSlot)
	if ring.IsValid() {
		t.Errorf("ring should have shattered, still wearing %v", ring)
	}
	if got := attributeInt(wearer.GetAttribute(RecoilDamageAttribute)); got != 0 {
		t.Errorf("charges after shatter = %d, want 0", got)
	}
}

func TestVengeance(t *testing.T) {
	setCombatCycle(t, 100)
	victim, _ := combatTestPlayer(t, "victim", 3000, 3000)
	attacker, _ := combatTestPlayer(t, "attacker", 3001, 3000)
	victim.GetCombat().SetVengeance(true)
	HandleVengeance(&victim.Mobile, &attacker.Mobile, 10)
	if got := attacker.GetCombat().HitQueue().QueuedDamage(); got != 7 {
		t.Errorf("vengeance damage = %d, want 7", got)
	}
	if victim.GetCombat().HasVengeance() {
		t.Errorf("vengeance should clear after rebounding")
	}
	if chat := victim.GetForcedChat(); chat == nil || *chat != "Taste vengeance!" {
		t.Errorf("force chat = %v, want Taste vengeance!", chat)
	}
	// Immune attacker: rebound spent, nothing dealt, still chats.
	victim.GetCombat().SetVengeance(true)
	attacker.SetAttribute(RecoilImmuneAttribute, true)
	HandleVengeance(&victim.Mobile, &attacker.Mobile, 10)
	if got := attacker.GetCombat().HitQueue().QueuedDamage(); got != 7 {
		t.Errorf("immune vengeance queued = %d, want 7 (unchanged)", got)
	}
	if victim.GetCombat().HasVengeance() {
		t.Errorf("vengeance should clear even when immune")
	}
}

func TestSmiteAndRedemption(t *testing.T) {
	setCombatCycle(t, 100)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillPrayer: 10})
	HandleSmite(&attacker.Mobile, victim, 9)
	if got := victim.Skills().CurrentLevel(SkillPrayer); got != 8 {
		t.Errorf("smite drained Prayer to %d, want 8", got)
	}
	// Redemption: HP 9/100, Prayer max 40 -> heal 10, prayer 0.
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 100, SkillPrayer: 40})
	victim.Skills().SetCurrentLevel(SkillHitpoints, 9, false)
	if !ActivatePrayer(victim, PrayerRedemption) {
		t.Fatal("ActivatePrayer(Redemption) failed")
	}
	HandleRedemption(&victim.Mobile)
	if got := victim.Skills().CurrentLevel(SkillHitpoints); got != 19 {
		t.Errorf("redemption HP = %d, want 19", got)
	}
	if got := victim.Skills().CurrentLevel(SkillPrayer); got != 0 {
		t.Errorf("redemption Prayer = %d, want 0", got)
	}
	if PrayerActive(&victim.Mobile, PrayerRedemption) {
		t.Errorf("redemption should deactivate all prayers")
	}
}

func TestSkullAttackedBy(t *testing.T) {
	advance := setCombatCycle(t, 100)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	RecordAttackedBy(target, attacker)
	if !WasAttackedBy(target, attacker) {
		t.Errorf("expected attacked-by record")
	}
	advance(199)
	if !WasAttackedBy(target, attacker) {
		t.Errorf("record should last 100 ticks")
	}
	advance(200)
	if WasAttackedBy(target, attacker) {
		t.Errorf("record should expire after 100 ticks")
	}
}

func TestHandleSkull(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	SetCombatWildernessSource(func(loc Location) (bool, bool) { return true, false })
	t.Cleanup(func() { SetCombatWildernessSource(nil) })
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	skulled := false
	Hooks.ApplySkull = func(player *Player, seconds int) {
		skulled = true
		if seconds != PVPSkullSeconds {
			t.Errorf("skull seconds = %d, want %d", seconds, PVPSkullSeconds)
		}
	}
	HandleSkull(attacker, target)
	if !skulled {
		t.Errorf("expected a skull in the wilderness")
	}
	if !WasAttackedBy(target, attacker) {
		t.Errorf("expected the attacked-by record")
	}
}

func TestStunAndFreezeTicks(t *testing.T) {
	setCombatCycle(t, 100)
	p, fs := combatTestPlayer(t, "stunned", 3000, 3000)
	StunTicks(&p.Mobile, 5, false, true)
	if !p.GetTimers().Has(TimerKeyStun) {
		t.Errorf("STUN timer missing")
	}
	if got := p.GetTimers().Left(TimerKeyStunImmunity); got != 6 {
		t.Errorf("STUN_IMMUNITY = %d, want 6", got)
	}
	if len(fs.messages) != 1 || fs.messages[0] != "You've been stunned!" {
		t.Errorf("stun messages = %v", fs.messages)
	}
	// Immunity grace: a second stun is refused.
	StunTicks(&p.Mobile, 5, true, false)
	if got := p.GetTimers().Left(TimerKeyStun); got != 5 {
		t.Errorf("stun during immunity changed the timer: %d", got)
	}
	// Freeze: FREEZE + FREEZE_IMMUNITY (ticks+5).
	q, _ := combatTestPlayer(t, "frozen", 3000, 3000)
	FreezeTicks(&q.Mobile, 10, nil, nil, "")
	if !q.GetTimers().Has(TimerKeyFreeze) {
		t.Errorf("FREEZE timer missing")
	}
	if got := q.GetTimers().Left(TimerKeyFreezeImmunity); got != 15 {
		t.Errorf("FREEZE_IMMUNITY = %d, want 15", got)
	}
	// Freeze resistance 100: immune.
	def := newCombatFakeDef()
	def.freezeResist = 100
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	FreezeTicks(&npc.Mobile, 10, nil, nil, "")
	if npc.GetTimers().Has(TimerKeyFreeze) {
		t.Errorf("freeze-resistant NPC should not freeze")
	}
	// Size > 2 without the flag or resistance: no freeze.
	big := newCombatFakeDef()
	big.fakeNPCDefinition.size = 3
	bigNPC := combatTestNPC(t, 2, 3000, 3000, big)
	FreezeTicks(&bigNPC.Mobile, 10, nil, nil, "")
	if bigNPC.GetTimers().Has(TimerKeyFreeze) {
		t.Errorf("large NPC without the freezable flag should not freeze")
	}
}

func TestCheckAmmoAndDecrement(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	weapon := &RangedWeaponData{AmmoIDs: []int{1000}}
	ammo := &AmmunitionData{ItemID: 1000}
	RegisterAmmunitionData(ammo)
	RegisterRangedDataResolver(func(player *Player) (*RangedWeaponData, *AmmunitionData) {
		return weapon, ammo
	})
	t.Cleanup(func() { RegisterRangedDataResolver(nil) })
	p, fs := combatTestPlayer(t, "archer", 3000, 3000)
	resolveRangedData(p)
	p.Equipment().Set(AmmunitionSlot, NewItem(1000, 10))
	if !CheckAmmo(p, 1, false) {
		t.Errorf("matching ammo should pass the gate")
	}
	// Wrong ammo: the bow line + reset.
	p.Equipment().Set(AmmunitionSlot, NewItem(2000, 10))
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	p.GetCombat().Attack(&target.Mobile)
	if CheckAmmo(p, 1, false) {
		t.Errorf("wrong ammo should fail the gate")
	}
	if p.GetCombat().Target() != nil {
		t.Errorf("failed ammo check should reset combat")
	}
	if len(fs.messages) == 0 || fs.messages[len(fs.messages)-1] != "You can't use that ammo with your bow." {
		t.Errorf("wrong-ammo messages = %v", fs.messages)
	}
	// Decrement: a break roll pays without a floor drop.
	p.Equipment().Set(AmmunitionSlot, NewItem(1000, 10))
	dropped := 0
	Hooks.DropGroundItem = func(item Item, loc Location, owner *Player) { dropped += item.Amount() }
	setFixedRandom(t, func(max int) int { return 10 }, 0.5) // < 20: break
	DecrementAmmo(p, Location{X: 3001, Y: 3000}, 1, 1)
	if got := p.Equipment().ItemAt(AmmunitionSlot).Amount(); got != 9 {
		t.Errorf("broken ammo left %d, want 9", got)
	}
	if dropped != 0 {
		t.Errorf("broken ammo dropped %d, want 0", dropped)
	}
	// A drop roll pays and drops.
	setFixedRandom(t, func(max int) int { return 50 }, 0.5) // 20..99: drop
	DecrementAmmo(p, Location{X: 3001, Y: 3000}, 1, 1)
	if got := p.Equipment().ItemAt(AmmunitionSlot).Amount(); got != 8 {
		t.Errorf("dropped ammo left %d, want 8", got)
	}
	if dropped != 1 {
		t.Errorf("dropped ammo = %d, want 1", dropped)
	}
}

func TestGetMethodResolution(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p, _ := combatTestPlayer(t, "fighter", 3000, 3000)
	if got := GetMethod(&p.Mobile); got.Type() != CombatTypeMelee {
		t.Errorf("default method = %v, want MELEE", got.Type())
	}
	spell := newFakeSpell()
	p.GetCombat().SetCastSpell(spell)
	if got := GetMethod(&p.Mobile); got.Type() != CombatTypeMagic {
		t.Errorf("cast spell method = %v, want MAGIC", got.Type())
	}
	p.GetCombat().SetCastSpell(nil)
	// Ranged resolver.
	RegisterRangedDataResolver(func(player *Player) (*RangedWeaponData, *AmmunitionData) {
		return &RangedWeaponData{}, &AmmunitionData{}
	})
	t.Cleanup(func() { RegisterRangedDataResolver(nil) })
	if got := GetMethod(&p.Mobile); got.Type() != CombatTypeRanged {
		t.Errorf("ranged weapon method = %v, want RANGED", got.Type())
	}
	// NPC: definition attack type.
	def := newCombatFakeDef()
	def.attackType = 1 // ranged (definition.CombatTypeRanged)
	npc := combatTestNPC(t, 9, 3000, 3000, def)
	if got := GetMethod(&npc.Mobile); got.Type() != CombatTypeRanged {
		t.Errorf("NPC ranged method = %v, want RANGED", got.Type())
	}
}
