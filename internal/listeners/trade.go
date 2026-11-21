// TradeRequestPacketListener + the trade_action dispatch:
// player_option option 2 (trade) and TRADE_ACTION (opcode 180).
//
// requestTrade ports the option-2 branch of
// PlayerOptionPacketListener.executeClientOption
// (PlayerOptionPacketListener.ts:17-33) plus
// TradeRequestPacketListener.request (TradeRequestPacketListener.ts:6-35):
// the shared gates (option range, player hitpoints and busy),
// the target lookup and gates, the plane check, the distance
// and registration gates, the trade-request emit, and the
// walk-to-entity with the sendRequest arrival callback. The
// route intercept is NOT fired for option 2: TS calls routed()
// for options 1 and 4-8 only, never for 2-3. The plane check
// compares plain plane equality: TS compares BoatManager root
// locations, and no boats exist in Go, so the root location is
// the location itself. Clearing following degenerates: no
// following system exists.
//
// sendTradeRequest ports TradeRequestPacketListener.sendRequest
// (lines 37-62): the busy gates with their messages, the
// local-players check, and requestTrade. busy() degenerates to
// the terms Go can observe — hitpoints, the derived trading
// status, and the Trading-local interface id (see tradeBusy).
//
// routeTradeAction ports NetworkBuilder.tradeAction
// (NetworkBuilder.ts:1868-1883): accept/confirm-accept,
// decline/confirm-decline, and offer/remove clicks routed to
// the Trading methods. trade_action is NOT in
// INTERACTION_CLICKS, so no click-run override is set. A nil
// player drops with ReasonNoPlayer: dispatch guards the case
// on `if (this.player)`. Anything else counts as acted:
// Trading absorbs invalid states silently, as in TS.

package listeners

import (
	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// tradeBusy is the Player.busy() degeneration for trade
// (Player.ts:831-848): hitpoints at or below 0, a non-NONE
// status, or an open interface. The status term reads the
// Trading-derived status (TRADING while a trade screen is
// open; no other status is producible in Go), and the
// interface term reads the Trading-local interface id (the
// trade window only — Go tracks no other open interface, so
// a bank, shop, or dialogue window does not make a player
// busy here). needsPlacement, teleporting, and force
// movement have no Go state and are not consulted, as in the
// NPC/object listener degenerations; the dialogue-open term
// is unported. A nil player reads busy.
func tradeBusy(p *game.Player) bool {
	if p == nil {
		return true
	}
	if hitpoints(p) <= 0 {
		return true
	}
	if p.Trading().Status() != game.PlayerStatusNone {
		return true
	}
	return p.Trading().InterfaceID() > 0
}

// isLocalPlayer reports whether target is in the player's
// local-players list (TradeRequestPacketListener.ts:58:
// getLocalPlayers().indexOf(target) !== -1).
func isLocalPlayer(p, target *game.Player) bool {
	for _, local := range p.LocalPlayers() {
		if local == target {
			return true
		}
	}
	return false
}

// tradeRequestResult is requestTrade's outcome: the walk or
// request started, the packet was refused silently, or the
// player lookup is unwired.
type tradeRequestResult int

const (
	tradeRequestStarted tradeRequestResult = iota
	tradeRequestRefused
	tradeRequestUnwired
)

// requestTrade routes the option-2 branch: the shared
// executeClientOption gates, the plane check, and the
// TradeRequestPacketListener.request flow.
func requestTrade(p *game.Player, msg protocol.PlayerOption, deps Deps) tradeRequestResult {
	if !playerReady(p) {
		return tradeRequestRefused
	}
	// Shared gates (PlayerOptionPacketListener.ts:18-19):
	// the option range (decoder-guaranteed; the check guards
	// hand-built messages), hitpoints above 0, and not busy.
	if msg.Option < 1 || msg.Option > 8 || hitpoints(p) <= 0 || tradeBusy(p) {
		return tradeRequestRefused
	}

	// Lookup (line 20): the index range and slot checks
	// collapse into the lookup miss. An unwired lookup drops
	// named: the target cannot be resolved at all.
	if deps.Players == nil {
		return tradeRequestUnwired
	}
	target := deps.Players.Find(msg.Index)

	// Target gates (line 21): the target must exist, must
	// not be the player themself, and must have hitpoints
	// above 0. A target without a SkillManager (a hand-built
	// fixture, never a world player) cannot be verified alive
	// and refuses.
	if target == nil || target == p || target.Skills() == nil ||
		target.Skills().CurrentLevel(game.SkillHitpoints) <= 0 {
		return tradeRequestRefused
	}

	// Plane check (line 30, options 2-8): plain plane
	// equality, as the BoatManager root location of a player
	// with no boat is the player's own location.
	if target.Location().Z != p.Location().Z {
		return tradeRequestRefused
	}

	// Clear following (line 32): degenerate, no following
	// system exists.
	return tradeRequestTo(p, target)
}

// tradeRequestTo ports TradeRequestPacketListener.request
// (TradeRequestPacketListener.ts:6-35): the distance and
// registration gates, the trade-request emit, and the walk
// to the target. The index-range and target-exists gates ran
// in requestTrade's lookup.
func tradeRequestTo(p, target *game.Player) tradeRequestResult {
	// Distance (lines 14-16): within 20 tiles, Chebyshev on
	// the same plane (Location.isWithinDistance).
	if !p.Location().IsWithinDistance(target.Location(), 20) {
		return tradeRequestRefused
	}
	// Registration and hitpoints (lines 18-25).
	if !p.IsRegistered() || !target.IsRegistered() ||
		hitpoints(p) <= 0 || hitpoints(target) <= 0 {
		return tradeRequestRefused
	}

	// The trade-request emit (lines 27-29): a handled event
	// replaces the request, and the listener returns
	// silently. Either the seam's return value or the
	// event's Handled flag claims it, matching the
	// player-option route dual-claim pattern. A nil seam
	// means no hooks: the request proceeds.
	if game.TradeHookSeam.EmitTradeRequest != nil {
		event := &game.TradeRequestEvent{Player: p, Target: target}
		if game.TradeHookSeam.EmitTradeRequest(event) || event.Handled {
			return tradeRequestRefused
		}
	}

	// Walk to the target, then deliver the request
	// (lines 31-35). A player without a movement queue (a
	// hand-built fixture, never a world player) refuses.
	q := p.MovementQueue()
	if q == nil {
		return tradeRequestRefused
	}
	q.WalkToEntity(game.PlayerTarget(target), func() { sendTradeRequest(p, target) })
	return tradeRequestStarted
}

// sendTradeRequest ports TradeRequestPacketListener.sendRequest
// (TradeRequestPacketListener.ts:37-62): the busy gates with
// their messages, the local-players check, and requestTrade.
// It runs as the walk-to-entity arrival callback, on the tick
// goroutine.
func sendTradeRequest(p, target *game.Player) {
	if tradeBusy(p) {
		p.SendMessage("You cannot do that right now.")
		return
	}
	if tradeBusy(target) {
		msg := "That player is currently busy."
		if target.Trading().Status() == game.PlayerStatusTrading {
			msg = "That player is currently trading with someone else."
		}
		p.SendMessage(msg)
		return
	}
	if isLocalPlayer(p, target) {
		p.Trading().RequestTrade(target)
	}
}

// routeTradeAction routes one decoded TRADE_ACTION message
// (NetworkBuilder.tradeAction, NetworkBuilder.ts:1868-1883).
// A nil player drops with ReasonNoPlayer; an unready player
// (a hand-built fixture, never a world player) returns
// noAction; anything else counts as acted. The containers
// are the player's inventory and offer container, in the
// order the action dictates; an offer/remove click without
// an item id or with a non-positive quantity is absorbed
// silently, as in TS.
func routeTradeAction(p *game.Player, msg protocol.TradeAction, drop func(DropReason) Outcome, acted, noAction func() Outcome) Outcome {
	if p == nil {
		return drop(ReasonNoPlayer)
	}
	if !playerReady(p) {
		return noAction()
	}
	trading := p.Trading()
	switch msg.Action {
	case protocol.TradeActionAccept, protocol.TradeActionConfirmAccept:
		trading.AcceptTrade()
	case protocol.TradeActionDecline, protocol.TradeActionConfirmDecline:
		trading.CloseTrade()
	case protocol.TradeActionOffer:
		if msg.HasItemID && msg.Quantity > 0 {
			trading.HandleItem(msg.ItemID, msg.Quantity, msg.Slot, p.Inventory(), trading.Container())
		}
	case protocol.TradeActionRemove:
		if msg.HasItemID && msg.Quantity > 0 {
			trading.HandleItem(msg.ItemID, msg.Quantity, msg.Slot, trading.Container(), p.Inventory())
		}
	}
	return acted()
}
