package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Item-on-object tests: verification, walk-to, dispatch, and the
// "Nothing interesting happens." fallback
// (UseItemPacketListener.ts:39-91), plus the INVENTORY_USE_ON
// routing boundary (NetworkBuilder.ts:836-857). Fixtures only.

func routeItemOutcome(p *game.Player, msg protocol.ItemOnObject, cycle int64, deps Deps) Outcome {
	drop, acted, noAction := objectOutcomeFuncs()
	return routeItemOnObject(p, msg, cycle, deps, drop, acted, noAction)
}

func routeUseOnOutcome(p *game.Player, msg protocol.InventoryUseOn, cycle int64, deps Deps) Outcome {
	drop, acted, noAction := objectOutcomeFuncs()
	return routeInventoryUseOn(p, msg, cycle, deps, drop, acted, noAction)
}

func itemOnObjectClick() protocol.ItemOnObject {
	return protocol.ItemOnObject{
		ObjectID: 100, X: 3090, Y: 3524,
		ItemID: 995, Slot: 0, WidgetID: 3214,
	}
}

func TestItemOnObjectUnwired(t *testing.T) {
	p := testPlayer()
	setInv(p, 0, 995, 1)
	msg := itemOnObjectClick()
	if got := routeItemOutcome(p, msg, 1, Deps{}); got.Reason != ReasonUseItem {
		t.Fatalf("unwired = %+v, want ReasonUseItem", got)
	}
	if got := routeItemOutcome(nil, msg, 1, Deps{}); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
	// INVENTORY_USE_ON with a loc target is unwired the same way.
	useOn := protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{
		Kind: protocol.UseOnTargetLoc, ID: 100, HasTile: true, X: 3090, Y: 3524,
	}}
	if got := routeUseOnOutcome(p, useOn, 1, Deps{}); got.Reason != ReasonUseItem {
		t.Fatalf("unwired use-on = %+v, want ReasonUseItem", got)
	}
}

func TestItemOnObjectVerification(t *testing.T) {
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, _ := objectTestDeps(entry, ObjectHooks{})

	// Slot out of bounds (negative and past capacity).
	p := testPlayer()
	setInv(p, 0, 995, 1)
	bad := itemOnObjectClick()
	bad.Slot = -1
	if got := routeItemOutcome(p, bad, 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("slot -1 = %+v, want noAction", got)
	}
	bad.Slot = p.Inventory().Capacity()
	if got := routeItemOutcome(p, bad, 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("slot at capacity = %+v, want noAction", got)
	}
	// Slot holds a different id.
	bad = itemOnObjectClick()
	bad.ItemID = 996
	if got := routeItemOutcome(p, bad, 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("id mismatch = %+v, want noAction", got)
	}
	// Empty slot.
	p2 := testPlayer()
	if got := routeItemOutcome(p2, itemOnObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("empty slot = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() || p2.MovementQueue().WalkToPending() {
		t.Fatal("refused uses must not start a walk-to")
	}
}

func TestItemOnObjectLookupMiss(t *testing.T) {
	p := testPlayer()
	setInv(p, 0, 995, 1)
	deps, _, _ := objectTestDeps(nil, ObjectHooks{})
	if got := routeItemOutcome(p, itemOnObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("lookup miss = %+v, want noAction", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("lookup miss must not start a walk-to")
	}
}

func TestItemOnObjectNoHitpointsGate(t *testing.T) {
	// itemOnObject has NO hitpoints/busy gate: a dead player
	// still starts the walk (unlike object clicks).
	p := testPlayer()
	killPlayer(p)
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, _ := objectTestDeps(entry, ObjectHooks{})
	if got := routeItemOutcome(p, itemOnObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("dead player = %+v, want acted (no HP gate on item-on-object)", got)
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("dead player must still start the walk-to")
	}
}

func TestItemOnObjectArrivalDispatch(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	setInv(p, 0, 995, 3)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	var seen *ItemOnObjectEvent
	var routeClick int = -1
	hooks := ObjectHooks{
		EmitObjectRoute: func(req *ObjectRouteRequest) { routeClick = req.ClickType },
		EmitItemOnObject: func(ev *ItemOnObjectEvent) bool {
			seen = ev
			return true
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	if got := routeItemOutcome(p, itemOnObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("route = %+v, want acted", got)
	}
	// No clickType argument: the route event carries clickType 0.
	if routeClick != 0 {
		t.Fatalf("route clickType = %d, want 0", routeClick)
	}
	tickPlayerQueue(p, 2)
	if seen == nil {
		t.Fatal("arrival must emit the item-on-object event")
	}
	if seen.ObjectID != 100 || seen.ItemID != 995 || seen.ItemSlot != 0 || seen.InterfaceType != 3214 {
		t.Fatalf("event = %+v, want object 100 item 995 slot 0 widget 3214", seen)
	}
	if seen.Item.ID() != 995 || seen.Item.Amount() != 3 {
		t.Fatalf("captured item = id %d amount %d, want 995/3", seen.Item.ID(), seen.Item.Amount())
	}
	if seen.Location != entry.loc {
		t.Fatalf("event location = %+v, want the object tile %+v", seen.Location, entry.loc)
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("handled use must send nothing, got %q", session.gameMessages)
	}
}

func TestItemOnObjectUnhandledMessage(t *testing.T) {
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	hooks := ObjectHooks{
		EmitItemOnObject: func(ev *ItemOnObjectEvent) bool { return false },
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	routeItemOutcome(p, itemOnObjectClick(), 1, deps)
	tickPlayerQueue(p, 2)
	if len(session.gameMessages) != 1 || session.gameMessages[0] != "Nothing interesting happens." {
		t.Fatalf("messages = %q, want exactly [\"Nothing interesting happens.\"]", session.gameMessages)
	}
	// With no hooks at all, the same fallback applies.
	p2 := testPlayer()
	session2 := &capturingSession{}
	p2.Session = session2
	setInv(p2, 0, 995, 1)
	deps2, _, _ := objectTestDeps(entry, ObjectHooks{})
	routeItemOutcome(p2, itemOnObjectClick(), 1, deps2)
	tickPlayerQueue(p2, 2)
	if len(session2.gameMessages) != 1 || session2.gameMessages[0] != "Nothing interesting happens." {
		t.Fatalf("no-hooks messages = %q, want the fallback line", session2.gameMessages)
	}
}

func TestItemOnObjectCapturedItemNotReverified(t *testing.T) {
	// The captured item reference is passed through: swapping
	// the slot's contents after the click does not change the
	// emitted item, and the use still resolves.
	p := testPlayer()
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	var seen *ItemOnObjectEvent
	hooks := ObjectHooks{
		EmitItemOnObject: func(ev *ItemOnObjectEvent) bool {
			seen = ev
			return true
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	routeItemOutcome(p, itemOnObjectClick(), 1, deps)
	setInv(p, 0, 1234, 1) // The slot now holds something else.
	tickPlayerQueue(p, 2)
	if seen == nil {
		t.Fatal("arrival must emit despite the changed slot")
	}
	if seen.Item.ID() != 995 || seen.ItemID != 995 {
		t.Fatalf("emitted item = %d/%d, want the captured 995", seen.Item.ID(), seen.ItemID)
	}
}

func TestItemOnObjectForIdFallback(t *testing.T) {
	// When forPlayer resolves nothing but forId does, the
	// route spec uses the forId definition (the
	// buildObjectRouteSpec fallback): the walk still starts.
	p := testPlayer()
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, defs := objectTestDeps(entry, ObjectHooks{})
	delete(defs.forPlayer, 100)
	if got := routeItemOutcome(p, itemOnObjectClick(), 1, deps); !got.Implemented {
		t.Fatalf("forId fallback = %+v, want acted", got)
	}
	// When neither resolves, the packet returns silently.
	delete(defs.byID, 100)
	p2 := testPlayer()
	setInv(p2, 0, 995, 1)
	if got := routeItemOutcome(p2, itemOnObjectClick(), 1, deps); got.Reason != ReasonNoAction {
		t.Fatalf("no definition = %+v, want noAction", got)
	}
}

func TestInventoryUseOnRouting(t *testing.T) {
	p := testPlayer()
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	var seenInterface int
	hooks := ObjectHooks{
		EmitItemOnObject: func(ev *ItemOnObjectEvent) bool {
			seenInterface = ev.InterfaceType
			return true
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)

	// Loc kind with a tile routes to item-on-object with
	// interfaceType 3214 (the inventory widget).
	loc := protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{
		Kind: protocol.UseOnTargetLoc, ID: 100, HasTile: true, X: 3090, Y: 3524,
	}}
	if got := routeUseOnOutcome(p, loc, 1, deps); !got.Implemented {
		t.Fatalf("loc use-on = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if seenInterface != 3214 {
		t.Fatalf("interfaceType = %d, want 3214", seenInterface)
	}

	// Every other kind drops with ReasonUseItem, as does a
	// loc without a tile. The inventory kind no longer drops:
	// since phase 33 it routes to item-on-item (see
	// TestItemOnItemRouting).
	drops := []struct {
		name   string
		target protocol.UseOnTarget
	}{
		{"loc without tile", protocol.UseOnTarget{Kind: protocol.UseOnTargetLoc, ID: 100}},
		{"npc", protocol.UseOnTarget{Kind: protocol.UseOnTargetNpc, ID: 1, HasTile: true, X: 3090, Y: 3524}},
		{"player", protocol.UseOnTarget{Kind: protocol.UseOnTargetPlayer, ID: 1, HasTile: true, X: 3090, Y: 3524}},
		{"ground", protocol.UseOnTarget{Kind: protocol.UseOnTargetGround, ID: 995, HasTile: true, X: 3090, Y: 3524}},
	}
	for _, c := range drops {
		t.Run(c.name, func(t *testing.T) {
			p := testPlayer()
			setInv(p, 0, 995, 1)
			msg := protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: c.target}
			if got := routeUseOnOutcome(p, msg, 1, deps); got.Reason != ReasonUseItem {
				t.Fatalf("%s = %+v, want ReasonUseItem", c.name, got)
			}
			if p.MovementQueue().WalkToPending() {
				t.Fatalf("%s must not start a walk-to", c.name)
			}
		})
	}
}

func TestItemOnItemRouting(t *testing.T) {
	// The inventory kind of INVENTORY_USE_ON routes to
	// item-on-item immediately: no walk-to, no click-run
	// override, and the packet's item ids are NOT verified
	// against the slots (UseItemPacketListener.ts:15-37 reads
	// the slot items). A registered handler fires through
	// the listener; an unhandled use falls through silently
	// (no "Nothing interesting happens." message).
	p := testPlayer()
	session := &capturingSession{}
	p.Session = session
	setInv(p, 0, 23962, 5)
	setInv(p, 1, 23673, 1)
	var seen *ItemOnItemEvent
	hooks := ObjectHooks{
		EmitItemOnItem: func(ev *ItemOnItemEvent) bool {
			seen = ev
			return true
		},
	}
	deps, _, _ := objectTestDeps(nil, hooks)
	msg := protocol.InventoryUseOn{Slot: 0, ItemID: 23962, Target: protocol.UseOnTarget{
		Kind: protocol.UseOnTargetInventory, ID: 0, Slot: 1, ItemID: 23673,
	}}
	if got := routeUseOnOutcome(p, msg, 1, deps); !got.Implemented {
		t.Fatalf("inventory use-on = %+v, want acted", got)
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("item-on-item must not start a walk-to")
	}
	if seen == nil {
		t.Fatal("route must emit the item-on-item event")
	}
	if seen.UsedItemID != 23962 || seen.UsedItemSlot != 0 || seen.UsedWithItemID != 23673 || seen.UsedWithItemSlot != 1 {
		t.Fatalf("event = %+v, want used 23962/0 with 23673/1", seen)
	}
	if seen.UsedItem.Amount() != 5 || seen.UsedWithItem.ID() != 23673 {
		t.Fatalf("captured items = %d x%d / %d, want 23962 x5 / 23673", seen.UsedItem.ID(), seen.UsedItem.Amount(), seen.UsedWithItem.ID())
	}
	if len(session.gameMessages) != 0 {
		t.Fatalf("handled use must send nothing, got %q", session.gameMessages)
	}

	// Unhandled uses fall through silently, with no hooks at all.
	p2 := testPlayer()
	session2 := &capturingSession{}
	p2.Session = session2
	setInv(p2, 0, 995, 1)
	setInv(p2, 1, 996, 1)
	deps2, _, _ := objectTestDeps(nil, ObjectHooks{
		EmitItemOnItem: func(ev *ItemOnItemEvent) bool { return false },
	})
	if got := routeUseOnOutcome(p2, msg, 1, deps2); !got.Implemented {
		t.Fatalf("unhandled use-on = %+v, want acted", got)
	}
	if len(session2.gameMessages) != 0 {
		t.Fatalf("unhandled use must send nothing, got %q", session2.gameMessages)
	}
	deps3, _, _ := objectTestDeps(nil, ObjectHooks{})
	p3 := testPlayer()
	setInv(p3, 0, 995, 1)
	setInv(p3, 1, 996, 1)
	if got := routeUseOnOutcome(p3, msg, 1, deps3); !got.Implemented {
		t.Fatalf("no-hooks use-on = %+v, want acted", got)
	}

	// The packet's ids are not verified: a mismatched used
	// id still emits the slot items.
	p4 := testPlayer()
	setInv(p4, 0, 995, 1)
	setInv(p4, 1, 996, 1)
	var seenIDs [2]int
	deps4, _, _ := objectTestDeps(nil, ObjectHooks{
		EmitItemOnItem: func(ev *ItemOnItemEvent) bool {
			seenIDs = [2]int{ev.UsedItemID, ev.UsedWithItemID}
			return true
		},
	})
	mismatch := protocol.InventoryUseOn{Slot: 0, ItemID: 1234, Target: protocol.UseOnTarget{
		Kind: protocol.UseOnTargetInventory, ID: 0, Slot: 1, ItemID: 5678,
	}}
	if got := routeUseOnOutcome(p4, mismatch, 1, deps4); !got.Implemented {
		t.Fatalf("mismatched ids = %+v, want acted (slots win)", got)
	}
	if seenIDs != [2]int{995, 996} {
		t.Fatalf("emitted ids = %v, want [995 996]", seenIDs)
	}
}

func TestItemOnItemVerification(t *testing.T) {
	// Slot bounds (UseItemPacketListener.ts:17-22) and empty
	// slots refuse the use silently (noAction, not a drop).
	p := testPlayer()
	setInv(p, 0, 995, 1)
	setInv(p, 1, 996, 1)
	hooks := ObjectHooks{
		EmitItemOnItem: func(ev *ItemOnItemEvent) bool { return true },
	}
	deps, _, _ := objectTestDeps(nil, hooks)
	bad := []struct {
		name string
		msg  protocol.InventoryUseOn
	}{
		{"used slot -1", protocol.InventoryUseOn{Slot: -1, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: 1, ItemID: 996}}},
		{"used slot at capacity", protocol.InventoryUseOn{Slot: p.Inventory().Capacity(), ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: 1, ItemID: 996}}},
		{"target slot -1", protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: -1, ItemID: 996}}},
		{"target slot at capacity", protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: p.Inventory().Capacity(), ItemID: 996}}},
		{"empty used slot", protocol.InventoryUseOn{Slot: 2, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: 1, ItemID: 996}}},
		{"empty target slot", protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: 2, ItemID: 996}}},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if got := routeUseOnOutcome(p, c.msg, 1, deps); got.Reason != ReasonNoAction {
				t.Fatalf("%s = %+v, want noAction", c.name, got)
			}
			if p.MovementQueue().WalkToPending() {
				t.Fatalf("%s must not start a walk-to", c.name)
			}
		})
	}
	// A nil player drops with ReasonNoPlayer.
	msg := protocol.InventoryUseOn{Slot: 0, ItemID: 995, Target: protocol.UseOnTarget{Kind: protocol.UseOnTargetInventory, Slot: 1, ItemID: 996}}
	if got := routeUseOnOutcome(nil, msg, 1, deps); got.Reason != ReasonNoPlayer {
		t.Fatalf("nil player = %+v, want ReasonNoPlayer", got)
	}
}

func TestHandleInventoryUseOnItemDispatch(t *testing.T) {
	// End-to-end dispatch: the INVENTORY_USE_ON opcode (241)
	// with a kind-4 inventory target routes through Handle
	// to the item-on-item path. The payload is built with
	// the decoder's field order: slot short, itemId short,
	// kind byte, id short, hasTile byte, level byte, target
	// slot short, target itemId short.
	p := testPlayer()
	setInv(p, 0, 995, 1)
	setInv(p, 1, 996, 1)
	emitted := false
	deps, _, _ := objectTestDeps(nil, ObjectHooks{
		EmitItemOnItem: func(ev *ItemOnItemEvent) bool {
			emitted = true
			return true
		},
	})
	var payload []byte
	putBE16 := func(v int) { payload = append(payload, byte(v>>8), byte(v)) }
	putBE16(0)                   // slot
	putBE16(995)                 // item id
	payload = append(payload, 4) // kind: inventory
	putBE16(0)                   // id (unused for kind 4)
	payload = append(payload, 0) // hasTile
	payload = append(payload, 0) // level
	putBE16(1)                   // target slot
	putBE16(996)                 // target item id
	got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUseOn, Payload: payload}, 1, deps)
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	if !emitted {
		t.Fatal("dispatch must reach the item-on-item emit")
	}
	if p.MovementQueue().WalkToPending() {
		t.Fatal("item-on-item must not start a walk-to")
	}
}

func TestSpellOnObjectDispatches(t *testing.T) {
	// Spell-on-object routes (phase 45): the opcode-2 dispatch
	// reaches routeSpellOnObject instead of dropping. The test
	// fake's lookup returns the entry for any id/tile, so even
	// the all-zero payload walks to the object — the route acts
	// and a walk starts. The lookup-miss refusal is pinned by
	// TestSpellOnObjectRoute.
	p := testPlayer()
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	deps, _, _ := objectTestDeps(entry, ObjectHooks{})
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpLocT, Payload: zeros(15)}, 1, deps)
	if !got.Implemented {
		t.Fatalf("spell-on-object = %+v, want Implemented (walk to object started)", got)
	}
	if !p.MovementQueue().WalkToPending() {
		t.Fatal("spell-on-object must start a walk-to")
	}
}

func TestHandleItemOnObjectDispatch(t *testing.T) {
	// End-to-end dispatch: the OPLOCU opcode routes through
	// Handle to the item-on-object path. The payload is built
	// with the decoder's field order: slot shortAddLE, object
	// id shortAdd, widget id intLE, modifier byteSub, x
	// shortLE, y short, item id shortAddLE.
	p := testPlayer()
	setInv(p, 0, 995, 1)
	entry := &fakeObjectEntry{id: 100, loc: game.Location{X: 3090, Y: 3524, Z: 0}, typ: 10}
	emitted := false
	hooks := ObjectHooks{
		EmitItemOnObject: func(ev *ItemOnObjectEvent) bool {
			emitted = true
			return true
		},
	}
	deps, _, _ := objectTestDeps(entry, hooks)
	payload := itemOnObjectPayload(0, 100, 3214, 0, 3090, 3524, 995)
	got := Handle(p, game.TurnPacket{Opcode: protocol.NativeOpLocU, Payload: payload}, 1, deps)
	if !got.Implemented {
		t.Fatalf("Handle = %+v, want acted", got)
	}
	tickPlayerQueue(p, 2)
	if !emitted {
		t.Fatal("dispatch must reach the item-on-object emit")
	}
}

// itemOnObjectPayload encodes an OPLOCU payload by inverting
// the decoder's transforms (see internal/protocol/reader.go):
// shortAddLE writes [high, low+128], shortAdd writes
// [low+128, high], intLE writes little-endian, byteSub writes
// 128-value, shortLE writes [low, high], short writes
// [high, low]. The values here keep every transform in range.
func itemOnObjectPayload(slot, objectID, widgetID, mods, x, y, itemID int) []byte {
	var b []byte
	putShortAddLE := func(v int) { b = append(b, byte(v>>8), byte(v+128)) }
	putShortAdd := func(v int) { b = append(b, byte(v+128), byte(v>>8)) }
	putLE16 := func(v int) { b = append(b, byte(v), byte(v>>8)) }
	putBE16 := func(v int) { b = append(b, byte(v>>8), byte(v)) }
	putShortAddLE(slot)                                                                      // slot
	putShortAdd(objectID)                                                                    // object id
	b = append(b, byte(widgetID), byte(widgetID>>8), byte(widgetID>>16), byte(widgetID>>24)) // widget id, intLE
	b = append(b, byte(128-mods))                                                            // modifier flags, byteSub
	putLE16(x)                                                                               // x, shortLE
	putBE16(y)                                                                               // y, short
	putShortAddLE(itemID)                                                                    // item id
	return b
}
