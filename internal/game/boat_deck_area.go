package game

// BoatDeckArea is a boat's deck: its own scene far outside the
// real map, where everyone aboard stands. Ported from
// game/content/sailing/BoatDeckArea.ts.
//
// As a private area it gets per-area collision, pathfinding,
// object lookup and visibility. Deck coordinates have no cache
// map region, so every tile would read as walkable; the deck
// blocks its whole scene except the boat type's walkable tiles.
//
// This file is written against the frozen Phase 21 contract 1
// PrivateArea API only (NewPrivateArea, Add/Remove/Detach,
// Destroy, Entities, the clip accessors, UsesTemplateMap), plus
// the Area boundary/Enter/Leave surface and the ClipStore seam
// Worker B's PrivateArea exposes for subclasses to override
// clip rules. The deck's clip rules live in deckClipStore,
// installed with SetClipStore: it consults the base store first
// (the TS super fallback) and answers the deck rules for the
// rest. SetClip/RemoveClip keep the base store's behavior
// (TS inherits them from PrivateArea), so content-set clips
// still win over the deck rules.

type deckTileKey struct{ x, y int }

// BoatDeckArea is a boat's deck scene. Construct through
// NewBoatDeckArea; BoatManager owns its lifetime (dispose
// destroys it).
type BoatDeckArea struct {
	*PrivateArea

	boat     *Boat
	walkable map[deckTileKey]bool
	solid    map[deckTileKey]bool
}

// NewBoatDeckArea builds the deck scene for a boat from its
// spec: the boundary over the boat's 104x104 scene, the
// walkable tiles, and the spec's locs as objects.
func NewBoatDeckArea(boat *Boat, spec BoatSpec) *BoatDeckArea {
	boundary := NewBoundary(boat.SceneBaseX(), boat.SceneBaseX()+103, boat.SceneBaseY(), boat.SceneBaseY()+103, 0)
	deck := &BoatDeckArea{
		PrivateArea: NewPrivateArea([]Boundary{boundary}),
		boat:        boat,
		walkable:    make(map[deckTileKey]bool),
		solid:       make(map[deckTileKey]bool),
	}
	// A deck lives as long as its boat (BoatManager.dispose
	// destroys it), aboard or not: leaving only removes the
	// actor, never destroying the area (BoatDeckArea.ts:
	// postLeave). The embedded Area's PostLeaveHook is the Go
	// form of the TS postLeave override; replacing it is the
	// mechanism Worker B's PrivateArea documents for decks.
	deck.Area.PostLeaveHook = func(actor AreaActor, logout bool) {
		_ = logout
		deck.Remove(actor)
	}
	deck.SetCountsAsMainWorld(true)
	deck.SetClipStore(&deckClipStore{deck: deck})
	for _, tile := range spec.WalkableDeck {
		deck.walkable[deckTileKey{x: boat.DeckBaseX() + tile.X, y: boat.DeckBaseY() + tile.Y}] = true
	}
	for _, loc := range spec.Locs {
		deck.addLoc(loc)
	}
	return deck
}

// Boat returns the boat this deck belongs to.
func (d *BoatDeckArea) Boat() *Boat { return d.boat }

// SetLoc swaps whatever loc of the same shape is on a deck tile
// for another (a facility built).
func (d *BoatDeckArea) SetLoc(loc BoatDeckLoc) {
	location := d.locLocation(loc)
	for _, object := range d.GetObjects() {
		if object.Location().Equals(location) && object.Type() == loc.Shape {
			d.DetachObject(object)
		}
	}
	delete(d.solid, deckTileKey{x: location.X, y: location.Y})
	d.addLoc(loc)
}

// RemoveLoc takes a loc of that shape off its deck tile (a
// Barracuda Trial's rum crate, delivered).
func (d *BoatDeckArea) RemoveLoc(loc BoatDeckLoc) {
	location := d.locLocation(loc)
	for _, object := range d.GetObjects() {
		if object.Location().Equals(location) && object.Type() == loc.Shape {
			d.DetachObject(object)
		}
	}
	delete(d.solid, deckTileKey{x: location.X, y: location.Y})
}

func (d *BoatDeckArea) addLoc(loc BoatDeckLoc) {
	location := d.locLocation(loc)
	if loc.Blocks {
		d.solid[deckTileKey{x: location.X, y: location.Y}] = true
	}
	// The GameObject constructor attaches the object to the
	// area (its spawned-object list, reported by GetObjects).
	NewGameObject(loc.ID, location, loc.Shape, loc.Rotation, d.PrivateArea)
}

// locLocation is where a deck loc sits for clicks: the level
// people aboard stand on (0). The client draws the loc on the
// template's deck plane (loc.Level); see WorldEntitySync's
// deck-loc encoding.
func (d *BoatDeckArea) locLocation(loc BoatDeckLoc) Location {
	return Location{X: d.boat.DeckBaseX() + loc.X, Y: d.boat.DeckBaseY() + loc.Y, Z: 0}
}

// deckClipStore is the deck's clip rules (BoatDeckArea.ts:
// hasClip/getClip), consulting the base store first: a clip
// set explicitly on the area (content, template objects) wins
// over the deck rules, exactly as TS's super.hasClip check.
type deckClipStore struct {
	deck *BoatDeckArea
}

func (s *deckClipStore) HasClip(loc Location) bool {
	return s.deck.boat.ContainsDeckTile(loc.X, loc.Y) || s.deck.BaseClipStore().HasClip(loc)
}

func (s *deckClipStore) GetClip(loc Location) int32 {
	if base := s.deck.BaseClipStore(); base.HasClip(loc) {
		return base.GetClip(loc)
	}
	if !s.deck.boat.ContainsDeckTile(loc.X, loc.Y) {
		return 0
	}
	// Everyone aboard stands on level 0 of the deck scene.
	if loc.Z != 0 {
		return ClipBlockedTile
	}
	key := deckTileKey{x: loc.X, y: loc.Y}
	if !s.deck.walkable[key] {
		return ClipBlockedTile
	}
	if s.deck.solid[key] {
		return solidObjectClip
	}
	return 0
}

func (s *deckClipStore) SetClip(loc Location, mask int32) {
	s.deck.BaseClipStore().SetClip(loc, mask)
}

func (s *deckClipStore) RemoveClip(loc Location) {
	s.deck.BaseClipStore().RemoveClip(loc)
}
