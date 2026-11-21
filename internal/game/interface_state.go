package game

// Interface-id state and the player-side close gate: the port of
// Player.interfaceId (Player.ts:156), get/setInterfaceId
// (Player.ts:1136-1151), and closeInterruptibleInterfaces
// (Player.ts:812-822). The sender-side machinery (sub-interface
// tracking, chatbox mounts, the close itself) lives in
// internal/session; this file holds only what Player itself
// owns, and reaches the sender through the InterfaceRuntimeSender
// optional SessionPort extension (the TradeSender /
// DialogueSender pattern), never by importing the session
// package.
//
// SESSION-ONLY STATE (verified against TS at commit bfd81e3:
// PlayerSave.ts and PlayerPersistence.ts contain no
// interfaceId, entered-action, or varp fields): the interface
// id, the entered-action holders (enteredaction.go), and the
// sender's varp store all reset on logout. No owned save fields
// exist for them, and no save code reads them.

// InterfaceClosedEventName / InterfaceChangedEventName are the
// custom events setInterfaceId emits (Player.ts:1143,1147).
// InterfaceOpenedEventName is the counterpart the sender emits
// when a group mounts (PacketSender.emitInterfaceOpened,
// PacketSender.ts:1613-1618); the session layer builds that
// payload through the wiring seam, and the shape lives here so
// content handlers share one type.
const (
	InterfaceClosedEventName  = "interface:closed"
	InterfaceChangedEventName = "interface:changed"
	InterfaceOpenedEventName  = "interface:opened"
)

// InterfaceClosedEvent is the "interface:closed" payload
// ({ player, interfaceId }, Player.ts:1143): the main or
// chatbox interface, or a sub-interface closed by name, closed
// by any route.
type InterfaceClosedEvent struct {
	Player      *Player
	InterfaceID int
}

// InterfaceChangedEvent is the "interface:changed" payload
// ({ player, previousId, interfaceId }, Player.ts:1147).
type InterfaceChangedEvent struct {
	Player      *Player
	PreviousID  int
	InterfaceID int
}

// InterfaceOpenedEvent is the "interface:opened" payload
// ({ player, groupId, targetUid, type },
// PacketSender.ts:1617): a group was mounted, by whichever
// plugin.
type InterfaceOpenedEvent struct {
	Player    *Player
	GroupID   int
	TargetUID int
	Type      int
}

// GetInterfaceId returns the open main interface id, or -1
// when none is open (Player.ts:1136-1138).
func (p *Player) GetInterfaceId() int {
	if p == nil {
		return -1
	}
	p.interfaceMu.Lock()
	defer p.interfaceMu.Unlock()
	return p.interfaceId
}

// SetInterfaceId records interfaceId as the open main interface
// (Player.ts:1140-1151):
//   - replacing an open interface with a different open one
//     emits "interface:closed" for the outgoing id (both ids
//     >= 0 and different);
//   - any change emits "interface:changed" with the previous
//     and new ids.
//
// Setting the same id emits nothing, and closing (setting -1)
// emits only "interface:changed": the "interface:closed" for a
// close comes from the sender's close path, not from here.
// Emits go through the EmitCustomEvent seam (nil-safe, and run
// without holding interfaceMu: a hook may call back into this
// player). DEVIATION: TS returns `this` for chaining; Go
// setters in this package return nothing (see SetRunning), so
// this returns nothing either.
func (p *Player) SetInterfaceId(interfaceId int) {
	if p == nil {
		return
	}
	p.interfaceMu.Lock()
	previousId := p.interfaceId
	emitClosed := p.interfaceId >= 0 && interfaceId >= 0 && p.interfaceId != interfaceId
	emitChanged := previousId != interfaceId
	closedId := p.interfaceId
	p.interfaceId = interfaceId
	emit := p.EmitCustomEvent
	p.interfaceMu.Unlock()
	if emit == nil {
		return
	}
	if emitClosed {
		emit(InterfaceClosedEventName, InterfaceClosedEvent{Player: p, InterfaceID: closedId})
	}
	if emitChanged {
		emit(InterfaceChangedEventName, InterfaceChangedEvent{Player: p, PreviousID: previousId, InterfaceID: interfaceId})
	}
}

// InterfaceSender is the optional SessionPort extension through
// which Player.SendInterface/CloseInterface reach the sender's
// open/close machinery (PacketSender.sendInterface,
// PacketSender.ts:408-418, and closeInterface,
// PacketSender.ts:996-1009). The session package implements it;
// a session-less player (or a SessionPort that does not
// implement it) yields nil from interfaceSender and the call is
// a no-op. This is the content-facing open/close surface: TS
// content calls player.getPacketSender().sendInterface(id),
// and Go content calls player.SendInterface(id).
type InterfaceSender interface {
	// SendInterface opens a main interface (PacketSender.ts:408-418).
	SendInterface(id int)
	// CloseInterface closes one interface by group id
	// (PacketSender.ts:996-1009).
	CloseInterface(groupID int)
}

// interfaceSender returns the player's InterfaceSender, or nil
// when the player is nil or the session does not implement it
// (the tradeSender assertion pattern, trade_hooks.go).
func (p *Player) interfaceSender() InterfaceSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(InterfaceSender)
	return sender
}

// SendInterface opens a main interface for the player
// (PacketSender.sendInterface, PacketSender.ts:408-418): it
// sets the interface id and mounts the group. A nil player or
// a session without the sender extension drops the call.
func (p *Player) SendInterface(id int) {
	if sender := p.interfaceSender(); sender != nil {
		sender.SendInterface(id)
	}
}

// CloseInterface closes one interface by group id
// (PacketSender.closeInterface, PacketSender.ts:996-1009). A
// nil player or a session without the sender extension drops
// the call.
func (p *Player) CloseInterface(groupID int) {
	if sender := p.interfaceSender(); sender != nil {
		sender.CloseInterface(groupID)
	}
}

// InterfaceRuntimeSender is the optional SessionPort extension
// through which Player.CloseInterruptibleInterfaces reaches the
// sender's close machinery (PacketSender.closeInterruptibleInterfaces,
// hasInterruptibleInterface, hasInputPrompt). The session
// package implements it; a session-less player (or a SessionPort
// that does not implement it) yields nil from
// interfaceRuntimeSender and the close is a no-op, as a player
// with no client has nothing to close.
type InterfaceRuntimeSender interface {
	// CloseInterruptibleInterfaces closes the tracked
	// interfaces (PacketSender.ts:981-995).
	CloseInterruptibleInterfaces()
	// HasInterruptibleInterface reports whether a chatbox or a
	// closable sub-interface is mounted
	// (PacketSender.ts:976-979).
	HasInterruptibleInterface() bool
	// HasInputPrompt reports whether an amount or text action
	// is armed (PacketSender.ts:455-457).
	HasInputPrompt() bool
}

// interfaceRuntimeSender returns the player's
// InterfaceRuntimeSender, or nil when the player is nil or the
// session does not implement it (the tradeSender assertion
// pattern, trade_hooks.go).
func (p *Player) interfaceRuntimeSender() InterfaceRuntimeSender {
	if p == nil {
		return nil
	}
	sender, _ := p.Session.(InterfaceRuntimeSender)
	return sender
}

// CloseInterruptibleInterfaces closes the player's open
// interfaces unless a trade or duel screen owns them
// (Player.ts:812-822):
//
//	if status != TRADING && status != DUELING &&
//	    (status != NONE || interfaceId >= 0 || dialogue active ||
//	     sender has interruptible interface || sender has input prompt):
//	    sender.closeInterruptibleInterfaces()
//
// Go has no PlayerStatus field, so the status reads degenerate
// onto the state machines that set it in TS, in the same order:
//   - TRADING: the trade state reached the trade screen.
//     Trading.initiateTrade sets the status and the state
//     together (Trading.ts:197-198), so a state at or past
//     TradeStateTradeScreen is exactly TRADING; an outstanding
//     request (TradeStateRequestedTrade) is not.
//   - DUELING: the duel state is anything but DuelStateNone.
//     APPROXIMATION: no core TS code sets PlayerStatus.DUELING
//     (grep-verified at bfd81e3: only the gate reads it), so
//     the status is plugin-driven in TS; the duel state is the
//     closest player-readable Go state.
//   - status != NONE: trading or duelling by the reads above.
//     GAP: SHOPPING, BANKING, and PRICE_CHECKING have no
//     player-readable Go state (no status field; the shop
//     manager is world-level). In practice their screens set
//     the interface id (shop open sets 300, ShopManager.ts:550;
//     bank open sets 12, Bank.ts:1388), so the interfaceId
//     disjunct covers them once the open paths set it — the
//     shop/trade unification edits are the coordinator's
//     integration work, not this file's.
//
// A nil sender reads as "nothing interruptible open" for the
// sender disjuncts, and the close itself is skipped: with no
// session there is no client state to close.
func (p *Player) CloseInterruptibleInterfaces() {
	if p == nil {
		return
	}
	if p.isTradingStatus() || p.isDuelingStatus() {
		return
	}
	sender := p.interfaceRuntimeSender()
	// The gate above already excluded the trading/duelling
	// statuses, so the TS "status != NONE" disjunct (which here
	// would only re-read them) contributes nothing further; the
	// remaining disjuncts are the interface id, the dialogue
	// manager, and the sender's state.
	hasSomethingOpen := p.GetInterfaceId() >= 0 || p.dialogueIsActive()
	if !hasSomethingOpen && sender != nil {
		hasSomethingOpen = sender.HasInterruptibleInterface() || sender.HasInputPrompt()
	}
	if !hasSomethingOpen {
		return
	}
	if sender != nil {
		sender.CloseInterruptibleInterfaces()
	}
}

// isTradingStatus reports whether the player holds the TRADING
// status, via the trade state (see CloseInterruptibleInterfaces).
func (p *Player) isTradingStatus() bool {
	trading := p.Trading()
	return trading != nil && trading.State() >= TradeStateTradeScreen
}

// isDuelingStatus reports whether the player holds the DUELING
// status, via the duel state (see CloseInterruptibleInterfaces).
func (p *Player) isDuelingStatus() bool {
	dueling := p.Dueling()
	return dueling != nil && dueling.State() != DuelStateNone
}

// dialogueIsActive reports whether the dialogue manager holds a
// chain. A zero-value Player has no manager; NewPlayer always
// installs one.
func (p *Player) dialogueIsActive() bool {
	return p.dialogueManager != nil && p.dialogueManager.IsActive()
}
