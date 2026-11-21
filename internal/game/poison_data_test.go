package game

import (
	"testing"
)

// CombatPoisonData table verification
// (CombatPoisonEffect.ts CombatPoisonData.init): counts and
// spot-checks against the upstream identifiers. The table
// itself was script-extracted (see poison_data_tables.go),
// never hand-typed.

func TestPoisonDataTableCounts(t *testing.T) {
	if got := len(poisonedItemTypes); got != 258 {
		t.Fatalf("poisoned item entries = %d, want 258 (upstream types.set calls)", got)
	}
	seen := make(map[int]bool, len(poisonedItemTypes))
	counts := map[PoisonType]int{}
	for _, entry := range poisonedItemTypes {
		if seen[entry.itemID] {
			t.Errorf("duplicate item id %d in poison table", entry.itemID)
		}
		seen[entry.itemID] = true
		counts[entry.typ]++
	}
	want := map[PoisonType]int{
		PoisonTypeVeryWeak: 43,
		PoisonTypeWeak:     43,
		PoisonTypeMild:     75,
		PoisonTypeExtra:    32,
		PoisonTypeSuper:    62,
		PoisonTypeVenom:    3,
	}
	for typ, wantCount := range want {
		if counts[typ] != wantCount {
			t.Errorf("type %d entries = %d, want %d", typ, counts[typ], wantCount)
		}
	}
	if got := len(poisonTypeByItemID); got != 258 {
		t.Errorf("poison map entries = %d, want 258", got)
	}
}

func TestPoisonDataTableSpotChecks(t *testing.T) {
	cases := []struct {
		id   int
		name string
		typ  PoisonType
	}{
		{812, "BRONZE_DART_P_", PoisonTypeVeryWeak},
		{893, "RUNE_ARROW_P_", PoisonTypeVeryWeak},
		{1221, "BRONZE_DAGGER_P_", PoisonTypeMild},
		{5616, "BRONZE_ARROW_P_PLUS_", PoisonTypeWeak},
		{5680, "DRAGON_DAGGER_P_PLUS_", PoisonTypeExtra},
		{5696, "RUNE_DAGGER_P_PLUS_PLUS_", PoisonTypeSuper},
		{10584, "KERIS_P_PLUS_PLUS_", PoisonTypeSuper},
		{25857, "AMETHYST_DART_P_PLUS_PLUS_", PoisonTypeMild},
		{12926, "TOXIC_BLOWPIPE", PoisonTypeVenom},
		{29796, "NOXIOUS_HALBERD", PoisonTypeVenom},
		{33178, "NOXIOUS_HALBERD_4", PoisonTypeVenom},
	}
	for _, tc := range cases {
		got, ok := PoisonTypeForItemID(tc.id)
		if !ok || got != tc.typ {
			t.Errorf("PoisonTypeForItemID(%d %s) = (%d, %v), want (%d, true)", tc.id, tc.name, got, ok, tc.typ)
		}
	}
	// Plain (unpoisoned) items and unknown ids have no type.
	for _, id := range []int{882, 892, 1213, 0, -1, 999999} {
		if typ, ok := PoisonTypeForItemID(id); ok {
			t.Errorf("PoisonTypeForItemID(%d) = (%d, true), want no type", id, typ)
		}
	}
	// getPoisonType rejects invalid items (id < 1, amount < 1).
	if _, ok := PoisonTypeForItem(NewItem(812, 0)); ok {
		t.Error("amount 0 item should have no poison type")
	}
	if _, ok := PoisonTypeForItem(NewItem(0, 1)); ok {
		t.Error("id 0 item should have no poison type")
	}
	if typ, ok := PoisonTypeForItem(NewItem(5696, 1)); !ok || typ != PoisonTypeSuper {
		t.Errorf("PoisonTypeForItem(rune dagger (p++)) = (%d, %v), want (SUPER, true)", typ, ok)
	}
}
