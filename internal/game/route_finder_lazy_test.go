package game

import (
	"runtime"
	"testing"
)

// The RouteFinder holds ~164KB of search scratch. Allocating one
// eagerly per MovementQueue and per NPCMovementCoordinator cost
// ~337KB per NPC — ~10GB for the 30k spawns — and OOM-killed the
// server at boot (Phase 16 finding). Both owners allocate lazily
// on first use; these tests pin that, plus the SetClipping
// invalidation that keeps a stale-clipping finder from surviving
// a lookup change.

func TestMovementQueueFinderIsLazy(t *testing.T) {
	p := NewPlayer("alice", Location{X: 3000, Y: 3000})
	q := NewMovementQueue(p, nil, nil)
	if q.routeFinder != nil {
		t.Fatal("fresh queue must not allocate a RouteFinder")
	}
	q.SetClipping(func(x, y, z int) int32 { return 0 })
	if q.routeFinder != nil {
		t.Fatal("SetClipping must not allocate a RouteFinder")
	}
	first := q.finder()
	if first == nil {
		t.Fatal("finder() must allocate on first use")
	}
	if second := q.finder(); second != first {
		t.Fatal("finder() must reuse the allocated finder")
	}
	q.SetClipping(nil)
	if q.routeFinder != nil {
		t.Fatal("SetClipping must drop the finder so the next route rebuilds it")
	}
	if rebuilt := q.finder(); rebuilt == first {
		t.Fatal("finder() after SetClipping must build a fresh finder")
	}
}

func TestCoordinatorFinderIsLazy(t *testing.T) {
	n := NewNPC(1, Location{X: 3000, Y: 3000})
	c := n.MovementCoordinator()
	if c.routeFinder != nil {
		t.Fatal("fresh coordinator must not allocate a RouteFinder")
	}
	c.SetClipping(func(x, y, z int) int32 { return 0 })
	if c.routeFinder != nil {
		t.Fatal("SetClipping must not allocate a RouteFinder")
	}
	first := c.finder()
	if first == nil {
		t.Fatal("finder() must allocate on first use")
	}
	if second := c.finder(); second != first {
		t.Fatal("finder() must reuse the allocated finder")
	}
	c.SetClipping(nil)
	if c.routeFinder != nil {
		t.Fatal("SetClipping must drop the finder so the next route rebuilds it")
	}
}

func TestNPCRetainedSizeStaysSmall(t *testing.T) {
	// 5000 NPCs must retain far less than the ~1.6GB the eager
	// finders cost. The bound (16KB each) leaves 20x headroom
	// over the ~0.7KB observed, while catching any return of a
	// 164KB eager allocation.
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	const n = 5000
	npcs := make([]*NPC, n)
	for i := range npcs {
		npcs[i] = NewNPC(1, Location{X: 3000, Y: 3000, Z: 0})
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	sum := 0
	for _, npc := range npcs { // keep the slice live past the read
		sum += npc.ID
	}
	_ = sum
	perNPC := float64(after.HeapAlloc-before.HeapAlloc) / float64(n)
	if perNPC > 16*1024 {
		t.Fatalf("retained per NPC = %.1fKB, want <= 16KB", perNPC/1024)
	}
}
