package game

import "testing"

// Boat visibility: the rootLocation/syncArea/canSeeAboard sites
// in visibility.go read the live BoatManager.

func boatVisibilitySetup(t *testing.T) (*World, *BoatManager, *Boat) {
	t.Helper()
	w := newTestWorld()
	m := NewBoatManager(nil)
	w.SetBoatManager(m)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	return w, m, boat
}

func TestVisibilityAboardPlayerSeenFromRoot(t *testing.T) {
	w, m, boat := boatVisibilitySetup(t)
	viewer := newVisibilityObserver(t, w, "viewer", Location{X: 3000, Y: 3000, Z: 0})
	aboard := addPlayerDirect(t, w, "aboard", Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0})
	// The deck member files at the boat's root tile, not the
	// deck tile thousands of tiles away.
	m.GetDeck(boat).Enter(aboard)
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(viewer)
	if !containsPlayer(viewer.LocalPlayers(), aboard) {
		t.Error("an aboard player must be visible from the boat's root tile")
	}
}

func TestVisibilityOwnerOnlyBoatHidesAboard(t *testing.T) {
	w, m, boat := boatVisibilitySetup(t)
	viewer := newVisibilityObserver(t, w, "viewer", Location{X: 3000, Y: 3000, Z: 0})
	aboard := addPlayerDirect(t, w, "aboard", Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0})
	m.GetDeck(boat).Enter(aboard)
	boat.OwnerOnly = true
	boat.OwnerPlayerID = aboard.GetIndex()
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(viewer)
	if containsPlayer(viewer.LocalPlayers(), aboard) {
		t.Error("an aboard player on an owner-only boat must be hidden from strangers")
	}
	// The owner sees them: the gate reads the viewer's index.
	ownerViewer := NewPlayer("ownerViewer", Location{X: 3000, Y: 3000, Z: 0})
	ownerViewer.SetIndex(aboard.GetIndex())
	if !m.CanSeeAboard(&ownerViewer.Mobile, &aboard.Mobile) {
		t.Error("the owner must see aboard their own boat")
	}
}

func TestVisibilityAboardNpcSeenFromRoot(t *testing.T) {
	w, m, boat := boatVisibilitySetup(t)
	viewer := newVisibilityObserver(t, w, "viewer", Location{X: 3000, Y: 3000, Z: 0})
	npc := addNPCDirect(t, w, 1, Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0}, true)
	m.GetDeck(boat).Enter(npc)
	w.RebuildUpdateBuckets()
	nearby := w.NearbyNPCsForUpdate(viewer)
	w.UpdateLocalNPCs(viewer, nearby)
	if !containsNPC(viewer.LocalNPCs(), npc) {
		t.Error("an aboard NPC must be visible from the boat's root tile")
	}
}

func TestVisibilitySyncAreaSeparates(t *testing.T) {
	w, _, _ := boatVisibilitySetup(t)
	viewer := newVisibilityObserver(t, w, "viewer", Location{X: 3000, Y: 3000, Z: 0})
	other := addPlayerDirect(t, w, "other", Location{X: 3000, Y: 3000, Z: 0})
	// Same tile, but the other player is in a private area:
	// sync areas differ, so they never see each other.
	area := NewPrivateArea([]Boundary{NewBoundary(2990, 3010, 2990, 3010, 0)})
	area.Enter(other)
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(viewer)
	if containsPlayer(viewer.LocalPlayers(), other) {
		t.Error("a player in a private area must be hidden from the main world")
	}
	// Both in the same private area: visible again.
	area.Enter(viewer)
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(viewer)
	if !containsPlayer(viewer.LocalPlayers(), other) {
		t.Error("players in the same private area must see each other")
	}
}
