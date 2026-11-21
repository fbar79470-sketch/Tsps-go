package game

import "testing"

func TestSocialPacketKinds(t *testing.T) {
	// The kind strings are the TS discriminant strings, verbatim
	// (PluginTypes.ts:70-78).
	cases := map[SocialPacketKind]string{
		SocialPacketPublicChat:        "public_chat",
		SocialPacketClanChat:          "clan_chat",
		SocialPacketClanChannelAction: "clan_channel_action",
		SocialPacketFriendsChatAction: "friends_chat_action",
		SocialPacketPrivateMessage:    "private_message",
		SocialPacketChatFilter:        "chat_filter",
		SocialPacketReportAbuse:       "report_abuse",
		SocialPacketChat:              "chat",
	}
	if len(cases) != 8 {
		t.Fatalf("kind constants = %d, want 8", len(cases))
	}
	for kind, want := range cases {
		if string(kind) != want {
			t.Errorf("kind %q, want %q", string(kind), want)
		}
	}
}

func TestSocialPacketVariants(t *testing.T) {
	// Plain data: each variant holds its fields, the rest stay zero.
	packets := []SocialPacket{
		{Kind: SocialPacketPublicChat, Text: "hello", AutoChat: true},
		{Kind: SocialPacketClanChat, ClanType: -1, Text: "hi clan"},
		{Kind: SocialPacketClanChannelAction, Action: "kick", Name: "bob", Value: 1},
		{Kind: SocialPacketFriendsChatAction, Action: "set_friend_rank", Name: "bob", Rank: -1},
		{Kind: SocialPacketFriendsChatAction, Action: "leave"},
		{Kind: SocialPacketPrivateMessage, Recipient: "bob", Text: "hey"},
		{Kind: SocialPacketChatFilter, PublicMode: 4, PrivateMode: 1, TradeMode: 2},
		{Kind: SocialPacketReportAbuse, Name: "bob", RuleID: 3, Mute: true},
		{Kind: SocialPacketChat, Text: "fc hello", MessageType: "friends_chat"},
	}
	for _, pkt := range packets {
		if pkt.Kind == "" {
			t.Error("packet with empty kind")
		}
	}
	// Spot-check flattened fields, including signed values.
	chat := packets[1]
	if chat.ClanType != -1 {
		t.Errorf("clan chat ClanType = %d, want -1 (guest clan)", chat.ClanType)
	}
	rank := packets[3]
	if rank.Rank != -1 {
		t.Errorf("set_friend_rank Rank = %d, want -1", rank.Rank)
	}
	leave := packets[4]
	if leave.Name != "" || leave.Rank != 0 {
		t.Errorf("leave carries Name/Rank: %+v", leave)
	}
	filter := packets[6]
	if filter.PublicMode != 4 || filter.PrivateMode != 1 || filter.TradeMode != 2 {
		t.Errorf("chat filter = %+v", filter)
	}
	// The zero packet is the zero value: no kind, no fields.
	var zero SocialPacket
	if zero.Kind != "" || zero.Text != "" || zero.AutoChat || zero.Mute {
		t.Errorf("zero packet = %+v", zero)
	}
}
