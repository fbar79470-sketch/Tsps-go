package fishing

import (
	"testing"

	"tsps-go/internal/game"
)

// Karambwan tests: ready / load / useBait, in exact TS
// order (Karambwan.Fishing.js). Fixtures only.

func TestKarambwanReady(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	p := newTestPlayer("alice", 3000, 3000)
	if f.karambwanReady(p) {
		t.Error("empty inventory must not be ready")
	}
	p.Inventory().Adds(ids.KarambwanVessel, 1)
	if f.karambwanReady(p) {
		t.Error("empty vessel alone must not be ready")
	}
	p.Inventory().Adds(ids.RawKarambwanji, 1)
	if !f.karambwanReady(p) {
		t.Error("empty vessel + karambwanji must be ready")
	}
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().Adds(ids.KarambwanVesselBaited, 1)
	if !f.karambwanReady(p2) {
		t.Error("baited vessel alone must be ready")
	}
	p3 := newTestPlayer("carol", 3000, 3000)
	p3.Inventory().Adds(ids.RawKarambwanji, 1)
	if f.karambwanReady(p3) {
		t.Error("bait alone must not be ready")
	}
	if f.karambwanReady(nil) {
		t.Error("nil player must not be ready")
	}
	if (&fishing{}).karambwanReady(p2) {
		t.Error("no-ids instance must not be ready")
	}
}

func TestKarambwanLoad(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	// Empty vessel + bait: the bait is consumed and the
	// vessel turns baited in its own slot.
	p := newTestPlayer("alice", 3000, 3000)
	p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{ids.RawKarambwanji: true}})
	p.Inventory().SetItem(2, game.NewItem(ids.KarambwanVessel, 1))
	p.Inventory().Adds(ids.RawKarambwanji, 3)
	if !f.karambwanLoad(p) {
		t.Fatal("load must succeed with an empty vessel and bait")
	}
	if got := p.Inventory().ItemAt(2).ID(); got != ids.KarambwanVesselBaited {
		t.Fatalf("vessel slot after load = %d, want the baited vessel", got)
	}
	if got := p.Inventory().GetAmount(ids.RawKarambwanji); got != 2 {
		t.Fatalf("karambwanji after load = %d, want 2", got)
	}
	// Already baited: true, nothing consumed.
	if !f.karambwanLoad(p) {
		t.Fatal("load with a baited vessel must report true")
	}
	if got := p.Inventory().GetAmount(ids.RawKarambwanji); got != 2 {
		t.Fatalf("karambwanji after no-op load = %d, want 2", got)
	}
	// Empty vessel, no bait: false, vessel untouched.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().Adds(ids.KarambwanVessel, 1)
	if f.karambwanLoad(p2) {
		t.Error("load with no bait must fail")
	}
	if got := p2.Inventory().ItemAt(0).ID(); got != ids.KarambwanVessel {
		t.Fatalf("vessel after failed load = %d, want the empty vessel", got)
	}
	// No vessel at all: false.
	p3 := newTestPlayer("carol", 3000, 3000)
	p3.Inventory().Adds(ids.RawKarambwanji, 1)
	if f.karambwanLoad(p3) {
		t.Error("load with no vessel must fail")
	}
	if f.karambwanLoad(nil) {
		t.Error("nil player load must fail")
	}
}

func TestKarambwanUseBait(t *testing.T) {
	f, _ := newTestFishing(t)
	ids := f.ids
	// Baited vessel + bait left: stays baited, one
	// karambwanji consumed per attempt — the bait consumed
	// is the one that reloads the vessel for the next
	// attempt, so with 2 bait the vessel stays baited
	// through two uses and empties on the third.
	p := newTestPlayer("alice", 3000, 3000)
	p.SetItemDefinitions(stackableDefs{stackable: map[int]bool{ids.RawKarambwanji: true}})
	p.Inventory().SetItem(2, game.NewItem(ids.KarambwanVesselBaited, 1))
	p.Inventory().Adds(ids.RawKarambwanji, 2)
	f.karambwanUseBait(p)
	if got := p.Inventory().ItemAt(2).ID(); got != ids.KarambwanVesselBaited {
		t.Fatalf("vessel after first use = %d, want baited", got)
	}
	if got := p.Inventory().GetAmount(ids.RawKarambwanji); got != 1 {
		t.Fatalf("karambwanji after first use = %d, want 1", got)
	}
	f.karambwanUseBait(p)
	if got := p.Inventory().ItemAt(2).ID(); got != ids.KarambwanVesselBaited {
		t.Fatalf("vessel after second use = %d, want baited (last bait reloaded it)", got)
	}
	if got := p.Inventory().GetAmount(ids.RawKarambwanji); got != 0 {
		t.Fatalf("karambwanji after second use = %d, want 0", got)
	}
	// No bait left: the vessel empties.
	f.karambwanUseBait(p)
	if got := p.Inventory().ItemAt(2).ID(); got != ids.KarambwanVessel {
		t.Fatalf("vessel after last bait = %d, want empty", got)
	}
	// No baited vessel: no-op, nothing changes.
	p2 := newTestPlayer("bob", 3000, 3000)
	p2.Inventory().Adds(ids.KarambwanVessel, 1)
	p2.Inventory().Adds(ids.RawKarambwanji, 5)
	f.karambwanUseBait(p2)
	if got := p2.Inventory().ItemAt(0).ID(); got != ids.KarambwanVessel {
		t.Fatalf("vessel after no-op use = %d, want empty", got)
	}
	if got := p2.Inventory().GetAmount(ids.RawKarambwanji); got != 5 {
		t.Fatalf("karambwanji after no-op use = %d, want 5", got)
	}
	f.karambwanUseBait(nil)
}
