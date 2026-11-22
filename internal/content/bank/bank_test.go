package bank

import (
	"bytes"
	"log"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/plugins"
)

// fakeObjectDef is a plugins.ObjectDefinition over an explicit
// name and interaction list, for open-path tests (the
// wildernessditch fakeObjectDef precedent).
type fakeObjectDef struct {
	name         string
	interactions []string
}

func (d fakeObjectDef) ID() int                { return 10355 }
func (d fakeObjectDef) Name() string           { return d.name }
func (d fakeObjectDef) SizeX() int             { return 1 }
func (d fakeObjectDef) SizeY() int             { return 1 }
func (d fakeObjectDef) BlockingMask() int      { return 0 }
func (d fakeObjectDef) Interactions() []string { return d.interactions }

// testHub builds a hub with only BankBooths loaded (the
// shopkeepers testHub precedent).
func testHub(t *testing.T) *plugins.Hub {
	t.Helper()
	var buf bytes.Buffer
	hub := plugins.NewHub([]plugins.Plugin{Plugin}, plugins.HubOptions{Logger: log.New(&buf, "", 0)})
	hub.Load()
	if len(hub.Loaded()) != 1 || hub.Loaded()[0] != "BankBooths" {
		t.Fatalf("loaded = %v, want [BankBooths] (log: %s)", hub.Loaded(), buf.String())
	}
	return hub
}

// objectEvent builds a named-object click: the definition's
// interaction at clickType is the clicked option (the
// OnObjectInteractionNamed wrapper reads it that way).
func objectEvent(p *game.Player, def fakeObjectDef, clickType int) *plugins.ObjectInteractionEvent {
	return &plugins.ObjectInteractionEvent{
		Player:     p,
		Object:     game.NewGameObject(10355, p.Location(), 10, 0, nil),
		Definition: def,
		ObjectID:   10355,
		ClickType:  clickType,
		Location:   p.Location(),
	}
}

// npcEvent builds a Banker click. EmitNpcInteraction requires a
// live NPC and matches on DefinitionName + Action.
func npcEvent(p *game.Player, action string, clickType int) *plugins.NpcInteractionEvent {
	return &plugins.NpcInteractionEvent{
		Player:         p,
		NPC:            game.NewNPC(1618, game.Location{X: 3090, Y: 3243, Z: 0}),
		NpcID:          1618,
		ClickType:      clickType,
		DefinitionName: "Banker",
		Action:         action,
		Actions:        []string{"Talk-to", "", "Bank", "Collect", ""},
	}
}

// installStubFlows installs recording Worker B stubs and
// returns the call counters; the caller restores the zero
// Flows when done.
func installStubFlows(t *testing.T) (opens *int, buttons *[][2]int, widgets *int) {
	t.Helper()
	opens = new(int)
	buttons = new([][2]int)
	widgets = new(int)
	InstallFlows(Flows{
		// The stub IsOpen is the landed seam itself
		// (game.BankIsOpen, Worker A): the tests exercise the
		// open gate, not the stub.
		IsOpen: func(p *game.Player) bool { return game.BankIsOpen(p) },
		Open: func(p *game.Player) bool {
			*opens++
			return true
		},
		HandleButton: func(p *game.Player, buttonID, action int) bool {
			*buttons = append(*buttons, [2]int{buttonID, action})
			return true
		},
		HandleWidget: func(ev *plugins.InterfaceActionClickEvent) bool {
			*widgets++
			return true
		},
	})
	t.Cleanup(func() { InstallFlows(Flows{}) })
	return opens, buttons, widgets
}

func newTestPlayer(name string) *game.Player {
	return game.NewPlayer(name, game.Location{X: 3093, Y: 3243, Z: 0})
}

func TestRegisterBoothBankAndUseOpen(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	cycle := int64(5)
	hub.SetCycleSource(func() int64 { return cycle })
	p := newTestPlayer("alice")
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "Use", "", ""}}
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("Bank booth Bank (clickType 2): want handled")
	}
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
	// Advance a tick: the second click is a fresh open, not a
	// debounce swallow (see TestOpenBankDebounceOneTick).
	cycle = 6
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 3)) {
		t.Fatal("Bank booth Use (clickType 3): want handled")
	}
	if *opens != 2 {
		t.Fatalf("opens = %d, want 2", *opens)
	}
}

func TestRegisterChestUseOpens(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	cycle := int64(5)
	hub.SetCycleSource(func() int64 { return cycle })
	p := newTestPlayer("alice")
	for _, name := range []string{"Bank chest", "Bank Chest"} {
		chest := fakeObjectDef{name: name, interactions: []string{"Use", "", "Collect", "", ""}}
		if !hub.EmitObjectInteraction(objectEvent(p, chest, 1)) {
			t.Fatalf("%s Use (clickType 1): want handled", name)
		}
		// Advance a tick between opens (debounce).
		cycle++
	}
	if *opens != 2 {
		t.Fatalf("opens = %d, want 2", *opens)
	}
}

func TestRegisterWrongNameOrOptionDoesNotOpen(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	// Wrong object name: the definition says "Bank booth" but
	// the click is the Collect slot, which no binding owns.
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "Collect", "", ""}}
	if hub.EmitObjectInteraction(objectEvent(p, booth, 3)) {
		t.Error("Bank booth Collect: want unhandled")
	}
	// Right option text, wrong object name entirely.
	other := fakeObjectDef{name: "Altar", interactions: []string{"", "Bank", "", "", ""}}
	if hub.EmitObjectInteraction(objectEvent(p, other, 2)) {
		t.Error("Altar Bank: want unhandled")
	}
	if *opens != 0 {
		t.Fatalf("opens = %d, want 0", *opens)
	}
}

func TestRegisterBankerBankOpens(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	if !hub.EmitNpcInteraction(npcEvent(p, "Bank", 3)) {
		t.Fatal("Banker Bank (clickType 3): want handled")
	}
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
}

func TestRegisterBankerTalkToEmitsDialogueStart(t *testing.T) {
	hub := testHub(t)
	var got *game.StartNamedRequest
	// Stand in for the coordinator's playback subscription
	// (SubscribeDialogueStart): claim the request, as the
	// playback does when a transcript plays. Without a
	// subscriber talkToBanker returns false — the TS behaviour
	// when nothing plays (request.handled stays false) — so
	// the claim here is what makes the click handled.
	hub.SubscribeCustomEvent(plugins.NpcDialogueStartEventName, func(payload any) {
		got, _ = payload.(*game.StartNamedRequest)
		if got != nil {
			got.Handled = true
		}
	})
	p := newTestPlayer("alice")
	if !hub.EmitNpcInteraction(npcEvent(p, "Talk-to", 1)) {
		t.Fatal("Banker Talk-to (clickType 1): want handled")
	}
	if got == nil {
		t.Fatal("Talk-to: want an npc-dialogue:start request emitted")
	}
	if got.Variant != "standard-dialogue" || !got.HasVariant {
		t.Fatalf("request variant = %q (hasVariant=%v), want \"standard-dialogue\" (true)", got.Variant, got.HasVariant)
	}
	if got.Player != p || got.DefinitionName != "Banker" || got.NPCID != 1618 {
		t.Fatalf("request = player=%v name=%q npcId=%d, want the clicked banker", got.Player == p, got.DefinitionName, got.NPCID)
	}
}

func TestOpenBankDebounceOneTick(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	cycle := int64(5)
	hub.SetCycleSource(func() int64 { return cycle })
	p := newTestPlayer("alice")
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "", "", ""}}
	// First open at tick 5 runs the flow.
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("first open: want handled")
	}
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
	// Second open on the same tick is debounced: claimed, but
	// the flow does not run again.
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("debounced open: want handled")
	}
	if *opens != 1 {
		t.Fatalf("opens after debounced retry = %d, want 1", *opens)
	}
	// One tick later the debounce has expired and the flow runs.
	cycle = 6
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("open one tick later: want handled")
	}
	if *opens != 2 {
		t.Fatalf("opens = %d, want 2", *opens)
	}
}

func TestOpenBankAlreadyOpenSkipsFlow(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	// BankIsOpen is the §4 contract: status BANKING and
	// interface 12, both set here.
	p.SetStatus(game.PlayerStatusBanking)
	p.SetInterfaceId(game.BankMainInterfaceID)
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "", "", ""}}
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("open while bank open: want handled")
	}
	if *opens != 0 {
		t.Fatalf("opens = %d, want 0 (already open skips the flow)", *opens)
	}
}

func TestOpenBankBotShortcut(t *testing.T) {
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("bot")
	p.IsBot = true
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "", "", ""}}
	if !hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Fatal("bot open: want handled")
	}
	if *opens != 0 {
		t.Fatalf("opens = %d, want 0 (bots skip the flow)", *opens)
	}
	if got := p.GetInterfaceId(); got != game.BankMainInterfaceID {
		t.Fatalf("bot interface id = %d, want %d", got, game.BankMainInterfaceID)
	}
	if got := p.Status(); got != game.PlayerStatusBanking {
		t.Fatalf("bot status = %v, want BANKING", got)
	}
}

func TestOpenBankNoFlowFallsThrough(t *testing.T) {
	// With no Worker B flow installed (the pre-integration
	// state) the click is NOT claimed: nothing can open the
	// bank yet, so swallowing the click would hide the gap.
	hub := testHub(t)
	p := newTestPlayer("alice")
	booth := fakeObjectDef{name: "Bank booth", interactions: []string{"", "Bank", "", "", ""}}
	if hub.EmitObjectInteraction(objectEvent(p, booth, 2)) {
		t.Error("open with no Open flow: want unhandled (fall through)")
	}
}

func TestOpenBankNilPlayer(t *testing.T) {
	if openBank(nil, nil) {
		t.Error("openBank(nil, nil): want false")
	}
	if HandleBankButton(nil, 50013) {
		t.Error("HandleBankButton(nil, ...): want false")
	}
	if HandleBankInterfaceAction(nil, 50013, 0) {
		t.Error("HandleBankInterfaceAction(nil, ...): want false")
	}
}

func TestHandleBankButtonGating(t *testing.T) {
	_, buttons, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	p.SetInterfaceId(game.BankMainInterfaceID)
	// Every main button id routes with action 0.
	for _, id := range []int{50013, 5386, 5387, 8130, 8131, 50004, 50007, 5384, 50001, 50010} {
		*buttons = nil
		if !HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 12, %d): want true", id)
		}
		if len(*buttons) != 1 || (*buttons)[0] != [2]int{id, 0} {
			t.Errorf("HandleBankButton(interface 12, %d): flow got %v, want [[%d 0]]", id, *buttons, id)
		}
	}
	// Tab-select: 50070 + 4*tab, tabs 0..10.
	for tab := 0; tab <= 10; tab++ {
		*buttons = nil
		id := 50070 + 4*tab
		if !HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 12, %d tab %d): want true", id, tab)
		}
		if len(*buttons) != 1 || (*buttons)[0] != [2]int{id, 0} {
			t.Errorf("HandleBankButton(interface 12, %d): flow got %v, want [[%d 0]]", id, *buttons, id)
		}
	}
	// Not tab-select: off-stride and tab 11.
	for _, id := range []int{50071, 50072, 50073, 50114, 50118, 49999} {
		if HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 12, %d): want false", id)
		}
	}
	// Settings ids are not main-interface buttons.
	for _, id := range []int{32503, 32512, 32513} {
		if HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 12, %d): want false", id)
		}
	}
	// Settings interface: only the settings ids route.
	p.SetInterfaceId(BankSettingsInterfaceID)
	for _, id := range []int{32503, 32512, 32513} {
		*buttons = nil
		if !HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 32500, %d): want true", id)
		}
		if len(*buttons) != 1 || (*buttons)[0] != [2]int{id, 0} {
			t.Errorf("HandleBankButton(interface 32500, %d): flow got %v, want [[%d 0]]", id, *buttons, id)
		}
	}
	for _, id := range []int{50013, 5386, 50070, 50010} {
		if HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface 32500, %d): want false", id)
		}
	}
	// No bank interface open: nothing routes.
	p.SetInterfaceId(-1)
	for _, id := range []int{50013, 32503, 50070} {
		if HandleBankButton(p, id) {
			t.Errorf("HandleBankButton(interface -1, %d): want false", id)
		}
	}
}

func TestHandleBankButtonNoFlowFallsThrough(t *testing.T) {
	// Gated in, but no Worker B flow: fall through, don't claim.
	p := newTestPlayer("alice")
	p.SetInterfaceId(game.BankMainInterfaceID)
	if HandleBankButton(p, 50013) {
		t.Error("HandleBankButton with no HandleButton flow: want false")
	}
}

func TestButtonClickEmitRoutesThroughHub(t *testing.T) {
	// The registered onButtonClick handler (TS bankButton):
	// a bank button on the open bank is claimed with action 0.
	hub := testHub(t)
	_, buttons, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	p.SetInterfaceId(game.BankMainInterfaceID)
	if !hub.EmitButtonClick(&plugins.ButtonClickEvent{Player: p, ButtonID: 50013}) {
		t.Fatal("button click 50013 on open bank: want handled")
	}
	if len(*buttons) != 1 || (*buttons)[0] != [2]int{50013, 0} {
		t.Fatalf("flow got %v, want [[50013 0]]", *buttons)
	}
	// Bank closed: the same click falls through.
	p.SetInterfaceId(-1)
	if hub.EmitButtonClick(&plugins.ButtonClickEvent{Player: p, ButtonID: 50013}) {
		t.Error("button click 50013 with bank closed: want unhandled")
	}
	// Not a bank button: falls through even with the bank open.
	p.SetInterfaceId(game.BankMainInterfaceID)
	if hub.EmitButtonClick(&plugins.ButtonClickEvent{Player: p, ButtonID: 12345}) {
		t.Error("button click 12345 on open bank: want unhandled")
	}
	if len(*buttons) != 1 {
		t.Fatalf("flow calls = %d, want 1", len(*buttons))
	}
}

func TestHandleBankInterfaceActionPassesAction(t *testing.T) {
	_, buttons, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	p.SetInterfaceId(game.BankMainInterfaceID)
	if !HandleBankInterfaceAction(p, 50070, 1) {
		t.Fatal("HandleBankInterfaceAction(interface 12, 50070, 1): want true")
	}
	if len(*buttons) != 1 || (*buttons)[0] != [2]int{50070, 1} {
		t.Fatalf("flow got %v, want [[50070 1]]", *buttons)
	}
	// The same gate applies: wrong interface, no route.
	p.SetInterfaceId(-1)
	if HandleBankInterfaceAction(p, 50070, 1) {
		t.Error("HandleBankInterfaceAction(interface -1, ...): want false")
	}
}

func TestInterfaceActionWidgetRouting(t *testing.T) {
	hub := testHub(t)
	_, _, widgets := installStubFlows(t)
	p := newTestPlayer("alice")
	// Bank open under the §4 contract: status BANKING and
	// interface 12.
	p.SetStatus(game.PlayerStatusBanking)
	p.SetInterfaceId(game.BankMainInterfaceID)
	grid := &plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (12 << 16) | 12, Action: 1,
		GroupID: 12, HasGroupID: true, ChildID: 12, HasChildID: true,
	}
	// Bank open (interface 12) + stub widget flow: claimed.
	if !hub.EmitInterfaceActionClick(grid) {
		t.Fatal("bank grid click while open: want handled")
	}
	if *widgets != 1 {
		t.Fatalf("widget calls = %d, want 1", *widgets)
	}
	// Bank closed: the same widget click falls through.
	p.SetInterfaceId(-1)
	if hub.EmitInterfaceActionClick(grid) {
		t.Error("bank grid click while closed: want unhandled")
	}
	if *widgets != 1 {
		t.Fatalf("widget calls = %d, want 1 (closed click must not route)", *widgets)
	}
	// Non-bank widget: never routes, even with the bank open.
	p.SetInterfaceId(game.BankMainInterfaceID)
	other := &plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (99 << 16) | 1, Action: 1,
		GroupID: 99, HasGroupID: true, ChildID: 1, HasChildID: true,
	}
	if hub.EmitInterfaceActionClick(other) {
		t.Error("non-bank widget click: want unhandled")
	}
	// No (group, child) metadata: no route.
	bare := &plugins.InterfaceActionClickEvent{Player: p, ButtonID: (12 << 16) | 12, Action: 1}
	if hub.EmitInterfaceActionClick(bare) {
		t.Error("widget click without group/child: want unhandled")
	}
	if *widgets != 1 {
		t.Fatalf("widget calls = %d, want 1", *widgets)
	}
}

func TestInterfaceActionWidgetRoutingNeedsFlow(t *testing.T) {
	// Gated in and bank open (status BANKING + interface 12),
	// but no HandleWidget flow: fall through, don't claim.
	hub := testHub(t)
	p := newTestPlayer("alice")
	p.SetStatus(game.PlayerStatusBanking)
	p.SetInterfaceId(game.BankMainInterfaceID)
	grid := &plugins.InterfaceActionClickEvent{
		Player: p, ButtonID: (12 << 16) | 12, Action: 1,
		GroupID: 12, HasGroupID: true, ChildID: 12, HasChildID: true,
	}
	if hub.EmitInterfaceActionClick(grid) {
		t.Error("bank grid click with no HandleWidget flow: want unhandled")
	}
}

func TestBankerDialogueAction(t *testing.T) {
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	// The matching step fires: handled, dialogue reset, bank opened.
	ev := &game.NpcDialogueActionEvent{
		Player: p, NPCID: 1618, DefinitionName: "Banker",
		Action: "open_interface", Target: "Bank",
	}
	bankerDialogueAction(nil, ev)
	if !ev.Handled {
		t.Fatal("matching dialogue action: want Handled")
	}
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
	// Already handled: no second open.
	ev2 := &game.NpcDialogueActionEvent{
		Player: p, NPCID: 1618, DefinitionName: "Banker",
		Action: "open_interface", Target: "Bank", Handled: true,
	}
	bankerDialogueAction(nil, ev2)
	if *opens != 1 {
		t.Fatalf("opens after handled event = %d, want 1", *opens)
	}
	// Wrong action / wrong target / wrong NPC name: no fire.
	for _, tc := range []struct {
		action, target, name string
	}{
		{"open_shop", "Bank", "Banker"},
		{"open_interface", "Shop", "Banker"},
		{"open_interface", "Bank", "Shopkeeper"},
		{"open_interface", "Bank", "Gnome Banker"},
	} {
		ev := &game.NpcDialogueActionEvent{
			Player: p, NPCID: 1618, DefinitionName: tc.name,
			Action: tc.action, Target: tc.target,
		}
		bankerDialogueAction(nil, ev)
		if ev.Handled {
			t.Errorf("dialogue action %+v: want unhandled", tc)
		}
	}
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1 (only the matching step fires)", *opens)
	}
	// Nil player: no fire, no panic.
	ev3 := &game.NpcDialogueActionEvent{
		DefinitionName: "Banker", Action: "open_interface", Target: "Bank",
	}
	bankerDialogueAction(nil, ev3)
	if ev3.Handled {
		t.Error("dialogue action with nil player: want unhandled")
	}
	// Non-event payloads are ignored.
	bankerDialogueAction(nil, "npc-dialogue:action")
	bankerDialogueAction(nil, nil)
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
}

func TestBankerDialogueActionViaHub(t *testing.T) {
	// The subscription itself: the hub delivers the synthetic
	// event to the registered handler.
	hub := testHub(t)
	opens, _, _ := installStubFlows(t)
	p := newTestPlayer("alice")
	hub.EmitCustomEvent(plugins.NpcDialogueActionEventName, &game.NpcDialogueActionEvent{
		Player: p, NPCID: 1618, DefinitionName: "Banker",
		Action: "open_interface", Target: "Bank",
	})
	if *opens != 1 {
		t.Fatalf("opens = %d, want 1", *opens)
	}
}

func TestTalkToBankerNilApi(t *testing.T) {
	// Without an api there is no bus to emit on: unhandled,
	// never a panic.
	p := newTestPlayer("alice")
	ev := &plugins.NpcInteractionEvent{Player: p, NpcID: 1618, DefinitionName: "Banker"}
	if talkToBanker(nil, ev) {
		t.Error("talkToBanker(nil api): want false")
	}
	if talkToBanker(nil, nil) {
		t.Error("talkToBanker(nil event): want false")
	}
}
