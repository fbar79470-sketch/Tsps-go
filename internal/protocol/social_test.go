package protocol

import (
	"bytes"
	"testing"
)

// The expected bytes below are hand-computed from the TS layouts
// cited in social.go, not copied from the encoders' output.

func TestEncodeFriendStatus(t *testing.T) {
	if got := EncodeFriendStatus(2); !bytes.Equal(got, []byte{221, 2}) {
		t.Errorf("EncodeFriendStatus(2) = %v, want [221 2]", got)
	}
	// PacketBuilder.put masks to 8 bits.
	if got := EncodeFriendStatus(300); !bytes.Equal(got, []byte{221, 44}) {
		t.Errorf("EncodeFriendStatus(300) = %v, want [221 44]", got)
	}
}

func TestEncodeFriend(t *testing.T) {
	hash := int64(0x0102030405060708)
	hashBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	// World 0 stays 0.
	want := append([]byte{50}, hashBytes...)
	want = append(want, 0)
	if got := EncodeFriend(hash, 0); !bytes.Equal(got, want) {
		t.Errorf("EncodeFriend(world 0) = %v, want %v", got, want)
	}
	// A non-zero world is sent as world+9.
	want = append([]byte{50}, hashBytes...)
	want = append(want, 10)
	if got := EncodeFriend(hash, 1); !bytes.Equal(got, want) {
		t.Errorf("EncodeFriend(world 1) = %v, want %v", got, want)
	}
	// The world byte is masked: 300+9 = 309 & 0xff = 53.
	want = append([]byte{50}, hashBytes...)
	want = append(want, 53)
	if got := EncodeFriend(hash, 300); !bytes.Equal(got, want) {
		t.Errorf("EncodeFriend(world 300) = %v, want %v", got, want)
	}
	// A negative hash renders mod 2^64, as toUnsignedLong does.
	if got := EncodeFriend(-1, 0); !bytes.Equal(got, []byte{50, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0}) {
		t.Errorf("EncodeFriend(-1) = %v", got)
	}
}

func TestEncodeDeleteFriendAddIgnoreDeleteIgnore(t *testing.T) {
	hash := int64(0x0102030405060708)
	hashBytes := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	for _, tc := range []struct {
		name string
		got  []byte
		op   byte
	}{
		{"delete friend", EncodeDeleteFriend(hash), 51},
		{"add ignore", EncodeAddIgnore(hash), 214},
		{"delete ignore", EncodeDeleteIgnore(hash), 215},
	} {
		want := append([]byte{tc.op}, hashBytes...)
		if !bytes.Equal(tc.got, want) {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, want)
		}
	}
}

func TestEncodeShowClanChatOptions(t *testing.T) {
	if got := EncodeShowClanChatOptions(true); !bytes.Equal(got, []byte{115, 1}) {
		t.Errorf("show = %v, want [115 1]", got)
	}
	if got := EncodeShowClanChatOptions(false); !bytes.Equal(got, []byte{115, 0}) {
		t.Errorf("hide = %v, want [115 0]", got)
	}
}

func TestLegacyOpcodesNotInServerTable(t *testing.T) {
	// 214/215/221 have no ServerPacketId entry: EncodePacket
	// must reject them, which is why the legacy encoders bypass
	// it (see social.go). 50/51/115 resolve to the NATIVE table
	// entries they collide with — proof the legacy opcodes are
	// a different family that must never be framed natively.
	for _, op := range []byte{214, 215, 221} {
		if _, ok := ServerPacketLength(op); ok {
			t.Errorf("ServerPacketLength(%d) found an entry, want none", op)
		}
	}
	if length, ok := ServerPacketLength(50); !ok || length != -2 {
		t.Errorf("ServerPacketLength(50) = %d, %v, want -2, true (inventory snapshot)", length, ok)
	}
	if length, ok := ServerPacketLength(51); !ok || length != -1 {
		t.Errorf("ServerPacketLength(51) = %d, %v, want -1, true (inventory slot)", length, ok)
	}
	if length, ok := ServerPacketLength(115); !ok || length != 4 {
		t.Errorf("ServerPacketLength(115) = %d, %v, want 4, true (widget set player head)", length, ok)
	}
}

func TestEncodeChatFilterSettings(t *testing.T) {
	if got := EncodeChatFilterSettings(0, 1, 2); !bytes.Equal(got, []byte{122, 0, 1, 2}) {
		t.Errorf("got %v, want [122 0 1 2]", got)
	}
	if got := EncodeChatFilterSettings(256, -1, 300); !bytes.Equal(got, []byte{122, 0, 255, 44}) {
		t.Errorf("masked got %v, want [122 0 255 44]", got)
	}
}

func TestEncodeFriendsChatSnapshot(t *testing.T) {
	// Empty: no channel, no friends, no ignores. Payload is the
	// present byte plus two zero counts: [0, 0, 0, 0, 0].
	if got := EncodeFriendsChatSnapshot(FriendsChatSnapshot{}); !bytes.Equal(got, []byte{121, 0, 5, 0, 0, 0, 0, 0}) {
		t.Errorf("empty snapshot = %v", got)
	}
	snapshot := FriendsChatSnapshot{
		Channel: &FriendsChatChannel{
			Name: "ch", Owner: "ow", MinKickRank: 2, LocalRank: 6,
			Members: []FriendsChatChannelMember{{Name: "m", World: 1, Rank: 3}},
		},
		Friends: []FriendsChatFriend{{Name: "f", PreviousName: "p", World: 2, Rank: 1, IsOnline: true}},
		Ignores: []FriendsChatIgnore{{Name: "i", PreviousName: "q"}},
	}
	want := []byte{121, 0, 32,
		1, 'c', 'h', 0, 'o', 'w', 0, 2, 6, 0, 1, 'm', 0, 0, 1, 3,
		0, 1, 'f', 0, 'p', 0, 0, 2, 1, 1,
		0, 1, 'i', 0, 'q', 0,
	}
	if got := EncodeFriendsChatSnapshot(snapshot); !bytes.Equal(got, want) {
		t.Errorf("snapshot = %v, want %v", got, want)
	}
	// Worlds mask to 16 bits, ranks to 8 bits; offline is 0.
	offline := FriendsChatSnapshot{
		Friends: []FriendsChatFriend{{Name: "f", PreviousName: "", World: 0x10001, Rank: 300}},
	}
	want = []byte{121, 0, 12, 0, 0, 1, 'f', 0, 0, 0, 1, 44, 0, 0, 0}
	if got := EncodeFriendsChatSnapshot(offline); !bytes.Equal(got, want) {
		t.Errorf("masked snapshot = %v, want %v", got, want)
	}
}

func TestEncodeClanSettingsFull(t *testing.T) {
	// A nil clan: clan type + absent byte only.
	if got := EncodeClanSettingsFull(1, nil); !bytes.Equal(got, []byte{124, 0, 2, 1, 0}) {
		t.Errorf("nil clan = %v, want [124 0 2 1 0]", got)
	}
	clan := &ClanSettingsSnapshot{
		UpdateNum: 1, CreationTime: 2, Name: "cl", AllowGuests: true,
		TalkRank: 3, KickRank: 4, OwnerSlot: 5, ReplacementOwnerSlot: 6,
		Members:  []ClanSettingsMember{{Name: "m", Rank: 7, Muted: true, ExtraInfo: -1, JoinRuneDay: 8}},
		Banned:   []string{"b"},
		Settings: []ClanSetting{{ID: 9, Value: 10}},
	}
	want := []byte{124, 0, 44,
		0, 1, 0, 0, 0, 1, 0, 0, 0, 2, 'c', 'l', 0, 1, 3, 4, 0, 5, 0, 6, 0, 1,
		'm', 0, 7, 1, 0xff, 0xff, 0xff, 0xff, 0, 8,
		0, 1, 'b', 0, 0, 1, 0, 9, 0, 0, 0, 10,
	}
	if got := EncodeClanSettingsFull(0, clan); !bytes.Equal(got, want) {
		t.Errorf("clan settings = %v, want %v", got, want)
	}
}

func TestEncodeClanChannelFull(t *testing.T) {
	// A nil channel is ClanChannelFull(Leave).
	if got := EncodeClanChannelFull(2, nil); !bytes.Equal(got, []byte{125, 0, 2, 2, 0}) {
		t.Errorf("nil channel = %v, want [125 0 2 2 0]", got)
	}
	channel := &ClanChannelSnapshot{
		Key: 1, UpdateNum: 2, Name: "n", TalkRank: 3, KickRank: 4,
		Users: []ClanChannelUser{{Name: "u", Rank: 5, World: 6}},
	}
	want := []byte{125, 0, 21,
		0, 1, 0, 0, 0, 1, 0, 0, 0, 2, 'n', 0, 3, 4, 0, 1, 'u', 0, 5, 0, 6,
	}
	if got := EncodeClanChannelFull(0, channel); !bytes.Equal(got, want) {
		t.Errorf("channel = %v, want %v", got, want)
	}
}

func TestEncodeClanChannelDelta(t *testing.T) {
	events := []ClanChannelDeltaEvent{
		{Type: ClanChannelDeltaAdd, User: ClanChannelUser{Name: "u", Rank: 4, World: 5}},
		{Type: ClanChannelDeltaDelete, Index: 6},
	}
	want := []byte{126, 0, 20,
		1, 0, 0, 0, 2, 0, 0, 0, 3, 0, 2,
		1, 'u', 0, 4, 0, 5, 2, 0, 6,
	}
	if got := EncodeClanChannelDelta(1, 2, 3, events); !bytes.Equal(got, want) {
		t.Errorf("delta = %v, want %v", got, want)
	}
	// Any non-add type encodes as a delete, as in TS.
	unknown := []ClanChannelDeltaEvent{{Type: "bogus", Index: 9}}
	want = []byte{126, 0, 14, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 2, 0, 9}
	if got := EncodeClanChannelDelta(0, 0, 0, unknown); !bytes.Equal(got, want) {
		t.Errorf("unknown-type delta = %v, want %v", got, want)
	}
	// No events: header only.
	if got := EncodeClanChannelDelta(0, 0, 0, nil); !bytes.Equal(got, []byte{126, 0, 11, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Errorf("empty delta = %v", got)
	}
}

func TestEncodeClanVar(t *testing.T) {
	if got := EncodeClanVar(7, -2); !bytes.Equal(got, []byte{128, 0, 7, 0xff, 0xff, 0xff, 0xfe}) {
		t.Errorf("got %v, want [128 0 7 255 255 255 254]", got)
	}
	if got := EncodeClanVar(0x10001, 1); !bytes.Equal(got, []byte{128, 0, 1, 0, 0, 0, 1}) {
		t.Errorf("masked id got %v, want [128 0 1 0 0 0 1]", got)
	}
}

func TestEncodeClanSettingsDelta(t *testing.T) {
	updates := []ClanSettingsDeltaUpdate{
		{Type: ClanSettingsDeltaAddMember, Name: "a", JoinRuneDay: 3},
		{Type: ClanSettingsDeltaDeleteMember, Index: 4},
		{Type: ClanSettingsDeltaMemberRank, Index: 5, Rank: 6},
		{Type: ClanSettingsDeltaMemberMuted, Index: 7, Muted: true},
		{Type: ClanSettingsDeltaMemberExtraInfo, Index: 8, Value: 9, StartBit: 10, EndBit: 11},
		{Type: ClanSettingsDeltaIntSetting, Setting: 12, Value: 13},
	}
	want := []byte{127, 0, 39,
		1, 0, 0, 0, 2, 0, 6,
		1, 'a', 0, 0, 3,
		2, 0, 4,
		3, 0, 5, 6,
		4, 0, 7, 1,
		5, 0, 8, 0, 0, 0, 9, 10, 11,
		6, 0, 12, 0, 0, 0, 13,
	}
	if got := EncodeClanSettingsDelta(1, 2, updates); !bytes.Equal(got, want) {
		t.Errorf("delta = %v, want %v", got, want)
	}
	// Any other type encodes as an int_setting, as in TS.
	unknown := []ClanSettingsDeltaUpdate{{Type: "bogus", Setting: 1, Value: 2}}
	want = []byte{127, 0, 14, 0, 0, 0, 0, 0, 0, 1, 6, 0, 1, 0, 0, 0, 2}
	if got := EncodeClanSettingsDelta(0, 0, unknown); !bytes.Equal(got, want) {
		t.Errorf("unknown-type delta = %v, want %v", got, want)
	}
}
