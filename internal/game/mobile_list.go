package game

import (
	"container/heap"
	"reflect"
	"sync"
)

// intMinHeap is a binary min-heap of free slot indices, the Go equivalent
// of the SlotMinHeap in game/entity/impl/MobileList.ts. It exists so slot
// assignment is lowest-free-first: a plain FIFO free-list would reuse freed
// slots in free order instead of numeric order (MobileList.ts header
// comment).
type intMinHeap []int

func (h intMinHeap) Len() int           { return len(h) }
func (h intMinHeap) Less(i, j int) bool { return h[i] < h[j] }
func (h intMinHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *intMinHeap) Push(x any)        { *h = append(*h, x.(int)) }
func (h *intMinHeap) Pop() any {
	old := *h
	n := len(old)
	v := old[n-1]
	*h = old[:n-1]
	return v
}

// MobileList ports game/entity/impl/MobileList.ts: a fixed-capacity slot
// array with lowest-free-first index allocation from a min-heap of free
// indices, matching OSRS index assignment.
//
// Indexing: slots run 1..capacity; slot 0 is never assigned
// (MobileList.ts constructor pushes 1..capacity onto the heap). The TS
// constructor stores capacity+1 internally and reports isFull as
// size+1 >= capacity; the Go list keeps the usable capacity and reports
// IsFull as Len() >= capacity, which is the same boundary.
//
// The list is safe for concurrent use: an internal RWMutex guards the
// slots, free heap, and size, because the network goroutines read it
// (IsFull, Get, Search via World) while the tick goroutine adds and
// removes. The lock is never held across user callbacks: Add/Remove
// mutate state under the write lock, release it, and only then run
// OnAdd/OnAdded and OnRemoved/OnRemove, so callbacks may safely call
// back into the list (e.g. to queue a removal). ForEach/Search
// snapshot the occupants under the read lock and iterate the snapshot
// without holding it. World queues remain the only path by which other
// goroutines ADD or REMOVE occupants; the lock exists to make the
// reads safe and the queue drains race-free, not to invite direct
// cross-goroutine mutation.
type MobileList[T IndexedMobile] struct {
	mu       sync.RWMutex
	slots    []T // index 0 unused; slots[i] holds the occupant of slot i.
	free     intMinHeap
	size     int
	capacity int

	// OnAdded runs after a successful Add, mirroring the onAdded
	// constructor callback (MobileList.ts: add() calls this.onAdded).
	OnAdded func(v T)
	// OnRemoved runs during Remove, mirroring the onRemoved constructor
	// callback. If it panics, the slot is still freed and the size is
	// still decremented (MobileList.ts removes()/remove() use
	// try/finally for exactly this), and the panic propagates.
	OnRemoved func(v T)
}

// NewMobileList returns an empty list holding at most capacity elements in
// slots 1..capacity. A non-positive capacity yields a list that is always
// full (TS MobileList(0) is used by World as a dummy instance field).
func NewMobileList[T IndexedMobile](capacity int) *MobileList[T] {
	if capacity < 0 {
		capacity = 0
	}
	m := &MobileList[T]{
		slots:    make([]T, capacity+1),
		capacity: capacity,
	}
	for i := 1; i <= capacity; i++ {
		m.free = append(m.free, i)
	}
	heap.Init(&m.free)
	return m
}

// Add assigns the lowest free index to v and stores it, returning the
// index. It reports ok=false when the list is full or v is already
// registered (MobileList.ts: add() returns false in both cases; a null
// element is rejected in TS — the Go equivalent is a nil interface/pointer,
// which Add also rejects).
//
// On success, v is marked registered, given its index, and its OnAdd hook
// runs before OnAdded, matching the TS call order
// (e.onAdd() then this.onAdded).
func (m *MobileList[T]) Add(v T) (index int, ok bool) {
	if isNilMobile(v) {
		return 0, false
	}
	m.mu.Lock()
	// Inline the IsFull check: IsFull takes the read lock, which must
	// not be acquired while the write lock is held.
	if m.size >= m.capacity || v.IsRegistered() || len(m.free) == 0 {
		m.mu.Unlock()
		return 0, false
	}
	index = heap.Pop(&m.free).(int)
	v.SetRegistered(true)
	v.SetIndex(index)
	m.slots[index] = v
	m.size++
	onAdded := m.OnAdded
	m.mu.Unlock()
	// Hooks run without the lock held, in TS order (OnAdd, then
	// OnAdded), after the index is assigned. A concurrent Remove may
	// interleave here; callbacks must tolerate that.
	v.OnAdd()
	if onAdded != nil {
		onAdded(v)
	}
	return index, true
}

// Remove frees slot index, returning false when the slot is empty or out
// of range. The occupant is unregistered, its slot cleared, and the index
// pushed back onto the free heap so the next Add reuses the lowest free
// index. OnRemoved runs before the occupant's OnRemove hook, matching TS
// order (this.onRemoved then e.onRemove in removes()/remove()). The size
// is decremented even if a hook panics.
func (m *MobileList[T]) Remove(index int) (removed bool) {
	m.mu.Lock()
	if index < 1 || index > m.capacity {
		m.mu.Unlock()
		return false
	}
	v := m.slots[index]
	if isNilMobile(v) {
		m.mu.Unlock()
		return false
	}
	v.SetRegistered(false)
	m.slots[index] = *new(T)
	heap.Push(&m.free, index)
	m.size--
	onRemoved := m.OnRemoved
	m.mu.Unlock()
	// Hooks run without the lock held, in TS order (OnRemoved, then
	// OnRemove). The size is already decremented, so a panicking hook
	// cannot leave the count wrong (TS uses try/finally for the same
	// guarantee).
	if onRemoved != nil {
		onRemoved(v)
	}
	v.OnRemove()
	return true
}

// Get returns the occupant of slot index, or ok=false when the slot is
// empty or out of range. (TS get() returns null for empty slots.)
func (m *MobileList[T]) Get(index int) (v T, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if index < 1 || index > m.capacity {
		return *new(T), false
	}
	v = m.slots[index]
	if isNilMobile(v) {
		return *new(T), false
	}
	return v, true
}

// Contains reports whether slot index is occupied.
func (m *MobileList[T]) Contains(index int) bool {
	_, ok := m.Get(index)
	return ok
}

// Len returns the number of occupants. MobileList.ts: sizeReturn().
func (m *MobileList[T]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size
}

// Capacity returns the usable capacity (slots 1..capacity).
// Capacity is immutable after construction, so it needs no lock.
func (m *MobileList[T]) Capacity() int { return m.capacity }

// SpaceLeft returns the number of free slots.
// MobileList.ts: spaceLeft() (adjusted for the +1 internal capacity).
func (m *MobileList[T]) SpaceLeft() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.capacity - m.size
}

// IsFull reports whether no free slot remains.
// MobileList.ts: isFull() (size+1 >= capacity+1, i.e. size >= capacity).
func (m *MobileList[T]) IsFull() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.size >= m.capacity
}

// All returns the occupants in index order, skipping empty slots.
// Iteration order in TS is slot order (MobileList.ts: forEach/iterator
// scan the characters array); processing order is maintained separately
// by World and does not derive from this order.
func (m *MobileList[T]) All() []T {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]T, 0, m.size)
	for i := 1; i <= m.capacity; i++ {
		if v := m.slots[i]; !isNilMobile(v) {
			out = append(out, v)
		}
	}
	return out
}

// ForEach calls fn for each occupant in index order. (MobileList.ts:
// forEach.) The occupants are snapshotted under the read lock and fn
// runs without it held, so fn may safely call back into the list;
// occupants added or removed during iteration are not visited.
func (m *MobileList[T]) ForEach(fn func(v T)) {
	for _, v := range m.All() {
		fn(v)
	}
}

// Search returns the first occupant in index order matching filter, or
// ok=false. MobileList.ts: search(). The occupants are snapshotted
// under the read lock and the filter runs without it held, so the
// filter may safely call back into the list.
func (m *MobileList[T]) Search(filter func(v T) bool) (T, bool) {
	for _, v := range m.All() {
		if filter(v) {
			return v, true
		}
	}
	return *new(T), false
}

// isNilMobile reports whether v is a nil interface value or a typed nil
// pointer. IndexedMobile is implemented by pointer types in this package
// (*Player, *NPC), so a plain nil comparison against the interface would
// miss typed nils.
func isNilMobile[T IndexedMobile](v T) bool {
	if any(v) == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

// NOTE: MobileList.ts clear() is deliberately NOT ported: it calls
// this.characters.forEach(this.remove) with an unbound method, so the TS
// implementation itself is broken. A correct Clear can be added by the
// milestone that needs one.
