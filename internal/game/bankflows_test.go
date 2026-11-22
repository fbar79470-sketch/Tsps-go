package game

import (
	"testing"
)

// bankTestDefs extends testItemDefs with the name and note-id
// capabilities the bank flows read (ItemNameSource,
// bankNoteIDSource): names for search/examine matching, note
// ids for note withdrawal.
type bankTestDefs struct {
	*testItemDefs
	names   map[int]string
	noteIDs map[int]int
}

func (s *bankTestDefs) ItemName(id int) string { return s.names[id] }
func (s *bankTestDefs) NoteID(id int) int {
	if n, ok := s.noteIDs[id]; ok {
		return n
	}
	return -1
}

// bankFakeSender records the BankSender calls the bank flows
// make, so the open sequence and prompt frames are asserted
// without a session.
type bankFakeSender struct {
	opens         []BankOpenView
	amountPrompts []string
	inputPrompts  []string
}

func (s *bankFakeSender) SendBankOpen(p *Player, view BankOpenView) {
	s.opens = append(s.opens, view)
}
func (s *bankFakeSender) SendAmountPrompt(p *Player, prompt string) {
	s.amountPrompts = append(s.amountPrompts, prompt)
}
func (s *bankFakeSender) SendInputPrompt(p *Player, prompt string) {
	s.inputPrompts = append(s.inputPrompts, prompt)
}

// bankOpenPlayer opens the bank on p with a recording sender
// and returns it. The sender is uninstalled by the caller via
// the returned cleanup.
func bankOpenPlayer(t *testing.T, p *Player) *bankFakeSender {
	t.Helper()
	fake := &bankFakeSender{}
	SetBankSender(fake)
	t.Cleanup(func() { SetBankSender(nil) })
	BankOpen(p)
	if !BankIsOpen(p) {
		t.Fatal("BankOpen did not open the bank")
	}
	return fake
}

// bankEvents records custom events the bank flows emit.
type bankEvents struct {
	names    []string
	payloads []any
}

func (e *bankEvents) install(p *Player) {
	p.EmitCustomEvent = func(name string, payload any) {
		e.names = append(e.names, name)
		e.payloads = append(e.payloads, payload)
	}
}

func (e *bankEvents) has(name string) bool {
	for _, n := range e.names {
		if n == name {
			return true
		}
	}
	return false
}

func bankTestSetup() (*Player, *bankTestDefs) {
	src := &bankTestDefs{
		testItemDefs: &testItemDefs{
			stackable:   map[int]bool{995: true, 996: true},
			noted:       map[int]bool{996: true},
			unNote:      map[int]int{996: 995},
			placeholder: map[int]int{995: 995},
			slots:       map[int]int{4151: 3, 1323: 3},
		},
		names: map[int]string{
			995:  "Coins",
			996:  "Coins",
			4151: "Abyssal whip",
			1323: "Iron scimitar",
			2:    "Cannonball",
		},
		noteIDs: map[int]int{995: 996},
	}
	p := testPlayerWithDefs(src)
	return p, src
}

func TestBankOpenSequence(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 100))
	p.Bank(1).SetItem(0, NewItem(4151, 1))
	p.Inventory().SetItem(2, NewItem(4151, 1))
	BankSetNoteWithdrawal(p, true)
	events := &bankEvents{}
	events.install(p)
	fake := bankOpenPlayer(t, p)
	if p.Status() != PlayerStatusBanking {
		t.Fatalf("status = %v, want BANKING", p.Status())
	}
	if p.GetInterfaceId() != BankMainInterfaceID {
		t.Fatalf("interface = %d, want 12", p.GetInterfaceId())
	}
	if BankWithdrawAsNote(p) {
		t.Fatal("open did not force note withdrawal off")
	}
	if len(fake.opens) != 1 {
		t.Fatalf("open sequences = %d, want 1", len(fake.opens))
	}
	view := fake.opens[0]
	if !view.Opening {
		t.Fatal("first open must report Opening")
	}
	if view.Varps[548] != 1 || view.Varps[BankLockedSlotsVarp] != 0 {
		t.Fatalf("varps = %v, want 548=1, 4611=0", view.Varps)
	}
	if view.Varps[BankSideWearOpsVarp] != 1<<2 {
		t.Fatalf("side wear-ops mask = %d, want %d", view.Varps[BankSideWearOpsVarp], 1<<2)
	}
	if view.Varbits[4150] != 0 || view.Varbits[4170] != 0 || view.Varbits[5450] != 1 {
		t.Fatalf("varbits 4150/4170/5450 = %d/%d/%d", view.Varbits[4150], view.Varbits[4170], view.Varbits[5450])
	}
	if view.Varbits[4171] != 1 {
		t.Fatalf("varbit 4171 (tab 1 size) = %d, want 1", view.Varbits[4171])
	}
	if !events.has(BankOpenedEventName) {
		t.Fatal("bank:opened was not emitted")
	}
	// A re-open is not an opening: no second sound/event, and
	// the view reports Opening false.
	events.names = nil
	fake.opens = nil
	BankOpen(p)
	if len(fake.opens) != 1 || fake.opens[0].Opening {
		t.Fatalf("re-open = %+v, want one non-opening sequence", fake.opens)
	}
	if events.has(BankOpenedEventName) {
		t.Fatal("re-open emitted bank:opened")
	}
}

func TestBankActionAmount(t *testing.T) {
	cases := []struct {
		kind    BankAmountKind
		button  int
		option  string
		avail   int
		custom  int
		mode    int
		want    int
		comment string
	}{
		{BankAmountWithdraw, 1, "", 100, 0, 0, 1, "withdraw default mode 0 = 1"},
		{BankAmountWithdraw, 1, "", 100, 7, 1, 5, "withdraw default mode 1 = 5"},
		{BankAmountWithdraw, 1, "", 100, 7, 3, 7, "withdraw default mode 3 = custom"},
		{BankAmountWithdraw, 1, "", 100, 7, 4, 100, "withdraw default mode 4 = all"},
		{BankAmountWithdraw, 2, "", 100, 0, 0, 1, "withdraw op 2 = 1"},
		{BankAmountWithdraw, 3, "", 100, 0, 0, 5, "withdraw op 3 = 5"},
		{BankAmountWithdraw, 4, "", 100, 0, 0, 10, "withdraw op 4 = 10"},
		{BankAmountWithdraw, 5, "", 100, 7, 0, 7, "withdraw op 5 = custom"},
		{BankAmountWithdraw, 6, "", 100, 7, 0, 7, "withdraw op 6 = custom"},
		{BankAmountWithdraw, 7, "", 100, 0, 0, 100, "withdraw op 7 = all"},
		{BankAmountWithdraw, 8, "", 100, 0, 0, 99, "withdraw op 8 = all-but-1"},
		{BankAmountWithdraw, 8, "", 1, 0, 0, 0, "withdraw op 8 of 1 = 0"},
		{BankAmountWithdraw, 9, "", 100, 0, 0, 0, "withdraw op 9 = 0"},
		{BankAmountWithdraw, 1, "Withdraw-5", 100, 0, 0, 5, "option suffix -5 wins"},
		{BankAmountWithdraw, 1, "Withdraw-all-but-1", 100, 0, 0, 99, "option -all-but-1 wins over -1"},
		{BankAmountWithdraw, 1, "Withdraw-all", 3, 0, 0, 3, "option -all clamps to available"},
		{BankAmountWithdraw, 1, "Withdraw-x", 100, 7, 0, 7, "option -x = custom"},
		{BankAmountDeposit, 1, "", 100, 0, 0, 1, "deposit op 1 = 1"},
		{BankAmountDeposit, 2, "", 100, 7, 3, 7, "deposit default mode 3 = custom"},
		{BankAmountDeposit, 2, "", 100, 7, 4, 100, "deposit default mode 4 = all"},
		{BankAmountDeposit, 3, "", 100, 0, 0, 1, "deposit op 3 = 1"},
		{BankAmountDeposit, 4, "", 100, 0, 0, 5, "deposit op 4 = 5"},
		{BankAmountDeposit, 5, "", 100, 0, 0, 10, "deposit op 5 = 10"},
		{BankAmountDeposit, 6, "", 100, 7, 0, 7, "deposit op 6 = custom"},
		{BankAmountDeposit, 7, "", 100, 7, 0, 7, "deposit op 7 = custom"},
		{BankAmountDeposit, 8, "", 100, 0, 0, 100, "deposit op 8 = all"},
		{BankAmountDeposit, 1, "Deposit-10", 100, 0, 0, 10, "deposit option -10 wins"},
		{BankAmountWithdraw, 2, "", 3, 0, 0, 1, "withdraw clamps to available"},
		{BankAmountWithdraw, 7, "", 0, 0, 0, 0, "withdraw of nothing = 0"},
	}
	for _, c := range cases {
		if got := BankActionAmount(c.kind, c.button, c.option, c.avail, c.custom, c.mode); got != c.want {
			t.Errorf("%s: BankActionAmount(%v,%d,%q,%d,%d,%d) = %d, want %d",
				c.comment, c.kind, c.button, c.option, c.avail, c.custom, c.mode, got, c.want)
		}
	}
}

func TestBankAsksAmount(t *testing.T) {
	if !BankAsksAmount(BankAmountWithdraw, 6, "", 0, 0) {
		t.Error("withdraw op 6 must ask for an amount")
	}
	if !BankAsksAmount(BankAmountDeposit, 7, "", 0, 0) {
		t.Error("deposit op 7 must ask for an amount")
	}
	if BankAsksAmount(BankAmountWithdraw, 2, "", 0, 0) {
		t.Error("withdraw op 2 must not ask for an amount")
	}
	if !BankAsksAmount(BankAmountWithdraw, 1, "", 0, BankQuantityXMode) {
		t.Error("withdraw default click in X mode with no custom amount must ask")
	}
	if BankAsksAmount(BankAmountWithdraw, 1, "", 5, BankQuantityXMode) {
		t.Error("withdraw default click in X mode with a custom amount must not ask")
	}
	if !BankAsksAmount(BankAmountWithdraw, 1, "Withdraw-x", 5, 0) {
		t.Error("option -x must ask for an amount")
	}
	if !BankAsksAmount(BankAmountDeposit, 2, "", 0, BankQuantityXMode) {
		t.Error("deposit default click in X mode with no custom amount must ask")
	}
}

func TestBankWithdrawDepositRoundTrip(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 100))
	bankOpenPlayer(t, p)
	BankWithdraw(p, 995, 0, 5, 0)
	if got := p.Inventory().GetAmount(995); got != 5 {
		t.Fatalf("inventory after withdraw-5 = %d, want 5", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 95 {
		t.Fatalf("bank after withdraw-5 = %d, want 95", got)
	}
	BankDeposits(p, 995, 0, 5)
	if got := p.Inventory().GetAmount(995); got != 0 {
		t.Fatalf("inventory after deposit-5 = %d, want 0", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 100 {
		t.Fatalf("bank after deposit-5 = %d, want 100", got)
	}
	// Withdraw-all (amount -1).
	BankWithdraw(p, 995, 0, -1, 0)
	if got := p.Inventory().GetAmount(995); got != 100 {
		t.Fatalf("inventory after withdraw-all = %d, want 100", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 0 {
		t.Fatalf("bank after withdraw-all = %d, want 0", got)
	}
}

func TestBankWithdrawFullInventory(t *testing.T) {
	p, _ := bankTestSetup()
	// Non-stackable items: the bank holds 5 whips, the
	// inventory has 27 of 28 slots filled.
	p.Bank(0).SetItem(0, NewItem(4151, 5))
	for i := 0; i < 27; i++ {
		p.Inventory().SetItem(i, NewItem(2000+i, 1))
	}
	bankOpenPlayer(t, p)
	BankWithdraw(p, 4151, 0, 5, 0)
	// Shortened to the one free slot (TS: partial message,
	// dropped here with no session).
	if got := p.Inventory().GetAmount(4151); got != 1 {
		t.Fatalf("inventory whips after partial withdraw = %d, want 1", got)
	}
	if got := p.Bank(0).GetAmount(4151); got != 4 {
		t.Fatalf("bank whips after partial withdraw = %d, want 4", got)
	}
	// Now the inventory is full: a further withdraw refuses
	// outright (TS: "You don't have enough inventory space.",
	// spam, dropped here).
	BankWithdraw(p, 4151, 0, 1, 0)
	if got := p.Inventory().GetAmount(4151); got != 1 {
		t.Fatalf("inventory whips after full withdraw = %d, want 1", got)
	}
	if got := p.Bank(0).GetAmount(4151); got != 4 {
		t.Fatalf("bank whips after full withdraw = %d, want 4", got)
	}
}

func TestBankWithdrawMaxStackClamp(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 100))
	p.Inventory().SetItem(0, NewItem(995, MaxStackAmount-2))
	bankOpenPlayer(t, p)
	BankWithdraw(p, 995, 0, 5, 0)
	if got := p.Inventory().GetAmount(995); got != MaxStackAmount {
		t.Fatalf("inventory coins = %d, want max stack %d", got, MaxStackAmount)
	}
	if got := p.Bank(0).GetAmount(995); got != 98 {
		t.Fatalf("bank coins = %d, want 98", got)
	}
	// At the cap, no more can be withdrawn at all.
	BankWithdraw(p, 995, 0, 1, 0)
	if got := p.Bank(0).GetAmount(995); got != 98 {
		t.Fatalf("bank coins after capped withdraw = %d, want 98", got)
	}
}

func TestBankWithdrawAsNote(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 100))
	p.Bank(0).SetItem(1, NewItem(4151, 3))
	bankOpenPlayer(t, p)
	BankSetNoteWithdrawal(p, true)
	BankWithdraw(p, 995, 0, 5, 0)
	if got := p.Inventory().GetAmount(996); got != 5 {
		t.Fatalf("inventory notes = %d, want 5", got)
	}
	if got := p.Inventory().GetAmount(995); got != 0 {
		t.Fatalf("inventory unnoted coins = %d, want 0", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 95 {
		t.Fatalf("bank coins = %d, want 95", got)
	}
	// A whip has no note form: it withdraws unnoted (and TS
	// sends "This item cannot be withdrawn as a note.",
	// dropped here with no session).
	BankWithdraw(p, 4151, 1, 1, 0)
	if got := p.Inventory().GetAmount(4151); got != 1 {
		t.Fatalf("inventory whips = %d, want 1", got)
	}
}

func TestBankDepositUnnotes(t *testing.T) {
	p, _ := bankTestSetup()
	p.Inventory().SetItem(0, NewItem(996, 5))
	bankOpenPlayer(t, p)
	BankDeposits(p, 996, 0, 5)
	if got := p.Bank(0).GetAmount(995); got != 5 {
		t.Fatalf("bank coins after noted deposit = %d, want 5", got)
	}
	if got := p.Inventory().GetAmount(996); got != 0 {
		t.Fatalf("inventory notes after deposit = %d, want 0", got)
	}
}

func TestBankDepositFullBank(t *testing.T) {
	p, _ := bankTestSetup()
	full := make([]Item, MaxBankSlots)
	for i := range full {
		full[i] = NewItem(2000+i, 1)
	}
	p.Bank(0).SetItems(full)
	p.Inventory().SetItem(0, NewItem(995, 5))
	bankOpenPlayer(t, p)
	BankDeposits(p, 995, 0, 5)
	if got := p.Inventory().GetAmount(995); got != 5 {
		t.Fatalf("inventory coins after full-bank deposit = %d, want 5 (refused)", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 0 {
		t.Fatalf("bank coins after full-bank deposit = %d, want 0", got)
	}
}

func TestBankPlaceholders(t *testing.T) {
	p, _ := bankTestSetup()
	p.SetAttribute(BankPlaceholdersAttribute, true)
	p.Bank(0).SetItem(0, NewItem(995, 5))
	p.Bank(0).SetItem(1, NewItem(4151, 3))
	bankOpenPlayer(t, p)
	// Coins have a cache placeholder id: a full withdraw
	// leaves an amount-0 placeholder.
	BankWithdraw(p, 995, 0, -1, 0)
	if got := p.Bank(0).ItemAt(0); got.ID() != 995 || got.Amount() != 0 {
		t.Fatalf("coin slot after full withdraw = %+v, want id 995 amount 0", got)
	}
	// The whip has no placeholder id: a full withdraw clears
	// the slot.
	BankWithdraw(p, 4151, 1, -1, 0)
	if got := p.Bank(0).ItemAt(1); got.ID() != -1 {
		t.Fatalf("whip slot after full withdraw = %+v, want cleared", got)
	}
	// Release the coin placeholder.
	BankReleasePlaceholder(p, 0, 0)
	if got := p.Bank(0).ItemAt(0); got.ID() != -1 {
		t.Fatalf("coin slot after release = %+v, want cleared", got)
	}
	// Release-all clears placeholders across tabs.
	p.Bank(1).SetItem(0, NewItem(995, 0))
	BankReleasePlaceholders(p, 0)
	if got := p.Bank(1).ItemAt(0); got.ID() != -1 {
		t.Fatalf("tab 1 placeholder after release-all = %+v, want cleared", got)
	}
}

func TestBankDepositItems(t *testing.T) {
	p, _ := bankTestSetup()
	p.Inventory().SetItem(0, NewItem(995, 10))
	p.Inventory().SetItem(1, NewItem(4151, 1))
	p.Inventory().SetItem(2, NewItem(1323, 1))
	p.Equipment().SetItem(HeadSlot, NewItem(2, 1))
	bankOpenPlayer(t, p)
	BankDepositItems(p, p.Inventory(), false)
	if got := p.Inventory().GetAmount(995); got != 0 {
		t.Fatalf("inventory coins after deposit-all = %d, want 0", got)
	}
	if got := p.Bank(0).GetAmount(995); got != 10 {
		t.Fatalf("bank coins after deposit-all = %d, want 10", got)
	}
	if got := p.Bank(0).GetAmount(4151); got != 1 {
		t.Fatalf("bank whips after deposit-all = %d, want 1", got)
	}
	BankDepositItems(p, p.Equipment(), false)
	if got := p.Equipment().ItemAt(HeadSlot); got.ID() != -1 {
		t.Fatalf("head slot after deposit-worn = %+v, want cleared", got)
	}
	if got := p.Bank(0).GetAmount(2); got != 1 {
		t.Fatalf("bank cannonballs after deposit-worn = %d, want 1", got)
	}
	// Worn-panel Bank op deposits only that slot.
	p.Equipment().SetItem(WeaponSlot, NewItem(1323, 1))
	p.Equipment().SetItem(ShieldSlot, NewItem(2, 1))
	BankDepositItems(p, p.Equipment(), false, WeaponSlot)
	if got := p.Equipment().ItemAt(WeaponSlot); got.ID() != -1 {
		t.Fatalf("weapon slot after single deposit = %+v, want cleared", got)
	}
	if got := p.Equipment().ItemAt(ShieldSlot); got.ID() != 2 {
		t.Fatalf("shield slot after single deposit = %+v, want untouched", got)
	}
}

func TestBankTabs(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(1).SetItem(0, NewItem(4151, 1))
	p.Bank(2).SetItem(0, NewItem(1323, 1))
	bankOpenPlayer(t, p)
	if got := BankGetTabCount(p); got != 2 {
		t.Fatalf("tab count = %d, want 2", got)
	}
	BankViewTab(p, 1)
	if got := BankCurrentTab(p); got != 1 {
		t.Fatalf("current tab after viewTab(1) = %d, want 1", got)
	}
	// An empty tab past the count refuses with the new-tab
	// message (dropped here) and keeps the tab.
	BankViewTab(p, 5)
	if got := BankCurrentTab(p); got != 1 {
		t.Fatalf("current tab after viewTab(5) = %d, want 1", got)
	}
	// Collapse tab 1 into the main tab.
	BankCollapseTab(p, 1)
	if got := p.Bank(0).GetAmount(4151); got != 1 {
		t.Fatalf("main tab whips after collapse = %d, want 1", got)
	}
	if got := BankCurrentTab(p); got != 0 {
		t.Fatalf("current tab after collapse = %d, want 0", got)
	}
	if got := BankGetTabCount(p); got != 1 {
		t.Fatalf("tab count after collapse = %d, want 1 (tab 2 compacted to 1)", got)
	}
	// Reconfigure compacts an empty leading tab.
	p.Bank(1).ResetItems()
	p.Bank(2).SetItem(0, NewItem(1323, 1))
	if BankReconfigureTabs(p) {
		t.Fatal("reconfigure must not re-open when the viewed tab is fine")
	}
	if got := p.Bank(1).GetAmount(1323); got != 1 {
		t.Fatalf("tab 1 scimitars after reconfigure = %d, want 1 (compacted from tab 2)", got)
	}
	if got := BankGetTabCount(p); got != 1 {
		t.Fatalf("tab count after reconfigure = %d, want 1", got)
	}
	// getTabForItem finds the tab holding the item, and the
	// current tab for a new item with room.
	if got := BankGetTabForItem(p, 1323); got != 1 {
		t.Fatalf("tab for scimitar = %d, want 1", got)
	}
	if got := BankGetTabForItem(p, 995); got != 0 {
		t.Fatalf("tab for new coins = %d, want 0 (current tab)", got)
	}
}

func TestBankSearch(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 10))
	p.Bank(0).SetItem(1, NewItem(4151, 1))
	p.Bank(0).SetItem(2, NewItem(1323, 1))
	bankOpenPlayer(t, p)
	BankSearch(p, "whip")
	if !BankSearching(p) {
		t.Fatal("search did not set the searching flag")
	}
	search := p.Bank(BankSearchTabIndex)
	if got := search.GetAmount(4151); got != 1 {
		t.Fatalf("search tab whips = %d, want 1", got)
	}
	if got := search.GetAmount(995); got != 0 {
		t.Fatalf("search tab coins = %d, want 0", got)
	}
	if got := search.GetAmount(1323); got != 0 {
		t.Fatalf("search tab scimitars = %d, want 0", got)
	}
	// Withdraw from the search tab resolves the real tab.
	BankWithdraw(p, 4151, 0, 1, BankSearchTabIndex)
	if got := p.Inventory().GetAmount(4151); got != 1 {
		t.Fatalf("inventory whips after search withdraw = %d, want 1", got)
	}
	if got := p.Bank(0).GetAmount(4151); got != 0 {
		t.Fatalf("bank whips after search withdraw = %d, want 0", got)
	}
	if got := search.GetAmount(4151); got != 0 {
		t.Fatalf("search tab whips after withdraw = %d, want 0", got)
	}
	// Exit the search, re-opening the bank.
	BankExitSearch(p, true)
	if BankSearching(p) {
		t.Fatal("exitSearch did not clear the searching flag")
	}
	if got := search.GetAmount(995); got != 0 {
		t.Fatalf("search tab coins after exit = %d, want 0 (cleared)", got)
	}
	if !BankIsOpen(p) {
		t.Fatal("exitSearch(true) did not re-open the bank")
	}
}

func TestBankPromptAmount(t *testing.T) {
	p, _ := bankTestSetup()
	fake := bankOpenPlayer(t, p)
	called := -1
	BankPromptAmount(p, func(amount int) { called = amount })
	if len(fake.amountPrompts) != 1 || fake.amountPrompts[0] != "Enter amount:" {
		t.Fatalf("amount prompts = %v, want [Enter amount:]", fake.amountPrompts)
	}
	action := p.GetEnteredAmountAction()
	if action == nil {
		t.Fatal("prompt did not arm an amount action")
	}
	action.Execute(25)
	if got := BankCustomQuantity(p); got != 25 {
		t.Fatalf("custom quantity after prompt = %d, want 25", got)
	}
	if called != 25 {
		t.Fatalf("prompt callback got %d, want 25", called)
	}
}

func TestBankHandleButton(t *testing.T) {
	p, _ := bankTestSetup()
	p.Inventory().SetItem(0, NewItem(995, 10))
	p.Bank(1).SetItem(0, NewItem(4151, 1))
	bankOpenPlayer(t, p)
	if !BankHandleButton(p, 5386, 0) || !BankWithdrawAsNote(p) {
		t.Fatal("button 5386 did not turn note withdrawal on")
	}
	if !BankHandleButton(p, 5387, 0) || BankWithdrawAsNote(p) {
		t.Fatal("button 5387 did not turn note withdrawal off")
	}
	if !BankHandleButton(p, 8131, 0) || !BankInsertMode(p) {
		t.Fatal("button 8131 did not turn insert mode on")
	}
	if !BankHandleButton(p, 8130, 0) || BankInsertMode(p) {
		t.Fatal("button 8130 did not turn insert mode off")
	}
	// Deposit inventory.
	if !BankHandleButton(p, 50004, 0) {
		t.Fatal("button 50004 was not handled")
	}
	if got := p.Bank(0).GetAmount(995); got != 10 {
		t.Fatalf("bank coins after button 50004 = %d, want 10", got)
	}
	// Search button arms the syntax prompt.
	fake := &bankFakeSender{}
	SetBankSender(fake)
	t.Cleanup(func() { SetBankSender(nil) })
	if !BankHandleButton(p, 50010, 0) {
		t.Fatal("button 50010 was not handled")
	}
	if len(fake.inputPrompts) != 1 || fake.inputPrompts[0] != "What do you wish to search for?" {
		t.Fatalf("input prompts = %v", fake.inputPrompts)
	}
	action := p.GetEnteredSyntaxAction()
	if action == nil {
		t.Fatal("search button did not arm a syntax action")
	}
	p.Bank(0).SetItem(1, NewItem(4151, 1))
	action.Execute("whip")
	if !BankSearching(p) {
		t.Fatal("syntax answer did not start a search")
	}
	// Tab-select button 50070 + 4*1 selects tab 1 (after
	// exiting the search).
	if !BankHandleButton(p, 50070+4, 0) {
		t.Fatal("tab-select button was not handled")
	}
	if got := BankCurrentTab(p); got != 1 {
		t.Fatalf("current tab after tab-select = %d, want 1", got)
	}
	// Tab-select with action 1 collapses the tab. The main
	// tab already holds a whip, so the collapsed stack merges
	// with it (STACKS semantics).
	if !BankHandleButton(p, 50070+4, 1) {
		t.Fatal("tab-collapse button was not handled")
	}
	if got := p.Bank(0).GetAmount(4151); got != 2 {
		t.Fatalf("main tab whips after tab collapse = %d, want 2 (merged)", got)
	}
	// Settings interface: placeholder toggle + re-open.
	p.SetInterfaceId(BankSettingsInterfaceID)
	if !BankHandleButton(p, 32513, 0) || !BankPlaceholders(p) {
		t.Fatal("button 32513 did not toggle placeholders on")
	}
	if !BankHandleButton(p, 32512, 0) || !BankIsOpen(p) {
		t.Fatal("button 32512 did not re-open the bank")
	}
	// Close button.
	if !BankHandleButton(p, 50001, 0) {
		t.Fatal("button 50001 was not handled")
	}
	if p.Status() != PlayerStatusNone {
		t.Fatalf("status after close button = %v, want NONE", p.Status())
	}
	// A button with the bank closed is not owned.
	if BankHandleButton(p, 5386, 0) {
		t.Fatal("button was handled with the bank closed")
	}
}

func TestBankHandleWidgetGrid(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 100))
	events := &bankEvents{}
	events.install(p)
	fake := bankOpenPlayer(t, p)
	// Withdraw-5 (op 3).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 12, ButtonNum: 3, Slot: 0, ItemID: 995}) {
		t.Fatal("grid op 3 was not handled")
	}
	if got := p.Inventory().GetAmount(995); got != 5 {
		t.Fatalf("inventory coins after op 3 = %d, want 5", got)
	}
	// Withdraw-X (op 6) asks first, then withdraws the
	// entered amount.
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 12, ButtonNum: 6, Slot: 0, ItemID: 995}) {
		t.Fatal("grid op 6 was not handled")
	}
	if len(fake.amountPrompts) != 1 {
		t.Fatalf("amount prompts = %v, want one", fake.amountPrompts)
	}
	p.GetEnteredAmountAction().Execute(7)
	if got := p.Inventory().GetAmount(995); got != 12 {
		t.Fatalf("inventory coins after op 6 = %d, want 12", got)
	}
	// Examine (op 11) withdraws nothing.
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 12, ButtonNum: 11, Slot: 0, ItemID: 995}) {
		t.Fatal("grid op 11 was not handled")
	}
	if got := p.Inventory().GetAmount(995); got != 12 {
		t.Fatalf("inventory coins after examine = %d, want 12", got)
	}
	// A handler claiming bank:item-op blocks the withdraw.
	p.EmitCustomEvent = func(name string, payload any) {
		if name == BankItemOpEventName {
			if ev, ok := payload.(*BankItemOpEvent); ok {
				ev.Handled = true
			}
		}
	}
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 12, ButtonNum: 2, Slot: 0, ItemID: 995}) {
		t.Fatal("grid op 2 was not handled")
	}
	if got := p.Inventory().GetAmount(995); got != 12 {
		t.Fatalf("inventory coins after claimed item-op = %d, want 12 (blocked)", got)
	}
}

func TestBankHandleWidgetGridPlaceholder(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 0)) // placeholder
	bankOpenPlayer(t, p)
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 12, ButtonNum: 8, Slot: 0, ItemID: 995}) {
		t.Fatal("grid op 8 on a placeholder was not handled")
	}
	if got := p.Bank(0).ItemAt(0); got.ID() != -1 {
		t.Fatalf("placeholder slot after op 8 = %+v, want cleared", got)
	}
}

func TestBankHandleWidgetSideAndFrame(t *testing.T) {
	p, _ := bankTestSetup()
	p.Inventory().SetItem(0, NewItem(995, 10))
	p.Inventory().SetItem(1, NewItem(4151, 1))
	p.Bank(1).SetItem(0, NewItem(1323, 1))
	fake := bankOpenPlayer(t, p)
	// Side inventory Deposit-5 (op 4).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 15, ChildID: 3, ButtonNum: 4, Slot: 0, ItemID: 995}) {
		t.Fatal("side-inventory op 4 was not handled")
	}
	if got := p.Bank(0).GetAmount(995); got != 5 {
		t.Fatalf("bank coins after side deposit-5 = %d, want 5", got)
	}
	// Side inventory equip (op 9) goes through the hook.
	equipped := -1
	BankEquipHook = func(pl *Player, itemID, slot int) bool {
		equipped = itemID
		return true
	}
	t.Cleanup(func() { BankEquipHook = nil })
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 15, ChildID: 3, ButtonNum: 9, Slot: 1, ItemID: 4151}) {
		t.Fatal("side-inventory op 9 was not handled")
	}
	if equipped != 4151 {
		t.Fatalf("equip hook got %d, want 4151", equipped)
	}
	// Tab bar: view tab 1 (slot 11).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 10, ButtonNum: 1, Slot: 11}) {
		t.Fatal("tab-bar click was not handled")
	}
	if got := BankCurrentTab(p); got != 1 {
		t.Fatalf("current tab after tab-bar click = %d, want 1", got)
	}
	// Frame: note toggle (child 25), insert toggle (23),
	// quantity-X prompt-only (child 35 button 2).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 25, ButtonNum: 1}) {
		t.Fatal("note toggle was not handled")
	}
	if !BankWithdrawAsNote(p) {
		t.Fatal("note toggle did not turn note withdrawal on")
	}
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 23, ButtonNum: 1}) {
		t.Fatal("swap/insert toggle was not handled")
	}
	if !BankInsertMode(p) {
		t.Fatal("swap/insert toggle did not turn insert mode on")
	}
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 35, ButtonNum: 2}) {
		t.Fatal("quantity-X prompt was not handled")
	}
	if len(fake.amountPrompts) != 1 {
		t.Fatalf("amount prompts = %v, want one (prompt-only)", fake.amountPrompts)
	}
	if got := p.Inventory().GetAmount(995); got != 5 {
		t.Fatalf("inventory coins after quantity-X prompt = %d, want 5 (nothing moved)", got)
	}
	// Quantity mode (child 33 = 10).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 33, ButtonNum: 1}) {
		t.Fatal("quantity-10 was not handled")
	}
	if got := BankQuantityMode(p); got != 2 {
		t.Fatalf("quantity mode = %d, want 2", got)
	}
	// Deposit inventory (child 47) and worn (child 49).
	p.Inventory().SetItem(3, NewItem(1323, 1))
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 47, ButtonNum: 1}) {
		t.Fatal("deposit-inventory control was not handled")
	}
	if got := p.Inventory().GetAmount(1323); got != 0 {
		t.Fatalf("inventory scimitars after deposit control = %d, want 0", got)
	}
	// Search child switches to the main tab and drops the
	// pending amount action.
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 42, ButtonNum: 1}) {
		t.Fatal("search child was not handled")
	}
	if got := BankCurrentTab(p); got != 0 {
		t.Fatalf("current tab after search child = %d, want 0", got)
	}
	// Frame close (child 2).
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 2, ButtonNum: 1}) {
		t.Fatal("frame close was not handled")
	}
	if p.Status() != PlayerStatusNone {
		t.Fatalf("status after frame close = %v, want NONE", p.Status())
	}
	// Unknown group / closed bank: not owned.
	if BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 99, ChildID: 1, ButtonNum: 1}) {
		t.Fatal("unknown widget group was handled")
	}
}

func TestBankHandleWidgetWornSlot(t *testing.T) {
	p, _ := bankTestSetup()
	p.Equipment().SetItem(HeadSlot, NewItem(2, 1))
	bankOpenPlayer(t, p)
	// Worn slot 68 (head) op 2 banks just that item.
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 68, ButtonNum: 2}) {
		t.Fatal("worn-slot op 2 was not handled")
	}
	if got := p.Equipment().ItemAt(HeadSlot); got.ID() != -1 {
		t.Fatalf("head slot after worn Bank op = %+v, want cleared", got)
	}
	if got := p.Bank(0).GetAmount(2); got != 1 {
		t.Fatalf("bank cannonballs after worn Bank op = %d, want 1", got)
	}
	// Worn slot op 1 goes through the unequip hook.
	p.Equipment().SetItem(HeadSlot, NewItem(2, 1))
	unequipped := -1
	BankUnequipHook = func(pl *Player, slot int) bool {
		unequipped = slot
		return true
	}
	t.Cleanup(func() { BankUnequipHook = nil })
	if !BankHandleWidgetAction(p, BankWidgetPacket{GroupID: 12, ChildID: 68, ButtonNum: 1}) {
		t.Fatal("worn-slot op 1 was not handled")
	}
	if unequipped != HeadSlot {
		t.Fatalf("unequip hook got slot %d, want %d", unequipped, HeadSlot)
	}
}

func TestBankDragMoveToTabAndReorder(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(4151, 1))
	p.Bank(0).SetItem(1, NewItem(1323, 1))
	bankOpenPlayer(t, p)
	// Drag the whip (display slot 0) onto tab 1 (tab-bar
	// slot 11).
	if !BankHandleDrag(p, BankDragPacket{
		SourceWidgetID: (12 << 16) | 12, SourceSlot: 0, SourceItemID: 4151,
		TargetWidgetID: (12 << 16) | 10, TargetSlot: 11,
	}) {
		t.Fatal("drag to tab was not handled")
	}
	if got := p.Bank(1).GetAmount(4151); got != 1 {
		t.Fatalf("tab 1 whips after drag = %d, want 1", got)
	}
	if got := p.Bank(0).GetAmount(4151); got != 0 {
		t.Fatalf("tab 0 whips after drag = %d, want 0", got)
	}
	// Within-tab reorder via the 245 path: the drag left
	// tab 0 as [empty, scimitar], so the cannonball goes
	// in slot 0. Display layout is tabs 1-9 then 0: tab 1
	// whip is display 0, tab 0 cannonball display 1,
	// scimitar display 2.
	p.Bank(0).SetItem(0, NewItem(2, 1))
	if !BankMoveSlots(p, 1, 2, false) {
		t.Fatal("bank-move slots did not run")
	}
	layout := BankLayout(p)
	if len(layout) < 3 || layout[1].Item.ID() != 1323 || layout[2].Item.ID() != 2 {
		t.Fatalf("layout after swap = %v, want scimitar before cannonball", layout)
	}
	// Insert mode: move display 2 back to display 1.
	if !BankMoveSlots(p, 2, 1, true) {
		t.Fatal("bank-move insert did not run")
	}
	layout = BankLayout(p)
	if len(layout) < 3 || layout[1].Item.ID() != 2 || layout[2].Item.ID() != 1323 {
		t.Fatalf("layout after insert = %v, want cannonball before scimitar", layout)
	}
	// A cross-tab 245 move refuses.
	if BankMoveSlots(p, 0, 1, false) {
		t.Fatal("cross-tab bank-move ran")
	}
}

func TestBankDragDepositAndIncinerate(t *testing.T) {
	p, _ := bankTestSetup()
	p.Inventory().SetItem(0, NewItem(995, 10))
	p.Bank(0).SetItem(0, NewItem(4151, 1))
	events := &bankEvents{}
	events.install(p)
	bankOpenPlayer(t, p)
	// Drag inventory coins onto the bank grid: deposit.
	if !BankHandleDrag(p, BankDragPacket{
		SourceWidgetID: (15 << 16) | 3, SourceSlot: 0, SourceItemID: 995,
		TargetWidgetID: (12 << 16) | 12, TargetSlot: 0,
	}) {
		t.Fatal("inventory-to-bank drag was not handled")
	}
	if got := p.Bank(0).GetAmount(995); got != 10 {
		t.Fatalf("bank coins after drag deposit = %d, want 10", got)
	}
	// Drag the whip onto the incinerator: the event fires,
	// the stack is NOT destroyed (no core consumer).
	if !BankHandleDrag(p, BankDragPacket{
		SourceWidgetID: (12 << 16) | 12, SourceSlot: 0, SourceItemID: 4151,
		TargetWidgetID: (12 << 16) | 54, TargetSlot: 0,
	}) {
		t.Fatal("incinerator drag was not handled")
	}
	if !events.has(BankIncinerateEventName) {
		t.Fatal("bank:incinerate was not emitted")
	}
	if got := p.Bank(0).GetAmount(4151); got != 1 {
		t.Fatalf("bank whips after incinerate drag = %d, want 1 (not destroyed)", got)
	}
}

func TestBankDragTabReorder(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(1).SetItem(0, NewItem(4151, 1))
	p.Bank(2).SetItem(0, NewItem(1323, 1))
	bankOpenPlayer(t, p)
	// Drag tab 1 (slot 11) onto tab 2 (slot 12).
	if !BankHandleDrag(p, BankDragPacket{
		SourceWidgetID: (12 << 16) | 10, SourceSlot: 11,
		TargetWidgetID: (12 << 16) | 10, TargetSlot: 12,
	}) {
		t.Fatal("tab reorder drag was not handled")
	}
	if got := p.Bank(1).GetAmount(1323); got != 1 {
		t.Fatalf("tab 1 scimitars after reorder = %d, want 1", got)
	}
	if got := p.Bank(2).GetAmount(4151); got != 1 {
		t.Fatalf("tab 2 whips after reorder = %d, want 1", got)
	}
}

func TestBankClose(t *testing.T) {
	p, _ := bankTestSetup()
	bankOpenPlayer(t, p)
	BankSetSearching(p, true)
	BankClose(p)
	if p.Status() != PlayerStatusNone {
		t.Fatalf("status after close = %v, want NONE", p.Status())
	}
	if BankSearching(p) {
		t.Fatal("close did not clear the searching flag")
	}
}

func TestBankLayoutAndDisplayID(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(995, 10))
	p.Bank(0).SetItem(1, NewItem(995, 0)) // placeholder
	p.Bank(1).SetItem(0, NewItem(4151, 1))
	layout := BankLayout(p)
	if len(layout) != 3 {
		t.Fatalf("layout entries = %d, want 3", len(layout))
	}
	// Tabs 1-9 first, then 0.
	if layout[0].Tab != 1 || layout[0].Item.ID() != 4151 {
		t.Fatalf("layout[0] = %+v, want tab 1 whip", layout[0])
	}
	if layout[1].Tab != 0 || layout[1].Item.ID() != 995 {
		t.Fatalf("layout[1] = %+v, want tab 0 coins", layout[1])
	}
	// A placeholder displays its faded cache placeholder id.
	if got := BankDisplayItemID(p, NewItem(995, 0)); got != 995 {
		t.Fatalf("display id of coin placeholder = %d, want 995", got)
	}
	if got := BankDisplayItemID(p, NewItem(4151, 0)); got != 4151 {
		t.Fatalf("display id of whip placeholder = %d, want 4151", got)
	}
	if _, ok := BankResolveDisplaySlot(p, 99); ok {
		t.Fatal("out-of-range display slot resolved")
	}
	if _, ok := BankResolveDisplaySlot(p, -1); ok {
		t.Fatal("negative display slot resolved")
	}
}

func TestBankMoveItemAcrossTabs(t *testing.T) {
	p, _ := bankTestSetup()
	p.Bank(0).SetItem(0, NewItem(4151, 1))
	p.Bank(1).SetItem(0, NewItem(1323, 1))
	bankOpenPlayer(t, p)
	// Swap mode across tabs: the two items trade places.
	// Layout: display 0 = tab 1 scimitar, display 1 = tab 0 whip.
	BankMoveItem(p, 1, 4151, 0)
	if got := p.Bank(0).GetAmount(1323); got != 1 {
		t.Fatalf("tab 0 scimitars after cross-tab swap = %d, want 1", got)
	}
	if got := p.Bank(1).GetAmount(4151); got != 1 {
		t.Fatalf("tab 1 whips after cross-tab swap = %d, want 1", got)
	}
}
