package session

import (
	"testing"

	"tsps-go/internal/persist"
)

// fakeWeights is an ItemWeightSource over a fixed id -> grams map.
// A missing id returns 0, as the production source does for unknown
// definitions; stackable and noted ids are represented by explicit
// 0 entries, mirroring the production mapping.
type fakeWeights map[int]int

func (f fakeWeights) ItemWeightGrams(id int) int { return f[id] }

func weightSave(equipment, inventory []persist.Item) *persist.PlayerSave {
	return &persist.PlayerSave{Equipment: equipment, Inventory: inventory}
}

func TestCarriedWeightKg(t *testing.T) {
	// Ids: 1 weighs 2500g, 2 weighs 1500g, 3 is stackable (0),
	// 4 is noted (0), 5 carries a graceful-style negative weight.
	weights := fakeWeights{1: 2500, 2: 1500, 3: 0, 4: 0, 5: -2000}

	t.Run("nil source sends 0", func(t *testing.T) {
		save := weightSave(
			[]persist.Item{{ID: 1, Amount: 1}},
			[]persist.Item{{ID: 2, Amount: 1}},
		)
		if got := carriedWeightKg(nil, save); got != 0 {
			t.Errorf("carriedWeightKg(nil, save) = %d, want 0", got)
		}
	})

	t.Run("nil save sends 0", func(t *testing.T) {
		if got := carriedWeightKg(weights, nil); got != 0 {
			t.Errorf("carriedWeightKg(weights, nil) = %d, want 0", got)
		}
	})

	t.Run("multi-slot sum truncates toward zero", func(t *testing.T) {
		// 2500 + 1500 = 4000g = 4kg exactly.
		save := weightSave(
			[]persist.Item{{ID: 1, Amount: 1}},
			[]persist.Item{{ID: 2, Amount: 1}},
		)
		if got := carriedWeightKg(weights, save); got != 4 {
			t.Errorf("carriedWeightKg = %d, want 4", got)
		}
		// 2500g alone truncates to 2kg (the fraction is dropped).
		save = weightSave(nil, []persist.Item{{ID: 1, Amount: 1}})
		if got := carriedWeightKg(weights, save); got != 2 {
			t.Errorf("carriedWeightKg = %d, want 2", got)
		}
	})

	t.Run("amount counts once per slot", func(t *testing.T) {
		save := weightSave(nil, []persist.Item{{ID: 1, Amount: 100}})
		if got := carriedWeightKg(weights, save); got != 2 {
			t.Errorf("carriedWeightKg = %d, want 2 (amount ignored)", got)
		}
	})

	t.Run("stackable and noted contribute 0", func(t *testing.T) {
		save := weightSave(
			[]persist.Item{{ID: 3, Amount: 1000}},
			[]persist.Item{{ID: 4, Amount: 1000}},
		)
		if got := carriedWeightKg(weights, save); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0", got)
		}
	})

	t.Run("unknown id contributes 0", func(t *testing.T) {
		save := weightSave(nil, []persist.Item{{ID: 999, Amount: 1}})
		if got := carriedWeightKg(weights, save); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0", got)
		}
	})

	t.Run("empty and zero-amount slots contribute 0", func(t *testing.T) {
		save := weightSave(
			[]persist.Item{{ID: -1, Amount: 0}, {ID: 1, Amount: 0}},
			[]persist.Item{{ID: -1, Amount: 5}, {ID: 2, Amount: 0}},
		)
		if got := carriedWeightKg(weights, save); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0", got)
		}
	})

	t.Run("negative weights sum and truncate toward zero", func(t *testing.T) {
		// -2000g = -2kg exactly.
		save := weightSave([]persist.Item{{ID: 5, Amount: 1}}, nil)
		if got := carriedWeightKg(weights, save); got != -2 {
			t.Errorf("carriedWeightKg = %d, want -2", got)
		}
		// 2500 + (-2000) = 500g -> 0kg (Math.trunc(0.5)).
		save = weightSave(
			[]persist.Item{{ID: 5, Amount: 1}},
			[]persist.Item{{ID: 1, Amount: 1}},
		)
		if got := carriedWeightKg(weights, save); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0", got)
		}
		// -2000 + 1500 = -500g -> 0kg: Math.trunc(-0.5) is 0, not -1.
		save = weightSave(
			[]persist.Item{{ID: 5, Amount: 1}},
			[]persist.Item{{ID: 2, Amount: 1}},
		)
		if got := carriedWeightKg(weights, save); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0 (trunc toward zero)", got)
		}
		// -2000 + -2000 + 2500 = -1500g -> -1kg: Math.trunc(-1.5).
		save = weightSave(
			[]persist.Item{{ID: 5, Amount: 1}, {ID: 5, Amount: 1}},
			[]persist.Item{{ID: 1, Amount: 1}},
		)
		if got := carriedWeightKg(weights, save); got != -1 {
			t.Errorf("carriedWeightKg = %d, want -1 (trunc toward zero)", got)
		}
	})

	t.Run("slots past container capacity are ignored", func(t *testing.T) {
		equipment := make([]persist.Item, persist.EquipmentSize+2)
		for i := range equipment {
			equipment[i] = persist.Item{ID: -1}
		}
		equipment[persist.EquipmentSize] = persist.Item{ID: 1, Amount: 1}
		inventory := make([]persist.Item, persist.InventorySize+2)
		for i := range inventory {
			inventory[i] = persist.Item{ID: -1}
		}
		inventory[persist.InventorySize] = persist.Item{ID: 1, Amount: 1}
		if got := carriedWeightKg(weights, weightSave(equipment, inventory)); got != 0 {
			t.Errorf("carriedWeightKg = %d, want 0 (over-capacity slots ignored)", got)
		}
	})
}

func TestSlotGramsSkipsDefinitionsForEmptySlots(t *testing.T) {
	// A source that fails the test if consulted: empty slots must
	// never reach it (Player.ts:1291 guards before forId).
	queried := false
	source := ItemWeightSourceFunc(func(id int) int {
		queried = true
		return 1000
	})
	if got := slotGrams(source, persist.Item{ID: -1, Amount: 1}); got != 0 {
		t.Errorf("slotGrams(empty) = %d, want 0", got)
	}
	if got := slotGrams(source, persist.Item{ID: 1, Amount: 0}); got != 0 {
		t.Errorf("slotGrams(zero amount) = %d, want 0", got)
	}
	if queried {
		t.Error("empty slots consulted the weight source")
	}
	if got := slotGrams(source, persist.Item{ID: 1, Amount: 3}); got != 1000 {
		t.Errorf("slotGrams(occupied) = %d, want 1000", got)
	}
}

// ItemWeightSourceFunc adapts a function to ItemWeightSource for tests.
type ItemWeightSourceFunc func(id int) int

func (f ItemWeightSourceFunc) ItemWeightGrams(id int) int { return f(id) }
