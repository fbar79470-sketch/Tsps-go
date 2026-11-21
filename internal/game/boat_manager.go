package game

// BoatManager runs boats at sea: it allocates their deck scenes,
// moves them each tick and answers which boat an actor is on.
// Sync and the boarding rules live elsewhere; this only runs
// the boats. Ported from game/content/sailing/BoatManager.ts.
//
// TS implements it as statics; Go uses a struct following the
// ShopManager/ObjectManager pattern (NewBoatManager, one
// manager per world). The manager lives on the World
// (World.SetBoatManager); visibility, movement, and the
// world-entity sync read it from there, degenerating to the
// no-boats behavior when none is installed. There is deliberately
// no process-wide current manager: the engine tolerates a tick
// in flight past Stop (see Engine.Stop), and a re-published
// global would let that tick read another world's boats. The
// coordinator installs the world's manager once at boot, before
// ticking starts.

const (
	// FirstBoatEntityIndex / LastBoatEntityIndex bound the
	// world-entity index range boats allocate from
	// (BoatManager.ts: FIRST_ENTITY_INDEX / LAST_ENTITY_INDEX).
	FirstBoatEntityIndex = 3000
	LastBoatEntityIndex  = 3999
	// deckRegionBase is DECK_REGION_BASE: deck scenes live far
	// outside the real map (tile 9600+), 16 chunks apart.
	deckRegionBase = 1200
	// deckRegionSpacing is DECK_REGION_SPACING, in chunks.
	deckRegionSpacing = 16
	// deckRegionColumns is DECK_REGION_COLUMNS.
	deckRegionColumns = 32
	// solidObjectClip is SOLID_OBJECT: a solid object in the
	// main world (loc shapes 9-11).
	solidObjectClip = 0x100
)

// HeadingListener reacts to the helm setting a new heading
// (BoatManager.ts: HeadingListener), e.g. content raising sail.
type HeadingListener func(player *Mobile, boat *Boat)

// BoatCollisionSource is the main-world surface IsSailable
// reads: RegionManager satisfies it (IsWater,
// RegionManager.ts:108-111; GetClipping,
// RegionManager.ts:472). A nil source reads no water anywhere.
type BoatCollisionSource interface {
	IsWater(pos Location) bool
	GetClipping(x, y, level int) int32
}

type activeBoat struct {
	boat *Boat
	spec BoatSpec
	deck *BoatDeckArea
	// locChanges are the deck locs changed since the boat
	// spawned, in order; viewers catch up by index.
	locChanges []BoatDeckLoc
}

// BoatManager is the boat runtime. Construct with
// NewBoatManager; drive Tick from the world's move_boats phase.
type BoatManager struct {
	collision BoatCollisionSource

	boats       map[int]*activeBoat
	order       []int // entity indices in spawn order (TS Map order)
	decksByArea map[*Area]*BoatDeckArea

	// lastEntityIndex is the index handed out last; the next
	// boat takes the next free one after it.
	lastEntityIndex int

	headingListeners   []HeadingListener
	afterTickListeners []func()

	// boatSquares tracks each non-static boat's 64-tile square
	// for RefreshRegionsUnderBoats (World.ts:876-892).
	boatSquares map[int]int
}

// NewBoatManager returns an empty manager reading main-world
// water/clipping through collision (nil degrades IsSailable to
// false everywhere).
func NewBoatManager(collision BoatCollisionSource) *BoatManager {
	return &BoatManager{
		collision:       collision,
		boats:           make(map[int]*activeBoat),
		decksByArea:     make(map[*Area]*BoatDeckArea),
		lastEntityIndex: LastBoatEntityIndex,
		boatSquares:     make(map[int]int),
	}
}

// IsDeckTile reports whether a tile is in the coordinate space
// reserved for boat decks (BoatManager.ts: isDeckTile).
func IsDeckTile(x, y int) bool {
	return x >= deckRegionBase*8 && y >= deckRegionBase*8
}

// Spawn spawns a player's boat (BoatManager.ts: spawn). It
// returns nil when no entity index is free.
func (m *BoatManager) Spawn(ownerIndex int, spec BoatSpec, placement BoatPlacement) *Boat {
	return m.spawnBoat(ownerIndex, spec, placement, false, 0, false)
}

// SpawnStatic spawns a fixed ship (a quest or world entity).
// Its entity index may be supplied so a quest can reuse the
// index the capture names; the deck scene allocation is the
// same. A supplied index outside the range, or already taken,
// falls back to allocation, exactly as TS does. It returns nil
// when no entity index is free.
func (m *BoatManager) SpawnStatic(ownerIndex int, spec BoatSpec, placement BoatPlacement, entityIndex ...int) *Boat {
	preferred := 0
	hasPreferred := false
	if len(entityIndex) > 0 {
		preferred, hasPreferred = entityIndex[0], true
	}
	return m.spawnBoat(ownerIndex, spec, placement, true, preferred, hasPreferred)
}

func (m *BoatManager) spawnBoat(ownerIndex int, spec BoatSpec, placement BoatPlacement, isStatic bool, preferredIndex int, hasPreferred bool) *Boat {
	entityIndex := 0
	allocated := false
	if hasPreferred && preferredIndex >= FirstBoatEntityIndex && preferredIndex <= LastBoatEntityIndex {
		if _, taken := m.boats[preferredIndex]; !taken {
			entityIndex, allocated = preferredIndex, true
		}
	}
	if !allocated {
		var ok bool
		entityIndex, ok = m.allocateEntityIndex()
		if !ok {
			return nil
		}
	}
	slot := entityIndex - FirstBoatEntityIndex
	init := BoatInit{
		EntityIndex:   entityIndex,
		ConfigID:      spec.ConfigID,
		OwnerPlayerID: ownerIndex,
		Static:        isStatic,
		DeckRegionX:   deckRegionBase + (slot%deckRegionColumns)*deckRegionSpacing,
		DeckRegionY:   deckRegionBase + (slot/deckRegionColumns)*deckRegionSpacing,
		SizeX:         spec.SizeX,
		SizeZ:         spec.SizeZ,
		Hull:          spec.Hull,
		DeckCentreX:   spec.DeckCentreX,
		DeckCentreY:   spec.DeckCentreY,
		FineX:         placement.FineX,
		FineY:         placement.FineY,
		Level:         placement.Level,
		Angle:         placement.Angle,
	}
	if spec.Stats != nil {
		init.BaseSpeed = spec.Stats.BaseSpeed
		init.SpeedCap = spec.Stats.SpeedCap
		init.Acceleration = spec.Stats.Acceleration
	}
	boat := NewBoat(init)
	deck := NewBoatDeckArea(boat, spec)
	m.boats[entityIndex] = &activeBoat{boat: boat, spec: spec, deck: deck}
	m.order = append(m.order, entityIndex)
	m.decksByArea[deck.Area] = deck
	return boat
}

// Dispose removes a boat from the sea and destroys its deck
// (BoatManager.ts: dispose). Disposing a boat this manager does
// not hold, or a different boat at the same index, is a no-op.
func (m *BoatManager) Dispose(boat *Boat) {
	if boat == nil {
		return
	}
	active, ok := m.boats[boat.EntityIndex]
	if !ok || active.boat != boat {
		return
	}
	delete(m.boats, boat.EntityIndex)
	delete(m.decksByArea, active.deck.Area)
	for i, index := range m.order {
		if index == boat.EntityIndex {
			m.order = append(m.order[:i], m.order[i+1:]...)
			break
		}
	}
	active.deck.Destroy()
}

// SetDeckLoc swaps a deck loc (whatever of the same shape is on
// its tile and level) for another, such as a facility built or
// removed. Viewers get it from WorldEntitySync.
func (m *BoatManager) SetDeckLoc(boat *Boat, loc BoatDeckLoc) {
	active := m.activeFor(boat)
	if active == nil {
		return
	}
	active.spec = replaceDeckLoc(active.spec, loc)
	active.deck.SetLoc(loc)
	active.locChanges = append(active.locChanges, loc)
}

// RemoveDeckLoc takes a deck loc (its tile and shape) off the
// boat, for everyone who sees it.
func (m *BoatManager) RemoveDeckLoc(boat *Boat, loc BoatDeckLoc) {
	active := m.activeFor(boat)
	if active == nil {
		return
	}
	if !hasDeckLoc(active.spec, loc) {
		return
	}
	active.spec = removeDeckLoc(active.spec, loc)
	active.deck.RemoveLoc(loc)
	removed := loc
	removed.Removed = true
	active.locChanges = append(active.locChanges, removed)
}

// UpdateDeckLoc swaps a deck loc as SetDeckLoc does, but
// without queueing it for WorldEntitySync: for a caller that
// has already sent it, so that it reaches viewers before that
// tick's loc animations (a loc sent after them would start
// over without its animation).
func (m *BoatManager) UpdateDeckLoc(boat *Boat, loc BoatDeckLoc) {
	active := m.activeFor(boat)
	if active == nil {
		return
	}
	active.spec = replaceDeckLoc(active.spec, loc)
	active.deck.SetLoc(loc)
}

func sameDeckLocSlot(a, b BoatDeckLoc) bool {
	return a.X == b.X && a.Y == b.Y && a.Level == b.Level && a.Shape == b.Shape
}

func hasDeckLoc(spec BoatSpec, loc BoatDeckLoc) bool {
	for _, other := range spec.Locs {
		if sameDeckLocSlot(other, loc) {
			return true
		}
	}
	return false
}

func replaceDeckLoc(spec BoatSpec, loc BoatDeckLoc) BoatSpec {
	locs := make([]BoatDeckLoc, 0, len(spec.Locs)+1)
	for _, other := range spec.Locs {
		if !sameDeckLocSlot(other, loc) {
			locs = append(locs, other)
		}
	}
	spec.Locs = append(locs, loc)
	return spec
}

func removeDeckLoc(spec BoatSpec, loc BoatDeckLoc) BoatSpec {
	locs := make([]BoatDeckLoc, 0, len(spec.Locs))
	for _, other := range spec.Locs {
		if !sameDeckLocSlot(other, loc) {
			locs = append(locs, other)
		}
	}
	spec.Locs = locs
	return spec
}

// DeckLocChanges reports how many deck loc changes a boat has
// had, and those from since on (BoatManager.ts:
// deckLocChanges).
func (m *BoatManager) DeckLocChanges(boat *Boat, since int) (count int, changes []BoatDeckLoc) {
	active := m.activeFor(boat)
	if active == nil {
		return 0, nil
	}
	changes = active.locChanges
	if since < 0 {
		since = 0
	}
	if since > len(changes) {
		since = len(changes)
	}
	return len(changes), append([]BoatDeckLoc(nil), changes[since:]...)
}

// GetBoat returns the boat at an entity index, or nil.
func (m *BoatManager) GetBoat(entityIndex int) *Boat {
	if active, ok := m.boats[entityIndex]; ok {
		return active.boat
	}
	return nil
}

// GetSpec returns the spec a boat was built from (with deck-loc
// swaps applied), or false when the manager does not hold the
// boat.
func (m *BoatManager) GetSpec(boat *Boat) (BoatSpec, bool) {
	active := m.activeFor(boat)
	if active == nil {
		return BoatSpec{}, false
	}
	return active.spec, true
}

// GetDeck returns a boat's deck area, or nil when the manager
// does not hold the boat.
func (m *BoatManager) GetDeck(boat *Boat) *BoatDeckArea {
	active := m.activeFor(boat)
	if active == nil {
		return nil
	}
	return active.deck
}

// All returns every boat, in spawn order (TS Map insertion
// order).
func (m *BoatManager) All() []*Boat {
	boats := make([]*Boat, 0, len(m.order))
	for _, index := range m.order {
		if active, ok := m.boats[index]; ok {
			boats = append(boats, active.boat)
		}
	}
	return boats
}

func (m *BoatManager) activeFor(boat *Boat) *activeBoat {
	if m == nil || boat == nil {
		return nil
	}
	active, ok := m.boats[boat.EntityIndex]
	if !ok || active.boat != boat {
		return nil
	}
	return active
}

// GetBoatAboard returns the boat whose deck an actor is on, if
// any (BoatManager.ts: getBoatAboard). TS tests the actor's
// area with instanceof BoatDeckArea; Go recovers the deck from
// the area pointer through the live-deck index (the deck's
// embedded *Area is what members carry), then verifies the boat
// is still registered, as TS verifies getBoat(area.boat.
// entityIndex) === area.boat.
func (m *BoatManager) GetBoatAboard(mobile *Mobile) *Boat {
	if m == nil || mobile == nil {
		return nil
	}
	deck, ok := m.decksByArea[mobile.Area()]
	if !ok {
		return nil
	}
	if m.GetBoat(deck.boat.EntityIndex) != deck.boat {
		return nil
	}
	return deck.boat
}

// RootLocation is where an actor is in the main world: the tile
// under their deck tile when aboard, otherwise their own tile.
// Sync range and main-world locs are measured from here
// (BoatManager.ts: rootLocation).
func (m *BoatManager) RootLocation(mobile *Mobile) Location {
	location := mobile.Location()
	boat := m.GetBoatAboard(mobile)
	if boat == nil {
		return location
	}
	x, y := boat.DeckTileToWorld(location.X, location.Y)
	return Location{X: x, Y: y, Z: boat.Level}
}

// DeckTileToward is where on their deck an actor aboard should
// walk to come nearest a main-world target: the walkable deck
// tile whose tile on the sea is closest to it. It reports false
// when the actor is not aboard, or when no deck tile is closer
// than where they stand (they're as near as the boat lets
// them). targetSize is the target's footprint in tiles (TS
// getSize, at least 1).
func (m *BoatManager) DeckTileToward(mobile *Mobile, target Location, targetSize int) (Location, bool) {
	boat := m.GetBoatAboard(mobile)
	spec, ok := m.GetSpec(boat)
	if boat == nil || !ok {
		return Location{}, false
	}
	size := max(1, targetSize)
	distance := func(x, y int) int {
		tileX, tileY := boat.DeckTileToWorld(x, y)
		dx := max(0, target.X-tileX, tileX-(target.X+size-1))
		dy := max(0, target.Y-tileY, tileY-(target.Y+size-1))
		return max(dx, dy)
	}
	here := mobile.Location()
	bestDistance := distance(here.X, here.Y)
	var best Location
	found := false
	for _, tile := range spec.WalkableDeck {
		x := boat.DeckBaseX() + tile.X
		y := boat.DeckBaseY() + tile.Y
		if d := distance(x, y); d < bestDistance {
			best = Location{X: x, Y: y, Z: here.Z}
			bestDistance = d
			found = true
		}
	}
	return best, found
}

// SyncArea is the private area an actor counts as being in for
// who-sees-whom: a deck counts as the main world, so people
// aboard and people ashore see each other (BoatManager.ts:
// syncArea). The actor's private area is Mobile.PrivateArea
// (Entity.getPrivateArea).
func (m *BoatManager) SyncArea(mobile *Mobile) *PrivateArea {
	if mobile == nil {
		return nil
	}
	area := mobile.PrivateArea()
	if area == nil || area.CountsAsMainWorld() {
		return nil
	}
	return area
}

// CanSeeBoat reports whether a viewer is shown a boat: any
// boat, except another player's owner-only one.
func (m *BoatManager) CanSeeBoat(viewer *Mobile, boat *Boat) bool {
	return !boat.OwnerOnly || boat.OwnerPlayerID == viewer.GetIndex()
}

// CanSeeAboard reports whether a viewer is shown an actor: not
// while it stands on a boat the viewer can't see.
func (m *BoatManager) CanSeeAboard(viewer, actor *Mobile) bool {
	boat := m.GetBoatAboard(actor)
	return boat == nil || m.CanSeeBoat(viewer, boat)
}

// CanInteractWithBoat is the world-entity click gate
// (NetworkBuilder.ts:747-762): the boat exists, the viewer can
// see it, and the viewer's root tile is within BOAT_VIEW_DISTANCE
// of the boat's tile. Forged/foreign/out-of-range indices fail
// silently, never reaching plugin handlers.
func (m *BoatManager) CanInteractWithBoat(viewer *Mobile, entityIndex int) bool {
	boat := m.GetBoat(entityIndex)
	if boat == nil || !m.CanSeeBoat(viewer, boat) {
		return false
	}
	root := m.RootLocation(viewer)
	return root.IsWithinDistance(Location{X: boat.TileX(), Y: boat.TileY(), Z: boat.Level}, BoatViewDistance)
}

// OnHeadingSet registers content that reacts to the helm
// setting a new heading (such as raising sail).
func (m *BoatManager) OnHeadingSet(listener HeadingListener) {
	m.headingListeners = append(m.headingListeners, listener)
}

// SteerToward handles a walk click from the helm. A sea tile
// points the boat at it (a client steering by the heading
// interaction mode sends SET_HEADING instead); the heading
// snaps to the 16 helm directions. A deck tile does nothing:
// the boat's own world stays in walk mode at the helm, and
// live neither steers nor walks the helmsman for those clicks
// (rsprox). It reports whether the click was consumed: true
// when the clicker steers (a deck click from the helmsman
// consumes the click by doing nothing; a captain whose
// crewmate holds the helm walks the deck as usual).
func (m *BoatManager) SteerToward(player *Mobile, x, y int) bool {
	boat := m.GetBoatAboard(player)
	if boat == nil || !m.Steers(player, boat) {
		return false
	}
	// A captain whose crewmate holds the helm walks the deck
	// as usual (rsprox).
	if boat.ContainsDeckTile(x, y) {
		return boat.HelmPlayerID != nil && *boat.HelmPlayerID == player.GetIndex()
	}
	if heading, ok := boat.HelmHeadingToward(x, y); ok {
		m.applyHelmHeading(player, boat, heading)
	}
	return true
}

// Steers reports whether a player steers the boat: from its
// helm, or as its owner while a crewmate holds the helm
// (rsprox: the owner's clicks on the sea set the heading then,
// from anywhere on deck).
func (m *BoatManager) Steers(player *Mobile, boat *Boat) bool {
	if boat.HelmPlayerID != nil && *boat.HelmPlayerID == player.GetIndex() {
		return true
	}
	return boat.HelmCrew != nil && boat.OwnerPlayerID == player.GetIndex()
}

// SetHelmHeading applies an OSRS SetHeading (0-15), sent by the
// client in place of a click while steering.
func (m *BoatManager) SetHelmHeading(player *Mobile, packedHeading int) {
	boat := m.GetBoatAboard(player)
	if boat == nil || !m.Steers(player, boat) {
		return
	}
	if packedHeading < 0 || packedHeading > 15 {
		return
	}
	m.applyHelmHeading(player, boat, PackedHeadingToAngle(packedHeading))
}

// IsSailable reports whether a main-world tile is one a hull
// may cover: open water with no solid object on it.
func (m *BoatManager) IsSailable(x, y, level int) bool {
	if level != 0 {
		return false
	}
	if m.collision == nil {
		return false
	}
	if !m.collision.IsWater(Location{X: x, Y: y, Z: 0}) {
		return false
	}
	return (m.collision.GetClipping(x, y, 0) & solidObjectClip) == 0
}

// BoatTeleportTarget is the destination of BoatManager.Teleport:
// a fine position and, when Angle is set, a facing.
type BoatTeleportTarget struct {
	FineX int
	FineY int
	Angle *int
}

// Teleport moves a boat at once to another position (and
// facing, if given): a Barracuda Trial's start, a portal.
// Viewers see it jump there (WorldEntitySync), and it sails on
// from the new facing.
func (m *BoatManager) Teleport(boat *Boat, to BoatTeleportTarget) {
	boat.FineX = to.FineX
	boat.FineY = to.FineY
	boat.PrevFineX = to.FineX
	boat.PrevFineY = to.FineY
	if to.Angle != nil {
		boat.Angle = *to.Angle
		boat.Heading = *to.Angle
	}
	boat.TeleportCount++
}

// OnAfterTick registers a listener that runs after every boat
// has moved each tick (Sailing.recordPositions).
func (m *BoatManager) OnAfterTick(listener func()) {
	m.afterTickListeners = append(m.afterTickListeners, listener)
}

// Tick moves every boat one tick, then runs the after-tick
// listeners (BoatManager.ts: tick).
func (m *BoatManager) Tick() {
	for _, index := range m.order {
		active, ok := m.boats[index]
		if !ok {
			continue
		}
		boat := active.boat
		boat.PrevFineX = boat.FineX
		boat.PrevFineY = boat.FineY
		step := TickBoat(boat, m.IsSailable)
		boat.LastStep = &step
	}
	for _, listener := range m.afterTickListeners {
		listener()
	}
}

// RefreshRegionsUnderBoats ports World.refreshRegionsUnderBoats
// (World.ts:876-892): it tracks each non-static boat's 64-tile
// square and reports whether any boat changed square this tick
// (or any boat arrived or left). In TS a change refreshes the
// active regions and re-collects the active NPCs; Go has no
// active-region index (visibility.go: region gating NOT
// STARTED), so the caller treats a true result as a no-op
// until region activation lands — the tracking itself is
// faithful and tested.
func (m *BoatManager) RefreshRegionsUnderBoats() bool {
	changed := false
	seen := make(map[int]bool, len(m.order))
	for _, index := range m.order {
		active, ok := m.boats[index]
		if !ok || active.boat.Static {
			continue
		}
		seen[index] = true
		square := ((active.boat.TileX() >> 6) << 16) | (active.boat.TileY() >> 6)
		if m.boatSquares[index] != square {
			m.boatSquares[index] = square
			changed = true
		}
	}
	for index := range m.boatSquares {
		if !seen[index] {
			delete(m.boatSquares, index)
		}
	}
	return changed
}

func (m *BoatManager) applyHelmHeading(player *Mobile, boat *Boat, heading int) {
	boat.Heading = heading
	for _, listener := range m.headingListeners {
		listener(player, boat)
	}
}

// allocateEntityIndex returns the next free index after the
// last one handed out, wrapping round: a boat replaced by a new
// one (a part swapped) gets a new index, as live does, so
// viewers despawn the old and spawn the new in the same update
// rather than a tick apart (an index can't be both in one
// update). It reports false when all 1000 indices are taken.
func (m *BoatManager) allocateEntityIndex() (int, bool) {
	count := LastBoatEntityIndex - FirstBoatEntityIndex + 1
	for step := 1; step <= count; step++ {
		index := FirstBoatEntityIndex + ((m.lastEntityIndex - FirstBoatEntityIndex + step) % count)
		if _, taken := m.boats[index]; taken {
			continue
		}
		m.lastEntityIndex = index
		return index, true
	}
	return 0, false
}

// --- world-manager helpers for visibility ---
//
// These read the world's manager and degenerate to the
// no-boats behavior when none is installed, so visibility.go
// keeps its TS shape with no manager plumbing beyond the World.

// rootLocationOf is BoatManager.rootLocation through the world's
// manager, or the actor's own tile with none installed.
func (w *World) rootLocationOf(mobile *Mobile) Location {
	if m := w.boats; m != nil {
		return m.RootLocation(mobile)
	}
	return mobile.Location()
}

// syncAreaOf is BoatManager.syncArea through the world's
// manager, or nil with none installed.
func (w *World) syncAreaOf(mobile *Mobile) *PrivateArea {
	if m := w.boats; m != nil {
		return m.SyncArea(mobile)
	}
	return nil
}

// canSeeAboardBy is BoatManager.canSeeAboard through the world's
// manager, or true with none installed.
func (w *World) canSeeAboardBy(viewer, actor *Mobile) bool {
	if m := w.boats; m != nil {
		return m.CanSeeAboard(viewer, actor)
	}
	return true
}

// boatAboard is BoatManager.getBoatAboard through the world's
// manager, or nil with none installed.
func (w *World) boatAboard(mobile *Mobile) *Boat {
	if m := w.boats; m != nil {
		return m.GetBoatAboard(mobile)
	}
	return nil
}
