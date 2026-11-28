package main

import (
	"testing"

	"tsps-go/internal/content/npcdrops"
	"tsps-go/internal/definition"
)

// The converter is the seam where Worker A's loader types meet Worker
// B's roller types. These tests lock the field-for-field mapping
// against hand-built loader values (the real-file counts are asserted
// in internal/definition's smoke test; the live legs prove the
// end-to-end roll).

func TestConvertNpcDropEntry(t *testing.T) {
	def := definition.NpcDropEntry{
		ItemID:    526,
		HasItemID: true,
		Name:      "Bones",
		Quantity:  [2]int{1, 1},
		Weight:    3,
		OutOf:     128,
		Rarity:    0.0234375,
		Section:   "Weapons and armour",
		Always:    true,
		Notes: []string{
			"Quest requirement: WikiQuestDropRequirement(questKey=quest_ragandboneman1, mode=RequiresDuring)",
		},
		QuantityAlternatives: []definition.NpcDropQuantityAlternative{
			{Quantity: [2]int{1, 1}},
			{Quantity: [2]int{2, 2}, Condition: &definition.NpcDropQuantityCondition{Type: "perfect_kill"}},
		},
		RollCountWeights: []definition.NpcDropRollCountWeight{{Rolls: 2, Weight: 11}},
		BonusDrops:       []definition.NpcDropBonusDrop{{ItemID: 21545, Name: "Pyrophosphite", Quantity: [2]int{1, 1}}},
	}
	got := convertNpcDropEntry(def)
	if got.ItemID != 526 || got.Name != "Bones" || got.Quantity != [2]int{1, 1} {
		t.Fatalf("identity fields: %+v", got)
	}
	if got.Weight != 3 || got.OutOf != 128 || got.Rarity != 0.0234375 {
		t.Fatalf("odds fields: weight=%v outOf=%v rarity=%v", got.Weight, got.OutOf, got.Rarity)
	}
	if !got.Always || got.Section != "Weapons and armour" {
		t.Fatalf("flags: always=%v section=%q", got.Always, got.Section)
	}
	if got.QuestKey != "quest_ragandboneman1" || got.QuestMode != "RequiresDuring" {
		t.Fatalf("quest requirement not parsed from notes: key=%q mode=%q", got.QuestKey, got.QuestMode)
	}
	if len(got.QuantityAlternatives) != 2 {
		t.Fatalf("quantity alternatives: got %d", len(got.QuantityAlternatives))
	}
	if got.QuantityAlternatives[0].Condition != nil {
		t.Fatalf("default alternative must carry no condition")
	}
	if got.QuantityAlternatives[1].Condition == nil || got.QuantityAlternatives[1].Condition.Type != "perfect_kill" {
		t.Fatalf("second alternative condition lost: %+v", got.QuantityAlternatives[1])
	}
	if len(got.RollCountWeights) != 1 || got.RollCountWeights[0] != (npcdrops.RollCount{Rolls: 2, Weight: 11}) {
		t.Fatalf("roll count weights: %+v", got.RollCountWeights)
	}
	if len(got.BonusDrops) != 1 || got.BonusDrops[0].ItemID != 21545 {
		t.Fatalf("bonus drops: %+v", got.BonusDrops)
	}
}

func TestConvertNpcDropEntryItemlessShapes(t *testing.T) {
	shared := convertNpcDropEntry(definition.NpcDropEntry{
		SharedTable: "herb", Weight: 2, OutOf: 128, Quantity: [2]int{1, 1},
	})
	if shared.ItemID != 0 || shared.SharedTable != "herb" || shared.Weight != 2 {
		t.Fatalf("shared-table row: %+v", shared)
	}
	nothing := convertNpcDropEntry(definition.NpcDropEntry{
		Nothing: true, Weight: 38, OutOf: 128,
	})
	if nothing.ItemID != 0 || !nothing.Nothing || nothing.Weight != 38 {
		t.Fatalf("nothing row: %+v", nothing)
	}
}

func TestConvertNpcDropTable(t *testing.T) {
	def := &definition.NpcDropTable{
		ID: "goblin_drop_table1", Label: "Drop table 1", MainMaxRoll: 128, Rolls: 1,
		Entries: []definition.NpcDropEntry{
			{ItemID: 526, HasItemID: true, Name: "Bones", Quantity: [2]int{1, 1}, Always: true},
			{ItemID: 995, HasItemID: true, Name: "Coins", Quantity: [2]int{15, 15}, Weight: 28, OutOf: 128},
		},
		Tertiary: []definition.NpcDropEntry{
			{ItemID: 23182, HasItemID: true, Name: "Clue scroll (beginner)", Quantity: [2]int{1, 1}, Weight: 1, OutOf: 64, ClueScrollBox: true},
		},
	}
	got := convertNpcDropTable(def)
	if got == nil || got.ID != "goblin_drop_table1" || got.MainMaxRoll != 128 {
		t.Fatalf("table header: %+v", got)
	}
	if len(got.Entries) != 2 || !got.Entries[0].Always || got.Entries[1].ItemID != 995 {
		t.Fatalf("entries: %+v", got.Entries)
	}
	if len(got.Tertiary) != 1 || !got.Tertiary[0].ClueScrollBox {
		t.Fatalf("tertiary: %+v", got.Tertiary)
	}
	if convertNpcDropTable(nil) != nil {
		t.Fatalf("nil table must convert to nil")
	}
	// Ordered chain is presence-only: the fail-closed flag survives.
	ordered := &definition.NpcDropTable{
		ID:          "duke_sucellus",
		RewardChain: &definition.NpcDropRewardChain{Type: "ordered_exclusive", StateDomain: "forgotten_four"},
	}
	converted := convertNpcDropTable(ordered)
	if converted.RewardChain == nil || converted.RewardChain.Type != "ordered_exclusive" || converted.RewardChain.StateDomain != "forgotten_four" {
		t.Fatalf("reward chain lost: %+v", converted.RewardChain)
	}
}
