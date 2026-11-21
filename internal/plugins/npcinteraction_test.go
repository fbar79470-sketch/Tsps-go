package plugins

import "testing"

import "tsps-go/internal/game"

// NPC-click tests: the onNpcClick registration, the
// emitNpcInteraction emit, and the Api passthrough in
// npcinteraction.go, grounded in PluginManager.ts
// (registerNpcClickHook 2944-2980, emitNpcInteraction
// 1246-1263, onNpcClick 4042-4044, emitNpcInteraction
// 4695) and PluginTypes.ts (1167, 1403, 160-168).
// Fixtures only: newTestHub and testPlayer come from
// api_test.go / hooks_test.go.

func TestOnNpcClickMatchesIDAndType(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ClickWatch")
	calls := 0
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		calls++
		return true
	})
	player := testPlayer("alice")
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	npc.SetNpcTransformationID(9474)
	// The trunk id at click type 1 claims.
	event := &NpcInteractionEvent{Player: player, NPC: npc, NpcID: npc.ContentID(), ClickType: 1, Location: npc.Location()}
	if !h.EmitNpcInteraction(event) {
		t.Fatal("trunk click: want handled")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !event.Handled {
		t.Error("want the event marked handled")
	}
	// Another NPC id does not match.
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1234, ClickType: 1, Location: npc.Location()}) {
		t.Error("other NPC id: want unhandled")
	}
	// Another click type does not match.
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 2, Location: npc.Location()}) {
		t.Error("click type 2: want unhandled")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 after non-matching emits", calls)
	}
}

func TestOnNpcClickShortCircuits(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ClickWatch")
	first, second := 0, 0
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		first++
		return true
	})
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		second++
		return true
	})
	player := testPlayer("alice")
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Location: npc.Location()}) {
		t.Fatal("want handled")
	}
	if first != 1 || second != 0 {
		t.Errorf("first=%d second=%d, want 1/0 (short-circuit)", first, second)
	}
}

func TestOnNpcClickFallthrough(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ClickWatch")
	calls := 0
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		calls++
		return false
	})
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		calls++
		return true
	})
	player := testPlayer("alice")
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Location: npc.Location()}) {
		t.Fatal("want handled by the second hook")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (fall-through)", calls)
	}
}

func TestOnNpcClickInvalidRegistration(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ClickWatch")
	// No valid ids, a bad click type, and a nil handler all
	// register nothing (the TS registerNpcClickHook guard).
	a.OnNpcClick([]int{-1}, 1, func(*NpcInteractionEvent) bool { return true })
	a.OnNpcClick([]int{9474}, 0, func(*NpcInteractionEvent) bool { return true })
	a.OnNpcClick([]int{9474}, 6, func(*NpcInteractionEvent) bool { return true })
	a.OnNpcClick([]int{9474}, 1, nil)
	player := testPlayer("alice")
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Location: npc.Location()}) {
		t.Error("invalid registrations: want unhandled")
	}
	// Mixed valid/invalid ids keep the valid ones.
	a.OnNpcClick([]int{-1, 9474}, 1, func(*NpcInteractionEvent) bool { return true })
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Location: npc.Location()}) {
		t.Error("mixed ids: want handled (valid id kept)")
	}
}

func TestEmitNpcInteractionGuards(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "ClickWatch")
	calls := 0
	a.OnNpcClick([]int{9474}, 1, func(*NpcInteractionEvent) bool {
		calls++
		return true
	})
	player := testPlayer("alice")
	npc := game.NewNPC(7234, game.Location{X: 3200, Y: 3200, Z: 0})
	// Nil event / player / NPC run no hooks.
	if h.EmitNpcInteraction(nil) {
		t.Error("nil event: want false")
	}
	if h.EmitNpcInteraction(&NpcInteractionEvent{NPC: npc, NpcID: 9474, ClickType: 1}) {
		t.Error("nil player: want false")
	}
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NpcID: 9474, ClickType: 1}) {
		t.Error("nil NPC: want false")
	}
	// A pre-handled event runs no hooks and returns true.
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Handled: true}) {
		t.Error("pre-handled: want true")
	}
	if calls != 0 {
		t.Errorf("calls = %d, want 0", calls)
	}
	// The Api passthrough delegates to the hub emit.
	if !a.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 9474, ClickType: 1, Location: npc.Location()}) {
		t.Error("Api.EmitNpcInteraction: want handled")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if a.EmitNpcInteraction(nil) {
		t.Error("Api nil event: want false")
	}
}

// NPC-interaction tests: the onNpcInteraction registration and
// its position after the onNpcClick bindings in
// npcinteraction.go, grounded in PluginManager.ts
// (registerNpcActions 3155-3180, onNpcInteraction 3488-3499,
// emitNpcInteraction 1246-1263) and PluginTypes.ts (167-176).

func TestOnNpcInteractionMatchesNameAndAction(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "SpotWatch")
	calls := 0
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool {
			calls++
			return true
		},
	})
	player := testPlayer("alice")
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	spot := &NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}
	if !h.EmitNpcInteraction(spot) {
		t.Fatal("matching name + action: want handled")
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want 1", calls)
	}
	if !spot.Handled {
		t.Error("want the event marked handled")
	}
	// Another definition name does not match.
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Rod Fishing spot", Action: "Small Net"}) {
		t.Error("other name: want unhandled")
	}
	// Another option does not match.
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 2, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Bait"}) {
		t.Error("other action: want unhandled")
	}
	// A click type outside 1-5 never reaches the action.
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 6, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}) {
		t.Error("click type 6: want unhandled")
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1 after non-matching emits", calls)
	}
}

func TestOnNpcInteractionFallthrough(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "SpotWatch")
	calls := 0
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool {
			calls++
			return false
		},
	})
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool {
			calls++
			return true
		},
	})
	player := testPlayer("alice")
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}) {
		t.Fatal("want handled by the second hook")
	}
	if calls != 2 {
		t.Errorf("calls = %d, want 2 (fall-through)", calls)
	}
}

func TestOnNpcInteractionClickHooksFirst(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "SpotWatch")
	clickCalls, nameCalls := 0, 0
	a.OnNpcClick([]int{1530}, 1, func(*NpcInteractionEvent) bool {
		clickCalls++
		return true
	})
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool {
			nameCalls++
			return true
		},
	})
	player := testPlayer("alice")
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	event := &NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}
	if !h.EmitNpcInteraction(event) {
		t.Fatal("want handled")
	}
	if clickCalls != 1 || nameCalls != 0 {
		t.Errorf("click=%d name=%d, want 1/0 (click claims first)", clickCalls, nameCalls)
	}

	// A click hook that falls through lets the name hook claim.
	h2, _ := newTestHub()
	b := newApi(h2, "SpotWatch")
	clickCalls, nameCalls = 0, 0
	b.OnNpcClick([]int{1530}, 1, func(*NpcInteractionEvent) bool {
		clickCalls++
		return false
	})
	b.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool {
			nameCalls++
			return true
		},
	})
	if !h2.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}) {
		t.Fatal("fall-through: want handled by the name hook")
	}
	if clickCalls != 1 || nameCalls != 1 {
		t.Errorf("click=%d name=%d, want 1/1 (fall-through to name hook)", clickCalls, nameCalls)
	}
}

func TestOnNpcInteractionInvalidRegistration(t *testing.T) {
	h, _ := newTestHub()
	a := newApi(h, "SpotWatch")
	// Empty name, a nil map, and an all-nil actions map all
	// register nothing (the TS registerNpcActions guards).
	a.OnNpcInteraction("", map[string]NpcInteractionHandler{
		"Small Net": func(*NpcInteractionEvent) bool { return true },
	})
	a.OnNpcInteraction("Fishing spot", nil)
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": nil,
	})
	player := testPlayer("alice")
	npc := game.NewNPC(1530, game.Location{X: 3240, Y: 3146, Z: 0})
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}) {
		t.Error("invalid registrations: want unhandled")
	}
	// A mixed map keeps the function-valued actions.
	a.OnNpcInteraction("Fishing spot", map[string]NpcInteractionHandler{
		"Small Net": nil,
		"Bait":      func(*NpcInteractionEvent) bool { return true },
	})
	if !h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 2, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Bait"}) {
		t.Error("mixed actions: want handled (valid action kept)")
	}
	if h.EmitNpcInteraction(&NpcInteractionEvent{Player: player, NPC: npc, NpcID: 1530, ClickType: 1, Location: npc.Location(), DefinitionName: "Fishing spot", Action: "Small Net"}) {
		t.Error("dropped nil action: want unhandled")
	}
}
