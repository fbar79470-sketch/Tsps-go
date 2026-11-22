package game

import "testing"

// Tests for the bank UI state block (bankstate.go, phase 46):
// BankIsOpen / BankClearStatus after the Worker A merge (the
// open state is the Player status field, status.go), plus the
// current-tab, search, and preference readers Worker B landed
// untested. The tab containers themselves are covered by
// bank_test.go; this file covers only the state block.

func TestBankIsOpen(t *testing.T) {
	if BankIsOpen(nil) {
		t.Fatal("nil player reads bank open")
	}
	p := testPlayerWithDefs(nil)
	if BankIsOpen(p) {
		t.Fatal("fresh player reads bank open")
	}
	// BANKING without the interface is not open.
	p.SetStatus(PlayerStatusBanking)
	if BankIsOpen(p) {
		t.Fatal("BANKING with no interface reads bank open")
	}
	// Interface 12 without BANKING is not open.
	p.SetStatusNone()
	p.SetInterfaceId(BankMainInterfaceID)
	if BankIsOpen(p) {
		t.Fatal("interface 12 without BANKING reads bank open")
	}
	// BANKING + interface 12 is open.
	p.SetStatus(PlayerStatusBanking)
	if !BankIsOpen(p) {
		t.Fatal("BANKING + interface 12 does not read bank open")
	}
	// Another interface id closes it.
	p.SetInterfaceId(300)
	if BankIsOpen(p) {
		t.Fatal("BANKING + interface 300 reads bank open")
	}
	// TRADING + interface 12 is not a bank.
	p.SetStatusNone()
	p.Trading().SetState(TradeStateTradeScreen)
	p.SetInterfaceId(BankMainInterfaceID)
	if BankIsOpen(p) {
		t.Fatal("TRADING + interface 12 reads bank open")
	}
}

func TestBankClearStatus(t *testing.T) {
	p := testPlayerWithDefs(nil)
	p.SetStatus(PlayerStatusBanking)
	p.SetInterfaceId(BankMainInterfaceID)
	if !BankIsOpen(p) {
		t.Fatal("setup: bank does not read open")
	}
	BankClearStatus(p)
	if got := p.Status(); got != PlayerStatusNone {
		t.Fatalf("Status() after BankClearStatus = %v, want NONE", got)
	}
	// The interface id is the session reset's job, not this
	// function's: it is untouched here.
	if got := p.GetInterfaceId(); got != BankMainInterfaceID {
		t.Fatalf("interface id after BankClearStatus = %d, want 12", got)
	}
	if BankIsOpen(p) {
		t.Fatal("bank still reads open after BankClearStatus")
	}
	BankClearStatus(nil)
}

func TestBankCurrentTab(t *testing.T) {
	p := testPlayerWithDefs(nil)
	if got := BankCurrentTab(p); got != 0 {
		t.Fatalf("default tab = %d, want 0", got)
	}
	BankSetCurrentTab(p, 5)
	if got := BankCurrentTab(p); got != 5 {
		t.Fatalf("tab after set(5) = %d, want 5", got)
	}
	BankSetCurrentTab(p, BankSearchTabIndex)
	if got := BankCurrentTab(p); got != BankSearchTabIndex {
		t.Fatalf("tab after set(10) = %d, want 10", got)
	}
	// Out-of-range tabs read as 0 (Player.ts:1424-1438).
	BankSetCurrentTab(p, -1)
	if got := BankCurrentTab(p); got != 0 {
		t.Fatalf("tab after set(-1) = %d, want 0", got)
	}
	BankSetCurrentTab(p, TotalBankTabs)
	if got := BankCurrentTab(p); got != 0 {
		t.Fatalf("tab after set(11) = %d, want 0", got)
	}
	if got := BankCurrentTab(nil); got != 0 {
		t.Fatalf("nil tab = %d, want 0", got)
	}
	BankSetCurrentTab(nil, 3)
}

func TestBankSearchState(t *testing.T) {
	p := testPlayerWithDefs(nil)
	if BankSearching(p) {
		t.Fatal("fresh player reads searching")
	}
	if got := BankSearchSyntax(p); got != "" {
		t.Fatalf("fresh syntax = %q, want empty", got)
	}
	BankSetSearching(p, true)
	BankSetSearchSyntax(p, "rune")
	if !BankSearching(p) {
		t.Fatal("search flag not set")
	}
	if got := BankSearchSyntax(p); got != "rune" {
		t.Fatalf("syntax = %q, want rune", got)
	}
	// EndBankSearch clears the flag/syntax and empties the
	// search tab, for any closing interface id (the session
	// passes the outgoing id through; closeSearch runs
	// regardless — see bankstate.go).
	search := p.Bank(BankSearchTabIndex)
	search.SetItem(0, NewItem(995, 1))
	BankEndSearch(p, BankMainInterfaceID)
	if BankSearching(p) {
		t.Fatal("search flag survives BankEndSearch")
	}
	if got := BankSearchSyntax(p); got != "" {
		t.Fatalf("syntax after BankEndSearch = %q, want empty", got)
	}
	if len(search.GetValidItems()) != 0 {
		t.Fatal("search tab not emptied by BankEndSearch")
	}
	// A non-bank closing id clears it too.
	BankSetSearching(p, true)
	BankSetSearchSyntax(p, "sword")
	BankEndSearch(p, 300)
	if BankSearching(p) || BankSearchSyntax(p) != "" {
		t.Fatal("non-bank close did not end the search")
	}
	if BankSearching(nil) || BankSearchSyntax(nil) != "" {
		t.Fatal("nil player reads searching/syntax")
	}
	BankEndSearch(nil, BankMainInterfaceID)
}

func TestBankPreferenceReaders(t *testing.T) {
	p := testPlayerWithDefs(nil)
	// Defaults (PlayerSave.ts:793-797): false, false, 0, 0,
	// false.
	if BankWithdrawAsNote(p) || BankInsertMode(p) || BankPlaceholders(p) {
		t.Fatal("fresh booleans are not all false")
	}
	if BankQuantityMode(p) != 0 || BankCustomQuantity(p) != 0 {
		t.Fatal("fresh numbers are not all 0")
	}
	// Strict === true for the booleans: only a stored bool
	// true counts.
	p.SetAttribute(BankNoteWithdrawalAttribute, true)
	p.SetAttribute(BankInsertModeAttribute, 1)
	p.SetAttribute(BankPlaceholdersAttribute, "true")
	if !BankWithdrawAsNote(p) {
		t.Fatal("note-withdrawal true not read")
	}
	if BankInsertMode(p) {
		t.Fatal("insert-mode 1 reads true (must be === true)")
	}
	if BankPlaceholders(p) {
		t.Fatal("placeholders string reads true (must be === true)")
	}
	// Numbers: Number(... ?? 0), truncated, with the mode
	// clamped to 0..4 and the custom quantity floored at 0.
	p.SetAttribute(BankQuantityModeAttribute, 9)
	if got := BankQuantityMode(p); got != 4 {
		t.Fatalf("quantity mode 9 = %d, want 4 (clamped)", got)
	}
	p.SetAttribute(BankQuantityModeAttribute, -3)
	if got := BankQuantityMode(p); got != 0 {
		t.Fatalf("quantity mode -3 = %d, want 0 (floored)", got)
	}
	p.SetAttribute(BankQuantityModeAttribute, 2.0)
	if got := BankQuantityMode(p); got != 2 {
		t.Fatalf("quantity mode 2.0 (JSON form) = %d, want 2", got)
	}
	p.SetAttribute(BankCustomQuantityAttribute, 500)
	if got := BankCustomQuantity(p); got != 500 {
		t.Fatalf("custom quantity 500 = %d, want 500", got)
	}
	p.SetAttribute(BankCustomQuantityAttribute, -5)
	if got := BankCustomQuantity(p); got != 0 {
		t.Fatalf("custom quantity -5 = %d, want 0 (floored)", got)
	}
	p.SetAttribute(BankCustomQuantityAttribute, "10")
	if got := BankCustomQuantity(p); got != 0 {
		t.Fatalf("custom quantity string = %d, want 0", got)
	}
	// Nil-player reads are the defaults.
	if BankWithdrawAsNote(nil) || BankInsertMode(nil) || BankPlaceholders(nil) ||
		BankQuantityMode(nil) != 0 || BankCustomQuantity(nil) != 0 {
		t.Fatal("nil player does not read defaults")
	}
}

func TestBankPreferenceSetters(t *testing.T) {
	p := testPlayerWithDefs(nil)
	BankSetNoteWithdrawal(p, true)
	BankSetInsertMode(p, true)
	BankSetPlaceholders(p, true)
	BankSetQuantityMode(p, 3)
	BankSetCustomQuantity(p, 42)
	if !BankWithdrawAsNote(p) || !BankInsertMode(p) || !BankPlaceholders(p) {
		t.Fatal("setters did not stick for the booleans")
	}
	if BankQuantityMode(p) != 3 || BankCustomQuantity(p) != 42 {
		t.Fatal("setters did not stick for the numbers")
	}
	BankSetInsertMode(p, false)
	if BankInsertMode(p) {
		t.Fatal("insert mode still true after set(false)")
	}
	// The setters clamp like the readers: mode 9 -> 4,
	// custom -1 -> 0.
	BankSetQuantityMode(p, 9)
	BankSetCustomQuantity(p, -1)
	if BankQuantityMode(p) != 4 || BankCustomQuantity(p) != 0 {
		t.Fatal("setter clamps not applied")
	}
	BankSetNoteWithdrawal(nil, true)
	BankSetInsertMode(nil, true)
	BankSetPlaceholders(nil, true)
	BankSetQuantityMode(nil, 1)
	BankSetCustomQuantity(nil, 1)
}
