package game

import "testing"

func TestSetMobileInteractionFacingAndFlag(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	p := NewPlayer("alice", Location{X: 12, Y: 10, Z: 0})
	n.SetMobileInteraction(p)
	if n.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST toward the target", n.Facing)
	}
	if !n.Flags.Has(FlagEntityInteraction) {
		t.Error("FlagEntityInteraction not set")
	}
	if got := n.GetInteractingMobile(); got != &p.Mobile {
		t.Error("GetInteractingMobile did not return the player's Mobile")
	}
	if got := n.InteractingPlayer(); got != p {
		t.Error("InteractingPlayer did not return the player")
	}
	if got := n.GetFacingDirection(); got != DirEast {
		t.Errorf("GetFacingDirection = %v, want EAST", got)
	}
	// Clearing keeps the last target-facing direction: the
	// previous-direction snapshot sets Facing before the clear.
	n.ResetUpdating()
	n.SetMobileInteraction(nil)
	if n.Facing != DirEast {
		t.Errorf("Facing after clear = %v, want EAST (previous target direction)", n.Facing)
	}
	if n.GetInteractingMobile() != nil {
		t.Error("interaction not cleared")
	}
	if !n.Flags.Has(FlagEntityInteraction) {
		t.Error("clear must still flag ENTITY_INTERACTION")
	}
}

func TestSetMobileInteractionSameTileLeavesFacing(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.Facing = DirSouth
	p := NewPlayer("alice", Location{X: 10, Y: 10, Z: 0})
	n.SetMobileInteraction(p)
	if n.Facing != DirSouth {
		t.Errorf("Facing = %v, want SOUTH (same-tile target gives no direction)", n.Facing)
	}
	if got := n.GetFacingDirection(); got != DirSouth {
		t.Errorf("GetFacingDirection = %v, want SOUTH fallback", got)
	}
}

func TestSetMobileInteractionRetarget(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	east := NewPlayer("east", Location{X: 12, Y: 10, Z: 0})
	north := NewPlayer("north", Location{X: 10, Y: 12, Z: 0})
	n.SetMobileInteraction(east)
	gen := n.interactionGeneration
	n.SetMobileInteraction(north)
	if n.Facing != DirNorth {
		t.Errorf("Facing = %v, want NORTH toward the new target", n.Facing)
	}
	if n.interactionGeneration != gen+1 {
		t.Errorf("generation = %d, want %d", n.interactionGeneration, gen+1)
	}
	// An NPC target resolves to its Mobile with no player identity.
	other := NewNPC(2, Location{X: 10, Y: 8, Z: 0})
	n.SetMobileInteraction(other)
	if got := n.GetInteractingMobile(); got != &other.Mobile {
		t.Error("NPC target did not resolve to its Mobile")
	}
	if n.InteractingPlayer() != nil {
		t.Error("InteractingPlayer must be nil for an NPC target")
	}
	if n.Facing != DirSouth {
		t.Errorf("Facing = %v, want SOUTH toward the NPC", n.Facing)
	}
	// A bare *Mobile resolves with no typed identity.
	n.SetMobileInteraction(&east.Mobile)
	if n.InteractingPlayer() != nil {
		t.Error("bare *Mobile must not invent a player identity")
	}
	if got := n.GetInteractingMobile(); got != &east.Mobile {
		t.Error("bare *Mobile target not stored")
	}
	// An unrecognized value clears, never invents.
	n.SetMobileInteraction("not a mobile")
	if n.GetInteractingMobile() != nil {
		t.Error("unrecognized target must clear the interaction")
	}
}

func TestSetPositionToFace(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.Facing = DirSouth
	east := Location{X: 11, Y: 10, Z: 0}
	n.SetPositionToFace(&east)
	if n.Facing != DirEast {
		t.Errorf("Facing = %v, want EAST", n.Facing)
	}
	if !n.Flags.Has(FlagFacePosition) {
		t.Error("FlagFacePosition not set")
	}
	if got, ok := n.PositionToFace(); !ok || !got.Equals(east) {
		t.Errorf("PositionToFace = %+v, %v, want %+v, true", got, ok, east)
	}
	// Same coordinates again: no-op, no flag.
	n.ResetUpdating()
	same := Location{X: 11, Y: 10, Z: 0}
	n.SetPositionToFace(&same)
	if n.Flags.Has(FlagFacePosition) {
		t.Error("same-coordinate set must not flag FACE_POSITION")
	}
	// Clearing: flagged, and the target is gone.
	n.SetPositionToFace(nil)
	if !n.Flags.Has(FlagFacePosition) {
		t.Error("clear must flag FACE_POSITION")
	}
	if _, ok := n.PositionToFace(); ok {
		t.Error("face target not cleared")
	}
	// Clearing when already clear: no-op, no flag.
	n.ResetUpdating()
	n.SetPositionToFace(nil)
	if n.Flags.Has(FlagFacePosition) {
		t.Error("clear-when-clear must not flag FACE_POSITION")
	}
	// A same-tile target leaves Facing alone but still stores.
	n.Facing = DirSouth
	here := Location{X: 10, Y: 10, Z: 0}
	n.SetPositionToFace(&here)
	if n.Facing != DirSouth {
		t.Errorf("Facing = %v, want SOUTH (same-tile face target)", n.Facing)
	}
}

func TestSetDirection(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	n.SetDirection(DirNorth)
	if got, ok := n.PositionToFace(); !ok || !got.Equals(Location{X: 10, Y: 11, Z: 0}) {
		t.Errorf("PositionToFace = %+v, %v", got, ok)
	}
	if n.Facing != DirNorth {
		t.Errorf("Facing = %v, want NORTH", n.Facing)
	}
}

func TestFollowingAccessors(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	p := NewPlayer("alice", Location{X: 12, Y: 10, Z: 0})
	if n.GetFollowing() != nil {
		t.Error("fresh NPC must follow nothing")
	}
	n.SetFollowing(p)
	if got := n.GetFollowing(); got != &p.Mobile {
		t.Error("GetFollowing did not return the player's Mobile")
	}
	if n.FollowingPlayer() != p {
		t.Error("FollowingPlayer did not return the player")
	}
	other := NewNPC(2, Location{X: 8, Y: 10, Z: 0})
	n.SetFollowing(other)
	if n.FollowingNPC() != other || n.FollowingPlayer() != nil {
		t.Error("NPC follow target identity mismatch")
	}
	n.SetFollowing(nil)
	if n.GetFollowing() != nil {
		t.Error("follow not cleared")
	}
	// Combat following is the combat target: nil against the stub.
	if n.GetCombatFollowing() != nil {
		t.Error("GetCombatFollowing must be nil with no combat target")
	}
}

func TestFollowDriverApproachesLeader(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	p := NewPlayer("alice", Location{X: 20, Y: 10, Z: 0})
	p.SetRegistered(true)
	n.SetFollowing(p)
	n.MovementCoordinator().random = cycleRandom(0.9) // no wandering
	for cycle := int64(1); cycle <= 15; cycle++ {
		n.Process(cycle)
	}
	gap := footprintGap(n.Location(), n.Size(), p.Location(), 1)
	if gap != 1 {
		t.Errorf("final gap = %d, want 1 (adjacent to the leader); NPC at %+v", gap, n.Location())
	}
	if n.GetFollowing() == nil {
		t.Error("follow ended while the leader stood adjacent")
	}
	if got := n.GetInteractingMobile(); got != &p.Mobile {
		t.Error("follow did not set the interaction to the leader")
	}
	// Adjacent to a stationary leader: no more routing.
	if got := n.MovementQueue().Size(); got != 0 {
		t.Errorf("queued points while idle-adjacent = %d, want 0", got)
	}
}

func TestFollowDriverTracksMovingLeader(t *testing.T) {
	n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
	p := NewPlayer("alice", Location{X: 13, Y: 10, Z: 0})
	p.SetRegistered(true)
	n.SetFollowing(p)
	n.MovementCoordinator().random = cycleRandom(0.9)
	// The leader walks east one tile per tick for five ticks,
	// then stops. Equal speeds mean the follower cannot close
	// the gap while the leader moves, but must never fall
	// further behind — and must end adjacent once the leader
	// stops.
	for cycle := int64(1); cycle <= 15; cycle++ {
		if cycle <= 5 {
			p.SetLocation(Location{X: 13 + int(cycle), Y: 10, Z: 0})
		}
		n.Process(cycle)
		gap := footprintGap(n.Location(), n.Size(), p.Location(), 1)
		if gap > 4 {
			t.Fatalf("cycle %d: gap %d, follower fell behind (NPC %+v leader %+v)", cycle, gap, n.Location(), p.Location())
		}
	}
	if gap := footprintGap(n.Location(), n.Size(), p.Location(), 1); gap != 1 {
		t.Errorf("final gap = %d, want 1; NPC at %+v, leader at %+v", gap, n.Location(), p.Location())
	}
}

func TestFollowDriverEndConditions(t *testing.T) {
	newPair := func() (*NPC, *Player) {
		n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
		p := NewPlayer("alice", Location{X: 12, Y: 10, Z: 0})
		p.SetRegistered(true)
		n.SetFollowing(p)
		n.MovementCoordinator().random = cycleRandom(0.9)
		return n, p
	}
	t.Run("unregistered leader ends follow", func(t *testing.T) {
		n, p := newPair()
		p.SetRegistered(false)
		n.Process(1)
		if n.GetFollowing() != nil || n.GetInteractingMobile() != nil {
			t.Error("follow/interaction must clear when the leader unregisters")
		}
	})
	t.Run("cross-plane leader ends follow", func(t *testing.T) {
		n, p := newPair()
		p.SetLocation(Location{X: 12, Y: 10, Z: 1})
		n.Process(1)
		if n.GetFollowing() != nil {
			t.Error("follow must end when the leader changes plane")
		}
	})
	t.Run("far leader ends follow", func(t *testing.T) {
		n, p := newPair()
		p.SetLocation(Location{X: 30, Y: 10, Z: 0})
		n.Process(1)
		if n.GetFollowing() != nil {
			t.Error("follow must end past the 15-tile auto-teleport distance")
		}
	})
	t.Run("dying NPC leader ends follow", func(t *testing.T) {
		n := NewNPC(1, Location{X: 10, Y: 10, Z: 0})
		leader := NewNPC(2, Location{X: 12, Y: 10, Z: 0})
		leader.SetRegistered(true)
		leader.Dying = true
		n.SetFollowing(leader)
		n.MovementCoordinator().random = cycleRandom(0.9)
		n.Process(1)
		if n.GetFollowing() != nil {
			t.Error("follow must end when the leader is dying")
		}
	})
	t.Run("self follow ends", func(t *testing.T) {
		n, _ := newPair()
		n.SetFollowing(n)
		n.Process(1)
		if n.GetFollowing() != nil {
			t.Error("self-follow must end")
		}
	})
}

func TestOuterTilesAndFootprintGap(t *testing.T) {
	tiles := outerTiles(Location{X: 10, Y: 10, Z: 0}, 1)
	want := []Location{
		{X: 10, Y: 9, Z: 0}, {X: 10, Y: 11, Z: 0},
		{X: 9, Y: 10, Z: 0}, {X: 11, Y: 10, Z: 0},
	}
	if len(tiles) != len(want) {
		t.Fatalf("outerTiles(1) = %d tiles, want %d", len(tiles), len(want))
	}
	for i := range want {
		if tiles[i] != want[i] {
			t.Errorf("tile %d = %+v, want %+v", i, tiles[i], want[i])
		}
	}
	if got := len(outerTiles(Location{}, 3)); got != 12 {
		t.Errorf("outerTiles(3) = %d tiles, want 12", got)
	}
	cases := []struct {
		a, b   Location
		as, bs int
		want   int
	}{
		{Location{X: 10, Y: 10}, Location{X: 10, Y: 10}, 1, 1, 0},
		{Location{X: 10, Y: 10}, Location{X: 11, Y: 10}, 1, 1, 1},
		{Location{X: 10, Y: 10}, Location{X: 12, Y: 10}, 1, 1, 2},
		{Location{X: 10, Y: 10}, Location{X: 12, Y: 12}, 3, 1, 0}, // inside the 3x3 footprint
		{Location{X: 10, Y: 10}, Location{X: 13, Y: 10}, 3, 1, 1},
		{Location{X: 10, Y: 10, Z: 1}, Location{X: 10, Y: 10, Z: 0}, 1, 1, unreachableDistance},
	}
	for _, tc := range cases {
		if got := footprintGap(tc.a, tc.as, tc.b, tc.bs); got != tc.want {
			t.Errorf("footprintGap(%+v/%d, %+v/%d) = %d, want %d", tc.a, tc.as, tc.b, tc.bs, got, tc.want)
		}
	}
}

// startNPCDialogue starts a one-line NPC dialogue for player,
// spoken by npcID, and returns the manager.
func startNPCDialogue(t *testing.T, p *Player, npcID int) *DialogueManager {
	t.Helper()
	builder := NewDialogueChainBuilder().Add(NewNpcDialogue(0, npcID, "Hello, traveller."))
	p.GetDialogueManager().StartDialogues(builder)
	m := p.GetDialogueManager()
	if !m.IsActive() {
		t.Fatal("dialogue did not start")
	}
	return m
}

func TestDialogueHold(t *testing.T) {
	spawn := Location{X: 100, Y: 100, Z: 0}
	newHeldPair := func() (*NPC, *Player) {
		n := NewNPC(7, spawn)
		p := NewPlayer("alice", Location{X: 101, Y: 100, Z: 0})
		p.SetRegistered(true)
		n.SetMobileInteraction(p)
		return n, p
	}
	t.Run("speaking NPC is held and stands still", func(t *testing.T) {
		n, p := newHeldPair()
		startNPCDialogue(t, p, 7)
		n.MovementQueue().AddCheckpoint(Location{X: 105, Y: 100, Z: 0})
		n.Process(1)
		if n.heldByDialogue != p {
			t.Error("NPC must be held by the speaking player's dialogue")
		}
		if !n.Location().Equals(spawn) {
			t.Errorf("held NPC stepped to %+v", n.Location())
		}
		if got := n.MovementQueue().Size(); got == 0 {
			t.Error("held NPC must keep its walk queued")
		}
		// The conversation ends: the walk resumes next tick.
		p.GetDialogueManager().Reset()
		n.Process(2)
		if n.heldByDialogue != nil {
			t.Error("hold must release when the dialogue closes")
		}
		if n.Location().Equals(spawn) {
			t.Error("NPC did not resume its queued walk after the hold")
		}
	})
	t.Run("another NPC speaking does not hold", func(t *testing.T) {
		n, p := newHeldPair()
		startNPCDialogue(t, p, 999) // a different NPC speaks
		if held := n.updateDialogueHold(); held {
			t.Error("NPC must not be held by another NPC's line")
		}
	})
	t.Run("player opening line does not hold", func(t *testing.T) {
		n, p := newHeldPair()
		builder := NewDialogueChainBuilder().Add(NewPlayerDialogue(0, "Hi."))
		p.GetDialogueManager().StartDialogues(builder)
		if held := n.updateDialogueHold(); held {
			t.Error("player's opening line must not hold the NPC")
		}
	})
	t.Run("no dialogue does not hold", func(t *testing.T) {
		n, _ := newHeldPair()
		if held := n.updateDialogueHold(); held {
			t.Error("no dialogue must not hold")
		}
	})
	t.Run("distant player does not hold", func(t *testing.T) {
		n, p := newHeldPair()
		startNPCDialogue(t, p, 7)
		p.SetLocation(Location{X: 103, Y: 100, Z: 0})
		if held := n.updateDialogueHold(); held {
			t.Error("player more than a tile away must not hold")
		}
	})
	t.Run("non-player interaction does not hold", func(t *testing.T) {
		n := NewNPC(7, spawn)
		other := NewNPC(8, Location{X: 101, Y: 100, Z: 0})
		n.SetMobileInteraction(other)
		if held := n.updateDialogueHold(); held {
			t.Error("an NPC interaction target must not hold")
		}
	})
	t.Run("held NPC does not wander", func(t *testing.T) {
		n, p := newHeldPair()
		startNPCDialogue(t, p, 7)
		n.MovementCoordinator().SetRadius(5)
		n.MovementCoordinator().random = cycleRandom(0.0)
		n.Process(1)
		if got := n.MovementQueue().Size(); got != 0 {
			t.Errorf("held NPC queued %d wander points", got)
		}
	})
}
