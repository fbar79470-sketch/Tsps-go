package listeners

import (
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// fakeExamineSource is an ExamineSource test double returning
// canned text per kind, recording the ids it was asked for.
type fakeExamineSource struct {
	npcText, objText, itemText string
	npcIDs, objIDs, itemIDs    []int
}

func (f *fakeExamineSource) NpcExamine(id int) string {
	f.npcIDs = append(f.npcIDs, id)
	return f.npcText
}
func (f *fakeExamineSource) ObjectExamine(id int) string {
	f.objIDs = append(f.objIDs, id)
	return f.objText
}
func (f *fakeExamineSource) ItemExamine(id int) string {
	f.itemIDs = append(f.itemIDs, id)
	return f.itemText
}

var _ ExamineSource = (*fakeExamineSource)(nil)

// TestHandleExamine pins the dispatch-owned examine cases
// (NetworkBuilder.ts:1283-1306): typed messages with the examine
// chat types, the object-examine intercept, and the unwired,
// nil-player, and malformed drops.
func TestHandleExamine(t *testing.T) {
	// shortAdd encoding of id 291 for opcode 9; shortAddLE for 85.
	npcPayload := []byte{0xA3, 0x01}
	locPayload := []byte{0x01, 0xA3}

	t.Run("npc examine sends typed message", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &fakeExamineSource{npcText: "A fearsome beast."}
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeExamineNpc, Payload: npcPayload}, 7, Deps{Examine: src})
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("Handle = %+v", got)
		}
		if len(sess.typedMessages) != 1 || sess.typedMessages[0] != [3]any{protocol.ChatTypeNPCExamine, "A fearsome beast.", ""} {
			t.Errorf("typed messages = %v", sess.typedMessages)
		}
		if len(src.npcIDs) != 1 || src.npcIDs[0] != 291 {
			t.Errorf("npc ids = %v, want [291]", src.npcIDs)
		}
	})

	t.Run("object examine sends typed message", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &fakeExamineSource{objText: "It's nothing special."}
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeExamineLoc, Payload: locPayload}, 7, Deps{Examine: src})
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("Handle = %+v", got)
		}
		if len(sess.typedMessages) != 1 || sess.typedMessages[0] != [3]any{protocol.ChatTypeLocExamine, "It's nothing special.", ""} {
			t.Errorf("typed messages = %v", sess.typedMessages)
		}
	})

	t.Run("handled object-examine event suppresses the answer", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &fakeExamineSource{objText: "unseen"}
		intercepted := false
		deps := Deps{Examine: src, ObjectExamineIntercept: func(pl *game.Player, id int) bool {
			intercepted = true
			if id != 291 {
				t.Errorf("intercept id = %d, want 291", id)
			}
			return true
		}}
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeExamineLoc, Payload: locPayload}, 7, deps)
		if got != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
			t.Fatalf("Handle = %+v", got)
		}
		if !intercepted {
			t.Error("intercept was not called")
		}
		if len(sess.typedMessages) != 0 {
			t.Errorf("typed messages = %v, want none", sess.typedMessages)
		}
		if len(src.objIDs) != 0 {
			t.Errorf("source was consulted: %v", src.objIDs)
		}
	})

	t.Run("unhandled intercept proceeds", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &fakeExamineSource{objText: "seen"}
		deps := Deps{Examine: src, ObjectExamineIntercept: func(*game.Player, int) bool { return false }}
		got := Handle(p, game.TurnPacket{Opcode: protocol.NativeExamineLoc, Payload: locPayload}, 7, deps)
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("Handle = %+v", got)
		}
		if len(sess.typedMessages) != 1 {
			t.Errorf("typed messages = %v, want one", sess.typedMessages)
		}
	})

	t.Run("unwired source drops", func(t *testing.T) {
		p := testPlayer()
		for _, opcode := range []byte{protocol.NativeExamineNpc, protocol.NativeExamineLoc} {
			payload := npcPayload
			if opcode == protocol.NativeExamineLoc {
				payload = locPayload
			}
			if got := Handle(p, game.TurnPacket{Opcode: opcode, Payload: payload}, 7, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonExamineUnwired}) {
				t.Errorf("opcode %d: Handle = %+v", opcode, got)
			}
		}
	})

	t.Run("nil player drops after decode", func(t *testing.T) {
		if got := Handle(nil, game.TurnPacket{Opcode: protocol.NativeExamineNpc, Payload: npcPayload}, 7, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
			t.Errorf("Handle = %+v", got)
		}
	})

	t.Run("malformed payload drops", func(t *testing.T) {
		p := testPlayer()
		if got := Handle(p, game.TurnPacket{Opcode: protocol.NativeExamineNpc, Payload: []byte{0xA3}}, 7, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
			t.Errorf("Handle = %+v", got)
		}
	})
}

// TestExamineItemRoutes pins examineItem through the inventory,
// ground-item, and worn-item routes: the text is sent, and with
// a price-less source no price line follows.
func TestExamineItemRoutes(t *testing.T) {
	t.Run("inventory examine sends the text", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setInv(p, 0, itemBronzeSword, 1)
		src := &fakeExamineSource{itemText: "A bronze sword."}
		got := Handle(p, game.TurnPacket{Opcode: protocol.ClientInventoryUse, Payload: inventoryUsePayload(0, itemBronzeSword, "Examine")}, 7, Deps{GroundItems: newManagerStore(), Examine: src})
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("Handle = %+v", got)
		}
		if len(sess.messages) != 1 || sess.messages[0] != "A bronze sword." {
			t.Errorf("messages = %v", sess.messages)
		}
	})

	t.Run("ground examine sends the text without resolving the stack", func(t *testing.T) {
		p, sess := messageTestPlayer()
		src := &fakeExamineSource{itemText: "On the ground."}
		// No ground-item store at all: the examine needs no
		// stack resolution (the amount feeds only the price
		// line), so it answers with Deps.Examine alone.
		msg := protocol.GroundItemAction{ItemID: itemPlain, X: 3089, Y: 3524, Option: "Examine", HasOption: true}
		got := routeGroundItemAction(p, msg, 7, Deps{Examine: src},
			func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} },
			func() Outcome { return Outcome{Recognized: true, Implemented: true} },
			func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} })
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route = %+v", got)
		}
		if len(sess.messages) != 1 || sess.messages[0] != "On the ground." {
			t.Errorf("messages = %v", sess.messages)
		}
	})

	t.Run("worn op10 examines", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setEq(p, game.WeaponSlot, itemBronzeSword, 1)
		src := &fakeExamineSource{itemText: "Worn and sharp."}
		msg := protocol.WidgetAction{WidgetID: int32(387<<16 | 18), GroupID: 387, ChildID: 18, ButtonNum: 10, HasButtonNum: true}
		got := routeWidgetAction(p, msg, Deps{Examine: src},
			func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} },
			func() Outcome { return Outcome{Recognized: true, Implemented: true} },
			func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} })
		if got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("route = %+v", got)
		}
		if len(sess.messages) != 1 || sess.messages[0] != "Worn and sharp." {
			t.Errorf("messages = %v", sess.messages)
		}
	})
}

// TestBackpackWarnOnce pins the unresolved-inventory-action message
// (NetworkBuilder.ts:1093-1101): sent once per connection, only
// for the main-inventory widget uid.
func TestBackpackWarnOnce(t *testing.T) {
	route := func(p *game.Player, msg protocol.WidgetAction, deps Deps) Outcome {
		return routeWidgetAction(p, msg, deps,
			func(r DropReason) Outcome { return Outcome{Recognized: true, Reason: r} },
			func() Outcome { return Outcome{Recognized: true, Implemented: true} },
			func() Outcome { return Outcome{Recognized: true, Reason: ReasonNoAction} })
	}
	want := "That backpack action is unavailable. If your client was open during an update, reload and try again."

	t.Run("first unresolved click warns, second drops silently", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setInv(p, 0, itemBronzeSword, 1)
		warned := false
		deps := Deps{UnresolvedInventoryWarned: &warned}
		// Button 3 resolves to the empty second action.
		msg := protocol.WidgetAction{WidgetID: protocol.MainInventoryWidgetUID, GroupID: 149, ChildID: 0, ButtonNum: 3, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, deps); got != (Outcome{Recognized: true, Implemented: true}) {
			t.Fatalf("first route = %+v", got)
		}
		if !warned {
			t.Error("warned flag not set")
		}
		if len(sess.messages) != 1 || sess.messages[0] != want {
			t.Fatalf("messages = %v", sess.messages)
		}
		if got := route(p, msg, deps); got != (Outcome{Recognized: true, Reason: ReasonWidgetAction}) {
			t.Errorf("second route = %+v", got)
		}
		if len(sess.messages) != 1 {
			t.Errorf("messages = %v, want exactly one", sess.messages)
		}
	})

	t.Run("other widget ids never warn", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setInv(p, 0, itemBronzeSword, 1)
		warned := false
		deps := Deps{UnresolvedInventoryWarned: &warned}
		msg := protocol.WidgetAction{WidgetID: int32(149<<16 | 1), GroupID: 149, ChildID: 1, ButtonNum: 3, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, deps); got != (Outcome{Recognized: true, Reason: ReasonWidgetAction}) {
			t.Errorf("route = %+v", got)
		}
		if warned || len(sess.messages) != 0 {
			t.Errorf("warned = %v, messages = %v", warned, sess.messages)
		}
	})

	t.Run("nil flag keeps the old drop", func(t *testing.T) {
		p, sess := messageTestPlayer()
		setInv(p, 0, itemBronzeSword, 1)
		msg := protocol.WidgetAction{WidgetID: protocol.MainInventoryWidgetUID, GroupID: 149, ChildID: 0, ButtonNum: 3, HasButtonNum: true, Slot: 0, ItemID: itemBronzeSword}
		if got := route(p, msg, Deps{}); got != (Outcome{Recognized: true, Reason: ReasonWidgetAction}) {
			t.Errorf("route = %+v", got)
		}
		if len(sess.messages) != 0 {
			t.Errorf("messages = %v, want none", sess.messages)
		}
	})
}
