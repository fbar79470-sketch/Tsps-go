package plugins

import (
	"sync"

	"tsps-go/internal/game"
)

// Simple-click button hooks: onButtonClick and onButton
// (PluginTypes.ts; PluginManager.ts:2052-2074, 3083-3110),
// fired by Hub.EmitButtonClick from the widget-action
// fallthrough's simple-click fallback (the IF_BUTTON opcode
// 13 case, NetworkBuilder.ts:1133-1137).
//
// TS keeps TWO hook lists: buttonClickHooks for onButtonClick
// and buttonHooks for onButton. The emit runs the
// onButtonClick list first, then the onButton list, each
// through the short-circuit handled gate (runHandledGate):
// the first hook to mark the event handled stops its list,
// and a handled event never reaches the second list. The
// registration split mirrors the interface-action pair in
// interfaces.go: onButtonClick is the plain form (every
// button click, claims by setting Handled) and onButton is
// the button-id-filtered form (runs only for its ids, claims
// unless the handler returns exactly false) — the same
// plain/filtered split as onInterfaceActionClick and
// onInterfaceActionButton (PluginManager.ts:3120-3155).
//
// The lists live in buttonClickRegistry, stored on the Hub
// beside the hook registry (see hub.go): the hookRegistry
// struct is owned by hooks.go, so the button lists need
// their own mutex-guarded home. The load seal that gates
// registration still lives in the hook registry
// (Api.canRegister), so the two registries seal together.

// ButtonClickEvent is the payload of the simple-click button
// hooks. It mirrors the TS button-click event
// (PluginTypes.ts): the player, the clicked button id, and
// the handled flag. Unlike the interface-action event it
// carries no action/option/slot: a simple IF_BUTTON click
// has only a widget id.
type ButtonClickEvent struct {
	Player *game.Player
	// ButtonID is the clicked button id (event.buttonId):
	// the widget uid of the simple click.
	ButtonID int

	Handled bool
}

// ButtonClickHandler is an onButton handler: it returns false
// to fall through to the next hook, and true claims the click
// (the emit sets Handled). It mirrors the `void | boolean`
// handler shape of the TS registration, where a return of
// exactly false falls through and every other return
// (including undefined) claims the event — the same contract
// as InterfaceActionButtonHandler (interfaces.go).
type ButtonClickHandler func(*ButtonClickEvent) bool

// buttonClickRegistry holds the two simple-click button hook
// lists for one Hub, guarded by a sync.RWMutex like the hook
// registry: registration happens on the boot goroutine
// (during Load) while emits happen on tick/session
// goroutines. Emits snapshot the lists under a read lock and
// run handlers without holding the lock.
type buttonClickRegistry struct {
	mu sync.RWMutex

	onButtonClick []hookEntry[*ButtonClickEvent]
	onButton      []hookEntry[*ButtonClickEvent]
}

func newButtonClickRegistry() *buttonClickRegistry {
	return &buttonClickRegistry{}
}

func (r *buttonClickRegistry) addOnButtonClick(e hookEntry[*ButtonClickEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onButtonClick = append(r.onButtonClick, e)
}

func (r *buttonClickRegistry) snapshotOnButtonClick() []hookEntry[*ButtonClickEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ButtonClickEvent](nil), r.onButtonClick...)
}

func (r *buttonClickRegistry) addOnButton(e hookEntry[*ButtonClickEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onButton = append(r.onButton, e)
}

func (r *buttonClickRegistry) snapshotOnButton() []hookEntry[*ButtonClickEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*ButtonClickEvent](nil), r.onButton...)
}

// OnButtonClick registers a plain simple-click button hook,
// mirroring onButtonClick (PluginTypes.ts). The handler runs
// for every simple button click and claims it by setting
// event.Handled; unclaimed clicks fall through to the next
// hook in registration order, then to the onButton list
// (EmitButtonClick). A nil handler is ignored silently,
// mirroring the createApi guard.
func (a *Api) OnButtonClick(handler func(*ButtonClickEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onButtonClick") {
		return
	}
	if a.hub.buttonClicks == nil {
		return
	}
	a.hub.buttonClicks.addOnButtonClick(hookEntry[*ButtonClickEvent]{pluginName: a.name, handler: func(event *ButtonClickEvent) {
		if event == nil || event.Handled || event.Player == nil {
			return
		}
		handler(event)
	}})
}

// OnButton registers a button-id-filtered simple-click button
// hook, mirroring onButton (PluginTypes.ts). The handler runs
// only when the event's button id is one of buttonIDs, and
// claims the click unless it returns false. Button ids are
// filtered to integers >= 0 and deduplicated; no valid id
// means the registration is warned about and dropped;
// partially invalid lists warn and keep the valid ids,
// mirroring registerInterfaceActionButtonHook
// (PluginManager.ts:3120-3142). A nil handler is ignored
// silently, mirroring the same guard.
func (a *Api) OnButton(buttonIDs []int, handler ButtonClickHandler) {
	if handler == nil {
		return
	}
	if !a.canRegister("onButton") {
		return
	}
	if a.hub.buttonClicks == nil {
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
			logger.Printf("[plugins] %s attempted invalid button hook registration buttonIds=%s",
				a.name, jsonString(buttonIDs))
		}
		return
	}
	if len(valid) != len(buttonIDs) {
		if logger := a.hub.Logger(); logger != nil {
			logger.Printf("[plugins] %s button hook registration dropped invalid ids (kept %d/%d): buttonIds=%s",
				a.name, len(valid), len(buttonIDs), jsonString(buttonIDs))
		}
	}
	buttonIDSet := make(map[int]bool, len(valid))
	for _, id := range valid {
		buttonIDSet[id] = true
	}
	a.hub.buttonClicks.addOnButton(hookEntry[*ButtonClickEvent]{pluginName: a.name, handler: func(event *ButtonClickEvent) {
		if event == nil || event.Handled || !buttonIDSet[event.ButtonID] {
			return
		}
		if handler(event) {
			event.Handled = true
		}
	}})
}

// EmitButtonClick runs the simple-click button hooks for a
// simple IF_BUTTON click, mirroring emitButtonClick
// (PluginManager.ts:2052-2074) through the runHandledGate
// helper (gates.go): the onButtonClick hooks first, then the
// onButton hooks, each list short-circuiting on handled —
// and a handled event never reaches the second list. It
// returns whether the event ended handled.
//
// The entry guards mirror the EmitInterfaceActionClick
// precedent: a nil event, a nil player, or an already-handled
// event runs no hooks and returns FALSE, including the
// pre-handled case. A nil Hub or a Hub without a registry
// returns false.
//
// The production call site is the widget-action fallthrough's
// simple-click fallback in the listeners package (the
// IF_BUTTON opcode 13 case, NetworkBuilder.ts:1133-1137),
// reached through the coordinator's adapter; the opcode's
// interface-action dispatch runs first, and only an unclaimed
// simple click reaches this emit.
func (h *Hub) EmitButtonClick(event *ButtonClickEvent) bool {
	if h == nil || h.buttonClicks == nil || event == nil || event.Player == nil || event.Handled {
		return false
	}
	if runHandledGate(h.Logger(), "button_click", h.buttonClicks.snapshotOnButtonClick(), event,
		func(e *ButtonClickEvent) bool { return e.Handled }) {
		return true
	}
	return runHandledGate(h.Logger(), "button", h.buttonClicks.snapshotOnButton(), event,
		func(e *ButtonClickEvent) bool { return e.Handled })
}
