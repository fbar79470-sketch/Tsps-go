package game

import (
	"testing"
)

// Poison application in the hit pipeline
// (CombatFactory.executeHit, CombatFactory.ts:1086-1145):
// melee / ranged / NPC call sites, through ExecuteHit with
// fixed-damage hits and a fixed RNG.

// equipWeapon puts item id in the attacker's weapon slot.
func equipPoisonWeapon(t *testing.T, p *Player, id int) {
	t.Helper()
	p.Equipment().Set(WeaponSlot, NewItem(id, 1))
}

// equipAmmo puts item id in the attacker's ammo slot.
func equipPoisonAmmo(t *testing.T, p *Player, id int) {
	t.Helper()
	p.Equipment().Set(AmmunitionSlot, NewItem(id, 1))
}

func TestExecuteHitAppliesMeleePoison(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, vfs := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	equipPoisonWeapon(t, attacker, 5696) // rune dagger (p++), SUPER
	// The roll always applies (getRandom(range) === 0).
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ExecuteHit(fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 5))
	if !victim.IsPoisoned() || victim.PoisonDamage() != 30 {
		t.Fatalf("victim poison = %d poisoned = %v, want 30 true (SUPER melee severity)", victim.PoisonDamage(), victim.IsPoisoned())
	}
	if !containsMessage(vfs.messages, "You have been poisoned!") {
		t.Errorf("messages = %v, want the poisoned line", vfs.messages)
	}
	// A missed roll applies nothing.
	victim2, _ := combatTestPlayer(t, "victim2", 3001, 3000)
	setPlayerLevels(victim2, map[int]int{SkillHitpoints: 99})
	setFixedRandom(t, func(max int) int { return max }, 0)
	ExecuteHit(fixedDamageHit(t, &attacker.Mobile, &victim2.Mobile, 5))
	if victim2.IsPoisoned() {
		t.Errorf("missed roll poisoned victim2: damage = %d", victim2.PoisonDamage())
	}
	// An unpoisoned weapon never poisons, even on a hit roll.
	attacker2, _ := combatTestPlayer(t, "attacker2", 3000, 3000)
	equipPoisonWeapon(t, attacker2, 1213) // plain rune dagger
	victim3, _ := combatTestPlayer(t, "victim3", 3001, 3000)
	setPlayerLevels(victim3, map[int]int{SkillHitpoints: 99})
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ExecuteHit(fixedDamageHit(t, &attacker2.Mobile, &victim3.Mobile, 5))
	if victim3.IsPoisoned() {
		t.Errorf("plain weapon poisoned victim3: damage = %d", victim3.PoisonDamage())
	}
	// A zero-damage hit never poisons.
	victim4, _ := combatTestPlayer(t, "victim4", 3001, 3000)
	setPlayerLevels(victim4, map[int]int{SkillHitpoints: 99})
	ExecuteHit(fixedDamageHit(t, &attacker.Mobile, &victim4.Mobile, 0))
	if victim4.IsPoisoned() {
		t.Errorf("zero-damage hit poisoned victim4: damage = %d", victim4.PoisonDamage())
	}
}

func TestExecuteHitAppliesMeleeVenom(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, vfs := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	equipPoisonWeapon(t, attacker, 29796) // noxious halberd, VENOM
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ExecuteHit(fixedDamageHit(t, &attacker.Mobile, &victim.Mobile, 5))
	if !victim.IsVenomed() || victim.PoisonDamage() != 6 {
		t.Fatalf("victim venom = %d venomed = %v, want 6 true", victim.PoisonDamage(), victim.IsVenomed())
	}
	if !containsMessage(vfs.messages, "You have been poisoned by venom!") {
		t.Errorf("messages = %v, want the venom line", vfs.messages)
	}
}

func TestExecuteHitAppliesRangedPoison(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	equipPoisonWeapon(t, attacker, 857) // yew shortbow (no poison type)
	equipPoisonAmmo(t, attacker, 893)   // rune arrow (p), VERY_WEAK
	// The hit captures the fired shot from the resolved
	// ranged data (bow fires from the ammo slot).
	if weapon, _ := ResolveRangedData(attacker); weapon != nil {
		attacker.GetCombat().SetRangedWeapon(weapon)
	}
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	hit := fixedDamageRangedHit(t, &attacker.Mobile, &victim.Mobile, 5)
	ExecuteHit(hit)
	// Ranged VERY_WEAK severity is the default 6.
	if !victim.IsPoisoned() || victim.PoisonDamage() != 6 {
		t.Fatalf("victim poison = %d poisoned = %v, want 6 true (ranged default severity)", victim.PoisonDamage(), victim.IsPoisoned())
	}
	// A (p+) arrow is WEAK: ranged severity 11.
	victim2, _ := combatTestPlayer(t, "victim2", 3001, 3000)
	setPlayerLevels(victim2, map[int]int{SkillHitpoints: 99})
	equipPoisonAmmo(t, attacker, 5621) // rune arrow (p+), WEAK
	ExecuteHit(fixedDamageRangedHit(t, &attacker.Mobile, &victim2.Mobile, 5))
	if !victim2.IsPoisoned() || victim2.PoisonDamage() != 11 {
		t.Fatalf("victim2 poison = %d poisoned = %v, want 11 true (ranged WEAK severity)", victim2.PoisonDamage(), victim2.IsPoisoned())
	}
}

// fixedDamageRangedHit builds a fixed-damage ranged hit.
func fixedDamageRangedHit(t *testing.T, attacker, target *Mobile, damage int) *PendingHit {
	t.Helper()
	d := damage
	noAccuracy := false
	return NewPendingHitConfig(attacker, target, RangedMethod, PendingHitConfig{Damage: &d, Delay: 0, RollAccuracy: &noAccuracy})
}

func TestExecuteHitBlowpipeNeverPoisons(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	victim, _ := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	equipPoisonWeapon(t, attacker, 12926) // toxic blowpipe
	equipPoisonAmmo(t, attacker, 811)     // rune dart (p), VERY_WEAK
	if weapon, _ := ResolveRangedData(attacker); weapon != nil {
		attacker.GetCombat().SetRangedWeapon(weapon)
	}
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ExecuteHit(fixedDamageRangedHit(t, &attacker.Mobile, &victim.Mobile, 5))
	if victim.IsPoisoned() {
		t.Errorf("blowpipe shot poisoned victim: damage = %d (its venom belongs to the ToxicBlowpipe plugin)", victim.PoisonDamage())
	}
}

func TestExecuteHitNpcPoisonEvent(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	def := newCombatFakeDef()
	def.poisonous = true
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	npc.Hitpoints = 100
	victim, vfs := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	// The NPC roll always applies (getRandom(10) <= 5).
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	seen := false
	victim.EmitCustomEvent = func(name string, payload any) {
		if name == "combat:npc-poison" {
			seen = true
			event, ok := payload.(*NpcPoisonPayload)
			if !ok {
				t.Error("npc-poison payload is not *NpcPoisonPayload")
				return
			}
			if event.Severity != 30 || event.Venomous {
				t.Errorf("npc-poison event = severity %d venomous %v, want 30 false", event.Severity, event.Venomous)
			}
		}
	}
	ExecuteHit(fixedDamageHit(t, &npc.Mobile, &victim.Mobile, 5))
	if !seen {
		t.Error("combat:npc-poison event was not emitted")
	}
	if !victim.IsPoisoned() || victim.PoisonDamage() != 30 {
		t.Fatalf("victim poison = %d poisoned = %v, want 30 true", victim.PoisonDamage(), victim.IsPoisoned())
	}
	// A handler that sets handled suppresses the application.
	victim2, _ := combatTestPlayer(t, "victim2", 3001, 3000)
	setPlayerLevels(victim2, map[int]int{SkillHitpoints: 99})
	victim2.EmitCustomEvent = func(name string, payload any) {
		if name == "combat:npc-poison" {
			if event, ok := payload.(*NpcPoisonPayload); ok {
				event.Handled = true
			}
		}
	}
	ExecuteHit(fixedDamageHit(t, &npc.Mobile, &victim2.Mobile, 5))
	if victim2.IsPoisoned() {
		t.Errorf("handled npc-poison still applied: damage = %d", victim2.PoisonDamage())
	}
	_ = vfs
}

func TestExecuteHitVenomousNpcVenoms(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	def := newCombatFakeDef()
	def.venomous = true
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	npc.Hitpoints = 100
	victim, vfs := combatTestPlayer(t, "victim", 3001, 3000)
	setPlayerLevels(victim, map[int]int{SkillHitpoints: 99})
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ExecuteHit(fixedDamageHit(t, &npc.Mobile, &victim.Mobile, 5))
	if !victim.IsVenomed() || victim.PoisonDamage() != 6 {
		t.Fatalf("victim venom = %d venomed = %v, want 6 true", victim.PoisonDamage(), victim.IsVenomed())
	}
	if !containsMessage(vfs.messages, "You have been poisoned by venom!") {
		t.Errorf("messages = %v, want the venom line", vfs.messages)
	}
	// A non-poisonous NPC never poisons.
	def2 := newCombatFakeDef()
	plain := combatTestNPC(t, 2, 3000, 3000, def2)
	plain.Hitpoints = 100
	victim2, _ := combatTestPlayer(t, "victim2", 3001, 3000)
	setPlayerLevels(victim2, map[int]int{SkillHitpoints: 99})
	ExecuteHit(fixedDamageHit(t, &plain.Mobile, &victim2.Mobile, 5))
	if victim2.IsPoisoned() {
		t.Errorf("plain NPC poisoned victim2: damage = %d", victim2.PoisonDamage())
	}
}
