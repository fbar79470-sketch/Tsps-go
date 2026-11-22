package game

// Player status: the PlayerStatus field and its derivation
// (phase 46, Worker A). TS keeps one PlayerStatus field on
// Player (Player.ts:155, getStatus/setStatus at 1415-1422)
// that Trading, Bank, ShopManager, and the Duel Arena
// content all write. Go derives TRADING from the trade state
// (Trading.Status, see trading.go) and keeps the field for
// the statuses no state machine derives — BANKING today (set
// by the bank open, cleared by the interface reset),
// SHOPPING/DUELING/PRICE_CHECKING when those systems land.
//
// Status() reads the field first and falls through to the
// trade derivation when the field is NONE, so NONE/TRADING
// behaviour is unchanged: a player with no status set still
// derives TRADING exactly when a trade screen is open, and
// NONE otherwise. The precedence differs from TS
// last-writer-wins in one edge: a trade opened while the
// field still holds BANKING reads BANKING from Status()
// until the field clears (Trading never writes the field).
// No trade decision depends on it — the trade gates read
// Trading().Status() directly — and the bank open/close
// paths that own the field keep it in step with the
// interface.
//
// The bank UI state that reads this field (current tab,
// searching flag/syntax, the preference readers) lives in
// bankstate.go; the field itself lives on Player so no
// second status store can drift from it.

// Status returns the player's modal-activity status
// (Player.getStatus, Player.ts:1415-1417): the status field
// when set, else the trade derivation (TRADING while a trade
// screen is open, NONE otherwise). A nil player reads NONE.
func (p *Player) Status() PlayerStatus {
	if p == nil {
		return PlayerStatusNone
	}
	p.interfaceMu.Lock()
	field := p.status
	p.interfaceMu.Unlock()
	if field != PlayerStatusNone {
		return field
	}
	if t := p.Trading(); t != nil {
		return t.Status()
	}
	return PlayerStatusNone
}

// SetStatus writes the status field (Player.setStatus,
// Player.ts:1419-1422). Bank.open sets BANKING; the interface
// reset clears it through SetStatusNone. Trading never writes
// the field: TRADING stays derived from the trade state (see
// Status). A nil player is a no-op.
func (p *Player) SetStatus(status PlayerStatus) {
	if p == nil {
		return
	}
	p.interfaceMu.Lock()
	p.status = status
	p.interfaceMu.Unlock()
}

// SetStatusNone resets the status field to NONE: the
// dep-shaped form of Player.setStatus(NONE), for the
// coordinator to wire into the session InterfaceDeps
// .SetStatusNone slot (resetInterfaceState,
// PacketSender.ts:887). A nil player is a no-op.
func (p *Player) SetStatusNone() {
	p.SetStatus(PlayerStatusNone)
}
