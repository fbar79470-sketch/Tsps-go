package game

import (
	"testing"
)

// varbitSession is a fakeSession with a varbit store, for the
// examine-price settings gate.
type varbitSession struct {
	*fakeSession
	varbits map[int]int
}

func (s *varbitSession) GetVarbit(id int) int { return s.varbits[id] }

func TestFormatExaminePrice(t *testing.T) {
	cases := []struct {
		name     string
		amount   int
		prices   ExaminePrices
		showGE   bool
		showAlch bool
		want     string
		ok       bool
	}{
		{"Steel scimitar", 1, ExaminePrices{GE: 67, HA: 240, LA: 160}, true, true,
			"Price of Steel scimitar: GE <col=ff0000>67</col>  HA <col=ff0000>240</col>  LA <col=ff0000>160</col>", true},
		{"Feather", 100, ExaminePrices{GE: 3, HA: 1, LA: 1}, true, true,
			"Price of 100 x Feather: GE <col=ff0000>300</col> (<col=ff0000>3</col>ea)  HA <col=ff0000>100</col> (<col=ff0000>1</col>ea)  LA <col=ff0000>100</col> (<col=ff0000>1</col>ea)", true},
		{"Feather", 100, ExaminePrices{GE: 3, HA: 1, LA: 1}, true, false,
			"Price of 100 x Feather: GE <col=ff0000>300</col> (<col=ff0000>3</col>ea)", true},
		{"Bond", 1, ExaminePrices{GE: 5000, HA: 0, LA: 0}, true, true,
			"Price of Bond: GE <col=ff0000>5,000</col>", true},
		{"Worthless", 1, ExaminePrices{GE: 0, HA: 0, LA: 0}, true, true, "", false},
		{"Hidden", 1, ExaminePrices{GE: 67, HA: 240, LA: 160}, false, false, "", false},
		{"Counted", 0, ExaminePrices{GE: 67, HA: 0, LA: 0}, true, true,
			"Price of Counted: GE <col=ff0000>67</col>", true},
		{"Counted", -5, ExaminePrices{GE: 67, HA: 0, LA: 0}, true, false,
			"Price of Counted: GE <col=ff0000>67</col>", true},
		{"Big", 1234, ExaminePrices{GE: 1000, HA: 0, LA: 0}, true, false,
			"Price of 1,234 x Big: GE <col=ff0000>1,234,000</col> (<col=ff0000>1,000</col>ea)", true},
	}
	for _, tc := range cases {
		got, ok := FormatExaminePrice(tc.name, tc.amount, tc.prices, tc.showGE, tc.showAlch)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("FormatExaminePrice(%q, %d, %+v, %v, %v) = %q, %v, want %q, %v",
				tc.name, tc.amount, tc.prices, tc.showGE, tc.showAlch, got, ok, tc.want, tc.ok)
		}
	}
}

func TestExaminePricesFor(t *testing.T) {
	defs := newShopFixtureDefs()

	// Iron sword: GE quote, set alch values.
	name, prices, ok := ExaminePricesFor(100, defs)
	if !ok || name != "Iron sword" || prices != (ExaminePrices{GE: 150, HA: 60, LA: 40}) {
		t.Fatalf("ExaminePricesFor(100) = %q, %+v, %v", name, prices, ok)
	}
	// Feather: no GE quote, alch falls back to 60%/40% of value 3.
	name, prices, ok = ExaminePricesFor(101, defs)
	if !ok || name != "Feather" || prices != (ExaminePrices{GE: 0, HA: 1, LA: 1}) {
		t.Fatalf("ExaminePricesFor(101) = %q, %+v, %v", name, prices, ok)
	}
	// Bond: alchemy refuses it — set 0 stays 0, no fallback.
	_, prices, ok = ExaminePricesFor(102, defs)
	if !ok || prices.HA != 0 || prices.LA != 0 {
		t.Fatalf("ExaminePricesFor(102) = %+v, %v, want zero alch", prices, ok)
	}
	// A note prices as the item it notes.
	name, prices, ok = ExaminePricesFor(104, defs)
	if !ok || name != "Iron sword" || prices.GE != 150 {
		t.Fatalf("ExaminePricesFor(104) = %q, %+v, %v", name, prices, ok)
	}
	// Untradeable: no line at all.
	if _, _, ok := ExaminePricesFor(103, defs); ok {
		t.Fatal("ExaminePricesFor(103) ok, want not-ok (untradeable)")
	}
	// Nil source: not-ok.
	if _, _, ok := ExaminePricesFor(100, nil); ok {
		t.Fatal("ExaminePricesFor with nil source ok, want not-ok")
	}
}

func TestSendExaminePriceGating(t *testing.T) {
	defs := newShopFixtureDefs()
	p := NewPlayer("examiner", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)

	// No session at all: nothing sent.
	SendExaminePrice(p, 100, 1, defs)

	// A session without a varbit reader: nothing sent (the
	// varbit gap — no store exists, so no line).
	sess := newFakeSession()
	p.Session = sess
	SendExaminePrice(p, 100, 1, defs)
	if len(sess.messages) != 0 {
		t.Fatalf("messages without varbit reader = %q", sess.messages)
	}

	// Both settings off: nothing.
	vsess := &varbitSession{fakeSession: newFakeSession(), varbits: map[int]int{}}
	p.Session = vsess
	SendExaminePrice(p, 100, 1, defs)
	if len(vsess.messages) != 0 {
		t.Fatalf("messages with settings off = %q", vsess.messages)
	}

	// GE on, alch off: GE only.
	vsess.varbits[ExaminePriceGEVarbit] = 1
	SendExaminePrice(p, 100, 1, defs)
	if len(vsess.messages) != 1 || vsess.messages[0] != "Price of Iron sword: GE <col=ff0000>150</col>" {
		t.Fatalf("messages = %q", vsess.messages)
	}

	// Both on: the full line, with the stack form for a stack.
	vsess.varbits[ExaminePriceAlchVarbit] = 1
	SendExaminePrice(p, 101, 100, defs)
	want := "Price of 100 x Feather: HA <col=ff0000>100</col> (<col=ff0000>1</col>ea)  LA <col=ff0000>100</col> (<col=ff0000>1</col>ea)"
	if len(vsess.messages) != 2 || vsess.messages[1] != want {
		t.Fatalf("messages = %q, want second %q", vsess.messages, want)
	}
}

func TestExamineItem(t *testing.T) {
	defs := newShopFixtureDefs()
	p := NewPlayer("examiner", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	vsess := &varbitSession{fakeSession: newFakeSession(), varbits: map[int]int{
		ExaminePriceGEVarbit: 1, ExaminePriceAlchVarbit: 1,
	}}
	p.Session = vsess

	// Examine text first, then the price line.
	ExamineItem(p, 100, 1, defs)
	if len(vsess.messages) != 2 || vsess.messages[0] != "A sword." {
		t.Fatalf("messages = %q", vsess.messages)
	}
	if vsess.messages[1] != "Price of Iron sword: GE <col=ff0000>150</col>  HA <col=ff0000>60</col>  LA <col=ff0000>40</col>" {
		t.Fatalf("price line = %q", vsess.messages[1])
	}

	// An empty examine falls back to the name.
	ExamineItem(p, 102, 1, defs)
	if len(vsess.messages) != 3 || vsess.messages[2] != "Bond" {
		t.Fatalf("messages = %q", vsess.messages)
	}
}
