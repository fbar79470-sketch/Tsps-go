package game

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

// tradeTestSession is a SessionPort test double recording trade
// sends, dialogue sends, and chat lines. It embeds *fakeSession
// for the SessionPort/ChatMessageSender surface and adds the
// TradeSender and DialogueSender extensions Trading needs.
type tradeTestSession struct {
	*fakeSession
	tradeRequests [][2]any // [index, name]
	tradeOpens    []tradeSend
	tradeUpdates  []tradeSend
	tradeCloses   []string
	requestLines  [][2]any // [name, index]
	subInterfaces [][3]int // [parent, group, slot]
	scriptArgs    [][2]any // [scriptID, args]
	strings       [][2]any // [uid, text]
	flags         [][4]int
	removals      int
}

type tradeSend struct {
	sessionID string
	stage     TradeStage
	self      TradeParty
	other     TradeParty
}

func newTradeTestSession() *tradeTestSession {
	return &tradeTestSession{fakeSession: newFakeSession()}
}

func (s *tradeTestSession) SendTradeRequest(index int, name string) {
	s.tradeRequests = append(s.tradeRequests, [2]any{index, name})
}

func (s *tradeTestSession) SendTradeOpen(sessionID string, stage TradeStage, self, other TradeParty) {
	s.tradeOpens = append(s.tradeOpens, tradeSend{sessionID, stage, self, other})
}

func (s *tradeTestSession) SendTradeUpdate(sessionID string, stage TradeStage, self, other TradeParty) {
	s.tradeUpdates = append(s.tradeUpdates, tradeSend{sessionID, stage, self, other})
}

func (s *tradeTestSession) SendTradeClose(message string) {
	s.tradeCloses = append(s.tradeCloses, message)
}

func (s *tradeTestSession) SendTradeRequestMessage(name string, index int) {
	s.requestLines = append(s.requestLines, [2]any{name, index})
}

func (s *tradeTestSession) SendSubInterface(parent, group, slot int) {
	s.subInterfaces = append(s.subInterfaces, [3]int{parent, group, slot})
}

func (s *tradeTestSession) SendInterfaceScriptArgs(scriptID int, args []int) {
	s.scriptArgs = append(s.scriptArgs, [2]any{scriptID, append([]int(nil), args...)})
}

func (s *tradeTestSession) SendChatboxInterface(int, bool)           {}
func (s *tradeTestSession) SendNpcHeadOnInterface(int, int)          {}
func (s *tradeTestSession) SendPlayerHeadOnInterface(int)            {}
func (s *tradeTestSession) SendInterfaceAnimation(int, int)          {}
func (s *tradeTestSession) SendClientScript(int, ...ClientScriptArg) {}
func (s *tradeTestSession) SendItemOnInterfaces(int, int, int)       {}
func (s *tradeTestSession) SendInterfaceScript(int)                  {}
func (s *tradeTestSession) IsChatboxInterface(int) bool              { return false }
func (s *tradeTestSession) WithChatboxOwner(any, func())             {}

func (s *tradeTestSession) SendString(text string, uid int) {
	s.strings = append(s.strings, [2]any{uid, text})
}

func (s *tradeTestSession) SendInterfaceFlagsRange(uid, fromSlot, toSlot, flags int) {
	s.flags = append(s.flags, [4]int{uid, fromSlot, toSlot, flags})
}

func (s *tradeTestSession) SendInterfaceRemoval() { s.removals++ }

// tradeTestDefs extends testItemDefs with Grand Exchange values
// and item names (the optional ItemGEValueSource/ItemNameSource
// capabilities Trading reads through the definition source).
type tradeTestDefs struct {
	*testItemDefs
	geValues map[int]int
	names    map[int]string
}

func (s *tradeTestDefs) GrandExchangeValue(id int) int { return s.geValues[id] }
func (s *tradeTestDefs) ItemName(id int) string        { return s.names[id] }

// newTradeTestDefs returns definitions where 995 is stackable
// tradeable coins (GE 1) and 1 is an unstackable tradeable
// sword (GE 100) named "Bronze_sword".
func newTradeTestDefs() *tradeTestDefs {
	return &tradeTestDefs{
		testItemDefs: &testItemDefs{
			stackable: map[int]bool{995: true},
			tradeable: map[int]bool{995: true, 1: true},
		},
		geValues: map[int]int{995: 1, 1: 100},
		names:    map[int]string{995: "Coins", 1: "Bronze_sword"},
	}
}

// tradeClock is a controllable clock shared by a trade pair's
// tradings and timers.
type tradeClock struct{ now time.Time }

func (c *tradeClock) Now() time.Time { return c.now }

// tradePair builds two players with trade sessions, indices 1
// and 2, sharing defs and a clock fixed at t0. The clock
// advances through the returned pointer.
func tradePair(t *testing.T, defs ItemDefinitionSource) (a, b *Player, sa, sb *tradeTestSession, clock *tradeClock) {
	t.Helper()
	clock = &tradeClock{now: time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)}
	mk := func(name string, index int) (*Player, *tradeTestSession) {
		p := NewPlayer(name, Location{X: 3200, Y: 3200, Z: 0})
		p.SetIndex(index)
		p.SetItemDefinitions(defs)
		s := newTradeTestSession()
		p.Session = s
		p.Trading().setClock(clock.Now)
		return p, s
	}
	a, sa = mk("Alice", 1)
	b, sb = mk("Bob", 2)
	return a, b, sa, sb, clock
}

// withTradeHooks installs hooks for the test, restoring the
// previous seam afterwards.
func withTradeHooks(t *testing.T, hooks TradeHooks) {
	t.Helper()
	prev := TradeHookSeam
	TradeHookSeam = hooks
	t.Cleanup(func() { TradeHookSeam = prev })
}

// startTrade runs the mutual request flow until both players
// are on the offer screen.
func startTrade(t *testing.T, a, b *Player) {
	t.Helper()
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	if a.Trading().State() != TradeStateTradeScreen || b.Trading().State() != TradeStateTradeScreen {
		t.Fatalf("states after mutual request = %v/%v, want trade screen", a.Trading().State(), b.Trading().State())
	}
}

func TestTradingAccessorIdentity(t *testing.T) {
	p := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	if p.Trading() == nil || p.Trading() != p.Trading() {
		t.Fatal("Trading() must return the same instance on repeat access")
	}
	q := NewPlayer("Bob", Location{X: 3200, Y: 3200, Z: 0})
	if p.Trading() == q.Trading() {
		t.Fatal("distinct players share a Trading instance")
	}
	var nilPlayer *Player
	if nilPlayer.Trading() != nil {
		t.Fatal("nil player Trading() must be nil")
	}
}

func TestTradingStatusDerivation(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	if a.Trading().Status() != PlayerStatusNone {
		t.Fatalf("fresh status = %v, want NONE", a.Trading().Status())
	}
	a.Trading().RequestTrade(b)
	if a.Trading().Status() != PlayerStatusNone {
		t.Fatalf("requested status = %v, want NONE", a.Trading().Status())
	}
	b.Trading().RequestTrade(a)
	if a.Trading().Status() != PlayerStatusTrading || b.Trading().Status() != PlayerStatusTrading {
		t.Fatal("offer screen status must be TRADING")
	}
	a.Trading().CloseTrade()
	if a.Trading().Status() != PlayerStatusNone || b.Trading().Status() != PlayerStatusNone {
		t.Fatal("status after close must be NONE")
	}
}

func TestTradingRequestAndInitiate(t *testing.T) {
	a, b, sa, sb, _ := tradePair(t, newTradeTestDefs())
	a.Trading().RequestTrade(b)
	if a.Trading().State() != TradeStateRequestedTrade || a.Trading().Interact() != b {
		t.Fatalf("requester state = %v interact set = %v", a.Trading().State(), a.Trading().Interact() == b)
	}
	if len(sa.typedMessages) != 1 || sa.typedMessages[0] != [3]any{chatTypeTrade, "Sending trade offer...", ""} {
		t.Fatalf("requester typed messages = %v", sa.typedMessages)
	}
	if len(sb.requestLines) != 1 || sb.requestLines[0] != [2]any{"Alice", 1} {
		t.Fatalf("target request lines = %v", sb.requestLines)
	}
	if len(sb.tradeRequests) != 1 || sb.tradeRequests[0] != [2]any{1, "Alice"} {
		t.Fatalf("target trade requests = %v", sb.tradeRequests)
	}

	b.Trading().RequestTrade(a)
	if a.Trading().InterfaceID() != 335 || b.Trading().InterfaceID() != 335 {
		t.Fatalf("interface ids = %d/%d, want 335", a.Trading().InterfaceID(), b.Trading().InterfaceID())
	}
	// The offer mount, the inventory mount, and the script.
	wantSubs := [][3]int{{(161 << 16) | 16, 335, 0}, {(161 << 16) | 79, 336, 1}}
	for name, s := range map[string]*tradeTestSession{"Alice": sa, "Bob": sb} {
		if fmt.Sprint(s.subInterfaces) != fmt.Sprint(wantSubs) {
			t.Fatalf("%s sub-interfaces = %v, want %v", name, s.subInterfaces, wantSubs)
		}
		if len(s.scriptArgs) != 1 || s.scriptArgs[0][0] != 3617 {
			t.Fatalf("%s script args = %v", name, s.scriptArgs)
		}
		if args, ok := s.scriptArgs[0][1].([]int); !ok || len(args) != 1 || args[0] != 336<<16 {
			t.Fatalf("%s script args = %v", name, s.scriptArgs)
		}
		if len(s.flags) != 3 {
			t.Fatalf("%s flags = %v, want 3 ranges", name, s.flags)
		}
		if len(s.tradeOpens) < 1 || s.tradeOpens[len(s.tradeOpens)-1].sessionID != "1:2" ||
			s.tradeOpens[len(s.tradeOpens)-1].stage != TradeStageOffer {
			t.Fatalf("%s opens = %v, want an offer open for 1:2", name, s.tradeOpens)
		}
	}
}

func TestTradingRequestDelayMessages(t *testing.T) {
	defs := newTradeTestDefs()
	a, b, sa, _, clock := tradePair(t, defs)
	a.Trading().RequestTrade(b) // starts the 2s delay
	clock.now = clock.now.Add(500 * time.Millisecond)
	a.Trading().RequestTrade(b)
	if len(sa.messages) != 1 || sa.messages[0] != "You must wait another 2 seconds before sending more trade requests." {
		t.Fatalf("messages at +0.5s = %v", sa.messages)
	}
	clock.now = clock.now.Add(time.Second) // +1.5s: one whole second remains
	a.Trading().RequestTrade(b)
	if len(sa.messages) != 2 || sa.messages[1] != "You must wait another second before sending more trade requests." {
		t.Fatalf("messages at +1.5s = %v", sa.messages)
	}
	clock.now = clock.now.Add(600 * time.Millisecond) // past 2s
	a.Trading().RequestTrade(b)
	if len(sa.messages) != 2 {
		t.Fatalf("request after delay expired must not refuse: %v", sa.messages)
	}
}

func TestTradingRequestRefusedInTrade(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	c := NewPlayer("Carol", Location{X: 3200, Y: 3200, Z: 0})
	c.SetIndex(3)
	startTrade(t, a, b)
	a.Trading().RequestTrade(c)
	if len(sa.messages) != 1 || sa.messages[0] != "You cannot do that right now." {
		t.Fatalf("messages = %v", sa.messages)
	}
}

func TestTradingCanTradeAreaRefusal(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	deny := false
	area := NewArea("NoTrade", nil)
	area.CanTradeHook = func(player, target *Player) *bool { return &deny }
	a.SetArea(area)
	a.Trading().RequestTrade(b)
	if len(sa.messages) != 1 || sa.messages[0] != "You cannot trade here." {
		t.Fatalf("messages = %v", sa.messages)
	}
	if a.Trading().State() != TradeStateNone {
		t.Fatalf("state = %v, want NONE", a.Trading().State())
	}
}

func TestTradingCanTradeSeamShapes(t *testing.T) {
	allow := true
	deny := false
	for _, tc := range []struct {
		name  string
		allow *bool
		want  bool // request proceeds
	}{
		{"nil opinion allows", nil, true},
		{"explicit allow allows", &allow, true},
		{"explicit deny refuses", &deny, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			withTradeHooks(t, TradeHooks{
				EmitCanTrade: func(player, target *Player) *bool { return tc.allow },
			})
			a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
			a.Trading().RequestTrade(b)
			if tc.want && a.Trading().State() != TradeStateRequestedTrade {
				t.Fatalf("state = %v, want REQUESTED", a.Trading().State())
			}
			if !tc.want && (len(sa.messages) != 1 || sa.messages[0] != "You cannot trade here.") {
				t.Fatalf("messages = %v", sa.messages)
			}
		})
	}
}

func TestTradingOfferAndFullAcceptFlow(t *testing.T) {
	defs := newTradeTestDefs()
	a, b, sa, sb, clock := tradePair(t, defs)
	var saves [][2]any
	var completed []*TradeCompletedEvent
	withTradeHooks(t, TradeHooks{
		SavePlayer: func(p *Player, reason string) { saves = append(saves, [2]any{p.Username, reason}) },
		EmitTradeCompleted: func(e *TradeCompletedEvent) {
			completed = append(completed, e)
		},
	})
	a.Inventory().AddItem(NewItem(995, 100))
	b.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)

	// Alice offers 50 coins; Bob offers his sword.
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	b.Trading().HandleItem(1, 1, 0, b.Inventory(), b.Trading().Container())
	if a.Inventory().GetAmount(995) != 50 || b.Inventory().GetAmount(1) != 0 {
		t.Fatalf("inventories after offer: coins=%d swords=%d", a.Inventory().GetAmount(995), b.Inventory().GetAmount(1))
	}
	if len(a.Trading().OfferedItems()) != 1 || a.Trading().OfferedItems()[0].Amount() != 50 {
		t.Fatalf("Alice offers = %v", a.Trading().OfferedItems())
	}

	// Offer-screen accepts promote to the confirm screen.
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedTradeScreen {
		t.Fatalf("Alice state = %v", a.Trading().State())
	}
	b.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateConfirmScreen || b.Trading().State() != TradeStateConfirmScreen {
		t.Fatalf("states = %v/%v, want confirm", a.Trading().State(), b.Trading().State())
	}
	if a.Trading().InterfaceID() != 334 || b.Trading().InterfaceID() != 334 {
		t.Fatalf("interface ids = %d/%d, want 334", a.Trading().InterfaceID(), b.Trading().InterfaceID())
	}
	last := sa.tradeOpens[len(sa.tradeOpens)-1]
	if last.stage != TradeStageConfirm {
		t.Fatalf("last open stage = %v, want confirm", last.stage)
	}

	// Confirm accepts complete the trade. The button delay
	// (1s) started at each offer-screen accept; advance past it.
	clock.now = clock.now.Add(2 * time.Second)
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateNone || b.Trading().State() != TradeStateNone {
		t.Fatalf("states = %v/%v, want NONE", a.Trading().State(), b.Trading().State())
	}
	if a.Inventory().GetAmount(995) != 50 || a.Inventory().GetAmount(1) != 1 {
		t.Fatalf("Alice inventory: coins=%d swords=%d", a.Inventory().GetAmount(995), a.Inventory().GetAmount(1))
	}
	if b.Inventory().GetAmount(995) != 50 || b.Inventory().GetAmount(1) != 0 {
		t.Fatalf("Bob inventory: coins=%d swords=%d", b.Inventory().GetAmount(995), b.Inventory().GetAmount(1))
	}
	// The completing side (Bob, who accepted second) saves
	// first, then the partner — this.save() before
	// interact's save (Trading.ts:318-319).
	if fmt.Sprint(saves) != "[[Bob trade] [Alice trade]]" {
		t.Fatalf("saves = %v", saves)
	}
	if len(completed) != 2 {
		t.Fatalf("completed events = %d, want 2 mirrored", len(completed))
	}
	if completed[0].Player != b || completed[0].Partner != a || len(completed[0].Received) != 1 || len(completed[0].Given) != 1 {
		t.Fatalf("first event = %+v", completed[0])
	}
	if completed[1].Player != a || completed[1].Partner != b {
		t.Fatalf("second event = %+v", completed[1])
	}
	for name, s := range map[string]*tradeTestSession{"Alice": sa, "Bob": sb} {
		if len(s.tradeCloses) == 0 || s.tradeCloses[len(s.tradeCloses)-1] != AcceptedTradeMessage {
			t.Fatalf("%s closes = %v", name, s.tradeCloses)
		}
		if s.removals == 0 {
			t.Fatalf("%s removals = 0", name)
		}
		last := s.typedMessages[len(s.typedMessages)-1]
		if last != [3]any{chatTypeTrade, AcceptedTradeMessage, ""} {
			t.Fatalf("%s last typed = %v", name, last)
		}
	}
}

func TestTradingSaveSkippedWithAttribute(t *testing.T) {
	a, b, _, _, clock := tradePair(t, newTradeTestDefs())
	var saved []string
	withTradeHooks(t, TradeHooks{
		SavePlayer: func(p *Player, reason string) { saved = append(saved, p.Username) },
	})
	a.SetAttribute(skipPersistenceAttribute, true)
	startTrade(t, a, b)
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	clock.now = clock.now.Add(2 * time.Second)
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if fmt.Sprint(saved) != "[Bob]" {
		t.Fatalf("saved = %v, want only Bob", saved)
	}
}

func TestTradingAcceptDelay(t *testing.T) {
	defs := newTradeTestDefs()
	a, b, _, _, clock := tradePair(t, defs)
	withTradeHooks(t, TradeHooks{
		GetVarbit: func(p *Player, id int) int {
			if id == acceptDelayVarbit {
				return 1
			}
			return 0
		},
	})
	a.Inventory().AddItem(NewItem(995, 10))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 10, 0, a.Inventory(), a.Trading().Container())
	// The offer changed at t0; the delay varbit is on.
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateTradeScreen {
		t.Fatalf("state = %v, want trade screen while delayed", a.Trading().State())
	}
	clock.now = clock.now.Add(2999 * time.Millisecond)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateTradeScreen {
		t.Fatalf("state at +2999ms = %v, want trade screen", a.Trading().State())
	}
	clock.now = clock.now.Add(time.Millisecond)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedTradeScreen {
		t.Fatalf("state at +3000ms = %v, want accepted", a.Trading().State())
	}
}

func TestTradingAcceptDelayOffWithoutVarbit(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	// No GetVarbit seam: the varbit reads 0, no delay.
	a.Inventory().AddItem(NewItem(995, 10))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 10, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedTradeScreen {
		t.Fatalf("state = %v, want accepted", a.Trading().State())
	}
}

func TestTradingButtonDelayBlocksConfirmAccept(t *testing.T) {
	a, b, _, _, clock := tradePair(t, newTradeTestDefs())
	startTrade(t, a, b)
	a.Trading().AcceptTrade() // starts Alice's 1s button delay
	b.Trading().AcceptTrade() // both accepted: confirm screens
	clock.now = clock.now.Add(500 * time.Millisecond)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateConfirmScreen {
		t.Fatalf("state at +0.5s = %v, want confirm (button delay)", a.Trading().State())
	}
	clock.now = clock.now.Add(600 * time.Millisecond)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedConfirmScreen {
		t.Fatalf("state at +1.1s = %v, want accepted confirm", a.Trading().State())
	}
}

func TestTradingCannotHoldOnOfferAccept(t *testing.T) {
	a, b, sa, sb, _ := tradePair(t, newTradeTestDefs())
	// Fill Bob's inventory with distinct unstackable items.
	for i := 0; i < 28; i++ {
		b.Inventory().AddItem(NewItem(1000+i, 1))
	}
	a.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(1, 1, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateTradeScreen {
		t.Fatalf("state = %v, want trade screen", a.Trading().State())
	}
	want := []string{"", "@or3@Bob will not be able to hold that item.", "@or3@They have 0 free inventory slots."}
	if fmt.Sprint(sa.messages) != fmt.Sprint(want) {
		t.Fatalf("Alice messages = %v, want %v", sa.messages, want)
	}
	if len(sb.messages) != 1 || sb.messages[0] != "Trade cannot be accepted, you don't have enough free inventory space." {
		t.Fatalf("Bob messages = %v", sb.messages)
	}
	// The refusal returns before the button delay starts, so a
	// freed slot lets Alice accept immediately.
	freed := NewItem(1000, 1)
	b.Inventory().DeleteItemContainer(&freed, 0, false, nil)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedTradeScreen {
		t.Fatalf("state after freeing a slot = %v, want accepted", a.Trading().State())
	}
}

func TestTradingCannotHoldSingularSlot(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	for i := 0; i < 27; i++ {
		b.Inventory().AddItem(NewItem(1000+i, 1))
	}
	a.Inventory().AddItem(NewItem(1, 2))
	startTrade(t, a, b)
	a.Trading().HandleItem(1, 2, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	if len(sa.messages) != 3 || sa.messages[2] != "@or3@They have 1 free inventory slot." {
		t.Fatalf("messages = %v", sa.messages)
	}
}

func TestTradingConfirmRoomRecheck(t *testing.T) {
	a, b, sa, sb, clock := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(1, 1, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	// Bob's inventory fills after the offer-screen check (gear
	// taken off mid-trade, in TS terms).
	for i := 0; i < 28; i++ {
		b.Inventory().AddItem(NewItem(1000+i, 1))
	}
	clock.now = clock.now.Add(2 * time.Second)
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateConfirmScreen || b.Trading().State() != TradeStateConfirmScreen {
		t.Fatalf("states = %v/%v, want reverted to confirm", a.Trading().State(), b.Trading().State())
	}
	if len(sb.messages) == 0 || sb.messages[len(sb.messages)-1] != "Trade cannot be accepted, you don't have enough free inventory space." {
		t.Fatalf("Bob messages = %v", sb.messages)
	}
	if len(sa.messages) < 3 || sa.messages[len(sa.messages)-2] != "@or3@Bob will not be able to hold that item." {
		t.Fatalf("Alice messages = %v", sa.messages)
	}
}

func TestTradingDeclineReturnsOffers(t *testing.T) {
	a, b, sa, sb, _ := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	b.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	b.Trading().HandleItem(1, 1, 0, b.Inventory(), b.Trading().Container())
	a.Trading().CloseTrade()
	if a.Trading().State() != TradeStateNone || b.Trading().State() != TradeStateNone {
		t.Fatal("states must be NONE after decline")
	}
	if a.Inventory().GetAmount(995) != 100 || b.Inventory().GetAmount(1) != 1 {
		t.Fatalf("offers not returned: coins=%d swords=%d", a.Inventory().GetAmount(995), b.Inventory().GetAmount(1))
	}
	if len(a.Trading().OfferedItems()) != 0 || len(b.Trading().OfferedItems()) != 0 {
		t.Fatal("offer containers must be empty after decline")
	}
	if len(sa.messages) != 1 || sa.messages[0] != "Trade declined." {
		t.Fatalf("Alice messages = %v", sa.messages)
	}
	if len(sb.messages) != 1 || sb.messages[0] != "Other player declined trade." {
		t.Fatalf("Bob messages = %v", sb.messages)
	}
	if a.Trading().InterfaceID() != -1 || b.Trading().InterfaceID() != -1 {
		t.Fatal("interface ids must clear on decline")
	}
	if sa.removals != 1 || sb.removals != 1 {
		t.Fatalf("removals = %d/%d", sa.removals, sb.removals)
	}
}

func TestTradingCloseTradeLeavesRequestedPartner(t *testing.T) {
	a, b, _, sb, _ := tradePair(t, newTradeTestDefs())
	a.Trading().RequestTrade(b)
	a.Trading().CloseTrade()
	if a.Trading().State() != TradeStateNone {
		t.Fatalf("Alice state = %v", a.Trading().State())
	}
	if b.Trading().State() != TradeStateNone || len(sb.messages) != 0 || len(sb.tradeCloses) != 0 {
		t.Fatalf("Bob must be untouched: state=%v messages=%v closes=%v", b.Trading().State(), sb.messages, sb.tradeCloses)
	}
}

func TestTradingAbortDropsWhenInventoryFull(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	var dropped []Item
	withTradeHooks(t, TradeHooks{
		DropGroundItem: func(item Item, loc Location, owner *Player) { dropped = append(dropped, item) },
	})
	a.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(1, 1, 0, a.Inventory(), a.Trading().Container())
	for i := 0; i < 28; i++ {
		a.Inventory().AddItem(NewItem(1000+i, 1))
	}
	a.Trading().CloseTrade()
	if len(dropped) != 1 || dropped[0].ID() != 1 {
		t.Fatalf("dropped = %v, want the sword", dropped)
	}
	if len(sa.messages) < 2 || sa.messages[len(sa.messages)-1] != "Your inventory is full, so some of your offered items were dropped on the floor." {
		t.Fatalf("messages = %v", sa.messages)
	}
}

func TestTradingHandleItemGates(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	startTrade(t, a, b)

	// Wrong interface: silent no-op.
	a.Trading().interfaceID = 334
	a.Trading().HandleItem(995, 10, 0, a.Inventory(), a.Trading().Container())
	if len(a.Trading().OfferedItems()) != 0 {
		t.Fatal("handleItem on the confirm screen must not move items")
	}
	a.Trading().interfaceID = 335

	// Wrong id for the slot: silent no-op.
	a.Trading().HandleItem(1, 10, 0, a.Inventory(), a.Trading().Container())
	if len(a.Trading().OfferedItems()) != 0 {
		t.Fatal("handleItem with a mismatched id must not move items")
	}

	// Untradeable item: refused with a message.
	untradeable := NewItemWithMeta(1, 1, map[string]json.RawMessage{ItemUntradeableMeta: json.RawMessage("true")})
	a.Inventory().AddItem(untradeable)
	a.Trading().HandleItem(1, 1, 1, a.Inventory(), a.Trading().Container())
	if len(sa.messages) != 1 || sa.messages[0] != "You cannot trade that item." {
		t.Fatalf("messages = %v", sa.messages)
	}
	if len(a.Trading().OfferedItems()) != 0 {
		t.Fatal("untradeable item must not be offered")
	}

	// Neither-tradeable-nor-untradeable without an eligibility
	// seam: refused.
	a.Inventory().AddItem(NewItem(777, 1))
	a.Trading().HandleItem(777, 1, 2, a.Inventory(), a.Trading().Container())
	if len(a.Trading().OfferedItems()) != 0 {
		t.Fatal("item without eligibility must not be offered")
	}
}

func TestTradingHandleItemEligibilitySeam(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	withTradeHooks(t, TradeHooks{
		ItemEligibility: func(item Item) bool { return item.ID() == 777 },
	})
	a.Inventory().AddItem(NewItem(777, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(777, 1, 0, a.Inventory(), a.Trading().Container())
	if len(a.Trading().OfferedItems()) != 1 {
		t.Fatalf("offers = %v, want the eligible item", a.Trading().OfferedItems())
	}
}

func TestTradingHandleItemStackableMovesClickedSlotOnly(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	a.Inventory().AddItem(NewItem(1, 1))
	// A second coins stack in slot 2 (Add merges into slot 0,
	// so place it directly).
	a.Inventory().SetItems([]Item{NewItem(995, 100), NewItem(1, 1), NewItem(995, 40)})
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 30, 2, a.Inventory(), a.Trading().Container())
	if a.Inventory().ItemAt(0).Amount() != 100 || a.Inventory().ItemAt(2).Amount() != 10 {
		t.Fatalf("slots after offer: 0=%v 2=%v", a.Inventory().ItemAt(0), a.Inventory().ItemAt(2))
	}
	offers := a.Trading().OfferedItems()
	if len(offers) != 1 || offers[0].Amount() != 30 {
		t.Fatalf("offers = %v", offers)
	}
}

func TestTradingHandleItemUnstackableClampAndSweep(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	for i := 0; i < 5; i++ {
		a.Inventory().AddItem(NewItem(1, 1))
	}
	startTrade(t, a, b)
	// Fill 25 offer slots first so only 3 free slots remain.
	for i := 0; i < 25; i++ {
		a.Trading().Container().Add(NewItem(2000+i, 1), false)
	}
	// Offer 5 swords from slot 0: clamped to the 3 free slots,
	// sweeping slots 0..2.
	a.Trading().HandleItem(1, 5, 0, a.Inventory(), a.Trading().Container())
	swords := 0
	for _, it := range a.Trading().OfferedItems() {
		if it.ID() == 1 {
			swords++
		}
	}
	if swords != 3 {
		t.Fatalf("offered swords = %d, want 3 (clamped)", swords)
	}
	if a.Inventory().GetAmount(1) != 2 {
		t.Fatalf("inventory swords = %d, want 2", a.Inventory().GetAmount(1))
	}
}

func TestTradingHandleItemResetsAccepts(t *testing.T) {
	a, b, sa, sb, clock := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	b.Inventory().AddItem(NewItem(995, 100))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 10, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateConfirmScreen {
		t.Fatalf("states must reach confirm, got %v", a.Trading().State())
	}
	// A fresh trade: accept on the offer screen only, then
	// change the offer.
	a.Trading().CloseTrade()
	clock.now = clock.now.Add(3 * time.Second) // past the request delay
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	a.Trading().AcceptTrade()
	if a.Trading().State() != TradeStateAcceptedTradeScreen {
		t.Fatalf("state = %v", a.Trading().State())
	}
	updatesBefore := len(sa.tradeUpdates) + len(sb.tradeUpdates)
	b.Trading().HandleItem(995, 10, 0, b.Inventory(), b.Trading().Container())
	if a.Trading().State() != TradeStateTradeScreen || b.Trading().State() != TradeStateTradeScreen {
		t.Fatalf("states = %v/%v, want both reset to trade screen", a.Trading().State(), b.Trading().State())
	}
	if len(sa.tradeUpdates)+len(sb.tradeUpdates) <= updatesBefore {
		t.Fatal("resetting the accepts must send state updates")
	}
}

func TestTradingRemoveFromOffer(t *testing.T) {
	a, b, _, _, _ := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(995, 100))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 60, 0, a.Inventory(), a.Trading().Container())
	a.Trading().HandleItem(995, 25, 0, a.Trading().Container(), a.Inventory())
	if a.Inventory().GetAmount(995) != 65 {
		t.Fatalf("coins in inventory = %d, want 65", a.Inventory().GetAmount(995))
	}
	offers := a.Trading().OfferedItems()
	if len(offers) != 1 || offers[0].Amount() != 35 {
		t.Fatalf("offers = %v, want 35 coins", offers)
	}
}

func TestTradingOfferContainerFullMessage(t *testing.T) {
	a, b, sa, _, _ := tradePair(t, newTradeTestDefs())
	a.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	// Fill the offer container, then switch into it: the
	// destination's Full runs through the Container interface.
	for i := 0; i < 28; i++ {
		a.Trading().Container().Add(NewItem(2000+i, 1), false)
	}
	sword := NewItem(1, 1)
	a.Inventory().SwitchItem(a.Trading().Container(), &sword, false, 0, false)
	if len(sa.messages) == 0 || sa.messages[len(sa.messages)-1] != "You cannot trade more items." {
		t.Fatalf("messages = %v", sa.messages)
	}
}

func TestTradingSendTextOfferScreen(t *testing.T) {
	defs := newTradeTestDefs()
	a, b, sa, _, _ := tradePair(t, defs)
	a.Inventory().AddItem(NewItem(995, 100))
	b.Inventory().AddItem(NewItem(1, 1))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	b.Trading().HandleItem(1, 1, 0, b.Inventory(), b.Trading().Container())
	// The last update's texts are the offer screen's.
	texts := map[int]string{}
	for _, s := range sa.strings {
		texts[s[0].(int)] = s[1].(string)
	}
	if texts[tradeOfferTitleUID] != "Trading With: Bob" {
		t.Fatalf("title = %q", texts[tradeOfferTitleUID])
	}
	if texts[tradeOfferOwnHeadingUID] != "You offer:<br>(Value: <col=ffffff>50</col> coins)" {
		t.Fatalf("own heading = %q", texts[tradeOfferOwnHeadingUID])
	}
	if texts[tradeOfferOtherHeadingUID] != "Bob offers:<br>(Value: <col=ffffff>100</col> coins)" {
		t.Fatalf("other heading = %q", texts[tradeOfferOtherHeadingUID])
	}
	if texts[tradeOfferFreeSlotsUID] != "Bob has 28 free inventory slots." {
		t.Fatalf("free slots = %q", texts[tradeOfferFreeSlotsUID])
	}
	if texts[tradeOfferStatusUID] != "" {
		t.Fatalf("status = %q, want empty idle", texts[tradeOfferStatusUID])
	}
}

func TestTradingSendTextStatusLines(t *testing.T) {
	a, b, sa, sb, _ := tradePair(t, newTradeTestDefs())
	startTrade(t, a, b)
	a.Trading().AcceptTrade()
	lastStatus := func(s *tradeTestSession) string {
		for i := len(s.strings) - 1; i >= 0; i-- {
			if s.strings[i][0].(int) == tradeOfferStatusUID {
				return s.strings[i][1].(string)
			}
		}
		return "<none>"
	}
	if lastStatus(sa) != "Waiting for other player..." {
		t.Fatalf("Alice status = %q", lastStatus(sa))
	}
	if lastStatus(sb) != "Other player has accepted." {
		t.Fatalf("Bob status = %q", lastStatus(sb))
	}
}

func TestTradingSendTextConfirmScreen(t *testing.T) {
	defs := newTradeTestDefs()
	a, b, sa, _, clock := tradePair(t, defs)
	a.Inventory().AddItem(NewItem(995, 100))
	startTrade(t, a, b)
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	clock.now = clock.now.Add(2 * time.Second)
	a.Trading().AcceptTrade()
	texts := map[int]string{}
	for _, s := range sa.strings {
		texts[s[0].(int)] = s[1].(string)
	}
	if texts[tradeConfirmOpponentUID] != "Trading with:<br>Bob" {
		t.Fatalf("opponent = %q", texts[tradeConfirmOpponentUID])
	}
	if texts[tradeConfirmOwnHeadingUID] != "You are about to give:<br>(Value: <col=ffffff>50</col> coins)" {
		t.Fatalf("own heading = %q", texts[tradeConfirmOwnHeadingUID])
	}
	// Bob offers nothing: the receive value is red at zero.
	if texts[tradeConfirmOtherHeadingUID] != "In return you will receive:<br>(Value: <col=ff0000>0</col> coins)" {
		t.Fatalf("other heading = %q", texts[tradeConfirmOtherHeadingUID])
	}
	if texts[tradeConfirmStatusUID] != "Waiting for other player..." {
		t.Fatalf("status = %q", texts[tradeConfirmStatusUID])
	}
}

func TestListTradeItems(t *testing.T) {
	defs := newTradeTestDefs()
	if got := ListTradeItems(nil, defs); got != "Absolutely nothing!" {
		t.Fatalf("empty = %q", got)
	}
	items := []Item{
		NewItem(995, 1),
		NewItem(995, 1500),
		NewItem(995, 2_500_000),
		NewItem(995, 1_500_000_000),
		NewItem(1, 3),
		NewItem(-1, 0),
	}
	want := "Coins x @red@1\n" +
		"Coins x @red@@cya@1K @whi@(1,500)\n" +
		"Coins x @red@@gre@2 million @whi@(2,500,000)\n" +
		"Coins x @red@@gre@1 billion @whi@(1,500,000,000)\n" +
		"Bronze sword x @red@3"
	if got := ListTradeItems(items, defs); got != want {
		t.Fatalf("list = %q\nwant %q", got, want)
	}
	// Without a name source the DEFAULT definition's empty
	// name is used.
	if got := ListTradeItems([]Item{NewItem(995, 5)}, nil); got != " x @red@5" {
		t.Fatalf("no-source list = %q", got)
	}
}

func TestFormatGrouped(t *testing.T) {
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{0, "0"}, {5, "5"}, {999, "999"}, {1000, "1,000"},
		{1234567, "1,234,567"}, {1000000000, "1,000,000,000"},
		{-1500, "-1,500"},
	} {
		if got := formatGrouped(tc.n); got != tc.want {
			t.Fatalf("formatGrouped(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}

func TestReceiverCanHold(t *testing.T) {
	defs := newTradeTestDefs()
	p := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	// Empty inventory holds anything that fits in slots.
	if !ReceiverCanHold(p, []Item{NewItem(1, 1), NewItem(1, 1)}) {
		t.Fatal("empty inventory must hold two swords")
	}
	// A held stack with room under the cap needs no slot.
	p.Inventory().AddItem(NewItem(995, 100))
	if !ReceiverCanHold(p, []Item{NewItem(995, 50)}) {
		t.Fatal("held stack with room must hold more coins")
	}
	// A stack at the cap cannot take more.
	p.Inventory().SetItems([]Item{NewItem(995, MaxStackAmount)})
	if ReceiverCanHold(p, []Item{NewItem(995, 1)}) {
		t.Fatal("capped stack must not hold more coins")
	}
	// A same-id stack with different metadata needs its own slot.
	meta := map[string]json.RawMessage{"charge": json.RawMessage("5")}
	p.Inventory().SetItems([]Item{NewItemWithMeta(995, 100, meta)})
	if !ReceiverCanHold(p, []Item{NewItem(995, 1)}) {
		t.Fatal("different-metadata stack must hold via a free slot")
	}
	// Fill the inventory: the different-metadata stack has
	// nowhere to go.
	items := []Item{NewItemWithMeta(995, 100, meta)}
	for i := 0; i < 27; i++ {
		items = append(items, NewItem(1000+i, 1))
	}
	p.Inventory().SetItems(items)
	if ReceiverCanHold(p, []Item{NewItem(995, 1)}) {
		t.Fatal("full inventory must not hold a different-metadata stack")
	}
	if !ReceiverCanHold(p, []Item{NewItemWithMeta(995, 1, meta)}) {
		t.Fatal("full inventory must hold onto the matching stack")
	}
}

func TestCanHold(t *testing.T) {
	defs := newTradeTestDefs()
	p := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	p.SetItemDefinitions(defs)
	inv := p.Inventory()
	if !canHold(inv, NewItem(1, 1)) {
		t.Fatal("empty inventory must hold")
	}
	for i := 0; i < 28; i++ {
		inv.AddItem(NewItem(1000+i, 1))
	}
	if canHold(inv, NewItem(1, 1)) {
		t.Fatal("full inventory must not hold an unstackable")
	}
	inv.SetItems(append([]Item{NewItem(995, 100)}, inv.Items()[1:]...))
	if !canHold(inv, NewItem(995, 1)) {
		t.Fatal("full inventory must hold onto a held stack")
	}
	meta := map[string]json.RawMessage{"charge": json.RawMessage("5")}
	if canHold(inv, NewItemWithMeta(995, 1, meta)) {
		t.Fatal("full inventory must not hold a different-metadata stack")
	}
}

func TestTradingNoSenderDropsSilently(t *testing.T) {
	// No session at all: the whole flow runs without panics
	// and the items still change hands.
	defs := newTradeTestDefs()
	a := NewPlayer("Alice", Location{X: 3200, Y: 3200, Z: 0})
	b := NewPlayer("Bob", Location{X: 3200, Y: 3200, Z: 0})
	a.SetItemDefinitions(defs)
	b.SetItemDefinitions(defs)
	a.Inventory().AddItem(NewItem(995, 100))
	a.Trading().RequestTrade(b)
	b.Trading().RequestTrade(a)
	a.Trading().HandleItem(995, 50, 0, a.Inventory(), a.Trading().Container())
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	a.Trading().AcceptTrade() // button delay blocks; advance via restart
	a.Trading().ButtonDelay().Stop()
	b.Trading().ButtonDelay().Stop()
	a.Trading().AcceptTrade()
	b.Trading().AcceptTrade()
	if b.Inventory().GetAmount(995) != 50 {
		t.Fatalf("Bob coins = %d, want 50", b.Inventory().GetAmount(995))
	}
}
