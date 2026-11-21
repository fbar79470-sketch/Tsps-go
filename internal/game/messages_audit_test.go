package game

import (
	"reflect"
	"testing"
)

// Wired-text audit tests: every game-layer site that used to drop
// a chat message for want of an encoder now sends the exact TS
// text through Player.SendMessage. The fake session records what
// the player was told; the state assertions pin that the message
// never affects the mutation (the TS behavior).

func messagePlayer(t *testing.T, src ItemDefinitionSource) (*Player, *fakeSession) {
	t.Helper()
	p := testPlayerWithDefs(src)
	sess := newFakeSession()
	p.Session = sess
	return p, sess
}

func TestFullSendsPerKindMessage(t *testing.T) {
	p, sess := messagePlayer(t, nil)
	p.Inventory().Full()
	if want := []string{"Not enough space in your inventory."}; !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("inventory Full messages = %v, want %v", sess.messages, want)
	}
	// Equipment.full is a no-op in TS (Equipment.ts:11-13).
	p.Equipment().Full()
	if len(sess.messages) != 1 {
		t.Fatalf("equipment Full sent: %v", sess.messages)
	}
	// Bank.full sends the default message: the bank:full event
	// that could rewrite it has no plugin system behind it.
	p.Bank(0).Full()
	want := []string{
		"Not enough space in your inventory.",
		"Not enough space in bank.",
	}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
}

func TestFullWithNilPlayerDrops(t *testing.T) {
	// A container with no player drops the message instead of
	// panicking (SendMessage is nil-safe).
	inv := NewInventory(nil)
	inv.Full()
	NewBank(nil).Full()
	NewEquipment(nil).Full()
}

func TestAddOverflowSendsMessage(t *testing.T) {
	// Non-stackable branch (ItemContainer.ts:567-568): a full
	// inventory keeps what fit (nothing) and sends the line.
	p, sess := messagePlayer(t, nil)
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		p.Inventory().SetItem(slot, NewItem(1000+slot, 1))
	}
	p.Inventory().Add(NewItem(2000, 3), false)
	want := []string{"You couldn't hold all those items."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("overflow messages = %v, want %v", sess.messages, want)
	}
	if p.Inventory().GetAmount(2000) != 0 {
		t.Fatal("overflow item landed in a full inventory")
	}
	// A successful add sends nothing.
	p.Inventory().SetItem(0, NewItem(-1, 0))
	p.Inventory().Add(NewItem(2000, 1), false)
	if len(sess.messages) != 1 {
		t.Fatalf("successful add sent: %v", sess.messages)
	}
}

func TestAddStackableOverflowSendsMessage(t *testing.T) {
	// Stackable branch (ItemContainer.ts:539-540): a stackable
	// item with no matching stack and no empty slot.
	src := &testItemDefs{stackable: map[int]bool{995: true}}
	p, sess := messagePlayer(t, src)
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		p.Inventory().SetItem(slot, NewItem(1000+slot, 1))
	}
	p.Inventory().Add(NewItem(995, 10), false)
	want := []string{"You couldn't hold all those items."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("stackable overflow messages = %v, want %v", sess.messages, want)
	}
	// Merging into a matching stack sends nothing.
	p.Inventory().SetItem(0, NewItem(995, 5))
	p.Inventory().Add(NewItem(995, 10), false)
	if len(sess.messages) != 1 {
		t.Fatalf("stack merge sent: %v", sess.messages)
	}
	if got := p.Inventory().ItemAt(0).Amount(); got != 15 {
		t.Fatalf("merged stack = %d, want 15", got)
	}
}

func TestSwitchItemDepositClampSendsMessage(t *testing.T) {
	// SwitchItem into a bank whose matching stack is at
	// MAX_STACK_AMOUNT: the clamp reads 0 and the deposit
	// refusal is sent (ItemContainer.ts:322-324). The source
	// slot is untouched.
	p, sess := messagePlayer(t, nil)
	bank := p.Bank(0)
	bank.SetItem(0, NewItem(995, MaxStackAmount))
	p.Inventory().SetItem(0, NewItem(995, 10))
	item := p.Inventory().ItemAt(0)
	p.Inventory().SwitchItem(bank, &item, false, 0, false)
	want := []string{"You cannot deposit that entire amount into your bank."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("SwitchItem messages = %v, want %v", sess.messages, want)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != 995 || got.Amount() != 10 {
		t.Fatalf("source slot changed: %+v", got)
	}
}

func TestSwitchItemsDepositClampSendsMessage(t *testing.T) {
	// SwitchItems (ItemContainer.ts:383-385): the same clamp
	// and the same line, without a slot.
	p, sess := messagePlayer(t, nil)
	bank := p.Bank(0)
	bank.SetItem(0, NewItem(995, MaxStackAmount))
	p.Inventory().SetItem(3, NewItem(995, 10))
	item := NewItem(995, 10)
	p.Inventory().SwitchItems(bank, &item, false, false)
	want := []string{"You cannot deposit that entire amount into your bank."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("SwitchItems messages = %v, want %v", sess.messages, want)
	}
	if got := p.Inventory().ItemAt(3); got.ID() != 995 || got.Amount() != 10 {
		t.Fatalf("source slot changed: %+v", got)
	}
}

func TestSwitchItemDepositWithRoomSendsNothing(t *testing.T) {
	// The clamp only fires at zero room: a deposit that fits
	// sends nothing.
	p, sess := messagePlayer(t, nil)
	bank := p.Bank(0)
	p.Inventory().SetItem(0, NewItem(995, 10))
	item := p.Inventory().ItemAt(0)
	p.Inventory().SwitchItem(bank, &item, false, 0, false)
	if len(sess.messages) != 0 {
		t.Fatalf("fitting deposit sent: %v", sess.messages)
	}
	if bank.GetAmount(995) != 10 {
		t.Fatal("deposit did not land in the bank")
	}
}

func TestSetLevelHitpointsFloorSendsMessage(t *testing.T) {
	p, sm := newTestSkillManager()
	sess := newFakeSession()
	p.Session = sess
	sm.SetLevel(SkillHitpoints, 5)
	want := []string{"Hitpoints must be set to at least level 10."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("SetLevel messages = %v, want %v", sess.messages, want)
	}
	if got := sm.CurrentLevel(SkillHitpoints); got != 10 {
		t.Fatalf("HP current = %d, want 10 (refused)", got)
	}
	// A legal setLevel sends nothing.
	sm.SetLevel(SkillHitpoints, 10)
	if len(sess.messages) != 1 {
		t.Fatalf("legal SetLevel sent: %v", sess.messages)
	}
}

func TestTickWalkToUnreachableSendsMessage(t *testing.T) {
	// Every tile answers blockWalk, so the ground-item route
	// fails: the walk-to ends with "I can't reach that!"
	// (MovementQueue.ts:1316), the queue reset, and the task
	// stopped — and the arrival action never runs.
	blocked := func(x, y, level int) int32 { return blockWalk }
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, blocked)
	acted := false
	q.WalkToGroundItem(Location{X: 130, Y: 100, Z: 0}, func() { acted = true }, 1)
	if !q.WalkToPending() {
		t.Fatal("walk-to not pending after WalkToGroundItem")
	}
	tickQueue(q, 2)
	if acted {
		t.Fatal("arrival action ran for an unreachable walk-to")
	}
	if q.WalkToPending() {
		t.Fatal("walk-to still pending after the unreachable tick")
	}
	if want := []string{"I can't reach that!"}; !reflect.DeepEqual(owner.messages, want) {
		t.Fatalf("messages = %v, want %v", owner.messages, want)
	}
	// No second line on later ticks: the task stopped.
	tickQueue(q, 3)
	if len(owner.messages) != 1 {
		t.Fatalf("messages = %v, want exactly one line", owner.messages)
	}
}

func TestTickWalkToReachableSendsNothing(t *testing.T) {
	// A reachable walk-to runs its action and stays silent.
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil) // open ground.
	acted := false
	q.WalkToGroundItem(Location{X: 102, Y: 100, Z: 0}, func() { acted = true }, 1)
	for cycle := int64(2); cycle <= 10 && !acted; cycle++ {
		tickQueue(q, cycle)
	}
	if !acted {
		t.Fatal("arrival action never ran on open ground")
	}
	if len(owner.messages) != 0 {
		t.Fatalf("reachable walk-to sent: %v", owner.messages)
	}
}

func TestBlockedWalkStaysSilent(t *testing.T) {
	// Regression: a blockMovement refusal sends NOTHING — the
	// only mobility Go produces is INVALID, whose TS
	// sendMessage falls in the default case (Mobility, 1998-2023).
	owner := newFakeOwner(Location{X: 100, Y: 100, Z: 0})
	q := NewMovementQueue(owner, nil, nil)
	q.SetBlockMovement(true)
	q.RequestWalk(Location{X: 105, Y: 100, Z: 0}, nil)
	if len(owner.messages) != 0 {
		t.Fatalf("blocked RequestWalk sent: %v", owner.messages)
	}
	q.WalkToTile(Location{X: 105, Y: 100, Z: 0}, func() {}, q.ReachedTile, false, 1)
	if len(owner.messages) != 0 {
		t.Fatalf("blocked WalkToTile sent: %v", owner.messages)
	}
}
