package listeners

import (
	"reflect"
	"testing"

	"tsps-go/internal/game"
)

// Wired-text audit tests: every listener site that used to drop a
// refusal line for want of a chat encoder now sends the exact TS
// text through Player.SendMessage. Regression tests pin the sites
// that stay silent because their TS text cannot be grounded (the
// DEVELOPER drop message, the slot -1 server log) or because TS
// sends nothing there.

// messageRecordingSession is a SessionPort + ChatMessageSender
// test double recording the game messages it receives.
type messageRecordingSession struct {
	messages      []string
	typedMessages [][3]any
}

func (s *messageRecordingSession) SendPacket([]byte)                 {}
func (s *messageRecordingSession) SendRunEnergy(int, bool, int, int) {}
func (s *messageRecordingSession) SendDestination(int, int)          {}
func (s *messageRecordingSession) SendVarp(int, int)                 {}
func (s *messageRecordingSession) SendGameMessage(message string) {
	s.messages = append(s.messages, message)
}
func (s *messageRecordingSession) SendTypedGameMessage(chatType int, message, from string) {
	s.typedMessages = append(s.typedMessages, [3]any{chatType, message, from})
}
func (s *messageRecordingSession) FlushTick(int64) error { return nil }
func (s *messageRecordingSession) Close(int, string)     {}
func (s *messageRecordingSession) IsOpen() bool          { return true }

var (
	_ game.SessionPort       = (*messageRecordingSession)(nil)
	_ game.ChatMessageSender = (*messageRecordingSession)(nil)
)

func messageTestPlayer() (*game.Player, *messageRecordingSession) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	sess := &messageRecordingSession{}
	p.Session = sess
	return p, sess
}

func TestEquipRequirementRefusalSendsMessage(t *testing.T) {
	p, sess := messageTestPlayer()
	setInv(p, 0, itemReqSword, 1)
	if equip(p, itemReqSword, 0, game.InventoryInterfaceID) {
		t.Fatal("equip wielded an Attack-40 sword at Attack 1")
	}
	// EquipmentRequirements.message, subject "this"
	// (EquipPacketListener.ts:152-158): "an" before the vowel.
	want := []string{"You need an Attack level of at least 40 to wear this."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
	// Meeting the requirement equips and stays silent.
	p.Skills().SetMaxLevels(game.SkillAttack, 40, false)
	if !equip(p, itemReqSword, 0, game.InventoryInterfaceID) {
		t.Fatal("equip refused an Attack-40 sword at Attack 40")
	}
	if len(sess.messages) != 1 {
		t.Fatalf("successful equip sent: %v", sess.messages)
	}
}

func TestEquipNoSlotSendsNothing(t *testing.T) {
	// Regression: the equipment-slot -1 case logs server-side
	// in TS (EquipPacketListener.ts:159-163) and sends no
	// player text. The refusal is ported; the silence is too.
	p, sess := messageTestPlayer()
	setInv(p, 0, itemPlain, 1)
	if equip(p, itemPlain, 0, game.InventoryInterfaceID) {
		t.Fatal("equip wielded a non-wearable item")
	}
	if len(sess.messages) != 0 {
		t.Fatalf("no-slot equip sent: %v", sess.messages)
	}
}

func TestUnequipNoSpaceSendsMessage(t *testing.T) {
	// Empty-slot branch: a full inventory, a worn sword.
	p, sess := messageTestPlayer()
	setEq(p, game.WeaponSlot, itemBronzeSword, 1)
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	if unequip(p, game.WeaponSlot) {
		t.Fatal("unequip removed a sword into a full inventory")
	}
	want := []string{"You don't have enough free space to do that."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
}

func TestUnequipNoSpaceStackBranchSendsMessage(t *testing.T) {
	// Stack branch: worn arrows cannot join the maxed inventory
	// stack, so the same line is sent (one TS ternary).
	p, sess := messageTestPlayer()
	setEq(p, game.AmmunitionSlot, itemArrows, 5)
	setInv(p, 7, itemArrows, game.MaxStackAmount)
	if unequip(p, game.AmmunitionSlot) {
		t.Fatal("unequip merged arrows into a maxed stack")
	}
	want := []string{"You don't have enough free space to do that."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
	// A roomy unequip stays silent.
	p2, sess2 := messageTestPlayer()
	setEq(p2, game.WeaponSlot, itemBronzeSword, 1)
	if !unequip(p2, game.WeaponSlot) {
		t.Fatal("unequip refused a roomy removal")
	}
	if len(sess2.messages) != 0 {
		t.Fatalf("roomy unequip sent: %v", sess2.messages)
	}
}

func TestItemAction9520SendsMessage(t *testing.T) {
	// ItemActionPacketListener.ts:134-136: unconditional.
	p, sess := messageTestPlayer()
	setInv(p, 0, 9520, 1)
	got := handleFirstAction(p, game.InventoryInterfaceID, 9520, 0, "", ItemActionHooks{})
	if got != itemActionPlugin {
		t.Fatalf("handleFirstAction = %v, want plugin (unhandled)", got)
	}
	want := []string{"You cannot use this in the Wilderness!"}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
}

func TestItemAction2545AttributeSendsMessage(t *testing.T) {
	// ItemActionPacketListener.ts:143-146: the attribute
	// branch returns handled (true) with the unlock line.
	p, sess := messageTestPlayer()
	setInv(p, 0, 2545, 1)
	p.SetAttribute("bounty-hunter:target-teleport-unlocked", true)
	got := handleFirstAction(p, game.InventoryInterfaceID, 2545, 0, "", ItemActionHooks{})
	if got != itemActionHandled {
		t.Fatalf("handleFirstAction = %v, want handled", got)
	}
	want := []string{"You have already unlocked that teleport."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
}

func TestItemAction2545WithoutAttributeStaysSilent(t *testing.T) {
	// Without the flag — absent, false, or a non-bool value —
	// TS falls through to unhandled with no message (the
	// comparison is strict === true; the busy() branch cannot
	// fire: busy() degenerates to the passed hitpoints term).
	cases := []struct {
		name string
		set  func(p *game.Player)
	}{
		{"absent", func(p *game.Player) {}},
		{"false", func(p *game.Player) { p.SetAttribute("bounty-hunter:target-teleport-unlocked", false) }},
		{"string", func(p *game.Player) { p.SetAttribute("bounty-hunter:target-teleport-unlocked", "true") }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p, sess := messageTestPlayer()
			setInv(p, 0, 2545, 1)
			tc.set(p)
			got := handleFirstAction(p, game.InventoryInterfaceID, 2545, 0, "", ItemActionHooks{})
			if got != itemActionPlugin {
				t.Fatalf("handleFirstAction = %v, want plugin (unhandled)", got)
			}
			if len(sess.messages) != 0 {
				t.Fatalf("messages = %v, want none", sess.messages)
			}
		})
	}
}

func TestDropStaysSilentWithoutRights(t *testing.T) {
	// Regression: the DEVELOPER debug message
	// (DropItemPacketListener.ts:57-59) stays unsent — no rights
	// system exists to evaluate its gate. A successful drop
	// sends nothing.
	p, sess := messageTestPlayer()
	setInv(p, 0, itemPlain, 1)
	store := newManagerStore()
	if got := dropItem(p, itemPlain, game.InventoryInterfaceID, 0, store); got != dropDone {
		t.Fatalf("dropItem = %v, want dropDone", got)
	}
	if len(sess.messages) != 0 {
		t.Fatalf("drop sent: %v", sess.messages)
	}
}

func TestPickupFullInventorySendsMessage(t *testing.T) {
	// transferToInventory calls Inventory.Full() when nothing
	// fits (ItemOnGroundManager.ts:297-303); Full() now sends
	// the inventory-full line. The stack stays on the floor.
	p, sess := messageTestPlayer()
	for slot := 0; slot < p.Inventory().Capacity(); slot++ {
		setInv(p, slot, itemPlain, 1)
	}
	loc := p.Location()
	store := newManagerStore()
	stack := game.NewItemOnGround(game.GroundItemSeenByEveryone, "", loc, itemBronzeSword, 1, nil, false, -1, false, nil)
	store.Register(stack)
	if !pickupItem(p, itemBronzeSword, loc.X, loc.Y, 0, false, 0, store) {
		t.Fatal("pickupItem refused a full-inventory take at the gate")
	}
	want := []string{"Not enough space in your inventory."}
	if !reflect.DeepEqual(sess.messages, want) {
		t.Fatalf("messages = %v, want %v", sess.messages, want)
	}
	if stack.PendingRemoval || stack.Amount != 1 {
		t.Fatalf("stack changed: pending=%v amount=%d", stack.PendingRemoval, stack.Amount)
	}
}
