package fletching

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// TestSpoolCapacityTiers pins the capacity tiers by spares
// (Spool.Fletching.js CAPACITY_BY_SPARES), clamped at the last
// tier.
func TestSpoolCapacityTiers(t *testing.T) {
	p := newTestPlayer("alice")
	want := []int{250, 500, 2000, 5000, 10000, 100000}
	for spares, capacity := range want {
		p.SetAttribute(SpoolSparesAttribute, spares)
		if got := spoolCapacity(p); got != capacity {
			t.Fatalf("capacity at %d spares = %d, want %d", spares, got, capacity)
		}
	}
	p.SetAttribute(SpoolSparesAttribute, 99)
	if got := spoolCapacity(p); got != 100000 {
		t.Fatalf("capacity at 99 spares = %d, want 100000 (clamped)", got)
	}
}

func TestSpoolFillPlainAndNoted(t *testing.T) {
	f, _, _ := newTestFletching(t)
	f.noteID = func(id int) int {
		if id == fxBowString {
			return fxNotedBowString
		}
		return 0
	}
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxBowString, 3)
	p.Inventory().Adds(fxNotedBowString, 4)
	f.fill(p)
	if got := spoolCharges(p); got != 7 {
		t.Fatalf("spool charges = %d, want 7 (3 plain + 4 noted)", got)
	}
	if p.Inventory().GetAmount(fxBowString) != 0 || p.Inventory().GetAmount(fxNotedBowString) != 0 {
		t.Fatal("bow strings were not all moved onto the spool")
	}
	if !session.hasMessage("You add 7 bow strings to the spool. It now holds 7.") {
		t.Fatalf("messages = %v, want the fill line", session.messages)
	}
	if got := session.varbitsSent[6000]; got != 7 {
		t.Fatalf("charges varbit sent = %d, want 7", got)
	}
}

func TestSpoolFillRespectsCapacity(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxBowString, 300)
	f.fill(p)
	if got := spoolCharges(p); got != 250 {
		t.Fatalf("spool charges = %d, want 250 (capacity)", got)
	}
	if got := p.Inventory().GetAmount(fxBowString); got != 50 {
		t.Fatalf("bow strings left = %d, want 50", got)
	}
	// A full spool refuses.
	f.fill(p)
	if !session.hasMessage("Your spool can't hold any more bow strings.") {
		t.Fatalf("messages = %v, want the full-spool line", session.messages)
	}
}

func TestSpoolFillWithNothing(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 1)
	f.fill(p)
	if !session.hasMessage("You don't have any bow strings to add to the spool.") {
		t.Fatalf("messages = %v, want the nothing-to-add line", session.messages)
	}
}

func TestSpoolCheckAction(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.setSpoolCharges(p, 42)
	if !f.checkSpoolAction(&plugins.PluginItemActionEvent{Player: p, ItemID: fxSpool}) {
		t.Fatal("checkSpoolAction = false, want true")
	}
	if !session.hasMessage("Your spool holds 42 / 250 bow strings.") {
		t.Fatalf("messages = %v, want the check line", session.messages)
	}
}

func TestSpoolCombineRaisesCapacity(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 2)
	if !f.combineSpools(&plugins.PluginItemOnItemEvent{Player: p, UsedItemID: fxSpool, UsedWithItemID: fxSpool}) {
		t.Fatal("combineSpools = false, want true (claims the use)")
	}
	if got := spoolSpares(p); got != 1 {
		t.Fatalf("spares = %d, want 1", got)
	}
	if got := spoolCapacity(p); got != 500 {
		t.Fatalf("capacity = %d, want 500", got)
	}
	if got := p.Inventory().GetAmount(fxSpool); got != 1 {
		t.Fatalf("spools left = %d, want 1", got)
	}
	if !session.hasMessage("You combine the spools. Your spool can now hold 500 bow strings.") {
		t.Fatalf("messages = %v, want the combine line", session.messages)
	}
	if got := session.varbitsSent[6001]; got != 1 {
		t.Fatalf("size varbit sent = %d, want 1", got)
	}
}

func TestSpoolCombineNeedsTwoSpools(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 1)
	// Claims the use (TS sets handled first) but combines
	// nothing.
	if !f.combineSpools(&plugins.PluginItemOnItemEvent{Player: p, UsedItemID: fxSpool, UsedWithItemID: fxSpool}) {
		t.Fatal("combineSpools = false, want true")
	}
	if got := spoolSpares(p); got != 0 {
		t.Fatalf("spares = %d, want 0", got)
	}
	if len(session.messages) != 0 {
		t.Fatalf("messages = %v, want none", session.messages)
	}
}

func TestSpoolCombineAtCap(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.SetAttribute(SpoolSparesAttribute, 5)
	p.Inventory().Adds(fxSpool, 2)
	f.combineSpools(&plugins.PluginItemOnItemEvent{Player: p, UsedItemID: fxSpool, UsedWithItemID: fxSpool})
	if !session.hasMessage("Your spool can't be made any bigger.") {
		t.Fatalf("messages = %v, want the cap line", session.messages)
	}
	if got := p.Inventory().GetAmount(fxSpool); got != 2 {
		t.Fatalf("spools left = %d, want 2 (nothing consumed at cap)", got)
	}
}

func TestSpoolWithdrawPlainAndNoted(t *testing.T) {
	f, _, _ := newTestFletching(t)
	f.noteID = func(id int) int {
		if id == fxBowString {
			return fxNotedBowString
		}
		return 0
	}
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.setSpoolCharges(p, 10)
	f.withdraw(p, false)
	if got := p.Inventory().GetAmount(fxBowString); got != 10 {
		t.Fatalf("plain bow strings = %d, want 10", got)
	}
	if got := spoolCharges(p); got != 0 {
		t.Fatalf("charges = %d, want 0", got)
	}
	if !session.hasMessage("You take 10 bow strings off the spool.") {
		t.Fatalf("messages = %v, want the withdraw line", session.messages)
	}
	// Noted: the strings come back as notes.
	f.setSpoolCharges(p, 5)
	f.withdraw(p, true)
	if got := p.Inventory().GetAmount(fxNotedBowString); got != 5 {
		t.Fatalf("noted bow strings = %d, want 5", got)
	}
	if got := spoolCharges(p); got != 0 {
		t.Fatalf("charges = %d, want 0", got)
	}
}

func TestSpoolWithdrawHonoursFreeSlots(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	// 26 junk slots + the spool: 1 free slot for plain strings.
	for i := 0; i < 26; i++ {
		p.Inventory().Adds(9100+i, 1)
	}
	p.Inventory().Adds(fxSpool, 1)
	f.setSpoolCharges(p, 10)
	f.withdraw(p, false)
	if got := p.Inventory().GetAmount(fxBowString); got != 1 {
		t.Fatalf("plain bow strings = %d, want 1 (one free slot)", got)
	}
	if got := spoolCharges(p); got != 9 {
		t.Fatalf("charges = %d, want 9", got)
	}
}

func TestSpoolWithdrawEmptyAndFull(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.withdraw(p, false)
	if !session.hasMessage("Your spool is empty.") {
		t.Fatalf("messages = %v, want the empty line", session.messages)
	}
	// A full inventory refuses with the space line.
	for i := 0; i < 28; i++ {
		p.Inventory().Adds(9200+i, 1)
	}
	f.setSpoolCharges(p, 5)
	session.messages = nil
	f.withdraw(p, false)
	if !session.hasMessage("You don't have enough inventory space.") {
		t.Fatalf("messages = %v, want the space refusal", session.messages)
	}
	if got := spoolCharges(p); got != 5 {
		t.Fatalf("charges = %d, want 5 (nothing withdrawn)", got)
	}
}

func TestSpoolEmptyDialogueWithdrawsPlain(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.setSpoolCharges(p, 6)
	if !f.emptySpoolAction(&plugins.PluginItemActionEvent{Player: p, ItemID: fxSpool}) {
		t.Fatal("emptySpoolAction = false, want true")
	}
	// The option dialogue mounted (group 219) and the manager
	// holds it current.
	dm := p.GetDialogueManager()
	if dm == nil || dm.GetCurrent() == nil {
		t.Fatal("no dialogue current after Empty")
	}
	if _, ok := dm.GetCurrent().(*game.OptionDialogue); !ok {
		t.Fatalf("current dialogue = %T, want *OptionDialogue", dm.GetCurrent())
	}
	// The resume path passes the one-based child index: 1 is
	// "Take them as bow strings."
	if !dm.HandleResume(game.OptionsWidgetID, 1) {
		t.Fatal("HandleResume(option 1) = false, want true")
	}
	if got := p.Inventory().GetAmount(fxBowString); got != 6 {
		t.Fatalf("plain bow strings = %d, want 6", got)
	}
	if got := spoolCharges(p); got != 0 {
		t.Fatalf("charges = %d, want 0", got)
	}
	// Choosing closed the interface (the TS
	// sendInterfaceRemoval, via DialogueManager.Advance).
	if session.removals != 1 {
		t.Fatalf("interface removals = %d, want 1", session.removals)
	}
}

func TestSpoolEmptyDialogueWithdrawsNoted(t *testing.T) {
	f, _, _ := newTestFletching(t)
	f.noteID = func(id int) int {
		if id == fxBowString {
			return fxNotedBowString
		}
		return 0
	}
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.setSpoolCharges(p, 4)
	f.emptySpoolAction(&plugins.PluginItemActionEvent{Player: p, ItemID: fxSpool})
	dm := p.GetDialogueManager()
	if !dm.HandleResume(game.OptionsWidgetID, 2) {
		t.Fatal("HandleResume(option 2) = false, want true")
	}
	if got := p.Inventory().GetAmount(fxNotedBowString); got != 4 {
		t.Fatalf("noted bow strings = %d, want 4", got)
	}
	_ = session
}

func TestSpoolEmptyWhenEmpty(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	if !f.emptySpoolAction(&plugins.PluginItemActionEvent{Player: p, ItemID: fxSpool}) {
		t.Fatal("emptySpoolAction = false, want true")
	}
	if !session.hasMessage("Your spool is empty.") {
		t.Fatalf("messages = %v, want the empty line", session.messages)
	}
	if p.GetDialogueManager().GetCurrent() != nil {
		t.Fatal("a dialogue opened for an empty spool")
	}
}

func TestSpoolLoginSendsVarbits(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	f.setSpoolCharges(p, 0) // clears any earlier sends
	session.varbitsSent = map[int]int{}
	p.SetAttribute(SpoolChargesAttribute, 12)
	p.SetAttribute(SpoolSparesAttribute, 2)
	f.onSpoolLogin(&plugins.PlayerLoginEvent{Player: p, Username: "alice"})
	if got := session.varbitsSent[6000]; got != 12 {
		t.Fatalf("charges varbit on login = %d, want 12", got)
	}
	if got := session.varbitsSent[6001]; got != 2 {
		t.Fatalf("size varbit on login = %d, want 2", got)
	}
}

func TestSpoolStringsOnSpoolFills(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	session := newFakeSession()
	p.Session = session
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxBowString, 9)
	event := &plugins.PluginItemOnItemEvent{Player: p, UsedItemID: fxBowString, UsedWithItemID: fxSpool}
	if !f.stringsOnSpool(event) {
		t.Fatal("stringsOnSpool = false, want true")
	}
	if got := spoolCharges(p); got != 9 {
		t.Fatalf("charges = %d, want 9", got)
	}
	if !session.hasMessage("You add 9 bow strings to the spool. It now holds 9.") {
		t.Fatalf("messages = %v, want the fill line", session.messages)
	}
}

func TestSpoolAbsorbSpun(t *testing.T) {
	f, _, _ := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxBowString, 8)
	f.absorbSpun(&CraftingSpunEvent{Player: p, ItemID: fxBowString, Amount: 5})
	if got := spoolCharges(p); got != 5 {
		t.Fatalf("charges = %d, want 5 (spun onto the spool)", got)
	}
	if got := p.Inventory().GetAmount(fxBowString); got != 3 {
		t.Fatalf("bow strings left = %d, want 3", got)
	}
	// A non-bow-string spin does nothing; neither does a spin
	// with no spool carried.
	f.absorbSpun(&CraftingSpunEvent{Player: p, ItemID: fxLogs, Amount: 5})
	if got := spoolCharges(p); got != 5 {
		t.Fatalf("charges after a log spin = %d, want 5", got)
	}
	p.Inventory().DeleteNumber(fxSpool, 1)
	f.absorbSpun(&CraftingSpunEvent{Player: p, ItemID: fxBowString, Amount: 2})
	if got := spoolCharges(p); got != 5 {
		t.Fatalf("charges without a spool = %d, want 5", got)
	}
}

func TestSpoolIsVirtualStringingSource(t *testing.T) {
	f, _, tm := newTestFletching(t)
	p := newTestPlayer("alice")
	p.Session = newFakeSession()
	setFletching(p, 99)
	p.Inventory().Adds(fxSpool, 1)
	p.Inventory().Adds(fxUnstrungShortbow, 1)
	f.setSpoolCharges(p, 3)
	// No plain bow strings at all: the spool alone strings the
	// bow, one batch available → the 1-batch fast path starts
	// the task (firstTick 1).
	if !f.handlePair(p, fxSpool, fxUnstrungShortbow) {
		t.Fatal("handlePair(spool, unstrung) = false, want true")
	}
	if len(f.running) != 1 {
		t.Fatal("no task running after spool stringing")
	}
	driveTicks(tm, 1)
	if got := p.Inventory().GetAmount(fxShortbow); got != 1 {
		t.Fatalf("shortbows = %d, want 1", got)
	}
	if got := spoolCharges(p); got != 2 {
		t.Fatalf("spool charges = %d, want 2 (one string spent)", got)
	}
	if len(f.running) != 0 {
		t.Fatal("task still running after the single batch")
	}
}
