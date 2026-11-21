package plugins

import (
	"testing"
)

// Item-on-item family tests: the registration methods and emit
// in itemonitem.go, grounded in PluginManager.ts
// (emitItemOnItem 1925-1930, onItemOnItem 4107-4144) and
// PluginTypes.ts (PluginItemOnItemEvent 722-731, overloads
// 1197-1204). Fixtures only.

func TestEmitItemOnItemPlain(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	var order []string
	api.OnItemOnItem(func(ev *PluginItemOnItemEvent) {
		order = append(order, "main-1")
	}, nil)
	api.OnItemOnItem(func(ev *PluginItemOnItemEvent) {
		order = append(order, "main-2")
		ev.Handled = true
	}, nil)
	// A plain handler after a claim is skipped by the wrapper.
	api.OnItemOnItem(func(ev *PluginItemOnItemEvent) {
		order = append(order, "main-3")
	}, nil)
	ev := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 23962, UsedItemSlot: 0,
		UsedWithItemID: 23673, UsedWithItemSlot: 1,
	}
	if !h.EmitItemOnItem(ev) {
		t.Fatal("a claiming plain hook must make the emit return true")
	}
	if len(order) != 2 || order[0] != "main-1" || order[1] != "main-2" {
		t.Fatalf("order = %v, want [main-1 main-2]", order)
	}
	// Unhandled uses fall through silently: no claim, no error.
	h2, _ := newTestHub()
	api2 := newApi(h2, "P")
	ran := false
	api2.OnItemOnItem(func(ev *PluginItemOnItemEvent) { ran = true }, nil)
	ev2 := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 995, UsedItemSlot: 0,
		UsedWithItemID: 996, UsedWithItemSlot: 1,
	}
	if h2.EmitItemOnItem(ev2) || !ran {
		t.Fatalf("unhandled emit must return false after running hooks: handled=%v ran=%v", ev2.Handled, ran)
	}
	// Nil guards: a nil hub, a nil event, and a nil handler run nothing.
	var nilHub *Hub
	if nilHub.EmitItemOnItem(ev2) {
		t.Fatal("nil Hub emit must return false")
	}
	if h.EmitItemOnItem(nil) {
		t.Fatal("nil event emit must return false")
	}
	api.OnItemOnItem(nil, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe", nil, nil)
}

func TestOnItemOnItemNamedMatching(t *testing.T) {
	h, _ := newTestHub()
	h.SetItemResolver(func(id int) (string, bool) {
		switch id {
		case 23962:
			return "Crystal shard", false
		case 23673:
			return "Crystal axe", false
		default:
			return "Coins", false
		}
	})
	api := newApi(h, "P")
	ran := false
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe", func(ev *PluginItemOnItemEvent) bool {
		ran = true
		return false // Fall through: must not claim.
	}, nil)
	ev := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 23962, UsedItemSlot: 0,
		UsedWithItemID: 23673, UsedWithItemSlot: 1,
	}
	// Either order matches, and the event keeps its original order.
	if h.EmitItemOnItem(ev) || !ran {
		t.Fatalf("exact name match must run the hook without claiming: handled=%v ran=%v", ev.Handled, ran)
	}
	if ev.UsedItemID != 23962 || ev.UsedWithItemID != 23673 {
		t.Fatalf("event order changed: used=%d with=%d, want 23962/23673", ev.UsedItemID, ev.UsedWithItemID)
	}
	ran = false
	ev.UsedItemID, ev.UsedWithItemID = 23673, 23962 // Reversed order.
	h.EmitItemOnItem(ev)
	if !ran {
		t.Fatal("the reversed order must also match a named hook")
	}
	// A name mismatch never matches.
	h.SetItemResolver(func(id int) (string, bool) { return "Coins", false })
	ran = false
	h.EmitItemOnItem(ev)
	if ran {
		t.Fatal("an item-name mismatch must not run a named hook")
	}
	// A claiming named handler sets Handled.
	h.SetItemResolver(func(id int) (string, bool) {
		if id == 23962 {
			return "Crystal shard", false
		}
		return "Crystal axe", false
	})
	api2 := newApi(h, "Q")
	api2.OnItemOnItemNamed("Crystal shard", "Crystal axe", func(ev *PluginItemOnItemEvent) bool {
		return true
	}, nil)
	ev2 := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 23962, UsedItemSlot: 0,
		UsedWithItemID: 23673, UsedWithItemSlot: 1,
	}
	if !h.EmitItemOnItem(ev2) {
		t.Fatal("a claiming named hook must make the emit return true")
	}
}

func TestOnItemOnItemNotedFilter(t *testing.T) {
	h, _ := newTestHub()
	h.SetItemResolver(func(id int) (string, bool) {
		// 23963 is the noted crystal shard; 23962 is unnoted.
		switch id {
		case 23962:
			return "Crystal shard", false
		case 23963:
			return "Crystal shard", true
		default:
			return "Crystal axe", false
		}
	})
	api := newApi(h, "P")
	ran := false
	noted := false
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe", func(ev *PluginItemOnItemEvent) bool {
		ran = true
		return true
	}, &ItemUseFilter{Noted: &noted})
	ev := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 23962, UsedItemSlot: 0,
		UsedWithItemID: 23673, UsedWithItemSlot: 1,
	}
	if !h.EmitItemOnItem(ev) || !ran {
		t.Fatalf("unnoted pair must match the noted:false filter: handled=%v ran=%v", ev.Handled, ran)
	}
	// A noted shard fails the same filter (TS checks both ids).
	ran = false
	ev.UsedItemID = 23963
	ev.Handled = false
	if h.EmitItemOnItem(ev) || ran {
		t.Fatalf("noted shard must skip the noted:false filter: handled=%v ran=%v", ev.Handled, ran)
	}
	// The plain form applies the same both-ids filter (fresh hub,
	// so no named hook claims the use first).
	h3, _ := newTestHub()
	h3.SetItemResolver(func(id int) (string, bool) {
		if id == 23963 {
			return "Crystal shard", true
		}
		return "Crystal axe", false
	})
	api3 := newApi(h3, "Q")
	ranPlain := false
	api3.OnItemOnItem(func(ev *PluginItemOnItemEvent) { ranPlain = true }, &ItemUseFilter{Noted: &noted})
	ev3 := &PluginItemOnItemEvent{
		Player: testObjectPlayer(), UsedItemID: 23963, UsedItemSlot: 0,
		UsedWithItemID: 23673, UsedWithItemSlot: 1,
	}
	if h3.EmitItemOnItem(ev3) || ranPlain {
		t.Fatalf("noted shard must skip a plain noted:false hook: handled=%v ranPlain=%v", ev3.Handled, ranPlain)
	}
	ev3.UsedItemID = 23962
	ev3.Handled = false
	h3.EmitItemOnItem(ev3)
	if !ranPlain {
		t.Fatal("unnoted pair must run a plain noted:false hook")
	}
}

func TestEmitItemOnItemNilPlayerGuard(t *testing.T) {
	h, _ := newTestHub()
	api := newApi(h, "P")
	ran := false
	api.OnItemOnItem(func(ev *PluginItemOnItemEvent) { ran = true }, nil)
	api.OnItemOnItemNamed("Crystal shard", "Crystal axe", func(ev *PluginItemOnItemEvent) bool {
		ran = true
		return true
	}, nil)
	ev := &PluginItemOnItemEvent{UsedItemID: 23962, UsedWithItemID: 23673}
	if h.EmitItemOnItem(ev) || ran {
		t.Fatalf("a nil-player event must run no hooks: handled=%v ran=%v", ev.Handled, ran)
	}
}
