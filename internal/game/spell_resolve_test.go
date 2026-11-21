package game

import "testing"

// Spell-resolution tests (NetworkBuilder.ts:1786-1800,
// CombatSpells.ts:2105-2113, EffectSpells.ts:227-237,
// MagicOnItemPacketListener.ts:66-89). Fixtures only: fake
// spells registered under test ids, unregistered afterwards so
// no test leaks a spell into another package test.

func TestResolveMagicItemSpellID(t *testing.T) {
	cases := map[string]int{
		"Low Level Alchemy":     1162,
		"low level alchemy":     1162,
		"  High Level Alchemy ": 1178,
		"Telekinetic Grab":      1168,
		"Basic Reanimation":     11997,
		"Adept Reanimation":     2885,
		"Expert Reanimation":    6208,
		"Master Reanimation":    6888,
		"Wind Strike":           -1,
		"":                      -1,
	}
	for name, want := range cases {
		if got := ResolveMagicItemSpellID(name); got != want {
			t.Errorf("ResolveMagicItemSpellID(%q) = %d, want %d", name, got, want)
		}
	}
}

func TestCombatSpellRegistry(t *testing.T) {
	const id = 424242
	spell := newFakeSpell()
	// A unique name: the real standard spells are also
	// registered in this package's test binary (Worker A's
	// tests install them), so a real name like "Wind Strike"
	// would clobber a real entry and its unregister would
	// delete the real spell's name mapping.
	RegisterCombatSpellNamed(id, "Test Wind Strike", spell)
	defer UnregisterCombatSpell(id)

	if got := GetCombatSpell(id); got != spell {
		t.Fatalf("GetCombatSpell(%d) = %v, want the registered spell", id, got)
	}
	// By-name lookup normalizes the way
	// getCombatSpellByName does: display name, property name,
	// and case/punctuation variants all resolve.
	for _, name := range []string{"Test Wind Strike", "TEST_WIND_STRIKE", "test wind strike", "TestWindStrike"} {
		if got := GetCombatSpellByName(name); got != spell {
			t.Fatalf("GetCombatSpellByName(%q) = %v, want the registered spell", name, got)
		}
	}
	if got := GetCombatSpellByName("No Such Spell"); got != nil {
		t.Fatalf("GetCombatSpellByName(unknown) = %v, want nil", got)
	}
	if got := GetCombatSpell(424243); got != nil {
		t.Fatalf("GetCombatSpell(unknown) = %v, want nil", got)
	}
	// A combat spell also serves as the scripted route's
	// SpellCaster.
	combat, utility, ok := ResolveSpell(id)
	if !ok || combat != spell || utility == nil {
		t.Fatalf("ResolveSpell(%d) = (%v, %v, %v), want the combat spell on both", id, combat, utility, ok)
	}
	if _, _, ok := ResolveSpell(424243); ok {
		t.Fatal("ResolveSpell(unknown) ok, want false")
	}
	// Unregistering removes the id and the name entries.
	UnregisterCombatSpell(id)
	if got := GetCombatSpell(id); got != nil {
		t.Fatalf("after unregister GetCombatSpell(%d) = %v, want nil", id, got)
	}
	if got := GetCombatSpellByName("Test Wind Strike"); got != nil {
		t.Fatalf("after unregister GetCombatSpellByName = %v, want nil", got)
	}
	RegisterCombatSpellNamed(id, "Test Wind Strike", spell)
	defer UnregisterCombatSpell(id)
}

func TestUtilitySpellRegistry(t *testing.T) {
	const id = 424243
	spell := newFakeSpell()
	RegisterUtilitySpell(id, spell)
	defer UnregisterUtilitySpell(id)

	if got := GetUtilitySpell(id); got != spell {
		t.Fatalf("GetUtilitySpell(%d) = %v, want the registered spell", id, got)
	}
	// A utility spell resolves without a combat spell.
	combat, utility, ok := ResolveSpell(id)
	if !ok || combat != nil || utility != spell {
		t.Fatalf("ResolveSpell(%d) = (%v, %v, %v), want utility only", id, combat, utility, ok)
	}
	if got := GetCombatSpell(id); got != nil {
		t.Fatalf("GetCombatSpell(%d) = %v, want nil for a utility id", id, got)
	}
}

func TestResolveClientCombatSpellID(t *testing.T) {
	// A small id: the widget-low try (spellWidget & 0xffff)
	// can only match ids below 65536.
	const id = 42424
	spell := newFakeSpell()
	// Unique name for the same reason as
	// TestCombatSpellRegistry: real spells share this
	// package's test binary.
	RegisterCombatSpellNamed(id, "Test Fire Bolt", spell)
	defer UnregisterCombatSpell(id)

	// Id tries, in TS order: spellChild, spellWidget & 0xffff,
	// spellItemId.
	if got := ResolveClientCombatSpellID(0, id, -1, nil); got != id {
		t.Errorf("child try = %d, want %d", got, id)
	}
	if got := ResolveClientCombatSpellID(id, -1, -1, nil); got != id {
		t.Errorf("widget-low try = %d, want %d", got, id)
	}
	if got := ResolveClientCombatSpellID((1151<<16)|9999, -1, id, nil); got != id {
		t.Errorf("item try = %d, want %d", got, id)
	}
	// Nothing resolves without a cache lookup.
	if got := ResolveClientCombatSpellID(0, -1, -1, nil); got != -1 {
		t.Errorf("unresolvable = %d, want -1", got)
	}
	// Name fallback: the cache name resolves the spell by name,
	// then the MAGIC_ITEMS ids.
	nameLookup := func(widgetID, itemID int) (string, bool) {
		if widgetID == (1151<<16)|7 {
			return "Test Fire Bolt", true
		}
		if widgetID == (1151<<16)|8 {
			return "Low Level Alchemy", true
		}
		return "", false
	}
	if got := ResolveClientCombatSpellID((1151<<16)|7, -1, -1, nameLookup); got != id {
		t.Errorf("name fallback = %d, want %d", got, id)
	}
	if got := ResolveClientCombatSpellID((1151<<16)|8, -1, -1, nameLookup); got != 1162 {
		t.Errorf("magic-item fallback = %d, want 1162", got)
	}
	// The second widget form ((group << 16) | child) is tried
	// when the packet widget misses.
	secondForm := func(widgetID, itemID int) (string, bool) {
		if widgetID == (29999<<16)|9 {
			return "Test Fire Bolt", true
		}
		return "", false
	}
	if got := ResolveClientCombatSpellID((29999<<16)|3, 9, -1, secondForm); got != id {
		t.Errorf("second-form fallback = %d, want %d", got, id)
	}
	// An unknown name resolves to -1, not to an invented spell.
	unknown := func(widgetID, itemID int) (string, bool) { return "No Such Spell", true }
	if got := ResolveClientCombatSpellID((1151<<16)|7, -1, -1, unknown); got != -1 {
		t.Errorf("unknown name = %d, want -1", got)
	}
}
