package game

import (
	"testing"
)

// Ancient Magicks combat spells (Phase 45, Worker P3):
// table verification against CombatSpells.ts:1720-2058,
// registration/resolution, autocast slots, CanCast, and
// each effect. Hand-built fixtures only: no cache, no
// network, no JSON data.

// installAncientSpells installs the Ancient-book table for
// one test and unregisters it when the test ends, so no
// spell leaks into another test's registry.
func installAncientSpells(t *testing.T) {
	t.Helper()
	InstallAncientSpells()
	t.Cleanup(uninstallAncientSpells)
}

// ancientHit builds an accurate PendingHit with the given
// damage, through the same structural assertion the magic
// method uses (combat_magic.go:85): OnHitCalc is not on the
// CombatSpell interface.
func ancientHit(t *testing.T, attacker *Player, target *Mobile, damage int) *PendingHit {
	t.Helper()
	hit := NewPendingHitConfig(&attacker.Mobile, target, MagicMethod, PendingHitConfig{})
	hit.accurate = true
	hit.totalDamage = damage
	return hit
}

// ancientOnHitCalc runs a spell's OnHitCalc on a hit built
// by ancientHit.
func ancientOnHitCalc(t *testing.T, spell CombatSpell, hit *PendingHit) {
	t.Helper()
	spellOnHitCalc(t, spell, hit)
}

func TestAncientSpellTableCounts(t *testing.T) {
	if len(ancientSpellDefs) != 16 {
		t.Fatalf("ancient spell defs = %d, want 16", len(ancientSpellDefs))
	}
	ids := map[int]bool{}
	kinds := map[ancientEffectKind]int{}
	radii := map[int]int{}
	for _, def := range ancientSpellDefs {
		if def.id <= 0 || ids[def.id] {
			t.Fatalf("bad or duplicate spell id %d (%s)", def.id, def.name)
		}
		ids[def.id] = true
		if def.level < 1 || def.maxHit < 1 || def.baseXP <= 0 || len(def.runes) == 0 || def.castAnim <= 0 {
			t.Fatalf("%s: incomplete def %+v", def.name, def)
		}
		if def.endGraphic < 0 {
			t.Fatalf("%s: no end graphic", def.name)
		}
		kinds[def.kind]++
		radii[def.radius]++
		switch def.kind {
		case ancientEffectSmoke:
			if def.poisonSeverity <= 0 {
				t.Fatalf("%s: no poison severity", def.name)
			}
		case ancientEffectShadow:
			if def.drainPercent <= 0 {
				t.Fatalf("%s: no drain percent", def.name)
			}
		case ancientEffectIce:
			if def.freezeTicks <= 0 {
				t.Fatalf("%s: no freeze ticks", def.name)
			}
		case ancientEffectBlood:
		default:
			t.Fatalf("%s: unknown kind %q", def.name, def.kind)
		}
	}
	if kinds[ancientEffectSmoke] != 4 || kinds[ancientEffectShadow] != 4 ||
		kinds[ancientEffectBlood] != 4 || kinds[ancientEffectIce] != 4 {
		t.Fatalf("kinds = %v, want 4 smoke / 4 shadow / 4 blood / 4 ice", kinds)
	}
	if radii[0] != 8 || radii[1] != 8 {
		t.Fatalf("radii = %v, want 8 radius-0 (Rush/Blitz) / 8 radius-1 (Burst/Barrage)", radii)
	}
}

// TestAncientSpellTableRows verifies every table row
// verbatim against CombatSpells.ts:1720-2058: id, level,
// max hit, base XP, rune cost in TS order, cast anim,
// start graphic, projectile, end graphic, radius, and the
// effect parameter.
func TestAncientSpellTableRows(t *testing.T) {
	installAncientSpells(t)
	// (name, id, level, maxHit, baseXP, runes flat
	// id/amount pairs in TS order, anim, start, proj, end,
	// radius, effect param: poison severity / drain percent
	// x100 / freeze ticks; blood carries 0).
	rows := []struct {
		name                   string
		id, level, maxHit      int
		baseXP                 int
		runes                  []int
		anim, start, proj, end int
		radius, effect         int
	}{
		{"SMOKE_RUSH", 12939, 50, 13, 30, []int{556, 1, 554, 1, 562, 2, 560, 2}, 1978, -1, 384, 385, 0, 10},
		{"SHADOW_RUSH", 12987, 52, 14, 31, []int{556, 1, 566, 1, 562, 2, 560, 2}, 1978, -1, 378, 379, 0, 10},
		{"BLOOD_RUSH", 12901, 56, 15, 33, []int{565, 1, 562, 2, 560, 2}, 1978, -1, 372, 373, 0, 0},
		{"ICE_RUSH", 12861, 58, 16, 34, []int{555, 2, 562, 2, 560, 2}, 1978, -1, 360, 361, 0, 8},
		{"SMOKE_BURST", 12963, 62, 17, 36, []int{556, 2, 554, 2, 562, 4, 560, 2}, 1979, -1, -1, 389, 1, 10},
		{"SHADOW_BURST", 13011, 64, 18, 37, []int{556, 1, 566, 2, 562, 4, 560, 2}, 1979, -1, -1, 382, 1, 10},
		{"BLOOD_BURST", 12919, 68, 21, 39, []int{565, 2, 562, 4, 560, 2}, 1979, -1, -1, 376, 1, 0},
		{"ICE_BURST", 12881, 70, 22, 40, []int{555, 4, 562, 4, 560, 2}, 1979, -1, -1, 363, 1, 16},
		{"SMOKE_BLITZ", 12951, 74, 23, 42, []int{556, 2, 554, 2, 565, 2, 560, 2}, 1978, -1, 386, 387, 0, 20},
		{"SHADOW_BLITZ", 12999, 76, 24, 43, []int{556, 2, 566, 2, 565, 2, 560, 2}, 1978, -1, 380, 381, 0, 15},
		{"BLOOD_BLITZ", 12911, 80, 25, 45, []int{565, 4, 560, 2}, 1978, -1, 374, 375, 0, 0},
		{"ICE_BLITZ", 12871, 82, 26, 46, []int{555, 3, 565, 2, 560, 2}, 1978, 366, -1, 367, 0, 24},
		{"SMOKE_BARRAGE", 12975, 86, 27, 48, []int{556, 4, 554, 4, 565, 2, 560, 4}, 1979, -1, -1, 391, 1, 20},
		{"SHADOW_BARRAGE", 13023, 88, 28, 49, []int{556, 4, 566, 3, 565, 2, 560, 4}, 1979, -1, -1, 383, 1, 15},
		{"BLOOD_BARRAGE", 12929, 92, 29, 51, []int{560, 4, 566, 1, 565, 4}, 1979, -1, -1, 377, 1, 0},
		{"ICE_BARRAGE", 12891, 94, 30, 52, []int{555, 6, 565, 2, 560, 4}, 1979, -1, -1, 369, 1, 32},
	}
	for _, row := range rows {
		spell := GetCombatSpell(row.id)
		if spell == nil {
			t.Fatalf("%s (%d): not registered", row.name, row.id)
		}
		if spell.SpellID() != row.id || spell.MaximumHit() != row.maxHit ||
			spell.BaseExperience() != row.baseXP || spell.SplashExperience() != row.baseXP {
			t.Fatalf("%s: got id=%d maxHit=%d baseXP=%d splashXP=%d, want id=%d maxHit=%d baseXP=%d splashXP=%d",
				row.name, spell.SpellID(), spell.MaximumHit(), spell.BaseExperience(),
				spell.SplashExperience(), row.id, row.maxHit, row.baseXP, row.baseXP)
		}
		if got := spell.AttackSpeed(); got != 5 {
			t.Fatalf("%s: attack speed %d, want 5", row.name, got)
		}
		if got := spell.HitDelay(); got != -1 {
			t.Fatalf("%s: hit delay %d, want -1", row.name, got)
		}
		if got := spell.SpellRadius(); got != row.radius {
			t.Fatalf("%s: radius %d, want %d", row.name, got, row.radius)
		}
		// The table def carries the level, presentation
		// ids, runes and effect parameter.
		var def *ancientSpellDef
		for i := range ancientSpellDefs {
			if ancientSpellDefs[i].id == row.id {
				def = &ancientSpellDefs[i]
				break
			}
		}
		if def == nil {
			t.Fatalf("%s: no table def", row.name)
		}
		if def.name != row.name {
			t.Fatalf("id %d: def name %q, want %q", row.id, def.name, row.name)
		}
		if def.level != row.level || def.castAnim != row.anim ||
			def.startGraphic != row.start || def.projectile != row.proj || def.endGraphic != row.end {
			t.Fatalf("%s: table def = level %d anim %d start %d proj %d end %d, want level %d anim %d start %d proj %d end %d",
				row.name, def.level, def.castAnim, def.startGraphic, def.projectile, def.endGraphic,
				row.level, row.anim, row.start, row.proj, row.end)
		}
		// Only Ice Blitz has a start graphic, at HIGH height
		// (new Graphic(366, GraphicHeight.HIGH)); the rest
		// have none.
		wantStartHeight := -1
		if row.id == 12871 {
			wantStartHeight = graphicHeightHigh
		}
		if def.startGraphicHeight != wantStartHeight {
			t.Fatalf("%s: start graphic height %d, want %d", row.name, def.startGraphicHeight, wantStartHeight)
		}
		if len(def.runes) != len(row.runes)/2 {
			t.Fatalf("%s: %d runes, want %d", row.name, len(def.runes), len(row.runes)/2)
		}
		for i, r := range def.runes {
			if r.id != row.runes[2*i] || r.amount != row.runes[2*i+1] {
				t.Fatalf("%s: rune %d = %d x%d, want %d x%d", row.name, i, r.id, r.amount, row.runes[2*i], row.runes[2*i+1])
			}
		}
		switch def.kind {
		case ancientEffectSmoke:
			if def.poisonSeverity != row.effect {
				t.Fatalf("%s: poison %d, want %d", row.name, def.poisonSeverity, row.effect)
			}
		case ancientEffectShadow:
			if int(def.drainPercent*100+0.5) != row.effect {
				t.Fatalf("%s: drain %v, want %d%%", row.name, def.drainPercent, row.effect)
			}
		case ancientEffectIce:
			if def.freezeTicks != row.effect {
				t.Fatalf("%s: freeze %d, want %d", row.name, def.freezeTicks, row.effect)
			}
		case ancientEffectBlood:
			if row.effect != 0 {
				t.Fatalf("%s: blood carries effect %d, want 0", row.name, row.effect)
			}
		}
		// The end graphic is reachable through the shared
		// impact-time lookup (combat_magic.go), at LOW
		// height (bare `new Graphic(id)` in TS).
		if id, height, ok := spellEndGraphic(spell); !ok || id != row.end || height != graphicHeightLow {
			t.Fatalf("%s: end graphic = %d/%d/%v, want %d/%d/true", row.name, id, height, ok, row.end, graphicHeightLow)
		}
		// The spellbook is ANCIENT, through the spell's own
		// answer and through spellbookOfSpell.
		if s, ok := spell.(interface{ Spellbook() Spellbook }); !ok || s.Spellbook() != SpellbookAncient {
			t.Fatalf("%s: Spellbook() is not Ancient", row.name)
		}
		if got := spellbookOfSpell(spell); got != SpellbookAncient {
			t.Fatalf("%s: spellbookOfSpell = %v, want Ancient", row.name, got)
		}
	}
}

func TestAncientSpellByName(t *testing.T) {
	installAncientSpells(t)
	cases := []struct {
		name string
		id   int
	}{
		{"Smoke Rush", 12939},
		{"SMOKE_RUSH", 12939},
		{"smoke rush", 12939},
		{"Ice Barrage", 12891},
		{"ICE_BARRAGE", 12891},
		{"Blood Blitz", 12911},
		{"Shadow Barrage", 13023},
	}
	for _, c := range cases {
		spell := GetCombatSpellByName(c.name)
		if spell == nil || spell.SpellID() != c.id {
			t.Fatalf("GetCombatSpellByName(%q) = %v, want id %d", c.name, spell, c.id)
		}
	}
	// The Ancient teleports are NOT registered (no teleport
	// system exists in Go).
	if spell := GetCombatSpellByName("Paddewwa Teleport"); spell != nil {
		t.Fatalf("GetCombatSpellByName(Paddewwa Teleport) = %v, want nil", spell.SpellID())
	}
}

func TestAncientAutocastSlots(t *testing.T) {
	installAncientSpells(t)
	// Slots 31-46 already mapped in autocasting.go and
	// resolve nil until the spells register; registration
	// lights them up.
	slots := map[int]int{
		31: 12939, 32: 12987, 33: 12901, 34: 12861,
		35: 12963, 36: 13011, 37: 12919, 38: 12881,
		39: 12951, 40: 12999, 41: 12911, 42: 12871,
		43: 12975, 44: 13023, 45: 12929, 46: 12891,
	}
	for slot, want := range slots {
		spell := AutocastSpell(slot)
		if spell == nil || spell.SpellID() != want {
			t.Fatalf("AutocastSpell(%d) = %v, want id %d", slot, spell, want)
		}
	}
	// The Arceuus slots stay nil (that book has not landed).
	for _, slot := range []int{53, 58} {
		if spell := AutocastSpell(slot); spell != nil {
			t.Fatalf("AutocastSpell(%d) = %v, want nil (Arceuus not ported)", slot, spell.SpellID())
		}
	}
}

func TestAncientSpellCanCast(t *testing.T) {
	installAncientSpells(t)
	resetCombatHooks(t)
	// Level refusal: Magic 49 cannot cast Smoke Rush (50).
	p := spellTestPlayer(t, 49)
	p.SetSpellbook(SpellbookAncient)
	rush := GetCombatSpell(12939)
	if rush.CanCast(p, false) {
		t.Fatalf("level-49 player may cast Smoke Rush")
	}
	// Wrong book refusal: level 99 on the Normal book.
	p = spellTestPlayer(t, 99)
	p.SetSpellbook(SpellbookNormal)
	if rush.CanCast(p, false) {
		t.Fatalf("Smoke Rush castable on the Normal book")
	}
	// Right book + runes: castable, and consume spends the
	// exact rune cost (air 1, fire 1, chaos 2, death 2).
	p.SetSpellbook(SpellbookAncient)
	giveRunes(t, p, 556, 1, 554, 1, 562, 2, 560, 2)
	if !rush.CanCast(p, false) {
		t.Fatalf("Smoke Rush refused with level + book + runes")
	}
	if !rush.CanCast(p, true) {
		t.Fatalf("Smoke Rush refused on consume")
	}
	for _, id := range []int{556, 554, 562, 560} {
		if got := p.Inventory().GetAmount(id); got != 0 {
			t.Fatalf("rune %d left %d after consume, want 0", id, got)
		}
	}
	// Short one death rune: refused.
	p = spellTestPlayer(t, 99)
	p.SetSpellbook(SpellbookAncient)
	giveRunes(t, p, 556, 1, 554, 1, 562, 2, 560, 1)
	if rush.CanCast(p, false) {
		t.Fatalf("Smoke Rush castable short a death rune")
	}
	// Ice Barrage (level 94) at 99: castable.
	p = spellTestPlayer(t, 99)
	p.SetSpellbook(SpellbookAncient)
	giveRunes(t, p, 555, 6, 565, 2, 560, 4)
	barrage := GetCombatSpell(12891)
	if !barrage.CanCast(p, false) {
		t.Fatalf("Ice Barrage refused with level + book + runes")
	}
}

func TestAncientSmokePoison(t *testing.T) {
	installAncientSpells(t)
	attacker := spellTestPlayer(t, 99)
	target := spellTestNPC(t, false, true)
	rush := GetCombatSpell(12939) // Smoke Rush, poison 10.
	// A splash applies nothing.
	hit := ancientHit(t, attacker, &target.Mobile, 10)
	hit.accurate = false
	ancientOnHitCalc(t, rush, hit)
	if target.IsPoisoned() {
		t.Fatalf("splash poisoned the target")
	}
	// The 1-in-8 roll landing poisons for 10 (Rush/Burst).
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target.Mobile, 10))
	if !target.IsPoisoned() || target.PoisonDamage() != 10 {
		t.Fatalf("Smoke Rush poison = %v/%d, want poisoned/10", target.IsPoisoned(), target.PoisonDamage())
	}
	// The roll missing poisons nothing.
	target2 := spellTestNPC(t, false, true)
	setFixedRandom(t, func(max int) int { return 1 }, 0)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target2.Mobile, 10))
	if target2.IsPoisoned() {
		t.Fatalf("Smoke Rush poisoned on a missed 1-in-8 roll")
	}
	// Blitz/Barrage poison for 20.
	blitz := GetCombatSpell(12951) // Smoke Blitz.
	setFixedRandom(t, func(max int) int { return 0 }, 0)
	target3 := spellTestNPC(t, false, true)
	ancientOnHitCalc(t, blitz, ancientHit(t, attacker, &target3.Mobile, 20))
	if !target3.IsPoisoned() || target3.PoisonDamage() != 20 {
		t.Fatalf("Smoke Blitz poison = %v/%d, want poisoned/20", target3.IsPoisoned(), target3.PoisonDamage())
	}
	// A vetoed hit-effects event applies nothing.
	attacker.EmitCustomEvent = func(name string, payload any) {
		if name == "magic:hit-effects" {
			payload.(*MagicHitEffectsPayload).Allowed = false
		}
	}
	target4 := spellTestNPC(t, false, true)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target4.Mobile, 10))
	if target4.IsPoisoned() {
		t.Fatalf("vetoed Smoke Rush poisoned the target")
	}
}

func TestAncientShadowDrain(t *testing.T) {
	installAncientSpells(t)
	attacker := spellTestPlayer(t, 99)
	victim := spellTestPlayer(t, 99)
	setPlayerLevels(victim, map[int]int{SkillAttack: 60})
	rush := GetCombatSpell(12987) // Shadow Rush, drain 10%.
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &victim.Mobile, 10))
	if got := victim.Skills().CurrentLevel(SkillAttack); got != 54 {
		t.Fatalf("Shadow Rush drain: attack %d, want 54", got)
	}
	// Already lowered: no second drain until the stat recovers.
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &victim.Mobile, 10))
	if got := victim.Skills().CurrentLevel(SkillAttack); got != 54 {
		t.Fatalf("Shadow Rush reapplied below max: attack %d, want 54", got)
	}
	// A splash drains nothing.
	victim2 := spellTestPlayer(t, 99)
	setPlayerLevels(victim2, map[int]int{SkillAttack: 60})
	hit := ancientHit(t, attacker, &victim2.Mobile, 10)
	hit.accurate = false
	ancientOnHitCalc(t, rush, hit)
	if got := victim2.Skills().CurrentLevel(SkillAttack); got != 60 {
		t.Fatalf("Shadow Rush splash drained: attack %d, want 60", got)
	}
	// Blitz/Barrage drain 15% (60 -> 51).
	blitz := GetCombatSpell(12999) // Shadow Blitz.
	victim3 := spellTestPlayer(t, 99)
	setPlayerLevels(victim3, map[int]int{SkillAttack: 60})
	ancientOnHitCalc(t, blitz, ancientHit(t, attacker, &victim3.Mobile, 10))
	if got := victim3.Skills().CurrentLevel(SkillAttack); got != 51 {
		t.Fatalf("Shadow Blitz drain: attack %d, want 51", got)
	}
	// An NPC target takes no drain and does not panic (the
	// NPC branch has no Go seam; see shadowAttackDrain).
	npc := spellTestNPC(t, false, true)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &npc.Mobile, 10))
}

func TestAncientBloodHeal(t *testing.T) {
	installAncientSpells(t)
	attacker := spellTestPlayer(t, 99)
	setPlayerLevels(attacker, map[int]int{SkillHitpoints: 99})
	attacker.Skills().SetCurrentLevel(SkillHitpoints, 50, false)
	target := spellTestNPC(t, false, true)
	rush := GetCombatSpell(12901) // Blood Rush, heal 25%.
	// 20 damage heals 5.
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target.Mobile, 20))
	if got := attacker.Skills().CurrentLevel(SkillHitpoints); got != 55 {
		t.Fatalf("Blood Rush heal: hitpoints %d, want 55", got)
	}
	// The heal floors: 21 damage heals 5, not 6.
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target.Mobile, 21))
	if got := attacker.Skills().CurrentLevel(SkillHitpoints); got != 60 {
		t.Fatalf("Blood Rush heal after 21 damage: hitpoints %d, want 60", got)
	}
	// The heal caps at max hitpoints.
	attacker.Skills().SetCurrentLevel(SkillHitpoints, 98, false)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target.Mobile, 20))
	if got := attacker.Skills().CurrentLevel(SkillHitpoints); got != 99 {
		t.Fatalf("Blood Rush heal at cap: hitpoints %d, want 99", got)
	}
	// A splash heals nothing, and a 0-damage hit heals nothing.
	attacker.Skills().SetCurrentLevel(SkillHitpoints, 50, false)
	hit := ancientHit(t, attacker, &target.Mobile, 20)
	hit.accurate = false
	ancientOnHitCalc(t, rush, hit)
	ancientOnHitCalc(t, rush, ancientHit(t, attacker, &target.Mobile, 0))
	if got := attacker.Skills().CurrentLevel(SkillHitpoints); got != 50 {
		t.Fatalf("Blood Rush healed on splash/0-hit: hitpoints %d, want 50", got)
	}
	// FinishCast is the no-op spellEffect (the heal runs in
	// OnHitCalc): it heals nothing on its own.
	rush.FinishCast(&attacker.Mobile, &target.Mobile, true, 20)
	if got := attacker.Skills().CurrentLevel(SkillHitpoints); got != 50 {
		t.Fatalf("Blood Rush FinishCast healed: hitpoints %d, want 50", got)
	}
}

func TestAncientIceFreeze(t *testing.T) {
	installAncientSpells(t)
	setCombatCycle(t, 100)
	attacker := spellTestPlayer(t, 99)
	recorded := captureEvents(attacker)
	// Rush 8, Burst 16, Blitz 24, Barrage 32 ticks.
	cases := []struct {
		id    int
		ticks int
	}{
		{12861, 8},  // ICE_RUSH
		{12881, 16}, // ICE_BURST
		{12871, 24}, // ICE_BLITZ
		{12891, 32}, // ICE_BARRAGE
	}
	for _, c := range cases {
		target, fs := combatTestPlayer(t, "frozen", 3200, 3200)
		spell := GetCombatSpell(c.id)
		before := len(*recorded)
		ancientOnHitCalc(t, spell, ancientHit(t, attacker, &target.Mobile, 10))
		if !target.GetTimers().Has(TimerKeyFreeze) {
			t.Fatalf("Ice spell %d: no FREEZE timer", c.id)
		}
		if got := target.GetTimers().Left(TimerKeyFreeze); got != c.ticks {
			t.Fatalf("Ice spell %d: freeze %d ticks, want %d", c.id, got, c.ticks)
		}
		if got := target.GetTimers().Left(TimerKeyFreezeImmunity); got != c.ticks+5 {
			t.Fatalf("Ice spell %d: freeze immunity %d, want %d", c.id, got, c.ticks+5)
		}
		// The freeze's own message is suppressed (the red
		// one is the Freezes consumer's, on combat:frozen):
		// the target hears nothing from the spell itself.
		if len(fs.messages) != 0 {
			t.Fatalf("Ice spell %d: target messages = %v, want none", c.id, fs.messages)
		}
		// A landed freeze emits exactly one combat:frozen
		// with the target, the caster, the ticks applied
		// and kind "ice" — the Freezes consumer's trigger.
		events := (*recorded)[before:]
		if len(events) != 3 ||
			events[0][0] != "magic:hit-effects" || events[1][0] != CombatFrozenEventName ||
			events[2][0] != "magic:hit-effects-applied" {
			t.Fatalf("Ice spell %d: events = %v, want [magic:hit-effects combat:frozen magic:hit-effects-applied]", c.id, eventNames(&events))
		}
		frozen, ok := events[1][1].(*CombatFrozenPayload)
		if !ok || frozen == nil {
			t.Fatalf("Ice spell %d: combat:frozen payload = %T, want *CombatFrozenPayload", c.id, events[1][1])
		}
		if frozen.Target != &target.Mobile || frozen.Source != &attacker.Mobile ||
			frozen.Ticks != c.ticks || frozen.Kind != "ice" {
			t.Fatalf("Ice spell %d: combat:frozen payload = %+v, want target/caster/%d/ice", c.id, frozen, c.ticks)
		}
	}
	// A splash freezes nothing and emits no combat:frozen.
	target, _ := combatTestPlayer(t, "splashed", 3200, 3200)
	before := len(*recorded)
	hit := ancientHit(t, attacker, &target.Mobile, 10)
	hit.accurate = false
	ancientOnHitCalc(t, GetCombatSpell(12861), hit)
	if target.GetTimers().Has(TimerKeyFreeze) {
		t.Fatalf("Ice Rush splash froze the target")
	}
	for _, e := range (*recorded)[before:] {
		if e[0] == CombatFrozenEventName {
			t.Fatalf("Ice Rush splash emitted combat:frozen")
		}
	}
	// A freeze-resistant NPC takes no freeze and no event.
	def := newCombatFakeDef()
	def.freezeResist = 100
	immune := combatTestNPC(t, 1, 3200, 3200, def)
	before = len(*recorded)
	ancientOnHitCalc(t, GetCombatSpell(12861), ancientHit(t, attacker, &immune.Mobile, 10))
	if immune.GetTimers().Has(TimerKeyFreeze) {
		t.Fatalf("Ice Rush froze a freeze-resistant NPC")
	}
	for _, e := range (*recorded)[before:] {
		if e[0] == CombatFrozenEventName {
			t.Fatalf("Ice Rush on a resistant NPC emitted combat:frozen")
		}
	}
	// Resistance shortens the freeze, and the event carries
	// the ticks actually applied (8 at 50% resistance -> 4).
	def = newCombatFakeDef()
	def.freezeResist = 50
	resistant := combatTestNPC(t, 2, 3200, 3200, def)
	before = len(*recorded)
	ancientOnHitCalc(t, GetCombatSpell(12861), ancientHit(t, attacker, &resistant.Mobile, 10))
	if got := resistant.GetTimers().Left(TimerKeyFreeze); got != 4 {
		t.Fatalf("Ice Rush at 50%% resistance: freeze %d ticks, want 4", got)
	}
	frozen := false
	for _, e := range (*recorded)[before:] {
		if e[0] == CombatFrozenEventName {
			frozen = true
			if p, ok := e[1].(*CombatFrozenPayload); !ok || p.Ticks != 4 {
				t.Fatalf("Ice Rush at 50%% resistance: combat:frozen ticks = %+v, want 4", e[1])
			}
		}
	}
	if !frozen {
		t.Fatalf("Ice Rush at 50%% resistance emitted no combat:frozen")
	}
}

func TestAncientHitEffectsEvents(t *testing.T) {
	installAncientSpells(t)
	attacker := spellTestPlayer(t, 99)
	target := spellTestNPC(t, false, true)
	spell := GetCombatSpell(12939)
	recorded := captureEvents(attacker)
	ancientOnHitCalc(t, spell, ancientHit(t, attacker, &target.Mobile, 10))
	names := eventNames(recorded)
	if len(names) != 2 || names[0] != "magic:hit-effects" || names[1] != "magic:hit-effects-applied" {
		t.Fatalf("hit-effects events = %v, want [magic:hit-effects magic:hit-effects-applied]", names)
	}
	effect := (*recorded)[0][1].(*MagicHitEffectsPayload)
	if !effect.Allowed || effect.EffectMultiplier != 1 || effect.Hit == nil {
		t.Fatalf("hit-effects payload = %+v, want Allowed + multiplier 1", effect)
	}
	if effect.Spell != spell {
		t.Fatalf("hit-effects spell = %v, want the Smoke Rush spell", effect.Spell)
	}
	// A splash emits nothing (onHitCalc returns early, as in TS).
	recorded2 := captureEvents(attacker)
	hit := ancientHit(t, attacker, &target.Mobile, 10)
	hit.accurate = false
	ancientOnHitCalc(t, spell, hit)
	if len(*recorded2) != 0 {
		t.Fatalf("splash emitted %d events, want 0", len(*recorded2))
	}
}

func TestAncientBarrageMultiTarget(t *testing.T) {
	installAncientSpells(t)
	resetCombatHooks(t)
	setCombatCycle(t, 100)
	// Max rolls, so the 99-magic attacker lands the primary
	// hit on the weak NPC.
	setFixedRandom(t, func(max int) int { return max }, 0)
	SetCombatMultiSource(func(c Combatant) bool { return true })
	t.Cleanup(func() { SetCombatMultiSource(nil) })
	attacker, _ := combatTestPlayer(t, "barrager", 3200, 3200)
	setPlayerLevels(attacker, map[int]int{SkillMagic: 99})
	attacker.SetSpellbook(SpellbookAncient)
	primary := combatTestNPC(t, 1, 3200, 3200, nil)
	secondary := combatTestNPC(t, 2, 3201, 3200, nil)
	// The production wiring is the world scan; the unit seam
	// is the hook, stubbed here to the secondary.
	Hooks.SpellAreaTargets = func(attacker, target *Mobile, radius int) []*Mobile {
		return []*Mobile{&secondary.Mobile}
	}
	barrage := GetCombatSpell(12975) // Smoke Barrage, radius 1.
	attacker.GetCombat().SetCastSpell(barrage)
	hits := MagicMethod.Hits(&attacker.Mobile, &primary.Mobile)
	if len(hits) != 2 {
		t.Fatalf("Smoke Barrage hits = %d, want 2 (primary + secondary)", len(hits))
	}
	if hits[0].Target() != &primary.Mobile || hits[1].Target() != &secondary.Mobile {
		t.Fatalf("barrage targets = %v/%v, want primary/secondary", hits[0].Target(), hits[1].Target())
	}
	if hits[1].Spell() != barrage {
		t.Fatalf("secondary hit spell = %v, want Smoke Barrage", hits[1].Spell())
	}
	if hits[1].HandleAfterHitEffects() {
		t.Fatalf("secondary hit runs after-hit effects (TS: handleAfterHitEffects false)")
	}
	// A Rush (radius 0) with the same stub produces one hit.
	rush := GetCombatSpell(12939)
	attacker.GetCombat().SetCastSpell(rush)
	hits = MagicMethod.Hits(&attacker.Mobile, &primary.Mobile)
	if len(hits) != 1 {
		t.Fatalf("Smoke Rush hits = %d, want 1", len(hits))
	}
	// A Barrage with no area hook produces one hit.
	Hooks.SpellAreaTargets = nil
	attacker.GetCombat().SetCastSpell(barrage)
	hits = MagicMethod.Hits(&attacker.Mobile, &primary.Mobile)
	if len(hits) != 1 {
		t.Fatalf("Smoke Barrage without SpellAreaTargets = %d hits, want 1", len(hits))
	}
}
