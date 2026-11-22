// ObjectActionPacketListener: object_option (opcodes 96, 28, 42, 38, 51)
// + LOC_INTERACT (231), porting
// net/packet/impl/ObjectActionPacketListener.ts (executeAction +
// executeInteraction, lines 29-172) and the dispatch preamble in
// NetworkBuilder.ts.
//
// The WORLD_INTERACTIONS preamble (NetworkBuilder.ts:680-693)
// runs in Handle before routing (worldInputPreamble): the
// animation-stall hold is a gap (no stalled-input queue), the
// TRADING skip and the player:world-input emit run, and
// closeInterruptibleInterfaces runs unless the packet is
// skipped. The INTERACTION_CLICKS preamble
// (NetworkBuilder.ts:713-719) IS ported: the click-run override
// is set before routing, even when routing then refuses the
// packet, matching the ground-item precedent in
// routeGroundItemAction.
//
// Gates (executeAction): the player must exist, have hitpoints
// above 0, and not be busy(). busy() (Player.ts:831-848) reads
// interface id, hitpoints, needsPlacement/teleporting, status,
// and forceMovement; only the hitpoints term exists in Go, so
// busy() degenerates to the hitpoints term (NeedsPlacement is a
// sync marker and is deliberately not consulted — see the
// movement-queue audit). The location is built from the packet
// x/y with the PLAYER's plane: the packet carries no plane.
//
// The boat-deck branches (DECK_REACH_TILES / OTHER_DECK_REACH_TILES
// = 12, OTHER_DECK_WAIT_TICKS = 10, OTHER_DECK_SEARCH_TILES = 32,
// pendingOtherDeck, onOtherDeck, ownOnly) are GAPS: sailing is
// migration milestone 11 and no boats or private areas exist in
// Go, so the remote/deck branches can never trigger. Only the
// main-world path is ported; setDeckReach/setUsableFromDeck stay
// table-only in the hub.
//
// The bank branch: when no plugin handles the interaction and
// the option is "bank", TS opens the bank (Bank.open sets status
// BANKING, the interface id, sub-interfaces, varps/varbits, and
// emits bank:opened). In Go the Bank content (phase 46) claims
// the click first, through the onObjectInteraction emit, so this
// branch only fires when no handler claims it: then it stays
// silent (no open, no operated emit, no message), a deviation
// from TS's own open, reachable only with the Bank content
// unloaded.
//
// Facing is a GAP: TS calls setPositionToFace in the arrival
// closure. The player-facing seam exists since Phase 33
// (Player.SetPositionToFace / ForcePositionToFace), but this
// arrival closure does not call it yet (pickup gaps facing the
// same way), so no facing is set.
//
// The DEVELOPER notice is a GAP: MapObjects.getPrivateArea sends
// "@red@Object with id X does not exist." to DEVELOPER-rights
// players when the lookup misses. Go has no live rights field
// (rights are read from the carried save), so a lookup miss stays
// silent.
//
// The no-handler fallback is SILENCE: executeInteraction emits
// the interaction, and when no plugin handles it and the option
// is not "bank", TS does nothing. The OPERATED_EVENT doc comment
// states: "A click no plugin took and no engine option covers
// stays silent, so a mere attempt is no deed." No default message
// is invented.
//
// Seams: the object lookup (ObjectLookup) and the definition
// source (ObjectDefinitionSource) are Deps fields, following the
// examine.go pattern — the coordinator implements them over the
// game registry and the definition Store, and listeners never
// import the definition package. Hub emits arrive as func fields
// on ObjectHooks, following the Phase 12 precedent
// (ChatHooks.SocialIntercept, ObjectExamineIntercept): listeners
// never import the plugins package. When the lookup or the
// definition source is unwired, the packet drops with
// ReasonObjectAction.

package listeners

import (
	"strings"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// ObjectEntry is the listener-facing view of a loc: the pinned
// seam Worker 1 implements (internal/game.GameObject satisfies
// this interface structurally). Only the identity the interaction
// path needs is exposed.
type ObjectEntry interface {
	ID() int
	Location() game.Location
	Type() int
	Face() int
}

// ObjectLookup resolves a loc for a player: the Go form of
// MapObjects.getPrivateArea (MapObjects.ts), including its
// forPlayer variant rule and bridge-tile fallback, as implemented
// by the game registry. A miss returns nil.
type ObjectLookup interface {
	Find(player *game.Player, id int, loc game.Location) ObjectEntry
}

// ObjectDefinition is the listener-facing view of a loc
// definition: the resolved (forPlayer) definition's identity,
// footprint, blocking mask, name, and menu. The definition
// package's ObjectDefinition satisfies this interface
// structurally; listeners never import that package.
type ObjectDefinition interface {
	ID() int
	Name() string
	SizeX() int
	SizeY() int
	BlockingMask() int
	Interactions() []string
}

// ObjectDefinitionSource resolves loc definitions: ObjectForPlayer
// is ObjectDefinition.forPlayer (the multi-loc variant rule;
// unresolvable variants report absent), ObjectByID is
// ObjectDefinition.forId (the base definition, for the type 10/11
// footprint rule in buildObjectRouteSpec). The coordinator
// implements it over the definition Store; the nil-var degradation
// of Store.ObjectForPlayer is inherited (see examine.go).
type ObjectDefinitionSource interface {
	ObjectForPlayer(id int, player *game.Player) (ObjectDefinition, bool)
	ObjectByID(id int) (ObjectDefinition, bool)
}

// ObjectRouteRequest is the listener-facing form of the
// PluginObjectRouteEvent (PluginTypes.ts:149-157): the walk-to
// path's view of the object being approached. Route hooks may set
// Destination (redirecting the walk to a tile) and/or
// BlockAccessFlags. Source is the player's tile at click time.
type ObjectRouteRequest struct {
	Player           *game.Player
	Object           ObjectEntry
	Definition       ObjectDefinition
	ObjectID         int
	ClickType        int
	Source           game.Location
	Destination      *game.Location
	BlockAccessFlags int
}

// ObjectInteractionEvent is the listener-facing form of the
// PluginObjectInteractionEvent (PluginTypes.ts:137-147). Option is
// the lower-cased menu text for the click type, when the slot
// holds text (HasOption); Location is the OBJECT's tile and
// SourceLocation is the player's tile at ARRIVAL (TS captures the
// interaction location once, at arrival, and passes the object's
// tile as location). Handled is the claim flag: a hook (or the
// emit adapter) sets it to claim the interaction.
type ObjectInteractionEvent struct {
	Player         *game.Player
	Object         ObjectEntry
	Definition     ObjectDefinition
	ObjectID       int
	ClickType      int
	Option         string
	HasOption      bool
	Location       game.Location
	SourceLocation game.Location
	Handled        bool
}

// ItemOnObjectEvent is the listener-facing form of the
// PluginItemOnObjectEvent (PluginTypes.ts:710-720). Item is the
// inventory item captured at CLICK time; TS passes the captured
// reference through and does not re-verify the inventory on
// arrival. Location is the OBJECT's tile. Handled is
// the claim flag.
type ItemOnObjectEvent struct {
	Player        *game.Player
	Object        ObjectEntry
	ObjectID      int
	Item          game.Item
	ItemID        int
	ItemSlot      int
	InterfaceType int
	Location      game.Location
	Handled       bool
}

// ObjectOperatedEvent is the listener-facing form of the
// object:operated payload (PluginTypes.ts OPERATED_EVENT): the
// interaction a plugin claimed (or the bank branch opened —
// which never fires in Go, see the file header). Location is the
// OBJECT's tile.
type ObjectOperatedEvent struct {
	Player    *game.Player
	Object    ObjectEntry
	ObjectID  int
	Option    string
	HasOption bool
	Location  game.Location
}

// ObjectHooks carries the object-interaction hub emits as func
// fields, following the Phase 12 precedent. Every field is
// optional: nil means no hooks, which degenerates to the
// no-hooks defaults (no route redirection, no interaction
// claimed, no operated delivery). The coordinator wires these
// to the hub in cmd/server/objectlistenerwiring.go.
type ObjectHooks struct {
	// EmitObjectRoute fires the onObjectRoute hooks. The
	// request's Destination and BlockAccessFlags are read
	// back after the call.
	EmitObjectRoute func(*ObjectRouteRequest)
	// EmitObjectInteraction fires the onObjectInteraction
	// hooks (before hooks, then the merged generic/id/name
	// hooks) and reports whether the interaction was
	// handled. The event's Handled flag carries the same
	// claim; either one claiming is enough.
	EmitObjectInteraction func(*ObjectInteractionEvent) bool
	// EmitItemOnObject fires the onItemOnObject hooks (all
	// main hooks, then all fallback hooks) and reports
	// whether the use was handled.
	EmitItemOnObject func(*ItemOnObjectEvent) bool
	// EmitItemOnItem fires the onItemOnItem hooks (all hooks,
	// in registration order) and reports whether the use was
	// handled. Unhandled uses are silent — itemOnItem has no
	// fallback message. The field lives here, beside
	// EmitItemOnObject, because both are UseItem routes
	// (the item-on-item route is in useitem.go).
	EmitItemOnItem func(*ItemOnItemEvent) bool
	// EmitOperated delivers the object:operated event for a
	// claimed interaction.
	EmitOperated func(*ObjectOperatedEvent)
}

// objectStillStands mirrors MapObjects.stillStands
// (MapObjects.ts): the object still stands when the re-lookup by
// (id, loc) returns the same instance, or a loc with the same id,
// type, and face at the same tile. The (id, loc) pair is the
// CALLER's: executeInteraction and useItem re-look up by the
// packet id and the click tile (ObjectActionPacketListener.ts:67,
// UseItemPacketListener.ts:62), while the walk-to-object validity
// check re-looks up by the object's own id and tile
// (MovementQueue.ts:1686, 1823). A nil lookup never stands.
// Entries are compared by interface equality (pointer identity in
// practice); a nil re-lookup means the object is gone.
func objectStillStands(lookup ObjectLookup, player *game.Player, id int, loc game.Location, object ObjectEntry) bool {
	if lookup == nil || object == nil {
		return false
	}
	current := lookup.Find(player, id, loc)
	if current == nil {
		return false
	}
	return current == object ||
		(current.ID() == object.ID() && current.Type() == object.Type() && current.Face() == object.Face())
}

// routeObjectAction routes one decoded object_option /
// LOC_INTERACT message. The click-run override is set before
// routing, even when routing then refuses the packet (the
// INTERACTION_CLICKS preamble + the ground-item precedent). An
// unwired lookup or definition source drops with
// ReasonObjectAction; a refused packet (gates, lookup miss,
// unresolvable definition or click type) returns noAction, the
// silent return TS gives.
func routeObjectAction(p *game.Player, msg protocol.ObjectOption, cycle int64, deps Deps, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if q := p.MovementQueue(); q != nil {
		q.SetClickRunOverride(game.ClickRunOverride(p.IsRunning(), msg.ModifierFlags))
	}
	if deps.Objects == nil || deps.ObjectDefs == nil {
		return drop(ReasonObjectAction)
	}
	if objectAction(p, msg, cycle, deps) {
		return acted()
	}
	return noAction()
}

// objectAction ports executeAction
// (ObjectActionPacketListener.ts:29-172): gates, lookup,
// definition + click-type normalization, and the walk-to-object
// with the executeInteraction arrival closure. It reports
// whether the walk-to started. A refused packet returns false
// silently; WalkToObject's own silent refusals (mobility,
// destination) still count as started — the route event fired
// and the override was set.
func objectAction(p *game.Player, msg protocol.ObjectOption, cycle int64, deps Deps) bool {
	if !playerReady(p) {
		return false
	}
	// Gate (line 38): hitpoints <= 0 or busy(). busy()
	// degenerates to the hitpoints term in Go (see the file
	// header).
	if hitpoints(p) <= 0 {
		return false
	}

	// Location (lines 41-45): the packet x/y with the player's
	// plane.
	loc := game.Location{X: msg.X, Y: msg.Y, Z: p.Location().Z}

	// Lookup (line 52): MapObjects.getPrivateArea. A miss
	// returns silently (the DEVELOPER notice is a gap).
	obj := deps.Objects.Find(p, msg.ID, loc)
	if obj == nil {
		return false
	}

	// Definition (line 55): ObjectDefinition.forPlayer on the
	// OBJECT's id (not the packet id: the lookup's forPlayer
	// variant rule may have matched a base id to the requested
	// variant). An unresolvable variant returns silently.
	def, ok := deps.ObjectDefs.ObjectForPlayer(obj.ID(), p)
	if !ok {
		return false
	}

	// Click-type normalization (lines 56-61): when the click
	// type is not an integer in 1..5, the action text —
	// trimmed and lower-cased — is matched against the
	// definition's interactions, and the click type becomes
	// the matching index + 1 (no match: 0). A valid click
	// type ignores the action text entirely. Still outside
	// 1..5: return silently.
	clickType := 0
	if msg.HasClickType {
		clickType = msg.ClickType
	}
	if clickType < 1 || clickType > 5 {
		normalized := ""
		if msg.HasAction {
			normalized = strings.ToLower(strings.TrimSpace(msg.Action))
		}
		if normalized != "" {
			if idx := findActionIndex(def.Interactions(), normalized); idx >= 0 {
				clickType = idx + 1
			}
		}
		if clickType < 1 || clickType > 5 {
			return false
		}
	}

	// Option (line 62): the lower-cased menu text at
	// clickType-1, which may be absent.
	interactions := def.Interactions()
	option := ""
	hasOption := false
	if clickType-1 < len(interactions) && interactions[clickType-1] != "" {
		option = strings.ToLower(interactions[clickType-1])
		hasOption = true
	}

	// Base definition for the type 10/11 footprint rule
	// (MovementQueue.ts:1357-1358): forId on the object's id,
	// falling back to the resolved definition when unavailable.
	base := def
	if b, ok := deps.ObjectDefs.ObjectByID(obj.ID()); ok {
		base = b
	}

	// The walk-to-object validity check re-looks the object up
	// by its own id and tile (MovementQueue.ts:1686, 1823).
	stillStands := func() bool {
		return objectStillStands(deps.Objects, p, obj.ID(), obj.Location(), obj)
	}

	// Arrival closure: executeInteraction (lines 63-100).
	arrival := func() {
		// Still-stands re-check (non-quiet), by the packet
		// id and the click tile (line 67): return if gone.
		if !objectStillStands(deps.Objects, p, msg.ID, loc, obj) {
			return
		}
		// Interaction location: the player's tile at
		// ARRIVAL. Then reset + walkToReset, and the
		// facing call (a gap — no facing system).
		arrivalLoc := p.Location()
		q := p.MovementQueue()
		q.Reset(true)
		q.CancelWalkTo()

		ev := &ObjectInteractionEvent{
			Player:         p,
			Object:         obj,
			Definition:     def,
			ObjectID:       obj.ID(),
			ClickType:      clickType,
			Option:         option,
			HasOption:      hasOption,
			Location:       obj.Location(),
			SourceLocation: arrivalLoc,
		}
		handled := false
		if deps.ObjectHooks.EmitObjectInteraction != nil {
			handled = deps.ObjectHooks.EmitObjectInteraction(ev)
		}
		if handled || ev.Handled {
			// Claimed: emit operated and return.
			if deps.ObjectHooks.EmitOperated != nil {
				deps.ObjectHooks.EmitOperated(&ObjectOperatedEvent{
					Player:    p,
					Object:    obj,
					ObjectID:  obj.ID(),
					Option:    option,
					HasOption: hasOption,
					Location:  obj.Location(),
				})
			}
			return
		}
		if option == "bank" {
			// Bank branch (lines 97-101): the Bank content
			// (phase 46) normally claims the click through
			// the interaction emit, so this fallback only
			// fires with no handler: silent (no open, no
			// operated emit, no message — a deviation from
			// TS's own open, reachable only with the Bank
			// content unloaded).
			return
		}
		// No-handler fallback: SILENCE. TS does nothing
		// here; no default message is invented.
	}

	target := game.ObjectTarget{
		ID:           obj.ID(),
		Location:     obj.Location(),
		Type:         obj.Type(),
		Face:         obj.Face(),
		SizeX:        def.SizeX(),
		SizeY:        def.SizeY(),
		BaseSizeX:    base.SizeX(),
		BaseSizeY:    base.SizeY(),
		BlockingMask: def.BlockingMask(),
		Name:         def.Name(),
		Interactions: interactions,
	}
	p.MovementQueue().WalkToObject(target, arrival, clickType, cycle, game.WalkToObjectHooks{
		Route:       translateRouteHook(deps, p, obj, def, clickType),
		StillStands: stillStands,
	})
	return true
}

// translateRouteHook adapts the game walk-to-object route event
// to the listener-facing ObjectRouteRequest and back: the
// request carries the resolved definition and the object entry,
// and the Destination / BlockAccessFlags the hooks set are
// written back onto the game event. A nil EmitObjectRoute means
// no hooks: the returned hook is a no-op.
func translateRouteHook(deps Deps, p *game.Player, obj ObjectEntry, def ObjectDefinition, clickType int) func(*game.ObjectRouteEvent) {
	return func(ev *game.ObjectRouteEvent) {
		if deps.ObjectHooks.EmitObjectRoute == nil {
			return
		}
		req := &ObjectRouteRequest{
			Player:     p,
			Object:     obj,
			Definition: def,
			ObjectID:   obj.ID(),
			ClickType:  clickType,
			Source:     ev.Source,
		}
		deps.ObjectHooks.EmitObjectRoute(req)
		ev.Destination = req.Destination
		ev.BlockAccessFlags = req.BlockAccessFlags
	}
}
