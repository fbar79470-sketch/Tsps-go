package game

import (
	"testing"
)

// spellbookPlayer builds a player with a recording fake session
// attached.
func spellbookPlayer() (*Player, *fakeSession) {
	p := NewPlayer("booker", Location{X: 3200, Y: 3200, Z: 0})
	session := newFakeSession()
	p.Session = session
	return p, session
}

func TestSpellbookInterfaceIDs(t *testing.T) {
	cases := []struct {
		book Spellbook
		id   int
		key  string
	}{
		{SpellbookNormal, 1151, "normal"},
		{SpellbookAncient, 12855, "ancient"},
		{SpellbookLunar, 29999, "lunar"},
		{SpellbookArceuus, 39999, "arceuus"},
	}
	for _, c := range cases {
		if got := c.book.InterfaceID(); got != c.id {
			t.Fatalf("book %v InterfaceID = %d, want %d", c.book, got, c.id)
		}
		if got := c.book.Key(); got != c.key {
			t.Fatalf("book %v Key = %q, want %q", c.book, got, c.key)
		}
		if got := SpellbookForInterfaceID(c.id); got != c.book {
			t.Fatalf("SpellbookForInterfaceID(%d) = %v, want %v", c.id, got, c.book)
		}
	}
	if got := SpellbookForInterfaceID(0); got != SpellbookNormal {
		t.Fatalf("SpellbookForInterfaceID(0) = %v, want NORMAL", got)
	}
	if got := SpellbookForInterfaceID(99999); got != SpellbookNormal {
		t.Fatalf("SpellbookForInterfaceID(99999) = %v, want NORMAL", got)
	}
	if got := Spellbook(99).InterfaceID(); got != 1151 {
		t.Fatalf("Spellbook(99).InterfaceID = %d, want 1151", got)
	}
	if got := Spellbook(99).Key(); got != "normal" {
		t.Fatalf("Spellbook(99).Key = %q, want normal", got)
	}
}

func TestPlayerSpellbookDefault(t *testing.T) {
	p := NewPlayer("booker", Location{X: 3200, Y: 3200, Z: 0})
	if got := p.Spellbook(); got != SpellbookNormal {
		t.Fatalf("fresh player spellbook = %v, want NORMAL", got)
	}
	if got := (*Player)(nil).Spellbook(); got != SpellbookNormal {
		t.Fatalf("nil player spellbook = %v, want NORMAL", got)
	}
	(*Player)(nil).SetSpellbook(SpellbookLunar)
	p.SetSpellbook(SpellbookAncient)
	if got := p.Spellbook(); got != SpellbookAncient {
		t.Fatalf("after SetSpellbook spellbook = %v, want ANCIENT", got)
	}
}

func TestChangeSpellbookSameBookRefreshesTab6(t *testing.T) {
	p, session := spellbookPlayer()
	p.SetSpellbook(SpellbookAncient)
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	if !ChangeSpellbook(p, SpellbookAncient, false, nil) {
		t.Fatal("same-book change returned false")
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{6, 12855} {
		t.Fatalf("tab interfaces = %v, want [[6 12855]]", session.tabInterfaces)
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none", session.messages)
	}
	if p.GetCombat().AutoCastSpell() == nil {
		t.Fatal("same-book change cleared the autocast spell")
	}
}

func TestChangeSpellbookLunarGate(t *testing.T) {
	p, session := spellbookPlayer()
	p.Skills().SetMaxLevel(SkillDefence, 39)
	if ChangeSpellbook(p, SpellbookLunar, false, nil) {
		t.Fatal("Lunar change below 40 Defence returned true")
	}
	if got := p.Spellbook(); got != SpellbookNormal {
		t.Fatalf("refused change moved spellbook to %v", got)
	}
	if len(session.messages) != 1 || session.messages[0] != "You need at least level 40 Defence to use the Lunar spellbook." {
		t.Fatalf("messages = %v, want the Defence refusal", session.messages)
	}
	if len(session.tabInterfaces) != 0 {
		t.Fatalf("tab interfaces = %v, want none", session.tabInterfaces)
	}
	// 40 Defence passes.
	p.Skills().SetMaxLevel(SkillDefence, 40)
	if !ChangeSpellbook(p, SpellbookLunar, false, nil) {
		t.Fatal("Lunar change at 40 Defence returned false")
	}
	if got := p.Spellbook(); got != SpellbookLunar {
		t.Fatalf("spellbook = %v, want LUNAR", got)
	}
	// Bypass passes at any level.
	p.SetSpellbook(SpellbookNormal)
	p.Skills().SetMaxLevel(SkillDefence, 1)
	if !ChangeSpellbook(p, SpellbookLunar, true, nil) {
		t.Fatal("bypassed Lunar change returned false")
	}
}

func TestChangeSpellbookSwitch(t *testing.T) {
	p, session := spellbookPlayer()
	p.GetCombat().SetAutoCastSpell(newFakeSpell())
	var emitted *SpellbookChangedPayload
	p.EmitCustomEvent = func(name string, payload any) {
		if name != SpellbookChangedEventName {
			t.Fatalf("event name = %q, want %q", name, SpellbookChangedEventName)
		}
		if pl, ok := payload.(SpellbookChangedPayload); ok {
			emitted = &pl
		} else {
			t.Fatalf("payload = %#v, want SpellbookChangedPayload", payload)
		}
	}
	if !ChangeSpellbook(p, SpellbookAncient, false, nil) {
		t.Fatal("switch returned false")
	}
	if got := p.Spellbook(); got != SpellbookAncient {
		t.Fatalf("spellbook = %v, want ANCIENT", got)
	}
	if p.GetCombat().AutoCastSpell() != nil {
		t.Fatal("switch did not reset autocast")
	}
	if len(session.messages) != 1 || session.messages[0] != defaultSpellbookChangeMessage {
		t.Fatalf("messages = %v, want the default switch line", session.messages)
	}
	if len(session.tabInterfaces) != 1 || session.tabInterfaces[0] != [2]int{6, 12855} {
		t.Fatalf("tab interfaces = %v, want [[6 12855]]", session.tabInterfaces)
	}
	if emitted == nil {
		t.Fatal("no magic:spellbook-changed event emitted")
	}
	if emitted.Player != p || emitted.Spellbook != "ancient" {
		t.Fatalf("event = %+v, want player + ancient", emitted)
	}
}

func TestChangeSpellbookMessageForms(t *testing.T) {
	p, session := spellbookPlayer()
	custom := "Switched by a quest."
	if !ChangeSpellbook(p, SpellbookLunar, true, &custom) {
		t.Fatal("switch with custom message returned false")
	}
	if len(session.messages) != 1 || session.messages[0] != custom {
		t.Fatalf("messages = %v, want the custom line", session.messages)
	}
	empty := ""
	p.SetSpellbook(SpellbookNormal)
	session.messages = nil
	if !ChangeSpellbook(p, SpellbookArceuus, true, &empty) {
		t.Fatal("switch with empty message returned false")
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none (empty message = TS null)", session.messages)
	}
}

func TestChangeSpellbookNilPlayer(t *testing.T) {
	if ChangeSpellbook(nil, SpellbookLunar, true, nil) {
		t.Fatal("nil player change returned true")
	}
}
