package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

func TestLegacyRelationsSendsQueueNothing(t *testing.T) {
	// The PacketBuilder sends are no-ops in TS (PlayerSession
	// .write), so the session senders queue nothing. If any of
	// these ever queues a frame, the wire stream would desync
	// (see protocol/social.go): the zero-packet assertion is
	// the safety property.
	s := NewSession(NewHeadlessChannel(""))
	s.SendFriendStatus(2)
	s.SendFriend(12345, 1)
	s.SendDeleteFriend(12345)
	s.SendAddIgnore(12345)
	s.SendDeleteIgnore(12345)
	if packets := queuedPackets(t, s); len(packets) != 0 {
		t.Fatalf("legacy sends queued %d packets, want 0", len(packets))
	}
}

func TestSendChatOptionsQueuesFilterPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendChatOptions(0, 1, 2)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := []byte{122, 0, 1, 2}
	if !bytes.Equal(packets[0], want) {
		t.Errorf("queued packet = %v, want %v", packets[0], want)
	}
}

func TestUnpackPackedText(t *testing.T) {
	// Packed vectors computed with the TS textPack (Misc.ts:
	// 417-455), run in node; the unpacked forms are the
	// intended nibble/table decode (see unpackPackedText).
	// Odd nibble counts surface textPack's padding as a
	// trailing space, which the port preserves.
	cases := []struct {
		packed []byte
		size   int
		want   string
	}{
		{[]byte{97, 187, 64}, 3, "hello "},
		{[]byte{101}, 1, "hi"},
		{[]byte{208}, 1, "m"},
		{[]byte{48}, 1, "a "},
		{[]byte{97, 187, 64, 209, 73, 186}, 6, "hello world"},
		{[]byte{61, 254, 144}, 3, "a1! "},
		{[]byte{221, 221}, 2, "zz"},
		{[]byte{33, 130, 223, 224, 225}, 5, "test123"},
		{nil, 0, ""},
		{[]byte{101}, 0, ""},
		// A size larger than the payload reads missing bytes
		// as 0 (spaces), as TS reads undefined shifts as 0.
		{[]byte{101}, 3, "hi    "},
		// A negative size decodes nothing.
		{[]byte{101}, -1, ""},
		// A dangling high nibble at the end is dropped:
		// 0x0d is nibbles [space, high-13] with no low nibble.
		{[]byte{0x0d}, 1, " "},
	}
	for _, tc := range cases {
		if got := unpackPackedText(tc.packed, tc.size); got != tc.want {
			t.Errorf("unpackPackedText(%v, %d) = %q, want %q", tc.packed, tc.size, got, tc.want)
		}
	}
}

func TestUcFirstPackedText(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", ""},
		{"a", "A"},
		{"hello ", "Hello "},
		{"HELLO", "Hello"},
		{"hELLO WORLD", "Hello world"},
	} {
		if got := ucFirstPackedText(tc.in); got != tc.want {
			t.Errorf("ucFirstPackedText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSendPrivateMessageQueuesPrivateInPacket(t *testing.T) {
	sender := game.NewPlayer("alice", game.Location{})
	sender.DisplayName = "Alice"
	sender.Index = 7
	s := NewSession(NewHeadlessChannel(""))
	// [101] unpacks to "hi"; lowercased + ucFirst => "Hi".
	s.SendPrivateMessage(sender, []byte{101}, 1)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindPrivateIn, Text: "Hi", From: "Alice", PlayerID: 7,
	})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("queued packet = %x, want %x", packets[0], want)
	}
	// The wire bytes, spelled out: opcode 120, var-short
	// length, "Hi" NUL, type 3, "Alice" NUL, NUL, id 7.
	wantBytes := []byte{120, 0, 13, 'H', 'i', 0, 3, 'A', 'l', 'i', 'c', 'e', 0, 0, 0, 7}
	if !bytes.Equal(packets[0], wantBytes) {
		t.Errorf("queued packet bytes = %v, want %v", packets[0], wantBytes)
	}
}

func TestSendPrivateMessageNilSender(t *testing.T) {
	// A nil sender sends "" and index 0, as TS's `?.`/`??`
	// fallbacks do (PacketSender.ts:852-854).
	s := NewSession(NewHeadlessChannel(""))
	s.SendPrivateMessage(nil, []byte{101}, 1)
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindPrivateIn, Text: "Hi", From: "", PlayerID: 0,
	})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("queued packet = %x, want %x", packets[0], want)
	}
}

func TestSendSpecialMessage(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendSpecialMessage("ClanName", 16, "hello clan")
	s.SendSpecialMessage("Channel", 0, "hello channel")
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued packets = %d, want 2", len(packets))
	}
	wantClan := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindClan, Text: "hello clan", From: "ClanName", PlayerID: -1,
	})
	if !bytes.Equal(packets[0], wantClan) {
		t.Errorf("clan packet = %x, want %x", packets[0], wantClan)
	}
	wantChannel := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindChannel, Text: "hello channel", From: "Channel", PlayerID: -1,
	})
	if !bytes.Equal(packets[1], wantChannel) {
		t.Errorf("channel packet = %x, want %x", packets[1], wantChannel)
	}
}
