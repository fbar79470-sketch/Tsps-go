package session

import (
	"bytes"
	"testing"

	"tsps-go/internal/game"
	"tsps-go/internal/protocol"
)

// Session must satisfy the game's multi-icon seam
// structurally, as it satisfies game.SessionPort and
// game.ChatMessageSender (see sessionport_test.go and
// chat_sender_test.go): if either side drifts, the build
// fails here rather than in the coordinator.
var _ game.MultiIconSender = (*Session)(nil)

func TestSendMultiIconQueuesVarbitPacket(t *testing.T) {
	s := NewSession(NewHeadlessChannel(""))
	s.SendMultiIcon(1)
	s.SendMultiIcon(0)
	packets := queuedPackets(t, s)
	if len(packets) != 2 {
		t.Fatalf("queued packets = %d, want 2", len(packets))
	}
	// The wire bytes, spelled out: opcode 42 (VARBIT),
	// varbit id 4605, value int32.
	if want := []byte{42, 0x11, 0xFD, 0, 0, 0, 1}; !bytes.Equal(packets[0], want) {
		t.Errorf("packet[0] = %x, want %x", packets[0], want)
	}
	if want := protocol.EncodeMultiIcon(0); !bytes.Equal(packets[1], want) {
		t.Errorf("packet[1] = %x, want %x", packets[1], want)
	}
}
