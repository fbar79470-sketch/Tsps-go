// Enchant Jewellery tests (fixtures only, no live server).
package enchantjewellery

import (
	"strings"
	"sync"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
	"tsps-go/internal/plugins/skillactions"
)

// ---- fixtures ----

const (
	fxCosmicRune = 564
	fxWaterRune  = 555
	fxAirRune    = 556
	fxFireRune   = 554
	fxEarthRune  = 557
	fxBloodRune  = 565
	fxSoulRune   = 566

	fxSapphireRing     = 1637
	fxRingOfRecoil     = 2550
	fxSapphireAmulet   = 1694
	fxAmuletOfMagic    = 1727
	fxSapphireNecklace = 1656
	fxGamesNecklace    = 3853
	fxOpalAmulet       = 21108
	fxAmuletOfBounty   = 21160
	fxIronOre          = 440

	fxEmeraldAmulet    = 1696
	fxAmuletOfDefence  = 1729
	fxJadeBracelet     = 21120
	fxFlamtaerBracelet = 21180

	fxRubyAmulet       = 1698
	fxAmuletOfStrength = 1725
	fxTopazRing        = 21087
	fxEfaritaysAid     = 21140

	fxDiamondAmulet = 1700
	fxAmuletOfPower = 1731

	fxDragonstoneAmulet = 1702
	fxAmuletOfGlory     = 1704
	fxDragonstoneRing   = 1645
	fxRingOfWealth      = 2572

	fxOnyxAmulet   = 6581
	fxAmuletOfFury = 6585

	fxZenyteAmulet      = 19541
	fxAmuletOfTorture   = 19553
	fxZenyteBracelet    = 19532
	fxTormentedBracelet = 19544
)

// installSpellsOnce installs the standard spell table (the
// production boot wiring's InstallStandardSpells) once for
// the Enchant tests: the utility spells only exist after
// registration, and test binaries never boot. The
// spellwiring_test.go precedent installs it the same way.
var installSpellsOnce sync.Once

func installEnchantSpells() {
	installSpellsOnce.Do(game.InstallStandardSpells)
}

type fakeItemDefs struct {
	names     map[int]string
	stackable map[int]bool
}

func (f fakeItemDefs) IsStackable(id int) bool      { return f.stackable[id] }
func (f fakeItemDefs) IsNoted(id int) bool          { return false }
func (f fakeItemDefs) WeightGrams(id int) int       { return 0 }
func (f fakeItemDefs) EquipmentSlot(id int) int     { return -1 }
func (f fakeItemDefs) IsDoubleHanded(id int) bool   { return false }
func (f fakeItemDefs) Requirements(id int) []int    { return nil }
func (f fakeItemDefs) CombatRequirement(id int) int { return 0 }
func (f fakeItemDefs) IsTradeable(id int) bool      { return false }
func (f fakeItemDefs) IsSellable(id int) bool       { return false }
func (f fakeItemDefs) IsDropable(id int) bool       { return false }
func (f fakeItemDefs) UnNote(id int) int            { return id }
func (f fakeItemDefs) PlaceholderID(id int) int     { return -1 }
func (f fakeItemDefs) ItemName(id int) string       { return f.names[id] }
func (f fakeItemDefs) IsMembersItem(id int) bool    { return false }

type fakeSession struct {
	messages []string
	scripts  [][2]int // script id, first int arg
	sounds   []int
}

func (s *fakeSession) SendPacket(packet []byte) {}
func (s *fakeSession) SendRunEnergy(units int, running bool, weightKg int, staminaTicks int) {
}
func (s *fakeSession) SendDestination(x, y int) {}
func (s *fakeSession) SendVarp(id, value int)   {}
func (s *fakeSession) FlushTick(cycle int64) error {
	return nil
}
func (s *fakeSession) Close(code int, reason string)  {}
func (s *fakeSession) IsOpen() bool                   { return true }
func (s *fakeSession) SendGameMessage(message string) { s.messages = append(s.messages, message) }
func (s *fakeSession) SendTypedGameMessage(chatType int, message, from string) {
	s.messages = append(s.messages, message)
}
func (s *fakeSession) SendSoundEffect(soundID, loopType, delay, volume int) {
	s.sounds = append(s.sounds, soundID)
}
func (s *fakeSession) SendAreaSound(soundID, x, y, level, loops, delay, radius, attenuation int) {
}
func (s *fakeSession) SendClientScript(scriptID int, args ...game.ClientScriptArg) {
	first := 0
	if len(args) > 0 {
		first = args[0].IntValue()
	}
	s.scripts = append(s.scripts, [2]int{scriptID, first})
}

func newTestEnchanter() *enchanter {
	return newEnchanter()
}

func newTestPlayer(name string) (*game.Player, *fakeSession) {
	p := game.NewPlayer(name, game.Location{X: 3200, Y: 3200, Z: 0})
	p.SetRegistered(true)
	p.SetItemDefinitions(fakeItemDefs{
		names: map[int]string{
			fxCosmicRune: "Cosmic rune", fxWaterRune: "Water rune",
			fxAirRune: "Air rune", fxFireRune: "Fire rune",
			fxEarthRune: "Earth rune", fxBloodRune: "Blood rune",
			fxSoulRune:     "Soul rune",
			fxSapphireRing: "Sapphire ring", fxRingOfRecoil: "Ring of recoil",
			fxSapphireAmulet: "Sapphire amulet", fxAmuletOfMagic: "Amulet of magic",
			fxIronOre: "Iron ore",
		},
		stackable: map[int]bool{
			fxCosmicRune: true, fxWaterRune: true, fxAirRune: true,
			fxFireRune: true, fxEarthRune: true, fxBloodRune: true,
			fxSoulRune: true,
		},
	})
	setLevel(p, game.SkillHitpoints, 10)
	session := &fakeSession{}
	p.Session = session
	return p, session
}

func setLevel(p *game.Player, skill, level int) {
	p.Skills().SetCurrentLevel(skill, level, true)
	p.Skills().SetMaxLevel(skill, level)
}

func give(p *game.Player, id, amount int) {
	p.Inventory().Add(game.NewItem(id, amount), true)
}

func slotOf(p *game.Player, id int) int {
	for i, item := range p.Inventory().Items() {
		if item.IsValid() && item.ID() == id {
			return i
		}
	}
	return -1
}

func hasMessage(sess *fakeSession, want string) bool {
	for _, m := range sess.messages {
		if strings.Contains(m, want) {
			return true
		}
	}
	return false
}

// castEnchant casts spellName on the player's item at the
// given cycle, returning the event for Handled checks.
func castEnchant(e *enchanter, p *game.Player, spellName string, itemID int, cycle int64) *plugins.SpellOnItemEvent {
	ev := &plugins.SpellOnItemEvent{
		Player:    p,
		SpellName: spellName,
		ItemID:    itemID,
		Slot:      slotOf(p, itemID),
		Cycle:     cycle,
	}
	e.castEnchantOnItem(ev)
	return ev
}

// driveTicks runs processTick for cycles from..to inclusive,
// the smithing driveTicks shape (tests set the clock, then
// drive).
func driveTicks(e *enchanter, from, to int64) {
	for cycle := from; cycle <= to; cycle++ {
		e.processTick(cycle)
	}
}

// ---- the table (every row) ----

func TestEnchantTableRows(t *testing.T) {
	want := []struct {
		name    string
		spellID int
		level   int
		xp      float64
		members bool
		runes   [][2]int
		results map[int]int
	}{
		{"Lvl-1 Enchant", 14286864, 7, 17.5, false,
			[][2]int{{564, 1}, {555, 1}},
			map[int]int{1694: 1727, 21108: 21160, 1637: 2550, 1656: 3853, 11072: 11074, 21081: 21126, 21090: 21143, 21117: 21177}},
		{"Lvl-2 Enchant", 14286875, 27, 37, false,
			[][2]int{{564, 1}, {556, 3}},
			map[int]int{1696: 1729, 21111: 21163, 21120: 21180, 1639: 2552, 1658: 5521, 11076: 11079, 21084: 21129, 21093: 21146}},
		{"Lvl-3 Enchant", 14286888, 49, 59, false,
			[][2]int{{564, 1}, {554, 5}},
			map[int]int{1698: 1725, 1660: 11194, 21114: 21166, 1641: 2568, 11085: 11088, 21087: 21140, 21096: 21157, 21123: 21183}},
		{"Lvl-4 Enchant", 14286897, 57, 67, false,
			[][2]int{{564, 1}, {557, 10}},
			map[int]int{1700: 1731, 1643: 2570, 1662: 11090, 11092: 11095}},
		{"Lvl-5 Enchant", 14286912, 68, 78, true,
			[][2]int{{564, 1}, {555, 15}, {557, 15}},
			map[int]int{1702: 1704, 1645: 2572, 1664: 11113, 11115: 11126}},
		{"Lvl-6 Enchant", 14286925, 87, 97, true,
			[][2]int{{564, 1}, {554, 20}, {557, 20}},
			map[int]int{6581: 6585, 6575: 6583, 6577: 11128, 11130: 11133}},
		{"Lvl-7 Enchant", 14286928, 93, 110, true,
			[][2]int{{564, 1}, {565, 20}, {566, 20}},
			map[int]int{19541: 19553, 19538: 19550, 19535: 19547, 19532: 19544}},
	}
	if len(enchantSpells) != len(want) {
		t.Fatalf("enchantSpells = %d rows, want %d", len(enchantSpells), len(want))
	}
	for i, w := range want {
		spell := &enchantSpells[i]
		if spell.name != w.name || spell.spellID != w.spellID || spell.level != w.level ||
			spell.xp != w.xp || spell.members != w.members {
			t.Errorf("row %d: got %+v, want name %q id %d level %d xp %v members %v",
				i, spell, w.name, w.spellID, w.level, w.xp, w.members)
		}
		if len(spell.runes) != len(w.runes) {
			t.Errorf("%s: runes = %v, want %v", w.name, spell.runes, w.runes)
		} else {
			for j, r := range w.runes {
				if spell.runes[j] != r {
					t.Errorf("%s: rune %d = %v, want %v", w.name, j, spell.runes[j], r)
				}
			}
		}
		got := spell.results()
		if len(got) != len(w.results) {
			t.Errorf("%s: %d results, want %d", w.name, len(got), len(w.results))
		}
		for in, wantOut := range w.results {
			if got[in] != wantOut {
				t.Errorf("%s: result of %d = %d, want %d", w.name, in, got[in], wantOut)
			}
		}
		if got := spellFor("  " + strings.ToUpper(w.name) + " "); got != spell {
			t.Errorf("%s: spellFor(case/space-insensitive) did not resolve", w.name)
		}
	}
}

// TestEnchantLooks checks the per-kind presentation
// (Spells.EnchantJewellery.js looks): gold amulets play the
// spell's own animation/graphic/sound, rings play the ring
// cast (931/238/jewellerySound), silver amulets the spell
// animation with the ring graphic and spell sound, and the
// jade bracelet the ring look with the spell animation.
func TestEnchantLooks(t *testing.T) {
	lvl1 := spellFor("Lvl-1 Enchant")
	if got := lvl1.lookFor(fxSapphireAmulet); got != (look{719, 114, 136}) {
		t.Errorf("Lvl-1 sapphire amulet look = %+v, want {719 114 136}", got)
	}
	if got := lvl1.lookFor(fxSapphireRing); got != (look{931, 238, 147}) {
		t.Errorf("Lvl-1 sapphire ring look = %+v, want {931 238 147}", got)
	}
	if got := lvl1.lookFor(fxOpalAmulet); got != (look{719, 238, 136}) {
		t.Errorf("Lvl-1 opal amulet look = %+v, want {719 238 136}", got)
	}
	lvl2 := spellFor("Lvl-2 Enchant")
	if got := lvl2.lookFor(fxJadeBracelet); got != (look{720, 238, 142}) {
		t.Errorf("Lvl-2 jade bracelet look = %+v, want {720 238 142}", got)
	}
	lvl3 := spellFor("Lvl-3 Enchant")
	// Lvl-3 has no jewellerySound: rings play the spell sound.
	if got := lvl3.lookFor(fxTopazRing); got != (look{931, 238, 145}) {
		t.Errorf("Lvl-3 topaz ring look = %+v, want {931 238 145}", got)
	}
	// The ruby necklace plays the amulet cast.
	if got := lvl3.lookFor(1660); got != (look{721, 116, 145}) {
		t.Errorf("Lvl-3 ruby necklace look = %+v, want {721 116 145}", got)
	}
}

// TestEnchantSpellsRegistered proves the seven spells resolve
// through the game utility-spell registry (the Superheat
// GetUtilitySpell precedent) and that their canCast enforces
// the table's level and runes: Lvl-1 casts at Magic 7 with
// its runes, and refuses at Magic 6.
func TestEnchantSpellsRegistered(t *testing.T) {
	installEnchantSpells()
	for i := range enchantSpells {
		spell := &enchantSpells[i]
		caster := game.GetUtilitySpell(spell.spellID)
		if caster == nil {
			t.Errorf("%s: GetUtilitySpell(%d) = nil", spell.name, spell.spellID)
		}
	}
	lvl1 := spellFor("Lvl-1 Enchant")
	p, _ := newTestPlayer("registered")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	if caster := lvl1.spellCaster(); caster == nil || !caster.CanCast(p, false) {
		t.Error("Lvl-1 Enchant CanCast(false) at level 7 with runes = false, want true")
	}
	setLevel(p, game.SkillMagic, 6)
	if caster := lvl1.spellCaster(); caster != nil && caster.CanCast(p, false) {
		t.Error("Lvl-1 Enchant CanCast(false) at level 6 = true, want false")
	}
}

// ---- the handler ----

func TestHandlerWrongSpellUnclaimed(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("wrongspell")
	setLevel(p, game.SkillMagic, 43)
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Superheat Item", fxSapphireRing, 10)
	if ev.Handled {
		t.Error("Superheat Item on a sapphire ring was claimed by Enchant Jewellery")
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings = %d, want 1", got)
	}
}

func TestHandlerWrongItemRefused(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, sess := newTestPlayer("wrongitem")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxIronOre, 1)
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxIronOre, 10)
	if !ev.Handled {
		t.Error("Lvl-1 Enchant on iron ore not claimed")
	}
	if !hasMessage(sess, "This spell cannot be cast on that item.") {
		t.Errorf("messages = %v, want the cannot-cast line", sess.messages)
	}
	if got := p.Inventory().GetAmount(fxIronOre); got != 1 {
		t.Errorf("iron ore = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxCosmicRune); got != 1 {
		t.Errorf("cosmic runes = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a refused cast started a repeat loop")
	}
}

func TestHandlerUnknownSpellUnclaimed(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("unknownspell")
	setLevel(p, game.SkillMagic, 99)
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Lvl-8 Enchant", fxSapphireRing, 10)
	if ev.Handled {
		t.Error("an unknown spell name was claimed")
	}
}

func TestLvl1EnchantSapphireRing(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, sess := newTestPlayer("lvl1ring")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if !ev.Handled {
		t.Error("Lvl-1 Enchant on a sapphire ring not claimed")
	}
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("rings of recoil = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 0 {
		t.Errorf("sapphire rings left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxCosmicRune); got != 0 {
		t.Errorf("cosmic runes left = %d, want 0 (1 spent)", got)
	}
	if got := p.Inventory().GetAmount(fxWaterRune); got != 0 {
		t.Errorf("water runes left = %d, want 0 (1 spent)", got)
	}
	// 17.5 Magic XP truncates to 17 (residual (a)).
	if got := p.Skills().Experience(game.SkillMagic); got != 17 {
		t.Errorf("magic XP = %d, want 17", got)
	}
	// The ring look: animation 931, graphic 238 at height 92,
	// sound 147 (the jewellery sound).
	if anim := p.Animation(); anim == nil || anim.ID != 931 {
		t.Errorf("animation = %v, want 931", anim)
	}
	if gfx := p.Graphic(); gfx == nil || gfx.ID != 238 || gfx.Height != 92 {
		t.Errorf("graphic = %+v, want id 238 height 92", gfx)
	}
	if len(sess.sounds) != 1 || sess.sounds[0] != 147 {
		t.Errorf("sounds = %v, want [147]", sess.sounds)
	}
	// Once per accepted click, the cast switches back to the
	// spellbook tab (script 915, tab 6).
	if len(sess.scripts) != 1 || sess.scripts[0] != [2]int{915, 6} {
		t.Errorf("client scripts = %v, want [[915 6]]", sess.scripts)
	}
	// The repeat loop is registered, first repeat at 10 + 7.
	sessState := e.sessionFor(p)
	if sessState == nil {
		t.Fatal("no repeat session after a successful cast")
	}
	if sessState.nextCastAt != 17 {
		t.Errorf("nextCastAt = %d, want 17 (cycle 10 + 7)", sessState.nextCastAt)
	}
	// The cast stamps its own 3-tick cooldown and the shared
	// 1-tick delay.
	if got, ok := p.GetAttribute(nextCastAttribute).(int64); !ok || got != 13 {
		t.Errorf("next-cast-at = %v, want 13 (cycle 10 + 3)", got)
	}
	if got, ok := p.GetAttribute(sharedCastAttribute).(int64); !ok || got != 11 {
		t.Errorf("magic:next-cast-at = %v, want 11 (cycle 10 + 1)", got)
	}
}

func TestLvl1EnchantSapphireAmuletLook(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("lvl1amulet")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxSapphireAmulet, 1)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireAmulet, 10)
	if got := p.Inventory().GetAmount(fxAmuletOfMagic); got != 1 {
		t.Errorf("amulets of magic = %d, want 1", got)
	}
	// Gold amulets play the spell's own look: 719/114/136.
	if anim := p.Animation(); anim == nil || anim.ID != 719 {
		t.Errorf("animation = %v, want 719", anim)
	}
	if gfx := p.Graphic(); gfx == nil || gfx.ID != 114 || gfx.Height != 92 {
		t.Errorf("graphic = %+v, want id 114 height 92", gfx)
	}
}

func TestEnchantEverySpellOnce(t *testing.T) {
	installEnchantSpells()
	// One cast of each spell, on its amulet: level, runes,
	// product and (truncated) XP per the table.
	rows := []struct {
		spell string
		level int
		in    int
		out   int
		runes [][2]int
		xp    int
	}{
		{"Lvl-1 Enchant", 7, fxSapphireAmulet, fxAmuletOfMagic, [][2]int{{564, 1}, {555, 1}}, 17},
		{"Lvl-2 Enchant", 27, fxEmeraldAmulet, fxAmuletOfDefence, [][2]int{{564, 1}, {556, 3}}, 37},
		{"Lvl-3 Enchant", 49, fxRubyAmulet, fxAmuletOfStrength, [][2]int{{564, 1}, {554, 5}}, 59},
		{"Lvl-4 Enchant", 57, fxDiamondAmulet, fxAmuletOfPower, [][2]int{{564, 1}, {557, 10}}, 67},
		{"Lvl-5 Enchant", 68, fxDragonstoneAmulet, fxAmuletOfGlory, [][2]int{{564, 1}, {555, 15}, {557, 15}}, 78},
		{"Lvl-6 Enchant", 87, fxOnyxAmulet, fxAmuletOfFury, [][2]int{{564, 1}, {554, 20}, {557, 20}}, 97},
		{"Lvl-7 Enchant", 93, fxZenyteAmulet, fxAmuletOfTorture, [][2]int{{564, 1}, {565, 20}, {566, 20}}, 110},
	}
	for _, row := range rows {
		e := newTestEnchanter()
		p, _ := newTestPlayer("everyspell")
		setLevel(p, game.SkillMagic, row.level)
		for _, r := range row.runes {
			give(p, r[0], r[1])
		}
		give(p, row.in, 1)
		ev := castEnchant(e, p, row.spell, row.in, 10)
		if !ev.Handled {
			t.Errorf("%s: not claimed", row.spell)
			continue
		}
		if got := p.Inventory().GetAmount(row.out); got != 1 {
			t.Errorf("%s: products = %d, want 1", row.spell, got)
		}
		if got := p.Inventory().GetAmount(row.in); got != 0 {
			t.Errorf("%s: inputs left = %d, want 0", row.spell, got)
		}
		for _, r := range row.runes {
			if got := p.Inventory().GetAmount(r[0]); got != 0 {
				t.Errorf("%s: rune %d left = %d, want 0", row.spell, r[0], got)
			}
		}
		if got := p.Skills().Experience(game.SkillMagic); got != row.xp {
			t.Errorf("%s: magic XP = %d, want %d", row.spell, got, row.xp)
		}
	}
}

func TestEnchantBelowLevelRefused(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, sess := newTestPlayer("lowlevel")
	setLevel(p, game.SkillMagic, 6)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if !ev.Handled {
		t.Error("the click was not claimed")
	}
	if !hasMessage(sess, "You need a Magic level of 7 to cast this spell.") {
		t.Errorf("messages = %v, want the level refusal", sess.messages)
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings = %d, want 1 (unspent)", got)
	}
	if got := p.Inventory().GetAmount(fxCosmicRune); got != 1 {
		t.Errorf("cosmic runes = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a refused cast started a repeat loop")
	}
}

func TestEnchantOutOfRunesRefused(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, sess := newTestPlayer("norunes")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	// No water rune.
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if !ev.Handled {
		t.Error("the click was not claimed")
	}
	if !hasMessage(sess, "You do not have enough Water Runes to cast this spell.") {
		t.Errorf("messages = %v, want the rune refusal", sess.messages)
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a refused cast started a repeat loop")
	}
}

func TestEnchantMembersSpellOnFreeWorldRefused(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	e.membersWorld = false
	p, sess := newTestPlayer("freeworld")
	setLevel(p, game.SkillMagic, 68)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 15)
	give(p, fxEarthRune, 15)
	give(p, fxDragonstoneRing, 1)
	ev := castEnchant(e, p, "Lvl-5 Enchant", fxDragonstoneRing, 10)
	if !ev.Handled {
		t.Error("the click was not claimed")
	}
	if !hasMessage(sess, "You need to be on a members' world to cast this spell.") {
		t.Errorf("messages = %v, want the members-world refusal", sess.messages)
	}
	if got := p.Inventory().GetAmount(fxDragonstoneRing); got != 1 {
		t.Errorf("dragonstone rings = %d, want 1 (unspent)", got)
	}
	if got := p.Inventory().GetAmount(fxCosmicRune); got != 1 {
		t.Errorf("cosmic runes = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a refused cast started a repeat loop")
	}
}

func TestEnchantStaleSlotRefused(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("staleslot")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxSapphireRing, 1)
	// The click names a slot that no longer holds the ring.
	ev := &plugins.SpellOnItemEvent{
		Player: p, SpellName: "Lvl-1 Enchant",
		ItemID: fxSapphireRing, Slot: slotOf(p, fxSapphireRing) + 1, Cycle: 10,
	}
	e.castEnchantOnItem(ev)
	if !ev.Handled {
		t.Error("the click was not claimed")
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a stale-slot click started a repeat loop")
	}
}

// ---- the repeat loop ----

func TestRepeatEverySevenTicks(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("repeat")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 3)
	give(p, fxWaterRune, 3)
	give(p, fxSapphireRing, 3)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Fatalf("after the first cast products = %d, want 1", got)
	}
	// No repeat before tick 17.
	driveTicks(e, 11, 16)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("at tick 16 products = %d, want 1", got)
	}
	driveTicks(e, 17, 17)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 2 {
		t.Errorf("at tick 17 products = %d, want 2", got)
	}
	driveTicks(e, 18, 23)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 2 {
		t.Errorf("at tick 23 products = %d, want 2", got)
	}
	driveTicks(e, 24, 24)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 3 {
		t.Errorf("at tick 24 products = %d, want 3", got)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 51 {
		t.Errorf("magic XP = %d, want 51 (3 x 17)", got)
	}
	// Out of jewellery: the next repeat fails and the loop
	// stops.
	driveTicks(e, 25, 31)
	if e.sessionFor(p) != nil {
		t.Error("the loop is still running with no jewellery left")
	}
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 3 {
		t.Errorf("products = %d, want 3", got)
	}
	// Repeats never send the tab switch again.
	// (Only the accepted click sent script 915.)
}

func TestRepeatStopsOutOfRunes(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("repeatrunes")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 2)
	give(p, fxWaterRune, 2)
	give(p, fxSapphireRing, 3)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	driveTicks(e, 11, 17)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 2 {
		t.Fatalf("at tick 17 products = %d, want 2", got)
	}
	driveTicks(e, 18, 24)
	if e.sessionFor(p) != nil {
		t.Error("the loop is still running with no runes left")
	}
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 2 {
		t.Errorf("products = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings left = %d, want 1", got)
	}
}

func TestRepeatStopsOnWalkAway(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("repeatwalk")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 3)
	give(p, fxWaterRune, 3)
	give(p, fxSapphireRing, 3)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	start := p.Location()
	p.SetLocation(game.Location{X: start.X + 1, Y: start.Y, Z: start.Z})
	driveTicks(e, 11, 17)
	if e.sessionFor(p) != nil {
		t.Error("the loop is still running after walking away")
	}
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("products = %d, want 1 (no repeat after the walk)", got)
	}
}

func TestRepeatStopsOnDisconnect(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("repeatdc")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 3)
	give(p, fxWaterRune, 3)
	give(p, fxSapphireRing, 3)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if e.sessionFor(p) == nil {
		t.Fatal("no repeat session after the cast")
	}
	e.disconnect(&plugins.PlayerDisconnectEvent{Player: p})
	if e.sessionFor(p) != nil {
		t.Error("the loop is still running after disconnect")
	}
}

func TestQueuedCastWaitsForCooldown(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, sess := newTestPlayer("queued")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 2)
	give(p, fxWaterRune, 2)
	give(p, fxSapphireRing, 2)
	// A cast 2 ticks ago: the 3-tick cooldown runs to 12.
	p.SetAttribute(nextCastAttribute, int64(12))
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if !ev.Handled {
		t.Error("the queued click was not claimed")
	}
	// Nothing enchanted yet; the loop waits for tick 12.
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 0 {
		t.Errorf("products = %d, want 0 (queued, not cast)", got)
	}
	if got := p.Inventory().GetAmount(fxCosmicRune); got != 2 {
		t.Errorf("cosmic runes = %d, want 2 (unspent)", got)
	}
	sessState := e.sessionFor(p)
	if sessState == nil {
		t.Fatal("no queued session after the click")
	}
	if sessState.nextCastAt != 12 {
		t.Errorf("nextCastAt = %d, want 12", sessState.nextCastAt)
	}
	driveTicks(e, 11, 11)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 0 {
		t.Errorf("at tick 11 products = %d, want 0", got)
	}
	driveTicks(e, 12, 12)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("at tick 12 products = %d, want 1", got)
	}
	if sessState.nextCastAt != 19 {
		t.Errorf("nextCastAt after the queued cast = %d, want 19", sessState.nextCastAt)
	}
	if len(sess.scripts) != 1 || sess.scripts[0] != [2]int{915, 6} {
		t.Errorf("client scripts = %v, want one [[915 6]] for the accepted click", sess.scripts)
	}
}

func TestNewCastReplacesOldLoop(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("replaces")
	setLevel(p, game.SkillMagic, 27)
	give(p, fxCosmicRune, 5)
	give(p, fxWaterRune, 5)
	give(p, fxAirRune, 3)
	give(p, fxSapphireRing, 2)
	give(p, fxEmeraldAmulet, 1)
	// Start a Lvl-1 loop (next cast at 17), then click Lvl-2
	// on the emerald amulet at cycle 11: the old loop stops,
	// the new spell casts at once.
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if e.sessionFor(p) == nil || e.sessionFor(p).spell.name != "Lvl-1 Enchant" {
		t.Fatal("no Lvl-1 session after the first cast")
	}
	// The Lvl-1 cast stamped its cooldown to 13; the Lvl-2
	// click at 11 must queue, not cast at once.
	ev := castEnchant(e, p, "Lvl-2 Enchant", fxEmeraldAmulet, 11)
	if !ev.Handled {
		t.Error("the Lvl-2 click was not claimed")
	}
	sessState := e.sessionFor(p)
	if sessState == nil || sessState.spell.name != "Lvl-2 Enchant" {
		t.Fatalf("session after the Lvl-2 click = %v, want Lvl-2", sessState)
	}
	if sessState.nextCastAt != 13 {
		t.Errorf("nextCastAt = %d, want 13 (queued behind the cooldown)", sessState.nextCastAt)
	}
	driveTicks(e, 12, 13)
	if got := p.Inventory().GetAmount(fxAmuletOfDefence); got != 1 {
		t.Errorf("amulets of defence = %d, want 1 after the queued cast", got)
	}
	// The abandoned Lvl-1 loop never repeated.
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("rings of recoil = %d, want 1 (the first cast only)", got)
	}
}

func TestLevelUpPauseHoldsSession(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	p, _ := newTestPlayer("pause")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 2)
	give(p, fxWaterRune, 2)
	give(p, fxSapphireRing, 2)
	castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	sessState := e.sessionFor(p)
	if sessState == nil {
		t.Fatal("no repeat session after the cast")
	}
	// The emitter does not exist in Go yet; drive the handler
	// directly with a pop-up payload, as the smithing pause
	// handler is driven.
	var resume func()
	payload := &skillactions.LevelUpPopupOpenedPayload{
		Player: p,
		Skill:  game.SkillMagic,
		Pause: func(action any, resumeFn, cancel func()) {
			resume = resumeFn
		},
	}
	e.pauseSessions(payload)
	if !sessState.levelUpPaused {
		t.Fatal("the session was not paused by the level-up pop-up")
	}
	driveTicks(e, 11, 17)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 1 {
		t.Errorf("products while paused = %d, want 1", got)
	}
	if resume == nil {
		t.Fatal("the pop-up never registered a resume")
	}
	resume()
	if sessState.levelUpPaused {
		t.Error("the session is still paused after resume")
	}
	driveTicks(e, 18, 18)
	if got := p.Inventory().GetAmount(fxRingOfRecoil); got != 2 {
		t.Errorf("products after resume = %d, want 2", got)
	}
}

func TestRecipePolicyEventCanDecline(t *testing.T) {
	installEnchantSpells()
	e := newTestEnchanter()
	e.emit = func(name string, payload any) {
		if name == "magic:enchant-item-policy" {
			if req, ok := payload.(*EnchantRecipeRequest); ok {
				req.Declined = true
				req.Message = "You need to be on a members' world to enchant that item."
			}
		}
	}
	p, sess := newTestPlayer("policy")
	setLevel(p, game.SkillMagic, 7)
	give(p, fxCosmicRune, 1)
	give(p, fxWaterRune, 1)
	give(p, fxSapphireRing, 1)
	ev := castEnchant(e, p, "Lvl-1 Enchant", fxSapphireRing, 10)
	if !ev.Handled {
		t.Error("the click was not claimed")
	}
	if !hasMessage(sess, "You need to be on a members' world to enchant that item.") {
		t.Errorf("messages = %v, want the policy refusal", sess.messages)
	}
	if got := p.Inventory().GetAmount(fxSapphireRing); got != 1 {
		t.Errorf("sapphire rings = %d, want 1 (unspent)", got)
	}
	if e.sessionFor(p) != nil {
		t.Error("a declined cast started a repeat loop")
	}
}
