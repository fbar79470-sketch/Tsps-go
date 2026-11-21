package plugins

import (
	"reflect"
	"strings"
	"testing"

	"tsps-go/internal/game"
)

// testPlayer builds a bare player for event payloads. Location is
// irrelevant to the hook tests; the zero tile is fine.
func testPlayer(username string) *game.Player {
	return game.NewPlayer(username, game.Location{})
}

func TestEmitOrderingSubscriptionOrder(t *testing.T) {
	h, _ := newTestHub()
	var order []string
	newApi(h, "First").OnPlayerLogin(func(*PlayerLoginEvent) { order = append(order, "First") })
	newApi(h, "Second").OnPlayerLogin(func(*PlayerLoginEvent) { order = append(order, "Second") })
	newApi(h, "First").OnPlayerLogin(func(*PlayerLoginEvent) { order = append(order, "First-again") })
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice"), Username: "alice"})
	want := []string{"First", "Second", "First-again"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestEmitErrorIsolation(t *testing.T) {
	h, buf := newTestHub()
	var ran []string
	newApi(h, "boom-plugin").OnPlayerLogin(func(*PlayerLoginEvent) {
		ran = append(ran, "boom")
		panic("kaboom")
	})
	newApi(h, "steady").OnPlayerLogin(func(*PlayerLoginEvent) { ran = append(ran, "steady") })
	// The emit must return normally despite the panic.
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice"), Username: "alice"})
	if len(ran) != 2 || ran[0] != "boom" || ran[1] != "steady" {
		t.Fatalf("ran = %v, want [boom steady]: later hooks must still run", ran)
	}
	logged := buf.String()
	if !strings.Contains(logged, "[plugins] login hook failed (boom-plugin)") {
		t.Fatalf("log = %q, want the TS-shaped failure line", logged)
	}
	if !strings.Contains(logged, "kaboom") {
		t.Fatalf("log = %q, want the recovered value", logged)
	}
}

func TestExecuteHookLabels(t *testing.T) {
	// Every lifecycle family logs its TS errorLabel on failure.
	cases := []struct {
		label string
		emit  func(h *Hub)
	}{
		{"server_startup", func(h *Hub) {
			newApi(h, "p").OnServerStartup(func(*ServerLifecycleEvent) { panic("x") })
			h.EmitServerStartup(&ServerLifecycleEvent{})
		}},
		{"server_shutdown", func(h *Hub) {
			newApi(h, "p").OnServerShutdown(func(*ServerLifecycleEvent) { panic("x") })
			h.EmitServerShutdown(&ServerLifecycleEvent{})
		}},
		{"logout", func(h *Hub) {
			newApi(h, "p").OnPlayerLogout(func(*PlayerLogoutEvent) { panic("x") })
			h.EmitPlayerLogout(&PlayerLogoutEvent{Player: testPlayer("a"), Username: "a"})
		}},
		{"disconnect", func(h *Hub) {
			newApi(h, "p").OnPlayerDisconnect(func(*PlayerDisconnectEvent) { panic("x") })
			h.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: testPlayer("a"), Username: "a", Source: "removed"})
		}},
		{"player_process", func(h *Hub) {
			newApi(h, "p").OnPlayerProcess(func(*PlayerProcessEvent) { panic("x") })
			h.EmitPlayerProcess(&PlayerProcessEvent{Player: testPlayer("a")})
		}},
	}
	for _, c := range cases {
		h, buf := newTestHub()
		c.emit(h)
		if want := "[plugins] " + c.label + " hook failed (p)"; !strings.Contains(buf.String(), want) {
			t.Errorf("label %s: log = %q, want it to contain %q", c.label, buf.String(), want)
		}
	}
}

func TestEmitGuards(t *testing.T) {
	h, _ := newTestHub()
	called := 0
	api := newApi(h, "p")
	api.OnPlayerLogin(func(*PlayerLoginEvent) { called++ })
	api.OnPlayerLogout(func(*PlayerLogoutEvent) { called++ })
	api.OnPlayerDisconnect(func(*PlayerDisconnectEvent) { called++ })
	api.OnPlayerProcess(func(*PlayerProcessEvent) { called++ })
	api.OnServerStartup(func(*ServerLifecycleEvent) { called++ })

	h.EmitPlayerLogin(nil)
	h.EmitPlayerLogin(&PlayerLoginEvent{Username: "alice"})           // nil Player
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice")}) // empty Username
	h.EmitPlayerLogout(nil)
	h.EmitPlayerLogout(&PlayerLogoutEvent{Player: testPlayer("alice")})
	h.EmitPlayerDisconnect(nil)
	h.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: testPlayer("alice")})
	h.EmitPlayerProcess(nil)
	h.EmitPlayerProcess(&PlayerProcessEvent{})
	h.EmitServerStartup(nil)
	if called != 0 {
		t.Fatalf("guarded emits ran %d hooks, want 0", called)
	}

	// Valid events run.
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("alice"), Username: "alice"})
	h.EmitPlayerLogout(&PlayerLogoutEvent{Player: testPlayer("alice"), Username: "alice"})
	h.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: testPlayer("alice"), Username: "alice", Source: "removed"})
	h.EmitPlayerProcess(&PlayerProcessEvent{Player: testPlayer("alice")})
	h.EmitServerStartup(&ServerLifecycleEvent{Timestamp: 1})
	if called != 5 {
		t.Fatalf("valid emits ran %d hooks, want 5", called)
	}
}

func TestLifecycleEventFields(t *testing.T) {
	h, _ := newTestHub()
	var login *PlayerLoginEvent
	var disc *PlayerDisconnectEvent
	var life *ServerLifecycleEvent
	newApi(h, "p").OnPlayerLogin(func(e *PlayerLoginEvent) { login = e })
	newApi(h, "p").OnPlayerDisconnect(func(e *PlayerDisconnectEvent) { disc = e })
	newApi(h, "p").OnServerStartup(func(e *ServerLifecycleEvent) { life = e })

	p := testPlayer("alice")
	h.EmitPlayerLogin(&PlayerLoginEvent{Player: p, Username: "alice", IsNewAccount: true, Profile: "league"})
	if login == nil || login.Player != p || login.Username != "alice" || !login.IsNewAccount || login.Profile != "league" {
		t.Fatalf("login event = %+v, want player/username/new-account/league carried", login)
	}
	h.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: p, Username: "alice", Source: "logout"})
	if disc == nil || disc.Source != "logout" {
		t.Fatalf("disconnect event = %+v, want Source logout", disc)
	}
	h.EmitServerStartup(&ServerLifecycleEvent{Timestamp: 123456789})
	if life == nil || life.Timestamp != 123456789 {
		t.Fatalf("lifecycle event = %+v, want Timestamp 123456789", life)
	}
}

func TestEmitOnNilOrBareHub(t *testing.T) {
	var nilHub *Hub
	nilHub.EmitServerStartup(&ServerLifecycleEvent{})
	nilHub.EmitServerShutdown(&ServerLifecycleEvent{})
	nilHub.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("a"), Username: "a"})
	nilHub.EmitPlayerLogout(&PlayerLogoutEvent{Player: testPlayer("a"), Username: "a"})
	nilHub.EmitPlayerDisconnect(&PlayerDisconnectEvent{Player: testPlayer("a"), Username: "a"})
	nilHub.EmitPlayerProcess(&PlayerProcessEvent{Player: testPlayer("a")})
	nilHub.EmitCustomEvent("x:y", nil)
	if nilHub.loaded() {
		t.Fatal("nil hub reports loaded")
	}
	bare := &Hub{}
	bare.EmitServerStartup(&ServerLifecycleEvent{})
	bare.EmitPlayerLogin(&PlayerLoginEvent{Player: testPlayer("a"), Username: "a"})
	bare.EmitCustomEvent("x:y", nil)
	if bare.loaded() {
		t.Fatal("bare hub reports loaded")
	}
}

// TestEmitSocialPacketAllKinds proves every packet kind of the TS union
// (PluginTypes.ts:67-79) reaches the social-packet hooks with its exact
// fields, in registration order with handled short-circuit. The emit
// validates nothing about the kind or the field combination
// (PluginManager.ts:836-843 checks only player/handled), so the
// empty-kind row emits too: no kind filtering is invented here. The
// friends_chat_action rows cover both a ranked action
// (set_friend_rank, which carries Rank) and an unranked one (join),
// the two shapes of the flattened FriendsChatAction
// (ClientProtocol.ts:276-284).
func TestEmitSocialPacketAllKinds(t *testing.T) {
	kinds := []struct {
		name   string
		packet game.SocialPacket
	}{
		{"public_chat", game.SocialPacket{Kind: SocialPacketPublicChat, Text: "hello", AutoChat: true}},
		{"clan_chat", game.SocialPacket{Kind: SocialPacketClanChat, ClanType: 1, Text: "clan hello"}},
		{"clan_channel_action", game.SocialPacket{Kind: SocialPacketClanChannelAction, Action: "kick", Name: "badactor", Value: 1}},
		{"friends_chat_action/set_friend_rank", game.SocialPacket{Kind: SocialPacketFriendsChatAction, Action: "set_friend_rank", Name: "friend", Rank: 3}},
		{"friends_chat_action/join", game.SocialPacket{Kind: SocialPacketFriendsChatAction, Action: "join", Name: "channelowner"}},
		{"private_message", game.SocialPacket{Kind: SocialPacketPrivateMessage, Recipient: "someone", Text: "hi there"}},
		{"chat_filter", game.SocialPacket{Kind: SocialPacketChatFilter, PublicMode: 1, PrivateMode: 2, TradeMode: 0}},
		{"report_abuse", game.SocialPacket{Kind: SocialPacketReportAbuse, Name: "cheater", RuleID: 5, Mute: true}},
		{"chat", game.SocialPacket{Kind: SocialPacketChat, Text: "fc hello", MessageType: "friends_chat"}},
		{"empty kind (no validation)", game.SocialPacket{}},
	}
	for _, tc := range kinds {
		t.Run(tc.name, func(t *testing.T) {
			h, _ := newTestHub()
			a := newApi(h, "Watcher")
			p := testPlayer("alice")
			var order []string
			var got *game.SocialPacket
			a.OnSocialPacket(func(e *SocialPacketEvent) {
				order = append(order, "observe")
				pkt := e.Packet
				got = &pkt
			})
			a.OnSocialPacket(func(e *SocialPacketEvent) { order = append(order, "claim"); e.Handled = true })
			a.OnSocialPacket(func(e *SocialPacketEvent) { order = append(order, "skipped") })
			event := &SocialPacketEvent{Player: p, Packet: tc.packet}
			if !h.EmitSocialPacket(event) {
				t.Fatal("EmitSocialPacket = false, want true")
			}
			if !reflect.DeepEqual(order, []string{"observe", "claim"}) {
				t.Errorf("order = %v, want [observe claim]", order)
			}
			if got == nil {
				t.Fatal("hook never ran")
			}
			if !reflect.DeepEqual(*got, tc.packet) {
				t.Errorf("packet = %+v, want %+v", *got, tc.packet)
			}
		})
	}
}

// TestSocialPacketKindConstants proves the plugins kind aliases carry
// the TS `type` strings (PluginTypes.ts:67-79, contract C1) as
// game.SocialPacketKind values, eight distinct values in all. The map
// keys type-check the aliases as game kind values at compile time.
func TestSocialPacketKindConstants(t *testing.T) {
	want := map[game.SocialPacketKind]string{
		SocialPacketPublicChat:        "public_chat",
		SocialPacketClanChat:          "clan_chat",
		SocialPacketClanChannelAction: "clan_channel_action",
		SocialPacketFriendsChatAction: "friends_chat_action",
		SocialPacketPrivateMessage:    "private_message",
		SocialPacketChatFilter:        "chat_filter",
		SocialPacketReportAbuse:       "report_abuse",
		SocialPacketChat:              "chat",
	}
	if len(want) != 8 {
		t.Fatalf("distinct kinds = %d, want 8", len(want))
	}
	for kind, s := range want {
		if string(kind) != s {
			t.Errorf("kind %q, want %q", string(kind), s)
		}
	}
}
