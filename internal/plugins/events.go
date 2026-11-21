package plugins

import (
	"regexp"

	"tsps-go/internal/game"
)

// Custom (namespaced) plugin events: subscribe with Api.OnCustomEvent, emit
// with Hub.EmitCustomEvent or Api.EmitCustomEvent. Payloads are opaque
// (any); handlers run in subscription order through executeHook, so a
// panicking handler is logged and the emit continues. Payloads are not
// queued or retained by the hub (PluginTypes.ts:1408).

// customEventNamePattern is the custom-event name rule, mirroring
// isPluginCustomEventName (PluginManager.ts:174-176): exactly one colon,
// non-empty on both sides, no whitespace anywhere.
var customEventNamePattern = regexp.MustCompile(`^[^:\s]+:[^:\s]+$`)

// isCustomEventName reports whether name is a valid custom-event name.
func isCustomEventName(name string) bool {
	return customEventNamePattern.MatchString(name)
}

// OnCustomEvent subscribes handler to the exact namespaced event name, such
// as "mining:success", mirroring onCustomEvent (PluginTypes.ts:1030-1033).
// An invalid name or a nil handler is silently ignored, mirroring the
// createApi guard (PluginManager.ts:3348-3360), which returns without
// logging when either check fails. Registration after Load() completes is
// ignored with a warning, like every other registration (see
// Api.canRegister).
func (a *Api) OnCustomEvent(name string, handler func(any)) {
	if !isCustomEventName(name) || handler == nil {
		return
	}
	if !a.canRegister("onCustomEvent") {
		return
	}
	a.hub.hooks.addCustomEvent(name, hookEntry[any]{pluginName: a.name, handler: handler})
}

// EmitCustomEvent emits a namespaced custom event to all subscribers, in
// subscription order, mirroring PluginManager.emitCustomEvent
// (PluginManager.ts:1117-1136). An invalid name is silently ignored, as is
// an emit with no subscribers. The error label is "custom_event:<name>",
// matching the TS errorLabel/profile name for custom events.
func (h *Hub) EmitCustomEvent(name string, payload any) {
	if h == nil || h.hooks == nil || !isCustomEventName(name) {
		return
	}
	emitHooks(h.Logger(), "custom_event:"+name, h.hooks.snapshotCustomEvent(name), payload)
}

// SubscribeCustomEvent subscribes an engine-internal handler to the
// exact namespaced event name, for coordinator wiring that must
// listen on the custom-event bus without going through a content
// package's Api (the dialogue playback's "npc-dialogue:start"
// subscription, porting the NpcDialogues subscription at
// NpcDialogues.plugin.js:1378, is the first such subscriber). It
// bypasses the Api registration gate and the load seal: engine
// subscriptions are installed by the coordinator at boot, not by
// content during Load, and the registry itself never rejects
// adds. An invalid name or a nil handler is silently ignored, as
// in Api.OnCustomEvent. Handlers run in subscription order with
// the content handlers, under the plugin name "engine".
func (h *Hub) SubscribeCustomEvent(name string, handler func(any)) {
	if h == nil || h.hooks == nil || !isCustomEventName(name) || handler == nil {
		return
	}
	h.hooks.addCustomEvent(name, hookEntry[any]{pluginName: "engine", handler: handler})
}

// ObjectExamineEvent is the payload of the "world:object-examine"
// custom event, emitted by the examine_object dispatch case
// (NetworkBuilder.ts:1291-1300) before the typed examine answer
// is sent. Handlers set Handled to suppress the answer. TS passes
// a plain object {player, objectId, handled}; Go passes a pointer
// so handlers can set Handled through the opaque any payload
// (handlers receive *ObjectExamineEvent).
type ObjectExamineEvent struct {
	Player   *game.Player
	ObjectID int
	Handled  bool
}

// WidgetActionEvent is the payload of the "player:widget-action"
// custom event, emitted by the widget_action dispatch case
// (NetworkBuilder.ts:977-979) and the widget_drag case
// (NetworkBuilder.ts:1219-1222) before any branch runs. TS
// passes {player, packet, handled}; Go passes a pointer so
// handlers can set Handled through the opaque any payload
// (handlers receive *WidgetActionEvent). A handled event skips
// the whole case, including the dialogue-option tail for
// widget_action. Packet holds the decoded packet — a
// protocol.WidgetAction for widget_action, a protocol.WidgetDrag
// for widget_drag — as the listeners package decoded it; the
// coordinator's adapter copies it through from the listener
// seam. The emit itself is the generic Hub.EmitCustomEvent;
// there is no typed Hub emit, following the ObjectExamineEvent
// precedent.
type WidgetActionEvent struct {
	Player  *game.Player
	Packet  any
	Handled bool
}

// WorldInputEvent is the payload of the "player:world-input"
// custom event, emitted by the WORLD_INTERACTIONS preamble
// (NetworkBuilder.ts:688-691) for every world-interaction
// packet, after the TRADING-status skip and before
// closeInterruptibleInterfaces. TS passes {player, packet,
// handled}; Go passes a pointer so handlers can set Handled
// through the opaque any payload (handlers receive
// *WorldInputEvent). A handled event skips the packet entirely
// — no close, no dispatch. PacketType is the TS packet type
// string ("npc_option", "object_option", "player_option",
// "ground_item_action", "inventory_action",
// "inventory_use_on", "item_on_player", "item_on_npc",
// "item_on_object", "item_on_ground", "spell_on_npc",
// "spell_on_player", "spell_on_object", "spell_on_ground";
// "move" is handled by the coordinator's walk path, which
// does not reach the listeners' preamble). Packet holds the
// decoded message the listeners package decoded, as for
// WidgetActionEvent. The emit itself is the generic
// Hub.EmitCustomEvent.
type WorldInputEvent struct {
	Player     *game.Player
	PacketType string
	Packet     any
	Handled    bool
}

// InterfaceCloseEvent is the payload of the "interface:close"
// custom event, emitted by the interface_close dispatch case
// (NetworkBuilder.ts:1230-1236) before the close runs. TS
// passes {player, handled}; Go passes a pointer so handlers
// can set Handled through the opaque any payload (handlers
// receive *InterfaceCloseEvent). A handled event still aborts
// any armed input prompt, then skips the rest of the close.
// The emit itself is the generic Hub.EmitCustomEvent.
type InterfaceCloseEvent struct {
	Player  *game.Player
	Handled bool
}

// PluginsLoadedEvent is the payload of the "plugins:loaded" custom event,
// emitted once at the end of Load(). TS passes { names: Set<string> }
// (PluginManager.ts:780); Go carries the load-ordered slice instead —
// documented deviation: a slice preserves the load order TS keeps in
// loadedPlugins, and a Set has no Go equivalent that keeps iteration order.
// Handlers receive the event by value (as the `any` payload) and must not
// mutate Names: the slice is a copy owned by the emit.
type PluginsLoadedEvent struct {
	Names []string
}

// emitPluginsLoaded emits "plugins:loaded" with the loaded plugin names in
// load order, then seals the registry so further registrations are ignored.
// Load() (loader-owned) calls it once, near the end of the load sequence,
// mirroring loadFromDirectory (PluginManager.ts:780). The seal happens
// AFTER the emit returns, so a plugins:loaded handler can still register —
// matching TS, where registration during the load passes is allowed. A
// second call is a no-op (Load() itself is idempotent and never calls it
// twice).
func (h *Hub) emitPluginsLoaded() {
	if h == nil || h.hooks == nil || h.hooks.isSealed() {
		return
	}
	names := append([]string(nil), h.Loaded()...)
	h.EmitCustomEvent("plugins:loaded", PluginsLoadedEvent{Names: names})
	h.hooks.seal()
}
