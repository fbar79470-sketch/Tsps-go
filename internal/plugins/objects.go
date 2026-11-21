// Object interaction hooks: the Go form of the object families of
// PluginApi (PluginTypes.ts) that the engine can fire since phase 13:
// onObjectRoute, onObjectInteraction (plain and name/actions forms),
// onBeforeObjectInteraction, onObjectClick and onObjectFirstClick
// through onObjectFifthClick, and onItemOnObject (plain and named
// forms), plus the cross-plugin emits emitObjectInteraction and
// emitItemOnObject, the object:operated custom event, and the
// getObjectManager / core.ObjectManager surface.
//
// Every emit below ports its PluginManager.emit* body, including
// the entry guards, the fill-ins, the hook ordering, and the
// short-circuit/run-all shape — the families differ from each
// other, and those differences are behavior content relies on:
//
//   - emitObjectInteraction (PluginManager.ts:1170-1212): entry
//     guards (nil event/player/object, pre-handled), the no-hooks
//     early return, definition + option fill-ins, the before-hooks
//     loop (a claim returns true without running the main hooks),
//     then the merged generic + id + name hooks in original
//     registration order, stopping once handled. The loops use
//     runHandledGate (gates.go), whose contract names this family.
//   - emitObjectRoute (PluginManager.ts:1214-1224): run-all until
//     a hook sets a destination, then stop. No handled flag.
//   - emitItemOnObject (PluginManager.ts:1915-1923): run ALL main
//     hooks, then ALL fallback hooks; each hook's registration
//     wrapper skips once the event is handled, so a fallback
//     handler effectively runs only when none handled the use.
//     runHandledGate must NOT be substituted (gates.go header).
//
// GameObject and ObjectDefinition are interfaces declared here so
// event payloads can carry objects and definitions without this
// package depending on the concrete registry or definition types:
// the concrete *game.GameObject (phase 13, Worker 1) satisfies
// GameObject structurally through the pinned ID/Location/Type/Face
// methods, and *definition.ObjectDefinition satisfies
// ObjectDefinition through its existing accessors. Content that
// needs more of an object than these four methods reads it from
// the event's Definition or from ObjectManagerAPI.
//
// onSpellOnObject stays table-only (magic NOT STARTED), and
// setDeckReach / setUsableFromDeck stay table-only (sailing,
// milestone 11): see the unfired-family table in hooks.go.

package plugins

import (
	"sort"
	"strconv"

	"tsps-go/internal/game"
)

// GameObject is the minimal object surface content handlers need:
// the pinned GameObject accessors (GameObject.ts:37-79). The
// concrete registry type satisfies it structurally; the hub never
// constructs one.
type GameObject interface {
	// ID is the object's cache id (getId).
	ID() int
	// Location is the object's tile (getLocation, via Entity).
	Location() game.Location
	// Type is the object's shape (getType): 10/11/22 are the
	// solid/footprint shapes, 0-9 the wall shapes.
	Type() int
	// Face is the object's rotation (getFace), 0-3.
	Face() int
}

// ObjectDefinition is the minimal definition surface the object
// emits need: the ObjectDefinition accessors the emit and
// registration code reads (ObjectDefinition.ts: getName,
// getSizeX/getSizeY, getBlockingMask, getInteractions). The
// resolved (forPlayer) definition is what the listener supplies;
// the emit's fill-in resolves through the hub's definition
// resolver when the event carries none.
type ObjectDefinition interface {
	// ID is the definition's id (forPlayer resolution may return
	// a different id than the clicked object id).
	ID() int
	// Name is the definition's name (getName).
	Name() string
	// SizeX/SizeY are the footprint sizes (getSizeX/getSizeY).
	SizeX() int
	SizeY() int
	// BlockingMask is the clip mask (getBlockingMask).
	BlockingMask() int
	// Interactions are the menu interactions (getInteractions),
	// or nil when the object has none. Slots with no action
	// read "".
	Interactions() []string
}

// ObjectDefinitionResolver resolves the definition a player sees
// for an object id: ObjectDefinition.forPlayer
// (ObjectDefinition.ts:76-88), including the multi-loc variant
// rule. It returns nil when no definition exists for the id.
// The coordinator installs it from the definition Store; with
// none installed, emits that need a definition run without one
// (name hooks never match, the option fill-in yields "").
type ObjectDefinitionResolver func(id int, player *game.Player) ObjectDefinition

// ItemInfoResolver answers the item name and noted flag the
// named onItemOnObject matching and the noted filter read:
// ItemDefinition.forId(id).getName() / .isNoted()
// (PluginManager.ts:4110-4111). With none installed, every id
// reads name "" and noted false — the TS DEFAULT definition's
// answers — so named handlers never match and a noted filter
// matches only noted=false.
type ItemInfoResolver func(id int) (name string, noted bool)

// ObjectRouteEvent is the payload of the onObjectRoute hooks. It
// mirrors PluginObjectRouteEvent (PluginTypes.ts:149-157): the
// player, the object, the resolved definition, the object id,
// the click type (0 for item uses, which have no option click),
// the source tile, and the two fields hooks may set —
// Destination (a plugin-supplied route target, replacing the
// object route entirely) and BlockAccessFlags (additional
// blocked approach sides, in the object's unrotated local
// frame). Destination nil means "no override", as TS null does.
type ObjectRouteEvent struct {
	Player           *game.Player
	Object           GameObject
	Definition       ObjectDefinition
	ObjectID         int
	ClickType        int
	SourceLocation   game.Location
	Destination      *game.Location
	BlockAccessFlags int
}

// ObjectInteractionEvent is the payload of the object
// interaction hooks (onObjectInteraction, onBeforeObjectInteraction,
// and the onObjectClick family). It mirrors
// PluginObjectInteractionEvent (PluginTypes.ts:137-147): the
// player, the object, the resolved definition, the object id,
// the click type (1-5), the clicked option, the object's tile,
// the tile the player stood on when the interaction fired, and
// the handled flag. Option is the lower-cased interaction text
// the listener computed; HasOption distinguishes "unset" from
// an empty option, because the emit's fill-in
// (PluginManager.ts:1184) only fills an unset option.
// SourceLocation is nil when the emit site did not supply one
// (TS sourceLocation is optional); the listener always supplies
// the arrival tile.
type ObjectInteractionEvent struct {
	Player         *game.Player
	Object         GameObject
	Definition     ObjectDefinition
	ObjectID       int
	ClickType      int
	Option         string
	HasOption      bool
	Location       game.Location
	SourceLocation *game.Location
	Handled        bool
}

// ObjectInteractionHandler is a name/actions or click handler:
// it returns false to fall through to the next handler, and any
// other result claims the interaction (the emit sets Handled).
// It mirrors the `void | boolean` handler shape of the TS
// overloads (PluginTypes.ts:1054-1056, 1294-1317), where a
// return of exactly false falls through and every other return
// (including undefined) claims the event
// (PluginManager.ts:3029-3032, 3451). Plain onObjectInteraction
// handlers instead return nothing and claim by setting
// event.Handled.
type ObjectInteractionHandler func(*ObjectInteractionEvent) bool

// ItemUseFilter mirrors PluginItemUseFilter
// (PluginTypes.ts:692-700). Noted, when non-nil, restricts the
// handler to noted (true) or unnoted (false) uses; nil accepts
// both. Fallback (item-on-object only) runs the handler after
// every other handler, and only when none handled the use —
// for an object's own refusal in place of "Nothing interesting
// happens."
type ItemUseFilter struct {
	Noted    *bool
	Fallback bool
}

// ItemOnObjectEvent is the payload of the onItemOnObject hooks.
// It mirrors PluginItemOnObjectEvent (PluginTypes.ts:710-720):
// the player, the object, the object id, the used item (the
// inventory stack as it stood when the use was clicked — TS
// captures the Item reference and never re-verifies it on
// arrival), the item id, slot, and source widget id
// (interfaceType; 3214 for inventory-use-on clicks), the
// object's tile, and the handled flag. Item is a game.Item
// value: handlers read its id/amount/meta, never mutate the
// player's inventory through it.
type ItemOnObjectEvent struct {
	Player        *game.Player
	Object        GameObject
	ObjectID      int
	Item          game.Item
	ItemID        int
	ItemSlot      int
	InterfaceType int
	Location      game.Location
	Handled       bool
}

// ItemOnObjectHandler is a named onItemOnObject handler: false
// falls through, any other result claims the use (the emit sets
// Handled). It mirrors the named overload's handler shape
// (PluginTypes.ts:1196; PluginManager.ts:4113). Plain
// onItemOnObject handlers return nothing and claim by setting
// event.Handled.
type ItemOnObjectHandler func(*ItemOnObjectEvent) bool

// ObjectOperatedEvent is the payload of the "object:operated"
// custom event (ObjectActionPacketListener.ts:150-173): a loc
// menu option something answered — a plugin-handled click, or
// an engine option the engine covers. A click no plugin took
// and no engine option covers stays silent and emits nothing.
// Option is the lower-cased option text; it may be "" when the
// clicked slot carries no interaction text.
type ObjectOperatedEvent struct {
	Player   *game.Player
	Object   GameObject
	ObjectID int
	Option   string
	Location game.Location
}

// ObjectOperatedEventName is the custom-event name for operated
// locs (ObjectActionPacketListener.OPERATED_EVENT).
const ObjectOperatedEventName = "object:operated"

// EmitObjectOperated emits the object:operated custom event with
// the given payload. A nil hub or a nil event emits nothing.
func (h *Hub) EmitObjectOperated(event *ObjectOperatedEvent) {
	if event == nil {
		return
	}
	h.EmitCustomEvent(ObjectOperatedEventName, event)
}

// ObjectManagerAPI is the Go-idiomatic equivalent of
// getObjectManager (PluginTypes.ts:1386;
// PluginManager.ts:4680-4681) and core.ObjectManager /
// core.MapObjects (PluginManager.ts:2738-2741): the registry
// operations content needs — spawn, query, and remove —
// reduced to the narrow surface the Go engine can honestly
// supply. TS returns the ObjectManager/MapObjects classes
// themselves (static singletons); Go returns this interface,
// installed by the coordinator at boot from the concrete
// registry (phase 13, Worker 1) behind a thin adapter: the
// concrete registry's methods take and return *game.GameObject,
// which satisfies the GameObject interface structurally, so the
// adapter only translates the signatures.
//
// Included, each grounded in ObjectManager.ts / MapObjects.ts:
// Register / Deregister / ReplaceInPlace (the spawn/remove
// surface, with the playerUpdate broadcast flag), ObjectsAt /
// ObjectsInRegion (the scene queries), Exists (the tile check),
// Find / StillStands (the MapObjects click-lookup surface the
// listeners use), and RemovePluginObjects (the shutdown sweep).
//
// Deliberately absent: hideMapObjectFor / showMapObjectFor
// (per-player hidden-loc state needs the session sync layer),
// onRegionChange and perform (engine-internal scene/sync
// plumbing, driven by the flush path, not by content),
// existsLocation (identical to Exists in TS; one method
// suffices), and the MapObjects statics that duplicate the
// registry (add/remove/clear/getHash/ownerObjectsInScene —
// private areas and owner objects do not exist in Go). A nil
// ObjectManagerAPI means the registry is not installed: content
// must check for nil before calling.
type ObjectManagerAPI interface {
	// Register adds object to the world, replacing any object
	// of the same shape on its tile, and — when playerUpdate
	// is true — sends it to the players who see the tile
	// (ObjectManager.register, ObjectManager.ts:238-276).
	Register(obj GameObject, playerUpdate bool)
	// Deregister removes object from the world, and — when
	// playerUpdate is true — sends the removal to the players
	// who see the tile (ObjectManager.deregister,
	// ObjectManager.ts:310-336).
	Deregister(obj GameObject, playerUpdate bool)
	// ReplaceInPlace swaps outgoing for incoming in the same
	// tile and shape slot, as one loc_add_change to the
	// players who see the tile (ObjectManager.replaceInPlace,
	// ObjectManager.ts:277-294).
	ReplaceInPlace(outgoing, incoming GameObject)
	// ObjectsAt returns the runtime objects registered on a
	// tile, in registration order (ObjectManager.objectsAt,
	// ObjectManager.ts:184-186).
	ObjectsAt(loc game.Location) []GameObject
	// ObjectsInRegion returns the runtime objects registered
	// in one 64x64 region (all planes)
	// (ObjectManager.objectsInRegion, ObjectManager.ts:148-151).
	ObjectsInRegion(regionX, regionY int) []GameObject
	// Exists reports whether any runtime object is registered
	// on the tile. The id argument is accepted for TS shape
	// but ignored, exactly as in TS: "any runtime object on
	// the tile counts, whatever its id"
	// (ObjectManager.exists, ObjectManager.ts:391-394).
	Exists(id int, loc game.Location) bool
	// Get returns the loc with id at loc, loading the region
	// first: MapObjects.get with a nil private area
	// (MapObjects.ts:94-122) — the map original when nothing
	// runtime stands there, or nil. A runtime replacement
	// (a stump swapped in for a felled tree) reads back under
	// its own id, so Get answers nil for the replaced id.
	// Content that must see untouched map objects (Woodcutting's
	// treeAt) reads through Get; ObjectsAt sees runtime
	// registrations only.
	Get(id int, loc game.Location) GameObject
	// Find returns the object with id at loc that player
	// would click, or nil: the MapObjects.find surface
	// (MapObjects.ts:64-95) — owner objects first, then the
	// shared map, with the forPlayer variant rule and the
	// bridge-tile z-1 fallback. The concrete registry owns
	// the lookup semantics.
	Find(player *game.Player, id int, loc game.Location) GameObject
	// StillStands reports whether the clicked object, or one
	// just like it (same id, type, and face), still stands at
	// loc (MapObjects.stillStands, MapObjects.ts:101-107).
	StillStands(player *game.Player, id int, loc game.Location, obj GameObject) bool
	// RemovePluginObjects removes every plugin-registered
	// runtime object except stand-ins for map objects, and
	// returns how many were removed
	// (ObjectManager.removePluginObjects,
	// ObjectManager.ts:158-168). The coordinator calls it at
	// shutdown.
	RemovePluginObjects() int
	// RemovedObjects returns the removed-objects list in push
	// order (World.getRemovedObjects(), World.ts:1099-1101):
	// the objects the session layer removed on region change.
	// Content reads it to hide lazy template originals whose
	// removal has no replacement in the shape slot
	// (Woodcutting's treeAt).
	RemovedObjects() []GameObject
}

// SetObjectManager installs the ObjectManagerAPI content reads
// through Api.GetObjectManager and Core.ObjectManager. The
// coordinator calls it once at boot, after the registry exists;
// calling it with nil clears the surface. It is a no-op on a
// nil Hub or a Hub without a registry.
func (h *Hub) SetObjectManager(api ObjectManagerAPI) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setObjectManager(api)
}

// ObjectManager returns the installed ObjectManagerAPI, or nil
// when none is installed (or the Hub is nil).
func (h *Hub) ObjectManager() ObjectManagerAPI {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.objectManager()
}

// SetObjectDefinitionResolver installs the forPlayer definition
// resolver the object emits consult (see
// ObjectDefinitionResolver). The coordinator calls it once at
// boot from the definition Store; calling it with nil clears
// the resolver. It is a no-op on a nil Hub or a Hub without a
// registry.
func (h *Hub) SetObjectDefinitionResolver(fn ObjectDefinitionResolver) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setObjectDefinitionResolver(fn)
}

// SetItemResolver installs the item name/noted resolver the
// named onItemOnObject matching and the noted filter consult
// (see ItemInfoResolver). The coordinator calls it once at
// boot from the definition Store; calling it with nil clears
// the resolver. It is a no-op on a nil Hub or a Hub without a
// registry.
func (h *Hub) SetItemResolver(fn ItemInfoResolver) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setItemResolver(fn)
}

// objectDefinition resolves the definition a player sees for
// id through the installed resolver, or returns nil when no
// resolver is installed.
func (h *Hub) objectDefinition(id int, player *game.Player) ObjectDefinition {
	if h == nil || h.hooks == nil {
		return nil
	}
	if fn := h.hooks.objectDefinitionResolver(); fn != nil {
		return fn(id, player)
	}
	return nil
}

// objectName resolves the name a player sees for an object id
// (the forPlayer variant name), or "" when no resolver is
// installed or no definition exists. Named onItemOnObject
// matching reads it (PluginManager.ts:4111).
func (h *Hub) objectName(id int, player *game.Player) string {
	if def := h.objectDefinition(id, player); def != nil {
		return def.Name()
	}
	return ""
}

// itemInfo resolves an item's name and noted flag through the
// installed resolver, or ("", false) when none is installed.
func (h *Hub) itemInfo(id int) (string, bool) {
	if h == nil || h.hooks == nil {
		return "", false
	}
	if fn := h.hooks.itemResolver(); fn != nil {
		return fn(id)
	}
	return "", false
}

// interactionAt returns the raw interaction text at the
// clickType slot (1-based) of def, or "" when def is nil, the
// click type is out of range, or the slot carries no text. The
// emit's option fill-in and the named-action lookup both read
// it (PluginManager.ts:1184, 3448).
func interactionAt(def ObjectDefinition, clickType int) string {
	if def == nil || clickType < 1 {
		return ""
	}
	interactions := def.Interactions()
	if clickType > len(interactions) {
		return ""
	}
	return interactions[clickType-1]
}

// OnObjectRoute registers an object-route hook, mirroring
// onObjectRoute (PluginTypes.ts:1038). Hooks run in
// registration order until one sets a destination
// (EmitObjectRoute); a hook may also add block-access flags.
// A nil handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:3452-3456).
func (a *Api) OnObjectRoute(handler func(*ObjectRouteEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onObjectRoute") {
		return
	}
	a.hub.hooks.addObjectRoute(hookEntry[*ObjectRouteEvent]{pluginName: a.name, handler: handler})
}

// OnObjectInteraction registers a plain object-interaction
// hook, mirroring the first onObjectInteraction overload
// (PluginTypes.ts:1052). The handler claims the interaction by
// setting event.Handled; unclaimed events fall through to the
// next hook in registration order (EmitObjectInteraction). A
// nil handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:3423-3430).
func (a *Api) OnObjectInteraction(handler func(*ObjectInteractionEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onObjectInteraction") {
		return
	}
	a.hub.hooks.addObjectGeneric(objectInteractionHook{
		entry: hookEntry[*ObjectInteractionEvent]{pluginName: a.name, handler: handler},
		order: a.hub.hooks.takeObjectOrder(),
	})
}

// OnObjectInteractionNamed registers a name/actions object
// interaction, mirroring the second onObjectInteraction
// overload (PluginTypes.ts:1054-1056): exact, case-sensitive
// object-name and option matching. The handler for the
// clicked option — looked up by the RAW interaction text at
// the click-type slot of the event's definition, not by the
// lower-cased event option — runs when the definition's name
// equals objectName; returning false falls through, any other
// result claims the interaction. A nil actions map registers
// nothing, mirroring the createApi guard
// (PluginManager.ts:3428); nil handlers inside the map are
// dropped, mirroring the registration filter
// (PluginManager.ts:3440).
func (a *Api) OnObjectInteractionNamed(objectName string, actions map[string]ObjectInteractionHandler) {
	if actions == nil {
		return
	}
	if !a.canRegister("onObjectInteraction") {
		return
	}
	named := make(map[string]ObjectInteractionHandler, len(actions))
	for option, handler := range actions {
		if handler != nil {
			named[option] = handler
		}
	}
	a.hub.hooks.addObjectByName(objectName, objectInteractionHook{
		entry: hookEntry[*ObjectInteractionEvent]{pluginName: a.name, handler: func(event *ObjectInteractionEvent) {
			if event == nil || event.ClickType < 1 || event.ClickType > 5 {
				return
			}
			action, ok := named[interactionAt(event.Definition, event.ClickType)]
			if !ok {
				return
			}
			if action(event) {
				event.Handled = true
			}
		}},
		order: a.hub.hooks.takeObjectOrder(),
	})
}

// OnBeforeObjectInteraction registers a before-interaction
// hook, mirroring onBeforeObjectInteraction
// (PluginTypes.ts:1059): conditional interaction ownership
// before the normal object handlers. A hook claims the
// interaction by setting event.Handled, which skips every
// normal handler; unclaimed events must be left untouched.
// A nil handler is ignored silently, mirroring the createApi
// guard (PluginManager.ts:3485-3487).
func (a *Api) OnBeforeObjectInteraction(handler func(*ObjectInteractionEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onBeforeObjectInteraction") {
		return
	}
	a.hub.hooks.addBeforeObject(hookEntry[*ObjectInteractionEvent]{pluginName: a.name, handler: handler})
}

// registerObjectClickHook is the shared registration behind
// OnObjectClick and the OnObjectFirstClick..OnObjectFifthClick
// family, mirroring registerObjectClickHook
// (PluginManager.ts:3006-3040): ids are filtered to integers
// >= 0 and deduplicated; no valid id means the registration
// is warned about and dropped; partially invalid lists warn
// and keep the valid ids. One hook (one registration order)
// is filed under every valid "id:clickType" key. family is
// the TS registration name, for the load-phase warning.
func (a *Api) registerObjectClickHook(family string, clickType int, objectIDs []int, handler ObjectInteractionHandler, label string) {
	if handler == nil {
		return
	}
	if !a.canRegister(family) {
		return
	}
	seen := make(map[int]bool, len(objectIDs))
	valid := make([]int, 0, len(objectIDs))
	for _, id := range objectIDs {
		if id < 0 || seen[id] {
			continue
		}
		seen[id] = true
		valid = append(valid, id)
	}
	if len(valid) == 0 {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s attempted invalid %s click hook registration objectIds=%s",
				a.name, label, jsonString(objectIDs))
		}
		return
	}
	if len(valid) != len(objectIDs) {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s %s click hook registration dropped invalid ids (kept %d/%d): objectIds=%s",
				a.name, label, len(valid), len(objectIDs), jsonString(objectIDs))
		}
	}
	hook := objectInteractionHook{
		entry: hookEntry[*ObjectInteractionEvent]{pluginName: a.name, handler: func(event *ObjectInteractionEvent) {
			if handler(event) {
				event.Handled = true
			}
		}},
		order: a.hub.hooks.takeObjectOrder(),
	}
	for _, id := range valid {
		a.hub.hooks.addObjectByID(objectHookKey(id, clickType), hook)
	}
}

// OnObjectClick registers an id/click-type object handler,
// mirroring onObjectClick (PluginTypes.ts:1294-1297). The
// handler runs when the clicked object id and click type
// match; returning false falls through, any other result
// claims the interaction. A click type outside 1-5 is warned
// about and dropped, mirroring the createApi guard
// (PluginManager.ts:4485-4494).
func (a *Api) OnObjectClick(objectIDs []int, clickType int, handler ObjectInteractionHandler) {
	if clickType < 1 || clickType > 5 {
		if a != nil && a.hub != nil {
			if logger := a.hub.Logger(); logger != nil {
				logger.Printf("[plugins] %s attempted invalid object click hook registration objectIds=%s clickType=%d",
					a.name, jsonString(objectIDs), clickType)
			}
		}
		return
	}
	a.registerObjectClickHook("onObjectClick", clickType, objectIDs, handler, "object")
}

// OnObjectFirstClick registers a click-type-1 object handler,
// mirroring onObjectFirstClick (PluginTypes.ts:1299-1301).
func (a *Api) OnObjectFirstClick(objectIDs []int, handler ObjectInteractionHandler) {
	a.registerObjectClickHook("onObjectFirstClick", 1, objectIDs, handler, "first")
}

// OnObjectSecondClick registers a click-type-2 object handler,
// mirroring onObjectSecondClick (PluginTypes.ts:1303-1305).
func (a *Api) OnObjectSecondClick(objectIDs []int, handler ObjectInteractionHandler) {
	a.registerObjectClickHook("onObjectSecondClick", 2, objectIDs, handler, "second")
}

// OnObjectThirdClick registers a click-type-3 object handler,
// mirroring onObjectThirdClick (PluginTypes.ts:1307-1309).
func (a *Api) OnObjectThirdClick(objectIDs []int, handler ObjectInteractionHandler) {
	a.registerObjectClickHook("onObjectThirdClick", 3, objectIDs, handler, "third")
}

// OnObjectFourthClick registers a click-type-4 object handler,
// mirroring onObjectFourthClick (PluginTypes.ts:1311-1313).
func (a *Api) OnObjectFourthClick(objectIDs []int, handler ObjectInteractionHandler) {
	a.registerObjectClickHook("onObjectFourthClick", 4, objectIDs, handler, "fourth")
}

// OnObjectFifthClick registers a click-type-5 object handler,
// mirroring onObjectFifthClick (PluginTypes.ts:1315-1317).
func (a *Api) OnObjectFifthClick(objectIDs []int, handler ObjectInteractionHandler) {
	a.registerObjectClickHook("onObjectFifthClick", 5, objectIDs, handler, "fifth")
}

// OnItemOnObject registers a plain item-on-object hook,
// mirroring the first onItemOnObject overload
// (PluginTypes.ts:1194). The handler claims the use by
// setting event.Handled. filter is optional (nil means no
// filter): a noted restriction and/or the fallback flag (see
// ItemUseFilter). A nil handler is ignored silently,
// mirroring the createApi guard
// (PluginManager.ts:4101-4105).
func (a *Api) OnItemOnObject(handler func(*ItemOnObjectEvent), filter *ItemUseFilter) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemOnObject") {
		return
	}
	noted, fallback := itemUseFilterValues(filter)
	a.hub.hooks.addItemOnObject(hookEntry[*ItemOnObjectEvent]{pluginName: a.name, handler: func(event *ItemOnObjectEvent) {
		if event == nil || event.Handled || event.Player == nil || event.Object == nil {
			return
		}
		if noted != nil {
			if _, isNoted := a.hub.itemInfo(event.ItemID); isNoted != *noted {
				return
			}
		}
		handler(event)
	}}, fallback)
}

// OnItemOnObjectNamed registers a named item-on-object hook,
// mirroring the second onItemOnObject overload
// (PluginTypes.ts:1196): exact item-name and object-name
// matching (the object name is the forPlayer variant name).
// Returning false falls through, any other result claims the
// use. filter is optional (nil means no filter). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:4101-4105).
func (a *Api) OnItemOnObjectNamed(itemName, objectName string, handler ItemOnObjectHandler, filter *ItemUseFilter) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemOnObject") {
		return
	}
	noted, fallback := itemUseFilterValues(filter)
	a.hub.hooks.addItemOnObject(hookEntry[*ItemOnObjectEvent]{pluginName: a.name, handler: func(event *ItemOnObjectEvent) {
		if event == nil || event.Handled || event.Player == nil || event.Object == nil {
			return
		}
		if noted != nil {
			if _, isNoted := a.hub.itemInfo(event.ItemID); isNoted != *noted {
				return
			}
		}
		name, _ := a.hub.itemInfo(event.ItemID)
		if name != itemName || a.hub.objectName(event.ObjectID, event.Player) != objectName {
			return
		}
		if handler(event) {
			event.Handled = true
		}
	}}, fallback)
}

// itemUseFilterValues copies a filter's values at registration
// time, so later mutation of the caller's filter cannot change
// the registered hook. A nil filter yields no noted
// restriction and no fallback.
func itemUseFilterValues(filter *ItemUseFilter) (noted *bool, fallback bool) {
	if filter == nil {
		return nil, false
	}
	if filter.Noted != nil {
		value := *filter.Noted
		noted = &value
	}
	return noted, filter.Fallback
}

// EmitObjectInteraction runs an object interaction through the
// registered hooks and reports whether one claimed it,
// mirroring PluginManager.emitObjectInteraction
// (PluginManager.ts:1170-1212) with label "object_interaction":
// the entry guards (nil event/player/object, or a pre-handled
// event, all return false), the no-hooks early return, the
// definition fill-in (through the installed resolver) and the
// option fill-in (the raw interaction text at the click-type
// slot, "" when the slot is empty), the before-hooks loop (a
// claim returns true without running the main hooks), then
// the merged generic + id + name hooks in original
// registration order, stopping once handled. A nil Hub or a
// Hub without a registry returns false.
//
// The production call site is the object-action listener's
// arrival callback; Api.EmitObjectInteraction exposes the
// same emit to content (PluginTypes.ts:1401).
func (h *Hub) EmitObjectInteraction(event *ObjectInteractionEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Object == nil || event.Handled {
		return false
	}
	before := h.hooks.snapshotBeforeObject()
	if h.hooks.objectHookCount() == 0 && len(before) == 0 {
		return false
	}
	if event.Definition == nil {
		event.Definition = h.objectDefinition(event.ObjectID, event.Player)
	}
	if !event.HasOption {
		event.Option = interactionAt(event.Definition, event.ClickType)
		event.HasOption = true
	}
	if runHandledGate(h.Logger(), "object_interaction", before, event,
		func(e *ObjectInteractionEvent) bool { return e.Handled }) {
		return true
	}
	name, hasName := "", false
	if event.Definition != nil {
		name, hasName = event.Definition.Name(), true
	}
	merged := h.hooks.snapshotObjectInteraction(event.ObjectID, event.ClickType, name, hasName)
	entries := make([]hookEntry[*ObjectInteractionEvent], len(merged))
	for i, hook := range merged {
		entries[i] = hook.entry
	}
	return runHandledGate(h.Logger(), "object_interaction", entries, event,
		func(e *ObjectInteractionEvent) bool { return e.Handled })
}

// EmitObjectRoute runs the object-route hooks in registration
// order until one sets a destination, mirroring
// PluginManager.emitObjectRoute (PluginManager.ts:1214-1224)
// with label "object_route". A nil event, a nil player, or a
// nil object runs no hooks. The production call site is the
// walk-to-object path (MovementQueue.WalkToObject, through the
// listener's route seam): hooks may set Destination to replace
// the object route, or add BlockAccessFlags to restrict the
// approach sides.
func (h *Hub) EmitObjectRoute(event *ObjectRouteEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Object == nil {
		return
	}
	for _, hook := range h.hooks.snapshotObjectRoute() {
		executeHook(h.Logger(), hook.pluginName, "object_route", hook.handler, event)
		if event.Destination != nil {
			return
		}
	}
}

// EmitItemOnObject runs an item-on-object use through the
// registered hooks and reports whether one claimed it,
// mirroring PluginManager.emitItemOnObject
// (PluginManager.ts:1915-1923) with label "item_on_object":
// every main hook runs, then every fallback hook; each hook's
// registration wrapper skips once the event is handled, so
// fallback handlers effectively run only when none handled
// the use. A nil Hub, a Hub without a registry, or a nil
// event returns false (the nil-event guard is Go-only; TS has
// no nil-event equivalent).
//
// The production call site is the item-on-object listener's
// arrival callback; Api.EmitItemOnObject exposes the same
// emit to content (PluginTypes.ts:1407).
func (h *Hub) EmitItemOnObject(event *ItemOnObjectEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	emitHooks(h.Logger(), "item_on_object", h.hooks.snapshotItemOnObject(false), event)
	emitHooks(h.Logger(), "item_on_object", h.hooks.snapshotItemOnObject(true), event)
	return event.Handled
}

// EmitObjectInteraction runs an object interaction through the
// registered hooks and reports whether one claimed it,
// mirroring the PluginApi emitObjectInteraction member
// (PluginTypes.ts:1401) and its createApi passthrough
// (PluginManager.ts:4694). It delegates to
// Hub.EmitObjectInteraction. A nil Api, a nil hub, or a nil
// event yields false. This is an emit, not a registration: it
// has no load-phase gate.
func (a *Api) EmitObjectInteraction(event *ObjectInteractionEvent) bool {
	if a == nil || a.hub == nil {
		return false
	}
	return a.hub.EmitObjectInteraction(event)
}

// EmitItemOnObject runs an item-on-object use through the
// registered hooks and reports whether one claimed it,
// mirroring the PluginApi emitItemOnObject member
// (PluginTypes.ts:1407) and its createApi passthrough
// (PluginManager.ts:4698). It delegates to
// Hub.EmitItemOnObject. A nil Api, a nil hub, or a nil event
// yields false. This is an emit, not a registration: it has
// no load-phase gate.
func (a *Api) EmitItemOnObject(event *ItemOnObjectEvent) bool {
	if a == nil || a.hub == nil {
		return false
	}
	return a.hub.EmitItemOnObject(event)
}

// GetObjectManager returns the installed ObjectManagerAPI,
// mirroring the PluginApi getObjectManager member
// (PluginTypes.ts:1386) and its createApi accessor
// (PluginManager.ts:4680-4681), which returns the
// ObjectManager class. Go returns the narrow interface (see
// ObjectManagerAPI), or nil when the coordinator has not
// installed the registry surface. A nil Api or a nil hub
// yields nil. This is a read, not a registration: it has no
// load-phase gate.
func (a *Api) GetObjectManager() ObjectManagerAPI {
	if a == nil || a.hub == nil {
		return nil
	}
	return a.hub.ObjectManager()
}

// objectHookKey is the by-id hook map key: "id:clickType"
// (PluginManager.ts:1195, 3036).
func objectHookKey(id, clickType int) string {
	return strconv.Itoa(id) + ":" + strconv.Itoa(clickType)
}

// sortObjectHooks orders merged object hooks by registration
// order (PluginManager.ts:1201-1202: "Generic, ID and name
// handlers retain their original registration priority").
func sortObjectHooks(hooks []objectInteractionHook) {
	sort.SliceStable(hooks, func(i, j int) bool { return hooks[i].order < hooks[j].order })
}
