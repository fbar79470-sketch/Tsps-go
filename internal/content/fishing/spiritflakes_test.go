package fishing

import (
	"testing"
)

// SpiritFlakes tests: one carried flake per landed fish,
// always consumed (SpiritFlakes.Fishing.js). Fixtures only.

func TestConsumeFlake(t *testing.T) {
	f, _ := newTestFishing(t)
	flakes := f.ids.SpiritFlakes
	p := newTestPlayer("alice", 3000, 3000)
	p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{flakes: true}})
	if f.consumeFlake(p) {
		t.Fatal("consume with no flakes must return false")
	}
	p.Inventory().Adds(flakes, 5)
	if !f.consumeFlake(p) {
		t.Fatal("consume with flakes must return true")
	}
	if got := p.Inventory().GetAmount(flakes); got != 4 {
		t.Fatalf("flakes after one consume = %d, want 4", got)
	}
	// The last flake consumes and the next call fails.
	for i := 0; i < 4; i++ {
		if !f.consumeFlake(p) {
			t.Fatalf("consume %d must return true", i+2)
		}
	}
	if got := p.Inventory().GetAmount(flakes); got != 0 {
		t.Fatalf("flakes after five consumes = %d, want 0", got)
	}
	if f.consumeFlake(p) {
		t.Fatal("consume with no flakes left must return false")
	}
	if f.consumeFlake(nil) {
		t.Fatal("nil player consume must return false")
	}
	if (&fishing{}).consumeFlake(p) {
		t.Fatal("no-ids consume must return false")
	}
	if SpiritFlakeExtraFishChance != 0.5 {
		t.Fatalf("SpiritFlakeExtraFishChance = %v, want 0.5", SpiritFlakeExtraFishChance)
	}
}
