package smithing

import (
	"sync"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

const (
	fxFireRune     = 554
	fxNatureRune   = 561
	fxJogreBones   = 3125
	fxBurntJogre   = 3127
	fxElementalOre = 2892
)

// installSpellsOnce installs the standard spell table (the
// production boot wiring's InstallStandardSpells) once for
// the Superheat tests: the utility spell only exists after
// registration, and test binaries never boot. The
// spellwiring_test.go precedent installs it the same way.
var installSpellsOnce sync.Once

func installSuperheatSpells() {
	installSpellsOnce.Do(game.InstallStandardSpells)
}

// slotOf is the first inventory slot holding id, or -1.
func slotOf(p *game.Player, id int) int {
	for i, item := range p.Inventory().Items() {
		if item.IsValid() && item.ID() == id {
			return i
		}
	}
	return -1
}

// superheatPlayer is a caster with the runes for one cast.
func superheatPlayer(t *testing.T, name string) *game.Player {
	t.Helper()
	p, _ := newTestPlayer(name)
	setLevel(p, game.SkillMagic, 43)
	give(p, fxFireRune, 4)
	give(p, fxNatureRune, 1)
	return p
}

// castSuperheat casts Superheat Item on the player's ore at
// the given cycle, returning the event for Handled checks.
func castSuperheat(s *smithing, p *game.Player, itemID, cycle int) *plugins.SpellOnItemEvent {
	ev := &plugins.SpellOnItemEvent{
		Player:    p,
		SpellName: "Superheat Item",
		ItemID:    itemID,
		Slot:      slotOf(p, itemID),
		Cycle:     int64(cycle),
	}
	s.castSuperheatOnItem(ev)
	return ev
}

func TestSuperheatIronToSteel(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 2)
	ev := castSuperheat(s, p, fxIronOre, 10)
	if !ev.Handled {
		t.Error("superheat on iron ore not claimed")
	}
	if got := p.Inventory().GetAmount(fxSteelBar); got != 1 {
		t.Errorf("steel bars = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxIronOre); got != 0 {
		t.Errorf("iron ore left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxCoal); got != 0 {
		t.Errorf("coal left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 0 {
		t.Errorf("fire runes left = %d, want 0 (4 spent)", got)
	}
	if got := p.Inventory().GetAmount(fxNatureRune); got != 0 {
		t.Errorf("nature runes left = %d, want 0 (1 spent)", got)
	}
	// 17.5 Smithing XP truncates to 17; Magic XP is 53.
	if got := p.Skills().Experience(game.SkillSmithing); got != 17 {
		t.Errorf("smithing XP = %d, want 17", got)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 53 {
		t.Errorf("magic XP = %d, want 53", got)
	}
	if got, ok := p.GetAttribute(superheatNextCastAttr).(int64); !ok || got != 13 {
		t.Errorf("next-cast-at = %v, want 13 (cycle 10 + 3)", got)
	}
}

func TestSuperheatIronBelowSteelLevel(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 29)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 2)
	castSuperheat(s, p, fxIronOre, 10)
	// Below 30 Smithing the iron fallback smelts iron, never
	// failing, and the coal is untouched.
	if got := p.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("iron bars = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("steel bars = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxCoal); got != 2 {
		t.Errorf("coal left = %d, want 2", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 12 {
		t.Errorf("smithing XP = %d, want 12 (12.5 truncates)", got)
	}
}

func TestSuperheatIronOneCoalFallsBack(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 1)
	castSuperheat(s, p, fxIronOre, 10)
	if got := p.Inventory().GetAmount(fxIronBar); got != 1 {
		t.Errorf("iron bars = %d, want 1 (one coal is not steel)", got)
	}
	if got := p.Inventory().GetAmount(fxCoal); got != 1 {
		t.Errorf("coal left = %d, want 1", got)
	}
}

func TestSuperheatCatalystHalvesCoalDoublesXP(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 1)
	give(p, 27017, 1) // smithing catalyst
	castSuperheat(s, p, fxIronOre, 10)
	if got := p.Inventory().GetAmount(fxSteelBar); got != 1 {
		t.Errorf("steel bars = %d, want 1 (one coal + catalyst)", got)
	}
	if got := p.Inventory().GetAmount(fxCoal); got != 0 {
		t.Errorf("coal left = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(27017); got != 0 {
		t.Errorf("catalyst left = %d, want 0 (1 spent per smelt)", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 35 {
		t.Errorf("smithing XP = %d, want 35 (17.5 doubled)", got)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 53 {
		t.Errorf("magic XP = %d, want 53 (not doubled)", got)
	}
}

func TestSuperheatSilverOre(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 20)
	give(p, fxSilverOre, 1)
	castSuperheat(s, p, fxSilverOre, 10)
	if got := p.Inventory().GetAmount(fxSilverBar); got != 1 {
		t.Errorf("silver bars = %d, want 1", got)
	}
	if got := p.Skills().Experience(game.SkillSmithing); got != 13 {
		t.Errorf("smithing XP = %d, want 13 (13.7 truncates)", got)
	}
}

func TestSuperheatJogreBones(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillCooking, 30)
	give(p, fxJogreBones, 1)
	slot := slotOf(p, fxJogreBones)
	castSuperheat(s, p, fxJogreBones, 10)
	// The bones burn in place: the same slot holds burnt bones.
	if got := p.Inventory().GetAmount(fxBurntJogre); got != 1 {
		t.Errorf("burnt jogre bones = %d, want 1", got)
	}
	if got := p.Inventory().GetAmount(fxJogreBones); got != 0 {
		t.Errorf("jogre bones left = %d, want 0", got)
	}
	if item := p.Inventory().ItemAt(slot); !item.IsValid() || item.ID() != fxBurntJogre {
		t.Errorf("slot %d = %v, want burnt jogre bones in place", slot, item)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 53 {
		t.Errorf("magic XP = %d, want 53", got)
	}
	if got := p.Skills().Experience(game.SkillCooking); got != 25 {
		t.Errorf("cooking XP = %d, want 25", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 0 {
		t.Errorf("fire runes left = %d, want 0", got)
	}

	// Cooking 29 refuses, runes untouched.
	p2 := superheatPlayer(t, "superheat")
	setLevel(p2, game.SkillCooking, 29)
	give(p2, fxJogreBones, 1)
	// The refusal message goes to p2's own session.
	p2Session := p2.Session.(*fakeSession)
	castSuperheat(s, p2, fxJogreBones, 10)
	if got := p2.Inventory().GetAmount(fxBurntJogre); got != 0 {
		t.Errorf("burnt jogre bones = %d, want 0", got)
	}
	if got := p2.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4 (no cast)", got)
	}
	if !hasMessage(p2Session, "Cooking level of at least 30") {
		t.Errorf("messages = %v, want the Cooking-30 line", p2Session.messages)
	}
}

func TestSuperheatFailPaths(t *testing.T) {
	installSuperheatSpells()
	// Coal: not an ore, the fail message, no runes, no XP.
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxCoal, 1)
	pSess := p.Session.(*fakeSession)
	castSuperheat(s, p, fxCoal, 10)
	if !hasMessage(pSess, "You need to cast superheat item on ore.") {
		t.Errorf("messages = %v, want the not-ore line", pSess.messages)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4 (fail spends none)", got)
	}
	if got := p.Skills().Experience(game.SkillMagic); got != 0 {
		t.Errorf("magic XP = %d, want 0", got)
	}

	// Elemental ore: not hot enough, same fail shape.
	p2 := superheatPlayer(t, "superheat")
	give(p2, fxElementalOre, 1)
	p2Sess := p2.Session.(*fakeSession)
	castSuperheat(s, p2, fxElementalOre, 10)
	if !hasMessage(p2Sess, "Even this spell is not hot enough") {
		t.Errorf("messages = %v, want the not-hot-enough line", p2Sess.messages)
	}
	if got := p2.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4", got)
	}

	// A hammer is not an ore either.
	p3 := superheatPlayer(t, "superheat")
	give(p3, fxHammer, 1)
	p3Sess := p3.Session.(*fakeSession)
	castSuperheat(s, p3, fxHammer, 10)
	if !hasMessage(p3Sess, "You need to cast superheat item on ore.") {
		t.Errorf("messages = %v, want the not-ore line", p3Sess.messages)
	}

	// Smithing 14 cannot smelt iron (level 15): the level
	// message, no cast.
	p4 := superheatPlayer(t, "superheat")
	setLevel(p4, game.SkillSmithing, 14)
	give(p4, fxIronOre, 1)
	p4Sess := p4.Session.(*fakeSession)
	castSuperheat(s, p4, fxIronOre, 10)
	if !hasMessage(p4Sess, "Smithing level of at least 15") {
		t.Errorf("messages = %v, want the level-15 line", p4Sess.messages)
	}
	if got := p4.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4", got)
	}
}

func TestSuperheatNoCastWithoutRunesOrLevel(t *testing.T) {
	installSuperheatSpells()
	// Magic 42: the spell cannot be paid, nothing happens.
	s, _ := newTestSmithing(t)
	p, _ := newTestPlayer("superheat")
	setLevel(p, game.SkillMagic, 42)
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxFireRune, 4)
	give(p, fxNatureRune, 1)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 2)
	ev := castSuperheat(s, p, fxIronOre, 10)
	if !ev.Handled {
		t.Error("low-Magic superheat not claimed (the spell owns the click)")
	}
	if got := p.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("steel bars = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4", got)
	}

	// No nature rune: same.
	p2, _ := newTestPlayer("superheat")
	setLevel(p2, game.SkillMagic, 43)
	setLevel(p2, game.SkillSmithing, 30)
	give(p2, fxFireRune, 4)
	give(p2, fxIronOre, 1)
	give(p2, fxCoal, 2)
	castSuperheat(s, p2, fxIronOre, 10)
	if got := p2.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("steel bars = %d, want 0", got)
	}
}

func TestSuperheatPace(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxFireRune, 4) // a second cast's runes
	give(p, fxNatureRune, 1)
	give(p, fxIronOre, 2)
	give(p, fxCoal, 4)
	castSuperheat(s, p, fxIronOre, 10)
	if got := p.Inventory().GetAmount(fxSteelBar); got != 1 {
		t.Fatalf("first cast: steel bars = %d, want 1", got)
	}
	// Cycle 12 is inside the 3-tick pace: refused, no bar,
	// no runes spent.
	castSuperheat(s, p, fxIronOre, 12)
	if got := p.Inventory().GetAmount(fxSteelBar); got != 1 {
		t.Errorf("cycle 12: steel bars = %d, want 1 (pace refused)", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("cycle 12: fire runes left = %d, want 4", got)
	}
	// Cycle 13 is the next castable tick: the second bar.
	castSuperheat(s, p, fxIronOre, 13)
	if got := p.Inventory().GetAmount(fxSteelBar); got != 2 {
		t.Errorf("cycle 13: steel bars = %d, want 2", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 0 {
		t.Errorf("cycle 13: fire runes left = %d, want 0", got)
	}
}

func TestSuperheatNonMatchAndStaleSlot(t *testing.T) {
	installSuperheatSpells()
	s, _ := newTestSmithing(t)
	p := superheatPlayer(t, "superheat")
	setLevel(p, game.SkillSmithing, 30)
	give(p, fxIronOre, 1)
	give(p, fxCoal, 2)

	// Another spell does not claim the click.
	ev := &plugins.SpellOnItemEvent{Player: p, SpellName: "Low Level Alchemy", ItemID: fxIronOre, Slot: slotOf(p, fxIronOre), Cycle: 10}
	s.castSuperheatOnItem(ev)
	if ev.Handled {
		t.Error("Low Level Alchemy claimed by Superheat")
	}
	if got := p.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("steel bars = %d, want 0", got)
	}

	// A stale slot (the slot no longer holds the ore) claims
	// the click but casts nothing.
	ev = &plugins.SpellOnItemEvent{Player: p, SpellName: "Superheat Item", ItemID: fxIronOre, Slot: slotOf(p, fxCoal), Cycle: 10}
	s.castSuperheatOnItem(ev)
	if !ev.Handled {
		t.Error("stale slot not claimed")
	}
	if got := p.Inventory().GetAmount(fxSteelBar); got != 0 {
		t.Errorf("steel bars = %d, want 0", got)
	}
	if got := p.Inventory().GetAmount(fxFireRune); got != 4 {
		t.Errorf("fire runes left = %d, want 4", got)
	}
}
