package game

import "testing"

// Trade/duel mobility gates (MovementQueue.ts:366-371, 1998-2023)
// and walk-to-entity (MovementQueue.ts:1478-1615). Fixtures only;
// open ground (nil collision/clipping) unless stated.

func tradeGatePlayer(t *testing.T, name string, loc Location) (*Player, *fakeSession) {
	t.Helper()
	p := NewPlayer(name, loc)
	sess := newFakeSession()
	p.Session = sess
	return p, sess
}

func TestMobilityBusyTradeButtonDelay(t *testing.T) {
	p, sess := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	q := p.MovementQueue()
	if q.getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("fresh player must be mobile")
	}
	p.Trading().ButtonDelay().Start(60)
	if q.getMobilityIgnoringFreeze() != mobilityBusy {
		t.Fatalf("mobility = %v, want BUSY", q.getMobilityIgnoringFreeze())
	}
	if q.canMove() {
		t.Fatal("BUSY must not move")
	}
	q.RequestWalk(Location{X: 3201, Y: 3200, Z: 0}, nil)
	if len(sess.messages) != 1 || sess.messages[0] != "You cannot do that right now." {
		t.Fatalf("messages = %v", sess.messages)
	}
	if len(q.Points()) != 0 {
		t.Fatal("refused walk must queue nothing")
	}
	p.Trading().ButtonDelay().Stop()
	if q.getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("stopped delay must restore mobility")
	}
}

func TestMobilityBusyDuelButtonDelay(t *testing.T) {
	p, _ := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	p.Dueling().ButtonDelay().Start(60)
	if p.MovementQueue().getMobilityIgnoringFreeze() != mobilityBusy {
		t.Fatal("unfinished duel button delay must report BUSY")
	}
	p.Dueling().ButtonDelay().Stop()
	if p.MovementQueue().getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("stopped duel delay must restore mobility")
	}
}

func TestMobilityDuelMovementDisabled(t *testing.T) {
	p, sess := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	q := p.MovementQueue()
	d := p.Dueling()
	d.Rules()[DuelRuleNoMovement.ButtonID()] = true
	// The rule alone does not disable movement: inDuel is
	// required.
	if q.getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("rule without a duel must stay mobile")
	}
	d.SetState(DuelStateDuelScreen)
	if q.getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("setup-screen duel must stay mobile")
	}
	d.SetState(DuelStateInDuel)
	if q.getMobilityIgnoringFreeze() != mobilityDuelMovementDisabled {
		t.Fatalf("mobility = %v, want DUEL_MOVEMENT_DISABLED", q.getMobilityIgnoringFreeze())
	}
	q.RequestWalk(Location{X: 3201, Y: 3200, Z: 0}, nil)
	if len(sess.messages) != 1 || sess.messages[0] != "Movement has been disabled in this duel!" {
		t.Fatalf("messages = %v", sess.messages)
	}
	// STARTING_DUEL is in a duel too.
	d.SetState(DuelStateStartingDuel)
	if q.getMobilityIgnoringFreeze() != mobilityDuelMovementDisabled {
		t.Fatal("starting duel with the rule must disable movement")
	}
	// The rule off restores movement mid-duel.
	d.Rules()[DuelRuleNoMovement.ButtonID()] = false
	if q.getMobilityIgnoringFreeze() != mobilityMobile {
		t.Fatal("rule off must restore mobility")
	}
}

func TestMobilityGateOrder(t *testing.T) {
	p, _ := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	q := p.MovementQueue()
	// BUSY beats DUEL_MOVEMENT_DISABLED (TS order).
	p.Trading().ButtonDelay().Start(60)
	p.Dueling().SetState(DuelStateInDuel)
	p.Dueling().Rules()[DuelRuleNoMovement.ButtonID()] = true
	if q.getMobilityIgnoringFreeze() != mobilityBusy {
		t.Fatalf("mobility = %v, want BUSY over DUEL", q.getMobilityIgnoringFreeze())
	}
	// STUN beats BUSY.
	p.GetTimers().Registers(TimerKeyStun, 5)
	if q.getMobilityIgnoringFreeze() != mobilityStunned {
		t.Fatalf("mobility = %v, want STUNNED over BUSY", q.getMobilityIgnoringFreeze())
	}
	// blockMovement (INVALID) beats BUSY.
	p.GetTimers().Cancel(TimerKeyStun)
	q.SetBlockMovement(true)
	if q.getMobilityIgnoringFreeze() != mobilityInvalid {
		t.Fatalf("mobility = %v, want INVALID over BUSY", q.getMobilityIgnoringFreeze())
	}
	// FREEZE beats everything, through getMobility only.
	q.SetBlockMovement(false)
	p.GetTimers().Registers(TimerKeyFreeze, 5)
	if q.getMobility() != mobilityFrozenSpell {
		t.Fatalf("mobility = %v, want FROZEN_SPELL", q.getMobility())
	}
	if q.getMobilityIgnoringFreeze() != mobilityBusy {
		t.Fatalf("ignoring-freeze mobility = %v, want BUSY", q.getMobilityIgnoringFreeze())
	}
}

func TestMobilitySendMessageLines(t *testing.T) {
	owner := newFakeOwner(Location{X: 3200, Y: 3200, Z: 0})
	mobilityBusy.sendMessage(owner)
	mobilityDuelMovementDisabled.sendMessage(owner)
	mobilityStunned.sendMessage(owner)
	mobilityFrozenSpell.sendMessage(owner)
	mobilityInvalid.sendMessage(owner)
	mobilityAnimationStall.sendMessage(owner)
	mobilityMobile.sendMessage(owner)
	if len(owner.messages) != 2 ||
		owner.messages[0] != "You cannot do that right now." ||
		owner.messages[1] != "Movement has been disabled in this duel!" {
		t.Fatalf("plain messages = %v", owner.messages)
	}
	if len(owner.typedMessages) != 1 || owner.typedMessages[0] != [3]any{chatTypeSpam, "You're stunned!", ""} {
		t.Fatalf("typed messages = %v", owner.typedMessages)
	}
}

// entityPair builds a walker and a registered target player.
func entityPair(t *testing.T, walkerLoc, targetLoc Location) (walker, target *Player, walkerSess *fakeSession) {
	t.Helper()
	walker, walkerSess = tradeGatePlayer(t, "Alice", walkerLoc)
	target = NewPlayer("Bob", targetLoc)
	target.SetRegistered(true)
	return walker, target, walkerSess
}

func TestWalkToEntityImmediateReach(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3201, Y: 3200, Z: 0})
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if !ran {
		t.Fatal("adjacent target must run the action at call time")
	}
	if walker.MovementQueue().WalkToPending() {
		t.Fatal("no walk-to state for an immediate reach")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityWalksAndArrives(t *testing.T) {
	walker, target, _ := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3205, Z: 0})
	q := walker.MovementQueue()
	ran := false
	q.WalkToEntity(PlayerTarget(target), func() { ran = true })
	if !q.WalkToPending() {
		t.Fatal("walk-to state must install")
	}
	for cycle := int64(2); cycle < 12 && !ran; cycle++ {
		tickQueue(q, cycle)
	}
	if !ran {
		t.Fatalf("never arrived; walker at %+v", walker.Location())
	}
	if q.WalkToPending() {
		t.Fatal("walk-to state must clear after arrival")
	}
	if !q.ReachedEntity(PlayerTarget(target)) {
		t.Fatalf("walker ended at %+v, not reaching the target", walker.Location())
	}
}

func TestWalkToEntityRepathsToMovingTarget(t *testing.T) {
	walker, target, _ := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3204, Z: 0})
	q := walker.MovementQueue()
	ran := false
	q.WalkToEntity(PlayerTarget(target), func() { ran = true })
	moved := false
	for cycle := int64(2); cycle < 20 && !ran; cycle++ {
		tickQueue(q, cycle)
		if !moved && walker.Location().Y >= 3202 {
			// The target steps away mid-walk; pursuit
			// re-paths to the live tile.
			target.SetLocation(Location{X: 3200, Y: 3207, Z: 0})
			moved = true
		}
	}
	if !ran {
		t.Fatalf("never arrived after the target moved; walker at %+v target at %+v",
			walker.Location(), target.Location())
	}
	if !q.ReachedEntity(PlayerTarget(target)) {
		t.Fatalf("walker ended at %+v, not reaching the moved target at %+v",
			walker.Location(), target.Location())
	}
}

func TestWalkToEntityInvalidTargetStopsSilently(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3205, Z: 0})
	q := walker.MovementQueue()
	ran := false
	q.WalkToEntity(PlayerTarget(target), func() { ran = true })
	tickQueue(q, 2)
	target.SetRegistered(false)
	tickQueue(q, 3)
	if ran {
		t.Fatal("deregistered target must not run the action")
	}
	if q.WalkToPending() {
		t.Fatal("invalid target must stop the walk")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("invalidity is silent, got %v", sess.messages)
	}
}

func TestWalkToEntityDeadTargetRefused(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3205, Z: 0})
	target.Skills().SetCurrentLevel(SkillHitpoints, 0, true)
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("dead target must refuse silently with no state")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityUnreachableSendsMessage(t *testing.T) {
	// A target outside the route finder's 128x128 window
	// routes nowhere: the task fails with the TS message.
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3400, Y: 3400, Z: 0})
	q := walker.MovementQueue()
	ran := false
	q.WalkToEntity(PlayerTarget(target), func() { ran = true })
	for cycle := int64(2); cycle < 6 && q.WalkToPending(); cycle++ {
		tickQueue(q, cycle)
	}
	if ran {
		t.Fatal("unreachable target must not run the action")
	}
	if q.WalkToPending() {
		t.Fatal("unreachable walk-to-entity should stop")
	}
	if len(sess.messages) != 1 || sess.messages[0] != "I can't reach that!" {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityFrozenReached(t *testing.T) {
	walker, target, _ := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3201, Y: 3200, Z: 0})
	walker.GetTimers().Registers(TimerKeyFreeze, 5)
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if !ran {
		t.Fatal("frozen player reaching the target must run the action")
	}
}

func TestWalkToEntityFrozenUnreached(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3205, Z: 0})
	walker.GetTimers().Registers(TimerKeyFreeze, 5)
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("frozen player out of reach must not walk")
	}
	if len(sess.messages) != 1 || sess.messages[0] != "I can't reach that!" {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityFrozenPlusStun(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3201, Y: 3200, Z: 0})
	walker.GetTimers().Registers(TimerKeyFreeze, 5)
	walker.GetTimers().Registers(TimerKeyStun, 5)
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("stun must stop even a frozen player")
	}
	if len(sess.typedMessages) != 1 || sess.typedMessages[0] != [3]any{chatTypeSpam, "You're stunned!", ""} {
		t.Fatalf("typed messages = %v", sess.typedMessages)
	}
}

func TestWalkToEntityBusyRefusal(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3200, Y: 3205, Z: 0})
	walker.Trading().ButtonDelay().Start(60)
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("BUSY must refuse the walk-to")
	}
	if len(sess.messages) != 1 || sess.messages[0] != "You cannot do that right now." {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityCrossPlaneRefused(t *testing.T) {
	walker, target, sess := entityPair(t,
		Location{X: 3200, Y: 3200, Z: 0}, Location{X: 3201, Y: 3200, Z: 1})
	ran := false
	walker.MovementQueue().WalkToEntity(PlayerTarget(target), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("cross-plane target must refuse silently")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestWalkToEntityNPCOwnerRule(t *testing.T) {
	walker, walkerSess := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	other := NewPlayer("Bob", Location{X: 3200, Y: 3200, Z: 0})
	npc := NewNPC(1, Location{X: 3200, Y: 3205, Z: 0})
	npc.SetRegistered(true)
	npc.Hitpoints = 10

	// Owned by someone else: invalid, silent.
	npc.SetOwner(other)
	ran := false
	walker.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("NPC owned by another player must refuse silently")
	}
	if len(walkerSess.messages) != 0 {
		t.Fatalf("messages = %v", walkerSess.messages)
	}

	// Owned by the walker: valid, walks.
	npc.SetOwner(walker)
	walker.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !walker.MovementQueue().WalkToPending() {
		t.Fatal("own NPC must be a valid target")
	}

	// Unowned: valid.
	npc.SetOwner(nil)
	walker.MovementQueue().CancelWalkTo()
	walker.MovementQueue().WalkToEntity(NPCTarget(npc), func() { ran = true })
	if !walker.MovementQueue().WalkToPending() {
		t.Fatal("unowned NPC must be a valid target")
	}
}

func TestWalkToEntityNilTarget(t *testing.T) {
	walker, sess := tradeGatePlayer(t, "Alice", Location{X: 3200, Y: 3200, Z: 0})
	ran := false
	walker.MovementQueue().WalkToEntity(EntityTarget{}, func() { ran = true })
	if ran || walker.MovementQueue().WalkToPending() {
		t.Fatal("nil target must refuse silently")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("messages = %v", sess.messages)
	}
}

func TestReachedEntityVectors(t *testing.T) {
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	target := NewPlayer("Bob", Location{X: 101, Y: 100, Z: 0})
	target.SetRegistered(true)
	if !q.ReachedEntity(PlayerTarget(target)) {
		t.Fatal("orthogonally adjacent must reach")
	}
	// Diagonal neighbours do NOT reach (reachRectangle1).
	target.SetLocation(Location{X: 101, Y: 101, Z: 0})
	if q.ReachedEntity(PlayerTarget(target)) {
		t.Fatal("diagonal neighbour must not reach")
	}
	target.SetLocation(Location{X: 102, Y: 100, Z: 0})
	if q.ReachedEntity(PlayerTarget(target)) {
		t.Fatal("two tiles away must not reach")
	}
	// Standing on the target's tile does NOT reach:
	// locShape -2 is rectangle-exclusive, so overlap is not
	// a reach (PathFinder.reachedEntity is the bare
	// ReachedAbsolute call, with no tile-equality
	// short-circuit).
	target.SetLocation(Location{X: 100, Y: 100, Z: 0})
	if q.ReachedEntity(PlayerTarget(target)) {
		t.Fatal("shared tile must not reach (rectangle-exclusive)")
	}
	// A 2x2 target reaches from the tile beside its far edge.
	big := EntityTarget{Mobile: &target.Mobile, Size: 2}
	target.SetLocation(Location{X: 101, Y: 100, Z: 0})
	if !q.ReachedEntity(big) {
		t.Fatal("tile beside a 2x2 target must reach")
	}
	if q.ReachedEntity(EntityTarget{}) {
		t.Fatal("nil target must never reach")
	}
}
