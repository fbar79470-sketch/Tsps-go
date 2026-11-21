package listeners

import (
	"reflect"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// TestHandleSocialPackets pins the social-packet dispatch: each
// of the five opcodes decodes, guards on the player, builds the
// exact game.SocialPacket, and emits it through Deps.SocialEmit,
// returning Implemented regardless of the emit's handled result
// (TS core takes no other action — NetworkBuilder.ts:905-911).
func TestHandleSocialPackets(t *testing.T) {
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
		want    game.SocialPacket
	}{
		{
			name:    "clan channel kick",
			opcode:  protocol.ClientClanChannelAction,
			payload: []byte{0, 3, 'b', 'o', 'b', 0},
			want: game.SocialPacket{
				Kind:   game.SocialPacketClanChannelAction,
				Action: "kick", Name: "bob", Value: 3,
			},
		},
		{
			name:    "friends chat set rank",
			opcode:  protocol.ClientFriendsChatAction,
			payload: []byte{5, 'a', 'l', 0, 0xff},
			want: game.SocialPacket{
				Kind:   game.SocialPacketFriendsChatAction,
				Action: "set_friend_rank", Name: "al", Rank: -1,
			},
		},
		{
			name:    "friends chat leave drops name and rank",
			opcode:  protocol.ClientFriendsChatAction,
			payload: []byte{1, 'x', 0, 7},
			want: game.SocialPacket{
				Kind:   game.SocialPacketFriendsChatAction,
				Action: "leave",
			},
		},
		{
			name:    "private message",
			opcode:  protocol.ClientPrivateMessage,
			payload: []byte{'b', 'o', 'b', 0, 'h', 'i', 0},
			want: game.SocialPacket{
				Kind:      game.SocialPacketPrivateMessage,
				Recipient: "bob", Text: "hi",
			},
		},
		{
			name:    "chat filter",
			opcode:  protocol.ClientChatFilter,
			payload: []byte{0, 1, 2},
			want: game.SocialPacket{
				Kind:       game.SocialPacketChatFilter,
				PublicMode: 0, PrivateMode: 1, TradeMode: 2,
			},
		},
		{
			name:    "report abuse muted",
			opcode:  protocol.ClientReportAbuse,
			payload: []byte{'b', 0, 5, 1},
			want: game.SocialPacket{
				Kind: game.SocialPacketReportAbuse,
				Name: "b", RuleID: 5, Mute: true,
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for _, handled := range []bool{false, true} {
				p := testPlayer()
				var got *game.SocialPacket
				var gotPlayer *game.Player
				deps := Deps{SocialEmit: func(pl *game.Player, pkt game.SocialPacket) bool {
					got = &pkt
					gotPlayer = pl
					return handled
				}}
				out := Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 7, deps)
				if out != (Outcome{Recognized: true, Implemented: true}) {
					t.Fatalf("handled=%v: Handle = %+v, want implemented", handled, out)
				}
				if got == nil {
					t.Fatalf("handled=%v: SocialEmit not called", handled)
				}
				if gotPlayer != p {
					t.Fatalf("handled=%v: emit player = %p, want %p", handled, gotPlayer, p)
				}
				if !reflect.DeepEqual(*got, tc.want) {
					t.Fatalf("handled=%v: emitted packet = %+v, want %+v", handled, *got, tc.want)
				}
			}
		})
	}
}

func TestHandleSocialPacketsNilEmit(t *testing.T) {
	// No hooks wired: the packet is routed with nothing
	// observing it, and the outcome is still Implemented —
	// nil is the no-hooks default, not a drop.
	p := testPlayer()
	for _, opcode := range []byte{
		protocol.ClientClanChannelAction, protocol.ClientFriendsChatAction,
		protocol.ClientPrivateMessage, protocol.ClientChatFilter,
		protocol.ClientReportAbuse,
	} {
		payload := socialPayload(opcode)
		out := Handle(p, game.TurnPacket{Opcode: opcode, Payload: payload}, 7, Deps{})
		if out != (Outcome{Recognized: true, Implemented: true}) {
			t.Errorf("opcode %d nil emit: Handle = %+v, want implemented", opcode, out)
		}
	}
}

func TestHandleSocialPacketsNilPlayer(t *testing.T) {
	// The `if (this.player)` guard runs AFTER decode: a nil
	// player drops with ReasonNoPlayer, and the emit never
	// runs. A malformed payload with a nil player reports
	// malformed, not NoPlayer.
	emitted := false
	deps := Deps{SocialEmit: func(*game.Player, game.SocialPacket) bool { emitted = true; return true }}
	for _, opcode := range []byte{
		protocol.ClientClanChannelAction, protocol.ClientFriendsChatAction,
		protocol.ClientPrivateMessage, protocol.ClientChatFilter,
		protocol.ClientReportAbuse,
	} {
		out := Handle(nil, game.TurnPacket{Opcode: opcode, Payload: socialPayload(opcode)}, 7, deps)
		if out != (Outcome{Recognized: true, Reason: ReasonNoPlayer}) {
			t.Errorf("opcode %d nil player: Handle = %+v, want ReasonNoPlayer", opcode, out)
		}
		out = Handle(nil, game.TurnPacket{Opcode: opcode, Payload: nil}, 7, deps)
		if out != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
			t.Errorf("opcode %d nil player empty payload: Handle = %+v, want malformed", opcode, out)
		}
	}
	if emitted {
		t.Error("SocialEmit ran with a nil player")
	}
}

func TestHandleSocialPacketsMalformed(t *testing.T) {
	p := testPlayer()
	cases := []struct {
		name    string
		opcode  byte
		payload []byte
	}{
		{"clan empty", protocol.ClientClanChannelAction, nil},
		{"clan truncated", protocol.ClientClanChannelAction, []byte{0}},
		{"clan unknown action", protocol.ClientClanChannelAction, []byte{3, 0, 'x', 0}},
		{"friends empty", protocol.ClientFriendsChatAction, nil},
		{"friends truncated", protocol.ClientFriendsChatAction, []byte{0, 'x', 0}},
		{"friends unknown action", protocol.ClientFriendsChatAction, []byte{8, 'x', 0, 0}},
		{"private empty", protocol.ClientPrivateMessage, nil},
		{"private truncated", protocol.ClientPrivateMessage, []byte{'x', 0}},
		{"filter short", protocol.ClientChatFilter, []byte{0, 1}},
		{"filter long", protocol.ClientChatFilter, []byte{0, 1, 2, 3}},
		{"abuse empty", protocol.ClientReportAbuse, nil},
		{"abuse truncated", protocol.ClientReportAbuse, []byte{'x', 0, 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			emitted := false
			deps := Deps{SocialEmit: func(*game.Player, game.SocialPacket) bool { emitted = true; return true }}
			out := Handle(p, game.TurnPacket{Opcode: tc.opcode, Payload: tc.payload}, 7, deps)
			if out != (Outcome{Recognized: true, Reason: ReasonMalformedPayload}) {
				t.Fatalf("Handle = %+v, want malformed", out)
			}
			if emitted {
				t.Error("SocialEmit ran for a malformed payload")
			}
		})
	}
}

func TestHandleChatSocialKindsEmit(t *testing.T) {
	// CHAT kind 4 (clan) and kind 2 (friends_chat) emit through
	// Deps.SocialEmit instead of the chat listener; the game
	// kind still takes no action, and the public kind still
	// broadcasts.
	p := testPlayer()
	var got *game.SocialPacket
	deps := Deps{SocialEmit: func(_ *game.Player, pkt game.SocialPacket) bool { got = &pkt; return false }}
	// Clan: kind 4, signed clan-type byte (-1 = guest clan),
	// then the text.
	out := Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{4, 0xff, 'c', 0}}, 7, deps)
	if out != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("clan chat Handle = %+v, want implemented", out)
	}
	wantClan := game.SocialPacket{Kind: game.SocialPacketClanChat, ClanType: -1, Text: "c"}
	if got == nil || !reflect.DeepEqual(*got, wantClan) {
		t.Fatalf("clan chat packet = %+v, want %+v", got, wantClan)
	}
	// Friends chat: kind 2, the text, MessageType friends_chat.
	got = nil
	out = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{2, 'f', 0}}, 7, deps)
	if out != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("friends chat Handle = %+v, want implemented", out)
	}
	wantFriends := game.SocialPacket{Kind: game.SocialPacketChat, Text: "f", MessageType: "friends_chat"}
	if got == nil || !reflect.DeepEqual(*got, wantFriends) {
		t.Fatalf("friends chat packet = %+v, want %+v", got, wantFriends)
	}
	// Nil emit: routed, still Implemented, no panic.
	got = nil
	out = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{4, 0, 'c', 0}}, 7, Deps{})
	if out != (Outcome{Recognized: true, Implemented: true}) {
		t.Fatalf("clan chat nil emit Handle = %+v, want implemented", out)
	}
	// Game kind: no action, no emit.
	got = nil
	out = Handle(p, game.TurnPacket{Opcode: protocol.ClientChat, Payload: []byte{1, 'g', 0}}, 7, deps)
	if out != (Outcome{Recognized: true, Reason: ReasonNoAction}) {
		t.Fatalf("game chat Handle = %+v, want ReasonNoAction", out)
	}
	if got != nil {
		t.Errorf("game chat emitted %+v, want no emit", *got)
	}
}

func TestSocialPacketFromChat(t *testing.T) {
	// The converter covers only the two social kinds; any other
	// kind yields the zero packet, which no caller emits.
	clan, err := protocol.DecodeChat([]byte{4, 0, 'c', 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := socialPacketFromChat(clan); got != (game.SocialPacket{Kind: game.SocialPacketClanChat, Text: "c"}) {
		t.Errorf("clan = %+v", got)
	}
	friends, err := protocol.DecodeChat([]byte{2, 'f', 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := socialPacketFromChat(friends); got != (game.SocialPacket{Kind: game.SocialPacketChat, Text: "f", MessageType: "friends_chat"}) {
		t.Errorf("friends = %+v", got)
	}
	public, err := protocol.DecodeChat([]byte{0, 'p', 0})
	if err != nil {
		t.Fatal(err)
	}
	if got := socialPacketFromChat(public); got != (game.SocialPacket{}) {
		t.Errorf("public = %+v, want zero packet", got)
	}
}

// socialPayload returns a minimal valid payload for a social
// opcode, for the nil-player/nil-emit tests.
func socialPayload(opcode byte) []byte {
	switch opcode {
	case protocol.ClientClanChannelAction:
		return []byte{0, 0, 0}
	case protocol.ClientFriendsChatAction:
		return []byte{1, 0, 0}
	case protocol.ClientPrivateMessage:
		return []byte{0, 0}
	case protocol.ClientChatFilter:
		return []byte{0, 0, 0}
	case protocol.ClientReportAbuse:
		return []byte{0, 0, 0}
	}
	return nil
}
