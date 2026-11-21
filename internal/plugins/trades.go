// Trade hooks and the scoped TradeState core surface: the Go form of
// the trading families of PluginApi (PluginTypes.ts) that the engine
// can fire since phase 17: onCanTrade, onTradeRequest, and
// onTradeCompleted, plus the TradeState core surface (PluginCoreApi).
//
// Every emit below ports its PluginManager.emit* body, including the
// entry guards, the registration wrappers, the hook ordering, and the
// short-circuit/run-all shape — the families differ from each other,
// and those differences are behavior content relies on:
//
//   - emitCanTrade (PluginManager.ts:1456-1469): first-answer gate
//     (runFirstAnswerGate, gates.go); the zero-hooks early return is
//     unobservable (the gate returns nil with no hooks). The
//     registration wrapper skips events without a player or target
//     (PluginManager.ts:3673-3686).
//   - emitTradeRequest (PluginManager.ts:1471-1481): short-circuit
//     handled gate (runHandledGate, gates.go); entry guards (nil
//     event, pre-handled, or missing player/target return false).
//     The registration wrapper skips events without a player or
//     target, or already handled (PluginManager.ts:3687-3700).
//   - emitTradeCompleted (PluginManager.ts:1483-1491): run ALL hooks;
//     entry guards (nil event/player/partner run none). The
//     registration wrapper skips events without a player or partner
//     (PluginManager.ts:3701-3714); its Array.isArray(received) check
//     has no Go form beyond the slice type (see OnTradeCompleted).
//
// EMIT SITES (Worker B / coordinator; this file owns only the hub side):
//
//   - emitCanTrade fires from Trading.canTrade (Trading.ts:142-147),
//     after the area canTrade checks: `!== false` allows the trade,
//     so a nil answer (no hooks, or silence) allows it. It is
//     consulted both when a trade is requested (requestTrade,
//     Trading.ts:151) and when one is accepted (acceptTrade,
//     Trading.ts:262-265) — a trade that becomes ungated mid-flow
//     is closed, not completed.
//   - emitTradeRequest fires from TradeRequestPacketListener.request
//     (TradeRequestPacketListener.ts:30), after the index/range/
//     hitpoints/registration validation and BEFORE the walk to the
//     target: a handled event replaces the whole default
//     walk-and-request flow. The listener-side walk and the
//     sendRequest busy/local checks are Worker B's.
//   - emitTradeCompleted fires TWICE per completed trade, once per
//     player with received/given swapped (Trading.ts:323-330),
//     after the offered items have changed hands and BEFORE either
//     player is saved — container mutations hooks make here (e.g.
//     converting a traded bond) are persisted with the trade.
//     The dual emit is the call site's job (Worker B's acceptTrade
//     port), not the hub's: Hub.EmitTradeCompleted fires one
//     player's event.
//
// CORE SURFACE (following the getObjectManager precedent, but
// WITHOUT an Api member: TS has no getTrading member — TradeState
// reaches content only through core, PluginTypes.ts:1746, built in
// getCoreApi, PluginManager.ts:2794): the TradeState enum below
// mirrors the TS enum order exactly, and Core.TradeState reads one
// player's current state through an installed TradeStateReader.
// Content's demonstrated use is the state comparison (the trade
// interface plugin compares getState() to CONFIRM_SCREEN). There
// is deliberately no Trading class mirror: the trade flow (offer,
// accept, decline, transfer) is Worker B's engine system, reached
// through the player, not through Core.
//
// NOT PORTED: the trade:item-eligibility custom event has no hub
// constant or payload type — it fires through the generic
// custom-event mechanism at Worker B's canOffer site
// (Trading.ts:447-453), and its payload ({item, allowed}) is
// Worker B's game-side shape. DuelRule / the Dueling state class
// are Worker B's game types (the Duel Arena gameplay is plugin
// content, NOT STARTED); no Core mirror is offered for them.

package plugins

import (
	"tsps-go/internal/game"
)

// CanTradeEvent is the payload of the onCanTrade hooks. It mirrors
// PluginCanTradeEvent (PluginTypes.ts:417-421): the player, the
// target, and the allow answer. Allow is a *bool: nil is TS null
// (undecided — the trade is allowed), non-nil is the hook's
// answer. A hook sets Allow through BoolPtr (gates.go).
type CanTradeEvent struct {
	Player *game.Player
	Target *game.Player
	Allow  *bool
}

// TradeRequestEvent is the payload of the onTradeRequest hooks. It
// mirrors PluginTradeRequestEvent (PluginTypes.ts:424-428): the
// player, the target, and the handled flag. A hook that sets
// Handled replaces the default walk-and-request flow entirely
// (TradeRequestPacketListener.ts:30-32).
type TradeRequestEvent struct {
	Player  *game.Player
	Target  *game.Player
	Handled bool
}

// TradeCompletedEvent is the payload of the onTradeCompleted hooks.
// It mirrors PluginTradeCompletedEvent (PluginTypes.ts:436-442):
// the player, the partner, and the item stacks this player gained
// (Received) and handed over (Given). Received and Given are the
// trade containers' valid items at completion (Trading.ts:317-320):
// handlers read them, and mutate the player's containers (not
// these slices) to change what the post-trade save persists. Item
// is a game.Item value: handlers read its id/amount/meta, never
// mutate the player's containers through it.
type TradeCompletedEvent struct {
	Player   *game.Player
	Partner  *game.Player
	Received []game.Item
	Given    []game.Item
}

// --- hookRegistry storage for the trade families (see hooks.go) ---

func (r *hookRegistry) addCanTrade(e hookEntry[*CanTradeEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.canTrade = append(r.canTrade, e)
}

func (r *hookRegistry) snapshotCanTrade() []hookEntry[*CanTradeEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*CanTradeEvent](nil), r.canTrade...)
}

func (r *hookRegistry) addTradeRequest(e hookEntry[*TradeRequestEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tradeRequest = append(r.tradeRequest, e)
}

func (r *hookRegistry) snapshotTradeRequest() []hookEntry[*TradeRequestEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*TradeRequestEvent](nil), r.tradeRequest...)
}

func (r *hookRegistry) addTradeCompleted(e hookEntry[*TradeCompletedEvent]) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tradeCompleted = append(r.tradeCompleted, e)
}

func (r *hookRegistry) snapshotTradeCompleted() []hookEntry[*TradeCompletedEvent] {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]hookEntry[*TradeCompletedEvent](nil), r.tradeCompleted...)
}

// --- registration methods ---

// OnCanTrade registers a can-trade hook, mirroring onCanTrade
// (PluginTypes.ts:1117). Hooks run in registration order until
// one answers (EmitCanTrade). A nil handler is ignored silently,
// mirroring the createApi guard (PluginManager.ts:3673-3686). The
// stored handler wraps the plugin's with the TS guard: an event
// without a player or target never reaches the plugin's handler.
func (a *Api) OnCanTrade(handler func(*CanTradeEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onCanTrade") {
		return
	}
	a.hub.hooks.addCanTrade(hookEntry[*CanTradeEvent]{pluginName: a.name, handler: func(event *CanTradeEvent) {
		if event == nil || event.Player == nil || event.Target == nil {
			return
		}
		handler(event)
	}})
}

// OnTradeRequest registers a trade-request hook, mirroring
// onTradeRequest (PluginTypes.ts:1118). Hooks run in registration
// order until one sets Handled (EmitTradeRequest). A nil handler
// is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3687-3700). The stored handler wraps the
// plugin's with the TS guard: a nil event, an already-handled
// event, or an event without a player or target never reaches
// the plugin's handler.
func (a *Api) OnTradeRequest(handler func(*TradeRequestEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onTradeRequest") {
		return
	}
	a.hub.hooks.addTradeRequest(hookEntry[*TradeRequestEvent]{pluginName: a.name, handler: func(event *TradeRequestEvent) {
		if event == nil || event.Handled || event.Player == nil || event.Target == nil {
			return
		}
		handler(event)
	}})
}

// OnTradeCompleted registers a trade-completed hook, mirroring
// onTradeCompleted (PluginTypes.ts:1119). Hooks run in
// registration order, all of them (EmitTradeCompleted). A nil
// handler is ignored silently, mirroring the createApi guard
// (PluginManager.ts:3701-3714). The stored handler wraps the
// plugin's with the TS guard: an event without a player or
// partner never reaches the plugin's handler. (The guard's
// Array.isArray(received) check has no Go form beyond the slice
// type: Received is always a []game.Item, nil or not, and the
// emit sites always build it from the trade containers — the
// wrapper checks the event and players only, and the emit's
// entry guard likewise treats the slices as always present.)
func (a *Api) OnTradeCompleted(handler func(*TradeCompletedEvent)) {
	if handler == nil {
		return
	}
	if !a.canRegister("onTradeCompleted") {
		return
	}
	a.hub.hooks.addTradeCompleted(hookEntry[*TradeCompletedEvent]{pluginName: a.name, handler: func(event *TradeCompletedEvent) {
		if event == nil || event.Player == nil || event.Partner == nil {
			return
		}
		handler(event)
	}})
}

// --- Hub emits ---

// EmitCanTrade asks the registered hooks whether player may trade
// with target, mirroring PluginManager.emitCanTrade
// (PluginManager.ts:1456-1469) with label "can_trade": the first
// non-nil Allow wins, later hooks never run, and no answer yields
// nil. A nil Hub or a Hub without a registry yields nil. A nil
// player or target yields nil through the registration wrappers
// (which skip player/target-less events), so no emit-level guard
// is added — the outcome is identical to TS, where the wrappers
// do the same work.
//
// The production call site is Trading.canTrade (Worker B): only a
// false answer refuses the trade (`!== false` allows it). See the
// file header for the request-time and accept-time positions.
func (h *Hub) EmitCanTrade(player, target *game.Player) *bool {
	if h == nil || h.hooks == nil {
		return nil
	}
	event := &CanTradeEvent{Player: player, Target: target}
	return runFirstAnswerGate(h.Logger(), "can_trade", h.hooks.snapshotCanTrade(), event,
		func(e *CanTradeEvent) *bool { return e.Allow })
}

// EmitTradeRequest runs a trade request through the registered
// hooks and reports whether one took it over, mirroring
// PluginManager.emitTradeRequest (PluginManager.ts:1471-1481)
// with label "trade_request": the entry guards (nil event, a
// pre-handled event, or a missing player/target all return
// false), then the short-circuit handled loop. A nil Hub or a
// Hub without a registry returns false.
//
// The production call site is the trade-request listener (Worker
// B), before the walk to the target; a true return replaces the
// default walk-and-request flow entirely.
func (h *Hub) EmitTradeRequest(event *TradeRequestEvent) bool {
	if h == nil || h.hooks == nil || event == nil || event.Handled || event.Player == nil || event.Target == nil {
		return false
	}
	return runHandledGate(h.Logger(), "trade_request", h.hooks.snapshotTradeRequest(), event,
		func(e *TradeRequestEvent) bool { return e.Handled })
}

// EmitTradeCompleted runs one player's completed trade through
// the registered hooks, mirroring
// PluginManager.emitTradeCompleted (PluginManager.ts:1483-1491)
// with label "trade_completed": every hook runs. A nil Hub, a Hub
// without a registry, a nil event, or an event without a player
// or partner runs no hooks.
//
// The production call site is Trading.acceptTrade (Worker B),
// which emits TWICE per completed trade (once per player, with
// received/given swapped) before either player is saved — see
// the file header.
func (h *Hub) EmitTradeCompleted(event *TradeCompletedEvent) {
	if h == nil || h.hooks == nil || event == nil || event.Player == nil || event.Partner == nil {
		return
	}
	emitHooks(h.Logger(), "trade_completed", h.hooks.snapshotTradeCompleted(), event)
}

// --- Scoped TradeState core surface ---

// TradeState is a player's trading state: the TS TradeState enum
// (Trading.ts:655: NONE, REQUESTED_TRADE, TRADE_SCREEN,
// ACCEPTED_TRADE_SCREEN, CONFIRM_SCREEN, ACCEPTED_CONFIRM_SCREEN)
// as a Go enum, in the same order. It is the core.TradeState
// equivalent (PluginTypes.ts:1746; PluginManager.ts:2794):
// content compares a player's state against these constants.
// Worker B's game-side trading state is the authority; the
// coordinator's TradeStateReader adapter converts it to this
// enum, and MUST keep the ordinals aligned with Trading.ts —
// the numeric order is behavior (Trading.ts:229 compares
// `state >= TRADE_SCREEN`, and sendState derives the stage from
// `state >= CONFIRM_SCREEN`, Trading.ts:575).
type TradeState int

const (
	// TradeStateNone is TradeState.NONE: no trade in progress.
	TradeStateNone TradeState = iota
	// TradeStateRequested is TradeState.REQUESTED_TRADE: a
	// request has been sent (or received) but not yet
	// reciprocated.
	TradeStateRequested
	// TradeStateScreen is TradeState.TRADE_SCREEN: the offer
	// screen is open.
	TradeStateScreen
	// TradeStateAcceptedScreen is TradeState.ACCEPTED_TRADE_SCREEN:
	// the offer screen has been accepted, awaiting the partner.
	TradeStateAcceptedScreen
	// TradeStateConfirm is TradeState.CONFIRM_SCREEN: the
	// confirm screen is open.
	TradeStateConfirm
	// TradeStateAcceptedConfirm is
	// TradeState.ACCEPTED_CONFIRM_SCREEN: the confirm screen
	// has been accepted, awaiting the partner.
	TradeStateAcceptedConfirm
)

// TradeStateReader answers one player's current trading state:
// the seam Core.TradeState reads through. It reports false when
// the player has no trading state to read (a nil player, or a
// player outside the trading system) — the Go form of "no
// Trading object". The coordinator installs it over Worker B's
// trading state, converting ordinals (see TradeState).
type TradeStateReader func(player *game.Player) (TradeState, bool)

// SetTradeStateReader installs the TradeStateReader
// Core.TradeState reads through. The coordinator calls it once
// at boot, after Worker B's trading system exists; calling it
// with nil clears the reader. It is a no-op on a nil Hub or a
// Hub without a registry.
func (h *Hub) SetTradeStateReader(reader TradeStateReader) {
	if h == nil || h.hooks == nil {
		return
	}
	h.hooks.setTradeStateReader(reader)
}

// TradeStateReader returns the installed reader, or nil when
// none is installed (or the Hub is nil).
func (h *Hub) TradeStateReader() TradeStateReader {
	if h == nil || h.hooks == nil {
		return nil
	}
	return h.hooks.installedTradeStateReader()
}

func (r *hookRegistry) setTradeStateReader(reader TradeStateReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tradeStateReader = reader
}

func (r *hookRegistry) installedTradeStateReader() TradeStateReader {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tradeStateReader
}
