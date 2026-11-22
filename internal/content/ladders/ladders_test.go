package ladders

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fakeClimber is a Climber seam fake: it records every request and
// reports configurable results. It stands in for Worker A's
// mechanism — these tests pin the plugin's routing, never the
// resolver.
type fakeClimber struct {
	climbs  []ClimbRequest
	crosses []ClimbRequest
	climbOK bool
	crossOK bool
}

func (f *fakeClimber) Climb(req ClimbRequest) bool {
	f.climbs = append(f.climbs, req)
	return f.climbOK
}

func (f *fakeClimber) CrossGangplank(req ClimbRequest) bool {
	f.crosses = append(f.crosses, req)
	return f.crossOK
}

// fakeObjectDef is a minimal plugins.ObjectDefinition with a chosen
// name and interactions (the doors test pattern).
type fakeObjectDef struct {
	id           int
	name         string
	interactions []string
}

func (d fakeObjectDef) ID() int                { return d.id }
func (d fakeObjectDef) Name() string           { return d.name }
func (d fakeObjectDef) SizeX() int             { return 1 }
func (d fakeObjectDef) SizeY() int             { return 1 }
func (d fakeObjectDef) BlockingMask() int      { return 0 }
func (d fakeObjectDef) Interactions() []string { return d.interactions }

// fakeSession is a minimal DialogueSender for the prompt tests: it
// embeds the (nil) SessionPort interface to satisfy the
// Player.Session field type and records the chatbox group so the
// prompt registry's ownership check sees group 219 mounted (the
// plugins dialogue-test pattern).
type fakeSession struct {
	game.SessionPort
	chatboxID int
}

func (s *fakeSession) SendChatboxInterface(groupID int, unclamp bool) { s.chatboxID = groupID }
func (s *fakeSession) SendNpcHeadOnInterface(int, int)                {}
func (s *fakeSession) SendPlayerHeadOnInterface(int)                  {}
func (s *fakeSession) SendInterfaceAnimation(int, int)                {}
func (s *fakeSession) SendString(string, int)                         {}
func (s *fakeSession) SendClientScript(int, ...game.ClientScriptArg)  {}
func (s *fakeSession) SendInterfaceFlagsRange(int, int, int, int)     {}
func (s *fakeSession) SendItemOnInterfaces(int, int, int)             {}
func (s *fakeSession) SendInterfaceScript(int)                        {}
func (s *fakeSession) SendInterfaceRemoval()                          { s.chatboxID = -1 }
func (s *fakeSession) IsChatboxInterface(groupID int) bool            { return groupID == s.chatboxID }
func (s *fakeSession) WithChatboxOwner(owner any, present func())     { present() }

func newTestPlayer(name string, x, y, z int) *game.Player {
	p := game.NewPlayer(name, game.Location{X: x, Y: y, Z: z})
	p.Session = &fakeSession{chatboxID: -1}
	return p
}

// clickEvent builds an object click on a climbable named name with
// the option in the click-type-1 slot, from a player standing at
// src. The named dispatch looks the option up by the raw
// interaction text at the click-type slot of the definition.
func clickEvent(p *game.Player, name, option string, loc game.Location, src game.Location) *plugins.ObjectInteractionEvent {
	obj := game.NewGameObject(1234, loc, 10, 0, nil)
	return &plugins.ObjectInteractionEvent{
		Player:         p,
		Object:         obj,
		Definition:     fakeObjectDef{id: 1234, name: name, interactions: []string{option, "", "", "", ""}},
		ObjectID:       1234,
		ClickType:      1,
		Option:         option,
		HasOption:      true,
		Location:       loc,
		SourceLocation: &src,
	}
}

func loadHub(t *testing.T, extra ...plugins.Plugin) *plugins.Hub {
	t.Helper()
	all := append([]plugins.Plugin{Plugin}, extra...)
	hub := plugins.NewHub(all, plugins.HubOptions{})
	hub.SetMultiChatboxPrompt(game.NewMultiChatboxPrompt(nil))
	hub.Load()
	if len(hub.Loaded()) != len(all) {
		t.Fatalf("hub loaded = %v, want %d plugins", hub.Loaded(), len(all))
	}
	return hub
}

// withMechanism installs fake as the package mechanism for the test
// and restores the previous one afterwards.
func withMechanism(t *testing.T, fake Climber) {
	t.Helper()
	prev := Mechanism()
	SetMechanism(fake)
	t.Cleanup(func() { SetMechanism(prev) })
}

// TestPluginMetadata pins the registered name and description
// against the plugin file (Ladders.plugin.js module.exports).
func TestPluginMetadata(t *testing.T) {
	if Plugin.Name != "Ladders" {
		t.Errorf("Plugin.Name = %q, want %q", Plugin.Name, "Ladders")
	}
	want := "Climbing for ladders, staircases, trapdoors, ropes and similar scenery, moving the player a level up or down with the climb animation."
	if Plugin.Description != want {
		t.Errorf("Plugin.Description = %q, want %q", Plugin.Description, want)
	}
}

// TestClimbableNames pins the 38 registered names against
// Ladders.plugin.js:21-38, in upstream order, with no duplicates.
func TestClimbableNames(t *testing.T) {
	if len(ClimbableNames) != 38 {
		t.Fatalf("len(ClimbableNames) = %d, want 38", len(ClimbableNames))
	}
	seen := make(map[string]bool, len(ClimbableNames))
	for _, name := range ClimbableNames {
		if seen[name] {
			t.Errorf("duplicate climbable name %q", name)
		}
		seen[name] = true
	}
	for _, want := range []string{"Ladder", "ladder", "Staircase", "Trapdoor", "Spiral staircase", "Ship's ladder", "Kings' ladder", "Old ruin entrance", "Exit", "Crystal Staircase", "Stairwell"} {
		if !seen[want] {
			t.Errorf("ClimbableNames missing %q", want)
		}
	}
	if seen["Gangplank"] {
		t.Error("Gangplank must not be in ClimbableNames: it registers separately with only Cross")
	}
}

// TestOptionRouting drives every option spelling through the hub
// and pins the request it reaches the mechanism with: up spellings
// climb +1, down spellings -1, top/bottom floor chain to the far
// end, all animated, destination unresolved.
func TestOptionRouting(t *testing.T) {
	up := []string{"Climb-up", "Climb up", "Climb Up", "Walk-up", "Walk-Up", "Ascend"}
	down := []string{"Climb-down", "Climb down", "Climb Down", "Walk-down", "Walk-Down", "Descend"}
	cases := []struct {
		option string
		delta  int
		toEnd  bool
	}{
		{"Top-floor", +1, true}, {"Top-Floor", +1, true},
		{"Bottom-floor", -1, true}, {"Bottom-Floor", -1, true},
	}
	for _, option := range up {
		cases = append(cases, struct {
			option string
			delta  int
			toEnd  bool
		}{option, +1, false})
	}
	for _, option := range down {
		cases = append(cases, struct {
			option string
			delta  int
			toEnd  bool
		}{option, -1, false})
	}
	for _, c := range cases {
		t.Run(c.option, func(t *testing.T) {
			fake := &fakeClimber{climbOK: true}
			withMechanism(t, fake)
			hub := loadHub(t)
			loc := game.Location{X: 3209, Y: 3216, Z: 0}
			p := newTestPlayer("alice", 3208, 3216, 0)
			if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", c.option, loc, p.Location())) {
				t.Fatalf("%q: want handled", c.option)
			}
			if len(fake.climbs) != 1 {
				t.Fatalf("%q: climbs = %d, want 1", c.option, len(fake.climbs))
			}
			req := fake.climbs[0]
			if req.Delta != c.delta || req.ToEnd != c.toEnd {
				t.Errorf("%q: request delta/toEnd = %d/%v, want %d/%v", c.option, req.Delta, req.ToEnd, c.delta, c.toEnd)
			}
			if !req.Animate {
				t.Errorf("%q: Animate = false, want true (object clicks climb animated)", c.option)
			}
			if req.Destination != nil {
				t.Errorf("%q: Destination = %+v, want nil (the map resolves it)", c.option, req.Destination)
			}
			if req.OnArrival != nil {
				t.Errorf("%q: OnArrival set, want nil for object clicks", c.option)
			}
			if req.Player != p || req.ObjectID != 1234 || req.Location != loc {
				t.Errorf("%q: request player/object/location = %v/%d/%v, want the clicked ones", c.option, req.Player, req.ObjectID, req.Location)
			}
		})
	}
}

// TestEveryNameRoutes clicks Climb-up on each of the 38 names
// through the hub: every name is registered, and a name that is
// not registered falls through.
func TestEveryNameRoutes(t *testing.T) {
	fake := &fakeClimber{climbOK: true}
	withMechanism(t, fake)
	hub := loadHub(t)
	loc := game.Location{X: 3209, Y: 3216, Z: 0}
	for _, name := range ClimbableNames {
		p := newTestPlayer("alice", 3208, 3216, 0)
		if !hub.EmitObjectInteraction(clickEvent(p, name, "Climb-up", loc, p.Location())) {
			t.Errorf("name %q Climb-up: want handled", name)
		}
	}
	if len(fake.climbs) != len(ClimbableNames) {
		t.Errorf("climbs = %d, want %d (one per name)", len(fake.climbs), len(ClimbableNames))
	}
	p := newTestPlayer("alice", 3208, 3216, 0)
	if hub.EmitObjectInteraction(clickEvent(p, "Door", "Climb-up", loc, p.Location())) {
		t.Error("unregistered name Door: want not handled")
	}
	if hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Examine", loc, p.Location())) {
		t.Error("unregistered option Examine: want not handled")
	}
}

// TestGangplankCross pins the Gangplank Cross registration: it
// goes to CrossGangplank (not Climb), unanimated, on the same plane.
func TestGangplankCross(t *testing.T) {
	fake := &fakeClimber{crossOK: true}
	withMechanism(t, fake)
	hub := loadHub(t)
	loc := game.Location{X: 3000, Y: 3000, Z: 1}
	p := newTestPlayer("alice", 3000, 2999, 1)
	if !hub.EmitObjectInteraction(clickEvent(p, "Gangplank", "Cross", loc, p.Location())) {
		t.Fatal("Gangplank Cross: want handled")
	}
	if len(fake.crosses) != 1 || len(fake.climbs) != 0 {
		t.Fatalf("crosses/climbs = %d/%d, want 1/0", len(fake.crosses), len(fake.climbs))
	}
	req := fake.crosses[0]
	if req.Delta != 0 || req.Animate {
		t.Errorf("cross request delta/animate = %d/%v, want 0/false", req.Delta, req.Animate)
	}
}

// TestNoMechanismFallsThrough pins the no-mechanism degradation:
// with no Climber installed (Worker A not yet wired), every climb
// option falls through and a click does nothing, as upstream does
// where the map has nothing at the other end.
func TestNoMechanismFallsThrough(t *testing.T) {
	if Mechanism() != nil {
		t.Skip("mechanism installed by another test; run without -run selection interference")
	}
	hub := loadHub(t)
	loc := game.Location{X: 3209, Y: 3216, Z: 0}
	p := newTestPlayer("alice", 3208, 3216, 0)
	for _, option := range []string{"Climb-up", "Climb-down", "Top-floor", "Bottom-Floor"} {
		if hub.EmitObjectInteraction(clickEvent(p, "Ladder", option, loc, p.Location())) {
			t.Errorf("%q with no mechanism: want not handled", option)
		}
	}
	if hub.EmitObjectInteraction(clickEvent(p, "Gangplank", "Cross", loc, p.Location())) {
		t.Error("Gangplank Cross with no mechanism: want not handled")
	}
}

// claimPlugin is a test plugin that listens on ladders:climb and
// optionally claims the click, recording every claim request it sees.
func claimPlugin(name string, claim bool, seen *[]*ClimbClaimEvent) plugins.Plugin {
	return plugins.Plugin{
		Name: name,
		Register: func(api *plugins.Api) {
			api.OnCustomEvent("ladders:climb", func(payload any) {
				ev, ok := payload.(*ClimbClaimEvent)
				if !ok {
					return
				}
				*seen = append(*seen, ev)
				if claim {
					ev.Handled = true
				}
			})
		},
	}
}

// TestClaimedElsewhere pins the ladders:climb claim
// (Ladders.plugin.js:201-207): a claimant that sets Handled
// short-circuits the generic handlers — the climb never runs and
// no prompt shows — while a non-claiming listener lets the climb
// proceed. The claim request carries player, object, object id
// and click type.
func TestClaimedElsewhere(t *testing.T) {
	loc := game.Location{X: 3209, Y: 3216, Z: 0}
	t.Run("claimed short-circuits the climb", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		var seen []*ClimbClaimEvent
		hub := loadHub(t, claimPlugin("Claimer", true, &seen))
		p := newTestPlayer("alice", 3208, 3216, 0)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb-up", loc, p.Location())) {
			t.Fatal("claimed Climb-up: want handled (the claim owns the click)")
		}
		if len(fake.climbs) != 0 {
			t.Errorf("climbs = %d, want 0 (claimed clicks never reach the mechanism)", len(fake.climbs))
		}
		if len(seen) != 1 {
			t.Fatalf("claim requests = %d, want 1", len(seen))
		}
		req := seen[0]
		if req.Player != p || req.ObjectID != 1234 || req.ClickType != 1 || req.Object == nil {
			t.Errorf("claim request = player %v objectID %d clickType %d object %v, want the clicked ones", req.Player, req.ObjectID, req.ClickType, req.Object)
		}
	})
	t.Run("unclaimed lets the climb proceed", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		var seen []*ClimbClaimEvent
		hub := loadHub(t, claimPlugin("Watcher", false, &seen))
		p := newTestPlayer("alice", 3208, 3216, 0)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb-down", loc, p.Location())) {
			t.Fatal("unclaimed Climb-down: want handled by the generic handler")
		}
		if len(fake.climbs) != 1 || len(seen) != 1 {
			t.Errorf("climbs/claims = %d/%d, want 1/1", len(fake.climbs), len(seen))
		}
	})
	t.Run("claimed suppresses the prompt", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		var seen []*ClimbClaimEvent
		hub := loadHub(t, claimPlugin("Claimer", true, &seen))
		p := newTestPlayer("alice", 3208, 3216, 0)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb", loc, p.Location())) {
			t.Fatal("claimed Climb: want handled")
		}
		if _, _, ok := hub.MultiChatboxPrompt().GetPending(p); ok {
			t.Error("claimed Climb: prompt shown, want none")
		}
	})
}

// TestPromptClimb drives the ambiguous "Climb" option through the
// hub and the real prompt registry: the prompt asks "Which way
// would you like to climb?" with Climb up / Climb down, and the
// pick hands off to the normal handlers only if the player has
// not moved (Ladders.plugin.js:189-199).
func TestPromptClimb(t *testing.T) {
	loc := game.Location{X: 3209, Y: 3216, Z: 0}
	start := game.Location{X: 3208, Y: 3216, Z: 0}
	pick := func(t *testing.T, hub *plugins.Hub, p *game.Player, action int) {
		t.Helper()
		if !hub.MultiChatboxPrompt().HandleInterfaceActionClick(p, game.OptionsWidgetID, action, 219) {
			t.Fatalf("pick action %d: not consumed", action)
		}
	}
	t.Run("pick up climbs up", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		p := newTestPlayer("alice", start.X, start.Y, start.Z)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb", loc, start)) {
			t.Fatal("Climb: want handled (the prompt claims the click)")
		}
		title, options, ok := hub.MultiChatboxPrompt().GetPending(p)
		if !ok {
			t.Fatal("Climb: no prompt pending")
		}
		if title != "Which way would you like to climb?" {
			t.Errorf("prompt title = %q, want the upstream question", title)
		}
		if len(options) != 2 || options[0] != "Climb up" || options[1] != "Climb down" {
			t.Errorf("prompt options = %v, want [Climb up Climb down]", options)
		}
		if len(fake.climbs) != 0 {
			t.Fatal("prompt shown: climb ran before the pick")
		}
		pick(t, hub, p, 1)
		if len(fake.climbs) != 1 {
			t.Fatalf("after picking up: climbs = %d, want 1", len(fake.climbs))
		}
		req := fake.climbs[0]
		if req.Delta != +1 || req.ToEnd {
			t.Errorf("picked-up request delta/toEnd = %d/%v, want +1/false", req.Delta, req.ToEnd)
		}
		if req.Destination != nil {
			t.Errorf("picked-up Destination = %+v, want nil", req.Destination)
		}
		if req.SourceLocation == nil || *req.SourceLocation != start {
			t.Errorf("picked-up SourceLocation = %v, want the start tile %v", req.SourceLocation, start)
		}
	})
	t.Run("pick down climbs down", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		p := newTestPlayer("alice", start.X, start.Y, start.Z)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb", loc, start)) {
			t.Fatal("Climb: want handled")
		}
		pick(t, hub, p, 2)
		if len(fake.climbs) != 1 || fake.climbs[0].Delta != -1 {
			t.Fatalf("after picking down: climbs = %+v, want one with delta -1", fake.climbs)
		}
	})
	t.Run("moved player does not climb", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		p := newTestPlayer("alice", start.X, start.Y, start.Z)
		if !hub.EmitObjectInteraction(clickEvent(p, "Ladder", "Climb", loc, start)) {
			t.Fatal("Climb: want handled")
		}
		p.SetLocation(game.Location{X: 3200, Y: 3200, Z: 0})
		pick(t, hub, p, 1)
		if len(fake.climbs) != 0 {
			t.Errorf("moved player picked up: climbs = %d, want 0", len(fake.climbs))
		}
	})
}

// TestCustomEventHandlers pins the ladders:climbUp /
// ladders:climbDown handlers (Ladders.plugin.js register):
// explicit destinations are trusted verbatim, animate defaults
// on and an explicit false suppresses it, onArrival passes
// through, and a bad payload is ignored without panicking.
func TestCustomEventHandlers(t *testing.T) {
	dest := game.Location{X: 3104, Y: 3162, Z: 0}
	boolPtr := func(b bool) *bool { return &b }
	t.Run("climbUp trusts an explicit destination", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		p := newTestPlayer("alice", 3208, 3216, 0)
		arrived := false
		hub.EmitCustomEvent("ladders:climbUp", &ClimbEvent{
			Player: p, ObjectID: 1234, Location: game.Location{X: 3209, Y: 3216, Z: 0},
			Destination: &dest, Animate: boolPtr(false), OnArrival: func() { arrived = true },
		})
		if len(fake.climbs) != 1 {
			t.Fatalf("climbs = %d, want 1", len(fake.climbs))
		}
		req := fake.climbs[0]
		if req.Delta != +1 {
			t.Errorf("delta = %d, want +1", req.Delta)
		}
		if req.Destination == nil || *req.Destination != dest {
			t.Errorf("Destination = %v, want %v verbatim", req.Destination, dest)
		}
		if req.Animate {
			t.Error("Animate = true, want false (explicit animate:false)")
		}
		if req.OnArrival == nil {
			t.Fatal("OnArrival = nil, want the callback passed through")
		}
		req.OnArrival()
		if !arrived {
			t.Error("OnArrival callback did not run")
		}
	})
	t.Run("climbDown animates by default", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		p := newTestPlayer("alice", 3208, 3216, 0)
		hub.EmitCustomEvent("ladders:climbDown", &ClimbEvent{Player: p, ObjectID: 1234})
		if len(fake.climbs) != 1 {
			t.Fatalf("climbs = %d, want 1", len(fake.climbs))
		}
		req := fake.climbs[0]
		if req.Delta != -1 || !req.Animate {
			t.Errorf("delta/animate = %d/%v, want -1/true", req.Delta, req.Animate)
		}
		if req.Destination != nil || req.ToEnd {
			t.Errorf("Destination/ToEnd = %v/%v, want nil/false", req.Destination, req.ToEnd)
		}
	})
	t.Run("bad payloads are ignored", func(t *testing.T) {
		fake := &fakeClimber{climbOK: true}
		withMechanism(t, fake)
		hub := loadHub(t)
		hub.EmitCustomEvent("ladders:climbUp", "not an event")
		hub.EmitCustomEvent("ladders:climbUp", nil)
		hub.EmitCustomEvent("ladders:climbDown", &ClimbEvent{})
		if len(fake.climbs) != 0 {
			t.Errorf("climbs = %d, want 0 for bad payloads", len(fake.climbs))
		}
	})
}

// TestEngineNilGuards pins the nil guards: a nil event, a nil
// player, and a nil prompt/emit surface never panic and never
// climb.
func TestEngineNilGuards(t *testing.T) {
	engine := &ladderEngine{climber: &fakeClimber{climbOK: true}}
	for name, fn := range map[string]func(*plugins.ObjectInteractionEvent) bool{
		"climbUp": engine.climbUpOption, "climbDown": engine.climbDownOption,
		"topFloor": engine.topFloorOption, "bottomFloor": engine.bottomFloorOption,
		"climb": engine.climbOption, "cross": engine.crossOption,
	} {
		if fn(nil) {
			t.Errorf("%s(nil): want false", name)
		}
		if fn(&plugins.ObjectInteractionEvent{}) {
			t.Errorf("%s(no player): want false", name)
		}
	}
	engine.onClimbUpEvent(nil)
	engine.onClimbDownEvent("not an event")
}
