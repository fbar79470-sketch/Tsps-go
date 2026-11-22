package game

import "testing"

// Tests for the Player status field (status.go, phase 46): the
// field + Status() derivation, and its interaction with the
// trade-state derivation.

func TestPlayerStatusDefaults(t *testing.T) {
	p := testPlayerWithDefs(nil)
	if got := p.Status(); got != PlayerStatusNone {
		t.Fatalf("fresh Status() = %v, want NONE", got)
	}
	var nilPlayer *Player
	if got := nilPlayer.Status(); got != PlayerStatusNone {
		t.Fatalf("nil Status() = %v, want NONE", got)
	}
	nilPlayer.SetStatus(PlayerStatusBanking)
	nilPlayer.SetStatusNone()
}

func TestPlayerStatusField(t *testing.T) {
	p := testPlayerWithDefs(nil)
	p.SetStatus(PlayerStatusBanking)
	if got := p.Status(); got != PlayerStatusBanking {
		t.Fatalf("Status() after SetStatus(BANKING) = %v, want BANKING", got)
	}
	p.SetStatusNone()
	if got := p.Status(); got != PlayerStatusNone {
		t.Fatalf("Status() after SetStatusNone() = %v, want NONE", got)
	}
	// The field stores any status value; only BANKING has a
	// producer today (the bank open).
	p.SetStatus(PlayerStatusShopping)
	if got := p.Status(); got != PlayerStatusShopping {
		t.Fatalf("Status() after SetStatus(SHOPPING) = %v, want SHOPPING", got)
	}
}

func TestPlayerStatusTradingFallthrough(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	if got := a.Status(); got != PlayerStatusNone {
		t.Fatalf("fresh Status() = %v, want NONE", got)
	}
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	if got := a.Status(); got != PlayerStatusTrading {
		t.Fatalf("offer-screen Status() = %v, want TRADING", got)
	}
	if got := b.Status(); got != PlayerStatusTrading {
		t.Fatalf("offer-screen Status() = %v, want TRADING", got)
	}
	a.Trading().CloseTrade()
	if got := a.Status(); got != PlayerStatusNone {
		t.Fatalf("Status() after close = %v, want NONE", got)
	}
}

func TestPlayerStatusFieldPrecedenceOverTrade(t *testing.T) {
	// Documented edge (status.go): the field wins over the
	// trade derivation, so a trade opened while the field
	// still holds BANKING reads BANKING until the field
	// clears. No trade decision reads Status() — the trade
	// gates read Trading().Status() directly — so this
	// precedence is observable only through Status() itself.
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	a.SetStatus(PlayerStatusBanking)
	if got := a.Status(); got != PlayerStatusBanking {
		t.Fatalf("trading + field BANKING Status() = %v, want BANKING", got)
	}
	if got := a.Trading().Status(); got != PlayerStatusTrading {
		t.Fatalf("Trading().Status() = %v, want TRADING (unaffected by the field)", got)
	}
	a.SetStatusNone()
	if got := a.Status(); got != PlayerStatusTrading {
		t.Fatalf("Status() after clearing the field = %v, want TRADING", got)
	}
}
