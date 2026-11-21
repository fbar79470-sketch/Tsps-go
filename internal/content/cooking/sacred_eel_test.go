package cooking

import (
	"testing"
)

// Sacred Eel unit tests (SacredEel.Cooking.js): hand-built
// fixtures only. Fixture ids: eel 500, scales 501, knife 946
// (fxKnife).

func wcEelResolver() wcMapResolver {
	return wcMapResolver{objs: map[string]int{
		"sacred_eel":     500,
		"zulrahs_scales": 501,
	}}
}

func TestSacredEelDissect(t *testing.T) {
	c, emitted, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	u.randFloat = func() float64 { return 0 }
	p := newTestPlayer("alice")
	setCooking(p, 72)
	give(p, 500, 1)
	give(p, fxKnife, 1)
	u.dissect(wcItemOnItem(p, fxKnife, 500))
	if got := countOf(p, 500); got != 0 {
		t.Fatalf("eel = %d, want 0", got)
	}
	// Level 72 → band 0; roll 0 → scales 3 + 0 + 0.
	if got := countOf(p, 501); got != 3 {
		t.Fatalf("scales = %d, want 3", got)
	}
	if got := wcXP(p); got != 100+3*3 {
		t.Fatalf("xp = %d, want 109", got)
	}
	// cookingLevel emits a level-boost event too, so filter
	// by name: exactly one dissected + one produced.
	var dissected, producedCount int
	for _, e := range *emitted {
		switch e.name {
		case SacredEelEventName:
			dissected++
		case SkillsProducedEventName:
			producedCount++
		}
	}
	if dissected != 1 || producedCount != 1 {
		t.Fatalf("emitted = %d dissected + %d produced, want 1 + 1", dissected, producedCount)
	}
	produced := wcProduced(emitted)
	if len(produced) != 1 || produced[0].Kind != "sacred-eel" || produced[0].ItemID != 501 ||
		produced[0].Amount != 3 || produced[0].XP != 109 {
		t.Fatalf("produced = %+v, want one sacred-eel event for 3 scales / 109 xp", produced)
	}
}

func TestSacredEelBandAndRoll(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	u.randFloat = func() float64 { return 0.99 }
	p := newTestPlayer("alice")
	setCooking(p, 80) // band (80-72)/8 = 1
	give(p, 500, 1)
	give(p, fxKnife, 1)
	u.dissect(wcItemOnItem(p, 500, fxKnife))
	// Scales 3 + band 1 + floor(0.99*3) = 6; xp 100 + 3*6.
	if got := countOf(p, 501); got != 6 {
		t.Fatalf("scales = %d, want 6", got)
	}
	if got := wcXP(p); got != 118 {
		t.Fatalf("xp = %d, want 118", got)
	}
}

func TestSacredEelBandCapsAtFour(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	u.randFloat = func() float64 { return 0 }
	p := newTestPlayer("alice")
	setCooking(p, 99) // band min(4, 27/8) = 3
	give(p, 500, 1)
	give(p, fxKnife, 1)
	u.dissect(wcItemOnItem(p, fxKnife, 500))
	if got := countOf(p, 501); got != 6 {
		t.Fatalf("scales = %d, want 6 (3 + band 3 + 0)", got)
	}
}

func TestSacredEelLevelGate(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 71)
	give(p, 500, 1)
	give(p, fxKnife, 1)
	u.dissect(wcItemOnItem(p, fxKnife, 500))
	if got := countOf(p, 500); got != 1 {
		t.Fatalf("eel = %d, want 1 (kept)", got)
	}
	if got := countOf(p, 501); got != 0 {
		t.Fatalf("scales = %d, want 0 (refused)", got)
	}
	if len(sess.messages) != 1 || sess.messages[0] != "You need a Cooking level of 72 to dissect sacred eels." {
		t.Fatalf("messages = %v, want the level message", sess.messages)
	}
}

func TestSacredEelFreeWorldSilent(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	p := newTestPlayer("alice")
	sess := newWCTestSession()
	p.Session = sess
	setCooking(p, 99)
	c.membersWorld = false
	give(p, 500, 1)
	give(p, fxKnife, 1)
	u.dissect(wcItemOnItem(p, fxKnife, 500))
	if got := countOf(p, 500); got != 1 {
		t.Fatalf("eel = %d, want 1 (kept)", got)
	}
	if got := countOf(p, 501); got != 0 {
		t.Fatalf("scales = %d, want 0 (silent return)", got)
	}
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %v, want none (silent return)", sess.messages)
	}
}

func TestSacredEelPair(t *testing.T) {
	c, _, _ := newTestCooking()
	u := newSacredEelUnit(c, wcEelResolver())
	p := newTestPlayer("alice")
	if !u.pair(wcItemOnItem(p, fxKnife, 500)) {
		t.Fatal("pair(knife, eel) = false, want true")
	}
	if !u.pair(wcItemOnItem(p, 500, fxKnife)) {
		t.Fatal("pair(eel, knife) = false, want true")
	}
	if u.pair(wcItemOnItem(p, fxKnife, fxGrapes)) {
		t.Fatal("pair(knife, grapes) = true, want false")
	}
	if u.pair(wcItemOnItem(p, 500, 500)) {
		t.Fatal("pair(eel, eel) = true, want false")
	}
}
