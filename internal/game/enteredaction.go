package game

// Entered-action holders: the Go forms of EnteredAmountAction
// (EnteredAmountAction.ts) and EnteredSyntaxAction
// (EnteredSyntaxAction.ts), held on the player
// (Player.ts:207-208) while a prompt waits for its answer.
//
// The prompt lifecycle is split across the layers, as in TS:
//   - Content arms an action with SetEnteredAmountAction /
//     SetEnteredSyntaxAction and opens the prompt through the
//     sender (PacketSender.sendEnterAmountPrompt /
//     sendEnterInputPrompt, PacketSender.ts:446-451: client
//     scripts 108/110). The sender methods are Worker 2's
//     dispatch work; the holders here are the shared contract.
//   - The turn-packet dispatch (NetworkBuilder.ts:954-975,
//     Worker 2) reads the armed action, applies the
//     acceptsZero / acceptsNegative gates for amounts, runs
//     Execute, and clears the holder only when the handler did
//     not re-arm a replacement.
//   - Closing interfaces aborts an unanswered prompt:
//     AbortEnteredActions is the player half of
//     PacketSender.abortInputPrompt (PacketSender.ts:465-475);
//     the session half sends the trigger_ondialogabort frame
//     first (see internal/session AbortInputPrompt).
//
// The holders are structs with func fields: content constructs
// them literally, and a nil *EnteredAmountAction /
// *EnteredSyntaxAction on the player is TS's undefined/null
// (no prompt armed). Execute is required; Cancel is optional
// (nil runs nothing). Nothing here persists: the holders are
// session-only, like the interface id (PlayerSave.ts has no
// prompt fields; verified for phase 18).

// EnteredAmountAction is EnteredAmountAction
// (EnteredAmountAction.ts): the action waiting on a numeric
// "enter amount" prompt answer.
//
// AcceptsZero / AcceptsNegative gate a 0 / negative answer:
// the dispatch drops such an answer unless the matching flag
// is set (NetworkBuilder.ts:954-959). Execute runs with the
// entered amount. Cancel, when non-nil, runs when the prompt
// is dropped unanswered (PacketSender.abortInputPrompt).
type EnteredAmountAction struct {
	AcceptsZero     bool
	AcceptsNegative bool
	Execute         func(amount int)
	Cancel          func()
}

// EnteredSyntaxAction is EnteredSyntaxAction
// (EnteredSyntaxAction.ts): the action waiting on a free-text
// prompt answer. It carries only Execute: every entered string
// is accepted (NetworkBuilder.ts:967-968 runs it
// unconditionally when an action is armed).
type EnteredSyntaxAction struct {
	Execute func(syntax string)
}

// GetEnteredAmountAction returns the armed amount action, or
// nil when none is armed (Player.ts:1626-1628).
func (p *Player) GetEnteredAmountAction() *EnteredAmountAction {
	if p == nil {
		return nil
	}
	p.interfaceMu.Lock()
	defer p.interfaceMu.Unlock()
	return p.enteredAmountAction
}

// SetEnteredAmountAction arms action, or clears the holder when
// action is nil (Player.ts:1630-1632; TS clears with
// setEnteredAmountAction(null)).
func (p *Player) SetEnteredAmountAction(action *EnteredAmountAction) {
	if p == nil {
		return
	}
	p.interfaceMu.Lock()
	defer p.interfaceMu.Unlock()
	p.enteredAmountAction = action
}

// GetEnteredSyntaxAction returns the armed syntax action, or
// nil when none is armed (Player.ts:1634-1636).
func (p *Player) GetEnteredSyntaxAction() *EnteredSyntaxAction {
	if p == nil {
		return nil
	}
	p.interfaceMu.Lock()
	defer p.interfaceMu.Unlock()
	return p.enteredSyntaxAction
}

// SetEnteredSyntaxAction arms action, or clears the holder when
// action is nil (Player.ts:1638-1640; TS clears with
// setEnteredSyntaxAction(null)).
func (p *Player) SetEnteredSyntaxAction(action *EnteredSyntaxAction) {
	if p == nil {
		return
	}
	p.interfaceMu.Lock()
	defer p.interfaceMu.Unlock()
	p.enteredSyntaxAction = action
}

// HasInputPrompt reports whether an amount or text action is
// armed (PacketSender.hasInputPrompt, PacketSender.ts:455-457,
// which reads the player's two getters). The session's
// AbortInputPrompt and the Player.closeInterruptibleInterfaces
// gate read it through the wiring seam; game code calls this
// directly.
func (p *Player) HasInputPrompt() bool {
	return p.GetEnteredAmountAction() != nil || p.GetEnteredSyntaxAction() != nil
}

// AbortEnteredActions is the player half of
// PacketSender.abortInputPrompt (PacketSender.ts:465-475): it
// captures the armed amount action, clears both holders, then
// runs the amount action's Cancel, if it has one. The session
// half sends the trigger_ondialogabort frame before calling
// this (only when HasInputPrompt holds). Cancel runs without
// holding interfaceMu: it may re-arm a prompt, and TS runs it
// after the clears too.
func (p *Player) AbortEnteredActions() {
	if p == nil {
		return
	}
	p.interfaceMu.Lock()
	amountAction := p.enteredAmountAction
	p.enteredAmountAction = nil
	p.enteredSyntaxAction = nil
	p.interfaceMu.Unlock()
	if amountAction != nil && amountAction.Cancel != nil {
		amountAction.Cancel()
	}
}
