package game

import "testing"

func TestBoatDeckAreaConstruction(t *testing.T) {
	boat := testBoat()
	deck := NewBoatDeckArea(boat, testBoatSpec())
	if deck.Boat() != boat {
		t.Error("deck must reference its boat")
	}
	if !deck.CountsAsMainWorld() {
		t.Error("a deck must count as the main world")
	}
	// The spec's loc is an object on the deck, at level 0.
	objects := deck.GetObjects()
	if len(objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(objects))
	}
	obj := objects[0]
	if obj.ID() != 100 || obj.Location() != (Location{X: 9600, Y: 9600, Z: 0}) || obj.Type() != 10 {
		t.Errorf("deck object = id %d at %+v type %d", obj.ID(), obj.Location(), obj.Type())
	}
	// The boundary covers the 104x104 scene at level 0.
	boundaries := deck.Area.GetBoundaries()
	if len(boundaries) != 1 {
		t.Fatalf("boundaries = %d, want 1", len(boundaries))
	}
	boundary := boundaries[0]
	if !boundary.Inside(Location{X: 9552, Y: 9552, Z: 0}) || !boundary.Inside(Location{X: 9655, Y: 9655, Z: 0}) {
		t.Error("deck boundary must cover the scene")
	}
	if boundary.Inside(Location{X: 9551, Y: 9600, Z: 0}) {
		t.Error("deck boundary must not cover outside the scene")
	}
}

func TestBoatDeckAreaClips(t *testing.T) {
	boat := testBoat()
	deck := NewBoatDeckArea(boat, testBoatSpec())
	// Walkable, not solid: open.
	if clip := deck.GetClip(Location{X: 9601, Y: 9600, Z: 0}); clip != 0 {
		t.Errorf("walkable clip = %x, want 0", clip)
	}
	// Walkable and solid (the spec loc): the solid-object flag.
	if clip := deck.GetClip(Location{X: 9600, Y: 9600, Z: 0}); clip != solidObjectClip {
		t.Errorf("solid clip = %x, want %x", clip, solidObjectClip)
	}
	// Inside the scene but not walkable: blocked.
	if clip := deck.GetClip(Location{X: 9602, Y: 9602, Z: 0}); clip != ClipBlockedTile {
		t.Errorf("non-walkable clip = %x, want blocked", clip)
	}
	// Another level: blocked (everyone stands on level 0).
	if clip := deck.GetClip(Location{X: 9601, Y: 9600, Z: 1}); clip != ClipBlockedTile {
		t.Errorf("level-1 clip = %x, want blocked", clip)
	}
	// Outside the scene: no clip.
	if clip := deck.GetClip(Location{X: 3000, Y: 3000, Z: 0}); clip != 0 {
		t.Errorf("outside clip = %x, want 0", clip)
	}
	// HasClip: deck tiles, plus explicitly clipped tiles.
	if !deck.HasClip(Location{X: 9601, Y: 9600, Z: 0}) {
		t.Error("a deck tile must have a clip")
	}
	if deck.HasClip(Location{X: 3000, Y: 3000, Z: 0}) {
		t.Error("a main-world tile must not have a clip")
	}
	// An explicitly set clip wins over the deck rules.
	deck.SetClip(Location{X: 9601, Y: 9600, Z: 0}, 0x42)
	if clip := deck.GetClip(Location{X: 9601, Y: 9600, Z: 0}); clip != 0x42 {
		t.Errorf("explicit clip = %x, want 0x42", clip)
	}
	deck.RemoveClip(Location{X: 9601, Y: 9600, Z: 0})
	if clip := deck.GetClip(Location{X: 9601, Y: 9600, Z: 0}); clip != 0 {
		t.Errorf("removed clip = %x, want 0 (deck rule again)", clip)
	}
}

func TestBoatDeckAreaSetRemoveLoc(t *testing.T) {
	boat := testBoat()
	deck := NewBoatDeckArea(boat, testBoatSpec())
	// Swap the loc for another of the same shape: the old
	// object detaches, the solid flag follows the new loc.
	deck.SetLoc(BoatDeckLoc{ID: 200, X: 0, Y: 0, Level: 1, Shape: 10, Rotation: 1})
	objects := deck.GetObjects()
	if len(objects) != 1 || objects[0].ID() != 200 {
		t.Errorf("after set: objects = %d, want one id-200 object", len(objects))
	}
	if clip := deck.GetClip(Location{X: 9600, Y: 9600, Z: 0}); clip != 0 {
		t.Errorf("non-blocking replacement clip = %x, want 0", clip)
	}
	// A blocking loc of another shape adds a second object.
	deck.SetLoc(BoatDeckLoc{ID: 300, X: 0, Y: 0, Level: 1, Shape: 11, Rotation: 0, Blocks: true})
	if len(deck.GetObjects()) != 2 {
		t.Errorf("after second-shape set: objects = %d, want 2", len(deck.GetObjects()))
	}
	if clip := deck.GetClip(Location{X: 9600, Y: 9600, Z: 0}); clip != solidObjectClip {
		t.Errorf("blocking loc clip = %x, want solid", clip)
	}
	// Remove takes only its shape off, and clears the solid.
	deck.RemoveLoc(BoatDeckLoc{X: 0, Y: 0, Level: 1, Shape: 11})
	objects = deck.GetObjects()
	if len(objects) != 1 || objects[0].ID() != 200 {
		t.Errorf("after remove: %d objects, want the id-200 one", len(objects))
	}
	if clip := deck.GetClip(Location{X: 9600, Y: 9600, Z: 0}); clip != 0 {
		t.Errorf("after remove clip = %x, want 0", clip)
	}
}

func TestBoatDeckAreaLeaveDoesNotDestroy(t *testing.T) {
	boat := testBoat()
	deck := NewBoatDeckArea(boat, testBoatSpec())
	p := NewPlayer("aboard", Location{X: 9604, Y: 9608, Z: 0})
	deck.Enter(p)
	if p.Area() != deck.Area {
		t.Error("entering must set the player's area")
	}
	deck.Leave(p, false)
	if deck.Destroyed() {
		t.Error("a deck must outlive its last member leaving")
	}
	if p.Area() != nil {
		t.Error("leaving must clear the player's area")
	}
	if len(deck.GetObjects()) != 1 {
		t.Error("leaving must not remove deck objects")
	}
	// Destroy still works when the boat goes.
	deck.Destroy()
	if !deck.Destroyed() {
		t.Error("Destroy must mark the deck destroyed")
	}
}
