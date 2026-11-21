package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Object-action tests: gates, lookup, click-type normalization,
// walk-to-object, dispatch, and the silent no-handler fallback
// (ObjectActionPacketListener.ts:30-172). Fixtures only: the
// fake lookup and definition source stand in for the game
// registry and the definition Store.

type fakeObjectEntry struct {
	id   int
	loc  game.Location
	typ  int
	face int
}

func (e *fakeObjectEntry) ID() int                 { return e.id }
func (e *fakeObjectEntry) Location() game.Location { return e.loc }
func (e *fakeObjectEntry) Type() int               { return e.typ }
func (e *fakeObjectEntry) Face() int               { return e.face }

type objectFindCall struct {
	id  int
	loc game.Location
}

type fakeObjectLookup struct {
	find  func(player *game.Player, id int, loc game.Location) ObjectEntry
	calls []objectFindCall
}

func (l *fakeObjectLookup) Find(player *game.Player, id int, loc game.Location) ObjectEntry {
	l.calls = append(l.calls, objectFindCall{id, loc})
	if l.find == nil {
		return nil
	}
	return l.find(player, id, loc)
}

type fakeObjectDef struct {
	id           int
	name         string
	sizeX, sizeY int
	mask         int
	interactions []string
}

func (d *fakeObjectDef) ID() int                { return d.id }
func (d *fakeObjectDef) Name() string           { return d.name }
func (d *fakeObjectDef) SizeX() int             { return d.sizeX }
func (d *fakeObjectDef) SizeY() int             { return d.sizeY }
func (d *fakeObjectDef) BlockingMask() int      { return d.mask }
func (d *fakeObjectDef) Interactions() []string { return d.interactions }

type fakeObjectDefs struct {
	forPlayer map[int]*fakeObjectDef
	byID      map[int]*fakeObjectDef
}

func (d *fakeObjectDefs) ObjectForPlayer(id int, player *game.Player) (ObjectDefinition, bool) {
	_ = player
	def, ok := d.forPlayer[id]
	if !ok {
		return nil, false
	}
	return def, true
}

func (d *fakeObjectDefs) ObjectByID(id int) (ObjectDefinition, bool) {
	def, ok := d.byID[id]
	if !ok {
		return nil, false
	}
	return def, true
}

func chopDef() *fakeObjectDef {
	return &fakeObjectDef{
		id: 100, name: "Oak tree", sizeX: 1, sizeY: 1,
		interactions: []string{"Chop down", "Examine"},
	}
}

// objectTestDeps wires a lookup returning entry for every call
// and definitions resolving chopDef for id 100 (both
// forPlayer and byID), with the given hooks.
func objectTestDeps(entry ObjectEntry, hooks ObjectHooks) (Deps, *fakeObjectLookup, *fakeObjectDefs) {
	lookup := &fakeObjectLookup{find: func(player *game.Player, id int, loc game.Location) ObjectEntry {
		return entry
	}}
	defs := &fakeObjectDefs{
		forPlayer: map[int]*fakeObjectDef{100: chopDef()},
		byID:      map[int]*fakeObjectDef{100: chopDef()},
	}
	return Deps{Objects: lookup, ObjectDefs: defs, ObjectHooks: hooks}, lookup, defs
}

func objectOutcomeFuncs() (drop func(DropReason) Outcome, acted, noAction func() Outcome) {
	drop = func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} }
	acted = func() Outcome { return Outcome{Recognized: true, Implemented: true} }
	noAction = func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} }
	return drop, acted, noAction
}

func routeObjectOutcome(p *game.Player, msg protocol.ObjectOption, cycle int64, deps Deps) Outcome {
	drop, acted, noAction := objectOutcomeFuncs()
	return routeObjectAction(p, msg, cycle, deps, drop, acted, noAction)
}

// adjacentObjectClick is a click on the tile east of testPlayer's
// spawn (3089,3524): adjacent, so the walk arrives on the first
// tick.
func adjacentObjectClick() protocol.ObjectOption {
	return protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, ClickType: 1, HasClickType: true}
}

func TestObjectActionUnwired(t *testing.T) {
	p := testPlayer()
	msg := adjacentObjectClick()
	// Neither seam wired.
	if got := routeObjectOutcome(p, msg, 1, Deps{}); got.Reason != ReasonObjectAction {
		t.Fatalf("unwired = %+v, want ReasonObjectAction", got)
	}
	// Only the lookup wired.
	lookup := &fakeObjectLookup{}
	if got := routeObjectOutcome(p, msg, 1, Deps{Objects: lookup}); got.Reason != ReasonObjectAction {
		t.Fatalf("lookup-only = %+v, want ReasonObjectAction", got)
	}
	// Only the definitions wired.
	defs := &fakeObjectDefs{}
	if got := routeObjectOutcome(p, msg, 1, Deps{ObjectDefs: defs}); got.Reason != ReasonObjectAction {
		t.Fatalf("defs-only = %+v, want ReasonObjectAction", got)
	}
	// A nil player reports ReasonNoPlayer, before the unwired check.
	if got := routeObjectOutcome(nil, msg, 1, Deps{}); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
}

func TestObjectActionDeadPlayer(t *testing.T) {
	p := testPlayer()
	killPlayer(p)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, _ := objectTestDeps(entry, ObjectHooks{})
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("dead player = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("dead player must not start a walk-to")
	}
}

func TestObjectActionLookupMissAndDefinitionMiss(t *testing.T) {
	p := testPlayer()
	// Lookup miss: silent, no walk-to.
	deps, _, _ := objectTestDeps(nil, ObjectHooks{})
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("lookup miss = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("lookup miss must not start a walk-to")
	}
	// Definition miss: the lookup finds the object but
	// forPlayer resolves nothing.
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, defs := objectTestDeps(entry, ObjectHooks{})
	delete(defs.forPlayer, 100)
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("definition miss = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("definition miss must not start a walk-to")
	}
}

func TestObjectActionPlayerPlane(t *testing.T) {
	// The packet carries no plane: the lookup runs on the
	// player's plane.
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 1})
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 1}, typ: 10}
	deps, lookup, _ := objectTestDeps(entry, ObjectHooks{})
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("plane-1 click = %+v, want acted", got)
	}
	if len(lookup.calls) == 0 || lookup.calls[0].loc.Z != 1 {
		t.Fatalf("lookup calls = %+v, want the first on plane 1", lookup.calls)
	}
}

func TestObjectActionClickTypeNormalization(t *testing.T) {
	cases := []struct {
		name      string
		msg       protocol.ObjectOption
		wantClick int // 0 means the packet is refused
		wantActed bool
	}{
		{
			"valid clickType ignores action text",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, ClickType: 2, HasClickType: true, Action: "Chop down", HasAction: true},
			2, true,
		},
		{
			"action text trims and lower-cases",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, Action: "  CHOP DOWN ", HasAction: true},
			1, true,
		},
		{
			"action text selects the second option",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, Action: "examine", HasAction: true},
			2, true,
		},
		{
			"unknown action text refuses",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, Action: "bogus", HasAction: true},
			0, false,
		},
		{
			"no clickType and no action refuses",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524},
			0, false,
		},
		{
			"invalid clickType falls back to action text",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, ClickType: 7, HasClickType: true, Action: "examine", HasAction: true},
			2, true,
		},
		{
			"invalid clickType without action refuses",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, ClickType: 7, HasClickType: true},
			0, false,
		},
		{
			"empty action text refuses",
			protocol.ObjectOption{ID: 100, X: 3090, Y: 3524, Action: "   ", HasAction: true},
			0, false,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := testPlayer()
			entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
			var seenClick int
			hooks := ObjectHooks{EmitObjectRoute: func(req *ObjectRouteRequest) {
				seenClick = req.ClickType
			}}
			deps, _, _ := objectTestDeps(entry, hooks)
			got := routeObjectOutcome(p, c.msg, 1, deps)
			if got.Implemented != c.wantActed {
				t.Fatalf("outcome = %+v, want acted=%v", got, c.wantActed)
			}
			if c.wantActed && seenClick != c.wantClick {
				t.Fatalf("route clickType = %d, want %d", seenClick, c.wantClick)
			}
			if !c.wantActed && p.MovementQueue().WalkToPending() {
				t.Fatal("refused packet must not start a walk-to")
			}
		})
	}
}

func TestObjectActionArrivalDispatch(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10, face: 1}
	var interaction *ObjectInteractionEvent
	var operated *ObjectOperatedEvent
	hooks := ObjectHooks{
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool {
			interaction = ev
			return true
		},
		EmitOperated: func(ev *ObjectOperatedEvent) { operated = ev },
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if interaction == nil {
		t.Fatal("arrival must emit the interaction")
	}
	if interaction.ObjectID != 100 || interaction.ClickType != 1 {
		t.Fatalf("interaction id/clickType = %d/%d, want 100/1", interaction.ObjectID, interaction.ClickType)
	}
	if interaction.Option != "chop down" || !interaction.HasOption {
		t.Fatalf("interaction option = %q (has=%v), want \"chop down\" (has=true)", interaction.Option, interaction.HasOption)
	}
	// Location is the OBJECT's tile; SourceLocation is the
	// player's tile at arrival.
	if interaction.Location != entry.loc {
		t.Fatalf("interaction location = %+v, want the object tile %+v", interaction.Location, entry.loc)
	}
	if interaction.SourceLocation != p.Location() {
		t.Fatalf("interaction source = %+v, want the arrival tile %+v", interaction.SourceLocation, p.Location())
	}
	if interaction.Definition == nil || interaction.Definition.Name() != "Oak tree" {
		t.Fatal("interaction must carry the resolved definition")
	}
	if operated == nil {
		t.Fatal("a claimed interaction must emit operated")
	}
	if operated.ObjectID != 100 || operated.Option != "chop down" || operated.Location != entry.loc {
		t.Fatalf("operated = %+v, want id 100 option \"chop down\" at the object tile", operated)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("messages = %q, want none", session.gameMessages)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("walk-to state must clear after arrival")
	}
}

func TestObjectActionNoHandlerSilence(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	emitted := false
	hooks := ObjectHooks{
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool {
			emitted = true
			return false // No plugin claims it.
		},
		EmitOperated: func(ev *ObjectOperatedEvent) {
			t.Error("operated must not fire for an unclaimed interaction")
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if !emitted {
		t.Fatal("the interaction must still be emitted")
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("unhandled interaction must stay silent, got %q", session.gameMessages)
	}
	// With no hooks at all, the same silence holds.
	p2 := testPlayer()
	session2 := &capturingSession{}
	p2.Session = session2
	deps2, _, _ := objectTestDeps(entry, ObjectHooks{})
	routeObjectOutcome(p2, adjacentObjectClick(), 1, deps2)
	tickPlayerQueue(p2, 2)
	if len(session2.gameMessages) != 0 {
		t.Fatalf("no-hooks interaction must stay silent, got %q", session2.gameMessages)
	}
}

func TestObjectActionBankOptionSilence(t *testing.T) {
	// The bank branch is a milestone-8 gap: an unhandled
	// "bank" option opens nothing, emits nothing, says nothing.
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	hooks := ObjectHooks{
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool { return false },
		EmitOperated: func(ev *ObjectOperatedEvent) {
			t.Error("operated must not fire for the unimplemented bank branch")
		},
	}
	deps, _, defs := objectTestDeps(entry, hooks)
	bankDef := chopDef()
	bankDef.interactions = []string{"Bank", "Examine"}
	defs.forPlayer[100] = bankDef
	defs.byID[100] = bankDef
	if got := routeObjectOutcome(p, adjacentObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if len(session.gameMessages) != 0 {
		t.Fatalf("bank option must stay silent, got %q", session.gameMessages)
	}
}

func TestObjectStillStands(t *testing.T) {
	p := testPlayer()
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10, face: 1}
	lookup := &fakeObjectLookup{find: func(player *game.Player, id int, loc game.Location) ObjectEntry {
		return entry
	}}
	if !objectStillStands(lookup, p, 100, entry.loc, entry) {
		t.Fatal("the same instance still stands")
	}
	// A different instance with the same id/type/face stands.
	lookup.find = func(player *game.Player, id int, loc game.Location) ObjectEntry {
		return &fakeObjectEntry{id: 100, loc: entry.loc, typ: 10, face: 1}
	}
	if !objectStillStands(lookup, p, 100, entry.loc, entry) {
		t.Fatal("same id/type/face still stands")
	}
	// A different face does not.
	lookup.find = func(player *game.Player, id int, loc game.Location) ObjectEntry {
		return &fakeObjectEntry{id: 100, loc: entry.loc, typ: 10, face: 2}
	}
	if objectStillStands(lookup, p, 100, entry.loc, entry) {
		t.Fatal("a different face must not stand")
	}
	// A miss does not.
	lookup.find = func(player *game.Player, id int, loc game.Location) ObjectEntry { return nil }
	if objectStillStands(lookup, p, 100, entry.loc, entry) {
		t.Fatal("a lookup miss must not stand")
	}
	if objectStillStands(nil, p, 100, entry.loc, entry) {
		t.Fatal("a nil lookup must not stand")
	}
	if objectStillStands(lookup, p, 100, entry.loc, nil) {
		t.Fatal("a nil object must not stand")
	}
}

func TestObjectActionRemovalStopsWalk(t *testing.T) {
	// The object stands at click time, then is removed: the
	// walk stops silently and the interaction never fires.
	p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3092, Y: 3524, Z: 0}, typ: 10}
	standing := true
	lookup := &fakeObjectLookup{find: func(player *game.Player, id int, loc game.Location) ObjectEntry {
		if standing {
			return entry
		}
		return nil
	}}
	defs := &fakeObjectDefs{
		forPlayer: map[int]*fakeObjectDef{100: chopDef()},
		byID:      map[int]*fakeObjectDef{100: chopDef()},
	}
	emitted := false
	deps := Deps{Objects: lookup, ObjectDefs: defs, ObjectHooks: ObjectHooks{
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool {
			emitted = true
			return true
		},
	}}
	msg := protocol.ObjectOption{ID: 100, X: 3092, Y: 3524, ClickType: 1, HasClickType: true}
	if got := routeObjectOutcome(p, msg, 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	standing = false
	tickPlayerQueue(p, 2)
	if emitted {
		t.Fatal("a removed object must not emit the interaction")
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("removal must stop the walk")
	}
}

func TestObjectActionArrivalRecheckUsesPacketIDAndClickTile(t *testing.T) {
	// Variant case: the packet names id 200, the lookup
	// returns the base object 100. The walk-to validity check
	// re-looks up by the object's id/tile (still standing);
	// the arrival re-check uses the PACKET id and the CLICK
	// tile (gone) — so the interaction never fires.
	p := testPlayer()
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	clicks := 0
	lookup := &fakeObjectLookup{find: func(player *game.Player, id int, loc game.Location) ObjectEntry {
		if id == 200 {
			clicks++
			if clicks == 1 {
				return entry // The click-time lookup.
			}
			return nil // Gone by arrival.
		}
		return entry // The queue's (object id, object tile) checks.
	}}
	defs := &fakeObjectDefs{
		forPlayer: map[int]*fakeObjectDef{100: chopDef()},
		byID:      map[int]*fakeObjectDef{100: chopDef()},
	}
	emitted := false
	deps := Deps{Objects: lookup, ObjectDefs: defs, ObjectHooks: ObjectHooks{
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool {
			emitted = true
			return true
		},
	}}
	msg := protocol.ObjectOption{ID: 200, X: 3090, Y: 3524, ClickType: 1, HasClickType: true}
	if got := routeObjectOutcome(p, msg, 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if emitted {
		t.Fatal("the arrival re-check must use the packet id and click tile")
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("walk-to state must clear after the refused arrival")
	}
}

func TestObjectActionRouteDestinationRedirect(t *testing.T) {
	p := testPlayer()
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3095, Y: 3524, Z: 0}, typ: 10}
	dest := game.Location{X: 3091, Y: 3524, Z: 0}
	emitted := false
	hooks := ObjectHooks{
		EmitObjectRoute: func(req *ObjectRouteRequest) {
			req.Destination = &dest
		},
		EmitObjectInteraction: func(ev *ObjectInteractionEvent) bool {
			emitted = true
			return true
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	msg := protocol.ObjectOption{ID: 100, X: 3095, Y: 3524, ClickType: 1, HasClickType: true}
	if got := routeObjectOutcome(p, msg, 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	for cycle := int64(2); cycle < 8 && !emitted; cycle++ {
		tickPlayerQueue(p, cycle)
	}
	if !emitted {
		t.Fatal("the redirected walk must still run the arrival callback")
	}
	if p.Location() != dest {
		t.Fatalf("player ended at %+v, want the route destination %+v", p.Location(), dest)
	}
}

func TestObjectActionClickRunOverride(t *testing.T) {
	// A Ctrl-click inverts the run mode for the walk to the
	// object, as for pickup: over two ticks the Ctrl-clicked
	// player covers strictly more ground. Mirrors
	// TestClickRunOverrideAppliesToPickupWalk.
	walk := func(mods int) int {
		p := game.NewPlayer("testplayer", game.Location{X: 3089, Y: 3524, Z: 0})
		p.SetRunning(false)
		entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3095, Y: 3524, Z: 0}, typ: 10}
		deps, _, _ := objectTestDeps(entry, ObjectHooks{})
		msg := protocol.ObjectOption{ID: 100, X: 3095, Y: 3524, ClickType: 1, HasClickType: true, ModifierFlags: mods}
		if got := routeObjectOutcome(p, msg, 0, deps); !got.Implemented {
			t.Fatalf("route with mods %d = %+v", mods, got)
		}
		// Drive through Player.Process so the pending
		// override activates at the end of the turn, as in
		// production.
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

func locInteractPayload(id, x, y int, action string, clickType byte) []byte {
	var b []byte
	put16 := func(v int) { b = append(b, byte(v>>8), byte(v)) }
	put16(id)
	put16(x)
	put16(y)
	b = append(b, 0) // Skipped byte; TS reads and discards it.
	b = append(b, action...)
	b = append(b, 0) // Null-terminated action string.
	b = append(b, clickType)
	return b
}

func TestHandleLocInteractDispatch(t *testing.T) {
	// End-to-end dispatch: a LOC_INTERACT packet with an
	// action name (no click-type byte) routes through Handle,
	// normalizes the action to clickType 1, and starts the
	// walk.
	p := testPlayer()
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	var seenClick int
	hooks := ObjectHooks{EmitObjectRoute: func(req *ObjectRouteRequest) {
		seenClick = req.ClickType
	}}
	deps, _, _ := objectTestDeps(entry, hooks)
	payload := locInteractPayload(100, 3090, 3524, "Chop down", 0)
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientLocInteract, Payload: payload}, 1, deps)
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	if seenClick != 1 {
		t.Fatalf("route clickType = %d, want 1 (action-text normalization)", seenClick)
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("dispatch must start the walk-to")
	}
	// The same packet with no seams wired drops with the
	// object reason.
	got = Handle(p, game.TurnPacket{Opcode: protocol.ClientLocInteract, Payload: payload}, 1, Deps{})
	if got.Reason != ReasonObjectAction {
		t.Fatalf("unwired Handle = %+v, want ReasonObjectAction", got)
	}
}
