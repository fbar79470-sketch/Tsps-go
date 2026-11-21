package persist

import (
	"encoding/json"
	"errors"
	"testing"
)

// escrowInv builds a 28-slot inventory from the given stacks,
// padding with empty slots.
func escrowInv(items ...Item) []Item {
	inv := make([]Item, 28)
	for i := range inv {
		inv[i] = EmptyItem()
	}
	copy(inv, items)
	return inv
}

func escrowStackable(ids ...int) func(int) bool {
	set := make(map[int]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(id int) bool { return set[id] }
}

func escrowMeta(t *testing.T, pairs ...string) map[string]json.RawMessage {
	t.Helper()
	if len(pairs)%2 != 0 {
		t.Fatal("meta pairs must be even")
	}
	m := make(map[string]json.RawMessage, len(pairs)/2)
	for i := 0; i < len(pairs); i += 2 {
		m[pairs[i]] = json.RawMessage(pairs[i+1])
	}
	return m
}

func TestFoldTradeEscrowEmpty(t *testing.T) {
	inv := escrowInv(Item{ID: 995, Amount: 10})
	got, err := FoldTradeEscrow(inv, nil, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(got) != 28 || got[0].Amount != 10 {
		t.Fatalf("folded = %v", got[:2])
	}
	// The input is never mutated, and the copy shares no meta.
	got[0].Amount = 999
	if inv[0].Amount != 10 {
		t.Fatal("fold mutated the input inventory")
	}
}

func TestFoldTradeEscrowStackableMerge(t *testing.T) {
	// A full inventory still takes a stackable offer onto its
	// matching stack: 0 slots needed.
	inv := escrowInv()
	for i := range inv {
		inv[i] = Item{ID: 1000 + i, Amount: 1}
	}
	inv[0] = Item{ID: 995, Amount: 100}
	got, err := FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 50}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != 150 {
		t.Fatalf("merged stack = %d, want 150", got[0].Amount)
	}
}

func TestFoldTradeEscrowStackableNewStack(t *testing.T) {
	inv := escrowInv(Item{ID: 995, Amount: 100})
	got, err := FoldTradeEscrow(inv, []Item{{ID: 996, Amount: 7}}, escrowStackable(995, 996))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[1].ID != 996 || got[1].Amount != 7 || got[1].Meta != nil {
		t.Fatalf("new stack = %+v", got[1])
	}
}

func TestFoldTradeEscrowStackableFull(t *testing.T) {
	inv := escrowInv()
	for i := range inv {
		inv[i] = Item{ID: 1000 + i, Amount: 1}
	}
	_, err := FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 1}}, escrowStackable(995))
	if !errors.Is(err, ErrTradeEscrowFull) {
		t.Fatalf("err = %v, want ErrTradeEscrowFull", err)
	}
}

func TestFoldTradeEscrowMetaDistinguishesStacks(t *testing.T) {
	charged := escrowMeta(t, "charge", "5")
	inv := escrowInv(Item{ID: 995, Amount: 100, Meta: charged})
	// Same id, different metadata: a new stack, needing a slot.
	got, err := FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 10}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != 100 || got[1].Amount != 10 {
		t.Fatalf("stacks = %v/%v", got[0], got[1])
	}
	// Same id, same metadata: merges, needing no slot — even
	// when the metadata keys arrive in a different order.
	inv = escrowInv(Item{ID: 995, Amount: 100, Meta: escrowMeta(t, "a", "1", "b", "2")})
	for i := 1; i < 28; i++ {
		inv[i] = Item{ID: 1000 + i, Amount: 1}
	}
	got, err = FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 10, Meta: escrowMeta(t, "b", "2", "a", "1")}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != 110 {
		t.Fatalf("merged = %d, want 110", got[0].Amount)
	}
	// Nil metadata and empty metadata are distinct (null vs {}).
	inv = escrowInv(Item{ID: 995, Amount: 100})
	got, err = FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 10, Meta: map[string]json.RawMessage{}}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != 100 || got[1].Amount != 10 {
		t.Fatalf("nil vs empty meta must not merge: %v/%v", got[0], got[1])
	}
}

func TestFoldTradeEscrowUnstackableFanOut(t *testing.T) {
	meta := escrowMeta(t, "charge", "3")
	inv := escrowInv()
	got, err := FoldTradeEscrow(inv, []Item{{ID: 1, Amount: 3, Meta: meta}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	for i := 0; i < 3; i++ {
		if got[i].ID != 1 || got[i].Amount != 1 || !escrowMetaEqual(got[i].Meta, meta) {
			t.Fatalf("slot %d = %+v, want one sword carrying the meta", i, got[i])
		}
	}
	if got[3].ID != -1 {
		t.Fatalf("slot 3 = %+v, want empty", got[3])
	}
}

func TestFoldTradeEscrowUnstackableFull(t *testing.T) {
	inv := escrowInv()
	for i := 0; i < 27; i++ {
		inv[i] = Item{ID: 1000 + i, Amount: 1}
	}
	// One free slot, two units needed.
	_, err := FoldTradeEscrow(inv, []Item{{ID: 1, Amount: 2}}, escrowStackable(995))
	if !errors.Is(err, ErrTradeEscrowFull) {
		t.Fatalf("err = %v, want ErrTradeEscrowFull", err)
	}
	// One unit fits.
	got, err := FoldTradeEscrow(inv, []Item{{ID: 1, Amount: 1}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[27].ID != 1 || got[27].Amount != 1 {
		t.Fatalf("last slot = %+v", got[27])
	}
}

func TestFoldTradeEscrowCapClamp(t *testing.T) {
	// A merge past the cap clamps, dropping the excess: no error.
	inv := escrowInv(Item{ID: 995, Amount: maxStackAmount - 5})
	got, err := FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 10}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != maxStackAmount {
		t.Fatalf("merged = %d, want the cap", got[0].Amount)
	}
	// A fresh stack over the cap clamps too.
	inv = escrowInv()
	got, err = FoldTradeEscrow(inv, []Item{{ID: 995, Amount: maxStackAmount + 100}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != maxStackAmount {
		t.Fatalf("fresh = %d, want the cap", got[0].Amount)
	}
}

func TestFoldTradeEscrowSequentialPreCheck(t *testing.T) {
	// The pre-check runs per item against the evolving copy:
	// the first offer takes the last slot, the second fails.
	inv := escrowInv()
	for i := 0; i < 27; i++ {
		inv[i] = Item{ID: 1000 + i, Amount: 1}
	}
	_, err := FoldTradeEscrow(inv, []Item{{ID: 1, Amount: 1}, {ID: 2, Amount: 1}}, escrowStackable(995))
	if !errors.Is(err, ErrTradeEscrowFull) {
		t.Fatalf("err = %v, want ErrTradeEscrowFull", err)
	}
}

func TestFoldTradeEscrowSkipsInvalid(t *testing.T) {
	inv := escrowInv()
	got, err := FoldTradeEscrow(inv, []Item{{ID: -1, Amount: 5}, {ID: 995, Amount: 0}, {ID: 995, Amount: 3}}, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].ID != 995 || got[0].Amount != 3 {
		t.Fatalf("folded = %+v", got[0])
	}
}

func TestFoldTradeEscrowNilStackable(t *testing.T) {
	// A nil stackability read treats every id as unstackable.
	inv := escrowInv(Item{ID: 995, Amount: 100})
	got, err := FoldTradeEscrow(inv, []Item{{ID: 995, Amount: 2}}, nil)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if got[0].Amount != 100 || got[1].Amount != 1 || got[2].Amount != 1 {
		t.Fatalf("folded = %v/%v/%v", got[0], got[1], got[2])
	}
}

func TestFoldTradeEscrowDoesNotAliasMeta(t *testing.T) {
	meta := escrowMeta(t, "charge", "5")
	inv := escrowInv()
	offered := []Item{{ID: 1, Amount: 1, Meta: meta}}
	got, err := FoldTradeEscrow(inv, offered, escrowStackable(995))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got[0].Meta["charge"] = json.RawMessage("9")
	if string(offered[0].Meta["charge"]) != "5" {
		t.Fatal("fold aliased the offered item's meta")
	}
}
