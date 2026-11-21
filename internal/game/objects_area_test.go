package game

import "testing"

// Deregister undoes the register-time AttachObject: a
// deregistered instance object no longer answers area lookups
// (ObjectManager.ts:324,328). The constructor attach is pinned
// here too, so the detach assertion is meaningful.

func TestDeregisterDetachesAreaObject(t *testing.T) {
	loc := Location{X: 3210, Y: 3210, Z: 0}
	pa := NewPrivateArea([]Boundary{NewBoundary(3200, 3263, 3200, 3263, 0)})
	obj := NewGameObject(100, loc, 10, 0, pa)
	if got := pa.GetObjects(); len(got) != 1 || got[0] != obj {
		t.Fatalf("after NewGameObject: area objects = %v, want [obj]", objectIDs(got))
	}

	r := NewObjectRegistry()
	r.Register(obj, true)
	if got := r.At(loc); len(got) != 1 {
		t.Fatalf("after Register: At = %d objects, want 1", len(got))
	}

	r.Deregister(obj, true)
	if got := pa.GetObjects(); len(got) != 0 {
		t.Fatalf("after Deregister: area objects = %v, want empty", objectIDs(got))
	}
	if got := r.At(loc); len(got) != 0 {
		t.Fatalf("after Deregister: At = %d objects, want 0", len(got))
	}

	// Deregistering again is a no-op, not a panic: the detach
	// is idempotent.
	r.Deregister(obj, true)
	if got := pa.GetObjects(); len(got) != 0 {
		t.Fatalf("after second Deregister: area objects = %v, want empty", objectIDs(got))
	}
}

func TestDeregisterQuietDetachesAreaObject(t *testing.T) {
	loc := Location{X: 3211, Y: 3210, Z: 0}
	pa := NewPrivateArea([]Boundary{NewBoundary(3200, 3263, 3200, 3263, 0)})
	obj := NewGameObject(101, loc, 10, 0, pa)

	r := NewObjectRegistry()
	r.Register(obj, false)
	r.Deregister(obj, false)
	if got := pa.GetObjects(); len(got) != 0 {
		t.Fatalf("after quiet Deregister: area objects = %v, want empty", objectIDs(got))
	}
}
