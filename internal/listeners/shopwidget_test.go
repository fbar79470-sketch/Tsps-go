package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Shop widget/container routing tests: HandleShopWidgetAction
// calls the seam only for groups 300/301 with the packet
// translated per the TS handleWidgetAction reads, and
// HandleShopContainerAction delegates unconditionally. The
// fake handler records what it receives; the ShopManager
// behavior behind the seam is Worker A's, not tested here.

// fakeShopWidgetHandler records shop seam calls and returns a
// configured consume result.
type fakeShopWidgetHandler struct {
	widgetCalls    []ShopWidgetPacket
	containerCalls []ShopItemContainerAction
	consume        bool
}

func (f *fakeShopWidgetHandler) HandleWidgetAction(player *game.Player, packet ShopWidgetPacket) bool {
	f.widgetCalls = append(f.widgetCalls, packet)
	return f.consume
}

func (f *fakeShopWidgetHandler) HandleItemContainerAction(player *game.Player, action ShopItemContainerAction) bool {
	f.containerCalls = append(f.containerCalls, action)
	return f.consume
}

func shopWidgetMsg(group, child, button int, hasButton bool, option string, hasOption bool, slot, itemID int) protocol.WidgetAction {
	return protocol.WidgetAction{
		GroupID:      group,
		ChildID:      child,
		ButtonNum:    button,
		HasButtonNum: hasButton,
		Option:       option,
		HasOption:    hasOption,
		Slot:         slot,
		ItemID:       itemID,
	}
}

func TestHandleShopWidgetActionRoutesShopGroups(t *testing.T) {
	handler := &fakeShopWidgetHandler{consume: true}
	player := testPlayer()

	// Group 300, stock list (child 16), button 2, slot 4,
	// item present: the seam is called with the packet
	// translated.
	msg := shopWidgetMsg(300, 16, 2, true, "Buy 5", true, 4, 995)
	if !HandleShopWidgetAction(player, msg, handler) {
		t.Fatal("group 300 must return the seam's consume result")
	}
	if len(handler.widgetCalls) != 1 {
		t.Fatalf("widget calls = %d, want 1", len(handler.widgetCalls))
	}
	got := handler.widgetCalls[0]
	if got.GroupID != 300 || got.ChildID != 16 || got.ButtonNum != 2 ||
		got.Option != "Buy 5" || !got.HasOption ||
		got.Slot != 4 || !got.HasSlot || got.ItemID != 995 || !got.HasItemID {
		t.Fatalf("packet = %+v, want group 300/child 16/button 2/option Buy 5/slot 4/item 995, all present", got)
	}

	// Group 301, sell list (child 0): also routed.
	msg = shopWidgetMsg(301, 0, 3, true, "Sell 10", true, 0, 4151)
	if !HandleShopWidgetAction(player, msg, handler) {
		t.Fatal("group 301 must return the seam's consume result")
	}
	if len(handler.widgetCalls) != 2 {
		t.Fatalf("widget calls = %d, want 2", len(handler.widgetCalls))
	}

	// The seam's false propagates (shop not open): the chain
	// falls through to the next branch.
	handler.consume = false
	if HandleShopWidgetAction(player, msg, handler) {
		t.Fatal("a false seam result must propagate")
	}
}

func TestHandleShopWidgetActionTranslation(t *testing.T) {
	handler := &fakeShopWidgetHandler{}
	player := testPlayer()

	// No button number in the packet: ButtonNum defaults to 0
	// (TS `packet.buttonNum ?? 0`).
	msg := shopWidgetMsg(300, 16, 0, false, "", false, 4, 995)
	HandleShopWidgetAction(player, msg, handler)
	if got := handler.widgetCalls[0]; got.ButtonNum != 0 {
		t.Fatalf("ButtonNum = %d, want 0 when absent", got.ButtonNum)
	}

	// Absent slot/item (-1): HasSlot/HasItemID false.
	msg = shopWidgetMsg(300, 5, 1, true, "", false, -1, -1)
	HandleShopWidgetAction(player, msg, handler)
	if got := handler.widgetCalls[1]; got.HasSlot || got.HasItemID {
		t.Fatalf("presence = slot %v item %v, want both false", got.HasSlot, got.HasItemID)
	}

	// A raw 0xffff slot (the SUB packet passes it through):
	// present, value 65535 — the ShopManager's display-slot
	// check consumes it as stale, it must not be rewritten
	// to absent here.
	msg = shopWidgetMsg(300, 16, 2, true, "", false, 65535, -1)
	HandleShopWidgetAction(player, msg, handler)
	if got := handler.widgetCalls[2]; !got.HasSlot || got.Slot != 65535 {
		t.Fatalf("slot = %d/%v, want 65535/true", got.Slot, got.HasSlot)
	}

	// Option passes verbatim (the ShopManager trims and
	// lowercases): surrounding space and case preserved.
	msg = shopWidgetMsg(300, 16, 2, true, "  Buy 5  ", true, 4, 995)
	HandleShopWidgetAction(player, msg, handler)
	if got := handler.widgetCalls[3]; got.Option != "  Buy 5  " {
		t.Fatalf("Option = %q, want verbatim", got.Option)
	}
}

func TestHandleShopWidgetActionFallsThroughOtherGroups(t *testing.T) {
	handler := &fakeShopWidgetHandler{consume: true}
	player := testPlayer()

	// Main inventory (149), equipment (12), bank (595),
	// prayer (541), and the shop interface id itself (3824
	// is never a group id): none reach the seam.
	for _, group := range []int{149, 12, 595, 541, 3824, 0, 3000} {
		msg := shopWidgetMsg(group, 0, 1, true, "", false, 0, 995)
		if HandleShopWidgetAction(player, msg, handler) {
			t.Fatalf("group %d must fall through (false)", group)
		}
	}
	if len(handler.widgetCalls) != 0 {
		t.Fatalf("widget calls = %d, want 0", len(handler.widgetCalls))
	}
}

func TestHandleShopWidgetActionNilDegeneration(t *testing.T) {
	handler := &fakeShopWidgetHandler{consume: true}
	msg := shopWidgetMsg(300, 16, 2, true, "", false, 4, 995)
	if HandleShopWidgetAction(nil, msg, handler) {
		t.Fatal("nil player must return false")
	}
	if HandleShopWidgetAction(testPlayer(), msg, nil) {
		t.Fatal("nil handler must return false")
	}
	if len(handler.widgetCalls) != 0 {
		t.Fatalf("widget calls = %d, want 0", len(handler.widgetCalls))
	}
}

func TestHandleShopContainerActionDelegatesUnconditionally(t *testing.T) {
	handler := &fakeShopWidgetHandler{consume: true}
	player := testPlayer()

	// Buy from the stock container: delegated with all fields.
	action := ShopItemContainerAction{
		Kind:        ShopContainerActionBuySell,
		ContainerID: ShopItemsInterfaceID,
		Slot:        3,
		ItemID:      995,
		Amount:      5,
	}
	if !HandleShopContainerAction(player, action, handler) {
		t.Fatal("container action must return the seam's consume result")
	}
	if len(handler.containerCalls) != 1 || handler.containerCalls[0] != action {
		t.Fatalf("container calls = %+v, want [%+v]", handler.containerCalls, action)
	}

	// A value check on the inventory container: delegated.
	action = ShopItemContainerAction{
		Kind:        ShopContainerActionValue,
		ContainerID: ShopInventoryInterfaceID,
		Slot:        0,
		ItemID:      4151,
	}
	HandleShopContainerAction(player, action, handler)
	if len(handler.containerCalls) != 2 || handler.containerCalls[1] != action {
		t.Fatalf("container calls = %+v, want the value action appended", handler.containerCalls)
	}

	// A NON-shop container id is still delegated: TS
	// handleItemContainerAction reads the current shop (with
	// its stale-state side effect) before checking the
	// container, so filtering here would change behavior.
	action = ShopItemContainerAction{
		Kind:        ShopContainerActionBuySell,
		ContainerID: 149,
		Slot:        0,
		ItemID:      995,
		Amount:      1,
	}
	HandleShopContainerAction(player, action, handler)
	if len(handler.containerCalls) != 3 {
		t.Fatalf("container calls = %d, want 3 (no container pre-filter)", len(handler.containerCalls))
	}

	// The seam's false propagates.
	handler.consume = false
	if HandleShopContainerAction(player, action, handler) {
		t.Fatal("a false seam result must propagate")
	}

	// Nil player / nil handler: false, no call.
	before := len(handler.containerCalls)
	if HandleShopContainerAction(nil, action, handler) {
		t.Fatal("nil player must return false")
	}
	if HandleShopContainerAction(player, action, nil) {
		t.Fatal("nil handler must return false")
	}
	if len(handler.containerCalls) != before {
		t.Fatal("nil calls must not reach the seam")
	}
}

func TestShopInterfaceIDs(t *testing.T) {
	// ShopManager.ts:75-79.
	if ShopInterfaceID != 3824 || ShopInventoryInterfaceID != 3823 ||
		ShopItemsInterfaceID != 3900 || ShopMainInterfaceID != 300 ||
		ShopSideInterfaceID != 301 {
		t.Fatalf("ids = %d/%d/%d/%d/%d, want 3824/3823/3900/300/301",
			ShopInterfaceID, ShopInventoryInterfaceID, ShopItemsInterfaceID,
			ShopMainInterfaceID, ShopSideInterfaceID)
	}
}
