package plugins

import (
	"tsps-go/internal/game"
)

// Interface-action click hooks: onInterfaceActionClick,
// onInterfaceActionButton, and refuseWhileBusy
// (PluginTypes.ts:1242-1253; PluginManager.ts:3120-3155,
// 4379-4410), fired by Hub.EmitInterfaceActionClick
// (PluginManager.ts:2076-2105).
//
// TS keeps ONE hook list (interfaceActionClickHooks,
// PluginManager.ts:394-396): the plain form and the button form
// both push onto it, in registration order, and the emit runs
// the single list until the event is handled. The button form's
// per-hook wrapper filters by button id and claims the event
// unless the handler returns exactly false; the plain form's
// wrapper runs every event and claims only when the handler
// sets handled itself. Go keeps the same single list
// (hookRegistry.interfaceActionClick) and the same wrapper
// split.
//
// The emit checks the MultiChatboxPrompt registry FIRST
// (PluginManager.ts:2089-2092): a pending prompt consumes the
// click, sets handled, and no hook runs. The registry is the
// one installed by Hub.SetMultiChatboxPrompt (dialogue.go),
// shared with Api.SendMultiChatboxPrompt; with none installed
// the check is skipped. The option number is the event's slot
// when present, else the action
// (MultiChatboxPrompt.handleInterfaceActionClick,
// MultiChatboxPrompt.ts:141-145) — the slot form of the prompt
// pick, which the Phase 14 registry method takes as its action
// argument. This emit is where the slot form lands: the
// registry method itself is unchanged.
//
// refusesWhileBusy is plugin-declared metadata, not core data:
// refuseWhileBusy registrations add button ids to a hub-owned
// set (busyRefusalButtons, PluginManager.ts:397,4108), and
// InterfaceActionClickOpcode.handle consults it before the
// emit. Hub.RefusesWhileBusy reads the set; with no
// registrations it refuses nothing. The opcode handle itself
// lives in the listeners package (which never imports this
// one); the coordinator wires RefusesWhileBusy and
// EmitInterfaceActionClick into the listener seams.
//
// registerCustomInterface (PluginTypes.ts:1289-1293) stays NOT
// STARTED: the definition is served at /api/interfaces/<groupId>
// over the HTTP ContentApi (net/http/ContentApi.ts), and the
// transport is WS-only. It stays in the unfired-member table in
// hooks.go.

// InterfaceActionClickEvent is the payload of the
// interface-action-click hooks. It mirrors
// PluginInterfaceActionClickEvent (PluginTypes.ts:873-895):
// the player, the clicked button id (a widget uid), the action
// (a button/op number), the optional routing metadata, and the
// handled flag. Optional numeric fields carry Has* presence
// flags, following the ObjectInteractionEvent precedent: TS
// distinguishes an absent field (undefined) from a present
// zero, and the prompt check reads Slot only when present.
// ButtonID and Action are always present: the emit requires
// integer values (PluginManager.ts:2081-2085), and Go ints
// always satisfy that. ArgsData is the local-trigger script
// argument block (IF_TRIGGEROPLOCAL); ScriptTrigger marks a
// cache-script trigger rather than a click.
type InterfaceActionClickEvent struct {
	Player *game.Player
	// ButtonID is the clicked button id (event.buttonId): the
	// widget uid for widget-action and local-trigger clicks,
	// the target widget uid for widget-target clicks, and the
	// resume widget id for dialogue_continue clicks.
	ButtonID int
	// Action is the click action (event.action): the button
	// number for widget-action clicks, the opcode parameter
	// for local triggers (0 for a script trigger), the source
	// slot for widget-target clicks, and the child index for
	// dialogue_continue clicks.
	Action int

	OpID       int
	HasOpID    bool
	GroupID    int
	HasGroupID bool
	ChildID    int
	HasChildID bool
	ItemID     int
	HasItemID  bool
	Slot       int
	HasSlot    bool
	// Option is the click option text, when the packet
	// carried one.
	Option    string
	HasOption bool
	// SubOpID is the op's submenu entry, 1-based (absent
	// when 0/HasSubOpID false).
	SubOpID    int
	HasSubOpID bool

	// Target/source fields: set only by the widget_target
	// route (IF_BUTTONT).
	TargetWidgetID    int32
	HasTargetWidgetID bool
	TargetSlot        int
	HasTargetSlot     bool
	TargetItemID      int
	HasTargetItemID   bool
	SourceWidgetID    int32
	HasSourceWidgetID bool
	SourceSlot        int
	HasSourceSlot     bool
	SourceItemID      int
	HasSourceItemID   bool

	ArgsData []byte
	// ScriptTrigger marks a cache-script trigger
	// (if_triggeroplocal from a cache script): its arguments
	// are in ArgsData.
	ScriptTrigger bool

	Handled bool
}

// InterfaceActionButtonHandler is an onInterfaceActionButton
// handler: it returns false to fall through to the next hook,
// and true claims the click (the emit sets Handled). It mirrors
// the `void | boolean` handler shape of the TS registration
// (PluginTypes.ts:1245-1248), where a return of exactly false
// falls through and every other return (including undefined)
// claims the event (PluginManager.ts:3143-3155). Plain
// onInterfaceActionClick handlers instead return nothing and
// claim by setting event.Handled.
type InterfaceActionButtonHandler func(*InterfaceActionClickEvent) bool

// OnInterfaceActionClick registers a plain interface-action
// hook, mirroring onInterfaceActionClick
// (PluginTypes.ts:1242-1244). The handler runs for every
// interface-action click (after the prompt check) and claims
// it by setting event.Handled; unclaimed events fall through
// to the next hook in registration order
// (EmitInterfaceActionClick). A nil handler is ignored
// silently, mirroring the createApi guard
// (PluginManager.ts:4379-4398).
func (a *Api) OnInterfaceActionClick(handler func(*InterfaceActionClickEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onInterfaceActionClick") {
		return
	}
	a.hub.hooks.addInterfaceActionClick(hookEntry[*InterfaceActionClickEvent]{pluginName: a.name, handler: func(event *InterfaceActionClickEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnInterfaceActionButton registers a button-id-filtered
// interface-action hook, mirroring onInterfaceActionButton
// (PluginTypes.ts:1245-1248). The handler runs only when the
// event's button id is one of buttonIDs, and claims the click
// unless it returns false. Button ids are filtered to
// integers >= 0 and deduplicated; no valid id means the
// registration is warned about and dropped; partially invalid
// lists warn and keep the valid ids, mirroring
// registerInterfaceActionButtonHook
// (PluginManager.ts:3120-3142). A nil handler is ignored
// silently, mirroring the same guard.
func (a *Api) OnInterfaceActionButton(buttonIDs []int, handler InterfaceActionButtonHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onInterfaceActionButton") {
		return
	}
	seen := make(map[int]bool, len(buttonIDs))
	valid := make([]int, 0, len(buttonIDs))
	for _, id := range buttonIDs {
		if id < 0 || seen[id] {
			continue
		}
		seen[id] = true
		valid = append(valid, id)
	}
	if len(valid) == 0 {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s attempted invalid interface_action_button hook registration buttonIds=%s",
				a.name, jsonString(buttonIDs))
		}
		return
	}
	if len(valid) != len(buttonIDs) {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s interface_action_button hook registration dropped invalid ids (kept %d/%d): buttonIds=%s",
				a.name, len(valid), len(buttonIDs), jsonString(buttonIDs))
		}
	}
	buttonIDSet := make(map[int]bool, len(valid))
	for _, id := range valid {
		buttonIDSet[id] = true
	}
	a.hub.hooks.addInterfaceActionClick(hookEntry[*InterfaceActionClickEvent]{pluginName: a.name, handler: func(event *InterfaceActionClickEvent) {
		if event == nil || event.Handled || !buttonIDSet[event.ButtonID] {
			return
		}
		if handler(event) {
			event.Handled = true
		}
	}})
}

// RefuseWhileBusy registers button ids whose clicks answer
// "Please finish what you are doing first." while the player
// is locked, mirroring refuseWhileBusy
// (PluginTypes.ts:1249-1253; PluginManager.ts:4404-4410).
// Only integers >= 0 are recorded; anything else is skipped
// silently, as in TS (no warning). Registration after Load()
// completes is refused with a warning, like every other
// registration (see Api.canRegister).
func (a *Api) RefuseWhileBusy(buttonIDs []int) {
	if !a.canRegister("refuseWhileBusy") {
		return
	}
	for _, id := range buttonIDs {
		if id >= 0 {
			a.hub.hooks.addBusyRefusal(id)
		}
	}
}

// RefusesWhileBusy reports whether buttonID was registered
// with refuseWhileBusy, mirroring
// PluginManager.refusesWhileBusy (PluginManager.ts:2072-2074).
// A nil Hub or a Hub without a registry refuses nothing.
func (h *Hub) RefusesWhileBusy(buttonID int) bool {
	if h == nil || h.hooks == nil {
		return false
	}
	return h.hooks.refusesWhileBusy(buttonID)
}

// EmitInterfaceActionClick runs the interface-action-click
// hooks in registration order until one marks the event
// handled, mirroring emitInterfaceActionClick
// (PluginManager.ts:2076-2105) with label
// "interface_action_click", through the runHandledGate helper
// (gates.go). It returns whether the event ended handled.
//
// The MultiChatboxPrompt check runs FIRST
// (PluginManager.ts:2089-2092): a pending prompt consumes the
// click, sets Handled, and no hook runs. The option number is
// the event's slot when present, else the action
// (MultiChatboxPrompt.ts:141-145). With no prompt registry
// installed the check is skipped.
//
// The entry guards mirror the TS early return: a nil event, a
// nil player, or an already-handled event runs no hooks — and
// no prompt check — and returns FALSE, including the
// pre-handled case, which differs from the bare gate loop and
// is reproduced here, not in the helper (see gates.go and the
// EmitSocialPacket precedent). A nil Hub or a Hub without a
// registry returns false.
//
// The production call sites are the interface-action dispatch
// paths in the listeners package (the widget-action
// fallthrough, the widget-target fallback, local triggers,
// and the dialogue_continue pre-branch), reached through the
// coordinator's adapter; the opcode's dead/busy/teleport
// gates run in the listener's handle, not here, exactly as
// InterfaceActionClickOpcode.handle gates before calling the
// TS emit.
func (h *Hub) EmitInterfaceActionClick(event *InterfaceActionClickEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Handled {
		return false
	}
	if prompts := h.hooks.multiChatboxPrompt(); prompts != nil {
		optionNumber := event.Action
		if event.HasSlot {
			optionNumber = event.Slot
		}
		groupID := 0
		if event.HasGroupID {
			groupID = event.GroupID
		}
		if prompts.HandleInterfaceActionClick(event.Player, event.ButtonID, optionNumber, groupID) {
			event.Handled = true
			return true
		}
	}
	return runHandledGate(h.Logger(), "interface_action_click", h.hooks.snapshotInterfaceActionClick(), event,
		func(e *InterfaceActionClickEvent) bool { return e.Handled })
}
