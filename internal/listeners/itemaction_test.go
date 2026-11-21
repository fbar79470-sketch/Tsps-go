package listeners

import (
	"testing"

	"tsps-go/internal/game"
)

// Item-action listener tests: the emit sites in itemaction.go
// (handleAction / handleFirstAction) firing through the
// ItemActionHooks seam, grounded in
// ItemActionPacketListener.ts (handleAction 78-108,
// handleFirstAction 110-147) and PluginManager.ts
// (emitCanUseItem 1654-1657, emitItemAction 2016-2043).
// The hooks here are inline funcs, not the hub: the hub-side
// registration and emit semantics are pinned in
// internal/plugins/itemaction_test.go, and the cmd/server
// adapter translation in
// cmd/server/itemactionlistenerwiring_test.go.
//
// itemSnack is a fixture-only non-wieldable with a second
// inventory action, so routeInventoryAction can drive a
// non-first click through to handleAction.
const itemSnack = 111

func snackDefs() fakeDefs {
	return fakeDefs{&defTable{cfgs: map[int]defConfig{
		itemSnack: {slot: -1, dropable: true, tradeable: true, invActions: []string{"", "Eat", "", "", "Drop"}},
	}}}
}

func TestHandleFirstActionPluginClaimsStopsEquip(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	var got *ItemActionEvent
	hooks := ItemActionHooks{
		EmitItemAction: func(ev *ItemActionEvent) bool {
			got = ev
			ev.Handled = true
			return true
		},
	}
	if got := handleFirstAction(p, game.InventoryInterfaceID, itemBronzeSword, 0, "", hooks); got != itemActionHandled {
		t.Fatalf("handleFirstAction = %v, want handled", got)
	}
	if got == nil {
		t.Fatal("plugin event never fired for the first click")
	}
	if got.Player != p || got.InterfaceID != game.InventoryInterfaceID ||
		got.ItemID != itemBronzeSword || got.Slot != 0 || got.ClickType != 1 {
		t.Fatalf("event = %+v, want the first-click fields", got)
	}
	// Handled stops the equip fallback: the sword stays in
	// the inventory and never reaches the weapon slot.
	if got := p.Inventory().ItemAt(0); got.ID() != itemBronzeSword {
		t.Errorf("inventory slot 0 = %+v, want the bronze sword", got)
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() == itemBronzeSword {
		t.Errorf("weapon slot = %+v, want empty: a handled click must not equip", got)
	}
}

func TestHandleFirstActionUnhandledStillEquips(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	emitRan := false
	hooks := ItemActionHooks{
		EmitItemAction: func(ev *ItemActionEvent) bool {
			emitRan = true
			return false
		},
	}
	if got := handleFirstAction(p, game.InventoryInterfaceID, itemBronzeSword, 0, "", hooks); got != itemActionHandled {
		t.Fatalf("handleFirstAction = %v, want handled (equip fallback)", got)
	}
	if !emitRan {
		t.Fatal("plugin event never fired for the unhandled first click")
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
		t.Errorf("weapon slot = %+v, want the bronze sword: unhandled clicks still equip", got)
	}
}

func TestHandleActionNonFirstPluginClaims(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(snackDefs())
	setInv(p, 0, itemSnack, 1)
	var got *ItemActionEvent
	hooks := ItemActionHooks{
		EmitItemAction: func(ev *ItemActionEvent) bool {
			got = ev
			ev.Handled = true
			return true
		},
	}
	if got := handleAction(p, game.InventoryInterfaceID, itemSnack, 0, 2, "Eat", hooks); got != itemActionHandled {
		t.Fatalf("handleAction = %v, want handled", got)
	}
	if got == nil {
		t.Fatal("plugin event never fired for the non-first click")
	}
	if got.ClickType != 2 || got.Option != "Eat" || got.ItemID != itemSnack {
		t.Fatalf("event = %+v, want clickType 2 / Eat / snack", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemSnack {
		t.Errorf("inventory slot 0 = %+v, want the snack: a handled click leaves the item put", got)
	}
}

func TestHandleActionNonFirstUnhandled(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(snackDefs())
	setInv(p, 0, itemSnack, 1)
	hooks := ItemActionHooks{
		EmitItemAction: func(ev *ItemActionEvent) bool { return false },
	}
	if got := handleAction(p, game.InventoryInterfaceID, itemSnack, 0, 2, "Eat", hooks); got != itemActionPlugin {
		t.Fatalf("handleAction = %v, want plugin (unhandled)", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemSnack {
		t.Errorf("inventory slot 0 = %+v, want the snack", got)
	}
}

func TestHandleActionGateRefusesBeforeEmit(t *testing.T) {
	p := testPlayer()
	p.SetItemDefinitions(snackDefs())
	setInv(p, 0, itemSnack, 1)
	emitRan := false
	deny := false
	hooks := ItemActionHooks{
		EmitCanUseItem: func(player *game.Player, itemID int, action, option string) *bool {
			if player != p || itemID != itemSnack || action != "action" || option != "Eat" {
				t.Errorf("gate called with player=%v itemID=%d action=%q option=%q", player, itemID, action, option)
			}
			return &deny
		},
		EmitItemAction: func(ev *ItemActionEvent) bool {
			emitRan = true
			return true
		},
	}
	// A gate refusal consumes the click: handled, no emit,
	// item stays put.
	if got := handleAction(p, game.InventoryInterfaceID, itemSnack, 0, 2, "Eat", hooks); got != itemActionHandled {
		t.Fatalf("handleAction = %v, want handled (gate refusal)", got)
	}
	if emitRan {
		t.Fatal("plugin event fired after a gate refusal")
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemSnack {
		t.Errorf("inventory slot 0 = %+v, want the snack", got)
	}
	// An allow proceeds to the emit; an undecided (nil)
	// answer does the same.
	allow := true
	hooks.EmitCanUseItem = func(player *game.Player, itemID int, action, option string) *bool {
		return &allow
	}
	emitRan = false
	if got := handleAction(p, game.InventoryInterfaceID, itemSnack, 0, 2, "Eat", hooks); got != itemActionHandled {
		t.Fatalf("handleAction = %v, want handled (plugin claim after allow)", got)
	}
	if !emitRan {
		t.Fatal("plugin event did not fire after a gate allow")
	}
	hooks.EmitCanUseItem = func(player *game.Player, itemID int, action, option string) *bool {
		return nil
	}
	emitRan = false
	if got := handleAction(p, game.InventoryInterfaceID, itemSnack, 0, 2, "Eat", hooks); got != itemActionHandled {
		t.Fatalf("handleAction = %v, want handled (plugin claim after undecided)", got)
	}
	if !emitRan {
		t.Fatal("plugin event did not fire after an undecided gate")
	}
}

func TestRouteInventoryActionItemActionHooks(t *testing.T) {
	// A claiming first-action hook through the route: the
	// click is Implemented and the sword never equips.
	p := testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	deps := Deps{ItemActionHooks: ItemActionHooks{
		EmitItemAction: func(ev *ItemActionEvent) bool {
			ev.Handled = true
			return true
		},
	}}
	got := routeInventoryAction(p, game.InventoryInterfaceID, 0, itemBronzeSword, "", nil, deps)
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("route = %+v, want Implemented", got)
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() == itemBronzeSword {
		t.Errorf("weapon slot = %+v, want empty: a handled click must not equip", got)
	}

	// A claiming non-first hook through the route: the "Eat"
	// click (slot 2) is Implemented and the snack stays put.
	p = testPlayer()
	p.SetItemDefinitions(snackDefs())
	setInv(p, 0, itemSnack, 1)
	got = routeInventoryAction(p, game.InventoryInterfaceID, 0, itemSnack, "Eat", nil, deps)
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("route = %+v, want Implemented", got)
	}
	if got := p.Inventory().ItemAt(0); got.ID() != itemSnack {
		t.Errorf("inventory slot 0 = %+v, want the snack", got)
	}

	// An unhandled first click through the route still equips.
	p = testPlayer()
	p.SetItemDefinitions(fixtureDefs())
	setInv(p, 0, itemBronzeSword, 1)
	deps.ItemActionHooks.EmitItemAction = func(ev *ItemActionEvent) bool { return false }
	got = routeInventoryAction(p, game.InventoryInterfaceID, 0, itemBronzeSword, "", nil, deps)
	if got != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("route = %+v, want Implemented (equip fallback)", got)
	}
	if got := p.Equipment().ItemAt(game.WeaponSlot); got.ID() != itemBronzeSword {
		t.Errorf("weapon slot = %+v, want the bronze sword", got)
	}
}
