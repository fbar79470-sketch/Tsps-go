package game

import (
	"sync"
	"testing"
)

func TestAttributesAbsentVsPresent(t *testing.T) {
	m := NewMobile(Location{X: 1, Y: 2, Z: 0})
	// Absent: GetAttribute returns nil (TS undefined) and
	// HasAttribute is false, before anything is ever stored.
	if got := m.GetAttribute("never:set"); got != nil {
		t.Errorf("GetAttribute(absent) = %v, want nil", got)
	}
	if m.HasAttribute("never:set") {
		t.Error("HasAttribute(absent) = true, want false")
	}
	// Falsy values are present: false, 0, and "" are stored
	// values, distinct from absent (TS stores them in the Map
	// and getAttribute returns them).
	falsy := map[string]any{"bool": false, "int": 0, "string": ""}
	for key, value := range falsy {
		m.SetAttribute(key, value)
		if got := m.GetAttribute(key); got != value {
			t.Errorf("GetAttribute(%q) = %#v, want %#v", key, got, value)
		}
		if !m.HasAttribute(key) {
			t.Errorf("HasAttribute(%q) = false, want true", key)
		}
	}
}

func TestAttributesSetGetOverwrite(t *testing.T) {
	m := NewMobile(Location{})
	m.SetAttribute("k", "one")
	if got := m.GetAttribute("k"); got != "one" {
		t.Fatalf("GetAttribute = %v, want one", got)
	}
	// Overwrite replaces the value; the key stays present.
	m.SetAttribute("k", "two")
	if got := m.GetAttribute("k"); got != "two" {
		t.Errorf("GetAttribute after overwrite = %v, want two", got)
	}
	if !m.HasAttribute("k") {
		t.Error("HasAttribute after overwrite = false, want true")
	}
	// Other keys are unaffected.
	m.SetAttribute("other", 42)
	if got := m.GetAttribute("k"); got != "two" {
		t.Errorf("GetAttribute(k) after other set = %v, want two", got)
	}
	if got := m.GetAttribute("other"); got != 42 {
		t.Errorf("GetAttribute(other) = %v, want 42", got)
	}
}

func TestAttributesNilDeletes(t *testing.T) {
	m := NewMobile(Location{})
	m.SetAttribute("k", "value")
	// A nil value deletes: the observable Go form of TS
	// setAttribute(key, undefined) (see attributes.go).
	m.SetAttribute("k", nil)
	if got := m.GetAttribute("k"); got != nil {
		t.Errorf("GetAttribute after nil set = %v, want nil", got)
	}
	if m.HasAttribute("k") {
		t.Error("HasAttribute after nil set = true, want false")
	}
	// Deleting an absent key is a no-op, not a panic.
	m.SetAttribute("absent", nil)
	if m.HasAttribute("absent") {
		t.Error("HasAttribute(absent) after nil set = true, want false")
	}
	// A deleted key can be set again.
	m.SetAttribute("k", "back")
	if got := m.GetAttribute("k"); got != "back" {
		t.Errorf("GetAttribute after re-set = %v, want back", got)
	}
}

func TestAttributesLazyAllocation(t *testing.T) {
	// A zero Mobile works: no store is allocated until the first
	// storing SetAttribute, and reads report absent (TS: the map
	// starts null and getAttribute returns undefined).
	var m Mobile
	if got := m.GetAttribute("k"); got != nil {
		t.Errorf("zero Mobile GetAttribute = %v, want nil", got)
	}
	if m.HasAttribute("k") {
		t.Error("zero Mobile HasAttribute = true, want false")
	}
	if m.attributes != nil {
		t.Error("zero Mobile allocated a store on read")
	}
	m.SetAttribute("k", nil)
	if m.attributes != nil {
		t.Error("nil-valued SetAttribute allocated a store")
	}
	m.SetAttribute("k", 1)
	if m.attributes == nil {
		t.Fatal("storing SetAttribute did not allocate a store")
	}
	if got := m.GetAttribute("k"); got != 1 {
		t.Errorf("GetAttribute after lazy set = %v, want 1", got)
	}
}

func TestAttributesStoredByReference(t *testing.T) {
	m := NewMobile(Location{})
	// TS stores the reference; mutating a stored map through the
	// returned value mutates the stored value.
	stored := map[string]int{"n": 1}
	m.SetAttribute("map", stored)
	stored["n"] = 2
	if got := m.GetAttribute("map").(map[string]int); got["n"] != 2 {
		t.Errorf("stored map n = %d, want 2 (reference semantics)", got["n"])
	}
	got := m.GetAttribute("map").(map[string]int)
	got["n"] = 3
	if stored["n"] != 3 {
		t.Errorf("original map n = %d, want 3 (shared reference)", stored["n"])
	}
	// Same for slices: an element write through the returned
	// slice is visible on the next read.
	slice := []int{1, 2, 3}
	m.SetAttribute("slice", slice)
	m.GetAttribute("slice").([]int)[0] = 9
	if slice[0] != 9 {
		t.Errorf("original slice[0] = %d, want 9", slice[0])
	}
}

func TestAttributesPlayerAndNPCInheritance(t *testing.T) {
	// Player and NPC inherit the store through the embedded
	// Mobile, as in TS (Mobile.ts:102-110 is the home of the map).
	p := NewPlayer("testplayer", Location{X: 3089, Y: 3524, Z: 0})
	p.SetAttribute("player:key", "p")
	if got := p.GetAttribute("player:key"); got != "p" {
		t.Errorf("player GetAttribute = %v, want p", got)
	}
	if !p.HasAttribute("player:key") {
		t.Error("player HasAttribute = false, want true")
	}
	n := NewNPC(1, Location{X: 3089, Y: 3524, Z: 0})
	n.SetAttribute("npc:key", 7)
	if got := n.GetAttribute("npc:key"); got != 7 {
		t.Errorf("npc GetAttribute = %v, want 7", got)
	}
	if !n.HasAttribute("npc:key") {
		t.Error("npc HasAttribute = false, want true")
	}
	// Stores are per-entity: the NPC does not see the player's
	// key and vice versa.
	if n.HasAttribute("player:key") || p.HasAttribute("npc:key") {
		t.Error("attribute stores leak across entities")
	}
	// Nil delete works through the embedding too.
	p.SetAttribute("player:key", nil)
	if p.HasAttribute("player:key") {
		t.Error("player HasAttribute after delete = true, want false")
	}
}

func TestAttributesNilReceiver(t *testing.T) {
	// Nil-receiver safety (see attributes.go): reads report
	// absent, writes are no-ops, nothing panics. This covers a
	// nil *Mobile only: calling a promoted method through a nil
	// *Player or *NPC panics at the promotion site before the
	// method body runs (Go evaluates the embedded field's
	// address), so callers must nil-check the entity itself,
	// as the listeners' playerReady does.
	var m *Mobile
	if got := m.GetAttribute("k"); got != nil {
		t.Errorf("nil GetAttribute = %v, want nil", got)
	}
	if m.HasAttribute("k") {
		t.Error("nil HasAttribute = true, want false")
	}
	m.SetAttribute("k", 1)
	m.SetAttribute("k", nil)
}

func TestAttributesConcurrentAccess(t *testing.T) {
	// Run with -race: concurrent readers and writers on one
	// store must not race (login goroutine vs tick goroutine).
	m := NewMobile(Location{})
	m.SetAttribute("seed", 0)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			key := "key"
			for i := 0; i < 200; i++ {
				m.SetAttribute(key, i)
				_ = m.GetAttribute(key)
				_ = m.HasAttribute(key)
				m.SetAttribute(key, nil)
				_ = m.GetAttribute(key)
			}
			m.SetAttribute("per-goroutine", g)
		}(g)
	}
	wg.Wait()
	if !m.HasAttribute("per-goroutine") {
		t.Error("per-goroutine key missing after concurrent writes")
	}
}

func TestAttributesConcurrentFirstSet(t *testing.T) {
	// Run with -race: concurrent FIRST sets on a fresh Mobile
	// exercise the lazy-allocation serialization; no write may
	// be lost to a torn pointer, and every key set must read
	// back. Each goroutine owns its key, so the final values
	// are deterministic.
	var m Mobile
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			key := string(rune('a' + g))
			for i := 0; i < 100; i++ {
				m.SetAttribute(key, i)
				_ = m.GetAttribute(key)
			}
		}(g)
	}
	wg.Wait()
	for g := 0; g < 8; g++ {
		key := string(rune('a' + g))
		if got := m.GetAttribute(key); got != 99 {
			t.Errorf("GetAttribute(%q) = %v, want 99", key, got)
		}
	}
}
