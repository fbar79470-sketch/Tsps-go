package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Trade tests: the option-2 request flow (gates, plane, distance,
// emit, walk-to-entity, sendRequest), the busy degenerations, and
// the trade_action dispatch. Fixtures only: the fake lookup stands
// in for the world player list, and a real World maintains the
// local-players lists.

// tradeWorld builds a world with two registered players: a at the
// spawn and b at bLoc. Buckets are rebuilt and local lists
// updated, as the update phase does.
func tradeWorld(t *testing.T, bLoc game.Location) (*game.World, *game.Player, *game.Player) {
	t.Helper()
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	w := game.NewWorld(spawn)
	add := func(name string, loc game.Location) *game.Player {
		p := game.NewPlayer(name, loc)
		if _, ok := w.Players().Add(p); !ok {
			t.Fatalf("add of player %s failed", name)
		}
		p.SetRegistered(true)
		return p
	}
	a := add("alice", spawn)
	b := add("bob", bLoc)
	w.RebuildUpdateBuckets()
	w.UpdateLocalPlayers(a)
	w.UpdateLocalPlayers(b)
	return w, a, b
}

// tradeDeps wires a lookup serving target at its world index.
func tradeDeps(target *game.Player) Deps {
	return Deps{Players: &fakePlayerLookup{players: map[int]*game.Player{target.GetIndex(): target}}}
}

func tradeOutcome(p *game.Player, msg protocol.TradeAction) Outcome {
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
	return routeTradeAction(p, msg, drop, acted, noAction)
}

func withTradeHookSeam(t *testing.T, hooks game.TradeHooks) {
	t.Helper()
	prev := game.TradeHookSeam
	game.TradeHookSeam = hooks
	t.Cleanup(func() { game.TradeHookSeam = prev })
}

func TestTradeRequestAdjacentHandshake(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	// Adjacent: the walk resolves at call time and the request
	// is delivered immediately.
	if got := routePlayerOutcome(a, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)); !got.Implemented {
		t.Fatalf("request = %+v, want acted", got)
	}
	if a.Trading().State() != game.TradeStateRequestedTrade {
		t.Fatalf("Alice state = %v, want REQUESTED", a.Trading().State())
	}
	// Bob answers: both reach the offer screen.
	if got := routePlayerOutcome(b, protocol.PlayerOption{Index: a.GetIndex(), Option: 2}, tradeDeps(a)); !got.Implemented {
		t.Fatalf("answer = %+v, want acted", got)
	}
	if a.Trading().State() != game.TradeStateTradeScreen || b.Trading().State() != game.TradeStateTradeScreen {
		t.Fatalf("states = %v/%v, want trade screen", a.Trading().State(), b.Trading().State())
	}
}

func TestTradeRequestWalksToTarget(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3099, Y: 3524, Z: 0})
	if got := routePlayerOutcome(a, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)); !got.Implemented {
		t.Fatalf("request = %+v, want acted", got)
	}
	q := a.MovementQueue()
	if !q.WalkToPending() {
		t.Fatal("distant target must install a walk-to")
	}
	// Tick the queue until the arrival callback delivers the
	// request (or the walk fails).
	for cycle := int64(2); cycle < 25 && a.Trading().State() == game.TradeStateNone; cycle++ {
		q.BeginCycle()
		if q.HasPendingWork() {
			q.Process(cycle)
		}
	}
	if a.Trading().State() != game.TradeStateRequestedTrade {
		t.Fatalf("Alice state = %v after walking, want REQUESTED", a.Trading().State())
	}
}

func TestTradeRequestRefusals(t *testing.T) {
	// Unwired lookup: the named drop.
	p := testPlayer()
	if got := routePlayerOutcome(p, protocol.PlayerOption{Index: 7, Option: 2}, Deps{}); got.Reason != ReasonTradeRequest {
		t.Fatalf("unwired = %+v, want ReasonTradeRequest", got)
	}
	// Nil player: the implemented-routes rule.
	if got := routePlayerOutcome(nil, protocol.PlayerOption{Index: 7, Option: 2}, Deps{}); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}

	// The remaining refusals are silent (noAction).
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	cases := []struct {
		name string
		run  func()
		msg  protocol.PlayerOption
		deps Deps
	}{
		{"self", func() {}, protocol.PlayerOption{Index: a.GetIndex(), Option: 2}, tradeDeps(a)},
		{"missing target", func() {}, protocol.PlayerOption{Index: 99, Option: 2}, tradeDeps(b)},
		{"unregistered player", func() { a.SetRegistered(false) }, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)},
		{"dead player", func() {
			a.SetRegistered(true)
			a.Skills().SetCurrentLevel(game.SkillHitpoints, 0, true)
		}, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)},
		{"dead target", func() {
			a.Skills().SetCurrentLevel(game.SkillHitpoints, 10, true)
			b.Skills().SetCurrentLevel(game.SkillHitpoints, 0, true)
		}, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.run()
			if got := routePlayerOutcome(a, tc.msg, tc.deps); got.Reason != ReasonNoAction {
				t.Fatalf("%s = %+v, want noAction", tc.name, got)
			}
			if a.MovementQueue().WalkToPending() {
				t.Fatalf("%s: must not queue a walk-to", tc.name)
			}
		})
	}

	// Too far: beyond 20 tiles.
	_, far, farTarget := tradeWorld(t, game.Location{X: 3110, Y: 3524, Z: 0})
	if got := routePlayerOutcome(far, protocol.PlayerOption{Index: farTarget.GetIndex(), Option: 2}, tradeDeps(farTarget)); got.Reason != ReasonNoAction {
		t.Fatalf("far target = %+v, want noAction", got)
	}
	// Cross-plane.
	_, plane, planeTarget := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 1})
	if got := routePlayerOutcome(plane, protocol.PlayerOption{Index: planeTarget.GetIndex(), Option: 2}, tradeDeps(planeTarget)); got.Reason != ReasonNoAction {
		t.Fatalf("cross-plane = %+v, want noAction", got)
	}
	// Busy player: mid-trade with a third party.
	_, busy, busyTarget := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	third := game.NewPlayer("third", game.Location{X: 3089, Y: 3524, Z: 0})
	busy.Trading().RequestTrade(third)
	third.Trading().RequestTrade(busy)
	if busy.Trading().State() != game.TradeStateTradeScreen {
		t.Fatalf("setup: busy state = %v", busy.Trading().State())
	}
	if got := routePlayerOutcome(busy, protocol.PlayerOption{Index: busyTarget.GetIndex(), Option: 2}, tradeDeps(busyTarget)); got.Reason != ReasonNoAction {
		t.Fatalf("busy player = %+v, want noAction", got)
	}
}

func TestTradeRequestEmitHandled(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	// The seam's return value claims it.
	withTradeHookSeam(t, game.TradeHooks{
		EmitTradeRequest: func(e *game.TradeRequestEvent) bool { return true },
	})
	if got := routePlayerOutcome(a, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)); got.Reason != ReasonNoAction {
		t.Fatalf("handled emit = %+v, want noAction", got)
	}
	if a.Trading().State() != game.TradeStateNone || a.MovementQueue().WalkToPending() {
		t.Fatal("handled emit must not start a walk or request")
	}
}

func TestTradeRequestEmitHandledFlag(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	// The event's Handled flag claims it, even when the seam
	// returns false (the dual-claim pattern).
	withTradeHookSeam(t, game.TradeHooks{
		EmitTradeRequest: func(e *game.TradeRequestEvent) bool {
			e.Handled = true
			return false
		},
	})
	if got := routePlayerOutcome(a, protocol.PlayerOption{Index: b.GetIndex(), Option: 2}, tradeDeps(b)); got.Reason != ReasonNoAction {
		t.Fatalf("handled flag = %+v, want noAction", got)
	}
	if a.MovementQueue().WalkToPending() {
		t.Fatal("handled flag must not start a walk")
	}
}

func TestSendTradeRequestBusyMessages(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	sess := &capturingSession{}
	a.Session = sess

	// The requester is busy (mid-trade): their own line.
	third := game.NewPlayer("third", game.Location{X: 3089, Y: 3524, Z: 0})
	a.Trading().RequestTrade(third)
	third.Trading().RequestTrade(a)
	sendTradeRequest(a, b)
	if len(sess.gameMessages) != 1 || sess.gameMessages[0] != "You cannot do that right now." {
		t.Fatalf("messages = %v", sess.gameMessages)
	}
	a.Trading().CloseTrade()

	// The target is busy trading: the trading line. (The
	// decline above adds its own message in between.)
	third.Trading().RequestDelay().Stop()
	b.Trading().RequestTrade(third)
	third.Trading().RequestTrade(b)
	sendTradeRequest(a, b)
	if last := sess.gameMessages[len(sess.gameMessages)-1]; last != "That player is currently trading with someone else." {
		t.Fatalf("messages = %v", sess.gameMessages)
	}
	b.Trading().CloseTrade()

	// The target is busy another way (dead): the generic line.
	b.Skills().SetCurrentLevel(game.SkillHitpoints, 0, true)
	sendTradeRequest(a, b)
	if last := sess.gameMessages[len(sess.gameMessages)-1]; last != "That player is currently busy." {
		t.Fatalf("messages = %v", sess.gameMessages)
	}
	if a.Trading().State() != game.TradeStateNone {
		t.Fatal("busy target must not start a request")
	}
}

func TestSendTradeRequestRequiresLocal(t *testing.T) {
	// The target is registered and near but never entered the
	// local list: silence, no request.
	spawn := game.Location{X: 3089, Y: 3524, Z: 0}
	w := game.NewWorld(spawn)
	a := game.NewPlayer("alice", spawn)
	b := game.NewPlayer("bob", game.Location{X: 3090, Y: 3524, Z: 0})
	w.Players().Add(a)
	w.Players().Add(b)
	a.SetRegistered(true)
	b.SetRegistered(true)
	// No bucket rebuild, no local update: a's local list is empty.
	sess := &capturingSession{}
	a.Session = sess
	sendTradeRequest(a, b)
	if len(sess.gameMessages) != 0 {
		t.Fatalf("messages = %v, want silence", sess.gameMessages)
	}
	if a.Trading().State() != game.TradeStateNone {
		t.Fatal("non-local target must not start a request")
	}
}

func TestTradeBusy(t *testing.T) {
	p := testPlayer()
	if tradeBusy(p) {
		t.Fatal("fresh player must not be trade-busy")
	}
	if tradeBusy(nil) {
		// Nil reads busy (defensive; call sites guard nil first).
	} else {
		t.Fatal("nil player must read busy")
	}
	p.Skills().SetCurrentLevel(game.SkillHitpoints, 0, true)
	if !tradeBusy(p) {
		t.Fatal("dead player must be trade-busy")
	}
	p.Skills().SetCurrentLevel(game.SkillHitpoints, 10, true)
	// A requested trade is not busy (status NONE, no window).
	other := game.NewPlayer("other", game.Location{X: 3089, Y: 3524, Z: 0})
	p.Trading().RequestTrade(other)
	if tradeBusy(p) {
		t.Fatal("requested trade must not be trade-busy")
	}
	// An open trade window is busy.
	other.Trading().RequestTrade(p)
	if !tradeBusy(p) {
		t.Fatal("open trade window must be trade-busy")
	}
}

func TestRouteTradeAction(t *testing.T) {
	// Nil player: the dispatch guard's drop.
	if got := tradeOutcome(nil, protocol.TradeAction{Action: protocol.TradeActionAccept}); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}

	defs := &defTable{cfgs: map[int]defConfig{
		995: {stackable: true, tradeable: true, dropable: true, slot: -1},
	}}
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	a.SetItemDefinitions(defs)
	b.SetItemDefinitions(defs)
	a.Inventory().AddItem(game.NewItem(995, 100))
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	if a.Trading().State() != game.TradeStateTradeScreen {
		t.Fatalf("setup: state = %v", a.Trading().State())
	}

	// Offer 50 coins.
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionOffer, Slot: 0, Quantity: 50, ItemID: 995, HasItemID: true}); !got.Implemented {
		t.Fatalf("offer = %+v, want acted", got)
	}
	if a.Inventory().GetAmount(995) != 50 || len(a.Trading().OfferedItems()) != 1 {
		t.Fatalf("after offer: inventory=%d offers=%v", a.Inventory().GetAmount(995), a.Trading().OfferedItems())
	}
	// An offer without an item id, and a zero quantity, are
	// absorbed silently but still acted.
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionOffer, Slot: 0, Quantity: 10, HasItemID: false}); !got.Implemented {
		t.Fatalf("id-less offer = %+v, want acted", got)
	}
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionOffer, Slot: 0, Quantity: 0, ItemID: 995, HasItemID: true}); !got.Implemented {
		t.Fatalf("zero offer = %+v, want acted", got)
	}
	if a.Inventory().GetAmount(995) != 50 {
		t.Fatal("absorbed offers must not move items")
	}
	// Remove 20 coins from the offer.
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionRemove, Slot: 0, Quantity: 20, ItemID: 995, HasItemID: true}); !got.Implemented {
		t.Fatalf("remove = %+v, want acted", got)
	}
	if a.Inventory().GetAmount(995) != 70 || a.Trading().OfferedItems()[0].Amount() != 30 {
		t.Fatalf("after remove: inventory=%d offers=%v", a.Inventory().GetAmount(995), a.Trading().OfferedItems())
	}
	// Accept, then decline from the accepted screen: the
	// offers return.
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionAccept}); !got.Implemented {
		t.Fatalf("accept = %+v, want acted", got)
	}
	if a.Trading().State() != game.TradeStateAcceptedTradeScreen {
		t.Fatalf("state = %v, want accepted", a.Trading().State())
	}
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionDecline}); !got.Implemented {
		t.Fatalf("decline = %+v, want acted", got)
	}
	if a.Trading().State() != game.TradeStateNone || a.Inventory().GetAmount(995) != 100 {
		t.Fatalf("after decline: state=%v coins=%d", a.Trading().State(), a.Inventory().GetAmount(995))
	}

	// Confirm-screen actions route to the same methods.
	a.Trading().RequestDelay().Stop()
	b.Trading().RequestDelay().Stop()
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	a.Trading().ButtonDelay().Stop()
	b.Trading().ButtonDelay().Stop()
	tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionAccept})
	tradeOutcome(b, protocol.TradeAction{Action: protocol.TradeActionAccept})
	if a.Trading().State() != game.TradeStateConfirmScreen {
		t.Fatalf("setup: state = %v, want confirm", a.Trading().State())
	}
	if got := tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionConfirmDecline}); !got.Implemented {
		t.Fatalf("confirm decline = %+v, want acted", got)
	}
	if a.Trading().State() != game.TradeStateNone || b.Trading().State() != game.TradeStateNone {
		t.Fatal("confirm decline must close both sides")
	}
}

func TestRouteTradeActionConfirmAccept(t *testing.T) {
	_, a, b := tradeWorld(t, game.Location{X: 3090, Y: 3524, Z: 0})
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionAccept})
	tradeOutcome(b, protocol.TradeAction{Action: protocol.TradeActionAccept})
	// The button delay (1s, wall clock) blocks an immediate
	// confirm accept; stop it to stage the second click.
	a.Trading().ButtonDelay().Stop()
	b.Trading().ButtonDelay().Stop()
	tradeOutcome(a, protocol.TradeAction{Action: protocol.TradeActionConfirmAccept})
	b.Trading().ButtonDelay().Stop()
	tradeOutcome(b, protocol.TradeAction{Action: protocol.TradeActionConfirmAccept})
	if a.Trading().State() != game.TradeStateNone || b.Trading().State() != game.TradeStateNone {
		t.Fatalf("states = %v/%v, want NONE after confirm accepts", a.Trading().State(), b.Trading().State())
	}
}
