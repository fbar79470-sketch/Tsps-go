package main

import (
	"tsps-go/internal/definition"
	"tsps-go/internal/game"
	"tsps-go/internal/listeners"
	"tsps-go/internal/plugins"
)

// Object-listener wiring: the cmd/server adapter between the
// object-action / item-on-object listeners (internal/listeners),
// the game object registry (internal/game), the definition Store,
// and the plugin hub (internal/plugins). The listeners never
// import the definition or plugins packages; the hub never
// imports game types in exported signatures beyond *game.Player
// and game.Location; this package imports all four, as the flush
// and ground-item adapters do.
//
// The coordinator wires it in two places in server.go (both
// edits are the coordinator's — this file only defines the
// adapters):
//
//   - In buildServer, next to the examine wiring
//     (adapter.examine / adapter.objectExamineIntercept):
//     InstallObjectResolvers(hub, store) for the hub's
//     definition/item resolvers, hub.SetObjectManager(
//     NewObjectManager(world.GameObjects())) for the content
//     registry surface, and a stored
//     NewObjectDefinitionSource(store) on a new adapter field
//     for the per-player Deps.
//   - In AddPlayer's Deps block, after
//     deps.ObjectExamineIntercept: InstallObjectListeners(
//     &deps, a.plugins, a.world.GameObjects(), a.objectDefs).
//
// A nil hub, nil registry, or nil store degrades each adapter
// to its documented no-op: the listeners drop with their
// unwired reasons and the hub emits run without definitions,
// exactly as the examine and chat seams degrade.

// storeObjectDefinitions implements listeners.ObjectDefinitionSource
// over a *definition.Store. *definition.ObjectDefinition satisfies
// the listeners.ObjectDefinition interface structurally (same
// accessor set), so the adapter only translates the lookup
// signatures. A nil Store is never wrapped —
// NewObjectDefinitionSource returns nil instead, and the
// listeners drop with their unwired reasons.
type storeObjectDefinitions struct {
	store *definition.Store
}

var _ listeners.ObjectDefinitionSource = storeObjectDefinitions{}

// NewObjectDefinitionSource returns the object-definition source
// for store, or nil when store is nil.
func NewObjectDefinitionSource(store *definition.Store) listeners.ObjectDefinitionSource {
	if store == nil {
		return nil
	}
	return storeObjectDefinitions{store: store}
}

// ObjectForPlayer is ObjectDefinition.forPlayer on the object's
// id. The player argument is accepted for the interface shape
// but the live var reader does not exist in Go, so the lookup
// runs with a nil VarReader: the documented Store degradation
// ("no vars set"), which resolves multi-loc variants to the
// default (last) transform. The registry's own forPlayer rule
// (its variant/vars sources, when the coordinator installs
// them) runs first, at lookup time.
func (s storeObjectDefinitions) ObjectForPlayer(id int, player *game.Player) (listeners.ObjectDefinition, bool) {
	_ = player
	def, ok := s.store.ObjectForPlayer(id, nil)
	if !ok {
		return nil, false
	}
	return def, true
}

// ObjectByID is ObjectDefinition.forId: the base definition, for
// the type 10/11 footprint rule.
func (s storeObjectDefinitions) ObjectByID(id int) (listeners.ObjectDefinition, bool) {
	def, ok := s.store.Object(id)
	if !ok {
		return nil, false
	}
	return def, true
}

// InstallObjectResolvers installs the hub's object-definition and
// item resolvers over the Store: the forPlayer definition lookup
// (nil-var degradation, as above) and the item name/noted lookup
// behind the named onItemOnObject matching and the noted filter.
// A nil hub or nil store installs nothing: the hub degrades to
// running the object emits without definitions (name hooks never
// match, the option fill-in yields "").
func InstallObjectResolvers(hub *plugins.Hub, store *definition.Store) {
	if hub == nil || store == nil {
		return
	}
	hub.SetObjectDefinitionResolver(func(id int, player *game.Player) plugins.ObjectDefinition {
		_ = player // Nil-var degradation; see ObjectForPlayer above.
		def, ok := store.ObjectForPlayer(id, nil)
		if !ok {
			return nil
		}
		return def
	})
	hub.SetItemResolver(func(id int) (string, bool) {
		item := store.Item(id)
		return item.Name(), item.IsNoted()
	})
}

// objectRegistryLookup adapts *game.ObjectRegistry to
// listeners.ObjectLookup. *game.GameObject satisfies the
// listeners.ObjectEntry interface structurally, so the adapter
// only translates the Find signature (the registry returns the
// concrete type; the interface returns the narrow view).
type objectRegistryLookup struct {
	registry *game.ObjectRegistry
}

var _ listeners.ObjectLookup = objectRegistryLookup{}

// Find is ObjectRegistry.Find: the MapObjects click lookup,
// including the forPlayer variant rule and the bridge-tile
// fallback. A nil registry never finds anything.
func (l objectRegistryLookup) Find(player *game.Player, id int, loc game.Location) listeners.ObjectEntry {
	if l.registry == nil {
		return nil
	}
	obj := l.registry.Find(player, id, loc)
	if obj == nil {
		return nil
	}
	return obj
}

// ObjectHooks adapts the hub's object emits to
// listeners.ObjectHooks. A nil hub yields the zero value: every
// emit degrades to its no-hooks default. The listener event
// structs and the hub event structs share method sets for their
// Object/Definition fields (both narrow interfaces over the same
// accessors), so the translation is a field copy; the Handled
// claim is written back onto the listener event, and the route
// request's Destination / BlockAccessFlags are read back after
// the route emit.
func ObjectHooks(hub *plugins.Hub) listeners.ObjectHooks {
	if hub == nil {
		return listeners.ObjectHooks{}
	}
	return listeners.ObjectHooks{
		EmitObjectRoute: func(req *listeners.ObjectRouteRequest) {
			ev := &plugins.ObjectRouteEvent{
				Player:         req.Player,
				Object:         req.Object,
				Definition:     req.Definition,
				ObjectID:       req.ObjectID,
				ClickType:      req.ClickType,
				SourceLocation: req.Source,
			}
			hub.EmitObjectRoute(ev)
			req.Destination = ev.Destination
			req.BlockAccessFlags = ev.BlockAccessFlags
		},
		EmitObjectInteraction: func(ev *listeners.ObjectInteractionEvent) bool {
			source := ev.SourceLocation
			hubEv := &plugins.ObjectInteractionEvent{
				Player:         ev.Player,
				Object:         ev.Object,
				Definition:     ev.Definition,
				ObjectID:       ev.ObjectID,
				ClickType:      ev.ClickType,
				Option:         ev.Option,
				HasOption:      ev.HasOption,
				Location:       ev.Location,
				SourceLocation: &source,
				Handled:        ev.Handled,
			}
			handled := hub.EmitObjectInteraction(hubEv)
			ev.Handled = hubEv.Handled
			return handled
		},
		EmitItemOnObject: func(ev *listeners.ItemOnObjectEvent) bool {
			hubEv := &plugins.ItemOnObjectEvent{
				Player:        ev.Player,
				Object:        ev.Object,
				ObjectID:      ev.ObjectID,
				Item:          ev.Item,
				ItemID:        ev.ItemID,
				ItemSlot:      ev.ItemSlot,
				InterfaceType: ev.InterfaceType,
				Location:      ev.Location,
				Handled:       ev.Handled,
			}
			handled := hub.EmitItemOnObject(hubEv)
			ev.Handled = hubEv.Handled
			return handled
		},
		EmitItemOnItem: func(ev *listeners.ItemOnItemEvent) bool {
			hubEv := &plugins.PluginItemOnItemEvent{
				Player:           ev.Player,
				UsedItem:         ev.UsedItem,
				UsedItemID:       ev.UsedItemID,
				UsedItemSlot:     ev.UsedItemSlot,
				UsedWithItem:     ev.UsedWithItem,
				UsedWithItemID:   ev.UsedWithItemID,
				UsedWithItemSlot: ev.UsedWithItemSlot,
				Handled:          ev.Handled,
			}
			handled := hub.EmitItemOnItem(hubEv)
			ev.Handled = hubEv.Handled
			return handled
		},
		EmitOperated: func(ev *listeners.ObjectOperatedEvent) {
			hub.EmitObjectOperated(&plugins.ObjectOperatedEvent{
				Player:   ev.Player,
				Object:   ev.Object,
				ObjectID: ev.ObjectID,
				Option:   ev.Option,
				Location: ev.Location,
			})
		},
	}
}

// InstallObjectListeners fills the object seams on deps: the
// registry lookup, the definition source, and the hub hook
// adapters. A nil deps is a no-op; a nil registry leaves
// deps.Objects unset (the unwired drop); a nil defs clears
// deps.ObjectDefs; a nil hub yields the zero (no-hooks) hook
// set. The coordinator calls it once per entering player, in
// AddPlayer's Deps block.
func InstallObjectListeners(deps *listeners.Deps, hub *plugins.Hub, registry *game.ObjectRegistry, defs listeners.ObjectDefinitionSource) {
	if deps == nil {
		return
	}
	if registry != nil {
		deps.Objects = objectRegistryLookup{registry: registry}
	}
	deps.ObjectDefs = defs
	deps.ObjectHooks = ObjectHooks(hub)
}

// registryObjectManager adapts *game.ObjectRegistry to
// plugins.ObjectManagerAPI: the getObjectManager /
// core.ObjectManager surface content calls. The registry's
// methods take and return *game.GameObject, which satisfies the
// plugins.GameObject interface structurally, so the adapter only
// translates the signatures — except where content could hand
// in a foreign GameObject implementation: Register, Deregister,
// and ReplaceInPlace ignore objects that are not
// *game.GameObject (content constructs locs through
// game.NewGameObject; nothing else can stand in the registry),
// and StillStands treats a foreign object as absent.
type registryObjectManager struct {
	registry *game.ObjectRegistry
}

var _ plugins.ObjectManagerAPI = registryObjectManager{}

// NewObjectManager returns the ObjectManagerAPI over registry,
// or nil when registry is nil (the hub then reports no manager
// installed). The coordinator installs the result with
// Hub.SetObjectManager at boot.
func NewObjectManager(registry *game.ObjectRegistry) plugins.ObjectManagerAPI {
	if registry == nil {
		return nil
	}
	return registryObjectManager{registry: registry}
}

// asGameObject unwraps a hub GameObject to the concrete registry
// type, or nil when it is not one.
func asGameObject(obj plugins.GameObject) *game.GameObject {
	g, _ := obj.(*game.GameObject)
	return g
}

func (m registryObjectManager) Register(obj plugins.GameObject, playerUpdate bool) {
	if g := asGameObject(obj); g != nil {
		m.registry.Register(g, playerUpdate)
	}
}

func (m registryObjectManager) Deregister(obj plugins.GameObject, playerUpdate bool) {
	if g := asGameObject(obj); g != nil {
		m.registry.Deregister(g, playerUpdate)
	}
}

func (m registryObjectManager) ReplaceInPlace(outgoing, incoming plugins.GameObject) {
	out, in := asGameObject(outgoing), asGameObject(incoming)
	if out != nil && in != nil {
		m.registry.ReplaceInPlace(out, in)
	}
}

func (m registryObjectManager) ObjectsAt(loc game.Location) []plugins.GameObject {
	list := m.registry.At(loc)
	if len(list) == 0 {
		return nil
	}
	out := make([]plugins.GameObject, len(list))
	for i, obj := range list {
		out[i] = obj
	}
	return out
}

func (m registryObjectManager) ObjectsInRegion(regionX, regionY int) []plugins.GameObject {
	list := m.registry.InRegion(regionX, regionY)
	if len(list) == 0 {
		return nil
	}
	out := make([]plugins.GameObject, len(list))
	for i, obj := range list {
		out[i] = obj
	}
	return out
}

func (m registryObjectManager) Exists(id int, loc game.Location) bool {
	return m.registry.Exists(id, loc)
}

// Get is the registry's MapObjects.get lookup: the loc with id
// at loc, loading the region first — the map original when
// nothing runtime stands there, or nil.
func (m registryObjectManager) Get(id int, loc game.Location) plugins.GameObject {
	obj := m.registry.Get(id, loc)
	if obj == nil {
		return nil
	}
	return obj
}

func (m registryObjectManager) Find(player *game.Player, id int, loc game.Location) plugins.GameObject {
	obj := m.registry.Find(player, id, loc)
	if obj == nil {
		return nil
	}
	return obj
}

// StillStands is the registry's non-quiet still-stands check:
// the default form TS arrival closures use (the quiet form is
// the walk-to validity check's, internal to the queue path).
// The DEVELOPER notice the non-quiet form can send is the
// registry's own, through its rights source.
func (m registryObjectManager) StillStands(player *game.Player, id int, loc game.Location, obj plugins.GameObject) bool {
	return m.registry.StillStands(player, id, loc, asGameObject(obj), false)
}

func (m registryObjectManager) RemovePluginObjects() int {
	return m.registry.RemovePluginObjects()
}

// RemovedObjects is the registry's removed-objects list
// (World.getRemovedObjects, World.ts:1099-1101), converted to
// the hub GameObject form like ObjectsAt.
func (m registryObjectManager) RemovedObjects() []plugins.GameObject {
	list := m.registry.RemovedObjects()
	if len(list) == 0 {
		return nil
	}
	out := make([]plugins.GameObject, len(list))
	for i, obj := range list {
		out[i] = obj
	}
	return out
}
