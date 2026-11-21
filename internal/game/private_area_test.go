package game

import "testing"

// privateAreaBounds is a 10x10 area at the origin on level 0.
func privateAreaBounds() []Boundary {
	return []Boundary{NewBoundary(0, 10, 0, 10, 0)}
}

func TestPrivateAreaAddSetsAreaAndTracksMembers(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	n := NewNPC(1, Location{X: 5, Y: 5, Z: 0})
	n.SetIndex(2)

	pa.Add(p)
	if p.Area() != pa.Area {
		t.Fatal("Add did not set the player's Area to the embedded *Area")
	}
	if p.PrivateArea() != pa {
		t.Fatal("Mobile.PrivateArea did not resolve the joined area")
	}
	pa.Add(n)
	if n.Area() != pa.Area {
		t.Fatal("Add did not set the NPC's Area to the embedded *Area")
	}
	if got := pa.Entities(); len(got) != 2 || got[0] != AreaActor(p) || got[1] != AreaActor(n) {
		t.Fatalf("Entities = %v, want [player npc] in join order", got)
	}

	// Re-adding is a no-op for the list but still sets the area
	// (PrivateArea.ts:45-56).
	p.SetArea(nil)
	pa.Add(p)
	if p.Area() != pa.Area {
		t.Fatal("re-Add did not set the area")
	}
	if got := len(pa.Entities()); got != 2 {
		t.Fatalf("re-Add duplicated the member: %d entities", got)
	}

	// Entities returns a copy: mutating it does not affect
	// membership.
	ents := pa.Entities()
	ents[0] = nil
	if pa.Entities()[0] == nil {
		t.Fatal("Entities exposed the live slice")
	}
}

func TestPrivateAreaEnteredEvent(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())

	// Uninstalled seam: no panic, no event.
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	pa.Add(p)

	// Installed seam: fires once, players only, with the TS
	// payload shape {player, area}.
	var events []PrivateAreaEnteredPayload
	var names []string
	p2 := NewPlayer("bob", Location{X: 5, Y: 5, Z: 0})
	p2.SetIndex(2)
	p2.EmitCustomEvent = func(name string, payload any) {
		names = append(names, name)
		if pl, ok := payload.(PrivateAreaEnteredPayload); ok {
			events = append(events, pl)
		}
	}
	pa.Add(p2)
	pa.Add(p2) // re-add: no second event
	n := NewNPC(1, Location{X: 5, Y: 5, Z: 0})
	n.SetIndex(3)
	pa.Add(n)
	if len(names) != 1 || names[0] != "private-area:entered" {
		t.Fatalf("event names = %v, want one private-area:entered", names)
	}
	if len(events) != 1 || events[0].Player != p2 || events[0].Area != pa {
		t.Fatalf("payload = %+v, want {bob, pa}", events)
	}
}

func TestPrivateAreaRemoveAndDetach(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	pa.Add(p)

	// Detach drops the member without touching the area
	// (PrivateArea.ts:39-41).
	pa.Detach(p)
	if len(pa.Entities()) != 0 {
		t.Fatal("Detach did not drop the member")
	}
	if p.Area() != pa.Area {
		t.Fatal("Detach touched the actor's area")
	}
	if p.PrivateArea() != pa {
		t.Fatal("PrivateArea lost after Detach (area pointer unchanged)")
	}

	// Remove detaches AND clears the area (PrivateArea.ts:34-37).
	pa.Add(p)
	pa.Remove(p)
	if len(pa.Entities()) != 0 {
		t.Fatal("Remove did not drop the member")
	}
	if p.Area() != nil {
		t.Fatal("Remove did not clear the actor's area")
	}
	if p.PrivateArea() != nil {
		t.Fatal("PrivateArea non-nil after Remove")
	}

	// Removing a non-member is a no-op for the list but still
	// clears the area, as TS remove() does unconditionally.
	pa.Remove(p)
	if p.Area() != nil {
		t.Fatal("Remove of non-member did not clear the area")
	}
}

func TestPrivateAreaDestroyOnEmptyViaLeave(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	pa.Add(p)

	// Direct Remove does NOT destroy (only postLeave does).
	pa.Remove(p)
	if pa.Destroyed() {
		t.Fatal("direct Remove destroyed the area")
	}

	// Leave through the embedded Area runs postLeave: Remove +
	// destroy when no players remain in the member map.
	pa.Add(p)
	pa.Area.Leave(p, false)
	if !pa.Destroyed() {
		t.Fatal("Leave did not destroy the empty area")
	}
	if len(pa.Entities()) != 0 {
		t.Fatal("Destroy did not clear members")
	}
	// Destroy is idempotent.
	pa.Destroy()
	if !pa.Destroyed() {
		t.Fatal("second Destroy cleared the destroyed flag")
	}
}

func TestPrivateAreaLeaveWithPlayersRemainingDoesNotDestroy(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	p2 := NewPlayer("bob", Location{X: 5, Y: 5, Z: 0})
	p2.SetIndex(2)
	// Enter populates the embedded Area's member map (the map
	// postLeave counts); Add alone does not.
	pa.Area.Enter(p)
	pa.Area.Enter(p2)
	pa.Area.Leave(p, false)
	if pa.Destroyed() {
		t.Fatal("Leave destroyed the area while a player remained")
	}
	if len(pa.Entities()) != 1 {
		t.Fatalf("Entities = %d, want 1 after one leave", len(pa.Entities()))
	}
	pa.Area.Leave(p2, false)
	if !pa.Destroyed() {
		t.Fatal("Leave of the last player did not destroy the area")
	}
}

func TestPrivateAreaDestroyHookOrderAndUnindex(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	pa.Add(p)
	loc := Location{X: 5, Y: 5, Z: 0}
	pa.SetClip(loc, 0x100)

	// The hook runs first, with members and clips intact.
	var sawMembers, sawClip bool
	prev := privateAreaDestroyHook
	SetPrivateAreaDestroyHook(func(hooked *PrivateArea) {
		sawMembers = len(hooked.Entities()) == 1
		sawClip = hooked.HasClip(loc)
	})
	defer SetPrivateAreaDestroyHook(prev)

	pa.Destroy()
	if !sawMembers || !sawClip {
		t.Fatalf("hook saw members=%v clip=%v, want both true", sawMembers, sawClip)
	}
	if pa.HasClip(loc) {
		t.Fatal("Destroy did not clear clips")
	}
	if p.PrivateArea() != nil {
		t.Fatal("PrivateArea still resolves after Destroy (not unindexed)")
	}
}

func TestPrivateAreaClipStoreSemantics(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	loc := Location{X: 3, Y: 4, Z: 1}
	other := Location{X: 3, Y: 4, Z: 0}

	if pa.HasClip(loc) {
		t.Fatal("fresh area has a clip opinion")
	}
	if pa.GetClip(loc) != 0 {
		t.Fatal("GetClip without an opinion is not 0")
	}
	pa.SetClip(loc, 0x12c0102)
	if !pa.HasClip(loc) {
		t.Fatal("SetClip did not record an opinion")
	}
	if got := pa.GetClip(loc); got != 0x12c0102 {
		t.Fatalf("GetClip = %#x, want 0x12c0102", got)
	}
	// Opinions are per-level: the same x,y on another level is
	// untouched.
	if pa.HasClip(other) {
		t.Fatal("clip opinion leaked across levels")
	}
	pa.RemoveClip(loc)
	if pa.HasClip(loc) || pa.GetClip(loc) != 0 {
		t.Fatal("RemoveClip did not clear the opinion")
	}
	// Removing a missing opinion is a no-op.
	pa.RemoveClip(loc)
}

func TestPrivateAreaClipStoreSwap(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	loc := Location{X: 3, Y: 4, Z: 0}
	pa.SetClip(loc, 0x100)

	// The base store is the map store: it holds the opinion
	// written before the swap.
	base := pa.BaseClipStore()
	if base == nil || !base.HasClip(loc) || base.GetClip(loc) != 0x100 {
		t.Fatal("BaseClipStore did not hold the map opinion")
	}

	// An installed store takes over reads and writes; the base
	// store keeps its own state (the `super` fallback).
	installed := &mapClipStore{clips: map[Location]int32{loc: 0x200}}
	pa.SetClipStore(installed)
	if got := pa.GetClip(loc); got != 0x200 {
		t.Fatalf("GetClip after swap = %#x, want 0x200", got)
	}
	pa.SetClip(loc, 0x400)
	if got := installed.GetClip(loc); got != 0x400 {
		t.Fatalf("installed store = %#x, want 0x400", got)
	}
	if got := base.GetClip(loc); got != 0x100 {
		t.Fatalf("base store changed through the installed store: %#x", got)
	}
	// Removing the opinion from the installed store does not
	// resurrect the base opinion: HasClip answers the active
	// store only.
	pa.RemoveClip(loc)
	if pa.HasClip(loc) {
		t.Fatal("HasClip true after removing the installed opinion")
	}
	// A nil install restores the base store.
	pa.SetClipStore(nil)
	if !pa.HasClip(loc) || pa.GetClip(loc) != 0x100 {
		t.Fatal("nil SetClipStore did not restore the base store")
	}
}

func TestPrivateAreaFlags(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	if pa.UsesTemplateMap() {
		t.Fatal("plain PrivateArea reports UsesTemplateMap")
	}
	if pa.Templated() != nil {
		t.Fatal("plain PrivateArea has a Templated back-pointer")
	}
	if pa.CountsAsMainWorld() {
		t.Fatal("CountsAsMainWorld defaults to true")
	}
	pa.SetCountsAsMainWorld(true)
	if !pa.CountsAsMainWorld() {
		t.Fatal("SetCountsAsMainWorld(true) did not stick")
	}
	pa.SetCountsAsMainWorld(false)
	if pa.CountsAsMainWorld() {
		t.Fatal("SetCountsAsMainWorld(false) did not stick")
	}
	// No despawn-delay opinion by default; the hook supplies one.
	if _, ok := pa.GroundItemDespawnDelay(); ok {
		t.Fatal("GroundItemDespawnDelay has an opinion with no hook")
	}
	pa.GroundItemDespawnDelayHook = func() (int, bool) { return 600, true }
	if delay, ok := pa.GroundItemDespawnDelay(); !ok || delay != 600 {
		t.Fatalf("GroundItemDespawnDelay = (%d, %v), want (600, true)", delay, ok)
	}
	// Nil-safe flag reads.
	var nilPA *PrivateArea
	if nilPA.UsesTemplateMap() || nilPA.CountsAsMainWorld() || nilPA.Destroyed() || nilPA.Templated() != nil {
		t.Fatal("nil PrivateArea flag reads are not all false/nil")
	}
	if _, ok := nilPA.GroundItemDespawnDelay(); ok {
		t.Fatal("nil PrivateArea GroundItemDespawnDelay has an opinion")
	}
}

func TestPrivateAreaObjectsViaConstructor(t *testing.T) {
	pa := NewPrivateArea(privateAreaBounds())
	loc := Location{X: 5, Y: 5, Z: 0}
	obj := NewGameObject(100, loc, 10, 0, pa)
	if obj.PrivateArea() != pa {
		t.Fatal("NewGameObject did not record the area")
	}
	if got := pa.GetObjects(); len(got) != 1 || got[0] != obj {
		t.Fatalf("GetObjects = %v, want [obj]", got)
	}
	// Attaching the same object twice is a no-op.
	pa.AttachObject(obj)
	if len(pa.GetObjects()) != 1 {
		t.Fatal("AttachObject duplicated the object")
	}
	// A shared-world object attaches nowhere.
	shared := NewGameObject(101, loc, 10, 0, nil)
	if shared.PrivateArea() != nil {
		t.Fatal("shared-world object has an area")
	}
	if len(pa.GetObjects()) != 1 {
		t.Fatal("shared-world object attached to the area")
	}
	// Detach drops it; Destroy clears the list.
	pa.DetachObject(obj)
	if len(pa.GetObjects()) != 0 {
		t.Fatal("DetachObject did not drop the object")
	}
	pa.AttachObject(obj)
	pa.Destroy()
	if len(pa.GetObjects()) != 0 {
		t.Fatal("Destroy did not clear objects")
	}
}

func TestMobilePrivateArea(t *testing.T) {
	var nilMobile *Mobile
	if nilMobile.PrivateArea() != nil {
		t.Fatal("nil Mobile PrivateArea is non-nil")
	}
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	if p.PrivateArea() != nil {
		t.Fatal("fresh player is in a private area")
	}
	// A plain registered-style area does not resolve.
	plain := NewArea("Plain", privateAreaBounds())
	p.SetArea(plain)
	if p.PrivateArea() != nil {
		t.Fatal("plain Area resolved as a PrivateArea")
	}
	// Joining resolves; destroying unresolves (stale members of
	// a destroyed area read nil, as the index unregisters).
	pa := NewPrivateArea(privateAreaBounds())
	pa.Add(p)
	if p.PrivateArea() != pa {
		t.Fatal("joined player did not resolve the area")
	}
	pa.Destroy()
	if p.PrivateArea() != nil {
		t.Fatal("member of a destroyed area still resolves it")
	}
}

func TestPrivateAreaManagerInterplay(t *testing.T) {
	mgr := NewAreaManager()
	// A registered area overlapping the private area's bounds:
	// it must NOT steal a private-area member (the manager
	// re-looks-up only for registered current areas).
	registered := NewArea("Registered", privateAreaBounds())
	mgr.Register(registered)

	pa := NewPrivateArea(privateAreaBounds())
	p := NewPlayer("alice", Location{X: 5, Y: 5, Z: 0})
	p.SetIndex(1)
	pa.Add(p)

	// Walking inside the boundaries keeps the area pointer
	// stable across Process ticks.
	for _, loc := range []Location{{X: 5, Y: 5, Z: 0}, {X: 6, Y: 5, Z: 0}, {X: 6, Y: 6, Z: 0}} {
		p.SetLocation(loc)
		mgr.Process(p)
		if p.Area() != pa.Area {
			t.Fatalf("Process at %v moved the player out of the private area", loc)
		}
		if p.PrivateArea() != pa {
			t.Fatalf("PrivateArea lost at %v", loc)
		}
	}

	// Walking outside fires Leave through Process: postLeave
	// removes the member and destroys the now-empty area.
	p.SetLocation(Location{X: 50, Y: 50, Z: 0})
	mgr.Process(p)
	if !pa.Destroyed() {
		t.Fatal("walking out did not destroy the empty private area")
	}
	if len(pa.Entities()) != 0 {
		t.Fatal("walking out did not remove the member")
	}
}
