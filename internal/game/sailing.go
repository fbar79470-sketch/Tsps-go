package game

import "sort"

// Sailing is every transition of a player's boats: boarding and
// disembarking at a dock, sinking on a teleport, Escape or
// death at sea, recording the boat when its owner logs out and
// putting them back aboard on login, and recovery by a
// shipwright. Ported from game/content/sailing/Sailing.ts.
//
// TS implements it as statics; Go uses a struct holding the
// BoatManager it spawns through, the registries, and the live
// instances, following the ShopManager pattern. One Sailing
// serves the world; the coordinator keeps it (login/logout call
// sites) and installs it on the hub (the Core.Sailing accessor's
// source, via Hub.SetSailing). There is deliberately no
// process-wide current instance: the engine tolerates a tick in
// flight past Stop (see Engine.Stop), and a re-published global
// would let that tick reach another world's Sailing. The sailing
// skill content that calls these transitions (boarding
// interfaces, the helm, shipwrights) is OUT OF SCOPE for this
// milestone; this is the engine mechanism content will call.

// MaxBoats is how many boats a player may own: OSRS lets a
// player own up to 5 (more slots unlock with Sailing level).
const MaxBoats = 5

// MooredRange is how far from a dock's landing (a mooring
// point's own tile) a boat still counts as moored there, in
// tiles.
const MooredRange = 15

// LostAtSea is the boarding refusal for a boat lost at sea,
// verbatim (Sailing.ts: LOST_AT_SEA).
const LostAtSea = "You cannot board your boat as it is lost at sea. You'll need to speak to a shipwright to recover it. To find a shipwright, just look for the relevant icon on your map."

// LeaveReason is why a player left their boat, sent with the
// sailing:left event.
type LeaveReason string

const (
	LeaveDisembark LeaveReason = "disembark"
	LeaveSunk      LeaveReason = "sunk"
	LeaveLogout    LeaveReason = "logout"
	LeaveRemoved   LeaveReason = "removed"
)

// GuestLeaveReason is why a guest left another player's boat,
// sent with sailing:guest-left: they disembarked, teleported
// (or died) off it, logged out, or the boat went with its
// owner ("evicted").
type GuestLeaveReason string

const (
	GuestLeaveDisembark GuestLeaveReason = "disembark"
	GuestLeaveTeleport  GuestLeaveReason = "teleport"
	GuestLeaveLogout    GuestLeaveReason = "logout"
	GuestLeaveEvicted   GuestLeaveReason = "evicted"
)

// GuestRefusal is why a player can't board another player's
// boat (content has the wording). The zero value "" means no
// refusal.
type GuestRefusal string

const (
	GuestRefusalAboard    GuestRefusal = "aboard"
	GuestRefusalNotAboard GuestRefusal = "not-aboard"
	GuestRefusalNotHere   GuestRefusal = "not-here"
	GuestRefusalFull      GuestRefusal = "full"
)

// SailingDock is a place boats moor: a port gangplank or an
// island mooring point (Sailing.ts: SailingDock).
type SailingDock struct {
	ID string
	// MooringPoint marks an island mooring point rather than
	// a port: it isn't a "standard dock".
	MooringPoint bool
	// Mooring is where a boat docked here sits: fine position
	// (1/128 tile) and angle.
	Mooring BoatPlacement
	// Moorings is where a boat of a given type sits instead,
	// when it differs (bigger hulls).
	Moorings map[string]BoatPlacement
	// Landing is where a player steps ashore when disembarking
	// here.
	Landing Location
	// Gangplank is the gangplank or mooring point loc's tile,
	// when the dock has one.
	Gangplank *Location
	// TheName is the port's name as a sentence uses it ("the
	// Pandemonium", "Catherby"); the id is used when empty.
	TheName string
}

// SailingBoardedPayload is the sailing:boarded custom-event
// payload: {player, boat, owned}.
type SailingBoardedPayload struct {
	Player *Player
	Boat   *Boat
	Owned  OwnedBoat
}

// SailingLeftPayload is the sailing:left custom-event payload:
// {player, boat, reason, atHelm}.
type SailingLeftPayload struct {
	Player *Player
	Boat   *Boat
	Reason LeaveReason
	AtHelm bool
}

// SailingGuestBoardedPayload is the sailing:guest-boarded
// custom-event payload: {player, boat, owner, owned}.
type SailingGuestBoardedPayload struct {
	Player *Player
	Boat   *Boat
	Owner  *Player
	Owned  OwnedBoat
}

// SailingGuestLeftPayload is the sailing:guest-left
// custom-event payload: {player, boat, reason, atHelm}.
type SailingGuestLeftPayload struct {
	Player *Player
	Boat   *Boat
	Reason GuestLeaveReason
	AtHelm bool
}

// SpecResolver builds a boat's spec from its owned parts.
// Content registers it (boat parts live in the cache and plugin
// data); without one a boat is built as its type's base spec.
type SpecResolver func(boat OwnedBoat, base BoatSpec) BoatSpec

type activeInstance struct {
	player *Player
	slot   int
}

// Sailing runs the boat transitions. Construct with
// NewSailing; Initialize wires the teleport and after-tick
// hooks (idempotent).
type Sailing struct {
	boats           *BoatManager
	defaultLocation Location

	types     map[string]BoatSpec
	docks     map[string]SailingDock
	instances map[*Boat]*activeInstance
	// expectedMove is the player whose move Sailing itself is
	// making, so it isn't taken for a teleport.
	expectedMove *Player
	initialized  bool
	specResolver SpecResolver

	// emit is the custom-event sink, installed by wiring (the
	// hub's EmitCustomEvent). When nil, events fire through
	// the subject player's EmitCustomEvent seam instead; when
	// that is nil too, they are dropped, nil-safely.
	emit func(name string, payload any)
}

// NewSailing returns a Sailing spawning through boats.
// defaultLocation is where Escape and logout-at-sea send a
// player with no return point: the world's spawn
// (GameConstants.DEFAULT_LOCATION, the world definition's
// spawn).
func NewSailing(boats *BoatManager, defaultLocation Location) *Sailing {
	return &Sailing{
		boats:           boats,
		defaultLocation: defaultLocation,
		types:           make(map[string]BoatSpec),
		docks:           make(map[string]SailingDock),
		instances:       make(map[*Boat]*activeInstance),
	}
}

// BoatManager returns the BoatManager this Sailing spawns
// through, or nil for a zero Sailing. Content and the
// coordinator reach the world's manager through here.
func (s *Sailing) BoatManager() *BoatManager {
	if s == nil {
		return nil
	}
	return s.boats
}

// SetEmit installs the custom-event sink (the hub's
// EmitCustomEvent). Wiring calls this; tests may leave it nil
// and read events off the players' EmitCustomEvent seams.
func (s *Sailing) SetEmit(emit func(name string, payload any)) {
	s.emit = emit
}

func (s *Sailing) emitEvent(player *Player, name string, payload any) {
	if s.emit != nil {
		s.emit(name, payload)
		return
	}
	if player != nil && player.EmitCustomEvent != nil {
		player.EmitCustomEvent(name, payload)
	}
}

// Initialize wires the teleport hook and the after-tick
// position recording. It is idempotent: a second call is a
// no-op (Sailing.ts: initialize).
func (s *Sailing) Initialize() {
	if s.initialized {
		return
	}
	s.initialized = true
	OnBeforeTeleport(func(player *Player, target Location) { s.onTeleport(player, target) })
	s.boats.OnAfterTick(func() { s.recordPositions() })
}

// RegisterBoatType registers a boat type's base spec, keyed by
// the spec's type.
func (s *Sailing) RegisterBoatType(spec BoatSpec) {
	s.types[spec.Type] = spec
}

// RegisterDock registers a dock, keyed by its id.
func (s *Sailing) RegisterDock(dock SailingDock) {
	s.docks[dock.ID] = dock
}

// GetDock returns a registered dock, or false when unknown.
func (s *Sailing) GetDock(id string) (SailingDock, bool) {
	dock, ok := s.docks[id]
	return dock, ok
}

// SetSpecResolver installs the spec resolver (see
// SpecResolver).
func (s *Sailing) SetSpecResolver(resolver SpecResolver) {
	s.specResolver = resolver
}

// SpecFor returns the spec an owned boat is built from: its
// type's, with its own parts. It reports false when the type
// is not registered.
func (s *Sailing) SpecFor(boat OwnedBoat) (BoatSpec, bool) {
	base, ok := s.types[boat.Type]
	if !ok {
		return BoatSpec{}, false
	}
	if s.specResolver != nil {
		return s.specResolver(boat, base), true
	}
	return base, true
}

// GiveBoat adds a boat docked at dockId in the player's next
// free slot. It reports false when the type or dock is unknown
// or the player already owns MaxBoats boats. The returned
// record is a copy of the live record: live state is reached
// through GetSailing().Boats (TS returns the live object; Go
// slices reallocate, so no stable pointer exists).
func (s *Sailing) GiveBoat(player *Player, boatType, dockId string, name BoatName) (OwnedBoat, bool) {
	state := player.GetSailing()
	if _, ok := s.types[boatType]; !ok {
		return OwnedBoat{}, false
	}
	if _, ok := s.docks[dockId]; !ok {
		return OwnedBoat{}, false
	}
	if len(state.Boats) >= MaxBoats {
		return OwnedBoat{}, false
	}
	slot := 0
	for s.ownedBoat(player, slot) != nil {
		slot++
	}
	boat := OwnedBoat{
		Slot:       slot,
		Type:       boatType,
		Name:       name,
		Facilities: []int{},
		Location:   DockedBoatLocation(dockId),
		Cargo:      []*CargoSlot{},
		Parts:      BaseParts(),
	}
	state.Boats = append(state.Boats, boat)
	sort.Slice(state.Boats, func(i, j int) bool { return state.Boats[i].Slot < state.Boats[j].Slot })
	if state.ActiveBoatSlot == nil {
		state.ActiveBoatSlot = &slot
	}
	return boat, true
}

// RemoveOwnedBoat removes the owned record in slot and its live
// deck, without refunds or sinking it. It reports false when
// the player owns no boat in that slot. Identity is by slot:
// TS removes the exact record object (indexOf), and a slot is
// unique within a player's boats. TS also empties the removed
// record's cargo so delayed cargo operations retaining the
// object find nothing; Go callers hold copies, so there is no
// retained object to empty.
func (s *Sailing) RemoveOwnedBoat(player *Player, slot int) bool {
	state := player.GetSailing()
	index := -1
	for i := range state.Boats {
		if state.Boats[i].Slot == slot {
			index = i
			break
		}
	}
	if index < 0 {
		return false
	}
	state.Boats = append(state.Boats[:index], state.Boats[index+1:]...)
	if state.ActiveBoatSlot != nil && *state.ActiveBoatSlot == slot {
		state.ActiveBoatSlot = nil
	}
	for instance, active := range s.instances {
		if active.player != player || active.slot != slot {
			continue
		}
		deck := s.boats.GetDeck(instance)
		ownerAboard := deck != nil && deck.HasMember(player)
		// Leaving puts any guests ashore first.
		s.leave(player, instance, LeaveRemoved)
		if ownerAboard {
			s.moveExpected(player, s.ReturnLocation(player))
		}
	}
	return true
}

// ActiveBoat returns the player's active boat record (the one
// they are aboard, or last set sail in), or nil.
func (s *Sailing) ActiveBoat(player *Player) *OwnedBoat {
	state := player.GetSailing()
	if state.ActiveBoatSlot == nil {
		return nil
	}
	return s.ownedBoat(player, *state.ActiveBoatSlot)
}

// ownedBoat returns the live record in slot, or nil. The
// pointer addresses the state slice: it is valid only until
// the next GiveBoat/RemoveOwnedBoat on this player.
func (s *Sailing) ownedBoat(player *Player, slot int) *OwnedBoat {
	state := player.GetSailing()
	for i := range state.Boats {
		if state.Boats[i].Slot == slot {
			return &state.Boats[i]
		}
	}
	return nil
}

// InstanceAboard returns the player's own boat instance they
// are standing on, if any.
func (s *Sailing) InstanceAboard(player *Player) *Boat {
	boat := s.boats.GetBoatAboard(&player.Mobile)
	if boat == nil {
		return nil
	}
	if active, ok := s.instances[boat]; !ok || active.player != player {
		return nil
	}
	return boat
}

// InstanceFor returns any live instance of this player's boat,
// aboard it or left while visiting a static ship.
func (s *Sailing) InstanceFor(player *Player) *Boat {
	if boat := s.InstanceAboard(player); boat != nil {
		return boat
	}
	for instance, active := range s.instances {
		if active.player == player {
			return instance
		}
	}
	return nil
}

// OwnerOf returns the player whose boat a live instance is, or
// nil when the boat is not a Sailing instance (a static ship).
func (s *Sailing) OwnerOf(instance *Boat) *Player {
	if active, ok := s.instances[instance]; ok {
		return active.player
	}
	return nil
}

// OwnedOf returns the owned record a live instance was built
// from, or nil. The pointer addresses the owner's state slice
// (see ownedBoat).
func (s *Sailing) OwnedOf(instance *Boat) *OwnedBoat {
	active, ok := s.instances[instance]
	if !ok {
		return nil
	}
	return s.ownedBoat(active.player, active.slot)
}

// GuestBoat returns another player's boat the player is aboard
// as a guest, if any.
func (s *Sailing) GuestBoat(player *Player) *Boat {
	boat := s.boats.GetBoatAboard(&player.Mobile)
	if boat == nil {
		return nil
	}
	owner := s.OwnerOf(boat)
	if owner == nil || owner == player {
		return nil
	}
	return boat
}

// GuestsOf returns the players aboard a boat who aren't its
// owner.
func (s *Sailing) GuestsOf(instance *Boat) []*Player {
	owner := s.OwnerOf(instance)
	deck := s.boats.GetDeck(instance)
	if deck == nil {
		return nil
	}
	var guests []*Player
	for _, aboard := range deck.GetPlayers() {
		if aboard != owner {
			guests = append(guests, aboard)
		}
	}
	return guests
}

// GuestRefusal reports why guest can't board owner's boat at
// dockId, or "" when they can: the boat has to be moored there
// with its owner aboard (a boat is only in the world while its
// owner is on it) and hold fewer than capacity players, its
// owner included.
func (s *Sailing) GuestRefusal(guest, owner *Player, dockId string, capacity int) GuestRefusal {
	if s.boats.GetBoatAboard(&guest.Mobile) != nil || s.InstanceFor(guest) != nil {
		return GuestRefusalAboard
	}
	var instance *Boat
	if guest != owner {
		instance = s.InstanceAboard(owner)
	}
	active, ok := s.instances[instance]
	if instance == nil || !ok {
		return GuestRefusalNotAboard
	}
	dock, ok := s.docks[dockId]
	deck := s.boats.GetDeck(instance)
	_, specOK := s.boats.GetSpec(instance)
	owned := s.ownedBoat(active.player, active.slot)
	if !ok || deck == nil || !specOK || owned == nil || s.LastPortOf(*owned) != dockId || !s.mooredAt(instance, dock) {
		return GuestRefusalNotHere
	}
	if len(deck.GetPlayers()) >= capacity {
		return GuestRefusalFull
	}
	return ""
}

// BoardAsGuest puts guest aboard owner's boat at dockId. It
// returns why they can't, otherwise "".
func (s *Sailing) BoardAsGuest(guest, owner *Player, dockId string, capacity int) GuestRefusal {
	if refusal := s.GuestRefusal(guest, owner, dockId, capacity); refusal != "" {
		return refusal
	}
	instance := s.InstanceAboard(owner)
	active := s.instances[instance]
	dock := s.docks[dockId]
	deck := s.boats.GetDeck(instance)
	spec, _ := s.boats.GetSpec(instance)
	guest.GetSailing().ReturnPoint = &ReturnPoint{X: dock.Landing.X, Y: dock.Landing.Y, Z: dock.Landing.Z}
	deck.Enter(guest)
	s.moveExpected(guest, Location{X: instance.DeckBaseX() + spec.BoardingTile.X, Y: instance.DeckBaseY() + spec.BoardingTile.Y, Z: 0})
	s.emitEvent(guest, "sailing:guest-boarded", SailingGuestBoardedPayload{Player: guest, Boat: instance, Owner: owner, Owned: *s.ownedBoat(active.player, active.slot)})
	return ""
}

// DisembarkGuest takes a guest off the boat they are visiting,
// onto dockId's landing. It returns a message to show when
// they can't, otherwise "".
func (s *Sailing) DisembarkGuest(guest *Player, dockId string) string {
	dock, ok := s.docks[dockId]
	instance := s.GuestBoat(guest)
	owned := s.OwnedOf(instance)
	// Only at the port the boat last docked at, as boarding is:
	// a port the owner would need the Sailing level to dock at
	// can't be stepped onto from a boat that sailed past it.
	if !ok || instance == nil || owned == nil || s.LastPortOf(*owned) != dockId || !s.mooredAt(instance, dock) {
		return "You can't disembark here."
	}
	guest.GetSailing().ReturnPoint = &ReturnPoint{X: dock.Landing.X, Y: dock.Landing.Y, Z: dock.Landing.Z}
	s.removeGuest(guest, instance, GuestLeaveDisembark)
	s.moveExpected(guest, dock.Landing)
	return ""
}

// BoardRefusal reports why the player can't board their boat at
// dockId (the boat in slot, or with no slot given the one
// moored here), or "" when they can. Live's message boxes: a
// boat docked at another port names that port, a sunk one is
// lost at sea. A nil slot means "no slot given".
func (s *Sailing) BoardRefusal(player *Player, dockId string, slot *int) string {
	if s.boats.GetBoatAboard(&player.Mobile) != nil {
		return "You're already on a boat."
	}
	if _, ok := s.docks[dockId]; !ok {
		return "You can't board a boat here."
	}
	if slot != nil {
		chosen := s.ownedBoat(player, *slot)
		if chosen != nil && s.dockedHere(*chosen, dockId) {
			return ""
		}
		return "You can't choose that boat at the moment."
	}
	if s.boatToBoard(player, dockId) != nil {
		return ""
	}
	boat := s.ActiveBoat(player)
	if boat == nil {
		state := player.GetSailing()
		if len(state.Boats) > 0 {
			boat = &state.Boats[0]
		}
	}
	if boat != nil && boat.Location.Kind == BoatLocationSunk {
		return LostAtSea
	}
	if boat != nil && boat.Location.Kind == BoatLocationDocked {
		if elsewhere, ok := s.docks[boat.Location.Dock]; ok {
			name := elsewhere.TheName
			if name == "" {
				name = elsewhere.ID
			}
			return "You cannot board your boat as it is currently docked at " + name + "."
		}
	}
	return "There isn't a boat here for you to board."
}

func (s *Sailing) dockedHere(boat OwnedBoat, dockId string) bool {
	return boat.Location.Kind == BoatLocationDocked && boat.Location.Dock == dockId
}

// boatToBoard is the player's boat moored at dockId, their
// active one first.
func (s *Sailing) boatToBoard(player *Player, dockId string) *OwnedBoat {
	if active := s.ActiveBoat(player); active != nil && s.dockedHere(*active, dockId) {
		return active
	}
	state := player.GetSailing()
	for i := range state.Boats {
		if s.dockedHere(state.Boats[i], dockId) {
			return &state.Boats[i]
		}
	}
	return nil
}

// Board boards the player's boat moored at dockId. It returns a
// message to show when they can't (BoardRefusal), otherwise "".
func (s *Sailing) Board(player *Player, dockId string, slot *int) string {
	if refusal := s.BoardRefusal(player, dockId, slot); refusal != "" {
		return refusal
	}
	dock := s.docks[dockId]
	var boat *OwnedBoat
	if slot != nil {
		boat = s.ownedBoat(player, *slot)
	} else {
		boat = s.boatToBoard(player, dockId)
	}
	var placement BoatPlacement
	placed := false
	if boat.Location.Kind == BoatLocationDocked && boat.Location.At != nil {
		at := boat.Location.At
		placement = BoatPlacement{FineX: at.FineX, FineY: at.FineY, Level: at.Level, Angle: at.Angle}
		placed = true
	}
	if !placed {
		if mooring, ok := dock.Moorings[boat.Type]; ok {
			placement = mooring
		} else {
			placement = dock.Mooring
		}
	}
	if !s.embark(player, boat, placement, dockId) {
		return "There's no room at sea right now."
	}
	s.dockedAt(player, dock, false)
	return ""
}

// Dock docks the boat the player is aboard at dockId (its
// buoy): the player stays aboard, the boat counts as this
// port's, and the port is where they return if they abandon
// it. It returns a message to show when they can't, otherwise
// "".
func (s *Sailing) Dock(player *Player, dockId string) string {
	dock, ok := s.docks[dockId]
	instance := s.InstanceAboard(player)
	boat := s.ActiveBoat(player)
	if !ok || instance == nil || boat == nil {
		return "You can't dock here."
	}
	boat.Location = atSea(instance, dockId)
	s.dockedAt(player, dock, true)
	s.returnGuestsTo(instance, dock)
	return ""
}

// Disembark takes the player off their boat at dockId, which
// docks it there (if it wasn't), where it is: boarding here
// again puts it back in the same place. It returns a message
// to show when they can't, otherwise "".
func (s *Sailing) Disembark(player *Player, dockId string) string {
	dock, ok := s.docks[dockId]
	instance := s.InstanceAboard(player)
	boat := s.ActiveBoat(player)
	if !ok || instance == nil || boat == nil {
		return "You can't disembark here."
	}
	at := BoatAt{FineX: instance.FineX, FineY: instance.FineY, Level: instance.Level, Angle: instance.Angle}
	boat.Location = DockedBoatLocation(dockId)
	boat.Location.At = &at
	s.dockedAt(player, dock, true)
	s.returnGuestsTo(instance, dock)
	s.leave(player, instance, LeaveDisembark)
	s.moveExpected(player, dock.Landing)
	return ""
}

// BoatNearDock reports whether the boat the player is aboard is
// close enough to dockId to step ashore there.
func (s *Sailing) BoatNearDock(player *Player, dockId string) bool {
	dock, ok := s.docks[dockId]
	instance := s.InstanceAboard(player)
	return ok && instance != nil && s.mooredAt(instance, dock)
}

// Escape is the helm's Escape: the boat sinks and the player
// returns to their last dock or buoy.
func (s *Sailing) Escape(player *Player) {
	instance := s.InstanceAboard(player)
	if instance == nil {
		return
	}
	s.sink(player, instance)
	s.moveExpected(player, s.ReturnLocation(player))
}

// RecoverRefusal reports why a shipwright at dockId can't
// recover the boat in slot (sunk, or docked at another port),
// or "" when they can. The texts are the boat selection
// interface's own (cache scripts).
func (s *Sailing) RecoverRefusal(player *Player, slot int, dockId string) string {
	boat := s.ownedBoat(player, slot)
	_, dockOK := s.docks[dockId]
	if boat == nil || !dockOK || boat.Location.Kind == BoatLocationAtSea {
		return "You can't choose that boat at the moment."
	}
	if boat.Location.Kind == BoatLocationDocked && boat.Location.Dock == dockId {
		return "That boat is already at the nearby dock. There's no need to recover it."
	}
	return ""
}

// Recover brings the boat in slot to dockId; the shipwright
// takes the fee. It returns a message to show when they can't,
// otherwise "".
func (s *Sailing) Recover(player *Player, slot int, dockId string) string {
	if refusal := s.RecoverRefusal(player, slot, dockId); refusal != "" {
		return refusal
	}
	boat := s.ownedBoat(player, slot)
	boat.Location = DockedBoatLocation(dockId)
	boat.Damage = 0
	return ""
}

// recordPositions keeps the saved position of each boat at sea
// current, so a save mid-voyage restores it. It runs after
// every boat has moved each tick (BoatManager.onAfterTick).
func (s *Sailing) recordPositions() {
	for instance, active := range s.instances {
		if owned := s.ownedBoat(active.player, active.slot); owned != nil {
			owned.Location = atSea(instance, s.LastPortOf(*owned))
		}
	}
}

// LastPortOf is the port a boat last docked at, for a boat at
// sea or docked. It returns "" for a sunk boat, or an at-sea
// boat whose port was never recorded.
func (s *Sailing) LastPortOf(boat OwnedBoat) string {
	if boat.Location.Kind == BoatLocationSunk {
		return ""
	}
	if !boat.Location.HasDock {
		return ""
	}
	return boat.Location.Dock
}

// OnLogout handles logging out at sea: the boat stays at sea —
// its position is recorded and the player's saved position
// becomes their return point, in case the boat can't be
// restored. The coordinator calls it from the logout path
// before the save (Player.ts:738 onLogout). The location sets
// are direct, exactly as TS: setLocation, not a teleport, so
// no teleport hook fires and the queue is untouched.
func (s *Sailing) OnLogout(player *Player) {
	// A guest leaves the deck they were visiting and logs out
	// ashore.
	if visited := s.GuestBoat(player); visited != nil {
		s.removeGuest(player, visited, GuestLeaveLogout)
		player.SetLocation(s.ReturnLocation(player))
	}
	// A player visiting a static ship has stepped off their
	// own boat's deck without disposing it; log that boat out
	// too so it cannot leak an instance.
	instance := s.InstanceFor(player)
	if instance == nil {
		return
	}
	if boat := s.ActiveBoat(player); boat != nil {
		boat.Location = atSea(instance, s.LastPortOf(*boat))
	}
	s.leave(player, instance, LeaveLogout)
	player.SetLocation(s.ReturnLocation(player))
}

// OnLogin puts a player who logged out at sea back aboard their
// boat. The coordinator calls it from the login path after the
// login extension, before the login emit and bootstrap
// (NetworkBuilder.ts:1692).
func (s *Sailing) OnLogin(player *Player) {
	if boat := s.ActiveBoat(player); boat != nil && boat.Location.Kind == BoatLocationAtSea {
		placement := BoatPlacement{FineX: boat.Location.FineX, FineY: boat.Location.FineY, Level: boat.Location.Level, Angle: boat.Location.Angle}
		if s.embark(player, boat, placement, s.LastPortOf(*boat)) {
			return
		}
	}
	// A save from a deck with no boat to go back to (such as
	// sailing data lost in a crash).
	location := player.Location()
	if IsDeckTile(location.X, location.Y) {
		player.SetLocation(s.ReturnLocation(player))
	}
}

// dockedAt records a dock as the player's last (and return
// point), as boarding or docking there does. moored also
// records it in the moored-docks history.
func (s *Sailing) dockedAt(player *Player, dock SailingDock, moored bool) {
	state := player.GetSailing()
	state.ReturnPoint = &ReturnPoint{X: dock.Landing.X, Y: dock.Landing.Y, Z: dock.Landing.Z}
	state.LastDock = &dock.ID
	if dock.MooringPoint {
		state.LastMooringPoint = &dock.ID
	} else {
		state.LastStandardDock = &dock.ID
	}
	if moored {
		for _, id := range state.MooredDocks {
			if id == dock.ID {
				return
			}
		}
		state.MooredDocks = append(state.MooredDocks, dock.ID)
	}
}

// embark spawns the boat at placement and puts the player on
// its deck. It reports false when the type is unregistered or
// no entity index is free. dockId is the port the boat counts
// as, when known ("" records none).
func (s *Sailing) embark(player *Player, boat *OwnedBoat, placement BoatPlacement, dockId string) bool {
	spec, ok := s.SpecFor(*boat)
	if !ok {
		return false
	}
	instance := s.boats.Spawn(player.GetIndex(), spec, placement)
	if instance == nil {
		return false
	}
	s.instances[instance] = &activeInstance{player: player, slot: boat.Slot}
	slot := boat.Slot
	player.GetSailing().ActiveBoatSlot = &slot
	boat.Location = atSea(instance, dockId)
	s.boats.GetDeck(instance).Enter(player)
	s.moveExpected(player, Location{X: instance.DeckBaseX() + spec.BoardingTile.X, Y: instance.DeckBaseY() + spec.BoardingTile.Y, Z: 0})
	s.emitEvent(player, "sailing:boarded", SailingBoardedPayload{Player: player, Boat: instance, Owned: *boat})
	return true
}

func (s *Sailing) sink(player *Player, instance *Boat) {
	if boat := s.ActiveBoat(player); boat != nil {
		boat.Location = SunkBoatLocation()
	}
	s.leave(player, instance, LeaveSunk)
}

// mooredAt reports whether a boat is close enough to a dock to
// step between them: measured from a port's landing, and from
// a mooring point itself, since its landing is further inland.
func (s *Sailing) mooredAt(instance *Boat, dock SailingDock) bool {
	tile := instance.WorldTile()
	from := dock.Landing
	if dock.MooringPoint && dock.Gangplank != nil {
		from = *dock.Gangplank
	}
	dx := absInt(tile.X - from.X)
	dy := absInt(tile.Y - from.Y)
	return max(dx, dy) <= MooredRange
}

// returnGuestsTo moves where the boat's guests go back to when
// it docks, as docking does for its owner.
func (s *Sailing) returnGuestsTo(instance *Boat, dock SailingDock) {
	for _, guest := range s.GuestsOf(instance) {
		guest.GetSailing().ReturnPoint = &ReturnPoint{X: dock.Landing.X, Y: dock.Landing.Y, Z: dock.Landing.Z}
	}
}

// removeGuest takes a guest off a deck; the caller moves them
// (they may already be on their way).
func (s *Sailing) removeGuest(guest *Player, instance *Boat, reason GuestLeaveReason) {
	atHelm := instance.HelmPlayerID != nil && *instance.HelmPlayerID == guest.GetIndex()
	if atHelm {
		instance.HelmPlayerID = nil
	}
	deck := s.boats.GetDeck(instance)
	if deck != nil && deck.HasMember(guest) {
		deck.Leave(guest, false)
	}
	s.emitEvent(guest, "sailing:guest-left", SailingGuestLeftPayload{Player: guest, Boat: instance, Reason: reason, AtHelm: atHelm})
}

// leave takes the player off the deck and removes the boat
// from the sea. A boat doesn't outlive its owner being aboard,
// so anyone visiting is put ashore at their return point first.
func (s *Sailing) leave(player *Player, instance *Boat, reason LeaveReason) {
	for _, guest := range s.GuestsOf(instance) {
		s.removeGuest(guest, instance, GuestLeaveEvicted)
		s.moveExpected(guest, s.ReturnLocation(guest))
	}
	delete(s.instances, instance)
	atHelm := instance.HelmPlayerID != nil && *instance.HelmPlayerID == player.GetIndex()
	if atHelm {
		instance.HelmPlayerID = nil
	}
	deck := s.boats.GetDeck(instance)
	if deck != nil && deck.HasMember(player) {
		deck.Leave(player, false)
	}
	s.boats.Dispose(instance)
	s.emitEvent(player, "sailing:left", SailingLeftPayload{Player: player, Boat: instance, Reason: reason, AtHelm: atHelm})
}

// onTeleport is the before-teleport hook: a guest teleporting
// off someone's boat just leaves it; teleporting off their own
// boat at sea sinks it. A teleport onto a deck tile of the
// same boat (boarding moves, deck-to-deck steps) is not a
// leave. Moves Sailing itself makes are skipped via
// expectedMove.
func (s *Sailing) onTeleport(player *Player, target Location) {
	if player == s.expectedMove {
		return
	}
	// A guest teleporting off someone's boat just leaves it.
	if visited := s.GuestBoat(player); visited != nil {
		if !visited.ContainsDeckTile(target.X, target.Y) {
			s.removeGuest(player, visited, GuestLeaveTeleport)
		}
		return
	}
	instance := s.InstanceAboard(player)
	if instance == nil || instance.ContainsDeckTile(target.X, target.Y) {
		return
	}
	// Teleporting off the boat at sea sinks it.
	s.sink(player, instance)
}

// moveExpected moves a player as Sailing's own move, so the
// teleport hook doesn't take it for a teleport.
func (s *Sailing) moveExpected(player *Player, target Location) {
	s.expectedMove = player
	defer func() { s.expectedMove = nil }()
	MovePlayerTo(player, target)
}

// ReturnLocation is where Escape and logout-at-sea send a
// player: their return point, or the world's spawn when they
// have none.
func (s *Sailing) ReturnLocation(player *Player) Location {
	if point := player.GetSailing().ReturnPoint; point != nil {
		return Location{X: point.X, Y: point.Y, Z: point.Z}
	}
	return s.defaultLocation
}

// atSea is the at-sea location for a live instance, with the
// port it counts as when dock is non-empty.
func atSea(instance *Boat, dock string) BoatLocation {
	return AtSeaBoatLocation(instance.FineX, instance.FineY, instance.Level, instance.Angle, dock)
}

// --- teleport seam (Mobile.onBeforeTeleport / Mobile.moveTo)
// ---
//
// TS keeps the teleport listeners on Mobile itself
// (Mobile.ts:173, onBeforeTeleport) and fires them from
// notifyTeleport, which moveTo calls. Go has no teleport path
// yet (no MoveTo exists), so the seam lives here, minimal and
// following the world.go Hooks pattern: listeners register
// with OnBeforeTeleport, and every teleport goes through
// MovePlayerTo, which fires them. The listener is player-typed
// — Go has no NPC teleport path, and *Mobile carries no
// back-pointer to *Player — a narrowing to revisit if NPC
// teleports land. Listeners are tick-goroutine-only, like the
// moves that fire them; registration happens at boot, before
// ticking starts.

type beforeTeleportListener func(player *Player, target Location)

var beforeTeleportListeners []beforeTeleportListener

// OnBeforeTeleport registers a listener that runs before a
// player is teleported (moved with MovePlayerTo), with the
// destination. Sailing.Initialize registers its sink-on-leave
// hook here.
func OnBeforeTeleport(listener func(player *Player, target Location)) {
	beforeTeleportListeners = append(beforeTeleportListeners, listener)
}

// fireBeforeTeleport runs the registered listeners, in
// registration order.
func fireBeforeTeleport(player *Player, target Location) {
	for _, listener := range beforeTeleportListeners {
		listener(player, target)
	}
}

// MovePlayerTo teleports a player to target: the Go form of
// Mobile.moveTo (Mobile.ts:194-204) — notify the teleport
// listeners, reset the queue, set the location, mark the sync
// placement, and refresh the region baseline. It does NOT
// clear following/interaction: Go has no exported clear for
// them (combat/following are NOT STARTED), so that half of
// notifyTeleport is a gap to close when those systems land.
// Callers that must not fire the teleport hook (Sailing's
// logout handling) set the location directly, exactly as TS
// does.
func MovePlayerTo(player *Player, target Location) {
	fireBeforeTeleport(player, target)
	if queue := player.MovementQueue(); queue != nil {
		queue.Reset(true)
	}
	player.SetLocation(target)
	player.NeedsPlacement = true
	player.ResetMovementQueue = true
	if queue := player.MovementQueue(); queue != nil {
		queue.HandleRegionChange()
	}
}
