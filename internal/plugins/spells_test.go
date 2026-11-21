package plugins

import (
	"testing"

	"tsps-go/internal/game"
)

// Spell-family tests: the registration methods, emits, gate
// contracts, and rune-source registry in spells.go, grounded in
// PluginManager.ts (emitSpellDisabled 1824-1849,
// emitSpellRuneBypass 1851-1876, registration guards 3978-4013,
// checkSpellRuneSource 5056-5074, consumeSpellRuneSource
// 5076-5092, registerSpellRuneSource 4849-4861) and
// PluginTypes.ts (event shapes 663-679, 981-988). Fixtures only:
// newTestHub and testPlayer come from api_test.go / hooks_test.go.

// testRuneSource is a hand-built SpellRuneSource: holdings maps
// item id -> amount held; consumed records what Consume took.
type testRuneSource struct {
	holdings map[int]int
	consumed []game.Item
}

func (s *testRuneSource) Check(player *game.Player, items []game.Item) bool {
	for _, item := range items {
		if s.holdings[item.ID()] < item.Amount() {
			return false
		}
	}
	return true
}

func (s *testRuneSource) Consume(player *game.Player, items []game.Item) {
	for _, item := range items {
		s.holdings[item.ID()] -= item.Amount()
	}
	s.consumed = append(s.consumed, items...)
}

func TestOnSpellDisabledFirstAnswer(t *testing.T) {
	h, _ := newTestHub()
	player := testPlayer("caster")
	calls := 0
	newApi(h, "First").OnSpellDisabled(func(ev *SpellDisabledEvent) {
		calls++
		if ev.Player != player || ev.Spellbook != game.SpellbookNormal || ev.SpellID != 1152 {
			t.Errorf("event = %+v, want player + normal + 1152", ev)
		}
		// Silence: no answer.
	})
	newApi(h, "Second").OnSpellDisabled(func(ev *SpellDisabledEvent) {
		calls++
		ev.Disabled = BoolPtr(true)
	})
	newApi(h, "Third").OnSpellDisabled(func(*SpellDisabledEvent) {
		calls++
	})
	if !h.EmitSpellDisabled(player, game.SpellbookNormal, 1152, nil) {
		t.Fatal("EmitSpellDisabled = false, want true (second hook disabled)")
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2 (first answer wins)", calls)
	}
	// A false answer wins just as immediately as a true one.
	h2, _ := newTestHub()
	newApi(h2, "Deny").OnSpellDisabled(func(ev *SpellDisabledEvent) { ev.Disabled = BoolPtr(false) })
	newApi(h2, "Allow").OnSpellDisabled(func(ev *SpellDisabledEvent) { ev.Disabled = BoolPtr(true) })
	if h2.EmitSpellDisabled(player, game.SpellbookNormal, 1152, nil) {
		t.Fatal("EmitSpellDisabled = true, want false (first answer was false)")
	}
	// No hooks: false. Nil hub: false.
	h3, _ := newTestHub()
	if h3.EmitSpellDisabled(player, game.SpellbookNormal, 1152, nil) {
		t.Fatal("EmitSpellDisabled with no hooks = true, want false")
	}
	var nilHub *Hub
	if nilHub.EmitSpellDisabled(player, game.SpellbookNormal, 1152, nil) {
		t.Fatal("nil hub EmitSpellDisabled = true, want false")
	}
	// Player-less events never reach the handler.
	h4, _ := newTestHub()
	seen := false
	newApi(h4, "Watch").OnSpellDisabled(func(*SpellDisabledEvent) { seen = true })
	if h4.EmitSpellDisabled(nil, game.SpellbookNormal, 1152, nil) {
		t.Fatal("player-less emit = true, want false")
	}
	if seen {
		t.Fatal("player-less event reached the handler")
	}
	// A nil handler registers nothing.
	newApi(h4, "Nil").OnSpellDisabled(nil)
	if h4.EmitSpellDisabled(player, game.SpellbookNormal, 1152, nil) {
		t.Fatal("emit after nil registration = true, want false")
	}
}

func TestOnSpellRuneBypassFirstAnswer(t *testing.T) {
	h, _ := newTestHub()
	player := testPlayer("caster")
	var seenRune int
	newApi(h, "Bypass").OnSpellRuneBypass(func(ev *SpellRuneBypassEvent) {
		seenRune = ev.RuneID
		ev.Bypass = BoolPtr(true)
	})
	// Whole-cost query: rune id 0 (TS undefined).
	if !h.EmitSpellRuneBypass(player, game.SpellbookNormal, 1152, 0) {
		t.Fatal("whole-cost bypass = false, want true")
	}
	if seenRune != 0 {
		t.Fatalf("whole-cost RuneID = %d, want 0", seenRune)
	}
	// Per-rune query: the rune id passes through.
	if !h.EmitSpellRuneBypass(player, game.SpellbookNormal, 1152, 556) {
		t.Fatal("per-rune bypass = false, want true")
	}
	if seenRune != 556 {
		t.Fatalf("per-rune RuneID = %d, want 556", seenRune)
	}
	// Silence: false. Nil hub: false.
	h2, _ := newTestHub()
	if h2.EmitSpellRuneBypass(player, game.SpellbookNormal, 1152, 556) {
		t.Fatal("silent bypass = true, want false")
	}
	var nilHub *Hub
	if nilHub.EmitSpellRuneBypass(player, game.SpellbookNormal, 1152, 556) {
		t.Fatal("nil hub bypass = true, want false")
	}
}

func TestSpellRuneSourceNoSourceDefault(t *testing.T) {
	h, _ := newTestHub()
	player := testPlayer("caster")
	items := []game.Item{game.NewItem(556, 1)}
	// No source registered: check false, consume a no-op.
	if h.CheckSpellRuneSource(player, items) {
		t.Fatal("check with no source = true, want false")
	}
	h.ConsumeSpellRuneSource(player, items)
	if h.CheckSpellRuneSource(player, nil) || h.CheckSpellRuneSource(player, []game.Item{}) {
		t.Fatal("empty-list check = true, want false")
	}
	var nilHub *Hub
	if nilHub.CheckSpellRuneSource(player, items) {
		t.Fatal("nil hub check = true, want false")
	}
	nilHub.ConsumeSpellRuneSource(player, items)
}

func TestSpellRuneSourceCheckConsume(t *testing.T) {
	h, _ := newTestHub()
	player := testPlayer("caster")
	poor := &testRuneSource{holdings: map[int]int{556: 1}}
	rich := &testRuneSource{holdings: map[int]int{556: 5, 565: 2}}
	newApi(h, "Poor").RegisterSpellRuneSource(poor)
	newApi(h, "Rich").RegisterSpellRuneSource(rich)
	items := []game.Item{game.NewItem(556, 3), game.NewItem(565, 2)}
	// The first source cannot supply everything; the second can.
	if !h.CheckSpellRuneSource(player, items) {
		t.Fatal("check = false, want true (second source covers)")
	}
	// Consume deducts from the first covering source only.
	h.ConsumeSpellRuneSource(player, items)
	if rich.holdings[556] != 2 || rich.holdings[565] != 0 {
		t.Fatalf("rich holdings = %v, want {556: 2, 565: 0}", rich.holdings)
	}
	if len(poor.consumed) != 0 {
		t.Fatalf("poor consumed = %v, want none", poor.consumed)
	}
	if len(rich.consumed) != 2 {
		t.Fatalf("rich consumed = %v, want the 2 items", rich.consumed)
	}
	// After consumption the source no longer covers.
	if h.CheckSpellRuneSource(player, items) {
		t.Fatal("check after consumption = true, want false")
	}
}

func TestRegisterSpellRuneSourceNil(t *testing.T) {
	h, buf := newTestHub()
	player := testPlayer("caster")
	newApi(h, "Nil").RegisterSpellRuneSource(nil)
	if h.CheckSpellRuneSource(player, []game.Item{game.NewItem(556, 1)}) {
		t.Fatal("check after nil registration = true, want false")
	}
	if buf.Len() == 0 {
		t.Fatal("no warning logged for a nil source registration")
	}
	// A nil Api or nil hub is a no-op, never a panic.
	var nilApi *Api
	nilApi.RegisterSpellRuneSource(&testRuneSource{})
	(&Api{}).RegisterSpellRuneSource(&testRuneSource{})
}

func TestSpellRuneSourcePanicIsolation(t *testing.T) {
	h, _ := newTestHub()
	player := testPlayer("caster")
	newApi(h, "Thrower").RegisterSpellRuneSource(&panicRuneSource{})
	good := &testRuneSource{holdings: map[int]int{556: 1}}
	newApi(h, "Good").RegisterSpellRuneSource(good)
	items := []game.Item{game.NewItem(556, 1)}
	// The throwing source is skipped; the good source answers.
	if !h.CheckSpellRuneSource(player, items) {
		t.Fatal("check = false, want true (good source after a throwing one)")
	}
	h.ConsumeSpellRuneSource(player, items)
	if good.holdings[556] != 0 {
		t.Fatalf("good holdings = %v, want {556: 0}", good.holdings)
	}
}

// panicRuneSource throws from Check, as a buggy source would.
type panicRuneSource struct{}

func (panicRuneSource) Check(*game.Player, []game.Item) bool { panic("boom") }
func (panicRuneSource) Consume(*game.Player, []game.Item)    {}
