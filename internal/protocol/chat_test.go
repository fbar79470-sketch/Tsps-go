package protocol

import (
	"bytes"
	"strings"
	"testing"
)

// chatFrame frames payload as CHAT_MESSAGE (opcode 120,
// var-short): opcode + 2-byte big-endian length + payload. The
// tests below build the expected payload byte by byte, so this
// helper only restates the framing EncodePacket already pins.
func chatFrame(payload ...byte) []byte {
	out := []byte{120, byte(len(payload) >> 8), byte(len(payload))}
	return append(out, payload...)
}

func chatPayload(text string, typ byte, from, prefix string, idHi, idLo byte, style ...byte) []byte {
	out := []byte(text)
	out = append(out, 0, typ)
	out = append(out, []byte(from)...)
	out = append(out, 0)
	out = append(out, []byte(prefix)...)
	out = append(out, 0, idHi, idLo)
	return append(out, style...)
}

func TestEncodeChatMessageGame(t *testing.T) {
	// sendMessage("Hello"): kind game, default type 0, no sender,
	// id -1 (0xffff). Payload: "Hello" NUL, 0, NUL, NUL, FF FF.
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: "Hello", PlayerID: -1})
	want := chatFrame(chatPayload("Hello", 0, "", "", 0xff, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("game message = %v, want %v", got, want)
	}
	// The exact wire bytes, spelled out: opcode 120, length 11.
	wantBytes := []byte{120, 0, 11, 'H', 'e', 'l', 'l', 'o', 0, 0, 0, 0, 0xff, 0xff}
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("game message bytes = %v, want %v", got, wantBytes)
	}
}

func TestEncodeChatMessageEmpty(t *testing.T) {
	// An empty game message still carries all three strings as
	// bare NULs: payload is 6 bytes.
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: "", PlayerID: -1})
	want := []byte{120, 0, 6, 0, 0, 0, 0, 0xff, 0xff}
	if !bytes.Equal(got, want) {
		t.Fatalf("empty game message = %v, want %v", got, want)
	}
}

func TestEncodeChatMessageTyped(t *testing.T) {
	// sendTypedMessage(SPAM, "You're stunned!"): the explicit
	// type byte 105 replaces the game default 0.
	got := EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindGame, Text: "You're stunned!", PlayerID: -1,
		ChatType: ChatTypeSpam, HasChatType: true,
	})
	want := chatFrame(chatPayload("You're stunned!", 105, "", "", 0xff, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("typed spam message = %v, want %v", got, want)
	}
	// sendTypedMessage(NPC_EXAMINE, text, "Man"): the sender
	// name rides in From.
	got = EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindGame, Text: "A man.", From: "Man", PlayerID: -1,
		ChatType: ChatTypeNPCExamine, HasChatType: true,
	})
	want = chatFrame(chatPayload("A man.", 28, "Man", "", 0xff, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("typed NPC examine = %v, want %v", got, want)
	}
	// An explicit chat type of 0 is honored when HasChatType is
	// set (it equals the game default, but the override path is
	// what sendTypedMessage uses); without HasChatType the
	// kind's default applies even when ChatType holds a value.
	got = EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindPublic, Text: "hi", From: "A", PlayerID: 1, ChatType: 99,
	})
	want = chatFrame(chatPayload("hi", 2, "A", "", 0, 1)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("unset override ignored = %v, want %v", got, want)
	}
}

func TestEncodeChatMessageDefaultTypes(t *testing.T) {
	// defaultTypes (ClientProtocol.ts:1197-1206): one type byte
	// per kind, read at payload offset 1 (frame offset 4).
	cases := []struct {
		kind ChatKind
		typ  byte
	}{
		{ChatKindGame, 0},
		{ChatKindPublic, 2},
		{ChatKindPrivateIn, 3},
		{ChatKindPrivateOut, 6},
		{ChatKindChannel, 9},
		{ChatKindClan, 7},
		{ChatKindTrade, 4},
		{ChatKindServer, 0},
	}
	for _, tc := range cases {
		got := EncodeChatMessage(OutgoingChatMessage{Kind: tc.kind, PlayerID: -1})
		if got[4] != tc.typ {
			t.Fatalf("kind %d type byte = %d, want %d", tc.kind, got[4], tc.typ)
		}
	}
	// A kind outside the table answers the game default 0.
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKind(99), PlayerID: -1})
	if got[4] != 0 {
		t.Fatalf("unknown kind type byte = %d, want 0", got[4])
	}
}

func TestEncodeChatMessagePublicWithStyle(t *testing.T) {
	// sendPublicChat with an autochat override and a style: type
	// 90, the sender name, the player index, then the style
	// bytes (colour, effect) after the index.
	got := EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindPublic, Text: "hi", From: "A", PlayerID: 1,
		ChatType: ChatTypeAutotyper, HasChatType: true,
		Style: &ChatStyle{Colour: 1, Effect: 2},
	})
	want := chatFrame(chatPayload("hi", 90, "A", "", 0, 1, 1, 2)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("public autochat = %v, want %v", got, want)
	}
	// A pattern colour appends its indices after colour/effect.
	got = EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindPublic, Text: "hi", From: "A", PlayerID: 1,
		Style: &ChatStyle{Colour: 13, Effect: 0, Pattern: []byte{5}},
	})
	want = chatFrame(chatPayload("hi", 2, "A", "", 0, 1, 13, 0, 5)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("public pattern style = %v, want %v", got, want)
	}
	// A nil style appends nothing.
	got = EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindPublic, Text: "hi", From: "A", PlayerID: 1})
	want = chatFrame(chatPayload("hi", 2, "A", "", 0, 1)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("public no style = %v, want %v", got, want)
	}
}

func TestEncodeChatMessagePrefix(t *testing.T) {
	// The prefix field rides between From and the player id;
	// sendMessage/sendTypedMessage always pass "".
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: "x", Prefix: "pre", PlayerID: -1})
	want := chatFrame(chatPayload("x", 0, "", "pre", 0xff, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("prefixed message = %v, want %v", got, want)
	}
}

func TestEncodeChatMessageMasking(t *testing.T) {
	// TS masks the type byte with & 0xff and the id with
	// & 0xffff; style colour/effect with & 0xff.
	got := EncodeChatMessage(OutgoingChatMessage{
		Kind: ChatKindGame, Text: "x", PlayerID: 0x10005,
		ChatType: 0x1ff, HasChatType: true,
		Style: &ChatStyle{Colour: 0x101, Effect: -1},
	})
	want := chatFrame(chatPayload("x", 0xff, "", "", 0, 5, 1, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("masked message = %v, want %v", got, want)
	}
	// Player id -2 keeps its low 16 bits.
	got = EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: "x", PlayerID: -2})
	want = chatFrame(chatPayload("x", 0, "", "", 0xff, 0xfe)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("id -2 = %v, want %v", got, want)
	}
}

func TestEncodeChatMessageLatin1(t *testing.T) {
	// Strings are latin1 NUL-terminated, as in every other
	// encoder: 'é' (U+00E9) is the single byte 0xe9.
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: "café", PlayerID: -1})
	// The expected payload spells the latin1 byte out: 'é'
	// (U+00E9) is the single byte 0xe9, not its UTF-8 form.
	want := chatFrame([]byte{'c', 'a', 'f', 0xe9, 0, 0, 0, 0, 0xff, 0xff}...)
	if !bytes.Equal(got, want) {
		t.Fatalf("latin1 message = %v, want %v", got, want)
	}
}

func TestEncodeChatMessageLegacyColours(t *testing.T) {
	// Game messages rewrite known legacy tags to <col=...>;
	// unknown, uppercase, and wrong-length tags stay verbatim.
	cases := []struct{ in, want string }{
		{"@red@Hello", "<col=ff0000>Hello"},
		{"@gre@a @blu@b", "<col=00ff00>a <col=0000ff>b"},
		{"@or1@x @gr3@y", "<col=ffb000>x <col=40ff00>y"},
		{"@zzz@Hi", "@zzz@Hi"},
		{"@RED@Hi", "@RED@Hi"},
		{"@re@Hi", "@re@Hi"},
		{"@redd@Hi", "@redd@Hi"},
		{"plain", "plain"},
		{"", ""},
	}
	for _, tc := range cases {
		got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: tc.in, PlayerID: -1})
		want := chatFrame(chatPayload(tc.want, 0, "", "", 0xff, 0xff)...)
		if !bytes.Equal(got, want) {
			t.Fatalf("legacy colours %q = %v, want %v", tc.in, got, want)
		}
	}
	// Server messages rewrite too; public messages do not.
	got := EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindServer, Text: "@red@x", PlayerID: -1})
	want := chatFrame(chatPayload("<col=ff0000>x", 0, "", "", 0xff, 0xff)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("server legacy colours = %v, want %v", got, want)
	}
	got = EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindPublic, Text: "@red@x", From: "A", PlayerID: 1})
	want = chatFrame(chatPayload("@red@x", 2, "A", "", 0, 1)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("public keeps legacy tags = %v, want %v", got, want)
	}
}

func TestResolveChatHighlights(t *testing.T) {
	cases := []struct {
		in, want    string
		transparent bool
	}{
		{"@mes_hl_gre@text</col>", "<col=06600c>text</col>", false},
		{"@mes_hl_gre@text</col>", "<col=229628>text</col>", true},
		{"@mes_hl_red@x", "<col=ef1020>x", false},
		{"@mes_hl_red@x", "<col=ef1020>x", true},
		{"@mes_hl_ora@x", "<col=ff9040>x", false},
		{"@mes_hl_pur@x", "<col=6800bf>x", false},
		{"@mes_hl_pur@x", "<col=a53fff>x", true},
		{"@mes_hl_pnk@x", "@mes_hl_pnk@x", false},
		{"@mes_hl_GRE@x", "@mes_hl_GRE@x", false},
		{"a @mes_hl_gre@b @mes_hl_red@c", "a <col=06600c>b <col=ef1020>c", false},
		{"plain", "plain", false},
		{"", "", false},
	}
	for _, tc := range cases {
		if got := ResolveChatHighlights(tc.in, tc.transparent); got != tc.want {
			t.Fatalf("ResolveChatHighlights(%q, %v) = %q, want %q", tc.in, tc.transparent, got, tc.want)
		}
	}
	// The highlight table itself, pinned to the TS values.
	if ChatHighlights["gre"] != (ChatHighlight{Opaque: "06600c", Transparent: "229628"}) {
		t.Fatalf("ChatHighlights[gre] = %+v", ChatHighlights["gre"])
	}
}

func TestEncodeChatMessageOversizePanics(t *testing.T) {
	// A payload over 65535 bytes panics, mirroring the TS
	// encodeServerPacket throw (only a >64 KiB text reaches it).
	defer func() {
		if recover() == nil {
			t.Fatal("oversize chat message did not panic")
		}
	}()
	EncodeChatMessage(OutgoingChatMessage{Kind: ChatKindGame, Text: strings.Repeat("a", 70000), PlayerID: -1})
}

func TestChatOpcodesAndTypes(t *testing.T) {
	// CHAT_MESSAGE is opcode 120, var-short (-2), per
	// ServerPackets.ts:100,263.
	if ServerChatMessage != 120 {
		t.Fatalf("ServerChatMessage = %d, want 120", ServerChatMessage)
	}
	length, ok := ServerPacketLength(ServerChatMessage)
	if !ok || length != -2 {
		t.Fatalf("ServerPacketLength(120) = %d, %v, want -2, true", length, ok)
	}
	// The ChatType values, pinned to ClientProtocol.ts:1101-1125.
	types := map[string]int{
		"game": 0, "autotyper": 90, "modautotyper": 91, "broadcast": 14,
		"friends_chat_notification": 11, "spam": 105, "trade": 102,
		"npc_say": 116, "mesbox": 115, "welcome": 108,
		"npc_examine": 28, "loc_examine": 29,
	}
	got := map[string]int{
		"game": ChatTypeGame, "autotyper": ChatTypeAutotyper,
		"modautotyper": ChatTypeModAutotyper, "broadcast": ChatTypeBroadcast,
		"friends_chat_notification": ChatTypeFriendsChatNotification,
		"spam":                      ChatTypeSpam, "trade": ChatTypeTrade,
		"npc_say": ChatTypeNPCSay, "mesbox": ChatTypeMesbox,
		"welcome": ChatTypeWelcome, "npc_examine": ChatTypeNPCExamine,
		"loc_examine": ChatTypeLocExamine,
	}
	for name, want := range types {
		if got[name] != want {
			t.Fatalf("ChatType %s = %d, want %d", name, got[name], want)
		}
	}
}
