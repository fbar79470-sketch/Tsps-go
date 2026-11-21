package game

import "testing"

// Combat presentation tests (Phase 29, Worker B). All fixtures
// are hand-built: no cache, no network, no JSON data files.
// Animation/graphic state is read through Mobile.Animation()
// and Mobile.Graphic() (Worker A's accessors); sounds are
// read through the recording soundFakeSession.

// TestMeleeStartPlaysAttackAnimation is MeleeCombatMethod.
// start (MeleeCombatMethod.ts:9-14): the attack animation
// plays, skipped when -1.
func TestMeleeStartPlaysAttackAnimation(t *testing.T) {
	attacker := combatTestNPC(t, 1, 3000, 3000, newCombatFakeDef())
	target, _ := combatTestPlayer(t, "target", 3001, 3000)
	MeleeMethod.Start(&attacker.Mobile, &target.Mobile)
	if anim := attacker.Mobile.Animation(); anim == nil || anim.ID != 422 {
		t.Fatalf("NPC attack animation = %v, want 422", anim)
	}
	// An attack animation of -1 plays nothing.
	attacker2 := combatTestNPC(t, 1, 3000, 3000, func() *combatFakeDef {
		def := newCombatFakeDef()
		def.attackAnim = -1
		return def
	}())
	MeleeMethod.Start(&attacker2.Mobile, &target.Mobile)
	if anim := attacker2.Mobile.Animation(); anim != nil {
		t.Fatalf("attack animation -1 played %v, want none", anim)
	}
}

// TestMeleeAttackPlaysBlockAnimation is Combat.ts:1054-1055:
// a melee swing plays the target's block animation at
// launch — unless the target has none (-1).
func TestMeleeAttackPlaysBlockAnimation(t *testing.T) {
	advance := setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	attacker, _ := combatTestPlayer(t, "attacker", 3000, 3000)
	def := newCombatFakeDef()
	def.defenceAnim = 404
	def.hitpoints = 99
	target := combatTestNPC(t, 1, 3001, 3000, def)
	target.Hitpoints = 99
	attacker.GetCombat().Attack(&target.Mobile)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	if anim := target.Mobile.Animation(); anim == nil || anim.ID != 404 {
		t.Fatalf("target block animation = %v, want 404", anim)
	}
	if anim := attacker.Mobile.Animation(); anim == nil || anim.ID != 422 {
		t.Fatalf("attacker attack animation = %v, want 422 (unarmed punch)", anim)
	}
	// A target with no block animation (-1) plays none.
	// Cycle 104: the first swing's cooldown (attack speed
	// 4) has expired.
	advance(104)
	def2 := newCombatFakeDef()
	def2.defenceAnim = -1
	def2.hitpoints = 99
	target2 := combatTestNPC(t, 1, 3001, 3000, def2)
	target2.Hitpoints = 99
	attacker.GetCombat().Attack(&target2.Mobile)
	attacker.GetCombat().PreMovementProcess()
	attacker.GetCombat().PostMovementProcess()
	if anim := target2.Mobile.Animation(); anim != nil {
		t.Fatalf("block animation -1 played %v, want none", anim)
	}
}

// TestRangedStartPlaysAttackAnimationAndFireSound is
// RangedCombatMethod.start (RangedCombatMethod.ts:133-183):
// the attack animation, then the fire sound (SHOOT_ARROW
// for an unarmed fallback).
func TestRangedStartPlaysAttackAnimationAndFireSound(t *testing.T) {
	attacker := combatTestNPC(t, 1, 3000, 3000, newCombatFakeDef())
	target, _ := combatTestPlayer(t, "target", 3005, 3000)
	RangedMethod.Start(&attacker.Mobile, &target.Mobile)
	if anim := attacker.Mobile.Animation(); anim == nil || anim.ID != 422 {
		t.Fatalf("NPC ranged attack animation = %v, want 422", anim)
	}
	// A player with no ranged weapon resolved still swings
	// (fight-type animation) and fires the fallback sound.
	p, _ := combatTestPlayer(t, "archer", 3000, 3000)
	soundSess := newSoundFakeSession()
	p.Session = soundSess
	RangedMethod.Start(&p.Mobile, &target.Mobile)
	if anim := p.Mobile.Animation(); anim == nil {
		t.Fatalf("player ranged attack played no animation")
	}
	if len(soundSess.effects) != 1 || soundSess.effects[0].soundID != 2702 {
		t.Fatalf("fire sound effects = %v, want one 2702 (SHOOT_ARROW)", soundSess.effects)
	}
}

// TestMagicStartCastWindStrikeNoStaff is CombatSpell.startCast
// (CombatSpell.ts:50-101) for Wind Strike (1152) with NO staff
// wielded: cast animation 711 (the base cast anim — the staff
// variant is 1162, see the WithStaff test below), start graphic
// 90 at HIGH, projectile 91, and the queued end-graphic for
// legacy-magic timing.
func TestMagicStartCastWindStrikeNoStaff(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	caster := spellTestPlayer(t, 99)
	castSess := newSoundFakeSession()
	caster.Session = castSess
	target := combatTestNPC(t, 1, 3203, 3200, newCombatFakeDef())
	giveRunes(t, caster, 556, 1, 558, 1)
	spell := GetCombatSpell(1152)
	if !spell.CanCast(caster, true) {
		t.Fatalf("paid Wind Strike refused")
	}
	caster.GetCombat().SetCastSpell(spell)
	MagicMethod.Start(&caster.Mobile, &target.Mobile)
	if len(castSess.effects) != 1 || castSess.effects[0].soundID != 220 {
		t.Fatalf("cast sound effects = %v, want one 220 (Wind Strike cast)", castSess.effects)
	}
	if anim := caster.Mobile.Animation(); anim == nil || anim.ID != 711 {
		t.Fatalf("cast animation = %v, want 711", anim)
	}
	if gfx := caster.Mobile.Graphic(); gfx == nil || gfx.ID != 90 || gfx.Height != 100 {
		t.Fatalf("start graphic = %v, want 90 at height 100", gfx)
	}
	// The legacy-magic visual is queued on the paid cast's
	// context (CombatSpell.ts:89-102).
	ctx := spellCastContext(caster, spell)
	visual := getProjectileVisual(spell, ctx, &target.Mobile)
	if visual == nil {
		t.Fatalf("no projectile visual queued for Wind Strike")
	}
	if visual.endCycleOffset != 76 {
		t.Fatalf("end cycle offset = %d, want 76 (46 + 10 x 3 tiles)", visual.endCycleOffset)
	}
	// hits() queues the end graphic on the target with the
	// projectile's landing delay — and the impact sound to
	// a listening player.
	hearer, hearSess := soundTestPlayer("hearer", Location{X: 3203, Y: 3200, Z: 0})
	_ = hearer
	oldSource := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{hearer} })
	t.Cleanup(func() { SetNetworkPlayerSource(oldSource) })
	hits := MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 92 || gfx.Delay != 76 {
		t.Fatalf("queued end graphic = %v, want 92 at delay 76", gfx)
	}
	if !visual.queued {
		t.Fatalf("visual not marked queued after hits()")
	}
	if len(hearSess.areas) != 1 || hearSess.areas[0].soundID != 221 || hearSess.areas[0].delay != 76 {
		t.Fatalf("queued impact sound = %v, want one 221 at delay 76", hearSess.areas)
	}
	// The impact does not replay a queued graphic or sound.
	MagicMethod.HandleAfterHitEffects(hits[0])
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 92 || gfx.Delay != 76 {
		t.Fatalf("impact replayed the graphic: %v, want 92 at delay 76", gfx)
	}
	if len(hearSess.areas) != 1 {
		t.Fatalf("impact replayed the sound: %v", hearSess.areas)
	}
}

// TestMagicStartCastWindStrikeWithStaff is the staff variant
// (CombatSpell.ts:42-47, STAFF_CAST_ANIMATIONS 711 -> 1162):
// with a staff of air wielded, Wind Strike plays cast
// animation 1162, not the base 711. Live-verified in the
// Phase 29 smoke (the staff-variant anim is what the real
// client shows).
func TestMagicStartCastWindStrikeWithStaff(t *testing.T) {
	installStandardSpells(t)
	installFixtureStyles(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	caster := spellTestPlayer(t, 99)
	caster.SetItemDefinitions(&styleTestDefs{
		testItemDefs: &testItemDefs{},
		interfaces:   map[int]string{1381: "STAFF"},
	})
	equipWeapon(caster, 1381)
	target := combatTestNPC(t, 1, 3203, 3200, newCombatFakeDef())
	giveRunes(t, caster, 556, 1, 558, 1)
	spell := GetCombatSpell(1152)
	if !spell.CanCast(caster, true) {
		t.Fatalf("paid Wind Strike refused")
	}
	caster.GetCombat().SetCastSpell(spell)
	MagicMethod.Start(&caster.Mobile, &target.Mobile)
	if anim := caster.Mobile.Animation(); anim == nil || anim.ID != 1162 {
		t.Fatalf("staff cast animation = %v, want 1162 (STAFF_CAST_ANIMATIONS 711 -> 1162)", anim)
	}
	if gfx := caster.Mobile.Graphic(); gfx == nil || gfx.ID != 90 || gfx.Height != 100 {
		t.Fatalf("start graphic = %v, want 90 at height 100", gfx)
	}
}

// TestMagicImpactGraphicWithoutQueue is the impact-time
// path (MagicCombatMethod.ts:358-372): with no queued
// visual (no paid-cast context), the end graphic plays at
// the impact tick, delay 0.
func TestMagicImpactGraphicWithoutQueue(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	caster := spellTestPlayer(t, 99)
	target := combatTestNPC(t, 1, 3203, 3200, newCombatFakeDef())
	spell := GetCombatSpell(1152)
	// No CanCast: no paid-cast context, so no visual queues.
	spell.StartCast(&caster.Mobile, &target.Mobile)
	if anim := caster.Mobile.Animation(); anim == nil || anim.ID != 711 {
		t.Fatalf("cast animation = %v, want 711", anim)
	}
	caster.GetCombat().SetCastSpell(spell)
	hits := MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if gfx := target.Mobile.Graphic(); gfx != nil {
		t.Fatalf("unpaid cast queued a graphic: %v, want none", gfx)
	}
	MagicMethod.HandleAfterHitEffects(hits[0])
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 92 || gfx.Delay != 0 {
		t.Fatalf("impact graphic = %v, want 92 at delay 0", gfx)
	}
}

// TestMagicNPCNoSpellCast is MagicCombatMethod.start's NPC
// branch (MagicCombatMethod.ts:268-282): the attack
// animation and the default wind-strike projectile shape.
// The projectile send needs no network source to be safe.
func TestMagicNPCNoSpellCast(t *testing.T) {
	def := newCombatFakeDef()
	def.attackAnim = 422
	def.projectileID = -1
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	target, _ := combatTestPlayer(t, "target", 3005, 3000)
	MagicMethod.Start(&npc.Mobile, &target.Mobile)
	if anim := npc.Mobile.Animation(); anim == nil || anim.ID != 422 {
		t.Fatalf("NPC cast animation = %v, want 422", anim)
	}
}

// TestMagicNPCWithSpellCast is CombatSpell.startCast for an
// NPC caster (CombatSpell.ts:50-101, MagicCombatMethod.ts:
// 261-300): an NPC holding a spell object casts it — the
// spell's cast animation (711, the base animation: the
// staff variant is player-only), its start graphic (90 at
// HIGH), and its projectile (91) — with the cast sound,
// and no base XP owed (TS adds it only when !isNpc).
func TestMagicNPCWithSpellCast(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	def := newCombatFakeDef()
	def.attackAnim = 422
	def.projectileID = -1
	npc := combatTestNPC(t, 1, 3000, 3000, def)
	target, sess := rangedProjectileTestPlayer(t, "target", 3003, 3000)
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{target} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	spell := GetCombatSpell(1152)
	npc.GetCombat().SetCastSpell(spell)
	MagicMethod.Start(&npc.Mobile, &target.Mobile)
	if anim := npc.Mobile.Animation(); anim == nil || anim.ID != 711 {
		t.Fatalf("NPC cast animation = %v, want 711", anim)
	}
	if gfx := npc.Mobile.Graphic(); gfx == nil || gfx.ID != 90 || gfx.Height != 100 {
		t.Fatalf("NPC start graphic = %v, want 90 at height 100", gfx)
	}
	if len(sess.sends) != 1 || sess.sends[0].id != 91 {
		t.Fatalf("NPC spell projectile sends = %v, want one 91", sess.sends)
	}
	if npc.GetCombat().takeBaseExperienceOwed() {
		t.Error("NPC cast marked base XP owed; TS owes it only for players")
	}
	// The impact-time path still plays the end graphic: an
	// NPC never queues the projectile visual
	// (CombatSpell.ts:89-102, player-only), so the hit's
	// HandleAfterHitEffects replays it at delay 0.
	hits := MagicMethod.Hits(&npc.Mobile, &target.Mobile)
	if len(hits) != 1 {
		t.Fatalf("NPC spell hits = %d, want 1", len(hits))
	}
	MagicMethod.HandleAfterHitEffects(hits[0])
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 92 || gfx.Delay != 0 {
		t.Fatalf("NPC spell impact graphic = %v, want 92 at delay 0", gfx)
	}
}

// TestMagicNPCWithSpellCastSkipsStartGraphic is the
// start-graphic exclusion in CombatSpell.startCast
// (CombatSpell.ts:60-72): NPC ids 2000, 109, 3580 and 2007
// cast without a start graphic, but the animation and
// projectile still play.
func TestMagicNPCWithSpellCastSkipsStartGraphic(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	def := newCombatFakeDef()
	def.attackAnim = 422
	def.projectileID = -1
	npc := combatTestNPC(t, 2000, 3000, 3000, def)
	target, sess := rangedProjectileTestPlayer(t, "target", 3003, 3000)
	old := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{target} })
	t.Cleanup(func() { SetNetworkPlayerSource(old) })
	npc.GetCombat().SetCastSpell(GetCombatSpell(1152))
	MagicMethod.Start(&npc.Mobile, &target.Mobile)
	if anim := npc.Mobile.Animation(); anim == nil || anim.ID != 711 {
		t.Fatalf("excluded NPC cast animation = %v, want 711", anim)
	}
	if gfx := npc.Mobile.Graphic(); gfx != nil {
		t.Fatalf("excluded NPC start graphic = %v, want none", gfx)
	}
	if len(sess.sends) != 1 || sess.sends[0].id != 91 {
		t.Fatalf("excluded NPC spell projectile sends = %v, want one 91", sess.sends)
	}
}

// TestMagicQueuedGraphicLosesPriority is the queued-flag
// priority check (MagicCombatMethod.ts:211: visual.queued =
// target.getGraphic() === delayed): when a higher-priority
// graphic already sits on the target, the queued end
// graphic loses, queued stays false, and the impact-time
// path replays it instead of skipping it.
func TestMagicQueuedGraphicLosesPriority(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	setFixedRandom(t, func(max int) int { return max }, 0.5)
	caster := spellTestPlayer(t, 99)
	target := combatTestNPC(t, 1, 3203, 3200, newCombatFakeDef())
	giveRunes(t, caster, 556, 1, 558, 1)
	spell := GetCombatSpell(1152)
	if !spell.CanCast(caster, true) {
		t.Fatalf("paid Wind Strike refused")
	}
	caster.GetCombat().SetCastSpell(spell)
	MagicMethod.Start(&caster.Mobile, &target.Mobile)
	ctx := spellCastContext(caster, spell)
	visual := getProjectileVisual(spell, ctx, &target.Mobile)
	if visual == nil {
		t.Fatalf("no projectile visual queued for Wind Strike")
	}
	// A higher-priority graphic already on the target: the
	// delayed end graphic (LOW) cannot replace it.
	if !target.Mobile.performGraphicValue(&Graphic{ID: 999, Height: 100, Delay: 0, Priority: AnimationPriorityHigh}) {
		t.Fatalf("could not set the blocking graphic")
	}
	hearer, hearSess := soundTestPlayer("hearer", Location{X: 3203, Y: 3200, Z: 0})
	_ = hearer
	oldSource := networkPlayerSource
	SetNetworkPlayerSource(func() []*Player { return []*Player{hearer} })
	t.Cleanup(func() { SetNetworkPlayerSource(oldSource) })
	hits := MagicMethod.Hits(&caster.Mobile, &target.Mobile)
	if len(hits) != 1 {
		t.Fatalf("hits = %d, want 1", len(hits))
	}
	if visual.queued {
		t.Error("visual marked queued although the delayed graphic lost priority")
	}
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 999 {
		t.Fatalf("blocking graphic = %v, want 999 still standing", gfx)
	}
	// The impact sound was still sent once, delayed to the
	// projectile's landing, as in TS (the sound half of
	// queueProjectileGraphic runs before the graphic).
	if len(hearSess.areas) != 1 || hearSess.areas[0].soundID != 221 {
		t.Fatalf("queued impact sound = %v, want one 221", hearSess.areas)
	}
	// The impact-time path runs (the graphic was not
	// queued): it replays the end graphic — which loses
	// again to the blocking HIGH graphic, as TS's
	// performGraphic would — and does not replay the
	// already-queued sound.
	MagicMethod.HandleAfterHitEffects(hits[0])
	if gfx := target.Mobile.Graphic(); gfx == nil || gfx.ID != 999 {
		t.Fatalf("blocking graphic after impact = %v, want 999 still standing", gfx)
	}
	if len(hearSess.areas) != 1 {
		t.Fatalf("impact replayed the sound: %v", hearSess.areas)
	}
}

// TestDeathAnimations is the death presentation (Phase 29):
// the player death animation 836 with the player:dying
// event (PlayerDeath.ts case 4), and the NPC definition
// death anim (NPCDeathTask.ts:44).
func TestDeathAnimations(t *testing.T) {
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	// Player: lethal damage starts the death sequence.
	p, _ := combatTestPlayer(t, "doomed", 3000, 3000)
	setPlayerLevels(p, map[int]int{SkillHitpoints: 99})
	var dying *PlayerDyingEvent
	p.EmitCustomEvent = func(name string, payload any) {
		if name == PlayerDyingEventName {
			dying, _ = payload.(*PlayerDyingEvent)
		}
	}
	DecrementHealth(&p.Mobile, NewHitDamage(99, HitMaskRed))
	if anim := p.Mobile.Animation(); anim == nil || anim.ID != 836 {
		t.Fatalf("player death animation = %v, want 836", anim)
	}
	if dying == nil || dying.Player != p {
		t.Fatalf("player:dying event = %v, want one for the player", dying)
	}
	// NPC: the definition's death anim.
	def := newCombatFakeDef()
	def.deathAnim = 1234
	def.hitpoints = 10
	npc := combatTestNPC(t, 1, 3001, 3000, def)
	npc.Hitpoints = 10
	DecrementHealth(&npc.Mobile, NewHitDamage(10, HitMaskRed))
	if anim := npc.Mobile.Animation(); anim == nil || anim.ID != 1234 {
		t.Fatalf("NPC death animation = %v, want 1234", anim)
	}
}

// TestChargeCastPresentation is ChargeSpell.cast
// (CombatSpells.ts:68-70): animation 811 and graphic 111
// at delay 32, height 128, with the synth sound 1651.
func TestChargeCastPresentation(t *testing.T) {
	installStandardSpells(t)
	setCombatCycle(t, 100)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	soundSess := newSoundFakeSession()
	p.Session = soundSess
	// The Mage Arena gate answers "received" so the cast
	// proceeds to the presentation.
	p.EmitCustomEvent = func(name string, payload any) {
		if q, ok := payload.(*MageArenaCapeReceivedPayload); ok {
			received := true
			q.Received = &received
		}
	}
	giveRunes(t, p, 556, 3, 554, 3, 565, 3)
	if !chargeSpellInstance.cast(p) {
		t.Fatalf("Charge cast refused")
	}
	if anim := p.Mobile.Animation(); anim == nil || anim.ID != 811 {
		t.Fatalf("Charge animation = %v, want 811", anim)
	}
	if gfx := p.Mobile.Graphic(); gfx == nil || gfx.ID != 111 || gfx.Height != 128 || gfx.Delay != 32 {
		t.Fatalf("Charge graphic = %v, want 111 at height 128, delay 32", gfx)
	}
	if len(soundSess.effects) != 1 || soundSess.effects[0].soundID != 1651 {
		t.Fatalf("Charge sound effects = %v, want one 1651", soundSess.effects)
	}
}
