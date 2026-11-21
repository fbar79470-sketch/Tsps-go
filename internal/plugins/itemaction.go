// Item-action hooks: the Go form of the onItemAction,
// onItemFirstAction, and onCanUseItem families of PluginApi
// (PluginTypes.ts) that the engine can fire since phase 32:
// onItemAction (plain and name/actions forms, main and
// fallback), onItemFirstAction, and the onCanUseItem gate,
// plus the Hub emits EmitItemAction and EmitCanUseItem and
// the item-actions resolver the named matching reads.
//
// Every emit below ports its PluginManager.emit* body,
// including the entry guards, the hook ordering, and the
// run-all shape — the families differ from each other, and
// those differences are behavior content relies on:
//
//   - emitItemAction (PluginManager.ts:2016-2043): run ALL
//     main hooks, then ALL fallback hooks; each hook's
//     registration wrapper skips once the event is handled, so
//     a fallback handler effectively runs only when none
//     handled the click. runHandledGate must NOT be substituted
//     (gates.go header). The named matching reads the option
//     through itemActionOption (PluginManager.ts:996-1007):
//     the event's option on the equipment interface, else the
//     item's inventory action at the click-type slot; the
//     click-type range is maxItemActionClick
//     (PluginManager.ts:987-990): 5 on inventory, 10 on the
//     equipment interface (FIRST_WORN_OPTION_CLICK 2,
//     MAX_WORN_OPTION_CLICK 10, PluginManager.ts:955-956).
//     onItemFirstAction (PluginManager.ts:4318-4330) pushes a
//     filtered hook onto the MAIN list: clickType 1 only, skip
//     if already handled, a non-false return claims the click.
//   - emitCanUseItem (PluginManager.ts:1654-1657): the
//     first-answer gate (gates.go runFirstAnswerGate): with no
//     hooks it returns null (proceed); otherwise the first
//     decided answer wins and later hooks never run. The
//     listener call site passes action "action"
//     (ItemActionPacketListener.ts:40); the "magic", "use", and
//     "bonus" call sites are NOT STARTED.
//
// Deliberately NOT ported (gaps, see the emit):
//   - The subOption fill-in (PluginManager.ts:2020-2023):
//     the submenu label comes from the item definition's
//     subops/params, and no Go accessor exists for either, so
//     SubOption stays as the call site supplied it ("" from
//     the two in-scope listener sites).
//   - The item:operated custom emit (PluginManager.ts:2032-2041):
//     it needs the stack amount the click covered, which the
//     event does not carry and the hub cannot read (no
//     inventory access from this package).
//   - The worn-option param lookup inside itemActionOption
//     (PluginManager.ts:1000-1005): on the equipment
//     interface with no event option, the option comes from
//     the definition's params 451+; no Go accessor exists, so
//     a named handler never matches an option-less worn click.
//   - The widget-action emit sites (NetworkBuilder.ts:996-1011
//     equipment branch, 1103-1114 subOpId branch): the Go
//     widget-action path has no equipment-slot branch and no
//     subOpId emit, so only the two ItemActionPacketListener
//     sites (handleAction / handleFirstAction) fire this
//     family; SubOpID is 0 and SubOption "" from both.
//
// The production call sites are the item-action listener's
// handleAction (non-first clicks) and handleFirstAction
// (first clicks) through the cmd/server adapter (see
// itemactionlistenerwiring.go); Api.OnItemAction and
// Api.OnItemFirstAction expose the registrations to content
// (PluginTypes.ts:831-860), and Api.OnCanUseItem the gate.

package plugins

import (
	"tsps-go/internal/game"
)

// PluginItemActionEvent is the payload of the item-action
// hooks (onItemAction and onItemFirstAction). It mirrors
// PluginItemActionEvent (PluginTypes.ts:831-843): the player,
// the interface the click arrived on (3214 inventory, 1688
// equipment), the item id (the item itself is an id, not an
// object — follow the Go ObjectInteractionEvent precedent in
// objects.go), the slot, the click type (1-5 on inventory,
// 1-10 on the equipment interface), the clicked option text
// ("" when the packet carried none), the submenu entry's
// 1-based index (0 when the click picked none — the protocol
// decode's "0 when absent" convention) and cache label (""
// when unset), and the handled flag. Option is the raw packet
// option; the named matching derives its own option from the
// definition (see itemActionOption), exactly as TS does.
type PluginItemActionEvent struct {
	Player      *game.Player
	InterfaceID int
	ItemID      int
	Slot        int
	ClickType   int
	Option      string
	SubOpID     int
	SubOption   string
	Handled     bool
}

// ItemActionHandler is a named onItemAction or
// onItemFirstAction handler: false falls through, any other
// result claims the click (the emit sets Handled). It mirrors
// the `void | boolean` handler shape of the TS overloads
// (PluginTypes.ts:837-848), where a return of exactly false
// falls through and every other return (including undefined)
// claims the event (PluginManager.ts:4292, 4326-4328).
// Plain onItemAction handlers instead return nothing and claim
// by setting event.Handled.
type ItemActionHandler func(*PluginItemActionEvent) bool

// ItemActionOptions mirrors PluginItemActionOptions
// (PluginTypes.ts:702-708). Fallback runs the handler after
// every other item-action handler, and only when none took the
// click: an item's own answer when no content claimed it.
type ItemActionOptions struct {
	Fallback bool
}

// ItemActionsResolver answers the five inventory action texts
// for an item id: definition.inventoryActions (ObjType.ts).
// It is the Go form of the CacheDefinitions.getItem read the
// named onItemAction matching makes
// (PluginManager.ts:4289-4290). With none installed, named
// matching reads the ObjType defaults (see itemActions) — the
// TS DEFAULT definition's answers — so only the Drop option
// (slot 5) can match.
type ItemActionsResolver func(id int) []string

// CanUseItemEvent is the payload of the onCanUseItem hooks.
// It mirrors PluginCanUseItemEvent (PluginTypes.ts:334-342):
// the player, the item id, the action ("action", "use",
// "magic", or "bonus"), the clicked menu option for "action"
// ("" when the call site passed none), and the allow answer:
// nil is undecided (TS null), non-nil is the hook's answer.
type CanUseItemEvent struct {
	Player *game.Player
	ItemID int
	Action string
	Option string
	Allow  *bool
}

// SetItemActionsResolver installs the inventory-actions
// resolver the named onItemAction matching reads (see
// ItemActionsResolver). The coordinator calls it once at boot
// from the definition Store/cache facade; calling it with nil
// clears the resolver. It is a no-op on a nil Hub or a Hub
// without a registry.
func (h *Hub) SetItemActionsResolver(fn ItemActionsResolver) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setItemActionsResolver(fn)
}

// itemActions resolves the five inventory action texts for id
// through the installed resolver, or the ObjType defaults when
// none is installed (or the resolver returns nothing). The
// result is always length 5, padded with "" or truncated,
// matching the listeners' normalization (see
// internal/listeners/helpers.go normalizeActions).
func (h *Hub) itemActions(id int) []string {
	actions := game.DefaultInventoryActions()
	if h != nil && h.hooks != nil {
		if fn := h.hooks.itemActionsResolver(); fn != nil {
			if resolved := fn(id); len(resolved) > 0 {
				actions = resolved
			}
		}
	}
	out := make([]string, 5)
	copy(out, actions)
	return out
}

// maxItemActionClick is the highest click type a named
// onItemAction handler may match on interfaceID: inventory
// items have five options; an equipped item's worn options
// run past that (PluginManager.ts:987-990,
// MAX_WORN_OPTION_CLICK 10).
func maxItemActionClick(interfaceID int) int {
	if interfaceID == game.EquipmentInventoryInterfaceID {
		return 10
	}
	return 5
}

// itemActionOption returns the option a named onItemAction
// handler matches against, mirroring
// PluginManager.itemActionOption (PluginManager.ts:996-1007):
// a worn option when the click came from the equipment tab
// (the event's option, when present), else the item's
// inventory action at the click-type slot. The second return
// value reports whether an option exists at all: false is the
// TS null, and no named action matches. The worn-option param
// lookup (params 451+) is a gap (see the file header): with
// no event option on the equipment interface, no option
// exists here.
func (h *Hub) itemActionOption(event *PluginItemActionEvent) (string, bool) {
	if event == nil {
		return "", false
	}
	if event.InterfaceID == game.EquipmentInventoryInterfaceID {
		if event.Option != "" {
			return event.Option, true
		}
		return "", false
	}
	actions := h.itemActions(event.ItemID)
	index := event.ClickType - 1
	if index < 0 || index >= len(actions) {
		return "", false
	}
	return actions[index], true
}

// OnItemAction registers a plain item-action hook, mirroring
// the first onItemAction overload (PluginTypes.ts:836). The
// handler claims the click by setting event.Handled;
// unclaimed clicks fall through to the next hook in
// registration order (EmitItemAction). A nil handler is
// ignored silently, mirroring the createApi guard
// (PluginManager.ts:4253-4257).
func (a *Api) OnItemAction(handler func(*PluginItemActionEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemAction") {
		return
	}
	a.hub.hooks.addItemAction(hookEntry[*PluginItemActionEvent]{pluginName: a.name, handler: func(event *PluginItemActionEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}}, false)
}

// OnItemActionNamed registers a name/actions item action,
// mirroring the second onItemAction overload
// (PluginTypes.ts:837-842): exact, case-sensitive item-name
// and option matching. The handler for the clicked option —
// looked up by the option itemActionOption derives (the
// inventory action at the click-type slot, or the event's
// option on the equipment interface) — runs when the item's
// definition name equals itemName; returning false falls
// through, any other result claims the click. options is
// optional (nil means no options): options.Fallback files
// the hook on the fallback list, which runs after every main
// hook and only when none handled the click (see
// ItemActionOptions). A nil actions map registers nothing,
// mirroring the createApi guard (PluginManager.ts:4254-4256);
// nil handlers inside the map are dropped, mirroring the
// registration filter (PluginManager.ts:4259).
func (a *Api) OnItemActionNamed(itemName string, actions map[string]ItemActionHandler, options *ItemActionOptions) {
	if actions == nil {
		return
	}
	if !a.canRegister("onItemAction") {
		return
	}
	named := make(map[string]ItemActionHandler, len(actions))
	for option, handler := range actions {
		if handler != nil {
			named[option] = handler
		}
	}
	fallback := options != nil && options.Fallback
	a.hub.hooks.addItemAction(hookEntry[*PluginItemActionEvent]{pluginName: a.name, handler: func(event *PluginItemActionEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		name, _ := a.hub.itemInfo(event.ItemID)
		if name != itemName {
			return
		}
		if event.ClickType < 1 || event.ClickType > maxItemActionClick(event.InterfaceID) {
			return
		}
		option, ok := a.hub.itemActionOption(event)
		if !ok {
			return
		}
		action, ok := named[option]
		if !ok {
			return
		}
		if action(event) {
			event.Handled = true
		}
	}}, fallback)
}

// OnItemFirstAction registers a first-action-only hook,
// mirroring onItemFirstAction (PluginTypes.ts:846-848): the
// hook runs only for clickType 1 and only when no earlier
// hook handled the click, and claims the click unless the
// handler returns false. TS pushes the filtered hook onto
// the MAIN item-action list (PluginManager.ts:4318-4330),
// so it runs in registration order interleaved with the plain
// onItemAction hooks — not on a separate list, and not before
// the generic fallbacks (the fallback list runs after every
// main hook). A nil handler is ignored silently, mirroring
// the createApi guard (PluginManager.ts:4319-4321).
func (a *Api) OnItemFirstAction(handler ItemActionHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onItemFirstAction") {
		return
	}
	a.hub.hooks.addItemAction(hookEntry[*PluginItemActionEvent]{pluginName: a.name, handler: func(event *PluginItemActionEvent) {
		if event == nil || event.Handled || event.ClickType != 1 || event.Player == nil {
			return
		}
		if handler(event) {
			event.Handled = true
		}
	}}, false)
}

// OnCanUseItem registers a can-use-item gate hook, mirroring
// onCanUseItem (PluginTypes.ts:1136). Hooks run in
// registration order through the first-answer gate
// (EmitCanUseItem): the first decided answer wins. A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3830-3832). TS pushes the raw handler
// with no guard wrapper, so Go does the same: the emit
// builds the event, and the handler sees it as-is.
func (a *Api) OnCanUseItem(handler func(*CanUseItemEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanUseItem") {
		return
	}
	a.hub.hooks.addCanUseItem(hookEntry[*CanUseItemEvent]{pluginName: a.name, handler: handler})
}

// EmitItemAction runs an item action through the registered
// hooks and reports whether one claimed it, mirroring
// PluginManager.emitItemAction (PluginManager.ts:2016-2043)
// with label "item_action": every main hook runs, then every
// fallback hook; each hook's registration wrapper skips once
// the event is handled, so fallback handlers effectively run
// only when none handled the click. A nil Hub, a Hub without
// a registry, or a nil event returns false (the nil-event
// guard is Go-only; TS has no nil-event equivalent).
//
// NOT ported here: the subOption fill-in and the
// item:operated custom emit (see the file header). The
// production call sites are the item-action listener's
// handleAction and handleFirstAction, through the cmd/server
// adapter.
func (h *Hub) EmitItemAction(event *PluginItemActionEvent) bool {
	if h == nil || h.hooks == nil || event == nil {
		return false
	}
	emitHooks(h.Logger(), "item_action", h.hooks.snapshotItemAction(false), event)
	emitHooks(h.Logger(), "item_action", h.hooks.snapshotItemAction(true), event)
	return event.Handled
}

// EmitCanUseItem runs the can-use-item hooks through the
// first-answer gate and reports the outcome, mirroring
// PluginManager.emitCanUseItem (PluginManager.ts:1654-1657)
// with label "can_use_item": with no hooks registered it
// returns nil (TS returns null without building an event);
// otherwise the first non-null answer wins and later hooks
// never run. Action is "action", "use", "magic", or "bonus";
// the listener call site passes "action" with the clicked
// menu option ("" when the packet carried none). A nil Hub or
// a Hub without a registry yields nil. There is no emit-level
// player guard: the registration pushes the raw handler,
// exactly as TS does, so the outcome is identical.
//
// The production call site is the item-action listener's
// handleAction, through the cmd/server adapter: a false
// answer refuses the click (handled, never dropped), a true
// or undecided answer proceeds.
func (h *Hub) EmitCanUseItem(player *game.Player, itemID int, action, option string) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &CanUseItemEvent{Player: player, ItemID: itemID, Action: action, Option: option}
	return runFirstAnswerGate(h.Logger(), "can_use_item", h.hooks.snapshotCanUseItem(), event,
		func(e *CanUseItemEvent) *bool { return e.Allow })
}
