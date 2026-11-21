package game

import (
	"fmt"
	"testing"
)

func TestMobileListLowestFreeFirst(t *testing.T) {
	l := NewMobileList[*Player](5)
	var got []int
	for i := 0; i < 5; i++ {
		p := NewPlayer(fmt.Sprintf("p%d", i), Location{})
		idx, ok := l.Add(p)
		if !ok {
			t.Fatalf("add %d failed", i)
		}
		got = append(got, idx)
		if p.Index != idx || !p.IsRegistered() {
			t.Fatal("Add must assign index and registration")
		}
	}
	// Slots start at 1, in order.
	for i, idx := range got {
		if idx != i+1 {
			t.Fatalf("indices = %v, want 1..5", got)
		}
	}
	if !l.IsFull() || l.Len() != 5 || l.SpaceLeft() != 0 {
		t.Fatal("list must be full at capacity")
	}
	if _, ok := l.Add(NewPlayer("extra", Location{})); ok {
		t.Fatal("add past capacity must fail")
	}
	// Free slots 2 and 4 (out of numeric order); next adds must reuse
	// the LOWEST free index first (2, then 4), not free order.
	if !l.Remove(4) || !l.Remove(2) {
		t.Fatal("removes must succeed")
	}
	idx, ok := l.Add(NewPlayer("r1", Location{}))
	if !ok || idx != 2 {
		t.Fatalf("first reuse = %d, want 2", idx)
	}
	idx, ok = l.Add(NewPlayer("r2", Location{}))
	if !ok || idx != 4 {
		t.Fatalf("second reuse = %d, want 4", idx)
	}
}

func TestMobileListRemoveAndGet(t *testing.T) {
	l := NewMobileList[*NPC](3)
	n := NewNPC(1, Location{})
	idx, _ := l.Add(n)
	if _, ok := l.Get(0); ok {
		t.Fatal("slot 0 must never be occupied")
	}
	if _, ok := l.Get(99); ok {
		t.Fatal("out-of-range Get must fail")
	}
	got, ok := l.Get(idx)
	if !ok || got != n || !l.Contains(idx) {
		t.Fatal("Get/Contains must find the occupant")
	}
	// Double-add of a registered entity must fail.
	if _, ok := l.Add(n); ok {
		t.Fatal("re-adding a registered entity must fail")
	}
	if !l.Remove(idx) {
		t.Fatal("Remove must succeed")
	}
	if n.IsRegistered() {
		t.Fatal("Remove must unregister")
	}
	if l.Remove(idx) {
		t.Fatal("double Remove must fail")
	}
	if l.Len() != 0 || l.IsFull() {
		t.Fatal("empty list state wrong")
	}
	// After removal the entity can be added again (fresh lowest index).
	if idx2, ok := l.Add(n); !ok || idx2 != 1 {
		t.Fatalf("re-add = %d,%v, want 1,true", idx2, ok)
	}
}

func TestMobileListAllInIndexOrder(t *testing.T) {
	l := NewMobileList[*Player](5)
	for i := 0; i < 5; i++ {
		l.Add(NewPlayer(fmt.Sprintf("p%d", i), Location{}))
	}
	l.Remove(3)
	all := l.All()
	if len(all) != 4 {
		t.Fatalf("All len = %d, want 4", len(all))
	}
	want := []int{1, 2, 4, 5}
	for i, p := range all {
		if p.Index != want[i] {
			t.Fatalf("All order indices = %v, want %v", indicesOf(all), want)
		}
	}
}

func indicesOf(ps []*Player) []int {
	out := make([]int, len(ps))
	for i, p := range ps {
		out[i] = p.Index
	}
	return out
}

func TestMobileListHooksAndPanicSafety(t *testing.T) {
	l := NewMobileList[*Player](2)
	var order []string
	l.OnAdded = func(p *Player) { order = append(order, "added") }
	l.OnRemoved = func(p *Player) { order = append(order, "removed") }
	p := NewPlayer("x", Location{})
	p.OnAddHook = func() { order = append(order, "onAdd") }
	p.OnRemoveHook = func() { order = append(order, "onRemove") }
	l.Add(p)
	if len(order) != 2 || order[0] != "onAdd" || order[1] != "added" {
		t.Fatalf("add hook order = %v, want [onAdd added]", order)
	}
	order = nil
	l.Remove(p.Index)
	if len(order) != 2 || order[0] != "removed" || order[1] != "onRemove" {
		t.Fatalf("remove hook order = %v, want [removed onRemove]", order)
	}

	// A panicking OnRemoved must still free the slot and decrement size
	// (MobileList.ts try/finally), with the panic propagating.
	l2 := NewMobileList[*Player](1)
	l2.OnRemoved = func(p *Player) { panic("hook boom") }
	q := NewPlayer("q", Location{})
	l2.Add(q)
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("Remove panic must propagate")
			}
		}()
		l2.Remove(q.Index)
	}()
	if l2.Len() != 0 || l2.Contains(q.Index) || q.IsRegistered() {
		t.Fatal("slot/size must be consistent after panicking remove hook")
	}
	if idx, ok := l2.Add(NewPlayer("z", Location{})); !ok || idx != 1 {
		t.Fatal("freed slot must be reusable after panicking remove hook")
	}
}

func TestMobileListZeroCapacity(t *testing.T) {
	l := NewMobileList[*Player](0)
	if !l.IsFull() {
		t.Fatal("zero-capacity list must be full")
	}
	if _, ok := l.Add(NewPlayer("x", Location{})); ok {
		t.Fatal("zero-capacity list must reject adds")
	}
}
