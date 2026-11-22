package game

import (
	"math"
	"strings"
	"testing"
)

// Hand-built fixtures only: no cache, no network, no JSON data.
// The spell tables are verified against the upstream TS source
// by count + spot-checks here (the extraction itself is
// brace-counted and verbatim; see combat_spells_data.go).

// installStandardSpells installs the standard-book and
// utility tables for one test and unregisters them when the
// test ends, so no spell leaks into another test's registry.
func installStandardSpells(t *testing.T) {
	t.Helper()
	InstallStandardSpells()
	t.Cleanup(uninstallStandardSpells)
}

// spellTestPlayer returns a player with the given Magic level
// (current and max) and an empty inventory/equipment.
func spellTestPlayer(t *testing.T, magic int) *Player {
	t.Helper()
	p := NewPlayer("spelltester", Location{X: 3200, Y: 3200, Z: 0})
	setPlayerLevels(p, map[int]int{SkillMagic: magic})
	// Bind the player combatant identity: the spell code
	// resolves casters/targets through combatantFor
	// (combatant.go), which reads nil for an unbound Mobile.
	p.GetCombat()
	return p
}

// giveRunes adds the spell's rune cost to the player's
// inventory from (id, amount) pairs.
func giveRunes(t *testing.T, p *Player, runes ...int) {
	t.Helper()
	if len(runes)%2 != 0 {
		t.Fatalf("giveRunes: odd arg count")
	}
	for i := 0; i < len(runes); i += 2 {
		p.Inventory().Adds(runes[i], runes[i+1])
	}
}

// captureEvents installs an EmitCustomEvent sink on p that
// records (name, payload) pairs, and returns the recorder.
func captureEvents(p *Player) *[][2]any {
	recorded := &[][2]any{}
	p.EmitCustomEvent = func(name string, payload any) {
		*recorded = append(*recorded, [2]any{name, payload})
	}
	return recorded
}

func eventNames(recorded *[][2]any) []string {
	names := make([]string, 0, len(*recorded))
	for _, e := range *recorded {
		names = append(names, e[0].(string))
	}
	return names
}

// spellTestNPCDef is a hand-built NPCDefinition for the
// Crumble Undead target check: the DEFAULT shape plus the
// undead / attackable flags (NpcDefinition.ts:283, isUndead;
// npc.go IsAttackable).
type spellTestNPCDef struct {
	fakeNPCDefinition
	undead     bool
	attackable bool
}

func (d *spellTestNPCDef) IsUndead() bool     { return d.undead }
func (d *spellTestNPCDef) IsAttackable() bool { return d.attackable }

func spellTestNPC(t *testing.T, undead, attackable bool) *NPC {
	t.Helper()
	n := NewNPC(1, Location{X: 3200, Y: 3200, Z: 0})
	n.definition = &spellTestNPCDef{
		fakeNPCDefinition: *defaultFakeDef(),
		undead:            undead,
		attackable:        attackable,
	}
	// Bind the NPC combatant identity so combatantFor resolves
	// the NPC (combat.go: GetCombat binds npcCombatant).
	n.GetCombat()
	return n
}

func TestStandardSpellTableCounts(t *testing.T) {
	if len(standardSpellDefs) != 36 {
		t.Fatalf("standard spell defs = %d, want 36", len(standardSpellDefs))
	}
	normal, effect := 0, 0
	ids := map[int]bool{}
	for _, def := range standardSpellDefs {
		if def.id <= 0 || ids[def.id] {
			t.Fatalf("bad or duplicate spell id %d (%s)", def.id, def.name)
		}
		ids[def.id] = true
		if def.level < 1 || def.maxHit < 0 || def.baseXP <= 0 || len(def.runes) == 0 || def.castAnim <= 0 {
			t.Fatalf("%s: incomplete def %+v", def.name, def)
		}
		switch def.kind {
		case "normal":
			normal++
		case "effect":
			effect++
		default:
			t.Fatalf("%s: unknown kind %q", def.name, def.kind)
		}
	}
	if normal != 26 || effect != 10 {
		t.Fatalf("normal/effect = %d/%d, want 26/10", normal, effect)
	}
	if len(effectSpellEntries) != 9 {
		t.Fatalf("effect spell entries = %d, want 9", len(effectSpellEntries))
	}
	eids := map[int]bool{}
	for _, e := range effectSpellEntries {
		if e.id <= 0 || eids[e.id] {
			t.Fatalf("bad or duplicate effect id %d (%s)", e.id, e.name)
		}
		eids[e.id] = true
		for _, alias := range e.aliases {
			if eids[alias] {
				t.Fatalf("alias id %d collides", alias)
			}
			eids[alias] = true
		}
	}
	if len(effectSpellFruitBones) != 9 {
		t.Fatalf("fruit bones = %d, want 9", len(effectSpellFruitBones))
	}
}

func TestStandardSpellSpotRows(t *testing.T) {
	installStandardSpells(t)
	// (id, level, maxHit, baseXP int, splashXP int, castAnim,
	// startGraphic, projectile, endGraphic)
	rows := []struct {
		name                   string
		id, level, maxHit      int
		baseXP, splashXP       int
		anim, start, proj, end int
	}{
		{"WIND_STRIKE", 1152, 1, 2, 6, 6, 711, 90, 91, 92},
		{"WATER_STRIKE", 1154, 5, 4, 8, 8, 711, 93, 94, 95},
		{"BIND", 1572, 20, 0, 31, 30, 710, 177, 178, 181},
		{"CRUMBLE_UNDEAD", 1171, 39, 15, 25, 25, 724, 145, 146, 147},
		{"IBAN_BLAST", 1539, 50, 25, 30, 30, 708, 87, 88, 89},
		{"SNARE", 1582, 50, 2, 61, 60, 710, 177, 178, 180},
		{"MAGIC_DART", 12037, 50, 10, 30, 30, 1576, -1, 328, 329},
		{"SARADOMIN_STRIKE", 1190, 60, 20, 35, 35, 811, -1, -1, 76},
		{"CLAWS_OF_GUTHIX", 1191, 60, 20, 35, 35, 811, -1, -1, 77},
		{"FLAMES_OF_ZAMORAK", 1192, 60, 20, 35, 35, 811, -1, -1, 78},
		{"ENTANGLE", 1592, 79, 5, 89, 70, 710, 177, 178, 179},
		{"STUN", 1562, 80, 0, 90, 90, 729, 173, 174, 80},
		{"TELEBLOCK", 12445, 85, 0, 95, 84, 1819, -1, -1, 345},
		{"WIND_SURGE", 21876, 81, 21, 45, 45, 7855, 1455, 1456, 1457},
		{"EARTH_SURGE", 21878, 90, 23, 48, 48, 7855, 1461, 1462, 1463},
		{"FIRE_SURGE", 21879, 95, 24, 51, 51, 7855, 1464, 1465, 1466},
	}
	for _, row := range rows {
		spell := GetCombatSpell(row.id)
		if spell == nil {
			t.Fatalf("%s (%d): not registered", row.name, row.id)
		}
		if spell.SpellID() != row.id || spell.MaximumHit() != row.maxHit ||
			spell.BaseExperience() != row.baseXP || spell.SplashExperience() != row.splashXP {
			t.Fatalf("%s: got id=%d maxHit=%d baseXP=%d splashXP=%d, want id=%d maxHit=%d baseXP=%d splashXP=%d",
				row.name, spell.SpellID(), spell.MaximumHit(), spell.BaseExperience(),
				spell.SplashExperience(), row.id, row.maxHit, row.baseXP, row.splashXP)
		}
		// The presentation ids live on the table def.
		var def *standardSpellDef
		for i := range standardSpellDefs {
			if standardSpellDefs[i].id == row.id {
				def = &standardSpellDefs[i]
				break
			}
		}
		if def == nil {
			t.Fatalf("%s: no table def", row.name)
		}
		if def.level != row.level || def.castAnim != row.anim ||
			def.startGraphic != row.start || def.projectile != row.proj || def.endGraphic != row.end {
			t.Fatalf("%s: table def = level %d anim %d start %d proj %d end %d, want level %d anim %d start %d proj %d end %d",
				row.name, def.level, def.castAnim, def.startGraphic, def.projectile, def.endGraphic,
				row.level, row.anim, row.start, row.proj, row.end)
		}
		if got := spell.AttackSpeed(); got != 5 {
			t.Fatalf("%s: attack speed %d, want 5", row.name, got)
		}
		if got := spell.HitDelay(); got != -1 {
			t.Fatalf("%s: hit delay %d, want -1", row.name, got)
		}
		if got := spell.SpellRadius(); got != 0 {
			t.Fatalf("%s: radius %d, want 0", row.name, got)
		}
	}
	// Autocast slots resolve through the same registry.
	if s := AutocastSpell(17); s == nil || s.SpellID() != 1171 {
		t.Fatalf("autocast slot 17 = %v, want Crumble Undead (1171)", s)
	}
	if s := AutocastSpell(18); s == nil || s.SpellID() != 12037 {
		t.Fatalf("autocast slot 18 = %v, want Magic Dart (12037)", s)
	}
	if s := AutocastSpell(47); s == nil || s.SpellID() != 1539 {
		t.Fatalf("autocast slot 47 = %v, want Iban Blast (1539)", s)
	}
	// Ancient slots resolve once the Ancient book registers
	// (Phase 45); the Arceuus slots stay nil.
	installAncientSpells(t)
	if s := AutocastSpell(31); s == nil || s.SpellID() != 12939 {
		t.Fatalf("autocast slot 31 = %v, want Smoke Rush (12939)", s)
	}
	if s := AutocastSpell(46); s == nil || s.SpellID() != 12891 {
		t.Fatalf("autocast slot 46 = %v, want Ice Barrage (12891)", s)
	}
}

func TestGetCombatSpellByName(t *testing.T) {
	installStandardSpells(t)
	installAncientSpells(t)
	cases := []struct {
		name string
		id   int
	}{
		{"Wind Strike", 1152},
		{"wind strike", 1152},
		{"WIND_STRIKE", 1152},
		{"Bind", 1572},
		{"BIND", 1572},
		{"Tele Block", 12445},
		{"Magic Dart", 12037},
		{"Fire Surge", 21879},
	}
	for _, c := range cases {
		spell := GetCombatSpellByName(c.name)
		if spell == nil || spell.SpellID() != c.id {
			t.Fatalf("GetCombatSpellByName(%q) = %v, want id %d", c.name, spell, c.id)
		}
	}
	// Ancient names resolve now that the book is ported
	// (Phase 45): display and static forms alike.
	if spell := GetCombatSpellByName("Smoke Rush"); spell == nil || spell.SpellID() != 12939 {
		t.Fatalf("GetCombatSpellByName(Smoke Rush) = %v, want id 12939", spell)
	}
	if spell := GetCombatSpellByName("SMOKE_RUSH"); spell == nil || spell.SpellID() != 12939 {
		t.Fatalf("GetCombatSpellByName(SMOKE_RUSH) = %v, want id 12939", spell)
	}
	if spell := GetCombatSpellByName("Ice Barrage"); spell == nil || spell.SpellID() != 12891 {
		t.Fatalf("GetCombatSpellByName(Ice Barrage) = %v, want id 12891", spell)
	}
	if spell := GetCombatSpellByName("no such spell"); spell != nil {
		t.Fatalf("GetCombatSpellByName(unknown) = %v, want nil", spell.SpellID())
	}
	if spell := GetCombatSpell(0); spell != nil {
		t.Fatalf("GetCombatSpell(0) = %v, want nil", spell.SpellID())
	}
	if spell := GetCombatSpell(999999); spell != nil {
		t.Fatalf("GetCombatSpell(999999) = %v, want nil", spell.SpellID())
	}
}

func TestCanCastLevelRefusal(t *testing.T) {
	installStandardSpells(t)
	p := spellTestPlayer(t, 1)
	spell := GetCombatSpell(1169) // Fire Bolt, level 35.
	if spell.CanCast(p, false) {
		t.Fatalf("level-1 player may cast Fire Bolt")
	}
	if spell.CanCast(p, true) {
		t.Fatalf("level-1 player may cast Fire Bolt (consume)")
	}
	// The refusal costs nothing and spends nothing.
	giveRunes(t, p, 556, 3, 562, 1, 554, 4)
	if spell.CanCast(p, false) {
		t.Fatalf("level-1 player may cast Fire Bolt with runes")
	}
	if p.Inventory().GetAmount(556) != 3 {
		t.Fatalf("refused cast spent runes")
	}
}

func TestCanCastRuneConsumptionAndSharedDelay(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1152) // Wind Strike: 556x1, 558x1.
	giveRunes(t, p, 556, 1, 558, 1)
	if !spell.CanCast(p, false) {
		t.Fatalf("check-only cast refused")
	}
	if p.Inventory().GetAmount(556) != 1 || p.Inventory().GetAmount(558) != 1 {
		t.Fatalf("check-only cast spent runes")
	}
	if !spell.CanCast(p, true) {
		t.Fatalf("paid cast refused")
	}
	if p.Inventory().GetAmount(556) != 0 || p.Inventory().GetAmount(558) != 0 {
		t.Fatalf("paid cast did not spend both runes")
	}
	if got := attributeTick(p, spellNextCastAtAttribute); got != combatCycle()+1 {
		t.Fatalf("magic:next-cast-at = %d, want combatCycle()+1 = %d", got, combatCycle()+1)
	}
	// The shared 1-tick delay refuses an immediate second cast.
	giveRunes(t, p, 556, 1, 558, 1)
	if spell.CanCast(p, true) {
		t.Fatalf("second cast inside the shared delay allowed")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("delay-refused cast spent runes")
	}
}

func TestCanCastMissingRunes(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1152)
	if spell.CanCast(p, false) {
		t.Fatalf("cast without runes allowed (check)")
	}
	if spell.CanCast(p, true) {
		t.Fatalf("cast without runes allowed (consume)")
	}
	// A partial stock still refuses.
	giveRunes(t, p, 556, 1)
	if spell.CanCast(p, true) {
		t.Fatalf("cast with only air runes allowed")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("refused cast spent the air rune")
	}
}

func TestCanCastRuneBypass(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	spell := GetCombatSpell(1152)
	// Whole-cost bypass: no runes held, none spent.
	p := spellTestPlayer(t, 99)
	Hooks.SpellRuneBypass = func(player *Player, book Spellbook, spellID, runeID int) bool {
		return runeID == 0
	}
	if !spell.CanCast(p, true) {
		t.Fatalf("whole-cost bypass cast refused")
	}
	if p.Inventory().GetAmount(556) != 0 || p.Inventory().GetAmount(558) != 0 {
		t.Fatalf("bypassed cast spent runes")
	}
	// Per-rune bypass (air only): the air rune is covered, the
	// mind rune is paid. A fresh player: the first cast spent
	// the shared 1-tick delay on its caster.
	p = spellTestPlayer(t, 99)
	Hooks.SpellRuneBypass = func(player *Player, book Spellbook, spellID, runeID int) bool {
		return runeID == 556
	}
	giveRunes(t, p, 556, 1, 558, 1)
	if !spell.CanCast(p, true) {
		t.Fatalf("per-rune bypass cast refused")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("covered air rune was spent")
	}
	if p.Inventory().GetAmount(558) != 0 {
		t.Fatalf("paid mind rune was not spent")
	}
	// Per-rune bypass with the paid rune missing: the cast
	// refuses (the bypass covers only the air rune).
	p = spellTestPlayer(t, 99)
	giveRunes(t, p, 556, 1)
	if spell.CanCast(p, true) {
		t.Fatalf("bypass cast allowed with the paid mind rune missing")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("refused bypass cast spent the covered air rune")
	}
}

func TestCanCastSpellCostEvent(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1152)
	recorded := captureEvents(p)
	Hooks.SpellRuneBypass = func(player *Player, book Spellbook, spellID, runeID int) bool {
		return runeID == 556
	}
	giveRunes(t, p, 556, 1, 558, 1)
	if !spell.CanCast(p, true) {
		t.Fatalf("cast refused")
	}
	var cost *MagicSpellCostPayload
	var committed *MagicCastCommittedPayload
	for _, e := range *recorded {
		switch payload := e[1].(type) {
		case *MagicSpellCostPayload:
			cost = payload
		case *MagicCastCommittedPayload:
			committed = payload
		}
	}
	if cost == nil {
		t.Fatalf("no magic:spell-cost event")
	}
	if len(cost.Items) != 1 || cost.Items[0].ID() != 558 {
		t.Fatalf("spell-cost items = %v, want [mind rune 558]", cost.Items)
	}
	if len(cost.Covered) != 1 || cost.Covered[0].ID() != 556 {
		t.Fatalf("spell-cost covered = %v, want [air rune 556]", cost.Covered)
	}
	if committed == nil {
		t.Fatalf("no magic:cast-committed event")
	}
	if committed.RunesSaved {
		t.Fatalf("cast-committed reports runes saved with no rune-saving event")
	}
	if committed.Context == nil {
		t.Fatalf("cast-committed has no context")
	}
}

func TestCanCastRuneSaving(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1152)
	giveRunes(t, p, 556, 1, 558, 1)
	p.EmitCustomEvent = func(name string, payload any) {
		if saving, ok := payload.(*MagicRuneSavingPayload); ok {
			saving.Saved = true
			saving.Message = "Your staff saves your runes."
		}
	}
	if !spell.CanCast(p, true) {
		t.Fatalf("rune-saving cast refused")
	}
	if p.Inventory().GetAmount(556) != 1 || p.Inventory().GetAmount(558) != 1 {
		t.Fatalf("saved runes were spent")
	}
}

func TestCanCastDisabledGate(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1152)
	giveRunes(t, p, 556, 1, 558, 1)
	Hooks.SpellDisabled = func(player *Player, book Spellbook, spellID int, s CombatSpell) bool {
		return spellID == 1152
	}
	p.GetCombat().SetCastSpell(spell)
	if spell.CanCast(p, false) {
		t.Fatalf("disabled spell castable")
	}
	if p.GetCombat().CastSpell() != nil {
		t.Fatalf("disabled gate did not clear the cast spell")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("disabled cast spent runes")
	}
}

func TestCanCastSpellbookMismatch(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	p.SetSpellbook(SpellbookLunar)
	spell := GetCombatSpell(1152)
	giveRunes(t, p, 556, 1, 558, 1)
	p.GetCombat().SetAutoCastSpell(spell)
	if spell.CanCast(p, false) {
		t.Fatalf("Normal spell castable on the Lunar book")
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatalf("spellbook mismatch did not clear autocast")
	}
	if p.Inventory().GetAmount(556) != 1 {
		t.Fatalf("mismatched cast spent runes")
	}
}

func TestCanCastEquipment(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	iban := GetCombatSpell(1539)
	giveRunes(t, p, 560, 1, 554, 5)
	if iban.CanCast(p, false) {
		t.Fatalf("Iban Blast castable without Iban's staff")
	}
	p.Equipment().Set(WeaponSlot, NewItem(1409, 1))
	if !iban.CanCast(p, false) {
		t.Fatalf("Iban Blast refused with Iban's staff wielded")
	}
	// An upgraded staff also casts it.
	p.Equipment().Set(WeaponSlot, NewItem(12658, 1))
	if !iban.CanCast(p, false) {
		t.Fatalf("Iban Blast refused with Iban's staff (u) wielded")
	}
	// God spells name their staves.
	sara := GetCombatSpell(1190)
	giveRunes(t, p, 556, 4, 565, 2, 554, 2)
	if sara.CanCast(p, false) {
		t.Fatalf("Saradomin Strike castable without its staff")
	}
	p.Equipment().Set(WeaponSlot, NewItem(22296, 1)) // Staff of Light.
	if !sara.CanCast(p, false) {
		t.Fatalf("Saradomin Strike refused with the Staff of Light")
	}
	claws := GetCombatSpell(1191)
	giveRunes(t, p, 556, 4, 565, 2, 554, 1)
	p.Equipment().Set(WeaponSlot, NewItem(2416, 1)) // Guthix staff.
	if !claws.CanCast(p, false) {
		t.Fatalf("Claws of Guthix refused with the Guthix staff")
	}
	flames := GetCombatSpell(1192)
	giveRunes(t, p, 556, 1, 565, 2, 554, 4)
	p.Equipment().Set(WeaponSlot, NewItem(27788, 1)) // Thammaron's sceptre (a).
	if !flames.CanCast(p, false) {
		t.Fatalf("Flames of Zamorak refused with Thammaron's sceptre")
	}
}

func TestGodSpellDrains(t *testing.T) {
	installStandardSpells(t)
	attacker := spellTestPlayer(t, 99)
	victim := spellTestPlayer(t, 99)
	setPlayerLevels(victim, map[int]int{
		SkillPrayer:  60,
		SkillDefence: 60,
		SkillMagic:   60,
	})
	// Saradomin Strike drains 1 Prayer from a player target.
	sara := GetCombatSpell(1190)
	sara.FinishCast(&attacker.Mobile, &victim.Mobile, true, 10)
	if got := victim.Skills().CurrentLevel(SkillPrayer); got != 59 {
		t.Fatalf("Saradomin drain: prayer %d, want 59", got)
	}
	// A splash or a 0-damage hit drains nothing.
	sara.FinishCast(&attacker.Mobile, &victim.Mobile, false, 10)
	sara.FinishCast(&attacker.Mobile, &victim.Mobile, true, 0)
	if got := victim.Skills().CurrentLevel(SkillPrayer); got != 59 {
		t.Fatalf("Saradomin drain after splash/0-hit: prayer %d, want 59", got)
	}
	// Claws of Guthix drains 1 + 5% of Defence (60 -> 56).
	claws := GetCombatSpell(1191)
	claws.FinishCast(&attacker.Mobile, &victim.Mobile, true, 10)
	if got := victim.Skills().CurrentLevel(SkillDefence); got != 56 {
		t.Fatalf("Claws drain: defence %d, want 56", got)
	}
	// Already lowered: no second drain until the stat recovers.
	claws.FinishCast(&attacker.Mobile, &victim.Mobile, true, 10)
	if got := victim.Skills().CurrentLevel(SkillDefence); got != 56 {
		t.Fatalf("Claws drain reapplied below max: defence %d, want 56", got)
	}
	// Flames of Zamorak drains 1 + 5% of Magic (60 -> 56).
	flames := GetCombatSpell(1192)
	flames.FinishCast(&attacker.Mobile, &victim.Mobile, true, 10)
	if got := victim.Skills().CurrentLevel(SkillMagic); got != 56 {
		t.Fatalf("Flames drain: magic %d, want 56", got)
	}
	// An NPC target takes no drain and does not panic (the NPC
	// branch has no Go seam; see godSpellDrain).
	npc := spellTestNPC(t, false, true)
	sara.FinishCast(&attacker.Mobile, &npc.Mobile, true, 10)
	claws.FinishCast(&attacker.Mobile, &npc.Mobile, true, 10)
}

func TestCrumbleUndeadTarget(t *testing.T) {
	installStandardSpells(t)
	p := spellTestPlayer(t, 99)
	spell := GetCombatSpell(1171)
	undead := spellTestNPC(t, true, true)
	if !spell.CanCastOnTarget(p, &undead.Mobile) {
		t.Fatalf("Crumble Undead refused on an undead NPC")
	}
	living := spellTestNPC(t, false, true)
	if spell.CanCastOnTarget(p, &living.Mobile) {
		t.Fatalf("Crumble Undead allowed on a living NPC")
	}
	// An unattackable target is left to its quest's handler
	// (TS: !definition.isAttackable?.() passes).
	scripted := spellTestNPC(t, false, false)
	if !spell.CanCastOnTarget(p, &scripted.Mobile) {
		t.Fatalf("Crumble Undead refused on an unattackable NPC")
	}
	// A player target is refused.
	other := spellTestPlayer(t, 99)
	if spell.CanCastOnTarget(p, &other.Mobile) {
		t.Fatalf("Crumble Undead allowed on a player")
	}
	// Ordinary spells accept anything.
	wind := GetCombatSpell(1152)
	if !wind.CanCastOnTarget(p, &living.Mobile) {
		t.Fatalf("Wind Strike refused on a living NPC")
	}
}

// spellOnHitCalc calls a spell's OnHitCalc through the same
// structural assertion the magic method uses
// (combat_magic.go:85): OnHitCalc is not on the CombatSpell
// interface (contract C1 adds SpellID only).
func spellOnHitCalc(t *testing.T, spell CombatSpell, hit *PendingHit) {
	t.Helper()
	calc, ok := spell.(interface{ OnHitCalc(hit *PendingHit) })
	if !ok {
		t.Fatalf("%T has no OnHitCalc", spell)
	}
	calc.OnHitCalc(hit)
}

func TestNormalSpellHitEffects(t *testing.T) {
	installStandardSpells(t)
	attacker := spellTestPlayer(t, 99)
	target := spellTestNPC(t, false, true)
	spell := GetCombatSpell(1152)
	recorded := captureEvents(attacker)
	hit := NewPendingHitConfig(&attacker.Mobile, &target.Mobile, MagicMethod, PendingHitConfig{})
	hit.accurate = true
	hit.totalDamage = 2
	spellOnHitCalc(t, spell, hit)
	names := eventNames(recorded)
	if len(names) != 2 || names[0] != "magic:hit-effects" || names[1] != "magic:hit-effects-applied" {
		t.Fatalf("hit-effects events = %v, want [magic:hit-effects magic:hit-effects-applied]", names)
	}
	effect := (*recorded)[0][1].(*MagicHitEffectsPayload)
	if !effect.Allowed || effect.EffectMultiplier != 1 || effect.Hit != hit {
		t.Fatalf("hit-effects payload = %+v", effect)
	}
	// A vetoed effect skips the applied event.
	*recorded = (*recorded)[:0]
	attacker.EmitCustomEvent = func(name string, payload any) {
		if e, ok := payload.(*MagicHitEffectsPayload); ok && name == "magic:hit-effects" {
			e.Allowed = false
		}
		*recorded = append(*recorded, [2]any{name, payload})
	}
	spellOnHitCalc(t, spell, hit)
	if names := eventNames(recorded); len(names) != 1 || names[0] != "magic:hit-effects" {
		t.Fatalf("vetoed hit-effects events = %v, want only magic:hit-effects", names)
	}
	// A splash emits nothing.
	*recorded = (*recorded)[:0]
	attacker.EmitCustomEvent = func(name string, payload any) {
		*recorded = append(*recorded, [2]any{name, payload})
	}
	hit.accurate = false
	spellOnHitCalc(t, spell, hit)
	if len(*recorded) != 0 {
		t.Fatalf("splash emitted %d events, want 0", len(*recorded))
	}
}

func TestEffectSpellUtilityEvents(t *testing.T) {
	installStandardSpells(t)
	attacker := spellTestPlayer(t, 99)
	target := spellTestNPC(t, false, true)
	bind := GetCombatSpell(1572)
	recorded := captureEvents(attacker)
	bind.FinishCast(&attacker.Mobile, &target.Mobile, true, 0)
	names := eventNames(recorded)
	if len(names) != 2 || names[0] != "magic:utility-effect-check" || names[1] != "magic:utility-effect" {
		t.Fatalf("bind events = %v, want [magic:utility-effect-check magic:utility-effect]", names)
	}
	// A splash runs no effect.
	*recorded = (*recorded)[:0]
	bind.FinishCast(&attacker.Mobile, &target.Mobile, false, 0)
	if len(*recorded) != 0 {
		t.Fatalf("splashed bind emitted %d events, want 0", len(*recorded))
	}
	// A check veto stops the utility event.
	*recorded = (*recorded)[:0]
	attacker.EmitCustomEvent = func(name string, payload any) {
		if e, ok := payload.(*MagicUtilityEffectPayload); ok && name == "magic:utility-effect-check" {
			e.Allowed = false
		}
		*recorded = append(*recorded, [2]any{name, payload})
	}
	bind.FinishCast(&attacker.Mobile, &target.Mobile, true, 0)
	if names := eventNames(recorded); len(names) != 1 || names[0] != "magic:utility-effect-check" {
		t.Fatalf("vetoed utility events = %v, want only magic:utility-effect-check", names)
	}
}

func TestTeleBlockSpell(t *testing.T) {
	installStandardSpells(t)
	caster := spellTestPlayer(t, 99)
	caster.Username = "caster"
	victim := spellTestPlayer(t, 99)
	spell := GetCombatSpell(12445)
	if spell.MaximumHit() != 0 || spell.BaseExperience() != 95 || spell.SplashExperience() != 84 {
		t.Fatalf("Tele Block: maxHit %d baseXP %d splashXP %d, want 0/95/84",
			spell.MaximumHit(), spell.BaseExperience(), spell.SplashExperience())
	}
	// An accurate cast blocks the victim for 5 minutes.
	spell.FinishCast(&caster.Mobile, &victim.Mobile, true, 0)
	if !TeleBlockIsBlocked(victim) {
		t.Fatalf("victim not blocked after a Tele Block hit")
	}
	if got := TeleBlockRemainingSeconds(victim); got != 300 {
		t.Fatalf("Tele Block remaining = %d seconds, want 300", got)
	}
	if got := TeleBlockDescribe(300); got != "5 minutes" {
		t.Fatalf("describe(300) = %q, want \"5 minutes\"", got)
	}
	if got := TeleBlockDescribe(150); got != "2 minutes, 30 seconds" {
		t.Fatalf("describe(150) = %q, want \"2 minutes, 30 seconds\"", got)
	}
	if got := TeleBlockDescribe(87); got != "1 minute, 27 seconds" {
		t.Fatalf("describe(87) = %q, want \"1 minute, 27 seconds\"", got)
	}
	// A second cast on a blocked victim refuses (no extension).
	until := attributeTick(victim, teleBlockUntilAttribute)
	spell.FinishCast(&caster.Mobile, &victim.Mobile, true, 0)
	if got := attributeTick(victim, teleBlockUntilAttribute); got != until {
		t.Fatalf("re-cast extended the block: %d -> %d", until, got)
	}
	// A handled utility event skips the fallback entirely.
	TeleBlockRemove(victim)
	caster.EmitCustomEvent = func(name string, payload any) {
		if e, ok := payload.(*MagicUtilityEffectPayload); ok && name == "magic:utility-effect" {
			e.Handled = true
		}
	}
	spell.FinishCast(&caster.Mobile, &victim.Mobile, true, 0)
	if TeleBlockIsBlocked(victim) {
		t.Fatalf("handled Tele Block still applied its fallback")
	}
	// An NPC target is refused with no block and no panic.
	caster.EmitCustomEvent = nil
	npc := spellTestNPC(t, false, true)
	spell.FinishCast(&caster.Mobile, &npc.Mobile, true, 0)
	if TeleBlockIsBlocked(caster) {
		t.Fatalf("caster blocked by its own NPC-targeted Tele Block")
	}
	// Protect from Magic halves the block.
	TeleBlockRemove(victim)
	if !ActivatePrayer(victim, PrayerProtectFromMagic) {
		t.Fatalf("could not activate Protect from Magic")
	}
	spell.FinishCast(&caster.Mobile, &victim.Mobile, true, 0)
	if got := TeleBlockRemainingSeconds(victim); got != 150 {
		t.Fatalf("protected Tele Block remaining = %d seconds, want 150", got)
	}
	if !strings.Contains(TeleBlockAttemptMessage(victim), "2 minutes, 30 seconds") {
		t.Fatalf("attempt message = %q", TeleBlockAttemptMessage(victim))
	}
	TeleBlockRemove(victim)
	if TeleBlockIsBlocked(victim) {
		t.Fatalf("victim still blocked after remove")
	}
}

func TestEffectSpellTable(t *testing.T) {
	installStandardSpells(t)
	// Spot rows: id, level, baseXP verbatim (including the dead
	// values TS never pays), spellbook, runes.
	rows := []struct {
		name      string
		id, level int
		baseXP    float64
		book      Spellbook
		members   bool
	}{
		{"BONES_TO_BANANAS", 1159, 15, 25, SpellbookNormal, false},
		{"LOW_ALCHEMY", 1162, 21, 4000, SpellbookNormal, false},
		{"TELEKINETIC_GRAB", 1168, 33, 43, SpellbookNormal, false},
		{"SUPERHEAT_ITEM", 1173, 43, 6544, SpellbookNormal, false},
		{"HIGH_ALCHEMY", 1178, 55, 20000, SpellbookNormal, false},
		{"BONES_TO_PEACHES", 15877, 60, 35.5, SpellbookNormal, true},
		{"BAKE_PIE", 9107, 65, 5121, SpellbookLunar, false},
		{"VENGEANCE_OTHER", 9135, 93, 10000, SpellbookLunar, false},
		{"VENGEANCE", 9136, 94, 112, SpellbookLunar, false},
	}
	for _, row := range rows {
		u := GetUtilitySpell(row.id)
		if u == nil {
			t.Fatalf("%s (%d): not registered", row.name, row.id)
		}
		us, ok := u.(*utilitySpell)
		if !ok {
			t.Fatalf("%s: registered as %T, want *utilitySpell", row.name, u)
		}
		e := us.Entry()
		if e.level != row.level || e.baseXP != row.baseXP || e.spellbook != row.book || e.members != row.members {
			t.Fatalf("%s: entry = %+v, want level %d baseXP %v book %v members %v",
				row.name, e, row.level, row.baseXP, row.book, row.members)
		}
		if us.SpellID() != row.id {
			t.Fatalf("%s: SpellID %d, want %d", row.name, us.SpellID(), row.id)
		}
	}
	// The map aliases resolve to the same spell.
	if u := GetUtilitySpell(3280); u == nil || u.(*utilitySpell).Entry().id != 1159 {
		t.Fatalf("alias 3280 did not resolve to Bones to Bananas")
	}
	if u := GetUtilitySpell(6884); u == nil || u.(*utilitySpell).Entry().id != 15877 {
		t.Fatalf("alias 6884 did not resolve to Bones to Peaches")
	}
	if u := GetUtilitySpell(1162); u == nil {
		t.Fatalf("Low Alchemy not registered")
	}
	// Fruit bones.
	if !IsFruitBone(526) || !IsFruitBone(30973) {
		t.Fatalf("known fruit bones not recognised")
	}
	if IsFruitBone(995) || IsFruitBone(526+1) {
		t.Fatalf("non-bone recognised as a fruit bone")
	}
}

func TestUtilitySpellCanCast(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	u := GetUtilitySpell(1159) // Bones to Bananas: 561x1, 555x2, 557x2.
	giveRunes(t, p, 561, 1, 555, 2, 557, 2)
	if !u.CanCast(p, false) {
		t.Fatalf("Bones to Bananas check refused")
	}
	if !u.CanCast(p, true) {
		t.Fatalf("Bones to Bananas cast refused")
	}
	if p.Inventory().GetAmount(561) != 0 || p.Inventory().GetAmount(555) != 0 {
		t.Fatalf("Bones to Bananas did not spend its runes")
	}
	// Utility spells do not use the shared cast delay.
	if got := attributeTick(p, spellNextCastAtAttribute); got != 0 {
		t.Fatalf("utility cast set magic:next-cast-at = %d, want 0", got)
	}
	// ... but they do pay the base XP through BaseExperience.
	if got := u.BaseExperience(); got != 25 {
		t.Fatalf("Bones to Bananas base XP = %d, want 25", got)
	}
	// A Lunar utility spell refuses on the Normal book, then
	// casts on the Lunar book and spends its runes.
	vengeance := GetUtilitySpell(9136) // 9075x4, 557x10, 560x2.
	if vengeance.CanCast(p, false) {
		t.Fatalf("Vengeance castable on the Normal book")
	}
	p.SetSpellbook(SpellbookLunar)
	giveRunes(t, p, 9075, 4, 557, 10, 560, 2)
	if !vengeance.CanCast(p, true) {
		t.Fatalf("Vengeance refused on the Lunar book")
	}
	if p.Inventory().GetAmount(9075) != 0 || p.Inventory().GetAmount(557) != 0 || p.Inventory().GetAmount(560) != 0 {
		t.Fatalf("Vengeance did not spend its runes")
	}
}

// chargeXP is the XP a Charge cast pays under the current
// world rate, through the same tenths step SkillManager
// applies (skills.go:517-523): 180 * rate, tenths truncated.
func chargeXP() int {
	return int(math.Floor(180*ExperienceRate()*10+1e-6) / 10)
}

func TestChargeSpell(t *testing.T) {
	installStandardSpells(t)
	resetCombatHooks(t)
	// Any name but "charge" is not this spell.
	p := spellTestPlayer(t, 99)
	if HandleSelfSpell(p, "fireball") {
		t.Fatalf("HandleSelfSpell claimed a non-charge name")
	}
	if HandleSelfSpell(p, "") {
		t.Fatalf("HandleSelfSpell claimed an empty name")
	}
	// Without a Mage Arena answer (and without a league
	// unlock), the cast refuses before the runes.
	giveRunes(t, p, 556, 3, 554, 3, 565, 3)
	if !HandleSelfSpell(p, "charge") {
		t.Fatalf("HandleSelfSpell did not claim 'charge'")
	}
	if p.Inventory().GetAmount(556) != 3 {
		t.Fatalf("refused Charge spent runes")
	}
	if !HandleSelfSpell(p, " Charge ") {
		t.Fatalf("HandleSelfSpell did not claim ' Charge ' (trim/case)")
	}
	// A league unlock lets it through: runes are spent, 180
	// XP is paid, the effect runs 700 ticks.
	p.EmitCustomEvent = func(name string, payload any) {
		if unlock, ok := payload.(*MagicSpellUnlockedPayload); ok {
			unlock.Unlocked = true
		}
	}
	before := p.Skills().Experience(SkillMagic)
	if !HandleSelfSpell(p, "charge") {
		t.Fatalf("unlocked Charge refused")
	}
	if p.Inventory().GetAmount(556) != 0 || p.Inventory().GetAmount(554) != 0 || p.Inventory().GetAmount(565) != 0 {
		t.Fatalf("Charge did not spend its runes")
	}
	if got := p.Skills().Experience(SkillMagic) - before; got != chargeXP() {
		t.Fatalf("Charge XP = %d, want %d", got, chargeXP())
	}
	if got := attributeTick(p, chargeUntilAttribute); got != combatCycle()+ChargeTicks {
		t.Fatalf("charge:until = %d, want combatCycle()+700 = %d", got, combatCycle()+ChargeTicks)
	}
	// The 60-second cooldown refuses a second paid cast.
	giveRunes(t, p, 556, 3, 554, 3, 565, 3)
	if chargeSpellInstance.CanCast(p, true) {
		t.Fatalf("Charge castable inside its 100-tick cooldown")
	}
	if p.Inventory().GetAmount(556) != 3 {
		t.Fatalf("cooldown-refused Charge spent runes")
	}
	// A Mage Arena cape answer skips the league-unlock question.
	p2 := spellTestPlayer(t, 99)
	setPlayerLevels(p2, map[int]int{SkillMagic: 99})
	giveRunes(t, p2, 556, 3, 554, 3, 565, 3)
	unlockAsked := false
	p2.EmitCustomEvent = func(name string, payload any) {
		if arena, ok := payload.(*MageArenaCapeReceivedPayload); ok {
			received := true
			arena.Received = &received
		}
		if _, ok := payload.(*MagicSpellUnlockedPayload); ok {
			unlockAsked = true
		}
	}
	if !HandleSelfSpell(p2, "charge") {
		t.Fatalf("cape-holder Charge refused")
	}
	if unlockAsked {
		t.Fatalf("cape-holder Charge asked the league-unlock question")
	}
	if p2.Inventory().GetAmount(556) != 0 {
		t.Fatalf("cape-holder Charge did not spend its runes")
	}
}

func TestApplyChargeMaxHit(t *testing.T) {
	installStandardSpells(t)
	p := spellTestPlayer(t, 99)
	sara := GetCombatSpell(1190)
	wind := GetCombatSpell(1152)
	// Not charged: unchanged.
	if got := ApplyChargeMaxHit(&p.Mobile, 20, sara); got != 20 {
		t.Fatalf("uncharged ApplyChargeMaxHit = %d, want 20", got)
	}
	// Charged but no cape: unchanged.
	p.SetAttribute(chargeUntilAttribute, combatCycle()+ChargeTicks)
	if got := ApplyChargeMaxHit(&p.Mobile, 20, sara); got != 20 {
		t.Fatalf("capeless ApplyChargeMaxHit = %d, want 20", got)
	}
	// Charged with the Saradomin cape: 30.
	p.Equipment().Set(CapeSlot, NewItem(2412, 1))
	if got := ApplyChargeMaxHit(&p.Mobile, 20, sara); got != 30 {
		t.Fatalf("charged+cape ApplyChargeMaxHit = %d, want 30", got)
	}
	// The cape must match the spell's god.
	if got := ApplyChargeMaxHit(&p.Mobile, 20, GetCombatSpell(1191)); got != 20 {
		t.Fatalf("Saradomin cape raised Claws of Guthix to %d, want 20", got)
	}
	// A non-god spell never rises.
	if got := ApplyChargeMaxHit(&p.Mobile, 2, wind); got != 2 {
		t.Fatalf("charged+cape raised Wind Strike to %d, want 2", got)
	}
	// Expired charge: unchanged.
	p.SetAttribute(chargeUntilAttribute, combatCycle())
	if got := ApplyChargeMaxHit(&p.Mobile, 20, sara); got != 20 {
		t.Fatalf("expired ApplyChargeMaxHit = %d, want 20", got)
	}
	// Nil spell falls back to the selected spell.
	p.SetAttribute(chargeUntilAttribute, combatCycle()+ChargeTicks)
	p.GetCombat().SetCastSpell(sara)
	if got := ApplyChargeMaxHit(&p.Mobile, 20, nil); got != 30 {
		t.Fatalf("nil-spell ApplyChargeMaxHit = %d, want 30", got)
	}
	p.GetCombat().SetCastSpell(nil)
	if got := ApplyChargeMaxHit(&p.Mobile, 20, nil); got != 20 {
		t.Fatalf("nil-spell+no-cast ApplyChargeMaxHit = %d, want 20", got)
	}
}

func TestMissingItemsMessage(t *testing.T) {
	resetCombatHooks(t)
	p := spellTestPlayer(t, 99)
	p.SetItemDefinitions(&styleTestDefs{
		testItemDefs: &testItemDefs{},
		names: map[int]string{
			563: "Law rune",
			554: "Fire rune",
			995: "Coins",
		},
	})
	// First shortfall item, rune name title-cased + "s".
	msg := missingItemsMessage(p, []Item{NewItem(563, 1), NewItem(554, 3)})
	if msg != "You do not have enough Law Runes to cast this spell." {
		t.Fatalf("missing message = %q", msg)
	}
	// A non-rune first item goes lower case + "s" ("Coins"
	// -> "coinss": TS lower-cases the name, then adds "s").
	msg = missingItemsMessage(p, []Item{NewItem(995, 10)})
	if msg != "You do not have enough coinss to cast this spell." {
		t.Fatalf("non-rune missing message = %q", msg)
	}
	// No name for the item: the generic line.
	msg = missingItemsMessage(p, []Item{NewItem(12345, 1)})
	if msg != "You do not have the required items to cast this spell." {
		t.Fatalf("unnamed missing message = %q", msg)
	}
	// A rune source covering the first shortfall moves to the next.
	Hooks.CheckSpellRuneSource = func(player *Player, items []Item) bool {
		return len(items) == 1 && items[0].ID() == 563
	}
	msg = missingItemsMessage(p, []Item{NewItem(563, 1), NewItem(554, 3)})
	if msg != "You do not have enough Fire Runes to cast this spell." {
		t.Fatalf("source-covered missing message = %q", msg)
	}
	if got := titleCaseWords("law rune"); got != "Law Rune" {
		t.Fatalf("titleCaseWords(law rune) = %q", got)
	}
	if got := titleCaseWords("air rune"); got != "Air Rune" {
		t.Fatalf("titleCaseWords(air rune) = %q", got)
	}
}

func TestSpellXPRounding(t *testing.T) {
	// spellXP rounds halves away from zero (math.Round): the
	// interface's int cannot keep tenths (skills.go:517-523).
	cases := []struct {
		in   float64
		want int
	}{
		{5.5, 6}, {7.5, 8}, {48.2, 48}, {50.5, 51}, {35.5, 36}, {25, 25},
	}
	for _, c := range cases {
		if got := spellXP(c.in); got != c.want {
			t.Fatalf("spellXP(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
