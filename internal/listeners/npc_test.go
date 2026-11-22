package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// NPC-action tests: gates, lookup, distance, the combat
// gate/attack (attackable/hitpoints gates, the attack-option
// gate, the combat seam), walk-to-NPC, arrival dispatch
// (dialogue seam, interaction emit, operated emit, bank fallback,
// fallback message), the dialogue_continue resume path, and
// the widget_action dialogue-option tail
// (NPCOptionPacketListener.ts, NetworkBuilder.ts:908-951,
// 1140-1143). Fixtures only: the fake lookup and definition
// source stand in for the world NPC list and the definition
// Store, and the fake dialogue seam stands in for Worker B's
// DialogueManager.

type fakeNpcDef struct {
	id         int
	name       string
	actions    []string
	size       int
	attackable bool
}

func (d *fakeNpcDef) ID() int            { return d.id }
func (d *fakeNpcDef) Name() string       { return d.name }
func (d *fakeNpcDef) Actions() []string  { return d.actions }
func (d *fakeNpcDef) Size() int          { return d.size }
func (d *fakeNpcDef) IsAttackable() bool { return d.attackable }

type fakeNpcDefs struct {
	defs map[int]*fakeNpcDef
}

func (d *fakeNpcDefs) NpcForPlayer(id int, player *game.Player) (NpcDefinition, bool) {
	_ = player
	def, ok := d.defs[id]
	if !ok {
		return nil, false
	}
	return def, true
}

type fakeNpcLookup struct {
	find func(index int) *game.NPC
}

func (l *fakeNpcLookup) Find(index int) *game.NPC {
	if l.find == nil {
		return nil
	}
	return l.find(index)
}

func talkToDef() *fakeNpcDef {
	return &fakeNpcDef{id: 100, name: "Bob", actions: []string{"Talk-to", "Trade"}, size: 1}
}

func attackableDef() *fakeNpcDef {
	return &fakeNpcDef{id: 200, name: "Goblin", actions: []string{"Attack"}, size: 1, attackable: true}
}

// npcTestDeps wires a lookup returning npc for every call and
// definitions resolving def for npc.ID, with the given hooks.
func npcTestDeps(npc *game.NPC, def *fakeNpcDef, hooks NpcHooks) Deps {
	lookup := &fakeNpcLookup{find: func(index int) *game.NPC { return npc }}
	defs := &fakeNpcDefs{defs: map[int]*fakeNpcDef{}}
	if def != nil {
		defs.defs[def.id] = def
	}
	return Deps{NPCs: lookup, NpcDefs: defs, NpcHooks: hooks}
}

func routeNpcOutcome(p *game.Player, msg protocol.NpcOption, cycle int64, deps Deps) Outcome {
	drop := func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted := func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction := func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
	return routeNpcAction(p, msg, cycle, deps, drop, acted, noAction)
}

// adjacentNpcClick is a click on the NPC at index 300, standing
// on the tile east of testPlayer's spawn (3089,3524).
func adjacentNpcClick() protocol.NpcOption {
	return protocol.NpcOption{Index: 300, ClickType: 1}
}

func adjacentNpc() *game.NPC {
	// A live NPC: the spawn path gives every world NPC
	// hitpoints above 0 (DefaultNPCHitpoints 10), so a
	// fixture NPC that must reach arrival carries 10 — the
	// arrival closure treats 0 or below as dead/dying
	// (MovementQueue.ts:1696-1704). It is registered, as
	// every spawned NPC is: walkToEntity's targetValid
	// (MovementQueue.ts:1525) refuses an unregistered
	// target, so an unregistered fixture would never
	// arrive.
	npc := game.NewNPC(100, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	npc.SetRegistered(true)
	return npc
}

// drivePlayer runs player ticks until done reports true or the
// tick budget runs out.
func drivePlayer(t *testing.T, p *game.Player, done func() bool, maxTicks int) {
	t.Helper()
	for cycle := int64(2); cycle < int64(2+maxTicks); cycle++ {
		p.Process(cycle)
		if done() {
			return
		}
	}
	t.Fatalf("condition not met after %d ticks (player at %+v)", maxTicks, p.Location())
}

func TestNpcActionUnwired(t *testing.T) {
	p := testPlayer()
	msg := adjacentNpcClick()
	if got := routeNpcOutcome(p, msg, 1, Deps{}); got.Reason != ReasonNpcOption {
		t.Fatalf("unwired = %+v, want ReasonNpcOption", got)
	}
	lookup := &fakeNpcLookup{}
	if got := routeNpcOutcome(p, msg, 1, Deps{NPCs: lookup}); got.Reason != ReasonNpcOption {
		t.Fatalf("lookup-only = %+v, want ReasonNpcOption", got)
	}
	defs := &fakeNpcDefs{defs: map[int]*fakeNpcDef{}}
	if got := routeNpcOutcome(p, msg, 1, Deps{NpcDefs: defs}); got.Reason != ReasonNpcOption {
		t.Fatalf("defs-only = %+v, want ReasonNpcOption", got)
	}
	// A nil player reports ReasonNpcOption, before the unwired
	// check (pinned behavior; see routeNpcAction).
	if got := routeNpcOutcome(nil, msg, 1, Deps{}); got.Reason != ReasonNpcOption {
		t.Fatalf("nil player = %+v, want ReasonNpcOption", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("unwired packet must not start a walk-to")
	}
}

func TestNpcActionGates(t *testing.T) {
	// Dead player.
	p := testPlayer()
	killPlayer(p)
	deps := npcTestDeps(adjacentNpc(), talkToDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("dead player = %+v, want noAction", got)
	}
	// Lookup miss: silent, no walk-to.
	p = testPlayer()
	deps = npcTestDeps(nil, talkToDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("lookup miss = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("lookup miss must not start a walk-to")
	}
	// Beyond 24 tiles: silent. The 24-tile boundary itself
	// is visible and starts the walk.
	p = testPlayer()
	far := game.NewNPC(100, game.Location{X: 3089 + 25, Y: 3524, Z: 0})
	deps = npcTestDeps(far, talkToDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("25 tiles away = %+v, want noAction", got)
	}
	p = testPlayer()
	edge := game.NewNPC(100, game.Location{X: 3089 + 24, Y: 3524, Z: 0})
	deps = npcTestDeps(edge, talkToDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("24 tiles away = %+v, want acted", got)
	}
	// Other plane: silent (the visibility test requires the
	// same plane; the separate plane check sits behind it).
	p = testPlayer()
	above := game.NewNPC(100, game.Location{X: 3090, Y: 3524, Z: 1})
	deps = npcTestDeps(above, talkToDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("other plane = %+v, want noAction", got)
	}
	// Out-of-range click types (hand-built messages only;
	// the decoder never produces them).
	p = testPlayer()
	deps = npcTestDeps(adjacentNpc(), talkToDef(), NpcHooks{})
	for _, clickType := range []int{0, 6} {
		msg := protocol.NpcOption{Index: 300, ClickType: clickType}
		if got := routeNpcOutcome(p, msg, 1, deps); got.Reason != ReasonNoAction {
			t.Fatalf("clickType %d = %+v, want noAction", clickType, got)
		}
	}
}

func TestNpcActionCombatOption(t *testing.T) {
	// Attack on a live attackable NPC with no combat seam
	// wired: the named drop, and no walk starts (TS attacks
	// before walking; the combat system routes the approach).
	p := testPlayer()
	npc := game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	deps := npcTestDeps(npc, attackableDef(), NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNpcOption {
		t.Fatalf("attack unwired = %+v, want ReasonNpcOption", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("combat option must not start a walk-to")
	}

	// Attack through the fake combat seam: acted, the seam
	// receives the player and the NPC's Mobile, and still no
	// walk starts.
	p = testPlayer()
	npc = game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	deps = npcTestDeps(npc, attackableDef(), NpcHooks{})
	var gotAttacker *game.Player
	var gotTarget *game.Mobile
	deps.CombatAttack = func(attacker *game.Player, target *game.Mobile) {
		gotAttacker, gotTarget = attacker, target
	}
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("attack = %+v, want acted", got)
	}
	if gotAttacker != p || gotTarget != &npc.Mobile {
		t.Fatalf("seam got (%v, %v), want (%v, %v)", gotAttacker, gotTarget, p, &npc.Mobile)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("combat option must not start a walk-to")
	}

	// Hitpoints at or below 0: silent, as TS returns silently
	// when the combat gate fails (the spawn path gives every
	// world NPC hitpoints above 0, so 0 or below means
	// dead/dying).
	for _, hp := range []int{0, -5} {
		p = testPlayer()
		npc = game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = hp
		deps = npcTestDeps(npc, attackableDef(), NpcHooks{})
		called := false
		deps.CombatAttack = func(*game.Player, *game.Mobile) { called = true }
		if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
			t.Fatalf("hitpoints %d = %+v, want noAction", hp, got)
		}
		if called {
			t.Fatalf("hitpoints %d: combat seam must not run", hp)
		}
	}

	// A claimed attack-option gate (return value or Handled
	// flag) returns silently without attacking.
	for name, claim := range map[string]func(*NpcAttackOptionEvent) bool{
		"return": func(*NpcAttackOptionEvent) bool { return true },
		"flag": func(ev *NpcAttackOptionEvent) bool {
			ev.Handled = true
			return false
		},
	} {
		p = testPlayer()
		npc = game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
		npc.Hitpoints = 10
		var gotEvent *NpcAttackOptionEvent
		hooks := NpcHooks{EmitNpcAttackOption: func(ev *NpcAttackOptionEvent) bool {
			gotEvent = ev
			return claim(ev)
		}}
		deps = npcTestDeps(npc, attackableDef(), hooks)
		called := false
		deps.CombatAttack = func(*game.Player, *game.Mobile) { called = true }
		if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
			t.Fatalf("gate %s = %+v, want noAction", name, got)
		}
		if called {
			t.Fatalf("gate %s: combat seam must not run", name)
		}
		if gotEvent == nil || gotEvent.Player != p || gotEvent.NPC != npc || gotEvent.NpcID != 200 || gotEvent.ClickType != 1 {
			t.Fatalf("gate %s: event = %+v, want player/NPC/id 200/clickType 1", name, gotEvent)
		}
	}

	// An unclaimed gate lets the attack proceed.
	p = testPlayer()
	npc = game.NewNPC(200, game.Location{X: 3090, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	deps = npcTestDeps(npc, attackableDef(), NpcHooks{EmitNpcAttackOption: func(*NpcAttackOptionEvent) bool { return false }})
	called := false
	deps.CombatAttack = func(*game.Player, *game.Mobile) { called = true }
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented || !called {
		t.Fatalf("unclaimed gate = %+v, called=%v, want acted", got, called)
	}

	// Attack on a non-attackable NPC: silent, as TS returns
	// silently when the combat gate fails.
	p = testPlayer()
	def := &fakeNpcDef{id: 100, name: "Bob", actions: []string{"Attack"}, size: 1, attackable: false}
	deps = npcTestDeps(adjacentNpc(), def, NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("attack on non-attackable = %+v, want noAction", got)
	}
	// No definition resolves: the option is absent, so the
	// click is not a combat option and the walk proceeds.
	p = testPlayer()
	deps = npcTestDeps(adjacentNpc(), nil, NpcHooks{})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("definition miss = %+v, want acted", got)
	}
}

func TestNpcActionTalkToArrival(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	var started *NpcDialogueRequest
	var operated *NpcOperatedEvent
	hooks := NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool {
			started = req
			return true
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = ev },
	}
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	drivePlayer(t, p, func() bool { return started != nil }, 10)
	if started.NpcID != 100 || started.NpcIndex != 300 || started.ClickType != 1 {
		t.Fatalf("dialogue request id/index/click = %d/%d/%d, want 100/300/1", started.NpcID, started.NpcIndex, started.ClickType)
	}
	if started.Option != "talk-to" || !started.HasOption {
		t.Fatalf("dialogue option = %q (has=%v), want \"talk-to\" (has=true)", started.Option, started.HasOption)
	}
	if started.Location != (game.Location{X: 3090, Y: 3524, Z: 0}) {
		t.Fatalf("dialogue location = %+v, want the NPC tile", started.Location)
	}
	if started.Definition == nil || started.Definition.Name() != "Bob" {
		t.Fatal("dialogue request must carry the resolved definition")
	}
	if operated == nil {
		t.Fatal("a started dialogue must emit operated")
	}
	if operated.NpcID != 100 || operated.Option != "talk-to" || !operated.HasOption {
		t.Fatalf("operated = %+v, want id 100 option \"talk-to\"", operated)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("walk-to state must clear after arrival")
	}
}

func TestNpcActionWalkToDistantNpc(t *testing.T) {
	p := testPlayer()
	npc := game.NewNPC(100, game.Location{X: 3095, Y: 3524, Z: 0})
	npc.Hitpoints = 10 // live NPC (see adjacentNpc)
	npc.SetRegistered(true)
	started := false
	hooks := NpcHooks{StartDialogue: func(req *NpcDialogueRequest) bool { started = true; return true }}
	deps := npcTestDeps(npc, talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("distant click must start a walk-to")
	}
	drivePlayer(t, p, func() bool { return started }, 20)
	// The player walked: adjacent to (or on) the NPC tile.
	if !p.Location().IsWithinDistance(npc.Location(), 1) {
		t.Fatalf("player ended at %+v, want within 1 of the NPC at %+v", p.Location(), npc.Location())
	}
}

func TestNpcActionNoHandlerMessage(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	operated := false
	hooks := NpcHooks{
		StartDialogue:      func(req *NpcDialogueRequest) bool { return false },
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool { return false },
		EmitOperated:       func(ev *NpcOperatedEvent) { operated = true },
	}
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	drivePlayer(t, p, func() bool { return len(session.gameMessages) > 0 }, 10)
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Nothing interesting happens." {
		t.Fatalf("messages = %q, want [\"Nothing interesting happens.\"]", session.gameMessages)
	}
	if operated {
		t.Fatal("the fallback message must not emit operated")
	}
}

func TestNpcActionBankFallback(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	operated := false
	hooks := NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { return false },
		EmitOperated:  func(ev *NpcOperatedEvent) { operated = true },
	}
	def := &fakeNpcDef{id: 100, name: "Banker", actions: []string{"Bank"}, size: 1}
	deps := npcTestDeps(adjacentNpc(), def, hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	// Drive past arrival: nothing observable may happen.
	for cycle := int64(2); cycle < 8; cycle++ {
		p.Process(cycle)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none (bank fallback stays silent)", session.gameMessages)
	}
	if operated {
		t.Fatal("the bank fallback must not emit operated")
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("walk-to state must clear after arrival")
	}
}

func TestNpcActionInteractionClaim(t *testing.T) {
	// A claimed interaction (return value) emits operated
	// and sends no message.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	var interaction *NpcInteractionEvent
	var operated *NpcOperatedEvent
	hooks := NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { return false },
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool {
			interaction = ev
			return true
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = ev },
	}
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	drivePlayer(t, p, func() bool { return interaction != nil }, 10)
	if interaction.NpcID != 100 || interaction.ClickType != 1 || interaction.Location != adjacentNpc().Location() {
		t.Fatalf("interaction = %+v, want id 100 click 1 at the NPC tile", interaction)
	}
	if operated == nil {
		t.Fatal("a claimed interaction must emit operated")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}

	// A claim through the Handled flag alone is enough.
	p = testPlayer()
	operated = nil
	hooks = NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { return false },
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool {
			ev.Handled = true
			return false
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = ev },
	}
	deps = npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	drivePlayer(t, p, func() bool { return operated != nil }, 10)
}

func TestNpcActionArrivalRecheck(t *testing.T) {
	// The NPC leaves before arrival: the interaction drops
	// silently, with no dialogue start and no message.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	npc := adjacentNpc()
	calls := 0
	lookup := &fakeNpcLookup{find: func(index int) *game.NPC {
		calls++
		if calls == 1 {
			return npc
		}
		return nil
	}}
	defs := &fakeNpcDefs{defs: map[int]*fakeNpcDef{100: talkToDef()}}
	started := false
	deps := Deps{NPCs: lookup, NpcDefs: defs, NpcHooks: NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { started = true; return true },
	}}
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	for cycle := int64(2); cycle < 8; cycle++ {
		p.Process(cycle)
	}
	if started {
		t.Fatal("a departed NPC must not start a dialogue")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestNpcActionArrivalWanderingNpcStillInteracts(t *testing.T) {
	// A wandering NPC that steps off its clicked tile mid-walk
	// still gets its interaction (MovementQueue.ts:1525:
	// targetValid checks registration, life, plane, and type —
	// never the tile). Phase 41 live-found: Trade on Shop
	// keeper 2821 never opened the shop because the arrival
	// closure compared tiles.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	npc := adjacentNpc()
	npc.Hitpoints = 10
	started := false
	deps := npcTestDeps(npc, talkToDef(), NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { started = true; return true },
	})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	// The NPC wanders one tile north before the player arrives.
	npc.SetLocation(game.Location{X: 3090, Y: 3525, Z: 0})
	drivePlayer(t, p, func() bool { return started }, 10)
	if !started {
		t.Fatal("a wandering NPC must still start its dialogue")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestNpcActionArrivalTransformedNpcDrops(t *testing.T) {
	// An NPC that changes type mid-walk (a sheep sheared by
	// someone else) is no longer what was clicked: the
	// interaction drops silently (MovementQueue.ts:1521-1525).
	// The player starts 4 tiles out so the walk defers the
	// arrival: an already-adjacent click resolves at once
	// (MovementQueue.ts:1535-1539), leaving no mid-walk
	// window for the transform.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	npc := game.NewNPC(100, game.Location{X: 3093, Y: 3524, Z: 0})
	npc.Hitpoints = 10
	npc.SetRegistered(true)
	started := false
	deps := npcTestDeps(npc, talkToDef(), NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { started = true; return true },
	})
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	npc.SetNpcTransformationID(200)
	for cycle := int64(2); cycle < 8; cycle++ {
		p.Process(cycle)
	}
	if started {
		t.Fatal("a transformed NPC must not start a dialogue")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestNpcActionRouteHook(t *testing.T) {
	p := testPlayer()
	var seen *NpcRouteRequest
	hooks := NpcHooks{EmitNpcRoute: func(req *NpcRouteRequest) {
		seen = req
		req.Range = 3
	}}
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, adjacentNpcClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	if seen == nil {
		t.Fatal("the route hook must fire before walking")
	}
	if seen.NpcID != 100 || seen.NpcIndex != 300 || seen.ClickType != 1 {
		t.Fatalf("route request id/index/click = %d/%d/%d, want 100/300/1", seen.NpcID, seen.NpcIndex, seen.ClickType)
	}
	if seen.Location != adjacentNpc().Location() {
		t.Fatalf("route location = %+v, want the NPC tile", seen.Location)
	}
	if seen.Range != 3 {
		t.Fatalf("route range after hook = %d, want 3", seen.Range)
	}
	if seen.IgnoreProjectileClip {
		t.Fatal("ignoreProjectileClip must start false")
	}
}

func TestNpcActionClickRunOverride(t *testing.T) {
	// A Ctrl-click inverts the run mode for the walk to the
	// NPC, as for objects and pickups: over two ticks the
	// Ctrl-clicked player covers strictly more ground.
	walk := func(mods int) int {
		p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
		p.SetRunning(false)
		npc := game.NewNPC(100, game.Location{X: 3095, Y: 3524, Z: 0})
		// A live NPC, as the spawn path makes them:
		// registered with hitpoints, so walkToEntity's
		// targetValid accepts the walk.
		npc.Hitpoints = 10
		npc.SetRegistered(true)
		deps := npcTestDeps(npc, talkToDef(), NpcHooks{})
		msg := protocol.NpcOption{Index: 300, ClickType: 1, ModifierFlags: mods}
		if got := routeNpcOutcome(p, msg, 0, deps); !got.Implemented {
			t.Fatalf("route with mods %d = %+v", mods, got)
		}
		p.Process(1)
		p.Process(2)
		return p.Location().X
	}
	plain := walk(0)
	ctrl := walk(1)
	if ctrl <= plain {
		t.Errorf("Ctrl-click reached x=%d, plain click x=%d: override did not invert the run mode", ctrl, plain)
	}
}

func TestNpcOptionOpcodesViaHandle(t *testing.T) {
	// All five npc_option opcodes decode to their click type
	// and route: payloads use each branch's read order and
	// transforms (DecodeNpcOption), with NPC index 300.
	payloads := map[byte][]byte{
		protocol.NativeOpNpc1Alt: {0, 1, 0xAC},   // mods, ShortAddLE(300)
		protocol.NativeOpNpc2:    {1, 0xAC, 0},   // ShortAddLE(300), mods
		protocol.NativeOpNpc3:    {0xAC, 1, 0},   // ShortAdd(300), ByteNeg(0)
		protocol.NativeOpNpc4:    {0, 0x2C, 1},   // ByteNeg(0), ShortLE(300)
		protocol.NativeOpNpc1:    {128, 0x2C, 1}, // ByteAdd(0), ShortLE(300)
	}
	wantClick := map[byte]int{
		protocol.NativeOpNpc1Alt: 1,
		protocol.NativeOpNpc2:    2,
		protocol.NativeOpNpc3:    3,
		protocol.NativeOpNpc4:    4,
		protocol.NativeOpNpc1:    5,
	}
	for opcode, payload := range payloads {
		p := testPlayer()
		var seenClick int
		hooks := NpcHooks{StartDialogue: func(req *NpcDialogueRequest) bool {
			seenClick = req.ClickType
			return true
		}}
		def := &fakeNpcDef{id: 100, name: "Bob", actions: []string{"Talk-to", "Trade", "Examine", "Pickpocket", "Bank"}, size: 1}
		deps := npcTestDeps(adjacentNpc(), def, hooks)
		got := Handle(p, game.TurnPacket{Opcode: opcode, Payload: payload}, 1, deps)
		if !got.Implemented {
			t.Fatalf("opcode %d: got %+v, want acted", opcode, got)
		}
		drivePlayer(t, p, func() bool { return seenClick != 0 }, 10)
		if seenClick != wantClick[opcode] {
			t.Fatalf("opcode %d: clickType %d, want %d", opcode, seenClick, wantClick[opcode])
		}
	}
}

func TestSpellOnNpcDrop(t *testing.T) {
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpNpcT, Payload: zeros(11)}, 7, Deps{})
	if got != (Outcome{Recognized: true, Reason: ReasonNpcOption}) {
		t.Fatalf("spell on npc = %+v, want ReasonNpcOption", got)
	}
}

// fakeDialogueAccess records resume/option calls and answers
// from its fields. currentSeq, when non-empty, supplies the
// Current() answers in call order (the last entry repeats),
// so tests can stage a current entry that changes mid-click.
type fakeDialogueAccess struct {
	resumeResult bool
	resumeCalls  [][2]int32
	active       bool
	current      any
	currentSeq   []any
	currentCalls int
	optionCalls  []int
}

func (f *fakeDialogueAccess) Resume(p *game.Player, widgetID int32, childIndex int) bool {
	_ = p
	f.resumeCalls = append(f.resumeCalls, [2]int32{widgetID, int32(childIndex)})
	return f.resumeResult
}

func (f *fakeDialogueAccess) IsActive(p *game.Player) bool {
	_ = p
	return f.active
}

func (f *fakeDialogueAccess) Current(p *game.Player) any {
	_ = p
	f.currentCalls++
	if len(f.currentSeq) > 0 {
		i := f.currentCalls - 1
		if i >= len(f.currentSeq) {
			i = len(f.currentSeq) - 1
		}
		return f.currentSeq[i]
	}
	return f.current
}

func (f *fakeDialogueAccess) HandleOption(p *game.Player, option int) {
	_ = p
	f.optionCalls = append(f.optionCalls, option)
}

func TestDialogueResumeHandle(t *testing.T) {
	// Native opcode 62: childIndex shortAddLE, then widgetId
	// int. Child 3, widget (219<<16)|1 (the options widget).
	native := []byte{0, 3 + 128, 0, 0xDB, 0, 1}
	// High opcode 252: widgetId int, then childIndex short.
	high := []byte{0, 0xDB, 0, 1, 0, 3}
	for _, tc := range []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{"native", protocol.NativeResumePausebutton, native},
		{"high", protocol.ClientResumePausebutton, high},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testPlayer()
			access := &fakeDialogueAccess{resumeResult: true}
			deps := Deps{Dialogue: access}
			got := Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 1, deps)
			if !got.Implemented {
				t.Fatalf("got %+v, want acted", got)
			}
			if len(access.resumeCalls) != 1 {
				t.Fatalf("resume calls = %v, want one", access.resumeCalls)
			}
			if access.resumeCalls[0] != ([2]int32{(219 << 16) | 1, 3}) {
				t.Fatalf("resume args = %v, want widget %d child 3", access.resumeCalls[0], (219<<16)|1)
			}
			// A refused resume is no action, not a drop.
			access.resumeResult = false
			got = Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 1, deps)
			if got.Reason != ReasonNoAction {
				t.Fatalf("refused resume = %+v, want noAction", got)
			}
		})
	}
	// Unwired dialogue seam: named drop.
	p := testPlayer()
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeResumePausebutton, Payload: native}, 1, Deps{})
	if got.Reason != ReasonDialogueUnwired {
		t.Fatalf("unwired = %+v, want ReasonDialogueUnwired", got)
	}
	// Nil player: ReasonNoPlayer, after decode.
	got = Handle(nil, game.TurnPacket{Opcode: protocol.NativeResumePausebutton, Payload: native}, 1, Deps{Dialogue: &fakeDialogueAccess{}})
	if got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
	// Malformed payloads: recognized, malformed, never a
	// panic — empty, truncated, and over-long.
	for _, payload := range [][]byte{nil, {}, {0}, zeros(5), zeros(7)} {
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeResumePausebutton, Payload: payload}, 1, Deps{})
		if got.Reason != ReasonMalformedPayload {
			t.Fatalf("payload len %d = %+v, want malformed", len(payload), got)
		}
	}
	// dialogue_amount / dialogue_input do NOT resume: they
	// answer prompts (prompts.go), so with no action armed
	// they are ReasonNoAction — never a resume call.
	access := &fakeDialogueAccess{resumeResult: true}
	for _, opcode := range []byte{protocol.ClientResumeCountdialog, protocol.ClientResumeNamedialog, protocol.ClientResumeStringdialog} {
		got := Handle(p, game.TurnPacket{Opcode: opcode, Payload: zeros(4)}, 1, Deps{Dialogue: access})
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("opcode %d = %+v, want recognized no-action", opcode, got)
		}
	}
	if len(access.resumeCalls) != 0 {
		t.Fatalf("resume calls = %v, want none", access.resumeCalls)
	}
}

func TestDialogueResumeDecodeOrders(t *testing.T) {
	// Distinctive values prove the read orders differ:
	// child 0x1234, widget 0x00DB0001.
	native := []byte{0x12, 0x34 + 128, 0, 0xDB, 0, 1}
	high := []byte{0, 0xDB, 0, 1, 0x12, 0x34}
	for _, tc := range []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{"native", protocol.NativeResumePausebutton, native},
		{"high", protocol.ClientResumePausebutton, high},
	} {
		t.Run(tc.name, func(t *testing.T) {
			access := &fakeDialogueAccess{resumeResult: true}
			Handle(testPlayer(), game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 1, Deps{Dialogue: access})
			if len(access.resumeCalls) != 1 || access.resumeCalls[0] != ([2]int32{(219 << 16) | 1, 0x1234}) {
				t.Fatalf("resume args = %v, want widget %d child 0x1234", access.resumeCalls, (219<<16)|1)
			}
		})
	}
	// An unsupported opcode is a decoder error, not a guess.
	if _, err := protocol.DecodeDialogueContinue(99, zeros(6)); err == nil {
		t.Fatal("DecodeDialogueContinue(99) must error")
	}
}

func TestWidgetActionDialogueOptionTail(t *testing.T) {
	// A widget_action click on a non-equipment, non-inventory
	// widget (group 500) falls through to the interface-action
	// emit, which runs unclaimed (ReasonNoAction); the
	// dialogue tail still runs. IF_BUTTON1 carries
	// buttonNum 1.
	widgetPayload := func() []byte {
		// widgetId int, slot short 0xffff, itemId short 0xffff.
		return []byte{0x01, 0xF4, 0, 1, 0xFF, 0xFF, 0xFF, 0xFF}
	}
	entry := &struct{ id int }{id: 1}

	// Dialogue active, current unchanged, button 1: option 0.
	p := testPlayer()
	access := &fakeDialogueAccess{active: true, current: entry}
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want ReasonNoAction", got)
	}
	if len(access.optionCalls) != 1 || access.optionCalls[0] != 0 {
		t.Fatalf("option calls = %v, want [0]", access.optionCalls)
	}

	// Button 5 selects option 4 (IF_BUTTON5).
	p = testPlayer()
	access = &fakeDialogueAccess{active: true, current: entry}
	got = Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton5, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want ReasonNoAction", got)
	}
	if len(access.optionCalls) != 1 || access.optionCalls[0] != 4 {
		t.Fatalf("option calls = %v, want [4]", access.optionCalls)
	}

	// Button 6 is outside 1..5: no option call.
	p = testPlayer()
	access = &fakeDialogueAccess{active: true, current: entry}
	Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton6, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if len(access.optionCalls) != 0 {
		t.Fatalf("option calls = %v, want none (button 6)", access.optionCalls)
	}

	// Dialogue inactive: no option call.
	p = testPlayer()
	access = &fakeDialogueAccess{active: false, current: entry}
	Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if len(access.optionCalls) != 0 {
		t.Fatalf("option calls = %v, want none (inactive)", access.optionCalls)
	}

	// Current entry changed during the click: no option call.
	p = testPlayer()
	other := &struct{ id int }{id: 2}
	access = &fakeDialogueAccess{active: true, currentSeq: []any{entry, other}}
	Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if len(access.optionCalls) != 0 {
		t.Fatalf("option calls = %v, want none (current changed)", access.optionCalls)
	}

	// No dialogue open at click time (nil token), one open
	// after: no option call — a dialogue opened by the click
	// must not take the same click as its option.
	p = testPlayer()
	access = &fakeDialogueAccess{active: true, currentSeq: []any{nil, entry}}
	Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: widgetPayload()}, 1, Deps{Dialogue: access})
	if len(access.optionCalls) != 0 {
		t.Fatalf("option calls = %v, want none (opened by the click)", access.optionCalls)
	}

	// No dialogue seam: the tail is a no-op and the outcome
	// is unchanged.
	p = testPlayer()
	got = Handle(p, game.TurnPacket{Opcode: protocol.NativeIfButton1, Payload: widgetPayload()}, 1, Deps{})
	if got.Reason != ReasonNoAction {
		t.Fatalf("outcome = %+v, want ReasonNoAction", got)
	}
}

// Shop-dispatch tests: the core.shops.open static step in
// npcOperate (NpcInteractionManager.ts handle branch), driven
// through routeNpcAction with a fake interaction source and a
// fake OpenShop seam. Click type 2 on talkToDef is the "Trade"
// option; the shop step keys on (content id 100, click type 2).

type fakeNpcInteractions struct {
	actions map[[2]int]NpcInteractionAction
	lookups [][2]int
}

func (f *fakeNpcInteractions) InteractionAction(npcID, clickType int) (NpcInteractionAction, bool) {
	f.lookups = append(f.lookups, [2]int{npcID, clickType})
	action, ok := f.actions[[2]int{npcID, clickType}]
	return action, ok
}

func shopAction(shopID any) NpcInteractionAction {
	return NpcInteractionAction{Method: "core.shops.open", Args: map[string]any{"shopId": shopID}}
}

func tradeClick() protocol.NpcOption {
	return protocol.NpcOption{Index: 300, ClickType: 2}
}

func driveShopClick(t *testing.T, hooks NpcHooks) (*game.Player, *capturingSession) {
	t.Helper()
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, tradeClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	// Drive until arrival dispatch has run: the walk-to clears
	// on arrival for every outcome.
	drivePlayer(t, p, func() bool { return !p.MovementQueue().WalkToPending() }, 10)
	// One extra tick so the arrival closure's effects land.
	p.Process(12)
	return p, session
}

func TestNpcShopOpenClaimsClick(t *testing.T) {
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: shopAction(7),
	}}
	opened := []int{}
	dialogueStarted := false
	interactionRan := false
	var operated *NpcOperatedEvent
	hooks := NpcHooks{
		Interactions: interactions,
		OpenShop: func(p *game.Player, shopID int) bool {
			opened = append(opened, shopID)
			return true
		},
		StartDialogue: func(req *NpcDialogueRequest) bool {
			dialogueStarted = true
			return true
		},
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool {
			interactionRan = true
			return false
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = ev },
	}
	_, session := driveShopClick(t, hooks)
	if len(opened) != 1 || opened[0] != 7 {
		t.Fatalf("opened = %v, want [7]", opened)
	}
	if dialogueStarted {
		t.Fatal("a shop click must not start a dialogue")
	}
	if interactionRan {
		t.Fatal("a claimed shop click must not reach the interaction emit")
	}
	if operated == nil || operated.NpcID != 100 || operated.Option != "trade" || !operated.HasOption {
		t.Fatalf("operated = %+v, want id 100 option \"trade\"", operated)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
	if len(interactions.lookups) != 1 || interactions.lookups[0] != [2]int{100, 2} {
		t.Fatalf("lookups = %v, want one lookup for (100, 2)", interactions.lookups)
	}
}

func TestNpcShopOpenFailedFallsThroughWithoutDialogue(t *testing.T) {
	// Unknown shop: OpenShop returns false. Dispatch falls
	// through to the interaction emit WITHOUT StartDialogue
	// (the static position consumed the click), and with no
	// handler the fallback message appears.
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: shopAction(7),
	}}
	dialogueStarted := false
	interactionRan := false
	operated := false
	hooks := NpcHooks{
		Interactions: interactions,
		OpenShop:     func(p *game.Player, shopID int) bool { return false },
		StartDialogue: func(req *NpcDialogueRequest) bool {
			dialogueStarted = true
			return true
		},
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool {
			interactionRan = true
			return false
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = true },
	}
	_, session := driveShopClick(t, hooks)
	if dialogueStarted {
		t.Fatal("a failed shop open must not start a dialogue")
	}
	if !interactionRan {
		t.Fatal("a failed shop open must reach the interaction emit")
	}
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Nothing interesting happens." {
		t.Fatalf("messages = %q, want the fallback", session.gameMessages)
	}
	if operated {
		t.Fatal("the fallback must not emit operated")
	}
}

func TestNpcShopOpenUnwiredSeamFallsThrough(t *testing.T) {
	// A shop action with a nil OpenShop seam behaves like a
	// failed open: fall through to the emit, no panic, and
	// an interaction hook can still claim the click.
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: shopAction(7),
	}}
	operated := false
	hooks := NpcHooks{
		Interactions: interactions,
		// OpenShop deliberately nil.
		StartDialogue: func(req *NpcDialogueRequest) bool { return false },
		EmitNpcInteraction: func(ev *NpcInteractionEvent) bool {
			ev.Handled = true
			return true
		},
		EmitOperated: func(ev *NpcOperatedEvent) { operated = true },
	}
	_, session := driveShopClick(t, hooks)
	if !operated {
		t.Fatal("the interaction hook's claim must emit operated")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
}

func TestNpcShopGatedOpenClaimsClick(t *testing.T) {
	// A gated shop (ShopManager.open returns true when
	// emitCanShop answers false, with no interface opened)
	// claims the click: operated, no message, no dialogue.
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: shopAction(7),
	}}
	hooks := NpcHooks{
		Interactions: interactions,
		OpenShop:     func(p *game.Player, shopID int) bool { return true },
		StartDialogue: func(req *NpcDialogueRequest) bool {
			t.Error("a gated shop open must not start a dialogue")
			return false
		},
		EmitOperated: func(ev *NpcOperatedEvent) {},
	}
	_, session := driveShopClick(t, hooks)
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none (gated shops stay silent)", session.gameMessages)
	}
}

func TestNpcShopInvalidIDsFallThrough(t *testing.T) {
	// Fractional, non-numeric, negative, and absent ids all
	// fail the Number()/integer check: the click falls through
	// to the fallback, StartDialogue is skipped (the action
	// IS a shop action), and OpenShop is never called.
	cases := []struct {
		name string
		args map[string]any
	}{
		{"fractional", map[string]any{"shopId": 7.5}},
		{"non-numeric string", map[string]any{"shopId": "abc"}},
		{"negative", map[string]any{"shopId": -1}},
		{"negative string", map[string]any{"shopId": "-1"}},
		{"absent key", map[string]any{}},
		{"null args", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
				{100, 2}: {Method: "core.shops.open", Args: tc.args},
			}}
			openCalls := 0
			dialogueStarted := false
			hooks := NpcHooks{
				Interactions: interactions,
				OpenShop: func(p *game.Player, shopID int) bool {
					openCalls++
					return true
				},
				StartDialogue: func(req *NpcDialogueRequest) bool {
					dialogueStarted = true
					return true
				},
			}
			_, session := driveShopClick(t, hooks)
			if openCalls != 0 {
				t.Fatalf("open calls = %d, want 0", openCalls)
			}
			if dialogueStarted {
				t.Fatal("an invalid shop id must skip StartDialogue")
			}
			if len(session.gameMessages) != 1 || session.gameMessages[0] != "Nothing interesting happens." {
				t.Fatalf("messages = %q, want the fallback", session.gameMessages)
			}
		})
	}
}

func TestNpcShopStringIDOpens(t *testing.T) {
	// Number("7") is 7: a string shop id opens shop 7.
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: shopAction(" 7 "),
	}}
	opened := []int{}
	hooks := NpcHooks{
		Interactions: interactions,
		OpenShop: func(p *game.Player, shopID int) bool {
			opened = append(opened, shopID)
			return true
		},
		EmitOperated: func(ev *NpcOperatedEvent) {},
	}
	driveShopClick(t, hooks)
	if len(opened) != 1 || opened[0] != 7 {
		t.Fatalf("opened = %v, want [7]", opened)
	}
}

func TestNpcNonShopActionKeepsDialogue(t *testing.T) {
	// A dialogue action is not a shop action: StartDialogue
	// runs exactly as before, and OpenShop is never called.
	interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: {Method: "core.dialogues.start", Args: map[string]any{"dialogueId": 3}},
	}}
	openCalls := 0
	dialogueStarted := false
	hooks := NpcHooks{
		Interactions: interactions,
		OpenShop: func(p *game.Player, shopID int) bool {
			openCalls++
			return true
		},
		StartDialogue: func(req *NpcDialogueRequest) bool {
			dialogueStarted = true
			return true
		},
		EmitOperated: func(ev *NpcOperatedEvent) {},
	}
	driveShopClick(t, hooks)
	if openCalls != 0 {
		t.Fatalf("open calls = %d, want 0", openCalls)
	}
	if !dialogueStarted {
		t.Fatal("a non-shop action must reach StartDialogue")
	}
}

func TestNpcShopNoSourceKeepsBehavior(t *testing.T) {
	// Nil Interactions: no shop step, StartDialogue runs.
	hooks := NpcHooks{
		StartDialogue: func(req *NpcDialogueRequest) bool { return true },
		EmitOperated:  func(ev *NpcOperatedEvent) {},
	}
	p := testPlayer()
	deps := npcTestDeps(adjacentNpc(), talkToDef(), hooks)
	if got := routeNpcOutcome(p, tradeClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	drivePlayer(t, p, func() bool { return !p.MovementQueue().WalkToPending() }, 10)
}

func TestStaticShopIDNumberVectors(t *testing.T) {
	// Direct vectors for the Number()/integer reading in
	// staticShopID (NpcInteractionManager.ts handle).
	cases := []struct {
		name       string
		args       map[string]any
		wantShopID int
		wantValid  bool
	}{
		{"int", map[string]any{"shopId": 7}, 7, true},
		{"float whole", map[string]any{"shopId": 7.0}, 7, true},
		{"float fractional", map[string]any{"shopId": 7.5}, 0, false},
		{"string digits", map[string]any{"shopId": "7"}, 7, true},
		{"string padded", map[string]any{"shopId": " 7 "}, 7, true},
		{"string empty is zero", map[string]any{"shopId": ""}, 0, true},
		{"string non-numeric", map[string]any{"shopId": "abc"}, 0, false},
		{"string trailing garbage", map[string]any{"shopId": "7abc"}, 0, false},
		{"string hex", map[string]any{"shopId": "0x7"}, 7, true},
		{"string underscore", map[string]any{"shopId": "1_0"}, 0, false},
		{"bool true is one", map[string]any{"shopId": true}, 1, true},
		{"bool false is zero", map[string]any{"shopId": false}, 0, true},
		{"null is zero", map[string]any{"shopId": nil}, 0, true},
		{"absent is NaN", map[string]any{}, 0, false},
		{"negative", map[string]any{"shopId": -1}, 0, false},
		{"zero", map[string]any{"shopId": 0}, 0, true},
		{"object", map[string]any{"shopId": map[string]any{}}, 0, false},
		{"array", map[string]any{"shopId": []any{7}}, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			interactions := &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
				{100, 2}: {Method: "core.shops.open", Args: tc.args},
			}}
			deps := Deps{NpcHooks: NpcHooks{Interactions: interactions}}
			shopID, isShop, valid := staticShopID(deps, 100, 2)
			if !isShop {
				t.Fatal("isShop must be true for a core.shops.open action")
			}
			if valid != tc.wantValid || shopID != tc.wantShopID {
				t.Fatalf("staticShopID = (%d, %v), want (%d, %v)", shopID, valid, tc.wantShopID, tc.wantValid)
			}
		})
	}
	// Non-shop methods and missing actions report not-a-shop.
	deps := Deps{NpcHooks: NpcHooks{Interactions: &fakeNpcInteractions{actions: map[[2]int]NpcInteractionAction{
		{100, 2}: {Method: "core.player.teleport", Args: map[string]any{}},
	}}}}
	if _, isShop, _ := staticShopID(deps, 100, 2); isShop {
		t.Fatal("a teleport action must not report as a shop")
	}
	if _, isShop, _ := staticShopID(deps, 100, 1); isShop {
		t.Fatal("a missing action must not report as a shop")
	}
	if _, isShop, _ := staticShopID(Deps{}, 100, 2); isShop {
		t.Fatal("a nil source must not report as a shop")
	}
}
