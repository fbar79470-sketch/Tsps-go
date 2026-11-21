package game

import "testing"

// Projected-NPC walk-to tests: walkToProjectedNpc
// (MovementQueue.ts:1619-1657) — a main-world NPC seen from a
// boat deck, resolved from the boat's root tile.

func projectedSetup(t *testing.T) (*BoatManager, *Boat, *Player, *fakeSession) {
	t.Helper()
	m := NewBoatManager(nil)
	boat := m.Spawn(1, testBoatSpec(), testPlacement())
	p := NewPlayer("aboard", Location{X: boat.DeckBaseX() + 4, Y: boat.DeckBaseY() + 8, Z: 0})
	p.SetIndex(1)
	// The queue reads the manager from its own install, as
	// the World/coordinator install it in production.
	p.MovementQueue().SetBoatManager(m)
	fs := newFakeSession()
	p.Session = fs
	m.GetDeck(boat).Enter(p)
	return m, boat, p, fs
}

func projectedNPC(t *testing.T, loc Location) *NPC {
	t.Helper()
	n := NewNPC(1, loc)
	n.SetRegistered(true)
	return n
}

func TestWalkToProjectedNpcImmediate(t *testing.T) {
	_, _, p, _ := projectedSetup(t)
	// The NPC stands on the boat's root tile: reached at once.
	npc := projectedNPC(t, Location{X: 3000, Y: 3000, Z: 0})
	ran := false
	p.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !ran {
		t.Error("a reached projected NPC must run the action at once")
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("no walk-to must pend after an immediate resolve")
	}
}

func TestWalkToProjectedNpcWalksDeck(t *testing.T) {
	_, boat, p, _ := projectedSetup(t)
	// The NPC is 2 tiles east of the root: out of reach (1)
	// from the boarding tile, in reach from the eastern
	// deck tile the walk ends on.
	npc := projectedNPC(t, Location{X: 3002, Y: 3000, Z: 0})
	ran := false
	q := p.MovementQueue()
	q.WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !q.WalkToPending() {
		t.Fatal("a walk-to must pend")
	}
	if q.walkTo == nil || q.walkTo.projected == nil {
		t.Fatal("the pending walk-to must be the projected shape")
	}
	// Tick until the action runs or the walk fails.
	for cycle := int64(1); cycle <= 10 && !ran; cycle++ {
		q.Process(cycle)
	}
	if !ran {
		t.Fatalf("the action never ran; player at %+v, pending=%v", p.Location(), q.WalkToPending())
	}
	// The player walked east along the deck, toward the NPC.
	if p.Location().X <= boat.DeckBaseX()+4 {
		t.Errorf("player at %+v, want east of the boarding tile", p.Location())
	}
}

func TestWalkToProjectedNpcNoNearerTile(t *testing.T) {
	_, boat, p, fs := projectedSetup(t)
	// The player already stands on the easternmost walkable
	// deck tile; the NPC is far east, out of reach, and no
	// deck tile comes nearer.
	p.SetLocation(Location{X: boat.DeckBaseX() + 5, Y: boat.DeckBaseY() + 8, Z: 0})
	npc := projectedNPC(t, Location{X: 3010, Y: 3000, Z: 0})
	ran := false
	p.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if ran {
		t.Error("the action must not run")
	}
	if p.MovementQueue().WalkToPending() {
		t.Error("no walk-to must pend when the deck takes the player no nearer")
	}
	if len(fs.messages) != 1 || fs.messages[0] != "I can't reach that!" {
		t.Errorf("messages = %v, want [I can't reach that!]", fs.messages)
	}
}

func TestWalkToProjectedNpcInvalidated(t *testing.T) {
	_, _, p, _ := projectedSetup(t)
	npc := projectedNPC(t, Location{X: 3003, Y: 3000, Z: 0})
	ran := false
	q := p.MovementQueue()
	q.WalkToEntity(NPCTarget(npc), func() { ran = true })
	// The NPC unregisters mid-walk: the task stops silently.
	npc.SetRegistered(false)
	q.Process(1)
	if ran || q.WalkToPending() {
		t.Errorf("ran=%v pending=%v, want a silent stop", ran, q.WalkToPending())
	}
}

func TestWalkToProjectedNpcNotProjected(t *testing.T) {
	m, boat, p, _ := projectedSetup(t)
	q := p.MovementQueue()

	// A player target is never projected (no NPCType read).
	other := NewPlayer("other", Location{X: 3003, Y: 3000, Z: 0})
	other.SetRegistered(true)
	q.WalkToEntity(PlayerTarget(other), func() {})
	if q.walkTo == nil || q.walkTo.projected != nil || q.walkTo.entity == nil {
		t.Errorf("a player target must take the entity shape, got %+v", q.walkTo)
	}
	q.CancelWalkTo()

	// An NPC aboard the same boat keeps local deck routing.
	deckNPC := projectedNPC(t, Location{X: boat.DeckBaseX(), Y: boat.DeckBaseY(), Z: 0})
	m.GetDeck(boat).Enter(deckNPC)
	q.WalkToEntity(NPCTarget(deckNPC), func() {})
	if q.walkTo == nil || q.walkTo.projected != nil || q.walkTo.entity == nil {
		t.Errorf("a same-deck NPC must take the entity shape, got %+v", q.walkTo)
	}
	q.CancelWalkTo()

	// An NPC in a private area does not project.
	area := NewPrivateArea([]Boundary{NewBoundary(3000, 3010, 3000, 3010, 0)})
	areaNPC := projectedNPC(t, Location{X: 3003, Y: 3000, Z: 0})
	area.Enter(areaNPC)
	q.WalkToEntity(NPCTarget(areaNPC), func() {})
	if q.walkTo == nil || q.walkTo.projected != nil {
		t.Errorf("a private-area NPC must not project, got %+v", q.walkTo)
	}
	q.CancelWalkTo()

	// An NPC owned by someone else does not project: the
	// walk either takes the entity shape or is refused by
	// the validity gate, but never the projected shape.
	owner := NewPlayer("owner", Location{X: 3000, Y: 3000, Z: 0})
	owned := projectedNPC(t, Location{X: 3003, Y: 3000, Z: 0})
	target := NPCTarget(owned)
	target.Owner = owner
	q.WalkToEntity(target, func() {})
	if q.walkTo != nil && q.walkTo.projected != nil {
		t.Errorf("a foreign-owned NPC must not project, got %+v", q.walkTo)
	}
	q.CancelWalkTo()

	// An ashore player never projects.
	ashore := NewPlayer("ashore", Location{X: 3000, Y: 3000, Z: 0})
	ashore.SetIndex(9)
	ashoreQ := ashore.MovementQueue()
	npc := projectedNPC(t, Location{X: 3003, Y: 3000, Z: 0})
	ashoreQ.WalkToEntity(NPCTarget(npc), func() {})
	if ashoreQ.walkTo == nil || ashoreQ.walkTo.projected != nil {
		t.Errorf("an ashore walk must not project, got %+v", ashoreQ.walkTo)
	}
}

func TestWalkToProjectedNpcReplacesPendingWalk(t *testing.T) {
	_, _, p, _ := projectedSetup(t)
	q := p.MovementQueue()
	// A pending tile walk-to is cancelled by the projected
	// walk's walkToReset.
	q.WalkToTile(Location{X: 9600, Y: 9600, Z: 0}, func() {}, nil, false, 0)
	if !q.WalkToPending() || q.walkTo.projected != nil {
		t.Fatal("a tile walk-to must pend first")
	}
	npc := projectedNPC(t, Location{X: 3003, Y: 3000, Z: 0})
	q.WalkToEntity(NPCTarget(npc), func() {})
	if q.walkTo == nil || q.walkTo.projected == nil {
		t.Errorf("the projected walk must replace the pending walk, got %+v", q.walkTo)
	}
}
