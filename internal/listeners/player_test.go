package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Player-option tests: the option-1 attack gates, the route
// intercept, attack initiation through the fake combat seam, the
// player-attack emit, the options 2-8 decode-drops, and the
// spell-on-player / item-on-NPC preamble drops
// (PlayerOptionPacketListener.ts, MagicOnPlayerPacketListener.ts,
// UseItemPacketListener.ts:146-197). Fixtures only: the fake
// lookup stands in for the world player list, and the fake combat
// seam stands in for the combat system.

type fakePlayerLookup struct {
	players map[int]*game.Player
}

func (l *fakePlayerLookup) Find(index int) *game.Player {
	if l.players == nil {
		return nil
	}
	return l.players[index]
}

// playerTestDeps wires a lookup returning target for index 7 (and
// nil elsewhere), the given hooks, and the given combat seam.
func playerTestDeps(target *game.Player, hooks PlayerHooks, seam func(*game.Player, *game.Mobile)) Deps {
	lookup := &fakePlayerLookup{players: map[int]*game.Player{}}
	if target != nil {
		lookup.players[7] = target
	}
	return Deps{Players: lookup, PlayerHooks: hooks, CombatAttack: seam}
}

func routePlayerOutcome(p *game.Player, msg protocol.PlayerOption, deps Deps) Outcome {
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
	return routePlayerOption(p, msg, deps, drop, acted, noAction)
}

func otherPlayer() *game.Player {
	return game.NewPlayer("otherplayer", game.Location{X: 3090, Y: 3524, Z: 0})
}

func TestPlayerOptionAttack(t *testing.T) {
	// Wired seams: the attack starts, the seam receives the
	// player and the target's Mobile, and the player-attack
	// emit fires after it (the emit observes the started
	// attack: the seam ran first).
	p := testPlayer()
	target := otherPlayer()
	deps := playerTestDeps(target, PlayerHooks{}, nil)
	var seamCalls int
	var emitSeen *PlayerAttackEvent
	var emitSeamCalls int
	deps.CombatAttack = func(attacker *game.Player, tgt *game.Mobile) {
		seamCalls++
		if attacker != p || tgt != &target.Mobile {
			t.Errorf("seam got (%v, %v), want (%v, %v)", attacker, tgt, p, &target.Mobile)
		}
	}
	deps.PlayerHooks.EmitPlayerAttack = func(ev *PlayerAttackEvent) {
		emitSeen = ev
		emitSeamCalls = seamCalls
	}
	if got := routePlayerOutcome(p, protocol.PlayerOption{Index: 7, Option: 1}, deps); !got.Implemented {
		t.Fatalf("attack = %+v, want acted", got)
	}
	if seamCalls != 1 {
		t.Fatalf("seam calls = %d, want 1", seamCalls)
	}
	if emitSeen == nil || emitSeen.Player != p || emitSeen.Target != target {
		t.Fatalf("emit event = %+v, want player/target", emitSeen)
	}
	if emitSeamCalls != 1 {
		t.Fatalf("emit fired after %d seam calls, want 1 (emit follows the attack)", emitSeamCalls)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("attack option must not start a walk-to")
	}

	// Unwired lookup: the target cannot be resolved — the
	// named drop.
	p = testPlayer()
	deps = Deps{CombatAttack: func(*game.Player, *game.Mobile) {}}
	if got := routePlayerOutcome(p, protocol.PlayerOption{Index: 7, Option: 1}, deps); got.Reason != ReasonPlayerOption {
		t.Fatalf("unwired lookup = %+v, want ReasonPlayerOption", got)
	}

	// Unwired combat seam: the named drop.
	p = testPlayer()
	deps = playerTestDeps(otherPlayer(), PlayerHooks{}, nil)
	if got := routePlayerOutcome(p, protocol.PlayerOption{Index: 7, Option: 1}, deps); got.Reason != ReasonPlayerOption {
		t.Fatalf("unwired seam = %+v, want ReasonPlayerOption", got)
	}

	// Nil player on option 1: the implemented-route rule.
	if got := routePlayerOutcome(nil, protocol.PlayerOption{Index: 7, Option: 1}, playerTestDeps(otherPlayer(), PlayerHooks{}, nil)); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
}

func TestPlayerOptionAttackGates(t *testing.T) {
	attack := protocol.PlayerOption{Index: 7, Option: 1}

	// Attacker at 0 hitpoints: silent (busy() degenerates to
	// the hitpoints term).
	p := testPlayer()
	p.Skills().SetCurrentLevel(game.SkillHitpoints, 0, false)
	deps := playerTestDeps(otherPlayer(), PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run for a dead attacker") })
	if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
		t.Fatalf("dead attacker = %+v, want noAction", got)
	}

	// Lookup miss: silent.
	p = testPlayer()
	deps = playerTestDeps(nil, PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run on a lookup miss") })
	if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
		t.Fatalf("lookup miss = %+v, want noAction", got)
	}

	// Self-target: silent.
	p = testPlayer()
	deps = playerTestDeps(p, PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run for self-target") })
	if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
		t.Fatalf("self-target = %+v, want noAction", got)
	}

	// Target at 0 hitpoints: silent.
	p = testPlayer()
	target := otherPlayer()
	target.Skills().SetCurrentLevel(game.SkillHitpoints, 0, false)
	deps = playerTestDeps(target, PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run for a dead target") })
	if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
		t.Fatalf("dead target = %+v, want noAction", got)
	}

	// Target without a SkillManager (a hand-built fixture,
	// never a world player): silent — aliveness cannot be
	// verified.
	p = testPlayer()
	deps = playerTestDeps(&game.Player{}, PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run for a skill-less target") })
	if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
		t.Fatalf("skill-less target = %+v, want noAction", got)
	}

	// Out-of-range options on the attack path itself
	// (hand-built messages only; the router never passes
	// them here): refused.
	p = testPlayer()
	deps = playerTestDeps(otherPlayer(), PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("seam must not run for an out-of-range option") })
	for _, option := range []int{0, 9} {
		if got := playerAttack(p, protocol.PlayerOption{Index: 7, Option: option}, deps); got != playerRefused {
			t.Fatalf("option %d = %v, want playerRefused", option, got)
		}
	}
}

func TestPlayerOptionAttackNoDistanceOrPlaneGate(t *testing.T) {
	// The listener has NO distance or plane check for option 1
	// (PlayerOptionPacketListener.ts:17-29): a target 100 tiles
	// away on another plane still starts the attack, and the
	// combat system (CombatFactory.validTarget) decides
	// permission. The far/plane cases are the substance of
	// the CombatFactory ownership documented in player.go.
	for name, loc := range map[string]game.Location{
		"far":   {X: 3089 + 100, Y: 3524, Z: 0},
		"plane": {X: 3090, Y: 3524, Z: 1},
	} {
		p := testPlayer()
		target := game.NewPlayer("otherplayer", loc)
		called := false
		deps := playerTestDeps(target, PlayerHooks{}, func(*game.Player, *game.Mobile) { called = true })
		if got := routePlayerOutcome(p, protocol.PlayerOption{Index: 7, Option: 1}, deps); !got.Implemented || !called {
			t.Fatalf("%s target = %+v, called=%v, want acted", name, got, called)
		}
	}
}

func TestPlayerOptionRouteIntercept(t *testing.T) {
	attack := protocol.PlayerOption{Index: 7, Option: 1}

	// A claimed route (return value or Handled flag) returns
	// silently: no attack, no player-attack emit.
	for name, claim := range map[string]func(*PlayerOptionRouteEvent) bool{
		"return": func(*PlayerOptionRouteEvent) bool { return true },
		"flag": func(ev *PlayerOptionRouteEvent) bool {
			ev.Handled = true
			return false
		},
	} {
		p := testPlayer()
		target := otherPlayer()
		var gotEvent *PlayerOptionRouteEvent
		hooks := PlayerHooks{EmitOptionRoute: func(ev *PlayerOptionRouteEvent) bool {
			gotEvent = ev
			return claim(ev)
		}}
		deps := playerTestDeps(target, hooks, func(*game.Player, *game.Mobile) { t.Errorf("%s: seam must not run on a claimed route", name) })
		deps.PlayerHooks.EmitPlayerAttack = func(*PlayerAttackEvent) { t.Errorf("%s: emit must not fire on a claimed route", name) }
		if got := routePlayerOutcome(p, attack, deps); got.Reason != ReasonNoAction {
			t.Fatalf("route %s = %+v, want noAction", name, got)
		}
		if gotEvent == nil || gotEvent.Player != p || gotEvent.Target != target || gotEvent.Option != 1 {
			t.Fatalf("route %s: event = %+v, want player/target/option 1", name, gotEvent)
		}
	}

	// An unclaimed route lets the attack proceed, and the
	// route event fires before the seam runs.
	p := testPlayer()
	target := otherPlayer()
	var routeOrder, seamOrder int
	counter := 0
	hooks := PlayerHooks{EmitOptionRoute: func(*PlayerOptionRouteEvent) bool {
		counter++
		routeOrder = counter
		return false
	}}
	deps := playerTestDeps(target, hooks, func(*game.Player, *game.Mobile) {
		counter++
		seamOrder = counter
	})
	if got := routePlayerOutcome(p, attack, deps); !got.Implemented {
		t.Fatalf("unclaimed route = %+v, want acted", got)
	}
	if routeOrder != 1 || seamOrder != 2 {
		t.Fatalf("order route=%d seam=%d, want route first", routeOrder, seamOrder)
	}

	// No route seam at all: the attack proceeds (nil means no
	// hooks, which degenerates to unclaimed).
	p = testPlayer()
	called := false
	deps = playerTestDeps(otherPlayer(), PlayerHooks{}, func(*game.Player, *game.Mobile) { called = true })
	if got := routePlayerOutcome(p, attack, deps); !got.Implemented || !called {
		t.Fatalf("no route seam = %+v, called=%v, want acted", got, called)
	}
}

func TestPlayerOptionOtherOptionsDrop(t *testing.T) {
	// Options 3-8 drop with their own reasons, without
	// touching the seams or queuing movement — including
	// with a nil player, where the pure-drop reason is kept.
	// Option 2 is implemented (trade.go); its routing is
	// pinned by the trade tests.
	want := map[int]DropReason{
		3: ReasonFollowPlayer,
		4: ReasonPlayerOption,
		5: ReasonPlayerOption,
		6: ReasonPlayerOption,
		7: ReasonPlayerOption,
		8: ReasonPlayerOption,
	}
	for option, reason := range want {
		p := testPlayer()
		deps := playerTestDeps(otherPlayer(), PlayerHooks{
			EmitOptionRoute: func(*PlayerOptionRouteEvent) bool {
				t.Errorf("option %d: route event must not fire", option)
				return false
			},
			EmitPlayerAttack: func(*PlayerAttackEvent) { t.Errorf("option %d: attack emit must not fire", option) },
		}, func(*game.Player, *game.Mobile) { t.Errorf("option %d: seam must not run", option) })
		msg := protocol.PlayerOption{Index: 7, Option: option}
		if got := routePlayerOutcome(p, msg, deps); got.Reason != reason {
			t.Fatalf("option %d = %+v, want %q", option, got, reason)
		}
		if p.MovementQueue().WalkToPending() {
			t.Fatalf("option %d: must not queue a walk-to", option)
		}
		if got := routePlayerOutcome(nil, msg, deps); got.Reason != reason {
			t.Fatalf("option %d nil player = %+v, want %q", option, got, reason)
		}
	}
}

func TestPlayerOptionOpcodesViaHandle(t *testing.T) {
	// All eight player_option opcodes decode to their option
	// and route (zero payloads: the fake lookup ignores the
	// decoded index, and the option comes from the opcode).
	// INTERACT decodes byte 0 as option 3 and any other byte
	// as option 2.
	opcodes := map[byte]int{
		protocol.NativeOpPlayer1: 1,
		protocol.NativeOpPlayer2: 2,
		protocol.NativeOpPlayer3: 3,
		protocol.NativeOpPlayer4: 4,
		protocol.NativeOpPlayer5: 5,
		protocol.NativeOpPlayer6: 6,
		protocol.NativeOpPlayer7: 7,
		protocol.NativeOpPlayer8: 8,
	}
	wantReason := map[int]DropReason{
		// Option 2 is implemented: the unregistered
		// fixture players refuse silently at the
		// registration gate (noAction), and the opcode
		// still decodes to option 2.
		2: ReasonNoAction, 3: ReasonFollowPlayer,
		4: ReasonPlayerOption, 5: ReasonPlayerOption,
		6: ReasonPlayerOption, 7: ReasonPlayerOption, 8: ReasonPlayerOption,
	}
	for opcode, option := range opcodes {
		p := testPlayer()
		called := false
		// The zero payloads decode index 0; the lookup
		// serves the target for every index here.
		deps := Deps{
			Players:      &fakePlayerLookup{players: map[int]*game.Player{0: otherPlayer()}},
			CombatAttack: func(*game.Player, *game.Mobile) { called = true },
		}
		got := Handle(p, game.TurnPacket{Opcode: opcode, Payload: zeros(3)}, 1, deps)
		if option == 1 {
			if !got.Implemented || !called {
				t.Fatalf("opcode %d = %+v, called=%v, want acted", opcode, got, called)
			}
			continue
		}
		if got.Reason != wantReason[option] {
			t.Fatalf("opcode %d = %+v, want %q", opcode, got, wantReason[option])
		}
	}
	for _, tc := range []struct {
		payload []byte
		reason  DropReason
	}{
		{[]byte{0, 0, 0}, ReasonFollowPlayer},
		// INTERACT byte 1 decodes as option 2 (trade);
		// the unregistered fixture refuses silently.
		{[]byte{1, 0, 0}, ReasonNoAction},
	} {
		p := testPlayer()
		deps := playerTestDeps(otherPlayer(), PlayerHooks{}, func(*game.Player, *game.Mobile) { t.Error("INTERACT must not attack") })
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInteract, Payload: tc.payload}, 1, deps)
		if got.Reason != tc.reason {
			t.Fatalf("INTERACT %v = %+v, want %q", tc.payload, got, tc.reason)
		}
	}
}

func TestSpellOnPlayerDropViaHandle(t *testing.T) {
	// Spell-on-player decodes and drops with
	// ReasonMagicOnPlayer (the spell front-end is NOT
	// STARTED — see listeners.go), queuing no movement.
	// Payload: ByteNeg + ShortLE + ShortLE + IntIME + ShortLE.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpPlayerT, Payload: zeros(11)}, 1, Deps{})
	if got.Reason != ReasonMagicOnPlayer {
		t.Fatalf("spell-on-player = %+v, want ReasonMagicOnPlayer", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("spell-on-player must not queue a walk-to")
	}
	// Nil player: the pure drop, no dereference.
	got = Handle(nil, game.TurnPacket{Opcode: protocol.NativeOpPlayerT, Payload: zeros(11)}, 1, Deps{})
	if got.Reason != ReasonMagicOnPlayer {
		t.Fatalf("spell-on-player nil player = %+v, want ReasonMagicOnPlayer", got)
	}
}

func TestItemOnNpcDropViaHandle(t *testing.T) {
	// Item-on-NPC decodes and drops with ReasonUseItem
	// (walkToEntity + unfired onItemOnNpc/onNpcRoute — see
	// useitem.go), queuing no movement. Payload: ShortAddLE
	// + Byte + IntME + ShortLE + ShortAddLE.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpNpcU, Payload: zeros(11)}, 1, Deps{})
	if got.Reason != ReasonUseItem {
		t.Fatalf("item-on-NPC = %+v, want ReasonUseItem", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("item-on-NPC must not queue a walk-to")
	}
	// Nil player: the pure drop, no dereference.
	got = Handle(nil, game.TurnPacket{Opcode: protocol.NativeOpNpcU, Payload: zeros(11)}, 1, Deps{})
	if got.Reason != ReasonUseItem {
		t.Fatalf("item-on-NPC nil player = %+v, want ReasonUseItem", got)
	}
}
