package game

import (
	"testing"
)

func testSailing(t *testing.T) (*Sailing, *BoatManager) {
	t.Helper()
	prevListeners := beforeTeleportListeners
	t.Cleanup(func() {
		beforeTeleportListeners = prevListeners
	})
	m := NewBoatManager(nil)
	s := NewSailing(m, Location{X: 3089, Y: 3524, Z: 0})
	s.RegisterBoatType(testBoatSpec())
	s.RegisterDock(SailingDock{
		ID:      "catherby",
		Mooring: BoatPlacement{FineX: 3000 * FineUnitsPerTile, FineY: 3000 * FineUnitsPerTile, Angle: 0},
		Landing: Location{X: 3005, Y: 3005, Z: 0},
		TheName: "Catherby",
	})
	s.RegisterDock(SailingDock{
		ID:      "ardougne",
		Mooring: BoatPlacement{FineX: 3100 * FineUnitsPerTile, FineY: 3100 * FineUnitsPerTile, Angle: 0},
		Landing: Location{X: 3105, Y: 3105, Z: 0},
		TheName: "Ardougne",
	})
	s.RegisterDock(SailingDock{
		ID:           "island",
		MooringPoint: true,
		Mooring:      BoatPlacement{FineX: 3000 * FineUnitsPerTile, FineY: 3000 * FineUnitsPerTile, Angle: 0},
		Landing:      Location{X: 2900, Y: 3500, Z: 0},
		Gangplank:    &Location{X: 3010, Y: 3000, Z: 0},
		TheName:      "the island",
	})
	s.Initialize()
	return s, m
}

func sailingPlayer(name string, index int) *Player {
	p := NewPlayer(name, Location{X: 3005, Y: 3005, Z: 0})
	p.SetIndex(index)
	return p
}

type sailingEvents struct {
	names   []string
	payload []any
}

func (e *sailingEvents) install(p *Player) {
	p.EmitCustomEvent = func(name string, payload any) {
		e.names = append(e.names, name)
		e.payload = append(e.payload, payload)
	}
}

func (e *sailingEvents) has(name string) bool {
	for _, n := range e.names {
		if n == name {
			return true
		}
	}
	return false
}

func TestSailingGiveBoat(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	boat, ok := s.GiveBoat(p, "sloop", "catherby", BoatName{1, 2, 3})
	if !ok {
		t.Fatal("giveBoat must succeed")
	}
	if boat.Slot != 0 || boat.Type != "sloop" || boat.Name != (BoatName{1, 2, 3}) {
		t.Errorf("boat = %+v", boat)
	}
	if boat.Location.Kind != BoatLocationDocked || boat.Location.Dock != "catherby" {
		t.Errorf("location = %+v, want docked at catherby", boat.Location)
	}
	if boat.Hitpoints != 0 || boat.Damage != 0 || boat.Parts != BaseParts() {
		t.Errorf("fresh boat = %+v", boat)
	}
	if slot := p.GetSailing().ActiveBoatSlot; slot == nil || *slot != 0 {
		t.Errorf("active slot = %v, want 0", slot)
	}
	if _, ok := s.GiveBoat(p, "unknown", "catherby", BoatName{}); ok {
		t.Error("unknown type must fail")
	}
	if _, ok := s.GiveBoat(p, "sloop", "nowhere", BoatName{}); ok {
		t.Error("unknown dock must fail")
	}
	// Fill to the cap: slots 1-4, then no more.
	for i := 0; i < 4; i++ {
		if _, ok := s.GiveBoat(p, "sloop", "catherby", BoatName{}); !ok {
			t.Fatalf("boat %d must succeed", i+2)
		}
	}
	if _, ok := s.GiveBoat(p, "sloop", "catherby", BoatName{}); ok {
		t.Error("a sixth boat must fail")
	}
	// Removing slot 2 frees it for the next boat.
	if !s.RemoveOwnedBoat(p, 2) {
		t.Fatal("remove must succeed")
	}
	boat, ok = s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if !ok || boat.Slot != 2 {
		t.Errorf("replacement boat = %+v, %v, want slot 2", boat, ok)
	}
	// Boats stay sorted by slot.
	slots := []int{}
	for _, b := range p.GetSailing().Boats {
		slots = append(slots, b.Slot)
	}
	for i := 1; i < len(slots); i++ {
		if slots[i] <= slots[i-1] {
			t.Errorf("slots %v must be sorted", slots)
		}
	}
}

func TestSailingBoardRefusal(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	if got := s.BoardRefusal(p, "catherby", nil); got != "There isn't a boat here for you to board." {
		t.Errorf("no boats: %q", got)
	}
	if got := s.BoardRefusal(p, "nowhere", nil); got != "You can't board a boat here." {
		t.Errorf("unknown dock: %q", got)
	}
	s.GiveBoat(p, "sloop", "ardougne", BoatName{})
	if got := s.BoardRefusal(p, "catherby", nil); got != "You cannot board your boat as it is currently docked at Ardougne." {
		t.Errorf("docked elsewhere: %q", got)
	}
	slot := 0
	if got := s.BoardRefusal(p, "catherby", &slot); got != "You can't choose that boat at the moment." {
		t.Errorf("wrong-slot choice: %q", got)
	}
	if got := s.BoardRefusal(p, "ardougne", &slot); got != "" {
		t.Errorf("right-slot choice: %q, want empty", got)
	}
	// A sunk boat is lost at sea.
	p.GetSailing().Boats[0].Location = SunkBoatLocation()
	if got := s.BoardRefusal(p, "catherby", nil); got != LostAtSea {
		t.Errorf("sunk: %q, want LOST_AT_SEA", got)
	}
	// Aboard: already on a boat.
	p.GetSailing().Boats[0].Location = DockedBoatLocation("catherby")
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	if got := s.BoardRefusal(p, "catherby", nil); got != "You're already on a boat." {
		t.Errorf("aboard: %q", got)
	}
}

func TestSailingBoardAndDisembark(t *testing.T) {
	s, m := testSailing(t)
	p := sailingPlayer("owner", 1)
	events := &sailingEvents{}
	events.install(p)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	instance := s.InstanceAboard(p)
	if instance == nil {
		t.Fatal("player must be aboard after boarding")
	}
	if instance.FineX != 3000*FineUnitsPerTile || instance.FineY != 3000*FineUnitsPerTile {
		t.Errorf("boat spawned at (%d, %d), want the catherby mooring", instance.FineX, instance.FineY)
	}
	// On the boarding tile of the deck.
	if want := (Location{X: 9604, Y: 9608, Z: 0}); p.Location() != want {
		t.Errorf("player at %+v, want %+v", p.Location(), want)
	}
	boat := s.ActiveBoat(p)
	if boat.Location.Kind != BoatLocationAtSea || boat.Location.Dock != "catherby" {
		t.Errorf("owned location = %+v, want at sea at catherby", boat.Location)
	}
	state := p.GetSailing()
	if state.ReturnPoint == nil || *state.ReturnPoint != (ReturnPoint{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("returnPoint = %+v, want the landing", state.ReturnPoint)
	}
	if state.LastDock == nil || *state.LastDock != "catherby" || state.LastStandardDock == nil {
		t.Errorf("last dock = %v/%v", state.LastDock, state.LastStandardDock)
	}
	if !events.has("sailing:boarded") {
		t.Errorf("events = %v, want sailing:boarded", events.names)
	}
	if s.GuestBoat(p) != nil || s.OwnerOf(instance) != p || s.OwnedOf(instance) == nil {
		t.Error("instance queries must resolve the owner")
	}

	// Disembark: docked where it is, player ashore, boat gone.
	if msg := s.Disembark(p, "catherby"); msg != "" {
		t.Fatalf("disembark: %q", msg)
	}
	if s.InstanceAboard(p) != nil || m.GetBoat(instance.EntityIndex) != nil {
		t.Error("the boat must be disposed after disembarking")
	}
	if p.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("player at %+v, want the landing", p.Location())
	}
	boat = s.ActiveBoat(p)
	if boat.Location.Kind != BoatLocationDocked || boat.Location.At == nil ||
		boat.Location.At.FineX != 3000*FineUnitsPerTile {
		t.Errorf("owned location = %+v, want docked with the recorded spot", boat.Location)
	}
	if len(state.MooredDocks) != 1 || state.MooredDocks[0] != "catherby" {
		t.Errorf("mooredDocks = %v", state.MooredDocks)
	}
	if !events.has("sailing:left") {
		t.Errorf("events = %v, want sailing:left", events.names)
	}
	payload := events.payload[len(events.payload)-1].(SailingLeftPayload)
	if payload.Reason != LeaveDisembark || payload.Boat != instance {
		t.Errorf("left payload = %+v", payload)
	}

	// Boarding again puts the boat back in the same place.
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("re-board: %q", msg)
	}
	if again := s.InstanceAboard(p); again == nil || again.FineX != 3000*FineUnitsPerTile {
		t.Errorf("re-boarded boat at %v", again)
	}
}

func TestSailingBoardNoRoom(t *testing.T) {
	s, m := testSailing(t)
	p := sailingPlayer("owner", 1)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	for i := FirstBoatEntityIndex; i <= LastBoatEntityIndex; i++ {
		m.boats[i] = &activeBoat{}
	}
	if msg := s.Board(p, "catherby", nil); msg != "There's no room at sea right now." {
		t.Errorf("board with no indices: %q", msg)
	}
}

func TestSailingDock(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	if msg := s.Dock(p, "ardougne"); msg != "" {
		t.Fatalf("dock: %q", msg)
	}
	// Still aboard; the boat counts as Ardougne's.
	if s.InstanceAboard(p) == nil {
		t.Error("docking must keep the player aboard")
	}
	boat := s.ActiveBoat(p)
	if boat.Location.Kind != BoatLocationAtSea || boat.Location.Dock != "ardougne" {
		t.Errorf("location = %+v, want at sea at ardougne", boat.Location)
	}
	if state := p.GetSailing(); len(state.MooredDocks) != 1 || state.MooredDocks[0] != "ardougne" {
		t.Errorf("mooredDocks = %v", state.MooredDocks)
	}
	if msg := s.Dock(p, "nowhere"); msg != "You can't dock here." {
		t.Errorf("unknown dock: %q", msg)
	}
	ashore := sailingPlayer("ashore", 2)
	if msg := s.Dock(ashore, "catherby"); msg != "You can't dock here." {
		t.Errorf("ashore dock: %q", msg)
	}
}

func TestSailingGuests(t *testing.T) {
	s, _ := testSailing(t)
	owner := sailingPlayer("owner", 1)
	guest := sailingPlayer("guest", 2)
	guestEvents := &sailingEvents{}
	guestEvents.install(guest)
	s.GiveBoat(owner, "sloop", "catherby", BoatName{})

	// Owner not aboard yet.
	if got := s.GuestRefusal(guest, owner, "catherby", 10); got != GuestRefusalNotAboard {
		t.Errorf("refusal = %q, want not-aboard", got)
	}
	if msg := s.Board(owner, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	// Wrong port: the boat's last port is catherby.
	if got := s.GuestRefusal(guest, owner, "ardougne", 10); got != GuestRefusalNotHere {
		t.Errorf("refusal = %q, want not-here", got)
	}
	// Full: capacity 1, owner aboard.
	if got := s.GuestRefusal(guest, owner, "catherby", 1); got != GuestRefusalFull {
		t.Errorf("refusal = %q, want full", got)
	}
	// Board as guest.
	if refusal := s.BoardAsGuest(guest, owner, "catherby", 10); refusal != "" {
		t.Fatalf("boardAsGuest: %q", refusal)
	}
	if s.GuestBoat(guest) == nil {
		t.Fatal("guest must be aboard the owner's boat")
	}
	if guests := s.GuestsOf(s.InstanceAboard(owner)); len(guests) != 1 || guests[0] != guest {
		t.Errorf("guests = %v", guests)
	}
	if rp := guest.GetSailing().ReturnPoint; rp == nil || rp.X != 3005 {
		t.Errorf("guest returnPoint = %+v, want the landing", rp)
	}
	if !guestEvents.has("sailing:guest-boarded") {
		t.Errorf("guest events = %v", guestEvents.names)
	}
	// Aboard already: cannot guest-board again.
	if got := s.GuestRefusal(guest, owner, "catherby", 10); got != GuestRefusalAboard {
		t.Errorf("refusal = %q, want aboard", got)
	}
	// Disembark at the wrong port fails; at the right port
	// the guest steps ashore.
	if msg := s.DisembarkGuest(guest, "ardougne"); msg != "You can't disembark here." {
		t.Errorf("wrong-port disembark: %q", msg)
	}
	if msg := s.DisembarkGuest(guest, "catherby"); msg != "" {
		t.Fatalf("disembarkGuest: %q", msg)
	}
	if s.GuestBoat(guest) != nil || guest.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("guest at %+v, aboard %v", guest.Location(), s.GuestBoat(guest))
	}
	if !guestEvents.has("sailing:guest-left") {
		t.Errorf("guest events = %v, want sailing:guest-left", guestEvents.names)
	}
}

func TestSailingEscapeAndRecover(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	s.Escape(p)
	if s.InstanceAboard(p) != nil {
		t.Error("escape must sink the boat")
	}
	boat := s.ActiveBoat(p)
	if boat.Location.Kind != BoatLocationSunk {
		t.Errorf("location = %+v, want sunk", boat.Location)
	}
	if p.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("escape sent the player to %+v, want the return point", p.Location())
	}
	// A sunk boat is recoverable at any dock (it is not
	// already docked anywhere).
	boat.Damage = 25
	if got := s.RecoverRefusal(p, 0, "catherby"); got != "" {
		t.Errorf("sunk refusal: %q, want empty", got)
	}
	if msg := s.Recover(p, 0, "ardougne"); msg != "" {
		t.Fatalf("recover: %q", msg)
	}
	if boat.Location.Kind != BoatLocationDocked || boat.Location.Dock != "ardougne" || boat.Damage != 0 {
		t.Errorf("recovered = %+v", boat.Location)
	}
	if got := s.RecoverRefusal(p, 0, "ardougne"); got != "That boat is already at the nearby dock. There's no need to recover it." {
		t.Errorf("already-here refusal: %q", got)
	}
	if got := s.RecoverRefusal(p, 9, "ardougne"); got != "You can't choose that boat at the moment." {
		t.Errorf("unknown slot refusal: %q", got)
	}
	// At sea: cannot recover.
	if msg := s.Board(p, "ardougne", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	if got := s.RecoverRefusal(p, 0, "catherby"); got != "You can't choose that boat at the moment." {
		t.Errorf("at-sea refusal: %q", got)
	}
}

func TestSailingLoginLogout(t *testing.T) {
	s, m := testSailing(t)
	p := sailingPlayer("owner", 1)
	events := &sailingEvents{}
	events.install(p)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	instance := s.InstanceAboard(p)
	instance.FineX += 256
	s.OnLogout(p)
	if m.GetBoat(instance.EntityIndex) != nil {
		t.Error("logout must dispose the instance")
	}
	boat := s.ActiveBoat(p)
	if boat.Location.Kind != BoatLocationAtSea || boat.Location.FineX != 3000*FineUnitsPerTile+256 {
		t.Errorf("logout recorded %+v", boat.Location)
	}
	if p.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("logout set the location to %+v, want the return point", p.Location())
	}
	if !events.has("sailing:left") {
		t.Errorf("events = %v, want sailing:left", events.names)
	}
	// Login puts the player back aboard at the saved position.
	s.OnLogin(p)
	again := s.InstanceAboard(p)
	if again == nil || again.FineX != 3000*FineUnitsPerTile+256 {
		t.Errorf("login instance = %v", again)
	}
	// The re-embarked boat takes the next entity index and
	// deck scene: the player lands on its boarding tile.
	if want := (Location{X: again.DeckBaseX() + 4, Y: again.DeckBaseY() + 8, Z: 0}); p.Location() != want {
		t.Errorf("login put the player at %+v, want %+v", p.Location(), want)
	}

	// A save from a deck with no boat to go back to lands at
	// the return point.
	p2 := sailingPlayer("lost", 2)
	p2.SetLocation(Location{X: 9604, Y: 9608, Z: 0})
	p2.GetSailing().ReturnPoint = &ReturnPoint{X: 3100, Y: 3100, Z: 0}
	s.OnLogin(p2)
	if p2.Location() != (Location{X: 3100, Y: 3100, Z: 0}) {
		t.Errorf("deck save without a boat landed at %+v", p2.Location())
	}

	// A guest logging out leaves the deck and logs out ashore.
	guest := sailingPlayer("guest", 3)
	if refusal := s.BoardAsGuest(guest, p, "catherby", 10); refusal != "" {
		t.Fatalf("boardAsGuest: %q", refusal)
	}
	s.OnLogout(guest)
	if s.GuestBoat(guest) != nil || guest.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("guest logout: at %+v, aboard %v", guest.Location(), s.GuestBoat(guest))
	}
	// The owner's boat survives the guest's logout.
	if s.InstanceAboard(p) == nil {
		t.Error("the owner's boat must survive a guest logout")
	}
}

func TestSailingTeleportSinks(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	events := &sailingEvents{}
	events.install(p)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	// Teleporting onto another deck tile of the same boat is
	// not a leave.
	MovePlayerTo(p, Location{X: 9605, Y: 9608, Z: 0})
	if s.InstanceAboard(p) == nil {
		t.Fatal("a deck-tile teleport must not sink the boat")
	}
	// Teleporting ashore sinks it.
	MovePlayerTo(p, Location{X: 3200, Y: 3200, Z: 0})
	if s.InstanceAboard(p) != nil {
		t.Error("teleporting ashore must sink the boat")
	}
	if boat := s.ActiveBoat(p); boat.Location.Kind != BoatLocationSunk {
		t.Errorf("location = %+v, want sunk", boat.Location)
	}
	if !events.has("sailing:left") {
		t.Errorf("events = %v, want sailing:left", events.names)
	}
	payload := events.payload[len(events.payload)-1].(SailingLeftPayload)
	if payload.Reason != LeaveSunk {
		t.Errorf("leave reason = %q, want sunk", payload.Reason)
	}

	// A guest teleporting off just leaves.
	p2 := sailingPlayer("owner2", 2)
	s.GiveBoat(p2, "sloop", "catherby", BoatName{})
	if msg := s.Board(p2, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	guest := sailingPlayer("guest", 3)
	guestEvents := &sailingEvents{}
	guestEvents.install(guest)
	if refusal := s.BoardAsGuest(guest, p2, "catherby", 10); refusal != "" {
		t.Fatalf("boardAsGuest: %q", refusal)
	}
	MovePlayerTo(guest, Location{X: 3200, Y: 3200, Z: 0})
	if s.GuestBoat(guest) != nil {
		t.Error("a teleporting guest must leave the boat")
	}
	if s.InstanceAboard(p2) == nil {
		t.Error("the owner's boat must survive a guest teleport")
	}
	if !guestEvents.has("sailing:guest-left") {
		t.Errorf("guest events = %v", guestEvents.names)
	}
}

func TestSailingRecordPositions(t *testing.T) {
	s, m := testSailing(t)
	p := sailingPlayer("owner", 1)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	instance := s.InstanceAboard(p)
	instance.FineX += 512
	m.Tick()
	boat := s.ActiveBoat(p)
	if boat.Location.FineX != 3000*FineUnitsPerTile+512 {
		t.Errorf("recorded fineX = %d", boat.Location.FineX)
	}
	if boat.Location.Dock != "catherby" {
		t.Errorf("recorded dock = %q, want catherby", boat.Location.Dock)
	}
}

func TestSailingRemoveOwnedBoat(t *testing.T) {
	s, _ := testSailing(t)
	owner := sailingPlayer("owner", 1)
	guest := sailingPlayer("guest", 2)
	s.GiveBoat(owner, "sloop", "catherby", BoatName{})
	if msg := s.Board(owner, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	if refusal := s.BoardAsGuest(guest, owner, "catherby", 10); refusal != "" {
		t.Fatalf("boardAsGuest: %q", refusal)
	}
	if !s.RemoveOwnedBoat(owner, 0) {
		t.Fatal("remove must succeed")
	}
	if len(owner.GetSailing().Boats) != 0 || owner.GetSailing().ActiveBoatSlot != nil {
		t.Errorf("state = %+v, want no boats", owner.GetSailing())
	}
	if s.InstanceAboard(owner) != nil {
		t.Error("the instance must be gone")
	}
	if owner.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("owner at %+v, want the return point", owner.Location())
	}
	if guest.Location() != (Location{X: 3005, Y: 3005, Z: 0}) {
		t.Errorf("guest at %+v, want the return point (evicted)", guest.Location())
	}
	if s.RemoveOwnedBoat(owner, 0) {
		t.Error("removing twice must fail")
	}
}

func TestSailingSpecResolver(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	boat, _ := s.GiveBoat(p, "sloop", "catherby", BoatName{})
	spec, ok := s.SpecFor(boat)
	if !ok || spec.ConfigID != 7 {
		t.Errorf("base spec = %+v, %v", spec, ok)
	}
	s.SetSpecResolver(func(owned OwnedBoat, base BoatSpec) BoatSpec {
		base.ConfigID = 99
		return base
	})
	spec, ok = s.SpecFor(boat)
	if !ok || spec.ConfigID != 99 {
		t.Errorf("resolved spec = %+v, %v", spec, ok)
	}
	if _, ok := s.SpecFor(OwnedBoat{Type: "unknown"}); ok {
		t.Error("an unknown type must have no spec")
	}
}

func TestSailingLastPortOf(t *testing.T) {
	s, _ := testSailing(t)
	if got := s.LastPortOf(OwnedBoat{Location: DockedBoatLocation("catherby")}); got != "catherby" {
		t.Errorf("docked port = %q", got)
	}
	if got := s.LastPortOf(OwnedBoat{Location: AtSeaBoatLocation(0, 0, 0, 0, "ardougne")}); got != "ardougne" {
		t.Errorf("at-sea port = %q", got)
	}
	if got := s.LastPortOf(OwnedBoat{Location: AtSeaBoatLocation(0, 0, 0, 0, "")}); got != "" {
		t.Errorf("portless at-sea port = %q", got)
	}
	if got := s.LastPortOf(OwnedBoat{Location: SunkBoatLocation()}); got != "" {
		t.Errorf("sunk port = %q", got)
	}
}

func TestSailingBoatNearDock(t *testing.T) {
	s, _ := testSailing(t)
	p := sailingPlayer("owner", 1)
	s.GiveBoat(p, "sloop", "catherby", BoatName{})
	if msg := s.Board(p, "catherby", nil); msg != "" {
		t.Fatalf("board: %q", msg)
	}
	// The boat sits at (3000, 3000): 5 from Catherby's
	// landing, 10 from the island's gangplank (its landing
	// is far inland, so the gangplank is what counts).
	if !s.BoatNearDock(p, "catherby") {
		t.Error("the boat must be near Catherby")
	}
	if !s.BoatNearDock(p, "island") {
		t.Error("the boat must be near the island's gangplank")
	}
	if s.BoatNearDock(p, "ardougne") {
		t.Error("the boat must not be near Ardougne")
	}
	ashore := sailingPlayer("ashore", 2)
	if s.BoatNearDock(ashore, "catherby") {
		t.Error("an ashore player has no boat near any dock")
	}
}

func TestSailingInitializeIdempotent(t *testing.T) {
	s, _ := testSailing(t)
	before := len(beforeTeleportListeners)
	s.Initialize()
	if len(beforeTeleportListeners) != before {
		t.Errorf("second Initialize registered %d listeners, want %d", len(beforeTeleportListeners), before)
	}
}

func TestSailingGetDock(t *testing.T) {
	s, _ := testSailing(t)
	dock, ok := s.GetDock("catherby")
	if !ok || dock.ID != "catherby" || dock.TheName != "Catherby" {
		t.Errorf("dock = %+v, %v", dock, ok)
	}
	if _, ok := s.GetDock("nowhere"); ok {
		t.Error("an unknown dock must report false")
	}
}

func TestMovePlayerToFiresTeleportHook(t *testing.T) {
	prev := beforeTeleportListeners
	t.Cleanup(func() { beforeTeleportListeners = prev })
	beforeTeleportListeners = nil
	fired := 0
	OnBeforeTeleport(func(player *Player, target Location) { fired++ })
	p := sailingPlayer("p", 1)
	MovePlayerTo(p, Location{X: 3200, Y: 3200, Z: 0})
	if fired != 1 {
		t.Errorf("teleport hook fired %d times, want 1", fired)
	}
	if p.Location() != (Location{X: 3200, Y: 3200, Z: 0}) {
		t.Errorf("player at %+v", p.Location())
	}
	if !p.NeedsPlacement || !p.ResetMovementQueue {
		t.Error("a teleport must mark placement + queue reset")
	}
}
