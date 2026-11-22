// Package ladders is the Go port of the generic climb system in
// server/plugins/objects/Ladders.plugin.js (upstream HEAD bfd81e3):
// ladders, staircases, trapdoors, ropes and gangplanks, moving the
// player a level up or down with the climb animation.
//
// The plugin half lives here: the 38 climbable names, every option
// spelling, the ambiguous-"Climb" prompt, the ladders:climb claim,
// and the ladders:climbUp / ladders:climbDown custom-event handlers.
// The mechanism half — ClimbLinks destination resolution (declared
// links, other-end search, underground offset, landing tiles,
// toEnd chaining) and the climb task (movement block, 1-cycle
// animation, plane move, onArrival) — is Worker A's, behind the
// Climber seam below. Until a Climber is installed (SetMechanism),
// every climb falls through and a click does nothing, matching
// upstream where the map has nothing at the other end.
package ladders

import (
	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// ClimbableNames is the 38 climbable names, exact as the cache
// spells them (Ladders.plugin.js:21-38).
var ClimbableNames = []string{
	"Ladder", "ladder", "Staircase", "Stairs", "Steps", "Trapdoor", "Manhole", "Spiral staircase",
	"Vine ladder", "Ship's ladder", "Bamboo Ladder", "Metal ladder", "Iron ladder", "Rope ladder", "Stone ladder",
	"Stone Ladder", "Copper Ladder", "Tower ladder", "Boney ladder", "Kings' ladder", "Troll ladder", "Ladder top",
	"Rope", "Climbing rope", "Escape rope", "Anchor rope", "Rope anchor",
	"Old ruin entrance", "Exit",
	"Stairs up", "Stairs down", "Wooden Stair", "Stone Staircase", "Crystal Staircase", "Crypt staircase",
	"Broken stairs", "Cellar stairs", "Stairwell",
}

// upOptions / downOptions are the option spellings for each
// direction, exactly as the cache spells them
// (Ladders.plugin.js:40-41).
var upOptions = []string{"Climb-up", "Climb up", "Climb Up", "Walk-up", "Walk-Up", "Ascend"}
var downOptions = []string{"Climb-down", "Climb down", "Climb Down", "Walk-down", "Walk-Down", "Descend"}

// topFloorOptions / bottomFloorOptions are the straight-to-the-far-end
// spellings (Ladders.plugin.js:45-50): Lumbridge's staircases say
// "Top-floor", the Grand Tree's ladder column says "Top-Floor".
var topFloorOptions = []string{"Top-floor", "Top-Floor"}
var bottomFloorOptions = []string{"Bottom-floor", "Bottom-Floor"}

// ClimbRequest is one climb for the mechanism seam: the player, the
// clicked object, and how the climb was asked for. Destination, when
// non-nil, is trusted verbatim (an explicit destination from the
// ladders:climbUp / ladders:climbDown custom events); nil means the
// mechanism resolves it from the map. Animate false suppresses the
// climb animation (walked stairs, gangplank crossings); the plugin
// always sets it explicitly, never relying on the zero value.
// OnArrival, when given, runs right after the move; a refused or
// cancelled climb never calls it.
type ClimbRequest struct {
	Player         *game.Player
	Object         plugins.GameObject
	ObjectID       int
	Definition     plugins.ObjectDefinition
	Location       game.Location
	SourceLocation *game.Location
	ClickType      int
	// Delta is the plane change: +1 up, -1 down (0 for a gangplank
	// crossing, which stays on the same plane).
	Delta int
	// Destination is the trusted explicit destination, or nil to
	// resolve from the map.
	Destination *game.Location
	Animate     bool
	// ToEnd chains to the far end of the shaft (Top-floor /
	// Bottom-floor).
	ToEnd     bool
	OnArrival func()
}

// Climber is the climb mechanism seam (Worker A): destination
// resolution plus the climb task. Climb moves the player one plane
// in req.Delta (resolving the destination from the map unless
// req.Destination is set, trusted verbatim); CrossGangplank moves
// the player across a gangplank without a climb animation. Both
// report false when the climb does not happen.
type Climber interface {
	Climb(req ClimbRequest) bool
	CrossGangplank(req ClimbRequest) bool
}

// installedMechanism is the Climber the wiring installs at boot
// (the doors SetCatalog precedent). A nil mechanism means no
// climb happens: every handler falls through, as upstream does
// where the map has nothing at the other end.
var installedMechanism Climber

// SetMechanism installs the climb mechanism. Called once at boot,
// before hub Load.
func SetMechanism(c Climber) { installedMechanism = c }

// Mechanism returns the installed climb mechanism, or nil.
func Mechanism() Climber { return installedMechanism }

// ClimbClaimEvent is the payload of the "ladders:climb" custom event
// (Ladders.plugin.js:201-207 claimedElsewhere): emitted before the
// generic handlers run. Content that owns a ladder (e.g. Castle
// Wars) sets Handled to claim the click, and the generic handlers
// never run. Handlers receive a *ClimbClaimEvent.
type ClimbClaimEvent struct {
	Player    *game.Player
	Object    plugins.GameObject
	ObjectID  int
	ClickType int
	Handled   bool
}

// ClimbEvent is the payload of the "ladders:climbUp" and
// "ladders:climbDown" custom events (Ladders.plugin.js:162-185):
// area/quest content asks for a climb with an explicit destination.
// Destination, when non-nil, is trusted verbatim; Animate nil
// means animated (the upstream default — only an explicit false
// suppresses the animation). Handlers receive a *ClimbEvent.
type ClimbEvent struct {
	Player         *game.Player
	Object         plugins.GameObject
	ObjectID       int
	Definition     plugins.ObjectDefinition
	Location       game.Location
	SourceLocation *game.Location
	ClickType      int
	Destination    *game.Location
	Animate        *bool
	OnArrival      func()
}

// ladderEngine is the plugin's state: the prompt + custom-event
// surfaces it drives (from the Api) and the climb mechanism it
// calls. The mechanism is resolved lazily per use through
// mechanism(): an engine built before the mechanism is installed
// still picks it up once it lands (the Phase 32 hub contract —
// surfaces installed after Load are never captured in Register).
type ladderEngine struct {
	prompt  func(player *game.Player, title string, pairs ...any) bool
	emit    func(name string, payload any)
	climber Climber
}

// engineFromAPI builds an engine from a plugin Api: the prompt and
// custom-event emit. The mechanism is NOT captured here — it is
// installed on the package after Load (SetMechanism), so it is
// resolved lazily per use.
func engineFromAPI(api *plugins.Api) *ladderEngine {
	e := &ladderEngine{}
	if api != nil {
		e.prompt = api.SendMultiChatboxPrompt
		e.emit = api.EmitCustomEvent
	}
	return e
}

// mechanism returns the climb mechanism: an explicitly set climber
// wins, else the package-installed one. A nil result is not cached
// — the next call re-asks.
func (e *ladderEngine) mechanism() Climber {
	if e.climber != nil {
		return e.climber
	}
	return installedMechanism
}

// climb runs one climb through the mechanism. With no mechanism
// installed the climb does not happen and the click falls through.
func (e *ladderEngine) climb(req ClimbRequest) bool {
	if req.Player == nil {
		return false
	}
	m := e.mechanism()
	if m == nil {
		return false
	}
	return m.Climb(req)
}

// requestFromEvent builds the climb request for an object click:
// animated, destination unresolved (the map decides), no arrival
// callback.
func (e *ladderEngine) requestFromEvent(ev *plugins.ObjectInteractionEvent, delta int, toEnd bool) ClimbRequest {
	req := ClimbRequest{Delta: delta, ToEnd: toEnd, Animate: true}
	if ev == nil {
		return req
	}
	req.Player = ev.Player
	req.Object = ev.Object
	req.ObjectID = ev.ObjectID
	if req.ObjectID == 0 && ev.Object != nil {
		req.ObjectID = ev.Object.ID()
	}
	req.Definition = ev.Definition
	req.Location = ev.Location
	req.SourceLocation = ev.SourceLocation
	req.ClickType = ev.ClickType
	return req
}

// claimedElsewhere is claimedElsewhere (Ladders.plugin.js:201-207):
// content that owns a ladder claims the click first via the
// ladders:climb custom event; if handled, the generic handlers
// never run.
func (e *ladderEngine) claimedElsewhere(ev *plugins.ObjectInteractionEvent) bool {
	if ev == nil || e.emit == nil {
		return false
	}
	objectID := ev.ObjectID
	if objectID == 0 && ev.Object != nil {
		objectID = ev.Object.ID()
	}
	req := &ClimbClaimEvent{
		Player:    ev.Player,
		Object:    ev.Object,
		ObjectID:  objectID,
		ClickType: ev.ClickType,
	}
	e.emit("ladders:climb", req)
	return req.Handled
}

// climbUpOption is climbUpOption (Ladders.plugin.js:221-223).
func (e *ladderEngine) climbUpOption(ev *plugins.ObjectInteractionEvent) bool {
	return e.claimedElsewhere(ev) || e.climb(e.requestFromEvent(ev, +1, false))
}

// climbDownOption is climbDownOption (Ladders.plugin.js:225-227).
func (e *ladderEngine) climbDownOption(ev *plugins.ObjectInteractionEvent) bool {
	return e.claimedElsewhere(ev) || e.climb(e.requestFromEvent(ev, -1, false))
}

// topFloorOption is topFloorOption (Ladders.plugin.js:239-241):
// straight to the last floor.
func (e *ladderEngine) topFloorOption(ev *plugins.ObjectInteractionEvent) bool {
	return e.claimedElsewhere(ev) || e.climb(e.requestFromEvent(ev, +1, true))
}

// bottomFloorOption is bottomFloorOption (Ladders.plugin.js:243-245).
func (e *ladderEngine) bottomFloorOption(ev *plugins.ObjectInteractionEvent) bool {
	return e.claimedElsewhere(ev) || e.climb(e.requestFromEvent(ev, -1, true))
}

// climbOption is climbOption (Ladders.plugin.js:215-217): the
// ambiguous "Climb" option asks which way instead of guessing.
func (e *ladderEngine) climbOption(ev *plugins.ObjectInteractionEvent) bool {
	return e.claimedElsewhere(ev) || e.promptClimb(ev)
}

// crossOption is crossOption (Ladders.plugin.js:232-236): Gangplank
// "Cross" — dock to ship deck, no climb animation.
func (e *ladderEngine) crossOption(ev *plugins.ObjectInteractionEvent) bool {
	if e.claimedElsewhere(ev) {
		return true
	}
	m := e.mechanism()
	if m == nil {
		return false
	}
	req := e.requestFromEvent(ev, 0, false)
	req.Animate = false
	return m.CrossGangplank(req)
}

// promptClimb is promptClimb (Ladders.plugin.js:189-199): asks
// "Which way would you like to climb?" via sendMultiChatboxPrompt,
// then hands off to the normal up/down handlers — only if the
// player has not moved. The hand-off clears any explicit
// destination and carries the start tile as the source location,
// exactly as upstream spreads {...event, destination: undefined,
// sourceLocation}.
func (e *ladderEngine) promptClimb(ev *plugins.ObjectInteractionEvent) bool {
	if ev == nil || ev.Player == nil || e.prompt == nil {
		return false
	}
	start := ev.Player.Location()
	source := start
	handOff := func(delta int) plugins.PromptCallback {
		return func(player *game.Player, _ int, _ string) {
			if player == nil || player.Location() != start {
				return
			}
			req := e.requestFromEvent(ev, delta, false)
			req.Destination = nil
			req.SourceLocation = &source
			e.climb(req)
		}
	}
	return e.prompt(ev.Player, "Which way would you like to climb?",
		"Climb up", handOff(+1),
		"Climb down", handOff(-1))
}

// onClimbUpEvent / onClimbDownEvent are the ladders:climbUp /
// ladders:climbDown custom-event handlers (Ladders.plugin.js
// register: climbUp / climbDown): explicit destinations are trusted,
// animate defaults on, onArrival runs after the move. They do NOT
// go through claimedElsewhere — an explicit climb is already owned
// by its emitter.
func (e *ladderEngine) onClimbUpEvent(payload any)   { e.onClimbEvent(payload, +1) }
func (e *ladderEngine) onClimbDownEvent(payload any) { e.onClimbEvent(payload, -1) }

func (e *ladderEngine) onClimbEvent(payload any, delta int) {
	ev, ok := payload.(*ClimbEvent)
	if !ok || ev == nil {
		return
	}
	objectID := ev.ObjectID
	if objectID == 0 && ev.Object != nil {
		objectID = ev.Object.ID()
	}
	animate := true
	if ev.Animate != nil {
		animate = *ev.Animate
	}
	e.climb(ClimbRequest{
		Player:         ev.Player,
		Object:         ev.Object,
		ObjectID:       objectID,
		Definition:     ev.Definition,
		Location:       ev.Location,
		SourceLocation: ev.SourceLocation,
		ClickType:      ev.ClickType,
		Delta:          delta,
		Destination:    ev.Destination,
		Animate:        animate,
		OnArrival:      ev.OnArrival,
	})
}

// climbOptions builds the shared option map for one climbable name
// (Ladders.plugin.js register: options): the ambiguous "Climb",
// both top/bottom-floor spellings, and every up/down spelling.
func climbOptions(engine *ladderEngine) map[string]plugins.ObjectInteractionHandler {
	options := map[string]plugins.ObjectInteractionHandler{
		"Climb": engine.climbOption,
	}
	for _, option := range topFloorOptions {
		options[option] = engine.topFloorOption
	}
	for _, option := range bottomFloorOptions {
		options[option] = engine.bottomFloorOption
	}
	for _, option := range upOptions {
		options[option] = engine.climbUpOption
	}
	for _, option := range downOptions {
		options[option] = engine.climbDownOption
	}
	return options
}

// Plugin is the registered Ladders plugin value: name and
// description from the plugin file (Ladders.plugin.js
// module.exports).
var Plugin = plugins.Plugin{
	Name:        "Ladders",
	Description: "Climbing for ladders, staircases, trapdoors, ropes and similar scenery, moving the player a level up or down with the climb animation.",
	Register: func(api *plugins.Api) {
		engine := engineFromAPI(api)
		options := climbOptions(engine)
		for _, name := range ClimbableNames {
			api.OnObjectInteractionNamed(name, options)
		}
		api.OnObjectInteractionNamed("Gangplank", map[string]plugins.ObjectInteractionHandler{
			"Cross": engine.crossOption,
		})
		api.OnCustomEvent("ladders:climbUp", engine.onClimbUpEvent)
		api.OnCustomEvent("ladders:climbDown", engine.onClimbDownEvent)
	},
}

func init() {
	plugins.Register(Plugin)
}
