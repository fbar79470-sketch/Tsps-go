// Entered-amount / entered-syntax prompt dispatch: the
// dialogue_amount and dialogue_input dispatch cases
// (NetworkBuilder.ts:954-975), the prompt sends
// (PacketSender.sendEnterAmountPrompt / sendEnterInputPrompt,
// PacketSender.ts:446-452), and the abort semantics
// (PacketSender.abortInputPrompt, PacketSender.ts:465-475).
//
// DISPATCH. dialogue_amount (RESUME_COUNTDIALOG, opcode 192)
// reads the armed EnteredAmountAction, runs Execute only when
// the amount passes the filter (positive, or 0 with
// AcceptsZero, or negative with AcceptsNegative), then clears
// the holder ONLY when the action did not replace itself: the
// re-arm rule compares the live holder with the action that
// ran by identity, so a handler that arms a replacement (a
// Bank PIN keypad opening another prompt) keeps its prompt
// armed, while a handler that re-arms the SAME action object
// still clears it. dialogue_input (RESUME_NAMEDIALOG /
// RESUME_STRINGDIALOG, opcodes 193/194) runs the armed
// EnteredSyntaxAction unconditionally and applies the same
// re-arm rule. A nil player takes no action: TS reads the
// action through `this.player?.` and the clear through
// `this.player?.`, so a playerless packet is a silent skip.
//
// SENDS. The prompt-open frames are client scripts, not
// widget scripts: sendClientScript (PacketSender.ts:1754-1757)
// sends encodeRunClientScript (opcode 170), so the amount
// prompt is EncodeRunClientScript(108, [title]) and the input
// prompt is EncodeRunClientScript(110, [title, 0]). The send
// side lives in the session package (Worker 1) and the arming
// call sites live in game content (ShopManager) and the
// coordinator's sender; this package owns only the dispatch
// and the abort helper. The coordinator wires the shop
// buy/sell-X path by arming the action and sending the frame
// in the shop sender (see the phase report for the exact
// lines): the ShopManager already calls
// ShopSender.SendEnterAmountPrompt with the buy/sell execute
// closures (ShopManager.ts:319,347), and the sender's current
// drop is the only unwired half.
//
// ABORT. abortInputPrompt ports PacketSender.abortInputPrompt
// (PacketSender.ts:465-475): when a prompt is armed, the
// trigger_ondialogabort frame goes out first (it closes the
// prompt without answering — meslayer_close would resume a
// count prompt with 0), then both holders clear and the
// amount action's Cancel runs (the syntax action has no
// Cancel). The frame send goes through the InterfaceSends
// seam (the session's AbortInputPrompt is the full form: the
// coordinator wires the seam over it); a nil seam falls back
// to the player half only (AbortEnteredActions: clear +
// Cancel, no frame). TS calls abort from three places:
// resetInterfaceState, inside closeInterruptibleInterfaces
// and sendInterfaceRemoval (Worker 1's session close path —
// the close flow aborts prompts by construction, not through
// this helper), Bank.abortingPrompt (the bank widget branch,
// a gap: no bank interface flows exist), and the
// interface_close handled/overlay branches (this package's
// routeInterfaceClose, which calls this helper).
//
// BANK / SKILL PROMPTS. Bank.promptAmount (Bank.ts:844-854)
// arms an amount action whose execute checks Bank.isOpen,
// records the custom quantity, and sends the requested-
// quantity varbit. It is NOT wired: no Go bank interface flow
// calls it (the bank widget branch is a gap and Go's Bank
// holds no open state, so the execute would no-op). The
// dispatch and arming mechanism now exist for a future bank
// phase. SkillManager's set-level prompt
// (SkillManager.ts:215-229) is content-adjacent (a staff
// ::setlevel-style flow gated on OWNER/DEVELOPER rights) and
// is left unwired: no core call site arms it.

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// InterfaceSends carries the session-side interface sends the
// dispatch needs, following the Deps seam pattern: every field
// is optional and degrades to a no-op (or the documented
// fallback) when unset. The coordinator wires these in
// cmd/server over the session's interface methods; this
// package never names the session type.
type InterfaceSends struct {
	// AbortInputPrompt drops an unanswered prompt: the
	// trigger frame, the holder clears, and the amount
	// action's Cancel (the session's AbortInputPrompt,
	// PacketSender.ts:465-475). A nil seam falls back to
	// the player half only (AbortEnteredActions) in
	// abortInputPrompt.
	AbortInputPrompt func(p *game.Player)
	// CloseInterface closes one interface by group id (the
	// session's CloseInterface, PacketSender.ts:996-1009).
	// A nil seam is a no-op.
	CloseInterface func(p *game.Player, groupID int)
	// CloseInterruptibleInterfaces closes every tracked
	// interface (the session's CloseInterruptibleInterfaces,
	// the sender form, PacketSender.ts:981-995). A nil seam
	// falls back to the Player form in
	// closeInterruptibleInterfacesSender.
	CloseInterruptibleInterfaces func(p *game.Player)
	// HasInterruptibleInterface reports whether a chatbox or
	// a closable sub-interface is mounted (the session's
	// HasInterruptibleInterface, PacketSender.ts:976-979).
	// A nil seam reads false.
	HasInterruptibleInterface func(p *game.Player) bool
}

// routeDialogueAmount routes one decoded dialogue_amount
// message: the dialogue_amount dispatch case
// (NetworkBuilder.ts:954-964). The amount filter, the
// execute, and the re-arm rule are exactly TS's; see the file
// header. An armed action counts as acted (Execute ran, or
// the filtered answer cleared the holder); no armed action
// is ReasonNoAction. A nil player drops with ReasonNoPlayer:
// TS reads everything through `this.player?.`, so a
// playerless packet is a silent skip, and the dialogue cases
// share the ReasonNoPlayer nil outcome. A nil Execute on an
// armed action is treated as absent (a defensive Go-only
// guard: TS requires execute, but a zero-valued struct has a
// nil func field, and the tick goroutine must not panic on
// it); the re-arm clear still runs.
func routeDialogueAmount(p *game.Player, msg protocol.DialogueAmount, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	action := p.GetEnteredAmountAction()
	if action == nil {
		return noAction()
	}
	amount := int(msg.Amount)
	if amount > 0 || (amount == 0 && action.AcceptsZero) || (amount < 0 && action.AcceptsNegative) {
		if action.Execute != nil {
			action.Execute(amount)
		}
	}
	// Re-arm rule: clear only if the action did not replace
	// itself. Identity comparison on the holder pointer: a
	// replacement object survives, the same object clears.
	if p.GetEnteredAmountAction() == action {
		p.SetEnteredAmountAction(nil)
	}
	return acted()
}

// routeDialogueInput routes one decoded dialogue_input
// message: the dialogue_input dispatch case
// (NetworkBuilder.ts:966-975). The armed syntax action runs
// unconditionally, and the re-arm rule clears the holder
// only when the action did not replace itself (see
// routeDialogueAmount). Outcomes and the nil-Execute guard
// match the amount route.
func routeDialogueInput(p *game.Player, msg protocol.DialogueInput, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	action := p.GetEnteredSyntaxAction()
	if action == nil {
		return noAction()
	}
	if action.Execute != nil {
		action.Execute(msg.Value)
	}
	if p.GetEnteredSyntaxAction() == action {
		p.SetEnteredSyntaxAction(nil)
	}
	return acted()
}

// abortInputPrompt ports PacketSender.abortInputPrompt
// (PacketSender.ts:465-475) for the interface_close branches:
// the trigger frame goes out first when a prompt is armed,
// then both holders clear and the amount action's Cancel
// runs. The session seam is the full form; a nil seam falls
// back to the player half (AbortEnteredActions), which
// clears and cancels without sending the frame. A nil player
// is a no-op.
func abortInputPrompt(p *game.Player, deps Deps) {
	if p == nil {
		return
	}
	if deps.InterfaceSends.AbortInputPrompt != nil {
		deps.InterfaceSends.AbortInputPrompt(p)
		return
	}
	p.AbortEnteredActions()
}
