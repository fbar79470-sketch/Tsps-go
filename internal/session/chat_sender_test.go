package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Session must satisfy the game's chat-message seam
// structurally, as it satisfies game.SessionPort (see
// sessionport_test.go): if either side drifts, the build fails
// here rather than in the coordinator.
var _ game.ChatMessageSender = (*Session)(nil)

func TestSendGameMessageQueuesChatPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendGameMessage("Hello, world.")
	packets := queuedPackets(t, s)
	if len(packets) != 1 {
		t.Fatalf("queued packets = %d, want 1", len(packets))
	}
	want := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: "Hello, world.", PlayerID: -1,
	})
	if !bytes.Equal(packets[0], want) {
		t.Errorf("queued packet = %x, want %x", packets[0], want)
	}
	// The wire bytes, spelled out: opcode 120, var-short length,
	// "Hello, world." NUL, type 0, NUL, NUL, id 0xffff.
	wantBytes := []byte{120, 0, 19, 'H', 'e', 'l', 'l', 'o', ',', ' ', 'w', 'o', 'r', 'l', 'd', '.', 0, 0, 0, 0, 0xff, 0xff}
	if !bytes.Equal(packets[0], wantBytes) {
		t.Errorf("queued packet bytes = %v, want %v", packets[0], wantBytes)
	}
}

func TestSendTypedGameMessageQueuesChatPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendTypedGameMessage(protocol.ChatTypeSpam, "You're stunned!", "")
	s.SendTypedGameMessage(protocol.ChatTypeNPCExamine, "A man.", "Man")
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued packets = %d, want 2", len(packets))
	}
	wantSpam := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: "You're stunned!", PlayerID: -1,
		ChatType: protocol.ChatTypeSpam, HasChatType: true,
	})
	if !bytes.Equal(packets[0], wantSpam) {
		t.Errorf("spam packet = %x, want %x", packets[0], wantSpam)
	}
	wantExamine := protocol.EncodeChatMessage(protocol.OutgoingChatMessage{
		Kind: protocol.ChatKindGame, Text: "A man.", From: "Man", PlayerID: -1,
		ChatType: protocol.ChatTypeNPCExamine, HasChatType: true,
	})
	if !bytes.Equal(packets[1], wantExamine) {
		t.Errorf("examine packet = %x, want %x", packets[1], wantExamine)
	}
}

func TestSendGameMessageDropsAfterClose(t *testing.T) {
	// A closed session drops queued sends, as SendPacket does:
	// no packet is queued after Close.
	s := NewSession(NewHeadlessChannel(""))
	s.Close(1000, "test")
	s.SendGameMessage("dropped")
	if packets := queuedPackets(t, s); len(packets) != 0 {
		t.Fatalf("queued packets after close = %d, want 0", len(packets))
	}
}
